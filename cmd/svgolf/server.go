package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lewtec/lewkit/x/event"
	"github.com/lewtec/lewkit/x/taskgroup"
	"github.com/lewtec/lewkit/x/taskgroup/progress"
	"github.com/lewtec/svgolf/cmd/svgolf/web"
	"github.com/lewtec/svgolf/internal/search"
	"github.com/lewtec/svgolf/internal/search/stack"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/spf13/cobra"
)

func newServerCmd() *cobra.Command {
	var (
		cache string
		addr  string
		algo  string
	)
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Watch Search converge from a cache folder",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cache == "" {
				return fmt.Errorf("server: --cache is required")
			}
			if err := os.MkdirAll(cache, 0o755); err != nil {
				return err
			}
			ctx := cmd.Context()
			if ctx == nil {
				return fmt.Errorf("server: missing parent context")
			}
			sess, sctx := taskgroup.New(ctx, taskgroup.DefaultLimits())
			return progress.Run(sess, sctx, func(ctx context.Context) error {
				s := &server{cache: cache, algo: algo, ctx: ctx, all: event.New[jobEvent]()}
				mux := http.NewServeMux()
				mux.HandleFunc("GET /", s.watch(s.handleHome))
				mux.HandleFunc("POST /jobs", s.watch(s.handleCreate))
				mux.HandleFunc("GET /events", s.watch(s.handleAllEvents))
				mux.HandleFunc("GET /jobs/{id}", s.watch(s.handleJob))
				mux.HandleFunc("GET /jobs/{id}/events", s.watch(s.handleEvents))
				mux.HandleFunc("GET /jobs/{id}/files/{name}", s.watch(s.handleFile))
				srv := &http.Server{
					Addr:    addr,
					Handler: mux,
					BaseContext: func(net.Listener) context.Context {
						return ctx
					},
				}
				cmd.Println("svgolf server", addr, "cache", cache)
				if algo == "stack" {
					stack.OpenDriver(ctx)
				}
				errc := make(chan error, 1)
				go func() { errc <- srv.ListenAndServe() }()
				select {
				case <-ctx.Done():
					shut, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
					defer cancel()
					err := srv.Shutdown(shut)
					<-errc
					return err
				case err := <-errc:
					if err == http.ErrServerClosed {
						return nil
					}
					return err
				}
			})
		},
	}
	cmd.Flags().StringVar(&cache, "cache", "", "folder that stores every job and epoch frame")
	cmd.Flags().StringVar(&addr, "addr", ":8080", "listen address")
	cmd.Flags().StringVar(&algo, "search", "stack", "Search adapter")
	_ = cmd.MarkFlagRequired("cache")
	return cmd
}

type server struct {
	cache string
	algo  string
	ctx   context.Context
	mu    sync.Mutex
	buses map[string]*event.Bus[jobEvent]
	all   *event.Bus[jobEvent]
}

// jobEvent is one epoch or the terminal done for a job.
// The browser listens with EventSource; this bus is what the handler subscribes to.
type jobEvent struct {
	ID   string
	Name string
	Meta jobMeta
}

var jobSeq atomic.Uint64

func newJobID(name string) string {
	now := time.Now().UTC()
	n := jobSeq.Add(1)
	return fmt.Sprintf("%s%09d-%08x-%s", now.Format("20060102-150405"), now.Nanosecond(), n, name)
}

type jobMeta struct {
	ID           string           `json:"id"`
	Status       string           `json:"status"`
	Search       string           `json:"search"`
	Epochs       int              `json:"epochs"`
	Operator     string           `json:"operator"`
	Score        float64          `json:"score"`
	Scores       []float64        `json:"scores,omitempty"`
	Rounds       [][]search.Rated `json:"rounds,omitempty"`
	Paths        int              `json:"paths"`
	Vertices     int              `json:"vertices"`
	PathCounts   []int            `json:"pathCounts,omitempty"`
	VertexCounts []int            `json:"vertexCounts,omitempty"`
	Elapsed      string           `json:"elapsed"`
	Err          string           `json:"error,omitempty"`
}

func (s *server) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	jobs, err := s.listJobs()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = web.Home(toViews(jobs)).Render(r.Context(), w)
}

func (s *server) handleCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f, hdr, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "image is required", http.StatusBadRequest)
		return
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		http.Error(w, "png decode: "+err.Error(), http.StatusBadRequest)
		return
	}
	want := search.FromImage(img)
	name := strings.TrimSuffix(filepath.Base(hdr.Filename), filepath.Ext(hdr.Filename))
	name = sanitize(name)
	if name == "" {
		name = "job"
	}
	id := newJobID(name)
	dir := filepath.Join(s.cache, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := writePNG(filepath.Join(dir, "want.png"), want); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	meta := jobMeta{ID: id, Status: "running", Search: s.algo}
	if err := writeMeta(dir, meta); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.jobBus(id)
	s.startJob(dir, id, want)
	http.Redirect(w, r, "/jobs/"+id, http.StatusSeeOther)
}

func (s *server) watch(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.ctx == nil || taskgroup.FromContext(s.ctx) == nil {
			h(w, r)
			return
		}
		finished := make(chan struct{})
		taskgroup.Go(s.ctx, r.Method+" "+r.URL.Path, taskgroup.Control, func(ctx context.Context, st *taskgroup.Status) error {
			st.Update(r.Method)
			select {
			case <-finished:
				return nil
			case <-ctx.Done():
				return context.Cause(ctx)
			}
		})
		defer close(finished)
		h(w, r)
	}
}

func (s *server) handleJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	meta, err := readMeta(filepath.Join(s.cache, id))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = web.JobPage(toView(meta)).Render(r.Context(), w)
}

func (s *server) handleEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	fl, ok := flushEvents(w)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}
	ctx := r.Context()
	sub := s.jobBus(id).Subscribe(ctx)
	if meta, err := readMeta(filepath.Join(s.cache, id)); err == nil {
		writeSSE(w, "epoch", meta)
		fl.Flush()
		if meta.Status != "running" {
			writeSSE(w, "done", meta)
			fl.Flush()
			return
		}
	}
	streamEvents(ctx, w, fl, sub, id)
}

func (s *server) handleAllEvents(w http.ResponseWriter, r *http.Request) {
	fl, ok := flushEvents(w)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}
	ctx := r.Context()
	sub := s.feed().Subscribe(ctx)
	if jobs, err := s.listJobs(); err == nil {
		for _, m := range jobs {
			writeSSE(w, "epoch", m)
			fl.Flush()
			if m.Status != "running" {
				writeSSE(w, "done", m)
				fl.Flush()
			}
		}
	}
	streamEvents(ctx, w, fl, sub, "")
}

func flushEvents(w http.ResponseWriter) (http.Flusher, bool) {
	fl, ok := w.(http.Flusher)
	if !ok {
		return nil, false
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	return fl, true
}

func (s *server) handleFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name := filepath.Base(r.PathValue("name"))
	if name == "." || name == string(os.PathSeparator) {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, filepath.Join(s.cache, id, name))
}

func (s *server) startJob(dir, id string, want *image.NRGBA) {
	parent := s.ctx
	go func() {
		if parent == nil || taskgroup.FromContext(parent) == nil {
			_ = s.runJob(parent, nil, dir, id, want)
			return
		}
		_ = taskgroup.GoIsolated(parent, id, taskgroup.Control, func(ctx context.Context, st *taskgroup.Status) error {
			return s.runJob(ctx, st, dir, id, want)
		})
	}()
}

func (s *server) runJob(ctx context.Context, st *taskgroup.Status, dir, id string, want *image.NRGBA) error {
	if ctx == nil {
		return s.stopJob(st, dir, id, fmt.Errorf("server: missing parent context"))
	}
	if st != nil {
		st.Update("search")
		st.Progress(0, -1)
	}
	searcher, err := search.New(s.algo)
	if err != nil {
		return s.stopJob(st, dir, id, err)
	}
	stack.ShowFrames(true)
	n := 0
	var scores []float64
	var rounds [][]search.Rated
	var pathCounts []int
	var vertexCounts []int
	for ep, err := range searcher.Search(ctx, want) {
		if err != nil {
			return s.stopJob(st, dir, id, err)
		}
		got, err := writeEpoch(dir, n, ep, want)
		if err != nil {
			return s.stopJob(st, dir, id, err)
		}
		n++
		sc := stack.Score(ctx, got, want)
		scores = append(scores, sc)
		rounds = append(rounds, ep.Rated)
		paths := documentPaths(ep.Document)
		vertices := documentVertices(ep.Document)
		pathCounts = append(pathCounts, paths)
		vertexCounts = append(vertexCounts, vertices)
		meta := jobMeta{
			ID:           id,
			Status:       "running",
			Search:       s.algo,
			Epochs:       n,
			Operator:     ep.Operator.String(),
			Score:        sc,
			Scores:       scores,
			Rounds:       rounds,
			Paths:        paths,
			Vertices:     vertices,
			PathCounts:   pathCounts,
			VertexCounts: vertexCounts,
			Elapsed:      ep.Elapsed.String(),
		}
		_ = writeMeta(dir, meta)
		s.publish(id, "epoch", meta)
		if st != nil {
			st.Update(fmt.Sprintf("epoch %d · %s · %.3f", n, meta.Operator, sc))
			st.Progress(int64(n), -1)
		}
	}
	meta, _ := readMeta(dir)
	meta.ID = id
	meta.Status = "done"
	_ = writeMeta(dir, meta)
	s.publish(id, "done", meta)
	if st != nil {
		st.Update("done")
		if n > 0 {
			st.Progress(int64(n), int64(n))
		}
	}
	return nil
}

func writeEpoch(dir string, n int, ep search.Epoch, want *image.NRGBA) (*image.NRGBA, error) {
	pad := fmt.Sprintf("%03d", n)
	if err := NewSVGFile(filepath.Join(dir, pad+".svg")).Render(ep.Document); err != nil {
		return nil, err
	}
	if err := NewSVGFile(filepath.Join(dir, "last.svg")).Render(ep.Document); err != nil {
		return nil, err
	}
	got, err := render.Render(ep.Document)
	if err != nil {
		return nil, err
	}
	if err := writePNG(filepath.Join(dir, pad+".png"), got); err != nil {
		return nil, err
	}
	if err := writePNG(filepath.Join(dir, "last.png"), got); err != nil {
		return nil, err
	}
	heat, island := ep.Heat, ep.Island
	if heat == nil || island == nil {
		heat, island = stack.DebugFrames(got, want, nil, nil)
	}
	if err := writePNG(filepath.Join(dir, pad+"-error.png"), heat); err != nil {
		return nil, err
	}
	if err := writePNG(filepath.Join(dir, "last-error.png"), heat); err != nil {
		return nil, err
	}
	if err := writePNG(filepath.Join(dir, pad+"-island.png"), island); err != nil {
		return nil, err
	}
	if err := writePNG(filepath.Join(dir, "last-island.png"), island); err != nil {
		return nil, err
	}
	return got, nil
}

func (s *server) stopJob(st *taskgroup.Status, dir, id string, err error) error {
	s.fail(dir, id, err)
	if st != nil {
		st.Update(err.Error())
	}
	return err
}

func (s *server) fail(dir, id string, err error) {
	meta, _ := readMeta(dir)
	meta.ID = id
	meta.Status = "error"
	meta.Err = err.Error()
	_ = writeMeta(dir, meta)
	s.publish(id, "done", meta)
}

func (s *server) jobBus(id string) *event.Bus[jobEvent] {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.buses == nil {
		s.buses = map[string]*event.Bus[jobEvent]{}
	}
	b := s.buses[id]
	if b == nil {
		b = event.New[jobEvent]()
		s.buses[id] = b
	}
	return b
}

func (s *server) feed() *event.Bus[jobEvent] {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.all == nil {
		s.all = event.New[jobEvent]()
	}
	return s.all
}

func (s *server) publish(id, name string, meta jobMeta) {
	ev := jobEvent{ID: id, Name: name, Meta: meta}
	s.jobBus(id).Publish(ev)
	s.feed().Publish(ev)
}

type queuedJob struct {
	epoch *jobEvent
	done  *jobEvent
}

func streamEvents(ctx context.Context, w http.ResponseWriter, fl http.Flusher, sub <-chan jobEvent, only string) {
	ctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	defer func() {
		cancel()
		wg.Wait()
	}()

	var mu sync.Mutex
	queued := map[string]*queuedJob{}
	var order []string
	ready := make(chan struct{}, 1)
	poke := func() {
		select {
		case ready <- struct{}{}:
		default:
		}
	}
	offer := func(ev jobEvent) {
		if only != "" && ev.ID != only {
			return
		}
		slot := queued[ev.ID]
		if slot == nil {
			slot = &queuedJob{}
			queued[ev.ID] = slot
			order = append(order, ev.ID)
		}
		cp := ev
		if ev.Name == "done" {
			slot.done = &cp
			return
		}
		if slot.done != nil {
			return
		}
		slot.epoch = &cp
	}
	live := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(live)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-sub:
				if !ok {
					return
				}
				mu.Lock()
				offer(ev)
				mu.Unlock()
				poke()
			}
		}
	}()

	writeSlot := func(slot *queuedJob) bool {
		if slot.epoch != nil {
			writeSSE(w, slot.epoch.Name, slot.epoch.Meta)
			fl.Flush()
		}
		if slot.done != nil {
			writeSSE(w, slot.done.Name, slot.done.Meta)
			fl.Flush()
			return only != ""
		}
		return false
	}
	take := func() ([]string, map[string]*queuedJob) {
		mu.Lock()
		defer mu.Unlock()
		batchOrder, batch := order, queued
		order = nil
		queued = map[string]*queuedJob{}
		return batchOrder, batch
	}
	for {
		batchOrder, batch := take()
		for _, id := range batchOrder {
			if writeSlot(batch[id]) {
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-live:
			batchOrder, batch = take()
			for _, id := range batchOrder {
				if writeSlot(batch[id]) {
					return
				}
			}
			return
		case <-ready:
		}
	}
}

func writeSSE(w http.ResponseWriter, ev string, meta jobMeta) {
	fmt.Fprintf(w, "event: %s\ndata: ", ev)
	enc, _ := json.Marshal(epochPayload(meta))
	_, _ = w.Write(enc)
	_, _ = io.WriteString(w, "\n\n")
}

func epochPayload(m jobMeta) map[string]any {
	n := m.Epochs - 1
	if n < 0 {
		n = 0
	}
	scores := m.Scores
	if scores == nil {
		scores = []float64{}
	}
	rounds := m.Rounds
	if rounds == nil {
		rounds = [][]search.Rated{}
	}
	pathCounts := m.PathCounts
	if pathCounts == nil {
		pathCounts = []int{}
	}
	vertexCounts := m.VertexCounts
	if vertexCounts == nil {
		vertexCounts = []int{}
	}
	return map[string]any{
		"id":           m.ID,
		"n":            n,
		"status":       m.Status,
		"operator":     m.Operator,
		"score":        fmt.Sprintf("%.3f", m.Score),
		"scores":       scores,
		"rounds":       rounds,
		"paths":        m.Paths,
		"vertices":     m.Vertices,
		"pathCounts":   pathCounts,
		"vertexCounts": vertexCounts,
		"elapsed":      m.Elapsed,
	}
}

func (s *server) listJobs() ([]jobMeta, error) {
	ents, err := os.ReadDir(s.cache)
	if err != nil {
		return nil, err
	}
	var out []jobMeta
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		m, err := readMeta(filepath.Join(s.cache, e.Name()))
		if err != nil {
			continue
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

func readMeta(dir string) (jobMeta, error) {
	b, err := os.ReadFile(filepath.Join(dir, "job.json"))
	if err != nil {
		return jobMeta{}, err
	}
	var m jobMeta
	err = json.Unmarshal(b, &m)
	return m, err
}

func writeMeta(dir string, m jobMeta) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return writeAtomic(filepath.Join(dir, "job.json"), func(w io.Writer) error {
		_, err := w.Write(b)
		return err
	})
}

func writePNG(path string, img image.Image) error {
	if img == nil {
		return fmt.Errorf("nil image")
	}
	return writeAtomic(path, func(w io.Writer) error {
		return png.Encode(w, img)
	})
}

func writeAtomic(path string, write func(io.Writer) error) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".tmp-"+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	err = write(f)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func toViews(ms []jobMeta) []web.JobView {
	out := make([]web.JobView, len(ms))
	for i, m := range ms {
		out[i] = toView(m)
	}
	return out
}

func toView(m jobMeta) web.JobView {
	scores := m.Scores
	if scores == nil {
		scores = []float64{}
	}
	b, err := json.Marshal(scores)
	if err != nil {
		b = []byte("[]")
	}
	rounds := m.Rounds
	if rounds == nil {
		rounds = [][]search.Rated{}
	}
	rb, err := json.Marshal(rounds)
	if err != nil {
		rb = []byte("[]")
	}
	pathCounts := m.PathCounts
	if pathCounts == nil {
		pathCounts = []int{}
	}
	pb, err := json.Marshal(pathCounts)
	if err != nil {
		pb = []byte("[]")
	}
	vertexCounts := m.VertexCounts
	if vertexCounts == nil {
		vertexCounts = []int{}
	}
	vb, err := json.Marshal(vertexCounts)
	if err != nil {
		vb = []byte("[]")
	}
	return web.JobView{
		ID:               m.ID,
		Status:           m.Status,
		Search:           m.Search,
		Epochs:           m.Epochs,
		Operator:         m.Operator,
		Score:            strconv.FormatFloat(m.Score, 'f', 3, 64),
		ScoresJSON:       string(b),
		RoundsJSON:       string(rb),
		PathCountsJSON:   string(pb),
		VertexCountsJSON: string(vb),
		Paths:            m.Paths,
		Vertices:         m.Vertices,
		Err:              m.Err,
	}
}
