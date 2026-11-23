package assign_test

import (
	"bytes"
	"encoding/json/v2"
	"errors"
	"fmt"
	"math"
	"math/big"
	"math/rand/v2"
	"os/exec"
	"strings"
	"testing"

	"github.com/paulourio/goblin/assign"
)

func mustAssignRat(tb testing.TB, v float64) *big.Rat {
	tb.Helper()
	r := new(big.Rat).SetFloat64(v)
	if r == nil {
		tb.Fatalf("non-finite float64 %v", v)
	}
	return r
}

func exactBruteForceLSAPMin(tb testing.TB, rows, cols int, cost []float64) *big.Rat {
	tb.Helper()
	if rows > 6 || cols > 7 {
		tb.Fatalf("exactBruteForceLSAPMin size too large: %dx%d", rows, cols)
	}
	ratCost := make([]*big.Rat, rows*cols)
	for i, v := range cost {
		ratCost[i] = mustAssignRat(tb, v)
	}
	used := make([]bool, cols)
	var best *big.Rat
	var dfs func(r int, acc *big.Rat)
	dfs = func(r int, acc *big.Rat) {
		if r == rows {
			if best == nil || acc.Cmp(best) < 0 {
				best = new(big.Rat).Set(acc)
			}
			return
		}
		base := r * cols
		for c := range cols {
			if used[c] {
				continue
			}
			used[c] = true
			next := new(big.Rat).Add(acc, ratCost[base+c])
			dfs(r+1, next)
			used[c] = false
		}
	}
	dfs(0, new(big.Rat))
	return best
}

func exactLSAPSumRat(tb testing.TB, cols int, cost []float64, rowInd, colInd []int) *big.Rat {
	tb.Helper()
	sum := new(big.Rat)
	for k := range rowInd {
		sum.Add(sum, mustAssignRat(tb, cost[rowInd[k]*cols+colInd[k]]))
	}
	return sum
}

func checkExactDualFeasRat(tb testing.TB, rows, cols int, cost []float64, sense assign.Sense, res assign.Result) {
	tb.Helper()
	sign := big.NewRat(1, 1)
	if sense == assign.Maximize {
		sign = big.NewRat(-1, 1)
	}
	uRat := make([]*big.Rat, len(res.U))
	vRat := make([]*big.Rat, len(res.V))
	for i, ui := range res.U {
		uRat[i] = mustAssignRat(tb, ui)
	}
	for j, vj := range res.V {
		vRat[j] = mustAssignRat(tb, vj)
	}
	for r := range rows {
		for c := range cols {
			cij := new(big.Rat).Mul(sign, mustAssignRat(tb, cost[r*cols+c]))
			lhs := new(big.Rat).Add(uRat[r], vRat[c])
			if lhs.Cmp(cij) > 0 {
				diff, _ := new(big.Rat).Sub(lhs, cij).Float64()
				tb.Fatalf("dual feasibility violated at (%d,%d): u+v - c = %g > 0 (u=%v v=%v c=%v)", r, c, diff, res.U[r], res.V[c], cost[r*cols+c])
			}
		}
	}
}

func checkFormalLSAPNoObjJSONL(tb testing.TB, rows, cols int, cost []float64, sense assign.Sense, res assign.Result, mode string, tol float64) *big.Rat {
	tb.Helper()
	costBits := make([][]lsapBitsVal, rows)
	for r := range rows {
		row := make([]lsapBitsVal, cols)
		for c := range cols {
			row[c] = lsapBitsVal{Bits: math.Float64bits(cost[r*cols+c])}
		}
		costBits[r] = row
	}
	uBits := make([]lsapBitsVal, len(res.U))
	for i, v := range res.U {
		uBits[i] = lsapBitsVal{Bits: math.Float64bits(v)}
	}
	vBits := make([]lsapBitsVal, len(res.V))
	for j, v := range res.V {
		vBits[j] = lsapBitsVal{Bits: math.Float64bits(v)}
	}
	senseStr := "min"
	if sense == assign.Maximize {
		senseStr = "max"
	}
	req := map[string]any{
		"rows":    rows,
		"cols":    cols,
		"sense":   senseStr,
		"mode":    mode,
		"cost":    costBits,
		"row_ind": res.RowInd,
		"col_ind": res.ColInd,
		"u":       uBits,
		"v":       vBits,
	}
	if mode == "safe" {
		req["tol"] = lsapBitsVal{Bits: math.Float64bits(tol)}
	}
	raw, err := json.Marshal(req)
	if err != nil {
		tb.Fatalf("marshal lsap-check: %v", err)
	}
	ok, boundStr, detail := formalAssignOracleCall(tb, raw)
	if !ok {
		tb.Fatalf("lsap-check (%s, tol=%g) rejected solution: %s", mode, tol, detail)
	}
	if mode == "safe" && boundStr[0] != "" {
		bRat, bErr := (formalAssignRat{Num: boundStr[0], Den: boundStr[1]}).bigRat()
		if bErr != nil {
			tb.Fatalf("parse lsap-check bound %v: %v", boundStr, bErr)
		}
		return bRat
	}
	return nil
}

func TestFormalAdversarialBug5NearRankOneULPDrift(t *testing.T) {
	t.Parallel()

	scales := []float64{1e-8, 1.0, 1e8, 1e15}
	dims := [][2]int{{4, 4}, {5, 5}, {6, 6}, {4, 6}, {5, 4}}
	rng := rand.New(rand.NewPCG(0xbad50001, 0xfeedface))

	checkedFormal := 0
	for _, scale := range scales {
		for _, dim := range dims {
			rows, cols := dim[0], dim[1]
			for trial := range 25 {
				a := make([]float64, rows)
				b := make([]float64, cols)
				for i := range rows {
					a[i] = scale * (1.0 + rng.Float64())
				}
				for j := range cols {
					b[j] = scale * (1.0 + rng.Float64())
				}
				cost := make([]float64, rows*cols)
				for i := range rows {
					for j := range cols {
						base := a[i] + b[j]
						k := rng.IntN(17) - 8
						val := base
						if k > 0 {
							for range k {
								val = math.Nextafter(val, math.Inf(1))
							}
						} else if k < 0 {
							for range -k {
								val = math.Nextafter(val, math.Inf(-1))
							}
						}
						cost[i*cols+j] = val
					}
				}

				sense := assign.Minimize
				if trial%2 == 1 {
					sense = assign.Maximize
				}
				var exactOpt *big.Rat
				if sense == assign.Minimize {
					if rows <= cols {
						exactOpt = exactBruteForceLSAPMin(t, rows, cols, cost)
					} else {
						tr := make([]float64, cols*rows)
						for i := range rows {
							for j := range cols {
								tr[j*rows+i] = cost[i*cols+j]
							}
						}
						exactOpt = exactBruteForceLSAPMin(t, cols, rows, tr)
					}
				} else {
					neg := make([]float64, len(cost))
					for idx, v := range cost {
						neg[idx] = -v
					}
					if rows <= cols {
						exactOpt = new(big.Rat).Neg(exactBruteForceLSAPMin(t, rows, cols, neg))
					} else {
						tr := make([]float64, cols*rows)
						for i := range rows {
							for j := range cols {
								tr[j*rows+i] = neg[i*cols+j]
							}
						}
						exactOpt = new(big.Rat).Neg(exactBruteForceLSAPMin(t, cols, rows, tr))
					}
				}

				for _, disableSIMD := range []bool{false, true} {
					var opts []assign.Option
					if disableSIMD {
						opts = append(opts, assign.WithoutSIMD())
					}
					res, err := assign.LinearSumFlat(t.Context(), rows, cols, cost, sense, opts...)
					if err != nil {
						t.Fatalf("scale=%g %dx%d trial=%d simd=%v: err=%v", scale, rows, cols, trial, !disableSIMD, err)
					}
					gotExact := exactLSAPSumRat(t, cols, cost, res.RowInd, res.ColInd)
					if gotExact.Cmp(exactOpt) != 0 {
						gap, _ := new(big.Rat).Sub(gotExact, exactOpt).Float64()
						t.Fatalf("PRIMAL SUBOPTIMAL scale=%g %dx%d trial=%d sense=%v simd=%v: got=%v want=%v gap=%g",
							scale, rows, cols, trial, sense, !disableSIMD, gotExact, exactOpt, gap)
					}
					checkExactDualFeasRat(t, rows, cols, cost, sense, res)

					if formalAssignOracleEnabled() && !disableSIMD && trial < 8 {
						// Safe dual gap must be within a few ULPs of scale (zero when exact duals fit in float64).
						tol := 64.0 * 0x1p-53 * float64(min(rows, cols)) * scale
						bRat := checkFormalLSAPNoObjJSONL(t, rows, cols, cost, sense, res, "safe", tol)
						if bRat != nil {
							if sense == assign.Minimize && bRat.Cmp(gotExact) > 0 {
								t.Fatalf("scale=%g %dx%d trial=%d: safe bound %v > primal %v", scale, rows, cols, trial, bRat, gotExact)
							}
							if sense == assign.Maximize && bRat.Cmp(gotExact) < 0 {
								t.Fatalf("scale=%g %dx%d trial=%d: safe bound %v < primal %v", scale, rows, cols, trial, bRat, gotExact)
							}
						}
						checkedFormal++
					}
				}
			}
		}
	}
	if formalAssignOracleEnabled() && checkedFormal == 0 {
		t.Fatal("expected formal lsap-check queries to run when GOBLIN_FORMAL=1")
	}
	_ = fmt.Sprintf("checked=%d", checkedFormal)
}

func TestCertifiedAssignRepairsDualFeasibility(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	rng := rand.New(rand.NewPCG(0x7120001, 0x7120002))

	for trial := range 120 {
		rows := 2 + rng.IntN(10)
		cols := 2 + rng.IntN(10)
		cost := make([]float64, rows*cols)
		a := make([]float64, rows)
		b := make([]float64, cols)
		scale := math.Pow(10, float64(rng.IntN(6)))
		for i := range a {
			a[i] = (rng.Float64() - 0.5) * scale
		}
		for j := range b {
			b[j] = (rng.Float64() - 0.5) * scale
		}
		for r := range rows {
			for c := range cols {
				switch trial % 3 {
				case 0:
					base := a[r] + b[c]
					ulp := math.Abs(math.Nextafter(base, math.Inf(1)) - base)
					cost[r*cols+c] = base + float64(rng.IntN(5))*ulp
				case 1:
					cost[r*cols+c] = (rng.Float64() - 0.5) * 1000.0 * math.Sin(float64(r*31+c*17))
				default:
					cost[r*cols+c] = float64(rng.IntN(50) - 25)
				}
			}
		}
		sense := assign.Minimize
		if trial%2 == 1 {
			sense = assign.Maximize
		}

		res, err := assign.LinearSumFlat(ctx, rows, cols, cost, sense, assign.WithCertified(true))
		if err != nil {
			t.Fatalf("trial %d (%dx%d, %v): WithCertified failed: %v", trial, rows, cols, sense, err)
		}
		checkExactDualFeasRat(t, rows, cols, cost, sense, res)

		matchedRow := make([]bool, rows)
		matchedCol := make([]bool, cols)
		for k := range res.RowInd {
			matchedRow[res.RowInd[k]] = true
			matchedCol[res.ColInd[k]] = true
		}
		if rows < cols {
			for j := range cols {
				if res.V[j] > 0 || (!matchedCol[j] && res.V[j] != 0) {
					t.Fatalf("trial %d wide rectangular dual violation V[%d]=%g (matched=%v)", trial, j, res.V[j], matchedCol[j])
				}
			}
		} else if rows > cols {
			for i := range rows {
				if res.U[i] > 0 || (!matchedRow[i] && res.U[i] != 0) {
					t.Fatalf("trial %d tall rectangular dual violation U[%d]=%g (matched=%v)", trial, i, res.U[i], matchedRow[i])
				}
			}
		}
	}
}

func TestFormalLSAPAdversarialRegressions(t *testing.T) {
	t.Parallel()
	if !formalAssignOracleEnabled() {
		t.Skip("set GOBLIN_FORMAL=1 to run formal lsap-check adversarial regression tests")
	}

	binPath := locateLSAPCheckBin(t)

	runStdin := func(payload string, args ...string) (string, error) {
		cmd := exec.CommandContext(t.Context(), binPath, append(args, "-")...)
		cmd.Stdin = strings.NewReader(payload)
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}

	t.Run("NativeForbiddenRejectsBigMExploits", func(t *testing.T) {
		t.Parallel()

		// T1-3 repro 1 (bigm_infeasible): [[1, null], [null, null]] has no finite matching.
		// Matching [0, 1] uses forbidden cell (1,1) and must fail closed in both exact and safe modes.
		bigmInfeasExact := `{"rows":2,"cols":2,"sense":"min","mode":"exact","cost":[[1,null],[null,null]],"row_ind":[0,1],"col_ind":[0,1],"objective":1,"u":[1,0],"v":[0,0]}`
		if resp, err := callFormalAssignJSONL(t, []byte(bigmInfeasExact)); err != nil || resp.OK {
			t.Fatalf("expected forbidden-edge infeasible exact cert to fail closed, err=%v resp=%+v", err, resp)
		}
		bigmInfeasSafe := `{"rows":2,"cols":2,"sense":"min","mode":"safe","tol":100,"cost":[[1,null],[null,null]],"row_ind":[0,1],"col_ind":[0,1],"u":[1,0],"v":[0,0]}`
		if resp, err := callFormalAssignJSONL(t, []byte(bigmInfeasSafe)); err != nil || resp.OK {
			t.Fatalf("expected forbidden-edge infeasible safe cert to fail closed, err=%v resp=%+v", err, resp)
		}
		bigmInfeasHall := `{"rows":2,"cols":2,"sense":"min","mode":"hall","cost":[[1,null],[null,null]],"subset":[0,1]}`
		if resp, err := callFormalAssignJSONL(t, []byte(bigmInfeasHall)); err != nil || !resp.OK || resp.Mode != "hall" {
			t.Fatalf("expected Hall witness on [[1,null],[null,null]] to verify, err=%v resp=%+v", err, resp)
		}

		// T1-3 repro 2 (bigm_small_M): [[10, null], [null, 10]]. Antidiagonal [1, 0] uses forbidden cells
		// and must be rejected; valid diagonal [0, 1] with objective 20 must verify.
		bigmSmallBad := `{"rows":2,"cols":2,"sense":"min","mode":"exact","cost":[[10,null],[null,10]],"row_ind":[0,1],"col_ind":[1,0],"objective":0,"u":[0,0],"v":[0,0]}`
		if resp, err := callFormalAssignJSONL(t, []byte(bigmSmallBad)); err != nil || resp.OK {
			t.Fatalf("expected forbidden antidiagonal matching to fail closed, err=%v resp=%+v", err, resp)
		}
		bigmSmallGood := `{"rows":2,"cols":2,"sense":"min","mode":"exact","cost":[[10,"+Inf"],[{"forbidden":true},10]],"row_ind":[0,1],"col_ind":[0,1],"objective":20,"u":[10,10],"v":[0,0]}`
		if resp, err := callFormalAssignJSONL(t, []byte(bigmSmallGood)); err != nil || !resp.OK || resp.Mode != "exact" {
			t.Fatalf("expected finite diagonal matching with native forbidden cells to verify, err=%v resp=%+v", err, resp)
		}

		// Wrong-sign infinity and malformed forbidden object must be rejected.
		wrongSignMin := `{"rows":2,"cols":2,"sense":"min","mode":"exact","cost":[[1,"-Inf"],[5,5]],"row_ind":[0,1],"col_ind":[0,1],"u":[1,5],"v":[0,0]}`
		if resp, err := callFormalAssignJSONL(t, []byte(wrongSignMin)); err != nil || resp.OK {
			t.Fatalf("expected -Inf in min sense to fail closed, err=%v resp=%+v", err, resp)
		}
		wrongSignMax := `{"rows":2,"cols":2,"sense":"max","mode":"exact","cost":[[1,"+Inf"],[5,5]],"row_ind":[0,1],"col_ind":[0,1],"u":[-1,-5],"v":[0,0]}`
		if resp, err := callFormalAssignJSONL(t, []byte(wrongSignMax)); err != nil || resp.OK {
			t.Fatalf("expected +Inf in max sense to fail closed, err=%v resp=%+v", err, resp)
		}
		forbFalse := `{"rows":2,"cols":2,"sense":"min","mode":"exact","cost":[[1,{"forbidden":false}],[5,5]],"row_ind":[0,1],"col_ind":[0,1],"u":[1,5],"v":[0,0]}`
		if resp, err := callFormalAssignJSONL(t, []byte(forbFalse)); err != nil || resp.OK {
			t.Fatalf("expected forbidden:false object to fail closed, err=%v resp=%+v", err, resp)
		}

		// Tall matrix (rows > cols) with native forbidden edges via checkLSAPForbiddenTranspose.
		tallForb := `{"rows":3,"cols":2,"sense":"max","mode":"exact","cost":[[10,"-Inf"],[null,8],["-Inf",{"forbidden":true}]],"row_ind":[0,1],"col_ind":[0,1],"objective":18,"u":[0,0,0],"v":[-10,-8]}`
		if resp, err := callFormalAssignJSONL(t, []byte(tallForb)); err != nil || !resp.OK || resp.Mode != "exact" {
			t.Fatalf("expected tall forbidden max certificate to verify, err=%v resp=%+v", err, resp)
		}
	})

	t.Run("HallInfeasibilityChecks", func(t *testing.T) {
		t.Parallel()

		// Positive wide and tall Hall witnesses.
		wideHall := `{"rows":3,"cols":5,"sense":"min","mode":"hall","cost":[[1,null,2,null,null],[3,null,4,null,null],[5,null,6,null,null]],"subset":[0,1,2]}`
		if resp, err := callFormalAssignJSONL(t, []byte(wideHall)); err != nil || !resp.OK || resp.Mode != "hall" {
			t.Fatalf("expected wide Hall witness to verify, err=%v resp=%+v", err, resp)
		}
		tallHall := `{"rows":5,"cols":3,"sense":"max","mode":"infeasible","side":"cols","cost":[[1,2,3],["-Inf","-Inf","-Inf"],[4,5,6],[null,null,null],[null,null,null]],"subset":[0,1,2]}`
		if resp, err := callFormalAssignJSONL(t, []byte(tallHall)); err != nil || !resp.OK || resp.Mode != "hall" {
			t.Fatalf("expected tall Hall witness to verify, err=%v resp=%+v", err, resp)
		}

		// Negative controls: invalid witness, feasible graph, wrong side, empty matrix, duplicate index.
		negCases := []struct {
			name string
			json string
		}{
			{"non_violating_subset", `{"rows":3,"cols":3,"sense":"min","mode":"hall","cost":[[1,null,null],[2,null,null],[null,3,4]],"subset":[0,2]}`},
			{"feasible_matrix", `{"rows":2,"cols":2,"sense":"min","mode":"hall","cost":[[1,null],[null,2]],"subset":[0,1]}`},
			{"wrong_side_tall", `{"rows":4,"cols":3,"sense":"min","mode":"hall","side":"rows","cost":[[1,null,null],[2,null,null],[null,null,3],[null,null,4]],"subset":[0,1]}`},
			{"empty_matrix", `{"rows":0,"cols":3,"sense":"min","mode":"hall","cost":[],"subset":[]}`},
			{"duplicate_subset_idx", `{"rows":3,"cols":3,"sense":"min","mode":"hall","cost":[[1,null,null],[2,null,null],[null,3,4]],"subset":[0,0]}`},
		}
		for _, tc := range negCases {
			if resp, err := callFormalAssignJSONL(t, []byte(tc.json)); err != nil || resp.OK {
				t.Fatalf("%s: expected Hall negative control to fail closed, err=%v resp=%+v", tc.name, err, resp)
			}
		}
	})

	t.Run("AllocationDoSAndEmptyBatchAndDuplicateKeys", func(t *testing.T) {
		t.Parallel()

		// T1-7: huge declared dimension must fail cleanly without OOM panic.
		dosPayloads := []string{
			`{"rows":0,"cols":1000000000000,"cost":[],"row_ind":[],"col_ind":[],"u":[],"v":[]}`,
			`{"rows":1000000000000,"cols":0,"cost":[],"row_ind":[],"col_ind":[],"u":[],"v":[]}`,
			`{"rows":0,"cols":1000000000000,"cost":[],"mode":"hall","subset":[]}`,
		}
		for idx, p := range dosPayloads {
			if resp, err := callFormalAssignJSONL(t, []byte(p)); err != nil || resp.OK {
				t.Fatalf("DoS[%d] jsonl: expected clean rejection, err=%v resp=%+v", idx, err, resp)
			}
			out, cliErr := runStdin(p)
			if cliErr == nil || strings.Contains(out, "PANIC") {
				t.Fatalf("DoS[%d] cli: expected clean non-zero exit without panic, err=%v out=%q", idx, cliErr, out)
			}
		}

		// T1-7: empty --batch [] must be rejected.
		cmdEmptyBatch := exec.CommandContext(t.Context(), binPath, "--batch", "-")
		cmdEmptyBatch.Stdin = bytes.NewBufferString("[]")
		if out, err := cmdEmptyBatch.CombinedOutput(); err == nil {
			t.Fatalf("expected empty --batch [] to fail, got ok: %s", out)
		}

		// Duplicate keys (top-level, nested, and Unicode-escaped) must be rejected by parseJsonStrict.
		dupCases := []string{
			`{"rows":1,"cols":1,"sense":"min","sense":"min","mode":"exact","cost":[[1]],"row_ind":[0],"col_ind":[0],"u":[1],"v":[0]}`,
			`{"rows":1,"cols":1,"sense":"min","mode":"exact","cost":[[1]],"row_ind":[0],"col_ind":[0],"objective":{"num":"1","den":"2","den":"1"},"u":[1],"v":[0]}`,
			`{"rows":1,"cols":1,"sense":"min","mode":"exact","cost":[[1]],"co\u0073t":[[1]],"row_ind":[0],"col_ind":[0],"u":[1],"v":[0]}`,
		}
		for idx, p := range dupCases {
			resp, err := callFormalAssignJSONL(t, []byte(p))
			if err != nil || resp.OK || !strings.Contains(resp.Error, "duplicate JSON object key") {
				t.Fatalf("dup[%d]: expected duplicate-key rejection, err=%v resp=%+v", idx, err, resp)
			}
		}
	})

	t.Run("SafeModeEliminatesTwoTolDriftAndEchoesModeTol", func(t *testing.T) {
		t.Parallel()

		// C=[[1,2],[2,1]], OPT=2, matching p=[1,0] has primal=4, v=[0,0] gives safeLB=2, tol=2.
		// Objective=5 drifts by 3 > tol from safeLB (2*tol drift) and must be rejected.
		twoTolBadHi := `{"rows":2,"cols":2,"sense":"min","mode":"safe","tol":2,"cost":[[1,2],[2,1]],"row_ind":[0,1],"col_ind":[1,0],"objective":5,"u":[1,1],"v":[0,0]}`
		if resp, err := callFormalAssignJSONL(t, []byte(twoTolBadHi)); err != nil || resp.OK {
			t.Fatalf("expected 2*tol upper objective drift to fail closed, err=%v resp=%+v", err, resp)
		}
		twoTolBadLo := `{"rows":2,"cols":2,"sense":"min","mode":"safe","tol":2,"cost":[[1,2],[2,1]],"row_ind":[0,1],"col_ind":[1,0],"objective":1,"u":[1,1],"v":[0,0]}`
		if resp, err := callFormalAssignJSONL(t, []byte(twoTolBadLo)); err != nil || resp.OK {
			t.Fatalf("expected lower objective drift below primal-tol to fail closed, err=%v resp=%+v", err, resp)
		}
		safeOK := `{"rows":2,"cols":2,"sense":"min","mode":"safe","tol":"2/1","cost":[[1,2],[2,1]],"row_ind":[0,1],"col_ind":[1,0],"objective":4,"u":[1,1],"v":[0,0]}`
		resp, err := callFormalAssignJSONL(t, []byte(safeOK))
		if err != nil || !resp.OK || resp.Mode != "safe" || resp.Tol == nil || resp.Tol.Num != "2" || resp.Tol.Den != "1" {
			t.Fatalf("expected safe cert to verify and echo mode/tol, err=%v resp=%+v", err, resp)
		}
		outCLI, cliErr := runStdin(safeOK)
		if cliErr != nil || outCLI != "VERIFIED mode=safe tol=2/1" {
			t.Fatalf("expected CLI output %q, got err=%v out=%q", "VERIFIED mode=safe tol=2/1", cliErr, outCLI)
		}
	})

	t.Run("Over4096CellsReachesLeanInFeasibleAndHallModes", func(t *testing.T) {
		t.Parallel()
		rng := rand.New(rand.NewPCG(0x409601, 0x409602))

		// 80x90 = 7200 > 4096 cells with forbidden +Inf cells and non-dyadic float64 entries.
		rows, cols := 80, 90
		cost := make([]float64, rows*cols)
		for i := range rows {
			for j := range cols {
				if j == i || rng.IntN(4) != 0 {
					cost[i*cols+j] = (rng.Float64()-0.5)*100.0 + math.Sin(float64(i*17+j*29))*0.1
				} else {
					cost[i*cols+j] = math.Inf(1)
				}
			}
		}
		res, err := assign.LinearSumFlat(t.Context(), rows, cols, cost, assign.Minimize)
		if err != nil {
			t.Fatalf("LinearSumFlat 80x90: %v", err)
		}
		if err := checkFormalAssignResult(t, "over4096_80x90_forb", rows, cols, cost, assign.Minimize, res, false); err != nil {
			t.Fatalf("checkFormalAssignResult >4096 cells failed: %v", err)
		}

		// 70x80 = 5600 > 4096 cells Hall-infeasible graph verified via lsap-check mode="hall".
		hRows, hCols := 70, 80
		hCost := make([]float64, hRows*hCols)
		for i := range hCost {
			hCost[i] = math.Inf(1)
		}
		// Rows 0..34 (35 rows) connect only to columns 0..33 (34 cols); remaining rows connect densely.
		for r := range 35 {
			for c := range 34 {
				hCost[r*hCols+c] = float64(1 + (r+c)%11)
			}
		}
		for r := 35; r < hRows; r++ {
			for c := range hCols {
				hCost[r*hCols+c] = float64(1 + (r*3+c)%17)
			}
		}
		if _, err := assign.LinearSumFlat(t.Context(), hRows, hCols, hCost, assign.Minimize); !errors.Is(err, assign.ErrInfeasible) {
			t.Fatalf("expected ErrInfeasible on 70x80 Hall graph, got %v", err)
		}
		verifyFormalAssignInfeasible(t, "over4096_70x80_hall", hRows, hCols, hCost, assign.Minimize)
	})
}
