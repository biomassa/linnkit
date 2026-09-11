package scala

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// CorpusDirs returns folders of .scl files to test against: LINNKIT_CORPUS
// (a path list) or, by default, the five12 archive in Dropbox if it exists.
func corpusDirs() []string {
	if env := os.Getenv("LINNKIT_CORPUS"); env != "" {
		return filepath.SplitList(env)
	}
	home, _ := os.UserHomeDir()
	def := filepath.Join(home, "Dropbox", "! modular", "five12", "SCALA scales")
	if _, err := os.Stat(def); err == nil {
		return []string{def}
	}
	return nil
}

// CorpusFiles lists every .scl file under the corpus folders.
func corpusFiles(t testing.TB) []string {
	dirs := corpusDirs()
	if len(dirs) == 0 {
		t.Skip("no corpus: set LINNKIT_CORPUS to folders of .scl files")
	}
	var files []string
	for _, dir := range dirs {
		filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.EqualFold(filepath.Ext(path), ".scl") {
				files = append(files, path)
			}
			return nil
		})
	}
	return files
}

func TestCorpusParses(t *testing.T) {
	files := corpusFiles(t)
	var empty, warned int
	var failures []string
	for _, f := range files {
		s, err := ParseSCLFile(f)
		switch {
		case errors.Is(err, ErrEmpty):
			empty++
		case err != nil:
			failures = append(failures, err.Error())
		case len(s.Warnings) > 0:
			warned++
		}
	}
	t.Logf("%d files: %d parsed (%d with warnings), %d empty, %d failed", len(files), len(files)-empty-len(failures), warned, empty, len(failures))
	for i, f := range failures {
		if i == 20 {
			t.Logf("... and %d more", len(failures)-20)
			break
		}
		t.Log(f)
	}
	if len(failures) > 0 {
		t.Errorf("%d files failed to parse", len(failures))
	}
}
