package gradebook

import "time"

type Category struct {
	ID          string    `json:"id"`
	CourseID    string    `json:"course_id"`
	Name        string    `json:"name"`
	Aggregation string    `json:"aggregation"`
	CreatedAt   time.Time `json:"created_at"`
}

type Item struct {
	ID             string    `json:"id"`
	CourseID       string    `json:"course_id"`
	CategoryID     string    `json:"category_id"`
	CourseModuleID *string   `json:"course_module_id"`
	Name           string    `json:"name"`
	MaxGrade       float64   `json:"max_grade"`
	Weight         *float64  `json:"weight"`
	CreatedAt      time.Time `json:"created_at"`
}

type Grade struct {
	ID          string    `json:"id"`
	GradeItemID string    `json:"grade_item_id"`
	UserID      string    `json:"user_id"`
	Grade       *float64  `json:"grade"`
	Feedback    string    `json:"feedback"`
	GradedBy    *string   `json:"graded_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ItemGrade pairs a grade item with the requesting user's own grade for it
// (possibly ungraded yet) — the shape a student's "my grades" view needs,
// one row per gradable item in the course regardless of whether they've
// been graded.
type ItemGrade struct {
	Item     Item     `json:"item"`
	Grade    *float64 `json:"grade"`
	Feedback string   `json:"feedback"`
}
