package goblin

import (
	"encoding/json/v2"
	"math"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type viprLinRef struct {
	ID   int        `json:"id"`
	Mult ratPayload `json:"mult"`
}

type viprBoundRef struct {
	Col  int        `json:"col"`
	Kind string     `json:"kind"`
	Mult ratPayload `json:"mult"`
}

type viprLinCon struct {
	Coeffs []ratPayload `json:"coeffs"`
	Sense  string       `json:"sense"`
	RHS    ratPayload   `json:"rhs"`
}

type viprStep struct {
	Rule        string         `json:"rule"`
	Row         *int           `json:"row,omitzero"`
	Col         *int           `json:"col,omitzero"`
	Sense       string         `json:"sense,omitzero"`
	Bound       *int64         `json:"bound,omitzero"`
	Refs        *[]viprLinRef  `json:"refs,omitzero"`
	Bounds      []viprBoundRef `json:"bounds,omitzero"`
	BoundAsms   []int          `json:"boundAsms,omitzero"`
	Ref         *int           `json:"ref,omitzero"`
	SplitVal    *int64         `json:"splitVal,omitzero"`
	LeftAsm     *int           `json:"leftAsm,omitzero"`
	RightAsm    *int           `json:"rightAsm,omitzero"`
	Left        *int           `json:"left,omitzero"`
	Right       *int           `json:"right,omitzero"`
	LBRef       *int           `json:"lbRef,omitzero"`
	UBRef       *int           `json:"ubRef,omitzero"`
	Con         *viprLinCon    `json:"con,omitzero"`
	Assumptions []int          `json:"assumptions,omitzero"`
}

type viprCert struct {
	N         int           `json:"n"`
	ObjConst  ratPayload    `json:"objConst"`
	Obj       []ratPayload  `json:"obj"`
	IsInt     []bool        `json:"isInt"`
	LB        []*ratPayload `json:"lb"`
	UB        []*ratPayload `json:"ub"`
	BaseRows  []viprLinCon  `json:"baseRows"`
	Mode      string        `json:"mode"`
	Incumbent []ratPayload  `json:"incumbent,omitzero"`
	Steps     []viprStep    `json:"steps"`
	RootStep  int           `json:"rootStep"`
}

func optIntPtr(v int) *int {
	p := v
	return &p
}

func optInt64Ptr(v int64) *int64 {
	p := v
	return &p
}

func optNonNegPtr(v int) *int {
	if v < 0 {
		return nil
	}
	return optIntPtr(v)
}

func optRefs(r []viprLinRef) *[]viprLinRef {
	if r == nil {
		empty := []viprLinRef{}
		return &empty
	}
	return &r
}

func runVIPRCheck(tb testing.TB, exe, name string, cert viprCert) (string, error) {
	tb.Helper()
	raw, err := json.Marshal(cert)
	if err != nil {
		tb.Fatalf("marshal vipr cert %s: %v", name, err)
	}
	path := filepath.Join(tb.TempDir(), name+".json")
	if writeErr := os.WriteFile(path, raw, 0o600); writeErr != nil {
		tb.Fatalf("write vipr cert %s: %v", name, writeErr)
	}
	cmd := exec.CommandContext(tb.Context(), exe, path)
	outBytes, runErr := cmd.CombinedOutput()
	return string(outBytes), runErr
}

func requireVIPRVerified(tb testing.TB, exe, name string, cert viprCert) {
	tb.Helper()
	out, err := runVIPRCheck(tb, exe, name, cert)
	if err != nil || strings.TrimSpace(out) != "VERIFIED" {
		tb.Fatalf("%s failed vipr-check: err=%v out=%q", name, err, out)
	}
}

func requireVIPRRejected(tb testing.TB, exe, name string, cert viprCert) {
	tb.Helper()
	out, err := runVIPRCheck(tb, exe, name, cert)
	if err == nil || strings.TrimSpace(out) == "VERIFIED" {
		tb.Fatalf("%s unexpectedly passed vipr-check: err=%v out=%q", name, err, out)
	}
}

// solveDenseRat reduces an m x m dense *big.Rat linear system mat * x = rhs.
func solveDenseRat(mat [][]*big.Rat, rhs []*big.Rat) ([]*big.Rat, bool) {
	m := len(rhs)
	if m == 0 {
		return nil, true
	}
	if len(mat) != m {
		return nil, false
	}
	aug := make([][]*big.Rat, m)
	for i := range m {
		if len(mat[i]) != m {
			return nil, false
		}
		row := make([]*big.Rat, m+1)
		for j := range m {
			row[j] = new(big.Rat).Set(mat[i][j])
		}
		row[m] = new(big.Rat).Set(rhs[i])
		aug[i] = row
	}
	prod := new(big.Rat)
	for col := range m {
		piv := -1
		for r := col; r < m; r++ {
			if aug[r][col].Sign() != 0 {
				piv = r
				break
			}
		}
		if piv < 0 {
			return nil, false
		}
		if piv != col {
			aug[col], aug[piv] = aug[piv], aug[col]
		}
		inv := new(big.Rat).Inv(aug[col][col])
		for c := col; c <= m; c++ {
			if aug[col][c].Sign() != 0 {
				aug[col][c].Mul(aug[col][c], inv)
			}
		}
		for r := range m {
			if r == col || aug[r][col].Sign() == 0 {
				continue
			}
			fac := new(big.Rat).Set(aug[r][col])
			aug[r][col].SetInt64(0)
			for c := col + 1; c <= m; c++ {
				if aug[col][c].Sign() != 0 {
					prod.Mul(fac, aug[col][c])
					aug[r][c].Sub(aug[r][c], prod)
				}
			}
		}
	}
	sol := make([]*big.Rat, m)
	for i := range m {
		sol[i] = aug[i][m]
	}
	return sol, true
}

type ratCompiledData struct {
	n        int
	m        int
	objConst *big.Rat
	objMin   []*big.Rat
	isInt    []bool
	lb       []*big.Rat
	ub       []*big.Rat
	rowSense []ConstraintSense
	rowRHS   []*big.Rat
	rowCols  [][]int
	rowVals  [][]*big.Rat
}

func mustRatFromFloat(tb testing.TB, v float64) *big.Rat {
	tb.Helper()
	r := new(big.Rat).SetFloat64(v)
	if r == nil {
		tb.Fatalf("non-finite float64 %v", v)
	}
	return r
}

func buildRatCompiledData(tb testing.TB, p *CompiledProblem) *ratCompiledData {
	tb.Helper()
	n := p.NumVars()
	m := p.NumConstraints()
	constMin := p.ObjConstant()
	if p.Sense() == Maximize && constMin != 0 {
		constMin = -constMin
	}
	d := &ratCompiledData{
		n:        n,
		m:        m,
		objConst: mustRatFromFloat(tb, constMin),
		objMin:   make([]*big.Rat, n),
		isInt:    make([]bool, n),
		lb:       make([]*big.Rat, n),
		ub:       make([]*big.Rat, n),
		rowSense: make([]ConstraintSense, m),
		rowRHS:   make([]*big.Rat, m),
		rowCols:  make([][]int, m),
		rowVals:  make([][]*big.Rat, m),
	}
	for j := range n {
		d.objMin[j] = mustRatFromFloat(tb, p.ObjMinCoeff(j))
		d.isInt[j] = p.varCat[j] != Continuous
		lo, hi := p.VarBounds(j)
		if !math.IsInf(lo, -1) {
			d.lb[j] = mustRatFromFloat(tb, lo)
		}
		if !math.IsInf(hi, 1) {
			d.ub[j] = mustRatFromFloat(tb, hi)
		}
	}
	for i := range m {
		d.rowSense[i] = p.RowSense(i)
		d.rowRHS[i] = mustRatFromFloat(tb, p.RowRHS(i))
		st, en := p.rowPtr[i], p.rowPtr[i+1]
		cols := make([]int, 0, en-st)
		vals := make([]*big.Rat, 0, en-st)
		for k := st; k < en; k++ {
			if p.rowVal[k] == 0 {
				continue
			}
			cols = append(cols, p.colIdx[k])
			vals = append(vals, mustRatFromFloat(tb, p.rowVal[k]))
		}
		d.rowCols[i] = cols
		d.rowVals[i] = vals
	}
	return d
}

func (d *ratCompiledData) buildVIPRBase(mode string) viprCert {
	senseStr := func(s ConstraintSense) string {
		switch s {
		case LessEqual:
			return "<="
		case GreaterEqual:
			return ">="
		case Equal:
			return "="
		default:
			return "<="
		}
	}
	objPay := make([]ratPayload, d.n)
	lbPay := make([]*ratPayload, d.n)
	ubPay := make([]*ratPayload, d.n)
	for j := range d.n {
		objPay[j] = bigRatToPayload(d.objMin[j])
		if d.lb[j] != nil {
			lp := bigRatToPayload(d.lb[j])
			lbPay[j] = &lp
		}
		if d.ub[j] != nil {
			up := bigRatToPayload(d.ub[j])
			ubPay[j] = &up
		}
	}
	rows := make([]viprLinCon, d.m)
	for i := range d.m {
		coeffs := make([]ratPayload, d.n)
		for j := range d.n {
			coeffs[j] = int64ToPayload(0)
		}
		for k, col := range d.rowCols[i] {
			coeffs[col] = bigRatToPayload(d.rowVals[i][k])
		}
		rows[i] = viprLinCon{
			Coeffs: coeffs,
			Sense:  senseStr(d.rowSense[i]),
			RHS:    bigRatToPayload(d.rowRHS[i]),
		}
	}
	return viprCert{
		N:        d.n,
		ObjConst: bigRatToPayload(d.objConst),
		Obj:      objPay,
		IsInt:    slices.Clone(d.isInt),
		LB:       lbPay,
		UB:       ubPay,
		BaseRows: rows,
		Mode:     mode,
	}
}

func (d *ratCompiledData) basisTransposeMatrix(basis []int) [][]*big.Rat {
	m := d.m
	mat := make([][]*big.Rat, m)
	for q := range m {
		row := make([]*big.Rat, m)
		for i := range m {
			row[i] = new(big.Rat)
		}
		mat[q] = row
	}
	for q := range m {
		bVar := basis[q]
		if bVar < d.n {
			for i := range m {
				for k, col := range d.rowCols[i] {
					if col == bVar {
						mat[q][i].Add(mat[q][i], d.rowVals[i][k])
					}
				}
			}
		} else {
			si := bVar - d.n
			if uint(si) < uint(m) {
				mat[q][si].SetInt64(1)
			}
		}
	}
	return mat
}

func (d *ratCompiledData) basisDualsRat(basis []int) ([]*big.Rat, bool) {
	if d.m == 0 {
		return nil, true
	}
	rhs := make([]*big.Rat, d.m)
	for q := range d.m {
		bVar := basis[q]
		if bVar < d.n {
			rhs[q] = new(big.Rat).Set(d.objMin[bVar])
		} else {
			rhs[q] = new(big.Rat)
		}
	}
	return solveDenseRat(d.basisTransposeMatrix(basis), rhs)
}

func (d *ratCompiledData) evalSafeDualRat(y []*big.Rat, nodeLB, nodeUB []*big.Rat) (*big.Rat, []*big.Rat, bool) {
	colDots := make([]*big.Rat, d.n)
	for j := range d.n {
		colDots[j] = new(big.Rat)
	}
	rhs := new(big.Rat)
	term := new(big.Rat)
	for i := range d.m {
		yi := y[i]
		if yi.Sign() == 0 {
			continue
		}
		if d.rowSense[i] == LessEqual && yi.Sign() > 0 {
			return nil, nil, false
		}
		if d.rowSense[i] == GreaterEqual && yi.Sign() < 0 {
			return nil, nil, false
		}
		term.Mul(yi, d.rowRHS[i])
		rhs.Add(rhs, term)
		for k, col := range d.rowCols[i] {
			term.Mul(yi, d.rowVals[i][k])
			colDots[col].Add(colDots[col], term)
		}
	}
	rc := new(big.Rat)
	for j := range d.n {
		rc.Sub(d.objMin[j], colDots[j])
		if rc.Sign() > 0 {
			if nodeLB[j] == nil {
				return nil, nil, false
			}
			term.Mul(rc, nodeLB[j])
			rhs.Add(rhs, term)
		} else if rc.Sign() < 0 {
			if nodeUB[j] == nil {
				return nil, nil, false
			}
			term.Mul(rc, nodeUB[j])
			rhs.Add(rhs, term)
		}
	}
	return rhs, colDots, true
}

func (d *ratCompiledData) projectConeFarkas(wRaw []*big.Rat, nodeLB, nodeUB []*big.Rat) ([]*big.Rat, []*big.Rat, bool) {
	wProj := make([]*big.Rat, d.m)
	anyNZ := false
	for i := range d.m {
		wi := new(big.Rat).Set(wRaw[i])
		if d.rowSense[i] == LessEqual && wi.Sign() < 0 {
			wi.SetInt64(0)
		}
		if d.rowSense[i] == GreaterEqual && wi.Sign() > 0 {
			wi.SetInt64(0)
		}
		if wi.Sign() != 0 {
			anyNZ = true
		}
		wProj[i] = wi
	}
	if !anyNZ {
		return nil, nil, false
	}
	v := make([]*big.Rat, d.n)
	for j := range d.n {
		v[j] = new(big.Rat)
	}
	gap := new(big.Rat)
	term := new(big.Rat)
	for i := range d.m {
		if wProj[i].Sign() == 0 {
			continue
		}
		term.Mul(wProj[i], d.rowRHS[i])
		gap.Add(gap, term)
		for k, col := range d.rowCols[i] {
			term.Mul(wProj[i], d.rowVals[i][k])
			v[col].Add(v[col], term)
		}
	}
	for j := range d.n {
		if v[j].Sign() > 0 {
			if nodeLB[j] == nil {
				return nil, nil, false
			}
			term.Mul(v[j], nodeLB[j])
			gap.Sub(gap, term)
		} else if v[j].Sign() < 0 {
			if nodeUB[j] == nil {
				return nil, nil, false
			}
			term.Mul(v[j], nodeUB[j])
			gap.Sub(gap, term)
		}
	}
	if gap.Sign() >= 0 {
		return nil, nil, false
	}
	return wProj, v, true
}

func (d *ratCompiledData) findNodeFarkas(s *LPSolver, lpDuals []float64, nodeLB, nodeUB []*big.Rat) ([]*big.Rat, []*big.Rat, bool) {
	m := d.m
	if m == 0 {
		return nil, nil, false
	}
	tryRHS := func(rhs []*big.Rat) ([]*big.Rat, []*big.Rat, bool) {
		wRaw, ok := solveDenseRat(d.basisTransposeMatrix(s.ws.basis[:m]), rhs)
		if !ok {
			return nil, nil, false
		}
		return d.projectConeFarkas(wRaw, nodeLB, nodeUB)
	}

	if s.lastDualPhase1Used && !s.lastPrimalFallback && uint(s.lastInfeasRow) < uint(m) {
		rhs := make([]*big.Rat, m)
		for i := range m {
			rhs[i] = new(big.Rat)
		}
		sgn := int64(-1)
		if s.lastInfeasDir < 0 {
			sgn = 1
		}
		rhs[s.lastInfeasRow].SetInt64(sgn)
		if w, v, ok := tryRHS(rhs); ok {
			return w, v, true
		}
	}

	if len(lpDuals) == m {
		wFloat := make([]*big.Rat, m)
		for i := range m {
			if r := new(big.Rat).SetFloat64(lpDuals[i]); r != nil {
				wFloat[i] = r
			} else {
				wFloat[i] = new(big.Rat)
			}
		}
		if w, v, ok := d.projectConeFarkas(wFloat, nodeLB, nodeUB); ok {
			return w, v, true
		}
		negFloat := make([]*big.Rat, m)
		for i := range m {
			negFloat[i] = new(big.Rat).Neg(wFloat[i])
		}
		if w, v, ok := d.projectConeFarkas(negFloat, nodeLB, nodeUB); ok {
			return w, v, true
		}
	}

	rhs := make([]*big.Rat, m)
	for i := range m {
		rhs[i] = new(big.Rat)
	}
	for r := range m {
		for _, sgn := range [2]int64{-1, 1} {
			rhs[r].SetInt64(sgn)
			if w, v, ok := tryRHS(rhs); ok {
				return w, v, true
			}
			rhs[r].SetInt64(0)
		}
	}
	return nil, nil, false
}

func (d *ratCompiledData) refineExactIncumbent(tb testing.TB, p *CompiledProblem, approx []float64) ([]*big.Rat, *big.Rat) {
	tb.Helper()
	fixedLB := make([]float64, d.n)
	fixedUB := make([]float64, d.n)
	xRat := make([]*big.Rat, d.n)
	hasCont := false
	for j := range d.n {
		if d.isInt[j] {
			val := math.Round(approx[j])
			fixedLB[j] = val
			fixedUB[j] = val
			xRat[j] = big.NewRat(int64(val), 1)
		} else {
			hasCont = true
			fixedLB[j], fixedUB[j] = p.VarBounds(j)
		}
	}

	if !hasCont {
		return xRat, d.evalObjNoConst(xRat)
	}

	s := NewLPSolver(p, nil)
	defer s.Close()
	s.SetStructuralBounds(fixedLB, fixedUB)
	lpRes := s.Solve()
	if lpRes.Status != StatusOptimal {
		tb.Fatalf("fixed-integer continuous LP not optimal: status=%v", lpRes.Status)
	}

	isBasicStruct := make([]bool, d.n)
	var basicCols []int
	for q := range d.m {
		bv := s.ws.basis[q]
		if bv < d.n {
			isBasicStruct[bv] = true
			basicCols = append(basicCols, bv)
		}
	}

	isBasicSlack := make([]bool, d.m)
	for q := range d.m {
		bv := s.ws.basis[q]
		if bv >= d.n {
			si := bv - d.n
			if uint(si) < uint(d.m) {
				isBasicSlack[si] = true
			}
		}
	}

	var activeRows []int
	for i := range d.m {
		if !isBasicSlack[i] {
			activeRows = append(activeRows, i)
		}
	}

	for j := range d.n {
		if d.isInt[j] || isBasicStruct[j] {
			continue
		}
		lo, hi := p.VarBounds(j)
		switch {
		case !math.IsInf(lo, -1) && math.Abs(lpRes.Primal[j]-lo) <= 1e-7:
			xRat[j] = mustRatFromFloat(tb, lo)
		case !math.IsInf(hi, 1) && math.Abs(lpRes.Primal[j]-hi) <= 1e-7:
			xRat[j] = mustRatFromFloat(tb, hi)
		case !math.IsInf(lo, -1) && lpRes.ReducedCosts[j] >= 0:
			xRat[j] = mustRatFromFloat(tb, lo)
		case !math.IsInf(hi, 1) && lpRes.ReducedCosts[j] <= 0:
			xRat[j] = mustRatFromFloat(tb, hi)
		default:
			xRat[j] = mustRatFromFloat(tb, lpRes.Primal[j])
		}
	}

	if len(basicCols) == 0 {
		return xRat, d.evalObjNoConst(xRat)
	}
	if len(activeRows) != len(basicCols) {
		tb.Fatalf("basis count mismatch solving continuous incumbent: %d != %d", len(activeRows), len(basicCols))
	}

	kMat := len(basicCols)
	colPos := make([]int, d.n)
	for j := range d.n {
		colPos[j] = -1
	}
	for pos, col := range basicCols {
		colPos[col] = pos
	}

	mat := make([][]*big.Rat, kMat)
	rhs := make([]*big.Rat, kMat)
	term := new(big.Rat)
	for rIdx, rowID := range activeRows {
		row := make([]*big.Rat, kMat)
		for c := range kMat {
			row[c] = new(big.Rat)
		}
		rRHS := new(big.Rat).Set(d.rowRHS[rowID])
		for idx, col := range d.rowCols[rowID] {
			coeff := d.rowVals[rowID][idx]
			if pPos := colPos[col]; pPos >= 0 && !d.isInt[col] {
				row[pPos].Add(row[pPos], coeff)
			} else {
				term.Mul(coeff, xRat[col])
				rRHS.Sub(rRHS, term)
			}
		}
		mat[rIdx] = row
		rhs[rIdx] = rRHS
	}

	solB, ok := solveDenseRat(mat, rhs)
	if !ok {
		tb.Fatalf("singular continuous basis solving exact incumbent")
	}
	for pos, col := range basicCols {
		if !d.isInt[col] {
			xRat[col] = solB[pos]
		}
	}
	return xRat, d.evalObjNoConst(xRat)
}

func (d *ratCompiledData) evalObjNoConst(x []*big.Rat) *big.Rat {
	sum := new(big.Rat)
	term := new(big.Rat)
	for j := range d.n {
		term.Mul(d.objMin[j], x[j])
		sum.Add(sum, term)
	}
	return sum
}

type viprTracer struct {
	tb       testing.TB
	p        *CompiledProblem
	d        *ratCompiledData
	lp       *LPSolver
	cutoff   *big.Rat
	steps    []viprStep
	maxNodes int
}

func (tr *viprTracer) addStep(s viprStep) int {
	id := tr.d.m + len(tr.steps)
	tr.steps = append(tr.steps, s)
	return id
}

func (tr *viprTracer) emitAssume(col int, isUpper bool, bound int64) int {
	sense := ">="
	if isUpper {
		sense = "<="
	}
	id := tr.d.m + len(tr.steps)
	tr.steps = append(tr.steps, viprStep{
		Rule:        "assume",
		Col:         optIntPtr(col),
		Sense:       sense,
		Bound:       optInt64Ptr(bound),
		Assumptions: []int{id},
	})
	return id
}

func (tr *viprTracer) emitFarkas(w, v []*big.Rat, lbAsmID, ubAsmID []int) int {
	var refs []viprLinRef
	var bounds []viprBoundRef
	for i := range tr.d.m {
		if w[i].Sign() != 0 {
			refs = append(refs, viprLinRef{ID: i, Mult: bigRatToPayload(w[i])})
		}
	}
	neg := new(big.Rat)
	for j := range tr.d.n {
		if v[j].Sign() == 0 {
			continue
		}
		neg.Neg(v[j])
		if v[j].Sign() > 0 {
			if lbAsmID[j] >= 0 {
				refs = append(refs, viprLinRef{ID: lbAsmID[j], Mult: bigRatToPayload(neg)})
			} else {
				bounds = append(bounds, viprBoundRef{Col: j, Kind: "lb", Mult: bigRatToPayload(neg)})
			}
		} else {
			if ubAsmID[j] >= 0 {
				refs = append(refs, viprLinRef{ID: ubAsmID[j], Mult: bigRatToPayload(neg)})
			} else {
				bounds = append(bounds, viprBoundRef{Col: j, Kind: "ub", Mult: bigRatToPayload(neg)})
			}
		}
	}
	return tr.addStep(viprStep{
		Rule:   "farkas",
		Refs:   optRefs(refs),
		Bounds: bounds,
	})
}

func (tr *viprTracer) emitSafeDual(y []*big.Rat, boundAsms []int) int {
	var refs []viprLinRef
	for i := range tr.d.m {
		if y[i].Sign() != 0 {
			refs = append(refs, viprLinRef{ID: i, Mult: bigRatToPayload(y[i])})
		}
	}
	asms := slices.Clone(boundAsms)
	return tr.addStep(viprStep{
		Rule:      "safeDual",
		Refs:      optRefs(refs),
		BoundAsms: asms,
	})
}

func (tr *viprTracer) clampWrongConeDuals(y []*big.Rat) []*big.Rat {
	out := make([]*big.Rat, len(y))
	for i, yi := range y {
		val := new(big.Rat).Set(yi)
		if tr.d.rowSense[i] == LessEqual && val.Sign() > 0 {
			val.SetInt64(0)
		}
		if tr.d.rowSense[i] == GreaterEqual && val.Sign() < 0 {
			val.SetInt64(0)
		}
		out[i] = val
	}
	return out
}

func (tr *viprTracer) selectBranchVar(primal []float64, nodeLB, nodeUB []*big.Rat) (int, int64, bool) {
	bestCol := -1
	bestScore := -1.0
	var bestSplit int64
	for j := range tr.d.n {
		if !tr.d.isInt[j] {
			continue
		}
		if nodeLB[j] != nil && nodeUB[j] != nil {
			loF, _ := nodeLB[j].Float64()
			hiF, _ := nodeUB[j].Float64()
			if hiF-loF < 0.5 {
				continue
			}
		}
		v := primal[j]
		fl := math.Floor(v)
		cl := math.Ceil(v)
		dist := min(v-fl, cl-v)
		if dist > 1e-6 && dist > bestScore {
			split := int64(fl)
			if tr.canSplitAt(split, nodeLB[j], nodeUB[j]) {
				bestCol = j
				bestScore = dist
				bestSplit = split
			}
		}
	}
	if bestCol >= 0 {
		return bestCol, bestSplit, true
	}
	// Split an unfixed integer column when lpRes.Primal matches an integer coordinate while node duals stay below tr.cutoff.
	for j := range tr.d.n {
		if !tr.d.isInt[j] {
			continue
		}
		if nodeLB[j] != nil && nodeUB[j] != nil && nodeLB[j].Cmp(nodeUB[j]) == 0 {
			continue
		}
		v := primal[j]
		split := int64(math.Floor(v + 0.5))
		if nodeLB[j] != nil {
			loCeil := ratCeilInt64(nodeLB[j])
			if split < loCeil {
				split = loCeil
			}
		}
		if nodeUB[j] != nil {
			hiFloor := ratFloorInt64(nodeUB[j])
			if split >= hiFloor {
				split = hiFloor - 1
			}
		}
		if tr.canSplitAt(split, nodeLB[j], nodeUB[j]) {
			return j, split, true
		}
	}
	return -1, 0, false
}

func ratFloorInt64(r *big.Rat) int64 {
	q, rem := new(big.Int).QuoRem(r.Num(), r.Denom(), new(big.Int))
	if rem.Sign() < 0 {
		q.Sub(q, big.NewInt(1))
	}
	return q.Int64()
}

func ratCeilInt64(r *big.Rat) int64 {
	q, rem := new(big.Int).QuoRem(r.Num(), r.Denom(), new(big.Int))
	if rem.Sign() > 0 {
		q.Add(q, big.NewInt(1))
	}
	return q.Int64()
}

func (tr *viprTracer) canSplitAt(split int64, lo, hi *big.Rat) bool {
	if lo != nil && lo.Cmp(big.NewRat(split+1, 1)) >= 0 {
		return false
	}
	if hi != nil && hi.Cmp(big.NewRat(split, 1)) <= 0 {
		return false
	}
	return true
}

func (tr *viprTracer) emitNode(
	nodeLBF, nodeUBF []float64,
	nodeLBR, nodeUBR []*big.Rat,
	lbAsmID, ubAsmID []int,
	boundAsms []int,
	depth int,
) int {
	tr.tb.Helper()
	if tr.maxNodes--; tr.maxNodes < 0 || depth > 32 {
		tr.tb.Fatalf("vipr tracer node/depth limit exceeded on %s", tr.p.Name())
	}

	for j := range tr.d.n {
		if nodeLBR[j] != nil && nodeUBR[j] != nil && nodeUBR[j].Cmp(nodeLBR[j]) < 0 {
			return tr.addStep(viprStep{
				Rule:  "boundConflict",
				Col:   optIntPtr(j),
				LBRef: optNonNegPtr(lbAsmID[j]),
				UBRef: optNonNegPtr(ubAsmID[j]),
			})
		}
	}

	tr.lp.SetStructuralBounds(nodeLBF, nodeUBF)
	lpRes := tr.lp.Solve()

	switch lpRes.Status {
	case StatusInfeasible:
		w, v, ok := tr.d.findNodeFarkas(tr.lp, lpRes.Dual, nodeLBR, nodeUBR)
		if !ok {
			tr.tb.Fatalf("failed to extract exact Farkas ray on infeasible node (depth %d, %s)", depth, tr.p.Name())
		}
		return tr.emitFarkas(w, v, lbAsmID, ubAsmID)

	case StatusOptimal:
		yBasis, okY := tr.d.basisDualsRat(tr.lp.ws.basis[:tr.d.m])
		if okY {
			yClean := tr.clampWrongConeDuals(yBasis)
			if dualLB, _, okDual := tr.d.evalSafeDualRat(yClean, nodeLBR, nodeUBR); okDual {
				if tr.cutoff != nil && dualLB.Cmp(tr.cutoff) >= 0 {
					return tr.emitSafeDual(yClean, boundAsms)
				}
			}
		}

		branchCol, splitVal, okBranch := tr.selectBranchVar(lpRes.Primal, nodeLBR, nodeUBR)
		if !okBranch {
			tr.tb.Fatalf("node optimal without cutoff fathom or integer branch candidate (depth %d, %s)", depth, tr.p.Name())
		}

		leftAsmID := tr.emitAssume(branchCol, true, splitVal)
		leftLBF := slices.Clone(nodeLBF)
		leftUBF := slices.Clone(nodeUBF)
		leftLBR := slices.Clone(nodeLBR)
		leftUBR := slices.Clone(nodeUBR)
		leftLBAsm := slices.Clone(lbAsmID)
		leftUBAsm := slices.Clone(ubAsmID)
		leftUBF[branchCol] = float64(splitVal)
		leftUBR[branchCol] = big.NewRat(splitVal, 1)
		leftUBAsm[branchCol] = leftAsmID
		leftBoundAsms := append(slices.Clone(boundAsms), leftAsmID)
		leftLeaf := tr.emitNode(leftLBF, leftUBF, leftLBR, leftUBR, leftLBAsm, leftUBAsm, leftBoundAsms, depth+1)

		rightAsmID := tr.emitAssume(branchCol, false, splitVal+1)
		rightLBF := slices.Clone(nodeLBF)
		rightUBF := slices.Clone(nodeUBF)
		rightLBR := slices.Clone(nodeLBR)
		rightUBR := slices.Clone(nodeUBR)
		rightLBAsm := slices.Clone(lbAsmID)
		rightUBAsm := slices.Clone(ubAsmID)
		rightLBF[branchCol] = float64(splitVal + 1)
		rightLBR[branchCol] = big.NewRat(splitVal+1, 1)
		rightLBAsm[branchCol] = rightAsmID
		rightBoundAsms := append(slices.Clone(boundAsms), rightAsmID)
		rightLeaf := tr.emitNode(rightLBF, rightUBF, rightLBR, rightUBR, rightLBAsm, rightUBAsm, rightBoundAsms, depth+1)

		return tr.addStep(viprStep{
			Rule:     "branch",
			Col:      optIntPtr(branchCol),
			SplitVal: optInt64Ptr(splitVal),
			LeftAsm:  optIntPtr(leftAsmID),
			RightAsm: optIntPtr(rightAsmID),
			Left:     optIntPtr(leftLeaf),
			Right:    optIntPtr(rightLeaf),
		})

	default:
		tr.tb.Fatalf("unexpected node LP status %v on %s (depth %d)", lpRes.Status, tr.p.Name(), depth)
		return -1
	}
}

func traceMIPWithVIPR(tb testing.TB, exe, name string, p *CompiledProblem) viprCert {
	tb.Helper()
	res, err := SolveCompiled(tb.Context(), p, WithPureGo(), WithWorkers(1))
	if err != nil {
		tb.Fatalf("%s SolveCompiled error: %v", name, err)
	}

	d := buildRatCompiledData(tb, p)
	s := NewLPSolver(p, nil)
	defer s.Close()

	var mode string
	var xStarRat []*big.Rat
	var cutoff *big.Rat
	switch res.Status {
	case StatusInfeasible:
		mode = "infeasible"
	case StatusOptimal:
		mode = "optimal"
		xStarRat, cutoff = d.refineExactIncumbent(tb, p, res.Values)
	default:
		tb.Fatalf("%s unexpected SolveCompiled status %v", name, res.Status)
	}

	lbF := make([]float64, d.n)
	ubF := make([]float64, d.n)
	lbAsm := make([]int, d.n)
	ubAsm := make([]int, d.n)
	for j := range d.n {
		lbF[j], ubF[j] = p.VarBounds(j)
		lbAsm[j] = -1
		ubAsm[j] = -1
	}

	tr := &viprTracer{
		tb:       tb,
		p:        p,
		d:        d,
		lp:       s,
		cutoff:   cutoff,
		maxNodes: 256,
	}
	rootEntryID := tr.emitNode(lbF, ubF, slices.Clone(d.lb), slices.Clone(d.ub), lbAsm, ubAsm, nil, 0)

	cert := d.buildVIPRBase(mode)
	if mode == "optimal" {
		incPay := make([]ratPayload, d.n)
		for j := range d.n {
			incPay[j] = bigRatToPayload(xStarRat[j])
		}
		cert.Incumbent = incPay
	}
	cert.Steps = tr.steps
	cert.RootStep = rootEntryID - d.m

	requireVIPRVerified(tb, exe, name, cert)
	return cert
}

func buildMultiNodeBranchMIP1() *Model {
	m := NewModel("multinode_mixed_3v", Minimize)
	x0 := m.AddIntVar("x0", 0, 3)
	x1 := m.AddIntVar("x1", 0, 3)
	x2 := m.AddContVar("x2", 0, 2)
	m.SetObjective(NewExpr(x0.T(3), x1.T(2), x2.T(1)))
	m.AddConstraint("c0", Ge(NewExpr(x0.T(2), x1.T(2)), 3))
	m.AddConstraint("c1", Le(NewExpr(x0.T(2), x1.T(-2)), 1))
	m.AddConstraint("c2", Ge(NewExpr(x1.T(1), x2.T(1)), 2))
	return m
}

func buildMultiNodeBranchMIP2() *Model {
	m := NewModel("multinode_double_cg_bin", Maximize)
	x0 := m.AddBinaryVar("x0")
	x1 := m.AddBinaryVar("x1")
	x2 := m.AddBinaryVar("x2")
	x3 := m.AddBinaryVar("x3")
	m.SetObjective(NewExpr(x0.T(5), x1.T(4), x2.T(3), x3.T(2)))
	m.AddConstraint("c0", Le(NewExpr(x0.T(2), x1.T(2)), 3))
	m.AddConstraint("c1", Le(NewExpr(x2.T(2), x3.T(2)), 3))
	return m
}

func buildMultiNodeInfeasibleMIP() *Model {
	m := NewModel("multinode_parity_inf", Minimize)
	x0 := m.AddBinaryVar("x0")
	x1 := m.AddBinaryVar("x1")
	x2 := m.AddBinaryVar("x2")
	m.SetObjective(NewExpr(x0.T(1), x1.T(1), x2.T(1)))
	m.AddConstraint("parity", Eq(NewExpr(x0.T(2), x1.T(2), x2.T(2)), 5))
	return m
}

func TestFormalTrack4(t *testing.T) {
	t.Parallel()
	requireFormalEnv(t)
	exe := formalExe(t, "vipr-check")

	t.Run("SampleFixtureAndUsage", func(t *testing.T) {
		t.Parallel()
		usageOut, usageErr := exec.CommandContext(t.Context(), exe).CombinedOutput()
		if usageErr != nil || !strings.Contains(string(usageOut), "usage:") {
			t.Fatalf("expected exit 0 usage message, got err=%v out=%q", usageErr, string(usageOut))
		}
		samplePath := filepath.Join("testdata", "formal", "vipr_sample.json")
		out, err := exec.CommandContext(t.Context(), exe, samplePath).CombinedOutput()
		if err != nil || strings.TrimSpace(string(out)) != "VERIFIED" {
			t.Fatalf("vipr_sample.json failed: err=%v out=%q", err, string(out))
		}
	})

	t.Run("CorpusAndMultiNodeMIPs", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name    string
			relPath string
			build   func() *Model
		}{
			{name: "t1_infeasible_mip", relPath: filepath.Join("testdata", "corpus", "t1_infeasible_mip.json")},
			{name: "t1_std_mip", relPath: filepath.Join("testdata", "corpus", "t1_std_mip.json")},
			{name: "t1_furniture", relPath: filepath.Join("testdata", "corpus", "t1_furniture.json")},
			{name: "t1_knapsack_12", relPath: filepath.Join("testdata", "corpus", "t1_knapsack_12.json")},
			{name: "multinode_mixed_3v", build: buildMultiNodeBranchMIP1},
			{name: "multinode_double_cg_bin", build: buildMultiNodeBranchMIP2},
			{name: "multinode_parity_inf", build: buildMultiNodeInfeasibleMIP},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				var mdl *Model
				if tc.relPath != "" {
					mdl = loadJSONModelForTest(t, tc.relPath)
				} else {
					mdl = tc.build()
				}
				p := mdl.Compile()
				cert := traceMIPWithVIPR(t, exe, tc.name, p)
				hasBranch := false
				for _, st := range cert.Steps {
					if st.Rule == "branch" {
						hasBranch = true
						break
					}
				}
				if !hasBranch && tc.name != "t1_furniture" {
					t.Fatalf("%s expected multi-node branching steps, got %d steps with no branch", tc.name, len(cert.Steps))
				}
			})
		}
	})

	t.Run("RootCGRoundingInfeasible", func(t *testing.T) {
		t.Parallel()
		// Derive conflicting bounds via roundDown and roundUp at the root node.
		p := loadJSONModelForTest(t, filepath.Join("testdata", "corpus", "t1_infeasible_mip.json")).Compile()
		d := buildRatCompiledData(t, p)
		cert := d.buildVIPRBase("infeasible")
		half := bigRatToPayload(big.NewRat(1, 2))
		cert.Steps = []viprStep{
			{Rule: "lincomb", Sense: "<=", Refs: optRefs([]viprLinRef{{ID: 0, Mult: half}})},
			{Rule: "roundDown", Ref: optIntPtr(d.m + 0)},
			{Rule: "lincomb", Sense: ">=", Refs: optRefs([]viprLinRef{{ID: 0, Mult: half}})},
			{Rule: "roundUp", Ref: optIntPtr(d.m + 2)},
			{Rule: "farkas", Refs: optRefs([]viprLinRef{
				{ID: d.m + 1, Mult: int64ToPayload(1)},
				{ID: d.m + 3, Mult: int64ToPayload(-1)},
			})},
		}
		cert.RootStep = 4
		requireVIPRVerified(t, exe, "t1_infeasible_mip_cg_root", cert)
	})

	t.Run("NegativeControls", func(t *testing.T) {
		t.Parallel()
		// Reject a branch rule that leaves an integer gap between splitVal and rightAsm.
		zeroPay := int64ToPayload(0)
		onePay := int64ToPayload(1)
		holeCert := viprCert{
			N:        1,
			ObjConst: zeroPay,
			Obj:      []ratPayload{onePay},
			IsInt:    []bool{true},
			LB:       []*ratPayload{&zeroPay},
			UB:       []*ratPayload{optRatPtr(4, 1)},
			BaseRows: []viprLinCon{{Coeffs: []ratPayload{onePay}, Sense: ">=", RHS: int64ToPayload(2)}},
			Mode:     "infeasible",
			Steps: []viprStep{
				{Rule: "assume", Col: optIntPtr(0), Sense: "<=", Bound: optInt64Ptr(1)},
				{Rule: "farkas", Refs: optRefs([]viprLinRef{{ID: 1, Mult: onePay}, {ID: 0, Mult: int64ToPayload(-1)}})},
				{Rule: "assume", Col: optIntPtr(0), Sense: ">=", Bound: optInt64Ptr(3)},
				{Rule: "safeDual", Refs: optRefs([]viprLinRef{{ID: 3, Mult: onePay}})},
				{Rule: "branch", Col: optIntPtr(0), SplitVal: optInt64Ptr(1), LeftAsm: optIntPtr(1), RightAsm: optIntPtr(3), Left: optIntPtr(2), Right: optIntPtr(4)},
			},
			RootStep: 4,
		}
		requireVIPRRejected(t, exe, "neg_branch_hole", holeCert)

		// Reject a right subtree leaf that references the sibling left assume step.
		leakCert := viprCert{
			N:        1,
			ObjConst: zeroPay,
			Obj:      []ratPayload{onePay},
			IsInt:    []bool{true},
			LB:       []*ratPayload{&zeroPay},
			UB:       []*ratPayload{optRatPtr(2, 1)},
			BaseRows: []viprLinCon{{Coeffs: []ratPayload{onePay}, Sense: ">=", RHS: zeroPay}},
			Mode:     "infeasible",
			Steps: []viprStep{
				{Rule: "assume", Col: optIntPtr(0), Sense: "<=", Bound: optInt64Ptr(0)},
				{Rule: "assume", Col: optIntPtr(0), Sense: ">=", Bound: optInt64Ptr(1)},
				{Rule: "farkas", Refs: optRefs([]viprLinRef{{ID: 1, Mult: onePay}, {ID: 2, Mult: int64ToPayload(-1)}})},
				{Rule: "farkas", Refs: optRefs([]viprLinRef{{ID: 1, Mult: onePay}, {ID: 0, Mult: int64ToPayload(-1)}})},
				{Rule: "branch", Col: optIntPtr(0), SplitVal: optInt64Ptr(0), LeftAsm: optIntPtr(1), RightAsm: optIntPtr(2), Left: optIntPtr(4), Right: optIntPtr(3)},
			},
			RootStep: 4,
		}
		requireVIPRRejected(t, exe, "neg_assumption_leak", leakCert)

		// Reject roundDown applied to a constraint with a nonzero continuous coefficient.
		cgContCert := viprCert{
			N:         2,
			ObjConst:  zeroPay,
			Obj:       []ratPayload{onePay, onePay},
			IsInt:     []bool{true, false},
			LB:        []*ratPayload{&zeroPay, &zeroPay},
			UB:        []*ratPayload{&onePay, &onePay},
			BaseRows:  []viprLinCon{{Coeffs: []ratPayload{onePay, onePay}, Sense: "<=", RHS: ratPayload{Num: "3", Den: "2"}}},
			Mode:      "optimal",
			Incumbent: []ratPayload{zeroPay, zeroPay},
			Steps: []viprStep{
				{Rule: "roundDown", Ref: optIntPtr(0)},
			},
			RootStep: 0,
		}
		requireVIPRRejected(t, exe, "neg_cg_continuous", cgContCert)

		// Reject infeasible incumbent.
		badIncCert := viprCert{
			N:         1,
			ObjConst:  zeroPay,
			Obj:       []ratPayload{onePay},
			IsInt:     []bool{true},
			LB:        []*ratPayload{&zeroPay},
			UB:        []*ratPayload{optRatPtr(2, 1)},
			BaseRows:  []viprLinCon{{Coeffs: []ratPayload{onePay}, Sense: ">=", RHS: onePay}},
			Mode:      "optimal",
			Incumbent: []ratPayload{zeroPay},
			Steps:     []viprStep{{Rule: "safeDual", Refs: optRefs([]viprLinRef{{ID: 0, Mult: onePay}})}},
			RootStep:  0,
		}
		requireVIPRRejected(t, exe, "neg_infeasible_incumbent", badIncCert)

		// Reject continuous assume.
		contBranchCert := viprCert{
			N:        1,
			ObjConst: zeroPay,
			Obj:      []ratPayload{onePay},
			IsInt:    []bool{false},
			LB:       []*ratPayload{&zeroPay},
			UB:       []*ratPayload{optRatPtr(2, 1)},
			BaseRows: []viprLinCon{},
			Mode:     "infeasible",
			Steps: []viprStep{
				{Rule: "assume", Col: optIntPtr(0), Sense: "<=", Bound: optInt64Ptr(0)},
			},
			RootStep: 0,
		}
		requireVIPRRejected(t, exe, "neg_continuous_assume", contBranchCert)

		// Reject a positive multiplier on a LessEqual constraint inside safeDual.
		badSignCert := viprCert{
			N:         1,
			ObjConst:  zeroPay,
			Obj:       []ratPayload{onePay},
			IsInt:     []bool{true},
			LB:        []*ratPayload{&zeroPay},
			UB:        []*ratPayload{optRatPtr(3, 1)},
			BaseRows:  []viprLinCon{{Coeffs: []ratPayload{onePay}, Sense: "<=", RHS: int64ToPayload(2)}},
			Mode:      "optimal",
			Incumbent: []ratPayload{zeroPay},
			Steps:     []viprStep{{Rule: "safeDual", Refs: optRefs([]viprLinRef{{ID: 0, Mult: onePay}})}},
			RootStep:  0,
		}
		requireVIPRRejected(t, exe, "neg_bad_multiplier_sign", badSignCert)

		// Reject a root safeDual lower bound strictly weaker than the incumbent objective.
		twoPay := int64ToPayload(2)
		weakBoundCert := viprCert{
			N:         1,
			ObjConst:  zeroPay,
			Obj:       []ratPayload{twoPay},
			IsInt:     []bool{true},
			LB:        []*ratPayload{&zeroPay},
			UB:        []*ratPayload{optRatPtr(3, 1)},
			BaseRows:  []viprLinCon{{Coeffs: []ratPayload{twoPay}, Sense: ">=", RHS: int64ToPayload(3)}},
			Mode:      "optimal",
			Incumbent: []ratPayload{twoPay},
			Steps:     []viprStep{{Rule: "safeDual", Refs: optRefs([]viprLinRef{{ID: 0, Mult: onePay}})}},
			RootStep:  0,
		}
		requireVIPRRejected(t, exe, "neg_weak_dual_bound", weakBoundCert)
	})
}

func optRatPtr(n, d int64) *ratPayload {
	r := bigRatToPayload(big.NewRat(n, d))
	return &r
}
