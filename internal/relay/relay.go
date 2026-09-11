// Package relay retunes the LinnStrument's notes for 12-TET instruments with
// no tuning of their own (Kurzweil K2600, Hexinverter Mutant Brain). Each note
// goes out as the nearest 12-TET note plus pitch bend, on an output channel of
// its own, and the LinnStrument's slides are turned into scale steps.
package relay

import (
	"math"
	"slices"
	"sync"

	"github.com/biomassa/linnkit/internal/device"
)

// Tuning places input notes: MIDI note Root is degree 0, and each note above
// or below it is one scale degree.
type Tuning struct {
	Root   int
	Cents  []float64 // degrees 1..N above the root; the last is the period
	Offset float64   // semitones added to every pitch (reference frequency)
}

// Pitch returns the pitch, in fractional MIDI notes, of a scale position in
// degrees from the root. Between degrees the cents interpolate linearly, so a
// slide passes each degree exactly as the pad plays it.
func (t Tuning) Pitch(pos float64) float64 {
	i := math.Floor(pos)
	c0 := t.degreeCents(int(i))
	c1 := t.degreeCents(int(i) + 1)
	return float64(t.Root) + t.Offset + (c0+(pos-i)*(c1-c0))/100
}

func (t Tuning) degreeCents(d int) float64 {
	n := len(t.Cents)
	q := d / n
	if d%n < 0 {
		q--
	}
	r := d - q*n
	c := float64(q) * t.Cents[n-1]
	if r > 0 {
		c += t.Cents[r-1]
	}
	return c
}

func (t Tuning) equal(u Tuning) bool {
	return t.Root == u.Root && t.Offset == u.Offset && slices.Equal(t.Cents, u.Cents)
}

// Target is an instrument the relay drives.
type Target struct {
	Name                string
	FirstChan, LastChan int // output channels 1-16, one note each
	BendRange           int // semitones; set the same on the target
	MinNote, MaxNote    int // notes that would go out beyond these are dropped
	Setup               []string
}

// Targets are the built-in targets. Both instruments take a bend range of 24.
var Targets = []Target{
	{Name: "Kurzweil K2600", FirstChan: 1, LastChan: 16, BendRange: 24, MinNote: 0, MaxNote: 127, Setup: []string{
		"MIDI receive mode Multi, with the same program on every relay channel",
		"in that program (or setup zone): pitch bend range 24 semitones, 0 cents",
		"no intonation table: the relay does the tuning",
		"MIDI cable: the output port's DIN out to the K2600 MIDI in",
	}},
	{Name: "Mutant Brain", FirstChan: 1, LastChan: 4, BendRange: 24, MinNote: 24, MaxNote: 120, Setup: []string{
		"patch: note inputs 1-4 on channels 1-4, last note priority, pitch bend ±24",
		"CV A-D: note input #1-#4, first note pitch, V/Oct; gates 1-4: note input #1-#4, first note on",
		"notes outside MIDI 24-120 are dropped (the module would move them by octaves)",
		"pitch accuracy about ±1.2 cents (12-bit CV)",
	}},
	{Name: "Generic 12-TET synth", FirstChan: 1, LastChan: 16, BendRange: 24, MinNote: 0, MaxNote: 127, Setup: []string{
		"the same sound on every relay channel, pitch bend range 24 semitones",
	}},
}

// Config is one relay setup.
type Config struct {
	Tuning              Tuning
	FirstChan, LastChan int
	BendRange           int
	MinNote, MaxNote    int
	InBend              int  // the LinnStrument's Bend Range: pads of slide per full bend
	SendRPN             bool // send the bend range (RPN 0) to every output channel on start
}

type voice struct {
	ch           int // output channel 0-15
	active       bool
	inCh, inNote int
	outNote      int
	bend         int    // last bend sent; -1 = none yet
	used         uint64 // when the voice last started or ended
}

// Voice is a sounding note, for display.
type Voice struct {
	OutCh, InCh, InNote, OutNote int
	Pitch                        float64 // fractional MIDI note it plays
}

// Stats counts messages and lists the sounding notes.
type Stats struct {
	In, Out, Dropped, Stolen, Clamped int
	Voices                            []Voice
}

// Engine does the retuning. Handle may be called from the MIDI thread.
type Engine struct {
	mu      sync.Mutex
	cfg     Config
	send    func([]byte) error
	voices  []*voice
	inBend  [16]int
	pending [16]map[int][]byte // controller values that came before a channel's note
	dropped map[[2]int]bool
	seq     uint64
	stats   Stats
	err     error
}

// New returns an engine that sends its output through send.
func New(cfg Config, send func([]byte) error) *Engine {
	e := &Engine{cfg: cfg, send: send, dropped: map[[2]int]bool{}}
	for ch := cfg.FirstChan; ch <= cfg.LastChan; ch++ {
		e.voices = append(e.voices, &voice{ch: ch - 1, bend: -1})
	}
	for i := range e.inBend {
		e.inBend[i] = 8192
	}
	return e
}

func (e *Engine) out(msg ...byte) {
	e.stats.Out++
	if err := e.send(msg); err != nil && e.err == nil {
		e.err = err
	}
}

// Start sends every output channel its bend range (when SendRPN) and centres its bend.
func (e *Engine) Start() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, v := range e.voices {
		if e.cfg.SendRPN {
			for _, m := range device.RPN(0, e.cfg.BendRange<<7, v.ch) {
				e.out(m...)
			}
		}
		e.setBend(v, 8192)
	}
	return e.err
}

// Stop releases every sounding note, sends All Notes Off and centres the bends.
func (e *Engine) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, v := range e.voices {
		if v.active {
			e.release(v, 0)
		}
		e.out(0xB0|byte(v.ch), 123, 0)
		e.setBend(v, 8192)
	}
	return e.err
}

// Err returns the first send error.
func (e *Engine) Err() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.err
}

// SetTuning changes the tuning; sounding notes follow at once.
func (e *Engine) SetTuning(t Tuning) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cfg.Tuning.equal(t) {
		return
	}
	e.cfg.Tuning = t
	for _, v := range e.voices {
		if v.active {
			e.retune(v)
		}
	}
}

// Stats returns the counters and the sounding notes in channel order.
func (e *Engine) Stats() Stats {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.stats
	s.Voices = nil
	for _, v := range e.voices {
		if v.active {
			s.Voices = append(s.Voices, Voice{OutCh: v.ch + 1, InCh: v.inCh + 1, InNote: v.inNote, OutNote: v.outNote, Pitch: e.pitch(v.inCh, v.inNote)})
		}
	}
	return s
}

// Handle takes one message from the LinnStrument.
func (e *Engine) Handle(msg []byte) {
	if len(msg) == 0 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stats.In++
	st := msg[0]
	switch {
	case st == 0xF8 || st == 0xFA || st == 0xFB || st == 0xFC: // clock and transport pass through
		e.out(st)
		return
	case st < 0x80 || st >= 0xF0:
		return
	}
	ch := int(st & 0x0F)
	need := 3
	if k := st & 0xF0; k == 0xC0 || k == 0xD0 {
		need = 2
	}
	if len(msg) < need {
		return
	}
	switch st & 0xF0 {
	case 0x90:
		if msg[2] == 0 {
			e.noteOff(ch, int(msg[1]), 0)
		} else {
			e.noteOn(ch, int(msg[1]), int(msg[2]))
		}
	case 0x80:
		e.noteOff(ch, int(msg[1]), int(msg[2]))
	case 0xE0:
		e.inBend[ch] = int(msg[1]) | int(msg[2])<<7
		for _, v := range e.voices {
			if v.active && v.inCh == ch {
				e.retune(v)
			}
		}
	case 0xA0:
		for _, v := range e.voices {
			if v.active && v.inCh == ch && v.inNote == int(msg[1]) {
				e.out(0xA0|byte(v.ch), byte(v.outNote), msg[2])
			}
		}
	case 0xB0:
		if cc := msg[1]; (cc >= 64 && cc <= 69) || cc >= 120 { // pedals and channel mode messages
			e.broadcast(0xB0, msg[1], msg[2])
		} else {
			e.route(ch, 0xB000|int(cc), msg)
		}
	case 0xD0:
		e.route(ch, 0xD000, msg)
	case 0xC0:
		e.broadcast(0xC0, msg[1])
	}
}

func (e *Engine) broadcast(status byte, data ...byte) {
	for _, v := range e.voices {
		e.out(append([]byte{status | byte(v.ch)}, data...)...)
	}
}

// route sends a controller to the notes of its input channel, or keeps it for
// that channel's next note (the LinnStrument sends Y and Z just before a note).
func (e *Engine) route(inCh, key int, msg []byte) {
	sent := false
	for _, v := range e.voices {
		if v.active && v.inCh == inCh {
			e.out(onChannel(msg, v.ch)...)
			sent = true
		}
	}
	if !sent {
		if e.pending[inCh] == nil {
			e.pending[inCh] = map[int][]byte{}
		}
		e.pending[inCh][key] = slices.Clone(msg)
	}
}

func onChannel(msg []byte, ch int) []byte {
	return append([]byte{msg[0]&0xF0 | byte(ch)}, msg[1:]...)
}

func (e *Engine) pads(inCh int) float64 {
	return float64(e.inBend[inCh]-8192) / 8192 * float64(e.cfg.InBend)
}

func (e *Engine) pitch(inCh, note int) float64 {
	return e.cfg.Tuning.Pitch(float64(note-e.cfg.Tuning.Root) + e.pads(inCh))
}

func (e *Engine) noteOn(inCh, note, vel int) {
	key := [2]int{inCh, note}
	for _, v := range e.voices {
		if v.active && v.inCh == inCh && v.inNote == note {
			e.release(v, 0)
		}
	}
	delete(e.dropped, key)
	outNote := int(math.Round(e.pitch(inCh, note)))
	if outNote < e.cfg.MinNote || outNote > e.cfg.MaxNote {
		e.stats.Dropped++
		e.dropped[key] = true
		return
	}
	v := e.pick()
	if v.active {
		e.stats.Stolen++
		e.release(v, 0)
	}
	e.seq++
	v.active, v.inCh, v.inNote, v.outNote, v.used = true, inCh, note, outNote, e.seq
	for _, m := range e.pending[inCh] {
		e.out(onChannel(m, v.ch)...)
	}
	e.pending[inCh] = nil
	e.retune(v)
	e.out(0x90|byte(v.ch), byte(outNote), byte(vel))
}

func (e *Engine) noteOff(inCh, note, vel int) {
	key := [2]int{inCh, note}
	if e.dropped[key] {
		delete(e.dropped, key)
		return
	}
	for _, v := range e.voices {
		if v.active && v.inCh == inCh && v.inNote == note {
			e.release(v, vel)
		}
	}
}

// pick returns the free channel that has been quiet longest, so release tails
// keep their pitch, or else the note that started first.
func (e *Engine) pick() *voice {
	var best *voice
	for _, v := range e.voices {
		if !v.active && (best == nil || v.used < best.used) {
			best = v
		}
	}
	if best != nil {
		return best
	}
	for _, v := range e.voices {
		if best == nil || v.used < best.used {
			best = v
		}
	}
	return best
}

func (e *Engine) release(v *voice, vel int) {
	e.out(0x80|byte(v.ch), byte(v.outNote), byte(vel))
	e.seq++
	v.active, v.used = false, e.seq
}

// retune sends the bend that moves the voice's 12-TET note to its pitch.
func (e *Engine) retune(v *voice) {
	semis := e.pitch(v.inCh, v.inNote) - float64(v.outNote)
	val := 8192 + int(math.Round(semis/float64(e.cfg.BendRange)*8192))
	if val < 0 || val > 16383 {
		e.stats.Clamped++
		val = max(0, min(16383, val))
	}
	e.setBend(v, val)
}

func (e *Engine) setBend(v *voice, val int) {
	if v.bend == val {
		return
	}
	v.bend = val
	e.out(0xE0|byte(v.ch), byte(val&0x7F), byte(val>>7))
}

// Runner connects an engine to MIDI ports.
type Runner struct {
	E       *Engine
	Out     string // output port name
	in, out device.Port
}

// Run opens outName, starts the engine and listens to inName.
func Run(cfg Config, inName, outName string) (*Runner, error) {
	out, err := device.OpenOutput(outName)
	if err != nil {
		return nil, err
	}
	e := New(cfg, out.Send)
	if err := e.Start(); err != nil {
		out.Close()
		return nil, err
	}
	in, err := device.OpenInput(inName)
	if err == nil {
		err = in.SetListener(e.Handle)
		if err != nil {
			in.Close()
		}
	}
	if err != nil {
		e.Stop()
		out.Close()
		return nil, err
	}
	return &Runner{E: e, Out: outName, in: in, out: out}, nil
}

// Stop stops listening, silences the target and closes the ports.
func (r *Runner) Stop() error {
	r.in.Close()
	err := r.E.Stop()
	r.out.Close()
	return err
}
