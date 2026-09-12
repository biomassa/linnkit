package lights

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/biomassa/linnkit/internal/layout"
	"github.com/biomassa/linnkit/internal/scala"
	"github.com/biomassa/linnkit/internal/testutil"
	"github.com/biomassa/linnkit/internal/theory"
)

var update = flag.Bool("update", false, "rewrite testdata/more golden grids")

func edoAnalysis(t *testing.T, n int) *theory.Analysis {
	t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "%d-EDO\n%d\n", n, n)
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "%.6f\n", 1200*float64(i)/float64(n))
	}
	return analysisOf(t, b.String())
}

func analysisOf(t *testing.T, scl string) *theory.Analysis {
	t.Helper()
	s, err := scala.ParseSCL([]byte(scl))
	if err != nil {
		t.Fatal(err)
	}
	return theory.Analyze(s, theory.DefaultOptions())
}

func TestChain12EDO(t *testing.T) {
	sw := Chain(edoAnalysis(t, 12), DefaultSettings())
	want := map[int]Swatch{0: {Magenta, "C"}, 7: {White, "G"}, 2: {White, "D"}, 9: {White, "A"}, 4: {White, "E"},
		11: {Yellow, "B"}, 6: {Yellow, "F#"}, 1: {Yellow, "C#"}, 8: {Yellow, "G#"},
		5: {Green, "F"}, 10: {Green, "Bb"}, 3: {Green, "Eb"}}
	for d, w := range want {
		if sw[d] != w {
			t.Errorf("degree %d: %+v, want %+v", d, sw[d], w)
		}
	}
}

func TestChain31EDOCounts(t *testing.T) {
	a := edoAnalysis(t, 31)
	sw := Chain(a, DefaultSettings())
	count := map[Color]int{}
	labels := map[string]Color{}
	for _, s := range sw {
		count[s.Color]++
		labels[s.Label] = s.Color
	}
	want := map[Color]int{Magenta: 1, White: 4, Yellow: 4, Green: 4, Blue: 4, Red: 4, Cyan: 4, Pink: 4, Off: 2}
	for c, n := range want {
		if count[c] != n {
			t.Errorf("%s: %d pads, want %d", c, count[c], n)
		}
	}
	if labels["Ax"] != Off || labels["Gbb"] != Off {
		t.Error("k = 17 (Ax) and k = -13 (Gbb) should be unlit with their names")
	}
	if pos := chainPositions(31, 18); pos[0] != 0 || pos[18] != 1 || pos[13] != -1 {
		t.Errorf("chain positions %v", pos)
	}
}

func TestChainNamesAndSharedPositions(t *testing.T) {
	for k, want := range map[int]string{-1: "F", 0: "C", 5: "B", 6: "F#", 13: "Fx", 20: "F###", -2: "Bb", -9: "Bbb", -16: "Bbbb"} {
		if got := chainName(k); got != want {
			t.Errorf("k %d: %s, want %s", k, got, want)
		}
	}
	// gcd(4, 6) = 2: several k reach a degree; the one closest to 2 wins (the root: k 3 beats k 0).
	pos := chainPositions(6, 4)
	if pos[2] != 2 || pos[0] != 3 || pos[4] != 1 || pos[1] != noPos {
		t.Errorf("shared positions %v", pos)
	}
}

func TestMOSKeysValentine(t *testing.T) {
	a := edoAnalysis(t, 31)
	sw, m, ok := MOSKeys(a, Settings{Generator: 2, MOSSize: 16})
	if !ok || m.Size != 16 {
		t.Fatalf("%v %+v", ok, m)
	}
	white := map[int]bool{29: true}
	for d := 2; d <= 28; d += 2 {
		white[d] = true
	}
	for d, s := range sw {
		switch {
		case d == 0:
			if s != (Swatch{Magenta, "R"}) {
				t.Errorf("root %+v", s)
			}
		case white[d] && s != (Swatch{White, ""}), !white[d] && s != (Swatch{Blue, ""}):
			t.Errorf("degree %d: %+v", d, s)
		}
	}
	// automatic size along the chosen generator; none found = root only
	if _, m, ok := MOSKeys(a, DefaultSettings()); !ok || m.Size != 7 || m.Generator != 18 {
		t.Errorf("31-EDO automatic: %+v %v", m, ok)
	}
	if sw, _, ok := MOSKeys(edoAnalysis(t, 4), DefaultSettings()); ok || sw[0] != (Swatch{Magenta, "R"}) || sw[1] != (Swatch{}) {
		t.Errorf("4-EDO has no MOS of 5+ notes: %v %v", sw, ok)
	}
}

func TestWijmenga(t *testing.T) {
	_, c, chroma, meantone := Wijmenga(edoAnalysis(t, 53), DefaultSettings())
	if c != 1 || chroma != 3 || meantone {
		t.Errorf("53-EDO: comma %d chroma %d meantone %v", c, chroma, meantone)
	}
	sw, c, _, meantone := Wijmenga(edoAnalysis(t, 12), DefaultSettings())
	if c != 0 || !meantone {
		t.Fatalf("12-EDO: comma %d meantone %v", c, meantone)
	}
	for d, want := range map[int]Color{0: Magenta, 7: Green, 5: Green, 11: Green, 6: White, 1: White, 8: White, 3: Off, 10: Off} {
		if sw[d].Color != want || (d > 0 && sw[d].Label != "") {
			t.Errorf("12-EDO degree %d: %+v, want %s", d, sw[d], want)
		}
	}
	sw, _, _, _ = Wijmenga(edoAnalysis(t, 53), DefaultSettings())
	// naturals white; D (2 fifths up = 62 mod 53 = 9) + comma = 10 white; D + chroma = 12 green; D - chroma = 6 orange
	for d, want := range map[int]Color{9: White, 10: White, 8: White, 12: Green, 6: Orange} {
		if sw[d].Color != want {
			t.Errorf("53-EDO degree %d: %s, want %s", d, sw[d].Color, want)
		}
	}
}

const ji7 = "! ji\nfive- and seven-limit\n9\n9/8\n7/6\n5/4\n21/16\n3/2\n105/64\n7/4\n15/8\n2/1\n"

func TestKiteAndFactors(t *testing.T) {
	a := analysisOf(t, ji7)
	k := Kite(a, DefaultSettings())
	want := []Swatch{{Magenta, "R"}, {White, "wa"}, {Blue, "zo"}, {Yellow, "yo"}, {Blue, "zo"}, {White, "wa"}, {Blue, "zo"}, {Blue, "zo"}, {Yellow, "yo"}}
	for d, w := range want {
		if k[d] != w {
			t.Errorf("kite degree %d (%s): %+v, want %+v", d, a.Degrees[d].Ratio.RatString(), k[d], w)
		}
	}
	f := Factors(a, DefaultSettings())
	wantF := []Swatch{{Off, "R"}, {Red, "3"}, {Magenta, "37"}, {Green, "5"}, {Magenta, "37"}, {Red, "3"}, {White, "357"}, {Blue, "7"}, {Yellow, "35"}}
	for d, w := range wantF {
		if f[d] != w {
			t.Errorf("factors degree %d: %+v, want %+v", d, f[d], w)
		}
	}
	if k5 := Kite(a, Settings{Limit: 5}); k5[2] != (Swatch{}) || k5[3] != (Swatch{Yellow, "yo"}) {
		t.Errorf("limit 5 leaves 7/6 unlit: %+v %+v", k5[2], k5[3])
	}
	// cents scale: 12-EDO degree 4 reads as 5/4 (yo), degree 3 as 6/5 only if within tolerance
	if k12 := Kite(edoAnalysis(t, 12), DefaultSettings()); k12[4] != (Swatch{Yellow, "yo"}) || k12[7] != (Swatch{White, "wa"}) {
		t.Errorf("12-EDO kite %+v %+v", k12[4], k12[7])
	}
}

func TestSteps(t *testing.T) {
	sw, k := Steps(edoAnalysis(t, 12))
	if k != 1 || sw[5] != (Swatch{White, "L"}) || sw[0] != (Swatch{Magenta, "R"}) {
		t.Errorf("12-EDO: %d classes, %+v", k, sw[5])
	}
	sw, k = Steps(analysisOf(t, "! major\nJust major\n7\n9/8\n5/4\n4/3\n3/2\n5/3\n15/8\n2/1\n"))
	if k != 3 || sw[1] != (Swatch{Green, "M"}) || sw[2] != (Swatch{Blue, "s"}) || sw[3] != (Swatch{White, "L"}) {
		t.Errorf("just major: %d classes %+v", k, sw)
	}
	if stepSwatch(7, 9) != (Swatch{Green, "8"}) || stepSwatch(8, 9) != (Swatch{Blue, "9"}) {
		t.Errorf("9 classes: middle colours cycle: %+v", stepSwatch(7, 9))
	}
}

func TestNested31EDO(t *testing.T) {
	sw, layers := Nested(edoAnalysis(t, 31), DefaultSettings())
	if fmt.Sprint(layers) != "[5 7 12 19]" {
		t.Fatalf("layers %v", layers)
	}
	// chain k -> degree 18k mod 31: layer 3 adds k 6..10 (15 2 20 7 25), layer 4 k 11..17; degree 3 (k -5) is in none
	for d, want := range map[int]Swatch{18: {White, "1"}, 10: {Yellow, "2"}, 28: {Yellow, "2"}, 2: {Blue, "3"}, 12: {Green, "4"}, 3: {}} {
		if sw[d] != want {
			t.Errorf("degree %d: %+v, want %+v", d, sw[d], want)
		}
	}
}

func TestConsonanceAndHarmonics(t *testing.T) {
	a := edoAnalysis(t, 12)
	c := Consonance(a)
	if c[7] != (Swatch{White, "3/2"}) || c[5] != (Swatch{White, "4/3"}) || c[4] != (Swatch{White, "5/4"}) {
		t.Errorf("consonance %+v %+v %+v", c[7], c[5], c[4])
	}
	h := Harmonics(a, DefaultSettings())
	for d, want := range map[int]Swatch{7: {Yellow, "3"}, 5: {Blue, "u3"}, 4: {Yellow, "5"}, 8: {Blue, "u5"}, 2: {Yellow, "9"}} {
		if h[d] != want {
			t.Errorf("harmonics degree %d: %+v, want %+v", d, h[d], want)
		}
	}
	if h := Harmonics(a, Settings{Harmonics: 16}); h[5] != (Swatch{}) {
		t.Errorf("subharmonics off: %+v", h[5])
	}
	if harmonicSwatch(3, 5) != (Swatch{White, "3"}) {
		t.Error("both series: white, the harmonic number")
	}
	// the period counts as degree 0: 1190 c lands on the root in a scale whose last degree is far below it
	h = Harmonics(analysisOf(t, "! t\nt\n3\n400.\n800.\n1200.\n"), Settings{Harmonics: 32})
	if h[0] != (Swatch{Magenta, "R"}) {
		t.Errorf("root %+v", h[0])
	}
}

func TestPlainWidensAndShowsUnlitLabels(t *testing.T) {
	sw := []Swatch{{Off, "R"}, {White, "63/32"}, {}}
	s := Paint(layout.Uniform(3, 60), 60, sw)
	out := s.Plain()
	if !strings.Contains(out, "R     63/32 .     ") {
		t.Errorf("cells should be 6 wide and show the unlit root's label:\n%s", out)
	}
	if !strings.HasPrefix(s.CC(), "row 8  ") || !strings.Contains(s.CC(), "0  8  0  ") {
		t.Errorf("CC grid:\n%s", s.CC())
	}
}

// TestGoldenGrids pins each new scheme on the repo's scales and on 12- and
// 53-EDO (bottom-left MIDI 48, rows +5 degrees, root 60): labels then CC22
// colours. Run with -update to rewrite testdata/more after a deliberate change.
func TestGoldenGrids(t *testing.T) {
	scales := map[string]*theory.Analysis{"12edo": edoAnalysis(t, 12), "53edo": edoAnalysis(t, 53)}
	for _, name := range []string{"31-edo", "22edo", "ji_7", "ji_13"} {
		s, err := scala.ParseSCLFile(filepath.Join(testutil.RepoRoot(t), "SCL", name+".scl"))
		if err != nil {
			t.Fatal(err)
		}
		scales[name] = theory.Analyze(s, theory.DefaultOptions())
	}
	for name, a := range scales {
		for _, scheme := range MoreSchemes {
			sw, legend, err := Scheme(scheme, a, DefaultSettings())
			if err != nil {
				t.Fatal(err)
			}
			surf := Paint(layout.Uniform(5, 48), 60, sw)
			got := surf.Plain() + legend + "\n\n" + surf.CC()
			path := filepath.Join("testdata", "more", name+"."+scheme+".txt")
			if *update {
				os.MkdirAll(filepath.Dir(path), 0o755)
				os.WriteFile(path, []byte(got), 0o644)
				continue
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run go test -run TestGoldenGrids -update)", err)
			}
			if string(want) != got {
				t.Errorf("%s %s differs from %s:\n%s", name, scheme, path, got)
			}
		}
	}
}
