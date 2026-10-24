package goblin

import (
	"math"
	"slices"
	"time"

	"github.com/paulourio/goblin/internal/simd"
)

const (
	primalTol = 1e-7
	dualTol   = 1e-7
	pivotTol  = 1e-8
	harrisTol = 1e-9
	// minScaledPrimalTol floors the column-normalized primal tolerance of structural columns.
	minScaledPrimalTol = 1e-12
	defaultMinMaxIter  = 2000
	defaultIterMult    = 20
	minStaysFeasPivot  = 1e-3
	pivotTieRelTol     = 1e-11
	minDualPivotElem   = 1e-10
	minDSEWeight       = 1e-4
	maxDSEWeight       = 1e12
	minDSEColEntry     = 1e-14
	phase1RelaxTol     = 1e-5
)

// LPResult holds the result of a continuous LP solve.
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
	Prob        *CompiledProblem
	WS          *Workspace
	ExtraRows   []dynamicRow
	MaxIter     int
	Deadline    time.Time
	Done        <-chan struct{}
	DisableSIMD bool
	luValid     bool
	dualValid   bool
	// colTol holds the primal bound tolerance of each structural column (see feasTol).
	colTol []float64
	// colHasEq reports whether structural column j has a nonzero in any equation row.
	colHasEq []bool
}

func (s *LPSolver) axpy64(dst, src []float64, a float64) {
	if s.DisableSIMD {
		simd.Axpy64Scalar(dst, src, a)
	} else {
		simd.Axpy64(dst, src, a)
	}
}

// feasTol returns the primal bound tolerance of variable k. A value error e in a
// structural x_k perturbs some row activity by up to e*max_i|a_ik|, so structural columns are
// measured in column-normalized units: primalTol / max(1, max_i|a_ik|), floored at
// minScaledPrimalTol. Without this, a basic big-M indicator resting 5e-8 outside its bound
// (inside an absolute 1e-7 tolerance) shifts its row by 0.05 once the value is reported.
// Slack columns are unit columns and use primalTol.
func (s *LPSolver) feasTol(k int) float64 {
	if k < len(s.colTol) {
		return s.colTol[k]
	}
	return primalTol
}

func columnFeasTolerances(p *CompiledProblem) ([]float64, []bool) {
	tol := make([]float64, p.NumVars)
	hasEq := make([]bool, p.NumVars)
	for j := range p.NumVars {
		norm := 1.0
		for k := p.ColPtr[j]; k < p.ColPtr[j+1]; k++ {
			norm = max(norm, math.Abs(p.ColVal[k]))
			if p.RowSense[p.RowIdx[k]] == Equal {
				hasEq[j] = true
			}
		}
		tol[j] = max(primalTol/norm, minScaledPrimalTol)
	}
	return tol, hasEq
}

func (s *LPSolver) timeExpired() bool {
	return stopRequested(s.Deadline, s.Done)
}

// NewLPSolver creates an LPSolver initialized for the given CompiledProblem.
func NewLPSolver(p *CompiledProblem, ws *Workspace) *LPSolver {
	if ws == nil {
		ws = AcquireWorkspace(p.NumVars, p.NumConstraints)
	} else {
		ws.EnsureSize(p.NumVars, p.NumConstraints)
	}
	colTol, colHasEq := columnFeasTolerances(p)
	s := &LPSolver{
		Prob:     p,
		WS:       ws,
		MaxIter:  max(defaultMinMaxIter, defaultIterMult*(p.NumVars+p.NumConstraints)),
		colTol:   colTol,
		colHasEq: colHasEq,
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

// AddCut appends the cutting plane sum_k val[k]*x[colIdx[k]] <= rhs to the continuous LP.
// The new row's slack enters the basis, so the current basis stays primal-degenerate-feasible
// for all existing rows and the next Solve warm-starts with the dual simplex.
func (s *LPSolver) AddCut(colIdx []int, val []float64, rhs float64) {
	s.addCutRows([]dynamicRow{s.newCutRow(colIdx, val, rhs)})
}

func (s *LPSolver) newCutRow(colIdx []int, val []float64, rhs float64) dynamicRow {
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
	return dr
}

// addCutRows appends a batch of cut rows with a single workspace resize. Dual steepest-edge
// weights of existing rows are preserved: appending rows whose slacks are basic leaves the
// existing rows of B^{-1} unchanged, so their weights ||e_r^T B^{-1}||^2 remain exact.
func (s *LPSolver) addCutRows(rows []dynamicRow) {
	if len(rows) == 0 {
		return
	}
	p := s.Prob
	n := p.NumVars
	s.ExtraRows = append(s.ExtraRows, rows...)

	ws := s.WS
	oldM := ws.M
	newM := oldM + len(rows)

	savedBasis := slices.Clone(ws.Basis[:oldM])
	savedStatus := slices.Clone(ws.Status[:n+oldM])
	savedLB := slices.Clone(ws.LB[:n])
	savedUB := slices.Clone(ws.UB[:n])
	savedDSE := slices.Clone(ws.DSE[:oldM])

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
		ws.LB[slackOld], ws.UB[slackOld] = s.slackBounds(i)
		ws.BasicRow[slackOld] = -1
		ws.Status[slackOld] = savedStatus[slackOld]
	}

	for i := range oldM {
		bVar := savedBasis[i]
		ws.Basis[i] = bVar
		ws.BasicRow[bVar] = i
		ws.Status[bVar] = statusBasic
		ws.DSE[i] = savedDSE[i]
	}
	for i := oldM; i < newM; i++ {
		newSlack := n + i
		ws.Cost[newSlack] = 0
		ws.LB[newSlack] = 0
		ws.UB[newSlack] = math.Inf(1)
		ws.Status[newSlack] = statusBasic
		ws.Basis[i] = newSlack
		ws.BasicRow[newSlack] = i
		ws.DSE[i] = 1.0
	}

	for k := range n + newM {
		if ws.Status[k] != statusBasic {
			ws.X[k] = nonBasicValue(ws.Status[k], ws.LB[k], ws.UB[k])
		}
	}
	s.luValid = false
	s.dualValid = false
}

// lpSnapshot captures the basis and bound state of an LPSolver so that a batch of
// cutting planes can be rolled back if the re-solve fails.
type lpSnapshot struct {
	numExtra int
	basis    []int
	status   []varBasisStatus
	lb, ub   []float64
	dse      []float64
}

func (s *LPSolver) snapshot() lpSnapshot {
	ws := s.WS
	n := s.Prob.NumVars
	m := s.TotalRows()
	return lpSnapshot{
		numExtra: len(s.ExtraRows),
		basis:    slices.Clone(ws.Basis[:m]),
		status:   slices.Clone(ws.Status[:n+m]),
		lb:       slices.Clone(ws.LB[:n]),
		ub:       slices.Clone(ws.UB[:n]),
		dse:      slices.Clone(ws.DSE[:m]),
	}
}

// restore rolls the solver back to snap, discarding any cut rows added after it was taken.
func (s *LPSolver) restore(snap lpSnapshot) {
	p := s.Prob
	n := p.NumVars
	s.ExtraRows = s.ExtraRows[:snap.numExtra]
	m := s.TotalRows()
	ws := s.WS
	ws.EnsureSize(n, m)
	for j := range n {
		ws.LB[j] = snap.lb[j]
		ws.UB[j] = snap.ub[j]
		ws.Cost[j] = p.ObjMin[j]
		ws.BasicRow[j] = -1
	}
	for i := range m {
		slack := n + i
		ws.Cost[slack] = 0
		ws.LB[slack], ws.UB[slack] = s.slackBounds(i)
		ws.BasicRow[slack] = -1
	}
	copy(ws.Status[:n+m], snap.status)
	for i := range m {
		ws.Basis[i] = snap.basis[i]
		ws.BasicRow[snap.basis[i]] = i
		ws.DSE[i] = snap.dse[i]
	}
	for k := range n + m {
		if ws.Status[k] != statusBasic {
			ws.X[k] = nonBasicValue(ws.Status[k], ws.LB[k], ws.UB[k])
		}
	}
	s.luValid = false
	s.dualValid = false
}

// slackBounds returns the bounds of the slack of row i (a_i x + s_i = rhs_i).
func (s *LPSolver) slackBounds(i int) (lb, ub float64) {
	if i >= s.Prob.NumConstraints {
		return 0, math.Inf(1)
	}
	switch s.Prob.RowSense[i] {
	case GreaterEqual:
		return math.Inf(-1), 0
	case Equal:
		return 0, 0
	default:
		return 0, math.Inf(1)
	}
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

func (s *LPSolver) addScaledColumn(colVar int, scale float64, dst []float64) {
	if scale == 0 {
		return
	}
	p := s.Prob
	n := p.NumVars
	if colVar >= n {
		dst[colVar-n] += scale
		return
	}
	for k := p.ColPtr[colVar]; k < p.ColPtr[colVar+1]; k++ {
		dst[p.RowIdx[k]] += scale * p.ColVal[k]
	}
	baseM := p.NumConstraints
	for r, dr := range s.ExtraRows {
		if dr.Dense != nil {
			dst[baseM+r] += scale * dr.Dense[colVar]
		} else {
			for k, c := range dr.ColIdx {
				if c == colVar {
					dst[baseM+r] += scale * dr.Val[k]
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
	tot := ws.TotVar

	cost = cost[:tot]
	basis := ws.Basis[:m]
	y := ws.Y[:m]
	status := ws.Status[:tot]
	dj := ws.Dj[:tot]

	for i, bVar := range basis {
		y[i] = cost[bVar]
	}
	ws.LU.BTRAN(ws.Y)

	baseM := p.NumConstraints
	colPtr := p.ColPtr[:n+1]
	rowIdx := p.RowIdx
	colVal := p.ColVal[:len(rowIdx)]
	for j := range n {
		if status[j] == statusBasic {
			dj[j] = 0
			continue
		}
		dot := 0.0
		start, end := colPtr[j], colPtr[j+1]
		rows := rowIdx[start:end]
		vals := colVal[start:end]
		for k, r := range rows {
			if uint(r) < uint(m) {
				dot += y[r] * vals[k]
			}
		}
		dj[j] = cost[j] - dot
	}
	for r, dr := range s.ExtraRows {
		yr := y[baseM+r]
		if yr <= luDropTol && yr >= -luDropTol {
			continue
		}
		cols := dr.ColIdx
		vals := dr.Val[:len(cols)]
		for k, c := range cols {
			if uint(c) < uint(tot) && status[c] != statusBasic {
				dj[c] -= yr * vals[k]
			}
		}
	}

	for i, yi := range y {
		slackVar := n + i
		if status[slackVar] == statusBasic {
			dj[slackVar] = 0
		} else {
			dj[slackVar] = cost[slackVar] - yi
		}
	}
}

func (s *LPSolver) stabilizeNonBasicBounds() bool {
	ws := s.WS
	tot := ws.TotVar
	status := ws.Status[:tot]
	lbSlice := ws.LB[:tot]
	ubSlice := ws.UB[:tot]
	djSlice := ws.Dj[:tot]
	xSlice := ws.X[:tot]
	dualFeas := true

	for k, st := range status {
		if st == statusBasic {
			continue
		}
		lb, ub, dj := lbSlice[k], ubSlice[k], djSlice[k]
		if st == statusFixed {
			xSlice[k] = lb
			continue
		}
		hasLB := !math.IsInf(lb, -1)
		hasUB := !math.IsInf(ub, 1)

		if hasLB && hasUB {
			if dj < -dualTol {
				status[k] = statusAtUpper
				xSlice[k] = ub
			} else if dj > dualTol {
				status[k] = statusAtLower
				xSlice[k] = lb
			} else {
				if st != statusAtUpper {
					status[k] = statusAtLower
					xSlice[k] = lb
				} else {
					xSlice[k] = ub
				}
			}
			continue
		}
		xSlice[k] = nonBasicValue(st, lb, ub)
		if st == statusAtLower && dj < -dualTol {
			dualFeas = false
		} else if st == statusAtUpper && dj > dualTol {
			dualFeas = false
		} else if st == statusSuperBasic && (dj > dualTol || dj < -dualTol) {
			dualFeas = false
		}
	}
	return dualFeas
}

func (s *LPSolver) isDualFeasible() bool {
	ws := s.WS
	status := ws.Status[:ws.TotVar]
	djSlice := ws.Dj[:len(status)]
	for k, st := range status {
		dj := djSlice[k]
		if (st == statusAtLower && dj < -1e-5) || (st == statusAtUpper && dj > 1e-5) || (st == statusSuperBasic && (dj > 1e-5 || dj < -1e-5)) {
			return false
		}
	}
	return true
}

func (s *LPSolver) computePrimalValues() {
	p := s.Prob
	ws := s.WS
	n := p.NumVars
	m := s.TotalRows()
	tot := ws.TotVar
	status := ws.Status[:tot]
	xSlice := ws.X[:tot]
	xbSlice := ws.XB[:m]
	basis := ws.Basis[:m]

	for i := range m {
		xb := s.rowRHS(i)
		slackVar := n + i
		if status[slackVar] != statusBasic {
			xb -= xSlice[slackVar]
		}
		xbSlice[i] = xb
	}

	baseM := p.NumConstraints
	colPtr := p.ColPtr[:n+1]
	rowIdx := p.RowIdx
	colVal := p.ColVal[:len(rowIdx)]
	for j := range n {
		if status[j] == statusBasic {
			continue
		}
		xj := xSlice[j]
		if xj == 0 {
			continue
		}
		start, end := colPtr[j], colPtr[j+1]
		rows := rowIdx[start:end]
		vals := colVal[start:end]
		for k, r := range rows {
			if uint(r) < uint(m) {
				xbSlice[r] -= vals[k] * xj
			}
		}
	}
	for r, dr := range s.ExtraRows {
		sum := 0.0
		cols := dr.ColIdx
		vals := dr.Val[:len(cols)]
		for k, c := range cols {
			if uint(c) < uint(tot) && status[c] != statusBasic {
				sum += vals[k] * xSlice[c]
			}
		}
		xbSlice[baseM+r] -= sum
	}

	ws.LU.FTRAN(ws.XB)
	for i, bVar := range basis {
		if uint(bVar) < uint(tot) {
			xSlice[bVar] = xbSlice[i]
		}
	}
}

func (s *LPSolver) ComputeTableauRow(r int) {
	p := s.Prob
	ws := s.WS
	n := p.NumVars
	m := s.TotalRows()
	tot := ws.TotVar
	rho := ws.Rho[:m]
	tabRow := ws.TabRow[:tot]
	tabMark := ws.TabRowMark[:tot]
	status := ws.Status[:tot]
	basis := ws.Basis[:m]
	numChunks := (n + 63) >> 6
	chunkMark := ws.TabRowChunkMark[:numChunks]

	if ws.RhoSparse {
		for _, i := range ws.RhoIdx {
			if uint(i) < uint(m) {
				rho[i] = 0
			}
		}
		ws.RhoIdx = ws.RhoIdx[:0]
	} else {
		clear(rho)
	}
	rho[r] = 1.0
	ws.RhoIdx = ws.LU.BTRANSparse(ws.Rho, ws.RhoIdx)
	ws.RhoSparse = true

	if ws.TabRowSparse {
		for _, k := range ws.TabRowIdx {
			if uint(k) < uint(tot) {
				tabRow[k] = 0
				tabMark[k] = false
			}
		}
		ws.TabRowIdx = ws.TabRowIdx[:0]
	} else {
		for _, ch := range ws.TabRowChunks {
			if uint(ch) < uint(numChunks) {
				chunkMark[ch] = false
				start := ch << 6
				end := min(start+64, n)
				clear(tabRow[start:end])
			}
		}
		ws.TabRowChunks = ws.TabRowChunks[:0]
		for _, k := range ws.TabRowIdx {
			if uint(k) < uint(tot) {
				tabRow[k] = 0
			}
		}
		ws.TabRowIdx = ws.TabRowIdx[:0]
	}
	baseM := p.NumConstraints

	if n > 0 && n <= DenseThreshold && len(p.DenseRows) == baseM*p.RowStride {
		ws.TabRowSparse = false
		chunks := ws.TabRowChunks[:0]
		for ch := range numChunks {
			chunkMark[ch] = true
			chunks = append(chunks, ch)
		}
		ws.TabRowChunks = chunks
		stride := p.RowStride
		tabStruct := tabRow[:n]
		sparseRowCap := n >> 2
		if simd.UsingHardwareSIMDWith(s.DisableSIMD) && n >= 32 {
			sparseRowCap = min(8, sparseRowCap)
		}
		tabIdx := ws.TabRowIdx
		for _, i := range ws.RhoIdx {
			if uint(i) >= uint(m) {
				continue
			}
			yi := rho[i]
			if i < baseM {
				start, end := p.RowPtr[i], p.RowPtr[i+1]
				if end-start <= sparseRowCap {
					cols := p.ColIdx[start:end]
					vals := p.RowVal[start:end]
					for k, col := range cols {
						if uint(col) < uint(n) {
							tabStruct[col] += yi * vals[k]
						}
					}
				} else {
					s.axpy64(tabStruct, p.DenseRows[i*stride:][:n], yi)
				}
			} else {
				dr := s.ExtraRows[i-baseM]
				if len(dr.ColIdx) <= sparseRowCap {
					cols := dr.ColIdx
					vals := dr.Val[:len(cols)]
					for k, col := range cols {
						if uint(col) < uint(n) {
							tabStruct[col] += yi * vals[k]
						}
					}
				} else if dr.Dense != nil {
					s.axpy64(tabStruct, dr.Dense[:n], yi)
				}
			}
			slack := n + i
			if uint(slack) < uint(tot) && status[slack] != statusBasic {
				tabRow[slack] = yi
				tabIdx = append(tabIdx, slack)
			}
		}
		for _, bVar := range basis {
			if uint(bVar) < uint(n) {
				tabStruct[bVar] = 0
			}
		}
		br := basis[r]
		tabRow[br] = 1.0
		if br >= n {
			tabIdx = append(tabIdx, br)
		}
		ws.TabRowIdx = tabIdx
		return
	}

	totalNNZ := 0
	for _, i := range ws.RhoIdx {
		if i < baseM {
			totalNNZ += p.RowPtr[i+1] - p.RowPtr[i]
		} else {
			totalNNZ += len(s.ExtraRows[i-baseM].ColIdx)
		}
	}

	if totalNNZ >= 64 {
		ws.TabRowSparse = false
		tabStruct := tabRow[:n]
		chunks := ws.TabRowChunks[:0]
		tabIdx := ws.TabRowIdx
		for _, i := range ws.RhoIdx {
			if uint(i) >= uint(m) {
				continue
			}
			yi := rho[i]
			var cols []int
			var vals []float64
			if i < baseM {
				start, end := p.RowPtr[i], p.RowPtr[i+1]
				cols = p.ColIdx[start:end]
				vals = p.RowVal[start:end]
			} else {
				dr := s.ExtraRows[i-baseM]
				cols = dr.ColIdx
				vals = dr.Val[:len(cols)]
			}
			cnt := len(cols)
			if cnt >= 32 {
				c0 := cols[0]
				if cols[cnt-1]-c0 == cnt-1 {
					s.axpy64(tabStruct[c0:][:cnt], vals[:cnt], yi)
					for ch := c0 >> 6; ch <= (c0+cnt-1)>>6; ch++ {
						if uint(ch) < uint(numChunks) && !chunkMark[ch] {
							chunkMark[ch] = true
							chunks = append(chunks, ch)
						}
					}
				} else if cnt >= 34 && cols[cnt-3]-c0 == cnt-3 {
					l := cnt - 2
					s.axpy64(tabStruct[c0:][:l], vals[:l], yi)
					for ch := c0 >> 6; ch <= (c0+l-1)>>6; ch++ {
						if uint(ch) < uint(numChunks) && !chunkMark[ch] {
							chunkMark[ch] = true
							chunks = append(chunks, ch)
						}
					}
					c1, c2 := cols[l], cols[l+1]
					tabStruct[c1] += yi * vals[l]
					tabStruct[c2] += yi * vals[l+1]
					if !chunkMark[c1>>6] {
						chunkMark[c1>>6] = true
						chunks = append(chunks, c1>>6)
					}
					if !chunkMark[c2>>6] {
						chunkMark[c2>>6] = true
						chunks = append(chunks, c2>>6)
					}
				} else {
					for k, col := range cols {
						if uint(col) < uint(n) {
							tabStruct[col] += yi * vals[k]
							if ch := col >> 6; uint(ch) < uint(numChunks) && !chunkMark[ch] {
								chunkMark[ch] = true
								chunks = append(chunks, ch)
							}
						}
					}
				}
			} else {
				for k, col := range cols {
					if uint(col) < uint(n) {
						tabStruct[col] += yi * vals[k]
						if ch := col >> 6; uint(ch) < uint(numChunks) && !chunkMark[ch] {
							chunkMark[ch] = true
							chunks = append(chunks, ch)
						}
					}
				}
			}
			slack := n + i
			if uint(slack) < uint(tot) && status[slack] != statusBasic {
				tabRow[slack] = yi
				tabIdx = append(tabIdx, slack)
			}
		}
		for _, bVar := range basis {
			if uint(bVar) < uint(n) && chunkMark[bVar>>6] {
				tabStruct[bVar] = 0
			}
		}
		br := basis[r]
		tabRow[br] = 1.0
		if br < n {
			if ch := br >> 6; uint(ch) < uint(numChunks) && !chunkMark[ch] {
				chunkMark[ch] = true
				chunks = append(chunks, ch)
			}
		} else {
			tabIdx = append(tabIdx, br)
		}
		ws.TabRowChunks = chunks
		ws.TabRowIdx = tabIdx
	} else {
		ws.TabRowSparse = true
		tabIdx := ws.TabRowIdx
		for _, i := range ws.RhoIdx {
			if uint(i) >= uint(m) {
				continue
			}
			yi := rho[i]
			if i < baseM {
				start, end := p.RowPtr[i], p.RowPtr[i+1]
				cols := p.ColIdx[start:end]
				vals := p.RowVal[start:end]
				for k, col := range cols {
					if uint(col) >= uint(tot) || status[col] == statusBasic {
						continue
					}
					if !tabMark[col] {
						tabMark[col] = true
						tabIdx = append(tabIdx, col)
					}
					tabRow[col] += yi * vals[k]
				}
			} else {
				dr := s.ExtraRows[i-baseM]
				cols := dr.ColIdx
				vals := dr.Val[:len(cols)]
				for k, col := range cols {
					if uint(col) >= uint(tot) || status[col] == statusBasic {
						continue
					}
					if !tabMark[col] {
						tabMark[col] = true
						tabIdx = append(tabIdx, col)
					}
					tabRow[col] += yi * vals[k]
				}
			}
			slack := n + i
			if uint(slack) < uint(tot) && status[slack] != statusBasic {
				tabMark[slack] = true
				tabIdx = append(tabIdx, slack)
				tabRow[slack] = yi
			}
		}
		br := basis[r]
		if uint(br) < uint(tot) {
			if !tabMark[br] {
				tabMark[br] = true
				tabIdx = append(tabIdx, br)
			}
			tabRow[br] = 1.0
		}
		ws.TabRowIdx = tabIdx
	}
}

func bfrtCandLess(a, b bfrtCand) bool {
	if a.ratio != b.ratio {
		return a.ratio < b.ratio
	}
	if a.absA != b.absA {
		return a.absA > b.absA
	}
	return a.col < b.col
}

func sortBFRTCands(a []bfrtCand) {
	for len(a) > 16 {
		mid := len(a) >> 1
		last := len(a) - 1
		if bfrtCandLess(a[mid], a[0]) {
			a[0], a[mid] = a[mid], a[0]
		}
		if bfrtCandLess(a[last], a[mid]) {
			a[mid], a[last] = a[last], a[mid]
			if bfrtCandLess(a[mid], a[0]) {
				a[0], a[mid] = a[mid], a[0]
			}
		}
		pivot := a[mid]
		i, j := 0, last
		for i <= j {
			for bfrtCandLess(a[i], pivot) {
				i++
			}
			for bfrtCandLess(pivot, a[j]) {
				j--
			}
			if i <= j {
				a[i], a[j] = a[j], a[i]
				i++
				j--
			}
		}
		if j < len(a)-i {
			if j > 0 {
				sortBFRTCands(a[:j+1])
			}
			a = a[i:]
		} else {
			if i < len(a)-1 {
				sortBFRTCands(a[i:])
			}
			a = a[:j+1]
		}
	}
	for i := 1; i < len(a); i++ {
		cur := a[i]
		j := i - 1
		for j >= 0 && bfrtCandLess(cur, a[j]) {
			a[j+1] = a[j]
			j--
		}
		a[j+1] = cur
	}
}

func (s *LPSolver) selectDualBFRT(primalInfeas, leaveDir float64) int {
	ws := s.WS
	cands := ws.BFRTCands
	if len(cands) == 0 {
		return -1
	}
	minIdx := 0
	for i := 1; i < len(cands); i++ {
		if bfrtCandLess(cands[i], cands[minIdx]) {
			minIdx = i
		}
	}
	c0 := cands[minIdx]
	if c0.ratio <= primalTol || c0.absA < minStaysFeasPivot {
		return -1
	}
	k0 := c0.col
	if (k0 < len(s.colHasEq) && s.colHasEq[k0]) || ws.Status[k0] == statusSuperBasic || math.IsInf(ws.LB[k0], -1) || math.IsInf(ws.UB[k0], 1) {
		return -1
	}
	maxFlip0 := c0.absA * (ws.UB[k0] - ws.LB[k0])
	if maxFlip0 <= 0 || primalInfeas-maxFlip0 <= primalTol {
		return -1
	}

	n := s.Prob.NumVars
	tot := ws.TotVar
	tabRow := ws.TabRow[:tot]
	status := ws.Status[:tot]
	dj := ws.Dj[:tot]
	cands = cands[:0]
	if !ws.TabRowSparse {
		for _, ch := range ws.TabRowChunks {
			start := ch << 6
			end := min(start+64, n)
			for k := start; k < end; k++ {
				tab := tabRow[k]
				if tab <= pivotTol && tab >= -pivotTol {
					continue
				}
				alphaBar := -leaveDir * tab
				var absA, d float64
				switch status[k] {
				case statusAtLower:
					absA = alphaBar
					d = dj[k]
				case statusAtUpper:
					absA = -alphaBar
					d = -dj[k]
				case statusSuperBasic:
					cands = append(cands, bfrtCand{col: k, ratio: 0, absA: math.Abs(alphaBar)})
					continue
				default:
					continue
				}
				if absA > pivotTol {
					d = max(0.0, d)
					cands = append(cands, bfrtCand{col: k, ratio: d / absA, absA: absA})
				}
			}
		}
	}
	for _, k := range ws.TabRowIdx {
		if uint(k) >= uint(tot) {
			continue
		}
		tab := tabRow[k]
		if tab <= pivotTol && tab >= -pivotTol {
			continue
		}
		alphaBar := -leaveDir * tab
		var absA, d float64
		switch status[k] {
		case statusAtLower:
			absA = alphaBar
			d = dj[k]
		case statusAtUpper:
			absA = -alphaBar
			d = -dj[k]
		case statusSuperBasic:
			cands = append(cands, bfrtCand{col: k, ratio: 0, absA: math.Abs(alphaBar)})
			continue
		default:
			continue
		}
		if absA > pivotTol {
			d = max(0.0, d)
			cands = append(cands, bfrtCand{col: k, ratio: d / absA, absA: absA})
		}
	}
	ws.BFRTCands = cands
	if len(cands) <= 1 {
		return -1
	}

	lbSlice := ws.LB[:tot]
	ubSlice := ws.UB[:tot]
	colHasEq := s.colHasEq
	for i := range cands {
		k := cands[i].col
		absA := cands[i].absA
		maxFlip := math.Inf(1)
		inEq := uint(k) < uint(len(colHasEq)) && colHasEq[k]
		if !inEq && absA >= 1e-5 && uint(k) < uint(tot) && status[k] != statusSuperBasic {
			lbk, ubk := lbSlice[k], ubSlice[k]
			if !math.IsInf(lbk, -1) && !math.IsInf(ubk, 1) {
				maxFlip = absA * (ubk - lbk)
			}
		}
		cands[i].maxFlip = maxFlip
	}

	sortBFRTCands(cands)

	remInfeas := primalInfeas
	stopIdx := 0
	for stopIdx < len(cands)-1 && len(ws.FlipCols) < 64 {
		c := cands[stopIdx]
		if math.IsInf(c.maxFlip, 1) || c.maxFlip <= 0 || c.absA < minStaysFeasPivot || remInfeas-c.maxFlip <= primalTol {
			break
		}
		ws.FlipCols = append(ws.FlipCols, c.col)
		remInfeas -= c.maxFlip
		stopIdx++
	}
	if len(ws.FlipCols) == 0 {
		return -1
	}

	thetaLim := math.Inf(1)
	for i := stopIdx; i < len(cands); i++ {
		hr := cands[i].ratio + harrisTol/cands[i].absA
		if hr < thetaLim {
			thetaLim = hr
		}
	}

	enterVar := -1
	enterRatio := 0.0
	bestAbs := 0.0
	bestStaysFeas := false
	for i := stopIdx; i < len(cands); i++ {
		c := cands[i]
		if c.ratio > thetaLim+minScaledPrimalTol {
			break
		}
		staysFeas := c.absA >= minStaysFeasPivot && remInfeas <= c.maxFlip+primalTol
		if staysFeas && !bestStaysFeas {
			bestStaysFeas = true
			bestAbs = c.absA
			enterVar = c.col
			enterRatio = c.ratio
		} else if staysFeas == bestStaysFeas && (c.absA > bestAbs || (c.absA == bestAbs && c.col < enterVar)) {
			bestAbs = c.absA
			enterVar = c.col
			enterRatio = c.ratio
		}
	}
	if enterVar == -1 || bestAbs < minStaysFeasPivot {
		ws.FlipCols = ws.FlipCols[:0]
		return -1
	}
	// Only flip variables whose breakpoint is strictly before enterRatio so every
	// flipped variable moves strictly into the dual-feasible interior.
	keep := 0
	for i := 0; i < len(ws.FlipCols); i++ {
		if cands[i].ratio < enterRatio-pivotTieRelTol {
			ws.FlipCols[keep] = ws.FlipCols[i]
			keep++
		}
	}
	ws.FlipCols = ws.FlipCols[:keep]
	if keep == 0 {
		return -1
	}
	return enterVar
}

func (s *LPSolver) recomputeRowBasicDeg() {
	p := s.Prob
	ws := s.WS
	n := p.NumVars
	m := s.TotalRows()
	rowDeg := ws.RowBasicDeg[:m]
	clear(rowDeg)
	basis := ws.Basis[:m]
	colPtr := p.ColPtr[:n+1]
	rowIdx := p.RowIdx
	for _, bVar := range basis {
		if uint(bVar) < uint(n) {
			for _, r := range rowIdx[colPtr[bVar]:colPtr[bVar+1]] {
				rowDeg[r]++
			}
		}
	}
}

func (s *LPSolver) colBasisLoad(k int, dynamic bool) int {
	p := s.Prob
	if uint(k) >= uint(p.NumVars) {
		return 0
	}
	start, end := p.ColPtr[k], p.ColPtr[k+1]
	if !dynamic {
		return end - start
	}
	rowDeg := s.WS.RowBasicDeg
	load := 0
	for _, r := range p.RowIdx[start:end] {
		load += rowDeg[r]
	}
	return load
}

func (s *LPSolver) runDualSimplexLoop(cost []float64) (Status, int) {
	p := s.Prob
	ws := s.WS
	n := p.NumVars
	m := s.TotalRows()
	tot := ws.TotVar
	s.recomputeRowBasicDeg()

	for iter := range s.MaxIter {
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

		basis := ws.Basis[:m]
		xb := ws.XB[:m]
		dse := ws.DSE[:m]
		lb := ws.LB[:tot]
		ub := ws.UB[:tot]
		colTol := s.colTol
		for r, bVar := range basis {
			val := xb[r]
			lbr, ubr := lb[bVar], ub[bVar]
			tol := primalTol
			if uint(bVar) < uint(len(colTol)) {
				tol = colTol[bVar]
			}
			var viol, dir float64
			if val < lbr-tol {
				viol = lbr - val
				dir = 1.0
			} else if val > ubr+tol {
				viol = val - ubr
				dir = -1.0
			}
			if viol > 0 {
				w := dse[r]
				if !(w >= minDSEWeight && w <= maxDSEWeight) {
					w = 1.0
					dse[r] = 1.0
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
		cands := ws.BFRTCands[:0]
		tabRow := ws.TabRow[:tot]
		status := ws.Status[:tot]
		dj := ws.Dj[:tot]
		if !ws.TabRowSparse {
			for _, ch := range ws.TabRowChunks {
				start := ch << 6
				end := min(start+64, n)
				for k := start; k < end; k++ {
					tab := tabRow[k]
					if tab <= pivotTol && tab >= -pivotTol {
						continue
					}
					alphaBar := -leaveDir * tab
					var absA, d float64
					switch status[k] {
					case statusAtLower:
						absA = alphaBar
						d = dj[k]
					case statusAtUpper:
						absA = -alphaBar
						d = -dj[k]
					case statusSuperBasic:
						thetaMax = 0
						cands = append(cands, bfrtCand{col: k, ratio: 0, absA: math.Abs(alphaBar)})
						continue
					default:
						continue
					}
					if absA > pivotTol {
						d = max(0.0, d)
						if d <= (thetaMax+2*minScaledPrimalTol)*absA {
							if ratio := (d + harrisTol) / absA; ratio < thetaMax {
								thetaMax = ratio
							}
							cands = append(cands, bfrtCand{col: k, ratio: d / absA, absA: absA})
						}
					}
				}
			}
		}
		for _, k := range ws.TabRowIdx {
			if uint(k) >= uint(tot) {
				continue
			}
			tab := tabRow[k]
			if tab <= pivotTol && tab >= -pivotTol {
				continue
			}
			alphaBar := -leaveDir * tab
			var absA, d float64
			switch status[k] {
			case statusAtLower:
				absA = alphaBar
				d = dj[k]
			case statusAtUpper:
				absA = -alphaBar
				d = -dj[k]
			case statusSuperBasic:
				thetaMax = 0
				cands = append(cands, bfrtCand{col: k, ratio: 0, absA: math.Abs(alphaBar)})
				continue
			default:
				continue
			}
			if absA > pivotTol {
				d = max(0.0, d)
				if d <= (thetaMax+2*minScaledPrimalTol)*absA {
					if ratio := (d + harrisTol) / absA; ratio < thetaMax {
						thetaMax = ratio
					}
					cands = append(cands, bfrtCand{col: k, ratio: d / absA, absA: absA})
				}
			}
		}
		ws.BFRTCands = cands

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
		bestLoad := 0
		bestStaysFeas := false
		dynLoad := thetaMax > harrisTol && iter < 3*m
		thetaLim := thetaMax + minScaledPrimalTol
		scalePiv := n > 100
		for _, c := range cands {
			if c.ratio <= thetaLim {
				k := c.col
				absA := c.absA
				staysFeas := absA >= minStaysFeasPivot && primalInfeas <= absA*(ub[k]-lb[k])+primalTol
				pivScore := absA
				if scalePiv {
					pivScore = absA * s.feasTol(k)
				}
				if enterVar == -1 || (staysFeas && !bestStaysFeas) {
					bestStaysFeas = staysFeas
					bestPivAbs = pivScore
					bestLoad = s.colBasisLoad(k, dynLoad)
					enterVar = k
				} else if staysFeas == bestStaysFeas {
					if pivScore > bestPivAbs*(1.0+pivotTieRelTol) {
						bestPivAbs = pivScore
						bestLoad = s.colBasisLoad(k, dynLoad)
						enterVar = k
					} else if pivScore >= bestPivAbs*(1.0-pivotTieRelTol) {
						load := s.colBasisLoad(k, dynLoad)
						if load < bestLoad || (load == bestLoad && k < enterVar) {
							bestPivAbs = pivScore
							bestLoad = load
							enterVar = k
						}
					}
				}
			}
		}

		if enterVar == -1 {
			return StatusInfeasible, iter
		}

		ws.FlipCols = ws.FlipCols[:0]
		if !bestStaysFeas && thetaMax > harrisTol && !math.IsInf(lb[enterVar], -1) && !math.IsInf(ub[enterVar], 1) {
			if bfrtEnter := s.selectDualBFRT(primalInfeas, leaveDir); bfrtEnter != -1 {
				enterVar = bfrtEnter
			}
		}

		if ws.FColSparse {
			for _, i := range ws.FColIdx {
				ws.FCol[i] = 0
			}
			ws.FColIdx = ws.FColIdx[:0]
		} else {
			clear(ws.FCol)
		}
		s.fillColumn(enterVar, ws.FCol)
		ws.FColIdx = ws.LU.FTRANSparse(ws.FCol, ws.FColIdx)
		ws.FColSparse = true

		pivElem := ws.FCol[leaveRow]
		if pivElem < minDualPivotElem && pivElem > -minDualPivotElem {
			if err := s.factorizeBasis(); err != nil {
				return StatusNumericalError, iter
			}
			s.computeDualsAndReducedCosts(cost)
			s.computePrimalValues()
			continue
		}

		if len(ws.FlipCols) > 0 {
			clear(ws.Tau)
			for _, fk := range ws.FlipCols {
				lbf, ubf := lb[fk], ub[fk]
				var deltaX float64
				if status[fk] == statusAtLower {
					status[fk] = statusAtUpper
					ws.X[fk] = ubf
					deltaX = ubf - lbf
				} else {
					status[fk] = statusAtLower
					ws.X[fk] = lbf
					deltaX = lbf - ubf
				}
				s.addScaledColumn(fk, deltaX, ws.Tau)
			}
			ws.TauIdx = ws.LU.FTRANSparse(ws.Tau, ws.TauIdx)
			for _, i := range ws.TauIdx {
				xb[i] -= ws.Tau[i]
				ws.X[basis[i]] = xb[i]
			}
			bVar := basis[leaveRow]
			if leaveDir > 0 {
				primalInfeas = max(0.0, lb[bVar]-xb[leaveRow])
			} else {
				primalInfeas = max(0.0, xb[leaveRow]-ub[bVar])
			}
		}

		invPiv := 1.0 / pivElem
		invPiv2 := invPiv * invPiv
		leaveWeight := dse[leaveRow]
		newLeaveWeight := leaveWeight * invPiv2
		if !(newLeaveWeight >= minDSEWeight && newLeaveWeight <= maxDSEWeight) {
			newLeaveWeight = min(max(newLeaveWeight, minDSEWeight), maxDSEWeight)
			if math.IsNaN(newLeaveWeight) {
				newLeaveWeight = 1.0
			}
		}
		fCol := ws.FCol[:m]
		if len(ws.FColIdx) > 1 {
			tau := ws.Tau[:m]
			clear(tau)
			if ws.RhoSparse {
				rho := ws.Rho[:m]
				for _, i := range ws.RhoIdx {
					tau[i] = rho[i]
				}
			} else {
				copy(tau, ws.Rho[:m])
			}
			ws.LU.FTRAN(ws.Tau)
			for _, i := range ws.FColIdx {
				if i != leaveRow && uint(i) < uint(m) {
					fi := fCol[i]
					if fi > minDSEColEntry || fi < -minDSEColEntry {
						ratio := fi * invPiv
						w := dse[i] - 2.0*ratio*tau[i] + ratio*ratio*leaveWeight
						if !(w >= minDSEWeight && w <= maxDSEWeight) {
							w = min(max(w, minDSEWeight), maxDSEWeight)
							if math.IsNaN(w) {
								w = 1.0
							}
						}
						dse[i] = w
					}
				}
			}
		}
		dse[leaveRow] = newLeaveWeight

		leaveVar := basis[leaveRow]
		var newLeaveVal float64
		if leaveDir > 0 {
			status[leaveVar] = statusAtLower
			newLeaveVal = lb[leaveVar]
		} else {
			status[leaveVar] = statusAtUpper
			newLeaveVal = ub[leaveVar]
		}
		if lb[leaveVar] == ub[leaveVar] {
			status[leaveVar] = statusFixed
		}

		primalStep := -leaveDir * primalInfeas / pivElem
		oldEnterVal := ws.X[enterVar]
		newEnterVal := oldEnterVal + primalStep

		xSlice := ws.X[:tot]
		for _, i := range ws.FColIdx {
			if i != leaveRow && uint(i) < uint(m) {
				xb[i] -= fCol[i] * primalStep
				xSlice[basis[i]] = xb[i]
			}
		}
		xb[leaveRow] = newEnterVal
		xSlice[leaveVar] = newLeaveVal
		xSlice[enterVar] = newEnterVal

		rowDeg := ws.RowBasicDeg[:m]
		if leaveVar < n {
			for _, r := range p.RowIdx[p.ColPtr[leaveVar]:p.ColPtr[leaveVar+1]] {
				rowDeg[r]--
			}
		}
		if enterVar < n {
			for _, r := range p.RowIdx[p.ColPtr[enterVar]:p.ColPtr[enterVar+1]] {
				rowDeg[r]++
			}
		}
		basis[leaveRow] = enterVar
		ws.BasicRow[leaveVar] = -1
		ws.BasicRow[enterVar] = leaveRow
		status[enterVar] = statusBasic

		if err := ws.LU.PushEtaSparse(leaveRow, ws.FCol, ws.FColIdx); err != nil {
			if err := s.factorizeBasis(); err != nil {
				return StatusNumericalError, iter
			}
			s.computeDualsAndReducedCosts(cost)
			s.computePrimalValues()
		} else {
			dualStep := dj[enterVar] / tabRow[enterVar]
			if dualStep != 0 {
				if ws.RhoSparse {
					y := ws.Y[:m]
					rho := ws.Rho[:m]
					for _, i := range ws.RhoIdx {
						y[i] += dualStep * rho[i]
					}
				} else {
					s.axpy64(ws.Y[:m], ws.Rho[:m], dualStep)
				}
				if ws.TabRowSparse {
					for _, k := range ws.TabRowIdx {
						dj[k] -= dualStep * tabRow[k]
					}
				} else {
					for _, ch := range ws.TabRowChunks {
						start := ch << 6
						end := min(start+64, n)
						s.axpy64(dj[start:end], tabRow[start:end], -dualStep)
					}
					for _, k := range ws.TabRowIdx {
						if status[k] != statusBasic {
							dj[k] -= dualStep * tabRow[k]
						}
					}
				}
			}
			dj[enterVar] = 0
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
		anyInfeasible := false
		for i := range m {
			bVar := ws.Basis[i]
			val := ws.XB[i]
			tol := s.feasTol(bVar)
			if val < ws.LB[bVar]-tol {
				ws.WorkCost[bVar] = -1.0
				anyInfeasible = true
			} else if val > ws.UB[bVar]+tol {
				ws.WorkCost[bVar] = 1.0
				anyInfeasible = true
			}
		}

		if !anyInfeasible {
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
			if val < ws.LB[bVar]-phase1RelaxTol {
				maxViol = max(maxViol, ws.LB[bVar]-val)
			} else if val > ws.UB[bVar]+phase1RelaxTol {
				maxViol = max(maxViol, val-ws.UB[bVar])
			}
		}
		if maxViol > phase1RelaxTol {
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
	ws.FColSparse = false
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
			tol := s.feasTol(bVar)
			if val < lb-tol {
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
			} else if val > ub+tol {
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
				// An improving phase-1 ray that no infeasible basic variable blocks
				// cannot occur in exact arithmetic (an infeasible basic would block at its
				// bound); it signals pivots below tolerance, not a certificate of an empty polyhedron.
				return StatusNumericalError
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
