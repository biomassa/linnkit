package lights

import (
	"math"
	"math/big"
	"sort"
	"strconv"

	"github.com/biomassa/linnkit/internal/theory"
)

// RootOnly colors the root and leaves every other degree off.
func RootOnly(n int, root Color) []Swatch {
	sw := make([]Swatch, n)
	sw[0] = Swatch{root, "R"}
	return sw
}

// JIFamilyColors are the default colors per prime limit; 1 is the root.
var JIFamilyColors = map[int]Color{1: Magenta, 3: White, 5: Green, 7: Blue, 11: Orange, 13: Yellow}

// JIFamilies colors each degree by the prime limit of its ratio from the root.
// Degrees written as ratios use their own limit (off above limit). Degrees in
// cents take the simplest ratio (lowest prime limit, then smallest n*d) whose
// nearest degree is within tol cents; tol 0 = min(15, 0.35 x mean step).
// This matches reference/linnstrument_edo_just.py for EDOs.
func JIFamilies(a *theory.Analysis, limit int, tol float64) []Swatch {
	n := len(a.Degrees)
	if tol <= 0 {
		tol = math.Min(15, 0.35*a.Structure.EqualStep)
	}
	family := make([]int, n) // 0 = undecided, -1 = off
	family[0] = 1
	for d := 1; d < n; d++ {
		if r := a.Degrees[d].Ratio; r != nil {
			if pl := theory.PrimeLimit(r); pl >= 3 && pl <= limit {
				family[d] = pl
			} else {
				family[d] = -1
			}
		}
	}
	for _, jr := range justRatios(limit, 400) {
		cents := 1200 * math.Log2(float64(jr.n)/float64(jr.d))
		if cents >= a.PeriodCents {
			continue
		}
		d, err := nearestDegree(a, cents)
		if d > 0 && math.Abs(err) <= tol && family[d] == 0 {
			family[d] = jr.limit
		}
	}
	sw := make([]Swatch, n)
	for d, f := range family {
		switch {
		case f == 1:
			sw[d] = Swatch{JIFamilyColors[1], "R"}
		case f > 1:
			sw[d] = Swatch{JIFamilyColors[f], strconv.Itoa(f)}
		}
	}
	return sw
}

type justRatio struct {
	n, d  int64
	limit int
}

// justRatios lists n/d inside the octave with d <= 32, n*d <= maxHeight and
// prime limit 3..limit, sorted by limit and then n*d (stable in generation order).
func justRatios(limit int, maxHeight int64) []justRatio {
	var out []justRatio
	for d := int64(1); d <= 32; d++ {
		for n := d + 1; n < 2*d; n++ {
			if gcd(n, d) != 1 || n*d > maxHeight {
				continue
			}
			if pl := theory.PrimeLimit(big.NewRat(n, d)); pl >= 3 && pl <= limit {
				out = append(out, justRatio{n, d, pl})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].limit != out[j].limit {
			return out[i].limit < out[j].limit
		}
		return out[i].n*out[i].d < out[j].n*out[j].d
	})
	return out
}

// nearestDegree returns the degree closest to a pitch within the period and the
// error in cents (degree minus pitch).
func nearestDegree(a *theory.Analysis, cents float64) (int, float64) {
	best, bestErr := 0, math.Inf(1)
	for d, deg := range a.Degrees {
		if e := deg.Cents - cents; math.Abs(e) < math.Abs(bestErr) {
			best, bestErr = d, e
		}
	}
	return best, bestErr
}

// MOSChoice picks a MOS inside a scale: Size notes along a chain of
// Generator-degree steps, starting Down generators below the root.
type MOSChoice struct {
	Generator, Size, Down int
}

// Degrees returns the degrees of the MOS, sorted.
func (m MOSChoice) Degrees(n int) []int {
	seen := map[int]bool{}
	var out []int
	for k := -m.Down; k < m.Size-m.Down; k++ {
		d := ((k*m.Generator)%n + n) % n
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	sort.Ints(out)
	return out
}

// DefaultMOS returns a diatonic-like choice: the degree nearest 3/2 as the
// generator, 7 notes, one generator below the root. If 7 notes do not form a
// MOS it tries sizes 5 to 12. ok is false when nothing fits.
func DefaultMOS(a *theory.Analysis) (MOSChoice, bool) {
	n := len(a.Degrees)
	g, _ := nearestDegree(a, 1200*math.Log2(1.5))
	if g <= 0 || n < 5 {
		return MOSChoice{}, false
	}
	sizes := []int{7, 5, 6, 8, 9, 10, 11, 12}
	for _, size := range sizes {
		m := MOSChoice{Generator: g, Size: size, Down: 1}
		if size < n && isMOS(m.Degrees(n), n) {
			return m, true
		}
	}
	return MOSChoice{}, false
}

// isMOS reports whether a set of degrees has exactly two step sizes (counted in degrees).
func isMOS(degs []int, n int) bool {
	if len(degs) < 2 {
		return false
	}
	sizes := map[int]bool{}
	for i, d := range degs {
		next := n
		if i+1 < len(degs) {
			next = degs[i+1]
		}
		sizes[next-d] = true
	}
	return len(sizes) == 2
}

// MOSPattern colors the root, the MOS degrees and the remaining degrees.
func MOSPattern(n int, m MOSChoice, root, in, rest Color) []Swatch {
	sw := make([]Swatch, n)
	for d := range sw {
		sw[d] = Swatch{rest, "x"}
	}
	for _, d := range m.Degrees(n) {
		sw[d] = Swatch{in, "o"}
	}
	sw[0] = Swatch{root, "R"}
	return sw
}

func gcd(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
