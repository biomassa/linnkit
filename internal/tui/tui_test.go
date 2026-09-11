package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/biomassa/linnkit/internal/testutil"
)

// ready returns a dashboard with the repo's SCL files loaded, at 160x50.
// Device commands never run: Init is not called and nothing is confirmed.
func ready(t *testing.T) Model {
	t.Helper()
	m := New(Config{ScaleDirs: []string{filepath.Join(testutil.RepoRoot(t), "SCL")}})
	next, _ := m.Update(loadScales(m.cfg.ScaleDirs)())
	next, _ = next.Update(tea.WindowSizeMsg{Width: MinWidth, Height: MinHeight})
	return next.(Model)
}

func press(t *testing.T, m Model, keys ...tea.KeyPressMsg) Model {
	t.Helper()
	for _, k := range keys {
		next, _ := m.Update(k)
		m = next.(Model)
	}
	return m
}

var (
	down  = tea.KeyPressMsg{Code: tea.KeyDown}
	up    = tea.KeyPressMsg{Code: tea.KeyUp}
	tab   = tea.KeyPressMsg{Code: tea.KeyTab}
	enter = tea.KeyPressMsg{Code: tea.KeyEnter}
	esc   = tea.KeyPressMsg{Code: tea.KeyEscape}
)

func char(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

func TestLoadsFirstScaleAndDrawsDashboard(t *testing.T) {
	m := ready(t)
	if m.analysis == nil || m.analysis.Scale.Name != "22edo" { // first alphabetically
		t.Fatalf("loaded %v", m.loaded)
	}
	out := m.View().Content
	lines := strings.Split(out, "\n")
	if len(lines) != MinHeight {
		t.Errorf("%d lines, want %d", len(lines), MinHeight)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != MinWidth {
			t.Errorf("line %d is %d cells wide, want %d", i+1, w, MinWidth)
			break
		}
	}
	for _, s := range []string{"SCALES", "SCALE", "LAYOUT", "LIGHTS", "GRID", "SEND", "22 equal", "rows +7"} {
		if !strings.Contains(ansi.Strip(out), s) {
			t.Errorf("dashboard lacks %q", s)
		}
	}
}

func TestSmallWindow(t *testing.T) {
	m := ready(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if out := next.(Model).View().Content; !strings.Contains(out, "needs at least 160x50") {
		t.Errorf("small window: %q", out)
	}
}

func TestPickScaleLayoutAndLights(t *testing.T) {
	m := ready(t)
	m = press(t, m, down, enter) // 31-edo
	if m.analysis.Scale.Name != "31-edo" || m.lay.Offset != 10 || m.lay.RowStart[0] != 30 {
		t.Fatalf("31-edo: %s rows +%d from %d", m.analysis.Scale.Name, m.lay.Offset, m.lay.RowStart[0])
	}
	m = press(t, m, tab, tab, down) // LAYOUT: second candidate
	if m.focus != paneLayout || m.lay.Offset != 8 {
		t.Errorf("layout pane: focus %v rows +%d", m.focus, m.lay.Offset)
	}
	m = press(t, m, char(']'))
	if m.lay.RowStart[0] != m.cands[1].RowStart[0]+1 {
		t.Errorf("] should raise the bottom-left note: %d", m.lay.RowStart[0])
	}
	m = press(t, m, tab, down) // LIGHTS: note names
	if schemes[m.scheme] != "names" {
		t.Fatalf("scheme %s", schemes[m.scheme])
	}
	found := false
	for _, row := range m.surface {
		for _, p := range row {
			if p.Note == 60 {
				found = true
				if p.Label != "C" {
					t.Errorf("the pad playing MIDI 60 should be labelled C: %+v", p)
				}
			}
		}
	}
	if !found {
		t.Error("no pad plays MIDI 60")
	}
}

func TestSynthPaneSetsBendRange(t *testing.T) {
	m := ready(t)
	m = press(t, m, down, enter) // 31-edo
	m.focus = paneSynth
	if j := m.job(); j.Config == nil || j.Config.Bend != 31 || j.Config.OneChannel {
		t.Fatalf("Aalto, S 12, 31-EDO: %+v", j.Config)
	}
	m = press(t, m, char('>')) // Aalto: S 24
	if m.synthBend != 24 || m.job().Config.Bend != 62 {
		t.Errorf("S 24: B %d", m.job().Config.Bend)
	}
	for m.profile().Name != "Legacy (non-MPE)" {
		m = press(t, m, down)
	}
	if j := m.job(); !j.Config.OneChannel || m.synthBend != 2 {
		t.Errorf("legacy: %+v S %d", j.Config, m.synthBend)
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "SYNTH") {
		t.Error("dashboard lacks the SYNTH pane")
	}
}

func TestFilter(t *testing.T) {
	m := ready(t)
	m = press(t, m, char('/'), char('c'), char('o'), char('h'), enter)
	vis := m.visible()
	if len(vis) != 2 || m.filtering {
		t.Errorf("filter coh: %d visible, filtering %v", len(vis), m.filtering)
	}
	m = press(t, m, enter)
	if m.analysis.Scale.Name != "ji_8coh" {
		t.Errorf("loaded %s", m.analysis.Scale.Name)
	}
}

func TestOverlaysAndSendNeedsConfirmation(t *testing.T) {
	m := ready(t)
	m = press(t, m, char('m'))
	if m.overlay != overlayMatrix || !strings.Contains(ansi.Strip(m.View().Content), "INTERVAL MATRIX") {
		t.Fatal("matrix overlay")
	}
	m = press(t, m, esc, char('s'))
	if m.overlay != overlayConfirmSend || !strings.Contains(ansi.Strip(m.View().Content), "light slot 2") {
		t.Fatal("send confirmation should show the target slot")
	}
	next, cmd := m.Update(char('n'))
	if m = next.(Model); m.overlay != noOverlay || m.busy || cmd != nil {
		t.Errorf("cancel must not send: busy %v cmd %v", m.busy, cmd != nil)
	}
	m = press(t, m, char('1'), char('s'))
	if !strings.Contains(ansi.Strip(m.View().Content), "not the scratch slot") {
		t.Error("slot 1 should carry a warning")
	}
}
