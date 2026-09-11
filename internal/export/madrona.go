// Package export copies scales to synths' scale folders, with a matching .kbm
// where the synth needs one. It never rewrites .scl files.
package export

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/biomassa/linnkit/internal/scala"
)

// MadronaDir is where linnkit puts scales for Madrona Labs synths:
// ~/Music/Madrona Labs/Scales, the top level of Aalto's scale menu.
func MadronaDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Music", "Madrona Labs", "Scales")
}

// Item is one scale to export.
type Item struct {
	Src     string       // the .scl file, copied unchanged
	Scale   *scala.Scale // parsed Src
	Root    int          // MIDI note of degree 0
	RefFreq float64      // Hz of the root note
}

// File is one file an export writes.
type File struct {
	Path    string
	Data    []byte
	Existed bool // a file is already there
	Same    bool // and it has exactly this content
}

// Madrona plans the files for items in dir: each .scl copied byte for byte,
// and a .kbm with degree 0 on Root at RefFreq that lists every degree
// (madronalib ignores the map size and needs the entries).
func Madrona(items []Item, dir string) ([]File, error) {
	var out []File
	seen := map[string]string{}
	for _, it := range items {
		data, err := os.ReadFile(it.Src)
		if err != nil {
			return nil, err
		}
		base := strings.TrimSuffix(filepath.Base(it.Src), filepath.Ext(it.Src))
		if prev, ok := seen[strings.ToLower(base)]; ok {
			return nil, fmt.Errorf("%s and %s have the same name", prev, it.Src)
		}
		seen[strings.ToLower(base)] = it.Src
		m := scala.LinearMapping(it.Scale.Size(), it.Root, it.RefFreq)
		kbm := m.Format(fmt.Sprintf("%s: degree 0 on MIDI %d at %.3f Hz, written by linnkit", base, it.Root, it.RefFreq))
		out = append(out, file(filepath.Join(dir, base+".scl"), data), file(filepath.Join(dir, base+".kbm"), []byte(kbm)))
	}
	return out, nil
}

func file(path string, data []byte) File {
	f := File{Path: path, Data: data}
	if old, err := os.ReadFile(path); err == nil {
		f.Existed, f.Same = true, bytes.Equal(old, data)
	}
	return f
}

// Write writes the files that differ, creating their folders. It returns how
// many it wrote.
func Write(files []File) (int, error) {
	n := 0
	for _, f := range files {
		if f.Same {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
			return n, err
		}
		tmp := f.Path + ".linnkit-tmp"
		if err := os.WriteFile(tmp, f.Data, 0o644); err != nil {
			return n, err
		}
		if err := os.Rename(tmp, f.Path); err != nil {
			os.Remove(tmp)
			return n, err
		}
		n++
	}
	return n, nil
}

// Existing lists the files already in dir (none if it doesn't exist).
func Existing(dir string) []string {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

// MadronaCents reads an .scl the way madronalib's Scale::loadScaleFromString
// does (MLDSPScale.h 48-120) and returns the cents of degrees 1..N:
//   - every line not starting with '!' in its first byte counts, blank lines too
//   - content line 1 is the description, line 2 (the note count) is ignored
//   - a line containing '.' anywhere is cents, else one with '/' is a ratio,
//     else an integer; ratios and integers must be positive
func MadronaCents(data []byte) []float64 {
	var cents []float64
	content := 0
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.HasPrefix(line, "!") {
			continue
		}
		content++
		if content == 2 {
			cents = nil
		}
		if content <= 2 {
			continue
		}
		fields := strings.Fields(line)
		switch {
		case strings.Contains(line, "."):
			if len(fields) > 0 {
				if v, err := strconv.ParseFloat(leadingNumber(fields[0]), 64); err == nil {
					cents = append(cents, v)
				} else {
					cents = append(cents, 0) // the C++ stream reads 0 and still adds it
				}
			} else {
				cents = append(cents, 0)
			}
		case strings.Contains(line, "/"):
			var n, d int
			if _, err := fmt.Sscanf(strings.TrimSpace(line), "%d/%d", &n, &d); err == nil && n > 0 && d > 0 {
				cents = append(cents, 1200*math.Log2(float64(n)/float64(d)))
			}
		default:
			if len(fields) > 0 {
				if n, err := strconv.Atoi(leadingNumber(fields[0])); err == nil && n > 0 {
					cents = append(cents, 1200*math.Log2(float64(n)))
				}
			}
		}
	}
	return cents
}

// leadingNumber keeps the numeric prefix of s, as a C++ stream extraction does.
func leadingNumber(s string) string {
	end := 0
	for end < len(s) && strings.ContainsRune("+-0123456789.eE", rune(s[end])) {
		end++
	}
	return s[:end]
}

// MadronaProblems compares madronalib's reading of the file with ours and
// describes the differences. Empty means Aalto tunes it as linnkit shows it.
func MadronaProblems(data []byte, s *scala.Scale) []string {
	got := MadronaCents(data)
	var want []float64
	for _, p := range s.Pitches {
		want = append(want, p.Cents)
	}
	var out []string
	if len(got) != len(want) {
		out = append(out, fmt.Sprintf("Aalto reads %d degrees, linnkit %d (blank lines or comments without '!' in the first column?)", len(got), len(want)))
	}
	for i := range min(len(got), len(want)) {
		if math.Abs(got[i]-want[i]) > 0.001 {
			out = append(out, fmt.Sprintf("degree %d: Aalto reads %.3f c, linnkit %.3f c", i+1, got[i], want[i]))
			if len(out) >= 4 {
				break
			}
		}
	}
	if len(got) == 0 {
		out = append(out, "Aalto reads no degrees and falls back to 12-equal")
	}
	if len(want) >= 127 {
		// the .kbm lists every degree; madronalib ignores maps of 127 or more entries
		out = append(out, fmt.Sprintf("%d degrees: Aalto ignores a .kbm this long and puts degree 0 on A4 = 440 Hz", len(want)))
	}
	return out
}
