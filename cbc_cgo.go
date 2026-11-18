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
	cbcUnboundedBound   = 1e48
	cbcFeasTol          = 1e-7
	cbcBoundSnapTol     = 1e-6
)

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
				boolToCInt(opt.EnablePresolve),
				clampCInt(opt.MaxIter),
				C.double(timeLimitSec),
				cancelPtr,
				&cRes,
				cFloat64Ptr(primal),
				cFloat64Ptr(duals),
				cFloat64Ptr(rc),
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
		res := Result{
			modelID:    p.modelID,
			Backend:    BackendCBC,
			Status:     status,
			Iterations: int(cRes.iterations),
			SolveTime:  time.Since(start),
		}
		if status == StatusOptimal {
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
			C.double(opt.RelGapTol),
			C.double(opt.AbsGapTol),
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
	res := Result{
		modelID:      p.modelID,
		Backend:      BackendCBC,
		Status:       status,
		RootLPBound:  p.ObjectiveFromMin(float64(cRes.root_lp_min)),
		RootCutBound: p.ObjectiveFromMin(float64(cRes.root_cut_min)),
		Nodes:        int(cRes.nodes),
		Iterations:   int(cRes.iterations),
		CutsAdded:    int(cRes.cuts_added),
		SolveTime:    time.Since(start),
	}
	if math.Abs(float64(cRes.best_bound_min)) < cbcUnboundedBound {
		res.BestBound = p.ObjectiveFromMin(float64(cRes.best_bound_min))
	}
	if len(primal) > 0 && cRes.has_sol != 0 {
		obj := p.EvaluateObjective(primal)
		res.Objective = obj
		res.Values = primal
		if status == StatusOptimal {
			res.BestBound = obj
		}
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
	prob    *CompiledProblem
	handle  *cbcLPHandle
	cleanup runtime.Cleanup
	curLB   []float64
	curUB   []float64
	cutRHS  []float64
	numCut  int
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
	if s == nil || s.prob == nil || math.IsNaN(rhs) || math.IsInf(rhs, 1) {
		return
	}
	n := s.prob.numVars
	if n == 0 {
		s.cutRHS = append(s.cutRHS, rhs)
		s.numCut++
		return
	}
	if s.handle == nil {
		return
	}
	kLim := min(len(colIdx), len(val))
	type termPair struct {
		col int
		val float64
	}
	pairs := make([]termPair, 0, kLim)
	for k := range kLim {
		c := colIdx[k]
		v := val[k]
		if c < 0 || c >= n || v == 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		pairs = append(pairs, termPair{col: c, val: v})
	}
	slices.SortFunc(pairs, func(a, b termPair) int { return cmp.Compare(a.col, b.col) })
	cols32 := make([]int32, 0, len(pairs))
	vals := make([]float64, 0, len(pairs))
	for i := 0; i < len(pairs); {
		c := pairs[i].col
		sum := pairs[i].val
		for i++; i < len(pairs) && pairs[i].col == c; i++ {
			sum += pairs[i].val
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
		s.cutRHS = append(s.cutRHS, rhs)
		s.numCut++
	}
	runtime.KeepAlive(s)
}

// Solve solves or warm-start resolves the current LP relaxation via COIN-OR CLP.
func (s *CBCLPSolver) Solve() LPResult {
	if s == nil {
		return LPResult{Status: StatusUndefined}
	}
	p := s.prob
	if p == nil || p.invalidErr != nil {
		return LPResult{Status: StatusUndefined}
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
	}
	s.handle.mu.Unlock()
	releaseCBCSlot()
	runtime.KeepAlive(s)
	if !hasPtr {
		return LPResult{Status: StatusNumericalError}
	}

	status := Status(cRes.status)
	if status != StatusOptimal {
		return LPResult{
			Status:     status,
			Iterations: int(cRes.iterations),
		}
	}

	senseSign := 1.0
	if p.sense == Maximize {
		senseSign = -1.0
	}
	if senseSign < 0 {
		for i := range m {
			duals[i] = -duals[i]
		}
		for j := range n {
			rc[j] = -rc[j]
		}
	}

	objMin := 0.0
	for j := range n {
		if !math.IsInf(s.curLB[j], -1) && primal[j] < s.curLB[j] && s.curLB[j]-primal[j] <= cbcBoundSnapTol {
			primal[j] = s.curLB[j]
		} else if !math.IsInf(s.curUB[j], 1) && primal[j] > s.curUB[j] && primal[j]-s.curUB[j] <= cbcBoundSnapTol {
			primal[j] = s.curUB[j]
		}
		objMin += p.objMin[j] * primal[j]
	}

	return LPResult{
		Status:       StatusOptimal,
		ObjectiveMin: objMin,
		Objective:    p.ObjectiveFromMin(objMin),
		Primal:       primal,
		Dual:         duals[:p.numConstraints],
		ReducedCosts: rc,
		Iterations:   int(cRes.iterations),
	}
}
