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

	validBinMap := func() map[string]BinarySizeEntry {
		return map[string]BinarySizeEntry{
			"assign_default": {
				ID:                "assign_default",
				Label:             "`assign` default (`LinearSum` + `SolveInto`)",
				RawBytes:          2840000,
				StrippedBytes:     1774000,
				SIMDRawBytes:      2950000,
				SIMDStrippedBytes: 1850000,
				GoblinTextBytes:   98000,
				GitCommit:         "abc",
				SourceFingerprint: "fp1",
				UnameRelease:      "6.1",
				DefaultGoVersion:  "go1.27.3-X:nodwarf5",
				SIMDGoVersion:     "go1.27.3-X:simd",
			},
			"assign_certified": {
				ID:                "assign_certified",
				Label:             "`assign` certified (`+ assign.WithCertified(true)`)",
				RawBytes:          2890000,
				StrippedBytes:     1810000,
				SIMDRawBytes:      3000000,
				SIMDStrippedBytes: 1886000,
				GoblinTextBytes:   122000,
				CertSyms:          14,
				CertBytes:         24000,
				GitCommit:         "abc",
				SourceFingerprint: "fp1",
				UnameRelease:      "6.1",
				DefaultGoVersion:  "go1.27.3-X:nodwarf5",
				SIMDGoVersion:     "go1.27.3-X:simd",
			},
			"goblin_default": {
				ID:                "goblin_default",
				Label:             "`goblin` default (`Model.Solve`, `DefaultOptions`)",
				RawBytes:          5310000,
				StrippedBytes:     3380000,
				SIMDRawBytes:      5520000,
				SIMDStrippedBytes: 3510000,
				GoblinTextBytes:   620000,
				MathBigSyms:       121,
				MathBigBytes:      73000,
				GitCommit:         "abc",
				SourceFingerprint: "fp1",
				UnameRelease:      "6.1",
				DefaultGoVersion:  "go1.27.3-X:nodwarf5",
				SIMDGoVersion:     "go1.27.3-X:simd",
			},
			"goblin_certified_vipr": {
				ID:                "goblin_certified_vipr",
				Label:             "`goblin` certified + VIPR (`+ WithCertified + WithVIPRTrace`)",
				RawBytes:          6810000,
				StrippedBytes:     4366000,
				SIMDRawBytes:      7020000,
				SIMDStrippedBytes: 4496000,
				GoblinTextBytes:   1120000,
				MathBigSyms:       134,
				MathBigBytes:      81000,
				VIPRSyms:          194,
				VIPRBytes:         288000,
				CertSyms:          82,
				CertBytes:         195000,
				GitCommit:         "abc",
				SourceFingerprint: "fp1",
				UnameRelease:      "6.1",
				DefaultGoVersion:  "go1.27.3-X:nodwarf5",
				SIMDGoVersion:     "go1.27.3-X:simd",
			},
		}
	}

	bWithBin := baseBundle()
	bWithBin.BinarySize = validBinMap()
	if _, err := telemetryProvenance(bWithBin); err != nil {
		t.Fatalf("valid telemetryProvenance with BinarySize failed: %v", err)
	}
	if err := ValidateBinarySizeTelemetry(bWithBin.BinarySize); err != nil {
		t.Fatalf("valid ValidateBinarySizeTelemetry failed: %v", err)
	}
	rendered, err := RenderBlock("BINARY_SIZE_TABLE", bWithBin)
	if err != nil || !strings.Contains(rendered, "baseline") || !strings.Contains(rendered, "+962.9 KiB") {
		t.Fatalf("RenderBlock(BINARY_SIZE_TABLE) = (%q, %v)", rendered, err)
	}

	// Negative provenance controls on BinarySize.
	bBadBinFP := baseBundle()
	bBadBinFP.BinarySize = validBinMap()
	eFP := bBadBinFP.BinarySize["goblin_default"]
	eFP.SourceFingerprint = "fp_stale"
	bBadBinFP.BinarySize["goblin_default"] = eFP
	if _, err := telemetryProvenance(bBadBinFP); err == nil {
		t.Fatal("expected error for stale BinarySize source_fingerprint")
	}

	bBadBinSIMDVer := baseBundle()
	bBadBinSIMDVer.BinarySize = validBinMap()
	eVer := bBadBinSIMDVer.BinarySize["assign_default"]
	eVer.SIMDGoVersion = "go1.27.3-X:nodwarf5"
	bBadBinSIMDVer.BinarySize["assign_default"] = eVer
	if _, err := telemetryProvenance(bBadBinSIMDVer); err == nil {
		t.Fatal("expected error for BinarySize simd_go_version missing -X:simd")
	}

	// Negative DCE zero-leakage controls.
	leakAssignBig := validBinMap()
	ad := leakAssignBig["assign_default"]
	ad.MathBigSyms = 1
	leakAssignBig["assign_default"] = ad
	if err := ValidateBinarySizeTelemetry(leakAssignBig); err == nil {
		t.Fatal("expected error when assign_default leaks math/big symbols")
	}

	leakGoblinVIPR := validBinMap()
	gd := leakGoblinVIPR["goblin_default"]
	gd.VIPRSyms = 1
	leakGoblinVIPR["goblin_default"] = gd
	if err := ValidateBinarySizeTelemetry(leakGoblinVIPR); err == nil {
		t.Fatal("expected error when goblin_default leaks VIPR symbols")
	}

	leakJSON := validBinMap()
	gcv := leakJSON["goblin_certified_vipr"]
	gcv.EncodingJSONSyms = 1
	leakJSON["goblin_certified_vipr"] = gcv
	if err := ValidateBinarySizeTelemetry(leakJSON); err == nil {
		t.Fatal("expected error when goblin_certified_vipr links encoding/json symbols")
	}
}

func TestRenderReadmeSummaryBlocks(t *testing.T) {
	tb, err := LoadTelemetryBundle(
		"results/python_baseline.json",
		"results/goblin_scalar.json",
		"results/goblin_simd.json",
	)
	if err != nil {
		t.Fatalf("LoadTelemetryBundle: %v", err)
	}

	milp, err := RenderBlock("README_MILP_SUMMARY_TABLE", tb)
	if err != nil {
		t.Fatalf("RenderBlock(README_MILP_SUMMARY_TABLE): %v", err)
	}
	if !strings.Contains(milp, "| Workload | Families (Trials) | Go Sum (ms) | Python Sum (ms) |") {
		t.Fatalf("unexpected MILP README header:\n%s", milp)
	}
	if strings.Contains(milp, "CBC") {
		t.Fatalf("README MILP table should omit in-process CBC columns:\n%s", milp)
	}

	mad, err := RenderBlock("README_MAD_SUMMARY_TABLE", tb)
	if err != nil {
		t.Fatalf("RenderBlock(README_MAD_SUMMARY_TABLE): %v", err)
	}
	if !strings.Contains(mad, "| Problem | $G \\times I$ | Trials | Go (ms) | Python (ms) | Speedup |") {
		t.Fatalf("unexpected MAD README header:\n%s", mad)
	}
	if strings.Contains(mad, "SIMD") || strings.Contains(mad, "CBC") {
		t.Fatalf("README MAD table should omit SIMD and CBC columns:\n%s", mad)
	}

	assignTbl, err := RenderBlock("README_ASSIGN_SUMMARY_TABLE", tb)
	if err != nil {
		t.Fatalf("RenderBlock(README_ASSIGN_SUMMARY_TABLE): %v", err)
	}
	if !strings.Contains(assignTbl, "| Workload | Families (Trials) | Go Sum (ms) | SciPy Sum (ms) |") {
		t.Fatalf("unexpected assign README header:\n%s", assignTbl)
	}
}
