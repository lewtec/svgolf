package resvg

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os/exec"
	"time"
)

const (
	defaultTimeout = 5 * time.Second
	miseHint       = "mise install"
)

func LookPath() (string, error) {
	p, err := exec.LookPath("resvg")
	if err != nil {
		return "", fmt.Errorf("resvg: not on PATH (%s / mise run test): %w", miseHint, err)
	}
	return p, nil
}

func Render(ctx context.Context, svgXML []byte) (*image.NRGBA, error) {
	bin, err := LookPath()
	if err != nil {
		return nil, err
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, bin, "-", "-c")
	cmd.Stdin = bytes.NewReader(svgXML)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("resvg: %w: %s", err, stderr.Bytes())
		}
		return nil, fmt.Errorf("resvg: %w", err)
	}
	img, err := png.Decode(bytes.NewReader(stdout.Bytes()))
	if err != nil {
		return nil, fmt.Errorf("resvg png: %w", err)
	}
	return toNRGBA(img), nil
}

func toNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok && n.Rect.Min == (image.Point{}) {
		return n
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	return out
}
