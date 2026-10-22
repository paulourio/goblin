package gopt

import "time"

// Options configures the gopt LP and MILP solver.
type Options struct {
	EnablePresolve   bool
	EnableCuts       bool
	EnableHeuristics bool
	MaxNodes         int
	MaxCutPasses     int
	RelGapTol        float64
	AbsGapTol        float64
	TimeLimit        time.Duration
}

// DefaultOptions returns the recommended production solver options.
func DefaultOptions() Options {
	return Options{
		EnablePresolve:   true,
		EnableCuts:       true,
		EnableHeuristics: true,
		MaxNodes:         50000,
		MaxCutPasses:     12,
		RelGapTol:        1e-6,
		AbsGapTol:        1e-6,
	}
}

// Result contains the solution status, objective value, variable assignments,
// and solver telemetry returned by Solve / SolveCompiled.
type Result struct {
	modelID      uint32
	Status       Status
	Objective    float64
	BestBound    float64
	RootLPBound  float64
	RootCutBound float64
	Values       []float64
	Duals        []float64
	ReducedCosts []float64
	Nodes        int
	Iterations   int
	CutsAdded    int
	SolveTime    time.Duration
}

// Value returns the solution value of variable v.
func (r *Result) Value(v Var) float64 {
	idx := v.Index()
	if (r.modelID == 0 || v.modelID() == 0 || v.modelID() == r.modelID) && idx >= 0 && idx < len(r.Values) {
		return r.Values[idx]
	}
	return 0
}

// Solve compiles and solves the Model using DefaultOptions (or the first provided Options).
func (m *Model) Solve(opts ...Options) Result {
	p := m.Compile()
	return SolveCompiled(p, opts...)
}

// SolveCompiled solves a pre-compiled CompiledProblem using Branch-and-Cut.
func SolveCompiled(p *CompiledProblem, opts ...Options) Result {
	start := time.Now()
	if p == nil || p.InvalidErr != nil {
		return Result{
			Status:    StatusUndefined,
			SolveTime: time.Since(start),
		}
	}
	opt := DefaultOptions()
	if len(opts) > 0 {
		opt = opts[0]
	}
	var deadline time.Time
	if opt.TimeLimit > 0 {
		deadline = start.Add(opt.TimeLimit)
	}

	workProb := p
	if opt.EnablePresolve && p.IsMIP {
		preRes := presolveWithDeadline(p, 6, deadline)
		if preRes.TimedOut {
			return Result{
				modelID:   p.modelID,
				Status:    StatusTimeLimit,
				SolveTime: time.Since(start),
			}
		}
		if preRes.Infeasible {
			return Result{
				modelID:   p.modelID,
				Status:    StatusInfeasible,
				SolveTime: time.Since(start),
			}
		}
		workProb = preRes.Reduced
	}

	return solveBranchAndCut(workProb, opt, start)
}
