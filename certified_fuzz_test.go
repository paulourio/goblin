package goblin_test

import (
	"bytes"
	"encoding/json/v2"
	"errors"
	"math"
	"math/big"
	"strconv"
	"strings"
	"testing"
	"time"

	gb "github.com/paulourio/goblin"
)

type certFuzzCursor struct {
	data []byte
	pos  int
}

func newCertFuzzCursor(data []byte) *certFuzzCursor {
	return &certFuzzCursor{data: data}
}

func (c *certFuzzCursor) u8() byte {
	if len(c.data) == 0 {
		return 0
	}
	b := c.data[c.pos%len(c.data)]
	c.pos++
	return b
}

func (c *certFuzzCursor) u64() uint64 {
	var v uint64
	for i := range 8 {
		v |= uint64(c.u8()) << (8 * i)
	}
	return v
}

func (c *certFuzzCursor) intRange(lo, hi int) int {
	if hi <= lo {
		return lo
	}
	return lo + int(c.u64()%uint64(hi-lo+1))
}

func (c *certFuzzCursor) pickFloat(choices []float64) float64 {
	if len(choices) == 0 {
		return 0
	}
	return choices[int(c.u64()%uint64(len(choices)))]
}

func buildVIPRScenario(cur *certFuzzCursor, scenario int) (*gb.CompiledProblem, []gb.Option) {
	switch scenario % 6 {
	case 0:
		// Binary knapsack MILP producing an optimal branching trace.
		m := gb.NewModel("vipr_optimal", gb.Minimize)
		n := cur.intRange(3, 5)
		xs := make([]gb.Var, n)
		objTerms := make([]gb.Term, n)
		rowTerms := make([]gb.Term, n)
		rhs := 0.0
		for j := range n {
			xs[j] = m.AddBinaryVar("x" + strconv.Itoa(j))
			w := float64(cur.intRange(2, 9))
			cost := float64(cur.intRange(-12, 12))
			if cost == 0 {
				cost = -3
			}
			objTerms[j] = gb.T(xs[j], cost)
			rowTerms[j] = gb.T(xs[j], w)
			rhs += 0.5 * w
		}
		m.AddConstraint("knap", gb.Le(gb.NewExpr(rowTerms...), math.Ceil(rhs)+1))
		if cur.u8()%2 == 0 {
			m.AddConstraint("cover", gb.Ge(gb.NewExpr(rowTerms...), math.Floor(0.4*rhs)))
		}
		m.SetObjective(gb.NewExpr(objTerms...))
		opts := []gb.Option{
			gb.WithPureGo(),
			gb.WithCertified(true),
			gb.WithRelGapTol(0),
			gb.WithAbsGapTol(0),
		}
		if cur.u8()%2 == 0 {
			opts = append(opts, gb.WithCuts(false))
		}
		return m.Compile(), opts

	case 1:
		// Infeasible integer parity or activity-conflict model.
		m := gb.NewModel("vipr_infeasible", gb.Minimize)
		if cur.u8()%2 == 0 {
			x := m.AddIntVar("x", 0, 8)
			y := m.AddIntVar("y", 0, 8)
			m.AddConstraint("parity", gb.Eq(gb.NewExpr(gb.T(x, 2), gb.T(y, 4)), 3))
			m.SetObjective(gb.NewExpr(gb.T(x, 1), gb.T(y, 1)))
		} else {
			x := m.AddIntVar("x", 2, 6)
			y := m.AddIntVar("y", 2, 6)
			m.AddConstraint("c1", gb.Le(gb.NewExpr(gb.T(x, 1), gb.T(y, 1)), 3))
			m.SetObjective(gb.NewExpr(gb.T(x, 1), gb.T(y, 2)))
		}
		return m.Compile(), []gb.Option{gb.WithPureGo(), gb.WithCertified(true)}

	case 2:
		// Cover MIP with large ObjConstant and positive RelGapTol exercising "bound" mode.
		m := gb.NewModel("vipr_bound", gb.Minimize)
		weights := []float64{5, 7, 9, 11, 13, 15}
		costs := []float64{6, 8, 10, 12, 14, 17}
		objTerms := make([]gb.Term, len(weights))
		cTerms := make([]gb.Term, len(weights))
		for i := range weights {
			v := m.AddBinaryVar("b" + strconv.Itoa(i))
			objTerms[i] = gb.T(v, costs[i])
			cTerms[i] = gb.T(v, weights[i])
		}
		m.AddConstraint("cover", gb.Ge(gb.NewExpr(cTerms...), 23))
		m.SetObjective(gb.NewExpr(objTerms...).AddConstant(1e6))
		return m.Compile(), []gb.Option{
			gb.WithPureGo(),
			gb.WithCertified(true),
			gb.WithCuts(false),
			gb.WithRelGapTol(0.5),
			gb.WithAbsGapTol(0),
		}

	case 3:
		// Symmetric K4 coloring with twin block producing presolve symBreakLe and orbitBranch.
		m := gb.NewModel("vipr_orbit", gb.Minimize)
		const numVerts = 4
		const numColors = 4
		y := make([]gb.Var, numColors)
		for c := range numColors {
			y[c] = m.AddBinaryVar("y" + strconv.Itoa(c))
		}
		x := make([][]gb.Var, numVerts)
		for v := range numVerts {
			x[v] = make([]gb.Var, numColors)
			for c := range numColors {
				x[v][c] = m.AddBinaryVar("x" + strconv.Itoa(v) + "_" + strconv.Itoa(c))
			}
		}
		for v := range numVerts {
			terms := make([]gb.Term, numColors)
			for c := range numColors {
				terms[c] = gb.T(x[v][c], 1)
			}
			m.AddConstraint("assign_"+strconv.Itoa(v), gb.Eq(gb.NewExpr(terms...), 1))
		}
		for u := range numVerts {
			for v := u + 1; v < numVerts; v++ {
				for c := range numColors {
					name := "edge_" + strconv.Itoa(u) + "_" + strconv.Itoa(v) + "_" + strconv.Itoa(c)
					m.AddConstraint(name, gb.Le(gb.NewExpr(gb.T(x[u][c], 1), gb.T(x[v][c], 1), gb.T(y[c], -1)), 0))
				}
			}
		}
		t0 := m.AddBinaryVar("t0")
		t1 := m.AddBinaryVar("t1")
		t2 := m.AddBinaryVar("t2")
		u := m.AddBinaryVar("u")
		objTerms := make([]gb.Term, 0, numColors+4)
		for c := range numColors {
			objTerms = append(objTerms, gb.T(y[c], 1))
		}
		objTerms = append(objTerms, gb.T(t0, -5), gb.T(t1, -5), gb.T(t2, -5), gb.T(u, -1))
		m.SetObjective(gb.NewExpr(objTerms...))
		m.AddConstraint("twin_cap", gb.Le(gb.NewExpr(gb.T(t0, 2), gb.T(t1, 2), gb.T(t2, 2), gb.T(u, 1)), 3))
		return m.Compile(), []gb.Option{
			gb.WithPureGo(),
			gb.WithCertified(true),
			gb.WithCuts(false),
			gb.WithHeuristics(false),
			gb.WithRelGapTol(0),
			gb.WithAbsGapTol(0),
		}

	case 4:
		// Mixed-integer problem triggering VIPRPresolveEnvelope reductions (singleton, GCD, activity).
		m := gb.NewModel("vipr_presolve_env", gb.Minimize)
		x0 := m.AddIntVar("x0", 0, 10)
		x1 := m.AddIntVar("x1", 0, 10)
		x2 := m.AddBinaryVar("x2")
		c0 := m.AddContVar("c0", 0, 5)
		m.AddConstraint("singleton", gb.Ge(gb.NewExpr(gb.T(x0, 2)), 3))
		m.AddConstraint("gcd", gb.Le(gb.NewExpr(gb.T(x0, 2), gb.T(x1, 4), gb.T(x2, 2)), 11))
		m.AddConstraint("link", gb.Le(gb.NewExpr(gb.T(x1, 1), gb.T(c0, -1)), 4))
		m.SetObjective(gb.NewExpr(gb.T(x0, 3), gb.T(x1, -2), gb.T(x2, -4), gb.T(c0, 1)))
		return m.Compile(), []gb.Option{
			gb.WithPureGo(),
			gb.WithCertified(true),
			gb.WithPresolve(true),
			gb.WithRelGapTol(0),
			gb.WithAbsGapTol(0),
		}

	default:
		// Byte-driven small binary/integer MIP with non-zero constant and sense toggle.
		sense := gb.Minimize
		if cur.u8()%2 == 1 {
			sense = gb.Maximize
		}
		m := gb.NewModel("vipr_fuzz_mip", sense)
		n := cur.intRange(2, 5)
		xs := make([]gb.Var, n)
		for j := range n {
			if cur.u8()%2 == 0 {
				xs[j] = m.AddBinaryVar("b" + strconv.Itoa(j))
			} else {
				xs[j] = m.AddIntVar("i"+strconv.Itoa(j), -1, 4)
			}
		}
		mRows := cur.intRange(1, 4)
		for i := range mRows {
			terms := make([]gb.Term, 0, n)
			for j := range n {
				coeff := float64(cur.intRange(-4, 5))
				if coeff != 0 {
					terms = append(terms, gb.T(xs[j], coeff))
				}
			}
			if len(terms) == 0 {
				terms = append(terms, gb.T(xs[0], 1))
			}
			rhs := float64(cur.intRange(-3, 8))
			switch cur.u8() % 3 {
			case 0:
				m.AddConstraint("r"+strconv.Itoa(i), gb.Le(gb.NewExpr(terms...), rhs))
			case 1:
				m.AddConstraint("r"+strconv.Itoa(i), gb.Ge(gb.NewExpr(terms...), rhs))
			default:
				m.AddConstraint("r"+strconv.Itoa(i), gb.Eq(gb.NewExpr(terms...), rhs))
			}
		}
		objTerms := make([]gb.Term, 0, n)
		for j := range n {
			c := float64(cur.intRange(-5, 5))
			if c != 0 {
				objTerms = append(objTerms, gb.T(xs[j], c))
			}
		}
		objExpr := gb.NewExpr(objTerms...)
		if cur.u8()%2 == 0 {
			objExpr = objExpr.AddConstant(float64(cur.intRange(-20, 20)))
		}
		m.SetObjective(objExpr)
		return m.Compile(), []gb.Option{
			gb.WithPureGo(),
			gb.WithCertified(true),
			gb.WithRelGapTol(0),
			gb.WithAbsGapTol(0),
		}
	}
}

func mutateVIPRJSONBytes(raw []byte, cur *certFuzzCursor) []byte {
	mode := cur.u8() % 6
	switch mode {
	case 0:
		// Inject duplicate root-level key before final closing brace.
		idx := bytes.LastIndexByte(raw, '}')
		if idx <= 0 {
			return append(bytes.Clone(raw), []byte(`,"mode":"infeasible"}`)...)
		}
		dupFields := []string{
			`,"mode":"infeasible"`,
			`,"n":99`,
			`,"rootStep":-1`,
			`,"objConst":{"num":"1","den":"1"}`,
		}
		ins := dupFields[int(cur.u8())%len(dupFields)]
		out := make([]byte, 0, len(raw)+len(ins))
		out = append(out, raw[:idx]...)
		out = append(out, ins...)
		out = append(out, raw[idx:]...)
		return out

	case 1:
		// Inject duplicate nested key inside rational object or step object.
		target := []byte(`"den":"`)
		pos := bytes.Index(raw, target)
		if pos > 0 {
			ins := `"num":"1",`
			out := make([]byte, 0, len(raw)+len(ins))
			out = append(out, raw[:pos]...)
			out = append(out, ins...)
			out = append(out, raw[pos:]...)
			return out
		}
		return mutateVIPRJSONBytesFallback(raw, cur)

	case 2:
		// Inflate a rational numerator near 8192 bytes or 16384 bits.
		lengths := []int{4931, 4933, 8191, 8192, 8193}
		nLen := lengths[int(cur.u8())%len(lengths)]
		huge := "9" + strings.Repeat("0", nLen-2) + "1"
		marker := []byte(`"num":"`)
		pos := bytes.Index(raw, marker)
		if pos >= 0 {
			valStart := pos + len(marker)
			valEndRel := bytes.IndexByte(raw[valStart:], '"')
			if valEndRel >= 0 {
				out := make([]byte, 0, len(raw)+len(huge))
				out = append(out, raw[:valStart]...)
				out = append(out, huge...)
				out = append(out, raw[valStart+valEndRel:]...)
				return out
			}
		}
		return mutateVIPRJSONBytesFallback(raw, cur)

	case 3:
		// Flip sense or rule token.
		replacements := [][2]string{
			{`"<="`, `">="`},
			{`">="`, `"<="`},
			{`"="`, `"<="`},
			{`"orbitBranch"`, `"assume"`},
			{`"farkas"`, `"safeDual"`},
			{`"symBreakLe"`, `"roundBound"`},
		}
		for range replacements {
			pair := replacements[int(cur.u8())%len(replacements)]
			if idx := bytes.Index(raw, []byte(pair[0])); idx >= 0 {
				out := make([]byte, 0, len(raw)+8)
				out = append(out, raw[:idx]...)
				out = append(out, pair[1]...)
				out = append(out, raw[idx+len(pair[0]):]...)
				return out
			}
		}
		return mutateVIPRJSONBytesFallback(raw, cur)

	case 4:
		// Splice or alter random bytes guided by cursor.
		return mutateVIPRJSONBytesFallback(raw, cur)

	default:
		// Duplicate "num" inside steps or drop suffix.
		if idx := bytes.Index(raw, []byte(`"rule":"`)); idx > 0 {
			ins := `"rule":"unknownRule",`
			out := make([]byte, 0, len(raw)+len(ins))
			out = append(out, raw[:idx]...)
			out = append(out, ins...)
			out = append(out, raw[idx:]...)
			return out
		}
		return mutateVIPRJSONBytesFallback(raw, cur)
	}
}

func mutateVIPRJSONBytesFallback(raw []byte, cur *certFuzzCursor) []byte {
	out := bytes.Clone(raw)
	if len(out) == 0 {
		return []byte("{}")
	}
	idx := int(cur.u64() % uint64(len(out)))
	out[idx] ^= 1 + (cur.u8() % 254)
	return out
}

func assertFailClosedResultAndWitness(
	tb testing.TB,
	label string,
	res gb.Result,
	err error,
	wit gb.CertifiedWitness,
) {
	tb.Helper()
	if !errors.Is(err, gb.ErrCertificationFailed) {
		tb.Fatalf("[%s] expected ErrCertificationFailed, got err=%v status=%s", label, err, res.Status)
	}
	if res.Status != gb.StatusNumericalError {
		tb.Fatalf("[%s] expected StatusNumericalError on fail-closed result, got %s", label, res.Status)
	}
	if len(res.Values) != 0 || len(res.Duals) != 0 || len(res.ReducedCosts) != 0 ||
		res.Objective != 0 || res.BestBound != 0 {
		tb.Fatalf("[%s] expected scrubbed Result fields on fail-closed return, got obj=%g bound=%g vals=%d duals=%d rc=%d",
			label, res.Objective, res.BestBound, len(res.Values), len(res.Duals), len(res.ReducedCosts))
	}
	if wit.Status != 0 || len(wit.Values) != 0 || len(wit.Duals) != 0 || len(wit.ReducedCosts) != 0 ||
		wit.Objective != nil || wit.BestBound != nil || len(wit.Farkas) != 0 || wit.Unbounded != nil {
		tb.Fatalf("[%s] expected scrubbed CertifiedWitness on fail-closed return, got status=%s", label, wit.Status)
	}
}

type lpOptimalJSONReq struct {
	Mode     string            `json:"mode"`
	NumVars  int               `json:"numVars"`
	NumRows  int               `json:"numRows"`
	ObjConst fuzzRatJSON       `json:"objConst"`
	Obj      []fuzzRatJSON     `json:"obj"`
	LB       []*fuzzRatJSON    `json:"lb"`
	UB       []*fuzzRatJSON    `json:"ub"`
	Rows     []lpBridgeRowJSON `json:"rows"`
	X        []fuzzRatJSON     `json:"x"`
	Y        []fuzzRatJSON     `json:"y"`
}

func encodeOptimalLPCheckJSON(p *gb.CompiledProblem, xRat, dualMin []*big.Rat) ([]byte, bool) {
	oc, obj, lb, ub, rows, err := encodeProblemHeaderForLPCheck(p)
	if err != nil || len(xRat) != p.NumVars() || len(dualMin) != p.NumConstraints() {
		return nil, false
	}
	xJSON := make([]fuzzRatJSON, len(xRat))
	for j, r := range xRat {
		if r == nil {
			return nil, false
		}
		xJSON[j] = fuzzRatFromBigRat(r)
	}
	yJSON := make([]fuzzRatJSON, len(dualMin))
	for i, r := range dualMin {
		if r == nil {
			return nil, false
		}
		yJSON[i] = fuzzRatFromBigRat(r)
	}
	raw, errM := json.Marshal(lpOptimalJSONReq{
		Mode:     "optimal",
		NumVars:  p.NumVars(),
		NumRows:  p.NumConstraints(),
		ObjConst: oc,
		Obj:      obj,
		LB:       lb,
		UB:       ub,
		Rows:     rows,
		X:        xJSON,
		Y:        yJSON,
	})
	return raw, errM == nil
}

func encodeFarkasLPCheckJSON(p *gb.CompiledProblem, farkasW []*big.Rat) ([]byte, bool) {
	oc, obj, lb, ub, rows, err := encodeProblemHeaderForLPCheck(p)
	if err != nil || len(farkasW) != p.NumConstraints() {
		return nil, false
	}
	wJSON := make([]fuzzRatJSON, len(farkasW))
	for i, r := range farkasW {
		if r == nil {
			return nil, false
		}
		// Track 3 lp-check expects the negated dual cone relative to Track 4 Mini-VIPR.
		wJSON[i] = fuzzRatFromBigRat(new(big.Rat).Neg(r))
	}
	raw, errM := json.Marshal(lpFarkasReqJSON{
		Mode:     "farkas",
		NumVars:  p.NumVars(),
		NumRows:  p.NumConstraints(),
		ObjConst: oc,
		Obj:      obj,
		LB:       lb,
		UB:       ub,
		Rows:     rows,
		W:        wJSON,
	})
	return raw, errM == nil
}

func encodeUnboundedLPCheckJSON(p *gb.CompiledProblem, u *gb.UnboundedCertificate) ([]byte, bool) {
	oc, obj, lb, ub, rows, err := encodeProblemHeaderForLPCheck(p)
	if err != nil || u == nil || len(u.X0) != p.NumVars() || len(u.Ray) != p.NumVars() {
		return nil, false
	}
	x0 := make([]fuzzRatJSON, len(u.X0))
	d := make([]fuzzRatJSON, len(u.Ray))
	for j := range p.NumVars() {
		if u.X0[j] == nil || u.Ray[j] == nil {
			return nil, false
		}
		x0[j] = fuzzRatFromBigRat(u.X0[j])
		d[j] = fuzzRatFromBigRat(u.Ray[j])
	}
	raw, errM := json.Marshal(lpUnboundedReqJSON{
		Mode:     "unbounded",
		NumVars:  p.NumVars(),
		NumRows:  p.NumConstraints(),
		ObjConst: oc,
		Obj:      obj,
		LB:       lb,
		UB:       ub,
		Rows:     rows,
		X0:       x0,
		D:        d,
	})
	return raw, errM == nil
}

func runLPCertFaultAndOracleCheck(tb testing.TB, cur *certFuzzCursor) {
	tb.Helper()
	lpType := cur.u8() % 3
	sense := gb.Minimize
	if cur.u8()%2 == 1 {
		sense = gb.Maximize
	}
	m := gb.NewModel("lp_cert_fault", sense)
	n := cur.intRange(2, 4)
	vars := make([]gb.Var, n)
	for j := range n {
		switch lpType {
		case 2:
			vars[j] = m.AddContVar("x"+strconv.Itoa(j), 0, math.Inf(1))
		default:
			vars[j] = m.AddContVar("x"+strconv.Itoa(j), 0, 10)
		}
	}
	switch lpType {
	case 0:
		// Optimal LP.
		terms := make([]gb.Term, n)
		objTerms := make([]gb.Term, n)
		for j := range n {
			terms[j] = gb.T(vars[j], float64(j+1))
			coeff := float64(cur.intRange(-6, 6))
			if coeff == 0 {
				coeff = 2
			}
			objTerms[j] = gb.T(vars[j], coeff)
		}
		m.AddConstraint("cap", gb.Le(gb.NewExpr(terms...), 7))
		m.AddConstraint("floor", gb.Ge(gb.NewExpr(gb.T(vars[0], 1), gb.T(vars[1], 1)), 1))
		m.SetObjective(gb.NewExpr(objTerms...))
	case 1:
		// Farkas-infeasible LP.
		m.AddConstraint("hi", gb.Le(gb.NewExpr(gb.T(vars[0], 1), gb.T(vars[1], 1)), 2))
		m.AddConstraint("lo", gb.Ge(gb.NewExpr(gb.T(vars[0], 1), gb.T(vars[1], 1)), 5))
		m.SetObjective(gb.NewExpr(gb.T(vars[0], 1)))
	default:
		// Unbounded LP.
		m.AddConstraint("ray", gb.Ge(gb.NewExpr(gb.T(vars[0], 1), gb.T(vars[1], 1)), 1))
		c := -1.0
		if sense == gb.Maximize {
			c = 1.0
		}
		m.SetObjective(gb.NewExpr(gb.T(vars[0], c)))
	}
	p := m.Compile()
	tr := gb.NewVIPRTrace()
	var wit gb.CertifiedWitness
	res, err := gb.SolveCompiled(tb.Context(), p, gb.WithPureGo(), gb.WithCertified(true), gb.WithVIPRTrace(tr), gb.WithCertifiedWitness(&wit))
	if err != nil {
		tb.Fatalf("clean LP certified solve failed: status=%s err=%v", res.Status, err)
	}
	if formalFuzzEnabled() {
		switch res.Status {
		case gb.StatusOptimal:
			dualMin := make([]*big.Rat, len(wit.Duals))
			for i, d := range wit.Duals {
				dualMin[i] = new(big.Rat).Set(d)
				if p.Sense() == gb.Maximize && dualMin[i].Sign() != 0 {
					dualMin[i].Neg(dualMin[i])
				}
			}
			if raw, ok := encodeOptimalLPCheckJSON(p, wit.Values, dualMin); ok {
				if _, errC := checkFormalLPJSONLForProblem(tb, p, raw); errC != nil {
					tb.Fatalf("clean LP optimal failed lp-check: %v", errC)
				}
			}
		case gb.StatusInfeasible:
			if len(wit.Farkas) == p.NumConstraints() {
				if raw, ok := encodeFarkasLPCheckJSON(p, wit.Farkas); ok {
					if _, errC := checkFormalLPJSONLForProblem(tb, p, raw); errC != nil {
						tb.Fatalf("clean LP farkas failed lp-check: %v", errC)
					}
				}
			}
		case gb.StatusUnbounded:
			if wit.Unbounded != nil {
				if raw, ok := encodeUnboundedLPCheckJSON(p, wit.Unbounded); ok {
					if _, errC := checkFormalLPJSONLForProblem(tb, p, raw); errC != nil {
						tb.Fatalf("clean LP unbounded failed lp-check: %v", errC)
					}
				}
			}
		}
	}

	faultSel := cur.u8()
	var capturedTamperedLP []byte
	fCtx := gb.WithCertFaultForTest(tb.Context(), func(fp *gb.CompiledProblem, fTrace *gb.VIPRTrace, lpWit *gb.CertLPFaultTargetForTest, fRes *gb.Result) {
		if lpWit == nil {
			return
		}
		switch fRes.Status {
		case gb.StatusOptimal:
			switch faultSel % 4 {
			case 0:
				if len(lpWit.DualMin) > 0 {
					// Move DualMin[0] within the <= 0 dual cone so safeDualBound does not clamp it to 0.
					delta := big.NewRat(-5, 1)
					if fp.RowSense(0) == gb.GreaterEqual {
						delta = big.NewRat(5, 1)
					}
					lpWit.DualMin[0] = new(big.Rat).Add(lpWit.DualMin[0], delta)
					if raw, ok := encodeOptimalLPCheckJSON(fp, lpWit.Primal, lpWit.DualMin); ok {
						capturedTamperedLP = raw
					}
				} else {
					fRes.Objective = math.Nextafter(fRes.Objective, math.Inf(1))
				}
			case 1:
				if len(lpWit.Primal) > 0 {
					lpWit.Primal[0] = new(big.Rat).Add(lpWit.Primal[0], big.NewRat(1, 2))
					if raw, ok := encodeOptimalLPCheckJSON(fp, lpWit.Primal, lpWit.DualMin); ok {
						capturedTamperedLP = raw
					}
				}
			case 2:
				// Nudge Values[0] or Objective by 1 ULP to check strict rounding bounds.
				if len(fRes.Values) > 0 {
					dir := math.Inf(1)
					if faultSel%2 == 1 {
						dir = math.Inf(-1)
					}
					fRes.Values[0] = math.Nextafter(fRes.Values[0], dir)
				} else {
					fRes.Objective = math.Nextafter(fRes.Objective, math.Inf(1))
				}
			default:
				if mut, ok := gb.TamperVIPRTraceForTest(fTrace, gb.VIPRTamperFlipSense, cur.u64()); ok {
					*fTrace = *mut
				} else {
					fRes.Status = gb.StatusInfeasible
				}
			}
		case gb.StatusInfeasible:
			if len(lpWit.FarkasW) > 0 && faultSel%2 == 0 {
				for i := range lpWit.FarkasW {
					lpWit.FarkasW[i] = new(big.Rat)
				}
				if raw, ok := encodeFarkasLPCheckJSON(fp, lpWit.FarkasW); ok {
					capturedTamperedLP = raw
				}
			} else {
				lpWit.FarkasW = nil
				lpWit.HasDirectInfeas = false
			}
		case gb.StatusUnbounded:
			if len(lpWit.Unbounded.Ray) > 0 && faultSel%2 == 0 {
				lpWit.Unbounded.Ray[0] = new(big.Rat).Neg(lpWit.Unbounded.Ray[0])
				if lpWit.Unbounded.Ray[0].Sign() == 0 {
					lpWit.Unbounded.Ray[0] = big.NewRat(-1, 1)
				}
				if raw, ok := encodeUnboundedLPCheckJSON(fp, &lpWit.Unbounded); ok {
					capturedTamperedLP = raw
				}
			} else {
				lpWit.HasUnbounded = false
			}
		}
	})
	var witFault gb.CertifiedWitness
	trFault := gb.NewVIPRTrace()
	resFault, errFault := gb.SolveCompiled(fCtx, p, gb.WithPureGo(), gb.WithCertified(true), gb.WithVIPRTrace(trFault), gb.WithCertifiedWitness(&witFault))
	assertFailClosedResultAndWitness(tb, "lp_fault_inject", resFault, errFault, witFault)
	if formalFuzzEnabled() && len(capturedTamperedLP) > 0 {
		if _, errLean := checkFormalLPJSONL(tb, capturedTamperedLP); errLean == nil {
			tb.Fatalf("expected lp-check to reject tampered LP certificate")
		}
	}
}

// FuzzVerifyVIPRTraceVsLean mutates solved VIPR traces at both AST and JSON-byte
// levels, compares Go VerifyVIPRTrace and VerifyMiniVIPRForTest against Lean vipr-check,
// checks 1-ULP witness rounding and exactModelOrig, and injects faults into
// certified LP solves against lp-check.
func FuzzVerifyVIPRTraceVsLean(f *testing.F) {
	seeds := [][]byte{
		{0, 0, 1, 2, 3, 4, 5, 6, 7, 8},
		{1, 1, 0, 3, 2, 5, 4, 7, 6, 9},
		{2, 2, 4, 6, 8, 10, 12, 14, 16, 18},
		{3, 3, 1, 1, 2, 2, 3, 3, 4, 4},
		{4, 5, 2, 4, 6, 8, 10, 12, 14, 16},
		{5, 6, 0, 1, 2, 3, 4, 5, 6, 7},
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		cur := newCertFuzzCursor(data)
		scenario := int(cur.u8() % 6)
		p, opts := buildVIPRScenario(cur, scenario)
		tr := gb.NewVIPRTrace()
		var wit gb.CertifiedWitness
		opts = append(opts, gb.WithVIPRTrace(tr), gb.WithCertifiedWitness(&wit))
		appliedOpt := gb.ApplyOptions(opts...)

		res, err := gb.SolveCompiled(t.Context(), p, opts...)
		if err != nil {
			t.Fatalf("clean scenario %d solve failed: status=%s err=%v", scenario, res.Status, err)
		}
		if scenario == 3 && tr.OrbitBranchesCertified() == 0 {
			t.Fatalf("scenario 3 expected OrbitBranchesCertified > 0")
		}
		if scenario == 4 && tr.PresolveStepCount() == 0 {
			t.Fatalf("scenario 4 expected PresolveStepCount > 0")
		}
		assertAllowedStatus(t, "vipr_clean", p, false, res)
		startV := time.Now()
		if errV := gb.VerifyVIPRTrace(p, tr, res, appliedOpt); errV != nil {
			t.Fatalf("VerifyVIPRTrace clean scenario %d failed: %v", scenario, errV)
		}
		assertVIPRComplexityBudget(t, "vipr_clean", p, tr, time.Since(startV))

		if tr.HasCertificate() {
			rawClean, errM := tr.MarshalVIPRJSON()
			if errM != nil {
				t.Fatalf("MarshalVIPRJSON failed: %v", errM)
			}
			if errMini := gb.VerifyMiniVIPRForTest(p, rawClean); errMini != nil {
				t.Fatalf("VerifyMiniVIPRForTest clean failed: %v", errMini)
			}
			if formalFuzzEnabled() {
				if errF := checkFormalVIPRResult(t, "vipr_clean", p, res, tr, appliedOpt.RelGapTol, appliedOpt.AbsGapTol); errF != nil {
					t.Fatal(errF)
				}
			}

			// 1. AST-level VIPR trace mutations.
			kind := int(cur.u8() % gb.VIPRTamperCount)
			sel := cur.u64()
			if mutTr, okMut := gb.TamperVIPRTraceForTest(tr, kind, sel); okMut {
				goTraceErr := gb.VerifyVIPRTrace(p, mutTr, res, appliedOpt)
				var rawMut []byte
				var goBytesErr error
				if mutTr.HasCertificate() {
					var errRM error
					rawMut, errRM = mutTr.MarshalVIPRJSON()
					if errRM == nil {
						goBytesErr = gb.VerifyMiniVIPRForTest(p, rawMut)
					} else {
						goBytesErr = errRM
					}
				}
				if kind == gb.VIPRTamperWipe && goTraceErr == nil {
					t.Fatalf("VerifyVIPRTrace accepted wiped VIPRTrace")
				}
				if kind == gb.VIPRTamperOrbitPerm && goTraceErr == nil {
					t.Fatalf("VerifyVIPRTrace accepted corrupted orbit permutation")
				}
				if formalFuzzEnabled() && len(rawMut) > 0 {
					_, leanErr := checkFormalVIPRJSONL(t, rawMut)
					if leanErr != nil {
						if goTraceErr == nil {
							t.Fatalf("Go VerifyVIPRTrace accepted AST mutation (kind=%d sel=%d) rejected by Lean vipr-check: %v", kind, sel, leanErr)
						}
						if goBytesErr == nil {
							t.Fatalf("Go VerifyMiniVIPRForTest accepted AST mutation (kind=%d sel=%d) rejected by Lean vipr-check: %v", kind, sel, leanErr)
						}
					}
				}
			}

			// 2. Raw JSON byte mutations (duplicate keys, inflated rationals, flips, splices).
			rawByteMut := mutateVIPRJSONBytes(rawClean, cur)
			goProbByteErr := gb.VerifyMiniVIPRForTest(p, rawByteMut)
			goSoloByteErr := gb.VerifyMiniVIPRForTest(nil, rawByteMut)
			if formalFuzzEnabled() {
				_, leanByteErr := checkFormalVIPRJSONL(t, rawByteMut)
				if leanByteErr != nil && (goProbByteErr == nil || goSoloByteErr == nil) {
					t.Fatalf("Go accepted mutated VIPR JSON bytes rejected by Lean (probErr=%v soloErr=%v leanErr=%v)",
						goProbByteErr, goSoloByteErr, leanByteErr)
				}
			}
		}

		// 3. Alter exactModelOrig metadata on p.
		if pTampered, okOrig := gb.TamperExactOrigForTest(p, cur.u64()); okOrig {
			if errT := gb.VerifyVIPRTrace(pTampered, tr, res, appliedOpt); errT == nil {
				t.Fatalf("VerifyVIPRTrace accepted tampered exactModelOrig")
			}
		}

		// 4. Nudge witness projection by 1 ULP under strict WithCertified(true).
		if res.Status == gb.StatusOptimal {
			resObjNudge := res
			dir := math.Inf(1)
			if cur.u8()%2 == 1 {
				dir = math.Inf(-1)
			}
			resObjNudge.Objective = math.Nextafter(res.Objective, dir)
			if errN := gb.VerifyVIPRTrace(p, tr, resObjNudge, appliedOpt); errN == nil {
				t.Fatalf("VerifyVIPRTrace accepted 1-ULP Objective nudge (%g -> %g)", res.Objective, resObjNudge.Objective)
			}

			resBndNudge := res
			strongDir := math.Inf(1)
			if p.Sense() == gb.Maximize {
				strongDir = math.Inf(-1)
			}
			resBndNudge.BestBound = math.Nextafter(res.BestBound, strongDir)
			if errB := gb.VerifyVIPRTrace(p, tr, resBndNudge, appliedOpt); errB == nil {
				t.Fatalf("VerifyVIPRTrace accepted 1-ULP stronger BestBound nudge (%g -> %g)", res.BestBound, resBndNudge.BestBound)
			}

			if len(res.Values) > 0 {
				col := int(cur.u8()) % len(res.Values)
				valsCopy := append([]float64(nil), res.Values...)
				valsCopy[col] = math.Nextafter(valsCopy[col], dir)
				resValNudge := res
				resValNudge.Values = valsCopy
				if errV := gb.VerifyVIPRTrace(p, tr, resValNudge, appliedOpt); errV == nil {
					t.Fatalf("VerifyVIPRTrace accepted 1-ULP coordinate nudge on col %d (%g -> %g)", col, res.Values[col], valsCopy[col])
				}
			}
		}

		// 5. Faulty LP solves via goblinCertFaultKey checked against lp-check.
		runLPCertFaultAndOracleCheck(t, cur)
	})
}

func buildAdversarialCertifiedModel(cur *certFuzzCursor) *gb.CompiledProblem {
	sense := gb.Minimize
	if cur.u8()%2 == 1 {
		sense = gb.Maximize
	}
	m := gb.NewModel("cert_adv", sense)
	isMIP := cur.u8()%2 == 0
	n := cur.intRange(2, 6)
	xs := make([]gb.Var, n)

	scales := []float64{1, 1e-8, 1e8, 1e-12, 1e12, 5e-324, 1e-308, 1e-310, 0.5, -2.5}
	for j := range n {
		lb := 0.0
		ub := float64(cur.intRange(1, 6))
		switch cur.u8() % 5 {
		case 0:
			// Fractional bounds on integers exercising SetVarBounds rounding.
			lb = 0.0000000000005
			ub = float64(cur.intRange(2, 5)) - 0.0000000000005
		case 1:
			lb = cur.pickFloat([]float64{-1e-310, -1, 0, 1e-10})
			ub = cur.pickFloat([]float64{1e-308, 3, 10, 1e4})
		case 2:
			if !isMIP && cur.u8()%2 == 0 {
				ub = math.Inf(1)
			}
		}
		if isMIP && (j < n-1 || cur.u8()%2 == 0) {
			if cur.u8()%2 == 0 {
				xs[j] = m.AddBinaryVar("x" + strconv.Itoa(j))
			} else {
				xs[j] = m.AddIntVar("x"+strconv.Itoa(j), lb, ub)
			}
			if cur.u8()%3 == 0 {
				newLB := lb
				newUB := ub
				if cur.u8()%2 == 0 {
					newUB = float64(cur.intRange(1, 8)) - 0.0000000000005
				}
				if cur.u8()%5 == 0 {
					newLB = newUB + 1
				}
				m.SetVarBounds(xs[j], newLB, newUB)
			}
		} else {
			xs[j] = m.AddContVar("c"+strconv.Itoa(j), lb, ub)
			if cur.u8()%4 == 0 {
				m.SetVarBounds(xs[j], lb, math.Max(lb, ub))
			}
		}
	}

	mRows := cur.intRange(1, 5)
	for i := range mRows {
		rowTerms := make([]gb.Term, 0, n+2)
		rowScale := 1.0
		if cur.u8()%2 == 0 {
			// Alternate extreme row scales to trigger Ruiz scaling.
			rowScale = cur.pickFloat([]float64{1e-10, 1e-6, 1, 1e6, 1e10, 1e-308})
		}
		for j := range n {
			base := cur.pickFloat(scales)
			colScale := 1.0
			if j == 0 && cur.u8()%2 == 0 {
				colScale = cur.pickFloat([]float64{1e-7, 1e7, 1e-308})
			}
			c := base * rowScale
			if colScale != 1.0 && !math.IsInf(c*colScale, 0) {
				c *= colScale
			}
			if c != 0 && !math.IsInf(c, 0) {
				rowTerms = append(rowTerms, gb.T(xs[j], c))
			}
		}
		if cur.u8()%2 == 0 {
			// Inject cancelling/lossy duplicate terms to populate exactModelOrig.
			bigC := 1e16 * cur.pickFloat([]float64{1, -1})
			rowTerms = append(rowTerms, gb.T(xs[0], bigC), gb.T(xs[0], -bigC+1.0))
		}
		if len(rowTerms) == 0 {
			rowTerms = append(rowTerms, gb.T(xs[0], 1))
		}
		expr := gb.NewExpr(rowTerms...)
		if cur.u8()%2 == 0 {
			// Nonzero Expr.Constant in row constraints.
			expr = expr.AddConstant(cur.pickFloat([]float64{-2.5, 0.25, 1e-9, 1e6, -1e-308}))
		}
		rhs := cur.pickFloat([]float64{-1e6, -3.5, 0, 2.5, 10, 1e6}) * rowScale
		if math.IsInf(rhs, 0) || math.IsNaN(rhs) {
			rhs = 4.0
		}
		switch cur.u8() % 3 {
		case 0:
			m.AddConstraint("r"+strconv.Itoa(i), gb.Le(expr, rhs))
		case 1:
			m.AddConstraint("r"+strconv.Itoa(i), gb.Ge(expr, rhs))
		default:
			m.AddConstraint("r"+strconv.Itoa(i), gb.Eq(expr, rhs))
		}
	}

	objTerms := make([]gb.Term, 0, n+2)
	for j := range n {
		c := cur.pickFloat([]float64{-10, -1, 0, 1, 7, 1e-9, 1e8, 1e-310})
		if c != 0 {
			objTerms = append(objTerms, gb.T(xs[j], c))
		}
	}
	if cur.u8()%2 == 0 {
		// Duplicate cancelling objective terms triggering objHasLoss.
		objTerms = append(objTerms, gb.T(xs[0], 1e16), gb.T(xs[0], -1e16+0.25))
	}
	objExpr := gb.NewExpr(objTerms...)
	if cur.u8()%2 == 0 {
		// Cancelling ObjConstant regime against constant shift and linear terms.
		c1 := cur.pickFloat([]float64{1e15, -1e15, 1e9, 0.5})
		c2 := -c1 + cur.pickFloat([]float64{0, 1.0, -0.5, 1e-6})
		if cur.u8()%2 == 0 {
			objExpr = objExpr.AddConstant(c1)
		} else {
			objExpr = objExpr.AddConstant(c1).AddConstant(c2)
		}
	}
	m.SetObjective(objExpr)
	return m.Compile()
}

func userObjRatFromMinRat(p *gb.CompiledProblem, minRat *big.Rat) *big.Rat {
	out := new(big.Rat).Set(minRat)
	if p != nil && p.Sense() == gb.Maximize && out.Sign() != 0 {
		out.Neg(out)
	}
	return out
}

func verifyWitnessRationalInvariants(
	tb testing.TB,
	label string,
	p *gb.CompiledProblem,
	res gb.Result,
	wit gb.CertifiedWitness,
	relTol, absTol float64,
	hasLimits bool,
) {
	tb.Helper()
	if wit.Status != res.Status {
		tb.Fatalf("[%s] witness status %s != result status %s", label, wit.Status, res.Status)
	}
	if res.Status == gb.StatusUndefined || res.Status == gb.StatusNotSolved || res.Status == gb.StatusNumericalError {
		tb.Fatalf("[%s] unexpected err==nil status %s", label, res.Status)
	}

	objConst, objMin, _, _, _, _, okProb := gb.ExactProblemRatsForTest(p)
	if !okProb {
		tb.Fatalf("[%s] ExactProblemRatsForTest failed", label)
	}

	switch res.Status {
	case gb.StatusOptimal:
		if len(wit.Values) != p.NumVars() || wit.Objective == nil || wit.BestBound == nil {
			tb.Fatalf("[%s] incomplete Optimal witness: vals=%d n=%d obj=%v bnd=%v",
				label, len(wit.Values), p.NumVars(), wit.Objective, wit.BestBound)
		}
		if !isExactRatSliceMIPFeasible(p, wit.Values) {
			tb.Fatalf("[%s] witness primal violates exact MIP/LP feasibility", label)
		}
		minEval := exactEvalProblemObjRat(objConst, objMin, wit.Values)
		userEval := userObjRatFromMinRat(p, minEval)
		if userEval.Cmp(wit.Objective) != 0 {
			tb.Fatalf("[%s] wit.Objective=%s != exact eval %s", label, wit.Objective.RatString(), userEval.RatString())
		}
		if p.Sense() == gb.Maximize {
			if wit.BestBound.Cmp(wit.Objective) < 0 {
				tb.Fatalf("[%s] Maximize BestBound %s < Objective %s", label, wit.BestBound.RatString(), wit.Objective.RatString())
			}
		} else if wit.BestBound.Cmp(wit.Objective) > 0 {
			tb.Fatalf("[%s] Minimize BestBound %s > Objective %s", label, wit.BestBound.RatString(), wit.Objective.RatString())
		}
		if !math.IsInf(relTol, 1) && !math.IsInf(absTol, 1) {
			gap := new(big.Rat).Sub(wit.Objective, wit.BestBound)
			if gap.Sign() < 0 {
				gap.Neg(gap)
			}
			linEval := new(big.Rat).Sub(minEval, objConst)
			linAbs := new(big.Rat).Abs(linEval)
			scaleRat := big.NewRat(1, 1)
			if linAbs.Cmp(scaleRat) > 0 {
				scaleRat = linAbs
			}
			relRat := new(big.Rat).SetFloat64(relTol)
			absRat := new(big.Rat).SetFloat64(absTol)
			if relRat != nil && absRat != nil {
				allowed := new(big.Rat).Mul(relRat, scaleRat)
				if absRat.Cmp(allowed) > 0 {
					allowed = absRat
				}
				if gap.Cmp(allowed) > 0 {
					tb.Fatalf("[%s] exact witness gap %s exceeds tolerance %s (rel=%g abs=%g)",
						label, gap.RatString(), allowed.RatString(), relTol, absTol)
				}
			}
		}
		if !p.IsMIP() {
			if len(wit.Duals) != p.NumConstraints() || len(wit.ReducedCosts) != p.NumVars() {
				tb.Fatalf("[%s] LP optimal witness missing Duals (%d/%d) or ReducedCosts (%d/%d)",
					label, len(wit.Duals), p.NumConstraints(), len(wit.ReducedCosts), p.NumVars())
			}
			dualMin := make([]*big.Rat, p.NumConstraints())
			for i, d := range wit.Duals {
				if d == nil {
					tb.Fatalf("[%s] nil Duals[%d]", label, i)
				}
				dualMin[i] = new(big.Rat).Set(d)
				if p.Sense() == gb.Maximize && dualMin[i].Sign() != 0 {
					dualMin[i].Neg(dualMin[i])
				}
				switch p.RowSense(i) {
				case gb.LessEqual:
					if dualMin[i].Sign() > 0 {
						tb.Fatalf("[%s] LP LessEqual row %d dualMin=%s > 0", label, i, dualMin[i].RatString())
					}
				case gb.GreaterEqual:
					if dualMin[i].Sign() < 0 {
						tb.Fatalf("[%s] LP GreaterEqual row %d dualMin=%s < 0", label, i, dualMin[i].RatString())
					}
				}
			}
			_, _, _, rowCols, rowVals, _, _ := gb.ExactProblemRatsForTest(p)
			atY := make([]*big.Rat, p.NumVars())
			for j := range p.NumVars() {
				atY[j] = new(big.Rat)
			}
			var prod big.Rat
			for i := range p.NumConstraints() {
				if dualMin[i].Sign() == 0 {
					continue
				}
				for k, col := range rowCols[i] {
					prod.Mul(dualMin[i], rowVals[i][k])
					atY[col].Add(atY[col], &prod)
				}
			}
			for j := range p.NumVars() {
				rc := wit.ReducedCosts[j]
				if rc == nil {
					tb.Fatalf("[%s] nil ReducedCosts[%d]", label, j)
				}
				rcMin := new(big.Rat).Set(rc)
				if p.Sense() == gb.Maximize && rcMin.Sign() != 0 {
					rcMin.Neg(rcMin)
				}
				expectedRC := new(big.Rat).Sub(objMin[j], atY[j])
				if rcMin.Cmp(expectedRC) != 0 {
					tb.Fatalf("[%s] ReducedCosts[%d] min=%s != exact c-A^T y=%s",
						label, j, rcMin.RatString(), expectedRC.RatString())
				}
			}
			if safeMin, okSafe := gb.ExactSafeDualBound(p, dualMin); okSafe {
				safeUser := userObjRatFromMinRat(p, safeMin)
				if safeUser.Cmp(wit.BestBound) > 0 && p.Sense() == gb.Maximize {
					tb.Fatalf("[%s] ExactSafeDualBound=%s > wit.BestBound=%s", label, safeUser.RatString(), wit.BestBound.RatString())
				}
				if safeUser.Cmp(wit.BestBound) < 0 && p.Sense() == gb.Minimize {
					tb.Fatalf("[%s] ExactSafeDualBound=%s < wit.BestBound=%s", label, safeUser.RatString(), wit.BestBound.RatString())
				}
			}
		}

	case gb.StatusInfeasible:
		// LP infeasibility may carry a Farkas ray (except direct bound conflict or zero-row).
		if !p.IsMIP() && len(wit.Farkas) > 0 {
			if len(wit.Farkas) != p.NumConstraints() {
				tb.Fatalf("[%s] Farkas vector len %d != constraints %d", label, len(wit.Farkas), p.NumConstraints())
			}
			_, _, rowRHS, rowCols, rowVals, _, _ := gb.ExactProblemRatsForTest(p)
			atW := make([]*big.Rat, p.NumVars())
			for j := range p.NumVars() {
				atW[j] = new(big.Rat)
			}
			wRHS := new(big.Rat)
			var prod big.Rat
			for i, wi := range wit.Farkas {
				if wi == nil {
					tb.Fatalf("[%s] nil Farkas[%d]", label, i)
				}
				switch p.RowSense(i) {
				case gb.LessEqual:
					if wi.Sign() < 0 {
						tb.Fatalf("[%s] Farkas LessEqual row %d coeff %s < 0", label, i, wi.RatString())
					}
				case gb.GreaterEqual:
					if wi.Sign() > 0 {
						tb.Fatalf("[%s] Farkas GreaterEqual row %d coeff %s > 0", label, i, wi.RatString())
					}
				}
				if wi.Sign() != 0 {
					prod.Mul(wi, rowRHS[i])
					wRHS.Add(wRHS, &prod)
					for k, col := range rowCols[i] {
						prod.Mul(wi, rowVals[i][k])
						atW[col].Add(atW[col], &prod)
					}
				}
			}
			// Farkas ray obeys projectVIPRFarkas: w^T A x <= w^T b contradicted by min_x (w^T A) x > w^T b (gap = w^T b - minActivity < 0).
			minActivity := new(big.Rat)
			for j := range p.NumVars() {
				c := atW[j]
				if c.Sign() == 0 {
					continue
				}
				lbj, ubj := p.VarBounds(j)
				if c.Sign() > 0 {
					lbRat := new(big.Rat).SetFloat64(lbj)
					if math.IsInf(lbj, -1) || lbRat == nil {
						tb.Fatalf("[%s] Farkas unbounded lower col %d with w^T A > 0", label, j)
					}
					prod.Mul(c, lbRat)
				} else {
					ubRat := new(big.Rat).SetFloat64(ubj)
					if math.IsInf(ubj, 1) || ubRat == nil {
						tb.Fatalf("[%s] Farkas unbounded upper col %d with w^T A < 0", label, j)
					}
					prod.Mul(c, ubRat)
				}
				minActivity.Add(minActivity, &prod)
			}
			gap := new(big.Rat).Sub(wRHS, minActivity)
			if gap.Sign() >= 0 {
				tb.Fatalf("[%s] invalid exact Farkas ray gap=%s >= 0", label, gap.RatString())
			}
		}

	case gb.StatusUnbounded:
		if wit.Unbounded == nil || !gb.VerifyUnboundedCertificate(p, *wit.Unbounded) {
			tb.Fatalf("[%s] StatusUnbounded missing valid UnboundedCertificate", label)
		}

	case gb.StatusTimeLimit, gb.StatusNodeLimit, gb.StatusIterationLimit:
		if !hasLimits && (isStandardScaledProblem(p) || res.Status != gb.StatusIterationLimit) {
			tb.Fatalf("[%s] unexpected limit status %s without limits", label, res.Status)
		}
		if len(wit.Values) > 0 {
			if len(wit.Values) != p.NumVars() || !isExactRatSliceMIPFeasible(p, wit.Values) {
				tb.Fatalf("[%s] limit witness incumbent violates exact feasibility", label)
			}
			if wit.Objective != nil {
				minEval := exactEvalProblemObjRat(objConst, objMin, wit.Values)
				userEval := userObjRatFromMinRat(p, minEval)
				if userEval.Cmp(wit.Objective) != 0 {
					tb.Fatalf("[%s] limit wit.Objective=%s != exact eval %s", label, wit.Objective.RatString(), userEval.RatString())
				}
			}
		}
	}
}

// FuzzCertifiedAdversarialAndWitness stresses WithCertified(true) and
// WithCertifiedWitness without standard-scaling filters across Ruiz scaling,
// subnormals, cancelling ObjConstant, lossy exactModelOrig, nonzero Expr.Constant,
// SetVarBounds, wide gap-tolerance sweeps, and tight node/iteration limits.
func FuzzCertifiedAdversarialAndWitness(f *testing.F) {
	seeds := [][]byte{
		{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11},
		{1, 0, 5, 4, 3, 2, 1, 0, 7, 6, 5, 4},
		{2, 2, 3, 6, 9, 12, 15, 18, 21, 24, 27, 30},
		{3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14},
	}
	for _, s := range seeds {
		f.Add(s)
	}

	tols := []float64{0, 1e-15, 1e-6, 0.5, 1.0, 10.0, math.Inf(1)}

	f.Fuzz(func(t *testing.T, data []byte) {
		cur := newCertFuzzCursor(data)
		p := buildAdversarialCertifiedModel(cur)
		relTol := cur.pickFloat(tols)
		absTol := cur.pickFloat(tols)

		opts := []gb.Option{
			gb.WithPureGo(),
			gb.WithCertified(true),
			gb.WithRelGapTol(relTol),
			gb.WithAbsGapTol(absTol),
			gb.WithPresolve(cur.u8()%2 == 0),
			gb.WithCuts(cur.u8()%2 == 0),
			gb.WithHeuristics(cur.u8()%2 == 0),
		}
		var tr *gb.VIPRTrace
		if cur.u8()%2 == 0 {
			tr = gb.NewVIPRTrace()
			opts = append(opts, gb.WithVIPRTrace(tr))
		}
		hasLimits := false
		if cur.u8()%3 == 0 {
			hasLimits = true
			if p.IsMIP() {
				opts = append(opts, gb.WithMaxNodes(cur.intRange(0, 6)))
			}
			opts = append(opts, gb.WithMaxIter(cur.intRange(1, 24)))
		}
		var wit gb.CertifiedWitness
		opts = append(opts, gb.WithCertifiedWitness(&wit))
		applied := gb.ApplyOptions(opts...)

		res, err := gb.SolveCompiled(t.Context(), p, opts...)
		if err != nil {
			assertFailClosedResultAndWitness(t, "adv_cert_fail_closed", res, err, wit)
			return
		}

		assertAllowedStatus(t, "adv_cert_ok", p, hasLimits, res)
		verifyWitnessRationalInvariants(t, "adv_cert_ok", p, res, wit, applied.RelGapTol, applied.AbsGapTol, hasLimits)
		if p.IsMIP() && tr != nil && res.Status != gb.StatusUnbounded &&
			(!isResourceLimitStatus(res.Status) || tr.HasCertificate()) {
			startV := time.Now()
			if errV := gb.VerifyVIPRTrace(p, tr, res, applied); errV != nil {
				t.Fatalf("VerifyVIPRTrace failed on certified solve (status=%s): %v", res.Status, errV)
			}
			assertVIPRComplexityBudget(t, "adv_cert_ok", p, tr, time.Since(startV))
		}
	})
}
