package goblin_test

import (
	"fmt"
	"math"
	"strconv"
	"testing"
	"time"

	gb "github.com/paulourio/goblin"
)

type stressByteCursor struct {
	data []byte
	pos  int
}

func (c *stressByteCursor) u8() uint8 {
	if c.pos >= len(c.data) {
		return 0
	}
	b := c.data[c.pos]
	c.pos++
	return b
}

func (c *stressByteCursor) pick(n int) int {
	if n <= 1 {
		return 0
	}
	return int(c.u8()) % n
}

func (c *stressByteCursor) boolBit() bool {
	return (c.u8() & 1) == 1
}

func buildSparseCrashDegenerateLP(c *stressByteCursor, forceLarge bool) *gb.Model {
	m := 32 + c.pick(161) // [32..192]
	n := 64 + c.pick(321) // [64..384]
	if forceLarge || c.boolBit() {
		m = 64 + c.pick(129)  // [64..192] >= hyperMinM=64
		n = 260 + c.pick(125) // [260..384] >= 4*64 chunks for AVX2 ScanDualRatioCand64With
	}

	sense := gb.Minimize
	if c.boolBit() {
		sense = gb.Maximize
	}
	mdl := gb.NewModel(fmt.Sprintf("sparse_lp_%dx%d", m, n), sense)
	xs := make([]gb.Var, n)
	obj := gb.NewExpr()

	// Create boxed variables with zero-cost and tied-cost groups to induce BFRT flips and stall streaks.
	cSign := 1.0
	if sense == gb.Maximize {
		cSign = -1.0
	}
	xRef := make([]float64, n)
	for j := range n {
		ub := []float64{1, 2, 5, 10}[c.pick(4)]
		xs[j] = mdl.AddContVar("x"+strconv.Itoa(j), 0, ub)
		if c.pick(4) == 0 {
			xRef[j] = ub
		} else if c.boolBit() {
			xRef[j] = 0.5 * ub
		}
		var cost float64
		if j < n/3 {
			// Zero-cost structural columns exercise crashEqualityBasis Pass 1.
			cost = 0
		} else {
			cost = cSign * float64(1+c.pick(3))
		}
		if cost != 0 {
			obj = obj.AddTerm(xs[j], cost)
		}
	}
	mdl.SetObjective(obj)

	// Strictly enforce < 5% matrix density while placing >= 25 equality rows of degree 5..12
	// whenever dimensions permit (all m >= 64 or n >= 160 instances).
	maxNNZ := max(m*3, (m*n*47)/1000)
	usedNNZ := 0

	var numEq int
	if maxNNZ >= 25*5 && m >= 25 {
		numEq = min(m, 25+c.pick(min(20, m-24)))
	} else {
		numEq = min(m/2, maxNNZ/5)
	}

	numChunks := max(1, (n+63)/64)
	usedCol := make([]bool, n)

	for i := range numEq {
		remRows := m - i - 1
		remBudget := maxNNZ - usedNNZ - remRows*2
		if remBudget < 5 {
			break
		}
		deg := min(remBudget, 5+c.pick(8)) // degree 5..12
		e := gb.NewExpr()
		rhs := 0.0
		// Seed across all 64-column chunks so dual ratio scans span >= 4 chunks for n >= 260.
		for tIdx := range deg {
			chunk := tIdx % numChunks
			chunkBase := chunk * 64
			chunkSpan := min(64, n-chunkBase)
			col := chunkBase + (i*7+tIdx*13+c.pick(chunkSpan))%chunkSpan
			for off := 0; off < n && usedCol[col]; off++ {
				col = (col + 1) % n
			}
			if usedCol[col] {
				continue
			}
			usedCol[col] = true
			// Positive entries on Pass-2 candidate rows trigger cost-to-coeff crashing.
			a := float64(1 + c.pick(4))
			if i%3 == 0 && c.boolBit() {
				a = -a
			}
			e = e.AddTerm(xs[col], a)
			rhs += a * xRef[col]
			usedNNZ++
		}
		for _, t := range e.Terms {
			usedCol[t.Var.Index()] = false
		}
		if len(e.Terms) > 0 {
			if c.pick(4) == 0 {
				rhs = 0 // degenerate zero RHS
			}
			mdl.AddConstraint("eq"+strconv.Itoa(i), gb.Eq(e, rhs))
		}
	}

	for i := numEq; i < m; i++ {
		remRows := m - i - 1
		remBudget := maxNNZ - usedNNZ - remRows*2
		if remBudget < 2 {
			break
		}
		deg := min(remBudget, 3+c.pick(6))
		e := gb.NewExpr()
		rhs := 0.0
		for tIdx := range deg {
			chunk := tIdx % numChunks
			chunkBase := chunk * 64
			chunkSpan := min(64, n-chunkBase)
			col := chunkBase + (i*11+tIdx*17+c.pick(chunkSpan))%chunkSpan
			for off := 0; off < n && usedCol[col]; off++ {
				col = (col + 1) % n
			}
			if usedCol[col] {
				continue
			}
			usedCol[col] = true
			a := float64(c.pick(7) - 3)
			if a == 0 {
				a = 1
			}
			e = e.AddTerm(xs[col], a)
			rhs += a * xRef[col]
			usedNNZ++
		}
		for _, t := range e.Terms {
			usedCol[t.Var.Index()] = false
		}
		if len(e.Terms) == 0 {
			continue
		}
		switch c.pick(3) {
		case 0:
			mdl.AddConstraint("le"+strconv.Itoa(i), gb.Le(e, rhs))
		case 1:
			mdl.AddConstraint("ge"+strconv.Itoa(i), gb.Ge(e, rhs))
		default:
			mdl.AddConstraint("le_sl"+strconv.Itoa(i), gb.Le(e, rhs+float64(c.pick(4))))
		}
	}
	return mdl
}

func buildBealeCyclingLP(c *stressByteCursor) *gb.Model {
	sense := gb.Minimize
	cSign := 1.0
	if c.boolBit() {
		sense = gb.Maximize
		cSign = -1.0
	}
	mdl := gb.NewModel("beale_cycling", sense)
	x := make([]gb.Var, 4)
	for j := range 4 {
		x[j] = mdl.AddContVar("x"+strconv.Itoa(j), 0, math.Inf(1))
	}
	mdl.SetObjective(gb.NewExpr(
		x[0].T(cSign*-0.75),
		x[1].T(cSign*20.0),
		x[2].T(cSign*-0.5),
		x[3].T(cSign*6.0),
	))
	s0 := math.Ldexp(1.0, c.pick(7)-3)
	s1 := math.Ldexp(1.0, c.pick(7)-3)
	s2 := math.Ldexp(1.0, c.pick(7)-3)
	mdl.AddConstraint("r0", gb.Le(gb.NewExpr(x[0].T(0.25*s0), x[1].T(-8*s0), x[2].T(-s0), x[3].T(9*s0)), 0))
	mdl.AddConstraint("r1", gb.Le(gb.NewExpr(x[0].T(0.5*s1), x[1].T(-12*s1), x[2].T(-0.5*s1), x[3].T(3*s1)), 0))
	mdl.AddConstraint("r2", gb.Le(gb.NewExpr(x[2].T(s2)), s2))
	return mdl
}

func buildKleeMintyLP(c *stressByteCursor) *gb.Model {
	d := 3 + c.pick(6) // n <= 8
	sense := gb.Maximize
	cSign := 1.0
	if c.boolBit() {
		sense = gb.Minimize
		cSign = -1.0
	}
	mdl := gb.NewModel("klee_minty_"+strconv.Itoa(d), sense)
	xs := make([]gb.Var, d)
	obj := gb.NewExpr()
	for j := range d {
		ub := math.Inf(1)
		if c.boolBit() {
			ub = math.Pow(5.0, float64(d+1))
		}
		xs[j] = mdl.AddContVar("km"+strconv.Itoa(j), 0, ub)
		obj = obj.AddTerm(xs[j], cSign*math.Ldexp(1.0, d-1-j))
	}
	mdl.SetObjective(obj)
	for i := range d {
		e := gb.NewExpr()
		for k := range i {
			e = e.AddTerm(xs[k], math.Ldexp(1.0, i-k+1))
		}
		e = e.AddTerm(xs[i], 1.0)
		mdl.AddConstraint("km_r"+strconv.Itoa(i), gb.Le(e, math.Pow(5.0, float64(i+1))))
	}
	return mdl
}

func buildTiedTransportationLP(c *stressByteCursor) *gb.Model {
	nSrc := 8 + c.pick(13)  // [8..20]
	nDst := 10 + c.pick(10) // [10..19]
	if c.boolBit() {
		// Force n = nSrc*nDst >= 260 with m = nSrc+nDst >= 38 and < 5% density.
		nSrc = 16 + c.pick(5) // [16..20]
		nDst = 17 + c.pick(3) // [17..19]
	}
	n := nSrc * nDst
	sense := gb.Minimize
	cSign := 1.0
	if c.boolBit() {
		sense = gb.Maximize
		cSign = -1.0
	}
	mdl := gb.NewModel(fmt.Sprintf("trans_%dx%d", nSrc, nDst), sense)
	xs := make([]gb.Var, n)
	obj := gb.NewExpr()
	for i := range nSrc {
		for j := range nDst {
			idx := i*nDst + j
			xs[idx] = mdl.AddContVar(fmt.Sprintf("f_%d_%d", i, j), 0, 100)
			// Tied small integer costs induce heavy dual/primal degeneracy.
			cost := float64(1 + (i+j+c.pick(2))%3)
			obj = obj.AddTerm(xs[idx], cSign*cost)
		}
	}
	mdl.SetObjective(obj)

	totalFlow := float64(nSrc * nDst * 5)
	supEach := totalFlow / float64(nSrc)
	demEach := totalFlow / float64(nDst)
	for i := range nSrc {
		e := gb.NewExpr()
		for j := range nDst {
			e = e.AddTerm(xs[i*nDst+j], 1)
		}
		mdl.AddConstraint("sup"+strconv.Itoa(i), gb.Eq(e, supEach))
	}
	for j := range nDst {
		e := gb.NewExpr()
		for i := range nSrc {
			e = e.AddTerm(xs[i*nDst+j], 1)
		}
		mdl.AddConstraint("dem"+strconv.Itoa(j), gb.Eq(e, demEach))
	}
	return mdl
}

func FuzzSparseLPStress(f *testing.F) {
	f.Add([]byte{0, 32, 200, 1, 2, 3, 4, 5, 6, 7})
	f.Add([]byte{10, 80, 210, 0, 1, 2, 3, 4, 5, 6})
	f.Add([]byte{20, 128, 250, 3, 5, 7, 11, 13, 17})
	f.Add([]byte{180, 3, 1, 2, 4, 6}) // Beale
	f.Add([]byte{181, 5, 1, 0, 2, 4}) // Klee-Minty d=8
	f.Add([]byte{182, 12, 5, 1, 3, 7})
	f.Add([]byte{183, 16, 17, 1, 2, 3, 4}) // Tied transportation n >= 260

	f.Fuzz(func(t *testing.T, data []byte) {
		c := &stressByteCursor{data: data}
		modeByte := c.u8()

		var mdl *gb.Model
		var label string
		switch {
		case modeByte < 180:
			mdl = buildSparseCrashDegenerateLP(c, modeByte&1 == 0)
			label = "sparse_crash"
		case modeByte%4 == 0:
			mdl = buildBealeCyclingLP(c)
			label = "beale"
		case modeByte%4 == 1:
			mdl = buildKleeMintyLP(c)
			label = "klee_minty"
		default:
			mdl = buildTiedTransportationLP(c)
			label = "transportation"
		}

		p, err := mdl.CompileChecked()
		if err != nil {
			t.Fatalf("[%s] CompileChecked failed: %v", label, err)
		}

		limit := 300 * time.Millisecond
		ws1 := gb.AcquireWorkspace(p.NumVars(), p.NumConstraints())
		defer gb.ReleaseWorkspace(ws1)
		lpSIMD := gb.NewLPSolver(p, ws1)
		lpSIMD.MaxIter = 20000
		lpSIMD.Deadline = time.Now().Add(limit)
		resSIMD := lpSIMD.Solve()

		ws2 := gb.AcquireWorkspace(p.NumVars(), p.NumConstraints())
		defer gb.ReleaseWorkspace(ws2)
		lpScalar := gb.NewLPSolver(p, ws2)
		lpScalar.DisableSIMD = true
		lpScalar.MaxIter = 20000
		lpScalar.Deadline = time.Now().Add(limit)
		resScalar := lpScalar.Solve()

		verifyFormalLPResult(t, label+"/simd", p, lpSIMD, resSIMD)
		verifyFormalLPResult(t, label+"/scalar", p, lpScalar, resScalar)
		resPre := verifyFormalPresolvedLP(t, label+"/presolve", p, true)

		if !isResourceLimitStatus(resSIMD.Status) && !isResourceLimitStatus(resScalar.Status) &&
			resSIMD.Status != gb.StatusNumericalError && resScalar.Status != gb.StatusNumericalError {
			if resSIMD.Status != resScalar.Status {
				t.Fatalf("[%s] SIMD status %v != Scalar status %v", label, resSIMD.Status, resScalar.Status)
			}
			if resSIMD.Status == gb.StatusOptimal {
				tol := 1e-4 * max(1.0, max(math.Abs(resSIMD.Objective), math.Abs(resScalar.Objective)))
				if math.Abs(resSIMD.Objective-resScalar.Objective) > tol {
					t.Fatalf("[%s] SIMD obj %.8g != Scalar obj %.8g", label, resSIMD.Objective, resScalar.Objective)
				}
			}
		}
		if !isResourceLimitStatus(resPre.Status) && !isResourceLimitStatus(resSIMD.Status) &&
			resPre.Status != gb.StatusNumericalError && resSIMD.Status != gb.StatusNumericalError {
			if resPre.Status != resSIMD.Status {
				t.Fatalf("[%s] Presolve status %v != LP status %v", label, resPre.Status, resSIMD.Status)
			}
			if resPre.Status == gb.StatusOptimal && resSIMD.Status == gb.StatusOptimal {
				tol := 1e-4 * max(1.0, max(math.Abs(resPre.Objective), math.Abs(resSIMD.Objective)))
				if math.Abs(resPre.Objective-resSIMD.Objective) > tol {
					t.Fatalf("[%s] Presolve obj %.8g != LP obj %.8g", label, resPre.Objective, resSIMD.Objective)
				}
			}
		}
	})
}

func buildMultiKnapsackMILP(c *stressByteCursor, nTarget int) *gb.Model {
	sense := gb.Maximize
	cSign := 1.0
	if c.boolBit() {
		sense = gb.Minimize
		cSign = -1.0
	}
	mdl := gb.NewModel("mk_"+strconv.Itoa(nTarget), sense)
	xs := make([]gb.Var, nTarget)
	obj := gb.NewExpr()
	for j := range nTarget {
		if j%4 == 0 && c.boolBit() {
			xs[j] = mdl.AddIntVar("mk"+strconv.Itoa(j), 0, 3)
		} else {
			xs[j] = mdl.AddBinaryVar("mk" + strconv.Itoa(j))
		}
		obj = obj.AddTerm(xs[j], cSign*float64(3+c.pick(17)))
	}
	mdl.SetObjective(obj)

	maxRowTerms := min(16, nTarget)
	nRows := max(4+c.pick(5), (nTarget+7)/8)
	stride := max(1, nTarget/nRows)
	densePrefix := min(maxRowTerms, max(stride+2, 10))
	for i := range nRows {
		e := gb.NewExpr()
		sumW := 0.0
		used := make([]bool, nTarget)
		start := (i * stride) % nTarget
		addTerm := func(col int) {
			if used[col] {
				return
			}
			used[col] = true
			w := float64(2 + (i*5+col*3+c.pick(7))%13)
			e = e.AddTerm(xs[col], w)
			_, ub := mdl.VarBounds(xs[col])
			sumW += w * ub
		}
		for off := range densePrefix {
			addTerm((start + off) % nTarget)
		}
		for off := densePrefix; off < maxRowTerms; off++ {
			addTerm((start + off*7 + c.pick(13)) % nTarget)
		}
		if len(e.Terms) == 0 {
			addTerm(0)
		}
		frac := 0.35 + float64(c.pick(4))*0.1
		rhs := math.Floor(frac*sumW) + 0.5
		if i%3 == 2 {
			mdl.AddConstraint("cov"+strconv.Itoa(i), gb.Ge(e, math.Max(2, math.Floor(0.25*sumW))))
		} else {
			mdl.AddConstraint("pack"+strconv.Itoa(i), gb.Le(e, math.Max(3, rhs)))
		}
	}
	return mdl
}

func buildCFLMILP(c *stressByteCursor, nTarget int) *gb.Model {
	var nF, nC int
	switch nTarget {
	case 32:
		nF, nC = 4, 7 // 4 + 28 = 32
	case 48:
		nF, nC = 4, 11 // 4 + 44 = 48
	case 100:
		nF, nC = 5, 19 // 5 + 95 = 100
	case 121:
		nF, nC = 11, 10 // 11 + 110 = 121
	case 160:
		nF, nC = 8, 19 // 8 + 152 = 160
	default:
		nF, nC = 10, 20 // 10 + 200 = 210
	}

	mdl := gb.NewModel("cfl_"+strconv.Itoa(nTarget), gb.Minimize)
	y := make([]gb.Var, nF)
	x := make([][]gb.Var, nF)
	obj := gb.NewExpr()
	dem := make([]float64, nC)
	totalDem := 0.0
	for j := range nC {
		dem[j] = float64(4 + c.pick(9))
		totalDem += dem[j]
	}

	useIntFlow := c.boolBit()
	for i := range nF {
		y[i] = mdl.AddBinaryVar("y" + strconv.Itoa(i))
		obj = obj.AddTerm(y[i], float64(25+c.pick(30)))
		x[i] = make([]gb.Var, nC)
		for j := range nC {
			name := fmt.Sprintf("x_%d_%d", i, j)
			if useIntFlow && j%2 == 0 {
				x[i][j] = mdl.AddIntVar(name, 0, dem[j])
			} else {
				x[i][j] = mdl.AddContVar(name, 0, dem[j])
			}
			obj = obj.AddTerm(x[i][j], float64(1+c.pick(8)))
		}
	}
	mdl.SetObjective(obj)

	for j := range nC {
		e := gb.NewExpr()
		for i := range nF {
			e = e.AddTerm(x[i][j], 1)
		}
		mdl.AddConstraint("dem"+strconv.Itoa(j), gb.Eq(e, dem[j]))
	}
	capEach := math.Ceil(totalDem/float64(max(1, nF-1))) + 2.0
	for i := range nF {
		e := gb.NewExpr(y[i].T(-capEach))
		for j := range nC {
			e = e.AddTerm(x[i][j], 1)
		}
		mdl.AddConstraint("cap"+strconv.Itoa(i), gb.Le(e, 0))
		// Add disaggregated VUB rows within maxVUBClusterLen=64.
		if i < 4 {
			for j := range min(nC, 8) {
				mdl.AddConstraint(fmt.Sprintf("vub_%d_%d", i, j), gb.Le(gb.NewExpr(x[i][j].T(1), y[i].T(-dem[j])), 0))
			}
		}
	}
	return mdl
}

func buildFixedChargeNetworkMILP(c *stressByteCursor, nTarget int) *gb.Model {
	nArcs := nTarget / 2
	nNodes := max(4, nArcs/3)
	mdl := gb.NewModel("fcnet_"+strconv.Itoa(nTarget), gb.Minimize)
	y := make([]gb.Var, nArcs)
	f := make([]gb.Var, nArcs)
	outArcs := make([][]int, nNodes)
	inArcs := make([][]int, nNodes)
	obj := gb.NewExpr()

	for e := range nArcs {
		u := e % (nNodes - 1)
		v := u + 1 + (e/(nNodes-1))%(nNodes-1-u)
		if v >= nNodes {
			v = nNodes - 1
		}
		if e < nNodes-1 {
			// Spanning backbone path ensures feasibility from node 0 to node nNodes-1.
			u = e
			v = e + 1
		}
		y[e] = mdl.AddBinaryVar("ye" + strconv.Itoa(e))
		capE := float64(15 + c.pick(16))
		f[e] = mdl.AddContVar("fe"+strconv.Itoa(e), 0, capE)
		obj = obj.AddTerm(y[e], float64(10+c.pick(20))).AddTerm(f[e], float64(1+c.pick(5)))
		mdl.AddConstraint("vub"+strconv.Itoa(e), gb.Le(gb.NewExpr(f[e].T(1), y[e].T(-capE)), 0))
		outArcs[u] = append(outArcs[u], e)
		inArcs[v] = append(inArcs[v], e)
	}
	if nTarget%2 == 1 {
		// Direct bypass continuous arc hits odd sizes such as n=121.
		bypass := mdl.AddContVar("fbypass", 0, 20)
		obj = obj.AddTerm(bypass, 50)
		ex0 := gb.NewExpr(bypass.T(1))
		for _, e := range outArcs[0] {
			ex0 = ex0.AddTerm(f[e], 1)
		}
		mdl.AddConstraint("src_bal", gb.Eq(ex0, 12))
		exLast := gb.NewExpr(bypass.T(1))
		for _, e := range inArcs[nNodes-1] {
			exLast = exLast.AddTerm(f[e], 1)
		}
		mdl.AddConstraint("snk_bal", gb.Eq(exLast, 12))
	} else {
		ex0 := gb.NewExpr()
		for _, e := range outArcs[0] {
			ex0 = ex0.AddTerm(f[e], 1)
		}
		mdl.AddConstraint("src_bal", gb.Eq(ex0, 12))
		exLast := gb.NewExpr()
		for _, e := range inArcs[nNodes-1] {
			exLast = exLast.AddTerm(f[e], 1)
		}
		mdl.AddConstraint("snk_bal", gb.Eq(exLast, 12))
	}
	mdl.SetObjective(obj)

	for v := 1; v < nNodes-1; v++ {
		if len(inArcs[v]) == 0 && len(outArcs[v]) == 0 {
			continue
		}
		bal := gb.NewExpr()
		for _, e := range inArcs[v] {
			bal = bal.AddTerm(f[e], 1)
		}
		for _, e := range outArcs[v] {
			bal = bal.AddTerm(f[e], -1)
		}
		mdl.AddConstraint("bal"+strconv.Itoa(v), gb.Eq(bal, 0))
	}
	return mdl
}

func buildLotSizingVUBChainMILP(c *stressByteCursor, nTarget int) *gb.Model {
	mdl := gb.NewModel("lotsizing_"+strconv.Itoa(nTarget), gb.Minimize)
	obj := gb.NewExpr()

	// Build product chains of (y_t, p_t, s_t) up to maxFlowChainLen=64.
	var chainLens []int
	switch nTarget {
	case 32:
		chainLens = []int{10}
	case 48:
		chainLens = []int{16}
	case 100:
		chainLens = []int{16, 17}
	case 121:
		chainLens = []int{40}
	case 160:
		chainLens = []int{53}
	default:
		if c.boolBit() {
			chainLens = []int{64, 6} // hits exact maxFlowChainLen=64 + multi-product coupling (210 vars)
		} else {
			chainLens = []int{35, 35}
		}
	}

	usedVars := 0
	var allProd [][]gb.Var
	for prodIdx, L := range chainLens {
		ps := make([]gb.Var, L)
		var prevS gb.Var
		for tIdx := range L {
			y := mdl.AddBinaryVar(fmt.Sprintf("y%d_%d", prodIdx, tIdx))
			pv := mdl.AddContVar(fmt.Sprintf("p%d_%d", prodIdx, tIdx), 0, math.Inf(1))
			sv := mdl.AddContVar(fmt.Sprintf("s%d_%d", prodIdx, tIdx), 0, math.Inf(1))
			ps[tIdx] = pv
			usedVars += 3

			capM := float64(35 + 10*((prodIdx+tIdx)%3))
			mdl.AddConstraint(fmt.Sprintf("vub%d_%d", prodIdx, tIdx), gb.Le(gb.NewExpr(pv.T(1), y.T(-capM)), 0))
			dem := float64(7 + c.pick(8))
			if tIdx == 0 {
				mdl.AddConstraint(fmt.Sprintf("bal%d_0", prodIdx), gb.Eq(gb.NewExpr(pv.T(1), sv.T(-1)), dem))
			} else {
				mdl.AddConstraint(fmt.Sprintf("bal%d_%d", prodIdx, tIdx), gb.Eq(gb.NewExpr(prevS.T(1), pv.T(1), sv.T(-1)), dem))
			}
			prevS = sv
			obj = obj.AddTerm(y, float64(14+c.pick(10))).AddTerm(sv, 0.5)
		}
		allProd = append(allProd, ps)
	}

	// Pad remaining 0..2 slots to exact nTarget with coupled binary side choices.
	for usedVars < nTarget {
		b := mdl.AddBinaryVar("pad" + strconv.Itoa(usedVars))
		obj = obj.AddTerm(b, 2)
		if len(allProd) > 0 && len(allProd[0]) > 0 {
			mdl.AddConstraint("padc"+strconv.Itoa(usedVars), gb.Le(gb.NewExpr(b.T(2), allProd[0][0].T(0.1)), 5))
		}
		usedVars++
	}
	mdl.SetObjective(obj)

	if len(allProd) >= 2 {
		minLen := min(len(allProd[0]), len(allProd[1]))
		for tIdx := range min(minLen, 6) {
			mdl.AddConstraint("joint"+strconv.Itoa(tIdx), gb.Le(gb.NewExpr(allProd[0][tIdx].T(1), allProd[1][tIdx].T(1)), 55))
		}
	}
	return mdl
}

func buildDynamicOrbitMediumMILP(c *stressByteCursor, nTarget int) *gb.Model {
	// Emergent K_5 4-coloring / K_6 5-coloring blocks triggered by fractional 5-cycles so
	// symmetry across colors breaks statically at root and dynamically re-emerges in-tree,
	// paired with disjoint S_3/S_4 assignment blocks up to nTarget.
	mdl := gb.NewModel("orbit_med_"+strconv.Itoa(nTarget), gb.Maximize)
	obj := gb.NewExpr()
	used := 0

	nV, nC := 5, 4
	if nTarget >= 48 {
		nV, nC = 6, 5
	}
	const nTriggers = 2
	var triggers [nTriggers]gb.Var
	for k := range nTriggers {
		cyc := make([]gb.Var, 5)
		w := 100.0 / float64(k+1)
		for i := range 5 {
			cyc[i] = mdl.AddBinaryVar(fmt.Sprintf("cyc%d_%d", k, i))
			obj = obj.AddTerm(cyc[i], w)
			used++
		}
		for i := range 5 {
			mdl.AddConstraint(fmt.Sprintf("ce%d_%d", k, i), gb.Le(gb.NewExpr(cyc[i].T(1), cyc[(i+1)%5].T(1)), 1))
		}
		triggers[k] = cyc[0]
	}

	x := make([][]gb.Var, nV)
	for v := range nV {
		x[v] = make([]gb.Var, nC)
		row := gb.NewExpr()
		for col := range nC {
			x[v][col] = mdl.AddBinaryVar(fmt.Sprintf("xc_%d_%d", v, col))
			row = row.AddTerm(x[v][col], 1)
			used++
		}
		mdl.AddConstraint("vcol"+strconv.Itoa(v), gb.Eq(row, 1))
	}
	shift := 0
	if c.boolBit() {
		shift = 1
	}
	for col := range nC {
		for u := range nV {
			for v := u + 1; v < nV; v++ {
				terms := []gb.Term{x[u][col].T(1), x[v][col].T(1)}
				if col < nTriggers && u == (2*col+shift)%nV && v == (2*col+shift+1)%nV {
					terms = append(terms, triggers[col].T(1))
				} else if col < nTriggers && u == 2*col && v == 2*col+1 && shift == 1 {
					terms = append(terms, triggers[col].T(1))
				}
				mdl.AddConstraint(fmt.Sprintf("edge_%d_%d_%d", u, v, col), gb.Le(gb.NewExpr(terms...), 1))
			}
		}
	}

	// Disjoint symmetric S_3 / S_4 machine blocks up to nTarget.
	blkIdx := 0
	for used+10 <= nTarget {
		gSz := 3
		if used+12 <= nTarget && blkIdx%2 == 0 {
			gSz = 4
		}
		gy := make([]gb.Var, gSz)
		blkCost := float64(5 + blkIdx + c.pick(4))
		for g := range gSz {
			gy[g] = mdl.AddBinaryVar(fmt.Sprintf("by%d_%d", blkIdx, g))
			obj = obj.AddTerm(gy[g], -blkCost)
			used++
		}
		for it := range 2 {
			sumIt := gb.NewExpr()
			for g := range gSz {
				av := mdl.AddBinaryVar(fmt.Sprintf("ba%d_%d_%d", blkIdx, it, g))
				sumIt = sumIt.AddTerm(av, 1)
				mdl.AddConstraint(fmt.Sprintf("bv_%d_%d_%d", blkIdx, it, g), gb.Le(gb.NewExpr(av.T(1), gy[g].T(-1)), 0))
				used++
			}
			mdl.AddConstraint(fmt.Sprintf("bsum_%d_%d", blkIdx, it), gb.Eq(sumIt, 1))
		}
		blkIdx++
	}

	// Pad remaining slots with fixed-parity pairs up to exact nTarget.
	for used+1 < nTarget {
		b0 := mdl.AddBinaryVar("ob" + strconv.Itoa(used))
		b1 := mdl.AddBinaryVar("ob" + strconv.Itoa(used+1))
		obj = obj.AddTerm(b0, -1).AddTerm(b1, -1)
		mdl.AddConstraint("opair"+strconv.Itoa(used), gb.Ge(gb.NewExpr(b0.T(1), b1.T(1)), 1))
		used += 2
	}
	if used < nTarget {
		b := mdl.AddBinaryVar("ob_last")
		obj = obj.AddTerm(b, -1)
	}
	mdl.SetObjective(obj)
	return mdl
}

func buildDeepNarrowTreeMILP(c *stressByteCursor, nTarget int) *gb.Model {
	// Parity-cylinder narrow plunge: 2*sum(x_0..x_{K-1}) + c0 - c1 == 2*K - 3 keeps every
	// binary variable unfixed at presolve and forces every x_j=0 child to close via domain
	// sweeps while x_j=1 plunges to exact depth K-2 >= 138 across nodeCompactStepCap=128.
	mdl := gb.NewModel("deep_tree_"+strconv.Itoa(nTarget), gb.Minimize)
	kBins := nTarget - 4
	switch nTarget {
	case 160:
		kBins = 142 + c.pick(4) // depth >= 140
	case 210:
		kBins = 148 + c.pick(8) // depth >= 146
	}
	if kBins < 6 {
		kBins = max(2, nTarget-2)
	}

	xs := make([]gb.Var, kBins)
	for j := range kBins {
		xs[j] = mdl.AddBinaryVar("d" + strconv.Itoa(j))
	}
	cBound := 0.4
	if c.boolBit() {
		cBound = 0.35
	}
	c0 := mdl.AddContVar("c0", 0, cBound)
	c1 := mdl.AddContVar("c1", 0, cBound)
	par := gb.NewExpr()
	for j := range kBins {
		par = par.AddTerm(xs[j], 2)
	}
	par = par.AddTerm(c0, 1).AddTerm(c1, -1)
	mdl.AddConstraint("par_chain", gb.Eq(par, float64(2*kBins-3)))

	obj := gb.NewExpr(c0.T(1))
	used := kBins + 2
	for used+1 < nTarget {
		b0 := mdl.AddBinaryVar("dp" + strconv.Itoa(used))
		b1 := mdl.AddBinaryVar("dp" + strconv.Itoa(used+1))
		obj = obj.AddTerm(b0, 1).AddTerm(b1, 1)
		mdl.AddConstraint("dppair"+strconv.Itoa(used), gb.Ge(gb.NewExpr(b0.T(1), b1.T(1)), 1))
		used += 2
	}
	if used < nTarget {
		b := mdl.AddBinaryVar("dp_last")
		obj = obj.AddTerm(b, 1)
	}
	mdl.SetObjective(obj)
	return mdl
}

func FuzzAdversarialMediumMILP(f *testing.F) {
	for sz := range 6 {
		for fam := range 6 {
			f.Add([]byte{byte(sz), byte(fam), byte(sz + fam), 1, 2, 3, 4, 5})
		}
	}
	f.Add([]byte{4, 5, 0, 1, 2, 3}) // n=160 deep narrow tree
	f.Add([]byte{5, 5, 1, 2, 3, 4}) // n=210 deep narrow tree
	f.Add([]byte{5, 3, 1, 0, 2, 4}) // n=210 lot-sizing chain len=64

	f.Fuzz(func(t *testing.T, data []byte) {
		c := &stressByteCursor{data: data}
		sizes := [6]int{32, 48, 100, 121, 160, 210}
		nTarget := sizes[c.pick(len(sizes))]
		fam := c.pick(6)

		var mdl *gb.Model
		var label string
		switch fam {
		case 0:
			mdl = buildMultiKnapsackMILP(c, nTarget)
			label = fmt.Sprintf("mk_%d", nTarget)
		case 1:
			mdl = buildCFLMILP(c, nTarget)
			label = fmt.Sprintf("cfl_%d", nTarget)
		case 2:
			mdl = buildFixedChargeNetworkMILP(c, nTarget)
			label = fmt.Sprintf("fcnet_%d", nTarget)
		case 3:
			mdl = buildLotSizingVUBChainMILP(c, nTarget)
			label = fmt.Sprintf("lot_%d", nTarget)
		case 4:
			mdl = buildDynamicOrbitMediumMILP(c, nTarget)
			label = fmt.Sprintf("orbit_%d", nTarget)
		default:
			mdl = buildDeepNarrowTreeMILP(c, nTarget)
			label = fmt.Sprintf("deep_%d", nTarget)
		}

		p, err := mdl.CompileChecked()
		if err != nil {
			t.Fatalf("[%s] CompileChecked failed: %v", label, err)
		}
		if p.NumVars() != nTarget {
			t.Fatalf("[%s] NumVars()=%d, want %d", label, p.NumVars(), nTarget)
		}

		maxNodes := 160
		timeLimit := 300 * time.Millisecond
		if fam == 5 {
			maxNodes = 400
			timeLimit = 500 * time.Millisecond
		} else if nTarget >= 160 {
			maxNodes = 120
		}
		workers := [3]int{1, 2, 4}[c.pick(3)]

		resFull, _, _ := runFullFeatureVsVIPRCheck(t, label, p, workers, maxNodes, timeLimit, 1e-6, 1e-6)

		if formalFuzzEnabled() {
			var sols [][]float64
			if resFull.Status == gb.StatusOptimal && len(resFull.Values) == p.NumVars() {
				sols = append(sols, resFull.Values)
			}
			verifyFormalSeparatedCuts(t, label+"/cuts", p, sols)
		}
	})
}
