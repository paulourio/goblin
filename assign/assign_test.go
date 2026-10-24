package assign_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	gb "github.com/paulourio/goblin"
	"github.com/paulourio/goblin/assign"
)

const certRelTol = 1e-11

// verifyOptimalityCertificate checks primal matching validity, dual reduced-cost bounds,
// unassigned dual boundary conditions, and complementary slackness in O(rows * cols) time.
func verifyOptimalityCertificate(tb testing.TB, label string, rows, cols int, cost []float64, sense assign.Sense, res assign.Result) {
	tb.Helper()
	k := min(rows, cols)
	if len(res.RowInd) != k || len(res.ColInd) != k {
		tb.Fatalf("[%s] unexpected matching cardinality: got (%d, %d), want %d", label, len(res.RowInd), len(res.ColInd), k)
	}
	if len(res.U) != rows || len(res.V) != cols {
		tb.Fatalf("[%s] unexpected dual lengths: len(U)=%d (want %d), len(V)=%d (want %d)",
			label, len(res.U), rows, len(res.V), cols)
	}
	if k == 0 {
		if res.Objective != 0 {
			tb.Fatalf("[%s] expected 0 objective for empty matrix, got %v", label, res.Objective)
		}
		return
	}

	usedRow := make([]bool, rows)
	usedCol := make([]bool, cols)
	var sumPrimal, sumMinPrimal, maxAbsCost float64

	for _, c := range cost[:rows*cols] {
		if !math.IsInf(c, 0) && !math.IsNaN(c) {
			maxAbsCost = max(maxAbsCost, math.Abs(c))
		}
	}
	tol := certRelTol * max(1.0, maxAbsCost) * float64(max(rows, cols))

	prevRow := -1
	for idx := range k {
		r := res.RowInd[idx]
		c := res.ColInd[idx]
		if r < 0 || r >= rows || c < 0 || c >= cols {
			tb.Fatalf("[%s] out-of-range assignment pair (%d, %d) for %dx%d", label, r, c, rows, cols)
		}
		if r <= prevRow {
			tb.Fatalf("[%s] RowInd not strictly ascending at %d: %v", label, idx, res.RowInd)
		}
		prevRow = r
		if usedRow[r] {
			tb.Fatalf("[%s] duplicate row %d in RowInd", label, r)
		}
		if usedCol[c] {
			tb.Fatalf("[%s] duplicate col %d in ColInd", label, c)
		}
		usedRow[r] = true
		usedCol[c] = true

		val := cost[r*cols+c]
		if math.IsInf(val, 0) || math.IsNaN(val) {
			tb.Fatalf("[%s] assigned forbidden/invalid edge (%d, %d) with cost %v", label, r, c, val)
		}
		sumPrimal += val
		normVal := val
		if sense == assign.Maximize {
			normVal = -val
		}
		sumMinPrimal += normVal

		// Check complementary slackness on each matched edge: u[r] + v[c] == normCost[r, c].
		slack := math.Abs((res.U[r] + res.V[c]) - normVal)
		if slack > tol {
			tb.Fatalf("[%s] complementary slackness violated on matched edge (%d, %d): u=%v + v=%v != normCost=%v (diff=%e, tol=%e)",
				label, r, c, res.U[r], res.V[c], normVal, slack, tol)
		}
	}

	if math.Abs(sumPrimal-res.Objective) > tol {
		tb.Fatalf("[%s] objective mismatch: sumPrimal=%.12g vs res.Objective=%.12g", label, sumPrimal, res.Objective)
	}

	// Check dual reduced-cost bounds: u[r] + v[c] <= normCost[r, c] + tol for all (r, c).
	for r := range rows {
		ur := res.U[r]
		for c := range cols {
			normVal := cost[r*cols+c]
			if sense == assign.Maximize {
				normVal = -normVal
			}
			if ur+res.V[c] > normVal+tol {
				tb.Fatalf("[%s] dual feasibility violated at (%d, %d): u=%v + v=%v = %v > normCost=%v (excess=%e, tol=%e)",
					label, r, c, ur, res.V[c], ur+res.V[c], normVal, (ur+res.V[c])-normVal, tol)
			}
		}
	}

	// Check unassigned dual sign and boundary conditions on rectangular matrices.
	if rows < cols {
		for c := range cols {
			if res.V[c] > tol {
				tb.Fatalf("[%s] rectangular (rows < cols) positive column dual v[%d]=%v", label, c, res.V[c])
			}
			if !usedCol[c] && math.Abs(res.V[c]) > tol {
				tb.Fatalf("[%s] rectangular (rows < cols) nonzero unassigned column dual v[%d]=%v", label, c, res.V[c])
			}
		}
	} else if rows > cols {
		for r := range rows {
			if res.U[r] > tol {
				tb.Fatalf("[%s] rectangular (rows > cols) positive row dual u[%d]=%v", label, r, res.U[r])
			}
			if !usedRow[r] && math.Abs(res.U[r]) > tol {
				tb.Fatalf("[%s] rectangular (rows > cols) nonzero unassigned row dual u[%d]=%v", label, r, res.U[r])
			}
		}
	}

	var sumDual float64
	for _, u := range res.U {
		sumDual += u
	}
	for _, v := range res.V {
		sumDual += v
	}
	if math.Abs(sumMinPrimal-sumDual) > tol {
		tb.Fatalf("[%s] strong duality gap: primal=%.12g vs dual=%.12g (diff=%e, tol=%e)",
			label, sumMinPrimal, sumDual, math.Abs(sumMinPrimal-sumDual), tol)
	}
}

// exactAssignmentObjective computes the ground-truth optimal objective via exhaustive
// depth-first search for small matrices (min(rows, cols) <= 7).
func exactAssignmentObjective(rows, cols int, cost []float64, sense assign.Sense) (float64, bool) {
	transposed := rows > cols
	m := min(rows, cols)
	n := max(rows, cols)
	if m == 0 {
		return 0, true
	}

	used := make([]bool, n)
	best := math.Inf(1)
	if sense == assign.Maximize {
		best = math.Inf(-1)
	}
	found := false

	var dfs func(i int, acc float64)
	dfs = func(i int, acc float64) {
		if i == m {
			found = true
			if (sense == assign.Minimize && acc < best) || (sense == assign.Maximize && acc > best) {
				best = acc
			}
			return
		}
		for j := range n {
			if used[j] {
				continue
			}
			var c float64
			if !transposed {
				c = cost[i*cols+j]
			} else {
				c = cost[j*cols+i]
			}
			if math.IsInf(c, 0) || math.IsNaN(c) {
				continue
			}
			used[j] = true
			dfs(i+1, acc+c)
			used[j] = false
		}
	}
	dfs(0, 0)
	return best, found
}

// solveViaGoblinMILP solves the assignment instance using goblin's general MILP engine as a reference oracle.
func solveViaGoblinMILP(rows, cols int, cost []float64, sense assign.Sense) (float64, gb.Status) {
	mdl := gb.NewModel("lsap_ref", gb.Sense(sense))
	vars := make([]gb.Var, rows*cols)
	obj := gb.NewExpr()
	for r := range rows {
		for c := range cols {
			val := cost[r*cols+c]
			v := mdl.AddBinaryVar(fmt.Sprintf("x_%d_%d", r, c))
			vars[r*cols+c] = v
			if math.IsInf(val, 0) {
				mdl.AddConstraint(fmt.Sprintf("forbid_%d_%d", r, c), gb.NewExpr(v.T(1)).Eq(0))
			} else if val != 0 {
				obj = obj.AddTerm(v, val)
			}
		}
	}
	mdl.SetObjective(obj)

	for r := range rows {
		e := gb.NewExpr()
		for c := range cols {
			e = e.AddTerm(vars[r*cols+c], 1)
		}
		if rows <= cols {
			mdl.AddConstraint(fmt.Sprintf("row_%d", r), e.Eq(1))
		} else {
			mdl.AddConstraint(fmt.Sprintf("row_%d", r), e.Le(1))
		}
	}
	for c := range cols {
		e := gb.NewExpr()
		for r := range rows {
			e = e.AddTerm(vars[r*cols+c], 1)
		}
		if cols <= rows {
			mdl.AddConstraint(fmt.Sprintf("col_%d", c), e.Eq(1))
		} else {
			mdl.AddConstraint(fmt.Sprintf("col_%d", c), e.Le(1))
		}
	}

	res, err := mdl.Solve(context.Background())
	if err != nil {
		return 0, gb.StatusUndefined
	}
	return res.Objective, res.Status
}

func TestCanonicalAndSciPyReferenceCases(t *testing.T) {
	inf := math.Inf(1)
	ninf := math.Inf(-1)

	cases := []struct {
		name    string
		matrix  [][]float64
		sense   assign.Sense
		wantObj float64
	}{
		{
			name: "scipy_doc_3x3_min",
			matrix: [][]float64{
				{4, 1, 3},
				{2, 0, 5},
				{3, 2, 2},
			},
			sense:   assign.Minimize,
			wantObj: 5,
		},
		{
			name: "scipy_doc_3x3_max",
			matrix: [][]float64{
				{4, 1, 3},
				{2, 0, 5},
				{3, 2, 2},
			},
			sense:   assign.Maximize,
			wantObj: 11,
		},
		{
			name: "scipy_input_object_2x3_min",
			matrix: [][]float64{
				{1, 2, 3},
				{4, 5, 6},
			},
			sense:   assign.Minimize,
			wantObj: 6,
		},
		{
			name: "canon_4x4_hungarian",
			matrix: [][]float64{
				{90, 75, 75, 80},
				{35, 85, 55, 65},
				{125, 95, 90, 105},
				{45, 110, 95, 115},
			},
			sense:   assign.Minimize,
			wantObj: 275,
		},
		{
			name: "wide_3x5_min",
			matrix: [][]float64{
				{10, 2, 8, 15, 4},
				{3, 9, 12, 1, 7},
				{6, 5, 11, 14, 3},
			},
			sense:   assign.Minimize,
			wantObj: 6,
		},
		{
			name: "tall_5x3_max",
			matrix: [][]float64{
				{10, 2, 8},
				{3, 9, 12},
				{6, 5, 11},
				{15, 1, 14},
				{4, 7, 3},
			},
			sense:   assign.Maximize,
			wantObj: 35,
		},
		{
			name: "single_1x1",
			matrix: [][]float64{
				{-42.5},
			},
			sense:   assign.Minimize,
			wantObj: -42.5,
		},
		{
			name: "single_row_1x4",
			matrix: [][]float64{
				{9, -3, 4, 2},
			},
			sense:   assign.Minimize,
			wantObj: -3,
		},
		{
			name: "single_col_4x1_max",
			matrix: [][]float64{
				{9},
				{-3},
				{14},
				{2},
			},
			sense:   assign.Maximize,
			wantObj: 14,
		},
		{
			name: "all_zeros_4x4",
			matrix: [][]float64{
				{0, 0, 0, 0},
				{0, 0, 0, 0},
				{0, 0, 0, 0},
				{0, 0, 0, 0},
			},
			sense:   assign.Minimize,
			wantObj: 0,
		},
		{
			name: "forbidden_inf_min_feasible",
			matrix: [][]float64{
				{inf, 5, inf},
				{10, inf, 2},
				{3, 4, inf},
			},
			sense:   assign.Minimize,
			wantObj: 10,
		},
		{
			name: "forbidden_ninf_max_feasible",
			matrix: [][]float64{
				{ninf, 5, ninf},
				{10, ninf, 2},
				{3, 4, ninf},
			},
			sense:   assign.Maximize,
			wantObj: 10,
		},
	}

	// Verify every reference case against the exhaustive depth-first oracle.
	for i := range cases {
		m := cases[i].matrix
		rows := len(m)
		cols := len(m[0])
		flat := make([]float64, rows*cols)
		for r := range rows {
			copy(flat[r*cols:(r+1)*cols], m[r])
		}
		wantExact, ok := exactAssignmentObjective(rows, cols, flat, cases[i].sense)
		if !ok {
			t.Fatalf("%s: expected feasible in exact oracle", cases[i].name)
		}
		if math.Abs(cases[i].wantObj-wantExact) > 1e-9 {
			t.Fatalf("%s: table wantObj=%v != exact=%v", cases[i].name, cases[i].wantObj, wantExact)
		}
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows := len(tc.matrix)
			cols := len(tc.matrix[0])
			flat := make([]float64, rows*cols)
			for r := range rows {
				copy(flat[r*cols:(r+1)*cols], tc.matrix[r])
			}

			for _, optMode := range []struct {
				tag string
				opt assign.Option
			}{
				{"default", assign.WithSIMD(true)},
				{"no_simd", assign.WithoutSIMD()},
			} {
				res2D, err := assign.LinearSum(t.Context(), tc.matrix, tc.sense, optMode.opt)
				if err != nil {
					t.Fatalf("%s/%s LinearSum failed: %v", tc.name, optMode.tag, err)
				}
				if math.Abs(res2D.Objective-tc.wantObj) > 1e-9 {
					t.Fatalf("%s/%s LinearSum obj = %v, want %v", tc.name, optMode.tag, res2D.Objective, tc.wantObj)
				}
				verifyOptimalityCertificate(t, tc.name+"/"+optMode.tag+"/2D", rows, cols, flat, tc.sense, res2D)

				resFlat, err := assign.LinearSumFlat(t.Context(), rows, cols, flat, tc.sense, optMode.opt)
				if err != nil {
					t.Fatalf("%s/%s LinearSumFlat failed: %v", tc.name, optMode.tag, err)
				}
				if math.Abs(resFlat.Objective-tc.wantObj) > 1e-9 {
					t.Fatalf("%s/%s LinearSumFlat obj = %v, want %v", tc.name, optMode.tag, resFlat.Objective, tc.wantObj)
				}
				verifyOptimalityCertificate(t, tc.name+"/"+optMode.tag+"/Flat", rows, cols, flat, tc.sense, resFlat)

				// Verify the root package alias on the same cost matrix.
				resAlias, err := gb.LinearSumAssignment(t.Context(), tc.matrix, gb.Sense(tc.sense), optMode.opt)
				if err != nil || math.Abs(resAlias.Objective-tc.wantObj) > 1e-9 {
					t.Fatalf("%s/%s gb.LinearSumAssignment failed: obj=%v err=%v", tc.name, optMode.tag, resAlias.Objective, err)
				}
			}
		})
	}
}

func TestSciPySmallInputs(t *testing.T) {
	inf := math.Inf(1)

	// Ported directly from linear_sum_assignment_test_cases in
	// scipy/sparse/csgraph/tests/test_matching.py and scipy/optimize/tests/test_linear_assignment.py.
	cases := []struct {
		name         string
		matrix       [][]float64
		expectedCost []float64
	}{
		{
			name: "scipy_square_400_150",
			matrix: [][]float64{
				{400, 150, 400},
				{400, 450, 600},
				{300, 225, 300},
			},
			expectedCost: []float64{150, 400, 300},
		},
		{
			name: "scipy_rect_3x4_400_150",
			matrix: [][]float64{
				{400, 150, 400, 1},
				{400, 450, 600, 2},
				{300, 225, 300, 3},
			},
			expectedCost: []float64{150, 2, 300},
		},
		{
			name: "scipy_square_ties_10_10_8",
			matrix: [][]float64{
				{10, 10, 8},
				{9, 8, 1},
				{9, 7, 4},
			},
			expectedCost: []float64{10, 1, 7},
		},
		{
			name: "scipy_rect_3x4_ties_10_10_8_11",
			matrix: [][]float64{
				{10, 10, 8, 11},
				{9, 8, 1, 1},
				{9, 7, 4, 10},
			},
			expectedCost: []float64{10, 1, 4},
		},
		{
			name: "scipy_square_inf_10_1_7",
			matrix: [][]float64{
				{10, inf, inf},
				{inf, inf, 1},
				{inf, 7, inf},
			},
			expectedCost: []float64{10, 1, 7},
		},
		{
			name: "scipy_identity_3x3",
			matrix: [][]float64{
				{1, 0, 0},
				{0, 1, 0},
				{0, 0, 1},
			},
			expectedCost: []float64{0, 0, 0},
		},
	}

	for _, tc := range cases {
		for _, sign := range []float64{1, -1} {
			sense := assign.Minimize
			senseTag := "min_pos"
			if sign == -1 {
				sense = assign.Maximize
				senseTag = "max_neg"
			}
			t.Run(fmt.Sprintf("%s/%s", tc.name, senseTag), func(t *testing.T) {
				rows := len(tc.matrix)
				cols := len(tc.matrix[0])
				mat := make([][]float64, rows)
				flat := make([]float64, rows*cols)
				for r := range rows {
					mat[r] = make([]float64, cols)
					for c := range cols {
						v := sign * tc.matrix[r][c]
						mat[r][c] = v
						flat[r*cols+c] = v
					}
				}
				wantCosts := make([]float64, len(tc.expectedCost))
				for i, v := range tc.expectedCost {
					wantCosts[i] = sign * v
				}

				for _, optMode := range []struct {
					tag string
					opt assign.Option
				}{
					{"simd", assign.WithSIMD(true)},
					{"scalar", assign.WithoutSIMD()},
				} {
					// Check the original matrix and verify that per-row assigned costs match expectedCost.
					res, err := assign.LinearSum(t.Context(), mat, sense, optMode.opt)
					if err != nil {
						t.Fatalf("LinearSum failed: %v", err)
					}
					verifyOptimalityCertificate(t, tc.name+"/"+senseTag+"/"+optMode.tag, rows, cols, flat, sense, res)
					for i, r := range res.RowInd {
						gotEdgeCost := mat[r][res.ColInd[i]]
						if gotEdgeCost != wantCosts[i] {
							t.Fatalf("row %d assigned cost = %v, want %v (RowInd=%v, ColInd=%v)",
								r, gotEdgeCost, wantCosts[i], res.RowInd, res.ColInd)
						}
					}

					// Check the transposed matrix and verify that sorted assigned costs match sorted expectedCost.
					matT := make([][]float64, cols)
					flatT := make([]float64, cols*rows)
					for c := range cols {
						matT[c] = make([]float64, rows)
						for r := range rows {
							matT[c][r] = mat[r][c]
							flatT[c*rows+r] = mat[r][c]
						}
					}
					resT, err := assign.LinearSum(t.Context(), matT, sense, optMode.opt)
					if err != nil {
						t.Fatalf("LinearSum(T) failed: %v", err)
					}
					verifyOptimalityCertificate(t, tc.name+"/"+senseTag+"/"+optMode.tag+"/T", cols, rows, flatT, sense, resT)
					gotCostsT := make([]float64, len(resT.RowInd))
					for i, r := range resT.RowInd {
						gotCostsT[i] = matT[r][resT.ColInd[i]]
					}
					slices.Sort(gotCostsT)
					wantSorted := slices.Clone(wantCosts)
					slices.Sort(wantSorted)
					if !slices.Equal(gotCostsT, wantSorted) {
						t.Fatalf("transposed sorted edge costs = %v, want %v", gotCostsT, wantSorted)
					}
				}
			})
		}
	}
}

func TestSciPyConstantCostMatrix(t *testing.T) {
	// Ported from test_constant_cost_matrix (SciPy issue #11602).
	// A constant cost matrix must yield the identity matching RowInd = [0..k-1], ColInd = [0..k-1].
	shapes := [][2]int{
		{1, 1},
		{2, 2},
		{3, 3},
		{4, 4},
		{5, 5},
		{8, 8},
		{16, 16},
		{3, 5},
		{5, 3},
	}
	for _, sh := range shapes {
		rows, cols := sh[0], sh[1]
		k := min(rows, cols)
		for _, constVal := range []float64{1.0, 0.0, -3.5} {
			for _, sense := range []assign.Sense{assign.Minimize, assign.Maximize} {
				mat := make([][]float64, rows)
				flat := make([]float64, rows*cols)
				for r := range rows {
					mat[r] = make([]float64, cols)
					for c := range cols {
						mat[r][c] = constVal
						flat[r*cols+c] = constVal
					}
				}
				res, err := assign.LinearSum(t.Context(), mat, sense)
				if err != nil {
					t.Fatalf("constant %dx%d (val=%v, %s) failed: %v", rows, cols, constVal, sense, err)
				}
				verifyOptimalityCertificate(t, fmt.Sprintf("const_%dx%d", rows, cols), rows, cols, flat, sense, res)
				for i := range k {
					if res.RowInd[i] != i || res.ColInd[i] != i {
						t.Fatalf("constant %dx%d (val=%v, %s): got RowInd=%v ColInd=%v, want identity [0..%d]",
							rows, cols, constVal, sense, res.RowInd, res.ColInd, k-1)
					}
				}
			}
		}
	}
}

func TestValidationAndInfeasibilityErrors(t *testing.T) {
	ctx := t.Context()
	inf := math.Inf(1)
	ninf := math.Inf(-1)
	nan := math.NaN()

	// Test trivial empty matrices (0, 0), (2, 0), and (0, 3) from test_linear_sum_assignment_trivial_cost.
	resNil, err := assign.LinearSum(ctx, nil, assign.Minimize)
	if err != nil || len(resNil.RowInd) != 0 || len(resNil.ColInd) != 0 {
		t.Fatalf("expected empty result for nil matrix, got %v, %v", resNil, err)
	}
	for _, dim := range [][2]int{{0, 0}, {2, 0}, {0, 3}} {
		r, c := dim[0], dim[1]
		mat := make([][]float64, r)
		for i := range r {
			mat[i] = make([]float64, c)
		}
		wantCols2D := c
		if r == 0 {
			wantCols2D = 0
		}
		res2D, err := assign.LinearSum(ctx, mat, assign.Minimize)
		if err != nil || len(res2D.RowInd) != 0 || len(res2D.ColInd) != 0 || res2D.Objective != 0 || len(res2D.U) != r || len(res2D.V) != wantCols2D {
			t.Fatalf("LinearSum trivial (%d, %d): got %v, %v", r, c, res2D, err)
		}
		resFlat, err := assign.LinearSumFlat(ctx, r, c, nil, assign.Minimize)
		if err != nil || len(resFlat.RowInd) != 0 || len(resFlat.ColInd) != 0 || resFlat.Objective != 0 || len(resFlat.U) != r || len(resFlat.V) != c {
			t.Fatalf("LinearSumFlat trivial (%d, %d): got %v, %v", r, c, resFlat, err)
		}
		objInto, err := assign.SolveInto(ctx, r, c, nil, assign.Minimize, nil, nil, nil)
		if err != nil || objInto != 0 {
			t.Fatalf("SolveInto trivial (%d, %d): got obj=%v, err=%v", r, c, objInto, err)
		}
	}

	// Test invalid matrix dimensions, ragged rows, negative counts, integer overflow, and short slices.
	if _, err := assign.LinearSum(ctx, [][]float64{{1, 2}, {3}}, assign.Minimize); !errors.Is(err, assign.ErrInvalidDimensions) {
		t.Fatalf("expected ErrInvalidDimensions on ragged matrix, got %v", err)
	}
	if _, err := assign.LinearSumFlat(ctx, -1, 2, []float64{1, 2}, assign.Minimize); !errors.Is(err, assign.ErrInvalidDimensions) {
		t.Fatalf("expected ErrInvalidDimensions on negative rows, got %v", err)
	}
	if _, err := assign.LinearSumFlat(ctx, 2, -1, []float64{1, 2}, assign.Minimize); !errors.Is(err, assign.ErrInvalidDimensions) {
		t.Fatalf("expected ErrInvalidDimensions on negative cols, got %v", err)
	}
	if _, err := assign.LinearSumFlat(ctx, 2, 2, []float64{1, 2, 3}, assign.Minimize); !errors.Is(err, assign.ErrInvalidDimensions) {
		t.Fatalf("expected ErrInvalidDimensions on short flat slice, got %v", err)
	}
	for _, ov := range [][2]int{{1 << 62, 4}, {1 << 32, 1 << 32}, {3, math.MaxInt / 2}} {
		if _, err := assign.LinearSumFlat(ctx, ov[0], ov[1], []float64{1, 2, 3}, assign.Minimize); !errors.Is(err, assign.ErrInvalidDimensions) {
			t.Fatalf("LinearSumFlat(%d, %d): expected ErrInvalidDimensions, got %v", ov[0], ov[1], err)
		}
		if _, err := assign.SolveInto(ctx, ov[0], ov[1], []float64{1, 2, 3}, assign.Minimize, make([]int, 4), make([]int, 4), nil); !errors.Is(err, assign.ErrInvalidDimensions) {
			t.Fatalf("SolveInto(%d, %d): expected ErrInvalidDimensions, got %v", ov[0], ov[1], err)
		}
	}
	for _, badSense := range []assign.Sense{0, 2, -2} {
		if badSense.String() != "Invalid" {
			t.Fatalf("expected Sense(%d).String() == 'Invalid', got %q", badSense, badSense.String())
		}
		if _, err := assign.LinearSum(ctx, [][]float64{{1, 2}, {3, 4}}, badSense); !errors.Is(err, assign.ErrInvalidSense) {
			t.Fatalf("LinearSum with Sense(%d): expected ErrInvalidSense, got %v", badSense, err)
		}
		if _, err := assign.LinearSumFlat(ctx, 2, 2, []float64{1, 2, 3, 4}, badSense); !errors.Is(err, assign.ErrInvalidSense) {
			t.Fatalf("LinearSumFlat with Sense(%d): expected ErrInvalidSense, got %v", badSense, err)
		}
		if _, err := assign.SolveInto(ctx, 2, 2, []float64{1, 2, 3, 4}, badSense, make([]int, 2), make([]int, 2), nil); !errors.Is(err, assign.ErrInvalidSense) {
			t.Fatalf("SolveInto with Sense(%d): expected ErrInvalidSense, got %v", badSense, err)
		}
	}
	if _, err := assign.SolveInto(ctx, 2, 2, []float64{1, 2, 3, 4}, assign.Minimize, make([]int, 1), make([]int, 2), nil); !errors.Is(err, assign.ErrInvalidDimensions) {
		t.Fatalf("expected ErrInvalidDimensions on short rowInd slice, got %v", err)
	}
	if _, err := assign.SolveInto(ctx, 2, 2, []float64{1, 2, 3, 4}, assign.Minimize, make([]int, 2), make([]int, 1), nil); !errors.Is(err, assign.ErrInvalidDimensions) {
		t.Fatalf("expected ErrInvalidDimensions on short colInd slice, got %v", err)
	}

	// Test NaN and invalid signed infinities on square and tall transposed matrices.
	invalidCostCases := []struct {
		name  string
		mat   [][]float64
		sense assign.Sense
	}{
		{
			name:  "scipy_diag_nan_3x3",
			mat:   [][]float64{{nan, 0, 0}, {0, 1, 0}, {0, 0, 1}},
			sense: assign.Minimize,
		},
		{
			name:  "scipy_diag_neginf_3x3",
			mat:   [][]float64{{1, 0, 0}, {0, ninf, 0}, {0, 0, 1}},
			sense: assign.Minimize,
		},
		{
			name:  "posinf_maximize_2x2",
			mat:   [][]float64{{1, inf}, {2, 3}},
			sense: assign.Maximize,
		},
		{
			name:  "nan_maximize_2x2",
			mat:   [][]float64{{1, nan}, {2, 3}},
			sense: assign.Maximize,
		},
		{
			name:  "tall_transposed_neginf_min_3x2",
			mat:   [][]float64{{1, 2}, {3, ninf}, {4, 5}},
			sense: assign.Minimize,
		},
		{
			name:  "tall_transposed_posinf_max_3x2",
			mat:   [][]float64{{1, 2}, {3, inf}, {4, 5}},
			sense: assign.Maximize,
		},
		{
			name:  "tall_transposed_nan_min_3x2",
			mat:   [][]float64{{1, 2}, {nan, 3}, {4, 5}},
			sense: assign.Minimize,
		},
	}
	for _, tc := range invalidCostCases {
		if _, err := assign.LinearSum(ctx, tc.mat, tc.sense); !errors.Is(err, assign.ErrInvalidCost) {
			t.Fatalf("%s: expected ErrInvalidCost, got %v", tc.name, err)
		}
	}

	// Test infeasible forbidden-edge cases ported from SciPy test_matching.py.
	infeasibleMatrices := [][][]float64{
		{{inf}},
		{{inf, inf}, {1, 2}},
		{{1, inf}, {2, inf}},
		{
			{inf, 0, 0},
			{inf, 1, 0},
			{inf, 0, 1},
		},
		{
			{1, 1, 1},
			{1, inf, inf},
			{1, inf, inf},
		},
		{
			{1, 1, 1},
			{inf, inf, 1},
			{inf, inf, 1},
		},
		{
			{1, inf, inf, 1},
			{1, 1, inf, 1},
			{inf, inf, inf, inf},
		},
		{
			{1, inf, inf},
			{2, inf, inf},
		},
		{
			{inf, 1, inf},
			{inf, 2, inf},
		},
		{
			{1, inf},
			{2, inf},
			{5, inf},
		},
		{
			{1, inf, inf},
			{2, inf, inf},
			{3, 4, 5},
		},
	}
	for idx, mat := range infeasibleMatrices {
		if _, err := assign.LinearSum(ctx, mat, assign.Minimize); !errors.Is(err, assign.ErrInfeasible) {
			t.Fatalf("infeasible case %d: expected ErrInfeasible, got %v", idx, err)
		}
	}

	// Verify the SciPy issue #17269 22x22 sparse infeasible regression matrix.
	gh17269Sparse := [22][22]float64{
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0.001, 0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0.001, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0.001, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0.001, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0.001, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0.001, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0.001, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0.001, 0},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0.001},
		{0, 0.11687445, 0, 0, 0.01319788, 0.07509257, 0, 0, 0, 0.74228317, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0.81087935, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0.8408466, 0, 0, 0, 0, 0.01194389, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0.82994211, 0, 0, 0, 0.11468516, 0, 0, 0, 0.11173505, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0.18796507, 0, 0.04002318, 0, 0, 0, 0, 0, 0.75883335, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0.71545464, 0, 0, 0, 0, 0, 0.02748488, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0.78470564, 0, 0, 0, 0, 0, 0, 0, 0.14829198, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0.10870609, 0, 0, 0, 0.8918677, 0, 0, 0, 0.06306644, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0.63844085, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0.7442354, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0.09850549, 0, 0, 0.18638258, 0.2769244, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0.73182464, 0, 0, 0.46443561, 0.38589284, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0.29510278, 0, 0, 0, 0, 0, 0, 0, 0.09666032, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	}
	gh17269Min := make([][]float64, 22)
	gh17269Max := make([][]float64, 22)
	for r := range 22 {
		gh17269Min[r] = make([]float64, 22)
		gh17269Max[r] = make([]float64, 22)
		for c := range 22 {
			v := gh17269Sparse[r][c]
			if v == 0 {
				gh17269Min[r][c] = inf
				gh17269Max[r][c] = ninf
			} else {
				gh17269Min[r][c] = v
				gh17269Max[r][c] = v
			}
		}
	}
	if _, err := assign.LinearSum(ctx, gh17269Min, assign.Minimize); !errors.Is(err, assign.ErrInfeasible) {
		t.Fatalf("gh17269 22x22 minimize: expected ErrInfeasible, got %v", err)
	}
	if _, err := assign.LinearSum(ctx, gh17269Max, assign.Maximize); !errors.Is(err, assign.ErrInfeasible) {
		t.Fatalf("gh17269 22x22 maximize: expected ErrInfeasible, got %v", err)
	}
}

func TestSciPySparseRandomInputs(t *testing.T) {
	// Mirror test_two_methods_give_same_result_on_many_sparse_inputs from SciPy.
	// First, test 100 sparse 100x100 matrices at density 0.06 and check SIMD vs. Scalar parity.
	// Second, test 40 sparse 14x14 matrices at density 0.22 against Goblin's MILP solver.
	ctx := t.Context()
	rng := rand.New(rand.NewPCG(12342309, 987654321))
	inf := math.Inf(1)

	feasible100, infeasible100 := 0, 0
	for trial := range 100 {
		const n = 100
		cost := make([]float64, n*n)
		for i := range cost {
			if rng.Float64() < 0.06 {
				cost[i] = float64(1 + rng.IntN(99))
			} else {
				cost[i] = inf
			}
		}
		resSIMD, errSIMD := assign.LinearSumFlat(ctx, n, n, cost, assign.Minimize, assign.WithSIMD(true))
		resScalar, errScalar := assign.LinearSumFlat(ctx, n, n, cost, assign.Minimize, assign.WithoutSIMD())
		if (errSIMD != nil) != (errScalar != nil) {
			t.Fatalf("sparse 100x100 trial %d: SIMD err=%v vs Scalar err=%v", trial, errSIMD, errScalar)
		}
		if errSIMD != nil {
			if !errors.Is(errSIMD, assign.ErrInfeasible) {
				t.Fatalf("sparse 100x100 trial %d: unexpected error %v", trial, errSIMD)
			}
			infeasible100++
			continue
		}
		feasible100++
		verifyOptimalityCertificate(t, fmt.Sprintf("sparse_100x100_%d_simd", trial), n, n, cost, assign.Minimize, resSIMD)
		verifyOptimalityCertificate(t, fmt.Sprintf("sparse_100x100_%d_scalar", trial), n, n, cost, assign.Minimize, resScalar)
		if resSIMD.Objective != resScalar.Objective {
			t.Fatalf("sparse 100x100 trial %d: SIMD obj=%v != Scalar obj=%v", trial, resSIMD.Objective, resScalar.Objective)
		}
	}
	if feasible100 == 0 || infeasible100 == 0 {
		t.Fatalf("expected both feasible (%d) and infeasible (%d) 100x100 sparse instances", feasible100, infeasible100)
	}

	// Cross-check 14x14 sparse instances against Goblin's MILP solver.
	feasibleMILP, infeasibleMILP := 0, 0
	for trial := range 40 {
		const n = 14
		cost := make([]float64, n*n)
		for i := range cost {
			if rng.Float64() < 0.22 {
				cost[i] = float64(1 + rng.IntN(99))
			} else {
				cost[i] = inf
			}
		}
		res, err := assign.LinearSumFlat(ctx, n, n, cost, assign.Minimize)
		milpObj, milpStatus := solveViaGoblinMILP(n, n, cost, assign.Minimize)
		if err != nil {
			if !errors.Is(err, assign.ErrInfeasible) {
				t.Fatalf("sparse 14x14 trial %d: unexpected error %v", trial, err)
			}
			if milpStatus == gb.StatusOptimal {
				t.Fatalf("sparse 14x14 trial %d: LinearSumFlat reported infeasible, but MILP found optimal %v", trial, milpObj)
			}
			infeasibleMILP++
			continue
		}
		if milpStatus != gb.StatusOptimal {
			t.Fatalf("sparse 14x14 trial %d: LinearSumFlat found obj=%v, but MILP status=%v", trial, res.Objective, milpStatus)
		}
		feasibleMILP++
		verifyOptimalityCertificate(t, fmt.Sprintf("sparse_14x14_%d", trial), n, n, cost, assign.Minimize, res)
		if math.Abs(res.Objective-milpObj) > 1e-6 {
			t.Fatalf("sparse 14x14 trial %d: obj=%v != MILP obj=%v", trial, res.Objective, milpObj)
		}
	}
	if feasibleMILP == 0 || infeasibleMILP == 0 {
		t.Fatalf("expected both feasible (%d) and infeasible (%d) 14x14 sparse instances", feasibleMILP, infeasibleMILP)
	}
}

func TestSolveIntoZeroAllocations(t *testing.T) {
	n := 32
	cost := make([]float64, n*n)
	for i := range n {
		for j := range n {
			cost[i*n+j] = float64((i*17 + j*31) % 97)
		}
	}
	rowInd := make([]int, n)
	colInd := make([]int, n)
	ws := assign.AcquireWorkspace(n, n)
	defer assign.ReleaseWorkspace(ws)

	bgCtx := context.Background()
	// Warm up the workspace buffers before measuring heap allocations.
	if _, err := assign.SolveInto(bgCtx, n, n, cost, assign.Minimize, rowInd, colInd, ws); err != nil {
		t.Fatalf("SolveInto warmup failed: %v", err)
	}

	allocs := testing.AllocsPerRun(20, func() {
		_, err := assign.SolveInto(bgCtx, n, n, cost, assign.Minimize, rowInd, colInd, ws)
		if err != nil {
			t.Fatal(err)
		}
	})
	if allocs != 0 {
		t.Fatalf("expected 0 allocations in warm SolveInto, got %v", allocs)
	}

	ctx := t.Context()
	allocsWithCtx := testing.AllocsPerRun(20, func() {
		_, err := assign.SolveInto(ctx, n, n, cost, assign.Minimize, rowInd, colInd, ws)
		if err != nil {
			t.Fatal(err)
		}
	})
	if allocsWithCtx != 0 {
		t.Fatalf("expected 0 allocations in warm SolveInto with cancellable ctx, got %v", allocsWithCtx)
	}
}

func TestAssignContextCancellation(t *testing.T) {
	const n = 200
	cost := make([]float64, n*n)
	for i := range n {
		for j := range n {
			cost[i*n+j] = float64((i*37 + j*53) % 101)
		}
	}

	// Verify that a pre-canceled context returns context.Canceled immediately.
	canceledCtx, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	if _, err := assign.LinearSumFlat(canceledCtx, n, n, cost, assign.Minimize); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled on pre-canceled LinearSumFlat, got %v", err)
	}

	// Verify that an expired deadline context returns context.DeadlineExceeded.
	expiredCtx, cancelExpired := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelExpired()
	if _, err := assign.LinearSumFlat(expiredCtx, n, n, cost, assign.Minimize); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded on expired LinearSumFlat, got %v", err)
	}

	// Verify that an active context solves the instance to completion.
	activeCtx := t.Context()
	res, err := assign.LinearSumFlat(activeCtx, n, n, cost, assign.Minimize)
	if err != nil || len(res.RowInd) != n {
		t.Fatalf("expected valid solution with active context, got err=%v len=%d", err, len(res.RowInd))
	}

	// Verify that a mid-solve timeout halts promptly during augmenting path search.
	const nLarge = 1500
	largeCost := make([]float64, nLarge*nLarge)
	for i := range largeCost {
		largeCost[i] = float64((i*73 + 19) % 997)
	}
	midCtx, cancelMid := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancelMid()
	if _, err := assign.LinearSumFlat(midCtx, nLarge, nLarge, largeCost, assign.Minimize); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded on mid-solve timeout, got %v", err)
	}
}

func TestExtremeFiniteMagnitudes(t *testing.T) {
	ctx := t.Context()
	// 3x3 instance where two matched -1e308 edges and one +1e308 edge cancel to -1e308
	// without overflowing reduced costs or partial sums.
	c3 := []float64{
		1e308, -1e308, 0,
		-1e308, 1e308, 0,
		0, 0, 1e308,
	}
	res3, err := assign.LinearSumFlat(ctx, 3, 3, c3, assign.Minimize)
	if err != nil {
		t.Fatalf("3x3 1e308 minimize failed: %v", err)
	}
	if res3.Objective != -1e308 {
		t.Fatalf("3x3 1e308 objective = %v, want -1e308", res3.Objective)
	}

	// Random square, wide, and tall matrices with |c| <= 1e308 must never falsely report ErrInfeasible
	// or return NaN/Inf dual potentials.
	rng := rand.New(rand.NewPCG(918273, 192837))
	for idx := range 60 {
		rows := 2 + rng.IntN(20)
		cols := 2 + rng.IntN(20)
		sense := assign.Minimize
		if idx%2 == 1 {
			sense = assign.Maximize
		}
		c := make([]float64, rows*cols)
		for i := range c {
			c[i] = (rng.Float64()*2 - 1) * 1e308
		}
		res, err := assign.LinearSumFlat(ctx, rows, cols, c, sense)
		if err != nil {
			t.Fatalf("iter %d (%dx%d %v): unexpected error on finite 1e308 matrix: %v", idx, rows, cols, sense, err)
		}
		for _, u := range res.U {
			if math.IsNaN(u) || math.IsInf(u, 0) {
				t.Fatalf("iter %d: non-finite U dual %v", idx, u)
			}
		}
		for _, v := range res.V {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Fatalf("iter %d: non-finite V dual %v", idx, v)
			}
		}
	}
}

func TestRowDominatedAndRank1Variants(t *testing.T) {
	ctx := t.Context()
	rng := rand.New(rand.NewPCG(445566, 778899))
	const n = 120

	buildCases := []struct {
		name string
		gen  func() []float64
	}{
		{
			name: "rank1_pos_b",
			gen: func() []float64 {
				a := make([]float64, n)
				b := make([]float64, n)
				for i := range n {
					a[i] = float64(100 + rng.IntN(900))
					b[i] = float64(100 + rng.IntN(900))
				}
				c := make([]float64, n*n)
				for r := range n {
					for j := range n {
						c[r*n+j] = a[r]*b[j] + float64(rng.IntN(64))/64.0
					}
				}
				return c
			},
		},
		{
			name: "rank1_neg_b",
			gen: func() []float64 {
				a := make([]float64, n)
				b := make([]float64, n)
				for i := range n {
					a[i] = float64(100 + rng.IntN(900))
					b[i] = -float64(100 + rng.IntN(900))
				}
				c := make([]float64, n*n)
				for r := range n {
					for j := range n {
						c[r*n+j] = a[r]*b[j] + float64(rng.IntN(64))/64.0
					}
				}
				return c
			},
		},
		{
			name: "row_dominated_single",
			gen: func() []float64 {
				c := make([]float64, n*n)
				for i := range c {
					c[i] = 100 + rng.Float64()*100
				}
				for j := range n {
					c[j] = rng.Float64()
				}
				return c
			},
		},
		{
			name: "row_and_col_dominated",
			gen: func() []float64 {
				c := make([]float64, n*n)
				for i := range c {
					c[i] = 100 + rng.Float64()*100
				}
				for j := range n {
					c[j] = rng.Float64() * 0.1
				}
				for r := 1; r < n; r++ {
					c[r*n+0] = 1.0
					c[r*n+1] = 2.0
				}
				return c
			},
		},
	}

	for _, tc := range buildCases {
		cost := tc.gen()
		res, err := assign.LinearSumFlat(ctx, n, n, cost, assign.Minimize)
		if err != nil {
			t.Fatalf("%s: unexpected error %v", tc.name, err)
		}
		verifyOptimalityCertificate(t, tc.name, n, n, cost, assign.Minimize, res)
	}
}
