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
	dirs := defaultScaleDirs()
	if *scales != "" {
		dirs = strings.Split(*scales, ",")
	}
	_, err := tea.NewProgram(tui.New(tui.Config{ScaleDirs: dirs, Port: *port})).Run()
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
	isDir := func(p string) bool {
		st, err := os.Stat(p)
		return err == nil && st.IsDir()
	}
	home, _ := os.UserHomeDir()
	var candidates []string
	if abs, err := filepath.Abs("SCL"); err == nil {
		candidates = append(candidates, abs)
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "..", "SCL"))
	}
	candidates = append(candidates, filepath.Join(home, "SCL"))
	var dirs []string
	seen := map[string]bool{}
	for _, c := range candidates {
		c = filepath.Clean(c)
		if real, err := filepath.EvalSymlinks(c); err == nil {
			c = real
		}
		if isDir(c) && !seen[c] {
			seen[c] = true
			dirs = append(dirs, c)
		}
	}
	if len(dirs) == 0 {
		dirs = []string{filepath.Join(home, "code", "linnkit", "SCL")}
	}
	return dirs
}
