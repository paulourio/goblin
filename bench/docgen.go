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
	// CBCSpawnFloorMS is the median prob.solve() time of a trivial 1-variable MILP: the fixed
	// cost of the PuLP + CBC subprocess path, independent of solver work.
	CBCSpawnFloorMS float64   `json:"cbc_spawn_floor_ms"`
	TSolveTrialsMS  []float64 `json:"t_solve_trials_ms,omitempty"`
	TTotalTrialsMS  []float64 `json:"t_total_trials_ms,omitempty"`
	// CPUAffinity is the CPU list the harness process was allowed to run on.
	CPUAffinity string `json:"cpu_affinity,omitempty"`
}

// CheckpointMetrics captures Solver-Core timings and counts for one solver setup.
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
	Backend         string            `json:"backend,omitempty"`
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
	// Per-trial timings in trial order (trial k is the same instance in every build), used
	// for paired build-to-build comparisons.
	TSolveTrialsMS []float64 `json:"t_solve_trials_ms,omitempty"`
	TTotalTrialsMS []float64 `json:"t_total_trials_ms,omitempty"`
	// Provenance of the measured binary.
	GitCommit string `json:"git_commit,omitempty"`
	GitDirty  bool   `json:"git_dirty,omitempty"`
	GoVersion string `json:"go_version,omitempty"`
	// SourceFingerprint identifies the exact sources measured (see SourceFingerprint).
	SourceFingerprint string `json:"source_fingerprint,omitempty"`
	// CPUAffinity is the CPU list the benchmark process was allowed to run on.
	CPUAffinity string `json:"cpu_affinity,omitempty"`
}

// TelemetryBundle holds the loaded benchmark results from canonical JSON files.
type TelemetryBundle struct {
	Python map[string]PythonBaselineEntry
	Goblin map[string]ProblemBenchResult
	Scalar map[string]ProblemBenchResult
	SIMD   map[string]ProblemBenchResult
}

// LoadTelemetryBundle reads the Python, pure-Go scalar, and pure-Go SIMD telemetry files plus the
// in-process CBC backend telemetry (goblin_cbc.json) stored next to the scalar file, when present.
func LoadTelemetryBundle(pyPath, scalarPath, simdPath string) (*TelemetryBundle, error) {
	var pyMap map[string]PythonBaselineEntry
	if err := readJSON(pyPath, &pyMap); err != nil {
		return nil, fmt.Errorf("python baseline: %w", err)
	}
	var scalarMap, simdMap map[string]ProblemBenchResult
	if err := readJSON(scalarPath, &scalarMap); err != nil {
		return nil, fmt.Errorf("scalar telemetry: %w", err)
	}
	if err := readJSON(simdPath, &simdMap); err != nil {
		return nil, fmt.Errorf("simd telemetry: %w", err)
	}
	var goblinMap map[string]ProblemBenchResult
	goblinPath := filepath.Join(filepath.Dir(scalarPath), "goblin_cbc.json")
	if _, err := os.Stat(goblinPath); err == nil {
		if err := readJSON(goblinPath, &goblinMap); err != nil {
			return nil, fmt.Errorf("goblin cbc telemetry: %w", err)
		}
	}
	return &TelemetryBundle{
		Python: pyMap,
		Goblin: goblinMap,
		Scalar: scalarMap,
		SIMD:   simdMap,
	}, nil
}

func readJSON(path string, dst any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("unmarshal %s: %w", path, err)
	}
	return nil
}

// ValidateTelemetryAgainstCorpus verifies that every problem in BuildCorpus() exists in
// testdata/corpus and in all telemetry maps with matching dimensions, that the telemetry
// carries consistent provenance, and that every goblin build (scalar, SIMD, and CBC when
// present) agrees with the Python CBC reference on status, canonical objective, and every per-trial
// objective. Rendered parity columns are therefore backed by this gate.
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

	var mismatches []string
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
		for label, m := range map[string]map[string]ProblemBenchResult{"scalar": tb.Scalar, "simd": tb.SIMD} {
			r, ok := m[c.ID]
			if !ok {
				return fmt.Errorf("missing %q in %s telemetry", c.ID, label)
			}
			if r.NumVars != compiled.NumVars || r.NumConstraints != compiled.NumConstraints {
				return fmt.Errorf("%s telemetry dimension mismatch for %q: got (%d, %d), want (%d, %d)",
					label, c.ID, r.NumVars, r.NumConstraints, compiled.NumVars, compiled.NumConstraints)
			}
		}
		mismatches = append(mismatches, CheckParity(tb, c.ID)...)
	}
	if len(mismatches) > 0 {
		return fmt.Errorf("parity check failed (%d mismatches):\n  %s", len(mismatches), strings.Join(mismatches, "\n  "))
	}
	if _, err := telemetryProvenance(tb); err != nil {
		return err
	}
	return nil
}

// CheckParity compares the scalar, SIMD, and CBC goblin telemetry (when present) for id
// against the Python CBC reference and returns one message per mismatch.
func CheckParity(tb *TelemetryBundle, id string) []string {
	py, ok := tb.Python[id]
	if !ok {
		return []string{id + ": missing python reference"}
	}
	builds := []struct {
		label string
		res   ProblemBenchResult
		ok    bool
	}{
		{label: "scalar"},
		{label: "simd"},
		{label: "cbc"},
	}
	builds[0].res, builds[0].ok = tb.Scalar[id]
	builds[1].res, builds[1].ok = tb.SIMD[id]
	builds[2].res, builds[2].ok = tb.Goblin[id]

	var out []string
	for i, b := range builds {
		if !b.ok {
			if i < minRequiredBuilds {
				out = append(out, fmt.Sprintf("%s/%s: missing telemetry", id, b.label))
			}
			continue
		}
		if b.res.CP7Full.Status != py.Status {
			out = append(out, fmt.Sprintf("%s/%s: status %s, CBC %s", id, b.label, b.res.CP7Full.Status, py.Status))
			continue
		}
		if py.Objective != nil && relDiff(b.res.CP7Full.Objective, *py.Objective) > parityRelTol {
			out = append(out, fmt.Sprintf("%s/%s: objective %.8g, CBC %.8g", id, b.label, b.res.CP7Full.Objective, *py.Objective))
		}
		for k, want := range py.TrialObjectives {
			if want == nil {
				continue
			}
			if k >= len(b.res.TrialObjectives) {
				out = append(out, fmt.Sprintf("%s/%s: trial %d missing", id, b.label, k))
				break
			}
			if got := b.res.TrialObjectives[k]; relDiff(got, *want) > parityRelTol {
				out = append(out, fmt.Sprintf("%s/%s: trial %d objective %.8g, CBC %.8g", id, b.label, k, got, *want))
			}
		}
	}
	return out
}

const (
	parityRelTol      = 1e-5
	minRequiredBuilds = 2
	madTimeHorizon    = 6
	geoMeanMinFloor   = 1e-6
	evenMedianHalf    = 0.5
	zCrit975          = 1.96
	minPairedSamples  = 2
	minNetSolveMS     = 0.001
	syncedDocPerm     = 0o644
)

func relDiff(a, b float64) float64 { return math.Abs(a-b) / max(1.0, math.Abs(b)) }

// provenance summarizes the code version that produced the goblin telemetry.
type provenance struct {
	commit      string
	dirty       bool
	goVersion   string
	fingerprint string
	affinity    map[string]bool
}

// telemetryProvenance requires every scalar, SIMD, and CBC entry to carry the same git commit
// and source fingerprint.
func telemetryProvenance(tb *TelemetryBundle) (provenance, error) {
	pv := provenance{affinity: map[string]bool{}}
	maps := []map[string]ProblemBenchResult{tb.Scalar, tb.SIMD}
	if len(tb.Goblin) > 0 {
		maps = append(maps, tb.Goblin)
	}
	for _, m := range maps {
		for id, r := range m {
			if r.GitCommit == "" || r.SourceFingerprint == "" {
				return pv, fmt.Errorf("telemetry entry %q lacks git_commit/source_fingerprint provenance (re-run goblin-bench)", id)
			}
			if pv.commit == "" {
				pv.commit, pv.goVersion, pv.fingerprint = r.GitCommit, r.GoVersion, r.SourceFingerprint
			}
			if r.GitCommit != pv.commit || r.SourceFingerprint != pv.fingerprint {
				return pv, fmt.Errorf("telemetry mixes sources (%s/%s vs %s/%s, entry %q)",
					pv.commit, pv.fingerprint, r.GitCommit, r.SourceFingerprint, id)
			}
			pv.dirty = pv.dirty || r.GitDirty
			pv.affinity[r.CPUAffinity] = true
		}
	}
	return pv, nil
}

func madDimensions(id string) (g, i, t int, ok bool) {
	if !strings.Contains(id, "_mad_") {
		return 0, 0, 0, false
	}
	idx := strings.Index(id, "_g")
	if idx < 0 {
		return 0, 0, 0, false
	}
	if _, err := fmt.Sscanf(id[idx:], "_g%d_i%d", &g, &i); err != nil {
		return 0, 0, 0, false
	}
	return g, i, madTimeHorizon, true
}

// isPlantedMAD reports whether id belongs to the structured MAD family whose planted
// zero-weight design yields a zero root LP gap (BuildMADAllocation).
func isPlantedMAD(id string) bool {
	_, _, _, ok := madDimensions(id)
	return ok && !strings.Contains(id, "_mad_rand_")
}

func geoMean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sumLog := 0.0
	for _, v := range vals {
		sumLog += math.Log(max(v, geoMeanMinFloor))
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
		med = evenMedianHalf * (cp[n/2-1] + cp[n/2])
	}
	return med, cp[0], cp[n-1]
}

// tCrit975 returns the two-sided 95% Student-t critical value for df degrees of freedom.
func tCrit975(df int) float64 {
	table := []float64{0, 12.706, 4.303, 3.182, 2.776, 2.571, 2.447, 2.365, 2.306, 2.262, 2.228,
		2.201, 2.179, 2.160, 2.145, 2.131, 2.120, 2.110, 2.101, 2.093, 2.086}
	if df <= 0 {
		return math.Inf(1)
	}
	if df < len(table) {
		return table[df]
	}
	return zCrit975
}

// pairedRatio returns the geometric-mean ratio a[k]/b[k] over paired trials and its 95%
// t-interval computed on the log ratios.
func pairedRatio(a, b []float64) (gm, lo, hi float64, n int) {
	var logs []float64
	for k := range min(len(a), len(b)) {
		if a[k] > 0 && b[k] > 0 {
			logs = append(logs, math.Log(a[k]/b[k]))
		}
	}
	n = len(logs)
	if n == 0 {
		return math.NaN(), math.NaN(), math.NaN(), 0
	}
	mean := 0.0
	for _, v := range logs {
		mean += v
	}
	mean /= float64(n)
	if n < minPairedSamples {
		return math.Exp(mean), math.Inf(-1), math.Inf(1), n
	}
	ss := 0.0
	for _, v := range logs {
		ss += (v - mean) * (v - mean)
	}
	half := tCrit975(n-1) * math.Sqrt(ss/float64(n-1)) / math.Sqrt(float64(n))
	return math.Exp(mean), math.Exp(mean - half), math.Exp(mean + half), n
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

// pyNetSolveMS is the Python solve time with the CBC subprocess spawn floor removed.
func pyNetSolveMS(py PythonBaselineEntry) float64 {
	return max(py.TSolveMedianMS-py.CBCSpawnFloorMS, minNetSolveMS)
}

func parityCell(tb *TelemetryBundle, id string) string {
	if len(CheckParity(tb, id)) == 0 {
		return "`OK`"
	}
	return "`MISMATCH`"
}

// affinityList renders the recorded CPU affinity sets; "unrecorded" marks missing values.
func affinityList(set map[string]bool) string {
	var parts []string
	for a := range set {
		if a == "" {
			a = "unrecorded"
		}
		parts = append(parts, "`"+a+"`")
	}
	slices.Sort(parts)
	return strings.Join(parts, ", ")
}

// simdReachSize is the smallest n + m for which an AVX2 Dot64/Axpy64 call (slice length
// >= 256) can occur; below it both builds run the same scalar kernels.
const simdReachSize = 256

// simdVerdict classifies one paired scalar/SIMD comparison. Instances below simdReachSize
// are negative controls: an interval excluding 1 there is noise, never a SIMD effect.
func simdVerdict(size int, lo, hi float64, n int) (reach, verdict string) {
	if size < simdReachSize {
		if n >= minPairedSamples && (lo > 1 || hi < 1) {
			return "no (control)", "identical source; interval excludes 1 (build/run effect)"
		}
		return "no (control)", "identical source"
	}
	switch {
	case n < minPairedSamples:
		return "possible", "insufficient trials"
	case lo > 1:
		return "possible", "SIMD faster"
	case hi < 1:
		return "possible", "SIMD slower"
	}
	return "possible", "no detectable effect"
}

// cbcBackendDescription states which CBC build and flag mapping produced goblin_cbc.json.
const cbcBackendDescription = "COIN-OR CBC/CLP `master` (commit `c1c171ec`, vendored in `third_party/Cbc`), linked in-process through `cgo` (`goblin.WithCBC()`). " +
	"The C++ backend maps default settings onto CBC with two deliberate differences: `MaxCutPasses = 12` becomes " +
	"`passC = 30` and `MaxNodes = 50000` becomes unlimited. It is therefore not a like-for-like setup and " +
	"not the CBC 2.10.3 binary used by the Python baseline."

// RenderBlock produces the markdown snippet for a named generated block marker.
//
// All headline timings use the default pure-Go scalar backend (`goblin` default). The
// GOEXPERIMENT=simd build is compared to it only in SIMD_EFFECT_TABLE, with paired statistics.
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
			{"Tier 1: Canonical (`N <= 26`)", func(c CorpusCase) bool { return c.Tier == "tier1_canonical" }},
			{"Tier 2: Target p90 (`30 <= N <= 84`)", func(c CorpusCase) bool { return c.Tier == "tier2_p90" }},
			{"Tier 3: Large Tail (`145 <= N <= 4036`)", func(c CorpusCase) bool { return c.Tier == "tier3_large" }},
			{"Tier 3 non-MAD MILPs", func(c CorpusCase) bool {
				_, _, _, isMAD := madDimensions(c.ID)
				return c.Tier == "tier3_large" && !isMAD
			}},
			{"Planted-gap MAD family (zero integrality gap by construction)", func(c CorpusCase) bool { return isPlantedMAD(c.ID) }},
			{"Random-weight MAD (negative controls)", func(c CorpusCase) bool { return strings.Contains(c.ID, "_mad_rand_") }},
			{"**All families except planted-gap MAD**", func(c CorpusCase) bool { return !isPlantedMAD(c.ID) }},
			{"All corpus families", func(_ CorpusCase) bool { return true }},
		}

		b.WriteString("| Workload Subset | Families | Py Total GeoMean (ms) | Py Net-of-Spawn GeoMean (ms) | `goblin (CBC)` GeoMean (ms) | `goblin (Go)` GeoMean (ms) | `Py / goblin (Go)` GeoMean | `Py / goblin (Go)` Median [Min-Max] | `Py net / goblin (Go)` GeoMean | `goblin (Go) / CBC` |\n")
		b.WriteString("| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
		for _, grp := range groups {
			var pyVals, pyNetVals, gbVals, goVals, speedups []float64
			for _, c := range cases {
				if !grp.filter(c) {
					continue
				}
				py := tb.Python[c.ID]
				goT := tb.Scalar[c.ID].TTotalMS
				pyVals = append(pyVals, py.TTotalMedianMS)
				pyNetVals = append(pyNetVals, max(py.TTotalMedianMS-py.CBCSpawnFloorMS, minNetSolveMS))
				if gb, ok := tb.Goblin[c.ID]; ok && gb.TTotalMS > 0 {
					gbVals = append(gbVals, gb.TTotalMS)
				}
				goVals = append(goVals, goT)
				if goT > 0 {
					speedups = append(speedups, py.TTotalMedianMS/goT)
				}
			}
			pyGM, pyNetGM, goGM := geoMean(pyVals), geoMean(pyNetVals), geoMean(goVals)
			medSp, minSp, maxSp := medianMinMax(speedups)
			gbStr, gbRatio := "N/A", "N/A"
			if len(gbVals) == len(goVals) && len(gbVals) > 0 {
				gbGM := geoMean(gbVals)
				gbStr = fmt.Sprintf("%.2f", gbGM)
				gbRatio = fmt.Sprintf("%.2fx", goGM/gbGM)
			}
			fmt.Fprintf(&b, "| %s | %d | %.2f | %.2f | %s | %.2f | **%.1fx** | %.1fx [%.1fx-%.1fx] | %.1fx | %s |\n",
				grp.label, len(goVals), pyGM, pyNetGM, gbStr, goGM, pyGM/goGM, medSp, minSp, maxSp, pyNetGM/goGM, gbRatio)
		}

	case "SIMD_EFFECT_TABLE":
		b.WriteString("| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Scalar Solve Median (ms) | SIMD Solve Median (ms) | Paired `scalar / SIMD` GeoMean [95% CI] | Trials | Verdict |\n")
		b.WriteString("| :--- | ---: | :---: | ---: | ---: | ---: | ---: | :--- |\n")
		for _, c := range cases {
			sc := tb.Scalar[c.ID]
			sm := tb.SIMD[c.ID]
			size := sm.NumVars + sm.NumConstraints
			gm, lo, hi, n := pairedRatio(sc.TSolveTrialsMS, sm.TSolveTrialsMS)
			reach, verdict := simdVerdict(size, lo, hi, n)
			fmt.Fprintf(&b, "| `%s` | %d | %s | %.3f | %.3f | %.2fx [%.2fx-%.2fx] | %d | %s |\n",
				c.ID, size, reach, sc.TSolveMS, sm.TSolveMS, gm, lo, hi, n, verdict)
		}

	case "SIMD_SUMMARY":
		var controls, controlsFlagged, reachable, faster, slower int
		for _, c := range cases {
			sm := tb.SIMD[c.ID]
			size := sm.NumVars + sm.NumConstraints
			_, lo, hi, n := pairedRatio(tb.Scalar[c.ID].TSolveTrialsMS, sm.TSolveTrialsMS)
			if n < minPairedSamples {
				continue
			}
			excludesOne := lo > 1 || hi < 1
			if size < simdReachSize {
				controls++
				if excludesOne {
					controlsFlagged++
				}
				continue
			}
			reachable++
			switch {
			case lo > 1:
				faster++
			case hi < 1:
				slower++
			}
		}
		fmt.Fprintf(&b, "- **Negative controls** (`n + m < %d`, no AVX2 kernel can run): %d of %d have a 95%% interval excluding 1. These differences come from the two binaries (compiler output and code layout differ under `GOEXPERIMENT=simd`) and from run-to-run effects, not from SIMD kernels. Intervals of similar size on the AVX2-capable instances are therefore not evidence of a SIMD effect.\n",
			simdReachSize, controlsFlagged, controls)
		fmt.Fprintf(&b, "- **Instances where AVX2 can run**: %d faster, %d slower, %d with no detectable effect, out of %d.\n",
			faster, slower, reachable-faster-slower, reachable)

	case "MAD_PIPELINE_TABLE":
		b.WriteString("| Problem ID | $G$ | $I$ | $T$ | Vars | Rows | Optimal Obj | PyPrep (ms) | GoPrep (ms) | PySolve (ms) | PySolve Net of Spawn (ms) | GoSolve (ms) | PyTotal (ms) | GoTotal (ms) | `Py / goblin (Go)` | Parity |\n")
		b.WriteString("| :--- | --: | ---: | --: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |\n")
		for _, c := range cases {
			g, i, t, ok := madDimensions(c.ID)
			if !ok {
				continue
			}
			py := tb.Python[c.ID]
			sc := tb.Scalar[c.ID]
			pyPrep := py.TParseMedianMS + py.TBuildMedianMS + py.TFormatMedianMS
			goPrep := sc.TParseMS + sc.TBuildMS + sc.TCompileMS
			fmt.Fprintf(&b, "| `%s` | %d | %d | %d | %d | %d | %.1f | %.3f | %.3f | %.2f | %.2f | %.2f | %.2f | %.2f | **%.1fx** | %s |\n",
				c.ID, g, i, t, sc.NumVars, sc.NumConstraints,
				sc.CP7Full.Objective,
				pyPrep, goPrep,
				py.TSolveMedianMS, pyNetSolveMS(py), sc.TSolveMS,
				py.TTotalMedianMS, sc.TTotalMS,
				py.TTotalMedianMS/sc.TTotalMS, parityCell(tb, c.ID))
		}

	case "MAD_CBC_GAP_TABLE":
		b.WriteString("| Problem ID | $G \\times I$ ($N \\times M$) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Py Net Solve (ms) | Go Total (ms) | CBC Total (ms) | Total (`Go / CBC`) |\n")
		b.WriteString("| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
		for _, c := range cases {
			g, i, _, ok := madDimensions(c.ID)
			if !ok {
				continue
			}
			py := tb.Python[c.ID]
			sc := tb.Scalar[c.ID]
			gb := tb.Goblin[c.ID]
			lpRatio := sc.RootLPTimeMS / max(gb.RootLPTimeMS, geoMeanMinFloor)
			totRatio := sc.TTotalMS / max(gb.TTotalMS, geoMeanMinFloor)
			fmt.Fprintf(&b, "| `%s` | $%d \\times %d$ ($%d \\times %d$) | %.3f (%d) | %.3f (%d) | %.2fx | %.2f (%d / %d) | %.2f (%d / %d) | %.2f | %.2f | %.2f | **%.2fx** |\n",
				c.ID, g, i, sc.NumVars, sc.NumConstraints,
				sc.RootLPTimeMS, sc.RootLPIters,
				gb.RootLPTimeMS, gb.RootLPIters,
				lpRatio,
				sc.TSolveMS, sc.CP7Full.Nodes, sc.CP7Full.Iterations,
				gb.TSolveMS, gb.CP7Full.Nodes, gb.CP7Full.Iterations,
				pyNetSolveMS(py),
				sc.TTotalMS, gb.TTotalMS, totRatio)
		}

	case "NON_MAD_CBC_GAP_TABLE":
		b.WriteString("| Problem ID | $N \\times M$ | Go CP5 +Presolve (ms / nodes) | Go CP6 +Cuts (ms / nodes / cuts) | Go CP7 Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Total Mean $\\pm$ SD (ms) | CBC Total Mean $\\pm$ SD (ms) | Total Median (`Go / CBC`) |\n")
		b.WriteString("| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
		gapIDs := []string{"t3_mkp_200", "t2_mkp_50", "t2_binpack_44", "t3_cflp_300", "t3_sched_1000"}
		for _, id := range gapIDs {
			sc, ok := tb.Scalar[id]
			if !ok {
				continue
			}
			gb := tb.Goblin[id]
			totRatio := sc.TTotalMS / max(gb.TTotalMS, geoMeanMinFloor)
			fmt.Fprintf(&b, "| `%s` | $%d \\times %d$ | %s | %s | %.2f (%d / %d) | %.2f (%d / %d) | $%.2f \\pm %.2f$ | $%.2f \\pm %.2f$ | **%.2fx** |\n",
				id, sc.NumVars, sc.NumConstraints,
				formatCP45(sc.CP5Presolve),
				formatCP67(sc.CP6Cuts, false),
				sc.TSolveMS, sc.CP7Full.Nodes, sc.CP7Full.Iterations,
				gb.TSolveMS, gb.CP7Full.Nodes, gb.CP7Full.Iterations,
				sc.TTotalMeanMS, sc.TTotalSDMS,
				gb.TTotalMeanMS, gb.TTotalSDMS,
				totRatio)
		}

	case "MAD_STATS_TABLE":
		b.WriteString("| Problem ID | $G \\times I$ | Python `PuLP + CBC` Total (Mean $\\pm$ SD [95% CI], ms) | `goblin (Go)` Scalar Total (Mean $\\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\\pm$ SD [95% CI], ms) |\n")
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
			tc := tCrit975(nTrials - 1)
			sqrtN := math.Sqrt(float64(max(1, nTrials)))
			fmt.Fprintf(&b, "| `%s` | $%d \\times %d$ | $%.2f \\pm %.2f$ ($\\pm %.2f$) | $%.2f \\pm %.2f$ ($\\pm %.2f$) | $%.2f \\pm %.2f$ ($\\pm %.2f$) |\n",
				c.ID, g, i,
				py.TTotalMeanMS, py.TTotalSDMS, tc*py.TTotalSDMS/sqrtN,
				sc.TTotalMeanMS, sc.TTotalSDMS, tc*sc.TTotalSDMS/sqrtN,
				sm.TTotalMeanMS, sm.TTotalSDMS, tc*sm.TTotalSDMS/sqrtN)
		}

	case "MAD_CHECKPOINT_TABLE", "FULL_CHECKPOINT_TABLE":
		b.WriteString("| Problem ID | Root LP Obj | Optimal Obj | Root LP (ms) | CP4 Base (ms / nodes) | CP5 +Presolve (ms / nodes) | CP6 +Cuts (ms / nodes / cuts) | CP7 Full (ms / nodes / cuts) |\n")
		b.WriteString("| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
		for _, c := range cases {
			if _, _, _, ok := madDimensions(c.ID); !ok && blockName == "MAD_CHECKPOINT_TABLE" {
				continue
			}
			sc := tb.Scalar[c.ID]
			fmt.Fprintf(&b, "| `%s` | %.2f | %.2f | %.3f | %s | %s | %s | %s |\n",
				c.ID,
				sc.RootLPObj,
				sc.CP7Full.Objective,
				sc.RootLPTimeMS,
				formatCP45(sc.CP4Baseline),
				formatCP45(sc.CP5Presolve),
				formatCP67(sc.CP6Cuts, false),
				formatCP67(sc.CP7Full, true))
		}

	case "FULL_PIPELINE_TABLE":
		b.WriteString("| Problem ID | Vars | Rows | PyPrep (ms) | GoPrep (ms) | PySolve (ms) | PySolve Net of Spawn (ms) | GoSolve (ms) | PyTotal (ms) | GoTotal (ms) | `Py / goblin (Go)` | Parity |\n")
		b.WriteString("| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |\n")
		for _, c := range cases {
			py := tb.Python[c.ID]
			sc := tb.Scalar[c.ID]
			pyPrep := py.TParseMedianMS + py.TBuildMedianMS + py.TFormatMedianMS
			goPrep := sc.TParseMS + sc.TBuildMS + sc.TCompileMS
			fmt.Fprintf(&b, "| `%s` | %d | %d | %.3f | %.3f | %.2f | %.2f | %.2f | %.2f | %.2f | **%.1fx** | %s |\n",
				c.ID, sc.NumVars, sc.NumConstraints,
				pyPrep, goPrep,
				py.TSolveMedianMS, pyNetSolveMS(py), sc.TSolveMS,
				py.TTotalMedianMS, sc.TTotalMS,
				py.TTotalMedianMS/sc.TTotalMS, parityCell(tb, c.ID))
		}

	case "PROVENANCE":
		pv, err := telemetryProvenance(tb)
		if err != nil {
			return "", err
		}
		var anyPy PythonBaselineEntry
		var floors []float64
		for _, c := range cases {
			anyPy = tb.Python[c.ID]
			floors = append(floors, tb.Python[c.ID].CBCSpawnFloorMS)
		}
		floorMed, _, _ := medianMinMax(floors)
		base := fmt.Sprintf("commit `%s`", pv.commit)
		if pv.dirty {
			base = fmt.Sprintf("working tree on top of commit `%s`", pv.commit)
		}
		fmt.Fprintf(&b, "- **goblin telemetry source**: fingerprint `%s` (%s), `%s`; the documentation gate rejects telemetry whose fingerprint differs from the checked-in sources.\n",
			pv.fingerprint, base, pv.goVersion)
		fmt.Fprintf(&b, "- **CPU affinity**: goblin %s; Python + CBC %s.\n", affinityList(pv.affinity), affinityList(map[string]bool{anyPy.CPUAffinity: true}))
		fmt.Fprintf(&b, "- **Python reference**: PuLP `%s`, CBC `%s` (`%s`).\n", anyPy.PuLPVersion, anyPy.CBCVersion, filepath.Base(anyPy.CBCPath))
		fmt.Fprintf(&b, "- **CBC subprocess spawn floor**: %.2f ms median (`prob.solve()` on a 1-variable MILP); the \"Net of Spawn\" columns subtract it.\n", floorMed)
		fmt.Fprintf(&b, "- **goblin `WithCBC()` backend**: %s\n", cbcBackendDescription)

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
			return fmt.Errorf("%s is out of sync with canonical benchmark JSON telemetry (run `go run ./cmd/goblin-bench -sync-docs`)", path)
		}
		return nil
	}
	if orig != synced {
		return os.WriteFile(path, []byte(synced), syncedDocPerm)
	}
	return nil
}

// BuildFinalReportMarkdown constructs the full bench/results/final_report.md content with generated markers.
func BuildFinalReportMarkdown(tb *TelemetryBundle) (string, error) {
	tpl := `# ` + "`goblin`" + ` vs. Python (` + "`PuLP + COIN-OR CBC`" + `) Benchmark Report

## 1. Experimental Setup and Provenance

` + "```bash\ngo run ./cmd/goblin-bench -check-docs\n```" + `

- **CPU**: ` + "`13th Gen Intel(R) Core(TM) i9-13900K`" + ` (x86_64, 30 logical CPUs exposed by ` + "`lscpu`" + `, AVX2 + FMA).
- **Default pure-Go backend**: ` + "`goblin`" + ` default (` + "`BackendGo`" + `, scalar kernels or ` + "`WithoutSIMD()`" + `). All headline ` + "`goblin (Go)`" + ` timings use this setup.
- **Experimental SIMD build**: ` + "`GOEXPERIMENT=simd`" + `, which dispatches AVX2 kernels only for slices of length 256 or more; see section 4.
- **Optional C++ backend**: ` + "`goblin.WithCBC()`" + ` (` + "`BackendCBC`" + `), driving in-process COIN-OR CBC/CLP through ` + "`cgo`" + ` under the same unified ` + "`goblin`" + ` API.
- **Python baseline**: Python 3.14 + ` + "`PuLP`" + ` + ` + "`COIN-OR CBC`" + ` subprocess (` + "`threads=1`" + `). Its solve time includes MPS writing, process spawn, and ` + "`.sol`" + ` parsing.
- **Corpus**: 31 deterministic LP and MILP families across 3 tiers (` + "`testdata/corpus/*.json`" + ` and ` + "`testdata/corpus/trials/*.jsonl`" + `; 11 randomized trial instances per family for Tiers 1-2 and 5 for Tier 3, 293 instances in total), measured in warm state.

<!-- BEGIN GENERATED:PROVENANCE -->
<!-- END GENERATED:PROVENANCE -->

## 2. Aggregate Workload Tier Summary

The planted-gap MAD family is constructed so that a root heuristic closes the gap at node 1; the bold row excludes it.

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

## 3. End-to-End Pipeline Comparison (` + "`JSON -> Solution`" + `)

Pipeline stages measured per instance (median across trials after warmup):
1. **Prep (` + "`t_parse + t_build + t_format/t_compile`" + `)**: JSON unmarshaling, model construction, and MPS formatting (Python) or ` + "`Model.Compile()`" + ` matrix assembly (Go).
2. **Solve (` + "`t_solve`" + `)**: CBC subprocess execution and ` + "`.sol`" + ` parsing (Python) vs. in-memory ` + "`goblin.SolveCompiled`" + ` (Go).
3. **Total (` + "`t_total`" + `)**: Complete end-to-end latency from raw JSON bytes to verified primal solution.

The Parity column is computed by ` + "`bench.CheckParity`" + `: status, canonical objective, and every per-trial objective of all ` + "`goblin`" + ` builds (` + "`scalar`" + `, ` + "`simd`" + `, and ` + "`cbc`" + `) must match CBC within a relative tolerance of 1e-5, and ` + "`ValidateTelemetryAgainstCorpus`" + ` rejects the telemetry otherwise.

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

## 4. Scalar vs. ` + "`GOEXPERIMENT=simd`" + ` Build

Ratios are geometric means of per-trial ` + "`scalar / SIMD`" + ` solve times over the same instances, with 95% t-intervals on the log ratios. "Reachable" is a necessary condition only: the AVX2 ` + "`Dot64`" + `/` + "`Axpy64`" + ` paths need a slice of length at least 256, which requires ` + "`n + m >= 256`" + `.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
<!-- END GENERATED:SIMD_EFFECT_TABLE -->

## 5. Solver-Core Checkpoint Progression (default pure-Go backend)

Progression across checkpoints on pre-compiled ` + "`CompiledProblem`" + ` instances:
- **CP2/3 (Root LP)**: Revised Dual and Two-Phase Primal Simplex.
- **CP4 (Base B&B)**: Branch-and-Bound with pseudocost/strong branching (` + "`Presolve=off, Cuts=off, Heuristics=off`" + `).
- **CP5 (+Presolve)**: Savelsbergh bound tightening, GCD reduction, and node propagation (` + "`Presolve=on, Cuts=off, Heuristics=off`" + `).
- **CP6 (+Cuts)**: GMI, lifted 0-1 knapsack covers, flow/VUB and $(l, S)$ cuts, and single-row MIR cuts (` + "`Presolve=on, Cuts=on, Heuristics=off`" + `).
- **CP7 (Full ` + "`goblin`" + `)**: Full Branch-and-Cut with Propagated Rounding, Root Sub-MIP, and Fractional Diving (` + "`Presolve=on, Cuts=on, Heuristics=on`" + `).

<!-- BEGIN GENERATED:FULL_CHECKPOINT_TABLE -->
<!-- END GENERATED:FULL_CHECKPOINT_TABLE -->
`
	return SyncMarkdownContent(tpl, tb)
}
