package theory

import (
	"math/big"

	"github.com/biomassa/linnkit/internal/scala"
)

// Options tunes the analysis. DefaultOptions gives the values used by the app.
type Options struct {
	StepTol       float64 // cents: steps closer than this count as the same size
	NearEqualFrac float64 // near-equal if every degree is within this fraction of a step from the same-size EDO
	MOSMinRatio   float64 // a two-step-size scale counts as MOS only if large/small is at least this
	JITol         float64 // cents for nearest-ratio matches; 0 = min(15, 0.35 x mean step)
	JIMaxPrime    int     // highest prime in candidate ratios
	JIMaxOdd      int64   // highest odd limit of candidate ratios
}

// DefaultOptions returns the default analysis settings.
func DefaultOptions() Options {
	return Options{StepTol: 0.5, NearEqualFrac: 0.15, MOSMinRatio: 1.05, JIMaxPrime: 23, JIMaxOdd: 63}
}

// Degree is one scale degree; degree 0 is 1/1.
type Degree struct {
	Index int
	Cents float64
	Ratio *big.Rat // nil when the file gave cents
	Step  float64  // cents to the next degree; the last step ends at the period
}

// Analysis is everything the app shows about a scale.
type Analysis struct {
	Scale       *scala.Scale
	Degrees     []Degree // degrees 0..n-1
	PeriodCents float64
	PeriodRatio *big.Rat // nil when the period was given in cents
	Structure   Structure
	JI          JISummary
}

// Analyze computes the structure and just-intonation analysis of a scale.
func Analyze(s *scala.Scale, opt Options) *Analysis {
	n := s.Size()
	period := s.Period()
	a := &Analysis{Scale: s, PeriodCents: period.Cents, PeriodRatio: period.Ratio}
	a.Degrees = make([]Degree, n)
	a.Degrees[0] = Degree{Index: 0, Cents: 0, Ratio: big.NewRat(1, 1)}
	for k := 1; k < n; k++ {
		p := s.Pitches[k-1]
		a.Degrees[k] = Degree{Index: k, Cents: p.Cents, Ratio: p.Ratio}
	}
	for k := range a.Degrees {
		next := a.PeriodCents
		if k+1 < n {
			next = a.Degrees[k+1].Cents
		}
		a.Degrees[k].Step = next - a.Degrees[k].Cents
	}
	a.Structure = analyzeStructure(a.Degrees, a.PeriodCents, opt)
	if opt.JITol <= 0 {
		opt.JITol = min(15, 0.35*a.Structure.EqualStep)
	}
	a.JI = analyzeJI(a, opt)
	return a
}

// PitchKinds describes how the file writes its pitches: "ratios", "cents" or "mixed".
func (a *Analysis) PitchKinds() string {
	ratios, cents := 0, 0
	for _, p := range a.Scale.Pitches {
		if p.IsRatio() {
			ratios++
		} else {
			cents++
		}
	}
	switch {
	case cents == 0:
		return "ratios"
	case ratios == 0:
		return "cents"
	default:
		return "mixed"
	}
}
