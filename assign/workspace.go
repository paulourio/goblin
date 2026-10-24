package assign

import "sync"

const (
	defaultDimCap = 64
	maxPooledDim  = 4096
	maxPooledCost = maxPooledDim * maxPooledDim
)

// Workspace holds reusable solver buffers for the Linear Sum Assignment solver.
// Callers may acquire a pooled Workspace via AcquireWorkspace and return it via
// ReleaseWorkspace to eliminate per-solve allocations in hot loops.
type Workspace struct {
	u         []float64
	v         []float64
	d         []float64
	pred      []int
	col4row   []int
	row4col   []int
	remaining []int
	freeRows  []int
	auxRows   []int
	workCost  []float64
	opts      Options
}

var workspacePool = sync.Pool{
	New: func() any {
		return &Workspace{
			u:         make([]float64, 0, defaultDimCap),
			v:         make([]float64, 0, defaultDimCap),
			d:         make([]float64, 0, defaultDimCap),
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
	m := min(rows, cols)
	n := max(rows, cols)
	if m > 0 && n > 0 {
		ws.ensure(m, n)
	}
	return ws
}

// ReleaseWorkspace returns ws to the pool if its capacity is within the pooling bound.
func ReleaseWorkspace(ws *Workspace) {
	if ws == nil || cap(ws.v) > maxPooledDim {
		return
	}
	if cap(ws.workCost) > maxPooledCost {
		ws.workCost = nil
	}
	workspacePool.Put(ws)
}

func (ws *Workspace) ensure(m, n int) {
	ws.u = growFloat64(ws.u, m)
	ws.v = growFloat64(ws.v, n)
	ws.d = growFloat64(ws.d, n)
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
