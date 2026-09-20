package course

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"polaris-api/internal/coursemodule"
	"polaris-api/internal/eventbus"
	"polaris-api/internal/rbac"
)

var (
	ErrShortNameTaken = errors.New("another course already uses that short name")
	ErrIDNumberTaken  = errors.New("another course already uses that ID number")
	ErrCourseNotFound = errors.New("course not found")
	ErrInvalidFormat  = errors.New("format must be 'topics' or 'weeks'")
)

// activeEnrolment is the SQL predicate for an enrolment that currently
// grants access: not suspended and inside its time window.
const activeEnrolment = `e.status = 'active'
	AND (e.time_start IS NULL OR e.time_start <= now())
	AND (e.time_end IS NULL OR e.time_end > now())`

type Service struct {
	pool    *pgxpool.Pool
	rbac    *rbac.Service
	events  *eventbus.Dispatcher
	modules *coursemodule.Service
}

func NewService(pool *pgxpool.Pool, rbacService *rbac.Service, events *eventbus.Dispatcher, modules *coursemodule.Service) *Service {
	return &Service{pool: pool, rbac: rbacService, events: events, modules: modules}
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func generateShortName(title string) string {
	slug := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if len(slug) > 24 {
		slug = strings.Trim(slug[:24], "-")
	}
	suffix := make([]byte, 2)
	rand.Read(suffix)
	if slug == "" {
		slug = "course"
	}
	return slug + "-" + hex.EncodeToString(suffix)
}

// CanCreateIn reports whether userID may create a course in the given
// category (or at the site level when categoryID is nil).
func (s *Service) CanCreateIn(ctx context.Context, userID string, categoryID *string) (bool, error) {
	var contextID string
	var err error
	if categoryID != nil {
		contextID, err = s.rbac.ContextID(ctx, rbac.ContextLevelCategory, *categoryID)
	} else {
		contextID, err = s.rbac.SystemContextID(ctx)
	}
	if err != nil {
		return false, err
	}
	return s.rbac.Can(ctx, userID, "course:create", contextID)
}

// Create inserts the course together with its default "General" section,
// manual and self enrolment methods and the grade category root every
// course needs, enrols the creator as its teacher, and emits course.created
// so other modules can react without Create knowing about them.
func (s *Service) Create(ctx context.Context, ownerID string, in CourseInput) (*Course, error) {
	if in.Format == "" {
		in.Format = "topics"
	}
	if in.Format != "topics" && in.Format != "weeks" {
		return nil, ErrInvalidFormat
	}
	if in.ShortName == "" {
		in.ShortName = generateShortName(in.Title)
	}
	visible := true
	if in.Visible != nil {
		visible = *in.Visible
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO courses (owner_id, title, short_name, id_number, description, category_id, format, start_date, end_date, visible)
		VALUES ($1, $2, $3, $4, $5, COALESCE($6::uuid, (SELECT id FROM course_categories WHERE is_default)), $7, $8, $9, $10)
		RETURNING id
	`, ownerID, in.Title, in.ShortName, in.IDNumber, in.Description, in.CategoryID, in.Format, in.StartDate, in.EndDate, visible).Scan(&id)
	if err != nil {
		return nil, mapCourseConflict(err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO course_sections (course_id, title, position) VALUES ($1, 'General', 0)
	`, id); err != nil {
		return nil, err
	}

	var manualMethodID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO enrolment_methods (course_id, type, role_id)
		VALUES ($1, 'manual', (SELECT id FROM roles WHERE name = 'student')) RETURNING id
	`, id).Scan(&manualMethodID); err != nil {
		return nil, err
	}

	// Self enrolment is opt-in per course, so it starts disabled.
	if _, err := tx.Exec(ctx, `
		INSERT INTO enrolment_methods (course_id, type, enabled, role_id)
		VALUES ($1, 'self', false, (SELECT id FROM roles WHERE name = 'student'))
	`, id); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO enrollments (course_id, user_id, enrolment_method_id) VALUES ($1, $2, $3)
	`, id, ownerID, manualMethodID); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO grade_categories (course_id, name, is_root) VALUES ($1, 'Course total', true)
	`, id); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	courseContextID, err := s.rbac.ContextID(ctx, rbac.ContextLevelCourse, id)
	if err != nil {
		return nil, err
	}
	if err := s.rbac.AssignRole(ctx, ownerID, "teacher", courseContextID); err != nil {
		return nil, err
	}

	s.events.Dispatch(ctx, eventbus.Event{
		Name:      "course.created",
		ContextID: &courseContextID,
		UserID:    &ownerID,
		Data:      map[string]any{"course_id": id, "title": in.Title},
	})

	return s.Get(ctx, id)
}

func mapCourseConflict(err error) error {
	var pgErr interface {
		SQLState() string
		Error() string
	}
	if errors.As(err, &pgErr) && pgErr.SQLState() == "23505" {
		if strings.Contains(pgErr.Error(), "id_number") {
			return ErrIDNumberTaken
		}
		return ErrShortNameTaken
	}
	return err
}

// Update applies a full edit of the course settings.
func (s *Service) Update(ctx context.Context, courseID, userID string, in CourseInput) (*Course, error) {
	if in.Format != "" && in.Format != "topics" && in.Format != "weeks" {
		return nil, ErrInvalidFormat
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE courses SET
			title = COALESCE(NULLIF($2, ''), title),
			short_name = COALESCE(NULLIF($3, ''), short_name),
			id_number = $4,
			description = $5,
			category_id = COALESCE($6::uuid, category_id),
			format = COALESCE(NULLIF($7, ''), format),
			start_date = $8,
			end_date = $9,
			visible = COALESCE($10, visible),
			updated_at = now()
		WHERE id = $1
	`, courseID, in.Title, in.ShortName, in.IDNumber, in.Description, in.CategoryID, in.Format, in.StartDate, in.EndDate, in.Visible)
	if err != nil {
		return nil, mapCourseConflict(err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrCourseNotFound
	}

	// A category change moves the course in the context tree; resolving the
	// context refreshes its parent.
	courseContextID, err := s.rbac.ContextID(ctx, rbac.ContextLevelCourse, courseID)
	if err != nil {
		return nil, err
	}
	s.events.Dispatch(ctx, eventbus.Event{
		Name:      "course.updated",
		ContextID: &courseContextID,
		UserID:    &userID,
		Data:      map[string]any{"course_id": courseID},
	})
	return s.Get(ctx, courseID)
}

// Delete removes a course and every activity instance inside it.
func (s *Service) Delete(ctx context.Context, courseID, userID string) error {
	rows, err := s.pool.Query(ctx, `
		SELECT cm.id FROM course_modules cm
		JOIN course_sections cs ON cs.id = cm.section_id WHERE cs.course_id = $1
	`, courseID)
	if err != nil {
		return err
	}
	var moduleIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		moduleIDs = append(moduleIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range moduleIDs {
		if err := s.modules.Delete(ctx, id); err != nil {
			return err
		}
	}

	tag, err := s.pool.Exec(ctx, `DELETE FROM courses WHERE id = $1`, courseID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrCourseNotFound
	}
	s.events.Dispatch(ctx, eventbus.Event{
		Name:   "course.deleted",
		UserID: &userID,
		Data:   map[string]any{"course_id": courseID},
	})
	return nil
}

const courseColumns = `
	c.id, c.owner_id, c.category_id, c.title, c.short_name, c.id_number, c.description,
	c.format, c.start_date, c.end_date, c.visible, c.created_at,
	COALESCE(se.enabled, false), COALESCE(se.enrol_key, '') <> ''
`

const courseFrom = `
	FROM courses c
	LEFT JOIN enrolment_methods se ON se.course_id = c.id AND se.type = 'self'
`

func scanCourse(row pgx.Row) (*Course, error) {
	var c Course
	err := row.Scan(&c.ID, &c.OwnerID, &c.CategoryID, &c.Title, &c.ShortName, &c.IDNumber, &c.Description,
		&c.Format, &c.StartDate, &c.EndDate, &c.Visible, &c.CreatedAt, &c.SelfEnrol, &c.RequiresKey)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Service) Get(ctx context.Context, courseID string) (*Course, error) {
	c, err := scanCourse(s.pool.QueryRow(ctx, `SELECT `+courseColumns+courseFrom+` WHERE c.id = $1`, courseID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCourseNotFound
	}
	return c, err
}

func (s *Service) queryCourses(ctx context.Context, query string, args ...any) ([]Course, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	courses := []Course{}
	for rows.Next() {
		c, err := scanCourse(rows)
		if err != nil {
			return nil, err
		}
		courses = append(courses, *c)
	}
	return courses, rows.Err()
}

// List returns the courses userID can open: their active enrolments, plus
// — for people holding course:view through a site or category role, such as
// managers and admins — every other course that permission reaches.
func (s *Service) List(ctx context.Context, userID string) ([]Course, error) {
	all, err := s.queryCourses(ctx, `SELECT `+courseColumns+courseFrom+` ORDER BY c.created_at DESC`)
	if err != nil {
		return nil, err
	}

	enrolled := map[string]bool{}
	rows, err := s.pool.Query(ctx, `SELECT e.course_id FROM enrollments e WHERE e.user_id = $1 AND `+activeEnrolment, userID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		enrolled[id] = true
	}
	rows.Close()

	// Only people with a site or category level role can open a course they
	// are not enrolled in, so everyone else skips the per-course check.
	var hasBroadRole bool
	if err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM role_assignments ra
			JOIN roles r ON r.id = ra.role_id
			JOIN contexts x ON x.id = ra.context_id
			WHERE ra.user_id = $1 AND x.level IN ('system', 'category') AND r.name <> 'coursecreator'
		)
	`, userID).Scan(&hasBroadRole); err != nil {
		return nil, err
	}

	out := []Course{}
	for _, c := range all {
		if !enrolled[c.ID] && !hasBroadRole {
			continue
		}
		ok, err := s.canOpen(ctx, userID, c.ID, c.Visible, enrolled[c.ID])
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, c)
		}
	}
	return out, nil
}

// canOpen applies the access rule: an active enrolment opens a course (a
// hidden course only for people who may see hidden courses); without one,
// only the course:view capability does.
func (s *Service) canOpen(ctx context.Context, userID, courseID string, visible, enrolled bool) (bool, error) {
	courseContextID, err := s.rbac.ContextID(ctx, rbac.ContextLevelCourse, courseID)
	if err != nil {
		return false, err
	}
	if enrolled {
		if visible {
			return true, nil
		}
		return s.rbac.Can(ctx, userID, "course:viewhidden", courseContextID)
	}
	return s.rbac.Can(ctx, userID, "course:view", courseContextID)
}

func (s *Service) CanAccess(ctx context.Context, userID, courseID string) (bool, error) {
	var visible, enrolled bool
	err := s.pool.QueryRow(ctx, `
		SELECT c.visible,
			EXISTS (SELECT 1 FROM enrollments e WHERE e.course_id = c.id AND e.user_id = $2 AND `+activeEnrolment+`)
		FROM courses c WHERE c.id = $1
	`, courseID, userID).Scan(&visible, &enrolled)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return s.canOpen(ctx, userID, courseID, visible, enrolled)
}

// ListAvailable returns visible courses that have self enrolment switched
// on and that userID has not joined yet.
func (s *Service) ListAvailable(ctx context.Context, userID string) ([]Course, error) {
	return s.queryCourses(ctx,
		`SELECT `+courseColumns+courseFrom+`
		 WHERE COALESCE(se.enabled, false) AND c.visible
		   AND (se.enrol_start IS NULL OR se.enrol_start <= now())
		   AND (se.enrol_end IS NULL OR se.enrol_end > now())
		   AND NOT EXISTS (SELECT 1 FROM enrollments e WHERE e.course_id = c.id AND e.user_id = $1)
		 ORDER BY c.created_at DESC`,
		userID)
}
