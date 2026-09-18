package groups

import "time"

type Group struct {
	ID          string    `json:"id"`
	CourseID    string    `json:"course_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	MemberIDs   []string  `json:"member_ids"`
}
