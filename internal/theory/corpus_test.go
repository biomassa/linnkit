package theory

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/biomassa/linnkit/internal/scala"
)

// TestCorpusClasses analyzes every file in the corpus (see scala's corpus test
// for LINNKIT_CORPUS) and logs how the scales classify. Run with -v to see it.
func TestCorpusClasses(t *testing.T) {
	dirs := filepath.SplitList(os.Getenv("LINNKIT_CORPUS"))
	if len(dirs) == 0 || dirs[0] == "" {
		home, _ := os.UserHomeDir()
		dirs = []string{filepath.Join(home, "Dropbox", "! modular", "five12", "SCALA scales")}
	}
	counts := map[string]int{}
	periods := map[string]int{}
	total := 0
	for _, dir := range dirs {
		if _, err := os.Stat(dir); err != nil {
			t.Skip("no corpus")
		}
		filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(path), ".scl") {
				return nil
			}
			s, err := scala.ParseSCLFile(path)
			if err != nil {
				return nil
			}
			a := Analyze(s, DefaultOptions())
			total++
			counts[a.Structure.Class.String()]++
			switch p := a.PeriodCents; {
			case p > 1199.99 && p < 1200.01:
				periods["octave"]++
			case p > 1901.95 && p < 1901.96:
				periods["tritave"]++
			default:
				periods["other"]++
			}
			return nil
		})
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	t.Logf("%d scales analyzed", total)
	for _, k := range keys {
		t.Logf("  %-11s %d", k, counts[k])
	}
	t.Logf("  periods: %v", periods)
}
