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
	return Result{
		modelID:   p.modelID,
		Backend:   BackendCBC,
		Status:    StatusUndefined,
		SolveTime: time.Since(start),
	}
}

// CBCLPSolver is a no-op stub when built without -tags cbc or with CGO_ENABLED=0.
type CBCLPSolver struct {
	Prob   *CompiledProblem
	numCut int
}

// NewCBCLPSolver returns a stub solver.
func NewCBCLPSolver(p *CompiledProblem) *CBCLPSolver {
	return &CBCLPSolver{Prob: p}
}

// Close is a no-op stub.
func (s *CBCLPSolver) Close() {}

// TotalRows returns the number of structural rows and added cuts.
func (s *CBCLPSolver) TotalRows() int { return s.Prob.NumConstraints + s.numCut }

// SetStructuralBounds is a no-op stub when built without -tags cbc or with CGO_ENABLED=0.
func (s *CBCLPSolver) SetStructuralBounds(_, _ []float64) {}

// AddCut records the cut count on the no-cgo stub.
func (s *CBCLPSolver) AddCut(_ []int, _ []float64, _ float64) { s.numCut++ }

// Solve returns StatusUndefined when built without cgo.
func (s *CBCLPSolver) Solve() LPResult {
	return LPResult{Status: StatusUndefined}
}
