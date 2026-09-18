// Package coursemodule wraps course_modules, the polymorphic table every
// activity/resource type attaches itself to. Module-specific packages
// (content, and later assign/quiz/forum/...) create their own instance row
// first, then call Attach here to place it in a section; List/Get resolve
// a module back to its (module_type, instance_id) pair so callers can look
// up the type-specific row.
package coursemodule

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Module struct {
	ID         string    `json:"id"`
	SectionID  string    `json:"section_id"`
	ModuleType string    `json:"module_type"`
	InstanceID string    `json:"instance_id"`
	Position   int       `json:"position"`
	Visible    bool      `json:"visible"`
	CreatedAt  time.Time `json:"created_at"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// Attach appends a new module to the end of a section, pointing at an
// already-created (moduleType, instanceID) row owned by another package.
func (s *Service) Attach(ctx context.Context, sectionID, moduleType, instanceID string) (*Module, error) {
	var m Module
	err := s.pool.QueryRow(ctx, `
		INSERT INTO course_modules (section_id, module_type, instance_id, position)
		VALUES ($1, $2, $3, (SELECT COALESCE(MAX(position) + 1, 0) FROM course_modules WHERE section_id = $1))
		RETURNING id, section_id, module_type, instance_id, position, visible, created_at
	`, sectionID, moduleType, instanceID).Scan(&m.ID, &m.SectionID, &m.ModuleType, &m.InstanceID, &m.Position, &m.Visible, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Service) ListBySection(ctx context.Context, sectionID string) ([]Module, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, section_id, module_type, instance_id, position, visible, created_at
		FROM course_modules WHERE section_id = $1 ORDER BY position ASC
	`, sectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	modules := []Module{}
	for rows.Next() {
		var m Module
		if err := rows.Scan(&m.ID, &m.SectionID, &m.ModuleType, &m.InstanceID, &m.Position, &m.Visible, &m.CreatedAt); err != nil {
			return nil, err
		}
		modules = append(modules, m)
	}
	return modules, rows.Err()
}

func (s *Service) Get(ctx context.Context, id string) (*Module, error) {
	var m Module
	err := s.pool.QueryRow(ctx, `
		SELECT id, section_id, module_type, instance_id, position, visible, created_at
		FROM course_modules WHERE id = $1
	`, id).Scan(&m.ID, &m.SectionID, &m.ModuleType, &m.InstanceID, &m.Position, &m.Visible, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}
