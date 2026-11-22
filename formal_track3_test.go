package goblin

import (
	"encoding/json/v2"
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
	X         []ratPayload   `json:"x,omitzero"`
	Y         []ratPayload   `json:"y,omitzero"`
	YBits     []uint64       `json:"yBits,omitzero"`
	W         []ratPayload   `json:"w,omitzero"`
	PrimalObj *ratPayload    `json:"primalObj,omitzero"`
	GapTol    *ratPayload    `json:"gapTol,omitzero"`
}

func compiledProblemToLPPayload(tb testing.TB, p *CompiledProblem, mode string) lpCertPayload {
	tb.Helper()
	n := p.NumVars()
	m := p.NumConstraints()

	objConstVal := p.ObjConstant()
	if p.Sense() == Maximize && objConstVal != 0 {
		objConstVal = -objConstVal
	}
	objConst, err := float64ToPayload(objConstVal)
	if err != nil {
		tb.Fatalf("encode objConst: %v", err)
	}

	obj := make([]ratPayload, n)
	lb := make([]*ratPayload, n)
	ub := make([]*ratPayload, n)
	for j := range n {
		cRat, cErr := float64ToPayload(p.ObjMinCoeff(j))
		if cErr != nil {
			tb.Fatalf("encode obj[%d]: %v", j, cErr)
		}
		obj[j] = cRat

		lo, hi := p.VarBounds(j)
		if !math.IsInf(lo, -1) {
			loRat, lErr := float64ToPayload(lo)
			if lErr != nil {
				tb.Fatalf("encode lb[%d]: %v", j, lErr)
			}
			lb[j] = &loRat
		}
		if !math.IsInf(hi, 1) {
			hiRat, uErr := float64ToPayload(hi)
			if uErr != nil {
				tb.Fatalf("encode ub[%d]: %v", j, uErr)
			}
			ub[j] = &hiRat
		}
	}

	rows := make([]lpRowPayload, m)
	for i := range m {
		var sense string
		switch p.RowSense(i) {
		case LessEqual:
			sense = "le"
		case GreaterEqual:
			sense = "ge"
		case Equal:
			sense = "eq"
		}
		rhs, rErr := float64ToPayload(p.RowRHS(i))
		if rErr != nil {
			tb.Fatalf("encode rhs[%d]: %v", i, rErr)
		}
		terms := make([]lpTermPayload, 0, p.RowLen(i))
		p.ForEachRowTerm(i, func(col int, val float64) {
			coeff, tErr := float64ToPayload(val)
			if tErr != nil {
				tb.Fatalf("encode row %d col %d: %v", i, col, tErr)
			}
			terms = append(terms, lpTermPayload{Col: col, Coeff: coeff})
		})
		rows[i] = lpRowPayload{Sense: sense, RHS: rhs, Terms: terms}
	}

	return lpCertPayload{
		Mode:     mode,
		NumVars:  n,
		NumRows:  m,
		ObjConst: objConst,
		Obj:      obj,
		LB:       lb,
		UB:       ub,
		Rows:     rows,
	}
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
	sum := new(big.Rat)
	if p.objConstMinVal != 0 {
		if sum.SetFloat64(p.objConstMinVal) == nil {
			return nil, fmt.Errorf("non-finite objConstMinVal %v", p.objConstMinVal)
		}
	}
	term := new(big.Rat)
	xj := new(big.Rat)
	for j := range p.NumVars() {
		c := p.ObjMinCoeff(j)
		if c == 0 || x[j] == 0 {
			continue
		}
		if term.SetFloat64(c) == nil || xj.SetFloat64(x[j]) == nil {
			return nil, fmt.Errorf("non-finite objective term at col %d", j)
		}
		term.Mul(term, xj)
		sum.Add(sum, term)
	}
	return sum, nil
}

func assertSafeDualBoundTight(tb testing.TB, exe, name string, p *CompiledProblem, primal []float64, yRats []ratPayload, yBits []uint64) {
	tb.Helper()
	payload := compiledProblemToLPPayload(tb, p, "safe-bound")
	payload.Y = yRats
	out, runErr := runLPCheck(tb, exe, name+"_safe", payload, true)
	if runErr != nil || !strings.Contains(out, "VERIFIED") {
		tb.Fatalf("%s safe-bound failed: err=%v out=%q", name, runErr, out)
	}
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
		if bitsErr != nil || strings.TrimSpace(outBits) != "VERIFIED" {
			tb.Fatalf("%s yBits safe-bound failed: err=%v out=%q", name, bitsErr, outBits)
		}
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
		if wi.Sign() != 0 && p.rowRHS[i] != 0 {
			tmp.SetFloat64(p.rowRHS[i])
			tmp.Mul(tmp, wi)
			wDotB.Add(wDotB, tmp)
		}
	}

	maxLHS := new(big.Rat)
	wa := new(big.Rat)
	for j := range n {
		wa.SetInt64(0)
		for idx := p.colPtr[j]; idx < p.colPtr[j+1]; idx++ {
			r := p.rowIdx[idx]
			if uint(r) < uint(m) && projW[r].Sign() != 0 && p.colVal[idx] != 0 {
				tmp.SetFloat64(p.colVal[idx])
				tmp.Mul(tmp, projW[r])
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

	type ratEntry struct {
		col int
		val *big.Rat
	}
	eqEntries := make([][]ratEntry, m)
	colEqs := make([][]int, m)
	eqActive := make([]bool, m)
	eqDeg := make([]int, m)
	varDeg := make([]int, m)

	for q := range m {
		bVar := basis[q]
		if bVar < n {
			var ents []ratEntry
			for idx := p.colPtr[bVar]; idx < p.colPtr[bVar+1]; idx++ {
				ri := p.rowIdx[idx]
				if uint(ri) < uint(m) && p.colVal[idx] != 0 {
					ents = append(ents, ratEntry{col: ri, val: new(big.Rat).SetFloat64(p.colVal[idx])})
				}
			}
			eqEntries[q] = ents
		} else {
			si := bVar - n
			if uint(si) >= uint(m) {
				return nil, false
			}
			eqEntries[q] = []ratEntry{{col: si, val: big.NewRat(1, 1)}}
		}
		if len(eqEntries[q]) == 0 {
			return nil, false
		}
		eqActive[q] = true
		eqDeg[q] = len(eqEntries[q])
		for _, ent := range eqEntries[q] {
			colEqs[ent.col] = append(colEqs[ent.col], q)
			varDeg[ent.col]++
		}
	}

	w := make([]*big.Rat, m)
	for i := range m {
		w[i] = new(big.Rat)
	}
	varKnown := make([]bool, m)
	varDeferred := make([]bool, m)
	type backStep struct {
		eq  int
		col int
	}
	var backStack []backStep
	var rowQ, colQ []int
	for q := range m {
		if eqDeg[q] == 1 {
			rowQ = append(rowQ, q)
		}
		if varDeg[q] == 0 {
			return nil, false
		}
		if varDeg[q] == 1 {
			colQ = append(colQ, q)
		}
	}

	tmp := new(big.Rat)
	acc := new(big.Rat)
	for len(rowQ) > 0 || len(colQ) > 0 {
		for len(rowQ) > 0 {
			q := rowQ[len(rowQ)-1]
			rowQ = rowQ[:len(rowQ)-1]
			if !eqActive[q] || eqDeg[q] != 1 {
				continue
			}
			targetCol := -1
			var targetVal *big.Rat
			acc.Set(eqRHS[q])
			for _, ent := range eqEntries[q] {
				if varKnown[ent.col] {
					if w[ent.col].Sign() != 0 {
						tmp.Mul(ent.val, w[ent.col])
						acc.Sub(acc, tmp)
					}
				} else if !varDeferred[ent.col] {
					targetCol = ent.col
					targetVal = ent.val
				}
			}
			if targetCol < 0 || targetVal.Sign() == 0 {
				return nil, false
			}
			w[targetCol].Quo(acc, targetVal)
			varKnown[targetCol] = true
			eqActive[q] = false
			for _, q2 := range colEqs[targetCol] {
				if !eqActive[q2] {
					continue
				}
				eqDeg[q2]--
				if eqDeg[q2] == 1 {
					rowQ = append(rowQ, q2)
				}
			}
		}
		for len(colQ) > 0 && len(rowQ) == 0 {
			c := colQ[len(colQ)-1]
			colQ = colQ[:len(colQ)-1]
			if varKnown[c] || varDeferred[c] || varDeg[c] != 1 {
				continue
			}
			actEq := -1
			for _, q := range colEqs[c] {
				if eqActive[q] {
					actEq = q
					break
				}
			}
			if actEq < 0 {
				return nil, false
			}
			backStack = append(backStack, backStep{eq: actEq, col: c})
			varDeferred[c] = true
			eqActive[actEq] = false
			for _, ent := range eqEntries[actEq] {
				if !varKnown[ent.col] && !varDeferred[ent.col] {
					varDeg[ent.col]--
					if varDeg[ent.col] == 1 {
						colQ = append(colQ, ent.col)
					}
				}
			}
		}
	}

	var nucEqs, nucVars []int
	nucVarPos := make([]int, m)
	for i := range m {
		nucVarPos[i] = -1
		if eqActive[i] {
			nucEqs = append(nucEqs, i)
		}
		if !varKnown[i] && !varDeferred[i] {
			nucVarPos[i] = len(nucVars)
			nucVars = append(nucVars, i)
		}
	}
	nNuc := len(nucVars)
	if len(nucEqs) != nNuc {
		return nil, false
	}
	if nNuc > 0 {
		mat := make([][]*big.Rat, nNuc)
		for rowIdx, q := range nucEqs {
			row := make([]*big.Rat, nNuc+1)
			for k := range nNuc {
				row[k] = new(big.Rat)
			}
			rhsR := new(big.Rat).Set(eqRHS[q])
			for _, ent := range eqEntries[q] {
				if varKnown[ent.col] {
					if w[ent.col].Sign() != 0 {
						tmp.Mul(ent.val, w[ent.col])
						rhsR.Sub(rhsR, tmp)
					}
				} else if pos := nucVarPos[ent.col]; pos >= 0 {
					row[pos].Set(ent.val)
				} else {
					return nil, false
				}
			}
			row[nNuc] = rhsR
			mat[rowIdx] = row
		}
		factor := new(big.Rat)
		for col := range nNuc {
			piv := -1
			for r := col; r < nNuc; r++ {
				if mat[r][col].Sign() != 0 {
					piv = r
					break
				}
			}
			if piv == -1 {
				return nil, false
			}
			if piv != col {
				mat[col], mat[piv] = mat[piv], mat[col]
			}
			invPiv := new(big.Rat).Inv(mat[col][col])
			for c := col; c <= nNuc; c++ {
				if mat[col][c].Sign() != 0 {
					mat[col][c].Mul(mat[col][c], invPiv)
				}
			}
			for r := range nNuc {
				if r == col || mat[r][col].Sign() == 0 {
					continue
				}
				factor.Set(mat[r][col])
				mat[r][col].SetInt64(0)
				for c := col + 1; c <= nNuc; c++ {
					if mat[col][c].Sign() != 0 {
						tmp.Mul(factor, mat[col][c])
						mat[r][c].Sub(mat[r][c], tmp)
					}
				}
			}
		}
		for k, origCol := range nucVars {
			w[origCol].Set(mat[k][nNuc])
			varKnown[origCol] = true
		}
	}

	for _, bs := range slices.Backward(backStack) {
		acc.Set(eqRHS[bs.eq])
		var pivVal *big.Rat
		for _, ent := range eqEntries[bs.eq] {
			if ent.col == bs.col {
				pivVal = ent.val
			} else if varKnown[ent.col] {
				if w[ent.col].Sign() != 0 {
					tmp.Mul(ent.val, w[ent.col])
					acc.Sub(acc, tmp)
				}
			} else {
				return nil, false
			}
		}
		if pivVal == nil || pivVal.Sign() == 0 {
			return nil, false
		}
		w[bs.col].Quo(acc, pivVal)
		varKnown[bs.col] = true
	}

	return w, true
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
	eqRHS := make([]*big.Rat, m)
	for q := range m {
		bVar := s.ws.basis[q]
		r := new(big.Rat)
		if bVar < n && p.objMin[bVar] != 0 {
			r.SetFloat64(p.objMin[bVar])
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
				if runErr != nil || !strings.Contains(out, "VERIFIED") {
					t.Fatalf("%s rat optimal failed: err=%v out=%q", tc.name, runErr, out)
				}

				payloadBits := payload
				payloadBits.Y = nil
				payloadBits.YBits = yBits
				outBits, bitsErr := runLPCheck(t, exe, tc.name+"_bits", payloadBits, false)
				if bitsErr != nil || strings.TrimSpace(outBits) != "VERIFIED" {
					t.Fatalf("%s bits optimal failed: err=%v out=%q", tc.name, bitsErr, outBits)
				}
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
				if err != nil || strings.TrimSpace(out) != "VERIFIED" {
					t.Fatalf("%s lp-check farkas failed: err=%v out=%q", tc.name, err, out)
				}
			})
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
}
