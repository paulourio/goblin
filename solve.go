package goblin

import (
	"context"
	"errors"
	"time"
)

// ErrNilProblem indicates that a nil *CompiledProblem was passed to SolveCompiled.
var ErrNilProblem = errors.New("goblin: nil CompiledProblem")

// Backend selects the underlying LP and MILP solver engine.
type Backend uint8

const (
	// BackendGo uses goblin's pure-Go Revised Dual/Primal Simplex and Branch-and-Cut solver (default).
	BackendGo Backend = iota
	// BackendCBC uses in-process C++ bindings (cgo) to the COIN-OR CBC and CLP libraries.
	BackendCBC
)

func (b Backend) String() string {
	switch b {
	case BackendCBC:
		return "CBC"
	default:
		return "Go"
	}
}

// Options configures the goblin LP and MILP solver.
//
// Callers may configure Solve / SolveCompiled via functional constructors (such as
// WithBackend, WithCBC, WithSIMD, WithTimeLimit) or pass a bulk struct in a single call.
type Options struct {
	// Backend selects the solver engine (BackendGo by default, or BackendCBC).
	Backend Backend
	// DisableSIMD forces the pure-Go scalar kernels even when compiled with GOEXPERIMENT=simd.
	DisableSIMD bool
	// EnablePresolve runs MIP presolve and node-level bound tightening.
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
	// MaxIter limits the total simplex pivots reported in the result counter
	// (0 = unlimited). Exceeding it returns StatusIterationLimit; for MILPs whose root LP
	// was solved, BestBound remains a valid global bound.
	MaxIter int
	// RelGapTol is the relative MIP gap at which the search stops.
	RelGapTol float64
	// AbsGapTol is the absolute MIP gap at which the search stops.
	AbsGapTol float64
	// TimeLimit bounds the wall-clock solve time (0 = unlimited).
	TimeLimit time.Duration

}

const (
	defaultMaxNodes        = 50000
	defaultMaxCutPasses    = 12
	defaultRelGapTol       = 1e-6
	defaultAbsGapTol       = 1e-6
	defaultPresolveMaxPass = 6
)

// DefaultOptions returns the default solver settings (pure-Go backend, presolve, cuts, and heuristics enabled).
func DefaultOptions() Options {
	return Options{
		Backend:          BackendGo,
		EnablePresolve:   true,
		EnableCuts:       true,
		EnableHeuristics: true,
		MaxNodes:         defaultMaxNodes,
		MaxCutPasses:     defaultMaxCutPasses,
		RelGapTol:        defaultRelGapTol,
		AbsGapTol:        defaultAbsGapTol,
	}
}

// Option mutates a solver settings struct before Solve or SolveCompiled runs.
type Option func(*Options)

// ApplyOptions folds a slice of functional modifiers onto the default solver settings.
func ApplyOptions(opts ...Option) Options {
	cfg := DefaultOptions()
	for _, fn := range opts {
		if fn != nil {
			fn(&cfg)
		}
	}
	return cfg
}

// WithOptions overwrites the solver settings with the supplied struct.
func WithOptions(o Options) Option {
	return func(cfg *Options) { *cfg = o }
}

// WithBackend selects the solver backend (BackendGo or BackendCBC).
func WithBackend(b Backend) Option {
	return func(cfg *Options) { cfg.Backend = b }
}

// WithCBC selects the in-process C++ COIN-OR CBC/CLP backend (shorthand for WithBackend(BackendCBC)).
func WithCBC() Option {
	return func(cfg *Options) { cfg.Backend = BackendCBC }
}

// WithPureGo selects the pure-Go solver backend (shorthand for WithBackend(BackendGo)).
func WithPureGo() Option {
	return func(cfg *Options) { cfg.Backend = BackendGo }
}

// WithSIMD enables or disables hardware SIMD vector kernels at runtime when built with GOEXPERIMENT=simd.
func WithSIMD(enabled bool) Option {
	return func(cfg *Options) { cfg.DisableSIMD = !enabled }
}

// WithoutSIMD disables hardware SIMD vector kernels, forcing the pure-Go scalar path.
func WithoutSIMD() Option {
	return func(cfg *Options) { cfg.DisableSIMD = true }
}

// WithPresolve enables or disables MIP presolve and node bound tightening.
func WithPresolve(enabled bool) Option {
	return func(cfg *Options) { cfg.EnablePresolve = enabled }
}

// WithCuts enables or disables root cutting-plane rounds.
func WithCuts(enabled bool) Option {
	return func(cfg *Options) { cfg.EnableCuts = enabled }
}

// WithHeuristics enables or disables primal heuristics.
func WithHeuristics(enabled bool) Option {
	return func(cfg *Options) { cfg.EnableHeuristics = enabled }
}

// WithMaxNodes sets the maximum number of branch-and-bound nodes (0 = unlimited).
func WithMaxNodes(n int) Option {
	return func(cfg *Options) { cfg.MaxNodes = n }
}

// WithMaxCutPasses sets the maximum number of root cutting-plane rounds (0 disables cutting).
func WithMaxCutPasses(n int) Option {
	return func(cfg *Options) { cfg.MaxCutPasses = n }
}

// WithMaxIter sets the maximum total simplex pivots (0 = unlimited).
func WithMaxIter(n int) Option {
	return func(cfg *Options) { cfg.MaxIter = n }
}

// WithRelGapTol sets the relative MIP gap threshold.
func WithRelGapTol(tol float64) Option {
	return func(cfg *Options) { cfg.RelGapTol = tol }
}

// WithAbsGapTol sets the absolute MIP gap threshold.
func WithAbsGapTol(tol float64) Option {
	return func(cfg *Options) { cfg.AbsGapTol = tol }
}

// WithTimeLimit sets the wall-clock time limit for the solve.
func WithTimeLimit(d time.Duration) Option {
	return func(cfg *Options) { cfg.TimeLimit = d }
}

func stopRequested(deadline time.Time, done <-chan struct{}) bool {
	if done != nil {
		select {
		case <-done:
			return true
		default:
		}
	}
	return !deadline.IsZero() && time.Now().After(deadline)
}

// Result holds the terminal status, objective value, primal and dual vectors,
// and search counters returned by Solve and SolveCompiled.
type Result struct {
	modelID      uint32
	Backend      Backend
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

// Value returns the primal value of variable v in the incumbent vector.
func (r *Result) Value(v Var) float64 {
	idx := v.Index()
	if (r.modelID == 0 || v.modelID() == 0 || v.modelID() == r.modelID) && idx >= 0 && idx < len(r.Values) {
		return r.Values[idx]
	}
	return 0
}

// Solve compiles and solves the Model under ctx, applying any supplied functional modifiers to the default settings.
// When ctx is canceled or its deadline expires, Solve aborts and returns (Result{}, ctx.Err()).
// When WithTimeLimit expires, Solve returns (res, nil) with res.Status == StatusTimeLimit and the best incumbent found.
func (m *Model) Solve(ctx context.Context, opts ...Option) (Result, error) {
	p := m.Compile()
	return SolveCompiled(ctx, p, opts...)
}

// SolveCompiled solves a pre-compiled CompiledProblem under ctx using the selected backend
// (pure-Go Branch-and-Cut by default, or COIN-OR CBC/CLP when WithCBC() / WithBackend(BackendCBC) is passed).
// When ctx is canceled or its deadline expires, SolveCompiled aborts and returns (Result{}, ctx.Err()).
// When WithTimeLimit expires, SolveCompiled returns (res, nil) with res.Status == StatusTimeLimit and the best incumbent found.
func SolveCompiled(ctx context.Context, p *CompiledProblem, opts ...Option) (Result, error) {
	start := time.Now()
	opt := ApplyOptions(opts...)
	var done <-chan struct{}
	if ctx != nil {
		done = ctx.Done()
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
	}
	if p == nil {
		return Result{
			Backend:   opt.Backend,
			Status:    StatusUndefined,
			SolveTime: time.Since(start),
		}, ErrNilProblem
	}
	if p.InvalidErr != nil {
		return Result{
			modelID:   p.modelID,
			Backend:   opt.Backend,
			Status:    StatusUndefined,
			SolveTime: time.Since(start),
		}, p.InvalidErr
	}
	var deadline time.Time
	if opt.TimeLimit > 0 {
		deadline = start.Add(opt.TimeLimit)
		if !start.Before(deadline) {
			return Result{
				modelID:   p.modelID,
				Backend:   opt.Backend,
				Status:    StatusTimeLimit,
				SolveTime: time.Since(start),
			}, nil
		}
	}
	if opt.Backend == BackendCBC {
		res := solveCBC(ctx, p, opt, start, deadline, done)
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return Result{}, err
			}
		}
		return res, nil
	}

	workProb := p
	if opt.EnablePresolve && p.IsMIP {
		preRes := presolveWithDeadline(p, defaultPresolveMaxPass, deadline, done)
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return Result{}, err
			}
		}
		if preRes.TimedOut {
			return Result{
				modelID:   p.modelID,
				Backend:   BackendGo,
				Status:    StatusTimeLimit,
				SolveTime: time.Since(start),
			}, nil
		}
		if preRes.Infeasible {
			return Result{
				modelID:   p.modelID,
				Backend:   BackendGo,
				Status:    StatusInfeasible,
				SolveTime: time.Since(start),
			}, nil
		}
		workProb = preRes.Reduced
	}

	res := solveBranchAndCutWithDone(workProb, opt, start, deadline, done)
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
	}
	res.Backend = BackendGo
	return res, nil
}
