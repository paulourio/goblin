package gopt

import "sync"

type varBasisStatus uint8

const (
	statusAtLower varBasisStatus = iota
	statusAtUpper
	statusBasic
	statusFixed
	statusSuperBasic
)

// Workspace holds all preallocated numerical buffers for an LP solve or
// warm-started Branch-and-Cut node re-optimization, eliminating GC allocations.
type Workspace struct {
	LU *LUFactorization

	N      int
	M      int
	TotVar int

	LB       []float64
	UB       []float64
	Cost     []float64
	WorkCost []float64

	Basis    []int
	BasicRow []int
	Status   []varBasisStatus

	X   []float64
	XB  []float64
	Y   []float64
	Dj  []float64
	DSE []float64

	Rho          []float64
	FCol         []float64
	Tau          []float64
	TabRow       []float64
	TabRowIdx    []int
	TabRowMark   []bool
	TabRowSparse bool
}

var workspacePool = sync.Pool{
	New: func() any {
		return &Workspace{
			LU: newLUFactorization(64),
		}
	},
}

// AcquireWorkspace retrieves a Workspace from the pool and sizes it for (n, m).
func AcquireWorkspace(n, m int) *Workspace {
	ws := workspacePool.Get().(*Workspace)
	ws.EnsureSize(n, m)
	return ws
}

// ReleaseWorkspace returns a Workspace to the pool.
func ReleaseWorkspace(ws *Workspace) {
	if ws != nil {
		workspacePool.Put(ws)
	}
}

// EnsureSize resizes all slices in ws to accommodate n structural variables and m constraints.
func (ws *Workspace) EnsureSize(n, m int) {
	tot := n + m
	ws.N = n
	ws.M = m
	ws.TotVar = tot

	ws.LB = resizeFloat64(ws.LB, tot)
	ws.UB = resizeFloat64(ws.UB, tot)
	ws.Cost = resizeFloat64(ws.Cost, tot)
	ws.WorkCost = resizeFloat64(ws.WorkCost, tot)

	ws.Basis = resizeInt(ws.Basis, m)
	ws.BasicRow = resizeInt(ws.BasicRow, tot)
	if cap(ws.Status) < tot {
		ws.Status = make([]varBasisStatus, tot)
	} else {
		ws.Status = ws.Status[:tot]
	}

	ws.X = resizeFloat64(ws.X, tot)
	ws.XB = resizeFloat64(ws.XB, m)
	ws.Y = resizeFloat64(ws.Y, m)
	ws.Dj = resizeFloat64(ws.Dj, tot)
	ws.DSE = resizeFloat64(ws.DSE, m)

	ws.Rho = resizeFloat64(ws.Rho, m)
	ws.FCol = resizeFloat64(ws.FCol, m)
	ws.Tau = resizeFloat64(ws.Tau, m)
	ws.TabRow = resizeFloat64(ws.TabRow, tot)
	if cap(ws.TabRowMark) < tot {
		ws.TabRowMark = make([]bool, tot)
	} else {
		ws.TabRowMark = ws.TabRowMark[:tot]
		clear(ws.TabRowMark)
	}
	ws.TabRowIdx = ws.TabRowIdx[:0]
	ws.TabRowSparse = false
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
