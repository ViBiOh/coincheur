package detect

import (
	"cmp"
	"slices"

	"coincheur/internal/scoring"
)

const (
	boxValues    = 4
	maxOverlap   = 0.5
	maxDetection = 100
)

type Box struct {
	X0 float32
	Y0 float32
	X1 float32
	Y1 float32
}

type Detection struct {
	Card  scoring.Card
	Score float32
	Box   Box
}

// candidate is a box in model input pixels, as centered coordinates.
type candidate struct {
	class   int
	score   float32
	centerX float32
	centerY float32
	width   float32
	height  float32
}

func (c candidate) corners() (left, top, right, bottom float32) {
	return c.centerX - c.width/2, c.centerY - c.height/2, c.centerX + c.width/2, c.centerY + c.height/2
}

// decode reads a YOLOv8 output laid out as (4 box values + classes) rows of anchors values.
func decode(data []float32, anchors, classes int, minScore float32) []candidate {
	var candidates []candidate

	for anchor := range anchors {
		bestClass, bestScore := -1, minScore

		for class := range classes {
			if score := data[(boxValues+class)*anchors+anchor]; score >= bestScore {
				bestClass, bestScore = class, score
			}
		}

		if bestClass < 0 {
			continue
		}

		candidates = append(candidates, candidate{
			class:   bestClass,
			score:   bestScore,
			centerX: data[anchor],
			centerY: data[anchors+anchor],
			width:   data[2*anchors+anchor],
			height:  data[3*anchors+anchor],
		})
	}

	return candidates
}

func overlap(a, b candidate) float32 {
	aLeft, aTop, aRight, aBottom := a.corners()
	bLeft, bTop, bRight, bBottom := b.corners()

	width := min(aRight, bRight) - max(aLeft, bLeft)
	height := min(aBottom, bBottom) - max(aTop, bTop)

	if width <= 0 || height <= 0 {
		return 0
	}

	intersection := width * height

	return intersection / (a.width*a.height + b.width*b.height - intersection)
}

// suppress keeps the best box of each group of overlapping boxes that share a class.
func suppress(candidates []candidate) []candidate {
	slices.SortFunc(candidates, func(a, b candidate) int {
		return cmp.Compare(b.score, a.score)
	})

	kept := make([]candidate, 0, len(candidates))

	for _, current := range candidates {
		duplicate := slices.ContainsFunc(kept, func(other candidate) bool {
			return other.class == current.class && overlap(current, other) > maxOverlap
		})

		if !duplicate {
			kept = append(kept, current)
		}

		if len(kept) == maxDetection {
			break
		}
	}

	return kept
}

func (l letterbox) detections(candidates []candidate, deck []scoring.Card, width, height int) []Detection {
	result := make([]Detection, 0, len(candidates))

	for _, current := range candidates {
		left, top, right, bottom := current.corners()
		x0, y0 := l.unmap(left, top)
		x1, y1 := l.unmap(right, bottom)

		result = append(result, Detection{
			Card:  deck[current.class],
			Score: current.score,
			Box: Box{
				X0: clamp(x0 / float32(width)),
				Y0: clamp(y0 / float32(height)),
				X1: clamp(x1 / float32(width)),
				Y1: clamp(y1 / float32(height)),
			},
		})
	}

	return result
}

func clamp(value float32) float32 {
	return min(max(value, 0), 1)
}
