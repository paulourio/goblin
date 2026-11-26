package goblin

import (
	"sync"
	"sync/atomic"
)

type varBasisStatus uint8

const (
	statusAtLower varBasisStatus = iota
	statusAtUpper
	statusBasic
	statusFixed
	statusSuperBasic
)

type bfrtCand struct {
	col     int
	ratio   float64
	absA    float64
	maxFlip float64
}

type dualShiftEntry struct {
	col    int
	origLB float64
	origUB float64
	shiftL bool
	shiftU bool
}

const (
	defaultLUInitCap      = 64
	tabRowChunkBits       = 6
	tabRowChunkSize       = 1 << tabRowChunkBits
	tabRowChunkMask       = tabRowChunkSize - 1
	maxPooledWorkspaceCap = 1 << 18
)

// Workspace holds all preallocated numerical buffers for an LP solve or
// warm-started Branch-and-Cut node re-optimization, eliminating GC allocations.
// Callers borrow instances via AcquireWorkspace and return them with ReleaseWorkspace.
type Workspace struct {
	lu *LUFactorization

	// Active dimensions (TotVar = N + M).
	n      int
	m      int
	totVar int

	// Working bounds and costs.
	lb       []float64
	ub       []float64
	cost     []float64
	workCost []float64
	pertCost []float64

	// Basis indices: Basis[r] stores slot r's variable, while BasicRow[k] maps variable k back to its basis row or -1 when nonbasic.
	basis       []int
	basicRow    []int
	rowBasicDeg []int
	status      []varBasisStatus
	basicLB     []float64
	basicUB     []float64
	basicTol    []float64

	// Primal, dual, reduced-cost, and steepest-edge vectors.
	x           []float64
	xb          []float64
	y           []float64
	dj          []float64
	dse         []float64
	devexWeight []float64
	devexRef    []uint8

	// Sparse linear-system scratch buffers: whenever a *Sparse flag is true, every nonzero entry of the paired slice is recorded inside its *Idx support list so subsequent clears touch only active positions.
	rho             []float64
	rhoIdx          []int
	rhoSparse       bool
	fCol            []float64
	fColIdx         []int
	fColSparse      bool
	tau             []float64
	tauIdx          []int
	tauSparse       bool
	tabRow          []float64
	tabRowErr       []float64
	tabRowIdx       []int
	tabRowMark      []bool
	tabRowSparse    bool
	tabRowChunks    []int
	tabRowChunkMark []bool

	// Ratio-test and residual-tracking scratch buffers.
	flipCols   []int
	bfrtCands  []bfrtCand
	dualShift  []dualShiftEntry
	preShiftCP lpCheckpoint
	sbCP       lpCheckpoint
	rowMaxTerm []float64
	varTol     []float64

	// Lazy refinement, exact-tableau, and residual-verification scratch buffers shared sequentially across post-solve checks.
	dotAccBuf     []dot2Acc
	refineBuf1    []float64
	refineBuf2    []float64
	refineBuf3    []float64
	evalDJOrig    []float64
	evalDJErr     []float64
	evalDY2Sc     []float64
	evalYDotSc    []float64
	evalRowRes    []float64
	dualRefY0     []float64
	dualRefDY1    []float64
	dualRefDY2    []float64
	dualRefTmp    []float64
	dualRefMax    []float64
	dualRefParent []int
	dualRefComp   []int
	dualRefRows   []int
	dualRefVals   []float64
	crashEqRows   []int
	crashRowDone  []bool
	crashRowLoad  []int

	inPool atomic.Bool
}

var workspacePool = sync.Pool{
	New: func() any {
		return &Workspace{
			lu: newLUFactorization(defaultLUInitCap),
		}
	},
}

// AcquireWorkspace retrieves a Workspace from the pool and sizes it for (n, m).
func AcquireWorkspace(n, m int) *Workspace {
	n = max(0, n)
	m = max(0, m)
	ws := workspacePool.Get().(*Workspace)
	ws.inPool.Store(false)
	ws.ensureSize(n, m)
	return ws
}

// ReleaseWorkspace returns a Workspace to the pool unless its buffers exceed maxPooledWorkspaceCap.
// Releasing a nil or already-pooled Workspace is a safe no-op.
func ReleaseWorkspace(ws *Workspace) {
	if ws == nil || cap(ws.x) > maxPooledWorkspaceCap {
		return
	}
	if !ws.inPool.CompareAndSwap(false, true) {
		return
	}
	workspacePool.Put(ws)
}

func (ws *Workspace) ensureSize(n, m int) {
	n = max(0, n)
	m = max(0, m)
	if ws.lu == nil {
		ws.lu = newLUFactorization(max(defaultLUInitCap, m))
	}
	tot := n + m
	ws.n = n
	ws.m = m
	ws.totVar = tot

	ws.lb = resizeZero(ws.lb, tot)
	ws.ub = resizeZero(ws.ub, tot)
	ws.cost = resizeZero(ws.cost, tot)
	ws.workCost = resizeZero(ws.workCost, tot)

	ws.basis = resizeZero(ws.basis, m)
	ws.basicRow = resizeZero(ws.basicRow, tot)
	ws.rowBasicDeg = resizeZero(ws.rowBasicDeg, m)
	ensureBuf(&ws.status, tot, false)
	ws.basicLB = resizeZero(ws.basicLB, m)
	ws.basicUB = resizeZero(ws.basicUB, m)
	ws.basicTol = resizeZero(ws.basicTol, m)

	ws.x = resizeZero(ws.x, tot)
	ws.xb = resizeZero(ws.xb, m)
	ws.y = resizeZero(ws.y, m)
	ws.dj = resizeZero(ws.dj, tot)
	ws.dse = resizeZero(ws.dse, m)

	ws.rho = resizeZero(ws.rho, m)
	ws.rhoIdx = ws.rhoIdx[:0]
	ws.rhoSparse = false
	ws.fCol = resizeZero(ws.fCol, m)
	ws.fColIdx = ws.fColIdx[:0]
	ws.fColSparse = false
	ws.tau = resizeZero(ws.tau, m)
	ws.tauIdx = ws.tauIdx[:0]
	ws.tauSparse = false
	ws.tabRow = resizeZero(ws.tabRow, tot)
	ws.tabRowErr = resizeZero(ws.tabRowErr, tot)
	ws.tabRowMark = resizeZero(ws.tabRowMark, tot)
	ws.tabRowIdx = resetCap(ws.tabRowIdx, tot)
	ws.tabRowSparse = false
	numChunks := (n + tabRowChunkMask) >> tabRowChunkBits
	ws.tabRowChunkMark = resizeZero(ws.tabRowChunkMark, numChunks)
	ws.tabRowChunks = resetCap(ws.tabRowChunks, numChunks)
	ws.flipCols = ws.flipCols[:0]
	ws.bfrtCands = resetCap(ws.bfrtCands, tot)
	ws.dualShift = ws.dualShift[:0]
	ws.rowMaxTerm = resizeZero(ws.rowMaxTerm, m)
	ensureBuf(&ws.varTol, tot, false)
}

func (ws *Workspace) ensureDotAcc(size int) []dot2Acc {
	if cap(ws.dotAccBuf) < size {
		ws.dotAccBuf = make([]dot2Acc, max(size, max(ws.n, ws.m)))
		return ws.dotAccBuf[:size]
	}
	buf := ws.dotAccBuf[:size]
	clear(buf)
	return buf
}

func ensureBuf[T any](buf *[]T, n int, zero bool) []T {
	if cap(*buf) < n {
		*buf = make([]T, n)
		return *buf
	}
	s := (*buf)[:n]
	if zero {
		clear(s)
	}
	return s
}

func resizeZero[T any](s []T, n int) []T {
	if cap(s) < n {
		return make([]T, n)
	}
	s = s[:n]
	clear(s)
	return s
}

func resetCap[T any](s []T, minCap int) []T {
	if cap(s) < minCap {
		return make([]T, 0, minCap)
	}
	return s[:0]
}
