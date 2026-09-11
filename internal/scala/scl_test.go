package scala

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/biomassa/linnkit/internal/testutil"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestParseRatiosAndComments(t *testing.T) {
	src := "! pyth.scl\n!\nPythagorean fragment\n 3\n!\n9/8\n 81/64   E\n2\n"
	s, err := ParseSCL([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if s.Description != "Pythagorean fragment" || s.Size() != 3 || len(s.Warnings) != 0 {
		t.Fatalf("got %q, %d pitches, warnings %v", s.Description, s.Size(), s.Warnings)
	}
	if !s.Pitches[0].IsRatio() || s.Pitches[0].Ratio.RatString() != "9/8" || !near(s.Pitches[0].Cents, 203.91000173077484) {
		t.Errorf("degree 1: %+v", s.Pitches[0])
	}
	if s.Pitches[1].Text != "81/64" || s.Pitches[1].Line != 7 {
		t.Errorf("trailing text or line number: %+v", s.Pitches[1])
	}
	if s.Period().Ratio.RatString() != "2" || !near(s.Period().Cents, 1200) {
		t.Errorf("period: %+v", s.Period())
	}
}

func TestParseCents(t *testing.T) {
	s, err := ParseSCL([]byte("cents\n3\n408.\n100.0cents\n1200.0\n"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Pitches[0].IsRatio() || !near(s.Pitches[0].Cents, 408) || !near(s.Pitches[1].Cents, 100) {
		t.Errorf("pitches: %+v", s.Pitches)
	}
	if len(s.Warnings) != 1 || !strings.Contains(s.Warnings[0].Msg, "not above") {
		t.Errorf("want one ordering warning, got %v", s.Warnings)
	}
}

func TestParseLineEndingsAndEncoding(t *testing.T) {
	for name, src := range map[string][]byte{
		"CR":   []byte("d\r2\r3/2\r2/1\r"),
		"CRLF": []byte("d\r\n2\r\n3/2\r\n2/1\r\n"),
	} {
		s, err := ParseSCL(src)
		if err != nil || s.Size() != 2 {
			t.Errorf("%s: %v, %+v", name, err, s)
		}
	}
	s, err := ParseSCL([]byte("caf\xe9\n1\n2/1\n"))
	if err != nil || s.Description != "café" {
		t.Errorf("latin-1 description: %v %q", err, s.Description)
	}
}

func TestParseEmptyDescriptionAndMismatch(t *testing.T) {
	s, err := ParseSCL([]byte("\n4\n9/8\n5/4\n2/1\n"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Description != "" || s.Size() != 3 || len(s.Warnings) != 1 || !strings.Contains(s.Warnings[0].Msg, "declared 4") {
		t.Errorf("got %q, %d pitches, %v", s.Description, s.Size(), s.Warnings)
	}
}

func TestParseErrors(t *testing.T) {
	if _, err := ParseSCL(nil); !errors.Is(err, ErrEmpty) {
		t.Errorf("empty: %v", err)
	}
	cases := map[string]string{
		"no count":       "desc\n",
		"bad count":      "desc\nx\n",
		"negative ratio": "d\n1\n-3/2\n",
		"zero ratio":     "d\n1\n0/1\n",
		"garbage":        "d\n1\nabc\n",
		"no notes":       "d\n0\n",
	}
	for name, src := range cases {
		if _, err := ParseSCL([]byte(src)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestParseFixtures(t *testing.T) {
	want := map[string]int{"22edo": 22, "31-edo": 31, "ji_7": 7, "ji_9": 9, "ji_13": 13, "ji_17": 17}
	for _, f := range testutil.SCLFiles(t) {
		s, err := ParseSCLFile(f)
		if err != nil {
			t.Errorf("%v", err)
			continue
		}
		if n, ok := want[s.Name]; ok && s.Size() != n {
			t.Errorf("%s: %d notes, want %d", s.Name, s.Size(), n)
		}
		if len(s.Warnings) != 0 {
			t.Errorf("%s: unexpected warnings %v", s.Name, s.Warnings)
		}
	}
}
