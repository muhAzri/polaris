package course

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"polaris-api/internal/rbac"
)

var (
	ErrSectionNotFound = errors.New("section not found")
	ErrGeneralSection  = errors.New("the first section cannot be moved or deleted")
	ErrInvalidPosition = errors.New("position is outside the course's sections")
)

func (s *Service) ListSections(ctx context.Context, courseID string) ([]Section, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, course_id, title, summary, position, visible, created_at
		FROM course_sections WHERE course_id = $1 ORDER BY position ASC
	`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sections := []Section{}
	for rows.Next() {
		var sec Section
		if err := rows.Scan(&sec.ID, &sec.CourseID, &sec.Title, &sec.Summary, &sec.Position, &sec.Visible, &sec.CreatedAt); err != nil {
			return nil, err
		}
		sections = append(sections, sec)
	}
	return sections, rows.Err()
}

// ListSectionsFor is ListSections without the sections hidden from userID.
func (s *Service) ListSectionsFor(ctx context.Context, courseID, userID string) ([]Section, error) {
	all, err := s.ListSections(ctx, courseID)
	if err != nil {
		return nil, err
	}
	courseContextID, err := s.rbac.ContextID(ctx, rbac.ContextLevelCourse, courseID)
	if err != nil {
		return nil, err
	}
	seeHidden, err := s.rbac.Can(ctx, userID, "mod:viewhidden", courseContextID)
	if err != nil {
		return nil, err
	}
	if seeHidden {
		return all, nil
	}
	visible := make([]Section, 0, len(all))
	for _, sec := range all {
		if sec.Visible {
			visible = append(visible, sec)
		}
	}
	return visible, nil
}

func (s *Service) CourseIDForSection(ctx context.Context, sectionID string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `SELECT course_id FROM course_sections WHERE id = $1`, sectionID).Scan(&id)
	return id, err
}

// CreateSection appends a new section at the end of the course.
func (s *Service) CreateSection(ctx context.Context, courseID, title, summary string) (*Section, error) {
	var sec Section
	err := s.pool.QueryRow(ctx, `
		INSERT INTO course_sections (course_id, title, summary, position)
		VALUES ($1, $2, $3, (SELECT COALESCE(MAX(position) + 1, 0) FROM course_sections WHERE course_id = $1))
		RETURNING id, course_id, title, summary, position, visible, created_at
	`, courseID, title, summary).Scan(&sec.ID, &sec.CourseID, &sec.Title, &sec.Summary, &sec.Position, &sec.Visible, &sec.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &sec, nil
}

// UpdateSection edits a section's text and visibility and, when a position
// is given, moves it, shifting the sections in between. Position 0 is the
// general section, which stays first.
func (s *Service) UpdateSection(ctx context.Context, sectionID string, in SectionInput) (*Section, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var courseID string
	var oldPos int
	err = tx.QueryRow(ctx, `SELECT course_id, position FROM course_sections WHERE id = $1 FOR UPDATE`, sectionID).Scan(&courseID, &oldPos)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSectionNotFound
	}
	if err != nil {
		return nil, err
	}

	if in.Position != nil && *in.Position != oldPos {
		if oldPos == 0 || *in.Position < 1 {
			return nil, ErrGeneralSection
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM course_sections WHERE course_id = $1`, courseID).Scan(&count); err != nil {
			return nil, err
		}
		if *in.Position >= count {
			return nil, ErrInvalidPosition
		}

		if *in.Position > oldPos {
			_, err = tx.Exec(ctx, `
				UPDATE course_sections SET position = position - 1
				WHERE course_id = $1 AND position > $2 AND position <= $3
			`, courseID, oldPos, *in.Position)
		} else {
			_, err = tx.Exec(ctx, `
				UPDATE course_sections SET position = position + 1
				WHERE course_id = $1 AND position >= $3 AND position < $2
			`, courseID, oldPos, *in.Position)
		}
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE course_sections SET position = $2 WHERE id = $1`, sectionID, *in.Position); err != nil {
			return nil, err
		}
	}

	var sec Section
	err = tx.QueryRow(ctx, `
		UPDATE course_sections SET
			title = COALESCE($2, title),
			summary = COALESCE($3, summary),
			visible = COALESCE($4, visible)
		WHERE id = $1
		RETURNING id, course_id, title, summary, position, visible, created_at
	`, sectionID, in.Title, in.Summary, in.Visible).Scan(&sec.ID, &sec.CourseID, &sec.Title, &sec.Summary, &sec.Position, &sec.Visible, &sec.CreatedAt)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &sec, nil
}

// DeleteSection removes a section along with the activities in it and
// closes the gap in the numbering.
func (s *Service) DeleteSection(ctx context.Context, sectionID string) error {
	var courseID string
	var position int
	err := s.pool.QueryRow(ctx, `SELECT course_id, position FROM course_sections WHERE id = $1`, sectionID).Scan(&courseID, &position)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrSectionNotFound
	}
	if err != nil {
		return err
	}
	if position == 0 {
		return ErrGeneralSection
	}

	mods, err := s.modules.ListBySection(ctx, sectionID)
	if err != nil {
		return err
	}
	for _, m := range mods {
		if err := s.modules.Delete(ctx, m.ID); err != nil {
			return err
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM course_sections WHERE id = $1`, sectionID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE course_sections SET position = position - 1 WHERE course_id = $1 AND position > $2
	`, courseID, position); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
