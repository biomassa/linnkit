package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/biomassa/linnkit/internal/report"
	"github.com/biomassa/linnkit/internal/scala"
	"github.com/biomassa/linnkit/internal/theory"
)

func runDescribe(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("describe", flag.ContinueOnError)
	fs.SetOutput(stderr)
	matrix := fs.Bool("matrix", false, "also print the interval matrix")
	tol := fs.Float64("tol", 0, "cents tolerance for nearest just ratios (0 = automatic)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(stderr, "usage: linnkit describe [--matrix] [--tol cents] FILE...")
		return 2
	}
	opt := theory.DefaultOptions()
	opt.JITol = *tol
	status := 0
	for i, path := range fs.Args() {
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		s, err := scala.ParseSCLFile(path)
		if err != nil {
			fmt.Fprintln(stderr, "error:", err)
			status = 1
			continue
		}
		writeDescription(stdout, theory.Analyze(s, opt), *matrix)
	}
	return status
}

func writeDescription(w io.Writer, a *theory.Analysis, matrix bool) {
	fmt.Fprintf(w, "%s  %s\n", a.Scale.Name, a.Scale.Description)
	for _, l := range report.Summary(a) {
		fmt.Fprintln(w, "  "+l)
	}
	fmt.Fprintln(w)
	for _, l := range report.DegreeTable(a) {
		fmt.Fprintln(w, "  "+l)
	}
	if matrix {
		fmt.Fprintln(w, "\n  intervals from row degree up to column degree (cents; ratio when exact)")
		for _, l := range report.Matrix(a) {
			fmt.Fprintln(w, "  "+l)
		}
	}
}
