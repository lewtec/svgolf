package main

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lewtec/lewkit/x/event"
	"github.com/lewtec/lewkit/x/taskgroup"
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

func TestNewJobIDUnique(t *testing.T) {
	seen := map[string]struct{}{}
	for i := 0; i < 1000; i++ {
		id := newJobID("logo")
		if _, ok := seen[id]; ok {
			t.Fatalf("duplicate id %s", id)
		}
		seen[id] = struct{}{}
	}
}

func TestWriteMetaRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := jobMeta{ID: "a", Status: "running", Score: 1.5, Epochs: 3}
	if err := writeMeta(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := readMeta(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || got.Status != want.Status || got.Score != want.Score || got.Epochs != want.Epochs {
		t.Fatalf("meta=%+v", got)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	path := filepath.Join(dir, "last.png")
	if err := writePNG(path, img); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	decoded, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Dx() != 2 {
		t.Fatalf("png bounds %v", decoded.Bounds())
	}
}

func TestEventsSnapshotDone(t *testing.T) {
	dir := t.TempDir()
	id := "job1"
	if err := os.MkdirAll(filepath.Join(dir, id), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeMeta(filepath.Join(dir, id), jobMeta{ID: id, Status: "done", Epochs: 1, Score: 1}); err != nil {
		t.Fatal(err)
	}
	s := &server{cache: dir}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/jobs/job1/events", nil)
	req.SetPathValue("id", id)
	s.handleEvents(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, "event: epoch") || !strings.Contains(body, "event: done") {
		t.Fatalf("body=%s", body)
	}
	if !strings.Contains(body, `"id":"job1"`) {
		t.Fatalf("missing id: %s", body)
	}
}

func TestStreamKeepsDoneAfterBurst(t *testing.T) {
	bus := event.New[jobEvent]()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	sub := bus.Subscribe(ctx)
	rec := httptest.NewRecorder()
	finished := make(chan struct{})
	go func() {
		streamEvents(ctx, rec, rec, sub, "j")
		close(finished)
	}()
	for i := 0; i < 10; i++ {
		bus.Publish(jobEvent{ID: "j", Name: "epoch", Meta: jobMeta{ID: "j", Status: "running", Epochs: i + 1, Scores: []float64{float64(i)}}})
	}
	bus.Publish(jobEvent{ID: "j", Name: "done", Meta: jobMeta{ID: "j", Status: "done", Epochs: 10}})
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not finish on done")
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: done") {
		t.Fatalf("body=%s", body)
	}
	if !strings.Contains(body, "event: epoch") {
		t.Fatalf("dropped the last epoch: %s", body)
	}
}

func TestPublishReachesJobAndFeed(t *testing.T) {
	s := &server{}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	job := s.jobBus("a").Subscribe(ctx)
	all := s.feed().Subscribe(ctx)
	s.publish("a", "epoch", jobMeta{ID: "a", Status: "running", Epochs: 1})
	s.publish("b", "done", jobMeta{ID: "b", Status: "done"})
	select {
	case ev := <-job:
		if ev.ID != "a" || ev.Name != "epoch" {
			t.Fatalf("job bus %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("job bus missed epoch")
	}
	got := map[string]string{}
	deadline := time.After(time.Second)
	for len(got) < 2 {
		select {
		case ev := <-all:
			got[ev.ID] = ev.Name
		case <-deadline:
			t.Fatalf("feed=%v", got)
		}
	}
	if got["a"] != "epoch" || got["b"] != "done" {
		t.Fatalf("feed=%v", got)
	}
}

func TestWatchShowsRequest(t *testing.T) {
	sess, ctx := taskgroup.New(t.Context(), taskgroup.DefaultLimits())
	t.Cleanup(func() {
		sess.Cancel(context.Canceled)
		_ = sess.Wait()
	})
	s := &server{ctx: ctx}
	saw := make(chan struct{})
	h := s.watch(func(w http.ResponseWriter, r *http.Request) {
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			for _, n := range sess.List(16) {
				if strings.Contains(n.Name, "GET /ping") && n.State == taskgroup.Running {
					close(saw)
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
			time.Sleep(time.Millisecond)
		}
		http.Error(w, "missing task", http.StatusInternalServerError)
	})
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/ping", nil))
	select {
	case <-saw:
	default:
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d", rec.Code)
	}
}
