package stack

import (
	"image"
	"image/color"
	"testing"

	"github.com/lewtec/svgolf/internal/loss"
	"github.com/stretchr/testify/require"
)

func TestTuneRejectsUnknown(t *testing.T) {
	st := &Stack{}
	err := st.Tune(map[string]string{"nope": "1"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown option")
}

func TestTuneKeepsDefaults(t *testing.T) {
	st := &Stack{}
	require.NoError(t, st.Tune(nil))
	require.Equal(t, beamWidth, st.knob.beamWidth())
	require.Equal(t, float64(commandCost), st.knob.cost())
	require.Equal(t, minIsland, st.knob.islandMin())
	require.Equal(t, float64(minErr), st.knob.errMin())
	require.Equal(t, 0, st.knob.epochLimit())
}

func TestCommandCostKnob(t *testing.T) {
	require.True(t, betterObjective(commandCost-1, 1, 0, 2))
	s := &world{knob: knob{ready: true, commandCostSet: true, commandCost: 0}}
	require.False(t, s.better(1, 1, 0, 5))
	require.True(t, s.better(0, 9, 1, 1))
}

func TestBeamWidthKnob(t *testing.T) {
	board := newBeam(snapshot{id: 1, errSum: 0, commands: 1})
	require.False(t, board.offer(snapshot{id: 2, errSum: 10, commands: 1}))
	require.Len(t, board.list(), 1)

	wide := newBeam(snapshot{id: 1, errSum: 0, commands: 1})
	wide.width = 2
	require.True(t, wide.offer(snapshot{id: 2, errSum: 10, commands: 1}))
	require.Len(t, wide.list(), 2)
}

func TestMinimumIslandKnob(t *testing.T) {
	island := make([]pix, minIsland)
	for i := range island {
		island[i] = pix{i, 0}
	}
	left := leftover{island: island, col: color.NRGBA{R: 255, A: 255}}
	open := &world{paths: 0}
	require.True(t, (Outline{world: open, left: left}).Applies())
	shut := &world{paths: 0, knob: knob{ready: true, minIslandSet: true, minIsland: minIsland + 1}}
	require.False(t, (Outline{world: shut, left: left}).Applies())
}

func TestMinimumErrorKnob(t *testing.T) {
	near := color.NRGBA{R: 252, G: 252, B: 252, A: 255}
	delta := loss.ColorAt(near, paper)
	require.Greater(t, delta, 0.0)
	require.LessOrEqual(t, delta, float64(minErr))
	require.True(t, paperLeftover(near))
	s := &world{knob: knob{ready: true, minErrSet: true, minErr: 0}}
	require.False(t, s.paperColor(near))
	require.True(t, s.paperColor(paper))
}

func TestEpochBudgetStops(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 16))
	red := color.NRGBA{R: 255, A: 255}
	for y := 2; y < 14; y++ {
		for x := 2; x < 14; x++ {
			img.SetNRGBA(x, y, red)
		}
		for x := 18; x < 30; x++ {
			img.SetNRGBA(x, y, red)
		}
	}
	st := &Stack{}
	require.NoError(t, st.Tune(map[string]string{"epochs": "1"}))
	n := 0
	for _, err := range st.Search(t.Context(), img) {
		require.NoError(t, err)
		n++
	}
	require.Equal(t, 1, n)
}
