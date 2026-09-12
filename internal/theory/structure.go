package theory

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Class is the structural class of a scale.
type Class int

const (
	Irregular Class = iota
	Equal
	NearEqual
	MOS
)

func (c Class) String() string {
	switch c {
	case Equal:
		return "equal"
	case NearEqual:
		return "near-equal"
	case MOS:
		return "MOS"
	default:
		return "irregular"
	}
}

// Structure describes the step pattern of a scale.
type Structure struct {
	Size           int
	PeriodCents    float64
	EqualStep      float64   // period / size: the step of the same-size EDO
	Steps          []float64 // degree k to k+1; the last ends at the period
	StepSizes      []float64 // distinct step sizes, ascending
	MaxDeviation   float64   // largest distance of a degree from the same-size EDO, cents
	Ascending      bool
	Class          Class
	Pattern        string  // for two step sizes that pass the MOS test: L/s word in file order
	GeneratorSteps int     // MOS generator in degrees (0 if not found)
	GeneratorCents float64 // MOS generator size, at most half the period
}

// Signature returns the MOS signature such as "5L2s", or "" without a pattern.
func (s Structure) Signature() string {
	if s.Pattern == "" {
		return ""
	}
	return fmt.Sprintf("%dL%ds", strings.Count(s.Pattern, "L"), strings.Count(s.Pattern, "s"))
}

func analyzeStructure(degs []Degree, period float64, opt Options) Structure {
	n := len(degs)
	st := Structure{Size: n, PeriodCents: period, EqualStep: period / float64(n), Ascending: true}
	st.Steps = make([]float64, n)
	for k, d := range degs {
		st.Steps[k] = d.Step
		if d.Step <= 0 {
			st.Ascending = false
		}
		if dev := math.Abs(d.Cents - float64(k)*st.EqualStep); dev > st.MaxDeviation {
			st.MaxDeviation = dev
		}
	}
	for _, g := range groups(st.Steps, opt.StepTol) {
		st.StepSizes = append(st.StepSizes, g.size)
	}
	switch {
	case !st.Ascending:
		st.Class = Irregular
	case len(st.StepSizes) == 1:
		st.Class = Equal
	default:
		if len(st.StepSizes) == 2 && checkMOS(&st, opt) && st.StepSizes[1]/st.StepSizes[0] >= opt.MOSMinRatio {
			st.Class = MOS
		} else if st.MaxDeviation <= opt.NearEqualFrac*st.EqualStep {
			st.Class = NearEqual
		} else {
			st.Class = Irregular
		}
	}
	return st
}

// checkMOS fills Pattern and the generator when the two-step-size scale has
// Myhill's property (every interval class comes in at most two sizes).
func checkMOS(st *Structure, opt Options) bool {
	n := st.Size
	small, large := st.StepSizes[0], st.StepSizes[1]
	for k := 1; k < n; k++ {
		if len(groups(classSizes(st.Steps, k), opt.StepTol*float64(k))) > 2 {
			return false
		}
	}
	word := make([]byte, n)
	for k, s := range st.Steps {
		if math.Abs(s-large) < math.Abs(s-small) {
			word[k] = 'L'
		} else {
			word[k] = 's'
		}
	}
	st.Pattern = string(word)
	// The generator's interval class has one size n-1 times.
	for k := 1; k < n; k++ {
		if gcd(k, n) != 1 {
			continue
		}
		for _, g := range groups(classSizes(st.Steps, k), opt.StepTol*float64(k)) {
			if g.count == n-1 && g.size <= st.PeriodCents/2+1e-9 {
				st.GeneratorSteps, st.GeneratorCents = k, g.size
				return true
			}
		}
	}
	return true
}

// classSizes returns the sizes of all k-step intervals, starting at each degree.
func classSizes(steps []float64, k int) []float64 {
	n := len(steps)
	out := make([]float64, n)
	for i := range n {
		for j := range k {
			out[i] += steps[(i+j)%n]
		}
	}
	return out
}

type group struct {
	size  float64 // mean of the members
	count int
}

// groups clusters values that lie within tol of the smallest value in the cluster.
func groups(values []float64, tol float64) []group {
	v := append([]float64(nil), values...)
	sort.Float64s(v)
	var out []group
	start, sum := 0, 0.0
	for i, x := range v {
		if i > start && x-v[start] > tol {
			out = append(out, group{sum / float64(i-start), i - start})
			start, sum = i, 0
		}
		sum += x
	}
	if len(v) > 0 {
		out = append(out, group{sum / float64(len(v)-start), len(v) - start})
	}
	return out
}

// StepClasses sorts steps into size classes the way groups does (a class
// holds the values within tol of its smallest member). It returns each step's
// class, 0 = the largest, and the number of classes.
func StepClasses(steps []float64, tol float64) ([]int, int) {
	v := append([]float64(nil), steps...)
	sort.Float64s(v)
	var lows []float64
	start := 0
	for i, x := range v {
		if i == 0 {
			lows = append(lows, x)
		} else if x-v[start] > tol {
			start = i
			lows = append(lows, x)
		}
	}
	out := make([]int, len(steps))
	for i, s := range steps {
		g := 0
		for j, lo := range lows {
			if s >= lo {
				g = j
			}
		}
		out[i] = len(lows) - 1 - g
	}
	return out, len(lows)
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
