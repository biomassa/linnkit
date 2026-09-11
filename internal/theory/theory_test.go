package theory

import (
	"math"
	"math/big"
	"path/filepath"
	"testing"

	"github.com/biomassa/linnkit/internal/scala"
	"github.com/biomassa/linnkit/internal/testutil"
)

func fixture(t *testing.T, name string) *Analysis {
	t.Helper()
	s, err := scala.ParseSCLFile(filepath.Join(testutil.RepoRoot(t), "SCL", name+".scl"))
	if err != nil {
		t.Fatal(err)
	}
	return Analyze(s, DefaultOptions())
}

func parse(t *testing.T, src string) *Analysis {
	t.Helper()
	s, err := scala.ParseSCL([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return Analyze(s, DefaultOptions())
}

func TestClassFixtures(t *testing.T) {
	want := map[string]Class{
		"31-edo": Equal, "22edo": Equal, "ji_9": NearEqual, "ji_8coh": Irregular, "ji_9coh": Irregular,
	}
	for name, class := range want {
		if got := fixture(t, name).Structure.Class; got != class {
			t.Errorf("%s: class %v, want %v", name, got, class)
		}
	}
	if st := fixture(t, "31-edo").Structure; math.Abs(st.EqualStep-1200.0/31) > 1e-3 {
		t.Errorf("31-edo step %f", st.EqualStep)
	}
}

func TestDiatonicMOS(t *testing.T) {
	a := parse(t, "Pythagorean diatonic\n7\n9/8\n81/64\n4/3\n3/2\n27/16\n243/128\n2/1\n")
	st := a.Structure
	if st.Class != MOS || st.Pattern != "LLsLLLs" || st.Signature() != "5L2s" {
		t.Fatalf("class %v pattern %q", st.Class, st.Pattern)
	}
	if st.GeneratorSteps != 3 || math.Abs(st.GeneratorCents-498.045) > 0.01 {
		t.Errorf("generator %d steps %.3f c, want 3 steps 498.045 c", st.GeneratorSteps, st.GeneratorCents)
	}
	if a.JI.PrimeLimit != 3 || !a.JI.AllExact {
		t.Errorf("JI: %+v", a.JI)
	}
	m := a.IntervalMatrix()
	if m[0][4].Ratio.RatString() != "3/2" || m[4][0].Ratio.RatString() != "4/3" {
		t.Errorf("matrix: %v %v", m[0][4].Ratio, m[4][0].Ratio)
	}
}

func TestLimits(t *testing.T) {
	for _, c := range []struct {
		r     string
		prime int
		odd   int64
	}{{"5/4", 5, 5}, {"7/4", 7, 7}, {"16/15", 5, 15}, {"2/1", 2, 1}, {"45/32", 5, 45}, {"11/8", 11, 11}} {
		r, _ := new(big.Rat).SetString(c.r)
		if p, o := PrimeLimit(r), OddLimit(r); p != c.prime || o != c.odd {
			t.Errorf("%s: limits %d/%d, want %d/%d", c.r, p, o, c.prime, c.odd)
		}
	}
}

func TestNearestRatiosFor31EDO(t *testing.T) {
	a := fixture(t, "31-edo")
	want := map[int]string{10: "5/4", 18: "3/2", 25: "7/4", 13: "4/3"}
	for deg, ratio := range want {
		d := a.JI.Degrees[deg]
		if d.Ratio == nil || d.Ratio.RatString() != ratio || d.Exact {
			t.Errorf("degree %d: %+v, want nearest %s", deg, d, ratio)
		}
	}
	// 31-edo.scl writes its period as 1200. (cents), so 2/1 is a nearest match, not exact.
	if p := a.JI.Degrees[31]; p.Ratio.RatString() != "2" || p.Exact || math.Abs(p.Error) > 1e-9 {
		t.Errorf("period: %+v", p)
	}
}

func TestNearest12(t *testing.T) {
	if name, oct, off := Nearest12(386.3137); name != "E" || oct != 0 || math.Abs(off+13.686) > 0.01 {
		t.Errorf("5/4: %s %d %.3f", name, oct, off)
	}
	if name, oct, _ := Nearest12(1901.955); name != "G" || oct != 1 {
		t.Errorf("3/1: %s %d", name, oct)
	}
}
