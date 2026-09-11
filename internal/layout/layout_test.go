package layout

import (
	"path/filepath"
	"testing"

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

func TestRootLow(t *testing.T) {
	cases := []struct{ offset, root, want int }{
		{10, 60, 30}, // 31-EDO rows +10: the current device setup
		{5, 60, 45},  // fourths, root on row 4
		{13, 60, 12}, // span 115 does not fit from 21, so shifted down
		{25, 60, 0},  // no overlap cannot fit; clamped at 0
	}
	for _, c := range cases {
		if got := RootLow(c.offset, c.root); got != c.want {
			t.Errorf("RootLow(%d, %d) = %d, want %d", c.offset, c.root, got, c.want)
		}
	}
	l := Uniform(10, 30)
	if l.Note(1, 3) != 60 || l.Note(25, 7) != 124 || !l.Fits() {
		t.Errorf("31-EDO layout: %v", l)
	}
}

func TestMove(t *testing.T) {
	// 31-EDO with rows +10: major third straight up, fifth two rows up and two left.
	cases := []struct{ s, r, dc, dr int }{{10, 10, 0, 1}, {18, 10, -2, 2}, {31, 10, 1, 3}, {5, 10, 5, 0}}
	for _, c := range cases {
		if dc, dr := Move(c.s, c.r); dc != c.dc || dr != c.dr {
			t.Errorf("Move(%d, %d) = %d, %d, want %d, %d", c.s, c.r, dc, dr, c.dc, c.dr)
		}
	}
}

func TestCandidates31EDO(t *testing.T) {
	cands := Candidates(analyze(t, "31-edo"), DefaultOptions())
	if len(cands) != Cols {
		t.Fatalf("%d candidates", len(cands))
	}
	best := cands[0]
	if best.Offset != 10 || !best.Fits || best.ChordSpan != 2 {
		t.Errorf("best: +%d fits %v span %.1f", best.Offset, best.Fits, best.ChordSpan)
	}
	for _, c := range cands {
		if c.Offset == 13 && !contains(c.Tags, "fourth") {
			t.Errorf("+13 should be tagged fourth: %v", c.Tags)
		}
		if c.Offset == 25 && (c.Fits || !contains(c.Tags, "no overlap")) {
			t.Errorf("+25: fits %v tags %v", c.Fits, c.Tags)
		}
	}
}

func TestCandidates12EDOFourthsAreReasonable(t *testing.T) {
	s, err := scala.ParseSCL([]byte("12-EDO\n12\n100.\n200.\n300.\n400.\n500.\n600.\n700.\n800.\n900.\n1000.\n1100.\n2/1\n"))
	if err != nil {
		t.Fatal(err)
	}
	cands := Candidates(theory.Analyze(s, theory.DefaultOptions()), DefaultOptions())
	rank := map[int]int{}
	for i, c := range cands {
		rank[c.Offset] = i
	}
	// Stock LinnStrument fourths (+5) and thirds (+4, +3) should be near the top.
	for _, r := range []int{3, 4, 5} {
		if rank[r] > 5 {
			t.Errorf("12-EDO +%d ranked %d", r, rank[r]+1)
		}
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
