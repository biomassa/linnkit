package theory

import (
	"fmt"
	"math"
	"math/big"
)

// Interval is the distance from one degree up to another.
type Interval struct {
	Cents float64
	Ratio *big.Rat // exact when both degrees (and the period, if crossed) are ratios
}

// IntervalMatrix returns m[i][j], the interval from degree i up to degree j.
// When j < i the interval crosses the period.
func (a *Analysis) IntervalMatrix() [][]Interval {
	n := len(a.Degrees)
	m := make([][]Interval, n)
	for i, di := range a.Degrees {
		m[i] = make([]Interval, n)
		for j, dj := range a.Degrees {
			iv := Interval{Cents: dj.Cents - di.Cents}
			wrap := j < i
			if wrap {
				iv.Cents += a.PeriodCents
			}
			if di.Ratio != nil && dj.Ratio != nil && (!wrap || a.PeriodRatio != nil) {
				iv.Ratio = new(big.Rat).Quo(dj.Ratio, di.Ratio)
				if wrap {
					iv.Ratio.Mul(iv.Ratio, a.PeriodRatio)
				}
			}
			m[i][j] = iv
		}
	}
	return m
}

var noteNames = [12]string{"C", "C#", "D", "Eb", "E", "F", "F#", "G", "Ab", "A", "Bb", "B"}

// Nearest12 returns the nearest 12-TET note to a pitch above the root (root = C),
// the number of octaves above the root, and the offset in cents.
func Nearest12(cents float64) (name string, octave int, offset float64) {
	n := int(math.Round(cents / 100))
	pc := ((n % 12) + 12) % 12
	octave = (n - pc) / 12
	return noteNames[pc], octave, cents - float64(n)*100
}

// Format12 renders Nearest12 as for example "E -13.7" or "C +0.0 (+1 oct)".
func Format12(cents float64) string {
	name, oct, off := Nearest12(cents)
	s := fmt.Sprintf("%-2s %+5.1f", name, off)
	if oct != 0 {
		s += fmt.Sprintf(" (%+d oct)", oct)
	}
	return s
}
