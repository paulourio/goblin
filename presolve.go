package goblin

import (
	"cmp"
	"math"
	"math/bits"
	"slices"
	"strconv"
	"time"
)

const (
	presolveFeasTol             = 1e-6
	presolveIntTol              = 1e-7
	presolveBoundChangeTol      = 1e-9
	presolveGCDIntTol           = 1e-8
	presolveGCDMaxRowErr        = 1e-9
	presolveGCDMaxCoeff         = 1e9
	presolveRelScaleTol         = 1e-12
	presolveZeroTol             = 1e-12
	presolveFastMaxAbs          = 1e12
	presolveFastMinAbs          = 1e-12
	presolveFastMaxRowLen       = 4096
	presolveFastRelMargin       = 1e-11
	presolveSlackGuardRelMargin = 0x1p-30
	maxOrbitGroupSize           = 64
	maxOrbitSeedTrials          = 64
	orbitUnassigned             = -1
	rowStateUnvisited           = -1
	rowStateInvariant           = -2
)

type presolveWorkRow struct {
	name   string
	cols   []int
	vals   []float64
	rhs    float64
	sense  ConstraintSense
	active bool
	shared bool
}

func (r *presolveWorkRow) ensureOwned() {
	if !r.shared {
		return
	}
	r.cols = slices.Clone(r.cols)
	r.vals = slices.Clone(r.vals)
	r.shared = false
}

// PresolveResult holds the reduced CompiledProblem or an immediate infeasible status flag.
type PresolveResult struct {
	Reduced          *CompiledProblem
	Infeasible       bool
	TimedOut         bool
	TightenedBounds  int
	RemovedRows      int
	StrengthenedRows int
}

// Presolve runs iterative bound tightening, Euclidean GCD integer row scaling,
// redundant row dropping, and binary coefficient strengthening on orig. When any
// variable bound interval becomes empty or an integer row proves unreachable, it
// returns Infeasible without allocating a reduced matrix.
func Presolve(orig *CompiledProblem, maxPasses int) PresolveResult {
	return presolveWithDeadline(orig, maxPasses, time.Time{}, nil)
}

func presolveWithDeadline(orig *CompiledProblem, maxPasses int, deadline time.Time, done <-chan struct{}) PresolveResult {
	if maxPasses <= 0 {
		maxPasses = defaultPresolveMaxPass
	}
	if stopRequested(deadline, done) {
		return PresolveResult{Reduced: orig, TimedOut: true}
	}
	n := orig.numVars
	m := orig.numConstraints
	if n == 0 {
		for i := range m {
			if 0 < orig.rowLB[i]-presolveFeasTol || 0 > orig.rowUB[i]+presolveFeasTol {
				return PresolveResult{Infeasible: true}
			}
		}
		return PresolveResult{Reduced: orig}
	}

	lb := slices.Clone(orig.varLB)
	ub := slices.Clone(orig.varUB)
	tightened := 0
	for j := range n {
		if orig.varCat[j] != Continuous {
			if !math.IsInf(lb[j], -1) {
				cl := math.Ceil(lb[j] - presolveIntTol)
				if cl > lb[j]+presolveBoundChangeTol {
					lb[j] = cl
					tightened++
				}
			}
			if !math.IsInf(ub[j], 1) {
				fl := math.Floor(ub[j] + presolveIntTol)
				if fl < ub[j]-presolveBoundChangeTol {
					ub[j] = fl
					tightened++
				}
			}
		}
		if lb[j] > ub[j]+presolveFeasTol {
			return PresolveResult{Infeasible: true}
		}
		if lb[j] > ub[j] {
			lb[j] = ub[j]
		}
	}
	if m == 0 && tightened == 0 {
		return PresolveResult{Reduced: orig}
	}

	rows := make([]presolveWorkRow, m)
	for i := range m {
		s, e := orig.rowPtr[i], orig.rowPtr[i+1]
		rows[i] = presolveWorkRow{
			name:   orig.rowNames[i],
			cols:   orig.colIdx[s:e:e],
			vals:   orig.rowVal[s:e:e],
			rhs:    orig.rowRHS[i],
			sense:  orig.rowSense[i],
			active: true,
			shared: true,
		}
	}

	strengthened := 0
	removedRows := 0

	// 1. Euclidean GCD reduction on pure-integer rows with domain-bounded rounding error.
	if orig.isMIP {
		for i := range m {
			r := &rows[i]
			if !r.active || len(r.cols) == 0 {
				continue
			}
			allInt := true
			var g int64
			maxRoundErr := 0.0
			for k, col := range r.cols {
				if orig.varCat[col] == Continuous {
					allInt = false
					break
				}
				cVal := r.vals[k]
				rc := math.Round(cVal)
				diff := math.Abs(cVal - rc)
				if diff > presolveGCDIntTol || math.Abs(rc) > presolveGCDMaxCoeff {
					allInt = false
					break
				}
				if diff > 0 {
					maxDom := max(math.Abs(lb[col]), math.Abs(ub[col]))
					if math.IsInf(maxDom, 0) || math.IsNaN(maxDom) {
						allInt = false
						break
					}
					maxRoundErr += diff * maxDom
					if maxRoundErr > presolveGCDMaxRowErr {
						allInt = false
						break
					}
				}
				ic := int64(math.Abs(rc))
				if ic == 0 {
					continue
				}
				g = gcdInt64(g, ic)
			}
			if allInt && g > 1 {
				if r.sense == Equal && maxRoundErr == 0 && math.Abs(r.rhs) <= 0x1p62 {
					loInt := int64(math.Ceil(r.rhs - presolveFeasTol))
					hiInt := int64(math.Floor(r.rhs + presolveFeasTol))
					if q := divCeilInt64(loInt, g); q*g > hiInt {
						return PresolveResult{Infeasible: true}
					}
				}
				if math.Abs(r.rhs) <= 0x1p52 {
					r.ensureOwned()
					fg := float64(g)
					for k := range r.vals {
						r.vals[k] = math.Round(r.vals[k]) / fg
					}
					switch r.sense {
					case LessEqual:
						ubInt := int64(math.Floor(r.rhs + maxRoundErr + presolveIntTol))
						r.rhs = float64(divFloorInt64(ubInt, g))
					case GreaterEqual:
						lbInt := int64(math.Ceil(r.rhs - maxRoundErr - presolveIntTol))
						r.rhs = float64(divCeilInt64(lbInt, g))
					case Equal:
						loInt := int64(math.Ceil(r.rhs - presolveFeasTol - maxRoundErr))
						hiInt := int64(math.Floor(r.rhs + presolveFeasTol + maxRoundErr))
						q := divCeilInt64(loInt, g)
						if q*g > hiInt {
							return PresolveResult{Infeasible: true}
						}
						r.rhs = float64(q)
					}
					strengthened++
				}
			} else if allInt && g == 1 && math.Abs(r.rhs) <= 0x1p52 {
				if maxRoundErr > 0 {
					r.ensureOwned()
					for k := range r.vals {
						r.vals[k] = math.Round(r.vals[k])
					}
				}
				switch r.sense {
				case LessEqual:
					fl := math.Floor(r.rhs + maxRoundErr + presolveIntTol)
					if fl < r.rhs-presolveBoundChangeTol {
						r.rhs = fl
						strengthened++
					}
				case GreaterEqual:
					cl := math.Ceil(r.rhs - maxRoundErr - presolveIntTol)
					if cl > r.rhs+presolveBoundChangeTol {
						r.rhs = cl
						strengthened++
					}
				case Equal:
					nearest := math.Round(r.rhs)
					if math.Abs(r.rhs-nearest) > presolveFeasTol+maxRoundErr {
						return PresolveResult{Infeasible: true}
					}
					r.rhs = nearest
				}
			}
		}
	}

	// 2. Iterative Savelsbergh activity bound tightening and redundant row dropping
	actTight, actRem, infeas, timedOut := runActivityTightening(orig, rows, lb, ub, maxPasses, deadline, done)
	if infeas {
		return PresolveResult{Infeasible: true}
	}
	if timedOut {
		return PresolveResult{Reduced: orig, TimedOut: true}
	}
	tightened += actTight
	removedRows += actRem

	// 3. Algebraic orbit symmetry breaking on symmetric group formulations
	if orig.isMIP {
		symTight, symRows, symTimedOut := applyOrbitSymmetryBreaking(orig, rows, lb, ub, deadline, done)
		if symTimedOut {
			return PresolveResult{Reduced: orig, TimedOut: true}
		}
		if len(symRows) > 0 {
			rows = slices.Concat(rows, symRows)
			m = len(rows)
			strengthened += len(symRows)
		}
		if symTight > 0 {
			tightened += symTight
			postTight, postRem, postInfeas, postTimedOut := runActivityTightening(orig, rows, lb, ub, maxPasses, deadline, done)
			if postInfeas {
				return PresolveResult{Infeasible: true}
			}
			if postTimedOut {
				return PresolveResult{Reduced: orig, TimedOut: true}
			}
			tightened += postTight
			removedRows += postRem
		}
	}

	// 4. Binary Coefficient Tightening (Savelsbergh)
	if orig.isMIP {
		for i := range m {
			r := &rows[i]
			if !r.active || r.sense != LessEqual {
				continue
			}
			s, _ := evalRowActivitySummary(r.cols, r.vals, lb, ub, r.rhs, false, false, true)
			if s.extreme || s.maxInfCount > 0 {
				continue
			}
			binTol := max(presolveFeasTol, presolveRelScaleTol*max(1.0, s.l1Max))
			surplusLo := -s.deficitHi
			surplusHi := -s.deficitLo
			if surplusLo > binTol {
				for k, col := range r.cols {
					if orig.varCat[col] == Binary && lb[col] == 0 && ub[col] == 1 {
						a := r.vals[k]
						if a > binTol && a-surplusHi > binTol {
							delta := a - surplusHi
							newVal := a - delta
							newRHS := r.rhs - delta
							tol := 1e-12 * max(1.0, math.Abs(delta))
							if delta > 0 && math.Abs((a-newVal)-delta) <= tol && math.Abs((r.rhs-newRHS)-delta) <= tol {
								r.ensureOwned()
								r.vals[k] = newVal
								r.rhs = newRHS
								strengthened++
							}
						} else if a < -binTol && -a-surplusHi > binTol {
							delta := -a - surplusHi
							newVal := a + delta
							tol := 1e-12 * max(1.0, math.Abs(delta))
							if delta > 0 && math.Abs((newVal-a)-delta) <= tol {
								r.ensureOwned()
								r.vals[k] = newVal
								strengthened++
							}
						}
					}
				}
			}
		}
	}

	if tightened == 0 && removedRows == 0 && strengthened == 0 {
		return PresolveResult{Reduced: orig}
	}

	red, droppedEmpty, infeas := buildCompiledDirect(orig, rows, lb, ub)
	if infeas {
		return PresolveResult{Infeasible: true}
	}
	removedRows += droppedEmpty
	return PresolveResult{
		Reduced:          red,
		TightenedBounds:  tightened,
		RemovedRows:      removedRows,
		StrengthenedRows: strengthened,
	}
}

// buildCompiledDirect constructs a canonical CompiledProblem directly from presolved
// rows and variable bounds, bypassing symbolic Model construction, map lookups,
// validation, and redundant sorting.
func buildCompiledDirect(orig *CompiledProblem, rows []presolveWorkRow, lb, ub []float64) (red *CompiledProblem, droppedEmpty int, infeas bool) {
	n := orig.numVars
	mc := 0
	totalNNZ := 0
	for i := range rows {
		r := &rows[i]
		if !r.active {
			continue
		}
		keptTerms := 0
		rowNNZ := 0
		for k, col := range r.cols {
			if lb[col] == 0 && ub[col] == 0 {
				continue
			}
			keptTerms++
			if r.vals[k] != 0 {
				rowNNZ++
			}
		}
		if keptTerms == 0 {
			switch r.sense {
			case LessEqual:
				if 0 > r.rhs+presolveFeasTol {
					return nil, 0, true
				}
			case GreaterEqual:
				if 0 < r.rhs-presolveFeasTol {
					return nil, 0, true
				}
			case Equal:
				if math.Abs(r.rhs) > presolveFeasTol {
					return nil, 0, true
				}
			}
			r.active = false
			droppedEmpty++
			continue
		}
		mc++
		totalNNZ += rowNNZ
	}

	sense := orig.Sense()
	constMinVal := orig.objConstant
	if sense == Maximize && constMinVal != 0 {
		constMinVal = -constMinVal
	}

	red = newCompiledProblem()
	red.name = orig.Name()
	red.sense = sense
	red.modelID = orig.modelID
	red.numVars = n
	red.numConstraints = mc
	red.objName = orig.objName
	red.objConstant = orig.objConstant
	red.objConstMinVal = constMinVal

	if len(orig.varNames) == n {
		red.varNames = orig.varNames
	} else {
		red.varNames = make([]string, n)
	}
	if len(orig.objCoeffs) == n && len(orig.objMin) == n {
		red.objCoeffs = orig.objCoeffs
		red.objMin = orig.objMin
	} else {
		red.objCoeffs = make([]float64, n)
		if sense == Maximize {
			red.objMin = make([]float64, n)
		} else {
			red.objMin = red.objCoeffs
		}
	}

	red.rowNames = make([]string, mc)
	red.rowSense = make([]ConstraintSense, mc)
	red.rowRHS = make([]float64, mc)
	red.rowLB = make([]float64, mc)
	red.rowUB = make([]float64, mc)
	red.rowPtr = make([]int, mc+1)
	red.colIdx = make([]int, 0, totalNNZ)
	red.rowVal = make([]float64, 0, totalNNZ)

	rowOut := 0
	for i := range rows {
		r := &rows[i]
		if !r.active {
			continue
		}
		red.rowNames[rowOut] = r.name
		red.rowSense[rowOut] = r.sense
		red.rowRHS[rowOut] = r.rhs
		switch r.sense {
		case LessEqual:
			red.rowLB[rowOut] = math.Inf(-1)
			red.rowUB[rowOut] = r.rhs
		case GreaterEqual:
			red.rowLB[rowOut] = r.rhs
			red.rowUB[rowOut] = math.Inf(1)
		case Equal:
			red.rowLB[rowOut] = r.rhs
			red.rowUB[rowOut] = r.rhs
		}

		rowStart := len(red.colIdx)
		red.rowPtr[rowOut] = rowStart
		prevCol := -1
		strictAsc := true
		for k, col := range r.cols {
			if lb[col] == 0 && ub[col] == 0 {
				continue
			}
			val := r.vals[k]
			if val == 0 {
				continue
			}
			if col <= prevCol {
				strictAsc = false
			}
			prevCol = col
			red.colIdx = append(red.colIdx, col)
			red.rowVal = append(red.rowVal, val)
		}
		if !strictAsc {
			canonicalizeCSRRow(&red.colIdx, &red.rowVal, rowStart)
		}
		rowOut++
	}
	red.rowPtr[mc] = len(red.colIdx)

	stride := denseRowStride(n, mc, len(red.colIdx))
	if stride > 0 {
		red.rowStride = stride
		red.denseRows = make([]float64, mc*stride)
		for i := range mc {
			denseRow := red.denseRows[i*stride : (i+1)*stride]
			for k := red.rowPtr[i]; k < red.rowPtr[i+1]; k++ {
				denseRow[red.colIdx[k]] = red.rowVal[k]
			}
		}
	} else if n > 0 && mc > 0 {
		red.rowStride = sparseNoDenseStride
	}

	buildCSCFromCSR(red)

	numInt := 0
	for j := range n {
		if orig.varCat[j] != Continuous {
			numInt++
		}
	}
	numWords := (n + wordBitMask) / bitsPerWord
	red.varLB = lb
	red.varUB = ub
	red.varCat = make([]VarCategory, n)
	red.binaryMask = make([]uint64, numWords)
	if numInt > 0 {
		red.isMIP = true
		red.intCols = make([]int, 0, numInt)
	}
	for j := range n {
		cat := orig.varCat[j]
		rlb, rub := roundBoundsForCategory(lb[j], ub[j], cat)
		lb[j] = rlb
		ub[j] = rub
		if cat == Integer && rlb >= 0 && rub <= 1 && rlb <= rub {
			cat = Binary
		}
		red.varCat[j] = cat
		if cat != Continuous {
			red.intCols = append(red.intCols, j)
			if cat == Binary {
				red.binaryMask[j/bitsPerWord] |= 1 << (uint(j) % bitsPerWord)
			}
		}
	}

	return red, droppedEmpty, false
}

// canonicalizeCSRRow sorts the newly appended CSR row slice [rowStart:] by ascending
// column index, sums duplicate columns via wideDotAcc, and drops resulting zero entries.
func canonicalizeCSRRow(colIdx *[]int, rowVal *[]float64, rowStart int) {
	cols := (*colIdx)[rowStart:]
	vals := (*rowVal)[rowStart:]
	k := len(cols)
	if k <= 1 {
		return
	}
	if k == 2 && cols[0] != cols[1] {
		if cols[0] > cols[1] {
			cols[0], cols[1] = cols[1], cols[0]
			vals[0], vals[1] = vals[1], vals[0]
		}
		return
	}
	type colValPair struct {
		col int
		val float64
	}
	pairs := make([]colValPair, k)
	for i := range k {
		pairs[i] = colValPair{col: cols[i], val: vals[i]}
	}
	slices.SortFunc(pairs, func(a, b colValPair) int {
		return cmp.Compare(a.col, b.col)
	})
	w := 0
	for i := 0; i < k; {
		start := i
		c := pairs[i].col
		sum := pairs[i].val
		for i++; i < k && pairs[i].col == c; i++ {
			sum += pairs[i].val
		}
		if i-start > 1 {
			var acc wideDotAcc
			for p := start; p < i; p++ {
				acc.addVal(pairs[p].val)
			}
			sum = acc.finish(math.NaN())
		}
		if sum == 0 {
			continue
		}
		cols[w] = c
		vals[w] = sum
		w++
	}
	*colIdx = (*colIdx)[:rowStart+w]
	*rowVal = (*rowVal)[:rowStart+w]
}

// fastRowCanSkip performs an O(rowLen) float64 pre-check with conservative ULP and
// feasibility margins to skip exact dot2Acc/wideDotAcc activity evaluation when a row
// can neither prove infeasibility, be dropped as redundant, nor tighten any variable bound.
// Extreme-scale rows (|a| > 1e12, 0 < |a| < 1e-12, |term| > 1e12, or |rhs| > 1e12)
// always fall back to exact activity evaluation.
func fastRowCanSkip(orig *CompiledProblem, r *presolveWorkRow, lb, ub []float64) bool {
	if len(r.cols) > presolveFastMaxRowLen {
		return false
	}
	absRHS := math.Abs(r.rhs)
	if math.IsNaN(absRHS) || absRHS > presolveFastMaxAbs {
		return false
	}
	minAct, maxAct := 0.0, 0.0
	absSum := absRHS
	minInf, maxInf := 0, 0
	maxSpan := 0.0
	hasInfSpan := false
	hasFracInt := false
	for k, col := range r.cols {
		a := r.vals[k]
		absA := math.Abs(a)
		if absA == 0 {
			continue
		}
		if math.IsNaN(absA) || absA > presolveFastMaxAbs || absA < presolveFastMinAbs {
			return false
		}
		l, u := lb[col], ub[col]
		if math.IsNaN(l) || math.IsNaN(u) {
			return false
		}
		if orig.varCat[col] != Continuous {
			if (!math.IsInf(l, 0) && l != math.Trunc(l)) || (!math.IsInf(u, 0) && u != math.Trunc(u)) {
				hasFracInt = true
			}
		}
		if a > 0 {
			if math.IsInf(l, -1) {
				minInf++
			} else {
				p := a * l
				ap := math.Abs(p)
				if ap > presolveFastMaxAbs {
					return false
				}
				minAct += p
				absSum += ap
			}
			if math.IsInf(u, 1) {
				maxInf++
			} else {
				p := a * u
				ap := math.Abs(p)
				if ap > presolveFastMaxAbs {
					return false
				}
				maxAct += p
				absSum += ap
			}
		} else {
			if math.IsInf(u, 1) {
				minInf++
			} else {
				p := a * u
				ap := math.Abs(p)
				if ap > presolveFastMaxAbs {
					return false
				}
				minAct += p
				absSum += ap
			}
			if math.IsInf(l, -1) {
				maxInf++
			} else {
				p := a * l
				ap := math.Abs(p)
				if ap > presolveFastMaxAbs {
					return false
				}
				maxAct += p
				absSum += ap
			}
		}
		span := u - l
		if math.IsInf(span, 0) || math.IsNaN(span) {
			hasInfSpan = true
		} else if span > 0 {
			sProd := absA * span
			if sProd > maxSpan {
				maxSpan = sProd
			}
		}
	}
	if absSum > presolveFastMaxAbs || math.IsNaN(absSum) || math.IsNaN(minAct) || math.IsNaN(maxAct) {
		return false
	}
	margin := presolveFastRelMargin*max(1.0, absSum) + 2.0*presolveFeasTol
	slackFast := r.rhs - minAct
	deficitFast := r.rhs - maxAct
	hasUpper := r.sense == LessEqual || r.sense == Equal
	hasLower := r.sense == GreaterEqual || r.sense == Equal
	if hasUpper && minInf == 0 && slackFast < margin {
		return false
	}
	if hasLower && maxInf == 0 && deficitFast > -margin {
		return false
	}
	if r.sense == LessEqual && maxInf == 0 && deficitFast >= -margin {
		return false
	}
	if r.sense == GreaterEqual && minInf == 0 && slackFast <= margin {
		return false
	}
	if r.sense == Equal && minInf == 0 && maxInf == 0 && slackFast <= margin && deficitFast >= -margin {
		return false
	}
	if hasUpper && minInf <= 1 {
		if minInf == 1 || hasInfSpan || hasFracInt || slackFast <= maxSpan+margin {
			return false
		}
	}
	if hasLower && maxInf <= 1 {
		if maxInf == 1 || hasInfSpan || hasFracInt || -deficitFast <= maxSpan+margin {
			return false
		}
	}
	return true
}

// presolveCanSkipTerm reports whether slackLo (a certified lower bound on row slack)
// strictly exceeds absA*(u-l) with a conservative relative margin so residualQuotient
// cannot tighten targetBound.
func presolveCanSkipTerm(slackLo, absA, l, u, targetBound float64, isInt bool) bool {
	if slackLo <= 0 || math.IsInf(targetBound, 0) {
		return false
	}
	if isInt && targetBound != math.Trunc(targetBound) {
		return false
	}
	span := u - l
	if span < 0 || math.IsNaN(span) || math.IsInf(span, 0) {
		return false
	}
	prod := absA * span
	if math.IsNaN(prod) || math.IsInf(prod, 0) {
		return false
	}
	return slackLo > prod+presolveSlackGuardRelMargin*prod
}

// runActivityTightening executes up to maxPasses rounds of Savelsbergh row-activity bound tightening and drops redundant rows.
func runActivityTightening(
	orig *CompiledProblem,
	rows []presolveWorkRow,
	lb, ub []float64,
	maxPasses int,
	deadline time.Time,
	done <-chan struct{},
) (tightened, removedRows int, infeas, timedOut bool) {
	m := len(rows)
	for range maxPasses {
		if stopRequested(deadline, done) {
			return tightened, removedRows, false, true
		}
		changedInPass := false

		for i := range m {
			if (i&63 == 0) && stopRequested(deadline, done) {
				return tightened, removedRows, false, true
			}
			r := &rows[i]
			if !r.active {
				continue
			}
			if fastRowCanSkip(orig, r, lb, ub) {
				continue
			}

			hasUpper := r.sense == LessEqual || r.sense == Equal
			hasLower := r.sense == GreaterEqual || r.sense == Equal
			s, _ := evalRowActivitySummary(r.cols, r.vals, lb, ub, r.rhs, false, true, true)
			if s.extreme {
				maxExp := 0
				if r.rhs != 0 && !math.IsInf(r.rhs, 0) && !math.IsNaN(r.rhs) {
					if _, re := math.Frexp(math.Abs(r.rhs)); re > maxExp {
						maxExp = re
					}
				}
				for k, col := range r.cols {
					a := r.vals[k]
					if a == 0 {
						continue
					}
					_, ea := math.Frexp(math.Abs(a))
					if l := lb[col]; !math.IsInf(l, 0) && l != 0 {
						if _, el := math.Frexp(math.Abs(l)); ea+el > maxExp {
							maxExp = ea + el
						}
					}
					if u := ub[col]; !math.IsInf(u, 0) && u != 0 {
						if _, eu := math.Frexp(math.Abs(u)); ea+eu > maxExp {
							maxExp = ea + eu
						}
					}
				}
				shift := max(32, maxExp-wideExpCutoff)
				s1 := shift / 2
				s2 := shift - s1
				var slackW, defW wideDotAcc
				rhsS := math.Ldexp(r.rhs, -shift)
				slackW.addVal(rhsS)
				defW.addVal(rhsS)
				l1Scaled := math.Abs(rhsS)
				for k, col := range r.cols {
					a := r.vals[k]
					l, u := lb[col], ub[col]
					as := math.Ldexp(a, -s1)
					if a > 0 {
						if !math.IsInf(l, -1) {
							ls := math.Ldexp(l, -s2)
							slackW.addProd(-as, ls)
							l1Scaled += math.Abs(as * ls)
						}
						if !math.IsInf(u, 1) {
							us := math.Ldexp(u, -s2)
							defW.addProd(-as, us)
							l1Scaled += math.Abs(as * us)
						}
					} else if a < 0 {
						if !math.IsInf(u, 1) {
							us := math.Ldexp(u, -s2)
							slackW.addProd(-as, us)
							l1Scaled += math.Abs(as * us)
						}
						if !math.IsInf(l, -1) {
							ls := math.Ldexp(l, -s2)
							defW.addProd(-as, ls)
							l1Scaled += math.Abs(as * ls)
						}
					}
				}
				slackS := slackW.finish(math.NaN())
				defS := defW.finish(math.NaN())
				if math.IsNaN(slackS) || math.IsInf(slackS, 0) || math.IsNaN(defS) || math.IsInf(defS, 0) || math.IsNaN(l1Scaled) || math.IsInf(l1Scaled, 0) {
					continue
				}
				errS := 0x1p-50 * (math.Abs(slackS) + math.Abs(defS))
				tolS := max(math.Ldexp(presolveFeasTol, -shift), presolveRelScaleTol*l1Scaled) + errS
				chgS := math.Ldexp(presolveBoundChangeTol, -shift)
				if hasUpper && s.minInfCount == 0 && slackS+errS < -tolS {
					return tightened, removedRows, true, false
				}
				if hasLower && s.maxInfCount == 0 && defS-errS > tolS {
					return tightened, removedRows, true, false
				}
				if r.sense == LessEqual && s.maxInfCount == 0 && defS-errS >= -chgS {
					r.active = false
					removedRows++
					continue
				}
				if r.sense == GreaterEqual && s.minInfCount == 0 && slackS+errS <= chgS {
					r.active = false
					removedRows++
					continue
				}
				if r.sense == Equal && s.minInfCount == 0 && s.maxInfCount == 0 && slackS+errS <= chgS && defS-errS >= -chgS {
					r.active = false
					removedRows++
					continue
				}
				continue
			}

			rowScale := max(1.0, max(s.l1Min, s.l1Max))
			rowFeasTol := max(presolveFeasTol, presolveRelScaleTol*rowScale)

			if hasUpper && s.minInfCount == 0 && s.slackHi < -rowFeasTol {
				return tightened, removedRows, true, false
			}
			if hasLower && s.maxInfCount == 0 && s.deficitLo > rowFeasTol {
				return tightened, removedRows, true, false
			}

			if r.sense == LessEqual && s.maxInfCount == 0 && s.deficitLo >= -presolveBoundChangeTol {
				r.active = false
				removedRows++
				continue
			}
			if r.sense == GreaterEqual && s.minInfCount == 0 && s.slackHi <= presolveBoundChangeTol {
				r.active = false
				removedRows++
				continue
			}
			if r.sense == Equal && s.minInfCount == 0 && s.maxInfCount == 0 && s.slackHi <= presolveBoundChangeTol && s.deficitLo >= -presolveBoundChangeTol {
				r.active = false
				removedRows++
				continue
			}

			upperTightenedRow := false
			if hasUpper && s.minInfCount <= 1 {
				canGuardUB := s.minInfCount == 0 && !s.useWideSlack
				ubSlackLo := s.slackLo
				for k, col := range r.cols {
					a := r.vals[k]
					if math.Abs(a) <= presolveZeroTol {
						continue
					}
					l, u := lb[col], ub[col]
					isInt := orig.varCat[col] != Continuous
					if a > 0 {
						if s.minInfCount == 1 && !math.IsInf(l, -1) {
							continue
						}
						if canGuardUB && presolveCanSkipTerm(ubSlackLo, a, l, u, u, isInt) {
							continue
						}
						qVal, qEB, qOK := s.residualQuotient(r.cols, r.vals, lb, ub, r.rhs, k, true)
						if !qOK {
							continue
						}
						newUB := qVal + qEB
						bTol := max(presolveIntTol, presolveRelScaleTol*rowScale/a)
						if isInt {
							newUB = math.Floor(newUB + bTol)
						} else {
							newUB += rowFeasTol / a
						}
						if newUB < u-presolveIntTol {
							feasCheck := max(presolveFeasTol, rowFeasTol/a)
							if lb[col] > newUB+feasCheck {
								return tightened, removedRows, true, false
							}
							if lb[col] > newUB {
								newUB = lb[col]
							}
							if newUB < u-presolveIntTol {
								ub[col] = newUB
								tightened++
								changedInPass = true
								upperTightenedRow = true
							}
						}
					} else {
						if s.minInfCount == 1 && !math.IsInf(u, 1) {
							continue
						}
						absA := -a
						if canGuardUB && presolveCanSkipTerm(ubSlackLo, absA, l, u, l, isInt) {
							continue
						}
						qVal, qEB, qOK := s.residualQuotient(r.cols, r.vals, lb, ub, r.rhs, k, true)
						if !qOK {
							continue
						}
						newLB := qVal - qEB
						bTol := max(presolveIntTol, presolveRelScaleTol*rowScale/absA)
						if isInt {
							newLB = math.Ceil(newLB - bTol)
						} else {
							newLB -= rowFeasTol / absA
						}
						if newLB > l+presolveIntTol {
							feasCheck := max(presolveFeasTol, rowFeasTol/absA)
							if newLB > ub[col]+feasCheck {
								return tightened, removedRows, true, false
							}
							if newLB > ub[col] {
								newLB = ub[col]
							}
							if newLB > l+presolveIntTol {
								lb[col] = newLB
								tightened++
								changedInPass = true
								upperTightenedRow = true
							}
						}
					}
				}
			}

			if hasLower && upperTightenedRow {
				s, _ = evalRowActivitySummary(r.cols, r.vals, lb, ub, r.rhs, false, false, true)
				if s.extreme {
					continue
				}
				rowScale = max(1.0, s.l1Max)
				rowFeasTol = max(presolveFeasTol, presolveRelScaleTol*rowScale)
				if s.maxInfCount == 0 && s.deficitLo > rowFeasTol {
					return tightened, removedRows, true, false
				}
			}

			if hasLower && s.maxInfCount <= 1 {
				canGuardLB := s.maxInfCount == 0 && !s.useWideDeficit
				lbSlackLo := -s.deficitHi
				for k, col := range r.cols {
					a := r.vals[k]
					if math.Abs(a) <= presolveZeroTol {
						continue
					}
					l, u := lb[col], ub[col]
					isInt := orig.varCat[col] != Continuous
					if a > 0 {
						if s.maxInfCount == 1 && !math.IsInf(u, 1) {
							continue
						}
						if canGuardLB && presolveCanSkipTerm(lbSlackLo, a, l, u, l, isInt) {
							continue
						}
						qVal, qEB, qOK := s.residualQuotient(r.cols, r.vals, lb, ub, r.rhs, k, false)
						if !qOK {
							continue
						}
						newLB := qVal - qEB
						bTol := max(presolveIntTol, presolveRelScaleTol*rowScale/a)
						if isInt {
							newLB = math.Ceil(newLB - bTol)
						} else {
							newLB -= rowFeasTol / a
						}
						if newLB > l+presolveIntTol {
							feasCheck := max(presolveFeasTol, rowFeasTol/a)
							if newLB > ub[col]+feasCheck {
								return tightened, removedRows, true, false
							}
							if newLB > ub[col] {
								newLB = ub[col]
							}
							if newLB > l+presolveIntTol {
								lb[col] = newLB
								tightened++
								changedInPass = true
							}
						}
					} else {
						if s.maxInfCount == 1 && !math.IsInf(l, -1) {
							continue
						}
						absA := -a
						if canGuardLB && presolveCanSkipTerm(lbSlackLo, absA, l, u, u, isInt) {
							continue
						}
						qVal, qEB, qOK := s.residualQuotient(r.cols, r.vals, lb, ub, r.rhs, k, false)
						if !qOK {
							continue
						}
						newUB := qVal + qEB
						bTol := max(presolveIntTol, presolveRelScaleTol*rowScale/absA)
						if isInt {
							newUB = math.Floor(newUB + bTol)
						} else {
							newUB += rowFeasTol / absA
						}
						if newUB < u-presolveIntTol {
							feasCheck := max(presolveFeasTol, rowFeasTol/absA)
							if lb[col] > newUB+feasCheck {
								return tightened, removedRows, true, false
							}
							if lb[col] > newUB {
								newUB = lb[col]
							}
							if newUB < u-presolveIntTol {
								ub[col] = newUB
								tightened++
								changedInPass = true
							}
						}
					}
				}
			}
		}

		if !changedInPass {
			break
		}
	}
	return tightened, removedRows, false, false
}

type rowActivitySummary struct {
	slackAcc             dot2Acc
	deficitAcc           dot2Acc
	slackLo, slackHi     float64
	deficitLo, deficitHi float64
	l1Min, l1Max         float64
	minInfCount          int
	maxInfCount          int
	useWideSlack         bool
	useWideDeficit       bool
	extreme              bool
}

func evalRowActivitySummary(
	cols []int,
	vals, lb, ub []float64,
	rhs float64,
	bailBothInf, hasUpper, hasLower bool,
) (s rowActivitySummary, earlyBail bool) {
	if math.IsNaN(rhs) || math.IsInf(rhs, 0) {
		s.extreme = true
		return s, false
	}
	if hasUpper {
		s.slackAcc.addVal(rhs)
	}
	if hasLower {
		s.deficitAcc.addVal(rhs)
	}
	for k, col := range cols {
		a := vals[k]
		l, u := lb[col], ub[col]
		if a > 0 {
			if hasUpper {
				if math.IsInf(l, -1) {
					s.minInfCount++
				} else {
					s.slackAcc.addProd(-a, l)
					if l < u {
						s.l1Min += math.Abs(a * l)
					}
				}
			}
			if hasLower {
				if math.IsInf(u, 1) {
					s.maxInfCount++
				} else {
					s.deficitAcc.addProd(-a, u)
					if l < u {
						s.l1Max += math.Abs(a * u)
					}
				}
			}
		} else if a < 0 {
			if hasUpper {
				if math.IsInf(u, 1) {
					s.minInfCount++
				} else {
					s.slackAcc.addProd(-a, u)
					if l < u {
						s.l1Min += math.Abs(a * u)
					}
				}
			}
			if hasLower {
				if math.IsInf(l, -1) {
					s.maxInfCount++
				} else {
					s.deficitAcc.addProd(-a, l)
					if l < u {
						s.l1Max += math.Abs(a * l)
					}
				}
			}
		}
		if bailBothInf && (!hasUpper || s.minInfCount > 0) && (!hasLower || s.maxInfCount > 0) {
			return s, true
		}
	}

	if math.IsNaN(s.l1Min) || math.IsInf(s.l1Min, 0) || math.IsNaN(s.l1Max) || math.IsInf(s.l1Max, 0) {
		s.extreme = true
		return s, false
	}

	if hasUpper {
		if val, eb, ok := s.slackAcc.result(); ok && eb <= 0x1p-20*max(1.0, math.Abs(val)) {
			s.slackLo = val - eb
			s.slackHi = val + eb
		} else {
			s.useWideSlack = true
			var w wideDotAcc
			w.addVal(rhs)
			for k, col := range cols {
				a := vals[k]
				if a > 0 && !math.IsInf(lb[col], -1) {
					w.addProd(-a, lb[col])
				} else if a < 0 && !math.IsInf(ub[col], 1) {
					w.addProd(-a, ub[col])
				}
			}
			val := w.finish(math.NaN())
			if math.IsNaN(val) || math.IsInf(val, 0) {
				s.extreme = true
				return s, false
			}
			ulp := 0x1p-50 * math.Abs(val)
			s.slackLo = val - ulp
			s.slackHi = val + ulp
		}
	}

	if hasLower {
		if val, eb, ok := s.deficitAcc.result(); ok && eb <= 0x1p-20*max(1.0, math.Abs(val)) {
			s.deficitLo = val - eb
			s.deficitHi = val + eb
		} else {
			s.useWideDeficit = true
			var w wideDotAcc
			w.addVal(rhs)
			for k, col := range cols {
				a := vals[k]
				if a > 0 && !math.IsInf(ub[col], 1) {
					w.addProd(-a, ub[col])
				} else if a < 0 && !math.IsInf(lb[col], -1) {
					w.addProd(-a, lb[col])
				}
			}
			val := w.finish(math.NaN())
			if math.IsNaN(val) || math.IsInf(val, 0) {
				s.extreme = true
				return s, false
			}
			ulp := 0x1p-50 * math.Abs(val)
			s.deficitLo = val - ulp
			s.deficitHi = val + ulp
		}
	}

	return s, false
}

func (s *rowActivitySummary) residualQuotient(
	cols []int,
	vals, lb, ub []float64,
	rhs float64,
	skipIdx int,
	isUpper bool,
) (qVal, qEB float64, ok bool) {
	a := vals[skipIdx]
	col := cols[skipIdx]
	if isUpper {
		if !s.useWideSlack {
			acc := s.slackAcc
			if s.minInfCount == 0 {
				if a > 0 {
					acc.addProd(a, lb[col])
				} else {
					acc.addProd(a, ub[col])
				}
			}
			if val, eb, accOK := acc.result(); accOK && eb <= 0x1p-20*max(1.0, math.Abs(val)) {
				if q, qErr, divOK := divDDByFloat(acc.s, acc.c, a, eb); divOK {
					return q, qErr, true
				}
			}
		}
		var w wideDotAcc
		w.addVal(rhs)
		for k, c := range cols {
			if k == skipIdx {
				continue
			}
			ak := vals[k]
			if ak > 0 && !math.IsInf(lb[c], -1) {
				w.addProd(-ak, lb[c])
			} else if ak < 0 && !math.IsInf(ub[c], 1) {
				w.addProd(-ak, ub[c])
			}
		}
		top, rem, wOK := w.finishDD()
		if !wOK {
			return 0, 0, false
		}
		return divDDByFloat(top, rem, a, 0)
	}

	if !s.useWideDeficit {
		acc := s.deficitAcc
		if s.maxInfCount == 0 {
			if a > 0 {
				acc.addProd(a, ub[col])
			} else {
				acc.addProd(a, lb[col])
			}
		}
		if val, eb, accOK := acc.result(); accOK && eb <= 0x1p-20*max(1.0, math.Abs(val)) {
			if q, qErr, divOK := divDDByFloat(acc.s, acc.c, a, eb); divOK {
				return q, qErr, true
			}
		}
	}
	var w wideDotAcc
	w.addVal(rhs)
	for k, c := range cols {
		if k == skipIdx {
			continue
		}
		ak := vals[k]
		if ak > 0 && !math.IsInf(ub[c], 1) {
			w.addProd(-ak, ub[c])
		} else if ak < 0 && !math.IsInf(lb[c], -1) {
			w.addProd(-ak, lb[c])
		}
	}
	top, rem, wOK := w.finishDD()
	if !wOK {
		return 0, 0, false
	}
	return divDDByFloat(top, rem, a, 0)
}

type orbitTrialScratch struct {
	colOrbit      []int
	colGroup      []int
	rowState      []int
	touchedRows   []int
	orderBuf      []int
	seedCols      []int
	orbits        []int // flat: numOrbits * gCount
	rowTuples     []int // flat: numTuples * gCount
	expectedStamp []int
	expectedVal   []float64
	orbSeen       []int
	orbCount      []int
	orbVal        []float64
	stamp         int
	invStamp      int
}

func newOrbitTrialScratch(n, m int) orbitTrialScratch {
	colOrbit := make([]int, n)
	colGroup := make([]int, n)
	for j := range n {
		colOrbit[j] = orbitUnassigned
		colGroup[j] = orbitUnassigned
	}
	rowState := make([]int, m)
	for i := range m {
		rowState[i] = rowStateUnvisited
	}
	return orbitTrialScratch{
		colOrbit: colOrbit,
		colGroup: colGroup,
		rowState: rowState,
		seedCols: make([]int, 0, maxOrbitGroupSize),
	}
}

func (sc *orbitTrialScratch) reset() {
	for _, c := range sc.orbits {
		sc.colOrbit[c] = orbitUnassigned
		sc.colGroup[c] = orbitUnassigned
	}
	for _, r := range sc.touchedRows {
		sc.rowState[r] = rowStateUnvisited
	}
	sc.orbits = sc.orbits[:0]
	sc.rowTuples = sc.rowTuples[:0]
	sc.touchedRows = sc.touchedRows[:0]
}

func presolveSplitmix64(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}

func canonicalFloatBits(v float64) uint64 {
	if v == 0 {
		return 0
	}
	return math.Float64bits(v)
}

func samePresolveBound(a, b float64) bool {
	return a == b
}

// applyOrbitSymmetryBreaking detects exact symmetric group actions on constraint rows and
// variable orbits, applying static orbitope/bijection fixings when objective costs are
// group-invariant or empty-block ordering inequalities when block costs are monotone and
// gated by a binary block-open indicator.
func applyOrbitSymmetryBreaking(
	orig *CompiledProblem,
	rows []presolveWorkRow,
	lb, ub []float64,
	deadline time.Time,
	done <-chan struct{},
) (tightened int, addedRows []presolveWorkRow, timedOut bool) {
	n := orig.numVars
	m := len(rows)
	if n < 4 || m < 2 {
		return 0, nil, false
	}

	var seedRows []int
	for i := range m {
		r := &rows[i]
		g := len(r.cols)
		if !r.active || g < 2 || g > maxOrbitGroupSize || g*2 > n {
			continue
		}
		if r.sense != Equal && r.sense != LessEqual {
			continue
		}
		if r.rhs != 1.0 {
			continue
		}
		unitBin := true
		for k, col := range r.cols {
			if r.vals[k] != 1.0 || orig.varCat[col] != Binary || lb[col] != 0 || ub[col] != 1 {
				unitBin = false
				break
			}
		}
		if unitBin {
			seedRows = append(seedRows, i)
		}
	}
	if len(seedRows) == 0 {
		return 0, nil, false
	}
	if stopRequested(deadline, done) {
		return 0, nil, true
	}

	// Reuse orig's CSC directly when every row is still active and unmodified;
	// otherwise build a flat two-pass CSC over active rows.
	reuseOrigCSC := m == orig.numConstraints && len(orig.colPtr) == n+1
	if reuseOrigCSC {
		for i := range m {
			if !rows[i].active || !rows[i].shared {
				reuseOrigCSC = false
				break
			}
		}
	}
	var colPtr, colRow []int
	var colVal []float64
	if reuseOrigCSC {
		colPtr = orig.colPtr
		colRow = orig.rowIdx
		colVal = orig.colVal
	} else {
		colPtr = make([]int, n+1)
		for i := range m {
			r := &rows[i]
			if !r.active {
				continue
			}
			for _, col := range r.cols {
				colPtr[col+1]++
			}
		}
		for j := range n {
			colPtr[j+1] += colPtr[j]
		}
		nnz := colPtr[n]
		colRow = make([]int, nnz)
		colVal = make([]float64, nnz)
		for i := range m {
			r := &rows[i]
			if !r.active {
				continue
			}
			for k, col := range r.cols {
				pos := colPtr[col]
				colRow[pos] = i
				colVal[pos] = r.vals[k]
				colPtr[col] = pos + 1
			}
		}
		copy(colPtr[1:], colPtr[:n])
		colPtr[0] = 0
	}

	filteredSeeds := seedRows[:0]
	for _, sr := range seedRows {
		cols := rows[sr].cols
		deg0 := colPtr[cols[0]+1] - colPtr[cols[0]]
		if deg0 < 2 {
			continue
		}
		sameDeg := true
		for _, c := range cols[1:] {
			if colPtr[c+1]-colPtr[c] != deg0 {
				sameDeg = false
				break
			}
		}
		if sameDeg {
			filteredSeeds = append(filteredSeeds, sr)
		}
	}
	seedRows = filteredSeeds
	if len(seedRows) == 0 {
		return 0, nil, false
	}
	slices.SortStableFunc(seedRows, func(a, b int) int {
		sa := rows[a].sense == Equal
		sb := rows[b].sense == Equal
		if sa != sb {
			if sa {
				return -1
			}
			return 1
		}
		return cmp.Compare(len(rows[b].cols), len(rows[a].cols))
	})

	rowHash := make([]uint64, m)
	for i := range m {
		r := &rows[i]
		if !r.active {
			continue
		}
		h := presolveSplitmix64(uint64(r.sense) ^ (canonicalFloatBits(r.rhs) << 3) ^ uint64(len(r.cols))*0x517cc1b727220a95)
		var sumH, xorH uint64
		for k, col := range r.cols {
			th := presolveSplitmix64(canonicalFloatBits(r.vals[k]))
			th ^= presolveSplitmix64(uint64(orig.varCat[col]) + 0x100)
			th ^= presolveSplitmix64(canonicalFloatBits(lb[col]) + 0x200)
			th ^= presolveSplitmix64(canonicalFloatBits(ub[col]) + 0x300)
			th ^= presolveSplitmix64(uint64(colPtr[col+1]-colPtr[col]) + 0x400)
			sumH += th
			xorH ^= bits.RotateLeft64(th, 17)
		}
		rowHash[i] = h ^ sumH ^ xorH
	}

	sc := newOrbitTrialScratch(n, m)
	maxTrials := min(len(seedRows), maxOrbitSeedTrials)
	for sIdx := range maxTrials {
		if stopRequested(deadline, done) {
			return 0, nil, true
		}
		if sIdx > 0 {
			sc.reset()
		}
		gCount, ok, to := proposeAndVerifyOrbits(
			orig, rows, lb, ub, colPtr, colRow, colVal, rowHash, seedRows[sIdx], &sc, deadline, done,
		)
		if to {
			return 0, nil, true
		}
		if !ok || len(sc.orbits) == 0 {
			continue
		}
		t, add := reduceVerifiedOrbits(orig, rows, lb, ub, sc.orbits, gCount, sc.colOrbit, sc.rowTuples)
		if t > 0 || len(add) > 0 {
			return t, add, false
		}
	}
	return 0, nil, false
}

// proposeAndVerifyOrbits expands a seed partitioning or packing row into G-way variable orbits
// and row tuples by BFS over the row-column incidence graph, then verifies exact matrix invariance.
// Sentinel encodings: colOrbit[j] and colGroup[j] hold orbitUnassigned (-1) until column j joins an orbit;
// rowState[i] holds rowStateUnvisited (-1), rowStateInvariant (-2) for rows invariant across all groups,
// or group index g in [0 .. G-1] when row i belongs to a G-row orbit tuple.
func proposeAndVerifyOrbits(
	orig *CompiledProblem,
	rows []presolveWorkRow,
	lb, ub []float64,
	colPtr, colRow []int,
	colVal []float64,
	rowHash []uint64,
	seedIdx int,
	sc *orbitTrialScratch,
	deadline time.Time,
	done <-chan struct{},
) (gCount int, ok, timedOut bool) {
	sc.seedCols = append(sc.seedCols[:0], rows[seedIdx].cols...)
	slices.Sort(sc.seedCols)
	gCount = len(sc.seedCols)
	for g := 1; g < gCount; g++ {
		if sc.seedCols[g] == sc.seedCols[g-1] {
			return 0, false, false
		}
	}

	colOrbit := sc.colOrbit
	colGroup := sc.colGroup
	rowState := sc.rowState

	sc.orbits = append(sc.orbits, sc.seedCols...)
	for g, c := range sc.seedCols {
		colOrbit[c] = 0
		colGroup[c] = g
	}

	rowState[seedIdx] = rowStateInvariant
	sc.touchedRows = append(sc.touchedRows, seedIdx)

	var orbBuf [maxOrbitGroupSize]int
	var tuple [maxOrbitGroupSize]int
	var newOrb [maxOrbitGroupSize]int

	for q := 0; q*gCount < len(sc.orbits); q++ {
		if (q&31 == 0) && stopRequested(deadline, done) {
			return 0, false, true
		}
		orb := orbBuf[:gCount]
		copy(orb, sc.orbits[q*gCount:(q+1)*gCount])
		cStart, cEnd := colPtr[orb[0]], colPtr[orb[0]+1]
		for p := cStart; p < cEnd; p++ {
			r0 := colRow[p]
			a0 := colVal[p]
			if rowState[r0] != rowStateUnvisited {
				continue
			}
			rr0 := &rows[r0]
			if isRowInvariantOnOrbit(rr0, orb, a0) {
				rowState[r0] = rowStateInvariant
				sc.touchedRows = append(sc.touchedRows, r0)
				continue
			}
			for _, c := range rr0.cols {
				if colGroup[c] > 0 {
					return 0, false, false
				}
			}

			tuple[0] = r0
			for g := 1; g < gCount; g++ {
				vg := orb[g]
				matched := -1
				vgStart, vgEnd := colPtr[vg], colPtr[vg+1]
				for pg := vgStart; pg < vgEnd; pg++ {
					rg := colRow[pg]
					if rowState[rg] != rowStateUnvisited || rg == r0 || rowHash[rg] != rowHash[r0] {
						continue
					}
					if colVal[pg] != a0 {
						continue
					}
					rrg := &rows[rg]
					if rrg.sense != rr0.sense || len(rrg.cols) != len(rr0.cols) || rrg.rhs != rr0.rhs {
						continue
					}
					dup := false
					for prev := 1; prev < g; prev++ {
						if tuple[prev] == rg {
							dup = true
							break
						}
					}
					if !dup {
						matched = rg
						break
					}
				}
				if matched == -1 {
					return 0, false, false
				}
				tuple[g] = matched
			}
			for g := range gCount {
				rg := tuple[g]
				rowState[rg] = g
				sc.touchedRows = append(sc.touchedRows, rg)
			}
			sc.rowTuples = append(sc.rowTuples, tuple[:gCount]...)

			rowLen := len(rr0.cols)
			needOrd := gCount * rowLen
			if cap(sc.orderBuf) < needOrd {
				sc.orderBuf = make([]int, needOrd)
			} else {
				sc.orderBuf = sc.orderBuf[:needOrd]
			}
			for g := range gCount {
				rg := &rows[tuple[g]]
				ord := sc.orderBuf[g*rowLen : (g+1)*rowLen]
				for idx := range rowLen {
					ord[idx] = idx
				}
				grp := g
				slices.SortFunc(ord, func(a, b int) int {
					ca, cb := rg.cols[a], rg.cols[b]
					ga, gb := colGroup[ca] == grp, colGroup[cb] == grp
					if ga != gb {
						if ga {
							return -1
						}
						return 1
					}
					if ga {
						return cmp.Compare(colOrbit[ca], colOrbit[cb])
					}
					if c := cmp.Compare(rg.vals[a], rg.vals[b]); c != 0 {
						return c
					}
					if c := cmp.Compare(orig.varCat[ca], orig.varCat[cb]); c != 0 {
						return c
					}
					if c := cmp.Compare(lb[ca], lb[cb]); c != 0 {
						return c
					}
					if c := cmp.Compare(ub[ca], ub[cb]); c != 0 {
						return c
					}
					if c := cmp.Compare(colPtr[ca+1]-colPtr[ca], colPtr[cb+1]-colPtr[cb]); c != 0 {
						return c
					}
					return cmp.Compare(ca, cb)
				})
			}

			ord0 := sc.orderBuf[:rowLen]
			for pos := range rowLen {
				c0 := rr0.cols[ord0[pos]]
				v0 := rr0.vals[ord0[pos]]
				allSameCol := true
				for g := 1; g < gCount; g++ {
					rg := &rows[tuple[g]]
					idxG := sc.orderBuf[g*rowLen+pos]
					cg := rg.cols[idxG]
					vg := rg.vals[idxG]
					if vg != v0 {
						return 0, false, false
					}
					if cg != c0 {
						allSameCol = false
					}
				}
				if allSameCol {
					if colOrbit[c0] != orbitUnassigned {
						return 0, false, false
					}
					continue
				}
				if colOrbit[c0] >= 0 {
					orbID := colOrbit[c0]
					if colGroup[c0] != 0 {
						return 0, false, false
					}
					for g := 1; g < gCount; g++ {
						cg := rows[tuple[g]].cols[sc.orderBuf[g*rowLen+pos]]
						if colOrbit[cg] != orbID || colGroup[cg] != g {
							return 0, false, false
						}
					}
					continue
				}
				newOrb[0] = c0
				degC0 := colPtr[c0+1] - colPtr[c0]
				for g := 1; g < gCount; g++ {
					cg := rows[tuple[g]].cols[sc.orderBuf[g*rowLen+pos]]
					if colOrbit[cg] != orbitUnassigned {
						return 0, false, false
					}
					if orig.varCat[cg] != orig.varCat[c0] || !samePresolveBound(lb[cg], lb[c0]) || !samePresolveBound(ub[cg], ub[c0]) || colPtr[cg+1]-colPtr[cg] != degC0 {
						return 0, false, false
					}
					for prev := range g {
						if newOrb[prev] == cg {
							return 0, false, false
						}
					}
					newOrb[g] = cg
				}
				orbID := len(sc.orbits) / gCount
				for g := range gCount {
					cg := newOrb[g]
					colOrbit[cg] = orbID
					colGroup[cg] = g
				}
				sc.orbits = append(sc.orbits, newOrb[:gCount]...)
			}
		}
	}

	if !verifyOrbitSymmetry(orig, rows, lb, ub, gCount, sc) {
		return 0, false, false
	}
	return gCount, true, false
}

func isRowInvariantOnOrbit(r *presolveWorkRow, orb []int, targetVal float64) bool {
	matched := 0
	for k, c := range r.cols {
		for _, oc := range orb {
			if c == oc {
				if r.vals[k] != targetVal {
					return false
				}
				matched++
				break
			}
		}
	}
	return matched == len(orb)
}

// verifyOrbitSymmetry confirms that every active row is either mapped identically across rowTuples
// under all group permutations or invariant on every orbit it intersects.
func verifyOrbitSymmetry(
	orig *CompiledProblem,
	rows []presolveWorkRow,
	lb, ub []float64,
	gCount int,
	sc *orbitTrialScratch,
) bool {
	orbits := sc.orbits
	rowTuples := sc.rowTuples
	colOrbit := sc.colOrbit
	colGroup := sc.colGroup
	rowState := sc.rowState
	if gCount == 0 || len(orbits) == 0 || len(orbits)%gCount != 0 || len(rowTuples)%gCount != 0 {
		return false
	}
	numOrbits := len(orbits) / gCount
	numTuples := len(rowTuples) / gCount
	n := orig.numVars

	for orbID := range numOrbits {
		base := orbID * gCount
		c0 := orbits[base]
		for g := range gCount {
			cg := orbits[base+g]
			if colOrbit[cg] != orbID || colGroup[cg] != g {
				return false
			}
			if g > 0 {
				if orig.varCat[cg] != orig.varCat[c0] || !samePresolveBound(lb[cg], lb[c0]) || !samePresolveBound(ub[cg], ub[c0]) {
					return false
				}
			}
		}
	}

	if len(sc.expectedStamp) < n {
		sc.expectedStamp = make([]int, n)
		sc.expectedVal = make([]float64, n)
		sc.stamp = 0
	}
	expectedStamp := sc.expectedStamp
	expectedVal := sc.expectedVal

	for tIdx := range numTuples {
		tBase := tIdx * gCount
		r0Idx := rowTuples[tBase]
		r0 := &rows[r0Idx]
		if !r0.active || rowState[r0Idx] != 0 {
			return false
		}
		for _, c0 := range r0.cols {
			if colOrbit[c0] >= 0 && colGroup[c0] != 0 {
				return false
			}
		}
		for g := 1; g < gCount; g++ {
			rgIdx := rowTuples[tBase+g]
			rg := &rows[rgIdx]
			if !rg.active || rowState[rgIdx] != g || rg.sense != r0.sense || len(rg.cols) != len(r0.cols) || rg.rhs != r0.rhs {
				return false
			}
			if sc.stamp == math.MaxInt {
				clear(expectedStamp)
				sc.stamp = 0
			}
			sc.stamp++
			stamp := sc.stamp
			for k, c0 := range r0.cols {
				cg := c0
				if orb := colOrbit[c0]; orb >= 0 {
					cg = orbits[orb*gCount+g]
				}
				expectedStamp[cg] = stamp
				expectedVal[cg] = r0.vals[k]
			}
			for k, cg := range rg.cols {
				if expectedStamp[cg] != stamp || rg.vals[k] != expectedVal[cg] {
					return false
				}
				expectedStamp[cg] = 0
			}
		}
	}

	if len(sc.orbSeen) < numOrbits {
		sc.orbSeen = make([]int, numOrbits)
		sc.orbCount = make([]int, numOrbits)
		sc.orbVal = make([]float64, numOrbits)
		sc.invStamp = 0
	}
	orbSeen := sc.orbSeen
	orbCount := sc.orbCount
	orbVal := sc.orbVal

	for i := range rows {
		r := &rows[i]
		if !r.active || rowState[i] >= 0 {
			continue
		}
		if sc.invStamp == math.MaxInt {
			clear(orbSeen)
			sc.invStamp = 0
		}
		sc.invStamp++
		invStamp := sc.invStamp
		for k, c := range r.cols {
			orb := colOrbit[c]
			if orb < 0 {
				continue
			}
			if orbSeen[orb] != invStamp {
				orbSeen[orb] = invStamp
				orbCount[orb] = 1
				orbVal[orb] = r.vals[k]
			} else {
				if r.vals[k] != orbVal[orb] {
					return false
				}
				orbCount[orb]++
			}
		}
		for _, c := range r.cols {
			if orb := colOrbit[c]; orb >= 0 && orbCount[orb] != gCount {
				return false
			}
		}
	}
	return true
}

// reduceVerifiedOrbits breaks a verified S_G symmetry either by fixing bijective/orbitope binary entries
// when costs are group-invariant or by appending monotone block-open ordering inequalities.
func reduceVerifiedOrbits(
	orig *CompiledProblem,
	rows []presolveWorkRow,
	lb, ub []float64,
	orbits []int,
	gCount int,
	colOrbit []int,
	rowTuples []int,
) (tightened int, addedRows []presolveWorkRow) {
	numOrbits := len(orbits) / gCount
	numTuples := len(rowTuples) / gCount

	exactObj := true
	monotoneNonNeg := true
	for orbID := range numOrbits {
		base := orbID * gCount
		if lb[orbits[base]] < 0 {
			monotoneNonNeg = false
		}
		c0 := orig.objMin[orbits[base]]
		for g := 1; g < gCount; g++ {
			cg := orig.objMin[orbits[base+g]]
			if cg != c0 {
				exactObj = false
			}
			if orig.objMin[orbits[base+g-1]] > cg {
				monotoneNonNeg = false
			}
		}
	}

	if exactObj {
		isUnitPartOrb := make([]bool, numOrbits)
		isEqualPartOrb := make([]bool, numOrbits)
		var eqPartOrbits, lePartOrbits []int
		for i := range rows {
			r := &rows[i]
			if !r.active || len(r.cols) != gCount || r.rhs != 1.0 {
				continue
			}
			if r.sense != Equal && r.sense != LessEqual {
				continue
			}
			orb := colOrbit[r.cols[0]]
			if orb < 0 || orig.varCat[orbits[orb*gCount]] != Binary || lb[orbits[orb*gCount]] != 0 || ub[orbits[orb*gCount]] != 1 {
				continue
			}
			valid := true
			for k, c := range r.cols {
				if colOrbit[c] != orb || r.vals[k] != 1.0 {
					valid = false
					break
				}
			}
			if valid && !isUnitPartOrb[orb] {
				isUnitPartOrb[orb] = true
				if r.sense == Equal {
					isEqualPartOrb[orb] = true
					eqPartOrbits = append(eqPartOrbits, orb)
				} else {
					lePartOrbits = append(lePartOrbits, orb)
				}
			} else if valid && r.sense == Equal && !isEqualPartOrb[orb] {
				isEqualPartOrb[orb] = true
			}
		}

		// Subcase A1: Bijective designated subset (G x G permutation subarray -> identity matrix)
		for tIdx := range numTuples {
			r0 := &rows[rowTuples[tIdx*gCount]]
			if !r0.active || r0.sense != Equal || len(r0.cols) != gCount || r0.rhs != 1.0 {
				continue
			}
			dOrbs := make([]int, 0, gCount)
			validBij := true
			for k, c := range r0.cols {
				orb := colOrbit[c]
				if orb < 0 || !isUnitPartOrb[orb] || r0.vals[k] != 1.0 {
					validBij = false
					break
				}
				if slices.Contains(dOrbs, orb) {
					validBij = false
					break
				}
				dOrbs = append(dOrbs, orb)
			}
			if validBij && len(dOrbs) == gCount {
				slices.Sort(dOrbs)
				for d, orb := range dOrbs {
					base := orb * gCount
					for g := range gCount {
						col := orbits[base+g]
						if g == d {
							if lb[col] < 1 {
								lb[col] = 1
								tightened++
							}
						} else {
							if ub[col] > 0 {
								ub[col] = 0
								tightened++
							}
						}
					}
				}
				return tightened, nil
			}
		}

		// Subcase A2: General partitioning/packing orbitope upper-triangular zero fixings
		partOrbs := make([]int, 0, len(eqPartOrbits)+len(lePartOrbits))
		partOrbs = append(partOrbs, eqPartOrbits...)
		partOrbs = append(partOrbs, lePartOrbits...)
		if len(partOrbs) > 0 {
			if isEqualPartOrb[partOrbs[0]] {
				c00 := orbits[partOrbs[0]*gCount]
				if lb[c00] < 1 {
					lb[c00] = 1
					tightened++
				}
			}
			limit := min(len(partOrbs), gCount-1)
			for rIdx := range limit {
				base := partOrbs[rIdx] * gCount
				for g := rIdx + 1; g < gCount; g++ {
					col := orbits[base+g]
					if ub[col] > 0 {
						ub[col] = 0
						tightened++
					}
				}
			}
			return tightened, nil
		}
		return 0, nil
	}

	if !monotoneNonNeg {
		return 0, nil
	}

	// Rule B: Monotone cost-dominated empty-block ordering (y_0 >= y_1 >= ... >= y_{G-1})
	forcedZero := make([]bool, numOrbits)
	for openOrb := range numOrbits {
		openBase := openOrb * gCount
		cOpen0 := orbits[openBase]
		if orig.varCat[cOpen0] != Binary || lb[cOpen0] != 0 || ub[cOpen0] != 1 {
			continue
		}
		clear(forcedZero)
		forcedZero[openOrb] = true
		for tIdx := range numTuples {
			r0 := &rows[rowTuples[tIdx*gCount]]
			if !r0.active || (r0.sense != LessEqual && r0.sense != GreaterEqual) {
				continue
			}
			sign := 1.0
			if r0.sense == GreaterEqual {
				sign = -1.0
			}
			if sign*r0.rhs > 0 {
				continue
			}
			hasOpenNeg := false
			allOtherPos := true
			for k, c := range r0.cols {
				a := sign * r0.vals[k]
				if c == cOpen0 {
					if a < 0 {
						hasOpenNeg = true
					} else {
						allOtherPos = false
						break
					}
				} else if a <= 0 || lb[c] < 0 {
					allOtherPos = false
					break
				}
			}
			if hasOpenNeg && allOtherPos {
				for _, c := range r0.cols {
					if orb := colOrbit[c]; orb >= 0 {
						forcedZero[orb] = true
					}
				}
			}
		}

		dominatesAll := true
		for orb := range numOrbits {
			base := orb * gCount
			if orig.objMin[orbits[base+gCount-1]] != orig.objMin[orbits[base]] && !forcedZero[orb] {
				dominatesAll = false
				break
			}
		}
		if dominatesAll {
			usedNames := make(map[string]struct{}, len(rows)+gCount)
			for i := range rows {
				usedNames[rows[i].name] = struct{}{}
			}
			addedRows = make([]presolveWorkRow, 0, gCount-1)
			for g := range gCount - 1 {
				name := "__sym_ord_" + strconv.Itoa(g)
				for seq := 1; ; seq++ {
					if _, exists := usedNames[name]; !exists {
						break
					}
					name = "__sym_ord_" + strconv.Itoa(g) + "_" + strconv.Itoa(seq)
				}
				usedNames[name] = struct{}{}
				addedRows = append(addedRows, presolveWorkRow{
					name:   name,
					cols:   []int{orbits[openBase+g], orbits[openBase+g+1]},
					vals:   []float64{1.0, -1.0},
					rhs:    0,
					sense:  GreaterEqual,
					active: true,
				})
			}
			return 0, addedRows
		}
	}

	return 0, nil
}

func gcdInt64(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func divFloorInt64(a, d int64) int64 {
	q := a / d
	if a%d < 0 {
		q--
	}
	return q
}

func divCeilInt64(a, d int64) int64 {
	q := a / d
	if a%d > 0 {
		q++
	}
	return q
}
