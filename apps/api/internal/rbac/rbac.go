// Package rbac implements Polaris's role/capability/context permission
// model, following Moodle's accesslib. A role is a bundle of capability
// permissions, a context is a node in the site → category → course →
// module tree, and a role assignment grants a role to a user at one node.
//
// A permission check walks the path from the site down to the target
// context. For every role the user holds anywhere on that path, the role's
// default permission is adjusted by any override set on a context along the
// way (allow, prevent, or prohibit — a prohibit cannot be undone further
// down). Across roles, one prohibit denies; otherwise one allow grants.
// Site administrators pass every check.
package rbac

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	ContextLevelSystem   = "system"
	ContextLevelCategory = "category"
	ContextLevelCourse   = "course"
	ContextLevelModule   = "module"
	ContextLevelUser     = "user"

	PermissionAllow    = "allow"
	PermissionPrevent  = "prevent"
	PermissionProhibit = "prohibit"
)

// ErrContextNotFound is returned when the instance a context should wrap
// (a course, category or module) does not exist.
var ErrContextNotFound = errors.New("context instance not found")

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// SystemContextID returns the id of the singleton system-level context.
func (s *Service) SystemContextID(ctx context.Context) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `SELECT id FROM contexts WHERE level = 'system'`).Scan(&id)
	return id, err
}

// ContextID returns the context row for (level, instanceID), creating it on
// first use and refreshing its parent so the tree stays correct when a
// course moves category. Contexts are created lazily because nothing else
// owns their lifecycle.
func (s *Service) ContextID(ctx context.Context, level, instanceID string) (string, error) {
	if level == ContextLevelSystem {
		return s.SystemContextID(ctx)
	}

	parentID, err := s.parentContextID(ctx, level, instanceID)
	if err != nil {
		return "", err
	}

	var id string
	err = s.pool.QueryRow(ctx, `
		INSERT INTO contexts (level, instance_id, parent_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (level, instance_id) WHERE instance_id IS NOT NULL
		DO UPDATE SET parent_id = EXCLUDED.parent_id
		RETURNING id
	`, level, instanceID, parentID).Scan(&id)
	return id, err
}

func (s *Service) parentContextID(ctx context.Context, level, instanceID string) (string, error) {
	switch level {
	case ContextLevelCourse:
		var categoryID *string
		err := s.pool.QueryRow(ctx, `SELECT category_id FROM courses WHERE id = $1`, instanceID).Scan(&categoryID)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrContextNotFound
		}
		if err != nil {
			return "", err
		}
		if categoryID == nil {
			return s.SystemContextID(ctx)
		}
		return s.ContextID(ctx, ContextLevelCategory, *categoryID)

	case ContextLevelCategory:
		var parentCategoryID *string
		err := s.pool.QueryRow(ctx, `SELECT parent_id FROM course_categories WHERE id = $1`, instanceID).Scan(&parentCategoryID)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrContextNotFound
		}
		if err != nil {
			return "", err
		}
		if parentCategoryID == nil {
			return s.SystemContextID(ctx)
		}
		return s.ContextID(ctx, ContextLevelCategory, *parentCategoryID)

	case ContextLevelModule:
		var courseID string
		err := s.pool.QueryRow(ctx, `
			SELECT cs.course_id FROM course_modules cm
			JOIN course_sections cs ON cs.id = cm.section_id
			WHERE cm.id = $1
		`, instanceID).Scan(&courseID)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrContextNotFound
		}
		if err != nil {
			return "", err
		}
		return s.ContextID(ctx, ContextLevelCourse, courseID)

	default:
		return s.SystemContextID(ctx)
	}
}

// contextPath returns the ids from the site context down to contextID.
func (s *Service) contextPath(ctx context.Context, contextID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		WITH RECURSIVE chain AS (
			SELECT id, parent_id, 0 AS depth FROM contexts WHERE id = $1
			UNION ALL
			SELECT c.id, c.parent_id, chain.depth + 1
			FROM contexts c JOIN chain ON c.id = chain.parent_id
		)
		SELECT id FROM chain ORDER BY depth DESC
	`, contextID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var path []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		path = append(path, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	systemID, err := s.SystemContextID(ctx)
	if err != nil {
		return nil, err
	}
	if len(path) == 0 || path[0] != systemID {
		path = append([]string{systemID}, path...)
	}
	return path, nil
}

func (s *Service) isSiteAdmin(ctx context.Context, userID string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM role_assignments ra
			JOIN roles r ON r.id = ra.role_id
			JOIN contexts c ON c.id = ra.context_id
			WHERE ra.user_id = $1 AND r.name = 'admin' AND c.level = 'system'
		)
	`, userID).Scan(&ok)
	return ok, err
}

// Can reports whether userID holds capability at contextID.
func (s *Service) Can(ctx context.Context, userID, capability, contextID string) (bool, error) {
	granted, err := s.resolve(ctx, userID, capability, contextID)
	if err != nil {
		return false, err
	}
	return granted[capability], nil
}

// Capabilities returns every capability userID holds at contextID, which
// lets a client show or hide controls from one request instead of probing
// each capability separately.
func (s *Service) Capabilities(ctx context.Context, userID, contextID string) ([]string, error) {
	granted, err := s.resolve(ctx, userID, "", contextID)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for name, ok := range granted {
		if ok {
			out = append(out, name)
		}
	}
	return out, nil
}

// resolve computes granted capabilities for one capability, or for all of
// them when capability is empty.
func (s *Service) resolve(ctx context.Context, userID, capability, contextID string) (map[string]bool, error) {
	granted := map[string]bool{}

	admin, err := s.isSiteAdmin(ctx, userID)
	if err != nil {
		return nil, err
	}
	if admin {
		rows, err := s.pool.Query(ctx, `SELECT name FROM capabilities WHERE $1 = '' OR name = $1`, capability)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return nil, err
			}
			granted[name] = true
		}
		return granted, rows.Err()
	}

	path, err := s.contextPath(ctx, contextID)
	if err != nil {
		return nil, err
	}

	// The implicit "user" role applies to every signed-in user, like
	// Moodle's authenticated-user role.
	roleRows, err := s.pool.Query(ctx, `
		SELECT DISTINCT role_id FROM role_assignments WHERE user_id = $1 AND context_id = ANY($2::uuid[])
		UNION
		SELECT id FROM roles WHERE name = 'user'
	`, userID, path)
	if err != nil {
		return nil, err
	}
	var roleIDs []string
	for roleRows.Next() {
		var id string
		if err := roleRows.Scan(&id); err != nil {
			roleRows.Close()
			return nil, err
		}
		roleIDs = append(roleIDs, id)
	}
	roleRows.Close()
	if err := roleRows.Err(); err != nil {
		return nil, err
	}

	// base[capability][role] is the role's default permission.
	base := map[string]map[string]string{}
	baseRows, err := s.pool.Query(ctx, `
		SELECT c.name, rc.role_id, rc.permission
		FROM role_capabilities rc JOIN capabilities c ON c.id = rc.capability_id
		WHERE ($1 = '' OR c.name = $1) AND rc.role_id = ANY($2::uuid[])
	`, capability, roleIDs)
	if err != nil {
		return nil, err
	}
	for baseRows.Next() {
		var name, roleID, permission string
		if err := baseRows.Scan(&name, &roleID, &permission); err != nil {
			baseRows.Close()
			return nil, err
		}
		if base[name] == nil {
			base[name] = map[string]string{}
		}
		base[name][roleID] = permission
	}
	baseRows.Close()
	if err := baseRows.Err(); err != nil {
		return nil, err
	}

	// overrides[capability][role][context] is a permission override.
	overrides := map[string]map[string]map[string]string{}
	overrideRows, err := s.pool.Query(ctx, `
		SELECT c.name, ro.role_id, ro.context_id, ro.permission
		FROM role_overrides ro JOIN capabilities c ON c.id = ro.capability_id
		WHERE ($1 = '' OR c.name = $1) AND ro.role_id = ANY($2::uuid[]) AND ro.context_id = ANY($3::uuid[])
	`, capability, roleIDs, path)
	if err != nil {
		return nil, err
	}
	for overrideRows.Next() {
		var name, roleID, contextID, permission string
		if err := overrideRows.Scan(&name, &roleID, &contextID, &permission); err != nil {
			overrideRows.Close()
			return nil, err
		}
		if overrides[name] == nil {
			overrides[name] = map[string]map[string]string{}
		}
		if overrides[name][roleID] == nil {
			overrides[name][roleID] = map[string]string{}
		}
		overrides[name][roleID][contextID] = permission
	}
	overrideRows.Close()
	if err := overrideRows.Err(); err != nil {
		return nil, err
	}

	names := map[string]bool{}
	for name := range base {
		names[name] = true
	}
	for name := range overrides {
		names[name] = true
	}

	for name := range names {
		prohibited := false
		allowed := false
		for _, roleID := range roleIDs {
			permission := base[name][roleID]
			for _, ctxID := range path {
				override, ok := overrides[name][roleID][ctxID]
				if ok && permission != PermissionProhibit {
					permission = override
				}
			}
			switch permission {
			case PermissionProhibit:
				prohibited = true
			case PermissionAllow:
				allowed = true
			}
		}
		granted[name] = allowed && !prohibited
	}
	return granted, nil
}
