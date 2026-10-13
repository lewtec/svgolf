package stack

import (
	"math"
	"testing"

	"github.com/lewtec/svgolf/pkg/svg"
	"github.com/stretchr/testify/require"
)

func TestStraightStairDoesNotBow(t *testing.T) {
	samples := make([][2]float64, 0, 40)
	for i := 0; i < 40; i++ {
		samples = append(samples, [2]float64{float64(i * 3), float64(i % 2)})
	}
	commands := pieceCommands(fitPieces(samples))
	require.NotEmpty(t, commands)
	for _, piece := range fitPieces(samples) {
		for _, command := range piece.commands {
			if command.Kind != svg.CmdCubic {
				continue
			}
			require.LessOrEqual(t, pieceBulge(piece), chordGap(piece.samples))
		}
	}
	// A two-edge corner is singular. It stays the original lines.
	corner := [][2]float64{{0, 0}, {5, 0}, {5, 1}}
	require.True(t, sameLineChain(pieceCommands(fitPieces(corner)), corner))
}

func TestQuarterCircleBeatsItsChord(t *testing.T) {
	const count = 64
	samples := make([][2]float64, count)
	for i := range samples {
		angle := float64(i) / float64(count-1) * math.Pi / 2
		samples[i] = [2]float64{400 * math.Cos(angle), 400 * math.Sin(angle)}
	}
	pieces := fitPieces(samples)
	commands := pieceCommands(pieces)
	require.Len(t, commands, 1)
	require.Equal(t, svg.CmdCubic, commands[0].Kind)
	require.LessOrEqual(t, pieceBulge(pieces[0]), chordGap(samples))
}

func TestRefineSplitsTheCubicThatMisses(t *testing.T) {
	const count = 64
	samples := make([][2]float64, count)
	for i := range samples {
		angle := float64(i) / float64(count-1) * math.Pi / 2
		samples[i] = [2]float64{400 * math.Cos(angle), 400 * math.Sin(angle)}
	}
	pieces := fitPieces(samples)
	refined, ok := refineFit(pieces)
	require.True(t, ok)
	commands := pieceCommands(refined)
	require.Greater(t, len(commands), len(pieceCommands(pieces)))
	for _, piece := range refined {
		for _, command := range piece.commands {
			if command.Kind != svg.CmdCubic {
				continue
			}
			require.LessOrEqual(t, pieceBulge(piece), chordGap(piece.samples))
		}
	}
}

func TestReplaceSpanKeepsTheJoint(t *testing.T) {
	ring := polylineRing([][2]float64{{0, 0}, {10, 0}, {10, 10}, {0, 10}})
	commands := []svg.PathCmd{lineCommand([2]float64{5, 1}), lineCommand([2]float64{10, 10})}
	next := ring.replaceSpan(0, 2, commands)
	require.Equal(t, len(next.edges), len(next.verts))
	require.Equal(t, [2]float64{5, 1}, next.verts[1])
	require.Equal(t, [2]float64{10, 10}, next.verts[2])
}

func TestPullCubicJointMovesTowardLeftover(t *testing.T) {
	left, right, ok := pullCubicJoint([2]float64{0, 0}, [2]float64{3, 4}, [2]float64{7, 4}, [2]float64{10, 0}, [2]float64{5, 10})
	require.True(t, ok)
	_, curve := splitBezier([2]float64{0, 0}, [2]float64{3, 4}, [2]float64{7, 4}, [2]float64{10, 0})
	mid := curve[0]
	require.InDelta(t, (mid[0]+5)/2, left.X, 1e-9)
	require.InDelta(t, (mid[1]+10)/2, left.Y, 1e-9)
	require.Equal(t, 10.0, right.X)
	require.Equal(t, 0.0, right.Y)
}
