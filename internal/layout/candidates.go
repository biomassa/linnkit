package layout

import (
	"math"
	"slices"
	"sort"

	"github.com/biomassa/linnkit/internal/theory"
)

// Candidate is a scored layout suggestion with uniform rows.
type Candidate struct {
	Layout
	Tags       []string // e.g. "fourth", "generator", "no overlap"
	RowCents   float64  // mean size of the row interval
	RowRatio   string   // simplest ratio near the row interval, "" if none
	Periods    float64  // periods spanned by the surface
	Fits       bool     // every pad inside MIDI 0..127
	ChordSpan  float64  // mean columns spanned by major and minor triads; -1 if none found
	PeriodMove [2]int   // (columns, rows) from a pad to the same degree one period up
	Score      float64  // lower is better
}

// Options controls candidate generation.
type Options struct {
	Root     int     // MIDI note of degree 0
	ChordTol float64 // cents for finding chord notes in the scale; 0 = max(30, 0.6 x mean step)
}

// DefaultOptions returns the defaults: root on MIDI 60.
func DefaultOptions() Options { return Options{Root: 60} }

var (
	fifth  = 1200 * math.Log2(3.0/2)
	triads = [][]float64{
		{1200 * math.Log2(5.0/4), fifth}, // major
		{1200 * math.Log2(6.0/5), fifth}, // minor
	}
)

// Candidates returns every uniform row offset from 1 to 25 (25 = no overlap),
// scored and sorted with the best first. Balanced scoring: compact triads first,
// then at least 2.5 periods of range, then a short period move; layouts that do
// not fit in MIDI 0..127 rank last.
func Candidates(a *theory.Analysis, opt Options) []Candidate {
	st := a.Structure
	n := st.Size
	tol := opt.ChordTol
	if tol <= 0 {
		tol = math.Max(30, 0.6*st.EqualStep)
	}
	tags := offsetTags(a, tol)
	out := make([]Candidate, 0, Cols)
	for r := 1; r <= Cols; r++ {
		c := Candidate{Layout: Uniform(r, RootLow(r, opt.Root))}
		c.Fits = c.Layout.Fits()
		c.RowCents = float64(r) * st.EqualStep
		if rat, _, ok := theory.NearestRatio(c.RowCents, math.Max(10, 0.35*st.EqualStep)); ok {
			c.RowRatio = rat.Num().String() + "/" + rat.Denom().String()
		}
		c.Periods = float64(7*r+Cols-1) / float64(n)
		c.Tags = tags[r]
		if r == Cols {
			c.Tags = append(c.Tags, "no overlap")
		}
		c.ChordSpan = chordSpan(a, r, tol)
		dc, dr := Move(n, r)
		c.PeriodMove = [2]int{dc, dr}
		c.Score = score(c)
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score < out[j].Score })
	return out
}

func score(c Candidate) float64 {
	s := c.ChordSpan
	if s < 0 {
		s = 30
	}
	s += 3 * math.Max(0, 2.5-c.Periods)
	s += 0.5 * math.Abs(float64(c.PeriodMove[0]))
	if !c.Fits {
		s += 50
	}
	return s - 0.01*c.Periods
}

// offsetTags marks offsets that follow the scale's generator or its fifth/fourth.
func offsetTags(a *theory.Analysis, tol float64) map[int][]string {
	n := a.Structure.Size
	tags := map[int][]string{}
	add := func(r int, tag string) {
		if r >= 1 && r <= Cols {
			tags[r] = append(tags[r], tag)
		}
	}
	if g := a.Structure.GeneratorSteps; g > 0 {
		add(g, "generator")
		add(n-g, "generator complement")
	}
	if f, ok := StepsFor(a, 0, fifth, tol); ok && f < n {
		add(f, "fifth")
		add(n-f, "fourth")
	}
	return tags
}

// chordSpan averages, over every root degree, the columns spanned by the
// major and minor triad shapes that the scale can form there.
func chordSpan(a *theory.Analysis, r int, tol float64) float64 {
	n := len(a.Degrees)
	total, count := 0.0, 0
	for _, chord := range triads {
		for k := range n {
			cols := []int{0}
			ok := true
			for _, target := range chord {
				s, found := StepsFor(a, k, target, tol)
				if !found {
					ok = false
					break
				}
				dc, _ := Move(s, r)
				cols = append(cols, dc)
			}
			if ok {
				total += float64(slices.Max(cols) - slices.Min(cols))
				count++
			}
		}
	}
	if count == 0 {
		return -1
	}
	return total / float64(count)
}

// StepsFor returns how many degrees above degree k the interval closest to
// target cents lies, and whether it is within tol.
func StepsFor(a *theory.Analysis, k int, target, tol float64) (int, bool) {
	n := len(a.Degrees)
	best, bestErr := 0, math.Inf(1)
	for s := 1; s <= 2*n; s++ {
		if e := math.Abs(Interval(a, k, s) - target); e < bestErr {
			best, bestErr = s, e
		}
	}
	return best, bestErr <= tol
}

// Interval returns the cents from degree k up s degrees, crossing periods as needed.
func Interval(a *theory.Analysis, k, s int) float64 {
	n := len(a.Degrees)
	j := k + s
	return a.Degrees[j%n].Cents + float64(j/n)*a.PeriodCents - a.Degrees[k].Cents
}

// Move returns the pad move (columns, rows up) for an interval of s notes with
// row offset r: the shortest column move using 0 to 4 rows, fewer rows on ties.
func Move(s, r int) (int, int) {
	bestDC, bestDR := s, 0
	for dr := 1; dr <= 4; dr++ {
		if dc := s - dr*r; abs(dc) < abs(bestDC) {
			bestDC, bestDR = dc, dr
		}
	}
	return bestDC, bestDR
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
