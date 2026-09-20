package groups

import "time"

type Member struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
}

type Group struct {
	ID          string    `json:"id"`
	CourseID    string    `json:"course_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	MemberIDs   []string  `json:"member_ids"`
	Members     []Member  `json:"members"`
}

type GroupInput struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

type Grouping struct {
	ID          string    `json:"id"`
	CourseID    string    `json:"course_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	GroupIDs    []string  `json:"group_ids"`
	CreatedAt   time.Time `json:"created_at"`
}

// AutoCreateInput asks for the course's students to be split into groups,
// either a fixed number of groups (Count) or a target size per group
// (Size).
type AutoCreateInput struct {
	Count  int    `json:"count"`
	Size   int    `json:"size"`
	Prefix string `json:"prefix"`
}
