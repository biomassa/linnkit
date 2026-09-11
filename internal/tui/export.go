package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/biomassa/linnkit/internal/export"
	"github.com/biomassa/linnkit/internal/layout"
	"github.com/biomassa/linnkit/internal/scala"
)

// candidateWith returns the index of the candidate with this row offset, or 0.
func (m Model) candidateWith(offset int) int {
	for i, c := range m.cands {
		if c.Offset == offset {
			return i
		}
	}
	return 0
}

// moveRoot moves degree 0 down ({) or up (}) a MIDI note, keeping the row offset.
func (m Model) moveRoot(k string) Model {
	r := m.root - 1
	if k == "}" {
		r = m.root + 1
	}
	if r < 0 || r > 127 {
		return m
	}
	offset := m.lay.Offset
	m.root = r
	m.cands = layout.Candidates(m.analysis, layout.Options{Root: m.root})
	m.candSel = m.candidateWith(offset)
	return m.paint()
}

// refFreq is the frequency of the root: set by the user, or its 12-TET frequency.
func refFreq(root int, hz float64) float64 {
	if hz > 0 {
		return hz
	}
	return scala.StandardFreq(root)
}

var sharpNames = []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}

// midiName names a MIDI note with C4 = 60.
func midiName(n int) string { return fmt.Sprintf("%s%d", sharpNames[n%12], n/12-1) }

type exportRow struct {
	name     string
	root     int
	hz       float64
	problems []string
	err      error
}

// exportPlan returns what an export writes (the scales Aalto reads as linnkit
// does) and one row per scale for the overlay.
func (m Model) exportPlan() ([]export.File, []exportRow) {
	var items []export.Item
	var rows []exportRow
	add := func(path string, root int, hz float64) {
		row := exportRow{name: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), root: root, hz: refFreq(root, hz)}
		data, err := os.ReadFile(path)
		var s *scala.Scale
		if err == nil {
			s, err = scala.ParseSCL(data)
		}
		if err != nil {
			row.err = err
		} else if row.problems = export.MadronaProblems(data, s); len(row.problems) == 0 {
			items = append(items, export.Item{Src: path, Scale: s, Root: root, RefFreq: row.hz})
		}
		rows = append(rows, row)
	}
	if !m.exportAll {
		add(m.loaded, m.root, m.refHz)
	} else {
		for _, e := range m.visible() {
			if e.err != nil {
				continue
			}
			if e.path == m.loaded {
				add(e.path, m.root, m.refHz)
				continue
			}
			root, hz := m.cfg.Root, 0.0
			if m.cfg.Store != nil {
				if v, ok, _ := m.cfg.Store.ScaleSettings(e.path); ok && v.Root > 0 {
					root, hz = v.Root, v.RefHz
				}
			}
			add(e.path, root, hz)
		}
	}
	files, err := export.Madrona(items, m.exportDir())
	if err != nil {
		rows = append(rows, exportRow{name: "export", err: err})
		files = nil
	}
	return files, rows
}

func (m Model) exportDir() string {
	if m.cfg.ExportDir != "" {
		return m.cfg.ExportDir
	}
	return export.MadronaDir()
}

func (m Model) exportKey(k string) (tea.Model, tea.Cmd) {
	if m.typingHz {
		switch k {
		case "enter":
			v, err := strconv.ParseFloat(strings.TrimSpace(m.hzText), 64)
			if err != nil || v < 1 || v > 20000 {
				m.status = "reference frequency: a number of Hz between 1 and 20000"
				return m, nil
			}
			m.refHz, m.typingHz = v, false
		case "esc":
			m.typingHz = false
		case "backspace":
			if m.hzText != "" {
				m.hzText = m.hzText[:len(m.hzText)-1]
			}
		default:
			if len(k) == 1 && strings.ContainsAny(k, "0123456789.") {
				m.hzText += k
			}
		}
		return m, nil
	}
	switch k {
	case "esc", "q", "n", "e":
		m.overlay = noOverlay
	case "a":
		m.exportAll, m.scrollY = !m.exportAll, 0
	case "up":
		m.scrollY = max(0, m.scrollY-1)
	case "down":
		m.scrollY++
	case "{", "}":
		m = m.moveRoot(k)
	case "h":
		m.typingHz, m.hzText = true, strconv.FormatFloat(refFreq(m.root, m.refHz), 'f', 3, 64)
	case "z":
		m.refHz = 0
	case "y":
		files, rows := m.exportPlan()
		n, err := export.Write(files)
		skipped := 0
		for _, r := range rows {
			if r.err != nil || len(r.problems) > 0 {
				skipped++
			}
		}
		switch {
		case err != nil:
			m.status = "export: " + err.Error()
		case len(files) == 0:
			m.status = "export: nothing to write"
		default:
			m.status = fmt.Sprintf("export: %d files written, %d already up to date, %d scales skipped; in Aalto: KEY scale menu > %s",
				n, len(files)-n, skipped, filepath.Base(m.exportDir()))
		}
		m.overlay = noOverlay
	}
	return m, nil
}

func (m Model) exportLines() []string {
	files, rows := m.exportPlan()
	out := []string{"",
		" " + labelStyle.Render("folder  ") + m.exportDir() + dimStyle.Render("   (Aalto, Kaivo and other Madrona Labs synths; subfolders are menus)"),
		" " + labelStyle.Render("scales  ") + map[bool]string{false: "the loaded scale", true: "every scale in the list (filter with / first)"}[m.exportAll] +
			dimStyle.Render("   a toggles"),
		"",
	}
	hz := refFreq(m.root, m.refHz)
	how := "its 12-TET frequency (A4 = 440 Hz)"
	if m.refHz > 0 {
		how = "set by hand"
	}
	out = append(out,
		" "+labelStyle.Render("root    ")+valueStyle.Render(fmt.Sprintf("MIDI %d (%s)", m.root, midiName(m.root)))+
			dimStyle.Render("   { } move it; this also moves degree 0 on the LinnStrument"),
		" "+labelStyle.Render("ref     ")+valueStyle.Render(fmt.Sprintf("%.3f Hz", hz))+" "+how+dimStyle.Render("   h type a frequency, z back to 12-TET"))
	if m.typingHz {
		out = append(out, "         Hz: "+m.hzText+"_"+dimStyle.Render("   Enter sets, Esc cancels"))
	}
	if m.exportAll {
		out = append(out, dimStyle.Render("         other scales use their own saved root and frequency (default MIDI "+strconv.Itoa(m.cfg.Root)+", 12-TET)"))
	}
	out = append(out, "", boldStyle.Render(" scale                            root        Hz          Aalto reads it"))
	for _, r := range rows {
		status := goodStyle.Render("as linnkit does")
		switch {
		case r.err != nil:
			status = warnStyle.Render("error: " + r.err.Error())
		case len(r.problems) > 0:
			status = warnStyle.Render("skipped: " + r.problems[0])
		}
		out = append(out, fmt.Sprintf(" %-32s %-11s %-11s ", r.name, fmt.Sprintf("%d %s", r.root, midiName(r.root)),
			fmt.Sprintf("%.3f", r.hz))+status)
	}
	out = append(out, "")
	write, same := 0, 0
	for _, f := range files {
		switch {
		case f.Same:
			same++
		case f.Existed:
			write++
			out = append(out, warnStyle.Render(" replaces "+filepath.Base(f.Path)))
		default:
			write++
		}
	}
	out = append(out,
		fmt.Sprintf(" %d files to write, %d already up to date. Each .scl is copied unchanged; each .kbm puts degree 0 on the root", write, same),
		" and lists every degree (Aalto ignores the map size).", "",
		" y write    Esc cancel")
	return out
}
