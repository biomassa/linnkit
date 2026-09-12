// Package store keeps app data in ~/.config/linnkit: config (extra scale
// folders, default synth), per-scale settings and named presets, as JSON.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Store is a folder of JSON files:
//
//	config.json      Config
//	scales.json      ScaleSettings per scale file, keyed by absolute path
//	presets/*.json   one Preset per file
type Store struct{ Dir string }

// DefaultDir is $LINNKIT_CONFIG_DIR, or ~/.config/linnkit.
func DefaultDir() string {
	if d := os.Getenv("LINNKIT_CONFIG_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "linnkit")
}

// Open returns the store in dir, creating the folder if needed.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "presets"), 0o755); err != nil {
		return nil, err
	}
	return &Store{Dir: dir}, nil
}

// Config is app-wide.
type Config struct {
	ScaleDirs []string       `json:"scale_dirs,omitempty"` // extra folders, added to the defaults
	Synth     string         `json:"synth,omitempty"`      // synth profile for scales with no settings yet
	SynthBend int            `json:"synth_bend,omitempty"`
	LastSent  *Preset        `json:"last_sent,omitempty"` // what the last send put on the LinnStrument
	Relay     *RelaySettings `json:"relay,omitempty"`
}

// RelaySettings are the tuning relay's last settings.
type RelaySettings struct {
	Target string `json:"target"`
	Port   string `json:"port"`
	First  int    `json:"first"`
	Last   int    `json:"last"`
	Bend   int    `json:"bend"`
	NoRPN  bool   `json:"no_rpn,omitempty"`
}

// ScaleSettings is what the dashboard remembers for one scale.
type ScaleSettings struct {
	Offset    int      `json:"offset"`                    // row offset in scale steps
	Low       int      `json:"low"`                       // bottom-left MIDI note; -1 = the candidate's own
	Root      int      `json:"root"`                      // MIDI note of degree 0
	RefHz     float64  `json:"ref_hz,omitempty"`          // frequency of the root; 0 = its 12-TET frequency
	Scheme    string   `json:"scheme"`                    // light scheme: ji, names, mos, root, palette
	Limit     int      `json:"limit,omitempty"`           // prime limit for the ji scheme
	Palette   []string `json:"palette,omitempty"`         // color name per degree, for the palette scheme
	Generator int      `json:"generator,omitempty"`       // light schemes: generator in degrees; 0 = nearest 3/2
	MOSSize   int      `json:"mos_size,omitempty"`        // moskeys: notes; 0 = automatic
	Harmonics int      `json:"harmonics,omitempty"`       // harmonics scheme: 16 (1-16) or 32 (16-32); 0 = 16
	NoSubharm bool     `json:"no_subharmonics,omitempty"` // harmonics scheme: leave out the subharmonics
	Synth     string   `json:"synth"`
	SynthBend int      `json:"synth_bend"`
}

// Preset is a named scale plus settings, pushed to the device on demand.
type Preset struct {
	Name       string        `json:"name"`
	Scale      string        `json:"scale"` // path of the .scl file
	Settings   ScaleSettings `json:"settings"`
	Slot       int           `json:"slot"` // custom light slot 0-2
	WithLayout bool          `json:"with_layout"`
	WithConfig bool          `json:"with_config"`
	Factory    bool          `json:"factory,omitempty"`     // the factory 12-TET layout was sent instead of the scale
	BottomLeft *int          `json:"bottom_left,omitempty"` // MIDI note of the bottom-left pad that was sent
	Sent       time.Time     `json:"sent,omitzero"`         // when it was sent (LastSent only)
}

// SaveLastSent records what was just sent, keeping the rest of the config.
func (s *Store) SaveLastSent(p Preset) error {
	c, err := s.Config()
	if err != nil {
		return err
	}
	p.Scale = key(p.Scale)
	c.LastSent = &p
	return s.SaveConfig(c)
}

// Config reads config.json; a missing file gives the zero Config.
func (s *Store) Config() (Config, error) {
	var c Config
	err := readJSON(filepath.Join(s.Dir, "config.json"), &c)
	return c, err
}

// SaveConfig writes config.json.
func (s *Store) SaveConfig(c Config) error {
	return writeJSON(filepath.Join(s.Dir, "config.json"), c)
}

func (s *Store) scales() (map[string]ScaleSettings, error) {
	m := map[string]ScaleSettings{}
	err := readJSON(filepath.Join(s.Dir, "scales.json"), &m)
	return m, err
}

// ScaleSettings returns the saved settings for the scale file at path.
func (s *Store) ScaleSettings(path string) (ScaleSettings, bool, error) {
	m, err := s.scales()
	if err != nil {
		return ScaleSettings{}, false, err
	}
	v, ok := m[key(path)]
	return v, ok, nil
}

// SaveScaleSettings records the settings for the scale file at path.
func (s *Store) SaveScaleSettings(path string, v ScaleSettings) error {
	m, err := s.scales()
	if err != nil {
		return err
	}
	m[key(path)] = v
	return writeJSON(filepath.Join(s.Dir, "scales.json"), m)
}

func key(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	return path
}

// Presets returns every preset, sorted by name.
func (s *Store) Presets() ([]Preset, error) {
	files, err := filepath.Glob(filepath.Join(s.Dir, "presets", "*.json"))
	if err != nil {
		return nil, err
	}
	var out []Preset
	for _, f := range files {
		var p Preset
		if err := readJSON(f, &p); err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(f), err)
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

// HasPreset reports whether a preset with this name exists.
func (s *Store) HasPreset(name string) bool {
	_, err := os.Stat(s.presetPath(name))
	return err == nil
}

// SavePreset writes the preset, replacing one with the same name.
func (s *Store) SavePreset(p Preset) error {
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("preset needs a name")
	}
	p.Scale = key(p.Scale)
	return writeJSON(s.presetPath(p.Name), p)
}

// DeletePreset removes the named preset.
func (s *Store) DeletePreset(name string) error {
	return os.Remove(s.presetPath(name))
}

// presetPath maps a name to a file name: letters, digits, '-', '_' and '.' are
// kept, everything else becomes '_'.
func (s *Store) presetPath(name string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return filepath.Join(s.Dir, "presets", b.String()+".json")
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// writeJSON writes through a temporary file, so a crash never leaves half a file.
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
