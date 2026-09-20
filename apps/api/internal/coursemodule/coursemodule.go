// Package coursemodule wraps course_modules, the polymorphic table every
// activity/resource type attaches itself to. Module-specific packages
// (content, and later assign/quiz/forum/...) create their own instance row
// first, then call Attach here to place it in a section; List/Get resolve
// a module back to its (module_type, instance_id) pair so callers can look
// up the type-specific row. Each package also registers a Deleter so that
// removing a module — or the section or course holding it — cleans up its
// instance row too.
package coursemodule

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrModuleNotFound   = errors.New("module not found")
	ErrInvalidGroupMode = errors.New("group_mode must be 'none', 'separate' or 'visible'")
	ErrCrossCourseMove  = errors.New("a module can only be moved to a section of the same course")
	ErrInvalidGrouping  = errors.New("that grouping does not belong to this course")
	ErrSectionNotFound  = errors.New("section not found")
	ErrInvalidWindow    = errors.New("available_until must be after available_from")
)

type Module struct {
	ID             string     `json:"id"`
	SectionID      string     `json:"section_id"`
	ModuleType     string     `json:"module_type"`
	InstanceID     string     `json:"instance_id"`
	Position       int        `json:"position"`
	Visible        bool       `json:"visible"`
	Intro          string     `json:"intro"`
	GroupMode      string     `json:"group_mode"`
	GroupingID     *string    `json:"grouping_id"`
	AvailableFrom  *time.Time `json:"available_from"`
	AvailableUntil *time.Time `json:"available_until"`
	CreatedAt      time.Time  `json:"created_at"`
}

// Update lists the generic settings every module shares. Nil fields are
// left alone; the Clear flags reset an optional field to NULL.
type Update struct {
	Visible        *bool      `json:"visible"`
	Intro          *string    `json:"intro"`
	GroupMode      *string    `json:"group_mode"`
	GroupingID     *string    `json:"grouping_id"`
	ClearGrouping  bool       `json:"clear_grouping"`
	AvailableFrom  *time.Time `json:"available_from"`
	AvailableUntil *time.Time `json:"available_until"`
	ClearWindow    bool       `json:"clear_availability"`
	SectionID      *string    `json:"section_id"`
	Position       *int       `json:"position"`
}

// Deleter removes the type-specific instance row a module points at.
type Deleter func(ctx context.Context, instanceID string) error

type Service struct {
	pool     *pgxpool.Pool
	mu       sync.RWMutex
	deleters map[string]Deleter
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, deleters: map[string]Deleter{}}
}

// RegisterDeleter tells the service how to remove instances of a module type.
func (s *Service) RegisterDeleter(moduleType string, d Deleter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleters[moduleType] = d
}

const moduleColumns = `
	id, section_id, module_type, instance_id, position, visible, intro, group_mode, grouping_id,
	available_from, available_until, created_at
`

func scan(row pgx.Row) (*Module, error) {
	var m Module
	err := row.Scan(&m.ID, &m.SectionID, &m.ModuleType, &m.InstanceID, &m.Position, &m.Visible, &m.Intro,
		&m.GroupMode, &m.GroupingID, &m.AvailableFrom, &m.AvailableUntil, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// Attach appends a new module to the end of a section, pointing at an
// already-created (moduleType, instanceID) row owned by another package.
func (s *Service) Attach(ctx context.Context, sectionID, moduleType, instanceID string) (*Module, error) {
	return scan(s.pool.QueryRow(ctx, `
		INSERT INTO course_modules (section_id, module_type, instance_id, position)
		VALUES ($1, $2, $3, (SELECT COALESCE(MAX(position) + 1, 0) FROM course_modules WHERE section_id = $1))
		RETURNING `+moduleColumns, sectionID, moduleType, instanceID))
}

func (s *Service) ListBySection(ctx context.Context, sectionID string) ([]Module, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+moduleColumns+` FROM course_modules WHERE section_id = $1 ORDER BY position ASC
	`, sectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	modules := []Module{}
	for rows.Next() {
		m, err := scan(rows)
		if err != nil {
			return nil, err
		}
		modules = append(modules, *m)
	}
	return modules, rows.Err()
}

func (s *Service) Get(ctx context.Context, id string) (*Module, error) {
	m, err := scan(s.pool.QueryRow(ctx, `SELECT `+moduleColumns+` FROM course_modules WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrModuleNotFound
	}
	return m, err
}

// Update changes generic module settings and, when SectionID or Position
// is set, moves the module within or between sections of its course.
func (s *Service) Update(ctx context.Context, id string, u Update) (*Module, error) {
	if u.GroupMode != nil && *u.GroupMode != "none" && *u.GroupMode != "separate" && *u.GroupMode != "visible" {
		return nil, ErrInvalidGroupMode
	}
	if u.AvailableFrom != nil && u.AvailableUntil != nil && !u.AvailableUntil.After(*u.AvailableFrom) {
		return nil, ErrInvalidWindow
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var courseID, sectionID string
	var position int
	err = tx.QueryRow(ctx, `
		SELECT cs.course_id, cm.section_id, cm.position FROM course_modules cm
		JOIN course_sections cs ON cs.id = cm.section_id
		WHERE cm.id = $1 FOR UPDATE OF cm
	`, id).Scan(&courseID, &sectionID, &position)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrModuleNotFound
	}
	if err != nil {
		return nil, err
	}

	if u.GroupingID != nil {
		var ok bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM groupings WHERE id = $1 AND course_id = $2)`, *u.GroupingID, courseID).Scan(&ok); err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrInvalidGrouping
		}
	}

	if u.SectionID != nil || u.Position != nil {
		targetSection := sectionID
		if u.SectionID != nil {
			targetSection = *u.SectionID
		}
		var targetCourse string
		err := tx.QueryRow(ctx, `SELECT course_id FROM course_sections WHERE id = $1`, targetSection).Scan(&targetCourse)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSectionNotFound
		}
		if err != nil {
			return nil, err
		}
		if targetCourse != courseID {
			return nil, ErrCrossCourseMove
		}

		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM course_modules WHERE section_id = $1`, targetSection).Scan(&count); err != nil {
			return nil, err
		}
		newPos := count
		if targetSection == sectionID {
			newPos = count - 1
		}
		if u.Position != nil && *u.Position >= 0 && *u.Position < newPos+1 {
			newPos = *u.Position
		}

		// Take the module out of its slot, then open a gap at the target.
		if _, err := tx.Exec(ctx, `UPDATE course_modules SET position = position - 1 WHERE section_id = $1 AND position > $2`, sectionID, position); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE course_modules SET position = position + 1 WHERE section_id = $1 AND position >= $2 AND id <> $3`, targetSection, newPos, id); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE course_modules SET section_id = $2, position = $3 WHERE id = $1`, id, targetSection, newPos); err != nil {
			return nil, err
		}
	}

	m, err := scan(tx.QueryRow(ctx, `
		UPDATE course_modules SET
			visible = COALESCE($2, visible),
			intro = COALESCE($3, intro),
			group_mode = COALESCE($4, group_mode),
			grouping_id = CASE WHEN $6 THEN NULL ELSE COALESCE($5::uuid, grouping_id) END,
			available_from = CASE WHEN $9 THEN NULL ELSE COALESCE($7, available_from) END,
			available_until = CASE WHEN $9 THEN NULL ELSE COALESCE($8, available_until) END,
			updated_at = now()
		WHERE id = $1
		RETURNING `+moduleColumns,
		id, u.Visible, u.Intro, u.GroupMode, u.GroupingID, u.ClearGrouping, u.AvailableFrom, u.AvailableUntil, u.ClearWindow))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return m, nil
}

// Delete removes a module, its type-specific instance row, and anything
// hanging off it by foreign key (grade items, calendar events), then
// closes the gap in the section's numbering.
func (s *Service) Delete(ctx context.Context, id string) error {
	m, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	s.mu.RLock()
	deleter := s.deleters[m.ModuleType]
	s.mu.RUnlock()
	if deleter != nil {
		if err := deleter(ctx, m.InstanceID); err != nil {
			return err
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM course_modules WHERE id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE course_modules SET position = position - 1 WHERE section_id = $1 AND position > $2`, m.SectionID, m.Position); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
