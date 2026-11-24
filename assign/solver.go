package assign

import (
	"context"
	"errors"
	"math"
	"slices"

	"github.com/paulourio/goblin/internal/simd"
)

var errContextCanceled = errors.New("assign: context canceled")

const hugeCostThreshold = 1e290

func clampFinite(x float64) float64 {
	if x > math.MaxFloat64 {
		return math.MaxFloat64
	}
	if x < -math.MaxFloat64 {
		return -math.MaxFloat64
	}
	return x
}

func minimalDownscaleExp(maxAbsFinite float64, n int) int {
	safeCap := math.MaxFloat64 / float64(max(64, 16*n))
	if !(maxAbsFinite > safeCap) {
		return 0
	}
	_, me := math.Frexp(maxAbsFinite)
	_, se := math.Frexp(safeCap)
	return max(1, me-se+1)
}

func twoSumAssign(a, b float64) (s, err float64) {
	// Explicit float64 conversions prevent FMA contraction from breaking the error-free transform under GOAMD64=v3/arm64.
	a = float64(a)
	b = float64(b)
	s = float64(a + b)
	bb := float64(s - a)
	err = float64(a-float64(s-bb)) + float64(b-bb)
	return s, err
}

const (
	assignWideHalf = 528
	assignWideCut  = 960
	assignExpCap   = 64
)

func compressAssignSlice(e []float64) []float64 {
	n := len(e)
	if n == 0 {
		return e
	}
	if n == 1 {
		if e[0] == 0 {
			return e[:0]
		}
		return e
	}
	q := e[n-1]
	bottom := n - 1
	for i := n - 2; i >= 0; i-- {
		newQ, err := twoSumAssign(q, e[i])
		if err != 0 {
			e[bottom] = newQ
			bottom--
			q = err
		} else {
			q = newQ
		}
	}
	e[bottom] = q
	w := 0
	q = e[bottom]
	for i := bottom + 1; i < n; i++ {
		newQ, err := twoSumAssign(e[i], q)
		if err != 0 {
			e[w] = err
			w++
		}
		q = newQ
	}
	if q != 0 {
		e[w] = q
		return e[:w+1]
	}
	return e[:w]
}

func growAssignSlice(e []float64, b float64) []float64 {
	if b == 0 || math.IsNaN(b) || math.IsInf(b, 0) {
		return e
	}
	q := b
	w := 0
	for _, ei := range e {
		var h float64
		q, h = twoSumAssign(q, ei)
		if math.IsNaN(q) || math.IsInf(q, 0) {
			return e[:w]
		}
		if h != 0 {
			e[w] = h
			w++
		}
	}
	e = e[:w]
	if q != 0 {
		e = append(e, q)
	}
	return e
}

func collapseAssignBuf(e *[assignExpCap]float64, n int) int {
	return len(compressAssignSlice(e[:n]))
}

func assignTopRem(comp []float64) (top, rem float64) {
	if len(comp) == 0 {
		return 0, 0
	}
	if len(comp) == 1 {
		return comp[0], 0
	}
	for i := range len(comp) - 1 {
		rem += comp[i]
	}
	return twoSumAssign(comp[len(comp)-1], rem)
}

func assignRoundToOddSticky(top, rem float64) float64 {
	if rem == 0 || math.Float64bits(top)&1 != 0 {
		return top
	}
	if (top > 0) == (rem > 0) {
		return math.Nextafter(top, math.Copysign(math.Inf(1), top))
	}
	return math.Nextafter(top, 0)
}

func assignRoundToOddSign(v float64, tailSign int) float64 {
	if tailSign == 0 || v == 0 || math.Float64bits(v)&1 != 0 {
		return v
	}
	if (v > 0) == (tailSign > 0) {
		return math.Nextafter(v, math.Copysign(math.Inf(1), v))
	}
	return math.Nextafter(v, 0)
}

func assignRoundComp(comp []float64, tailSign int) float64 {
	n := len(comp)
	if n == 0 {
		return 0
	}
	top := comp[n-1]
	if n == 1 {
		return top
	}
	tSign := tailSign
	if n >= 3 {
		tSign = assignSignOf(comp[n-3])
	}
	rem := assignRoundToOddSign(comp[n-2], tSign)
	comb := top + rem
	if math.IsNaN(comb) {
		return top
	}
	return comb
}

func assignRoundToSubnormalComp(comp []float64, biasScale float64, tailSign int) float64 {
	n := len(comp)
	if n == 0 {
		return 0
	}
	cTop := comp[n-1]
	tSign := tailSign
	if n >= 3 {
		tSign = assignSignOf(comp[n-3])
	}
	rem := 0.0
	if n >= 2 {
		rem = assignRoundToOddSign(comp[n-2], tSign)
	}
	ac := math.Abs(cTop)
	if ac > biasScale || (ac == biasScale && (rem == 0 || (cTop > 0) == (rem > 0))) {
		comb := cTop + rem
		if math.IsNaN(comb) {
			return cTop
		}
		return comb
	}
	bias := math.Copysign(biasScale, cTop)
	hi, lo1 := twoSumAssign(bias, cTop)
	if n == 1 {
		tail := assignRoundToOddSign(lo1, tSign)
		return (hi + tail) - bias
	}
	t, u := twoSumAssign(lo1, rem)
	var tail float64
	if u != 0 {
		tail = assignRoundToOddSticky(t, u)
	} else {
		tail = assignRoundToOddSign(t, tSign)
	}
	return (hi + tail) - bias
}

func growAssignBuf(e *[assignExpCap]float64, n int, b float64) (int, bool) {
	if b == 0 {
		return n, true
	}
	if math.IsNaN(b) || math.IsInf(b, 0) {
		return n, false
	}
	if n >= assignExpCap {
		n = collapseAssignBuf(e, n)
		if n >= assignExpCap {
			top, rem := assignTopRem(e[:n])
			n = 0
			if rem != 0 {
				e[0] = rem
				n = 1
			}
			if top != 0 {
				e[n] = top
				n++
			}
		}
	}
	q := b
	w := 0
	for i := range n {
		var h float64
		q, h = twoSumAssign(q, e[i])
		if math.IsNaN(q) || math.IsInf(q, 0) {
			return w, false
		}
		if h != 0 {
			e[w] = h
			w++
		}
	}
	if q != 0 {
		e[w] = q
		w++
	}
	return w, true
}

type assignWideAcc struct {
	hiBuf  [assignExpCap]float64
	midBuf [assignExpCap]float64
	loBuf  [assignExpCap]float64
	hiLen  int
	midLen int
	loLen  int
	nonFin bool
}

func (w *assignWideAcc) reset() {
	w.hiLen = 0
	w.midLen = 0
	w.loLen = 0
	w.nonFin = false
}

func (w *assignWideAcc) addVal(v float64) {
	if v == 0 || w.nonFin {
		return
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		w.nonFin = true
		return
	}
	_, e := math.Frexp(math.Abs(v))
	var ok bool
	switch {
	case e > assignWideCut:
		w.hiLen, ok = growAssignBuf(&w.hiBuf, w.hiLen, math.Ldexp(math.Ldexp(v, -assignWideHalf), -assignWideHalf))
	case e < -assignWideCut:
		w.loLen, ok = growAssignBuf(&w.loBuf, w.loLen, math.Ldexp(math.Ldexp(v, assignWideHalf), assignWideHalf))
	default:
		w.midLen, ok = growAssignBuf(&w.midBuf, w.midLen, v)
	}
	if !ok {
		w.nonFin = true
	}
}

func (w *assignWideAcc) addScaled(v float64, shift int) {
	if v == 0 || w.nonFin {
		return
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		w.nonFin = true
		return
	}
	f, e := math.Frexp(v)
	totE := e + shift
	var ok bool
	switch {
	case totE > assignWideCut:
		w.hiLen, ok = growAssignBuf(&w.hiBuf, w.hiLen, math.Ldexp(f, totE-2*assignWideHalf))
	case totE < -assignWideCut:
		w.loLen, ok = growAssignBuf(&w.loBuf, w.loLen, math.Ldexp(f, totE+2*assignWideHalf))
	default:
		w.midLen, ok = growAssignBuf(&w.midBuf, w.midLen, math.Ldexp(f, totE))
	}
	if !ok {
		w.nonFin = true
	}
}

func assignSignOf(v float64) int {
	if v > 0 {
		return 1
	}
	if v < 0 {
		return -1
	}
	return 0
}

func (w *assignWideAcc) eval() (val float64, sgn int) {
	if w.nonFin {
		return math.NaN(), 0
	}
	if w.hiLen == 0 && w.midLen == 0 && w.loLen == 0 {
		return 0, 0
	}
	w.hiLen = collapseAssignBuf(&w.hiBuf, w.hiLen)
	w.midLen = collapseAssignBuf(&w.midBuf, w.midLen)
	w.loLen = collapseAssignBuf(&w.loBuf, w.loLen)
	hi := w.hiBuf[:w.hiLen]
	mid := w.midBuf[:w.midLen]
	lo := w.loBuf[:w.loLen]

	var midWork [3 * assignExpCap]float64
	midMerged := midWork[:0]
	if len(hi) == 0 {
		midMerged = append(midMerged, mid...)
	} else {
		hiTop := hi[len(hi)-1]
		_, eHi := math.Frexp(math.Abs(hiTop))
		if eHi+2*assignWideHalf > 1026 {
			return math.Copysign(math.Inf(1), hiTop), assignSignOf(hiTop)
		}
		var halfWork [2 * assignExpCap]float64
		half := halfWork[:0]
		for _, h := range hi {
			half = growAssignSlice(half, math.Ldexp(h, assignWideHalf))
		}
		for _, m := range mid {
			if math.Abs(m) >= 0x1p-400 {
				half = growAssignSlice(half, math.Ldexp(m, -assignWideHalf))
			}
		}
		half = compressAssignSlice(half)
		if len(half) > 0 {
			halfTop := half[len(half)-1]
			_, eHalf := math.Frexp(math.Abs(halfTop))
			if eHalf+assignWideHalf > 1024 {
				var sc4Work [3 * assignExpCap]float64
				sc4 := sc4Work[:0]
				midDropSign := 0
				for _, m := range mid {
					if math.Abs(m) < 0x1p-400 {
						if math.Abs(m) >= 0x1p-1018 {
							sc4 = growAssignSlice(sc4, math.Ldexp(m, -4))
						} else {
							midDropSign = assignSignOf(m)
						}
					}
				}
				for _, c := range half {
					sc4 = growAssignSlice(sc4, math.Ldexp(c, assignWideHalf-4))
				}
				sc4 = compressAssignSlice(sc4)
				if len(sc4) == 0 {
					return 0, 0
				}
				tailSign := midDropSign
				if tailSign == 0 && len(lo) > 0 {
					tailSign = assignSignOf(lo[len(lo)-1])
				}
				val4 := assignRoundComp(sc4, tailSign)
				return math.Ldexp(val4, 4), assignSignOf(sc4[len(sc4)-1])
			}
		}
		for _, m := range mid {
			if math.Abs(m) < 0x1p-400 {
				midMerged = growAssignSlice(midMerged, m)
			}
		}
		for _, c := range half {
			midMerged = growAssignSlice(midMerged, math.Ldexp(c, assignWideHalf))
		}
		midMerged = compressAssignSlice(midMerged)
	}

	if len(lo) == 0 {
		if len(midMerged) == 0 {
			return 0, 0
		}
		return assignRoundComp(midMerged, 0), assignSignOf(midMerged[len(midMerged)-1])
	}

	var loSmallWork [assignExpCap]float64
	loSmall := loSmallWork[:0]
	addedLoNorm := false
	for _, l := range lo {
		if math.Abs(l) >= 0x1p34 {
			midMerged = growAssignSlice(midMerged, math.Ldexp(math.Ldexp(l, -assignWideHalf), -assignWideHalf))
			addedLoNorm = true
		} else {
			loSmall = append(loSmall, l)
		}
	}
	if addedLoNorm {
		midMerged = compressAssignSlice(midMerged)
	}
	if len(loSmall) == 0 {
		if len(midMerged) == 0 {
			return 0, 0
		}
		return assignRoundComp(midMerged, 0), assignSignOf(midMerged[len(midMerged)-1])
	}

	if len(midMerged) == 0 {
		adj := assignRoundToSubnormalComp(loSmall, 0x1p34, 0)
		return math.Ldexp(math.Ldexp(adj, -assignWideHalf), -assignWideHalf), assignSignOf(loSmall[len(loSmall)-1])
	}

	if math.Abs(midMerged[0]) < 0x1p-960 {
		var upWork [2 * assignExpCap]float64
		up := upWork[:0]
		var topKeepWork [assignExpCap]float64
		topKeep := topKeepWork[:0]
		for _, m := range midMerged {
			if math.Abs(m) < 0x1p400 {
				up = growAssignSlice(up, math.Ldexp(m, assignWideHalf))
			} else {
				topKeep = append(topKeep, m)
			}
		}
		var loTinySign int
		for _, l := range loSmall {
			if math.Abs(l) >= 0x1p-490 {
				up = growAssignSlice(up, math.Ldexp(l, -assignWideHalf))
			} else {
				loTinySign = assignSignOf(l)
			}
		}
		up = compressAssignSlice(up)
		if len(topKeep) == 0 {
			if len(up) == 0 {
				return 0, loTinySign
			}
			adj := assignRoundToSubnormalComp(up, 0x1p-494, loTinySign)
			return math.Ldexp(adj, -assignWideHalf), assignSignOf(up[len(up)-1])
		}
		rebuilt := midWork[:0]
		upDropSign := 0
		for _, u := range up {
			if math.Abs(u) >= 0x1p-494 {
				rebuilt = growAssignSlice(rebuilt, math.Ldexp(u, -assignWideHalf))
			} else {
				upDropSign = assignSignOf(u)
			}
		}
		for _, m := range topKeep {
			rebuilt = growAssignSlice(rebuilt, m)
		}
		rebuilt = compressAssignSlice(rebuilt)
		dropSign := upDropSign
		if dropSign == 0 {
			dropSign = loTinySign
		}
		return assignRoundComp(rebuilt, dropSign), assignSignOf(rebuilt[len(rebuilt)-1])
	}

	return assignRoundComp(midMerged, assignSignOf(loSmall[len(loSmall)-1])), assignSignOf(midMerged[len(midMerged)-1])
}

func (w *assignWideAcc) sign() int {
	_, sgn := w.eval()
	return sgn
}

func (w *assignWideAcc) finish() float64 {
	val, _ := w.eval()
	return val
}

//go:noinline
func computeMatchedObjectiveWide(m, n int, cost []float64, col4row []int) float64 {
	var acc assignWideAcc
	for i := range m {
		j := col4row[i]
		if uint(j) < uint(n) {
			acc.addVal(cost[i*n+j])
		}
	}
	return acc.finish()
}

func computeMatchedObjective(m, n int, cost []float64, col4row []int, allowFast bool) float64 {
	if allowFast {
		sum := 0.0
		comp := 0.0
		sumAbs := 0.0
		needWide := false
		for i := range m {
			j := col4row[i]
			if uint(j) >= uint(n) {
				continue
			}
			c := cost[i*n+j]
			ac := math.Abs(c)
			if !(ac <= 1e150) || (ac != 0 && ac < 1e-150) {
				needWide = true
				break
			}
			sumAbs += ac
			t := sum + c
			if math.Abs(sum) >= ac {
				comp += (sum - t) + c
			} else {
				comp += (c - t) + sum
			}
			sum = t
		}
		if !needWide {
			res := sum + comp
			if !math.IsNaN(res) && !math.IsInf(res, 0) && math.Abs(res) >= 0.01*sumAbs {
				return res
			}
		}
	}
	return computeMatchedObjectiveWide(m, n, cost, col4row)
}

// exactReducedCost computes c - ui - vj with exact sign for finite inputs via
// four TwoSum steps, returning ok=false if any intermediate sum overflows.
func exactReducedCost(c, ui, vj float64) (rc float64, ok bool) {
	s1, e1 := twoSumAssign(c, -ui)
	s2, e2 := twoSumAssign(s1, -vj)
	q1, r1 := twoSumAssign(e1, e2)
	q2, r2 := twoSumAssign(s2, q1)
	if math.IsInf(s1, 0) || math.IsInf(s2, 0) || math.IsInf(q1, 0) || math.IsInf(q2, 0) {
		return 0, false
	}
	return q2 + (r1 + r2), true
}

const (
	cancelDualFiniteCap = 0x1p1016
	cancelRowLimitCap   = 0x1p1020
	exactIntDualCap     = 0x1p50
)

// exactSmallIntLimit returns the largest integer cost magnitude M such that every
// intermediate sum/difference in Stage-1 Jonker-Volgenant / Bijsterbosch-Volgenant
// (bounded by (142*n + 34)*M < (256*n + 256)*M <= 2^52 < 2^53) executes bit-exactly
// in IEEE-754 float64 with zero rounding error.
func exactSmallIntLimit(n int) float64 {
	if n <= 0 {
		return 0
	}
	return min(0x1p40, math.Floor(0x1p52/float64(256*n+256)))
}

func isExactSmallInt(c, intLim float64) bool {
	return math.Abs(c) <= intLim && math.Trunc(c) == c
}

// probeExactSmallInt checks three representative entries (first, second, last) in O(1)
// so floating-point cost matrices skip exact-integer tracking in ~1 ns.
func probeExactSmallInt(cost []float64, intLim float64, negate bool) bool {
	n := len(cost)
	if n == 0 || !(0 <= intLim) {
		return false
	}
	forbid := math.Inf(1)
	if negate {
		forbid = math.Inf(-1)
	}
	c0 := cost[0]
	if c0 != forbid && !isExactSmallInt(c0, intLim) {
		return false
	}
	if n > 1 {
		c1 := cost[1]
		if c1 != forbid && !isExactSmallInt(c1, intLim) {
			return false
		}
		cl := cost[n-1]
		if cl != forbid && !isExactSmallInt(cl, intLim) {
			return false
		}
	}
	return true
}

// verifyExactSmallIntStage1 verifies in O(m + n) that Stage 1 produced exact integer
// potentials (u, v) and an exact zero-slack primal matching on a validated exact
// small-integer cost matrix (all finite entries integers with |c| <= intLim).
// Because every Stage-1 operation (+, -, <, <=) stays strictly below 2^52 and executes
// bit-exactly in IEEE-754 float64, dual feasibility u[i] + v[j] <= c[i,j] holds over Q
// identically and no O(m*n) cancellation scan is needed.
func verifyExactSmallIntStage1(m, n int, normCost, u, v []float64, col4row, row4col []int, intLim float64) (float64, bool) {
	for j, vj := range v[:n] {
		if !(math.Abs(vj) <= exactIntDualCap) || math.Trunc(vj) != vj {
			return 0, false
		}
		if m < n && (vj > 0 || (row4col[j] < 0 && vj != 0)) {
			return 0, false
		}
	}
	obj := 0.0
	for i := range m {
		mj := col4row[i]
		if uint(mj) >= uint(n) || row4col[mj] != i {
			return 0, false
		}
		ui := u[i]
		if !(math.Abs(ui) <= exactIntDualCap) || math.Trunc(ui) != ui {
			return 0, false
		}
		cm := normCost[i*n+mj]
		if !(math.Abs(cm) <= intLim) || math.Trunc(cm) != cm {
			return 0, false
		}
		if (cm-ui)-v[mj] != 0 {
			return 0, false
		}
		obj += cm
	}
	return obj, true
}

// hasPotentialCancellation certifies exact primal optimality and dual feasibility of the
// Stage-1 solution (col4row, u, v) over Q with zero heap allocations.
//
// For each row i, viol_i = max(0, max_j (u[i] + v[j] - c[i,j])) and
// slack_i = max(0, c[i,col4row[i]] - u[i] - v[col4row[i]]). Shifting u'[i] = u[i] - viol_i
// yields an exact dual-feasible potential with total duality gap G = sum_i (slack_i + viol_i).
// A preliminary O(m + n) matched-edge scan establishes a conservative upper bound candThresh
// on G, allowing a single fused matrix SIMD sweep (ScanNearTightMatrix64With) to gather all
// near-tight DAG edges and violating cells into preallocated workspace scratch so Kahn
// acyclicity and downward u[i] projection touch only O(m) cached candidates instead of
// rescanning m*n entries.
func hasPotentialCancellation(m, n int, normCost, u, v []float64, col4row, row4col []int, ws *Workspace, disableSIMD bool) bool {
	if m <= 1 {
		return false
	}
	vMaxAbs := 0.0
	for j, vj := range v[:n] {
		if math.IsNaN(vj) || math.IsInf(vj, 0) {
			return true
		}
		av := math.Abs(vj)
		if av > cancelDualFiniteCap {
			return true
		}
		if av > vMaxAbs {
			vMaxAbs = av
		}
		if m < n && (vj > 0 || (row4col[j] < 0 && vj != 0)) {
			return true
		}
	}

	rowViol := ws.d[:m]
	clear(rowViol)
	hubMinRC := ws.dQueue[:m]
	if m < n {
		posInf := math.Inf(1)
		for i := range m {
			hubMinRC[i] = posInf
		}
	}

	matchedGap := 0.0
	maxPossViolSum := 0.0
	anyGap := false
	anyViol := false
	candCap := 2 * m
	violRow := ws.violRow[:candCap]
	violCol := ws.violCol[:candCap]
	violCount := 0
	violOverflow := false

	for i := range m {
		mj := col4row[i]
		if uint(mj) >= uint(n) {
			return true
		}
		ui := u[i]
		if math.IsNaN(ui) || math.IsInf(ui, 0) {
			return true
		}
		aui := math.Abs(ui)
		if aui > cancelDualFiniteCap {
			return true
		}
		cm := normCost[i*n+mj]
		if math.IsInf(cm, 0) || math.IsNaN(cm) || math.Abs(cm) > cancelDualFiniteCap {
			return true
		}
		vmj := v[mj]
		rcM, okM := exactReducedCost(cm, ui, vmj)
		if !okM || math.IsNaN(rcM) {
			return true
		}
		mScale := math.Abs(cm) + aui + math.Abs(vmj)
		if math.Abs(cm) > 0 && aui+math.Abs(vmj) > 0x1p20*math.Abs(cm) {
			return true
		}
		if math.Abs(rcM) > 16.0*0x1p-53*mScale {
			return true
		}
		if rcM < 0 {
			rowViol[i] = -rcM
			anyViol = true
			anyGap = true
			if violCount < len(violRow) {
				violRow[violCount] = i
				violCol[violCount] = mj
				violCount++
			} else {
				violOverflow = true
			}
		} else if rcM > 0 {
			anyGap = true
			matchedGap += rcM * (1.0 + 0x1p-48)
		}
		maxPossViolSum += 33.0 * 0x1p-53 * (aui + vMaxAbs)
	}

	candThresh := 2.0*(matchedGap+maxPossViolSum) + 64.0*float64(m)*math.SmallestNonzeroFloat64
	if math.IsInf(candThresh, 0) || math.IsNaN(candThresh) {
		return true
	}

	rowLimits := ws.rowLimits[:m]
	for i := range m {
		rl := (candThresh+0x1p-48*(math.Abs(u[i])+vMaxAbs))*(1.0+0x1p-47) + 8.0*math.SmallestNonzeroFloat64
		if math.IsInf(rl, 0) || math.IsNaN(rl) || rl > cancelRowLimitCap {
			return true
		}
		rowLimits[i] = rl
	}

	candRow := ws.candRow[:candCap]
	candCol := ws.candCol[:candCap]
	candRC := ws.candRC[:candCap]
	candCount := 0
	candOverflow := false
	scanBuf := ws.scanBuf[:cancelScanBufLen]

	scanCap := max(cancelScanBufLen, 4*m)
	scanRow := ws.scanRow[:scanCap]
	scanCol := ws.scanCol[:scanCap]
	scanCnt, scanOverflow := simd.ScanNearTightMatrix64With(m, n, normCost, u, v, rowLimits, col4row, scanRow, scanCol, disableSIMD)

	if !scanOverflow {
		for k := range scanCnt {
			i := scanRow[k]
			j := scanCol[k]
			ui := u[i]
			aui := math.Abs(ui)
			c := normCost[i*n+j]
			vj := v[j]
			rc, ok := exactReducedCost(c, ui, vj)
			if !ok || math.IsNaN(rc) {
				return true
			}
			if rc < 0 {
				vMag := -rc
				if vMag > 16.0*0x1p-53*(math.Abs(c)+aui+math.Abs(vj)) {
					return true
				}
				if vMag > rowViol[i] {
					rowViol[i] = vMag
				}
				anyViol = true
				anyGap = true
				if violCount < len(violRow) {
					violRow[violCount] = i
					violCol[violCount] = j
					violCount++
				} else {
					violOverflow = true
				}
			}
			if rc <= candThresh {
				dst := row4col[j]
				if dst < 0 {
					if m == n {
						return true
					}
					if rc < hubMinRC[i] {
						hubMinRC[i] = rc
					}
				} else if dst == i {
					return true
				} else if candCount < len(candRow) {
					candRow[candCount] = i
					candCol[candCount] = dst
					candRC[candCount] = rc
					candCount++
				} else {
					candOverflow = true
				}
			}
		}
	} else {
		var okScan bool
		candCount, violCount, candOverflow, violOverflow, anyViol, okScan = scanCancelRowsFallback(
			m, n, normCost, u, v, rowLimits, col4row, row4col,
			rowViol, hubMinRC, candRow, candCol, candRC, violRow, violCol, scanBuf,
			violCount, violOverflow, anyViol, candThresh, disableSIMD,
		)
		if !okScan {
			return true
		}
		if anyViol {
			anyGap = true
		}
	}
	if !anyGap {
		return false
	}

	gapBound := matchedGap
	if anyViol {
		for i := range m {
			vi := rowViol[i]
			if vi > 0 {
				if vi < 0x1p-60*math.Abs(u[i]) {
					return true
				}
				gapBound += vi * (1.0 + 0x1p-48)
			}
		}
	}

	gapBound = gapBound*(1.0+float64(m)*0x1p-48) + float64(m)*math.SmallestNonzeroFloat64
	if math.IsInf(gapBound, 0) || math.IsNaN(gapBound) {
		return true
	}

	// Build near-tight residual graph H on vertices 0..V-1 (V = m for square, m+1 for rectangular)
	// using preallocated workspace slices and test acyclicity with zero-allocation Kahn topo sort.
	numVerts := m
	if m < n {
		numVerts = m + 1
	}
	edgeFrom := ws.freeRows[:m]
	edgeTo := ws.auxRows[:m]
	numEdges := 0
	edgeThresh := gapBound*(1.0+0x1p-48) + 4.0*math.SmallestNonzeroFloat64
	if edgeThresh > candThresh {
		return true
	}

	if !candOverflow {
		for k := range candCount {
			src := candRow[k]
			if candRC[k]+rowViol[src] > edgeThresh {
				continue
			}
			if numEdges >= m {
				return true
			}
			edgeFrom[numEdges] = src
			edgeTo[numEdges] = candCol[k]
			numEdges++
		}
		if m < n {
			for i := range m {
				if hubMinRC[i]+rowViol[i] <= edgeThresh {
					if numEdges >= m {
						return true
					}
					edgeFrom[numEdges] = i
					edgeTo[numEdges] = m
					numEdges++
				}
			}
		}
	} else {
		var ok bool
		numEdges, ok = buildCancelEdgesFallback(m, n, normCost, u, v, col4row, row4col, rowViol, edgeFrom, edgeTo, scanBuf, vMaxAbs, edgeThresh, disableSIMD)
		if !ok {
			return true
		}
	}

	head := ws.pred[:numVerts]
	inDeg := ws.remaining[:numVerts]
	for idx := range numVerts {
		head[idx] = -1
		inDeg[idx] = 0
	}
	for e := range numEdges {
		src := edgeFrom[e]
		dst := edgeTo[e]
		edgeFrom[e] = head[src]
		head[src] = e
		inDeg[dst]++
	}
	if m < n {
		for k := range m {
			if -v[col4row[k]] <= edgeThresh {
				inDeg[k]++
			}
		}
	}

	qHead := -1
	for idx := range numVerts {
		if inDeg[idx] == 0 {
			inDeg[idx] = -(qHead + 2)
			qHead = idx
		}
	}
	visited := 0
	for qHead >= 0 {
		cur := qHead
		qHead = -inDeg[cur] - 2
		visited++
		if m < n && cur == m {
			for k := range m {
				if -v[col4row[k]] <= edgeThresh {
					inDeg[k]--
					if inDeg[k] == 0 {
						inDeg[k] = -(qHead + 2)
						qHead = k
					}
				}
			}
			continue
		}
		for e := head[cur]; e >= 0; e = edgeFrom[e] {
			dst := edgeTo[e]
			inDeg[dst]--
			if inDeg[dst] == 0 {
				inDeg[dst] = -(qHead + 2)
				qHead = dst
			}
		}
	}
	if visited != numVerts {
		return true
	}

	if anyViol {
		if !violOverflow {
			candUArr := ws.dQueue[:m]
			steppedRow := ws.remaining[:m]
			for i := range m {
				vi := rowViol[i]
				if vi > 0 {
					candUArr[i] = clampFinite(u[i] - vi)
					steppedRow[i] = 0
				}
			}
			for k := range violCount {
				i := violRow[k]
				j := violCol[k]
				c := normCost[i*n+j]
				vj := v[j]
				cu := candUArr[i]
				rc, ok := exactReducedCost(c, cu, vj)
				if !ok || math.IsNaN(rc) {
					return true
				}
				if rc < 0 {
					if steppedRow[i] != 0 {
						return true
					}
					cu = clampFinite(math.Nextafter(cu, math.Inf(-1)))
					candUArr[i] = cu
					steppedRow[i] = 1
					if rc2, ok2 := exactReducedCost(c, cu, vj); !ok2 || math.IsNaN(rc2) || rc2 < 0 {
						return true
					}
				}
			}
			for i := range m {
				if rowViol[i] > 0 {
					u[i] = candUArr[i]
				}
			}
		} else if !projectCancelViolFallback(m, n, normCost, u, v, rowViol, scanBuf, vMaxAbs, disableSIMD) {
			return true
		}
	}
	return false
}

//go:noinline
func scanCancelRowsFallback(
	m, n int,
	normCost, u, v, rowLimits []float64,
	col4row, row4col []int,
	rowViol, hubMinRC []float64,
	candRow, candCol []int,
	candRC []float64,
	violRow, violCol, scanBuf []int,
	violCount int,
	violOverflow, anyViol bool,
	candThresh float64,
	disableSIMD bool,
) (candCount, outViolCount int, candOverflow, outViolOverflow, outAnyViol, ok bool) {
	outViolCount = violCount
	outViolOverflow = violOverflow
	outAnyViol = anyViol
	for i := range m {
		ui := u[i]
		aui := math.Abs(ui)
		mj := col4row[i]
		vi := rowViol[i]
		rowLimit := rowLimits[i]
		rowSlice := normCost[i*n : (i+1)*n]
		for pos := 0; pos < n; {
			next, cnt := simd.ScanNearTightRow64With(rowSlice, v, pos, ui, rowLimit, scanBuf, disableSIMD)
			for k := range cnt {
				j := scanBuf[k]
				if j == mj {
					continue
				}
				c := rowSlice[j]
				vj := v[j]
				rc, okRC := exactReducedCost(c, ui, vj)
				if !okRC || math.IsNaN(rc) {
					return 0, 0, false, false, false, false
				}
				if rc < 0 {
					vMag := -rc
					if vMag > 16.0*0x1p-53*(math.Abs(c)+aui+math.Abs(vj)) {
						return 0, 0, false, false, false, false
					}
					if vMag > vi {
						vi = vMag
					}
					outAnyViol = true
					if outViolCount < len(violRow) {
						violRow[outViolCount] = i
						violCol[outViolCount] = j
						outViolCount++
					} else {
						outViolOverflow = true
					}
				}
				if rc <= candThresh {
					dst := row4col[j]
					if dst < 0 {
						if m == n {
							return 0, 0, false, false, false, false
						}
						if rc < hubMinRC[i] {
							hubMinRC[i] = rc
						}
					} else if dst == i {
						return 0, 0, false, false, false, false
					} else if candCount < len(candRow) {
						candRow[candCount] = i
						candCol[candCount] = dst
						candRC[candCount] = rc
						candCount++
					} else {
						candOverflow = true
					}
				}
			}
			pos = next
		}
		rowViol[i] = vi
	}
	return candCount, outViolCount, candOverflow, outViolOverflow, outAnyViol, true
}

//go:noinline
func buildCancelEdgesFallback(m, n int, normCost, u, v []float64, col4row, row4col []int, rowViol []float64, edgeFrom, edgeTo, scanBuf []int, vMaxAbs, edgeThresh float64, disableSIMD bool) (int, bool) {
	numEdges := 0
	for i := range m {
		ui := u[i]
		aui := math.Abs(ui)
		mj := col4row[i]
		vi := rowViol[i]
		addedHub := false
		rowLimit := (edgeThresh+0x1p-48*(aui+vMaxAbs))*(1.0+0x1p-47) + 8.0*math.SmallestNonzeroFloat64
		rowSlice := normCost[i*n : (i+1)*n]
		for pos := 0; pos < n; {
			next, cnt := simd.ScanNearTightRow64With(rowSlice, v, pos, ui, rowLimit, scanBuf, disableSIMD)
			for k := range cnt {
				j := scanBuf[k]
				if j == mj {
					continue
				}
				rc, ok := exactReducedCost(rowSlice[j], ui, v[j])
				if !ok || math.IsNaN(rc) {
					return 0, false
				}
				if rc+vi > edgeThresh {
					continue
				}
				dst := row4col[j]
				if dst < 0 {
					if addedHub {
						continue
					}
					addedHub = true
					dst = m
				}
				if dst == i || numEdges >= m {
					return 0, false
				}
				edgeFrom[numEdges] = i
				edgeTo[numEdges] = dst
				numEdges++
			}
			pos = next
		}
	}
	return numEdges, true
}

//go:noinline
func projectCancelViolFallback(m, n int, normCost, u, v, rowViol []float64, scanBuf []int, vMaxAbs float64, disableSIMD bool) bool {
	for i := range m {
		vi := rowViol[i]
		if vi <= 0 {
			continue
		}
		candU := clampFinite(u[i] - vi)
		rowLimit := 0x1p-48*(math.Abs(candU)+vMaxAbs)*(1.0+0x1p-47) + 8.0*math.SmallestNonzeroFloat64
		rowSlice := normCost[i*n : (i+1)*n]
		stepped := false
		for pos := 0; pos < n; {
			next, cnt := simd.ScanNearTightRow64With(rowSlice, v, pos, candU, rowLimit, scanBuf, disableSIMD)
			for k := range cnt {
				j := scanBuf[k]
				c := rowSlice[j]
				rc, ok := exactReducedCost(c, candU, v[j])
				if !ok || math.IsNaN(rc) {
					return false
				}
				if rc < 0 {
					if stepped {
						return false
					}
					candU = clampFinite(math.Nextafter(candU, math.Inf(-1)))
					stepped = true
					if rc2, ok2 := exactReducedCost(c, candU, v[j]); !ok2 || math.IsNaN(rc2) || rc2 < 0 {
						return false
					}
				}
			}
			pos = next
		}
		u[i] = candU
	}
	return true
}

func compareAssignmentObj(m, n int, candCol4Row, baseCol4Row []int, unscaledCost []float64) int {
	var diffAcc assignWideAcc
	for i := range m {
		jc := candCol4Row[i]
		jb := baseCol4Row[i]
		if jc < 0 || jc >= n {
			return 1
		}
		if jc == jb {
			continue
		}
		cc := unscaledCost[i*n+jc]
		if math.IsInf(cc, 1) || math.IsNaN(cc) {
			return 1
		}
		diffAcc.addVal(cc)
		diffAcc.addVal(-unscaledCost[i*n+jb])
	}
	if diffAcc.nonFin {
		return 1
	}
	return diffAcc.sign()
}

func hasTinyEntry(cost []float64) bool {
	for _, c := range cost {
		if c != 0 && math.Abs(c) < 1.0 {
			return true
		}
	}
	return false
}

func downscaleSlice(orig, dst []float64, downExp int) bool {
	scaleDown := math.Ldexp(1.0, -downExp)
	lostSub := false
	for idx, c := range orig {
		sc := c * scaleDown
		dst[idx] = sc
		if !lostSub && c != 0 && !math.IsInf(c, 0) && (math.Ldexp(sc, downExp) != c || math.Abs(c) < 0x1p950) {
			lostSub = true
		}
	}
	return lostSub
}

func scanCostExtremeAndInt(vals []float64, intLim float64, checkInt bool) (maxAbsFinite float64, allInt bool, err error) {
	allInt = checkInt
	for _, c := range vals {
		if !(math.Abs(c) <= hugeCostThreshold) {
			if math.IsNaN(c) || math.IsInf(c, -1) {
				return 0, false, ErrInvalidCost
			}
			if !math.IsInf(c, 1) {
				allInt = false
				if ac := math.Abs(c); ac > maxAbsFinite {
					maxAbsFinite = ac
				}
			}
		} else if allInt && !isExactSmallInt(c, intLim) {
			allInt = false
		}
	}
	return maxAbsFinite, allInt, nil
}

type preparedCost struct {
	m, n          int
	normCost      []float64
	unscaledCost  []float64
	transposed    bool
	lostSubnormal bool
	allSmallInt   bool
	scaleExp      int
}

func validateAndPrepareCost(rows, cols int, cost []float64, ownedInWork bool, sense Sense, ws *Workspace, disableSIMD bool) (preparedCost, error) {
	negate := sense == Maximize
	transposed := rows > cols
	m := min(rows, cols)
	n := max(rows, cols)
	maxAbsFinite := 0.0
	intLim := exactSmallIntLimit(n)
	checkInt := probeExactSmallInt(cost[:rows*cols], intLim, negate)

	if !transposed && !negate {
		if checkInt {
			if ok, allInt := simd.AllAbsLECheckInt64With(cost, hugeCostThreshold, intLim, disableSIMD); ok {
				return preparedCost{m: m, n: n, normCost: cost, unscaledCost: cost, allSmallInt: allInt}, nil
			}
		} else if simd.AllAbsLE64With(cost, hugeCostThreshold, disableSIMD) {
			return preparedCost{m: m, n: n, normCost: cost, unscaledCost: cost}, nil
		}
		var allInt bool
		var err error
		maxAbsFinite, allInt, err = scanCostExtremeAndInt(cost, intLim, checkInt)
		if err != nil {
			return preparedCost{}, err
		}
		downExp := minimalDownscaleExp(maxAbsFinite, n)
		if downExp == 0 {
			return preparedCost{
				m:             m,
				n:             n,
				normCost:      cost,
				unscaledCost:  cost,
				lostSubnormal: maxAbsFinite > hugeCostThreshold && hasTinyEntry(cost),
				allSmallInt:   allInt && !(maxAbsFinite > hugeCostThreshold),
			}, nil
		}
		orig := cost
		var dst []float64
		if ownedInWork {
			orig = slices.Clone(cost)
			dst = cost
		} else {
			dst = ws.ensureWorkCost(rows * cols)
		}
		lostSub := downscaleSlice(orig, dst, downExp)
		return preparedCost{m: m, n: n, normCost: dst, unscaledCost: orig, lostSubnormal: lostSub, scaleExp: downExp}, nil
	}

	if !transposed && negate {
		var dst []float64
		if ownedInWork {
			dst = cost
		} else {
			dst = ws.ensureWorkCost(rows * cols)
		}
		if checkInt {
			if ok, allInt := simd.ValidateAndNegateCheckInt64With(dst, cost, hugeCostThreshold, intLim, disableSIMD); ok {
				return preparedCost{m: m, n: n, normCost: dst, unscaledCost: dst, allSmallInt: allInt}, nil
			}
		} else if simd.ValidateAndNegate64With(dst, cost, hugeCostThreshold, disableSIMD) {
			return preparedCost{m: m, n: n, normCost: dst, unscaledCost: dst}, nil
		}
		var allInt bool
		var err error
		maxAbsFinite, allInt, err = scanCostExtremeAndInt(dst, intLim, checkInt)
		if err != nil {
			return preparedCost{}, err
		}
		downExp := minimalDownscaleExp(maxAbsFinite, n)
		if downExp > 0 {
			orig := slices.Clone(dst)
			lostSub := downscaleSlice(orig, dst, downExp)
			return preparedCost{m: m, n: n, normCost: dst, unscaledCost: orig, lostSubnormal: lostSub, scaleExp: downExp}, nil
		}
		return preparedCost{
			m:             m,
			n:             n,
			normCost:      dst,
			unscaledCost:  dst,
			lostSubnormal: maxAbsFinite > hugeCostThreshold && hasTinyEntry(dst),
			allSmallInt:   allInt && !(maxAbsFinite > hugeCostThreshold),
		}, nil
	}

	// Transposed (rows > cols): m = cols, n = rows.
	// If ownedInWork is true, cost aliases ws.workCost[:total], so preserve a reference
	// to the source slice before growing ws.workCost to 2*total.
	total := rows * cols
	src := cost[:total]
	var dst []float64
	if ownedInWork {
		buf := ws.ensureWorkCost(2 * total)
		if &buf[0] != &src[0] {
			copy(buf[:total], src)
		}
		src = buf[:total]
		dst = buf[total : 2*total]
	} else {
		dst = ws.ensureWorkCost(total)
	}

	if checkInt {
		if ok, allInt := simd.AllAbsLECheckInt64With(src, hugeCostThreshold, intLim, disableSIMD); ok {
			simd.Transpose64With(dst, src, rows, cols, negate, disableSIMD)
			return preparedCost{m: m, n: n, normCost: dst, unscaledCost: dst, transposed: true, allSmallInt: allInt}, nil
		}
	} else if simd.AllAbsLE64With(src, hugeCostThreshold, disableSIMD) {
		simd.Transpose64With(dst, src, rows, cols, negate, disableSIMD)
		return preparedCost{m: m, n: n, normCost: dst, unscaledCost: dst, transposed: true}, nil
	}

	allInt := checkInt
	const tile = 16
	if negate {
		for r0 := 0; r0 < rows; r0 += tile {
			rLim := min(rows, r0+tile)
			for c0 := 0; c0 < cols; c0 += tile {
				cLim := min(cols, c0+tile)
				for r := r0; r < rLim; r++ {
					rowSlice := src[r*cols : (r+1)*cols]
					for c := c0; c < cLim; c++ {
						val := rowSlice[c]
						if !(math.Abs(val) <= hugeCostThreshold) {
							if math.IsNaN(val) || math.IsInf(val, 1) {
								return preparedCost{}, ErrInvalidCost
							}
							if !math.IsInf(val, -1) {
								allInt = false
								if av := math.Abs(val); av > maxAbsFinite {
									maxAbsFinite = av
								}
							}
						} else if allInt && !isExactSmallInt(val, intLim) {
							allInt = false
						}
						dst[c*rows+r] = 0.0 - val
					}
				}
			}
		}
	} else {
		for r0 := 0; r0 < rows; r0 += tile {
			rLim := min(rows, r0+tile)
			for c0 := 0; c0 < cols; c0 += tile {
				cLim := min(cols, c0+tile)
				for r := r0; r < rLim; r++ {
					rowSlice := src[r*cols : (r+1)*cols]
					for c := c0; c < cLim; c++ {
						val := rowSlice[c]
						if !(math.Abs(val) <= hugeCostThreshold) {
							if math.IsNaN(val) || math.IsInf(val, -1) {
								return preparedCost{}, ErrInvalidCost
							}
							if !math.IsInf(val, 1) {
								allInt = false
								if av := math.Abs(val); av > maxAbsFinite {
									maxAbsFinite = av
								}
							}
						} else if allInt && !isExactSmallInt(val, intLim) {
							allInt = false
						}
						dst[c*rows+r] = val
					}
				}
			}
		}
	}
	downExp := minimalDownscaleExp(maxAbsFinite, n)
	if downExp > 0 {
		orig := slices.Clone(dst)
		lostSub := downscaleSlice(orig, dst, downExp)
		return preparedCost{m: m, n: n, normCost: dst, unscaledCost: orig, transposed: true, lostSubnormal: lostSub, scaleExp: downExp}, nil
	}
	return preparedCost{
		m:             m,
		n:             n,
		normCost:      dst,
		unscaledCost:  dst,
		transposed:    true,
		lostSubnormal: maxAbsFinite > hugeCostThreshold && hasTinyEntry(dst),
		allSmallInt:   allInt && !(maxAbsFinite > hugeCostThreshold),
	}, nil
}

func solveWithWorkspace(ctx context.Context, rows, cols int, cost []float64, ownedInWork bool, sense Sense, ws *Workspace, cfg Options) (Result, error) {
	k := min(rows, cols)
	rowInd := make([]int, k)
	colInd := make([]int, k)

	obj, transposed, err := solveCore(ctx, rows, cols, cost, ownedInWork, sense, rowInd, colInd, ws, cfg)
	if err != nil {
		return Result{}, err
	}

	uOut := make([]float64, rows)
	vOut := make([]float64, cols)
	if !transposed {
		copy(uOut, ws.u[:rows])
		copy(vOut, ws.v[:cols])
	} else {
		copy(uOut, ws.v[:rows])
		copy(vOut, ws.u[:cols])
	}

	return Result{
		RowInd:    rowInd,
		ColInd:    colInd,
		Objective: obj,
		U:         uOut,
		V:         vOut,
	}, nil
}

func solveIntoPreallocated(ctx context.Context, rows, cols int, cost []float64, ownedInWork bool, sense Sense, rowInd, colInd []int, ws *Workspace, cfg Options) (float64, error) {
	obj, _, err := solveCore(ctx, rows, cols, cost, ownedInWork, sense, rowInd, colInd, ws, cfg)
	return obj, err
}

func colReduceValidateSquare(n int, cost, v []float64, row4col []int, intLim float64, checkInt, disableSIMD bool, done <-chan struct{}) (ok, allInt bool, err error) {
	if !checkInt {
		if done == nil || n <= 256 {
			return simd.ColReduceValidateRows64With(n, 0, n, cost, v, row4col, hugeCostThreshold, disableSIMD), false, nil
		}
		for r0 := 0; r0 < n; r0 += 256 {
			if r0 > 0 {
				select {
				case <-done:
					return false, false, errContextCanceled
				default:
				}
			}
			if !simd.ColReduceValidateRows64With(n, r0, min(n, r0+256), cost, v, row4col, hugeCostThreshold, disableSIMD) {
				return false, false, nil
			}
		}
		return true, false, nil
	}
	if done == nil || n <= 256 {
		ok, allInt = simd.ColReduceValidateCheckIntRows64With(n, 0, n, cost, v, row4col, hugeCostThreshold, intLim, disableSIMD)
		return ok, allInt, nil
	}
	allInt = true
	for r0 := 0; r0 < n; r0 += 256 {
		if r0 > 0 {
			select {
			case <-done:
				return false, false, errContextCanceled
			default:
			}
		}
		var chunkOK, chunkInt bool
		if allInt {
			chunkOK, chunkInt = simd.ColReduceValidateCheckIntRows64With(n, r0, min(n, r0+256), cost, v, row4col, hugeCostThreshold, intLim, disableSIMD)
			allInt = chunkInt
		} else {
			chunkOK = simd.ColReduceValidateRows64With(n, r0, min(n, r0+256), cost, v, row4col, hugeCostThreshold, disableSIMD)
		}
		if !chunkOK {
			return false, false, nil
		}
	}
	return true, allInt, nil
}

const (
	sparseCoreMinNScalar = 1000
	sparseCoreMinNSIMD   = 1500
	sparseCoreMinFree    = 48
	sparseCoreK          = 24
)

func wrapContextCanceled(ctx context.Context, err error) error {
	if errors.Is(err, errContextCanceled) {
		return ctx.Err()
	}
	return err
}

func augmentFreeRows(ctx context.Context, numFree, n int, normCost, u, v []float64, col4row, row4col []int, ws *Workspace, disableSIMD bool, done <-chan struct{}) error {
	if numFree == 0 {
		return nil
	}
	remaining := ws.remaining[:n]
	d := ws.d[:n]
	dQueue := ws.dQueue[:n]
	pred := ws.pred[:n]
	freeRows := ws.freeRows[:numFree]
	useSIMD := !disableSIMD && simd.UsingHardwareSIMD() && n >= 32
	if done == nil {
		if useSIMD {
			for _, curRow := range freeRows {
				if err := augmentRowSIMD(curRow, n, normCost, u, v, d, dQueue, pred, col4row, row4col, remaining, nil); err != nil {
					return err
				}
			}
			return nil
		}
		for _, curRow := range freeRows {
			if err := augmentRowScalar(curRow, n, normCost, u, v, d, pred, col4row, row4col, remaining, nil); err != nil {
				return err
			}
		}
		return nil
	}
	for idx, curRow := range freeRows {
		if idx&3 == 0 {
			select {
			case <-done:
				return ctx.Err()
			default:
			}
		}
		var err error
		if useSIMD {
			err = augmentRowSIMD(curRow, n, normCost, u, v, d, dQueue, pred, col4row, row4col, remaining, done)
		} else {
			err = augmentRowScalar(curRow, n, normCost, u, v, d, pred, col4row, row4col, remaining, done)
		}
		if err != nil {
			return wrapContextCanceled(ctx, err)
		}
	}
	return nil
}

func runNormalizedLSAP(ctx context.Context, m, n int, normCost, u, v []float64, col4row, row4col []int, ws *Workspace, disableSIMD, colReduced bool, done <-chan struct{}) error {
	for i := range m {
		col4row[i] = -1
	}
	if !colReduced {
		for j := range n {
			row4col[j] = -1
		}
	}

	var numFree int
	var errPre error
	if m == n {
		numFree, errPre = preprocessSquareJV(n, normCost, u, v, col4row, row4col, ws.freeRows[:m], ws.auxRows[:m], disableSIMD, colReduced, done)
	} else {
		numFree, errPre = preprocessRectangularBV(m, n, normCost, u, v, col4row, row4col, ws.freeRows[:m], ws.auxRows[:m], disableSIMD, done)
	}
	if errPre != nil {
		return wrapContextCanceled(ctx, errPre)
	}
	if numFree == 0 {
		return nil
	}

	useSIMD := !disableSIMD && simd.UsingHardwareSIMD()
	minN := sparseCoreMinNScalar
	if useSIMD {
		minN = sparseCoreMinNSIMD
	}
	if m == n && n >= minN && numFree >= sparseCoreMinFree && !hasDenseTiesOnFreeRows(n, normCost, u, v, ws.freeRows[:numFree], sparseCoreK) {
		return wrapContextCanceled(ctx, runSparseCorePostARR(ctx, n, normCost, u, v, col4row, row4col, numFree, ws, disableSIMD, done))
	}

	return augmentFreeRows(ctx, numFree, n, normCost, u, v, col4row, row4col, ws, disableSIMD, done)
}

func solveCore(ctx context.Context, rows, cols int, cost []float64, ownedInWork bool, sense Sense, rowInd, colInd []int, ws *Workspace, cfg Options) (float64, bool, error) {
	var done <-chan struct{}
	if ctx != nil {
		done = ctx.Done()
		if done != nil {
			select {
			case <-done:
				return 0, false, ctx.Err()
			default:
			}
		}
	}

	var (
		prep       preparedCost
		colReduced bool
		err        error
	)

	if rows == cols && rows > 1 && sense == Minimize {
		ws.ensure(rows, cols)
		intLim := exactSmallIntLimit(cols)
		checkInt := probeExactSmallInt(cost[:rows*cols], intLim, false)
		ok, allInt, crErr := colReduceValidateSquare(cols, cost, ws.v[:cols], ws.row4col[:cols], intLim, checkInt, cfg.DisableSIMD, done)
		if crErr != nil {
			if errors.Is(crErr, errContextCanceled) {
				return 0, false, ctx.Err()
			}
			return 0, false, crErr
		}
		if ok {
			prep = preparedCost{
				m:            rows,
				n:            cols,
				normCost:     cost,
				unscaledCost: cost,
				allSmallInt:  allInt,
			}
			colReduced = true
		} else {
			prep, err = validateAndPrepareCost(rows, cols, cost, ownedInWork, sense, ws, cfg.DisableSIMD)
			if err != nil {
				return 0, false, err
			}
		}
	} else {
		prep, err = validateAndPrepareCost(rows, cols, cost, ownedInWork, sense, ws, cfg.DisableSIMD)
		if err != nil {
			return 0, false, err
		}
		ws.ensure(prep.m, prep.n)
	}

	m, n := prep.m, prep.n
	normCost, unscaledCost := prep.normCost, prep.unscaledCost
	transposed, lostSubnormal, allSmallInt := prep.transposed, prep.lostSubnormal, prep.allSmallInt
	scaleExp := prep.scaleExp

	u := ws.u[:m]
	v := ws.v[:n]
	col4row := ws.col4row[:m]
	row4col := ws.row4col[:n]

	// Fast path for m == 1 (single row or single column).
	if m == 1 {
		minVal := math.Inf(1)
		bestJ := -1
		for j, val := range unscaledCost[:n] {
			if val < minVal {
				minVal = val
				bestJ = j
			}
		}
		if bestJ == -1 || math.IsInf(minVal, 1) {
			return 0, transposed, ErrInfeasible
		}
		u[0] = clampFinite(minVal)
		clear(v)
		if !transposed {
			rowInd[0] = 0
			colInd[0] = bestJ
		} else {
			rowInd[0] = bestJ
			colInd[0] = 0
		}
		obj := minVal
		if sense == Maximize {
			obj = -obj
		}
		if obj == 0 {
			obj = 0
		}
		return obj, transposed, nil
	}

	if err := runNormalizedLSAP(ctx, m, n, normCost, u, v, col4row, row4col, ws, cfg.DisableSIMD, colReduced, done); err != nil {
		return 0, transposed, err
	}

	var (
		normObj      float64
		hasPotCancel bool
	)
	if scaleExp == 0 && !lostSubnormal && allSmallInt {
		if exactObj, ok := verifyExactSmallIntStage1(m, n, normCost, u, v, col4row, row4col, exactSmallIntLimit(n)); ok {
			normObj = exactObj
		} else {
			normObj = computeMatchedObjective(m, n, normCost, col4row, true)
			hasPotCancel = hasPotentialCancellation(m, n, normCost, u, v, col4row, row4col, ws, cfg.DisableSIMD)
		}
	} else {
		normObj = computeMatchedObjective(m, n, normCost, col4row, scaleExp == 0)
		hasPotCancel = hasPotentialCancellation(m, n, normCost, u, v, col4row, row4col, ws, cfg.DisableSIMD)
	}
	var (
		st             stage2State
		stage2Accepted bool
	)
	if lostSubnormal || hasPotCancel {
		s2 := stage2Problem{
			m:            m,
			n:            n,
			scaleExp:     scaleExp,
			normCost:     normCost,
			unscaledCost: unscaledCost,
			u:            u,
			v:            v,
			col4row:      col4row,
			row4col:      row4col,
			ws:           ws,
			disableSIMD:  cfg.DisableSIMD,
		}
		st, stage2Accepted, err = runStage2Refinement(ctx, s2, done)
		if err != nil {
			return 0, transposed, err
		}
	}

	// Extract primal assignment in ascending row order and reuse Stage-1 objective when valid.
	if !transposed {
		for i := range m {
			rowInd[i] = i
			colInd[i] = col4row[i]
		}
	} else {
		outIdx := 0
		for origRow := range n {
			origCol := row4col[origRow]
			if origCol >= 0 {
				rowInd[outIdx] = origRow
				colInd[outIdx] = origCol
				outIdx++
			}
		}
	}
	var totalMin float64
	if !lostSubnormal && scaleExp == 0 && !stage2Accepted {
		totalMin = normObj
	} else {
		totalMin = computeMatchedObjective(m, n, unscaledCost, col4row, scaleExp == 0)
	}

	if scaleExp > 0 || st.numDualPasses > 0 || hasPotCancel {
		s2 := stage2Problem{
			m:            m,
			n:            n,
			scaleExp:     scaleExp,
			normCost:     normCost,
			unscaledCost: unscaledCost,
			u:            u,
			v:            v,
			col4row:      col4row,
			row4col:      row4col,
			ws:           ws,
			disableSIMD:  cfg.DisableSIMD,
		}
		finalizeStage2Duals(s2, &st, hasPotCancel)
	}

	obj := totalMin
	if sense == Maximize {
		obj = -obj
	}
	if obj == 0 {
		obj = 0
	}
	return obj, transposed, nil
}

const maxStage2Passes = 8

type stage2State struct {
	uBase         []float64
	vBase         []float64
	uDeltas       []float64
	vDeltas       []float64
	vBonus        []float64
	passShift     [maxStage2Passes]int
	numDualPasses int
}

func evalStage2Pot(acc *assignWideAcc, base float64, scaleExp int, deltas, bonus []float64, stride, idx, numPasses int, passShift []int, extraShift int) (float64, int) {
	acc.reset()
	acc.addScaled(base, scaleExp+extraShift)
	for p := range numPasses {
		s := passShift[p] + extraShift
		acc.addScaled(deltas[p*stride+idx], s)
		if bonus != nil {
			acc.addScaled(bonus[p*stride+idx], s)
		}
	}
	return acc.eval()
}

func evalStage2Edge(acc *assignWideAcc, m, n, scaleExp, i, j int, normCost, unscaledCost []float64, st *stage2State, extraShift int) (float64, int) {
	idx := i*n + j
	oc := unscaledCost[idx]
	if st.numDualPasses == 0 && extraShift == 0 {
		if scaleExp == 0 {
			s1, e1 := twoSumAssign(oc, -st.uBase[i])
			s2, e2 := twoSumAssign(s1, -st.vBase[j])
			if !math.IsInf(s1, 0) && !math.IsInf(s2, 0) {
				d := s2 + (e1 + e2)
				return d, assignSignOf(d)
			}
		} else {
			nc := normCost[idx]
			if math.Ldexp(nc, scaleExp) == oc {
				s1, e1 := twoSumAssign(nc, -st.uBase[i])
				s2, e2 := twoSumAssign(s1, -st.vBase[j])
				if !math.IsInf(s1, 0) && !math.IsInf(s2, 0) {
					dScaled := s2 + (e1 + e2)
					return math.Ldexp(dScaled, scaleExp), assignSignOf(dScaled)
				}
			}
		}
	}
	acc.reset()
	acc.addScaled(oc, extraShift)
	acc.addScaled(-st.uBase[i], scaleExp+extraShift)
	acc.addScaled(-st.vBase[j], scaleExp+extraShift)
	for p := range st.numDualPasses {
		s := st.passShift[p] + extraShift
		acc.addScaled(-st.uDeltas[p*m+i], s)
		acc.addScaled(-st.vDeltas[p*n+j], s)
		if st.vBonus != nil {
			acc.addScaled(-st.vBonus[p*n+j], s)
		}
	}
	return acc.eval()
}

type stage2Problem struct {
	m            int
	n            int
	scaleExp     int
	normCost     []float64
	unscaledCost []float64
	u            []float64
	v            []float64
	col4row      []int
	row4col      []int
	ws           *Workspace
	disableSIMD  bool
}

func runStage2Refinement(ctx context.Context, p stage2Problem, done <-chan struct{}) (stage2State, bool, error) {
	m, n, scaleExp := p.m, p.n, p.scaleExp
	normCost, unscaledCost := p.normCost, p.unscaledCost
	u, v := p.u, p.v
	col4row, row4col := p.col4row, p.row4col
	ws, disableSIMD := p.ws, p.disableSIMD
	tau := 4.0 * float64(m+1) * math.SmallestNonzeroFloat64
	subCap := float64(m+2) * tau
	st := stage2State{
		uBase:   make([]float64, m),
		vBase:   make([]float64, n),
		uDeltas: make([]float64, maxStage2Passes*m),
		vDeltas: make([]float64, maxStage2Passes*n),
	}
	if m < n {
		st.vBonus = make([]float64, maxStage2Passes*n)
	}
	if scaleExp > 0 {
		for i, ui := range u {
			if math.Abs(ui) > subCap {
				st.uBase[i] = ui
			}
		}
		if m == n {
			for j, vj := range v {
				if math.Abs(vj) > subCap {
					st.vBase[j] = vj
				}
			}
		} else {
			for j, vj := range v {
				if vj < -subCap {
					st.vBase[j] = vj
				}
			}
		}
	} else {
		copy(st.uBase, u)
		copy(st.vBase, v)
	}

	uApprox := make([]float64, m)
	uApproxAbs := make([]float64, m)
	vApprox := make([]float64, n)
	vApproxAbs := make([]float64, n)
	resCost := make([]float64, m*n)
	u2 := make([]float64, m)
	v2 := make([]float64, n)
	c4r2 := make([]int, m)
	r4c2 := make([]int, n)
	var colBonus []float64
	if m < n {
		colBonus = make([]float64, n)
	}
	capRC := math.MaxFloat64 / (64.0 * float64(m+2) * float64(m+2))

	var (
		acc                          assignWideAcc
		stage2Accepted               bool
		prevPassWasDualOnly          bool
		prevPassHadPrimalImprovement bool
		savedCutCap                  float64
		prevCutCap                   float64
	)

	for pass := range maxStage2Passes {
		for i := range m {
			uiVal, _ := evalStage2Pot(&acc, st.uBase[i], scaleExp, st.uDeltas, nil, m, i, st.numDualPasses, st.passShift[:], 0)
			uApprox[i] = uiVal
			uApproxAbs[i] = math.Abs(uiVal)
		}
		unassignedNegV := 0.0
		assignedPosV := 0.0
		anyUnassignedNegV := false
		anyPosV := false
		for j := range n {
			vjVal, vjSgn := evalStage2Pot(&acc, st.vBase[j], scaleExp, st.vDeltas, st.vBonus, n, j, st.numDualPasses, st.passShift[:], 0)
			vApprox[j] = vjVal
			vApproxAbs[j] = math.Abs(vjVal)
			if m < n {
				if row4col[j] < 0 && vjSgn < 0 {
					anyUnassignedNegV = true
					unassignedNegV += -vjVal
				}
				if vjSgn > 0 {
					anyPosV = true
					if row4col[j] >= 0 {
						assignedPosV += vjVal
					}
				}
			}
		}

		s1Pos := 0.0
		maxNegD := 0.0
		anyMatchedPos := false
		anyNegD := false
		for i := range m {
			uiApprox := uApprox[i]
			uiAbs := uApproxAbs[i]
			uiFin := !math.IsInf(uiApprox, 0)
			matchedJ := col4row[i]
			rowOff := i * n
			for j := range n {
				idx := rowOff + j
				oc := unscaledCost[idx]
				if math.IsInf(oc, 1) {
					resCost[idx] = math.Inf(1)
					continue
				}
				if st.numDualPasses > 0 && j != matchedJ && uiFin && !math.IsInf(vApprox[j], 0) {
					t1 := oc - uiApprox
					est := t1 - vApprox[j]
					margin := 16.0*0x1p-53*(math.Abs(oc)+uiAbs+vApproxAbs[j]) + 16.0*math.SmallestNonzeroFloat64
					if !math.IsInf(t1, 0) && !math.IsInf(est, 0) && !math.IsNaN(est) && est > margin {
						resCost[idx] = math.NaN()
						continue
					}
				}
				d, sgn := evalStage2Edge(&acc, m, n, scaleExp, i, j, normCost, unscaledCost, &st, 0)
				resCost[idx] = d
				if j == matchedJ && sgn > 0 {
					anyMatchedPos = true
					s1Pos += d
				}
				if sgn < 0 {
					anyNegD = true
					if -d > maxNegD {
						maxNegD = -d
					}
				}
			}
		}

		cutCap := s1Pos + float64(m)*maxNegD + unassignedNegV + assignedPosV
		if prevPassWasDualOnly && (anyMatchedPos || anyNegD || anyUnassignedNegV || anyPosV) && !(cutCap < 0.5*savedCutCap) {
			st.numDualPasses--
			break
		}
		if !anyMatchedPos && !anyNegD && !anyUnassignedNegV && !anyPosV {
			break
		}
		if pass >= 2 && !prevPassHadPrimalImprovement && !(cutCap < 0.5*prevCutCap) {
			break
		}
		prevCutCap = cutCap

		thresh := max(2.0*cutCap, 4.0*math.SmallestNonzeroFloat64)
		maxCand := 0.0
		for i := range m {
			uiApprox := uApprox[i]
			uiAbs := uApproxAbs[i]
			uiFin := !math.IsInf(uiApprox, 0)
			rowOff := i * n
			for j := range n {
				idx := rowOff + j
				d := resCost[idx]
				if math.IsInf(d, 1) {
					continue
				}
				if math.IsNaN(d) {
					oc := unscaledCost[idx]
					if uiFin && !math.IsInf(vApprox[j], 0) {
						t1 := oc - uiApprox
						est := t1 - vApprox[j]
						margin := 16.0*0x1p-53*(math.Abs(oc)+uiAbs+vApproxAbs[j]) + 16.0*math.SmallestNonzeroFloat64
						if !math.IsInf(t1, 0) && !math.IsInf(est, 0) && !math.IsNaN(est) && est > thresh+margin {
							resCost[idx] = math.Inf(1)
							continue
						}
					}
					d, _ = evalStage2Edge(&acc, m, n, scaleExp, i, j, normCost, unscaledCost, &st, 0)
					resCost[idx] = d
				}
				if d > thresh {
					resCost[idx] = math.Inf(1)
					continue
				}
				ad := math.Abs(d)
				if math.IsInf(ad, 1) {
					ad = math.MaxFloat64
				}
				if ad > maxCand {
					maxCand = ad
				}
			}
		}

		liftExp := 64
		if maxCand > 0 {
			_, maxE := math.Frexp(maxCand)
			_, capE := math.Frexp(capRC)
			liftExp = max(0, min(64, capE-maxE-1))
		}
		minNegRC := 0.0
		for i := range m {
			rowOff := i * n
			for j := range n {
				idx := rowOff + j
				d := resCost[idx]
				if math.IsInf(d, 1) {
					continue
				}
				rc := d
				if liftExp > 0 {
					if a := math.Abs(d); a >= 0x1p-950 && !math.IsInf(a, 0) {
						rc = math.Ldexp(d, liftExp)
					} else {
						rc, _ = evalStage2Edge(&acc, m, n, scaleExp, i, j, normCost, unscaledCost, &st, liftExp)
					}
				}
				if rc > capRC {
					rc = capRC
				} else if rc < -capRC {
					rc = -capRC
				} else if math.IsNaN(rc) {
					rc = 0
				}
				resCost[idx] = rc
				if rc < minNegRC {
					minNegRC = rc
				}
			}
		}

		if m < n {
			clear(colBonus)
			stage1PosSum := 0.0
			for i := range m {
				if j := col4row[i]; uint(j) < uint(n) {
					if rc := resCost[i*n+j]; rc > 0 && !math.IsInf(rc, 1) {
						stage1PosSum += rc
					}
				}
			}
			swing := stage1PosSum - float64(m)*minNegRC + math.Ldexp(unassignedNegV+assignedPosV, liftExp)
			if math.IsInf(swing, 0) || math.IsNaN(swing) {
				swing = capRC
			}
			mustBonusCap := min(capRC, 4.0*swing+math.Ldexp(8.0*subCap, scaleExp+liftExp))
			if !(mustBonusCap > 0) || math.IsNaN(mustBonusCap) {
				mustBonusCap = math.SmallestNonzeroFloat64
			}
			for j := range n {
				totVJ, totSgn := evalStage2Pot(&acc, st.vBase[j], scaleExp, st.vDeltas, st.vBonus, n, j, st.numDualPasses, st.passShift[:], liftExp)
				if totSgn >= 0 {
					continue
				}
				b := mustBonusCap
				if !math.IsInf(totVJ, -1) && -totVJ < b {
					b = -totVJ
				}
				colBonus[j] = b
				for i := range m {
					idx := i*n + j
					if !math.IsInf(resCost[idx], 1) {
						resCost[idx] -= b
					}
				}
			}
		}

		if err2 := runNormalizedLSAP(ctx, m, n, resCost, u2, v2, c4r2, r4c2, ws, disableSIMD, false, done); err2 != nil {
			if errors.Is(err2, context.Canceled) || errors.Is(err2, context.DeadlineExceeded) {
				return st, stage2Accepted, err2
			}
			break
		}
		finiteDuals := true
		for i := range m {
			if math.IsNaN(u2[i]) || math.IsInf(u2[i], 0) {
				finiteDuals = false
				break
			}
		}
		if finiteDuals {
			for j := range n {
				if math.IsNaN(v2[j]) || math.IsInf(v2[j], 0) {
					finiteDuals = false
					break
				}
			}
		}
		if !finiteDuals {
			break
		}

		cmp := compareAssignmentObj(m, n, c4r2, col4row, unscaledCost)
		recordDualPass := func() {
			p := st.numDualPasses
			copy(st.uDeltas[p*m:(p+1)*m], u2)
			copy(st.vDeltas[p*n:(p+1)*n], v2)
			if st.vBonus != nil {
				copy(st.vBonus[p*n:(p+1)*n], colBonus)
			}
			st.passShift[p] = -liftExp
			st.numDualPasses++
		}
		if cmp <= 0 {
			stage2Accepted = true
			copy(col4row, c4r2)
			copy(row4col, r4c2)
			recordDualPass()
			prevPassHadPrimalImprovement = cmp < 0
			prevPassWasDualOnly = false
		} else {
			if pass+1 < maxStage2Passes {
				recordDualPass()
				prevPassWasDualOnly = true
				prevPassHadPrimalImprovement = false
				savedCutCap = cutCap
			} else {
				break
			}
		}
	}
	return st, stage2Accepted, nil
}

func computeSquareDualShift(minU, maxU, minV, maxV, mLim float64) float64 {
	deltaLo := max(-mLim-minU, maxV-mLim)
	deltaHi := min(mLim-maxU, minV+mLim)
	if !(0.0 >= deltaLo && 0.0 <= deltaHi) {
		return 0.5 * (deltaLo + deltaHi)
	}
	return 0.0
}

func finalizeStage2Duals(p stage2Problem, st *stage2State, hasPotCancel bool) {
	m, n, scaleExp := p.m, p.n, p.scaleExp
	unscaledCost := p.unscaledCost
	row4col := p.row4col
	u, v := p.u, p.v
	if scaleExp > 0 || st.numDualPasses > 0 {
		anyClamped := false
		if st.numDualPasses > 0 {
			var acc assignWideAcc
			shift := 0.0
			shiftExp := scaleExp + 2
			if m == n {
				u0, _ := evalStage2Pot(&acc, st.uBase[0], scaleExp, st.uDeltas, nil, m, 0, st.numDualPasses, st.passShift[:], -shiftExp)
				minU, maxU := u0, u0
				for i := 1; i < m; i++ {
					ui, _ := evalStage2Pot(&acc, st.uBase[i], scaleExp, st.uDeltas, nil, m, i, st.numDualPasses, st.passShift[:], -shiftExp)
					minU = min(minU, ui)
					maxU = max(maxU, ui)
				}
				v0, _ := evalStage2Pot(&acc, st.vBase[0], scaleExp, st.vDeltas, nil, n, 0, st.numDualPasses, st.passShift[:], -shiftExp)
				minV, maxV := v0, v0
				for j := 1; j < n; j++ {
					vj, _ := evalStage2Pot(&acc, st.vBase[j], scaleExp, st.vDeltas, nil, n, j, st.numDualPasses, st.passShift[:], -shiftExp)
					minV = min(minV, vj)
					maxV = max(maxV, vj)
				}
				shift = computeSquareDualShift(minU, maxU, minV, maxV, math.Ldexp(math.MaxFloat64, -shiftExp))
			}
			for i := range m {
				acc.reset()
				acc.addScaled(st.uBase[i], scaleExp)
				acc.addScaled(shift, shiftExp)
				for pass := range st.numDualPasses {
					acc.addScaled(st.uDeltas[pass*m+i], st.passShift[pass])
				}
				scaled, _ := acc.eval()
				clamped := clampFinite(scaled)
				if clamped != scaled {
					anyClamped = true
				}
				u[i] = clamped
			}
			for j := range n {
				if m < n && row4col[j] < 0 {
					v[j] = 0
					continue
				}
				acc.reset()
				acc.addScaled(st.vBase[j], scaleExp)
				acc.addScaled(-shift, shiftExp)
				for pass := range st.numDualPasses {
					acc.addScaled(st.vDeltas[pass*n+j], st.passShift[pass])
					if st.vBonus != nil {
						acc.addScaled(st.vBonus[pass*n+j], st.passShift[pass])
					}
				}
				scaled, _ := acc.eval()
				clamped := clampFinite(scaled)
				if m < n && clamped > 0 {
					clamped = 0
				}
				if clamped != scaled {
					anyClamped = true
				}
				v[j] = clamped
			}
		} else {
			shift := 0.0
			if m == n {
				minU, maxU := u[0], u[0]
				for _, ui := range u[1:] {
					minU = min(minU, ui)
					maxU = max(maxU, ui)
				}
				minV, maxV := v[0], v[0]
				for _, vj := range v[1:] {
					minV = min(minV, vj)
					maxV = max(maxV, vj)
				}
				shift = computeSquareDualShift(minU, maxU, minV, maxV, math.Ldexp(math.MaxFloat64, -scaleExp))
			}
			for i := range m {
				scaled := math.Ldexp(u[i]+shift, scaleExp)
				clamped := clampFinite(scaled)
				if clamped != scaled {
					anyClamped = true
				}
				u[i] = clamped
			}
			for j := range n {
				if m < n && row4col[j] < 0 {
					v[j] = 0
					continue
				}
				scaled := math.Ldexp(v[j]-shift, scaleExp)
				clamped := clampFinite(scaled)
				if m < n && clamped > 0 {
					clamped = 0
				}
				if clamped != scaled {
					anyClamped = true
				}
				v[j] = clamped
			}
		}
		if anyClamped {
			if m == n {
				for i := range m {
					rowSlice := unscaledCost[i*n : (i+1)*n]
					for j, c := range rowSlice {
						if c < 0 && math.IsInf(c-v[j], -1) {
							v[j] = min(v[j], c+math.MaxFloat64)
						}
					}
				}
			}
			for i := range m {
				rowSlice := unscaledCost[i*n : (i+1)*n]
				ui := u[i]
				for j, c := range rowSlice {
					if math.IsInf(c, 1) {
						continue
					}
					lim, rem := twoSumAssign(c, -v[j])
					if rem < 0 && !math.IsInf(lim, 0) {
						lim = math.Nextafter(lim, math.Inf(-1))
					}
					if !math.IsInf(lim, 1) && lim < ui {
						ui = clampFinite(lim)
					}
				}
				u[i] = ui
			}
		} else if st.numDualPasses > 0 || hasPotCancel {
			polishStage2Duals(p)
		}
	} else if hasPotCancel {
		polishStage2Duals(p)
	}
}

func adjustStage2Dual(cur, delta float64) float64 {
	sum, errTerm := twoSumAssign(cur, delta)
	next := clampFinite(sum)
	if errTerm < 0 && math.Abs(errTerm) >= 0x1p-60*math.Abs(next) && !math.IsInf(sum, 0) {
		next = clampFinite(math.Nextafter(next, math.Inf(-1)))
	}
	return next
}

func rowNearTightLimit(ui, vMaxAbs float64) float64 {
	return 0x1p-48*(math.Abs(ui)+vMaxAbs)*(1.0+0x1p-47) + 8.0*math.SmallestNonzeroFloat64
}

func polishStage2Duals(p stage2Problem) {
	m, n := p.m, p.n
	unscaledCost := p.unscaledCost
	col4row, row4col := p.col4row, p.row4col
	u, v := p.u, p.v
	ws, disableSIMD := p.ws, p.disableSIMD
	var pi []float64
	var rootToRow []float64
	var scanBuf []int
	if ws != nil && len(ws.scanBuf) > 0 {
		scanBuf = ws.scanBuf
	} else {
		var stackBuf [cancelScanBufLen]int
		scanBuf = stackBuf[:]
	}
	vFilterBound := func() (float64, bool) {
		vMax := 0.0
		for _, vj := range v[:n] {
			if math.IsNaN(vj) || math.IsInf(vj, 0) {
				return 0, false
			}
			av := math.Abs(vj)
			if av > cancelDualFiniteCap {
				return 0, false
			}
			if av > vMax {
				vMax = av
			}
		}
		return vMax, true
	}
	for range 3 {
		for i := range m {
			j := col4row[i]
			if uint(j) >= uint(n) {
				return
			}
			cij := unscaledCost[i*n+j]
			if math.IsInf(cij, 0) || math.IsNaN(cij) {
				return
			}
			if lim := cij - v[j]; !math.IsNaN(lim) && !math.IsInf(lim, 0) && lim != u[i] {
				rcCur, okCur := exactReducedCost(cij, u[i], v[j])
				rcLim, okLim := exactReducedCost(cij, lim, v[j])
				if !okCur || (okLim && math.Abs(rcLim) < math.Abs(rcCur)) {
					u[i] = lim
				}
			}
		}
		vMaxAbs, canFilter := vFilterBound()
		needPolish := false
		for i := range m {
			ui := u[i]
			rowSlice := unscaledCost[i*n : (i+1)*n]
			if canFilter && !math.IsNaN(ui) && !math.IsInf(ui, 0) && math.Abs(ui) <= cancelDualFiniteCap {
				rowLimit := rowNearTightLimit(ui, vMaxAbs)
				for pos := 0; pos < n && !needPolish; {
					next, cnt := simd.ScanNearTightRow64With(rowSlice, v, pos, ui, rowLimit, scanBuf, disableSIMD)
					for k := range cnt {
						j := scanBuf[k]
						rc, ok := exactReducedCost(rowSlice[j], ui, v[j])
						if !ok || math.IsNaN(rc) || rc < 0 {
							needPolish = true
							break
						}
					}
					pos = next
				}
			} else {
				for j, cij := range rowSlice {
					if math.IsInf(cij, 1) {
						continue
					}
					rc, ok := exactReducedCost(cij, ui, v[j])
					if !ok || rc < 0 {
						needPolish = true
						break
					}
				}
			}
			if needPolish {
				break
			}
		}
		if !needPolish {
			return
		}

		if pi == nil {
			pi = make([]float64, m+1)
			if m < n {
				rootToRow = make([]float64, m)
			}
		} else {
			clear(pi)
		}
		if m < n {
			for i := range m {
				best := math.Inf(1)
				ui := u[i]
				rowSlice := unscaledCost[i*n : (i+1)*n]
				for j, cij := range rowSlice {
					if row4col[j] >= 0 || math.IsInf(cij, 1) {
						continue
					}
					s, e := twoSumAssign(cij, -ui)
					if g := s + e; g < best {
						best = g
					}
				}
				rootToRow[i] = best
			}
		}

		for range m + 1 {
			changed := false
			if m < n {
				pm := pi[m]
				for i, g := range rootToRow {
					if math.IsInf(g, 1) {
						continue
					}
					if cand := pm + g; !math.IsNaN(cand) && !math.IsInf(cand, 0) && cand < pi[i] {
						pi[i] = cand
						changed = true
					}
				}
				for k := range m {
					j := col4row[k]
					if cand := pi[k] - v[j]; !math.IsNaN(cand) && !math.IsInf(cand, 0) && cand < pi[m] {
						pi[m] = cand
						changed = true
					}
				}
			}
			for k := range m {
				j := col4row[k]
				pk := pi[k]
				vj := v[j]
				for i := range m {
					if i == k {
						continue
					}
					cij := unscaledCost[i*n+j]
					if math.IsInf(cij, 1) {
						continue
					}
					rc, ok := exactReducedCost(cij, u[i], vj)
					if !ok {
						continue
					}
					cand := pk + rc
					if !math.IsNaN(cand) && !math.IsInf(cand, 0) && cand < pi[i] {
						pi[i] = cand
						changed = true
					}
				}
			}
			if !changed {
				break
			}
		}

		base := 0.0
		if m < n {
			base = pi[m]
		}
		for i := range m {
			delta := pi[i] - base
			if math.IsNaN(delta) || math.IsInf(delta, 0) || delta == 0 {
				continue
			}
			j := col4row[i]
			newU := adjustStage2Dual(u[i], delta)
			newV := adjustStage2Dual(v[j], -delta)
			if m < n && newV > 0 {
				newV = 0
				newU = clampFinite(unscaledCost[i*n+j])
			}
			u[i] = newU
			v[j] = newV
		}
	}
	// Final single-ULP downward projection when residual dual violation is on u[i]'s ULP scale
	// (avoids touching extreme exponent spreads where viol << 2^-60 * |u[i]|).
	vMaxAbs, canFilter := vFilterBound()
	for i := range m {
		ui := u[i]
		if math.IsNaN(ui) || math.IsInf(ui, 0) {
			continue
		}
		worstViol := 0.0
		worstJ := -1
		rowSlice := unscaledCost[i*n : (i+1)*n]
		if canFilter && math.Abs(ui) <= cancelDualFiniteCap {
			rowLimit := rowNearTightLimit(ui, vMaxAbs)
			for pos := 0; pos < n; {
				next, cnt := simd.ScanNearTightRow64With(rowSlice, v, pos, ui, rowLimit, scanBuf, disableSIMD)
				for k := range cnt {
					j := scanBuf[k]
					rc, ok := exactReducedCost(rowSlice[j], ui, v[j])
					if ok && rc < 0 && -rc > worstViol {
						worstViol = -rc
						worstJ = j
					}
				}
				pos = next
			}
		} else {
			for j, cij := range rowSlice {
				if math.IsInf(cij, 1) {
					continue
				}
				rc, ok := exactReducedCost(cij, ui, v[j])
				if ok && rc < 0 && -rc > worstViol {
					worstViol = -rc
					worstJ = j
				}
			}
		}
		if worstJ >= 0 && worstViol >= 0x1p-60*math.Abs(ui) {
			candU := clampFinite(ui - worstViol)
			if rc2, ok2 := exactReducedCost(rowSlice[worstJ], candU, v[worstJ]); ok2 && rc2 < 0 {
				candU = clampFinite(math.Nextafter(candU, math.Inf(-1)))
			}
			u[i] = candU
		}
	}
}

func initSquareColReduction(n int, cost, u, v []float64, col4row, row4col []int, disableSIMD, colReduced bool, done <-chan struct{}) error {
	u = u[:n]
	v = v[:n]
	col4row = col4row[:n]
	row4col = row4col[:n]
	clear(u)

	if !colReduced {
		if done == nil || n <= 256 {
			simd.ColReduceRows64With(n, 0, n, cost, v, row4col, disableSIMD)
		} else {
			for r0 := 0; r0 < n; r0 += 256 {
				if r0 > 0 {
					select {
					case <-done:
						return errContextCanceled
					default:
					}
				}
				simd.ColReduceRows64With(n, r0, min(n, r0+256), cost, v, row4col, disableSIMD)
			}
		}
	}

	for j, vj := range v {
		if math.IsInf(vj, 1) {
			return ErrInfeasible
		}
		i := row4col[j]
		if uint(i) >= uint(n) {
			continue
		}
		if col4row[i] == -1 {
			col4row[i] = j
		} else {
			if col4row[i] >= 0 {
				col4row[i] = -2 - col4row[i]
			}
			row4col[j] = -1
		}
	}
	return nil
}

// preprocessSquareJV executes Jonker-Volgenant (1987) Column Reduction (in row-major order),
// Reduction Transfer, and 2-pass Augmenting Row Reduction for square n x n instances.
// When colReduced is true, v and row4col already hold the column minima and argmin rows.
func preprocessSquareJV(n int, cost, u, v []float64, col4row, row4col, freeRows, auxRows []int, disableSIMD, colReduced bool, done <-chan struct{}) (int, error) {
	if n <= 0 || len(u) < n || len(v) < n || len(col4row) < n || len(row4col) < n || len(freeRows) < n || len(auxRows) < n {
		return 0, nil
	}
	if err := initSquareColReduction(n, cost, u, v, col4row, row4col, disableSIMD, colReduced, done); err != nil {
		return 0, err
	}
	return finishSquareJVReduction(n, cost, u, v, col4row, row4col, freeRows, auxRows, disableSIMD, done)
}

func applySquareReductionTransfer(n int, cost, u, v []float64, col4row, freeRows []int, disableSIMD bool, done <-chan struct{}) (int, error) {
	numFree := 0
	for i, c := range col4row {
		if done != nil && n > 256 && i&255 == 0 {
			select {
			case <-done:
				return 0, errContextCanceled
			default:
			}
		}
		if c == -1 {
			freeRows[numFree] = i
			numFree++
		} else if c < -1 {
			// Contended row: restore original matched column; u[i] stays 0.
			col4row[i] = -2 - c
		} else if uint(c) < uint(n) {
			// Uniquely matched row i to column c: transfer dual slack from second-best column.
			rowCost := cost[i*n:][:n]
			minRed := simd.MinReducedSkip64With(rowCost, v, c, disableSIMD)
			if !math.IsInf(minRed, 1) {
				u[i] = minRed
				v[c] = rowCost[c] - minRed
			}
		}
	}
	return numFree, nil
}

func initCollinearSquareJV(n int, cost, u, v []float64, col4row, row4col, freeRows, auxRows []int, disableSIMD bool, done <-chan struct{}) (int, error) {
	r0 := freeRows[0]
	row0 := cost[r0*n : (r0+1)*n]
	jMin, jMax := 0, 0
	cMin, cMax := row0[0], row0[0]
	for j := 1; j < n; j++ {
		if c := row0[j]; !math.IsInf(c, 0) {
			if c < cMin || math.IsInf(cMin, 0) {
				cMin = c
				jMin = j
			}
			if c > cMax || math.IsInf(cMax, 0) {
				cMax = c
				jMax = j
			}
		}
	}
	for i := range n {
		col4row[i] = -1
		row4col[i] = -1
		freeRows[i] = i
		u[i] = cost[i*n+jMax] - cost[i*n+jMin]
	}
	for i := 1; i < n; i++ {
		r := freeRows[i]
		sr := u[r]
		j := i - 1
		for j >= 0 && u[freeRows[j]] < sr {
			freeRows[j+1] = freeRows[j]
			j--
		}
		freeRows[j+1] = r
	}
	clear(u)
	for k := range n {
		if done != nil && n > 256 && k&255 == 0 {
			select {
			case <-done:
				return 0, errContextCanceled
			default:
			}
		}
		r := freeRows[k]
		rowCost := cost[r*n : (r+1)*n]
		bestJ := -1
		bestC := math.Inf(1)
		for j, c := range rowCost {
			if row4col[j] == -1 && c < bestC {
				bestC = c
				bestJ = j
			}
		}
		if bestJ == -1 {
			for j := range n {
				if row4col[j] == -1 {
					bestJ = j
					break
				}
			}
		}
		auxRows[k] = bestJ
		row4col[bestJ] = r
	}
	v[auxRows[n-1]] = 0
	for k := n - 1; k >= 1; k-- {
		r := freeRows[k]
		ck := auxRows[k]
		ckPrev := auxRows[k-1]
		diff := cost[r*n+ck] - cost[r*n+ckPrev]
		if math.IsNaN(diff) || math.IsInf(diff, 0) || diff < 0 {
			diff = 0
		}
		v[ckPrev] = v[ck] - diff
	}
	for j := range n {
		row4col[j] = -1
	}
	return augmentingRowReduction(n, cost, u, v, col4row, row4col, freeRows, auxRows, n, disableSIMD, done)
}

func reinitRowDominatedSquareJV(n int, cost, u, v []float64, col4row, row4col, freeRows []int, disableSIMD bool, done <-chan struct{}) (int, error) {
	// Non-collinear row-dominated matrix (1-2 cheap rows hid all other rows during
	// column reduction): normalize row scales via row minima u[i] = min_j cost[i, j],
	// then re-run column reduction and reduction transfer on (cost[i, j] - u[i]).
	for i := range n {
		if done != nil && n > 256 && i&255 == 0 {
			select {
			case <-done:
				return 0, errContextCanceled
			default:
			}
		}
		rowCost := cost[i*n : (i+1)*n]
		minRow := rowCost[0]
		for _, c := range rowCost[1:] {
			if c < minRow {
				minRow = c
			}
		}
		if math.IsInf(minRow, 1) {
			return 0, ErrInfeasible
		}
		u[i] = minRow
		col4row[i] = -1
	}
	u0 := u[0]
	for j, c := range cost[:n] {
		v[j] = c - u0
		row4col[j] = 0
	}
	for i := 1; i < n; i++ {
		if done != nil && n > 256 && i&255 == 0 {
			select {
			case <-done:
				return 0, errContextCanceled
			default:
			}
		}
		ui := u[i]
		rowCost := cost[i*n : (i+1)*n]
		for j, c := range rowCost {
			if r := c - ui; r < v[j] {
				v[j] = r
				row4col[j] = i
			}
		}
	}
	for j := range n {
		i := row4col[j]
		if col4row[i] == -1 {
			col4row[i] = j
		} else {
			if col4row[i] >= 0 {
				col4row[i] = -2 - col4row[i]
			}
			row4col[j] = -1
		}
	}
	return applySquareReductionTransfer(n, cost, u, v, col4row, freeRows, disableSIMD, done)
}

func finishSquareJVReduction(n int, cost, u, v []float64, col4row, row4col, freeRows, auxRows []int, disableSIMD bool, done <-chan struct{}) (int, error) {
	u = u[:n]
	v = v[:n]
	col4row = col4row[:n]
	row4col = row4col[:n]
	freeRows = freeRows[:n]
	auxRows = auxRows[:n]

	// Apply Reduction Transfer on uniquely matched rows and collect unassigned rows into freeRows.
	numFree, err := applySquareReductionTransfer(n, cost, u, v, col4row, freeRows, disableSIMD, done)
	if err != nil || numFree == 0 {
		return 0, err
	}
	// When column reduction matches <= max(2, n/32) rows, almost all rows and columns
	// remain unassigned because 1-2 rows dominated every column.
	if numFree >= n-max(2, n/32) {
		if isCollinearSquare(n, numFree, cost, freeRows) {
			return initCollinearSquareJV(n, cost, u, v, col4row, row4col, freeRows, auxRows, disableSIMD, done)
		}
		numFree, err = reinitRowDominatedSquareJV(n, cost, u, v, col4row, row4col, freeRows, disableSIMD, done)
		if err != nil || numFree == 0 {
			return 0, err
		}
	}
	return augmentingRowReduction(n, cost, u, v, col4row, row4col, freeRows, auxRows, numFree, disableSIMD, done)
}

// isCollinearSquare checks in O(n) whether unassigned rows share a common column
// preference order (as in rank-1 / Monge matrices) rather than independent random bulk
// hidden behind a few globally cheap rows or columns.
func isCollinearSquare(n, numFree int, cost []float64, freeRows []int) bool {
	if numFree < 4 || n < 8 {
		return false
	}
	r0 := freeRows[0]
	r1 := freeRows[numFree/3]
	r2 := freeRows[(2*numFree)/3]
	r3 := freeRows[numFree-1]
	row0 := cost[r0*n : (r0+1)*n]
	row1 := cost[r1*n : (r1+1)*n]
	row2 := cost[r2*n : (r2+1)*n]
	row3 := cost[r3*n : (r3+1)*n]

	step := max(1, n/32)
	agree, total := 0, 0
	for j := 0; j+step < n; j += step {
		d0 := row0[j+step] - row0[j]
		d1 := row1[j+step] - row1[j]
		d2 := row2[j+step] - row2[j]
		d3 := row3[j+step] - row3[j]
		if math.IsInf(d0, 0) || math.IsNaN(d0) ||
			math.IsInf(d1, 0) || math.IsNaN(d1) ||
			math.IsInf(d2, 0) || math.IsNaN(d2) ||
			math.IsInf(d3, 0) || math.IsNaN(d3) {
			continue
		}
		total += 3
		if (d0 >= 0) == (d1 >= 0) {
			agree++
		}
		if (d0 >= 0) == (d2 >= 0) {
			agree++
		}
		if (d0 >= 0) == (d3 >= 0) {
			agree++
		}
	}
	return total >= 12 && agree*5 >= total*4
}

// preprocessRectangularBV executes Bijsterbosch-Volgenant (2010) Row Reduction and
// 2-pass Augmenting Row Reduction for rectangular m x n (m < n) instances, preserving
// v[j] == 0 on all unassigned columns.
func preprocessRectangularBV(m, n int, cost, u, v []float64, col4row, row4col, freeRows, auxRows []int, disableSIMD bool, done <-chan struct{}) (int, error) {
	clear(v)
	numFree := 0

	for i := range m {
		if done != nil && n > 256 && i&255 == 0 {
			select {
			case <-done:
				return 0, errContextCanceled
			default:
			}
		}
		rowCost := cost[i*n : (i+1)*n]
		u1, u2, j1, j2 := simd.Min2ReducedRow64With(rowCost, v, disableSIMD)
		if math.IsInf(u1, 1) {
			return 0, ErrInfeasible
		}
		u[i] = u1
		if row4col[j1] == -1 {
			col4row[i] = j1
			row4col[j1] = i
		} else if j2 >= 0 && u1 == u2 && row4col[j2] == -1 {
			col4row[i] = j2
			row4col[j2] = i
		} else {
			freeRows[numFree] = i
			numFree++
		}
	}

	if numFree == 0 {
		return 0, nil
	}
	return augmentingRowReduction(n, cost, u, v, col4row, row4col, freeRows, auxRows, numFree, disableSIMD, done)
}

// augmentingRowReduction performs up to 2 passes of Jonker-Volgenant / Bijsterbosch-Volgenant
// augmenting row reduction over the unassigned rows in freeRows[:numFree].
func augmentingRowReduction(n int, cost, u, v []float64, col4row, row4col, freeRows, auxRows []int, numFree int, disableSIMD bool, done <-chan struct{}) (int, error) {
	if n <= 0 || len(v) < n || len(row4col) < n || len(freeRows) < numFree || len(auxRows) < numFree {
		return 0, nil
	}
	v = v[:n]
	row4col = row4col[:n]
	maxRowHops := 16
	maxTransfers := 2 * n
	if n >= 1000 {
		maxRowHops = 64
		maxTransfers = 6 * n
	}
	for range 2 {
		if numFree == 0 {
			break
		}
		nextFree := 0
		transfers := 0
		transfersSinceLastMatch := 0
		for rowIdx, i := range freeRows[:numFree] {
			if done != nil && n > 256 && rowIdx&127 == 0 {
				select {
				case <-done:
					return 0, errContextCanceled
				default:
				}
			}
			rowHops := 0
			for {
				rowCost := cost[i*n:][:n]
				u1, u2, j1, j2 := simd.Min2ReducedRow64With(rowCost, v, disableSIMD)

				if math.IsInf(u1, 1) {
					return 0, ErrInfeasible
				}
				if uint(j1) >= uint(n) {
					break
				}
				if math.IsInf(u2, 1) {
					// Only column j1 has a finite cost in row i; do not set u[i] = +Inf.
					u[i] = u1
					if row4col[j1] == -1 {
						col4row[i] = j1
						row4col[j1] = i
						transfersSinceLastMatch = 0
					} else {
						auxRows[nextFree] = i
						nextFree++
					}
					break
				}

				i0 := row4col[j1]
				if i0 == -1 {
					// Column j1 is unassigned: match (i, j1) and tighten u[i] without increasing v[j1].
					newVj1 := rowCost[j1] - u2
					if newVj1 < v[j1] {
						u[i] = u2
						v[j1] = newVj1
					} else {
						u[i] = u1
					}
					col4row[i] = j1
					row4col[j1] = i
					transfersSinceLastMatch = 0
					break
				}

				if u1 == u2 && uint(j2) < uint(n) && row4col[j2] == -1 {
					u[i] = u1
					col4row[i] = j2
					row4col[j2] = i
					transfersSinceLastMatch = 0
					break
				}

				if u1 < u2 && rowHops < maxRowHops && transfers < maxTransfers && transfersSinceLastMatch <= n {
					newVj1 := rowCost[j1] - u2
					if newVj1 < v[j1] {
						u[i] = u2
						v[j1] = newVj1
						col4row[i] = j1
						row4col[j1] = i
						col4row[i0] = -1
						i = i0
						rowHops++
						transfers++
						transfersSinceLastMatch++
						if done != nil && n > 256 && transfers&255 == 0 {
							select {
							case <-done:
								return 0, errContextCanceled
							default:
							}
						}
						continue
					}
				}

				u[i] = u1
				auxRows[nextFree] = i
				nextFree++
				break
			}
		}
		numFree = nextFree
		copy(freeRows[:numFree], auxRows[:numFree])
	}
	return numFree, nil
}

// initAugmentRowSIMD initializes contiguous column distances d[0:n] for curRow, marks
// tight columns at minVal as queued with NaN sentinels in d, and returns matched=true
// immediately if an unassigned column achieves minVal in one hop.
func initAugmentRowSIMD(curRow, n int, cost, u, v, d, dQueue []float64, pred, col4row, row4col, remaining []int) (minVal float64, up int, matched bool, err error) {
	if n <= 0 || len(d) < n || len(dQueue) < n || len(pred) < n || len(row4col) < n || len(remaining) < n {
		return 0, 0, false, nil
	}
	d = d[:n]
	dQueue = dQueue[:n]
	pred = pred[:n]
	row4col = row4col[:n]
	remaining = remaining[:n]
	rowCost := cost[curRow*n:][:n]
	ui := u[curRow]

	minVal = simd.InitReducedRow64With(d, rowCost, v, ui, false)
	if !(minVal < math.Inf(1)) {
		return 0, 0, false, ErrInfeasible
	}

	nan := math.NaN()
	var hits [8]int
	for pos := 0; pos < n; {
		next, cnt := simd.CollectLE64(d, pos, minVal, hits[:])
		if cnt > len(hits) {
			cnt = len(hits)
		}
		for k := range cnt {
			j := hits[k]
			if j < 0 || j >= n {
				continue
			}
			if row4col[j] == -1 {
				u[curRow] += d[j]
				col4row[curRow] = j
				row4col[j] = curRow
				return minVal, 0, true, nil
			}
			if up < n {
				remaining[up] = j
				dQueue[up] = minVal
				d[j] = nan
				up++
			}
		}
		pos = next
	}
	simd.FillInt(pred, curRow)
	return minVal, up, false, nil
}

// refillQueueContigSIMD finds the next minimum finite non-NaN distance in d[0:n], enqueues
// all achieving columns with NaN sentinels, and returns sink >= 0 if any achieving
// column is unassigned.
func refillQueueContigSIMD(d, dQueue []float64, row4col, remaining []int, up int) (minVal float64, newUp, sink int, err error) {
	n := len(d)
	if n == 0 || len(dQueue) < n || len(row4col) < n || len(remaining) < n {
		return 0, up, -1, ErrInfeasible
	}
	if up < 0 {
		up = 0
	}
	d = d[:n]
	dQueue = dQueue[:n]
	row4col = row4col[:n]
	remaining = remaining[:n]
	nan := math.NaN()
	minVal = simd.MinNoNaN64(d)
	if !(minVal < math.Inf(1)) {
		return 0, up, -1, ErrInfeasible
	}
	var hits [8]int
	for pos := 0; pos < n; {
		next, cnt := simd.CollectLE64(d, pos, minVal, hits[:])
		if cnt > len(hits) {
			cnt = len(hits)
		}
		for k := range cnt {
			jj := hits[k]
			if jj < 0 || jj >= n {
				continue
			}
			if row4col[jj] == -1 {
				return minVal, up, jj, nil
			}
			if up < n {
				remaining[up] = jj
				dQueue[up] = minVal
				d[jj] = nan
				up++
			}
		}
		pos = next
	}
	return minVal, up, -1, nil
}

// relaxRowContigSIMD relaxes contiguous d[0:n] from row rowI using the 4-wide AVX2
// branchless kernel, enqueuing tight columns and returning sink >= 0 on an unassigned hit.
func relaxRowContigSIMD(rCost, v, d, dQueue []float64, pred, row4col, remaining []int, n, rowI, up int, ui, minVal float64) (newUp, sink int) {
	if n <= 0 || len(rCost) < n || len(v) < n || len(d) < n || len(dQueue) < n || len(pred) < n || len(row4col) < n || len(remaining) < n {
		return up, -1
	}
	if up < 0 {
		up = 0
	}
	rCost = rCost[:n]
	v = v[:n]
	d = d[:n]
	dQueue = dQueue[:n]
	pred = pred[:n]
	row4col = row4col[:n]
	remaining = remaining[:n]
	nan := math.NaN()
	for j := 0; j < n; {
		j = simd.RelaxRow64(rCost, v, d, pred, j, rowI, ui, minVal)
		if j < 0 || j >= n {
			break
		}
		lim := min(j+4, n)
		for ; j < lim; j++ {
			r := minVal + ((rCost[j] - v[j]) - ui)
			if r < d[j] {
				pred[j] = rowI
				if r <= minVal {
					if row4col[j] == -1 {
						return up, j
					}
					if up < n {
						remaining[up] = j
						dQueue[up] = minVal
						d[j] = nan
						up++
					}
				} else {
					d[j] = r
				}
			}
		}
	}
	return up, -1
}

// finishAugmentPath applies dual potential shifts over expanded columns remaining[:low]
// using saved distances in dQueue[:low], then augments the primal matching along pred.
func finishAugmentPath(curRow, sink, low int, u, v, dQueue []float64, pred, col4row, row4col, remaining []int, minVal float64) {
	u[curRow] += minVal
	remLow := remaining[:low]
	dLow := dQueue[:low]
	for k, j := range remLow {
		delta := dLow[k] - minVal
		v[j] += delta
		u[row4col[j]] -= delta
	}
	for j := sink; ; {
		r := pred[j]
		row4col[j] = r
		j, col4row[r] = col4row[r], j
		if r == curRow {
			break
		}
	}
}

// augmentRowSIMD finds a shortest augmenting path from curRow to an unassigned column
// using contiguous column-indexed distances d[0:n] with NaN sentinels and AVX2 relaxation.
func augmentRowSIMD(curRow, n int, cost, u, v, d, dQueue []float64, pred, col4row, row4col, remaining []int, done <-chan struct{}) error {
	if n <= 0 || len(v) < n || len(d) < n || len(dQueue) < n || len(pred) < n || len(row4col) < n || len(remaining) < n {
		return nil
	}
	v = v[:n]
	d = d[:n]
	dQueue = dQueue[:n]
	pred = pred[:n]
	row4col = row4col[:n]
	remaining = remaining[:n]

	minVal, up, matched, err := initAugmentRowSIMD(curRow, n, cost, u, v, d, dQueue, pred, col4row, row4col, remaining)
	if err != nil || matched {
		return err
	}

	low := 0
	var sink int
	for {
		if low == up {
			var rErr error
			minVal, up, sink, rErr = refillQueueContigSIMD(d, dQueue, row4col, remaining, up)
			if rErr != nil {
				return rErr
			}
			if sink >= 0 {
				break
			}
		}

		col := remaining[low]
		low++
		if done != nil && low&127 == 0 {
			select {
			case <-done:
				return errContextCanceled
			default:
			}
		}
		i := row4col[col]
		rCost := cost[i*n:][:n]
		ui := u[i]
		up, sink = relaxRowContigSIMD(rCost, v, d, dQueue, pred, row4col, remaining, n, i, up, ui, minVal)
		if sink >= 0 {
			break
		}
	}

	finishAugmentPath(curRow, sink, low, u, v, dQueue, pred, col4row, row4col, remaining, minVal)
	return nil
}

// relaxRowIndirectScalar relaxes unscanned columns remaining[up:n] in scalar mode,
// kept separate from augmentRowScalar so register pressure never spills cancellation state.
func relaxRowIndirectScalar(rCost, v, d []float64, pred, row4col, remaining []int, n, rowI, up int, ui, minVal float64) (newUp, sink int) {
	if n <= 0 {
		return up, -1
	}
	if up < 0 {
		up = 0
	}
	rCost = rCost[:n]
	v = v[:n]
	d = d[:n]
	pred = pred[:n]
	row4col = row4col[:n]
	remaining = remaining[:n]

	for k := up; k < n; k++ {
		j := remaining[k]
		if j < 0 || j >= n {
			continue
		}
		r := minVal + ((rCost[j] - v[j]) - ui)
		if r < d[k] {
			pred[j] = rowI
			if r <= minVal {
				if row4col[j] == -1 {
					return up, j
				}
				if up < n {
					remaining[k] = remaining[up]
					remaining[up] = j
					d[k] = d[up]
					d[up] = minVal
					up++
				}
			} else {
				d[k] = r
			}
		}
	}
	return up, -1
}

// refillQueueIndirectScalar finds the next minimum finite distance in d[up:n], swaps all
// achieving columns into remaining[up:], and returns sink >= 0 on an unassigned column.
func refillQueueIndirectScalar(d []float64, row4col, remaining []int, n, up int) (minVal float64, newUp, sink int, err error) {
	if uint(up) >= uint(n) {
		return 0, up, -1, ErrInfeasible
	}
	minVal = d[up]
	minK := up
	hasTies := false
	k := up + 1
	dRem := d[k:n]
	for len(dRem) >= 4 {
		v0, v1, v2, v3 := dRem[0], dRem[1], dRem[2], dRem[3]
		if v0 <= minVal || v1 <= minVal || v2 <= minVal || v3 <= minVal {
			if v0 <= minVal {
				if v0 < minVal {
					minVal, minK, hasTies = v0, k, false
				} else {
					hasTies = true
				}
			}
			if v1 <= minVal {
				if v1 < minVal {
					minVal, minK, hasTies = v1, k+1, false
				} else {
					hasTies = true
				}
			}
			if v2 <= minVal {
				if v2 < minVal {
					minVal, minK, hasTies = v2, k+2, false
				} else {
					hasTies = true
				}
			}
			if v3 <= minVal {
				if v3 < minVal {
					minVal, minK, hasTies = v3, k+3, false
				} else {
					hasTies = true
				}
			}
		}
		dRem = dRem[4:]
		k += 4
	}
	for idx, val := range dRem {
		if val <= minVal {
			if val < minVal {
				minVal, minK, hasTies = val, k+idx, false
			} else {
				hasTies = true
			}
		}
	}
	if math.IsInf(minVal, 1) {
		return 0, up, -1, ErrInfeasible
	}
	if !hasTies {
		j := remaining[minK]
		if row4col[j] == -1 {
			return minVal, up, j, nil
		}
		remaining[minK] = remaining[up]
		remaining[up] = j
		d[minK] = d[up]
		d[up] = minVal
		return minVal, up + 1, -1, nil
	}
	for k := minK; k < n; k++ {
		if dk := d[k]; dk <= minVal {
			j := remaining[k]
			if row4col[j] == -1 {
				return minVal, up, j, nil
			}
			remaining[k] = remaining[up]
			remaining[up] = j
			d[k] = d[up]
			d[up] = dk
			up++
		}
	}
	return minVal, up, -1, nil
}

// augmentRowScalar finds a shortest augmenting path from curRow in scalar mode using
// compact remaining[up:n] indexing so already-queued columns are never rescanned.
func augmentRowScalar(curRow, n int, cost, u, v, d []float64, pred, col4row, row4col, remaining []int, done <-chan struct{}) error {
	if n <= 0 || len(v) < n || len(d) < n || len(pred) < n || len(row4col) < n || len(remaining) < n {
		return nil
	}
	v = v[:n]
	d = d[:n]
	pred = pred[:n]
	row4col = row4col[:n]
	remaining = remaining[:n]

	rowCost := cost[curRow*n:][:n]
	ui := u[curRow]

	minVal := simd.InitReducedRow64Scalar(d, rowCost, v, ui)
	if !(minVal < math.Inf(1)) {
		return ErrInfeasible
	}

	up := 0
	for j, dj := range d {
		remaining[j] = j
		pred[j] = curRow
		if dj <= minVal {
			if row4col[j] == -1 {
				u[curRow] += dj
				col4row[curRow] = j
				row4col[j] = curRow
				return nil
			}
			remaining[j] = remaining[up]
			remaining[up] = j
			d[j] = d[up]
			d[up] = dj
			up++
		}
	}

	low := 0
	var sink int
	for {
		if low == up {
			var rErr error
			minVal, up, sink, rErr = refillQueueIndirectScalar(d, row4col, remaining, n, up)
			if rErr != nil {
				return rErr
			}
			if sink >= 0 {
				break
			}
		}

		col := remaining[low]
		low++
		if done != nil && low&127 == 0 {
			select {
			case <-done:
				return errContextCanceled
			default:
			}
		}
		i := row4col[col]
		rCost := cost[i*n:][:n]
		ui := u[i]
		up, sink = relaxRowIndirectScalar(rCost, v, d, pred, row4col, remaining, n, i, up, ui, minVal)
		if sink >= 0 {
			break
		}
	}

	finishAugmentPath(curRow, sink, low, u, v, d, pred, col4row, row4col, remaining, minVal)
	return nil
}

func hasDenseTiesOnFreeRows(n int, cost, u, v []float64, freeRows []int, k int) bool {
	sample := min(len(freeRows), 4)
	for s := range sample {
		i := freeRows[s]
		rowCost := cost[i*n:][:n]
		ui := u[i]
		cnt := 0
		for j, vj := range v[:n] {
			if rowCost[j]-vj <= ui {
				cnt++
				if cnt >= k {
					return true
				}
			}
		}
	}
	return false
}

func runSparseCorePostARR(ctx context.Context, n int, cost, u, v []float64, col4row, row4col []int, numFree int, ws *Workspace, disableSIMD bool, done <-chan struct{}) error {
	ws.ensureSparseCore(n, sparseCoreK)
	if err := buildSparseCorePostARR(n, sparseCoreK, cost, v, col4row, ws.coreCol, ws.coreCost, ws.coreDeg, done); err != nil {
		return err
	}
	if err := augmentFreeRowsSparse(n, sparseCoreK, u, v, col4row, row4col, ws.freeRows[:numFree], ws, done); err != nil {
		return err
	}
	return priceAndRepairSparseCore(ctx, n, cost, u, v, col4row, row4col, ws, disableSIMD, done)
}

func replaceTopKMax(reds *[32]float64, cols *[32]int32, k int, r float64, c int32) {
	p := 0
	for {
		left := 2*p + 1
		if left >= k {
			break
		}
		best := left
		if right := left + 1; right < k && reds[right] > reds[left] {
			best = right
		}
		if !(reds[best] > r) {
			break
		}
		reds[p] = reds[best]
		cols[p] = cols[best]
		p = best
	}
	reds[p] = r
	cols[p] = c
}

func siftUpTopKMax(reds *[32]float64, cols *[32]int32, p int, r float64, c int32) {
	for p > 0 {
		parent := (p - 1) >> 1
		if !(r > reds[parent]) {
			break
		}
		reds[p] = reds[parent]
		cols[p] = cols[parent]
		p = parent
	}
	reds[p] = r
	cols[p] = c
}

func buildSparseCorePostARR(n, k int, cost, v []float64, col4row []int, coreCol []int32, coreCost []float64, coreDeg []int32, done <-chan struct{}) error {
	var topRed [32]float64
	var topCol [32]int32
	v = v[:n]
	for i := range n {
		if done != nil && i&255 == 0 {
			select {
			case <-done:
				return errContextCanceled
			default:
			}
		}
		matchedCol := col4row[i]
		rowCost := cost[i*n:][:n]
		cnt := 0
		j := 0
		for ; j < n && cnt < k; j++ {
			c := rowCost[j]
			if math.IsInf(c, 1) {
				continue
			}
			r := c - v[j]
			if math.IsInf(r, 1) {
				continue
			}
			siftUpTopKMax(&topRed, &topCol, cnt, r, int32(j))
			cnt++
		}
		if cnt == k && j < n {
			maxRed := topRed[0]
			for ; j+4 <= n; j += 4 {
				r0 := rowCost[j] - v[j]
				r1 := rowCost[j+1] - v[j+1]
				r2 := rowCost[j+2] - v[j+2]
				r3 := rowCost[j+3] - v[j+3]
				if r0 < maxRed || r1 < maxRed || r2 < maxRed || r3 < maxRed {
					if r0 < maxRed {
						replaceTopKMax(&topRed, &topCol, k, r0, int32(j))
						maxRed = topRed[0]
					}
					if r1 < maxRed {
						replaceTopKMax(&topRed, &topCol, k, r1, int32(j+1))
						maxRed = topRed[0]
					}
					if r2 < maxRed {
						replaceTopKMax(&topRed, &topCol, k, r2, int32(j+2))
						maxRed = topRed[0]
					}
					if r3 < maxRed {
						replaceTopKMax(&topRed, &topCol, k, r3, int32(j+3))
						maxRed = topRed[0]
					}
				}
			}
			for ; j < n; j++ {
				if r := rowCost[j] - v[j]; r < maxRed {
					replaceTopKMax(&topRed, &topCol, k, r, int32(j))
					maxRed = topRed[0]
				}
			}
		}
		if cnt == 0 {
			return ErrInfeasible
		}
		if matchedCol >= 0 {
			hasMatch := false
			for t := range cnt {
				if int(topCol[t]) == matchedCol {
					hasMatch = true
					break
				}
			}
			if !hasMatch && matchedCol <= math.MaxInt32 {
				topCol[0] = int32(matchedCol)
			}
		}

		base := i * k
		dstCol := coreCol[base : base+cnt]
		dstCost := coreCost[base : base+cnt]
		for t := range cnt {
			colT := topCol[t]
			dstCol[t] = colT
			dstCost[t] = rowCost[colT]
		}
		coreDeg[i] = int32(cnt)
	}
	return nil
}

func pushMinHeap(dists *[]float64, cols *[]int32, heapLen int, d float64, col int32) int {
	ds := *dists
	cs := *cols
	if heapLen >= cap(ds) {
		newCap := max(16, 2*cap(ds))
		nds := make([]float64, newCap)
		ncs := make([]int32, newCap)
		copy(nds, ds[:heapLen])
		copy(ncs, cs[:heapLen])
		ds = nds
		cs = ncs
		*dists = ds
		*cols = cs
	} else if heapLen >= len(ds) {
		ds = ds[:cap(ds)]
		cs = cs[:cap(cs)]
		*dists = ds
		*cols = cs
	}
	p := heapLen
	for p > 0 {
		parent := (p - 1) >> 1
		if !(d < ds[parent]) {
			break
		}
		ds[p] = ds[parent]
		cs[p] = cs[parent]
		p = parent
	}
	ds[p] = d
	cs[p] = col
	return heapLen + 1
}

func popMinHeap(ds []float64, cs []int32, heapLen int) (dist float64, col int, newLen int) {
	dist = ds[0]
	col = int(cs[0])
	heapLen--
	if heapLen > 0 {
		lastD := ds[heapLen]
		lastC := cs[heapLen]
		p := 0
		for {
			left := 2*p + 1
			if left >= heapLen {
				break
			}
			best := left
			if right := left + 1; right < heapLen && ds[right] < ds[left] {
				best = right
			}
			if !(ds[best] < lastD) {
				break
			}
			ds[p] = ds[best]
			cs[p] = cs[best]
			p = best
		}
		ds[p] = lastD
		cs[p] = lastC
	}
	return dist, col, heapLen
}

func augmentFreeRowsSparse(n, k int, u, v []float64, col4row, row4col, freeRows []int, ws *Workspace, done <-chan struct{}) error {
	d := ws.d[:n]
	dQueue := ws.dQueue[:n]
	pred := ws.pred[:n]
	remaining := ws.remaining[:n]
	coreCol := ws.coreCol
	coreCost := ws.coreCost
	coreDeg := ws.coreDeg
	dStamp := ws.dStamp[:n]
	closedStamp := ws.closedStamp[:n]
	u = u[:n]
	v = v[:n]
	row4col = row4col[:n]

	for idx, curRow := range freeRows {
		if done != nil && idx&7 == 0 {
			select {
			case <-done:
				return errContextCanceled
			default:
			}
		}
		if ws.stamp >= math.MaxInt32-2 {
			clear(dStamp)
			clear(closedStamp)
			ws.stamp = 0
		}
		ws.stamp++
		stamp := ws.stamp

		deg := int(coreDeg[curRow])
		if deg == 0 {
			continue
		}
		base := curRow * k
		ui := u[curRow]
		minInit := math.Inf(1)
		bestFreeVal := math.Inf(1)
		bestFreeCol := -1
		for t := range deg {
			j := int(coreCol[base+t])
			r := (coreCost[base+t] - v[j]) - ui
			dStamp[j] = stamp
			d[j] = r
			pred[j] = curRow
			if r < minInit {
				minInit = r
			}
			if row4col[j] == -1 && r < bestFreeVal {
				bestFreeVal = r
				bestFreeCol = j
			}
		}
		if !(minInit < math.Inf(1)) {
			continue
		}
		if bestFreeCol >= 0 && bestFreeVal <= minInit {
			u[curRow] += bestFreeVal
			col4row[curRow] = bestFreeCol
			row4col[bestFreeCol] = curRow
			continue
		}

		heapLen := 0
		for t := range deg {
			col32 := coreCol[base+t]
			heapLen = pushMinHeap(&ws.heapDist, &ws.heapCol, heapLen, d[col32], col32)
		}

		low := 0
		sink := -1
		minVal := 0.0
		for heapLen > 0 {
			var col int
			var dist float64
			dist, col, heapLen = popMinHeap(ws.heapDist, ws.heapCol, heapLen)
			if closedStamp[col] == stamp || dist > d[col] {
				continue
			}
			if done != nil && low&127 == 0 {
				select {
				case <-done:
					return errContextCanceled
				default:
				}
			}
			if row4col[col] == -1 {
				sink = col
				minVal = dist
				break
			}
			closedStamp[col] = stamp
			remaining[low] = col
			dQueue[low] = dist
			low++

			rowI := row4col[col]
			rDeg := int(coreDeg[rowI])
			rBase := rowI * k
			uRow := u[rowI]
			for t := range rDeg {
				col32 := coreCol[rBase+t]
				if closedStamp[col32] == stamp {
					continue
				}
				r := dist + ((coreCost[rBase+t] - v[col32]) - uRow)
				if r < dist {
					r = dist
				}
				if dStamp[col32] != stamp || r < d[col32] {
					dStamp[col32] = stamp
					d[col32] = r
					pred[col32] = rowI
					heapLen = pushMinHeap(&ws.heapDist, &ws.heapCol, heapLen, r, col32)
				}
			}
		}
		if sink >= 0 {
			finishAugmentPath(curRow, sink, low, u, v, dQueue, pred, col4row, row4col, remaining, minVal)
		}
	}
	return nil
}

func minReducedSkipScalarCheck(rowCost, v []float64, skip int) float64 {
	best := math.Inf(1)
	for j, c := range rowCost {
		if j == skip || math.IsInf(c, 1) {
			continue
		}
		if r := c - v[j]; r < best {
			best = r
		}
	}
	return best
}

func priceAndRepairSparseCore(ctx context.Context, n int, cost, u, v []float64, col4row, row4col []int, ws *Workspace, disableSIMD bool, done <-chan struct{}) error {
	u = u[:n]
	v = v[:n]
	col4row = col4row[:n]
	row4col = row4col[:n]
	freeRows := ws.freeRows[:n]
	numRepair := 0

	for i := range n {
		if done != nil && i&255 == 0 {
			select {
			case <-done:
				return errContextCanceled
			default:
			}
		}
		rowCost := cost[i*n:][:n]
		if c := col4row[i]; c >= 0 {
			ui := rowCost[c] - v[c]
			u[i] = ui
			minRed := simd.MinReducedSkip64With(rowCost, v, c, disableSIMD)
			if math.IsNaN(minRed) {
				minRed = minReducedSkipScalarCheck(rowCost, v, c)
			}
			if minRed < ui {
				col4row[i] = -1
				row4col[c] = -1
				u[i] = minRed
				freeRows[numRepair] = i
				numRepair++
			}
		} else {
			minRed := simd.MinReducedSkip64With(rowCost, v, -1, disableSIMD)
			if math.IsNaN(minRed) {
				minRed = minReducedSkipScalarCheck(rowCost, v, -1)
			}
			if math.IsInf(minRed, 1) {
				return ErrInfeasible
			}
			u[i] = minRed
			freeRows[numRepair] = i
			numRepair++
		}
	}
	if numRepair == 0 {
		return nil
	}
	return augmentFreeRows(ctx, numRepair, n, cost, u, v, col4row, row4col, ws, disableSIMD, done)
}
