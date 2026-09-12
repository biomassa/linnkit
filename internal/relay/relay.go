// Package relay retunes the LinnStrument's notes for 12-TET instruments with
// no tuning of their own (Kurzweil K2600, Hexinverter Mutant Brain). Each note
// goes out as the nearest 12-TET note plus pitch bend, on an output channel of
// its own, and the LinnStrument's slides are turned into scale steps.
package relay

import (
	"math"
	"slices"
	"sync"
	"time"

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
	FirstChan, LastChan int  // output channels 1-16, one note each
	BendRange           int  // semitones; set the same on the target
	MinNote, MaxNote    int  // notes that would go out beyond these are dropped
	Scala               bool // the synth has the scale: notes pass unchanged, only bends are converted
	Mono                bool // one voice on FirstChan: the newest held pad sounds
	Setup               []string
}

// Targets are the built-in targets. Both instruments take a bend range of 24.
var Targets = []Target{
	{Name: "Kurzweil K2600", FirstChan: 1, LastChan: 16, BendRange: 24, MinNote: 0, MaxNote: 127, Setup: []string{
		"MIDI receive mode Multi, with the same program on every relay channel",
		"in that program (or setup zone): pitch bend range 24 semitones = 2400 cents",
		"no intonation table: the relay does the tuning",
		"MIDI cable: the output port's DIN out to the K2600 MIDI in",
	}},
	{Name: "Mutant Brain", FirstChan: 1, LastChan: 1, BendRange: 24, MinNote: 24, MaxNote: 120, Mono: true, Setup: []string{
		"mono on channel 1: the newest held pad sounds; releasing it returns to the newest still held",
		"patch: note input 1 on channel 1, last note priority, pitch bend ±24; gate 1: note input 1, first note on",
		"CV A: note input 1 pitch (V/Oct, with the bend); B: note input 1 velocity; C: channel aftertouch ch 1 (Z); D: CC74 ch 1 (Y)",
		"notes outside MIDI 24-120 are dropped (the module would move them by octaves); pitch accuracy about ±1.2 cents",
	}},
	{Name: "Generic 12-TET synth", FirstChan: 1, LastChan: 16, BendRange: 24, MinNote: 0, MaxNote: 127, Setup: []string{
		"the same sound on every relay channel, pitch bend range 24 semitones",
	}},
	{Name: "Scala synth", Scala: true, FirstChan: 2, LastChan: 16, BendRange: 48, MinNote: 0, MaxNote: 127, Setup: []string{
		"for synths that load the scale themselves: Aalto, Kaivo, Surge XT, Pigments",
		"on start the relay exports the scale + .kbm to ~/Music/Madrona Labs/Scales/linnkit: load them in the synth",
		"synth: MPE on, per-note bend range = the relay's bend range (Aalto: gear menu, MPE bend range)",
		"Pigments reads the .scl only: set its root note to the relay's root in the Load .SCL dialog",
		"DAW / synth MIDI input: the output port (IAC bus), not the LinnStrument",
		"notes pass unchanged; slides are converted, so they land on every pad",
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
	// Scala: the synth plays the scale itself (a .scl + .kbm with degree 0 on the
	// root). Note numbers pass unchanged and each bend becomes the synth bend
	// that reaches the same scale position (shared spec scala-synth-bends.md).
	Scala bool
	// Vibrato widens small bends around each pad by this gain (1 = off, up to 3):
	// the bend in pads s becomes s + (g-1) sin(2 pi s) / (2 pi), so whole pads
	// still land exactly. OnsetMs fades the gain in from 1 after each strike.
	Vibrato float64
	OnsetMs int
	Voices  int  // at most this many notes, on the first channels (0 = all)
	Mono    bool // one voice on FirstChan (Mutant Brain)
	Legato  bool // mono: a new note starts before the old one ends (the gate stays high)
}

type voice struct {
	ch           int // output channel 0-15
	active       bool
	inCh, inNote int
	outNote      int
	bend         int       // last bend sent; -1 = none yet
	used         uint64    // when the voice last started or ended
	t0           time.Time // when its note was struck (for the vibrato fade-in)
	faded        bool      // the vibrato gain has fully faded in
}

// heldNote is a pad held in mono mode.
type heldNote struct {
	c, note, vel int
	t0           time.Time
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
	tab     [128]float64 // Scala mode: cents of MIDI notes 0..127 under the tuning
	stack   []heldNote   // mono: held pads, oldest first
	now     func() time.Time
	ramping bool // a 5 ms ticker re-tunes voices whose vibrato is fading in
	manual  bool // tests drive the ticker by hand
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
	e.tab = cfg.Tuning.table()
	e.now = time.Now
	return e
}

// table returns the cents (from MIDI 0) of every MIDI note under the tuning.
func (t Tuning) table() [128]float64 {
	var tab [128]float64
	if len(t.Cents) == 0 {
		return tab
	}
	for m := range tab {
		tab[m] = t.Pitch(float64(m-t.Root)) * 100
	}
	return tab
}

// tablePitch returns the cents at fractional MIDI position x: linear between
// neighbouring notes, extrapolated with the end steps beyond 0..127.
func tablePitch(tab *[128]float64, x float64) float64 {
	switch {
	case x <= 0:
		return tab[0] + x*(tab[1]-tab[0])
	case x >= 127:
		return tab[127] + (x-127)*(tab[127]-tab[126])
	}
	i := int(math.Floor(x))
	return tab[i] + (x-float64(i))*(tab[i+1]-tab[i])
}

// scalaCents is the pitch change of a voice's slide in Scala mode.
func (e *Engine) scalaCents(v *voice) float64 {
	n := float64(v.inNote)
	return tablePitch(&e.tab, n+e.pads(v.inCh)) - tablePitch(&e.tab, n)
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
	for _, v := range e.pool() {
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
	for _, v := range e.pool() {
		if v.active {
			e.release(v, 0)
		}
		e.out(0xB0|byte(v.ch), 123, 0)
		e.setBend(v, 8192)
	}
	e.stack = nil
	for i := range e.pending {
		e.pending[i] = nil
	}
	e.dropped = map[[2]int]bool{}
	return e.err
}

// pool is the output channels in use: all of them, the first Voices, or one in mono.
func (e *Engine) pool() []*voice {
	n := len(e.voices)
	switch {
	case e.cfg.Mono:
		n = min(n, 1)
	case e.cfg.Voices > 0:
		n = min(n, e.cfg.Voices)
	}
	return e.voices[:n]
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
	e.tab = t.table()
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
			p := e.pitch(v.inCh, v.inNote)
			if e.cfg.Scala {
				p = float64(v.outNote) + e.scalaCents(v)/100
			}
			s.Voices = append(s.Voices, Voice{OutCh: v.ch + 1, InCh: v.inCh + 1, InNote: v.inNote, OutNote: v.outNote, Pitch: p})
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
	for _, v := range e.pool() {
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

// outNote is the note a pad goes out as: the nearest 12-TET note, or its own (Scala).
func (e *Engine) outNote(inCh, note int) int {
	if e.cfg.Scala {
		return note
	}
	return roundHalfUp(e.pitch(inCh, note))
}

func (e *Engine) noteOn(inCh, note, vel int) {
	key := [2]int{inCh, note}
	delete(e.dropped, key)
	out := e.outNote(inCh, note)
	if out < e.cfg.MinNote || out > e.cfg.MaxNote {
		e.stats.Dropped++
		e.dropped[key] = true
		return
	}
	if e.cfg.Mono {
		e.monoOn(inCh, note, vel)
		return
	}
	for _, v := range e.pool() {
		if v.active && v.inCh == inCh && v.inNote == note {
			e.release(v, 0)
		}
	}
	v := e.pick()
	if v == nil {
		return
	}
	if v.active {
		e.stats.Stolen++
		e.release(v, 0)
	}
	e.start(v, inCh, note, out, e.now())
	e.out(0x90|byte(v.ch), byte(out), byte(vel))
}

// start gives a voice its note (struck at t0), sends what the input channel
// held back, then the bend.
func (e *Engine) start(v *voice, inCh, note, out int, t0 time.Time) {
	faded := e.cfg.OnsetMs <= 0 || e.vibrato() <= 1 || e.now().Sub(t0) >= e.onset()
	e.seq++
	v.active, v.inCh, v.inNote, v.outNote, v.used, v.t0, v.faded = true, inCh, note, out, e.seq, t0, faded
	for _, m := range e.pending[inCh] {
		e.out(onChannel(m, v.ch)...)
	}
	e.pending[inCh] = nil
	e.retune(v)
	if !faded {
		e.startRamp()
	}
}

// Mono (Mutant Brain): one voice; the newest held pad sounds, and releasing it
// returns to the newest still held. Legato: the new note starts before the old
// one ends (the gate stays high); else the old one ends first (retrigger).
// Bend, pressure and Y come from the pad that sounds.

func (e *Engine) monoOn(c, note, vel int) {
	e.stack = slices.DeleteFunc(e.stack, func(s heldNote) bool { return s.c == c && s.note == note })
	e.stack = append(e.stack, heldNote{c, note, vel, e.now()})
	e.sound(e.stack[len(e.stack)-1])
}

func (e *Engine) monoOff(c, note, vel int) {
	i := slices.IndexFunc(e.stack, func(s heldNote) bool { return s.c == c && s.note == note })
	if i < 0 {
		return
	}
	sounding := i == len(e.stack)-1
	e.stack = slices.Delete(e.stack, i, i+1)
	if !sounding || len(e.pool()) == 0 {
		return
	}
	if len(e.stack) > 0 {
		e.sound(e.stack[len(e.stack)-1])
	} else if v := e.pool()[0]; v.active {
		e.release(v, vel)
	}
}

func (e *Engine) sound(s heldNote) {
	if len(e.pool()) == 0 {
		return
	}
	v := e.pool()[0]
	out := e.outNote(s.c, s.note)
	if v.active && e.cfg.Legato {
		old := v.outNote
		e.start(v, s.c, s.note, out, s.t0)
		if out != old {
			e.out(0x90|byte(v.ch), byte(out), byte(s.vel))
			e.out(0x80|byte(v.ch), byte(old), 0)
		}
		return
	}
	if v.active {
		e.release(v, 0)
	}
	e.start(v, s.c, s.note, out, s.t0)
	e.out(0x90|byte(v.ch), byte(out), byte(s.vel))
}

// vibrato is the configured gain, 1..3 (unset = 1, off).
func (e *Engine) vibrato() float64 { return min(3, max(1, e.cfg.Vibrato)) }

func (e *Engine) onset() time.Duration { return time.Duration(e.cfg.OnsetMs) * time.Millisecond }

// gainAt is a voice's vibrato gain: from 1 at the strike up to the full gain
// after OnsetMs, so the wobble of a landing finger isn't widened.
func (e *Engine) gainAt(v *voice) float64 {
	g := e.vibrato()
	if e.cfg.OnsetMs <= 0 || v.faded {
		return g
	}
	return 1 + (g-1)*min(1, float64(e.now().Sub(v.t0))/float64(e.onset()))
}

// warp widens a bend of s pads around each pad: whole and half pads stay put.
func warp(s, g float64) float64 { return s + (g-1)*math.Sin(2*math.Pi*s)/(2*math.Pi) }

// startRamp re-tunes fading voices every 5 ms (a finger held still sends no bends).
func (e *Engine) startRamp() {
	if e.ramping {
		return
	}
	e.ramping = true
	if e.manual {
		return
	}
	go func() {
		t := time.NewTicker(5 * time.Millisecond)
		defer t.Stop()
		for range t.C {
			e.mu.Lock()
			more := e.rampTick()
			e.mu.Unlock()
			if !more {
				return
			}
		}
	}()
}

// rampTick re-tunes the voices still fading in; false when none is left.
func (e *Engine) rampTick() bool {
	now, any := e.now(), false
	for _, v := range e.voices {
		if !v.active || v.faded {
			continue
		}
		if now.Sub(v.t0) >= e.onset() {
			v.faded = true
		} else {
			any = true
		}
		e.retune(v)
	}
	if !any {
		e.ramping = false
	}
	return any
}

func (e *Engine) noteOff(inCh, note, vel int) {
	key := [2]int{inCh, note}
	if e.dropped[key] {
		delete(e.dropped, key)
		return
	}
	if e.cfg.Mono {
		e.monoOff(inCh, note, vel)
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
	for _, v := range e.pool() {
		if !v.active && (best == nil || v.used < best.used) {
			best = v
		}
	}
	if best != nil {
		return best
	}
	for _, v := range e.pool() {
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
	steps := warp(e.pads(v.inCh), e.gainAt(v))
	var semis float64
	if e.cfg.Scala { // from the pitch the synth plays for the note to the pitch the slide reached
		n := float64(v.inNote)
		semis = (tablePitch(&e.tab, n+steps) - tablePitch(&e.tab, n)) / 100
	} else { // from the voice's 12-TET note to its pitch
		semis = e.cfg.Tuning.Pitch(float64(v.inNote-e.cfg.Tuning.Root)+steps) - float64(v.outNote)
	}
	val := 8192 + roundHalfUp(semis/float64(e.cfg.BendRange)*8192)
	if val < 0 || val > 16383 {
		e.stats.Clamped++
		val = max(0, min(16383, val))
	}
	e.setBend(v, val)
}

// roundHalfUp rounds like JavaScript's Math.round (halves go up: -2.5 -> -2),
// so both relay routes give the same bends as linn.retune (Max).
func roundHalfUp(x float64) int { return int(math.Floor(x + 0.5)) }

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
