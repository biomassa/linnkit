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
	generator, mossize       int
	harmonics, subharmonics  int
}

func (g *gridOpts) settings() lights.Settings {
	return lights.Settings{Limit: g.limit, Generator: g.generator, MOSSize: g.mossize, Harmonics: g.harmonics,
		Subharmonics: g.subharmonics != 0}
}

func addGridFlags(fs *flag.FlagSet) *gridOpts {
	g := &gridOpts{}
	fs.IntVar(&g.offset, "offset", 0, "row offset in notes (0 = best candidate)")
	fs.IntVar(&g.low, "low", -1, "MIDI note of the bottom-left pad (-1 = root on row 4, column 1)")
	fs.IntVar(&g.root, "root", 60, "MIDI note of degree 0")
	fs.StringVar(&g.scheme, "scheme", "ji", "lights: root, ji, names, mos, "+strings.Join(lights.MoreSchemes, ", "))
	fs.IntVar(&g.limit, "limit", 7, "ji, kite, factors: highest prime")
	fs.Float64Var(&g.tol, "tol", 0, "ji scheme: cents tolerance (0 = automatic)")
	fs.IntVar(&g.generator, "generator", 0, "chain, moskeys, wijmenga, nested: generator in degrees (0 = the degree nearest 3/2)")
	fs.IntVar(&g.mossize, "mossize", 0, "moskeys: MOS size in notes (0 = automatic)")
	fs.IntVar(&g.harmonics, "harmonics", 16, "harmonics scheme: 16 (harmonics 1-16) or 32 (16-32)")
	fs.IntVar(&g.subharmonics, "subharmonics", 1, "harmonics scheme: 1 = also the subharmonics, 0 = not")
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
		sw = lights.MOSPattern(n, m, lights.Magenta, lights.White, lights.Off)
		r.legend = fmt.Sprintf("R root  o MOS (%d notes, generator %d, degrees %v)  other degrees unlit", m.Size, m.Generator, m.Degrees(n))
	case "names":
		sw = lights.NoteNames(r.analysis, lights.Magenta, lights.White, lights.Blue, lights.Green)
		r.legend = "C root (magenta)  naturals white  sharps blue  flats green  . other"
	case "root":
		sw = lights.RootOnly(n, lights.Magenta)
		r.legend = "R root"
	default:
		if g.harmonics != 16 && g.harmonics != 32 {
			return nil, fmt.Errorf("-harmonics must be 16 or 32, not %d", g.harmonics)
		}
		if sw, r.legend, err = lights.Scheme(g.scheme, r.analysis, g.settings()); err != nil {
			return nil, err
		}
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
	cc := fs.Bool("cc", false, "also print each pad's CC22 colour number")
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
	if *cc {
		fmt.Fprint(stdout, "\nCC22 colours (0 = unlit)\n", r.surface.CC())
	}
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
