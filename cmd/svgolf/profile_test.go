package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServerMountsProfiles(t *testing.T) {
	mux := http.NewServeMux()
	mountProfiles(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "goroutine")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine?debug=1", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "goroutine ")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debug/pprof/cmdline", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotZero(t, rec.Body.Len())
}

func TestStartProfilesWrites(t *testing.T) {
	dir := t.TempDir()
	stop, err := startProfiles(dir)
	require.NoError(t, err)
	require.NoError(t, stop())
	require.NoError(t, stop())
	for _, name := range []string{
		"cpu.pprof", "trace.out",
		"heap.pprof", "allocs.pprof",
		"goroutine.pprof", "mutex.pprof", "block.pprof",
	} {
		_, err := os.Stat(filepath.Join(dir, name))
		require.NoError(t, err)
	}
}
