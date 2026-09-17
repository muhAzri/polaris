package course

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) Create(ctx context.Context, ownerID, title, description string) (*Course, error) {
	var c Course
	err := s.pool.QueryRow(ctx, `
		INSERT INTO courses (owner_id, title, description)
		VALUES ($1, $2, $3)
		RETURNING id, owner_id, title, description, created_at
	`, ownerID, title, description).Scan(&c.ID, &c.OwnerID, &c.Title, &c.Description, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Service) List(ctx context.Context) ([]Course, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, owner_id, title, description, created_at
		FROM courses ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	courses := []Course{}
	for rows.Next() {
		var c Course
		if err := rows.Scan(&c.ID, &c.OwnerID, &c.Title, &c.Description, &c.CreatedAt); err != nil {
			return nil, err
		}
		courses = append(courses, c)
	}
	return courses, rows.Err()
}

func (s *Service) Enroll(ctx context.Context, courseID, userID string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO enrollments (course_id, user_id)
		VALUES ($1, $2)
		ON CONFLICT (course_id, user_id) DO NOTHING
	`, courseID, userID)
	return err
}
