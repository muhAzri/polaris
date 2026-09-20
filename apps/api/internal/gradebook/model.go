package gradebook

import (
	"encoding/json"
	"time"
)

type Category struct {
	ID          string    `json:"id"`
	CourseID    string    `json:"course_id"`
	ParentID    *string   `json:"parent_id"`
	IsRoot      bool      `json:"is_root"`
	Name        string    `json:"name"`
	Aggregation string    `json:"aggregation"`
	MaxGrade    float64   `json:"max_grade"`
	Weight      *float64  `json:"weight"`
	DropLowest  int       `json:"drop_lowest"`
	Hidden      bool      `json:"hidden"`
	SortOrder   int       `json:"sort_order"`
	CreatedAt   time.Time `json:"created_at"`
}

type CategoryInput struct {
	Name        string   `json:"name"`
	ParentID    *string  `json:"parent_id"`
	Aggregation *string  `json:"aggregation"`
	MaxGrade    *float64 `json:"max_grade"`
	Weight      *float64 `json:"weight"`
	DropLowest  *int     `json:"drop_lowest"`
	Hidden      *bool    `json:"hidden"`
}

type Item struct {
	ID             string    `json:"id"`
	CourseID       string    `json:"course_id"`
	CategoryID     string    `json:"category_id"`
	CourseModuleID *string   `json:"course_module_id"`
	Name           string    `json:"name"`
	MinGrade       float64   `json:"min_grade"`
	MaxGrade       float64   `json:"max_grade"`
	PassGrade      *float64  `json:"pass_grade"`
	Weight         *float64  `json:"weight"`
	Hidden         bool      `json:"hidden"`
	SortOrder      int       `json:"sort_order"`
	CreatedAt      time.Time `json:"created_at"`
}

type ItemInput struct {
	CategoryID string   `json:"category_id"`
	Name       string   `json:"name"`
	MinGrade   *float64 `json:"min_grade"`
	MaxGrade   *float64 `json:"max_grade"`
	PassGrade  *float64 `json:"pass_grade"`
	Weight     *float64 `json:"weight"`
	Hidden     *bool    `json:"hidden"`
}

type Grade struct {
	ID          string    `json:"id"`
	GradeItemID string    `json:"grade_item_id"`
	UserID      string    `json:"user_id"`
	Grade       *float64  `json:"grade"`
	Feedback    string    `json:"feedback"`
	GradedBy    *string   `json:"graded_by"`
	Hidden      bool      `json:"hidden"`
	Locked      bool      `json:"locked"`
	Overridden  bool      `json:"overridden"`
	Excluded    bool      `json:"excluded"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// OptFloat distinguishes "field absent" from "field set to null", so a
// request can clear a grade explicitly without an unrelated edit (locking
// a grade, say) wiping it.
type OptFloat struct {
	Set   bool
	Value *float64
}

func (o *OptFloat) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Value = nil
		return nil
	}
	var v float64
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

// GradeInput is a manual grade edit. Absent fields are left unchanged.
type GradeInput struct {
	Grade    OptFloat `json:"grade"`
	Feedback *string  `json:"feedback"`
	Hidden   *bool    `json:"hidden"`
	Locked   *bool    `json:"locked"`
	Excluded *bool    `json:"excluded"`
}

type HistoryEntry struct {
	ID        string    `json:"id"`
	OldGrade  *float64  `json:"old_grade"`
	NewGrade  *float64  `json:"new_grade"`
	Feedback  string    `json:"feedback"`
	ChangedBy *string   `json:"changed_by"`
	CreatedAt time.Time `json:"created_at"`
}

// Total is an aggregated grade for a category or the whole course.
type Total struct {
	Grade      *float64 `json:"grade"`
	Max        float64  `json:"max"`
	Percentage *float64 `json:"percentage"`
	Letter     string   `json:"letter"`
}

// ItemResult is one grade item as a single user sees it.
type ItemResult struct {
	Item       Item     `json:"item"`
	Grade      *float64 `json:"grade"`
	Feedback   string   `json:"feedback"`
	Percentage *float64 `json:"percentage"`
	Letter     string   `json:"letter"`
	Passed     *bool    `json:"passed"`
	Hidden     bool     `json:"hidden"`
	Locked     bool     `json:"locked"`
	Overridden bool     `json:"overridden"`
	Excluded   bool     `json:"excluded"`
}

// UserReport is one user's view of the gradebook: every visible item with
// their grade, plus the total of every category and of the course.
type UserReport struct {
	UserID      string           `json:"user_id"`
	Name        string           `json:"name"`
	Email       string           `json:"email"`
	Items       []ItemResult     `json:"items"`
	Categories  map[string]Total `json:"categories"`
	CourseTotal Total            `json:"course_total"`
}

// GraderReport is the teacher's overview: the structure once, then one
// UserReport per student.
type GraderReport struct {
	Categories []Category   `json:"categories"`
	Items      []Item       `json:"items"`
	Students   []UserReport `json:"students"`
}
