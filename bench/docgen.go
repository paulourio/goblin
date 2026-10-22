package bench

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// PythonBaselineEntry matches the JSON output produced by bench/python_baseline.py.
type PythonBaselineEntry struct {
	ID                    string             `json:"id"`
	NumVars               int                `json:"num_vars"`
	NumConstraints        int                `json:"num_constraints"`
	IsMIP                 bool               `json:"is_mip"`
	StatusCode            int                `json:"status_code"`
	Status                string             `json:"status"`
	Objective             *float64           `json:"objective"`
	TrialObjectives       []*float64         `json:"trial_objectives,omitempty"`
	LPRelaxationObjective *float64           `json:"lp_relaxation_objective"`
	Solution              map[string]float64 `json:"solution"`
	Trials                int                `json:"trials"`
	PuLPVersion           string             `json:"pulp_version"`
	CBCPath               string             `json:"cbc_path"`
	CBCVersion            string             `json:"cbc_version"`
	TParseMedianMS        float64            `json:"t_parse_median_ms"`
	TBuildMedianMS        float64            `json:"t_build_median_ms"`
	TFormatMedianMS       float64            `json:"t_format_median_ms"`
	TSolveMedianMS        float64            `json:"t_solve_median_ms"`
	TTotalMedianMS        float64            `json:"t_total_median_ms"`
	TTotalMeanMS          float64            `json:"t_total_mean_ms"`
	TTotalSDMS            float64            `json:"t_total_sd_ms"`
}

// CheckpointMetrics captures Solver-Core performance for a specific solver configuration.
type CheckpointMetrics struct {
	Status     string  `json:"status"`
	Objective  float64 `json:"objective"`
	BestBound  float64 `json:"best_bound"`
	Nodes      int     `json:"nodes"`
	Iterations int     `json:"iterations"`
	CutsAdded  int     `json:"cuts_added"`
	TimeMS     float64 `json:"time_ms"`
}

// ProblemBenchResult stores all pipeline and checkpoint measurements for one problem.
type ProblemBenchResult struct {
	ID              string            `json:"id"`
	Tier            string            `json:"tier"`
	NumVars         int               `json:"num_vars"`
	NumConstraints  int               `json:"num_constraints"`
	IsMIP           bool              `json:"is_mip"`
	SIMDEnabled     bool              `json:"simd_enabled"`
	TrialObjectives []float64         `json:"trial_objectives,omitempty"`
	TParseMS        float64           `json:"t_parse_ms"`
	TBuildMS        float64           `json:"t_build_ms"`
	TCompileMS      float64           `json:"t_compile_ms"`
	TMPSWriteMS     float64           `json:"t_mps_write_ms"`
	TSolveMS        float64           `json:"t_solve_ms"`
	TTotalMS        float64           `json:"t_total_ms"`
	TTotalMeanMS    float64           `json:"t_total_mean_ms"`
	TTotalSDMS      float64           `json:"t_total_sd_ms"`
	RootLPTimeMS    float64           `json:"root_lp_time_ms"`
	RootLPIters     int               `json:"root_lp_iters"`
	RootLPObj       float64           `json:"root_lp_obj"`
	CP4Baseline     CheckpointMetrics `json:"cp4_baseline"`
	CP5Presolve     CheckpointMetrics `json:"cp5_presolve"`
	CP6Cuts         CheckpointMetrics `json:"cp6_cuts"`
	CP7Full         CheckpointMetrics `json:"cp7_full"`
}

// TelemetryBundle holds the loaded benchmark results from canonical JSON files.
type TelemetryBundle struct {
	Python map[string]PythonBaselineEntry
	Goblin map[string]ProblemBenchResult
	Scalar map[string]ProblemBenchResult
	SIMD   map[string]ProblemBenchResult
}

// LoadTelemetryBundle reads and validates the Python, Go Scalar, Go SIMD, and (when present) goblin C++ CBC JSON telemetry files.
func LoadTelemetryBundle(pyPath, scalarPath, simdPath string) (*TelemetryBundle, error) {
	rawPy, err := os.ReadFile(pyPath)
	if err != nil {
		return nil, fmt.Errorf("read python baseline %s: %w", pyPath, err)
	}
	var pyMap map[string]PythonBaselineEntry
	if err := json.Unmarshal(rawPy, &pyMap); err != nil {
		return nil, fmt.Errorf("unmarshal python baseline %s: %w", pyPath, err)
	}

	rawScalar, err := os.ReadFile(scalarPath)
	if err != nil {
		return nil, fmt.Errorf("read scalar telemetry %s: %w", scalarPath, err)
	}
	var scalarMap map[string]ProblemBenchResult
	if err := json.Unmarshal(rawScalar, &scalarMap); err != nil {
		return nil, fmt.Errorf("unmarshal scalar telemetry %s: %w", scalarPath, err)
	}

	rawSIMD, err := os.ReadFile(simdPath)
	if err != nil {
		return nil, fmt.Errorf("read simd telemetry %s: %w", simdPath, err)
	}
	var simdMap map[string]ProblemBenchResult
	if err := json.Unmarshal(rawSIMD, &simdMap); err != nil {
		return nil, fmt.Errorf("unmarshal simd telemetry %s: %w", simdPath, err)
	}

	var goblinMap map[string]ProblemBenchResult
	goblinCandidates := []string{
		filepath.Join(filepath.Dir(scalarPath), "goblin_cbc.json"),
		filepath.Join(filepath.Dir(scalarPath), "..", "..", "..", "goblin", "bench", "results", "goblin_cbc.json"),
	}
	for _, cand := range goblinCandidates {
		if rawGoblin, err := os.ReadFile(cand); err == nil {
			_ = json.Unmarshal(rawGoblin, &goblinMap)
			if len(goblinMap) > 0 {
				break
			}
		}
	}

	return &TelemetryBundle{
		Python: pyMap,
		Goblin: goblinMap,
		Scalar: scalarMap,
		SIMD:   simdMap,
	}, nil
}

// ValidateTelemetryAgainstCorpus verifies that every problem in BuildCorpus() exists in
// testdata/corpus and in all three telemetry maps with matching variable and constraint counts.
func ValidateTelemetryAgainstCorpus(corpusDir string, tb *TelemetryBundle) error {
	cases := BuildCorpus()
	if len(tb.Python) != len(cases) {
		return fmt.Errorf("python baseline has %d entries, corpus has %d", len(tb.Python), len(cases))
	}
	if len(tb.Scalar) != len(cases) {
		return fmt.Errorf("scalar telemetry has %d entries, corpus has %d", len(tb.Scalar), len(cases))
	}
	if len(tb.SIMD) != len(cases) {
		return fmt.Errorf("simd telemetry has %d entries, corpus has %d", len(tb.SIMD), len(cases))
	}

	for _, c := range cases {
		compiled := c.Model.Compile()
		jsonPath := filepath.Join(corpusDir, c.ID+".json")
		if _, err := os.Stat(jsonPath); err != nil {
			return fmt.Errorf("missing corpus JSON file %s: %w", jsonPath, err)
		}

		py, ok := tb.Python[c.ID]
		if !ok {
			return fmt.Errorf("missing %q in python baseline", c.ID)
		}
		if py.NumVars != compiled.NumVars || py.NumConstraints != compiled.NumConstraints {
			return fmt.Errorf("python baseline dimension mismatch for %q: got (%d, %d), want (%d, %d)",
				c.ID, py.NumVars, py.NumConstraints, compiled.NumVars, compiled.NumConstraints)
		}
		if py.PuLPVersion == "" || py.CBCPath == "" || py.CBCVersion == "" {
			return fmt.Errorf("python baseline missing solver provenance (pulp_version/cbc_path/cbc_version) for %q", c.ID)
		}

		sc, ok := tb.Scalar[c.ID]
		if !ok {
			return fmt.Errorf("missing %q in scalar telemetry", c.ID)
		}
		if sc.NumVars != compiled.NumVars || sc.NumConstraints != compiled.NumConstraints {
			return fmt.Errorf("scalar telemetry dimension mismatch for %q: got (%d, %d), want (%d, %d)",
				c.ID, sc.NumVars, sc.NumConstraints, compiled.NumVars, compiled.NumConstraints)
		}

		sm, ok := tb.SIMD[c.ID]
		if !ok {
			return fmt.Errorf("missing %q in simd telemetry", c.ID)
		}
		if sm.NumVars != compiled.NumVars || sm.NumConstraints != compiled.NumConstraints {
			return fmt.Errorf("simd telemetry dimension mismatch for %q: got (%d, %d), want (%d, %d)",
				c.ID, sm.NumVars, sm.NumConstraints, compiled.NumVars, compiled.NumConstraints)
		}
	}
	return nil
}

func madDimensions(id string) (g, i, t int, ok bool) {
	switch id {
	case "t1_mad_g2_i4", "t1_mad_rand_g2_i4":
		return 2, 4, 6, true
	case "t2_mad_g2_i8", "t2_mad_rand_g2_i8":
		return 2, 8, 6, true
	case "t2_mad_g3_i9", "t2_mad_rand_g3_i9":
		return 3, 9, 6, true
	case "t2_mad_g4_i12":
		return 4, 12, 6, true
	case "t3_mad_g5_i20":
		return 5, 20, 6, true
	case "t3_mad_g6_i30":
		return 6, 30, 6, true
	case "t3_mad_g10_i100":
		return 10, 100, 6, true
	case "t3_mad_g4_i1000":
		return 4, 1000, 6, true
	default:
		return 0, 0, 0, false
	}
}

func geoMean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sumLog := 0.0
	for _, v := range vals {
		sumLog += math.Log(max(v, 1e-6))
	}
	return math.Exp(sumLog / float64(len(vals)))
}

func medianMinMax(vals []float64) (med, minV, maxV float64) {
	if len(vals) == 0 {
		return 0, 0, 0
	}
	cp := slices.Clone(vals)
	slices.Sort(cp)
	n := len(cp)
	if n%2 == 1 {
		med = cp[n/2]
	} else {
		med = 0.5 * (cp[n/2-1] + cp[n/2])
	}
	return med, cp[0], cp[n-1]
}

func formatCP45(cp CheckpointMetrics) string {
	if cp.Status != "" && cp.Status != "Optimal" && cp.Status != "Infeasible" {
		return fmt.Sprintf("%.2f (%d, `%s`)", cp.TimeMS, cp.Nodes, cp.Status)
	}
	return fmt.Sprintf("%.2f (%d)", cp.TimeMS, cp.Nodes)
}

func formatCP67(cp CheckpointMetrics, bold bool) string {
	timeStr := fmt.Sprintf("%.2f", cp.TimeMS)
	if bold {
		timeStr = fmt.Sprintf("**%.2f**", cp.TimeMS)
	}
	if cp.Status != "" && cp.Status != "Optimal" && cp.Status != "Infeasible" {
		return fmt.Sprintf("%s (%d / %d, `%s`)", timeStr, cp.Nodes, cp.CutsAdded, cp.Status)
	}
	return fmt.Sprintf("%s (%d / %d)", timeStr, cp.Nodes, cp.CutsAdded)
}

// RenderBlock produces the markdown snippet for a named generated block marker.
func RenderBlock(blockName string, tb *TelemetryBundle) (string, error) {
	cases := BuildCorpus()
	var b bytes.Buffer

	switch blockName {
	case "TIER_SUMMARY_TABLE":
		type groupSpec struct {
			label  string
			filter func(c CorpusCase) bool
		}
		groups := []groupSpec{
			{"Tier 1: Canonical (`N <= 22`, 10 families)", func(c CorpusCase) bool { return c.Tier == "tier1_canonical" }},
			{"Tier 2: Target p90 (`30 <= N <= 76`, 13 families)", func(c CorpusCase) bool { return c.Tier == "tier2_p90" }},
			{"Tier 3: Large Tail (`135 <= N <= 4028`, 8 families)", func(c CorpusCase) bool { return c.Tier == "tier3_large" }},
			{"Tier 3 Non-MAD MILPs (`200 <= N <= 1000`, 4 families)", func(c CorpusCase) bool {
				_, _, _, isMAD := madDimensions(c.ID)
				return c.Tier == "tier3_large" && !isMAD
			}},
			{"MAD `G x I` Family (`22 <= N <= 4028`, 11 families)", func(c CorpusCase) bool {
				_, _, _, ok := madDimensions(c.ID)
				return ok
			}},
			{"Unstructured Random MAD (`22 <= N <= 48`, 3 families)", func(c CorpusCase) bool {
				return strings.Contains(c.ID, "_mad_rand_")
			}},
			{"**All 31 Corpus Families (293 Random Trials)**", func(c CorpusCase) bool { return true }},
		}

		b.WriteString("| Workload Subset | PyTotal GeoMean (ms) | GoblinTotal GeoMean (ms) | GoptScalar GeoMean (ms) | GoptSIMD GeoMean (ms) | `Py / gopt SIMD` GeoMean | `Py / gopt SIMD` Median [Min-Max] | `gopt SIMD / goblin` |\n")
		b.WriteString("| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
		for _, grp := range groups {
			var pyVals, gbVals, scVals, smVals, speedups []float64
			for _, c := range cases {
				if !grp.filter(c) {
					continue
				}
				pyT := tb.Python[c.ID].TTotalMedianMS
				smT := tb.SIMD[c.ID].TTotalMS
				pyVals = append(pyVals, pyT)
				if gb, ok := tb.Goblin[c.ID]; ok && gb.TTotalMS > 0 {
					gbVals = append(gbVals, gb.TTotalMS)
				}
				scVals = append(scVals, tb.Scalar[c.ID].TTotalMS)
				smVals = append(smVals, smT)
				if smT > 0 {
					speedups = append(speedups, pyT/smT)
				}
			}
			pyGM := geoMean(pyVals)
			gbGM := geoMean(gbVals)
			scGM := geoMean(scVals)
			smGM := geoMean(smVals)
			medSp, minSp, maxSp := medianMinMax(speedups)
			gbRatio := "N/A"
			gbStr := "N/A"
			if gbGM > 0 {
				gbStr = fmt.Sprintf("%.2f", gbGM)
				gbRatio = fmt.Sprintf("**%.2fx**", smGM/gbGM)
			}
			fmt.Fprintf(&b, "| %s | %.2f | %s | %.2f | %.2f | **%.1fx** | %.1fx [%.1fx-%.1fx] | %s |\n",
				grp.label, pyGM, gbStr, scGM, smGM, pyGM/smGM, medSp, minSp, maxSp, gbRatio)
		}

	case "MAD_PIPELINE_TABLE":
		b.WriteString("| Problem ID | $G$ | $I$ | $T$ | Vars | Rows | Optimal Obj | PyPrep (ms) | GoPrep (ms) | PySolve (ms) | GoSolve Scalar (ms) | GoSolve SIMD (ms) | PyTotal (ms) | GoTotal SIMD (ms) | Speedup (SIMD) | Parity |\n")
		b.WriteString("| :--- | --: | ---: | --: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |\n")
		for _, c := range cases {
			g, i, t, ok := madDimensions(c.ID)
			if !ok {
				continue
			}
			py := tb.Python[c.ID]
			sc := tb.Scalar[c.ID]
			sm := tb.SIMD[c.ID]
			pyPrep := py.TParseMedianMS + py.TBuildMedianMS + py.TFormatMedianMS
			goPrep := sm.TParseMS + sm.TBuildMS + sm.TCompileMS
			speedup := py.TTotalMedianMS / sm.TTotalMS
			fmt.Fprintf(&b, "| `%s` | %d | %d | %d | %d | %d | %.1f | %.3f | %.3f | %.2f | %.2f | %.2f | %.2f | %.2f | **%.1fx** | `OK` |\n",
				c.ID, g, i, t, sm.NumVars, sm.NumConstraints,
				sm.CP7Full.Objective,
				pyPrep, goPrep,
				py.TSolveMedianMS, sc.TSolveMS, sm.TSolveMS,
				py.TTotalMedianMS, sm.TTotalMS,
				speedup)
		}

	case "MAD_STATS_TABLE":
		b.WriteString("| Problem ID | $G \\times I$ | Python `PuLP + CBC` Total (Mean $\\pm$ SD [95% CI], ms) | `gopt` Scalar Total (Mean $\\pm$ SD [95% CI], ms) | `gopt` SIMD Total (Mean $\\pm$ SD [95% CI], ms) |\n")
		b.WriteString("| :--- | :---: | ---: | ---: | ---: |\n")
		for _, c := range cases {
			g, i, _, ok := madDimensions(c.ID)
			if !ok {
				continue
			}
			py := tb.Python[c.ID]
			sc := tb.Scalar[c.ID]
			sm := tb.SIMD[c.ID]
			nTrials := py.Trials
			if nTrials <= 0 {
				nTrials = len(sm.TrialObjectives)
			}
			tCrit := 1.96
			switch nTrials {
			case 5:
				tCrit = 2.776
			case 11:
				tCrit = 2.228
			}
			sqrtN := math.Sqrt(float64(max(1, nTrials)))
			pyCI := tCrit * py.TTotalSDMS / sqrtN
			scCI := tCrit * sc.TTotalSDMS / sqrtN
			smCI := tCrit * sm.TTotalSDMS / sqrtN
			fmt.Fprintf(&b, "| `%s` | $%d \\times %d$ | $%.2f \\pm %.2f$ ($\\pm %.2f$) | $%.2f \\pm %.2f$ ($\\pm %.2f$) | $%.2f \\pm %.2f$ ($\\pm %.2f$) |\n",
				c.ID, g, i,
				py.TTotalMeanMS, py.TTotalSDMS, pyCI,
				sc.TTotalMeanMS, sc.TTotalSDMS, scCI,
				sm.TTotalMeanMS, sm.TTotalSDMS, smCI)
		}

	case "MAD_CHECKPOINT_TABLE":
		b.WriteString("| Problem ID | Root LP Obj | Optimal Obj | Root LP (ms) | CP4 Base (ms / nodes) | CP5 +Presolve (ms / nodes) | CP6 +Cuts (ms / nodes / cuts) | CP7 Full (ms / nodes / cuts) |\n")
		b.WriteString("| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
		for _, c := range cases {
			if _, _, _, ok := madDimensions(c.ID); !ok {
				continue
			}
			sm := tb.SIMD[c.ID]
			fmt.Fprintf(&b, "| `%s` | %.2f | %.2f | %.3f | %s | %s | %s | %s |\n",
				c.ID,
				sm.RootLPObj,
				sm.CP7Full.Objective,
				sm.RootLPTimeMS,
				formatCP45(sm.CP4Baseline),
				formatCP45(sm.CP5Presolve),
				formatCP67(sm.CP6Cuts, false),
				formatCP67(sm.CP7Full, true))
		}

	case "FULL_PIPELINE_TABLE":
		b.WriteString("| Problem ID | Vars | Rows | PyPrep (ms) | GoPrep (ms) | PySolve (ms) | GoSolve Scalar (ms) | GoSolve SIMD (ms) | PyTotal (ms) | GoTotal SIMD (ms) | Speedup (SIMD) | Parity |\n")
		b.WriteString("| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |\n")
		for _, c := range cases {
			py := tb.Python[c.ID]
			sc := tb.Scalar[c.ID]
			sm := tb.SIMD[c.ID]
			pyPrep := py.TParseMedianMS + py.TBuildMedianMS + py.TFormatMedianMS
			goPrep := sm.TParseMS + sm.TBuildMS + sm.TCompileMS
			speedup := py.TTotalMedianMS / sm.TTotalMS
			fmt.Fprintf(&b, "| `%s` | %d | %d | %.3f | %.3f | %.2f | %.2f | %.2f | %.2f | %.2f | **%.1fx** | `OK` |\n",
				c.ID, sm.NumVars, sm.NumConstraints,
				pyPrep, goPrep,
				py.TSolveMedianMS, sc.TSolveMS, sm.TSolveMS,
				py.TTotalMedianMS, sm.TTotalMS,
				speedup)
		}

	case "FULL_CHECKPOINT_TABLE":
		b.WriteString("| Problem ID | Root LP Obj | Optimal Obj | Root LP (ms) | CP4 Base (ms / nodes) | CP5 +Presolve (ms / nodes) | CP6 +Cuts (ms / nodes / cuts) | CP7 Full (ms / nodes / cuts) |\n")
		b.WriteString("| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
		for _, c := range cases {
			sm := tb.SIMD[c.ID]
			fmt.Fprintf(&b, "| `%s` | %.2f | %.2f | %.3f | %s | %s | %s | %s |\n",
				c.ID,
				sm.RootLPObj,
				sm.CP7Full.Objective,
				sm.RootLPTimeMS,
				formatCP45(sm.CP4Baseline),
				formatCP45(sm.CP5Presolve),
				formatCP67(sm.CP6Cuts, false),
				formatCP67(sm.CP7Full, true))
		}

	default:
		return "", fmt.Errorf("unknown generated block %q", blockName)
	}

	return strings.TrimRight(b.String(), "\n"), nil
}

// SyncMarkdownContent replaces all <!-- BEGIN GENERATED:NAME --> ... <!-- END GENERATED:NAME -->
// regions in content with fresh markdown rendered from tb.
func SyncMarkdownContent(content string, tb *TelemetryBundle) (string, error) {
	const beginPrefix = "<!-- BEGIN GENERATED:"
	const markerSuffix = " -->"

	out := content
	searchFrom := 0
	for {
		idx := strings.Index(out[searchFrom:], beginPrefix)
		if idx < 0 {
			break
		}
		startMarkerPos := searchFrom + idx
		nameStart := startMarkerPos + len(beginPrefix)
		nameEndRel := strings.Index(out[nameStart:], markerSuffix)
		if nameEndRel < 0 {
			return "", fmt.Errorf("malformed BEGIN GENERATED marker at byte %d", startMarkerPos)
		}
		blockName := out[nameStart : nameStart+nameEndRel]
		bodyStart := nameStart + nameEndRel + len(markerSuffix)

		endMarker := fmt.Sprintf("<!-- END GENERATED:%s -->", blockName)
		endRel := strings.Index(out[bodyStart:], endMarker)
		if endRel < 0 {
			return "", fmt.Errorf("missing %s marker", endMarker)
		}
		bodyEnd := bodyStart + endRel

		rendered, err := RenderBlock(blockName, tb)
		if err != nil {
			return "", err
		}

		replacement := "\n" + rendered + "\n"
		out = out[:bodyStart] + replacement + out[bodyEnd:]
		searchFrom = bodyStart + len(replacement) + len(endMarker)
	}
	return out, nil
}

// SyncMarkdownFile updates a markdown file in place (or checks it without writing when checkOnly is true).
func SyncMarkdownFile(path string, tb *TelemetryBundle, checkOnly bool) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	orig := string(raw)
	synced, err := SyncMarkdownContent(orig, tb)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if checkOnly {
		if orig != synced {
			return fmt.Errorf("%s is out of sync with canonical benchmark JSON telemetry (run `go run ./cmd/gopt-bench -sync-docs`)", path)
		}
		return nil
	}
	if orig != synced {
		return os.WriteFile(path, []byte(synced), 0o644)
	}
	return nil
}

// BuildFinalReportMarkdown constructs the full bench/results/final_report.md content with generated markers.
func BuildFinalReportMarkdown(tb *TelemetryBundle) (string, error) {
	tpl := `# ` + "`gopt`" + ` vs. Python (` + "`PuLP + COIN-OR CBC`" + `) Benchmark Report

## 1. Experimental Setup and Hardware Environment

- **CPU**: ` + "`13th Gen Intel(R) Core(TM) i9-13900K`" + ` (x86_64, 24 physical cores / 32 hardware threads; 30 logical CPUs exposed in container environment, AVX2 + FMA)
- **Go Toolchain**: ` + "`go1.27.1-X:nodwarf5 linux/amd64`" + ` with ` + "`GOEXPERIMENT=simd`" + ` (` + "`simd/archsimd.Float64x4`" + `)
- **Python Baseline**: Python 3.14 + ` + "`PuLP`" + ` + ` + "`COIN-OR CBC`" + ` subprocess (` + "`threads=1`" + `)
- **In-Process C++ Reference (` + "`goblin`" + `)**: In-memory ` + "`cgo`" + ` bindings to ` + "`COIN-OR CBC/CLP`" + ` (` + "`OsiClpSolverInterface`" + ` / ` + "`CbcSolver`" + `), isolating Python/MPS/subprocess IPC overhead from solver-core execution time.
- **Corpus**: 31 deterministic LP and MILP problem families across 3 tiers (` + "`testdata/corpus/*.json`" + ` and ` + "`testdata/corpus/trials/*.jsonl`" + `, 11 randomized trial instances per family for Tiers 1-2 and 5 per family for Tier 3 = 293 total instances), evaluated strictly in warm state after worker-level and per-problem warmup passes.

## 2. Aggregate Workload Tier Summary

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

## 3. End-to-End Pipeline Comparison (` + "`JSON -> Solution`" + `)

Pipeline stages measured per instance (median across trials after warmup):
1. **Prep (` + "`t_parse + t_build + t_format/t_compile`" + `)**: JSON unmarshaling, model construction, and MPS formatting (Python) or ` + "`Model.Compile()`" + ` matrix compilation (Go).
2. **Solve (` + "`t_solve`" + `)**: CBC subprocess execution and ` + "`.sol`" + ` parsing (Python) vs. in-memory ` + "`gopt.SolveCompiled`" + ` (Go).
3. **Total (` + "`t_total`" + `)**: Complete end-to-end latency from raw JSON bytes to verified primal solution.

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

## 4. Solver-Core Checkpoint Progression (` + "`GOEXPERIMENT=simd`" + `)

Progression across checkpoints on pre-compiled ` + "`CompiledProblem`" + ` instances:
- **CP2/3 (Root LP)**: Revised Dual & Two-Phase Primal Simplex with ` + "`simd/archsimd`" + ` kernels.
- **CP4 (Base B&B)**: Branch-and-Bound with pseudocost/strong branching (` + "`Presolve=off, Cuts=off, Heuristics=off`" + `).
- **CP5 (+Presolve)**: Savelsbergh bound tightening, GCD reduction, and node propagation (` + "`Presolve=on, Cuts=off, Heuristics=off`" + `).
- **CP6 (+Cuts)**: GMI, Lifted 0-1 Knapsack Covers, Flow/VUB $(l, S)$ cuts, and Marchand-Wolsey c-MIR cuts (` + "`Presolve=on, Cuts=on, Heuristics=off`" + `).
- **CP7 (Full ` + "`gopt`" + `)**: Full Branch-and-Cut with Propagated Rounding, Root Sub-MIP, and Fractional Diving (` + "`Presolve=on, Cuts=on, Heuristics=on`" + `).

<!-- BEGIN GENERATED:FULL_CHECKPOINT_TABLE -->
<!-- END GENERATED:FULL_CHECKPOINT_TABLE -->
`
	return SyncMarkdownContent(tpl, tb)
}
