package stack

import (
	"image"
	"image/color"
	"testing"

	"github.com/lewtec/svgolf/internal/loss"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

func nrgba1024(c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 1024, 1024))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = c.R
		img.Pix[i+1] = c.G
		img.Pix[i+2] = c.B
		img.Pix[i+3] = c.A
	}
	return img
}

func benchDoc() (svg.Document, *image.NRGBA) {
	want := nrgba1024(color.NRGBA{R: 12, G: 52, B: 88, A: 255})
	navy := color.NRGBA{R: 12, G: 52, B: 88, A: 255}
	doc := svg.NewDocument(1024, 1024).WithViewBox(0, 0, 1024, 1024)
	doc = doc.Append(whitePane(1024, 1024).Node())
	for i := 0; i < 8; i++ {
		x := float64(80 + i*110)
		y := float64(80 + (i%3)*280)
		p := svg.NewPath().MoveTo(x, y).LineTo(x+200, y+40).LineTo(x+40, y+220).Close().WithFill(navy)
		doc = doc.Append(p.Node())
	}
	return doc, want
}

func BenchmarkScore(b *testing.B) {
	got := nrgba1024(color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	want := nrgba1024(color.NRGBA{R: 12, G: 52, B: 88, A: 255})
	_ = Score(got, want)
	b.ReportAllocs()
	for b.Loop() {
		_ = Score(got, want)
	}
}

func BenchmarkScoreOn(b *testing.B) {
	got := nrgba1024(color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	want := nrgba1024(color.NRGBA{R: 12, G: 52, B: 88, A: 255})
	gotP := loss.NewPlane(got)
	wantP := loss.NewPlane(want)
	gotP.Ensure()
	wantP.Ensure()
	b.ReportAllocs()
	for b.Loop() {
		_ = ScoreOn(gotP, wantP)
	}
}

func BenchmarkScratchScore(b *testing.B) {
	doc, want := benchDoc()
	img, err := render.Scratch(doc)
	if err != nil {
		b.Fatal(err)
	}
	_ = Score(img, want)
	render.Release(img)
	b.ReportAllocs()
	for b.Loop() {
		img, err := render.Scratch(doc)
		if err != nil {
			b.Fatal(err)
		}
		_ = Score(img, want)
		render.Release(img)
	}
}
