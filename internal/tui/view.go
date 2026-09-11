package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/biomassa/linnkit/internal/layout"
	"github.com/biomassa/linnkit/internal/lights"
	"github.com/biomassa/linnkit/internal/report"
	"github.com/biomassa/linnkit/internal/synth"
	"github.com/biomassa/linnkit/internal/theory"
)

var (
	focusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#5fafff"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#6c6c6c"))
	boldStyle   = lipgloss.NewStyle().Bold(true)
	selStyle    = lipgloss.NewStyle().Reverse(true)
	warnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffaf5f"))
	goodStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#87d787"))
	accentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#5fd7d7"))
	valueStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffd75f")).Bold(true)
	labelStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#af87ff"))
	keyStyle    = focusStyle
)

const (
	scalesWidth = 32
	rightWidth  = 52
	gridHeight  = 20 // 16 pad lines + legend + blank + 2 borders
	gridWidth   = 4 + layout.Cols*4 + 2
)

// View renders the dashboard in the alternate screen.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "linnkit"
	return v
}

func (m Model) render() string {
	if m.width < MinWidth || m.height < MinHeight {
		msg := fmt.Sprintf("linnkit needs at least %dx%d; this window is %dx%d.\nEnlarge it or make the font smaller.  (q quits)",
			MinWidth, MinHeight, m.width, m.height)
		return lipgloss.Place(max(m.width, 1), max(m.height, 1), lipgloss.Center, lipgloss.Center, msg)
	}
	if m.overlay != noOverlay {
		return m.renderOverlay()
	}
	w, h := m.width, m.height
	topH := h - 2 - gridHeight
	const lightsH, synthH = 7, 12
	layoutH := topH - lightsH - synthH
	scaleW := w - scalesWidth - rightWidth
	top := joinH(
		box("SCALES", scalesWidth, topH, m.scalesLines(scalesWidth-2, topH-2), m.focus == paneScales),
		box("SCALE", scaleW, topH, m.scaleLines(topH-2), m.focus == paneScale),
		joinV(
			box("LAYOUT", rightWidth, layoutH, m.layoutLines(layoutH-2), m.focus == paneLayout),
			box("LIGHTS", rightWidth, lightsH, m.lightsLines(), m.focus == paneLights),
			box("SYNTH", rightWidth, synthH, m.synthLines(rightWidth-3, synthH-2), m.focus == paneSynth),
		),
	)
	gridTitle := "GRID"
	if m.analysis != nil {
		l := m.shown()
		lo, hi := l.Span()
		gridTitle = fmt.Sprintf("GRID  rows +%d, bottom-left MIDI %d, notes %d..%d", l.Offset, l.RowStart[0], lo, hi)
		if m.factory {
			gridTitle = "GRID  FACTORY 12-TET  " + gridTitle[6:]
		}
	}
	bottom := joinH(
		box(gridTitle, gridWidth, gridHeight, m.gridLines(), false),
		box("SEND", w-gridWidth, gridHeight, m.sendLines(), m.focus == paneSend),
	)
	lines := append([]string{m.header(w)}, top...)
	lines = append(lines, bottom...)
	lines = append(lines, m.footer(w))
	return strings.Join(lines, "\n")
}

func (m Model) header(w int) string {
	left := " linnkit"
	if m.relayRun != nil {
		left += "  " + goodStyle.Render("● relay → "+m.relayRun.Out)
	}
	if m.analysis != nil {
		left += "  " + boldStyle.Render(m.analysis.Scale.Name) + "  " + m.analysis.Scale.Description
	}
	right := m.status + "  "
	if m.busy {
		right = warnStyle.Render(right)
	}
	return fit(left, w-ansi.StringWidth(right)) + right
}

func (m Model) footer(w int) string {
	keys := "↑↓ move  Tab pane  / filter  0-2 slot  l layout  c config  f factory  s send  " +
		"p presets  e export  R relay  b backup  r restore  m t tables  ? help  q quit"
	return dimStyle.Render(fit(" "+keys, w))
}

func (m Model) scalesLines(w, h int) []string {
	vis := m.visible()
	var out []string
	if m.loading {
		out = append(out, dimStyle.Render("loading..."))
	}
	listH := h - 2
	start := max(0, min(m.sel-listH/2, len(vis)-listH))
	for i := start; i < len(vis) && len(out) < listH; i++ {
		e := vis[i]
		mark := " "
		if e.path == m.loaded {
			mark = "*"
		}
		info := fmt.Sprintf("%3d %-9s", e.size, shortClass(e.class))
		line := fit(fmt.Sprintf("%s %s", mark, e.name), w-ansi.StringWidth(info)-1) + " " + info
		if i == m.sel && m.focus == paneScales {
			line = selStyle.Render(fit(line, w))
		}
		out = append(out, line)
	}
	for len(out) < listH {
		out = append(out, "")
	}
	f := "filter: " + m.filter
	if m.filtering {
		f += "_"
	} else if m.filter == "" {
		f = dimStyle.Render("/ to filter")
	}
	return append(out, "", f)
}

func shortClass(c string) string {
	switch c {
	case "near-equal":
		return "near-eq"
	case "irregular":
		return "irreg"
	}
	return c
}

func (m Model) scaleLines(h int) []string {
	if m.analysis == nil {
		return []string{dimStyle.Render("pick a scale on the left and press Enter")}
	}
	out := report.Summary(m.analysis)
	out = append(out, "")
	table := report.DegreeTable(m.analysis)
	out = append(out, boldStyle.Render(table[0]))
	rows := table[1:]
	room := h - len(out) - 1
	scroll := min(m.tableScroll, max(0, len(rows)-room))
	for i := scroll; i < len(rows) && i-scroll < room; i++ {
		out = append(out, rows[i])
	}
	more := ""
	if len(rows) > room {
		more = fmt.Sprintf("rows %d-%d of %d, ↑↓ scroll   ", scroll+1, min(len(rows), scroll+room), len(rows))
	}
	for len(out) < h-1 {
		out = append(out, "")
	}
	return append(out, dimStyle.Render(more+"m interval matrix   t full table"))
}

func (m Model) layoutLines(h int) []string {
	if m.analysis == nil {
		return nil
	}
	out := []string{boldStyle.Render("  rows interval      periods triad move")}
	listH := h - 3
	start := max(0, min(m.candSel-listH/2, len(m.cands)-listH))
	for i := start; i < len(m.cands) && i-start < listH; i++ {
		c := m.cands[i]
		span := "  -"
		if c.ChordSpan >= 0 {
			span = fmt.Sprintf("%4.1f", c.ChordSpan)
		}
		line := fmt.Sprintf("  +%-3d %4.0fc ~%-7s %5.2f  %s  %+3d/%d", c.Offset, c.RowCents, c.RowRatio,
			c.Periods, span, c.PeriodMove[0], c.PeriodMove[1])
		tags := c.Tags
		if !c.Fits {
			tags = append([]string{"does not fit 0-127"}, tags...)
		}
		if len(tags) > 0 {
			line += "  " + dimStyle.Render(strings.Join(tags, ", "))
		}
		if i == m.candSel {
			line = ">" + line[1:]
			if m.focus == paneLayout {
				line = selStyle.Render(fit(line, rightWidth-2))
			}
		}
		out = append(out, line)
	}
	for len(out) < h-1 {
		out = append(out, "")
	}
	lowNote := "auto"
	if m.low >= 0 {
		lowNote = "set with [ ]"
	}
	return append(out, dimStyle.Render(fmt.Sprintf("root %d %s ({ }), bottom-left %d (%s)", m.root, midiName(m.root), m.lay.RowStart[0], lowNote)))
}

func (m Model) lightsLines() []string {
	var out []string
	for i, title := range schemeTitles {
		mark := "( )"
		if i == m.scheme {
			mark = "(•)"
		}
		line := fmt.Sprintf(" %s %s", mark, title)
		if i == 0 {
			line += fmt.Sprintf("   prime limit %d (< >)", limits[m.limitIx])
		}
		if i == m.scheme && m.focus == paneLights {
			line = selStyle.Render(fit(line, rightWidth-2))
		}
		out = append(out, line)
	}
	return append(out, " "+dimStyle.Render("palette editor: M5"))
}

func (m Model) synthLines(w, h int) []string {
	p := m.profile()
	verified := warnStyle.Render("? not yet verified")
	if p.Verified {
		verified = goodStyle.Render("✓ verified")
	}
	values := fmt.Sprintf("%d-%d", p.BendMin, p.BendMax)
	if p.BendValues != nil {
		values = strings.Trim(fmt.Sprint(p.BendValues), "[]")
	}
	if p.BendMin == p.BendMax && p.BendValues == nil {
		values = fmt.Sprintf("fixed %d", p.BendMin)
	}
	out := []string{
		" " + accentStyle.Bold(true).Render(p.Name) + "  " + verified + "  " + keyStyle.Render("↑↓"),
		" " + labelStyle.Render("synth bend S ") + valueStyle.Render(fmt.Sprintf("%-3d", m.synthBend)) +
			" " + keyStyle.Render("< >") + dimStyle.Render(" "+values),
	}
	if m.analysis != nil {
		pl := m.bendPlan()
		errStyle := goodStyle
		if abs(pl.Error) >= 0.5 {
			errStyle = warnStyle
		}
		out[1] += labelStyle.Render("  Linn bend B ") + valueStyle.Render(fmt.Sprint(pl.LinnBend))
		if pl.InSteps {
			out = append(out, wrapStyled("bend counts in scale steps: 1 pad = 1 degree", w, goodStyle)...)
		} else {
			out = append(out, " "+labelStyle.Render("pad ")+fmt.Sprintf("%.2f c", pl.PadCents)+
				labelStyle.Render("  step ")+fmt.Sprintf("%.2f c", pl.Target)+
				labelStyle.Render("  error ")+errStyle.Render(fmt.Sprintf("%+.2f c", pl.Error)))
		}
		if m.analysis.Structure.Class != theory.Equal && !m.factory && !pl.InSteps {
			out = append(out, wrapStyled("steps differ: worst pad/step error "+fmt.Sprintf("%.1f c", pl.WorstStep), w, warnStyle)...)
		}
		if pl.LinnBend == 96 || pl.LinnBend == 1 {
			out = append(out, wrapStyled("B is at the LinnStrument's limit; try another S", w, warnStyle)...)
		}
	}
	switch p.ObeysRPN {
	case synth.No:
		out = append(out, labelled("MPE bend msg", "ignored", w, lipgloss.NewStyle())...)
	case synth.Unknown:
		out = append(out, labelled("MPE bend msg", "unknown; sends keep MPE state off", w, lipgloss.NewStyle())...)
	}
	out = append(out, labelled("tuning", p.Tuning, w, lipgloss.NewStyle())...)
	for _, n := range p.Notes {
		out = append(out, wrapStyled("· "+n, w, dimStyle)...)
	}
	if len(out) > h {
		out = append(out[:h-1], dimStyle.Render(" …"))
	}
	return out
}

// labelled wraps "label: text", styling the label and the text separately.
func labelled(label, text string, w int, st lipgloss.Style) []string {
	lines := wrap(label+": "+text, w)
	for i, l := range lines {
		if i == 0 {
			l = strings.TrimPrefix(l, " "+label+":")
			lines[i] = " " + labelStyle.Render(label+":") + st.Render(l)
		} else {
			lines[i] = st.Render(l)
		}
	}
	return lines
}

// wrapStyled wraps plain text, then styles each line.
func wrapStyled(s string, w int, st lipgloss.Style) []string {
	lines := wrap(s, w)
	for i, l := range lines {
		lines[i] = st.Render(l)
	}
	return lines
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// wrap splits s at spaces into lines of at most w cells: a one-space margin,
// continuation lines indented by three.
func wrap(s string, w int) []string {
	var out []string
	line := ""
	for _, word := range strings.Fields(s) {
		if line != "" && ansi.StringWidth(line)+1+ansi.StringWidth(word) > w {
			out = append(out, line)
			line = "   " + word
			continue
		}
		if line == "" {
			line = " " + word
		} else {
			line += " " + word
		}
	}
	return append(out, line)
}

func (m Model) sendLines() []string {
	slots := ""
	for s := 0; s <= 2; s++ {
		if s == m.slot {
			slots += fmt.Sprintf("[%d] ", s)
		} else {
			slots += fmt.Sprintf(" %d  ", s)
		}
	}
	check := func(b bool) string {
		if b {
			return "[x]"
		}
		return "[ ]"
	}
	cfg := "Channel Per Note 2-16, main 1"
	if !m.profile().MPE {
		cfg = "One Channel, channel 1"
	}
	if m.analysis != nil {
		cfg += fmt.Sprintf(", bend %d (synth %d)", m.bendPlan().LinnBend, m.synthBend)
	}
	out := []string{
		" device: " + m.deviceInfo,
		" port:   " + m.cfg.Port,
		"",
		" light slot   " + slots + "  (0 1 2)",
		" " + check(m.withLayout) + " send row layout      (l)",
		" " + check(m.withConfig) + " configure MIDI       (c)",
		"     " + dimStyle.Render(cfg),
		" " + check(m.factory) + " factory 12-TET layout (f)",
		"",
		" s send (asks first; backs up settings before)",
		" b backup   r restore latest backup",
	}
	if m.factory {
		out[3] = dimStyle.Render(ansi.Strip(out[3]) + "  unused")
		out[4] = dimStyle.Render(ansi.Strip(out[4]) + "  unused")
		out[7] = warnStyle.Render(out[7])
	}
	if p := m.lastSent; p != nil {
		what := strings.TrimSuffix(filepath.Base(p.Scale), filepath.Ext(p.Scale))
		if p.Factory {
			what = "factory 12-TET"
		}
		out = append(out, "", " "+labelStyle.Render("last sent ")+what+fmt.Sprintf(", slot %d, %s", p.Slot, p.Settings.Synth))
		detail := ""
		if bl := p.BottomLeft; bl != nil {
			rows := p.Settings.Offset
			if p.Factory {
				rows = 5
			}
			detail = fmt.Sprintf("bottom-left %d %s, rows +%d, ", *bl, midiName(*bl), rows)
		}
		out = append(out, "           "+detail+dimStyle.Render(p.Sent.Local().Format("Jan 2 15:04")))
	}
	if m.lastBackup != "" {
		out = append(out, " "+dimStyle.Render("last backup: "+filepath.Base(m.lastBackup)))
	}
	return out
}

var controlLabels = [layout.Rows]string{"PS", "PR", "VO", "OT", "S1", "S2", "SP", "GS"} // top to bottom

func (m Model) gridLines() []string {
	if m.analysis == nil {
		return nil
	}
	ctl := [3]uint8{52, 52, 52}
	var out []string
	for row := layout.Rows - 1; row >= 0; row-- {
		var top, bottom strings.Builder
		top.WriteString(rgbBG(ctl) + rgbFG([3]uint8{150, 150, 150}) + " " + controlLabels[layout.Rows-1-row] + "\x1b[0m ")
		bottom.WriteString(rgbFG(ctl) + "▀▀▀\x1b[0m ")
		for _, p := range m.surface[row] {
			c := p.Color.RGB()
			label := p.Label
			if p.Color == lights.Off {
				label = ""
			}
			top.WriteString(rgbBG(c) + rgbFG(textOn(p.Color)) + center(label, 3) + "\x1b[0m ")
			bottom.WriteString(rgbFG(c) + "▀▀▀\x1b[0m ")
		}
		out = append(out, top.String(), bottom.String())
	}
	return append(out, "", " "+m.legend)
}

func textOn(c lights.Color) [3]uint8 {
	if c.Light() {
		return [3]uint8{20, 20, 20}
	}
	return [3]uint8{255, 255, 255}
}

func rgbBG(c [3]uint8) string { return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", c[0], c[1], c[2]) }
func rgbFG(c [3]uint8) string { return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", c[0], c[1], c[2]) }

func (m Model) renderOverlay() string {
	w, h := m.width, m.height
	var title string
	var lines []string
	switch m.overlay {
	case overlayMatrix:
		title = "INTERVAL MATRIX  row degree up to column degree; ratios where exact, else cents   ↑↓←→ scroll, Esc close"
		lines = report.Matrix(m.analysis)
	case overlayTable:
		title = "DEGREE TABLE   ↑↓ scroll, Esc close"
		lines = append(report.Summary(m.analysis), "")
		lines = append(lines, report.DegreeTable(m.analysis)...)
	case overlayPresets:
		title = "PRESETS   scale + layout + lights + synth + slot"
		lines = m.presetLines()
	case overlayExport:
		title = "EXPORT TO MADRONA LABS   .scl copied unchanged + a matching .kbm"
		lines = m.exportLines()
	case overlayRelay:
		title = "TUNING RELAY   for 12-TET gear: Kurzweil K2600, Mutant Brain"
		lines = m.relayLines()
	case overlayHelp:
		title = "HELP   Esc close"
		lines = helpLines
	case overlayConfirmSend:
		title = "SEND TO THE LINNSTRUMENT?"
		lines = m.confirmSendLines()
	case overlayConfirmRestore:
		title = "RESTORE DEVICE SETTINGS?"
		lines = []string{"", " Restore every setting from", "   " + m.restoreFrom, "",
			" Parameters that differ are written back; actions such as preset load are skipped.", "",
			" y restore    n cancel"}
	}
	scroll := min(m.scrollY, max(0, len(lines)-(h-2)))
	var view []string
	for i := scroll; i < len(lines) && len(view) < h-2; i++ {
		view = append(view, cutLeft(lines[i], m.scrollX))
	}
	return strings.Join(box(title, w, h, view, true), "\n")
}

func (m Model) confirmSendLines() []string {
	if m.factory {
		return m.confirmFactoryLines()
	}
	a := m.analysis
	out := []string{"",
		fmt.Sprintf(" scale:       %s (%d notes)", a.Scale.Name, a.Structure.Size),
		fmt.Sprintf(" lights:      %s -> custom light slot %d (saved on the device)", schemeTitles[m.scheme], m.slot),
	}
	if m.withLayout || m.withConfig {
		out = append(out, fmt.Sprintf(" row layout:  Guitar rows %v", m.lay.RowStart))
	} else {
		out = append(out, " row layout:  unchanged")
	}
	if m.withConfig {
		j := m.job()
		mode := "Channel Per Note, main 1, per-note 2-16"
		if j.Config.OneChannel {
			mode = "One Channel, channel 1"
		}
		out = append(out, fmt.Sprintf(" MIDI setup:  %s, Bend Range %d, Y CC74, Z channel pressure", mode, j.Config.Bend),
			fmt.Sprintf(" synth:       %s: set its per-note bend range to %d", m.profile().Name, m.synthBend))
	} else {
		out = append(out, " MIDI setup:  unchanged")
	}
	if m.slot != 2 {
		out = append(out, "", warnStyle.Render(fmt.Sprintf(" note: slot %d is not the scratch slot (2); its current pattern will be replaced", m.slot)))
	}
	return append(out, "",
		" Every setting is backed up to ~/.config/linnkit/backups first; the send is verified by readback.",
		" The LinnStrument must show its normal play screen.", "",
		" y send    n cancel")
}

func (m Model) confirmFactoryLines() []string {
	out := []string{"",
		" layout:      factory 12-TET: row offset +5 (fourths), bottom-left F#1 (MIDI 30), octave and transpose 0",
		" lights:      note-light pattern 0: C in cyan, D E F G A B in green; stock Guitar tuning rows",
		" light slots: unchanged (the custom slots keep their patterns)",
	}
	if m.withConfig {
		j := m.job()
		mode := "Channel Per Note, main 1, per-note 2-16"
		if j.Config.OneChannel {
			mode = "One Channel, channel 1"
		}
		out = append(out, fmt.Sprintf(" MIDI setup:  %s, Bend Range %d, Y CC74, Z channel pressure", mode, j.Config.Bend),
			fmt.Sprintf(" synth:       %s: set its per-note bend range to %d, and its tuning to 12-TET", m.profile().Name, m.synthBend))
	} else {
		out = append(out, " MIDI setup:  unchanged")
	}
	return append(out, "",
		" Every setting is backed up to ~/.config/linnkit/backups first; the send is verified by readback.",
		warnStyle.Render(" To keep this after power-off, press and release a control button (e.g. Preset) afterwards:"),
		warnStyle.Render(" only that writes these settings to flash."), "",
		" y send    n cancel")
}

var helpLines = []string{
	"",
	" Panes: SCALES, SCALE, LAYOUT, LIGHTS, SEND. Tab or Left/Right moves between them; the GRID shows the result.",
	"",
	" SCALES   ↑↓ move, Enter load, / filter by name or description (Enter or Esc ends the filter)",
	" SCALE    ↑↓ scroll the degree table; m interval matrix; t full table",
	" LAYOUT   ↑↓ pick a row offset (best first); [ ] move the bottom-left note down or up",
	"          { } move the root (MIDI note of degree 0) down or up",
	" LIGHTS   ↑↓ pick a scheme; < > change the prime limit for just-interval lights",
	" SYNTH    ↑↓ pick a synth; < > its per-note bend range S. The LinnStrument Bend Range B follows:",
	"          one pad of slide = 100 x S / B cents, aimed at the scale's step (average step if unequal)",
	" SEND     0 1 2 light slot; l row layout on/off; c MIDI setup on/off; s send (asks first)",
	"          f factory 12-TET layout instead of the scale (rows in fourths, stock note lights)",
	"          b backup settings; r restore the latest backup",
	" PRESETS  p opens them: a saves the scale with its settings under a name, Enter loads one, d deletes",
	"          Each scale also remembers its own settings between runs.",
	" RELAY    R opens the tuning relay: it retunes the LinnStrument for 12-TET gear (K2600, Mutant Brain)",
	"          with pitch bend, one channel per note. Space starts and stops it; Esc closes the window only.",
	" EXPORT   e copies the scale plus a matching .kbm to ~/Music/Madrona Labs/Scales/linnkit for Aalto;",
	"          { } root, h reference Hz, a all listed scales. Files Aalto would read differently are skipped.",
	"",
	" Sends always back up every setting first and verify by readback afterwards.",
	" The LinnStrument must be awake and on its normal play screen.",
	"",
	" q quits (Ctrl+C anywhere).",
}

// box draws a titled frame of exactly w x h cells around lines.
func box(title string, w, h int, lines []string, focused bool) []string {
	st := dimStyle
	if focused {
		st = focusStyle
	}
	t := " " + title + " "
	if ansi.StringWidth(t) > w-4 {
		t = ansi.Truncate(t, w-4, "…")
	}
	out := []string{st.Render("┌─") + boldStyle.Render(t) + st.Render(strings.Repeat("─", max(0, w-3-ansi.StringWidth(t)))+"┐")}
	for i := 0; i < h-2; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		out = append(out, st.Render("│")+fit(line, w-2)+st.Render("│"))
	}
	return append(out, st.Render("└"+strings.Repeat("─", w-2)+"┘"))
}

// fit truncates or pads s to exactly w cells.
func fit(s string, w int) string {
	if ansi.StringWidth(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	return s + strings.Repeat(" ", max(0, w-ansi.StringWidth(s)))
}

// cutLeft drops the first n cells of s (for horizontal scrolling).
func cutLeft(s string, n int) string {
	if n <= 0 {
		return s
	}
	return ansi.TruncateLeft(s, n, "")
}

func center(s string, w int) string {
	pad := w - ansi.StringWidth(s)
	if pad <= 0 {
		return s
	}
	return strings.Repeat(" ", pad/2) + s + strings.Repeat(" ", pad-pad/2)
}

func joinH(cols ...[]string) []string {
	var out []string
	for i := range cols[0] {
		var b strings.Builder
		for _, c := range cols {
			if i < len(c) {
				b.WriteString(c[i])
			}
		}
		out = append(out, b.String())
	}
	return out
}

func joinV(parts ...[]string) []string {
	var out []string
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
