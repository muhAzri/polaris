package course

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"polaris-api/internal/rbac"
)

var (
	ErrCategoryNotFound   = errors.New("category not found")
	ErrCategoryCycle      = errors.New("a category cannot be moved into itself or one of its own subcategories")
	ErrDefaultCategory    = errors.New("the default category cannot be deleted")
	ErrCategoryNotEmpty   = errors.New("category still has courses or subcategories; pass move_to to relocate them")
	ErrCategoryNameNeeded = errors.New("category name is required")
)

const categoryColumns = `
	cc.id, cc.parent_id, cc.name, cc.description, cc.id_number, cc.visible, cc.is_default, cc.sort_order, cc.created_at,
	(SELECT count(*) FROM courses c WHERE c.category_id = cc.id)
`

func scanCategory(row pgx.Row) (*Category, error) {
	var c Category
	err := row.Scan(&c.ID, &c.ParentID, &c.Name, &c.Description, &c.IDNumber, &c.Visible, &c.IsDefault, &c.SortOrder, &c.CreatedAt, &c.CourseCount)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ListCategories returns the flat category list; the client builds the
// tree from parent_id. Hidden categories are left out unless
// includeHidden is set.
func (s *Service) ListCategories(ctx context.Context, includeHidden bool) ([]Category, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+categoryColumns+` FROM course_categories cc
		WHERE $1 OR cc.visible ORDER BY cc.sort_order, cc.name
	`, includeHidden)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Category{}
	for rows.Next() {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (s *Service) GetCategory(ctx context.Context, id string) (*Category, error) {
	c, err := scanCategory(s.pool.QueryRow(ctx, `SELECT `+categoryColumns+` FROM course_categories cc WHERE cc.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCategoryNotFound
	}
	return c, err
}

// CanManageCategory checks category:manage at the parent (or the site).
func (s *Service) CanManageIn(ctx context.Context, userID string, parentID *string) (bool, error) {
	var contextID string
	var err error
	if parentID != nil {
		contextID, err = s.rbac.ContextID(ctx, rbac.ContextLevelCategory, *parentID)
	} else {
		contextID, err = s.rbac.SystemContextID(ctx)
	}
	if err != nil {
		return false, err
	}
	return s.rbac.Can(ctx, userID, "category:manage", contextID)
}

func (s *Service) CreateCategory(ctx context.Context, in CategoryInput) (*Category, error) {
	if in.Name == "" {
		return nil, ErrCategoryNameNeeded
	}
	visible := true
	if in.Visible != nil {
		visible = *in.Visible
	}
	sortOrder := 0
	if in.SortOrder != nil {
		sortOrder = *in.SortOrder
	}

	var id string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO course_categories (name, parent_id, description, id_number, visible, sort_order)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id
	`, in.Name, in.ParentID, in.Description, in.IDNumber, visible, sortOrder).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.GetCategory(ctx, id)
}

func (s *Service) UpdateCategory(ctx context.Context, id string, in CategoryInput) (*Category, error) {
	if in.ParentID != nil {
		cyclic, err := s.wouldCycle(ctx, id, *in.ParentID)
		if err != nil {
			return nil, err
		}
		if cyclic {
			return nil, ErrCategoryCycle
		}
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE course_categories SET
			name = COALESCE(NULLIF($2, ''), name),
			parent_id = COALESCE($3::uuid, parent_id),
			description = $4,
			id_number = $5,
			visible = COALESCE($6, visible),
			sort_order = COALESCE($7, sort_order),
			updated_at = now()
		WHERE id = $1
	`, id, in.Name, in.ParentID, in.Description, in.IDNumber, in.Visible, in.SortOrder)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrCategoryNotFound
	}
	return s.GetCategory(ctx, id)
}

// wouldCycle reports whether making newParent the parent of id would put
// id underneath itself.
func (s *Service) wouldCycle(ctx context.Context, id, newParent string) (bool, error) {
	var cyclic bool
	err := s.pool.QueryRow(ctx, `
		WITH RECURSIVE ancestors AS (
			SELECT id, parent_id FROM course_categories WHERE id = $2
			UNION ALL
			SELECT c.id, c.parent_id FROM course_categories c JOIN ancestors a ON c.id = a.parent_id
		)
		SELECT EXISTS (SELECT 1 FROM ancestors WHERE id = $1)
	`, id, newParent).Scan(&cyclic)
	return cyclic, err
}

// DeleteCategory removes a category. If it still holds courses or
// subcategories they are moved to moveTo (the default category when nil
// and moveToDefault is set); otherwise deletion is refused.
func (s *Service) DeleteCategory(ctx context.Context, id string, moveTo *string) error {
	var isDefault bool
	err := s.pool.QueryRow(ctx, `SELECT is_default FROM course_categories WHERE id = $1`, id).Scan(&isDefault)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrCategoryNotFound
	}
	if err != nil {
		return err
	}
	if isDefault {
		return ErrDefaultCategory
	}

	var inUse bool
	if err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM courses WHERE category_id = $1)
			OR EXISTS (SELECT 1 FROM course_categories WHERE parent_id = $1)
	`, id).Scan(&inUse); err != nil {
		return err
	}
	if inUse && moveTo == nil {
		return ErrCategoryNotEmpty
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if moveTo != nil {
		cyclic, err := s.wouldCycle(ctx, id, *moveTo)
		if err != nil {
			return err
		}
		if cyclic || *moveTo == id {
			return ErrCategoryCycle
		}
		if _, err := tx.Exec(ctx, `UPDATE courses SET category_id = $2 WHERE category_id = $1`, id, *moveTo); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE course_categories SET parent_id = $2 WHERE parent_id = $1`, id, *moveTo); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM course_categories WHERE id = $1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
