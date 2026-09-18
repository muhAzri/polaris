package calendar

import "time"

type Event struct {
	ID          string     `json:"id"`
	EventType   string     `json:"event_type"`
	CourseID    *string    `json:"course_id"`
	UserID      *string    `json:"user_id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	StartAt     time.Time  `json:"start_at"`
	EndAt       *time.Time `json:"end_at"`
	CreatedAt   time.Time  `json:"created_at"`
}
