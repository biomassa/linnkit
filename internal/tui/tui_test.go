package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/biomassa/linnkit/internal/export"
	"github.com/biomassa/linnkit/internal/lights"
	"github.com/biomassa/linnkit/internal/scala"
	"github.com/biomassa/linnkit/internal/store"
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

func withStore(t *testing.T, st *store.Store) Model {
	t.Helper()
	m := New(Config{ScaleDirs: []string{filepath.Join(testutil.RepoRoot(t), "SCL")}, Store: st})
	next, _ := m.Update(loadScales(m.cfg.ScaleDirs)())
	next, _ = next.Update(tea.WindowSizeMsg{Width: MinWidth, Height: MinHeight})
	return next.(Model)
}

func TestScaleSettingsPersist(t *testing.T) {
	st, _ := store.Open(t.TempDir())
	m := withStore(t, st)
	m = press(t, m, down, enter)          // 31-edo
	m = press(t, m, tab, tab, down)       // LAYOUT: second candidate (+8)
	m = press(t, m, tab, down)            // LIGHTS: note names
	m = press(t, m, tab, down, char('>')) // SYNTH: Pigments, S up from 12
	want := m.settings()

	m2 := withStore(t, st)
	if m2.profile().Name != "Pigments" {
		t.Errorf("new run should default to the last synth, got %s", m2.profile().Name)
	}
	m2 = press(t, m2, down, enter)
	if got := m2.settings(); !equalSettings(got, want) {
		t.Errorf("31-edo settings not restored:\n got %+v\nwant %+v", got, want)
	}
	if m2.lay.Offset != 8 || schemes[m2.scheme] != "names" {
		t.Errorf("layout +%d scheme %s", m2.lay.Offset, schemes[m2.scheme])
	}
}

func TestPresets(t *testing.T) {
	st, _ := store.Open(t.TempDir())
	m := withStore(t, st)
	m = press(t, m, down, enter)    // 31-edo
	m = press(t, m, tab, tab, down) // +8
	m = press(t, m, char('1'), char('p'), char('a'))
	if !m.naming || m.name != "31-edo Aalto / Kaivo" {
		t.Fatalf("naming %v %q", m.naming, m.name)
	}
	m = press(t, m, tea.KeyPressMsg{Code: tea.KeyBackspace}, char('x'), enter)
	if !st.HasPreset("31-edo Aalto / Kaivx") || len(m.presets) != 1 {
		t.Fatalf("preset not saved: %v", m.status)
	}
	m = press(t, m, char('a'), tea.KeyPressMsg{Code: tea.KeyBackspace}, char('x'), enter) // same name: asks first
	if !m.confirmRep {
		t.Error("an existing name should ask before replacing")
	}
	m = press(t, m, char('n'), esc, esc)

	m = press(t, m, char('0'))
	m.focus = paneScales
	m = press(t, m, up, enter) // 22edo
	m = press(t, m, char('p'), enter)
	if m.analysis.Scale.Name != "31-edo" || m.lay.Offset != 8 || m.slot != 1 || m.overlay != noOverlay {
		t.Errorf("preset load: %s +%d slot %d", m.analysis.Scale.Name, m.lay.Offset, m.slot)
	}
	m = press(t, m, char('p'), char('d'), char('y'))
	if len(m.presets) != 0 {
		t.Error("delete")
	}
}

func TestFactoryLayout(t *testing.T) {
	m := ready(t)
	m = press(t, m, down, enter, char('f'))
	j := m.job()
	if !j.Factory || j.Layout != nil || j.Config == nil || j.Config.Bend != 12 {
		t.Fatalf("factory job: %+v %+v", j, j.Config)
	}
	if m.surface[0][0].Note != 30 || m.surface[1][0].Note != 35 {
		t.Errorf("factory grid starts %d, %d", m.surface[0][0].Note, m.surface[1][0].Note)
	}
	for _, row := range m.surface {
		for _, p := range row {
			if p.Note%12 == 0 && p.Color != lights.Cyan || p.Note%12 == 1 && p.Color != lights.Off {
				t.Fatalf("pad %+v", p)
			}
		}
	}
	m = press(t, m, char('s'))
	if out := ansi.Strip(m.View().Content); !strings.Contains(out, "factory 12-TET") || !strings.Contains(out, "light slots: unchanged") {
		t.Error("confirmation should describe the factory send")
	}
	m = press(t, m, char('n'), char('f'))
	if m.job().Factory || m.lay.Offset != 10 {
		t.Error("f again returns to the scale layout")
	}
}

func TestRootAndExport(t *testing.T) {
	st, _ := store.Open(t.TempDir())
	m := withStore(t, st)
	m.cfg.ExportDir = filepath.Join(t.TempDir(), "linnkit")
	m = press(t, m, down, enter) // 31-edo
	m.focus = paneLayout
	m = press(t, m, char('}'), char('}'))
	if m.root != 62 || m.lay.Offset != 10 {
		t.Fatalf("root %d offset %d", m.root, m.lay.Offset)
	}
	for _, row := range m.surface {
		for _, p := range row {
			if p.Note == 62 && p.Degree != 0 {
				t.Errorf("MIDI 62 should play degree 0: %+v", p)
			}
		}
	}
	m = press(t, m, char('e'), char('h'))
	if !m.typingHz || m.hzText != "293.665" {
		t.Fatalf("typing %v %q", m.typingHz, m.hzText)
	}
	for range 7 {
		m = press(t, m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	for _, r := range "300" {
		m = press(t, m, char(r))
	}
	m = press(t, m, enter)
	if m.refHz != 300 || !strings.Contains(ansi.Strip(m.View().Content), "300.000 Hz") {
		t.Fatalf("ref %v", m.refHz)
	}
	m = press(t, m, char('y'))
	kbm, err := scala.ParseKBMFile(filepath.Join(m.cfg.ExportDir, "31-edo.kbm"))
	if err != nil || kbm.MiddleNote != 62 || kbm.RefFreq != 300 || len(kbm.Keys) != 31 {
		t.Fatalf("kbm %+v %v (status %s)", kbm, err, m.status)
	}
	if v, _, _ := st.ScaleSettings(m.loaded); v.Root != 62 || v.RefHz != 300 {
		t.Errorf("root and frequency should be saved with the scale: %+v", v)
	}
	m = press(t, m, char('e'), char('a'), char('y'))
	if got := len(export.Existing(m.cfg.ExportDir)); got != 20 {
		t.Errorf("exporting all 10 scales should leave 20 files, found %d (%s)", got, m.status)
	}
}

func TestAbletonProfilesSendBend48(t *testing.T) {
	for _, name := range []string{"Ableton Live built-ins", "Live tuning + MPE plugin"} {
		m := ready(t)
		m = press(t, m, down, enter) // 31-edo
		m.focus = paneSynth
		for m.profile().Name != name {
			m = press(t, m, down)
		}
		if j := m.job(); m.synthBend != 48 || j.Config == nil || j.Config.Bend != 48 || j.Config.OneChannel {
			t.Errorf("%s: S %d, config %+v", name, m.synthBend, j.Config)
		}
		if !strings.Contains(ansi.Strip(m.View().Content), "1 pad = 1 degree") {
			t.Errorf("%s: SYNTH pane should say bends count in scale steps", name)
		}
	}
}
