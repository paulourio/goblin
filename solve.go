package goblin

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"
)

// ErrNilProblem indicates that a nil *CompiledProblem was passed to SolveCompiled.
var ErrNilProblem = errors.New("goblin: nil CompiledProblem")

// Backend selects the underlying LP and MILP solver engine.
type Backend uint8

const (
	// BackendGo uses goblin's pure-Go Revised Dual/Primal Simplex and Branch-and-Cut solver (default).
	BackendGo Backend = iota
	// BackendCBC uses optional in-process C++ bindings (-tags cbc with cgo enabled) to COIN-OR CBC and CLP.
	BackendCBC
)

// String returns the short backend name ("Go" or "CBC").
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
	// MaxNodes limits the number of explored branch-and-bound nodes, including any root
	// sub-MIP nodes (default 50000; 0 = unlimited). Exceeding it returns StatusNodeLimit
	// with a valid global bound.
	MaxNodes int
	// MaxCutPasses limits the number of root cut rounds (0 disables cutting).
	MaxCutPasses int
	// MaxIter limits the total main-tree and root sub-MIP simplex pivots reported in
	// Result.Iterations (0 = unlimited). Auxiliary primal-heuristic LP probes use bounded
	// local budgets. Exceeding MaxIter returns StatusIterationLimit; once the root LP is
	// solved, BestBound remains a valid global bound.
	MaxIter int
	// RelGapTol is the relative MIP gap threshold measured on the linear objective
	// (|upper - lower| / max(1, |upper|), excluding ObjConstant, matching BackendCBC).
	RelGapTol float64
	// AbsGapTol is the absolute MIP gap at which the search stops.
	AbsGapTol float64
	// TimeLimit bounds the wall-clock solve time (0 = unlimited).
	TimeLimit time.Duration
	// Workers controls parallel Branch-and-Bound node evaluation (<= 1 runs single-threaded).
	Workers int

	cutoffMin float64

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

	hasCutoff            bool
	isSubMIP             bool
	defaultMaxNodesUsed  bool
	defaultCutPassesUsed bool
}

const (
	defaultMaxNodes        = 50000
	defaultMaxCutPasses    = 12
	defaultRelGapTol       = 1e-6
	defaultAbsGapTol       = 1e-6
	defaultPresolveMaxPass = 6
)

// DefaultOptions returns the default solver settings (pure-Go backend, presolve, cuts,
// and heuristics enabled, with MaxNodes = 50000; pass WithMaxNodes(0) for an unlimited tree).
func DefaultOptions() Options {
	return Options{
		Backend:              BackendGo,
		EnablePresolve:       true,
		EnableCuts:           true,
		EnableHeuristics:     true,
		MaxNodes:             defaultMaxNodes,
		MaxCutPasses:         defaultMaxCutPasses,
		RelGapTol:            defaultRelGapTol,
		AbsGapTol:            defaultAbsGapTol,
		defaultMaxNodesUsed:  true,
		defaultCutPassesUsed: true,
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
	if cfg.RelGapTol < 0 || math.IsNaN(cfg.RelGapTol) {
		cfg.RelGapTol = 0
	}
	if cfg.AbsGapTol < 0 || math.IsNaN(cfg.AbsGapTol) {
		cfg.AbsGapTol = 0
	}
	return cfg
}

// WithOptions overwrites the solver settings with the supplied struct.
func WithOptions(o Options) Option {
	return func(cfg *Options) {
		*cfg = o
		cfg.defaultMaxNodesUsed = false
		cfg.defaultCutPassesUsed = false
	}
}

// WithBackend selects the solver backend (BackendGo or BackendCBC).
func WithBackend(b Backend) Option {
	return func(cfg *Options) { cfg.Backend = b }
}

// WithCBC selects the optional in-process C++ COIN-OR CBC/CLP backend (shorthand for WithBackend(BackendCBC)).
// Requires building with -tags cbc and cgo enabled; otherwise Solve returns StatusUndefined.
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
	return func(cfg *Options) {
		cfg.MaxNodes = n
		cfg.defaultMaxNodesUsed = false
	}
}

// WithMaxCutPasses sets the maximum number of root cutting-plane rounds (0 disables cutting).
func WithMaxCutPasses(n int) Option {
	return func(cfg *Options) {
		cfg.MaxCutPasses = n
		cfg.defaultCutPassesUsed = false
	}
}

// WithWorkers sets the number of parallel Branch-and-Bound worker goroutines (<= 1 runs single-threaded).
func WithWorkers(n int) Option {
	return func(cfg *Options) { cfg.Workers = n }
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
	Values       []float64
	Duals        []float64
	ReducedCosts []float64
	modelID      uint64
	Objective    float64
	BestBound    float64
	RootLPBound  float64
	RootCutBound float64
	Nodes        int
	Iterations   int
	CutsAdded    int
	SolveTime    time.Duration
	Backend      Backend
	Status       Status
}

// Lookup returns the primal value of variable v in the incumbent vector and reports
// whether v belongs to the solved model and an incumbent vector is present.
func (r *Result) Lookup(v Var) (float64, bool) {
	if r == nil || v == 0 {
		return 0, false
	}
	idx := v.Index()
	if idx < 0 || idx >= len(r.Values) {
		return 0, false
	}
	if r.modelID != 0 && v.modelTag() != modelVarTag(r.modelID) {
		return 0, false
	}
	return r.Values[idx], true
}

// Value returns the primal value of variable v in the incumbent vector, or 0 when absent.
func (r *Result) Value(v Var) float64 {
	val, _ := r.Lookup(v)
	return val
}

// Solve compiles and solves the Model under ctx, applying any supplied functional modifiers to the default settings.
// When ctx is canceled or its deadline expires, Solve aborts and returns (Result{}, ctx.Err()).
// When WithTimeLimit expires, Solve returns (res, nil) with res.Status == StatusTimeLimit and the best incumbent found.
func (m *Model) Solve(ctx context.Context, opts ...Option) (Result, error) {
	if m == nil {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return Result{}, err
			}
		}
		opt := ApplyOptions(opts...)
		return Result{
			Backend: opt.Backend,
			Status:  StatusUndefined,
		}, ErrNilModel
	}
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
	if opt.Backend != BackendGo && opt.Backend != BackendCBC {
		return Result{
			modelID:   p.modelID,
			Backend:   opt.Backend,
			Status:    StatusUndefined,
			SolveTime: time.Since(start),
		}, fmt.Errorf("goblin: invalid backend %d", opt.Backend)
	}
	if p.invalidErr != nil {
		return Result{
			modelID:   p.modelID,
			Backend:   opt.Backend,
			Status:    StatusUndefined,
			SolveTime: time.Since(start),
		}, p.invalidErr
	}
	var deadline time.Time
	if opt.TimeLimit < 0 {
		return Result{
			modelID:   p.modelID,
			Backend:   opt.Backend,
			Status:    StatusTimeLimit,
			SolveTime: time.Since(start),
		}, nil
	}
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

	res := solveCompensatedEngine(ctx, p, opt, BackendGo, start, deadline, done)
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
	}
	return res, nil
}

func solveCompensatedEngine(ctx context.Context, p *CompiledProblem, opt Options, backend Backend, start, deadline time.Time, done <-chan struct{}) Result {
	workProb := p
	var lpPre LPPresolveResult
	usedLPPresolve := false
	if opt.EnablePresolve && p.isMIP {
		preRes := presolveWithDeadline(p, defaultPresolveMaxPass, deadline, done)
		if ctx != nil && ctx.Err() != nil {
			return Result{}
		}
		if preRes.TimedOut {
			return Result{
				modelID:   p.modelID,
				Backend:   backend,
				Status:    StatusTimeLimit,
				SolveTime: time.Since(start),
			}
		}
		if preRes.Infeasible {
			return Result{
				modelID:   p.modelID,
				Backend:   backend,
				Status:    StatusInfeasible,
				SolveTime: time.Since(start),
			}
		}
		if preRes.Reduced != nil && preRes.Reduced.invalidErr == nil {
			workProb = preRes.Reduced
		}
	} else if opt.EnablePresolve && !p.isMIP && p.numConstraints > 0 && p.numVars > 0 {
		lpPre = presolveLPWithDeadline(p, defaultPresolveMaxPass, deadline, done)
		if ctx != nil && ctx.Err() != nil {
			return Result{}
		}
		if lpPre.TimedOut {
			return Result{
				modelID:   p.modelID,
				Backend:   backend,
				Status:    StatusTimeLimit,
				SolveTime: time.Since(start),
			}
		}
		if lpPre.Infeasible {
			return Result{
				modelID:   p.modelID,
				Backend:   backend,
				Status:    StatusInfeasible,
				SolveTime: time.Since(start),
			}
		}
		if lpPre.Reduced != nil && lpPre.Reduced != p && lpPre.Reduced.invalidErr == nil {
			workProb = lpPre.Reduced
			usedLPPresolve = true
		}
	}

	res := solveBranchAndCutWithDone(workProb, opt, start, deadline, done)
	if ctx != nil && ctx.Err() != nil {
		return Result{}
	}
	if workProb != p {
		needFallback := false
		if usedLPPresolve {
			if res.Status == StatusUnbounded || res.Status == StatusNumericalError {
				needFallback = true
			} else if res.Status == StatusOptimal && (len(res.Values) != p.numVars || !postsolveLP(p, &lpPre, &res)) {
				needFallback = true
			}
		} else if res.Status == StatusNumericalError {
			needFallback = true
		} else if len(res.Values) == p.numVars {
			bV, rV, iV := p.MaxViolation(res.Values, p.isMIP)
			if !(bV <= incumbentTol && rV <= incumbentTol && iV <= incumbentTol) {
				needFallback = true
			}
		}
		if needFallback {
			fallbackOpt := opt
			fallbackOpt.EnablePresolve = false
			if fallbackOpt.MaxIter > 0 {
				rem := fallbackOpt.MaxIter - res.Iterations
				if rem <= 0 {
					return Result{
						modelID:    p.modelID,
						Backend:    backend,
						Status:     StatusIterationLimit,
						Nodes:      res.Nodes,
						Iterations: res.Iterations,
						SolveTime:  time.Since(start),
					}
				}
				fallbackOpt.MaxIter = rem
			}
			if fallbackOpt.MaxNodes > 0 {
				remNodes := fallbackOpt.MaxNodes - res.Nodes
				if remNodes <= 0 {
					return Result{
						modelID:    p.modelID,
						Backend:    backend,
						Status:     StatusNodeLimit,
						Nodes:      res.Nodes,
						Iterations: res.Iterations,
						SolveTime:  time.Since(start),
					}
				}
				fallbackOpt.MaxNodes = remNodes
			}
			fbRes := solveBranchAndCutWithDone(p, fallbackOpt, start, deadline, done)
			if ctx != nil && ctx.Err() != nil {
				return Result{}
			}
			fbRes.Nodes += res.Nodes
			fbRes.Iterations += res.Iterations
			fbRes.Backend = backend
			return fbRes
		}
		if len(res.Values) == p.numVars {
			res.Objective = p.EvaluateObjective(res.Values)
			if res.Status == StatusOptimal && (math.IsNaN(res.Objective) || math.IsInf(res.Objective, 0) || math.IsNaN(res.BestBound) || math.IsInf(res.BestBound, 0)) {
				return Result{
					modelID:    p.modelID,
					Backend:    backend,
					Status:     StatusNumericalError,
					Nodes:      res.Nodes,
					Iterations: res.Iterations,
					CutsAdded:  res.CutsAdded,
					SolveTime:  time.Since(start),
				}
			}
		}
	}
	res.Backend = backend
	return res
}
