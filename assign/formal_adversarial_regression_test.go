package assign_test

import (
	"encoding/json/v2"
	"fmt"
	"math"
	"math/big"
	"math/rand/v2"
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
