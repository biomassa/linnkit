package synth

import (
	"math"
	"testing"
)

func equal(n int, period float64) []float64 {
	steps := make([]float64, n)
	for i := range steps {
		steps[i] = period / float64(n)
	}
	return steps
}

func profile(name string) Profile {
	for _, p := range Profiles {
		if p.Name == name {
			return p
		}
	}
	panic(name)
}

func TestPlan31EDO(t *testing.T) {
	p := PlanFor(12, equal(31, 1200), 1200)
	if p.LinnBend != 31 || math.Abs(p.Error) > 1e-9 {
		t.Errorf("31-EDO, S 12: %+v", p)
	}
	if p := Best(profile("Aalto / Kaivo"), equal(31, 1200), 1200); p.SynthBend != 12 || p.LinnBend != 31 {
		t.Errorf("Aalto best: %+v", p)
	}
}

func TestPlanLiveTuningCountsSteps(t *testing.T) {
	// Under Live's Tuning System, bend is in scale steps: B = S = 48 for 31-EDO
	// (in semitones it would need B = 124, past the LinnStrument's 96).
	for _, name := range []string{"Ableton Live built-ins", "Live tuning + MPE plugin"} {
		p := Best(profile(name), equal(31, 1200), 1200)
		if !p.InSteps || p.LinnBend != 48 || p.Error != 0 {
			t.Errorf("%s: %+v", name, p)
		}
	}
}

func TestPlanBohlenPierce(t *testing.T) {
	// 13 equal steps of the tritave, 146.3 c: Pigments can pick S and B freely.
	period := 1200 * math.Log2(3)
	p := Best(profile("Pigments"), equal(13, period), period)
	if math.Abs(p.Error) > 0.2 {
		t.Errorf("BP with Pigments: %+v", p)
	}
}

func TestPlanUnequalSteps(t *testing.T) {
	steps := []float64{111.73, 92.18, 90.22, 92.18, 111.73, 92.18, 19.55, 92.18, 111.73, 92.18, 90.22, 92.18, 111.73}
	p := PlanFor(12, steps, 1200)
	if p.LinnBend != 13 || p.WorstStep < 70 {
		t.Errorf("ji_13: %+v", p)
	}
}
