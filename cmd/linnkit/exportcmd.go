package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/biomassa/linnkit/internal/export"
	"github.com/biomassa/linnkit/internal/scala"
	"github.com/biomassa/linnkit/internal/store"
)

// runExport copies scales plus a matching .kbm to the Madrona Labs scale
// folder. Each scale uses its saved root and reference frequency, or --root
// and its 12-TET frequency.
func runExport(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", export.MadronaDir(), "target folder")
	root := fs.Int("root", 60, "MIDI note of degree 0 for scales with no saved settings")
	dry := fs.Bool("n", false, "show what would be written, write nothing")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(stderr, "usage: linnkit export [-n] [--root n] [--dir DIR] FILE...")
		return 2
	}
	st, _ := store.Open(store.DefaultDir())
	var items []export.Item
	for _, path := range fs.Args() {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		s, err := scala.ParseSCL(data)
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", path, err)
			return 1
		}
		if p := export.MadronaProblems(data, s); len(p) > 0 {
			fmt.Fprintf(stdout, "skip   %s: %s\n", filepath.Base(path), p[0])
			continue
		}
		r, hz := *root, 0.0
		if st != nil {
			if v, ok, _ := st.ScaleSettings(path); ok && v.Root > 0 {
				r, hz = v.Root, v.RefHz
			}
		}
		if hz <= 0 {
			hz = scala.StandardFreq(r)
		}
		items = append(items, export.Item{Src: path, Scale: s, Root: r, RefFreq: hz})
	}
	files, err := export.Madrona(items, *dir)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	for _, f := range files {
		state := "new    "
		switch {
		case f.Same:
			state = "same   "
		case f.Existed:
			state = "replace"
		}
		fmt.Fprintln(stdout, state, f.Path)
	}
	if *dry {
		return 0
	}
	n, err := export.Write(files)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	fmt.Fprintf(stdout, "%d files written to %s\n", n, *dir)
	return 0
}
