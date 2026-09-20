package calendar

import (
	"testing"
	"time"
)

func TestExpandRepeating(t *testing.T) {
	start := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	until := time.Date(2026, 3, 30, 0, 0, 0, 0, time.UTC)
	weekly := Event{ID: "a", StartAt: start, EndAt: &end, RepeatRule: "weekly", RepeatUntil: &until}

	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	got := expand([]Event{weekly}, from, to)

	// Mar 2, 9, 16, 23 — Mar 30 is the day of repeat_until at 09:00, after 00:00.
	if len(got) != 4 {
		t.Fatalf("got %d occurrences, want 4", len(got))
	}
	if !got[1].StartAt.Equal(start.AddDate(0, 0, 7)) || !got[1].EndAt.Equal(end.AddDate(0, 0, 7)) {
		t.Fatalf("second occurrence moved wrongly: %v - %v", got[1].StartAt, got[1].EndAt)
	}
}

func TestExpandSkipsOccurrencesBeforeRange(t *testing.T) {
	start := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	daily := Event{ID: "b", StartAt: start, RepeatRule: "daily"}
	from := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 12, 23, 0, 0, 0, time.UTC)
	got := expand([]Event{daily}, from, to)
	if len(got) != 3 || got[0].StartAt.Day() != 10 {
		t.Fatalf("got %d occurrences starting %v", len(got), got[0].StartAt)
	}
}

func TestExpandKeepsSingleEvents(t *testing.T) {
	e := Event{ID: "c", StartAt: time.Now(), RepeatRule: "none"}
	if got := expand([]Event{e}, time.Now().Add(-time.Hour), time.Now().Add(time.Hour)); len(got) != 1 {
		t.Fatalf("got %d events", len(got))
	}
}

func TestICSEscape(t *testing.T) {
	if got := icsEscape("a,b;c\nd"); got != `a\,b\;c\nd` {
		t.Fatalf("got %q", got)
	}
}
