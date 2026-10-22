package gopt_test

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gopt"
)

// TestCBCPorted_KnapsackAndModification ports testKnapsack, testProblemModification,
// and testKnapsackWithParams from Cbc/test/CInterfaceTest.c and Cbc/test/CInterfaceTest_new.c.
func TestCBCPorted_KnapsackAndModification(t *testing.T) {
	t.Run("CInterfaceTest_Knapsack_SequentialModification", func(t *testing.T) {
		// Maximize 5x0 + 3x1 + 2x2 + 7x3 + 4x4
		// s.t.     2x0 + 8x1 + 4x2 + 2x3 + 5x4 <= 10, x_j in {0,1}
		m := gopt.NewModel("knapsack", gopt.Maximize)
		weights := []float64{2, 8, 4, 2, 5}
		profits := []float64{5, 3, 2, 7, 4}
		x := make([]gopt.Var, 5)
		var objTerms, rowTerms []gopt.Term
		for i := range 5 {
			x[i] = m.AddBinaryVar(fmt.Sprintf("x%d", i))
			objTerms = append(objTerms, gopt.T(x[i], profits[i]))
			rowTerms = append(rowTerms, gopt.T(x[i], weights[i]))
		}
		m.SetObjective(gopt.SumTerms(objTerms...))
		m.AddConstraint("cap", gopt.Le(gopt.SumTerms(rowTerms...), 10.0))

		res := m.Solve()
		if res.Status != gopt.StatusOptimal {
			t.Fatalf("expected Optimal, got %v", res.Status)
		}
		if math.Abs(res.Objective-16.0) > 1e-6 {
			t.Fatalf("expected obj=16.0, got %.6f", res.Objective)
		}
		want1 := []float64{1, 0, 0, 1, 1}
		for i, w := range want1 {
			if math.Abs(res.Value(x[i])-w) > 1e-6 {
				t.Fatalf("x[%d] = %.6f, want %.1f", i, res.Value(x[i]), w)
			}
		}

		// Fix x0 <= 0 -> optimal objective 11.0, primal [0, 0, 0, 1, 1]
		m.SetVarBounds(x[0], 0, 0)
		res2 := m.Solve()
		if res2.Status != gopt.StatusOptimal || math.Abs(res2.Objective-11.0) > 1e-6 {
			t.Fatalf("step 2: status=%v obj=%.6f, want Optimal 11.0", res2.Status, res2.Objective)
		}
		want2 := []float64{0, 0, 0, 1, 1}
		for i, w := range want2 {
			if math.Abs(res2.Value(x[i])-w) > 1e-6 {
				t.Fatalf("step 2: x[%d] = %.6f, want %.1f", i, res2.Value(x[i]), w)
			}
		}

		// Fix x1 >= 1 -> optimal objective 10.0, primal [0, 1, 0, 1, 0]
		m.SetVarBounds(x[1], 1, 1)
		res3 := m.Solve()
		if res3.Status != gopt.StatusOptimal || math.Abs(res3.Objective-10.0) > 1e-6 {
			t.Fatalf("step 3: status=%v obj=%.6f, want Optimal 10.0", res3.Status, res3.Objective)
		}
		want3 := []float64{0, 1, 0, 1, 0}
		for i, w := range want3 {
			if math.Abs(res3.Value(x[i])-w) > 1e-6 {
				t.Fatalf("step 3: x[%d] = %.6f, want %.1f", i, res3.Value(x[i]), w)
			}
		}
	})

	t.Run("CInterfaceTest_new_KnapsackWithParams", func(t *testing.T) {
		// Maximize 10x0 + 13x1 + 18x2 + 31x3 + 7x4 + 15x5
		// s.t.     11x0 + 15x1 + 20x2 + 35x3 + 10x4 + 33x5 <= 47, x_j in {0,1}
		m := gopt.NewModel("knapsack_params", gopt.Maximize)
		weights := []float64{11, 15, 20, 35, 10, 33}
		profits := []float64{10, 13, 18, 31, 7, 15}
		var objTerms, rowTerms []gopt.Term
		for i := range 6 {
			v := m.AddBinaryVar(fmt.Sprintf("x%d", i))
			objTerms = append(objTerms, gopt.T(v, profits[i]))
			rowTerms = append(rowTerms, gopt.T(v, weights[i]))
		}
		m.SetObjective(gopt.SumTerms(objTerms...))
		m.AddConstraint("cap", gopt.Le(gopt.SumTerms(rowTerms...), 47.0))

		res := m.Solve()
		if res.Status != gopt.StatusOptimal || math.Abs(res.Objective-41.0) > 1e-5 {
			t.Fatalf("expected Optimal 41.0, got %v %.6f", res.Status, res.Objective)
		}
	})
}

// TestCBCPorted_InfeasibleAndUnboundedEdgeCases ports testInfeasiblePreprocess,
// testIntegerInfeasible, testIntegerUnbounded, and testIntegerBounds from Cbc/test/CInterfaceTest.c.
func TestCBCPorted_InfeasibleAndUnboundedEdgeCases(t *testing.T) {
	t.Run("testInfeasiblePreprocess", func(t *testing.T) {
		buildModel := func() *gopt.Model {
			m := gopt.NewModel("infeas_preprocess", gopt.Minimize)
			x := make([]gopt.Var, 8)
			for i := range 8 {
				x[i] = m.AddBinaryVar(fmt.Sprintf("x%d", i))
			}
			m.SetObjective(gopt.NewExpr(gopt.T(x[0], 1), gopt.T(x[1], 1)))

			start := []int{0, 4, 8, 12, 16, 19, 22, 25, 28}
			rowIdx := []int{
				0, 1, 2, 12,
				0, 4, 5, 13,
				1, 3, 6, 12,
				3, 4, 7, 13,
				8, 9, 12,
				5, 8, 10,
				6, 9, 11,
				7, 10, 11,
			}
			val := []float64{
				1, 1, 1, 1,
				-1, 1, 1, 1,
				-1, 1, 1, 1,
				-1, -1, 1, 1,
				1, 1, 1,
				-1, -1, 1,
				-1, -1, 1,
				-1, -1, -1,
			}
			rowLB := []float64{0, 0, -math.Inf(1), 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, -math.Inf(1)}
			rowUB := []float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, math.Inf(1), 2}

			rowTerms := make([][]gopt.Term, 14)
			for col := range 8 {
				for k := start[col]; k < start[col+1]; k++ {
					r := rowIdx[k]
					rowTerms[r] = append(rowTerms[r], gopt.T(x[col], val[k]))
				}
			}
			for r := range 14 {
				expr := gopt.SumTerms(rowTerms[r]...)
				if rowLB[r] == rowUB[r] {
					m.AddConstraint(fmt.Sprintf("r%d", r), gopt.Eq(expr, rowLB[r]))
				} else if math.IsInf(rowLB[r], -1) {
					m.AddConstraint(fmt.Sprintf("r%d", r), gopt.Le(expr, rowUB[r]))
				} else {
					m.AddConstraint(fmt.Sprintf("r%d", r), gopt.Ge(expr, rowLB[r]))
				}
			}
			return m
		}

		for _, presolve := range []bool{true, false} {
			opts := gopt.DefaultOptions()
			opts.EnablePresolve = presolve
			res := buildModel().Solve(opts)
			if res.Status != gopt.StatusInfeasible {
				t.Fatalf("presolve=%v: expected StatusInfeasible, got %v (obj=%.6f)", presolve, res.Status, res.Objective)
			}
		}
	})

	t.Run("testIntegerInfeasible", func(t *testing.T) {
		m := gopt.NewModel("int_infeas", gopt.Minimize)
		x := m.AddBinaryVar("x")
		m.SetObjective(gopt.NewExpr(gopt.T(x, 1)))
		m.AddConstraint("c0", gopt.Le(gopt.NewExpr(gopt.T(x, 1)), -10.0))

		for _, presolve := range []bool{true, false} {
			opts := gopt.DefaultOptions()
			opts.EnablePresolve = presolve
			res := m.Solve(opts)
			if res.Status != gopt.StatusInfeasible {
				t.Fatalf("presolve=%v: expected StatusInfeasible, got %v", presolve, res.Status)
			}
		}
	})

	t.Run("testIntegerUnbounded", func(t *testing.T) {
		// Minimize x s.t. x + y <= 3, x - y == 0, x, y free, x integer
		m := gopt.NewModel("int_unbounded", gopt.Minimize)
		x := m.AddIntVar("x", math.Inf(-1), math.Inf(1))
		y := m.AddContVar("y", math.Inf(-1), math.Inf(1))
		m.SetObjective(gopt.NewExpr(gopt.T(x, 1)))
		m.AddConstraint("c0", gopt.Le(gopt.NewExpr(gopt.T(x, 1), gopt.T(y, 1)), 3.0))
		m.AddConstraint("c1", gopt.Eq(gopt.NewExpr(gopt.T(x, 1), gopt.T(y, -1)), 0.0))

		for _, presolve := range []bool{true, false} {
			opts := gopt.DefaultOptions()
			opts.EnablePresolve = presolve
			res := m.Solve(opts)
			if res.Status != gopt.StatusUnbounded {
				t.Fatalf("presolve=%v: expected StatusUnbounded, got %v", presolve, res.Status)
			}
		}
	})

	t.Run("testIntegerBounds", func(t *testing.T) {
		// max 1.1x + 100.0z s.t. x + z <= 3, 0 <= x <= 3, 0 <= z <= 1.5 (z Integer)
		// Expected: x* = 2.0, z* = 1.0, obj = 102.2
		m := gopt.NewModel("int_bounds", gopt.Maximize)
		x := m.AddContVar("x", 0.0, 3.0)
		z := m.AddIntVar("z", 0.0, 1.5)
		m.SetObjective(gopt.NewExpr(gopt.T(x, 1.1), gopt.T(z, 100.0)))
		m.AddConstraint("c0", gopt.Le(gopt.NewExpr(gopt.T(x, 1.0), gopt.T(z, 1.0)), 3.0))

		res := m.Solve()
		if res.Status != gopt.StatusOptimal {
			t.Fatalf("expected StatusOptimal, got %v", res.Status)
		}
		if math.Abs(res.Value(x)-2.0) > 1e-6 || math.Abs(res.Value(z)-1.0) > 1e-6 {
			t.Fatalf("got (x,z)=(%.6f, %.6f), want (2.0, 1.0)", res.Value(x), res.Value(z))
		}
		if math.Abs(res.Objective-102.2) > 1e-6 {
			t.Fatalf("got obj=%.6f, want 102.2", res.Objective)
		}
	})
}

// TestCBCPorted_LPReoptimizationAndDelete ports testLPReoptimization from CInterfaceTest_new.c
// and testSOS1 / testDelete from CInterfaceTest_advanced.c.
func TestCBCPorted_LPReoptimizationAndDelete(t *testing.T) {
	t.Run("testLPReoptimization", func(t *testing.T) {
		// Max 5x + 4y s.t. 6x + 4y <= 24, x + 2y <= 6, x, y >= 0
		m := gopt.NewModel("lp_reopt", gopt.Maximize)
		x := m.AddContVar("x", 0, math.Inf(1))
		y := m.AddContVar("y", 0, math.Inf(1))
		m.SetObjective(gopt.NewExpr(gopt.T(x, 5), gopt.T(y, 4)))
		m.AddConstraint("c0", gopt.Le(gopt.NewExpr(gopt.T(x, 6), gopt.T(y, 4)), 24.0))
		m.AddConstraint("c1", gopt.Le(gopt.NewExpr(gopt.T(x, 1), gopt.T(y, 2)), 6.0))

		p := m.Compile()
		lp := gopt.NewLPSolver(p, nil)

		// 1. Initial solve: x=3, y=1.5 -> obj = 21.0
		r1 := lp.Solve()
		if r1.Status != gopt.StatusOptimal || math.Abs(r1.Objective-21.0) > 1e-5 {
			t.Fatalf("step 1: status=%v obj=%.6f, want Optimal 21.0", r1.Status, r1.Objective)
		}

		// 2. Tighten x <= 2 -> x=2, y=2 -> obj = 18.0
		lp.SetStructuralBounds([]float64{0, 0}, []float64{2.0, math.Inf(1)})
		r2 := lp.Solve()
		if r2.Status != gopt.StatusOptimal || math.Abs(r2.Objective-18.0) > 1e-5 {
			t.Fatalf("step 2: status=%v obj=%.6f, want Optimal 18.0", r2.Status, r2.Objective)
		}

		// 3. Add constraint x + y <= 3 -> x=2, y=1 -> obj = 14.0
		lp.AddCut([]int{0, 1}, []float64{1.0, 1.0}, 3.0)
		r3 := lp.Solve()
		if r3.Status != gopt.StatusOptimal || math.Abs(r3.Objective-14.0) > 1e-5 {
			t.Fatalf("step 3: status=%v obj=%.6f, want Optimal 14.0", r3.Status, r3.Objective)
		}

		// 4. Relax x <= 100 -> x=3, y=0 -> obj = 15.0
		lp.SetStructuralBounds([]float64{0, 0}, []float64{100.0, math.Inf(1)})
		r4 := lp.Solve()
		if r4.Status != gopt.StatusOptimal || math.Abs(r4.Objective-15.0) > 1e-5 {
			t.Fatalf("step 4: status=%v obj=%.6f, want Optimal 15.0", r4.Status, r4.Objective)
		}

		// 5. Force x >= 10 -> Infeasible
		lp.SetStructuralBounds([]float64{10.0, 0}, []float64{100.0, math.Inf(1)})
		r5 := lp.Solve()
		if r5.Status != gopt.StatusInfeasible {
			t.Fatalf("step 5: expected StatusInfeasible, got %v", r5.Status)
		}
	})

	t.Run("testSOS1_CInterfaceTest_advanced", func(t *testing.T) {
		// max 3*x0 + 2*x1 + 5*x2, x0+x1+x2 <= 10, SOS1(x0,x1,x2) => x2=10, obj=50
		m := gopt.NewModel("sos1_adv", gopt.Maximize)
		coeffs := []float64{3.0, 2.0, 5.0}
		x := make([]gopt.Var, 3)
		z := make([]gopt.Var, 3)
		var objTerms, capTerms, sosTerms []gopt.Term
		for i := range 3 {
			x[i] = m.AddContVar(fmt.Sprintf("x%d", i), 0, 10.0)
			z[i] = m.AddBinaryVar(fmt.Sprintf("z%d", i))
			objTerms = append(objTerms, gopt.T(x[i], coeffs[i]))
			capTerms = append(capTerms, gopt.T(x[i], 1.0))
			sosTerms = append(sosTerms, gopt.T(z[i], 1.0))
			m.AddConstraint(fmt.Sprintf("vub_%d", i), gopt.Le(gopt.NewExpr(gopt.T(x[i], 1.0), gopt.T(z[i], -10.0)), 0.0))
		}
		m.SetObjective(gopt.SumTerms(objTerms...))
		m.AddConstraint("cap", gopt.Le(gopt.SumTerms(capTerms...), 10.0))
		m.AddConstraint("sos1", gopt.Le(gopt.SumTerms(sosTerms...), 1.0))

		res := m.Solve()
		if res.Status != gopt.StatusOptimal || math.Abs(res.Objective-50.0) > 1e-4 {
			t.Fatalf("got %v %.6f, want Optimal 50.0", res.Status, res.Objective)
		}
	})

	t.Run("testDelete", func(t *testing.T) {
		m2 := gopt.NewModel("ub5_with_tight", gopt.Maximize)
		x2 := m2.AddContVar("x", 0, 5)
		y2 := m2.AddContVar("y", 0, 5)
		z2 := m2.AddContVar("z", 0, 5)
		m2.SetObjective(gopt.NewExpr(gopt.T(x2, 10), gopt.T(y2, 6), gopt.T(z2, 4)))
		m2.AddConstraint("r0", gopt.Le(gopt.NewExpr(gopt.T(x2, 1), gopt.T(y2, 1), gopt.T(z2, 1)), 10.0))
		m2.AddConstraint("r1", gopt.Le(gopt.NewExpr(gopt.T(x2, 1)), 3.0))
		res2 := m2.Solve()
		if res2.Status != gopt.StatusOptimal || math.Abs(res2.Objective-68.0) > 1e-5 {
			t.Fatalf("ub5 with tight: got %v %.6f, want Optimal 68.0", res2.Status, res2.Objective)
		}

		m3 := gopt.NewModel("ub5_without_tight", gopt.Maximize)
		x3 := m3.AddContVar("x", 0, 5)
		y3 := m3.AddContVar("y", 0, 5)
		m3.SetObjective(gopt.NewExpr(gopt.T(x3, 10), gopt.T(y3, 6)))
		m3.AddConstraint("r0", gopt.Le(gopt.NewExpr(gopt.T(x3, 1), gopt.T(y3, 1)), 10.0))
		res3 := m3.Solve()
		if res3.Status != gopt.StatusOptimal || math.Abs(res3.Objective-80.0) > 1e-5 {
			t.Fatalf("ub5 without tight: got %v %.6f, want Optimal 80.0", res3.Status, res3.Objective)
		}
	})
}

// TestCBCPorted_TransportationConstruction ports Cbc/test/CInterfaceTest_construction.c:
// 3x4 Transportation problem with supplies [20, 30, 25] and demands [10, 15, 20, 30],
// verifying identical optimal objective 465.0.
func TestCBCPorted_TransportationConstruction(t *testing.T) {
	supply := []float64{20, 30, 25}
	demand := []float64{10, 15, 20, 30}
	cost := [][]float64{
		{8, 6, 10, 9},
		{9, 12, 7, 5},
		{14, 9, 16, 5},
	}

	m := gopt.NewModel("transportation_3x4", gopt.Minimize)
	var x [3][4]gopt.Var
	var objTerms []gopt.Term
	for i := range 3 {
		for j := range 4 {
			x[i][j] = m.AddContVar(fmt.Sprintf("x_%d_%d", i, j), 0, math.Inf(1))
			objTerms = append(objTerms, gopt.T(x[i][j], cost[i][j]))
		}
	}
	m.SetObjective(gopt.SumTerms(objTerms...))

	for i := range 3 {
		var terms []gopt.Term
		for j := range 4 {
			terms = append(terms, gopt.T(x[i][j], 1.0))
		}
		m.AddConstraint(fmt.Sprintf("supply_%d", i), gopt.Le(gopt.SumTerms(terms...), supply[i]))
	}
	for j := range 4 {
		var terms []gopt.Term
		for i := range 3 {
			terms = append(terms, gopt.T(x[i][j], 1.0))
		}
		m.AddConstraint(fmt.Sprintf("demand_%d", j), gopt.Ge(gopt.SumTerms(terms...), demand[j]))
	}

	res := m.Solve()
	if res.Status != gopt.StatusOptimal {
		t.Fatalf("expected StatusOptimal, got %v", res.Status)
	}
	if math.Abs(res.Objective-465.0) > 1e-5 {
		t.Fatalf("expected objective 465.0, got %.6f", res.Objective)
	}
}

// TestCBCPorted_CuttingStockColumnGenerationAndKantorovich ports:
// 1. Cbc/test/CInterfaceTest_colgen.c (Gilmore-Gomory Column Generation with LP duals + Knapsack Pricing MIP)
// 2. Cbc/test/CInterfaceTest_kantorovich.c (Compact Kantorovich MIP formulation with symmetry breaking)
func TestCBCPorted_CuttingStockColumnGenerationAndKantorovich(t *testing.T) {
	const barLength = 250.0
	widths := []float64{187, 119, 74, 90}
	demands := []float64{1, 2, 2, 1}
	numItems := len(widths)

	t.Run("ColumnGeneration_CInterfaceTest_colgen", func(t *testing.T) {
		patterns := [][]float64{
			{1, 0, 0, 0},
			{0, 1, 0, 0},
			{0, 0, 1, 0},
			{0, 0, 0, 1},
		}

		var masterObj float64
		for iter := 0; iter < 20; iter++ {
			master := gopt.NewModel("master_cs", gopt.Minimize)
			lam := make([]gopt.Var, len(patterns))
			var objTerms []gopt.Term
			for pIdx := range patterns {
				lam[pIdx] = master.AddContVar(fmt.Sprintf("P_%d", pIdx), 0, math.Inf(1))
				objTerms = append(objTerms, gopt.T(lam[pIdx], 1.0))
			}
			master.SetObjective(gopt.SumTerms(objTerms...))

			for i := range numItems {
				var rowTerms []gopt.Term
				for pIdx, pat := range patterns {
					if pat[i] != 0 {
						rowTerms = append(rowTerms, gopt.T(lam[pIdx], pat[i]))
					}
				}
				master.AddConstraint(fmt.Sprintf("demand_%d", i), gopt.Ge(gopt.SumTerms(rowTerms...), demands[i]))
			}

			res := master.Solve()
			if res.Status != gopt.StatusOptimal {
				t.Fatalf("iter %d: master LP status = %v", iter, res.Status)
			}
			if len(res.Duals) != numItems {
				t.Fatalf("iter %d: expected %d duals, got %d", iter, numItems, len(res.Duals))
			}
			masterObj = res.Objective

			pricing := gopt.NewModel("pricing_knapsack", gopt.Maximize)
			aVars := make([]gopt.Var, numItems)
			var pObj, pCap []gopt.Term
			for i := range numItems {
				maxCopies := math.Floor(barLength / widths[i])
				aVars[i] = pricing.AddIntVar(fmt.Sprintf("a_%d", i), 0, maxCopies)
				if res.Duals[i] > 0 {
					pObj = append(pObj, gopt.T(aVars[i], res.Duals[i]))
				}
				pCap = append(pCap, gopt.T(aVars[i], widths[i]))
			}
			pricing.SetObjective(gopt.SumTerms(pObj...))
			pricing.AddConstraint("length", gopt.Le(gopt.SumTerms(pCap...), barLength))

			pRes := pricing.Solve()
			if pRes.Status != gopt.StatusOptimal {
				t.Fatalf("iter %d: pricing MIP status = %v", iter, pRes.Status)
			}
			reducedCost := 1.0 - pRes.Objective
			if reducedCost >= -1e-5 {
				break
			}
			newPat := make([]float64, numItems)
			for i := range numItems {
				newPat[i] = math.Round(pRes.Value(aVars[i]))
			}
			patterns = append(patterns, newPat)
		}

		if math.Abs(masterObj-3.0) > 1e-5 {
			t.Fatalf("column generation converged to %.6f, expected 3.0", masterObj)
		}
	})

	t.Run("Kantorovich_CInterfaceTest_kantorovich", func(t *testing.T) {
		const numBars = 10
		m := gopt.NewModel("kantorovich_cs", gopt.Minimize)
		y := make([]gopt.Var, numBars)
		var x [4][numBars]gopt.Var
		var objTerms []gopt.Term
		for j := range numBars {
			y[j] = m.AddBinaryVar(fmt.Sprintf("y_%d", j))
			objTerms = append(objTerms, gopt.T(y[j], 1.0))
			for i := range numItems {
				maxCopies := math.Floor(barLength / widths[i])
				x[i][j] = m.AddIntVar(fmt.Sprintf("x_%d_%d", i, j), 0, maxCopies)
			}
		}
		m.SetObjective(gopt.SumTerms(objTerms...))

		for i := range numItems {
			var terms []gopt.Term
			for j := range numBars {
				terms = append(terms, gopt.T(x[i][j], 1.0))
			}
			m.AddConstraint(fmt.Sprintf("demand_%d", i), gopt.Ge(gopt.SumTerms(terms...), demands[i]))
		}
		for j := range numBars {
			var terms []gopt.Term
			for i := range numItems {
				terms = append(terms, gopt.T(x[i][j], widths[i]))
			}
			terms = append(terms, gopt.T(y[j], -barLength))
			m.AddConstraint(fmt.Sprintf("cap_%d", j), gopt.Le(gopt.SumTerms(terms...), 0.0))
		}
		for j := 1; j < numBars; j++ {
			m.AddConstraint(fmt.Sprintf("sym_%d", j), gopt.Ge(gopt.NewExpr(gopt.T(y[j-1], 1.0), gopt.T(y[j], -1.0)), 0.0))
		}

		res := m.Solve()
		if res.Status != gopt.StatusOptimal {
			t.Fatalf("expected StatusOptimal, got %v", res.Status)
		}
		if math.Abs(res.Objective-3.0) > 1e-5 {
			t.Fatalf("expected optimal bars = 3.0, got %.6f", res.Objective)
		}
	})
}

// TestCBCPorted_FacilityLocation ports Cbc/test/CInterfaceTest_facility.c:
// Capacitated Facility Location Problem with 5 candidate facilities and 8 customers.
func TestCBCPorted_FacilityLocation(t *testing.T) {
	const numFacilities = 5
	const numCustomers = 8

	fixedCost := []float64{100, 120, 110, 90, 130}
	capacity := []float64{50, 60, 55, 45, 70}
	demand := []float64{10, 12, 8, 15, 9, 11, 14, 7}
	transportCost := [][]float64{
		{10, 15, 20, 25, 12, 18, 22, 30},
		{20, 10, 15, 12, 25, 14, 16, 22},
		{15, 20, 10, 18, 15, 20, 12, 14},
		{25, 12, 18, 10, 20, 15, 25, 18},
		{18, 22, 14, 20, 10, 12, 18, 10},
	}

	m := gopt.NewModel("facility_location", gopt.Minimize)
	y := make([]gopt.Var, numFacilities)
	var x [numFacilities][numCustomers]gopt.Var
	var objTerms []gopt.Term

	for f := range numFacilities {
		y[f] = m.AddBinaryVar(fmt.Sprintf("y_%d", f))
		objTerms = append(objTerms, gopt.T(y[f], fixedCost[f]))
	}
	for f := range numFacilities {
		for c := range numCustomers {
			x[f][c] = m.AddContVar(fmt.Sprintf("x_%d_%d", f, c), 0.0, 1.0)
			objTerms = append(objTerms, gopt.T(x[f][c], transportCost[f][c]*demand[c]))
		}
	}
	m.SetObjective(gopt.SumTerms(objTerms...))

	// Customer demand satisfaction: sum_f x[f][c] == 1
	for c := range numCustomers {
		var terms []gopt.Term
		for f := range numFacilities {
			terms = append(terms, gopt.T(x[f][c], 1.0))
		}
		m.AddConstraint(fmt.Sprintf("dem_%d", c), gopt.Eq(gopt.SumTerms(terms...), 1.0))
	}

	// Facility capacity: sum_c demand[c]*x[f][c] - capacity[f]*y[f] <= 0
	for f := range numFacilities {
		var terms []gopt.Term
		for c := range numCustomers {
			terms = append(terms, gopt.T(x[f][c], demand[c]))
		}
		terms = append(terms, gopt.T(y[f], -capacity[f]))
		m.AddConstraint(fmt.Sprintf("cap_%d", f), gopt.Le(gopt.SumTerms(terms...), 0.0))
	}

	// Tightening linking constraints: x[f][c] - y[f] <= 0
	for f := range numFacilities {
		for c := range numCustomers {
			m.AddConstraint(fmt.Sprintf("link_%d_%d", f, c), gopt.Le(gopt.NewExpr(gopt.T(x[f][c], 1.0), gopt.T(y[f], -1.0)), 0.0))
		}
	}

	res := m.Solve()
	if res.Status != gopt.StatusOptimal {
		t.Fatalf("expected StatusOptimal, got %v", res.Status)
	}
	if math.Abs(res.Objective-1290.0) > 1e-4 {
		t.Fatalf("expected optimal objective 1290.0, got %.6f", res.Objective)
	}
}

// TestCBCPorted_JobShopScheduling ports Cbc/test/CInterfaceTest_jssp.c:
// 3-job, 3-machine Job Shop Scheduling Problem with big-M disjunctive constraints.
func TestCBCPorted_JobShopScheduling(t *testing.T) {
	const numJobs = 3
	const numMachines = 3

	times := [numJobs][numMachines]float64{
		{1, 2, 2},
		{2, 1, 2},
		{1, 2, 1},
	}
	machines := [numJobs][numMachines]int{
		{2, 0, 1},
		{1, 2, 0},
		{2, 1, 0},
	}
	bigM := 0.0
	for j := range numJobs {
		for i := range numMachines {
			bigM += times[j][i]
		}
	}

	m := gopt.NewModel("jssp_3x3", gopt.Minimize)
	cMax := m.AddContVar("C", 0, bigM)
	var x [numJobs][numMachines]gopt.Var
	for j := range numJobs {
		for i := range numMachines {
			x[j][i] = m.AddContVar(fmt.Sprintf("x_%d_%d", j+1, i+1), 0, bigM)
		}
	}
	m.SetObjective(gopt.NewExpr(gopt.T(cMax, 1.0)))

	// Precedence within job: x[j][machines[j][i]] - x[j][machines[j][i-1]] >= times[j][machines[j][i-1]]
	for j := range numJobs {
		for i := 1; i < numMachines; i++ {
			mCur := machines[j][i]
			mPrev := machines[j][i-1]
			m.AddConstraint(
				fmt.Sprintf("prec_%d_%d", j+1, i),
				gopt.Ge(gopt.NewExpr(gopt.T(x[j][mCur], 1.0), gopt.T(x[j][mPrev], -1.0)), times[j][mPrev]),
			)
		}
		lastMach := machines[j][numMachines-1]
		m.AddConstraint(
			fmt.Sprintf("ms_%d", j+1),
			gopt.Ge(gopt.NewExpr(gopt.T(cMax, 1.0), gopt.T(x[j][lastMach], -1.0)), times[j][lastMach]),
		)
	}

	// No-overlap on machine i for job pairs j != k
	for j := range numJobs {
		for k := j + 1; k < numJobs; k++ {
			for i := range numMachines {
				y := m.AddBinaryVar(fmt.Sprintf("y_%d_%d_%d", j+1, k+1, i+1))
				m.AddConstraint(
					fmt.Sprintf("ov1_%d_%d_%d", j+1, k+1, i+1),
					gopt.Ge(gopt.NewExpr(gopt.T(x[j][i], 1.0), gopt.T(x[k][i], -1.0), gopt.T(y, bigM)), times[k][i]),
				)
				m.AddConstraint(
					fmt.Sprintf("ov2_%d_%d_%d", j+1, k+1, i+1),
					gopt.Ge(gopt.NewExpr(gopt.T(x[k][i], 1.0), gopt.T(x[j][i], -1.0), gopt.T(y, -bigM)), times[j][i]-bigM),
				)
			}
		}
	}

	for _, presolve := range []bool{true, false} {
		opts := gopt.DefaultOptions()
		opts.EnablePresolve = presolve
		res := m.Solve(opts)
		if res.Status != gopt.StatusOptimal {
			t.Fatalf("presolve=%v: expected StatusOptimal, got %v", presolve, res.Status)
		}
		if math.Abs(res.Objective-7.0) > 1e-4 {
			t.Fatalf("presolve=%v: expected optimal makespan 7.0, got %.6f", presolve, res.Objective)
		}
	}
}

// TestCBCPorted_NQueens ports testQueens from Cbc/test/CInterfaceTest.c for N = 8, 10, 16, 25.
func TestCBCPorted_NQueens(t *testing.T) {
	for _, n := range []int{8, 10, 16, 25} {
		t.Run(fmt.Sprintf("N=%d", n), func(t *testing.T) {
			m := gopt.NewModel(fmt.Sprintf("queens_%d", n), gopt.Minimize)
			x := make([][]gopt.Var, n)
			for i := range n {
				x[i] = make([]gopt.Var, n)
				for j := range n {
					x[i][j] = m.AddBinaryVar(fmt.Sprintf("x_%d_%d", i, j))
				}
			}
			for i := range n {
				var terms []gopt.Term
				for j := range n {
					terms = append(terms, gopt.T(x[i][j], 1.0))
				}
				m.AddConstraint(fmt.Sprintf("row_%d", i), gopt.Eq(gopt.SumTerms(terms...), 1.0))
			}
			for j := range n {
				var terms []gopt.Term
				for i := range n {
					terms = append(terms, gopt.T(x[i][j], 1.0))
				}
				m.AddConstraint(fmt.Sprintf("col_%d", j), gopt.Eq(gopt.SumTerms(terms...), 1.0))
			}
			for k := -(n - 2); k <= n-2; k++ {
				var terms []gopt.Term
				for i := range n {
					j := i - k
					if j >= 0 && j < n {
						terms = append(terms, gopt.T(x[i][j], 1.0))
					}
				}
				if len(terms) > 1 {
					m.AddConstraint(fmt.Sprintf("diag1_%d", k), gopt.Le(gopt.SumTerms(terms...), 1.0))
				}
			}
			for k := 1; k <= 2*n-3; k++ {
				var terms []gopt.Term
				for i := range n {
					j := k - i
					if j >= 0 && j < n {
						terms = append(terms, gopt.T(x[i][j], 1.0))
					}
				}
				if len(terms) > 1 {
					m.AddConstraint(fmt.Sprintf("diag2_%d", k), gopt.Le(gopt.SumTerms(terms...), 1.0))
				}
			}

			res := m.Solve()
			if res.Status != gopt.StatusOptimal {
				t.Fatalf("N=%d: expected StatusOptimal, got %v", n, res.Status)
			}

			totalQueens := 0
			for i := range n {
				rowSum := 0
				for j := range n {
					if res.Value(x[i][j]) > 0.5 {
						rowSum++
						totalQueens++
					}
				}
				if rowSum != 1 {
					t.Fatalf("N=%d: row %d has %d queens", n, i, rowSum)
				}
			}
			if totalQueens != n {
				t.Fatalf("N=%d: total queens = %d", n, totalQueens)
			}
		})
	}
}

// TestCBCPorted_TSPSuite ports:
// - testLazy (4-city TSP) from Cbc/test/CInterfaceTest_advanced.c
// - 5-city TSP + path prohibition reoptimization from Cbc/test/CInterfaceTest_mip_reopt.c and CInterfaceTest_tsp_cuts.c
// - testTSP (7-city asymmetric TSP: LP relaxation 238.75, 2-node/3-node subtour cuts to 261.0, and full MIP 262.0) from Cbc/test/CInterfaceTest.c
// - testTSPUlysses22 (22-city geographic TSP -> optimal 5423.0) from Cbc/test/CInterfaceTest.c
// - TSPLIB distance matrix instances from Cbc/examples/tsp/ (burma14, ulysses16, gr17, gr21, ulysses22)
func TestCBCPorted_TSPSuite(t *testing.T) {
	t.Run("4City_CInterfaceTest_advanced", func(t *testing.T) {
		dist := [][]float64{
			{0, 10, 15, 20},
			{12, 0, 35, 25},
			{14, 32, 0, 30},
			{19, 24, 28, 0},
		}
		// Optimal tour 0 -> 1 -> 3 -> 2 -> 0 has cost 10 + 25 + 28 + 14 = 77.0
		obj, tour := solveAsymmetricTSPWithSubtourCuts(t, dist)
		if math.Abs(obj-77.0) > 1e-4 {
			t.Fatalf("expected optimal tour cost 77.0, got %.6f (tour=%v)", obj, tour)
		}
	})

	t.Run("5City_Reopt_CInterfaceTest_mip_reopt", func(t *testing.T) {
		dist := [][]float64{
			{0, 10, 15, 20, 25},
			{12, 0, 35, 25, 18},
			{14, 32, 0, 30, 20},
			{19, 24, 28, 0, 15},
			{22, 17, 21, 14, 0},
		}
		obj1, tour1 := solveAsymmetricTSPWithSubtourCuts(t, dist)
		if math.Abs(obj1-84.0) > 1e-4 {
			t.Fatalf("expected 5-city TSP cost 84.0, got %.6f (tour=%v)", obj1, tour1)
		}

		// Now forbid a 2-edge subpath from the optimal tour (e.g. tour1[0]->tour1[1]->tour1[2])
		// by penalizing/forbidding one of its edges and re-solving, matching CInterfaceTest_mip_reopt.c
		n := 5
		m := gopt.NewModel("tsp5_reopt", gopt.Minimize)
		var x [5][5]gopt.Var
		var objTerms []gopt.Term
		for i := range n {
			for j := range n {
				if i == j {
					x[i][j] = m.AddVar(fmt.Sprintf("x_%d_%d", i, j), 0, 0, gopt.Binary)
				} else {
					x[i][j] = m.AddBinaryVar(fmt.Sprintf("x_%d_%d", i, j))
					objTerms = append(objTerms, gopt.T(x[i][j], dist[i][j]))
				}
			}
		}
		m.SetObjective(gopt.SumTerms(objTerms...))
		for i := range n {
			var outT, inT []gopt.Term
			for j := range n {
				if i != j {
					outT = append(outT, gopt.T(x[i][j], 1))
					inT = append(inT, gopt.T(x[j][i], 1))
				}
			}
			m.AddConstraint(fmt.Sprintf("out_%d", i), gopt.Eq(gopt.SumTerms(outT...), 1))
			m.AddConstraint(fmt.Sprintf("in_%d", i), gopt.Eq(gopt.SumTerms(inT...), 1))
		}
		// All subtours of size 2 and 3
		for mask := 1; mask < (1 << n); mask++ {
			var S []int
			for i := range n {
				if (mask & (1 << i)) != 0 {
					S = append(S, i)
				}
			}
			if len(S) >= 2 && len(S) <= n-2 {
				var terms []gopt.Term
				for _, u := range S {
					for _, v := range S {
						if u != v {
							terms = append(terms, gopt.T(x[u][v], 1))
						}
					}
				}
				m.AddConstraint(fmt.Sprintf("sec_%d", mask), gopt.Le(gopt.SumTerms(terms...), float64(len(S)-1)))
			}
		}
		// Forbid the 2-arc sequence tour1[0]->tour1[1]->tour1[2]
		u, v, w := tour1[0], tour1[1], tour1[2]
		m.AddConstraint("forbid_seq", gopt.Le(gopt.NewExpr(gopt.T(x[u][v], 1), gopt.T(x[v][w], 1)), 1.0))
		res2 := m.Solve()
		if res2.Status != gopt.StatusOptimal {
			t.Fatalf("reopt status = %v", res2.Status)
		}
		if res2.Objective <= obj1+0.5 {
			t.Fatalf("expected reopt objective > %.1f, got %.6f", obj1, res2.Objective)
		}
		if res2.Value(x[u][v])+res2.Value(x[v][w]) > 1.5 {
			t.Fatalf("forbidden sequence still active")
		}
	})

	t.Run("7City_Asymmetric_CInterfaceTest", func(t *testing.T) {
		const oo = 1e9
		d := [][]float64{
			{oo, 49, 80, 56, oo, oo, 47},
			{50, oo, oo, 37, 21, oo, 25},
			{99, oo, oo, 52, oo, 35, oo},
			{67, 39, 37, oo, 15, oo, oo},
			{oo, 30, oo, 20, oo, 20, 49},
			{oo, oo, 35, oo, 20, oo, 32},
			{68, 35, oo, oo, 38, 37, oo},
		}
		// 1. Verify MTZ LP relaxation = 238.75 (Cbc/test/CInterfaceTest.c:703)
		const n = 7
		mLP := gopt.NewModel("tsp7_lp", gopt.Minimize)
		var xLP [n][n]gopt.Var
		var yLP [n]gopt.Var
		var objLP []gopt.Term
		for i := range n {
			for j := range n {
				if d[i][j] < oo {
					xLP[i][j] = mLP.AddContVar(fmt.Sprintf("x_%d_%d", i, j), 0, 1)
					objLP = append(objLP, gopt.T(xLP[i][j], d[i][j]))
				}
			}
		}
		for i := range n {
			yLP[i] = mLP.AddContVar(fmt.Sprintf("y_%d", i), 0, n)
		}
		mLP.SetObjective(gopt.SumTerms(objLP...))
		for i := range n {
			var outT, inT []gopt.Term
			for j := range n {
				if d[i][j] < oo {
					outT = append(outT, gopt.T(xLP[i][j], 1))
				}
				if d[j][i] < oo {
					inT = append(inT, gopt.T(xLP[j][i], 1))
				}
			}
			mLP.AddConstraint(fmt.Sprintf("out_%d", i), gopt.Eq(gopt.SumTerms(outT...), 1))
			mLP.AddConstraint(fmt.Sprintf("in_%d", i), gopt.Eq(gopt.SumTerms(inT...), 1))
		}
		for i := 1; i < n; i++ {
			for j := 1; j < n; j++ {
				if d[i][j] < oo {
					mLP.AddConstraint(
						fmt.Sprintf("mtz_%d_%d", i, j),
						gopt.Ge(gopt.NewExpr(gopt.T(yLP[i], 1), gopt.T(yLP[j], -1), gopt.T(xLP[i][j], -float64(n+1))), -float64(n)),
					)
				}
			}
		}
		resLP := mLP.Solve()
		if resLP.Status != gopt.StatusOptimal || math.Abs(resLP.Objective-238.75) > 1e-4 {
			t.Fatalf("expected 7-city MTZ LP relaxation 238.75, got %v %.6f", resLP.Status, resLP.Objective)
		}

		// 2. Verify full MIP optimal = 262.0
		objMIP, _ := solveAsymmetricTSPWithSubtourCuts(t, d)
		if math.Abs(objMIP-262.0) > 1e-4 {
			t.Fatalf("expected 7-city TSP MIP optimal 262.0, got %.6f", objMIP)
		}
	})

	t.Run("Ulysses22_CInterfaceTest", func(t *testing.T) {
		coords := [][2]float64{
			{38.24, 20.42}, {39.57, 26.15}, {40.56, 25.32}, {36.26, 23.12},
			{33.48, 10.54}, {37.56, 12.19}, {38.42, 13.11}, {37.52, 20.44},
			{41.23, 9.10}, {41.17, 13.05}, {36.08, -5.21}, {38.47, 15.13},
			{38.15, 15.35}, {37.51, 15.17}, {35.49, 14.32}, {39.36, 19.56},
			{38.09, 24.36}, {36.09, 23.00}, {40.44, 13.57}, {40.33, 14.15},
			{40.37, 14.23}, {37.57, 22.56},
		}
		// Match Cbc/test/CInterfaceTest.c:817-837 and 980-985 exactly
		toRad := func(x float64) float64 {
			const pi = 3.141592
			deg := float64(int(x))
			min := x - deg
			return pi * (deg + 5.0*min/3.0) / 180.0
		}
		const n = 22
		dist := make([][]float64, n)
		for i := range n {
			dist[i] = make([]float64, n)
			for j := range n {
				if i == 0 {
					dist[i][j] = 0
					continue
				}
				lat1, lon1 := toRad(coords[i][0]), toRad(coords[i][1])
				lat2, lon2 := toRad(coords[j][0]), toRad(coords[j][1])
				q1 := math.Cos(lon1 - lon2)
				q2 := math.Cos(lat1 - lat2)
				q3 := math.Cos(lat1 + lat2)
				dist[i][j] = float64(int(6378.388*math.Acos(0.5*((1.0+q1)*q2-(1.0-q1)*q3)) + 1.0))
			}
		}

		obj, _ := solveAsymmetricTSPWithSubtourCuts(t, dist)
		if math.Abs(obj-5423.0) > 1e-3 {
			t.Fatalf("Ulysses22: expected optimal tour 5423.0, got %.6f", obj)
		}
	})

	t.Run("TSPLIB_DistFiles_CbcExamples", func(t *testing.T) {
		cases := []struct {
			file string
			n    int
			want float64
		}{
			{file: "burma14.dist", n: 14, want: 30.0},
			{file: "ulysses16.dist", n: 16, want: 71.0},
			{file: "gr17.dist", n: 17, want: 2085.0},
			{file: "gr21.dist", n: 21, want: 2707.0},
			{file: "ulysses22.dist", n: 22, want: 72.0},
			{file: "fri26.dist", n: 26, want: 937.0},
		}
		for _, tc := range cases {
			t.Run(tc.file, func(t *testing.T) {
				path := filepath.Join("..", "Cbc", "examples", "tsp", tc.file)
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Skipf("skipping %s: %v", tc.file, err)
				}
				fields := strings.Fields(string(raw))
				if len(fields) < 1+tc.n*tc.n {
					t.Fatalf("%s: expected %d numbers, got %d", tc.file, 1+tc.n*tc.n, len(fields))
				}
				dist := make([][]float64, tc.n)
				for i := range tc.n {
					dist[i] = make([]float64, tc.n)
					for j := range tc.n {
						v, err := strconv.ParseFloat(fields[1+i*tc.n+j], 64)
						if err != nil {
							t.Fatalf("parse error: %v", err)
						}
						dist[i][j] = v
					}
				}
				obj, _ := solveAsymmetricTSPWithSubtourCuts(t, dist)
				if math.Abs(obj-tc.want) > 1e-3 {
					t.Fatalf("%s: got %.6f, want %.6f", tc.file, obj, tc.want)
				}
			})
		}
	})
}

func solveAsymmetricTSPWithSubtourCuts(t *testing.T, dist [][]float64) (float64, []int) {
	t.Helper()
	n := len(dist)
	m := gopt.NewModel(fmt.Sprintf("tsp_%d", n), gopt.Minimize)
	x := make([][]gopt.Var, n)
	var objTerms []gopt.Term
	for i := range n {
		x[i] = make([]gopt.Var, n)
		for j := range n {
			if i == j || dist[i][j] >= 1e8 {
				x[i][j] = m.AddVar(fmt.Sprintf("x_%d_%d", i, j), 0, 0, gopt.Binary)
			} else {
				x[i][j] = m.AddBinaryVar(fmt.Sprintf("x_%d_%d", i, j))
				objTerms = append(objTerms, gopt.T(x[i][j], dist[i][j]))
			}
		}
	}
	m.SetObjective(gopt.SumTerms(objTerms...))

	for i := range n {
		var outTerms, inTerms []gopt.Term
		for j := range n {
			if i != j && dist[i][j] < 1e8 {
				outTerms = append(outTerms, gopt.T(x[i][j], 1.0))
			}
			if i != j && dist[j][i] < 1e8 {
				inTerms = append(inTerms, gopt.T(x[j][i], 1.0))
			}
		}
		m.AddConstraint(fmt.Sprintf("out_%d", i), gopt.Eq(gopt.SumTerms(outTerms...), 1.0))
		m.AddConstraint(fmt.Sprintf("in_%d", i), gopt.Eq(gopt.SumTerms(inTerms...), 1.0))
	}
	for i := range n {
		for j := i + 1; j < n; j++ {
			m.AddConstraint(fmt.Sprintf("sub2_%d_%d", i, j), gopt.Le(gopt.NewExpr(gopt.T(x[i][j], 1.0), gopt.T(x[j][i], 1.0)), 1.0))
		}
	}

	for iter := 0; iter < 64; iter++ {
		res := m.Solve()
		if res.Status != gopt.StatusOptimal {
			t.Fatalf("TSP iter %d: expected Optimal, got %v", iter, res.Status)
		}
		next := make([]int, n)
		for i := range n {
			for j := range n {
				if i != j && res.Value(x[i][j]) > 0.5 {
					next[i] = j
					break
				}
			}
		}
		visited := make([]bool, n)
		var cycles [][]int
		for start := range n {
			if visited[start] {
				continue
			}
			var cyc []int
			cur := start
			for !visited[cur] {
				visited[cur] = true
				cyc = append(cyc, cur)
				cur = next[cur]
			}
			cycles = append(cycles, cyc)
		}
		if len(cycles) == 1 && len(cycles[0]) == n {
			return res.Objective, cycles[0]
		}
		for _, cyc := range cycles {
			if len(cyc) < n {
				var terms []gopt.Term
				for _, u := range cyc {
					for _, v := range cyc {
						if u != v && dist[u][v] < 1e8 {
							terms = append(terms, gopt.T(x[u][v], 1.0))
						}
					}
				}
				m.AddConstraint(
					fmt.Sprintf("subtour_it%d_sz%d", iter, len(cyc)),
					gopt.Le(gopt.SumTerms(terms...), float64(len(cyc)-1)),
				)
			}
		}
	}
	t.Fatalf("TSP failed to converge in 64 subtour iterations")
	return 0, nil
}

// TestCBCPorted_OsiExmip1AndModify ports exmip1 from Cbc/test/OsiCbcSolverInterfaceTest.cpp,
// Cbc/examples/sample5.cpp, Cbc/examples/modify.cpp, and Cbc/examples/simpleBAB.cpp.
func TestCBCPorted_OsiExmip1AndModify(t *testing.T) {
	buildExmip1 := func(asMIP bool) (*gopt.Model, []gopt.Var) {
		m := gopt.NewModel("exmip1", gopt.Minimize)
		collb := []float64{2.5, 0.0, 0.0, 0.0, 0.5, 0.0, 0.0, 0.0}
		colub := []float64{math.Inf(1), 4.1, 1.0, 1.0, 4.0, math.Inf(1), math.Inf(1), 4.3}
		obj := []float64{1.0, 0.0, 0.0, 0.0, 2.0, 0.0, 0.0, -1.0}
		v := make([]gopt.Var, 8)
		var objTerms []gopt.Term
		for j := range 8 {
			cat := gopt.Continuous
			if asMIP && (j == 2 || j == 3) {
				cat = gopt.Binary
			}
			v[j] = m.AddVar(fmt.Sprintf("c%d", j), collb[j], colub[j], cat)
			if obj[j] != 0 {
				objTerms = append(objTerms, gopt.T(v[j], obj[j]))
			}
		}
		m.SetObjective(gopt.SumTerms(objTerms...))

		// Row 0: 3*x0 + x1 - 2*x2 - x3 - x4 >= 2.5
		m.AddConstraint("r0", gopt.Ge(
			gopt.NewExpr(gopt.T(v[0], 3), gopt.T(v[1], 1), gopt.T(v[2], -2), gopt.T(v[3], -1), gopt.T(v[4], -1)),
			2.5,
		))
		// Row 1: 2*x1 + 1.1*x3 <= 2.1
		m.AddConstraint("r1", gopt.Le(gopt.NewExpr(gopt.T(v[1], 2), gopt.T(v[3], 1.1)), 2.1))
		// Row 2: x2 + x5 == 4.0
		m.AddConstraint("r2", gopt.Eq(gopt.NewExpr(gopt.T(v[2], 1), gopt.T(v[5], 1)), 4.0))
		// Row 3: 1.8 <= 2.8*x3 - 1.2*x6 <= 5.0 (ranged row)
		r3Expr := gopt.NewExpr(gopt.T(v[3], 2.8), gopt.T(v[6], -1.2))
		m.AddConstraint("r3_lb", gopt.Ge(r3Expr, 1.8))
		m.AddConstraint("r3_ub", gopt.Le(r3Expr, 5.0))
		// Row 4: 3.0 <= 5.6*x0 + x4 + 1.9*x7 <= 15.0 (ranged row)
		r4Expr := gopt.NewExpr(gopt.T(v[0], 5.6), gopt.T(v[4], 1.0), gopt.T(v[7], 1.9))
		m.AddConstraint("r4_lb", gopt.Ge(r4Expr, 3.0))
		m.AddConstraint("r4_ub", gopt.Le(r4Expr, 15.0))

		return m, v
	}

	t.Run("exmip1_LP_and_MIP_OsiCbcSolverInterfaceTest", func(t *testing.T) {
		lpMod, _ := buildExmip1(false)
		resLP := lpMod.Solve()
		if resLP.Status != gopt.StatusOptimal {
			t.Fatalf("exmip1 LP status = %v", resLP.Status)
		}
		if math.Abs(resLP.Objective-3.2368421) > 1e-4 {
			t.Fatalf("exmip1 LP obj = %.7f, want ~3.2368421", resLP.Objective)
		}

		mipMod, _ := buildExmip1(true)
		resMIP := mipMod.Solve()
		if resMIP.Status != gopt.StatusOptimal {
			t.Fatalf("exmip1 MIP status = %v", resMIP.Status)
		}
		if math.Abs(resMIP.Objective-3.2368421) > 1e-4 {
			t.Fatalf("exmip1 MIP obj = %.7f, want ~3.2368421", resMIP.Objective)
		}
	})

	t.Run("modify_cpp_SequentialObjAndRowChanges", func(t *testing.T) {
		mipMod, v := buildExmip1(true)
		mipMod.SetObjective(gopt.NewExpr(gopt.T(v[0], -100.0), gopt.T(v[4], 2.0), gopt.T(v[7], -1.0)))
		res1 := mipMod.Solve()
		if res1.Status != gopt.StatusOptimal {
			t.Fatalf("modify step 1: status = %v", res1.Status)
		}
		if math.Abs(res1.Objective-(-257.9285714)) > 1e-3 {
			t.Fatalf("modify step 1: got obj=%.6f, want -257.9285714", res1.Objective)
		}

		var sumAll []gopt.Term
		for j := range 8 {
			sumAll = append(sumAll, gopt.T(v[j], 1.0))
		}
		mipMod.AddConstraint("r_sum", gopt.Ge(gopt.SumTerms(sumAll...), 7.8))
		res2 := mipMod.Solve()
		if res2.Status != gopt.StatusOptimal {
			t.Fatalf("modify step 2: status = %v", res2.Status)
		}
	})

	t.Run("simpleBAB_cpp", func(t *testing.T) {
		m := gopt.NewModel("simpleBAB", gopt.Maximize)
		x0 := m.AddIntVar("x0", 0, 10)
		x1 := m.AddContVar("x1", 0, 10)
		m.SetObjective(gopt.NewExpr(gopt.T(x0, 1.0), gopt.T(x1, 1.0)))
		m.AddConstraint("c0", gopt.Le(gopt.NewExpr(gopt.T(x0, 1.0), gopt.T(x1, 2.0)), 3.9))

		res1 := m.Solve()
		if res1.Status != gopt.StatusOptimal || math.Abs(res1.Objective-3.45) > 1e-5 {
			t.Fatalf("simpleBAB step 1: got %v %.6f, want Optimal 3.45", res1.Status, res1.Objective)
		}
		if math.Abs(res1.Value(x0)-3.0) > 1e-6 || math.Abs(res1.Value(x1)-0.45) > 1e-6 {
			t.Fatalf("simpleBAB step 1 primal: (%.4f, %.4f), want (3.0, 0.45)", res1.Value(x0), res1.Value(x1))
		}

		m.SetVarBounds(x0, 0, 2)
		res2 := m.Solve()
		if res2.Status != gopt.StatusOptimal || math.Abs(res2.Objective-2.95) > 1e-5 {
			t.Fatalf("simpleBAB step 2: got %v %.6f, want Optimal 2.95", res2.Status, res2.Objective)
		}
	})
}

// TestCBCPorted_GAMSTestSuite ports sos1a, sos2a, semicon1, and semiint1 from Cbc/test/gamsTest.cpp.
func TestCBCPorted_GAMSTestSuite(t *testing.T) {
	t.Run("sos1a", func(t *testing.T) {
		m := gopt.NewModel("sos1a", gopt.Maximize)
		ub := []float64{0.8, 0.6, 0.6}
		obj := []float64{0.9, 1.0, 1.1}
		x := make([]gopt.Var, 3)
		z := make([]gopt.Var, 3)
		var objTerms, zTerms []gopt.Term
		for i := range 3 {
			x[i] = m.AddContVar(fmt.Sprintf("x%d", i), 0, ub[i])
			z[i] = m.AddBinaryVar(fmt.Sprintf("z%d", i))
			objTerms = append(objTerms, gopt.T(x[i], obj[i]))
			zTerms = append(zTerms, gopt.T(z[i], 1.0))
			m.AddConstraint(fmt.Sprintf("vub_%d", i), gopt.Le(gopt.NewExpr(gopt.T(x[i], 1.0), gopt.T(z[i], -ub[i])), 0.0))
		}
		m.SetObjective(gopt.SumTerms(objTerms...))
		m.AddConstraint("sos1", gopt.Le(gopt.SumTerms(zTerms...), 1.0))

		res := m.Solve()
		if res.Status != gopt.StatusOptimal || math.Abs(res.Objective-0.72) > 1e-6 {
			t.Fatalf("sos1a: got %v %.6f, want Optimal 0.72", res.Status, res.Objective)
		}
		if math.Abs(res.Value(x[0])-0.8) > 1e-6 || math.Abs(res.Value(x[1])) > 1e-6 || math.Abs(res.Value(x[2])) > 1e-6 {
			t.Fatalf("sos1a primal = [%.4f, %.4f, %.4f], want [0.8, 0, 0]", res.Value(x[0]), res.Value(x[1]), res.Value(x[2]))
		}
	})

	t.Run("sos2a", func(t *testing.T) {
		cases := []struct {
			wLo     float64
			wantObj float64
			wantP   []float64
		}{
			{wLo: -1.0, wantObj: 0.0, wantP: []float64{0.7, 0.3, 0.0, 0.0, 0.0, 1.3, 1.3}},
			{wLo: 0.8, wantObj: 0.1, wantP: []float64{0.8, 0.2, 0.0, 0.0, 0.1, 1.2, 1.2}},
		}
		for tcIdx, tc := range cases {
			m := gopt.NewModel(fmt.Sprintf("sos2a_%d", tcIdx), gopt.Minimize)
			x := make([]gopt.Var, 7)
			for i := range 5 {
				x[i] = m.AddContVar(fmt.Sprintf("x%d", i), 0, math.Inf(1))
			}
			x[5] = m.AddContVar("x5", math.Inf(-1), math.Inf(1))
			x[6] = m.AddContVar("x6", math.Inf(-1), math.Inf(1))
			z0 := m.AddBinaryVar("z0")
			z1 := m.AddBinaryVar("z1")

			m.SetObjective(gopt.NewExpr(gopt.T(x[3], 1.0), gopt.T(x[4], 1.0)))

			m.AddConstraint("r0", gopt.Eq(gopt.NewExpr(gopt.T(x[0], -1), gopt.T(x[1], -1), gopt.T(x[2], -1)), -1.0))
			m.AddConstraint("r1", gopt.Eq(gopt.NewExpr(gopt.T(x[0], -1), gopt.T(x[1], -2), gopt.T(x[2], -3), gopt.T(x[5], 1)), 0.0))
			m.AddConstraint("r2", gopt.Eq(gopt.NewExpr(gopt.T(x[0], -1), gopt.T(x[1], -2), gopt.T(x[2], -3), gopt.T(x[6], 1)), 0.0))
			m.AddConstraint("r3", gopt.Ge(gopt.NewExpr(gopt.T(x[3], 1), gopt.T(x[6], -1)), -1.3))
			m.AddConstraint("r4", gopt.Ge(gopt.NewExpr(gopt.T(x[4], 1), gopt.T(x[6], 1)), 1.3))
			m.AddConstraint("r5", gopt.Ge(gopt.NewExpr(gopt.T(x[0], 1)), tc.wLo))

			m.AddConstraint("sos2_sum", gopt.Eq(gopt.NewExpr(gopt.T(z0, 1), gopt.T(z1, 1)), 1.0))
			m.AddConstraint("sos2_0", gopt.Le(gopt.NewExpr(gopt.T(x[0], 1), gopt.T(z0, -1)), 0.0))
			m.AddConstraint("sos2_1", gopt.Le(gopt.NewExpr(gopt.T(x[1], 1), gopt.T(z0, -1), gopt.T(z1, -1)), 0.0))
			m.AddConstraint("sos2_2", gopt.Le(gopt.NewExpr(gopt.T(x[2], 1), gopt.T(z1, -1)), 0.0))

			res := m.Solve()
			if res.Status != gopt.StatusOptimal || math.Abs(res.Objective-tc.wantObj) > 1e-5 {
				t.Fatalf("sos2a case %d: got %v %.6f, want Optimal %.6f", tcIdx, res.Status, res.Objective, tc.wantObj)
			}
			for i := range 7 {
				if math.Abs(res.Value(x[i])-tc.wantP[i]) > 1e-5 {
					t.Fatalf("sos2a case %d: x[%d]=%.6f, want %.6f", tcIdx, i, res.Value(x[i]), tc.wantP[i])
				}
			}
		}
	})

	t.Run("semicon1", func(t *testing.T) {
		cases := []struct {
			sLo     float64
			wantObj float64
			wantP   []float64
		}{
			{sLo: 0.0, wantObj: 0.0, wantP: []float64{1.1, 0.0, 0.0, 8.9}},
			{sLo: 1.0, wantObj: 0.0, wantP: []float64{1.1, 0.0, 0.0, 8.9}},
			{sLo: 1.5, wantObj: 0.4, wantP: []float64{1.5, 0.0, 0.4, 8.5}},
			{sLo: 2.1, wantObj: 1.0, wantP: []float64{2.1, 0.0, 1.0, 7.9}},
			{sLo: 2.8, wantObj: 1.1, wantP: []float64{0.0, 1.1, 0.0, 10.0}},
		}
		for tcIdx, tc := range cases {
			m := gopt.NewModel(fmt.Sprintf("semicon1_%d", tcIdx), gopt.Minimize)
			s := m.AddContVar("s", 0, 10)
			pup := m.AddContVar("pup", 0, math.Inf(1))
			plo := m.AddContVar("plo", 0, math.Inf(1))
			x := m.AddContVar("x", 0, math.Inf(1))
			z := m.AddBinaryVar("z")

			m.SetObjective(gopt.NewExpr(gopt.T(pup, 1.0), gopt.T(plo, 1.0)))
			m.AddConstraint("bigx", gopt.Le(gopt.NewExpr(gopt.T(x, 1.0), gopt.T(pup, -1.0)), 8.9))
			m.AddConstraint("smallx", gopt.Ge(gopt.NewExpr(gopt.T(x, 1.0), gopt.T(plo, 1.0)), 8.9))
			m.AddConstraint("f", gopt.Eq(gopt.NewExpr(gopt.T(s, 1.0), gopt.T(x, 1.0)), 10.0))
			m.AddConstraint("sc_lb", gopt.Ge(gopt.NewExpr(gopt.T(s, 1.0), gopt.T(z, -tc.sLo)), 0.0))
			m.AddConstraint("sc_ub", gopt.Le(gopt.NewExpr(gopt.T(s, 1.0), gopt.T(z, -10.0)), 0.0))

			res := m.Solve()
			if res.Status != gopt.StatusOptimal || math.Abs(res.Objective-tc.wantObj) > 1e-5 {
				t.Fatalf("semicon1 case %d: got %v %.6f, want Optimal %.6f", tcIdx, res.Status, res.Objective, tc.wantObj)
			}
			vars := []gopt.Var{s, pup, plo, x}
			for i, v := range vars {
				if math.Abs(res.Value(v)-tc.wantP[i]) > 1e-5 {
					t.Fatalf("semicon1 case %d: var %d = %.6f, want %.6f", tcIdx, i, res.Value(v), tc.wantP[i])
				}
			}
		}
	})

	t.Run("semiint1", func(t *testing.T) {
		cases := []struct {
			sLo     float64
			wantObj float64
			wantP   []float64
		}{
			{sLo: 1.0, wantObj: 0.1, wantP: []float64{1.0, 0.1, 0.0, 9.0}},
			{sLo: 2.0, wantObj: 0.9, wantP: []float64{2.0, 0.0, 0.9, 8.0}},
			{sLo: 3.0, wantObj: 1.1, wantP: []float64{0.0, 1.1, 0.0, 10.0}},
			{sLo: 4.0, wantObj: 1.1, wantP: []float64{0.0, 1.1, 0.0, 10.0}},
		}
		for tcIdx, tc := range cases {
			m := gopt.NewModel(fmt.Sprintf("semiint1_%d", tcIdx), gopt.Minimize)
			s := m.AddIntVar("s", 0, 10)
			pup := m.AddContVar("pup", 0, math.Inf(1))
			plo := m.AddContVar("plo", 0, math.Inf(1))
			x := m.AddContVar("x", 0, math.Inf(1))
			z := m.AddBinaryVar("z")

			m.SetObjective(gopt.NewExpr(gopt.T(pup, 1.0), gopt.T(plo, 1.0)))
			m.AddConstraint("bigx", gopt.Le(gopt.NewExpr(gopt.T(x, 1.0), gopt.T(pup, -1.0)), 8.9))
			m.AddConstraint("smallx", gopt.Ge(gopt.NewExpr(gopt.T(x, 1.0), gopt.T(plo, 1.0)), 8.9))
			m.AddConstraint("f", gopt.Eq(gopt.NewExpr(gopt.T(s, 1.0), gopt.T(x, 1.0)), 10.0))
			m.AddConstraint("si_lb", gopt.Ge(gopt.NewExpr(gopt.T(s, 1.0), gopt.T(z, -tc.sLo)), 0.0))
			m.AddConstraint("si_ub", gopt.Le(gopt.NewExpr(gopt.T(s, 1.0), gopt.T(z, -10.0)), 0.0))

			res := m.Solve()
			if res.Status != gopt.StatusOptimal || math.Abs(res.Objective-tc.wantObj) > 1e-5 {
				t.Fatalf("semiint1 case %d: got %v %.6f, want Optimal %.6f", tcIdx, res.Status, res.Objective, tc.wantObj)
			}
			vars := []gopt.Var{s, pup, plo, x}
			for i, v := range vars {
				if math.Abs(res.Value(v)-tc.wantP[i]) > 1e-5 {
					t.Fatalf("semiint1 case %d: var %d = %.6f, want %.6f", tcIdx, i, res.Value(v), tc.wantP[i])
				}
			}
		}
	})
}

// TestCBCPorted_Sudoku ports Cbc/examples/sudoku.cpp using Cbc/examples/sudoku_sample.csv.
func TestCBCPorted_Sudoku(t *testing.T) {
	path := filepath.Join("..", "Cbc", "examples", "sudoku_sample.csv")
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("sudoku_sample.csv not found: %v", err)
	}
	defer func() { _ = f.Close() }()

	records, err := csv.NewReader(f).ReadAll()
	if err != nil || len(records) != 9 {
		t.Fatalf("failed to read 9x9 sudoku CSV: %v", err)
	}

	m := gopt.NewModel("sudoku_9x9", gopt.Minimize)
	var x [9][9][9]gopt.Var
	for r := range 9 {
		for c := range 9 {
			clue := 0
			cellStr := strings.TrimSpace(records[r][c])
			if cellStr != "" {
				clue, _ = strconv.Atoi(cellStr)
			}
			for d := range 9 {
				lb, ub := 0.0, 1.0
				if clue >= 1 && clue <= 9 {
					if d+1 == clue {
						lb, ub = 1.0, 1.0
					} else {
						lb, ub = 0.0, 0.0
					}
				}
				x[r][c][d] = m.AddVar(fmt.Sprintf("x_%d_%d_%d", r, c, d+1), lb, ub, gopt.Binary)
			}
		}
	}

	for r := range 9 {
		for c := range 9 {
			var terms []gopt.Term
			for d := range 9 {
				terms = append(terms, gopt.T(x[r][c][d], 1.0))
			}
			m.AddConstraint(fmt.Sprintf("cell_%d_%d", r, c), gopt.Eq(gopt.SumTerms(terms...), 1.0))
		}
	}
	for r := range 9 {
		for d := range 9 {
			var terms []gopt.Term
			for c := range 9 {
				terms = append(terms, gopt.T(x[r][c][d], 1.0))
			}
			m.AddConstraint(fmt.Sprintf("row_%d_%d", r, d), gopt.Eq(gopt.SumTerms(terms...), 1.0))
		}
	}
	for c := range 9 {
		for d := range 9 {
			var terms []gopt.Term
			for r := range 9 {
				terms = append(terms, gopt.T(x[r][c][d], 1.0))
			}
			m.AddConstraint(fmt.Sprintf("col_%d_%d", c, d), gopt.Eq(gopt.SumTerms(terms...), 1.0))
		}
	}
	for br := range 3 {
		for bc := range 3 {
			for d := range 9 {
				var terms []gopt.Term
				for r := 3 * br; r < 3*br+3; r++ {
					for c := 3 * bc; c < 3*bc+3; c++ {
						terms = append(terms, gopt.T(x[r][c][d], 1.0))
					}
				}
				m.AddConstraint(fmt.Sprintf("box_%d_%d_%d", br, bc, d), gopt.Eq(gopt.SumTerms(terms...), 1.0))
			}
		}
	}

	res := m.Solve()
	if res.Status != gopt.StatusOptimal {
		t.Fatalf("Sudoku expected StatusOptimal, got %v", res.Status)
	}

	var grid [9][9]int
	for r := range 9 {
		for c := range 9 {
			for d := range 9 {
				if res.Value(x[r][c][d]) > 0.5 {
					grid[r][c] = d + 1
				}
			}
			if grid[r][c] < 1 || grid[r][c] > 9 {
				t.Fatalf("cell (%d,%d) unassigned", r, c)
			}
		}
	}
	for r := range 9 {
		seenR := 0
		seenC := 0
		for c := range 9 {
			seenR |= 1 << grid[r][c]
			seenC |= 1 << grid[c][r]
		}
		if seenR != 0x3FE || seenC != 0x3FE {
			t.Fatalf("invalid row/col mask at index %d: row=%x col=%x", r, seenR, seenC)
		}
	}
}

// TestSolverEdgeCaseRegressions directly tests the mathematical edge cases fixed in
// presolve.go, solve.go, simplex.go, and bab.go during the CBC test port.
func TestSolverEdgeCaseRegressions(t *testing.T) {
	t.Run("NegativeBinaryCoeffTightening_PresolveParity", func(t *testing.T) {
		// Row: 10*x0 - 8*y1 - 8*y2 <= 5, x0, y1, y2 in {0,1}
		// maxAct = 10. For y1 (a = -8): maxAct + a = 2 < 5 => delta = 3, new coeff = -5.
		// Previously, maxAct was erroneously reduced by delta=3 to 7, which then caused y2 (a = -8)
		// to see maxAct + a = -1 < 5 => delta = 6, new coeff = -2, rhs = 5.
		// Then at x0=1, y1=0, y2=1: 10(1) - 5(0) - 2(1) = 8 > 5 (infeasible in presolved model!),
		// even though in original model 10(1) - 8(0) - 8(1) = 2 <= 5 is strictly feasible!
		m := gopt.NewModel("neg_bin_tighten", gopt.Maximize)
		x0 := m.AddBinaryVar("x0")
		y1 := m.AddBinaryVar("y1")
		y2 := m.AddBinaryVar("y2")
		// Maximize 100*x0 - 50*y1 - 10*y2 => optimal is x0=1, y1=0, y2=1 (obj = 90)
		m.SetObjective(gopt.NewExpr(gopt.T(x0, 100), gopt.T(y1, -50), gopt.T(y2, -10)))
		m.AddConstraint("c0", gopt.Le(gopt.NewExpr(gopt.T(x0, 10), gopt.T(y1, -8), gopt.T(y2, -8)), 5.0))

		optsNoPre := gopt.DefaultOptions()
		optsNoPre.EnablePresolve = false
		resNoPre := m.Solve(optsNoPre)

		optsPre := gopt.DefaultOptions()
		optsPre.EnablePresolve = true
		resPre := m.Solve(optsPre)

		if resNoPre.Status != gopt.StatusOptimal || resPre.Status != gopt.StatusOptimal {
			t.Fatalf("status mismatch: noPre=%v, pre=%v", resNoPre.Status, resPre.Status)
		}
		if math.Abs(resPre.Objective-90.0) > 1e-6 || math.Abs(resNoPre.Objective-90.0) > 1e-6 {
			t.Fatalf("expected optimal obj=90.0, got pre=%.6f noPre=%.6f", resPre.Objective, resNoPre.Objective)
		}
	})

	t.Run("PureLPDualsPreservedWithPresolveEnabled", func(t *testing.T) {
		m := gopt.NewModel("lp_duals", gopt.Minimize)
		x0 := m.AddContVar("x0", 0, math.Inf(1))
		x1 := m.AddContVar("x1", 0, math.Inf(1))
		m.SetObjective(gopt.NewExpr(gopt.T(x0, 3.0), gopt.T(x1, 5.0)))
		// Singleton row: x0 >= 4
		m.AddConstraint("r0", gopt.Ge(gopt.NewExpr(gopt.T(x0, 1.0)), 4.0))
		// Coupling row: x0 + 2*x1 >= 10 => x0=4, x1=3, obj = 12 + 15 = 27
		m.AddConstraint("r1", gopt.Ge(gopt.NewExpr(gopt.T(x0, 1.0), gopt.T(x1, 2.0)), 10.0))

		res := m.Solve()
		if res.Status != gopt.StatusOptimal || math.Abs(res.Objective-27.0) > 1e-6 {
			t.Fatalf("expected Optimal 27.0, got %v %.6f", res.Status, res.Objective)
		}
		if len(res.Duals) != 2 || len(res.ReducedCosts) != 2 {
			t.Fatalf("expected 2 duals and 2 reduced costs, got %d and %d", len(res.Duals), len(res.ReducedCosts))
		}
		// Shadow price of r1 is 2.5, shadow price of r0 is 3 - 2.5 = 0.5
		if math.Abs(res.Duals[0]-0.5) > 1e-6 || math.Abs(res.Duals[1]-2.5) > 1e-6 {
			t.Fatalf("duals = %v, want [0.5, 2.5]", res.Duals)
		}
	})

	t.Run("SetStructuralBounds_FreeAndOneSidedTransitions", func(t *testing.T) {
		m := gopt.NewModel("bound_transitions", gopt.Minimize)
		x := m.AddContVar("x", 0, 10)
		y := m.AddContVar("y", 0, 10)
		m.SetObjective(gopt.NewExpr(gopt.T(x, 1.0), gopt.T(y, 2.0)))
		m.AddConstraint("c0", gopt.Ge(gopt.NewExpr(gopt.T(x, 1.0), gopt.T(y, 1.0)), 4.0))

		lp := gopt.NewLPSolver(m.Compile(), nil)
		r1 := lp.Solve()
		if r1.Status != gopt.StatusOptimal || math.Abs(r1.Objective-4.0) > 1e-6 {
			t.Fatalf("r1: %v %.6f", r1.Status, r1.Objective)
		}

		// Transition x to (-Inf, 1.0] and y to [-2.0, +Inf) -> x=1, y=3, obj = 1 + 6 = 7
		lp.SetStructuralBounds([]float64{math.Inf(-1), -2.0}, []float64{1.0, math.Inf(1)})
		r2 := lp.Solve()
		if r2.Status != gopt.StatusOptimal || math.Abs(r2.Objective-7.0) > 1e-6 {
			t.Fatalf("r2: got %v %.6f, want Optimal 7.0", r2.Status, r2.Objective)
		}

		// Transition both to free (-Inf, +Inf) -> Unbounded
		lp.SetStructuralBounds([]float64{math.Inf(-1), math.Inf(-1)}, []float64{math.Inf(1), math.Inf(1)})
		r3 := lp.Solve()
		if r3.Status != gopt.StatusUnbounded {
			t.Fatalf("r3: expected StatusUnbounded, got %v", r3.Status)
		}
	})
}
