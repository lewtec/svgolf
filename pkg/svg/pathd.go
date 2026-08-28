package svg

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

func encodePathD(cmds []PathCmd) string {
	var b strings.Builder
	for i, c := range cmds {
		if i > 0 {
			b.WriteByte(' ')
		}
		switch c.Kind {
		case CmdMove:
			b.WriteByte('M')
			b.WriteString(fmtNum(c.X))
			b.WriteByte(' ')
			b.WriteString(fmtNum(c.Y))
		case CmdLine:
			b.WriteByte('L')
			b.WriteString(fmtNum(c.X))
			b.WriteByte(' ')
			b.WriteString(fmtNum(c.Y))
		case CmdCubic:
			b.WriteByte('C')
			b.WriteString(fmtNum(c.X1))
			b.WriteByte(' ')
			b.WriteString(fmtNum(c.Y1))
			b.WriteByte(' ')
			b.WriteString(fmtNum(c.X2))
			b.WriteByte(' ')
			b.WriteString(fmtNum(c.Y2))
			b.WriteByte(' ')
			b.WriteString(fmtNum(c.X))
			b.WriteByte(' ')
			b.WriteString(fmtNum(c.Y))
		case CmdClose:
			b.WriteByte('Z')
		}
	}
	return b.String()
}

func parsePathD(s string) ([]PathCmd, error) {
	t := pathTok{s: s}
	var cmds []PathCmd
	var cx, cy, mx, my, sx, sy float64
	var have, prevC bool
	cmd := byte(0)
	for {
		t.skip()
		if t.done() {
			break
		}
		if isPathCmd(t.peek()) {
			cmd = t.next()
			if cmd >= 'a' && cmd <= 'z' {
				cmd -= 'a' - 'A'
			}
		} else if cmd == 0 {
			return nil, fmt.Errorf("parse: path d must start with a command")
		}
		rel := t.last >= 'a' && t.last <= 'z'
		switch cmd {
		case 'M':
			x, y, err := t.pair()
			if err != nil {
				return nil, err
			}
			if rel && have {
				x += cx
				y += cy
			}
			cmds = append(cmds, PathCmd{Kind: CmdMove, X: x, Y: y})
			cx, cy, mx, my = x, y, x, y
			have, prevC = true, false
			// remaining pairs of M/m are L/l
			if t.last >= 'a' {
				cmd, t.last = 'L', 'l'
			} else {
				cmd = 'L'
			}
		case 'L':
			x, y, err := t.pair()
			if err != nil {
				return nil, err
			}
			if rel {
				x += cx
				y += cy
			}
			if !have {
				cmds = append(cmds, PathCmd{Kind: CmdMove, X: x, Y: y})
				mx, my = x, y
			} else {
				cmds = append(cmds, PathCmd{Kind: CmdLine, X: x, Y: y})
			}
			cx, cy = x, y
			have, prevC = true, false
		case 'H':
			x, err := t.num()
			if err != nil {
				return nil, err
			}
			if rel {
				x += cx
			}
			cmds = append(cmds, PathCmd{Kind: CmdLine, X: x, Y: cy})
			cx = x
			have, prevC = true, false
		case 'V':
			y, err := t.num()
			if err != nil {
				return nil, err
			}
			if rel {
				y += cy
			}
			cmds = append(cmds, PathCmd{Kind: CmdLine, X: cx, Y: y})
			cy = y
			have, prevC = true, false
		case 'C':
			x1, y1, err := t.pair()
			if err != nil {
				return nil, err
			}
			x2, y2, err := t.pair()
			if err != nil {
				return nil, err
			}
			x, y, err := t.pair()
			if err != nil {
				return nil, err
			}
			if rel {
				x1 += cx
				y1 += cy
				x2 += cx
				y2 += cy
				x += cx
				y += cy
			}
			cmds = append(cmds, PathCmd{Kind: CmdCubic, X: x, Y: y, X1: x1, Y1: y1, X2: x2, Y2: y2})
			cx, cy, sx, sy = x, y, x2, y2
			have, prevC = true, true
		case 'S':
			x2, y2, err := t.pair()
			if err != nil {
				return nil, err
			}
			x, y, err := t.pair()
			if err != nil {
				return nil, err
			}
			if rel {
				x2 += cx
				y2 += cy
				x += cx
				y += cy
			}
			x1, y1 := cx, cy
			if prevC {
				x1 = 2*cx - sx
				y1 = 2*cy - sy
			}
			cmds = append(cmds, PathCmd{Kind: CmdCubic, X: x, Y: y, X1: x1, Y1: y1, X2: x2, Y2: y2})
			cx, cy, sx, sy = x, y, x2, y2
			have, prevC = true, true
		case 'Z':
			cmds = append(cmds, PathCmd{Kind: CmdClose})
			cx, cy = mx, my
			prevC = false
		case 'Q', 'T', 'A':
			return nil, fmt.Errorf("parse: path command %c not supported", cmd)
		default:
			return nil, fmt.Errorf("parse: path command %c not supported", cmd)
		}
		if len(cmds) > maxPathCmds {
			return nil, fmt.Errorf("path: more than %d commands", maxPathCmds)
		}
	}
	return cmds, nil
}

type pathTok struct {
	s    string
	i    int
	last byte
}

func (t *pathTok) done() bool { return t.i >= len(t.s) }

func (t *pathTok) peek() byte {
	if t.done() {
		return 0
	}
	return t.s[t.i]
}

func (t *pathTok) next() byte {
	c := t.s[t.i]
	t.i++
	t.last = c
	return c
}

func (t *pathTok) skip() {
	for t.i < len(t.s) {
		c := t.s[t.i]
		if c == ',' || unicode.IsSpace(rune(c)) {
			t.i++
			continue
		}
		return
	}
}

func isPathCmd(c byte) bool {
	if c >= 'A' && c <= 'Z' {
		return strings.ContainsRune("MLHVCSQTAZ", rune(c))
	}
	if c >= 'a' && c <= 'z' {
		return strings.ContainsRune("mlhvcsqtaz", rune(c))
	}
	return false
}

func (t *pathTok) num() (float64, error) {
	t.skip()
	if t.done() {
		return 0, fmt.Errorf("parse: path number expected")
	}
	start := t.i
	if t.s[t.i] == '+' || t.s[t.i] == '-' {
		t.i++
	}
	seenDot, seenE := false, false
	for t.i < len(t.s) {
		c := t.s[t.i]
		if c >= '0' && c <= '9' {
			t.i++
			continue
		}
		if c == '.' && !seenDot && !seenE {
			seenDot = true
			t.i++
			continue
		}
		if (c == 'e' || c == 'E') && !seenE {
			seenE = true
			t.i++
			if t.i < len(t.s) && (t.s[t.i] == '+' || t.s[t.i] == '-') {
				t.i++
			}
			continue
		}
		break
	}
	if t.i == start || (t.i == start+1 && (t.s[start] == '+' || t.s[start] == '-')) {
		return 0, fmt.Errorf("parse: path number expected")
	}
	v, err := strconv.ParseFloat(t.s[start:t.i], 64)
	if err != nil {
		return 0, fmt.Errorf("parse: path number: %w", err)
	}
	return v, nil
}

func (t *pathTok) pair() (float64, float64, error) {
	x, err := t.num()
	if err != nil {
		return 0, 0, err
	}
	y, err := t.num()
	if err != nil {
		return 0, 0, err
	}
	return x, y, nil
}
