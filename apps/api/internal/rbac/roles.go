package rbac

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrRoleNotFound       = errors.New("role not found")
	ErrCapabilityNotFound = errors.New("capability not found")
	ErrSystemRole         = errors.New("built-in roles cannot be deleted")
	ErrRoleNameTaken      = errors.New("a role with that name already exists")
	ErrInvalidPermission  = errors.New("permission must be allow, prevent, prohibit or notset")
	ErrRoleNotAssignable  = errors.New("that role cannot be assigned at this level")
)

type Role struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	Archetype   string `json:"archetype"`
	IsSystem    bool   `json:"is_system"`
}

type Capability struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type RoleDetail struct {
	Role
	Permissions map[string]string `json:"permissions"`
}

type Assignment struct {
	UserID    string    `json:"user_id"`
	UserName  string    `json:"user_name"`
	UserEmail string    `json:"user_email"`
	Role      string    `json:"role"`
	RoleName  string    `json:"role_display_name"`
	CreatedAt time.Time `json:"created_at"`
}

type Override struct {
	Role       string `json:"role"`
	Capability string `json:"capability"`
	Permission string `json:"permission"`
}

// siteOnlyRoles are only meaningful at the site level: admin is the
// site-wide super role, user is implicit, and coursecreator is a
// site-level "may create courses" flag.
var siteOnlyRoles = map[string]bool{"admin": true, "user": true, "coursecreator": true}

func (s *Service) ListRoles(ctx context.Context) ([]Role, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, display_name, description, archetype, is_system FROM roles ORDER BY is_system DESC, name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	roles := []Role{}
	for rows.Next() {
		var r Role
		if err := rows.Scan(&r.ID, &r.Name, &r.DisplayName, &r.Description, &r.Archetype, &r.IsSystem); err != nil {
			return nil, err
		}
		roles = append(roles, r)
	}
	return roles, rows.Err()
}

func (s *Service) RoleByName(ctx context.Context, name string) (*Role, error) {
	var r Role
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, display_name, description, archetype, is_system FROM roles WHERE name = $1
	`, name).Scan(&r.ID, &r.Name, &r.DisplayName, &r.Description, &r.Archetype, &r.IsSystem)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRoleNotFound
	}
	return &r, err
}

func (s *Service) ListCapabilities(ctx context.Context) ([]Capability, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, description FROM capabilities ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Capability{}
	for rows.Next() {
		var c Capability
		if err := rows.Scan(&c.ID, &c.Name, &c.Description); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetRole returns a role together with its default permission for every
// capability it has one for.
func (s *Service) GetRole(ctx context.Context, roleID string) (*RoleDetail, error) {
	var d RoleDetail
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, display_name, description, archetype, is_system FROM roles WHERE id = $1
	`, roleID).Scan(&d.ID, &d.Name, &d.DisplayName, &d.Description, &d.Archetype, &d.IsSystem)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRoleNotFound
	}
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT c.name, rc.permission FROM role_capabilities rc
		JOIN capabilities c ON c.id = rc.capability_id WHERE rc.role_id = $1
	`, roleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	d.Permissions = map[string]string{}
	for rows.Next() {
		var name, permission string
		if err := rows.Scan(&name, &permission); err != nil {
			return nil, err
		}
		d.Permissions[name] = permission
	}
	return &d, rows.Err()
}

// CreateRole defines a custom role, optionally starting from a copy of an
// existing role's permissions.
func (s *Service) CreateRole(ctx context.Context, name, displayName, description, cloneFromRoleID string) (*Role, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if displayName == "" {
		displayName = name
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var r Role
	err = tx.QueryRow(ctx, `
		INSERT INTO roles (name, display_name, description) VALUES ($1, $2, $3)
		RETURNING id, name, display_name, description, archetype, is_system
	`, name, displayName, description).Scan(&r.ID, &r.Name, &r.DisplayName, &r.Description, &r.Archetype, &r.IsSystem)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrRoleNameTaken
		}
		return nil, err
	}

	if cloneFromRoleID != "" {
		if _, err := tx.Exec(ctx, `
			INSERT INTO role_capabilities (role_id, capability_id, permission)
			SELECT $1, capability_id, permission FROM role_capabilities WHERE role_id = $2
		`, r.ID, cloneFromRoleID); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Service) DeleteRole(ctx context.Context, roleID string) error {
	var isSystem bool
	err := s.pool.QueryRow(ctx, `SELECT is_system FROM roles WHERE id = $1`, roleID).Scan(&isSystem)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRoleNotFound
	}
	if err != nil {
		return err
	}
	if isSystem {
		return ErrSystemRole
	}
	_, err = s.pool.Exec(ctx, `DELETE FROM roles WHERE id = $1`, roleID)
	return err
}

// SetRoleCapability sets a role's default permission for a capability;
// "notset" removes it.
func (s *Service) SetRoleCapability(ctx context.Context, roleID, capability, permission string) error {
	capabilityID, err := s.capabilityID(ctx, capability)
	if err != nil {
		return err
	}
	if permission == "notset" {
		_, err := s.pool.Exec(ctx, `DELETE FROM role_capabilities WHERE role_id = $1 AND capability_id = $2`, roleID, capabilityID)
		return err
	}
	if !validPermission(permission) {
		return ErrInvalidPermission
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO role_capabilities (role_id, capability_id, permission) VALUES ($1, $2, $3)
		ON CONFLICT (role_id, capability_id) DO UPDATE SET permission = EXCLUDED.permission
	`, roleID, capabilityID, permission)
	return err
}

func (s *Service) capabilityID(ctx context.Context, name string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `SELECT id FROM capabilities WHERE name = $1`, name).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrCapabilityNotFound
	}
	return id, err
}

func validPermission(p string) bool {
	return p == PermissionAllow || p == PermissionPrevent || p == PermissionProhibit
}

// AssignRole grants a role to a user at a context. Site-only roles cannot
// be assigned below the site.
func (s *Service) AssignRole(ctx context.Context, userID, roleName, contextID string) error {
	role, err := s.RoleByName(ctx, roleName)
	if err != nil {
		return err
	}

	systemID, err := s.SystemContextID(ctx)
	if err != nil {
		return err
	}
	if siteOnlyRoles[role.Name] && contextID != systemID {
		return ErrRoleNotAssignable
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO role_assignments (role_id, user_id, context_id) VALUES ($1, $2, $3)
		ON CONFLICT (role_id, user_id, context_id) DO NOTHING
	`, role.ID, userID, contextID)
	return err
}

func (s *Service) UnassignRole(ctx context.Context, userID, roleName, contextID string) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM role_assignments
		WHERE user_id = $1 AND context_id = $3 AND role_id = (SELECT id FROM roles WHERE name = $2)
	`, userID, roleName, contextID)
	return err
}

// UnassignAll removes every role a user holds at exactly this context, used
// when a user leaves a course.
func (s *Service) UnassignAll(ctx context.Context, userID, contextID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM role_assignments WHERE user_id = $1 AND context_id = $2`, userID, contextID)
	return err
}

func (s *Service) ListAssignments(ctx context.Context, contextID string) ([]Assignment, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.name, u.email, r.name, r.display_name, ra.created_at
		FROM role_assignments ra
		JOIN users u ON u.id = ra.user_id
		JOIN roles r ON r.id = ra.role_id
		WHERE ra.context_id = $1
		ORDER BY u.name ASC, r.name ASC
	`, contextID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Assignment{}
	for rows.Next() {
		var a Assignment
		if err := rows.Scan(&a.UserID, &a.UserName, &a.UserEmail, &a.Role, &a.RoleName, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SetOverride sets (or, for "notset", clears) a role's permission override
// at a context.
func (s *Service) SetOverride(ctx context.Context, roleName, capability, permission, contextID string) error {
	role, err := s.RoleByName(ctx, roleName)
	if err != nil {
		return err
	}
	capabilityID, err := s.capabilityID(ctx, capability)
	if err != nil {
		return err
	}

	if permission == "notset" {
		_, err := s.pool.Exec(ctx, `
			DELETE FROM role_overrides WHERE role_id = $1 AND capability_id = $2 AND context_id = $3
		`, role.ID, capabilityID, contextID)
		return err
	}
	if !validPermission(permission) {
		return ErrInvalidPermission
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO role_overrides (role_id, capability_id, context_id, permission) VALUES ($1, $2, $3, $4)
		ON CONFLICT (role_id, capability_id, context_id) DO UPDATE SET permission = EXCLUDED.permission
	`, role.ID, capabilityID, contextID, permission)
	return err
}

func (s *Service) ListOverrides(ctx context.Context, contextID string) ([]Override, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.name, c.name, ro.permission
		FROM role_overrides ro
		JOIN roles r ON r.id = ro.role_id
		JOIN capabilities c ON c.id = ro.capability_id
		WHERE ro.context_id = $1
		ORDER BY r.name, c.name
	`, contextID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Override{}
	for rows.Next() {
		var o Override
		if err := rows.Scan(&o.Role, &o.Capability, &o.Permission); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == "23505"
	}
	return false
}
