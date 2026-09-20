// Package gradebook implements the course gradebook: a tree of grade
// categories holding grade items, per-user grades with hidden/locked/
// overridden/excluded flags, a change history, and the aggregation that
// turns the tree into category and course totals. Activity modules
// (assignment/quiz/forum) create a grade item for each graded instance and
// push grades through PushGrade rather than keeping their own grade columns.
package gradebook

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"polaris-api/internal/eventbus"
)

var (
	ErrNotFound         = errors.New("not found")
	ErrInvalidGrade     = errors.New("grade is outside the item's minimum and maximum")
	ErrGradeLocked      = errors.New("this grade is locked")
	ErrRootCategory     = errors.New("the course total category cannot be moved or deleted")
	ErrModuleItem       = errors.New("items created by an activity cannot be deleted here; delete the activity instead")
	ErrInvalidMethod    = errors.New("unknown aggregation method")
	ErrInvalidRange     = errors.New("max_grade must be greater than min_grade")
	ErrCategoryCycle    = errors.New("a category cannot be moved under itself")
	ErrInvalidParent    = errors.New("parent category must belong to the same course")
	ErrNameRequired     = errors.New("name is required")
	ErrInvalidDropCount = errors.New("drop_lowest cannot be negative")
)

var aggregationMethods = map[string]bool{
	"mean": true, "weighted": true, "natural": true, "min": true, "max": true, "median": true, "mode": true,
}

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

func (s *Service) CourseIDForCategory(ctx context.Context, categoryID string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `SELECT course_id FROM grade_categories WHERE id = $1`, categoryID).Scan(&id)
	return id, err
}

// --- Categories ---

const categoryColumns = `id, course_id, parent_id, is_root, name, aggregation, max_grade, weight, drop_lowest, hidden, sort_order, created_at`

func scanCategory(row pgx.Row) (*Category, error) {
	var c Category
	err := row.Scan(&c.ID, &c.CourseID, &c.ParentID, &c.IsRoot, &c.Name, &c.Aggregation, &c.MaxGrade, &c.Weight, &c.DropLowest, &c.Hidden, &c.SortOrder, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Service) rootCategoryID(ctx context.Context, courseID string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `SELECT id FROM grade_categories WHERE course_id = $1 AND is_root`, courseID).Scan(&id)
	return id, err
}

func (s *Service) CreateCategory(ctx context.Context, courseID string, in CategoryInput) (*Category, error) {
	if in.Name == "" {
		return nil, ErrNameRequired
	}
	method := "mean"
	if in.Aggregation != nil {
		method = *in.Aggregation
	}
	if !aggregationMethods[method] {
		return nil, ErrInvalidMethod
	}

	parentID := ""
	if in.ParentID != nil {
		parentID = *in.ParentID
	} else {
		root, err := s.rootCategoryID(ctx, courseID)
		if err != nil {
			return nil, err
		}
		parentID = root
	}
	var sameCourse bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM grade_categories WHERE id = $1 AND course_id = $2)`, parentID, courseID).Scan(&sameCourse); err != nil {
		return nil, err
	}
	if !sameCourse {
		return nil, ErrInvalidParent
	}

	maxGrade := 100.0
	if in.MaxGrade != nil {
		maxGrade = *in.MaxGrade
	}
	drop := 0
	if in.DropLowest != nil {
		drop = *in.DropLowest
	}
	if drop < 0 {
		return nil, ErrInvalidDropCount
	}
	hidden := in.Hidden != nil && *in.Hidden

	return scanCategory(s.pool.QueryRow(ctx, `
		INSERT INTO grade_categories (course_id, parent_id, name, aggregation, max_grade, weight, drop_lowest, hidden)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+categoryColumns,
		courseID, parentID, in.Name, method, maxGrade, in.Weight, drop, hidden))
}

func (s *Service) UpdateCategory(ctx context.Context, categoryID string, in CategoryInput) (*Category, error) {
	cur, err := scanCategory(s.pool.QueryRow(ctx, `SELECT `+categoryColumns+` FROM grade_categories WHERE id = $1`, categoryID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if in.Aggregation != nil && !aggregationMethods[*in.Aggregation] {
		return nil, ErrInvalidMethod
	}
	if in.DropLowest != nil && *in.DropLowest < 0 {
		return nil, ErrInvalidDropCount
	}
	if in.ParentID != nil && *in.ParentID != "" {
		if cur.IsRoot {
			return nil, ErrRootCategory
		}
		var sameCourse, cyclic bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM grade_categories WHERE id = $1 AND course_id = $2)`, *in.ParentID, cur.CourseID).Scan(&sameCourse); err != nil {
			return nil, err
		}
		if !sameCourse {
			return nil, ErrInvalidParent
		}
		if err := s.pool.QueryRow(ctx, `
			WITH RECURSIVE ancestors AS (
				SELECT id, parent_id FROM grade_categories WHERE id = $2
				UNION ALL
				SELECT c.id, c.parent_id FROM grade_categories c JOIN ancestors a ON c.id = a.parent_id
			)
			SELECT EXISTS (SELECT 1 FROM ancestors WHERE id = $1)
		`, categoryID, *in.ParentID).Scan(&cyclic); err != nil {
			return nil, err
		}
		if cyclic {
			return nil, ErrCategoryCycle
		}
	}

	return scanCategory(s.pool.QueryRow(ctx, `
		UPDATE grade_categories SET
			name = COALESCE(NULLIF($2, ''), name),
			parent_id = COALESCE(NULLIF($3, '')::uuid, parent_id),
			aggregation = COALESCE($4, aggregation),
			max_grade = COALESCE($5, max_grade),
			weight = COALESCE($6, weight),
			drop_lowest = COALESCE($7, drop_lowest),
			hidden = COALESCE($8, hidden)
		WHERE id = $1
		RETURNING `+categoryColumns,
		categoryID, in.Name, deref(in.ParentID), in.Aggregation, in.MaxGrade, in.Weight, in.DropLowest, in.Hidden))
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// DeleteCategory removes a category; its items and sub-categories move up
// to its parent so nothing graded is lost.
func (s *Service) DeleteCategory(ctx context.Context, categoryID string) error {
	cur, err := scanCategory(s.pool.QueryRow(ctx, `SELECT `+categoryColumns+` FROM grade_categories WHERE id = $1`, categoryID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if cur.IsRoot || cur.ParentID == nil {
		return ErrRootCategory
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE grade_items SET category_id = $2 WHERE category_id = $1`, categoryID, *cur.ParentID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE grade_categories SET parent_id = $2 WHERE parent_id = $1`, categoryID, *cur.ParentID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM grade_categories WHERE id = $1`, categoryID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) ListCategories(ctx context.Context, courseID string) ([]Category, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+categoryColumns+` FROM grade_categories WHERE course_id = $1 ORDER BY is_root DESC, sort_order, created_at
	`, courseID)
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

// --- Items ---

const itemColumns = `id, course_id, category_id, course_module_id, name, min_grade, max_grade, pass_grade, weight, hidden, sort_order, created_at`

func scanItem(row pgx.Row) (*Item, error) {
	var it Item
	err := row.Scan(&it.ID, &it.CourseID, &it.CategoryID, &it.CourseModuleID, &it.Name, &it.MinGrade, &it.MaxGrade, &it.PassGrade, &it.Weight, &it.Hidden, &it.SortOrder, &it.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &it, nil
}

// CreateItem attaches a new grade item to a course. CategoryID may be empty,
// in which case the item lands directly under the course total.
// courseModuleID is nil for a manually-added item, or set when a module
// (assign, quiz, ...) creates the item for one of its instances.
func (s *Service) CreateItem(ctx context.Context, courseID string, courseModuleID *string, in ItemInput) (*Item, error) {
	if in.Name == "" {
		return nil, ErrNameRequired
	}
	minGrade, maxGrade := 0.0, 100.0
	if in.MinGrade != nil {
		minGrade = *in.MinGrade
	}
	if in.MaxGrade != nil {
		maxGrade = *in.MaxGrade
	}
	if maxGrade <= minGrade {
		return nil, ErrInvalidRange
	}

	categoryID := in.CategoryID
	if categoryID == "" {
		root, err := s.rootCategoryID(ctx, courseID)
		if err != nil {
			return nil, err
		}
		categoryID = root
	} else {
		var ok bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM grade_categories WHERE id = $1 AND course_id = $2)`, categoryID, courseID).Scan(&ok); err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrInvalidParent
		}
	}
	hidden := in.Hidden != nil && *in.Hidden

	return scanItem(s.pool.QueryRow(ctx, `
		INSERT INTO grade_items (course_id, category_id, course_module_id, name, min_grade, max_grade, pass_grade, weight, hidden)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING `+itemColumns,
		courseID, categoryID, courseModuleID, in.Name, minGrade, maxGrade, in.PassGrade, in.Weight, hidden))
}

func (s *Service) UpdateItem(ctx context.Context, itemID string, in ItemInput) (*Item, error) {
	cur, err := scanItem(s.pool.QueryRow(ctx, `SELECT `+itemColumns+` FROM grade_items WHERE id = $1`, itemID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	minGrade, maxGrade := cur.MinGrade, cur.MaxGrade
	if in.MinGrade != nil {
		minGrade = *in.MinGrade
	}
	if in.MaxGrade != nil {
		maxGrade = *in.MaxGrade
	}
	if maxGrade <= minGrade {
		return nil, ErrInvalidRange
	}
	if in.CategoryID != "" {
		var ok bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM grade_categories WHERE id = $1 AND course_id = $2)`, in.CategoryID, cur.CourseID).Scan(&ok); err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrInvalidParent
		}
	}

	return scanItem(s.pool.QueryRow(ctx, `
		UPDATE grade_items SET
			name = COALESCE(NULLIF($2, ''), name),
			category_id = COALESCE(NULLIF($3, '')::uuid, category_id),
			min_grade = $4, max_grade = $5,
			pass_grade = COALESCE($6, pass_grade),
			weight = COALESCE($7, weight),
			hidden = COALESCE($8, hidden),
			updated_at = now()
		WHERE id = $1
		RETURNING `+itemColumns,
		itemID, in.Name, in.CategoryID, minGrade, maxGrade, in.PassGrade, in.Weight, in.Hidden))
}

func (s *Service) DeleteItem(ctx context.Context, itemID string) error {
	var moduleID *string
	err := s.pool.QueryRow(ctx, `SELECT course_module_id FROM grade_items WHERE id = $1`, itemID).Scan(&moduleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if moduleID != nil {
		return ErrModuleItem
	}
	_, err = s.pool.Exec(ctx, `DELETE FROM grade_items WHERE id = $1`, itemID)
	return err
}

func (s *Service) ListItems(ctx context.Context, courseID string, includeHidden bool) ([]Item, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+itemColumns+` FROM grade_items
		WHERE course_id = $1 AND ($2 OR NOT hidden) ORDER BY sort_order, created_at
	`, courseID, includeHidden)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *it)
	}
	return items, rows.Err()
}

// --- Grades ---

const gradeColumns = `id, grade_item_id, user_id, grade, feedback, graded_by, hidden, locked, overridden, excluded, created_at, updated_at`

func scanGrade(row pgx.Row) (*Grade, error) {
	var g Grade
	err := row.Scan(&g.ID, &g.GradeItemID, &g.UserID, &g.Grade, &g.Feedback, &g.GradedBy, &g.Hidden, &g.Locked, &g.Overridden, &g.Excluded, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// SetGrade is a manual edit by a grader: it can change the grade and
// feedback and flip the hidden, locked and excluded flags. Editing the
// grade of an activity-created item marks it overridden, so later pushes
// from the activity leave it alone. A locked grade refuses edits until it
// is unlocked (which may happen in the same request).
func (s *Service) SetGrade(ctx context.Context, itemID, userID, graderID string, in GradeInput) (*Grade, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	item, err := scanItem(tx.QueryRow(ctx, `SELECT `+itemColumns+` FROM grade_items WHERE id = $1`, itemID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	cur, err := scanGrade(tx.QueryRow(ctx, `SELECT `+gradeColumns+` FROM grade_grades WHERE grade_item_id = $1 AND user_id = $2 FOR UPDATE`, itemID, userID))
	exists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if !exists {
		cur = &Grade{}
	}

	unlocking := in.Locked != nil && !*in.Locked
	if cur.Locked && !unlocking && (in.Grade.Set || in.Feedback != nil) {
		return nil, ErrGradeLocked
	}

	next := *cur
	changed := false
	if in.Grade.Set {
		if v := in.Grade.Value; v != nil && (*v < item.MinGrade || *v > item.MaxGrade) {
			return nil, ErrInvalidGrade
		}
		next.Grade = in.Grade.Value
		changed = !sameFloat(cur.Grade, next.Grade)
		if item.CourseModuleID != nil {
			next.Overridden = true
		}
	}
	if in.Feedback != nil {
		next.Feedback = *in.Feedback
	}
	if in.Hidden != nil {
		next.Hidden = *in.Hidden
	}
	if in.Locked != nil {
		next.Locked = *in.Locked
	}
	if in.Excluded != nil {
		next.Excluded = *in.Excluded
	}

	saved, err := scanGrade(tx.QueryRow(ctx, `
		INSERT INTO grade_grades (grade_item_id, user_id, grade, feedback, graded_by, hidden, locked, overridden, excluded, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
		ON CONFLICT (grade_item_id, user_id) DO UPDATE SET
			grade = EXCLUDED.grade, feedback = EXCLUDED.feedback, graded_by = EXCLUDED.graded_by,
			hidden = EXCLUDED.hidden, locked = EXCLUDED.locked, overridden = EXCLUDED.overridden,
			excluded = EXCLUDED.excluded, updated_at = now()
		RETURNING `+gradeColumns,
		itemID, userID, next.Grade, next.Feedback, graderID, next.Hidden, next.Locked, next.Overridden, next.Excluded))
	if err != nil {
		return nil, err
	}

	if changed || (in.Feedback != nil && *in.Feedback != cur.Feedback) {
		if _, err := tx.Exec(ctx, `
			INSERT INTO grade_history (grade_item_id, user_id, old_grade, new_grade, feedback, changed_by)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, itemID, userID, cur.Grade, next.Grade, next.Feedback, graderID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	s.events.Dispatch(ctx, eventbus.Event{
		Name:   "grade.updated",
		UserID: &graderID,
		Data:   map[string]any{"grade_item_id": itemID, "user_id": userID},
	})
	return saved, nil
}

// PushGrade is how an activity module reports a grade for its item. It
// skips grades a teacher locked or overrode by hand, and never marks the
// grade as overridden itself.
func (s *Service) PushGrade(ctx context.Context, itemID, userID string, grade *float64, feedback string) error {
	item, err := scanItem(s.pool.QueryRow(ctx, `SELECT `+itemColumns+` FROM grade_items WHERE id = $1`, itemID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if grade != nil && (*grade < item.MinGrade || *grade > item.MaxGrade) {
		return ErrInvalidGrade
	}

	var oldGrade *float64
	var skip bool
	err = s.pool.QueryRow(ctx, `
		SELECT grade, locked OR overridden FROM grade_grades WHERE grade_item_id = $1 AND user_id = $2
	`, itemID, userID).Scan(&oldGrade, &skip)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if skip {
		return nil
	}

	if _, err := s.pool.Exec(ctx, `
		INSERT INTO grade_grades (grade_item_id, user_id, grade, feedback, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (grade_item_id, user_id) DO UPDATE SET grade = EXCLUDED.grade, feedback = EXCLUDED.feedback, updated_at = now()
	`, itemID, userID, grade, feedback); err != nil {
		return err
	}
	if !sameFloat(oldGrade, grade) {
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO grade_history (grade_item_id, user_id, old_grade, new_grade, feedback) VALUES ($1, $2, $3, $4, $5)
		`, itemID, userID, oldGrade, grade, feedback); err != nil {
			return err
		}
	}
	s.events.Dispatch(ctx, eventbus.Event{
		Name: "grade.updated",
		Data: map[string]any{"grade_item_id": itemID, "user_id": userID},
	})
	return nil
}

func sameFloat(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func (s *Service) ListGrades(ctx context.Context, itemID string) ([]Grade, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+gradeColumns+` FROM grade_grades WHERE grade_item_id = $1 ORDER BY created_at ASC
	`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	grades := []Grade{}
	for rows.Next() {
		g, err := scanGrade(rows)
		if err != nil {
			return nil, err
		}
		grades = append(grades, *g)
	}
	return grades, rows.Err()
}

func (s *Service) History(ctx context.Context, itemID, userID string) ([]HistoryEntry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, old_grade, new_grade, feedback, changed_by, created_at FROM grade_history
		WHERE grade_item_id = $1 AND user_id = $2 ORDER BY created_at DESC
	`, itemID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []HistoryEntry{}
	for rows.Next() {
		var h HistoryEntry
		if err := rows.Scan(&h.ID, &h.OldGrade, &h.NewGrade, &h.Feedback, &h.ChangedBy, &h.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// --- Reports ---

// gradesByUser loads every stored grade in the course, keyed by user id
// then item id.
func (s *Service) gradesByUser(ctx context.Context, courseID string) (map[string]map[string]Grade, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT gg.id, gg.grade_item_id, gg.user_id, gg.grade, gg.feedback, gg.graded_by, gg.hidden, gg.locked, gg.overridden, gg.excluded, gg.created_at, gg.updated_at
		FROM grade_grades gg JOIN grade_items gi ON gi.id = gg.grade_item_id WHERE gi.course_id = $1
	`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]map[string]Grade{}
	for rows.Next() {
		g, err := scanGrade(rows)
		if err != nil {
			return nil, err
		}
		if out[g.UserID] == nil {
			out[g.UserID] = map[string]Grade{}
		}
		out[g.UserID][g.GradeItemID] = *g
	}
	return out, rows.Err()
}

// node is a category in the tree being aggregated.
type node struct {
	cat      Category
	children []*node
	items    []Item
}

func buildTree(cats []Category, items []Item) *node {
	nodes := map[string]*node{}
	for _, c := range cats {
		nodes[c.ID] = &node{cat: c}
	}
	var root *node
	for _, c := range cats {
		n := nodes[c.ID]
		if c.IsRoot || c.ParentID == nil {
			root = n
			continue
		}
		if parent, ok := nodes[*c.ParentID]; ok {
			parent.children = append(parent.children, n)
		}
	}
	for _, it := range items {
		if n, ok := nodes[it.CategoryID]; ok {
			n.items = append(n.items, it)
		}
	}
	return root
}

func percentageOf(grade *float64, min, max float64) *float64 {
	if grade == nil || max == min {
		return nil
	}
	p := round2((*grade - min) / (max - min) * 100)
	return &p
}

func makeTotal(grade *float64, max float64) Total {
	t := Total{Grade: grade, Max: max}
	if grade != nil {
		g := round2(*grade)
		t.Grade = &g
		t.Percentage = percentageOf(&g, 0, max)
		if t.Percentage != nil {
			t.Letter = letterFor(*t.Percentage)
		}
	}
	return t
}

// compute aggregates a category from the bottom up for one user's grades,
// recording each category's total in out. Viewers who may not see hidden
// items get totals that leave them out.
func (n *node) compute(grades map[string]Grade, seeHidden bool, out map[string]Total) (float64, float64, bool) {
	var entries []entry
	for _, it := range n.items {
		if it.Hidden && !seeHidden {
			continue
		}
		g, ok := grades[it.ID]
		if !ok || g.Grade == nil || g.Excluded || (g.Hidden && !seeHidden) {
			continue
		}
		w := 1.0
		if it.Weight != nil {
			w = *it.Weight
		}
		entries = append(entries, entry{grade: *g.Grade, min: it.MinGrade, max: it.MaxGrade, weight: w})
	}
	for _, child := range n.children {
		if child.cat.Hidden && !seeHidden {
			continue
		}
		grade, max, ok := child.compute(grades, seeHidden, out)
		if !ok {
			continue
		}
		w := 1.0
		if child.cat.Weight != nil {
			w = *child.cat.Weight
		}
		entries = append(entries, entry{grade: grade, min: 0, max: max, weight: w})
	}

	grade, max, ok := aggregate(n.cat.Aggregation, n.cat.DropLowest, entries, n.cat.MaxGrade)
	if ok {
		out[n.cat.ID] = makeTotal(&grade, max)
	} else {
		out[n.cat.ID] = makeTotal(nil, max)
	}
	return grade, max, ok
}

type structure struct {
	cats  []Category
	items []Item
	tree  *node
}

func (s *Service) loadStructure(ctx context.Context, courseID string, includeHidden bool) (*structure, error) {
	cats, err := s.ListCategories(ctx, courseID)
	if err != nil {
		return nil, err
	}
	items, err := s.ListItems(ctx, courseID, true)
	if err != nil {
		return nil, err
	}
	tree := buildTree(cats, items)
	if tree == nil {
		return nil, fmt.Errorf("course %s has no grade category root", courseID)
	}

	visible := items
	if !includeHidden {
		visible = visible[:0:0]
		for _, it := range items {
			if !it.Hidden {
				visible = append(visible, it)
			}
		}
	}
	return &structure{cats: cats, items: visible, tree: tree}, nil
}

func (st *structure) userReport(grades map[string]Grade, seeHidden bool) UserReport {
	totals := map[string]Total{}
	st.tree.compute(grades, seeHidden, totals)

	results := make([]ItemResult, 0, len(st.items))
	for _, it := range st.items {
		g := grades[it.ID]
		if g.Hidden && !seeHidden {
			results = append(results, ItemResult{Item: it, Hidden: true})
			continue
		}
		r := ItemResult{
			Item: it, Grade: g.Grade, Feedback: g.Feedback, Hidden: g.Hidden,
			Locked: g.Locked, Overridden: g.Overridden, Excluded: g.Excluded,
		}
		r.Percentage = percentageOf(g.Grade, it.MinGrade, it.MaxGrade)
		if r.Percentage != nil {
			r.Letter = letterFor(*r.Percentage)
		}
		if g.Grade != nil && it.PassGrade != nil {
			passed := *g.Grade >= *it.PassGrade
			r.Passed = &passed
		}
		results = append(results, r)
	}

	return UserReport{Items: results, Categories: totals, CourseTotal: totals[st.tree.cat.ID]}
}

// UserReport builds one user's view of the course gradebook. seeHidden is
// true when the viewer may see hidden items and grades (teachers).
func (s *Service) UserReport(ctx context.Context, courseID, userID string, seeHidden bool) (*UserReport, error) {
	st, err := s.loadStructure(ctx, courseID, seeHidden)
	if err != nil {
		return nil, err
	}
	all, err := s.gradesByUser(ctx, courseID)
	if err != nil {
		return nil, err
	}
	report := st.userReport(all[userID], seeHidden)
	report.UserID = userID
	if err := s.pool.QueryRow(ctx, `SELECT name, email FROM users WHERE id = $1`, userID).Scan(&report.Name, &report.Email); err != nil {
		return nil, err
	}
	return &report, nil
}

// GraderReport builds the teacher overview: every student in the course
// with their grades and totals.
func (s *Service) GraderReport(ctx context.Context, courseID string) (*GraderReport, error) {
	st, err := s.loadStructure(ctx, courseID, true)
	if err != nil {
		return nil, err
	}
	all, err := s.gradesByUser(ctx, courseID)
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT u.id, u.name, u.email
		FROM enrollments e
		JOIN users u ON u.id = e.user_id
		JOIN role_assignments ra ON ra.user_id = u.id
		JOIN roles r ON r.id = ra.role_id AND r.name = 'student'
		JOIN contexts cx ON cx.id = ra.context_id AND cx.level = 'course' AND cx.instance_id = e.course_id
		WHERE e.course_id = $1
	`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	students := []UserReport{}
	for rows.Next() {
		var id, name, email string
		if err := rows.Scan(&id, &name, &email); err != nil {
			return nil, err
		}
		report := st.userReport(all[id], true)
		report.UserID, report.Name, report.Email = id, name, email
		students = append(students, report)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(students, func(i, j int) bool { return students[i].Name < students[j].Name })

	return &GraderReport{Categories: st.cats, Items: st.items, Students: students}, nil
}
