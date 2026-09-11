package main

import (
	"flag"
	"fmt"
	"io"
	"math/big"
	"strings"
	"text/tabwriter"

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
	s, st, ji := a.Scale, a.Structure, a.JI
	fmt.Fprintf(w, "%s  %s\n", s.Name, s.Description)

	period := fmt.Sprintf("%.2f c", a.PeriodCents)
	if a.PeriodRatio != nil {
		period = fmt.Sprintf("%s (%.2f c)", ratioText(a.PeriodRatio), a.PeriodCents)
	}
	limits := ""
	if ji.PrimeLimit > 0 {
		limits = fmt.Sprintf("; %d-limit, odd limit %d", ji.PrimeLimit, ji.OddLimit)
	}
	fmt.Fprintf(w, "  %d notes per period %s; pitches: %s%s\n", st.Size, period, a.PitchKinds(), limits)

	steps := fmt.Sprintf("step %.2f c", st.StepSizes[0])
	if len(st.StepSizes) > 1 {
		steps = fmt.Sprintf("steps %.1f-%.1f c (%d sizes)", st.StepSizes[0], st.StepSizes[len(st.StepSizes)-1], len(st.StepSizes))
	}
	fmt.Fprintf(w, "  class: %s; %s; max deviation from %d-EDO %.1f c\n", st.Class, steps, st.Size, st.MaxDeviation)
	if sig := st.Signature(); sig != "" {
		gen := ""
		if st.GeneratorSteps > 0 {
			gen = fmt.Sprintf(", generator %d steps = %.1f c", st.GeneratorSteps, st.GeneratorCents)
		}
		fmt.Fprintf(w, "  two step sizes: %s, pattern %s%s\n", sig, st.Pattern, gen)
	}
	for _, warn := range s.Warnings {
		fmt.Fprintf(w, "  warning: %s\n", warn)
	}
	if !ji.AllExact {
		fmt.Fprintf(w, "  just ratios for cents pitches: simplest within %.1f c\n", ji.Tol)
	}
	fmt.Fprintln(w)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  deg\tcents\tratio\tstep\tjust\t12-TET")
	for k, d := range a.Degrees {
		fmt.Fprintf(tw, "  %d\t%.2f\t%s\t%.2f\t%s\t%s\n", k, d.Cents, ratioText(d.Ratio), d.Step, justText(ji.Degrees[k]), theory.Format12(d.Cents))
	}
	fmt.Fprintf(tw, "  %d\t%.2f\t%s\t\t%s\t%s\n", st.Size, a.PeriodCents, ratioText(a.PeriodRatio), justText(ji.Degrees[st.Size]), theory.Format12(a.PeriodCents))
	tw.Flush()

	if matrix {
		fmt.Fprintln(w, "\n  intervals from row degree up to column degree (cents; ratio when exact)")
		tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', tabwriter.AlignRight)
		header := []string{"  "}
		for j := range a.Degrees {
			header = append(header, fmt.Sprint(j))
		}
		fmt.Fprintln(tw, strings.Join(header, "\t")+"\t")
		for i, row := range a.IntervalMatrix() {
			cells := []string{fmt.Sprintf("  %d", i)}
			for _, iv := range row {
				cell := fmt.Sprintf("%.1f", iv.Cents)
				if iv.Ratio != nil {
					cell = ratioText(iv.Ratio)
				}
				cells = append(cells, cell)
			}
			fmt.Fprintln(tw, strings.Join(cells, "\t")+"\t")
		}
		tw.Flush()
	}
}

// ratioText writes ratios as n/d, including whole numbers (2/1), as Scala does.
func ratioText(r *big.Rat) string {
	if r == nil {
		return "-"
	}
	return r.Num().String() + "/" + r.Denom().String()
}

func justText(d theory.JIDegree) string {
	switch {
	case d.Ratio == nil:
		return "-"
	case d.Index == 0:
		return ""
	case d.Exact:
		return fmt.Sprintf("%d-lim", d.PrimeLimit)
	default:
		return fmt.Sprintf("%s %+.1f (%d-lim)", ratioText(d.Ratio), d.Error, d.PrimeLimit)
	}
}
