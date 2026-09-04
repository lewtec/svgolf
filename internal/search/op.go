package search

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Op is one Search operator. Epoch.Operator holds this, not a name string.
type Op int

const (
	OpNone Op = iota
	OpAbsorb
	OpTriangle
	OpRing
	OpRectangle
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
	OpTriangle:  "triangle",
	OpRing:      "ring",
	OpRectangle: "rectangle",
	OpGrow:      "grow",
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

func ParseOp(s string) (Op, error) {
	for id := OpNone; id < OpCount; id++ {
		if operatorNames[id] == s {
			return id, nil
		}
	}
	return OpNone, fmt.Errorf("search: unknown operator %q", s)
}

func (id Op) MarshalJSON() ([]byte, error) {
	return json.Marshal(id.String())
}

func (id *Op) UnmarshalJSON(b []byte) error {
	if bytes.Equal(b, []byte("null")) {
		*id = OpNone
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if s == "" {
		*id = OpNone
		return nil
	}
	op, err := ParseOp(s)
	if err != nil {
		return err
	}
	*id = op
	return nil
}
