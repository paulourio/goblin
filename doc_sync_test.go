package gopt_test

import (
	"os"
	"testing"

	"gopt/bench"
)

func TestDocumentationAndBenchmarkSync(t *testing.T) {
	requiredDocs := []string{
		"README.md",
		"docs/index.md",
		"docs/modeling-and-formats.md",
		"docs/solver-internals.md",
		"docs/benchmarks.md",
		"bench/results/final_report.md",
	}
	for _, p := range requiredDocs {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("missing required documentation file %s: %v", p, err)
		}
		if info.Size() == 0 {
			t.Fatalf("documentation file %s is empty", p)
		}
	}

	tb, err := bench.LoadTelemetryBundle(
		"bench/results/python_baseline.json",
		"bench/results/gopt_scalar.json",
		"bench/results/gopt_simd.json",
	)
	if err != nil {
		t.Fatalf("failed to load canonical benchmark telemetry: %v", err)
	}

	if err := bench.ValidateTelemetryAgainstCorpus("testdata/corpus", tb); err != nil {
		t.Fatalf("corpus vs. telemetry validation failed: %v", err)
	}
	if err := bench.CheckTelemetryFreshness(".", tb); err != nil {
		t.Fatal(err)
	}

	syncedFiles := []string{
		"README.md",
		"docs/benchmarks.md",
		"bench/results/final_report.md",
	}
	for _, path := range syncedFiles {
		if err := bench.SyncMarkdownFile(path, tb, true); err != nil {
			t.Errorf("documentation sync gate failed: %v", err)
		}
	}

	expectedReport, err := bench.BuildFinalReportMarkdown(tb)
	if err != nil {
		t.Fatalf("BuildFinalReportMarkdown failed: %v", err)
	}
	rawReport, err := os.ReadFile("bench/results/final_report.md")
	if err != nil {
		t.Fatalf("read final_report.md: %v", err)
	}
	if string(rawReport) != expectedReport {
		t.Errorf("bench/results/final_report.md is out of sync with BuildFinalReportMarkdown (run `go run ./cmd/gopt-bench -sync-docs`)")
	}
}
