package search

// Op is one Search operator. Epoch.Operator holds this, not a name string.
type Op int

const (
	OpNone Op = iota
	OpAbsorb
	OpTriangle
	OpRing
	OpGrow
	OpCarve
	OpSlide
	OpBend
	OpSimplify
	OpWash
	OpJoin
	OpSubtract
	OpSwap
	OpDelete
	OpUnhole
	OpCount
)

var operatorNames = [OpCount]string{
	OpNone:     "",
	OpAbsorb:   "absorb",
	OpTriangle: "triangle",
	OpRing:     "ring",
	OpGrow:     "grow",
	OpCarve:    "carve",
	OpSlide:    "slide",
	OpBend:     "bend",
	OpSimplify: "simplify",
	OpWash:     "wash",
	OpJoin:     "join",
	OpSubtract: "subtract",
	OpSwap:     "swap",
	OpDelete:   "delete",
	OpUnhole:   "unhole",
}

func (id Op) String() string {
	if id < 0 || id >= OpCount {
		return ""
	}
	return operatorNames[id]
}
