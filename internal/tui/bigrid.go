package tui

import (
	"fmt"
	"strings"

	"github.com/biomassa/linnkit/internal/layout"
	"github.com/biomassa/linnkit/internal/lights"
)

const bigCtlW = 4 // control column: " PS " like the dashboard

// padSize returns the largest square pad that fits a w x h window 25 x 8
// times: bw columns by bh lines, with a terminal cell about twice as tall as
// wide (so bw = 2 x bh), plus a 1-column and 1-line gap.
func padSize(w, h int) (bw, bh int) {
	pitchW := (w - 1 - bigCtlW) / layout.Cols
	pitchH := (h - 3) / layout.Rows
	bw, bh = max(2, pitchW-1), max(1, pitchH-1)
	if bw > 2*bh {
		bw = 2 * bh
	} else {
		bh = max(1, (bw+1)/2)
	}
	return bw, bh
}

// bigGrid draws the pads large and square in the middle of the window (key
// g): each a block of its colour with the label centred and the scale degree
// under it. Labels are cut only when a pad is too narrow.
func (m Model) bigGrid() string {
	w, h := m.width, m.height
	bw, bh := padSize(w, h)
	labelLine := (bh - 1) / 2
	degLine := labelLine + 1
	gridW := 1 + bigCtlW + layout.Cols*(bw+1)
	gridH := layout.Rows * (bh + 1)
	left := strings.Repeat(" ", max(0, (w-gridW)/2))

	l := m.shown()
	lo, hi := l.Span()
	name := schemeTitles[m.scheme]
	if m.factory {
		name = "factory 12-TET"
	}
	title := fmt.Sprintf(" GRID  %s   rows +%d, bottom-left MIDI %d, notes %d..%d", name, l.Offset, l.RowStart[0], lo, hi)
	out := []string{boldStyle.Render(fit(title, w))}
	for range max(0, (h-3-gridH)/2) {
		out = append(out, strings.Repeat(" ", w))
	}

	ctl := [3]uint8{52, 52, 52}
	for row := layout.Rows - 1; row >= 0; row-- {
		for line := range bh + 1 {
			if line == bh { // gap between pad rows
				out = append(out, strings.Repeat(" ", w))
				continue
			}
			var b strings.Builder
			b.WriteString(left + " ")
			text := ""
			if line == labelLine {
				text = controlLabels[layout.Rows-1-row]
			}
			b.WriteString(rgbBG(ctl) + rgbFG([3]uint8{150, 150, 150}) + center(text, bigCtlW-1) + "\x1b[0m ")
			for _, p := range m.surface[row] {
				text = ""
				switch {
				case p.Degree < 0: // outside MIDI 0..127: dark, no text
				case line == labelLine:
					text = p.Label
				case line == degLine:
					text = fmt.Sprintf("d%d", p.Degree)
				}
				if len(text) > bw {
					text = text[:bw]
				}
				fg := textOn(p.Color)
				switch {
				case p.Color == lights.Off && line == degLine:
					fg = [3]uint8{175, 175, 175}
				case p.Color == lights.Off:
					fg = [3]uint8{235, 235, 235}
				case line == degLine:
					fg = mix(fg, p.Color.RGB())
				}
				b.WriteString(rgbBG(p.Color.RGB()) + rgbFG(fg) + center(text, bw) + "\x1b[0m ")
			}
			out = append(out, fit(b.String(), w))
		}
	}
	for len(out) < h-2 {
		out = append(out, strings.Repeat(" ", w))
	}
	out = append(out[:h-2], fit(" "+m.legend, w), dimStyle.Render(fit(" pads: label, then scale degree (d)   g or Esc: back to the dashboard", w)))
	return strings.Join(out, "\n")
}

// mix blends the text colour halfway to the pad colour, for the quieter degree line.
func mix(a, b [3]uint8) [3]uint8 {
	var out [3]uint8
	for i := range out {
		out[i] = uint8((int(a[i]) + int(b[i])) / 2)
	}
	return out
}
