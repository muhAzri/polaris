// Package rbac implements Polaris's role/capability/context permission
// model (ROADMAP.md section 2.1): a role is a bundle of capabilities, a
// context is where a check applies (system/category/course/module/user),
// and a role assignment grants a role to a user at a context. A capability
// granted at the system context is treated as global and satisfies checks
// at any more specific context, mirroring Moodle's context inheritance
// without needing the full category-path hierarchy yet.
package rbac

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	ContextLevelSystem   = "system"
	ContextLevelCategory = "category"
	ContextLevelCourse   = "course"
	ContextLevelModule   = "module"
	ContextLevelUser     = "user"
)

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
// first use. Non-system contexts are created lazily this way because
// nothing else in Phase 0 owns their lifecycle yet (e.g. a course's
// context is created the first time a capability check needs it).
func (s *Service) ContextID(ctx context.Context, level, instanceID string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO contexts (level, instance_id)
		VALUES ($1, $2)
		ON CONFLICT (level, instance_id) WHERE instance_id IS NOT NULL
		DO UPDATE SET level = EXCLUDED.level
		RETURNING id
	`, level, instanceID).Scan(&id)
	return id, err
}

// Can reports whether userID holds capability at contextID, either via a
// role assigned directly at that context or via a role assigned at the
// system context (which applies everywhere).
func (s *Service) Can(ctx context.Context, userID, capability, contextID string) (bool, error) {
	var allowed bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM role_assignments ra
			JOIN role_capabilities rc ON rc.role_id = ra.role_id
			JOIN capabilities c ON c.id = rc.capability_id
			JOIN contexts ctx ON ctx.id = ra.context_id
			WHERE ra.user_id = $1
			  AND c.name = $2
			  AND (ctx.level = 'system' OR ra.context_id = $3)
		)
	`, userID, capability, contextID).Scan(&allowed)
	return allowed, err
}
