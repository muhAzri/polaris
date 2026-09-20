package gradebook

import (
	"math"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestAggregate(t *testing.T) {
	entries := []entry{
		{grade: 80, min: 0, max: 100, weight: 1},
		{grade: 45, min: 0, max: 50, weight: 3},
		{grade: 10, min: 0, max: 100, weight: 1},
	}

	cases := []struct {
		name      string
		method    string
		drop      int
		catMax    float64
		wantGrade float64
		wantMax   float64
	}{
		{"mean scales to category max", "mean", 0, 100, (0.8 + 0.9 + 0.1) / 3 * 100, 100},
		{"weighted uses weights", "weighted", 0, 100, (0.8*1 + 0.9*3 + 0.1*1) / 5 * 100, 100},
		{"natural adds points and max", "natural", 0, 100, 135, 250},
		{"min", "min", 0, 100, 10, 100},
		{"max", "max", 0, 100, 90, 100},
		{"median", "median", 0, 100, 80, 100},
		{"drop lowest before mean", "mean", 1, 100, (0.8 + 0.9) / 2 * 100, 100},
		{"mean rescales to a 10 point category", "mean", 0, 10, (0.8 + 0.9 + 0.1) / 3 * 10, 10},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			grade, max, ok := aggregate(c.method, c.drop, entries, c.catMax)
			if !ok {
				t.Fatal("expected a result")
			}
			if !near(grade, c.wantGrade) || !near(max, c.wantMax) {
				t.Fatalf("got %v/%v, want %v/%v", grade, max, c.wantGrade, c.wantMax)
			}
		})
	}
}

func TestAggregateModeAndEmpty(t *testing.T) {
	entries := []entry{
		{grade: 50, min: 0, max: 100, weight: 1},
		{grade: 50, min: 0, max: 100, weight: 1},
		{grade: 90, min: 0, max: 100, weight: 1},
	}
	grade, _, _ := aggregate("mode", 0, entries, 100)
	if !near(grade, 50) {
		t.Fatalf("mode = %v, want 50", grade)
	}
	if _, _, ok := aggregate("mean", 0, nil, 100); ok {
		t.Fatal("no entries should report no result")
	}
}

func TestDropLowestKeepsOneEntry(t *testing.T) {
	entries := []entry{{grade: 40, min: 0, max: 100, weight: 1}}
	grade, _, _ := aggregate("mean", 3, entries, 100)
	if !near(grade, 40) {
		t.Fatalf("got %v, want the only entry kept", grade)
	}
}

func TestLetterFor(t *testing.T) {
	for pct, want := range map[float64]string{100: "A", 93: "A", 89.9: "B+", 60: "D", 59.9: "F", 0: "F"} {
		if got := letterFor(pct); got != want {
			t.Errorf("letterFor(%v) = %s, want %s", pct, got, want)
		}
	}
}
