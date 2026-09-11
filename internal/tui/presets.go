package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/biomassa/linnkit/internal/store"
)

// openPresets shows the named presets.
func (m Model) openPresets() Model {
	if m.cfg.Store == nil {
		m.status = "presets: no app data folder"
		return m
	}
	ps, err := m.cfg.Store.Presets()
	if err != nil {
		m.status = "presets: " + err.Error()
		return m
	}
	m.presets, m.overlay = ps, overlayPresets
	m.presetSel = min(m.presetSel, max(0, len(ps)-1))
	m.naming, m.confirmDel, m.confirmRep = false, false, false
	return m
}

// defaultPresetName is "<scale> <synth>".
func (m Model) defaultPresetName() string {
	if m.analysis == nil {
		return ""
	}
	return strings.TrimSuffix(filepath.Base(m.loaded), filepath.Ext(m.loaded)) + " " + m.profile().Name
}

func (m Model) presetKey(k string) Model {
	switch {
	case m.confirmDel:
		if k == "y" && len(m.presets) > 0 {
			name := m.presets[m.presetSel].Name
			if err := m.cfg.Store.DeletePreset(name); err != nil {
				m.status = "delete: " + err.Error()
			} else {
				m.status = "deleted preset " + name
			}
			m = m.openPresets()
		}
		m.confirmDel = false
		return m
	case m.confirmRep:
		if k == "y" {
			return m.savePreset()
		}
		m.confirmRep = false
		return m
	case m.naming:
		switch k {
		case "enter":
			if strings.TrimSpace(m.name) == "" {
				return m
			}
			if m.cfg.Store.HasPreset(m.name) {
				m.confirmRep = true
				return m
			}
			return m.savePreset()
		case "esc":
			m.naming = false
		case "backspace":
			if r := []rune(m.name); len(r) > 0 {
				m.name = string(r[:len(r)-1])
			}
		case "space":
			m.name += " "
		default:
			if len([]rune(k)) == 1 {
				m.name += k
			}
		}
		return m
	}
	switch k {
	case "esc", "q", "p":
		m.overlay = noOverlay
	case "up":
		m.presetSel = max(0, m.presetSel-1)
	case "down":
		m.presetSel = min(max(0, len(m.presets)-1), m.presetSel+1)
	case "a":
		if m.analysis != nil {
			m.naming, m.name = true, m.defaultPresetName()
		}
	case "d":
		if len(m.presets) > 0 {
			m.confirmDel = true
		}
	case "enter":
		if len(m.presets) > 0 {
			return m.loadPreset(m.presets[m.presetSel])
		}
	}
	return m
}

func (m Model) savePreset() Model {
	p := store.Preset{Name: strings.TrimSpace(m.name), Scale: m.loaded, Settings: m.settings(), Slot: m.slot,
		WithLayout: m.withLayout, WithConfig: m.withConfig}
	if err := m.cfg.Store.SavePreset(p); err != nil {
		m.status = "save preset: " + err.Error()
	} else {
		m.status = "saved preset " + p.Name
	}
	m = m.openPresets()
	for i, q := range m.presets {
		if q.Name == p.Name {
			m.presetSel = i
		}
	}
	return m
}

// loadPreset opens the preset's scale with its settings. It sends nothing.
func (m Model) loadPreset(p store.Preset) Model {
	m.filter, m.factory = "", false
	i := -1
	for j, e := range m.entries {
		if sameFile(e.path, p.Scale) {
			i = j
		}
	}
	if i < 0 {
		m.status = fmt.Sprintf("preset %s: scale %s is not in the scale folders", p.Name, p.Scale)
		return m
	}
	m = m.load(i)
	m = m.apply(p.Settings)
	m.slot, m.withLayout, m.withConfig = p.Slot, p.WithLayout, p.WithConfig
	m.overlay = noOverlay
	m.status = "loaded preset " + p.Name + "; s sends it"
	return m
}

func sameFile(a, b string) bool {
	norm := func(p string) string {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		if real, err := filepath.EvalSymlinks(p); err == nil {
			p = real
		}
		return p
	}
	return norm(a) == norm(b)
}

func (m Model) presetLines() []string {
	out := []string{""}
	if len(m.presets) == 0 {
		out = append(out, dimStyle.Render(" no presets yet: a saves the current scale and settings"))
	}
	for i, p := range m.presets {
		v := p.Settings
		line := fmt.Sprintf(" %-32s %-22s rows +%-3d %-6s %-22s slot %d", p.Name,
			strings.TrimSuffix(filepath.Base(p.Scale), filepath.Ext(p.Scale)), v.Offset, v.Scheme, v.Synth, p.Slot)
		if i == m.presetSel {
			line = selStyle.Render(line)
		}
		out = append(out, line)
	}
	out = append(out, "")
	switch {
	case m.confirmDel:
		out = append(out, warnStyle.Render(fmt.Sprintf(" delete preset %q? y / n", m.presets[m.presetSel].Name)))
	case m.confirmRep:
		out = append(out, warnStyle.Render(fmt.Sprintf(" replace preset %q? y / n", strings.TrimSpace(m.name))))
	case m.naming:
		out = append(out, " name: "+m.name+"_", dimStyle.Render(" Enter saves   Esc cancels"))
	default:
		out = append(out, dimStyle.Render(" ↑↓ pick   Enter load (sends nothing)   a save current as new   d delete   Esc close"))
	}
	if m.cfg.Store != nil {
		out = append(out, "", dimStyle.Render(" stored in "+filepath.Join(m.cfg.Store.Dir, "presets")))
	}
	return out
}
