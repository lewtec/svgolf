package search

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lewtec/svgolf/internal/loss"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

func TestFitResSolid(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 10, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 10; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	r := &FitRes{}
	doc, err := r.Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Renders=%d Parts=%d RMSE=%g Fit=%g", r.Renders, svg.PartsDocument(doc), r.RMSE, r.Fit)
	if r.Renders == 0 || r.Renders > fitMaxRenders {
		t.Fatalf("Renders=%d", r.Renders)
	}
	if n := svg.PartsDocument(doc); n != 1 {
		t.Fatalf("Parts=%d want 1 plate", n)
	}
	got, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	if e := loss.RMSE(got, img); e != 0 {
		t.Fatalf("RMSE=%g want 0", e)
	}
}

func TestFitResTwoColor(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			c := color.NRGBA{R: 255, A: 255}
			if x >= 2 && x < 6 && y >= 2 && y < 6 {
				c = color.NRGBA{B: 255, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	r := &FitRes{Colors: 2}
	doc, err := r.Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	got, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	gotFit, err := loss.OfFit(doc, img)
	if err != nil {
		t.Fatal(err)
	}
	plate := svg.NewDocument(8, 8).WithViewBox(0, 0, 8, 8).Append(
		svg.NewRect().WithX(0).WithY(0).WithWidth(8).WithHeight(8).WithFill(color.NRGBA{R: 255, A: 255}).Node(),
	)
	plateFit, err := loss.OfFit(plate, img)
	if err != nil {
		t.Fatal(err)
	}
	parts := svg.PartsDocument(doc)
	rmse := loss.RMSE(got, img)
	t.Logf("Renders=%d Parts=%d RMSE=%g Fit=%g plateFit=%g", r.Renders, parts, rmse, gotFit, plateFit)
	if !(gotFit < plateFit) {
		t.Fatalf("Fit=%g not better than 1 plate %g", gotFit, plateFit)
	}
	if r.Renders > fitMaxRenders {
		t.Fatalf("Renders=%d", r.Renders)
	}
	if parts >= fitMaxShapes && rmse < loss.Eps {
		t.Fatalf("Parts=%d at cap with RMSE=%g already low; Fit should stop", parts, rmse)
	}
	if rmse < loss.Eps && parts > 8 {
		t.Fatalf("Parts=%d not modest with RMSE=%g", parts, rmse)
	}
}

func TestFitResNilPixmap(t *testing.T) {
	_, err := (&FitRes{}).Search(t.Context(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFitResEval(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "eval")
	ents, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".png") {
			continue
		}
		n++
		name := e.Name()
		if name == "bliss.png" {
			// Native 4510×3627 is scored on a 4096-capped copy; skip the long eval.
			continue
		}
		t.Run(name, func(t *testing.T) {
			f, err := os.Open(filepath.Join(root, name))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			img, err := png.Decode(f)
			if err != nil {
				t.Fatal(err)
			}
			want := FitCanvas(FromImage(img), MaxCanvas)
			w, h := want.Rect.Dx(), want.Rect.Dy()
			r := &FitRes{}
			doc, err := r.Search(t.Context(), want)
			if err != nil {
				t.Fatal(err)
			}
			if r.Renders > fitMaxRenders {
				t.Fatalf("Renders=%d over budget", r.Renders)
			}
			got, err := render.Render(doc)
			if err != nil {
				t.Fatal(err)
			}
			parts := svg.PartsDocument(doc)
			rmse := loss.RMSE(got, want)
			fit := loss.Fit(got, want, parts)
			kinds := map[svg.Kind]int{}
			for _, c := range doc.Children() {
				kinds[c.Kind()]++
			}
			t.Logf("RMSE=%g Fit=%g Parts=%d Renders=%d canvas=%d×%d kinds=%v", rmse, fit, parts, r.Renders, w, h, kinds)
			if name == "launcher.png" || name == "lewtec.png" {
				if parts > 2 {
					t.Fatalf("%s Parts=%d want ~1 plate", name, parts)
				}
			}
		})
	}
	if n == 0 {
		t.Fatal("no eval pngs")
	}
}
