package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/biomassa/linnkit/internal/layout"
	"github.com/biomassa/linnkit/internal/lights"
	"github.com/biomassa/linnkit/internal/scala"
	"github.com/biomassa/linnkit/internal/theory"
)

func runGrid(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("grid", flag.ContinueOnError)
	fs.SetOutput(stderr)
	offset := fs.Int("offset", 0, "row offset in notes (0 = best candidate)")
	low := fs.Int("low", -1, "MIDI note of the bottom-left pad (-1 = root on row 4, column 1)")
	root := fs.Int("root", 60, "MIDI note of degree 0")
	scheme := fs.String("scheme", "ji", "lights: ji, mos or root")
	limit := fs.Int("limit", 7, "ji scheme: highest prime")
	tol := fs.Float64("tol", 0, "ji scheme: cents tolerance (0 = automatic)")
	top := fs.Int("top", 5, "number of layout candidates to list")
	color := fs.Bool("color", false, "24-bit color grid instead of text labels")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: linnkit grid [flags] FILE")
		fs.PrintDefaults()
		return 2
	}
	s, err := scala.ParseSCLFile(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	a := theory.Analyze(s, theory.DefaultOptions())
	cands := layout.Candidates(a, layout.Options{Root: *root})

	fmt.Fprintf(stdout, "%s  %s\n\nlayout candidates (best first)\n", s.Name, s.Description)
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  rows\tinterval\tperiods\ttriad span\tperiod move\tfits\tscore\t")
	for _, c := range cands[:min(*top, len(cands))] {
		fmt.Fprintf(tw, "  +%d\t%.0f c ~%s\t%.2f\t%s\t%+d cols, %d rows\t%s\t%.2f\t%s\n", c.Offset, c.RowCents, orDash(c.RowRatio),
			c.Periods, spanText(c.ChordSpan), c.PeriodMove[0], c.PeriodMove[1], yesNo(c.Fits), c.Score, strings.Join(c.Tags, ", "))
	}
	tw.Flush()

	chosen := cands[0]
	if *offset > 0 {
		for _, c := range cands {
			if c.Offset == *offset {
				chosen = c
			}
		}
		if chosen.Offset != *offset {
			chosen.Layout = layout.Uniform(*offset, layout.RootLow(*offset, *root))
		}
	}
	l := chosen.Layout
	if *low >= 0 {
		l = layout.Uniform(l.Offset, *low)
	}

	n := a.Structure.Size
	var sw []lights.Swatch
	legend := ""
	switch *scheme {
	case "ji":
		sw = lights.JIFamilies(a, *limit, *tol)
		legend = "R root  3 3-limit  5 5-limit  7 7-limit  11 11-limit  13 13-limit"
	case "mos":
		m, ok := lights.DefaultMOS(a)
		if !ok {
			fmt.Fprintln(stderr, "error: no MOS found in this scale")
			return 1
		}
		sw = lights.MOSPattern(n, m, lights.Magenta, lights.White, lights.Blue)
		legend = fmt.Sprintf("R root  o MOS (%d notes, generator %d, degrees %v)  x other", m.Size, m.Generator, m.Degrees(n))
	case "root":
		sw = lights.RootOnly(n, lights.Magenta)
		legend = "R root"
	default:
		fmt.Fprintf(stderr, "error: unknown scheme %q\n", *scheme)
		return 2
	}
	surface := lights.Paint(l, *root, sw)

	lo, hi := l.Span()
	fmt.Fprintf(stdout, "\nrows +%d, bottom-left MIDI %d, notes %d..%d, root MIDI %d, lights %s\n", l.Offset, l.RowStart[0], lo, hi, *root, *scheme)
	if *color {
		fmt.Fprint(stdout, surface.ANSI())
	} else {
		fmt.Fprint(stdout, surface.Plain())
	}
	fmt.Fprintln(stdout, legend)
	return 0
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func spanText(span float64) string {
	if span < 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f cols", span)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
