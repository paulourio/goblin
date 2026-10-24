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
	var u, v []float64
	if rows > 0 {
		u = make([]float64, rows)
	}
	if cols > 0 {
		v = make([]float64, cols)
	}
	return Result{U: u, V: v}
}

// Options configures the Linear Sum Assignment solver.
type Options struct {
	// DisableSIMD forces the scalar reduced-cost kernels even when built with `GOEXPERIMENT=simd`.
	DisableSIMD bool
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
	Objective float64
	// U holds the row dual potentials for the normalized minimum-cost problem (length rows).
	U []float64
	// V holds the column dual potentials for the normalized minimum-cost problem (length cols).
	V []float64
}

// LinearSum solves the rectangular Linear Sum Assignment Problem on a 2D cost matrix under ctx.
// It returns RowInd (sorted ascending), ColInd, the optimal Objective, and the dual potentials U, V.
func LinearSum(ctx context.Context, cost [][]float64, sense Sense, opts ...Option) (Result, error) {
	if !sense.valid() {
		return Result{}, ErrInvalidSense
	}
	rows := len(cost)
	if rows == 0 {
		return Result{}, nil
	}
	cols := len(cost[0])
	for i := 1; i < rows; i++ {
		if len(cost[i]) != cols {
			return Result{}, ErrInvalidDimensions
		}
	}
	if cols == 0 {
		return emptyResult(rows, 0), nil
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
	return solveWithWorkspace(ctx, rows, cols, flat, true, sense, ws, ws.applyOptions(opts))
}

// LinearSumFlat solves the rectangular Linear Sum Assignment Problem on a row-major flat slice
// of length at least rows*cols under ctx without mutating cost.
func LinearSumFlat(ctx context.Context, rows, cols int, cost []float64, sense Sense, opts ...Option) (Result, error) {
	if !sense.valid() {
		return Result{}, ErrInvalidSense
	}
	if !validDims(rows, cols, len(cost)) {
		return Result{}, ErrInvalidDimensions
	}
	if rows == 0 || cols == 0 {
		return emptyResult(rows, cols), nil
	}

	ws := AcquireWorkspace(rows, cols)
	defer ReleaseWorkspace(ws)

	return solveWithWorkspace(ctx, rows, cols, cost[:rows*cols], false, sense, ws, ws.applyOptions(opts))
}

// SolveInto solves the rectangular Linear Sum Assignment Problem under ctx into caller-supplied
// rowInd and colInd slices (each of length at least min(rows, cols)) using ws, performing
// zero heap allocations when ws has sufficient capacity.
func SolveInto(ctx context.Context, rows, cols int, cost []float64, sense Sense, rowInd, colInd []int, ws *Workspace, opts ...Option) (float64, error) {
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
		return 0, nil
	}
	if ws == nil {
		ws = AcquireWorkspace(rows, cols)
		defer ReleaseWorkspace(ws)
	} else {
		ws.ensure(k, max(rows, cols))
	}

	cfg := ws.applyOptions(opts)
	return solveIntoPreallocated(ctx, rows, cols, cost[:rows*cols], false, sense, rowInd[:k], colInd[:k], ws, cfg)
}
