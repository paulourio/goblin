//go:build !cgo || !cbc

package goblin

import (
	"context"
	"time"
)

// CBCAvailable reports whether the optional C++ COIN-OR CBC/CLP backend (-tags cbc with cgo enabled) is compiled in.
// Without -tags cbc (or under CGO_ENABLED=0) this stub returns false, and callers requesting BackendCBC
// receive StatusUndefined without invoking C++ symbols.
func CBCAvailable() bool { return false }

func solveCBC(_ context.Context, p *CompiledProblem, _ Options, start, _ time.Time, _ <-chan struct{}) Result {
	var id uint64
	if p != nil {
		id = p.modelID
	}
	return Result{
		modelID:   id,
		Backend:   BackendCBC,
		Status:    StatusUndefined,
		SolveTime: time.Since(start),
	}
}

// CBCLPSolver is a no-op stub when built without -tags cbc or with CGO_ENABLED=0.
type CBCLPSolver struct {
	prob *CompiledProblem
}

// NewCBCLPSolver returns a stub solver.
func NewCBCLPSolver(p *CompiledProblem) *CBCLPSolver {
	if p == nil {
		return &CBCLPSolver{}
	}
	return &CBCLPSolver{prob: p}
}

// Close is a no-op stub.
func (s *CBCLPSolver) Close() {}

// NumVars returns the number of structural columns in the problem, or 0 when nil.
func (s *CBCLPSolver) NumVars() int {
	if s == nil || s.prob == nil {
		return 0
	}
	return s.prob.numVars
}

// NumCuts returns the number of cuts added to the solver.
func (s *CBCLPSolver) NumCuts() int {
	return 0
}

// TotalRows returns the number of structural rows and added cuts, or 0 when nil.
func (s *CBCLPSolver) TotalRows() int {
	if s == nil || s.prob == nil {
		return 0
	}
	return s.prob.numConstraints
}

// SetStructuralBounds is a no-op stub when built without -tags cbc or with CGO_ENABLED=0.
func (s *CBCLPSolver) SetStructuralBounds(_, _ []float64) {}

// AddCut is a no-op stub when built without -tags cbc or with CGO_ENABLED=0.
func (s *CBCLPSolver) AddCut(_ []int, _ []float64, _ float64) {}

// Solve returns StatusUndefined when built without -tags cbc or with CGO_ENABLED=0.
func (s *CBCLPSolver) Solve() LPResult {
	return LPResult{Status: StatusUndefined}
}
