package assign

import (
	"encoding/json/v2"
	"math"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
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
	if out != "VERIFIED" {
		tb.Fatalf("%s: expected stdout VERIFIED, got %q", name, out)
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
		if strings.TrimSpace(string(outOk)) != "VERIFIED" {
			t.Fatalf("lsap_sample.json output = %q, want VERIFIED", outOk)
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
			tol := 1e-11 * maxAbs * float64(max(rows, cols))

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
