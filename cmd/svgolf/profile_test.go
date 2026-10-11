package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerMountsProfiles(t *testing.T) {
	mux := http.NewServeMux()
	mountProfiles(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("index status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "goroutine") {
		t.Fatalf("index=%s", rec.Body.String())
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine?debug=1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("goroutine status=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "goroutine ") {
		t.Fatalf("goroutine=%s", rec.Body.String())
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debug/pprof/cmdline", nil))
	if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
		t.Fatalf("cmdline status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestStartProfilesWrites(t *testing.T) {
	dir := t.TempDir()
	stop, err := startProfiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"cpu.pprof", "trace.out",
		"heap.pprof", "allocs.pprof",
		"goroutine.pprof", "mutex.pprof", "block.pprof",
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}
