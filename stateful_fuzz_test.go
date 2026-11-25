package goblin_test

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strconv"
	"testing"

	gb "github.com/paulourio/goblin"
)

type statefulByteCursor struct {
	data []byte
	pos  int
}

func (c *statefulByteCursor) u8() byte {
	if c.pos >= len(c.data) {
		return 0
	}
	b := c.data[c.pos]
	c.pos++
	return b
}

func (c *statefulByteCursor) u16() uint16 {
	return uint16(c.u8()) | uint16(c.u8())<<8
}

func (c *statefulByteCursor) scaledFloat(scale float64) float64 {
	raw := int16(c.u16())
	return (float64(raw) / 256.0) * scale
}

type lpRowSpec struct {
	sense gb.ConstraintSense
	cols  []int
	vals  []float64
	rhs   float64
}

type lpCutSpec struct {
	cols []int
	vals []float64
	rhs  float64
}

type lpModelSpec struct {
	sense    gb.Sense
	nVars    int
	objConst float64
	obj      []float64
	lb       []float64
	ub       []float64
	rows     []lpRowSpec
}

func (sp *lpModelSpec) buildProblem(curLB, curUB []float64, cuts []lpCutSpec) *gb.CompiledProblem {
	mdl := gb.NewModel("stateful_lp", sp.sense)
	xs := make([]gb.Var, sp.nVars)
	for j := range sp.nVars {
		xs[j] = mdl.AddContVar("x"+strconv.Itoa(j), curLB[j], curUB[j])
	}
	obj := gb.ConstExpr(sp.objConst)
	for j := range sp.nVars {
		if sp.obj[j] != 0 {
			obj = obj.AddTerm(xs[j], sp.obj[j])
		}
	}
	mdl.SetObjective(obj)
	for i, rw := range sp.rows {
		e := gb.NewExpr()
		for k, col := range rw.cols {
			e = e.AddTerm(xs[col], rw.vals[k])
		}
		name := "r" + strconv.Itoa(i)
		switch rw.sense {
		case gb.LessEqual:
			mdl.AddConstraint(name, gb.Le(e, rw.rhs))
		case gb.GreaterEqual:
			mdl.AddConstraint(name, gb.Ge(e, rw.rhs))
		default:
			mdl.AddConstraint(name, gb.Eq(e, rw.rhs))
		}
	}
	for eIdx, ct := range cuts {
		e := gb.NewExpr()
		for k, col := range ct.cols {
			e = e.AddTerm(xs[col], ct.vals[k])
		}
		mdl.AddConstraint("cut"+strconv.Itoa(eIdx), gb.Le(e, ct.rhs))
	}
	return mdl.Compile()
}

func decodeStatefulLPSpec(cur *statefulByteCursor) lpModelSpec {
	header := cur.u8()
	sense := gb.Minimize
	if header&1 != 0 {
		sense = gb.Maximize
	}
	nVars := 1 + int(cur.u8()%10)
	if header&0x70 == 0x70 {
		// Occasionally exceed 64 columns to exercise ComputeTableauRow's chunked path.
		nVars = 66 + int(cur.u8()%6)
	}
	nRows := int(cur.u8() % 11)
	if header&0x02 != 0 && nRows == 0 {
		nRows = 1
	}

	scaleMode := (header >> 2) % 3
	colScales := make([]float64, nVars)
	ruizChoices := []float64{1e-4, 1e-2, 1.0, 100.0, 1e4}
	for j := range nVars {
		switch scaleMode {
		case 1:
			colScales[j] = ruizChoices[int(cur.u8())%len(ruizChoices)]
		case 2:
			colScales[j] = []float64{1e-5, 1e-2, 1.0, 1e3, 1e7}[int(cur.u8())%5]
		default:
			colScales[j] = 1.0
		}
	}

	objConst := 0.0
	if header&0x08 != 0 {
		objConst = cur.scaledFloat(10.0)
	}
	obj := make([]float64, nVars)
	lb := make([]float64, nVars)
	ub := make([]float64, nVars)
	for j := range nVars {
		bKind := cur.u8() % 6
		c := cur.scaledFloat(4.0) / colScales[j]
		if c == 0 && bKind != 5 {
			c = float64(1+int(cur.u8()%5)) / colScales[j]
		}
		obj[j] = c
		s := colScales[j]
		switch bKind {
		case 0:
			lo := -float64(cur.u8()%6) * s
			hi := lo + float64(1+int(cur.u8()%12))*s
			lb[j], ub[j] = lo, hi
		case 1:
			lb[j], ub[j] = 0, math.Inf(1)
		case 2:
			lb[j], ub[j] = math.Inf(-1), float64(1+int(cur.u8()%8))*s
		case 3:
			lb[j], ub[j] = math.Inf(-1), math.Inf(1)
		case 4:
			v := float64(int(cur.u8()%7)-3) * s
			lb[j], ub[j] = v, v
		default:
			lb[j], ub[j] = 0, float64(2+int(cur.u8()%10))*s
		}
	}

	rows := make([]lpRowSpec, nRows)
	rowScales := []float64{1.0, 1e-3, 1e3}
	for i := range nRows {
		sByte := cur.u8()
		var rSense gb.ConstraintSense
		switch sByte % 3 {
		case 0:
			rSense = gb.LessEqual
		case 1:
			rSense = gb.GreaterEqual
		default:
			rSense = gb.Equal
		}
		rScale := 1.0
		if scaleMode != 0 {
			rScale = rowScales[int(sByte>>2)%len(rowScales)]
		}
		nnz := min(nVars, 1+int(cur.u8()%5))
		used := make([]bool, nVars)
		cols := make([]int, 0, nnz)
		vals := make([]float64, 0, nnz)
		for range nnz {
			col := int(cur.u8()) % nVars
			if used[col] {
				continue
			}
			used[col] = true
			v := cur.scaledFloat(3.0)
			if v == 0 {
				v = 1.0
			}
			cols = append(cols, col)
			vals = append(vals, (v*rScale)/colScales[col])
		}
		rhs := cur.scaledFloat(8.0) * rScale
		rows[i] = lpRowSpec{sense: rSense, cols: cols, vals: vals, rhs: rhs}
	}

	return lpModelSpec{
		sense:    sense,
		nVars:    nVars,
		objConst: objConst,
		obj:      obj,
		lb:       lb,
		ub:       ub,
		rows:     rows,
	}
}

func normalizeCutForShadow(nVars int, rawCols []int, rawVals []float64, rhs float64) (lpCutSpec, bool) {
	if math.IsInf(rhs, 1) || math.IsInf(rhs, -1) || math.IsNaN(rhs) {
		return lpCutSpec{}, false
	}
	type pair struct {
		col int
		val float64
	}
	kLim := min(len(rawCols), len(rawVals))
	pairs := make([]pair, 0, kLim)
	for k := range kLim {
		c := rawCols[k]
		if c < 0 || c >= nVars || rawVals[k] == 0 {
			continue
		}
		pairs = append(pairs, pair{col: c, val: rawVals[k]})
	}
	slices.SortFunc(pairs, func(a, b pair) int { return cmp.Compare(a.col, b.col) })
	cols := make([]int, 0, len(pairs))
	vals := make([]float64, 0, len(pairs))
	for i := 0; i < len(pairs); {
		c := pairs[i].col
		sum := pairs[i].val
		for i++; i < len(pairs) && pairs[i].col == c; i++ {
			sum += pairs[i].val
		}
		if sum != 0 {
			cols = append(cols, c)
			vals = append(vals, sum)
		}
	}
	return lpCutSpec{cols: cols, vals: vals, rhs: rhs}, true
}

func cloneLPResult(r gb.LPResult) gb.LPResult {
	r.Primal = slices.Clone(r.Primal)
	r.Dual = slices.Clone(r.Dual)
	r.ReducedCosts = slices.Clone(r.ReducedCosts)
	return r
}

func assertLPResultsBitExact(t *testing.T, label string, a, b gb.LPResult) {
	t.Helper()
	if a.Status != b.Status || a.Iterations != b.Iterations ||
		math.Float64bits(a.Objective) != math.Float64bits(b.Objective) ||
		math.Float64bits(a.ObjectiveMin) != math.Float64bits(b.ObjectiveMin) {
		t.Fatalf("[%s] dirty workspace mismatch: status=(%v,%v) iters=(%d,%d) obj=(%g,%g) min=(%g,%g)",
			label, a.Status, b.Status, a.Iterations, b.Iterations, a.Objective, b.Objective, a.ObjectiveMin, b.ObjectiveMin)
	}
	if len(a.Primal) != len(b.Primal) || len(a.Dual) != len(b.Dual) || len(a.ReducedCosts) != len(b.ReducedCosts) {
		t.Fatalf("[%s] dirty workspace vector lengths differ: primal=(%d,%d) dual=(%d,%d) rc=(%d,%d)",
			label, len(a.Primal), len(b.Primal), len(a.Dual), len(b.Dual), len(a.ReducedCosts), len(b.ReducedCosts))
	}
	for j := range a.Primal {
		if math.Float64bits(a.Primal[j]) != math.Float64bits(b.Primal[j]) {
			t.Fatalf("[%s] dirty workspace Primal[%d] bits differ: %016x != %016x (%.17g vs %.17g)",
				label, j, math.Float64bits(a.Primal[j]), math.Float64bits(b.Primal[j]), a.Primal[j], b.Primal[j])
		}
	}
	for i := range a.Dual {
		if math.Float64bits(a.Dual[i]) != math.Float64bits(b.Dual[i]) {
			t.Fatalf("[%s] dirty workspace Dual[%d] bits differ: %016x != %016x (%.17g vs %.17g)",
				label, i, math.Float64bits(a.Dual[i]), math.Float64bits(b.Dual[i]), a.Dual[i], b.Dual[i])
		}
	}
	for j := range a.ReducedCosts {
		if math.Float64bits(a.ReducedCosts[j]) != math.Float64bits(b.ReducedCosts[j]) {
			t.Fatalf("[%s] dirty workspace ReducedCosts[%d] bits differ: %016x != %016x (%.17g vs %.17g)",
				label, j, math.Float64bits(a.ReducedCosts[j]), math.Float64bits(b.ReducedCosts[j]), a.ReducedCosts[j], b.ReducedCosts[j])
		}
	}
}

func buildDirtyPartnerLP(mode uint8, nVars int) *gb.CompiledProblem {
	switch mode % 4 {
	case 0:
		// Larger-dimension feasible optimal LP with cuts.
		m := gb.NewModel("dirty_opt", gb.Maximize)
		nB := max(nVars+5, 14)
		xs := make([]gb.Var, nB)
		obj := gb.NewExpr().AddConstant(3.5)
		for j := range nB {
			xs[j] = m.AddContVar("b"+strconv.Itoa(j), -2, float64(3+j%4))
			obj = obj.AddTerm(xs[j], float64(1+(j%5))*0.75)
		}
		m.SetObjective(obj)
		for i := range nB/2 + 2 {
			j1 := (i + 1) % nB
			m.AddConstraint("bc"+strconv.Itoa(i), gb.Le(gb.NewExpr(xs[i%nB].T(2), xs[j1].T(1.5)), float64(5+i%3)))
		}
		return m.Compile()
	case 1:
		// Smaller-dimension Farkas-infeasible LP.
		m := gb.NewModel("dirty_infeas", gb.Minimize)
		x := m.AddContVar("x", 0, 10)
		y := m.AddContVar("y", 0, 10)
		m.SetObjective(gb.NewExpr(x.T(1), y.T(2)))
		m.AddConstraint("le", gb.Le(gb.NewExpr(x.T(1), y.T(1)), 2))
		m.AddConstraint("ge", gb.Ge(gb.NewExpr(x.T(1), y.T(1)), 7))
		return m.Compile()
	case 2:
		// Unbounded LP with free column.
		m := gb.NewModel("dirty_unb", gb.Minimize)
		x := m.AddContVar("x", math.Inf(-1), math.Inf(1))
		y := m.AddContVar("y", 0, math.Inf(1))
		m.SetObjective(gb.NewExpr(x.T(-3), y.T(1)))
		m.AddConstraint("eq", gb.Eq(gb.NewExpr(x.T(1), y.T(-2)), 1))
		return m.Compile()
	default:
		// Multi-row LP exercised with MaxIter=1 or SolveBoundOnly(2) to leave mid-pivot workspace dirt.
		m := gb.NewModel("dirty_iter", gb.Minimize)
		nB := max(4, nVars+2)
		xs := make([]gb.Var, nB)
		obj := gb.NewExpr()
		for j := range nB {
			xs[j] = m.AddContVar("d"+strconv.Itoa(j), 0, 10)
			obj = obj.AddTerm(xs[j], -float64(1+(j%7)))
		}
		m.SetObjective(obj)
		for i := range nB {
			m.AddConstraint("dc"+strconv.Itoa(i), gb.Le(gb.NewExpr(xs[i].T(1), xs[(i+1)%nB].T(2)), float64(4+i)))
		}
		return m.Compile()
	}
}

func assertIncrementalMatchesFreshLP(
	t *testing.T,
	label string,
	sp *lpModelSpec,
	curLB, curUB []float64,
	cuts []lpCutSpec,
	baseM int,
	lpInc *gb.LPSolver,
	resInc gb.LPResult,
) (*gb.CompiledProblem, gb.LPResult) {
	t.Helper()
	pStep := sp.buildProblem(curLB, curUB, cuts)
	lpFresh := gb.NewLPSolver(pStep, nil)
	defer lpFresh.Close()
	resFresh := lpFresh.Solve()

	verifyFormalLPResult(t, label+"/fresh", pStep, lpFresh, cloneLPResult(resFresh))
	if len(cuts) == 0 {
		verifyFormalLPResult(t, label+"/inc", pStep, lpInc, cloneLPResult(resInc))
	}

	if resInc.Status == gb.StatusUndefined || resFresh.Status == gb.StatusUndefined {
		t.Fatalf("[%s] unexpected StatusUndefined: inc=%v fresh=%v", label, resInc.Status, resFresh.Status)
	}
	if isStandardScaledProblem(pStep) && (resInc.Status == gb.StatusNumericalError || resFresh.Status == gb.StatusNumericalError) {
		if resFresh.Status == gb.StatusNumericalError {
			// Non-trivial unbounded/infeasible sub-tol exits pass verifyFormalLPResult only when certified.
			return pStep, resFresh
		}
		t.Fatalf("[%s] incremental solver returned StatusNumericalError on standard-scaled problem while fresh returned %v", label, resFresh.Status)
	}

	if resInc.Status != gb.StatusNumericalError && resFresh.Status != gb.StatusNumericalError {
		if resInc.Status != resFresh.Status {
			t.Fatalf("[%s] status mismatch: inc=%v vs fresh=%v", label, resInc.Status, resFresh.Status)
		}
	}
	if resInc.Status == gb.StatusOptimal {
		bV, rV, _ := pStep.MaxViolation(resInc.Primal, false)
		if bV > 1e-5 || rV > 1e-5 {
			t.Fatalf("[%s] incremental LP primal violation: bound=%.2e row=%.2e", label, bV, rV)
		}
		evalObj := pStep.EvaluateObjective(resInc.Primal)
		if math.Abs(evalObj-resInc.Objective) > 1e-5*max(1.0, math.Abs(resInc.Objective)) {
			t.Fatalf("[%s] incremental EvaluateObjective=%.10g != Objective=%.10g", label, evalObj, resInc.Objective)
		}
		if len(resInc.Dual) != baseM {
			t.Fatalf("[%s] incremental Dual len=%d, want baseM=%d", label, len(resInc.Dual), baseM)
		}
		if resFresh.Status == gb.StatusOptimal {
			tol := 1e-5 * max(1.0, max(math.Abs(resInc.Objective), math.Abs(resFresh.Objective)))
			if math.Abs(resInc.Objective-resFresh.Objective) > tol {
				t.Fatalf("[%s] objective mismatch: inc=%.10g vs fresh=%.10g (tol=%.2e)",
					label, resInc.Objective, resFresh.Objective, tol)
			}
		}
	}
	return pStep, resFresh
}

// FuzzLPSolverIncrementalStateMachine fuzzes stateful *gb.LPSolver and pooled *gb.Workspace
// transitions across Solve, SolveBoundOnly, SetColBound, AddCut, PurgeSlackCuts,
// ComputeTableauRow, CopyRootBasisFrom, and A->B->A workspace pool reuse.
func FuzzLPSolverIncrementalStateMachine(f *testing.F) {
	// Curated seeds covering all 8 ops, Ruiz-scaled models, inversions, cuts, and chunked rows.
	f.Add([]byte{0x00, 3, 3, 0, 10, 0x80, 0x01, 0, 0, 0, 2, 0x40, 0x01, 1, 0, 4, 2, 0x00, 0xfe, 2, 0, 1, 0, 0, 1, 0x80, 0x01, 1, 0x40, 0x01, 2, 1, 1, 0x00, 0xff, 2, 0, 1, 2, 0x80, 0x00, 8, 2, 0, 0, 3, 1, 5, 0, 4, 2, 6, 7, 1, 0})
	f.Add([]byte{0x0d, 5, 5, 0x40, 0x01, 1, 0x40, 0x01, 2, 0, 1, 0x80, 0xff, 3, 1, 2, 0x80, 0x01, 4, 2, 3, 0x80, 0xfe, 0, 0, 0, 0, 1, 0, 1, 1, 1, 1, 1, 2, 1, 2, 0, 2, 0, 2, 1, 2, 2, 10, 3, 0, 1, 2, 4, 5, 6, 7, 0, 2, 1, 4, 2, 3, 5, 0, 6, 7, 0})
	f.Add([]byte{0x74, 2, 6, 0x80, 0x01, 5, 0x80, 0xff, 1, 0x80, 0x02, 2, 0x80, 0xfe, 0, 3, 0x40, 0x01, 2, 0x40, 0x01, 3, 12, 0, 3, 5, 0, 1, 4, 2, 6, 7, 0, 2, 4, 2, 5, 0, 3, 4, 6, 7, 0})
	f.Add([]byte{0x04, 4, 0, 0x80, 0x02, 1, 0x80, 0xfe, 2, 0x40, 0x01, 3, 0x80, 0x01, 0, 9, 3, 0, 5, 1, 4, 2, 6, 7, 0, 2, 5, 3, 0})

	f.Fuzz(func(t *testing.T, data []byte) {
		cur := statefulByteCursor{data: data}
		sp := decodeStatefulLPSpec(&cur)
		curLB := slices.Clone(sp.lb)
		curUB := slices.Clone(sp.ub)
		var activeCuts []lpCutSpec

		pBase := sp.buildProblem(curLB, curUB, nil)
		baseM := pBase.NumConstraints()

		ws := gb.AcquireWorkspace(sp.nVars, baseM)
		lp := gb.NewLPSolver(pBase, ws)
		defer func() {
			if lp != nil {
				lp.Close()
			}
			if ws != nil {
				gb.ReleaseWorkspace(ws)
			}
		}()

		// Build a distinct-dimension sibling model once for CopyRootBasisFrom mismatch checks.
		mDiff := gb.NewModel("diff_dim", gb.Minimize)
		dx := mDiff.AddContVar("dx", 0, 5)
		mDiff.SetObjective(gb.NewExpr(dx.T(1)))
		mDiff.AddConstraint("dc0", gb.Ge(gb.NewExpr(dx.T(1)), 1))
		for k := range baseM + 1 {
			mDiff.AddConstraint("dc_extra"+strconv.Itoa(k), gb.Le(gb.NewExpr(dx.T(1)), float64(5+k)))
		}
		pDiff := mDiff.Compile()

		nOps := 5 + int(cur.u8()%21)
		var lastRes gb.LPResult
		hasLastSolved := false
		luCleanForPurge := false

		for step := range nOps {
			op := cur.u8() % 8
			stepTag := fmt.Sprintf("step%d_op%d", step, op)

			switch op {
			case 0: // Solve()
				resInc := lp.Solve()
				_, _ = assertIncrementalMatchesFreshLP(t, stepTag, &sp, curLB, curUB, activeCuts, baseM, lp, resInc)
				lastRes = resInc
				hasLastSolved = true
				luCleanForPurge = lp.TotalRows() > 0 && resInc.Status != gb.StatusInfeasible && resInc.Status != gb.StatusUndefined

			case 1: // SolveBoundOnly(maxIter)
				maxIters := []int{0, 1, 2, 5, 25, 200, 2000}
				maxIt := maxIters[int(cur.u8())%len(maxIters)]
				savedMaxIter := lp.MaxIter
				st, dualBnd, iters := lp.SolveBoundOnly(maxIt)
				if lp.MaxIter != savedMaxIter {
					t.Fatalf("[%s] SolveBoundOnly mutated MaxIter: got %d, want %d", stepTag, lp.MaxIter, savedMaxIter)
				}
				if st == gb.StatusUndefined {
					t.Fatalf("[%s] SolveBoundOnly returned StatusUndefined", stepTag)
				}
				if iters < 0 || (maxIt == 0 && iters != 0) {
					t.Fatalf("[%s] invalid SolveBoundOnly iters=%d for maxIter=%d", stepTag, iters, maxIt)
				}
				pStep := sp.buildProblem(curLB, curUB, activeCuts)
				lpFresh := gb.NewLPSolver(pStep, nil)
				resFresh := lpFresh.Solve()
				verifyFormalLPResult(t, stepTag+"/fresh", pStep, lpFresh, resFresh)
				lpFresh.Close()

				if isStandardScaledProblem(pStep) && st == gb.StatusNumericalError && resFresh.Status != gb.StatusNumericalError {
					t.Fatalf("[%s] SolveBoundOnly returned NumericalError while fresh returned %v", stepTag, resFresh.Status)
				}
				switch st {
				case gb.StatusOptimal:
					if math.IsNaN(dualBnd) || math.IsInf(dualBnd, 0) {
						t.Fatalf("[%s] SolveBoundOnly Optimal returned non-finite bound %v", stepTag, dualBnd)
					}
					switch resFresh.Status {
					case gb.StatusOptimal:
						tol := 1e-5 * max(1.0, max(math.Abs(dualBnd), math.Abs(resFresh.ObjectiveMin)))
						if math.Abs(dualBnd-resFresh.ObjectiveMin) > tol {
							t.Fatalf("[%s] SolveBoundOnly bound=%.10g != ObjectiveMin=%.10g", stepTag, dualBnd, resFresh.ObjectiveMin)
						}
					case gb.StatusInfeasible, gb.StatusUnbounded:
						t.Fatalf("[%s] SolveBoundOnly claimed Optimal (%.6g) while fresh returned %v", stepTag, dualBnd, resFresh.Status)
					}
				case gb.StatusIterationLimit:
					if !math.IsNaN(dualBnd) {
						switch resFresh.Status {
						case gb.StatusOptimal:
							tol := 1e-5 * max(1.0, math.Abs(resFresh.ObjectiveMin))
							if dualBnd > resFresh.ObjectiveMin+tol {
								t.Fatalf("[%s] SolveBoundOnly dual bound %.10g > ObjectiveMin %.10g", stepTag, dualBnd, resFresh.ObjectiveMin)
							}
						case gb.StatusUnbounded:
							t.Fatalf("[%s] SolveBoundOnly returned finite dual bound %.10g on unbounded LP", stepTag, dualBnd)
						}
					}
				case gb.StatusInfeasible:
					if !math.IsNaN(dualBnd) {
						t.Fatalf("[%s] SolveBoundOnly Infeasible returned non-NaN bound %v", stepTag, dualBnd)
					}
				}
				hasLastSolved = false
				luCleanForPurge = false

			case 2: // SetColBound(j, lo, hi)
				col := int(cur.u8()) % sp.nVars
				bMode := cur.u8() % 6
				lo, hi := curLB[col], curUB[col]
				switch bMode {
				case 0: // Tighten
					if math.IsInf(lo, -1) && math.IsInf(hi, 1) {
						lo, hi = -2.0, 2.0
					} else if math.IsInf(lo, -1) {
						lo = hi - 4.0
					} else if math.IsInf(hi, 1) {
						hi = lo + 4.0
					} else if lo < hi {
						span := hi - lo
						lo += 0.25 * span
						hi -= 0.25 * span
					}
				case 1: // Loosen (including infinite bounds)
					switch cur.u8() % 3 {
					case 0:
						lo, hi = math.Inf(-1), math.Inf(1)
					case 1:
						if math.IsInf(lo, -1) {
							lo = -10.0
						} else {
							lo -= 5.0
						}
						hi = math.Inf(1)
					default:
						if math.IsInf(lo, -1) {
							lo = -12.0
						} else {
							lo -= 6.0
						}
						if math.IsInf(hi, 1) {
							hi = 12.0
						} else {
							hi += 6.0
						}
					}
				case 2: // Fix
					val := 0.0
					if hasLastSolved && lastRes.Status == gb.StatusOptimal && col < len(lastRes.Primal) {
						val = math.Round(lastRes.Primal[col]*4.0) / 4.0
					} else if !math.IsInf(lo, -1) && !math.IsInf(hi, 1) {
						val = 0.5 * (lo + hi)
					} else if !math.IsInf(lo, -1) {
						val = lo
					} else if !math.IsInf(hi, 1) {
						val = hi
					}
					lo, hi = val, val
				case 3: // Unfix
					base := 0.0
					if !math.IsInf(lo, -1) {
						base = lo
					} else if !math.IsInf(hi, 1) {
						base = hi
					}
					lo, hi = base-3.0, base+3.0
				case 4: // Invert lo > hi (and subsequent SetColBound can restore feasibility)
					base := 2.0
					if !math.IsInf(lo, -1) {
						base = lo
					}
					lo, hi = base+3.0, base+1.0
				default: // Scale shift (exercises Ruiz column scaling and occasional unscaleToOriginal)
					shifts := []float64{1e-6, 1e-3, 1e4, 1e8, 1e10, 1e305}
					sc := shifts[int(cur.u8())%len(shifts)]
					lo, hi = -sc, sc
					if sc >= 1e300 {
						lo = 0
					}
				}
				curLB[col], curUB[col] = lo, hi
				lp.SetColBound(col, lo, hi)
				hasLastSolved = false
				luCleanForPurge = false

			case 3: // AddCut(cols, vals, rhs)
				if len(activeCuts) >= 16 {
					continue
				}
				kTerms := 1 + int(cur.u8()%4)
				rawCols := make([]int, 0, kTerms+2)
				rawVals := make([]float64, 0, kTerms+2)
				for range kTerms {
					c := int(cur.u8()) % sp.nVars
					v := cur.scaledFloat(2.5)
					if v == 0 {
						v = 1.0
					}
					rawCols = append(rawCols, c)
					rawVals = append(rawVals, v)
				}
				cutStyle := cur.u8() % 5
				switch cutStyle {
				case 0: // Duplicate column merging + cancellation
					c0 := rawCols[0]
					rawCols = append(rawCols, c0, c0)
					rawVals = append(rawVals, 1.5, -1.5)
				case 1: // Out-of-range / negative indices must be ignored safely
					rawCols = append(rawCols, -1, sp.nVars+2)
					rawVals = append(rawVals, 99.0, -42.0)
				}
				rhs := cur.scaledFloat(6.0)
				if hasLastSolved && lastRes.Status == gb.StatusOptimal && len(lastRes.Primal) == sp.nVars {
					act := 0.0
					for k, c := range rawCols {
						if c >= 0 && c < sp.nVars {
							act += rawVals[k] * lastRes.Primal[c]
						}
					}
					switch cutStyle {
					case 2:
						// Violated cut to force dual simplex re-optimization.
						rhs = act - 0.5
					case 3:
						// Strictly slack cut for PurgeSlackCuts coverage.
						rhs = act + 5.0
					}
				}
				if cutStyle == 4 {
					// +Inf RHS cut must be skipped cleanly; also check -Inf on a one-shot throwaway solver.
					throwLP := gb.NewLPSolver(pBase, nil)
					throwLP.AddCut(rawCols, rawVals, math.Inf(-1))
					if stInf := throwLP.Solve().Status; stInf != gb.StatusInfeasible {
						throwLP.Close()
						t.Fatalf("[%s] AddCut(-Inf) returned status=%v, want Infeasible", stepTag, stInf)
					}
					throwLP.Close()
					rhs = math.Inf(1)
				}

				beforeCuts := lp.NumCuts()
				lp.AddCut(rawCols, rawVals, rhs)
				if normCut, ok := normalizeCutForShadow(sp.nVars, rawCols, rawVals, rhs); ok {
					activeCuts = append(activeCuts, normCut)
					if lp.NumCuts() != beforeCuts+1 || lp.TotalRows() != baseM+len(activeCuts) {
						t.Fatalf("[%s] cut count mismatch after AddCut: got NumCuts=%d TotalRows=%d, want %d/%d",
							stepTag, lp.NumCuts(), lp.TotalRows(), beforeCuts+1, baseM+len(activeCuts))
					}
				} else if lp.NumCuts() != beforeCuts {
					t.Fatalf("[%s] skipped cut incremented NumCuts from %d to %d", stepTag, beforeCuts, lp.NumCuts())
				}
				hasLastSolved = false
				luCleanForPurge = false

			case 4: // PurgeSlackCuts(tol)
				tols := []float64{1e-6, 1e-4, 1e-2, 1.0}
				tol := tols[int(cur.u8())%len(tols)]
				if !luCleanForPurge {
					if dropped := lp.PurgeSlackCuts(tol); dropped != 0 {
						t.Fatalf("[%s] PurgeSlackCuts without valid LU dropped %d cuts, want 0", stepTag, dropped)
					}
					continue
				}
				if lp.NumCuts() == 0 {
					if dropped := lp.PurgeSlackCuts(tol); dropped != 0 {
						t.Fatalf("[%s] PurgeSlackCuts with 0 cuts dropped %d", stepTag, dropped)
					}
					continue
				}
				if !hasLastSolved {
					lastRes = lp.Solve()
					_, _ = assertIncrementalMatchesFreshLP(t, stepTag+"/pre", &sp, curLB, curUB, activeCuts, baseM, lp, lastRes)
					hasLastSolved = true
				}
				beforeCuts := lp.NumCuts()
				dropped := lp.PurgeSlackCuts(tol)
				afterCuts := lp.NumCuts()
				if dropped < 0 || dropped > beforeCuts || afterCuts != beforeCuts-dropped || lp.TotalRows() != baseM+afterCuts {
					t.Fatalf("[%s] invalid PurgeSlackCuts counts: before=%d dropped=%d after=%d totalRows=%d",
						stepTag, beforeCuts, dropped, afterCuts, lp.TotalRows())
				}
				if lp.TotalRows() > 0 {
					if err := gb.VerifyTableauRowForTest(lp, lp.TotalRows()-1); err != nil {
						t.Fatalf("[%s] post-purge ComputeTableauRow failed: %v", stepTag, err)
					}
				}
				resAfter := lp.Solve()
				if lastRes.Status == gb.StatusOptimal && resAfter.Status == gb.StatusOptimal {
					objTol := 1e-5 * max(1.0, max(math.Abs(lastRes.Objective), math.Abs(resAfter.Objective)))
					if math.Abs(lastRes.Objective-resAfter.Objective) > objTol {
						t.Fatalf("[%s] PurgeSlackCuts shifted optimum: before=%.10g after=%.10g",
							stepTag, lastRes.Objective, resAfter.Objective)
					}
				} else if lastRes.Status == gb.StatusOptimal && isStandardScaledProblem(sp.buildProblem(curLB, curUB, activeCuts)) && resAfter.Status != gb.StatusOptimal {
					t.Fatalf("[%s] PurgeSlackCuts altered Optimal status to %v", stepTag, resAfter.Status)
				}
				if dropped == beforeCuts {
					activeCuts = nil
				} else if dropped > 0 {
					// Re-synchronize solver with activeCuts after partial purge so subsequent bound loosening compares identical formulations.
					lp.Close()
					lp = gb.NewLPSolver(pBase, ws)
					lp.SetStructuralBounds(curLB, curUB)
					for _, ct := range activeCuts {
						lp.AddCut(ct.cols, ct.vals, ct.rhs)
					}
				}
				hasLastSolved = dropped == 0 && hasLastSolved
				luCleanForPurge = false

			case 5: // ComputeTableauRow(r)
				totRows := lp.TotalRows()
				if totRows == 0 {
					continue
				}
				r := int(cur.u8()) % totRows
				if err := gb.VerifyTableauRowForTest(lp, r); err != nil {
					t.Fatalf("[%s] VerifyTableauRowForTest(r=%d): %v", stepTag, r, err)
				}

			case 6: // CopyRootBasisFrom(other)
				// 1. Mismatched dimensions must be a safe no-op.
				otherDiff := gb.NewLPSolver(pDiff, nil)
				_ = otherDiff.Solve()
				lp.CopyRootBasisFrom(otherDiff)
				otherDiff.CopyRootBasisFrom(lp)
				otherDiff.Close()

				// 2. Sibling with cuts must not alter lp if cut counts differ.
				otherCut := gb.NewLPSolver(pBase, nil)
				otherCut.SetStructuralBounds(curLB, curUB)
				otherCut.AddCut([]int{0}, []float64{1.0}, 10.0)
				_ = otherCut.Solve()
				lp.CopyRootBasisFrom(otherCut)
				otherCut.Close()

				// 3. Scaled/unscaled root copy on matching problem when lp has 0 cuts.
				other := gb.NewLPSolver(pBase, nil)
				if cur.u8()%2 == 0 {
					// Force unscaling on sibling, then restore curLB/curUB before solving.
					other.SetColBound(0, -1e305, 1e305)
				}
				other.SetStructuralBounds(curLB, curUB)
				_ = other.Solve()
				lp.CopyRootBasisFrom(other)
				other.Close()

				resCopy := lp.Solve()
				_, _ = assertIncrementalMatchesFreshLP(t, stepTag, &sp, curLB, curUB, activeCuts, baseM, lp, resCopy)
				lastRes = resCopy
				hasLastSolved = true
				luCleanForPurge = lp.TotalRows() > 0 && resCopy.Status != gb.StatusInfeasible && resCopy.Status != gb.StatusUndefined

			case 7: // ReleaseWorkspace -> AcquireWorkspace A->B->A dirty-state reuse
				pA := sp.buildProblem(curLB, curUB, activeCuts)
				pB := buildDirtyPartnerLP(cur.u8(), sp.nVars)

				// Close lp before releasing ws so lp never retains a pooled workspace pointer.
				lp.Close()
				lp = nil

				lpA1 := gb.NewLPSolver(pA, ws)
				resA1 := lpA1.Solve()
				lpA1.Close()

				gb.ReleaseWorkspace(ws)
				ws = nil

				wsB := gb.AcquireWorkspace(pB.NumVars(), pB.NumConstraints())
				lpB := gb.NewLPSolver(pB, wsB)
				lpB.SetColBound(0, -1.5, 4.5)
				lpB.AddCut([]int{0, min(1, pB.NumVars()-1)}, []float64{1.0, -0.5}, 2.0)
				_, _, _ = lpB.SolveBoundOnly(2)
				_ = lpB.Solve()
				if lpB.TotalRows() > 0 {
					if err := gb.VerifyTableauRowForTest(lpB, 0); err != nil {
						lpB.Close()
						gb.ReleaseWorkspace(wsB)
						t.Fatalf("[%s] dirty B tableau verify failed: %v", stepTag, err)
					}
				}
				lpB.Close()
				gb.ReleaseWorkspace(wsB)

				ws = gb.AcquireWorkspace(pA.NumVars(), pA.NumConstraints())
				lpA2 := gb.NewLPSolver(pA, ws)
				resA2 := lpA2.Solve()
				assertLPResultsBitExact(t, stepTag+"/pooled_ABA", resA1, resA2)
				verifyFormalLPResult(t, stepTag+"/A2", pA, lpA2, resA2)
				lpA2.Close()

				// Also exercise direct same-pointer ws reuse from B back to A without going through sync.Pool.
				lpBDirect := gb.NewLPSolver(pB, ws)
				lpBDirect.MaxIter = 1
				_ = lpBDirect.Solve()
				lpBDirect.Close()

				lpA3 := gb.NewLPSolver(pA, ws)
				resA3 := lpA3.Solve()
				lpA3.Close()
				assertLPResultsBitExact(t, stepTag+"/direct_ABA", resA1, resA3)

				// Re-attach live incremental solver to ws with current bounds and cuts.
				lp = gb.NewLPSolver(pBase, ws)
				lp.SetStructuralBounds(curLB, curUB)
				for _, ct := range activeCuts {
					lp.AddCut(ct.cols, ct.vals, ct.rhs)
				}
				hasLastSolved = false
				luCleanForPurge = false
			}
		}

		finalRes := lp.Solve()
		_, _ = assertIncrementalMatchesFreshLP(t, "final", &sp, curLB, curUB, activeCuts, baseM, lp, finalRes)
	})
}
