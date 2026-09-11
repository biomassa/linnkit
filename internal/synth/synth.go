// Package synth describes synths that play the LinnStrument's notes: their
// per-note pitch-bend options, tuning support and quirks. It also computes the
// LinnStrument Bend Range that makes one pad of slide equal one scale step.
package synth

import "math"

// Tri is yes, no or unknown.
type Tri int

const (
	Unknown Tri = iota
	Yes
	No
)

func (t Tri) String() string {
	switch t {
	case Yes:
		return "yes"
	case No:
		return "no"
	}
	return "unknown"
}

// Profile is what the app knows about one synth.
type Profile struct {
	Name        string
	MPE         bool  // per-note pitch bend (false: one channel, one bend for all notes)
	BendValues  []int // allowed synth bend ranges in semitones; nil = BendMin..BendMax
	BendMin     int
	BendMax     int
	DefaultBend int
	// ObeysRPN tells whether the synth takes the bend range the LinnStrument
	// sends in MPE state. If it does, or if unknown, keep the LinnStrument out of
	// MPE state (the send does: plain Channel Per Note).
	ObeysRPN Tri
	Tuning   string // how the synth takes a tuning
	Verified bool   // checked on this machine or in the synth's source
	Notes    []string
}

// Profiles are the built-in synths. Facts: reference/linnstrument-facts.md.
var Profiles = []Profile{
	{
		Name: "Aalto / Kaivo", MPE: true, BendValues: []int{12, 24, 48, 96}, DefaultBend: 12, ObeysRPN: No,
		Tuning: ".scl + .kbm listing every degree, in ~/Music/Madrona Labs/Scales", Verified: true,
		Notes: []string{"gear menu: Input protocol = MIDI MPE, MPE bend range", "KEY bend knob is channel bend, not per-note"},
	},
	{
		Name: "Pigments", MPE: true, BendMin: 2, BendMax: 96, DefaultBend: 12, ObeysRPN: Unknown,
		Tuning: ".scl only; set Reference Note C3 (= MIDI 60) when loading", Verified: true,
		Notes: []string{"Settings: Enable MPE, Nb Channels 15, Bend Range", "tuning lives in plugin state, not in presets"},
	},
	{
		Name: "Surge XT", MPE: true, BendMin: 1, BendMax: 96, DefaultBend: 12, ObeysRPN: Unknown,
		Tuning: ".scl + .kbm (Menu > Tuning), or MTS-ESP",
		Notes:  []string{"Menu > MPE: enable, default pitch bend range", "not yet checked on this machine"},
	},
	{
		Name: "Plasmonic", MPE: true, BendMin: 1, BendMax: 96, DefaultBend: 48, ObeysRPN: Unknown,
		Tuning: "MTS-ESP (no MTS-ESP master installed); .scl support unknown",
		Notes:  []string{"bend range options not yet checked"},
	},
	{
		Name: "Cypher2", MPE: true, BendMin: 48, BendMax: 48, DefaultBend: 48, ObeysRPN: Unknown,
		Tuning: ".tun files only (not .scl)",
		Notes:  []string{"MPE range assumed 48; its bend settings are for non-MPE input"},
	},
	{
		Name: "Ableton Live built-ins", MPE: true, BendMin: 48, BendMax: 48, DefaultBend: 48, ObeysRPN: Unknown,
		Tuning: "Live 12 Tuning System: drop the .scl into Live's browser (Tunings)",
		Notes:  []string{"Live needs 48 semitones per-note bend with tuning systems", "Wavetable, Meld, Drift, Sampler take MPE"},
	},
	{
		Name: "Bitwig built-ins / Grid", MPE: true, BendMin: 1, BendMax: 96, DefaultBend: 48, ObeysRPN: Unknown,
		Tuning: "Micro-pitch device: .scl of up to 12 notes only",
		Notes:  []string{"MPE bend range per device, in the Inspector", "over 12 notes: use a tuning-aware plugin"},
	},
	{
		Name: "Legacy (non-MPE)", MPE: false, BendMin: 1, BendMax: 24, DefaultBend: 2, ObeysRPN: No,
		Tuning: "depends on the synth",
		Notes:  []string{"one channel: slides bend every sounding note", "the LinnStrument is set to One Channel mode"},
	},
	{
		Name: "Generic MPE", MPE: true, BendMin: 1, BendMax: 96, DefaultBend: 12, ObeysRPN: Unknown,
		Tuning: "set in the synth", Notes: []string{"enter the synth's per-note bend range with < >"},
	},
}

// Allowed returns the synth bend ranges the profile allows.
func (p Profile) Allowed() []int {
	if p.BendValues != nil {
		return p.BendValues
	}
	var out []int
	for s := p.BendMin; s <= p.BendMax; s++ {
		out = append(out, s)
	}
	return out
}

// Plan is the pitch-bend setup for one scale and one synth bend range.
type Plan struct {
	SynthBend int     // S: the synth's per-note bend range, semitones
	LinnBend  int     // B: the LinnStrument Bend Range, 1-96
	PadCents  float64 // one pad of slide: 100*S/B cents
	Target    float64 // what one pad should be: period / notes (the step, or the average step)
	Error     float64 // PadCents - Target
	WorstStep float64 // largest |PadCents - step| over the scale's steps (0 for equal scales)
}

// PlanFor returns the LinnStrument Bend Range for synth bend s that makes one
// pad of slide closest to the average step of a scale with these steps.
func PlanFor(s int, steps []float64, period float64) Plan {
	target := period / float64(len(steps))
	b := int(math.Round(100 * float64(s) / target))
	b = max(1, min(96, b))
	p := Plan{SynthBend: s, LinnBend: b, PadCents: 100 * float64(s) / float64(b), Target: target}
	p.Error = p.PadCents - target
	for _, st := range steps {
		p.WorstStep = math.Max(p.WorstStep, math.Abs(p.PadCents-st))
	}
	return p
}

// Best returns the plan with the smallest error among the synth bend ranges the
// profile allows; ties go to the range closest to the profile's default.
func Best(p Profile, steps []float64, period float64) Plan {
	var best Plan
	for i, s := range p.Allowed() {
		c := PlanFor(s, steps, period)
		better := i == 0 || math.Abs(c.Error) < math.Abs(best.Error)-1e-9 ||
			(math.Abs(math.Abs(c.Error)-math.Abs(best.Error)) <= 1e-9 && abs(s-p.DefaultBend) < abs(best.SynthBend-p.DefaultBend))
		if better {
			best = c
		}
	}
	return best
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
