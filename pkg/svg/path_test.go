package svg

import (
	"strings"
	"testing"
)

func TestPathRoundTrip(t *testing.T) {
	p := NewPath().MoveTo(40, 40).LineTo(120, 40).LineTo(80, 100).Close()
	doc := NewDocument(256, 256).Append(p.Node())
	s, err := EncodeToString(doc)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	gp, ok := got.Children()[0].Path()
	if !ok {
		t.Fatal("not path")
	}
	if n := len(gp.Commands()); n != 4 {
		t.Fatalf("cmds=%d", n)
	}
}

func TestParseRelativeAndSmooth(t *testing.T) {
	doc, err := Parse(strings.NewReader(
		`<svg xmlns="http://www.w3.org/2000/svg" width="256" height="256"><path d="m10 10 l10 0 c0 10 10 10 10 0 s10-10 10 0 z"/></svg>`,
	))
	if err != nil {
		t.Fatal(err)
	}
	p, ok := doc.Children()[0].Path()
	if !ok {
		t.Fatal("not path")
	}
	cmds := p.Commands()
	if cmds[0].Kind != CmdMove || cmds[0].X != 10 || cmds[0].Y != 10 {
		t.Fatalf("move %+v", cmds[0])
	}
	if cmds[1].Kind != CmdLine || cmds[1].X != 20 || cmds[1].Y != 10 {
		t.Fatalf("line %+v", cmds[1])
	}
	if cmds[2].Kind != CmdCubic || cmds[2].X1 != 20 || cmds[2].Y1 != 20 || cmds[2].X2 != 30 || cmds[2].Y2 != 20 || cmds[2].X != 30 || cmds[2].Y != 10 {
		t.Fatalf("cubic %+v", cmds[2])
	}
	if cmds[3].Kind != CmdCubic || cmds[3].X1 != 30 || cmds[3].Y1 != 0 || cmds[3].X2 != 40 || cmds[3].Y2 != 0 || cmds[3].X != 40 || cmds[3].Y != 10 {
		t.Fatalf("smooth %+v", cmds[3])
	}
	if cmds[4].Kind != CmdClose {
		t.Fatalf("z %+v", cmds[4])
	}
}

func TestParseRejectsQuadraticAndArc(t *testing.T) {
	for _, d := range []string{
		`M0 0 Q10 10 20 0`,
		`M0 0 C10 0 10 10 20 10 T40 10`,
		`M0 0 A10 10 0 0 1 10 10`,
	} {
		_, err := Parse(strings.NewReader(
			`<svg xmlns="http://www.w3.org/2000/svg" width="256" height="256"><path d="` + d + `"/></svg>`,
		))
		if err == nil {
			t.Fatalf("expected error for %s", d)
		}
	}
}
