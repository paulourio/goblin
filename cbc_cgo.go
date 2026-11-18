//go:build cgo && cbc

package goblin

/*
#cgo CXXFLAGS: -std=gnu++14 -O3 -I${SRCDIR}/third_party/Cbc/dist/include/coin-or
#cgo LDFLAGS: -L${SRCDIR}/third_party/Cbc/dist/lib -Wl,-rpath,${SRCDIR}/third_party/Cbc/dist/lib -lOsiCbc -lCbc -lCgl -lOsiClp -lClp -lOsi -lCoinUtils -lbz2 -lz -llapack -lblas -lpthread -lm
#cgo linux LDFLAGS: -lrt
#include "cbc_bridge.h"
*/
import "C"

import (
	"cmp"
	"context"
	"math"
	"runtime"
	"slices"
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
)

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

// CBCAvailable reports whether the optional C++ COIN-OR CBC/CLP backend (-tags cbc with cgo enabled) is compiled in.
func CBCAvailable() bool { return true }

func solveCBC(ctx context.Context, p *CompiledProblem, opt Options, start, deadline time.Time, done <-chan struct{}) Result {
	n := p.NumVars
	m := p.NumConstraints
	if n > math.MaxInt32 || m > math.MaxInt32 || len(p.ColVal) > math.MaxInt32 ||
		(m > 0 && len(p.ColPtr32) != n+1) {
		return Result{
			modelID:   p.modelID,
			Backend:   BackendCBC,
			Status:    StatusNumericalError,
			SolveTime: time.Since(start),
		}
	}

	for j := range n {
		if p.VarLB[j] > p.VarUB[j]+cbcFeasTol {
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
			if 0 < p.RowLB[i]-cbcFeasTol || 0 > p.RowUB[i]+cbcFeasTol {
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
			Objective:    p.ObjConstant,
			BestBound:    p.ObjConstant,
			RootLPBound:  p.ObjConstant,
			RootCutBound: p.ObjConstant,
			Values:       []float64{},
			Duals:        make([]float64, m),
			ReducedCosts: []float64{},
			SolveTime:    time.Since(start),
		}
	}

	if m == 0 {
		return solveUnconstrainedCBC(p, start)
	}

	var cancelFlag C.int
	var cancelPtr *C.int
	timeLimitSec := 0.0
	if !deadline.IsZero() {
		rem := time.Until(deadline)
		if rem <= 0 {
			return Result{
				modelID:   p.modelID,
				Backend:   BackendCBC,
				Status:    StatusTimeLimit,
				SolveTime: time.Since(start),
			}
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

	senseSign := 1.0
	if p.Sense == Maximize {
		senseSign = -1.0
	}

	if !p.IsMIP {
		primal := make([]float64, n)
		duals := make([]float64, m)
		rc := make([]float64, n)
		var cRes C.GoblinResult

		C.goblin_solve_lp(
			C.int(n),
			C.int(m),
			cInt32Ptr(p.ColPtr32),
			cInt32Ptr(p.RowIdx32),
			cFloat64Ptr(p.ColVal),
			cFloat64Ptr(p.ColLBClp),
			cFloat64Ptr(p.ColUBClp),
			cFloat64Ptr(p.ObjMin),
			cFloat64Ptr(p.RowLBClp),
			cFloat64Ptr(p.RowUBClp),
			boolToCInt(opt.EnablePresolve),
			C.int(opt.MaxIter),
			C.double(timeLimitSec),
			cancelPtr,
			&cRes,
			cFloat64Ptr(primal),
			cFloat64Ptr(duals),
			cFloat64Ptr(rc),
		)
		if ctx != nil && ctx.Err() != nil {
			return Result{}
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

	primal := make([]float64, n)
	var cRes C.GoblinResult

	// Map `DefaultOptions` (`MaxCutPasses=12`, `MaxNodes=50000`) to CBC's standard root cut pass limit (30)
	// and unlimited nodes unless explicitly overridden by the caller.
	cutPasses := opt.MaxCutPasses
	if cutPasses == defaultMaxCutPasses {
		cutPasses = cbcDefaultCutPasses
	}
	maxNodes := opt.MaxNodes
	if maxNodes == defaultMaxNodes {
		maxNodes = -1
	}

	C.goblin_solve_mip(
		C.int(n),
		C.int(m),
		cInt32Ptr(p.ColPtr32),
		cInt32Ptr(p.RowIdx32),
		cFloat64Ptr(p.ColVal),
		cFloat64Ptr(p.ColLBClp),
		cFloat64Ptr(p.ColUBClp),
		cFloat64Ptr(p.ObjMin),
		cFloat64Ptr(p.RowLBClp),
		cFloat64Ptr(p.RowUBClp),
		C.int(len(p.IntCols32)),
		cInt32Ptr(p.IntCols32),
		boolToCInt(opt.EnablePresolve),
		boolToCInt(opt.EnableCuts),
		boolToCInt(opt.EnableHeuristics),
		C.int(maxNodes),
		C.int(cutPasses),
		C.int(opt.MaxIter),
		C.double(opt.RelGapTol),
		C.double(opt.AbsGapTol),
		C.double(timeLimitSec),
		cancelPtr,
		&cRes,
		cFloat64Ptr(primal),
	)
	if ctx != nil && ctx.Err() != nil {
		return Result{}
	}

	status := Status(cRes.status)
	if status == StatusUnbounded && len(p.IntCols32) > 0 {
		feasProb := *p
		feasProb.ObjCoeffs = make([]float64, n)
		feasProb.ObjMin = make([]float64, n)
		feasProb.ObjConstant = 0
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
	n := p.NumVars
	primal := make([]float64, n)
	rc := make([]float64, n)
	senseSign := 1.0
	if p.Sense == Maximize {
		senseSign = -1.0
	}
	for j := range n {
		c := p.ObjMin[j]
		lb, ub := p.VarLB[j], p.VarUB[j]
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
		if p.VarCat[j] != Continuous {
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

// CBCLPSolver wraps a persistent C++ OsiClpSolverInterface instance that can be
// solved from scratch and repeatedly warm-started with modified bounds or added
// cutting planes. A single CBCLPSolver instance is not safe for concurrent use
// by multiple goroutines without external synchronization.
type CBCLPSolver struct {
	Prob   *CompiledProblem
	handle *C.GoblinLPSolver
	curLB  []float64
	curUB  []float64
	cutRHS []float64
	numCut int
}

// NewCBCLPSolver creates a C++ OsiClpSolverInterface LP solver for p.
func NewCBCLPSolver(p *CompiledProblem) *CBCLPSolver {
	if p == nil {
		return &CBCLPSolver{}
	}
	var h *C.GoblinLPSolver
	if p.NumVars <= math.MaxInt32 && p.NumConstraints <= math.MaxInt32 &&
		len(p.ColVal) <= math.MaxInt32 && len(p.ColPtr32) == p.NumVars+1 {
		h = C.goblin_lp_new(
			C.int(p.NumVars),
			C.int(p.NumConstraints),
			cInt32Ptr(p.ColPtr32),
			cInt32Ptr(p.RowIdx32),
			cFloat64Ptr(p.ColVal),
			cFloat64Ptr(p.ColLBClp),
			cFloat64Ptr(p.ColUBClp),
			cFloat64Ptr(p.ObjMin),
			cFloat64Ptr(p.RowLBClp),
			cFloat64Ptr(p.RowUBClp),
		)
	}
	s := &CBCLPSolver{
		Prob:   p,
		handle: h,
		curLB:  slices.Clone(p.VarLB),
		curUB:  slices.Clone(p.VarUB),
	}
	runtime.SetFinalizer(s, (*CBCLPSolver).Close)
	return s
}

// Close releases the underlying C++ OsiClpSolverInterface instance.
func (s *CBCLPSolver) Close() {
	if s != nil && s.handle != nil {
		C.goblin_lp_free(s.handle)
		s.handle = nil
		runtime.KeepAlive(s)
	}
}

// TotalRows returns the total number of rows (structural constraints + added cuts).
func (s *CBCLPSolver) TotalRows() int {
	if s == nil || s.Prob == nil {
		return 0
	}
	return s.Prob.NumConstraints + s.numCut
}

// SetStructuralBounds updates the column bounds in place while preserving the
// current simplex basis for warm-started dual or primal resolves.
func (s *CBCLPSolver) SetStructuralBounds(lb, ub []float64) {
	if s == nil || s.Prob == nil {
		return
	}
	n := s.Prob.NumVars
	lbClp := make([]float64, n)
	ubClp := make([]float64, n)
	for j := range n {
		s.curLB[j] = lb[j]
		s.curUB[j] = ub[j]
		lbClp[j] = clampCBCBound(lb[j])
		ubClp[j] = clampCBCBound(ub[j])
	}
	if s.handle != nil {
		C.goblin_lp_set_bounds(s.handle, C.int(n), cFloat64Ptr(lbClp), cFloat64Ptr(ubClp))
		runtime.KeepAlive(s)
	}
}

// AddCut appends a linear inequality sum(val[k] * x[colIdx[k]]) <= rhs to the LP
// while preserving the existing basis for warm-started dual simplex resolve.
// Out-of-range indices are ignored and duplicate column indices are summed.
func (s *CBCLPSolver) AddCut(colIdx []int, val []float64, rhs float64) {
	if s == nil || s.Prob == nil || s.handle == nil {
		return
	}
	n := s.Prob.NumVars
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
		if math.Abs(sum) <= compileZeroTol {
			continue
		}
		cols32 = append(cols32, int32(c))
		vals = append(vals, sum)
	}
	C.goblin_lp_add_cut(
		s.handle,
		C.int(len(cols32)),
		cInt32Ptr(cols32),
		cFloat64Ptr(vals),
		C.double(clampCBCBound(rhs)),
	)
	s.cutRHS = append(s.cutRHS, rhs)
	s.numCut++
	runtime.KeepAlive(s)
}

// Solve solves or warm-start resolves the current LP relaxation via COIN-OR CLP.
func (s *CBCLPSolver) Solve() LPResult {
	if s == nil {
		return LPResult{Status: StatusUndefined}
	}
	p := s.Prob
	if p == nil || p.InvalidErr != nil {
		return LPResult{Status: StatusUndefined}
	}
	n := p.NumVars
	m := s.TotalRows()

	for j := range n {
		if s.curLB[j] > s.curUB[j]+cbcFeasTol {
			return LPResult{Status: StatusInfeasible}
		}
	}
	if n == 0 {
		for i := range p.NumConstraints {
			if 0 < p.RowLB[i]-cbcFeasTol || 0 > p.RowUB[i]+cbcFeasTol {
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
			Objective:    p.ObjConstant,
			Primal:       []float64{},
			Dual:         make([]float64, p.NumConstraints),
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

	C.goblin_lp_solve(
		s.handle,
		&cRes,
		cFloat64Ptr(primal),
		cFloat64Ptr(duals),
		cFloat64Ptr(rc),
	)
	runtime.KeepAlive(s)

	status := Status(cRes.status)
	if status != StatusOptimal {
		return LPResult{
			Status:     status,
			Iterations: int(cRes.iterations),
		}
	}

	senseSign := 1.0
	if p.Sense == Maximize {
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
		objMin += p.ObjMin[j] * primal[j]
	}

	return LPResult{
		Status:       StatusOptimal,
		ObjectiveMin: objMin,
		Objective:    p.ObjectiveFromMin(objMin),
		Primal:       primal,
		Dual:         duals[:p.NumConstraints],
		ReducedCosts: rc,
		Iterations:   int(cRes.iterations),
	}
}
