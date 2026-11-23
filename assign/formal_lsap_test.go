package assign

import (
	"encoding/json/v2"
	"math"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

type lsapCertPayload struct {
	Rows      int            `json:"rows"`
	Cols      int            `json:"cols"`
	Sense     string         `json:"sense"`
	Mode      string         `json:"mode"`
	Cost      [][]ratPayload `json:"cost"`
	RowInd    []int          `json:"row_ind"`
	ColInd    []int          `json:"col_ind"`
	Objective ratPayload     `json:"objective"`
	U         []ratPayload   `json:"u"`
	V         []ratPayload   `json:"v"`
	Tol       *ratPayload    `json:"tol,omitempty"`
}

func buildLSAPCertPayload(tb testing.TB, rows, cols int, cost [][]float64, sense Sense, res Result, mode string, tol float64) lsapCertPayload {
	tb.Helper()

	costRat := make([][]ratPayload, rows)
	for r := range rows {
		costRat[r] = make([]ratPayload, cols)
		for c := range cols {
			rp, err := float64ToPayload(cost[r][c])
			if err != nil {
				tb.Fatalf("cost[%d][%d]: %v", r, c, err)
			}
			costRat[r][c] = rp
		}
	}

	uRat := make([]ratPayload, len(res.U))
	for i := range res.U {
		rp, err := float64ToPayload(res.U[i])
		if err != nil {
			tb.Fatalf("U[%d]: %v", i, err)
		}
		uRat[i] = rp
	}

	vRat := make([]ratPayload, len(res.V))
	for j := range res.V {
		rp, err := float64ToPayload(res.V[j])
		if err != nil {
			tb.Fatalf("V[%d]: %v", j, err)
		}
		vRat[j] = rp
	}

	objRat, err := float64ToPayload(res.Objective)
	if err != nil {
		tb.Fatalf("Objective: %v", err)
	}

	senseStr := "min"
	if sense == Maximize {
		senseStr = "max"
	}

	payload := lsapCertPayload{
		Rows:      rows,
		Cols:      cols,
		Sense:     senseStr,
		Mode:      mode,
		Cost:      costRat,
		RowInd:    append([]int(nil), res.RowInd...),
		ColInd:    append([]int(nil), res.ColInd...),
		Objective: objRat,
		U:         uRat,
		V:         vRat,
	}
	if mode == "safe" {
		tolRat, tolErr := float64ToPayload(tol)
		if tolErr != nil {
			tb.Fatalf("tol: %v", tolErr)
		}
		payload.Tol = &tolRat
	}
	return payload
}

func runLSAPChecker(tb testing.TB, exe, dir, name string, payload lsapCertPayload, extraArgs ...string) (string, error) {
	tb.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		tb.Fatalf("marshal %s: %v", name, err)
	}
	path := filepath.Join(dir, name+".json")
	if writeErr := os.WriteFile(path, raw, 0o600); writeErr != nil {
		tb.Fatalf("write %s: %v", path, writeErr)
	}
	args := make([]string, 0, len(extraArgs)+1)
	args = append(args, extraArgs...)
	args = append(args, path)
	cmd := exec.CommandContext(tb.Context(), exe, args...)
	out, cmdErr := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), cmdErr
}

func assertLSAPAccepted(tb testing.TB, exe, dir, name string, payload lsapCertPayload, extraArgs ...string) {
	tb.Helper()
	out, err := runLSAPChecker(tb, exe, dir, name, payload, extraArgs...)
	if err != nil {
		tb.Fatalf("%s: expected VERIFIED, got err=%v\noutput:\n%s", name, err, out)
	}
	wantMode := payload.Mode
	for _, arg := range extraArgs {
		switch arg {
		case "--safe":
			wantMode = "safe"
		case "--exact":
			wantMode = "exact"
		case "--hall", "--infeasible":
			wantMode = "hall"
		}
	}
	if wantMode == "" {
		wantMode = "exact"
	}
	wantPrefix := "VERIFIED mode=" + wantMode
	if !strings.HasPrefix(out, wantPrefix) {
		tb.Fatalf("%s: expected stdout prefix %q, got %q", name, wantPrefix, out)
	}
	if wantMode == "exact" && out != "VERIFIED mode=exact tol=0/1" {
		tb.Fatalf("%s: expected exact output %q, got %q", name, "VERIFIED mode=exact tol=0/1", out)
	}
	if wantMode == "safe" && !strings.Contains(out, " tol=") {
		tb.Fatalf("%s: expected safe output to echo tol, got %q", name, out)
	}
}

func assertLSAPRejected(tb testing.TB, exe, dir, name string, payload lsapCertPayload, extraArgs ...string) {
	tb.Helper()
	out, err := runLSAPChecker(tb, exe, dir, name, payload, extraArgs...)
	if err == nil {
		tb.Fatalf("%s: expected rejection (non-zero exit), got success:\n%s", name, out)
	}
}

func TestFormalLSAPVerification(t *testing.T) {
	t.Parallel()
	requireFormalEnv(t)

	exe := formalExe(t, "lsap-check")
	tmpDir := t.TempDir()

	t.Run("Fixtures", func(t *testing.T) {
		t.Parallel()
		samplePath := filepath.Join("..", "testdata", "formal", "lsap_sample.json")
		cmdOk := exec.CommandContext(t.Context(), exe, samplePath)
		outOk, errOk := cmdOk.CombinedOutput()
		if errOk != nil {
			t.Fatalf("lsap_sample.json failed: %v\n%s", errOk, outOk)
		}
		if strings.TrimSpace(string(outOk)) != "VERIFIED mode=exact tol=0/1" {
			t.Fatalf("lsap_sample.json output = %q, want %q", outOk, "VERIFIED mode=exact tol=0/1")
		}

		negPath := filepath.Join("..", "testdata", "formal", "lsap_negative.json")
		cmdNeg := exec.CommandContext(t.Context(), exe, negPath)
		if outNeg, errNeg := cmdNeg.CombinedOutput(); errNeg == nil {
			t.Fatalf("lsap_negative.json unexpectedly accepted:\n%s", outNeg)
		}
	})

	t.Run("ExactLiveSolutions", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(tmpDir, "exact")
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		cases := []struct {
			name   string
			matrix [][]float64
			sense  Sense
		}{
			{
				name:   "square_3x3_min",
				matrix: [][]float64{{4, 1, 3}, {2, 0, 5}, {3, 2, 2}},
				sense:  Minimize,
			},
			{
				name:   "square_3x3_max",
				matrix: [][]float64{{4, 1, 3}, {2, 0, 5}, {3, 2, 2}},
				sense:  Maximize,
			},
			{
				name: "square_4x4_hungarian_min",
				matrix: [][]float64{
					{90, 75, 75, 80},
					{35, 85, 55, 65},
					{125, 95, 90, 105},
					{45, 110, 95, 115},
				},
				sense: Minimize,
			},
			{
				name: "square_4x4_dyadic_max",
				matrix: [][]float64{
					{1.25, -0.5, 2.75, 0.125},
					{3.5, 0.25, -1.5, 4.0},
					{-2.0, 1.75, 3.25, 0.5},
					{0.75, 2.5, 1.0, -0.25},
				},
				sense: Maximize,
			},
			{
				name: "wide_3x5_min",
				matrix: [][]float64{
					{10, 2, 8, 15, 4},
					{3, 9, 12, 1, 7},
					{6, 5, 11, 14, 3},
				},
				sense: Minimize,
			},
			{
				name: "wide_3x5_dyadic_max",
				matrix: [][]float64{
					{10.5, 2.25, 8.0, 15.75, 4.125},
					{3.25, 9.5, 12.0, 1.5, 7.25},
					{6.75, 5.0, 11.25, 14.0, 3.5},
				},
				sense: Maximize,
			},
			{
				name: "tall_5x3_min",
				matrix: [][]float64{
					{10, 2, 8},
					{3, 9, 12},
					{6, 5, 11},
					{15, 1, 14},
					{4, 7, 3},
				},
				sense: Minimize,
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
				sense: Maximize,
			},
			{
				name:   "single_1x1_min",
				matrix: [][]float64{{-42.5}},
				sense:  Minimize,
			},
			{
				name:   "single_1x1_max",
				matrix: [][]float64{{-42.5}},
				sense:  Maximize,
			},
			{
				name:   "single_row_1x4_min",
				matrix: [][]float64{{9, -3.5, 4, 2.25}},
				sense:  Minimize,
			},
			{
				name:   "single_col_4x1_max",
				matrix: [][]float64{{9}, {-3.5}, {14.25}, {2}},
				sense:  Maximize,
			},
		}

		for _, tc := range cases {
			rows := len(tc.matrix)
			cols := len(tc.matrix[0])
			for _, mode := range []struct {
				tag string
				opt Option
			}{
				{"simd", WithSIMD(true)},
				{"scalar", WithoutSIMD()},
			} {
				res, err := LinearSum(t.Context(), tc.matrix, tc.sense, mode.opt)
				if err != nil {
					t.Fatalf("%s/%s LinearSum: %v", tc.name, mode.tag, err)
				}
				payload := buildLSAPCertPayload(t, rows, cols, tc.matrix, tc.sense, res, "exact", 0)
				assertLSAPAccepted(t, exe, dir, tc.name+"_"+mode.tag, payload)
			}
		}

		// Verify empty matrices (0x0, 0x3, 2x0) via LinearSumFlat.
		for _, dim := range [][2]int{{0, 0}, {0, 3}, {2, 0}} {
			r, c := dim[0], dim[1]
			res, err := LinearSumFlat(t.Context(), r, c, nil, Minimize, WithoutSIMD())
			if err != nil {
				t.Fatalf("LinearSumFlat(%d, %d): %v", r, c, err)
			}
			mat := make([][]float64, r)
			for i := range r {
				mat[i] = make([]float64, c)
			}
			name := "empty_" + strconv.Itoa(r) + "x" + strconv.Itoa(c)
			payload := buildLSAPCertPayload(t, r, c, mat, Minimize, res, "exact", 0)
			assertLSAPAccepted(t, exe, dir, name, payload)
		}

		// Verify CLI --safe and --tol overrides on a valid exact certificate.
		assertLSAPAccepted(t, exe, dir, "cli_safe_override",
			buildLSAPCertPayload(t, 3, 3, [][]float64{{4, 1, 3}, {2, 0, 5}, {3, 2, 2}}, Minimize, func() Result {
				r, err := LinearSum(t.Context(), [][]float64{{4, 1, 3}, {2, 0, 5}, {3, 2, 2}}, Minimize)
				if err != nil {
					t.Fatal(err)
				}
				return r
			}(), "exact", 0),
			"--safe", "--tol", "1/1000")

		// Verify randomized integer and dyadic instances across square, wide, and tall shapes.
		rng := rand.New(rand.NewPCG(42, 1337))
		shapes := [][2]int{{8, 8}, {6, 11}, {11, 6}, {12, 12}, {5, 9}, {9, 5}}
		for idx, sh := range shapes {
			rows, cols := sh[0], sh[1]
			mat := make([][]float64, rows)
			for r := range rows {
				mat[r] = make([]float64, cols)
				for c := range cols {
					mat[r][c] = float64(rng.IntN(401)-200) / 8.0
				}
			}
			for _, sense := range []Sense{Minimize, Maximize} {
				for _, mode := range []struct {
					tag string
					opt Option
				}{
					{"simd", WithSIMD(true)},
					{"scalar", WithoutSIMD()},
				} {
					res, err := LinearSum(t.Context(), mat, sense, mode.opt)
					if err != nil {
						t.Fatalf("rand_%d/%s/%s: %v", idx, sense, mode.tag, err)
					}
					name := "rand_" + strconv.Itoa(idx) + "_" + sense.String() + "_" + mode.tag
					payload := buildLSAPCertPayload(t, rows, cols, mat, sense, res, "exact", 0)
					assertLSAPAccepted(t, exe, dir, name, payload)
				}
			}
		}
	})

	t.Run("SafeFloat64Solutions", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(tmpDir, "safe")
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		rng := rand.New(rand.NewPCG(9001, 777))
		shapes := [][2]int{{4, 4}, {8, 8}, {4, 7}, {7, 4}, {6, 10}, {10, 6}, {12, 12}}
		for idx, sh := range shapes {
			rows, cols := sh[0], sh[1]
			mat := make([][]float64, rows)
			maxAbs := 0.0
			for r := range rows {
				mat[r] = make([]float64, cols)
				for c := range cols {
					val := (rng.Float64()-0.5)*100.0 + math.Sin(float64(r*31+c*17))/3.0 + math.Pi*0.1*float64((r+c)%5)
					mat[r][c] = val
					maxAbs = max(maxAbs, math.Abs(val))
				}
			}
			if maxAbs == 0 {
				maxAbs = 1.0
			}
			tol := 64.0 * 0x1p-52 * maxAbs * float64(max(rows, cols))

			for _, sense := range []Sense{Minimize, Maximize} {
				for _, mode := range []struct {
					tag string
					opt Option
				}{
					{"simd", WithSIMD(true)},
					{"scalar", WithoutSIMD()},
				} {
					res, err := LinearSum(t.Context(), mat, sense, mode.opt)
					if err != nil {
						t.Fatalf("safe_%d/%s/%s: %v", idx, sense, mode.tag, err)
					}
					name := "safe_" + strconv.Itoa(idx) + "_" + sense.String() + "_" + mode.tag
					payload := buildLSAPCertPayload(t, rows, cols, mat, sense, res, "safe", tol)
					assertLSAPAccepted(t, exe, dir, name, payload)
				}
			}
		}
	})

	t.Run("NegativeControls", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(tmpDir, "negative")
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		// Base valid 3x4 rectangular Minimize certificate.
		rectCost := [][]float64{
			{10, 2, 8, 15},
			{3, 9, 12, 1},
			{6, 5, 11, 14},
		}
		rectRes, err := LinearSum(t.Context(), rectCost, Minimize, WithoutSIMD())
		if err != nil {
			t.Fatalf("LinearSum rectCost: %v", err)
		}
		baseRect := buildLSAPCertPayload(t, 3, 4, rectCost, Minimize, rectRes, "exact", 0)
		assertLSAPAccepted(t, exe, dir, "base_rect_valid", baseRect)

		// 1. Duplicate ColInd.
		dupCol := buildLSAPCertPayload(t, 3, 4, rectCost, Minimize, rectRes, "exact", 0)
		dupCol.ColInd[1] = dupCol.ColInd[0]
		assertLSAPRejected(t, exe, dir, "neg_dup_col", dupCol)

		// Reject duplicate ColInd in safe mode too.
		dupColSafe := buildLSAPCertPayload(t, 3, 4, rectCost, Minimize, rectRes, "safe", 100.0)
		dupColSafe.ColInd[1] = dupColSafe.ColInd[0]
		assertLSAPRejected(t, exe, dir, "neg_dup_col_safe", dupColSafe)

		// 2. Duplicate RowInd.
		dupRow := buildLSAPCertPayload(t, 3, 4, rectCost, Minimize, rectRes, "exact", 0)
		dupRow.RowInd[2] = dupRow.RowInd[0]
		assertLSAPRejected(t, exe, dir, "neg_dup_row", dupRow)

		// 3. Violate U[i] + V[j] <= C[i][j] while keeping complementary slackness.
		violDual := lsapCertPayload{
			Rows:      3,
			Cols:      4,
			Sense:     "min",
			Mode:      "exact",
			Cost:      baseRect.Cost,
			RowInd:    []int{0, 1, 2},
			ColInd:    []int{1, 3, 0},
			Objective: int64ToPayload(9),
			U:         []ratPayload{int64ToPayload(10), int64ToPayload(1), int64ToPayload(6)},
			V:         []ratPayload{int64ToPayload(0), int64ToPayload(-8), int64ToPayload(0), int64ToPayload(0)},
		}
		assertLSAPRejected(t, exe, dir, "neg_dual_feasibility", violDual)

		// 4. Set V[j] > 0 on a rectangular matrix while all other checks pass.
		posRectCost := [][]float64{
			{1, 10, 10, 0},
			{10, 2, 10, 10},
			{10, 10, 3, 10},
		}
		posRectRes := Result{
			RowInd:    []int{0, 1, 2},
			ColInd:    []int{0, 1, 2},
			Objective: 6,
			U:         []float64{0, 2, 3},
			V:         []float64{1, 0, 0, 0},
		}
		posRect := buildLSAPCertPayload(t, 3, 4, posRectCost, Minimize, posRectRes, "exact", 0)
		assertLSAPRejected(t, exe, dir, "neg_pos_rect_v", posRect)

		posRectSafe := buildLSAPCertPayload(t, 3, 4, posRectCost, Minimize, posRectRes, "safe", 1.0)
		assertLSAPRejected(t, exe, dir, "neg_pos_rect_v_safe", posRectSafe)

		// 5. Set unassigned V[j] < 0 while all other checks pass.
		unassCost := [][]float64{
			{1, 10, 10, 10},
			{10, 2, 10, 10},
			{10, 10, 3, 10},
		}
		unassRes := Result{
			RowInd:    []int{0, 1, 2},
			ColInd:    []int{0, 1, 2},
			Objective: 6,
			U:         []float64{1, 2, 3},
			V:         []float64{0, 0, 0, -5},
		}
		unassPayload := buildLSAPCertPayload(t, 3, 4, unassCost, Minimize, unassRes, "exact", 0)
		assertLSAPRejected(t, exe, dir, "neg_nonzero_unassigned_v", unassPayload)

		// 6. Set unassigned U[r] < 0 on a tall matrix (rows > cols).
		tallCost := [][]float64{
			{1, 10, 10},
			{10, 2, 10},
			{10, 10, 3},
			{10, 10, 10},
		}
		tallUnassRes := Result{
			RowInd:    []int{0, 1, 2},
			ColInd:    []int{0, 1, 2},
			Objective: 6,
			U:         []float64{0, 0, 0, -5},
			V:         []float64{1, 2, 3},
		}
		tallUnassPayload := buildLSAPCertPayload(t, 4, 3, tallCost, Minimize, tallUnassRes, "exact", 0)
		assertLSAPRejected(t, exe, dir, "neg_tall_nonzero_unassigned_u", tallUnassPayload)

		// 7. Break complementary slackness while keeping dual bounds valid.
		brokenCS := lsapCertPayload{
			Rows:      3,
			Cols:      4,
			Sense:     "min",
			Mode:      "exact",
			Cost:      baseRect.Cost,
			RowInd:    []int{0, 1, 2},
			ColInd:    []int{1, 3, 0},
			Objective: int64ToPayload(9),
			U:         []ratPayload{int64ToPayload(2), int64ToPayload(1), int64ToPayload(6)},
			V:         []ratPayload{int64ToPayload(0), int64ToPayload(-1), int64ToPayload(0), int64ToPayload(0)},
		}
		assertLSAPRejected(t, exe, dir, "neg_broken_comp_slack", brokenCS)

		// 8. Reject suboptimal matching in safe mode when gap exceeds tol.
		suboptRes := Result{
			RowInd:    []int{0, 1, 2},
			ColInd:    []int{0, 1, 2},
			Objective: 30,
			U:         rectRes.U,
			V:         rectRes.V,
		}
		suboptSafe := buildLSAPCertPayload(t, 3, 4, rectCost, Minimize, suboptRes, "safe", 1e-6)
		assertLSAPRejected(t, exe, dir, "neg_safe_gap_exceeded", suboptSafe)

		// 9. Reject out-of-range ColInd, mismatched objective, and malformed JSON.
		badCol := buildLSAPCertPayload(t, 3, 4, rectCost, Minimize, rectRes, "exact", 0)
		badCol.ColInd[0] = 4
		assertLSAPRejected(t, exe, dir, "neg_col_oob", badCol)

		badObj := buildLSAPCertPayload(t, 3, 4, rectCost, Minimize, rectRes, "exact", 0)
		badObj.Objective = int64ToPayload(999)
		assertLSAPRejected(t, exe, dir, "neg_bad_obj", badObj)

		malformedPath := filepath.Join(dir, "malformed.json")
		if writeErr := os.WriteFile(malformedPath, []byte(`{"rows":`), 0o600); writeErr != nil {
			t.Fatalf("write malformed: %v", writeErr)
		}
		cmdMalformed := exec.CommandContext(t.Context(), exe, malformedPath)
		if outMal, malErr := cmdMalformed.CombinedOutput(); malErr == nil {
			t.Fatalf("malformed JSON unexpectedly accepted:\n%s", outMal)
		}
	})
}

func TestHasPotentialCancellationFallbackAndZeroAlloc(t *testing.T) {
	// 1. Constant/tied matrix with zero duality gap saturates the Pass-1 candidate cache
	//    while remaining certified without Stage 2 and with zero heap allocations.
	const n = 40
	cost := make([]float64, n*n)
	u := make([]float64, n)
	v := make([]float64, n)
	col4row := make([]int, n)
	row4col := make([]int, n)
	for i := range n {
		col4row[i] = i
		row4col[i] = i
		u[i] = 1.0
		v[i] = 2.0
		for j := range n {
			cost[i*n+j] = 3.0
		}
	}
	ws := AcquireWorkspace(n, n)
	defer ReleaseWorkspace(ws)
	for _, dis := range []bool{false, true} {
		if hasPotentialCancellation(n, n, cost, u, v, col4row, row4col, ws, dis) {
			t.Fatalf("disableSIMD=%v: exact tied matrix falsely flagged for Stage 2", dis)
		}
	}
	allocs := testing.AllocsPerRun(50, func() {
		if hasPotentialCancellation(n, n, cost, u, v, col4row, row4col, ws, false) {
			t.Fatal("unexpected Stage 2 flag")
		}
	})
	if allocs != 0 {
		t.Fatalf("hasPotentialCancellation allocated %.1f times/run, want 0", allocs)
	}

	// 2. Violation-overflow fallback (projectCancelViolFallback): inject > 2*n tiny 1-ULP
	//    violations on an acyclic upper-triangular tight structure and verify parity & feasibility.
	cost2 := make([]float64, n*n)
	u2 := make([]float64, n)
	v2 := make([]float64, n)
	for i := range n {
		u2[i] = 1.0
		v2[i] = 1.0
		for j := range n {
			switch {
			case i == j:
				cost2[i*n+j] = 2.0
			case j > i:
				cost2[i*n+j] = math.Nextafter(2.0, 0)
			default:
				cost2[i*n+j] = 10.0
			}
		}
	}
	for _, dis := range []bool{false, true} {
		uTry := slices.Clone(u2)
		vTry := slices.Clone(v2)
		got := hasPotentialCancellation(n, n, cost2, uTry, vTry, col4row, row4col, ws, dis)
		if !got {
			for i := range n {
				for j := range n {
					rc, ok := exactReducedCost(cost2[i*n+j], uTry[i], vTry[j])
					if !ok || rc < 0 {
						t.Fatalf("disableSIMD=%v: projected dual infeasible at (%d,%d): rc=%g ok=%v", dis, i, j, rc, ok)
					}
				}
			}
		}
	}
}

func TestExactSmallIntFastPathAndNegativeControls(t *testing.T) {
	verifyExactIntStrongDuality := func(t *testing.T, rows, cols int, cost []float64, sense Sense, res Result) {
		t.Helper()
		k := min(rows, cols)
		if len(res.RowInd) != k || len(res.ColInd) != k {
			t.Fatalf("matching len (%d,%d), want %d", len(res.RowInd), len(res.ColInd), k)
		}
		if len(res.U) != rows || len(res.V) != cols {
			t.Fatalf("dual len (%d,%d), want (%d,%d)", len(res.U), len(res.V), rows, cols)
		}
		matchedRow := make([]bool, rows)
		matchedCol := make([]bool, cols)
		objSum := 0.0
		sign := 1.0
		if sense == Maximize {
			sign = -1.0
		}
		for idx := range k {
			r, c := res.RowInd[idx], res.ColInd[idx]
			if r < 0 || r >= rows || c < 0 || c >= cols || matchedRow[r] || matchedCol[c] {
				t.Fatalf("invalid matching pair (%d,%d)", r, c)
			}
			matchedRow[r] = true
			matchedCol[c] = true
			val := cost[r*cols+c]
			objSum += val
			if rc, ok := exactReducedCost(sign*val, res.U[r], res.V[c]); !ok || rc != 0 {
				t.Fatalf("matched pair (%d,%d) nonzero slack rc=%g ok=%v", r, c, rc, ok)
			}
		}
		if res.Objective != objSum {
			t.Fatalf("objective=%g, want exact %g", res.Objective, objSum)
		}
		for r := range rows {
			if rows > cols {
				if res.U[r] > 0 || (!matchedRow[r] && res.U[r] != 0) {
					t.Fatalf("tall unassigned/sign violation u[%d]=%g matched=%v", r, res.U[r], matchedRow[r])
				}
			}
			for c := range cols {
				val := cost[r*cols+c]
				if (sense == Minimize && math.IsInf(val, 1)) || (sense == Maximize && math.IsInf(val, -1)) {
					continue
				}
				if rc, ok := exactReducedCost(sign*val, res.U[r], res.V[c]); !ok || rc < 0 {
					t.Fatalf("dual infeasible at (%d,%d): rc=%g ok=%v", r, c, rc, ok)
				}
			}
		}
		if rows < cols {
			for c := range cols {
				if res.V[c] > 0 || (!matchedCol[c] && res.V[c] != 0) {
					t.Fatalf("wide unassigned/sign violation v[%d]=%g matched=%v", c, res.V[c], matchedCol[c])
				}
			}
		}
	}

	rng := rand.New(rand.NewPCG(9001, 1337))
	// 1. Positive controls: square tied, sparse +Inf, wide, tall, maximize (including -Inf).
	type caseSpec struct {
		name  string
		rows  int
		cols  int
		sense Sense
		build func() []float64
	}
	cases := []caseSpec{
		{
			name: "square_tied_36x36_min", rows: 36, cols: 36, sense: Minimize,
			build: func() []float64 {
				c := make([]float64, 36*36)
				for i := range c {
					c[i] = float64(rng.IntN(7) - 3)
				}
				return c
			},
		},
		{
			name: "square_sparse_inf_24x24_min", rows: 24, cols: 24, sense: Minimize,
			build: func() []float64 {
				c := make([]float64, 24*24)
				for i := range c {
					c[i] = math.Inf(1)
				}
				for i := range 24 {
					c[i*24+i] = float64((i % 5) + 1)
					c[i*24+((i+1)%24)] = float64(((i + 2) % 5) + 1)
					c[i*24+((i+7)%24)] = float64(((i + 3) % 7) - 2)
				}
				return c
			},
		},
		{
			name: "wide_12x40_min", rows: 12, cols: 40, sense: Minimize,
			build: func() []float64 {
				c := make([]float64, 12*40)
				for i := range c {
					c[i] = float64(rng.IntN(25) - 12)
				}
				return c
			},
		},
		{
			name: "tall_40x12_max_negInf", rows: 40, cols: 12, sense: Maximize,
			build: func() []float64 {
				c := make([]float64, 40*12)
				for i := range c {
					c[i] = float64(rng.IntN(30) - 10)
				}
				for i := range 20 {
					c[(i+15)*12+(i%12)] = math.Inf(-1)
				}
				return c
			},
		},
		{
			name: "square_36x36_max", rows: 36, cols: 36, sense: Maximize,
			build: func() []float64 {
				c := make([]float64, 36*36)
				for i := range c {
					c[i] = float64(rng.IntN(100) - 50)
				}
				return c
			},
		},
	}

	for _, tc := range cases {
		cost := tc.build()
		ws := AcquireWorkspace(tc.rows, tc.cols)
		for _, dis := range []bool{false, true} {
			res, err := LinearSumFlat(t.Context(), tc.rows, tc.cols, cost, tc.sense, WithSIMD(!dis))
			if err != nil {
				t.Fatalf("%s (disableSIMD=%v): LinearSumFlat failed: %v", tc.name, dis, err)
			}
			verifyExactIntStrongDuality(t, tc.rows, tc.cols, cost, tc.sense, res)
		}
		rowInd := make([]int, min(tc.rows, tc.cols))
		colInd := make([]int, min(tc.rows, tc.cols))
		// Warm workspace and confirm zero allocations on SolveInto.
		if _, err := SolveInto(t.Context(), tc.rows, tc.cols, cost, tc.sense, rowInd, colInd, ws); err != nil {
			t.Fatalf("%s warm SolveInto failed: %v", tc.name, err)
		}
		allocs := testing.AllocsPerRun(50, func() {
			if _, err := SolveInto(t.Context(), tc.rows, tc.cols, cost, tc.sense, rowInd, colInd, ws); err != nil {
				t.Fatalf("SolveInto failed: %v", err)
			}
		})
		if allocs != 0 {
			t.Fatalf("%s SolveInto allocated %.1f times/run, want 0", tc.name, allocs)
		}
		ReleaseWorkspace(ws)
	}

	// 2. Negative controls: hidden fractional entry and out-of-range integer must set allSmallInt=false.
	ws := AcquireWorkspace(40, 40)
	defer ReleaseWorkspace(ws)
	n := 36
	intLim := exactSmallIntLimit(n)
	hiddenFrac := make([]float64, n*n)
	for i := range hiddenFrac {
		hiddenFrac[i] = float64((i % 11) - 5)
	}
	hiddenFrac[(n/2)*n+(n/2)] = 1.5
	for _, dis := range []bool{false, true} {
		ok, allInt, err := colReduceValidateSquare(n, hiddenFrac, ws.v[:n], ws.row4col[:n], intLim, true, dis, nil)
		if err != nil || !ok || allInt {
			t.Fatalf("hiddenFrac square (dis=%v): got ok=%v allInt=%v err=%v, want ok=true allInt=false", dis, ok, allInt, err)
		}
		// Place fractional entry inside 12*40 window and check wide & tall maximize paths.
		rectFrac := slices.Clone(hiddenFrac[:12*40])
		rectFrac[6*40+21] = -0.25
		_, _, _, _, _, _, allIntRect, _, err := validateAndPrepareCost(12, 40, rectFrac, false, Minimize, ws, dis)
		if err != nil || allIntRect {
			t.Fatalf("hiddenFrac wide (dis=%v): got allInt=%v err=%v, want false", dis, allIntRect, err)
		}
		_, _, _, _, _, _, allIntTall, _, err := validateAndPrepareCost(40, 12, rectFrac, false, Maximize, ws, dis)
		if err != nil || allIntTall {
			t.Fatalf("hiddenFrac tall max (dis=%v): got allInt=%v err=%v, want false", dis, allIntTall, err)
		}
	}

	// Out-of-range exact integer (Bug #5 scale 1e15+1 > exactSmallIntLimit) must NOT set allSmallInt=true.
	bigIntMat := make([]float64, n*n)
	for i := range bigIntMat {
		bigIntMat[i] = 1e15 + float64(i%3)
	}
	if probeExactSmallInt(bigIntMat, intLim, false) {
		t.Fatalf("probeExactSmallInt unexpectedly accepted 1e15 > intLim=%g", intLim)
	}
	bigHidden := make([]float64, n*n)
	for i := range bigHidden {
		bigHidden[i] = float64(i % 7)
	}
	bigHidden[(n/2)*n+5] = intLim + 1
	if ok, allInt, err := colReduceValidateSquare(n, bigHidden, ws.v[:n], ws.row4col[:n], intLim, true, false, nil); err != nil || !ok || allInt {
		t.Fatalf("bigHidden: got ok=%v allInt=%v err=%v, want ok=true allInt=false", ok, allInt, err)
	}

	// 3. Direct negative controls on verifyExactSmallIntStage1.
	c2 := []float64{1, 4, 2, 3}
	u2 := []float64{1, 2}
	v2 := []float64{0, 1}
	c4r2 := []int{0, 1}
	r4c2 := []int{0, 1}
	if obj, ok := verifyExactSmallIntStage1(2, 2, c2, u2, v2, c4r2, r4c2, 100); !ok || obj != 4 {
		t.Fatalf("verifyExactSmallIntStage1 valid 2x2: got (%g,%v), want (4,true)", obj, ok)
	}
	if _, ok := verifyExactSmallIntStage1(2, 2, c2, []float64{1.5, 2}, v2, c4r2, r4c2, 100); ok {
		t.Fatal("verifyExactSmallIntStage1 accepted non-integer u")
	}
	if _, ok := verifyExactSmallIntStage1(2, 2, c2, u2, []float64{0, 1.25}, c4r2, r4c2, 100); ok {
		t.Fatal("verifyExactSmallIntStage1 accepted non-integer v")
	}
	if _, ok := verifyExactSmallIntStage1(2, 2, c2, []float64{0x1p51, 2}, []float64{1 - 0x1p51, 1}, c4r2, r4c2, 100); ok {
		t.Fatal("verifyExactSmallIntStage1 accepted |u| > exactIntDualCap")
	}
	if _, ok := verifyExactSmallIntStage1(2, 2, c2, []float64{0, 2}, v2, c4r2, r4c2, 100); ok {
		t.Fatal("verifyExactSmallIntStage1 accepted nonzero matched slack")
	}
	if _, ok := verifyExactSmallIntStage1(2, 2, c2, u2, v2, []int{0, 0}, r4c2, 100); ok {
		t.Fatal("verifyExactSmallIntStage1 accepted non-injective col4row")
	}
	// Rectangular unassigned column with nonzero v or positive v must be rejected.
	cRect := []float64{1, 2, 3, 4, 5, 6}
	if _, ok := verifyExactSmallIntStage1(2, 3, cRect, []float64{1, 5}, []float64{0, 0, -1}, []int{0, 1}, []int{0, 1, -1}, 100); ok {
		t.Fatal("verifyExactSmallIntStage1 accepted unassigned rectangular column with v != 0")
	}
	if _, ok := verifyExactSmallIntStage1(2, 3, cRect, []float64{0, 5}, []float64{1, 0, 0}, []int{0, 1}, []int{0, 1, -1}, 100); ok {
		t.Fatal("verifyExactSmallIntStage1 accepted rectangular column with v > 0")
	}
}
