package scala

import (
	"strings"
	"testing"
)

func TestLinearMappingRoundTrip(t *testing.T) {
	m := LinearMapping(5, 60, StandardFreq(60))
	text := m.Format("5edo.kbm")
	if !strings.Contains(text, "261.625565\n") {
		t.Errorf("reference frequency not written as expected:\n%s", text)
	}
	got, err := ParseKBM([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	if got.Size != 5 || got.MiddleNote != 60 || got.RefNote != 60 || got.FormalOctave != 5 || len(got.Keys) != 5 || got.Keys[4] != 4 {
		t.Errorf("round trip: %+v", got)
	}
}

func TestParseKBMUnmappedAndShort(t *testing.T) {
	src := "! 7 of 12\n12\n0\n127\n60\n69\n440.0\n12\n0\nx\n2\n"
	m, err := ParseKBM([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Keys) != 12 || m.Keys[1] != -1 || m.Keys[2] != 2 || m.Keys[11] != -1 {
		t.Errorf("keys: %v", m.Keys)
	}
	if len(m.Warnings) != 1 {
		t.Errorf("want one warning for missing entries, got %v", m.Warnings)
	}
}

func TestParseKBMErrors(t *testing.T) {
	for name, src := range map[string]string{
		"empty":   "",
		"short":   "12\n0\n127\n",
		"bad ref": "0\n0\n127\n60\n69\nabc\n12\n",
	} {
		if _, err := ParseKBM([]byte(src)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}
