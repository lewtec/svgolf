package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerHomeEmptyCache(t *testing.T) {
	dir := t.TempDir()
	s := &server{cache: dir, algo: "stack"}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleHome)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "svgolf") {
		t.Fatalf("body=%s", rec.Body.String())
	}
}

func TestServerJobFromCache(t *testing.T) {
	dir := t.TempDir()
	id := "job1"
	job := filepath.Join(dir, id)
	if err := os.MkdirAll(job, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := jobMeta{ID: id, Status: "done", Search: "stack", Epochs: 2, Operator: "rectangle", Score: 12.5, Scores: []float64{20, 12.5}, Paths: 3, Vertices: 12, PathCounts: []int{2, 3}, VertexCounts: []int{8, 12}}
	b, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(job, "job.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	s := &server{cache: dir, algo: "stack"}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /jobs/{id}", s.handleJob)
	mux.HandleFunc("GET /jobs/{id}/files/{name}", s.handleFile)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/jobs/job1", nil))
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "HSV delta") {
		t.Fatalf("missing debug frames: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `id="loss"`) {
		t.Fatalf("missing loss plot: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "20") || !strings.Contains(rec.Body.String(), "12.5") {
		t.Fatalf("missing score series: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `id="epochs"`) || !strings.Contains(rec.Body.String(), "<table") {
		t.Fatalf("missing epochs table: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "3 paths") || !strings.Contains(rec.Body.String(), "12 vertices") {
		t.Fatalf("missing path and vertex counts: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "[2,3]") || !strings.Contains(rec.Body.String(), "[8,12]") {
		t.Fatalf("missing per-epoch path and vertex series: %s", rec.Body.String())
	}
	if err := os.WriteFile(filepath.Join(job, "want.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/jobs/job1/files/want.png", nil))
	if rec.Code != 200 {
		t.Fatalf("file status=%d", rec.Code)
	}
}

func TestEpochPayloadScores(t *testing.T) {
	p := epochPayload(jobMeta{Epochs: 2, Score: 12.5, Scores: []float64{20, 12.5}, Operator: "rectangle", Paths: 3, Vertices: 12, PathCounts: []int{2, 3}, VertexCounts: []int{8, 12}})
	got, ok := p["scores"].([]float64)
	if !ok || len(got) != 2 || got[0] != 20 || got[1] != 12.5 {
		t.Fatalf("scores=%v", p["scores"])
	}
	if p["n"] != 1 {
		t.Fatalf("n=%v want 1", p["n"])
	}
	if _, ok := p["rounds"]; !ok {
		t.Fatal("missing rounds")
	}
	if p["paths"] != 3 || p["vertices"] != 12 {
		t.Fatalf("paths=%v vertices=%v", p["paths"], p["vertices"])
	}
	pc, ok := p["pathCounts"].([]int)
	if !ok || len(pc) != 2 || pc[0] != 2 || pc[1] != 3 {
		t.Fatalf("pathCounts=%v", p["pathCounts"])
	}
	vc, ok := p["vertexCounts"].([]int)
	if !ok || len(vc) != 2 || vc[0] != 8 || vc[1] != 12 {
		t.Fatalf("vertexCounts=%v", p["vertexCounts"])
	}
}

func TestSanitizeJobName(t *testing.T) {
	if sanitize(`../foo bar.png`) != "foobarpng" {
		t.Fatalf("sanitize=%q", sanitize(`../foo bar.png`))
	}
}
