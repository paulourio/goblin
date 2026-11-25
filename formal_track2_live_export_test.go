package goblin

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"math"
	"math/big"
	"slices"
)

type liveRowMultPayload struct {
	RowIdx int        `json:"rowIdx"`
	Mu     ratPayload `json:"mu"`
}

type liveRhoEntryPayload struct {
	RowIdx int        `json:"rowIdx"`
	Val    ratPayload `json:"val"`
}

type liveCoverItemPayload struct {
	Col     int        `json:"col"`
	Weight  ratPayload `json:"weight"`
	Comp    bool       `json:"comp"`
	InCover bool       `json:"inCover"`
	Alpha   int        `json:"alpha"`
}

type liveLotSizingNodePayload struct {
	ProdCol    int        `json:"prodCol"`
	InvOutCol  int        `json:"invOutCol"`
	BinCol     int        `json:"binCol"`
	Demand     ratPayload `json:"demand"`
	VUBCap     ratPayload `json:"vubCap"`
	VUBRHS     ratPayload `json:"vubRHS"`
	InS        bool       `json:"inS"`
	CoeffY     ratPayload `json:"coeffY"`
	FlowRowIdx *int       `json:"flowRowIdx,omitempty"`
	VUBRowIdx  *int       `json:"vubRowIdx,omitempty"`
}

func pow2BigRat(exp int) *big.Rat {
	if exp >= 0 {
		n := new(big.Int).Lsh(big.NewInt(1), uint(exp))
		return new(big.Rat).SetInt(n)
	}
	d := new(big.Int).Lsh(big.NewInt(1), uint(-exp))
	return new(big.Rat).SetFrac(big.NewInt(1), d)
}

func float64ToBigRatChecked(v float64) (*big.Rat, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil, fmt.Errorf("non-finite float64 %v", v)
	}
	r := new(big.Rat).SetFloat64(v)
	if r == nil {
		return nil, fmt.Errorf("failed to convert float64 %v to *big.Rat", v)
	}
	return r, nil
}

func optBoundPayload(v float64, infSign int) (*ratPayload, error) {
	if math.IsInf(v, infSign) {
		return nil, nil
	}
	p, err := float64ToPayload(v)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func problemBaseMetaPayloads(p *CompiledProblem) ([]bool, []*ratPayload, []*ratPayload, []track2BRowPayload, error) {
	n := p.numVars
	isInt := make([]bool, n)
	lb := make([]*ratPayload, n)
	ub := make([]*ratPayload, n)
	for j := range n {
		isInt[j] = p.varCat[j] != Continuous
		var err error
		lb[j], err = optBoundPayload(p.varLB[j], -1)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("var %d lb: %w", j, err)
		}
		ub[j], err = optBoundPayload(p.varUB[j], 1)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("var %d ub: %w", j, err)
		}
	}
	rows := make([]track2BRowPayload, p.numConstraints)
	for i := range p.numConstraints {
		s, e := p.rowPtr[i], p.rowPtr[i+1]
		terms := make([]track2BTermPayload, 0, e-s)
		for k := s; k < e; k++ {
			vp, err := float64ToPayload(p.rowVal[k])
			if err != nil {
				return nil, nil, nil, nil, fmt.Errorf("row %d term %d: %w", i, k, err)
			}
			terms = append(terms, track2BTermPayload{Col: p.colIdx[k], Val: vp})
		}
		rhsP, err := float64ToPayload(p.rowRHS[i])
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("row %d rhs: %w", i, err)
		}
		rows[i] = track2BRowPayload{
			Terms: terms,
			Sense: senseName(p.rowSense[i]),
			RHS:   rhsP,
		}
	}
	return isInt, lb, ub, rows, nil
}

func sparseTermsPayload(cols []int, vals []float64) ([]track2BTermPayload, error) {
	terms := make([]track2BTermPayload, len(cols))
	for k, col := range cols {
		vp, err := float64ToPayload(vals[k])
		if err != nil {
			return nil, err
		}
		terms[k] = track2BTermPayload{Col: col, Val: vp}
	}
	return terms, nil
}

func cutPlanePayload(c cutPlane) (track2BCutPayload, error) {
	terms, err := sparseTermsPayload(c.ColIdx, c.Val)
	if err != nil {
		return track2BCutPayload{}, err
	}
	rhs, err := float64ToPayload(c.RHS)
	if err != nil {
		return track2BCutPayload{}, err
	}
	return track2BCutPayload{Terms: terms, RHS: rhs}, nil
}

func rawCutPayload(cols []int, vals []float64, rhs float64) (track2BCutPayload, error) {
	terms, err := sparseTermsPayload(cols, vals)
	if err != nil {
		return track2BCutPayload{}, err
	}
	rhsP, err := float64ToPayload(rhs)
	if err != nil {
		return track2BCutPayload{}, err
	}
	return track2BCutPayload{Terms: terms, RHS: rhsP}, nil
}

func ratsToPayloads(vals []*big.Rat) []ratPayload {
	row := make([]ratPayload, len(vals))
	for j, r := range vals {
		row[j] = bigRatToPayload(r)
	}
	return row
}

func feasibleSolutionsPayload(p *CompiledProblem, sols [][]float64) ([][]ratPayload, error) {
	if len(sols) == 0 {
		return nil, nil
	}
	var rp *viprRatProblem
	var scratch *LPSolver
	rpChecked := false
	defer func() {
		if scratch != nil {
			scratch.Close()
		}
	}()
	getScratch := func() *LPSolver {
		if scratch == nil && p != nil {
			scratch = NewLPSolver(p, nil)
		}
		return scratch
	}

	out := make([][]ratPayload, len(sols))
	for i, s := range sols {
		if p != nil && len(s) == p.numVars {
			if xRat, ok := exactPrimalIfFeasible(p, s, true); ok {
				out[i] = ratsToPayloads(xRat)
				continue
			}
			if !rpChecked {
				rpChecked = true
				if built, okRP := buildVIPRRatProblem(p); okRP {
					rp = built
				}
			}
			if rp != nil {
				if xRat, _, okRef := rp.refineExactIncumbent(s, getScratch); okRef && len(xRat) == p.numVars {
					out[i] = ratsToPayloads(xRat)
					continue
				}
			}
		}
		row := make([]ratPayload, len(s))
		for j, v := range s {
			rpVal, err := float64ToPayload(v)
			if err != nil {
				return nil, err
			}
			row[j] = rpVal
		}
		out[i] = row
	}
	return out, nil
}

// cutMatchesScaledRaw reports whether cut matches exact 2^scaleExp * raw across all columns.
func cutMatchesScaledRaw(n int, rawCols []int, rawVals []float64, scaleExp int, cut cutPlane) bool {
	scale := pow2BigRat(scaleExp)
	rawDense := make([]*big.Rat, n)
	cutDense := make([]*big.Rat, n)
	for j := range n {
		rawDense[j] = new(big.Rat)
		cutDense[j] = new(big.Rat)
	}
	for k, col := range rawCols {
		if col < 0 || col >= n {
			return false
		}
		r, err := float64ToBigRatChecked(rawVals[k])
		if err != nil {
			return false
		}
		rawDense[col].Add(rawDense[col], r)
	}
	for k, col := range cut.ColIdx {
		if col < 0 || col >= n {
			return false
		}
		r, err := float64ToBigRatChecked(cut.Val[k])
		if err != nil {
			return false
		}
		cutDense[col].Add(cutDense[col], r)
	}
	scaled := new(big.Rat)
	for j := range n {
		scaled.Mul(scale, rawDense[j])
		if cutDense[j].Cmp(scaled) != 0 {
			return false
		}
	}
	return true
}

func bigRatFloor(r *big.Rat) *big.Int {
	q := new(big.Int)
	rem := new(big.Int)
	q.QuoRem(r.Num(), r.Denom(), rem)
	if rem.Sign() < 0 {
		q.Sub(q, big.NewInt(1))
	}
	return q
}

func bigRatFrac(r *big.Rat) *big.Rat {
	fl := new(big.Rat).SetInt(bigRatFloor(r))
	return new(big.Rat).Sub(r, fl)
}

func exactGMIIntRat(a, f0 *big.Rat) *big.Rat {
	f := bigRatFrac(a)
	if f.Cmp(f0) <= 0 {
		return new(big.Rat).Quo(f, f0)
	}
	one := big.NewRat(1, 1)
	num := new(big.Rat).Sub(one, f)
	den := new(big.Rat).Sub(one, f0)
	return new(big.Rat).Quo(num, den)
}

func exactGMIContRat(a, f0 *big.Rat) *big.Rat {
	if a.Sign() >= 0 {
		return new(big.Rat).Quo(a, f0)
	}
	one := big.NewRat(1, 1)
	neg := new(big.Rat).Neg(a)
	den := new(big.Rat).Sub(one, f0)
	return new(big.Rat).Quo(neg, den)
}

// solveExactBasisTransposeRow solves B^T * rho = e_rowIdx in exact rational arithmetic.
func solveExactBasisTransposeRow(p *CompiledProblem, basis []int, rowIdx int) ([]*big.Rat, bool) {
	m := p.numConstraints
	n := p.numVars
	if len(basis) != m || rowIdx < 0 || rowIdx >= m {
		return nil, false
	}
	aug := make([][]*big.Rat, m)
	for i := range m {
		aug[i] = make([]*big.Rat, m+1)
		for j := range m + 1 {
			aug[i][j] = new(big.Rat)
		}
	}
	for colPos, bCol := range basis {
		if bCol < n {
			for k := p.colPtr[bCol]; k < p.colPtr[bCol+1]; k++ {
				r, err := float64ToBigRatChecked(p.colVal[k])
				if err != nil {
					return nil, false
				}
				aug[colPos][p.rowIdx[k]].Add(aug[colPos][p.rowIdx[k]], r)
			}
		} else {
			sRow := bCol - n
			if sRow < 0 || sRow >= m {
				return nil, false
			}
			aug[colPos][sRow].SetInt64(1)
		}
	}
	aug[rowIdx][m].SetInt64(1)

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
				if aug[col][j].Sign() == 0 {
					continue
				}
				prod := new(big.Rat).Mul(fac, aug[col][j])
				aug[r][j].Sub(aug[r][j], prod)
			}
		}
	}
	rho := make([]*big.Rat, m)
	for i := range m {
		rho[i] = new(big.Rat).Set(aug[i][m])
	}
	return rho, true
}

// compactBaseRows returns the deduplicated subset of baseRows referenced by usedIdxs
// in ascending index order together with a map from original row indices to compacted indices.
func compactBaseRows(baseRows []track2BRowPayload, usedIdxs []int) ([]track2BRowPayload, map[int]int) {
	seen := make(map[int]struct{}, len(usedIdxs))
	sorted := make([]int, 0, len(usedIdxs))
	for _, idx := range usedIdxs {
		if idx < 0 || idx >= len(baseRows) {
			continue
		}
		if _, ok := seen[idx]; ok {
			continue
		}
		seen[idx] = struct{}{}
		sorted = append(sorted, idx)
	}
	slices.Sort(sorted)
	compact := make([]track2BRowPayload, len(sorted))
	remap := make(map[int]int, len(sorted))
	for newIdx, oldIdx := range sorted {
		compact[newIdx] = baseRows[oldIdx]
		remap[oldIdx] = newIdx
	}
	return compact, remap
}

// exportObserverItems converts captured production cut records into canonical cuts-check JSONL items.
func exportObserverItems(p *CompiledProblem, obs *cutCertObserver, feasibleSols [][]float64) ([][]byte, error) {
	isInt, lb, ub, baseRows, err := problemBaseMetaPayloads(p)
	if err != nil {
		return nil, err
	}
	solsPayload, err := feasibleSolutionsPayload(p, feasibleSols)
	if err != nil {
		return nil, err
	}

	n := p.numVars
	m := p.numConstraints
	var items [][]byte

	// 1. Structural GMI certificates (universal derivation over baseRows via exact B^T * rho = e_r).
	for _, rec := range obs.gmis {
		if rec.numRows != m {
			continue
		}
		rho, ok := solveExactBasisTransposeRow(p, rec.basis, rec.basicRowIdx)
		if !ok {
			continue
		}
		alpha := make([]*big.Rat, n)
		for j := range n {
			alpha[j] = new(big.Rat)
		}
		beta0 := new(big.Rat)
		for i := range m {
			if rho[i].Sign() == 0 {
				continue
			}
			rhsR, err := float64ToBigRatChecked(p.rowRHS[i])
			if err != nil {
				return nil, err
			}
			beta0.Add(beta0, new(big.Rat).Mul(rho[i], rhsR))
			for k := p.rowPtr[i]; k < p.rowPtr[i+1]; k++ {
				vR, err := float64ToBigRatChecked(p.rowVal[k])
				if err != nil {
					return nil, err
				}
				alpha[p.colIdx[k]].Add(alpha[p.colIdx[k]], new(big.Rat).Mul(rho[i], vR))
			}
		}
		beta := new(big.Rat).Set(beta0)
		bndRats := make([]*big.Rat, n)
		bndOK := true
		for j := range n {
			if j == rec.basicCol || alpha[j].Sign() == 0 {
				continue
			}
			bnd := p.varLB[j]
			if rec.colAtUpper[j] {
				bnd = p.varUB[j]
			}
			bR, err := float64ToBigRatChecked(bnd)
			if err != nil {
				bndOK = false
				break
			}
			bndRats[j] = bR
			beta.Sub(beta, new(big.Rat).Mul(alpha[j], bR))
		}
		if !bndOK {
			continue
		}
		f0 := bigRatFrac(beta)
		if f0.Sign() <= 0 || f0.Cmp(big.NewRat(1, 1)) >= 0 {
			continue
		}

		cStruct := make([]ratPayload, n)
		for j := range n {
			if j == rec.basicCol || alpha[j].Sign() == 0 {
				cStruct[j] = int64ToPayload(0)
				continue
			}
			// Fixed structural columns (lb == ub) are skipped by separateGMICutsEx and cancel in dropMinTerm.
			if p.varLB[j] == p.varUB[j] && rec.cStruct[j] == 0 {
				aTilde := new(big.Rat).Set(alpha[j])
				if rec.colAtUpper[j] {
					aTilde.Neg(aTilde)
				}
				var exactC *big.Rat
				if isInt[j] && bndRats[j].IsInt() {
					exactC = exactGMIIntRat(aTilde, f0)
				} else {
					exactC = exactGMIContRat(aTilde, f0)
				}
				cStruct[j] = bigRatToPayload(exactC)
				continue
			}
			cp, err := float64ToPayload(rec.cStruct[j])
			if err != nil {
				return nil, err
			}
			cStruct[j] = cp
		}

		var rhoEntries []liveRhoEntryPayload
		var cSlack []ratPayload
		for i := range m {
			if rho[i].Sign() == 0 && rec.cSlack[i] == 0 {
				continue
			}
			rhoEntries = append(rhoEntries, liveRhoEntryPayload{
				RowIdx: i,
				Val:    bigRatToPayload(rho[i]),
			})
			sp, err := float64ToPayload(rec.cSlack[i])
			if err != nil {
				return nil, err
			}
			cSlack = append(cSlack, sp)
		}
		cutP, err := cutPlanePayload(rec.cut)
		if err != nil {
			return nil, err
		}
		usedIdxs := make([]int, len(rhoEntries))
		for idx, entry := range rhoEntries {
			usedIdxs[idx] = entry.RowIdx
		}
		subRows, rowRemap := compactBaseRows(baseRows, usedIdxs)
		for idx := range rhoEntries {
			rhoEntries[idx].RowIdx = rowRemap[rhoEntries[idx].RowIdx]
		}
		obj := map[string]any{
			"op":         "gmi-check",
			"numVars":    n,
			"isInt":      isInt,
			"lb":         lb,
			"ub":         ub,
			"baseRows":   subRows,
			"basicCol":   rec.basicCol,
			"rho":        rhoEntries,
			"colAtUpper": rec.colAtUpper,
			"cStruct":    cStruct,
			"cSlack":     cSlack,
			"scale":      bigRatToPayload(pow2BigRat(rec.scaleExp)),
			"cut":        cutP,
		}
		if len(solsPayload) > 0 {
			obj["solutions"] = solsPayload
		}
		raw, err := json.Marshal(obj)
		if err != nil {
			return nil, err
		}
		items = append(items, raw)
	}

	// 2. Knapsack cover certificates (universal derivation over baseRows + binary domains).
	for _, rec := range obs.covers {
		rowTerms, err := sparseTermsPayload(rec.rowCols, rec.rowVals)
		if err != nil {
			return nil, err
		}
		rowRHS, err := float64ToPayload(rec.rowRHS)
		if err != nil {
			return nil, err
		}
		signP, err := float64ToPayload(rec.sign)
		if err != nil {
			return nil, err
		}
		knapRHS, err := float64ToPayload(rec.knapRHS)
		if err != nil {
			return nil, err
		}
		covItems := make([]liveCoverItemPayload, len(rec.items))
		for idx, it := range rec.items {
			wp, err := float64ToPayload(it.weight)
			if err != nil {
				return nil, err
			}
			covItems[idx] = liveCoverItemPayload{
				Col:     it.col,
				Weight:  wp,
				Comp:    it.comp,
				InCover: it.inCover,
				Alpha:   it.alpha,
			}
		}
		var scaleP ratPayload
		var cutP track2BCutPayload
		if cutMatchesScaledRaw(n, rec.rawCols, rec.rawVals, rec.scaleExp, rec.cut) {
			scaleP = bigRatToPayload(pow2BigRat(rec.scaleExp))
			cutP, err = cutPlanePayload(rec.cut)
		} else {
			scaleP = int64ToPayload(1)
			cutP, err = rawCutPayload(rec.rawCols, rec.rawVals, rec.rawRHS)
		}
		if err != nil {
			return nil, err
		}
		var usedIdxs []int
		if len(rec.aggMults) > 0 {
			usedIdxs = make([]int, len(rec.aggMults))
			for idx, rm := range rec.aggMults {
				usedIdxs[idx] = rm.rowIdx
			}
		} else if rec.rowIdx >= 0 {
			usedIdxs = []int{rec.rowIdx}
		}
		subRows, rowRemap := compactBaseRows(baseRows, usedIdxs)
		obj := map[string]any{
			"op":       "cover-check",
			"numVars":  n,
			"isInt":    isInt,
			"lb":       lb,
			"ub":       ub,
			"baseRows": subRows,
			"row": track2BRowPayload{
				Terms: rowTerms,
				Sense: senseName(rec.rowSense),
				RHS:   rowRHS,
			},
			"sign":    signP,
			"items":   covItems,
			"knapRHS": knapRHS,
			"scale":   scaleP,
			"cut":     cutP,
		}
		if len(rec.aggMults) > 0 {
			mults := make([]liveRowMultPayload, len(rec.aggMults))
			for idx, rm := range rec.aggMults {
				mp, err := float64ToPayload(rm.mu)
				if err != nil {
					return nil, err
				}
				mults[idx] = liveRowMultPayload{RowIdx: rowRemap[rm.rowIdx], Mu: mp}
			}
			obj["aggMultipliers"] = mults
		} else if rec.rowIdx >= 0 {
			obj["rowIdx"] = rowRemap[rec.rowIdx]
		}
		if len(solsPayload) > 0 {
			obj["solutions"] = solsPayload
		}
		raw, err := json.Marshal(obj)
		if err != nil {
			return nil, err
		}
		items = append(items, raw)
	}

	// 3. Structural disaggregated VUB certificates (universal derivation over baseRows[rowIdx]).
	for _, rec := range obs.vubs {
		effUB, err := float64ToPayload(rec.effUB)
		if err != nil {
			return nil, err
		}
		cutP, err := cutPlanePayload(rec.cut)
		if err != nil {
			return nil, err
		}
		subRows, rowRemap := compactBaseRows(baseRows, []int{rec.rowIdx})
		obj := map[string]any{
			"op":        "vub-check",
			"numVars":   n,
			"isInt":     isInt,
			"lb":        lb,
			"ub":        ub,
			"baseRows":  subRows,
			"rowIdx":    rowRemap[rec.rowIdx],
			"binCol":    rec.binCol,
			"targetCol": rec.targetCol,
			"effUB":     effUB,
			"scale":     bigRatToPayload(pow2BigRat(rec.scaleExp)),
			"cut":       cutP,
		}
		if len(solsPayload) > 0 {
			obj["solutions"] = solsPayload
		}
		raw, err := json.Marshal(obj)
		if err != nil {
			return nil, err
		}
		items = append(items, raw)
	}

	// 4. Lot-sizing (l, S) certificates (universal derivation over baseRows + column distinctness).
	for _, rec := range obs.lotSizings {
		usedIdxs := make([]int, 0, 2*len(rec.nodes))
		for _, nd := range rec.nodes {
			usedIdxs = append(usedIdxs, nd.flowRowIdx, nd.vubRowIdx)
		}
		subRows, rowRemap := compactBaseRows(baseRows, usedIdxs)
		nodes := make([]liveLotSizingNodePayload, len(rec.nodes))
		for idx, nd := range rec.nodes {
			demP, err := float64ToPayload(nd.demand)
			if err != nil {
				return nil, err
			}
			capP, err := float64ToPayload(nd.vubCap)
			if err != nil {
				return nil, err
			}
			rhsP, err := float64ToPayload(nd.vubRHS)
			if err != nil {
				return nil, err
			}
			cYP, err := float64ToPayload(nd.coeffY)
			if err != nil {
				return nil, err
			}
			flIdx := nd.flowRowIdx
			if mapped, ok := rowRemap[flIdx]; ok {
				flIdx = mapped
			}
			vbIdx := nd.vubRowIdx
			if mapped, ok := rowRemap[vbIdx]; ok {
				vbIdx = mapped
			}
			nodes[idx] = liveLotSizingNodePayload{
				ProdCol:    nd.prodCol,
				InvOutCol:  nd.invOutCol,
				BinCol:     nd.binCol,
				Demand:     demP,
				VUBCap:     capP,
				VUBRHS:     rhsP,
				InS:        nd.inS,
				CoeffY:     cYP,
				FlowRowIdx: &flIdx,
				VUBRowIdx:  &vbIdx,
			}
		}
		cutDemP, err := float64ToPayload(rec.cutDem)
		if err != nil {
			return nil, err
		}
		var scaleP ratPayload
		var cutP track2BCutPayload
		if cutMatchesScaledRaw(n, rec.rawCols, rec.rawVals, rec.scaleExp, rec.cut) {
			scaleP = bigRatToPayload(pow2BigRat(rec.scaleExp))
			cutP, err = cutPlanePayload(rec.cut)
		} else {
			scaleP = int64ToPayload(1)
			cutP, err = rawCutPayload(rec.rawCols, rec.rawVals, rec.rawRHS)
		}
		if err != nil {
			return nil, err
		}
		var invIn any
		if rec.invInCol >= 0 {
			invIn = rec.invInCol
		}
		obj := map[string]any{
			"op":       "lotsizing-check",
			"numVars":  n,
			"isInt":    isInt,
			"lb":       lb,
			"ub":       ub,
			"baseRows": subRows,
			"invInCol": invIn,
			"nodes":    nodes,
			"cutDem":   cutDemP,
			"scale":    scaleP,
			"cut":      cutP,
		}
		if len(solsPayload) > 0 {
			obj["solutions"] = solsPayload
		}
		raw, err := json.Marshal(obj)
		if err != nil {
			return nil, err
		}
		items = append(items, raw)
	}

	// 5. Single-row and multi-row aggregated cMIR certificates (universal derivation via agg-cmir-check).
	for _, rec := range obs.cmirs {
		aggRows := make([]track2BAggRowPayload, len(rec.rows))
		for idx, rm := range rec.rows {
			br := baseRows[rm.rowIdx]
			mp, err := float64ToPayload(rm.mu)
			if err != nil {
				return nil, err
			}
			aggRows[idx] = track2BAggRowPayload{
				Terms: br.Terms,
				Sense: br.Sense,
				RHS:   br.RHS,
				Mu:    mp,
			}
		}
		aggTerms, err := sparseTermsPayload(rec.aggCols, rec.aggVals)
		if err != nil {
			return nil, err
		}
		aggRHSP, err := float64ToPayload(rec.aggRHS)
		if err != nil {
			return nil, err
		}
		deltaP, err := float64ToPayload(rec.delta)
		if err != nil {
			return nil, err
		}
		b0R, err := float64ToBigRatChecked(rec.betaB0)
		if err != nil {
			return nil, err
		}
		f0R, err := float64ToBigRatChecked(rec.betaF0)
		if err != nil {
			return nil, err
		}
		betaCert := new(big.Rat).Add(b0R, f0R)

		alphaHat := make([]ratPayload, n)
		cInt := make([]ratPayload, n)
		cCont := make([]ratPayload, n)
		for j := range n {
			alphaHat[j], err = float64ToPayload(rec.alphaHat[j])
			if err != nil {
				return nil, err
			}
			cInt[j], err = float64ToPayload(rec.cInt[j])
			if err != nil {
				return nil, err
			}
			cCont[j], err = float64ToPayload(rec.cCont[j])
			if err != nil {
				return nil, err
			}
		}
		var scaleP ratPayload
		var cutP track2BCutPayload
		if cutMatchesScaledRaw(n, rec.rawCols, rec.rawVals, rec.scaleExp, rec.cut) {
			scaleP = bigRatToPayload(pow2BigRat(rec.scaleExp))
			cutP, err = cutPlanePayload(rec.cut)
		} else {
			scaleP = int64ToPayload(1)
			cutP, err = rawCutPayload(rec.rawCols, rec.rawVals, rec.rawRHS)
		}
		if err != nil {
			return nil, err
		}
		obj := map[string]any{
			"op":       "agg-cmir-check",
			"numVars":  n,
			"isInt":    isInt,
			"lb":       lb,
			"ub":       ub,
			"rows":     aggRows,
			"aggTerms": aggTerms,
			"aggRHS":   aggRHSP,
			"delta":    deltaP,
			"useLower": rec.useLower,
			"betaHat":  bigRatToPayload(betaCert),
			"alphaHat": alphaHat,
			"cInt":     cInt,
			"cCont":    cCont,
			"scale":    scaleP,
			"cut":      cutP,
		}
		if len(solsPayload) > 0 {
			obj["solutions"] = solsPayload
		}
		raw, err := json.Marshal(obj)
		if err != nil {
			return nil, err
		}
		items = append(items, raw)
	}

	// 6. Universal finalizeSparseCut drop and scaling transition certificates (drop-check).
	for _, rec := range obs.drops {
		rawP, err := rawCutPayload(rec.rawCols, rec.rawVals, rec.rawRHS)
		if err != nil {
			return nil, err
		}
		finP, err := cutPlanePayload(rec.finalCut)
		if err != nil {
			return nil, err
		}
		obj := map[string]any{
			"op":       "drop-check",
			"numVars":  n,
			"lb":       lb,
			"ub":       ub,
			"rawCut":   rawP,
			"scale":    bigRatToPayload(pow2BigRat(rec.scaleExp)),
			"finalCut": finP,
		}
		if len(solsPayload) > 0 {
			obj["solutions"] = solsPayload
		}
		raw, err := json.Marshal(obj)
		if err != nil {
			return nil, err
		}
		items = append(items, raw)
	}

	return items, nil
}

// ExportLiveSeparatedCutsItems runs production root cut separators on p with an attached
// witness observer and exports canonical cuts-check JSONL items for every separated cut.
func ExportLiveSeparatedCutsItems(p *CompiledProblem, feasibleSols [][]float64) ([][]byte, error) {
	if p == nil {
		return nil, errors.New("nil CompiledProblem")
	}
	lp := NewLPSolver(p, nil)
	defer lp.Close()
	root := lp.Solve()
	if root.Status != StatusOptimal {
		return nil, nil
	}
	obs := &cutCertObserver{srcRowIdx: -1}
	ws := &cutGenWorkspace{certObs: obs}

	_ = separateGMICutsEx(lp, root.Primal, ws, false)
	_ = separateKnapsackCoverCuts(p, root.Primal, ws)
	_ = separateFlowAndVUBCuts(p, root.Primal, ws)
	_ = separateRowMIRCuts(p, root.Primal, ws)
	_ = separateAggregatedMIRCuts(lp, root.Primal, ws)

	return exportObserverItems(p, obs, feasibleSols)
}
