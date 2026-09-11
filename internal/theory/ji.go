package theory

import (
	"math"
	"math/big"
	"sync"
)

// JIDegree is the just-intonation reading of one degree (or of the period, index n).
type JIDegree struct {
	Index      int
	Ratio      *big.Rat // exact ratio from the file, or the nearest simple ratio; nil if none
	Exact      bool     // Ratio was written in the file
	Error      float64  // cents, ratio minus actual pitch (0 when exact)
	PrimeLimit int      // 0 when unknown
	OddLimit   int64    // 0 when unknown
}

// JISummary is the just-intonation analysis of a scale.
type JISummary struct {
	Degrees    []JIDegree // degrees 0..n-1, then the period at index n
	Tol        float64    // cents used for nearest-ratio matches
	AllExact   bool       // every pitch in the file is a ratio
	PrimeLimit int        // over the exact ratios (0 if none)
	OddLimit   int64      // over the exact ratios (0 if none)
}

func analyzeJI(a *Analysis, opt Options) JISummary {
	sum := JISummary{Tol: opt.JITol, AllExact: true}
	cands := candidates(math.Max(a.PeriodCents, 0)+opt.JITol, opt.JIMaxPrime, opt.JIMaxOdd)
	read := func(index int, cents float64, r *big.Rat) JIDegree {
		d := JIDegree{Index: index}
		if r != nil {
			d.Ratio, d.Exact = r, true
			d.PrimeLimit, d.OddLimit = PrimeLimit(r), OddLimit(r)
			if d.PrimeLimit > sum.PrimeLimit {
				sum.PrimeLimit = d.PrimeLimit
			}
			if d.OddLimit > sum.OddLimit {
				sum.OddLimit = d.OddLimit
			}
			return d
		}
		sum.AllExact = false
		if c, ok := nearest(cands, cents, opt.JITol); ok {
			d.Ratio = big.NewRat(c.n, c.d)
			d.Error = c.cents - cents
			d.PrimeLimit, d.OddLimit = primeLimit64(c.n, c.d), oddLimit64(c.n, c.d)
		}
		return d
	}
	for _, deg := range a.Degrees {
		sum.Degrees = append(sum.Degrees, read(deg.Index, deg.Cents, deg.Ratio))
	}
	sum.Degrees = append(sum.Degrees, read(len(a.Degrees), a.PeriodCents, a.PeriodRatio))
	return sum
}

// PrimeLimit returns the largest prime factor of the ratio's numerator and denominator,
// or 0 if they do not fit in 64 bits.
func PrimeLimit(r *big.Rat) int {
	if !r.Num().IsInt64() || !r.Denom().IsInt64() {
		return 0
	}
	return primeLimit64(r.Num().Int64(), r.Denom().Int64())
}

// OddLimit returns the larger odd part of the ratio's numerator and denominator,
// or 0 if they do not fit in 64 bits.
func OddLimit(r *big.Rat) int64 {
	if !r.Num().IsInt64() || !r.Denom().IsInt64() {
		return 0
	}
	return oddLimit64(r.Num().Int64(), r.Denom().Int64())
}

func primeLimit64(n, d int64) int {
	return int(max(largestPrime(n), largestPrime(d)))
}

func largestPrime(x int64) int64 {
	lp := int64(1)
	for p := int64(2); p*p <= x; p++ {
		for x%p == 0 {
			lp, x = p, x/p
		}
	}
	if x > 1 {
		lp = x
	}
	return lp
}

func oddLimit64(n, d int64) int64 {
	odd := func(x int64) int64 {
		for x > 0 && x%2 == 0 {
			x /= 2
		}
		return x
	}
	return max(odd(n), odd(d))
}

type candidate struct {
	n, d   int64
	cents  float64
	height int64 // n*d, the Tenney height before the log
}

// nearest returns the simplest candidate within tol cents of the pitch.
func nearest(cands []candidate, cents, tol float64) (candidate, bool) {
	var best candidate
	found := false
	for _, c := range cands {
		e := math.Abs(c.cents - cents)
		if e > tol {
			continue
		}
		if !found || c.height < best.height || (c.height == best.height && e < math.Abs(best.cents-cents)) {
			best, found = c, true
		}
	}
	return best, found
}

var (
	candMu    sync.Mutex
	candCache = map[[3]int64][]candidate{}
)

// candidates returns ratios n/d >= 1 with d <= 64, below maxCents, within the limits.
// Results are cached per octave-rounded range.
func candidates(maxCents float64, maxPrime int, maxOdd int64) []candidate {
	octaves := min(int64(math.Ceil(maxCents/1200))+1, 4) // ratios up to 16/1
	key := [3]int64{octaves, int64(maxPrime), maxOdd}
	candMu.Lock()
	defer candMu.Unlock()
	if c, ok := candCache[key]; ok {
		return c
	}
	var out []candidate
	for d := int64(1); d <= 64; d++ {
		for n := d; n < d<<octaves; n++ {
			if gcd64(n, d) != 1 || primeLimit64(n, d) > maxPrime || oddLimit64(n, d) > maxOdd {
				continue
			}
			out = append(out, candidate{n, d, 1200 * math.Log2(float64(n)/float64(d)), n * d})
		}
	}
	candCache[key] = out
	return out
}

func gcd64(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
