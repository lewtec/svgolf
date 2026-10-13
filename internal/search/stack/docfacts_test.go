package stack

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPathBucketsAreComputedOncePerDocument(t *testing.T) {
	s := &world{
		w:     2,
		h:     2,
		paths: 2,
		owner: []uint16{1, 1, 2, 2},
	}
	first := s.snap()
	copied := first
	a := first.facts.buckets()
	b := copied.facts.buckets()
	require.NotEmpty(t, a)
	require.NotEmpty(t, a[0])
	require.Equal(t, &a[0][0], &b[0][0])

	fork := first.fork(nil, nil, s.w, s.h)
	c := fork.pathBuckets()
	require.Equal(t, &a[0][0], &c[0][0])
}

func TestLeftoverOutlineIsTracedOnce(t *testing.T) {
	island := []pix{{0, 0}, {1, 0}, {1, 1}, {0, 1}}
	left := leftover{island: island, glow: island}.withTraces()
	copied := left
	a := left.glowOutline()
	b := copied.islandOutline()
	require.NotEmpty(t, a)
	require.Equal(t, &a[0], &b[0])
	require.Equal(t, a, left.glowOutline())
}
