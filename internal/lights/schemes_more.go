package lights

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/biomassa/linnkit/internal/theory"
)

// The schemes below follow the shared spec in the Max project
// (~/Dropbox/musicstuff/Max 9/linnstrument/docs/light-schemes.md), which the
// Max port linn.lights is checked against through `linnkit grid`. Decisions
// the spec left open (2026-09-12, the user's choices):
//   - a chain position reached by several k: the one closest to k = 2, ties to the lower k
//   - chain: positions outside the coloured range are unlit but keep their name
//   - chain labels the root C; the other new schemes label it R
//   - moskeys: the automatic size is searched along the chosen generator; no MOS = root only
//   - steps: more than 6 middle classes cycle the middle colours
//   - factors: labels are the primes side by side (3, 35, 357)
//   - harmonics: a degree hit by both series takes its smallest harmonic number

// Settings are the per-scale options of the light schemes.
type Settings struct {
	Limit        int  // highest prime for ji, kite and factors (3-13)
	Generator    int  // generator in degrees; 0 = the degree nearest 3/2
	MOSSize      int  // moskeys: notes; 0 = automatic
	Harmonics    int  // 16 = harmonics 1-16, 32 = harmonics 16-32
	Subharmonics bool // harmonics: also the subharmonic series
}

// DefaultSettings: limit 7, automatic generator and MOS size, harmonics 1-16 with subharmonics.
func DefaultSettings() Settings { return Settings{Limit: 7, Harmonics: 16, Subharmonics: true} }

// MoreSchemes are the schemes after root, ji, names and mos, in the spec's order.
var MoreSchemes = []string{"chain", "moskeys", "wijmenga", "kite", "factors", "steps", "nested", "consonance", "harmonics"}

// Scheme builds one of MoreSchemes: a swatch per degree and a legend.
func Scheme(name string, a *theory.Analysis, s Settings) ([]Swatch, string, error) {
	switch name {
	case "chain":
		return Chain(a, s), fmt.Sprintf("C root  white k 0..4  yellow 5..8  green -1..-4  blue 9..12  red -5..-8  "+
			"cyan 13..16  pink -9..-12  (generator %d)", Generator(a, s)), nil
	case "moskeys":
		sw, m, ok := MOSKeys(a, s)
		if !ok {
			return sw, "no MOS found in this scale: root only", nil
		}
		return sw, fmt.Sprintf("R root  white MOS (%d notes, generator %d)  blue other", m.Size, m.Generator), nil
	case "wijmenga":
		sw, c, chroma, meantone := Wijmenga(a, s)
		if meantone {
			return sw, "R root  green naturals  white sharps  flats unlit  yellow outer sharps  blue outer flats  (meantone)", nil
		}
		return sw, fmt.Sprintf("R root  white naturals and a comma either side  green sharps  orange flats  (comma %d, chroma %d degrees)", c, chroma), nil
	case "kite":
		return Kite(a, s), fmt.Sprintf("R root  wa white  yo yellow  gu green  zo blue  ru red  ilo lime  lu orange  tho pink  thu cyan  (limit %d)", s.Limit), nil
	case "factors":
		return Factors(a, s), fmt.Sprintf("R root (unlit)  3 red  5 green  7 blue  35 yellow  57 cyan  37 magenta  357 white  (limit %d)", s.Limit), nil
	case "steps":
		sw, k := Steps(a)
		return sw, fmt.Sprintf("R root  %d step sizes, largest first: %s", k, stepsLegend(k)), nil
	case "nested":
		sw, layers := Nested(a, s)
		if len(layers) == 0 {
			return sw, "no MOS layers found: root only", nil
		}
		var parts []string
		for i, size := range layers {
			parts = append(parts, fmt.Sprintf("%d %s (%d notes)", i+1, layerColors[i], size))
		}
		return sw, "R root  " + strings.Join(parts, "  "), nil
	case "consonance":
		return Consonance(a), "R root  odd limit <=5 white  <=9 green  <=15 blue  higher cyan", nil
	case "harmonics":
		lo, hi := harmonicRange(s)
		l := fmt.Sprintf("R root  harmonics %d-%d yellow", lo, hi)
		if s.Subharmonics {
			l += "  subharmonics blue (u)  both white"
		}
		return Harmonics(a, s), l, nil
	}
	return nil, "", fmt.Errorf("unknown scheme %q", name)
}

func tolerance(a *theory.Analysis) float64 { return math.Min(15, 0.35*a.Structure.EqualStep) }

func mod(x, n int) int { return (x%n + n) % n }

func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// Generator returns the generator in degrees: the setting, or the degree nearest 3/2.
func Generator(a *theory.Analysis, s Settings) int {
	n := len(a.Degrees)
	if s.Generator > 0 {
		return s.Generator % n
	}
	g, _ := nearestDegree(a, 1200*math.Log2(1.5))
	return g
}

const noPos = math.MinInt

// chainPositions returns each degree's chain position: the k in
// [2 - floor((n-1)/2), 2 + ceil((n-1)/2)] with k*g mod n = d, the one closest
// to k = 2 when several reach it (ties to the lower k). Unreached degrees get noPos.
func chainPositions(n, g int) []int {
	pos := make([]int, n)
	for i := range pos {
		pos[i] = noPos
	}
	for k := 2 - (n-1)/2; k <= 2+n/2; k++ {
		d := mod(k*g, n)
		if pos[d] == noPos || abs(k-2) < abs(pos[d]-2) {
			pos[d] = k
		}
	}
	return pos
}

// chainName names chain position k: k = -1..5 are F C G D A E B.
func chainName(k int) string {
	name := fifthsNames[mod(k+1, 7)]
	switch acc := floorDiv(k+1, 7); {
	case acc == 2:
		return name + "x"
	case acc > 0:
		return name + strings.Repeat("#", acc)
	case acc < 0:
		return name + strings.Repeat("b", -acc)
	}
	return name
}

func chainColor(k int) Color {
	switch {
	case k >= 0 && k <= 4:
		return White
	case k >= 5 && k <= 8:
		return Yellow
	case k >= -4 && k <= -1:
		return Green
	case k >= 9 && k <= 12:
		return Blue
	case k >= -8 && k <= -5:
		return Red
	case k >= 13 && k <= 16:
		return Cyan
	case k >= -12 && k <= -9:
		return Pink
	}
	return Off
}

// Chain colours degrees by their place on the chain of generators from the
// root (Lumatone 31-EDO colours after Kite) and labels them with note names.
func Chain(a *theory.Analysis, s Settings) []Swatch {
	n := len(a.Degrees)
	sw := make([]Swatch, n)
	for d, k := range chainPositions(n, Generator(a, s)) {
		if k != noPos {
			sw[d] = Swatch{chainColor(k), chainName(k)}
		}
	}
	sw[0] = Swatch{Magenta, "C"}
	return sw
}

// MOSKeys lights a MOS white and every other degree blue, like the white and
// black keys of a MOS keyboard. ok is false when no automatic size fits.
func MOSKeys(a *theory.Analysis, s Settings) ([]Swatch, MOSChoice, bool) {
	n := len(a.Degrees)
	g := Generator(a, s)
	m := MOSChoice{Generator: g, Size: s.MOSSize, Down: 1}
	if m.Size <= 0 {
		m.Size = 0
		for _, size := range []int{7, 5, 6, 8, 9, 10, 11, 12} {
			c := MOSChoice{Generator: g, Size: size, Down: 1}
			if g > 0 && size < n && isMOS(c.Degrees(n), n) {
				m.Size = size
				break
			}
		}
		if m.Size == 0 {
			return RootOnly(n, Magenta), m, false
		}
	}
	sw := make([]Swatch, n)
	for d := range sw {
		sw[d] = Swatch{Blue, ""}
	}
	for _, d := range m.Degrees(n) {
		sw[d] = Swatch{White, ""}
	}
	sw[0] = Swatch{Magenta, "R"}
	return sw, m, true
}

// Wijmenga colours a keyboard after Wijmenga's microtonal keyboard designs. It
// returns the syntonic comma and the chroma (25/24) in degrees, and whether the
// tuning is meantone (the comma vanishes).
func Wijmenga(a *theory.Analysis, s Settings) ([]Swatch, int, int, bool) {
	n := len(a.Degrees)
	g := Generator(a, s)
	t, _ := nearestDegree(a, 1200*math.Log2(1.25))
	c := 4*g - 2*n - t
	chroma := 2*t - g
	sw := make([]Swatch, n)
	meantone := mod(c, n) == 0
	if meantone {
		for d, k := range chainPositions(n, g) {
			switch {
			case k == noPos:
			case k >= -1 && k <= 5:
				sw[d] = Swatch{Green, ""}
			case k >= 6 && k <= 10:
				sw[d] = Swatch{White, ""}
			case k >= 11 && k <= 17:
				sw[d] = Swatch{Yellow, ""}
			case k >= -13 && k <= -7:
				sw[d] = Swatch{Blue, ""}
			}
		}
	} else {
		set := func(d int, col Color) {
			if sw[d].Color == Off {
				sw[d] = Swatch{col, ""}
			}
		}
		var naturals []int
		for k := -1; k <= 5; k++ {
			naturals = append(naturals, mod(k*g, n))
		}
		for _, d := range naturals {
			set(d, White)
			set(mod(d+c, n), White)
			set(mod(d-c, n), White)
		}
		for _, d := range naturals {
			set(mod(d+chroma, n), Green)
		}
		for _, d := range naturals {
			set(mod(d-chroma, n), Orange)
		}
	}
	sw[0] = Swatch{Magenta, "R"}
	return sw, c, chroma, meantone
}

// jiRatios returns the ratio of each degree as the ji scheme reads it: the
// written ratio when its prime limit is 3..limit (none when higher), else the
// first justRatios(limit, 400) entry that lands on the degree within tolerance.
func jiRatios(a *theory.Analysis, limit int) []*big.Rat {
	n := len(a.Degrees)
	tol := tolerance(a)
	out := make([]*big.Rat, n)
	decided := make([]bool, n)
	out[0], decided[0] = big.NewRat(1, 1), true
	for d := 1; d < n; d++ {
		if r := a.Degrees[d].Ratio; r != nil {
			decided[d] = true
			if pl := theory.PrimeLimit(r); pl >= 3 && pl <= limit {
				out[d] = r
			}
		}
	}
	for _, jr := range justRatios(limit, 400) {
		cents := 1200 * math.Log2(float64(jr.n)/float64(jr.d))
		if cents >= a.PeriodCents {
			continue
		}
		d, err := nearestDegree(a, cents)
		if d > 0 && math.Abs(err) <= tol && !decided[d] {
			out[d], decided[d] = big.NewRat(jr.n, jr.d), true
		}
	}
	return out
}

var kiteSwatches = map[int][2]Swatch{ // prime: over, under
	5: {{Yellow, "yo"}, {Green, "gu"}}, 7: {{Blue, "zo"}, {Red, "ru"}},
	11: {{Lime, "ilo"}, {Orange, "lu"}}, 13: {{Pink, "tho"}, {Cyan, "thu"}},
}

// Kite colours degrees by Kite Giedraitis's colour notation: the highest prime
// above 3 in the ratio, over (numerator) or under (denominator); 3-limit is wa.
func Kite(a *theory.Analysis, s Settings) []Swatch {
	rs := jiRatios(a, s.Limit)
	sw := make([]Swatch, len(rs))
	for d, r := range rs {
		if d == 0 || r == nil {
			continue
		}
		num := theory.PrimeLimit(new(big.Rat).SetInt(r.Num()))
		den := theory.PrimeLimit(new(big.Rat).SetInt(r.Denom()))
		p := max(num, den)
		if p < 5 {
			sw[d] = Swatch{White, "wa"}
		} else if pair, ok := kiteSwatches[p]; ok {
			if num > den {
				sw[d] = pair[0]
			} else {
				sw[d] = pair[1]
			}
		}
	}
	sw[0] = Swatch{Magenta, "R"}
	return sw
}

var factorColors = map[string]Color{"3": Red, "5": Green, "7": Blue, "35": Yellow, "57": Cyan, "37": Magenta, "357": White}

// Factors mixes the LED channels like the primes 3 (red), 5 (green) and 7
// (blue) in each degree's ratio. The root is unlit (magenta means 3·7 here).
func Factors(a *theory.Analysis, s Settings) []Swatch {
	rs := jiRatios(a, s.Limit)
	sw := make([]Swatch, len(rs))
	for d, r := range rs {
		if d == 0 || r == nil {
			continue
		}
		label := ""
		for _, p := range []int64{3, 5, 7} {
			bp := big.NewInt(p)
			if new(big.Int).Mod(r.Num(), bp).Sign() == 0 || new(big.Int).Mod(r.Denom(), bp).Sign() == 0 {
				label += strconv.FormatInt(p, 10)
			}
		}
		if c, ok := factorColors[label]; ok {
			sw[d] = Swatch{c, label}
		}
	}
	sw[0] = Swatch{Off, "R"}
	return sw
}

var stepMiddle = []Color{Green, Cyan, Yellow, Orange, Lime, Pink}

func stepSwatch(c, k int) Swatch {
	var col Color
	switch {
	case c == 0:
		col = White
	case c == k-1:
		col = Blue
	default:
		col = stepMiddle[(c-1)%len(stepMiddle)]
	}
	var label string
	switch k {
	case 1:
		label = "L"
	case 2:
		label = []string{"L", "s"}[c]
	case 3:
		label = []string{"L", "M", "s"}[c]
	default:
		label = strconv.Itoa(c + 1)
	}
	return Swatch{col, label}
}

func stepsLegend(k int) string {
	var parts []string
	for c := range k {
		s := stepSwatch(c, k)
		parts = append(parts, s.Label+" "+s.Color.String())
	}
	return strings.Join(parts, "  ")
}

// Steps colours each degree by the size class of the step that starts on it.
// It returns the number of classes.
func Steps(a *theory.Analysis) ([]Swatch, int) {
	cls, k := theory.StepClasses(a.Structure.Steps, 0.5)
	sw := make([]Swatch, len(cls))
	for d, c := range cls {
		sw[d] = stepSwatch(c, k)
	}
	sw[0] = Swatch{Magenta, "R"}
	return sw, k
}

var layerColors = []Color{White, Yellow, Blue, Green}

// Nested lights the MOS scales along the generator as nested layers: the
// sizes 5..n-1 whose chain (from one generator below the root) is a MOS,
// smallest first, up to four. It returns the layer sizes.
func Nested(a *theory.Analysis, s Settings) ([]Swatch, []int) {
	n := len(a.Degrees)
	g := Generator(a, s)
	var layers []int
	for m := 5; g > 0 && m < n && len(layers) < 4; m++ {
		if isMOS(MOSChoice{Generator: g, Size: m, Down: 1}.Degrees(n), n) {
			layers = append(layers, m)
		}
	}
	sw := make([]Swatch, n)
	for i := len(layers) - 1; i >= 0; i-- { // smaller layers overwrite: the first layer wins
		for _, d := range (MOSChoice{Generator: g, Size: layers[i], Down: 1}).Degrees(n) {
			sw[d] = Swatch{layerColors[i], strconv.Itoa(i + 1)}
		}
	}
	sw[0] = Swatch{Magenta, "R"}
	return sw, layers
}

// Consonance colours degrees in bands by the odd limit of their ratio (written,
// or the nearest simple ratio within tolerance) and labels them with it.
func Consonance(a *theory.Analysis) []Swatch {
	n := len(a.Degrees)
	tol := tolerance(a)
	sw := make([]Swatch, n)
	for d := 1; d < n; d++ {
		r := a.Degrees[d].Ratio
		if r == nil {
			if nr, _, ok := theory.NearestRatio(a.Degrees[d].Cents, tol); ok {
				r = nr
			}
		}
		if r == nil {
			continue
		}
		col := Cyan
		switch ol := theory.OddLimit(r); {
		case ol == 0: // too large to factor: highest band
		case ol <= 5:
			col = White
		case ol <= 9:
			col = Green
		case ol <= 15:
			col = Blue
		}
		sw[d] = Swatch{col, r.Num().String() + "/" + r.Denom().String()}
	}
	sw[0] = Swatch{Magenta, "R"}
	return sw
}

func harmonicRange(s Settings) (int, int) {
	if s.Harmonics == 32 {
		return 16, 32
	}
	return 1, 16
}

// harmonicSwatch colours a degree hit by harmonic h and/or subharmonic u (0 = none).
func harmonicSwatch(h, u int) Swatch {
	switch {
	case h > 0 && u > 0:
		return Swatch{White, strconv.Itoa(h)}
	case h > 0:
		return Swatch{Yellow, strconv.Itoa(h)}
	case u > 0:
		return Swatch{Blue, "u" + strconv.Itoa(u)}
	}
	return Swatch{}
}

// Harmonics lights the degrees that the root's harmonic (and subharmonic)
// series land on within tolerance, reduced into the period.
func Harmonics(a *theory.Analysis, s Settings) []Swatch {
	n := len(a.Degrees)
	period := a.PeriodCents
	tol := tolerance(a)
	place := func(c float64) int {
		c = math.Mod(c, period)
		if c < 0 {
			c += period
		}
		d, err := nearestDegree(a, c)
		if e := period - c; math.Abs(e) < math.Abs(err) { // the period is degree 0 again
			d, err = 0, e
		}
		if math.Abs(err) <= tol {
			return d
		}
		return -1
	}
	harm, sub := make([]int, n), make([]int, n)
	lo, hi := harmonicRange(s)
	for h := lo; h <= hi; h++ {
		c := 1200 * math.Log2(float64(h))
		if d := place(c); d >= 0 && harm[d] == 0 {
			harm[d] = h
		}
		if s.Subharmonics {
			if d := place(-c); d >= 0 && sub[d] == 0 {
				sub[d] = h
			}
		}
	}
	sw := make([]Swatch, n)
	for d := range sw {
		sw[d] = harmonicSwatch(harm[d], sub[d])
	}
	sw[0] = Swatch{Magenta, "R"}
	return sw
}
