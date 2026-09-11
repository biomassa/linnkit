package export

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/biomassa/linnkit/internal/scala"
)

func TestMadronaCents(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []float64
	}{
		{"plain", "! x\ndesc\n3\n9/8\n701.955\n2\n", []float64{203.910, 701.955, 1200}},
		{"comment with a dot", "desc\n2\n3/2 ! 1.5 times\n2/1\n", []float64{3, 1200}},
		{"blank line before the count", "desc\n\n2\n3/2\n2/1\n", []float64{1200 * math.Log2(2), 701.955, 1200}},
		{"CRLF", "desc\r\n2\r\n3/2\r\n2/1\r\n", []float64{701.955, 1200}},
		{"indented comment counts", "desc\n2\n ! note\n3/2\n2/1\n", []float64{701.955, 1200}},
	}
	for _, c := range cases {
		got := MadronaCents([]byte(c.text))
		if len(got) != len(c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
			continue
		}
		for i := range got {
			if math.Abs(got[i]-c.want[i]) > 0.001 {
				t.Errorf("%s: %v, want %v", c.name, got, c.want)
				break
			}
		}
	}
}

func TestProblemsAndFiles(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.scl")
	os.WriteFile(good, []byte("! good\nfifths\n2\n3/2\n2/1\n"), 0o644)
	bad := []byte("dotted comment\n2\n3/2 ! 1.5\n2/1\n")
	s, _ := scala.ParseSCL(bad)
	if p := MadronaProblems(bad, s); len(p) == 0 || !strings.Contains(p[0], "degree 1") {
		t.Errorf("problems: %v", p)
	}
	gs, _ := scala.ParseSCLFile(good)
	if p := MadronaProblems([]byte("! good\nfifths\n2\n3/2\n2/1\n"), gs); len(p) != 0 {
		t.Errorf("good file: %v", p)
	}

	out := filepath.Join(dir, "Scales", "linnkit")
	files, err := Madrona([]Item{{Src: good, Scale: gs, Root: 62, RefFreq: 293.665}}, out)
	if err != nil || len(files) != 2 || files[0].Existed {
		t.Fatalf("%v %v", files, err)
	}
	if n, err := Write(files); n != 2 || err != nil {
		t.Fatalf("wrote %d: %v", n, err)
	}
	copied, _ := os.ReadFile(filepath.Join(out, "good.scl"))
	orig, _ := os.ReadFile(good)
	if string(copied) != string(orig) {
		t.Error(".scl must be copied unchanged")
	}
	kbm, err := scala.ParseKBMFile(filepath.Join(out, "good.kbm"))
	if err != nil || kbm.MiddleNote != 62 || kbm.RefNote != 62 || kbm.FormalOctave != 2 || len(kbm.Keys) != 2 ||
		math.Abs(kbm.RefFreq-293.665) > 1e-6 {
		t.Errorf("kbm %+v %v", kbm, err)
	}
	files, _ = Madrona([]Item{{Src: good, Scale: gs, Root: 62, RefFreq: 293.665}}, out)
	if !files[0].Same || !files[1].Same {
		t.Error("a second export should find the files up to date")
	}
	if n, _ := Write(files); n != 0 {
		t.Errorf("rewrote %d unchanged files", n)
	}
}
