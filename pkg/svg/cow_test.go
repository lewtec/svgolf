package svg

import (
	"errors"
	"testing"
)

func TestAppendCopyOnWrite(t *testing.T) {
	t.Parallel()
	d := NewDocument(10, 10)
	d2 := d.Append(NewCircle().Node())
	d3 := d.Append(NewRect().Node())
	if n := len(d.Children()); n != 0 {
		t.Errorf("original Document has %d children; want 0", n)
	}
	if n := len(d2.Children()); n != 1 || d2.Children()[0].Kind() != KindCircle {
		t.Errorf("d2 children = %v", kinds(d2.Children()))
	}
	if n := len(d3.Children()); n != 1 || d3.Children()[0].Kind() != KindRect {
		t.Errorf("d3 children = %v", kinds(d3.Children()))
	}

	kids := d2.Children()
	kids[0] = NewEllipse().Node()
	if d2.Children()[0].Kind() != KindCircle {
		t.Error("mutating Children() result changed the document")
	}

	g := NewGroup()
	g2 := g.Append(NewCircle().Node())
	g3 := g.Append(NewRect().Node())
	if len(g.Children()) != 0 {
		t.Error("original Group grew")
	}
	if g2.Children()[0].Kind() != KindCircle || g3.Children()[0].Kind() != KindRect {
		t.Error("group append aliased")
	}
	gk := g2.Children()
	gk[0] = NewEllipse().Node()
	if g2.Children()[0].Kind() != KindCircle {
		t.Error("mutating Group.Children() result changed the group")
	}
}

func TestWithPointsCopies(t *testing.T) {
	t.Parallel()
	in := [][2]float64{{0, 0}, {10, 0}, {0, 10}}
	p, err := NewPolygon().WithPoints(in)
	if err != nil {
		t.Fatal(err)
	}
	in[0] = [2]float64{99, 99}
	got := p.Points()
	if got[0] != ([2]float64{0, 0}) {
		t.Errorf("mutating WithPoints input changed stored points: %v", got[0])
	}
	got[1] = [2]float64{7, 7}
	again := p.Points()
	if again[1] != ([2]float64{10, 0}) {
		t.Errorf("mutating Points() result changed stored points: %v", again[1])
	}
}

func TestWithPointsRejectsBadLength(t *testing.T) {
	t.Parallel()
	_, err := NewPolygon().WithPoints(nil)
	if !errors.Is(err, ErrPolygonPoints) {
		t.Errorf("nil: err = %v; want ErrPolygonPoints", err)
	}
	two := [][2]float64{{0, 0}, {1, 1}}
	p, err := NewPolygon().WithPoints(two)
	if !errors.Is(err, ErrPolygonPoints) {
		t.Errorf("2 pts: err = %v", err)
	}
	if len(p.Points()) != 0 {
		t.Error("invalid WithPoints stored points")
	}
	big := make([][2]float64, 1025)
	if _, err := NewPolygon().WithPoints(big); !errors.Is(err, ErrPolygonPoints) {
		t.Errorf("1025 pts: err = %v", err)
	}
	ok := make([][2]float64, 1024)
	if _, err := NewPolygon().WithPoints(ok); err != nil {
		t.Errorf("1024 pts: %v", err)
	}
}

func TestPathCommandsCopy(t *testing.T) {
	t.Parallel()
	in := []PathCmd{{Kind: CmdMove, X: 0, Y: 0}, {Kind: CmdLine, X: 1, Y: 0}, {Kind: CmdClose}}
	p, err := NewPath().WithCommands(in)
	if err != nil {
		t.Fatal(err)
	}
	in[0].X = 99
	got := p.Commands()
	if got[0].X != 0 {
		t.Errorf("mutating WithCommands input changed stored cmds: %+v", got[0])
	}
	got[1].X = 7
	again := p.Commands()
	if again[1].X != 1 {
		t.Errorf("mutating Commands() result changed stored cmds: %+v", again[1])
	}
}

func TestRXIndependentOfRY(t *testing.T) {
	t.Parallel()
	r := NewRect().WithWidth(10).WithHeight(10).WithRX(5)
	if r.RX() != 5 || r.RY() != 0 {
		t.Errorf("RX,RY = %v,%v; want 5,0", r.RX(), r.RY())
	}
	rxSet, rySet := r.radiiSet()
	if !rxSet || rySet {
		t.Errorf("radiiSet = %v,%v; want true,false", rxSet, rySet)
	}
}

func kinds(ns []Node) []Kind {
	out := make([]Kind, len(ns))
	for i, n := range ns {
		out[i] = n.Kind()
	}
	return out
}
