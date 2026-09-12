package lights

import (
	"fmt"
	"strings"

	"github.com/biomassa/linnkit/internal/layout"
)

// Color is a LinnStrument LED color, as sent in CC22.
type Color uint8

const (
	Off     Color = 0
	Red     Color = 1
	Yellow  Color = 2
	Green   Color = 3
	Cyan    Color = 4
	Blue    Color = 5
	Magenta Color = 6
	White   Color = 8
	Orange  Color = 9
	Lime    Color = 10
	Pink    Color = 11
)

// Palette is every color a pad can be lit in (CC22 1-11; 7 is black, i.e. off).
var Palette = []Color{Red, Yellow, Green, Cyan, Blue, Magenta, White, Orange, Lime, Pink}

var colorNames = map[Color]string{Off: "off", Red: "red", Yellow: "yellow", Green: "green", Cyan: "cyan",
	Blue: "blue", Magenta: "magenta", White: "white", Orange: "orange", Lime: "lime", Pink: "pink"}

func (c Color) String() string {
	if s, ok := colorNames[c]; ok {
		return s
	}
	return fmt.Sprintf("color(%d)", uint8(c))
}

// ParseColor returns the color with the given name.
func ParseColor(s string) (Color, error) {
	for c, name := range colorNames {
		if strings.EqualFold(s, name) {
			return c, nil
		}
	}
	return Off, fmt.Errorf("unknown color %q", s)
}

// duty is each color's red, green and blue on-time, as the firmware drives the
// LEDs (ls_leds.ino 340-403): every channel is only on or off, and white,
// orange, lime and pink alternate between two base colors on every other
// refresh (white: R+G+B / G+B, orange: R+G / R, lime: R+G / G, pink: R+B / R+G).
// By this math white is cyan-tinted and pink is salmon; the closest pairs are
// white/cyan, lime/yellow and orange/yellow (not yet checked by eye).
var duty = map[Color][3]float64{
	Red: {1, 0, 0}, Yellow: {1, 1, 0}, Green: {0, 1, 0}, Cyan: {0, 1, 1}, Blue: {0, 0, 1}, Magenta: {1, 0, 1},
	White: {0.5, 1, 1}, Orange: {1, 0.5, 0}, Lime: {0.5, 1, 0}, Pink: {1, 0.5, 0.5},
}

// RGB returns the color for terminal previews, from the firmware's channel mix.
// Unlit pads show as dark grey.
func (c Color) RGB() [3]uint8 {
	d, ok := duty[c]
	if !ok {
		return [3]uint8{40, 40, 40}
	}
	var out [3]uint8
	for i, v := range d {
		out[i] = uint8(20 + 235*v)
	}
	return out
}

// Light reports whether preview text on this color should be dark.
func (c Color) Light() bool {
	switch c {
	case White, Yellow, Lime, Green, Cyan, Orange, Pink:
		return true
	}
	return false
}

// Swatch is how one scale degree is shown.
type Swatch struct {
	Color Color
	Label string // short text for previews; the text grid widens its cells for longer ones
}

// Pad is what one pad plays and shows.
type Pad struct {
	Note   int
	Degree int // -1 when the note is outside MIDI 0..127
	Swatch
}

// Surface is a painted layout: Surface[row][col-1], row 0 nearest the player.
type Surface [layout.Rows][layout.Cols]Pad

// Paint shows every pad in the swatch of the degree it plays. Degree 0 sits on
// MIDI note root; sw has one swatch per degree.
func Paint(l layout.Layout, root int, sw []Swatch) Surface {
	n := len(sw)
	var s Surface
	for row := range layout.Rows {
		for col := 1; col <= layout.Cols; col++ {
			note := l.Note(col, row)
			if note < 0 || note > 127 {
				s[row][col-1] = Pad{Note: note, Degree: -1, Swatch: Swatch{Off, "!!"}}
				continue
			}
			d := ((note-root)%n + n) % n
			s[row][col-1] = Pad{Note: note, Degree: d, Swatch: sw[d]}
		}
	}
	return s
}

// Plain renders the surface as text, top row first: "row 8  " then one cell
// per pad, as wide as the longest label plus a space and at least 3. A pad
// shows its label; an unlit pad without one shows ".".
func (s Surface) Plain() string {
	cell := func(p Pad) string {
		if p.Label == "" && p.Color == Off {
			return "."
		}
		return p.Label
	}
	w := 3
	for _, row := range s {
		for _, p := range row {
			w = max(w, len(cell(p))+1)
		}
	}
	var b strings.Builder
	for row := layout.Rows - 1; row >= 0; row-- {
		fmt.Fprintf(&b, "row %d  ", row+1)
		for _, p := range s[row] {
			fmt.Fprintf(&b, "%-*s", w, cell(p))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// CC renders each pad's CC22 colour number, top row first, like Plain.
func (s Surface) CC() string {
	var b strings.Builder
	for row := layout.Rows - 1; row >= 0; row-- {
		fmt.Fprintf(&b, "row %d  ", row+1)
		for _, p := range s[row] {
			fmt.Fprintf(&b, "%-3d", int(p.Color))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// ANSI renders the surface with 24-bit terminal colors, 4 x 2 cells per pad.
func (s Surface) ANSI() string {
	var b strings.Builder
	for row := layout.Rows - 1; row >= 0; row-- {
		var top, bottom strings.Builder
		for _, p := range s[row] {
			c := p.Color.RGB()
			fg := [3]uint8{255, 255, 255}
			if p.Color.Light() {
				fg = [3]uint8{20, 20, 20}
			}
			label := p.Label
			if label == "!!" { // outside MIDI 0..127: dark; unlit pads keep other labels
				label = ""
			}
			label = Short(label)
			fmt.Fprintf(&top, "\x1b[48;2;%d;%d;%dm\x1b[38;2;%d;%d;%dm%-3s\x1b[0m ", c[0], c[1], c[2], fg[0], fg[1], fg[2], center(label, 3))
			fmt.Fprintf(&bottom, "\x1b[38;2;%d;%d;%dm▀▀▀\x1b[0m ", c[0], c[1], c[2])
		}
		b.WriteString(" " + top.String() + "\n " + bottom.String() + "\n")
	}
	return b.String()
}

// Short cuts a label to the 3 characters a colour preview cell holds.
func Short(label string) string {
	if len(label) > 3 {
		return label[:3]
	}
	return label
}

func center(s string, w int) string {
	if len(s) >= w {
		return s
	}
	left := (w - len(s)) / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", w-len(s)-left)
}
