package tui

import (
	"fmt"
	"strconv"

	"github.com/biomassa/linnkit/internal/device"
	"github.com/biomassa/linnkit/internal/lights"
)

// lightsRecord is what a send paints into its light slot, kept for restores.
func (m Model) lightsRecord() device.LightsRecord {
	return device.NewLightsRecord(m.slot, m.analysis.Scale.Name, schemes[m.scheme], limits[m.limitIx], m.root, m.lay.Offset, m.low, m.surface)
}

// lightSettings are the loaded scale's scheme options.
func (m Model) lightSettings() lights.Settings {
	return lights.Settings{Limit: limits[m.limitIx], Generator: m.gen, MOSSize: m.mosSize, Harmonics: m.harm, Subharmonics: m.subharm}
}

// lightSetting changes the selected scheme's settings: < > the main one, { } the second.
func (m Model) lightSetting(k string) Model {
	n := 0
	if m.analysis != nil {
		n = m.analysis.Structure.Size
	}
	d := 1
	if k == "<" || k == "{" {
		d = -1
	}
	main := k == "<" || k == ">"
	switch schemes[m.scheme] {
	case "ji", "kite", "factors":
		if main {
			m.limitIx = max(0, min(len(limits)-1, m.limitIx+d))
		}
	case "chain", "wijmenga", "nested":
		if main {
			m.gen = max(0, min(n-1, m.gen+d))
		}
	case "moskeys":
		if main {
			m.gen = max(0, min(n-1, m.gen+d))
		} else {
			m.mosSize = max(0, min(n, m.mosSize+d))
		}
	case "harmonics":
		if main {
			m.harm = map[int]int{16: 32, 32: 16}[m.harm]
		} else {
			m.subharm = !m.subharm
		}
	}
	return m.paint()
}

// lightSettingText shows the selected scheme's settings and their keys.
func (m Model) lightSettingText() string {
	if m.analysis == nil {
		return ""
	}
	s := m.lightSettings()
	gen := fmt.Sprintf("generator %d", lights.Generator(m.analysis, s))
	if m.gen == 0 {
		gen += " (auto)"
	}
	switch schemes[m.scheme] {
	case "ji", "kite", "factors":
		return fmt.Sprintf("prime limit %d  < >", limits[m.limitIx])
	case "chain", "wijmenga", "nested":
		return gen + "  < >"
	case "moskeys":
		size := strconv.Itoa(m.mosSize)
		if _, ms, ok := lights.MOSKeys(m.analysis, s); m.mosSize == 0 && ok {
			size = fmt.Sprintf("%d (auto)", ms.Size)
		} else if m.mosSize == 0 {
			size = "auto: none"
		}
		return gen + " < >  size " + size + " { }"
	case "harmonics":
		sub := "off"
		if m.subharm {
			sub = "on"
		}
		lo, hi := 1, 16
		if m.harm == 32 {
			lo, hi = 16, 32
		}
		return fmt.Sprintf("harmonics %d-%d < >  subharmonics %s { }", lo, hi, sub)
	}
	return "no settings"
}
