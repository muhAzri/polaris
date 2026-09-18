// Package groups implements course-scoped roster groups, used for "group
// mode" by later modules (assign, forum, ...). Groupings (bundles of
// groups) have their schema created alongside this but no API yet —
// nothing consumes them until a module that supports group mode exists.
package groups

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

func (s *Service) CourseIDForGroup(ctx context.Context, groupID string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `SELECT course_id FROM groups WHERE id = $1`, groupID).Scan(&id)
	return id, err
}

func (s *Service) Create(ctx context.Context, courseID, userID, name, description string) (*Group, error) {
	var g Group
	err := s.pool.QueryRow(ctx, `
		INSERT INTO groups (course_id, name, description) VALUES ($1, $2, $3)
		RETURNING id, course_id, name, description, created_at
	`, courseID, name, description).Scan(&g.ID, &g.CourseID, &g.Name, &g.Description, &g.CreatedAt)
	if err != nil {
		return nil, err
	}
	g.MemberIDs = []string{}

	s.events.Dispatch(ctx, eventbus.Event{
		Name:   "group.created",
		UserID: &userID,
		Data:   map[string]any{"group_id": g.ID, "course_id": courseID},
	})
	return &g, nil
}

// List returns every group in a course with its member ids inlined — a
// course's group roster is small enough that this beats making the client
// fan out to a separate members endpoint per group.
func (s *Service) List(ctx context.Context, courseID string) ([]Group, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, course_id, name, description, created_at
		FROM groups WHERE course_id = $1 ORDER BY created_at ASC
	`, courseID)
	if err != nil {
		return nil, err
	}

	list := []Group{}
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.CourseID, &g.Name, &g.Description, &g.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		g.MemberIDs = []string{}
		list = append(list, g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range list {
		memberRows, err := s.pool.Query(ctx, `SELECT user_id FROM group_members WHERE group_id = $1`, list[i].ID)
		if err != nil {
			return nil, err
		}
		for memberRows.Next() {
			var userID string
			if err := memberRows.Scan(&userID); err != nil {
				memberRows.Close()
				return nil, err
			}
			list[i].MemberIDs = append(list[i].MemberIDs, userID)
		}
		memberRows.Close()
		if err := memberRows.Err(); err != nil {
			return nil, err
		}
	}
	return list, nil
}

func (s *Service) AddMember(ctx context.Context, groupID, addedByUserID, memberUserID string) error {
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO group_members (group_id, user_id) VALUES ($1, $2)
		ON CONFLICT (group_id, user_id) DO NOTHING
	`, groupID, memberUserID); err != nil {
		return err
	}

	s.events.Dispatch(ctx, eventbus.Event{
		Name:   "group.member_added",
		UserID: &addedByUserID,
		Data:   map[string]any{"group_id": groupID, "member_id": memberUserID},
	})
	return nil
}
