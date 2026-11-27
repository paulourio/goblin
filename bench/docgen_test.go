package bench

import (
	"bytes"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
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
			MAD: map[string]MADBenchResult{
				"m1": {ID: "m1", GitCommit: "abc", SourceFingerprint: "fp1", UnameRelease: "6.1", GoVersion: "go1.27.3-X:nodwarf5", SIMDEnabled: false},
			},
		}
	}
	if _, err := telemetryProvenance(baseBundle()); err != nil {
		t.Fatalf("valid telemetryProvenance failed: %v", err)
	}

	bBadMADSIMDVer := baseBundle()
	mrBadVer := bBadMADSIMDVer.MAD["m1"]
	mrBadVer.GoVersion = "go1.27.3-X:simd"
	bBadMADSIMDVer.MAD["m1"] = mrBadVer
	if _, err := telemetryProvenance(bBadMADSIMDVer); err == nil {
		t.Fatal("expected error for goblin_mad entry built with -X:simd")
	}

	bBadMADSIMDFlag := baseBundle()
	mrBadFlag := bBadMADSIMDFlag.MAD["m1"]
	mrBadFlag.SIMDEnabled = true
	bBadMADSIMDFlag.MAD["m1"] = mrBadFlag
	if _, err := telemetryProvenance(bBadMADSIMDFlag); err == nil {
		t.Fatal("expected error for goblin_mad entry with simd_enabled=true")
	}

	bBadScalarVer := baseBundle()
	s := bBadScalarVer.Scalar["p1"]
	s.GoVersion = "go1.27.3-X:simd"
	bBadScalarVer.Scalar["p1"] = s
	if _, err := telemetryProvenance(bBadScalarVer); err == nil {
		t.Fatal("expected error for non-paired scalar entry built with -X:simd")
	}

	bBadAssignScalarVer := baseBundle()
	asUnpaired := bBadAssignScalarVer.AssignScalar["a1"]
	asUnpaired.GoVersion = "go1.27.3-X:simd"
	bBadAssignScalarVer.AssignScalar["a1"] = asUnpaired
	if _, err := telemetryProvenance(bBadAssignScalarVer); err == nil {
		t.Fatal("expected error for non-paired assign scalar entry built with -X:simd")
	}

	bPairedOK := baseBundle()
	sPaired := bPairedOK.Scalar["p1"]
	sPaired.GoVersion = "go1.27.3-X:simd"
	sPaired.PairedABBA = true
	bPairedOK.Scalar["p1"] = sPaired
	asPaired := bPairedOK.AssignScalar["a1"]
	asPaired.GoVersion = "go1.27.3-X:simd"
	asPaired.PairedABBA = true
	bPairedOK.AssignScalar["a1"] = asPaired
	bPairedOK.Goblin = map[string]ProblemBenchResult{
		"p1": {ID: "p1", GitCommit: "abc", SourceFingerprint: "fp1", UnameRelease: "6.1", GoVersion: "go1.27.3-X:nodwarf5", SIMDEnabled: false},
	}
	if _, err := telemetryProvenance(bPairedOK); err != nil {
		t.Fatalf("expected paired_ab_ba scalar entries with -X:simd to pass provenance, got %v", err)
	}

	bPairedSIMDTrue := baseBundle()
	sBadPaired := bPairedSIMDTrue.Scalar["p1"]
	sBadPaired.GoVersion = "go1.27.3-X:simd"
	sBadPaired.PairedABBA = true
	sBadPaired.SIMDEnabled = true
	bPairedSIMDTrue.Scalar["p1"] = sBadPaired
	if _, err := telemetryProvenance(bPairedSIMDTrue); err == nil {
		t.Fatal("expected error for paired_ab_ba scalar entry with simd_enabled=true")
	}

	bPairedNoSIMDExp := baseBundle()
	sNoExp := bPairedNoSIMDExp.Scalar["p1"]
	sNoExp.PairedABBA = true
	bPairedNoSIMDExp.Scalar["p1"] = sNoExp
	if _, err := telemetryProvenance(bPairedNoSIMDExp); err == nil {
		t.Fatal("expected error for paired_ab_ba scalar entry built without -X:simd")
	}

	bBadCBCPaired := baseBundle()
	bBadCBCPaired.Goblin = map[string]ProblemBenchResult{
		"p1": {ID: "p1", GitCommit: "abc", SourceFingerprint: "fp1", UnameRelease: "6.1", GoVersion: "go1.27.3-X:simd", SIMDEnabled: false, PairedABBA: true},
	}
	if _, err := telemetryProvenance(bBadCBCPaired); err == nil {
		t.Fatal("expected error for goblin_cbc entry carrying -X:simd even with paired_ab_ba=true")
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

	madSolverSummary, err := RenderBlock("README_MAD_SOLVER_SUMMARY_TABLE", tb)
	if err != nil {
		t.Fatalf("RenderBlock(README_MAD_SOLVER_SUMMARY_TABLE): %v", err)
	}
	if !strings.Contains(madSolverSummary, "| Workload | Families (Trials) | Auto Med Sum (ms) | DP Med Sum (ms) | DFS Med Sum (ms) |") {
		t.Fatalf("unexpected README_MAD_SOLVER_SUMMARY_TABLE header:\n%s", madSolverSummary)
	}
	if !strings.Contains(madSolverSummary, "skip (>250k)") || !strings.Contains(madSolverSummary, "(7/13)") {
		t.Fatalf("README_MAD_SOLVER_SUMMARY_TABLE missing DFS cap/coverage cells:\n%s", madSolverSummary)
	}

	madCombSummary, err := RenderBlock("MAD_COMBINATORIAL_SUMMARY_TABLE", tb)
	if err != nil {
		t.Fatalf("RenderBlock(MAD_COMBINATORIAL_SUMMARY_TABLE): %v", err)
	}
	if !strings.Contains(madCombSummary, "| Workload Subset | Families (Trials) | Auto Trial (Med) Sum (ms) | DP Trial (Med) Sum (ms) | DFS Trial (Med) Sum (ms) |") {
		t.Fatalf("unexpected MAD_COMBINATORIAL_SUMMARY_TABLE header:\n%s", madCombSummary)
	}
	if !strings.Contains(madCombSummary, "skip (>250k)") || !strings.Contains(madCombSummary, "[7/13]") {
		t.Fatalf("MAD_COMBINATORIAL_SUMMARY_TABLE missing DFS cap/coverage cells:\n%s", madCombSummary)
	}

	madCombPipe, err := RenderBlock("MAD_COMBINATORIAL_PIPELINE_TABLE", tb)
	if err != nil {
		t.Fatalf("RenderBlock(MAD_COMBINATORIAL_PIPELINE_TABLE): %v", err)
	}
	if !strings.Contains(madCombPipe, "pmad_base_g10_i2") || !strings.Contains(madCombPipe, "24137.0") {
		t.Fatalf("MAD_COMBINATORIAL_PIPELINE_TABLE missing pmad_base_g10_i2 24137.0:\n%s", madCombPipe)
	}

	cases := BuildCorpus()
	nonPlanted := func(c CorpusCase) bool {
		return c.Tier != "tier1_canonical" && !isPlantedMAD(c.ID)
	}
	pyNP := summarizeTierSubset(cases, nonPlanted, tb)
	cbcNP, err := summarizeCBCTierSubset(cases, nonPlanted, tb)
	if err != nil {
		t.Fatalf("summarizeCBCTierSubset(non-planted): %v", err)
	}
	cbcAll, err := summarizeCBCTierSubset(cases, func(_ CorpusCase) bool { return true }, tb)
	if err != nil {
		t.Fatalf("summarizeCBCTierSubset(all): %v", err)
	}

	cbcSummary, err := RenderBlock("CBC_SUMMARY_TABLE", tb)
	if err != nil {
		t.Fatalf("RenderBlock(CBC_SUMMARY_TABLE): %v", err)
	}
	wantNP := fmt.Sprintf("**Tier 2+3 non-planted MILPs (solver-dominated)** | %d (%d) | **%.2fx** [%.2fx-%.2fx] | **%.2fx** [%.2fx-%.2fx]",
		cbcNP.nFams, cbcNP.nInst, cbcNP.famTotGM, cbcNP.famTotLo, cbcNP.famTotHi, cbcNP.famSolveGM, cbcNP.famSolveLo, cbcNP.famSolveHi)
	wantAll := fmt.Sprintf("All 31 synthetic families | %d (%d) | **%.2fx** [%.2fx-%.2fx]",
		cbcAll.nFams, cbcAll.nInst, cbcAll.famTotGM, cbcAll.famTotLo, cbcAll.famTotHi)
	for _, want := range []string{
		"Equal-Family `BackendCBC / BackendGo` Total [Clustered 95% CI]",
		wantNP,
		wantAll,
	} {
		if !strings.Contains(cbcSummary, want) {
			t.Fatalf("CBC_SUMMARY_TABLE missing %q:\n%s", want, cbcSummary)
		}
	}

	cbcHeadline, err := RenderBlock("README_CBC_HEADLINE", tb)
	if err != nil {
		t.Fatalf("RenderBlock(README_CBC_HEADLINE): %v", err)
	}
	wantPyHeadline := fmt.Sprintf("**%.2fx** [%.2fx-%.2fx]", pyNP.famGM, pyNP.famLo, pyNP.famHi)
	wantCBCHeadline := fmt.Sprintf("**%.2fx** [%.2fx-%.2fx]", cbcNP.famTotGM, cbcNP.famTotLo, cbcNP.famTotHi)
	if !strings.Contains(cbcHeadline, wantPyHeadline) || !strings.Contains(cbcHeadline, wantCBCHeadline) {
		t.Fatalf("README_CBC_HEADLINE missing expected ratios %q / %q:\n%s", wantPyHeadline, wantCBCHeadline, cbcHeadline)
	}

	cbcNotes, err := RenderBlock("NON_MAD_CBC_NOTES", tb)
	if err != nil {
		t.Fatalf("RenderBlock(NON_MAD_CBC_NOTES): %v", err)
	}
	scBP := tb.Scalar["t2_binpack_44"]
	gbBP := tb.Goblin["t2_binpack_44"]
	wantBPRatio := fmt.Sprintf("**%.2fx**", pairedRatioOrFallback(gbBP.TTotalTrialsMS, scBP.TTotalTrialsMS, gbBP.TTotalMS, scBP.TTotalMS))
	if !strings.Contains(cbcNotes, "t2_binpack_44") || !strings.Contains(cbcNotes, wantBPRatio) {
		t.Fatalf("NON_MAD_CBC_NOTES missing t2_binpack_44 %s disclosure:\n%s", wantBPRatio, cbcNotes)
	}
}

func TestRenderNonMADCBCNotesBranches(t *testing.T) {
	t.Parallel()

	mkBundle := func(goTrials, cbcTrials []float64, goMed, goMean, cbcMed, cbcMean float64) *TelemetryBundle {
		return &TelemetryBundle{
			Scalar: map[string]ProblemBenchResult{
				"t2_binpack_44": {
					TTotalMS:       goMed,
					TTotalMeanMS:   goMean,
					TTotalSDMS:     1.25,
					TTotalTrialsMS: goTrials,
					CP5Presolve:    CheckpointMetrics{TimeMS: 1.10},
					CP6Cuts:        CheckpointMetrics{TimeMS: 2.30},
				},
			},
			Goblin: map[string]ProblemBenchResult{
				"t2_binpack_44": {
					TTotalMS:       cbcMed,
					TTotalMeanMS:   cbcMean,
					TTotalSDMS:     0.50,
					TTotalTrialsMS: cbcTrials,
				},
			},
		}
	}

	var negBuf bytes.Buffer
	tbTrail := mkBundle([]float64{20.0, 30.0}, []float64{10.0, 15.0}, 25.0, 25.0, 12.5, 12.5)
	if err := renderNonMADCBCNotes(&negBuf, tbTrail); err != nil {
		t.Fatalf("renderNonMADCBCNotes(trail): %v", err)
	}
	negOut := negBuf.String()
	for _, want := range []string{
		"Heavy-tail regression on `t2_binpack_44`",
		"trails in-process `BackendCBC`",
		"**0.50x** `BackendCBC / BackendGo` geometric-mean ratio (`Go / CBC = 2.00x`)",
		"running slower on 2 of 2 trials (peaking at 30.00 ms vs. 15.00 ms)",
		"while root cuts increase `CP5 +Presolve` (1.10 ms) to `CP6 +Cuts` (2.30 ms).",
	} {
		if !strings.Contains(negOut, want) {
			t.Fatalf("trail output missing %q:\n%s", want, negOut)
		}
	}

	var leadBuf bytes.Buffer
	tbLead := mkBundle([]float64{8.0, 12.0, 14.0}, []float64{16.0, 18.0, 10.0}, 12.0, 11.33, 16.0, 14.67)
	if err := renderNonMADCBCNotes(&leadBuf, tbLead); err != nil {
		t.Fatalf("renderNonMADCBCNotes(lead): %v", err)
	}
	leadOut := leadBuf.String()
	for _, want := range []string{
		"Bin-packing tail behavior on `t2_binpack_44`",
		"leads in-process `BackendCBC`",
		"`BackendCBC / BackendGo` geometric-mean speedup",
		"running slower on 1 of 3 trials (peaking at 14.00 ms vs. 10.00 ms)",
		"with `CP5 +Presolve` at 1.10 ms vs. `CP6 +Cuts` at 2.30 ms.",
	} {
		if !strings.Contains(leadOut, want) {
			t.Fatalf("lead output missing %q:\n%s", want, leadOut)
		}
	}
}

func TestCollectFormalInventoryAndHelpers(t *testing.T) {
	t.Parallel()

	rawLean := `
/-
  theorem ignoredInBlockComment : True := trivial
  /- nested comment with lemma nestedIgnored : True := trivial -/
-/
-- theorem ignoredInLineComment : True := trivial
theorem pubOne : True := trivial
private theorem privOne : True := trivial
@[simp] private lemma privAttr : True := trivial
protected noncomputable lemma pubProt : True := trivial
def helperWithString : String := "-- theorem notAComment"
`
	stripped := stripLeanComments(rawLean)
	pub, priv := countLeanTheoremDecls(stripped)
	if pub != 2 || priv != 2 {
		t.Fatalf("countLeanTheoremDecls = (%d pub, %d priv), want (2, 2); stripped:\n%s", pub, priv, stripped)
	}

	// Verify .lake is ignored whether it is a directory or a symlink.
	tmp := t.TempDir()
	extDir := filepath.Join(tmp, "external_lake")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extDir, "Ignored.lean"), []byte("theorem leaked : True := trivial\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	formalDir := filepath.Join(tmp, "formal")
	if err := os.MkdirAll(filepath.Join(formalDir, "GoblinFormal"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(formalDir, "Apps"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(extDir, filepath.Join(formalDir, ".lake")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(formalDir, "GoblinFormal.lean"), []byte("import GoblinFormal.Track1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(formalDir, "GoblinFormal", "Track1.lean"), []byte("theorem t1 : True := trivial\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(formalDir, "GoblinFormal", "AxiomAudit.lean"), []byte("#guard_msgs in\n#print axioms GoblinFormal.Track1.t1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(formalDir, "Apps", "Common.lean"), []byte("def c : Nat := 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var synthInv FormalInventory
	if err := collectLeanInventory(tmp, &synthInv); err != nil {
		t.Fatalf("collectLeanInventory symlink .lake test failed: %v", err)
	}
	if synthInv.LeanTotalFiles != 4 || synthInv.LeanTotalAuthored != 1 || synthInv.AxiomLocks != 1 {
		t.Fatalf("unexpected symlink .lake inventory: %+v", synthInv)
	}

	// Verify collectFuzzTargetsInventory rejects drift against targets.tsv.
	if err := os.MkdirAll(filepath.Join(tmp, "scripts", "gcp-fuzz"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "foo_test.go"), []byte("package foo\nimport \"testing\"\nfunc FuzzOne(f *testing.F) {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "scripts", "gcp-fuzz", "targets.tsv"), []byte("0\t/app/bin/goblin.test\t/app/work\tFuzzOther\tfast\t16\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var driftInv FormalInventory
	if err := collectFuzzTargetsInventory(tmp, &driftInv); err == nil {
		t.Fatal("expected collectFuzzTargetsInventory to reject Go vs targets.tsv mismatch")
	}

	// Verify live repository inventory matches exact expected counts.
	inv, err := CollectFormalInventory("..")
	if err != nil {
		t.Fatalf("CollectFormalInventory: %v", err)
	}
	if inv.LeanGoblinFiles != 16 || inv.LeanAppsFiles != 8 || inv.LeanTotalFiles != 24 {
		t.Fatalf("unexpected Lean file counts: goblin=%d apps=%d total=%d", inv.LeanGoblinFiles, inv.LeanAppsFiles, inv.LeanTotalFiles)
	}
	if inv.LeanGoblinLOC != 51295 || inv.LeanAppsLOC != 2577 || inv.LeanTotalLOC != 53872 {
		t.Fatalf("unexpected Lean LOC: goblin=%d apps=%d total=%d", inv.LeanGoblinLOC, inv.LeanAppsLOC, inv.LeanTotalLOC)
	}
	if inv.LeanGoblinAuthored != 1324 || inv.LeanGoblinPublic != 620 || inv.LeanGoblinPrivate != 704 || inv.LeanAppsAuthored != 1 || inv.LeanTotalAuthored != 1325 {
		t.Fatalf("unexpected authored theorem counts: %+v", inv)
	}
	if inv.LeanGoblinKernel != 5843 || inv.LeanAppsKernel != 33 || inv.LeanTotalKernel != 5876 || inv.AxiomLocks != 220 {
		t.Fatalf("unexpected kernel/lock counts: kernel=(%d+%d=%d) locks=%d",
			inv.LeanGoblinKernel, inv.LeanAppsKernel, inv.LeanTotalKernel, inv.AxiomLocks)
	}
	if len(inv.ProdCertFiles) != 6 || inv.ProdCertLOC != 20615 {
		t.Fatalf("unexpected prod cert files=%d loc=%d", len(inv.ProdCertFiles), inv.ProdCertLOC)
	}
	if len(inv.FormalTestFiles) != 21 || inv.TotalGoTestFiles != 31 {
		t.Fatalf("unexpected Go test file counts: formal=%d total=%d", len(inv.FormalTestFiles), inv.TotalGoTestFiles)
	}
	if inv.DefaultFuzzCount != 42 || inv.CBCFuzzCount != 8 || inv.TotalFuzzCount != 50 || inv.TSVActiveRowCount != 50 {
		t.Fatalf("unexpected Fuzz counts: default=%d cbc=%d total=%d tsv=%d",
			inv.DefaultFuzzCount, inv.CBCFuzzCount, inv.TotalFuzzCount, inv.TSVActiveRowCount)
	}
	if err := SyncFormalDocs("..", true); err != nil {
		t.Fatalf("SyncFormalDocs: %v", err)
	}
}

func TestTargetSiliconDocgenAndNegativeControls(t *testing.T) {
	t.Parallel()

	cases := BuildCorpus()
	assignCases := AssignCorpusSpecs()

	tsDir := filepath.Join("results", "target_silicon")
	ts, err := LoadTargetSiliconBundle(tsDir)
	if err != nil {
		t.Fatalf("LoadTargetSiliconBundle: %v", err)
	}
	if err := ValidateTargetSiliconTelemetry(cases, assignCases, ts); err != nil {
		t.Fatalf("ValidateTargetSiliconTelemetry: %v", err)
	}

	tb, err := LoadTelemetryBundle(
		"results/python_baseline.json",
		"results/goblin_scalar.json",
		"results/goblin_simd.json",
	)
	if err != nil {
		t.Fatalf("LoadTelemetryBundle: %v", err)
	}
	rendered, err := RenderBlock("TARGET_SILICON_SUMMARY", tb)
	if err != nil {
		t.Fatalf("RenderBlock(TARGET_SILICON_SUMMARY): %v", err)
	}
	for _, want := range []string{
		"Native `linux/amd64` AVX-512 host",
		"Native `linux/arm64` host",
		"`simd.HardwareSIMDTier() == \"avx512\"`",
		"Emerald Rapids 6-Round Interleaved AB/BA Vector Kernel Crossover",
		"Target-Silicon End-to-End LP/MILP and LSAP Corpus Summary",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("TARGET_SILICON_SUMMARY missing %q:\n%s", want, rendered)
		}
	}

	// Future solver commits must not fail ValidateTargetSiliconTelemetry when SourceFingerprint advances.
	cloneTS := func() *TargetSiliconBundle {
		cp := *ts
		return &cp
	}
	tsHistorical := cloneTS()
	tsHistorical.AMD64.SourceFingerprint = "0123456789abcdef"
	tsHistorical.ARM64.SourceFingerprint = "0123456789abcdef"
	if err := ValidateTargetSiliconTelemetry(cases, assignCases, tsHistorical); err != nil {
		t.Fatalf("expected historical target-silicon fingerprint to remain valid without GCE re-run, got: %v", err)
	}

	// Negative controls on ValidateTargetSiliconTelemetry.
	badTier := cloneTS()
	badTier.AMD64.HardwareSIMDTier = "avx2"
	if err := ValidateTargetSiliconTelemetry(cases, assignCases, badTier); err == nil {
		t.Fatal("expected error when amd64_avx512 HardwareSIMDTier != avx512")
	}

	badBusy := cloneTS()
	badBusy.AMD64.PreBenchCPUBusyPct = 7.5
	if err := ValidateTargetSiliconTelemetry(cases, assignCases, badBusy); err == nil {
		t.Fatal("expected error when pre_bench_cpu_busy_pct >= 5.0")
	}

	badParity := cloneTS()
	badParity.ARM64.AssignCorpus.ParityMismatches = 1
	if err := ValidateTargetSiliconTelemetry(cases, assignCases, badParity); err == nil {
		t.Fatal("expected error on non-zero arm64 LSAP parity_mismatches")
	}

	badKernel := cloneTS()
	badKernel.AMD64.KernelABBA.Entries = nil
	if err := ValidateTargetSiliconTelemetry(cases, assignCases, badKernel); err == nil {
		t.Fatal("expected error when kernel_abba entries are empty")
	}
}

func TestCheckMADParityAndCorpusValidationNegativeControls(t *testing.T) {
	t.Parallel()

	ptr := func(v float64) *float64 {
		return &v
	}
	baseEntry := func() MADBenchResult {
		return MADBenchResult{
			ID:              "m1",
			Groups:          2,
			Items:           4,
			Periods:         10,
			Trials:          2,
			Objective:       100.0,
			TrialObjectives: []float64{100.0, 120.0},
			ParityOK:        true,
			MILPGo: MADMILPMetrics{
				Status:       "Optimal",
				HasIncumbent: true,
				HasBestBound: true,
				Objective:    ptr(100.0),
				BestBound:    100.0,
			},
			MILPCBC: &MADMILPMetrics{
				Status:       "TimeLimit",
				HasIncumbent: true,
				HasBestBound: true,
				Objective:    ptr(101.0),
				BestBound:    95.0,
			},
		}
	}

	// Ensure a valid baseline passes `CheckMADParity` with zero errors.
	validTB := &TelemetryBundle{MAD: map[string]MADBenchResult{"m1": baseEntry()}}
	if errs := CheckMADParity(validTB, "m1"); len(errs) != 0 {
		t.Fatalf("CheckMADParity(valid) = %v, want empty", errs)
	}
	if errs := CheckMADParity(validTB, "missing"); len(errs) != 1 || !strings.Contains(errs[0], "missing mad telemetry") {
		t.Fatalf("CheckMADParity(missing) = %v, want missing mad telemetry", errs)
	}

	// Test `CheckMADParity` negative controls.
	tol := parityRelTol * 100.0
	parityCases := []struct {
		name   string
		mutate func(*MADBenchResult)
		wants  []string
	}{
		{
			name:   "parity_ok_false",
			mutate: func(r *MADBenchResult) { r.ParityOK = false },
			wants:  []string{"m1: parity_ok=false"},
		},
		{
			name:   "trial_objectives_length_mismatch",
			mutate: func(r *MADBenchResult) { r.TrialObjectives = []float64{100.0} },
			wants:  []string{"m1: trial_objectives count 1 != trials 2"},
		},
		{
			name:   "zero_trials",
			mutate: func(r *MADBenchResult) { r.Trials = 0; r.TrialObjectives = nil },
			wants:  []string{"m1: trial_objectives count 0 != trials 0"},
		},
		{
			name:   "objective_trial0_mismatch",
			mutate: func(r *MADBenchResult) { r.TrialObjectives[0] = 100.1 },
			wants:  []string{"m1: objective", "!= trial0"},
		},
		{
			name:   "milp_go_infeasible",
			mutate: func(r *MADBenchResult) { r.MILPGo.Status = "Infeasible" },
			wants:  []string{`m1/milp_go: unexpected status "Infeasible"`},
		},
		{
			name:   "milp_go_best_bound_above_exact",
			mutate: func(r *MADBenchResult) { r.MILPGo.BestBound = 100.0 + 2.0*tol },
			wants:  []string{"m1/milp_go: best_bound", "> exact"},
		},
		{
			name:   "milp_go_incumbent_nil",
			mutate: func(r *MADBenchResult) { r.MILPGo.Status = "TimeLimit"; r.MILPGo.Objective = nil },
			wants:  []string{"m1/milp_go: has_incumbent=true with nil objective"},
		},
		{
			name:   "milp_go_incumbent_below_exact",
			mutate: func(r *MADBenchResult) { r.MILPGo.Status = "TimeLimit"; r.MILPGo.Objective = ptr(100.0 - 2.0*tol) },
			wants:  []string{"m1/milp_go: incumbent", "< exact"},
		},
		{
			name: "milp_go_best_bound_above_incumbent",
			mutate: func(r *MADBenchResult) {
				r.MILPGo.Status = "TimeLimit"
				r.MILPGo.BestBound = 100.0 + 0.8*tol
				r.MILPGo.Objective = ptr(100.0 - 0.5*tol)
			},
			wants: []string{"m1/milp_go: best_bound", "> incumbent"},
		},
		{
			name:   "milp_go_optimal_wrong_objective",
			mutate: func(r *MADBenchResult) { r.MILPGo.Objective = ptr(100.5) },
			wants:  []string{"m1/milp_go: optimal objective"},
		},
		{
			name:   "milp_go_optimal_no_incumbent",
			mutate: func(r *MADBenchResult) { r.MILPGo.HasIncumbent = false; r.MILPGo.Objective = nil },
			wants:  []string{"m1/milp_go: Optimal status without incumbent"},
		},
		{
			name:   "milp_cbc_best_bound_above_exact",
			mutate: func(r *MADBenchResult) { r.MILPCBC.BestBound = 100.0 + 2.0*tol },
			wants:  []string{"m1/milp_cbc: best_bound", "> exact"},
		},
		{
			name: "milp_cbc_best_bound_above_incumbent",
			mutate: func(r *MADBenchResult) {
				r.MILPCBC.BestBound = 100.0 + 0.8*tol
				r.MILPCBC.Objective = ptr(100.0 - 0.5*tol)
			},
			wants: []string{"m1/milp_cbc: best_bound", "> incumbent"},
		},
	}

	for _, tc := range parityCases {
		entry := baseEntry()
		tc.mutate(&entry)
		tb := &TelemetryBundle{MAD: map[string]MADBenchResult{"m1": entry}}
		errs := CheckMADParity(tb, "m1")
		joined := strings.Join(errs, "; ")
		for _, want := range tc.wants {
			if len(errs) == 0 || !strings.Contains(joined, want) {
				t.Fatalf("%s: CheckMADParity() = %v, want substring %q", tc.name, errs, want)
			}
		}
	}

	// Test `ValidateTelemetryAgainstCorpus` MAD negative controls.
	baseTB, err := LoadTelemetryBundle(
		"results/python_baseline.json",
		"results/goblin_scalar.json",
		"results/goblin_simd.json",
	)
	if err != nil {
		t.Fatalf("LoadTelemetryBundle: %v", err)
	}
	if err := ValidateTelemetryAgainstCorpus("../testdata/corpus", baseTB); err != nil {
		t.Fatalf("ValidateTelemetryAgainstCorpus(baseline): %v", err)
	}

	cloneBundleWithMAD := func(mut func(map[string]MADBenchResult)) *TelemetryBundle {
		cp := *baseTB
		cp.MAD = maps.Clone(baseTB.MAD)
		mut(cp.MAD)
		return &cp
	}

	corpusCases := []struct {
		name string
		mut  func(map[string]MADBenchResult)
		want string
	}{
		{
			name: "missing_mad_entry_len_mismatch",
			mut:  func(m map[string]MADBenchResult) { delete(m, "pmad_base_g10_i2") },
			want: "mad telemetry has",
		},
		{
			name: "missing_mad_id_same_len",
			mut: func(m map[string]MADBenchResult) {
				v := m["pmad_base_g10_i2"]
				delete(m, "pmad_base_g10_i2")
				m["unknown_mad_family"] = v
			},
			want: "missing \"pmad_base_g10_i2\" in mad telemetry",
		},
		{
			name: "mad_dimension_mismatch",
			mut: func(m map[string]MADBenchResult) {
				v := m["pmad_base_g10_i2"]
				v.Items++
				m["pmad_base_g10_i2"] = v
			},
			want: "mad telemetry dimension mismatch",
		},
		{
			name: "mad_parity_failure_propagated",
			mut: func(m map[string]MADBenchResult) {
				v := m["pmad_base_g10_i2"]
				v.MILPGo.BestBound = v.Objective + 1.0
				m["pmad_base_g10_i2"] = v
			},
			want: "parity check failed",
		},
	}

	for _, tc := range corpusCases {
		tb := cloneBundleWithMAD(tc.mut)
		err := ValidateTelemetryAgainstCorpus("../testdata/corpus", tb)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: ValidateTelemetryAgainstCorpus() = %v, want substring %q", tc.name, err, tc.want)
		}
	}
}
