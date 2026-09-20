package course

import "time"

type Course struct {
	ID          string     `json:"id"`
	OwnerID     string     `json:"owner_id"`
	CategoryID  *string    `json:"category_id"`
	Title       string     `json:"title"`
	ShortName   string     `json:"short_name"`
	IDNumber    string     `json:"id_number"`
	Description string     `json:"description"`
	Format      string     `json:"format"`
	StartDate   *time.Time `json:"start_date"`
	EndDate     *time.Time `json:"end_date"`
	Visible     bool       `json:"visible"`
	SelfEnrol   bool       `json:"self_enrol"`
	RequiresKey bool       `json:"requires_key"`
	CreatedAt   time.Time  `json:"created_at"`
}

// CourseInput carries the editable course fields for create and update.
type CourseInput struct {
	Title       string     `json:"title"`
	ShortName   string     `json:"short_name"`
	IDNumber    string     `json:"id_number"`
	Description string     `json:"description"`
	CategoryID  *string    `json:"category_id"`
	Format      string     `json:"format"`
	StartDate   *time.Time `json:"start_date"`
	EndDate     *time.Time `json:"end_date"`
	Visible     *bool      `json:"visible"`
}

type Category struct {
	ID          string    `json:"id"`
	ParentID    *string   `json:"parent_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IDNumber    string    `json:"id_number"`
	Visible     bool      `json:"visible"`
	IsDefault   bool      `json:"is_default"`
	SortOrder   int       `json:"sort_order"`
	CourseCount int       `json:"course_count"`
	CreatedAt   time.Time `json:"created_at"`
}

type CategoryInput struct {
	Name        string  `json:"name"`
	ParentID    *string `json:"parent_id"`
	Description string  `json:"description"`
	IDNumber    string  `json:"id_number"`
	Visible     *bool   `json:"visible"`
	SortOrder   *int    `json:"sort_order"`
}

type Section struct {
	ID        string    `json:"id"`
	CourseID  string    `json:"course_id"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary"`
	Position  int       `json:"position"`
	Visible   bool      `json:"visible"`
	CreatedAt time.Time `json:"created_at"`
}

type SectionInput struct {
	Title    *string `json:"title"`
	Summary  *string `json:"summary"`
	Visible  *bool   `json:"visible"`
	Position *int    `json:"position"`
}

type Participant struct {
	UserID     string     `json:"user_id"`
	Name       string     `json:"name"`
	Email      string     `json:"email"`
	Roles      []string   `json:"roles"`
	Method     string     `json:"method"`
	Status     string     `json:"status"`
	TimeStart  *time.Time `json:"time_start"`
	TimeEnd    *time.Time `json:"time_end"`
	EnrolledAt time.Time  `json:"enrolled_at"`
}

type EnrolInput struct {
	Email     string     `json:"email"`
	UserID    string     `json:"user_id"`
	Role      string     `json:"role"`
	TimeStart *time.Time `json:"time_start"`
	TimeEnd   *time.Time `json:"time_end"`
}

type EnrolmentUpdate struct {
	Status    *string    `json:"status"`
	TimeStart *time.Time `json:"time_start"`
	TimeEnd   *time.Time `json:"time_end"`
	ClearEnd  bool       `json:"clear_end"`
}

// SelfEnrolment is the per-course self enrolment configuration. Enabled
// means new enrolments are accepted.
type SelfEnrolment struct {
	Enabled      bool       `json:"enabled"`
	Key          string     `json:"key"`
	MaxUsers     *int       `json:"max_users"`
	EnrolStart   *time.Time `json:"enrol_start"`
	EnrolEnd     *time.Time `json:"enrol_end"`
	DurationDays *int       `json:"duration_days"`
	Role         string     `json:"role"`
}
