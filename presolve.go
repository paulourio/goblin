package goblin

import (
	"cmp"
	"math"
	"math/big"
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
	rhsRat *big.Rat
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
		if lb[j] > ub[j] {
			return PresolveResult{Infeasible: true}
		}
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
		if lb[j] > ub[j] {
			return PresolveResult{Infeasible: true}
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
				if isNonFinite(cVal) {
					allInt = false
					break
				}
				rc := math.Round(cVal)
				diff := math.Abs(cVal - rc)
				if diff > presolveGCDIntTol || math.Abs(rc) > presolveGCDMaxCoeff {
					allInt = false
					break
				}
				ic := int64(math.Abs(rc))
				if ic == 0 {
					if cVal != 0 {
						allInt = false
						break
					}
					continue
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
					if r.sense == Equal {
						loInt := int64(math.Ceil(r.rhs - presolveFeasTol - maxRoundErr))
						hiInt := int64(math.Floor(r.rhs + presolveFeasTol + maxRoundErr))
						q := divCeilInt64(loInt, g)
						if q*g > hiInt {
							return PresolveResult{Infeasible: true}
						}
						// Do not round a sub-tolerance non-multiple equality RHS
						// (such as 2x - 2y = 5e-7) into a falsely feasible integer multiple.
						if maxRoundErr == 0 && r.rhs != float64(q*g) {
							continue
						}
						r.ensureOwned()
						fg := float64(g)
						for k := range r.vals {
							r.vals[k] = math.Round(r.vals[k]) / fg
						}
						r.rhs = float64(q)
						strengthened++
					} else {
						r.ensureOwned()
						fg := float64(g)
						for k := range r.vals {
							r.vals[k] = math.Round(r.vals[k]) / fg
						}
						if r.sense == LessEqual {
							ubInt := int64(math.Floor(r.rhs + maxRoundErr + presolveIntTol))
							r.rhs = float64(divFloorInt64(ubInt, g))
						} else {
							lbInt := int64(math.Ceil(r.rhs - maxRoundErr - presolveIntTol))
							r.rhs = float64(divCeilInt64(lbInt, g))
						}
						strengthened++
					}
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
					if maxRoundErr > 0 {
						r.rhs = nearest
					}
				}
			}
		}
	}

	// 2. Iterative Savelsbergh activity bound tightening and redundant row dropping
	actTight, actRem, infeas, timedOut := runActivityTighteningDirty(orig, rows, lb, ub, nil, maxPasses, deadline, done)
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
		var prevLB, prevUB []float64
		prevM := len(rows)
		if m > 32 {
			prevLB = slices.Clone(lb)
			prevUB = slices.Clone(ub)
		}
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
			var postDirty []bool
			if prevLB != nil {
				postDirty = make([]bool, len(rows))
				for j := range n {
					if lb[j] != prevLB[j] || ub[j] != prevUB[j] {
						for p := orig.colPtr[j]; p < orig.colPtr[j+1]; p++ {
							postDirty[orig.rowIdx[p]] = true
						}
					}
				}
				for i := prevM; i < len(rows); i++ {
					postDirty[i] = true
				}
			}
			postTight, postRem, postInfeas, postTimedOut := runActivityTighteningDirty(orig, rows, lb, ub, postDirty, maxPasses, deadline, done)
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
			var s rowActivitySummary
			evalRowActivitySummaryInto(&s, r.cols, r.vals, lb, ub, r.rhs, false, false, true)
			if s.extreme || s.maxInfCount > 0 {
				continue
			}
			binTol := max(presolveFeasTol, presolveRelScaleTol*max(1.0, s.l1Max))
			surplusLo := -s.deficitHi
			surplusHi := -s.deficitLo
			if surplusLo > binTol {
				for k, col := range r.cols {
					if orig.varCat[col] != Binary || lb[col] != 0 || ub[col] != 1 {
						continue
					}
					a := r.vals[k]
					if a > binTol && surplusHi < a-binTol {
						newVal := surplusHi
						top, rem, ok := evalBinaryTightenRHSWide(r.rhs, a, newVal)
						if !ok || math.IsNaN(top) || math.IsInf(top, 0) {
							continue
						}
						if rem > 0 {
							top = nextUp(top)
						}
						if newVal > 0 && newVal < a-binTol && top < r.rhs-binTol && !isNonFinite(top) {
							r.ensureOwned()
							r.vals[k] = newVal
							r.rhs = top
							strengthened++
							evalRowActivitySummaryInto(&s, r.cols, r.vals, lb, ub, r.rhs, false, false, true)
							if s.extreme || s.maxInfCount > 0 {
								break
							}
							binTol = max(presolveFeasTol, presolveRelScaleTol*max(1.0, s.l1Max))
							surplusLo = -s.deficitHi
							surplusHi = -s.deficitLo
							if surplusLo <= binTol {
								break
							}
						}
					} else if a < -binTol && surplusHi < -a-binTol {
						newVal := -surplusHi
						if newVal > a+binTol && newVal < 0 {
							r.ensureOwned()
							r.vals[k] = newVal
							strengthened++
							evalRowActivitySummaryInto(&s, r.cols, r.vals, lb, ub, r.rhs, false, false, true)
							if s.extreme || s.maxInfCount > 0 {
								break
							}
							binTol = max(presolveFeasTol, presolveRelScaleTol*max(1.0, s.l1Max))
							surplusLo = -s.deficitHi
							surplusHi = -s.deficitLo
							if surplusLo <= binTol {
								break
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
	red, _, droppedEmpty, infeas = buildCompiledDirectWithRowMap(orig, rows, lb, ub, false)
	return red, droppedEmpty, infeas
}

func buildCompiledDirectWithRowMap(orig *CompiledProblem, rows []presolveWorkRow, lb, ub []float64, trackOrigRows bool) (red *CompiledProblem, origRowIdx []int, droppedEmpty int, infeas bool) {
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
					return nil, nil, 0, true
				}
			case GreaterEqual:
				if 0 < r.rhs-presolveFeasTol {
					return nil, nil, 0, true
				}
			case Equal:
				if math.Abs(r.rhs) > presolveFeasTol {
					return nil, nil, 0, true
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
	if trackOrigRows {
		origRowIdx = make([]int, mc)
	}

	rowOut := 0
	for i := range rows {
		r := &rows[i]
		if !r.active {
			continue
		}
		if trackOrigRows {
			origRowIdx[rowOut] = i
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

	return red, origRowIdx, droppedEmpty, false
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

// isFractionalFloat64 stays within the Go inline budget (cost <= 80) to avoid math.Trunc calls.
func isFractionalFloat64(x float64) bool {
	absBits := math.Float64bits(x) &^ (uint64(1) << 63)
	if absBits == 0 {
		return false
	}
	exp := absBits >> 52
	if exp < 1023 {
		return true
	}
	if exp >= 1075 {
		return exp == 0x7ff && (absBits&0x000fffffffffffff) != 0
	}
	fracMask := uint64(0x000fffffffffffff) >> (exp - 1023)
	return (absBits & fracMask) != 0
}

// evalBinaryTightenRHSWide isolates wideDotAcc stack allocation off the fast-path frame.
//
//go:noinline
func evalBinaryTightenRHSWide(rhs, a, newVal float64) (top, rem float64, ok bool) {
	var w wideDotAcc
	w.addVal(rhs)
	w.addVal(-a)
	w.addVal(newVal)
	return w.finishDD()
}

// fastRowCanSkip performs an O(rowLen) float64 pre-check with conservative ULP and
// feasibility margins to skip exact dot2Acc/wideDotAcc activity evaluation when a row
// can neither prove infeasibility, be dropped as redundant, nor tighten any variable bound.
// Extreme-scale rows (|a| > 1e12, 0 < |a| < 1e-12, |term| > 1e12, or |rhs| > 1e12)
// always fall back to exact activity evaluation.
func fastRowCanSkip(orig *CompiledProblem, r *presolveWorkRow, lb, ub []float64) bool {
	cols := r.cols
	vals := r.vals
	if len(cols) > presolveFastMaxRowLen || len(vals) < len(cols) {
		return false
	}
	n := orig.numVars
	if len(lb) < n || len(ub) < n || len(orig.varCat) < n {
		return false
	}
	vals = vals[:len(cols)]
	lb = lb[:n]
	ub = ub[:n]
	varCat := orig.varCat[:n]
	absRHS := math.Abs(r.rhs)
	if math.IsNaN(absRHS) || absRHS > presolveFastMaxAbs {
		return false
	}
	minAct, maxAct := 0.0, 0.0
	absSum := absRHS
	minInf, maxInf := 0, 0
	minInfCol, maxInfCol := -1, -1
	minInfA, maxInfA := 0.0, 0.0
	maxSpan := 0.0
	hasInfSpan := false
	hasFracInt := false
	for k, col := range cols {
		if col < 0 || col >= n {
			return false
		}
		a := vals[k]
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
		if varCat[col] != Continuous {
			if isFractionalFloat64(l) || isFractionalFloat64(u) {
				hasFracInt = true
			}
		}
		if a > 0 {
			if l != 0 {
				if math.IsInf(l, -1) {
					minInf++
					minInfCol = col
					minInfA = a
				} else {
					p := a * l
					ap := math.Abs(p)
					if ap > presolveFastMaxAbs {
						return false
					}
					minAct += p
					absSum += ap
				}
			}
			if u != 0 {
				if math.IsInf(u, 1) {
					maxInf++
					maxInfCol = col
					maxInfA = a
				} else {
					p := a * u
					ap := math.Abs(p)
					if ap > presolveFastMaxAbs {
						return false
					}
					maxAct += p
					absSum += ap
				}
			}
		} else {
			if u != 0 {
				if math.IsInf(u, 1) {
					minInf++
					minInfCol = col
					minInfA = a
				} else {
					p := a * u
					ap := math.Abs(p)
					if ap > presolveFastMaxAbs {
						return false
					}
					minAct += p
					absSum += ap
				}
			}
			if l != 0 {
				if math.IsInf(l, -1) {
					maxInf++
					maxInfCol = col
					maxInfA = a
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
		if hasFracInt {
			return false
		}
		if minInf == 1 {
			if minInfCol < 0 || minInfCol >= n {
				return false
			}
			if minInfA > 0 {
				uInf := ub[minInfCol]
				if math.IsInf(uInf, 1) || slackFast-margin < minInfA*uInf {
					return false
				}
			} else {
				lInf := lb[minInfCol]
				if math.IsInf(lInf, -1) || slackFast-margin < minInfA*lInf {
					return false
				}
			}
		} else if hasInfSpan || slackFast <= maxSpan+margin {
			return false
		}
	}
	if hasLower && maxInf <= 1 {
		if hasFracInt {
			return false
		}
		if maxInf == 1 {
			if maxInfCol < 0 || maxInfCol >= n {
				return false
			}
			if maxInfA > 0 {
				lInf := lb[maxInfCol]
				if math.IsInf(lInf, -1) || deficitFast+margin > maxInfA*lInf {
					return false
				}
			} else {
				uInf := ub[maxInfCol]
				if math.IsInf(uInf, 1) || deficitFast+margin > maxInfA*uInf {
					return false
				}
			}
		} else if hasInfSpan || -deficitFast <= maxSpan+margin {
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
	if isInt && isFractionalFloat64(targetBound) {
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
	return runActivityTighteningDirty(orig, rows, lb, ub, nil, maxPasses, deadline, done)
}

// runActivityTighteningDirty executes Savelsbergh row-activity bound tightening using an optional initial dirty-row mask.
func runActivityTighteningDirty(
	orig *CompiledProblem,
	rows []presolveWorkRow,
	lb, ub []float64,
	initDirty []bool,
	maxPasses int,
	deadline time.Time,
	done <-chan struct{},
) (tightened, removedRows int, infeas, timedOut bool) {
	m := len(rows)
	baseM := orig.numConstraints
	hasCSC := len(orig.colPtr) == orig.numVars+1 && m >= baseM
	var curDirty, nextDirty []bool
	if initDirty != nil {
		curDirty = initDirty
	}
	if hasCSC && maxPasses > 1 {
		nextDirty = make([]bool, m)
	}
	markColDirty := func(col, curRow int) {
		if !hasCSC || nextDirty == nil {
			return
		}
		for p := orig.colPtr[col]; p < orig.colPtr[col+1]; p++ {
			rr := orig.rowIdx[p]
			if curDirty != nil && rr > curRow {
				curDirty[rr] = true
			} else {
				nextDirty[rr] = true
			}
		}
		for rr := baseM; rr < m; rr++ {
			if curDirty != nil && rr > curRow {
				curDirty[rr] = true
			} else {
				nextDirty[rr] = true
			}
		}
	}
	for range maxPasses {
		if stopRequested(deadline, done) {
			return tightened, removedRows, false, true
		}
		changedInPass := false

		for i := range m {
			if (i&63 == 0) && stopRequested(deadline, done) {
				return tightened, removedRows, false, true
			}
			if curDirty != nil && !curDirty[i] {
				continue
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
			var s rowActivitySummary
			evalRowActivitySummaryInto(&s, r.cols, r.vals, lb, ub, r.rhs, false, true, true)
			if s.extreme {
				extInfeas, extRemove := handleExtremeRowTightening(r, lb, ub, s.minInfCount, s.maxInfCount, hasUpper, hasLower)
				if extInfeas {
					return tightened, removedRows, true, false
				}
				if extRemove {
					r.active = false
					removedRows++
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

			if r.sense == LessEqual && s.maxInfCount == 0 && s.deficitLo >= 0 {
				r.active = false
				removedRows++
				continue
			}
			if r.sense == GreaterEqual && s.minInfCount == 0 && s.slackHi <= 0 {
				r.active = false
				removedRows++
				continue
			}
			if r.sense == Equal && s.minInfCount == 0 && s.maxInfCount == 0 && s.slackHi <= 0 && s.deficitLo >= 0 {
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
								markColDirty(col, i)
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
								markColDirty(col, i)
							}
						}
					}
				}
			}

			if hasLower && upperTightenedRow {
				evalRowActivitySummaryInto(&s, r.cols, r.vals, lb, ub, r.rhs, false, false, true)
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
								markColDirty(col, i)
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
								markColDirty(col, i)
							}
						}
					}
				}
			}
		}

		if !changedInPass {
			break
		}
		if nextDirty != nil {
			if curDirty == nil {
				curDirty = nextDirty
				nextDirty = make([]bool, m)
			} else {
				curDirty, nextDirty = nextDirty, curDirty
				clear(nextDirty)
			}
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
	domMaxA              float64
	rowMaxA              float64
	minInfCount          int
	maxInfCount          int
	useWideSlack         bool
	useWideDeficit       bool
	extreme              bool
}

func (s *rowActivitySummary) domainMaxAbs() float64 {
	if s.domMaxA > 0 {
		return s.domMaxA
	}
	if s.rowMaxA > 0 {
		return s.rowMaxA
	}
	return 1.0
}

//go:noinline
func handleExtremeRowTightening(
	r *presolveWorkRow,
	lb, ub []float64,
	minInfCount, maxInfCount int,
	hasUpper, hasLower bool,
) (infeas, remove bool) {
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
	underflowed := r.rhs != 0 && rhsS == 0
	slackW.addVal(rhsS)
	defW.addVal(rhsS)
	l1Scaled := math.Abs(rhsS)
	for k, col := range r.cols {
		a := r.vals[k]
		l, u := lb[col], ub[col]
		as := math.Ldexp(a, -s1)
		if a != 0 && as == 0 {
			underflowed = true
		}
		if a > 0 {
			if !math.IsInf(l, -1) {
				ls := math.Ldexp(l, -s2)
				if (l != 0 && ls == 0) || (as != 0 && l != 0 && as*ls == 0) {
					underflowed = true
				}
				slackW.addProd(-as, ls)
				l1Scaled += math.Abs(as * ls)
			}
			if !math.IsInf(u, 1) {
				us := math.Ldexp(u, -s2)
				if (u != 0 && us == 0) || (as != 0 && u != 0 && as*us == 0) {
					underflowed = true
				}
				defW.addProd(-as, us)
				l1Scaled += math.Abs(as * us)
			}
		} else if a < 0 {
			if !math.IsInf(u, 1) {
				us := math.Ldexp(u, -s2)
				if (u != 0 && us == 0) || (as != 0 && u != 0 && as*us == 0) {
					underflowed = true
				}
				slackW.addProd(-as, us)
				l1Scaled += math.Abs(as * us)
			}
			if !math.IsInf(l, -1) {
				ls := math.Ldexp(l, -s2)
				if (l != 0 && ls == 0) || (as != 0 && l != 0 && as*ls == 0) {
					underflowed = true
				}
				defW.addProd(-as, ls)
				l1Scaled += math.Abs(as * ls)
			}
		}
	}
	if underflowed {
		return false, false
	}
	slackS := slackW.finish(math.NaN())
	defS := defW.finish(math.NaN())
	if math.IsNaN(slackS) || math.IsInf(slackS, 0) || math.IsNaN(defS) || math.IsInf(defS, 0) || math.IsNaN(l1Scaled) || math.IsInf(l1Scaled, 0) {
		return false, false
	}
	errS := 0x1p-45 * (l1Scaled + math.Abs(slackS) + math.Abs(defS))
	tolS := max(math.Ldexp(presolveFeasTol, -shift), presolveRelScaleTol*l1Scaled) + errS
	if hasUpper && minInfCount == 0 && slackS+errS < -tolS {
		return true, false
	}
	if hasLower && maxInfCount == 0 && defS-errS > tolS {
		return true, false
	}
	if r.sense == LessEqual && maxInfCount == 0 && defS-errS >= 0 {
		return false, true
	}
	if r.sense == GreaterEqual && minInfCount == 0 && slackS+errS <= 0 {
		return false, true
	}
	if r.sense == Equal && minInfCount == 0 && maxInfCount == 0 && slackS+errS <= 0 && defS-errS >= 0 {
		return false, true
	}
	return false, false
}

func evalRowActivitySummary(
	cols []int,
	vals, lb, ub []float64,
	rhs float64,
	bailBothInf, hasUpper, hasLower bool,
) (s rowActivitySummary, earlyBail bool) {
	earlyBail = evalRowActivitySummaryInto(&s, cols, vals, lb, ub, rhs, bailBothInf, hasUpper, hasLower)
	return s, earlyBail
}

func evalRowActivitySummaryInto(
	s *rowActivitySummary,
	cols []int,
	vals, lb, ub []float64,
	rhs float64,
	bailBothInf, hasUpper, hasLower bool,
) (earlyBail bool) {
	*s = rowActivitySummary{}
	if math.IsNaN(rhs) || math.IsInf(rhs, 0) || len(vals) < len(cols) {
		s.extreme = true
		return false
	}
	vals = vals[:len(cols)]
	if hasUpper {
		s.slackAcc.addVal(rhs)
	}
	if hasLower {
		s.deficitAcc.addVal(rhs)
	}
	for k, col := range cols {
		a := vals[k]
		if a == 0 {
			continue
		}
		if isNonFinite(a) {
			s.extreme = true
			return false
		}
		av := math.Abs(a)
		if av > s.rowMaxA {
			s.rowMaxA = av
		}
		l, u := lb[col], ub[col]
		if (l != 0 || u != 0) && av > s.domMaxA {
			s.domMaxA = av
		}
		if a > 0 {
			if hasUpper && l != 0 {
				if math.IsInf(l, -1) {
					s.minInfCount++
				} else {
					s.slackAcc.addProd(-a, l)
					if l < u {
						s.l1Min += math.Abs(a * l)
					}
				}
			}
			if hasLower && u != 0 {
				if math.IsInf(u, 1) {
					s.maxInfCount++
				} else {
					s.deficitAcc.addProd(-a, u)
					if l < u {
						s.l1Max += math.Abs(a * u)
					}
				}
			}
		} else {
			if hasUpper && u != 0 {
				if math.IsInf(u, 1) {
					s.minInfCount++
				} else {
					s.slackAcc.addProd(-a, u)
					if l < u {
						s.l1Min += math.Abs(a * u)
					}
				}
			}
			if hasLower && l != 0 {
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
			return true
		}
	}

	if math.IsNaN(s.l1Min) || math.IsInf(s.l1Min, 0) || math.IsNaN(s.l1Max) || math.IsInf(s.l1Max, 0) {
		s.extreme = true
		return false
	}

	if hasUpper {
		if val, eb, ok := s.slackAcc.result(); ok && eb <= 0x1p-20*max(1.0, math.Abs(val)) {
			lo := val - eb
			hi := val + eb
			if eb > 0 || s.slackAcc.errL1 > 0 || s.slackAcc.lowCount > 0 {
				lo = nextDown(lo)
				hi = nextUp(hi)
			}
			s.slackLo = lo
			s.slackHi = hi
		} else {
			s.useWideSlack = true
			lo, hi, ok := evalRowActivityWide(cols, vals, lb, ub, rhs, true)
			if !ok {
				s.extreme = true
				return false
			}
			s.slackLo = lo
			s.slackHi = hi
		}
	}

	if hasLower {
		if val, eb, ok := s.deficitAcc.result(); ok && eb <= 0x1p-20*max(1.0, math.Abs(val)) {
			lo := val - eb
			hi := val + eb
			if eb > 0 || s.deficitAcc.errL1 > 0 || s.deficitAcc.lowCount > 0 {
				lo = nextDown(lo)
				hi = nextUp(hi)
			}
			s.deficitLo = lo
			s.deficitHi = hi
		} else {
			s.useWideDeficit = true
			lo, hi, ok := evalRowActivityWide(cols, vals, lb, ub, rhs, false)
			if !ok {
				s.extreme = true
				return false
			}
			s.deficitLo = lo
			s.deficitHi = hi
		}
	}

	return false
}

// evalRowActivityWide isolates wideDotAcc stack allocation off evalRowActivitySummaryInto
// and returns a sound [lo, hi] enclosure including subnormal and underflow margins.
//
//go:noinline
func evalRowActivityWide(cols []int, vals, lb, ub []float64, rhs float64, upper bool) (lo, hi float64, ok bool) {
	if len(vals) < len(cols) {
		return 0, 0, false
	}
	vals = vals[:len(cols)]
	var w wideDotAcc
	w.addVal(rhs)
	if upper {
		for k, col := range cols {
			a := vals[k]
			if a > 0 && !math.IsInf(lb[col], -1) {
				w.addProd(-a, lb[col])
			} else if a < 0 && !math.IsInf(ub[col], 1) {
				w.addProd(-a, ub[col])
			}
		}
	} else {
		for k, col := range cols {
			a := vals[k]
			if a > 0 && !math.IsInf(ub[col], 1) {
				w.addProd(-a, ub[col])
			} else if a < 0 && !math.IsInf(lb[col], -1) {
				w.addProd(-a, lb[col])
			}
		}
	}
	top, rem, wOK := w.finishDD()
	if !wOK {
		return 0, 0, false
	}
	val := top + rem
	if math.IsNaN(val) {
		val = top
	}
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return 0, 0, false
	}
	if w.isEmpty() {
		return 0, 0, true
	}
	if val == 0 {
		if w.loLen == 0 {
			return 0, 0, true
		}
		return -math.SmallestNonzeroFloat64, math.SmallestNonzeroFloat64, true
	}
	ulp := 0x1p-50 * math.Abs(val)
	lo = val - ulp
	if lo >= val {
		lo = nextDown(val)
	}
	if rem < 0 || w.loLen > 0 {
		lo = nextDown(lo)
	}
	hi = val + ulp
	if hi <= val {
		hi = nextUp(val)
	}
	if rem > 0 || w.loLen > 0 {
		hi = nextUp(hi)
	}
	return lo, hi, true
}

//go:noinline
func residualQuotientWide(
	cols []int,
	vals, lb, ub []float64,
	rhs float64,
	skipIdx int,
	a float64,
	isUpper bool,
) (qVal, qEB float64, ok bool) {
	if len(vals) < len(cols) {
		return 0, 0, false
	}
	vals = vals[:len(cols)]
	var w wideDotAcc
	w.addVal(rhs)
	if isUpper {
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
	} else {
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
	}
	top, rem, wOK := w.finishDD()
	if !wOK {
		return 0, 0, false
	}
	return divDDByFloat(top, rem, a, 0)
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
		return residualQuotientWide(cols, vals, lb, ub, rhs, skipIdx, a, true)
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
	return residualQuotientWide(cols, vals, lb, ub, rhs, skipIdx, a, false)
}

type orbitSortEntry struct {
	idx    int
	col    int
	orbit  int // >= 0 when colGroup[col] == grp, else -1
	deg    int
	varCat VarCategory
	val    float64
	lb     float64
	ub     float64
}

type orbitTrialScratch struct {
	colOrbit      []int
	colGroup      []int
	rowState      []int
	touchedRows   []int
	orderBuf      []int
	sortKeys      []orbitSortEntry
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
		colPtr, colRow, colVal = buildPresolveCSC(n, rows)
	}

	seedRows = filterAndSortOrbitSeeds(rows, colPtr, seedRows)
	if len(seedRows) == 0 {
		return 0, nil, false
	}

	rowHash := computeOrbitRowHashes(orig, rows, lb, ub, colPtr)

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

func buildPresolveCSC(n int, rows []presolveWorkRow) (colPtr, colRow []int, colVal []float64) {
	colPtr = make([]int, n+1)
	for i := range rows {
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
	for i := range rows {
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
	return colPtr, colRow, colVal
}

func filterAndSortOrbitSeeds(rows []presolveWorkRow, colPtr, seedRows []int) []int {
	filtered := seedRows[:0]
	for _, sr := range seedRows {
		cols := rows[sr].cols
		if len(cols) < 2 {
			continue
		}
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
			filtered = append(filtered, sr)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	slices.SortStableFunc(filtered, func(a, b int) int {
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
	return filtered
}

func sameWorkRowRHS(a, b *presolveWorkRow) bool {
	if a.rhsRat == nil && b.rhsRat == nil {
		return a.rhs == b.rhs
	}
	if a.rhsRat == nil || b.rhsRat == nil {
		return false
	}
	return a.rhsRat.Cmp(b.rhsRat) == 0
}

func hashBigRat(r *big.Rat) uint64 {
	h := presolveSplitmix64(uint64(r.Sign()+2) ^ 0x9e3779b97f4a7c15)
	for _, w := range r.Num().Bits() {
		h = presolveSplitmix64(h ^ uint64(w))
	}
	h = presolveSplitmix64(h ^ 0xbf58476d1ce4e5b9)
	for _, w := range r.Denom().Bits() {
		h = presolveSplitmix64(h ^ uint64(w))
	}
	return h
}

func evalFixedRowResidualRat(cols []int, vals, x []float64, isFree []bool, rhs float64) (*big.Rat, bool) {
	sum := new(big.Rat).SetFloat64(rhs)
	if sum == nil {
		return nil, false
	}
	var aRat, xRat, prod big.Rat
	for k, col := range cols {
		if isFree != nil && isFree[col] {
			continue
		}
		v := vals[k]
		xv := x[col]
		if v == 0 || xv == 0 {
			continue
		}
		if aRat.SetFloat64(v) == nil || xRat.SetFloat64(xv) == nil {
			return nil, false
		}
		prod.Mul(&aRat, &xRat)
		sum.Sub(sum, &prod)
	}
	return sum, true
}

func computeOrbitRowHashes(orig *CompiledProblem, rows []presolveWorkRow, lb, ub []float64, colPtr []int) []uint64 {
	rowHash := make([]uint64, len(rows))
	for i := range rows {
		r := &rows[i]
		if !r.active {
			continue
		}
		rhsHash := canonicalFloatBits(r.rhs) << 3
		if r.rhsRat != nil {
			rhsHash = hashBigRat(r.rhsRat)
		}
		h := presolveSplitmix64(uint64(r.sense) ^ rhsHash ^ uint64(len(r.cols))*0x517cc1b727220a95)
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
	return rowHash
}

type nodeSymmetry struct {
	colTwinClass []int
	twinClasses  [][]int

	symG        int
	symOrbits   []int
	symColOrbit []int
	symColGroup []int

	dynScratch *orbitTrialScratch
}

func initNodeSymmetry(p *CompiledProblem) nodeSymmetry {
	if p == nil || len(p.intCols) < 2 {
		return nodeSymmetry{}
	}
	var ns nodeSymmetry
	ns.initBinaryColumnTwins(p)
	ns.initStaticSymGroup(p)
	return ns
}

// cloneForWorker returns a worker-local copy of s whose immutable symmetry
// tables are shared while lazy dynamic-orbit scratch is cleared.
func (s nodeSymmetry) cloneForWorker() nodeSymmetry {
	s.dynScratch = nil
	return s
}

func (s *nodeSymmetry) initBinaryColumnTwins(p *CompiledProblem) {
	n := p.numVars
	buckets := make(map[uint64][]int, len(p.binaryMask))
	for j := range n {
		if p.varCat[j] != Binary || p.varLB[j] == p.varUB[j] {
			continue
		}
		cs, ce := p.colPtr[j], p.colPtr[j+1]
		h := presolveSplitmix64(canonicalFloatBits(p.objMin[j]) ^ uint64(ce-cs)*0x517cc1b727220a95)
		for k := cs; k < ce; k++ {
			h = presolveSplitmix64(h ^ uint64(p.rowIdx[k]))
			h = presolveSplitmix64(h ^ canonicalFloatBits(p.colVal[k]))
		}
		buckets[h] = append(buckets[h], j)
	}
	sameCol := func(a, b int) bool {
		if p.objMin[a] != p.objMin[b] {
			return false
		}
		as, ae := p.colPtr[a], p.colPtr[a+1]
		bs, be := p.colPtr[b], p.colPtr[b+1]
		if ae-as != be-bs {
			return false
		}
		for k := range ae - as {
			if p.rowIdx[as+k] != p.rowIdx[bs+k] || p.colVal[as+k] != p.colVal[bs+k] {
				return false
			}
		}
		return true
	}
	for _, group := range buckets {
		if len(group) < 2 {
			continue
		}
		assigned := make([]bool, len(group))
		for idx, rep := range group {
			if assigned[idx] {
				continue
			}
			var cls []int
			cls = append(cls, rep)
			assigned[idx] = true
			for other := idx + 1; other < len(group); other++ {
				if !assigned[other] && sameCol(rep, group[other]) {
					assigned[other] = true
					cls = append(cls, group[other])
				}
			}
			if len(cls) < 2 {
				continue
			}
			if s.colTwinClass == nil {
				s.colTwinClass = make([]int, n)
				for j := range n {
					s.colTwinClass[j] = -1
				}
			}
			clsID := len(s.twinClasses)
			slices.Sort(cls)
			for _, col := range cls {
				s.colTwinClass[col] = clsID
			}
			s.twinClasses = append(s.twinClasses, cls)
		}
	}
}

func (s *nodeSymmetry) initStaticSymGroup(p *CompiledProblem) {
	n := p.numVars
	m := p.numConstraints
	if n < 4 || m < 2 || len(p.colPtr) != n+1 {
		return
	}
	rows := make([]presolveWorkRow, m)
	var seedRows []int
	for i := range m {
		rs, re := p.rowPtr[i], p.rowPtr[i+1]
		r := presolveWorkRow{
			cols:   p.colIdx[rs:re:re],
			vals:   p.rowVal[rs:re:re],
			rhs:    p.rowRHS[i],
			sense:  p.rowSense[i],
			active: true,
			shared: true,
		}
		rows[i] = r
		g := len(r.cols)
		if g < 2 || g > maxOrbitGroupSize || g*2 > n || (r.sense != Equal && r.sense != LessEqual) || r.rhs != 1.0 {
			continue
		}
		unitBin := true
		for k, col := range r.cols {
			if r.vals[k] != 1.0 || p.varCat[col] != Binary || math.IsInf(p.varLB[col], -1) || math.IsInf(p.varUB[col], 1) {
				unitBin = false
				break
			}
		}
		if unitBin {
			seedRows = append(seedRows, i)
		}
	}
	seedRows = filterAndSortOrbitSeeds(rows, p.colPtr, seedRows)
	if len(seedRows) == 0 {
		return
	}

	dummyZero := make([]float64, n)
	rowHash := computeOrbitRowHashes(p, rows, dummyZero, dummyZero, p.colPtr)
	sc := newOrbitTrialScratch(n, m)
	maxTrials := min(len(seedRows), 16)
	for sIdx := range maxTrials {
		if sIdx > 0 {
			sc.reset()
		}
		gCount, ok, _ := proposeAndVerifyOrbits(
			p, rows, dummyZero, dummyZero, p.colPtr, p.rowIdx, p.colVal, rowHash, seedRows[sIdx], &sc, time.Time{}, nil,
		)
		if !ok || len(sc.orbits) == 0 || !hasStaticSymPair(p, sc.orbits, gCount) {
			continue
		}
		s.symG = gCount
		s.symOrbits = slices.Clone(sc.orbits)
		s.symColOrbit = slices.Clone(sc.colOrbit)
		s.symColGroup = slices.Clone(sc.colGroup)
		return
	}
}

func hasStaticSymPair(p *CompiledProblem, orbits []int, gCount int) bool {
	if gCount < 2 || len(orbits) == 0 {
		return false
	}
	numOrbs := len(orbits) / gCount
	for g0 := range gCount - 1 {
		for g1 := g0 + 1; g1 < gCount; g1++ {
			ok := true
			for o := range numOrbs {
				a := orbits[o*gCount+g0]
				b := orbits[o*gCount+g1]
				if p.objMin[a] != p.objMin[b] || p.varLB[a] != p.varLB[b] || p.varUB[a] != p.varUB[b] {
					ok = false
					break
				}
			}
			if ok {
				return true
			}
		}
	}
	return false
}

// activeOrbit returns a verified slice of symmetric Binary [0, 1] variables (including bVar)
// interchangeable with bVar under the current node bounds [curLB, curUB] and objective.
func (s *nodeSymmetry) activeOrbit(
	p *CompiledProblem,
	curLB, curUB []float64,
	bVar, depth int,
	deadline time.Time,
	done <-chan struct{},
	out []int,
) []int {
	out = out[:0]
	if bVar < 0 || bVar >= p.numVars || p.varCat[bVar] != Binary || curLB[bVar] != 0 || curUB[bVar] != 1 {
		return out
	}

	if bVar < len(s.colTwinClass) {
		if tc := s.colTwinClass[bVar]; tc >= 0 {
			for _, c := range s.twinClasses[tc] {
				if curLB[c] == 0 && curUB[c] == 1 {
					out = append(out, c)
				}
			}
			if len(out) >= 2 {
				return out
			}
			out = out[:0]
		}
	}

	if s.symG >= 2 && bVar < len(s.symColOrbit) {
		if orbID := s.symColOrbit[bVar]; orbID >= 0 {
			g0 := s.symColGroup[bVar]
			numOrbs := len(s.symOrbits) / s.symG
			for g := range s.symG {
				cg := s.symOrbits[orbID*s.symG+g]
				if curLB[cg] != 0 || curUB[cg] != 1 {
					continue
				}
				if g != g0 {
					okSym := true
					for o := range numOrbs {
						a := s.symOrbits[o*s.symG+g0]
						bCol := s.symOrbits[o*s.symG+g]
						if p.objMin[a] != p.objMin[bCol] || curLB[a] != curLB[bCol] || curUB[a] != curUB[bCol] {
							okSym = false
							break
						}
					}
					if !okSym {
						continue
					}
				}
				out = append(out, cg)
			}
			if len(out) >= 2 {
				return out
			}
			out = out[:0]
		}
	}

	if depth >= 1 && depth <= 6 && p.numVars <= 512 && p.numConstraints <= 512 {
		return detectDynamicNodeOrbit(p, curLB, curUB, bVar, deadline, done, &s.dynScratch, out)
	}
	return out
}

func detectDynamicNodeOrbit(
	p *CompiledProblem,
	curLB, curUB []float64,
	bVar int,
	deadline time.Time,
	done <-chan struct{},
	scratchPtr **orbitTrialScratch,
	out []int,
) []int {
	if stopRequested(deadline, done) {
		return out
	}
	n := p.numVars
	m := p.numConstraints

	hasCandidateSeed := false
	for ck := p.colPtr[bVar]; ck < p.colPtr[bVar+1]; ck++ {
		r := p.rowIdx[ck]
		if p.rowSense[r] != Equal && p.rowSense[r] != LessEqual {
			continue
		}
		var rhsAcc dot2Acc
		rhsAcc.addVal(p.rowRHS[r])
		freeCnt := 0
		validSeed := true
		for k := p.rowPtr[r]; k < p.rowPtr[r+1]; k++ {
			col := p.colIdx[k]
			val := p.rowVal[k]
			if curLB[col] == curUB[col] {
				rhsAcc.addProd(-val, curLB[col])
				continue
			}
			if val != 1.0 || p.varCat[col] != Binary || curLB[col] != 0 || curUB[col] != 1 {
				validSeed = false
				break
			}
			freeCnt++
			if freeCnt > maxOrbitGroupSize {
				validSeed = false
				break
			}
		}
		if validSeed && freeCnt >= 2 {
			if !rhsAcc.nonFin && rhsAcc.lowCount == 0 && rhsAcc.errL1 == 0 && rhsAcc.c == 0 {
				if rhsAcc.s == 1.0 {
					hasCandidateSeed = true
					break
				}
			} else if effRHS, _, ok := rhsAcc.result(); !ok || math.Abs(effRHS-1.0) <= 1e-6 {
				var sum big.Rat
				if sum.SetFloat64(p.rowRHS[r]) != nil {
					var aRat, xRat, prod big.Rat
					allFin := true
					for k := p.rowPtr[r]; k < p.rowPtr[r+1]; k++ {
						col := p.colIdx[k]
						if curLB[col] != curUB[col] {
							continue
						}
						v, xv := p.rowVal[k], curLB[col]
						if v == 0 || xv == 0 {
							continue
						}
						if aRat.SetFloat64(v) == nil || xRat.SetFloat64(xv) == nil {
							allFin = false
							break
						}
						prod.Mul(&aRat, &xRat)
						sum.Sub(&sum, &prod)
					}
					if allFin && sum.IsInt() && sum.Num().IsInt64() && sum.Num().Int64() == 1 {
						hasCandidateSeed = true
						break
					}
				}
			}
		}
	}
	if !hasCandidateSeed {
		return out
	}

	isFree := make([]bool, n)
	for j := range n {
		if curLB[j] < curUB[j] {
			isFree[j] = true
		}
	}

	resRows := make([]presolveWorkRow, 0, m)
	var seedRows []int
	for i := range m {
		rs, re := p.rowPtr[i], p.rowPtr[i+1]
		freeCnt := 0
		for k := rs; k < re; k++ {
			if isFree[p.colIdx[k]] {
				freeCnt++
			}
		}
		if freeCnt == 0 {
			continue
		}
		remRHS, exact, ok := evalFixedRowResidualExact(p.colIdx[rs:re], p.rowVal[rs:re], curLB, isFree, p.rowRHS[i])
		var rhsRat *big.Rat
		if !exact {
			rat, ratOK := evalFixedRowResidualRat(p.colIdx[rs:re], p.rowVal[rs:re], curLB, isFree, p.rowRHS[i])
			if !ratOK {
				return out
			}
			if f, fExact := rat.Float64(); fExact && !math.IsInf(f, 0) && !math.IsNaN(f) {
				remRHS = f
			} else {
				remRHS, _ = rat.Float64()
				rhsRat = rat
			}
		} else if !ok {
			return out
		}
		cols := make([]int, 0, freeCnt)
		vals := make([]float64, 0, freeCnt)
		touchesBVar := false
		for k := rs; k < re; k++ {
			col := p.colIdx[k]
			if isFree[col] {
				cols = append(cols, col)
				vals = append(vals, p.rowVal[k])
				if col == bVar {
					touchesBVar = true
				}
			}
		}
		rowIdxOut := len(resRows)
		resRows = append(resRows, presolveWorkRow{
			cols:   cols,
			vals:   vals,
			rhs:    remRHS,
			rhsRat: rhsRat,
			sense:  p.rowSense[i],
			active: true,
		})
		if touchesBVar && freeCnt >= 2 && freeCnt <= maxOrbitGroupSize && (p.rowSense[i] == Equal || p.rowSense[i] == LessEqual) && rhsRat == nil && remRHS == 1.0 {
			unitBin := true
			for k, col := range cols {
				if vals[k] != 1.0 || p.varCat[col] != Binary || curLB[col] != 0 || curUB[col] != 1 {
					unitBin = false
					break
				}
			}
			if unitBin {
				seedRows = append(seedRows, rowIdxOut)
			}
		}
	}
	if len(seedRows) == 0 || len(resRows) < 2 {
		return out
	}

	colPtr, colRow, colVal := buildPresolveCSC(n, resRows)
	seedRows = filterAndSortOrbitSeeds(resRows, colPtr, seedRows)
	if len(seedRows) == 0 {
		return out
	}
	rowHash := computeOrbitRowHashes(p, resRows, curLB, curUB, colPtr)

	sc := *scratchPtr
	if sc == nil || len(sc.colOrbit) < n || len(sc.rowState) < len(resRows) {
		tmp := newOrbitTrialScratch(n, len(resRows))
		sc = &tmp
		*scratchPtr = sc
	} else {
		sc.reset()
	}

	maxTrials := min(len(seedRows), 4)
	for sIdx := range maxTrials {
		if stopRequested(deadline, done) {
			return out
		}
		if sIdx > 0 {
			sc.reset()
		}
		gCount, ok, to := proposeAndVerifyOrbits(
			p, resRows, curLB, curUB, colPtr, colRow, colVal, rowHash, seedRows[sIdx], sc, deadline, done,
		)
		if to {
			return out
		}
		if !ok || len(sc.orbits) == 0 || sc.colOrbit[bVar] < 0 {
			continue
		}
		orbID := sc.colOrbit[bVar]
		g0 := sc.colGroup[bVar]
		numOrbs := len(sc.orbits) / gCount
		out = out[:0]
		for g := range gCount {
			cg := sc.orbits[orbID*gCount+g]
			if g != g0 {
				sameObj := true
				for o := range numOrbs {
					if p.objMin[sc.orbits[o*gCount+g0]] != p.objMin[sc.orbits[o*gCount+g]] {
						sameObj = false
						break
					}
				}
				if !sameObj {
					continue
				}
			}
			out = append(out, cg)
		}
		if len(out) >= 2 {
			return out
		}
		out = out[:0]
	}
	return out
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
					if rrg.sense != rr0.sense || len(rrg.cols) != len(rr0.cols) || !sameWorkRowRHS(rrg, rr0) {
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
			unassignedCnt := 0
			unassignedIdx := -1
			for k, c0 := range rr0.cols {
				if colOrbit[c0] < 0 {
					unassignedCnt++
					unassignedIdx = k
					if unassignedCnt > 1 {
						break
					}
				}
			}
			if unassignedCnt <= 1 {
				n := orig.numVars
				if len(sc.expectedStamp) < n {
					sc.expectedStamp = make([]int, n)
					sc.expectedVal = make([]float64, n)
					sc.stamp = 0
				}
				expectedStamp := sc.expectedStamp
				expectedVal := sc.expectedVal
				orbits := sc.orbits
				fastMatched := true
				var u0Col int
				var u0Val float64
				if unassignedCnt == 1 {
					u0Col = rr0.cols[unassignedIdx]
					u0Val = rr0.vals[unassignedIdx]
					newOrb[0] = u0Col
				}
				allSameUnassigned := true
				for g := 1; g < gCount; g++ {
					rg := &rows[tuple[g]]
					if sc.stamp == math.MaxInt {
						clear(expectedStamp)
						sc.stamp = 0
					}
					sc.stamp++
					stamp := sc.stamp
					hasDup := false
					for k, c0 := range rr0.cols {
						if k == unassignedIdx {
							continue
						}
						cg := orbits[colOrbit[c0]*gCount+g]
						if expectedStamp[cg] == stamp {
							hasDup = true
							break
						}
						expectedStamp[cg] = stamp
						expectedVal[cg] = rr0.vals[k]
					}
					if hasDup {
						fastMatched = false
						break
					}
					unmatchedCol := -1
					unmatchedVal := 0.0
					unmatchedCount := 0
					for k, cg := range rg.cols {
						if expectedStamp[cg] == stamp {
							if rg.vals[k] != expectedVal[cg] {
								return 0, false, false
							}
							expectedStamp[cg] = 0
						} else {
							unmatchedCount++
							if unmatchedCount > unassignedCnt {
								return 0, false, false
							}
							unmatchedCol = cg
							unmatchedVal = rg.vals[k]
						}
					}
					if unmatchedCount != unassignedCnt {
						return 0, false, false
					}
					if unassignedCnt == 1 {
						if unmatchedVal != u0Val {
							return 0, false, false
						}
						if unmatchedCol != u0Col {
							allSameUnassigned = false
						}
						newOrb[g] = unmatchedCol
					}
				}
				if fastMatched {
					if unassignedCnt == 1 && !allSameUnassigned {
						degC0 := colPtr[u0Col+1] - colPtr[u0Col]
						for g := 1; g < gCount; g++ {
							cg := newOrb[g]
							if colOrbit[cg] != orbitUnassigned {
								return 0, false, false
							}
							if orig.varCat[cg] != orig.varCat[u0Col] || !samePresolveBound(lb[cg], lb[u0Col]) || !samePresolveBound(ub[cg], ub[u0Col]) || colPtr[cg+1]-colPtr[cg] != degC0 {
								return 0, false, false
							}
							for prev := range g {
								if newOrb[prev] == cg {
									return 0, false, false
								}
							}
						}
						orbID := len(sc.orbits) / gCount
						for g := range gCount {
							cg := newOrb[g]
							colOrbit[cg] = orbID
							colGroup[cg] = g
						}
						sc.orbits = append(sc.orbits, newOrb[:gCount]...)
					}
					continue
				}
			}

			needOrd := gCount * rowLen
			if cap(sc.orderBuf) < needOrd {
				sc.orderBuf = make([]int, needOrd)
			} else {
				sc.orderBuf = sc.orderBuf[:needOrd]
			}
			if cap(sc.sortKeys) < rowLen {
				sc.sortKeys = make([]orbitSortEntry, rowLen)
			}
			keys := sc.sortKeys[:rowLen]
			for g := range gCount {
				rg := &rows[tuple[g]]
				ord := sc.orderBuf[g*rowLen : (g+1)*rowLen]
				for idx, col := range rg.cols {
					orb := -1
					if colGroup[col] == g {
						orb = colOrbit[col]
					}
					keys[idx] = orbitSortEntry{
						idx:    idx,
						col:    col,
						orbit:  orb,
						deg:    colPtr[col+1] - colPtr[col],
						varCat: orig.varCat[col],
						val:    rg.vals[idx],
						lb:     lb[col],
						ub:     ub[col],
					}
				}
				slices.SortFunc(keys, func(a, b orbitSortEntry) int {
					ga, gb := a.orbit >= 0, b.orbit >= 0
					if ga != gb {
						if ga {
							return -1
						}
						return 1
					}
					if ga {
						return cmp.Compare(a.orbit, b.orbit)
					}
					if c := cmp.Compare(a.val, b.val); c != 0 {
						return c
					}
					if c := cmp.Compare(a.varCat, b.varCat); c != 0 {
						return c
					}
					if c := cmp.Compare(a.lb, b.lb); c != 0 {
						return c
					}
					if c := cmp.Compare(a.ub, b.ub); c != 0 {
						return c
					}
					if c := cmp.Compare(a.deg, b.deg); c != 0 {
						return c
					}
					return cmp.Compare(a.col, b.col)
				})
				for pos, e := range keys {
					ord[pos] = e.idx
				}
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
			if !rg.active || rowState[rgIdx] != g || rg.sense != r0.sense || len(rg.cols) != len(r0.cols) || !sameWorkRowRHS(rg, r0) {
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

type lpSingletonAction struct {
	origRow int
	col     int
	coeff   float64
	sense   ConstraintSense
	isUpper bool
}

// LPPresolveResult holds a dual-exact reduced LP problem and the postsolve mapping
// needed to recover primal, dual, and reduced-cost KKT vectors on the original LP.
type LPPresolveResult struct {
	Reduced         *CompiledProblem
	Infeasible      bool
	TimedOut        bool
	OrigRowIdx      []int
	Singletons      []lpSingletonAction
	FixedSubst      int
	TightenedBounds int
	RemovedRows     int
}

// presolveLPWithDeadline performs dual-exact LP presolve (fixed-variable substitution,
// singleton bound absorption, redundant/empty row dropping, and dual-dominated column fixing).
func presolveLPWithDeadline(orig *CompiledProblem, maxPasses int, deadline time.Time, done <-chan struct{}) LPPresolveResult {
	if maxPasses <= 0 {
		maxPasses = defaultPresolveMaxPass
	}
	if stopRequested(deadline, done) {
		return LPPresolveResult{Reduced: orig, TimedOut: true}
	}
	n := orig.numVars
	m := orig.numConstraints
	if n == 0 || m == 0 {
		return LPPresolveResult{Reduced: orig}
	}

	lb := slices.Clone(orig.varLB)
	ub := slices.Clone(orig.varUB)
	for j := range n {
		if lb[j] > ub[j] {
			return LPPresolveResult{Infeasible: true}
		}
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

	colSubstDone := make([]bool, n)
	colSkipped := make([]bool, n)
	colDirMask := make([]uint8, n) // bit 0: blocks up (LE a>0 or GE a<0), bit 1: blocks down (LE a<0 or GE a>0), bit 2: touches EQ
	var nzSubstIdx []int
	var singletons []lpSingletonAction
	fixedSubst := 0
	tightened := 0
	removedRows := 0

	const (
		lpFixedMaxVal      = 1e12
		lpFixedMaxTermRel  = 1e8
		lpFixedMaxRHSMag   = 1e14
		lpFixedMaxRHSErr   = 1e-11
		lpSingletonMinCoef = 1e-9
		lpSingletonMaxBnd  = 1e12
		lpRedundMargin     = 1e-6
	)

	exactSingletonRes := func(a, xVal, rhs float64) float64 {
		var acc dot2Acc
		acc.addVal(-rhs)
		acc.addProd(a, xVal)
		if v, _, ok := acc.result(); ok {
			return v
		}
		var w wideDotAcc
		w.addVal(-rhs)
		w.addProd(a, xVal)
		return w.finish(math.NaN())
	}

	for range maxPasses {
		if stopRequested(deadline, done) {
			return LPPresolveResult{Reduced: orig, TimedOut: true}
		}
		progress := false

		// 1. Substitute fixed variables (both zero and bounded non-zero) out of active rows.
		hasNewFixed := false
		for j := range n {
			if !colSubstDone[j] && lb[j] == ub[j] && !math.IsNaN(lb[j]) && math.Abs(lb[j]) <= lpFixedMaxVal {
				hasNewFixed = true
				break
			}
		}
		if hasNewFixed {
			clear(colSkipped)
			for i := range m {
				r := &rows[i]
				if !r.active || len(r.cols) == 0 {
					continue
				}
				k := 0
				for k < len(r.cols) {
					col := r.cols[k]
					if !colSubstDone[col] && lb[col] == 0 && ub[col] == 0 {
						r.ensureOwned()
						last := len(r.cols) - 1
						r.cols[k] = r.cols[last]
						r.vals[k] = r.vals[last]
						r.cols = r.cols[:last]
						r.vals = r.vals[:last]
						fixedSubst++
						progress = true
						continue
					}
					k++
				}
				nzSubstIdx = nzSubstIdx[:0]
				scale := max(1.0, math.Abs(r.rhs))
				for k, col := range r.cols {
					if !colSubstDone[col] && lb[col] == ub[col] && lb[col] != 0 && !math.IsNaN(lb[col]) && math.Abs(lb[col]) <= lpFixedMaxVal {
						term := r.vals[k] * lb[col]
						if !isNonFinite(term) && math.Abs(term) <= lpFixedMaxTermRel*scale {
							nzSubstIdx = append(nzSubstIdx, k)
						} else {
							colSkipped[col] = true
						}
					}
				}
				if len(nzSubstIdx) > 0 {
					var w wideDotAcc
					w.addVal(r.rhs)
					for _, idx := range nzSubstIdx {
						w.addProd(-r.vals[idx], lb[r.cols[idx]])
					}
					top, rem, ok := w.finishDD()
					if ok && !isNonFinite(top) && math.Abs(top) <= lpFixedMaxRHSMag && math.Abs(rem) <= lpFixedMaxRHSErr {
						r.ensureOwned()
						r.rhs = top
						for _, pos := range slices.Backward(nzSubstIdx) {
							last := len(r.cols) - 1
							r.cols[pos] = r.cols[last]
							r.vals[pos] = r.vals[last]
							r.cols = r.cols[:last]
							r.vals = r.vals[:last]
							fixedSubst++
						}
						progress = true
					} else {
						for _, idx := range nzSubstIdx {
							colSkipped[r.cols[idx]] = true
						}
					}
				}
			}
			for j := range n {
				if !colSubstDone[j] && !colSkipped[j] && lb[j] == ub[j] && !math.IsNaN(lb[j]) && math.Abs(lb[j]) <= lpFixedMaxVal {
					colSubstDone[j] = true
				}
			}
		}

		// 2. Singleton row bound absorption and empty/redundant row removal.
		for i := range m {
			r := &rows[i]
			if !r.active {
				continue
			}
			if len(r.cols) == 0 {
				switch r.sense {
				case LessEqual:
					if 0 > r.rhs+presolveFeasTol {
						return LPPresolveResult{Infeasible: true}
					}
				case GreaterEqual:
					if 0 < r.rhs-presolveFeasTol {
						return LPPresolveResult{Infeasible: true}
					}
				case Equal:
					if math.Abs(r.rhs) > presolveFeasTol {
						return LPPresolveResult{Infeasible: true}
					}
				}
				r.active = false
				removedRows++
				progress = true
				continue
			}
			if len(r.cols) == 1 {
				col := r.cols[0]
				a := r.vals[0]
				if math.Abs(a) >= lpSingletonMinCoef && !isNonFinite(r.rhs) {
					imp := r.rhs / a
					if !isNonFinite(imp) && math.Abs(imp) <= lpSingletonMaxBnd {
						sense := r.sense
						// Convert negative-coefficient inequality into implied bound direction.
						isUpper := (sense == LessEqual && a > 0) || (sense == GreaterEqual && a < 0)
						isLower := (sense == GreaterEqual && a > 0) || (sense == LessEqual && a < 0)
						colFeasTol := presolveFeasTol / max(1.0, math.Abs(a))
						if sense == Equal {
							if imp < lb[col]-colFeasTol || imp > ub[col]+colFeasTol {
								return LPPresolveResult{Infeasible: true}
							}
							clamped := min(max(imp, lb[col]), ub[col])
							if res := exactSingletonRes(a, clamped, r.rhs); math.IsNaN(res) || math.Abs(res) > presolveFeasTol {
								return LPPresolveResult{Infeasible: true}
							}
							if lb[col] < clamped-presolveBoundChangeTol || ub[col] > clamped+presolveBoundChangeTol {
								lb[col] = clamped
								ub[col] = clamped
								tightened++
								singletons = append(singletons, lpSingletonAction{
									origRow: i,
									col:     col,
									coeff:   a,
									sense:   sense,
									isUpper: true,
								})
							}
							r.active = false
							removedRows++
							progress = true
							continue
						}
						if isUpper {
							if imp < lb[col]-colFeasTol {
								return LPPresolveResult{Infeasible: true}
							}
							if imp < ub[col]-presolveBoundChangeTol {
								if imp < lb[col] {
									imp = lb[col]
									res := exactSingletonRes(a, imp, r.rhs)
									viol := res
									if sense == GreaterEqual {
										viol = -res
									}
									if math.IsNaN(viol) || viol > presolveFeasTol {
										return LPPresolveResult{Infeasible: true}
									}
								}
								ub[col] = imp
								tightened++
								singletons = append(singletons, lpSingletonAction{
									origRow: i,
									col:     col,
									coeff:   a,
									sense:   sense,
									isUpper: true,
								})
								r.active = false
								removedRows++
								progress = true
								continue
							}
							if imp >= ub[col]+lpRedundMargin {
								r.active = false
								removedRows++
								progress = true
								continue
							}
						} else if isLower {
							if imp > ub[col]+colFeasTol {
								return LPPresolveResult{Infeasible: true}
							}
							if imp > lb[col]+presolveBoundChangeTol {
								if imp > ub[col] {
									imp = ub[col]
									res := exactSingletonRes(a, imp, r.rhs)
									viol := res
									if sense == GreaterEqual {
										viol = -res
									}
									if math.IsNaN(viol) || viol > presolveFeasTol {
										return LPPresolveResult{Infeasible: true}
									}
								}
								lb[col] = imp
								tightened++
								singletons = append(singletons, lpSingletonAction{
									origRow: i,
									col:     col,
									coeff:   a,
									sense:   sense,
									isUpper: false,
								})
								r.active = false
								removedRows++
								progress = true
								continue
							}
							if imp <= lb[col]-lpRedundMargin {
								r.active = false
								removedRows++
								progress = true
								continue
							}
						}
					}
				}
			}

			// Drop strictly redundant inequality rows (never drop if margin is zero/ambiguous).
			var s rowActivitySummary
			switch r.sense {
			case LessEqual:
				early := evalRowActivitySummaryInto(&s, r.cols, r.vals, lb, ub, r.rhs, true, true, true)
				if !early && !s.extreme {
					tol := max(presolveFeasTol, presolveRelScaleTol*max(1.0, max(s.l1Min, s.l1Max)))
					if s.minInfCount == 0 && s.slackHi < -tol {
						return LPPresolveResult{Infeasible: true}
					}
					if s.maxInfCount == 0 && s.deficitLo > max(lpRedundMargin, tol) {
						r.active = false
						removedRows++
						progress = true
					}
				}
			case GreaterEqual:
				early := evalRowActivitySummaryInto(&s, r.cols, r.vals, lb, ub, r.rhs, true, true, true)
				if !early && !s.extreme {
					tol := max(presolveFeasTol, presolveRelScaleTol*max(1.0, max(s.l1Min, s.l1Max)))
					if s.maxInfCount == 0 && s.deficitLo > tol {
						return LPPresolveResult{Infeasible: true}
					}
					if s.minInfCount == 0 && s.slackHi < -max(lpRedundMargin, tol) {
						r.active = false
						removedRows++
						progress = true
					}
				}
			}
		}

		// 3. Dual column fixing on unblocked/dominated columns.
		clear(colDirMask)
		for i := range m {
			r := &rows[i]
			if !r.active {
				continue
			}
			for k, col := range r.cols {
				a := r.vals[k]
				if a == 0 {
					continue
				}
				switch r.sense {
				case Equal:
					colDirMask[col] |= 4
				case LessEqual:
					if a > 0 {
						colDirMask[col] |= 1
					} else {
						colDirMask[col] |= 2
					}
				case GreaterEqual:
					if a > 0 {
						colDirMask[col] |= 2
					} else {
						colDirMask[col] |= 1
					}
				}
			}
		}
		for j := range n {
			if lb[j] == ub[j] {
				continue
			}
			mask := colDirMask[j]
			if mask&4 != 0 {
				continue
			}
			cj := orig.objMin[j]
			if mask == 0 {
				if cj > 0 && !math.IsInf(lb[j], -1) {
					ub[j] = lb[j]
					tightened++
					progress = true
				} else if cj < 0 && !math.IsInf(ub[j], 1) {
					lb[j] = ub[j]
					tightened++
					progress = true
				} else if cj == 0 {
					if !math.IsInf(lb[j], -1) && !math.IsInf(ub[j], 1) {
						val := lb[j]
						if lb[j] <= 0 && 0 <= ub[j] {
							val = 0
						}
						lb[j] = val
						ub[j] = val
						tightened++
						progress = true
					} else if !math.IsInf(lb[j], -1) {
						val := max(0.0, lb[j])
						lb[j] = val
						ub[j] = val
						tightened++
						progress = true
					} else if !math.IsInf(ub[j], 1) {
						val := min(0.0, ub[j])
						lb[j] = val
						ub[j] = val
						tightened++
						progress = true
					}
				}
			} else if mask == 1 && cj >= 0 && !math.IsInf(lb[j], -1) {
				ub[j] = lb[j]
				tightened++
				progress = true
			} else if mask == 2 && cj <= 0 && !math.IsInf(ub[j], 1) {
				lb[j] = ub[j]
				tightened++
				progress = true
			}
		}

		if !progress {
			break
		}
	}

	if tightened == 0 && removedRows == 0 && fixedSubst == 0 {
		return LPPresolveResult{Reduced: orig}
	}

	red, origRowIdx, droppedEmpty, infeas := buildCompiledDirectWithRowMap(orig, rows, lb, ub, true)
	if infeas {
		return LPPresolveResult{Infeasible: true}
	}
	removedRows += droppedEmpty
	return LPPresolveResult{
		Reduced:         red,
		OrigRowIdx:      origRowIdx,
		Singletons:      singletons,
		FixedSubst:      fixedSubst,
		TightenedBounds: tightened,
		RemovedRows:     removedRows,
	}
}

func evalOrigColDualMin(orig *CompiledProblem, col int, yMin []float64) float64 {
	var acc dot2Acc
	acc.addVal(orig.objMin[col])
	for p := orig.colPtr[col]; p < orig.colPtr[col+1]; p++ {
		yi := yMin[orig.rowIdx[p]]
		if yi != 0 {
			acc.addProd(-yi, orig.colVal[p])
		}
	}
	if v, _, ok := acc.result(); ok {
		return v
	}
	var w wideDotAcc
	w.addVal(orig.objMin[col])
	for p := orig.colPtr[col]; p < orig.colPtr[col+1]; p++ {
		yi := yMin[orig.rowIdx[p]]
		if yi != 0 {
			w.addProd(-yi, orig.colVal[p])
		}
	}
	return w.finish(orig.objMin[col])
}

//go:noinline
func evalOrigRowSlackWide(orig *CompiledProblem, row int, x []float64) float64 {
	var w wideDotAcc
	w.addVal(-orig.rowRHS[row])
	for p := orig.rowPtr[row]; p < orig.rowPtr[row+1]; p++ {
		w.addProd(orig.rowVal[p], x[orig.colIdx[p]])
	}
	return w.finish(0)
}

func evalOrigRowSlack(orig *CompiledProblem, row int, x []float64) (slack, rowL1 float64) {
	var actAcc dot2Acc
	rhs := orig.rowRHS[row]
	actAcc.addVal(-rhs)
	rowL1 = math.Abs(rhs)
	for p := orig.rowPtr[row]; p < orig.rowPtr[row+1]; p++ {
		a := orig.rowVal[p]
		xv := x[orig.colIdx[p]]
		actAcc.addProd(a, xv)
		rowL1 += math.Abs(a * xv)
	}
	if s, _, ok := actAcc.result(); ok {
		return s, rowL1
	}
	return evalOrigRowSlackWide(orig, row, x), rowL1
}

// postsolveLP reconstructs original-model Duals and ReducedCosts from a solved reduced LP,
// unwinds absorbed singleton bounds in reverse order, and verifies primal/dual KKT conditions.
func postsolveLP(orig *CompiledProblem, pre *LPPresolveResult, res *Result) bool {
	if orig == nil || pre == nil || res == nil || len(res.Values) != orig.numVars || len(res.Duals) != len(pre.OrigRowIdx) {
		return false
	}
	n := orig.numVars
	m := orig.numConstraints
	bV, rV, _ := orig.MaxViolation(res.Values, false)
	if bV > incumbentTol || rV > incumbentTol {
		return false
	}

	senseSign := 1.0
	if orig.sense == Maximize {
		senseSign = -1.0
	}
	yMin := make([]float64, m)
	for rowOut, origRow := range pre.OrigRowIdx {
		if uint(origRow) >= uint(m) {
			return false
		}
		y := senseSign * res.Duals[rowOut]
		if math.IsNaN(y) || math.IsInf(y, 0) {
			return false
		}
		yMin[origRow] = y
	}

	for _, s := range slices.Backward(pre.Singletons) {
		if s.sense != Equal {
			slack, rowL1 := evalOrigRowSlack(orig, s.origRow, res.Values)
			activeTol := max(presolveFeasTol*math.Abs(s.coeff)*max(1.0, math.Abs(res.Values[s.col])), 1e-11*rowL1)
			if (s.sense == LessEqual && slack < -activeTol) || (s.sense == GreaterEqual && slack > activeTol) {
				continue
			}
		}
		rcCol := evalOrigColDualMin(orig, s.col, yMin)
		if math.IsNaN(rcCol) || math.IsInf(rcCol, 0) {
			return false
		}
		var absorb bool
		if s.sense == Equal {
			absorb = math.Abs(rcCol) > 1e-11
		} else if s.isUpper {
			absorb = rcCol < -1e-11
		} else {
			absorb = rcCol > 1e-11
		}
		if absorb {
			yVal := rcCol / s.coeff
			if math.IsNaN(yVal) || math.IsInf(yVal, 0) {
				return false
			}
			yMin[s.origRow] = yVal
		}
	}

	const dualSignTol = 1e-5
	const compTol = 1e-3
	for i := range m {
		yi := yMin[i]
		if math.IsNaN(yi) || math.IsInf(yi, 0) {
			return false
		}
		slack, rowL1 := evalOrigRowSlack(orig, i, res.Values)
		switch orig.rowSense[i] {
		case LessEqual:
			if yi > dualSignTol*max(1.0, math.Abs(yi)) {
				return false
			}
			if yi > 0 {
				yi = 0
				yMin[i] = 0
			}
			if slack < -1e-5*rowL1 && math.Abs(yi*slack) > compTol*max(1.0, math.Abs(orig.rowRHS[i])) {
				return false
			}
		case GreaterEqual:
			if yi < -dualSignTol*max(1.0, math.Abs(yi)) {
				return false
			}
			if yi < 0 {
				yi = 0
				yMin[i] = 0
			}
			if slack > 1e-5*rowL1 && math.Abs(yi*slack) > compTol*max(1.0, math.Abs(orig.rowRHS[i])) {
				return false
			}
		}
	}

	rcOut := make([]float64, n)
	for j := range n {
		rcj := evalOrigColDualMin(orig, j, yMin)
		if math.IsNaN(rcj) || math.IsInf(rcj, 0) {
			return false
		}
		xj := res.Values[j]
		lbj, ubj := orig.varLB[j], orig.varUB[j]
		tol := dualSignTol * max(1.0, max(math.Abs(orig.objMin[j]), math.Abs(rcj)))
		if !math.IsInf(ubj, 1) && rcj < -tol && xj < ubj-1e-5 {
			if math.Abs(rcj*(ubj-xj)) > compTol*max(1.0, max(math.Abs(rcj), math.Abs(xj))) {
				return false
			}
		}
		if !math.IsInf(lbj, -1) && rcj > tol && xj > lbj+1e-5 {
			if math.Abs(rcj*(xj-lbj)) > compTol*max(1.0, max(math.Abs(rcj), math.Abs(xj))) {
				return false
			}
		}
		if math.IsInf(ubj, 1) && rcj < -tol {
			return false
		}
		if math.IsInf(lbj, -1) && rcj > tol {
			return false
		}
		rcOut[j] = senseSign * rcj
	}

	dualsOut := yMin
	if orig.sense == Maximize {
		for i := range m {
			dualsOut[i] = -dualsOut[i]
		}
	}
	res.Duals = dualsOut
	res.ReducedCosts = rcOut
	return true
}
