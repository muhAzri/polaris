package calendar

import "time"

type Event struct {
	ID             string     `json:"id"`
	EventType      string     `json:"event_type"`
	CourseID       *string    `json:"course_id"`
	UserID         *string    `json:"user_id"`
	CourseModuleID *string    `json:"course_module_id"`
	EventKind      string     `json:"event_kind"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	StartAt        time.Time  `json:"start_at"`
	EndAt          *time.Time `json:"end_at"`
	RepeatRule     string     `json:"repeat_rule"`
	RepeatUntil    *time.Time `json:"repeat_until"`
	CreatedAt      time.Time  `json:"created_at"`
}

// EventInput carries the editable fields of an event. On update, nil
// pointers are left unchanged.
type EventInput struct {
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	StartAt     *time.Time `json:"start_at"`
	EndAt       *time.Time `json:"end_at"`
	ClearEnd    bool       `json:"clear_end"`
	RepeatRule  *string    `json:"repeat_rule"`
	RepeatUntil *time.Time `json:"repeat_until"`
	ClearRepeat bool       `json:"clear_repeat_until"`
}
