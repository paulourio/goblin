//go:build cgo && cbc

package goblin_test

import (
	"testing"
	"time"

	gb "github.com/paulourio/goblin"
)

func compareWithOptionalCBC(tb testing.TB, label string, p *gb.CompiledProblem, resGo gb.Result) {
	tb.Helper()
	if resGo.Status != gb.StatusOptimal && resGo.Status != gb.StatusInfeasible {
		return
	}
	resCBC, err := gb.SolveCompiled(tb.Context(), p,
		gb.WithCBC(),
		gb.WithRelGapTol(1e-6),
		gb.WithAbsGapTol(1e-6),
		gb.WithTimeLimit(1500*time.Millisecond),
		gb.WithMaxNodes(4000),
	)
	if err != nil {
		tb.Fatalf("[%s] CBC solve error: %v", label, err)
	}
	assertMIPResultsSoundParity(tb, label, p, "go", resGo, "cbc", resCBC)
}
