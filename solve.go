package gopt

import "time"

// Options configures the gopt LP and MILP solver.
//
// Start from DefaultOptions and override individual fields. The zero value is a valid but
// minimal configuration: presolve, cuts, and heuristics are disabled, the node and iteration
// budgets are unlimited, and both gap tolerances are zero (prove exact optimality).
type Options struct {
	// EnablePresolve runs MIP presolve and node-level bound propagation.
	EnablePresolve bool
	// EnableCuts runs root cutting-plane rounds (GMI, lifted covers, flow/VUB, MIR).
	EnableCuts bool
	// EnableHeuristics runs primal heuristics (rounding, diving, root sub-MIP).
	EnableHeuristics bool
	// MaxNodes limits the number of explored branch-and-bound nodes (0 = unlimited).
	// Exceeding it returns StatusNodeLimit with a valid global bound.
	MaxNodes int
	// MaxCutPasses limits the number of root cut rounds (0 disables cutting).
	MaxCutPasses int
	// MaxIter limits the total simplex iterations reported in Result.Iterations
	// (0 = unlimited). Exceeding it returns StatusIterationLimit; for MILPs whose root LP
	// was solved, BestBound remains a valid global bound.
	MaxIter int
	// RelGapTol is the relative optimality gap at which the search stops.
	RelGapTol float64
	// AbsGapTol is the absolute optimality gap at which the search stops.
	AbsGapTol float64
	// TimeLimit bounds the wall-clock solve time (0 = unlimited).
	TimeLimit time.Duration
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
