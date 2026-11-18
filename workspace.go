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

	// Basis indices: Basis[r] stores slot r's variable, while BasicRow[k] maps variable k back to its basis row or -1 when nonbasic.
	basis       []int
	basicRow    []int
	rowBasicDeg []int
	status      []varBasisStatus

	// Primal, dual, reduced-cost, and steepest-edge vectors.
	x   []float64
	xb  []float64
	y   []float64
	dj  []float64
	dse []float64

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
	tabRowIdx       []int
	tabRowMark      []bool
	tabRowSparse    bool
	tabRowChunks    []int
	tabRowChunkMark []bool

	// Ratio-test scratch buffers.
	flipCols  []int
	bfrtCands []bfrtCand
	inPool    atomic.Bool
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
	tot := n + m
	ws.n = n
	ws.m = m
	ws.totVar = tot

	ws.lb = resizeFloat64(ws.lb, tot)
	ws.ub = resizeFloat64(ws.ub, tot)
	ws.cost = resizeFloat64(ws.cost, tot)
	ws.workCost = resizeFloat64(ws.workCost, tot)

	ws.basis = resizeInt(ws.basis, m)
	ws.basicRow = resizeInt(ws.basicRow, tot)
	ws.rowBasicDeg = resizeInt(ws.rowBasicDeg, m)
	if cap(ws.status) < tot {
		ws.status = make([]varBasisStatus, tot)
	} else {
		ws.status = ws.status[:tot]
	}

	ws.x = resizeFloat64(ws.x, tot)
	ws.xb = resizeFloat64(ws.xb, m)
	ws.y = resizeFloat64(ws.y, m)
	ws.dj = resizeFloat64(ws.dj, tot)
	ws.dse = resizeFloat64(ws.dse, m)

	ws.rho = resizeFloat64(ws.rho, m)
	ws.rhoIdx = ws.rhoIdx[:0]
	ws.rhoSparse = false
	ws.fCol = resizeFloat64(ws.fCol, m)
	ws.fColIdx = ws.fColIdx[:0]
	ws.fColSparse = false
	ws.tau = resizeFloat64(ws.tau, m)
	ws.tauIdx = ws.tauIdx[:0]
	ws.tauSparse = false
	ws.tabRow = resizeFloat64(ws.tabRow, tot)
	if cap(ws.tabRowMark) < tot {
		ws.tabRowMark = make([]bool, tot)
	} else {
		ws.tabRowMark = ws.tabRowMark[:tot]
		clear(ws.tabRowMark)
	}
	ws.tabRowIdx = ws.tabRowIdx[:0]
	ws.tabRowSparse = false
	numChunks := (n + tabRowChunkMask) >> tabRowChunkBits
	if cap(ws.tabRowChunkMark) < numChunks {
		ws.tabRowChunkMark = make([]bool, numChunks)
		ws.tabRowChunks = make([]int, 0, numChunks)
	} else {
		ws.tabRowChunkMark = ws.tabRowChunkMark[:numChunks]
		clear(ws.tabRowChunkMark)
		ws.tabRowChunks = ws.tabRowChunks[:0]
	}
	ws.flipCols = ws.flipCols[:0]
	ws.bfrtCands = ws.bfrtCands[:0]
}

func resizeFloat64(s []float64, n int) []float64 {
	if cap(s) < n {
		return make([]float64, n)
	}
	s = s[:n]
	clear(s)
	return s
}

func resizeInt(s []int, n int) []int {
	if cap(s) < n {
		return make([]int, n)
	}
	s = s[:n]
	clear(s)
	return s
}
