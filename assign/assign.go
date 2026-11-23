// Package assign provides a Jonker-Volgenant and Bijsterbosch-Volgenant
// shortest augmenting path solver for the rectangular Linear Sum Assignment Problem (LSAP).
package assign

import (
	"context"
	"errors"
	"math"
)

// Sense specifies whether the solver minimizes or maximizes total assignment cost.
type Sense int8

const (
	// Minimize selects minimum-cost linear sum matching.
	Minimize Sense = 1
	// Maximize selects maximum-weight linear sum matching.
	Maximize Sense = -1
)

func (s Sense) valid() bool {
	return s == Minimize || s == Maximize
}

// String returns "Minimize", "Maximize", or "Invalid".
func (s Sense) String() string {
	switch s {
	case Minimize:
		return "Minimize"
	case Maximize:
		return "Maximize"
	default:
		return "Invalid"
	}
}

var (
	// ErrInvalidDimensions indicates negative matrix dimensions, integer overflow in rows*cols,
	// ragged 2D rows, or a flat slice shorter than rows*cols.
	ErrInvalidDimensions = errors.New("assign: invalid matrix dimensions")
	// ErrInvalidSense indicates an optimization sense other than Minimize or Maximize.
	ErrInvalidSense = errors.New("assign: invalid optimization sense")
	// ErrInvalidCost indicates that the cost matrix contains NaN, -Inf when minimizing,
	// or +Inf when maximizing.
	ErrInvalidCost = errors.New("assign: matrix contains invalid numeric entries")
	// ErrInfeasible indicates that forbidden edges (+Inf when minimizing, -Inf when maximizing)
	// prevent any complete finite-cost matching of cardinality min(rows, cols).
	ErrInfeasible = errors.New("assign: cost matrix is infeasible")
	// ErrCertificationFailed indicates that runtime exact rational verification rejected the solver result.
	ErrCertificationFailed = errors.New("assign: runtime exact rational certification failed")
)

func validDims(rows, cols, flatLen int) bool {
	if rows < 0 || cols < 0 {
		return false
	}
	if rows == 0 || cols == 0 {
		return true
	}
	if rows > (math.MaxInt/2)/cols {
		return false
	}
	return flatLen >= rows*cols
}

func emptyResult(rows, cols int) Result {
	return Result{
		RowInd: []int{},
		ColInd: []int{},
		U:      make([]float64, rows),
		V:      make([]float64, cols),
	}
}

type assignCertHooks struct {
	cert2D   func(ctx context.Context, rows, cols int, cost [][]float64, sense Sense, res Result, solveErr error) (Result, error)
	certFlat func(ctx context.Context, rows, cols int, origCost []float64, sense Sense, res Result, solveErr error) (Result, error)
	certInto func(ctx context.Context, rows, cols int, origCost []float64, sense Sense, rowInd, colInd []int, ws *Workspace, obj float64, solveErr error) (float64, error)
}

// Options configures the Linear Sum Assignment solver.
type Options struct {
	cert *assignCertHooks
	// DisableSIMD forces the scalar reduced-cost kernels even when built with `GOEXPERIMENT=simd`.
	DisableSIMD bool
	// Certified enables fail-closed exact rational verification on the original cost matrix.
	Certified bool
}

// Option mutates solver settings before a LinearSum call.
type Option func(*Options)

// ApplyOptions folds functional options onto the default settings.
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

// WithSIMD enables or disables hardware SIMD kernels at runtime when built with `GOEXPERIMENT=simd`.
func WithSIMD(enabled bool) Option {
	return func(cfg *Options) { cfg.DisableSIMD = !enabled }
}

// WithoutSIMD disables hardware SIMD kernels, forcing the pure-Go scalar path.
func WithoutSIMD() Option {
	return func(cfg *Options) { cfg.DisableSIMD = true }
}

// Result holds the optimal primal matching, objective value, and dual potentials
// for a solved Linear Sum Assignment instance.
type Result struct {
	// RowInd contains the matched row indices in ascending order (length k = min(rows, cols)).
	RowInd []int
	// ColInd contains the column index matched to each RowInd[i] (length k = min(rows, cols)).
	ColInd []int
	// Objective is the total cost sum_i cost[RowInd[i]][ColInd[i]] evaluated on the input matrix.
	// If the exact sum of finite matched costs overflows float64 (for example large ~1e308 entries),
	// Objective is returned as +Inf or -Inf.
	Objective float64
	// U holds the row dual potentials for the normalized minimum-cost problem (length rows).
	U []float64
	// V holds the column dual potentials for the normalized minimum-cost problem (length cols).
	V []float64
}

func finishEmptyResult(ctx context.Context, rows, cols int, cost2D [][]float64, sense Sense, opts []Option, use2D bool) (Result, error) {
	res := emptyResult(rows, cols)
	if len(opts) > 0 {
		if cfg := ApplyOptions(opts...); cfg.Certified {
			if cfg.cert == nil {
				return Result{}, ErrCertificationFailed
			}
			if use2D {
				return cfg.cert.cert2D(ctx, rows, cols, cost2D, sense, res, nil)
			}
			return cfg.cert.certFlat(ctx, rows, cols, nil, sense, res, nil)
		}
	}
	return res, nil
}

// LinearSum solves the rectangular Linear Sum Assignment Problem on a 2D cost matrix under ctx.
// It returns RowInd (sorted ascending), ColInd, the optimal Objective, and the dual potentials U, V.
func LinearSum(ctx context.Context, cost [][]float64, sense Sense, opts ...Option) (Result, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
	}
	if !sense.valid() {
		return Result{}, ErrInvalidSense
	}
	rows := len(cost)
	if rows == 0 {
		return finishEmptyResult(ctx, 0, 0, nil, sense, opts, true)
	}
	cols := len(cost[0])
	for i := 1; i < rows; i++ {
		if len(cost[i]) != cols {
			return Result{}, ErrInvalidDimensions
		}
	}
	if cols == 0 {
		return finishEmptyResult(ctx, rows, 0, cost, sense, opts, true)
	}
	if rows > (math.MaxInt/2)/cols {
		return Result{}, ErrInvalidDimensions
	}

	ws := AcquireWorkspace(rows, cols)
	defer ReleaseWorkspace(ws)

	flat := ws.ensureWorkCost(rows * cols)
	for i, row := range cost {
		copy(flat[i*cols:(i+1)*cols], row)
	}
	cfg := ws.applyOptions(opts)
	res, err := solveWithWorkspace(ctx, rows, cols, flat, true, sense, ws, cfg)
	if cfg.Certified {
		if cfg.cert == nil {
			return Result{}, ErrCertificationFailed
		}
		return cfg.cert.cert2D(ctx, rows, cols, cost, sense, res, err)
	}
	return res, err
}

// LinearSumFlat solves the rectangular Linear Sum Assignment Problem on a row-major flat slice
// of length at least rows*cols under ctx without mutating cost.
func LinearSumFlat(ctx context.Context, rows, cols int, cost []float64, sense Sense, opts ...Option) (Result, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
	}
	if !sense.valid() {
		return Result{}, ErrInvalidSense
	}
	if !validDims(rows, cols, len(cost)) {
		return Result{}, ErrInvalidDimensions
	}
	if rows == 0 || cols == 0 {
		return finishEmptyResult(ctx, rows, cols, nil, sense, opts, false)
	}

	ws := AcquireWorkspace(rows, cols)
	defer ReleaseWorkspace(ws)

	cfg := ws.applyOptions(opts)
	origCost := cost[:rows*cols]
	res, err := solveWithWorkspace(ctx, rows, cols, origCost, false, sense, ws, cfg)
	if cfg.Certified {
		if cfg.cert == nil {
			return Result{}, ErrCertificationFailed
		}
		return cfg.cert.certFlat(ctx, rows, cols, origCost, sense, res, err)
	}
	return res, err
}

// SolveInto solves the rectangular Linear Sum Assignment Problem under ctx into caller-supplied
// rowInd and colInd slices (each of length at least min(rows, cols)) using ws, performing
// zero heap allocations when ws has sufficient capacity.
func SolveInto(ctx context.Context, rows, cols int, cost []float64, sense Sense, rowInd, colInd []int, ws *Workspace, opts ...Option) (float64, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
	}
	if !sense.valid() {
		return 0, ErrInvalidSense
	}
	if !validDims(rows, cols, len(cost)) {
		return 0, ErrInvalidDimensions
	}
	k := min(rows, cols)
	if len(rowInd) < k || len(colInd) < k {
		return 0, ErrInvalidDimensions
	}
	if k == 0 {
		if len(opts) > 0 {
			if cfg := ApplyOptions(opts...); cfg.Certified {
				if cfg.cert == nil {
					return 0, ErrCertificationFailed
				}
				return cfg.cert.certInto(ctx, rows, cols, nil, sense, nil, nil, nil, 0, nil)
			}
		}
		return 0, nil
	}
	if ws == nil {
		ws = AcquireWorkspace(rows, cols)
		defer ReleaseWorkspace(ws)
	} else {
		ws.ensure(k, max(rows, cols))
	}

	cfg := ws.applyOptions(opts)
	origCost := cost[:rows*cols]
	rOut := rowInd[:k]
	cOut := colInd[:k]
	obj, err := solveIntoPreallocated(ctx, rows, cols, origCost, false, sense, rOut, cOut, ws, cfg)
	if cfg.Certified {
		if cfg.cert == nil {
			for i := range rOut {
				rOut[i] = -1
				cOut[i] = -1
			}
			return 0, ErrCertificationFailed
		}
		return cfg.cert.certInto(ctx, rows, cols, origCost, sense, rOut, cOut, ws, obj, err)
	}
	return obj, err
}
