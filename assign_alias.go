package goblin

import (
	"context"

	"github.com/paulourio/goblin/assign"
)

// AssignResult aliases `assign.Result` and holds the optimal primal matching, total objective
// value, and row and column dual potentials returned by the root linear sum solvers.
// Callers importing the root package can inspect `RowInd`, `ColInd`, `Objective`, `U`, and `V`
// directly without importing the subpackage.
type AssignResult = assign.Result

// AssignOption configures the root linear sum entry points (`WithSIMD` and `WithoutSIMD`).
// Pass these functional modifiers to toggle hardware vector kernels at runtime.
type AssignOption = assign.Option

// LinearSumAssignment solves the rectangular Linear Sum Assignment Problem on a 2D cost matrix
// under ctx and returns the optimal matching and dual potentials via `assign.LinearSum`.
// Every row of `cost` must have equal length, and entries must be finite or valid forbidden `+Inf`/`-Inf` values.
func LinearSumAssignment(ctx context.Context, cost [][]float64, sense Sense, opts ...AssignOption) (AssignResult, error) {
	return assign.LinearSum(ctx, cost, assign.Sense(sense), opts...)
}

// LinearSumAssignmentFlat solves the rectangular Linear Sum Assignment Problem on a row-major
// flat slice of length at least `rows*cols` without mutating `cost`, delegating directly to
// `assign.LinearSumFlat`. Use this entry point when the cost matrix is already stored contiguously.
func LinearSumAssignmentFlat(ctx context.Context, rows, cols int, cost []float64, sense Sense, opts ...AssignOption) (AssignResult, error) {
	return assign.LinearSumFlat(ctx, rows, cols, cost, assign.Sense(sense), opts...)
}
