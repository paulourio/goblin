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

func isAssignmentStrictlyWorse(m, n int, candCol4Row, baseCol4Row []int, unscaledCost []float64) bool {
	const bigExp = 24
	var bigS, bigC, smallS, smallC float64
	accTerm := func(v float64) {
		if v == 0 {
			return
		}
		if math.Abs(v) >= 1e150 {
			var e float64
			bigS, e = twoSumAssign(bigS, math.Ldexp(v, -bigExp))
			bigC += e
		} else {
			var e float64
			smallS, e = twoSumAssign(smallS, v)
			smallC += e
		}
	}
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
		accTerm(cc)
		accTerm(-unscaledCost[i*n+jb])
	}
	bigSum, bigRem := twoSumAssign(bigS, bigC)
	smallSum, smallRem := twoSumAssign(smallS, smallC)
	if bigSum != 0 {
		return bigSum > 0
	}
	if bigRem != 0 {
		remOrig := math.Ldexp(bigRem, bigExp)
		if comb := remOrig + smallSum; comb != 0 {
			return comb > 0
		}
	}
	if smallSum != 0 {
		return smallSum > 0
	}
	return smallRem > 0
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
		if simd.AllAbsLE64With(cost, hugeCostThreshold, disableSIMD || len(cost) < 256) {
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
		for idx, c := range cost {
			if !(math.Abs(c) <= hugeCostThreshold) {
				if math.IsNaN(c) || math.IsInf(c, 1) {
					return 0, 0, nil, nil, false, false, 0, ErrInvalidCost
				}
				if !math.IsInf(c, -1) {
					if ac := math.Abs(c); ac > maxAbsFinite {
						maxAbsFinite = ac
					}
				}
			}
			if c == 0 {
				dst[idx] = 0
			} else {
				dst[idx] = -c
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

	const tile = 32
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
						if val == 0 {
							dst[c*rows+r] = 0
						} else {
							dst[c*rows+r] = -val
						}
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

func runNormalizedLSAP(ctx context.Context, m, n int, normCost, u, v []float64, col4row, row4col []int, ws *Workspace, disableSIMD bool, done <-chan struct{}) error {
	for i := range m {
		col4row[i] = -1
	}
	for j := range n {
		row4col[j] = -1
	}

	var numFree int
	var errPre error
	if m == n {
		numFree, errPre = preprocessSquareJV(n, normCost, u, v, col4row, row4col, ws.freeRows[:m], ws.auxRows[:m], disableSIMD, done)
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
		pred := ws.pred[:n]
		freeRows := ws.freeRows[:numFree]
		if done == nil {
			for _, curRow := range freeRows {
				if err := augmentRow(curRow, n, normCost, u, v, d, pred, col4row, row4col, remaining, disableSIMD, nil); err != nil {
					return err
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
				if err := augmentRow(curRow, n, normCost, u, v, d, pred, col4row, row4col, remaining, disableSIMD, done); err != nil {
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

	m, n, normCost, unscaledCost, transposed, lostSubnormal, scaleExp, err := validateAndPrepareCost(rows, cols, cost, ownedInWork, sense, ws, cfg.DisableSIMD)
	if err != nil {
		return 0, false, err
	}

	ws.ensure(m, n)
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

	if err := runNormalizedLSAP(ctx, m, n, normCost, u, v, col4row, row4col, ws, cfg.DisableSIMD, done); err != nil {
		return 0, transposed, err
	}

	var uSubOrig, vSubOrig []float64
	if lostSubnormal {
		hasDeepSub := false
		for _, oc := range unscaledCost[:m*n] {
			if oc != 0 && math.Abs(oc) < 0x1p-950 {
				hasDeepSub = true
				break
			}
		}
		tau := 4.0 * float64(m+1) * math.SmallestNonzeroFloat64
		subCap := float64(m+2) * tau
		uBase := make([]float64, m)
		for i, ui := range u {
			if math.Abs(ui) > subCap {
				uBase[i] = ui
			}
		}
		vBase := make([]float64, n)
		if m == n {
			for j, vj := range v {
				if math.Abs(vj) > subCap {
					vBase[j] = vj
				}
			}
		} else {
			for j, vj := range v {
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
					if d > max(fpTol, 2.0*subCap) {
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
					if d > max(fpTol, 2.0*subCap) {
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
			if err2 := runNormalizedLSAP(ctx, m, n, resCost, u2, v2, c4r2, r4c2, ws, cfg.DisableSIMD, done); err2 != nil {
				if errors.Is(err2, context.Canceled) || errors.Is(err2, context.DeadlineExceeded) {
					return 0, transposed, err2
				}
				break
			}
			if !isAssignmentStrictlyWorse(m, n, c4r2, col4row, unscaledCost) {
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

	// Extract primal assignment in ascending row order and compute exact objective on input scale.
	var totalMin float64
	if scaleExp == 0 && uSubOrig == nil {
		if !transposed {
			for i := range m {
				j := col4row[i]
				rowInd[i] = i
				colInd[i] = j
				totalMin += unscaledCost[i*n+j]
			}
		} else {
			outIdx := 0
			for origRow := range n {
				origCol := row4col[origRow]
				if origCol >= 0 {
					rowInd[outIdx] = origRow
					colInd[outIdx] = origCol
					totalMin += unscaledCost[origCol*n+origRow]
					outIdx++
				}
			}
		}
	} else {
		const bigExp = 24
		var bigS, bigC, smallS, smallC float64
		accCost := func(c float64) {
			if math.Abs(c) >= 1e150 {
				var e float64
				bigS, e = twoSumAssign(bigS, math.Ldexp(c, -bigExp))
				bigC += e
			} else {
				var e float64
				smallS, e = twoSumAssign(smallS, c)
				smallC += e
			}
		}
		if !transposed {
			for i := range m {
				j := col4row[i]
				rowInd[i] = i
				colInd[i] = j
				accCost(unscaledCost[i*n+j])
			}
		} else {
			outIdx := 0
			for origRow := range n {
				origCol := row4col[origRow]
				if origCol >= 0 {
					rowInd[outIdx] = origRow
					colInd[outIdx] = origCol
					accCost(unscaledCost[origCol*n+origRow])
					outIdx++
				}
			}
		}
		bigSum, bigRem := twoSumAssign(bigS, bigC)
		smallSum := smallS + smallC
		hi := math.Ldexp(bigSum, bigExp)
		if math.IsInf(hi, 0) {
			mid := math.Ldexp(bigSum, 12) + (math.Ldexp(bigRem, 12) + math.Ldexp(smallSum, -12))
			totalMin = math.Ldexp(mid, 12)
		} else {
			totalMin = hi + (math.Ldexp(bigRem, bigExp) + smallSum)
		}
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

func minReducedSlice(rowCost, v []float64, disableSIMD bool) float64 {
	return simd.MinReducedSlice64With(rowCost, v, disableSIMD)
}

// preprocessSquareJV executes Jonker-Volgenant (1987) Column Reduction (in row-major order),
// Reduction Transfer, and 2-pass Augmenting Row Reduction for square n x n instances.
func preprocessSquareJV(n int, cost, u, v []float64, col4row, row4col, freeRows, auxRows []int, disableSIMD bool, done <-chan struct{}) (int, error) {
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

	// 1. Row-major scan to find minimum cost and argmin row for every column j.
	copy(v, cost[:n])
	for j := range row4col {
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
		rSlice := cost[i*n:][:n]
		vSlice := v
		r4cSlice := row4col
		for len(rSlice) >= 4 && len(vSlice) >= 4 && len(r4cSlice) >= 4 {
			c0, c1, c2, c3 := rSlice[0], rSlice[1], rSlice[2], rSlice[3]
			v0, v1, v2, v3 := vSlice[0], vSlice[1], vSlice[2], vSlice[3]
			if c0 < v0 || c1 < v1 || c2 < v2 || c3 < v3 {
				if c0 < v0 {
					vSlice[0] = c0
					r4cSlice[0] = i
				}
				if c1 < v1 {
					vSlice[1] = c1
					r4cSlice[1] = i
				}
				if c2 < v2 {
					vSlice[2] = c2
					r4cSlice[2] = i
				}
				if c3 < v3 {
					vSlice[3] = c3
					r4cSlice[3] = i
				}
			}
			rSlice = rSlice[4:]
			vSlice = vSlice[4:]
			r4cSlice = r4cSlice[4:]
		}
		if len(rSlice) > 0 {
			vSlice = vSlice[:len(rSlice)]
			r4cSlice = r4cSlice[:len(rSlice)]
			for k, c := range rSlice {
				if c < vSlice[k] {
					vSlice[k] = c
					r4cSlice[k] = i
				}
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

	// 2. Reduction Transfer on uniquely matched rows; collect unassigned rows into freeRows.
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
			// Uniquely matched row i -> column c: transfer dual slack from second-best column.
			rowCost := cost[i*n:][:n]
			minRed := min(
				minReducedSlice(rowCost[:c], v[:c], disableSIMD),
				minReducedSlice(rowCost[c+1:], v[c+1:], disableSIMD),
			)
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
				minRed := math.Inf(1)
				for j, val := range rowCost {
					if j != c {
						if r := val - v[j]; r < minRed {
							minRed = r
						}
					}
				}
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
	maxTransfers := 2 * n
	for range 2 {
		if numFree == 0 {
			break
		}
		nextFree := 0
		transfers := 0
		for rowIdx, i := range freeRows[:numFree] {
			if done != nil && n > 256 && rowIdx&127 == 0 {
				select {
				case <-done:
					return 0, errContextCanceled
				default:
				}
			}
			rowHops := 0
			const maxRowHops = 16
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
					break
				}

				if u1 == u2 && uint(j2) < uint(n) && row4col[j2] == -1 {
					u[i] = u1
					col4row[i] = j2
					row4col[j2] = i
					break
				}

				if u1 < u2 && rowHops < maxRowHops && transfers < maxTransfers {
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

// augmentRow finds a shortest augmenting path from unassigned row curRow to an unassigned
// column using Jonker-Volgenant's three-way partitioned Dijkstra queue (remaining[0:low],
// remaining[low:up], remaining[up:n]), maintaining d[0:n] in lockstep with remaining[0:n]
// for contiguous memory access, updates duals u and v in O(low), and augments
// the primal matching along pred.
func augmentRow(curRow, n int, cost, u, v, d []float64, pred, col4row, row4col, remaining []int, disableSIMD bool, done <-chan struct{}) error {
	if n <= 0 || len(v) < n || len(d) < n || len(pred) < n || len(row4col) < n || len(remaining) < n {
		return nil
	}
	v = v[:n]
	d = d[:n]
	pred = pred[:n]
	row4col = row4col[:n]
	remaining = remaining[:n]

	rowCost := cost[curRow*n : curRow*n+n]
	ui := u[curRow]

	minVal := simd.InitReducedRow64With(d, rowCost, v, ui, disableSIMD)
	if math.IsInf(minVal, 1) {
		return ErrInfeasible
	}

	// Initialize pred and remaining, partitioning columns at minVal into [0:up]
	// and keeping d[k] aligned with remaining[k].
	// If any column achieving minVal is already free, match immediately in 1 hop.
	low, up := 0, 0
	for j, dj := range d {
		pred[j] = curRow
		remaining[j] = j
		if dj <= minVal {
			if row4col[j] == -1 {
				u[curRow] += dj
				col4row[curRow] = j
				row4col[j] = curRow
				return nil
			}
			if uint(up) < uint(n) {
				remaining[j] = remaining[up]
				remaining[up] = j
				d[j] = d[up]
				d[up] = dj
				up++
			}
		}
	}

	var sink int
	for {
		if low == up {
			if uint(up) >= uint(n) {
				return ErrInfeasible
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
							minVal = v0
							minK = k
							hasTies = false
						} else {
							hasTies = true
						}
					}
					if v1 <= minVal {
						if v1 < minVal {
							minVal = v1
							minK = k + 1
							hasTies = false
						} else {
							hasTies = true
						}
					}
					if v2 <= minVal {
						if v2 < minVal {
							minVal = v2
							minK = k + 2
							hasTies = false
						} else {
							hasTies = true
						}
					}
					if v3 <= minVal {
						if v3 < minVal {
							minVal = v3
							minK = k + 3
							hasTies = false
						} else {
							hasTies = true
						}
					}
				}
				dRem = dRem[4:]
				k += 4
			}
			for idx, v := range dRem {
				if v <= minVal {
					if v < minVal {
						minVal = v
						minK = k + idx
						hasTies = false
					} else {
						hasTies = true
					}
				}
			}
			if math.IsInf(minVal, 1) {
				return ErrInfeasible
			}
			if !hasTies {
				if uint(minK) < uint(n) && uint(up) < uint(n) {
					j := remaining[minK]
					if uint(j) < uint(n) && row4col[j] == -1 {
						sink = j
						goto foundSink
					}
					remaining[minK] = remaining[up]
					remaining[up] = j
					d[minK] = d[up]
					d[up] = minVal
					up++
				}
			} else {
				for k := minK; k < n; k++ {
					if dk := d[k]; dk <= minVal {
						j := remaining[k]
						if uint(j) < uint(n) && row4col[j] == -1 {
							sink = j
							goto foundSink
						}
						if uint(up) < uint(n) {
							remaining[k] = remaining[up]
							remaining[up] = j
							d[k] = d[up]
							d[up] = dk
							up++
						}
					}
				}
			}
		}

		if uint(low) >= uint(n) {
			return ErrInfeasible
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
		if uint(col) >= uint(n) {
			continue
		}
		i := row4col[col]
		rCost := cost[i*n:][:n]
		ui := u[i]

		for k := max(0, up); k < n; k++ {
			j := remaining[k]
			if uint(j) >= uint(n) {
				continue
			}
			r := minVal + ((rCost[j] - v[j]) - ui)
			if r < d[k] {
				if r <= minVal {
					r = minVal
					d[k] = r
					pred[j] = i
					if row4col[j] == -1 {
						sink = j
						goto foundSink
					}
					if uint(up) < uint(n) {
						remaining[k] = remaining[up]
						remaining[up] = j
						d[k] = d[up]
						d[up] = r
						up++
					}
				} else {
					d[k] = r
					pred[j] = i
				}
			}
		}
	}

foundSink:
	// Dual update over expanded columns remaining[:low] with d[k] < minVal.
	u[curRow] += minVal
	remLow := remaining[:low]
	dLow := d[:low]
	for k, j := range remLow {
		delta := dLow[k] - minVal
		if uint(j) < uint(n) {
			v[j] += delta
			u[row4col[j]] -= delta
		}
	}

	// Augment primal matching along pred from sink back to curRow.
	for j := sink; ; {
		r := pred[j]
		row4col[j] = r
		j, col4row[r] = col4row[r], j
		if r == curRow {
			break
		}
	}
	return nil
}
