package assign

import (
	"context"
	"errors"
	"math"
	"math/big"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func withAssignCertFault(ctx context.Context, fn assignFaultFunc) context.Context {
	return context.WithValue(ctx, certFaultKey{}, fn)
}

func TestAssignCertifiedValidModes(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	t.Run("EmptyMatrices", func(t *testing.T) {
		t.Parallel()
		for _, dims := range [][2]int{{0, 0}, {0, 4}, {3, 0}} {
			r, c := dims[0], dims[1]
			for _, sense := range []Sense{Minimize, Maximize} {
				mat := make([][]float64, r)
				for i := range r {
					mat[i] = make([]float64, c)
				}
				res2D, err := LinearSum(ctx, mat, sense, WithCertified(true))
				if err != nil {
					t.Fatalf("LinearSum(%dx%d, %v): %v", r, c, sense, err)
				}
				if len(res2D.RowInd) != 0 || len(res2D.ColInd) != 0 || res2D.Objective != 0 {
					t.Fatalf("LinearSum(%dx%d) unexpected result: %+v", r, c, res2D)
				}

				resFlat, err := LinearSumFlat(ctx, r, c, nil, sense, WithCertified(true))
				if err != nil {
					t.Fatalf("LinearSumFlat(%dx%d, %v): %v", r, c, sense, err)
				}
				if resFlat.Objective != 0 {
					t.Fatalf("LinearSumFlat(%dx%d) obj=%v, want 0", r, c, resFlat.Objective)
				}

				objInto, err := SolveInto(ctx, r, c, nil, sense, nil, nil, nil, WithCertified(true))
				if err != nil || objInto != 0 {
					t.Fatalf("SolveInto(%dx%d, %v): obj=%v err=%v", r, c, sense, objInto, err)
				}
			}
		}
	})

	t.Run("SquareWideTallAndNonDyadic", func(t *testing.T) {
		t.Parallel()
		rng := rand.New(rand.NewPCG(20261122, 991))
		shapes := [][2]int{{1, 1}, {1, 5}, {5, 1}, {3, 3}, {4, 7}, {7, 4}, {10, 10}, {6, 11}, {11, 6}}
		for idx, sh := range shapes {
			rows, cols := sh[0], sh[1]
			mat := make([][]float64, rows)
			flat := make([]float64, rows*cols)
			for r := range rows {
				mat[r] = make([]float64, cols)
				for c := range cols {
					// Non-dyadic entries (0.1/0.3 + trigonometric terms) force ULP dual slack.
					v := (rng.Float64()-0.5)*50.0 + (0.1/0.3)*float64((r+2*c)%7) + math.Sin(float64(r*19+c*13+idx))
					mat[r][c] = v
					flat[r*cols+c] = v
				}
			}

			k := min(rows, cols)
			ws := AcquireWorkspace(rows, cols)
			defer ReleaseWorkspace(ws)
			rowInd := make([]int, k)
			colInd := make([]int, k)

			for _, sense := range []Sense{Minimize, Maximize} {
				for _, simdOpt := range []Option{WithSIMD(true), WithoutSIMD()} {
					ref, err := LinearSum(ctx, mat, sense, simdOpt)
					if err != nil {
						t.Fatalf("ref LinearSum(%dx%d, %v): %v", rows, cols, sense, err)
					}

					cert2D, err := LinearSum(ctx, mat, sense, simdOpt, WithCertified(true))
					if err != nil {
						t.Fatalf("certified LinearSum(%dx%d, %v): %v", rows, cols, sense, err)
					}
					if !isAdjacentFloat64(cert2D.Objective, ref.Objective) {
						t.Fatalf("LinearSum(%dx%d, %v) obj=%.17g vs ref=%.17g", rows, cols, sense, cert2D.Objective, ref.Objective)
					}

					certFlat, err := LinearSumFlat(ctx, rows, cols, flat, sense, simdOpt, WithCertified(true))
					if err != nil {
						t.Fatalf("certified LinearSumFlat(%dx%d, %v): %v", rows, cols, sense, err)
					}
					if certFlat.Objective != cert2D.Objective {
						t.Fatalf("LinearSumFlat obj=%.17g != LinearSum obj=%.17g", certFlat.Objective, cert2D.Objective)
					}

					objInto, err := SolveInto(ctx, rows, cols, flat, sense, rowInd, colInd, ws, simdOpt, WithCertified(true))
					if err != nil {
						t.Fatalf("certified SolveInto(%dx%d, %v): %v", rows, cols, sense, err)
					}
					if objInto != cert2D.Objective {
						t.Fatalf("SolveInto obj=%.17g != LinearSum obj=%.17g", objInto, cert2D.Objective)
					}
				}
			}
		}
	})

	t.Run("ForbiddenEdgesFeasibleAndInfeasible", func(t *testing.T) {
		t.Parallel()
		pInf := math.Inf(1)
		nInf := math.Inf(-1)

		// Feasible square, wide, and tall matrices with forbidden edges and disconnected components.
		feasibleMin := [][][]float64{
			{
				{pInf, 0.1 / 0.3, pInf},
				{0.7 / 0.9, pInf, 0.2 / 0.7},
				{1.1 / 0.3, pInf, pInf},
			},
			{
				{pInf, 4.2, pInf, 1.1, pInf},
				{2.3, pInf, pInf, pInf, 0.9},
				{pInf, pInf, 3.7, pInf, 5.1},
			},
			{
				{pInf, 4.2, pInf},
				{2.3, pInf, pInf},
				{pInf, pInf, 3.7},
				{1.1, pInf, pInf},
				{pInf, 0.9, 5.1},
			},
		}
		for idx, matMin := range feasibleMin {
			rows, cols := len(matMin), len(matMin[0])
			flatMin := make([]float64, rows*cols)
			matMax := make([][]float64, rows)
			flatMax := make([]float64, rows*cols)
			for r := range rows {
				matMax[r] = make([]float64, cols)
				for c := range cols {
					v := matMin[r][c]
					flatMin[r*cols+c] = v
					if math.IsInf(v, 1) {
						matMax[r][c] = nInf
						flatMax[r*cols+c] = nInf
					} else {
						matMax[r][c] = -v
						flatMax[r*cols+c] = -v
					}
				}
			}
			k := min(rows, cols)
			rInd := make([]int, k)
			cInd := make([]int, k)

			resMin, err := LinearSum(ctx, matMin, Minimize, WithCertified(true))
			if err != nil {
				t.Fatalf("feasibleMin[%d] LinearSum: %v", idx, err)
			}
			if _, err := LinearSumFlat(ctx, rows, cols, flatMin, Minimize, WithCertified(true)); err != nil {
				t.Fatalf("feasibleMin[%d] LinearSumFlat: %v", idx, err)
			}
			if obj, err := SolveInto(ctx, rows, cols, flatMin, Minimize, rInd, cInd, nil, WithCertified(true)); err != nil || obj != resMin.Objective {
				t.Fatalf("feasibleMin[%d] SolveInto: obj=%v want=%v err=%v", idx, obj, resMin.Objective, err)
			}

			resMax, err := LinearSum(ctx, matMax, Maximize, WithCertified(true))
			if err != nil {
				t.Fatalf("feasibleMax[%d] LinearSum: %v", idx, err)
			}
			if resMax.Objective != -resMin.Objective {
				t.Fatalf("feasibleMax[%d] obj=%v, want %v", idx, resMax.Objective, -resMin.Objective)
			}
			if obj, err := SolveInto(ctx, rows, cols, flatMax, Maximize, rInd, cInd, nil, WithCertified(true)); err != nil || obj != resMax.Objective {
				t.Fatalf("feasibleMax[%d] SolveInto: obj=%v want=%v err=%v", idx, obj, resMax.Objective, err)
			}
		}

		// Genuinely infeasible matrices with non-trivial Hall witness |S| = 3 > |N(S)| = 2 (square, wide, tall).
		infeasMin := [][][]float64{
			{
				{1, 2, pInf, pInf},
				{3, 4, pInf, pInf},
				{5, 6, pInf, pInf},
				{7, 8, 9, 10},
			},
			{
				{1, pInf, 2, pInf, pInf},
				{3, pInf, 4, pInf, pInf},
				{5, pInf, 6, pInf, pInf},
			},
			{
				{1, 3, 5},
				{pInf, pInf, pInf},
				{2, 4, 6},
				{pInf, pInf, pInf},
				{pInf, pInf, pInf},
			},
		}
		for idx, matMin := range infeasMin {
			rows, cols := len(matMin), len(matMin[0])
			flatMin := make([]float64, rows*cols)
			matMax := make([][]float64, rows)
			flatMax := make([]float64, rows*cols)
			for r := range rows {
				matMax[r] = make([]float64, cols)
				for c := range cols {
					v := matMin[r][c]
					flatMin[r*cols+c] = v
					if math.IsInf(v, 1) {
						matMax[r][c] = nInf
						flatMax[r*cols+c] = nInf
					} else {
						matMax[r][c] = -v
						flatMax[r*cols+c] = -v
					}
				}
			}
			k := min(rows, cols)
			rInd := make([]int, k)
			cInd := make([]int, k)

			if _, err := LinearSum(ctx, matMin, Minimize, WithCertified(true)); !errors.Is(err, ErrInfeasible) {
				t.Fatalf("infeasMin[%d] LinearSum: got %v, want ErrInfeasible", idx, err)
			}
			if _, err := LinearSumFlat(ctx, rows, cols, flatMin, Minimize, WithCertified(true)); !errors.Is(err, ErrInfeasible) {
				t.Fatalf("infeasMin[%d] LinearSumFlat: got %v, want ErrInfeasible", idx, err)
			}
			if _, err := SolveInto(ctx, rows, cols, flatMin, Minimize, rInd, cInd, nil, WithCertified(true)); !errors.Is(err, ErrInfeasible) {
				t.Fatalf("infeasMin[%d] SolveInto: got %v, want ErrInfeasible", idx, err)
			}
			if _, err := LinearSum(ctx, matMax, Maximize, WithCertified(true)); !errors.Is(err, ErrInfeasible) {
				t.Fatalf("infeasMax[%d] LinearSum: got %v, want ErrInfeasible", idx, err)
			}
			if _, err := SolveInto(ctx, rows, cols, flatMax, Maximize, rInd, cInd, nil, WithCertified(true)); !errors.Is(err, ErrInfeasible) {
				t.Fatalf("infeasMax[%d] SolveInto: got %v, want ErrInfeasible", idx, err)
			}
		}
	})

	t.Run("HugeDownscaledAndOverflowObjective", func(t *testing.T) {
		t.Parallel()
		// Costs > 1e290 trigger internal downscaling and clamped float duals, exercising Bellman-Ford over Q.
		mat := [][]float64{
			{1e308, 1.5e308},
			{1.5e308, 1e308},
		}
		res, err := LinearSum(ctx, mat, Minimize, WithCertified(true))
		if err != nil {
			t.Fatalf("huge overflow LinearSum: %v", err)
		}
		if !math.IsInf(res.Objective, 1) {
			t.Fatalf("expected +Inf overflow objective, got %v", res.Objective)
		}

		// Reverse-topological strictly lower-triangular DAGs (n = 7 and n = 12) where
		// acyclic SPFA relaxes vertices O(n^2) times while re-enqueueing each < n times.
		for _, n := range []int{7, 12} {
			dagMin := make([][]float64, n)
			dagMax := make([][]float64, n)
			flatMin := make([]float64, n*n)
			flatMax := make([]float64, n*n)
			for i := range n {
				dagMin[i] = make([]float64, n)
				dagMax[i] = make([]float64, n)
				for j := range n {
					var v float64
					switch {
					case i == j:
						v = 0
					case i < j:
						v = math.Inf(1)
					case j == 0:
						v = -float64(i) * 2e291
					case j == i-1:
						v = -math.Pow10(i) * 2e291
					default:
						v = -float64(i+j) * 1e290
					}
					dagMin[i][j] = v
					flatMin[i*n+j] = v
					if math.IsInf(v, 1) {
						dagMax[i][j] = math.Inf(-1)
						flatMax[i*n+j] = math.Inf(-1)
					} else {
						dagMax[i][j] = -v
						flatMax[i*n+j] = -v
					}
				}
			}
			rInd := make([]int, n)
			cInd := make([]int, n)
			resDAGMin, err := LinearSum(ctx, dagMin, Minimize, WithCertified(true))
			if err != nil || resDAGMin.Objective != 0 {
				t.Fatalf("dagMin(n=%d) LinearSum: obj=%v err=%v", n, resDAGMin.Objective, err)
			}
			if _, err := LinearSumFlat(ctx, n, n, flatMin, Minimize, WithCertified(true)); err != nil {
				t.Fatalf("dagMin(n=%d) LinearSumFlat: %v", n, err)
			}
			if obj, err := SolveInto(ctx, n, n, flatMin, Minimize, rInd, cInd, nil, WithCertified(true)); err != nil || obj != 0 {
				t.Fatalf("dagMin(n=%d) SolveInto: obj=%v err=%v", n, obj, err)
			}
			resDAGMax, err := LinearSum(ctx, dagMax, Maximize, WithCertified(true))
			if err != nil || resDAGMax.Objective != 0 {
				t.Fatalf("dagMax(n=%d) LinearSum: obj=%v err=%v", n, resDAGMax.Objective, err)
			}
			if obj, err := SolveInto(ctx, n, n, flatMax, Maximize, rInd, cInd, nil, WithCertified(true)); err != nil || obj != 0 {
				t.Fatalf("dagMax(n=%d) SolveInto: obj=%v err=%v", n, obj, err)
			}
		}
	})

	t.Run("DirectResidualFallbackOnDualULPDrift", func(t *testing.T) {
		t.Parallel()
		mat := [][]float64{
			{0.1 / 0.3, 0.7 / 0.9, 1.2, 0.8},
			{0.5 / 0.7, 0.2 / 0.3, 0.9, 1.5},
			{1.1 / 0.7, 0.4 / 0.3, 0.3, 0.6},
		}
		// Perturb V by 2 ULPs so fast exact reduced-cost check fails while approximate dual check passes,
		// forcing verifyExactResidualOptimality to prove optimality over Q and write back repaired U/V.
		faultCtx := withAssignCertFault(ctx, func(_, _ int, _ Sense, res *Result, err *error) {
			if *err == nil && len(res.V) > 0 {
				for j := range res.V {
					if res.V[j] != 0 {
						res.V[j] = math.Nextafter(math.Nextafter(res.V[j], math.Inf(1)), math.Inf(1))
						if res.V[j] > 0 {
							res.V[j] = 0
						}
					}
				}
			}
		})
		res, err := LinearSum(faultCtx, mat, Minimize, WithCertified(true))
		if err != nil {
			t.Fatalf("expected residual Bellman-Ford fallback to certify optimal matching under 2-ULP V drift, got %v", err)
		}
		// Repaired float64 U/V must be strictly dual-feasible over Q with valid rectangular signs.
		matchedCol := make([]bool, 4)
		for _, c := range res.ColInd {
			matchedCol[c] = true
		}
		for j := range 4 {
			if res.V[j] > 0 || (!matchedCol[j] && res.V[j] != 0) {
				t.Fatalf("repaired V[%d]=%g violates rectangular invariant (matched=%v)", j, res.V[j], matchedCol[j])
			}
		}
		for i := range 3 {
			ui := new(big.Rat).SetFloat64(res.U[i])
			for j := range 4 {
				vj := new(big.Rat).SetFloat64(res.V[j])
				cij := new(big.Rat).SetFloat64(mat[i][j])
				if new(big.Rat).Add(ui, vj).Cmp(cij) > 0 {
					t.Fatalf("repaired dual infeasible at (%d,%d): U=%g V=%g C=%g", i, j, res.U[i], res.V[j], mat[i][j])
				}
			}
		}
	})
}

func TestAssignSolveIntoZeroAllocsWhenCertifiedDisabled(t *testing.T) {
	n := 16
	cost := make([]float64, n*n)
	for i := range cost {
		cost[i] = float64((i*37+11)%97) * 0.25
	}
	rowInd := make([]int, n)
	colInd := make([]int, n)
	ws := AcquireWorkspace(n, n)
	defer ReleaseWorkspace(ws)
	bg := t.Context()

	if _, err := SolveInto(bg, n, n, cost, Minimize, rowInd, colInd, ws); err != nil {
		t.Fatalf("warmup SolveInto failed: %v", err)
	}

	allocsDefault := testing.AllocsPerRun(20, func() {
		if _, err := SolveInto(bg, n, n, cost, Minimize, rowInd, colInd, ws); err != nil {
			t.Fatal(err)
		}
	})
	if allocsDefault != 0 {
		t.Fatalf("SolveInto default allocs = %v, want 0", allocsDefault)
	}

	allocsCertOff := testing.AllocsPerRun(20, func() {
		if _, err := SolveInto(bg, n, n, cost, Minimize, rowInd, colInd, ws, WithCertified(false)); err != nil {
			t.Fatal(err)
		}
	})
	if allocsCertOff != 0 {
		t.Fatalf("SolveInto WithCertified(false) allocs = %v, want 0", allocsCertOff)
	}

	allocsApplyEmpty := testing.AllocsPerRun(20, func() {
		cfg := ApplyOptions()
		if cfg.Certified {
			t.Fatal("unexpected Certified=true")
		}
	})
	if allocsApplyEmpty != 0 {
		t.Fatalf("ApplyOptions() allocs = %v, want 0", allocsApplyEmpty)
	}
}

func TestAssignCertifiedNegativeControlsFailClosed(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	assertAllEntrypointsFailClosed := func(t *testing.T, name string, mat [][]float64, sense Sense, fault assignFaultFunc) {
		t.Helper()
		rows := len(mat)
		cols := 0
		if rows > 0 {
			cols = len(mat[0])
		}
		flat := make([]float64, rows*cols)
		for r := range rows {
			copy(flat[r*cols:(r+1)*cols], mat[r])
		}
		fCtx := withAssignCertFault(ctx, fault)

		if _, err := LinearSum(fCtx, mat, sense, WithCertified(true)); !errors.Is(err, ErrCertificationFailed) {
			t.Fatalf("%s: LinearSum got err=%v, want ErrCertificationFailed", name, err)
		}
		if _, err := LinearSumFlat(fCtx, rows, cols, flat, sense, WithCertified(true)); !errors.Is(err, ErrCertificationFailed) {
			t.Fatalf("%s: LinearSumFlat got err=%v, want ErrCertificationFailed", name, err)
		}
		k := min(rows, cols)
		rInd := make([]int, max(1, k))
		cInd := make([]int, max(1, k))
		obj, err := SolveInto(fCtx, rows, cols, flat, sense, rInd[:k], cInd[:k], nil, WithCertified(true))
		if !errors.Is(err, ErrCertificationFailed) || obj != 0 {
			t.Fatalf("%s: SolveInto got (obj=%v, err=%v), want (0, ErrCertificationFailed)", name, obj, err)
		}
		for i := range k {
			if rInd[i] != -1 || cInd[i] != -1 {
				t.Fatalf("%s: SolveInto did not sanitize indices at %d: (%d, %d)", name, i, rInd[i], cInd[i])
			}
		}
	}

	// 1. 1-ULP suboptimal matching on a forbidden-edge matrix (even with exact sum).
	pInf := math.Inf(1)
	qBase := 0.25
	qPlusULP := math.Nextafter(qBase, 1.0)
	ulp025 := qPlusULP - qBase
	matULP := [][]float64{
		{qBase, qPlusULP, pInf},
		{qBase, qBase, 5.0},
		{pInf, 5.0, qBase},
	}
	assertAllEntrypointsFailClosed(t, "1ulp_suboptimal_matching", matULP, Minimize, func(_, _ int, _ Sense, res *Result, _ *error) {
		// Swap rows 0 and 1: matched edges (0,1)=0.25+ulp, (1,0)=0.25, (2,2)=0.25 -> exact sum 0.75+ulp > optimal 0.75.
		res.ColInd[0], res.ColInd[1] = 1, 0
		res.Objective = 0.75 + ulp025
		res.U = []float64{qPlusULP, qBase, qBase}
		res.V = []float64{0, 0, 0}
	})
	// Also test pure residual graph negative-cycle detection directly without dual vector precheck.
	if _, err := certifyAssignCore(
		ctx, 3, 3,
		func(r, c int) float64 { return matULP[r][c] },
		Minimize,
		[]int{0, 1, 2}, []int{1, 0, 2},
		nil, nil, false,
		0.75+ulp025, nil,
	); !errors.Is(err, ErrCertificationFailed) {
		t.Fatalf("direct residual negative-cycle check got %v, want ErrCertificationFailed", err)
	}

	// 2. Suboptimal rectangular augmenting path to an unmatched column (1 ULP above optimal).
	c2PlusULP := math.Nextafter(0.5, 1.0)
	matWide := [][]float64{
		{10.0, 0.5, c2PlusULP},
		{0.25, 9.0, 8.0},
	}
	if _, err := certifyAssignCore(
		ctx, 2, 3,
		func(r, c int) float64 { return matWide[r][c] },
		Minimize,
		[]int{0, 1}, []int{2, 0},
		nil, nil, false,
		0.25+c2PlusULP, nil,
	); !errors.Is(err, ErrCertificationFailed) {
		t.Fatalf("wide suboptimal augmenting-path check got %v, want ErrCertificationFailed", err)
	}

	// 3. Duplicate column (non-injective matching).
	base3 := [][]float64{{4, 1, 3}, {2, 0, 5}, {3, 2, 2}}
	assertAllEntrypointsFailClosed(t, "duplicate_col", base3, Minimize, func(_, _ int, _ Sense, res *Result, _ *error) {
		res.ColInd[1] = res.ColInd[0]
		res.Objective = base3[0][res.ColInd[0]] + base3[1][res.ColInd[0]] + base3[2][res.ColInd[2]]
	})

	// 4. Non-ascending / duplicate row indices.
	assertAllEntrypointsFailClosed(t, "non_ascending_rows", base3, Minimize, func(_, _ int, _ Sense, res *Result, _ *error) {
		res.RowInd[0], res.RowInd[1] = res.RowInd[1], res.RowInd[0]
		res.ColInd[0], res.ColInd[1] = res.ColInd[1], res.ColInd[0]
	})

	// 5. Matched forbidden edge (+Inf on Minimize).
	matForb := [][]float64{{1, pInf}, {2, 3}}
	assertAllEntrypointsFailClosed(t, "matched_forbidden_edge", matForb, Minimize, func(_, _ int, _ Sense, res *Result, _ *error) {
		res.ColInd[0], res.ColInd[1] = 1, 0
	})

	// 6. Corrupted Objective (by 1 ULP on exact dyadic matrix).
	assertAllEntrypointsFailClosed(t, "corrupted_objective_1ulp", base3, Minimize, func(_, _ int, _ Sense, res *Result, _ *error) {
		res.Objective = math.Nextafter(res.Objective, math.Inf(1))
	})

	// 7. Tampered dual potential U.
	assertAllEntrypointsFailClosed(t, "tampered_dual_u", base3, Maximize, func(_, _ int, _ Sense, res *Result, _ *error) {
		res.U[0] += 0.5
	})

	// 8. Tampered rectangular unmatched V != 0.
	assertAllEntrypointsFailClosed(t, "tampered_rect_dual_v", matWide, Minimize, func(_, _ int, _ Sense, res *Result, _ *error) {
		for j := range 3 {
			if j != res.ColInd[0] && j != res.ColInd[1] {
				res.V[j] = -0.25
			}
		}
	})

	// 9. False ErrInfeasible on a feasible forbidden-edge matrix (square, wide, tall).
	feasibleForb := [][][]float64{
		{{pInf, 1, pInf}, {2, pInf, 3}, {pInf, pInf, 4}}, // wait: row 0 -> col 1, row 2 -> col 2, row 1 -> col 0: feasible!
		{{pInf, 2, pInf, 5}, {3, pInf, pInf, 1}},
		{{pInf, 3}, {2, pInf}, {pInf, 4}},
	}
	for idx, mFeas := range feasibleForb {
		assertAllEntrypointsFailClosed(t, "false_infeasible_"+string(rune('0'+idx)), mFeas, Minimize, func(_, _ int, _ Sense, _ *Result, err *error) {
			*err = ErrInfeasible
		})
	}

	// 10. False ErrInvalidCost on a valid matrix.
	assertAllEntrypointsFailClosed(t, "false_invalid_cost", base3, Minimize, func(_, _ int, _ Sense, _ *Result, err *error) {
		*err = ErrInvalidCost
	})
}

func TestAssignStructLiteralCertifiedWithoutHookFailsClosed(t *testing.T) {
	ctx := t.Context()

	rawCertOpt := func(cfg *Options) {
		cfg.Certified = true
	}

	mat := [][]float64{{1, 2}, {3, 1}}
	flat := []float64{1, 2, 3, 1}
	if _, err := LinearSum(ctx, mat, Minimize, rawCertOpt); !errors.Is(err, ErrCertificationFailed) {
		t.Fatalf("LinearSum(2x2) err = %v, want ErrCertificationFailed", err)
	}
	if _, err := LinearSum(ctx, nil, Minimize, rawCertOpt); !errors.Is(err, ErrCertificationFailed) {
		t.Fatalf("LinearSum(0x0) err = %v, want ErrCertificationFailed", err)
	}
	if _, err := LinearSum(ctx, [][]float64{{}, {}}, Minimize, rawCertOpt); !errors.Is(err, ErrCertificationFailed) {
		t.Fatalf("LinearSum(2x0) err = %v, want ErrCertificationFailed", err)
	}
	if _, err := LinearSumFlat(ctx, 2, 2, flat, Minimize, rawCertOpt); !errors.Is(err, ErrCertificationFailed) {
		t.Fatalf("LinearSumFlat(2x2) err = %v, want ErrCertificationFailed", err)
	}
	if _, err := LinearSumFlat(ctx, 0, 3, nil, Minimize, rawCertOpt); !errors.Is(err, ErrCertificationFailed) {
		t.Fatalf("LinearSumFlat(0x3) err = %v, want ErrCertificationFailed", err)
	}

	rowInd := []int{9, 9}
	colInd := []int{9, 9}
	ws := AcquireWorkspace(2, 2)
	defer ReleaseWorkspace(ws)
	if _, err := SolveInto(ctx, 2, 2, flat, Minimize, rowInd, colInd, ws, rawCertOpt); !errors.Is(err, ErrCertificationFailed) {
		t.Fatalf("SolveInto(2x2) err = %v, want ErrCertificationFailed", err)
	}
	if rowInd[0] != -1 || rowInd[1] != -1 || colInd[0] != -1 || colInd[1] != -1 {
		t.Fatalf("SolveInto fail-closed did not scrub output slices: row=%v col=%v", rowInd, colInd)
	}
	if _, err := SolveInto(ctx, 0, 2, nil, Minimize, nil, nil, ws, rawCertOpt); !errors.Is(err, ErrCertificationFailed) {
		t.Fatalf("SolveInto(0x2) err = %v, want ErrCertificationFailed", err)
	}

	// Chaining WithCertified(true) after rawCertOpt installs the hooks and succeeds.
	resOK, errOK := LinearSum(ctx, mat, Minimize, rawCertOpt, WithCertified(true))
	if errOK != nil || resOK.Objective != 2 {
		t.Fatalf("LinearSum with WithCertified(true) = (%v, %v), want obj=2", resOK.Objective, errOK)
	}

	// Default SolveInto must remain 0 B/op, 0 allocs/op.
	allocs := testing.AllocsPerRun(50, func() {
		obj, err := SolveInto(ctx, 2, 2, flat, Minimize, rowInd, colInd, ws)
		if err != nil || obj != 2 {
			t.Fatalf("SolveInto default = (%v, %v)", obj, err)
		}
	})
	if allocs != 0 {
		t.Fatalf("SolveInto default allocs = %v, want 0", allocs)
	}
}

func TestDyadic2304EquivalenceWithBigRat(t *testing.T) {
	t.Parallel()

	samples := []float64{
		0,
		math.Copysign(0, -1),
		math.SmallestNonzeroFloat64,
		-math.SmallestNonzeroFloat64,
		math.Float64frombits((uint64(1) << 52) - 1),
		-math.Float64frombits((uint64(1) << 52) - 1),
		math.Float64frombits(uint64(1) << 52),
		-math.Float64frombits(uint64(1) << 52),
		1.0,
		-1.0,
		0.1,
		-0.3,
		math.Nextafter(1.0, 2.0) - 1.0,
		0.5 * (math.Nextafter(1.0, 2.0) - 1.0),
		1.5 * (math.Nextafter(1.0, 2.0) - 1.0),
		1e308,
		-1e308,
		1e-308,
		-1e-308,
		math.MaxFloat64,
		-math.MaxFloat64,
		0.25 * (math.MaxFloat64 - math.Nextafter(math.MaxFloat64, 0)),
		0.5 * (math.MaxFloat64 - math.Nextafter(math.MaxFloat64, 0)),
	}

	checkVal := func(d dyadic2304, r *big.Rat) {
		t.Helper()
		if d.sign() != r.Sign() {
			t.Fatalf("sign mismatch: dyadic=%d rat=%d", d.sign(), r.Sign())
		}
		df, dExact := d.toFloat64()
		rf, rExact := r.Float64()
		if df == 0 {
			df = 0
		}
		if rf == 0 {
			rf = 0
		}
		if math.Float64bits(df) != math.Float64bits(rf) || dExact != rExact {
			t.Fatalf("toFloat64 mismatch: dyadic=(%.17g, %v) rat=(%.17g, %v)", df, dExact, rf, rExact)
		}
	}

	for _, a := range samples {
		da, okA := dyadicFromFloat(a)
		if !okA {
			t.Fatalf("dyadicFromFloat(%v) failed", a)
		}
		ra := new(big.Rat).SetFloat64(a)
		checkVal(da, ra)
		for _, b := range samples {
			db, _ := dyadicFromFloat(b)
			rb := new(big.Rat).SetFloat64(b)
			if dyadicCmp(da, db) != ra.Cmp(rb) {
				t.Fatalf("cmp(%v, %v): dyadic=%d rat=%d", a, b, dyadicCmp(da, db), ra.Cmp(rb))
			}
			checkVal(dyadicAdd(da, db), new(big.Rat).Add(ra, rb))
			checkVal(dyadicSub(da, db), new(big.Rat).Sub(ra, rb))
		}
	}

	// Catastrophic cancellation + tiny sticky bit round-to-nearest-even check:
	// (1.0 + 0.5*ulp(1.0) + SmallestNonzeroFloat64) must round UP past the halfway tie.
	dOne, _ := dyadicFromFloat(1.0)
	dHalfULP, _ := dyadicFromFloat(math.Ldexp(1.0, -53))
	dTiny, _ := dyadicFromFloat(math.SmallestNonzeroFloat64)
	rOne := new(big.Rat).SetFloat64(1.0)
	rHalfULP := new(big.Rat).SetFloat64(math.Ldexp(1.0, -53))
	rTiny := new(big.Rat).SetFloat64(math.SmallestNonzeroFloat64)
	checkVal(dyadicAdd(dyadicAdd(dOne, dHalfULP), dTiny), new(big.Rat).Add(new(big.Rat).Add(rOne, rHalfULP), rTiny))
	checkVal(dyadicAdd(dOne, dHalfULP), new(big.Rat).Add(rOne, rHalfULP))
}

func TestAssignBinaryDeadCodeElimination(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	tmp := t.TempDir()
	mainPath := filepath.Join(tmp, "main.go")
	binPath := filepath.Join(tmp, "assign_default_bin")
	src := `package main

import (
	"context"
	"fmt"
	as "github.com/paulourio/goblin/assign"
)

func main() {
	res, err := as.LinearSum(context.Background(), [][]float64{{1, 2}, {3, 1}}, as.Minimize)
	fmt.Println(res.Objective, err)
}
`
	if err := os.WriteFile(mainPath, []byte(src), 0o600); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", binPath, mainPath)
	buildCmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}

	runOut, err := exec.CommandContext(ctx, binPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run built binary: %v\n%s", err, runOut)
	}
	if got := strings.TrimSpace(string(runOut)); got != "2 <nil>" {
		t.Fatalf("binary output = %q, want %q", got, "2 <nil>")
	}

	nmOut, err := exec.CommandContext(ctx, "go", "tool", "nm", "-size", binPath).CombinedOutput()
	if err != nil {
		t.Fatalf("go tool nm failed: %v\n%s", err, nmOut)
	}

	forbidden := regexp.MustCompile(`(certifyLinearSum|certifySolveInto|bellmanFordResidual|extractHallWitness|dyadic2304|dyadicFromFloat|math/big)`)
	var leaked []string
	for line := range strings.SplitSeq(string(nmOut), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		sym := fields[len(fields)-1]
		if forbidden.MatchString(sym) {
			leaked = append(leaked, sym)
		}
	}
	if len(leaked) > 0 {
		t.Fatalf("unreferenced assign certification symbols linked into default binary (%d): %v", len(leaked), leaked[:min(len(leaked), 10)])
	}
}
