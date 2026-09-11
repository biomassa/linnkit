package relay

import (
	"math"
	"testing"
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
