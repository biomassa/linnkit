package device

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/biomassa/linnkit/internal/layout"
	"github.com/biomassa/linnkit/internal/lights"
)

// LightsRecord is a pattern painted into a custom light slot. The LinnStrument
// can't report its patterns, so every send keeps one, backups carry them, and
// a restore paints them again. The format is shared with linn.lights (Max):
// a backup's "lights" entry is the slot 2 record, so each side restores the
// other's backups; linnkit adds "lights_slots" for slots 0 and 1.
type LightsRecord struct {
	Slot    int     `json:"slot"`
	Scale   string  `json:"scale"`
	Scheme  string  `json:"scheme"`
	Limit   int     `json:"limit"`
	Root    int     `json:"root"`
	Offset  int     `json:"offset"`
	Low     int     `json:"low"`     // bottom-left MIDI note setting; -1 = automatic
	Taken   string  `json:"taken"`   // ISO 8601, UTC
	Pattern [][]int `json:"pattern"` // [8 rows][25 columns] of CC22 colours, row 0 nearest the player
}

// NewLightsRecord records a painted surface.
func NewLightsRecord(slot int, scale, scheme string, limit, root, offset, low int, s lights.Surface) LightsRecord {
	p := make([][]int, layout.Rows)
	for row := range p {
		p[row] = make([]int, layout.Cols)
		for col := range layout.Cols {
			p[row][col] = int(s[row][col].Color)
		}
	}
	return LightsRecord{Slot: slot, Scale: scale, Scheme: scheme, Limit: limit, Root: root, Offset: offset, Low: low,
		Taken: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), Pattern: p}
}

// Valid reports whether the record is a full 8 x 25 pattern for slot 0-2.
func (r LightsRecord) Valid() bool {
	if r.Slot < 0 || r.Slot > 2 || len(r.Pattern) != layout.Rows {
		return false
	}
	for _, row := range r.Pattern {
		if len(row) != layout.Cols {
			return false
		}
		for _, c := range row {
			if c < 0 || c > 11 {
				return false
			}
		}
	}
	return true
}

func (r LightsRecord) surface() lights.Surface {
	var s lights.Surface
	for row := range layout.Rows {
		for col := range layout.Cols {
			s[row][col].Color = lights.Color(r.Pattern[row][col])
		}
	}
	return s
}

func lightsFile(dir string, slot int) string {
	return filepath.Join(dir, fmt.Sprintf("lights-slot%d.json", slot))
}

// SaveLightsRecord writes the record as dir/lights-slotN.json.
func SaveLightsRecord(dir string, r LightsRecord) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(lightsFile(dir, r.Slot), append(data, '\n'), 0o644)
}

// LoadLightsRecords returns the valid records in dir, slot order.
func LoadLightsRecords(dir string) []LightsRecord {
	var out []LightsRecord
	for slot := range 3 {
		data, err := os.ReadFile(lightsFile(dir, slot))
		if err != nil {
			continue
		}
		var r LightsRecord
		if decodeFirst(data, &r) == nil && r.Valid() && r.Slot == slot {
			out = append(out, r)
		}
	}
	return out
}

// AttachLights puts the records in dir into the snapshot: slot 2 as "lights"
// (the Max format), slots 0 and 1 as "lights_slots".
func (s *Snapshot) AttachLights(dir string) {
	for _, r := range LoadLightsRecords(dir) {
		if r.Slot == 2 {
			s.Lights = &r
		} else {
			s.LightsSlots = append(s.LightsSlots, r)
		}
	}
}

// lightRecords returns the snapshot's valid records in slot order. The
// "lights" entry is always slot 2, as linn.lights writes it.
func (s Snapshot) lightRecords() []LightsRecord {
	var out []LightsRecord
	for _, r := range s.LightsSlots {
		if r.Valid() && r.Slot < 2 {
			out = append(out, r)
		}
	}
	if s.Lights != nil {
		r := *s.Lights
		r.Slot = 2
		if r.Valid() {
			out = append(out, r)
		}
	}
	slices.SortFunc(out, func(a, b LightsRecord) int { return a.Slot - b.Slot })
	return out
}

// decodeFirst decodes the first JSON value and ignores whatever follows: Max's
// File object doesn't cut a file it rewrites, so a shorter JSON can be
// followed by the old tail.
func decodeFirst(data []byte, v any) error {
	return json.NewDecoder(bytes.NewReader(data)).Decode(v)
}
