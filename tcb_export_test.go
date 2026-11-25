package goblin

import (
	"context"
	"math/big"
	"slices"
	"strings"
)

// VIPR trace tampering modes exposed to package goblin_test fuzz targets.
const (
	VIPRTamperMult = iota
	VIPRTamperDropAsm
	VIPRTamperFlipSense
	VIPRTamperOrbitPerm
	VIPRTamperInflateRat
	VIPRTamperPresolve
	VIPRTamperHeader
	VIPRTamperWipe
	VIPRTamperCount
)

// CertLPFaultTargetForTest aliases mutable fields of an internal LP witness
// so fault tests can corrupt individual fields cleanly.
type CertLPFaultTargetForTest struct {
	Primal          []*big.Rat
	DualMin         []*big.Rat
	FarkasW         []*big.Rat
	Unbounded       UnboundedCertificate
	HasUnbounded    bool
	HasDirectInfeas bool
	Polished        bool
}

// WithCertFaultForTest attaches fn under ctx so solveCertified invokes fn before
// fail-closed verification.
func WithCertFaultForTest(
	ctx context.Context,
	fn func(p *CompiledProblem, trace *VIPRTrace, lpWit *CertLPFaultTargetForTest, res *Result),
) context.Context {
	if fn == nil {
		return ctx
	}
	return context.WithValue(ctx, goblinCertFaultKey{}, goblinCertFaultFunc(func(p *CompiledProblem, trace *VIPRTrace, wit *lpCertWitness, res *Result) {
		var tgt *CertLPFaultTargetForTest
		if wit != nil {
			var clonedUnb UnboundedCertificate
			if cu := cloneUnboundedCertificate(wit.unbounded); cu != nil {
				clonedUnb = *cu
			}
			tgt = &CertLPFaultTargetForTest{
				Primal:          cloneExactRatSlice(wit.primal),
				DualMin:         cloneExactRatSlice(wit.dualMin),
				FarkasW:         cloneExactRatSlice(wit.farkasW),
				Unbounded:       clonedUnb,
				HasUnbounded:    wit.hasUnbounded,
				HasDirectInfeas: wit.hasDirectInfeas,
				Polished:        wit.polished,
			}
		}
		fn(p, trace, tgt, res)
		if wit != nil && tgt != nil {
			wit.primal = tgt.Primal
			wit.dualMin = tgt.DualMin
			wit.farkasW = tgt.FarkasW
			if cu := cloneUnboundedCertificate(tgt.Unbounded); cu != nil {
				wit.unbounded = *cu
			}
			wit.hasUnbounded = tgt.HasUnbounded
			wit.hasDirectInfeas = tgt.HasDirectInfeas
			wit.polished = tgt.Polished
		}
	}))
}

// VerifyMiniVIPRForTest runs the internal Go Mini-VIPR parser and verifier on raw
// JSON bytes, checking agreement with p when p is non-nil.
func VerifyMiniVIPRForTest(p *CompiledProblem, raw []byte) error {
	if p != nil {
		_, err := verifyMiniVIPRProblem(p, raw)
		return err
	}
	_, err := verifyMiniVIPRBytes(raw, nil)
	return err
}

func cloneVIPROptIntPtr(v *int) *int {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

func cloneVIPROptInt64Ptr(v *int64) *int64 {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

func cloneVIPROptBoolPtr(v *bool) *bool {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

func cloneVIPRRatPtr(v *viprRatPayload) *viprRatPayload {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

func cloneVIPRRatOptSlice(src []*viprRatPayload) []*viprRatPayload {
	if src == nil {
		return nil
	}
	out := make([]*viprRatPayload, len(src))
	for i, item := range src {
		out[i] = cloneVIPRRatPtr(item)
	}
	return out
}

func cloneVIPRBaseRows(src []viprLinConJSON) []viprLinConJSON {
	if src == nil {
		return nil
	}
	out := make([]viprLinConJSON, len(src))
	for i, r := range src {
		out[i] = viprLinConJSON{
			Coeffs: slices.Clone(r.Coeffs),
			Sense:  r.Sense,
			RHS:    r.RHS,
		}
	}
	return out
}

func cloneVIPRInstance(src *viprInstanceJSON) *viprInstanceJSON {
	if src == nil {
		return nil
	}
	return &viprInstanceJSON{
		N:        src.N,
		ObjConst: src.ObjConst,
		Obj:      slices.Clone(src.Obj),
		IsInt:    slices.Clone(src.IsInt),
		LB:       cloneVIPRRatOptSlice(src.LB),
		UB:       cloneVIPRRatOptSlice(src.UB),
		BaseRows: cloneVIPRBaseRows(src.BaseRows),
	}
}

func cloneVIPRTrace(tr *VIPRTrace) *VIPRTrace {
	if tr == nil {
		return nil
	}
	out := *tr
	if tr.targetLB != nil {
		out.targetLB = new(big.Rat).Set(tr.targetLB)
	}
	c := tr.cert
	c.Orig = cloneVIPRInstance(c.Orig)
	c.Obj = slices.Clone(c.Obj)
	c.IsInt = slices.Clone(c.IsInt)
	c.LB = cloneVIPRRatOptSlice(c.LB)
	c.UB = cloneVIPRRatOptSlice(c.UB)
	c.BaseRows = cloneVIPRBaseRows(c.BaseRows)
	c.TargetLB = cloneVIPRRatPtr(c.TargetLB)
	c.Incumbent = slices.Clone(c.Incumbent)
	if c.Presolve != nil {
		pre := make([]viprPresolveStepJSON, len(c.Presolve))
		for i, ps := range c.Presolve {
			ps.Row = cloneVIPROptIntPtr(ps.Row)
			ps.Col = cloneVIPROptIntPtr(ps.Col)
			ps.U = cloneVIPROptIntPtr(ps.U)
			ps.V = cloneVIPROptIntPtr(ps.V)
			ps.IsUpper = cloneVIPROptBoolPtr(ps.IsUpper)
			ps.RoundInt = cloneVIPROptBoolPtr(ps.RoundInt)
			ps.Divisor = cloneVIPROptInt64Ptr(ps.Divisor)
			ps.NewBound = cloneVIPRRatPtr(ps.NewBound)
			ps.Val = cloneVIPRRatPtr(ps.Val)
			ps.Delta = cloneVIPRRatPtr(ps.Delta)
			ps.ColPerm = slices.Clone(ps.ColPerm)
			ps.RowPerm = slices.Clone(ps.RowPerm)
			pre[i] = ps
		}
		c.Presolve = pre
	}
	if c.Steps != nil {
		steps := make([]viprStepJSON, len(c.Steps))
		for i, st := range c.Steps {
			st.Row = cloneVIPROptIntPtr(st.Row)
			st.Col = cloneVIPROptIntPtr(st.Col)
			st.Bound = slices.Clone(st.Bound)
			if st.Refs != nil {
				refs := slices.Clone(*st.Refs)
				st.Refs = &refs
			}
			st.Bounds = slices.Clone(st.Bounds)
			st.BoundAsms = slices.Clone(st.BoundAsms)
			st.Ref = cloneVIPROptIntPtr(st.Ref)
			st.SplitVal = slices.Clone(st.SplitVal)
			st.LeftAsm = cloneVIPROptIntPtr(st.LeftAsm)
			st.RightAsm = cloneVIPROptIntPtr(st.RightAsm)
			st.Left = cloneVIPROptIntPtr(st.Left)
			st.Right = cloneVIPROptIntPtr(st.Right)
			st.OrbitAsms = slices.Clone(st.OrbitAsms)
			if st.Witnesses != nil {
				ws := make([]viprOrbitWitnessJSON, len(st.Witnesses))
				for k, w := range st.Witnesses {
					w.ColPerm = slices.Clone(w.ColPerm)
					w.RowPerm = slices.Clone(w.RowPerm)
					w.SupportEntries = slices.Clone(w.SupportEntries)
					ws[k] = w
				}
				st.Witnesses = ws
			}
			st.ExtraAsms = slices.Clone(st.ExtraAsms)
			st.LBRef = cloneVIPROptIntPtr(st.LBRef)
			st.UBRef = cloneVIPROptIntPtr(st.UBRef)
			st.Assumptions = slices.Clone(st.Assumptions)
			steps[i] = st
		}
		c.Steps = steps
	}
	out.cert = c
	return &out
}

func tamperRatPayload(p *viprRatPayload, selector uint64) {
	if p == nil {
		return
	}
	switch selector % 4 {
	case 0:
		if p.Num == "0" {
			p.Num = "1"
		} else if trimmed, ok := strings.CutPrefix(p.Num, "-"); ok {
			p.Num = trimmed
		} else {
			p.Num = "-" + p.Num
		}
	case 1:
		var z big.Int
		if _, ok := z.SetString(p.Num, 10); ok {
			z.Add(&z, big.NewInt(1))
			p.Num = z.String()
		} else {
			p.Num = "999"
		}
	case 2:
		p.Den = "2"
	default:
		p.Num = "0"
		p.Den = "1"
	}
}

func flipVIPRSenseStr(s string, selector uint64) string {
	switch s {
	case "<=":
		if selector%2 == 0 {
			return ">="
		}
		return "="
	case ">=":
		if selector%2 == 0 {
			return "<="
		}
		return "="
	case "=":
		if selector%2 == 0 {
			return "<="
		}
		return ">="
	default:
		return "<="
	}
}

func inflatedDecimalDigits(selector uint64) string {
	// Lengths spanning 4931..4934 digits (~16380..16390 bits) and 8191..8193 chars.
	lengths := []int{4931, 4933, 8191, 8192, 8193}
	n := lengths[int(selector%uint64(len(lengths)))]
	return "9" + strings.Repeat("0", n-2) + "1"
}

func tamperVIPRMult(c *viprCertJSON, selector uint64) bool {
	var candRefs []int
	var candBnds []int
	for i, st := range c.Steps {
		if st.Refs != nil && len(*st.Refs) > 0 {
			candRefs = append(candRefs, i)
		}
		if len(st.Bounds) > 0 {
			candBnds = append(candBnds, i)
		}
	}
	if len(candRefs) > 0 && (len(candBnds) == 0 || selector%2 == 0) {
		idx := candRefs[int((selector>>4)%uint64(len(candRefs)))]
		refs := *c.Steps[idx].Refs
		rIdx := int((selector >> 8) % uint64(len(refs)))
		tamperRatPayload(&refs[rIdx].Mult, selector>>12)
		return true
	}
	if len(candBnds) > 0 {
		idx := candBnds[int((selector>>4)%uint64(len(candBnds)))]
		bIdx := int((selector >> 8) % uint64(len(c.Steps[idx].Bounds)))
		tamperRatPayload(&c.Steps[idx].Bounds[bIdx].Mult, selector>>12)
		return true
	}
	return false
}

func tamperVIPRDropAsm(c *viprCertJSON, selector uint64) bool {
	for i := range c.Steps {
		idx := int((uint64(i) + selector) % uint64(len(c.Steps)))
		cur := &c.Steps[idx]
		if len(cur.Assumptions) > 0 {
			drop := int((selector >> 4) % uint64(len(cur.Assumptions)))
			cur.Assumptions = append(cur.Assumptions[:drop], cur.Assumptions[drop+1:]...)
			return true
		}
		if len(cur.OrbitAsms) > 0 {
			cur.OrbitAsms = cur.OrbitAsms[:len(cur.OrbitAsms)-1]
			return true
		}
		if len(cur.BoundAsms) > 0 {
			cur.BoundAsms = cur.BoundAsms[:len(cur.BoundAsms)-1]
			return true
		}
		if len(cur.ExtraAsms) > 0 {
			cur.ExtraAsms = cur.ExtraAsms[:len(cur.ExtraAsms)-1]
			return true
		}
		if cur.LeftAsm != nil && cur.RightAsm != nil {
			if selector%2 == 0 {
				*cur.LeftAsm = *cur.RightAsm
			} else {
				cur.LeftAsm = nil
			}
			return true
		}
	}
	return false
}

func tamperVIPRFlipSense(c *viprCertJSON, selector uint64) bool {
	if len(c.BaseRows) > 0 && selector%2 == 0 {
		idx := int((selector >> 4) % uint64(len(c.BaseRows)))
		c.BaseRows[idx].Sense = flipVIPRSenseStr(c.BaseRows[idx].Sense, selector>>8)
		return true
	}
	for i := range c.Steps {
		idx := int((uint64(i) + selector) % uint64(len(c.Steps)))
		if c.Steps[idx].Sense != "" {
			c.Steps[idx].Sense = flipVIPRSenseStr(c.Steps[idx].Sense, selector>>8)
			return true
		}
	}
	if len(c.BaseRows) > 0 {
		c.BaseRows[0].Sense = flipVIPRSenseStr(c.BaseRows[0].Sense, selector>>8)
		return true
	}
	return false
}

func tamperVIPROrbitPerm(c *viprCertJSON, selector uint64) bool {
	for i := range c.Steps {
		idx := int((uint64(i) + selector) % uint64(len(c.Steps)))
		ws := c.Steps[idx].Witnesses
		if len(ws) == 0 {
			continue
		}
		wIdx := int((selector >> 4) % uint64(len(ws)))
		w := &ws[wIdx]
		mode := (selector >> 8) % 3
		if mode == 0 && len(w.ColPerm) >= 2 {
			// Duplicate entry breaks the one-to-one column map.
			w.ColPerm[1] = w.ColPerm[0]
			return true
		}
		if mode == 1 && len(w.ColPerm) >= 2 {
			// Swap two elements to break objective or constraint invariance.
			a := int((selector >> 12) % uint64(len(w.ColPerm)))
			b := (a + 1) % len(w.ColPerm)
			w.ColPerm[a], w.ColPerm[b] = w.ColPerm[b], w.ColPerm[a]
			return true
		}
		if len(w.RowPerm) >= 2 {
			w.RowPerm[1] = w.RowPerm[0]
			return true
		}
	}
	for i := range c.Presolve {
		ps := &c.Presolve[i]
		if len(ps.ColPerm) >= 2 {
			if (selector>>4)%2 == 0 {
				ps.ColPerm[1] = ps.ColPerm[0]
			} else {
				ps.ColPerm[0], ps.ColPerm[len(ps.ColPerm)-1] = ps.ColPerm[len(ps.ColPerm)-1], ps.ColPerm[0]
			}
			return true
		}
	}
	return false
}

func tamperVIPRInflateRat(c *viprCertJSON, selector uint64) bool {
	digits := inflatedDecimalDigits(selector)
	switch (selector >> 4) % 4 {
	case 0:
		if c.TargetLB != nil {
			c.TargetLB.Num = digits
			c.TargetLB.Den = "1"
			return true
		}
		if len(c.Incumbent) > 0 {
			c.Incumbent[0].Num = digits
			c.Incumbent[0].Den = "1"
			return true
		}
	case 1:
		for i := range c.Steps {
			if c.Steps[i].Refs != nil && len(*c.Steps[i].Refs) > 0 {
				(*c.Steps[i].Refs)[0].Mult = viprRatPayload{Num: digits, Den: "1"}
				return true
			}
		}
	case 2:
		if len(c.BaseRows) > 0 {
			c.BaseRows[0].RHS = viprRatPayload{Num: digits, Den: "1"}
			return true
		}
	default:
		if len(c.Obj) > 0 {
			c.Obj[0] = viprRatPayload{Num: digits, Den: "1"}
			return true
		}
	}
	c.ObjConst = viprRatPayload{Num: digits, Den: "1"}
	return true
}

func tamperVIPRPresolveStep(c *viprCertJSON, selector uint64) bool {
	if len(c.Presolve) == 0 {
		return false
	}
	ps := &c.Presolve[int(selector%uint64(len(c.Presolve)))]
	switch (selector >> 4) % 4 {
	case 0:
		if ps.NewBound != nil {
			tamperRatPayload(ps.NewBound, selector>>8)
			return true
		}
		if ps.Val != nil {
			tamperRatPayload(ps.Val, selector>>8)
			return true
		}
	case 1:
		if ps.Col != nil {
			*ps.Col++
			return true
		}
		if ps.Row != nil {
			*ps.Row++
			return true
		}
	case 2:
		if ps.Divisor != nil {
			*ps.Divisor++
			return true
		}
	default:
		ps.Op = "tamperedOp"
		return true
	}
	ps.Op = "tamperedOp"
	return true
}

func tamperVIPRHeaderField(c *viprCertJSON, selector uint64) bool {
	switch selector % 5 {
	case 0:
		if c.TargetLB != nil {
			tamperRatPayload(c.TargetLB, selector>>4)
			return true
		}
	case 1:
		if len(c.Incumbent) > 0 {
			idx := int((selector >> 4) % uint64(len(c.Incumbent)))
			tamperRatPayload(&c.Incumbent[idx], selector>>8)
			return true
		}
	case 2:
		if len(c.Obj) > 0 {
			idx := int((selector >> 4) % uint64(len(c.Obj)))
			tamperRatPayload(&c.Obj[idx], selector>>8)
			return true
		}
	case 3:
		if len(c.BaseRows) > 0 {
			idx := int((selector >> 4) % uint64(len(c.BaseRows)))
			tamperRatPayload(&c.BaseRows[idx].RHS, selector>>8)
			return true
		}
	default:
		tamperRatPayload(&c.ObjConst, selector>>4)
		return true
	}
	tamperRatPayload(&c.ObjConst, selector>>4)
	return true
}

// TamperVIPRTraceForTest returns a deep clone of tr mutated according to kind and
// selector without mutating tr itself.
func TamperVIPRTraceForTest(tr *VIPRTrace, kind int, selector uint64) (*VIPRTrace, bool) {
	if tr == nil || !tr.hasCert {
		return nil, false
	}
	cp := cloneVIPRTrace(tr)
	var ok bool
	switch kind {
	case VIPRTamperMult:
		ok = tamperVIPRMult(&cp.cert, selector)
	case VIPRTamperDropAsm:
		ok = tamperVIPRDropAsm(&cp.cert, selector)
	case VIPRTamperFlipSense:
		ok = tamperVIPRFlipSense(&cp.cert, selector)
	case VIPRTamperOrbitPerm:
		ok = tamperVIPROrbitPerm(&cp.cert, selector)
	case VIPRTamperInflateRat:
		ok = tamperVIPRInflateRat(&cp.cert, selector)
	case VIPRTamperPresolve:
		ok = tamperVIPRPresolveStep(&cp.cert, selector)
	case VIPRTamperHeader:
		ok = tamperVIPRHeaderField(&cp.cert, selector)
	case VIPRTamperWipe:
		cp.hasCert = false
		cp.cert = viprCertJSON{}
		ok = true
	default:
		ok = false
	}
	if !ok {
		return nil, false
	}
	return cp, true
}

// HasSolvedInfeasWitnessForTest reports whether s executed Solve past basis
// factorization or infeasible cut detection rather than holding an unsolved basis.
func (s *LPSolver) HasSolvedInfeasWitnessForTest() bool {
	return s != nil && (s.luValid || s.lastInfeasDir != 0 || s.infeasibleCut)
}

// ExactPolishBasisSlackFallbackForTest runs exact basis polishing on s and falls
// back to an all-slack exact dual-simplex solve when the active basis polish returns false.
func ExactPolishBasisSlackFallbackForTest(ctx context.Context, s *LPSolver) (ExactLPSolution, bool) {
	stop := exactStopFromCtx(ctx)
	if sol, ok := exactPolishBasisSolutionStop(stop, s); ok {
		return sol, true
	}
	snap, okS := buildExactSolverSnapshot(stop, s)
	if !okS {
		return ExactLPSolution{}, false
	}
	snap.resetToAllSlackBasis()
	return snap.solveExactDualSimplex(stop, min(128, max(32, snap.m+16)))
}

// TamperExactOrigForTest clones p and alters its unmerged exactModelOrig metadata
// so VerifyVIPRTrace detects mismatches against original model terms.
func TamperExactOrigForTest(p *CompiledProblem, selector uint64) (*CompiledProblem, bool) {
	if p == nil || (p.numVars == 0 && p.numConstraints == 0) {
		return nil, false
	}
	cp := *p
	var eo exactModelOrig
	if p.exactOrig != nil {
		eo = *p.exactOrig
		eo.objTerms = slices.Clone(p.exactOrig.objTerms)
		eo.rows = slices.Clone(p.exactOrig.rows)
		eo.rowTerms = slices.Clone(p.exactOrig.rowTerms)
	} else {
		eo.objConst = p.objConstant
		eo.rows = make([]exactModelRow, p.numConstraints)
		for i := range p.numConstraints {
			start := len(eo.rowTerms)
			for k := p.rowPtr[i]; k < p.rowPtr[i+1]; k++ {
				eo.rowTerms = append(eo.rowTerms, exactModelTerm{
					col:   p.colIdx[k],
					coeff: p.rowVal[k],
				})
			}
			eo.rows[i] = exactModelRow{
				termStart: start,
				termEnd:   len(eo.rowTerms),
				rhs:       p.rowRHS[i],
			}
		}
	}
	if !eo.objHasLoss && len(eo.objTerms) == 0 && p.numVars > 0 {
		eo.objTerms = make([]exactModelTerm, 0, p.numVars)
		for j := range p.numVars {
			c := p.objMin[j]
			if p.sense == Maximize && c != 0 {
				c = -c
			}
			if c != 0 {
				eo.objTerms = append(eo.objTerms, exactModelTerm{col: j, coeff: c})
			}
		}
	}

	altered := false
	if p.numVars > 0 && (p.numConstraints == 0 || selector%2 == 0) {
		eo.objHasLoss = true
		if len(eo.objTerms) > 0 {
			idx := int((selector >> 4) % uint64(len(eo.objTerms)))
			eo.objTerms[idx].coeff += 1.0
		} else {
			eo.objTerms = append(eo.objTerms, exactModelTerm{col: 0, coeff: 1.0})
		}
		altered = true
	} else if len(eo.rows) > 0 {
		rIdx := int((selector >> 4) % uint64(len(eo.rows)))
		if (selector>>8)%2 == 0 && p.numVars > 0 {
			start := len(eo.rowTerms)
			existing := eo.rowTerms[eo.rows[rIdx].termStart:eo.rows[rIdx].termEnd]
			eo.rowTerms = append(eo.rowTerms, existing...)
			if len(existing) > 0 {
				eo.rowTerms[start].coeff += 1.0
			} else {
				eo.rowTerms = append(eo.rowTerms, exactModelTerm{col: 0, coeff: 1.0})
			}
			eo.rows[rIdx].termStart = start
			eo.rows[rIdx].termEnd = len(eo.rowTerms)
			eo.rows[rIdx].hasDupLoss = true
		} else {
			eo.rows[rIdx].rhs += 1.0
			eo.rows[rIdx].rhsInexact = true
			eo.rows[rIdx].intTightened = false
		}
		altered = true
	}
	if !altered {
		return nil, false
	}
	cp.exactOrig = &eo
	return &cp, true
}
