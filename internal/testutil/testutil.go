// Package testutil has helpers shared by tests.
package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// RepoRoot returns the directory that contains go.mod.
func RepoRoot(t testing.TB) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the test directory")
		}
		dir = parent
	}
}

// SCLFiles returns the paths of all .scl files in the repo's SCL folder.
func SCLFiles(t testing.TB) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(RepoRoot(t), "SCL", "*.scl"))
	if err != nil {
		t.Fatal(err)
	}
	return files
}
