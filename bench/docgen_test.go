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

	if assignSIMDReachable(AssignCorpusCase{Rows: 10, Cols: 10, Sense: assign.Minimize, Family: "uniform"}) {
		t.Fatal("assignSIMDReachable(10x10 Min uniform) = true, want false")
	}
	if assignSIMDReachable(AssignCorpusCase{Rows: 20, Cols: 20, Sense: assign.Minimize, Family: "integer_ties"}) {
		t.Fatal("assignSIMDReachable(20x20 Min integer_ties) = true, want false (square Minimize skips AllAbsLE64)")
	}
	if !assignSIMDReachable(AssignCorpusCase{Rows: 24, Cols: 24, Sense: assign.Minimize, Family: "sparse"}) {
		t.Fatal("assignSIMDReachable(24x24 Min sparse) = false, want true")
	}
	if !assignSIMDReachable(AssignCorpusCase{Rows: 16, Cols: 16, Sense: assign.Maximize, Family: "maximize"}) {
		t.Fatal("assignSIMDReachable(16x16 Max maximize) = false, want true")
	}
	if !assignSIMDReachable(AssignCorpusCase{Rows: 10, Cols: 8, Sense: assign.Minimize, Family: "tall"}) {
		t.Fatal("assignSIMDReachable(10x8 Min tall) = false, want true (Transpose64 >= 8x8)")
	}
}

func TestTelemetryProvenanceEnforcesBuildSIMDMode(t *testing.T) {
	baseBundle := func() *TelemetryBundle {
		return &TelemetryBundle{
			Python: map[string]PythonBaselineEntry{
				"p1": {ID: "p1", GitCommit: "abc", SourceFingerprint: "fp1", UnameRelease: "6.1"},
			},
			SciPyAssign: map[string]SciPyAssignEntry{
				"a1": {ID: "a1", GitCommit: "abc", SourceFingerprint: "fp1", UnameRelease: "6.1"},
			},
			Scalar: map[string]ProblemBenchResult{
				"p1": {ID: "p1", GitCommit: "abc", SourceFingerprint: "fp1", UnameRelease: "6.1", GoVersion: "go1.27.3-X:nodwarf5", SIMDEnabled: false},
			},
			SIMD: map[string]ProblemBenchResult{
				"p1": {ID: "p1", GitCommit: "abc", SourceFingerprint: "fp1", UnameRelease: "6.1", GoVersion: "go1.27.3-X:simd", SIMDEnabled: true},
			},
			AssignScalar: map[string]AssignBenchResult{
				"a1": {ID: "a1", GitCommit: "abc", SourceFingerprint: "fp1", UnameRelease: "6.1", GoVersion: "go1.27.3-X:nodwarf5", SIMDEnabled: false},
			},
			AssignSIMD: map[string]AssignBenchResult{
				"a1": {ID: "a1", GitCommit: "abc", SourceFingerprint: "fp1", UnameRelease: "6.1", GoVersion: "go1.27.3-X:simd", SIMDEnabled: true},
			},
		}
	}
	if _, err := telemetryProvenance(baseBundle()); err != nil {
		t.Fatalf("valid telemetryProvenance failed: %v", err)
	}

	bBadScalarVer := baseBundle()
	s := bBadScalarVer.Scalar["p1"]
	s.GoVersion = "go1.27.3-X:simd"
	bBadScalarVer.Scalar["p1"] = s
	if _, err := telemetryProvenance(bBadScalarVer); err == nil {
		t.Fatal("expected error for scalar entry built with -X:simd")
	}

	bBadScalarFlag := baseBundle()
	as := bBadScalarFlag.AssignScalar["a1"]
	as.SIMDEnabled = true
	bBadScalarFlag.AssignScalar["a1"] = as
	if _, err := telemetryProvenance(bBadScalarFlag); err == nil {
		t.Fatal("expected error for scalar assign entry with simd_enabled=true")
	}

	bBadSIMDVer := baseBundle()
	m := bBadSIMDVer.SIMD["p1"]
	m.GoVersion = "go1.27.3-X:nodwarf5"
	bBadSIMDVer.SIMD["p1"] = m
	if _, err := telemetryProvenance(bBadSIMDVer); err == nil {
		t.Fatal("expected error for SIMD entry missing -X:simd")
	}

	bBadSIMDFlag := baseBundle()
	am := bBadSIMDFlag.AssignSIMD["a1"]
	am.SIMDEnabled = false
	bBadSIMDFlag.AssignSIMD["a1"] = am
	if _, err := telemetryProvenance(bBadSIMDFlag); err == nil {
		t.Fatal("expected error for SIMD assign entry with simd_enabled=false")
	}
}
