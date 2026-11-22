package goblin

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"time"
)

const (
	intFeasTol               = 1e-6
	heurCoeffZeroTol         = 1e-12
	heurIntBoundTol          = 1e-7
	heurDedupTol             = 1e-9
	heurSnapNearFrac         = 0.15
	minPropSteps             = 32
	propStepsPerRow          = 3
	rowQueueCapMult          = 2
	maxDiveStepsCap          = 18
	maxEqSupportRowNNZ       = 32
	heurSubLPSkipFracCap     = 32
	heurCompactLPMaxVars     = 128
	heurNoAuxSkipVars        = 256
	heurSubMIPBaseFracCap    = 120
	heurSubMIPFreeFracCap    = 240
	heurSubMIPSmallModelVars = 200
	heurSubMIPMaxNodes       = 250
	heurSubMIPCutPasses      = 3
	nodePropPasses           = 2
	subMIPPresolvePasses     = 4
	subMIPTimeLimitMillis    = 1500
	propSlackGuardRelMargin  = 0x1p-30
	propSlackGuardTolScale   = 0.25
	ownerUnassigned          = -2
	ownerPureEpigraph        = -1
	fpMinVars                = 32
	fpMaxVars                = 1024
	fpMaxPasses              = 10
	fpMinTotalIters          = 160
	fpMaxTotalIters          = 500
	fpPerPassMinIter         = 36
	fpPerPassMaxIter         = 100
	fpMaxTimeMillis          = 12
	fpMinSlackTimeMillis     = 60
	fpAlphaInit              = 0.8
	fpAlphaDecay             = 0.78
	fpAlphaFloor             = 1e-3
	fpAlphaEarlyStop         = 0.3
	fpFoundStopCap           = 2
	fpMaxStallPassesWithInc  = 4
	fpSplitmixSeedMult       = 0x9e3779b97f4a7c15
	fpFnvOffset64            = 14695981039346656037
	fpFnvPrime64             = 1099511628211
)

// canSkipBoundTightening reports whether slackLo, a certified lower bound on the row
// slack, exceeds absA*(u-l) by enough margin that residualQuotient cannot tighten
// targetBound beyond heurIntBoundTol.
func canSkipBoundTightening(slackLo, absA, l, u, targetBound float64, isInt bool) bool {
	if slackLo < 0 || math.IsInf(targetBound, 0) {
		return false
	}
	if isInt && isFractionalFloat64(targetBound) {
		return false
	}
	span := u - l
	if span <= 0 || math.IsNaN(span) || math.IsInf(span, 0) {
		return false
	}
	prod := absA * span
	if math.IsNaN(prod) || math.IsInf(prod, 0) {
		return false
	}
	return slackLo > prod+propSlackGuardRelMargin*prod-propSlackGuardTolScale*absA*heurIntBoundTol
}

// propagateRow tightens variable bounds on row i from residual row activities and reports
// whether the row remains feasible and whether any bound shrunk by more than heurIntBoundTol.
func propagateRow(p *CompiledProblem, i int, lb, ub []float64, pushCol func(col int)) (feasible, changed bool) {
	s, e := p.rowPtr[i], p.rowPtr[i+1]
	cols := p.colIdx[s:e]
	vals := p.rowVal[s:e]
	rhs := p.rowRHS[i]
	sense := p.rowSense[i]
	hasUB := sense == LessEqual || sense == Equal
	hasLB := sense == GreaterEqual || sense == Equal

	var sum rowActivitySummary
	earlyBail := evalRowActivitySummaryInto(&sum, cols, vals, lb, ub, rhs, true, hasUB, hasLB)
	if earlyBail || sum.extreme {
		return true, false
	}

	rowScale := max(1.0, max(sum.l1Min, sum.l1Max))
	rowFeasTol := max(intFeasTol, presolveRelScaleTol*rowScale)
	domScale := min(1.0, sum.domainMaxAbs())
	infeasTol := max(intFeasTol*domScale, presolveRelScaleTol*max(sum.l1Min, sum.l1Max))

	if hasUB && sum.minInfCount == 0 && sum.slackHi < -infeasTol {
		return false, false
	}
	if hasLB && sum.maxInfCount == 0 && sum.deficitLo > infeasTol {
		return false, false
	}

	upperChanged := false
	if hasUB && sum.minInfCount == 0 {
		ubSlackLo := sum.slackLo
		canGuardUB := !sum.useWideSlack
		for idx, col := range cols {
			a := vals[idx]
			l, u := lb[col], ub[col]
			if l == u {
				continue
			}
			isInt := p.varCat[col] != Continuous
			if a > heurCoeffZeroTol {
				if canGuardUB && canSkipBoundTightening(ubSlackLo, a, l, u, u, isInt) {
					continue
				}
				qVal, qEB, qOK := sum.residualQuotient(cols, vals, lb, ub, rhs, idx, true)
				if !qOK {
					continue
				}
				newUB := qVal + qEB
				if isInt {
					bTol := max(heurIntBoundTol, presolveRelScaleTol*rowScale/a)
					newUB = math.Floor(newUB + bTol)
				} else {
					newUB += rowFeasTol / a
				}
				if newUB < u-heurIntBoundTol {
					feasCheck := max(intFeasTol, rowFeasTol/a)
					if newUB < l-feasCheck {
						return false, false
					}
					clamped := max(l, newUB)
					if clamped < u-heurIntBoundTol {
						ub[col] = clamped
						changed = true
						upperChanged = true
						if pushCol != nil {
							pushCol(col)
						}
					}
				}
			} else if a < -heurCoeffZeroTol {
				absA := -a
				if canGuardUB && canSkipBoundTightening(ubSlackLo, absA, l, u, l, isInt) {
					continue
				}
				qVal, qEB, qOK := sum.residualQuotient(cols, vals, lb, ub, rhs, idx, true)
				if !qOK {
					continue
				}
				newLB := qVal - qEB
				if isInt {
					bTol := max(heurIntBoundTol, presolveRelScaleTol*rowScale/absA)
					newLB = math.Ceil(newLB - bTol)
				} else {
					newLB -= rowFeasTol / absA
				}
				if newLB > l+heurIntBoundTol {
					feasCheck := max(intFeasTol, rowFeasTol/absA)
					if newLB > u+feasCheck {
						return false, false
					}
					clamped := min(u, newLB)
					if clamped > l+heurIntBoundTol {
						lb[col] = clamped
						changed = true
						upperChanged = true
						if pushCol != nil {
							pushCol(col)
						}
					}
				}
			}
		}
	}

	if hasLB && upperChanged {
		evalRowActivitySummaryInto(&sum, cols, vals, lb, ub, rhs, false, false, true)
		if sum.extreme {
			return true, changed
		}
		rowScale = max(1.0, sum.l1Max)
		rowFeasTol = max(intFeasTol, presolveRelScaleTol*rowScale)
		domScale = min(1.0, sum.domainMaxAbs())
		infeasTol = max(intFeasTol*domScale, presolveRelScaleTol*sum.l1Max)
		if sum.maxInfCount == 0 && sum.deficitLo > infeasTol {
			return false, false
		}
	}

	if hasLB && sum.maxInfCount == 0 {
		lbSlackLo := -sum.deficitHi
		canGuardLB := !sum.useWideDeficit
		for idx, col := range cols {
			a := vals[idx]
			l, u := lb[col], ub[col]
			if l == u {
				continue
			}
			isInt := p.varCat[col] != Continuous
			if a > heurCoeffZeroTol {
				if canGuardLB && canSkipBoundTightening(lbSlackLo, a, l, u, l, isInt) {
					continue
				}
				qVal, qEB, qOK := sum.residualQuotient(cols, vals, lb, ub, rhs, idx, false)
				if !qOK {
					continue
				}
				newLB := qVal - qEB
				if isInt {
					bTol := max(heurIntBoundTol, presolveRelScaleTol*rowScale/a)
					newLB = math.Ceil(newLB - bTol)
				} else {
					newLB -= rowFeasTol / a
				}
				if newLB > l+heurIntBoundTol {
					feasCheck := max(intFeasTol, rowFeasTol/a)
					if newLB > u+feasCheck {
						return false, false
					}
					clamped := min(u, newLB)
					if clamped > l+heurIntBoundTol {
						lb[col] = clamped
						changed = true
						if pushCol != nil {
							pushCol(col)
						}
					}
				}
			} else if a < -heurCoeffZeroTol {
				absA := -a
				if canGuardLB && canSkipBoundTightening(lbSlackLo, absA, l, u, u, isInt) {
					continue
				}
				qVal, qEB, qOK := sum.residualQuotient(cols, vals, lb, ub, rhs, idx, false)
				if !qOK {
					continue
				}
				newUB := qVal + qEB
				if isInt {
					bTol := max(heurIntBoundTol, presolveRelScaleTol*rowScale/absA)
					newUB = math.Floor(newUB + bTol)
				} else {
					newUB += rowFeasTol / absA
				}
				if newUB < u-heurIntBoundTol {
					feasCheck := max(intFeasTol, rowFeasTol/absA)
					if newUB < l-feasCheck {
						return false, false
					}
					clamped := max(l, newUB)
					if clamped < u-heurIntBoundTol {
						ub[col] = clamped
						changed = true
						if pushCol != nil {
							pushCol(col)
						}
					}
				}
			}
		}
	}
	return true, changed
}

func evalFixedRowResidual(cols []int, vals, x []float64, isFree []bool, rhs float64) (remRHS float64, ok bool) {
	remRHS, _, ok = evalFixedRowResidualExact(cols, vals, x, isFree, rhs)
	return remRHS, ok
}

// evalFixedRowResidualExact computes rhs - sum_{fixed} vals[k]*x[col] and reports exact=true
// only when every intermediate product and sum is provably error-free in float64.
func evalFixedRowResidualExact(cols []int, vals, x []float64, isFree []bool, rhs float64) (remRHS float64, exact, ok bool) {
	if math.IsNaN(rhs) || math.IsInf(rhs, 0) {
		return 0, false, false
	}
	var acc dot2Acc
	acc.addVal(rhs)
	for k, col := range cols {
		if isFree != nil && isFree[col] {
			continue
		}
		acc.addProd(-vals[k], x[col])
	}
	if !acc.nonFin && acc.lowCount == 0 && acc.errL1 == 0 && acc.c == 0 && !isNonFinite(acc.s) {
		return acc.s, true, true
	}
	if val, eb, accOK := acc.result(); accOK && eb <= 0x1p-20*max(1.0, math.Abs(val)) {
		return val, false, true
	}
	var w wideDotAcc
	w.addVal(rhs)
	for k, col := range cols {
		if isFree != nil && isFree[col] {
			continue
		}
		w.addProd(-vals[k], x[col])
	}
	val := w.finish(math.NaN())
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return 0, false, false
	}
	return val, false, true
}

func propagateBounds(p *CompiledProblem, lb, ub []float64, maxPasses int) bool {
	m := p.numConstraints
	for range maxPasses {
		anyChanged := false
		for i := range m {
			feas, chg := propagateRow(p, i, lb, ub, nil)
			if !feas {
				return false
			}
			if chg {
				anyChanged = true
			}
		}
		if !anyChanged {
			break
		}
	}
	return true
}

func propagateFromCol(p *CompiledProblem, lb, ub []float64, startCol int, queue *[]int, inQueue []bool) bool {
	q := (*queue)[:0]
	pushCol := func(col int) {
		for k := p.colPtr[col]; k < p.colPtr[col+1]; k++ {
			r := p.rowIdx[k]
			if !inQueue[r] {
				inQueue[r] = true
				q = append(q, r)
			}
		}
	}
	pushCol(startCol)

	head := 0
	maxSteps := max(minPropSteps, propStepsPerRow*p.numConstraints)
	for head < len(q) && head < maxSteps {
		r := q[head]
		head++
		inQueue[r] = false
		feas, _ := propagateRow(p, r, lb, ub, pushCol)
		if !feas {
			for ; head < len(q); head++ {
				inQueue[q[head]] = false
			}
			*queue = q[:0]
			return false
		}
	}
	for ; head < len(q); head++ {
		inQueue[q[head]] = false
	}
	*queue = q[:0]
	return true
}

// findContLinkedIntCols marks general integer columns whose nonzeros appear only in mixed rows
// coupling continuous variables with a single integer variable (such as roundness-penalty multiples).
func findContLinkedIntCols(p *CompiledProblem) []bool {
	if len(p.intCols) == 0 || len(p.intCols) == p.numVars {
		return nil
	}
	rowHasCont := make([]bool, p.numConstraints)
	for i := range p.numConstraints {
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		nCont, nInt := 0, 0
		for k := s; k < e; k++ {
			if p.varCat[p.colIdx[k]] == Continuous {
				nCont++
			} else {
				nInt++
			}
		}
		if nCont > 0 && nInt == 1 {
			rowHasCont[i] = true
		}
	}
	var isContLinked []bool
	for _, col := range p.intCols {
		if p.varCat[col] != Integer {
			continue
		}
		s, e := p.colPtr[col], p.colPtr[col+1]
		if s == e {
			continue
		}
		allContRows := true
		for k := s; k < e; k++ {
			if !rowHasCont[p.rowIdx[k]] {
				allContRows = false
				break
			}
		}
		if allContRows {
			if isContLinked == nil {
				isContLinked = make([]bool, p.numVars)
			}
			isContLinked[col] = true
		}
	}
	return isContLinked
}

// rowDomainMaxAbsAtMost1 returns min(1.0, rowDomainMaxAbs(cols, vals, lb, ub)),
// exiting immediately on the first non-zero-fixed column whose |a_k| >= 1.0.
func rowDomainMaxAbsAtMost1(cols []int, vals, lb, ub []float64) float64 {
	domMaxA := 0.0
	rowMaxA := 0.0
	for k, col := range cols {
		av := math.Abs(vals[k])
		if lb[col] != 0 || ub[col] != 0 {
			if av >= 1.0 {
				return 1.0
			}
			if av > domMaxA {
				domMaxA = av
			}
		} else if av > rowMaxA {
			rowMaxA = av
		}
	}
	if domMaxA > 0 {
		return domMaxA
	}
	if rowMaxA > 0 {
		return min(1.0, rowMaxA)
	}
	return 1.0
}

// evalRowActiveL1 returns the active L1 norm of row at x.
func evalRowActiveL1(p *CompiledProblem, row int, x []float64) float64 {
	l1 := 0.0
	if !math.IsInf(p.rowRHS[row], 0) && !math.IsNaN(p.rowRHS[row]) {
		l1 = math.Abs(p.rowRHS[row])
	}
	s, e := p.rowPtr[row], p.rowPtr[row+1]
	for k := s; k < e; k++ {
		l1 += math.Abs(p.rowVal[k] * x[p.colIdx[k]])
	}
	return l1
}

// intFixedRowMaxAbsAtMost1 returns min(1.0, largest coefficient magnitude in row among columns
// that are continuous with a non-zero domain or integer with a non-zero value in x),
// exiting immediately once an active coefficient >= 1.0 is observed.
func intFixedRowMaxAbsAtMost1(p *CompiledProblem, row int, x []float64) float64 {
	s, e := p.rowPtr[row], p.rowPtr[row+1]
	actMaxA := 0.0
	for k := s; k < e; k++ {
		col := p.colIdx[k]
		av := math.Abs(p.rowVal[k])
		if p.varCat[col] == Continuous {
			if p.varLB[col] != 0 || p.varUB[col] != 0 {
				if av >= 1.0 {
					return 1.0
				}
				if av > actMaxA {
					actMaxA = av
				}
			}
		} else if x[col] != 0 {
			if av >= 1.0 {
				return 1.0
			}
			if av > actMaxA {
				actMaxA = av
			}
		}
	}
	if actMaxA > 0 {
		return actMaxA
	}
	if p.lazy != nil && row < len(p.lazy.rowMaxAbs) {
		return min(1.0, p.lazy.rowMaxAbs[row])
	}
	rowMaxA := 0.0
	for k := s; k < e; k++ {
		if av := math.Abs(p.rowVal[k]); av > rowMaxA {
			rowMaxA = av
		}
	}
	return min(1.0, rowMaxA)
}

// incumbentRowTol computes the largest allowed residual on row for base tolerance tol,
// scaling down on small-norm or Ruiz-amplified rows so heuristic and polished incumbents
// match the scaled LP feasibility contract.
func incumbentRowTol(p *CompiledProblem, sc *scaledLPData, row int, tol float64) float64 {
	rowTol := tol
	var rowMaxA float64
	if p.lazy != nil && row < len(p.lazy.rowMaxAbs) {
		rowMaxA = p.lazy.rowMaxAbs[row]
	} else {
		s, e := p.rowPtr[row], p.rowPtr[row+1]
		for k := s; k < e; k++ {
			if av := math.Abs(p.rowVal[k]); av > rowMaxA {
				rowMaxA = av
			}
		}
	}
	if rowMaxA > 0 && rowMaxA < 1.0 {
		rowTol = min(rowTol, tol*rowMaxA)
	}
	if sc != nil && sc.scaled && row < len(sc.rowScale) && sc.rowScale[row] > 1.0 {
		rowTol = min(rowTol, tol/sc.rowScale[row])
	}
	return rowTol
}

// evalRowViolation computes the nonnegative constraint residual excess of row at x.
// It returns ok=false when the row activity or residual is NaN or infinite.
func evalRowViolation(p *CompiledProblem, row int, x []float64) (float64, bool) {
	s, e := p.rowPtr[row], p.rowPtr[row+1]
	cols := p.colIdx[s:e]
	vals := p.rowVal[s:e]
	lb, ub := p.rowLB[row], p.rowUB[row]
	hasLB := !math.IsInf(lb, -1)
	hasUB := !math.IsInf(ub, 1)
	switch {
	case hasLB && hasUB && lb == ub:
		res := evalSparseRowResidual(cols, vals, x, lb)
		if isNonFinite(res) {
			return 0, false
		}
		return math.Abs(res), true
	case !hasLB && !hasUB:
		if math.IsNaN(evalSparseRowSum(cols, vals, x)) {
			return 0, false
		}
		return 0, true
	default:
		viol := 0.0
		if hasLB {
			res := evalSparseRowResidual(cols, vals, x, lb)
			if isNonFinite(res) {
				return 0, false
			}
			if res < 0 {
				viol = max(viol, -res)
			}
		}
		if hasUB {
			res := evalSparseRowResidual(cols, vals, x, ub)
			if isNonFinite(res) {
				return 0, false
			}
			if res > 0 {
				viol = max(viol, res)
			}
		}
		return viol, true
	}
}

// incumbentFeasibleScaled reports whether x satisfies all variable bounds, integrality
// constraints, and row-normalized or Ruiz-scaled constraint residuals within tol.
func incumbentFeasibleScaled(p *CompiledProblem, x []float64, tol float64) bool {
	return incumbentFeasibleWithDual(p, x, nil, tol)
}

// incumbentFeasibleWithDual extends incumbentFeasibleScaled with optional per-row root dual
// amplification factors so rows whose large dual multipliers magnify tiny residuals cannot
// accept infeasible incumbents that beat the LP relaxation.
func incumbentFeasibleWithDual(p *CompiledProblem, x, dualAmp []float64, tol float64) bool {
	if p == nil || len(x) < p.numVars {
		return false
	}
	for j := range p.numVars {
		v := x[j]
		if isNonFinite(v) {
			return false
		}
		if v < p.varLB[j]-tol || v > p.varUB[j]+tol {
			return false
		}
		if p.varCat[j] != Continuous && math.Abs(v-math.Round(v)) > tol {
			return false
		}
	}
	var sc *scaledLPData
	scLoaded := false
	for i := range p.numConstraints {
		viol, ok := evalRowViolation(p, i, x)
		if !ok {
			return false
		}
		if viol == 0 {
			continue
		}
		if viol > tol {
			return false
		}
		if !scLoaded {
			sc = p.getScaledLP()
			scLoaded = true
		}
		rowTol := incumbentRowTol(p, sc, i, tol)
		if viol > rowTol {
			return false
		}
		if actMaxA := intFixedRowMaxAbsAtMost1(p, i, x); actMaxA > 0 && actMaxA < 1.0 {
			rowTol = min(rowTol, tol*actMaxA)
			if viol > rowTol {
				return false
			}
		}
		if i < len(dualAmp) && dualAmp[i] > 1.0 && tol > 0 {
			// Retain a 16-ulp active-activity floor so float64 coordinate rounding on
			// valid continuous optima is never rejected under large dual multipliers.
			dualTol := max(tol/dualAmp[i], 16.0*unitRoundoff64*evalRowActiveL1(p, i, x))
			rowTol = min(rowTol, dualTol)
			if viol > rowTol {
				return false
			}
		}
	}
	return true
}

// fixedContFeasibleScaled checks a continuous assignment against row-normalized tolerances
// and confirms no active row residual exceeds primalTol relative to its non-fixed continuous terms.
// When a fixed-to-zero big-M term hides a tiny active continuous coefficient, returning false
// forces solveFixedIntContinuousLP to fall through to the stripped continuous sub-LP.
func fixedContFeasibleScaled(p *CompiledProblem, x, hlb, hub []float64) bool {
	if !incumbentFeasibleScaled(p, x, primalTol) {
		return false
	}
	for i := range p.numConstraints {
		viol, ok := evalRowViolation(p, i, x)
		if !ok {
			return false
		}
		if viol == 0 {
			continue
		}
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		cols := p.colIdx[s:e]
		vals := p.rowVal[s:e]
		maxFreeA := 0.0
		for k, col := range cols {
			if hlb[col] < hub[col] {
				if av := math.Abs(vals[k]); av > maxFreeA {
					maxFreeA = av
				}
			}
		}
		if maxFreeA > 0 && maxFreeA < 1.0 && viol > primalTol*maxFreeA {
			return false
		}
	}
	return true
}

// tryPropagatedRounding rounds fractional integer columns in ascending fractionality order
// with queue-driven bound propagation, testing nearest and objective-directed rounding strategies.
func tryPropagatedRounding(
	p *CompiledProblem,
	lpPrimal []float64,
	curLB, curUB []float64,
	auxSolver *LPSolver,
	searchShift float64,
	isContLinked []bool,
) (sol []float64, objMin float64, ok bool) {
	n := p.numVars
	hasCont := len(p.intCols) < n

	type cand struct {
		col  int
		frac float64
	}
	baseHLB := slices.Clone(curLB)
	baseHUB := slices.Clone(curUB)
	fracCands := make([]cand, 0, min(len(p.intCols), 32))
	fixedAny := false
	for _, col := range p.intCols {
		if isContLinked != nil && isContLinked[col] {
			continue
		}
		v := lpPrimal[col]
		rv := math.Round(v)
		f := math.Abs(v - rv)
		if f <= intFeasTol {
			iv := min(max(rv, baseHLB[col]), baseHUB[col])
			baseHLB[col] = iv
			baseHUB[col] = iv
			fixedAny = true
			continue
		}
		fracCands = append(fracCands, cand{col: col, frac: f})
	}
	if len(fracCands) > 1 {
		slices.SortFunc(fracCands, func(a, b cand) int {
			return cmp.Compare(a.frac, b.frac)
		})
	}
	if fixedAny {
		if !propagateBounds(p, baseHLB, baseHUB, 1) {
			return nil, 0, false
		}
	}
	lpFallbackSolver := auxSolver
	if hasCont && len(fracCands) > heurSubLPSkipFracCap {
		lpFallbackSolver = nil
	}

	hlb := make([]float64, n)
	hub := make([]float64, n)
	candX := make([]float64, n)
	rowQueue := make([]int, 0, p.numConstraints*rowQueueCapMult)
	inQueue := make([]bool, p.numConstraints)

	var bestSol []float64
	bestObj := math.Inf(1)

	maxStrat := 2
	if len(fracCands) == 0 {
		maxStrat = 1
	}

	snapHLB := make([]float64, n)
	snapHUB := make([]float64, n)
	var prevHLB, prevHUB []float64
	hasPrevFixed := false
	for strategy := range maxStrat {
		if auxSolver != nil && auxSolver.timeExpired() {
			break
		}
		copy(hlb, baseHLB)
		copy(hub, baseHUB)
		feasible := true

		for idx, c := range fracCands {
			if (idx&15 == 0) && auxSolver != nil && auxSolver.timeExpired() {
				feasible = false
				break
			}
			col := c.col
			if hlb[col] == hub[col] {
				continue
			}
			v := min(max(lpPrimal[col], hlb[col]), hub[col])
			var target float64
			if strategy == 0 || math.Abs(v-math.Round(v)) <= heurSnapNearFrac {
				target = math.Round(v)
			} else {
				if p.objMin[col] > heurCoeffZeroTol {
					target = math.Floor(v)
				} else if p.objMin[col] < -heurCoeffZeroTol {
					target = math.Ceil(v)
				} else if v >= halfFracTarget {
					target = math.Floor(v)
				} else {
					target = math.Ceil(v)
				}
			}
			target = min(max(target, hlb[col]), hub[col])

			copy(snapHLB, hlb)
			copy(snapHUB, hub)
			savedLB := hlb[col]
			savedUB := hub[col]
			hlb[col] = target
			hub[col] = target

			if !propagateFromCol(p, hlb, hub, col, &rowQueue, inQueue) {
				copy(hlb, snapHLB)
				copy(hub, snapHUB)
				alt := math.Ceil(v)
				if target == alt {
					alt = math.Floor(v)
				}
				if alt >= savedLB && alt <= savedUB {
					hlb[col] = alt
					hub[col] = alt
					if !propagateFromCol(p, hlb, hub, col, &rowQueue, inQueue) {
						feasible = false
						break
					}
				} else {
					feasible = false
					break
				}
			}
		}

		if !feasible {
			continue
		}
		if hasPrevFixed && slices.Equal(hlb, prevHLB) && slices.Equal(hub, prevHUB) {
			continue
		}
		if strategy == 0 && maxStrat > 1 {
			prevHLB = slices.Clone(hlb)
			prevHUB = slices.Clone(hub)
			hasPrevFixed = true
		}

		if !hasCont {
			for j := range n {
				candX[j] = hlb[j]
			}
			if incumbentFeasibleScaled(p, candX, primalTol) {
				obj := evalLinearSum(searchShift, p.objMin, candX, n)
				if !math.IsNaN(obj) && !math.IsInf(obj, 0) && obj < bestObj {
					bestObj = obj
					bestSol = slices.Clone(candX)
				}
			}
		} else if auxSolver != nil {
			if sSol, _, sOk := solveFixedIntContinuousLP(p, hlb, hub, lpFallbackSolver, isContLinked); sOk {
				if sObj := evalLinearSum(searchShift, p.objMin, sSol, n); !math.IsNaN(sObj) && !math.IsInf(sObj, 0) && sObj < bestObj {
					bestObj = sObj
					bestSol = sSol
				}
			}
		}
	}

	if bestSol != nil {
		return bestSol, bestObj, true
	}
	return nil, 0, false
}

type epigraphRowTerm struct {
	epiCol int
	epiIdx int
	c      float64
	alpha  float64
	beta   float64
}

// solveEpigraphContinuousLP solves the fixed-integer continuous (plus ContLinked integer)
// subproblem in closed form when every active constraint row is an epigraph lower bound
// z >= c + alpha * m + beta * n coupling an epigraph variable z (ObjMin[z] > 0, UB = +Inf)
// with at most one non-epigraph continuous variable m and one ContLinked integer variable n.
func solveEpigraphContinuousLP(p *CompiledProblem, hlb, hub []float64, isContLinked []bool) ([]float64, float64, bool) {
	n := p.numVars
	m := p.numConstraints

	// Check if all free variables are either Continuous or ContLinked integers.
	for _, col := range p.intCols {
		if hlb[col] < hub[col] && (isContLinked == nil || !isContLinked[col]) {
			return nil, 0, false
		}
	}

	// Identify candidate epigraph continuous variables.
	isEpi := make([]bool, n)
	for j := range n {
		if hlb[j] < hub[j] && p.varCat[j] == Continuous && p.objMin[j] > heurCoeffZeroTol &&
			!math.IsInf(hlb[j], -1) && math.IsInf(hub[j], 1) {
			isEpi[j] = true
		}
	}

	// Verify active rows and refine isEpi: in every active row touching z, z must be lower-bounded.
	for i := range m {
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		hasFree := false
		for k := s; k < e; k++ {
			if col := p.colIdx[k]; hlb[col] < hub[col] {
				hasFree = true
				break
			}
		}
		if !hasFree {
			continue
		}
		sense := p.rowSense[i]
		for k := s; k < e; k++ {
			col := p.colIdx[k]
			if !isEpi[col] {
				continue
			}
			a := p.rowVal[k]
			if !((sense == GreaterEqual && a > heurCoeffZeroTol) || (sense == LessEqual && a < -heurCoeffZeroTol)) {
				isEpi[col] = false
			}
		}
	}

	// Map each non-epigraph free variable into an independent component around contCol.
	// Sentinel values in epiOwner and intOwner: ownerUnassigned (-2) before first row visit,
	// ownerPureEpigraph (-1) for epigraph columns with no partner variables, or contCol >= 0.
	epiOwner := make([]int, n)
	intOwner := make([]int, n)
	contIntPartner := make([]int, n)
	for j := range n {
		epiOwner[j] = ownerUnassigned
		intOwner[j] = ownerUnassigned
		contIntPartner[j] = -1
	}

	compIdxByCont := make([]int, n)
	for j := range n {
		compIdxByCont[j] = -1
	}
	var comps []epigraphComponent
	var pureEpiMaxLB []float64

	getComp := func(contCol int) int {
		idx := compIdxByCont[contCol]
		if idx == -1 {
			idx = len(comps)
			compIdxByCont[contCol] = idx
			comps = append(comps, epigraphComponent{contCol: contCol, intCol: -1})
		}
		return idx
	}

	isFreeEpi := make([]bool, n)
	for j := range n {
		if hlb[j] < hub[j] {
			isFreeEpi[j] = true
		}
	}

	for i := range m {
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		epiCol := -1
		contCol := -1
		intCol := -1
		freeCount := 0
		for k := s; k < e; k++ {
			col := p.colIdx[k]
			if !isFreeEpi[col] {
				continue
			}
			freeCount++
			switch {
			case isEpi[col] && epiCol == -1:
				epiCol = col
			case p.varCat[col] == Continuous && !isEpi[col] && contCol == -1:
				contCol = col
			case p.varCat[col] != Continuous && intCol == -1:
				intCol = col
			default:
				return nil, 0, false
			}
		}
		remRHS, remOK := evalFixedRowResidual(p.colIdx[s:e], p.rowVal[s:e], hlb, isFreeEpi, p.rowRHS[i])
		if !remOK {
			return nil, 0, false
		}
		sense := p.rowSense[i]
		if freeCount == 0 {
			switch sense {
			case LessEqual:
				if remRHS < -intFeasTol {
					return nil, 0, false
				}
			case GreaterEqual:
				if remRHS > intFeasTol {
					return nil, 0, false
				}
			case Equal:
				if math.Abs(remRHS) > intFeasTol {
					return nil, 0, false
				}
			}
			continue
		}
		if epiCol == -1 {
			return nil, 0, false
		}
		var aEpi, aCont, aInt float64
		for k := s; k < e; k++ {
			col := p.colIdx[k]
			switch col {
			case epiCol:
				aEpi = p.rowVal[k]
			case contCol:
				aCont = p.rowVal[k]
			case intCol:
				aInt = p.rowVal[k]
			}
		}
		cTerm := remRHS / aEpi
		alpha := -aCont / aEpi
		beta := -aInt / aEpi

		if contCol == -1 && intCol == -1 {
			if epiOwner[epiCol] != ownerUnassigned && epiOwner[epiCol] != ownerPureEpigraph {
				return nil, 0, false
			}
			epiOwner[epiCol] = ownerPureEpigraph
			if pureEpiMaxLB == nil {
				pureEpiMaxLB = slices.Clone(hlb)
			}
			if cTerm > pureEpiMaxLB[epiCol] {
				pureEpiMaxLB[epiCol] = cTerm
			}
			continue
		}
		if contCol == -1 {
			return nil, 0, false
		}
		if epiOwner[epiCol] == ownerUnassigned {
			epiOwner[epiCol] = contCol
		} else if epiOwner[epiCol] != contCol {
			return nil, 0, false
		}
		cIdx := getComp(contCol)
		if intCol != -1 {
			if intOwner[intCol] == ownerUnassigned {
				intOwner[intCol] = contCol
			} else if intOwner[intCol] != contCol {
				return nil, 0, false
			}
			if comps[cIdx].intCol == -1 {
				comps[cIdx].intCol = intCol
				contIntPartner[contCol] = intCol
			} else if comps[cIdx].intCol != intCol {
				return nil, 0, false
			}
		}
		epiIdx := slices.Index(comps[cIdx].epiCols, epiCol)
		if epiIdx < 0 {
			epiIdx = len(comps[cIdx].epiCols)
			comps[cIdx].epiCols = append(comps[cIdx].epiCols, epiCol)
		}
		comps[cIdx].rows = append(comps[cIdx].rows, epigraphRowTerm{
			epiCol: epiCol,
			epiIdx: epiIdx,
			c:      cTerm,
			alpha:  alpha,
			beta:   beta,
		})
	}

	cand := slices.Clone(hlb)
	for j := range n {
		if hlb[j] < hub[j] {
			if pureEpiMaxLB != nil && epiOwner[j] == ownerPureEpigraph {
				cand[j] = pureEpiMaxLB[j]
			} else if p.objMin[j] < 0 {
				if math.IsInf(hub[j], 1) {
					return nil, 0, false
				}
				cand[j] = hub[j]
			} else if math.IsInf(hlb[j], -1) {
				if !math.IsInf(hub[j], 1) && p.objMin[j] == 0 {
					cand[j] = hub[j]
				} else {
					cand[j] = 0
				}
			}
		}
	}

	for idx := range comps {
		solveEpigraphComponent(p, &comps[idx], hlb, hub, cand)
	}

	if fixedContFeasibleScaled(p, cand, hlb, hub) {
		obj := minObjective(p, cand)
		if math.IsNaN(obj) || math.IsInf(obj, 0) {
			return nil, 0, false
		}
		return cand, obj, true
	}
	return nil, 0, false
}

type epigraphComponent struct {
	contCol int
	intCol  int
	epiCols []int
	rows    []epigraphRowTerm
}

// evalEpigraphCompObjWide isolates wideDotAcc stack allocation off solveEpigraphComponent.
//
//go:noinline
func evalEpigraphCompObjWide(objM, mv, objN, nv float64, nCol int, epiObj, epiMax []float64) float64 {
	var compObj wideDotAcc
	compObj.addProd(objM, mv)
	if nCol != -1 {
		compObj.addProd(objN, nv)
	}
	n := min(len(epiObj), len(epiMax))
	for ei := range n {
		compObj.addProd(epiObj[ei], epiMax[ei])
	}
	return compObj.finish(math.NaN())
}

// solveEpigraphComponent enumerates piecewise-linear breakpoints (m, n) of one independent epigraph component and writes the optimal assignment into cand.
func solveEpigraphComponent(p *CompiledProblem, comp *epigraphComponent, hlb, hub, cand []float64) {
	mCol := comp.contCol
	nCol := comp.intCol
	mLB, mUB := hlb[mCol], hub[mCol]

	var mCands []float64
	addMCand := func(v float64) {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return
		}
		v = min(max(v, mLB), mUB)
		for _, ex := range mCands {
			if math.Abs(ex-v) <= heurDedupTol {
				return
			}
		}
		mCands = append(mCands, v)
	}
	addMCand(mLB)
	addMCand(mUB)
	addMCand(0.0)

	rows := comp.rows
	for r1 := range rows {
		if math.Abs(rows[r1].beta) <= heurCoeffZeroTol && math.Abs(rows[r1].alpha) > heurCoeffZeroTol {
			addMCand((hlb[rows[r1].epiCol] - rows[r1].c) / rows[r1].alpha)
		}
		for r2 := r1 + 1; r2 < len(rows); r2++ {
			if rows[r1].epiCol == rows[r2].epiCol &&
				math.Abs(rows[r1].beta) <= heurCoeffZeroTol && math.Abs(rows[r2].beta) <= heurCoeffZeroTol {
				da := rows[r1].alpha - rows[r2].alpha
				if math.Abs(da) > heurCoeffZeroTol {
					addMCand((rows[r2].c - rows[r1].c) / da)
				}
			}
		}
	}

	var nCands []float64
	if nCol != -1 {
		nLB, nUB := hlb[nCol], hub[nCol]
		addNCand := func(v float64) {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return
			}
			v = min(max(math.Round(v), nLB), nUB)
			if slices.Contains(nCands, v) {
				return
			}
			nCands = append(nCands, v)
		}
		addNCand(nLB)
		addNCand(nUB)
		addNCand(0.0)
		baseMCands := slices.Clone(mCands)
		for _, mv := range baseMCands {
			for _, r := range rows {
				if math.Abs(r.beta) > heurCoeffZeroTol {
					nt := (hlb[r.epiCol] - r.c - r.alpha*mv) / r.beta
					addNCand(math.Floor(nt))
					addNCand(math.Ceil(nt))
				}
			}
		}
		for _, nv := range nCands {
			for r1 := range rows {
				if math.Abs(rows[r1].beta) > heurCoeffZeroTol && math.Abs(rows[r1].alpha) > heurCoeffZeroTol {
					addMCand((hlb[rows[r1].epiCol] - rows[r1].c - rows[r1].beta*nv) / rows[r1].alpha)
				}
				for r2 := r1 + 1; r2 < len(rows); r2++ {
					if rows[r1].epiCol == rows[r2].epiCol &&
						(math.Abs(rows[r1].beta) > heurCoeffZeroTol || math.Abs(rows[r2].beta) > heurCoeffZeroTol) {
						da := rows[r1].alpha - rows[r2].alpha
						if math.Abs(da) > heurCoeffZeroTol {
							c1 := rows[r1].c + rows[r1].beta*nv
							c2 := rows[r2].c + rows[r2].beta*nv
							addMCand((c2 - c1) / da)
						}
					}
				}
			}
		}
	} else {
		nCands = []float64{0.0}
	}

	if len(mCands) == 0 || len(nCands) == 0 {
		return
	}
	numEpi := len(comp.epiCols)
	objM := p.objMin[mCol]
	objN := 0.0
	if nCol != -1 {
		objN = p.objMin[nCol]
	}

	// Partition rows into beta==0 (m-only) and beta!=0 (m+n coupled) so m-only
	// epigraph maxima and objective terms are evaluated once per m candidate.
	var mOnlyBuf [32]epigraphRowTerm
	var mnBuf [8]epigraphRowTerm
	mOnlyRows := mOnlyBuf[:0]
	mnRows := mnBuf[:0]
	var epiHasBetaBuf [32]bool
	var epiHasBeta []bool
	if numEpi <= len(epiHasBetaBuf) {
		epiHasBeta = epiHasBetaBuf[:numEpi]
	} else {
		epiHasBeta = make([]bool, numEpi)
	}
	for _, r := range rows {
		if r.beta == 0 {
			mOnlyRows = append(mOnlyRows, r)
		} else {
			mnRows = append(mnRows, r)
			epiHasBeta[r.epiIdx] = true
		}
	}
	var mnEpiBuf [8]int
	mnEpiIdxs := mnEpiBuf[:0]
	for ei := range numEpi {
		if epiHasBeta[ei] {
			mnEpiIdxs = append(mnEpiIdxs, ei)
		}
	}

	var baseEpiBuf, epiMaxBuf, bestEpiBuf, epiObjBuf, epiLBBuf [32]float64
	var baseEpi, epiMax, bestEpiVals, epiObj, epiLB []float64
	if numEpi <= len(baseEpiBuf) {
		baseEpi = baseEpiBuf[:numEpi]
		epiMax = epiMaxBuf[:numEpi]
		bestEpiVals = bestEpiBuf[:numEpi]
		epiObj = epiObjBuf[:numEpi]
		epiLB = epiLBBuf[:numEpi]
	} else {
		baseEpi = make([]float64, numEpi)
		epiMax = make([]float64, numEpi)
		bestEpiVals = make([]float64, numEpi)
		epiObj = make([]float64, numEpi)
		epiLB = make([]float64, numEpi)
	}
	for ei, ec := range comp.epiCols {
		epiObj[ei] = p.objMin[ec]
		epiLB[ei] = hlb[ec]
	}

	if len(mnRows) == 0 && len(nCands) > 1 {
		bestNV := nCands[0]
		bestNC := objN * bestNV
		for _, nv := range nCands[1:] {
			if c := objN * nv; c < bestNC {
				bestNC = c
				bestNV = nv
			}
		}
		nCands = []float64{bestNV}
	}

	bestCompObj := math.Inf(1)
	bestM := mCands[0]
	bestN := nCands[0]
	screenFactor := float64(numEpi+4) * 1e-13

	for _, mv := range mCands {
		copy(baseEpi, epiLB)
		for _, r := range mOnlyRows {
			if val := r.c + r.alpha*mv; val > baseEpi[r.epiIdx] {
				baseEpi[r.epiIdx] = val
			}
		}
		baseSum := objM * mv
		baseAbs := math.Abs(baseSum)
		for ei := range numEpi {
			if !epiHasBeta[ei] {
				term := epiObj[ei] * baseEpi[ei]
				baseSum += term
				baseAbs += math.Abs(term)
			}
		}

		for _, nv := range nCands {
			for _, ei := range mnEpiIdxs {
				epiMax[ei] = baseEpi[ei]
			}
			for _, r := range mnRows {
				if val := r.c + r.alpha*mv + r.beta*nv; val > epiMax[r.epiIdx] {
					epiMax[r.epiIdx] = val
				}
			}
			nTerm := objN * nv
			candObj := baseSum + nTerm
			candAbs := baseAbs + math.Abs(nTerm)
			for _, ei := range mnEpiIdxs {
				term := epiObj[ei] * epiMax[ei]
				candObj += term
				candAbs += math.Abs(term)
			}
			errEst := screenFactor * max(1.0, candAbs)
			if !isNonFinite(candObj) && !isNonFinite(candAbs) && candObj-errEst >= bestCompObj {
				continue
			}

			copy(epiMax, baseEpi)
			for _, r := range mnRows {
				if val := r.c + r.alpha*mv + r.beta*nv; val > epiMax[r.epiIdx] {
					epiMax[r.epiIdx] = val
				}
			}
			var acc dot2Acc
			acc.addProd(objM, mv)
			if nCol != -1 {
				acc.addProd(objN, nv)
			}
			for ei := range numEpi {
				acc.addProd(epiObj[ei], epiMax[ei])
			}
			obj, eb, ok := acc.result()
			if !ok || eb > 0x1p-48*math.Abs(obj) {
				obj = evalEpigraphCompObjWide(objM, mv, objN, nv, nCol, epiObj, epiMax[:numEpi])
			}
			if !math.IsNaN(obj) && !math.IsInf(obj, 0) && obj < bestCompObj {
				bestCompObj = obj
				bestM = mv
				bestN = nv
				copy(bestEpiVals, epiMax)
			}
		}
	}

	cand[mCol] = bestM
	if nCol != -1 {
		cand[nCol] = bestN
	}
	for ei, ec := range comp.epiCols {
		cand[ec] = bestEpiVals[ei]
	}
}

// trySingleRowContinuousCompletion solves separable single-row bounded continuous knapsacks in closed form when every free column appears in at most one row.
func trySingleRowContinuousCompletion(p *CompiledProblem, hlb, hub []float64) ([]float64, float64, bool) {
	n := p.numVars
	cand := slices.Clone(hlb)
	allSingleRow := true
	for j := range n {
		if hlb[j] < hub[j] {
			if p.objMin[j] < 0 {
				cand[j] = hub[j]
			}
			if p.colPtr[j+1]-p.colPtr[j] > 1 {
				allSingleRow = false
			}
		}
	}
	if allSingleRow {
		sc := p.getScaledLP()
		type rowMove struct {
			col         int
			absA        float64
			dxSign      float64
			maxRoom     float64
			unitPenalty float64
		}
		var moves []rowMove
		for i := range p.numConstraints {
			s, e := p.rowPtr[i], p.rowPtr[i+1]
			rhs := p.rowRHS[i]
			delta, deltaOK := evalFixedRowResidual(p.colIdx[s:e], p.rowVal[s:e], cand, nil, rhs)
			if !deltaOK {
				return nil, 0, false
			}
			sense := p.rowSense[i]
			if (sense == LessEqual && delta >= 0) ||
				(sense == GreaterEqual && delta <= 0) ||
				(sense == Equal && delta == 0) {
				continue
			}
			moves = moves[:0]
			for k := s; k < e; k++ {
				col := p.colIdx[k]
				if hlb[col] >= hub[col] {
					continue
				}
				a := p.rowVal[k]
				var room, dxSign float64
				if (delta > 0 && a > 0) || (delta < 0 && a < 0) {
					room = hub[col] - cand[col]
					dxSign = 1.0
				} else {
					room = cand[col] - hlb[col]
					dxSign = -1.0
				}
				if room > heurCoeffZeroTol {
					absA := math.Abs(a)
					pen := max(0.0, p.objMin[col]*dxSign) / absA
					moves = append(moves, rowMove{
						col:         col,
						absA:        absA,
						dxSign:      dxSign,
						maxRoom:     room,
						unitPenalty: pen,
					})
				}
			}
			slices.SortFunc(moves, func(a, b rowMove) int {
				return cmp.Compare(a.unitPenalty, b.unitPenalty)
			})
			rem := math.Abs(delta)
			for _, mv := range moves {
				if rem <= 0 {
					break
				}
				take := min(mv.maxRoom, rem/mv.absA)
				cand[mv.col] += mv.dxSign * take
				rem -= take * mv.absA
			}
			if rem > incumbentRowTol(p, sc, i, primalTol) {
				return nil, 0, false
			}
		}
		if fixedContFeasibleScaled(p, cand, hlb, hub) {
			obj := minObjective(p, cand)
			if math.IsNaN(obj) || math.IsInf(obj, 0) {
				return nil, 0, false
			}
			return cand, obj, true
		}
		return nil, 0, false
	}
	// When multi-row continuous variables exist, bypass the LP solve only if the
	// unconstrained corner satisfies every bound and row with zero residual.
	if incumbentFeasibleScaled(p, cand, 0) {
		obj := minObjective(p, cand)
		if math.IsNaN(obj) || math.IsInf(obj, 0) {
			return nil, 0, false
		}
		return cand, obj, true
	}
	return nil, 0, false
}

// solveFixedIntContinuousLP optimizes the continuous (and continuous-linked integer) variables
// after fixing primary integer decisions, trying closed-form epigraph and single-row solvers before LP fallback.
func solveFixedIntContinuousLP(
	p *CompiledProblem,
	hlb, hub []float64,
	auxSolver *LPSolver,
	isContLinked []bool,
) (sol []float64, objMin float64, ok bool) {
	if !propagateBounds(p, hlb, hub, nodePropPasses) {
		return nil, 0, false
	}
	if eSol, eObj, eOk := solveEpigraphContinuousLP(p, hlb, hub, isContLinked); eOk {
		return eSol, eObj, true
	}
	n := p.numVars
	nCont := n - len(p.intCols)
	hasUnfixedInt := false
	for _, col := range p.intCols {
		if hlb[col] < hub[col] {
			hasUnfixedInt = true
			break
		}
	}
	if !hasUnfixedInt {
		if cSol, cObj, cOk := trySingleRowContinuousCompletion(p, hlb, hub); cOk {
			return cSol, cObj, true
		}
	}
	if !hasUnfixedInt && auxSolver != nil && n <= heurCompactLPMaxVars {
		var savedLB, savedUB []float64
		if auxSolver.scaled {
			savedLB = slices.Clone(auxSolver.origLB)
			savedUB = slices.Clone(auxSolver.origUB)
		} else {
			savedLB = slices.Clone(auxSolver.ws.lb[:n])
			savedUB = slices.Clone(auxSolver.ws.ub[:n])
		}
		auxSolver.SetStructuralBounds(hlb, hub)
		res := auxSolver.Solve()
		auxSolver.SetStructuralBounds(savedLB, savedUB)
		if res.Status == StatusOptimal {
			if fixedContFeasibleScaled(p, res.Primal, hlb, hub) {
				obj := minObjective(p, res.Primal)
				if !math.IsNaN(obj) && !math.IsInf(obj, 0) {
					return slices.Clone(res.Primal), obj, true
				}
			}
		}
		if res.Status == StatusInfeasible {
			return nil, 0, false
		}
	}

	if auxSolver == nil && n > heurNoAuxSkipVars {
		return nil, 0, false
	}

	subModel := NewModel("__cont_lp__", Minimize)
	subVarMap := make([]Var, n)
	isFree := make([]bool, n)
	freeCols := make([]int, 0, nCont)
	var subIntIdxs []int
	var objTerms []Term

	for j := range n {
		if hlb[j] == hub[j] {
			continue
		}
		isFree[j] = true
		subIdx := len(freeCols)
		v := subModel.AddVar(fmt.Sprintf("c_%d", j), hlb[j], hub[j], Continuous)
		subVarMap[j] = v
		freeCols = append(freeCols, j)
		if p.varCat[j] != Continuous {
			subIntIdxs = append(subIntIdxs, subIdx)
		}
		if p.objMin[j] != 0 {
			objTerms = append(objTerms, T(v, p.objMin[j]))
		}
	}
	subModel.SetObjective(Expr{Terms: objTerms})

	sc := p.getScaledLP()
	for i := range p.numConstraints {
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		var terms []Term
		for k := s; k < e; k++ {
			col := p.colIdx[k]
			if isFree[col] {
				terms = append(terms, T(subVarMap[col], p.rowVal[k]))
			}
		}
		remRHS, remOK := evalFixedRowResidual(p.colIdx[s:e], p.rowVal[s:e], hlb, isFree, p.rowRHS[i])
		if !remOK {
			return nil, 0, false
		}
		sense := p.rowSense[i]
		if len(terms) == 0 {
			rowTol := incumbentRowTol(p, sc, i, primalTol)
			switch sense {
			case LessEqual:
				if remRHS < -rowTol {
					return nil, 0, false
				}
			case GreaterEqual:
				if remRHS > rowTol {
					return nil, 0, false
				}
			case Equal:
				if math.Abs(remRHS) > rowTol {
					return nil, 0, false
				}
			}
			continue
		}
		subModel.AddConstraint(fmt.Sprintf("r_%d", i), Constraint{
			Expr:  Expr{Terms: terms},
			Sense: sense,
			RHS:   remRHS,
		})
	}

	subP := subModel.Compile()
	ws := AcquireWorkspace(subP.numVars, subP.numConstraints)
	defer ReleaseWorkspace(ws)
	lp := NewLPSolver(subP, ws)
	if auxSolver != nil {
		lp.Deadline = auxSolver.Deadline
		lp.Done = auxSolver.Done
		lp.DisableSIMD = auxSolver.DisableSIMD
	}
	res := lp.Solve()
	if res.Status != StatusOptimal || len(res.Primal) != subP.numVars {
		return nil, 0, false
	}

	var subLB, subUB []float64
	skippedIntegralFix := false
	if len(subIntIdxs) > 0 {
		subLB = slices.Clone(subP.varLB)
		subUB = slices.Clone(subP.varUB)
		var fracSubIdxs []int
		for _, idx := range subIntIdxs {
			v := min(max(res.Primal[idx], subLB[idx]), subUB[idx])
			rv := min(max(math.Round(v), subLB[idx]), subUB[idx])
			if math.Abs(v-rv) <= intFeasTol {
				subLB[idx] = rv
				subUB[idx] = rv
				skippedIntegralFix = true
			} else {
				fracSubIdxs = append(fracSubIdxs, idx)
			}
		}
		if len(fracSubIdxs) > 0 {
			// Apply all batched integral fixings once before rounding fractional variables.
			lp.SetStructuralBounds(subLB, subUB)
			skippedIntegralFix = false
			resLoPrimal := make([]float64, subP.numVars)
			for _, idx := range fracSubIdxs {
				v := min(max(res.Primal[idx], subLB[idx]), subUB[idx])
				rv := min(max(math.Round(v), subLB[idx]), subUB[idx])
				if math.Abs(v-rv) <= intFeasTol {
					subLB[idx] = rv
					subUB[idx] = rv
					skippedIntegralFix = true
					continue
				}
				lo := max(subLB[idx], math.Floor(v))
				hi := min(subUB[idx], math.Ceil(v))
				subLB[idx] = lo
				subUB[idx] = lo
				lp.SetStructuralBounds(subLB, subUB)
				resLo := lp.Solve()
				okLo := resLo.Status == StatusOptimal && len(resLo.Primal) == subP.numVars

				if hi > lo {
					if okLo {
						copy(resLoPrimal, resLo.Primal)
					}
					subLB[idx] = hi
					subUB[idx] = hi
					lp.SetStructuralBounds(subLB, subUB)
					resHi := lp.Solve()
					okHi := resHi.Status == StatusOptimal && len(resHi.Primal) == subP.numVars
					if okHi && (!okLo || lpMinWithConst(subP, resHi) < lpMinWithConst(subP, resLo)-heurDedupTol) {
						res = resHi
						skippedIntegralFix = false
						continue
					}
				}
				if !okLo {
					return nil, 0, false
				}
				subLB[idx] = lo
				subUB[idx] = lo
				if hi > lo {
					resLo.Primal = resLoPrimal
				}
				res = resLo
				skippedIntegralFix = false
			}
		}
	}

	cand := slices.Clone(hlb)
	for idx, col := range freeCols {
		cand[col] = res.Primal[idx]
	}
	for _, idx := range subIntIdxs {
		col := freeCols[idx]
		cand[col] = math.Round(cand[col])
	}
	feasCand := fixedContFeasibleScaled(p, cand, hlb, hub)
	if !feasCand && skippedIntegralFix {
		// Re-solve once with all batched integer fixings applied when rounding small
		// integral residuals violated a high-coefficient constraint.
		lp.SetStructuralBounds(subLB, subUB)
		res = lp.Solve()
		if res.Status != StatusOptimal || len(res.Primal) != subP.numVars {
			return nil, 0, false
		}
		copy(cand, hlb)
		for idx, col := range freeCols {
			cand[col] = res.Primal[idx]
		}
		for _, idx := range subIntIdxs {
			col := freeCols[idx]
			cand[col] = math.Round(cand[col])
		}
		feasCand = fixedContFeasibleScaled(p, cand, hlb, hub)
	}
	if feasCand {
		obj := minObjective(p, cand)
		if math.IsNaN(obj) || math.IsInf(obj, 0) {
			return nil, 0, false
		}
		return cand, obj, true
	}
	return nil, 0, false
}

// tryFractionalDiving performs up to maxDiveStepsCap rounds of fixing fractional equality-supported
// integer variables, propagating bounds, and warm-starting the LP relaxation.
func tryFractionalDiving(
	p *CompiledProblem,
	rootPrimal []float64,
	baseLB, baseUB []float64,
	diveSolver *LPSolver,
	bestObjMin float64,
	onlyEqSupport bool,
	isContLinked []bool,
) (sol []float64, objMin float64, ok bool) {
	return tryFractionalDivingWithShift(p, rootPrimal, baseLB, baseUB, diveSolver, bestObjMin, p.objConstMin(), onlyEqSupport, isContLinked)
}

func tryFractionalDivingWithShift(
	p *CompiledProblem,
	rootPrimal []float64,
	baseLB, baseUB []float64,
	diveSolver *LPSolver,
	bestObjMin, searchShift float64,
	onlyEqSupport bool,
	isContLinked []bool,
) (sol []float64, objMin float64, ok bool) {
	n := p.numVars
	isRootFrac := make([]bool, n)
	for _, col := range p.intCols {
		v := rootPrimal[col]
		f := v - math.Floor(v)
		if f > intFeasTol && f < 1.0-intFeasTol {
			isRootFrac[col] = true
		}
	}
	eqRowCount := make([]int, n)
	inRootSupport := make([]bool, n)
	hasEqSupport := false
	for i := range p.numConstraints {
		if p.rowSense[i] != Equal {
			continue
		}
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		if e-s < 2 || e-s > maxEqSupportRowNNZ {
			continue
		}
		hasFrac := false
		allInt := true
		for k := s; k < e; k++ {
			col := p.colIdx[k]
			if p.varCat[col] == Continuous {
				allInt = false
				break
			}
			if isRootFrac[col] {
				hasFrac = true
			}
		}
		if allInt {
			for k := s; k < e; k++ {
				col := p.colIdx[k]
				eqRowCount[col]++
				if hasFrac {
					inRootSupport[col] = true
					hasEqSupport = true
				}
			}
		}
	}
	if onlyEqSupport && !hasEqSupport {
		return nil, 0, false
	}

	dlb := slices.Clone(baseLB)
	dub := slices.Clone(baseUB)
	curX := slices.Clone(rootPrimal)
	rowQueue := make([]int, 0, p.numConstraints*rowQueueCapMult)
	inQueue := make([]bool, p.numConstraints)

	curObjMin := evalLinearSum(searchShift, p.objMin, curX, n)
	savedDLB := make([]float64, n)
	savedDUB := make([]float64, n)
	branch1DLB := make([]float64, n)
	branch1DUB := make([]float64, n)
	branch1X := make([]float64, n)

	if diveSolver != nil {
		prevFast := diveSolver.fastHeuristicPrimal
		diveSolver.fastHeuristicPrimal = true
		defer func() {
			diveSolver.fastHeuristicPrimal = prevFast
		}()
	}

	maxDiveSteps := min(maxDiveStepsCap, len(p.intCols))
	for range maxDiveSteps {
		if diveSolver != nil && diveSolver.timeExpired() {
			return nil, 0, false
		}
		bestCol := -1
		bestScore := -1.0
		bestVal := 0.0
		for _, col := range p.intCols {
			if (isContLinked != nil && isContLinked[col]) || !inRootSupport[col] || dlb[col] == dub[col] {
				continue
			}
			v := curX[col]
			f := v - math.Floor(v)
			if f > intFeasTol && f < 1.0-intFeasTol {
				score := float64(eqRowCount[col]) + f
				if score > bestScore {
					bestScore = score
					bestCol = col
					bestVal = v
				}
			}
		}
		if bestCol != -1 && inRootSupport[bestCol] {
			bestEq := eqRowCount[bestCol]
			for _, col := range p.intCols {
				if (isContLinked != nil && isContLinked[col]) || dlb[col] == dub[col] || eqRowCount[col] <= bestEq {
					continue
				}
				v := curX[col]
				f := v - math.Floor(v)
				if f <= intFeasTol || f >= 1.0-intFeasTol {
					iv := min(max(math.Round(v), dlb[col]), dub[col])
					dlb[col] = iv
					dub[col] = iv
				}
			}
		}
		if bestCol == -1 {
			bestFrac := -1.0
			for _, col := range p.intCols {
				if (isContLinked != nil && isContLinked[col]) || dlb[col] == dub[col] {
					continue
				}
				v := curX[col]
				f := v - math.Floor(v)
				if f > intFeasTol && f < 1.0-intFeasTol {
					if f > bestFrac {
						bestFrac = f
						bestCol = col
						bestVal = v
					}
				} else {
					iv := math.Round(v)
					iv = min(max(iv, dlb[col]), dub[col])
					dlb[col] = iv
					dub[col] = iv
				}
			}
		}

		if bestCol == -1 {
			if isContLinked != nil {
				fixLB := slices.Clone(baseLB)
				fixUB := slices.Clone(baseUB)
				for _, col := range p.intCols {
					if isContLinked[col] {
						continue
					}
					iv := min(max(math.Round(curX[col]), baseLB[col]), baseUB[col])
					fixLB[col] = iv
					fixUB[col] = iv
				}
				if lpSol, _, okLP := solveFixedIntContinuousLP(p, fixLB, fixUB, diveSolver, isContLinked); okLP {
					if lpObj := evalLinearSum(searchShift, p.objMin, lpSol, n); !math.IsNaN(lpObj) && !math.IsInf(lpObj, 0) && lpObj < bestObjMin {
						return lpSol, lpObj, true
					}
				}
			}
			for _, col := range p.intCols {
				curX[col] = math.Round(curX[col])
			}
			if incumbentFeasibleScaled(p, curX, primalTol) {
				obj := evalLinearSum(searchShift, p.objMin, curX, n)
				if !math.IsNaN(obj) && !math.IsInf(obj, 0) && obj < bestObjMin {
					return curX, obj, true
				}
			}
			return nil, 0, false
		}

		copy(savedDLB, dlb)
		copy(savedDUB, dub)
		v := min(max(bestVal, dlb[bestCol]), dub[bestCol])
		target := min(max(math.Ceil(v), dlb[bestCol]), dub[bestCol])
		alt := min(max(math.Floor(v), dlb[bestCol]), dub[bestCol])

		dlb[bestCol] = target
		dub[bestCol] = target
		var res1 LPResult
		var m1 float64
		ok1 := propagateFromCol(p, dlb, dub, bestCol, &rowQueue, inQueue)
		if ok1 {
			diveSolver.SetStructuralBounds(dlb, dub)
			res1 = diveSolver.Solve()
			m1 = lpMinWithShift(p, res1, searchShift)
			ok1 = res1.Status == StatusOptimal && !math.IsNaN(m1) && !math.IsInf(m1, 0) && m1 < bestObjMin-heurIntBoundTol
		}

		if ok1 && m1 <= curObjMin+incumbentTol {
			copy(curX, res1.Primal)
			curObjMin = m1
			continue
		}

		if alt == target {
			if !ok1 {
				return nil, 0, false
			}
			copy(curX, res1.Primal)
			curObjMin = m1
			continue
		}

		copy(branch1DLB, dlb)
		copy(branch1DUB, dub)
		if ok1 {
			copy(branch1X, res1.Primal)
		}
		copy(dlb, savedDLB)
		copy(dub, savedDUB)
		dlb[bestCol] = alt
		dub[bestCol] = alt
		var res2 LPResult
		var m2 float64
		ok2 := propagateFromCol(p, dlb, dub, bestCol, &rowQueue, inQueue)
		if ok2 {
			diveSolver.SetStructuralBounds(dlb, dub)
			res2 = diveSolver.Solve()
			m2 = lpMinWithShift(p, res2, searchShift)
			ok2 = res2.Status == StatusOptimal && !math.IsNaN(m2) && !math.IsInf(m2, 0) && m2 < bestObjMin-heurIntBoundTol
		}

		if ok2 && (!ok1 || m2 <= m1+intFeasTol) {
			copy(curX, res2.Primal)
			curObjMin = m2
		} else if ok1 {
			copy(dlb, branch1DLB)
			copy(dub, branch1DUB)
			copy(curX, branch1X)
			curObjMin = m1
		} else {
			return nil, 0, false
		}
	}
	return nil, 0, false
}

// tryRootSubMIP fixes integer variables that are already integral at the root LP
// relaxation and solves the compact reduced sub-MIP over the remaining fractional
// integer variables and continuous variables, returning any explored sub-MIP nodes
// and simplex pivots so the parent search accounts for shared budgets.
func tryRootSubMIP(
	p *CompiledProblem,
	lpSolver *LPSolver,
	rootStatus []varBasisStatus,
	lpPrimal []float64,
	curLB, curUB []float64,
	bestObjMin, searchShift float64,
	deadline time.Time,
	isSubMIP bool,
	maxNodesCap int,
	maxIterCap int,
	isContLinked []bool,
	enableCuts bool,
) (sol []float64, objMin float64, nodesUsed, itersUsed int, ok bool) {
	if isSubMIP || len(p.intCols) == 0 || maxNodesCap <= 0 || maxIterCap <= 0 {
		return nil, 0, 0, 0, false
	}
	var done <-chan struct{}
	if lpSolver != nil {
		done = lpSolver.Done
	}
	if stopRequested(deadline, done) {
		return nil, 0, 0, 0, false
	}

	n := p.numVars
	hlb := slices.Clone(curLB)
	hub := slices.Clone(curUB)

	isFrac := make([]bool, n)
	baseFrac := 0
	for _, col := range p.intCols {
		v := lpPrimal[col]
		rv := math.Round(v)
		if math.Abs(v-rv) > intFeasTol {
			isFrac[col] = true
			baseFrac++
		}
	}
	if baseFrac == 0 || baseFrac > heurSubMIPBaseFracCap {
		return nil, 0, 0, 0, false
	}

	var status []varBasisStatus
	if len(rootStatus) >= n {
		status = rootStatus
	} else if lpSolver != nil && lpSolver.ws != nil && len(lpSolver.ws.status) >= n {
		status = lpSolver.ws.status
	}

	hasEqRows := false
	for i := range p.numConstraints {
		if p.rowSense[i] != Equal {
			continue
		}
		hasEqRows = true
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		if e-s > maxEqSupportRowNNZ {
			continue
		}
		rowHasFrac := false
		basicInRow := 0
		for k := s; k < e; k++ {
			col := p.colIdx[k]
			if isFrac[col] {
				rowHasFrac = true
			}
			if status != nil && status[col] == statusBasic {
				basicInRow++
			}
		}
		if rowHasFrac || (n <= heurSubMIPSmallModelVars && basicInRow >= 2) {
			for k := s; k < e; k++ {
				col := p.colIdx[k]
				if p.varCat[col] != Continuous {
					isFrac[col] = true
				}
			}
		}
	}

	if !hasEqRows && isContLinked == nil {
		return nil, 0, 0, 0, false
	}
	for _, col := range p.intCols {
		if isContLinked != nil && isContLinked[col] {
			isFrac[col] = true
		}
	}

	nFrac := 0
	for _, col := range p.intCols {
		if isFrac[col] {
			nFrac++
		} else {
			rv := math.Round(lpPrimal[col])
			iv := min(max(rv, hlb[col]), hub[col])
			hlb[col] = iv
			hub[col] = iv
		}
	}

	minFixed := max(2, len(p.intCols)/4)
	if !enableCuts {
		minFixed = max(minFixed, len(p.intCols)/2)
	}
	if nFrac == 0 || nFrac > heurSubMIPFreeFracCap || len(p.intCols)-nFrac < minFixed {
		return nil, 0, 0, 0, false
	}

	if !propagateBounds(p, hlb, hub, nodePropPasses) {
		return nil, 0, 0, 0, false
	}

	subModel := NewModel("submip", Minimize)
	subVarMap := make([]Var, n)
	isFree := make([]bool, n)
	freeCols := make([]int, 0, nFrac+(n-len(p.intCols)))

	var objTerms []Term
	var fixedObjAcc dot2Acc
	fixedObjAcc.addVal(searchShift)
	for j := range n {
		if hlb[j] == hub[j] {
			fixedObjAcc.addProd(p.objMin[j], hlb[j])
			continue
		}
		isFree[j] = true
		v := subModel.AddVar(fmt.Sprintf("v_%d", j), hlb[j], hub[j], p.varCat[j])
		subVarMap[j] = v
		freeCols = append(freeCols, j)
		if p.objMin[j] != 0 {
			objTerms = append(objTerms, T(v, p.objMin[j]))
		}
	}

	if len(freeCols) == 0 {
		return nil, 0, 0, 0, false
	}

	var fixedObjMin float64
	if val, eb, fOK := fixedObjAcc.result(); fOK && eb <= 0x1p-20*max(1.0, math.Abs(val)) {
		fixedObjMin = val
	} else {
		var w wideDotAcc
		w.addVal(searchShift)
		for j := range n {
			if !isFree[j] {
				w.addProd(p.objMin[j], hlb[j])
			}
		}
		fixedObjMin = w.finish(math.NaN())
	}
	if math.IsNaN(fixedObjMin) || math.IsInf(fixedObjMin, 0) {
		return nil, 0, 0, 0, false
	}

	subModel.SetObjective(Expr{Terms: objTerms})

	for i := range p.numConstraints {
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		var terms []Term
		for k := s; k < e; k++ {
			col := p.colIdx[k]
			if isFree[col] {
				terms = append(terms, T(subVarMap[col], p.rowVal[k]))
			}
		}
		rhs, rhsOK := evalFixedRowResidual(p.colIdx[s:e], p.rowVal[s:e], hlb, isFree, p.rowRHS[i])
		if !rhsOK {
			return nil, 0, 0, 0, false
		}
		sense := p.rowSense[i]
		if len(terms) == 0 {
			switch sense {
			case LessEqual:
				if rhs < -intFeasTol {
					return nil, 0, 0, 0, false
				}
			case GreaterEqual:
				if rhs > intFeasTol {
					return nil, 0, 0, 0, false
				}
			case Equal:
				if math.Abs(rhs) > intFeasTol {
					return nil, 0, 0, 0, false
				}
			}
			continue
		}
		subModel.AddConstraint(fmt.Sprintf("c_%d", i), Constraint{
			Expr:  Expr{Terms: terms},
			Sense: sense,
			RHS:   rhs,
		})
	}

	subP := subModel.Compile()
	if pres := presolveWithDeadline(subP, subMIPPresolvePasses, deadline, done); pres.TimedOut || pres.Infeasible {
		return nil, 0, 0, 0, false
	} else if pres.Reduced != nil && pres.Reduced.invalidErr == nil {
		subP = pres.Reduced
	}

	subOpts := DefaultOptions()
	subOpts.isSubMIP = true
	subOpts.MaxNodes = min(heurSubMIPMaxNodes, maxNodesCap)
	if maxIterCap < math.MaxInt {
		subOpts.MaxIter = maxIterCap
	}
	subOpts.TimeLimit = subMIPTimeLimitMillis * time.Millisecond
	if !deadline.IsZero() {
		rem := time.Until(deadline)
		if rem <= 0 {
			return nil, 0, 0, 0, false
		}
		subOpts.TimeLimit = min(subOpts.TimeLimit, rem)
	}
	subOpts.EnablePresolve = true
	subOpts.EnableCuts = enableCuts
	if enableCuts {
		subOpts.MaxCutPasses = heurSubMIPCutPasses
	} else {
		subOpts.MaxCutPasses = 0
	}
	subOpts.EnableHeuristics = true
	subOpts.Workers = 1
	if lpSolver != nil {
		subOpts.DisableSIMD = lpSolver.DisableSIMD
	}
	if !math.IsInf(bestObjMin, 1) {
		if cutFree := bestObjMin - fixedObjMin; !math.IsNaN(cutFree) && !math.IsInf(cutFree, 0) {
			subOpts.hasCutoff = true
			subOpts.cutoffMin = cutFree
		}
	}

	subRes := solveBranchAndCutWithDone(subP, subOpts, time.Now(), deadline, done)
	usedN, usedIt := subRes.Nodes, subRes.Iterations
	if len(subRes.Values) != subP.numVars {
		return nil, 0, usedN, usedIt, false
	}

	cand := slices.Clone(hlb)
	for idx, origCol := range freeCols {
		cand[origCol] = subRes.Values[idx]
	}
	for _, col := range p.intCols {
		cand[col] = math.Round(cand[col])
	}

	if !incumbentFeasibleScaled(p, cand, primalTol) {
		fixLB := slices.Clone(curLB)
		fixUB := slices.Clone(curUB)
		for _, col := range p.intCols {
			fixLB[col] = cand[col]
			fixUB[col] = cand[col]
		}
		if lpSol, _, okLP := solveFixedIntContinuousLP(p, fixLB, fixUB, lpSolver, isContLinked); okLP {
			if lpObj := evalLinearSum(searchShift, p.objMin, lpSol, n); !math.IsNaN(lpObj) && !math.IsInf(lpObj, 0) && lpObj < bestObjMin-heurIntBoundTol {
				return lpSol, lpObj, usedN, usedIt, true
			}
		}
		return nil, 0, usedN, usedIt, false
	}

	obj := evalLinearSum(searchShift, p.objMin, cand, n)
	if math.IsNaN(obj) || math.IsInf(obj, 0) || obj >= bestObjMin-heurIntBoundTol {
		return nil, 0, usedN, usedIt, false
	}
	return cand, obj, usedN, usedIt, true
}

func fpHashPrimaryInts(p *CompiledProblem, x []float64, isContLinked []bool) uint64 {
	var h uint64 = fpFnvOffset64
	for _, col := range p.intCols {
		if isContLinked != nil && isContLinked[col] {
			continue
		}
		bits := math.Float64bits(x[col] + 0.0)
		h ^= bits
		h *= fpFnvPrime64
		h ^= bits >> 32
		h *= fpFnvPrime64
	}
	return h
}

func fpHasHash(hashes []uint64, target uint64) bool {
	return slices.Contains(hashes, target)
}

// fpRoundCandidate propagates nearest-integer fixings for primary integer columns,
// falling back to direct clamped rounding on remaining columns if propagation stalls.
func fpRoundCandidate(
	p *CompiledProblem,
	xLP, curLB, curUB, xInt, rlb, rub, snapLB, snapUB []float64,
	rowQueue *[]int,
	inQueue []bool,
	isContLinked []bool,
) (nFrac int, propagatedAll bool) {
	type fpFracCand struct {
		col  int
		frac float64
	}
	copy(xInt, xLP)
	copy(rlb, curLB)
	copy(rub, curUB)
	for j := range p.numVars {
		if xInt[j] < curLB[j] {
			xInt[j] = curLB[j]
		} else if xInt[j] > curUB[j] {
			xInt[j] = curUB[j]
		}
	}

	var cands [64]fpFracCand
	fracs := cands[:0]
	for _, col := range p.intCols {
		if isContLinked != nil && isContLinked[col] {
			continue
		}
		v := min(max(xLP[col], rlb[col]), rub[col])
		rv := min(max(math.Round(v), rlb[col]), rub[col])
		f := math.Abs(v - rv)
		if f <= intFeasTol {
			rlb[col] = rv
			rub[col] = rv
			xInt[col] = rv
		} else {
			fracs = append(fracs, fpFracCand{col: col, frac: f})
		}
	}
	nFrac = len(fracs)
	if nFrac > 1 {
		slices.SortFunc(fracs, func(a, b fpFracCand) int {
			if c := cmp.Compare(a.frac, b.frac); c != 0 {
				return c
			}
			return cmp.Compare(a.col, b.col)
		})
	}
	propagatedAll = true
	for idx, fc := range fracs {
		col := fc.col
		if rlb[col] == rub[col] {
			xInt[col] = rlb[col]
			continue
		}
		v := min(max(xLP[col], rlb[col]), rub[col])
		target := min(max(math.Round(v), rlb[col]), rub[col])
		copy(snapLB, rlb)
		copy(snapUB, rub)
		rlb[col] = target
		rub[col] = target
		if propagateFromCol(p, rlb, rub, col, rowQueue, inQueue) {
			xInt[col] = target
			continue
		}
		copy(rlb, snapLB)
		copy(rub, snapUB)
		alt := math.Floor(v)
		if target == alt {
			alt = math.Ceil(v)
		}
		alt = min(max(alt, rlb[col]), rub[col])
		rlb[col] = alt
		rub[col] = alt
		if propagateFromCol(p, rlb, rub, col, rowQueue, inQueue) {
			xInt[col] = alt
			continue
		}
		propagatedAll = false
		for j := idx; j < len(fracs); j++ {
			c := fracs[j].col
			vj := min(max(xLP[c], curLB[c]), curUB[c])
			xInt[c] = min(max(math.Round(vj), curLB[c]), curUB[c])
		}
		break
	}
	if propagatedAll {
		for _, col := range p.intCols {
			if isContLinked != nil && isContLinked[col] {
				continue
			}
			if rlb[col] == rub[col] {
				xInt[col] = rlb[col]
			}
		}
	}
	return nFrac, propagatedAll
}

func fpPerturbOnCycle(
	p *CompiledProblem,
	xLP, curLB, curUB, xInt []float64,
	pass int,
	seenHashes []uint64,
	isContLinked []bool,
) uint64 {
	type flipCand struct {
		col  int
		dist float64
	}
	var fracCands, unfixedCands []flipCand
	for _, col := range p.intCols {
		if isContLinked != nil && isContLinked[col] {
			continue
		}
		if curUB[col]-curLB[col] < 0.5 {
			continue
		}
		v := min(max(xLP[col], curLB[col]), curUB[col])
		d := math.Abs(v - xInt[col])
		unfixedCands = append(unfixedCands, flipCand{col: col, dist: d})
		if math.Abs(v-math.Round(v)) > intFeasTol {
			fracCands = append(fracCands, flipCand{col: col, dist: d})
		}
	}
	flipToAlt := func(col int) {
		l, u := curLB[col], curUB[col]
		v := min(max(xLP[col], l), u)
		cur := xInt[col]
		fl := min(max(math.Floor(v), l), u)
		cl := min(max(math.Ceil(v), l), u)
		switch {
		case fl != cur:
			xInt[col] = fl
		case cl != cur:
			xInt[col] = cl
		case cur+1.0 <= u+intFeasTol && !math.IsInf(u, 1):
			xInt[col] = cur + 1.0
		case cur-1.0 >= l-intFeasTol && !math.IsInf(l, -1):
			xInt[col] = cur - 1.0
		}
	}

	if len(fracCands) > 0 {
		slices.SortFunc(fracCands, func(a, b flipCand) int {
			if c := cmp.Compare(b.dist, a.dist); c != 0 {
				return c
			}
			return cmp.Compare(a.col, b.col)
		})
		flipCnt := min(max(2, len(fracCands)/2), 10)
		start := pass % len(fracCands)
		for k := range flipCnt {
			flipToAlt(fracCands[(start+k)%len(fracCands)].col)
		}
		h := fpHashPrimaryInts(p, xInt, isContLinked)
		if !fpHasHash(seenHashes, h) {
			return h
		}
	}

	if len(unfixedCands) > 0 {
		pertCnt := min(max(3, len(unfixedCands)/4), 12)
		rng := (uint64(max(0, pass)) + 1) * fpSplitmixSeedMult
		for range pertCnt {
			rng ^= rng >> 30
			rng *= 0xbf58476d1ce4e5b9
			rng ^= rng >> 27
			rng *= 0x94d049bb133111eb
			rng ^= rng >> 31
			idx := int(uint32(rng>>33)) % len(unfixedCands)
			col := unfixedCands[idx].col
			l, u := curLB[col], curUB[col]
			if l == 0 && u == 1 {
				xInt[col] = 1.0 - xInt[col]
			} else {
				flipToAlt(col)
			}
		}
	}
	return fpHashPrimaryInts(p, xInt, isContLinked)
}

func fpEvaluateCandidate(
	p *CompiledProblem,
	xCand, curLB, curUB, rlb, rub []float64,
	propagatedOK bool,
	isContLinked []bool,
) (sol []float64, objMin float64, ok bool) {
	if len(p.intCols) == p.numVars {
		if !incumbentFeasibleScaled(p, xCand, primalTol) {
			return nil, 0, false
		}
		obj := minObjective(p, xCand)
		if math.IsNaN(obj) || math.IsInf(obj, 0) {
			return nil, 0, false
		}
		return slices.Clone(xCand), obj, true
	}
	if !propagatedOK {
		copy(rlb, curLB)
		copy(rub, curUB)
		for _, col := range p.intCols {
			if isContLinked != nil && isContLinked[col] {
				continue
			}
			v := min(max(xCand[col], rlb[col]), rub[col])
			rlb[col] = v
			rub[col] = v
		}
	}
	return solveFixedIntContinuousLP(p, rlb, rub, nil, isContLinked)
}

// tryFeasibilityPump runs a budgeted Objective Feasibility Pump (OFP) alternating between
// propagated rounding and warm Phase II primal simplex projections over primary integers.
func tryFeasibilityPump(
	p *CompiledProblem,
	lpPrimal, curLB, curUB []float64,
	heurSolver *LPSolver,
	bestObjMin, lowerBoundMin, relGapTol, absGapTol float64,
	deadline time.Time,
	done <-chan struct{},
	maxIterCap int,
	isContLinked []bool,
) (sol []float64, objMin float64, itersUsed int, ok bool) {
	return tryFeasibilityPumpWithShift(
		p, lpPrimal, curLB, curUB, heurSolver,
		bestObjMin, lowerBoundMin, p.objConstMin(), relGapTol, absGapTol,
		deadline, done, maxIterCap, isContLinked,
	)
}

func tryFeasibilityPumpWithShift(
	p *CompiledProblem,
	lpPrimal, curLB, curUB []float64,
	heurSolver *LPSolver,
	bestObjMin, lowerBoundMin, searchShift, relGapTol, absGapTol float64,
	deadline time.Time,
	done <-chan struct{},
	maxIterCap int,
	isContLinked []bool,
) (sol []float64, objMin float64, itersUsed int, ok bool) {
	if p == nil || heurSolver == nil || len(p.intCols) < 2 || p.numVars > fpMaxVars || maxIterCap <= 0 {
		return nil, 0, 0, false
	}
	if len(lpPrimal) < p.numVars || stopRequested(deadline, done) {
		return nil, 0, 0, false
	}
	primaryFrac := 0
	for _, col := range p.intCols {
		if isContLinked != nil && isContLinked[col] {
			continue
		}
		v := lpPrimal[col]
		if math.Abs(v-math.Round(v)) > intFeasTol {
			primaryFrac++
		}
	}
	if primaryFrac == 0 {
		return nil, 0, 0, false
	}

	now := time.Now()
	fpDur := fpMaxTimeMillis * time.Millisecond
	if !deadline.IsZero() {
		rem := deadline.Sub(now)
		if rem <= 0 {
			return nil, 0, 0, false
		}
		if rem < fpMinSlackTimeMillis*time.Millisecond {
			fpDur = min(fpDur, rem/4)
		} else {
			fpDur = min(fpDur, rem)
		}
	}
	if fpDur <= 0 {
		return nil, 0, 0, false
	}
	fpDeadline := now.Add(fpDur)
	iterBudget := min(maxIterCap, min(fpMaxTotalIters, max(fpMinTotalIters, 4*p.numConstraints)))
	perPassIter := max(fpPerPassMinIter, min(fpPerPassMaxIter, 2*p.numConstraints))

	savedDeadline := heurSolver.Deadline
	heurSolver.Deadline = fpDeadline
	heurSolver.SetStructuralBounds(curLB, curUB)
	defer func() {
		heurSolver.Deadline = savedDeadline
		heurSolver.SetStructuralBounds(p.varLB, p.varUB)
	}()

	n := p.numVars
	cHat := make([]float64, n)
	cNormSq := 0.0
	for j := range n {
		c := p.objMin[j]
		cNormSq += c * c
	}
	alpha := 0.0
	if cNormSq > 0 {
		invCNorm := 1.0 / math.Sqrt(cNormSq)
		for j := range n {
			cHat[j] = p.objMin[j] * invCNorm
		}
		alpha = fpAlphaInit
	}

	xLP := slices.Clone(lpPrimal)
	xInt := make([]float64, n)
	rlb := make([]float64, n)
	rub := make([]float64, n)
	snapLB := make([]float64, n)
	snapUB := make([]float64, n)
	distVec := make([]float64, n)
	fpCost := make([]float64, n)
	rowQueue := make([]int, 0, p.numConstraints*rowQueueCapMult)
	inQueue := make([]bool, p.numConstraints)
	seenHashes := make([]uint64, 0, fpMaxPasses)
	evalHashes := make([]uint64, 0, fpMaxPasses)

	var bestSol []float64
	curBestObjMin := bestObjMin
	hasIncomingInc := !math.IsInf(bestObjMin, 1)
	foundCount := 0
	stallPasses := 0

	for pass := 0; pass < fpMaxPasses && itersUsed < iterBudget && !stopRequested(fpDeadline, done); pass++ {
		_, propOK := fpRoundCandidate(p, xLP, curLB, curUB, xInt, rlb, rub, snapLB, snapUB, &rowQueue, inQueue, isContLinked)
		h := fpHashPrimaryInts(p, xInt, isContLinked)
		cycled := fpHasHash(seenHashes, h)
		if cycled {
			h = fpPerturbOnCycle(p, xLP, curLB, curUB, xInt, pass, seenHashes, isContLinked)
			propOK = false
		}
		seenHashes = append(seenHashes, h)

		improvedThisPass := false
		if !fpHasHash(evalHashes, h) {
			evalHashes = append(evalHashes, h)
			if cand, _, cOK := fpEvaluateCandidate(p, xInt, curLB, curUB, rlb, rub, propOK, isContLinked); cOK {
				if cObj := evalLinearSum(searchShift, p.objMin, cand, n); !math.IsNaN(cObj) && !math.IsInf(cObj, 0) && cObj < curBestObjMin-heurIntBoundTol {
					bestSol = cand
					curBestObjMin = cObj
					foundCount++
					improvedThisPass = true
					if isGapClosed(lowerBoundMin, curBestObjMin, searchShift, relGapTol, absGapTol) {
						return bestSol, curBestObjMin, itersUsed, true
					}
				}
			}
		}
		if improvedThisPass {
			stallPasses = 0
		} else if pass > 0 && hasIncomingInc {
			stallPasses++
			if stallPasses >= fpMaxStallPassesWithInc && bestSol == nil {
				break
			}
		}

		clear(distVec)
		dNormSq := 0.0
		for _, col := range p.intCols {
			if isContLinked != nil && isContLinked[col] {
				continue
			}
			l, u := curLB[col], curUB[col]
			if u-l < 0.5 {
				continue
			}
			if !math.IsInf(l, -1) && xInt[col] <= l+0.5 {
				distVec[col] = 1.0
				dNormSq += 1.0
			} else if !math.IsInf(u, 1) && xInt[col] >= u-0.5 {
				distVec[col] = -1.0
				dNormSq += 1.0
			}
		}
		if dNormSq == 0 {
			break
		}
		invDNorm := 1.0 / math.Sqrt(dNormSq)
		for j := range n {
			fpCost[j] = (1.0-alpha)*distVec[j]*invDNorm + alpha*cHat[j]
		}

		stepCap := min(perPassIter, iterBudget-itersUsed)
		if stepCap <= 0 {
			break
		}
		projRes, used := heurSolver.solvePrimalObjective(fpCost, stepCap)
		itersUsed += used
		if projRes.Status != StatusOptimal || len(projRes.Primal) < n {
			break
		}
		copy(xLP, projRes.Primal)

		if isIntegerFeasible(p, xLP) {
			cand := slices.Clone(xLP)
			snapToIntegers(p, cand)
			if len(p.intCols) < n {
				copy(rlb, curLB)
				copy(rub, curUB)
				for _, col := range p.intCols {
					if isContLinked != nil && isContLinked[col] {
						continue
					}
					v := min(max(cand[col], rlb[col]), rub[col])
					rlb[col] = v
					rub[col] = v
				}
				if pSol, _, pOK := solveFixedIntContinuousLP(p, rlb, rub, nil, isContLinked); pOK {
					if pObj := evalLinearSum(searchShift, p.objMin, pSol, n); !math.IsNaN(pObj) && !math.IsInf(pObj, 0) && pObj < curBestObjMin-heurIntBoundTol {
						bestSol = pSol
						curBestObjMin = pObj
						foundCount++
					}
				}
			}
			if incumbentFeasibleScaled(p, cand, primalTol) {
				if cObj := evalLinearSum(searchShift, p.objMin, cand, n); !math.IsNaN(cObj) && !math.IsInf(cObj, 0) && cObj < curBestObjMin-heurIntBoundTol {
					bestSol = cand
					curBestObjMin = cObj
					foundCount++
				}
			}
			if bestSol != nil && (isGapClosed(lowerBoundMin, curBestObjMin, searchShift, relGapTol, absGapTol) || foundCount >= fpFoundStopCap || alpha <= fpAlphaEarlyStop) {
				return bestSol, curBestObjMin, itersUsed, true
			}
		}

		alpha *= fpAlphaDecay
		if alpha < fpAlphaFloor {
			alpha = 0
		}
	}

	if bestSol != nil {
		return bestSol, curBestObjMin, itersUsed, true
	}
	return nil, 0, itersUsed, false
}
