//go:build cgo && cbc

package goblin

import (
	"cmp"
	"math"
	"slices"
	"time"
)

const (
	cbcCancelMagThreshold      = 1e8
	cbcUnresolvedCancelFloor   = cbcCancelMagThreshold
	cbcPrecondRemTol           = 1e-12
	cbcPrecondMinPivot         = 1e-6
	cbcPrecondMinSafeCoeff     = 1e-9
	cbcPrecondMaxEqRowLen      = 64
	cbcPrecondMaxTightenPasses = 32
	cbcPrecondNormShrinkRatio  = 0.25
)

type cbcDualFixup struct {
	isObj  bool
	eqRow  int
	dstRow int
	mu     float64
}

type cbcWorkRow struct {
	sense ConstraintSense
	rhs   float64
	cols  []int
	vals  []float64
}

type cbcEqBasisRow struct {
	pivCol      int
	pivVal      float64
	cols        []int
	vals        []float64
	rhs         float64
	combRows    []int
	combWeights []float64
}

type cbcPrecond struct {
	active         bool
	infeasible     bool
	timedOut       bool
	unresolved     bool
	colPtr32       []int32
	rowIdx32       []int32
	colVal         []float64
	colLBClp       []float64
	colUBClp       []float64
	workLB         []float64
	objMin         []float64
	rowLBClp       []float64
	rowUBClp       []float64
	cuts           cbcCutSet
	fixedCols      []bool
	fixedHi        []float64
	fixedMid       []float64
	fixedLo        []float64
	fixedObjNonFin bool
	hasFixedObj    bool
	dualFixups     []cbcDualFixup
}

func isPow2OrZero(v float64) bool {
	if v == 0 {
		return true
	}
	fr, _ := math.Frexp(math.Abs(v))
	return fr == 0.5
}

func termCancelContribution(coeff, lb, ub float64, isBin bool) (posMag, negMag float64, hasPosInf, hasNegInf bool, limbs int) {
	if coeff == 0 || (lb == 0 && ub == 0) {
		return 0, 0, false, false, 0
	}
	valPow2 := isBin || (lb == ub && !math.IsInf(lb, 0) && isPow2OrZero(lb))
	if isPow2OrZero(coeff) || valPow2 {
		limbs = 1
	} else {
		limbs = 2
	}
	addEnd := func(v float64) {
		prod := coeff * v
		if math.IsInf(prod, 0) {
			prod = math.Copysign(math.MaxFloat64, prod)
			limbs = max(limbs, 3)
		}
		if prod > posMag {
			posMag = prod
		} else if -prod > negMag {
			negMag = -prod
		}
	}
	if math.IsInf(lb, -1) {
		if coeff > 0 {
			hasNegInf = true
			negMag = max(negMag, coeff)
		} else {
			hasPosInf = true
			posMag = max(posMag, -coeff)
		}
	} else if !math.IsInf(lb, 0) {
		addEnd(lb)
	}
	if math.IsInf(ub, 1) {
		if coeff > 0 {
			hasPosInf = true
			posMag = max(posMag, coeff)
		} else {
			hasNegInf = true
			negMag = max(negMag, -coeff)
		}
	} else if !math.IsInf(ub, 0) {
		addEnd(ub)
	}
	return posMag, negMag, hasPosInf, hasNegInf, limbs
}

func sparseExprCancelStats(cols []int, vals, lb, ub []float64, varCat []VarCategory, rhsLB, rhsUB float64) (posSum, negSum float64, hasPosInf, hasNegInf bool, limbs int) {
	n := min(len(lb), len(ub))
	for k := range min(len(cols), len(vals)) {
		col := cols[k]
		if uint(col) >= uint(n) {
			continue
		}
		isBin := col < len(varCat) && varCat[col] == Binary && lb[col] >= 0 && ub[col] <= 1
		pm, nm, pInf, nInf, l := termCancelContribution(vals[k], lb[col], ub[col], isBin)
		if l == 0 {
			continue
		}
		limbs += l
		posSum = satAddNonNeg(posSum, pm)
		negSum = satAddNonNeg(negSum, nm)
		hasPosInf = hasPosInf || pInf
		hasNegInf = hasNegInf || nInf
	}
	rhsCounted := false
	for _, rVal := range [2]float64{rhsLB, rhsUB} {
		if math.IsNaN(rVal) || math.IsInf(rVal, 0) || rVal == 0 {
			continue
		}
		if !rhsCounted {
			limbs++
			rhsCounted = true
		}
		if -rVal > 0 {
			posSum = satAddNonNeg(posSum, -rVal)
		} else {
			negSum = satAddNonNeg(negSum, rVal)
		}
	}
	return posSum, negSum, hasPosInf, hasNegInf, limbs
}

func evalCancelRisk(posSum, negSum float64, hasPosInf, hasNegInf bool, limbs int, threshold, infScale float64) bool {
	if limbs < 3 {
		return false
	}
	if hasPosInf && hasNegInf && infScale >= threshold {
		return true
	}
	if hasPosInf && negSum >= threshold {
		posSum = max(posSum, negSum)
	}
	if hasNegInf && posSum >= threshold {
		negSum = max(negSum, posSum)
	}
	return posSum >= threshold && negSum >= threshold
}

func sparseExprHasCancelRisk(cols []int, vals, lb, ub []float64, varCat []VarCategory, rhsLB, rhsUB, threshold, infScale float64) bool {
	posSum, negSum, hasPosInf, hasNegInf, limbs := sparseExprCancelStats(cols, vals, lb, ub, varCat, rhsLB, rhsUB)
	return evalCancelRisk(posSum, negSum, hasPosInf, hasNegInf, limbs, threshold, infScale)
}

func denseObjHasCancelRisk(obj []float64, objConstMin float64, lb, ub []float64, varCat []VarCategory, threshold, infScale float64) bool {
	n := min(len(obj), min(len(lb), len(ub)))
	var posSum, negSum float64
	var hasPosInf, hasNegInf bool
	limbs := 0
	for j := range n {
		if obj[j] == 0 {
			continue
		}
		isBin := j < len(varCat) && varCat[j] == Binary && lb[j] >= 0 && ub[j] <= 1
		pm, nm, pInf, nInf, l := termCancelContribution(obj[j], lb[j], ub[j], isBin)
		if l == 0 {
			continue
		}
		limbs += l
		posSum = satAddNonNeg(posSum, pm)
		negSum = satAddNonNeg(negSum, nm)
		hasPosInf = hasPosInf || pInf
		hasNegInf = hasNegInf || nInf
	}
	if !math.IsNaN(objConstMin) && !math.IsInf(objConstMin, 0) && objConstMin != 0 {
		limbs++
		if objConstMin > 0 {
			posSum = satAddNonNeg(posSum, objConstMin)
		} else {
			negSum = satAddNonNeg(negSum, -objConstMin)
		}
	}
	return evalCancelRisk(posSum, negSum, hasPosInf, hasNegInf, limbs, threshold, infScale)
}

func problemImpliedScaleHint(p *CompiledProblem, lb, ub []float64, cuts cbcCutSet) (maxFin float64, hasSmallCoeff bool) {
	n := min(p.numVars, min(len(lb), len(ub)))
	if ac := math.Abs(p.objConstant); !math.IsInf(ac, 0) && ac > maxFin {
		maxFin = ac
	}
	for j := range n {
		if !math.IsInf(lb[j], 0) && math.Abs(lb[j]) > maxFin {
			maxFin = math.Abs(lb[j])
		}
		if !math.IsInf(ub[j], 0) && math.Abs(ub[j]) > maxFin {
			maxFin = math.Abs(ub[j])
		}
		if ao := math.Abs(p.objMin[j]); ao > maxFin {
			maxFin = ao
		}
	}
	for i := range p.numConstraints {
		if !math.IsInf(p.rowLB[i], 0) && math.Abs(p.rowLB[i]) > maxFin {
			maxFin = math.Abs(p.rowLB[i])
		}
		if !math.IsInf(p.rowUB[i], 0) && math.Abs(p.rowUB[i]) > maxFin {
			maxFin = math.Abs(p.rowUB[i])
		}
	}
	for _, v := range p.colVal {
		av := math.Abs(v)
		if av > maxFin {
			maxFin = av
		}
		if av > 0 && av <= 1e-2 {
			hasSmallCoeff = true
		}
	}
	for _, r := range cuts.rhs {
		if !math.IsInf(r, 0) && math.Abs(r) > maxFin {
			maxFin = math.Abs(r)
		}
	}
	for _, cvs := range cuts.vals {
		for _, v := range cvs {
			av := math.Abs(v)
			if av > maxFin {
				maxFin = av
			}
			if av > 0 && av <= 1e-2 {
				hasSmallCoeff = true
			}
		}
	}
	return maxFin, hasSmallCoeff
}

func hasCBCCancellationRisk(p *CompiledProblem, lb, ub []float64, cuts cbcCutSet) (rowCancel, objCancel bool) {
	if p == nil || p.numVars == 0 {
		return false, false
	}
	n := p.numVars
	constMin := p.objConstMin()
	evalOnBounds := func(bLB, bUB []float64, infScale float64) (rc, oc bool) {
		oc = denseObjHasCancelRisk(p.objMin, constMin, bLB, bUB, p.varCat, cbcCancelMagThreshold, infScale)
		for i := range p.numConstraints {
			st, en := p.rowPtr[i], p.rowPtr[i+1]
			if sparseExprHasCancelRisk(p.colIdx[st:en], p.rowVal[st:en], bLB, bUB, p.varCat, p.rowLB[i], p.rowUB[i], cbcCancelMagThreshold, infScale) {
				rc = true
				break
			}
		}
		if !rc {
			for e := range cuts.count() {
				if sparseExprHasCancelRisk(cuts.cols[e], cuts.vals[e], bLB, bUB, p.varCat, math.Inf(-1), cuts.rhs[e], cbcCancelMagThreshold, infScale) {
					rc = true
					break
				}
			}
		}
		return rc, oc
	}
	rowCancel, objCancel = evalOnBounds(lb, ub, 0)
	if rowCancel || objCancel || !hasAnyInfiniteBound(lb, ub, n) {
		return rowCancel, objCancel
	}
	maxFin, hasSmallCoeff := problemImpliedScaleHint(p, lb, ub, cuts)
	if maxFin < cbcCancelMagThreshold && !(hasSmallCoeff && maxFin > 0) {
		return false, false
	}
	tLB := slices.Clone(lb[:n])
	tUB := slices.Clone(ub[:n])
	infeas, _ := tightenContinuousBoundsCBC(p, nil, cuts, tLB, tUB, cbcPrecondMaxTightenPasses, time.Time{}, nil)
	if infeas {
		return true, false
	}
	infScale := maxFin
	for j := range n {
		if !math.IsInf(tLB[j], 0) && math.Abs(tLB[j]) > infScale {
			infScale = math.Abs(tLB[j])
		}
		if !math.IsInf(tUB[j], 0) && math.Abs(tUB[j]) > infScale {
			infScale = math.Abs(tUB[j])
		}
	}
	return evalOnBounds(tLB, tUB, infScale)
}

func cbcMIPObjectiveScaleExp(objMin []float64) (objExp int, maxAbsObj float64) {
	minNonZero := math.Inf(1)
	for _, c := range objMin {
		ao := math.Abs(c)
		if ao > maxAbsObj {
			maxAbsObj = ao
		}
		if ao > 0 && ao < minNonZero {
			minNonZero = ao
		}
	}
	if maxAbsObj >= 1e35 && len(objMin) > 0 {
		_, refExp := math.Frexp(maxAbsObj)
		wantExp := max(0, refExp-40)
		if !math.IsInf(minNonZero, 1) && minNonZero > 0 {
			_, minExp := math.Frexp(minNonZero)
			maxSafeExp := max(0, (minExp-1)-(-1000))
			wantExp = min(wantExp, maxSafeExp)
		}
		objExp = wantExp
	}
	return objExp, maxAbsObj
}

func hasUnscalableCBCMIPObjective(objMin []float64) bool {
	objExp, maxAbsObj := cbcMIPObjectiveScaleExp(objMin)
	return maxAbsObj >= 1e35 && math.Ldexp(maxAbsObj, -objExp) >= 1e40
}

func hasCBCDroppedTiny(p *CompiledProblem, lb, ub []float64, cuts cbcCutSet) bool {
	n := min(p.numVars, min(len(lb), len(ub)))
	objExp := 0
	if p.isMIP {
		objExp, _ = cbcMIPObjectiveScaleExp(p.objMin)
	}
	droppedObjSum := 0.0
	for j := range n {
		if lb[j] == 0 && ub[j] == 0 {
			continue
		}
		if lb[j] > ub[j] {
			continue
		}
		origAO := math.Abs(p.objMin[j])
		if origAO <= 0 {
			continue
		}
		ao := origAO
		if objExp > 0 {
			ao = math.Ldexp(ao, -objExp)
		}
		if ao <= 1e-18 {
			unbounded := math.IsInf(lb[j], 0) || math.IsInf(ub[j], 0) ||
				math.Abs(lb[j]) >= 1e29 || math.Abs(ub[j]) >= 1e29 || math.IsInf(ub[j]-lb[j], 0)
			if unbounded {
				return true
			}
			maxB := max(math.Abs(lb[j]), math.Abs(ub[j]))
			droppedObjSum += origAO * maxB
			if droppedObjSum >= 1e-8 || math.IsNaN(droppedObjSum) || math.IsInf(droppedObjSum, 0) {
				return true
			}
		}
	}
	return hasCBCTinyMatrixCoeffRisk(p, lb, ub, cuts, 1e-18, 1e-8)
}

func isCBCObjAmplifiedTinyEntry(av, objAbs, lb, ub float64) bool {
	if av <= 0 || av > 1e-7 || objAbs == 0 {
		return false
	}
	if av > 1e-18 && objAbs < 1e7 {
		return false
	}
	if math.IsInf(lb, -1) || math.IsInf(ub, 1) {
		return true
	}
	return objAbs*(ub-lb) > 1e-6
}

func hasCBCPivotRiskTiny(p *CompiledProblem, lb, ub []float64, cuts cbcCutSet) bool {
	if hasCBCTinyMatrixCoeffRisk(p, lb, ub, cuts, 1e-11, 1e-7) {
		return true
	}
	n := min(p.numVars, min(len(lb), len(ub)))
	var rowTiny18 []float64
	for j := range n {
		if lb[j] >= ub[j] && lb[j] == 0 {
			continue
		}
		free := lb[j] < ub[j]
		objAbs := math.Abs(p.objMin[j])
		infB := math.IsInf(lb[j], 0) || math.IsInf(ub[j], 0) || math.Abs(lb[j]) >= 1e30 || math.Abs(ub[j]) >= 1e30
		maxB := max(1.0, max(math.Abs(lb[j]), math.Abs(ub[j])))
		for k := p.colPtr[j]; k < p.colPtr[j+1]; k++ {
			av := math.Abs(p.colVal[k])
			if free && objAbs > 0 && isCBCObjAmplifiedTinyEntry(av, objAbs, lb[j], ub[j]) {
				return true
			}
			if av > 0 && av <= 1e-18 {
				r := p.rowIdx[k]
				if r >= 0 && r < p.numConstraints {
					if rowTiny18 == nil {
						rowTiny18 = make([]float64, p.numConstraints)
					}
					if infB {
						rowTiny18[r] = math.Inf(1)
					} else {
						rowTiny18[r] += av * maxB
					}
				}
			}
		}
	}
	if rowTiny18 != nil {
		for j := range n {
			if lb[j] >= ub[j] {
				continue
			}
			objAbs := math.Abs(p.objMin[j])
			if objAbs == 0 {
				continue
			}
			for k := p.colPtr[j]; k < p.colPtr[j+1]; k++ {
				r := p.rowIdx[k]
				if r >= 0 && r < p.numConstraints && rowTiny18[r] > 0 {
					av := math.Abs(p.colVal[k])
					if av > 0 && (math.IsInf(rowTiny18[r], 1) || (rowTiny18[r]/av)*objAbs > 1e-6) {
						return true
					}
				}
			}
		}
	}
	for e := range cuts.colValCount() {
		cutTiny18 := 0.0
		for k := range min(len(cuts.cols[e]), len(cuts.vals[e])) {
			c := cuts.cols[e][k]
			if uint(c) >= uint(n) || (lb[c] >= ub[c] && lb[c] == 0) {
				continue
			}
			av := math.Abs(cuts.vals[e][k])
			if lb[c] < ub[c] && isCBCObjAmplifiedTinyEntry(av, math.Abs(p.objMin[c]), lb[c], ub[c]) {
				return true
			}
			if av > 0 && av <= 1e-18 {
				if math.IsInf(lb[c], 0) || math.IsInf(ub[c], 0) || math.Abs(lb[c]) >= 1e30 || math.Abs(ub[c]) >= 1e30 {
					cutTiny18 = math.Inf(1)
				} else {
					cutTiny18 += av * max(1.0, max(math.Abs(lb[c]), math.Abs(ub[c])))
				}
			}
		}
		if cutTiny18 > 0 {
			for k := range min(len(cuts.cols[e]), len(cuts.vals[e])) {
				c := cuts.cols[e][k]
				if uint(c) >= uint(n) || lb[c] >= ub[c] {
					continue
				}
				objAbs := math.Abs(p.objMin[c])
				if av := math.Abs(cuts.vals[e][k]); objAbs > 0 && av > 0 && (math.IsInf(cutTiny18, 1) || (cutTiny18/av)*objAbs > 1e-6) {
					return true
				}
			}
		}
	}
	return false
}

// hasCBCTinyMatrixCoeffRisk reports whether any constraint or cut contains non-zero
// coefficients at or below coefThresh whose active domain bound contribution reaches sumThresh.
func hasCBCTinyMatrixCoeffRisk(p *CompiledProblem, lb, ub []float64, cuts cbcCutSet, coefThresh, sumThresh float64) bool {
	n := min(p.numVars, min(len(lb), len(ub)))
	var rowSum []float64
	for j := range n {
		if lb[j] >= ub[j] && lb[j] == 0 {
			continue
		}
		infB := math.IsInf(lb[j], 0) || math.IsInf(ub[j], 0) || math.Abs(lb[j]) >= 1e30 || math.Abs(ub[j]) >= 1e30
		maxB := max(1.0, max(math.Abs(lb[j]), math.Abs(ub[j])))
		for k := p.colPtr[j]; k < p.colPtr[j+1]; k++ {
			if av := math.Abs(p.colVal[k]); av > 0 && av <= coefThresh {
				if infB {
					return true
				}
				r := p.rowIdx[k]
				if r >= 0 && r < p.numConstraints {
					if rowSum == nil {
						rowSum = make([]float64, p.numConstraints)
					}
					rowSum[r] += av * maxB
					if rowSum[r] >= sumThresh || math.IsNaN(rowSum[r]) || math.IsInf(rowSum[r], 0) {
						return true
					}
				}
			}
		}
	}
	for e := range cuts.colValCount() {
		cutSum := 0.0
		for k := range min(len(cuts.cols[e]), len(cuts.vals[e])) {
			c := cuts.cols[e][k]
			if uint(c) >= uint(n) || (lb[c] >= ub[c] && lb[c] == 0) {
				continue
			}
			if av := math.Abs(cuts.vals[e][k]); av > 0 && av <= coefThresh {
				infB := math.IsInf(lb[c], 0) || math.IsInf(ub[c], 0) || math.Abs(lb[c]) >= 1e30 || math.Abs(ub[c]) >= 1e30
				if infB {
					return true
				}
				maxB := max(1.0, max(math.Abs(lb[c]), math.Abs(ub[c])))
				cutSum += av * maxB
				if cutSum >= sumThresh || math.IsNaN(cutSum) || math.IsInf(cutSum, 0) {
					return true
				}
			}
		}
	}
	return false
}

func hasCBCScalingRisk(p *CompiledProblem, lb, ub []float64, cuts cbcCutSet) bool {
	n := min(p.numVars, min(len(lb), len(ub)))
	for j := range n {
		if lb[j] >= ub[j] && lb[j] == 0 {
			continue
		}
		if ao := math.Abs(p.objMin[j]); ao >= 1e7 || (ao > 0 && ao <= 1e-7) {
			return true
		}
		for k := p.colPtr[j]; k < p.colPtr[j+1]; k++ {
			if av := math.Abs(p.colVal[k]); av >= 1e7 || (av > 0 && av <= 1e-7) {
				return true
			}
		}
	}
	for _, cvs := range cuts.vals {
		for _, v := range cvs {
			if av := math.Abs(v); av >= 1e7 || (av > 0 && av <= 1e-7) {
				return true
			}
		}
	}
	return false
}

func divExactFromWide(w *wideDotAcc, a float64) (qVal, qLo, qHi float64, exact, ok bool) {
	if a == 0 {
		return 0, 0, 0, false, false
	}
	top, rem, wOK := w.finishDD()
	if !wOK {
		return 0, 0, 0, false, false
	}
	if rem == 0 {
		q := top / a
		if !math.IsNaN(q) && !math.IsInf(q, 0) {
			p, pe := twoProd(q, a)
			if p == top && pe == 0 {
				return q, q, q, true, true
			}
		}
	}
	val, eb, divOK := divDDByFloat(top, rem, a, 0)
	if !divOK {
		return 0, 0, 0, false, false
	}
	return val, val - eb, val + eb, false, true
}

func checkRowDirFeasibleCBC(cols []int, vals []float64, rhs, rowFeasTol float64, lb, ub []float64, n, kLim int, isUpper bool) bool {
	var w wideDotAcc
	w.addVal(rhs)
	for k := range kLim {
		col := cols[k]
		if uint(col) >= uint(n) {
			continue
		}
		a := vals[k]
		if a == 0 {
			continue
		}
		if (a > 0) == isUpper {
			w.addProd(-a, lb[col])
		} else {
			w.addProd(-a, ub[col])
		}
	}
	top, rem, ok := w.finishDD()
	if !ok {
		return false
	}
	if isUpper {
		return top+rem < -rowFeasTol
	}
	return top+rem > rowFeasTol
}

func tightenRowDirBoundsCBC(cols []int, vals []float64, rhs, rowFeasTol float64, varCat []VarCategory, lb, ub []float64, n, kLim, infCount int, isUpper bool) (infeas, changed bool) {
	for k := range kLim {
		col := cols[k]
		if uint(col) >= uint(n) {
			continue
		}
		a := vals[k]
		if math.Abs(a) <= cbcPrecondMinSafeCoeff || lb[col] >= ub[col] {
			continue
		}
		if infCount == 1 {
			useLB := (a > 0) == isUpper
			if (useLB && !math.IsInf(lb[col], -1)) || (!useLB && !math.IsInf(ub[col], 1)) {
				continue
			}
		}
		var w wideDotAcc
		w.addVal(rhs)
		for t := range kLim {
			if t == k {
				continue
			}
			c := cols[t]
			if uint(c) >= uint(n) {
				continue
			}
			at := vals[t]
			if at == 0 {
				continue
			}
			if (at > 0) == isUpper {
				if !math.IsInf(lb[c], -1) {
					w.addProd(-at, lb[c])
				}
			} else if !math.IsInf(ub[c], 1) {
				w.addProd(-at, ub[c])
			}
		}
		q, qLo, qHi, exact, qOK := divExactFromWide(&w, a)
		if !qOK {
			continue
		}
		isInt := len(varCat) > col && varCat[col] != Continuous
		infeasBnd, chg := applyQuotientBoundCBC((a > 0) == isUpper, isInt, q, qLo, qHi, exact, math.Abs(a), rowFeasTol, lb, ub, col)
		if infeasBnd {
			return true, false
		}
		if chg {
			changed = true
		}
	}
	return false, changed
}

func tightenRowSliceCBC(cols []int, vals []float64, rhs float64, sense ConstraintSense, varCat []VarCategory, lb, ub []float64, n int) (infeas, changed bool) {
	hasUpper := sense == LessEqual || sense == Equal
	hasLower := sense == GreaterEqual || sense == Equal
	kLim := min(len(cols), len(vals))

	minInfCount := 0
	maxInfCount := 0
	rowScale := max(1.0, math.Abs(rhs))
	for k := range kLim {
		col := cols[k]
		if uint(col) >= uint(n) {
			continue
		}
		a := vals[k]
		if a == 0 {
			continue
		}
		l, u := lb[col], ub[col]
		isFree := l < u
		if a > 0 {
			if math.IsInf(l, -1) {
				minInfCount++
			} else if isFree {
				rowScale = satAddNonNeg(rowScale, math.Abs(a*l))
			}
			if math.IsInf(u, 1) {
				maxInfCount++
			} else if isFree {
				rowScale = satAddNonNeg(rowScale, math.Abs(a*u))
			}
		} else {
			if math.IsInf(u, 1) {
				minInfCount++
			} else if isFree {
				rowScale = satAddNonNeg(rowScale, math.Abs(a*u))
			}
			if math.IsInf(l, -1) {
				maxInfCount++
			} else if isFree {
				rowScale = satAddNonNeg(rowScale, math.Abs(a*l))
			}
		}
	}
	rowFeasTol := max(cbcFeasTol, 1e-12*rowScale)

	if hasUpper && minInfCount == 0 && checkRowDirFeasibleCBC(cols, vals, rhs, rowFeasTol, lb, ub, n, kLim, true) {
		return true, false
	}
	if hasLower && maxInfCount == 0 && checkRowDirFeasibleCBC(cols, vals, rhs, rowFeasTol, lb, ub, n, kLim, false) {
		return true, false
	}
	if hasUpper && minInfCount <= 1 {
		infU, chgU := tightenRowDirBoundsCBC(cols, vals, rhs, rowFeasTol, varCat, lb, ub, n, kLim, minInfCount, true)
		if infU {
			return true, false
		}
		changed = changed || chgU
	}
	if hasLower && maxInfCount <= 1 {
		infL, chgL := tightenRowDirBoundsCBC(cols, vals, rhs, rowFeasTol, varCat, lb, ub, n, kLim, maxInfCount, false)
		if infL {
			return true, false
		}
		changed = changed || chgL
	}
	return false, changed
}

func tightenContinuousBoundsCBC(p *CompiledProblem, varCat []VarCategory, cuts cbcCutSet, lb, ub []float64, maxPasses int, deadline time.Time, done <-chan struct{}) (infeas, timedOut bool) {
	n := min(p.numVars, min(len(lb), len(ub)))
	m := p.numConstraints
	numExtra := cuts.count()
	for range maxPasses {
		if stopRequested(deadline, done) {
			return false, true
		}
		changed := false
		for i := range m {
			if (i&63 == 0) && stopRequested(deadline, done) {
				return false, true
			}
			st, en := p.rowPtr[i], p.rowPtr[i+1]
			infRow, chgRow := tightenRowSliceCBC(p.colIdx[st:en], p.rowVal[st:en], p.rowRHS[i], p.rowSense[i], varCat, lb, ub, n)
			if infRow {
				return true, false
			}
			if chgRow {
				changed = true
			}
		}
		for e := range numExtra {
			if (e&63 == 0) && stopRequested(deadline, done) {
				return false, true
			}
			infCut, chgCut := tightenRowSliceCBC(cuts.cols[e], cuts.vals[e], cuts.rhs[e], LessEqual, varCat, lb, ub, n)
			if infCut {
				return true, false
			}
			if chgCut {
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return false, false
}

func applyQuotientBoundCBC(tightenUpper, isInt bool, q, qLo, qHi float64, exact bool, absA, rowFeasTol float64, lb, ub []float64, col int) (infeas, changed bool) {
	feasAllow := rowFeasTol / absA
	if tightenUpper {
		if qHi < lb[col]-feasAllow {
			return true, false
		}
		var candUB float64
		if isInt {
			qBase := qHi
			if exact {
				qBase = q
			}
			candUB = math.Floor(qBase + min(1e-9, feasAllow))
		} else {
			candUB = qHi + 1e-12*max(1.0, math.Abs(qHi))
			if exact {
				candUB = q
			}
			if candUB < lb[col] {
				candUB = lb[col]
			}
		}
		if candUB < lb[col] {
			return true, false
		}
		if candUB < ub[col]-1e-12*max(1.0, math.Abs(ub[col])) {
			ub[col] = candUB
			return false, true
		}
		return false, false
	}
	if qLo > ub[col]+feasAllow {
		return true, false
	}
	var candLB float64
	if isInt {
		qBase := qLo
		if exact {
			qBase = q
		}
		candLB = math.Ceil(qBase - min(1e-9, feasAllow))
	} else {
		candLB = qLo - 1e-12*max(1.0, math.Abs(qLo))
		if exact {
			candLB = q
		}
		if candLB > ub[col] {
			candLB = ub[col]
		}
	}
	if candLB > ub[col] {
		return true, false
	}
	if candLB > lb[col]+1e-12*max(1.0, math.Abs(lb[col])) {
		lb[col] = candLB
		return false, true
	}
	return false, false
}

func colFiniteScale(lb, ub float64) float64 {
	m := 1.0
	if !math.IsInf(lb, 0) && math.Abs(lb) > m {
		m = math.Abs(lb)
	}
	if !math.IsInf(ub, 0) && math.Abs(ub) > m {
		m = math.Abs(ub)
	}
	return m
}

func denseObjPotentialNorm(obj []float64, objConstMin float64, lb, ub []float64) float64 {
	norm := 0.0
	if !math.IsNaN(objConstMin) && !math.IsInf(objConstMin, 0) {
		norm = math.Abs(objConstMin)
	}
	for j := range min(len(obj), min(len(lb), len(ub))) {
		if obj[j] == 0 || (lb[j] == 0 && ub[j] == 0) {
			continue
		}
		p := math.Abs(obj[j]) * colFiniteScale(lb[j], ub[j])
		if math.IsInf(p, 1) {
			return math.MaxFloat64
		}
		norm = satAddNonNeg(norm, p)
	}
	return norm
}

func sparseRowPotentialNorm(cols []int, vals, lb, ub []float64, rhs float64) float64 {
	norm := 0.0
	if !math.IsInf(rhs, 0) {
		norm = math.Abs(rhs)
	}
	for k := range min(len(cols), len(vals)) {
		col := cols[k]
		if vals[k] == 0 || (lb[col] == 0 && ub[col] == 0) {
			continue
		}
		p := math.Abs(vals[k]) * colFiniteScale(lb[col], ub[col])
		if math.IsInf(p, 1) {
			return math.MaxFloat64
		}
		norm = satAddNonNeg(norm, p)
	}
	return norm
}

func mergeRowLinearComb(dstCols []int, dstVals []float64, dstRHS float64, eqCols []int, eqVals []float64, eqRHS, mu float64) (outCols []int, outVals []float64, outRHS float64, ok bool) {
	var rw wideDotAcc
	rw.addVal(dstRHS)
	rw.addProd(-mu, eqRHS)
	rTop, rRem, rOK := rw.finishDD()
	if !rOK || math.Abs(rRem) != 0 || (!math.IsInf(rTop, 0) && math.Abs(rTop) >= 1e29) {
		return nil, nil, 0, false
	}
	outCols = make([]int, 0, len(dstCols)+len(eqCols))
	outVals = make([]float64, 0, len(dstCols)+len(eqCols))
	i, j := 0, 0
	for i < len(dstCols) || j < len(eqCols) {
		var col int
		var w wideDotAcc
		switch {
		case j >= len(eqCols) || (i < len(dstCols) && dstCols[i] < eqCols[j]):
			col = dstCols[i]
			w.addVal(dstVals[i])
			i++
		case i >= len(dstCols) || eqCols[j] < dstCols[i]:
			col = eqCols[j]
			w.addProd(-mu, eqVals[j])
			j++
		default:
			col = dstCols[i]
			w.addVal(dstVals[i])
			w.addProd(-mu, eqVals[j])
			i++
			j++
		}
		vTop, vRem, vOK := w.finishDD()
		if !vOK || math.Abs(vRem) != 0 {
			return nil, nil, 0, false
		}
		if vTop == 0 {
			continue
		}
		if math.Abs(vTop) <= cbcPrecondMinSafeCoeff {
			return nil, nil, 0, false
		}
		outCols = append(outCols, col)
		outVals = append(outVals, vTop)
	}
	return outCols, outVals, rTop, true
}

func mergeCombWeights(dstRows []int, dstW []float64, eqRows []int, eqW []float64, mu float64) (outRows []int, outW []float64, ok bool) {
	outRows = make([]int, 0, len(dstRows)+len(eqRows))
	outW = make([]float64, 0, len(dstRows)+len(eqRows))
	i, j := 0, 0
	for i < len(dstRows) || j < len(eqRows) {
		var row int
		var w wideDotAcc
		switch {
		case j >= len(eqRows) || (i < len(dstRows) && dstRows[i] < eqRows[j]):
			row = dstRows[i]
			w.addVal(dstW[i])
			i++
		case i >= len(dstRows) || eqRows[j] < dstRows[i]:
			row = eqRows[j]
			w.addProd(-mu, eqW[j])
			j++
		default:
			row = dstRows[i]
			w.addVal(dstW[i])
			w.addProd(-mu, eqW[j])
			i++
			j++
		}
		v := w.finish(math.NaN())
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, nil, false
		}
		if v == 0 {
			continue
		}
		outRows = append(outRows, row)
		outW = append(outW, v)
	}
	return outRows, outW, true
}

func buildCBCEqRREFBasis(wRows []cbcWorkRow, exclude []bool, scaleLB, scaleUB []float64, n int) []cbcEqBasisRow {
	var eqIdxs []int
	for i := range wRows {
		if i < len(exclude) && exclude[i] {
			continue
		}
		if wRows[i].sense == Equal && len(wRows[i].cols) > 0 && len(wRows[i].cols) <= cbcPrecondMaxEqRowLen {
			eqIdxs = append(eqIdxs, i)
		}
	}
	if len(eqIdxs) == 0 {
		return nil
	}
	slices.SortFunc(eqIdxs, func(a, b int) int {
		if c := cmp.Compare(len(wRows[a].cols), len(wRows[b].cols)); c != 0 {
			return c
		}
		return cmp.Compare(a, b)
	})
	isPivCol := make([]bool, n)
	basis := make([]cbcEqBasisRow, 0, min(len(eqIdxs), 64))
	for _, rIdx := range eqIdxs {
		curCols := slices.Clone(wRows[rIdx].cols)
		curVals := slices.Clone(wRows[rIdx].vals)
		curRHS := wRows[rIdx].rhs
		combRows := []int{rIdx}
		combW := []float64{1.0}
		for bIdx := range basis {
			b := &basis[bIdx]
			pos, found := slices.BinarySearch(curCols, b.pivCol)
			if !found {
				continue
			}
			alpha := curVals[pos] / b.pivVal
			if math.IsNaN(alpha) || math.IsInf(alpha, 0) || alpha == 0 {
				continue
			}
			mCols, mVals, mRHS, mOK := mergeRowLinearComb(curCols, curVals, curRHS, b.cols, b.vals, b.rhs, alpha)
			if !mOK || len(mCols) > cbcPrecondMaxEqRowLen {
				continue
			}
			cRows, cW, cOK := mergeCombWeights(combRows, combW, b.combRows, b.combWeights, alpha)
			if !cOK {
				continue
			}
			curCols, curVals, curRHS = mCols, mVals, mRHS
			combRows, combW = cRows, cW
		}
		pivPos := -1
		bestScore := 0.0
		for k, c := range curCols {
			if uint(c) >= uint(n) || isPivCol[c] {
				continue
			}
			av := math.Abs(curVals[k])
			if av < cbcPrecondMinPivot {
				continue
			}
			score := av * colFiniteScale(scaleLB[c], scaleUB[c])
			if math.IsInf(score, 1) {
				score = math.MaxFloat64
			}
			if isPow2OrZero(av) && score <= 0.25*math.MaxFloat64 {
				score *= 4.0
			}
			if score > bestScore {
				bestScore = score
				pivPos = k
			}
		}
		if pivPos < 0 {
			continue
		}
		pivCol := curCols[pivPos]
		pivVal := curVals[pivPos]
		for bIdx := range basis {
			b := &basis[bIdx]
			pos, found := slices.BinarySearch(b.cols, pivCol)
			if !found {
				continue
			}
			alpha := b.vals[pos] / pivVal
			if math.IsNaN(alpha) || math.IsInf(alpha, 0) || alpha == 0 {
				continue
			}
			mCols, mVals, mRHS, mOK := mergeRowLinearComb(b.cols, b.vals, b.rhs, curCols, curVals, curRHS, alpha)
			if !mOK || len(mCols) > cbcPrecondMaxEqRowLen {
				continue
			}
			cRows, cW, cOK := mergeCombWeights(b.combRows, b.combWeights, combRows, combW, alpha)
			if !cOK {
				continue
			}
			pIdx, stillThere := slices.BinarySearch(mCols, b.pivCol)
			if !stillThere || math.Abs(mVals[pIdx]) < cbcPrecondMinPivot {
				continue
			}
			b.cols, b.vals, b.rhs = mCols, mVals, mRHS
			b.pivVal = mVals[pIdx]
			b.combRows, b.combWeights = cRows, cW
		}
		isPivCol[pivCol] = true
		basis = append(basis, cbcEqBasisRow{
			pivCol:      pivCol,
			pivVal:      pivVal,
			cols:        curCols,
			vals:        curVals,
			rhs:         curRHS,
			combRows:    combRows,
			combWeights: combW,
		})
	}
	return basis
}

func evalShiftedConstMin(fixedW *wideDotAcc, baseConstMin float64) float64 {
	if fixedW == nil || (fixedW.isEmpty() && !fixedW.nonFin) {
		return baseConstMin
	}
	w := *fixedW
	w.addVal(baseConstMin)
	return w.finish(math.NaN())
}

func seedWideDotAcc(w *wideDotAcc, hi, mid, lo []float64) {
	seedPart := func(buf *[expansionCap]float64, n *int, vals []float64) bool {
		if len(vals) <= expansionCap {
			*n = copy(buf[:], vals)
			return true
		}
		for _, v := range vals {
			var ok, capped bool
			*n, ok, capped = growExpansionBuf(buf, *n, v)
			if capped {
				w.capped = true
			}
			if !ok {
				w.nonFin = true
				return false
			}
		}
		return true
	}
	if !seedPart(&w.hiBuf, &w.hiLen, hi) || !seedPart(&w.midBuf, &w.midLen, mid) {
		return
	}
	seedPart(&w.loBuf, &w.loLen, lo)
}

func rowBoundsFromSense(sense ConstraintSense, rhs float64) (rLB, rUB float64) {
	switch sense {
	case LessEqual:
		return math.Inf(-1), rhs
	case GreaterEqual:
		return rhs, math.Inf(1)
	default:
		return rhs, rhs
	}
}

func eliminateFixedColsInRow(origCols []int, origVals []float64, origRHS float64, anyFixed bool, fixedCols []bool, workLB []float64, n int) (cols []int, vals []float64, rhs float64, changed bool) {
	hasFix := false
	if anyFixed {
		for _, col := range origCols {
			if uint(col) < uint(n) && fixedCols[col] {
				hasFix = true
				break
			}
		}
	}
	if hasFix {
		var w wideDotAcc
		w.addVal(origRHS)
		remCount := 0
		for k, col := range origCols {
			if uint(col) >= uint(n) {
				continue
			}
			if fixedCols[col] {
				w.addProd(-origVals[k], workLB[col])
			} else {
				remCount++
			}
		}
		top, rem, ok := w.finishDD()
		if ok && math.Abs(top) < 1e29 && math.Abs(rem) <= cbcPrecondRemTol {
			newCols := make([]int, 0, remCount)
			newVals := make([]float64, 0, remCount)
			for k, col := range origCols {
				if uint(col) < uint(n) && !fixedCols[col] {
					newCols = append(newCols, col)
					newVals = append(newVals, origVals[k])
				}
			}
			return newCols, newVals, top, true
		}
	}
	return slices.Clone(origCols), slices.Clone(origVals), origRHS, false
}

func hasWorkCancelRisk(
	p *CompiledProblem,
	workObjMin []float64,
	curConstMin float64,
	wRows []cbcWorkRow,
	wCuts cbcCutSet,
	effLB, effUB []float64,
	threshold, infScale float64,
) bool {
	if denseObjHasCancelRisk(workObjMin, curConstMin, effLB, effUB, p.varCat, threshold, infScale) {
		return true
	}
	for i := range wRows {
		rLB, rUB := rowBoundsFromSense(wRows[i].sense, wRows[i].rhs)
		if sparseExprHasCancelRisk(wRows[i].cols, wRows[i].vals, effLB, effUB, p.varCat, rLB, rUB, threshold, infScale) {
			return true
		}
	}
	for e := range wCuts.count() {
		if sparseExprHasCancelRisk(wCuts.cols[e], wCuts.vals[e], effLB, effUB, p.varCat, math.Inf(-1), wCuts.rhs[e], threshold, infScale) {
			return true
		}
	}
	return false
}

func buildEqBasisDualFixups(b *cbcEqBasisRow, isObj bool, dstRow int, mu float64) ([]cbcDualFixup, bool) {
	stepFixups := make([]cbcDualFixup, 0, len(b.combRows))
	for t, eqR := range b.combRows {
		effMu := mu * b.combWeights[t]
		if math.IsNaN(effMu) || math.IsInf(effMu, 0) {
			return nil, false
		}
		if effMu != 0 {
			stepFixups = append(stepFixups, cbcDualFixup{isObj: isObj, eqRow: eqR, dstRow: dstRow, mu: effMu})
		}
	}
	return stepFixups, true
}

func reduceSparseRowByEqBasis(
	cols []int,
	vals []float64,
	rhs float64,
	dstRow int,
	skipSelfRow int,
	eqBasis []cbcEqBasisRow,
	effLB, effUB []float64,
) (newCols []int, newVals []float64, newRHS float64, fixups []cbcDualFixup, reduced bool) {
	oldNorm := sparseRowPotentialNorm(cols, vals, effLB, effUB, rhs)
	candCols := cols
	candVals := vals
	candRHS := rhs
	var candFixups []cbcDualFixup
	for bIdx := range eqBasis {
		b := &eqBasis[bIdx]
		if skipSelfRow >= 0 && slices.Contains(b.combRows, skipSelfRow) {
			continue
		}
		pos, found := slices.BinarySearch(candCols, b.pivCol)
		if !found {
			continue
		}
		mu := candVals[pos] / b.pivVal
		if math.IsNaN(mu) || math.IsInf(mu, 0) || mu == 0 {
			continue
		}
		mCols, mVals, mRHS, mOK := mergeRowLinearComb(candCols, candVals, candRHS, b.cols, b.vals, b.rhs, mu)
		if !mOK {
			continue
		}
		stepFixups, valid := buildEqBasisDualFixups(b, false, dstRow, mu)
		if !valid {
			continue
		}
		candCols, candVals, candRHS = mCols, mVals, mRHS
		candFixups = append(candFixups, stepFixups...)
	}
	if len(candFixups) > 0 && sparseRowPotentialNorm(candCols, candVals, effLB, effUB, candRHS) <= cbcPrecondNormShrinkRatio*oldNorm {
		return candCols, candVals, candRHS, candFixups, true
	}
	return cols, vals, rhs, nil, false
}

// preconditionCBC eliminates IEEE-754 cancellation from fixed columns, continuous implied-fixed
// MIP domains, and equality-coupled rows or objectives using Shewchuk expansions before passing
// the reduced matrix to COIN-OR CLP or CBC.
func preconditionCBC(
	p *CompiledProblem,
	lb, ub []float64,
	cuts cbcCutSet,
	isMIP bool,
	deadline time.Time,
	done <-chan struct{},
) cbcPrecond {
	n := p.numVars
	baseM := p.numConstraints
	numExtra := cuts.count()
	baseConstMin := p.objConstMin()

	scaleLB := slices.Clone(lb[:n])
	scaleUB := slices.Clone(ub[:n])
	var varCat []VarCategory
	if isMIP {
		varCat = p.varCat
	}
	infeas, timedOut := tightenContinuousBoundsCBC(p, varCat, cuts, scaleLB, scaleUB, cbcPrecondMaxTightenPasses, deadline, done)
	if timedOut {
		return cbcPrecond{timedOut: true}
	}
	if infeas {
		return cbcPrecond{infeasible: true}
	}

	var workLB, workUB, effLB, effUB []float64
	if isMIP {
		workLB = scaleLB
		workUB = scaleUB
		effLB = scaleLB
		effUB = scaleUB
		for j := range n {
			if p.varCat[j] != Continuous || math.IsInf(workLB[j], -1) || math.IsInf(workUB[j], 1) {
				continue
			}
			span := workUB[j] - workLB[j]
			if span > 0 && span <= 1e-7 && math.Abs(p.objMin[j])*span > 1e-6 {
				return cbcPrecond{unresolved: true}
			}
		}
	} else {
		workLB = slices.Clone(lb[:n])
		workUB = slices.Clone(ub[:n])
		effLB = make([]float64, n)
		effUB = make([]float64, n)
		for j := range n {
			if !math.IsInf(workLB[j], 0) {
				effLB[j] = workLB[j]
			} else {
				effLB[j] = scaleLB[j]
			}
			if !math.IsInf(workUB[j], 0) {
				effUB[j] = workUB[j]
			} else {
				effUB[j] = scaleUB[j]
			}
		}
	}

	infScale, _ := problemImpliedScaleHint(p, scaleLB, scaleUB, cuts)

	colHasTiny := make([]bool, n)
	for j := range n {
		if ao := math.Abs(p.objMin[j]); ao > 0 && ao <= cbcPrecondMinSafeCoeff {
			colHasTiny[j] = true
			continue
		}
		for k := p.colPtr[j]; k < p.colPtr[j+1]; k++ {
			if av := math.Abs(p.colVal[k]); av > 0 && av <= cbcPrecondMinSafeCoeff {
				colHasTiny[j] = true
				break
			}
		}
	}
	for e := range numExtra {
		for k := range min(len(cuts.cols[e]), len(cuts.vals[e])) {
			c := cuts.cols[e][k]
			if uint(c) < uint(n) {
				if av := math.Abs(cuts.vals[e][k]); av > 0 && av <= cbcPrecondMinSafeCoeff {
					colHasTiny[c] = true
				}
			}
		}
	}

	fixedCols := make([]bool, n)
	anyFixed := false
	for j := range n {
		if workLB[j] == workUB[j] && !math.IsInf(workLB[j], 0) && !colHasTiny[j] {
			fixedCols[j] = true
			anyFixed = true
		}
	}

	wRows := make([]cbcWorkRow, baseM)
	modified := false
	for i := range baseM {
		st, en := p.rowPtr[i], p.rowPtr[i+1]
		rCols, rVals, rRHS, chg := eliminateFixedColsInRow(p.colIdx[st:en], p.rowVal[st:en], p.rowRHS[i], anyFixed, fixedCols, workLB, n)
		wRows[i] = cbcWorkRow{sense: p.rowSense[i], rhs: rRHS, cols: rCols, vals: rVals}
		if chg {
			modified = true
		}
	}

	wCuts := cbcCutSet{
		cols: make([][]int, numExtra),
		vals: make([][]float64, numExtra),
		rhs:  make([]float64, numExtra),
	}
	for e := range numExtra {
		kLim := min(len(cuts.cols[e]), len(cuts.vals[e]))
		cCols, cVals, cRHS, chg := eliminateFixedColsInRow(cuts.cols[e][:kLim], cuts.vals[e][:kLim], cuts.rhs[e], anyFixed, fixedCols, workLB, n)
		wCuts.cols[e], wCuts.vals[e], wCuts.rhs[e] = cCols, cVals, cRHS
		if chg {
			modified = true
		}
	}

	workObjMin := slices.Clone(p.objMin)
	var fixedObjW wideDotAcc
	hasFixedObj := false
	if anyFixed && denseObjHasCancelRisk(workObjMin, baseConstMin, effLB, effUB, p.varCat, cbcCancelMagThreshold, infScale) {
		for j := range n {
			if fixedCols[j] && workObjMin[j] != 0 {
				fixedObjW.addProd(workObjMin[j], workLB[j])
				workObjMin[j] = 0
				hasFixedObj = true
				modified = true
			}
		}
	}

	curConstMin := evalShiftedConstMin(&fixedObjW, baseConstMin)
	needEqBasis := hasWorkCancelRisk(p, workObjMin, curConstMin, wRows, wCuts, effLB, effUB, cbcCancelMagThreshold, infScale)

	var dualFixups []cbcDualFixup
	if needEqBasis {
		cancelEqRow := make([]bool, baseM)
		anyCancelEq := false
		for rIdx := range baseM {
			if wRows[rIdx].sense != Equal {
				continue
			}
			rLB, rUB := rowBoundsFromSense(Equal, wRows[rIdx].rhs)
			if sparseExprHasCancelRisk(wRows[rIdx].cols, wRows[rIdx].vals, effLB, effUB, p.varCat, rLB, rUB, cbcCancelMagThreshold, infScale) {
				cancelEqRow[rIdx] = true
				anyCancelEq = true
			}
		}
		reduceWorkRow := func(rIdx int, eqBasis []cbcEqBasisRow) {
			rCols, rVals, rRHS, fxs, ok := reduceSparseRowByEqBasis(wRows[rIdx].cols, wRows[rIdx].vals, wRows[rIdx].rhs, rIdx, rIdx, eqBasis, effLB, effUB)
			if ok {
				wRows[rIdx].cols, wRows[rIdx].vals, wRows[rIdx].rhs = rCols, rVals, rRHS
				dualFixups = append(dualFixups, fxs...)
				modified = true
			}
		}

		if anyCancelEq {
			cleanBasis := buildCBCEqRREFBasis(wRows, cancelEqRow, effLB, effUB, n)
			if len(cleanBasis) > 0 {
				for rIdx := range baseM {
					if cancelEqRow[rIdx] {
						reduceWorkRow(rIdx, cleanBasis)
					}
				}
			}
		}

		eqBasis := buildCBCEqRREFBasis(wRows, nil, effLB, effUB, n)
		if len(eqBasis) > 0 {
			if denseObjHasCancelRisk(workObjMin, curConstMin, effLB, effUB, p.varCat, cbcCancelMagThreshold, infScale) {
				oldNorm := denseObjPotentialNorm(workObjMin, curConstMin, effLB, effUB)
				candObj := slices.Clone(workObjMin)
				candFixedW := fixedObjW
				candHasFixed := hasFixedObj
				var candFixups []cbcDualFixup
				for bIdx := range eqBasis {
					b := &eqBasis[bIdx]
					cVal := candObj[b.pivCol]
					if cVal == 0 {
						continue
					}
					mu := cVal / b.pivVal
					if math.IsNaN(mu) || math.IsInf(mu, 0) || mu == 0 {
						continue
					}
					if b.rhs != 0 {
						if pProd, _ := twoProd(mu, b.rhs); math.IsNaN(pProd) || math.IsInf(pProd, 0) {
							continue
						}
					}
					stepVals := make([]float64, len(b.cols))
					valid := true
					for k, c := range b.cols {
						var w wideDotAcc
						w.addVal(candObj[c])
						w.addProd(-mu, b.vals[k])
						vTop, vRem, vOK := w.finishDD()
						if !vOK || math.Abs(vRem) != 0 || (vTop != 0 && math.Abs(vTop) <= cbcPrecondMinSafeCoeff) {
							valid = false
							break
						}
						stepVals[k] = vTop
					}
					if !valid {
						continue
					}
					stepFixups, okFx := buildEqBasisDualFixups(b, true, 0, mu)
					if !okFx {
						continue
					}
					for k, c := range b.cols {
						candObj[c] = stepVals[k]
					}
					if b.rhs != 0 {
						candFixedW.addProd(mu, b.rhs)
						candHasFixed = true
					}
					candFixups = append(candFixups, stepFixups...)
				}
				if len(candFixups) > 0 {
					candConstMin := evalShiftedConstMin(&candFixedW, baseConstMin)
					if !math.IsNaN(candConstMin) && !math.IsInf(candConstMin, 0) &&
						denseObjPotentialNorm(candObj, candConstMin, effLB, effUB) <= cbcPrecondNormShrinkRatio*oldNorm {
						workObjMin = candObj
						fixedObjW = candFixedW
						hasFixedObj = candHasFixed
						dualFixups = append(dualFixups, candFixups...)
						modified = true
					}
				}
			}

			for rIdx := range baseM {
				if wRows[rIdx].sense == Equal {
					continue
				}
				rLB, rUB := rowBoundsFromSense(wRows[rIdx].sense, wRows[rIdx].rhs)
				if !sparseExprHasCancelRisk(wRows[rIdx].cols, wRows[rIdx].vals, effLB, effUB, p.varCat, rLB, rUB, cbcCancelMagThreshold, infScale) {
					continue
				}
				reduceWorkRow(rIdx, eqBasis)
			}

			for e := range numExtra {
				if !sparseExprHasCancelRisk(wCuts.cols[e], wCuts.vals[e], effLB, effUB, p.varCat, math.Inf(-1), wCuts.rhs[e], cbcCancelMagThreshold, infScale) {
					continue
				}
				cCols, cVals, cRHS, fxs, ok := reduceSparseRowByEqBasis(wCuts.cols[e], wCuts.vals[e], wCuts.rhs[e], baseM+e, -1, eqBasis, effLB, effUB)
				if ok {
					wCuts.cols[e], wCuts.vals[e], wCuts.rhs[e] = cCols, cVals, cRHS
					dualFixups = append(dualFixups, fxs...)
					modified = true
				}
			}
		}
	}

	if isMIP && !modified {
		for j := range n {
			if workLB[j] != lb[j] || workUB[j] != ub[j] {
				modified = true
				break
			}
		}
	}

	curConstMin = evalShiftedConstMin(&fixedObjW, baseConstMin)
	unresolved := hasWorkCancelRisk(p, workObjMin, curConstMin, wRows, wCuts, effLB, effUB, cbcUnresolvedCancelFloor, infScale)

	if !modified {
		return cbcPrecond{unresolved: unresolved}
	}

	nnz := 0
	for i := range baseM {
		nnz += len(wRows[i].cols)
	}
	if nnz > math.MaxInt32 {
		return cbcPrecond{unresolved: true}
	}

	colPtr := make([]int, n+1)
	for i := range baseM {
		for _, col := range wRows[i].cols {
			colPtr[col+1]++
		}
	}
	colPtr32 := make([]int32, n+1)
	for j := range n {
		colPtr[j+1] += colPtr[j]
		colPtr32[j+1] = int32(colPtr[j+1])
	}
	rowIdx32 := make([]int32, nnz)
	colVal := make([]float64, nnz)
	writePos := slices.Clone(colPtr[:n])
	for i := range baseM {
		for k, col := range wRows[i].cols {
			pos := writePos[col]
			rowIdx32[pos] = int32(i)
			colVal[pos] = wRows[i].vals[k]
			writePos[col]++
		}
	}

	colLBClp := make([]float64, n)
	colUBClp := make([]float64, n)
	for j := range n {
		colLBClp[j] = clampCBCBound(workLB[j])
		colUBClp[j] = clampCBCBound(workUB[j])
	}
	rowLBClp := make([]float64, baseM)
	rowUBClp := make([]float64, baseM)
	for i := range baseM {
		rLB, rUB := rowBoundsFromSense(wRows[i].sense, wRows[i].rhs)
		rowLBClp[i] = clampCBCBound(rLB)
		rowUBClp[i] = clampCBCBound(rUB)
	}

	var fixedHi, fixedMid, fixedLo []float64
	fixedNonFin := false
	if hasFixedObj {
		if fixedObjW.nonFin {
			fixedNonFin = true
		} else if !fixedObjW.isEmpty() {
			fixedObjW.hiLen = collapseExpansionBuf(&fixedObjW.hiBuf, fixedObjW.hiLen)
			fixedObjW.midLen = collapseExpansionBuf(&fixedObjW.midBuf, fixedObjW.midLen)
			fixedObjW.loLen = collapseExpansionBuf(&fixedObjW.loBuf, fixedObjW.loLen)
			fixedHi = slices.Clone(fixedObjW.hiBuf[:fixedObjW.hiLen])
			fixedMid = slices.Clone(fixedObjW.midBuf[:fixedObjW.midLen])
			fixedLo = slices.Clone(fixedObjW.loBuf[:fixedObjW.loLen])
		}
	}

	return cbcPrecond{
		active:         true,
		unresolved:     unresolved,
		colPtr32:       colPtr32,
		rowIdx32:       rowIdx32,
		colVal:         colVal,
		colLBClp:       colLBClp,
		colUBClp:       colUBClp,
		workLB:         workLB,
		objMin:         workObjMin,
		rowLBClp:       rowLBClp,
		rowUBClp:       rowUBClp,
		cuts:           wCuts,
		fixedCols:      fixedCols,
		fixedHi:        fixedHi,
		fixedMid:       fixedMid,
		fixedLo:        fixedLo,
		fixedObjNonFin: fixedNonFin,
		hasFixedObj:    hasFixedObj,
		dualFixups:     dualFixups,
	}
}

func (pc *cbcPrecond) addFixedObjOffset(val float64) float64 {
	if !pc.hasFixedObj {
		return val
	}
	if pc.fixedObjNonFin || math.IsNaN(val) || math.IsInf(val, 0) {
		return math.NaN()
	}
	var w wideDotAcc
	seedWideDotAcc(&w, pc.fixedHi, pc.fixedMid, pc.fixedLo)
	w.addVal(val)
	return w.finish(math.NaN())
}

func (pc *cbcPrecond) totalMinShift(p *CompiledProblem) float64 {
	baseConst := p.objConstMin()
	if !pc.active {
		return baseConst
	}
	return pc.addFixedObjOffset(baseConst)
}

func (pc *cbcPrecond) userBoundFromReducedMin(p *CompiledProblem, redMin, clampFullMin float64, hasValidSol bool) (float64, bool) {
	if math.IsNaN(redMin) || math.IsInf(redMin, 0) {
		return 0, false
	}
	baseConst := p.objConstMin()
	var fullMin float64
	if pc.active && pc.hasFixedObj {
		if pc.fixedObjNonFin || math.IsNaN(baseConst) || math.IsInf(baseConst, 0) {
			return 0, false
		}
		var w wideDotAcc
		seedWideDotAcc(&w, pc.fixedHi, pc.fixedMid, pc.fixedLo)
		w.addVal(baseConst)
		w.addVal(redMin)
		fullMin = w.finish(math.NaN())
	} else if baseConst != 0 {
		if math.IsNaN(baseConst) || math.IsInf(baseConst, 0) {
			return 0, false
		}
		var w wideDotAcc
		w.addVal(baseConst)
		w.addVal(redMin)
		fullMin = w.finish(math.NaN())
	} else {
		fullMin = redMin
	}
	if math.IsNaN(fullMin) || math.IsInf(fullMin, 0) {
		return 0, false
	}
	if hasValidSol && !math.IsNaN(clampFullMin) && !math.IsInf(clampFullMin, 0) && fullMin > clampFullMin {
		fullMin = clampFullMin
	}
	return p.userObjFromShiftedMin(fullMin), true
}

func (pc *cbcPrecond) restoreLPPrimalDuals(p *CompiledProblem, cuts cbcCutSet, primal, duals, rc []float64) {
	n := p.numVars
	baseM := p.numConstraints
	numExtra := cuts.colValCount()
	for j, fixed := range pc.fixedCols {
		if fixed && j < len(primal) {
			primal[j] = pc.workLB[j]
		}
	}
	for _, fx := range slices.Backward(pc.dualFixups) {
		if fx.eqRow < 0 || fx.eqRow >= len(duals) {
			continue
		}
		var w wideDotAcc
		w.addVal(duals[fx.eqRow])
		if fx.isObj {
			w.addVal(fx.mu)
		} else if fx.dstRow >= 0 && fx.dstRow < len(duals) {
			w.addProd(-fx.mu, duals[fx.dstRow])
		}
		duals[fx.eqRow] = w.finish(duals[fx.eqRow])
	}
	var colRows []int
	var colVals []float64
	for j, fixed := range pc.fixedCols {
		if !fixed || j >= n || j >= len(rc) {
			continue
		}
		st, en := p.colPtr[j], p.colPtr[j+1]
		colRows = append(colRows[:0], p.rowIdx[st:en]...)
		colVals = append(colVals[:0], p.colVal[st:en]...)
		for e := range numExtra {
			for k := range min(len(cuts.cols[e]), len(cuts.vals[e])) {
				if cuts.cols[e][k] == j {
					colRows = append(colRows, baseM+e)
					colVals = append(colVals, cuts.vals[e][k])
					break
				}
			}
		}
		if v, _, _, _, ok := evalColDualDot(p.objMin[j], colRows, colVals, duals, nil, nil); ok {
			rc[j] = v
		}
	}
}
