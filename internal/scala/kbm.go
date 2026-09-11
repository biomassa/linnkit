package scala

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

// Mapping is a Scala .kbm keyboard mapping.
type Mapping struct {
	Size         int     // number of map entries; 0 means linear
	FirstNote    int     // first MIDI note to retune
	LastNote     int     // last MIDI note to retune
	MiddleNote   int     // MIDI note where the first map entry sits
	RefNote      int     // MIDI note with the reference frequency
	RefFreq      float64 // Hz
	FormalOctave int     // scale degree used as the octave
	Keys         []int   // scale degree per map entry; -1 = unmapped ("x")
	Warnings     []Warning
}

// ParseKBMFile reads and parses a .kbm file.
func ParseKBMFile(path string) (*Mapping, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseKBM(data)
}

// ParseKBM parses the contents of a .kbm file.
func ParseKBM(data []byte) (*Mapping, error) {
	type value struct {
		text string
		line int
	}
	var values []value
	for i, line := range splitLines(data) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "!") {
			continue
		}
		values = append(values, value{strings.Fields(trimmed)[0], i + 1})
	}
	if len(values) == 0 {
		return nil, ErrEmpty
	}
	if len(values) < 7 {
		return nil, fmt.Errorf("expected 7 header values, found %d", len(values))
	}
	m := &Mapping{}
	ints := []*int{&m.Size, &m.FirstNote, &m.LastNote, &m.MiddleNote, &m.RefNote}
	for i, dst := range ints {
		n, err := strconv.Atoi(values[i].text)
		if err != nil {
			return nil, fmt.Errorf("line %d: invalid integer %q", values[i].line, values[i].text)
		}
		*dst = n
	}
	f, err := strconv.ParseFloat(values[5].text, 64)
	if err != nil || f <= 0 {
		return nil, fmt.Errorf("line %d: invalid reference frequency %q", values[5].line, values[5].text)
	}
	m.RefFreq = f
	if m.FormalOctave, err = strconv.Atoi(values[6].text); err != nil {
		return nil, fmt.Errorf("line %d: invalid formal octave %q", values[6].line, values[6].text)
	}
	if m.Size < 0 {
		return nil, errors.New("negative map size")
	}
	entries := values[7:]
	if m.Size == 0 && len(entries) > 0 {
		m.Warnings = append(m.Warnings, Warning{entries[0].line, "map size is 0 but mapping lines follow; some synths use them"})
	}
	for i := 0; i < m.Size; i++ {
		if i >= len(entries) {
			m.Warnings = append(m.Warnings, Warning{0, fmt.Sprintf("map size %d but only %d entries; the rest are unmapped", m.Size, len(entries))})
			for ; i < m.Size; i++ {
				m.Keys = append(m.Keys, -1)
			}
			break
		}
		e := entries[i]
		if strings.EqualFold(e.text, "x") {
			m.Keys = append(m.Keys, -1)
			continue
		}
		k, err := strconv.Atoi(e.text)
		if err != nil {
			return nil, fmt.Errorf("line %d: invalid mapping entry %q", e.line, e.text)
		}
		m.Keys = append(m.Keys, k)
	}
	return m, nil
}

// LinearMapping maps consecutive MIDI notes to consecutive degrees of a
// size-note scale, with degree 0 on root tuned to refFreq. Every degree is
// listed explicitly: Madrona Labs synths ignore the size and need the entries.
func LinearMapping(size, root int, refFreq float64) Mapping {
	keys := make([]int, size)
	for i := range keys {
		keys[i] = i
	}
	return Mapping{Size: size, FirstNote: 0, LastNote: 127, MiddleNote: root, RefNote: root,
		RefFreq: refFreq, FormalOctave: size, Keys: keys}
}

// StandardFreq returns the 12-TET frequency of a MIDI note (A4 = 69 = 440 Hz).
func StandardFreq(note int) float64 {
	return 440 * math.Pow(2, float64(note-69)/12)
}

// Format renders the mapping as .kbm text.
func (m Mapping) Format(title string) string {
	var b strings.Builder
	line := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	line("! %s", title)
	line("! Size of map:")
	line("%d", m.Size)
	line("! First MIDI note number to retune:")
	line("%d", m.FirstNote)
	line("! Last MIDI note number to retune:")
	line("%d", m.LastNote)
	line("! Middle note where the first entry of the mapping is mapped to:")
	line("%d", m.MiddleNote)
	line("! Reference note for which frequency is given:")
	line("%d", m.RefNote)
	line("! Frequency to tune the above note to:")
	line("%.6f", m.RefFreq)
	line("! Scale degree to consider as formal octave:")
	line("%d", m.FormalOctave)
	line("! Mapping:")
	for _, k := range m.Keys {
		if k < 0 {
			line("x")
		} else {
			line("%d", k)
		}
	}
	return b.String()
}
