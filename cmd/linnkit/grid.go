package main

import (
	"errors"
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

// gridOpts are the flags shared by grid and send.
type gridOpts struct {
	offset, low, root, limit int
	scheme                   string
	tol                      float64
}

func addGridFlags(fs *flag.FlagSet) *gridOpts {
	g := &gridOpts{}
	fs.IntVar(&g.offset, "offset", 0, "row offset in notes (0 = best candidate)")
	fs.IntVar(&g.low, "low", -1, "MIDI note of the bottom-left pad (-1 = root on row 4, column 1)")
	fs.IntVar(&g.root, "root", 60, "MIDI note of degree 0")
	fs.StringVar(&g.scheme, "scheme", "ji", "lights: ji, names, mos or root")
	fs.IntVar(&g.limit, "limit", 7, "ji scheme: highest prime")
	fs.Float64Var(&g.tol, "tol", 0, "ji scheme: cents tolerance (0 = automatic)")
	return g
}

type gridResult struct {
	analysis   *theory.Analysis
	candidates []layout.Candidate
	layout     layout.Layout
	surface    lights.Surface
	legend     string
}

func (g *gridOpts) build(path string) (*gridResult, error) {
	s, err := scala.ParseSCLFile(path)
	if err != nil {
		return nil, err
	}
	r := &gridResult{analysis: theory.Analyze(s, theory.DefaultOptions())}
	r.candidates = layout.Candidates(r.analysis, layout.Options{Root: g.root})
	r.layout = r.candidates[0].Layout
	if g.offset > 0 {
		r.layout = layout.Uniform(g.offset, layout.RootLow(g.offset, g.root))
	}
	if g.low >= 0 {
		r.layout = layout.Uniform(r.layout.Offset, g.low)
	}
	n := r.analysis.Structure.Size
	var sw []lights.Swatch
	switch g.scheme {
	case "ji":
		sw = lights.JIFamilies(r.analysis, g.limit, g.tol)
		r.legend = "R root  3 3-limit  5 5-limit  7 7-limit  11 11-limit  13 13-limit"
	case "mos":
		m, ok := lights.DefaultMOS(r.analysis)
		if !ok {
			return nil, errors.New("no MOS found in this scale")
		}
		sw = lights.MOSPattern(n, m, lights.Magenta, lights.White, lights.Blue)
		r.legend = fmt.Sprintf("R root  o MOS (%d notes, generator %d, degrees %v)  x other", m.Size, m.Generator, m.Degrees(n))
	case "names":
		sw = lights.NoteNames(r.analysis, lights.Magenta, lights.White, lights.Blue, lights.Green)
		r.legend = "C root (magenta)  naturals white  sharps blue  flats green  . other"
	case "root":
		sw = lights.RootOnly(n, lights.Magenta)
		r.legend = "R root"
	default:
		return nil, fmt.Errorf("unknown scheme %q", g.scheme)
	}
	r.surface = lights.Paint(r.layout, g.root, sw)
	return r, nil
}

func runGrid(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("grid", flag.ContinueOnError)
	fs.SetOutput(stderr)
	g := addGridFlags(fs)
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
	r, err := g.build(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	s := r.analysis.Scale
	fmt.Fprintf(stdout, "%s  %s\n\nlayout candidates (best first)\n", s.Name, s.Description)
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  rows\tinterval\tperiods\ttriad span\tperiod move\tfits\tscore\t")
	for _, c := range r.candidates[:min(*top, len(r.candidates))] {
		fmt.Fprintf(tw, "  +%d\t%.0f c ~%s\t%.2f\t%s\t%+d cols, %d rows\t%s\t%.2f\t%s\n", c.Offset, c.RowCents, orDash(c.RowRatio),
			c.Periods, spanText(c.ChordSpan), c.PeriodMove[0], c.PeriodMove[1], yesNo(c.Fits), c.Score, strings.Join(c.Tags, ", "))
	}
	tw.Flush()
	writeSurface(stdout, r, g, *color)
	return 0
}

func writeSurface(w io.Writer, r *gridResult, g *gridOpts, color bool) {
	lo, hi := r.layout.Span()
	fmt.Fprintf(w, "\nrows +%d, bottom-left MIDI %d, notes %d..%d, root MIDI %d, lights %s\n",
		r.layout.Offset, r.layout.RowStart[0], lo, hi, g.root, g.scheme)
	if color {
		fmt.Fprint(w, r.surface.ANSI())
	} else {
		fmt.Fprint(w, r.surface.Plain())
	}
	fmt.Fprintln(w, r.legend)
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
