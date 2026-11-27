package goblin

import (
	"context"
	"math"
	"slices"
	"time"

	"github.com/paulourio/goblin/internal/mad"
)

type detectedMAD struct {
	prob       mad.Problem
	numGroups  int
	numItems   int
	numPeriods int
	directStep bool
	rounded    bool
	xCols      [][]int // [g][i] variable index in CompiledProblem
	zCols      [][]int // [g][t] variable index in CompiledProblem
	mCols      []int   // [g] variable index, or -1 in direct-step mode
	nCols      []int   // [g] variable index, or -1 in unrounded mode
	rCols      []int   // [g] variable index, or -1 in unrounded or direct-step mode
}

type madRoleCounts struct {
	numBin       int
	numInt       int
	numLevel     int
	numObj1Slack int
	numOthSlack  int
	numGroups    int
	numItems     int
	remRows      int
}

// fastScreenMAD performs a zero-allocation structural pre-screen on p.
// Non-MAD LP/MILP models reject on the first incompatible variable or row count.
func fastScreenMAD(p *CompiledProblem) (madRoleCounts, bool) {
	if p == nil || !p.isMIP || p.sense != Minimize || p.numVars == 0 || p.numConstraints == 0 {
		return madRoleCounts{}, false
	}
	var c madRoleCounts
	for j := range p.numVars {
		cat := p.varCat[j]
		lb := p.varLB[j]
		ub := p.varUB[j]
		obj := p.objMin[j]
		deg := p.colPtr[j+1] - p.colPtr[j]
		switch cat {
		case Binary:
			if lb != 0 || ub != 1 || obj != 0 {
				return madRoleCounts{}, false
			}
			c.numBin++
		case Integer:
			if lb < 0 || math.IsInf(lb, 0) || lb != math.Trunc(lb) || ub < lb || obj != 0 || deg < 2 {
				return madRoleCounts{}, false
			}
			c.numInt++
		case Continuous:
			if obj > 0 {
				if lb != 0 || !math.IsInf(ub, 1) || deg != 2 {
					return madRoleCounts{}, false
				}
				if obj == 1.0 {
					c.numObj1Slack++
				} else {
					c.numOthSlack++
				}
			} else if obj == 0 {
				if lb != 0 || ub < 0 || deg < 2 {
					return madRoleCounts{}, false
				}
				c.numLevel++
			} else {
				return madRoleCounts{}, false
			}
		default:
			return madRoleCounts{}, false
		}
	}

	if c.numObj1Slack == 0 {
		return madRoleCounts{}, false
	}
	if c.numLevel > 0 {
		c.numGroups = c.numLevel
		if c.numInt > 0 && c.numInt != c.numGroups {
			return madRoleCounts{}, false
		}
		if c.numInt == 0 && c.numOthSlack > 0 {
			return madRoleCounts{}, false
		}
	} else {
		// Direct-step formulation: m_g substituted by Step*n_g, no r_g slacks.
		if c.numInt == 0 || c.numOthSlack > 0 {
			return madRoleCounts{}, false
		}
		c.numGroups = c.numInt
	}

	nSlacks := c.numObj1Slack + c.numOthSlack
	if c.numLevel > 0 && c.numInt > 0 {
		if nSlacks <= c.numGroups {
			return madRoleCounts{}, false
		}
		if (nSlacks-c.numGroups)%c.numGroups != 0 {
			return madRoleCounts{}, false
		}
	} else if nSlacks%c.numGroups != 0 {
		return madRoleCounts{}, false
	}

	if 2*nSlacks > p.numConstraints {
		return madRoleCounts{}, false
	}
	c.remRows = p.numConstraints - 2*nSlacks
	if c.numBin == 0 {
		if c.remRows != 0 {
			return madRoleCounts{}, false
		}
		return c, true
	}
	if c.numBin%c.numGroups != 0 {
		return madRoleCounts{}, false
	}
	c.numItems = c.numBin / c.numGroups
	if c.remRows != c.numItems && (c.numGroups > c.numItems || c.remRows != c.numItems+c.numGroups) {
		return madRoleCounts{}, false
	}
	return c, true
}

type madL1Pair struct {
	zCol    int
	a       float64
	binCols []int
	binVals []float64
}

func normGeRowSign(s ConstraintSense) (float64, bool) {
	switch s {
	case GreaterEqual:
		return 1.0, true
	case LessEqual:
		return -1.0, true
	default:
		return 0, false
	}
}

func cleanZeroFloat(v float64) float64 {
	if v == 0 {
		return 0
	}
	return v
}

// matchEpigraphPair verifies that rows rA and rB form an exact opposite-sign
// epigraph pair for slack column sCol:
//
//	sCol + f(rest) >= rhs   and   sCol - f(rest) >= -rhs
//
// returning signA so normalized coefficients of rA are signA*rowVal[k] and signA*rowRHS[rA].
func matchEpigraphPair(p *CompiledProblem, sCol, rA, rB int, scratchMark []int, scratchVal []float64, stamp int) (float64, bool) {
	if rA == rB {
		return 0, false
	}
	signA, okA := normGeRowSign(p.rowSense[rA])
	signB, okB := normGeRowSign(p.rowSense[rB])
	if !okA || !okB {
		return 0, false
	}
	sA, eA := p.rowPtr[rA], p.rowPtr[rA+1]
	sB, eB := p.rowPtr[rB], p.rowPtr[rB+1]
	if eA-sA != eB-sB || eA-sA < 2 {
		return 0, false
	}
	rhsA := signA * p.rowRHS[rA]
	rhsB := signB * p.rowRHS[rB]
	if rhsB != -rhsA {
		return 0, false
	}

	// Fast path: identical column ordering in both rows.
	sameOrder := true
	seenSlack := false
	for off := range eA - sA {
		cA := p.colIdx[sA+off]
		cB := p.colIdx[sB+off]
		if cA != cB {
			sameOrder = false
			break
		}
		vA := signA * p.rowVal[sA+off]
		vB := signB * p.rowVal[sB+off]
		if cA == sCol {
			if vA != 1.0 || vB != 1.0 {
				return 0, false
			}
			seenSlack = true
		} else if vB != -vA {
			return 0, false
		}
	}
	if sameOrder {
		return signA, seenSlack
	}

	// Order-independent fallback using reusable stamp buffer.
	seenSlackA := false
	for k := sA; k < eA; k++ {
		col := p.colIdx[k]
		val := signA * p.rowVal[k]
		if col == sCol {
			if val != 1.0 {
				return 0, false
			}
			seenSlackA = true
		}
		scratchMark[col] = stamp
		scratchVal[col] = val
	}
	if !seenSlackA {
		return 0, false
	}
	seenSlackB := false
	for k := sB; k < eB; k++ {
		col := p.colIdx[k]
		if scratchMark[col] != stamp {
			return 0, false
		}
		val := signB * p.rowVal[k]
		if col == sCol {
			if val != 1.0 {
				return 0, false
			}
			seenSlackB = true
		} else if val != -scratchVal[col] {
			return 0, false
		}
	}
	return signA, seenSlackB
}

// detectMADStructure extracts an exact separable L1/MAD problem from p with
// 100% variable, bound, row, and objective closure. Returns ok=false on any
// unrecognized constraint, variable role, or parameter mismatch.
func detectMADStructure(p *CompiledProblem) (detectedMAD, bool) {
	c, ok := fastScreenMAD(p)
	if !ok {
		return detectedMAD{}, false
	}

	numGroups := c.numGroups
	numItems := c.numItems
	directStep := c.numLevel == 0
	hasRounding := c.numInt > 0

	levelGroupOf := make([]int, p.numVars)
	for j := range levelGroupOf {
		levelGroupOf[j] = -1
	}

	mCols := make([]int, numGroups)
	nCols := make([]int, numGroups)
	rCols := make([]int, numGroups)
	for g := range numGroups {
		mCols[g] = -1
		nCols[g] = -1
		rCols[g] = -1
	}

	gIdx := 0
	for j := range p.numVars {
		if directStep {
			if p.varCat[j] == Integer {
				nCols[gIdx] = j
				levelGroupOf[j] = gIdx
				gIdx++
			}
		} else if p.varCat[j] == Continuous && p.objMin[j] == 0 {
			mCols[gIdx] = j
			levelGroupOf[j] = gIdx
			gIdx++
		}
	}
	if gIdx != numGroups {
		return detectedMAD{}, false
	}

	rowConsumed := make([]bool, p.numConstraints)
	scratchMark := make([]int, p.numVars)
	scratchVal := make([]float64, p.numVars)
	stamp := 0

	xGroupOwner := make([]int, p.numVars)
	for j := range xGroupOwner {
		xGroupOwner[j] = -1
	}
	nUsed := make([]bool, p.numVars)

	groupPairs := make([][]madL1Pair, numGroups)
	var step float64
	var stepSet bool
	var roundWeight float64
	var roundWeightSet bool
	var allowZeroStep bool
	var nMinSet bool
	roundPairsCount := 0

	for sCol := range p.numVars {
		if p.varCat[sCol] != Continuous || p.objMin[sCol] <= 0 {
			continue
		}
		cs := p.colPtr[sCol]
		rA := p.rowIdx[cs]
		rB := p.rowIdx[cs+1]
		if rowConsumed[rA] || rowConsumed[rB] {
			return detectedMAD{}, false
		}
		stamp++
		signA, pairOK := matchEpigraphPair(p, sCol, rA, rB, scratchMark, scratchVal, stamp)
		if !pairOK {
			return detectedMAD{}, false
		}
		rowConsumed[rA] = true
		rowConsumed[rB] = true

		sA, eA := p.rowPtr[rA], p.rowPtr[rA+1]
		rhsA := signA * p.rowRHS[rA]

		// Check whether this epigraph pair is a roundness penalty r_g >= |m_g - B*n_g|.
		if !directStep && hasRounding && eA-sA == 3 && rhsA == 0 {
			mCol, nCol := -1, -1
			var cM, cN float64
			for k := sA; k < eA; k++ {
				col := p.colIdx[k]
				if col == sCol {
					continue
				}
				val := signA * p.rowVal[k]
				if p.varCat[col] == Continuous && p.objMin[col] == 0 {
					mCol = col
					cM = val
				} else if p.varCat[col] == Integer {
					nCol = col
					cN = val
				}
			}
			if mCol >= 0 && nCol >= 0 {
				if (cM != 1.0 && cM != -1.0) || cM*cN >= 0 {
					return detectedMAD{}, false
				}
				g := levelGroupOf[mCol]
				if g < 0 || nCols[g] >= 0 || nUsed[nCol] {
					return detectedMAD{}, false
				}
				bVal := math.Abs(cN)
				if bVal <= 0 || math.IsNaN(bVal) || math.IsInf(bVal, 0) {
					return detectedMAD{}, false
				}
				if stepSet {
					if bVal != step {
						return detectedMAD{}, false
					}
				} else {
					step = bVal
					stepSet = true
				}
				wr := p.objMin[sCol]
				if roundWeightSet {
					if wr != roundWeight {
						return detectedMAD{}, false
					}
				} else {
					roundWeight = wr
					roundWeightSet = true
				}
				if p.varLB[nCol] == 0 {
					allowZeroStep = true
				}
				nMinSet = true
				nCols[g] = nCol
				rCols[g] = sCol
				nUsed[nCol] = true
				roundPairsCount++
				continue
			}
		}

		// Otherwise this epigraph pair must be an L1 deviation z[g][t] with obj == 1.0.
		if p.objMin[sCol] != 1.0 {
			return detectedMAD{}, false
		}
		levelCol := -1
		var cLevel float64
		nBinsInRow := 0
		for k := sA; k < eA; k++ {
			col := p.colIdx[k]
			if col == sCol {
				continue
			}
			cat := p.varCat[col]
			if cat == Binary {
				nBinsInRow++
				continue
			}
			if levelCol >= 0 {
				return detectedMAD{}, false
			}
			if directStep {
				if cat != Integer {
					return detectedMAD{}, false
				}
			} else if cat != Continuous || p.objMin[col] != 0 {
				return detectedMAD{}, false
			}
			levelCol = col
			cLevel = signA * p.rowVal[k]
		}
		if levelCol < 0 {
			return detectedMAD{}, false
		}
		g := levelGroupOf[levelCol]
		if g < 0 {
			return detectedMAD{}, false
		}

		var flip float64
		if directStep {
			if cLevel == 0 {
				return detectedMAD{}, false
			}
			bVal := math.Abs(cLevel)
			if stepSet {
				if bVal != step {
					return detectedMAD{}, false
				}
			} else {
				step = bVal
				stepSet = true
			}
			if p.varLB[levelCol] == 0 {
				allowZeroStep = true
			}
			nMinSet = true
			if cLevel > 0 {
				flip = 1.0
			} else {
				flip = -1.0
			}
		} else {
			if cLevel != 1.0 && cLevel != -1.0 {
				return detectedMAD{}, false
			}
			flip = cLevel
		}

		pair := madL1Pair{
			zCol: sCol,
			a:    cleanZeroFloat(flip * rhsA),
		}
		if nBinsInRow > 0 {
			pair.binCols = make([]int, 0, nBinsInRow)
			pair.binVals = make([]float64, 0, nBinsInRow)
			for k := sA; k < eA; k++ {
				col := p.colIdx[k]
				if p.varCat[col] != Binary {
					continue
				}
				if prev := xGroupOwner[col]; prev >= 0 && prev != g {
					return detectedMAD{}, false
				}
				xGroupOwner[col] = g
				dVal := cleanZeroFloat(-(flip * signA * p.rowVal[k]))
				pair.binCols = append(pair.binCols, col)
				pair.binVals = append(pair.binVals, dVal)
			}
		}
		groupPairs[g] = append(groupPairs[g], pair)
	}

	if !directStep && hasRounding && roundPairsCount != numGroups {
		return detectedMAD{}, false
	}

	numPeriods := len(groupPairs[0])
	if numPeriods == 0 {
		return detectedMAD{}, false
	}
	for g := 1; g < numGroups; g++ {
		if len(groupPairs[g]) != numPeriods {
			return detectedMAD{}, false
		}
	}
	if directStep {
		if !stepSet || !nMinSet {
			return detectedMAD{}, false
		}
		roundWeight = float64(numPeriods)
	}

	// Parse remaining equality rows: item partitions and optional designated-item rows.
	xCols, designated, eqOK := parseMADEqualityRows(p, rowConsumed, xGroupOwner, numGroups, numItems, c.remRows)
	if !eqOK {
		return detectedMAD{}, false
	}

	// Map each binary column to its item index i.
	xItemOf := make([]int, p.numVars)
	for j := range xItemOf {
		xItemOf[j] = -1
	}
	for g := range numGroups {
		for i := range numItems {
			col := xCols[g][i]
			if xItemOf[col] >= 0 {
				return detectedMAD{}, false
			}
			xItemOf[col] = i
		}
	}

	// Assemble zCols, baselines, and verify item weights match identically across all groups.
	zCols := make([][]int, numGroups)
	baselines := make([][]float64, numGroups)
	hasNonZeroBaseline := false
	var items [][]float64
	if numItems > 0 {
		items = make([][]float64, numItems)
		for i := range numItems {
			items[i] = make([]float64, numPeriods)
		}
	}
	rowItemVals := make([]float64, numItems)

	for g := range numGroups {
		zCols[g] = make([]int, numPeriods)
		baselines[g] = make([]float64, numPeriods)
		for t := range numPeriods {
			pair := groupPairs[g][t]
			zCols[g][t] = pair.zCol
			baselines[g][t] = pair.a
			if pair.a != 0 {
				hasNonZeroBaseline = true
			}
			clear(rowItemVals)
			for idx, bCol := range pair.binCols {
				it := xItemOf[bCol]
				if it < 0 || xCols[g][it] != bCol {
					return detectedMAD{}, false
				}
				rowItemVals[it] = pair.binVals[idx]
			}
			if g == 0 {
				for i := range numItems {
					items[i][t] = rowItemVals[i]
				}
			} else {
				for i := range numItems {
					if rowItemVals[i] != items[i][t] {
						return detectedMAD{}, false
					}
				}
			}
		}
	}
	if !hasNonZeroBaseline {
		baselines = nil
	}

	// Verify 100% row closure.
	for r := range p.numConstraints {
		if !rowConsumed[r] {
			return detectedMAD{}, false
		}
	}

	prob := mad.Problem{
		Groups:          numGroups,
		Baselines:       baselines,
		Items:           items,
		Step:            step,
		RoundWeight:     roundWeight,
		AllowZeroStep:   hasRounding && allowZeroStep,
		DesignatedItems: designated,
	}
	if err := mad.ValidateProblem(prob); err != nil {
		return detectedMAD{}, false
	}

	return detectedMAD{
		prob:       prob,
		numGroups:  numGroups,
		numItems:   numItems,
		numPeriods: numPeriods,
		directStep: directStep,
		rounded:    hasRounding,
		xCols:      xCols,
		zCols:      zCols,
		mCols:      mCols,
		nCols:      nCols,
		rCols:      rCols,
	}, true
}

func parseMADEqualityRows(
	p *CompiledProblem,
	rowConsumed []bool,
	xGroupOwner []int,
	numGroups, numItems, remRows int,
) (xCols [][]int, designated []int, ok bool) {
	if numItems == 0 {
		if remRows != 0 {
			return nil, nil, false
		}
		xCols = make([][]int, numGroups)
		return xCols, nil, true
	}

	hasDesignated := remRows == numItems+numGroups
	eqRows := make([][]int, 0, remRows)
	eqDeg := make([]int, p.numVars)

	for r := range p.numConstraints {
		if rowConsumed[r] {
			continue
		}
		if p.rowSense[r] != Equal {
			return nil, nil, false
		}
		var sign float64
		switch p.rowRHS[r] {
		case 1.0:
			sign = 1.0
		case -1.0:
			sign = -1.0
		default:
			return nil, nil, false
		}
		s, e := p.rowPtr[r], p.rowPtr[r+1]
		if e-s != numGroups {
			return nil, nil, false
		}
		rowCols := make([]int, numGroups)
		for off := range numGroups {
			col := p.colIdx[s+off]
			if p.varCat[col] != Binary || sign*p.rowVal[s+off] != 1.0 {
				return nil, nil, false
			}
			rowCols[off] = col
			eqDeg[col]++
		}
		eqRows = append(eqRows, rowCols)
		rowConsumed[r] = true
	}
	if len(eqRows) != remRows {
		return nil, nil, false
	}

	var partRows [][]int
	var desigRows [][]int

	if !hasDesignated {
		partRows = eqRows
	} else if numGroups == 1 {
		seen := make(map[int]bool, numItems)
		for _, rc := range eqRows {
			col := rc[0]
			if seen[col] {
				desigRows = append(desigRows, rc)
			} else {
				seen[col] = true
				partRows = append(partRows, rc)
			}
		}
		if len(partRows) != numItems || len(desigRows) != 1 {
			return nil, nil, false
		}
	} else {
		// Each non-designated binary appears in 1 eq row; each designated binary appears in 2.
		deg2Count := 0
		for j := range p.numVars {
			if p.varCat[j] != Binary {
				continue
			}
			switch eqDeg[j] {
			case 1:
			case 2:
				deg2Count++
			default:
				return nil, nil, false
			}
		}
		if deg2Count != numGroups*numGroups {
			return nil, nil, false
		}

		// Classify each equality row using group ownership and degree-1 witnesses.
		// Partition rows span distinct groups (or contain deg-1 binaries); designated rows
		// contain only deg-2 binaries all belonging to a single group g.
		for _, rc := range eqRows {
			allDeg2 := true
			firstGrp := -1
			sameKnownGrp := true
			hasDistinctKnownGrp := false
			for _, col := range rc {
				if eqDeg[col] != 2 {
					allDeg2 = false
				}
				g := xGroupOwner[col]
				if g >= 0 {
					if firstGrp < 0 {
						firstGrp = g
					} else if g != firstGrp {
						sameKnownGrp = false
						hasDistinctKnownGrp = true
					}
				}
			}
			switch {
			case !allDeg2 || hasDistinctKnownGrp:
				partRows = append(partRows, rc)
			case sameKnownGrp && firstGrp >= 0:
				desigRows = append(desigRows, rc)
			default:
				// Ambiguous all-zero designated row: fall back safely to B&B.
				return nil, nil, false
			}
		}
		if len(partRows) != numItems || len(desigRows) != numGroups {
			return nil, nil, false
		}
	}

	xCols = make([][]int, numGroups)
	for g := range numGroups {
		xCols[g] = make([]int, numItems)
	}
	binAssigned := make([]bool, p.numVars)
	grpSeen := make([]bool, numGroups)
	unownedCols := make([]int, 0, numGroups)

	for i, rc := range partRows {
		clear(grpSeen)
		unownedCols = unownedCols[:0]
		for _, col := range rc {
			if binAssigned[col] {
				return nil, nil, false
			}
			binAssigned[col] = true
			g := xGroupOwner[col]
			if g >= 0 {
				if grpSeen[g] {
					return nil, nil, false
				}
				grpSeen[g] = true
				xCols[g][i] = col
			} else {
				unownedCols = append(unownedCols, col)
			}
		}
		uIdx := 0
		for g := range numGroups {
			if !grpSeen[g] {
				xCols[g][i] = unownedCols[uIdx]
				uIdx++
			}
		}
		if uIdx != len(unownedCols) {
			return nil, nil, false
		}
	}

	// Verify every binary variable in p was assigned to a (g, i) slot.
	for j := range p.numVars {
		if p.varCat[j] == Binary && !binAssigned[j] {
			return nil, nil, false
		}
	}

	if hasDesignated {
		colGroup := make([]int, p.numVars)
		colItem := make([]int, p.numVars)
		for j := range p.numVars {
			colGroup[j] = -1
			colItem[j] = -1
		}
		for g := range numGroups {
			for i := range numItems {
				col := xCols[g][i]
				colGroup[col] = g
				colItem[col] = i
			}
		}

		desigGroupSeen := make([]bool, numGroups)
		var refItems []int
		for _, rc := range desigRows {
			g := colGroup[rc[0]]
			if g < 0 || desigGroupSeen[g] {
				return nil, nil, false
			}
			desigGroupSeen[g] = true
			rowItems := make([]int, numGroups)
			for idx, col := range rc {
				if colGroup[col] != g {
					return nil, nil, false
				}
				rowItems[idx] = colItem[col]
			}
			slices.Sort(rowItems)
			for idx := 1; idx < len(rowItems); idx++ {
				if rowItems[idx] == rowItems[idx-1] {
					return nil, nil, false
				}
			}
			if refItems == nil {
				refItems = rowItems
			} else if !slices.Equal(refItems, rowItems) {
				return nil, nil, false
			}
		}
		designated = refItems
	}

	return xCols, designated, true
}

func madDDTwoSum(a, b float64) (hi, lo float64) {
	hi = a + b
	v := hi - a
	lo = (a - (hi - v)) + (b - v)
	return hi, lo
}

func madDDAdd(aHi, aLo, bHi, bLo float64) (hi, lo float64) {
	sHi, sLo := madDDTwoSum(aHi, bHi)
	tHi, tLo := madDDTwoSum(aLo, bLo)
	sLo += tHi
	sHi, sLo = madDDTwoSum(sHi, sLo)
	sLo += tLo
	return madDDTwoSum(sHi, sLo)
}

func madDDTwoProd(a, b float64) (hi, lo float64) {
	hi = a * b
	lo = math.FMA(a, b, -hi)
	return hi, lo
}

// reconstructMADPrimal maps a mad.Result back onto CompiledProblem variables
// and checks that all variable bounds (including any finite upper bounds on m_g or n_g)
// and constraint tolerances are satisfied.
func reconstructMADPrimal(p *CompiledProblem, det detectedMAD, mRes mad.Result) ([]float64, bool) {
	if len(mRes.Assignment) != det.numItems || len(mRes.Levels) != det.numGroups {
		return nil, false
	}
	if det.rounded && len(mRes.Steps) != det.numGroups {
		return nil, false
	}

	vals := make([]float64, p.numVars)
	for i, g := range mRes.Assignment {
		if g < 0 || g >= det.numGroups {
			return nil, false
		}
		vals[det.xCols[g][i]] = 1.0
	}

	yHi := make([]float64, det.numPeriods)
	yLo := make([]float64, det.numPeriods)

	for g := range det.numGroups {
		for t := range det.numPeriods {
			if det.prob.Baselines != nil {
				yHi[t] = det.prob.Baselines[g][t]
			} else {
				yHi[t] = 0
			}
			yLo[t] = 0
		}
		for i, ag := range mRes.Assignment {
			if ag != g {
				continue
			}
			row := det.prob.Items[i]
			for t := range det.numPeriods {
				if d := row[t]; d != 0 {
					yHi[t], yLo[t] = madDDAdd(yHi[t], yLo[t], d, 0)
				}
			}
		}

		mVal := mRes.Levels[g]
		if math.IsNaN(mVal) || math.IsInf(mVal, 0) {
			return nil, false
		}
		var mHi, mLo float64
		if det.directStep {
			nVal := float64(mRes.Steps[g])
			nCol := det.nCols[g]
			if nVal < p.varLB[nCol] || nVal > p.varUB[nCol] {
				return nil, false
			}
			vals[nCol] = nVal
			mHi, mLo = madDDTwoProd(det.prob.Step, nVal)
		} else {
			mCol := det.mCols[g]
			if mVal < p.varLB[mCol] || mVal > p.varUB[mCol] {
				return nil, false
			}
			vals[mCol] = mVal
			mHi, mLo = mVal, 0
			if det.rounded {
				nVal := float64(mRes.Steps[g])
				nCol := det.nCols[g]
				if nVal < p.varLB[nCol] || nVal > p.varUB[nCol] {
					return nil, false
				}
				vals[nCol] = nVal
				pHi, pLo := madDDTwoProd(det.prob.Step, nVal)
				dHi, dLo := madDDAdd(mHi, mLo, -pHi, -pLo)
				rVal := math.Abs(dHi + dLo)
				if rVal < 0 || math.IsNaN(rVal) || math.IsInf(rVal, 0) {
					return nil, false
				}
				vals[det.rCols[g]] = rVal
			}
		}

		for t := range det.numPeriods {
			dHi, dLo := madDDAdd(yHi[t], yLo[t], -mHi, -mLo)
			zVal := math.Abs(dHi + dLo)
			if zVal < 0 || math.IsNaN(zVal) || math.IsInf(zVal, 0) {
				return nil, false
			}
			vals[det.zCols[g][t]] = zVal
		}
	}

	bV, rV, iV := p.MaxViolation(vals, true)
	if bV > incumbentTol || rV > incumbentTol || iV > incumbentTol {
		return nil, false
	}
	return vals, true
}

// trySolveCombinatorialMAD attempts to detect and solve an algebraic MAD model
// via internal/mad (exact DFS / bitmask DP) before Branch-and-Cut.
// Returns handled=false whenever p is not a closed MAD instance, exceeds
// combinatorial caps, or has binding finite bounds so the caller falls back to B&B.
func trySolveCombinatorialMAD(
	ctx context.Context,
	p *CompiledProblem,
	backend Backend,
	start, deadline time.Time,
) (Result, bool) {
	det, ok := detectMADStructure(p)
	if !ok {
		return Result{}, false
	}

	var madOpts []mad.Option
	if !deadline.IsZero() {
		rem := time.Until(deadline)
		if rem <= 0 {
			triv := trivialDualBound(p.sense)
			return Result{
				modelID:      p.modelID,
				Backend:      backend,
				Status:       StatusTimeLimit,
				BestBound:    triv,
				RootLPBound:  triv,
				RootCutBound: triv,
				SolveTime:    time.Since(start),
			}, true
		}
		madOpts = append(madOpts, mad.WithTimeLimit(rem))
	}

	mRes, err := mad.Solve(ctx, det.prob, madOpts...)
	if err != nil {
		// ErrCombinatorialLimit or validation fallback: transparently defer to B&B.
		return Result{}, false
	}

	switch mRes.Status {
	case mad.StatusOptimal:
		vals, okRec := reconstructMADPrimal(p, det, mRes)
		if !okRec {
			return Result{}, false
		}
		obj := p.EvaluateObjective(vals)
		if math.IsNaN(obj) || math.IsInf(obj, 0) {
			return Result{}, false
		}
		bound := mRes.BestBound + p.objConstant
		if math.IsNaN(bound) || math.IsInf(bound, 0) {
			return Result{}, false
		}
		if bound > obj {
			bound = obj
		}
		return Result{
			modelID:      p.modelID,
			Backend:      backend,
			Status:       StatusOptimal,
			Values:       vals,
			Objective:    obj,
			BestBound:    bound,
			RootLPBound:  bound,
			RootCutBound: bound,
			SolveTime:    time.Since(start),
		}, true

	case mad.StatusTimeLimit:
		triv := trivialDualBound(p.sense)
		res := Result{
			modelID:      p.modelID,
			Backend:      backend,
			Status:       StatusTimeLimit,
			BestBound:    triv,
			RootLPBound:  triv,
			RootCutBound: triv,
			SolveTime:    time.Since(start),
		}
		if vals, okRec := reconstructMADPrimal(p, det, mRes); okRec {
			obj := p.EvaluateObjective(vals)
			if !math.IsNaN(obj) && !math.IsInf(obj, 0) {
				res.Values = vals
				res.Objective = obj
			}
		}
		if !math.IsNaN(mRes.BestBound) && !math.IsInf(mRes.BestBound, 0) {
			b := mRes.BestBound + p.objConstant
			if !math.IsNaN(b) && !math.IsInf(b, 0) && (len(res.Values) == 0 || b <= res.Objective) {
				res.BestBound = b
			}
		}
		return res, true

	default:
		return Result{}, false
	}
}
