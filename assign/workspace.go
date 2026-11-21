package assign

import (
	"sync"
	"sync/atomic"
)

const (
	defaultDimCap     = 64
	maxPooledDim      = 4096
	maxPooledCost     = maxPooledDim * maxPooledDim
	maxPooledCoreCols = maxPooledDim * 32
)

// Workspace holds reusable solver buffers for the Linear Sum Assignment solver.
// Callers may acquire a pooled Workspace via AcquireWorkspace and return it via
// ReleaseWorkspace to eliminate per-solve allocations in hot loops.
type Workspace struct {
	u           []float64
	v           []float64
	d           []float64
	dQueue      []float64
	pred        []int
	col4row     []int
	row4col     []int
	remaining   []int
	freeRows    []int
	auxRows     []int
	workCost    []float64
	coreCol     []int32
	coreCost    []float64
	coreDeg     []int32
	dStamp      []int32
	closedStamp []int32
	heapCol     []int32
	heapDist    []float64
	stamp       int32
	opts        Options
	inPool      atomic.Bool
}

var workspacePool = sync.Pool{
	New: func() any {
		return &Workspace{
			u:         make([]float64, 0, defaultDimCap),
			v:         make([]float64, 0, defaultDimCap),
			d:         make([]float64, 0, defaultDimCap),
			dQueue:    make([]float64, 0, defaultDimCap),
			pred:      make([]int, 0, defaultDimCap),
			col4row:   make([]int, 0, defaultDimCap),
			row4col:   make([]int, 0, defaultDimCap),
			remaining: make([]int, 0, defaultDimCap),
			freeRows:  make([]int, 0, defaultDimCap),
			auxRows:   make([]int, 0, defaultDimCap),
		}
	},
}

// AcquireWorkspace returns a pooled Workspace sized for at least rows and cols.
func AcquireWorkspace(rows, cols int) *Workspace {
	ws := workspacePool.Get().(*Workspace)
	ws.inPool.Store(false)
	m := min(rows, cols)
	n := max(rows, cols)
	if m > 0 && n > 0 {
		ws.ensure(m, n)
	}
	return ws
}

// ReleaseWorkspace returns ws to the pool if its capacity is within the pooling bound.
// Releasing a nil or already-pooled Workspace is a safe no-op.
func ReleaseWorkspace(ws *Workspace) {
	if ws == nil || cap(ws.v) > maxPooledDim {
		return
	}
	if !ws.inPool.CompareAndSwap(false, true) {
		return
	}
	if cap(ws.workCost) > maxPooledCost {
		ws.workCost = nil
	}
	if cap(ws.coreCol) > maxPooledCoreCols {
		ws.coreCol = nil
		ws.coreCost = nil
		ws.heapCol = nil
		ws.heapDist = nil
	}
	workspacePool.Put(ws)
}

func (ws *Workspace) ensureSparseCore(n, k int) {
	need := n * k
	ws.coreCol = growInt32(ws.coreCol, need)
	ws.coreCost = growFloat64(ws.coreCost, need)
	ws.coreDeg = growInt32(ws.coreDeg, n)
	if cap(ws.dStamp) < n {
		ws.dStamp = make([]int32, n)
		ws.closedStamp = make([]int32, n)
		ws.stamp = 0
	} else {
		oldLen := len(ws.dStamp)
		ws.dStamp = ws.dStamp[:n]
		ws.closedStamp = ws.closedStamp[:n]
		if oldLen < n {
			clear(ws.dStamp[oldLen:n])
			clear(ws.closedStamp[oldLen:n])
		}
	}
	initHeapCap := min(need, 4*n)
	if cap(ws.heapCol) < initHeapCap {
		ws.heapCol = make([]int32, initHeapCap)
		ws.heapDist = make([]float64, initHeapCap)
	}
}

func (ws *Workspace) ensure(m, n int) {
	ws.u = growFloat64(ws.u, m)
	ws.v = growFloat64(ws.v, n)
	ws.d = growFloat64(ws.d, n)
	ws.dQueue = growFloat64(ws.dQueue, n)
	ws.pred = growInt(ws.pred, n)
	ws.col4row = growInt(ws.col4row, m)
	ws.row4col = growInt(ws.row4col, n)
	ws.remaining = growInt(ws.remaining, n)
	ws.freeRows = growInt(ws.freeRows, m)
	ws.auxRows = growInt(ws.auxRows, m)
}

func (ws *Workspace) ensureWorkCost(size int) []float64 {
	ws.workCost = growFloat64(ws.workCost, size)
	return ws.workCost
}

func (ws *Workspace) applyOptions(opts []Option) Options {
	if len(opts) == 0 {
		return Options{}
	}
	ws.opts = Options{}
	for _, fn := range opts {
		if fn != nil {
			fn(&ws.opts)
		}
	}
	cfg := ws.opts
	ws.opts = Options{}
	return cfg
}

func growFloat64(s []float64, n int) []float64 {
	if cap(s) < n {
		return make([]float64, n)
	}
	return s[:n]
}

func growInt(s []int, n int) []int {
	if cap(s) < n {
		return make([]int, n)
	}
	return s[:n]
}

func growInt32(s []int32, n int) []int32 {
	if cap(s) < n {
		return make([]int32, n)
	}
	return s[:n]
}
