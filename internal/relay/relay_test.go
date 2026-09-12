package relay

import (
	"fmt"
	"math"
	"testing"
	"time"
)

type sink struct{ msgs [][]byte }

func (s *sink) send(m []byte) error { s.msgs = append(s.msgs, append([]byte(nil), m...)); return nil }

func (s *sink) take() [][]byte { m := s.msgs; s.msgs = nil; return m }

func edo(n int) Tuning {
	c := make([]float64, n)
	for i := range c {
		c[i] = 1200 * float64(i+1) / float64(n)
	}
	return Tuning{Root: 60, Cents: c}
}

func newEngine(t Tuning, first, last int) (*Engine, *sink) {
	s := &sink{}
	return New(Config{Tuning: t, FirstChan: first, LastChan: last, BendRange: 24, MinNote: 0, MaxNote: 127, InBend: 24}, s.send), s
}

func bendValue(m []byte) int { return int(m[1]) | int(m[2])<<7 }

func TestPitch(t *testing.T) {
	e31 := edo(31)
	for _, c := range []struct{ pos, want float64 }{
		{0, 60}, {1, 60 + 12.0/31}, {31, 72}, {-1, 60 - 12.0/31}, {-31, 48}, {0.5, 60 + 6.0/31},
	} {
		if got := e31.Pitch(c.pos); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("31-EDO pos %v: %v, want %v", c.pos, got, c.want)
		}
	}
	ji := Tuning{Root: 60, Cents: []float64{203.91, 386.31, 498.04, 701.96, 884.36, 1088.27, 1200}}
	if got := ji.Pitch(1.5); math.Abs(got-(60+2.9511)) > 1e-4 {
		t.Errorf("halfway between 9/8 and 5/4: %v", got)
	}
	if got := ji.Pitch(9); math.Abs(got-(72+3.8631)) > 1e-4 {
		t.Errorf("5/4 an octave up: %v", got)
	}
}

func TestNoteOnBendThenNote(t *testing.T) {
	e, s := newEngine(edo(31), 1, 16)
	e.Handle([]byte{0x91, 61, 100}) // degree 1: 60 + 0.387 semitones
	m := s.take()
	if len(m) != 2 || m[0][0] != 0xE0 || m[1][0] != 0x90 || m[1][1] != 60 || m[1][2] != 100 {
		t.Fatalf("messages % x", m)
	}
	if b := bendValue(m[0]); b != 8192+132 { // 0.3871/24*8192 = 132.1
		t.Errorf("bend %d", b)
	}
	e.Handle([]byte{0x81, 61, 0})
	if m := s.take(); len(m) != 1 || m[0][0] != 0x80 || m[0][1] != 60 {
		t.Errorf("note off % x", m)
	}
}

func TestSlideLandsOnScaleSteps(t *testing.T) {
	ji := Tuning{Root: 60, Cents: []float64{203.91, 386.31, 498.04, 701.96, 884.36, 1088.27, 1200}}
	e, s := newEngine(ji, 1, 16)
	e.Handle([]byte{0x92, 60, 90})
	s.take()
	// slide two pads up: bend = 2/24 of full range
	v := 8192 + int(math.Round(2.0/24*8192))
	e.Handle([]byte{0xE2, byte(v & 0x7F), byte(v >> 7)})
	m := s.take()
	if len(m) != 1 || m[0][0] != 0xE0 {
		t.Fatalf("% x", m)
	}
	got := float64(bendValue(m[0])-8192) / 8192 * 24
	if math.Abs(got-3.8631) > 0.01 { // lands on 5/4 (386.31 c) above the held C
		t.Errorf("slide of two pads bends %v semitones, want 3.863", got)
	}
}

func TestChannelsReuseTheQuietestAndSteal(t *testing.T) {
	e, s := newEngine(edo(12), 1, 3)
	on := func(ch, n byte) { e.Handle([]byte{0x90 | ch, n, 100}) }
	off := func(ch, n byte) { e.Handle([]byte{0x80 | ch, n, 0}) }
	outCh := func() int {
		for _, m := range s.take() {
			if m[0]&0xF0 == 0x90 {
				return int(m[0]&0x0F) + 1
			}
		}
		return -1
	}
	on(1, 60)
	if c := outCh(); c != 1 {
		t.Fatalf("first note on channel %d", c)
	}
	on(2, 62)
	outCh()
	on(3, 64)
	outCh()
	off(1, 60) // channel 1 frees first
	off(2, 62)
	s.take()
	on(4, 65)
	if c := outCh(); c != 1 {
		t.Errorf("new note should reuse the channel quiet longest (1), got %d", c)
	}
	on(5, 67) // channel 2
	outCh()
	on(6, 69) // all three busy: steal the oldest (channel 3, note 64)
	m := s.take()
	if m[0][0] != 0x82 || m[0][1] != 64 || e.Stats().Stolen != 1 {
		t.Errorf("steal: % x", m)
	}
}

func TestDroppedNotesAndControllers(t *testing.T) {
	s := &sink{}
	e := New(Config{Tuning: edo(12), FirstChan: 1, LastChan: 4, BendRange: 24, MinNote: 24, MaxNote: 120, InBend: 24}, s.send)
	e.Handle([]byte{0x90, 20, 100}) // below the Mutant Brain's range
	e.Handle([]byte{0x80, 20, 0})
	if m := s.take(); len(m) != 0 || e.Stats().Dropped != 1 {
		t.Errorf("dropped note sent % x", m)
	}
	e.Handle([]byte{0xB5, 74, 30}) // Y before the note on channel 6
	e.Handle([]byte{0xD5, 50})     // Z before the note
	if m := s.take(); len(m) != 0 {
		t.Fatalf("controllers without a note should wait: % x", m)
	}
	e.Handle([]byte{0x95, 60, 100})
	m := s.take()
	if len(m) != 4 || m[len(m)-1][0] != 0x90 {
		t.Fatalf("pending controllers, bend, note: % x", m)
	}
	e.Handle([]byte{0xD5, 70})
	if m := s.take(); len(m) != 1 || m[0][0] != 0xD0 || m[0][1] != 70 {
		t.Errorf("pressure should follow the note to its channel: % x", m)
	}
	e.Handle([]byte{0xB0, 64, 127}) // sustain goes everywhere
	if m := s.take(); len(m) != 4 {
		t.Errorf("sustain to %d channels", len(m))
	}
	e.Handle([]byte{0xF8})
	if m := s.take(); len(m) != 1 || m[0][0] != 0xF8 {
		t.Errorf("clock % x", m)
	}
}

func TestStartStopAndRetune(t *testing.T) {
	s := &sink{}
	e := New(Config{Tuning: edo(12), FirstChan: 1, LastChan: 2, BendRange: 24, MaxNote: 127, InBend: 24, SendRPN: true}, s.send)
	e.Start()
	m := s.take()
	if len(m) != 14 || m[2][1] != 6 || m[2][2] != 24 { // 6 RPN CCs + a centred bend, per channel
		t.Fatalf("start % x", m)
	}
	e.Handle([]byte{0x90, 64, 100})
	s.take()
	e.SetTuning(edo(31)) // E (4 degrees) now = 4 steps of 31-EDO: 60 + 1.548
	m = s.take()
	if len(m) != 1 || m[0][0] != 0xE0 {
		t.Fatalf("retune % x", m)
	}
	got := 64 + float64(bendValue(m[0])-8192)/8192*24
	if math.Abs(got-(60+48.0/31)) > 0.01 {
		t.Errorf("retuned pitch %v", got)
	}
	e.SetTuning(edo(31))
	if m := s.take(); len(m) != 0 {
		t.Errorf("the same tuning again should send nothing: % x", m)
	}
	e.Stop()
	m = s.take()
	if m[0][0] != 0x80 || len(m) != 4 { // note off, CC123 on both channels, bend reset on channel 1 only (2 is centred)
		t.Errorf("stop % x", m)
	}
}

func scalaEngine(tu Tuning, s, b int) (*Engine, *sink) {
	sk := &sink{}
	return New(Config{Tuning: tu, Scala: true, FirstChan: 2, LastChan: 16, BendRange: s, MaxNote: 127, InBend: b}, sk.send), sk
}

func bendMsg(v int, ch byte) []byte { return []byte{0xE0 | ch, byte(v & 0x7F), byte(v >> 7)} }

// The examples of the shared spec scala-synth-bends.md.
func TestScalaSynthSpecExamples(t *testing.T) {
	for _, c := range []struct{ s, want int }{{48, 9778}, {12, 14534}} {
		e, sk := scalaEngine(edo(31), c.s, 48)
		e.Handle([]byte{0x91, 70, 100})
		m := sk.take()
		if last := m[len(m)-1]; last[0] != 0x91 || last[1] != 70 || bendValue(m[0]) != 8192 {
			t.Fatalf("S %d: note should pass unchanged on the first relay channel with a centred bend: % x", c.s, m)
		}
		e.Handle(bendMsg(12288, 1)) // +24 steps
		if m := sk.take(); len(m) != 1 || bendValue(m[0]) != c.want {
			t.Errorf("31-EDO +24 steps, S %d: % x, want %d", c.s, m, c.want)
		}
	}
	ji := Tuning{Root: 60, Cents: []float64{203.91, 386.31, 498.04, 701.96, 884.36, 1088.27, 1200}}
	e, sk := scalaEngine(ji, 48, 64)
	e.Handle([]byte{0x90, 61, 100}) // degree 1 (9/8)
	sk.take()
	e.Handle(bendMsg(8256, 0)) // +0.5 step towards 5/4
	if m := sk.take(); len(m) != 1 || bendValue(m[0]) != 8348 {
		t.Errorf("JI half step: % x, want 8348", m)
	}
	if s := e.Stats(); len(s.Voices) != 1 || math.Abs((s.Voices[0].Pitch-61)*100-91.2) > 0.01 {
		t.Errorf("stats pitch %+v", s.Voices)
	}
}

func TestScalaTableExtrapolates(t *testing.T) {
	tab := edo(31).table()
	step := 1200.0 / 31
	if got := tablePitch(&tab, 129) - tab[127]; math.Abs(got-2*step) > 1e-9 {
		t.Errorf("beyond 127: %v", got)
	}
	if got := tab[0] - tablePitch(&tab, -1); math.Abs(got-step) > 1e-9 {
		t.Errorf("below 0: %v", got)
	}
	if got := tablePitch(&tab, 60.5) - tab[60]; math.Abs(got-step/2) > 1e-9 {
		t.Errorf("between notes: %v", got)
	}
}

func TestRoundHalfUp(t *testing.T) {
	for x, want := range map[float64]int{2.5: 3, -2.5: -2, -2.51: -3, 0.49: 0, -0.5: 0, 132.13: 132} {
		if got := roundHalfUp(x); got != want {
			t.Errorf("roundHalfUp(%v) = %d, want %d", x, got, want)
		}
	}
}

// The cases below mirror linn.retune's tests (Max, tests/linn.retune.test.mjs at d0c4edb).

func clocked(cfg Config) (*Engine, *sink, *time.Time) {
	s := &sink{}
	e := New(cfg, s.send)
	now := time.Unix(1000, 0)
	e.now = func() time.Time { return now }
	e.manual = true
	return e, s, &now
}

func (e *Engine) tick() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.rampTick()
}

// channels are 1-based in these helpers, as in the Max tests
func on(ch, n, v int) []byte  { return []byte{0x90 | byte(ch-1), byte(n), byte(v)} }
func off(ch, n, v int) []byte { return []byte{0x80 | byte(ch-1), byte(n), byte(v)} }
func bnd(ch, v int) []byte    { return bendMsg(v, byte(ch-1)) }
func bendAt(semis float64, r int) int {
	return max(0, min(16383, 8192+roundHalfUp(semis/float64(r)*8192)))
}

func same(t *testing.T, what string, got, want [][]byte) {
	t.Helper()
	if fmt.Sprintf("% x", got) != fmt.Sprintf("% x", want) {
		t.Errorf("%s:\n got % x\nwant % x", what, got, want)
	}
}

func TestVoiceLimit(t *testing.T) {
	e, s, _ := clocked(Config{Tuning: edo(12), FirstChan: 1, LastChan: 16, BendRange: 24, MaxNote: 127, InBend: 48, Voices: 4})
	e.Start()
	s.take()
	var chans []int
	for i := range 4 {
		e.Handle(on(2, 40+i, 90))
	}
	for _, m := range s.take() {
		if m[0]&0xF0 == 0x90 {
			chans = append(chans, int(m[0]&0x0F)+1)
		}
	}
	if fmt.Sprint(chans) != "[1 2 3 4]" {
		t.Errorf("channels %v", chans)
	}
	e.Handle(on(2, 50, 90))
	same(t, "a fifth note cuts the oldest", s.take(), [][]byte{off(1, 40, 0), on(1, 50, 90)})
	e.Stop()
	n := 0
	for _, m := range s.take() {
		if m[0]&0xF0 == 0xB0 && m[1] == 123 {
			n++
		}
	}
	if n != 4 {
		t.Errorf("stop covers the 4 channels in use, got %d", n)
	}
}

func TestMutantBrainMono(t *testing.T) {
	st := 12.0 / 31
	e, s, _ := clocked(Config{Tuning: edo(31), FirstChan: 1, LastChan: 1, BendRange: 24, MinNote: 24, MaxNote: 120, InBend: 48, Mono: true, Legato: true})
	e.Start()
	s.take()
	// note range 24-120, on a 12-TET table as in the Max test (in 31-EDO every pad is in range)
	d, ds, _ := clocked(Config{Tuning: edo(12), FirstChan: 1, LastChan: 1, BendRange: 24, MinNote: 24, MaxNote: 120, InBend: 48, Mono: true, Legato: true})
	d.Start()
	ds.take()
	for _, m := range [][]byte{on(2, 20, 90), off(2, 20, 0), on(2, 121, 90), off(2, 121, 0)} {
		d.Handle(m)
	}
	same(t, "out of range: dropped with their note-offs", ds.take(), nil)
	e.Handle(on(2, 61, 90))
	same(t, "first note", s.take(), [][]byte{bnd(1, bendAt(st, 24)), on(1, 60, 90)})
	e.Handle(on(3, 70, 80))
	same(t, "legato: the new note starts first", s.take(), [][]byte{bnd(1, bendAt(10*st-4, 24)), on(1, 64, 80), off(1, 60, 0)})
	e.Handle(off(3, 70, 0))
	same(t, "back to the held note", s.take(), [][]byte{bnd(1, bendAt(st, 24)), on(1, 60, 90), off(1, 64, 0)})
	e.Handle(off(2, 61, 0))
	same(t, "last release", s.take(), [][]byte{off(1, 60, 0)})

	e.cfg.Legato = false
	e.Handle(on(2, 61, 90))
	e.Handle(on(3, 70, 80))
	same(t, "legato off: the old note ends first", s.take(), [][]byte{on(1, 60, 90), off(1, 60, 0), bnd(1, bendAt(10*st-4, 24)), on(1, 64, 80)})
	e.Handle([]byte{0xE1, 0, 72}) // +6 steps on the held pad that isn't sounding
	same(t, "a silent pad's bend waits", s.take(), nil)
	e.Handle(off(3, 70, 0))
	same(t, "it counts when that pad sounds again", s.take(), [][]byte{off(1, 64, 0), bnd(1, bendAt(7*st-3, 24)), on(1, 63, 90)})
}

func TestMonoLegatoSameNote(t *testing.T) {
	e, s, _ := clocked(Config{Tuning: edo(31), FirstChan: 1, LastChan: 1, BendRange: 24, MinNote: 24, MaxNote: 120, InBend: 48, Mono: true, Legato: true})
	e.Start()
	s.take()
	e.Handle(on(2, 60, 90))
	e.Handle(on(3, 61, 90)) // both note 60 in 12-TET: only the bend changes
	same(t, "same 12-TET note", s.take(), [][]byte{on(1, 60, 90), bnd(1, bendAt(12.0/31, 24))})
}

func TestVibrato(t *testing.T) {
	w := func(s, g float64) float64 { return s + (g-1)*math.Sin(2*math.Pi*s)/(2*math.Pi) }
	e, s, _ := clocked(Config{Tuning: edo(12), FirstChan: 2, LastChan: 16, BendRange: 48, MaxNote: 127, InBend: 48, Vibrato: 1.8})
	e.Start()
	e.Handle(on(2, 60, 90))
	s.take()
	e.Handle([]byte{0xE1, 64, 64}) // 0.375 pad
	same(t, "widened", s.take(), [][]byte{bnd(2, bendAt(w(0.375, 1.8), 48))})
	e.Handle([]byte{0xE1, 0, 72}) // 6 pads
	same(t, "whole pads land exactly", s.take(), [][]byte{bnd(2, bendAt(6, 48))})
	e.Handle([]byte{0xE1, 0, 64})
	same(t, "back to the touch point", s.take(), [][]byte{bnd(2, 8192)})
}

func TestVibratoFadeIn(t *testing.T) {
	w := func(s, g float64) float64 { return s + (g-1)*math.Sin(2*math.Pi*s)/(2*math.Pi) }
	e, s, now := clocked(Config{Tuning: edo(12), FirstChan: 2, LastChan: 16, BendRange: 48, MaxNote: 127, InBend: 48, Vibrato: 2.5, OnsetMs: 40})
	e.Start()
	e.Handle(on(2, 60, 90))
	s.take()
	e.Handle([]byte{0xE1, 64, 64})
	same(t, "at the strike: the plain bend", s.take(), [][]byte{bnd(2, bendAt(0.375, 48))})
	*now = now.Add(20 * time.Millisecond)
	e.tick()
	same(t, "halfway: gain 1.75", s.take(), [][]byte{bnd(2, bendAt(w(0.375, 1.75), 48))})
	*now = now.Add(20 * time.Millisecond)
	if e.tick() {
		t.Error("the fade should stop at 40 ms")
	}
	same(t, "40 ms: full gain", s.take(), [][]byte{bnd(2, bendAt(w(0.375, 2.5), 48))})
	*now = now.Add(100 * time.Millisecond)
	e.tick()
	same(t, "nothing more", s.take(), nil)
	e.Handle([]byte{0xE1, 0, 72})
	same(t, "6 pads at any gain", s.take(), [][]byte{bnd(2, bendAt(6, 48))})
	e.Handle(on(3, 62, 90))
	e.Handle([]byte{0xE2, 0, 72})
	if m := s.take(); fmt.Sprintf("% x", m[len(m)-1]) != fmt.Sprintf("% x", bnd(3, bendAt(6, 48))) {
		t.Errorf("a slide right after a strike lands exactly: % x", m)
	}
}
