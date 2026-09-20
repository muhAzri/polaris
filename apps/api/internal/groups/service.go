// Package groups implements course-scoped roster groups and the groupings
// that bundle them, used for "group mode" by activity modules (assign,
// forum, ...). A module's group_mode and grouping_id live on course_modules;
// this package owns the groups themselves.
package groups

import (
	"context"
	"errors"
	"fmt"
	"math/rand"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"polaris-api/internal/eventbus"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrNameTaken       = errors.New("a group or grouping with that name already exists in this course")
	ErrNotEnrolled     = errors.New("only users enrolled in the course can be group members")
	ErrNameRequired    = errors.New("name is required")
	ErrInvalidAuto     = errors.New("give either count or size, greater than zero")
	ErrCrossCourse     = errors.New("the group and the grouping belong to different courses")
	ErrNoStudentsToUse = errors.New("the course has no students to split into groups")
)

type Service struct {
	pool   *pgxpool.Pool
	events *eventbus.Dispatcher
}

func NewService(pool *pgxpool.Pool, events *eventbus.Dispatcher) *Service {
	return &Service{pool: pool, events: events}
}

func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	return errors.As(err, &pgErr) && pgErr.SQLState() == "23505"
}

func (s *Service) CourseIDForGroup(ctx context.Context, groupID string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `SELECT course_id FROM groups WHERE id = $1`, groupID).Scan(&id)
	return id, err
}

func (s *Service) CourseIDForGrouping(ctx context.Context, groupingID string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `SELECT course_id FROM groupings WHERE id = $1`, groupingID).Scan(&id)
	return id, err
}

func (s *Service) Create(ctx context.Context, courseID, userID string, in GroupInput) (*Group, error) {
	if in.Name == "" {
		return nil, ErrNameRequired
	}
	description := ""
	if in.Description != nil {
		description = *in.Description
	}
	var g Group
	err := s.pool.QueryRow(ctx, `
		INSERT INTO groups (course_id, name, description) VALUES ($1, $2, $3)
		RETURNING id, course_id, name, description, created_at
	`, courseID, in.Name, description).Scan(&g.ID, &g.CourseID, &g.Name, &g.Description, &g.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrNameTaken
		}
		return nil, err
	}
	g.MemberIDs = []string{}
	g.Members = []Member{}

	s.events.Dispatch(ctx, eventbus.Event{
		Name:   "group.created",
		UserID: &userID,
		Data:   map[string]any{"group_id": g.ID, "course_id": courseID},
	})
	return &g, nil
}

func (s *Service) Update(ctx context.Context, groupID string, in GroupInput) (*Group, error) {
	var courseID string
	err := s.pool.QueryRow(ctx, `
		UPDATE groups SET name = COALESCE(NULLIF($2, ''), name), description = COALESCE($3, description)
		WHERE id = $1 RETURNING course_id
	`, groupID, in.Name, in.Description).Scan(&courseID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrNameTaken
		}
		return nil, err
	}
	list, err := s.List(ctx, courseID)
	if err != nil {
		return nil, err
	}
	for _, g := range list {
		if g.ID == groupID {
			return &g, nil
		}
	}
	return nil, ErrNotFound
}

func (s *Service) Delete(ctx context.Context, groupID string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM groups WHERE id = $1`, groupID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// List returns every group in a course with its members inlined — a
// course's group roster is small enough that this beats making the client
// fan out to a separate members endpoint per group.
func (s *Service) List(ctx context.Context, courseID string) ([]Group, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, course_id, name, description, created_at
		FROM groups WHERE course_id = $1 ORDER BY name ASC
	`, courseID)
	if err != nil {
		return nil, err
	}

	list := []Group{}
	index := map[string]int{}
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.CourseID, &g.Name, &g.Description, &g.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		g.MemberIDs = []string{}
		g.Members = []Member{}
		index[g.ID] = len(list)
		list = append(list, g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	memberRows, err := s.pool.Query(ctx, `
		SELECT gm.group_id, u.id, u.name, u.email
		FROM group_members gm
		JOIN groups g ON g.id = gm.group_id
		JOIN users u ON u.id = gm.user_id
		WHERE g.course_id = $1 ORDER BY u.name
	`, courseID)
	if err != nil {
		return nil, err
	}
	defer memberRows.Close()
	for memberRows.Next() {
		var groupID string
		var m Member
		if err := memberRows.Scan(&groupID, &m.UserID, &m.Name, &m.Email); err != nil {
			return nil, err
		}
		i := index[groupID]
		list[i].MemberIDs = append(list[i].MemberIDs, m.UserID)
		list[i].Members = append(list[i].Members, m)
	}
	return list, memberRows.Err()
}

func (s *Service) AddMember(ctx context.Context, groupID, addedByUserID, memberUserID string) error {
	var enrolled bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM groups g JOIN enrollments e ON e.course_id = g.course_id
			WHERE g.id = $1 AND e.user_id = $2
		)
	`, groupID, memberUserID).Scan(&enrolled)
	if err != nil {
		return err
	}
	if !enrolled {
		return ErrNotEnrolled
	}

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

func (s *Service) RemoveMember(ctx context.Context, groupID, removedByUserID, memberUserID string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM group_members WHERE group_id = $1 AND user_id = $2`, groupID, memberUserID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	s.events.Dispatch(ctx, eventbus.Event{
		Name:   "group.member_removed",
		UserID: &removedByUserID,
		Data:   map[string]any{"group_id": groupID, "member_id": memberUserID},
	})
	return nil
}

// AutoCreate splits the course's students at random into new groups named
// "<prefix> 1", "<prefix> 2", … and returns them. Students already in a
// group are still included, so run it on a course without groups yet.
func (s *Service) AutoCreate(ctx context.Context, courseID, userID string, in AutoCreateInput) ([]Group, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT e.user_id
		FROM enrollments e
		JOIN role_assignments ra ON ra.user_id = e.user_id
		JOIN roles r ON r.id = ra.role_id AND r.name = 'student'
		JOIN contexts cx ON cx.id = ra.context_id AND cx.level = 'course' AND cx.instance_id = e.course_id
		WHERE e.course_id = $1
	`, courseID)
	if err != nil {
		return nil, err
	}
	var students []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		students = append(students, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(students) == 0 {
		return nil, ErrNoStudentsToUse
	}

	count := in.Count
	if count <= 0 && in.Size > 0 {
		count = (len(students) + in.Size - 1) / in.Size
	}
	if count <= 0 {
		return nil, ErrInvalidAuto
	}
	if count > len(students) {
		count = len(students)
	}
	prefix := in.Prefix
	if prefix == "" {
		prefix = "Group"
	}

	rand.Shuffle(len(students), func(i, j int) { students[i], students[j] = students[j], students[i] })

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	groupIDs := make([]string, count)
	for i := 0; i < count; i++ {
		err := tx.QueryRow(ctx, `INSERT INTO groups (course_id, name) VALUES ($1, $2) RETURNING id`,
			courseID, fmt.Sprintf("%s %d", prefix, i+1)).Scan(&groupIDs[i])
		if err != nil {
			if isUniqueViolation(err) {
				return nil, ErrNameTaken
			}
			return nil, err
		}
	}
	for i, student := range students {
		if _, err := tx.Exec(ctx, `INSERT INTO group_members (group_id, user_id) VALUES ($1, $2)`, groupIDs[i%count], student); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	s.events.Dispatch(ctx, eventbus.Event{
		Name:   "group.created",
		UserID: &userID,
		Data:   map[string]any{"course_id": courseID, "auto": true, "count": count},
	})
	return s.List(ctx, courseID)
}

// --- Groupings ---

func (s *Service) CreateGrouping(ctx context.Context, courseID string, in GroupInput) (*Grouping, error) {
	if in.Name == "" {
		return nil, ErrNameRequired
	}
	description := ""
	if in.Description != nil {
		description = *in.Description
	}
	var g Grouping
	err := s.pool.QueryRow(ctx, `
		INSERT INTO groupings (course_id, name, description) VALUES ($1, $2, $3)
		RETURNING id, course_id, name, description, created_at
	`, courseID, in.Name, description).Scan(&g.ID, &g.CourseID, &g.Name, &g.Description, &g.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrNameTaken
		}
		return nil, err
	}
	g.GroupIDs = []string{}
	return &g, nil
}

func (s *Service) UpdateGrouping(ctx context.Context, groupingID string, in GroupInput) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE groupings SET name = COALESCE(NULLIF($2, ''), name), description = COALESCE($3, description) WHERE id = $1
	`, groupingID, in.Name, in.Description)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrNameTaken
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) DeleteGrouping(ctx context.Context, groupingID string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM groupings WHERE id = $1`, groupingID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) ListGroupings(ctx context.Context, courseID string) ([]Grouping, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT g.id, g.course_id, g.name, g.description, g.created_at,
			COALESCE(array_agg(gg.group_id) FILTER (WHERE gg.group_id IS NOT NULL), '{}')
		FROM groupings g
		LEFT JOIN groupings_groups gg ON gg.grouping_id = g.id
		WHERE g.course_id = $1
		GROUP BY g.id ORDER BY g.name
	`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Grouping{}
	for rows.Next() {
		var g Grouping
		if err := rows.Scan(&g.ID, &g.CourseID, &g.Name, &g.Description, &g.CreatedAt, &g.GroupIDs); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Service) AddGroupToGrouping(ctx context.Context, groupingID, groupID string) error {
	var sameCourse bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM groupings gi JOIN groups g ON g.course_id = gi.course_id WHERE gi.id = $1 AND g.id = $2)
	`, groupingID, groupID).Scan(&sameCourse)
	if err != nil {
		return err
	}
	if !sameCourse {
		return ErrCrossCourse
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO groupings_groups (grouping_id, group_id) VALUES ($1, $2) ON CONFLICT DO NOTHING
	`, groupingID, groupID)
	return err
}

func (s *Service) RemoveGroupFromGrouping(ctx context.Context, groupingID, groupID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM groupings_groups WHERE grouping_id = $1 AND group_id = $2`, groupingID, groupID)
	return err
}
