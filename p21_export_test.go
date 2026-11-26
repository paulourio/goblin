package goblin

import (
	"errors"
	"fmt"
	"math"
	"slices"
)

// PrimalDevexDiagForTest captures internal Devex and simplex diagnostics from a forced primal solve.
type PrimalDevexDiagForTest struct {
	Status           Status
	Iterations       int
	DevexResets      int
	DevexActiveAfter bool
	PrimalFresh      bool
	DualFresh        bool
}

// RunForcedPrimalForTest runs solveTwoPhasePrimalWithLimit directly on p with optional Devex
// disabling and scaling toggle, reusing ws when non-nil.
func RunForcedPrimalForTest(
	p *CompiledProblem,
	ws *Workspace,
	disableDevex bool,
	useScaling bool,
	maxIter int,
) (*LPSolver, LPResult, PrimalDevexDiagForTest) {
	s := NewLPSolver(p, ws)
	if !useScaling && s.scaled {
		s.unscaleToOriginal()
	}
	s.disableDevex = disableDevex
	s.MaxIter = maxIter
	s.lastPrimalFallback = true
	st, iters, pFresh, dFresh := s.solveTwoPhasePrimalWithLimit(maxIter)
	diag := PrimalDevexDiagForTest{
		Status:           st,
		Iterations:       iters,
		DevexResets:      s.devexResets,
		DevexActiveAfter: s.devexActive,
		PrimalFresh:      pFresh,
		DualFresh:        dFresh,
	}
	if st != StatusOptimal {
		return s, LPResult{Status: st, Iterations: iters}, diag
	}
	s.dualValid = true
	res, ok := s.finalizeOptimalResult(iters, pFresh, dFresh)
	if !ok {
		diag.Status = StatusNumericalError
		return s, LPResult{Status: StatusNumericalError, Iterations: iters}, diag
	}
	return s, res, diag
}

// VerifyPrimalIncrementalStepsForTest steps through primalSimplexStep in Phase I and Phase II,
// checking incremental dual/reduced-cost updates against fresh BTRAN recomputation, Phase I
// sign-change invalidation, bound-flip preservation, Devex weight bounds, and rectifier resets.
func VerifyPrimalIncrementalStepsForTest(
	p *CompiledProblem,
	maxSteps int,
	disableDevex bool,
	useScaling bool,
	injectRectifierMask uint8,
) error {
	s := NewLPSolver(p, nil)
	defer s.Close()
	if !useScaling && s.scaled {
		s.unscaleToOriginal()
	}
	s.disableDevex = disableDevex

	ws := s.ws
	m := s.TotalRows()
	tot := ws.totVar
	if m <= 0 || tot <= 0 {
		return nil
	}
	if err := s.factorizeBasis(); err != nil {
		s.reinitCrashKeepingBounds()
		if err2 := s.factorizeBasis(); err2 != nil {
			return nil
		}
	}
	s.computePrimalValues()
	s.syncBasicBounds()
	if !disableDevex {
		s.resetDevexFramework(false)
	} else {
		s.clearDevexState()
	}

	checkWeights := func(step int) error {
		if !s.devexActive {
			return nil
		}
		if len(ws.devexWeight) < tot || len(ws.devexRef) < tot {
			return fmt.Errorf("step %d: short devex buffers w=%d ref=%d tot=%d",
				step, len(ws.devexWeight), len(ws.devexRef), tot)
		}
		for k := range tot {
			w := ws.devexWeight[k]
			if math.IsNaN(w) || math.IsInf(w, 0) || w < minDevexWeight || w > maxDevexWeight {
				return fmt.Errorf("step %d: invalid devexWeight[%d]=%g", step, k, w)
			}
			if ws.devexRef[k] > 1 {
				return fmt.Errorf("step %d: invalid devexRef[%d]=%d", step, k, ws.devexRef[k])
			}
		}
		return nil
	}

	compareDuals := func(step int, phase1 bool, cost []float64) error {
		gotY := slices.Clone(ws.y[:m])
		gotDJ := slices.Clone(ws.dj[:tot])
		s.computeDualsAndReducedCosts(cost)
		maxY := 1.0
		for _, v := range ws.y[:m] {
			if av := math.Abs(v); av > maxY {
				maxY = av
			}
		}
		maxDJ := 1.0
		for _, v := range ws.dj[:tot] {
			if av := math.Abs(v); av > maxDJ {
				maxDJ = av
			}
		}
		// Skip strict comparison only when the current working basis has extreme dual magnitude
		// where float64 cancellation legitimately exceeds incremental tolerance.
		if maxY > 1e7 || maxDJ > 1e7 || math.IsNaN(maxY) || math.IsInf(maxY, 0) {
			return nil
		}
		tolY := 1e-5 * maxY
		tolDJ := 1e-5 * maxDJ
		for i, want := range ws.y[:m] {
			if math.IsNaN(gotY[i]) || math.Abs(gotY[i]-want) > tolY {
				return fmt.Errorf("step %d (phase1=%v): incremental y[%d]=%.12g != fresh %.12g (tol=%.2e)",
					step, phase1, i, gotY[i], want, tolY)
			}
		}
		for k, want := range ws.dj[:tot] {
			if math.IsNaN(gotDJ[k]) || math.Abs(gotDJ[k]-want) > tolDJ {
				return fmt.Errorf("step %d (phase1=%v): incremental dj[%d]=%.12g != fresh %.12g (tol=%.2e)",
					step, phase1, k, gotDJ[k], want, tolDJ)
			}
		}
		copy(ws.y[:m], gotY)
		copy(ws.dj[:tot], gotDJ)
		return nil
	}

	dualsValid := false
	dualsFresh := false
	prevPhase1 := false
	hasPrevPhase := false
	for step := range maxSteps {
		if ws.lu.NeedsRefactor() {
			if err := s.factorizeBasis(); err != nil {
				return nil
			}
			s.computePrimalValues()
			dualsValid = false
			dualsFresh = false
		}
		clear(ws.workCost)
		anyInfeas := false
		for i := range m {
			if c := phase1VarCost(ws.xb[i], ws.basicLB[i], ws.basicUB[i], ws.basicTol[i]); c != 0 {
				ws.workCost[ws.basis[i]] = c
				anyInfeas = true
			}
		}
		phase1 := anyInfeas
		if hasPrevPhase && phase1 != prevPhase1 {
			dualsValid = false
			dualsFresh = false
		}
		prevPhase1 = phase1
		hasPrevPhase = true
		cost := ws.cost
		if phase1 {
			cost = ws.workCost
		}
		if !dualsValid {
			s.computeDualsAndReducedCosts(cost)
			dualsFresh = true
		}

		// Optionally perturb Devex state before selected steps to cover rectifier reset branches.
		if s.devexActive && injectRectifierMask != 0 && step > 0 {
			switch (step + int(injectRectifierMask)) % 7 {
			case 0:
				s.devexPivotsSinceReset = max(minDevexPivotLimit, 2*m)
			case 1:
				if enterVar, _, hasNaN := s.pricePrimalEnter(phase1, false); !hasNaN && enterVar >= 0 && enterVar < tot {
					ws.devexWeight[enterVar] = 1e6
				}
			}
		}

		prevBasis := slices.Clone(ws.basis[:m])
		prevPivots := s.devexPivotsSinceReset
		prevResets := s.devexResets
		var prevWorkCost []float64
		if phase1 {
			prevWorkCost = slices.Clone(ws.workCost[:tot])
		}

		st, _, nextValid, nextFresh := s.primalSimplexStep(phase1, false, dualsFresh)
		if err := checkWeights(step); err != nil {
			return err
		}
		if st != StatusNotSolved {
			return nil
		}

		// Detect whether a basis exchange happened or a pure bound flip occurred.
		changedSlot := -1
		for i := range m {
			if ws.basis[i] != prevBasis[i] {
				changedSlot = i
				break
			}
		}
		if changedSlot == -1 && nextValid && s.devexActive {
			// Bound flip must not advance devexPivotsSinceReset or reset Devex.
			if s.devexPivotsSinceReset != prevPivots || s.devexResets != prevResets {
				return fmt.Errorf("step %d: bound flip altered Devex counters (pivots %d->%d, resets %d->%d)",
					step, prevPivots, s.devexPivotsSinceReset, prevResets, s.devexResets)
			}
		}
		if phase1 && nextValid {
			// Verify no surviving basic variable changed Phase I cost sign while dualsValid remained true.
			for i := range m {
				bVar := ws.basis[i]
				if i == changedSlot {
					if c := phase1VarCost(ws.xb[i], ws.basicLB[i], ws.basicUB[i], ws.basicTol[i]); c != 0 {
						return fmt.Errorf("step %d: entering Phase I var %d infeasible (c=%g) with dualsValid=true", step, bVar, c)
					}
					continue
				}
				if c := phase1VarCost(ws.xb[i], ws.basicLB[i], ws.basicUB[i], ws.basicTol[i]); c != prevWorkCost[bVar] {
					return fmt.Errorf("step %d: Phase I basic var %d changed cost %g->%g with dualsValid=true",
						step, bVar, prevWorkCost[bVar], c)
				}
			}
		}
		if nextValid {
			checkCost := ws.cost
			if phase1 {
				checkCost = make([]float64, tot)
				for i := range m {
					checkCost[ws.basis[i]] = phase1VarCost(ws.xb[i], ws.basicLB[i], ws.basicUB[i], ws.basicTol[i])
				}
			}
			if err := compareDuals(step, phase1, checkCost); err != nil {
				return err
			}
		}
		dualsValid = nextValid
		dualsFresh = nextFresh
	}
	return nil
}

// VerifyPrimalTabRowDriftGuardForTest corrupts an updated LU factor or invokes a perturbed
// pivot check to verify primalSimplexStep refactors cleanly when |tabRow[enterVar]-piv| drifts.
func VerifyPrimalTabRowDriftGuardForTest(p *CompiledProblem) error {
	s := NewLPSolver(p, nil)
	defer s.Close()
	ws := s.ws
	m := s.TotalRows()
	if m < 2 {
		return nil
	}
	if err := s.factorizeBasis(); err != nil {
		return nil
	}
	s.computePrimalValues()
	s.syncBasicBounds()
	s.resetDevexFramework(false)
	s.computeDualsAndReducedCosts(ws.cost)

	// Perform one clean step so ws.lu.numUpdates > 0 when a basis pivot occurs.
	st1, _, _, _ := s.primalSimplexStep(false, false, true)
	if st1 != StatusNotSolved || ws.lu.numUpdates == 0 {
		return nil
	}
	// Inject an asymmetric perturbation into the BTRAN triangular factor while numUpdates > 0
	// so ComputeTableauRow(leaveRow) disagrees with FTRAN fCol[leaveRow], exercising the
	// |tabRow[enterVar]-piv| drift refactor guard on the next step.
	switch {
	case len(ws.lu.uRowVal) > 0:
		ws.lu.uRowVal[0] += 10.0
	case len(ws.lu.lRowVal) > 0:
		ws.lu.lRowVal[0] += 10.0
	case len(ws.lu.uColVal) > 0:
		ws.lu.uColVal[0] += 10.0
	case len(ws.lu.lColVal) > 0:
		ws.lu.lColVal[0] += 10.0
	}
	s.computeDualsAndReducedCosts(ws.cost)
	st2, _, valid2, fresh2 := s.primalSimplexStep(false, false, true)
	if st2 == StatusNotSolved && ws.lu.numUpdates == 0 && (valid2 || fresh2) {
		return fmt.Errorf("drift refactor left dualsValid=%v dualsFresh=%v", valid2, fresh2)
	}
	return nil
}

// ExerciseRelaxCutDynamismForTest invokes relaxCutDynamismTerms and finalizeSparseCut directly
// on synthetic cut vectors and checks that every relaxed cut remains valid on all feasiblePts
// and never mutates caller slices on rejection.
func ExerciseRelaxCutDynamismForTest(
	p *CompiledProblem,
	idx []int,
	val []float64,
	rhs float64,
	lpPrimal []float64,
	feasiblePts [][]float64,
) (relaxed bool, finalized bool, err error) {
	origIdx := slices.Clone(idx)
	origVal := slices.Clone(val)
	gotIdx, gotVal, gotRHS, ok := relaxCutDynamismTerms(p, idx, val, rhs)
	if !ok {
		if !slices.Equal(gotIdx, origIdx) || !slices.Equal(gotVal, origVal) || gotRHS != rhs {
			return false, false, errors.New("rejected relaxCutDynamismTerms mutated inputs")
		}
		return false, false, nil
	}
	if len(gotIdx) == 0 || len(gotIdx) != len(gotVal) || math.IsNaN(gotRHS) || math.IsInf(gotRHS, 0) {
		return true, false, fmt.Errorf("invalid relaxed cut output len=%d rhs=%g", len(gotIdx), gotRHS)
	}

	// Verify relaxation validity: for every point x within [varLB, varUB] satisfying origIdx*origVal <= rhs,
	// the relaxed inequality gotIdx*gotVal <= gotRHS must hold (modulo tiny float64 eval tolerance).
	evalCut := func(cols []int, vals, x []float64) float64 {
		sum := 0.0
		for k, c := range cols {
			sum += vals[k] * x[c]
		}
		return sum
	}
	for ptIdx, x := range feasiblePts {
		if len(x) != p.numVars {
			continue
		}
		origLHS := evalCut(origIdx, origVal, x)
		if origLHS <= rhs+1e-12 {
			relLHS := evalCut(gotIdx, gotVal, x)
			scale := max(1.0, math.Abs(gotRHS), math.Abs(relLHS))
			if relLHS > gotRHS+1e-9*scale {
				return true, false, fmt.Errorf("relaxed cut violated feasible point %d: orig %.17g <= %.17g, relaxed %.17g > %.17g",
					ptIdx, origLHS, rhs, relLHS, gotRHS)
			}
		}
	}

	// Check dynamism cap on kept entries.
	maxAbs := 0.0
	minAbs := math.Inf(1)
	for _, v := range gotVal {
		av := math.Abs(v)
		if av > maxAbs {
			maxAbs = av
		}
		if av > cutDropTol && av < minAbs {
			minAbs = av
		}
	}
	if maxAbs > 0 && minAbs < math.Inf(1) && maxAbs/minAbs > maxCutDynamism*(1+1e-9) {
		return true, false, fmt.Errorf("relaxed cut still exceeds maxCutDynamism: max=%g min=%g ratio=%g",
			maxAbs, minAbs, maxAbs/minAbs)
	}

	if len(lpPrimal) == p.numVars {
		var ws cutGenWorkspace
		cut, finOK := finalizeSparseCut(p, gotIdx, gotVal, gotRHS, lpPrimal, 0, &ws)
		if finOK {
			if len(cut.ColIdx) == 0 || len(cut.ColIdx) != len(cut.Val) || math.IsNaN(cut.RHS) || math.IsInf(cut.RHS, 0) {
				return true, true, errors.New("finalizeSparseCut produced malformed cut")
			}
			for ptIdx, x := range feasiblePts {
				if len(x) != p.numVars {
					continue
				}
				origLHS := evalCut(origIdx, origVal, x)
				if origLHS <= rhs+1e-12 {
					finLHS := evalCut(cut.ColIdx, cut.Val, x)
					scale := max(1.0, math.Abs(cut.RHS), math.Abs(finLHS))
					if finLHS > cut.RHS+1e-7*scale {
						return true, true, fmt.Errorf("finalized relaxed cut violated feasible point %d: %.17g > %.17g",
							ptIdx, finLHS, cut.RHS)
					}
				}
			}
			return true, true, nil
		}
	}
	return true, false, nil
}
