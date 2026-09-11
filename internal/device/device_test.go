package device

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/biomassa/linnkit/internal/layout"
	"github.com/biomassa/linnkit/internal/lights"
	"github.com/biomassa/linnkit/internal/scala"
	"github.com/biomassa/linnkit/internal/testutil"
	"github.com/biomassa/linnkit/internal/theory"
)

func newFake(t *testing.T, params map[int]int) (*Device, *FakePort) {
	t.Helper()
	f := &FakePort{Params: params}
	d, err := New(f)
	if err != nil {
		t.Fatal(err)
	}
	d.Delay = 0
	return d, f
}

func TestEncodeNRPN(t *testing.T) {
	got := NRPN(247, 10)
	want := [][]byte{{0xB0, 99, 1}, {0xB0, 98, 119}, {0xB0, 6, 0}, {0xB0, 38, 10}, {0xB0, 101, 127}, {0xB0, 100, 127}}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Errorf("message %d: % x, want % x", i, got[i], want[i])
		}
	}
	if r := RPN(6, 4<<7, 15); !bytes.Equal(r[0], []byte{0xBF, 101, 0}) || !bytes.Equal(r[2], []byte{0xBF, 6, 4}) {
		t.Errorf("RPN: % x", r)
	}
}

func TestParamTable(t *testing.T) {
	p, ok := LookupParam(ParamBendRange)
	if !ok || p.Min != 1 || p.Max != 96 || !p.Readable || p.Name != "Split MIDI Bend Range" {
		t.Errorf("bend range: %+v", p)
	}
	if p, ok := LookupParam(ParamBendRange + RightSplit); !ok || p.Num != 119 || !strings.HasPrefix(p.Name, "Right: ") {
		t.Errorf("right split: %+v", p)
	}
	if p, _ := LookupParam(ParamPlayedMode); p.Min != 0 || p.Max != 14 {
		t.Errorf("played mode: %+v", p)
	}
	readable := ReadableParams()
	for _, n := range []int{0, 19, 119, 227, 247, 263, 270} {
		if !slices.Contains(readable, n) {
			t.Errorf("parameter %d should be readable", n)
		}
	}
	if slices.Contains(readable, ParamQuery) {
		t.Error("299 is the query itself")
	}
}

func TestSetNRPNRejectsOutOfRange(t *testing.T) {
	d, f := newFake(t, nil)
	if err := d.SetNRPN(ParamBendRange, 0); err == nil {
		t.Error("bend range 0 should be rejected")
	}
	if len(f.Sent) != 0 {
		t.Errorf("nothing should be sent: %d messages", len(f.Sent))
	}
}

func golden(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// The message sequences must match reference/linnstrument_edo_just.py exactly
// (recorded with a fake port into testdata).
func TestMatchesPythonScript(t *testing.T) {
	s, err := scala.ParseSCLFile(filepath.Join(testutil.RepoRoot(t), "SCL", "31-edo.scl"))
	if err != nil {
		t.Fatal(err)
	}
	a := theory.Analyze(s, theory.DefaultOptions())
	l := layout.Uniform(10, 30)
	surface := lights.Paint(l, 60, lights.JIFamilies(a, 7, 0))
	channels := func(lo, hi int) []int {
		var out []int
		for ch := lo; ch <= hi; ch++ {
			out = append(out, ch)
		}
		return out
	}
	cases := []struct {
		name string
		cfg  ChannelConfig
	}{
		{"python_31edo_configure_slot2.txt", ChannelConfig{Main: 1, PerNote: channels(2, 16), Bend: 31}},
		{"python_31edo_mpe_2-5_slot2.txt", ChannelConfig{Main: 1, PerNote: channels(2, 5), Bend: 31, MPE: true}},
	}
	for _, c := range cases {
		d, f := newFake(t, nil)
		if err := d.Configure(c.cfg); err != nil {
			t.Fatal(err)
		}
		if err := d.SendLayout(l); err != nil {
			t.Fatal(err)
		}
		if err := d.PaintLights(surface, 2); err != nil {
			t.Fatal(err)
		}
		if got, want := f.Log(), golden(t, c.name); got != want {
			g, w := strings.Split(got, "\n"), strings.Split(want, "\n")
			for i := 0; i < min(len(g), len(w)); i++ {
				if g[i] != w[i] {
					t.Errorf("%s: first difference at message %d: got %q, want %q (%d vs %d messages)", c.name, i+1, g[i], w[i], len(g), len(w))
					break
				}
			}
			if len(g) != len(w) {
				t.Errorf("%s: %d messages, want %d", c.name, len(g), len(w))
			}
		}
	}
}

func TestReadBackupRestore(t *testing.T) {
	device := map[int]int{0: 1, 19: 31, 119: 31, 247: 10, 243: 5, 263: 30}
	d, f := newFake(t, device)
	got, err := d.Read([]int{19, 247}, time.Second)
	if err != nil || got[19] != 31 || got[247] != 10 {
		t.Fatalf("read: %v %v", got, err)
	}
	snap, err := d.Backup(50 * time.Millisecond) // most readable parameters are missing from the fake
	if err == nil || snap.Values[263] != 30 || len(snap.Values) != len(device) {
		t.Fatalf("backup: %v, %v", snap.Values, err)
	}

	device[19], device[263], device[243] = 48, 45, 2
	f.Sent = nil
	changed, saved, err := d.Restore(snap, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(changed, []int{19, 263}) || !saved {
		t.Errorf("changed %v saved %v", changed, saved)
	}
	if device[19] != 31 || device[263] != 30 || device[243] != 2 {
		t.Errorf("after restore: %v (243, preset load, must not be written)", device)
	}
	if last := f.Sent[len(f.Sent)-1]; !bytes.Equal(last, []byte{0xB0, 23, 1}) {
		t.Errorf("last message % x, want CC23 1 (save slot 1 to flash)", last)
	}
}

func TestSnapshotFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snap.json")
	s := Snapshot{Taken: time.Unix(0, 0).UTC(), Values: map[int]int{19: 31, 263: 30}}
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSnapshot(path)
	if err != nil || got.Values[19] != 31 || got.Values[263] != 30 {
		t.Errorf("round trip: %+v %v", got, err)
	}
}

func TestFactoryJob(t *testing.T) {
	device := map[int]int{247: 10, 227: 13, 263: 20, 36: 5}
	d, f := newFake(t, device)
	n, err := d.Run(Job{Factory: true, Slot: 1}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(FactorySettings()) {
		t.Errorf("verified %d values, want %d", n, len(FactorySettings()))
	}
	if device[247] != 0 || device[227] != 5 || device[263] != 30 || device[270] != 64 || device[203] != 1 ||
		device[204] != 0 || device[215] != 1 || device[216] != 0 || device[30] != 3 || device[RightSplit+30] != 5 {
		t.Errorf("device after factory send: %v", device)
	}
	for _, m := range f.Sent {
		if m[0]&0xF0 == 0xB0 && m[1] >= 20 && m[1] <= 23 {
			t.Fatalf("factory send must not touch custom lights: % x", m)
		}
	}
}
