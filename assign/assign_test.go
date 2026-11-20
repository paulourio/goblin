package assign_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/bits"
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
	scaleRef := maxAbsCost
	if scaleRef == 0 {
		scaleRef = 1.0
	}
	tol := certRelTol * scaleRef * float64(max(rows, cols))

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
	for _, ov := range [][2]int{{1 << (bits.UintSize - 2), 4}, {1 << (bits.UintSize / 2), 1 << (bits.UintSize / 2)}, {3, math.MaxInt / 2}} {
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

	// Verify that a mid-solve timeout halts promptly during preprocessing or augmenting path search.
	const nLarge = 2500
	largeCost := make([]float64, nLarge*nLarge)
	for r := range nLarge {
		row := largeCost[r*nLarge : (r+1)*nLarge]
		rf := float64(r + 1)
		for c := range nLarge {
			row[c] = rf*float64(c+1) + float64((r*31+c*17)%7)*1e-6
		}
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

func TestExtremeScaleOffsetAndDownscaleBruteForce(t *testing.T) {
	ctx := t.Context()

	// Empty inputs return non-nil slices.
	resZero2D, err := assign.LinearSum(ctx, [][]float64{}, assign.Minimize)
	if err != nil || resZero2D.RowInd == nil || resZero2D.ColInd == nil || resZero2D.U == nil || resZero2D.V == nil {
		t.Fatalf("LinearSum(0x0) returned nil slices or err=%v: %+v", err, resZero2D)
	}

	regimes := []struct {
		name   string
		scale  float64
		offset float64
	}{
		{name: "scale_1e-14", scale: 1e-14, offset: 0},
		{name: "scale_1e-16", scale: 1e-16, offset: 0},
		{name: "scale_1e-20", scale: 1e-20, offset: 0},
		{name: "offset_1e14", scale: 1, offset: 1e14},
		{name: "offset_1e15", scale: 1, offset: 1e15},
	}

	for _, rg := range regimes {
		rng := rand.New(rand.NewPCG(11, 12))
		for trial := range 600 {
			rows := 2 + rng.IntN(5)
			cols := 2 + rng.IntN(5)
			sense := assign.Minimize
			if trial%2 == 1 {
				sense = assign.Maximize
			}
			flat := make([]float64, rows*cols)
			mat := make([][]float64, rows)
			for r := range rows {
				mat[r] = make([]float64, cols)
				for c := range cols {
					v := rg.offset + rg.scale*float64(rng.IntN(10))
					flat[r*cols+c] = v
					mat[r][c] = v
				}
			}
			want, ok := exactAssignmentObjective(rows, cols, flat, sense)
			if !ok {
				t.Fatalf("%s trial %d: unexpected infeasible oracle", rg.name, trial)
			}
			res, err := assign.LinearSum(ctx, mat, sense)
			if err != nil {
				t.Fatalf("%s trial %d: unexpected error %v", rg.name, trial, err)
			}
			verifyOptimalityCertificate(t, rg.name, rows, cols, flat, sense, res)

			gotDelta := 0.0
			for idx := range res.RowInd {
				gotDelta += mat[res.RowInd[idx]][res.ColInd[idx]] - rg.offset
			}
			wantDelta := want - float64(min(rows, cols))*rg.offset
			if math.Abs(gotDelta-wantDelta) > 1e-9*rg.scale*float64(max(rows, cols)) {
				t.Fatalf("%s trial %d (%dx%d %s): got delta %.15g, want %.15g",
					rg.name, trial, rows, cols, sense, gotDelta, wantDelta)
			}
		}
	}

	// Verify that mixed 1e299 penalty entries combined with small integer costs trigger the 0x1p-64 exact power-of-two downscale in Jonker-Volgenant preprocessing without losing unit-level optimality resolution across rectangular and square instances.
	rng := rand.New(rand.NewPCG(91, 92))
	for trial := range 300 {
		rows := 3 + rng.IntN(3)
		cols := 3 + rng.IntN(3)
		k := min(rows, cols)
		flat := make([]float64, rows*cols)
		for i := range flat {
			flat[i] = float64(rng.IntN(10))
		}
		// Place 1e299 blockers off at least one valid permutation so optimum stays finite and small.
		perm := rng.Perm(cols)
		protected := make(map[[2]int]bool, k)
		for r := range k {
			protected[[2]int{r, perm[r]}] = true
		}
		placed := false
		for r := range rows {
			for c := range cols {
				if !protected[[2]int{r, c}] && rng.IntN(4) == 0 {
					flat[r*cols+c] = 1e299
					placed = true
				}
			}
		}
		if !placed {
			for r := range rows {
				for c := range cols {
					if !protected[[2]int{r, c}] {
						flat[r*cols+c] = 1e299
						placed = true
						break
					}
				}
				if placed {
					break
				}
			}
		}
		want, ok := exactAssignmentObjective(rows, cols, flat, assign.Minimize)
		if !ok {
			t.Fatalf("downscale trial %d: expected finite matching", trial)
		}
		res, err := assign.LinearSumFlat(ctx, rows, cols, flat, assign.Minimize)
		if err != nil {
			t.Fatalf("downscale trial %d: unexpected error %v", trial, err)
		}
		if math.Abs(res.Objective-want) > 1e-9*max(1.0, math.Abs(want)) {
			t.Fatalf("downscale trial %d: got %.15g, want %.15g", trial, res.Objective, want)
		}
	}
}

// TestAssignPeerReviewRegressions verifies dual-feasibility preservation in augmenting row
// reduction, minimal power-of-two downscaling with unscaled objective accumulation, extreme-cost
// dual scaling, empty-matrix context cancellation, ApplyOptions, and root goblin aliases.
func TestAssignPeerReviewRegressions(t *testing.T) {
	ctx := t.Context()

	// 1. C9: Augmenting row reduction must never increase v[j1] when matching an unassigned column.
	// Columns 1 and 2 contend for row 0 with v[1]=v[2]=-1000 while row 1 has 1e20 on both columns.
	c9Mat := [][]float64{
		{1000, -1000, -1000, 1000},
		{1e21, 1e20, 1e20, 1e21},
		{1000, -999, 1000, 0},
		{0, 1000, 1000, 1000},
	}
	c9Flat := make([]float64, 0, 16)
	for _, row := range c9Mat {
		c9Flat = append(c9Flat, row...)
	}
	c9Res, err := assign.LinearSum(ctx, c9Mat, assign.Minimize)
	if err != nil {
		t.Fatalf("C9 matrix solve failed: %v", err)
	}
	verifyOptimalityCertificate(t, "C9_square", 4, 4, c9Flat, assign.Minimize, c9Res)
	// Also test a 7x7 square matrix where a free row sees 1e20 on contended columns with negative v[j1]
	// alongside another free row that exposes any broken dual feasibility in augmentRow.
	c9Mat7 := [][]float64{
		{500, -1000, -1000, -1000, 500, 500, 500},
		{500, 500, 500, 500, 500, -999, 500},
		{500, -999, 1.5, 500, 500, 500, 500},
		{500, 1e20, 1e20, 0, 500, 500, 500},
		{500, 500, 500, 500, 0, 500, 500},
		{0, 500, 500, 500, 500, 500, 500},
		{500, 500, 500, 500, 500, 500, 0},
	}
	c9Flat7 := make([]float64, 0, 49)
	for _, row := range c9Mat7 {
		c9Flat7 = append(c9Flat7, row...)
	}
	want7, ok7 := exactAssignmentObjective(7, 7, c9Flat7, assign.Minimize)
	if !ok7 {
		t.Fatal("C9 7x7 expected finite optimum")
	}
	c9Res7, err := assign.LinearSum(ctx, c9Mat7, assign.Minimize)
	if err != nil {
		t.Fatalf("C9 7x7 solve failed: %v", err)
	}
	verifyOptimalityCertificate(t, "C9_7x7", 7, 7, c9Flat7, assign.Minimize, c9Res7)
	if math.Abs(c9Res7.Objective-want7) > 1e-9*max(1.0, math.Abs(want7)) {
		t.Fatalf("C9 7x7 objective = %.15g, want %.15g", c9Res7.Objective, want7)
	}

	// 2. C10: Huge costs paired with small normal (1e-306) and subnormal (1e-315) entries preserve
	// discrimination and unscaled objective values.
	for _, tc := range []struct {
		name string
		mat  [][]float64
		want float64
	}{
		{
			name: "1e291_with_normal_1e-306",
			mat: [][]float64{
				{1e-306, 1e-315, 1e291},
				{0, 0, 1e291},
				{1e291, 1e291, 0},
			},
			want: 1e-315,
		},
		{
			name: "1e308_with_subnormal_1e-315",
			mat: [][]float64{
				{1e-310, 1e-315, 1e308},
				{0, 0, 1e308},
				{1e308, 1e308, 0},
			},
			want: 1e-315,
		},
	} {
		res, sErr := assign.LinearSum(ctx, tc.mat, assign.Minimize)
		if sErr != nil {
			t.Fatalf("%s: unexpected error %v", tc.name, sErr)
		}
		if res.Objective != tc.want {
			t.Fatalf("%s: Objective = %.17g, want %.17g (cols=%v)", tc.name, res.Objective, tc.want, res.ColInd)
		}
		if !slices.Equal(res.ColInd, []int{1, 0, 2}) {
			t.Fatalf("%s: ColInd = %v, want [1 0 2]", tc.name, res.ColInd)
		}
	}

	// 3. M11: Extreme-cost duals remain dual-feasible after upscaling.
	extremeCases := [][][]float64{
		{
			{5e307, -5e307, 0},
			{-5e307, 5e307, 0},
			{0, 0, 5e307},
		},
		{
			{1e307, 2e307, 3e307},
			{4e307, 1e307, 2e307},
			{2e307, 3e307, 1e307},
		},
	}
	for idx, mat := range extremeCases {
		flat := make([]float64, 0, 9)
		for _, row := range mat {
			flat = append(flat, row...)
		}
		res, sErr := assign.LinearSum(ctx, mat, assign.Minimize)
		if sErr != nil {
			t.Fatalf("M11 case %d: unexpected error %v", idx, sErr)
		}
		verifyOptimalityCertificate(t, fmt.Sprintf("M11_case_%d", idx), 3, 3, flat, assign.Minimize, res)
	}
	// Verify cancelling +-1e308 entries still yield finite objective -1e308 and dual-feasible (u, v).
	mat1e308 := [][]float64{
		{1e308, -1e308, 0},
		{-1e308, 1e308, 0},
		{0, 0, 1e308},
	}
	res1e308, err := assign.LinearSum(ctx, mat1e308, assign.Minimize)
	if err != nil {
		t.Fatalf("M11 1e308: unexpected error %v", err)
	}
	if math.Abs(res1e308.Objective-(-1e308)) > 1e296 {
		t.Fatalf("M11 1e308 objective = %.15g, want -1e308", res1e308.Objective)
	}
	for i := range 3 {
		for j := range 3 {
			if viol := res1e308.U[i] + res1e308.V[j] - mat1e308[i][j]; viol > 1e297 {
				t.Fatalf("M11 1e308 dual violation at (%d,%d): u=%.6e v=%.6e c=%.6e viol=%.6e",
					i, j, res1e308.U[i], res1e308.V[j], mat1e308[i][j], viol)
			}
		}
	}

	// 4. m1: Already-canceled context returns context.Canceled even on empty matrices.
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := assign.LinearSum(canceledCtx, nil, assign.Minimize); !errors.Is(err, context.Canceled) {
		t.Fatalf("empty LinearSum with canceled ctx: err=%v, want context.Canceled", err)
	}
	if _, err := assign.LinearSum(canceledCtx, [][]float64{{}, {}}, assign.Minimize); !errors.Is(err, context.Canceled) {
		t.Fatalf("2x0 LinearSum with canceled ctx: err=%v, want context.Canceled", err)
	}
	if _, err := assign.LinearSumFlat(canceledCtx, 0, 3, nil, assign.Minimize); !errors.Is(err, context.Canceled) {
		t.Fatalf("0x3 LinearSumFlat with canceled ctx: err=%v, want context.Canceled", err)
	}

	// 5. Coverage for assign.ApplyOptions and root goblin LSAP aliases.
	cfg := assign.ApplyOptions(assign.WithoutSIMD())
	if !cfg.DisableSIMD {
		t.Fatal("assign.ApplyOptions(WithNoSIMD()) expected DisableSIMD=true")
	}
	aliasMat := [][]float64{{4, 1}, {2, 3}}
	rAlias, err := gb.LinearSumAssignment(ctx, aliasMat, gb.Minimize)
	if err != nil || math.Abs(rAlias.Objective-3.0) > 1e-12 || !slices.Equal(rAlias.ColInd, []int{1, 0}) {
		t.Fatalf("gb.LinearSumAssignment = (%+v, %v), want obj=3 cols=[1 0]", rAlias, err)
	}
	rFlatAlias, err := gb.LinearSumAssignmentFlat(ctx, 2, 2, []float64{4, 1, 2, 3}, gb.Maximize)
	if err != nil || math.Abs(rFlatAlias.Objective-7.0) > 1e-12 || !slices.Equal(rFlatAlias.ColInd, []int{0, 1}) {
		t.Fatalf("gb.LinearSumAssignmentFlat = (%+v, %v), want obj=7 cols=[0 1]", rFlatAlias, err)
	}
}

func TestAdversarialScaledRectangularAndSubnormalLSAP(t *testing.T) {
	ctx := t.Context()

	// CRIT-6a: Scaled rectangular wide (2x3) and tall (3x2) instances must keep zero
	// dual potential on unassigned columns/rows and preserve strong duality.
	wide := [][]float64{
		{1e307, 2e307, 3e307},
		{2e307, 1e307, 3e307},
	}
	resWide, err := assign.LinearSum(ctx, wide, assign.Minimize)
	if err != nil {
		t.Fatalf("wide 2x3 1e307 failed: %v", err)
	}
	if math.Abs(resWide.Objective-2e307) > 1e293 {
		t.Fatalf("wide 2x3 objective = %.15e, want 2e307", resWide.Objective)
	}
	if resWide.V[2] != 0 {
		t.Fatalf("wide 2x3 unassigned V[2] = %.6e, want 0", resWide.V[2])
	}
	dualWide := resWide.U[0] + resWide.U[1] + resWide.V[0] + resWide.V[1] + resWide.V[2]
	if math.Abs(dualWide-resWide.Objective) > 1e293 {
		t.Fatalf("wide 2x3 strong duality broken: dual=%.15e primal=%.15e", dualWide, resWide.Objective)
	}

	tall := [][]float64{
		{1e307, 2e307},
		{2e307, 1e307},
		{3e307, 3e307},
	}
	resTall, err := assign.LinearSum(ctx, tall, assign.Minimize)
	if err != nil {
		t.Fatalf("tall 3x2 1e307 failed: %v", err)
	}
	if math.Abs(resTall.Objective-2e307) > 1e293 {
		t.Fatalf("tall 3x2 objective = %.15e, want 2e307", resTall.Objective)
	}
	if resTall.U[2] != 0 {
		t.Fatalf("tall 3x2 unassigned U[2] = %.6e, want 0", resTall.U[2])
	}
	dualTall := resTall.U[0] + resTall.U[1] + resTall.U[2] + resTall.V[0] + resTall.V[1]
	if math.Abs(dualTall-resTall.Objective) > 1e293 {
		t.Fatalf("tall 3x2 strong duality broken: dual=%.15e primal=%.15e", dualTall, resTall.Objective)
	}

	// CRIT-6b: Extreme square 3x3 (+-1.5e308) and 4x4 canceling sum (two -1e308 and two +1e308).
	sq3 := [][]float64{
		{-1.5e308, 1.5e308, 0},
		{1.5e308, 0, -1.5e308},
		{0, 1.5e308, 1.5e308},
	}
	resSq3, err := assign.LinearSum(ctx, sq3, assign.Minimize)
	if err != nil {
		t.Fatalf("sq3 +-1.5e308 failed: %v", err)
	}
	if math.IsInf(resSq3.Objective, 0) || math.Abs(resSq3.Objective-(-1.5e308)) > 1e294 {
		t.Fatalf("sq3 objective = %.15e, want -1.5e308", resSq3.Objective)
	}
	for i := range 3 {
		for j := range 3 {
			if viol := resSq3.U[i] + resSq3.V[j] - sq3[i][j]; viol > 1e294 {
				t.Fatalf("sq3 dual violation (%d,%d): u=%.6e v=%.6e c=%.6e viol=%.6e",
					i, j, resSq3.U[i], resSq3.V[j], sq3[i][j], viol)
			}
		}
	}

	sq4 := [][]float64{
		{-1e308, 1e308, 1e308, 1e308},
		{1e308, -1e308, 1e308, 1e308},
		{1e308, 1e308, 1e308, 1e308},
		{1e308, 1e308, 1e308, 1e308},
	}
	resSq4, err := assign.LinearSum(ctx, sq4, assign.Minimize)
	if err != nil {
		t.Fatalf("sq4 canceling +-1e308 failed: %v", err)
	}
	if math.IsInf(resSq4.Objective, 0) || math.Abs(resSq4.Objective) > 1e293 {
		t.Fatalf("sq4 objective = %.15e, want 0", resSq4.Objective)
	}

	// MAJ-1: Mixed subnormal (5e-324, 1e-322) and 1e308 entries must preserve exact subnormal
	// tie-breaking instead of flushing both subnormals to 0 under 2^-7 downscaling.
	tiny := math.SmallestNonzeroFloat64
	sub20 := math.Float64frombits(20)
	matSub := [][]float64{
		{sub20, tiny, 1e308},
		{0, sub20, 1e308},
		{1e308, 1e308, 0},
	}
	resSub, err := assign.LinearSum(ctx, matSub, assign.Minimize)
	if err != nil {
		t.Fatalf("subnormal+1e308 Minimize failed: %v", err)
	}
	if resSub.Objective != tiny || !slices.Equal(resSub.ColInd, []int{1, 0, 2}) {
		t.Fatalf("subnormal+1e308 Minimize = (obj=%g cols=%v), want (obj=%g cols=[1 0 2])",
			resSub.Objective, resSub.ColInd, tiny)
	}

	// Maximize counterpart with negated entries.
	matSubMax := [][]float64{
		{-sub20, -tiny, -1e308},
		{0, -sub20, -1e308},
		{-1e308, -1e308, 0},
	}
	resSubMax, err := assign.LinearSum(ctx, matSubMax, assign.Maximize)
	if err != nil {
		t.Fatalf("subnormal+1e308 Maximize failed: %v", err)
	}
	if resSubMax.Objective != -tiny || !slices.Equal(resSubMax.ColInd, []int{1, 0, 2}) {
		t.Fatalf("subnormal+1e308 Maximize = (obj=%g cols=%v), want (obj=%g cols=[1 0 2])",
			resSubMax.Objective, resSubMax.ColInd, -tiny)
	}

	// Coupled 4x4 cycle with 1e308, normal 1/3 costs, and subnormal tie-breaking (5e-324 vs 9.88e-322).
	sub200 := tiny * 200
	third := 1.0 / 3.0
	matCycle := [][]float64{
		{third, 2.0 * third, 1e308, 1e308},
		{1e308, 2.0 * third, third, 1e308},
		{1e308, 1e308, sub200, tiny},
		{0, 1e308, 1e308, 0},
	}
	resCycle, err := assign.LinearSum(ctx, matCycle, assign.Minimize)
	if err != nil {
		t.Fatalf("coupled subnormal+1e308 4x4 failed: %v", err)
	}
	if !slices.Equal(resCycle.ColInd, []int{1, 2, 3, 0}) {
		t.Fatalf("coupled subnormal+1e308 4x4 ColInd=%v, want [1 2 3 0]", resCycle.ColInd)
	}
	flatCycle := make([]float64, 0, 16)
	for _, r := range matCycle {
		flatCycle = append(flatCycle, r...)
	}
	verifyOptimalityCertificate(t, "coupled_subnormal_4x4", 4, 4, flatCycle, assign.Minimize, resCycle)

	// Rectangular 2x3 and 2x4 with 1e308 and negative/positive subnormal residual edges.
	cNegSub := -math.Ldexp(tiny, 8)
	matRectSub := [][]float64{
		{cNegSub, tiny, 1e308},
		{cNegSub, tiny * 10, tiny * 3},
	}
	resRectSub, err := assign.LinearSum(ctx, matRectSub, assign.Minimize)
	if err != nil || !slices.Equal(resRectSub.ColInd, []int{1, 0}) {
		t.Fatalf("rect subnormal+1e308 2x3: err=%v cols=%v, want [1 0]", err, resRectSub.ColInd)
	}
	for i := range 2 {
		for j := range 3 {
			if resRectSub.U[i]+resRectSub.V[j] > matRectSub[i][j]+1e-320 {
				t.Fatalf("rect subnormal+1e308 dual violation at (%d,%d): u+v=%g > c=%g",
					i, j, resRectSub.U[i]+resRectSub.V[j], matRectSub[i][j])
			}
		}
	}

	// Stress 200 deterministic 5x5 matrices mixing +-1e305 normal blocks, 1e308, and subnormals
	// so Stage-1 dual roundoff (~1e289) never overflows Stage-2 resCost to -Inf or NaN.
	var seed uint64 = 1
	nextRand := func() uint64 {
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		return seed
	}
	for trial := range 200 {
		mat5 := make([][]float64, 5)
		for i := range 5 {
			mat5[i] = make([]float64, 5)
			for j := range 5 {
				r := nextRand()
				switch r % 6 {
				case 0:
					mat5[i][j] = 1e308
				case 1:
					mat5[i][j] = tiny * float64(1+(r%15))
				default:
					sgn := 1.0
					if (r>>8)&1 == 1 {
						sgn = -1.0
					}
					mat5[i][j] = sgn * 1e305 * (1.0 + float64((r>>16)%10000)/10000.0)
				}
			}
			mat5[i][i] = min(mat5[i][i], 1e305)
		}
		mat5[0][1] = tiny
		mat5[4][4] = 1e308
		res5, sErr := assign.LinearSum(ctx, mat5, assign.Minimize)
		if sErr != nil {
			continue
		}
		for i, u := range res5.U {
			if math.IsNaN(u) || math.IsInf(u, 0) {
				t.Fatalf("trial %d: non-finite U[%d]=%v", trial, i, u)
			}
		}
		for j, v := range res5.V {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Fatalf("trial %d: non-finite V[%d]=%v", trial, j, v)
			}
		}
		for i := range 5 {
			for j := range 5 {
				c := mat5[i][j]
				tol := 1e-11 * max(1.0, math.Abs(c)+math.Abs(res5.U[i])+math.Abs(res5.V[j]))
				if res5.U[i]+res5.V[j] > c+tol {
					t.Fatalf("trial %d (%d,%d): dual violation u+v=%.15g > c=%.15g",
						trial, i, j, res5.U[i]+res5.V[j], c)
				}
			}
		}
	}

	// Stage-2 must preserve multi-ULP normal reduced-cost gaps (2^970 at 2^1020 scale)
	// when subnormal entries exist in other columns, rather than zeroing 0 < d <= fpTol.
	hi := math.Ldexp(1, 1020)
	hiGap := hi + math.Ldexp(1, 970)
	matGap3 := [][]float64{
		{hi, math.Inf(1), tiny},
		{math.Inf(1), hiGap, sub200},
		{hi, hi, math.Inf(1)},
	}
	resGap3, err := assign.LinearSum(ctx, matGap3, assign.Minimize)
	if err != nil {
		t.Fatalf("Stage-2 normal-gap 3x3 Minimize failed: %v", err)
	}
	if !slices.Equal(resGap3.ColInd, []int{0, 2, 1}) {
		t.Fatalf("Stage-2 normal-gap 3x3 ColInd=%v, want [0 2 1]", resGap3.ColInd)
	}
	flatGap3 := make([]float64, 0, 9)
	for _, r := range matGap3 {
		flatGap3 = append(flatGap3, r...)
	}
	verifyOptimalityCertificate(t, "stage2_normal_gap_3x3", 3, 3, flatGap3, assign.Minimize, resGap3)

	// Coupled 5x5 combining the 2^970 normal gap with an independent 2x2 subnormal tie block.
	matGap5 := [][]float64{
		{hi, math.Inf(1), tiny, math.Inf(1), math.Inf(1)},
		{math.Inf(1), hiGap, sub200, math.Inf(1), math.Inf(1)},
		{hi, hi, math.Inf(1), math.Inf(1), math.Inf(1)},
		{math.Inf(1), math.Inf(1), math.Inf(1), tiny, sub200},
		{math.Inf(1), math.Inf(1), math.Inf(1), tiny, tiny * 2},
	}
	resGap5, err := assign.LinearSum(ctx, matGap5, assign.Minimize)
	if err != nil {
		t.Fatalf("coupled 5x5 normal-gap + subnormal tie failed: %v", err)
	}
	if !slices.Equal(resGap5.ColInd, []int{0, 2, 1, 3, 4}) {
		t.Fatalf("coupled 5x5 ColInd=%v, want [0 2 1 3 4]", resGap5.ColInd)
	}
	flatGap5 := make([]float64, 0, 25)
	for _, r := range matGap5 {
		flatGap5 = append(flatGap5, r...)
	}
	verifyOptimalityCertificate(t, "stage2_coupled_5x5", 5, 5, flatGap5, assign.Minimize, resGap5)

	// Rectangular 2x3 with 2^970 normal gap, negative column duals (mustAssign), and subnormals.
	matGapRect := [][]float64{
		{-hi, hiGap, tiny},
		{-hi, hi, sub200},
	}
	resGapRect, err := assign.LinearSum(ctx, matGapRect, assign.Minimize)
	if err != nil {
		t.Fatalf("rectangular 2x3 normal-gap + subnormal failed: %v", err)
	}
	if !slices.Equal(resGapRect.ColInd, []int{2, 0}) {
		t.Fatalf("rectangular 2x3 normal-gap ColInd=%v, want [2 0]", resGapRect.ColInd)
	}
	flatGapRect := make([]float64, 0, 6)
	for _, r := range matGapRect {
		flatGapRect = append(flatGapRect, r...)
	}
	verifyOptimalityCertificate(t, "stage2_rect_2x3", 2, 3, flatGapRect, assign.Minimize, resGapRect)

	// Transposed 3x2 rectangular normal-gap + mustAssign + subnormal certificate check.
	flatGapTall := []float64{-hi, -hi, hiGap, hi, tiny, sub200}
	resGapTall, err := assign.LinearSumFlat(ctx, 3, 2, flatGapTall, assign.Minimize)
	if err != nil {
		t.Fatalf("rectangular 3x2 normal-gap + subnormal failed: %v", err)
	}
	verifyOptimalityCertificate(t, "stage2_tall_3x2", 3, 2, flatGapTall, assign.Minimize, resGapTall)

	// Rectangular 5x4 three-scale case (1.12e307, +-3.99e292, subnormals) verifying bounded colBonus
	// does not flip v[j] > 0 or break complementary slackness after transpose.
	flatRect5x4 := []float64{
		math.Inf(1), -7.98336123813888e292, 1.1235582092889534e307, math.Inf(1),
		-2.3e-322, math.Inf(1), 3.99168061906944e292, 0,
		-1e-323, -1.43e-322, 0, 2.17e-322,
		0, 0, 0, -1.1235582092889444e307,
		1.123558209288957e307, -1.1235582092889395e307, -1.1235582092889514e307, 0,
	}
	resRect5x4, err := assign.LinearSumFlat(ctx, 5, 4, flatRect5x4, assign.Minimize)
	if err != nil {
		t.Fatalf("rectangular 5x4 three-scale Minimize failed: %v", err)
	}
	verifyOptimalityCertificate(t, "stage2_rect_5x4_colbonus", 5, 4, flatRect5x4, assign.Minimize, resRect5x4)

	// Divisible subnormal (8 * 5e-324 = 4e-323 with downExp = 3) and unscaled >1e290 + subnormal 2x2 cases.
	matDivSub := [][]float64{
		{4e-323, -hi},
		{hi, 0},
	}
	resDivSub, err := assign.LinearSum(ctx, matDivSub, assign.Minimize)
	if err != nil {
		t.Fatalf("divisible subnormal 2x2 failed: %v", err)
	}
	if !slices.Equal(resDivSub.ColInd, []int{1, 0}) {
		t.Fatalf("divisible subnormal 2x2 ColInd=%v, want [1 0]", resDivSub.ColInd)
	}
	verifyOptimalityCertificate(t, "stage2_div_sub_2x2", 2, 2, []float64{4e-323, -hi, hi, 0}, assign.Minimize, resDivSub)

	c292 := math.Ldexp(1, 972)
	matUnscaledSub := [][]float64{
		{2e-323, -c292},
		{c292, -4e-323},
	}
	resUnscaledSub, err := assign.LinearSum(ctx, matUnscaledSub, assign.Maximize)
	if err != nil {
		t.Fatalf("unscaled 2^972 + subnormal 2x2 Maximize failed: %v", err)
	}
	if !slices.Equal(resUnscaledSub.ColInd, []int{1, 0}) {
		t.Fatalf("unscaled 2^972 + subnormal 2x2 ColInd=%v, want [1 0]", resUnscaledSub.ColInd)
	}
	verifyOptimalityCertificate(t, "stage2_unscaled_sub_2x2", 2, 2, []float64{2e-323, -c292, c292, -4e-323}, assign.Maximize, resUnscaledSub)
}

func TestAssignPotentialCancellationPermutations(t *testing.T) {
	ctx := t.Context()
	base := [3][3]float64{
		{1e17, 0, 1e18},
		{1e18, 9, 0},
		{14, 1e18, -1e17},
	}
	perms := [6][3]int{
		{0, 1, 2},
		{0, 2, 1},
		{1, 0, 2},
		{1, 2, 0},
		{2, 0, 1},
		{2, 1, 0},
	}
	for _, rp := range perms {
		for _, cp := range perms {
			flatMin := make([]float64, 9)
			flatMax := make([]float64, 9)
			for i := range 3 {
				for j := range 3 {
					v := base[rp[i]][cp[j]]
					flatMin[i*3+j] = v
					flatMax[i*3+j] = -v
				}
			}
			for _, opt := range [][]assign.Option{{}, {assign.WithoutSIMD()}} {
				resMin, err := assign.LinearSumFlat(ctx, 3, 3, flatMin, assign.Minimize, opt...)
				if err != nil {
					t.Fatalf("rp=%v cp=%v Minimize err: %v", rp, cp, err)
				}
				if resMin.Objective != 9.0 {
					t.Fatalf("rp=%v cp=%v Minimize obj=%.17g, want 9", rp, cp, resMin.Objective)
				}
				resMax, err := assign.LinearSumFlat(ctx, 3, 3, flatMax, assign.Maximize, opt...)
				if err != nil {
					t.Fatalf("rp=%v cp=%v Maximize err: %v", rp, cp, err)
				}
				if resMax.Objective != -9.0 {
					t.Fatalf("rp=%v cp=%v Maximize obj=%.17g, want -9", rp, cp, resMax.Objective)
				}
			}
		}
	}

	wantLo := math.Ldexp(1, -970)
	matLo := [][]float64{
		{math.Ldexp(1, 960), 1e300, 1e300, 1e300, 1e300, 1e300},
		{1e300, -math.Ldexp(1, 958), 1e300, 1e300, 1e300, 1e300},
		{1e300, 1e300, -math.Ldexp(1, 958), 1e300, 1e300, 1e300},
		{1e300, 1e300, 1e300, -math.Ldexp(1, 958), 1e300, 1e300},
		{1e300, 1e300, 1e300, 1e300, -math.Ldexp(1, 958), 1e300},
		{1e300, 1e300, 1e300, 1e300, 1e300, wantLo},
	}
	resLo, err := assign.LinearSum(ctx, matLo, assign.Minimize)
	if err != nil || resLo.Objective != wantLo {
		t.Fatalf("assign lo after hi+mid cancel = (%g, %v), want %g", resLo.Objective, err, wantLo)
	}
}
