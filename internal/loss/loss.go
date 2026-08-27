package loss

import (
	"image"
	"math"

	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

// Loss compares two pixmaps. got is Render(doc). want is the scene (decoded PNG).
// Same Go type; different origin. Lower is better.
type Loss interface {
	Loss(got, want *image.NRGBA) float64
}

// PerCost is the v1 ranking metric: deviate / Cost(doc).
// Extra primitives shrink the number — known; replace later if it inflates trees.
// Cost 0: 0 if deviate == 0, otherwise +Inf.
func PerCost(deviate float64, complexity int) float64 {
	if math.IsInf(deviate, 0) || math.IsNaN(deviate) {
		return deviate
	}
	if complexity <= 0 {
		if deviate == 0 {
			return 0
		}
		return math.Inf(1)
	}
	return deviate / float64(complexity)
}

// Of renders doc and returns PerCost(Pixels.Loss(got, want), Cost(doc)).
func Of(doc svg.Document, want *image.NRGBA) (float64, error) {
	got, err := render.Render(doc)
	if err != nil {
		return math.Inf(1), err
	}
	return PerCost((Pixels{}).Loss(got, want), svg.CostDocument(doc)), nil
}
