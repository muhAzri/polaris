package gradebook

import (
	"math"
	"sort"
)

// entry is one child (a grade item or a sub-category total) feeding into a
// category's aggregation.
type entry struct {
	grade  float64
	min    float64
	max    float64
	weight float64
}

func (e entry) normalized() float64 {
	if e.max == e.min {
		return 0
	}
	return (e.grade - e.min) / (e.max - e.min)
}

// aggregate combines a category's entries into one grade. For every method
// but "natural" the result is scaled to catMax; "natural" adds raw points
// and reports the summed maximum. Empty grades are never passed in — like
// Moodle's default, only graded children count. dropLowest removes that
// many of the lowest normalised entries first (keeping at least one).
// The boolean is false when there is nothing to aggregate.
func aggregate(method string, dropLowest int, entries []entry, catMax float64) (float64, float64, bool) {
	if len(entries) == 0 {
		return 0, catMax, false
	}

	if dropLowest > 0 && len(entries) > dropLowest {
		sorted := append([]entry(nil), entries...)
		sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].normalized() < sorted[j].normalized() })
		entries = sorted[dropLowest:]
	}

	if method == "natural" {
		var sum, sumMax float64
		for _, e := range entries {
			sum += e.grade - e.min
			sumMax += e.max - e.min
		}
		return sum, sumMax, true
	}

	norms := make([]float64, len(entries))
	for i, e := range entries {
		norms[i] = e.normalized()
	}

	var n float64
	switch method {
	case "weighted":
		var wsum, total float64
		for i, e := range entries {
			wsum += e.weight
			total += e.weight * norms[i]
		}
		if wsum == 0 {
			n = 0
		} else {
			n = total / wsum
		}
	case "min":
		n = norms[0]
		for _, v := range norms {
			n = math.Min(n, v)
		}
	case "max":
		n = norms[0]
		for _, v := range norms {
			n = math.Max(n, v)
		}
	case "median":
		sorted := append([]float64(nil), norms...)
		sort.Float64s(sorted)
		mid := len(sorted) / 2
		if len(sorted)%2 == 1 {
			n = sorted[mid]
		} else {
			n = (sorted[mid-1] + sorted[mid]) / 2
		}
	case "mode":
		counts := map[int64]int{}
		best, bestCount := norms[0], 0
		for _, v := range norms {
			key := int64(math.Round(v * 100000))
			counts[key]++
			if counts[key] > bestCount || (counts[key] == bestCount && v > best) {
				best, bestCount = v, counts[key]
			}
		}
		n = best
	default: // "mean"
		var sum float64
		for _, v := range norms {
			sum += v
		}
		n = sum / float64(len(norms))
	}
	return n * catMax, catMax, true
}

// letterBoundaries are Moodle's default letter grade cut-offs.
var letterBoundaries = []struct {
	min    float64
	letter string
}{
	{93, "A"}, {90, "A-"}, {87, "B+"}, {83, "B"}, {80, "B-"},
	{77, "C+"}, {73, "C"}, {70, "C-"}, {67, "D+"}, {60, "D"}, {0, "F"},
}

func letterFor(percentage float64) string {
	for _, b := range letterBoundaries {
		if percentage >= b.min {
			return b.letter
		}
	}
	return "F"
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
