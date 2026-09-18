// Package calendar implements calendar_events: course events, personal
// user events, and site-wide events. It's deliberately just event storage
// + range queries — a monthly grid is a frontend rendering concern, not
// something this package needs to build.
package calendar

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) CreateCourseEvent(ctx context.Context, courseID, name, description string, startAt time.Time, endAt *time.Time) (*Event, error) {
	return s.insert(ctx, "course", &courseID, nil, name, description, startAt, endAt)
}

func (s *Service) CreateUserEvent(ctx context.Context, userID, name, description string, startAt time.Time, endAt *time.Time) (*Event, error) {
	return s.insert(ctx, "user", nil, &userID, name, description, startAt, endAt)
}

func (s *Service) CreateSiteEvent(ctx context.Context, name, description string, startAt time.Time, endAt *time.Time) (*Event, error) {
	return s.insert(ctx, "site", nil, nil, name, description, startAt, endAt)
}

func (s *Service) insert(ctx context.Context, eventType string, courseID, userID *string, name, description string, startAt time.Time, endAt *time.Time) (*Event, error) {
	var e Event
	err := s.pool.QueryRow(ctx, `
		INSERT INTO calendar_events (event_type, course_id, user_id, name, description, start_at, end_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, event_type, course_id, user_id, name, description, start_at, end_at, created_at
	`, eventType, courseID, userID, name, description, startAt, endAt).Scan(
		&e.ID, &e.EventType, &e.CourseID, &e.UserID, &e.Name, &e.Description, &e.StartAt, &e.EndAt, &e.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (s *Service) ListCourseEvents(ctx context.Context, courseID string, from, to time.Time) ([]Event, error) {
	return s.query(ctx, `
		SELECT id, event_type, course_id, user_id, name, description, start_at, end_at, created_at
		FROM calendar_events
		WHERE event_type = 'course' AND course_id = $1 AND start_at >= $2 AND start_at <= $3
		ORDER BY start_at ASC
	`, courseID, from, to)
}

// ListMyEvents is the "my calendar" view: the user's own personal events,
// site-wide events, and course events for every course they're enrolled
// in — combined and ordered, so a monthly view is a single call away.
func (s *Service) ListMyEvents(ctx context.Context, userID string, from, to time.Time) ([]Event, error) {
	return s.query(ctx, `
		SELECT id, event_type, course_id, user_id, name, description, start_at, end_at, created_at
		FROM calendar_events
		WHERE start_at >= $2 AND start_at <= $3
		  AND (
		    event_type = 'site'
		    OR (event_type = 'user' AND user_id = $1)
		    OR (event_type = 'course' AND course_id IN (SELECT course_id FROM enrollments WHERE user_id = $1))
		  )
		ORDER BY start_at ASC
	`, userID, from, to)
}

func (s *Service) query(ctx context.Context, sql string, args ...any) ([]Event, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.EventType, &e.CourseID, &e.UserID, &e.Name, &e.Description, &e.StartAt, &e.EndAt, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
