//go:build cgo && cbc

package goblin

import (
	"context"
	"math"
	"slices"
	"time"
)

func hasAnyInfiniteBound(lb, ub []float64, n int) bool {
	for j := range min(n, min(len(lb), len(ub))) {
		if lb[j] < ub[j] && (math.IsInf(lb[j], -1) || math.IsInf(ub[j], 1)) {
			return true
		}
	}
	return false
}

func snapCBCBounds(primal, lb, ub []float64, n int) {
	for j := range n {
		snapTol := max(cbcBoundSnapTol, 1e-6*max(1.0, math.Abs(primal[j])))
		if !math.IsInf(lb[j], -1) && primal[j] < lb[j] && lb[j]-primal[j] <= snapTol {
			primal[j] = lb[j]
		} else if !math.IsInf(ub[j], 1) && primal[j] > ub[j] && primal[j]-ub[j] <= snapTol {
			primal[j] = ub[j]
		}
	}
}

func canRecedeNegLP(p *CompiledProblem, lb, ub []float64) (anyNeg, contNeg bool) {
	n := min(p.numVars, min(len(lb), len(ub)))
	for j := range n {
		if lb[j] >= ub[j] {
			continue
		}
		c := p.objMin[j]
		if (math.IsInf(ub[j], 1) && c < 0) || (math.IsInf(lb[j], -1) && c > 0) {
			anyNeg = true
			if p.varCat[j] == Continuous {
				contNeg = true
			}
		}
	}
	return anyNeg, contNeg
}

func hasExtremeCBCBound(colLB, colUB, rowLB, rowUB, cutRHS []float64) bool {
	for _, s := range [][]float64{colLB, colUB, rowLB, rowUB, cutRHS} {
		for _, v := range s {
			if !math.IsInf(v, 0) && math.Abs(v) >= 1e29 {
				return true
			}
		}
	}
	return false
}

// polishCBCMIPPrimal repairs small continuous-variable primal/row residuals left by CBC's
// internal tolerances (e.g. on wide dynamic-range MILPs) by fixing all integer variables to
// their rounded values and solving the resulting continuous LP in pure Go. It succeeds only
// when all integer variables are already within integrality tolerance, the polished LP is
// optimal and passes cbcPrimalWithinTol, and (for claimed StatusOptimal) its objective agrees
// with CBC's best dual bound within gap tolerance.
func polishCBCMIPPrimal(
	ctx context.Context,
	p *CompiledProblem,
	opt Options,
	baseIters int,
	claimedOptimal bool,
	hasBestBound bool,
	bestBoundMin float64,
	primal []float64,
	deadline time.Time,
	done <-chan struct{},
) ([]float64, int, bool) {
	n := p.numVars
	if len(primal) < n || len(p.intCols) >= n {
		return nil, 0, false
	}
	if stopRequested(deadline, done) || (ctx != nil && ctx.Err() != nil) {
		return nil, 0, false
	}
	lbFix := slices.Clone(p.varLB)
	ubFix := slices.Clone(p.varUB)
	for _, col := range p.intCols {
		v := primal[col]
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, 0, false
		}
		rv := math.Round(v)
		if math.Abs(v-rv) > 1e-3 {
			return nil, 0, false
		}
		if (!math.IsInf(p.varLB[col], -1) && rv < p.varLB[col]-1e-9) ||
			(!math.IsInf(p.varUB[col], 1) && rv > p.varUB[col]+1e-9) {
			return nil, 0, false
		}
		lbFix[col] = rv
		ubFix[col] = rv
	}

	resFix, canceled := runBudgetedGoLPProbe(ctx, p, opt, baseIters, lbFix, ubFix, deadline, done)
	extraIters := resFix.Iterations
	if canceled || resFix.Status != StatusOptimal || len(resFix.Primal) < n {
		return nil, extraIters, false
	}
	polished := slices.Clone(resFix.Primal[:n])
	snapCBCBounds(polished, p.varLB, p.varUB, n)
	if !cbcPrimalWithinTol(p, p.varLB, p.varUB, cbcCutSet{}, polished, true) {
		return nil, extraIters, false
	}
	polishedMin := evalLinearSum(0, p.objMin, polished, n)
	if math.IsNaN(polishedMin) || math.IsInf(polishedMin, 0) {
		return nil, extraIters, false
	}
	if claimedOptimal && hasBestBound && !math.IsNaN(bestBoundMin) && !math.IsInf(bestBoundMin, 0) {
		gapTol := max(1e-5, max(clampCBCGapTol(opt.AbsGapTol), clampCBCGapTol(opt.RelGapTol)*max(1.0, math.Abs(polishedMin))))
		if math.Abs(polishedMin-bestBoundMin) > gapTol {
			return nil, extraIters, false
		}
	}
	return polished, extraIters, true
}

// fallbackCBCMIPToGo delegates a CBC MIP solve to the compensated Go engine while preserving remaining iteration and node budgets.
func fallbackCBCMIPToGo(
	ctx context.Context,
	p *CompiledProblem,
	opt Options,
	start, deadline time.Time,
	done <-chan struct{},
	cNodes, cIters, cCuts int,
) Result {
	goOpt := opt
	if opt.defaultMaxNodesUsed {
		goOpt.MaxNodes = 0
	}
	if opt.defaultCutPassesUsed {
		goOpt.MaxCutPasses = defaultMaxCutPasses
	}
	var limitSt Status
	goOpt, limitSt = remainingCBCMIPBudget(goOpt, cNodes, cIters)
	if limitSt != StatusNotSolved {
		return cbcStatusResult(p, limitSt, start, cNodes, cIters, cCuts)
	}
	goRes := solveCompensatedEngine(ctx, p, goOpt, BackendCBC, start, deadline, done)
	if ctx != nil && ctx.Err() != nil {
		return Result{}
	}
	goRes.Nodes += cNodes
	goRes.Iterations += cIters
	goRes.CutsAdded += cCuts
	goRes.Backend = BackendCBC
	goRes.FellBackToGo = true
	return goRes
}

type cbcMIPVerifyReq struct {
	p            *CompiledProblem
	opt          Options
	baseIters    int
	hasRootLP    bool
	rootLPMin    float64
	hasBestBound bool
	bestBoundMin float64
	primal       []float64
	deadline     time.Time
	done         <-chan struct{}
}

func verifyCBCMIPOptimality(ctx context.Context, req *cbcMIPVerifyReq) (Status, int) {
	p := req.p
	opt := req.opt
	baseIters := req.baseIters
	hasRootLP, rootLPMin := req.hasRootLP, req.rootLPMin
	hasBestBound, bestBoundMin := req.hasBestBound, req.bestBoundMin
	primal := req.primal
	deadline, done := req.deadline, req.done
	n := p.numVars
	curMin := evalLinearSum(0, p.objMin, primal, n)
	if math.IsNaN(curMin) || math.IsInf(curMin, 0) {
		return StatusNumericalError, 0
	}
	gapTol := max(1e-6, max(clampCBCGapTol(opt.AbsGapTol), clampCBCGapTol(opt.RelGapTol)*max(1.0, math.Abs(curMin))))
	anyNeg, contNeg := canRecedeNegLP(p, p.varLB, p.varUB)
	hasInfCont := false
	hasInfInt := false
	hasWideCont := false
	hasExtremeIntObj := false
	hasTinyCoeffOrObj := false
	for j := range n {
		if p.varLB[j] >= p.varUB[j] {
			continue
		}
		if ao := math.Abs(p.objMin[j]); ao > 0 && ao <= 1e-5 {
			hasTinyCoeffOrObj = true
		}
		infBnd := math.IsInf(p.varLB[j], -1) || math.IsInf(p.varUB[j], 1)
		if p.varCat[j] == Continuous {
			if infBnd {
				hasInfCont = true
			} else {
				span := p.varUB[j] - p.varLB[j]
				if 1e-5*span > gapTol || (math.Abs(p.objMin[j]) >= 1e7 && math.Abs(p.objMin[j])*span > gapTol) {
					hasWideCont = true
				}
			}
		} else if infBnd {
			hasInfInt = true
		} else {
			span := p.varUB[j] - p.varLB[j]
			if math.Abs(p.objMin[j]) >= 1e7 && math.Abs(p.objMin[j])*span > gapTol {
				hasExtremeIntObj = true
			}
		}
	}
	if (hasInfCont || hasInfInt || hasExtremeIntObj) && !hasTinyCoeffOrObj {
		for _, v := range p.colVal {
			if av := math.Abs(v); av > 0 && av <= 1e-7 {
				hasTinyCoeffOrObj = true
				break
			}
		}
	}
	needFixedLP := contNeg || hasWideCont || (hasInfCont && hasTinyCoeffOrObj)
	needRelaxLP := (hasInfInt && (anyNeg || hasTinyCoeffOrObj)) || (hasExtremeIntObj && hasTinyCoeffOrObj)
	if !needFixedLP && !needRelaxLP {
		return StatusOptimal, 0
	}

	extraIters := 0
	if needFixedLP {
		lbFix := slices.Clone(p.varLB)
		ubFix := slices.Clone(p.varUB)
		for _, col := range p.intCols {
			v := math.Round(primal[col])
			lbFix[col] = v
			ubFix[col] = v
		}
		resFix, canceled := runBudgetedGoLPProbe(ctx, p, opt, baseIters+extraIters, lbFix, ubFix, deadline, done)
		extraIters += resFix.Iterations
		if canceled {
			return StatusTimeLimit, extraIters
		}
		switch resFix.Status {
		case StatusUnbounded, StatusTimeLimit, StatusIterationLimit:
			return resFix.Status, extraIters
		case StatusOptimal:
			if resFix.ObjectiveMin < curMin-gapTol {
				if len(resFix.Primal) == n &&
					(!hasBestBound || resFix.ObjectiveMin >= bestBoundMin-gapTol) &&
					cbcPrimalWithinTol(p, p.varLB, p.varUB, cbcCutSet{}, resFix.Primal, true) {
					copy(primal, resFix.Primal)
					curMin = evalLinearSum(0, p.objMin, primal, n)
				} else {
					return StatusNumericalError, extraIters
				}
			}
		default:
			return StatusNumericalError, extraIters
		}
	}

	if needRelaxLP {
		resRelax, canceled := runBudgetedGoLPProbe(ctx, p, opt, baseIters+extraIters, nil, nil, deadline, done)
		extraIters += resRelax.Iterations
		if canceled {
			return StatusTimeLimit, extraIters
		}
		switch resRelax.Status {
		case StatusUnbounded, StatusTimeLimit, StatusIterationLimit, StatusNumericalError:
			return resRelax.Status, extraIters
		case StatusOptimal:
			if resRelax.ObjectiveMin > curMin+gapTol {
				return StatusNumericalError, extraIters
			}
			if !opt.EnablePresolve && hasRootLP && resRelax.ObjectiveMin < rootLPMin-gapTol {
				return StatusNumericalError, extraIters
			}
		}
	}
	return StatusOptimal, extraIters
}

func cbcRowRepairMaxCoeffAtMost1(cols []int, vals, primal, effLB, effUB []float64, decrease bool) float64 {
	dirMax := 0.0
	domMax := 0.0
	n := min(len(primal), min(len(effLB), len(effUB)))
	for k := range min(len(cols), len(vals)) {
		col := cols[k]
		if uint(col) >= uint(n) || effLB[col] >= effUB[col] {
			continue
		}
		a := vals[k]
		if a == 0 {
			continue
		}
		av := min(1.0, math.Abs(a))
		if av > domMax {
			domMax = av
		}
		var canMove bool
		if decrease {
			canMove = (a > 0 && (math.IsInf(effLB[col], -1) || primal[col] > effLB[col])) ||
				(a < 0 && (math.IsInf(effUB[col], 1) || primal[col] < effUB[col]))
		} else {
			canMove = (a > 0 && (math.IsInf(effUB[col], 1) || primal[col] < effUB[col])) ||
				(a < 0 && (math.IsInf(effLB[col], -1) || primal[col] > effLB[col]))
		}
		if canMove && av > dirMax {
			if av >= 1.0 {
				return 1.0
			}
			dirMax = av
		}
	}
	if dirMax > 0 {
		return dirMax
	}
	if domMax > 0 {
		return domMax
	}
	return 1.0
}

func cbcRowResidualWithinTol(cols []int, vals, primal, effLB, effUB []float64, rhs, rowLB, rowUB float64) bool {
	res, rTolRes, ok := evalSparseRowResidualAndTol(cols, vals, primal, effLB, effUB, rhs, 1e-4, 1e-6)
	if !ok {
		return false
	}
	dirLo, dirHi, ok := evalSparseRowDirTol(cols, vals, primal, effLB, effUB, 0, 1e-6)
	if !ok {
		return false
	}
	if !math.IsInf(rowLB, -1) && res < 0 {
		scaleLo := cbcRowRepairMaxCoeffAtMost1(cols, vals, primal, effLB, effUB, false)
		rTolLo := min(rTolRes, max(1e-4*scaleLo, dirLo))
		if res < -rTolLo {
			return false
		}
	}
	if !math.IsInf(rowUB, 1) && res > 0 {
		scaleHi := cbcRowRepairMaxCoeffAtMost1(cols, vals, primal, effLB, effUB, true)
		rTolHi := min(rTolRes, max(1e-4*scaleHi, dirHi))
		if res > rTolHi {
			return false
		}
	}
	return true
}

func cbcPrimalWithinTol(p *CompiledProblem, lb, ub []float64, cuts cbcCutSet, primal []float64, checkInt bool) bool {
	n := p.numVars
	if len(primal) < n || len(lb) < n || len(ub) < n {
		return false
	}
	effLB := lb
	effUB := ub
	var snappedLB, snappedUB []float64
	for j := range n {
		v := primal[j]
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
		lj := lb[j]
		uj := ub[j]
		if checkInt && p.varCat[j] != Continuous {
			rv := math.Round(v)
			if math.Abs(v-rv) > 1e-3 {
				return false
			}
			// Snap near-integer primal values onto exact integers when computing row tolerances
			// so residual tolerance never scales up on non-integer noise within integrality tol.
			if math.Abs(rv-lj) <= 1e-3 || math.Abs(rv-uj) <= 1e-3 {
				if snappedLB == nil {
					snappedLB = slices.Clone(lb)
					snappedUB = slices.Clone(ub)
					effLB = snappedLB
					effUB = snappedUB
				}
				if rv >= lj-1e-3 && rv <= uj+1e-3 {
					// If variable round-snaps inside its declared integer bounds, treat its bound as tight.
					effLB[j] = rv
					effUB[j] = rv
				}
			}
		}
		bTol := max(1e-4, 1e-6*max(1.0, math.Abs(v)))
		if (!math.IsInf(lj, -1) && v < lj && lj-v > bTol) ||
			(!math.IsInf(uj, 1) && v > uj && v-uj > bTol) {
			return false
		}
	}
	for i := range p.numConstraints {
		st, en := p.rowPtr[i], p.rowPtr[i+1]
		if !cbcRowResidualWithinTol(p.colIdx[st:en], p.rowVal[st:en], primal, effLB, effUB, p.rowRHS[i], p.rowLB[i], p.rowUB[i]) {
			return false
		}
	}
	for e := range cuts.count() {
		if !cbcRowResidualWithinTol(cuts.cols[e], cuts.vals[e], primal, effLB, effUB, cuts.rhs[e], math.Inf(-1), cuts.rhs[e]) {
			return false
		}
	}
	return true
}

func cbcLPDualWithinTol(p *CompiledProblem, lb, ub []float64, cuts cbcCutSet, primal, yMin, rcMin []float64, objMin float64) bool {
	n := p.numVars
	baseM := p.numConstraints
	numExtra := cuts.count()
	m := baseM + numExtra
	if len(primal) < n || len(lb) < n || len(ub) < n || len(yMin) < m || math.IsNaN(objMin) || math.IsInf(objMin, 0) {
		return false
	}
	maxDual := 1.0
	maxFinData := 1.0
	if !math.IsNaN(p.objConstant) && !math.IsInf(p.objConstant, 0) {
		maxFinData = max(maxFinData, math.Abs(p.objConstant))
	}
	hasInfStruct := false
	hasTinyEntry := false
	for j := range n {
		if math.IsNaN(primal[j]) || math.IsInf(primal[j], 0) {
			return false
		}
		if !math.IsInf(lb[j], 0) {
			maxFinData = max(maxFinData, math.Abs(lb[j]))
		}
		if !math.IsInf(ub[j], 0) {
			maxFinData = max(maxFinData, math.Abs(ub[j]))
		}
		av := math.Abs(p.objMin[j])
		if av > maxDual {
			maxDual = av
		}
		if lb[j] < ub[j] {
			if av > 0 && av <= 1e-5 {
				hasTinyEntry = true
			}
			if math.IsInf(lb[j], -1) || math.IsInf(ub[j], 1) {
				hasInfStruct = true
			}
		}
	}
	for i := range baseM {
		if !math.IsInf(p.rowRHS[i], 0) {
			maxFinData = max(maxFinData, math.Abs(p.rowRHS[i]))
		}
	}
	for e := range numExtra {
		if !math.IsInf(cuts.rhs[e], 0) {
			maxFinData = max(maxFinData, math.Abs(cuts.rhs[e]))
		}
	}
	for j := range n {
		if lb[j] < ub[j] && (math.IsInf(lb[j], -1) || math.IsInf(ub[j], 1)) &&
			math.Abs(primal[j]) >= 1e18 && math.Abs(primal[j]) > 100.0*maxFinData {
			return false
		}
	}
	for i := range m {
		yi := yMin[i]
		if math.IsNaN(yi) || math.IsInf(yi, 0) {
			return false
		}
		if ay := math.Abs(yi); ay > maxDual {
			maxDual = ay
		}
	}

	dj := make([]float64, n)
	eb := make([]float64, n)
	yDotSc := make([]float64, n)
	maxBasisDualRel := 0.0
	var colRows []int
	var colVals []float64
	for j := range n {
		if lb[j] == ub[j] {
			continue
		}
		st, en := p.colPtr[j], p.colPtr[j+1]
		colRows = append(colRows[:0], p.rowIdx[st:en]...)
		colVals = append(colVals[:0], p.colVal[st:en]...)
		for e := range numExtra {
			for k, c := range cuts.cols[e] {
				if c == j {
					colRows = append(colRows, baseM+e)
					colVals = append(colVals, cuts.vals[e][k])
					break
				}
			}
		}
		if hasInfStruct && !hasTinyEntry {
			for _, cv := range colVals {
				if acv := math.Abs(cv); acv > 0 && acv <= 1e-7 {
					hasTinyEntry = true
					break
				}
			}
		}
		v, ebj, _, yds, ok := evalColDualDot(p.objMin[j], colRows, colVals, yMin, nil, nil)
		if !ok {
			return false
		}
		dj[j] = v
		eb[j] = ebj
		yDotSc[j] = yds

		bTol := max(1e-6, 1e-8*max(1.0, math.Abs(primal[j])))
		interior := (math.IsInf(lb[j], -1) || primal[j] > lb[j]+bTol) &&
			(math.IsInf(ub[j], 1) || primal[j] < ub[j]-bTol)
		// Only basic columns (rcMin[j] == 0 when available) estimate basis-solve
		// dual roundoff; an interior column with non-zero rcMin (e.g. an unbounded
		// ray stopped at CLP's internal 1e10 bound) must not inflate maxBasisDualRel.
		isBasic := (j < len(rcMin) && rcMin[j] == 0) || (len(rcMin) < n && interior)
		if isBasic {
			denSc := yds + math.Ldexp(math.Abs(p.objMin[j]), -32)
			if denSc > 0 {
				numSc := math.Ldexp(math.Abs(v)+ebj, -32)
				rel := numSc / denSc
				if rel > 1e-4 {
					if math.Abs(v) > max(1e-12*maxDual, ebj) {
						return false
					}
					rel = (math.Abs(v) + ebj) / maxDual
				}
				if rel > maxBasisDualRel {
					maxBasisDualRel = rel
				}
			}
		}
	}

	canRecede, _ := canRecedeNegLP(p, lb, ub)
	strictRecede := canRecede || (hasInfStruct && hasTinyEntry)
	rowSlackUnbounded := func(cols []int, vals []float64, toPosInf bool) bool {
		if !hasInfStruct || !strictRecede {
			return false
		}
		for k, c := range cols {
			if uint(c) >= uint(n) || lb[c] >= ub[c] {
				continue
			}
			a := vals[k]
			if a == 0 {
				continue
			}
			if toPosInf {
				if (a > 0 && math.IsInf(lb[c], -1)) || (a < 0 && math.IsInf(ub[c], 1)) {
					return true
				}
			} else {
				if (a > 0 && math.IsInf(ub[c], 1)) || (a < 0 && math.IsInf(lb[c], -1)) {
					return true
				}
			}
		}
		return false
	}

	signTol := max(1e-4, 1e-5*maxDual)
	slackNoise := max(1e-7, 1e-7*maxDual)
	compGap := 0.0

	rowDataAt := func(i int) (cols []int, vals []float64, rhs float64, sense ConstraintSense) {
		if i < baseM {
			st, en := p.rowPtr[i], p.rowPtr[i+1]
			return p.colIdx[st:en], p.rowVal[st:en], p.rowRHS[i], p.rowSense[i]
		}
		e := i - baseM
		return cuts.cols[e], cuts.vals[e], cuts.rhs[e], LessEqual
	}

	rowRes := make([]float64, m)
	rowTols := make([]float64, m)
	rowRelTols := make([]float64, m)
	for i := range m {
		yi := yMin[i]
		if yi == 0 {
			continue
		}
		cols, vals, rhs, sense := rowDataAt(i)
		res, relRTol, ok := evalSparseRowResidualAndTol(cols, vals, primal, lb, ub, rhs, 0, 1e-6)
		if !ok {
			return false
		}
		rTol := max(1e-4, relRTol)
		rowRes[i] = res
		rowTols[i] = rTol
		rowRelTols[i] = relRTol
		slackRow := (sense == LessEqual && res < -10.0*rTol) || (sense == GreaterEqual && res > 10.0*rTol)
		if slackRow {
			if math.Abs(yi) > signTol {
				return false
			}
			if math.Abs(yi) <= slackNoise {
				if rel := math.Abs(yi) / maxDual; rel > maxBasisDualRel {
					maxBasisDualRel = rel
				}
			}
		}
	}

	for i := range m {
		yi := yMin[i]
		if yi == 0 {
			continue
		}
		cols, vals, _, sense := rowDataAt(i)
		res := rowRes[i]
		relRTol := rowRelTols[i]
		switch sense {
		case LessEqual:
			rSignTol := signTol
			if rowSlackUnbounded(cols, vals, true) {
				rSignTol = combineDualNoiseTol(0, 0, math.Ldexp(math.Abs(yi), -32), maxBasisDualRel, 64.0)
			}
			if yi > rSignTol {
				return false
			}
			if yi < -slackNoise && res < -relRTol {
				compGap += (-yi - slackNoise) * (-res - relRTol)
			}
		case GreaterEqual:
			rSignTol := signTol
			if rowSlackUnbounded(cols, vals, false) {
				rSignTol = combineDualNoiseTol(0, 0, math.Ldexp(math.Abs(yi), -32), maxBasisDualRel, 64.0)
			}
			if yi < -rSignTol {
				return false
			}
			if yi > slackNoise && res > relRTol {
				compGap += (yi - slackNoise) * (res - relRTol)
			}
		}
	}

	for j := range n {
		if lb[j] == ub[j] {
			continue
		}
		colL1Sc := yDotSc[j] + math.Ldexp(math.Abs(p.objMin[j]), -32)
		colNoise := eb[j] + math.Ldexp(1e-8*colL1Sc, 32)
		if math.IsInf(colNoise, 1) {
			colNoise = math.MaxFloat64
		}
		infTol := colNoise
		if strictRecede {
			infTol = combineDualNoiseTol(eb[j], 0, colL1Sc, maxBasisDualRel, 64.0)
		}
		if math.IsInf(ub[j], 1) && dj[j] < -infTol {
			return false
		}
		if math.IsInf(lb[j], -1) && dj[j] > infTol {
			return false
		}
		colTol := max(1e-4, max(colNoise, combineDualNoiseTol(eb[j], 0, colL1Sc, maxBasisDualRel, 64.0)))
		if j < len(rcMin) {
			if math.IsNaN(rcMin[j]) || math.IsInf(rcMin[j], 0) {
				return false
			}
			if math.IsInf(ub[j], 1) && rcMin[j] < -infTol {
				return false
			}
			if math.IsInf(lb[j], -1) && rcMin[j] > infTol {
				return false
			}
			if math.Abs(rcMin[j]-dj[j]) > colTol {
				return false
			}
		}
		bTol := max(1e-4, 1e-6*max(1.0, math.Abs(primal[j])))
		if !math.IsInf(ub[j], 1) && dj[j] < -colTol && primal[j] < ub[j] && ub[j]-primal[j] > bTol {
			return false
		}
		if !math.IsInf(lb[j], -1) && dj[j] > colTol && primal[j] > lb[j] && primal[j]-lb[j] > bTol {
			return false
		}
		if !math.IsInf(ub[j], 1) && dj[j] < -colNoise && primal[j] < ub[j] {
			rem := ub[j] - primal[j]
			if math.IsInf(rem, 1) {
				compGap += math.Ldexp(-dj[j]-colNoise, 2) * (math.Ldexp(ub[j], -2) - math.Ldexp(primal[j], -2))
			} else if rem > 0 {
				compGap += (-dj[j] - colNoise) * rem
			}
		}
		if !math.IsInf(lb[j], -1) && dj[j] > colNoise && primal[j] > lb[j] {
			rem := primal[j] - lb[j]
			if math.IsInf(rem, 1) {
				compGap += math.Ldexp(dj[j]-colNoise, 2) * (math.Ldexp(primal[j], -2) - math.Ldexp(lb[j], -2))
			} else if rem > 0 {
				compGap += (dj[j] - colNoise) * rem
			}
		}
	}

	if math.IsNaN(compGap) || math.IsInf(compGap, 0) {
		return false
	}
	return compGap <= max(1e-3, 1e-4*max(1.0, math.Abs(objMin)))
}
