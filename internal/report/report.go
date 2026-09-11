// Package report formats scale analyses as text lines, for the CLI and the TUI.
package report

import (
	"bytes"
	"fmt"
	"math/big"
	"strings"
	"text/tabwriter"

	"github.com/biomassa/linnkit/internal/theory"
)

// Summary returns the header lines: size and period, class and steps, MOS, warnings.
func Summary(a *theory.Analysis) []string {
	s, st, ji := a.Scale, a.Structure, a.JI
	period := fmt.Sprintf("%.2f c", a.PeriodCents)
	if a.PeriodRatio != nil {
		period = fmt.Sprintf("%s (%.2f c)", RatioText(a.PeriodRatio), a.PeriodCents)
	}
	// Limits only mean something when scale degrees (not just the period) are ratios.
	exactDegrees := 0
	for _, d := range a.Degrees[1:] {
		if d.Ratio != nil {
			exactDegrees++
		}
	}
	limits := ""
	if ji.PrimeLimit > 0 && exactDegrees > 0 {
		limits = fmt.Sprintf("; %d-limit, odd limit %d", ji.PrimeLimit, ji.OddLimit)
		if !ji.AllExact {
			limits += fmt.Sprintf(" (%d of %d degrees are ratios)", exactDegrees, len(a.Degrees)-1)
		}
	}
	out := []string{fmt.Sprintf("%d notes per period %s; pitches: %s%s", st.Size, period, a.PitchKinds(), limits)}

	steps := fmt.Sprintf("step %.2f c", st.StepSizes[0])
	if len(st.StepSizes) > 1 {
		steps = fmt.Sprintf("steps %.1f-%.1f c (%d sizes)", st.StepSizes[0], st.StepSizes[len(st.StepSizes)-1], len(st.StepSizes))
	}
	out = append(out, fmt.Sprintf("class: %s; %s; max deviation from %d-EDO %.1f c", st.Class, steps, st.Size, st.MaxDeviation))
	if sig := st.Signature(); sig != "" {
		gen := ""
		if st.GeneratorSteps > 0 {
			gen = fmt.Sprintf(", generator %d steps = %.1f c", st.GeneratorSteps, st.GeneratorCents)
		}
		out = append(out, fmt.Sprintf("two step sizes: %s, pattern %s%s", sig, st.Pattern, gen))
	}
	for _, w := range s.Warnings {
		out = append(out, "warning: "+w.String())
	}
	if !ji.AllExact {
		out = append(out, fmt.Sprintf("just ratios for cents pitches: simplest within %.1f c", ji.Tol))
	}
	return out
}

// DegreeTable returns the aligned degree table: a header, degrees 0..n-1, then the period.
func DegreeTable(a *theory.Analysis) []string {
	var b bytes.Buffer
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "deg\tcents\tratio\tstep\tjust\t12-TET")
	for k, d := range a.Degrees {
		fmt.Fprintf(tw, "%d\t%.2f\t%s\t%.2f\t%s\t%s\n", k, d.Cents, RatioText(d.Ratio), d.Step, JustText(a.JI.Degrees[k]), theory.Format12(d.Cents))
	}
	n := a.Structure.Size
	fmt.Fprintf(tw, "%d\t%.2f\t%s\t\t%s\t%s\n", n, a.PeriodCents, RatioText(a.PeriodRatio), JustText(a.JI.Degrees[n]), theory.Format12(a.PeriodCents))
	tw.Flush()
	return lines(b.String())
}

// Matrix returns the interval matrix: rows are "from" degrees, columns "to" degrees;
// ratios where exact, cents otherwise.
func Matrix(a *theory.Analysis) []string {
	var b bytes.Buffer
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', tabwriter.AlignRight)
	header := []string{""}
	for j := range a.Degrees {
		header = append(header, fmt.Sprint(j))
	}
	fmt.Fprintln(tw, strings.Join(header, "\t")+"\t")
	for i, row := range a.IntervalMatrix() {
		cells := []string{fmt.Sprint(i)}
		for _, iv := range row {
			cell := fmt.Sprintf("%.1f", iv.Cents)
			if iv.Ratio != nil {
				cell = RatioText(iv.Ratio)
			}
			cells = append(cells, cell)
		}
		fmt.Fprintln(tw, strings.Join(cells, "\t")+"\t")
	}
	tw.Flush()
	return lines(b.String())
}

// RatioText writes ratios as n/d, including whole numbers (2/1), as Scala does.
func RatioText(r *big.Rat) string {
	if r == nil {
		return "-"
	}
	return r.Num().String() + "/" + r.Denom().String()
}

// JustText describes a degree's just reading: its prime limit if exact, else
// the nearest simple ratio with its error.
func JustText(d theory.JIDegree) string {
	switch {
	case d.Ratio == nil:
		return "-"
	case d.Index == 0:
		return ""
	case d.Exact:
		return fmt.Sprintf("%d-lim", d.PrimeLimit)
	default:
		return fmt.Sprintf("%s %+.1f (%d-lim)", RatioText(d.Ratio), d.Error, d.PrimeLimit)
	}
}

func lines(s string) []string {
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}
