//go:build cgo && cbc

package goblin

/*
#cgo CXXFLAGS: -std=gnu++14 -O3
#cgo !cbc_system CXXFLAGS: -I${SRCDIR}/third_party/Cbc/dist/include/coin-or
#cgo !cbc_system LDFLAGS: -L${SRCDIR}/third_party/Cbc/dist/lib -Wl,-rpath,${SRCDIR}/third_party/Cbc/dist/lib -lOsiCbc -lCbc -lCgl -lOsiClp -lClp -lOsi -lCoinUtils -lbz2 -lz -llapack -lblas -lpthread -lm
#cgo !cbc_system,linux LDFLAGS: -lrt
#cgo cbc_system pkg-config: osi-cbc cbc cgl osi-clp clp osi coinutils
#include "cbc_bridge.h"
*/
import "C"

import (
	"cmp"
	"context"
	"math"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

const (
	cbcDefaultCutPasses = 30
	cbcMaxGapTol        = 1e30
	cbcFeasTol          = 1e-7
	cbcBoundSnapTol     = 1e-6
)

func clampCBCGapTol(tol float64) float64 {
	if math.IsNaN(tol) || tol < 0 {
		return 0
	}
	if math.IsInf(tol, 1) || tol > cbcMaxGapTol {
		return cbcMaxGapTol
	}
	return tol
}

var (
	zeroCInt    C.int
	zeroCDouble C.double

	// Go-level semaphore serializing access to COIN-OR's non-reentrant C++ state
	// so queued callers honor context cancellation and wall-clock deadlines without
	// blocking OS threads inside C++.
	cbcSem = make(chan struct{}, 1)
)

func releaseCBCSlot() {
	<-cbcSem
}

func acquireCBCSlot(ctx context.Context, deadline time.Time, done <-chan struct{}) (func(), Status) {
	if ctx != nil && ctx.Err() != nil {
		return nil, StatusTimeLimit
	}
	if !deadline.IsZero() && !time.Now().Before(deadline) {
		return nil, StatusTimeLimit
	}
	select {
	case cbcSem <- struct{}{}:
		return releaseCBCSlot, StatusOptimal
	default:
	}
	var timer *time.Timer
	var timerC <-chan time.Time
	if !deadline.IsZero() {
		rem := time.Until(deadline)
		if rem <= 0 {
			return nil, StatusTimeLimit
		}
		timer = time.NewTimer(rem)
		defer timer.Stop()
		timerC = timer.C
	}
	select {
	case cbcSem <- struct{}{}:
		return releaseCBCSlot, StatusOptimal
	case <-done:
		return nil, StatusTimeLimit
	case <-timerC:
		return nil, StatusTimeLimit
	}
}

func cInt32Ptr(s []int32) *C.int {
	if len(s) == 0 {
		return &zeroCInt
	}
	return (*C.int)(unsafe.Pointer(&s[0]))
}

func cFloat64Ptr(s []float64) *C.double {
	if len(s) == 0 {
		return &zeroCDouble
	}
	return (*C.double)(unsafe.Pointer(&s[0]))
}

func boolToCInt(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

func clampCInt(v int) C.int {
	if v > math.MaxInt32 {
		return C.int(math.MaxInt32)
	}
	if v < math.MinInt32 {
		return C.int(math.MinInt32)
	}
	return C.int(v)
}

func validateCompiledForCBC(p *CompiledProblem) (*cbcMatrixData, bool) {
	if p == nil || p.invalidErr != nil {
		return nil, false
	}
	n := p.numVars
	if n < 0 || len(p.objMin) != n || len(p.objCoeffs) != n {
		return nil, false
	}
	c := p.ensureCBCData()
	if c == nil || !c.valid {
		return nil, false
	}
	return c, true
}

// CBCAvailable reports whether the optional C++ COIN-OR CBC/CLP backend (-tags cbc with cgo enabled) is compiled in.
func CBCAvailable() bool { return true }

type cbcCutSet struct {
	cols [][]int
	vals [][]float64
	rhs  []float64
}

func (cs cbcCutSet) count() int {
	return min(len(cs.cols), min(len(cs.vals), len(cs.rhs)))
}

func (cs cbcCutSet) colValCount() int {
	return min(len(cs.cols), len(cs.vals))
}

type cbcMatrixView struct {
	colPtr32  []int32
	rowIdx32  []int32
	colVal    []float64
	colLBClp  []float64
	colUBClp  []float64
	objMinClp []float64
	rowLBClp  []float64
	rowUBClp  []float64
}

func selectCBCMatrixView(p *CompiledProblem, cbc *cbcMatrixData, pc *cbcPrecond) cbcMatrixView {
	if pc != nil && pc.active {
		return cbcMatrixView{
			colPtr32:  pc.colPtr32,
			rowIdx32:  pc.rowIdx32,
			colVal:    pc.colVal,
			colLBClp:  pc.colLBClp,
			colUBClp:  pc.colUBClp,
			objMinClp: pc.objMin,
			rowLBClp:  pc.rowLBClp,
			rowUBClp:  pc.rowUBClp,
		}
	}
	return cbcMatrixView{
		colPtr32:  cbc.colPtr32,
		rowIdx32:  cbc.rowIdx32,
		colVal:    p.colVal,
		colLBClp:  cbc.colLBClp,
		colUBClp:  cbc.colUBClp,
		objMinClp: p.objMin,
		rowLBClp:  cbc.rowLBClp,
		rowUBClp:  cbc.rowUBClp,
	}
}

func cbcStatusResult(p *CompiledProblem, status Status, start time.Time, nodes, iters, cuts int) Result {
	bestBound := 0.0
	if isLimitStatus(status) {
		bestBound = trivialDualBound(p.sense)
	}
	return Result{
		modelID:    p.modelID,
		Backend:    BackendCBC,
		Status:     status,
		BestBound:  bestBound,
		Nodes:      nodes,
		Iterations: iters,
		CutsAdded:  cuts,
		SolveTime:  time.Since(start),
	}
}

func withCBCCancel(ctx context.Context, deadline time.Time, done <-chan struct{}, fn func(timeLimitSec float64, cancelPtr *C.int)) (timedOut bool) {
	var cancelFlag C.int
	var cancelPtr *C.int
	timeLimitSec := 0.0
	if !deadline.IsZero() {
		rem := time.Until(deadline)
		if rem <= 0 {
			return true
		}
		timeLimitSec = rem.Seconds()
		cancelPtr = &cancelFlag
		timer := time.AfterFunc(rem, func() {
			atomic.StoreInt32((*int32)(unsafe.Pointer(&cancelFlag)), 1)
		})
		defer timer.Stop()
	}
	if done != nil && ctx != nil {
		cancelPtr = &cancelFlag
		stopCancel := context.AfterFunc(ctx, func() {
			atomic.StoreInt32((*int32)(unsafe.Pointer(&cancelFlag)), 1)
		})
		defer stopCancel()
	}
	fn(timeLimitSec, cancelPtr)
	return false
}

func runBudgetedGoLPProbe(
	ctx context.Context,
	p *CompiledProblem,
	opt Options,
	usedIters int,
	lb, ub []float64,
	deadline time.Time,
	done <-chan struct{},
) (LPResult, bool) {
	if ctx != nil && ctx.Err() != nil {
		return LPResult{}, true
	}
	if stopRequested(deadline, done) {
		return LPResult{Status: StatusTimeLimit}, false
	}
	remIter := 0
	if opt.MaxIter > 0 {
		remIter = opt.MaxIter - usedIters
		if remIter <= 0 {
			return LPResult{Status: StatusIterationLimit}, false
		}
	}
	lpRef := NewLPSolver(p, nil)
	defer lpRef.Close()
	lpRef.DisableSIMD = opt.DisableSIMD
	lpRef.Deadline = deadline
	lpRef.Done = done
	if remIter > 0 {
		lpRef.MaxIter = remIter
	}
	if lb != nil && ub != nil {
		lpRef.SetStructuralBounds(lb, ub)
	}
	refRes := lpRef.Solve()
	if ctx != nil && ctx.Err() != nil {
		return LPResult{}, true
	}
	return refRes, false
}

func remainingCBCMIPBudget(opt Options, usedNodes, usedIters int) (remOpt Options, limitSt Status) {
	remOpt = opt
	if opt.MaxIter > 0 {
		rem := opt.MaxIter - usedIters
		if rem <= 0 {
			return remOpt, StatusIterationLimit
		}
		remOpt.MaxIter = rem
	}
	if !opt.defaultMaxNodesUsed && opt.MaxNodes > 0 {
		remNodes := opt.MaxNodes - usedNodes
		if remNodes <= 0 {
			return remOpt, StatusNodeLimit
		}
		remOpt.MaxNodes = remNodes
	}
	return remOpt, StatusNotSolved
}

type cbcLPFinalizeReq struct {
	p                *CompiledProblem
	lb, ub           []float64
	cuts             cbcCutSet
	status           Status
	primal           []float64
	duals            []float64
	rc               []float64
	totalIters       int
	droppedTiny      bool
	unresolvedCancel bool
	fallback         func(acceptInfeasible bool) (LPResult, bool, bool)
}

func finalizeCBCLP(req *cbcLPFinalizeReq) (res LPResult, fellBack, canceled bool) {
	p := req.p
	lb, ub := req.lb, req.ub
	cuts := req.cuts
	status := req.status
	primal, duals, rc := req.primal, req.duals, req.rc
	totalIters := req.totalIters
	droppedTiny, unresolvedCancel := req.droppedTiny, req.unresolvedCancel
	fallback := req.fallback
	n := p.numVars
	m := p.numConstraints + cuts.count()
	if status == StatusUnbounded && !hasAnyInfiniteBound(lb, ub, n) {
		status = StatusNumericalError
	}
	if status != StatusOptimal {
		if !droppedTiny && status == StatusUnbounded {
			if fbRes, handled, isCanceled := fallback(true); handled {
				return fbRes, !isCanceled, isCanceled
			}
			return LPResult{Status: StatusNumericalError, Iterations: totalIters}, false, false
		}
		if !droppedTiny && status == StatusInfeasible &&
			(unresolvedCancel || hasAnyInfiniteBound(lb, ub, n) || totalIters == 0 ||
				hasCBCScalingRisk(p, lb, ub, cuts) || hasCBCPivotRiskTiny(p, lb, ub, cuts)) {
			if fbRes, handled, isCanceled := fallback(true); handled {
				return fbRes, !isCanceled, isCanceled
			}
		}
		if unresolvedCancel && status == StatusNumericalError {
			if fbRes, handled, isCanceled := fallback(true); handled {
				return fbRes, !isCanceled, isCanceled
			}
		}
		return LPResult{Status: status, Iterations: totalIters}, false, false
	}

	snapCBCBounds(primal, lb, ub, n)
	objMin := evalLinearSum(0, p.objMin, primal, n)
	if !cbcPrimalWithinTol(p, lb, ub, cuts, primal, false) {
		if !droppedTiny {
			if fbRes, handled, isCanceled := fallback(true); handled {
				return fbRes, !isCanceled, isCanceled
			}
		}
		return LPResult{Status: StatusNumericalError, Iterations: totalIters}, false, false
	}
	if !cbcLPDualWithinTol(p, lb, ub, cuts, primal, duals, rc, objMin) {
		if hasAnyInfiniteBound(lb, ub, n) || unresolvedCancel {
			if fbRes, handled, isCanceled := fallback(unresolvedCancel); handled {
				return fbRes, !isCanceled, isCanceled
			}
		}
		return LPResult{Status: StatusNumericalError, Iterations: totalIters}, false, false
	}
	userObj := p.EvaluateObjective(primal)
	if math.IsNaN(userObj) || math.IsInf(userObj, 0) {
		return LPResult{Status: StatusNumericalError, Iterations: totalIters}, false, false
	}
	if p.sense == Maximize {
		for i := range m {
			duals[i] = -duals[i]
		}
		for j := range n {
			rc[j] = -rc[j]
		}
	}
	return LPResult{
		Status:       StatusOptimal,
		ObjectiveMin: objMin,
		Objective:    userObj,
		Primal:       primal,
		Dual:         duals[:p.numConstraints],
		ReducedCosts: rc,
		Iterations:   totalIters,
	}, false, false
}

func solveCBC(ctx context.Context, p *CompiledProblem, opt Options, start, deadline time.Time, done <-chan struct{}) Result {
	if p == nil {
		return Result{
			Backend:   BackendCBC,
			Status:    StatusUndefined,
			SolveTime: time.Since(start),
		}
	}
	n := p.numVars
	m := p.numConstraints
	if n < 0 || m < 0 || n > math.MaxInt32 || m > math.MaxInt32 ||
		len(p.varLB) != n || len(p.varUB) != n || len(p.rowLB) != m || len(p.rowUB) != m {
		return cbcStatusResult(p, StatusNumericalError, start, 0, 0, 0)
	}

	for j := range n {
		if p.varLB[j] > p.varUB[j] {
			return cbcStatusResult(p, StatusInfeasible, start, 0, 0, 0)
		}
	}

	if n == 0 {
		for i := range m {
			if 0 < p.rowLB[i]-cbcFeasTol || 0 > p.rowUB[i]+cbcFeasTol {
				return cbcStatusResult(p, StatusInfeasible, start, 0, 0, 0)
			}
		}
		if math.IsNaN(p.objConstant) || math.IsInf(p.objConstant, 0) {
			return cbcStatusResult(p, StatusNumericalError, start, 0, 0, 0)
		}
		var duals, rc []float64
		if !p.isMIP {
			duals = make([]float64, m)
			rc = []float64{}
		}
		return Result{
			modelID:      p.modelID,
			Backend:      BackendCBC,
			Status:       StatusOptimal,
			Objective:    p.objConstant,
			BestBound:    p.objConstant,
			RootLPBound:  p.objConstant,
			RootCutBound: p.objConstant,
			Values:       []float64{},
			Duals:        duals,
			ReducedCosts: rc,
			SolveTime:    time.Since(start),
		}
	}

	if m == 0 {
		if len(p.objMin) != n || len(p.varCat) != n || len(p.objCoeffs) != n {
			return cbcStatusResult(p, StatusNumericalError, start, 0, 0, 0)
		}
		return solveUnconstrainedCBC(p, start)
	}

	cbc, validCBC := validateCompiledForCBC(p)
	if !validCBC || hasExtremeCBCBound(p.varLB, p.varUB, p.rowLB, p.rowUB, nil) {
		return cbcStatusResult(p, StatusNumericalError, start, 0, 0, 0)
	}

	if hasEmptyInfeasibleRow(p) {
		return cbcStatusResult(p, StatusInfeasible, start, 0, 0, 0)
	}

	if !p.isMIP {
		return solveCBCLP(ctx, p, cbc, opt, start, deadline, done)
	}
	return solveCBCMIP(ctx, p, cbc, opt, start, deadline, done)
}

func solveCBCLP(ctx context.Context, p *CompiledProblem, cbc *cbcMatrixData, opt Options, start, deadline time.Time, done <-chan struct{}) Result {
	n := p.numVars
	m := p.numConstraints
	noCuts := cbcCutSet{}
	rowCancel, objCancel := hasCBCCancellationRisk(p, p.varLB, p.varUB, noCuts)
	droppedTiny := hasCBCDroppedTiny(p, p.varLB, p.varUB, noCuts)
	var pc cbcPrecond
	if (rowCancel || objCancel) && !droppedTiny {
		pc = preconditionCBC(p, p.varLB, p.varUB, noCuts, false, deadline, done)
		if ctx != nil && ctx.Err() != nil {
			return Result{}
		}
		if pc.timedOut {
			return cbcStatusResult(p, StatusTimeLimit, start, 0, 0, 0)
		}
		if pc.infeasible {
			return cbcStatusResult(p, StatusInfeasible, start, 0, 0, 0)
		}
	}

	mat := selectCBCMatrixView(p, cbc, &pc)
	release, waitSt := acquireCBCSlot(ctx, deadline, done)
	if release == nil {
		if ctx != nil && ctx.Err() != nil {
			return Result{}
		}
		return cbcStatusResult(p, waitSt, start, 0, 0, 0)
	}

	primal := make([]float64, n)
	duals := make([]float64, m)
	rc := make([]float64, n)
	var cRes C.GoblinResult
	runLP := func(presolveFlag C.int, maxIt int) (timedOut bool) {
		return withCBCCancel(ctx, deadline, done, func(timeLimitSec float64, cancelPtr *C.int) {
			C.goblin_solve_lp(
				C.int(n),
				C.int(m),
				cInt32Ptr(mat.colPtr32),
				cInt32Ptr(mat.rowIdx32),
				cFloat64Ptr(mat.colVal),
				cFloat64Ptr(mat.colLBClp),
				cFloat64Ptr(mat.colUBClp),
				cFloat64Ptr(mat.objMinClp),
				cFloat64Ptr(mat.rowLBClp),
				cFloat64Ptr(mat.rowUBClp),
				presolveFlag,
				clampCInt(maxIt),
				C.double(timeLimitSec),
				cancelPtr,
				&cRes,
				cFloat64Ptr(primal),
				cFloat64Ptr(duals),
				cFloat64Ptr(rc),
			)
			if pc.active && Status(cRes.status) == StatusOptimal {
				pc.restoreLPPrimalDuals(p, noCuts, primal, duals, rc)
			}
		})
	}
	totalIters := 0
	timedOutBeforeCall := func() bool {
		defer release()
		if runLP(boolToCInt(opt.EnablePresolve), opt.MaxIter) {
			return true
		}
		totalIters = int(cRes.iterations)
		if Status(cRes.status) == StatusOptimal {
			snapCBCBounds(primal, p.varLB, p.varUB, n)
			objMin := evalLinearSum(0, p.objMin, primal, n)
			if cbcPrimalWithinTol(p, p.varLB, p.varUB, noCuts, primal, false) &&
				!cbcLPDualWithinTol(p, p.varLB, p.varUB, noCuts, primal, duals, rc, objMin) {
				remIter := opt.MaxIter
				if opt.MaxIter > 0 {
					remIter = opt.MaxIter - totalIters
					if remIter <= 0 {
						cRes.status = C.int(StatusIterationLimit)
						return false
					}
				}
				if !runLP(-1, remIter) {
					totalIters += int(cRes.iterations)
				}
			}
		}
		return false
	}()
	if ctx != nil && ctx.Err() != nil {
		return Result{}
	}
	if timedOutBeforeCall {
		return cbcStatusResult(p, StatusTimeLimit, start, 0, 0, 0)
	}

	unresolvedCancel := (rowCancel || objCancel) && !droppedTiny && (!pc.active || pc.unresolved)
	lpRes, fellBack, canceled := finalizeCBCLP(&cbcLPFinalizeReq{
		p: p, lb: p.varLB, ub: p.varUB, cuts: noCuts,
		status: Status(cRes.status), primal: primal, duals: duals, rc: rc,
		totalIters: totalIters, droppedTiny: droppedTiny, unresolvedCancel: unresolvedCancel,
		fallback: func(acceptInfeasible bool) (LPResult, bool, bool) {
			refRes, isCanceled := runBudgetedGoLPProbe(ctx, p, opt, totalIters, nil, nil, deadline, done)
			if isCanceled {
				return LPResult{}, true, true
			}
			totalIters += refRes.Iterations
			if refRes.Status == StatusOptimal && !math.IsNaN(refRes.Objective) && !math.IsInf(refRes.Objective, 0) {
				refRes.Iterations = totalIters
				return refRes, true, false
			}
			if refRes.Status == StatusUnbounded || refRes.Status == StatusTimeLimit || refRes.Status == StatusIterationLimit ||
				(acceptInfeasible && refRes.Status == StatusInfeasible) {
				return LPResult{Status: refRes.Status, Iterations: totalIters}, true, false
			}
			return LPResult{}, false, false
		},
	})
	if canceled {
		return Result{}
	}
	res := cbcStatusResult(p, lpRes.Status, start, 0, lpRes.Iterations, 0)
	res.FellBackToGo = fellBack
	if lpRes.Status == StatusOptimal {
		res.Objective = lpRes.Objective
		res.BestBound = lpRes.Objective
		res.RootLPBound = lpRes.Objective
		res.RootCutBound = lpRes.Objective
		res.Values = lpRes.Primal
		res.Duals = lpRes.Dual
		res.ReducedCosts = lpRes.ReducedCosts
	}
	return res
}

func solveCBCMIP(ctx context.Context, p *CompiledProblem, cbc *cbcMatrixData, opt Options, start, deadline time.Time, done <-chan struct{}) Result {
	n := p.numVars
	m := p.numConstraints
	noCuts := cbcCutSet{}
	droppedTiny := hasCBCDroppedTiny(p, p.varLB, p.varUB, noCuts)
	if !droppedTiny && !hasUnscalableCBCMIPObjective(p.objMin) && hasCBCPivotRiskTiny(p, p.varLB, p.varUB, noCuts) {
		return fallbackCBCMIPToGo(ctx, p, opt, start, deadline, done, 0, 0, 0)
	}
	rowCancel, objCancel := hasCBCCancellationRisk(p, p.varLB, p.varUB, noCuts)
	var pc cbcPrecond
	if (rowCancel || objCancel) && !droppedTiny && !hasUnscalableCBCMIPObjective(p.objMin) {
		pc = preconditionCBC(p, p.varLB, p.varUB, noCuts, true, deadline, done)
		if ctx != nil && ctx.Err() != nil {
			return Result{}
		}
		if pc.timedOut {
			return cbcStatusResult(p, StatusTimeLimit, start, 0, 0, 0)
		}
		if pc.infeasible {
			return cbcStatusResult(p, StatusInfeasible, start, 0, 0, 0)
		}
		if !pc.active || pc.unresolved {
			return fallbackCBCMIPToGo(ctx, p, opt, start, deadline, done, 0, 0, 0)
		}
	}

	mat := selectCBCMatrixView(p, cbc, &pc)
	effRelGap := clampCBCGapTol(opt.RelGapTol)
	if (pc.hasFixedObj || p.objConstant != 0) && effRelGap > 0 {
		off := math.Abs(pc.totalMinShift(p))
		if math.IsNaN(off) || math.IsInf(off, 0) {
			effRelGap = 0
		} else if off > 0 {
			effRelGap /= 1.0 + off
		}
	}

	release, waitSt := acquireCBCSlot(ctx, deadline, done)
	if release == nil {
		if ctx != nil && ctx.Err() != nil {
			return Result{}
		}
		return cbcStatusResult(p, waitSt, start, 0, 0, 0)
	}

	primal := make([]float64, n)
	var cRes C.GoblinResult

	// Map untouched default settings to CBC's standard root cut pass limit (30)
	// and unlimited nodes while preserving explicit caller overrides.
	cutPasses := opt.MaxCutPasses
	if opt.defaultCutPassesUsed {
		cutPasses = cbcDefaultCutPasses
	}
	maxNodes := opt.MaxNodes
	if opt.defaultMaxNodesUsed || maxNodes <= 0 {
		maxNodes = -1
	}

	timedOutBeforeCall := func() bool {
		defer release()
		return withCBCCancel(ctx, deadline, done, func(timeLimitSec float64, cancelPtr *C.int) {
			C.goblin_solve_mip(
				C.int(n),
				C.int(m),
				cInt32Ptr(mat.colPtr32),
				cInt32Ptr(mat.rowIdx32),
				cFloat64Ptr(mat.colVal),
				cFloat64Ptr(mat.colLBClp),
				cFloat64Ptr(mat.colUBClp),
				cFloat64Ptr(mat.objMinClp),
				cFloat64Ptr(mat.rowLBClp),
				cFloat64Ptr(mat.rowUBClp),
				C.int(len(cbc.intCols32)),
				cInt32Ptr(cbc.intCols32),
				boolToCInt(opt.EnablePresolve),
				boolToCInt(opt.EnableCuts),
				boolToCInt(opt.EnableHeuristics),
				clampCInt(maxNodes),
				clampCInt(cutPasses),
				clampCInt(opt.MaxIter),
				C.double(effRelGap),
				C.double(clampCBCGapTol(opt.AbsGapTol)),
				C.double(timeLimitSec),
				cancelPtr,
				&cRes,
				cFloat64Ptr(primal),
			)
		})
	}()
	if ctx != nil && ctx.Err() != nil {
		return Result{}
	}
	if timedOutBeforeCall {
		return cbcStatusResult(p, StatusTimeLimit, start, 0, 0, 0)
	}
	rawRootLP := float64(cRes.root_lp_min)
	rawRootCut := float64(cRes.root_cut_min)
	rawBestBnd := float64(cRes.best_bound_min)
	if pc.active {
		pc.restoreMIPResult(p, primal, &cRes)
	}
	totalIters := int(cRes.iterations)

	status := Status(cRes.status)
	if status == StatusUnbounded && !hasAnyInfiniteBound(p.varLB, p.varUB, n) {
		status = StatusNumericalError
	}
	if status == StatusUnbounded && len(cbc.intCols32) > 0 {
		if !droppedTiny {
			resRelax, canceled := runBudgetedGoLPProbe(ctx, p, opt, totalIters, nil, nil, deadline, done)
			if canceled {
				return Result{}
			}
			totalIters += resRelax.Iterations
			switch resRelax.Status {
			case StatusOptimal:
				return fallbackCBCMIPToGo(ctx, p, opt, start, deadline, done, int(cRes.nodes), totalIters, int(cRes.cuts_added))
			case StatusInfeasible, StatusTimeLimit, StatusIterationLimit:
				return cbcStatusResult(p, resRelax.Status, start, int(cRes.nodes), totalIters, int(cRes.cuts_added))
			}
		}
		feasProb := p.zeroObjCopy()
		feasOpts, limitSt := remainingCBCMIPBudget(opt, int(cRes.nodes), totalIters)
		if limitSt != StatusNotSolved {
			return cbcStatusResult(p, limitSt, start, int(cRes.nodes), totalIters, int(cRes.cuts_added))
		}
		feasRes := solveCBC(ctx, feasProb, feasOpts, start, deadline, done)
		if ctx != nil && ctx.Err() != nil {
			return Result{}
		}
		st := feasRes.Status
		if st == StatusOptimal {
			st = StatusUnbounded
		}
		outRes := cbcStatusResult(
			p, st, start,
			int(cRes.nodes)+feasRes.Nodes,
			totalIters+feasRes.Iterations,
			int(cRes.cuts_added)+feasRes.CutsAdded,
		)
		outRes.FellBackToGo = feasRes.FellBackToGo
		return outRes
	}
	if status == StatusInfeasible && !droppedTiny &&
		(hasAnyInfiniteBound(p.varLB, p.varUB, n) || hasCBCScalingRisk(p, p.varLB, p.varUB, noCuts)) {
		resRelax, canceled := runBudgetedGoLPProbe(ctx, p, opt, totalIters, nil, nil, deadline, done)
		if canceled {
			return Result{}
		}
		totalIters += resRelax.Iterations
		switch resRelax.Status {
		case StatusInfeasible:
			// Root LP relaxation is Farkas-certified infeasible; preserve StatusInfeasible.
		case StatusTimeLimit, StatusIterationLimit:
			return cbcStatusResult(p, resRelax.Status, start, int(cRes.nodes), totalIters, int(cRes.cuts_added))
		default:
			if cRes.nodes == 0 || resRelax.Status == StatusUnbounded || hasCBCScalingRisk(p, p.varLB, p.varUB, noCuts) {
				return fallbackCBCMIPToGo(ctx, p, opt, start, deadline, done, int(cRes.nodes), totalIters, int(cRes.cuts_added))
			}
		}
	}
	hasValidSol := len(primal) > 0 && cRes.has_sol != 0
	if hasValidSol {
		snapCBCBounds(primal, p.varLB, p.varUB, n)
		if !cbcPrimalWithinTol(p, p.varLB, p.varUB, noCuts, primal, true) {
			if polished, extraIt, ok := polishCBCMIPPrimal(
				ctx, p, opt, totalIters, status == StatusOptimal,
				cRes.has_best_bound != 0, float64(cRes.best_bound_min), primal, deadline, done,
			); ok {
				totalIters += extraIt
				primal = polished
			} else {
				totalIters += extraIt
				hasValidSol = false
				if status == StatusOptimal {
					status = StatusNumericalError
					cRes.has_best_bound = 0
					cRes.has_root_lp = 0
					cRes.has_root_cut = 0
				}
			}
		}
	}
	if status == StatusOptimal && hasValidSol {
		mipSt, extraIt := verifyCBCMIPOptimality(ctx, &cbcMIPVerifyReq{
			p: p, opt: opt, baseIters: totalIters,
			hasRootLP: cRes.has_root_lp != 0, rootLPMin: float64(cRes.root_lp_min),
			hasBestBound: cRes.has_best_bound != 0, bestBoundMin: float64(cRes.best_bound_min),
			primal: primal, deadline: deadline, done: done,
		})
		totalIters += extraIt
		if mipSt != StatusOptimal {
			if ctx != nil && ctx.Err() != nil {
				return Result{}
			}
			if mipSt == StatusUnbounded {
				return cbcStatusResult(p, StatusUnbounded, start, int(cRes.nodes), totalIters, int(cRes.cuts_added))
			}
			if mipSt == StatusNumericalError && !droppedTiny && (pc.active || hasCBCScalingRisk(p, p.varLB, p.varUB, noCuts)) {
				return fallbackCBCMIPToGo(ctx, p, opt, start, deadline, done, int(cRes.nodes), totalIters, int(cRes.cuts_added))
			}
			status = mipSt
			hasValidSol = false
			cRes.has_best_bound = 0
			cRes.has_root_lp = 0
			cRes.has_root_cut = 0
		}
	}
	res := Result{
		modelID:    p.modelID,
		Backend:    BackendCBC,
		Status:     status,
		Nodes:      int(cRes.nodes),
		Iterations: totalIters,
		CutsAdded:  int(cRes.cuts_added),
		SolveTime:  time.Since(start),
	}
	curFullMin := math.Inf(1)
	if hasValidSol {
		obj := p.EvaluateObjective(primal)
		res.Objective = obj
		res.Values = primal
		curFullMin = evalLinearSum(p.objConstMin(), p.objMin, primal, n)
	}
	if cRes.has_root_lp != 0 {
		if v, ok := pc.userBoundFromReducedMin(p, rawRootLP, curFullMin, hasValidSol); ok {
			res.RootLPBound = v
		} else {
			res.RootLPBound = p.ObjectiveFromMin(float64(cRes.root_lp_min))
		}
	}
	if cRes.has_root_cut != 0 {
		if v, ok := pc.userBoundFromReducedMin(p, rawRootCut, curFullMin, hasValidSol); ok {
			res.RootCutBound = v
		} else {
			res.RootCutBound = p.ObjectiveFromMin(float64(cRes.root_cut_min))
		}
	}
	if cRes.has_best_bound != 0 {
		if v, ok := pc.userBoundFromReducedMin(p, rawBestBnd, curFullMin, hasValidSol); ok {
			res.BestBound = v
		} else {
			res.BestBound = p.ObjectiveFromMin(float64(cRes.best_bound_min))
		}
	}
	if isLimitStatus(res.Status) && (cRes.has_best_bound == 0 || math.IsNaN(res.BestBound) || isOppositeInfBound(p.sense, res.BestBound)) {
		res.BestBound = trivialDualBound(p.sense)
	}
	if res.Status == StatusOptimal && (!hasValidSol || math.IsNaN(res.Objective) || math.IsInf(res.Objective, 0) || math.IsNaN(res.BestBound) || math.IsInf(res.BestBound, 0)) {
		return cbcStatusResult(p, StatusNumericalError, start, res.Nodes, res.Iterations, res.CutsAdded)
	}
	return res
}

func solveUnconstrainedCBC(p *CompiledProblem, start time.Time) Result {
	n := p.numVars
	primal := make([]float64, n)
	var rc []float64
	if !p.isMIP {
		rc = make([]float64, n)
	}
	senseSign := 1.0
	if p.sense == Maximize {
		senseSign = -1.0
	}
	for j := range n {
		c := p.objMin[j]
		lb, ub := p.varLB[j], p.varUB[j]
		if c < 0 {
			if math.IsInf(ub, 1) {
				return Result{modelID: p.modelID, Backend: BackendCBC, FellBackToGo: true, Status: StatusUnbounded, SolveTime: time.Since(start)}
			}
			primal[j] = ub
		} else if c > 0 {
			if math.IsInf(lb, -1) {
				return Result{modelID: p.modelID, Backend: BackendCBC, FellBackToGo: true, Status: StatusUnbounded, SolveTime: time.Since(start)}
			}
			primal[j] = lb
		} else {
			if !math.IsInf(lb, -1) && !math.IsInf(ub, 1) {
				if lb <= 0 && 0 <= ub {
					primal[j] = 0
				} else {
					primal[j] = lb
				}
			} else if !math.IsInf(lb, -1) {
				primal[j] = max(0.0, lb)
			} else if !math.IsInf(ub, 1) {
				primal[j] = min(0.0, ub)
			} else {
				primal[j] = 0
			}
		}
		if p.varCat[j] != Continuous {
			primal[j] = math.Round(primal[j])
		}
		if rc != nil {
			rc[j] = senseSign * c
		}
	}
	obj := p.EvaluateObjective(primal)
	if math.IsNaN(obj) || math.IsInf(obj, 0) {
		return Result{
			modelID:      p.modelID,
			Backend:      BackendCBC,
			FellBackToGo: true,
			Status:       StatusNumericalError,
			SolveTime:    time.Since(start),
		}
	}
	var duals []float64
	if !p.isMIP {
		duals = []float64{}
	}
	return Result{
		modelID:      p.modelID,
		Backend:      BackendCBC,
		FellBackToGo: true,
		Status:       StatusOptimal,
		Objective:    obj,
		BestBound:    obj,
		RootLPBound:  obj,
		RootCutBound: obj,
		Values:       primal,
		Duals:        duals,
		ReducedCosts: rc,
		SolveTime:    time.Since(start),
	}
}

type cbcLPHandle struct {
	mu  sync.Mutex
	ptr *C.GoblinLPSolver
}

func (h *cbcLPHandle) closeSync() {
	if h == nil {
		return
	}
	h.mu.Lock()
	if h.ptr == nil {
		h.mu.Unlock()
		return
	}
	h.mu.Unlock()
	cbcSem <- struct{}{}
	defer releaseCBCSlot()
	h.mu.Lock()
	ptr := h.ptr
	h.ptr = nil
	h.mu.Unlock()
	if ptr != nil {
		C.goblin_lp_free(ptr)
	}
}

func cleanupCBCLPHandle(h *cbcLPHandle) {
	if h == nil {
		return
	}
	h.mu.Lock()
	alive := h.ptr != nil
	h.mu.Unlock()
	if alive {
		go h.closeSync()
	}
}

type cbcCutTermPair struct {
	col int
	val float64
}

// CBCLPSolver wraps a persistent C++ OsiClpSolverInterface instance that can be
// solved from scratch and repeatedly warm-started with modified bounds or added
// cutting planes. A single CBCLPSolver instance is not safe for concurrent use
// by multiple goroutines without external synchronization.
type CBCLPSolver struct {
	prob            *CompiledProblem
	handle          *cbcLPHandle
	cleanup         runtime.Cleanup
	curLB           []float64
	curUB           []float64
	scratchLBClp    []float64
	scratchUBClp    []float64
	scratchCols32   []int32
	scratchCutPairs []cbcCutTermPair
	cuts            cbcCutSet
	numCut          int
	invalidBounds   bool
	invalidCut      bool
	cutNumErr       bool
	infeasibleCut   bool
}

func (s *CBCLPSolver) withHandleLocked(fn func(ptr *C.GoblinLPSolver)) bool {
	if s == nil || s.handle == nil {
		return false
	}
	cbcSem <- struct{}{}
	defer releaseCBCSlot()
	s.handle.mu.Lock()
	defer s.handle.mu.Unlock()
	defer runtime.KeepAlive(s)
	if s.handle.ptr == nil {
		return false
	}
	fn(s.handle.ptr)
	return true
}

// NewCBCLPSolver creates a C++ OsiClpSolverInterface LP solver for p.
func NewCBCLPSolver(p *CompiledProblem) *CBCLPSolver {
	if p == nil {
		return &CBCLPSolver{}
	}
	var h *cbcLPHandle
	if p.numVars > 0 {
		if cbc, ok := validateCompiledForCBC(p); ok {
			cbcSem <- struct{}{}
			raw := C.goblin_lp_new(
				C.int(p.numVars),
				C.int(p.numConstraints),
				cInt32Ptr(cbc.colPtr32),
				cInt32Ptr(cbc.rowIdx32),
				cFloat64Ptr(p.colVal),
				cFloat64Ptr(cbc.colLBClp),
				cFloat64Ptr(cbc.colUBClp),
				cFloat64Ptr(p.objMin),
				cFloat64Ptr(cbc.rowLBClp),
				cFloat64Ptr(cbc.rowUBClp),
			)
			releaseCBCSlot()
			if raw != nil {
				h = &cbcLPHandle{ptr: raw}
			}
		}
	}
	s := &CBCLPSolver{
		prob:   p,
		handle: h,
		curLB:  slices.Clone(p.varLB),
		curUB:  slices.Clone(p.varUB),
	}
	if h != nil {
		s.cleanup = runtime.AddCleanup(s, cleanupCBCLPHandle, h)
	}
	return s
}

// Close releases the underlying C++ OsiClpSolverInterface instance.
func (s *CBCLPSolver) Close() {
	if s == nil || s.handle == nil {
		return
	}
	s.cleanup.Stop()
	s.handle.closeSync()
	runtime.KeepAlive(s)
}

// NumVars returns the number of structural columns in the problem, or 0 when nil.
func (s *CBCLPSolver) NumVars() int {
	if s == nil || s.prob == nil {
		return 0
	}
	return s.prob.numVars
}

// NumCuts returns the number of cuts added to the solver.
func (s *CBCLPSolver) NumCuts() int {
	if s == nil {
		return 0
	}
	return s.numCut
}

// TotalRows returns the total number of rows (structural constraints + added cuts).
func (s *CBCLPSolver) TotalRows() int {
	if s == nil || s.prob == nil {
		return 0
	}
	return s.prob.numConstraints + s.numCut
}

// SetStructuralBounds updates the column bounds in place while preserving the
// current simplex basis for warm-started dual or primal resolves.
func (s *CBCLPSolver) SetStructuralBounds(lb, ub []float64) {
	if s == nil || s.prob == nil {
		return
	}
	n := s.prob.numVars
	if n < 0 || len(lb) < n || len(ub) < n || len(s.curLB) < n || len(s.curUB) < n {
		s.invalidBounds = true
		return
	}
	if n == 0 {
		s.invalidBounds = false
		return
	}
	if s.handle == nil {
		s.invalidBounds = true
		return
	}
	for j := range n {
		if math.IsNaN(lb[j]) || math.IsNaN(ub[j]) || math.IsInf(lb[j], 1) || math.IsInf(ub[j], -1) {
			s.invalidBounds = true
			return
		}
	}
	if cap(s.scratchLBClp) < n {
		s.scratchLBClp = make([]float64, n)
		s.scratchUBClp = make([]float64, n)
	}
	lbClp := s.scratchLBClp[:n]
	ubClp := s.scratchUBClp[:n]
	for j := range n {
		lbClp[j] = clampCBCBound(lb[j])
		ubClp[j] = clampCBCBound(ub[j])
	}
	var setOK C.int
	called := s.withHandleLocked(func(ptr *C.GoblinLPSolver) {
		setOK = C.goblin_lp_set_bounds(ptr, C.int(n), cFloat64Ptr(lbClp), cFloat64Ptr(ubClp))
	})
	if !called || setOK == 0 {
		s.invalidBounds = true
		return
	}
	for j := range n {
		s.curLB[j] = lb[j]
		s.curUB[j] = ub[j]
	}
	s.invalidBounds = false
}

// AddCut appends a linear inequality sum(val[k] * x[colIdx[k]]) <= rhs to the LP
// while preserving the existing basis for warm-started dual simplex resolve.
// Out-of-range indices are ignored and duplicate column indices are summed.
func (s *CBCLPSolver) AddCut(colIdx []int, val []float64, rhs float64) {
	if s == nil || s.prob == nil {
		return
	}
	if math.IsNaN(rhs) {
		s.invalidCut = true
		return
	}
	n := s.prob.numVars
	kLim := min(len(colIdx), len(val))
	for k := range kLim {
		c := colIdx[k]
		if c < 0 || c >= n {
			continue
		}
		if math.IsNaN(val[k]) || math.IsInf(val[k], 0) {
			s.invalidCut = true
			return
		}
	}
	if len(colIdx) != len(val) {
		s.invalidCut = true
		s.cutNumErr = true
		return
	}
	if math.IsInf(rhs, 1) {
		return
	}
	if math.IsInf(rhs, -1) {
		s.infeasibleCut = true
		return
	}
	if n == 0 {
		s.cuts.rhs = append(s.cuts.rhs, rhs)
		s.numCut++
		return
	}
	if s.handle == nil {
		s.invalidCut = true
		s.cutNumErr = true
		return
	}
	pairs := s.scratchCutPairs[:0]
	for k := range kLim {
		c := colIdx[k]
		v := val[k]
		if c < 0 || c >= n || v == 0 {
			continue
		}
		pairs = append(pairs, cbcCutTermPair{col: c, val: v})
	}
	s.scratchCutPairs = pairs
	slices.SortFunc(pairs, func(a, b cbcCutTermPair) int { return cmp.Compare(a.col, b.col) })
	cols32 := s.scratchCols32[:0]
	vals := make([]float64, 0, len(pairs))
	for i := 0; i < len(pairs); {
		start := i
		c := pairs[i].col
		sum := pairs[i].val
		for i++; i < len(pairs) && pairs[i].col == c; i++ {
			sum += pairs[i].val
		}
		if i-start > 1 {
			var w wideDotAcc
			for k := start; k < i; k++ {
				w.addVal(pairs[k].val)
			}
			sum = w.finish(math.NaN())
			if math.IsNaN(sum) || math.IsInf(sum, 0) {
				s.invalidCut = true
				return
			}
		}
		if sum == 0 {
			continue
		}
		cols32 = append(cols32, int32(c))
		vals = append(vals, sum)
	}
	s.scratchCols32 = cols32
	if len(cols32) > math.MaxInt32 {
		s.invalidCut = true
		s.cutNumErr = true
		return
	}
	var added C.int
	called := s.withHandleLocked(func(ptr *C.GoblinLPSolver) {
		added = C.goblin_lp_add_cut(
			ptr,
			C.int(len(cols32)),
			cInt32Ptr(cols32),
			cFloat64Ptr(vals),
			C.double(clampCBCBound(rhs)),
		)
	})
	if !called || added == 0 {
		s.invalidCut = true
		s.cutNumErr = true
		return
	}
	colsInt := make([]int, len(cols32))
	for i, c := range cols32 {
		colsInt[i] = int(c)
	}
	s.cuts.cols = append(s.cuts.cols, colsInt)
	s.cuts.vals = append(s.cuts.vals, vals)
	s.cuts.rhs = append(s.cuts.rhs, rhs)
	s.numCut++
}

// Solve solves or warm-start resolves the current LP relaxation via COIN-OR CLP.
func (s *CBCLPSolver) Solve() LPResult {
	if s == nil || s.invalidBounds {
		return LPResult{Status: StatusUndefined}
	}
	if s.invalidCut {
		if s.cutNumErr {
			return LPResult{Status: StatusNumericalError}
		}
		return LPResult{Status: StatusUndefined}
	}
	p := s.prob
	if p == nil || p.invalidErr != nil {
		return LPResult{Status: StatusUndefined}
	}
	if s.infeasibleCut {
		return LPResult{Status: StatusInfeasible}
	}
	n := p.numVars
	m := s.TotalRows()
	if n < 0 || m < 0 || n > math.MaxInt32 || m > math.MaxInt32 ||
		len(s.curLB) < n || len(s.curUB) < n || len(p.objMin) < n ||
		len(p.rowLB) < p.numConstraints || len(p.rowUB) < p.numConstraints {
		return LPResult{Status: StatusNumericalError}
	}

	for j := range n {
		if s.curLB[j] > s.curUB[j] {
			return LPResult{Status: StatusInfeasible}
		}
	}
	if n == 0 {
		for i := range p.numConstraints {
			if 0 < p.rowLB[i]-cbcFeasTol || 0 > p.rowUB[i]+cbcFeasTol {
				return LPResult{Status: StatusInfeasible}
			}
		}
		for _, rhs := range s.cuts.rhs {
			if 0 > rhs+cbcFeasTol {
				return LPResult{Status: StatusInfeasible}
			}
		}
		if math.IsNaN(p.objConstant) || math.IsInf(p.objConstant, 0) {
			return LPResult{Status: StatusNumericalError}
		}
		return LPResult{
			Status:       StatusOptimal,
			ObjectiveMin: 0,
			Objective:    p.objConstant,
			Primal:       []float64{},
			Dual:         make([]float64, p.numConstraints),
			ReducedCosts: []float64{},
		}
	}
	if s.handle == nil || hasExtremeCBCBound(s.curLB, s.curUB, p.rowLB, p.rowUB, s.cuts.rhs) {
		return LPResult{Status: StatusNumericalError}
	}

	rowCancel, objCancel := hasCBCCancellationRisk(p, s.curLB, s.curUB, s.cuts)
	droppedTiny := hasCBCDroppedTiny(p, s.curLB, s.curUB, s.cuts)
	var pc cbcPrecond
	if (rowCancel || objCancel) && !droppedTiny {
		pc = preconditionCBC(p, s.curLB, s.curUB, s.cuts, false, time.Time{}, nil)
		if pc.infeasible {
			return LPResult{Status: StatusInfeasible}
		}
	}

	primal := make([]float64, n)
	duals := make([]float64, m)
	rc := make([]float64, n)
	var cRes C.GoblinResult
	totalIters := 0

	hasPtr := s.withHandleLocked(func(basePtr *C.GoblinLPSolver) {
		solvePtr := basePtr
		var tmpPtr *C.GoblinLPSolver
		if pc.active {
			tmpPtr = C.goblin_lp_new(
				C.int(n),
				C.int(p.numConstraints),
				cInt32Ptr(pc.colPtr32),
				cInt32Ptr(pc.rowIdx32),
				cFloat64Ptr(pc.colVal),
				cFloat64Ptr(pc.colLBClp),
				cFloat64Ptr(pc.colUBClp),
				cFloat64Ptr(pc.objMin),
				cFloat64Ptr(pc.rowLBClp),
				cFloat64Ptr(pc.rowUBClp),
			)
			if tmpPtr != nil {
				allCutsOK := true
				for e := range len(pc.cuts.rhs) {
					cols32 := s.scratchCols32[:0]
					for _, c := range pc.cuts.cols[e] {
						cols32 = append(cols32, int32(c))
					}
					s.scratchCols32 = cols32
					if C.goblin_lp_add_cut(
						tmpPtr,
						C.int(len(cols32)),
						cInt32Ptr(cols32),
						cFloat64Ptr(pc.cuts.vals[e]),
						C.double(clampCBCBound(pc.cuts.rhs[e])),
					) == 0 {
						allCutsOK = false
						break
					}
				}
				if allCutsOK {
					solvePtr = tmpPtr
				} else {
					C.goblin_lp_free(tmpPtr)
					tmpPtr = nil
					pc.active = false
				}
			} else {
				pc.active = false
			}
		}
		if tmpPtr != nil {
			defer C.goblin_lp_free(tmpPtr)
		}
		C.goblin_lp_solve(
			solvePtr,
			C.int(n),
			C.int(m),
			&cRes,
			cFloat64Ptr(primal),
			cFloat64Ptr(duals),
			cFloat64Ptr(rc),
		)
		totalIters = int(cRes.iterations)
		if Status(cRes.status) == StatusOptimal {
			if pc.active {
				pc.restoreLPPrimalDuals(p, s.cuts, primal, duals, rc)
			}
			snapCBCBounds(primal, s.curLB, s.curUB, n)
			objMinTry := evalLinearSum(0, p.objMin, primal, n)
			if cbcPrimalWithinTol(p, s.curLB, s.curUB, s.cuts, primal, false) &&
				!cbcLPDualWithinTol(p, s.curLB, s.curUB, s.cuts, primal, duals, rc, objMinTry) {
				C.goblin_lp_tighten_dual_tol(solvePtr, 1e-16)
				C.goblin_lp_solve(
					solvePtr,
					C.int(n),
					C.int(m),
					&cRes,
					cFloat64Ptr(primal),
					cFloat64Ptr(duals),
					cFloat64Ptr(rc),
				)
				if pc.active && Status(cRes.status) == StatusOptimal {
					pc.restoreLPPrimalDuals(p, s.cuts, primal, duals, rc)
				}
				totalIters += int(cRes.iterations)
			}
		}
	})
	if !hasPtr {
		return LPResult{Status: StatusNumericalError}
	}

	unresolvedCancel := (rowCancel || objCancel) && !droppedTiny && (!pc.active || pc.unresolved)
	lpRes, _, _ := finalizeCBCLP(&cbcLPFinalizeReq{
		p: p, lb: s.curLB, ub: s.curUB, cuts: s.cuts,
		status: Status(cRes.status), primal: primal, duals: duals, rc: rc,
		totalIters: totalIters, droppedTiny: droppedTiny, unresolvedCancel: unresolvedCancel,
		fallback: func(acceptInfeasible bool) (LPResult, bool, bool) {
			lpRef := NewLPSolver(p, nil)
			defer lpRef.Close()
			lpRef.SetStructuralBounds(s.curLB, s.curUB)
			for e := range s.cuts.count() {
				lpRef.AddCut(s.cuts.cols[e], s.cuts.vals[e], s.cuts.rhs[e])
			}
			refRes := lpRef.Solve()
			totalIters += refRes.Iterations
			if refRes.Status == StatusOptimal && !math.IsNaN(refRes.Objective) && !math.IsInf(refRes.Objective, 0) {
				refRes.Iterations = totalIters
				return refRes, true, false
			}
			if refRes.Status == StatusUnbounded || (acceptInfeasible && refRes.Status == StatusInfeasible) {
				return LPResult{Status: refRes.Status, Iterations: totalIters}, true, false
			}
			return LPResult{}, false, false
		},
	})
	return lpRes
}

func (pc *cbcPrecond) restoreMIPResult(p *CompiledProblem, primal []float64, cRes *C.GoblinResult) {
	if cRes.has_sol != 0 {
		for j, fixed := range pc.fixedCols {
			if fixed && j < len(primal) {
				primal[j] = pc.workLB[j]
			}
		}
	}
	if pc.hasFixedObj {
		if cRes.has_root_lp != 0 {
			v := pc.addFixedObjOffset(float64(cRes.root_lp_min))
			if math.IsNaN(v) || math.IsInf(v, 0) {
				cRes.has_root_lp = 0
			} else {
				cRes.root_lp_min = C.double(v)
			}
		}
		if cRes.has_root_cut != 0 {
			v := pc.addFixedObjOffset(float64(cRes.root_cut_min))
			if math.IsNaN(v) || math.IsInf(v, 0) {
				cRes.has_root_cut = 0
			} else {
				cRes.root_cut_min = C.double(v)
			}
		}
		if cRes.has_best_bound != 0 {
			v := pc.addFixedObjOffset(float64(cRes.best_bound_min))
			if math.IsNaN(v) || math.IsInf(v, 0) {
				cRes.has_best_bound = 0
			} else {
				cRes.best_bound_min = C.double(v)
			}
		}
	}
	if cRes.has_sol != 0 {
		curMin := evalLinearSum(0, p.objMin, primal, p.numVars)
		if !math.IsNaN(curMin) && !math.IsInf(curMin, 0) {
			if cRes.has_best_bound != 0 && float64(cRes.best_bound_min) > curMin {
				cRes.best_bound_min = C.double(curMin)
			}
			if cRes.has_root_lp != 0 && float64(cRes.root_lp_min) > curMin {
				cRes.root_lp_min = C.double(curMin)
			}
			if cRes.has_root_cut != 0 && float64(cRes.root_cut_min) > curMin {
				cRes.root_cut_min = C.double(curMin)
			}
		}
	}
}
