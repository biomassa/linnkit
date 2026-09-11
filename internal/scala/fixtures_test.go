package scala

import (
	"os"
	"testing"

	"github.com/biomassa/linnkit/internal/testutil"
)

// The curated SCL files are test fixtures: they must exist and be non-empty.
func TestSCLFixturesPresent(t *testing.T) {
	files := testutil.SCLFiles(t)
	if len(files) == 0 {
		t.Fatal("no .scl files in SCL/")
	}
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("%s: empty file", f)
		}
	}
}
