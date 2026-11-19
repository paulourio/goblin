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
		return Result{
			modelID:      p.modelID,
			Backend:      BackendCBC,
			Status:       StatusOptimal,
			Objective:    p.objConstant,
			BestBound:    p.objConstant,
			RootLPBound:  p.objConstant,
			RootCutBound: p.objConstant,
			Values:       []float64{},
			Duals:        make([]float64, m),
			ReducedCosts: []float64{},
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

	if !validateCompiledForCBC(p) {
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
				cInt32Ptr(p.colPtr32),
				cInt32Ptr(p.rowIdx32),
				cFloat64Ptr(p.colVal),
				cFloat64Ptr(p.colLBClp),
				cFloat64Ptr(p.colUBClp),
				cFloat64Ptr(p.objMin),
				cFloat64Ptr(p.rowLBClp),
				cFloat64Ptr(p.rowUBClp),
				presolveFlag,
				clampCInt(maxIt),
				C.double(timeLimitSec),
				cancelPtr,
				&cRes,
				cFloat64Ptr(primal),
				cFloat64Ptr(duals),
				cFloat64Ptr(rc),
			)
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
		if status == StatusOptimal {
			snapCBCBounds(primal, p.varLB, p.varUB, n)
			objMin := evalLinearSum(0, p.objMin, primal, n)
			if !cbcPrimalWithinTol(p, p.varLB, p.varUB, nil, nil, nil, primal, false) {
				res.Status = StatusNumericalError
				return res
			}
			if !cbcLPDualWithinTol(p, p.varLB, p.varUB, nil, nil, nil, primal, duals, rc, objMin) {
				if hasAnyInfiniteBound(p.varLB, p.varUB, n) {
					lpRef := NewLPSolver(p, nil)
					lpRef.Deadline = deadline
					lpRef.Done = done
					if opt.MaxIter > 0 {
						rem := opt.MaxIter - totalIters
						if rem <= 0 {
							lpRef.Close()
							res.Status = StatusIterationLimit
							return res
						}
						lpRef.MaxIter = rem
					}
					refRes := lpRef.Solve()
					res.Iterations += refRes.Iterations
					lpRef.Close()
					if ctx != nil && ctx.Err() != nil {
						return Result{}
					}
					if refRes.Status == StatusUnbounded || refRes.Status == StatusTimeLimit || refRes.Status == StatusIterationLimit {
						res.Status = refRes.Status
						return res
					}
				}
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
			obj := p.EvaluateObjective(primal)
			res.Objective = obj
			res.BestBound = obj
			res.RootLPBound = obj
			res.RootCutBound = obj
			res.Values = primal
			res.Duals = duals
			res.ReducedCosts = rc
		}
		return res
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
			cInt32Ptr(p.colPtr32),
			cInt32Ptr(p.rowIdx32),
			cFloat64Ptr(p.colVal),
			cFloat64Ptr(p.colLBClp),
			cFloat64Ptr(p.colUBClp),
			cFloat64Ptr(p.objMin),
			cFloat64Ptr(p.rowLBClp),
			cFloat64Ptr(p.rowUBClp),
			C.int(len(p.intCols32)),
			cInt32Ptr(p.intCols32),
			boolToCInt(opt.EnablePresolve),
			boolToCInt(opt.EnableCuts),
			boolToCInt(opt.EnableHeuristics),
			clampCInt(maxNodes),
			clampCInt(cutPasses),
			clampCInt(opt.MaxIter),
			C.double(clampCBCGapTol(opt.RelGapTol)),
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

	status := Status(cRes.status)
	if status == StatusUnbounded && !hasAnyInfiniteBound(p.varLB, p.varUB, n) {
		status = StatusNumericalError
	}
	if status == StatusUnbounded && len(p.intCols32) > 0 {
		feasProb := *p
		feasProb.objCoeffs = make([]float64, n)
		feasProb.objMin = make([]float64, n)
		feasProb.objConstant = 0
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
		if mipSt, extraIt := verifyCBCMIPOptimality(ctx, p, opt, primal, deadline, done); mipSt != StatusOptimal {
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
		} else {
			cRes.iterations += C.int(extraIt)
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
	if cRes.has_root_lp != 0 {
		res.RootLPBound = p.ObjectiveFromMin(float64(cRes.root_lp_min))
	}
	if cRes.has_root_cut != 0 {
		res.RootCutBound = p.ObjectiveFromMin(float64(cRes.root_cut_min))
	}
	if cRes.has_best_bound != 0 {
		res.BestBound = p.ObjectiveFromMin(float64(cRes.best_bound_min))
	}
	if hasValidSol {
		obj := p.EvaluateObjective(primal)
		res.Objective = obj
		res.Values = primal
	}
	return res
}

func solveUnconstrainedCBC(p *CompiledProblem, start time.Time) Result {
	n := p.numVars
	primal := make([]float64, n)
	rc := make([]float64, n)
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
		rc[j] = senseSign * c
	}
	obj := p.EvaluateObjective(primal)
	return Result{
		modelID:      p.modelID,
		Backend:      BackendCBC,
		Status:       StatusOptimal,
		Objective:    obj,
		BestBound:    obj,
		RootLPBound:  obj,
		RootCutBound: obj,
		Values:       primal,
		Duals:        []float64{},
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
		if math.IsNaN(sum) || math.IsInf(sum, 0) {
			var w wideDotAcc
			for k := start; k < i; k++ {
				w.addVal(pairs[k].val)
			}
			sum = w.finish(sum)
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
		return LPResult{
			Status:       StatusOptimal,
			ObjectiveMin: 0,
			Objective:    p.objConstant,
			Primal:       []float64{},
			Dual:         make([]float64, p.numConstraints),
			ReducedCosts: []float64{},
		}
	}
	if s.handle == nil {
		return LPResult{Status: StatusNumericalError}
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
		C.goblin_lp_solve(
			s.handle.ptr,
			C.int(n),
			C.int(m),
			&cRes,
			cFloat64Ptr(primal),
			cFloat64Ptr(duals),
			cFloat64Ptr(rc),
		)
		totalIters = int(cRes.iterations)
		if Status(cRes.status) == StatusOptimal {
			snapCBCBounds(primal, s.curLB, s.curUB, n)
			objMinTry := evalLinearSum(0, p.objMin, primal, n)
			if cbcPrimalWithinTol(p, s.curLB, s.curUB, s.cutCols, s.cutVals, s.cutRHS, primal, false) &&
				!cbcLPDualWithinTol(p, s.curLB, s.curUB, s.cutCols, s.cutVals, s.cutRHS, primal, duals, rc, objMinTry) {
				C.goblin_lp_tighten_dual_tol(s.handle.ptr, 1e-16)
				C.goblin_lp_solve(
					s.handle.ptr,
					C.int(n),
					C.int(m),
					&cRes,
					cFloat64Ptr(primal),
					cFloat64Ptr(duals),
					cFloat64Ptr(rc),
				)
				totalIters += int(cRes.iterations)
			}
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
	if status != StatusOptimal {
		return LPResult{
			Status:     status,
			Iterations: totalIters,
		}
	}

	snapCBCBounds(primal, s.curLB, s.curUB, n)
	objMin := evalLinearSum(0, p.objMin, primal, n)
	if !cbcPrimalWithinTol(p, s.curLB, s.curUB, s.cutCols, s.cutVals, s.cutRHS, primal, false) {
		return LPResult{
			Status:     StatusNumericalError,
			Iterations: totalIters,
		}
	}
	if !cbcLPDualWithinTol(p, s.curLB, s.curUB, s.cutCols, s.cutVals, s.cutRHS, primal, duals, rc, objMin) {
		if hasAnyInfiniteBound(s.curLB, s.curUB, n) {
			lpRef := NewLPSolver(p, nil)
			lpRef.SetStructuralBounds(s.curLB, s.curUB)
			for e := range min(len(s.cutCols), min(len(s.cutVals), len(s.cutRHS))) {
				lpRef.AddCut(s.cutCols[e], s.cutVals[e], s.cutRHS[e])
			}
			refRes := lpRef.Solve()
			totalIters += refRes.Iterations
			lpRef.Close()
			if refRes.Status == StatusUnbounded {
				return LPResult{
					Status:     StatusUnbounded,
					Iterations: totalIters,
				}
			}
		}
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
		Objective:    p.ObjectiveFromMin(objMin),
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
		if !math.IsInf(lb[j], -1) && primal[j] < lb[j] && lb[j]-primal[j] <= cbcBoundSnapTol {
			primal[j] = lb[j]
		} else if !math.IsInf(ub[j], 1) && primal[j] > ub[j] && primal[j]-ub[j] <= cbcBoundSnapTol {
			primal[j] = ub[j]
		}
	}
}

func verifyCBCMIPOptimality(
	ctx context.Context,
	p *CompiledProblem,
	opt Options,
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
	hasInfCont := false
	hasInfInt := false
	hasWideCont := false
	for j := range n {
		if p.varLB[j] >= p.varUB[j] {
			continue
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
	if !hasInfCont && !hasWideCont && !hasInfInt {
		return StatusOptimal, 0
	}

	extraIters := 0
	if hasInfCont || hasWideCont {
		if stopRequested(deadline, done) || (ctx != nil && ctx.Err() != nil) {
			return StatusTimeLimit, 0
		}
		lpFixed := NewLPSolver(p, nil)
		lpFixed.Deadline = deadline
		lpFixed.Done = done
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
		case StatusOptimal:
			if resFix.ObjectiveMin < curMin-gapTol {
				return StatusNumericalError, extraIters
			}
		}
	}

	if hasInfInt {
		if stopRequested(deadline, done) || (ctx != nil && ctx.Err() != nil) {
			return StatusTimeLimit, extraIters
		}
		lpRelax := NewLPSolver(p, nil)
		lpRelax.Deadline = deadline
		lpRelax.Done = done
		resRelax := lpRelax.Solve()
		extraIters += resRelax.Iterations
		lpRelax.Close()
		if resRelax.Status == StatusUnbounded {
			return StatusUnbounded, extraIters
		}
		if resRelax.Status == StatusTimeLimit {
			return StatusTimeLimit, extraIters
		}
	}
	return StatusOptimal, extraIters
}

func cbcPrimalWithinTol(p *CompiledProblem, lb, ub []float64, cutCols [][]int, cutVals [][]float64, cutRHS, primal []float64, checkInt bool) bool {
	n := p.numVars
	if len(primal) < n || len(lb) < n || len(ub) < n {
		return false
	}
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
	}
	for i := range p.numConstraints {
		st, en := p.rowPtr[i], p.rowPtr[i+1]
		act, rTol, ok := evalSparseRowSumAndTol(p.colIdx[st:en], p.rowVal[st:en], primal, p.rowRHS[i], 1e-4, 1e-6)
		if !ok {
			return false
		}
		if (!math.IsInf(p.rowLB[i], -1) && act < p.rowLB[i] && p.rowLB[i]-act > rTol) ||
			(!math.IsInf(p.rowUB[i], 1) && act > p.rowUB[i] && act-p.rowUB[i] > rTol) {
			return false
		}
	}
	for e := range min(len(cutCols), min(len(cutVals), len(cutRHS))) {
		rhs := cutRHS[e]
		act, rTol, ok := evalSparseRowSumAndTol(cutCols[e], cutVals[e], primal, rhs, 1e-4, 1e-6)
		if !ok {
			return false
		}
		if act > rhs && act-rhs > rTol {
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
	for j := range n {
		if av := math.Abs(p.objMin[j]); av > maxDual {
			maxDual = av
		}
		if lb[j] < ub[j] && (math.IsInf(lb[j], -1) || math.IsInf(ub[j], 1)) {
			hasInfStruct = true
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

	rowSlackUnbounded := func(cols []int, vals []float64, toPosInf bool) bool {
		if !hasInfStruct {
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

	rowActs := make([]float64, baseM)
	rowTols := make([]float64, baseM)
	for i := range baseM {
		yi := yMin[i]
		if yi == 0 {
			continue
		}
		st, en := p.rowPtr[i], p.rowPtr[i+1]
		act, rTol, ok := evalSparseRowSumAndTol(p.colIdx[st:en], p.rowVal[st:en], primal, p.rowRHS[i], 1e-4, 1e-6)
		if !ok {
			return false
		}
		rowActs[i] = act
		rowTols[i] = rTol
		switch p.rowSense[i] {
		case LessEqual:
			if act < p.rowRHS[i]-10.0*rTol {
				if rel := math.Abs(yi) / maxDual; rel > maxBasisDualRel {
					maxBasisDualRel = rel
				}
			}
		case GreaterEqual:
			if act > p.rowRHS[i]+10.0*rTol {
				if rel := math.Abs(yi) / maxDual; rel > maxBasisDualRel {
					maxBasisDualRel = rel
				}
			}
		}
	}

	cutActs := make([]float64, numExtra)
	cutTols := make([]float64, numExtra)
	for e := range numExtra {
		yi := yMin[baseM+e]
		if yi == 0 {
			continue
		}
		rhs := cutRHS[e]
		act, rTol, ok := evalSparseRowSumAndTol(cutCols[e], cutVals[e], primal, rhs, 1e-4, 1e-6)
		if !ok {
			return false
		}
		cutActs[e] = act
		cutTols[e] = rTol
		if act < rhs-10.0*rTol {
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
		act := rowActs[i]
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
			if yi < -slackNoise && act < p.rowRHS[i] {
				if diff := p.rowRHS[i] - act; diff > rTol {
					compGap += (-yi - slackNoise) * (diff - rTol)
				}
			}
		case GreaterEqual:
			rSignTol := signTol
			if rowSlackUnbounded(cols, vals, false) {
				rSignTol = combineDualNoiseTol(0, 0, math.Ldexp(math.Abs(yi), -32), maxBasisDualRel, 64.0)
			}
			if yi < -rSignTol {
				return false
			}
			if yi > slackNoise && act > p.rowRHS[i] {
				if diff := act - p.rowRHS[i]; diff > rTol {
					compGap += (yi - slackNoise) * (diff - rTol)
				}
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
			rhs := cutRHS[e]
			act := cutActs[e]
			rTol := cutTols[e]
			if act < rhs {
				if diff := rhs - act; diff > rTol {
					compGap += (-yi - slackNoise) * (diff - rTol)
				}
			}
		}
	}

	for j := range n {
		if lb[j] == ub[j] {
			continue
		}
		colL1Sc := yDotSc[j] + math.Ldexp(math.Abs(p.objMin[j]), -32)
		infTol := combineDualNoiseTol(eb[j], 0, colL1Sc, maxBasisDualRel, 64.0)
		if math.IsInf(ub[j], 1) && dj[j] < -infTol {
			return false
		}
		if math.IsInf(lb[j], -1) && dj[j] > infTol {
			return false
		}
		colNoise := eb[j] + math.Ldexp(1e-8*colL1Sc, 32)
		if math.IsInf(colNoise, 1) {
			colNoise = math.MaxFloat64
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
