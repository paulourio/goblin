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

func validateCompiledForCBC(p *CompiledProblem) bool {
	if p == nil || p.invalidErr != nil {
		return false
	}
	n := p.numVars
	m := p.numConstraints
	if n < 0 || m < 0 || n > math.MaxInt32 || m > math.MaxInt32 {
		return false
	}
	if len(p.varLB) != n || len(p.varUB) != n || len(p.varCat) != n ||
		len(p.colLBClp) != n || len(p.colUBClp) != n ||
		len(p.objMin) != n || len(p.objCoeffs) != n {
		return false
	}
	if len(p.rowLB) != m || len(p.rowUB) != m ||
		len(p.rowLBClp) != m || len(p.rowUBClp) != m {
		return false
	}
	if len(p.colPtr32) != n+1 || p.colPtr32[0] != 0 {
		return false
	}
	for j := range n {
		if p.colPtr32[j] > p.colPtr32[j+1] {
			return false
		}
	}
	nnz := int(p.colPtr32[n])
	if nnz < 0 || nnz > math.MaxInt32 || len(p.rowIdx32) != nnz || len(p.colVal) != nnz {
		return false
	}
	for k := range nnz {
		r := p.rowIdx32[k]
		if r < 0 || int(r) >= m {
			return false
		}
	}
	if len(p.intCols32) > n {
		return false
	}
	for _, col := range p.intCols32 {
		if col < 0 || int(col) >= n {
			return false
		}
	}
	return true
}

// CBCAvailable reports whether the optional C++ COIN-OR CBC/CLP backend (-tags cbc with cgo enabled) is compiled in.
func CBCAvailable() bool { return true }

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
		return Result{
			modelID:   p.modelID,
			Backend:   BackendCBC,
			Status:    StatusNumericalError,
			SolveTime: time.Since(start),
		}
	}

	for j := range n {
		if p.varLB[j] > p.varUB[j]+cbcFeasTol {
			return Result{
				modelID:   p.modelID,
				Backend:   BackendCBC,
				Status:    StatusInfeasible,
				SolveTime: time.Since(start),
			}
		}
	}

	if n == 0 {
		for i := range m {
			if 0 < p.rowLB[i]-cbcFeasTol || 0 > p.rowUB[i]+cbcFeasTol {
				return Result{
					modelID:   p.modelID,
					Backend:   BackendCBC,
					Status:    StatusInfeasible,
					SolveTime: time.Since(start),
				}
			}
		}
		if math.IsNaN(p.objConstant) || math.IsInf(p.objConstant, 0) {
			return Result{
				modelID:   p.modelID,
				Backend:   BackendCBC,
				Status:    StatusNumericalError,
				SolveTime: time.Since(start),
			}
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
			return Result{
				modelID:   p.modelID,
				Backend:   BackendCBC,
				Status:    StatusNumericalError,
				SolveTime: time.Since(start),
			}
		}
		return solveUnconstrainedCBC(p, start)
	}

	if !validateCompiledForCBC(p) || hasUnrepresentableCBCFiniteBound(p.varLB, p.varUB, p.rowLB, p.rowUB, nil) {
		return Result{
			modelID:   p.modelID,
			Backend:   BackendCBC,
			Status:    StatusNumericalError,
			SolveTime: time.Since(start),
		}
	}

	senseSign := 1.0
	if p.sense == Maximize {
		senseSign = -1.0
	}

	if !p.isMIP {
		rowCancel, objCancel := hasCBCCancellationRisk(p, p.varLB, p.varUB, nil, nil, nil)
		droppedTiny := false
		var pc cbcPrecond
		if rowCancel || objCancel {
			droppedTiny = hasCBCDroppedTiny(p, p.varLB, p.varUB, nil, nil)
			if !droppedTiny {
				pc = preconditionCBC(p, p.varLB, p.varUB, nil, nil, nil, false, deadline, done)
				if ctx != nil && ctx.Err() != nil {
					return Result{}
				}
				if pc.timedOut {
					return Result{
						modelID:   p.modelID,
						Backend:   BackendCBC,
						Status:    StatusTimeLimit,
						SolveTime: time.Since(start),
					}
				}
				if pc.infeasible {
					return Result{
						modelID:   p.modelID,
						Backend:   BackendCBC,
						Status:    StatusInfeasible,
						SolveTime: time.Since(start),
					}
				}
			}
		}

		colPtr32 := p.colPtr32
		rowIdx32 := p.rowIdx32
		colVal := p.colVal
		colLBClp := p.colLBClp
		colUBClp := p.colUBClp
		objMinClp := p.objMin
		rowLBClp := p.rowLBClp
		rowUBClp := p.rowUBClp
		if pc.active {
			colPtr32 = pc.colPtr32
			rowIdx32 = pc.rowIdx32
			colVal = pc.colVal
			colLBClp = pc.colLBClp
			colUBClp = pc.colUBClp
			objMinClp = pc.objMin
			rowLBClp = pc.rowLBClp
			rowUBClp = pc.rowUBClp
		}

		release, waitSt := acquireCBCSlot(ctx, deadline, done)
		if release == nil {
			if ctx != nil && ctx.Err() != nil {
				return Result{}
			}
			return Result{
				modelID:   p.modelID,
				Backend:   BackendCBC,
				Status:    waitSt,
				SolveTime: time.Since(start),
			}
		}
		primal := make([]float64, n)
		duals := make([]float64, m)
		rc := make([]float64, n)
		var cRes C.GoblinResult
		runLP := func(presolveFlag C.int, maxIt int) (timedOut bool) {
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
			C.goblin_solve_lp(
				C.int(n),
				C.int(m),
				cInt32Ptr(colPtr32),
				cInt32Ptr(rowIdx32),
				cFloat64Ptr(colVal),
				cFloat64Ptr(colLBClp),
				cFloat64Ptr(colUBClp),
				cFloat64Ptr(objMinClp),
				cFloat64Ptr(rowLBClp),
				cFloat64Ptr(rowUBClp),
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
				pc.restoreLPPrimalDuals(p, nil, nil, primal, duals, rc)
			}
			return false
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
				if cbcPrimalWithinTol(p, p.varLB, p.varUB, nil, nil, nil, primal, false) &&
					!cbcLPDualWithinTol(p, p.varLB, p.varUB, nil, nil, nil, primal, duals, rc, objMin) {
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
			return Result{
				modelID:   p.modelID,
				Backend:   BackendCBC,
				Status:    StatusTimeLimit,
				SolveTime: time.Since(start),
			}
		}

		status := Status(cRes.status)
		if status == StatusUnbounded && !hasAnyInfiniteBound(p.varLB, p.varUB, n) {
			status = StatusNumericalError
		}
		res := Result{
			modelID:    p.modelID,
			Backend:    BackendCBC,
			Status:     status,
			Iterations: totalIters,
			SolveTime:  time.Since(start),
		}
		unresolvedCancel := (rowCancel || objCancel) && !droppedTiny && (!pc.active || pc.unresolved)
		tryLPFallback := func(acceptInfeasible bool) (Result, bool) {
			lpRef := NewLPSolver(p, nil)
			lpRef.DisableSIMD = opt.DisableSIMD
			lpRef.Deadline = deadline
			lpRef.Done = done
			if opt.MaxIter > 0 {
				rem := opt.MaxIter - totalIters
				if rem <= 0 {
					lpRef.Close()
					res.Status = StatusIterationLimit
					return res, true
				}
				lpRef.MaxIter = rem
			}
			refRes := lpRef.Solve()
			res.Iterations += refRes.Iterations
			lpRef.Close()
			if ctx != nil && ctx.Err() != nil {
				return Result{}, true
			}
			if refRes.Status == StatusOptimal && !math.IsNaN(refRes.Objective) && !math.IsInf(refRes.Objective, 0) {
				res.Status = StatusOptimal
				res.Objective = refRes.Objective
				res.BestBound = refRes.Objective
				res.RootLPBound = refRes.Objective
				res.RootCutBound = refRes.Objective
				res.Values = refRes.Primal
				res.Duals = refRes.Dual
				res.ReducedCosts = refRes.ReducedCosts
				return res, true
			}
			if refRes.Status == StatusUnbounded || refRes.Status == StatusTimeLimit || refRes.Status == StatusIterationLimit ||
				(acceptInfeasible && refRes.Status == StatusInfeasible) {
				res.Status = refRes.Status
				return res, true
			}
			return res, false
		}
		if status == StatusOptimal {
			snapCBCBounds(primal, p.varLB, p.varUB, n)
			objMin := evalLinearSum(0, p.objMin, primal, n)
			if !cbcPrimalWithinTol(p, p.varLB, p.varUB, nil, nil, nil, primal, false) {
				if unresolvedCancel {
					if fbRes, handled := tryLPFallback(true); handled {
						return fbRes
					}
				}
				res.Status = StatusNumericalError
				return res
			}
			if !cbcLPDualWithinTol(p, p.varLB, p.varUB, nil, nil, nil, primal, duals, rc, objMin) {
				if hasAnyInfiniteBound(p.varLB, p.varUB, n) || unresolvedCancel {
					if fbRes, handled := tryLPFallback(unresolvedCancel); handled {
						return fbRes
					}
				}
				res.Status = StatusNumericalError
				return res
			}
			obj := p.EvaluateObjective(primal)
			if math.IsNaN(obj) || math.IsInf(obj, 0) {
				res.Status = StatusNumericalError
				return res
			}
			if senseSign < 0 {
				for i := range m {
					duals[i] = -duals[i]
				}
				for j := range n {
					rc[j] = -rc[j]
				}
			}
			res.Objective = obj
			res.BestBound = obj
			res.RootLPBound = obj
			res.RootCutBound = obj
			res.Values = primal
			res.Duals = duals
			res.ReducedCosts = rc
			return res
		}
		if unresolvedCancel && (status == StatusInfeasible || status == StatusNumericalError) {
			if fbRes, handled := tryLPFallback(true); handled {
				return fbRes
			}
		}
		return res
	}

	rowCancel, objCancel := hasCBCCancellationRisk(p, p.varLB, p.varUB, nil, nil, nil)
	var pc cbcPrecond
	if (rowCancel || objCancel) && !hasCBCDroppedTiny(p, p.varLB, p.varUB, nil, nil) && !hasUnscalableCBCMIPObjective(p.objMin) {
		pc = preconditionCBC(p, p.varLB, p.varUB, nil, nil, nil, true, deadline, done)
		if ctx != nil && ctx.Err() != nil {
			return Result{}
		}
		if pc.timedOut {
			return Result{
				modelID:   p.modelID,
				Backend:   BackendCBC,
				Status:    StatusTimeLimit,
				SolveTime: time.Since(start),
			}
		}
		if pc.infeasible {
			return Result{
				modelID:   p.modelID,
				Backend:   BackendCBC,
				Status:    StatusInfeasible,
				SolveTime: time.Since(start),
			}
		}
		if !pc.active || pc.unresolved {
			goOpt := opt
			if opt.defaultMaxNodesUsed {
				goOpt.MaxNodes = 0
			}
			if opt.defaultCutPassesUsed {
				goOpt.MaxCutPasses = defaultMaxCutPasses
			}
			return solveCompensatedEngine(ctx, p, goOpt, BackendCBC, start, deadline, done)
		}
	}

	colPtr32 := p.colPtr32
	rowIdx32 := p.rowIdx32
	colVal := p.colVal
	colLBClp := p.colLBClp
	colUBClp := p.colUBClp
	objMinClp := p.objMin
	rowLBClp := p.rowLBClp
	rowUBClp := p.rowUBClp
	effRelGap := clampCBCGapTol(opt.RelGapTol)
	if pc.active {
		colPtr32 = pc.colPtr32
		rowIdx32 = pc.rowIdx32
		colVal = pc.colVal
		colLBClp = pc.colLBClp
		colUBClp = pc.colUBClp
		objMinClp = pc.objMin
		rowLBClp = pc.rowLBClp
		rowUBClp = pc.rowUBClp
	}
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
		return Result{
			modelID:   p.modelID,
			Backend:   BackendCBC,
			Status:    waitSt,
			SolveTime: time.Since(start),
		}
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
		C.goblin_solve_mip(
			C.int(n),
			C.int(m),
			cInt32Ptr(colPtr32),
			cInt32Ptr(rowIdx32),
			cFloat64Ptr(colVal),
			cFloat64Ptr(colLBClp),
			cFloat64Ptr(colUBClp),
			cFloat64Ptr(objMinClp),
			cFloat64Ptr(rowLBClp),
			cFloat64Ptr(rowUBClp),
			C.int(len(p.intCols32)),
			cInt32Ptr(p.intCols32),
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
		return false
	}()
	if ctx != nil && ctx.Err() != nil {
		return Result{}
	}
	if timedOutBeforeCall {
		return Result{
			modelID:   p.modelID,
			Backend:   BackendCBC,
			Status:    StatusTimeLimit,
			SolveTime: time.Since(start),
		}
	}
	rawRootLP := float64(cRes.root_lp_min)
	rawRootCut := float64(cRes.root_cut_min)
	rawBestBnd := float64(cRes.best_bound_min)
	if pc.active {
		pc.restoreMIPResult(p, primal, &cRes)
	}

	status := Status(cRes.status)
	if status == StatusUnbounded && !hasAnyInfiniteBound(p.varLB, p.varUB, n) {
		status = StatusNumericalError
	}
	if status == StatusUnbounded && len(p.intCols32) > 0 {
		feasProb := *p
		feasProb.objCoeffs = make([]float64, n)
		feasProb.objMin = make([]float64, n)
		feasProb.objConstant = 0
		feasProb.objConstMinVal = 0
		feasOpts := opt
		if opt.MaxIter > 0 {
			rem := opt.MaxIter - int(cRes.iterations)
			if rem <= 0 {
				return Result{
					modelID:    p.modelID,
					Backend:    BackendCBC,
					Status:     StatusIterationLimit,
					Nodes:      int(cRes.nodes),
					Iterations: int(cRes.iterations),
					CutsAdded:  int(cRes.cuts_added),
					SolveTime:  time.Since(start),
				}
			}
			feasOpts.MaxIter = rem
		}
		if !opt.defaultMaxNodesUsed && opt.MaxNodes > 0 {
			remNodes := opt.MaxNodes - int(cRes.nodes)
			if remNodes <= 0 {
				return Result{
					modelID:    p.modelID,
					Backend:    BackendCBC,
					Status:     StatusNodeLimit,
					Nodes:      int(cRes.nodes),
					Iterations: int(cRes.iterations),
					CutsAdded:  int(cRes.cuts_added),
					SolveTime:  time.Since(start),
				}
			}
			feasOpts.MaxNodes = remNodes
		}
		feasRes := solveCBC(ctx, &feasProb, feasOpts, start, deadline, done)
		if ctx != nil && ctx.Err() != nil {
			return Result{}
		}
		st := feasRes.Status
		if st == StatusOptimal {
			st = StatusUnbounded
		}
		return Result{
			modelID:    p.modelID,
			Backend:    BackendCBC,
			Status:     st,
			Nodes:      int(cRes.nodes) + feasRes.Nodes,
			Iterations: int(cRes.iterations) + feasRes.Iterations,
			CutsAdded:  int(cRes.cuts_added) + feasRes.CutsAdded,
			SolveTime:  time.Since(start),
		}
	}
	hasValidSol := len(primal) > 0 && cRes.has_sol != 0
	if hasValidSol {
		snapCBCBounds(primal, p.varLB, p.varUB, n)
		if !cbcPrimalWithinTol(p, p.varLB, p.varUB, nil, nil, nil, primal, true) {
			hasValidSol = false
			if status == StatusOptimal {
				status = StatusNumericalError
				cRes.has_best_bound = 0
				cRes.has_root_lp = 0
				cRes.has_root_cut = 0
			}
		}
	}
	if status == StatusOptimal && hasValidSol {
		if mipSt, extraIt := verifyCBCMIPOptimality(ctx, p, opt, int(cRes.iterations), cRes.has_root_lp != 0, float64(cRes.root_lp_min), primal, deadline, done); mipSt != StatusOptimal {
			cRes.iterations += C.int(extraIt)
			if ctx != nil && ctx.Err() != nil {
				return Result{}
			}
			if mipSt == StatusUnbounded {
				return Result{
					modelID:    p.modelID,
					Backend:    BackendCBC,
					Status:     StatusUnbounded,
					Nodes:      int(cRes.nodes),
					Iterations: int(cRes.iterations),
					CutsAdded:  int(cRes.cuts_added),
					SolveTime:  time.Since(start),
				}
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
		Iterations: int(cRes.iterations),
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
	if res.Status == StatusOptimal && (!hasValidSol || math.IsNaN(res.Objective) || math.IsInf(res.Objective, 0) || math.IsNaN(res.BestBound) || math.IsInf(res.BestBound, 0)) {
		return Result{
			modelID:    p.modelID,
			Backend:    BackendCBC,
			Status:     StatusNumericalError,
			Nodes:      res.Nodes,
			Iterations: res.Iterations,
			CutsAdded:  res.CutsAdded,
			SolveTime:  res.SolveTime,
		}
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
				return Result{modelID: p.modelID, Backend: BackendCBC, Status: StatusUnbounded, SolveTime: time.Since(start)}
			}
			primal[j] = ub
		} else if c > 0 {
			if math.IsInf(lb, -1) {
				return Result{modelID: p.modelID, Backend: BackendCBC, Status: StatusUnbounded, SolveTime: time.Since(start)}
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
			modelID:   p.modelID,
			Backend:   BackendCBC,
			Status:    StatusNumericalError,
			SolveTime: time.Since(start),
		}
	}
	var duals []float64
	if !p.isMIP {
		duals = []float64{}
	}
	return Result{
		modelID:      p.modelID,
		Backend:      BackendCBC,
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

// CBCLPSolver wraps a persistent C++ OsiClpSolverInterface instance that can be
// solved from scratch and repeatedly warm-started with modified bounds or added
// cutting planes. A single CBCLPSolver instance is not safe for concurrent use
// by multiple goroutines without external synchronization.
type CBCLPSolver struct {
	prob          *CompiledProblem
	handle        *cbcLPHandle
	cleanup       runtime.Cleanup
	curLB         []float64
	curUB         []float64
	cutCols       [][]int
	cutVals       [][]float64
	cutRHS        []float64
	numCut        int
	invalidBounds bool
	invalidCut    bool
	infeasibleCut bool
}

// NewCBCLPSolver creates a C++ OsiClpSolverInterface LP solver for p.
func NewCBCLPSolver(p *CompiledProblem) *CBCLPSolver {
	if p == nil {
		return &CBCLPSolver{}
	}
	var h *cbcLPHandle
	if p.numVars > 0 && validateCompiledForCBC(p) {
		cbcSem <- struct{}{}
		raw := C.goblin_lp_new(
			C.int(p.numVars),
			C.int(p.numConstraints),
			cInt32Ptr(p.colPtr32),
			cInt32Ptr(p.rowIdx32),
			cFloat64Ptr(p.colVal),
			cFloat64Ptr(p.colLBClp),
			cFloat64Ptr(p.colUBClp),
			cFloat64Ptr(p.objMin),
			cFloat64Ptr(p.rowLBClp),
			cFloat64Ptr(p.rowUBClp),
		)
		releaseCBCSlot()
		if raw != nil {
			h = &cbcLPHandle{ptr: raw}
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
	if len(lb) < n || len(ub) < n || len(s.curLB) < n || len(s.curUB) < n {
		return
	}
	for j := range n {
		if math.IsNaN(lb[j]) || math.IsNaN(ub[j]) || math.IsInf(lb[j], 1) || math.IsInf(ub[j], -1) {
			s.invalidBounds = true
			return
		}
	}
	s.invalidBounds = false
	lbClp := make([]float64, n)
	ubClp := make([]float64, n)
	for j := range n {
		s.curLB[j] = lb[j]
		s.curUB[j] = ub[j]
		lbClp[j] = clampCBCBound(lb[j])
		ubClp[j] = clampCBCBound(ub[j])
	}
	if s.handle != nil {
		cbcSem <- struct{}{}
		s.handle.mu.Lock()
		if s.handle.ptr != nil {
			C.goblin_lp_set_bounds(s.handle.ptr, C.int(n), cFloat64Ptr(lbClp), cFloat64Ptr(ubClp))
		}
		s.handle.mu.Unlock()
		releaseCBCSlot()
		runtime.KeepAlive(s)
	}
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
	if math.IsInf(rhs, 1) {
		return
	}
	if math.IsInf(rhs, -1) {
		s.infeasibleCut = true
		return
	}
	if n == 0 {
		s.cutRHS = append(s.cutRHS, rhs)
		s.numCut++
		return
	}
	if s.handle == nil {
		return
	}
	type termPair struct {
		col int
		val float64
	}
	pairs := make([]termPair, 0, kLim)
	for k := range kLim {
		c := colIdx[k]
		v := val[k]
		if c < 0 || c >= n || v == 0 {
			continue
		}
		pairs = append(pairs, termPair{col: c, val: v})
	}
	slices.SortFunc(pairs, func(a, b termPair) int { return cmp.Compare(a.col, b.col) })
	cols32 := make([]int32, 0, len(pairs))
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
	if len(cols32) > math.MaxInt32 {
		return
	}
	cbcSem <- struct{}{}
	s.handle.mu.Lock()
	var added C.int
	if s.handle.ptr != nil {
		added = C.goblin_lp_add_cut(
			s.handle.ptr,
			C.int(len(cols32)),
			cInt32Ptr(cols32),
			cFloat64Ptr(vals),
			C.double(clampCBCBound(rhs)),
		)
	}
	s.handle.mu.Unlock()
	releaseCBCSlot()
	if added != 0 {
		colsInt := make([]int, len(cols32))
		for i, c := range cols32 {
			colsInt[i] = int(c)
		}
		s.cutCols = append(s.cutCols, colsInt)
		s.cutVals = append(s.cutVals, vals)
		s.cutRHS = append(s.cutRHS, rhs)
		s.numCut++
	}
	runtime.KeepAlive(s)
}

// Solve solves or warm-start resolves the current LP relaxation via COIN-OR CLP.
func (s *CBCLPSolver) Solve() LPResult {
	if s == nil || s.invalidBounds || s.invalidCut {
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
		if s.curLB[j] > s.curUB[j]+cbcFeasTol {
			return LPResult{Status: StatusInfeasible}
		}
	}
	if n == 0 {
		for i := range p.numConstraints {
			if 0 < p.rowLB[i]-cbcFeasTol || 0 > p.rowUB[i]+cbcFeasTol {
				return LPResult{Status: StatusInfeasible}
			}
		}
		for _, rhs := range s.cutRHS {
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
	if s.handle == nil || hasUnrepresentableCBCFiniteBound(s.curLB, s.curUB, p.rowLB, p.rowUB, s.cutRHS) {
		return LPResult{Status: StatusNumericalError}
	}

	rowCancel, objCancel := hasCBCCancellationRisk(p, s.curLB, s.curUB, s.cutCols, s.cutVals, s.cutRHS)
	droppedTiny := false
	var pc cbcPrecond
	if rowCancel || objCancel {
		droppedTiny = hasCBCDroppedTiny(p, s.curLB, s.curUB, s.cutCols, s.cutVals)
		if !droppedTiny {
			pc = preconditionCBC(p, s.curLB, s.curUB, s.cutCols, s.cutVals, s.cutRHS, false, time.Time{}, nil)
			if pc.infeasible {
				return LPResult{Status: StatusInfeasible}
			}
		}
	}

	primal := make([]float64, n)
	duals := make([]float64, m)
	rc := make([]float64, n)
	var cRes C.GoblinResult

	cbcSem <- struct{}{}
	s.handle.mu.Lock()
	hasPtr := s.handle.ptr != nil
	totalIters := 0
	if hasPtr {
		solvePtr := s.handle.ptr
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
				for e := range len(pc.cutRHS) {
					cols32 := make([]int32, len(pc.cutCols[e]))
					for k, c := range pc.cutCols[e] {
						cols32[k] = int32(c)
					}
					if C.goblin_lp_add_cut(
						tmpPtr,
						C.int(len(cols32)),
						cInt32Ptr(cols32),
						cFloat64Ptr(pc.cutVals[e]),
						C.double(clampCBCBound(pc.cutRHS[e])),
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
				pc.restoreLPPrimalDuals(p, s.cutCols, s.cutVals, primal, duals, rc)
			}
			snapCBCBounds(primal, s.curLB, s.curUB, n)
			objMinTry := evalLinearSum(0, p.objMin, primal, n)
			if cbcPrimalWithinTol(p, s.curLB, s.curUB, s.cutCols, s.cutVals, s.cutRHS, primal, false) &&
				!cbcLPDualWithinTol(p, s.curLB, s.curUB, s.cutCols, s.cutVals, s.cutRHS, primal, duals, rc, objMinTry) {
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
					pc.restoreLPPrimalDuals(p, s.cutCols, s.cutVals, primal, duals, rc)
				}
				totalIters += int(cRes.iterations)
			}
		}
		if tmpPtr != nil {
			C.goblin_lp_free(tmpPtr)
		}
	}
	s.handle.mu.Unlock()
	releaseCBCSlot()
	runtime.KeepAlive(s)
	if !hasPtr {
		return LPResult{Status: StatusNumericalError}
	}

	status := Status(cRes.status)
	if status == StatusUnbounded && !hasAnyInfiniteBound(s.curLB, s.curUB, n) {
		status = StatusNumericalError
	}
	unresolvedCancel := (rowCancel || objCancel) && !droppedTiny && (!pc.active || pc.unresolved)
	tryGoFallback := func(acceptInfeasible bool) (LPResult, bool) {
		lpRef := NewLPSolver(p, nil)
		lpRef.SetStructuralBounds(s.curLB, s.curUB)
		for e := range min(len(s.cutCols), min(len(s.cutVals), len(s.cutRHS))) {
			lpRef.AddCut(s.cutCols[e], s.cutVals[e], s.cutRHS[e])
		}
		refRes := lpRef.Solve()
		totalIters += refRes.Iterations
		lpRef.Close()
		if refRes.Status == StatusOptimal && !math.IsNaN(refRes.Objective) && !math.IsInf(refRes.Objective, 0) {
			return LPResult{
				Status:       StatusOptimal,
				ObjectiveMin: refRes.ObjectiveMin,
				Objective:    refRes.Objective,
				Primal:       refRes.Primal,
				Dual:         refRes.Dual,
				ReducedCosts: refRes.ReducedCosts,
				Iterations:   totalIters,
			}, true
		}
		if refRes.Status == StatusUnbounded || (acceptInfeasible && refRes.Status == StatusInfeasible) {
			return LPResult{
				Status:     refRes.Status,
				Iterations: totalIters,
			}, true
		}
		return LPResult{}, false
	}
	if status != StatusOptimal {
		if unresolvedCancel && (status == StatusInfeasible || status == StatusNumericalError) {
			if fbRes, ok := tryGoFallback(true); ok {
				return fbRes
			}
		}
		return LPResult{
			Status:     status,
			Iterations: totalIters,
		}
	}

	snapCBCBounds(primal, s.curLB, s.curUB, n)
	objMin := evalLinearSum(0, p.objMin, primal, n)
	if !cbcPrimalWithinTol(p, s.curLB, s.curUB, s.cutCols, s.cutVals, s.cutRHS, primal, false) {
		if unresolvedCancel {
			if fbRes, ok := tryGoFallback(true); ok {
				return fbRes
			}
		}
		return LPResult{
			Status:     StatusNumericalError,
			Iterations: totalIters,
		}
	}
	if !cbcLPDualWithinTol(p, s.curLB, s.curUB, s.cutCols, s.cutVals, s.cutRHS, primal, duals, rc, objMin) {
		if hasAnyInfiniteBound(s.curLB, s.curUB, n) || unresolvedCancel {
			if fbRes, ok := tryGoFallback(unresolvedCancel); ok {
				return fbRes
			}
		}
		return LPResult{
			Status:     StatusNumericalError,
			Iterations: totalIters,
		}
	}

	userObj := p.EvaluateObjective(primal)
	if math.IsNaN(userObj) || math.IsInf(userObj, 0) {
		return LPResult{
			Status:     StatusNumericalError,
			Iterations: totalIters,
		}
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
	}
}

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

func hasUnrepresentableCBCFiniteBound(colLB, colUB, rowLB, rowUB, cutRHS []float64) bool {
	for _, s := range [][]float64{colLB, colUB, rowLB, rowUB, cutRHS} {
		for _, v := range s {
			if !math.IsInf(v, 0) && math.Abs(v) >= 1e29 {
				return true
			}
		}
	}
	return false
}

func verifyCBCMIPOptimality(
	ctx context.Context,
	p *CompiledProblem,
	opt Options,
	baseIters int,
	hasRootLP bool,
	rootLPMin float64,
	primal []float64,
	deadline time.Time,
	done <-chan struct{},
) (Status, int) {
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
			} else if 1e-5*(p.varUB[j]-p.varLB[j]) > gapTol {
				hasWideCont = true
			}
		} else if infBnd {
			hasInfInt = true
		}
	}
	if (hasInfCont || hasInfInt) && !hasTinyCoeffOrObj {
		for _, v := range p.colVal {
			if av := math.Abs(v); av > 0 && av <= 1e-7 {
				hasTinyCoeffOrObj = true
				break
			}
		}
	}
	needFixedLP := contNeg || hasWideCont || (hasInfCont && hasTinyCoeffOrObj)
	needRelaxLP := hasInfInt && (anyNeg || hasTinyCoeffOrObj)
	if !needFixedLP && !needRelaxLP {
		return StatusOptimal, 0
	}

	extraIters := 0
	if needFixedLP {
		if stopRequested(deadline, done) || (ctx != nil && ctx.Err() != nil) {
			return StatusTimeLimit, 0
		}
		lpFixed := NewLPSolver(p, nil)
		lpFixed.DisableSIMD = opt.DisableSIMD
		lpFixed.Deadline = deadline
		lpFixed.Done = done
		if opt.MaxIter > 0 {
			rem := opt.MaxIter - baseIters - extraIters
			if rem <= 0 {
				lpFixed.Close()
				return StatusIterationLimit, extraIters
			}
			lpFixed.MaxIter = rem
		}
		lbFix := slices.Clone(p.varLB)
		ubFix := slices.Clone(p.varUB)
		for _, col := range p.intCols {
			v := math.Round(primal[col])
			lbFix[col] = v
			ubFix[col] = v
		}
		lpFixed.SetStructuralBounds(lbFix, ubFix)
		resFix := lpFixed.Solve()
		extraIters += resFix.Iterations
		lpFixed.Close()
		switch resFix.Status {
		case StatusUnbounded:
			return StatusUnbounded, extraIters
		case StatusTimeLimit:
			return StatusTimeLimit, extraIters
		case StatusIterationLimit:
			return StatusIterationLimit, extraIters
		case StatusOptimal:
			if resFix.ObjectiveMin < curMin-gapTol {
				return StatusNumericalError, extraIters
			}
		default:
			return StatusNumericalError, extraIters
		}
	}

	if needRelaxLP {
		if stopRequested(deadline, done) || (ctx != nil && ctx.Err() != nil) {
			return StatusTimeLimit, extraIters
		}
		lpRelax := NewLPSolver(p, nil)
		lpRelax.DisableSIMD = opt.DisableSIMD
		lpRelax.Deadline = deadline
		lpRelax.Done = done
		if opt.MaxIter > 0 {
			rem := opt.MaxIter - baseIters - extraIters
			if rem <= 0 {
				lpRelax.Close()
				return StatusIterationLimit, extraIters
			}
			lpRelax.MaxIter = rem
		}
		resRelax := lpRelax.Solve()
		extraIters += resRelax.Iterations
		lpRelax.Close()
		switch resRelax.Status {
		case StatusUnbounded:
			return StatusUnbounded, extraIters
		case StatusTimeLimit:
			return StatusTimeLimit, extraIters
		case StatusIterationLimit:
			return StatusIterationLimit, extraIters
		case StatusNumericalError:
			return StatusNumericalError, extraIters
		case StatusOptimal:
			if hasRootLP && resRelax.ObjectiveMin < rootLPMin-gapTol {
				return StatusNumericalError, extraIters
			}
		}
	}
	return StatusOptimal, extraIters
}

func cbcPrimalWithinTol(p *CompiledProblem, lb, ub []float64, cutCols [][]int, cutVals [][]float64, cutRHS, primal []float64, checkInt bool) bool {
	n := p.numVars
	if len(primal) < n || len(lb) < n || len(ub) < n {
		return false
	}
	effLB := make([]float64, n)
	effUB := make([]float64, n)
	for j := range n {
		v := primal[j]
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
		bTol := max(1e-4, 1e-6*max(1.0, math.Abs(v)))
		if (!math.IsInf(lb[j], -1) && v < lb[j] && lb[j]-v > bTol) ||
			(!math.IsInf(ub[j], 1) && v > ub[j] && v-ub[j] > bTol) {
			return false
		}
		if checkInt && p.varCat[j] != Continuous {
			if math.Abs(v-math.Round(v)) > 1e-3 {
				return false
			}
		}
		effLB[j] = lb[j]
		effUB[j] = ub[j]
		if checkInt && p.varCat[j] != Continuous && v == math.Round(v) {
			effLB[j] = v
			effUB[j] = v
		}
	}
	for i := range p.numConstraints {
		st, en := p.rowPtr[i], p.rowPtr[i+1]
		rhs := p.rowRHS[i]
		res, rTolLo, rTolHi, ok := evalSparseRowResidualAndDirTol(p.colIdx[st:en], p.rowVal[st:en], primal, effLB, effUB, rhs, 1e-4, 1e-6)
		if !ok {
			return false
		}
		if (!math.IsInf(p.rowLB[i], -1) && res < -rTolLo) ||
			(!math.IsInf(p.rowUB[i], 1) && res > rTolHi) {
			return false
		}
	}
	for e := range min(len(cutCols), min(len(cutVals), len(cutRHS))) {
		rhs := cutRHS[e]
		res, _, rTolHi, ok := evalSparseRowResidualAndDirTol(cutCols[e], cutVals[e], primal, effLB, effUB, rhs, 1e-4, 1e-6)
		if !ok {
			return false
		}
		if res > rTolHi {
			return false
		}
	}
	return true
}

func cbcLPDualWithinTol(p *CompiledProblem, lb, ub []float64, cutCols [][]int, cutVals [][]float64, cutRHS, primal, yMin, rcMin []float64, objMin float64) bool {
	n := p.numVars
	baseM := p.numConstraints
	numExtra := min(len(cutCols), min(len(cutVals), len(cutRHS)))
	m := baseM + numExtra
	if len(yMin) < m || math.IsNaN(objMin) || math.IsInf(objMin, 0) {
		return false
	}
	maxDual := 1.0
	hasInfStruct := false
	hasTinyEntry := false
	for j := range n {
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
			for k, c := range cutCols[e] {
				if c == j {
					colRows = append(colRows, baseM+e)
					colVals = append(colVals, cutVals[e][k])
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
		if interior || (j < len(rcMin) && rcMin[j] == 0) {
			denSc := yds + math.Ldexp(math.Abs(p.objMin[j]), -32)
			if denSc > 0 {
				numSc := math.Ldexp(math.Abs(v)+ebj, -32)
				if rel := numSc / denSc; rel > maxBasisDualRel {
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

	rowRes := make([]float64, baseM)
	rowTols := make([]float64, baseM)
	for i := range baseM {
		yi := yMin[i]
		if yi == 0 {
			continue
		}
		st, en := p.rowPtr[i], p.rowPtr[i+1]
		res, rTol, ok := evalSparseRowResidualAndTol(p.colIdx[st:en], p.rowVal[st:en], primal, lb, ub, p.rowRHS[i], 1e-4, 1e-6)
		if !ok {
			return false
		}
		rowRes[i] = res
		rowTols[i] = rTol
		switch p.rowSense[i] {
		case LessEqual:
			if res < -10.0*rTol {
				if rel := math.Abs(yi) / maxDual; rel > maxBasisDualRel {
					maxBasisDualRel = rel
				}
			}
		case GreaterEqual:
			if res > 10.0*rTol {
				if rel := math.Abs(yi) / maxDual; rel > maxBasisDualRel {
					maxBasisDualRel = rel
				}
			}
		}
	}

	cutRes := make([]float64, numExtra)
	cutTols := make([]float64, numExtra)
	for e := range numExtra {
		yi := yMin[baseM+e]
		if yi == 0 {
			continue
		}
		rhs := cutRHS[e]
		res, rTol, ok := evalSparseRowResidualAndTol(cutCols[e], cutVals[e], primal, lb, ub, rhs, 1e-4, 1e-6)
		if !ok {
			return false
		}
		cutRes[e] = res
		cutTols[e] = rTol
		if res < -10.0*rTol {
			if rel := math.Abs(yi) / maxDual; rel > maxBasisDualRel {
				maxBasisDualRel = rel
			}
		}
	}

	for i := range baseM {
		yi := yMin[i]
		if yi == 0 {
			continue
		}
		st, en := p.rowPtr[i], p.rowPtr[i+1]
		cols := p.colIdx[st:en]
		vals := p.rowVal[st:en]
		res := rowRes[i]
		rTol := rowTols[i]
		switch p.rowSense[i] {
		case LessEqual:
			rSignTol := signTol
			if rowSlackUnbounded(cols, vals, true) {
				rSignTol = combineDualNoiseTol(0, 0, math.Ldexp(math.Abs(yi), -32), maxBasisDualRel, 64.0)
			}
			if yi > rSignTol {
				return false
			}
			if yi < -slackNoise && res < -rTol {
				compGap += (-yi - slackNoise) * (-res - rTol)
			}
		case GreaterEqual:
			rSignTol := signTol
			if rowSlackUnbounded(cols, vals, false) {
				rSignTol = combineDualNoiseTol(0, 0, math.Ldexp(math.Abs(yi), -32), maxBasisDualRel, 64.0)
			}
			if yi < -rSignTol {
				return false
			}
			if yi > slackNoise && res > rTol {
				compGap += (yi - slackNoise) * (res - rTol)
			}
		}
	}
	for e := range numExtra {
		yi := yMin[baseM+e]
		if yi == 0 {
			continue
		}
		rSignTol := signTol
		if rowSlackUnbounded(cutCols[e], cutVals[e], true) {
			rSignTol = combineDualNoiseTol(0, 0, math.Ldexp(math.Abs(yi), -32), maxBasisDualRel, 64.0)
		}
		if yi > rSignTol {
			return false
		}
		if yi < -slackNoise {
			res := cutRes[e]
			rTol := cutTols[e]
			if res < -rTol {
				compGap += (-yi - slackNoise) * (-res - rTol)
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
	workUB         []float64
	objMin         []float64
	rowLBClp       []float64
	rowUBClp       []float64
	cutCols        [][]int
	cutVals        [][]float64
	cutRHS         []float64
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

func sparseExprHasCancelRisk(cols []int, vals, lb, ub []float64, varCat []VarCategory, rhsLB, rhsUB, threshold, infScale float64) bool {
	posSum, negSum, hasPosInf, hasNegInf, limbs := sparseExprCancelStats(cols, vals, lb, ub, varCat, rhsLB, rhsUB)
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

func problemImpliedScaleHint(p *CompiledProblem, lb, ub []float64, cutVals [][]float64, cutRHS []float64) (maxFin float64, hasSmallCoeff bool) {
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
	for _, r := range cutRHS {
		if !math.IsInf(r, 0) && math.Abs(r) > maxFin {
			maxFin = math.Abs(r)
		}
	}
	for _, cvs := range cutVals {
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

func hasCBCCancellationRisk(p *CompiledProblem, lb, ub []float64, cutCols [][]int, cutVals [][]float64, cutRHS []float64) (rowCancel, objCancel bool) {
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
			for e := range min(len(cutCols), min(len(cutVals), len(cutRHS))) {
				if sparseExprHasCancelRisk(cutCols[e], cutVals[e], bLB, bUB, p.varCat, math.Inf(-1), cutRHS[e], cbcCancelMagThreshold, infScale) {
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
	maxFin, hasSmallCoeff := problemImpliedScaleHint(p, lb, ub, cutVals, cutRHS)
	if maxFin < cbcCancelMagThreshold && !(hasSmallCoeff && maxFin > 0) {
		return false, false
	}
	tLB := slices.Clone(lb[:n])
	tUB := slices.Clone(ub[:n])
	infeas, _ := tightenContinuousBoundsCBC(p, cutCols, cutVals, cutRHS, tLB, tUB, cbcPrecondMaxTightenPasses, time.Time{}, nil)
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

func hasCBCDroppedTiny(p *CompiledProblem, lb, ub []float64, cutCols [][]int, cutVals [][]float64) bool {
	n := min(p.numVars, min(len(lb), len(ub)))
	objExp := 0
	if p.isMIP {
		objExp, _ = cbcMIPObjectiveScaleExp(p.objMin)
	}
	for j := range n {
		if lb[j] == 0 && ub[j] == 0 {
			continue
		}
		ao := math.Abs(p.objMin[j])
		if objExp > 0 {
			ao = math.Ldexp(ao, -objExp)
		}
		if ao > 0 && ao <= 1e-18 {
			return true
		}
	}
	for j := range n {
		if lb[j] >= ub[j] && lb[j] == 0 {
			continue
		}
		for k := p.colPtr[j]; k < p.colPtr[j+1]; k++ {
			if av := math.Abs(p.colVal[k]); av > 0 && av <= 1e-18 {
				return true
			}
		}
	}
	for e := range min(len(cutCols), len(cutVals)) {
		for k := range min(len(cutCols[e]), len(cutVals[e])) {
			c := cutCols[e][k]
			if uint(c) >= uint(n) || (lb[c] >= ub[c] && lb[c] == 0) {
				continue
			}
			if av := math.Abs(cutVals[e][k]); av > 0 && av <= 1e-18 {
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

func tightenRowSliceCBC(cols []int, vals []float64, rhs float64, sense ConstraintSense, lb, ub []float64, n int) (infeas, changed bool) {
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

	if hasUpper && minInfCount == 0 {
		var sw wideDotAcc
		sw.addVal(rhs)
		for k := range kLim {
			col := cols[k]
			if uint(col) >= uint(n) {
				continue
			}
			a := vals[k]
			if a > 0 {
				sw.addProd(-a, lb[col])
			} else if a < 0 {
				sw.addProd(-a, ub[col])
			}
		}
		if sTop, sRem, sOK := sw.finishDD(); sOK && sTop+sRem < -rowFeasTol {
			return true, false
		}
	}
	if hasLower && maxInfCount == 0 {
		var dw wideDotAcc
		dw.addVal(rhs)
		for k := range kLim {
			col := cols[k]
			if uint(col) >= uint(n) {
				continue
			}
			a := vals[k]
			if a > 0 {
				dw.addProd(-a, ub[col])
			} else if a < 0 {
				dw.addProd(-a, lb[col])
			}
		}
		if dTop, dRem, dOK := dw.finishDD(); dOK && dTop+dRem > rowFeasTol {
			return true, false
		}
	}

	if hasUpper && minInfCount <= 1 {
		for k := range kLim {
			col := cols[k]
			if uint(col) >= uint(n) {
				continue
			}
			a := vals[k]
			if math.Abs(a) <= cbcPrecondMinSafeCoeff || lb[col] >= ub[col] {
				continue
			}
			if minInfCount == 1 {
				if (a > 0 && !math.IsInf(lb[col], -1)) || (a < 0 && !math.IsInf(ub[col], 1)) {
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
				if at > 0 && !math.IsInf(lb[c], -1) {
					w.addProd(-at, lb[c])
				} else if at < 0 && !math.IsInf(ub[c], 1) {
					w.addProd(-at, ub[c])
				}
			}
			q, qLo, qHi, exact, qOK := divExactFromWide(&w, a)
			if !qOK {
				continue
			}
			infeasBnd, chg := applyQuotientBoundCBC(a > 0, q, qLo, qHi, exact, math.Abs(a), rowFeasTol, lb, ub, col)
			if infeasBnd {
				return true, false
			}
			if chg {
				changed = true
			}
		}
	}

	if hasLower && maxInfCount <= 1 {
		for k := range kLim {
			col := cols[k]
			if uint(col) >= uint(n) {
				continue
			}
			a := vals[k]
			if math.Abs(a) <= cbcPrecondMinSafeCoeff || lb[col] >= ub[col] {
				continue
			}
			if maxInfCount == 1 {
				if (a > 0 && !math.IsInf(ub[col], 1)) || (a < 0 && !math.IsInf(lb[col], -1)) {
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
				if at > 0 && !math.IsInf(ub[c], 1) {
					w.addProd(-at, ub[c])
				} else if at < 0 && !math.IsInf(lb[c], -1) {
					w.addProd(-at, lb[c])
				}
			}
			q, qLo, qHi, exact, qOK := divExactFromWide(&w, a)
			if !qOK {
				continue
			}
			infeasBnd, chg := applyQuotientBoundCBC(a < 0, q, qLo, qHi, exact, math.Abs(a), rowFeasTol, lb, ub, col)
			if infeasBnd {
				return true, false
			}
			if chg {
				changed = true
			}
		}
	}
	return false, changed
}

func tightenContinuousBoundsCBC(p *CompiledProblem, cutCols [][]int, cutVals [][]float64, cutRHS, lb, ub []float64, maxPasses int, deadline time.Time, done <-chan struct{}) (infeas, timedOut bool) {
	n := min(p.numVars, min(len(lb), len(ub)))
	m := p.numConstraints
	numExtra := min(len(cutCols), min(len(cutVals), len(cutRHS)))
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
			infRow, chgRow := tightenRowSliceCBC(p.colIdx[st:en], p.rowVal[st:en], p.rowRHS[i], p.rowSense[i], lb, ub, n)
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
			infCut, chgCut := tightenRowSliceCBC(cutCols[e], cutVals[e], cutRHS[e], LessEqual, lb, ub, n)
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

func applyQuotientBoundCBC(tightenUpper bool, q, qLo, qHi float64, exact bool, absA, rowFeasTol float64, lb, ub []float64, col int) (infeas, changed bool) {
	feasAllow := max(cbcFeasTol, rowFeasTol/absA)
	if tightenUpper {
		if qHi < lb[col]-feasAllow {
			return true, false
		}
		candUB := qHi + 1e-12*max(1.0, math.Abs(qHi))
		if exact {
			candUB = q
		}
		if candUB <= lb[col]+cbcPrecondRemTol {
			candUB = lb[col]
		}
		if candUB < ub[col]-1e-9 {
			ub[col] = candUB
			return false, true
		}
		return false, false
	}
	if qLo > ub[col]+feasAllow {
		return true, false
	}
	candLB := qLo - 1e-12*max(1.0, math.Abs(qLo))
	if exact {
		candLB = q
	}
	if candLB >= ub[col]-cbcPrecondRemTol {
		candLB = ub[col]
	}
	if candLB > lb[col]+1e-9 {
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

func cloneWideDotAcc(src *wideDotAcc) wideDotAcc {
	var dst wideDotAcc
	dst.nonFin = src.nonFin
	if src.hi != nil {
		dst.ensureInit()
		dst.hi = append(dst.hi, src.hi...)
		dst.mid = append(dst.mid, src.mid...)
		dst.lo = append(dst.lo, src.lo...)
	}
	return dst
}

func evalShiftedConstMin(fixedW *wideDotAcc, baseConstMin float64) float64 {
	if fixedW == nil || (fixedW.hi == nil && !fixedW.nonFin) {
		return baseConstMin
	}
	w := cloneWideDotAcc(fixedW)
	w.addVal(baseConstMin)
	return w.finish(math.NaN())
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

// preconditionCBC eliminates IEEE-754 cancellation from fixed columns, continuous implied-fixed
// MIP domains, and equality-coupled rows or objectives using Shewchuk expansions before passing
// the reduced matrix to COIN-OR CLP or CBC.
func preconditionCBC(
	p *CompiledProblem,
	lb, ub []float64,
	cutCols [][]int,
	cutVals [][]float64,
	cutRHS []float64,
	isMIP bool,
	deadline time.Time,
	done <-chan struct{},
) cbcPrecond {
	n := p.numVars
	baseM := p.numConstraints
	numExtra := min(len(cutCols), min(len(cutVals), len(cutRHS)))
	baseConstMin := p.objConstMin()

	scaleLB := slices.Clone(lb[:n])
	scaleUB := slices.Clone(ub[:n])
	infeas, timedOut := tightenContinuousBoundsCBC(p, cutCols, cutVals, cutRHS, scaleLB, scaleUB, cbcPrecondMaxTightenPasses, deadline, done)
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

	infScale, _ := problemImpliedScaleHint(p, scaleLB, scaleUB, cutVals, cutRHS)
	for j := range n {
		if !math.IsInf(scaleLB[j], 0) && math.Abs(scaleLB[j]) > infScale {
			infScale = math.Abs(scaleLB[j])
		}
		if !math.IsInf(scaleUB[j], 0) && math.Abs(scaleUB[j]) > infScale {
			infScale = math.Abs(scaleUB[j])
		}
	}

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
		for k := range min(len(cutCols[e]), len(cutVals[e])) {
			c := cutCols[e][k]
			if uint(c) < uint(n) {
				if av := math.Abs(cutVals[e][k]); av > 0 && av <= cbcPrecondMinSafeCoeff {
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
		origCols := p.colIdx[st:en]
		origVals := p.rowVal[st:en]
		hasFixInRow := false
		if anyFixed {
			for _, col := range origCols {
				if fixedCols[col] {
					hasFixInRow = true
					break
				}
			}
		}
		if hasFixInRow {
			var w wideDotAcc
			w.addVal(p.rowRHS[i])
			remCount := 0
			for k, col := range origCols {
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
					if !fixedCols[col] {
						newCols = append(newCols, col)
						newVals = append(newVals, origVals[k])
					}
				}
				wRows[i] = cbcWorkRow{
					sense: p.rowSense[i],
					rhs:   top,
					cols:  newCols,
					vals:  newVals,
				}
				modified = true
				continue
			}
		}
		wRows[i] = cbcWorkRow{
			sense: p.rowSense[i],
			rhs:   p.rowRHS[i],
			cols:  slices.Clone(origCols),
			vals:  slices.Clone(origVals),
		}
	}

	wCutCols := make([][]int, numExtra)
	wCutVals := make([][]float64, numExtra)
	wCutRHS := make([]float64, numExtra)
	for e := range numExtra {
		kLim := min(len(cutCols[e]), len(cutVals[e]))
		origCols := cutCols[e][:kLim]
		origVals := cutVals[e][:kLim]
		hasFixInCut := false
		if anyFixed {
			for _, col := range origCols {
				if uint(col) < uint(n) && fixedCols[col] {
					hasFixInCut = true
					break
				}
			}
		}
		if hasFixInCut {
			var w wideDotAcc
			w.addVal(cutRHS[e])
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
				wCutCols[e] = newCols
				wCutVals[e] = newVals
				wCutRHS[e] = top
				modified = true
				continue
			}
		}
		wCutCols[e] = slices.Clone(origCols)
		wCutVals[e] = slices.Clone(origVals)
		wCutRHS[e] = cutRHS[e]
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
	needEqBasis := denseObjHasCancelRisk(workObjMin, curConstMin, effLB, effUB, p.varCat, cbcCancelMagThreshold, infScale)
	if !needEqBasis {
		for i := range baseM {
			rLB, rUB := rowBoundsFromSense(wRows[i].sense, wRows[i].rhs)
			if sparseExprHasCancelRisk(wRows[i].cols, wRows[i].vals, effLB, effUB, p.varCat, rLB, rUB, cbcCancelMagThreshold, infScale) {
				needEqBasis = true
				break
			}
		}
	}
	if !needEqBasis {
		for e := range numExtra {
			if sparseExprHasCancelRisk(wCutCols[e], wCutVals[e], effLB, effUB, p.varCat, math.Inf(-1), wCutRHS[e], cbcCancelMagThreshold, infScale) {
				needEqBasis = true
				break
			}
		}
	}

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
		reduceRowByBasis := func(rIdx int, eqBasis []cbcEqBasisRow) bool {
			oldNorm := sparseRowPotentialNorm(wRows[rIdx].cols, wRows[rIdx].vals, effLB, effUB, wRows[rIdx].rhs)
			candCols := wRows[rIdx].cols
			candVals := wRows[rIdx].vals
			candRHS := wRows[rIdx].rhs
			var candFixups []cbcDualFixup
			for bIdx := range eqBasis {
				b := &eqBasis[bIdx]
				if slices.Contains(b.combRows, rIdx) {
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
				stepFixups := make([]cbcDualFixup, 0, len(b.combRows))
				valid := true
				for t, eqR := range b.combRows {
					effMu := mu * b.combWeights[t]
					if math.IsNaN(effMu) || math.IsInf(effMu, 0) {
						valid = false
						break
					}
					if effMu != 0 {
						stepFixups = append(stepFixups, cbcDualFixup{isObj: false, eqRow: eqR, dstRow: rIdx, mu: effMu})
					}
				}
				if !valid {
					continue
				}
				candCols, candVals, candRHS = mCols, mVals, mRHS
				candFixups = append(candFixups, stepFixups...)
			}
			if len(candFixups) > 0 && sparseRowPotentialNorm(candCols, candVals, effLB, effUB, candRHS) <= cbcPrecondNormShrinkRatio*oldNorm {
				wRows[rIdx].cols = candCols
				wRows[rIdx].vals = candVals
				wRows[rIdx].rhs = candRHS
				dualFixups = append(dualFixups, candFixups...)
				modified = true
				return true
			}
			return false
		}

		if anyCancelEq {
			cleanBasis := buildCBCEqRREFBasis(wRows, cancelEqRow, effLB, effUB, n)
			if len(cleanBasis) > 0 {
				for rIdx := range baseM {
					if cancelEqRow[rIdx] {
						reduceRowByBasis(rIdx, cleanBasis)
					}
				}
			}
		}

		eqBasis := buildCBCEqRREFBasis(wRows, nil, effLB, effUB, n)
		if len(eqBasis) > 0 {
			if denseObjHasCancelRisk(workObjMin, curConstMin, effLB, effUB, p.varCat, cbcCancelMagThreshold, infScale) {
				oldNorm := denseObjPotentialNorm(workObjMin, curConstMin, effLB, effUB)
				candObj := slices.Clone(workObjMin)
				candFixedW := cloneWideDotAcc(&fixedObjW)
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
					stepFixups := make([]cbcDualFixup, 0, len(b.combRows))
					for t, eqR := range b.combRows {
						effMu := mu * b.combWeights[t]
						if math.IsNaN(effMu) || math.IsInf(effMu, 0) {
							valid = false
							break
						}
						if effMu != 0 {
							stepFixups = append(stepFixups, cbcDualFixup{isObj: true, eqRow: eqR, mu: effMu})
						}
					}
					if !valid {
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
				reduceRowByBasis(rIdx, eqBasis)
			}

			for e := range numExtra {
				if !sparseExprHasCancelRisk(wCutCols[e], wCutVals[e], effLB, effUB, p.varCat, math.Inf(-1), wCutRHS[e], cbcCancelMagThreshold, infScale) {
					continue
				}
				oldNorm := sparseRowPotentialNorm(wCutCols[e], wCutVals[e], effLB, effUB, wCutRHS[e])
				candCols := wCutCols[e]
				candVals := wCutVals[e]
				candRHS := wCutRHS[e]
				var candFixups []cbcDualFixup
				for bIdx := range eqBasis {
					b := &eqBasis[bIdx]
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
					stepFixups := make([]cbcDualFixup, 0, len(b.combRows))
					valid := true
					for t, eqR := range b.combRows {
						effMu := mu * b.combWeights[t]
						if math.IsNaN(effMu) || math.IsInf(effMu, 0) {
							valid = false
							break
						}
						if effMu != 0 {
							stepFixups = append(stepFixups, cbcDualFixup{isObj: false, eqRow: eqR, dstRow: baseM + e, mu: effMu})
						}
					}
					if !valid {
						continue
					}
					candCols, candVals, candRHS = mCols, mVals, mRHS
					candFixups = append(candFixups, stepFixups...)
				}
				if len(candFixups) > 0 && sparseRowPotentialNorm(candCols, candVals, effLB, effUB, candRHS) <= cbcPrecondNormShrinkRatio*oldNorm {
					wCutCols[e] = candCols
					wCutVals[e] = candVals
					wCutRHS[e] = candRHS
					dualFixups = append(dualFixups, candFixups...)
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
	unresolved := denseObjHasCancelRisk(workObjMin, curConstMin, effLB, effUB, p.varCat, cbcUnresolvedCancelFloor, infScale)
	if !unresolved {
		for i := range baseM {
			rLB, rUB := rowBoundsFromSense(wRows[i].sense, wRows[i].rhs)
			if sparseExprHasCancelRisk(wRows[i].cols, wRows[i].vals, effLB, effUB, p.varCat, rLB, rUB, cbcUnresolvedCancelFloor, infScale) {
				unresolved = true
				break
			}
		}
	}
	if !unresolved {
		for e := range numExtra {
			if sparseExprHasCancelRisk(wCutCols[e], wCutVals[e], effLB, effUB, p.varCat, math.Inf(-1), wCutRHS[e], cbcUnresolvedCancelFloor, infScale) {
				unresolved = true
				break
			}
		}
	}

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
		} else if fixedObjW.hi != nil {
			fixedHi = slices.Clone(compressExpansion(fixedObjW.hi))
			fixedMid = slices.Clone(compressExpansion(fixedObjW.mid))
			fixedLo = slices.Clone(compressExpansion(fixedObjW.lo))
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
		workUB:         workUB,
		objMin:         workObjMin,
		rowLBClp:       rowLBClp,
		rowUBClp:       rowUBClp,
		cutCols:        wCutCols,
		cutVals:        wCutVals,
		cutRHS:         wCutRHS,
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
	w.ensureInit()
	w.hi = append(w.hi, pc.fixedHi...)
	w.mid = append(w.mid, pc.fixedMid...)
	w.lo = append(w.lo, pc.fixedLo...)
	w.addVal(val)
	return w.finish(math.NaN())
}

func (pc *cbcPrecond) totalMinShift(p *CompiledProblem) float64 {
	baseConst := p.objConstMin()
	if !pc.active || !pc.hasFixedObj {
		return baseConst
	}
	if pc.fixedObjNonFin || math.IsNaN(baseConst) || math.IsInf(baseConst, 0) {
		return math.NaN()
	}
	var w wideDotAcc
	w.ensureInit()
	w.hi = append(w.hi, pc.fixedHi...)
	w.mid = append(w.mid, pc.fixedMid...)
	w.lo = append(w.lo, pc.fixedLo...)
	w.addVal(baseConst)
	return w.finish(math.NaN())
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
		w.ensureInit()
		w.hi = append(w.hi, pc.fixedHi...)
		w.mid = append(w.mid, pc.fixedMid...)
		w.lo = append(w.lo, pc.fixedLo...)
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

func (pc *cbcPrecond) restoreLPPrimalDuals(p *CompiledProblem, cutCols [][]int, cutVals [][]float64, primal, duals, rc []float64) {
	n := p.numVars
	baseM := p.numConstraints
	numExtra := min(len(cutCols), len(cutVals))
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
			for k := range min(len(cutCols[e]), len(cutVals[e])) {
				if cutCols[e][k] == j {
					colRows = append(colRows, baseM+e)
					colVals = append(colVals, cutVals[e][k])
					break
				}
			}
		}
		if v, _, _, _, ok := evalColDualDot(p.objMin[j], colRows, colVals, duals, nil, nil); ok {
			rc[j] = v
		}
	}
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
