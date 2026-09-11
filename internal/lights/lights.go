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

// rgb approximates the LED colors for terminal previews.
var rgb = map[Color][3]uint8{Off: {40, 40, 40}, Red: {224, 52, 47}, Yellow: {235, 215, 60}, Green: {63, 191, 74},
	Cyan: {60, 200, 220}, Blue: {59, 108, 240}, Magenta: {210, 63, 210}, White: {239, 239, 239},
	Orange: {245, 150, 40}, Lime: {170, 230, 60}, Pink: {245, 130, 190}}

// RGB returns an approximation of the LED color.
func (c Color) RGB() [3]uint8 {
	if v, ok := rgb[c]; ok {
		return v
	}
	return rgb[Off]
}

// Swatch is how one scale degree is shown.
type Swatch struct {
	Color Color
	Label string // short text for previews, up to 2 characters
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

// Plain renders the surface as text, top row first: "row 8  " then 3-character cells,
// "." for unlit pads.
func (s Surface) Plain() string {
	var b strings.Builder
	for row := layout.Rows - 1; row >= 0; row-- {
		fmt.Fprintf(&b, "row %d  ", row+1)
		for _, p := range s[row] {
			label := p.Label
			if p.Color == Off && label != "!!" {
				label = "."
			}
			fmt.Fprintf(&b, "%-3s", label)
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
			if p.Color == White || p.Color == Yellow || p.Color == Lime || p.Color == Green || p.Color == Cyan {
				fg = [3]uint8{20, 20, 20}
			}
			label := p.Label
			if p.Color == Off {
				label = ""
			}
			fmt.Fprintf(&top, "\x1b[48;2;%d;%d;%dm\x1b[38;2;%d;%d;%dm%-3s\x1b[0m ", c[0], c[1], c[2], fg[0], fg[1], fg[2], center(label, 3))
			fmt.Fprintf(&bottom, "\x1b[38;2;%d;%d;%dm▀▀▀\x1b[0m ", c[0], c[1], c[2])
		}
		b.WriteString(" " + top.String() + "\n " + bottom.String() + "\n")
	}
	return b.String()
}

func center(s string, w int) string {
	if len(s) >= w {
		return s
	}
	left := (w - len(s)) / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", w-len(s)-left)
}
