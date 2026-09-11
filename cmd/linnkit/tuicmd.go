package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/biomassa/linnkit/internal/device"
	"github.com/biomassa/linnkit/internal/store"
	"github.com/biomassa/linnkit/internal/tui"
)

func runTUI(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	fs.SetOutput(stderr)
	scales := fs.String("scales", "", "comma-separated folders of .scl files (default: the project SCL folder)")
	port := fs.String("port", device.DefaultPortName, "MIDI port name (substring)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	st, err := store.Open(store.DefaultDir())
	if err != nil {
		fmt.Fprintln(stderr, "app data:", err)
		st = nil
	}
	dirs := defaultScaleDirs()
	if st != nil {
		if c, err := st.Config(); err != nil {
			fmt.Fprintln(stderr, "config:", err)
		} else {
			dirs = uniqueDirs(append(dirs, c.ScaleDirs...))
		}
	}
	if *scales != "" {
		dirs = strings.Split(*scales, ",")
	}
	_, err = tea.NewProgram(tui.New(tui.Config{ScaleDirs: dirs, Port: *port, Store: st})).Run()
	device.CloseDriver()
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}

// defaultScaleDirs returns every scale folder that exists, without duplicates:
// the project SCL folder (in the current directory, or next to the binary as
// bin/../SCL) and ~/SCL. Falls back to ~/code/linnkit/SCL.
func defaultScaleDirs() []string {
	home, _ := os.UserHomeDir()
	var candidates []string
	if abs, err := filepath.Abs("SCL"); err == nil {
		candidates = append(candidates, abs)
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "..", "SCL"))
	}
	candidates = append(candidates, filepath.Join(home, "SCL"))
	dirs := uniqueDirs(candidates)
	if len(dirs) == 0 {
		dirs = []string{filepath.Join(home, "code", "linnkit", "SCL")}
	}
	return dirs
}

// uniqueDirs keeps the folders that exist, resolving symlinks and dropping duplicates.
func uniqueDirs(candidates []string) []string {
	var dirs []string
	seen := map[string]bool{}
	for _, c := range candidates {
		c = expandHome(filepath.Clean(c))
		if real, err := filepath.EvalSymlinks(c); err == nil {
			c = real
		}
		if st, err := os.Stat(c); err == nil && st.IsDir() && !seen[c] {
			seen[c] = true
			dirs = append(dirs, c)
		}
	}
	return dirs
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[1:])
	}
	return p
}
