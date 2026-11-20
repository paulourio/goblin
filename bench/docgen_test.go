package bench

import (
	"math"
	"strings"
	"testing"

	"github.com/paulourio/goblin/assign"
)

func TestTCrit975AndFamilyClusteredRatio(t *testing.T) {
	// Reference two-sided 95% Student-t critical values (alpha/2 = 0.025).
	ref := []struct {
		df   int
		want float64
		tol  float64
	}{
		{df: 1, want: 12.7062047364, tol: 1e-3},
		{df: 4, want: 2.7764451052, tol: 1e-3},
		{df: 5, want: 2.5705818356, tol: 1e-3},
		{df: 9, want: 2.2621571628, tol: 1e-3},
		{df: 13, want: 2.1603686565, tol: 1e-3},
		{df: 15, want: 2.1314495456, tol: 1e-3},
		{df: 21, want: 2.0796138447, tol: 1e-5},
		{df: 30, want: 2.0422724563, tol: 1e-5},
		{df: 60, want: 2.0002978220, tol: 1e-5},
	}
	for _, tc := range ref {
		got := tCrit975(tc.df)
		if math.Abs(got-tc.want) > tc.tol {
			t.Fatalf("tCrit975(%d) = %.10f, want %.10f (tol %g)", tc.df, got, tc.want, tc.tol)
		}
	}

	gm, lo, hi := familyClusteredRatio([]float64{4.0, 16.0})
	if math.Abs(gm-8.0) > 1e-12 || !(lo < 8.0 && hi > 8.0) {
		t.Fatalf("familyClusteredRatio([4, 16]) = (%v, [%v, %v]), want 8.0 inside CI", gm, lo, hi)
	}
}

func TestCheckParityRejectsBrokenRootLPAndAssignSIMDReachability(t *testing.T) {
	obj := 100.0
	lpObj := 80.0
	tb := &TelemetryBundle{
		Python: map[string]PythonBaselineEntry{
			"p1": {
				ID:                    "p1",
				Status:                "Optimal",
				Objective:             &obj,
				LPRelaxationObjective: &lpObj,
			},
		},
		Scalar: map[string]ProblemBenchResult{
			"p1": {
				ID:           "p1",
				RootLPObj:    80.0,
				RootLPStatus: "Optimal",
				CP7Full:      CheckpointMetrics{Status: "Optimal", Objective: 100.0},
			},
		},
		SIMD: map[string]ProblemBenchResult{
			"p1": {
				ID:           "p1",
				RootLPObj:    0.0,
				RootLPStatus: "NumericalError",
				CP7Full:      CheckpointMetrics{Status: "Optimal", Objective: 100.0},
			},
		},
	}
	errs := CheckParity(tb, "p1")
	if len(errs) != 1 || !strings.Contains(errs[0], "root LP status NumericalError") {
		t.Fatalf("expected CheckParity to reject NumericalError RootLPStatus, got %v", errs)
	}

	tb.SIMD["p1"] = ProblemBenchResult{
		ID:           "p1",
		RootLPObj:    95.0,
		RootLPStatus: "Optimal",
		CP7Full:      CheckpointMetrics{Status: "Optimal", Objective: 100.0},
	}
	errsObj := CheckParity(tb, "p1")
	if len(errsObj) != 1 || !strings.Contains(errsObj[0], "root LP objective") {
		t.Fatalf("expected CheckParity to reject drifted RootLPObj, got %v", errsObj)
	}

	tb.SIMD["p1"] = ProblemBenchResult{
		ID:           "p1",
		RootLPObj:    80.0,
		RootLPStatus: "",
		CP7Full:      CheckpointMetrics{Status: "Optimal", Objective: 100.0},
	}
	errsEmpty := CheckParity(tb, "p1")
	if len(errsEmpty) != 1 || !strings.Contains(errsEmpty[0], "root LP status") {
		t.Fatalf("expected CheckParity to reject empty RootLPStatus, got %v", errsEmpty)
	}

	if assignSIMDReachable(AssignCorpusCase{Rows: 10, Cols: 10, Sense: assign.Minimize}) {
		t.Fatal("assignSIMDReachable(10x10) = true, want false (max<32 and 100<256)")
	}
	if !assignSIMDReachable(AssignCorpusCase{Rows: 20, Cols: 20, Sense: assign.Minimize}) {
		t.Fatal("assignSIMDReachable(20x20) = false, want true (20*20 >= 256 exercises AllAbsLE64)")
	}
}
