package lights

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/biomassa/linnkit/internal/layout"
	"github.com/biomassa/linnkit/internal/scala"
	"github.com/biomassa/linnkit/internal/testutil"
	"github.com/biomassa/linnkit/internal/theory"
)

func analyze(t *testing.T, name string) *theory.Analysis {
	t.Helper()
	s, err := scala.ParseSCLFile(filepath.Join(testutil.RepoRoot(t), "SCL", name+".scl"))
	if err != nil {
		t.Fatal(err)
	}
	return theory.Analyze(s, theory.DefaultOptions())
}

func trimLines(s string) []string {
	var out []string
	for _, l := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		out = append(out, strings.TrimRight(l, " "))
	}
	return out
}

// The 31-EDO just-interval grid must match reference/linnstrument_edo_just.py
// (--edo 31: rows +10 from MIDI 30, root 60, 7-limit, tolerance 13.5 c).
func TestJIFamilies31EDOMatchesPython(t *testing.T) {
	a := analyze(t, "31-edo")
	got := Paint(layout.Uniform(10, 30), 60, JIFamilies(a, 7, 0)).Plain()
	want, err := os.ReadFile("testdata/31edo_just7_python.txt")
	if err != nil {
		t.Fatal(err)
	}
	g, w := trimLines(got), trimLines(string(want))
	if !slices.Equal(g, w) {
		t.Errorf("grid differs from the Python reference\n got:\n%s\nwant:\n%s", strings.Join(g, "\n"), strings.Join(w, "\n"))
	}
}

func TestJIFamiliesExactRatios(t *testing.T) {
	a := analyze(t, "ji_13") // degree 6 is 45/32 (5-limit), degree 7 is 64/45 (5-limit)
	sw := JIFamilies(a, 7, 0)
	if sw[6].Color != Green || sw[7].Color != Green {
		t.Errorf("45/32 and 64/45 should be 5-limit green: %v %v", sw[6], sw[7])
	}
	if sw[8].Color != White { // 3/2
		t.Errorf("3/2: %v", sw[8])
	}
	sw5 := JIFamilies(analyze(t, "ji_9"), 5, 0) // 7-limit ratios are off in a 5-limit scheme
	if sw5[2].Color != Off {                    // 7/6
		t.Errorf("7/6 with limit 5: %v", sw5[2])
	}
}

func TestDefaultMOS31EDO(t *testing.T) {
	a := analyze(t, "31-edo")
	m, ok := DefaultMOS(a)
	if !ok || m.Generator != 18 || m.Size != 7 {
		t.Fatalf("MOS: %+v %v", m, ok)
	}
	if got := m.Degrees(31); !slices.Equal(got, []int{0, 5, 10, 13, 18, 23, 28}) {
		t.Errorf("diatonic degrees: %v", got)
	}
}

func TestPaintOutOfRange(t *testing.T) {
	s := Paint(layout.Uniform(25, 0), 60, RootOnly(12, Magenta))
	if p := s[7][24]; p.Degree != -1 || p.Label != "!!" {
		t.Errorf("top-right pad of a no-overlap layout from 0: %+v", p)
	}
	if p := s[2][10]; p.Note != 60 || p.Color != Magenta {
		t.Errorf("root pad: %+v", p)
	}
}
