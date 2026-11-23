package assign_test

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"math/bits"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/paulourio/goblin/assign"
)

func twoSumExact(a, b float64) (s, err float64) {
	a = float64(a)
	b = float64(b)
	s = float64(a + b)
	bb := float64(s - a)
	err = float64(a-float64(s-bb)) + float64(b-bb)
	return s, err
}

func exactBigRatSum(vals []float64) *big.Rat {
	sum := new(big.Rat)
	term := new(big.Rat)
	for _, v := range vals {
		if term.SetFloat64(v) == nil {
			panic(fmt.Sprintf("exactBigRatSum: non-finite float64 %v", v))
		}
		sum.Add(sum, term)
	}
	return sum
}

func exactMatchedObjRat(rows, cols int, cost []float64, rowInd, colInd []int) (*big.Rat, bool) {
	sum := new(big.Rat)
	term := new(big.Rat)
	for k := range rowInd {
		r, c := rowInd[k], colInd[k]
		if r < 0 || r >= rows || c < 0 || c >= cols {
			return nil, false
		}
		v := cost[r*cols+c]
		if math.IsInf(v, 0) || math.IsNaN(v) || term.SetFloat64(v) == nil {
			return nil, false
		}
		sum.Add(sum, term)
	}
	return sum, true
}

func bigRatCheckGeSum(c, u, v float64, strict bool) bool {
	rc := new(big.Rat).SetFloat64(c)
	ru := new(big.Rat).SetFloat64(u)
	rv := new(big.Rat).SetFloat64(v)
	if rc == nil || ru == nil || rv == nil {
		return false
	}
	ru.Add(ru, rv)
	cmp := rc.Cmp(ru)
	if strict {
		return cmp > 0
	}
	return cmp >= 0
}

// roundUpFloatGeSum returns a finite float64 >= (u + v) + delta (strictly > u + v when
// strict is true) verified in exact rational arithmetic, or +Inf if overflow occurs.
func roundUpFloatGeSum(u, v, delta float64, strict bool) float64 {
	s, e := twoSumExact(u, v)
	if math.IsInf(s, 1) {
		return math.Inf(1)
	}
	if math.IsInf(s, -1) {
		c := -math.MaxFloat64
		if delta > 0 {
			c = delta
		}
		if bigRatCheckGeSum(c, u, v, strict) {
			return c
		}
		return math.Inf(1)
	}
	if delta == 0 && e == 0 {
		if !strict {
			return s
		}
		nxt := math.Nextafter(s, math.Inf(1))
		if math.IsInf(nxt, 1) {
			return math.Inf(1)
		}
		return nxt
	}

	c := (s + e) + delta
	if math.IsNaN(c) {
		c = s + delta
	}
	if math.IsInf(c, 1) {
		return math.Inf(1)
	}
	if math.IsInf(c, -1) {
		c = -math.MaxFloat64
	}
	// Fast path via Sterbenz exact difference when c is within factor 2 of s.
	if d, f := twoSumExact(c, -s); f == 0 {
		if (strict && d > e) || (!strict && d >= e) {
			return c
		}
	}
	for range 8 {
		if bigRatCheckGeSum(c, u, v, strict) {
			return c
		}
		nxt := math.Nextafter(c, math.Inf(1))
		if math.IsInf(nxt, 1) {
			return math.Inf(1)
		}
		c = nxt
	}
	// Fallback when double-rounding of large delta missed exact sum: build directly via big.Rat.
	ru := new(big.Rat).SetFloat64(u)
	rv := new(big.Rat).SetFloat64(v)
	rd := new(big.Rat).SetFloat64(delta)
	target := new(big.Rat).Add(ru, rv)
	target.Add(target, rd)
	f, _ := target.Float64()
	if math.IsInf(f, 1) {
		return math.Inf(1)
	}
	for range 4 {
		if bigRatCheckGeSum(f, u, v, strict) {
			return f
		}
		f = math.Nextafter(f, math.Inf(1))
		if math.IsInf(f, 1) {
			return math.Inf(1)
		}
	}
	return math.Inf(1)
}

// buildPlantedPotentialMatrix constructs a rows x cols cost matrix from exact dual
// potentials uStar[0..m-1], vStar[0..n-1] (m=min(rows,cols)), injection pi[0..m-1],
// non-negative slacks[0..m*n-1] (with +Inf for forbidden edges), and an optional strict
// positive slack flag. On every matched edge, C equals uStar[i] + vStar[pi[i]] exactly
// in Q; on every off-diagonal edge, C >= uStar[i] + vStar[j] is exact in Q.
func buildPlantedPotentialMatrix(
	rows, cols int,
	sense assign.Sense,
	uStar, vStar []float64,
	pi []int,
	slacks []float64,
	strictOff bool,
) (cost []float64, plantedRow, plantedCol []int) {
	m := min(rows, cols)
	n := max(rows, cols)
	transposed := rows > cols

	minCost := make([]float64, m*n)
	for i := range m {
		ui := uStar[i]
		matchJ := pi[i]
		for j := range n {
			idx := i*n + j
			if j == matchJ {
				s, e := twoSumExact(ui, vStar[j])
				if e != 0 || math.IsInf(s, 0) || math.IsNaN(s) {
					panic(fmt.Sprintf("planted matched edge (%d,%d) not exact in Q: u=%g v=%g e=%g", i, j, ui, vStar[j], e))
				}
				minCost[idx] = s
				continue
			}
			sl := slacks[idx]
			if math.IsInf(sl, 1) {
				minCost[idx] = math.Inf(1)
				continue
			}
			minCost[idx] = roundUpFloatGeSum(ui, vStar[j], sl, strictOff)
		}
	}

	cost = make([]float64, rows*cols)
	if !transposed {
		for idx, v := range minCost {
			if sense == assign.Maximize {
				cost[idx] = -v
			} else {
				cost[idx] = v
			}
		}
		plantedRow = make([]int, m)
		for i := range m {
			plantedRow[i] = i
		}
		plantedCol = slices.Clone(pi)
		return cost, plantedRow, plantedCol
	}

	// Transpose m x n into rows x cols (rows > cols) and sort planted pairs by ascending row.
	for i := range m {
		for j := range n {
			v := minCost[i*n+j]
			if sense == assign.Maximize {
				v = -v
			}
			cost[j*cols+i] = v
		}
	}
	type pair struct{ r, c int }
	pairs := make([]pair, m)
	for i := range m {
		pairs[i] = pair{r: pi[i], c: i}
	}
	slices.SortFunc(pairs, func(a, b pair) int { return a.r - b.r })
	plantedRow = make([]int, m)
	plantedCol = make([]int, m)
	for k, p := range pairs {
		plantedRow[k] = p.r
		plantedCol[k] = p.c
	}
	return cost, plantedRow, plantedCol
}

func verifyPlantedBigRat(
	tb testing.TB,
	label string,
	rows, cols int,
	cost []float64,
	sense assign.Sense,
	res assign.Result,
	plantedRow, plantedCol []int,
	uniquePlanted, requireBitExactObj bool,
) {
	tb.Helper()
	k := min(rows, cols)
	if len(res.RowInd) != k || len(res.ColInd) != k {
		tb.Fatalf("[%s] cardinality got (%d,%d), want %d", label, len(res.RowInd), len(res.ColInd), k)
	}
	if len(res.U) != rows || len(res.V) != cols {
		tb.Fatalf("[%s] dual lens got (%d,%d), want (%d,%d)", label, len(res.U), len(res.V), rows, cols)
	}

	usedRow := make([]bool, rows)
	usedCol := make([]bool, cols)
	prevRow := -1
	matchedMinVals := make([]float64, 0, k)
	maxMatchedAbs := 0.0
	for idx := range k {
		r, c := res.RowInd[idx], res.ColInd[idx]
		if r < 0 || r >= rows || c < 0 || c >= cols {
			tb.Fatalf("[%s] out-of-range (%d,%d) for %dx%d", label, r, c, rows, cols)
		}
		if r <= prevRow {
			tb.Fatalf("[%s] RowInd not strictly ascending at %d: %v", label, idx, res.RowInd)
		}
		prevRow = r
		if usedRow[r] || usedCol[c] {
			tb.Fatalf("[%s] duplicate row/col (%d,%d)", label, r, c)
		}
		usedRow[r] = true
		usedCol[c] = true
		v := cost[r*cols+c]
		if math.IsInf(v, 0) || math.IsNaN(v) {
			tb.Fatalf("[%s] matched forbidden/invalid (%d,%d)=%v", label, r, c, v)
		}
		if a := math.Abs(v); a > maxMatchedAbs {
			maxMatchedAbs = a
		}
		if sense == assign.Maximize {
			matchedMinVals = append(matchedMinVals, -v)
		} else {
			matchedMinVals = append(matchedMinVals, v)
		}
	}

	gotRat, okGot := exactMatchedObjRat(rows, cols, cost, res.RowInd, res.ColInd)
	plantedRat, okPlanted := exactMatchedObjRat(rows, cols, cost, plantedRow, plantedCol)
	if !okGot || !okPlanted {
		tb.Fatalf("[%s] failed to rationalize matched objective", label)
	}
	if gotRat.Cmp(plantedRat) != 0 {
		gf, _ := gotRat.Float64()
		pf, _ := plantedRat.Float64()
		tb.Fatalf("[%s] suboptimal matching: gotRat=%s (~%.17g) != plantedRat=%s (~%.17g)",
			label, gotRat.RatString(), gf, plantedRat.RatString(), pf)
	}
	if uniquePlanted && (!slices.Equal(res.RowInd, plantedRow) || !slices.Equal(res.ColInd, plantedCol)) {
		tb.Fatalf("[%s] unique matching mismatch: got (%v,%v), want (%v,%v)",
			label, res.RowInd, res.ColInd, plantedRow, plantedCol)
	}

	wantF, _ := gotRat.Float64()
	if gotRat.Sign() == 0 {
		if math.Float64bits(res.Objective) != 0 {
			tb.Fatalf("[%s] zero objective must be +0.0, got bits=%016x (%g)",
				label, math.Float64bits(res.Objective), res.Objective)
		}
	} else if requireBitExactObj {
		if math.Float64bits(res.Objective) != math.Float64bits(wantF) {
			tb.Fatalf("[%s] bit-exact objective mismatch: got %.17g (%016x), want %.17g (%016x), rat=%s",
				label, res.Objective, math.Float64bits(res.Objective), wantF, math.Float64bits(wantF), gotRat.RatString())
		}
	} else {
		if math.IsInf(wantF, 0) {
			if res.Objective != wantF {
				tb.Fatalf("[%s] overflow objective mismatch: got %v, want %v", label, res.Objective, wantF)
			}
		} else if math.IsInf(res.Objective, 0) || math.IsNaN(res.Objective) {
			tb.Fatalf("[%s] unexpected non-finite objective %v (want %.17g)", label, res.Objective, wantF)
		} else {
			diff := math.Abs(res.Objective - wantF)
			ulpTol := max(4.0*math.SmallestNonzeroFloat64, 8.0*0x1p-53*max(math.Abs(wantF), maxMatchedAbs))
			if diff > ulpTol {
				tb.Fatalf("[%s] objective %.17g differs from exact %.17g by %g > tol %g",
					label, res.Objective, wantF, diff, ulpTol)
			}
		}
	}

	// Verify finiteness of duals and rectangular unassigned dual boundary conditions.
	maxDualAbs := 0.0
	for i, ui := range res.U {
		if math.IsNaN(ui) || math.IsInf(ui, 0) {
			tb.Fatalf("[%s] non-finite U[%d]=%v", label, i, ui)
		}
		if a := math.Abs(ui); a > maxDualAbs {
			maxDualAbs = a
		}
	}
	for j, vj := range res.V {
		if math.IsNaN(vj) || math.IsInf(vj, 0) {
			tb.Fatalf("[%s] non-finite V[%d]=%v", label, j, vj)
		}
		if a := math.Abs(vj); a > maxDualAbs {
			maxDualAbs = a
		}
	}
	zeroDualTol := 1e-11 * max(1.0, maxDualAbs) * float64(max(rows, cols))
	if rows < cols {
		for c := range cols {
			if res.V[c] > zeroDualTol {
				tb.Fatalf("[%s] wide V[%d]=%g > 0", label, c, res.V[c])
			}
			if !usedCol[c] && math.Abs(res.V[c]) > zeroDualTol {
				tb.Fatalf("[%s] wide unmatched V[%d]=%g != 0", label, c, res.V[c])
			}
		}
	} else if rows > cols {
		for r := range rows {
			if res.U[r] > zeroDualTol {
				tb.Fatalf("[%s] tall U[%d]=%g > 0", label, r, res.U[r])
			}
			if !usedRow[r] && math.Abs(res.U[r]) > zeroDualTol {
				tb.Fatalf("[%s] tall unmatched U[%d]=%g != 0", label, r, res.U[r])
			}
		}
	}

	if rows*cols <= 16384 {
		verifyDualFeasibilityRat(tb, label, rows, cols, cost, sense, res, matchedMinVals)
	}
}

func verifyDualFeasibilityRat(tb testing.TB, label string, rows, cols int, cost []float64, sense assign.Sense, res assign.Result, matchedMinVals []float64) {
	tb.Helper()
	dimScale := float64(max(rows, cols))
	sumMinRat := exactBigRatSum(matchedMinVals)
	dualSum := new(big.Rat)
	uRats := make([]*big.Rat, rows)
	vRats := make([]*big.Rat, cols)
	var sumAbsU, sumAbsV float64
	for i, u := range res.U {
		uRats[i] = new(big.Rat).SetFloat64(u)
		dualSum.Add(dualSum, uRats[i])
		sumAbsU += math.Abs(u)
	}
	for j, v := range res.V {
		vRats[j] = new(big.Rat).SetFloat64(v)
		dualSum.Add(dualSum, vRats[j])
		sumAbsV += math.Abs(v)
	}

	sumGap := new(big.Rat).Sub(sumMinRat, dualSum)
	sumGap.Abs(sumGap)
	gapTol := max(1e-316, 1e-9*dimScale*(sumAbsU+sumAbsV))
	if math.IsInf(gapTol, 0) {
		gapTol = math.MaxFloat64
	}
	gapF, _ := sumGap.Float64()
	if gapF > gapTol {
		tb.Fatalf("[%s] strong duality gap in Q: gap=%g > tol=%g", label, gapF, gapTol)
	}

	sumRat := new(big.Rat)
	cRat := new(big.Rat)
	excess := new(big.Rat)
	for r := range rows {
		ur := res.U[r]
		uRat := uRats[r]
		for c := range cols {
			normVal := cost[r*cols+c]
			if sense == assign.Maximize {
				normVal = -normVal
			}
			if math.IsInf(normVal, 1) {
				continue
			}
			sumRat.Add(uRat, vRats[c])
			cRat.SetFloat64(normVal)
			if sumRat.Cmp(cRat) <= 0 {
				continue
			}
			excess.Sub(sumRat, cRat)
			tol := max(1e-318, 1e-10*dimScale*(math.Abs(normVal)+math.Abs(ur)+math.Abs(res.V[c])))
			if math.IsInf(tol, 0) {
				tol = math.MaxFloat64
			}
			exF, _ := excess.Float64()
			if exF > tol {
				tb.Fatalf("[%s] dual violation at (%d,%d): u=%g v=%g c=%g excess=%g > tol=%g",
					label, r, c, ur, res.V[c], normVal, exF, tol)
			}
		}
	}
}

// =============================================================================
// 1. Planted-Potential Cancellation, Subnormals & Rectangular Stage-2 Fuzzer
// =============================================================================

func generatePlantedCancellationCase(rng *rand.Rand, rows, cols int, sense assign.Sense, regime uint8) (cost []float64, pRow, pCol []int, unique, bitExact bool) {
	m := min(rows, cols)
	n := max(rows, cols)
	pi := rng.Perm(n)[:m]
	uStar := make([]float64, m)
	vStar := make([]float64, n)
	slacks := make([]float64, m*n)
	for i := range slacks {
		slacks[i] = math.Inf(1)
	}

	tiny := math.SmallestNonzeroFloat64
	unique = true

	switch regime % 5 {
	case 0:
		// Opposing-giant Sterbenz cancellation (2^48..2^52) down to O(1) or 2^-20.
		exp := 48 + rng.IntN(5)
		giant := math.Ldexp(1.0, exp)
		for i := range m {
			uStar[i] = giant + float64(rng.IntN(16))
			if rng.IntN(3) == 0 {
				vStar[pi[i]] = -uStar[i] + float64(rng.IntN(5)-2)
			} else {
				vStar[pi[i]] = -uStar[i]
			}
		}
		// Connect dense or cyclic decoy edges with tiny positive slacks.
		for i := range m {
			for j := range n {
				if j == pi[i] {
					continue
				}
				if rng.Float64() < 0.7 {
					slacks[i*n+j] = math.Ldexp(float64(1+rng.IntN(32)), -12)
				}
			}
		}
		bitExact = true

	case 1:
		// Extreme downscale (1e295..2^1018) mixed with small O(1) entries.
		hi := math.Ldexp(1.0, 990+rng.IntN(28))
		for i := range m {
			switch rng.IntN(3) {
			case 0:
				uStar[i] = hi
				vStar[pi[i]] = -hi
			case 1:
				uStar[i] = -hi
				vStar[pi[i]] = hi
			default:
				uStar[i] = float64(rng.IntN(10))
				vStar[pi[i]] = -float64(rng.IntN(10))
			}
		}
		if m < n {
			// Rectangular requires vStar[j] <= 0.
			for i := range m {
				v := vStar[pi[i]]
				if v > 0 {
					uStar[i] += v
					vStar[pi[i]] = 0
				}
			}
		}
		for i := range m {
			for j := range n {
				if j == pi[i] {
					continue
				}
				switch rng.IntN(4) {
				case 0:
					slacks[i*n+j] = math.Ldexp(1.0, 980)
				case 1:
					slacks[i*n+j] = float64(1 + rng.IntN(50))
				case 2:
					slacks[i*n+j] = math.Ldexp(float64(1+rng.IntN(8)), -10)
				}
			}
		}
		bitExact = true

	case 2:
		// Deep-subnormal tie-breaking mixed with 2^1015 blockers (hasDeepSub).
		hi := math.Ldexp(1.0, 1015)
		for i := range m {
			switch i % 3 {
			case 0:
				uStar[i] = hi
				vStar[pi[i]] = -hi
			case 1:
				uStar[i] = 0
				vStar[pi[i]] = -tiny * float64(1+rng.IntN(32))
			default:
				uStar[i] = 0
				if m < n {
					vStar[pi[i]] = -tiny * float64(1+rng.IntN(16))
				} else {
					vStar[pi[i]] = tiny * float64(1+rng.IntN(32))
				}
			}
		}
		for i := range m {
			for j := range n {
				if j == pi[i] {
					continue
				}
				switch rng.IntN(4) {
				case 0:
					slacks[i*n+j] = tiny * float64(1+rng.IntN(64))
				case 1:
					slacks[i*n+j] = math.Ldexp(1.0, 970)
				case 2:
					slacks[i*n+j] = 1.0
				}
			}
		}
		bitExact = true

	case 3:
		// Rectangular Stage-2 cancellation (colBonus + virtual-root polishStage2Duals).
		hi := math.Ldexp(1.0, 1016)
		for i := range m {
			uStar[i] = hi
			vStar[pi[i]] = -hi
		}
		for i := range m {
			for j := range n {
				if j == pi[i] {
					continue
				}
				if rng.IntN(3) != 0 {
					slacks[i*n+j] = tiny * float64(1+rng.IntN(16))
				} else {
					slacks[i*n+j] = float64(1 + rng.IntN(20))
				}
			}
		}
		bitExact = true

	default:
		// Near-MaxFloat64 clamping (anyClamped) with opposing +-1e307 entries.
		hi := math.Ldexp(1.0, 1020) // ~8.98e306
		for i := range m {
			if i%2 == 0 {
				uStar[i] = hi
				vStar[pi[i]] = -hi
			} else if m == n {
				uStar[i] = -hi
				vStar[pi[i]] = hi
			} else {
				uStar[i] = hi
				vStar[pi[i]] = -hi + math.Ldexp(1.0, 970)
			}
		}
		for i := range m {
			for j := range n {
				if j == pi[i] {
					continue
				}
				if rng.IntN(2) == 0 {
					slacks[i*n+j] = math.Ldexp(float64(1+rng.IntN(7)), 972)
				} else {
					slacks[i*n+j] = math.Ldexp(1.0, 10)
				}
			}
		}
		bitExact = true
	}

	// Ensure off-diagonal edges have slack > 0 (uniquePlanted when strictOff=true).
	for i := range m {
		for j := range n {
			if j != pi[i] && math.IsInf(slacks[i*n+j], 1) {
				if rng.IntN(2) == 0 {
					slacks[i*n+j] = math.Ldexp(1.0, 10)
				}
			}
		}
	}
	cost, pRow, pCol = buildPlantedPotentialMatrix(rows, cols, sense, uStar, vStar, pi, slacks, true)
	return cost, pRow, pCol, unique, bitExact
}

// FuzzAssignPlantedCancellationAndStage2 fuzzes Stage-2 cancellation, downscaling,
// deep-subnormal recovery, rectangular colBonus/polishStage2Duals, and near-MaxFloat64
// clamping against exact math/big.Rat optimal objectives.
//
//	GOEXPERIMENT=simd go test -run=^$ -fuzz=^FuzzAssignPlantedCancellationAndStage2$ -fuzztime=15s ./assign
func FuzzAssignPlantedCancellationAndStage2(f *testing.F) {
	for seed := range uint64(15) {
		f.Add(seed, uint8(seed%5), uint8(2+seed%18), uint8(2+(seed*3)%22), uint8(seed%5))
	}

	f.Fuzz(func(t *testing.T, seed uint64, shapeByte, rByte, cByte, regimeByte uint8) {
		rng := rand.New(rand.NewPCG(seed^0x41c6ce57, seed^0x9e3779b97f4a7c15))
		var rows, cols int
		switch shapeByte % 4 {
		case 0:
			rows = 2 + int(rByte%47)
			cols = rows
		case 1:
			rows = 2 + int(rByte%28)
			cols = rows + 1 + int(cByte%24)
		case 2:
			cols = 2 + int(cByte%28)
			rows = cols + 1 + int(rByte%24)
		default:
			if raceEnabled && testing.Short() {
				rows = 16 + int(rByte%16)
				cols = 16 + int(cByte%20)
			} else {
				rows = 32 + int(rByte%64)
				cols = rows + int(cByte%33)
			}
		}
		sense := assign.Minimize
		if (seed>>4)&1 == 1 {
			sense = assign.Maximize
		}

		cost, pRow, pCol, unique, bitExact := generatePlantedCancellationCase(rng, rows, cols, sense, regimeByte)
		label := fmt.Sprintf("cancel_r%d_%dx%d_%s", regimeByte%5, rows, cols, sense)

		resSIMD, err := assign.LinearSumFlat(t.Context(), rows, cols, cost, sense, assign.WithSIMD(true))
		if err != nil {
			t.Fatalf("[%s/simd] unexpected error: %v", label, err)
		}
		verifyPlantedBigRat(t, label+"/simd", rows, cols, cost, sense, resSIMD, pRow, pCol, unique, bitExact)

		resScalar, err := assign.LinearSumFlat(t.Context(), rows, cols, cost, sense, assign.WithoutSIMD())
		if err != nil {
			t.Fatalf("[%s/scalar] unexpected error: %v", label, err)
		}
		verifyPlantedBigRat(t, label+"/scalar", rows, cols, cost, sense, resScalar, pRow, pCol, unique, bitExact)

		verifyFormalAssignResult(t, label+"/simd", rows, cols, cost, sense, resSIMD, false)
		verifyFormalAssignResult(t, label+"/scalar", rows, cols, cost, sense, resScalar, false)
	})
}

// =============================================================================
// 2. Sparse-Core Post-ARR Trap Fuzzer (N in [1000, 1550])
// =============================================================================

func makeHallInfeasibleGraph(rng *rand.Rand, rows, cols int, sense assign.Sense) []float64 {
	m := min(rows, cols)
	n := max(rows, cols)
	if m < 7 || n < 7 {
		m = max(m, 7)
		n = max(n, 7)
	}
	forbid := math.Inf(1)
	if sense == assign.Maximize {
		forbid = math.Inf(-1)
	}

	// Build an m x n Hall bottleneck where H_rows (size k >= 4) only connects to H_cols (size k-1 >= 3),
	// while every row and column still has degree >= 3.
	maxK := min(m-3, n-2)
	k := 4 + rng.IntN(max(1, min(maxK-3, max(1, m/8))))
	rowPerm := rng.Perm(m)
	colPerm := rng.Perm(n)
	hallRows := rowPerm[:k]
	restRows := rowPerm[k:]
	hallCols := colPerm[:k-1]
	restCols := colPerm[k-1:]

	mat := make([]float64, m*n)
	for i := range mat {
		mat[i] = forbid
	}
	randCost := func() float64 {
		return float64(1 + rng.IntN(500))
	}

	// Connect every Hall row to every Hall column (degree = k-1 >= 3, and every Hall col has degree >= k >= 4).
	for _, r := range hallRows {
		for _, c := range hallCols {
			mat[r*n+c] = randCost()
		}
	}
	// Connect every remaining row to >= 4 remaining cols, and every remaining col to >= 4 remaining rows.
	degR := min(4, len(restCols))
	for ri, r := range restRows {
		for off := range degR {
			c := restCols[(ri+off)%len(restCols)]
			mat[r*n+c] = randCost()
		}
	}
	degC := min(4, len(restRows))
	for ci, c := range restCols {
		for off := range degC {
			r := restRows[(ci+off)%len(restRows)]
			mat[r*n+c] = randCost()
		}
	}
	// Sprinkle extra non-Hall-breaking finite edges for realism.
	for _, r := range restRows {
		for range min(4, n) {
			c := rng.IntN(n)
			mat[r*n+c] = randCost()
		}
	}

	if rows <= cols {
		return mat
	}
	out := make([]float64, rows*cols)
	for i := range m {
		for j := range n {
			out[j*cols+i] = mat[i*n+j]
		}
	}
	return out
}

// generateSparseCoreTrap builds an n x n (n >= 1000) instance specifically engineered
// to leave >= 48 free rows after ARR and trigger priceAndRepairSparseCore, the
// hasDenseTiesOnFreeRows blindspot, low-degree sparse cores, or Hall infeasibility.
func generateSparseCoreTrap(rng *rand.Rand, n int, sense assign.Sense, regime uint8) (cost []float64, pRow, pCol []int, expectInfeas bool) {
	if regime%4 == 3 {
		return makeHallInfeasibleGraph(rng, n, n, sense), nil, nil, true
	}

	pi := rng.Perm(n)
	permRows := rng.Perm(n)
	numTrap := 70 + rng.IntN(40)
	numShadow := numTrap
	numDecoy := 36
	if numTrap+numShadow+numDecoy+8 > n {
		numTrap = 56
		numShadow = 56
	}

	trapRows := permRows[:numTrap]
	shadowRows := permRows[numTrap : numTrap+numShadow]
	otherRows := permRows[numTrap+numShadow:]

	isTrap := make([]bool, n)
	for _, r := range trapRows {
		isTrap[r] = true
	}
	isShadow := make([]bool, n)
	for _, r := range shadowRows {
		isShadow[r] = true
	}

	decoyCols := make([]int, numDecoy)
	for idx := range numDecoy {
		decoyCols[idx] = pi[otherRows[idx]]
	}

	uStar := make([]float64, n)
	vStar := make([]float64, n)
	for _, r := range trapRows {
		uStar[r] = 0.0
		vStar[pi[r]] = 500.0
	}
	for _, r := range shadowRows {
		uStar[r] = -500.0
		vStar[pi[r]] = 0.0
	}

	// Build row-by-row cost directly using exact integers so planted complementary slackness holds.
	cost = make([]float64, n*n)
	inf := math.Inf(1)
	mode := regime % 4
	baseOff := 0.0

	if mode == 2 {
		// Low-degree sparse matrix (< 24 finite edges per row) forcing sink == -1 repair.
		for i := range n {
			row := cost[i*n : (i+1)*n]
			for j := range n {
				row[j] = inf
			}
			row[pi[i]] = uStar[i] + vStar[pi[i]]
			deg := 3 + rng.IntN(16)
			for range deg {
				j := rng.IntN(n)
				if j != pi[i] {
					row[j] = uStar[i] + vStar[j] + float64(5+rng.IntN(400))
				}
			}
		}
		for idx, r := range trapRows {
			s := shadowRows[idx]
			cost[s*n+pi[r]] = 1.0 // -500 + 500 + 1
			for d := range 4 {
				dc := decoyCols[d]
				cost[r*n+dc] = float64(2 + d)
			}
		}
	} else {
		for i := range n {
			row := cost[i*n : (i+1)*n]
			ui := uStar[i]
			for j := range n {
				if j == pi[i] {
					row[j] = ui + vStar[j]
				} else {
					row[j] = ui + vStar[j] + float64(150+rng.IntN(200))
				}
			}
		}
		// Shadow rows drag v[pi[trap]] down to ~1 during Column Reduction.
		for idx, r := range trapRows {
			s := shadowRows[idx]
			cost[s*n+pi[r]] = 1.0
		}
		// Give trap rows 36 cheap decoy columns (< 499) so pi[r] sits outside top-24 core.
		for ti, r := range trapRows {
			for dIdx, dc := range decoyCols {
				var c float64
				switch {
				case dIdx < 2:
					c = 1.0
				case mode == 1 && ti >= 6:
					// Blindspot regime: rows 0..5 distinct, rows >= 6 tied on top 34 decoys.
					c = 2.0
				default:
					c = float64(2 + dIdx)
				}
				cost[r*n+dc] = c
			}
		}
		// Other rows own decoy columns at 0 with second-best 1 so ARR cannot clear trap rows.
		for _, r := range otherRows {
			cost[r*n+decoyCols[0]] = min(cost[r*n+decoyCols[0]], 1.0)
			cost[r*n+decoyCols[1]] = min(cost[r*n+decoyCols[1]], 1.0)
		}
	}
	_ = baseOff

	if sense == assign.Maximize {
		for i, v := range cost {
			cost[i] = -v
		}
	}
	pRow = make([]int, n)
	for i := range n {
		pRow[i] = i
	}
	return cost, pRow, pi, false
}

// FuzzAssignSparseCoreTraps fuzzes sparse-core post-ARR (n >= 1000), exploiting the
// SIMD-vs-Scalar threshold split in [1000, 1499] and testing rank->24 edge recovery,
// dense-tie blindspots, low-degree repair, and Hall-infeasible 1000x1000 graphs.
//
//	GOEXPERIMENT=simd go test -run=^$ -fuzz=^FuzzAssignSparseCoreTraps$ -fuzztime=20s ./assign
func FuzzAssignSparseCoreTraps(f *testing.F) {
	dims := []int{1000, 1003, 1200, 1499, 1500, 1507}
	for seed := range uint64(8) {
		f.Add(seed, uint8(seed%uint64(len(dims))), uint8(seed%4))
	}

	f.Fuzz(func(t *testing.T, seed uint64, dimByte, regimeByte uint8) {
		if raceEnabled && testing.Short() {
			t.Skip("skipping N>=1000 sparse-core fuzz under -race -short")
		}
		dims := []int{1000, 1003, 1024, 1200, 1499, 1500, 1507}
		n := dims[int(dimByte)%len(dims)]
		rng := rand.New(rand.NewPCG(seed^0x7f4a7c159e3779b9, seed^0x517cc1b727220a95))
		sense := assign.Minimize
		if (seed>>3)&1 == 1 {
			sense = assign.Maximize
		}

		cost, pRow, pCol, expectInfeas := generateSparseCoreTrap(rng, n, sense, regimeByte)
		label := fmt.Sprintf("sparse_r%d_n%d_%s", regimeByte%4, n, sense)

		resSIMD, errSIMD := assign.LinearSumFlat(t.Context(), n, n, cost, sense, assign.WithSIMD(true))
		resScalar, errScalar := assign.LinearSumFlat(t.Context(), n, n, cost, sense, assign.WithoutSIMD())

		if expectInfeas {
			if !errors.Is(errSIMD, assign.ErrInfeasible) || !errors.Is(errScalar, assign.ErrInfeasible) {
				t.Fatalf("[%s] expected ErrInfeasible on both paths, got simd=%v scalar=%v", label, errSIMD, errScalar)
			}
			verifyFormalAssignInfeasible(t, label, n, n, cost, sense)
			return
		}
		if errSIMD != nil || errScalar != nil {
			t.Fatalf("[%s] unexpected error: simd=%v scalar=%v", label, errSIMD, errScalar)
		}
		verifyPlantedBigRat(t, label+"/simd", n, n, cost, sense, resSIMD, pRow, pCol, false, false)
		verifyPlantedBigRat(t, label+"/scalar", n, n, cost, sense, resScalar, pRow, pCol, false, false)
		verifyFormalAssignResult(t, label+"/simd", n, n, cost, sense, resSIMD, false)
		verifyFormalAssignResult(t, label+"/scalar", n, n, cost, sense, resScalar, false)
	})
}

// =============================================================================
// 3. Collinear / Monge Spoofing & Row-Dominated Fuzzer
// =============================================================================

func generateCollinearOrRowDomMatrix(rng *rand.Rand, n int, sense assign.Sense, regime uint8) (cost []float64, intCost []int64, checkInt bool) {
	cost = make([]float64, n*n)
	inf := math.Inf(1)
	perm := rng.Perm(n)
	mode := regime % 4

	switch mode {
	case 0:
		// Spoof isCollinearSquare: 1-2 dominant rows, 4 strided collinear sample rows,
		// non-Monge spikes and +Inf holes on unsampled positions.
		rowW := make([]float64, n)
		colW := make([]float64, n)
		for i := range n {
			rowW[i] = float64(1 + rng.IntN(20))
			colW[i] = float64(i * 3)
		}
		for i := range n {
			for j := range n {
				cost[i*n+j] = rowW[i] * colW[j]
			}
		}
		// Make 1-2 dominating rows way cheaper so col reduction leaves >= n-2 rows free.
		domRows := 1 + rng.IntN(2)
		for r := range domRows {
			for j := range n {
				cost[r*n+j] = -5000.0 + float64(j)
			}
		}
		// Inject non-Monge spikes and +Inf holes off the sampled rows (0, n/3, 2n/3, n-1)
		// and off the guaranteed feasible permutation perm[i].
		step := max(1, n/32)
		for i := domRows; i < n; i++ {
			for j := range n {
				if j == perm[i] || j%step == 0 {
					continue
				}
				roll := rng.IntN(16)
				if roll == 0 {
					cost[i*n+j] = inf
				} else if roll <= 3 {
					cost[i*n+j] = float64(rng.IntN(6000) - 3000)
				}
			}
		}

	case 1:
		// Sign-mixed rank-1 / Monge matrix with small perturbations and +Inf holes.
		a := make([]float64, n)
		b := make([]float64, n)
		for i := range n {
			a[i] = float64(rng.IntN(101) - 50)
			b[i] = float64(rng.IntN(101) - 50)
		}
		for i := range n {
			for j := range n {
				if j != perm[i] && rng.IntN(12) == 0 {
					cost[i*n+j] = inf
				} else {
					cost[i*n+j] = a[i]*b[j] + float64(rng.IntN(7)-3)
				}
			}
		}

	case 2:
		// Non-collinear row-dominated multi-scale matrix (Branch B).
		numDom := 1 + rng.IntN(2)
		for i := range n {
			if i < numDom {
				for j := range n {
					cost[i*n+j] = -1e12 + float64(rng.IntN(1000))
				}
			} else {
				for j := range n {
					if j != perm[i] && rng.IntN(10) == 0 {
						cost[i*n+j] = inf
					} else {
						cost[i*n+j] = 1e12 + float64(rng.IntN(50000))
					}
				}
			}
		}

	default:
		// Exact integer rank-1 / row-dominated cross-checked against int64 Hungarian.
		checkInt = true
		intCost = make([]int64, n*n)
		dom := 1 + rng.IntN(2)
		a := make([]int64, n)
		b := make([]int64, n)
		for i := range n {
			a[i] = int64(1 + rng.IntN(40))
			b[i] = int64(1 + rng.IntN(40))
		}
		for i := range n {
			for j := range n {
				var v int64
				if i < dom {
					v = -100000 + int64(rng.IntN(500))
				} else {
					v = a[i]*b[j] + int64(rng.IntN(5)-2)
				}
				intCost[i*n+j] = v
				cost[i*n+j] = float64(v)
			}
		}
	}

	if sense == assign.Maximize && !checkInt {
		for i, v := range cost {
			cost[i] = -v
		}
	}
	return cost, intCost, checkInt
}

// FuzzAssignCollinearAndRowDominated fuzzes collinear/Monge detection, spoofing,
// sign-mixed rank-1 matrices, and multi-scale row-dominated matrices across SIMD and Scalar.
//
//	GOEXPERIMENT=simd go test -run=^$ -fuzz=^FuzzAssignCollinearAndRowDominated$ -fuzztime=15s ./assign
func FuzzAssignCollinearAndRowDominated(f *testing.F) {
	for seed := range uint64(12) {
		f.Add(seed, uint8(seed%4), uint8(16+seed*13))
	}

	f.Fuzz(func(t *testing.T, seed uint64, regimeByte, sizeByte uint8) {
		rng := rand.New(rand.NewPCG(seed^0x243f6a8885a308d3, seed^0x13198a2e03707344))
		mode := regimeByte % 4
		n := 8 + int(sizeByte%240)
		if mode == 3 {
			n = 8 + int(sizeByte%65)
		}
		if raceEnabled && testing.Short() && n > 100 {
			n = 16 + int(sizeByte%64)
		}
		sense := assign.Minimize
		if mode != 3 && (seed>>2)&1 == 1 {
			sense = assign.Maximize
		}

		cost, intCost, checkInt := generateCollinearOrRowDomMatrix(rng, n, sense, mode)
		label := fmt.Sprintf("col_r%d_n%d_%s", mode, n, sense)

		resSIMD, err := assign.LinearSumFlat(t.Context(), n, n, cost, sense, assign.WithSIMD(true))
		if err != nil {
			t.Fatalf("[%s/simd] unexpected error: %v", label, err)
		}
		resScalar, err := assign.LinearSumFlat(t.Context(), n, n, cost, sense, assign.WithoutSIMD())
		if err != nil {
			t.Fatalf("[%s/scalar] unexpected error: %v", label, err)
		}

		verifyOptimalityCertificate(t, label+"/simd", n, n, cost, sense, resSIMD)
		verifyOptimalityCertificate(t, label+"/scalar", n, n, cost, sense, resScalar)
		verifyFormalAssignResult(t, label+"/simd", n, n, cost, sense, resSIMD, checkInt)
		verifyFormalAssignResult(t, label+"/scalar", n, n, cost, sense, resScalar, checkInt)

		maxAbs := 1.0
		for _, v := range cost {
			if !math.IsInf(v, 0) && math.Abs(v) > maxAbs {
				maxAbs = math.Abs(v)
			}
		}
		tol := 1e-9 * max(maxAbs, math.Abs(resScalar.Objective)) * float64(n)
		if math.Abs(resSIMD.Objective-resScalar.Objective) > tol {
			t.Fatalf("[%s] SIMD=%.15g != Scalar=%.15g", label, resSIMD.Objective, resScalar.Objective)
		}

		if checkInt {
			wantInt, ok := exactInt64Hungarian(n, n, intCost, nil)
			if !ok {
				t.Fatalf("[%s] exactInt64Hungarian unexpected infeasible", label)
			}
			var gotInt int64
			for k, r := range resSIMD.RowInd {
				gotInt += intCost[r*n+resSIMD.ColInd[k]]
			}
			if gotInt != wantInt {
				t.Fatalf("[%s] integer objective mismatch: got %d, want %d", label, gotInt, wantInt)
			}
		}
	})
}

// =============================================================================
// 4. assignWideAcc Overflow & IEEE-754 Edge Fuzzer
// =============================================================================

func generateWideAccForcedCase(rng *rand.Rand, regime uint8, sense assign.Sense) (n int, cost []float64, pRow, pCol []int) {
	mode := regime % 4
	switch mode {
	case 0:
		// > 64 non-overlapping exponent bands plus cancelling +-2^600 giants.
		n = 72 + rng.IntN(48)
		vals := make([]float64, n)
		vals[0] = math.Ldexp(1.0, 600)
		vals[1] = -math.Ldexp(1.0, 600)
		for i := 2; i < n; i++ {
			exp := 960 - i*28
			if exp < -1040 {
				exp = -1040 + (i % 15)
			}
			m := float64(1 + rng.IntN(1023))
			if rng.IntN(2) == 1 {
				m = -m
			}
			vals[i] = math.Ldexp(m, exp)
		}
		rng.Shuffle(len(vals), func(i, j int) { vals[i], vals[j] = vals[j], vals[i] })
		return packForcedPermMatrix(rng, vals, sense)

	case 1:
		// Half-ULP sticky-bit traps across separated exponent tiers.
		switch rng.IntN(3) {
		case 0:
			sgn := 1.0
			if rng.IntN(2) == 1 {
				sgn = -1.0
			}
			vals := []float64{math.Ldexp(1.0, 53), 1.0, sgn * math.Ldexp(1.0, -1050)}
			return packForcedPermMatrix(rng, vals, sense)
		case 1:
			// Midpoint near MaxFloat64 (2^1023 + 2^970 - 2^969 + subnormal nudge).
			base := math.MaxFloat64 - math.Ldexp(1.0, 972)
			half := math.Ldexp(1.0, 970)
			tiny := math.Ldexp(1.0, -1050)
			if rng.IntN(2) == 1 {
				tiny = -tiny
			}
			vals := []float64{base, half, tiny, math.Ldexp(1.0, 500), -math.Ldexp(1.0, 500)}
			return packForcedPermMatrix(rng, vals, sense)
		default:
			expA := 20 + rng.IntN(100)
			ulpHalf := math.Ldexp(1.0, expA-53)
			sgn := 1.0
			if rng.IntN(2) == 1 {
				sgn = -1.0
			}
			vals := []float64{
				math.Ldexp(1.0, expA),
				ulpHalf,
				sgn * math.Ldexp(1.0, -1020),
				math.Ldexp(1.0, -1050),
			}
			return packForcedPermMatrix(rng, vals, sense)
		}

	case 2:
		// Signed zeros, hugeCostThreshold boundaries (1e289..1e291), and subnormals.
		n = 6 + rng.IntN(14)
		negZero := math.Copysign(0.0, -1)
		pool := []float64{
			0.0, negZero,
			1e289, -1e289,
			1e290, -1e290,
			math.Nextafter(1e290, math.Inf(1)), math.Nextafter(-1e290, math.Inf(-1)),
			1e291, -1e291,
			math.SmallestNonzeroFloat64, -math.SmallestNonzeroFloat64,
		}
		vals := make([]float64, n)
		for i := range n {
			vals[i] = pool[rng.IntN(len(pool))]
		}
		// Keep opposing huge terms balanced so sum stays finite or clean overflow.
		if n >= 2 && math.Abs(vals[0]) >= 1e290 {
			vals[1] = -vals[0]
		}
		return packForcedPermMatrix(rng, vals, sense)

	default:
		// Exact MaxFloat64 overflow saturation boundary.
		vals := []float64{
			math.MaxFloat64,
			math.Ldexp(1.0, 970),
			math.Ldexp(1.0, 969),
		}
		if rng.IntN(2) == 1 {
			vals = append(vals, -math.Ldexp(1.0, 970))
		}
		if rng.IntN(2) == 1 {
			for i := range vals {
				vals[i] = -vals[i]
			}
		}
		return packForcedPermMatrix(rng, vals, sense)
	}
}

func packForcedPermMatrix(rng *rand.Rand, vals []float64, sense assign.Sense) (n int, cost []float64, pRow, pCol []int) {
	n = len(vals)
	pi := rng.Perm(n)
	forbid := math.Inf(1)
	if sense == assign.Maximize {
		forbid = math.Inf(-1)
	}
	cost = make([]float64, n*n)
	for i := range cost {
		cost[i] = forbid
	}
	pRow = make([]int, n)
	for i := range n {
		pRow[i] = i
		cost[i*n+pi[i]] = vals[i]
	}
	return n, cost, pRow, pi
}

// FuzzAssignWideAccAndIEEE754Edges targets assignWideAcc buffer collapse, half-ULP sticky-bit
// rounding, signed zeros, hugeCostThreshold transitions, and MaxFloat64 overflow against
// bit-exact math/big.Rat evaluation.
//
//	GOEXPERIMENT=simd go test -run=^$ -fuzz=^FuzzAssignWideAccAndIEEE754Edges$ -fuzztime=15s ./assign
func FuzzAssignWideAccAndIEEE754Edges(f *testing.F) {
	for seed := range uint64(16) {
		f.Add(seed, uint8(seed%4))
	}

	f.Fuzz(func(t *testing.T, seed uint64, regimeByte uint8) {
		rng := rand.New(rand.NewPCG(seed^0x85ebca6b, seed^0xc2b2ae35))
		sense := assign.Minimize
		if (seed>>2)&1 == 1 {
			sense = assign.Maximize
		}

		n, cost, pRow, pCol := generateWideAccForcedCase(rng, regimeByte, sense)
		label := fmt.Sprintf("wide_r%d_n%d_%s", regimeByte%4, n, sense)

		resSIMD, err := assign.LinearSumFlat(t.Context(), n, n, cost, sense, assign.WithSIMD(true))
		if err != nil {
			t.Fatalf("[%s/simd] unexpected error: %v", label, err)
		}
		verifyPlantedBigRat(t, label+"/simd", n, n, cost, sense, resSIMD, pRow, pCol, true, true)

		resScalar, err := assign.LinearSumFlat(t.Context(), n, n, cost, sense, assign.WithoutSIMD())
		if err != nil {
			t.Fatalf("[%s/scalar] unexpected error: %v", label, err)
		}
		verifyPlantedBigRat(t, label+"/scalar", n, n, cost, sense, resScalar, pRow, pCol, true, true)
		verifyFormalAssignResult(t, label+"/simd", n, n, cost, sense, resSIMD, false)
		verifyFormalAssignResult(t, label+"/scalar", n, n, cost, sense, resScalar, false)
	})
}

// =============================================================================
// 5. Hall's-Theorem Knife-Edge & Topology Fuzzer
// =============================================================================

func generateHallTopologyCase(rng *rand.Rand, rows, cols int, sense assign.Sense, topo uint8) (cost []float64, pRow, pCol []int, expectInfeas bool) {
	mode := topo % 3
	forbid := math.Inf(1)
	if sense == assign.Maximize {
		forbid = math.Inf(-1)
	}

	if mode == 1 {
		return makeHallInfeasibleGraph(rng, rows, cols, sense), nil, nil, true
	}

	m := min(rows, cols)
	n := max(rows, cols)
	if mode == 0 {
		// Long alternating-chain feasible graph forcing repeated queue refills.
		rowOrder := rng.Perm(m)
		colOrder := rng.Perm(n)
		uStar := make([]float64, m)
		vStar := make([]float64, n)
		slacks := make([]float64, m*n)
		for i := range slacks {
			slacks[i] = math.Inf(1)
		}
		pi := make([]int, m)
		for k := range m {
			pi[rowOrder[k]] = colOrder[k]
		}
		for k := range m {
			r := rowOrder[k]
			// Chain edge (r, colOrder[(k+1)%m]) creates an O(m) cycle with small slack.
			nxtC := colOrder[(k+1)%m]
			if nxtC != pi[r] {
				slacks[r*n+nxtC] = float64(1 + rng.IntN(5))
			}
			// Also attach a couple extra random edges with higher slack.
			for range 2 {
				c := colOrder[rng.IntN(n)]
				if c != pi[r] && math.IsInf(slacks[r*n+c], 1) {
					slacks[r*n+c] = float64(20 + rng.IntN(80))
				}
			}
		}
		cost, pRow, pCol = buildPlantedPotentialMatrix(rows, cols, sense, uStar, vStar, pi, slacks, true)
		return cost, pRow, pCol, false
	}

	// Mode 2: multi-scale disconnected components spanning 2^-50 to 2^50 separated by Inf.
	numBlocks := min(m, 4)
	scales := []float64{0x1p-50, 0x1p-20, 1.0, 0x1p20, 0x1p50}
	rowPerm := rng.Perm(rows)
	colPerm := rng.Perm(cols)
	blockOfRow := make([]int, rows)
	blockOfCol := make([]int, cols)
	for r := range rows {
		blockOfRow[rowPerm[r]] = r % numBlocks
	}
	for c := range cols {
		blockOfCol[colPerm[c]] = c % numBlocks
	}

	cost = make([]float64, rows*cols)
	for i := range cost {
		cost[i] = forbid
	}
	for r := range rows {
		br := blockOfRow[r]
		sc := scales[br%len(scales)]
		for c := range cols {
			if blockOfCol[c] == br {
				v := float64(1+rng.IntN(50)) * sc
				if sense == assign.Maximize {
					v = -v
				}
				cost[r*cols+c] = v
			}
		}
	}
	return cost, nil, nil, false
}

// FuzzAssignHallAndTopology fuzzes long augmenting-chain graphs, zero-isolated-vertex
// Hall-infeasible bipartite graphs, and multi-scale disconnected block-diagonal matrices.
//
//	GOEXPERIMENT=simd go test -run=^$ -fuzz=^FuzzAssignHallAndTopology$ -fuzztime=15s ./assign
func FuzzAssignHallAndTopology(f *testing.F) {
	for seed := range uint64(12) {
		f.Add(seed, uint8(seed%3), uint8(8+seed*11), uint8(8+seed*13))
	}

	f.Fuzz(func(t *testing.T, seed uint64, topoByte, rByte, cByte uint8) {
		rng := rand.New(rand.NewPCG(seed^0x9e3779b97f4a7c15, seed^0x3c6ef372fe94f82b))
		topo := topoByte % 3
		maxDim := 220
		if raceEnabled && testing.Short() {
			maxDim = 90
		}
		rows := 7 + int(rByte)%(maxDim-6)
		cols := 7 + int(cByte)%(maxDim-6)
		if topo == 1 && rows == 7 && cols == 7 {
			cols = 8
		}
		sense := assign.Minimize
		if (seed>>2)&1 == 1 {
			sense = assign.Maximize
		}

		cost, pRow, pCol, expectInfeas := generateHallTopologyCase(rng, rows, cols, sense, topo)
		label := fmt.Sprintf("topo_%d_%dx%d_%s", topo, rows, cols, sense)

		resSIMD, errSIMD := assign.LinearSumFlat(t.Context(), rows, cols, cost, sense, assign.WithSIMD(true))
		resScalar, errScalar := assign.LinearSumFlat(t.Context(), rows, cols, cost, sense, assign.WithoutSIMD())

		if expectInfeas {
			if !errors.Is(errSIMD, assign.ErrInfeasible) || !errors.Is(errScalar, assign.ErrInfeasible) {
				t.Fatalf("[%s] expected ErrInfeasible, got simd=%v scalar=%v", label, errSIMD, errScalar)
			}
			verifyFormalAssignInfeasible(t, label, rows, cols, cost, sense)
			return
		}
		if errSIMD != nil || errScalar != nil {
			t.Fatalf("[%s] unexpected error: simd=%v scalar=%v", label, errSIMD, errScalar)
		}

		if pRow != nil {
			verifyPlantedBigRat(t, label+"/simd", rows, cols, cost, sense, resSIMD, pRow, pCol, true, false)
			verifyPlantedBigRat(t, label+"/scalar", rows, cols, cost, sense, resScalar, pRow, pCol, true, false)
			verifyFormalAssignResult(t, label+"/simd", rows, cols, cost, sense, resSIMD, false)
			verifyFormalAssignResult(t, label+"/scalar", rows, cols, cost, sense, resScalar, false)
			return
		}

		// Mode 2 (multi-scale disconnected blocks): check block isolation and SIMD/Scalar parity.
		if len(resSIMD.RowInd) != min(rows, cols) || len(resScalar.RowInd) != min(rows, cols) {
			t.Fatalf("[%s] matching length mismatch", label)
		}
		verifyOptimalityCertificate(t, label+"/simd", rows, cols, cost, sense, resSIMD)
		verifyOptimalityCertificate(t, label+"/scalar", rows, cols, cost, sense, resScalar)
		verifyFormalAssignResult(t, label+"/simd", rows, cols, cost, sense, resSIMD, false)
		verifyFormalAssignResult(t, label+"/scalar", rows, cols, cost, sense, resScalar, false)
		r1, ok1 := exactMatchedObjRat(rows, cols, cost, resSIMD.RowInd, resSIMD.ColInd)
		r2, ok2 := exactMatchedObjRat(rows, cols, cost, resScalar.RowInd, resScalar.ColInd)
		if !ok1 || !ok2 || r1.Cmp(r2) != 0 {
			t.Fatalf("[%s] SIMD vs Scalar multi-scale block mismatch", label)
		}
	})
}

// =============================================================================
// 6. SIMD-Boundary, Metamorphic & Dirty-Workspace Fuzzer
// =============================================================================

// FuzzAssignMetamorphicAndDirtyWorkspace tests SIMD tail widths, 5-way metamorphic
// invariances (SIMD vs Scalar, sense negation, transpose, row/col permutation, exact dyadic
// gauge shift), and SolveInto Workspace reuse after poisoned +Inf/-0.0/1e295 solves.
//
//	GOEXPERIMENT=simd go test -run=^$ -fuzz=^FuzzAssignMetamorphicAndDirtyWorkspace$ -fuzztime=15s ./assign
func FuzzAssignMetamorphicAndDirtyWorkspace(f *testing.F) {
	for seed := range uint64(14) {
		f.Add(seed, uint8(seed%20), uint8((seed+5)%20), uint8(seed%4))
	}

	f.Fuzz(func(t *testing.T, seed uint64, rSel, cSel, modeByte uint8) {
		rng := rand.New(rand.NewPCG(seed^0xbf58476d1ce4e5b9, seed^0x94d049bb133111eb))
		tails := []int{1, 2, 3, 4, 5, 7, 8, 9, 15, 16, 17, 31, 32, 33, 63, 64, 65, 255, 256, 257}

		var rows, cols int
		switch modeByte % 3 {
		case 0:
			rows = tails[int(rSel)%len(tails)]
			cols = tails[int(cSel)%len(tails)]
		case 1:
			rows = 2 + int(rSel%3)
			cols = 257 + int(cSel%64)
		default:
			cols = 2 + int(cSel%3)
			rows = 257 + int(rSel%64)
		}
		if raceEnabled && testing.Short() && rows*cols > 16384 {
			rows = min(rows, 65)
			cols = min(cols, 65)
		}

		sense := assign.Minimize
		if (seed>>1)&1 == 1 {
			sense = assign.Maximize
		}

		cost := make([]float64, rows*cols)
		for i := range cost {
			cost[i] = float64(rng.IntN(4096)-1024) * 0.125
		}
		label := fmt.Sprintf("meta_%dx%d_%s_%d", rows, cols, sense, seed)

		resSIMD, err := assign.LinearSumFlat(t.Context(), rows, cols, cost, sense, assign.WithSIMD(true))
		if err != nil {
			t.Fatalf("[%s/simd] failed: %v", label, err)
		}
		resScalar, err := assign.LinearSumFlat(t.Context(), rows, cols, cost, sense, assign.WithoutSIMD())
		if err != nil {
			t.Fatalf("[%s/scalar] failed: %v", label, err)
		}
		verifyOptimalityCertificate(t, label+"/simd", rows, cols, cost, sense, resSIMD)
		verifyOptimalityCertificate(t, label+"/scalar", rows, cols, cost, sense, resScalar)
		verifyFormalAssignResult(t, label+"/simd", rows, cols, cost, sense, resSIMD, true)
		verifyFormalAssignResult(t, label+"/scalar", rows, cols, cost, sense, resScalar, true)

		ratBase, ok := exactMatchedObjRat(rows, cols, cost, resSIMD.RowInd, resSIMD.ColInd)
		ratSc, okSc := exactMatchedObjRat(rows, cols, cost, resScalar.RowInd, resScalar.ColInd)
		if !ok || !okSc || ratBase.Cmp(ratSc) != 0 {
			t.Fatalf("[%s] SIMD vs Scalar exact objective mismatch", label)
		}

		// 1. Sense negation: Min(C) == -Max(-C).
		negCost := make([]float64, len(cost))
		for i, v := range cost {
			negCost[i] = -v
		}
		oppSense := assign.Maximize
		if sense == assign.Maximize {
			oppSense = assign.Minimize
		}
		resNeg, err := assign.LinearSumFlat(t.Context(), rows, cols, negCost, oppSense)
		if err != nil {
			t.Fatalf("[%s/neg] failed: %v", label, err)
		}
		ratNeg, _ := exactMatchedObjRat(rows, cols, negCost, resNeg.RowInd, resNeg.ColInd)
		if new(big.Rat).Add(ratBase, ratNeg).Sign() != 0 {
			t.Fatalf("[%s/neg] sense negation mismatch", label)
		}

		// 2. Transpose symmetry: Solve(C) == Solve(C^T).
		transCost := make([]float64, rows*cols)
		for r := range rows {
			for c := range cols {
				transCost[c*rows+r] = cost[r*cols+c]
			}
		}
		resTrans, err := assign.LinearSumFlat(t.Context(), cols, rows, transCost, sense)
		if err != nil {
			t.Fatalf("[%s/trans] failed: %v", label, err)
		}
		ratTrans, _ := exactMatchedObjRat(cols, rows, transCost, resTrans.RowInd, resTrans.ColInd)
		if ratBase.Cmp(ratTrans) != 0 {
			t.Fatalf("[%s/trans] transpose objective mismatch", label)
		}

		// 3. Row/column permutation invariance.
		rPerm := rng.Perm(rows)
		cPerm := rng.Perm(cols)
		permCost := make([]float64, rows*cols)
		for r := range rows {
			for c := range cols {
				permCost[r*cols+c] = cost[rPerm[r]*cols+cPerm[c]]
			}
		}
		resPerm, err := assign.LinearSumFlat(t.Context(), rows, cols, permCost, sense)
		if err != nil {
			t.Fatalf("[%s/perm] failed: %v", label, err)
		}
		ratPerm, _ := exactMatchedObjRat(rows, cols, permCost, resPerm.RowInd, resPerm.ColInd)
		if ratBase.Cmp(ratPerm) != 0 {
			t.Fatalf("[%s/perm] permutation objective mismatch", label)
		}

		// 4. Exact dyadic gauge invariance on square matrices (exact for rectangular when beta == 0).
		alpha := make([]float64, rows)
		beta := make([]float64, cols)
		shiftRat := new(big.Rat)
		for r := range rows {
			alpha[r] = float64(rng.IntN(64)-32) * 0.25
		}
		if rows == cols {
			for c := range cols {
				beta[c] = float64(rng.IntN(64)-32) * 0.25
			}
			for r := range rows {
				shiftRat.Add(shiftRat, new(big.Rat).SetFloat64(alpha[r]))
				shiftRat.Add(shiftRat, new(big.Rat).SetFloat64(beta[r]))
			}
		} else {
			if rows < cols {
				for r := range rows {
					shiftRat.Add(shiftRat, new(big.Rat).SetFloat64(alpha[r]))
				}
			} else {
				clear(alpha)
				for c := range cols {
					beta[c] = float64(rng.IntN(64)-32) * 0.25
					shiftRat.Add(shiftRat, new(big.Rat).SetFloat64(beta[c]))
				}
			}
		}
		gaugeCost := make([]float64, rows*cols)
		for r := range rows {
			for c := range cols {
				gaugeCost[r*cols+c] = cost[r*cols+c] + alpha[r] + beta[c]
			}
		}
		resGauge, err := assign.LinearSumFlat(t.Context(), rows, cols, gaugeCost, sense)
		if err != nil {
			t.Fatalf("[%s/gauge] failed: %v", label, err)
		}
		ratGauge, _ := exactMatchedObjRat(rows, cols, gaugeCost, resGauge.RowInd, resGauge.ColInd)
		wantGauge := new(big.Rat).Add(ratBase, shiftRat)
		if ratGauge.Cmp(wantGauge) != 0 {
			t.Fatalf("[%s/gauge] gauge shift objective mismatch", label)
		}

		// 5. Dirty Workspace reuse after a poisoned solve (1e295, -0.0, +Inf).
		ws := assign.AcquireWorkspace(rows, cols)
		defer assign.ReleaseWorkspace(ws)

		poisonN := min(max(rows, cols), 16)
		poisonCost := make([]float64, poisonN*poisonN)
		negZ := math.Copysign(0.0, -1)
		for i := range poisonN {
			for j := range poisonN {
				if i == j {
					poisonCost[i*poisonN+j] = negZ
				} else if (i+j)%2 == 0 {
					poisonCost[i*poisonN+j] = 1e295
				} else {
					poisonCost[i*poisonN+j] = math.Inf(1)
				}
			}
		}
		pR := make([]int, poisonN)
		pC := make([]int, poisonN)
		if _, pErr := assign.SolveInto(t.Context(), poisonN, poisonN, poisonCost, assign.Minimize, pR, pC, ws); pErr != nil {
			t.Fatalf("[%s/poison] unexpected error: %v", label, pErr)
		}

		k := min(rows, cols)
		rBuf := make([]int, k)
		cBuf := make([]int, k)

		obj1, err := assign.SolveInto(t.Context(), rows, cols, cost, sense, rBuf, cBuf, ws, assign.WithSIMD(true))
		if err != nil {
			t.Fatalf("[%s/ws-simd] failed: %v", label, err)
		}
		if err2 := checkReuseObj(ratBase, rows, cols, cost, rBuf, cBuf, obj1); err2 != nil {
			t.Fatalf("[%s/ws-simd] %v", label, err2)
		}

		obj2, err := assign.SolveInto(t.Context(), cols, rows, transCost, sense, rBuf, cBuf, ws, assign.WithoutSIMD())
		if err != nil {
			t.Fatalf("[%s/ws-trans-scalar] failed: %v", label, err)
		}
		if err2 := checkReuseObj(ratBase, cols, rows, transCost, rBuf, cBuf, obj2); err2 != nil {
			t.Fatalf("[%s/ws-trans-scalar] %v", label, err2)
		}

		obj3, err := assign.SolveInto(t.Context(), rows, cols, negCost, oppSense, rBuf, cBuf, ws)
		if err != nil {
			t.Fatalf("[%s/ws-opp] failed: %v", label, err)
		}
		if err2 := checkReuseObj(ratNeg, rows, cols, negCost, rBuf, cBuf, obj3); err2 != nil {
			t.Fatalf("[%s/ws-opp] %v", label, err2)
		}
	})
}

func checkReuseObj(wantRat *big.Rat, rows, cols int, cost []float64, rBuf, cBuf []int, obj float64) error {
	gotRat, ok := exactMatchedObjRat(rows, cols, cost, rBuf, cBuf)
	if !ok || gotRat.Cmp(wantRat) != 0 {
		return errors.New("reused workspace matched wrong objective")
	}
	wantF, _ := wantRat.Float64()
	if wantRat.Sign() == 0 && math.Float64bits(obj) != 0 {
		return fmt.Errorf("expected +0.0 objective, got %v", obj)
	}
	if math.Abs(obj-wantF) > max(4.0*math.SmallestNonzeroFloat64, 8.0*0x1p-53*max(1.0, math.Abs(wantF))) {
		return fmt.Errorf("reused workspace scalar obj %.17g != %.17g", obj, wantF)
	}
	return nil
}

// =============================================================================
// 7. Near-Rank-1 ULP Drift & Extreme Exponent Spread Formal Fuzzer
// =============================================================================

func buildFormalLSAPNoObjJSON(rows, cols int, flatCost []float64, sense assign.Sense, res assign.Result, tol float64) ([]byte, bool) {
	if len(flatCost) != rows*cols {
		return nil, false
	}
	raw, err := buildCompactFormalLSAPJSON(rows, cols, flatCost, sense, res, "safe", tol, false)
	if err != nil {
		return nil, false
	}
	return raw, true
}

func exactBitmaskLSAPRat(rows, cols int, cost []float64, sense assign.Sense) (*big.Rat, bool) {
	if rows == 0 || cols == 0 {
		return new(big.Rat), true
	}
	transposed := rows > cols
	m := min(rows, cols)
	n := max(rows, cols)
	if n > 10 {
		return nil, false
	}
	mat := make([][]*big.Rat, m)
	for i := range m {
		mat[i] = make([]*big.Rat, n)
		for j := range n {
			var v float64
			if !transposed {
				v = cost[i*cols+j]
			} else {
				v = cost[j*cols+i]
			}
			if math.IsInf(v, 0) || math.IsNaN(v) {
				continue
			}
			mat[i][j] = new(big.Rat).SetFloat64(v)
		}
	}

	numStates := 1 << n
	dp := make([]*big.Rat, numStates)
	dp[0] = new(big.Rat)
	cand := new(big.Rat)
	for mask := range numStates {
		cur := dp[mask]
		if cur == nil {
			continue
		}
		i := bits.OnesCount(uint(mask))
		if i >= m {
			continue
		}
		for j := range n {
			if mask&(1<<j) != 0 || mat[i][j] == nil {
				continue
			}
			nxt := mask | (1 << j)
			cand.Add(cur, mat[i][j])
			if dp[nxt] == nil ||
				(sense == assign.Minimize && cand.Cmp(dp[nxt]) < 0) ||
				(sense == assign.Maximize && cand.Cmp(dp[nxt]) > 0) {
				dp[nxt] = new(big.Rat).Set(cand)
			}
		}
	}

	var best *big.Rat
	for mask := range numStates {
		if bits.OnesCount(uint(mask)) != m || dp[mask] == nil {
			continue
		}
		if best == nil ||
			(sense == assign.Minimize && dp[mask].Cmp(best) < 0) ||
			(sense == assign.Maximize && dp[mask].Cmp(best) > 0) {
			best = dp[mask]
		}
	}
	if best == nil {
		return nil, false
	}
	return best, true
}

func verifyAdversarialFormalLSAP(
	tb testing.TB,
	label string,
	rows, cols int,
	cost []float64,
	sense assign.Sense,
	res assign.Result,
	wantOptRat *big.Rat,
	requireZeroSafeGap bool,
	safeTol float64,
) {
	tb.Helper()
	gotRat, okGot := exactMatchedObjRat(rows, cols, cost, res.RowInd, res.ColInd)
	if !okGot {
		tb.Fatalf("[%s] invalid matched indices (%v, %v)", label, res.RowInd, res.ColInd)
	}
	if wantOptRat != nil && gotRat.Cmp(wantOptRat) != 0 {
		gotF, _ := gotRat.Float64()
		wantF, _ := wantOptRat.Float64()
		gap := new(big.Rat).Sub(gotRat, wantOptRat)
		tb.Fatalf("[%s] suboptimal primal assignment: got=%s (~%.17g) want=%s (~%.17g) gap=%s",
			label, gotRat.RatString(), gotF, wantOptRat.RatString(), wantF, gap.RatString())
	}
	wantF, _ := gotRat.Float64()
	if !math.IsInf(wantF, 0) {
		ulp := math.Abs(math.Nextafter(wantF, math.Inf(1)) - wantF)
		if diff := math.Abs(res.Objective - wantF); diff > max(4*math.SmallestNonzeroFloat64, 4*ulp) {
			tb.Fatalf("[%s] res.Objective=%.17g differs from exact sum %.17g by %g",
				label, res.Objective, wantF, diff)
		}
	}
	if !formalAssignOracleEnabled() {
		return
	}

	if requireZeroSafeGap {
		rawZero, okZ := buildFormalLSAPNoObjJSON(rows, cols, cost, sense, res, 0)
		if !okZ {
			tb.Fatalf("[%s] buildFormalLSAPNoObjJSON failed", label)
		}
		okCall, bndPair, detail := formalAssignOracleCall(tb, rawZero)
		if !okCall {
			tb.Fatalf("[%s] lsap-check --jsonl safe (tol=0) rejected: %s", label, detail)
		}
		bndRat, bErr := (formalAssignRat{Num: bndPair[0], Den: bndPair[1]}).bigRat()
		if bErr != nil || bndRat.Cmp(gotRat) != 0 {
			tb.Fatalf("[%s] lsap-check bound=%v (err=%v), want exact %s", label, bndPair, bErr, gotRat.RatString())
		}
	}

	objTol := safeTol
	if objRat := new(big.Rat).SetFloat64(res.Objective); objRat != nil {
		diffRat := new(big.Rat).Sub(gotRat, objRat)
		diffRat.Abs(diffRat)
		df, exact := diffRat.Float64()
		if !exact || new(big.Rat).SetFloat64(df).Cmp(diffRat) < 0 {
			df = math.Nextafter(df, math.Inf(1))
		}
		objTol = safeTol + df
	}
	if math.IsInf(objTol, 0) {
		objTol = math.MaxFloat64
	}
	includeObj := !math.IsInf(res.Objective, 0) && !math.IsNaN(res.Objective)
	rawFull, errFull := buildCompactFormalLSAPJSON(rows, cols, cost, sense, res, "safe", objTol, includeObj)
	if errFull != nil {
		tb.Fatalf("[%s] buildCompactFormalLSAPJSON safe (tol=%g) failed: %v", label, objTol, errFull)
	}
	respFull, callErr := callFormalAssignJSONL(tb, rawFull)
	if callErr != nil || !respFull.OK || respFull.Mode != "safe" {
		tb.Fatalf("[%s] lsap-check --jsonl safe (tol=%g) rejected (err=%v resp=%+v)", label, objTol, callErr, respFull)
	}
	if respFull.Bound != nil {
		bndRat, bErr := respFull.Bound.bigRat()
		if bErr != nil {
			tb.Fatalf("[%s] invalid bound %+v: %v", label, respFull.Bound, bErr)
		}
		if sense == assign.Minimize && bndRat.Cmp(gotRat) > 0 {
			tb.Fatalf("[%s] safe lower bound %s > primal %s", label, bndRat.RatString(), gotRat.RatString())
		}
		if sense == assign.Maximize && bndRat.Cmp(gotRat) < 0 {
			tb.Fatalf("[%s] safe upper bound %s < primal %s", label, bndRat.RatString(), gotRat.RatString())
		}
	}
}

// FuzzAssignAdversarialFormal tests near-rank-1 ULP drift (c[i][j] = A[i] + B[j] + k*ulp
// across scales 1e-8, 1, 1e8, 1e15) and extreme exponent spread (1e-320..1e305) against
// exact rational matching optima and lsap-check --jsonl certificates.
func FuzzAssignAdversarialFormal(f *testing.F) {
	for seed := range uint64(24) {
		f.Add(seed, uint8(seed%3), uint8(seed%6), uint8((seed*3)%8))
	}

	f.Fuzz(func(t *testing.T, seed uint64, regimeByte, dimByte, optByte uint8) {
		rng := rand.New(rand.NewPCG(seed^0x517cc1b727220a95, seed^0x9e3779b97f4a7c15))
		sense := assign.Minimize
		if optByte&1 == 1 {
			sense = assign.Maximize
		}
		regime := regimeByte % 3

		if regime < 2 {
			// Generator #7: c[i][j] = A[i] + B[j] + k*ulp across square and rectangular shapes.
			var rows, cols int
			switch dimByte % 3 {
			case 0:
				rows = 5 + int((dimByte/3)%5)
				cols = rows
			case 1:
				rows = 3 + int((dimByte/3)%5)
				cols = rows + 1 + rng.IntN(min(3, 10-rows))
			default:
				cols = 3 + int((dimByte/3)%5)
				rows = cols + 1 + rng.IntN(min(3, 10-cols))
			}
			scales := []float64{1e-8, 1.0, 1e8, 1e15}
			scIdx := int(optByte>>1) % len(scales)
			scale := scales[scIdx]

			a := make([]float64, rows)
			b := make([]float64, cols)
			singleOctave := regime == 0
			for r := range rows {
				if singleOctave {
					if scIdx == 3 {
						a[r] = scale * (0.60 + 0.42*rng.Float64())
					} else {
						a[r] = scale * (1.00 + 0.26*rng.Float64())
					}
				} else {
					a[r] = scale * (1.0 + rng.Float64())
				}
			}
			for c := range cols {
				if singleOctave {
					if scIdx == 3 {
						b[c] = scale * (0.60 + 0.42*rng.Float64())
					} else {
						b[c] = scale * (1.00 + 0.26*rng.Float64())
					}
				} else {
					b[c] = scale * (1.0 + rng.Float64())
				}
			}

			forbid := math.Inf(1)
			if sense == assign.Maximize {
				forbid = math.Inf(-1)
			}
			injectInf := regime == 1 && (optByte&8 != 0)
			cost := make([]float64, rows*cols)
			maxULP := 0.0
			for r := range rows {
				for c := range cols {
					if injectInf && rng.IntN(6) == 0 {
						cost[r*cols+c] = forbid
						continue
					}
					base := a[r] + b[c]
					ulp := math.Abs(math.Nextafter(base, math.Inf(1)) - base)
					if ulp > maxULP {
						maxULP = ulp
					}
					k := rng.IntN(33) - 16
					val := base + float64(k)*ulp
					if sense == assign.Maximize && (optByte&16 != 0) {
						val = -val
					}
					cost[r*cols+c] = val
				}
			}

			wantRat, wantFeas := exactBitmaskLSAPRat(rows, cols, cost, sense)
			label := fmt.Sprintf("adv_lsap_ulp_r%d_%dx%d_sc%g_%s_%d", regime, rows, cols, scale, sense, seed)

			resSIMD, errSIMD := assign.LinearSumFlat(t.Context(), rows, cols, cost, sense, assign.WithSIMD(true))
			resScalar, errScalar := assign.LinearSumFlat(t.Context(), rows, cols, cost, sense, assign.WithoutSIMD())
			if !wantFeas {
				if !errors.Is(errSIMD, assign.ErrInfeasible) || !errors.Is(errScalar, assign.ErrInfeasible) {
					t.Fatalf("[%s] expected ErrInfeasible, got simd=%v scalar=%v", label, errSIMD, errScalar)
				}
				verifyFormalAssignInfeasible(t, label, rows, cols, cost, sense)
				return
			}
			if errSIMD != nil || errScalar != nil {
				t.Fatalf("[%s] unexpected error: simd=%v scalar=%v", label, errSIMD, errScalar)
			}
			safeTol := 0.0
			if !singleOctave {
				safeTol = float64(max(rows, cols)) * maxULP
			}
			verifyAdversarialFormalLSAP(t, label+"/simd", rows, cols, cost, sense, resSIMD, wantRat, singleOctave && !injectInf, safeTol)
			verifyAdversarialFormalLSAP(t, label+"/scalar", rows, cols, cost, sense, resScalar, wantRat, singleOctave && !injectInf, safeTol)
			return
		}

		// Generator #8: extreme exponent spread (1e-320..1e305) across 2..6 dimensions.
		rows := 2 + int(dimByte%5)
		cols := 2 + int((dimByte/5)%5)
		cost := make([]float64, rows*cols)
		maxAbs := 1.0
		for i := range cost {
			var v float64
			switch rng.IntN(4) {
			case 0:
				v = math.Ldexp(float64(1+rng.IntN(63)), -1070+rng.IntN(40))
			case 1:
				v = float64(1+rng.IntN(32)) * 0.25
			case 2:
				v = math.Ldexp(1.0+rng.Float64(), 965+rng.IntN(45))
			default:
				v = math.Ldexp(1.0+rng.Float64(), -400+rng.IntN(800))
			}
			if rng.IntN(3) == 0 {
				v = -v
			}
			cost[i] = v
			if a := math.Abs(v); a > maxAbs {
				maxAbs = a
			}
		}

		wantRat, wantFeas := exactBitmaskLSAPRat(rows, cols, cost, sense)
		if !wantFeas {
			return
		}
		label := fmt.Sprintf("adv_lsap_exp_%dx%d_%s_%d", rows, cols, sense, seed)
		resSIMD, errSIMD := assign.LinearSumFlat(t.Context(), rows, cols, cost, sense, assign.WithSIMD(true))
		resScalar, errScalar := assign.LinearSumFlat(t.Context(), rows, cols, cost, sense, assign.WithoutSIMD())
		if errSIMD != nil || errScalar != nil {
			t.Fatalf("[%s] unexpected error: simd=%v scalar=%v", label, errSIMD, errScalar)
		}
		tol := max(1e-300, maxAbs*0x1p-48*float64(max(rows, cols)))
		if math.IsInf(tol, 0) {
			tol = math.MaxFloat64
		}
		verifyAdversarialFormalLSAP(t, label+"/simd", rows, cols, cost, sense, resSIMD, wantRat, false, tol)
		verifyAdversarialFormalLSAP(t, label+"/scalar", rows, cols, cost, sense, resScalar, wantRat, false, tol)
	})
}

// =============================================================================
// Deterministic Invariant Checks for Crafted Assign Generators
// =============================================================================

func TestCraftedAssignGeneratorInvariants(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	// 1. Verify exact big.Rat complementary slackness across all planted cancellation regimes.
	for regime := range uint8(5) {
		for _, dims := range [][2]int{{6, 6}, {5, 9}, {9, 5}} {
			rng := rand.New(rand.NewPCG(uint64(regime)+100, uint64(dims[0]*31+dims[1])))
			sense := assign.Minimize
			if regime%2 == 1 {
				sense = assign.Maximize
			}
			cost, pRow, pCol, unique, bitExact := generatePlantedCancellationCase(rng, dims[0], dims[1], sense, regime)
			res, err := assign.LinearSumFlat(ctx, dims[0], dims[1], cost, sense)
			if err != nil {
				t.Fatalf("cancel regime %d (%dx%d): err=%v", regime, dims[0], dims[1], err)
			}
			verifyPlantedBigRat(t, fmt.Sprintf("inv_cancel_r%d_%dx%d", regime, dims[0], dims[1]),
				dims[0], dims[1], cost, sense, res, pRow, pCol, unique, bitExact)
		}
	}

	// 2. Verify Hall-infeasible generator has degree >= 3 on every vertex and no finite matching.
	for _, dims := range [][2]int{{8, 8}, {9, 12}, {12, 9}} {
		rng := rand.New(rand.NewPCG(909, uint64(dims[0]*17+dims[1])))
		cost := makeHallInfeasibleGraph(rng, dims[0], dims[1], assign.Minimize)
		for r := range dims[0] {
			deg := 0
			for c := range dims[1] {
				if !math.IsInf(cost[r*dims[1]+c], 0) {
					deg++
				}
			}
			if deg < 3 {
				t.Fatalf("Hall %dx%d row %d degree=%d < 3", dims[0], dims[1], r, deg)
			}
		}
		for c := range dims[1] {
			deg := 0
			for r := range dims[0] {
				if !math.IsInf(cost[r*dims[1]+c], 0) {
					deg++
				}
			}
			if deg < 3 {
				t.Fatalf("Hall %dx%d col %d degree=%d < 3", dims[0], dims[1], c, deg)
			}
		}
		if hasCompleteFiniteMatching(dims[0], dims[1], cost) {
			t.Fatalf("Hall %dx%d generator unexpectedly had complete finite matching", dims[0], dims[1])
		}
	}

	// 3. Verify sparse-core trap regimes solve cleanly against planted exact rationals at N=1000.
	if !raceEnabled || !testing.Short() {
		for regime := range uint8(4) {
			rng := rand.New(rand.NewPCG(uint64(regime)+5000, 777))
			cost, pRow, pCol, expectInfeas := generateSparseCoreTrap(rng, 1000, assign.Minimize, regime)
			res, err := assign.LinearSumFlat(ctx, 1000, 1000, cost, assign.Minimize, assign.WithoutSIMD())
			if expectInfeas {
				if !errors.Is(err, assign.ErrInfeasible) {
					t.Fatalf("sparse trap regime %d expected ErrInfeasible, got %v", regime, err)
				}
				continue
			}
			if err != nil {
				t.Fatalf("sparse trap regime %d failed: %v", regime, err)
			}
			verifyPlantedBigRat(t, fmt.Sprintf("inv_sparse_r%d", regime), 1000, 1000, cost, assign.Minimize, res, pRow, pCol, false, false)
		}
	}
}

// TestAssignStage2AndWideAccRegressions pins the exact double-rounding and Stage-2
// multi-pass gauge-collapse regressions across SIMD and scalar backends.
func TestAssignStage2AndWideAccRegressions(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	backends := []struct {
		name string
		opt  assign.Option
	}{
		{name: "simd", opt: assign.WithSIMD(true)},
		{name: "scalar", opt: assign.WithoutSIMD()},
	}

	wideCases := []struct {
		name   string
		vals   []float64
		senses []assign.Sense
	}{
		{
			name:   "ulp53_plus_subnormal",
			vals:   []float64{0x1p53, 1.0, 0x1p-1050},
			senses: []assign.Sense{assign.Minimize, assign.Maximize},
		},
		{
			name:   "ulp53_minus_subnormal",
			vals:   []float64{0x1p53, 1.0, -0x1p-1050},
			senses: []assign.Sense{assign.Minimize, assign.Maximize},
		},
		{
			name:   "subnormal_boundary_bias",
			vals:   []float64{0x1p82, 0x1p29, 0x1p-1020, 0x1p-1050},
			senses: []assign.Sense{assign.Minimize, assign.Maximize},
		},
		{
			name:   "near_max_float64_cancel_subnormal",
			vals:   []float64{math.MaxFloat64 - 0x1p972, 0x1p970, -0x1p-1050, 0x1p500, -0x1p500},
			senses: []assign.Sense{assign.Minimize, assign.Maximize},
		},
	}

	for idx, tc := range wideCases {
		for _, sense := range tc.senses {
			rng := rand.New(rand.NewPCG(uint64(idx+101), uint64(sense)+17))
			n, cost, pRow, pCol := packForcedPermMatrix(rng, tc.vals, sense)
			for _, backend := range backends {
				label := fmt.Sprintf("%s_%s_%s", tc.name, sense, backend.name)
				res, err := assign.LinearSumFlat(ctx, n, n, cost, sense, backend.opt)
				if err != nil {
					t.Fatalf("[%s] unexpected error: %v", label, err)
				}
				verifyPlantedBigRat(t, label, n, n, cost, sense, res, pRow, pCol, true, true)
			}
		}
	}

	for _, seed := range []struct {
		name string
		seed uint64
		rows int
		cols int
	}{
		{name: "stage2_seed6_10x31", seed: 6, rows: 10, cols: 31},
		{name: "stage2_seed9_eb0466_8x8", seed: 9, rows: 8, cols: 8},
	} {
		rng := rand.New(rand.NewPCG(seed.seed^0x41c6ce57, seed.seed^0x9e3779b97f4a7c15))
		cost, pRow, pCol, unique, bitExact := generatePlantedCancellationCase(rng, seed.rows, seed.cols, assign.Minimize, 1)
		for _, backend := range backends {
			label := seed.name + "_" + backend.name
			res, err := assign.LinearSumFlat(ctx, seed.rows, seed.cols, cost, assign.Minimize, backend.opt)
			if err != nil {
				t.Fatalf("[%s] unexpected error: %v", label, err)
			}
			verifyPlantedBigRat(t, label, seed.rows, seed.cols, cost, assign.Minimize, res, pRow, pCol, unique, bitExact)
		}
	}

	// Verify 3-tier multi-scale cancellation staircase across square (8x8) and rectangular (8x14).
	for _, shape := range [][2]int{{8, 8}, {8, 14}} {
		rows, cols := shape[0], shape[1]
		for _, sense := range []assign.Sense{assign.Minimize, assign.Maximize} {
			rng := rand.New(rand.NewPCG(uint64(rows*31+cols), uint64(sense)+99))
			pi := rng.Perm(cols)[:rows]
			vMatched := []float64{-0x1p1014, -0x1p1014, -0x1p960, -0x1p960, -0x1p500, -0x1p500, -4.0, -0x1p-1050}
			var uStar []float64
			if rows == cols {
				uStar = []float64{0x1p1014, -0x1p1014, 0x1p960, -0x1p960, 0x1p500, -0x1p500, 3.0, 0x1p-1049}
				vMatched = []float64{-0x1p1014, 0x1p1014, -0x1p960, 0x1p960, -0x1p500, 0x1p500, -4.0, -0x1p-1050}
			} else {
				uStar = []float64{0x1p1014, 0x1p1014 + 0x1p962, 0x1p960, 0x1p960 + 0x1p910, 0x1p500, 0x1p500, 3.0, 0x1p-1049}
			}
			vStar := make([]float64, cols)
			for i, c := range pi {
				vStar[c] = vMatched[i]
			}
			tierSlacks := []float64{0x1p970, 0x1p900, 0x1p450, 1.0, 0x1p-1048}
			slacks := make([]float64, rows*cols)
			for i := range rows {
				for j := range cols {
					if j == pi[i] {
						slacks[i*cols+j] = math.Inf(1)
						continue
					}
					slacks[i*cols+j] = tierSlacks[(i*cols+j)%len(tierSlacks)]
				}
			}
			cost, pRow, pCol := buildPlantedPotentialMatrix(rows, cols, sense, uStar, vStar, pi, slacks, true)
			for _, backend := range backends {
				label := fmt.Sprintf("staircase_%dx%d_%s_%s", rows, cols, sense, backend.name)
				res, err := assign.LinearSumFlat(ctx, rows, cols, cost, sense, backend.opt)
				if err != nil {
					t.Fatalf("[%s] unexpected error: %v", label, err)
				}
				verifyPlantedBigRat(t, label, rows, cols, cost, sense, res, pRow, pCol, true, true)
			}
		}
	}
}
