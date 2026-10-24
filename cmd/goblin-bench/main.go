package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	gb "github.com/paulourio/goblin"
	"github.com/paulourio/goblin/assign"
	"github.com/paulourio/goblin/bench"
	"github.com/paulourio/goblin/format"
	"github.com/paulourio/goblin/internal/simd"
)

const (
	nsPerMS            = 1e6
	minSampleForSD     = 2
	warmMaxVars        = 4036
	warmMaxRows        = 5000
	warmAssignMaxDim   = 2000
	mpsInitBufBytes    = 1 << 20
	reportFilePerm     = 0o644
	outDirPerm         = 0o755
	defaultTrialsCount = 11
	tier3MaxTrials     = 5
	exitCodeBadFlag    = 2
	tableRuleWidth     = 109
	cpTimeoutMS        = 250
	cpMaxNodes         = 5000
	cpMaxCutPasses     = 12
	cpGapTol           = 1e-6
	parityObjectiveTol = 1e-5
	assignParityObjTol = 1e-9
)

func medianDurationMS(d []time.Duration) float64 {
	if len(d) == 0 {
		return 0
	}
	cp := slices.Clone(d)
	slices.Sort(cp)
	return float64(cp[len(cp)/2].Nanoseconds()) / nsPerMS
}

func meanSDDurationMS(d []time.Duration) (mean, sd float64) {
	if len(d) == 0 {
		return 0, 0
	}
	sum := 0.0
	for _, v := range d {
		sum += float64(v.Nanoseconds()) / nsPerMS
	}
	mean = sum / float64(len(d))
	if len(d) < minSampleForSD {
		return mean, 0
	}
	var sq float64
	for _, v := range d {
		diff := (float64(v.Nanoseconds()) / nsPerMS) - mean
		sq += diff * diff
	}
	sd = math.Sqrt(sq / float64(len(d)-1))
	return mean, sd
}

func durationsMS(d []time.Duration) []float64 {
	out := make([]float64, len(d))
	for i, v := range d {
		out[i] = float64(v.Nanoseconds()) / nsPerMS
	}
	return out
}

// Read the HEAD commit hash of the working tree and report whether any tracked
// or untracked files differ from HEAD. Every saved JSON benchmark entry stores
// both fields alongside the source hash so downstream checks reject stale or
// mixed-commit telemetry before rendering markdown tables. Always pin runs to a
// single core.
func gitProvenance() (commit string, dirty bool) {
	ctx := context.Background()
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown", true
	}
	status, err := exec.CommandContext(ctx, "git", "status", "--porcelain").Output()
	return strings.TrimSpace(string(out)), err != nil || len(bytes.TrimSpace(status)) > 0
}

// Read Cpus_allowed_list from /proc/self/status on Linux so the saved benchmark
// JSON records the exact core mask used during the run.
func cpuAffinity() string {
	raw, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return "unknown"
	}
	for line := range strings.SplitSeq(string(raw), "\n") {
		if v, ok := strings.CutPrefix(line, "Cpus_allowed_list:"); ok {
			return strings.TrimSpace(v)
		}
	}
	return "unknown"
}

func loadTrialPayloads(corpusDir, id string, nTrials int) ([][]byte, error) {
	trialsPath := filepath.Join(corpusDir, "trials", id+".jsonl")
	if rawTrials, err := os.ReadFile(trialsPath); err == nil {
		lines := bytes.Split(bytes.TrimSpace(rawTrials), []byte("\n"))
		payloads := make([][]byte, 0, nTrials)
		for _, line := range lines {
			trimmed := bytes.TrimSpace(line)
			if len(trimmed) > 0 {
				payloads = append(payloads, trimmed)
			}
		}
		if nBase := len(payloads); nBase > 0 {
			for len(payloads) < nTrials {
				payloads = append(payloads, payloads[len(payloads)%nBase])
			}
			return payloads[:nTrials], nil
		}
	}
	jsonPath := filepath.Join(corpusDir, id+".json")
	rawBytes, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, err
	}
	payloads := make([][]byte, nTrials)
	for i := range nTrials {
		payloads[i] = rawBytes
	}
	return payloads, nil
}

func warmWorker(corpusDir string, mpsBuf *bytes.Buffer, opts ...gb.Option) {
	ws := gb.AcquireWorkspace(warmMaxVars, warmMaxRows)
	gb.ReleaseWorkspace(ws)
	mpsBuf.Grow(mpsInitBufBytes)

	for range 5 {
		for _, id := range []string{"t1_whiskas", "t2_mkp_30"} {
			p := filepath.Join(corpusDir, id+".json")
			raw, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			doc, err := format.ParsePuLPJSON(raw)
			if err != nil {
				continue
			}
			m, err := format.BuildModelFromPuLPJSON(doc)
			if err != nil {
				continue
			}
			cp := m.Compile()
			_, _ = gb.SolveCompiled(context.Background(), cp, opts...)
			mpsBuf.Reset()
			_ = format.WriteMPS(cp, mpsBuf)
		}
	}
}

func runCheckpoint(compiledTrials []*gb.CompiledProblem, trials int, opts ...gb.Option) bench.CheckpointMetrics {
	if len(compiledTrials) == 0 {
		return bench.CheckpointMetrics{}
	}
	ctx := context.Background()
	// Always perform an untimed warmup pass so even 1-trial Tier 3 checkpoints execute in warm state.
	_, _ = gb.SolveCompiled(ctx, compiledTrials[0], opts...)

	var canonical gb.Result
	times := make([]time.Duration, 0, trials)
	for i := range trials {
		p := compiledTrials[i%len(compiledTrials)]
		t0 := time.Now()
		res, err := gb.SolveCompiled(ctx, p, opts...)
		if err != nil {
			panic(err)
		}
		dt := time.Since(t0)
		if i == 0 {
			canonical = res
		}
		times = append(times, dt)
	}
	return bench.CheckpointMetrics{
		Status:     canonical.Status.String(),
		Objective:  canonical.Objective,
		BestBound:  canonical.BestBound,
		Nodes:      canonical.Nodes,
		Iterations: canonical.Iterations,
		CutsAdded:  canonical.CutsAdded,
		TimeMS:     medianDurationMS(times),
	}
}

func syncAllDocs(corpusDir, pyPath, scalarPath, simdPath, reportPath string, checkOnly bool) error {
	tb, err := bench.LoadTelemetryBundle(pyPath, scalarPath, simdPath)
	if err != nil {
		return err
	}
	if err := bench.ValidateTelemetryAgainstCorpus(corpusDir, tb); err != nil {
		return err
	}
	if err := bench.CheckTelemetryFreshness(".", tb); err != nil {
		return err
	}
	if reportPath != "" {
		rendered, err := bench.BuildFinalReportMarkdown(tb)
		if err != nil {
			return err
		}
		if checkOnly {
			raw, err := os.ReadFile(reportPath)
			if err != nil {
				return err
			}
			if string(raw) != rendered {
				return fmt.Errorf("%s is out of sync with canonical telemetry", reportPath)
			}
		} else {
			if err := os.WriteFile(reportPath, []byte(rendered), reportFilePerm); err != nil {
				return err
			}
		}
	}
	targets := []string{"README.md", "docs/benchmarks.md", "bench/results/final_report.md"}
	for _, path := range targets {
		if _, err := os.Stat(path); err == nil {
			if err := bench.SyncMarkdownFile(path, tb, checkOnly); err != nil {
				return err
			}
		} else if checkOnly {
			return fmt.Errorf("missing required documentation file %s", path)
		}
	}
	return nil
}

func parseBackend(name string) (gb.Backend, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "go", "purego", "pure-go":
		return gb.BackendGo, nil
	case "cbc", "cpp", "c++":
		return gb.BackendCBC, nil
	default:
		return gb.BackendGo, fmt.Errorf("unknown -backend %q (want 'go' or 'cbc')", name)
	}
}

func runAssignBenchSuite(scipyRefPath, outPath string, noSIMD bool, defaultTrials int) {
	var assignOpts []assign.Option
	if noSIMD {
		assignOpts = append(assignOpts, assign.WithoutSIMD())
	}
	var scipyRef map[string]bench.SciPyAssignEntry
	if rawRef, err := os.ReadFile(scipyRefPath); err == nil {
		_ = json.Unmarshal(rawRef, &scipyRef)
	}

	assignCases := bench.BuildAssignCorpus()
	results := make(map[string]bench.AssignBenchResult, len(assignCases))
	gitCommit, gitDirty := gitProvenance()
	fingerprint, err := bench.SourceFingerprint(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "source fingerprint: %v\n", err)
		os.Exit(1)
	}
	if gitDirty && outPath != "" {
		fmt.Fprintf(os.Stderr, "warning: working tree is dirty; telemetry will be marked git_dirty=true\n")
	}

	// Warm up workspace pool and solver paths before timed trials.
	ctx := context.Background()
	ws := assign.AcquireWorkspace(warmAssignMaxDim, warmAssignMaxDim)
	assign.ReleaseWorkspace(ws)
	for range 5 {
		for _, warmID := range []string{"a1_unif_10x10", "a2_unif_100x100", "a2_unif_256x256"} {
			for _, ac := range assignCases {
				if ac.ID == warmID {
					mat := bench.GenerateAssignMatrix(ac, 0)
					_, _ = assign.LinearSumFlat(ctx, ac.Rows, ac.Cols, mat, assign.Minimize, assignOpts...)
					_, _ = assign.LinearSumFlat(ctx, ac.Rows, ac.Cols, mat, assign.Maximize, assignOpts...)
				}
			}
		}
	}

	simdActive := simd.UsingHardwareSIMDWith(noSIMD)
	fmt.Printf("=== GOBLIN ASSIGN WARM-STATE BENCHMARK SUITE (SIMD=%v) ===\n\n", simdActive)
	fmt.Printf("%-18s | %9s | %10s %10s | %10s %10s | %8s | %7s\n",
		"Problem ID", "Shape",
		"SciPySolve", "GoSolve",
		"SciPyTotal", "GoTotal",
		"Speedup", "Parity")
	fmt.Println(strings.Repeat("-", tableRuleWidth))

	for _, ac := range assignCases {
		nTrials := defaultTrials
		if ac.Tier == "tier3_large" {
			nTrials = min(nTrials, tier3MaxTrials)
		}
		warmup := 2
		if ac.Tier == "tier3_large" {
			warmup = 1
		}

		tPrep := make([]time.Duration, 0, nTrials)
		tSolve := make([]time.Duration, 0, nTrials)
		tTotal := make([]time.Duration, 0, nTrials)
		trialObjs := make([]float64, 0, nTrials)

		for i := range warmup + nTrials {
			trialIdx := 0
			if i >= warmup {
				trialIdx = i - warmup
			}
			t0 := time.Now()
			mat := bench.GenerateAssignMatrix(ac, trialIdx)
			t1 := time.Now()
			res, err := assign.LinearSumFlat(ctx, ac.Rows, ac.Cols, mat, ac.Sense, assignOpts...)
			if err != nil {
				panic(fmt.Sprintf("%s trial %d failed: %v", ac.ID, trialIdx, err))
			}
			t2 := time.Now()

			if i >= warmup {
				tPrep = append(tPrep, t1.Sub(t0))
				tSolve = append(tSolve, t2.Sub(t1))
				tTotal = append(tTotal, t2.Sub(t0))
				trialObjs = append(trialObjs, res.Objective)
			}
		}

		goPrepMS := medianDurationMS(tPrep)
		goSolveMS := medianDurationMS(tSolve)
		goTotalMS := medianDurationMS(tTotal)
		solveMeanMS, solveSDMS := meanSDDurationMS(tSolve)
		totalMeanMS, totalSDMS := meanSDDurationMS(tTotal)

		r := bench.AssignBenchResult{
			ID:                ac.ID,
			Tier:              ac.Tier,
			Rows:              ac.Rows,
			Cols:              ac.Cols,
			Sense:             int(ac.Sense),
			Family:            ac.Family,
			SIMDEnabled:       simdActive,
			Objective:         trialObjs[0],
			TrialObjectives:   trialObjs,
			TPrepMS:           goPrepMS,
			TSolveMS:          goSolveMS,
			TTotalMS:          goTotalMS,
			TSolveMeanMS:      solveMeanMS,
			TSolveSDMS:        solveSDMS,
			TTotalMeanMS:      totalMeanMS,
			TTotalSDMS:        totalSDMS,
			TSolveTrialsMS:    durationsMS(tSolve),
			TTotalTrialsMS:    durationsMS(tTotal),
			GitCommit:         gitCommit,
			GitDirty:          gitDirty,
			GoVersion:         runtime.Version(),
			SourceFingerprint: fingerprint,
			CPUAffinity:       cpuAffinity(),
		}
		results[ac.ID] = r

		spSolveMS, spTotalMS := 0.0, 0.0
		speedupStr := "N/A"
		parityStr := "OK"
		if ref, ok := scipyRef[ac.ID]; ok {
			spSolveMS = ref.TSolveMedianMS
			spTotalMS = ref.TTotalMedianMS
			if goSolveMS > 0 {
				speedupStr = fmt.Sprintf("%.2fx", spSolveMS/goSolveMS)
			}
			diff := math.Abs(r.Objective-ref.Objective) / max(1.0, math.Abs(ref.Objective))
			if diff > assignParityObjTol {
				parityStr = fmt.Sprintf("%.1e", diff)
			}
			for k, want := range ref.TrialObjectives {
				if k < len(r.TrialObjectives) {
					td := math.Abs(r.TrialObjectives[k]-want) / max(1.0, math.Abs(want))
					if td > assignParityObjTol {
						parityStr = fmt.Sprintf("T%d:%.1e", k, td)
						break
					}
				}
			}
		}

		shapeStr := fmt.Sprintf("%dx%d", ac.Rows, ac.Cols)
		fmt.Printf("%-18s | %9s | %8.4fms %8.4fms | %8.4fms %8.4fms | %8s | %7s\n",
			ac.ID, shapeStr,
			spSolveMS, goSolveMS,
			spTotalMS, goTotalMS,
			speedupStr, parityStr)
	}

	if outPath != "" {
		if err := os.MkdirAll(filepath.Dir(outPath), outDirPerm); err != nil {
			panic(err)
		}
		rawOut, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			panic(err)
		}
		if err := os.WriteFile(outPath, rawOut, reportFilePerm); err != nil {
			panic(err)
		}
		fmt.Printf("\nSaved assign benchmark telemetry to %s\n", outPath)
	}
}

func main() {
	corpusDir := flag.String("corpus", "testdata/corpus", "Directory for JSON benchmark corpus")
	genCorpus := flag.Bool("generate-corpus", false, "Generate deterministic 3-tier JSON corpus")
	assignFlag := flag.Bool("assign", false, "Run Linear Sum Assignment (LSAP) benchmark suite")
	pyRefPath := flag.String("python-ref", "bench/results/python_baseline.json", "Path to Python baseline JSON")
	scipyRefPath := flag.String("scipy-ref", "bench/results/scipy_assign_baseline.json", "Path to SciPy LSAP baseline JSON")
	outPath := flag.String("out", "", "Optional output JSON path for benchmark results")
	reportPath := flag.String("report", "", "Generate markdown benchmark report and sync docs from saved JSON files")
	syncDocs := flag.Bool("sync-docs", false, "Synchronize README.md, docs/benchmarks.md, and final_report.md from JSON telemetry")
	checkDocs := flag.Bool("check-docs", false, "Verify that README.md, docs/benchmarks.md, and final_report.md match JSON telemetry")
	scalarJSONPath := flag.String("scalar-json", "bench/results/goblin_scalar.json", "Scalar JSON path for report")
	simdJSONPath := flag.String("simd-json", "bench/results/goblin_simd.json", "SIMD JSON path for report")
	backendFlag := flag.String("backend", "go", "Solver backend to benchmark ('go' or 'cbc')")
	noSIMD := flag.Bool("no-simd", false, "Disable hardware SIMD kernels via gb.WithoutSIMD()")
	trials := flag.Int("trials", defaultTrialsCount, "Number of measured trials per problem")
	workers := flag.Int("workers", 1, "Number of concurrent benchmark workers")
	flag.Parse()

	if *genCorpus {
		cases, err := bench.WriteCorpusJSON(*corpusDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to write corpus: %v\n", err)
			os.Exit(1)
		}
		assignDir := filepath.Join(filepath.Dir(*corpusDir), "assign_corpus")
		assignCases, err := bench.WriteAssignCorpusManifest(assignDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to write assign corpus manifest: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Generated %d MILP benchmark families in %s and %d LSAP families in %s\n",
			len(cases), *corpusDir, len(assignCases), assignDir)
		return
	}

	if *assignFlag {
		runAssignBenchSuite(*scipyRefPath, *outPath, *noSIMD, *trials)
		return
	}

	if *checkDocs {
		rep := *reportPath
		if rep == "" {
			rep = "bench/results/final_report.md"
		}
		if err := syncAllDocs(*corpusDir, *pyRefPath, *scalarJSONPath, *simdJSONPath, rep, true); err != nil {
			fmt.Fprintf(os.Stderr, "documentation sync check failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Documentation and benchmark tables are in sync with canonical telemetry.")
		return
	}

	if *syncDocs || *reportPath != "" {
		rep := *reportPath
		if rep == "" {
			rep = "bench/results/final_report.md"
		}
		if err := syncAllDocs(*corpusDir, *pyRefPath, *scalarJSONPath, *simdJSONPath, rep, false); err != nil {
			fmt.Fprintf(os.Stderr, "failed to synchronize documentation: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Synchronized benchmark report (%s), README.md, and docs/benchmarks.md\n", rep)
		return
	}

	backend, err := parseBackend(*backendFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(exitCodeBadFlag)
	}
	if backend == gb.BackendCBC && !gb.CBCAvailable() {
		fmt.Fprintln(os.Stderr, "CBC backend requested (-backend cbc), but binary was built with CGO_ENABLED=0")
		os.Exit(1)
	}

	var baseOpts []gb.Option
	if backend == gb.BackendCBC {
		baseOpts = append(baseOpts, gb.WithCBC())
	} else {
		baseOpts = append(baseOpts, gb.WithPureGo())
	}
	if *noSIMD {
		baseOpts = append(baseOpts, gb.WithoutSIMD())
	}

	var pyRef map[string]bench.PythonBaselineEntry
	if rawRef, err := os.ReadFile(*pyRefPath); err == nil {
		_ = json.Unmarshal(rawRef, &pyRef)
	}

	cases := bench.BuildCorpus()
	results := make(map[string]bench.ProblemBenchResult, len(cases))
	gitCommit, gitDirty := gitProvenance()
	fingerprint, err := bench.SourceFingerprint(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "source fingerprint: %v\n", err)
		os.Exit(1)
	}
	if gitDirty && *outPath != "" {
		fmt.Fprintf(os.Stderr, "warning: working tree is dirty; telemetry will be marked git_dirty=true\n")
	}

	simdActive := backend == gb.BackendGo && simd.UsingHardwareSIMDWith(*noSIMD)
	nWorkers := max(1, *workers)
	fmt.Printf("=== GOBLIN WARM-STATE MULTI-TRIAL BENCHMARK SUITE (Backend=%s, SIMD=%v, Workers=%d) ===\n\n",
		backend, simdActive, nWorkers)
	fmt.Printf("%-18s | %5s %4s | %8s %8s | %8s %8s | %9s %9s | %7s | %7s\n",
		"Problem ID", "Vars", "Rows",
		"PyPrep", "GoPrep",
		"PySolve", "GoSolve",
		"PyTotal", "GoTotal",
		"Speedup", "Parity")
	fmt.Println(strings.Repeat("-", tableRuleWidth))

	type jobResult struct {
		idx          int
		res          bench.ProblemBenchResult
		canonicalRes gb.Result
	}

	jobCh := make(chan int, len(cases))
	for idx := range cases {
		jobCh <- idx
	}
	close(jobCh)

	jobResults := make([]jobResult, len(cases))
	var wg sync.WaitGroup
	for range min(nWorkers, len(cases)) {
		wg.Go(func() {
			var mpsBuf bytes.Buffer
			warmWorker(*corpusDir, &mpsBuf, baseOpts...)

			for idx := range jobCh {
				c := cases[idx]
				nTrials := *trials
				if c.Tier == "tier3_large" {
					nTrials = min(nTrials, tier3MaxTrials)
				}
				warmup := 2
				if c.Tier == "tier3_large" {
					warmup = 1
				}

				trialPayloads, err := loadTrialPayloads(*corpusDir, c.ID, nTrials)
				if err != nil {
					fmt.Fprintf(os.Stderr, "missing %s: %v\n", c.ID, err)
					os.Exit(1)
				}

				tParse := make([]time.Duration, 0, nTrials)
				tBuild := make([]time.Duration, 0, nTrials)
				tCompile := make([]time.Duration, 0, nTrials)
				tMPS := make([]time.Duration, 0, nTrials)
				tSolve := make([]time.Duration, 0, nTrials)
				tTotal := make([]time.Duration, 0, nTrials)
				trialObjs := make([]float64, 0, nTrials)
				compiledTrials := make([]*gb.CompiledProblem, 0, nTrials)

				var canonicalRes gb.Result
				var canonicalCompiled *gb.CompiledProblem

				for i := range warmup + nTrials {
					rawBytes := trialPayloads[0]
					if i >= warmup {
						rawBytes = trialPayloads[i-warmup]
					}

					t0 := time.Now()
					doc, err := format.ParsePuLPJSON(rawBytes)
					if err != nil {
						panic(err)
					}
					t1 := time.Now()
					model, err := format.BuildModelFromPuLPJSON(doc)
					if err != nil {
						panic(err)
					}
					t2 := time.Now()
					compiled := model.Compile()
					t3 := time.Now()
					res, err := gb.SolveCompiled(context.Background(), compiled, baseOpts...)
					if err != nil {
						panic(err)
					}
					t4 := time.Now()

					mpsBuf.Reset()
					if err := format.WriteMPS(compiled, &mpsBuf); err != nil {
						panic(err)
					}
					t5 := time.Now()

					if i >= warmup {
						trialIdx := i - warmup
						if trialIdx == 0 {
							canonicalCompiled = compiled
							canonicalRes = res
						}
						compiledTrials = append(compiledTrials, compiled)
						trialObjs = append(trialObjs, res.Objective)
						tParse = append(tParse, t1.Sub(t0))
						tBuild = append(tBuild, t2.Sub(t1))
						tCompile = append(tCompile, t3.Sub(t2))
						tSolve = append(tSolve, t4.Sub(t3))
						tTotal = append(tTotal, t4.Sub(t0))
						tMPS = append(tMPS, t5.Sub(t4))
					}
				}

				// Measure Root LP Relaxation (Checkpoint 2/3) across warm randomized trials
				lpTimes := make([]time.Duration, 0, nTrials)
				var canonicalRootLP gb.LPResult
				for i := range warmup + nTrials {
					cp := canonicalCompiled
					if i >= warmup {
						cp = compiledTrials[i-warmup]
					}
					var dt time.Duration
					var lpRes gb.LPResult
					if backend == gb.BackendCBC {
						t0 := time.Now()
						lpSolver := gb.NewCBCLPSolver(cp)
						lpRes = lpSolver.Solve()
						dt = time.Since(t0)
						lpSolver.Close()
					} else {
						ws := gb.AcquireWorkspace(cp.NumVars, cp.NumConstraints)
						t0 := time.Now()
						lpSolver := gb.NewLPSolver(cp, ws)
						lpSolver.DisableSIMD = *noSIMD
						lpRes = lpSolver.Solve()
						dt = time.Since(t0)
						gb.ReleaseWorkspace(ws)
					}
					if i >= warmup {
						if i == warmup {
							canonicalRootLP = lpRes
						}
						lpTimes = append(lpTimes, dt)
					}
				}

				cpTrials := 3
				if c.Tier == "tier3_large" {
					cpTrials = 1
				}
				cpTimeout := cpTimeoutMS * time.Millisecond

				cp4 := runCheckpoint(compiledTrials, cpTrials,
					gb.WithBackend(backend),
					gb.WithSIMD(!*noSIMD),
					gb.WithPresolve(false),
					gb.WithCuts(false),
					gb.WithHeuristics(false),
					gb.WithMaxNodes(cpMaxNodes),
					gb.WithRelGapTol(cpGapTol),
					gb.WithAbsGapTol(cpGapTol),
					gb.WithTimeLimit(cpTimeout),
				)

				cp5 := runCheckpoint(compiledTrials, cpTrials,
					gb.WithBackend(backend),
					gb.WithSIMD(!*noSIMD),
					gb.WithPresolve(true),
					gb.WithCuts(false),
					gb.WithHeuristics(false),
					gb.WithMaxNodes(cpMaxNodes),
					gb.WithRelGapTol(cpGapTol),
					gb.WithAbsGapTol(cpGapTol),
					gb.WithTimeLimit(cpTimeout),
				)

				cp6 := runCheckpoint(compiledTrials, cpTrials,
					gb.WithBackend(backend),
					gb.WithSIMD(!*noSIMD),
					gb.WithPresolve(true),
					gb.WithCuts(true),
					gb.WithHeuristics(false),
					gb.WithMaxNodes(cpMaxNodes),
					gb.WithMaxCutPasses(cpMaxCutPasses),
					gb.WithRelGapTol(cpGapTol),
					gb.WithAbsGapTol(cpGapTol),
					gb.WithTimeLimit(cpTimeout),
				)

				cp7 := bench.CheckpointMetrics{
					Status:     canonicalRes.Status.String(),
					Objective:  canonicalRes.Objective,
					BestBound:  canonicalRes.BestBound,
					Nodes:      canonicalRes.Nodes,
					Iterations: canonicalRes.Iterations,
					CutsAdded:  canonicalRes.CutsAdded,
					TimeMS:     medianDurationMS(tSolve),
				}

				goParseMS := medianDurationMS(tParse)
				goBuildMS := medianDurationMS(tBuild)
				goCompileMS := medianDurationMS(tCompile)
				goSolveMS := medianDurationMS(tSolve)
				goTotalMS := medianDurationMS(tTotal)
				goMeanMS, goSDMS := meanSDDurationMS(tTotal)

				jobResults[idx] = jobResult{
					idx:          idx,
					canonicalRes: canonicalRes,
					res: bench.ProblemBenchResult{
						ID:                c.ID,
						Tier:              c.Tier,
						NumVars:           canonicalCompiled.NumVars,
						NumConstraints:    canonicalCompiled.NumConstraints,
						IsMIP:             canonicalCompiled.IsMIP,
						Backend:           backend.String(),
						SIMDEnabled:       simdActive,
						TrialObjectives:   trialObjs,
						TParseMS:          goParseMS,
						TBuildMS:          goBuildMS,
						TCompileMS:        goCompileMS,
						TMPSWriteMS:       medianDurationMS(tMPS),
						TSolveMS:          goSolveMS,
						TTotalMS:          goTotalMS,
						TTotalMeanMS:      goMeanMS,
						TTotalSDMS:        goSDMS,
						TSolveTrialsMS:    durationsMS(tSolve),
						TTotalTrialsMS:    durationsMS(tTotal),
						GitCommit:         gitCommit,
						GitDirty:          gitDirty,
						GoVersion:         runtime.Version(),
						SourceFingerprint: fingerprint,
						CPUAffinity:       cpuAffinity(),
						RootLPTimeMS:      medianDurationMS(lpTimes),
						RootLPIters:       canonicalRootLP.Iterations,
						RootLPObj:         canonicalRootLP.Objective,
						CP4Baseline:       cp4,
						CP5Presolve:       cp5,
						CP6Cuts:           cp6,
						CP7Full:           cp7,
					},
				}
			}
		})
	}
	wg.Wait()

	for idx, c := range cases {
		jr := jobResults[idx]
		r := jr.res
		canonicalRes := jr.canonicalRes
		goPrepMS := r.TParseMS + r.TBuildMS + r.TCompileMS
		goSolveMS := r.TSolveMS
		goTotalMS := r.TTotalMS

		pyPrepMS, pySolveMS, pyTotalMS := 0.0, 0.0, 0.0
		speedupStr := "N/A"
		parityStr := "OK"

		if ref, ok := pyRef[c.ID]; ok {
			pyPrepMS = ref.TParseMedianMS + ref.TBuildMedianMS + ref.TFormatMedianMS
			pySolveMS = ref.TSolveMedianMS
			pyTotalMS = ref.TTotalMedianMS
			if goTotalMS > 0 {
				speedupStr = fmt.Sprintf("%.1fx", pyTotalMS/goTotalMS)
			}
			if canonicalRes.Status.String() != ref.Status {
				parityStr = "STAT!"
			} else if ref.Objective != nil {
				diff := math.Abs(canonicalRes.Objective-*ref.Objective) / max(1.0, math.Abs(*ref.Objective))
				if diff > parityObjectiveTol {
					parityStr = fmt.Sprintf("%.1e", diff)
				}
				for k, wantPtr := range ref.TrialObjectives {
					if wantPtr != nil && k < len(r.TrialObjectives) {
						td := math.Abs(r.TrialObjectives[k]-*wantPtr) / max(1.0, math.Abs(*wantPtr))
						if td > parityObjectiveTol {
							parityStr = fmt.Sprintf("T%d:%.1e", k, td)
							break
						}
					}
				}
			}
		}

		fmt.Printf("%-18s | %5d %4d | %6.3fms %6.3fms | %6.2fms %6.2fms | %7.2fms %7.2fms | %7s | %7s\n",
			c.ID, r.NumVars, r.NumConstraints,
			pyPrepMS, goPrepMS,
			pySolveMS, goSolveMS,
			pyTotalMS, goTotalMS,
			speedupStr, parityStr)

		results[c.ID] = r
	}

	if *outPath != "" {
		if err := os.MkdirAll(filepath.Dir(*outPath), outDirPerm); err != nil {
			panic(err)
		}
		rawOut, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			panic(err)
		}
		if err := os.WriteFile(*outPath, rawOut, reportFilePerm); err != nil {
			panic(err)
		}
		fmt.Printf("\nSaved benchmark telemetry to %s\n", *outPath)
	}
}
