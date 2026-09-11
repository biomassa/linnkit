package scala

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ErrEmpty is returned for files with no content.
var ErrEmpty = errors.New("empty file")

// Pitch is one scale degree as written in the file.
type Pitch struct {
	Cents float64  // size above 1/1, in cents
	Ratio *big.Rat // exact ratio, or nil when the file gave cents
	Text  string   // the value as written
	Line  int      // 1-based line number in the file
}

// IsRatio reports whether the pitch was written as a ratio.
func (p Pitch) IsRatio() bool { return p.Ratio != nil }

// Warning is a non-fatal problem found while reading a file.
type Warning struct {
	Line int // 0 when not tied to a line
	Msg  string
}

func (w Warning) String() string {
	if w.Line > 0 {
		return fmt.Sprintf("line %d: %s", w.Line, w.Msg)
	}
	return w.Msg
}

// Scale is a parsed .scl file. Degree 0 (1/1) is implicit: Pitches holds
// degrees 1..n and the last one is the period.
type Scale struct {
	Name        string // file name without extension
	Description string
	Declared    int // note count given in the file
	Pitches     []Pitch
	Comments    []string // comment lines without the leading '!'
	Warnings    []Warning
}

// Size returns the number of notes per period.
func (s *Scale) Size() int { return len(s.Pitches) }

// Period returns the last pitch, the interval at which the scale repeats.
func (s *Scale) Period() Pitch { return s.Pitches[len(s.Pitches)-1] }

// ParseSCLFile reads and parses a .scl file.
func ParseSCLFile(path string) (*Scale, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s, err := ParseSCL(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	s.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return s, nil
}

// ParseSCL parses the contents of a .scl file.
func ParseSCL(data []byte) (*Scale, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, ErrEmpty
	}
	s := &Scale{}
	const (
		wantDescription = iota
		wantCount
		wantPitches
	)
	state := wantDescription
	warnedExtra := false
	for i, line := range splitLines(data) {
		lineNo := i + 1
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "!") {
			s.Comments = append(s.Comments, strings.TrimSpace(trimmed[1:]))
			continue
		}
		switch state {
		case wantDescription:
			s.Description = trimmed
			state = wantCount
		case wantCount:
			fields := strings.Fields(line)
			if len(fields) == 0 {
				return nil, fmt.Errorf("line %d: missing note count", lineNo)
			}
			n, err := strconv.Atoi(fields[0])
			if err != nil || n < 0 {
				return nil, fmt.Errorf("line %d: invalid note count %q", lineNo, fields[0])
			}
			s.Declared = n
			state = wantPitches
		case wantPitches:
			if trimmed == "" {
				continue
			}
			if len(s.Pitches) == s.Declared {
				if !warnedExtra {
					s.Warnings = append(s.Warnings, Warning{lineNo, "lines after the declared notes, ignored"})
					warnedExtra = true
				}
				continue
			}
			p, err := parsePitch(strings.Fields(line)[0])
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNo, err)
			}
			p.Line = lineNo
			s.Pitches = append(s.Pitches, p)
		}
	}
	switch {
	case state != wantPitches:
		return nil, errors.New("missing note count")
	case s.Declared == 0:
		return nil, errors.New("scale has no notes")
	case len(s.Pitches) == 0:
		return nil, fmt.Errorf("declared %d notes, found none", s.Declared)
	case len(s.Pitches) < s.Declared:
		s.Warnings = append(s.Warnings, Warning{0, fmt.Sprintf("declared %d notes, found %d", s.Declared, len(s.Pitches))})
	}
	s.checkOrder()
	return s, nil
}

// checkOrder warns about degrees that are not above the previous one.
func (s *Scale) checkOrder() {
	prev := 0.0
	for i, p := range s.Pitches {
		if p.Cents <= prev {
			s.Warnings = append(s.Warnings, Warning{p.Line, fmt.Sprintf("degree %d (%s) is not above degree %d", i+1, p.Text, i)})
		}
		prev = p.Cents
	}
	if s.Period().Cents <= 0 {
		s.Warnings = append(s.Warnings, Warning{0, "period is not above 1/1"})
	}
}

// parsePitch parses the first field of a pitch line. Anything after the
// numeric value (for example "5/4E" or "100.0cents") is ignored.
func parsePitch(field string) (Pitch, error) {
	tok := valuePrefix(field)
	p := Pitch{Text: tok}
	if tok == "" {
		return p, fmt.Errorf("invalid pitch %q", field)
	}
	if strings.Contains(tok, ".") {
		c, err := strconv.ParseFloat(tok, 64)
		if err != nil {
			return p, fmt.Errorf("invalid cents value %q", tok)
		}
		p.Cents = c
		return p, nil
	}
	num, den := tok, "1"
	if i := strings.IndexByte(tok, '/'); i >= 0 {
		num, den = tok[:i], tok[i+1:]
	}
	n, ok1 := new(big.Int).SetString(num, 10)
	d, ok2 := new(big.Int).SetString(den, 10)
	if !ok1 || !ok2 {
		return p, fmt.Errorf("invalid ratio %q", tok)
	}
	if n.Sign() <= 0 || d.Sign() <= 0 {
		return p, fmt.Errorf("ratio %q must be positive", tok)
	}
	p.Ratio = new(big.Rat).SetFrac(n, d)
	p.Cents = RatioCents(p.Ratio)
	return p, nil
}

// valuePrefix returns the leading run of characters that can form a pitch value.
func valuePrefix(field string) string {
	end := 0
	for end < len(field) && strings.IndexByte("0123456789+-./", field[end]) >= 0 {
		end++
	}
	return field[:end]
}

// RatioCents converts a ratio to cents.
func RatioCents(r *big.Rat) float64 {
	f, _ := r.Float64()
	return 1200 * math.Log2(f)
}

// splitLines decodes the file and splits it on CRLF, LF or CR.
func splitLines(data []byte) []string {
	text := toUTF8(data)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.Split(text, "\n")
}

// toUTF8 returns data as a string, decoding it as Latin-1 when it is not valid UTF-8.
func toUTF8(data []byte) string {
	if utf8.Valid(data) {
		return string(data)
	}
	runes := make([]rune, len(data))
	for i, b := range data {
		runes[i] = rune(b)
	}
	return string(runes)
}
