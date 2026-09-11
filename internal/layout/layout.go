package layout

const (
	Cols = 25 // playing columns (the control column is not part of the layout)
	Rows = 8
)

// Layout assigns a MIDI note to every pad: note = RowStart[row] + col - 1,
// with col 1..25 and row 0 nearest the player. Columns always step by one note.
type Layout struct {
	Offset   int // notes (scale degrees) between rows; 0 when rows are not uniform
	RowStart [Rows]int
}

// Uniform returns rows that are offset notes apart, starting at low.
func Uniform(offset, low int) Layout {
	l := Layout{Offset: offset}
	for r := range Rows {
		l.RowStart[r] = low + r*offset
	}
	return l
}

// Note returns the MIDI note of the pad at col (1..25) and row (0..7).
func (l Layout) Note(col, row int) int { return l.RowStart[row] + col - 1 }

// Span returns the lowest and highest note on the surface.
func (l Layout) Span() (lo, hi int) {
	lo, hi = l.Note(1, 0), l.Note(Cols, 0)
	for r := range Rows {
		lo = min(lo, l.Note(1, r))
		hi = max(hi, l.Note(Cols, r))
	}
	return lo, hi
}

// Fits reports whether every pad sends a note in MIDI 0..127.
func (l Layout) Fits() bool {
	lo, hi := l.Span()
	return lo >= 0 && hi <= 127
}

// RootLow returns the bottom-left note that puts root on row 4 (index 3), column 1,
// moved to fit inside MIDI 0..127 when the surface is small enough.
func RootLow(offset, root int) int {
	low := root - 3*offset
	span := 7*offset + Cols - 1
	if low+span > 127 {
		low = 127 - span
	}
	return max(low, 0)
}
