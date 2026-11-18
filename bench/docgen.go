package bench

import (
	"bytes"
	"encoding/json/v2"
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
	TrialStatuses         []string           `json:"trial_statuses,omitempty"`
	LPRelaxationObjective *float64           `json:"lp_relaxation_objective"`
	Solution              map[string]float64 `json:"solution"`
	Trials                int                `json:"trials"`
	PuLPVersion           string             `json:"pulp_version"`
	CBCPath               string             `json:"cbc_path"`
	CBCVersion            string             `json:"cbc_version"`
	CBCSha256             string             `json:"cbc_sha256,omitempty"`
	TParseMedianMS        float64            `json:"t_parse_median_ms"`
	TBuildMedianMS        float64            `json:"t_build_median_ms"`
	TFormatMedianMS       float64            `json:"t_format_median_ms"`
	TSolveMedianMS        float64            `json:"t_solve_median_ms"`
	TTotalMedianMS        float64            `json:"t_total_median_ms"`
	TTotalMeanMS          float64            `json:"t_total_mean_ms"`
	TTotalSDMS            float64            `json:"t_total_sd_ms"`
	CBCSpawnFloorMS       float64            `json:"cbc_spawn_floor_ms"`
	CBCLPSpawnFloorMS     float64            `json:"cbc_lp_spawn_floor_ms,omitzero"`
	TSolveTrialsMS        []float64          `json:"t_solve_trials_ms,omitempty"`
	TTotalTrialsMS        []float64          `json:"t_total_trials_ms,omitempty"`
	CPUAffinity           string             `json:"cpu_affinity,omitempty"`
	GitCommit             string             `json:"git_commit,omitempty"`
	GitDirty              bool               `json:"git_dirty,omitzero"`
	SourceFingerprint     string             `json:"source_fingerprint,omitempty"`
	UnameRelease          string             `json:"uname_release,omitempty"`
	MeasuredAt            string             `json:"measured_at,omitempty"`
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
	ID                string            `json:"id"`
	Tier              string            `json:"tier"`
	NumVars           int               `json:"num_vars"`
	NumConstraints    int               `json:"num_constraints"`
	IsMIP             bool              `json:"is_mip"`
	Backend           string            `json:"backend,omitempty"`
	SIMDEnabled       bool              `json:"simd_enabled"`
	TrialObjectives   []float64         `json:"trial_objectives,omitempty"`
	TrialStatuses     []string          `json:"trial_statuses,omitempty"`
	TParseMS          float64           `json:"t_parse_ms"`
	TBuildMS          float64           `json:"t_build_ms"`
	TCompileMS        float64           `json:"t_compile_ms"`
	TMPSWriteMS       float64           `json:"t_mps_write_ms"`
	TSolveMS          float64           `json:"t_solve_ms"`
	TTotalMS          float64           `json:"t_total_ms"`
	TTotalMeanMS      float64           `json:"t_total_mean_ms"`
	TTotalSDMS        float64           `json:"t_total_sd_ms"`
	RootLPTimeMS      float64           `json:"root_lp_time_ms"`
	RootLPIters       int               `json:"root_lp_iters"`
	RootLPObj         float64           `json:"root_lp_obj"`
	CP4Baseline       CheckpointMetrics `json:"cp4_baseline"`
	CP5Presolve       CheckpointMetrics `json:"cp5_presolve"`
	CP6Cuts           CheckpointMetrics `json:"cp6_cuts"`
	CP7Full           CheckpointMetrics `json:"cp7_full"`
	TSolveTrialsMS    []float64         `json:"t_solve_trials_ms,omitempty"`
	TTotalTrialsMS    []float64         `json:"t_total_trials_ms,omitempty"`
	GitCommit         string            `json:"git_commit,omitempty"`
	GitDirty          bool              `json:"git_dirty,omitzero"`
	GoVersion         string            `json:"go_version,omitempty"`
	SourceFingerprint string            `json:"source_fingerprint,omitempty"`
	CPUAffinity       string            `json:"cpu_affinity,omitempty"`
	UnameRelease      string            `json:"uname_release,omitempty"`
	MeasuredAt        string            `json:"measured_at,omitempty"`
}

// SciPyAssignEntry matches the JSON output produced by bench/scipy_assign_baseline.py.
type SciPyAssignEntry struct {
	ID                string    `json:"id"`
	Tier              string    `json:"tier"`
	Rows              int       `json:"rows"`
	Cols              int       `json:"cols"`
	Sense             int       `json:"sense"`
	Family            string    `json:"family"`
	Trials            int       `json:"trials"`
	SciPyVersion      string    `json:"scipy_version"`
	NumPyVersion      string    `json:"numpy_version"`
	CPUAffinity       string    `json:"cpu_affinity,omitempty"`
	GitCommit         string    `json:"git_commit,omitempty"`
	GitDirty          bool      `json:"git_dirty,omitzero"`
	SourceFingerprint string    `json:"source_fingerprint,omitempty"`
	UnameRelease      string    `json:"uname_release,omitempty"`
	MeasuredAt        string    `json:"measured_at,omitempty"`
	Objective         float64   `json:"objective"`
	TrialObjectives   []float64 `json:"trial_objectives,omitempty"`
	TPrepMedianMS     float64   `json:"t_prep_median_ms"`
	TSolveMedianMS    float64   `json:"t_solve_median_ms"`
	TTotalMedianMS    float64   `json:"t_total_median_ms"`
	TSolveMeanMS      float64   `json:"t_solve_mean_ms"`
	TSolveSDMS        float64   `json:"t_solve_sd_ms"`
	TTotalMeanMS      float64   `json:"t_total_mean_ms"`
	TTotalSDMS        float64   `json:"t_total_sd_ms"`
	TSolveTrialsMS    []float64 `json:"t_solve_trials_ms,omitempty"`
	TTotalTrialsMS    []float64 `json:"t_total_trials_ms,omitempty"`
}

// AssignBenchResult stores all pipeline and solve measurements for one LSAP corpus family.
type AssignBenchResult struct {
	ID                string    `json:"id"`
	Tier              string    `json:"tier"`
	Rows              int       `json:"rows"`
	Cols              int       `json:"cols"`
	Sense             int       `json:"sense"`
	Family            string    `json:"family"`
	SIMDEnabled       bool      `json:"simd_enabled"`
	Objective         float64   `json:"objective"`
	TrialObjectives   []float64 `json:"trial_objectives,omitempty"`
	TPrepMS           float64   `json:"t_prep_ms"`
	TSolveMS          float64   `json:"t_solve_ms"`
	TTotalMS          float64   `json:"t_total_ms"`
	TSolveMeanMS      float64   `json:"t_solve_mean_ms"`
	TSolveSDMS        float64   `json:"t_solve_sd_ms"`
	TTotalMeanMS      float64   `json:"t_total_mean_ms"`
	TTotalSDMS        float64   `json:"t_total_sd_ms"`
	TSolveTrialsMS    []float64 `json:"t_solve_trials_ms,omitempty"`
	TTotalTrialsMS    []float64 `json:"t_total_trials_ms,omitempty"`
	GitCommit         string    `json:"git_commit,omitempty"`
	GitDirty          bool      `json:"git_dirty,omitzero"`
	GoVersion         string    `json:"go_version,omitempty"`
	SourceFingerprint string    `json:"source_fingerprint,omitempty"`
	CPUAffinity       string    `json:"cpu_affinity,omitempty"`
	UnameRelease      string    `json:"uname_release,omitempty"`
	MeasuredAt        string    `json:"measured_at,omitempty"`
}

// TelemetryBundle holds the loaded benchmark results from canonical JSON files.
type TelemetryBundle struct {
	Python       map[string]PythonBaselineEntry
	Goblin       map[string]ProblemBenchResult
	Scalar       map[string]ProblemBenchResult
	SIMD         map[string]ProblemBenchResult
	SciPyAssign  map[string]SciPyAssignEntry
	AssignScalar map[string]AssignBenchResult
	AssignSIMD   map[string]AssignBenchResult
}

// LoadTelemetryBundle reads the Python, pure-Go scalar, and pure-Go SIMD telemetry files plus the
// in-process CBC backend telemetry (goblin_cbc.json) and the LSAP telemetry files
// (scipy_assign_baseline.json, goblin_assign_scalar.json, goblin_assign_simd.json) stored next to
// the scalar file.
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
	resultsDir := filepath.Dir(scalarPath)
	var goblinMap map[string]ProblemBenchResult
	goblinPath := filepath.Join(resultsDir, "goblin_cbc.json")
	if _, err := os.Stat(goblinPath); err == nil {
		if err := readJSON(goblinPath, &goblinMap); err != nil {
			return nil, fmt.Errorf("goblin cbc telemetry: %w", err)
		}
	}
	var scipyAssignMap map[string]SciPyAssignEntry
	if err := readJSON(filepath.Join(resultsDir, "scipy_assign_baseline.json"), &scipyAssignMap); err != nil {
		return nil, fmt.Errorf("scipy assign baseline: %w", err)
	}
	var assignScalarMap, assignSIMDMap map[string]AssignBenchResult
	if err := readJSON(filepath.Join(resultsDir, "goblin_assign_scalar.json"), &assignScalarMap); err != nil {
		return nil, fmt.Errorf("assign scalar telemetry: %w", err)
	}
	if err := readJSON(filepath.Join(resultsDir, "goblin_assign_simd.json"), &assignSIMDMap); err != nil {
		return nil, fmt.Errorf("assign simd telemetry: %w", err)
	}
	return &TelemetryBundle{
		Python:       pyMap,
		Goblin:       goblinMap,
		Scalar:       scalarMap,
		SIMD:         simdMap,
		SciPyAssign:  scipyAssignMap,
		AssignScalar: assignScalarMap,
		AssignSIMD:   assignSIMDMap,
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
// testdata/corpus and in all telemetry maps with matching dimensions, that all seven telemetry
// files carry uniform provenance, and that every goblin build agrees with the Python CBC
// reference on status, canonical objective, per-trial status, and per-trial objective.
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
		if py.NumVars != compiled.NumVars() || py.NumConstraints != compiled.NumConstraints() {
			return fmt.Errorf("python baseline dimension mismatch for %q: got (%d, %d), want (%d, %d)",
				c.ID, py.NumVars, py.NumConstraints, compiled.NumVars(), compiled.NumConstraints())
		}
		for label, m := range map[string]map[string]ProblemBenchResult{"scalar": tb.Scalar, "simd": tb.SIMD} {
			r, ok := m[c.ID]
			if !ok {
				return fmt.Errorf("missing %q in %s telemetry", c.ID, label)
			}
			if r.NumVars != compiled.NumVars() || r.NumConstraints != compiled.NumConstraints() {
				return fmt.Errorf("%s telemetry dimension mismatch for %q: got (%d, %d), want (%d, %d)",
					label, c.ID, r.NumVars, r.NumConstraints, compiled.NumVars(), compiled.NumConstraints())
			}
		}
		mismatches = append(mismatches, CheckParity(tb, c.ID)...)
	}

	assignCases := BuildAssignCorpus()
	assignManifest := filepath.Join(filepath.Dir(corpusDir), "assign_corpus", "manifest.json")
	if _, err := os.Stat(assignManifest); err != nil {
		return fmt.Errorf("missing assign corpus manifest %s: %w", assignManifest, err)
	}
	if len(tb.SciPyAssign) != len(assignCases) {
		return fmt.Errorf("scipy assign baseline has %d entries, assign corpus has %d", len(tb.SciPyAssign), len(assignCases))
	}
	if len(tb.AssignScalar) != len(assignCases) {
		return fmt.Errorf("assign scalar telemetry has %d entries, assign corpus has %d", len(tb.AssignScalar), len(assignCases))
	}
	if len(tb.AssignSIMD) != len(assignCases) {
		return fmt.Errorf("assign simd telemetry has %d entries, assign corpus has %d", len(tb.AssignSIMD), len(assignCases))
	}
	for _, ac := range assignCases {
		sp, ok := tb.SciPyAssign[ac.ID]
		if !ok {
			return fmt.Errorf("missing %q in scipy assign baseline", ac.ID)
		}
		if sp.Rows != ac.Rows || sp.Cols != ac.Cols {
			return fmt.Errorf("scipy assign dimension mismatch for %q: got (%d, %d), want (%d, %d)",
				ac.ID, sp.Rows, sp.Cols, ac.Rows, ac.Cols)
		}
		for label, m := range map[string]map[string]AssignBenchResult{"assign_scalar": tb.AssignScalar, "assign_simd": tb.AssignSIMD} {
			r, ok := m[ac.ID]
			if !ok {
				return fmt.Errorf("missing %q in %s telemetry", ac.ID, label)
			}
			if r.Rows != ac.Rows || r.Cols != ac.Cols {
				return fmt.Errorf("%s telemetry dimension mismatch for %q: got (%d, %d), want (%d, %d)",
					label, ac.ID, r.Rows, r.Cols, ac.Rows, ac.Cols)
			}
		}
		mismatches = append(mismatches, CheckAssignParity(tb, ac.ID)...)
	}

	if len(mismatches) > 0 {
		return fmt.Errorf("parity check failed (%d mismatches):\n  %s", len(mismatches), strings.Join(mismatches, "\n  "))
	}
	if _, err := telemetryProvenance(tb); err != nil {
		return err
	}
	return nil
}

// CheckAssignParity compares the scalar and SIMD goblin/assign telemetry for id against
// the SciPy reference and returns one message per mismatch.
func CheckAssignParity(tb *TelemetryBundle, id string) []string {
	sp, ok := tb.SciPyAssign[id]
	if !ok {
		return []string{id + ": missing scipy assign reference"}
	}
	builds := []struct {
		label string
		res   AssignBenchResult
		ok    bool
	}{
		{label: "assign_scalar"},
		{label: "assign_simd"},
	}
	builds[0].res, builds[0].ok = tb.AssignScalar[id]
	builds[1].res, builds[1].ok = tb.AssignSIMD[id]

	var out []string
	for _, b := range builds {
		if !b.ok {
			out = append(out, fmt.Sprintf("%s/%s: missing telemetry", id, b.label))
			continue
		}
		if relDiff(b.res.Objective, sp.Objective) > assignParityRelTol {
			out = append(out, fmt.Sprintf("%s/%s: objective %.10g, SciPy %.10g", id, b.label, b.res.Objective, sp.Objective))
		}
		for k, want := range sp.TrialObjectives {
			if k >= len(b.res.TrialObjectives) {
				out = append(out, fmt.Sprintf("%s/%s: trial %d missing", id, b.label, k))
				break
			}
			if got := b.res.TrialObjectives[k]; relDiff(got, want) > assignParityRelTol {
				out = append(out, fmt.Sprintf("%s/%s: trial %d objective %.10g, SciPy %.10g", id, b.label, k, got, want))
			}
		}
	}
	return out
}

// CheckParity compares the scalar, SIMD, and CBC goblin telemetry (when present) for id
// against the Python CBC reference across canonical and per-trial statuses and objectives.
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
		for k, wantSt := range py.TrialStatuses {
			if k < len(b.res.TrialStatuses) && wantSt != "" && b.res.TrialStatuses[k] != "" && b.res.TrialStatuses[k] != wantSt {
				out = append(out, fmt.Sprintf("%s/%s: trial %d status %s, CBC %s", id, b.label, k, b.res.TrialStatuses[k], wantSt))
			}
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
	parityRelTol       = 1e-5
	assignParityRelTol = 1e-9
	minRequiredBuilds  = 2
	madTimeHorizon     = 6
	geoMeanMinFloor    = 1e-6
	evenMedianHalf     = 0.5
	zCrit975           = 1.96
	minPairedSamples   = 2
	syncedDocPerm      = 0o644
)

func relDiff(a, b float64) float64 { return math.Abs(a-b) / max(1.0, math.Abs(b)) }

// provenance summarizes the code version and host kernel that produced all benchmark telemetry.
type provenance struct {
	commit       string
	dirty        bool
	goVersion    string
	fingerprint  string
	unameRelease string
	affinity     map[string]bool
}

// telemetryProvenance requires every Python, SciPy, scalar, SIMD, CBC, and LSAP entry to carry
// the same git commit, source fingerprint, and host kernel release.
func telemetryProvenance(tb *TelemetryBundle) (provenance, error) {
	pv := provenance{affinity: map[string]bool{}}
	checkRec := func(sourceLabel, id, commit, fp, uname, goVer, aff string, dirty bool) error {
		if commit == "" || fp == "" {
			return fmt.Errorf("%s entry %q lacks git_commit/source_fingerprint provenance (re-run benchmark suite)", sourceLabel, id)
		}
		if pv.commit == "" {
			pv.commit = commit
			pv.fingerprint = fp
			pv.unameRelease = uname
		}
		if goVer != "" && pv.goVersion == "" {
			pv.goVersion = goVer
		}
		if commit != pv.commit || fp != pv.fingerprint {
			return fmt.Errorf("%s mixes sources (%s/%s vs %s/%s, entry %q)",
				sourceLabel, pv.commit, pv.fingerprint, commit, fp, id)
		}
		if uname != "" && pv.unameRelease != "" && uname != pv.unameRelease {
			return fmt.Errorf("%s mixes host kernels (%s vs %s, entry %q)", sourceLabel, pv.unameRelease, uname, id)
		}
		if uname != "" && pv.unameRelease == "" {
			pv.unameRelease = uname
		}
		pv.dirty = pv.dirty || dirty
		if aff != "" {
			pv.affinity[aff] = true
		}
		return nil
	}

	for id, py := range tb.Python {
		if err := checkRec("python_baseline", id, py.GitCommit, py.SourceFingerprint, py.UnameRelease, "", py.CPUAffinity, py.GitDirty); err != nil {
			return pv, err
		}
	}
	for id, sp := range tb.SciPyAssign {
		if err := checkRec("scipy_assign_baseline", id, sp.GitCommit, sp.SourceFingerprint, sp.UnameRelease, "", sp.CPUAffinity, sp.GitDirty); err != nil {
			return pv, err
		}
	}

	maps := []struct {
		label string
		m     map[string]ProblemBenchResult
	}{
		{"goblin_scalar", tb.Scalar},
		{"goblin_simd", tb.SIMD},
	}
	if len(tb.Goblin) > 0 {
		maps = append(maps, struct {
			label string
			m     map[string]ProblemBenchResult
		}{"goblin_cbc", tb.Goblin})
	}
	for _, item := range maps {
		for id, r := range item.m {
			if err := checkRec(item.label, id, r.GitCommit, r.SourceFingerprint, r.UnameRelease, r.GoVersion, r.CPUAffinity, r.GitDirty); err != nil {
				return pv, err
			}
		}
	}
	for _, item := range []struct {
		label string
		m     map[string]AssignBenchResult
	}{
		{"goblin_assign_scalar", tb.AssignScalar},
		{"goblin_assign_simd", tb.AssignSIMD},
	} {
		for id, r := range item.m {
			if err := checkRec(item.label, id, r.GitCommit, r.SourceFingerprint, r.UnameRelease, r.GoVersion, r.CPUAffinity, r.GitDirty); err != nil {
				return pv, err
			}
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

func parityCell(tb *TelemetryBundle, id string) string {
	if len(CheckParity(tb, id)) == 0 {
		return "`OK`"
	}
	return "`MISMATCH`"
}

func assignParityCell(tb *TelemetryBundle, id string) string {
	if len(CheckAssignParity(tb, id)) == 0 {
		return "`OK`"
	}
	return "`MISMATCH`"
}

func formatAssignMS(ms float64) string {
	if ms < 0.1 {
		return fmt.Sprintf("%.4f", ms)
	}
	return fmt.Sprintf("%.3f", ms)
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

// simdReachSize is the smallest n + m for which an AVX2 Dot64/Axpy64/Scale64 call (slice length
// >= 32) can occur; below it both builds run the same scalar kernels.
const simdReachSize = 32

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
const cbcBackendDescription = "COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). " +
	"Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides."

// RenderBlock produces the markdown snippet for a named generated block marker.
//
// All headline timings use the default pure-Go scalar backend (`goblin` default). The
// GOEXPERIMENT=simd build is compared to it only in SIMD_EFFECT_TABLE, with paired statistics.
func RenderBlock(blockName string, tb *TelemetryBundle) (string, error) {
	cases := BuildCorpus()
	var b bytes.Buffer

	switch blockName {
	case "TIER_SUMMARY_TABLE":
		renderTierSummaryTable(&b, cases, tb)
	case "SIMD_EFFECT_TABLE":
		renderSIMDEffectTable(&b, cases, tb)
	case "SIMD_SUMMARY":
		renderSIMDSummary(&b, cases, tb)
	case "MAD_PIPELINE_TABLE":
		renderMADPipelineTable(&b, cases, tb)
	case "MAD_CBC_GAP_TABLE":
		renderMADCBCGapTable(&b, cases, tb)
	case "NON_MAD_CBC_GAP_TABLE":
		renderNonMADCBCGapTable(&b, tb)
	case "MAD_STATS_TABLE":
		renderMADStatsTable(&b, cases, tb)
	case "MAD_CHECKPOINT_TABLE":
		renderCheckpointTable(&b, cases, tb, true)
	case "FULL_CHECKPOINT_TABLE":
		renderCheckpointTable(&b, cases, tb, false)
	case "FULL_PIPELINE_TABLE":
		renderFullPipelineTable(&b, cases, tb)
	case "PROVENANCE":
		if err := renderProvenance(&b, cases, tb); err != nil {
			return "", err
		}
	case "ASSIGN_TIER_SUMMARY_TABLE":
		renderAssignTierSummaryTable(&b, tb)
	case "ASSIGN_PIPELINE_TABLE":
		renderAssignPipelineTable(&b, tb)
	case "ASSIGN_SIMD_SUMMARY":
		renderAssignSIMDSummary(&b, tb)
	case "ASSIGN_SIMD_TABLE":
		renderAssignSIMDTable(&b, tb)
	default:
		return "", fmt.Errorf("unknown generated block %q", blockName)
	}

	return strings.TrimRight(b.String(), "\n"), nil
}

func renderTierSummaryTable(b *bytes.Buffer, cases []CorpusCase, tb *TelemetryBundle) {
	type groupSpec struct {
		label  string
		filter func(c CorpusCase) bool
	}
	groups := []groupSpec{
		{"Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated)", func(c CorpusCase) bool { return c.Tier == "tier1_canonical" }},
		{"Tier 2: Target p90 MILPs (`30 <= N <= 84`)", func(c CorpusCase) bool { return c.Tier == "tier2_p90" }},
		{"Tier 3: Large tail MILPs (`145 <= N <= 4036`)", func(c CorpusCase) bool { return c.Tier == "tier3_large" }},
		{"**Tier 2+3 non-planted MILPs (solver-dominated)**", func(c CorpusCase) bool {
			return c.Tier != "tier1_canonical" && !isPlantedMAD(c.ID)
		}},
		{"Tier 3 non-MAD MILPs", func(c CorpusCase) bool {
			_, _, _, isMAD := madDimensions(c.ID)
			return c.Tier == "tier3_large" && !isMAD
		}},
		{"Planted-gap MAD family (zero root LP gap by design)", func(c CorpusCase) bool { return isPlantedMAD(c.ID) }},
		{"Random-weight MAD (negative controls)", func(c CorpusCase) bool { return strings.Contains(c.ID, "_mad_rand_") }},
		{"All non-planted families (Tiers 1-3)", func(c CorpusCase) bool { return !isPlantedMAD(c.ID) }},
		{"All 31 synthetic corpus families", func(_ CorpusCase) bool { return true }},
	}

	b.WriteString("| Workload Subset | Families (Instances) | Paired `Py / Go` GeoMean [95% CI] | Paired `Go / CBC` | `Go` Sum (ms) | `CBC` Sum (ms) | `Py` Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |\n")
	b.WriteString("| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, grp := range groups {
		var nFams int
		var pyPaired, goPaired, cbcPaired, instSpeedups []float64
		var goSum, cbcSum, pySum float64
		hasAllCBC := true
		for _, c := range cases {
			if !grp.filter(c) {
				continue
			}
			nFams++
			py := tb.Python[c.ID]
			sc := tb.Scalar[c.ID]
			gbRes, hasCBC := tb.Goblin[c.ID]
			if !hasCBC || len(gbRes.TTotalTrialsMS) == 0 {
				hasAllCBC = false
			}
			nInst := min(len(py.TTotalTrialsMS), len(sc.TTotalTrialsMS))
			if nInst == 0 {
				pyPaired = append(pyPaired, py.TTotalMedianMS)
				goPaired = append(goPaired, sc.TTotalMS)
				pySum += py.TTotalMedianMS
				goSum += sc.TTotalMS
				if sc.TTotalMS > 0 {
					instSpeedups = append(instSpeedups, py.TTotalMedianMS/sc.TTotalMS)
				}
				if hasCBC {
					cbcPaired = append(cbcPaired, gbRes.TTotalMS)
					cbcSum += gbRes.TTotalMS
				}
				continue
			}
			for k := range nInst {
				pt := py.TTotalTrialsMS[k]
				gt := sc.TTotalTrialsMS[k]
				pyPaired = append(pyPaired, pt)
				goPaired = append(goPaired, gt)
				pySum += pt
				goSum += gt
				if gt > 0 {
					instSpeedups = append(instSpeedups, pt/gt)
				}
				if hasCBC && k < len(gbRes.TTotalTrialsMS) {
					ct := gbRes.TTotalTrialsMS[k]
					cbcPaired = append(cbcPaired, ct)
					cbcSum += ct
				}
			}
		}
		gm, lo, hi, nInst := pairedRatio(pyPaired, goPaired)
		medSp, minSp, maxSp := medianMinMax(instSpeedups)
		cbcRatioStr, cbcSumStr := "N/A", "N/A"
		if hasAllCBC && len(cbcPaired) == len(goPaired) && len(cbcPaired) > 0 {
			cbcGM, _, _, _ := pairedRatio(goPaired, cbcPaired)
			cbcRatioStr = fmt.Sprintf("%.2fx", cbcGM)
			cbcSumStr = fmt.Sprintf("%.1f", cbcSum)
		}
		fmt.Fprintf(b, "| %s | %d (%d) | **%.2fx** [%.2fx-%.2fx] | %s | %.1f | %s | %.1f | %.2fx [%.2fx-%.2fx] |\n",
			grp.label, nFams, nInst, gm, lo, hi, cbcRatioStr, goSum, cbcSumStr, pySum, medSp, minSp, maxSp)
	}
}

func renderSIMDEffectTable(b *bytes.Buffer, cases []CorpusCase, tb *TelemetryBundle) {
	b.WriteString("| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |\n")
	b.WriteString("| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |\n")
	for _, c := range cases {
		sc := tb.Scalar[c.ID]
		sm := tb.SIMD[c.ID]
		size := sm.NumVars + sm.NumConstraints
		gm, lo, hi, n := pairedRatio(sc.TSolveTrialsMS, sm.TSolveTrialsMS)
		reach, verdict := simdVerdict(size, lo, hi, n)
		fmt.Fprintf(b, "| `%s` | %d | %s | %s | %.2fx [%.2fx-%.2fx] | %.3f | %.3f | %d |\n",
			c.ID, size, reach, verdict, gm, lo, hi, sc.TSolveMS, sm.TSolveMS, n)
	}
}

func renderSIMDSummary(b *bytes.Buffer, cases []CorpusCase, tb *TelemetryBundle) {
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
	fmt.Fprintf(b, "- **Negative controls** (`n + m < %d`, no AVX2 kernel can run): %d of %d have a 95%% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.\n",
		simdReachSize, controlsFlagged, controls)
	fmt.Fprintf(b, "- **Instances where AVX2 can run**: %d faster, %d slower, %d with no detectable effect, out of %d.\n",
		faster, slower, reachable-faster-slower, reachable)
}

func familyTrialRatioRange(py PythonBaselineEntry, sc ProblemBenchResult) (gm, minR, maxR float64) {
	n := min(len(py.TTotalTrialsMS), len(sc.TTotalTrialsMS))
	if n == 0 {
		r := py.TTotalMedianMS / max(sc.TTotalMS, geoMeanMinFloor)
		return r, r, r
	}
	ratios := make([]float64, 0, n)
	for k := range n {
		if sc.TTotalTrialsMS[k] > 0 {
			ratios = append(ratios, py.TTotalTrialsMS[k]/sc.TTotalTrialsMS[k])
		}
	}
	gm, _, _, _ = pairedRatio(py.TTotalTrialsMS, sc.TTotalTrialsMS)
	_, minR, maxR = medianMinMax(ratios)
	return gm, minR, maxR
}

func renderMADPipelineTable(b *bytes.Buffer, cases []CorpusCase, tb *TelemetryBundle) {
	b.WriteString("| Problem ID | $G \\times I$ | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Trial 0 Obj | Parity |\n")
	b.WriteString("| :--- | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |\n")
	for _, c := range cases {
		g, i, _, ok := madDimensions(c.ID)
		if !ok {
			continue
		}
		py := tb.Python[c.ID]
		sc := tb.Scalar[c.ID]
		pyPrep := py.TParseMedianMS + py.TBuildMedianMS + py.TFormatMedianMS
		goPrep := sc.TParseMS + sc.TBuildMS + sc.TCompileMS
		gm, minR, maxR := familyTrialRatioRange(py, sc)
		fmt.Fprintf(b, "| `%s` | $%d \\times %d$ | %d | %d | **%.1fx** [%.1fx-%.1fx] | %.2f | %.2f | %.2f | %.2f | %.3f | %.3f | %.1f | %s |\n",
			c.ID,
			g, i, sc.NumVars, sc.NumConstraints,
			gm, minR, maxR,
			sc.TTotalMS, py.TTotalMedianMS,
			sc.TSolveMS, py.TSolveMedianMS,
			goPrep, pyPrep,
			sc.CP7Full.Objective,
			parityCell(tb, c.ID))
	}
}

func renderMADCBCGapTable(b *bytes.Buffer, cases []CorpusCase, tb *TelemetryBundle) {
	b.WriteString("| Problem ID | $G \\times I$ ($N \\times M$) | Paired `Go / CBC` | Go Total Med (ms) | CBC Total Med (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) |\n")
	b.WriteString("| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, c := range cases {
		g, i, _, ok := madDimensions(c.ID)
		if !ok {
			continue
		}
		sc := tb.Scalar[c.ID]
		gb := tb.Goblin[c.ID]
		lpRatio := sc.RootLPTimeMS / max(gb.RootLPTimeMS, geoMeanMinFloor)
		totRatio, _, _, _ := pairedRatio(sc.TTotalTrialsMS, gb.TTotalTrialsMS)
		if math.IsNaN(totRatio) {
			totRatio = sc.TTotalMS / max(gb.TTotalMS, geoMeanMinFloor)
		}
		fmt.Fprintf(b, "| `%s` | $%d \\times %d$ ($%d \\times %d$) | **%.2fx** | %.2f | %.2f | %.2fx | %.2f (%d / %d) | %.2f (%d / %d) | %.3f (%d) | %.3f (%d) |\n",
			c.ID,
			g, i, sc.NumVars, sc.NumConstraints,
			totRatio,
			sc.TTotalMS, gb.TTotalMS,
			lpRatio,
			sc.TSolveMS, sc.CP7Full.Nodes, sc.CP7Full.Iterations,
			gb.TSolveMS, gb.CP7Full.Nodes, gb.CP7Full.Iterations,
			sc.RootLPTimeMS, sc.RootLPIters,
			gb.RootLPTimeMS, gb.RootLPIters)
	}
}

func renderNonMADCBCGapTable(b *bytes.Buffer, tb *TelemetryBundle) {
	b.WriteString("| Problem ID | $N \\times M$ | Paired `Go / CBC` | Go Total Mean $\\pm$ SD (ms) | CBC Total Mean $\\pm$ SD (ms) | Go CP7 Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go CP6 +Cuts (ms / nodes / cuts) | Go CP5 +Presolve (ms / nodes) |\n")
	b.WriteString("| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	gapIDs := []string{"t3_mkp_200", "t2_mkp_50", "t2_binpack_44", "t3_cflp_300", "t3_lotsizing_504", "t3_sched_1000"}
	for _, id := range gapIDs {
		sc, ok := tb.Scalar[id]
		if !ok {
			continue
		}
		gb := tb.Goblin[id]
		totRatio, _, _, _ := pairedRatio(sc.TTotalTrialsMS, gb.TTotalTrialsMS)
		if math.IsNaN(totRatio) {
			totRatio = sc.TTotalMS / max(gb.TTotalMS, geoMeanMinFloor)
		}
		fmt.Fprintf(b, "| `%s` | $%d \\times %d$ | **%.2fx** | $%.2f \\pm %.2f$ | $%.2f \\pm %.2f$ | %.2f (%d / %d) | %.2f (%d / %d) | %s | %s |\n",
			id,
			sc.NumVars, sc.NumConstraints,
			totRatio,
			sc.TTotalMeanMS, sc.TTotalSDMS,
			gb.TTotalMeanMS, gb.TTotalSDMS,
			sc.TSolveMS, sc.CP7Full.Nodes, sc.CP7Full.Iterations,
			gb.TSolveMS, gb.CP7Full.Nodes, gb.CP7Full.Iterations,
			formatCP67(sc.CP6Cuts, false),
			formatCP45(sc.CP5Presolve))
	}
}

func renderMADStatsTable(b *bytes.Buffer, cases []CorpusCase, tb *TelemetryBundle) {
	b.WriteString("| Problem ID | $G \\times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\\pm$ SD [95% CI], ms) |\n")
	b.WriteString("| :--- | :---: | ---: | ---: | ---: | ---: |\n")
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
		fmt.Fprintf(b, "| `%s` | $%d \\times %d$ | %d | $%.2f \\pm %.2f$ ($\\pm %.2f$) | $%.2f \\pm %.2f$ ($\\pm %.2f$) | $%.2f \\pm %.2f$ ($\\pm %.2f$) |\n",
			c.ID,
			g, i, nTrials,
			sc.TTotalMeanMS, sc.TTotalSDMS, tc*sc.TTotalSDMS/sqrtN,
			py.TTotalMeanMS, py.TTotalSDMS, tc*py.TTotalSDMS/sqrtN,
			sm.TTotalMeanMS, sm.TTotalSDMS, tc*sm.TTotalSDMS/sqrtN)
	}
}

func renderCheckpointTable(b *bytes.Buffer, cases []CorpusCase, tb *TelemetryBundle, madOnly bool) {
	b.WriteString("| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |\n")
	b.WriteString("| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, c := range cases {
		if _, _, _, ok := madDimensions(c.ID); !ok && madOnly {
			continue
		}
		sc := tb.Scalar[c.ID]
		fmt.Fprintf(b, "| `%s` | %s | %s | %s | %s | %.3f | %.2f | %.2f |\n",
			c.ID,
			formatCP67(sc.CP7Full, true),
			formatCP67(sc.CP6Cuts, false),
			formatCP45(sc.CP5Presolve),
			formatCP45(sc.CP4Baseline),
			sc.RootLPTimeMS,
			sc.CP7Full.Objective,
			sc.RootLPObj)
	}
}

func renderFullPipelineTable(b *bytes.Buffer, cases []CorpusCase, tb *TelemetryBundle) {
	b.WriteString("| Problem ID | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Parity |\n")
	b.WriteString("| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |\n")
	for _, c := range cases {
		py := tb.Python[c.ID]
		sc := tb.Scalar[c.ID]
		pyPrep := py.TParseMedianMS + py.TBuildMedianMS + py.TFormatMedianMS
		goPrep := sc.TParseMS + sc.TBuildMS + sc.TCompileMS
		gm, minR, maxR := familyTrialRatioRange(py, sc)
		fmt.Fprintf(b, "| `%s` | %d | %d | **%.1fx** [%.1fx-%.1fx] | %.2f | %.2f | %.2f | %.2f | %.3f | %.3f | %s |\n",
			c.ID,
			sc.NumVars, sc.NumConstraints,
			gm, minR, maxR,
			sc.TTotalMS, py.TTotalMedianMS,
			sc.TSolveMS, py.TSolveMedianMS,
			goPrep, pyPrep,
			parityCell(tb, c.ID))
	}
}

func renderProvenance(b *bytes.Buffer, cases []CorpusCase, tb *TelemetryBundle) error {
	pv, err := telemetryProvenance(tb)
	if err != nil {
		return err
	}
	var anyPy PythonBaselineEntry
	var mipFloors, lpFloors []float64
	for _, c := range cases {
		anyPy = tb.Python[c.ID]
		mipFloors = append(mipFloors, tb.Python[c.ID].CBCSpawnFloorMS)
		if tb.Python[c.ID].CBCLPSpawnFloorMS > 0 {
			lpFloors = append(lpFloors, tb.Python[c.ID].CBCLPSpawnFloorMS)
		}
	}
	var anySciPy SciPyAssignEntry
	for _, ac := range BuildAssignCorpus() {
		anySciPy = tb.SciPyAssign[ac.ID]
	}
	mipFloorMed, _, _ := medianMinMax(mipFloors)
	lpFloorMed, _, _ := medianMinMax(lpFloors)
	base := fmt.Sprintf("commit `%s`", pv.commit)
	if pv.dirty {
		base = fmt.Sprintf("working tree on top of commit `%s`", pv.commit)
	}
	kernelStr := pv.unameRelease
	if kernelStr == "" {
		kernelStr = "unrecorded"
	}
	cbcHash := anyPy.CBCSha256
	if cbcHash == "" {
		cbcHash = "unrecorded"
	}
	fmt.Fprintf(b, "- **Unified telemetry source**: fingerprint `%s` (%s), `%s`, Linux kernel `%s`; the documentation gate rejects any telemetry file whose commit, fingerprint, or kernel differs.\n",
		pv.fingerprint, base, pv.goVersion, kernelStr)
	fmt.Fprintf(b, "- **CPU affinity**: goblin %s; Python + CBC %s; Python + SciPy %s.\n",
		affinityList(pv.affinity),
		affinityList(map[string]bool{anyPy.CPUAffinity: true}),
		affinityList(map[string]bool{anySciPy.CPUAffinity: true}))
	fmt.Fprintf(b, "- **Python MILP reference**: PuLP `%s`, `%s` (`%s`, SHA-256 `%s`, `ratioGap=1e-6`, `allowableGap=1e-6`).\n",
		anyPy.PuLPVersion, anyPy.CBCVersion, filepath.Base(anyPy.CBCPath), cbcHash)
	fmt.Fprintf(b, "- **Python LSAP reference**: SciPy `%s` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `%s`.\n",
		anySciPy.SciPyVersion, anySciPy.NumPyVersion)
	fmt.Fprintf(b, "- **CBC subprocess spawn floor**: %.2f ms median on a 1-variable MILP and %.2f ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).\n",
		mipFloorMed, lpFloorMed)
	fmt.Fprintf(b, "- **goblin `WithCBC()` backend**: %s\n", cbcBackendDescription)
	return nil
}

func renderAssignTierSummaryTable(b *bytes.Buffer, tb *TelemetryBundle) {
	assignCases := BuildAssignCorpus()
	type assignGroupSpec struct {
		label  string
		filter func(c AssignCorpusCase) bool
	}
	groups := []assignGroupSpec{
		{"Tier 1: Micro/Small (`N <= 32`)", func(c AssignCorpusCase) bool { return c.Tier == "tier1_small" }},
		{"Tier 2: Medium / Target p90 (`64 <= N <= 256`)", func(c AssignCorpusCase) bool { return c.Tier == "tier2_medium" }},
		{"Tier 3: Large Tail (`500 <= N <= 2000`)", func(c AssignCorpusCase) bool { return c.Tier == "tier3_large" }},
		{"Square families ($m = n$)", func(c AssignCorpusCase) bool { return c.Rows == c.Cols }},
		{"Rectangular families ($m \\neq n$, wide and tall)", func(c AssignCorpusCase) bool { return c.Rows != c.Cols }},
		{"Degenerate / structured (ties, rank-1, geometric, sparse)", func(c AssignCorpusCase) bool {
			return c.Family == "integer_ties" || c.Family == "rank1" || c.Family == "geometric" || c.Family == "sparse"
		}},
		{"**All 22 synthetic LSAP families**", func(_ AssignCorpusCase) bool { return true }},
	}

	b.WriteString("| Workload Subset | Families (Instances) | Paired `SciPy Solve / goblin` GeoMean [95% CI] | Paired `SciPy Total / goblin` GeoMean | `goblin` Solve Sum (ms) | SciPy Solve Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |\n")
	b.WriteString("| :--- | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, grp := range groups {
		var nFams int
		var spSolvePaired, goSolvePaired, spTotPaired, goTotPaired, solveSpeedups []float64
		var goSolveSum, spSolveSum float64
		for _, ac := range assignCases {
			if !grp.filter(ac) {
				continue
			}
			nFams++
			sp := tb.SciPyAssign[ac.ID]
			sc := tb.AssignScalar[ac.ID]
			nInst := min(len(sp.TSolveTrialsMS), len(sc.TSolveTrialsMS))
			for k := range nInst {
				ss := sp.TSolveTrialsMS[k]
				gs := sc.TSolveTrialsMS[k]
				spSolvePaired = append(spSolvePaired, ss)
				goSolvePaired = append(goSolvePaired, gs)
				goSolveSum += gs
				spSolveSum += ss
				if gs > 0 {
					solveSpeedups = append(solveSpeedups, ss/gs)
				}
			}
			nTot := min(len(sp.TTotalTrialsMS), len(sc.TTotalTrialsMS))
			for k := range nTot {
				spTotPaired = append(spTotPaired, sp.TTotalTrialsMS[k])
				goTotPaired = append(goTotPaired, sc.TTotalTrialsMS[k])
			}
		}
		solveGM, solveLo, solveHi, nInst := pairedRatio(spSolvePaired, goSolvePaired)
		totGM, _, _, _ := pairedRatio(spTotPaired, goTotPaired)
		medSp, minSp, maxSp := medianMinMax(solveSpeedups)
		fmt.Fprintf(b, "| %s | %d (%d) | **%.2fx** [%.2fx-%.2fx] | %.2fx | %s | %s | %.2fx [%.2fx-%.2fx] |\n",
			grp.label, nFams, nInst,
			solveGM, solveLo, solveHi, totGM,
			formatAssignMS(goSolveSum), formatAssignMS(spSolveSum),
			medSp, minSp, maxSp)
	}
}

func renderAssignPipelineTable(b *bytes.Buffer, tb *TelemetryBundle) {
	assignCases := BuildAssignCorpus()
	b.WriteString("| Problem ID | Shape ($m \\times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\\pm$ SD (ms) | SciPy Solve Mean $\\pm$ SD (ms) | Trial 0 Obj | Parity |\n")
	b.WriteString("| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |\n")
	for _, ac := range assignCases {
		sp := tb.SciPyAssign[ac.ID]
		sc := tb.AssignScalar[ac.ID]
		solveRatio, _, _, _ := pairedRatio(sp.TSolveTrialsMS, sc.TSolveTrialsMS)
		if math.IsNaN(solveRatio) {
			solveRatio = sp.TSolveMedianMS / max(sc.TSolveMS, geoMeanMinFloor)
		}
		fmt.Fprintf(b, "| `%s` | $%d \\times %d$ | `%s` | **%.2fx** | %s | %s | %s | %s | $%s \\pm %s$ | $%s \\pm %s$ | %.2f | %s |\n",
			ac.ID, ac.Rows, ac.Cols, ac.Family,
			solveRatio,
			formatAssignMS(sc.TSolveMS), formatAssignMS(sp.TSolveMedianMS),
			formatAssignMS(sc.TTotalMS), formatAssignMS(sp.TTotalMedianMS),
			formatAssignMS(sc.TSolveMeanMS), formatAssignMS(sc.TSolveSDMS),
			formatAssignMS(sp.TSolveMeanMS), formatAssignMS(sp.TSolveSDMS),
			sc.Objective,
			assignParityCell(tb, ac.ID))
	}
}

func renderAssignSIMDSummary(b *bytes.Buffer, tb *TelemetryBundle) {
	assignCases := BuildAssignCorpus()
	var controls, controlsFlagged, reachable, faster, slower int
	for _, ac := range assignCases {
		sc := tb.AssignScalar[ac.ID]
		sm := tb.AssignSIMD[ac.ID]
		size := max(sm.Rows, sm.Cols)
		_, lo, hi, n := pairedRatio(sc.TSolveTrialsMS, sm.TSolveTrialsMS)
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
	fmt.Fprintf(b, "- **Negative controls** (`max(rows, cols) < %d`, no AVX2 row-reduction kernel runs): %d of %d have a 95%% interval excluding 1.\n",
		simdReachSize, controlsFlagged, controls)
	fmt.Fprintf(b, "- **Instances where AVX2 row kernels run** (`max(rows, cols) >= %d`): %d faster, %d slower, %d with no detectable effect, out of %d.\n",
		simdReachSize, faster, slower, reachable-faster-slower, reachable)
}

func renderAssignSIMDTable(b *bytes.Buffer, tb *TelemetryBundle) {
	assignCases := BuildAssignCorpus()
	b.WriteString("| Problem ID | Shape ($m \\times n$) | `max(m, n)` | AVX2 row kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |\n")
	b.WriteString("| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |\n")
	for _, ac := range assignCases {
		sc := tb.AssignScalar[ac.ID]
		sm := tb.AssignSIMD[ac.ID]
		size := max(sm.Rows, sm.Cols)
		gm, lo, hi, n := pairedRatio(sc.TSolveTrialsMS, sm.TSolveTrialsMS)
		reach, verdict := simdVerdict(size, lo, hi, n)
		fmt.Fprintf(b, "| `%s` | $%d \\times %d$ | %d | %s | %s | %.2fx [%.2fx-%.2fx] | %s | %s | %d |\n",
			ac.ID, ac.Rows, ac.Cols, size, reach, verdict, gm, lo, hi,
			formatAssignMS(sc.TSolveMS), formatAssignMS(sm.TSolveMS), n)
	}
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
- **Default pure-Go backend**: ` + "`goblin`" + ` default (` + "`BackendGo`" + `, scalar kernels or ` + "`WithoutSIMD()`" + `). All headline ` + "`goblin (Go)`" + ` timings use that build.
- **Experimental SIMD build**: ` + "`GOEXPERIMENT=simd`" + `, which dispatches AVX2 kernels for slices of length 32 or more; see section 4.
- **Optional C++ backend**: ` + "`gb.WithCBC()`" + ` (` + "`BackendCBC`" + `, built with ` + "`-tags cbc`" + `), driving in-process COIN-OR CBC/CLP (` + "`c1c171ec`" + `) through ` + "`cgo`" + `.
- **Python baseline**: Python + ` + "`PuLP`" + ` + ` + "`COIN-OR CBC`" + ` (` + "`c1c171ec`" + `, ` + "`threads=1`" + `, ` + "`ratioGap=1e-6`" + `, ` + "`allowableGap=1e-6`" + `) subprocess. Its solve time includes MPS writing, process spawn, and ` + "`.sol`" + ` parsing.
- **Synthetic corpus and public validation**: 31 synthetic LP and MILP families across 3 tiers (` + "`testdata/corpus/*.json`" + ` and ` + "`testdata/corpus/trials/*.jsonl`" + `; 11 distinct randomized trial instances per family for Tiers 1-2 and 5 for Tier 3, 293 instances in total), measured in warm state, complemented by 17 held-out public Netlib LP and MIPLIB/COIN-OR instances verified in ` + "`public_benchmarks_test.go`" + `.

<!-- BEGIN GENERATED:PROVENANCE -->
<!-- END GENERATED:PROVENANCE -->

## 2. Aggregate Workload Tier Summary

Tier 1 micro-models are dominated by Python subprocess spawn overhead, and the planted-gap MAD family is constructed so that root heuristics close the gap at node 1; the bold row isolates solver-dominated Tier 2 and Tier 3 non-planted MILPs across paired trial instances.

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

## 3. End-to-End Pipeline Comparison (` + "`JSON -> Solution`" + `)

Pipeline stages measured per instance (median across randomized trials after warmup, plus paired per-trial speedup range):
1. **Prep (` + "`t_parse + t_build + t_format/t_compile`" + `)**: JSON unmarshaling, model construction, and MPS formatting (Python) or ` + "`Model.Compile()`" + ` matrix assembly (Go).
2. **Solve (` + "`t_solve`" + `)**: CBC subprocess execution and ` + "`.sol`" + ` parsing (Python) vs. in-memory ` + "`gb.SolveCompiled`" + ` (Go).
3. **Total (` + "`t_total`" + `)**: Complete end-to-end latency from raw JSON bytes to verified primal solution.

The Parity column is computed by ` + "`bench.CheckParity`" + `: canonical and per-trial statuses and objectives across all 293 instances for ` + "`scalar`" + `, ` + "`simd`" + `, and ` + "`cbc`" + ` must match CBC within a relative tolerance of 1e-5, and ` + "`ValidateTelemetryAgainstCorpus`" + ` rejects the telemetry otherwise.

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

## 4. Scalar vs. ` + "`GOEXPERIMENT=simd`" + ` Build

Ratios are geometric means of per-trial ` + "`scalar / SIMD`" + ` solve times over the same instances, with 95% t-intervals on the log ratios. "Reachable" is a necessary condition only: the AVX2 ` + "`Dot64`" + `/` + "`Axpy64`" + `/` + "`Scale64`" + ` paths need a slice of length at least 32, which requires ` + "`n + m >= 32`" + `.

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

## 6. Linear Sum Assignment (` + "`goblin/assign`" + ` vs. ` + "`scipy.optimize.linear_sum_assignment`" + `)

The 22-family synthetic Linear Sum Assignment corpus (` + "`testdata/assign_corpus/manifest.json`" + `; 11 randomized trials per family for Tiers 1-2 and 5 for Tier 3, 200 instances in total) compares pure-Go ` + "`assign.LinearSumFlat`" + ` against SciPy's C++ ` + "`rectangular_lsap`" + ` extension in warm state on bit-identical ` + "`float64`" + ` cost matrices, timing ` + "`linear_sum_assignment`" + ` without post-solve NumPy array gather overhead.

### 6.1 Aggregate LSAP Tier Summary

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 6.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 6.3 LSAP Scalar vs. ` + "`GOEXPERIMENT=simd`" + ` (` + "`AllAbsLE64`" + `, ` + "`MinReducedSlice64`" + `, ` + "`InitReducedRow64`" + `, and ` + "`Min2ReducedRow64`" + `)

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->
`
	return SyncMarkdownContent(tpl, tb)
}
