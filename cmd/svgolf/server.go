package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

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
			s := &server{cache: cache, algo: algo}
			mux := http.NewServeMux()
			mux.HandleFunc("GET /", s.handleHome)
			mux.HandleFunc("POST /jobs", s.handleCreate)
			mux.HandleFunc("GET /jobs/{id}", s.handleJob)
			mux.HandleFunc("GET /jobs/{id}/events", s.handleEvents)
			mux.HandleFunc("GET /jobs/{id}/files/{name}", s.handleFile)
			srv := &http.Server{Addr: addr, Handler: mux}
			cmd.Println("svgolf server", addr, "cache", cache)
			return srv.ListenAndServe()
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
	mu    sync.Mutex
	subs  map[string][]chan []byte
}

type jobMeta struct {
	ID       string  `json:"id"`
	Status   string  `json:"status"`
	Search   string  `json:"search"`
	Epochs   int     `json:"epochs"`
	Operator string  `json:"operator"`
	Score    float64 `json:"score"`
	Paths    int     `json:"paths"`
	Elapsed  string  `json:"elapsed"`
	Err      string  `json:"error,omitempty"`
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
	id := time.Now().UTC().Format("20060102-150405") + "-" + name
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
	go s.runJob(dir, id, want)
	http.Redirect(w, r, "/jobs/"+id, http.StatusSeeOther)
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
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	ch := s.subscribe(id)
	defer s.unsubscribe(id, ch)
	if meta, err := readMeta(filepath.Join(s.cache, id)); err == nil {
		writeSSE(w, "epoch", meta)
		fl.Flush()
		if meta.Status != "running" {
			writeSSE(w, "done", meta)
			fl.Flush()
			return
		}
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case b, ok := <-ch:
			if !ok {
				return
			}
			_, _ = w.Write(b)
			fl.Flush()
		}
	}
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

func (s *server) runJob(dir, id string, want *image.NRGBA) {
	searcher, err := search.New(s.algo)
	if err != nil {
		s.fail(dir, id, err)
		return
	}
	n := 0
	for ep, err := range searcher.Search(context.Background(), want) {
		if err != nil {
			s.fail(dir, id, err)
			return
		}
		if err := writeEpoch(dir, n, ep, want); err != nil {
			s.fail(dir, id, err)
			return
		}
		n++
		meta := jobMeta{
			ID:       id,
			Status:   "running",
			Search:   s.algo,
			Epochs:   n,
			Operator: ep.Operator,
			Score:    epScore(ep, want),
			Paths:    len(ep.Document.Children()),
			Elapsed:  ep.Elapsed.String(),
		}
		_ = writeMeta(dir, meta)
		s.publish(id, "epoch", meta)
	}
	meta, _ := readMeta(dir)
	meta.Status = "done"
	_ = writeMeta(dir, meta)
	s.publish(id, "done", meta)
}

func writeEpoch(dir string, n int, ep search.Epoch, want *image.NRGBA) error {
	pad := fmt.Sprintf("%03d", n)
	if err := NewSVGFile(filepath.Join(dir, pad+".svg")).Render(ep.Document); err != nil {
		return err
	}
	if err := NewSVGFile(filepath.Join(dir, "last.svg")).Render(ep.Document); err != nil {
		return err
	}
	got, err := render.Render(ep.Document)
	if err != nil {
		return err
	}
	if err := writePNG(filepath.Join(dir, pad+".png"), got); err != nil {
		return err
	}
	if err := writePNG(filepath.Join(dir, "last.png"), got); err != nil {
		return err
	}
	heat, island := ep.Heat, ep.Island
	if heat == nil || island == nil {
		heat, island = stack.DebugFrames(got, want, nil)
	}
	if err := writePNG(filepath.Join(dir, pad+"-error.png"), heat); err != nil {
		return err
	}
	if err := writePNG(filepath.Join(dir, "last-error.png"), heat); err != nil {
		return err
	}
	if err := writePNG(filepath.Join(dir, pad+"-island.png"), island); err != nil {
		return err
	}
	return writePNG(filepath.Join(dir, "last-island.png"), island)
}

func epScore(ep search.Epoch, want *image.NRGBA) float64 {
	got, err := render.Render(ep.Document)
	if err != nil {
		return 0
	}
	return stack.Score(got, want, len(ep.Document.Children()))
}

func (s *server) fail(dir, id string, err error) {
	meta, _ := readMeta(dir)
	meta.ID = id
	meta.Status = "error"
	meta.Err = err.Error()
	_ = writeMeta(dir, meta)
	s.publish(id, "done", meta)
}

func (s *server) subscribe(id string) chan []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.subs == nil {
		s.subs = map[string][]chan []byte{}
	}
	ch := make(chan []byte, 8)
	s.subs[id] = append(s.subs[id], ch)
	return ch
}

func (s *server) unsubscribe(id string, ch chan []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.subs[id][:0]
	for _, c := range s.subs[id] {
		if c != ch {
			out = append(out, c)
		}
	}
	s.subs[id] = out
	close(ch)
}

func (s *server) publish(id, ev string, meta jobMeta) {
	var b strings.Builder
	b.WriteString("event: ")
	b.WriteString(ev)
	b.WriteString("\ndata: ")
	enc, _ := json.Marshal(epochPayload(meta))
	b.Write(enc)
	b.WriteString("\n\n")
	msg := []byte(b.String())
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range s.subs[id] {
		select {
		case ch <- msg:
		default:
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
	return map[string]any{
		"n":        n,
		"status":   m.Status,
		"operator": m.Operator,
		"score":    fmt.Sprintf("%.3f", m.Score),
		"paths":    m.Paths,
		"elapsed":  m.Elapsed,
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
	return os.WriteFile(filepath.Join(dir, "job.json"), b, 0o644)
}

func writePNG(path string, img image.Image) error {
	if img == nil {
		return fmt.Errorf("nil image")
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	err = png.Encode(f, img)
	if c := f.Close(); err == nil {
		err = c
	}
	return err
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
	return web.JobView{
		ID:       m.ID,
		Status:   m.Status,
		Search:   m.Search,
		Epochs:   m.Epochs,
		Operator: m.Operator,
		Score:    strconv.FormatFloat(m.Score, 'f', 3, 64),
		Paths:    m.Paths,
		Err:      m.Err,
	}
}
