package course

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"polaris-api/internal/eventbus"
	"polaris-api/internal/rbac"
)

var (
	ErrSelfEnrolDisabled = errors.New("self enrolment is not enabled for this course")
	ErrSelfEnrolClosed   = errors.New("self enrolment is not open right now")
	ErrEnrolKeyWrong     = errors.New("the enrolment key is incorrect")
	ErrCourseFull        = errors.New("this course has reached its maximum number of self-enrolled users")
	ErrUserNotFound      = errors.New("no user with that email")
	ErrOwnerEnrolment    = errors.New("the course owner cannot be unenrolled")
	ErrNotEnrolled       = errors.New("that user is not enrolled in this course")
	ErrInvalidStatus     = errors.New("status must be 'active' or 'suspended'")
)

type enrolTarget struct {
	userID    string
	method    string
	role      string
	timeStart *time.Time
	timeEnd   *time.Time
}

// enrol records the enrolment through the given method, grants the role at
// the course context and announces it, so the manual and self paths share
// one write and one event.
func (s *Service) enrol(ctx context.Context, courseID string, t enrolTarget) error {
	var methodID string
	if err := s.pool.QueryRow(ctx, `
		SELECT id FROM enrolment_methods WHERE course_id = $1 AND type = $2
	`, courseID, t.method).Scan(&methodID); err != nil {
		return err
	}

	if _, err := s.pool.Exec(ctx, `
		INSERT INTO enrollments (course_id, user_id, enrolment_method_id, time_start, time_end)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (course_id, user_id) DO UPDATE SET
			time_start = COALESCE(EXCLUDED.time_start, enrollments.time_start),
			time_end = COALESCE(EXCLUDED.time_end, enrollments.time_end),
			status = 'active', updated_at = now()
	`, courseID, t.userID, methodID, t.timeStart, t.timeEnd); err != nil {
		return err
	}

	courseContextID, err := s.rbac.ContextID(ctx, rbac.ContextLevelCourse, courseID)
	if err != nil {
		return err
	}
	if err := s.rbac.AssignRole(ctx, t.userID, t.role, courseContextID); err != nil {
		return err
	}

	s.events.Dispatch(ctx, eventbus.Event{
		Name:      "course.enrolled",
		ContextID: &courseContextID,
		UserID:    &t.userID,
		Data:      map[string]any{"course_id": courseID, "method": t.method, "role": t.role},
	})
	return nil
}

// EnrolSelf lets a user join a course on their own, subject to the
// method's switch, enrolment window, key, seat limit and duration.
func (s *Service) EnrolSelf(ctx context.Context, courseID, userID, key string) error {
	var (
		enabled      bool
		enrolKey     string
		maxUsers     *int
		enrolStart   *time.Time
		enrolEnd     *time.Time
		durationDays *int
		roleName     *string
		visible      bool
	)
	err := s.pool.QueryRow(ctx, `
		SELECT em.enabled, em.enrol_key, em.max_users, em.enrol_start, em.enrol_end, em.duration_days, r.name, c.visible
		FROM enrolment_methods em
		JOIN courses c ON c.id = em.course_id
		LEFT JOIN roles r ON r.id = em.role_id
		WHERE em.course_id = $1 AND em.type = 'self'
	`, courseID).Scan(&enabled, &enrolKey, &maxUsers, &enrolStart, &enrolEnd, &durationDays, &roleName, &visible)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrSelfEnrolDisabled
	}
	if err != nil {
		return err
	}
	if !enabled || !visible {
		return ErrSelfEnrolDisabled
	}

	now := time.Now()
	if (enrolStart != nil && now.Before(*enrolStart)) || (enrolEnd != nil && now.After(*enrolEnd)) {
		return ErrSelfEnrolClosed
	}
	if enrolKey != "" && key != enrolKey {
		return ErrEnrolKeyWrong
	}
	if maxUsers != nil {
		var used int
		if err := s.pool.QueryRow(ctx, `
			SELECT count(*) FROM enrollments e
			JOIN enrolment_methods em ON em.id = e.enrolment_method_id
			WHERE e.course_id = $1 AND em.type = 'self'
		`, courseID).Scan(&used); err != nil {
			return err
		}
		if used >= *maxUsers {
			return ErrCourseFull
		}
	}

	role := "student"
	if roleName != nil {
		role = *roleName
	}
	t := enrolTarget{userID: userID, method: "self", role: role}
	if durationDays != nil {
		end := now.AddDate(0, 0, *durationDays)
		t.timeEnd = &end
	}
	return s.enrol(ctx, courseID, t)
}

// EnrolManual is the manual method: someone with enrol:manage adds a user,
// picking the role they get in the course.
func (s *Service) EnrolManual(ctx context.Context, courseID string, in EnrolInput) error {
	var userID string
	var err error
	switch {
	case in.UserID != "":
		err = s.pool.QueryRow(ctx, `SELECT id FROM users WHERE id = $1`, in.UserID).Scan(&userID)
	default:
		err = s.pool.QueryRow(ctx, `SELECT id FROM users WHERE lower(email) = lower($1)`, in.Email).Scan(&userID)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUserNotFound
	}
	if err != nil {
		return err
	}

	role := in.Role
	if role == "" {
		role = "student"
	}
	return s.enrol(ctx, courseID, enrolTarget{
		userID: userID, method: "manual", role: role, timeStart: in.TimeStart, timeEnd: in.TimeEnd,
	})
}

// UpdateEnrolment suspends or reactivates an enrolment and edits its
// time window.
func (s *Service) UpdateEnrolment(ctx context.Context, courseID, userID string, u EnrolmentUpdate) error {
	if u.Status != nil && *u.Status != "active" && *u.Status != "suspended" {
		return ErrInvalidStatus
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE enrollments SET
			status = COALESCE($3, status),
			time_start = COALESCE($4, time_start),
			time_end = CASE WHEN $6 THEN NULL ELSE COALESCE($5, time_end) END,
			updated_at = now()
		WHERE course_id = $1 AND user_id = $2
	`, courseID, userID, u.Status, u.TimeStart, u.TimeEnd, u.ClearEnd)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotEnrolled
	}
	return nil
}

func (s *Service) Unenrol(ctx context.Context, courseID, userID string) error {
	var isOwner bool
	if err := s.pool.QueryRow(ctx,
		`SELECT owner_id = $2 FROM courses WHERE id = $1`, courseID, userID,
	).Scan(&isOwner); err != nil {
		return err
	}
	if isOwner {
		return ErrOwnerEnrolment
	}

	courseContextID, err := s.rbac.ContextID(ctx, rbac.ContextLevelCourse, courseID)
	if err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx,
		`DELETE FROM enrollments WHERE course_id = $1 AND user_id = $2`, courseID, userID); err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, `
		DELETE FROM group_members WHERE user_id = $2
		AND group_id IN (SELECT id FROM groups WHERE course_id = $1)
	`, courseID, userID); err != nil {
		return err
	}
	if err := s.rbac.UnassignAll(ctx, userID, courseContextID); err != nil {
		return err
	}

	s.events.Dispatch(ctx, eventbus.Event{
		Name:      "course.unenrolled",
		ContextID: &courseContextID,
		UserID:    &userID,
		Data:      map[string]any{"course_id": courseID},
	})
	return nil
}

func (s *Service) Participants(ctx context.Context, courseID string) ([]Participant, error) {
	courseContextID, err := s.rbac.ContextID(ctx, rbac.ContextLevelCourse, courseID)
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.name, u.email, COALESCE(em.type, 'manual'), e.status, e.time_start, e.time_end, e.created_at,
			COALESCE(array_agg(r.name ORDER BY r.name) FILTER (WHERE r.name IS NOT NULL), '{}')
		FROM enrollments e
		JOIN users u ON u.id = e.user_id
		LEFT JOIN enrolment_methods em ON em.id = e.enrolment_method_id
		LEFT JOIN role_assignments ra ON ra.user_id = u.id AND ra.context_id = $2
		LEFT JOIN roles r ON r.id = ra.role_id
		WHERE e.course_id = $1
		GROUP BY u.id, u.name, u.email, em.type, e.status, e.time_start, e.time_end, e.created_at
		ORDER BY u.name ASC
	`, courseID, courseContextID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Participant{}
	for rows.Next() {
		var p Participant
		if err := rows.Scan(&p.UserID, &p.Name, &p.Email, &p.Method, &p.Status, &p.TimeStart, &p.TimeEnd, &p.EnrolledAt, &p.Roles); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Service) GetSelfEnrolment(ctx context.Context, courseID string) (*SelfEnrolment, error) {
	var se SelfEnrolment
	var roleName *string
	err := s.pool.QueryRow(ctx, `
		SELECT em.enabled, em.enrol_key, em.max_users, em.enrol_start, em.enrol_end, em.duration_days, r.name
		FROM enrolment_methods em LEFT JOIN roles r ON r.id = em.role_id
		WHERE em.course_id = $1 AND em.type = 'self'
	`, courseID).Scan(&se.Enabled, &se.Key, &se.MaxUsers, &se.EnrolStart, &se.EnrolEnd, &se.DurationDays, &roleName)
	if errors.Is(err, pgx.ErrNoRows) {
		return &SelfEnrolment{Role: "student"}, nil
	}
	if err != nil {
		return nil, err
	}
	se.Role = "student"
	if roleName != nil {
		se.Role = *roleName
	}
	return &se, nil
}

func (s *Service) SetSelfEnrolment(ctx context.Context, courseID string, in SelfEnrolment) error {
	if in.Role == "" {
		in.Role = "student"
	}
	role, err := s.rbac.RoleByName(ctx, in.Role)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO enrolment_methods (course_id, type, enabled, enrol_key, max_users, enrol_start, enrol_end, duration_days, role_id)
		VALUES ($1, 'self', $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (course_id, type) DO UPDATE SET
			enabled = EXCLUDED.enabled, enrol_key = EXCLUDED.enrol_key, max_users = EXCLUDED.max_users,
			enrol_start = EXCLUDED.enrol_start, enrol_end = EXCLUDED.enrol_end,
			duration_days = EXCLUDED.duration_days, role_id = EXCLUDED.role_id
	`, courseID, in.Enabled, in.Key, in.MaxUsers, in.EnrolStart, in.EnrolEnd, in.DurationDays, role.ID)
	return err
}
