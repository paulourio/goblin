package assign_test

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"testing"

	gb "github.com/paulourio/goblin"
	"github.com/paulourio/goblin/assign"
)

// generateFuzzMatrix synthesizes square, wide, and tall cost matrices across seven distinct weight families,
// including uniform continuous values, small integer ties, binary costs, 2D Euclidean distances, rank-1 outer
// products, multi-scale rows, and sparse forbidden-edge topologies.
func generateFuzzMatrix(rng *rand.Rand, family int) (rows, cols int, cost []float64, sense assign.Sense) {
	switch rng.IntN(4) {
	case 0:
		// Use square dimensions.
		rows = 1 + rng.IntN(45)
		cols = rows
	case 1:
		// Use wide dimensions.
		rows = 1 + rng.IntN(30)
		cols = rows + 1 + rng.IntN(35)
	case 2:
		// Use tall dimensions.
		cols = 1 + rng.IntN(30)
		rows = cols + 1 + rng.IntN(35)
	default:
		// Cross the n = 256 AVX2 threshold on a subset of trials to exercise both scalar and vector paths.
		if rng.IntN(6) == 0 {
			rows = 64 + rng.IntN(64)
			cols = 256 + rng.IntN(20)
		} else {
			rows = 2 + rng.IntN(25)
			cols = 2 + rng.IntN(25)
		}
	}

	sense = assign.Minimize
	if rng.IntN(2) == 1 {
		sense = assign.Maximize
	}

	cost = make([]float64, rows*cols)
	switch family % 7 {
	case 0:
		// Sample uniform continuous weights in [-1000, 1000].
		for i := range cost {
			cost[i] = (rng.Float64() - 0.5) * 2000.0
		}
	case 1:
		// Sample small-range integer ties in {0, 1, 2, 3, 4}.
		for i := range cost {
			cost[i] = float64(rng.IntN(5))
		}
	case 2:
		// Sample binary {0, 1} high-degeneracy costs.
		for i := range cost {
			cost[i] = float64(rng.IntN(2))
		}
	case 3:
		// Compute 2D Euclidean squared distances ||p_r - q_c||^2.
		px := make([]float64, rows)
		py := make([]float64, rows)
		qx := make([]float64, cols)
		qy := make([]float64, cols)
		for r := range rows {
			px[r] = rng.Float64() * 100.0
			py[r] = rng.Float64() * 100.0
		}
		for c := range cols {
			qx[c] = rng.Float64() * 100.0
			qy[c] = rng.Float64() * 100.0
		}
		for r := range rows {
			for c := range cols {
				dx := px[r] - qx[c]
				dy := py[r] - qy[c]
				cost[r*cols+c] = math.Round((dx*dx+dy*dy)*1e3) / 1e3
			}
		}
	case 4:
		// Construct a pathological rank-1 outer product where C[r, c] = a[r] * b[c].
		a := make([]float64, rows)
		b := make([]float64, cols)
		for r := range rows {
			a[r] = float64(1 + rng.IntN(50))
		}
		for c := range cols {
			b[c] = float64(1 + rng.IntN(50))
		}
		for r := range rows {
			for c := range cols {
				cost[r*cols+c] = a[r] * b[c]
			}
		}
	case 5:
		// Scale rows across orders of magnitude spanning 1e-4 to 1e6.
		exps := []float64{1e-4, 1e-2, 1.0, 1e2, 1e4, 1e6}
		for r := range rows {
			scale := exps[rng.IntN(len(exps))]
			for c := range cols {
				cost[r*cols+c] = math.Round((rng.Float64()-0.3)*scale*1e4) / 1e4
			}
		}
	case 6:
		// Inject sparse forbidden edges (+Inf for Minimize, -Inf for Maximize).
		forbid := math.Inf(1)
		if sense == assign.Maximize {
			forbid = math.Inf(-1)
		}
		for i := range cost {
			if rng.Float64() < 0.35 {
				cost[i] = forbid
			} else {
				cost[i] = float64(rng.IntN(200) - 50)
			}
		}
	}
	return rows, cols, cost, sense
}

func TestRandomFuzzAssignment(t *testing.T) {
	rng := rand.New(rand.NewPCG(20261023, 9918273))
	const numInstances = 420

	for idx := range numInstances {
		rows, cols, cost, sense := generateFuzzMatrix(rng, idx)
		label := fmt.Sprintf("fuzz_%d_fam%d_%dx%d_%s", idx, idx%7, rows, cols, sense)

		resSIMD, errSIMD := assign.LinearSumFlat(t.Context(), rows, cols, cost, sense, assign.WithSIMD(true))
		resScalar, errScalar := assign.LinearSumFlat(t.Context(), rows, cols, cost, sense, assign.WithoutSIMD())

		if (errSIMD != nil) != (errScalar != nil) {
			t.Fatalf("[%s] error mismatch between SIMD (%v) and Scalar (%v)", label, errSIMD, errScalar)
		}
		if errSIMD != nil {
			if !errors.Is(errSIMD, assign.ErrInfeasible) {
				t.Fatalf("[%s] unexpected error: %v", label, errSIMD)
			}
			// Confirm via exact bipartite matching that no complete finite matching exists.
			if hasCompleteFiniteMatching(rows, cols, cost) {
				t.Fatalf("[%s] solver reported ErrInfeasible, but a complete finite matching exists", label)
			}
			// Cross-check small and medium infeasible instances against the exact oracle and MILP solver.
			if max(rows, cols) <= 6 {
				if _, ok := exactAssignmentObjective(rows, cols, cost, sense); ok {
					t.Fatalf("[%s] solver reported ErrInfeasible, but exact oracle found a valid matching", label)
				}
			} else if rows*cols <= 100 {
				if _, st := solveViaGoblinMILP(rows, cols, cost, sense); st == gb.StatusOptimal {
					t.Fatalf("[%s] solver reported ErrInfeasible, but MILP solver found Optimal", label)
				}
			}
			continue
		}

		// Verify primal-dual certificates for both SIMD and Scalar solutions.
		verifyOptimalityCertificate(t, label+"/SIMD", rows, cols, cost, sense, resSIMD)
		verifyOptimalityCertificate(t, label+"/Scalar", rows, cols, cost, sense, resScalar)

		tol := 1e-9 * max(1.0, math.Abs(resScalar.Objective))
		if math.Abs(resSIMD.Objective-resScalar.Objective) > tol {
			t.Fatalf("[%s] SIMD obj %.12g != Scalar obj %.12g", label, resSIMD.Objective, resScalar.Objective)
		}

		// Verify sense symmetry so Minimize(C) equals -Maximize(-C).
		negCost := make([]float64, len(cost))
		for i, v := range cost {
			negCost[i] = -v
		}
		oppSense := assign.Maximize
		if sense == assign.Maximize {
			oppSense = assign.Minimize
		}
		resNeg, errNeg := assign.LinearSumFlat(t.Context(), rows, cols, negCost, oppSense)
		if errNeg != nil {
			t.Fatalf("[%s] negated problem failed: %v", label, errNeg)
		}
		verifyOptimalityCertificate(t, label+"/Negated", rows, cols, negCost, oppSense, resNeg)
		if math.Abs(resSIMD.Objective+resNeg.Objective) > tol {
			t.Fatalf("[%s] sense duality failed: obj=%.12g vs -negObj=%.12g", label, resSIMD.Objective, -resNeg.Objective)
		}

		// Verify transpose symmetry so Solve(C) equals Solve(C^T).
		transCost := make([]float64, rows*cols)
		for r := range rows {
			for c := range cols {
				transCost[c*rows+r] = cost[r*cols+c]
			}
		}
		resTrans, errTrans := assign.LinearSumFlat(t.Context(), cols, rows, transCost, sense)
		if errTrans != nil {
			t.Fatalf("[%s] transposed problem failed: %v", label, errTrans)
		}
		verifyOptimalityCertificate(t, label+"/Transposed", cols, rows, transCost, sense, resTrans)
		if math.Abs(resSIMD.Objective-resTrans.Objective) > tol {
			t.Fatalf("[%s] transpose symmetry failed: obj=%.12g vs transObj=%.12g", label, resSIMD.Objective, resTrans.Objective)
		}

		// Cross-check against exhaustive search on small instances and Goblin MILP on medium instances.
		if max(rows, cols) <= 6 {
			wantExact, ok := exactAssignmentObjective(rows, cols, cost, sense)
			if !ok {
				t.Fatalf("[%s] exact oracle reported infeasible on solved instance", label)
			}
			if math.Abs(resSIMD.Objective-wantExact) > tol {
				t.Fatalf("[%s] exact oracle mismatch: got %.12g, want %.12g", label, resSIMD.Objective, wantExact)
			}
		} else if idx%8 == 0 && rows*cols <= 144 {
			wantMILP, st := solveViaGoblinMILP(rows, cols, cost, sense)
			if st != gb.StatusOptimal {
				t.Fatalf("[%s] MILP oracle returned status %v on solved instance", label, st)
			}
			if math.Abs(resSIMD.Objective-wantMILP) > 1e-5*max(1.0, math.Abs(wantMILP)) {
				t.Fatalf("[%s] MILP oracle mismatch: got %.12g, want %.12g", label, resSIMD.Objective, wantMILP)
			}
		}
	}
}

func FuzzLinearSum(f *testing.F) {
	f.Add(uint8(4), uint8(4), true, []byte{4, 1, 3, 2, 2, 0, 5, 1, 3, 2, 2, 6, 1, 4, 2, 3})
	f.Add(uint8(3), uint8(5), false, []byte{10, 2, 8, 15, 4, 3, 9, 12, 1, 7, 6, 5, 11, 14, 3})
	f.Add(uint8(5), uint8(3), true, []byte{0, 255, 1, 255, 0, 2, 3, 4, 255, 1, 0, 5, 2, 2, 2})

	f.Fuzz(func(t *testing.T, rRaw, cRaw uint8, minimize bool, data []byte) {
		rows := int(rRaw%24) + 1
		cols := int(cRaw%24) + 1
		sense := assign.Minimize
		if !minimize {
			sense = assign.Maximize
		}
		forbid := math.Inf(1)
		if !minimize {
			forbid = math.Inf(-1)
		}

		cost := make([]float64, rows*cols)
		for i := range cost {
			if len(data) == 0 {
				cost[i] = float64((i * 7) % 13)
				continue
			}
			b := data[i%len(data)]
			if b == 255 {
				cost[i] = forbid
			} else {
				cost[i] = float64(int(b)-100) * 0.5
			}
		}

		res, err := assign.LinearSumFlat(t.Context(), rows, cols, cost, sense)
		if err != nil {
			if !errors.Is(err, assign.ErrInfeasible) {
				t.Fatalf("unexpected error: %v", err)
			}
			if hasCompleteFiniteMatching(rows, cols, cost) {
				t.Fatalf("fuzz_target reported ErrInfeasible, but a complete finite matching exists (%dx%d)", rows, cols)
			}
			return
		}
		verifyOptimalityCertificate(t, "fuzz_target", rows, cols, cost, sense, res)
	})
}

func hasCompleteFiniteMatching(rows, cols int, cost []float64) bool {
	if rows == 0 || cols == 0 {
		return true
	}
	if rows <= cols {
		matchCol := make([]int, cols)
		for j := range matchCol {
			matchCol[j] = -1
		}
		seen := make([]int, cols)
		var dfs func(r, token int) bool
		dfs = func(r, token int) bool {
			rowOff := r * cols
			for c := range cols {
				if math.IsInf(cost[rowOff+c], 0) || seen[c] == token {
					continue
				}
				seen[c] = token
				if matchCol[c] == -1 || dfs(matchCol[c], token) {
					matchCol[c] = r
					return true
				}
			}
			return false
		}
		for r := range rows {
			if !dfs(r, r+1) {
				return false
			}
		}
		return true
	}
	matchRow := make([]int, rows)
	for i := range matchRow {
		matchRow[i] = -1
	}
	seen := make([]int, rows)
	var dfs func(c, token int) bool
	dfs = func(c, token int) bool {
		for r := range rows {
			if math.IsInf(cost[r*cols+c], 0) || seen[r] == token {
				continue
			}
			seen[r] = token
			if matchRow[r] == -1 || dfs(matchRow[r], token) {
				matchRow[r] = c
				return true
			}
		}
		return false
	}
	for c := range cols {
		if !dfs(c, c+1) {
			return false
		}
	}
	return true
}

// exactInt64Hungarian computes the exact minimum-cost bipartite matching in 64-bit integer arithmetic
// without floating-point rounding so tests can verify bit-exact objective parity on integer and scaled inputs.
func exactInt64Hungarian(rows, cols int, cost []int64, forbid []bool) (int64, bool) {
	transpose := rows > cols
	nr, nc := rows, cols
	if transpose {
		nr, nc = cols, rows
	}
	const inf64 = int64(1) << 60
	a := make([]int64, (nr+1)*(nc+1))
	for i := 1; i <= nr; i++ {
		for j := 1; j <= nc; j++ {
			var idx int
			if transpose {
				idx = (j-1)*cols + (i - 1)
			} else {
				idx = (i-1)*cols + (j - 1)
			}
			if forbid != nil && forbid[idx] {
				a[i*(nc+1)+j] = inf64
			} else {
				a[i*(nc+1)+j] = cost[idx]
			}
		}
	}
	u := make([]int64, nr+1)
	v := make([]int64, nc+1)
	p := make([]int, nc+1)
	way := make([]int, nc+1)
	minv := make([]int64, nc+1)
	used := make([]bool, nc+1)

	for i := 1; i <= nr; i++ {
		p[0] = i
		j0 := 0
		for j := 1; j <= nc; j++ {
			minv[j] = inf64
			used[j] = false
		}
		for {
			used[j0] = true
			i0 := p[j0]
			delta := inf64
			j1 := 0
			for j := 1; j <= nc; j++ {
				if !used[j] {
					cur := a[i0*(nc+1)+j] - u[i0] - v[j]
					if cur < minv[j] {
						minv[j] = cur
						way[j] = j0
					}
					if minv[j] < delta {
						delta = minv[j]
						j1 = j
					}
				}
			}
			if delta >= inf64/2 {
				return 0, false
			}
			for j := 0; j <= nc; j++ {
				if used[j] {
					u[p[j]] += delta
					v[j] -= delta
				} else {
					minv[j] -= delta
				}
			}
			j0 = j1
			if p[j0] == 0 {
				break
			}
		}
		for {
			j1 := way[j0]
			p[j0] = p[j1]
			j0 = j1
			if j0 == 0 {
				break
			}
		}
	}

	var total int64
	for j := 1; j <= nc; j++ {
		if p[j] > 0 {
			c := a[p[j]*(nc+1)+j]
			if c >= inf64/2 {
				return 0, false
			}
			total += c
		}
	}
	return total, true
}

func TestExactIntegerHungarianParity(t *testing.T) {
	rng := rand.New(rand.NewPCG(777111, 222333))
	for trial := range 80 {
		n := 25 + rng.IntN(45)
		icost := make([]int64, n*n)
		fcost := make([]float64, n*n)
		forbid := make([]bool, n*n)

		switch trial % 5 {
		case 0:
			// Build an exact integer rank-1 matrix with large offsets.
			for i := range n {
				for j := range n {
					v := int64(i+1)*int64(j+1)*1000 + int64(rng.IntN(5))
					icost[i*n+j] = v
					fcost[i*n+j] = float64(v)
				}
			}
		case 1:
			// Build a row-dominated integer matrix.
			for i := range n {
				base := int64(1_000_000_000)
				if i == 0 {
					base = 0
				}
				for j := range n {
					v := base + int64(rng.IntN(1000))
					icost[i*n+j] = v
					fcost[i*n+j] = float64(v)
				}
			}
		case 2:
			// Sample a sparse matrix near the forbidden-edge threshold.
			for i := range n * n {
				if rng.Float64() < 0.72 {
					forbid[i] = true
					fcost[i] = math.Inf(1)
				} else {
					v := int64(rng.IntN(10_000) - 5000)
					icost[i] = v
					fcost[i] = float64(v)
				}
			}
		case 3:
			// Add small integer tie-breakers to a 2^40 base offset.
			base := int64(1) << 40
			for i := range n * n {
				v := base + int64(rng.IntN(200))
				icost[i] = v
				fcost[i] = float64(v)
			}
		case 4:
			// Compare non-dyadic tenths scaled by 10 in int64 against float64.
			for i := range n * n {
				v := int64(rng.IntN(100_000) - 50_000)
				icost[i] = v
				fcost[i] = float64(v) / 10.0
			}
		}

		wantInt, wantFeas := exactInt64Hungarian(n, n, icost, forbid)
		res, err := assign.LinearSumFlat(t.Context(), n, n, fcost, assign.Minimize)
		if !wantFeas {
			if !errors.Is(err, assign.ErrInfeasible) {
				t.Fatalf("trial %d: expected ErrInfeasible, got err=%v", trial, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("trial %d: unexpected err=%v when exact Hungarian found %d", trial, err, wantInt)
		}
		verifyOptimalityCertificate(t, fmt.Sprintf("int64_trial_%d", trial), n, n, fcost, assign.Minimize, res)

		if trial%5 != 4 {
			var gotInt int64
			for k, i := range res.RowInd {
				j := res.ColInd[k]
				gotInt += icost[i*n+j]
			}
			if gotInt != wantInt {
				t.Fatalf("trial %d (fam %d): integer objective mismatch: got %d, want %d", trial, trial%5, gotInt, wantInt)
			}
		} else {
			wantF := float64(wantInt) / 10.0
			if math.Abs(res.Objective-wantF) > 1e-8*max(1.0, math.Abs(wantF)) {
				t.Fatalf("trial %d: float objective mismatch: got %.15g, want %.15g", trial, res.Objective, wantF)
			}
		}
	}
}
