// Package mad implements an exact combinatorial solver for group-item L1/MAD
// grouping problems with optional rounding-step penalties and fail-closed
// fixed-width dyadic proof checks.
package mad

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Strategy selects the search engine for Solve.
type Strategy uint8

const (
	// StrategyAuto dispatches to DP or DFS within combinatorial caps, falling back to MILP when configured.
	StrategyAuto Strategy = iota
	// StrategyBruteForce uses exact incremental double-double depth-first search.
	StrategyBruteForce
	// StrategyDP uses exact subset bitmask dynamic programming.
	StrategyDP
	// StrategyMILP delegates directly to the configured MILP fallback hook.
	StrategyMILP
)

// String returns the canonical name of the strategy.
func (s Strategy) String() string {
	switch s {
	case StrategyAuto:
		return "Auto"
	case StrategyBruteForce:
		return "BruteForce"
	case StrategyDP:
		return "DP"
	case StrategyMILP:
		return "MILP"
	default:
		return fmt.Sprintf("Strategy(%d)", int(s))
	}
}

func (s Strategy) valid() bool {
	return s <= StrategyMILP
}

// Status reports the terminal outcome of a Solve call.
type Status uint8

const (
	// StatusOptimal indicates that Solve proved a globally optimal discrete answer.
	StatusOptimal Status = iota + 1
	// StatusTimeLimit indicates that WithTimeLimit expired before Solve proved the global optimum.
	StatusTimeLimit
	// StatusNodeLimit indicates that the MILP fallback reached MaxNodes.
	StatusNodeLimit
	// StatusIterationLimit indicates that the MILP fallback reached MaxIter.
	StatusIterationLimit
)

// String returns the canonical label for the terminal status.
func (s Status) String() string {
	switch s {
	case StatusOptimal:
		return "Optimal"
	case StatusTimeLimit:
		return "TimeLimit"
	case StatusNodeLimit:
		return "NodeLimit"
	case StatusIterationLimit:
		return "IterationLimit"
	default:
		return fmt.Sprintf("Status(%d)", int(s))
	}
}

var (
	// ErrInvalidProblem indicates bad dimensions, non-finite inputs, negative parameters,
	// ragged rows, bad Strategy values, bad DesignatedItems, or scale overflow bounds.
	ErrInvalidProblem = errors.New("mad: invalid problem")
	// ErrCombinatorialLimit indicates that combinatorial search exceeded work or memory ceilings
	// without an installed MILP fallback hook.
	ErrCombinatorialLimit = errors.New("mad: combinatorial search limit exceeded")
	// ErrCertificationFailed indicates that runtime exact dyadic checks failed or
	// Options.Certified was set without WithCertified or certified fallback hooks.
	ErrCertificationFailed = errors.New("mad: exact rational certification failed")
)

// Problem specifies a group-item L1/MAD grouping instance with optional baselines,
// step rounding, and bijective designated items.
type Problem struct {
	Groups          int
	Baselines       [][]float64
	Items           [][]float64
	Step            float64
	RoundWeight     float64
	AllowZeroStep   bool
	DesignatedItems []int
}

// Result holds the discrete item map, per-group levels and steps,
// objective value, global lower bound, and solver metadata.
type Result struct {
	Assignment []int
	Levels     []float64
	Steps      []int64
	Objective  float64
	BestBound  float64
	SolveTime  time.Duration
	Strategy   Strategy
	Status     Status
}

// FallbackFunc delegates problems beyond combinatorial ceilings to an external MILP solver.
type FallbackFunc func(ctx context.Context, prob Problem, opt Options) (Result, error)

type validatedProblem struct {
	numGroups   int
	numItems    int
	numPeriods  int
	uMaxF       float64
	valTimedOut bool
}

type madCertHooks struct {
	certify func(
		ctx context.Context,
		prob Problem,
		opt Options,
		info validatedProblem,
		start, deadline time.Time,
		hasDeadline, expired bool,
		done <-chan struct{},
	) (Result, error)
}

// Options sets search strategy, limits, MILP fallback hooks, and proof mode.
type Options struct {
	Strategy    Strategy
	MILPBackend uint8
	MaxNodes    int
	MaxIter     int
	RelGapTol   float64
	AbsGapTol   float64
	TimeLimit   time.Duration
	Certified   bool

	relGapTolSet bool
	absGapTolSet bool
	maxNodesSet  bool
	maxIterSet   bool

	fallback     FallbackFunc
	certFallback FallbackFunc
	cert         *madCertHooks
}

// RelGapTolSet reports whether RelGapTol was explicitly set.
func (o Options) RelGapTolSet() bool { return o.relGapTolSet }

// AbsGapTolSet reports whether AbsGapTol was explicitly set.
func (o Options) AbsGapTolSet() bool { return o.absGapTolSet }

// MaxNodesSet reports whether MaxNodes was explicitly set.
func (o Options) MaxNodesSet() bool { return o.maxNodesSet }

// MaxIterSet reports whether MaxIter was explicitly set.
func (o Options) MaxIterSet() bool { return o.maxIterSet }

// HasFallback reports whether an uncertified MILP fallback hook is installed.
func (o Options) HasFallback() bool { return o.fallback != nil }

// HasCertFallback reports whether a certified MILP fallback hook is installed.
func (o Options) HasCertFallback() bool { return o.certFallback != nil }

// Option mutates solver settings before Solve runs.
type Option func(*Options)

// DefaultOptions returns a zero-initialized Options struct with StrategyAuto
// and unset solver gap bounds.
func DefaultOptions() Options {
	return Options{}
}

// ApplyOptions folds functional options onto DefaultOptions.
func ApplyOptions(opts ...Option) Options {
	if len(opts) == 0 {
		return Options{}
	}
	return applyOptionsSlow(opts)
}

func applyOptionsSlow(opts []Option) Options {
	var cfg Options
	for _, fn := range opts {
		if fn != nil {
			fn(&cfg)
		}
	}
	return cfg
}

// WithOptions replaces options with o while preserving pre-installed fallback hooks
// when o leaves them nil and marking non-zero limits or gap bounds as explicitly set.
// Pre-installed proof hooks are not preserved when o.cert is nil so raw
// Certified literals fail closed without WithCertified.
func WithOptions(o Options) Option {
	return func(cfg *Options) {
		prevFallback := cfg.fallback
		prevCertFallback := cfg.certFallback
		*cfg = o
		if cfg.fallback == nil {
			cfg.fallback = prevFallback
		}
		if cfg.certFallback == nil {
			cfg.certFallback = prevCertFallback
		}
		if o.RelGapTol != 0 {
			cfg.relGapTolSet = true
		}
		if o.AbsGapTol != 0 {
			cfg.absGapTolSet = true
		}
		if o.MaxNodes != 0 {
			cfg.maxNodesSet = true
		}
		if o.MaxIter != 0 {
			cfg.maxIterSet = true
		}
	}
}

// WithStrategy sets the search strategy.
func WithStrategy(s Strategy) Option {
	return func(cfg *Options) { cfg.Strategy = s }
}

// WithMILPBackend stores the MILP backend identifier forwarded to fallback hooks.
func WithMILPBackend(b uint8) Option {
	return func(cfg *Options) { cfg.MILPBackend = b }
}

// WithTimeLimit sets the wall-clock search budget for Solve.
func WithTimeLimit(d time.Duration) Option {
	return func(cfg *Options) { cfg.TimeLimit = d }
}

// WithMaxNodes sets the MILP fallback node cap.
func WithMaxNodes(n int) Option {
	return func(cfg *Options) {
		cfg.MaxNodes = n
		cfg.maxNodesSet = true
	}
}

// WithMaxIter sets the MILP fallback step cap.
func WithMaxIter(n int) Option {
	return func(cfg *Options) {
		cfg.MaxIter = n
		cfg.maxIterSet = true
	}
}

// WithRelGapTol sets the relative gap bound forwarded to MILP fallback hooks.
func WithRelGapTol(tol float64) Option {
	return func(cfg *Options) {
		cfg.RelGapTol = tol
		cfg.relGapTolSet = true
	}
}

// WithAbsGapTol sets the absolute gap bound forwarded to MILP fallback hooks.
func WithAbsGapTol(tol float64) Option {
	return func(cfg *Options) {
		cfg.AbsGapTol = tol
		cfg.absGapTolSet = true
	}
}

// WithFallback installs the MILP fallback hook for uncertified runs.
func WithFallback(fn FallbackFunc) Option {
	return func(cfg *Options) { cfg.fallback = fn }
}

// WithCertFallback installs the MILP fallback hook for certified runs.
func WithCertFallback(fn FallbackFunc) Option {
	return func(cfg *Options) { cfg.certFallback = fn }
}

// TransposePeriodWeights converts a period-by-item matrix (T x I) into the
// item-by-period matrix (I x T) expected by Problem.Items.
func TransposePeriodWeights(w [][]float64) ([][]float64, error) {
	if len(w) == 0 {
		return nil, fmt.Errorf("%w: empty period weight matrix", ErrInvalidProblem)
	}
	numPeriods := len(w)
	numItems := len(w[0])
	for t := 1; t < numPeriods; t++ {
		if len(w[t]) != numItems {
			return nil, fmt.Errorf("%w: ragged period weight row %d has length %d, want %d", ErrInvalidProblem, t, len(w[t]), numItems)
		}
	}
	if numItems == 0 {
		return [][]float64{}, nil
	}
	items := make([][]float64, numItems)
	for i := range numItems {
		items[i] = make([]float64, numPeriods)
	}
	for t := range numPeriods {
		row := w[t]
		for i := range numItems {
			items[i][t] = row[i]
		}
	}
	return items, nil
}

// ValidateProblem verifies dimensions, finiteness, designated items, and scale bounds on prob.
func ValidateProblem(prob Problem) error {
	_, _, _, _, err := validateProblem(prob)
	return err
}

// Solve finds a globally optimal item assignment, levels, and rounding steps for prob.
func Solve(ctx context.Context, prob Problem, opts ...Option) (Result, error) {
	return solve(ctx, prob, opts...)
}
