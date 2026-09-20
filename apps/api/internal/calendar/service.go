// Package calendar implements calendar_events: course events, personal
// user events, site-wide events, and the deadline events activities own.
// It stores events and answers range queries, expanding repeating events
// into their occurrences; a monthly grid is a frontend rendering concern.
package calendar

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"polaris-api/internal/rbac"
)

var (
	ErrNotFound        = errors.New("event not found")
	ErrManagedByModule = errors.New("this event belongs to an activity; change the activity instead")
	ErrInvalidRepeat   = errors.New("repeat_rule must be none, daily, weekly or monthly")
	ErrInvalidRange    = errors.New("end_at and repeat_until must be after start_at")
	ErrNameRequired    = errors.New("name and start_at are required")
)

// maxOccurrences bounds the expansion of one repeating event so a daily
// event with no end cannot blow up a wide range query.
const maxOccurrences = 1000

type Service struct {
	pool *pgxpool.Pool
	rbac *rbac.Service
}

func NewService(pool *pgxpool.Pool, rbacService *rbac.Service) *Service {
	return &Service{pool: pool, rbac: rbacService}
}

const eventColumns = `
	id, event_type, course_id, user_id, course_module_id, event_kind, name, description,
	start_at, end_at, repeat_rule, repeat_until, created_at
`

func scanEvent(row pgx.Row) (*Event, error) {
	var e Event
	err := row.Scan(&e.ID, &e.EventType, &e.CourseID, &e.UserID, &e.CourseModuleID, &e.EventKind, &e.Name, &e.Description,
		&e.StartAt, &e.EndAt, &e.RepeatRule, &e.RepeatUntil, &e.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func validRepeat(rule string) bool {
	return rule == "none" || rule == "daily" || rule == "weekly" || rule == "monthly"
}

func (s *Service) create(ctx context.Context, eventType string, courseID, userID *string, in EventInput) (*Event, error) {
	if strings.TrimSpace(in.Name) == "" || in.StartAt == nil {
		return nil, ErrNameRequired
	}
	rule := "none"
	if in.RepeatRule != nil {
		rule = *in.RepeatRule
	}
	if !validRepeat(rule) {
		return nil, ErrInvalidRepeat
	}
	if (in.EndAt != nil && in.EndAt.Before(*in.StartAt)) || (in.RepeatUntil != nil && in.RepeatUntil.Before(*in.StartAt)) {
		return nil, ErrInvalidRange
	}
	description := ""
	if in.Description != nil {
		description = *in.Description
	}

	return scanEvent(s.pool.QueryRow(ctx, `
		INSERT INTO calendar_events (event_type, course_id, user_id, name, description, start_at, end_at, repeat_rule, repeat_until)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING `+eventColumns,
		eventType, courseID, userID, in.Name, description, *in.StartAt, in.EndAt, rule, in.RepeatUntil))
}

func (s *Service) CreateCourseEvent(ctx context.Context, courseID string, in EventInput) (*Event, error) {
	return s.create(ctx, "course", &courseID, nil, in)
}

func (s *Service) CreateUserEvent(ctx context.Context, userID string, in EventInput) (*Event, error) {
	return s.create(ctx, "user", nil, &userID, in)
}

func (s *Service) CreateSiteEvent(ctx context.Context, in EventInput) (*Event, error) {
	return s.create(ctx, "site", nil, nil, in)
}

// CanEdit reports whether userID may change or delete the event.
func (s *Service) CanEdit(ctx context.Context, userID, eventID string) (bool, error) {
	e, err := scanEvent(s.pool.QueryRow(ctx, `SELECT `+eventColumns+` FROM calendar_events WHERE id = $1`, eventID))
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, err
	}

	switch e.EventType {
	case "user":
		return e.UserID != nil && *e.UserID == userID, nil
	case "course":
		contextID, err := s.rbac.ContextID(ctx, rbac.ContextLevelCourse, *e.CourseID)
		if err != nil {
			return false, err
		}
		return s.rbac.Can(ctx, userID, "calendar:manageentries", contextID)
	default:
		contextID, err := s.rbac.SystemContextID(ctx)
		if err != nil {
			return false, err
		}
		return s.rbac.Can(ctx, userID, "calendar:manage", contextID)
	}
}

func (s *Service) Update(ctx context.Context, eventID string, in EventInput) (*Event, error) {
	cur, err := scanEvent(s.pool.QueryRow(ctx, `SELECT `+eventColumns+` FROM calendar_events WHERE id = $1`, eventID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if cur.CourseModuleID != nil {
		return nil, ErrManagedByModule
	}
	if in.RepeatRule != nil && !validRepeat(*in.RepeatRule) {
		return nil, ErrInvalidRepeat
	}

	start := cur.StartAt
	if in.StartAt != nil {
		start = *in.StartAt
	}
	end := cur.EndAt
	if in.EndAt != nil {
		end = in.EndAt
	}
	if in.ClearEnd {
		end = nil
	}
	until := cur.RepeatUntil
	if in.RepeatUntil != nil {
		until = in.RepeatUntil
	}
	if in.ClearRepeat {
		until = nil
	}
	if (end != nil && end.Before(start)) || (until != nil && until.Before(start)) {
		return nil, ErrInvalidRange
	}

	return scanEvent(s.pool.QueryRow(ctx, `
		UPDATE calendar_events SET
			name = COALESCE(NULLIF($2, ''), name),
			description = COALESCE($3, description),
			start_at = $4, end_at = $5,
			repeat_rule = COALESCE($6, repeat_rule), repeat_until = $7,
			updated_at = now()
		WHERE id = $1
		RETURNING `+eventColumns,
		eventID, strings.TrimSpace(in.Name), in.Description, start, end, in.RepeatRule, until))
}

func (s *Service) Delete(ctx context.Context, eventID string) error {
	var moduleID *string
	err := s.pool.QueryRow(ctx, `SELECT course_module_id FROM calendar_events WHERE id = $1`, eventID).Scan(&moduleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if moduleID != nil {
		return ErrManagedByModule
	}
	_, err = s.pool.Exec(ctx, `DELETE FROM calendar_events WHERE id = $1`, eventID)
	return err
}

// SetModuleEvent creates or moves the event an activity owns for one of its
// dates (a due date, an opening time, ...). There is at most one per
// (module, kind); activities call this whenever the date changes.
func (s *Service) SetModuleEvent(ctx context.Context, courseID, moduleID, kind, name string, at time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM calendar_events WHERE course_module_id = $1 AND event_kind = $2`, moduleID, kind); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO calendar_events (event_type, course_id, course_module_id, event_kind, name, start_at)
		VALUES ('course', $1, $2, $3, $4, $5)
	`, courseID, moduleID, kind, name, at); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ClearModuleEvent removes an activity's event of the given kind, for when
// the activity drops that date.
func (s *Service) ClearModuleEvent(ctx context.Context, moduleID, kind string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM calendar_events WHERE course_module_id = $1 AND event_kind = $2`, moduleID, kind)
	return err
}

// rangeFilter matches events that overlap [from, to] — or, for repeating
// events, might have an occurrence there — so callers expand afterwards.
const rangeFilter = `
	start_at <= $2 AND (
		(repeat_rule = 'none' AND COALESCE(end_at, start_at) >= $1)
		OR (repeat_rule <> 'none' AND (repeat_until IS NULL OR repeat_until >= $1))
	)
`

func (s *Service) ListCourseEvents(ctx context.Context, courseID, userID string, from, to time.Time) ([]Event, error) {
	events, err := s.query(ctx, `
		SELECT `+eventColumns+`, true FROM calendar_events
		WHERE `+rangeFilter+` AND event_type = 'course' AND course_id = $3
		ORDER BY start_at ASC
	`, from, to, courseID)
	if err != nil {
		return nil, err
	}
	return s.filterHiddenModuleEvents(ctx, userID, expand(events, from, to))
}

// ListMyEvents is the "my calendar" view: the user's own personal events,
// site-wide events, and course events for every course they are actively
// enrolled in — combined and ordered, so a monthly view is one call away.
func (s *Service) ListMyEvents(ctx context.Context, userID string, from, to time.Time) ([]Event, error) {
	events, err := s.query(ctx, `
		SELECT `+eventColumns+`, true FROM calendar_events
		WHERE `+rangeFilter+`
		  AND (
		    event_type = 'site'
		    OR (event_type = 'user' AND user_id = $3)
		    OR (event_type = 'course' AND course_id IN (
		        SELECT e.course_id FROM enrollments e
		        WHERE e.user_id = $3 AND e.status = 'active'
		          AND (e.time_start IS NULL OR e.time_start <= now())
		          AND (e.time_end IS NULL OR e.time_end > now())))
		  )
		ORDER BY start_at ASC
	`, from, to, userID)
	if err != nil {
		return nil, err
	}
	return s.filterHiddenModuleEvents(ctx, userID, expand(events, from, to))
}

// filterHiddenModuleEvents drops deadline events of hidden activities for
// people who may not see hidden activities.
func (s *Service) filterHiddenModuleEvents(ctx context.Context, userID string, events []Event) ([]Event, error) {
	hiddenModules := map[string]bool{}
	rows, err := s.pool.Query(ctx, `SELECT id FROM course_modules WHERE NOT visible AND id IN (SELECT course_module_id FROM calendar_events WHERE course_module_id IS NOT NULL)`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		hiddenModules[id] = true
	}
	rows.Close()
	if len(hiddenModules) == 0 {
		return events, nil
	}

	canSee := map[string]bool{}
	out := events[:0:0]
	for _, e := range events {
		if e.CourseModuleID == nil || !hiddenModules[*e.CourseModuleID] {
			out = append(out, e)
			continue
		}
		courseID := *e.CourseID
		allowed, cached := canSee[courseID]
		if !cached {
			contextID, err := s.rbac.ContextID(ctx, rbac.ContextLevelCourse, courseID)
			if err != nil {
				return nil, err
			}
			allowed, err = s.rbac.Can(ctx, userID, "mod:viewhidden", contextID)
			if err != nil {
				return nil, err
			}
			canSee[courseID] = allowed
		}
		if allowed {
			out = append(out, e)
		}
	}
	return out, nil
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
		var ignored bool
		if err := rows.Scan(&e.ID, &e.EventType, &e.CourseID, &e.UserID, &e.CourseModuleID, &e.EventKind, &e.Name, &e.Description,
			&e.StartAt, &e.EndAt, &e.RepeatRule, &e.RepeatUntil, &e.CreatedAt, &ignored); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func next(t time.Time, rule string, n int, origin time.Time) time.Time {
	switch rule {
	case "daily":
		return origin.AddDate(0, 0, n)
	case "weekly":
		return origin.AddDate(0, 0, 7*n)
	default:
		return origin.AddDate(0, n, 0)
	}
}

// expand turns repeating events into one Event per occurrence inside
// [from, to], sorted by start time. Occurrences keep the event id, since
// they are all edited together.
func expand(events []Event, from, to time.Time) []Event {
	out := make([]Event, 0, len(events))
	for _, e := range events {
		if e.RepeatRule == "none" {
			out = append(out, e)
			continue
		}
		var duration time.Duration
		if e.EndAt != nil {
			duration = e.EndAt.Sub(e.StartAt)
		}
		for i := 0; i < maxOccurrences; i++ {
			start := next(e.StartAt, e.RepeatRule, i, e.StartAt)
			if start.After(to) || (e.RepeatUntil != nil && start.After(*e.RepeatUntil)) {
				break
			}
			end := start.Add(duration)
			if end.Before(from) {
				continue
			}
			occ := e
			occ.StartAt = start
			if e.EndAt != nil {
				occ.EndAt = &end
			}
			out = append(out, occ)
		}
	}
	sortEvents(out)
	return out
}

func sortEvents(events []Event) {
	for i := 1; i < len(events); i++ {
		for j := i; j > 0 && events[j].StartAt.Before(events[j-1].StartAt); j-- {
			events[j], events[j-1] = events[j-1], events[j]
		}
	}
}

// ExportICS renders the user's calendar (the next and previous year) as an
// iCalendar file, with repeating events as RRULEs.
func (s *Service) ExportICS(ctx context.Context, userID string) (string, error) {
	now := time.Now()
	from, to := now.AddDate(-1, 0, 0), now.AddDate(1, 0, 0)
	events, err := s.query(ctx, `
		SELECT `+eventColumns+`, true FROM calendar_events
		WHERE `+rangeFilter+`
		  AND (
		    event_type = 'site'
		    OR (event_type = 'user' AND user_id = $3)
		    OR (event_type = 'course' AND course_id IN (SELECT course_id FROM enrollments WHERE user_id = $3 AND status = 'active'))
		  )
		ORDER BY start_at ASC
	`, from, to, userID)
	if err != nil {
		return "", err
	}
	events, err = s.filterHiddenModuleEvents(ctx, userID, events)
	if err != nil {
		return "", err
	}

	const layout = "20060102T150405Z"
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\r\n", args...) }
	line("BEGIN:VCALENDAR")
	line("VERSION:2.0")
	line("PRODID:-//Polaris//Calendar//EN")
	for _, e := range events {
		line("BEGIN:VEVENT")
		line("UID:%s@polaris", e.ID)
		line("DTSTAMP:%s", now.UTC().Format(layout))
		line("DTSTART:%s", e.StartAt.UTC().Format(layout))
		if e.EndAt != nil {
			line("DTEND:%s", e.EndAt.UTC().Format(layout))
		}
		line("SUMMARY:%s", icsEscape(e.Name))
		if e.Description != "" {
			line("DESCRIPTION:%s", icsEscape(e.Description))
		}
		if e.RepeatRule != "none" {
			rule := "FREQ=" + strings.ToUpper(e.RepeatRule)
			if e.RepeatUntil != nil {
				rule += ";UNTIL=" + e.RepeatUntil.UTC().Format(layout)
			}
			line("RRULE:%s", rule)
		}
		line("END:VEVENT")
	}
	line("END:VCALENDAR")
	return b.String(), nil
}

func icsEscape(s string) string {
	r := strings.NewReplacer("\\", "\\\\", ";", "\\;", ",", "\\,", "\r\n", "\\n", "\n", "\\n")
	return r.Replace(s)
}
