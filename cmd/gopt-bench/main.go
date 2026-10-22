package main

import (
	"bytes"
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

	"gopt"
	"gopt/bench"
	"gopt/format"
	"gopt/internal/simd"
)

func medianDurationMS(d []time.Duration) float64 {
	if len(d) == 0 {
		return 0
	}
	cp := slices.Clone(d)
	slices.Sort(cp)
	return float64(cp[len(cp)/2].Nanoseconds()) / 1e6
}

func meanSDDurationMS(d []time.Duration) (mean, sd float64) {
	if len(d) == 0 {
		return 0, 0
	}
	sum := 0.0
	for _, v := range d {
		sum += float64(v.Nanoseconds()) / 1e6
	}
	mean = sum / float64(len(d))
	if len(d) < 2 {
		return mean, 0
	}
	var sq float64
	for _, v := range d {
		diff := (float64(v.Nanoseconds()) / 1e6) - mean
		sq += diff * diff
	}
	sd = math.Sqrt(sq / float64(len(d)-1))
	return mean, sd
}

func durationsMS(d []time.Duration) []float64 {
	out := make([]float64, len(d))
	for i, v := range d {
		out[i] = float64(v.Nanoseconds()) / 1e6
	}
	return out
}

// gitProvenance returns the HEAD commit of the working tree and whether tracked or untracked
// files differ from it, so every telemetry file can be traced to the code that produced it.
func gitProvenance() (commit string, dirty bool) {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown", true
	}
	status, err := exec.Command("git", "status", "--porcelain").Output()
	return strings.TrimSpace(string(out)), err != nil || len(bytes.TrimSpace(status)) > 0
}

// cpuAffinity returns the Cpus_allowed_list of the current process (Linux), so telemetry
// records whether the run was pinned to specific cores.
func cpuAffinity() string {
	raw, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(raw), "\n") {
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

func warmWorker(corpusDir string, mpsBuf *bytes.Buffer) {
	ws := gopt.AcquireWorkspace(4028, 5000)
	gopt.ReleaseWorkspace(ws)
	mpsBuf.Grow(1 << 20)

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
		_ = gopt.SolveCompiled(cp)
		mpsBuf.Reset()
		_ = format.WriteMPS(cp, mpsBuf)
	}
}

func runCheckpoint(compiledTrials []*gopt.CompiledProblem, opts gopt.Options, trials int) bench.CheckpointMetrics {
	if len(compiledTrials) == 0 {
		return bench.CheckpointMetrics{}
	}
	// Always perform an untimed warmup pass so even 1-trial Tier 3 checkpoints execute in warm state.
	_ = gopt.SolveCompiled(compiledTrials[0], opts)

	var canonical gopt.Result
	times := make([]time.Duration, 0, trials)
	for i := range trials {
		p := compiledTrials[i%len(compiledTrials)]
		t0 := time.Now()
		res := gopt.SolveCompiled(p, opts)
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
			if err := os.WriteFile(reportPath, []byte(rendered), 0o644); err != nil {
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

func main() {
	corpusDir := flag.String("corpus", "testdata/corpus", "Directory for JSON benchmark corpus")
	genCorpus := flag.Bool("generate-corpus", false, "Generate deterministic 3-tier JSON corpus")
	pyRefPath := flag.String("python-ref", "bench/results/python_baseline.json", "Path to Python baseline JSON")
	outPath := flag.String("out", "", "Optional output JSON path for benchmark results")
	reportPath := flag.String("report", "", "Generate markdown benchmark report and sync docs from saved JSON files")
	syncDocs := flag.Bool("sync-docs", false, "Synchronize README.md, docs/benchmarks.md, and final_report.md from JSON telemetry")
	checkDocs := flag.Bool("check-docs", false, "Verify that README.md, docs/benchmarks.md, and final_report.md match JSON telemetry")
	scalarJSONPath := flag.String("scalar-json", "bench/results/gopt_scalar.json", "Scalar JSON path for report")
	simdJSONPath := flag.String("simd-json", "bench/results/gopt_simd.json", "SIMD JSON path for report")
	trials := flag.Int("trials", 11, "Number of measured trials per problem")
	workers := flag.Int("workers", 1, "Number of concurrent benchmark workers")
	flag.Parse()

	if *genCorpus {
		cases, err := bench.WriteCorpusJSON(*corpusDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to write corpus: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Generated %d benchmark instances (plus randomized trial ensembles) in %s\n", len(cases), *corpusDir)
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

	nWorkers := max(1, *workers)
	fmt.Printf("=== GOPT WARM-STATE MULTI-TRIAL BENCHMARK SUITE (SIMD=%v, Workers=%d) ===\n\n", simd.UsingHardwareSIMD(), nWorkers)
	fmt.Printf("%-18s | %5s %4s | %8s %8s | %8s %8s | %9s %9s | %7s | %7s\n",
		"Problem ID", "Vars", "Rows",
		"PyPrep", "GoPrep",
		"PySolve", "GoSolve",
		"PyTotal", "GoTotal",
		"Speedup", "Parity")
	fmt.Println(strings.Repeat("-", 109))

	type jobResult struct {
		idx          int
		res          bench.ProblemBenchResult
		canonicalRes gopt.Result
	}

	jobCh := make(chan int, len(cases))
	for idx := range cases {
		jobCh <- idx
	}
	close(jobCh)

	jobResults := make([]jobResult, len(cases))
	var wg sync.WaitGroup
	for range min(nWorkers, len(cases)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var mpsBuf bytes.Buffer
			warmWorker(*corpusDir, &mpsBuf)

			for idx := range jobCh {
				c := cases[idx]
				nTrials := *trials
				if c.Tier == "tier3_large" {
					nTrials = min(nTrials, 5)
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
				compiledTrials := make([]*gopt.CompiledProblem, 0, nTrials)

				var canonicalRes gopt.Result
				var canonicalCompiled *gopt.CompiledProblem

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
					res := gopt.SolveCompiled(compiled)
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
				var canonicalRootLP gopt.LPResult
				for i := range warmup + nTrials {
					cp := canonicalCompiled
					if i >= warmup {
						cp = compiledTrials[i-warmup]
					}
					ws := gopt.AcquireWorkspace(cp.NumVars, cp.NumConstraints)
					t0 := time.Now()
					lpSolver := gopt.NewLPSolver(cp, ws)
					lpRes := lpSolver.Solve()
					dt := time.Since(t0)
					gopt.ReleaseWorkspace(ws)
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
				cpTimeout := 250 * time.Millisecond

				cp4 := runCheckpoint(compiledTrials, gopt.Options{
					EnablePresolve:   false,
					EnableCuts:       false,
					EnableHeuristics: false,
					MaxNodes:         5000,
					RelGapTol:        1e-6,
					AbsGapTol:        1e-6,
					TimeLimit:        cpTimeout,
				}, cpTrials)

				cp5 := runCheckpoint(compiledTrials, gopt.Options{
					EnablePresolve:   true,
					EnableCuts:       false,
					EnableHeuristics: false,
					MaxNodes:         5000,
					RelGapTol:        1e-6,
					AbsGapTol:        1e-6,
					TimeLimit:        cpTimeout,
				}, cpTrials)

				cp6 := runCheckpoint(compiledTrials, gopt.Options{
					EnablePresolve:   true,
					EnableCuts:       true,
					EnableHeuristics: false,
					MaxNodes:         5000,
					MaxCutPasses:     12,
					RelGapTol:        1e-6,
					AbsGapTol:        1e-6,
					TimeLimit:        cpTimeout,
				}, cpTrials)

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
						SIMDEnabled:       simd.UsingHardwareSIMD(),
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
		}()
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
				if diff > 1e-5 {
					parityStr = fmt.Sprintf("%.1e", diff)
				}
				for k, wantPtr := range ref.TrialObjectives {
					if wantPtr != nil && k < len(r.TrialObjectives) {
						td := math.Abs(r.TrialObjectives[k]-*wantPtr) / max(1.0, math.Abs(*wantPtr))
						if td > 1e-5 {
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
		if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
			panic(err)
		}
		rawOut, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			panic(err)
		}
		if err := os.WriteFile(*outPath, rawOut, 0o644); err != nil {
			panic(err)
		}
		fmt.Printf("\nSaved benchmark telemetry to %s\n", *outPath)
	}
}
