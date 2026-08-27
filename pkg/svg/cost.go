package svg

// Cost is the primitive rank. Dumb does not use it; a looping Search may.
func Cost(n Node) int {
	switch n.kind {
	case KindInvalid:
		return 0
	case KindGroup:
		sum := 0
		for _, c := range n.group.children {
			sum += Cost(c)
		}
		return sum
	case KindCircle:
		return 1
	case KindEllipse:
		return 2
	case KindRect:
		rx, ry := n.rect.clampedRadii()
		if rx == 0 && ry == 0 {
			return 1
		}
		return 2
	case KindPolygon:
		return 4
	default:
		return 0
	}
}
