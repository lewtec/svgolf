package stack

import (
	"fmt"
	"image/color"
	"sort"
	"strconv"
	"strings"

	"github.com/lewtec/svgolf/internal/loss"
)

// knob is the stack search's exposed parameters. A zero knob is the
// built-in behavior. Tune sets ready and only the keys the caller passed.
type knob struct {
	ready          bool
	epochs         int
	beam           int
	beamSet        bool
	commandCost    float64
	commandCostSet bool
	minIsland      int
	minIslandSet   bool
	minErr         float64
	minErrSet      bool
}

func (k knob) epochLimit() int {
	if !k.ready {
		return 0
	}
	return k.epochs
}

func (k knob) beamWidth() int {
	if !k.ready || !k.beamSet {
		return beamWidth
	}
	return k.beam
}

func (k knob) cost() float64 {
	if !k.ready || !k.commandCostSet {
		return commandCost
	}
	return k.commandCost
}

func (k knob) islandMin() int {
	if !k.ready || !k.minIslandSet {
		return minIsland
	}
	return k.minIsland
}

func (k knob) errMin() float64 {
	if !k.ready || !k.minErrSet {
		return float64(minErr)
	}
	return k.minErr
}

func (s *world) islandMin() int {
	if s == nil {
		return minIsland
	}
	return s.knob.islandMin()
}

func (s *world) errLimit() float64 {
	if s == nil {
		return float64(minErr)
	}
	return s.knob.errMin()
}

func (s *world) paperColor(col color.NRGBA) bool {
	return loss.ColorAt(col, paper) <= s.errLimit()
}

func (s *world) better(errSum float64, commands int, parentErr float64, parentCommands int) bool {
	cost := float64(commandCost)
	if s != nil {
		cost = s.knob.cost()
	}
	return objectiveCost(errSum, commands, cost) < objectiveCost(parentErr, parentCommands, cost)
}

var knobKeys = []string{"beam", "command-cost", "epochs", "minimum-error", "minimum-island"}

// Tune sets stack knobs. Unknown keys are an error. Unset keys keep
// the built-in values, so an empty map does not change the search.
func (st *Stack) Tune(opts map[string]string) error {
	if st == nil {
		return fmt.Errorf("search: nil stack")
	}
	k := st.knob
	k.ready = true
	for key, val := range opts {
		switch key {
		case "epochs":
			n, err := nonNegativeInt(key, val)
			if err != nil {
				return err
			}
			k.epochs = n
		case "beam":
			n, err := positiveInt(key, val)
			if err != nil {
				return err
			}
			k.beam = n
			k.beamSet = true
		case "command-cost":
			n, err := nonNegativeFloat(key, val)
			if err != nil {
				return err
			}
			k.commandCost = n
			k.commandCostSet = true
		case "minimum-island":
			n, err := nonNegativeInt(key, val)
			if err != nil {
				return err
			}
			k.minIsland = n
			k.minIslandSet = true
		case "minimum-error":
			n, err := nonNegativeFloat(key, val)
			if err != nil {
				return err
			}
			k.minErr = n
			k.minErrSet = true
		default:
			known := append([]string(nil), knobKeys...)
			sort.Strings(known)
			return fmt.Errorf("search: unknown option %q (have %s)", key, strings.Join(known, ", "))
		}
	}
	st.knob = k
	return nil
}

func nonNegativeInt(key, val string) (int, error) {
	n, err := strconv.Atoi(val)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("search: %s=%q want a non-negative integer", key, val)
	}
	return n, nil
}

func positiveInt(key, val string) (int, error) {
	n, err := strconv.Atoi(val)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("search: %s=%q want an integer >= 1", key, val)
	}
	return n, nil
}

func nonNegativeFloat(key, val string) (float64, error) {
	n, err := strconv.ParseFloat(val, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("search: %s=%q want a non-negative number", key, val)
	}
	return n, nil
}
