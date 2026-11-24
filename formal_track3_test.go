package goblin

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type lpTermPayload struct {
	Col   int        `json:"col"`
	Coeff ratPayload `json:"coeff"`
}

type lpRowPayload struct {
	Sense string          `json:"sense"`
	RHS   ratPayload      `json:"rhs"`
	Terms []lpTermPayload `json:"terms"`
}

type lpCertPayload struct {
	Mode      string         `json:"mode"`
	NumVars   int            `json:"numVars"`
	NumRows   int            `json:"numRows"`
	ObjConst  ratPayload     `json:"objConst"`
	Obj       []ratPayload   `json:"obj"`
	LB        []*ratPayload  `json:"lb"`
	UB        []*ratPayload  `json:"ub"`
	Rows      []lpRowPayload `json:"rows"`
	Col       *int           `json:"col,omitzero"`
	X         []ratPayload   `json:"x,omitzero"`
	Y         []ratPayload   `json:"y,omitzero"`
	YBits     []uint64       `json:"yBits,omitzero"`
	W         []ratPayload   `json:"w,omitzero"`
	PrimalObj *ratPayload    `json:"primalObj,omitzero"`
	GapTol    *ratPayload    `json:"gapTol,omitzero"`
}

func compiledProblemToLPPayload(tb testing.TB, p *CompiledProblem, mode string) lpCertPayload {
	tb.Helper()
	payload, ok := exportCompiledProblemToLPPayload(p, mode)
	if !ok {
		tb.Fatalf("exportCompiledProblemToLPPayload(%s) failed", mode)
	}
	return payload
}

func runLPCheck(tb testing.TB, exe, name string, payload lpCertPayload, printBound bool) (string, error) {
	tb.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		tb.Fatalf("marshal %s payload: %v", name, err)
	}
	path := filepath.Join(tb.TempDir(), name+".json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		tb.Fatalf("write %s payload: %v", name, err)
	}
	args := make([]string, 0, 2)
	if printBound {
		args = append(args, "--print-bound")
	}
	args = append(args, path)
	cmd := exec.CommandContext(tb.Context(), exe, args...)
	out, runErr := cmd.CombinedOutput()
	return string(out), runErr
}

func evalMinObjRat(p *CompiledProblem, x []float64) (*big.Rat, error) {
	objConst, objMinVec, _, _, _, _, ok := ExactProblemRatsForTest(p)
	if !ok || len(objMinVec) != p.NumVars() {
		return nil, errors.New("extract exact objective rationals failed")
	}
	sum := new(big.Rat).Set(objConst)
	term := new(big.Rat)
	xj := new(big.Rat)
	for j := range p.NumVars() {
		if objMinVec[j].Sign() == 0 || x[j] == 0 {
			continue
		}
		if xj.SetFloat64(x[j]) == nil {
			return nil, fmt.Errorf("non-finite objective variable at col %d", j)
		}
		term.Mul(objMinVec[j], xj)
		sum.Add(sum, term)
	}
	return sum, nil
}

func assertLPCheckVerifiedMode(tb testing.TB, name, out, wantMode string) {
	tb.Helper()
	firstLine, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	if strings.TrimSpace(firstLine) != "VERIFIED" {
		tb.Fatalf("%s expected first line VERIFIED, got %q", name, out)
	}
	if wantMode != "" && !strings.Contains(out, "MODE "+wantMode) {
		tb.Fatalf("%s expected MODE %s in lp-check output %q", name, wantMode, out)
	}
}

func assertSafeDualBoundTight(tb testing.TB, exe, name string, p *CompiledProblem, primal []float64, yRats []ratPayload, yBits []uint64) {
	tb.Helper()
	payload := compiledProblemToLPPayload(tb, p, "safe-bound")
	payload.Y = yRats
	out, runErr := runLPCheck(tb, exe, name+"_safe", payload, true)
	if runErr != nil {
		tb.Fatalf("%s safe-bound failed: err=%v out=%q", name, runErr, out)
	}
	assertLPCheckVerifiedMode(tb, name+"_safe", out, "safe-bound")
	lbRat := parseBoundFromLPCheck(tb, out)
	primalObjRat, err := evalMinObjRat(p, primal)
	if err != nil {
		tb.Fatalf("%s exact primal objective: %v", name, err)
	}

	tol := new(big.Rat).SetFloat64(1e-6)
	scale := new(big.Rat).Abs(primalObjRat)
	one := big.NewRat(1, 1)
	if scale.Cmp(one) < 0 {
		scale.Set(one)
	}
	tol.Mul(tol, scale)

	// Allow ~1e-12 slack for floating-point primal row residual against exact rational dual bound.
	feasSlack := new(big.Rat).Mul(big.NewRat(1, 1_000_000_000_000), scale)
	gap := new(big.Rat).Sub(primalObjRat, lbRat)
	negSlack := new(big.Rat).Neg(feasSlack)
	if gap.Cmp(negSlack) < 0 || gap.Cmp(tol) > 0 {
		lbF, _ := lbRat.Float64()
		pF, _ := primalObjRat.Float64()
		tb.Fatalf("%s certified bound %.12g vs primal %.12g out of range (gap=%s)", name, lbF, pF, gap.RatString())
	}

	if yBits != nil {
		payloadBits := compiledProblemToLPPayload(tb, p, "safe-bound")
		payloadBits.YBits = yBits
		outBits, bitsErr := runLPCheck(tb, exe, name+"_safe_bits", payloadBits, false)
		if bitsErr != nil {
			tb.Fatalf("%s yBits safe-bound failed: err=%v out=%q", name, bitsErr, outBits)
		}
		assertLPCheckVerifiedMode(tb, name+"_safe_bits", outBits, "safe-bound")
	}
}

func parseBoundFromLPCheck(tb testing.TB, out string) *big.Rat {
	tb.Helper()
	for line := range strings.SplitSeq(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if valStr, ok := strings.CutPrefix(trimmed, "BOUND "); ok {
			r, okParse := new(big.Rat).SetString(strings.TrimSpace(valStr))
			if !okParse {
				tb.Fatalf("parse bound %q from lp-check output %q", valStr, out)
			}
			return r
		}
	}
	tb.Fatalf("missing BOUND line in lp-check output %q", out)
	return nil
}

func minDualsToPayloads(tb testing.TB, p *CompiledProblem, duals []float64) ([]ratPayload, []uint64) {
	tb.Helper()
	sign := 1.0
	if p.Sense() == Maximize {
		sign = -1.0
	}
	m := p.NumConstraints()
	if len(duals) != m {
		tb.Fatalf("duals length %d != %d", len(duals), m)
	}
	rats := make([]ratPayload, m)
	bits := make([]uint64, m)
	for i := range m {
		yi := sign * duals[i]
		if yi == 0 {
			yi = 0
		}
		rp, err := float64ToPayload(yi)
		if err != nil {
			tb.Fatalf("encode dual[%d]: %v", i, err)
		}
		rats[i] = rp
		bits[i] = math.Float64bits(yi)
	}
	return rats, bits
}

func buildStdLPModel(name string, sense Sense) *Model {
	m := NewModel(name, sense)
	x := m.AddContVar("x", 0, 4)
	y := m.AddContVar("y", -1, 1)
	z := m.AddContVar("z", 0, 100)
	m.SetObjective(NewExpr(x.T(1), y.T(4), z.T(9)))
	m.AddConstraint("c1", Le(NewExpr(x.T(1), y.T(1)), 5))
	m.AddConstraint("c2", Ge(NewExpr(x.T(1), z.T(1)), 10))
	m.AddConstraint("c3", Eq(NewExpr(y.T(-1), z.T(1)), 7))
	return m
}

func buildWhiskasLPModel() *Model {
	m := NewModel("t1_whiskas", Minimize)
	x1 := m.AddContVar("Chicken", 0, 100)
	x2 := m.AddContVar("Beef", 0, 100)
	x3 := m.AddContVar("Mutton", 0, 100)
	x4 := m.AddContVar("Rice", 0, 100)
	x5 := m.AddContVar("Wheat", 0, 100)
	x6 := m.AddContVar("Gel", 0, 100)
	m.SetObjective(NewExpr(
		x1.T(0.013), x2.T(0.008), x3.T(0.010),
		x4.T(0.002), x5.T(0.005), x6.T(0.001),
	))
	m.AddConstraint("Ingr", Eq(NewExpr(x1.T(1), x2.T(1), x3.T(1), x4.T(1), x5.T(1), x6.T(1)), 100))
	m.AddConstraint("Prot", Ge(NewExpr(x1.T(0.100), x2.T(0.200), x3.T(0.150), x4.T(0.000), x5.T(0.040)), 8.0))
	m.AddConstraint("Fat", Ge(NewExpr(x1.T(0.080), x2.T(0.100), x3.T(0.110), x4.T(0.010), x5.T(0.010)), 6.0))
	m.AddConstraint("Fibre", Le(NewExpr(x1.T(0.001), x2.T(0.005), x3.T(0.003), x4.T(0.100), x5.T(0.150)), 2.0))
	m.AddConstraint("Salt", Le(NewExpr(x1.T(0.002), x2.T(0.005), x3.T(0.007), x4.T(0.002), x5.T(0.008)), 0.4))
	return m
}

func isExactRatFarkasValid(p *CompiledProblem, w []*big.Rat) bool {
	n := p.numVars
	m := p.numConstraints
	if len(w) != m {
		return false
	}
	projW := make([]*big.Rat, m)
	wDotB := new(big.Rat)
	tmp := new(big.Rat)
	for i := range m {
		wi := new(big.Rat).Set(w[i])
		switch p.rowSense[i] {
		case LessEqual:
			if wi.Sign() > 0 {
				wi.SetInt64(0)
			}
		case GreaterEqual:
			if wi.Sign() < 0 {
				wi.SetInt64(0)
			}
		}
		projW[i] = wi
		if wi.Sign() != 0 {
			rhs, okR := exactRowRHSRat(p, i)
			if !okR {
				return false
			}
			if rhs.Sign() != 0 {
				tmp.Mul(rhs, wi)
				wDotB.Add(wDotB, tmp)
			}
		}
	}

	colRows, colVals, okMat := exactColMatrixRat(p)
	if !okMat {
		return false
	}
	maxLHS := new(big.Rat)
	wa := new(big.Rat)
	for j := range n {
		wa.SetInt64(0)
		for idx, r := range colRows[j] {
			if uint(r) < uint(m) && projW[r].Sign() != 0 && colVals[j][idx].Sign() != 0 {
				tmp.Mul(colVals[j][idx], projW[r])
				wa.Add(wa, tmp)
			}
		}
		sgn := wa.Sign()
		if sgn > 0 {
			if math.IsInf(p.varUB[j], 1) {
				return false
			}
			tmp.SetFloat64(p.varUB[j])
			tmp.Mul(tmp, wa)
			maxLHS.Add(maxLHS, tmp)
		} else if sgn < 0 {
			if math.IsInf(p.varLB[j], -1) {
				return false
			}
			tmp.SetFloat64(p.varLB[j])
			tmp.Mul(tmp, wa)
			maxLHS.Add(maxLHS, tmp)
		}
	}
	return maxLHS.Cmp(wDotB) < 0
}

func tryFloatBTRANFarkasRay(s *LPSolver, d []float64) ([]*big.Rat, bool) {
	p := s.prob
	m := p.numConstraints
	if s.TotalRows() != m || len(d) < m || !s.luValid {
		return nil, false
	}
	rho := slices.Clone(d[:m])
	s.ws.lu.BTRANExact(rho)
	w := make([]*big.Rat, m)
	for i := range m {
		wi := rho[i] * s.rowScaleOf(i)
		if math.IsNaN(wi) || math.IsInf(wi, 0) {
			return nil, false
		}
		w[i] = new(big.Rat).SetFloat64(wi)
	}
	if isExactRatFarkasValid(p, w) {
		return w, true
	}
	return nil, false
}

func solveExactBasisTranspose(p *CompiledProblem, basis []int, eqRHS []*big.Rat) ([]*big.Rat, bool) {
	n := p.numVars
	m := p.numConstraints
	if len(basis) < m || len(eqRHS) < m {
		return nil, false
	}
	colRows, colVals, okMat := exactColMatrixRat(p)
	if !okMat {
		return nil, false
	}

	eqEntries := make([][]ratEqEntry, m)
	for q := range m {
		bVar := basis[q]
		if bVar < n {
			var ents []ratEqEntry
			for idx, ri := range colRows[bVar] {
				if uint(ri) < uint(m) && colVals[bVar][idx].Sign() != 0 {
					ents = append(ents, ratEqEntry{col: ri, val: new(big.Rat).Set(colVals[bVar][idx])})
				}
			}
			eqEntries[q] = ents
		} else {
			si := bVar - n
			if uint(si) >= uint(m) {
				return nil, false
			}
			eqEntries[q] = []ratEqEntry{{col: si, val: big.NewRat(1, 1)}}
		}
		if len(eqEntries[q]) == 0 {
			return nil, false
		}
	}
	return solveSparseRationalSystem(eqEntries, eqRHS[:m], m)
}

func solveExactBasisTransposeRay(s *LPSolver, d []float64) ([]*big.Rat, bool) {
	p := s.prob
	n := p.numVars
	m := p.numConstraints
	if s.TotalRows() != m || len(d) < m {
		return nil, false
	}
	eqRHS := make([]*big.Rat, m)
	for q := range m {
		bVar := s.ws.basis[q]
		r := new(big.Rat)
		if d[q] != 0 {
			rhsVal := d[q]
			if s.scaled {
				if bVar < n {
					rhsVal *= s.invColScale[bVar]
				} else {
					rhsVal *= s.rowScaleOf(bVar - n)
				}
			}
			if math.IsNaN(rhsVal) || math.IsInf(rhsVal, 0) {
				return nil, false
			}
			r.SetFloat64(rhsVal)
		}
		eqRHS[q] = r
	}
	w, ok := solveExactBasisTranspose(p, s.ws.basis[:m], eqRHS)
	if !ok || !isExactRatFarkasValid(p, w) {
		return nil, false
	}
	return w, true
}

func extractExactBasisDualsRat(tb testing.TB, s *LPSolver) []*big.Rat {
	tb.Helper()
	p := s.prob
	n := p.numVars
	m := p.numConstraints
	if s.TotalRows() != m {
		tb.Fatalf("unexpected extra rows %d != %d", s.TotalRows(), m)
	}
	objMinVec, okObj := exactMinObjVecRat(p)
	if !okObj {
		tb.Fatalf("exact objective vector extraction failed on %s", p.Name())
	}
	eqRHS := make([]*big.Rat, m)
	for q := range m {
		bVar := s.ws.basis[q]
		r := new(big.Rat)
		if bVar < n {
			r.Set(objMinVec[bVar])
		}
		eqRHS[q] = r
	}
	y, ok := solveExactBasisTranspose(p, s.ws.basis[:m], eqRHS)
	if !ok {
		tb.Fatalf("exact rational basis dual solve failed on %s", p.Name())
	}
	return y
}

func extractFarkasRayRat(tb testing.TB, s *LPSolver) []*big.Rat {
	tb.Helper()
	m := s.TotalRows()
	tryCandidate := func(d []float64) ([]*big.Rat, bool) {
		if w, ok := tryFloatBTRANFarkasRay(s, d); ok {
			return w, true
		}
		return solveExactBasisTransposeRay(s, d)
	}

	d := make([]float64, m)
	if s.lastDualPhase1Used && !s.lastPrimalFallback && uint(s.lastInfeasRow) < uint(m) {
		sign := -1.0
		if s.lastInfeasDir < 0 {
			sign = 1.0
		}
		d[s.lastInfeasRow] = sign
		if w, ok := tryCandidate(d); ok {
			return w
		}
	}
	if s.lastPrimalFallback {
		for i := range m {
			d[i] = s.ws.workCost[s.ws.basis[i]]
		}
		if w, ok := tryCandidate(d); ok {
			return w
		}
	}
	for q := range m {
		clear(d)
		d[q] = 1.0
		if w, ok := tryCandidate(d); ok {
			return w
		}
		d[q] = -1.0
		if w, ok := tryCandidate(d); ok {
			return w
		}
	}
	for i := range m {
		d[i] = s.ws.workCost[s.ws.basis[i]]
	}
	if w, ok := tryCandidate(d); ok {
		return w
	}
	tb.Fatalf("failed to extract exact Farkas ray from infeasible LPSolver (m=%d dualP1=%v primalFB=%v lastRow=%d lastDir=%g basis=%v x=%v lb=%v ub=%v scaled=%v)",
		m, s.lastDualPhase1Used, s.lastPrimalFallback, s.lastInfeasRow, s.lastInfeasDir, s.ws.basis[:m], s.ws.x[:s.prob.numVars+m], s.ws.lb[:s.prob.numVars+m], s.ws.ub[:s.prob.numVars+m], s.scaled)
	return nil
}

func TestFormalTrack3(t *testing.T) {
	t.Parallel()
	requireFormalEnv(t)
	exe := formalExe(t, "lp-check")

	t.Run("ExactDyadicOptimal", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name  string
			model *Model
		}{
			{"t1_std_lp", buildStdLPModel("t1_std_lp", Minimize)},
			{"t1_std_lp_max", buildStdLPModel("t1_std_lp_max", Maximize)},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				p := tc.model.Compile()
				res, err := SolveCompiled(t.Context(), p, WithPureGo(), WithPresolve(false))
				if err != nil || res.Status != StatusOptimal {
					t.Fatalf("%s solve failed: status=%v err=%v", tc.name, res.Status, err)
				}
				payload := compiledProblemToLPPayload(t, p, "optimal")
				xVec := make([]ratPayload, p.NumVars())
				for j := range p.NumVars() {
					xr, xErr := float64ToPayload(res.Values[j])
					if xErr != nil {
						t.Fatalf("%s encode x[%d]: %v", tc.name, j, xErr)
					}
					xVec[j] = xr
				}
				yRats, yBits := minDualsToPayloads(t, p, res.Duals)

				payload.X = xVec
				payload.Y = yRats
				out, runErr := runLPCheck(t, exe, tc.name+"_rat", payload, true)
				if runErr != nil {
					t.Fatalf("%s rat optimal failed: err=%v out=%q", tc.name, runErr, out)
				}
				assertLPCheckVerifiedMode(t, tc.name+"_rat", out, "optimal")

				payloadBits := payload
				payloadBits.Y = nil
				payloadBits.YBits = yBits
				outBits, bitsErr := runLPCheck(t, exe, tc.name+"_bits", payloadBits, false)
				if bitsErr != nil {
					t.Fatalf("%s bits optimal failed: err=%v out=%q", tc.name, bitsErr, outBits)
				}
				assertLPCheckVerifiedMode(t, tc.name+"_bits", outBits, "optimal")
			})
		}
	})

	t.Run("SafeDualBoundBoxedFloatDuals", func(t *testing.T) {
		t.Parallel()
		buildRandomBoxed := func(seed uint64, rows, cols int, sense Sense) *Model {
			m := NewModel(fmt.Sprintf("boxed_%d", seed), sense)
			xs := make([]Var, cols)
			obj := NewExpr().AddConstant(2.5)
			for j := range cols {
				xs[j] = m.AddContVar(fmt.Sprintf("x%d", j), 0, 10)
				c := 1 + float64((int(seed)+j*7)%13)*0.25
				obj = obj.AddTerm(xs[j], c)
			}
			m.SetObjective(obj)
			for i := range rows {
				e := NewExpr()
				for j := range cols {
					if (i+j+int(seed))%3 == 0 {
						e = e.AddTerm(xs[j], 0.5+float64((i*3+j)%5)*0.4)
					}
				}
				if len(e.Terms) > 0 {
					if sense == Maximize {
						m.AddConstraint(fmt.Sprintf("r%d", i), Le(e, 15.0+float64(i%4)))
					} else {
						m.AddConstraint(fmt.Sprintf("r%d", i), Ge(e, 3.0+float64(i%4)))
					}
				}
			}
			return m
		}

		for _, tc := range []struct {
			name  string
			model *Model
		}{
			{"t1_whiskas", buildWhiskasLPModel()},
			{"boxed_lp_1", buildRandomBoxed(1, 8, 14, Minimize)},
			{"boxed_lp_2", buildRandomBoxed(2, 12, 18, Minimize)},
			{"boxed_lp_max", buildRandomBoxed(3, 8, 12, Maximize)},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				p := tc.model.Compile()
				res, err := SolveCompiled(t.Context(), p, WithPureGo())
				if err != nil || res.Status != StatusOptimal {
					t.Fatalf("%s solve failed: status=%v err=%v", tc.name, res.Status, err)
				}
				yRats, yBits := minDualsToPayloads(t, p, res.Duals)
				assertSafeDualBoundTight(t, exe, tc.name, p, res.Values, yRats, yBits)
			})
		}
	})

	t.Run("SafeDualBoundNetlib", func(t *testing.T) {
		t.Parallel()
		for _, id := range []string{"netlib_afiro", "netlib_sc50a", "netlib_sc50b", "netlib_blend"} {
			t.Run(id, func(t *testing.T) {
				t.Parallel()
				m := loadPublicPuLPJSONForTest(t, id)
				p := m.Compile()
				s := NewLPSolver(p, nil)
				defer s.Close()
				res := s.Solve()
				if res.Status != StatusOptimal {
					t.Fatalf("%s solve failed: status=%v", id, res.Status)
				}
				yExact := extractExactBasisDualsRat(t, s)
				yRats := make([]ratPayload, len(yExact))
				for i, yr := range yExact {
					yRats[i] = bigRatToPayload(yr)
				}
				assertSafeDualBoundTight(t, exe, id, p, res.Primal, yRats, nil)
			})
		}
	})

	t.Run("FarkasInfeasible", func(t *testing.T) {
		t.Parallel()
		buildInfBoth := func() *Model {
			m := NewModel("inf_both", Minimize)
			x := m.AddContVar("x", 0, math.Inf(1))
			y := m.AddContVar("y", 0, math.Inf(1))
			m.SetObjective(NewExpr(x.T(-3), y.T(1)))
			m.AddConstraint("c0", NewExpr(x.T(1), y.T(1)).Le(2))
			m.AddConstraint("c1", NewExpr(x.T(1), y.T(1)).Ge(5))
			return m
		}
		buildFreeInf := func() *Model {
			m := NewModel("free_inf", Minimize)
			f := m.AddContVar("f", math.Inf(-1), math.Inf(1))
			x := m.AddContVar("x", 0, 1)
			m.SetObjective(NewExpr(f.T(1)))
			m.AddConstraint("c0", NewExpr(f.T(1), x.T(1)).Le(1))
			m.AddConstraint("c1", NewExpr(f.T(1), x.T(1)).Ge(5))
			return m
		}
		buildNonDyadicRec := func() *Model {
			m := NewModel("farkas_nondyadic_recession", Minimize)
			x0 := m.AddContVar("x0", 0, 1)
			x3 := m.AddContVar("x3", 0, math.Inf(1))
			x4 := m.AddContVar("x4", 0, math.Inf(1))
			m.AddConstraint("e0", NewExpr(x0.T(1), x3.T(3.5), x4.T(-3.5)).Eq(0))
			m.AddConstraint("g1", NewExpr(x0.T(-1), x3.T(-7.75), x4.T(7.75)).Ge(10))
			return m
		}
		buildLargeM := func() *Model {
			m := NewModel("farkas_m66_3_7", Minimize)
			fx := m.AddContVar("x", math.Inf(-1), math.Inf(1))
			fy := m.AddContVar("y", math.Inf(-1), math.Inf(1))
			m.AddConstraint("c0", NewExpr(fx.T(3), fy.T(-3)).Le(0))
			m.AddConstraint("c1", NewExpr(fx.T(7), fy.T(-7)).Ge(1))
			for k := range 64 {
				fz := m.AddContVar(fmt.Sprintf("z_%d", k), 0, 10)
				m.AddConstraint(fmt.Sprintf("dz_%d", k), NewExpr(fz.T(1)).Le(5))
			}
			return m
		}

		for _, tc := range []struct {
			name  string
			build func() *Model
		}{
			{"inf_both", buildInfBoth},
			{"free_inf", buildFreeInf},
			{"farkas_nondyadic_recession", buildNonDyadicRec},
			{"farkas_m66_3_7", buildLargeM},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				p := tc.build().Compile()
				s := NewLPSolver(p, nil)
				defer s.Close()
				res := s.Solve()
				if res.Status != StatusInfeasible {
					t.Fatalf("%s status=%v, want Infeasible", tc.name, res.Status)
				}
				wRat := extractFarkasRayRat(t, s)
				payload := compiledProblemToLPPayload(t, p, "farkas")
				wPayload := make([]ratPayload, len(wRat))
				for i, wr := range wRat {
					wPayload[i] = bigRatToPayload(wr)
				}
				payload.W = wPayload
				out, err := runLPCheck(t, exe, tc.name+"_farkas", payload, false)
				if err != nil {
					t.Fatalf("%s lp-check farkas failed: err=%v out=%q", tc.name, err, out)
				}
				assertLPCheckVerifiedMode(t, tc.name+"_farkas", out, "farkas")
			})
		}
	})

	t.Run("BoundConflictInfeasible", func(t *testing.T) {
		t.Parallel()
		mBC := NewModel("bound_conflict_m0", Minimize)
		x0 := mBC.AddContVar("x0", 0, 5)
		x1 := mBC.AddContVar("x1", 7, 3)
		mBC.SetObjective(NewExpr(x0.T(1), x1.T(2)))
		pBC := mBC.Compile()

		colOne := 1
		bcPayload := compiledProblemToLPPayload(t, pBC, "bound-conflict")
		bcPayload.Col = &colOne
		out, err := runLPCheck(t, exe, "bc_pos", bcPayload, false)
		if err != nil {
			t.Fatalf("bound-conflict positive control failed: err=%v out=%q", err, out)
		}
		assertLPCheckVerifiedMode(t, "bc_pos", out, "bound-conflict")

		colZero := 0
		badBC := bcPayload
		badBC.Col = &colZero
		if badOut, badErr := runLPCheck(t, exe, "bc_neg_col0", badBC, false); badErr == nil {
			t.Fatalf("expected bound-conflict rejection on non-conflicting col 0, got: %q", badOut)
		}
	})

	t.Run("NegativeControls", func(t *testing.T) {
		t.Parallel()
		// 1. Optimal perturbed primal violating row constraint.
		pOpt := buildStdLPModel("neg_opt", Minimize).Compile()
		resOpt, err := SolveCompiled(t.Context(), pOpt, WithPureGo(), WithPresolve(false))
		if err != nil || resOpt.Status != StatusOptimal {
			t.Fatalf("solve neg_opt: status=%v err=%v", resOpt.Status, err)
		}
		badPrimal := compiledProblemToLPPayload(t, pOpt, "optimal")
		badPrimal.X = make([]ratPayload, pOpt.NumVars())
		for j := range pOpt.NumVars() {
			v := resOpt.Values[j]
			if j == 0 {
				v += 1.0
			}
			rp, rErr := float64ToPayload(v)
			if rErr != nil {
				t.Fatalf("encode x: %v", rErr)
			}
			badPrimal.X[j] = rp
		}
		badPrimal.Y, _ = minDualsToPayloads(t, pOpt, resOpt.Duals)
		if out, runErr := runLPCheck(t, exe, "neg_bad_primal", badPrimal, false); runErr == nil {
			t.Fatalf("expected rejection for infeasible primal, got: %q", out)
		}

		// 2. Corrupted dual sign / non-zero duality gap in optimal mode.
		badDual := compiledProblemToLPPayload(t, pOpt, "optimal")
		badDual.X = make([]ratPayload, pOpt.NumVars())
		for j := range pOpt.NumVars() {
			rp, _ := float64ToPayload(resOpt.Values[j])
			badDual.X[j] = rp
		}
		badDual.Y = []ratPayload{int64ToPayload(0), int64ToPayload(-2), int64ToPayload(8)}
		if out, runErr := runLPCheck(t, exe, "neg_bad_dual", badDual, false); runErr == nil {
			t.Fatalf("expected rejection for wrong dual sign / gap, got: %q", out)
		}

		// 3. Corrupted safe-bound with Inf/NaN yBits or unbounded reduced cost.
		badBits := compiledProblemToLPPayload(t, pOpt, "safe-bound")
		badBits.YBits = []uint64{math.Float64bits(math.NaN()), 0, 0}
		if out, runErr := runLPCheck(t, exe, "neg_nan_bits", badBits, false); runErr == nil {
			t.Fatalf("expected rejection for NaN yBits, got: %q", out)
		}

		// Unbounded variable with negative reduced cost under safe-bound.
		mUnbGe := NewModel("neg_unb_rc_ge", Minimize)
		ugx := mUnbGe.AddContVar("x", 0, math.Inf(1))
		mUnbGe.SetObjective(NewExpr(ugx.T(1)))
		mUnbGe.AddConstraint("r0", Ge(NewExpr(ugx.T(1)), 5))
		badSafe := compiledProblemToLPPayload(t, mUnbGe.Compile(), "safe-bound")
		badSafe.Y = []ratPayload{int64ToPayload(2)}
		if out, runErr := runLPCheck(t, exe, "neg_unb_rc", badSafe, false); runErr == nil {
			t.Fatalf("expected rejection for negative reduced cost on +Inf upper bound, got: %q", out)
		}

		// 4. Corrupted Farkas rays (zero ray, unbounded ray, and non-strict gap on feasible instance).
		mInf := NewModel("neg_farkas", Minimize)
		fx := mInf.AddContVar("x", 0, math.Inf(1))
		mInf.AddConstraint("c0", Le(NewExpr(fx.T(1)), 2))
		mInf.AddConstraint("c1", Ge(NewExpr(fx.T(1)), 5))
		badFarkas := compiledProblemToLPPayload(t, mInf.Compile(), "farkas")
		badFarkas.W = []ratPayload{int64ToPayload(0), int64ToPayload(0)}
		if out, runErr := runLPCheck(t, exe, "neg_zero_farkas", badFarkas, false); runErr == nil {
			t.Fatalf("expected rejection for zero Farkas ray, got: %q", out)
		}
		badFarkas.W = []ratPayload{int64ToPayload(1), int64ToPayload(1)}
		if out, runErr := runLPCheck(t, exe, "neg_bad_cone_farkas", badFarkas, false); runErr == nil {
			t.Fatalf("expected rejection for unbounded Farkas cone ray, got: %q", out)
		}

		mFeas := NewModel("neg_farkas_gap", Minimize)
		gx := mFeas.AddContVar("x", 0, math.Inf(1))
		mFeas.AddConstraint("c0", Le(NewExpr(gx.T(1)), 5))
		mFeas.AddConstraint("c1", Ge(NewExpr(gx.T(1)), 2))
		badGapFarkas := compiledProblemToLPPayload(t, mFeas.Compile(), "farkas")
		badGapFarkas.W = []ratPayload{int64ToPayload(-1), int64ToPayload(1)}
		if out, runErr := runLPCheck(t, exe, "neg_gap_farkas", badGapFarkas, false); runErr == nil {
			t.Fatalf("expected rejection for non-positive Farkas gap, got: %q", out)
		}
	})

	t.Run("LossyExactOrig", func(t *testing.T) {
		t.Parallel()

		verifyPayload := func(t *testing.T, name string, p *CompiledProblem, payload []byte, wantMode string) {
			t.Helper()
			if err := verifySerializedLPPayloadMatchesProblem(p, payload); err != nil {
				t.Fatalf("%s verifySerializedLPPayloadMatchesProblem failed: %v", name, err)
			}
			resp, err := FormalCheckLPJSONL(t, payload)
			if err != nil {
				t.Fatalf("%s FormalCheckLPJSONL transport error: %v", name, err)
			}
			if !resp.OK {
				t.Fatalf("%s lp-check rejected payload: %s", name, resp.Error)
			}
			if wantMode != "" && resp.Mode != wantMode {
				t.Fatalf("%s lp-check mode=%q, want %q", name, resp.Mode, wantMode)
			}
		}

		// Optimal lossy LP: combine non-zero Expr.Constant (4.3 - 0.1) with duplicate 0.1*x + 0.2*x terms in row and objective.
		mOpt := NewModel("lossy_exact_orig_opt", Minimize)
		x := mOpt.AddContVar("x", 0, 10)
		y := mOpt.AddContVar("y", 0, 10)
		z := mOpt.AddContVar("z", 0, 10)
		mOpt.SetObjective(NewExpr(x.T(0.1), x.T(0.2), y.T(1), z.T(1)).AddConstant(1))
		mOpt.AddConstraint("ge_43", Ge(NewExpr(x.T(1), y.T(1), z.T(0.5)).AddConstant(0.1), 4.3))
		mOpt.AddConstraint("le_03", Le(NewExpr(x.T(0.1), x.T(0.2)), 0.3))
		pOpt := mOpt.Compile()
		if _, _, _, _, _, hasInexact, okR := ExactProblemRatsForTest(pOpt); !okR || !hasInexact {
			t.Fatalf("expected hasInexactOrig=true on lossy_exact_orig_opt, got ok=%v inexact=%v", okR, hasInexact)
		}

		wsOpt := AcquireWorkspace(pOpt.NumVars(), pOpt.NumConstraints())
		defer ReleaseWorkspace(wsOpt)
		lpOpt := NewLPSolver(pOpt, wsOpt)
		resOpt := lpOpt.Solve()
		if resOpt.Status != StatusOptimal {
			t.Fatalf("lossy_exact_orig_opt status=%v, want Optimal", resOpt.Status)
		}
		modeOpt, payOpt, okOpt := ExportLPCheckPayload(pOpt, lpOpt, resOpt)
		if !okOpt || modeOpt != "optimal" {
			t.Fatalf("ExportLPCheckPayload(lossy_exact_orig_opt) ok=%v mode=%q, want optimal", okOpt, modeOpt)
		}
		verifyPayload(t, "lossy_exact_orig_opt", pOpt, payOpt, "optimal")

		// Sticky ULP case.
		mULP := NewModel("lossy_exact_orig_ulp", Minimize)
		xu := mULP.AddContVar("x", 0, 10)
		yu := mULP.AddContVar("y", 0, 10)
		mULP.SetObjective(NewExpr(xu.T(1), xu.T(0x1p-53), xu.T(0x1p-53), yu.T(2)).AddConstant(1))
		mULP.AddConstraint("ge_21", Ge(NewExpr(xu.T(1), yu.T(1)).AddConstant(0.1), 2.1))
		mULP.AddConstraint("le_ulp", Le(NewExpr(xu.T(1), xu.T(0x1p-53), xu.T(0x1p-53)), 1+0x1p-52))
		pULP := mULP.Compile()

		wsULP := AcquireWorkspace(pULP.NumVars(), pULP.NumConstraints())
		defer ReleaseWorkspace(wsULP)
		lpULP := NewLPSolver(pULP, wsULP)
		resULP := lpULP.Solve()
		if resULP.Status != StatusOptimal {
			t.Fatalf("lossy_exact_orig_ulp status=%v, want Optimal", resULP.Status)
		}
		modeULP, payULP, okULP := ExportLPCheckPayload(pULP, lpULP, resULP)
		if !okULP || modeULP != "optimal" {
			t.Fatalf("ExportLPCheckPayload(lossy_exact_orig_ulp) ok=%v mode=%q, want optimal", okULP, modeULP)
		}
		verifyPayload(t, "lossy_exact_orig_ulp", pULP, payULP, "optimal")

		// Infeasible lossy LPs with non-zero Expr.Constant and duplicate terms.
		mInf := NewModel("lossy_exact_orig_inf", Minimize)
		xi := mInf.AddContVar("x", 0, 10)
		yi := mInf.AddContVar("y", 0, 10)
		mInf.SetObjective(NewExpr(xi.T(0.1), xi.T(0.2), yi.T(1)).AddConstant(1))
		mInf.AddConstraint("ge_43", Ge(NewExpr(xi.T(1), yi.T(1)).AddConstant(0.1), 4.3))
		mInf.AddConstraint("le_03", Le(NewExpr(xi.T(0.1), xi.T(0.2)), 0.3))
		mInf.AddConstraint("le_cap", Le(NewExpr(xi.T(1), yi.T(1)).AddConstant(0.1), 4.0))
		pInf := mInf.Compile()

		wsInf := AcquireWorkspace(pInf.NumVars(), pInf.NumConstraints())
		defer ReleaseWorkspace(wsInf)
		lpInf := NewLPSolver(pInf, wsInf)
		resInf := lpInf.Solve()
		if resInf.Status != StatusInfeasible {
			t.Fatalf("lossy_exact_orig_inf status=%v, want Infeasible", resInf.Status)
		}
		modeInf, payInf, okInf := ExportLPCheckPayload(pInf, lpInf, resInf)
		if !okInf || modeInf != "farkas" {
			t.Fatalf("ExportLPCheckPayload(lossy_exact_orig_inf) ok=%v mode=%q, want farkas", okInf, modeInf)
		}
		verifyPayload(t, "lossy_exact_orig_inf", pInf, payInf, "farkas")

		mDupDirectInf := NewModel("lossy_exact_dup_direct_inf", Minimize)
		xdi := mDupDirectInf.AddContVar("x", 0, 10)
		mDupDirectInf.SetObjective(NewExpr(xdi.T(1)))
		mDupDirectInf.AddConstraint("le_03", Le(NewExpr(xdi.T(0.1), xdi.T(0.2)), 0.3))
		mDupDirectInf.AddConstraint("ge_20", Ge(NewExpr(xdi.T(1)), 2.0))
		pDupDirectInf := mDupDirectInf.Compile()
		wsDI := AcquireWorkspace(pDupDirectInf.NumVars(), pDupDirectInf.NumConstraints())
		defer ReleaseWorkspace(wsDI)
		lpDI := NewLPSolver(pDupDirectInf, wsDI)
		resDI := lpDI.Solve()
		if resDI.Status != StatusInfeasible {
			t.Fatalf("lossy_exact_dup_direct_inf status=%v, want Infeasible", resDI.Status)
		}
		modeDI, payDI, okDI := ExportLPCheckPayload(pDupDirectInf, lpDI, resDI)
		if !okDI || modeDI != "farkas" {
			t.Fatalf("ExportLPCheckPayload(lossy_exact_dup_direct_inf) ok=%v mode=%q, want farkas", okDI, modeDI)
		}
		verifyPayload(t, "lossy_exact_dup_direct_inf", pDupDirectInf, payDI, "farkas")

		// Verify BuildPhase1FarkasProblemForTest preserves exactOrig on an infeasible lossy LP.
		pFeas, okFeas := BuildPhase1FarkasProblemForTest(pDupDirectInf)
		if !okFeas || pFeas == nil {
			t.Fatal("BuildPhase1FarkasProblemForTest failed on lossy_exact_dup_direct_inf")
		}
		lpFeas := NewLPSolver(pFeas, nil)
		defer lpFeas.Close()
		resFeas := lpFeas.Solve()
		if resFeas.Status != StatusOptimal {
			t.Fatalf("Phase-I Farkas LP status=%v, want Optimal", resFeas.Status)
		}
		solFeas, okSolFeas := ExactBasisSolution(t.Context(), lpFeas)
		if !okSolFeas || solFeas.ObjectiveMin == nil || solFeas.ObjectiveMin.Sign() <= 0 || len(solFeas.DualMin) != pDupDirectInf.NumConstraints() {
			t.Fatalf("Phase-I Farkas ExactBasisSolution failed: ok=%v obj=%v", okSolFeas, solFeas.ObjectiveMin)
		}
		dualF := make([]float64, len(solFeas.DualMin))
		for i, d := range solFeas.DualMin {
			dualF[i], _ = d.Float64()
		}
		modeF, payF, okF := ExportLPCheckPayload(pDupDirectInf, nil, LPResult{Status: StatusInfeasible, Dual: dualF})
		if !okF || modeF != "farkas" {
			t.Fatalf("Phase-I Farkas ExportLPCheckPayload ok=%v mode=%q", okF, modeF)
		}
		verifyPayload(t, "lossy_exact_dup_phase1", pDupDirectInf, payF, "farkas")

		// Exact-Q infeasible LPs where float64 rounding agrees within 1 ULP, while exact rationals reject every point.
		checkExactQInfeasibleLP := func(name string, p *CompiledProblem, dualHint []float64) {
			t.Helper()
			certRes, errCert := SolveCompiled(t.Context(), p, WithPureGo(), WithCertified(true))
			if certRes.Status == StatusOptimal && errCert == nil {
				t.Fatalf("%s unexpectedly certified StatusOptimal on exact-Q infeasible LP", name)
			}
			modeF, payF, okF := ExportLPCheckPayload(p, nil, LPResult{Status: StatusInfeasible, Dual: dualHint})
			if !okF || modeF != "farkas" {
				t.Fatalf("%s ExportLPCheckPayload(farkas) ok=%v mode=%q", name, okF, modeF)
			}
			verifyPayload(t, name, p, payF, "farkas")
		}

		mExactRHSInf := NewModel("lossy_exact_rhs_inf", Minimize)
		xr := mExactRHSInf.AddContVar("x", 0, 5)
		mExactRHSInf.SetObjective(NewExpr(xr.T(1)))
		mExactRHSInf.AddConstraint("ge_21", Ge(NewExpr(xr.T(1)).AddConstant(0.1), 2.1))
		mExactRHSInf.AddConstraint("le_20", Le(NewExpr(xr.T(1)), 2.0))
		checkExactQInfeasibleLP("lossy_exact_rhs_inf", mExactRHSInf.Compile(), []float64{1, -1})

		mExactDupInf := NewModel("lossy_exact_dup_inf", Minimize)
		xd := mExactDupInf.AddContVar("x", 0, 5)
		mExactDupInf.SetObjective(NewExpr(xd.T(1)))
		mExactDupInf.AddConstraint("le_03", Le(NewExpr(xd.T(0.1), xd.T(0.2)), 0.3))
		mExactDupInf.AddConstraint("ge_10", Ge(NewExpr(xd.T(1)), 1.0))
		checkExactQInfeasibleLP("lossy_exact_dup_inf", mExactDupInf.Compile(), []float64{-3, 0.9})

		// Negative controls.
		var decoded lpCertPayload
		if err := json.Unmarshal(payOpt, &decoded); err != nil {
			t.Fatalf("unmarshal payOpt: %v", err)
		}

		badRHS := decoded
		badRHS.Rows = slices.Clone(decoded.Rows)
		compRHS0, errCR := float64ToPayload(pOpt.rowRHS[0])
		if errCR != nil {
			t.Fatalf("encode compiled rowRHS[0]: %v", errCR)
		}
		badRHS.Rows[0].RHS = compRHS0
		rawBadRHS, err := json.Marshal(badRHS)
		if err != nil {
			t.Fatalf("marshal badRHS: %v", err)
		}
		if err := verifySerializedLPPayloadMatchesProblem(pOpt, rawBadRHS); err == nil {
			t.Fatal("verifySerializedLPPayloadMatchesProblem accepted compiled float64 rowRHS substitution")
		}

		badRowCoeff := decoded
		badRowCoeff.Rows = slices.Clone(decoded.Rows)
		badRowCoeff.Rows[1].Terms = slices.Clone(decoded.Rows[1].Terms)
		compCoeff, errCC := float64ToPayload(pOpt.rowVal[pOpt.rowPtr[1]])
		if errCC != nil {
			t.Fatalf("encode compiled rowVal: %v", errCC)
		}
		badRowCoeff.Rows[1].Terms[0].Coeff = compCoeff
		rawBadRowCoeff, err := json.Marshal(badRowCoeff)
		if err != nil {
			t.Fatalf("marshal badRowCoeff: %v", err)
		}
		if err := verifySerializedLPPayloadMatchesProblem(pOpt, rawBadRowCoeff); err == nil {
			t.Fatal("verifySerializedLPPayloadMatchesProblem accepted compiled float64 duplicate-term row substitution")
		}

		badObj := decoded
		badObj.Obj = slices.Clone(decoded.Obj)
		compObj0, errCO := float64ToPayload(pOpt.objMin[0])
		if errCO != nil {
			t.Fatalf("encode compiled objMin[0]: %v", errCO)
		}
		badObj.Obj[0] = compObj0
		rawBadObj, err := json.Marshal(badObj)
		if err != nil {
			t.Fatalf("marshal badObj: %v", err)
		}
		if err := verifySerializedLPPayloadMatchesProblem(pOpt, rawBadObj); err == nil {
			t.Fatal("verifySerializedLPPayloadMatchesProblem accepted compiled float64 duplicate-term objective substitution")
		}
	})
}
