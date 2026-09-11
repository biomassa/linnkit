package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/biomassa/linnkit/internal/store"
)

// runFolders lists, adds or removes extra scale folders in the app config.
func runFolders(args []string, stdout, stderr io.Writer) int {
	st, err := store.Open(store.DefaultDir())
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	c, err := st.Config()
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	if len(args) == 0 {
		fmt.Fprintln(stdout, "default folders (when present):")
		for _, d := range defaultScaleDirs() {
			fmt.Fprintln(stdout, "  "+d)
		}
		fmt.Fprintln(stdout, "extra folders (linnkit folders add|remove DIR):")
		for _, d := range c.ScaleDirs {
			note := ""
			if len(uniqueDirs([]string{d})) == 0 {
				note = "  (missing)"
			}
			fmt.Fprintln(stdout, "  "+d+note)
		}
		return 0
	}
	if len(args) != 2 || (args[0] != "add" && args[0] != "remove") {
		fmt.Fprintln(stderr, "usage: linnkit folders [add|remove DIR]")
		return 2
	}
	dir, err := filepath.Abs(expandHome(args[1]))
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	switch args[0] {
	case "add":
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			fmt.Fprintln(stderr, "error: not a folder:", dir)
			return 1
		}
		if slices.Contains(c.ScaleDirs, dir) {
			fmt.Fprintln(stdout, "already listed:", dir)
			return 0
		}
		c.ScaleDirs = append(c.ScaleDirs, dir)
	case "remove":
		i := slices.Index(c.ScaleDirs, dir)
		if i < 0 {
			fmt.Fprintln(stderr, "not listed:", dir)
			return 1
		}
		c.ScaleDirs = slices.Delete(c.ScaleDirs, i, i+1)
	}
	if err := st.SaveConfig(c); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	verb := "added"
	if args[0] == "remove" {
		verb = "removed"
	}
	fmt.Fprintln(stdout, verb, dir)
	return 0
}
