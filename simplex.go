package gopt

import (
	"math"
	"slices"
	"time"

	"gopt/internal/simd"
)

const (
	primalTol = 1e-7
	dualTol   = 1e-7
	pivotTol  = 1e-8
	harrisTol = 1e-9
)

// LPResult holds the result of an LP relaxation solve.
type LPResult struct {
	Status       Status
	ObjectiveMin float64
	Objective    float64
	Primal       []float64
	Dual         []float64
	ReducedCosts []float64
	Iterations   int
}

type dynamicRow struct {
	ColIdx []int
	Val    []float64
	Dense  []float64
	RHS    float64
}

// LPSolver encapsulates a Revised Dual & Primal Simplex state that can be solved
// from scratch and repeatedly warm-started with modified bounds or added cutting planes.
type LPSolver struct {
	Prob      *CompiledProblem
	WS        *Workspace
	ExtraRows []dynamicRow
	MaxIter   int
	Deadline  time.Time
	luValid   bool
	dualValid bool
}

func (s *LPSolver) timeExpired() bool {
	return !s.Deadline.IsZero() && time.Now().After(s.Deadline)
}

// NewLPSolver creates an LPSolver initialized for the given CompiledProblem.
func NewLPSolver(p *CompiledProblem, ws *Workspace) *LPSolver {
	if ws == nil {
		ws = AcquireWorkspace(p.NumVars, p.NumConstraints)
	} else {
		ws.EnsureSize(p.NumVars, p.NumConstraints)
	}
	s := &LPSolver{
		Prob:    p,
		WS:      ws,
		MaxIter: max(2000, 20*(p.NumVars+p.NumConstraints)),
	}
	s.InitCrashBasis()
	return s
}

func (s *LPSolver) TotalRows() int {
	return s.Prob.NumConstraints + len(s.ExtraRows)
}

func (s *LPSolver) InitCrashBasis() {
	p := s.Prob
	n := p.NumVars
	m := s.TotalRows()
	ws := s.WS
	ws.EnsureSize(n, m)

	for j := range n {
		ws.LB[j] = p.VarLB[j]
		ws.UB[j] = p.VarUB[j]
		ws.Cost[j] = p.ObjMin[j]
		ws.BasicRow[j] = -1
		ws.Status[j] = initialNonBasicStatus(ws.LB[j], ws.UB[j], ws.Cost[j])
		ws.X[j] = nonBasicValue(ws.Status[j], ws.LB[j], ws.UB[j])
	}

	for i := range m {
		slackVar := n + i
		ws.Cost[slackVar] = 0
		if i < p.NumConstraints {
			switch p.RowSense[i] {
			case LessEqual:
				ws.LB[slackVar] = 0
				ws.UB[slackVar] = math.Inf(1)
			case GreaterEqual:
				ws.LB[slackVar] = math.Inf(-1)
				ws.UB[slackVar] = 0
			case Equal:
				ws.LB[slackVar] = 0
				ws.UB[slackVar] = 0
			}
		} else {
			ws.LB[slackVar] = 0
			ws.UB[slackVar] = math.Inf(1)
		}
		ws.Basis[i] = slackVar
		ws.BasicRow[slackVar] = i
		ws.Status[slackVar] = statusBasic
		ws.DSE[i] = 1.0
	}
	s.luValid = false
	s.dualValid = false
}

func (s *LPSolver) CopyRootBasisFrom(src *LPSolver) {
	if len(src.ExtraRows) > 0 || len(s.ExtraRows) > 0 {
		return
	}
	n := s.Prob.NumVars
	m := s.Prob.NumConstraints
	ws := s.WS
	sws := src.WS
	ws.EnsureSize(n, m)
	copy(ws.Cost[:n+m], sws.Cost[:n+m])
	copy(ws.Basis[:m], sws.Basis[:m])
	copy(ws.BasicRow[:n+m], sws.BasicRow[:n+m])
	copy(ws.Status[:n+m], sws.Status[:n+m])
	copy(ws.X[:n+m], sws.X[:n+m])
	copy(ws.LB[:n+m], sws.LB[:n+m])
	copy(ws.UB[:n+m], sws.UB[:n+m])
	copy(ws.DSE[:m], sws.DSE[:m])
	copy(ws.Y[:m], sws.Y[:m])
	copy(ws.Dj[:n+m], sws.Dj[:n+m])
	s.luValid = false
	s.dualValid = true
}

func initialNonBasicStatus(lb, ub, cost float64) varBasisStatus {
	if lb == ub {
		return statusFixed
	}
	hasLB := !math.IsInf(lb, -1)
	hasUB := !math.IsInf(ub, 1)
	if hasLB && hasUB {
		if cost < 0 {
			return statusAtUpper
		}
		return statusAtLower
	}
	if hasLB {
		return statusAtLower
	}
	if hasUB {
		return statusAtUpper
	}
	return statusSuperBasic
}

func nonBasicValue(st varBasisStatus, lb, ub float64) float64 {
	switch st {
	case statusAtLower, statusFixed:
		return lb
	case statusAtUpper:
		return ub
	default:
		return 0
	}
}

func (s *LPSolver) SetStructuralBounds(lb, ub []float64) {
	ws := s.WS
	n := s.Prob.NumVars
	for j := range n {
		ws.LB[j] = lb[j]
		ws.UB[j] = ub[j]
		st := ws.Status[j]
		if st == statusBasic {
			continue
		}
		hasLB := !math.IsInf(lb[j], -1)
		hasUB := !math.IsInf(ub[j], 1)
		if lb[j] == ub[j] {
			ws.Status[j] = statusFixed
			ws.X[j] = lb[j]
		} else if hasLB && hasUB {
			if st == statusAtUpper || (st == statusFixed && ws.Dj[j] < 0) {
				ws.Status[j] = statusAtUpper
				ws.X[j] = ub[j]
			} else {
				ws.Status[j] = statusAtLower
				ws.X[j] = lb[j]
			}
		} else if hasLB {
			ws.Status[j] = statusAtLower
			ws.X[j] = lb[j]
		} else if hasUB {
			ws.Status[j] = statusAtUpper
			ws.X[j] = ub[j]
		} else {
			ws.Status[j] = statusSuperBasic
			ws.X[j] = 0
		}
	}
}

func (s *LPSolver) AddCut(colIdx []int, val []float64, rhs float64) {
	p := s.Prob
	n := p.NumVars
	dr := dynamicRow{
		ColIdx: slices.Clone(colIdx),
		Val:    slices.Clone(val),
		RHS:    rhs,
	}
	if n > 0 && n <= 2048 {
		dr.Dense = make([]float64, max(n, p.RowStride))
		for k, col := range colIdx {
			dr.Dense[col] = val[k]
		}
	}
	s.ExtraRows = append(s.ExtraRows, dr)

	oldM := s.WS.M
	newM := oldM + 1
	ws := s.WS

	savedBasis := slices.Clone(ws.Basis[:oldM])
	savedStatus := slices.Clone(ws.Status[:n+oldM])
	savedLB := slices.Clone(ws.LB[:n])
	savedUB := slices.Clone(ws.UB[:n])

	ws.EnsureSize(n, newM)
	for j := range n {
		ws.LB[j] = savedLB[j]
		ws.UB[j] = savedUB[j]
		ws.Cost[j] = p.ObjMin[j]
		ws.BasicRow[j] = -1
		ws.Status[j] = savedStatus[j]
	}
	for i := range oldM {
		slackOld := n + i
		ws.Cost[slackOld] = 0
		if i < p.NumConstraints {
			switch p.RowSense[i] {
			case LessEqual:
				ws.LB[slackOld] = 0
				ws.UB[slackOld] = math.Inf(1)
			case GreaterEqual:
				ws.LB[slackOld] = math.Inf(-1)
				ws.UB[slackOld] = 0
			case Equal:
				ws.LB[slackOld] = 0
				ws.UB[slackOld] = 0
			}
		} else {
			ws.LB[slackOld] = 0
			ws.UB[slackOld] = math.Inf(1)
		}
		ws.BasicRow[slackOld] = -1
		ws.Status[slackOld] = savedStatus[slackOld]
	}

	newSlack := n + oldM
	ws.Cost[newSlack] = 0
	ws.LB[newSlack] = 0
	ws.UB[newSlack] = math.Inf(1)
	ws.Status[newSlack] = statusBasic

	for i := range oldM {
		bVar := savedBasis[i]
		ws.Basis[i] = bVar
		ws.BasicRow[bVar] = i
		ws.Status[bVar] = statusBasic
		ws.DSE[i] = 1.0
	}
	ws.Basis[oldM] = newSlack
	ws.BasicRow[newSlack] = oldM
	ws.DSE[oldM] = 1.0

	for k := range n + newM {
		if ws.Status[k] != statusBasic {
			ws.X[k] = nonBasicValue(ws.Status[k], ws.LB[k], ws.UB[k])
		}
	}
	s.luValid = false
	s.dualValid = false
}

func (s *LPSolver) fillColumn(colVar int, dst []float64) {
	p := s.Prob
	n := p.NumVars
	if colVar >= n {
		row := colVar - n
		dst[row] = 1.0
		return
	}
	for k := p.ColPtr[colVar]; k < p.ColPtr[colVar+1]; k++ {
		dst[p.RowIdx[k]] = p.ColVal[k]
	}
	baseM := p.NumConstraints
	for r, dr := range s.ExtraRows {
		if dr.Dense != nil {
			dst[baseM+r] = dr.Dense[colVar]
		} else {
			for k, c := range dr.ColIdx {
				if c == colVar {
					dst[baseM+r] = dr.Val[k]
					break
				}
			}
		}
	}
}

func (s *LPSolver) appendBasisColumn(colVar int, rows []int, vals []float64) ([]int, []float64) {
	p := s.Prob
	n := p.NumVars
	if colVar >= n {
		rows = append(rows, colVar-n)
		vals = append(vals, 1.0)
		return rows, vals
	}
	start, end := p.ColPtr[colVar], p.ColPtr[colVar+1]
	rows = append(rows, p.RowIdx[start:end]...)
	vals = append(vals, p.ColVal[start:end]...)
	baseM := p.NumConstraints
	for r, dr := range s.ExtraRows {
		if dr.Dense != nil {
			if v := dr.Dense[colVar]; v != 0 {
				rows = append(rows, baseM+r)
				vals = append(vals, v)
			}
		} else {
			for k, c := range dr.ColIdx {
				if c == colVar {
					if v := dr.Val[k]; v != 0 {
						rows = append(rows, baseM+r)
						vals = append(vals, v)
					}
					break
				}
			}
		}
	}
	return rows, vals
}

func (s *LPSolver) rowRHS(i int) float64 {
	if i < s.Prob.NumConstraints {
		return s.Prob.RowRHS[i]
	}
	return s.ExtraRows[i-s.Prob.NumConstraints].RHS
}

func (s *LPSolver) Solve() LPResult {
	p := s.Prob
	if p == nil || p.InvalidErr != nil {
		return LPResult{Status: StatusUndefined}
	}
	if s.timeExpired() {
		return LPResult{Status: StatusTimeLimit}
	}
	n := p.NumVars
	m := s.TotalRows()
	ws := s.WS

	for j := range n {
		if ws.LB[j] > ws.UB[j]+primalTol {
			return LPResult{Status: StatusInfeasible}
		}
	}
	if n == 0 {
		return LPResult{
			Status:       StatusOptimal,
			ObjectiveMin: 0,
			Objective:    p.ObjConstant,
		}
	}
	if m == 0 {
		return s.solveUnconstrained()
	}

	copy(ws.WorkCost, ws.Cost)
	if !s.luValid || ws.LU.m != m || ws.LU.NeedsRefactor() {
		if err := s.factorizeBasis(); err != nil {
			savedLB := slices.Clone(ws.LB[:n])
			savedUB := slices.Clone(ws.UB[:n])
			s.InitCrashBasis()
			s.SetStructuralBounds(savedLB, savedUB)
			if err := s.factorizeBasis(); err != nil {
				return LPResult{Status: StatusNumericalError}
			}
		}
		s.computeDualsAndReducedCosts(ws.WorkCost)
		s.dualValid = true
	} else if !s.dualValid {
		s.computeDualsAndReducedCosts(ws.WorkCost)
		s.dualValid = true
	}

	dualFeasible := s.stabilizeNonBasicBounds()
	s.computePrimalValues()

	totalIter := 0

	if dualFeasible {
		status, iters := s.runDualSimplexLoop(ws.WorkCost)
		totalIter += iters
		if status == StatusOptimal {
			s.dualValid = true
			return s.buildResult(StatusOptimal, totalIter)
		}
		if status == StatusInfeasible || status == StatusIterationLimit || status == StatusTimeLimit {
			s.dualValid = false
			return LPResult{Status: status, Iterations: totalIter}
		}
	}

	s.dualValid = false
	remIter := s.MaxIter - totalIter
	if remIter <= 0 {
		return LPResult{Status: StatusIterationLimit, Iterations: totalIter}
	}
	status, iters := s.solveTwoPhasePrimalWithLimit(remIter)
	totalIter += iters
	if status != StatusOptimal {
		return LPResult{Status: status, Iterations: totalIter}
	}
	s.dualValid = true
	return s.buildResult(StatusOptimal, totalIter)
}

func (s *LPSolver) solveUnconstrained() LPResult {
	p := s.Prob
	n := p.NumVars
	ws := s.WS
	primal := make([]float64, n)
	dj := make([]float64, n)
	objMin := 0.0
	for j := range n {
		c := ws.Cost[j]
		dj[j] = c
		if c > dualTol {
			if math.IsInf(ws.LB[j], -1) {
				return LPResult{Status: StatusUnbounded}
			}
			primal[j] = ws.LB[j]
		} else if c < -dualTol {
			if math.IsInf(ws.UB[j], 1) {
				return LPResult{Status: StatusUnbounded}
			}
			primal[j] = ws.UB[j]
		} else {
			if !math.IsInf(ws.LB[j], -1) {
				primal[j] = ws.LB[j]
			} else if !math.IsInf(ws.UB[j], 1) {
				primal[j] = ws.UB[j]
			}
		}
		objMin += c * primal[j]
	}
	return LPResult{
		Status:       StatusOptimal,
		ObjectiveMin: objMin,
		Objective:    p.ObjectiveFromMin(objMin),
		Primal:       primal,
		ReducedCosts: dj,
	}
}

func (s *LPSolver) factorizeBasis() error {
	ws := s.WS
	m := s.TotalRows()
	err := ws.LU.FactorizeSparse(m, func(col int, rows []int, vals []float64) ([]int, []float64) {
		return s.appendBasisColumn(ws.Basis[col], rows, vals)
	})
	s.luValid = err == nil
	return err
}

func (s *LPSolver) computeDualsAndReducedCosts(cost []float64) {
	p := s.Prob
	ws := s.WS
	n := p.NumVars
	m := s.TotalRows()

	for i := range m {
		ws.Y[i] = cost[ws.Basis[i]]
	}
	ws.LU.BTRAN(ws.Y)

	baseM := p.NumConstraints
	for j := range n {
		if ws.Status[j] == statusBasic {
			ws.Dj[j] = 0
			continue
		}
		dot := 0.0
		for k := p.ColPtr[j]; k < p.ColPtr[j+1]; k++ {
			dot += ws.Y[p.RowIdx[k]] * p.ColVal[k]
		}
		ws.Dj[j] = cost[j] - dot
	}
	for r, dr := range s.ExtraRows {
		yr := ws.Y[baseM+r]
		if math.Abs(yr) <= 1e-16 {
			continue
		}
		for k, c := range dr.ColIdx {
			if ws.Status[c] != statusBasic {
				ws.Dj[c] -= yr * dr.Val[k]
			}
		}
	}

	for i := range m {
		slackVar := n + i
		if ws.Status[slackVar] == statusBasic {
			ws.Dj[slackVar] = 0
		} else {
			ws.Dj[slackVar] = cost[slackVar] - ws.Y[i]
		}
	}
}

func (s *LPSolver) stabilizeNonBasicBounds() bool {
	ws := s.WS
	tot := ws.TotVar
	dualFeas := true

	for k := range tot {
		st := ws.Status[k]
		if st == statusBasic {
			continue
		}
		lb, ub, dj := ws.LB[k], ws.UB[k], ws.Dj[k]
		if st == statusFixed {
			ws.X[k] = lb
			continue
		}
		hasLB := !math.IsInf(lb, -1)
		hasUB := !math.IsInf(ub, 1)

		if hasLB && hasUB {
			if dj < -dualTol {
				ws.Status[k] = statusAtUpper
				ws.X[k] = ub
			} else if dj > dualTol {
				ws.Status[k] = statusAtLower
				ws.X[k] = lb
			} else {
				if st != statusAtUpper {
					ws.Status[k] = statusAtLower
					ws.X[k] = lb
				} else {
					ws.X[k] = ub
				}
			}
			continue
		}
		ws.X[k] = nonBasicValue(st, lb, ub)
		if st == statusAtLower && dj < -dualTol {
			dualFeas = false
		} else if st == statusAtUpper && dj > dualTol {
			dualFeas = false
		} else if st == statusSuperBasic && math.Abs(dj) > dualTol {
			dualFeas = false
		}
	}
	return dualFeas
}

func (s *LPSolver) isDualFeasible() bool {
	ws := s.WS
	tot := ws.TotVar
	tol := 1e-5
	for k := range tot {
		st := ws.Status[k]
		dj := ws.Dj[k]
		switch st {
		case statusAtLower:
			if dj < -tol {
				return false
			}
		case statusAtUpper:
			if dj > tol {
				return false
			}
		case statusSuperBasic:
			if math.Abs(dj) > tol {
				return false
			}
		}
	}
	return true
}

func (s *LPSolver) computePrimalValues() {
	p := s.Prob
	ws := s.WS
	n := p.NumVars
	m := s.TotalRows()

	for i := range m {
		ws.XB[i] = s.rowRHS(i)
	}

	baseM := p.NumConstraints
	for j := range n {
		if ws.Status[j] == statusBasic {
			continue
		}
		xj := ws.X[j]
		if xj == 0 {
			continue
		}
		for k := p.ColPtr[j]; k < p.ColPtr[j+1]; k++ {
			ws.XB[p.RowIdx[k]] -= p.ColVal[k] * xj
		}
	}
	for r, dr := range s.ExtraRows {
		sum := 0.0
		for k, c := range dr.ColIdx {
			if ws.Status[c] != statusBasic {
				sum += dr.Val[k] * ws.X[c]
			}
		}
		ws.XB[baseM+r] -= sum
	}
	for i := range m {
		slackVar := n + i
		if ws.Status[slackVar] != statusBasic {
			ws.XB[i] -= ws.X[slackVar]
		}
	}

	ws.LU.FTRAN(ws.XB)
	for i := range m {
		ws.X[ws.Basis[i]] = ws.XB[i]
	}
}

func (s *LPSolver) ComputeTableauRow(r int) {
	p := s.Prob
	ws := s.WS
	n := p.NumVars
	m := s.TotalRows()

	clear(ws.Rho)
	ws.Rho[r] = 1.0
	ws.LU.BTRAN(ws.Rho)

	if ws.TabRowSparse {
		for _, k := range ws.TabRowIdx {
			ws.TabRow[k] = 0
			ws.TabRowMark[k] = false
		}
		ws.TabRowIdx = ws.TabRowIdx[:0]
	} else {
		clear(ws.TabRow)
		ws.TabRowIdx = ws.TabRowIdx[:0]
	}
	baseM := p.NumConstraints

	if n > 0 && n <= DenseThreshold && len(p.DenseRows) == baseM*p.RowStride {
		ws.TabRowSparse = false
		stride := p.RowStride
		tabStruct := ws.TabRow[:n]
		for i := range baseM {
			yi := ws.Rho[i]
			if math.Abs(yi) > 1e-16 {
				simd.Axpy64(tabStruct, p.DenseRows[i*stride:i*stride+n], yi)
			}
		}
		for idx, dr := range s.ExtraRows {
			yi := ws.Rho[baseM+idx]
			if math.Abs(yi) > 1e-16 && dr.Dense != nil {
				simd.Axpy64(tabStruct, dr.Dense[:n], yi)
			}
		}
		for i := range m {
			ws.TabRow[n+i] = ws.Rho[i]
		}
		for i := range m {
			ws.TabRow[ws.Basis[i]] = 0
		}
		ws.TabRow[ws.Basis[r]] = 1.0
	} else {
		ws.TabRowSparse = true
		for i := range baseM {
			yi := ws.Rho[i]
			if math.Abs(yi) <= 1e-16 {
				continue
			}
			for k := p.RowPtr[i]; k < p.RowPtr[i+1]; k++ {
				col := p.ColIdx[k]
				if !ws.TabRowMark[col] {
					ws.TabRowMark[col] = true
					ws.TabRowIdx = append(ws.TabRowIdx, col)
				}
				ws.TabRow[col] += yi * p.RowVal[k]
			}
		}
		for idx, dr := range s.ExtraRows {
			yi := ws.Rho[baseM+idx]
			if math.Abs(yi) <= 1e-16 {
				continue
			}
			for k, col := range dr.ColIdx {
				if !ws.TabRowMark[col] {
					ws.TabRowMark[col] = true
					ws.TabRowIdx = append(ws.TabRowIdx, col)
				}
				ws.TabRow[col] += yi * dr.Val[k]
			}
		}
		for i := range m {
			yi := ws.Rho[i]
			if math.Abs(yi) > 1e-16 {
				slack := n + i
				ws.TabRowMark[slack] = true
				ws.TabRowIdx = append(ws.TabRowIdx, slack)
				ws.TabRow[slack] = yi
			}
		}
		for i := range m {
			ws.TabRow[ws.Basis[i]] = 0
		}
		br := ws.Basis[r]
		if !ws.TabRowMark[br] {
			ws.TabRowMark[br] = true
			ws.TabRowIdx = append(ws.TabRowIdx, br)
		}
		ws.TabRow[br] = 1.0
	}
}

func (s *LPSolver) runDualSimplexLoop(cost []float64) (Status, int) {
	ws := s.WS
	m := s.TotalRows()
	tot := ws.TotVar

	for iter := 0; iter < s.MaxIter; iter++ {
		if (iter&15 == 0) && s.timeExpired() {
			return StatusTimeLimit, iter
		}
		if ws.LU.NeedsRefactor() {
			if err := s.factorizeBasis(); err != nil {
				return StatusNumericalError, iter
			}
			s.computeDualsAndReducedCosts(cost)
			s.computePrimalValues()
		}

		leaveRow := -1
		bestScore := -1.0
		leaveDir := 0.0
		primalInfeas := 0.0

		for r := range m {
			bVar := ws.Basis[r]
			val := ws.XB[r]
			lb, ub := ws.LB[bVar], ws.UB[bVar]
			var viol, dir float64
			if val < lb-primalTol {
				viol = lb - val
				dir = 1.0
			} else if val > ub+primalTol {
				viol = val - ub
				dir = -1.0
			}
			if viol > 0 {
				w := ws.DSE[r]
				if !(w >= 1e-4 && w <= 1e12) {
					w = 1.0
					ws.DSE[r] = 1.0
				}
				score := (viol * viol) / w
				if leaveRow == -1 || score > bestScore {
					bestScore = score
					leaveRow = r
					leaveDir = dir
					primalInfeas = viol
				}
			}
		}

		if leaveRow == -1 {
			if len(ws.LU.etas) > 0 {
				if err := s.factorizeBasis(); err != nil {
					return StatusNumericalError, iter
				}
				s.computeDualsAndReducedCosts(cost)
				s.computePrimalValues()
				continue
			}
			if !s.isDualFeasible() {
				return StatusNumericalError, iter
			}
			return StatusOptimal, iter
		}

		s.ComputeTableauRow(leaveRow)

		thetaMax := math.Inf(1)
		if ws.TabRowSparse {
			for _, k := range ws.TabRowIdx {
				st := ws.Status[k]
				if st == statusBasic || st == statusFixed {
					continue
				}
				alphaBar := -leaveDir * ws.TabRow[k]
				switch st {
				case statusAtLower:
					if alphaBar > pivotTol {
						d := max(0.0, ws.Dj[k])
						ratio := (d + harrisTol) / alphaBar
						if ratio < thetaMax {
							thetaMax = ratio
						}
					}
				case statusAtUpper:
					if alphaBar < -pivotTol {
						d := max(0.0, -ws.Dj[k])
						ratio := (d + harrisTol) / (-alphaBar)
						if ratio < thetaMax {
							thetaMax = ratio
						}
					}
				case statusSuperBasic:
					if math.Abs(alphaBar) > pivotTol {
						thetaMax = 0
					}
				}
			}
		} else {
			for k := range tot {
				st := ws.Status[k]
				if st == statusBasic || st == statusFixed {
					continue
				}
				alphaBar := -leaveDir * ws.TabRow[k]
				switch st {
				case statusAtLower:
					if alphaBar > pivotTol {
						d := max(0.0, ws.Dj[k])
						ratio := (d + harrisTol) / alphaBar
						if ratio < thetaMax {
							thetaMax = ratio
						}
					}
				case statusAtUpper:
					if alphaBar < -pivotTol {
						d := max(0.0, -ws.Dj[k])
						ratio := (d + harrisTol) / (-alphaBar)
						if ratio < thetaMax {
							thetaMax = ratio
						}
					}
				case statusSuperBasic:
					if math.Abs(alphaBar) > pivotTol {
						thetaMax = 0
					}
				}
			}
		}

		if math.IsInf(thetaMax, 1) {
			if len(ws.LU.etas) > 0 {
				if err := s.factorizeBasis(); err == nil {
					s.computeDualsAndReducedCosts(cost)
					s.computePrimalValues()
					continue
				}
			}
			return StatusInfeasible, iter
		}

		enterVar := -1
		bestPivAbs := 0.0
		bestStaysFeas := false
		if ws.TabRowSparse {
			for _, k := range ws.TabRowIdx {
				st := ws.Status[k]
				if st == statusBasic || st == statusFixed {
					continue
				}
				alphaBar := -leaveDir * ws.TabRow[k]
				eligible := false
				switch st {
				case statusAtLower:
					if alphaBar > pivotTol {
						d := max(0.0, ws.Dj[k])
						eligible = (d / alphaBar) <= thetaMax+1e-12
					}
				case statusAtUpper:
					if alphaBar < -pivotTol {
						d := max(0.0, -ws.Dj[k])
						eligible = (d / (-alphaBar)) <= thetaMax+1e-12
					}
				case statusSuperBasic:
					eligible = math.Abs(alphaBar) > pivotTol
				}
				if eligible {
					absA := math.Abs(ws.TabRow[k])
					staysFeas := absA >= 1e-3 && primalInfeas <= absA*(ws.UB[k]-ws.LB[k])+primalTol
					if staysFeas && !bestStaysFeas {
						bestStaysFeas = true
						bestPivAbs = absA
						enterVar = k
					} else if staysFeas == bestStaysFeas && (absA > bestPivAbs || (absA == bestPivAbs && k < enterVar)) {
						bestPivAbs = absA
						enterVar = k
					}
				}
			}
		} else {
			for k := range tot {
				st := ws.Status[k]
				if st == statusBasic || st == statusFixed {
					continue
				}
				alphaBar := -leaveDir * ws.TabRow[k]
				eligible := false
				switch st {
				case statusAtLower:
					if alphaBar > pivotTol {
						d := max(0.0, ws.Dj[k])
						eligible = (d / alphaBar) <= thetaMax+1e-12
					}
				case statusAtUpper:
					if alphaBar < -pivotTol {
						d := max(0.0, -ws.Dj[k])
						eligible = (d / (-alphaBar)) <= thetaMax+1e-12
					}
				case statusSuperBasic:
					eligible = math.Abs(alphaBar) > pivotTol
				}
				if eligible {
					absA := math.Abs(ws.TabRow[k])
					staysFeas := absA >= 1e-3 && primalInfeas <= absA*(ws.UB[k]-ws.LB[k])+primalTol
					if staysFeas && !bestStaysFeas {
						bestStaysFeas = true
						bestPivAbs = absA
						enterVar = k
					} else if staysFeas == bestStaysFeas && (absA > bestPivAbs || (absA == bestPivAbs && k < enterVar)) {
						bestPivAbs = absA
						enterVar = k
					}
				}
			}
		}

		if enterVar == -1 {
			return StatusInfeasible, iter
		}

		clear(ws.FCol)
		s.fillColumn(enterVar, ws.FCol)
		ws.LU.FTRAN(ws.FCol)

		pivElem := ws.FCol[leaveRow]
		if math.Abs(pivElem) < 1e-10 {
			if err := s.factorizeBasis(); err != nil {
				return StatusNumericalError, iter
			}
			s.computeDualsAndReducedCosts(cost)
			s.computePrimalValues()
			continue
		}

		copy(ws.Tau, ws.Rho)
		ws.LU.FTRAN(ws.Tau)
		invPiv := 1.0 / pivElem
		invPiv2 := invPiv * invPiv
		newLeaveWeight := ws.DSE[leaveRow] * invPiv2
		if !(newLeaveWeight >= 1e-4 && newLeaveWeight <= 1e12) {
			newLeaveWeight = min(max(newLeaveWeight, 1e-4), 1e12)
			if math.IsNaN(newLeaveWeight) {
				newLeaveWeight = 1.0
			}
		}
		for i := range m {
			if i != leaveRow {
				fi := ws.FCol[i]
				if math.Abs(fi) > 1e-14 {
					ratio := fi * invPiv
					w := ws.DSE[i] - 2.0*ratio*ws.Tau[i] + ratio*ratio*ws.DSE[leaveRow]
					if !(w >= 1e-4 && w <= 1e12) {
						w = min(max(w, 1e-4), 1e12)
						if math.IsNaN(w) {
							w = 1.0
						}
					}
					ws.DSE[i] = w
				}
			}
		}
		ws.DSE[leaveRow] = newLeaveWeight

		leaveVar := ws.Basis[leaveRow]
		var newLeaveVal float64
		if leaveDir > 0 {
			ws.Status[leaveVar] = statusAtLower
			newLeaveVal = ws.LB[leaveVar]
		} else {
			ws.Status[leaveVar] = statusAtUpper
			newLeaveVal = ws.UB[leaveVar]
		}
		if ws.LB[leaveVar] == ws.UB[leaveVar] {
			ws.Status[leaveVar] = statusFixed
		}

		primalStep := -leaveDir * primalInfeas / pivElem
		oldEnterVal := ws.X[enterVar]
		newEnterVal := oldEnterVal + primalStep

		for i := range m {
			if i == leaveRow {
				ws.XB[i] = newEnterVal
			} else {
				ws.XB[i] -= ws.FCol[i] * primalStep
			}
			ws.X[ws.Basis[i]] = ws.XB[i]
		}
		ws.X[leaveVar] = newLeaveVal
		ws.X[enterVar] = newEnterVal

		ws.Basis[leaveRow] = enterVar
		ws.BasicRow[leaveVar] = -1
		ws.BasicRow[enterVar] = leaveRow
		ws.Status[enterVar] = statusBasic

		if err := ws.LU.PushEta(leaveRow, ws.FCol); err != nil {
			if err := s.factorizeBasis(); err != nil {
				return StatusNumericalError, iter
			}
			s.computeDualsAndReducedCosts(cost)
			s.computePrimalValues()
		} else {
			dualStep := ws.Dj[enterVar] / ws.TabRow[enterVar]
			if math.Abs(dualStep) > 0 {
				simd.Axpy64(ws.Y[:m], ws.Rho[:m], dualStep)
				if ws.TabRowSparse {
					for _, k := range ws.TabRowIdx {
						ws.Dj[k] -= dualStep * ws.TabRow[k]
					}
				} else {
					simd.Axpy64(ws.Dj[:tot], ws.TabRow[:tot], -dualStep)
				}
			}
			ws.Dj[enterVar] = 0
		}
	}

	return StatusIterationLimit, s.MaxIter
}

func (s *LPSolver) solveTwoPhasePrimalWithLimit(maxIter int) (Status, int) {
	ws := s.WS
	m := s.TotalRows()
	totalIter := 0
	phase1Feasible := false

	for totalIter < maxIter {
		if (totalIter&15 == 0) && s.timeExpired() {
			return StatusTimeLimit, totalIter
		}
		if ws.LU.NeedsRefactor() {
			if err := s.factorizeBasis(); err != nil {
				return StatusNumericalError, totalIter
			}
			s.computePrimalValues()
		}

		clear(ws.WorkCost)
		maxViol := 0.0
		for i := range m {
			bVar := ws.Basis[i]
			val := ws.XB[i]
			if val < ws.LB[bVar]-primalTol {
				ws.WorkCost[bVar] = -1.0
				maxViol = max(maxViol, ws.LB[bVar]-val)
			} else if val > ws.UB[bVar]+primalTol {
				ws.WorkCost[bVar] = 1.0
				maxViol = max(maxViol, val-ws.UB[bVar])
			}
		}

		if maxViol <= primalTol {
			if len(ws.LU.etas) > 0 {
				if err := s.factorizeBasis(); err != nil {
					return StatusNumericalError, totalIter
				}
				s.computePrimalValues()
				continue
			}
			phase1Feasible = true
			break
		}

		s.computeDualsAndReducedCosts(ws.WorkCost)
		stepStatus := s.primalSimplexStep(true)
		totalIter++
		if stepStatus == StatusOptimal {
			if len(ws.LU.etas) > 0 {
				_ = s.factorizeBasis()
				s.computePrimalValues()
				continue
			}
			return StatusInfeasible, totalIter
		}
		if stepStatus != StatusNotSolved {
			return stepStatus, totalIter
		}
	}

	if !phase1Feasible {
		maxViol := 0.0
		for i := range m {
			bVar := ws.Basis[i]
			val := ws.XB[i]
			if val < ws.LB[bVar]-1e-5 {
				maxViol = max(maxViol, ws.LB[bVar]-val)
			} else if val > ws.UB[bVar]+1e-5 {
				maxViol = max(maxViol, val-ws.UB[bVar])
			}
		}
		if maxViol > 1e-5 {
			return StatusIterationLimit, totalIter
		}
	}

	for totalIter < maxIter {
		if (totalIter&15 == 0) && s.timeExpired() {
			return StatusTimeLimit, totalIter
		}
		if ws.LU.NeedsRefactor() {
			if err := s.factorizeBasis(); err != nil {
				return StatusNumericalError, totalIter
			}
			s.computePrimalValues()
		}
		s.computeDualsAndReducedCosts(ws.Cost)
		stepStatus := s.primalSimplexStep(false)
		totalIter++
		if stepStatus == StatusOptimal {
			if len(ws.LU.etas) > 0 {
				if err := s.factorizeBasis(); err != nil {
					return StatusNumericalError, totalIter
				}
				s.computePrimalValues()
				continue
			}
			for i := range m {
				bVar := ws.Basis[i]
				val := ws.XB[i]
				if val < ws.LB[bVar]-1e-5 || val > ws.UB[bVar]+1e-5 {
					return StatusNumericalError, totalIter
				}
			}
			return StatusOptimal, totalIter
		}
		if stepStatus != StatusNotSolved {
			return stepStatus, totalIter
		}
	}

	return StatusIterationLimit, totalIter
}

func (s *LPSolver) primalSimplexStep(phase1 bool) Status {
	ws := s.WS
	m := s.TotalRows()
	tot := ws.TotVar

	enterVar := -1
	enterDir := 0.0
	bestViolation := dualTol

	for k := range tot {
		st := ws.Status[k]
		if st == statusBasic || st == statusFixed {
			continue
		}
		dj := ws.Dj[k]
		switch st {
		case statusAtLower:
			if -dj > bestViolation {
				bestViolation = -dj
				enterVar = k
				enterDir = 1.0
			}
		case statusAtUpper:
			if dj > bestViolation {
				bestViolation = dj
				enterVar = k
				enterDir = -1.0
			}
		case statusSuperBasic:
			if math.Abs(dj) > bestViolation {
				bestViolation = math.Abs(dj)
				enterVar = k
				if dj < 0 {
					enterDir = 1.0
				} else {
					enterDir = -1.0
				}
			}
		}
	}

	if enterVar == -1 {
		return StatusOptimal
	}

	clear(ws.FCol)
	s.fillColumn(enterVar, ws.FCol)
	ws.LU.FTRAN(ws.FCol)

	var maxTheta float64
	if enterDir > 0 {
		maxTheta = ws.UB[enterVar] - ws.X[enterVar]
	} else {
		maxTheta = ws.X[enterVar] - ws.LB[enterVar]
	}
	leaveRow := -1
	leaveToUpper := false
	bestTheta := maxTheta
	bestPivot := 0.0

	for i := range m {
		bVar := ws.Basis[i]
		val := ws.XB[i]
		lb, ub := ws.LB[bVar], ws.UB[bVar]
		d := enterDir * ws.FCol[i]

		if phase1 {
			if val < lb-primalTol {
				if d < -pivotTol && !math.IsInf(ub, 1) {
					theta := (ub - val) / (-d)
					if theta < bestTheta-1e-12 || (math.Abs(theta-bestTheta) <= 1e-12 && math.Abs(d) > bestPivot) {
						bestTheta = theta
						leaveRow = i
						leaveToUpper = true
						bestPivot = math.Abs(d)
					}
				} else if d < -pivotTol {
					theta := (lb - val) / (-d)
					if theta < bestTheta-1e-12 || (math.Abs(theta-bestTheta) <= 1e-12 && math.Abs(d) > bestPivot) {
						bestTheta = theta
						leaveRow = i
						leaveToUpper = false
						bestPivot = math.Abs(d)
					}
				}
				continue
			} else if val > ub+primalTol {
				if d > pivotTol && !math.IsInf(lb, -1) {
					theta := (val - lb) / d
					if theta < bestTheta-1e-12 || (math.Abs(theta-bestTheta) <= 1e-12 && math.Abs(d) > bestPivot) {
						bestTheta = theta
						leaveRow = i
						leaveToUpper = false
						bestPivot = math.Abs(d)
					}
				} else if d > pivotTol {
					theta := (val - ub) / d
					if theta < bestTheta-1e-12 || (math.Abs(theta-bestTheta) <= 1e-12 && math.Abs(d) > bestPivot) {
						bestTheta = theta
						leaveRow = i
						leaveToUpper = true
						bestPivot = math.Abs(d)
					}
				}
				continue
			}
		}

		if d > pivotTol && !math.IsInf(lb, -1) {
			theta := max(0.0, val-lb) / d
			if theta < bestTheta-1e-12 || (math.Abs(theta-bestTheta) <= 1e-12 && math.Abs(d) > bestPivot) {
				bestTheta = theta
				leaveRow = i
				leaveToUpper = false
				bestPivot = math.Abs(d)
			}
		} else if d < -pivotTol && !math.IsInf(ub, 1) {
			theta := max(0.0, ub-val) / (-d)
			if theta < bestTheta-1e-12 || (math.Abs(theta-bestTheta) <= 1e-12 && math.Abs(d) > bestPivot) {
				bestTheta = theta
				leaveRow = i
				leaveToUpper = true
				bestPivot = math.Abs(d)
			}
		}
	}

	if leaveRow == -1 {
		if math.IsInf(maxTheta, 1) {
			if phase1 {
				return StatusOptimal
			}
			return StatusUnbounded
		}
		step := enterDir * maxTheta
		for i := range m {
			ws.XB[i] -= ws.FCol[i] * step
			ws.X[ws.Basis[i]] = ws.XB[i]
		}
		if enterDir > 0 {
			ws.Status[enterVar] = statusAtUpper
			ws.X[enterVar] = ws.UB[enterVar]
		} else {
			ws.Status[enterVar] = statusAtLower
			ws.X[enterVar] = ws.LB[enterVar]
		}
		return StatusNotSolved
	}

	step := enterDir * bestTheta
	newEnterVal := ws.X[enterVar] + step
	leaveVar := ws.Basis[leaveRow]

	for i := range m {
		if i == leaveRow {
			ws.XB[i] = newEnterVal
		} else {
			ws.XB[i] -= ws.FCol[i] * step
		}
		ws.X[ws.Basis[i]] = ws.XB[i]
	}

	if ws.LB[leaveVar] == ws.UB[leaveVar] {
		ws.Status[leaveVar] = statusFixed
		ws.X[leaveVar] = ws.LB[leaveVar]
	} else if leaveToUpper {
		ws.Status[leaveVar] = statusAtUpper
		ws.X[leaveVar] = ws.UB[leaveVar]
	} else {
		ws.Status[leaveVar] = statusAtLower
		ws.X[leaveVar] = ws.LB[leaveVar]
	}

	ws.X[enterVar] = newEnterVal
	ws.Basis[leaveRow] = enterVar
	ws.BasicRow[leaveVar] = -1
	ws.BasicRow[enterVar] = leaveRow
	ws.Status[enterVar] = statusBasic

	if err := ws.LU.PushEta(leaveRow, ws.FCol); err != nil {
		_ = s.factorizeBasis()
		s.computePrimalValues()
	}

	return StatusNotSolved
}

func (s *LPSolver) buildResult(status Status, iters int) LPResult {
	p := s.Prob
	ws := s.WS
	n := p.NumVars
	m := p.NumConstraints

	s.computePrimalValues()
	s.computeDualsAndReducedCosts(ws.Cost)

	primal := make([]float64, n)
	dj := make([]float64, n)
	duals := make([]float64, m)

	objMin := 0.0
	senseSign := 1.0
	if p.Sense == Maximize {
		senseSign = -1.0
	}
	for j := range n {
		val := ws.X[j]
		if val < ws.LB[j] && ws.LB[j]-val <= 1e-6 {
			val = ws.LB[j]
		} else if val > ws.UB[j] && val-ws.UB[j] <= 1e-6 {
			val = ws.UB[j]
		}
		primal[j] = val
		objMin += ws.Cost[j] * val
		dj[j] = senseSign * ws.Dj[j]
	}
	for i := range m {
		duals[i] = senseSign * ws.Y[i]
	}

	return LPResult{
		Status:       status,
		ObjectiveMin: objMin,
		Objective:    p.ObjectiveFromMin(objMin),
		Primal:       primal,
		Dual:         duals,
		ReducedCosts: dj,
		Iterations:   iters,
	}
}
