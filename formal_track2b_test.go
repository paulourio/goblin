package goblin

import (
	"bytes"
	"encoding/json/v2"
	"fmt"
	"maps"
	"math"
	"math/big"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type track2BTermPayload struct {
	Col int        `json:"col"`
	Val ratPayload `json:"val"`
}

type track2BRowPayload struct {
	Terms []track2BTermPayload `json:"terms"`
	Sense string               `json:"sense"`
	RHS   ratPayload           `json:"rhs"`
}

type track2BAggRowPayload struct {
	Terms []track2BTermPayload `json:"terms"`
	Sense string               `json:"sense"`
	RHS   ratPayload           `json:"rhs"`
	Mu    ratPayload           `json:"mu"`
}

type track2BCutPayload struct {
	Terms []track2BTermPayload `json:"terms"`
	RHS   ratPayload           `json:"rhs"`
}

type track2BMIPPayload struct {
	NumVars int                 `json:"numVars"`
	IsInt   []bool              `json:"isInt"`
	LB      []*ratPayload       `json:"lb"`
	UB      []*ratPayload       `json:"ub"`
	Rows    []track2BRowPayload `json:"rows"`
}

func runCutsCheck(tb testing.TB, exe, subcmd string, payload any, wantOK bool) string {
	tb.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		tb.Fatalf("marshal %s payload: %v", subcmd, err)
	}
	inPath := filepath.Join(tb.TempDir(), subcmd+".json")
	if err := os.WriteFile(inPath, raw, 0o600); err != nil {
		tb.Fatalf("write %s payload: %v", subcmd, err)
	}
	cmd := exec.CommandContext(tb.Context(), exe, subcmd, inPath)
	out, err := cmd.CombinedOutput()
	outStr := strings.TrimSpace(string(out))
	if wantOK && err != nil {
		tb.Fatalf("cuts-check %s failed unexpectedly: %v\noutput: %s", subcmd, err, outStr)
	}
	if !wantOK && err == nil {
		tb.Fatalf("cuts-check %s accepted invalid input; output: %s", subcmd, outStr)
	}
	return outStr
}

func senseName(s ConstraintSense) string {
	switch s {
	case LessEqual:
		return "le"
	case GreaterEqual:
		return "ge"
	default:
		return "eq"
	}
}

func mustRatPayload(tb testing.TB, v float64) ratPayload {
	tb.Helper()
	p, err := float64ToPayload(v)
	if err != nil {
		tb.Fatalf("float64ToPayload(%g): %v", v, err)
	}
	return p
}

func mustOptRatPayload(tb testing.TB, v float64) *ratPayload {
	tb.Helper()
	if math.IsInf(v, 0) {
		return nil
	}
	p := mustRatPayload(tb, v)
	return &p
}

func bigRatSliceToPayloads(rs []*big.Rat) []ratPayload {
	out := make([]ratPayload, len(rs))
	for i, r := range rs {
		out[i] = bigRatToPayload(r)
	}
	return out
}

func float64SliceToPayloads(tb testing.TB, xs []float64) []ratPayload {
	tb.Helper()
	out := make([]ratPayload, len(xs))
	for i, x := range xs {
		out[i] = mustRatPayload(tb, x)
	}
	return out
}

func compiledToMIPPayload(tb testing.TB, p *CompiledProblem) track2BMIPPayload {
	tb.Helper()
	n := p.numVars
	isInt := make([]bool, n)
	lb := make([]*ratPayload, n)
	ub := make([]*ratPayload, n)
	for j := range n {
		isInt[j] = p.varCat[j] != Continuous
		lb[j] = mustOptRatPayload(tb, p.varLB[j])
		ub[j] = mustOptRatPayload(tb, p.varUB[j])
	}
	rows := make([]track2BRowPayload, p.numConstraints)
	for i := range p.numConstraints {
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		terms := make([]track2BTermPayload, 0, e-s)
		for k := s; k < e; k++ {
			terms = append(terms, track2BTermPayload{
				Col: p.colIdx[k],
				Val: mustRatPayload(tb, p.rowVal[k]),
			})
		}
		rows[i] = track2BRowPayload{
			Terms: terms,
			Sense: senseName(p.rowSense[i]),
			RHS:   mustRatPayload(tb, p.rowRHS[i]),
		}
	}
	return track2BMIPPayload{
		NumVars: n,
		IsInt:   isInt,
		LB:      lb,
		UB:      ub,
		Rows:    rows,
	}
}

func cutsToPayloads(tb testing.TB, cuts []cutPlane) []track2BCutPayload {
	tb.Helper()
	out := make([]track2BCutPayload, len(cuts))
	for i, c := range cuts {
		terms := make([]track2BTermPayload, len(c.ColIdx))
		for k, col := range c.ColIdx {
			terms[k] = track2BTermPayload{
				Col: col,
				Val: mustRatPayload(tb, c.Val[k]),
			}
		}
		out[i] = track2BCutPayload{
			Terms: terms,
			RHS:   mustRatPayload(tb, c.RHS),
		}
	}
	return out
}

func solutionsToPayloads(tb testing.TB, sols [][]float64) [][]ratPayload {
	tb.Helper()
	out := make([][]ratPayload, len(sols))
	for i, s := range sols {
		out[i] = float64SliceToPayloads(tb, s)
	}
	return out
}

func loadLocalPuLPJSONModel(tb testing.TB, relPath string) *CompiledProblem {
	tb.Helper()
	data, err := os.ReadFile(relPath)
	if err != nil {
		tb.Fatalf("read %s: %v", relPath, err)
	}
	type coeffDoc struct {
		Name  string  `json:"name"`
		Value float64 `json:"value"`
	}
	type varDoc struct {
		Name     string   `json:"name"`
		Cat      string   `json:"cat"`
		LowBound *float64 `json:"lowBound"`
		UpBound  *float64 `json:"upBound"`
	}
	type rowDoc struct {
		Name         string     `json:"name"`
		Sense        int        `json:"sense"`
		Coefficients []coeffDoc `json:"coefficients"`
		Constant     float64    `json:"constant"`
	}
	type modelDoc struct {
		Parameters struct {
			Name  string `json:"name"`
			Sense int    `json:"sense"`
		} `json:"parameters"`
		Objective struct {
			Coefficients []coeffDoc `json:"coefficients"`
		} `json:"objective"`
		Variables   []varDoc `json:"variables"`
		Constraints []rowDoc `json:"constraints"`
	}
	var doc modelDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		tb.Fatalf("unmarshal %s: %v", relPath, err)
	}
	sense := Minimize
	if doc.Parameters.Sense == -1 {
		sense = Maximize
	}
	m := NewModel(doc.Parameters.Name, sense)
	vars := make(map[string]Var, len(doc.Variables))
	for _, vd := range doc.Variables {
		lb := math.Inf(-1)
		if vd.LowBound != nil {
			lb = *vd.LowBound
		}
		ub := math.Inf(1)
		if vd.UpBound != nil {
			ub = *vd.UpBound
		}
		var v Var
		switch vd.Cat {
		case "Binary":
			v = m.AddBinaryVar(vd.Name)
		case "Integer":
			v = m.AddIntVar(vd.Name, lb, ub)
		default:
			v = m.AddContVar(vd.Name, lb, ub)
		}
		vars[vd.Name] = v
	}
	objTerms := make([]Term, 0, len(doc.Objective.Coefficients))
	for _, oc := range doc.Objective.Coefficients {
		v, ok := vars[oc.Name]
		if !ok {
			tb.Fatalf("%s: unknown objective var %q", relPath, oc.Name)
		}
		objTerms = append(objTerms, v.T(oc.Value))
	}
	m.SetObjective(NewExpr(objTerms...))
	for _, cd := range doc.Constraints {
		terms := make([]Term, 0, len(cd.Coefficients))
		for _, tc := range cd.Coefficients {
			v, ok := vars[tc.Name]
			if !ok {
				tb.Fatalf("%s: unknown constraint var %q", relPath, tc.Name)
			}
			terms = append(terms, v.T(tc.Value))
		}
		rhs := -cd.Constant
		ex := NewExpr(terms...)
		switch cd.Sense {
		case -1:
			m.AddConstraint(cd.Name, ex.Le(rhs))
		case 1:
			m.AddConstraint(cd.Name, ex.Ge(rhs))
		default:
			m.AddConstraint(cd.Name, ex.Eq(rhs))
		}
	}
	p, err := m.CompileChecked()
	if err != nil {
		tb.Fatalf("compile %s: %v", relPath, err)
	}
	return p
}

// snapExactMIPSolution rounds integer coordinates to exact integers and cleans small
// continuous LP noise when zero-violation feasibility holds.
func snapExactMIPSolution(tb testing.TB, p *CompiledProblem, raw []float64) []float64 {
	tb.Helper()
	x := make([]float64, p.numVars)
	copy(x, raw)
	for j := range p.numVars {
		if p.varCat[j] != Continuous {
			x[j] = math.Round(x[j])
		} else {
			if !math.IsInf(p.varLB[j], -1) && math.Abs(x[j]-p.varLB[j]) < 1e-8 {
				x[j] = p.varLB[j]
			} else if !math.IsInf(p.varUB[j], 1) && math.Abs(x[j]-p.varUB[j]) < 1e-8 {
				x[j] = p.varUB[j]
			} else if r := math.Round(x[j]); math.Abs(x[j]-r) < 1e-8 {
				x[j] = r
			}
		}
	}
	bV, rV, iV := p.MaxViolation(x, true)
	if rV > 0 && rV < 1e-7 {
		// Step interior continuous variables slightly inward to exact dyadic grids if needed.
		for j := range p.numVars {
			if p.varCat[j] == Continuous && x[j] > p.varLB[j]+1e-6 {
				cand := math.Floor(x[j]*1024.0) / 1024.0
				if cand >= p.varLB[j] {
					x[j] = cand
				}
			}
		}
		bV, rV, iV = p.MaxViolation(x, true)
	}
	if bV != 0 || rV != 0 || iV != 0 {
		tb.Fatalf("%s snapped solution not exact: viol=(%g, %g, %g)", p.name, bV, rV, iV)
	}
	return x
}

func solveExactRootTableauRow(tb testing.TB, mat [][]*big.Rat, rowIdx int) []*big.Rat {
	tb.Helper()
	m := len(mat)
	aug := make([][]*big.Rat, m)
	for i := range m {
		aug[i] = make([]*big.Rat, m+1)
		for j := range m {
			// Solve B^T * rho = e_rowIdx using transpose columns.
			aug[i][j] = new(big.Rat).Set(mat[j][i])
		}
		if i == rowIdx {
			aug[i][m] = big.NewRat(1, 1)
		} else {
			aug[i][m] = new(big.Rat)
		}
	}
	for col := range m {
		piv := -1
		for r := col; r < m; r++ {
			if aug[r][col].Sign() != 0 {
				piv = r
				break
			}
		}
		if piv < 0 {
			tb.Fatalf("singular basis matrix at column %d", col)
		}
		if piv != col {
			aug[col], aug[piv] = aug[piv], aug[col]
		}
		invP := new(big.Rat).Inv(aug[col][col])
		for j := col; j <= m; j++ {
			aug[col][j].Mul(aug[col][j], invP)
		}
		for r := range m {
			if r == col || aug[r][col].Sign() == 0 {
				continue
			}
			fac := new(big.Rat).Set(aug[r][col])
			for j := col; j <= m; j++ {
				prod := new(big.Rat).Mul(fac, aug[col][j])
				aug[r][j].Sub(aug[r][j], prod)
			}
		}
	}
	rho := make([]*big.Rat, m)
	for i := range m {
		rho[i] = new(big.Rat).Set(aug[i][m])
	}
	return rho
}

func TestFormalTrack2B(t *testing.T) {
	t.Parallel()
	requireFormalEnv(t)
	exe := formalExe(t, "cuts-check")

	t.Run("UsageExitsCleanly", func(t *testing.T) {
		t.Parallel()
		cmd := exec.CommandContext(t.Context(), exe)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("cuts-check with no args failed: %v\n%s", err, out)
		}
		if !bytes.Contains(out, []byte("usage:")) {
			t.Fatalf("expected usage banner, got %q", string(out))
		}
	})

	t.Run("RowLegalMuSignParityAndAgg", func(t *testing.T) {
		t.Parallel()
		type muCase struct {
			Sense    string     `json:"sense"`
			Mu       ratPayload `json:"mu"`
			Expected bool       `json:"expected"`
		}
		rng := rand.New(rand.NewPCG(202611, 2201))
		senses := []ConstraintSense{LessEqual, Equal, GreaterEqual}
		fixedMus := []float64{
			0, math.Copysign(0, -1), 1, -1, 1e-9, -1e-9,
			math.SmallestNonzeroFloat64, -math.SmallestNonzeroFloat64, 1e4, -1e4,
		}
		cases := make([]muCase, 0, 630)
		for _, s := range senses {
			for _, mu := range fixedMus {
				cases = append(cases, muCase{
					Sense:    senseName(s),
					Mu:       mustRatPayload(t, mu),
					Expected: rowLegalMuSign(s, mu),
				})
			}
		}
		for len(cases) < 630 {
			s := senses[rng.IntN(len(senses))]
			var mu float64
			switch rng.IntN(5) {
			case 0:
				mu = 0
			case 1:
				mu = float64(rng.IntN(200)+1) * math.Ldexp(1, -10)
			case 2:
				mu = -float64(rng.IntN(200)+1) * math.Ldexp(1, -10)
			default:
				mu = (rng.Float64()*2 - 1) * 500
			}
			cases = append(cases, muCase{
				Sense:    senseName(s),
				Mu:       mustRatPayload(t, mu),
				Expected: rowLegalMuSign(s, mu),
			})
		}
		out := runCutsCheck(t, exe, "row-legal-mu", map[string]any{"cases": cases}, true)
		if !strings.HasPrefix(out, "VERIFIED") {
			t.Fatalf("unexpected row-legal-mu output: %s", out)
		}

		// Positive row aggregation check with materialized Go cut.
		m := NewModel("agg_pos", Minimize)
		x0 := m.AddIntVar("x0", 0, 10)
		x1 := m.AddContVar("x1", 0, 10)
		x2 := m.AddIntVar("x2", 0, 10)
		m.AddConstraint("r0", NewExpr(x0.T(2), x1.T(1)).Le(10))
		m.AddConstraint("r1", NewExpr(x1.T(1), x2.T(-2)).Ge(1))
		m.AddConstraint("r2", NewExpr(x0.T(1), x2.T(1)).Eq(4))
		p := m.Compile()

		ws := &cutGenWorkspace{}
		ws.ensureAgg(p.numVars, p.numConstraints)
		colGen := ws.nextAggColGen()
		var aggCols []int
		var rhsAcc dot2Acc
		mus := []float64{1.5, -1.5, 2.0}
		for r, mu := range mus {
			if !rowLegalMuSign(p.rowSense[r], mu) {
				t.Fatalf("expected legal mu=%g for row %d", mu, r)
			}
			addScaledProblemRowToAgg(p, r, mu, &rhsAcc, ws, colGen, &aggCols)
		}
		outCols, outVals, aggRHS, ok := materializeAggRow(p, rhsAcc, aggCols, 0, ws)
		if !ok {
			t.Fatal("materializeAggRow returned ok=false")
		}
		aggRows := make([]track2BAggRowPayload, p.numConstraints)
		mipRows := compiledToMIPPayload(t, p).Rows
		for r, mr := range mipRows {
			aggRows[r] = track2BAggRowPayload{
				Terms: mr.Terms,
				Sense: mr.Sense,
				RHS:   mr.RHS,
				Mu:    mustRatPayload(t, mus[r]),
			}
		}
		aggTerms := make([]track2BTermPayload, len(outCols))
		for k, col := range outCols {
			aggTerms[k] = track2BTermPayload{Col: col, Val: mustRatPayload(t, outVals[k])}
		}
		feasSols := [][]float64{{3, 3, 1}, {2, 5, 2}, {4, 1, 0}}
		for _, pt := range feasSols {
			if bV, rV, iV := p.MaxViolation(pt, true); bV != 0 || rV != 0 || iV != 0 {
				t.Fatalf("infeasible witness %v: (%g, %g, %g)", pt, bV, rV, iV)
			}
		}
		runCutsCheck(t, exe, "row-legal-mu", map[string]any{
			"rows":      aggRows,
			"aggCut":    track2BCutPayload{Terms: aggTerms, RHS: mustRatPayload(t, aggRHS)},
			"solutions": solutionsToPayloads(t, feasSols),
		}, true)

		// Negative controls: illegal multiplier signs on <=, >=, and = rows.
		badSingles := []map[string]any{
			{"sense": "le", "mu": mustRatPayload(t, 0)},
			{"sense": "le", "mu": mustRatPayload(t, -0.25)},
			{"sense": "ge", "mu": mustRatPayload(t, 0)},
			{"sense": "ge", "mu": mustRatPayload(t, 0.25)},
			{"sense": "eq", "mu": mustRatPayload(t, 0)},
		}
		for _, bad := range badSingles {
			runCutsCheck(t, exe, "row-legal-mu", bad, false)
		}
		badAgg := make([]track2BAggRowPayload, len(aggRows))
		copy(badAgg, aggRows)
		badAgg[1].Mu = mustRatPayload(t, 1.5)
		runCutsCheck(t, exe, "row-legal-mu", map[string]any{
			"rows":      badAgg,
			"solutions": solutionsToPayloads(t, feasSols),
		}, false)
	})

	t.Run("MirUseLowerParity", func(t *testing.T) {
		t.Parallel()
		type mirCase struct {
			X        ratPayload `json:"x"`
			L        ratPayload `json:"l"`
			U        ratPayload `json:"u"`
			HasL     bool       `json:"hasL"`
			HasU     bool       `json:"hasU"`
			Expected bool       `json:"expected"`
		}
		rng := rand.New(rand.NewPCG(202611, 2202))
		cases := make([]mirCase, 0, 620)
		edge := [][3]float64{
			{0, 0, math.Inf(1)},
			{5, math.Inf(-1), 10},
			{3, math.Inf(-1), math.Inf(1)},
			{5, 0, 10},
			{4.999999999999999, 0, 10},
			{5.000000000000001, 0, 10},
			{3, 3, 3},
			{-2, -4, 0},
		}
		for _, tc := range edge {
			cases = append(cases, mirCase{
				X:        mustRatPayload(t, tc[0]),
				L:        mustRatPayload(t, ternaryFinite(tc[1])),
				U:        mustRatPayload(t, ternaryFinite(tc[2])),
				HasL:     !math.IsInf(tc[1], -1),
				HasU:     !math.IsInf(tc[2], 1),
				Expected: mirUseLower(tc[0], tc[1], tc[2]),
			})
		}
		for len(cases) < 620 {
			// Use exact 1/16 dyadics in [-256, 256] so IEEE-754 subtraction matches exact Rat.
			lNum := rng.IntN(4096) - 2048
			span := rng.IntN(2048)
			l := float64(lNum) / 16.0
			u := float64(lNum+span) / 16.0
			var x float64
			switch rng.IntN(5) {
			case 0:
				u = math.Inf(1)
				x = l + float64(rng.IntN(100))/16.0
			case 1:
				l = math.Inf(-1)
				x = u - float64(rng.IntN(100))/16.0
			case 2:
				x = 0.5 * (l + u)
			default:
				x = float64(lNum+rng.IntN(span+1)) / 16.0
			}
			cases = append(cases, mirCase{
				X:        mustRatPayload(t, x),
				L:        mustRatPayload(t, ternaryFinite(l)),
				U:        mustRatPayload(t, ternaryFinite(u)),
				HasL:     !math.IsInf(l, -1),
				HasU:     !math.IsInf(u, 1),
				Expected: mirUseLower(x, l, u),
			})
		}
		out := runCutsCheck(t, exe, "mir-use-lower", map[string]any{"cases": cases}, true)
		if !strings.HasPrefix(out, "VERIFIED") {
			t.Fatalf("unexpected mir-use-lower output: %s", out)
		}

		// Negative control: mismatched expected decision.
		runCutsCheck(t, exe, "mir-use-lower", map[string]any{
			"x":        mustRatPayload(t, 2),
			"l":        mustRatPayload(t, 0),
			"u":        mustRatPayload(t, 10),
			"hasL":     true,
			"hasU":     true,
			"expected": false,
		}, false)
	})

	t.Run("GMICutsAndTableauSoundness", func(t *testing.T) {
		t.Parallel()

		// 1. Direct mixed-integer tableau check + upward nextUp weakening.
		beta := 11.0 / 4.0
		aI := []float64{3.0 / 4.0, -5.0 / 4.0}
		aC := []float64{1.0 / 2.0, -1.0 / 4.0}
		f0 := beta - math.Floor(beta) // 0.75
		cI := make([]float64, len(aI))
		for j, a := range aI {
			fk := a - math.Floor(a)
			var exact float64
			if fk <= f0 {
				exact = fk / f0
			} else {
				exact = (1 - fk) / (1 - f0)
			}
			cI[j] = nextUp(exact)
		}
		cC := make([]float64, len(aC))
		for k, g := range aC {
			var exact float64
			if g >= 0 {
				exact = g / f0
			} else {
				exact = (-g) / (1 - f0)
			}
			cC[k] = nextUp(exact)
		}
		// Enumerate feasible integer xI in [0..4]^2 and solve non-negative xC matching row.
		type tabSol struct {
			XB ratPayload   `json:"xB"`
			XI []ratPayload `json:"xI"`
			XC []ratPayload `json:"xC"`
		}
		var tabSols []tabSol
		for x0 := range 4 {
			for x1 := range 4 {
				for xB := -2; xB <= 5; xB++ {
					rem := beta - float64(xB) - aI[0]*float64(x0) - aI[1]*float64(x1)
					if rem >= 0 {
						tabSols = append(tabSols, tabSol{
							XB: int64ToPayload(int64(xB)),
							XI: []ratPayload{int64ToPayload(int64(x0)), int64ToPayload(int64(x1))},
							XC: []ratPayload{mustRatPayload(t, 2*rem), int64ToPayload(0)},
						})
					} else {
						tabSols = append(tabSols, tabSol{
							XB: int64ToPayload(int64(xB)),
							XI: []ratPayload{int64ToPayload(int64(x0)), int64ToPayload(int64(x1))},
							XC: []ratPayload{int64ToPayload(0), mustRatPayload(t, -4*rem)},
						})
					}
				}
			}
		}
		runCutsCheck(t, exe, "gmi-check", map[string]any{
			"beta":      mustRatPayload(t, beta),
			"aI":        float64SliceToPayloads(t, aI),
			"aC":        float64SliceToPayloads(t, aC),
			"cI":        float64SliceToPayloads(t, cI),
			"cC":        float64SliceToPayloads(t, cC),
			"solutions": tabSols,
		}, true)

		// Negative controls on GMI tableau.
		badCI := []float64{cI[0] * 0.9, cI[1]}
		runCutsCheck(t, exe, "gmi-check", map[string]any{
			"beta":      mustRatPayload(t, beta),
			"aI":        float64SliceToPayloads(t, aI),
			"aC":        float64SliceToPayloads(t, aC),
			"cI":        float64SliceToPayloads(t, badCI),
			"cC":        float64SliceToPayloads(t, cC),
			"solutions": tabSols[:3],
		}, false)
		runCutsCheck(t, exe, "gmi-check", map[string]any{
			"beta":      int64ToPayload(3),
			"aI":        float64SliceToPayloads(t, aI),
			"aC":        float64SliceToPayloads(t, aC),
			"solutions": []tabSol{{XB: int64ToPayload(3), XI: []ratPayload{int64ToPayload(0), int64ToPayload(0)}, XC: []ratPayload{int64ToPayload(0), int64ToPayload(0)}}},
		}, false)

		// 2. Exact rational LP basis extraction + Go separateGMICutsEx on a real MIP.
		mGMI := NewModel("gmi_exact_tab", Minimize)
		gx0 := mGMI.AddIntVar("x0", 0, 10)
		gx1 := mGMI.AddIntVar("x1", 0, 10)
		gx2 := mGMI.AddIntVar("x2", 0, 10)
		mGMI.SetObjective(NewExpr(gx0.T(-5), gx1.T(-4), gx2.T(-3)))
		mGMI.AddConstraint("c0", NewExpr(gx0.T(2), gx1.T(3), gx2.T(2)).Le(7))
		mGMI.AddConstraint("c1", NewExpr(gx0.T(3), gx1.T(2), gx2.T(4)).Le(7))
		pGMI := mGMI.Compile()

		var exactFeas [][]float64
		for v0 := range 5 {
			for v1 := range 5 {
				for v2 := range 5 {
					if 2*v0+3*v1+2*v2 <= 7 && 3*v0+2*v1+4*v2 <= 7 {
						exactFeas = append(exactFeas, []float64{float64(v0), float64(v1), float64(v2)})
					}
				}
			}
		}

		lpGMI := NewLPSolver(pGMI, nil)
		defer lpGMI.Close()
		lpRes := lpGMI.Solve()
		if lpRes.Status != StatusOptimal {
			t.Fatalf("gmi_exact_tab LP status=%v", lpRes.Status)
		}
		var cutWS cutGenWorkspace
		gmiCuts := separateGMICutsEx(lpGMI, lpRes.Primal, &cutWS, false)
		if len(gmiCuts) == 0 {
			t.Fatal("expected GMI cuts on gmi_exact_tab")
		}
		// Build exact rational tableau for row 0 (basis[0]) and check against feasible points.
		nCols := pGMI.numVars
		mRows := lpGMI.TotalRows()
		basisMat := make([][]*big.Rat, mRows)
		for i := range mRows {
			basisMat[i] = make([]*big.Rat, mRows)
			for j := range mRows {
				basisMat[i][j] = new(big.Rat)
			}
		}
		for j, bCol := range lpGMI.ws.basis[:mRows] {
			if bCol < nCols {
				for k := pGMI.colPtr[bCol]; k < pGMI.colPtr[bCol+1]; k++ {
					basisMat[pGMI.rowIdx[k]][j].SetFloat64(pGMI.colVal[k])
				}
			} else {
				// Standard LP slack column in Goblin satisfies Ax + s = b -> +1 in row bCol-nCols.
				basisMat[bCol-nCols][j].SetInt64(1)
			}
		}
		foundFracRow := false
		for r, bVar := range lpGMI.ws.basis[:mRows] {
			if bVar >= nCols || pGMI.varCat[bVar] == Continuous {
				continue
			}
			rho := solveExactRootTableauRow(t, basisMat, r)
			betaRat := new(big.Rat)
			for i := range mRows {
				rhsR := new(big.Rat).SetFloat64(pGMI.rowRHS[i])
				betaRat.Add(betaRat, new(big.Rat).Mul(rho[i], rhsR))
			}
			// Only integer vars at lower bound 0 are non-basic here (slacks at 0 too).
			var nbAI []*big.Rat
			var nbCols []int
			for k := range nCols + mRows {
				if k == bVar || lpGMI.ws.status[k] == statusBasic {
					continue
				}
				entry := new(big.Rat)
				if k < nCols {
					for ck := pGMI.colPtr[k]; ck < pGMI.colPtr[k+1]; ck++ {
						valR := new(big.Rat).SetFloat64(pGMI.colVal[ck])
						entry.Add(entry, new(big.Rat).Mul(rho[pGMI.rowIdx[ck]], valR))
					}
				} else {
					entry.Set(rho[k-nCols])
				}
				if lpGMI.ws.status[k] == statusAtUpper {
					ubR := new(big.Rat).SetFloat64(lpGMI.unscaledBound(k, true))
					betaRat.Sub(betaRat, new(big.Rat).Mul(entry, ubR))
					entry.Neg(entry)
				}
				nbAI = append(nbAI, entry)
				nbCols = append(nbCols, k)
			}
			if betaRat.IsInt() {
				continue
			}
			foundFracRow = true
			var exactTabSols []tabSol
			for _, pt := range exactFeas {
				sVals := []float64{
					7 - (2*pt[0] + 3*pt[1] + 2*pt[2]),
					7 - (3*pt[0] + 2*pt[1] + 4*pt[2]),
				}
				xBRat := new(big.Rat).SetFloat64(pt[bVar])
				xIList := make([]ratPayload, len(nbCols))
				for idx, k := range nbCols {
					var rawVal float64
					if k < nCols {
						rawVal = pt[k]
					} else {
						rawVal = sVals[k-nCols]
					}
					var nbVal *big.Rat
					if lpGMI.ws.status[k] == statusAtUpper {
						nbVal = new(big.Rat).SetFloat64(lpGMI.unscaledBound(k, true) - rawVal)
					} else {
						nbVal = new(big.Rat).SetFloat64(rawVal)
					}
					xIList[idx] = bigRatToPayload(nbVal)
				}
				exactTabSols = append(exactTabSols, tabSol{
					XB: bigRatToPayload(xBRat),
					XI: xIList,
					XC: []ratPayload{},
				})
			}
			runCutsCheck(t, exe, "gmi-check", map[string]any{
				"beta":      bigRatToPayload(betaRat),
				"aI":        bigRatSliceToPayloads(nbAI),
				"aC":        []ratPayload{},
				"solutions": exactTabSols,
			}, true)
			break
		}
		if !foundFracRow {
			t.Fatal("expected fractional basic integer row in gmi_exact_tab")
		}

		// Check real structural cuts from separateGMICutsEx on crafted & corpus MIPs.
		runCutsCheck(t, exe, "gmi-check", map[string]any{
			"mip":       compiledToMIPPayload(t, pGMI),
			"cuts":      cutsToPayloads(t, gmiCuts),
			"solutions": solutionsToPayloads(t, exactFeas),
		}, true)

		// Negative control: invalid structural cut cutting off feasible integer points (2, 0, 0) and (1, 1, 0).
		runCutsCheck(t, exe, "gmi-check", map[string]any{
			"mip": compiledToMIPPayload(t, pGMI),
			"cuts": []track2BCutPayload{{
				Terms: []track2BTermPayload{
					{Col: 0, Val: int64ToPayload(1)},
					{Col: 1, Val: int64ToPayload(1)},
					{Col: 2, Val: int64ToPayload(1)},
				},
				RHS: int64ToPayload(1),
			}},
			"solutions": solutionsToPayloads(t, exactFeas),
		}, false)

		// In-tree globalOnly=true GMI cuts must preserve all globally feasible points.
		mTree := NewModel("intree_global_gmi_formal", Minimize)
		ty := mTree.AddBinaryVar("y")
		tk := mTree.AddIntVar("k", 0, 5)
		tz := mTree.AddIntVar("z", 0, 5)
		tw := mTree.AddContVar("w", 0, 10)
		mTree.SetObjective(NewExpr(tz.T(-5), tw.T(-1), tk.T(0), ty.T(1)))
		mTree.AddConstraint("c0", NewExpr(tz.T(2), tk.T(1), tw.T(1), ty.T(1)).Eq(5))
		mTree.AddConstraint("c1", NewExpr(tw.T(1), tz.T(-1)).Ge(0))
		pTree := mTree.Compile()
		var globalFeas [][]float64
		for yV := range 2 {
			for kV := range 6 {
				for zV := range 6 {
					wV := 5 - 2*zV - kV - yV
					if wV >= zV && wV <= 10 {
						globalFeas = append(globalFeas, []float64{float64(yV), float64(kV), float64(zV), float64(wV)})
					}
				}
			}
		}
		lpTree := NewLPSolver(pTree, nil)
		defer lpTree.Close()
		lpTree.SetStructuralBounds([]float64{1, 0, 0, 0}, []float64{1, 0, 5, 10})
		resTree := lpTree.Solve()
		if resTree.Status != StatusOptimal {
			t.Fatalf("intree node LP status=%v", resTree.Status)
		}
		treeCuts := separateGMICutsEx(lpTree, resTree.Primal, &cutWS, true)
		if len(treeCuts) == 0 {
			t.Fatal("expected globally valid in-tree GMI cut")
		}
		runCutsCheck(t, exe, "gmi-check", map[string]any{
			"mip":       compiledToMIPPayload(t, pTree),
			"cuts":      cutsToPayloads(t, treeCuts),
			"solutions": solutionsToPayloads(t, globalFeas),
		}, true)

		// Corpus & public MIPs: t1_std_mip, t1_furniture, t1_knapsack_12, miplib_p0033.
		type corpusGMI struct {
			path string
			sols [][]float64
		}
		corpusPaths := []string{
			"testdata/corpus/t1_furniture.json",
			"testdata/corpus/t1_std_mip.json",
			"testdata/corpus/t1_knapsack_12.json",
			"testdata/public/miplib_p0033.json",
		}
		var checkedCorpus int
		var items []corpusGMI
		for _, relPath := range corpusPaths {
			pC := loadLocalPuLPJSONModel(t, relPath)
			resC, err := SolveCompiled(t.Context(), pC)
			if err != nil || resC.Status != StatusOptimal {
				t.Fatalf("solve %s: status=%v err=%v", relPath, resC.Status, err)
			}
			sol := snapExactMIPSolution(t, pC, resC.Values)
			items = append(items, corpusGMI{path: relPath, sols: [][]float64{sol}})
		}
		for _, it := range items {
			pC := loadLocalPuLPJSONModel(t, it.path)
			lpC := NewLPSolver(pC, nil)
			rootC := lpC.Solve()
			if rootC.Status == StatusOptimal {
				cutsC := separateGMICutsEx(lpC, rootC.Primal, nil, false)
				if len(cutsC) > 0 {
					checkedCorpus++
					runCutsCheck(t, exe, "gmi-check", map[string]any{
						"mip":       compiledToMIPPayload(t, pC),
						"cuts":      cutsToPayloads(t, cutsC),
						"solutions": solutionsToPayloads(t, it.sols),
					}, true)
				}
			}
			lpC.Close()
		}
		if checkedCorpus < 2 {
			t.Fatalf("expected GMI cuts on >= 2 corpus MIPs, got %d", checkedCorpus)
		}
	})

	t.Run("CMIRCutsAndSafeRoundingSoundness", func(t *testing.T) {
		t.Parallel()

		// 1. Safe-rounded cMIR certificate modeling cuts.go:separateCMIRFromSparseRow (CRIT4-style).
		delta := 5.0 + math.Ldexp(1.0, -50)
		a1 := 15.0 + math.Ldexp(1.0, -49)
		a2 := -(10.0 + math.Ldexp(1.0, -49))
		u1 := math.Ldexp(1.0, 49)
		u2 := 3.0 * math.Ldexp(1.0, 48)
		rhs := 4.9

		lpPrimalI := []float64{4.9 / delta, 0, 0}
		useLowerI := []bool{
			mirUseLower(lpPrimalI[0], 0, 1),
			mirUseLower(lpPrimalI[1], 0, u1),
			mirUseLower(lpPrimalI[2], 0, u2),
		}
		var betaAcc dot2Acc
		betaAcc.addVal(rhs)
		if !useLowerI[0] {
			betaAcc.addProd(-delta, 1)
		}
		if !useLowerI[1] {
			betaAcc.addProd(-a1, u1)
		}
		if !useLowerI[2] {
			betaAcc.addProd(-a2, u2)
		}
		bVal, bErr, okB := betaAcc.result()
		if !okB {
			t.Fatal("betaAcc failed")
		}
		betaHat := (bVal + bErr) / delta
		betaHat += 4.0 * unitRoundoff64 * math.Abs(betaHat)
		b0 := math.Floor(betaHat)
		f0 := betaHat - b0
		intDenHi := nextUp(1.0 - f0)
		aVec := []float64{delta, a1, a2}
		uVec := []float64{1, u1, u2}
		alphaHat := make([]float64, 3)
		cI := make([]float64, 3)
		var cutRHSAcc dot2Acc
		cutRHSAcc.addVal(b0)
		for j := range 3 {
			num := aVec[j]
			if !useLowerI[j] {
				num = -num
			}
			al := num / delta
			if al != 0 {
				al = nextDown(al)
			}
			alphaHat[j] = al
			flA := math.Floor(al)
			fj := al - flA
			coeff := flA
			if fj > f0 {
				numLo := nextDown(fj - f0)
				if numLo > 0 {
					fracLo := nextDown(numLo / intDenHi)
					if fracLo > 0 {
						coeff = nextDown(flA + fracLo)
					}
				}
			}
			cI[j] = coeff
			if useLowerI[j] {
				cutRHSAcc.addProd(coeff, 0)
			} else {
				cutRHSAcc.addProd(-coeff, uVec[j])
			}
		}
		cRHSVal, cRHSErr, okCRHS := cutRHSAcc.result()
		if !okCRHS {
			t.Fatal("cutRHSAcc failed")
		}
		cutRHS := cRHSVal + cRHSErr
		if cRHSErr > 0 {
			cutRHS = nextUp(cutRHS)
		}
		type cmirSol struct {
			XI []ratPayload `json:"xI"`
			XC []ratPayload `json:"xC"`
		}
		cmirSols := []cmirSol{
			{XI: []ratPayload{int64ToPayload(1), mustRatPayload(t, u1), mustRatPayload(t, u2)}, XC: []ratPayload{}},
			{XI: []ratPayload{int64ToPayload(0), int64ToPayload(0), int64ToPayload(0)}, XC: []ratPayload{}},
		}
		cmirCertPayload := map[string]any{
			"aI":        float64SliceToPayloads(t, aVec),
			"aC":        []ratPayload{},
			"lbI":       []*ratPayload{mustOptRatPayload(t, 0), mustOptRatPayload(t, 0), mustOptRatPayload(t, 0)},
			"ubI":       []*ratPayload{mustOptRatPayload(t, 1), mustOptRatPayload(t, u1), mustOptRatPayload(t, u2)},
			"lbC":       []*ratPayload{},
			"ubC":       []*ratPayload{},
			"useLowerI": useLowerI,
			"useLowerC": []bool{},
			"lpPrimalI": float64SliceToPayloads(t, lpPrimalI),
			"lpPrimalC": []ratPayload{},
			"rhs":       mustRatPayload(t, rhs),
			"delta":     mustRatPayload(t, delta),
			"betaHat":   mustRatPayload(t, betaHat),
			"alphaHat":  float64SliceToPayloads(t, alphaHat),
			"cI":        float64SliceToPayloads(t, cI),
			"cC":        []ratPayload{},
			"cutRHS":    mustRatPayload(t, cutRHS),
			"solutions": cmirSols,
		}
		runCutsCheck(t, exe, "cmir-check", cmirCertPayload, true)

		// Mixed-integer complementation check with one-sided bounds and continuous terms.
		runCutsCheck(t, exe, "cmir-check", map[string]any{
			"aI":        []ratPayload{int64ToPayload(3), int64ToPayload(-2)},
			"aC":        []ratPayload{int64ToPayload(1), int64ToPayload(-2)},
			"lbI":       []*ratPayload{mustOptRatPayload(t, 0), nil},
			"ubI":       []*ratPayload{mustOptRatPayload(t, 4), mustOptRatPayload(t, 5)},
			"lbC":       []*ratPayload{mustOptRatPayload(t, 0), mustOptRatPayload(t, 0)},
			"ubC":       []*ratPayload{nil, mustOptRatPayload(t, 3)},
			"useLowerI": []bool{true, false},
			"useLowerC": []bool{true, true},
			"rhs":       ratPayload{Num: "11", Den: "2"},
			"delta":     int64ToPayload(3),
			"solutions": []cmirSol{
				{XI: []ratPayload{int64ToPayload(1), int64ToPayload(2)}, XC: []ratPayload{int64ToPayload(0), int64ToPayload(0)}},
				{XI: []ratPayload{int64ToPayload(2), int64ToPayload(1)}, XC: []ratPayload{int64ToPayload(1), int64ToPayload(2)}},
			},
		}, true)

		// Negative controls for cMIR safe rounding.
		badCertCoeff := maps.Clone(cmirCertPayload)
		badCI := []float64{cI[0] + 0.5, cI[1], cI[2]}
		badCertCoeff["cI"] = float64SliceToPayloads(t, badCI)
		runCutsCheck(t, exe, "cmir-check", badCertCoeff, false)

		badCertBeta := maps.Clone(cmirCertPayload)
		badCertBeta["betaHat"] = mustRatPayload(t, betaHat-1.0)
		runCutsCheck(t, exe, "cmir-check", badCertBeta, false)

		// 2. Real separateCMIRFromSparseRow cuts on crafted and corpus MIPs.
		mMIR := NewModel("mir_direct_formal", Maximize)
		mx0 := mMIR.AddBinaryVar("x0")
		mx1 := mMIR.AddIntVar("x1", 0, u1)
		mx2 := mMIR.AddIntVar("x2", 0, u2)
		mMIR.AddConstraint("r", NewExpr(mx0.T(delta), mx1.T(a1), mx2.T(a2)).Le(rhs))
		pMIR := mMIR.Compile()
		var mirWS cutGenWorkspace
		mirCuts := separateCMIRFromSparseRow(pMIR, []int{0, 1, 2}, []float64{delta, a1, a2}, rhs, 1.0, lpPrimalI, 0, maxMIRDivisors, &mirWS, nil)
		if len(mirCuts) == 0 {
			t.Fatal("expected cMIR cut from separateCMIRFromSparseRow")
		}
		runCutsCheck(t, exe, "cmir-check", map[string]any{
			"mip":       compiledToMIPPayload(t, pMIR),
			"cuts":      cutsToPayloads(t, mirCuts),
			"solutions": solutionsToPayloads(t, [][]float64{{1, u1, u2}, {0, 0, 0}, {1, 1, 2}}),
		}, true)

		// Negative control: violated structural cut on feasible integer witness.
		runCutsCheck(t, exe, "cmir-check", map[string]any{
			"mip": compiledToMIPPayload(t, pMIR),
			"cuts": []track2BCutPayload{{
				Terms: []track2BTermPayload{{Col: 0, Val: int64ToPayload(1)}},
				RHS:   int64ToPayload(0),
			}},
			"solutions": solutionsToPayloads(t, [][]float64{{1, u1, u2}}),
		}, false)

		// Corpus MIPs with separateRowMIRCuts.
		var mirCorpusChecked int
		for _, relPath := range []string{
			"testdata/corpus/t1_knapsack_12.json",
			"testdata/corpus/t2_binpack_44.json",
			"testdata/corpus/t2_mkp_30.json",
			"testdata/public/miplib_p0033.json",
		} {
			pC := loadLocalPuLPJSONModel(t, relPath)
			resC, err := SolveCompiled(t.Context(), pC)
			if err != nil || resC.Status != StatusOptimal {
				t.Fatalf("solve %s: status=%v err=%v", relPath, resC.Status, err)
			}
			sol := snapExactMIPSolution(t, pC, resC.Values)
			lpC := NewLPSolver(pC, nil)
			rootC := lpC.Solve()
			if rootC.Status == StatusOptimal {
				cutsC := separateRowMIRCuts(pC, rootC.Primal)
				if len(cutsC) > 0 {
					mirCorpusChecked++
					runCutsCheck(t, exe, "cmir-check", map[string]any{
						"mip":       compiledToMIPPayload(t, pC),
						"cuts":      cutsToPayloads(t, cutsC),
						"solutions": solutionsToPayloads(t, [][]float64{sol}),
					}, true)
				}
			}
			lpC.Close()
		}
		if mirCorpusChecked < 2 {
			t.Fatalf("expected cMIR cuts on >= 2 corpus MIPs, got %d", mirCorpusChecked)
		}
	})

	t.Run("VUBDisaggregationSoundness", func(t *testing.T) {
		t.Parallel()

		// 1. Direct VUB disaggregation certificate checks (finite & infinite upper bounds).
		type vubSol struct {
			X []ratPayload `json:"x"`
			Y ratPayload   `json:"y"`
		}
		vubSols := []vubSol{
			{X: []ratPayload{int64ToPayload(0), int64ToPayload(0)}, Y: int64ToPayload(0)},
			{X: []ratPayload{int64ToPayload(6), int64ToPayload(0)}, Y: int64ToPayload(1)},
			{X: []ratPayload{int64ToPayload(2), int64ToPayload(5)}, Y: int64ToPayload(1)},
		}
		// 1*x0 + 2*x1 - 12*y <= 0 with u = [6, +Inf]; min(6, 12/1)=6, min(+Inf, 12/2)=6.
		runCutsCheck(t, exe, "vub-check", map[string]any{
			"a":         []ratPayload{int64ToPayload(1), int64ToPayload(2)},
			"u":         []*ratPayload{mustOptRatPayload(t, 6), nil},
			"cap":       int64ToPayload(12),
			"rhsRow":    int64ToPayload(0),
			"targetIdx": 0,
			"effUB":     int64ToPayload(6),
			"cutRHS":    int64ToPayload(0),
			"solutions": vubSols,
		}, true)
		runCutsCheck(t, exe, "vub-check", map[string]any{
			"a":         []ratPayload{int64ToPayload(1), int64ToPayload(2)},
			"u":         []*ratPayload{mustOptRatPayload(t, 6), nil},
			"cap":       int64ToPayload(12),
			"rhsRow":    int64ToPayload(0),
			"targetIdx": 1,
			"effUB":     int64ToPayload(6),
			"cutRHS":    int64ToPayload(0),
			"solutions": vubSols,
		}, true)

		// Negative controls: violated min bound (effUB < min(u_k, C/a_k)) and violated point.
		runCutsCheck(t, exe, "vub-check", map[string]any{
			"a":         []ratPayload{int64ToPayload(1), int64ToPayload(2)},
			"u":         []*ratPayload{mustOptRatPayload(t, 6), nil},
			"cap":       int64ToPayload(12),
			"rhsRow":    int64ToPayload(0),
			"targetIdx": 0,
			"effUB":     int64ToPayload(5),
			"cutRHS":    int64ToPayload(0),
			"solutions": vubSols[:1],
		}, false)
		runCutsCheck(t, exe, "vub-check", map[string]any{
			"a":         []ratPayload{int64ToPayload(1), int64ToPayload(2)},
			"u":         []*ratPayload{mustOptRatPayload(t, 6), nil},
			"cap":       int64ToPayload(12),
			"rhsRow":    int64ToPayload(0),
			"targetIdx": 1,
			"effUB":     int64ToPayload(5),
			"cutRHS":    int64ToPayload(0),
			"solutions": vubSols[:1],
		}, false)

		// 2. Real separateFlowAndVUBCuts on crafted MIP with disaggregated VUB & flow chains.
		mVUB := NewModel("vub_flow_formal", Minimize)
		const nPer = 4
		yBin := make([]Var, nPer)
		pCont := make([]Var, nPer)
		sCont := make([]Var, nPer)
		for idx := range nPer {
			yBin[idx] = mVUB.AddBinaryVar(fmt.Sprintf("y%d", idx))
			pCont[idx] = mVUB.AddContVar(fmt.Sprintf("p%d", idx), 0, 50)
			sCont[idx] = mVUB.AddContVar(fmt.Sprintf("s%d", idx), 0, 50)
		}
		zBin := mVUB.AddBinaryVar("z0")
		z1 := mVUB.AddContVar("z1", 0, 6)
		z2 := mVUB.AddContVar("z2", 0, 6)
		mVUB.SetObjective(NewExpr(yBin[0].T(10), yBin[1].T(10), yBin[2].T(10), yBin[3].T(10), zBin.T(5)))
		for idx := range nPer {
			mVUB.AddConstraint(fmt.Sprintf("vub%d", idx), NewExpr(pCont[idx].T(1), yBin[idx].T(-40)).Le(0))
			if idx == 0 {
				mVUB.AddConstraint("bal0", NewExpr(pCont[0].T(1), sCont[0].T(-1)).Eq(10))
			} else {
				mVUB.AddConstraint(fmt.Sprintf("bal%d", idx), NewExpr(sCont[idx-1].T(1), pCont[idx].T(1), sCont[idx].T(-1)).Eq(10))
			}
		}
		mVUB.AddConstraint("disagg_vub", NewExpr(z1.T(1), z2.T(2), zBin.T(-12)).Le(0))
		mVUB.AddConstraint("z_dem", NewExpr(z1.T(1), z2.T(1)).Ge(4))
		pVUB := mVUB.Compile()

		lpPt := make([]float64, pVUB.numVars)
		for idx := range nPer {
			lpPt[yBin[idx].Index()] = 0.25
			lpPt[pCont[idx].Index()] = 10.0
		}
		lpPt[zBin.Index()] = 0.4
		lpPt[z1.Index()] = 4.0
		vubCuts := separateFlowAndVUBCuts(pVUB, lpPt, nil)
		if len(vubCuts) == 0 {
			t.Fatal("expected flow/VUB cuts on vub_flow_formal")
		}
		// Exact feasible mixed-integer schedules for vub_flow_formal.
		pt1 := make([]float64, pVUB.numVars)
		pt1[yBin[0].Index()] = 1
		pt1[pCont[0].Index()] = 40
		pt1[sCont[0].Index()] = 30
		pt1[sCont[1].Index()] = 20
		pt1[sCont[2].Index()] = 10
		pt1[zBin.Index()] = 1
		pt1[z1.Index()] = 4
		pt2 := make([]float64, pVUB.numVars)
		pt2[yBin[0].Index()] = 1
		pt2[yBin[2].Index()] = 1
		pt2[pCont[0].Index()] = 20
		pt2[sCont[0].Index()] = 10
		pt2[pCont[2].Index()] = 20
		pt2[sCont[2].Index()] = 10
		pt2[zBin.Index()] = 1
		pt2[z2.Index()] = 4
		runCutsCheck(t, exe, "vub-check", map[string]any{
			"mip":       compiledToMIPPayload(t, pVUB),
			"cuts":      cutsToPayloads(t, vubCuts),
			"solutions": solutionsToPayloads(t, [][]float64{pt1, pt2}),
		}, true)

		// Negative control: structural VUB cut evaluated on invalid point violating cut.
		badPt := make([]float64, pVUB.numVars)
		copy(badPt, pt1)
		badPt[zBin.Index()] = 0
		runCutsCheck(t, exe, "vub-check", map[string]any{
			"cuts":      cutsToPayloads(t, vubCuts),
			"solutions": solutionsToPayloads(t, [][]float64{badPt}),
		}, false)

		// Corpus VUB/lot-sizing/CFLP instances with separateFlowAndVUBCuts.
		var vubCorpusChecked int
		for _, relPath := range []string{
			"testdata/corpus/t2_cflp_36.json",
			"testdata/corpus/t2_lotsizing_48.json",
		} {
			pC := loadLocalPuLPJSONModel(t, relPath)
			resC, err := SolveCompiled(t.Context(), pC)
			if err != nil || resC.Status != StatusOptimal {
				t.Fatalf("solve %s: status=%v err=%v", relPath, resC.Status, err)
			}
			sol := snapExactMIPSolution(t, pC, resC.Values)
			lpC := NewLPSolver(pC, nil)
			rootC := lpC.Solve()
			if rootC.Status == StatusOptimal {
				cutsC := separateFlowAndVUBCuts(pC, rootC.Primal, nil)
				if len(cutsC) > 0 {
					vubCorpusChecked++
					runCutsCheck(t, exe, "vub-check", map[string]any{
						"mip":       compiledToMIPPayload(t, pC),
						"cuts":      cutsToPayloads(t, cutsC),
						"solutions": solutionsToPayloads(t, [][]float64{sol}),
					}, true)
				}
			}
			lpC.Close()
		}
		if vubCorpusChecked == 0 {
			t.Fatal("expected flow/VUB cuts on at least one corpus instance")
		}
	})
}

func ternaryFinite(v float64) float64 {
	if math.IsInf(v, 0) {
		return 0
	}
	return v
}
