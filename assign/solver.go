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
	s = a + b
	bb := s - a
	err = (a - (s - bb)) + (b - bb)
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
				for _, m := range mid {
					if math.Abs(m) < 0x1p-400 && math.Abs(m) >= 0x1p-1018 {
						sc4 = growAssignSlice(sc4, math.Ldexp(m, -4))
					}
				}
				for _, c := range half {
					sc4 = growAssignSlice(sc4, math.Ldexp(c, assignWideHalf-4))
				}
				sc4 = compressAssignSlice(sc4)
				t4, r4 := assignTopRem(sc4)
				res := math.Ldexp(t4, 4)
				if math.IsInf(res, 0) {
					if math.Abs(t4) == 0x1p1020 && (t4 > 0) != (r4 > 0) && math.Abs(r4) > 0x1p966 {
						res = math.Copysign(math.MaxFloat64, t4)
					}
					return res, assignSignOf(t4)
				}
				comb := res + math.Ldexp(r4, 4)
				if math.IsNaN(comb) {
					return res, assignSignOf(t4)
				}
				return comb, assignSignOf(t4)
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
		top, rem := assignTopRem(midMerged)
		comb := top + rem
		if math.IsNaN(comb) {
			comb = top
		}
		return comb, assignSignOf(midMerged[len(midMerged)-1])
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
		top, rem := assignTopRem(midMerged)
		comb := top + rem
		if math.IsNaN(comb) {
			comb = top
		}
		return comb, assignSignOf(midMerged[len(midMerged)-1])
	}

	if len(midMerged) == 0 {
		loTop, loRem := assignTopRem(loSmall)
		if loTop == 0 {
			return 0, 0
		}
		adj := assignRoundToOddSticky(loTop, loRem)
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
			uTop, uRem := assignTopRem(up)
			if math.Abs(uTop) >= 0x1p-493 {
				s0 := math.Ldexp(uTop, -assignWideHalf)
				e0 := math.Ldexp(uRem, -assignWideHalf)
				s, e := twoSumAssign(s0, e0)
				return s + e, assignSignOf(up[len(up)-1])
			}
			adj := assignRoundToOddSticky(uTop, uRem)
			return math.Ldexp(adj, -assignWideHalf), assignSignOf(up[len(up)-1])
		}
		rebuilt := midWork[:0]
		for _, u := range up {
			if math.Abs(u) >= 0x1p-494 {
				rebuilt = growAssignSlice(rebuilt, math.Ldexp(u, -assignWideHalf))
			}
		}
		for _, m := range topKeep {
			rebuilt = growAssignSlice(rebuilt, m)
		}
		rebuilt = compressAssignSlice(rebuilt)
		top, rem := assignTopRem(rebuilt)
		comb := top + rem
		if math.IsNaN(comb) {
			comb = top
		}
		return comb, assignSignOf(rebuilt[len(rebuilt)-1])
	}

	loTop, loRem := assignTopRem(loSmall)
	loUnscaled := math.Ldexp(math.Ldexp(assignRoundToOddSticky(loTop, loRem), -assignWideHalf), -assignWideHalf)
	r := 0.0
	for i := range len(midMerged) - 1 {
		r += midMerged[i]
	}
	if r == 0 {
		r = loUnscaled
	} else {
		r += loUnscaled
	}
	s, e := twoSumAssign(midMerged[len(midMerged)-1], r)
	comb := s + e
	if math.IsNaN(comb) {
		comb = s
	}
	return comb, assignSignOf(midMerged[len(midMerged)-1])
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

func hasPotentialCancellation(m, n int, normCost, u, v []float64, col4row []int, obj float64) bool {
	if m <= 1 {
		return false
	}
	minU, maxU := u[0], u[0]
	maxPot := 0.0
	minNonZeroPot := math.Inf(1)
	for _, ui := range u[:m] {
		if ui < minU {
			minU = ui
		}
		if ui > maxU {
			maxU = ui
		}
		a := math.Abs(ui)
		if a > maxPot {
			maxPot = a
		}
		if a > 0 && a < minNonZeroPot {
			minNonZeroPot = a
		}
	}
	minV, maxV := v[0], v[0]
	for _, vj := range v[:n] {
		if vj < minV {
			minV = vj
		}
		if vj > maxV {
			maxV = vj
		}
		a := math.Abs(vj)
		if a > maxPot {
			maxPot = a
		}
		if a > 0 && a < minNonZeroPot {
			minNonZeroPot = a
		}
	}
	if maxPot == 0 {
		return false
	}
	suspect := minNonZeroPot <= 0x1p-30*maxPot
	if !suspect {
		oppSign := (maxU > 0 && minV < 0) || (minU < 0 && maxV > 0)
		if oppSign && math.Abs(obj) <= 0x1p-12*maxPot {
			suspect = true
		}
	}
	if !suspect {
		for i := range m {
			j := col4row[i]
			if uint(j) >= uint(n) {
				continue
			}
			c := normCost[i*n+j]
			ui := u[i]
			vj := v[j]
			ac := math.Abs(c)
			auv := math.Abs(ui) + math.Abs(vj)
			if ac > 0 && auv > 0x1p20*ac {
				suspect = true
				break
			}
			s1, e1 := twoSumAssign(c, -ui)
			s2, e2 := twoSumAssign(s1, -vj)
			if math.Abs(s2+(e1+e2)) > 64.0*0x1p-53*(ac+auv) {
				suspect = true
				break
			}
		}
	}
	if !suspect {
		return false
	}
	for i := range m {
		ui := u[i]
		aui := math.Abs(ui)
		rowSlice := normCost[i*n : (i+1)*n]
		mj := col4row[i]
		for j, c := range rowSlice {
			if math.IsInf(c, 1) {
				continue
			}
			vj := v[j]
			s1, e1 := twoSumAssign(c, -ui)
			s2, e2 := twoSumAssign(s1, -vj)
			rc := s2 + (e1 + e2)
			ac := math.Abs(c)
			avj := math.Abs(vj)
			auv := aui + avj
			severeCancel := auv > 0x1p20*ac ||
				(avj > 0 && ac+aui > 0x1p20*avj) ||
				(aui > 0 && ac+avj > 0x1p20*aui)
			if severeCancel {
				if (j == mj && rc > 0) || rc < 0 {
					return true
				}
			} else {
				tol := 16.0 * 0x1p-53 * (ac + auv)
				if (j == mj && rc > tol) || rc < -tol {
					return true
				}
			}
		}
	}
	return false
}

func isAssignmentStrictlyWorse(m, n int, candCol4Row, baseCol4Row []int, unscaledCost []float64) bool {
	var diffAcc assignWideAcc
	for i := range m {
		jc := candCol4Row[i]
		jb := baseCol4Row[i]
		if jc < 0 || jc >= n {
			return true
		}
		if jc == jb {
			continue
		}
		cc := unscaledCost[i*n+jc]
		if math.IsInf(cc, 1) || math.IsNaN(cc) {
			return true
		}
		diffAcc.addVal(cc)
		diffAcc.addVal(-unscaledCost[i*n+jb])
	}
	if diffAcc.nonFin {
		return true
	}
	return diffAcc.sign() > 0
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

func validateAndPrepareCost(rows, cols int, cost []float64, ownedInWork bool, sense Sense, ws *Workspace, disableSIMD bool) (m, n int, normCost, unscaledCost []float64, transposed, lostSubnormal bool, scaleExp int, err error) {
	negate := sense == Maximize
	transposed = rows > cols
	m = min(rows, cols)
	n = max(rows, cols)
	maxAbsFinite := 0.0

	if !transposed && !negate {
		if simd.AllAbsLE64With(cost, hugeCostThreshold, disableSIMD) {
			return m, n, cost, cost, false, false, 0, nil
		}
		for _, c := range cost {
			if !(math.Abs(c) <= hugeCostThreshold) {
				if math.IsNaN(c) || math.IsInf(c, -1) {
					return 0, 0, nil, nil, false, false, 0, ErrInvalidCost
				}
				if !math.IsInf(c, 1) {
					if ac := math.Abs(c); ac > maxAbsFinite {
						maxAbsFinite = ac
					}
				}
			}
		}
		downExp := minimalDownscaleExp(maxAbsFinite, n)
		if downExp == 0 {
			return m, n, cost, cost, false, maxAbsFinite > hugeCostThreshold && hasTinyEntry(cost), 0, nil
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
		return m, n, dst, orig, false, lostSub, downExp, nil
	}

	if !transposed && negate {
		var dst []float64
		if ownedInWork {
			dst = cost
		} else {
			dst = ws.ensureWorkCost(rows * cols)
		}
		if simd.ValidateAndNegate64With(dst, cost, hugeCostThreshold, disableSIMD) {
			return m, n, dst, dst, false, false, 0, nil
		}
		for _, nc := range dst {
			if !(math.Abs(nc) <= hugeCostThreshold) {
				if math.IsNaN(nc) || math.IsInf(nc, -1) {
					return 0, 0, nil, nil, false, false, 0, ErrInvalidCost
				}
				if !math.IsInf(nc, 1) {
					if ac := math.Abs(nc); ac > maxAbsFinite {
						maxAbsFinite = ac
					}
				}
			}
		}
		downExp := minimalDownscaleExp(maxAbsFinite, n)
		if downExp > 0 {
			orig := slices.Clone(dst)
			lostSub := downscaleSlice(orig, dst, downExp)
			return m, n, dst, orig, false, lostSub, downExp, nil
		}
		return m, n, dst, dst, false, maxAbsFinite > hugeCostThreshold && hasTinyEntry(dst), 0, nil
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

	if simd.AllAbsLE64With(src, hugeCostThreshold, disableSIMD) {
		simd.Transpose64With(dst, src, rows, cols, negate, disableSIMD)
		return m, n, dst, dst, true, false, 0, nil
	}

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
								return 0, 0, nil, nil, false, false, 0, ErrInvalidCost
							}
							if !math.IsInf(val, -1) {
								if av := math.Abs(val); av > maxAbsFinite {
									maxAbsFinite = av
								}
							}
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
								return 0, 0, nil, nil, false, false, 0, ErrInvalidCost
							}
							if !math.IsInf(val, 1) {
								if av := math.Abs(val); av > maxAbsFinite {
									maxAbsFinite = av
								}
							}
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
		return m, n, dst, orig, true, lostSub, downExp, nil
	}
	return m, n, dst, dst, true, maxAbsFinite > hugeCostThreshold && hasTinyEntry(dst), 0, nil
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

func colReduceValidateSquare(n int, cost, v []float64, row4col []int, disableSIMD bool, done <-chan struct{}) (bool, error) {
	if done == nil || n <= 256 {
		return simd.ColReduceValidateRows64With(n, 0, n, cost, v, row4col, hugeCostThreshold, disableSIMD), nil
	}
	for r0 := 0; r0 < n; r0 += 256 {
		if r0 > 0 {
			select {
			case <-done:
				return false, errContextCanceled
			default:
			}
		}
		if !simd.ColReduceValidateRows64With(n, r0, min(n, r0+256), cost, v, row4col, hugeCostThreshold, disableSIMD) {
			return false, nil
		}
	}
	return true, nil
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
		if errors.Is(errPre, errContextCanceled) {
			return ctx.Err()
		}
		return errPre
	}

	if numFree > 0 {
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
			} else {
				for _, curRow := range freeRows {
					if err := augmentRowScalar(curRow, n, normCost, u, v, d, pred, col4row, row4col, remaining, nil); err != nil {
						return err
					}
				}
			}
		} else {
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
					if errors.Is(err, errContextCanceled) {
						return ctx.Err()
					}
					return err
				}
			}
		}
	}
	return nil
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
		m, n                   int
		normCost, unscaledCost []float64
		transposed             bool
		lostSubnormal          bool
		scaleExp               int
		colReduced             bool
		err                    error
	)

	if rows == cols && rows > 1 && sense == Minimize {
		m, n = rows, cols
		ws.ensure(m, n)
		ok, crErr := colReduceValidateSquare(n, cost, ws.v[:n], ws.row4col[:n], cfg.DisableSIMD, done)
		if crErr != nil {
			if errors.Is(crErr, errContextCanceled) {
				return 0, false, ctx.Err()
			}
			return 0, false, crErr
		}
		if ok {
			normCost = cost
			unscaledCost = cost
			colReduced = true
		} else {
			m, n, normCost, unscaledCost, transposed, lostSubnormal, scaleExp, err = validateAndPrepareCost(rows, cols, cost, ownedInWork, sense, ws, cfg.DisableSIMD)
			if err != nil {
				return 0, false, err
			}
		}
	} else {
		m, n, normCost, unscaledCost, transposed, lostSubnormal, scaleExp, err = validateAndPrepareCost(rows, cols, cost, ownedInWork, sense, ws, cfg.DisableSIMD)
		if err != nil {
			return 0, false, err
		}
		ws.ensure(m, n)
	}

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
		uSubOrig, vSubOrig []float64
		normObj            float64
		hasPotCancel       bool
		stage2Accepted     bool
	)
	if !lostSubnormal {
		normObj = computeMatchedObjective(m, n, normCost, col4row, scaleExp == 0)
		hasPotCancel = hasPotentialCancellation(m, n, normCost, u, v, col4row, normObj)
	}
	if lostSubnormal || hasPotCancel {
		hasDeepSub := false
		if lostSubnormal {
			for _, oc := range unscaledCost[:m*n] {
				if oc != 0 && math.Abs(oc) < 0x1p-950 {
					hasDeepSub = true
					break
				}
			}
		}
		tau := 4.0 * float64(m+1) * math.SmallestNonzeroFloat64
		subCap := float64(m+2) * tau
		maxPot := 0.0
		uBase := make([]float64, m)
		for i, ui := range u {
			if a := math.Abs(ui); a > maxPot {
				maxPot = a
			}
			if math.Abs(ui) > subCap {
				uBase[i] = ui
			}
		}
		vBase := make([]float64, n)
		if m == n {
			for j, vj := range v {
				if a := math.Abs(vj); a > maxPot {
					maxPot = a
				}
				if math.Abs(vj) > subCap {
					vBase[j] = vj
				}
			}
		} else {
			for j, vj := range v {
				if a := math.Abs(vj); a > maxPot {
					maxPot = a
				}
				if vj < -subCap {
					vBase[j] = vj
				}
			}
		}
		uSubCur := make([]float64, m)
		vSubCur := make([]float64, n)
		edgeResidual := func(idx int, ui, uSub, vj, vSub float64) (s2, rem float64) {
			nc := normCost[idx]
			s1, e1 := twoSumAssign(nc, -ui)
			s2, e2 := twoSumAssign(s1, -vj)
			rem = (e1 + e2) - math.Ldexp(uSub, -scaleExp) - math.Ldexp(vSub, -scaleExp)
			return s2, rem
		}
		maxPasses := 1
		if hasDeepSub {
			maxPasses = 2
		}
		resCost := make([]float64, m*n)
		u2 := make([]float64, m)
		v2 := make([]float64, n)
		c4r2 := make([]int, m)
		r4c2 := make([]int, n)
		capRC := math.MaxFloat64 / (64.0 * float64(m+2) * float64(m+2))
		for pass := range maxPasses {
			s1Pos := 0.0
			maxNegD := 0.0
			for i := range m {
				ui, uSub := uBase[i], uSubCur[i]
				for j := range n {
					idx := i*n + j
					if math.IsInf(unscaledCost[idx], 1) {
						continue
					}
					s2, rem := edgeResidual(idx, ui, uSub, vBase[j], vSubCur[j])
					d := s2 + rem
					if j == col4row[i] && d > 0 {
						s1Pos += d
					}
					if -d > maxNegD {
						maxNegD = -d
					}
				}
			}
			cutCap := s1Pos + float64(m)*maxNegD
			globalFpTol := 0.0
			if hasPotCancel {
				globalFpTol = 8.0 * float64(m+2) * 0x1p-53 * (2.0 * maxPot)
			}
			baseCap := max(2.0*subCap, max(cutCap, globalFpTol))
			maxOrigCandAbs := 0.0
			for i := range m {
				ui, uSub := uBase[i], uSubCur[i]
				for j := range n {
					idx := i*n + j
					oc := unscaledCost[idx]
					if math.IsInf(oc, 1) {
						continue
					}
					nc := normCost[idx]
					s2, rem := edgeResidual(idx, ui, uSub, vBase[j], vSubCur[j])
					d := s2 + rem
					edgeScale := math.Abs(nc) + math.Abs(ui) + math.Abs(vBase[j])
					fpTol := 8.0 * float64(m+2) * 0x1p-53 * edgeScale
					if pass > 0 {
						fpTol = 2.0 * float64(m+2) * 0x1p-53 * edgeScale
					}
					if d > max(fpTol, baseCap) {
						continue
					}
					origAbs := math.Ldexp(math.Abs(d), scaleExp)
					if math.IsInf(origAbs, 1) {
						origAbs = math.MaxFloat64
					}
					if origAbs > maxOrigCandAbs {
						maxOrigCandAbs = origAbs
					}
				}
			}
			liftExp := 64
			if maxOrigCandAbs > 0 {
				_, maxE := math.Frexp(maxOrigCandAbs)
				_, capE := math.Frexp(capRC)
				liftExp = max(0, min(64, capE-maxE-1))
			}
			minNegRC := 0.0
			for i := range m {
				ui, uSub := uBase[i], uSubCur[i]
				for j := range n {
					idx := i*n + j
					oc := unscaledCost[idx]
					if math.IsInf(oc, 1) {
						resCost[idx] = math.Inf(1)
						continue
					}
					nc := normCost[idx]
					s2, rem := edgeResidual(idx, ui, uSub, vBase[j], vSubCur[j])
					d := s2 + rem
					edgeScale := math.Abs(nc) + math.Abs(ui) + math.Abs(vBase[j])
					fpTol := 8.0 * float64(m+2) * 0x1p-53 * edgeScale
					if pass > 0 {
						fpTol = 2.0 * float64(m+2) * 0x1p-53 * edgeScale
					}
					if d > max(fpTol, baseCap) {
						resCost[idx] = math.Inf(1)
						continue
					}
					s1, e1 := twoSumAssign(nc, -ui)
					s2b, e2 := twoSumAssign(s1, -vBase[j])
					rc := math.Ldexp(s2b, scaleExp+liftExp) + math.Ldexp(e1+e2, scaleExp+liftExp) -
						math.Ldexp(uSub, liftExp) - math.Ldexp(vSubCur[j], liftExp)
					if math.Ldexp(nc, scaleExp) != oc {
						rc += math.Ldexp(oc, liftExp) - math.Ldexp(nc, scaleExp+liftExp)
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
			var colBonus []float64
			if m < n {
				stage1PosSum := 0.0
				for i := range m {
					if j := col4row[i]; uint(j) < uint(n) {
						if rc := resCost[i*n+j]; rc > 0 && !math.IsInf(rc, 1) {
							stage1PosSum += rc
						}
					}
				}
				swing := stage1PosSum - float64(m)*minNegRC
				mustBonusCap := min(capRC, 4.0*swing+math.Ldexp(8.0*subCap, scaleExp+liftExp))
				if mustBonusCap <= 0 {
					mustBonusCap = math.SmallestNonzeroFloat64
				}
				colBonus = make([]float64, n)
				for j, vj := range vBase {
					totVJ := math.Ldexp(vj, scaleExp) + vSubCur[j]
					if totVJ >= 0 {
						continue
					}
					b := min(mustBonusCap, math.Ldexp(-totVJ, liftExp))
					colBonus[j] = b
					for i := range m {
						idx := i*n + j
						if !math.IsInf(resCost[idx], 1) {
							resCost[idx] -= b
						}
					}
				}
			}
			if err2 := runNormalizedLSAP(ctx, m, n, resCost, u2, v2, c4r2, r4c2, ws, cfg.DisableSIMD, false, done); err2 != nil {
				if errors.Is(err2, context.Canceled) || errors.Is(err2, context.DeadlineExceeded) {
					return 0, transposed, err2
				}
				break
			}
			if !isAssignmentStrictlyWorse(m, n, c4r2, col4row, unscaledCost) {
				stage2Accepted = true
				copy(col4row, c4r2)
				copy(row4col, r4c2)
				copy(u, uBase)
				copy(v, vBase)
				for i := range m {
					uSubCur[i] += math.Ldexp(u2[i], -liftExp)
				}
				for j := range n {
					vj := v2[j]
					if m < n && colBonus != nil {
						vj += colBonus[j]
					}
					vSubCur[j] += math.Ldexp(vj, -liftExp)
				}
				uSubOrig = uSubCur
				vSubOrig = vSubCur
			}
			if liftExp >= 53 {
				break
			}
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

	if scaleExp > 0 || uSubOrig != nil {
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
			mLim := math.Ldexp(math.MaxFloat64, -scaleExp)
			deltaLo := max(-mLim-minU, maxV-mLim)
			deltaHi := min(mLim-maxU, minV+mLim)
			if !(0.0 >= deltaLo && 0.0 <= deltaHi) {
				shift = 0.5 * (deltaLo + deltaHi)
			}
		}
		anyClamped := false
		for i := range m {
			scaled := math.Ldexp(u[i]+shift, scaleExp)
			if uSubOrig != nil {
				scaled += uSubOrig[i]
			}
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
			if vSubOrig != nil {
				scaled += vSubOrig[j]
			}
			clamped := clampFinite(scaled)
			if m < n && clamped > 0 {
				clamped = 0
			}
			if clamped != scaled {
				anyClamped = true
			}
			v[j] = clamped
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
					lim := c - v[j]
					if !math.IsInf(lim, 1) && lim < ui {
						ui = clampFinite(lim)
					}
				}
				u[i] = ui
			}
		} else if uSubOrig != nil {
			polishStage2Duals(m, n, unscaledCost, col4row, row4col, u, v)
		}
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

func polishStage2Duals(m, n int, unscaledCost []float64, col4row, row4col []int, u, v []float64) {
	for i := range m {
		j := col4row[i]
		if uint(j) >= uint(n) {
			return
		}
		cij := unscaledCost[i*n+j]
		if math.IsInf(cij, 0) || math.IsNaN(cij) {
			return
		}
		if lim := cij - v[j]; !math.IsNaN(lim) && !math.IsInf(lim, 0) {
			s, e := twoSumAssign(u[i], v[j])
			if math.Abs((cij-s)-e) > 4.0*0x1p-53*(math.Abs(cij)+math.Abs(u[i])+math.Abs(v[j])) {
				u[i] = lim
			}
		}
	}
	needPolish := false
	for i := range m {
		ui := u[i]
		rowSlice := unscaledCost[i*n : (i+1)*n]
		for j, cij := range rowSlice {
			if math.IsInf(cij, 1) {
				continue
			}
			s, e := twoSumAssign(ui, v[j])
			tol := max(1e-320, 16.0*0x1p-53*(math.Abs(cij)+math.Abs(ui)+math.Abs(v[j])))
			if (s-cij)+e > tol {
				needPolish = true
				break
			}
		}
		if needPolish {
			break
		}
	}
	if !needPolish {
		return
	}

	pi := make([]float64, m+1)
	var rootToRow []float64
	if m < n {
		rootToRow = make([]float64, m)
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
				s1, e1 := twoSumAssign(cij, -u[i])
				s2, e2 := twoSumAssign(s1, -vj)
				cand := pk + (s2 + (e1 + e2))
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

	base := pi[0]
	if m < n {
		base = pi[m]
	}
	for i := range m {
		delta := pi[i] - base
		if math.IsNaN(delta) || math.IsInf(delta, 0) || delta == 0 {
			continue
		}
		j := col4row[i]
		newU := clampFinite(u[i] + delta)
		newV := clampFinite(v[j] - delta)
		if m < n && newV > 0 {
			newV = 0
			newU = clampFinite(unscaledCost[i*n+j])
		}
		u[i] = newU
		v[j] = newV
	}
}

// preprocessSquareJV executes Jonker-Volgenant (1987) Column Reduction (in row-major order),
// Reduction Transfer, and 2-pass Augmenting Row Reduction for square n x n instances.
// When colReduced is true, v and row4col already hold the column minima and argmin rows.
func preprocessSquareJV(n int, cost, u, v []float64, col4row, row4col, freeRows, auxRows []int, disableSIMD, colReduced bool, done <-chan struct{}) (int, error) {
	if n <= 0 || len(u) < n || len(v) < n || len(col4row) < n || len(row4col) < n || len(freeRows) < n || len(auxRows) < n {
		return 0, nil
	}
	u = u[:n]
	v = v[:n]
	col4row = col4row[:n]
	row4col = row4col[:n]
	freeRows = freeRows[:n]
	auxRows = auxRows[:n]
	clear(u)

	// Perform row-major column reduction unless fused validation already populated v and row4col.
	if !colReduced {
		if done == nil || n <= 256 {
			simd.ColReduceRows64With(n, 0, n, cost, v, row4col, disableSIMD)
		} else {
			for r0 := 0; r0 < n; r0 += 256 {
				if r0 > 0 {
					select {
					case <-done:
						return 0, errContextCanceled
					default:
					}
				}
				simd.ColReduceRows64With(n, r0, min(n, r0+256), cost, v, row4col, disableSIMD)
			}
		}
	}

	// Assign columns in ascending order (retaining the smallest column index on ties
	// so constant cost matrices yield the identity assignment) and flag contended rows.
	// Contended rows are temporarily marked with col4row[i] = -2 - firstJ.
	for j, vj := range v {
		if math.IsInf(vj, 1) {
			return 0, ErrInfeasible
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

	// Apply Reduction Transfer on uniquely matched rows and collect unassigned rows into freeRows.
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

	if numFree == 0 {
		return 0, nil
	}
	// When column reduction matches <= max(2, n/32) rows, almost all rows and columns
	// remain unassigned because 1-2 rows dominated every column.
	if numFree >= n-max(2, n/32) {
		if isCollinearSquare(n, numFree, cost, freeRows) {
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
		numFree = 0
		for i := range n {
			if done != nil && n > 256 && i&255 == 0 {
				select {
				case <-done:
					return 0, errContextCanceled
				default:
				}
			}
			c := col4row[i]
			if c == -1 {
				freeRows[numFree] = i
				numFree++
			} else if c < -1 {
				col4row[i] = -2 - c
			} else {
				rowCost := cost[i*n : (i+1)*n]
				minRed := simd.MinReducedSkip64With(rowCost, v, c, disableSIMD)
				if !math.IsInf(minRed, 1) {
					u[i] = minRed
					v[c] = rowCost[c] - minRed
				}
			}
		}
		if numFree == 0 {
			return 0, nil
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
	rowCost := cost[curRow*n:][:n]
	ui := u[curRow]

	minVal = simd.InitReducedRow64With(d, rowCost, v, ui, false)
	if !(minVal < math.Inf(1)) {
		return 0, 0, false, ErrInfeasible
	}

	nan := math.NaN()
	for j, dj := range d {
		pred[j] = curRow
		if dj <= minVal {
			if row4col[j] == -1 {
				u[curRow] += dj
				col4row[curRow] = j
				row4col[j] = curRow
				return minVal, 0, true, nil
			}
			remaining[up] = j
			dQueue[up] = minVal
			d[j] = nan
			up++
		}
	}
	return minVal, up, false, nil
}

// refillQueueContigSIMD finds the next minimum finite non-NaN distance in d[0:n], enqueues
// all achieving columns with NaN sentinels, and returns sink >= 0 if any achieving
// column is unassigned.
func refillQueueContigSIMD(d, dQueue []float64, row4col, remaining []int, up int) (minVal float64, newUp, sink int, err error) {
	nan := math.NaN()
	minVal = simd.MinNoNaN64(d)
	if !(minVal < math.Inf(1)) {
		return 0, up, -1, ErrInfeasible
	}
	dS := d
	j := 0
	for len(dS) >= 4 {
		if dS[0] <= minVal || dS[1] <= minVal || dS[2] <= minVal || dS[3] <= minVal {
			for k := range 4 {
				if dS[k] <= minVal {
					jj := j + k
					if row4col[jj] == -1 {
						return minVal, up, jj, nil
					}
					remaining[up] = jj
					dQueue[up] = minVal
					d[jj] = nan
					up++
				}
			}
		}
		dS = dS[4:]
		j += 4
	}
	for k, dj := range dS {
		if dj <= minVal {
			jj := j + k
			if row4col[jj] == -1 {
				return minVal, up, jj, nil
			}
			remaining[up] = jj
			dQueue[up] = minVal
			d[jj] = nan
			up++
		}
	}
	return minVal, up, -1, nil
}

// relaxRowContigSIMD relaxes contiguous d[0:n] from row rowI using the 4-wide AVX2
// branchless kernel, enqueuing tight columns and returning sink >= 0 on an unassigned hit.
func relaxRowContigSIMD(rCost, v, d, dQueue []float64, pred, row4col, remaining []int, n, rowI, up int, ui, minVal float64) (newUp, sink int) {
	nan := math.NaN()
	for j := 0; j < n; {
		j = simd.RelaxRow64(rCost, v, d, pred, j, rowI, ui, minVal)
		if j >= n {
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
					remaining[up] = j
					dQueue[up] = minVal
					d[j] = nan
					up++
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
	rCost = rCost[:n]
	v = v[:n]
	d = d[:n]
	pred = pred[:n]
	row4col = row4col[:n]
	remaining = remaining[:n]

	for k := up; k < n; k++ {
		j := remaining[k]
		r := minVal + ((rCost[j] - v[j]) - ui)
		if r < d[k] {
			pred[j] = rowI
			if r <= minVal {
				if row4col[j] == -1 {
					return up, j
				}
				remaining[k] = remaining[up]
				remaining[up] = j
				d[k] = d[up]
				d[up] = minVal
				up++
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
