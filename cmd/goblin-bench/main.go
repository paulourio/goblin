// Command goblin-bench runs benchmark suites, checks parity, and updates benchmark documentation.
package main

import (
	"bytes"
	"context"
	"debug/buildinfo"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	gb "github.com/paulourio/goblin"
	"github.com/paulourio/goblin/assign"
	"github.com/paulourio/goblin/bench"
	"github.com/paulourio/goblin/format"
	"github.com/paulourio/goblin/internal/mad"
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
func gitProvenanceCtx(ctx context.Context) (commit string, dirty bool) {
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown", true
	}
	status, err := exec.CommandContext(
		ctx, "git", "status", "--porcelain", "--", ".", ":(exclude)bench/results",
	).Output()
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

func unameRelease() string {
	raw, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(raw))
}

func cpuModelName() string {
	raw, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return "unknown"
	}
	for line := range strings.SplitSeq(string(raw), "\n") {
		if after, ok := strings.CutPrefix(line, "model name"); ok {
			if _, val, found := strings.Cut(after, ":"); found {
				return strings.TrimSpace(val)
			}
		}
	}
	return "unknown"
}

func extractGOAMD64(settings []debug.BuildSetting, goArch string) string {
	for _, s := range settings {
		if s.Key == "GOAMD64" {
			return s.Value
		}
	}
	if goArch == "amd64" {
		return "v1"
	}
	return goArch
}

func goAMD64Level() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	return extractGOAMD64(bi.Settings, runtime.GOARCH)
}

type runProvenance struct {
	gitCommit   string
	gitDirty    bool
	fingerprint string
	cpuModel    string
	goAMD64     string
	cpuAffinity string
	goVersion   string
	kernelRel   string
	measuredAt  string
}

func collectRunProvenance(ctx context.Context) (runProvenance, error) {
	gitCommit, gitDirty := gitProvenanceCtx(ctx)
	fingerprint, err := bench.SourceFingerprint(".")
	if err != nil {
		return runProvenance{}, fmt.Errorf("source fingerprint: %w", err)
	}
	return runProvenance{
		gitCommit:   gitCommit,
		gitDirty:    gitDirty,
		fingerprint: fingerprint,
		cpuModel:    cpuModelName(),
		goAMD64:     goAMD64Level(),
		cpuAffinity: cpuAffinity(),
		goVersion:   runtime.Version(),
		kernelRel:   unameRelease(),
		measuredAt:  time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func tierTrialCounts(tier string, defaultTrials int) (nTrials, warmup int) {
	nTrials = defaultTrials
	warmup = 2
	if tier == "tier3_large" {
		nTrials = min(nTrials, tier3MaxTrials)
		warmup = 1
	}
	return nTrials, warmup
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
			m, err := format.FromPuLPJSONBytes(raw)
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
	cleanReport := ""
	if reportPath != "" {
		cleanReport = filepath.Clean(reportPath)
	}
	for _, path := range targets {
		if cleanReport != "" && filepath.Clean(path) == cleanReport {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			if err := bench.SyncMarkdownFile(path, tb, checkOnly); err != nil {
				return err
			}
		} else if checkOnly {
			return fmt.Errorf("missing required documentation file %s", path)
		}
	}
	if err := bench.SyncFormalDocs(".", checkOnly); err != nil {
		return err
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

func writeJSONReport(path string, v any, label string) {
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), outDirPerm); err != nil {
		panic(err)
	}
	rawOut, err := json.Marshal(v, json.Deterministic(true), jsontext.WithIndent("  "))
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(path, rawOut, reportFilePerm); err != nil {
		panic(err)
	}
	fmt.Printf("\nSaved %stelemetry to %s\n", label, path)
}

type assignTrialAcc struct {
	tPrep  []time.Duration
	tSolve []time.Duration
	tTotal []time.Duration
	objs   []float64
}

func newAssignTrialAcc(n int) assignTrialAcc {
	return assignTrialAcc{
		tPrep:  make([]time.Duration, 0, n),
		tSolve: make([]time.Duration, 0, n),
		tTotal: make([]time.Duration, 0, n),
		objs:   make([]float64, 0, n),
	}
}

func (a *assignTrialAcc) record(prepD, solveD, totalD time.Duration, obj float64) {
	a.tPrep = append(a.tPrep, prepD)
	a.tSolve = append(a.tSolve, solveD)
	a.tTotal = append(a.tTotal, totalD)
	a.objs = append(a.objs, obj)
}

func buildAssignResult(ac bench.AssignCorpusCase, simdActive bool, acc *assignTrialAcc, prov runProvenance) bench.AssignBenchResult {
	solveMeanMS, solveSDMS := meanSDDurationMS(acc.tSolve)
	totalMeanMS, totalSDMS := meanSDDurationMS(acc.tTotal)
	return bench.AssignBenchResult{
		ID:                ac.ID,
		Tier:              ac.Tier,
		Rows:              ac.Rows,
		Cols:              ac.Cols,
		Sense:             int(ac.Sense),
		Family:            ac.Family,
		SIMDEnabled:       simdActive,
		SIMDTier:          simd.ActiveSIMDTierWith(!simdActive),
		CPUModel:          prov.cpuModel,
		GOAMD64:           prov.goAMD64,
		Objective:         acc.objs[0],
		TrialObjectives:   acc.objs,
		TPrepMS:           medianDurationMS(acc.tPrep),
		TSolveMS:          medianDurationMS(acc.tSolve),
		TTotalMS:          medianDurationMS(acc.tTotal),
		TSolveMeanMS:      solveMeanMS,
		TSolveSDMS:        solveSDMS,
		TTotalMeanMS:      totalMeanMS,
		TTotalSDMS:        totalSDMS,
		TSolveTrialsMS:    durationsMS(acc.tSolve),
		TTotalTrialsMS:    durationsMS(acc.tTotal),
		GitCommit:         prov.gitCommit,
		GitDirty:          prov.gitDirty,
		GoVersion:         prov.goVersion,
		SourceFingerprint: prov.fingerprint,
		CPUAffinity:       prov.cpuAffinity,
		UnameRelease:      prov.kernelRel,
		MeasuredAt:        prov.measuredAt,
	}
}

func runAssignBenchSuite(scipyRefPath, outPath, pairedScalarOut string, noSIMD bool, defaultTrials int) {
	if defaultTrials <= 0 {
		fmt.Fprintf(os.Stderr, "-trials must be positive (got %d)\n", defaultTrials)
		os.Exit(1)
	}
	paired := pairedScalarOut != ""
	if paired && (noSIMD || !simd.UsingHardwareSIMDWith(false)) {
		fmt.Fprintln(os.Stderr, "-paired-scalar-out requires a GOEXPERIMENT=simd build on AVX2+FMA hardware without -no-simd")
		os.Exit(exitCodeBadFlag)
	}
	var assignOpts []assign.Option
	if noSIMD {
		assignOpts = append(assignOpts, assign.WithoutSIMD())
	}
	var scipyRef map[string]bench.SciPyAssignEntry
	if rawRef, err := os.ReadFile(scipyRefPath); err == nil {
		if err := json.Unmarshal(rawRef, &scipyRef); err != nil {
			fmt.Fprintf(os.Stderr, "parse %s: %v\n", scipyRefPath, err)
			os.Exit(1)
		}
	}

	assignCases := bench.AssignCorpusSpecs()
	results := make(map[string]bench.AssignBenchResult, len(assignCases))
	var scalarResults map[string]bench.AssignBenchResult
	if paired {
		scalarResults = make(map[string]bench.AssignBenchResult, len(assignCases))
	}
	ctx := context.Background()
	prov, err := collectRunProvenance(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if prov.gitDirty && (outPath != "" || pairedScalarOut != "") {
		fmt.Fprintf(os.Stderr, "warning: working tree is dirty; telemetry will be marked git_dirty=true\n")
	}

	// Warm up workspace pool and solver paths before timed trials.
	ws := assign.AcquireWorkspace(warmAssignMaxDim, warmAssignMaxDim)
	assign.ReleaseWorkspace(ws)
	for range 5 {
		for _, warmID := range []string{"a1_unif_10x10", "a2_unif_100x100", "a2_unif_256x256"} {
			for _, ac := range assignCases {
				if ac.ID == warmID {
					mat := bench.GenerateAssignMatrix(ac, 0)
					_, _ = assign.LinearSumFlat(ctx, ac.Rows, ac.Cols, mat, assign.Minimize, assignOpts...)
					_, _ = assign.LinearSumFlat(ctx, ac.Rows, ac.Cols, mat, assign.Maximize, assignOpts...)
					if paired {
						_, _ = assign.LinearSumFlat(ctx, ac.Rows, ac.Cols, mat, assign.Minimize, assign.WithoutSIMD())
						_, _ = assign.LinearSumFlat(ctx, ac.Rows, ac.Cols, mat, assign.Maximize, assign.WithoutSIMD())
					}
				}
			}
		}
	}

	simdActive := simd.UsingHardwareSIMDWith(noSIMD)
	fmt.Printf("=== GOBLIN ASSIGN WARM-STATE BENCHMARK SUITE (SIMD=%v, Tier=%s/%s, GOAMD64=%s, Paired=%v) ===\n",
		simdActive, simd.ActiveSIMDTierWith(noSIMD), simd.HardwareSIMDTier(), prov.goAMD64, paired)
	fmt.Printf("CPU: %s\n\n", prov.cpuModel)
	fmt.Printf("%-18s | %9s | %10s %10s | %10s %10s | %8s | %7s\n",
		"Problem ID", "Shape",
		"SciPySolve", "GoSolve",
		"SciPyTotal", "GoTotal",
		"Speedup", "Parity")
	fmt.Println(strings.Repeat("-", tableRuleWidth))

	scalarOpts := []assign.Option{assign.WithoutSIMD()}
	simdOpts := []assign.Option{assign.WithSIMD(true)}

	for _, ac := range assignCases {
		nTrials, warmup := tierTrialCounts(ac.Tier, defaultTrials)
		mainAcc := newAssignTrialAcc(nTrials)
		var scAcc assignTrialAcc
		if paired {
			scAcc = newAssignTrialAcc(nTrials)
		}

		runOne := func(trialIdx int, opts []assign.Option) (prepD, solveD, totalD time.Duration, obj float64) {
			t0 := time.Now()
			mat := bench.GenerateAssignMatrix(ac, trialIdx)
			t1 := time.Now()
			res, sErr := assign.LinearSumFlat(ctx, ac.Rows, ac.Cols, mat, ac.Sense, opts...)
			if sErr != nil {
				panic(fmt.Sprintf("%s trial %d failed: %v", ac.ID, trialIdx, sErr))
			}
			t2 := time.Now()
			return t1.Sub(t0), t2.Sub(t1), t2.Sub(t0), res.Objective
		}

		for i := range warmup + nTrials {
			trialIdx := 0
			if i >= warmup {
				trialIdx = i - warmup
			}
			if !paired {
				pD, sD, totD, obj := runOne(trialIdx, assignOpts)
				if i >= warmup {
					mainAcc.record(pD, sD, totD, obj)
				}
				continue
			}
			var pSc, sSc, totSc, pSm, sSm, totSm time.Duration
			var objSc, objSm float64
			if i%2 == 0 {
				pSc, sSc, totSc, objSc = runOne(trialIdx, scalarOpts)
				pSm, sSm, totSm, objSm = runOne(trialIdx, simdOpts)
			} else {
				pSm, sSm, totSm, objSm = runOne(trialIdx, simdOpts)
				pSc, sSc, totSc, objSc = runOne(trialIdx, scalarOpts)
			}
			if i >= warmup {
				scAcc.record(pSc, sSc, totSc, objSc)
				mainAcc.record(pSm, sSm, totSm, objSm)
			}
		}

		r := buildAssignResult(ac, simdActive, &mainAcc, prov)
		results[ac.ID] = r
		displayRes := r
		if paired {
			sr := buildAssignResult(ac, false, &scAcc, prov)
			sr.PairedABBA = true
			scalarResults[ac.ID] = sr
			displayRes = sr
		}

		spSolveMS, spTotalMS := 0.0, 0.0
		speedupStr := "N/A"
		parityStr := "OK"
		if ref, ok := scipyRef[ac.ID]; ok {
			spSolveMS = ref.TSolveMedianMS
			spTotalMS = ref.TTotalMedianMS
			if displayRes.TSolveMS > 0 {
				speedupStr = fmt.Sprintf("%.2fx", spSolveMS/displayRes.TSolveMS)
			}
			diff := math.Abs(displayRes.Objective-ref.Objective) / max(1.0, math.Abs(ref.Objective))
			if diff > assignParityObjTol {
				parityStr = fmt.Sprintf("%.1e", diff)
			}
			for k, want := range ref.TrialObjectives {
				if k < len(displayRes.TrialObjectives) {
					td := math.Abs(displayRes.TrialObjectives[k]-want) / max(1.0, math.Abs(want))
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
			spSolveMS, displayRes.TSolveMS,
			spTotalMS, displayRes.TTotalMS,
			speedupStr, parityStr)
	}

	writeJSONReport(pairedScalarOut, scalarResults, "assign scalar benchmark ")
	writeJSONReport(outPath, results, "assign benchmark ")
}

type madMILPAcc struct {
	times          []time.Duration
	statuses       []string
	canonSet       bool
	canonRes       gb.Result
	canonNumVars   int
	optimalCount   int
	timeLimitCount int
}

func (a *madMILPAcc) record(trialIdx int, dt time.Duration, res gb.Result, numVars int) {
	a.times = append(a.times, dt)
	a.statuses = append(a.statuses, res.Status.String())
	switch res.Status {
	case gb.StatusOptimal:
		a.optimalCount++
	case gb.StatusTimeLimit:
		a.timeLimitCount++
	}
	if trialIdx == 0 && !a.canonSet {
		a.canonSet = true
		a.canonRes = res
		a.canonNumVars = numVars
	}
}

func (a *madMILPAcc) metrics() bench.MADMILPMetrics {
	meanMS, sdMS := meanSDDurationMS(a.times)
	hasInc := len(a.canonRes.Values) == a.canonNumVars
	var objPtr *float64
	if hasInc && !math.IsNaN(a.canonRes.Objective) && !math.IsInf(a.canonRes.Objective, 0) {
		v := a.canonRes.Objective
		objPtr = &v
	} else {
		hasInc = false
	}
	hasBB := !math.IsNaN(a.canonRes.BestBound) && !math.IsInf(a.canonRes.BestBound, 0)
	bb := 0.0
	if hasBB {
		bb = a.canonRes.BestBound
	}
	return bench.MADMILPMetrics{
		Status:         a.canonRes.Status.String(),
		TrialStatuses:  a.statuses,
		HasIncumbent:   hasInc,
		HasBestBound:   hasBB,
		Objective:      objPtr,
		BestBound:      bb,
		Nodes:          a.canonRes.Nodes,
		Iterations:     a.canonRes.Iterations,
		TimeMS:         medianDurationMS(a.times),
		MeanMS:         meanMS,
		SDMS:           sdMS,
		TrialsMS:       durationsMS(a.times),
		OptimalCount:   a.optimalCount,
		TimeLimitCount: a.timeLimitCount,
	}
}

func solveAndCheckMADMILP(
	ctx context.Context,
	id string,
	trialIdx int,
	prob mad.Problem,
	optObj float64,
	backendLabel string,
	solveOpts ...gb.Option,
) (time.Duration, gb.Result, int) {
	m, buildErr := bench.BuildMADModel(prob)
	if buildErr != nil {
		panic(fmt.Sprintf("%s trial %d BuildMADModel(%s) failed: %v", id, trialIdx, backendLabel, buildErr))
	}
	t0 := time.Now()
	res, sErr := m.Solve(ctx, solveOpts...)
	dt := time.Since(t0)
	if sErr != nil {
		panic(fmt.Sprintf("%s trial %d MILP(%s) failed: %v", id, trialIdx, backendLabel, sErr))
	}
	if res.Status != gb.StatusOptimal && res.Status != gb.StatusTimeLimit {
		panic(fmt.Sprintf("%s trial %d MILP(%s) status=%v, want Optimal or TimeLimit", id, trialIdx, backendLabel, res.Status))
	}
	milpTol := parityObjectiveTol * max(1.0, math.Abs(optObj))
	bbFinite := !math.IsNaN(res.BestBound) && !math.IsInf(res.BestBound, 0)
	if bbFinite && res.BestBound > optObj+milpTol {
		panic(fmt.Sprintf("%s trial %d MILP(%s) BestBound %.6f > exact %.6f", id, trialIdx, backendLabel, res.BestBound, optObj))
	}
	hasInc := len(res.Values) == m.NumVars()
	if hasInc {
		if res.Objective < optObj-milpTol {
			panic(fmt.Sprintf("%s trial %d MILP(%s) incumbent %.6f < exact %.6f", id, trialIdx, backendLabel, res.Objective, optObj))
		}
		if bbFinite && res.BestBound > res.Objective+milpTol {
			panic(fmt.Sprintf("%s trial %d MILP(%s) BestBound %.6f > incumbent %.6f", id, trialIdx, backendLabel, res.BestBound, res.Objective))
		}
	}
	if res.Status == gb.StatusOptimal {
		if !hasInc {
			panic(fmt.Sprintf("%s trial %d MILP(%s) Optimal without full incumbent", id, trialIdx, backendLabel))
		}
		rel := math.Abs(res.Objective-optObj) / max(1.0, math.Abs(optObj))
		if rel > parityObjectiveTol {
			panic(fmt.Sprintf("%s trial %d MILP(%s) Optimal obj=%.6f != exact %.6f (rel=%.2e)", id, trialIdx, backendLabel, res.Objective, optObj, rel))
		}
	}
	return dt, res, m.NumVars()
}

func runMADBenchSuite(outPath string, defaultTrials int, noSIMD bool) {
	if defaultTrials <= 0 {
		fmt.Fprintf(os.Stderr, "-trials must be positive (got %d)\n", defaultTrials)
		os.Exit(1)
	}
	ctx := context.Background()
	prov, err := collectRunProvenance(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if prov.gitDirty && outPath != "" {
		fmt.Fprintf(os.Stderr, "warning: working tree is dirty; telemetry will be marked git_dirty=true\n")
	}

	madCases := bench.MADCorpusSpecs()
	cbcAvail := gb.CBCAvailable()
	simdActive := simd.UsingHardwareSIMDWith(noSIMD)

	for range 3 {
		for _, warmID := range []string{"pmad_base_g10_i2", "mad_comb_rand_g2_i4"} {
			for _, mc := range madCases {
				if mc.ID != warmID {
					continue
				}
				prob := bench.GenerateMADProblem(mc, 0)
				if m, bErr := bench.BuildMADModel(prob); bErr == nil {
					cp := m.Compile()
					_, _ = gb.SolveCompiled(ctx, cp)
					_, _ = gb.SolveCompiled(ctx, cp, gb.WithPureGo(), gb.WithCombinatorialSearch(false), gb.WithMaxNodes(0), gb.WithTimeLimit(cpTimeoutMS*time.Millisecond))
					if cbcAvail {
						_, _ = gb.SolveCompiled(ctx, cp, gb.WithCBC(), gb.WithTimeLimit(cpTimeoutMS*time.Millisecond))
					}
				}
				_, _ = mad.Solve(ctx, prob, mad.WithStrategy(mad.StrategyDP))
				_, _ = mad.Solve(ctx, prob, mad.WithStrategy(mad.StrategyBruteForce))
			}
		}
	}

	fmt.Printf("=== GOBLIN ISOLATED MAD COMBINATORIAL BENCHMARK SUITE (CBC=%v, SIMD=%v, GOAMD64=%s) ===\n",
		cbcAvail, simdActive, prov.goAMD64)
	fmt.Printf("CPU: %s\n\n", prov.cpuModel)
	fmt.Printf("%-22s | %7s | %8s | %8s | %8s | %12s | %12s | %7s\n",
		"Problem ID", "GxIxT", "AutoMed", "DPMed", "BFMed", "MILP-Go(250ms)", "MILP-CBC(250ms)", "Parity")
	fmt.Println(strings.Repeat("-", tableRuleWidth))

	results := make(map[string]bench.MADBenchResult, len(madCases))
	milpTimeout := cpTimeoutMS * time.Millisecond

	for _, mc := range madCases {
		nTrials, warmup := tierTrialCounts(mc.Tier, defaultTrials)
		if mc.Trials > 0 && defaultTrials == defaultTrialsCount {
			nTrials = mc.Trials
		}
		prob0 := bench.GenerateMADProblem(mc, 0)
		dfsStates, dpStates, dpWork := bench.MADPlanMetrics(prob0)
		runBF := dfsStates <= mad.MaxDFSStates

		autoTimes := make([]time.Duration, 0, nTrials)
		dpTimes := make([]time.Duration, 0, nTrials)
		var bfTimes []time.Duration
		if runBF {
			bfTimes = make([]time.Duration, 0, nTrials)
		}
		trialObjs := make([]float64, 0, nTrials)
		var autoStrategy string
		parityOK := true
		var milpGoAcc, milpCBCAcc madMILPAcc

		for i := range warmup + nTrials {
			trialIdx := 0
			if i >= warmup {
				trialIdx = i - warmup
			}
			prob := bench.GenerateMADProblem(mc, trialIdx)
			mComb, bErr := bench.BuildMADModel(prob)
			if bErr != nil {
				panic(fmt.Sprintf("%s trial %d BuildMADModel failed: %v", mc.ID, trialIdx, bErr))
			}
			cpComb := mComb.Compile()

			t0 := time.Now()
			autoRes, aErr := gb.SolveCompiled(ctx, cpComb)
			dAuto := time.Since(t0)
			if aErr != nil || autoRes.Status != gb.StatusOptimal {
				panic(fmt.Sprintf("%s trial %d Model.Solve(Auto) failed: status=%v err=%v", mc.ID, trialIdx, autoRes.Status, aErr))
			}
			if len(autoRes.Values) != cpComb.NumVars() {
				panic(fmt.Sprintf("%s trial %d Model.Solve(Auto) len(Values)=%d, want %d", mc.ID, trialIdx, len(autoRes.Values), cpComb.NumVars()))
			}

			t1 := time.Now()
			dpRes, dpErr := mad.Solve(ctx, prob, mad.WithStrategy(mad.StrategyDP))
			dDP := time.Since(t1)
			if dpErr != nil || dpRes.Status != mad.StatusOptimal {
				panic(fmt.Sprintf("%s trial %d MADDP failed: status=%v err=%v", mc.ID, trialIdx, dpRes.Status, dpErr))
			}
			if math.Abs(autoRes.Objective-dpRes.Objective)/max(1.0, math.Abs(dpRes.Objective)) > assignParityObjTol {
				panic(fmt.Sprintf("%s trial %d Model.Solve(Auto) %.12g != MADDP %.12g", mc.ID, trialIdx, autoRes.Objective, dpRes.Objective))
			}

			var dBF time.Duration
			if runBF {
				t2 := time.Now()
				bfRes, bfErr := mad.Solve(ctx, prob, mad.WithStrategy(mad.StrategyBruteForce))
				dBF = time.Since(t2)
				if bfErr != nil || bfRes.Status != mad.StatusOptimal {
					panic(fmt.Sprintf("%s trial %d MADBruteForce failed: status=%v err=%v", mc.ID, trialIdx, bfRes.Status, bfErr))
				}
				if math.Abs(bfRes.Objective-dpRes.Objective)/max(1.0, math.Abs(dpRes.Objective)) > assignParityObjTol {
					panic(fmt.Sprintf("%s trial %d MADBruteForce %.12g != MADDP %.12g", mc.ID, trialIdx, bfRes.Objective, dpRes.Objective))
				}
			}

			if mc.ID == "pmad_base_g10_i2" && trialIdx == 0 && math.Abs(dpRes.Objective-24137.0) > assignParityObjTol {
				panic(fmt.Sprintf("pmad_base_g10_i2 trial 0 obj=%.12g, want 24137", dpRes.Objective))
			}

			dGo, resGo, nVarsGo := solveAndCheckMADMILP(
				ctx, mc.ID, trialIdx, prob, dpRes.Objective, "go",
				gb.WithPureGo(), gb.WithCombinatorialSearch(false), gb.WithMaxNodes(0), gb.WithTimeLimit(milpTimeout),
			)

			var dCBC time.Duration
			var resCBC gb.Result
			var nVarsCBC int
			if cbcAvail {
				dCBC, resCBC, nVarsCBC = solveAndCheckMADMILP(
					ctx, mc.ID, trialIdx, prob, dpRes.Objective, "cbc",
					gb.WithCBC(), gb.WithTimeLimit(milpTimeout),
				)
			}

			if i >= warmup {
				if trialIdx == 0 {
					madAuto, maErr := mad.Solve(ctx, prob)
					if maErr != nil || madAuto.Status != mad.StatusOptimal {
						panic(fmt.Sprintf("%s trial 0 internal mad.Solve(Auto) failed: status=%v err=%v", mc.ID, madAuto.Status, maErr))
					}
					autoStrategy = madAuto.Strategy.String()
				}
				autoTimes = append(autoTimes, dAuto)
				dpTimes = append(dpTimes, dDP)
				if runBF {
					bfTimes = append(bfTimes, dBF)
				}
				trialObjs = append(trialObjs, dpRes.Objective)
				milpGoAcc.record(trialIdx, dGo, resGo, nVarsGo)
				if cbcAvail {
					milpCBCAcc.record(trialIdx, dCBC, resCBC, nVarsCBC)
				}
			}
		}

		autoMean, autoSD := meanSDDurationMS(autoTimes)
		dpMean, dpSD := meanSDDurationMS(dpTimes)
		var bfMed, bfMean, bfSD float64
		var bfTrialsMS []float64
		bfStr := "skip"
		if runBF {
			bfMed = medianDurationMS(bfTimes)
			bfMean, bfSD = meanSDDurationMS(bfTimes)
			bfTrialsMS = durationsMS(bfTimes)
			bfStr = fmt.Sprintf("%.3fms", bfMed)
		}

		goMetrics := milpGoAcc.metrics()
		var cbcMetricsPtr *bench.MADMILPMetrics
		cbcStr := "n/a"
		if cbcAvail {
			m := milpCBCAcc.metrics()
			cbcMetricsPtr = &m
			cbcStr = fmt.Sprintf("%.1f(%d/%d)", m.TimeMS, m.OptimalCount, nTrials)
		}
		goStr := fmt.Sprintf("%.1f(%d/%d)", goMetrics.TimeMS, goMetrics.OptimalCount, nTrials)

		results[mc.ID] = bench.MADBenchResult{
			ID:                 mc.ID,
			Tier:               mc.Tier,
			Family:             mc.Family,
			Groups:             mc.Groups,
			Items:              mc.Items,
			Periods:            mc.Periods,
			HasBaselines:       mc.HasBaselines,
			HasDesignated:      mc.HasDesignated,
			Step:               mc.Step,
			Lambda:             mc.Lambda,
			RoundWeight:        mc.RoundWeight,
			Trials:             nTrials,
			DFSStates:          dfsStates,
			DPStates:           dpStates,
			DPWork:             dpWork,
			BruteForceRan:      runBF,
			CBCAvailable:       cbcAvail,
			SIMDEnabled:        simdActive,
			SIMDTier:           simd.ActiveSIMDTierWith(noSIMD),
			CPUModel:           prov.cpuModel,
			GOAMD64:            prov.goAMD64,
			Objective:          trialObjs[0],
			TrialObjectives:    trialObjs,
			ParityOK:           parityOK,
			AutoStrategy:       autoStrategy,
			AutoTimeMS:         medianDurationMS(autoTimes),
			AutoMeanMS:         autoMean,
			AutoSDMS:           autoSD,
			AutoTrialsMS:       durationsMS(autoTimes),
			DPTimeMS:           medianDurationMS(dpTimes),
			DPMeanMS:           dpMean,
			DPSDMS:             dpSD,
			DPTrialsMS:         durationsMS(dpTimes),
			BruteForceTimeMS:   bfMed,
			BruteForceMeanMS:   bfMean,
			BruteForceSDMS:     bfSD,
			BruteForceTrialsMS: bfTrialsMS,
			MILPGo:             goMetrics,
			MILPCBC:            cbcMetricsPtr,
			GitCommit:          prov.gitCommit,
			GitDirty:           prov.gitDirty,
			GoVersion:          prov.goVersion,
			SourceFingerprint:  prov.fingerprint,
			CPUAffinity:        prov.cpuAffinity,
			UnameRelease:       prov.kernelRel,
			MeasuredAt:         prov.measuredAt,
		}

		fmt.Printf("%-22s | %2dx%2dx%d | %7.3fms | %7.3fms | %8s | %14s | %15s | %7s\n",
			mc.ID, mc.Groups, mc.Items, mc.Periods,
			medianDurationMS(autoTimes), medianDurationMS(dpTimes), bfStr,
			goStr, cbcStr, "OK")
	}

	writeJSONReport(outPath, results, "MAD combinatorial benchmark ")
}

func main() {
	corpusDir := flag.String("corpus", "testdata/corpus", "Directory for JSON benchmark corpus")
	genCorpus := flag.Bool("generate-corpus", false, "Generate deterministic 3-tier JSON corpus")
	printFP := flag.Bool("print-fingerprint", false, "Print canonical source and corpus SHA-256 fingerprint")
	assignFlag := flag.Bool("assign", false, "Run Linear Sum Assignment (LSAP) benchmark suite")
	madFlag := flag.Bool("mad", false, "Run isolated MAD combinatorial vs. MILP benchmark suite")
	exportMADDir := flag.String("export-mad-dir", "", "Export combinatorial MAD trial models as PuLP JSONL into directory")
	binSizeFlag := flag.Bool("binary-size", false, "Measure production binary sizes and linker DCE invariants")
	pyRefPath := flag.String("python-ref", "bench/results/python_baseline.json", "Path to Python baseline JSON")
	scipyRefPath := flag.String("scipy-ref", "bench/results/scipy_assign_baseline.json", "Path to SciPy LSAP baseline JSON")
	outPath := flag.String("out", "", "Optional output JSON path for benchmark results")
	pairedScalarOut := flag.String("paired-scalar-out", "", "Optional output JSON path for AB/BA interleaved scalar trials")
	reportPath := flag.String("report", "", "Generate markdown benchmark report and sync docs from saved JSON files")
	syncDocs := flag.Bool("sync-docs", false, "Synchronize README.md, docs/benchmarks.md, and final_report.md from JSON telemetry")
	checkDocs := flag.Bool("check-docs", false, "Verify that README.md, docs/benchmarks.md, and final_report.md match JSON telemetry")
	scalarJSONPath := flag.String("scalar-json", "bench/results/goblin_scalar.json", "Scalar JSON path for report")
	simdJSONPath := flag.String("simd-json", "bench/results/goblin_simd.json", "SIMD JSON path for report")
	backendFlag := flag.String("backend", "go", "Solver backend to benchmark ('go' or 'cbc')")
	noSIMD := flag.Bool("no-simd", false, "Disable hardware SIMD kernels via gb.WithoutSIMD()")
	simdTier := flag.String("simd-tier", "auto", "Cap runtime SIMD dispatch tier ('auto', 'avx512', 'avx2', or 'scalar')")
	trials := flag.Int("trials", defaultTrialsCount, "Number of measured trials per problem")
	workers := flag.Int("workers", 1, "Number of concurrent benchmark workers")
	flag.Parse()

	if err := simd.SetMaxTier(strings.ToLower(strings.TrimSpace(*simdTier))); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(exitCodeBadFlag)
	}

	if *printFP {
		fp, err := bench.SourceFingerprint(".")
		if err != nil {
			fmt.Fprintf(os.Stderr, "source fingerprint: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(fp)
		return
	}

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
		madDir := filepath.Join(filepath.Dir(*corpusDir), "mad_corpus")
		madCases, err := bench.WriteMADCorpusManifest(madDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to write mad corpus manifest: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Generated %d MILP benchmark families in %s, %d LSAP families in %s, and %d MAD combinatorial families in %s\n",
			len(cases), *corpusDir, len(assignCases), assignDir, len(madCases), madDir)
		return
	}

	if *assignFlag {
		runAssignBenchSuite(*scipyRefPath, *outPath, *pairedScalarOut, *noSIMD, *trials)
		return
	}

	if *exportMADDir != "" {
		madCases, err := bench.WriteMADCorpusTrialJSONL(*exportMADDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to export MAD trial JSONL: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Exported %d combinatorial MAD families to %s\n", len(madCases), *exportMADDir)
		return
	}

	if *madFlag {
		runMADBenchSuite(*outPath, *trials, *noSIMD)
		return
	}

	if *binSizeFlag {
		resMap, err := measureBinarySizeSuite(context.Background())
		if err != nil {
			fmt.Fprintf(os.Stderr, "binary-size suite failed: %v\n", err)
			os.Exit(1)
		}
		writeJSONReport(*outPath, resMap, "binary size ")
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

	runMILPBenchSuite(milpSuiteConfig{
		corpusDir:       *corpusDir,
		pyRefPath:       *pyRefPath,
		outPath:         *outPath,
		pairedScalarOut: *pairedScalarOut,
		backendName:     *backendFlag,
		noSIMD:          *noSIMD,
		trials:          *trials,
		workers:         *workers,
	})
}

type milpSuiteConfig struct {
	corpusDir       string
	pyRefPath       string
	outPath         string
	pairedScalarOut string
	backendName     string
	noSIMD          bool
	trials          int
	workers         int
}

type milpJobResult struct {
	idx          int
	res          bench.ProblemBenchResult
	canonicalRes gb.Result
	scalarRes    bench.ProblemBenchResult
	scalarCanon  gb.Result
}

type milpTrialSample struct {
	compiled *gb.CompiledProblem
	res      gb.Result
	dParse   time.Duration
	dBuild   time.Duration
	dCompile time.Duration
	dSolve   time.Duration
	dTotal   time.Duration
	dMPS     time.Duration
}

type milpTrialAcc struct {
	tParse, tBuild, tCompile, tMPS, tSolve, tTotal []time.Duration
	objs                                           []float64
	statuses                                       []string
	compiledTrials                                 []*gb.CompiledProblem
	canonCompiled                                  *gb.CompiledProblem
	canonRes                                       gb.Result
	lpTimes                                        []time.Duration
	canonRootLP                                    gb.LPResult
}

func newMILPTrialAcc(n int) milpTrialAcc {
	return milpTrialAcc{
		tParse:         make([]time.Duration, 0, n),
		tBuild:         make([]time.Duration, 0, n),
		tCompile:       make([]time.Duration, 0, n),
		tMPS:           make([]time.Duration, 0, n),
		tSolve:         make([]time.Duration, 0, n),
		tTotal:         make([]time.Duration, 0, n),
		objs:           make([]float64, 0, n),
		statuses:       make([]string, 0, n),
		compiledTrials: make([]*gb.CompiledProblem, 0, n),
	}
}

func (a *milpTrialAcc) record(trialIdx int, s milpTrialSample) {
	if trialIdx == 0 {
		a.canonCompiled = s.compiled
		a.canonRes = s.res
	}
	a.compiledTrials = append(a.compiledTrials, s.compiled)
	a.objs = append(a.objs, s.res.Objective)
	a.statuses = append(a.statuses, s.res.Status.String())
	a.tParse = append(a.tParse, s.dParse)
	a.tBuild = append(a.tBuild, s.dBuild)
	a.tCompile = append(a.tCompile, s.dCompile)
	a.tSolve = append(a.tSolve, s.dSolve)
	a.tTotal = append(a.tTotal, s.dTotal)
	a.tMPS = append(a.tMPS, s.dMPS)
}

func runMILPTrialPass(rawBytes []byte, mpsBuf *bytes.Buffer, opts []gb.Option) milpTrialSample {
	t0 := time.Now()
	model, err := format.FromPuLPJSONBytes(rawBytes)
	if err != nil {
		panic(err)
	}
	t1 := time.Now()
	t2 := t1
	compiled := model.Compile()
	t3 := time.Now()
	res, err := gb.SolveCompiled(context.Background(), compiled, opts...)
	if err != nil {
		panic(err)
	}
	t4 := time.Now()
	mpsBuf.Reset()
	if err := format.WriteMPS(compiled, mpsBuf); err != nil {
		panic(err)
	}
	t5 := time.Now()
	return milpTrialSample{
		compiled: compiled,
		res:      res,
		dParse:   t1.Sub(t0),
		dBuild:   t2.Sub(t1),
		dCompile: t3.Sub(t2),
		dSolve:   t4.Sub(t3),
		dTotal:   t4.Sub(t0),
		dMPS:     t5.Sub(t4),
	}
}

func solveRootLPOne(cp *gb.CompiledProblem, backend gb.Backend, noSIMD bool) (time.Duration, gb.LPResult) {
	var dt time.Duration
	var lpRes gb.LPResult
	if backend == gb.BackendCBC {
		t0 := time.Now()
		lpSolver := gb.NewCBCLPSolver(cp)
		lpRes = lpSolver.Solve()
		dt = time.Since(t0)
		lpSolver.Close()
	} else {
		t0 := time.Now()
		ws := gb.AcquireWorkspace(cp.NumVars(), cp.NumConstraints())
		lpSolver := gb.NewLPSolver(cp, ws)
		lpSolver.DisableSIMD = noSIMD
		lpRes = lpSolver.Solve()
		gb.ReleaseWorkspace(ws)
		dt = time.Since(t0)
	}
	if lpRes.Status != gb.StatusOptimal && lpRes.Status != gb.StatusInfeasible && lpRes.Status != gb.StatusUnbounded {
		panic(fmt.Sprintf("root LP relaxation failed on %s (backend=%v, noSIMD=%v): status=%v", cp.Name(), backend, noSIMD, lpRes.Status))
	}
	return dt, lpRes
}

func measureRootLPRelaxation(
	canonicalCompiled *gb.CompiledProblem,
	compiledTrials []*gb.CompiledProblem,
	warmup, nTrials int,
	backend gb.Backend,
	noSIMD bool,
) ([]time.Duration, gb.LPResult) {
	lpTimes := make([]time.Duration, 0, nTrials)
	var canonicalRootLP gb.LPResult
	for i := range warmup + nTrials {
		cp := canonicalCompiled
		if i >= warmup {
			cp = compiledTrials[i-warmup]
		}
		dt, lpRes := solveRootLPOne(cp, backend, noSIMD)
		if i >= warmup {
			if i == warmup {
				canonicalRootLP = lpRes
			}
			lpTimes = append(lpTimes, dt)
		}
	}
	return lpTimes, canonicalRootLP
}

func benchSingleCorpusCase(
	c bench.CorpusCase,
	cfg milpSuiteConfig,
	backend gb.Backend,
	simdActive bool,
	baseOpts []gb.Option,
	mpsBuf *bytes.Buffer,
	prov runProvenance,
) (bench.ProblemBenchResult, gb.Result) {
	nTrials, warmup := tierTrialCounts(c.Tier, cfg.trials)
	trialPayloads, err := loadTrialPayloads(cfg.corpusDir, c.ID, nTrials)
	if err != nil {
		fmt.Fprintf(os.Stderr, "missing %s: %v\n", c.ID, err)
		os.Exit(1)
	}

	acc := newMILPTrialAcc(nTrials)
	for i := range warmup + nTrials {
		rawBytes := trialPayloads[0]
		if i >= warmup {
			rawBytes = trialPayloads[i-warmup]
		}
		sample := runMILPTrialPass(rawBytes, mpsBuf, baseOpts)
		if i >= warmup {
			acc.record(i-warmup, sample)
		}
	}

	acc.lpTimes, acc.canonRootLP = measureRootLPRelaxation(acc.canonCompiled, acc.compiledTrials, warmup, nTrials, backend, cfg.noSIMD)
	return buildMILPBenchResult(c, backend, simdActive, cfg.noSIMD, &acc, prov), acc.canonRes
}

func buildMILPBenchResult(
	c bench.CorpusCase,
	backend gb.Backend,
	simdActive, noSIMD bool,
	acc *milpTrialAcc,
	prov runProvenance,
) bench.ProblemBenchResult {
	cpTrials := 3
	if c.Tier == "tier3_large" {
		cpTrials = 1
	}
	cpTimeout := cpTimeoutMS * time.Millisecond

	cp4 := runCheckpoint(acc.compiledTrials, cpTrials,
		gb.WithBackend(backend),
		gb.WithSIMD(!noSIMD),
		gb.WithCombinatorialSearch(false),
		gb.WithPresolve(false),
		gb.WithCuts(false),
		gb.WithHeuristics(false),
		gb.WithMaxNodes(cpMaxNodes),
		gb.WithRelGapTol(cpGapTol),
		gb.WithAbsGapTol(cpGapTol),
		gb.WithTimeLimit(cpTimeout),
	)

	cp5 := runCheckpoint(acc.compiledTrials, cpTrials,
		gb.WithBackend(backend),
		gb.WithSIMD(!noSIMD),
		gb.WithCombinatorialSearch(false),
		gb.WithPresolve(true),
		gb.WithCuts(false),
		gb.WithHeuristics(false),
		gb.WithMaxNodes(cpMaxNodes),
		gb.WithRelGapTol(cpGapTol),
		gb.WithAbsGapTol(cpGapTol),
		gb.WithTimeLimit(cpTimeout),
	)

	cp6 := runCheckpoint(acc.compiledTrials, cpTrials,
		gb.WithBackend(backend),
		gb.WithSIMD(!noSIMD),
		gb.WithCombinatorialSearch(false),
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
		Status:     acc.canonRes.Status.String(),
		Objective:  acc.canonRes.Objective,
		BestBound:  acc.canonRes.BestBound,
		Nodes:      acc.canonRes.Nodes,
		Iterations: acc.canonRes.Iterations,
		CutsAdded:  acc.canonRes.CutsAdded,
		TimeMS:     medianDurationMS(acc.tSolve),
	}

	simdTierStr := ""
	if backend == gb.BackendGo {
		simdTierStr = simd.ActiveSIMDTierWith(noSIMD)
	}
	goMeanMS, goSDMS := meanSDDurationMS(acc.tTotal)
	return bench.ProblemBenchResult{
		ID:                c.ID,
		Tier:              c.Tier,
		NumVars:           acc.canonCompiled.NumVars(),
		NumConstraints:    acc.canonCompiled.NumConstraints(),
		IsMIP:             acc.canonCompiled.IsMIP(),
		Backend:           backend.String(),
		SIMDEnabled:       simdActive,
		SIMDTier:          simdTierStr,
		CPUModel:          prov.cpuModel,
		GOAMD64:           prov.goAMD64,
		TrialObjectives:   acc.objs,
		TrialStatuses:     acc.statuses,
		TParseMS:          medianDurationMS(acc.tParse),
		TBuildMS:          medianDurationMS(acc.tBuild),
		TCompileMS:        medianDurationMS(acc.tCompile),
		TMPSWriteMS:       medianDurationMS(acc.tMPS),
		TSolveMS:          medianDurationMS(acc.tSolve),
		TTotalMS:          medianDurationMS(acc.tTotal),
		TTotalMeanMS:      goMeanMS,
		TTotalSDMS:        goSDMS,
		TSolveTrialsMS:    durationsMS(acc.tSolve),
		TTotalTrialsMS:    durationsMS(acc.tTotal),
		GitCommit:         prov.gitCommit,
		GitDirty:          prov.gitDirty,
		GoVersion:         prov.goVersion,
		SourceFingerprint: prov.fingerprint,
		CPUAffinity:       prov.cpuAffinity,
		UnameRelease:      prov.kernelRel,
		MeasuredAt:        prov.measuredAt,
		RootLPTimeMS:      medianDurationMS(acc.lpTimes),
		RootLPIters:       acc.canonRootLP.Iterations,
		RootLPObj:         acc.canonRootLP.Objective,
		RootLPStatus:      acc.canonRootLP.Status.String(),
		CP4Baseline:       cp4,
		CP5Presolve:       cp5,
		CP6Cuts:           cp6,
		CP7Full:           cp7,
	}
}

func benchPairedCorpusCase(
	c bench.CorpusCase,
	cfg milpSuiteConfig,
	mpsBuf *bytes.Buffer,
	prov runProvenance,
) (simdBench, scalarBench bench.ProblemBenchResult, simdCanon, scalarCanon gb.Result) {
	nTrials, warmup := tierTrialCounts(c.Tier, cfg.trials)
	trialPayloads, err := loadTrialPayloads(cfg.corpusDir, c.ID, nTrials)
	if err != nil {
		fmt.Fprintf(os.Stderr, "missing %s: %v\n", c.ID, err)
		os.Exit(1)
	}

	scalarOpts := []gb.Option{gb.WithPureGo(), gb.WithoutSIMD(), gb.WithCombinatorialSearch(false)}
	simdOpts := []gb.Option{gb.WithPureGo(), gb.WithSIMD(true), gb.WithCombinatorialSearch(false)}
	scAcc := newMILPTrialAcc(nTrials)
	smAcc := newMILPTrialAcc(nTrials)

	for i := range warmup + nTrials {
		rawBytes := trialPayloads[0]
		if i >= warmup {
			rawBytes = trialPayloads[i-warmup]
		}
		var sSc, sSm milpTrialSample
		if i%2 == 0 {
			sSc = runMILPTrialPass(rawBytes, mpsBuf, scalarOpts)
			sSm = runMILPTrialPass(rawBytes, mpsBuf, simdOpts)
		} else {
			sSm = runMILPTrialPass(rawBytes, mpsBuf, simdOpts)
			sSc = runMILPTrialPass(rawBytes, mpsBuf, scalarOpts)
		}
		if i >= warmup {
			scAcc.record(i-warmup, sSc)
			smAcc.record(i-warmup, sSm)
		}
	}

	scAcc.lpTimes = make([]time.Duration, 0, nTrials)
	smAcc.lpTimes = make([]time.Duration, 0, nTrials)
	for i := range warmup + nTrials {
		cpSc, cpSm := scAcc.canonCompiled, smAcc.canonCompiled
		if i >= warmup {
			cpSc = scAcc.compiledTrials[i-warmup]
			cpSm = smAcc.compiledTrials[i-warmup]
		}
		var dSc, dSm time.Duration
		var rSc, rSm gb.LPResult
		if i%2 == 0 {
			dSc, rSc = solveRootLPOne(cpSc, gb.BackendGo, true)
			dSm, rSm = solveRootLPOne(cpSm, gb.BackendGo, false)
		} else {
			dSm, rSm = solveRootLPOne(cpSm, gb.BackendGo, false)
			dSc, rSc = solveRootLPOne(cpSc, gb.BackendGo, true)
		}
		if i >= warmup {
			if i == warmup {
				scAcc.canonRootLP = rSc
				smAcc.canonRootLP = rSm
			}
			scAcc.lpTimes = append(scAcc.lpTimes, dSc)
			smAcc.lpTimes = append(smAcc.lpTimes, dSm)
		}
	}

	scalarBench = buildMILPBenchResult(c, gb.BackendGo, false, true, &scAcc, prov)
	scalarBench.PairedABBA = true
	simdBench = buildMILPBenchResult(c, gb.BackendGo, true, false, &smAcc, prov)
	return simdBench, scalarBench, smAcc.canonRes, scAcc.canonRes
}

func runMILPBenchSuite(cfg milpSuiteConfig) {
	backend, err := parseBackend(cfg.backendName)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(exitCodeBadFlag)
	}
	if backend == gb.BackendCBC && !gb.CBCAvailable() {
		fmt.Fprintln(os.Stderr, "CBC backend requested (-backend cbc), but binary was built without -tags cbc (or with CGO_ENABLED=0)")
		os.Exit(1)
	}
	paired := cfg.pairedScalarOut != ""
	if paired && (backend != gb.BackendGo || cfg.noSIMD || !simd.UsingHardwareSIMDWith(false)) {
		fmt.Fprintln(os.Stderr, "-paired-scalar-out requires -backend go on a GOEXPERIMENT=simd build (AVX2+FMA) without -no-simd")
		os.Exit(exitCodeBadFlag)
	}

	if cfg.trials <= 0 {
		fmt.Fprintf(os.Stderr, "-trials must be positive (got %d)\n", cfg.trials)
		os.Exit(1)
	}
	var baseOpts []gb.Option
	if backend == gb.BackendCBC {
		baseOpts = append(baseOpts, gb.WithCBC())
	} else {
		baseOpts = append(baseOpts, gb.WithPureGo(), gb.WithCombinatorialSearch(false))
	}
	if cfg.noSIMD {
		baseOpts = append(baseOpts, gb.WithoutSIMD())
	}

	var pyRef map[string]bench.PythonBaselineEntry
	if rawRef, err := os.ReadFile(cfg.pyRefPath); err == nil {
		if err := json.Unmarshal(rawRef, &pyRef); err != nil {
			fmt.Fprintf(os.Stderr, "parse %s: %v\n", cfg.pyRefPath, err)
			os.Exit(1)
		}
	}

	cases := bench.BuildCorpus()
	results := make(map[string]bench.ProblemBenchResult, len(cases))
	var scalarResults map[string]bench.ProblemBenchResult
	if paired {
		scalarResults = make(map[string]bench.ProblemBenchResult, len(cases))
	}
	prov, err := collectRunProvenance(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if prov.gitDirty && (cfg.outPath != "" || cfg.pairedScalarOut != "") {
		fmt.Fprintf(os.Stderr, "warning: working tree is dirty; telemetry will be marked git_dirty=true\n")
	}

	simdActive := backend == gb.BackendGo && simd.UsingHardwareSIMDWith(cfg.noSIMD)
	nWorkers := max(1, cfg.workers)
	fmt.Printf("=== GOBLIN WARM-STATE MULTI-TRIAL BENCHMARK SUITE (Backend=%s, SIMD=%v, Tier=%s/%s, GOAMD64=%s, Paired=%v, Workers=%d) ===\n",
		backend, simdActive, simd.ActiveSIMDTierWith(cfg.noSIMD), simd.HardwareSIMDTier(), prov.goAMD64, paired, nWorkers)
	fmt.Printf("CPU: %s\n\n", prov.cpuModel)
	fmt.Printf("%-18s | %5s %4s | %8s %8s | %8s %8s | %9s %9s | %7s | %7s\n",
		"Problem ID", "Vars", "Rows",
		"PyPrep", "GoPrep",
		"PySolve", "GoSolve",
		"PyTotal", "GoTotal",
		"Speedup", "Parity")
	fmt.Println(strings.Repeat("-", tableRuleWidth))

	jobCh := make(chan int, len(cases))
	for idx := range cases {
		jobCh <- idx
	}
	close(jobCh)

	jobResults := make([]milpJobResult, len(cases))
	var wg sync.WaitGroup
	for range min(nWorkers, len(cases)) {
		wg.Go(func() {
			var mpsBuf bytes.Buffer
			warmWorker(cfg.corpusDir, &mpsBuf, baseOpts...)
			if paired {
				warmWorker(cfg.corpusDir, &mpsBuf, gb.WithPureGo(), gb.WithoutSIMD())
			}

			for idx := range jobCh {
				if paired {
					smRes, scRes, smCanon, scCanon := benchPairedCorpusCase(cases[idx], cfg, &mpsBuf, prov)
					jobResults[idx] = milpJobResult{
						idx:          idx,
						res:          smRes,
						canonicalRes: smCanon,
						scalarRes:    scRes,
						scalarCanon:  scCanon,
					}
					continue
				}
				res, canonicalRes := benchSingleCorpusCase(cases[idx], cfg, backend, simdActive, baseOpts, &mpsBuf, prov)
				jobResults[idx] = milpJobResult{
					idx:          idx,
					res:          res,
					canonicalRes: canonicalRes,
				}
			}
		})
	}
	wg.Wait()

	for idx, c := range cases {
		jr := jobResults[idx]
		if paired {
			printMILPBenchRow(c.ID, jr.scalarRes, jr.scalarCanon, pyRef)
			scalarResults[c.ID] = jr.scalarRes
		} else {
			printMILPBenchRow(c.ID, jr.res, jr.canonicalRes, pyRef)
		}
		results[c.ID] = jr.res
	}

	writeJSONReport(cfg.pairedScalarOut, scalarResults, "scalar benchmark ")
	writeJSONReport(cfg.outPath, results, "benchmark ")
}

func printMILPBenchRow(
	id string,
	r bench.ProblemBenchResult,
	canonicalRes gb.Result,
	pyRef map[string]bench.PythonBaselineEntry,
) {
	goPrepMS := r.TParseMS + r.TBuildMS + r.TCompileMS
	pyPrepMS, pySolveMS, pyTotalMS := 0.0, 0.0, 0.0
	speedupStr := "N/A"
	parityStr := "OK"

	if ref, ok := pyRef[id]; ok {
		pyPrepMS = ref.TParseMedianMS + ref.TBuildMedianMS
		pySolveMS = ref.TSolveMedianMS
		pyTotalMS = ref.TTotalMedianMS
		if r.TTotalMS > 0 {
			speedupStr = fmt.Sprintf("%.1fx", pyTotalMS/r.TTotalMS)
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
		id, r.NumVars, r.NumConstraints,
		pyPrepMS, goPrepMS,
		pySolveMS, r.TSolveMS,
		pyTotalMS, r.TTotalMS,
		speedupStr, parityStr)
}

var (
	reNMLine  = regexp.MustCompile(`^\s*(?:[0-9a-fA-F]+\s+)?([0-9]+)\s+([A-Za-z?])\s+(.+)$`)
	reVIPRSym = regexp.MustCompile(`(VIPR|vipr)`)
	reCertSym = regexp.MustCompile(`(solveCertified|finalizeCertifiedSolve|verifyCertified|VerifyVIPRTrace|verifyMiniVIPR|certPresolve|certEvalProofStep|certifyLinearSum|certifySolveInto|certifyAssign|dyadic2304|dyadicFromFloat|bellmanFordResidual|extractHallWitness|defaultAssignCertHooks|setCertifiedTrue|setRootCertifiedTrue|certifyMAD|defaultMADCertHooks|setMADCertifiedTrue|setMADCertifiedFalse|EvaluateAssignmentDyadic|evalGroupExactDyadic)`)
)

type binSizeProfileSpec struct {
	id      string
	label   string
	desc    string
	wantOut string
	src     string
}

func binSizeProfiles() []binSizeProfileSpec {
	return []binSizeProfileSpec{
		{
			id:      "assign_default",
			label:   "`assign` default (`LinearSum` + `SolveInto`)",
			desc:    "Uncertified LSAP solver (`assign.LinearSum` and `assign.SolveInto`)",
			wantOut: "2 2 <nil>",
			src: `package main

import (
	"context"
	"fmt"
	as "github.com/paulourio/goblin/assign"
)

func main() {
	ctx := context.Background()
	res, err := as.LinearSum(ctx, [][]float64{{1, 2}, {3, 1}}, as.Minimize)
	if err != nil {
		panic(err)
	}
	ws := as.AcquireWorkspace(2, 2)
	defer as.ReleaseWorkspace(ws)
	rInd := make([]int, 2)
	cInd := make([]int, 2)
	obj, errInto := as.SolveInto(ctx, 2, 2, []float64{1, 2, 3, 1}, as.Minimize, rInd, cInd, ws)
	fmt.Println(res.Objective, obj, errInto)
}
`,
		},
		{
			id:      "assign_certified",
			label:   "`assign` certified (`+ assign.WithCertified(true)`)",
			desc:    "LSAP solver with fail-closed 2304-bit dyadic verification (`assign.WithCertified(true)`)",
			wantOut: "2 2 <nil>",
			src: `package main

import (
	"context"
	"fmt"
	as "github.com/paulourio/goblin/assign"
)

func main() {
	ctx := context.Background()
	res, err := as.LinearSum(ctx, [][]float64{{1, 2}, {3, 1}}, as.Minimize, as.WithCertified(true))
	if err != nil {
		panic(err)
	}
	ws := as.AcquireWorkspace(2, 2)
	defer as.ReleaseWorkspace(ws)
	rInd := make([]int, 2)
	cInd := make([]int, 2)
	obj, errInto := as.SolveInto(ctx, 2, 2, []float64{1, 2, 3, 1}, as.Minimize, rInd, cInd, ws, as.WithCertified(true))
	fmt.Println(res.Objective, obj, errInto)
}
`,
		},
		{
			id:      "goblin_default",
			label:   "`goblin` default (`Model.Solve`, `DefaultOptions`)",
			desc:    "Uncertified LP/MILP Branch-and-Cut solver (`Model.Solve` with `WithOptions(DefaultOptions())`)",
			wantOut: "2 <nil>",
			src: `package main

import (
	"context"
	"fmt"
	gb "github.com/paulourio/goblin"
)

func main() {
	m := gb.NewModel("t", gb.Minimize)
	x := m.AddBinaryVar("x")
	y := m.AddBinaryVar("y")
	m.AddConstraint("c1", gb.Ge(gb.NewExpr(gb.T(x, 1), gb.T(y, 1)), 1))
	m.SetObjective(gb.NewExpr(gb.T(x, 2), gb.T(y, 3)))
	res, err := m.Solve(context.Background(), gb.WithOptions(gb.DefaultOptions()))
	fmt.Println(res.Objective, err)
}
`,
		},
		{
			id:      "goblin_certified_vipr",
			label:   "`goblin` certified + VIPR (`+ WithCertified + WithVIPRTrace`)",
			desc:    "LP/MILP solver with fail-closed `*big.Rat` certification and presolve/orbital `VIPRTrace` (`WithCertified(true)`, `WithVIPRTrace(NewVIPRTrace())`)",
			wantOut: "2 <nil> true",
			src: `package main

import (
	"context"
	"fmt"
	gb "github.com/paulourio/goblin"
)

func main() {
	m := gb.NewModel("t", gb.Minimize)
	x := m.AddBinaryVar("x")
	y := m.AddBinaryVar("y")
	m.AddConstraint("c1", gb.Ge(gb.NewExpr(gb.T(x, 1), gb.T(y, 1)), 1))
	m.SetObjective(gb.NewExpr(gb.T(x, 2), gb.T(y, 3)))
	tr := gb.NewVIPRTrace()
	res, err := m.Solve(context.Background(), gb.WithCertified(true), gb.WithVIPRTrace(tr))
	fmt.Println(res.Objective, err, tr.HasCertificate())
}
`,
		},
	}
}

func cleanGoBuildEnv(enableSIMD bool) []string {
	base := os.Environ()
	out := make([]string, 0, len(base)+2)
	for _, kv := range base {
		if strings.HasPrefix(kv, "GOEXPERIMENT=") || strings.HasPrefix(kv, "CGO_ENABLED=") {
			continue
		}
		out = append(out, kv)
	}
	out = append(out, "CGO_ENABLED=0")
	if enableSIMD {
		out = append(out, "GOEXPERIMENT=simd")
	}
	return out
}

func buildProbeBinary(ctx context.Context, dir, pkg, outBin string, strip, enableSIMD bool) (int64, error) {
	args := []string{"build"}
	if strip {
		args = append(args, "-ldflags=-s -w")
	}
	args = append(args, "-o", outBin, pkg)
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir
	cmd.Env = cleanGoBuildEnv(enableSIMD)
	if out, err := cmd.CombinedOutput(); err != nil {
		return 0, fmt.Errorf("go %v (simd=%v): %w\n%s", args, enableSIMD, err, out)
	}
	info, err := os.Stat(outBin)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func readBinaryBuildMeta(binPath string) (goVer, goOS, goArch, goAMD64 string, err error) {
	bi, err := buildinfo.ReadFile(binPath)
	if err != nil {
		return "", "", "", "", err
	}
	goOS = runtime.GOOS
	goArch = runtime.GOARCH
	for _, s := range bi.Settings {
		switch s.Key {
		case "GOOS":
			goOS = s.Value
		case "GOARCH":
			goArch = s.Value
		}
	}
	return bi.GoVersion, goOS, goArch, extractGOAMD64(bi.Settings, goArch), nil
}

func parseNMSymbolStats(nmOut string, entry *bench.BinarySizeEntry) {
	for line := range strings.SplitSeq(nmOut, "\n") {
		m := reNMLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		sz, errSz := strconv.ParseInt(m[1], 10, 64)
		if errSz != nil {
			continue
		}
		typ := m[2]
		sym := m[3]
		isGoblinPkg := strings.HasPrefix(sym, "github.com/paulourio/goblin")
		if (typ == "T" || typ == "t") && isGoblinPkg {
			entry.GoblinTextBytes += sz
		}
		if strings.Contains(sym, "math/big") {
			entry.MathBigSyms++
			entry.MathBigBytes += sz
		}
		if strings.Contains(sym, "encoding/json") {
			entry.EncodingJSONSyms++
			entry.EncodingJSONBytes += sz
		}
		if isGoblinPkg && reVIPRSym.MatchString(sym) {
			entry.VIPRSyms++
			entry.VIPRBytes += sz
		}
		if isGoblinPkg && reCertSym.MatchString(sym) {
			entry.CertSyms++
			entry.CertBytes += sz
		}
	}
}

func measureBinarySizeSuite(ctx context.Context) (map[string]bench.BinarySizeEntry, error) {
	absRoot, err := filepath.Abs(".")
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(absRoot, "go.mod")); err != nil {
		return nil, fmt.Errorf("missing go.mod in %s: %w", absRoot, err)
	}
	prov, err := collectRunProvenance(ctx)
	if err != nil {
		return nil, err
	}
	if prov.gitDirty {
		fmt.Fprintln(os.Stderr, "warning: working tree is dirty; binary size telemetry will be marked git_dirty=true")
	}

	tmpDir, err := os.MkdirTemp("", "goblin-binsize-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	goMod := fmt.Sprintf("module goblinbinsizeprobe\n\ngo 1.27\n\nrequire github.com/paulourio/goblin v0.0.0\n\nreplace github.com/paulourio/goblin => %s\n", absRoot)
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0o600); err != nil {
		return nil, err
	}

	specs := binSizeProfiles()
	results := make(map[string]bench.BinarySizeEntry, len(specs))

	fmt.Printf("=== GOBLIN PRODUCTION BINARY SIZE & LINKER DCE SUITE (CGO_ENABLED=0) ===\n")
	fmt.Printf("%-24s | %19s | %19s | %10s | %5s %5s %5s %5s\n",
		"Profile ID", "Default Raw/Strip", "SIMD Raw/Strip", "GoblinText", "Big", "JSON", "VIPR", "Cert")
	fmt.Println(strings.Repeat("-", tableRuleWidth))

	for _, sp := range specs {
		pkgDir := filepath.Join(tmpDir, sp.id)
		if err := os.MkdirAll(pkgDir, 0o700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(pkgDir, "main.go"), []byte(sp.src), 0o600); err != nil {
			return nil, err
		}

		pkgSpec := "./" + sp.id
		rawDefBin := filepath.Join(tmpDir, sp.id+"_def_raw")
		stripDefBin := filepath.Join(tmpDir, sp.id+"_def_strip")
		rawSIMDBin := filepath.Join(tmpDir, sp.id+"_simd_raw")
		stripSIMDBin := filepath.Join(tmpDir, sp.id+"_simd_strip")

		rawBytes, err := buildProbeBinary(ctx, tmpDir, pkgSpec, rawDefBin, false, false)
		if err != nil {
			return nil, err
		}
		stripBytes, err := buildProbeBinary(ctx, tmpDir, pkgSpec, stripDefBin, true, false)
		if err != nil {
			return nil, err
		}
		simdRawBytes, err := buildProbeBinary(ctx, tmpDir, pkgSpec, rawSIMDBin, false, true)
		if err != nil {
			return nil, err
		}
		simdStripBytes, err := buildProbeBinary(ctx, tmpDir, pkgSpec, stripSIMDBin, true, true)
		if err != nil {
			return nil, err
		}

		for _, binPath := range []string{rawDefBin, rawSIMDBin} {
			runOut, errRun := exec.CommandContext(ctx, binPath).CombinedOutput()
			if errRun != nil {
				return nil, fmt.Errorf("execute %s: %w\n%s", filepath.Base(binPath), errRun, runOut)
			}
			if got := strings.TrimSpace(string(runOut)); got != sp.wantOut {
				return nil, fmt.Errorf("%s output = %q, want %q", filepath.Base(binPath), got, sp.wantOut)
			}
		}

		defVer, goOS, goArch, goAMD64, err := readBinaryBuildMeta(rawDefBin)
		if err != nil {
			return nil, fmt.Errorf("buildinfo %s: %w", sp.id, err)
		}
		simdVer, _, _, _, err := readBinaryBuildMeta(rawSIMDBin)
		if err != nil {
			return nil, fmt.Errorf("simd buildinfo %s: %w", sp.id, err)
		}

		nmRaw, err := exec.CommandContext(ctx, "go", "tool", "nm", "-size", rawDefBin).CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("go tool nm %s: %w\n%s", sp.id, err, nmRaw)
		}

		entry := bench.BinarySizeEntry{
			ID:                sp.id,
			Label:             sp.label,
			Description:       sp.desc,
			RawBytes:          rawBytes,
			StrippedBytes:     stripBytes,
			SIMDRawBytes:      simdRawBytes,
			SIMDStrippedBytes: simdStripBytes,
			GitCommit:         prov.gitCommit,
			GitDirty:          prov.gitDirty,
			DefaultGoVersion:  defVer,
			SIMDGoVersion:     simdVer,
			GOOS:              goOS,
			GOARCH:            goArch,
			GOAMD64:           goAMD64,
			SourceFingerprint: prov.fingerprint,
			UnameRelease:      prov.kernelRel,
			MeasuredAt:        prov.measuredAt,
		}
		parseNMSymbolStats(string(nmRaw), &entry)

		results[sp.id] = entry
		fmt.Printf("%-24s | %8.1f / %7.1fK | %8.1f / %7.1fK | %8.1f K | %5d %5d %5d %5d\n",
			sp.id,
			float64(entry.RawBytes)/1024.0, float64(entry.StrippedBytes)/1024.0,
			float64(entry.SIMDRawBytes)/1024.0, float64(entry.SIMDStrippedBytes)/1024.0,
			float64(entry.GoblinTextBytes)/1024.0,
			entry.MathBigSyms, entry.EncodingJSONSyms, entry.VIPRSyms, entry.CertSyms)
	}

	if err := bench.ValidateBinarySizeTelemetry(results); err != nil {
		return nil, err
	}
	return results, nil
}
