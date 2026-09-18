// Package gradebook implements grade storage: categories, items, and
// per-user grades. Activity modules (assignment/quiz/forum) should write
// grades here instead of keeping their own grade columns.
package gradebook

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"polaris-api/internal/eventbus"
)

type Service struct {
	pool   *pgxpool.Pool
	events *eventbus.Dispatcher
}

func NewService(pool *pgxpool.Pool, events *eventbus.Dispatcher) *Service {
	return &Service{pool: pool, events: events}
}

func (s *Service) CourseIDForItem(ctx context.Context, itemID string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `SELECT course_id FROM grade_items WHERE id = $1`, itemID).Scan(&id)
	return id, err
}

func (s *Service) CreateCategory(ctx context.Context, courseID, name, aggregation string) (*Category, error) {
	if aggregation == "" {
		aggregation = "mean"
	}
	var c Category
	err := s.pool.QueryRow(ctx, `
		INSERT INTO grade_categories (course_id, name, aggregation)
		VALUES ($1, $2, $3)
		RETURNING id, course_id, name, aggregation, created_at
	`, courseID, name, aggregation).Scan(&c.ID, &c.CourseID, &c.Name, &c.Aggregation, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Service) ListCategories(ctx context.Context, courseID string) ([]Category, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, course_id, name, aggregation, created_at
		FROM grade_categories WHERE course_id = $1 ORDER BY created_at ASC
	`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	categories := []Category{}
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.CourseID, &c.Name, &c.Aggregation, &c.CreatedAt); err != nil {
			return nil, err
		}
		categories = append(categories, c)
	}
	return categories, rows.Err()
}

// CreateItem attaches a new grade item to a course. categoryID may be empty,
// in which case the item lands in the course's default "Uncategorized"
// category created alongside the course (course.Service.Create).
// courseModuleID is nil for a manually-added item, or set when a later
// module (assign, quiz, ...) creates the item for one of its instances.
func (s *Service) CreateItem(ctx context.Context, courseID, categoryID string, courseModuleID *string, name string, maxGrade float64, weight *float64) (*Item, error) {
	if categoryID == "" {
		if err := s.pool.QueryRow(ctx, `
			SELECT id FROM grade_categories WHERE course_id = $1 AND name = 'Uncategorized'
		`, courseID).Scan(&categoryID); err != nil {
			return nil, err
		}
	}

	var it Item
	err := s.pool.QueryRow(ctx, `
		INSERT INTO grade_items (course_id, category_id, course_module_id, name, max_grade, weight)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, course_id, category_id, course_module_id, name, max_grade, weight, created_at
	`, courseID, categoryID, courseModuleID, name, maxGrade, weight).Scan(
		&it.ID, &it.CourseID, &it.CategoryID, &it.CourseModuleID, &it.Name, &it.MaxGrade, &it.Weight, &it.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &it, nil
}

func (s *Service) ListItems(ctx context.Context, courseID string) ([]Item, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, course_id, category_id, course_module_id, name, max_grade, weight, created_at
		FROM grade_items WHERE course_id = $1 ORDER BY created_at ASC
	`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Item{}
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.CourseID, &it.CategoryID, &it.CourseModuleID, &it.Name, &it.MaxGrade, &it.Weight, &it.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// SetGrade upserts a user's grade for an item — teachers re-grade the same
// item/user pair often (feedback revisions, regrades), so this is a write
// path that must be idempotent per (item, user) rather than append-only.
func (s *Service) SetGrade(ctx context.Context, itemID, userID, graderID string, grade *float64, feedback string) (*Grade, error) {
	var g Grade
	err := s.pool.QueryRow(ctx, `
		INSERT INTO grade_grades (grade_item_id, user_id, grade, feedback, graded_by, updated_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (grade_item_id, user_id) DO UPDATE
		SET grade = EXCLUDED.grade, feedback = EXCLUDED.feedback, graded_by = EXCLUDED.graded_by, updated_at = now()
		RETURNING id, grade_item_id, user_id, grade, feedback, graded_by, created_at, updated_at
	`, itemID, userID, grade, feedback, graderID).Scan(
		&g.ID, &g.GradeItemID, &g.UserID, &g.Grade, &g.Feedback, &g.GradedBy, &g.CreatedAt, &g.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	s.events.Dispatch(ctx, eventbus.Event{
		Name:   "grade.updated",
		UserID: &graderID,
		Data:   map[string]any{"grade_item_id": itemID, "user_id": userID},
	})
	return &g, nil
}

func (s *Service) ListGrades(ctx context.Context, itemID string) ([]Grade, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, grade_item_id, user_id, grade, feedback, graded_by, created_at, updated_at
		FROM grade_grades WHERE grade_item_id = $1 ORDER BY created_at ASC
	`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	grades := []Grade{}
	for rows.Next() {
		var g Grade
		if err := rows.Scan(&g.ID, &g.GradeItemID, &g.UserID, &g.Grade, &g.Feedback, &g.GradedBy, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		grades = append(grades, g)
	}
	return grades, rows.Err()
}

// ListMyGrades returns every grade item in the course paired with the
// given user's grade for it, including items they haven't been graded on
// yet (LEFT JOIN) — a student's gradebook page needs to show all gradable
// items, not just the ones already graded.
func (s *Service) ListMyGrades(ctx context.Context, courseID, userID string) ([]ItemGrade, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT gi.id, gi.course_id, gi.category_id, gi.course_module_id, gi.name, gi.max_grade, gi.weight, gi.created_at,
		       gg.grade, gg.feedback
		FROM grade_items gi
		LEFT JOIN grade_grades gg ON gg.grade_item_id = gi.id AND gg.user_id = $2
		WHERE gi.course_id = $1
		ORDER BY gi.created_at ASC
	`, courseID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := []ItemGrade{}
	for rows.Next() {
		var ig ItemGrade
		var feedback *string
		if err := rows.Scan(
			&ig.Item.ID, &ig.Item.CourseID, &ig.Item.CategoryID, &ig.Item.CourseModuleID, &ig.Item.Name, &ig.Item.MaxGrade, &ig.Item.Weight, &ig.Item.CreatedAt,
			&ig.Grade, &feedback,
		); err != nil {
			return nil, err
		}
		if feedback != nil {
			ig.Feedback = *feedback
		}
		results = append(results, ig)
	}
	return results, rows.Err()
}
