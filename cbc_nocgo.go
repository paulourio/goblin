//go:build !cgo

package goblin

import "time"

// CBCAvailable reports whether the in-process C++ COIN-OR CBC/CLP backend is compiled in.
// Under CGO_ENABLED=0 this stub always returns false, and callers requesting BackendCBC
// receive StatusUndefined without invoking C++ symbols.
func CBCAvailable() bool { return false }

func solveCBC(p *CompiledProblem, _ Options, start time.Time) Result {
	return Result{
		modelID:   p.modelID,
		Backend:   BackendCBC,
		Status:    StatusUndefined,
		SolveTime: time.Since(start),
	}
}

// CBCLPSolver is a no-op stub when built with CGO_ENABLED=0.
type CBCLPSolver struct {
	Prob *CompiledProblem
}

// NewCBCLPSolver returns a stub solver.
func NewCBCLPSolver(p *CompiledProblem) *CBCLPSolver {
	return &CBCLPSolver{Prob: p}
}

// Close is a no-op stub.
func (s *CBCLPSolver) Close() {}

// TotalRows returns the number of structural rows.
func (s *CBCLPSolver) TotalRows() int { return s.Prob.NumConstraints }

// SetStructuralBounds is a no-op stub when built with CGO_ENABLED=0.
func (s *CBCLPSolver) SetStructuralBounds(_, _ []float64) {}

// AddCut is a no-op stub.
func (s *CBCLPSolver) AddCut(_ []int, _ []float64, _ float64) {}

// Solve returns StatusUndefined when built without cgo.
func (s *CBCLPSolver) Solve() LPResult {
	return LPResult{Status: StatusUndefined}
}
