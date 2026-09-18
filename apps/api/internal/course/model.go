package course

import "time"

type Course struct {
	ID          string    `json:"id"`
	OwnerID     string    `json:"owner_id"`
	CategoryID  *string   `json:"category_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type Section struct {
	ID        string    `json:"id"`
	CourseID  string    `json:"course_id"`
	Title     string    `json:"title"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"created_at"`
}
