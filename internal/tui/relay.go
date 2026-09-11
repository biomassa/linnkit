package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/biomassa/linnkit/internal/device"
	"github.com/biomassa/linnkit/internal/relay"
	"github.com/biomassa/linnkit/internal/scala"
	"github.com/biomassa/linnkit/internal/store"
)

type (
	relayStartedMsg struct {
		run    *relay.Runner
		inBend int
		note   string
		err    error
	}
	relayTickMsg struct{ id int }
)

const relayFields = 6

func relayTick(id int) tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg { return relayTickMsg{id} })
}

// openRelay shows the relay window.
func (m Model) openRelay() (Model, tea.Cmd) {
	m.overlay = overlayRelay
	m.relayTickID++
	if m.relayRun != nil {
		return m, relayTick(m.relayTickID)
	}
	list := device.OutputPorts
	if m.cfg.ListPorts != nil {
		list = m.cfg.ListPorts
	}
	m.relayPorts = nil
	for _, p := range list() {
		if !strings.Contains(p, m.cfg.Port) { // never back into the LinnStrument
			m.relayPorts = append(m.relayPorts, p)
		}
	}
	m.relayPort = 0
	for i, p := range m.relayPorts {
		if p == m.relayPortName {
			m.relayPort = i
			break
		}
		if m.relayPortName == "" && strings.Contains(p, "UltraLite") {
			m.relayPort = i
		}
	}
	return m, nil
}

func (m Model) stopRelay() Model {
	if m.relayRun != nil {
		if err := m.relayRun.Stop(); err != nil {
			m.relayMsg = "stopped: " + err.Error()
		} else {
			m.relayMsg = "stopped"
		}
		m.relayRun = nil
	}
	return m
}

// relayTuning is the dashboard's scale, root and reference frequency.
func (m Model) relayTuning() relay.Tuning {
	cents := make([]float64, len(m.analysis.Scale.Pitches))
	for i, p := range m.analysis.Scale.Pitches {
		cents[i] = p.Cents
	}
	off := 12 * math.Log2(refFreq(m.root, m.refHz)/scala.StandardFreq(m.root))
	return relay.Tuning{Root: m.root, Cents: cents, Offset: off}
}

func (m Model) relayConfig() relay.Config {
	t := relay.Targets[m.relayTarget]
	return relay.Config{Tuning: m.relayTuning(), FirstChan: m.relayFirst, LastChan: m.relayLast, BendRange: m.relayBend,
		MinNote: t.MinNote, MaxNote: t.MaxNote, InBend: 24, SendRPN: m.relayRPN}
}

// relayStartCmd reads the LinnStrument's Bend Range, then starts the relay.
func relayStartCmd(linn, out string, cfg relay.Config) tea.Cmd {
	return func() tea.Msg {
		note := "the LinnStrument did not answer; assuming Bend Range 24"
		if d, err := openDevice(linn); err == nil {
			v, _ := d.ReadRetry([]int{device.ParamBendRange}, 2*time.Second, 1)
			d.Close()
			if b, ok := v[device.ParamBendRange]; ok {
				cfg.InBend, note = b, ""
			}
		}
		r, err := relay.Run(cfg, linn, out)
		return relayStartedMsg{run: r, inBend: cfg.InBend, note: note, err: err}
	}
}

func (m Model) setRelayTarget(i int) Model {
	t := relay.Targets[i]
	m.relayTarget, m.relayFirst, m.relayLast, m.relayBend = i, t.FirstChan, t.LastChan, t.BendRange
	return m
}

func (m Model) saveRelaySettings() Model {
	if m.cfg.Store == nil {
		return m
	}
	c, err := m.cfg.Store.Config()
	if err == nil {
		c.Relay = &store.RelaySettings{Target: relay.Targets[m.relayTarget].Name, Port: m.relayPortName,
			First: m.relayFirst, Last: m.relayLast, Bend: m.relayBend, NoRPN: !m.relayRPN}
		err = m.cfg.Store.SaveConfig(c)
	}
	if err != nil {
		m.relayMsg = "settings not saved: " + err.Error()
	}
	return m
}

// loadRelaySettings applies the saved relay settings, if any.
func (m Model) loadRelaySettings(r *store.RelaySettings) Model {
	for i, t := range relay.Targets {
		if t.Name == r.Target {
			m = m.setRelayTarget(i)
		}
	}
	if r.First >= 1 && r.Last <= 16 && r.First <= r.Last {
		m.relayFirst, m.relayLast = r.First, r.Last
	}
	if r.Bend > 0 {
		m.relayBend = r.Bend
	}
	m.relayPortName, m.relayRPN = r.Port, !r.NoRPN
	return m
}

func (m Model) relayKey(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "esc", "R", "q":
		m.overlay = noOverlay
	case "space", " ", "enter":
		if m.relayRun != nil {
			return m.stopRelay(), nil
		}
		switch {
		case m.analysis == nil:
			m.relayMsg = "load a scale first"
		case len(m.relayPorts) == 0:
			m.relayMsg = "no MIDI output besides the LinnStrument"
		default:
			m.relayPortName = m.relayPorts[m.relayPort]
			m = m.saveRelaySettings()
			m.relayMsg = "starting"
			return m, relayStartCmd(m.cfg.Port, m.relayPortName, m.relayConfig())
		}
	case "up":
		m.relayField = max(0, m.relayField-1)
	case "down":
		m.relayField = min(relayFields-1, m.relayField+1)
	case "left", "right":
		if m.relayRun != nil {
			m.relayMsg = "stop the relay (Space) to change its settings"
			return m, nil
		}
		d := 1
		if k == "left" {
			d = -1
		}
		switch m.relayField {
		case 0:
			m = m.setRelayTarget((m.relayTarget + d + len(relay.Targets)) % len(relay.Targets))
		case 1:
			if n := len(m.relayPorts); n > 0 {
				m.relayPort = (m.relayPort + d + n) % n
				m.relayPortName = m.relayPorts[m.relayPort]
			}
		case 2:
			m.relayFirst = max(1, min(m.relayLast, m.relayFirst+d))
		case 3:
			m.relayLast = max(m.relayFirst, min(16, m.relayLast+d))
		case 4:
			m.relayBend = max(1, min(96, m.relayBend+d))
		case 5:
			m.relayRPN = !m.relayRPN
		}
	}
	return m, nil
}

func (m Model) relayLines() []string {
	t := relay.Targets[m.relayTarget]
	port := "(no MIDI output found)"
	if len(m.relayPorts) > 0 {
		port = m.relayPorts[m.relayPort]
	}
	if m.relayRun != nil {
		port = m.relayRun.Out
	}
	out := []string{"",
		" LinnStrument (USB) → linnkit: nearest 12-TET note + pitch bend, one channel per note → " + port + " → " + t.Name, ""}
	if a := m.analysis; a != nil {
		out = append(out, " "+labelStyle.Render(fmt.Sprintf("%-19s", "scale"))+valueStyle.Render(a.Scale.Name)+
			fmt.Sprintf(", %d notes, root %d %s, %.3f Hz", a.Structure.Size, m.root, midiName(m.root), refFreq(m.root, m.refHz))+
			dimStyle.Render("   from the dashboard; changes reach a running relay at once"))
	}
	fields := []struct{ label, value, hint string }{
		{"target", t.Name, "K2600, Mutant Brain, generic"},
		{"output port", port, "every MIDI output but the LinnStrument"},
		{"first channel", fmt.Sprint(m.relayFirst), ""},
		{"last channel", fmt.Sprintf("%d   (%d voices)", m.relayLast, m.relayLast-m.relayFirst+1), ""},
		{"bend range", fmt.Sprintf("%d semitones", m.relayBend), "set the same on the target"},
		{"send bend range", yesNo(m.relayRPN), "RPN 0 to every channel on start"},
	}
	for i, f := range fields {
		mark := "   "
		if i == m.relayField {
			mark = focusStyle.Render(" ▸ ")
		}
		line := mark + labelStyle.Render(fmt.Sprintf("%-17s", f.label)) + valueStyle.Render(f.value)
		if f.hint != "" {
			line += dimStyle.Render("   " + f.hint)
		}
		out = append(out, line)
	}
	out = append(out, "")
	if r := m.relayRun; r != nil {
		s := r.E.Stats()
		out = append(out, " "+goodStyle.Render("● running")+dimStyle.Render("   Space stops"),
			fmt.Sprintf(" LinnStrument Bend Range %d: one pad of slide = one scale step", m.relayInBend),
			fmt.Sprintf(" messages in %d, out %d   dropped %d   stolen %d   bend clamped %d", s.In, s.Out, s.Dropped, s.Stolen, s.Clamped))
		if err := r.E.Err(); err != nil {
			out = append(out, warnStyle.Render(" send error: "+err.Error()))
		}
		out = append(out, "", boldStyle.Render(" out ch   in ch  in note   out note   + cents"))
		for _, v := range s.Voices {
			out = append(out, fmt.Sprintf(" %6d   %5d  %7d   %4d %-4s  %+8.2f", v.OutCh, v.InCh, v.InNote, v.OutNote, midiName(v.OutNote),
				(v.Pitch-float64(v.OutNote))*100))
		}
		if len(s.Voices) == 0 {
			out = append(out, dimStyle.Render(" (no notes sounding)"))
		}
	} else {
		out = append(out, " "+dimStyle.Render("○ stopped   Space starts"))
	}
	if m.relayMsg != "" {
		out = append(out, " "+m.relayMsg)
	}
	out = append(out, "", boldStyle.Render(" Set up "+t.Name))
	for _, s := range t.Setup {
		out = append(out, "  · "+s)
	}
	out = append(out, `  · LinnStrument: send with the synth profile "linnkit relay" (Channel Per Note, Bend Range 24)`,
		"", dimStyle.Render(" ↑↓ field   ←→ change   Space start/stop   Esc close (the relay keeps running)"))
	return out
}
