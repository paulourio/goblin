package main

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func buildHasSIMDExperiment() bool {
	tok := runtime.Version()
	if fields := strings.Fields(tok); len(fields) > 0 {
		tok = fields[0]
	}
	_, exp, ok := strings.Cut(tok, "-X:")
	if !ok {
		return false
	}
	for item := range strings.SplitSeq(exp, ",") {
		if item == "simd" {
			return true
		}
	}
	return false
}

func TestCodegenInvariants(t *testing.T) {
	if testing.Short() || raceEnabled {
		t.Skip("skipping codegen invariant audit under -short or -race")
	}
	if runtime.GOARCH != "amd64" || runtime.GOOS != "linux" {
		t.Skipf("skipping amd64/linux codegen invariant audit on %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	useSIMD := buildHasSIMDExperiment()
	res, err := checkCodegenInvariants(context.Background(), repoRoot, useSIMD)
	if err != nil {
		t.Fatalf("checkCodegenInvariants(simd=%v) failed: %v", useSIMD, err)
	}
	if len(res.Violations) > 0 {
		t.Fatalf("codegen invariant violations (simd=%v):\n  - %s", useSIMD, strings.Join(res.Violations, "\n  - "))
	}
	t.Logf("codegen invariants passed (simd=%v): inline=%d noinline=%d bce_loops=%d disasm=%d vector_audits=%d",
		useSIMD, res.CheckedInline, res.CheckedNoinline, res.CheckedBCELoops, res.CheckedDisasm, res.VectorAudits)
}

func TestVerifyStaticInvariantsNegativeControls(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	spans, err := findUnrolledLoop4Spans(repoRoot, filepath.Join("internal", "simd", "kernels_scalar.go"), "Min2ReducedRow64")
	if err != nil || len(spans) == 0 {
		t.Fatalf("findUnrolledLoop4Spans(Min2ReducedRow64) = (%v, %v), want non-empty", spans, err)
	}
	loopLine := spans[0].start + 1

	diags := parsedCompilerDiags{
		canInline: map[string]int{
			"cuts.go:nextDown":                                           50,
			"presolve.go:isFractionalFloat64":                            60,
			"simplex.go:combineDualNoiseTol":                             80,
			"lu.go:(*LUFactorization).removeURowEntry":                   78,
			"lu.go:(*LUFactorization).removeUColEntry":                   78,
			"format/pulp_json_fast.go:(*pulpScanner).consumeByte":        65,
			"format/pulp_json_fast.go:(*pulpScanner).readUnescapedBytes": 76,
			"simplex.go:evalColDualDotWide":                              70,
		},
		cannotInline: map[string]string{
			"cuts.go:nextUp": "function too complex: cost 82 exceeds budget 80",
			"simplex.go:recomputeWorkingRowResidualWide": "marked go:noinline",
			"presolve.go:evalBinaryTightenRHSWide":       "marked go:noinline",
			"presolve.go:evalRowActivityWide":            "marked go:noinline",
			"heuristics.go:evalEpigraphCompObjWide":      "marked go:noinline",
		},
		bceByFile: map[string]map[int][]string{
			filepath.Join("internal", "simd", "kernels_scalar.go"): {
				loopLine: {"Found IsInBounds"},
			},
		},
	}

	var res invariantCheckResult
	if err := verifyStaticInvariants(repoRoot, false, diags, &res); err != nil {
		t.Fatalf("verifyStaticInvariants error: %v", err)
	}
	joined := strings.Join(res.Violations, "\n")
	for _, want := range []string{
		"inline lock failed for nextUp",
		"noinline lock failed for evalColDualDotWide",
		"BCE regression inside 4-way loop of Min2ReducedRow64",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing expected violation %q in:\n%s", want, joined)
		}
	}
}
