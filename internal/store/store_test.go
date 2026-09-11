package store

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEmptyStore(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if c, err := s.Config(); err != nil || !reflect.DeepEqual(c, Config{}) {
		t.Errorf("config %+v %v", c, err)
	}
	if _, ok, err := s.ScaleSettings("/x.scl"); ok || err != nil {
		t.Errorf("settings %v %v", ok, err)
	}
	if p, err := s.Presets(); len(p) != 0 || err != nil {
		t.Errorf("presets %v %v", p, err)
	}
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	c := Config{ScaleDirs: []string{"/a"}, Synth: "Pigments", SynthBend: 24}
	if err := s.SaveConfig(c); err != nil {
		t.Fatal(err)
	}
	scl := filepath.Join(dir, "31-edo.scl")
	os.WriteFile(scl, []byte("x"), 0o644)
	v := ScaleSettings{Offset: 10, Low: -1, Root: 60, Scheme: "names", Limit: 7, Synth: "Aalto / Kaivo", SynthBend: 12,
		Palette: []string{"magenta", "off"}}
	if err := s.SaveScaleSettings(scl, v); err != nil {
		t.Fatal(err)
	}
	s.SaveScaleSettings(filepath.Join(dir, "other.scl"), ScaleSettings{Offset: 3})
	p := Preset{Name: "31edo Aalto/names", Scale: scl, Settings: v, Slot: 2, WithConfig: true}
	if err := s.SavePreset(p); err != nil {
		t.Fatal(err)
	}

	s2, _ := Open(dir)
	if got, _ := s2.Config(); !reflect.DeepEqual(got, c) {
		t.Errorf("config %+v", got)
	}
	if got, ok, _ := s2.ScaleSettings(scl); !ok || !reflect.DeepEqual(got, v) {
		t.Errorf("settings %+v", got)
	}
	ps, _ := s2.Presets()
	if len(ps) != 1 || ps[0].Name != p.Name || ps[0].Settings.Offset != 10 || !s2.HasPreset(p.Name) {
		t.Fatalf("presets %+v", ps)
	}
	if _, err := os.Stat(filepath.Join(dir, "presets", "31edo_Aalto_names.json")); err != nil {
		t.Error(err)
	}
	if err := s2.DeletePreset(p.Name); err != nil || s2.HasPreset(p.Name) {
		t.Errorf("delete: %v", err)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".tmp-*")); len(left) != 0 {
		t.Errorf("temporary files left: %v", left)
	}
}

func TestBadJSONIsAnError(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	os.WriteFile(filepath.Join(dir, "config.json"), []byte("{"), 0o644)
	if _, err := s.Config(); err == nil {
		t.Error("broken config.json should be reported")
	}
}
