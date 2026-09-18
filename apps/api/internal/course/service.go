package course

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"polaris-api/internal/eventbus"
	"polaris-api/internal/rbac"
)

type Service struct {
	pool   *pgxpool.Pool
	rbac   *rbac.Service
	events *eventbus.Dispatcher
}

func NewService(pool *pgxpool.Pool, rbacService *rbac.Service, events *eventbus.Dispatcher) *Service {
	return &Service{pool: pool, rbac: rbacService, events: events}
}

// Create inserts the course together with the default "General" section
// and manual enrolment method every course needs, then emits
// course.created so other modules (notifications, search, ...) can react
// without Create knowing about them.
func (s *Service) Create(ctx context.Context, ownerID, title, description string) (*Course, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var c Course
	err = tx.QueryRow(ctx, `
		INSERT INTO courses (owner_id, title, description, category_id)
		VALUES ($1, $2, $3, (SELECT id FROM course_categories WHERE name = 'Uncategorized'))
		RETURNING id, owner_id, category_id, title, description, created_at
	`, ownerID, title, description).Scan(&c.ID, &c.OwnerID, &c.CategoryID, &c.Title, &c.Description, &c.CreatedAt)
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO course_sections (course_id, title, position) VALUES ($1, 'General', 0)
	`, c.ID); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO enrolment_methods (course_id, type) VALUES ($1, 'manual')
	`, c.ID); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	courseContextID, err := s.rbac.ContextID(ctx, rbac.ContextLevelCourse, c.ID)
	if err != nil {
		return nil, err
	}

	s.events.Dispatch(ctx, eventbus.Event{
		Name:      "course.created",
		ContextID: &courseContextID,
		UserID:    &ownerID,
		Data:      map[string]any{"course_id": c.ID, "title": c.Title},
	})

	return &c, nil
}

func (s *Service) List(ctx context.Context) ([]Course, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, owner_id, category_id, title, description, created_at
		FROM courses ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	courses := []Course{}
	for rows.Next() {
		var c Course
		if err := rows.Scan(&c.ID, &c.OwnerID, &c.CategoryID, &c.Title, &c.Description, &c.CreatedAt); err != nil {
			return nil, err
		}
		courses = append(courses, c)
	}
	return courses, rows.Err()
}

func (s *Service) ListSections(ctx context.Context, courseID string) ([]Section, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, course_id, title, position, created_at
		FROM course_sections WHERE course_id = $1 ORDER BY position ASC
	`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sections := []Section{}
	for rows.Next() {
		var sec Section
		if err := rows.Scan(&sec.ID, &sec.CourseID, &sec.Title, &sec.Position, &sec.CreatedAt); err != nil {
			return nil, err
		}
		sections = append(sections, sec)
	}
	return sections, rows.Err()
}

// Enroll uses the course's manual enrolment method rather than writing to
// enrollments directly, so self/cohort/guest methods can be added later
// without touching this call site.
func (s *Service) Enroll(ctx context.Context, courseID, userID string) error {
	var methodID string
	err := s.pool.QueryRow(ctx, `
		SELECT id FROM enrolment_methods WHERE course_id = $1 AND type = 'manual'
	`, courseID).Scan(&methodID)
	if err != nil {
		return err
	}

	if _, err := s.pool.Exec(ctx, `
		INSERT INTO enrollments (course_id, user_id, enrolment_method_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (course_id, user_id) DO NOTHING
	`, courseID, userID, methodID); err != nil {
		return err
	}

	courseContextID, err := s.rbac.ContextID(ctx, rbac.ContextLevelCourse, courseID)
	if err != nil {
		return err
	}

	s.events.Dispatch(ctx, eventbus.Event{
		Name:      "course.enrolled",
		ContextID: &courseContextID,
		UserID:    &userID,
		Data:      map[string]any{"course_id": courseID},
	})

	return nil
}
