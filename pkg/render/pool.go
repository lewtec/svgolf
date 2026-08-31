package render

import (
	"image"
	"runtime"
	"sync"
)

var (
	workersOnce sync.Once
	pixmaps     chan *pixmap
	images      chan *image.NRGBA
)

func workerN() int {
	n := runtime.GOMAXPROCS(0)
	if n < 1 {
		n = 1
	}
	return n
}

func initWorkers() {
	workersOnce.Do(func() {
		n := workerN()
		pixmaps = make(chan *pixmap, n)
		images = make(chan *image.NRGBA, n)
		for i := 0; i < n; i++ {
			pixmaps <- &pixmap{}
			images <- &image.NRGBA{}
		}
	})
}

func acquirePixmap(w, h int) *pixmap {
	initWorkers()
	p := <-pixmaps
	need := w * h * 4
	if cap(p.pix) < need {
		p.pix = make([]uint8, need)
	} else {
		p.pix = p.pix[:need]
		clear(p.pix)
	}
	p.w, p.h = w, h
	return p
}

func releasePixmap(p *pixmap) {
	if p == nil {
		return
	}
	pixmaps <- p
}

func acquireImage(w, h int) *image.NRGBA {
	initWorkers()
	img := <-images
	need := w * h * 4
	if cap(img.Pix) < need {
		img.Pix = make([]uint8, need)
	} else {
		img.Pix = img.Pix[:need]
	}
	img.Stride = w * 4
	img.Rect = image.Rect(0, 0, w, h)
	return img
}

// Release returns a Scratch image to the worker pool.
func Release(img *image.NRGBA) {
	if img == nil {
		return
	}
	images <- img
}

// Keep copies a Scratch image so it can outlive Release.
func Keep(img *image.NRGBA) *image.NRGBA {
	if img == nil {
		return nil
	}
	out := image.NewNRGBA(img.Rect)
	copy(out.Pix, img.Pix)
	return out
}
