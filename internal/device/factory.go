package device

import "github.com/biomassa/linnkit/internal/layout"

// Setting is one NRPN value.
type Setting struct{ Param, Value int }

// FactoryLayout is the stock 12-TET layout: rows a fourth apart, the bottom
// row starting at F#1 (MIDI 30) (ls_handleTouches.ino 1950-1988).
var FactoryLayout = layout.Uniform(5, 30)

// FactoryNaturals and FactoryAccent are the note-light pattern 0 the
// LinnStrument ships with: C D E F G A B in the main color, C in the accent
// color (ls_settings.ino initializeNoteLights).
var (
	FactoryNaturals = [12]bool{true, false, true, false, true, true, false, true, false, true, false, true}
	FactoryAccent   = [12]bool{true}
)

// FactorySettings returns the NRPNs that restore the stock row layout and
// note lights: row offset 5, the stock Guitar tuning, no octave or transpose,
// note-light pattern 0 with its stock notes, and the stock main and accent
// colors (ls_settings.ino 496-503, 565, 632-663). MIDI settings are left alone.
// Order matters: NRPN 203-226 edit the active pattern, so 247 comes first.
func FactorySettings() []Setting {
	s := []Setting{{ParamNoteLights, 0}}
	for i := range 12 {
		s = append(s, Setting{203 + i, b2i(FactoryNaturals[i])}, Setting{215 + i, b2i(FactoryAccent[i])})
	}
	s = append(s, Setting{ParamRowOffset, 5})
	for row, n := range []int{30, 35, 40, 45, 50, 55, 59, 64} {
		s = append(s, Setting{ParamGuitarRow1 + row, n})
	}
	for _, base := range []int{0, RightSplit} {
		main := 3 // green on the left split
		if base == RightSplit {
			main = 5 // blue on the right
		}
		s = append(s, Setting{base + 36, 5}, Setting{base + 37, 7}, Setting{base + 38, 7},
			Setting{base + 30, main}, Setting{base + 31, 4})
	}
	return s
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
