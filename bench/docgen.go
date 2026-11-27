package bench

import (
	"bytes"
	"encoding/json/v2"
	"errors"
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/paulourio/goblin/assign"
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
	PairedABBA        bool              `json:"paired_ab_ba,omitzero"`
	SIMDTier          string            `json:"simd_tier,omitempty"`
	CPUModel          string            `json:"cpu_model,omitempty"`
	GOAMD64           string            `json:"goamd64,omitempty"`
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
	RootLPStatus      string            `json:"root_lp_status,omitempty"`
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
	PairedABBA        bool      `json:"paired_ab_ba,omitzero"`
	SIMDTier          string    `json:"simd_tier,omitempty"`
	CPUModel          string    `json:"cpu_model,omitempty"`
	GOAMD64           string    `json:"goamd64,omitempty"`
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

// CanonicalBinarySizeIDs lists the four required binary-size and linker-DCE profiles in table order.
var CanonicalBinarySizeIDs = []string{
	"assign_default",
	"assign_certified",
	"goblin_default",
	"goblin_certified_vipr",
}

// BinarySizeEntry records compiled binary sizes and symbol-table DCE counts for one build profile.
type BinarySizeEntry struct {
	ID                string `json:"id"`
	Label             string `json:"label"`
	Description       string `json:"description"`
	RawBytes          int64  `json:"raw_bytes"`
	StrippedBytes     int64  `json:"stripped_bytes"`
	SIMDRawBytes      int64  `json:"simd_raw_bytes"`
	SIMDStrippedBytes int64  `json:"simd_stripped_bytes"`
	GoblinTextBytes   int64  `json:"goblin_text_bytes"`
	MathBigSyms       int    `json:"math_big_syms"`
	MathBigBytes      int64  `json:"math_big_bytes"`
	EncodingJSONSyms  int    `json:"encoding_json_syms"`
	EncodingJSONBytes int64  `json:"encoding_json_bytes"`
	VIPRSyms          int    `json:"vipr_syms"`
	VIPRBytes         int64  `json:"vipr_bytes"`
	CertSyms          int    `json:"cert_syms"`
	CertBytes         int64  `json:"cert_bytes"`
	GitCommit         string `json:"git_commit,omitempty"`
	GitDirty          bool   `json:"git_dirty,omitzero"`
	DefaultGoVersion  string `json:"default_go_version,omitempty"`
	SIMDGoVersion     string `json:"simd_go_version,omitempty"`
	GOOS              string `json:"goos,omitempty"`
	GOARCH            string `json:"goarch,omitempty"`
	GOAMD64           string `json:"goamd64,omitempty"`
	SourceFingerprint string `json:"source_fingerprint,omitempty"`
	UnameRelease      string `json:"uname_release,omitempty"`
	MeasuredAt        string `json:"measured_at,omitempty"`
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
	BinarySize   map[string]BinarySizeEntry
	MAD          map[string]MADBenchResult
}

// LoadTelemetryBundle reads the Python, pure-Go scalar, and pure-Go SIMD telemetry files plus the
// in-process CBC backend telemetry (goblin_cbc.json), the LSAP telemetry files
// (scipy_assign_baseline.json, goblin_assign_scalar.json, goblin_assign_simd.json), the
// binary-size and linker-DCE telemetry (goblin_binary_size.json), and the isolated combinatorial
// MAD telemetry (goblin_mad.json) stored next to the scalar file.
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
	var binSizeMap map[string]BinarySizeEntry
	if err := readJSON(filepath.Join(resultsDir, "goblin_binary_size.json"), &binSizeMap); err != nil {
		return nil, fmt.Errorf("binary size telemetry: %w", err)
	}
	var madMap map[string]MADBenchResult
	if err := readJSON(filepath.Join(resultsDir, "goblin_mad.json"), &madMap); err != nil {
		return nil, fmt.Errorf("mad telemetry: %w", err)
	}
	return &TelemetryBundle{
		Python:       pyMap,
		Goblin:       goblinMap,
		Scalar:       scalarMap,
		SIMD:         simdMap,
		SciPyAssign:  scipyAssignMap,
		AssignScalar: assignScalarMap,
		AssignSIMD:   assignSIMDMap,
		BinarySize:   binSizeMap,
		MAD:          madMap,
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

func isZeroPair(syms int, byteCount int64) bool { return syms == 0 && byteCount == 0 }
func isPosPair(syms int, byteCount int64) bool  { return syms > 0 && byteCount > 0 }

// ValidateBinarySizeTelemetry checks that all six canonical binary-size profiles are present with
// positive byte counts and satisfy the linker dead-code-elimination zero-leakage invariants.
func ValidateBinarySizeTelemetry(binMap map[string]BinarySizeEntry) error {
	if len(binMap) != len(CanonicalBinarySizeIDs) {
		return fmt.Errorf("binary size telemetry has %d entries, want %d", len(binMap), len(CanonicalBinarySizeIDs))
	}
	for _, id := range CanonicalBinarySizeIDs {
		bs, ok := binMap[id]
		if !ok {
			return fmt.Errorf("missing %q in binary size telemetry", id)
		}
		if bs.ID != id {
			return fmt.Errorf("binary size entry %q has mismatched id %q", id, bs.ID)
		}
		if bs.Label == "" {
			return fmt.Errorf("binary size entry %q has empty label", id)
		}
		if bs.RawBytes <= 0 || bs.StrippedBytes <= 0 || bs.StrippedBytes > bs.RawBytes {
			return fmt.Errorf("binary size entry %q has invalid default sizes raw=%d stripped=%d", id, bs.RawBytes, bs.StrippedBytes)
		}
		if bs.SIMDRawBytes <= 0 || bs.SIMDStrippedBytes <= 0 || bs.SIMDStrippedBytes > bs.SIMDRawBytes {
			return fmt.Errorf("binary size entry %q has invalid SIMD sizes raw=%d stripped=%d", id, bs.SIMDRawBytes, bs.SIMDStrippedBytes)
		}
		if bs.GoblinTextBytes <= 0 {
			return fmt.Errorf("binary size entry %q has non-positive goblin_text_bytes=%d", id, bs.GoblinTextBytes)
		}
		switch id {
		case "assign_default":
			if !isZeroPair(bs.MathBigSyms, bs.MathBigBytes) || !isZeroPair(bs.EncodingJSONSyms, bs.EncodingJSONBytes) ||
				!isZeroPair(bs.VIPRSyms, bs.VIPRBytes) || !isZeroPair(bs.CertSyms, bs.CertBytes) {
				return fmt.Errorf("assign_default linker DCE violation: math/big=%d(%dB) json=%d(%dB) vipr=%d(%dB) cert=%d(%dB), want all 0",
					bs.MathBigSyms, bs.MathBigBytes, bs.EncodingJSONSyms, bs.EncodingJSONBytes, bs.VIPRSyms, bs.VIPRBytes, bs.CertSyms, bs.CertBytes)
			}
		case "assign_certified":
			if !isZeroPair(bs.MathBigSyms, bs.MathBigBytes) || !isZeroPair(bs.EncodingJSONSyms, bs.EncodingJSONBytes) ||
				!isZeroPair(bs.VIPRSyms, bs.VIPRBytes) || !isPosPair(bs.CertSyms, bs.CertBytes) {
				return fmt.Errorf("assign_certified symbol invariant failed: math/big=%d(%dB) json=%d(%dB) vipr=%d(%dB) cert=%d(%dB), want 0/0/0 and cert>0",
					bs.MathBigSyms, bs.MathBigBytes, bs.EncodingJSONSyms, bs.EncodingJSONBytes, bs.VIPRSyms, bs.VIPRBytes, bs.CertSyms, bs.CertBytes)
			}
		case "goblin_default":
			if !isZeroPair(bs.EncodingJSONSyms, bs.EncodingJSONBytes) ||
				!isZeroPair(bs.VIPRSyms, bs.VIPRBytes) || !isZeroPair(bs.CertSyms, bs.CertBytes) {
				return fmt.Errorf("goblin_default linker DCE violation: json=%d(%dB) vipr=%d(%dB) cert=%d(%dB), want all 0",
					bs.EncodingJSONSyms, bs.EncodingJSONBytes, bs.VIPRSyms, bs.VIPRBytes, bs.CertSyms, bs.CertBytes)
			}
		case "goblin_certified_vipr":
			if !isZeroPair(bs.EncodingJSONSyms, bs.EncodingJSONBytes) ||
				!isPosPair(bs.VIPRSyms, bs.VIPRBytes) || !isPosPair(bs.CertSyms, bs.CertBytes) {
				return fmt.Errorf("goblin_certified_vipr symbol invariant failed: json=%d(%dB) vipr=%d(%dB) cert=%d(%dB), want json=0 and vipr,cert>0",
					bs.EncodingJSONSyms, bs.EncodingJSONBytes, bs.VIPRSyms, bs.VIPRBytes, bs.CertSyms, bs.CertBytes)
			}
		}
	}
	return nil
}

// ValidateTelemetryAgainstCorpus verifies that every problem in BuildCorpus(), AssignCorpusSpecs(),
// and MADCorpusSpecs() exists in testdata/ and in all telemetry maps with matching dimensions,
// that all nine telemetry files carry uniform provenance and linker-DCE invariants, and that every
// goblin build agrees with its reference on status, canonical objective, and per-trial objectives.
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

	milpMaps := []struct {
		label string
		m     map[string]ProblemBenchResult
	}{
		{"scalar", tb.Scalar},
		{"simd", tb.SIMD},
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
		for _, item := range milpMaps {
			r, ok := item.m[c.ID]
			if !ok {
				return fmt.Errorf("missing %q in %s telemetry", c.ID, item.label)
			}
			if r.NumVars != compiled.NumVars() || r.NumConstraints != compiled.NumConstraints() {
				return fmt.Errorf("%s telemetry dimension mismatch for %q: got (%d, %d), want (%d, %d)",
					item.label, c.ID, r.NumVars, r.NumConstraints, compiled.NumVars(), compiled.NumConstraints())
			}
		}
		mismatches = append(mismatches, CheckParity(tb, c.ID)...)
	}

	assignCases := AssignCorpusSpecs()
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

	assignMaps := []struct {
		label string
		m     map[string]AssignBenchResult
	}{
		{"assign_scalar", tb.AssignScalar},
		{"assign_simd", tb.AssignSIMD},
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
		for _, item := range assignMaps {
			r, ok := item.m[ac.ID]
			if !ok {
				return fmt.Errorf("missing %q in %s telemetry", ac.ID, item.label)
			}
			if r.Rows != ac.Rows || r.Cols != ac.Cols {
				return fmt.Errorf("%s telemetry dimension mismatch for %q: got (%d, %d), want (%d, %d)",
					item.label, ac.ID, r.Rows, r.Cols, ac.Rows, ac.Cols)
			}
		}
		mismatches = append(mismatches, CheckAssignParity(tb, ac.ID)...)
	}

	madCases := MADCorpusSpecs()
	madManifest := filepath.Join(filepath.Dir(corpusDir), "mad_corpus", "manifest.json")
	if _, err := os.Stat(madManifest); err != nil {
		return fmt.Errorf("missing mad corpus manifest %s: %w", madManifest, err)
	}
	if len(tb.MAD) != len(madCases) {
		return fmt.Errorf("mad telemetry has %d entries, mad corpus has %d", len(tb.MAD), len(madCases))
	}
	for _, mc := range madCases {
		mr, ok := tb.MAD[mc.ID]
		if !ok {
			return fmt.Errorf("missing %q in mad telemetry", mc.ID)
		}
		if mr.Groups != mc.Groups || mr.Items != mc.Items || mr.Periods != mc.Periods || mr.Trials != mc.Trials {
			return fmt.Errorf("mad telemetry dimension mismatch for %q: got (%d,%d,%d,trials=%d), want (%d,%d,%d,trials=%d)",
				mc.ID, mr.Groups, mr.Items, mr.Periods, mr.Trials, mc.Groups, mc.Items, mc.Periods, mc.Trials)
		}
		mismatches = append(mismatches, CheckMADParity(tb, mc.ID)...)
	}

	if len(mismatches) > 0 {
		return fmt.Errorf("parity check failed (%d mismatches):\n  %s", len(mismatches), strings.Join(mismatches, "\n  "))
	}
	if err := ValidateBinarySizeTelemetry(tb.BinarySize); err != nil {
		return err
	}
	if _, err := telemetryProvenance(tb); err != nil {
		return err
	}
	return nil
}

// CheckMADParity validates combinatorial parity and 250 ms-capped MILP [BestBound, Incumbent]
// containment for one combinatorial MAD corpus family.
func CheckMADParity(tb *TelemetryBundle, id string) []string {
	mr, ok := tb.MAD[id]
	if !ok {
		return []string{id + ": missing mad telemetry"}
	}
	var out []string
	if !mr.ParityOK {
		out = append(out, id+": parity_ok=false")
	}
	if len(mr.TrialObjectives) != mr.Trials || mr.Trials <= 0 {
		out = append(out, fmt.Sprintf("%s: trial_objectives count %d != trials %d", id, len(mr.TrialObjectives), mr.Trials))
	} else if relDiff(mr.Objective, mr.TrialObjectives[0]) > assignParityRelTol {
		out = append(out, fmt.Sprintf("%s: objective %.10g != trial0 %.10g", id, mr.Objective, mr.TrialObjectives[0]))
	}
	checkMILP := func(label string, m *MADMILPMetrics) {
		if m == nil {
			return
		}
		if m.Status != "Optimal" && m.Status != "TimeLimit" {
			out = append(out, fmt.Sprintf("%s/%s: unexpected status %q", id, label, m.Status))
		}
		tol := parityRelTol * max(1.0, math.Abs(mr.Objective))
		if m.HasBestBound && m.BestBound > mr.Objective+tol {
			out = append(out, fmt.Sprintf("%s/%s: best_bound %.8g > exact %.8g", id, label, m.BestBound, mr.Objective))
		}
		if m.HasIncumbent {
			if m.Objective == nil {
				out = append(out, fmt.Sprintf("%s/%s: has_incumbent=true with nil objective", id, label))
			} else {
				if *m.Objective < mr.Objective-tol {
					out = append(out, fmt.Sprintf("%s/%s: incumbent %.8g < exact %.8g", id, label, *m.Objective, mr.Objective))
				}
				if m.HasBestBound && m.BestBound > *m.Objective+tol {
					out = append(out, fmt.Sprintf("%s/%s: best_bound %.8g > incumbent %.8g", id, label, m.BestBound, *m.Objective))
				}
				if m.Status == "Optimal" && relDiff(*m.Objective, mr.Objective) > parityRelTol {
					out = append(out, fmt.Sprintf("%s/%s: optimal objective %.8g != exact %.8g", id, label, *m.Objective, mr.Objective))
				}
			}
		} else if m.Status == "Optimal" {
			out = append(out, fmt.Sprintf("%s/%s: Optimal status without incumbent", id, label))
		}
	}
	checkMILP("milp_go", &mr.MILPGo)
	checkMILP("milp_cbc", mr.MILPCBC)
	return out
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
		if py.LPRelaxationObjective != nil {
			if b.res.RootLPStatus != "Optimal" {
				out = append(out, fmt.Sprintf("%s/%s: root LP status %s, want Optimal", id, b.label, b.res.RootLPStatus))
			} else if relDiff(b.res.RootLPObj, *py.LPRelaxationObjective) > parityRelTol {
				out = append(out, fmt.Sprintf("%s/%s: root LP objective %.8g, CBC %.8g", id, b.label, b.res.RootLPObj, *py.LPRelaxationObjective))
			}
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
	zCrit975           = 1.959963984540054
	minPairedSamples   = 2
	syncedDocPerm      = 0o644
)

func relDiff(a, b float64) float64 { return math.Abs(a-b) / max(1.0, math.Abs(b)) }

// provenance summarizes the code version and host kernel that produced all benchmark telemetry.
type provenance struct {
	commit           string
	dirty            bool
	defaultGoVersion string
	simdGoVersion    string
	fingerprint      string
	unameRelease     string
	affinity         map[string]bool
	goblinAffinity   map[string]bool
}

func goVersionHasSIMD(v string) bool {
	tok := v
	if fields := strings.Fields(v); len(fields) > 0 {
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

// telemetryProvenance requires every Python, SciPy, scalar, SIMD, CBC, LSAP, binary-size, and
// isolated MAD entry to carry the same git commit, source fingerprint, host kernel release, and
// matching default vs. SIMD toolchain.
func telemetryProvenance(tb *TelemetryBundle) (provenance, error) {
	pv := provenance{
		affinity:       map[string]bool{},
		goblinAffinity: map[string]bool{},
	}
	checkRec := func(sourceLabel, id, commit, fp, uname, aff string, dirty, isGoblin bool) error {
		if commit == "" || fp == "" {
			return fmt.Errorf("%s entry %q lacks git_commit/source_fingerprint provenance (re-run benchmark suite)", sourceLabel, id)
		}
		if pv.commit == "" {
			pv.commit = commit
			pv.fingerprint = fp
			pv.unameRelease = uname
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
			if isGoblin {
				pv.goblinAffinity[aff] = true
			}
		}
		return nil
	}
	checkGoBuild := func(sourceLabel, id, goVer string, simdEnabled, pairedABBA, expectSIMD bool) error {
		if goVer == "" {
			return fmt.Errorf("%s entry %q lacks go_version provenance (re-run benchmark suite)", sourceLabel, id)
		}
		hasSIMDExp := goVersionHasSIMD(goVer)
		if expectSIMD {
			if !hasSIMDExp || !simdEnabled {
				return fmt.Errorf("%s entry %q requires GOEXPERIMENT=simd and simd_enabled=true (got go_version=%q simd_enabled=%v)",
					sourceLabel, id, goVer, simdEnabled)
			}
			if pv.simdGoVersion == "" {
				pv.simdGoVersion = goVer
			} else if goVer != pv.simdGoVersion {
				return fmt.Errorf("%s mixes SIMD Go versions (%s vs %s, entry %q)",
					sourceLabel, pv.simdGoVersion, goVer, id)
			}
			return nil
		}
		if simdEnabled {
			return fmt.Errorf("%s entry %q requires simd_enabled=false (got go_version=%q simd_enabled=%v)",
				sourceLabel, id, goVer, simdEnabled)
		}
		if hasSIMDExp {
			if !pairedABBA {
				return fmt.Errorf("%s entry %q requires non-SIMD default build or paired_ab_ba=true (got go_version=%q simd_enabled=%v)",
					sourceLabel, id, goVer, simdEnabled)
			}
			if pv.simdGoVersion == "" {
				pv.simdGoVersion = goVer
			} else if goVer != pv.simdGoVersion {
				return fmt.Errorf("%s mixes SIMD Go versions (%s vs %s, entry %q)",
					sourceLabel, pv.simdGoVersion, goVer, id)
			}
			return nil
		}
		if pairedABBA {
			return fmt.Errorf("%s entry %q sets paired_ab_ba=true without GOEXPERIMENT=simd go_version=%q",
				sourceLabel, id, goVer)
		}
		if pv.defaultGoVersion == "" {
			pv.defaultGoVersion = goVer
		} else if goVer != pv.defaultGoVersion {
			return fmt.Errorf("%s mixes default Go versions (%s vs %s, entry %q)",
				sourceLabel, pv.defaultGoVersion, goVer, id)
		}
		return nil
	}

	for id, py := range tb.Python {
		if err := checkRec("python_baseline", id, py.GitCommit, py.SourceFingerprint, py.UnameRelease, py.CPUAffinity, py.GitDirty, false); err != nil {
			return pv, err
		}
	}
	for id, sp := range tb.SciPyAssign {
		if err := checkRec("scipy_assign_baseline", id, sp.GitCommit, sp.SourceFingerprint, sp.UnameRelease, sp.CPUAffinity, sp.GitDirty, false); err != nil {
			return pv, err
		}
	}

	maps := []struct {
		label       string
		expectSIMD  bool
		allowPaired bool
		m           map[string]ProblemBenchResult
	}{
		{"goblin_scalar", false, true, tb.Scalar},
		{"goblin_simd", true, false, tb.SIMD},
	}
	if len(tb.Goblin) > 0 {
		maps = append(maps, struct {
			label       string
			expectSIMD  bool
			allowPaired bool
			m           map[string]ProblemBenchResult
		}{"goblin_cbc", false, false, tb.Goblin})
	}
	for _, item := range maps {
		for id, r := range item.m {
			if err := checkRec(item.label, id, r.GitCommit, r.SourceFingerprint, r.UnameRelease, r.CPUAffinity, r.GitDirty, true); err != nil {
				return pv, err
			}
			if err := checkGoBuild(item.label, id, r.GoVersion, r.SIMDEnabled, item.allowPaired && r.PairedABBA, item.expectSIMD); err != nil {
				return pv, err
			}
		}
	}
	for _, item := range []struct {
		label       string
		expectSIMD  bool
		allowPaired bool
		m           map[string]AssignBenchResult
	}{
		{"goblin_assign_scalar", false, true, tb.AssignScalar},
		{"goblin_assign_simd", true, false, tb.AssignSIMD},
	} {
		for id, r := range item.m {
			if err := checkRec(item.label, id, r.GitCommit, r.SourceFingerprint, r.UnameRelease, r.CPUAffinity, r.GitDirty, true); err != nil {
				return pv, err
			}
			if err := checkGoBuild(item.label, id, r.GoVersion, r.SIMDEnabled, item.allowPaired && r.PairedABBA, item.expectSIMD); err != nil {
				return pv, err
			}
		}
	}
	for id, bs := range tb.BinarySize {
		if err := checkRec("goblin_binary_size", id, bs.GitCommit, bs.SourceFingerprint, bs.UnameRelease, "", bs.GitDirty, false); err != nil {
			return pv, err
		}
		if err := checkGoBuild("goblin_binary_size(default)", id, bs.DefaultGoVersion, false, false, false); err != nil {
			return pv, err
		}
		if err := checkGoBuild("goblin_binary_size(simd)", id, bs.SIMDGoVersion, true, false, true); err != nil {
			return pv, err
		}
	}
	for id, mr := range tb.MAD {
		if err := checkRec("goblin_mad", id, mr.GitCommit, mr.SourceFingerprint, mr.UnameRelease, mr.CPUAffinity, mr.GitDirty, true); err != nil {
			return pv, err
		}
		if err := checkGoBuild("goblin_mad", id, mr.GoVersion, mr.SIMDEnabled, false, false); err != nil {
			return pv, err
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

var tCrit975Table = [...]float64{
	0, 12.706, 4.303, 3.182, 2.776, 2.571, 2.447, 2.365, 2.306, 2.262, 2.228,
	2.201, 2.179, 2.160, 2.145, 2.131, 2.120, 2.110, 2.101, 2.093, 2.086,
}

// tCrit975 returns the two-sided 95% Student-t critical value for df degrees of freedom,
// using exact tabulated values for df <= 20 and a 4-term Cornish-Fisher expansion for df >= 21.
func tCrit975(df int) float64 {
	if df <= 0 {
		return math.Inf(1)
	}
	if df < len(tCrit975Table) {
		return tCrit975Table[df]
	}
	z := zCrit975
	z2 := z * z
	z3 := z2 * z
	z5 := z3 * z2
	z7 := z5 * z2
	z9 := z7 * z2
	inv := 1.0 / float64(df)
	inv2 := inv * inv
	inv3 := inv2 * inv
	inv4 := inv2 * inv2
	g1 := (z3 + z) / 4.0
	g2 := (5.0*z5 + 16.0*z3 + 3.0*z) / 96.0
	g3 := (3.0*z7 + 19.0*z5 + 17.0*z3 - 15.0*z) / 384.0
	g4 := (79.0*z9 + 776.0*z7 + 1482.0*z5 - 1920.0*z3 - 945.0*z) / 92160.0
	return z + g1*inv + g2*inv2 + g3*inv3 + g4*inv4
}

func logMeanTCI(logs []float64) (gm, lo, hi float64, n int) {
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

// familyClusteredRatio returns the equal-family geometric mean over per-family geometric means
// and its two-sided 95% Student-t_{F-1} confidence interval on family log-means.
func familyClusteredRatio(famGMs []float64) (gm, lo, hi float64) {
	var logs []float64
	for _, g := range famGMs {
		if g > 0 && !math.IsNaN(g) && !math.IsInf(g, 0) {
			logs = append(logs, math.Log(g))
		}
	}
	gm, lo, hi, _ = logMeanTCI(logs)
	return gm, lo, hi
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
	return logMeanTCI(logs)
}

func pairedRatioOrFallback(aTrials, bTrials []float64, aMed, bMed float64) float64 {
	r, _, _, _ := pairedRatio(aTrials, bTrials)
	if math.IsNaN(r) {
		return aMed / max(bMed, geoMeanMinFloor)
	}
	return r
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

// simdVerdict classifies one paired scalar/SIMD comparison. Unreachable instances
// are negative controls: an interval excluding 1 there is noise, never a SIMD effect.
func simdVerdict(reachable bool, lo, hi float64, n int) (reach, verdict string) {
	if !reachable {
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

type simdTally struct {
	controls        int
	controlsFlagged int
	reachable       int
	faster          int
	slower          int
}

func (t *simdTally) observe(reachable bool, scalarTrials, simdTrials []float64) {
	_, lo, hi, n := pairedRatio(scalarTrials, simdTrials)
	if n < minPairedSamples {
		return
	}
	if !reachable {
		t.controls++
		if lo > 1 || hi < 1 {
			t.controlsFlagged++
		}
		return
	}
	t.reachable++
	switch {
	case lo > 1:
		t.faster++
	case hi < 1:
		t.slower++
	}
}

type subsetSpec[T any] struct {
	label  string
	filter func(T) bool
}

type docgenCorpus struct {
	cases         []CorpusCase
	assignCases   []AssignCorpusCase
	madCases      []MADCorpusCase
	repoRoot      string
	formalInv     *FormalInventory
	targetSilicon *TargetSiliconBundle
}

func (dc *docgenCorpus) milp() []CorpusCase {
	if dc.cases == nil {
		dc.cases = BuildCorpus()
	}
	return dc.cases
}

func (dc *docgenCorpus) assign() []AssignCorpusCase {
	if dc.assignCases == nil {
		dc.assignCases = AssignCorpusSpecs()
	}
	return dc.assignCases
}

func (dc *docgenCorpus) mad() []MADCorpusCase {
	if dc.madCases == nil {
		dc.madCases = MADCorpusSpecs()
	}
	return dc.madCases
}

func (dc *docgenCorpus) formal() (*FormalInventory, error) {
	if dc.formalInv != nil {
		return dc.formalInv, nil
	}
	root := dc.repoRoot
	if root == "" {
		found, err := findRepoRoot(".")
		if err != nil {
			return nil, err
		}
		root = found
	}
	inv, err := CollectFormalInventory(root)
	if err != nil {
		return nil, err
	}
	dc.formalInv = inv
	return inv, nil
}

func (dc *docgenCorpus) loadTargetSilicon() (*TargetSiliconBundle, error) {
	if dc.targetSilicon != nil {
		return dc.targetSilicon, nil
	}
	root := dc.repoRoot
	if root == "" {
		found, err := findRepoRoot(".")
		if err != nil {
			return nil, err
		}
		root = found
	}
	ts, err := LoadTargetSiliconBundle(filepath.Join(root, "bench", "results", "target_silicon"))
	if err != nil {
		return nil, err
	}
	dc.targetSilicon = ts
	return ts, nil
}

// cbcBackendDescription states which CBC build and flag mapping produced goblin_cbc.json.
const cbcBackendDescription = "COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). " +
	"Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides."

// RenderBlock produces the markdown snippet for a named generated block marker.
//
// All headline timings use the default pure-Go scalar backend (`goblin` default). The
// GOEXPERIMENT=simd build is compared to it only in SIMD_EFFECT_TABLE, with paired statistics.
func RenderBlock(blockName string, tb *TelemetryBundle) (string, error) {
	return renderBlockWith(blockName, tb, &docgenCorpus{})
}

func renderBlockWith(blockName string, tb *TelemetryBundle, dc *docgenCorpus) (string, error) {
	var b bytes.Buffer

	switch blockName {
	case "TIER_SUMMARY_TABLE":
		renderTierSummaryTable(&b, dc.milp(), tb)
	case "CBC_SUMMARY_TABLE":
		if err := renderCBCSummaryTable(&b, dc.milp(), tb); err != nil {
			return "", err
		}
	case "README_MILP_SUMMARY_TABLE":
		renderReadmeMILPSummaryTable(&b, dc.milp(), tb)
	case "README_CBC_HEADLINE":
		if err := renderReadmeCBCHeadline(&b, dc.milp(), tb); err != nil {
			return "", err
		}
	case "SIMD_EFFECT_TABLE":
		renderSIMDEffectTable(&b, dc.milp(), tb)
	case "SIMD_SUMMARY":
		renderSIMDSummary(&b, dc.milp(), tb)
	case "MAD_PIPELINE_TABLE":
		renderMADPipelineTable(&b, dc.milp(), tb)
	case "README_MAD_SUMMARY_TABLE":
		renderReadmeMADSummaryTable(&b, dc.milp(), tb)
	case "README_MAD_SOLVER_SUMMARY_TABLE":
		renderReadmeMADSolverSummaryTable(&b, dc.mad(), tb)
	case "MAD_COMBINATORIAL_SUMMARY_TABLE":
		renderMADCombinatorialSummaryTable(&b, dc.mad(), tb)
	case "MAD_COMBINATORIAL_PIPELINE_TABLE":
		renderMADCombinatorialPipelineTable(&b, dc.mad(), tb)
	case "MAD_CBC_GAP_TABLE":
		renderMADCBCGapTable(&b, dc.milp(), tb)
	case "NON_MAD_CBC_GAP_TABLE":
		renderNonMADCBCGapTable(&b, tb)
	case "NON_MAD_CBC_NOTES":
		if err := renderNonMADCBCNotes(&b, tb); err != nil {
			return "", err
		}
	case "MAD_STATS_TABLE":
		renderMADStatsTable(&b, dc.milp(), tb)
	case "MAD_CHECKPOINT_TABLE":
		renderCheckpointTable(&b, dc.milp(), tb, true)
	case "FULL_CHECKPOINT_TABLE":
		renderCheckpointTable(&b, dc.milp(), tb, false)
	case "FULL_PIPELINE_TABLE":
		renderFullPipelineTable(&b, dc.milp(), tb)
	case "PROVENANCE":
		if err := renderProvenance(&b, dc.milp(), dc.assign(), tb); err != nil {
			return "", err
		}
	case "ASSIGN_TIER_SUMMARY_TABLE":
		renderAssignTierSummaryTable(&b, dc.assign(), tb)
	case "README_ASSIGN_SUMMARY_TABLE":
		renderReadmeAssignSummaryTable(&b, dc.assign(), tb)
	case "ASSIGN_PIPELINE_TABLE":
		renderAssignPipelineTable(&b, dc.assign(), tb)
	case "ASSIGN_SIMD_SUMMARY":
		renderAssignSIMDSummary(&b, dc.assign(), tb)
	case "ASSIGN_SIMD_TABLE":
		renderAssignSIMDTable(&b, dc.assign(), tb)
	case "BINARY_SIZE_TABLE":
		if err := renderBinarySizeTable(&b, tb); err != nil {
			return "", err
		}
	case "TARGET_SILICON_SUMMARY":
		ts, err := dc.loadTargetSilicon()
		if err != nil {
			return "", err
		}
		if err := renderTargetSiliconSummary(&b, dc.milp(), dc.assign(), ts); err != nil {
			return "", err
		}
	case "README_FORMAL_SUMMARY",
		"INDEX_FORMAL_SUMMARY",
		"BENCHMARKS_FORMAL_SUMMARY",
		"FORMAL_OVERVIEW_STATS",
		"FORMAL_GO_STATS",
		"FORMAL_MODULE_MATRIX",
		"FORMAL_FUZZ_INVENTORY":
		inv, err := dc.formal()
		if err != nil {
			return "", err
		}
		return renderFormalBlock(blockName, inv)
	default:
		return "", fmt.Errorf("unknown generated block %q", blockName)
	}

	return strings.TrimRight(b.String(), "\n"), nil
}

type tierSummaryStats struct {
	nFams      int
	nInst      int
	famGM      float64
	famLo      float64
	famHi      float64
	pooledGM   float64
	goSum      float64
	goMedSum   float64
	pySum      float64
	pyMedSum   float64
	medSp      float64
	minSp      float64
	maxSp      float64
	cbcRatio   string
	cbcSumText string
}

func summarizeTierSubset(cases []CorpusCase, filter func(CorpusCase) bool, tb *TelemetryBundle) tierSummaryStats {
	var st tierSummaryStats
	var famPyGo, famGoCBC, pyPaired, goPaired, instSpeedups []float64
	var cbcSum, cbcMedSum float64
	hasAllCBC := true
	for _, c := range cases {
		if !filter(c) {
			continue
		}
		st.nFams++
		py := tb.Python[c.ID]
		sc := tb.Scalar[c.ID]
		gbRes, hasCBC := tb.Goblin[c.ID]
		if !hasCBC || len(gbRes.TTotalTrialsMS) == 0 {
			hasAllCBC = false
		}
		st.pyMedSum += py.TTotalMedianMS
		st.goMedSum += sc.TTotalMS
		if hasCBC {
			cbcMedSum += gbRes.TTotalMS
		}
		nInst := min(len(py.TTotalTrialsMS), len(sc.TTotalTrialsMS))
		if nInst == 0 {
			pyPaired = append(pyPaired, py.TTotalMedianMS)
			goPaired = append(goPaired, sc.TTotalMS)
			st.pySum += py.TTotalMedianMS
			st.goSum += sc.TTotalMS
			if sc.TTotalMS > 0 {
				r := py.TTotalMedianMS / sc.TTotalMS
				instSpeedups = append(instSpeedups, r)
				famPyGo = append(famPyGo, r)
			}
			if hasCBC {
				cbcSum += gbRes.TTotalMS
				if gbRes.TTotalMS > 0 {
					famGoCBC = append(famGoCBC, sc.TTotalMS/gbRes.TTotalMS)
				}
			}
			continue
		}
		if fgm, _, _, _ := pairedRatio(py.TTotalTrialsMS[:nInst], sc.TTotalTrialsMS[:nInst]); !math.IsNaN(fgm) {
			famPyGo = append(famPyGo, fgm)
		}
		if hasCBC && len(gbRes.TTotalTrialsMS) >= nInst {
			if cgm, _, _, _ := pairedRatio(sc.TTotalTrialsMS[:nInst], gbRes.TTotalTrialsMS[:nInst]); !math.IsNaN(cgm) {
				famGoCBC = append(famGoCBC, cgm)
			}
		}
		for k := range nInst {
			pt := py.TTotalTrialsMS[k]
			gt := sc.TTotalTrialsMS[k]
			pyPaired = append(pyPaired, pt)
			goPaired = append(goPaired, gt)
			st.pySum += pt
			st.goSum += gt
			if gt > 0 {
				instSpeedups = append(instSpeedups, pt/gt)
			}
			if hasCBC && k < len(gbRes.TTotalTrialsMS) {
				cbcSum += gbRes.TTotalTrialsMS[k]
			}
		}
	}
	st.famGM, st.famLo, st.famHi = familyClusteredRatio(famPyGo)
	st.pooledGM, _, _, st.nInst = pairedRatio(pyPaired, goPaired)
	st.medSp, st.minSp, st.maxSp = medianMinMax(instSpeedups)
	st.cbcRatio, st.cbcSumText = "N/A", "N/A"
	if hasAllCBC && len(famGoCBC) == st.nFams && len(famGoCBC) > 0 {
		cbcGM, _, _ := familyClusteredRatio(famGoCBC)
		st.cbcRatio = fmt.Sprintf("%.2fx", cbcGM)
		st.cbcSumText = fmt.Sprintf("%.1f (%.1f)", cbcSum, cbcMedSum)
	}
	return st
}

func renderTierSummaryTable(b *bytes.Buffer, cases []CorpusCase, tb *TelemetryBundle) {
	groups := []subsetSpec[CorpusCase]{
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

	b.WriteString("| Workload Subset | Families (Trials) | Equal-Family `Py / Go` GeoMean [Clustered 95% CI] | Pooled Trial `Py / Go` | Equal-Family `Go / CBC` | `Go` Trial (Med) Sum (ms) | `CBC` Trial (Med) Sum (ms) | `Py` Trial (Med) Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |\n")
	b.WriteString("| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, grp := range groups {
		st := summarizeTierSubset(cases, grp.filter, tb)
		fmt.Fprintf(b, "| %s | %d (%d) | **%.2fx** [%.2fx-%.2fx] | %.2fx | %s | %.1f (%.1f) | %s | %.1f (%.1f) | %.2fx [%.2fx-%.2fx] |\n",
			grp.label, st.nFams, st.nInst, st.famGM, st.famLo, st.famHi, st.pooledGM, st.cbcRatio,
			st.goSum, st.goMedSum, st.cbcSumText, st.pySum, st.pyMedSum, st.medSp, st.minSp, st.maxSp)
	}
}

type cbcSummaryStats struct {
	nFams        int
	nInst        int
	famTotGM     float64
	famTotLo     float64
	famTotHi     float64
	famSolveGM   float64
	famSolveLo   float64
	famSolveHi   float64
	goTotSum     float64
	goTotMedSum  float64
	cbcTotSum    float64
	cbcTotMedSum float64
	medTotSp     float64
	minTotSp     float64
	maxTotSp     float64
}

func summarizeCBCTierSubset(cases []CorpusCase, filter func(CorpusCase) bool, tb *TelemetryBundle) (cbcSummaryStats, error) {
	var st cbcSummaryStats
	var famTotGMs, famSolveGMs, instTotSpeedups []float64
	for _, c := range cases {
		if !filter(c) {
			continue
		}
		sc, okSc := tb.Scalar[c.ID]
		gbRes, okCBC := tb.Goblin[c.ID]
		if !okSc || !okCBC {
			return st, fmt.Errorf("CBC_SUMMARY_TABLE missing scalar or cbc telemetry for %q", c.ID)
		}
		st.nFams++
		st.goTotMedSum += sc.TTotalMS
		st.cbcTotMedSum += gbRes.TTotalMS

		nTot := min(len(gbRes.TTotalTrialsMS), len(sc.TTotalTrialsMS))
		if nTot == 0 {
			st.nInst++
			st.goTotSum += sc.TTotalMS
			st.cbcTotSum += gbRes.TTotalMS
			totR := gbRes.TTotalMS / max(sc.TTotalMS, geoMeanMinFloor)
			famTotGMs = append(famTotGMs, totR)
			instTotSpeedups = append(instTotSpeedups, totR)
		} else {
			st.nInst += nTot
			if fgm, _, _, _ := pairedRatio(gbRes.TTotalTrialsMS[:nTot], sc.TTotalTrialsMS[:nTot]); !math.IsNaN(fgm) {
				famTotGMs = append(famTotGMs, fgm)
			}
			for k := range nTot {
				gt := sc.TTotalTrialsMS[k]
				ct := gbRes.TTotalTrialsMS[k]
				st.goTotSum += gt
				st.cbcTotSum += ct
				if gt > 0 {
					instTotSpeedups = append(instTotSpeedups, ct/gt)
				}
			}
		}

		nSol := min(len(gbRes.TSolveTrialsMS), len(sc.TSolveTrialsMS))
		if nSol == 0 {
			solR := gbRes.TSolveMS / max(sc.TSolveMS, geoMeanMinFloor)
			famSolveGMs = append(famSolveGMs, solR)
		} else if sgm, _, _, _ := pairedRatio(gbRes.TSolveTrialsMS[:nSol], sc.TSolveTrialsMS[:nSol]); !math.IsNaN(sgm) {
			famSolveGMs = append(famSolveGMs, sgm)
		}
	}
	st.famTotGM, st.famTotLo, st.famTotHi = familyClusteredRatio(famTotGMs)
	st.famSolveGM, st.famSolveLo, st.famSolveHi = familyClusteredRatio(famSolveGMs)
	st.medTotSp, st.minTotSp, st.maxTotSp = medianMinMax(instTotSpeedups)
	return st, nil
}

func renderCBCSummaryTable(b *bytes.Buffer, cases []CorpusCase, tb *TelemetryBundle) error {
	groups := []subsetSpec[CorpusCase]{
		{"Tier 1: Micro-models (`N <= 26`)", func(c CorpusCase) bool { return c.Tier == "tier1_canonical" }},
		{"Tier 2: Medium MILPs (`30 <= N <= 84`)", func(c CorpusCase) bool { return c.Tier == "tier2_p90" }},
		{"Tier 3: Large MILPs (`145 <= N <= 4036`)", func(c CorpusCase) bool { return c.Tier == "tier3_large" }},
		{"**Tier 2+3 non-planted MILPs (solver-dominated)**", func(c CorpusCase) bool {
			return c.Tier != "tier1_canonical" && !isPlantedMAD(c.ID)
		}},
		{"All 31 synthetic families", func(_ CorpusCase) bool { return true }},
	}

	b.WriteString("| Workload Subset | Families (Trials) | Equal-Family `BackendCBC / BackendGo` Total [Clustered 95% CI] | Equal-Family `BackendCBC / BackendGo` Solve [Clustered 95% CI] | `BackendGo` Trial (Med) Sum (ms) | `BackendCBC` Trial (Med) Sum (ms) | Per-Instance `CBC / Go` Total Median [Min-Max] |\n")
	b.WriteString("| :--- | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, grp := range groups {
		st, err := summarizeCBCTierSubset(cases, grp.filter, tb)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "| %s | %d (%d) | **%.2fx** [%.2fx-%.2fx] | **%.2fx** [%.2fx-%.2fx] | %.1f (%.1f) | %.1f (%.1f) | %.2fx [%.2fx-%.2fx] |\n",
			grp.label, st.nFams, st.nInst,
			st.famTotGM, st.famTotLo, st.famTotHi,
			st.famSolveGM, st.famSolveLo, st.famSolveHi,
			st.goTotSum, st.goTotMedSum,
			st.cbcTotSum, st.cbcTotMedSum,
			st.medTotSp, st.minTotSp, st.maxTotSp)
	}
	return nil
}

func renderReadmeCBCHeadline(b *bytes.Buffer, cases []CorpusCase, tb *TelemetryBundle) error {
	nonPlanted := func(c CorpusCase) bool {
		return c.Tier != "tier1_canonical" && !isPlantedMAD(c.ID)
	}
	pySt := summarizeTierSubset(cases, nonPlanted, tb)
	cbcSt, err := summarizeCBCTierSubset(cases, nonPlanted, tb)
	if err != nil {
		return err
	}
	fmt.Fprintf(b, "Across the %d solver-dominated Tier 2+3 non-planted MILP families, pure-Go `BackendGo` achieves a **%.2fx** [%.2fx-%.2fx] equal-family geometric-mean total speedup against the Python `PuLP + cbc` subprocess baseline and **%.2fx** [%.2fx-%.2fx] total (**%.2fx** [%.2fx-%.2fx] solve) against in-process C++ `BackendCBC`; Python MPS serialization and process spawn account for the difference between those two ratios.",
		cbcSt.nFams,
		pySt.famGM, pySt.famLo, pySt.famHi,
		cbcSt.famTotGM, cbcSt.famTotLo, cbcSt.famTotHi,
		cbcSt.famSolveGM, cbcSt.famSolveLo, cbcSt.famSolveHi)
	return nil
}

func renderNonMADCBCNotes(b *bytes.Buffer, tb *TelemetryBundle) error {
	sc, okSc := tb.Scalar["t2_binpack_44"]
	gbRes, okCBC := tb.Goblin["t2_binpack_44"]
	if !okSc || !okCBC {
		return errors.New("NON_MAD_CBC_NOTES missing t2_binpack_44 telemetry")
	}
	cbcOverGo := pairedRatioOrFallback(gbRes.TTotalTrialsMS, sc.TTotalTrialsMS, gbRes.TTotalMS, sc.TTotalMS)
	nTrials := min(len(sc.TTotalTrialsMS), len(gbRes.TTotalTrialsMS))
	losses := 0
	worstGo, worstCBC := 0.0, 0.0
	for k := range nTrials {
		gt := sc.TTotalTrialsMS[k]
		ct := gbRes.TTotalTrialsMS[k]
		if gt > ct {
			losses++
		}
		if gt > worstGo {
			worstGo = gt
			worstCBC = ct
		}
	}
	if cbcOverGo >= 1.0 {
		fmt.Fprintf(b, "- **Bin-packing tail behavior on `t2_binpack_44`**: pure-Go `BackendGo` ($%.2f \\pm %.2f$ ms mean, %.2f ms median) leads in-process `BackendCBC` ($%.2f \\pm %.2f$ ms mean, %.2f ms median) with a **%.2fx** `BackendCBC / BackendGo` geometric-mean speedup (`Go / CBC = %.2fx`), running slower on %d of %d trials (peaking at %.2f ms vs. %.2f ms), with `CP5 +Presolve` at %.2f ms vs. `CP6 +Cuts` at %.2f ms.\n",
			sc.TTotalMeanMS, sc.TTotalSDMS, sc.TTotalMS,
			gbRes.TTotalMeanMS, gbRes.TTotalSDMS, gbRes.TTotalMS,
			cbcOverGo, 1.0/max(cbcOverGo, geoMeanMinFloor),
			losses, nTrials, worstGo, worstCBC,
			sc.CP5Presolve.TimeMS, sc.CP6Cuts.TimeMS)
	} else {
		fmt.Fprintf(b, "- **Heavy-tail regression on `t2_binpack_44`**: pure-Go `BackendGo` ($%.2f \\pm %.2f$ ms mean, %.2f ms median) trails in-process `BackendCBC` ($%.2f \\pm %.2f$ ms mean, %.2f ms median) with a **%.2fx** `BackendCBC / BackendGo` geometric-mean ratio (`Go / CBC = %.2fx`), running slower on %d of %d trials (peaking at %.2f ms vs. %.2f ms) while root cuts increase `CP5 +Presolve` (%.2f ms) to `CP6 +Cuts` (%.2f ms).\n",
			sc.TTotalMeanMS, sc.TTotalSDMS, sc.TTotalMS,
			gbRes.TTotalMeanMS, gbRes.TTotalSDMS, gbRes.TTotalMS,
			cbcOverGo, 1.0/max(cbcOverGo, geoMeanMinFloor),
			losses, nTrials, worstGo, worstCBC,
			sc.CP5Presolve.TimeMS, sc.CP6Cuts.TimeMS)
	}
	return nil
}

func renderReadmeMILPSummaryTable(b *bytes.Buffer, cases []CorpusCase, tb *TelemetryBundle) {
	groups := []subsetSpec[CorpusCase]{
		{"Tier 1: Micro-models (`N <= 26`, spawn-dominated)", func(c CorpusCase) bool { return c.Tier == "tier1_canonical" }},
		{"Tier 2: Medium MILPs (`30 <= N <= 84`)", func(c CorpusCase) bool { return c.Tier == "tier2_p90" }},
		{"Tier 3: Large MILPs (`145 <= N <= 4036`)", func(c CorpusCase) bool { return c.Tier == "tier3_large" }},
		{"**Tier 2+3 non-planted MILPs (solver-dominated)**", func(c CorpusCase) bool {
			return c.Tier != "tier1_canonical" && !isPlantedMAD(c.ID)
		}},
		{"All 31 synthetic families", func(_ CorpusCase) bool { return true }},
	}

	b.WriteString("| Workload | Families (Trials) | Go Sum (ms) | Python Sum (ms) | Speedup (GeoMean [95% CI]) | Per-Instance Median [Min-Max] |\n")
	b.WriteString("| :--- | ---: | ---: | ---: | ---: | ---: |\n")
	for _, grp := range groups {
		st := summarizeTierSubset(cases, grp.filter, tb)
		fmt.Fprintf(b, "| %s | %d (%d) | %.1f | %.1f | **%.2fx** [%.2fx-%.2fx] | %.2fx [%.2fx-%.2fx] |\n",
			grp.label, st.nFams, st.nInst, st.goSum, st.pySum, st.famGM, st.famLo, st.famHi, st.medSp, st.minSp, st.maxSp)
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
		reach, verdict := simdVerdict(size >= simdReachSize, lo, hi, n)
		fmt.Fprintf(b, "| `%s` | %d | %s | %s | %.2fx [%.2fx-%.2fx] | %.3f | %.3f | %d |\n",
			c.ID, size, reach, verdict, gm, lo, hi, sc.TSolveMS, sm.TSolveMS, n)
	}
}

func renderSIMDSummary(b *bytes.Buffer, cases []CorpusCase, tb *TelemetryBundle) {
	var tally simdTally
	for _, c := range cases {
		sm := tb.SIMD[c.ID]
		tally.observe(sm.NumVars+sm.NumConstraints >= simdReachSize, tb.Scalar[c.ID].TSolveTrialsMS, sm.TSolveTrialsMS)
	}
	fmt.Fprintf(b, "- **Negative controls** (`n + m < %d`, no AVX2 kernel can run): %d of %d have a 95%% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.\n",
		simdReachSize, tally.controlsFlagged, tally.controls)
	fmt.Fprintf(b, "- **Instances where AVX2 can run**: %d faster, %d slower, %d with no detectable effect, out of %d.\n",
		tally.faster, tally.slower, tally.reachable-tally.faster-tally.slower, tally.reachable)
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
		pyPrep := py.TParseMedianMS + py.TBuildMedianMS
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

func formatMADRatioCI(gm, lo, hi float64, nFams int) string {
	if math.IsNaN(gm) {
		return "n/a"
	}
	if nFams < minPairedSamples || math.IsInf(lo, 0) || math.IsInf(hi, 0) {
		return fmt.Sprintf("**%.1fx**", gm)
	}
	return fmt.Sprintf("**%.1fx** [%.1fx-%.1fx]", gm, lo, hi)
}

type madCombSummaryStats struct {
	nFams         int
	nInst         int
	autoSum       float64
	autoMedSum    float64
	dpSum         float64
	dpMedSum      float64
	bfRanFams     int
	bfSum         float64
	bfMedSum      float64
	milpGoSum     float64
	milpGoMedSum  float64
	milpGoOpt     int
	milpCBCSum    float64
	milpCBCMedSum float64
	milpCBCOpt    int
	hasCBC        bool
	famGoGM       float64
	famGoLo       float64
	famGoHi       float64
	famCBCGM      float64
	famCBCLo      float64
	famCBCHi      float64
}

func summarizeMADCombSubset(madCases []MADCorpusCase, filter func(MADCorpusCase) bool, tb *TelemetryBundle) madCombSummaryStats {
	var st madCombSummaryStats
	var famGoGMs, famCBCGMs []float64
	hasCBCAll := true
	for _, mc := range madCases {
		if !filter(mc) {
			continue
		}
		mr, ok := tb.MAD[mc.ID]
		if !ok {
			continue
		}
		st.nFams++
		st.nInst += mr.Trials
		st.autoMedSum += mr.AutoTimeMS
		st.dpMedSum += mr.DPTimeMS
		if mr.BruteForceRan {
			st.bfRanFams++
			st.bfMedSum += mr.BruteForceTimeMS
			for _, v := range mr.BruteForceTrialsMS {
				st.bfSum += v
			}
		}
		st.milpGoMedSum += mr.MILPGo.TimeMS
		st.milpGoOpt += mr.MILPGo.OptimalCount
		for _, v := range mr.AutoTrialsMS {
			st.autoSum += v
		}
		for _, v := range mr.DPTrialsMS {
			st.dpSum += v
		}
		for _, v := range mr.MILPGo.TrialsMS {
			st.milpGoSum += v
		}
		if fgm, _, _, n := pairedRatio(mr.MILPGo.TrialsMS, mr.AutoTrialsMS); n > 0 && !math.IsNaN(fgm) {
			famGoGMs = append(famGoGMs, fgm)
		} else if mr.AutoTimeMS > 0 {
			famGoGMs = append(famGoGMs, mr.MILPGo.TimeMS/mr.AutoTimeMS)
		}
		if mr.MILPCBC != nil {
			st.milpCBCMedSum += mr.MILPCBC.TimeMS
			st.milpCBCOpt += mr.MILPCBC.OptimalCount
			for _, v := range mr.MILPCBC.TrialsMS {
				st.milpCBCSum += v
			}
			if cgm, _, _, n := pairedRatio(mr.MILPCBC.TrialsMS, mr.AutoTrialsMS); n > 0 && !math.IsNaN(cgm) {
				famCBCGMs = append(famCBCGMs, cgm)
			} else if mr.AutoTimeMS > 0 {
				famCBCGMs = append(famCBCGMs, mr.MILPCBC.TimeMS/mr.AutoTimeMS)
			}
		} else {
			hasCBCAll = false
		}
	}
	st.hasCBC = st.nFams > 0 && hasCBCAll
	st.famGoGM, st.famGoLo, st.famGoHi = familyClusteredRatio(famGoGMs)
	if st.hasCBC {
		st.famCBCGM, st.famCBCLo, st.famCBCHi = familyClusteredRatio(famCBCGMs)
	}
	return st
}

func madCombSubsetSpecs() []subsetSpec[MADCorpusCase] {
	return []subsetSpec[MADCorpusCase]{
		{"PMAD base (`G = 10, I = 2`)", func(c MADCorpusCase) bool { return c.Family == "pmad_base" }},
		{"PMAD small & medium (`I <= 10`)", func(c MADCorpusCase) bool { return c.Family == "pmad" && c.Tier != "tier3_large" }},
		{"PMAD large (`I = 12`)", func(c MADCorpusCase) bool { return c.Family == "pmad" && c.Tier == "tier3_large" }},
		{"Random-weight MAD controls (`I <= 12`)", func(c MADCorpusCase) bool { return c.Family == "mad_comb_rand" }},
		{"**All 13 combinatorial MAD families**", func(_ MADCorpusCase) bool { return true }},
	}
}

func formatMADDFSMedCell(st madCombSummaryStats) string {
	if st.bfRanFams == 0 {
		return "skip (>250k)"
	}
	if st.bfRanFams == st.nFams {
		return formatAssignMS(st.bfMedSum)
	}
	return fmt.Sprintf("%s (%d/%d)", formatAssignMS(st.bfMedSum), st.bfRanFams, st.nFams)
}

func formatMADDFSSumCell(st madCombSummaryStats) string {
	if st.bfRanFams == 0 {
		return "skip (>250k)"
	}
	if st.bfRanFams == st.nFams {
		return fmt.Sprintf("%s (%s)", formatAssignMS(st.bfSum), formatAssignMS(st.bfMedSum))
	}
	return fmt.Sprintf("%s (%s) [%d/%d]", formatAssignMS(st.bfSum), formatAssignMS(st.bfMedSum), st.bfRanFams, st.nFams)
}

func renderReadmeMADSolverSummaryTable(b *bytes.Buffer, madCases []MADCorpusCase, tb *TelemetryBundle) {
	b.WriteString("| Workload | Families (Trials) | Auto Med Sum (ms) | DP Med Sum (ms) | DFS Med Sum (ms) | MILP Go 250 ms Sum (ms) | MILP CBC 250 ms Sum (ms) | Equal-Family `MILP Go / Auto` [95% CI] |\n")
	b.WriteString("| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, grp := range madCombSubsetSpecs() {
		st := summarizeMADCombSubset(madCases, grp.filter, tb)
		cbcCell := "n/a"
		if st.hasCBC {
			cbcCell = fmt.Sprintf("%.1f (%d/%d opt)", st.milpCBCMedSum, st.milpCBCOpt, st.nInst)
		}
		fmt.Fprintf(b, "| %s | %d (%d) | %s | %s | %s | %.1f (%d/%d opt) | %s | %s |\n",
			grp.label, st.nFams, st.nInst,
			formatAssignMS(st.autoMedSum), formatAssignMS(st.dpMedSum),
			formatMADDFSMedCell(st),
			st.milpGoMedSum, st.milpGoOpt, st.nInst,
			cbcCell,
			formatMADRatioCI(st.famGoGM, st.famGoLo, st.famGoHi, st.nFams))
	}
}

func renderMADCombinatorialSummaryTable(b *bytes.Buffer, madCases []MADCorpusCase, tb *TelemetryBundle) {
	b.WriteString("| Workload Subset | Families (Trials) | Auto Trial (Med) Sum (ms) | DP Trial (Med) Sum (ms) | DFS Trial (Med) Sum (ms) | MILP Go Trial (Med) Sum (ms) [Opt] | MILP CBC Trial (Med) Sum (ms) [Opt] | Equal-Family `MILP Go / Auto` [95% CI] | Equal-Family `MILP CBC / Auto` [95% CI] |\n")
	b.WriteString("| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, grp := range madCombSubsetSpecs() {
		st := summarizeMADCombSubset(madCases, grp.filter, tb)
		cbcSumCell := "n/a"
		cbcRatioCell := "n/a"
		if st.hasCBC {
			cbcSumCell = fmt.Sprintf("%.1f (%.1f) [%d/%d]", st.milpCBCSum, st.milpCBCMedSum, st.milpCBCOpt, st.nInst)
			cbcRatioCell = formatMADRatioCI(st.famCBCGM, st.famCBCLo, st.famCBCHi, st.nFams)
		}
		fmt.Fprintf(b, "| %s | %d (%d) | %s (%s) | %s (%s) | %s | %.1f (%.1f) [%d/%d] | %s | %s | %s |\n",
			grp.label, st.nFams, st.nInst,
			formatAssignMS(st.autoSum), formatAssignMS(st.autoMedSum),
			formatAssignMS(st.dpSum), formatAssignMS(st.dpMedSum),
			formatMADDFSSumCell(st),
			st.milpGoSum, st.milpGoMedSum, st.milpGoOpt, st.nInst,
			cbcSumCell,
			formatMADRatioCI(st.famGoGM, st.famGoLo, st.famGoHi, st.nFams),
			cbcRatioCell)
	}
}

func madParityCell(tb *TelemetryBundle, id string) string {
	if len(CheckMADParity(tb, id)) == 0 {
		return "`OK`"
	}
	return "`MISMATCH`"
}

func formatMADMILPCell(m *MADMILPMetrics, trials int) string {
	if m == nil {
		return "n/a"
	}
	if m.OptimalCount == trials {
		return fmt.Sprintf("%.2f (`Optimal`)", m.TimeMS)
	}
	if m.OptimalCount == 0 {
		return fmt.Sprintf("%.2f (`TimeLimit`)", m.TimeMS)
	}
	return fmt.Sprintf("%.2f (%d/%d opt)", m.TimeMS, m.OptimalCount, trials)
}

func renderMADCombinatorialPipelineTable(b *bytes.Buffer, madCases []MADCorpusCase, tb *TelemetryBundle) {
	b.WriteString("| Problem ID | $G \\times I \\times T$ | $\\lambda$ | Auto Strategy | Auto Med (ms) | DP Med (ms) | DFS Med (ms) | MILP Go 250 ms | MILP CBC 250 ms | Trial 0 Obj | Parity |\n")
	b.WriteString("| :--- | :---: | ---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | :---: |\n")
	for _, mc := range madCases {
		mr, ok := tb.MAD[mc.ID]
		if !ok {
			continue
		}
		dfsCell := "skip (>250k)"
		if mr.BruteForceRan {
			dfsCell = formatAssignMS(mr.BruteForceTimeMS)
		}
		fmt.Fprintf(b, "| `%s` | $%d \\times %d \\times %d$ | %.2f | `%s` | %s | %s | %s | %s | %s | %.1f | %s |\n",
			mc.ID, mc.Groups, mc.Items, mc.Periods, mc.Lambda,
			mr.AutoStrategy,
			formatAssignMS(mr.AutoTimeMS),
			formatAssignMS(mr.DPTimeMS),
			dfsCell,
			formatMADMILPCell(&mr.MILPGo, mr.Trials),
			formatMADMILPCell(mr.MILPCBC, mr.Trials),
			mr.Objective,
			madParityCell(tb, mc.ID))
	}
}

func renderReadmeMADSummaryTable(b *bytes.Buffer, cases []CorpusCase, tb *TelemetryBundle) {
	b.WriteString("| Problem | $G \\times I$ | Trials | Go (ms) | Python (ms) | Speedup |\n")
	b.WriteString("| :--- | :---: | ---: | ---: | ---: | ---: |\n")
	for _, c := range cases {
		g, i, _, ok := madDimensions(c.ID)
		if !ok {
			continue
		}
		py := tb.Python[c.ID]
		sc := tb.Scalar[c.ID]
		nTrials := py.Trials
		if nTrials <= 0 {
			nTrials = len(sc.TrialObjectives)
		}
		gm, _, _ := familyTrialRatioRange(py, sc)
		fmt.Fprintf(b, "| `%s` | $%d \\times %d$ | %d | %.2f | %.2f | **%.1fx** |\n",
			c.ID, g, i, nTrials, sc.TTotalMS, py.TTotalMedianMS, gm)
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
		totRatio := pairedRatioOrFallback(sc.TTotalTrialsMS, gb.TTotalTrialsMS, sc.TTotalMS, gb.TTotalMS)
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
		totRatio := pairedRatioOrFallback(sc.TTotalTrialsMS, gb.TTotalTrialsMS, sc.TTotalMS, gb.TTotalMS)
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
		pyPrep := py.TParseMedianMS + py.TBuildMedianMS
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

func renderProvenance(b *bytes.Buffer, cases []CorpusCase, assignCases []AssignCorpusCase, tb *TelemetryBundle) error {
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
	for _, ac := range assignCases {
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
	defaultGoVer := pv.defaultGoVersion
	if defaultGoVer == "" && pv.simdGoVersion != "" {
		defaultGoVer = "paired-ab-ba (" + pv.simdGoVersion + ")"
	}
	fmt.Fprintf(b, "- **Unified telemetry source**: fingerprint `%s` (%s), Go toolchains default `%s` and SIMD `%s`, Linux kernel `%s`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.\n",
		pv.fingerprint, base, defaultGoVer, pv.simdGoVersion, kernelStr)
	fmt.Fprintf(b, "- **CPU affinity**: goblin %s; Python + CBC %s; Python + SciPy %s.\n",
		affinityList(pv.goblinAffinity),
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

type assignTierSummaryStats struct {
	nFams         int
	nInst         int
	famSolveGM    float64
	famSolveLo    float64
	famSolveHi    float64
	pooledSolveGM float64
	famTotGM      float64
	goSolveSum    float64
	goSolveMedSum float64
	spSolveSum    float64
	spSolveMedSum float64
	medSp         float64
	minSp         float64
	maxSp         float64
}

func summarizeAssignTierSubset(assignCases []AssignCorpusCase, filter func(AssignCorpusCase) bool, tb *TelemetryBundle) assignTierSummaryStats {
	var st assignTierSummaryStats
	var famSolveGMs, famTotGMs, spSolvePaired, goSolvePaired, solveSpeedups []float64
	for _, ac := range assignCases {
		if !filter(ac) {
			continue
		}
		st.nFams++
		sp := tb.SciPyAssign[ac.ID]
		sc := tb.AssignScalar[ac.ID]
		st.goSolveMedSum += sc.TSolveMS
		st.spSolveMedSum += sp.TSolveMedianMS
		nInst := min(len(sp.TSolveTrialsMS), len(sc.TSolveTrialsMS))
		if fgm, _, _, _ := pairedRatio(sp.TSolveTrialsMS[:nInst], sc.TSolveTrialsMS[:nInst]); !math.IsNaN(fgm) {
			famSolveGMs = append(famSolveGMs, fgm)
		}
		for k := range nInst {
			ss := sp.TSolveTrialsMS[k]
			gs := sc.TSolveTrialsMS[k]
			spSolvePaired = append(spSolvePaired, ss)
			goSolvePaired = append(goSolvePaired, gs)
			st.goSolveSum += gs
			st.spSolveSum += ss
			if gs > 0 {
				solveSpeedups = append(solveSpeedups, ss/gs)
			}
		}
		nTot := min(len(sp.TTotalTrialsMS), len(sc.TTotalTrialsMS))
		if tgm, _, _, _ := pairedRatio(sp.TTotalTrialsMS[:nTot], sc.TTotalTrialsMS[:nTot]); !math.IsNaN(tgm) {
			famTotGMs = append(famTotGMs, tgm)
		}
	}
	st.famSolveGM, st.famSolveLo, st.famSolveHi = familyClusteredRatio(famSolveGMs)
	st.pooledSolveGM, _, _, st.nInst = pairedRatio(spSolvePaired, goSolvePaired)
	st.famTotGM, _, _ = familyClusteredRatio(famTotGMs)
	st.medSp, st.minSp, st.maxSp = medianMinMax(solveSpeedups)
	return st
}

func renderAssignTierSummaryTable(b *bytes.Buffer, assignCases []AssignCorpusCase, tb *TelemetryBundle) {
	groups := []subsetSpec[AssignCorpusCase]{
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

	b.WriteString("| Workload Subset | Families (Trials) | Equal-Family `SciPy Solve / goblin` GeoMean [Clustered 95% CI] | Pooled Trial `SciPy Solve / goblin` | Equal-Family `SciPy Total / goblin` | `goblin` Solve Trial (Med) Sum (ms) | SciPy Solve Trial (Med) Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |\n")
	b.WriteString("| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, grp := range groups {
		st := summarizeAssignTierSubset(assignCases, grp.filter, tb)
		fmt.Fprintf(b, "| %s | %d (%d) | **%.2fx** [%.2fx-%.2fx] | %.2fx | %.2fx | %s (%s) | %s (%s) | %.2fx [%.2fx-%.2fx] |\n",
			grp.label, st.nFams, st.nInst,
			st.famSolveGM, st.famSolveLo, st.famSolveHi, st.pooledSolveGM, st.famTotGM,
			formatAssignMS(st.goSolveSum), formatAssignMS(st.goSolveMedSum),
			formatAssignMS(st.spSolveSum), formatAssignMS(st.spSolveMedSum),
			st.medSp, st.minSp, st.maxSp)
	}
}

func renderReadmeAssignSummaryTable(b *bytes.Buffer, assignCases []AssignCorpusCase, tb *TelemetryBundle) {
	groups := []subsetSpec[AssignCorpusCase]{
		{"Tier 1: Small (`N <= 32`)", func(c AssignCorpusCase) bool { return c.Tier == "tier1_small" }},
		{"Tier 2: Medium (`64 <= N <= 256`)", func(c AssignCorpusCase) bool { return c.Tier == "tier2_medium" }},
		{"Tier 3: Large (`500 <= N <= 2000`)", func(c AssignCorpusCase) bool { return c.Tier == "tier3_large" }},
		{"**All 22 synthetic LSAP families**", func(_ AssignCorpusCase) bool { return true }},
	}

	b.WriteString("| Workload | Families (Trials) | Go Sum (ms) | SciPy Sum (ms) | Speedup (GeoMean [95% CI]) | Per-Instance Median [Min-Max] |\n")
	b.WriteString("| :--- | ---: | ---: | ---: | ---: | ---: |\n")
	for _, grp := range groups {
		st := summarizeAssignTierSubset(assignCases, grp.filter, tb)
		fmt.Fprintf(b, "| %s | %d (%d) | %s | %s | **%.2fx** [%.2fx-%.2fx] | %.2fx [%.2fx-%.2fx] |\n",
			grp.label, st.nFams, st.nInst,
			formatAssignMS(st.goSolveSum), formatAssignMS(st.spSolveSum),
			st.famSolveGM, st.famSolveLo, st.famSolveHi,
			st.medSp, st.minSp, st.maxSp)
	}
}

func renderAssignPipelineTable(b *bytes.Buffer, assignCases []AssignCorpusCase, tb *TelemetryBundle) {
	b.WriteString("| Problem ID | Shape ($m \\times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\\pm$ SD (ms) | SciPy Solve Mean $\\pm$ SD (ms) | Trial 0 Obj | Parity |\n")
	b.WriteString("| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |\n")
	for _, ac := range assignCases {
		sp := tb.SciPyAssign[ac.ID]
		sc := tb.AssignScalar[ac.ID]
		solveRatio := pairedRatioOrFallback(sp.TSolveTrialsMS, sc.TSolveTrialsMS, sp.TSolveMedianMS, sc.TSolveMS)
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

func assignSIMDReachable(ac AssignCorpusCase) bool {
	if max(ac.Rows, ac.Cols) >= simdReachSize {
		return true
	}
	if ac.Rows == ac.Cols && ac.Sense == assign.Minimize && ac.Family != "sparse" {
		return false
	}
	if ac.Rows > ac.Cols && ac.Rows >= 8 && ac.Cols >= 8 {
		return true
	}
	return ac.Rows*ac.Cols >= simdReachSize
}

func renderAssignSIMDSummary(b *bytes.Buffer, assignCases []AssignCorpusCase, tb *TelemetryBundle) {
	var tally simdTally
	for _, ac := range assignCases {
		tally.observe(assignSIMDReachable(ac), tb.AssignScalar[ac.ID].TSolveTrialsMS, tb.AssignSIMD[ac.ID].TSolveTrialsMS)
	}
	fmt.Fprintf(b, "- **Negative controls** (`max(rows, cols) < %d` with no `>= 8x8` tall transpose and either fused square `Minimize` non-sparse path or `rows * cols < %d`, so no AVX2 kernel runs): %d of %d have a 95%% interval excluding 1.\n",
		simdReachSize, simdReachSize, tally.controlsFlagged, tally.controls)
	fmt.Fprintf(b, "- **Instances where AVX2 kernels run** (`max(rows, cols) >= %d`, tall `rows > cols` with `>= 8x8`, or fallback `AllAbsLE64`/`ValidateAndNegate64` at `rows * cols >= %d`): %d faster, %d slower, %d with no detectable effect, out of %d.\n",
		simdReachSize, simdReachSize, tally.faster, tally.slower, tally.reachable-tally.faster-tally.slower, tally.reachable)
}

func renderAssignSIMDTable(b *bytes.Buffer, assignCases []AssignCorpusCase, tb *TelemetryBundle) {
	b.WriteString("| Problem ID | Shape ($m \\times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |\n")
	b.WriteString("| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |\n")
	for _, ac := range assignCases {
		sc := tb.AssignScalar[ac.ID]
		sm := tb.AssignSIMD[ac.ID]
		size := max(sm.Rows, sm.Cols)
		gm, lo, hi, n := pairedRatio(sc.TSolveTrialsMS, sm.TSolveTrialsMS)
		reach, verdict := simdVerdict(assignSIMDReachable(ac), lo, hi, n)
		fmt.Fprintf(b, "| `%s` | $%d \\times %d$ | %d | %s | %s | %.2fx [%.2fx-%.2fx] | %s | %s | %d |\n",
			ac.ID, ac.Rows, ac.Cols, size, reach, verdict, gm, lo, hi,
			formatAssignMS(sc.TSolveMS), formatAssignMS(sm.TSolveMS), n)
	}
}

const bytesPerKiB = 1024.0

func renderBinarySizeTable(b *bytes.Buffer, tb *TelemetryBundle) error {
	if err := ValidateBinarySizeTelemetry(tb.BinarySize); err != nil {
		return err
	}
	baseID := map[string]string{
		"assign_default":        "assign_default",
		"assign_certified":      "assign_default",
		"goblin_default":        "goblin_default",
		"goblin_certified_vipr": "goblin_default",
	}
	b.WriteString("| Profile | Default Raw / Stripped (KiB) | `GOEXPERIMENT=simd` Raw / Stripped (KiB) | Stripped Delta vs. Default | `goblin` Text (KiB) | `math/big` Syms | `encoding/json` Syms | `VIPR` Syms | `Certif` Syms |\n")
	b.WriteString("| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, id := range CanonicalBinarySizeIDs {
		bs := tb.BinarySize[id]
		base := tb.BinarySize[baseID[id]]
		deltaStr := "baseline"
		if id != baseID[id] && base.StrippedBytes > 0 {
			diffKiB := float64(bs.StrippedBytes-base.StrippedBytes) / bytesPerKiB
			pct := 100.0 * float64(bs.StrippedBytes-base.StrippedBytes) / float64(base.StrippedBytes)
			deltaStr = fmt.Sprintf("%+.1f KiB (%+.1f%%)", diffKiB, pct)
		}
		fmt.Fprintf(b, "| %s | %.1f / **%.1f** | %.1f / **%.1f** | %s | %.1f | `%d` | `%d` | `%d` | `%d` |\n",
			bs.Label,
			float64(bs.RawBytes)/bytesPerKiB, float64(bs.StrippedBytes)/bytesPerKiB,
			float64(bs.SIMDRawBytes)/bytesPerKiB, float64(bs.SIMDStrippedBytes)/bytesPerKiB,
			deltaStr,
			float64(bs.GoblinTextBytes)/bytesPerKiB,
			bs.MathBigSyms, bs.EncodingJSONSyms, bs.VIPRSyms, bs.CertSyms)
	}
	return nil
}

func replaceGeneratedBlocks(
	content string,
	shouldRender func(blockName string) bool,
	render func(blockName string) (string, error),
) (out string, replaced int, err error) {
	const beginPrefix = "<!-- BEGIN GENERATED:"
	const markerSuffix = " -->"

	out = content
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
			return "", 0, fmt.Errorf("malformed BEGIN GENERATED marker at byte %d", startMarkerPos)
		}
		blockName := out[nameStart : nameStart+nameEndRel]
		bodyStart := nameStart + nameEndRel + len(markerSuffix)

		endMarker := fmt.Sprintf("<!-- END GENERATED:%s -->", blockName)
		endRel := strings.Index(out[bodyStart:], endMarker)
		if endRel < 0 {
			return "", 0, fmt.Errorf("missing %s marker", endMarker)
		}
		bodyEnd := bodyStart + endRel

		if shouldRender != nil && !shouldRender(blockName) {
			searchFrom = bodyEnd + len(endMarker)
			continue
		}

		rendered, err := render(blockName)
		if err != nil {
			return "", 0, err
		}

		replacement := "\n" + rendered + "\n"
		out = out[:bodyStart] + replacement + out[bodyEnd:]
		replaced++
		searchFrom = bodyStart + len(replacement) + len(endMarker)
	}
	return out, replaced, nil
}

// SyncMarkdownContent replaces all <!-- BEGIN GENERATED:NAME --> ... <!-- END GENERATED:NAME -->
// regions in content with fresh markdown rendered from tb.
func SyncMarkdownContent(content string, tb *TelemetryBundle) (string, error) {
	var dc docgenCorpus
	return syncMarkdownContentWith(content, tb, &dc)
}

func syncMarkdownContentWith(content string, tb *TelemetryBundle, dc *docgenCorpus) (string, error) {
	synced, _, err := replaceGeneratedBlocks(content, nil, func(blockName string) (string, error) {
		return renderBlockWith(blockName, tb, dc)
	})
	return synced, err
}

// SyncMarkdownFile updates a markdown file in place (or checks it without writing when checkOnly is true).
func SyncMarkdownFile(path string, tb *TelemetryBundle, checkOnly bool) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	orig := string(raw)
	var dc docgenCorpus
	if root, rootErr := findRepoRoot(filepath.Dir(path)); rootErr == nil {
		dc.repoRoot = root
	}
	synced, err := syncMarkdownContentWith(orig, tb, &dc)
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
- **Default pure-Go backend**: default ` + "`go build`" + ` (` + "`BackendGo`" + `, scalar kernels). All headline ` + "`goblin (Go)`" + ` timings use that build.
- **Experimental SIMD build**: ` + "`GOEXPERIMENT=simd`" + `, which dispatches AVX2 kernels for slices of length 32 or more; see section 4.
- **Optional C++ backend**: ` + "`gb.WithCBC()`" + ` (` + "`BackendCBC`" + `, built with ` + "`-tags cbc`" + `), driving in-process COIN-OR CBC/CLP (` + "`c1c171ec`" + `) through ` + "`cgo`" + `.
- **Python baseline**: Python + ` + "`PuLP`" + ` + ` + "`COIN-OR CBC`" + ` (` + "`c1c171ec`" + `, ` + "`threads=1`" + `, ` + "`ratioGap=1e-6`" + `, ` + "`allowableGap=1e-6`" + `) subprocess. Its solve time includes MPS writing, process spawn, and ` + "`.sol`" + ` parsing.
- **Synthetic corpus and public validation**: 31 synthetic LP and MILP families across 3 tiers (` + "`testdata/corpus/*.json`" + ` and ` + "`testdata/corpus/trials/*.jsonl`" + `; 11 distinct randomized trial instances per family for Tiers 1-2 and 5 for Tier 3, 293 instances in total), measured in warm state, complemented by 20 held-out public Netlib LP and MIPLIB 3.0 MILP instances solved to optimality plus a 25-node dual-bound check on ` + "`coinor_capplan1`" + ` with cuts and heuristics disabled (` + "`WithCuts(false)`" + `, ` + "`WithHeuristics(false)`" + `, ` + "`WithMaxNodes(25)`" + `, ` + "`public_benchmarks_test.go`" + `).

<!-- BEGIN GENERATED:PROVENANCE -->
<!-- END GENERATED:PROVENANCE -->

## 2. Aggregate Workload Tier Summary

Tier 1 micro-models are dominated by Python subprocess spawn overhead, and the planted-gap MAD family is constructed so that root heuristics close the gap without branching; the bold row isolates solver-dominated Tier 2 and Tier 3 non-planted MILPs across paired trial instances.

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2.1 Pure-Go vs. In-Process C++ CBC Aggregate Summary

Comparing ` + "`BackendGo`" + ` directly against in-process ` + "`BackendCBC`" + ` removes Python MPS formatting and subprocess spawn overhead across the same five workload subsets:

<!-- BEGIN GENERATED:CBC_SUMMARY_TABLE -->
<!-- END GENERATED:CBC_SUMMARY_TABLE -->

## 3. End-to-End Pipeline Comparison (` + "`JSON -> Solution`" + `)

Pipeline stages measured per instance (median across randomized trials after warmup, plus paired per-trial speedup range):
1. **Prep (` + "`t_parse + t_build`" + ` vs. ` + "`t_parse + t_compile`" + `)**: JSON unmarshaling and PuLP model construction in Python (MPS writing ` + "`t_format`" + ` is recorded separately and timed inside ` + "`t_solve`" + `) vs. streaming ` + "`FromPuLPJSONBytes`" + ` plus ` + "`Model.Compile()`" + ` matrix assembly in Go.
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
- **CP6 (+Cuts)**: GMI, lifted 0-1 knapsack covers, flow/VUB and $(l, S)$ cuts, single-row and multi-row MIR cuts, and globally valid in-tree cuts (` + "`Presolve=on, Cuts=on, Heuristics=off`" + `).
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

### 6.3 LSAP Scalar vs. ` + "`GOEXPERIMENT=simd`" + ` (` + "`AllAbsLE64`" + `, ` + "`MinReducedSkip64`" + `, ` + "`InitReducedRow64`" + `, and ` + "`Min2ReducedRow64`" + `)

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

## 7. Production Binary Footprint and Linker Dead-Code Elimination

Standalone ` + "`CGO_ENABLED=0`" + ` binaries compiled across the four canonical ` + "`goblin/assign`" + ` and ` + "`goblin`" + ` build profiles (` + "`bench/results/goblin_binary_size.json`" + `), verifying that unreferenced ` + "`WithCertified`" + `, ` + "`WithVIPRTrace`" + `, ` + "`encoding/json`" + `, and (in ` + "`goblin/assign`" + `) ` + "`math/big`" + ` symbols are eliminated by the Go linker:

<!-- BEGIN GENERATED:BINARY_SIZE_TABLE -->
<!-- END GENERATED:BINARY_SIZE_TABLE -->

## 8. Combinatorial Group-Item Allocation (Automatic ` + "`Model.Solve`" + ` Dispatch vs. Pure Branch-and-Cut)

The 13-family combinatorial MAD corpus (` + "`testdata/mad_corpus/manifest.json`" + `; 131 randomized trials across the ` + "`pmad_base_g10_i2`" + ` base instance with exact Trial-0 optimum ` + "`24137`" + `, 8 PMAD families across $G \in \{6, 10, 20\}$, $I \in \{4..12\}$, and $\lambda \in \{0.25, 1.0\}$, plus 4 random-weight controls) evaluates default ` + "`Model.Solve`" + ` (automatic algebraic MAD detection routed to ` + "`internal/mad`" + `, alongside internal ` + "`StrategyDP`" + ` and ` + "`StrategyBruteForce`" + ` breakdowns when DFS states are at most ` + "`250,000`" + `) against 250 ms-capped pure Branch-and-Cut (` + "`WithCombinatorialSearch(false)`" + ` on ` + "`BackendGo`" + `, and ` + "`BackendCBC`" + `) with ` + "`1e-9`" + ` combinatorial parity and ` + "`[BestBound, Incumbent]`" + ` containment.

### 8.1 Aggregate Combinatorial MAD Summary

<!-- BEGIN GENERATED:MAD_COMBINATORIAL_SUMMARY_TABLE -->
<!-- END GENERATED:MAD_COMBINATORIAL_SUMMARY_TABLE -->

### 8.2 Full 13-Family Combinatorial MAD Pipeline Comparison

<!-- BEGIN GENERATED:MAD_COMBINATORIAL_PIPELINE_TABLE -->
<!-- END GENERATED:MAD_COMBINATORIAL_PIPELINE_TABLE -->
`
	return SyncMarkdownContent(tpl, tb)
}

// LeanModuleStats summarizes source size, authored declarations, kernel checks, and axiom locks.
type LeanModuleStats struct {
	Name          string
	Files         int
	LOC           int
	AuthoredDecls int
	PublicDecls   int
	PrivateDecls  int
	KernelDecls   int
	AxiomLocks    int
}

// GoFileStats records line count for a single Go source or test file.
type GoFileStats struct {
	RelPath string
	LOC     int
}

// FuzzTargetInfo records a Go fuzz entry point parsed via go/parser and cross-checked against targets.tsv.
type FuzzTargetInfo struct {
	Name       string
	RelPath    string
	PkgDir     string
	IsCBC      bool
	TSVBinary  string
	TSVWorkdir string
	TSVClass   string
}

// FormalInventory captures deterministic disk counts across Lean 4 proofs, Go certifiers, and Fuzz targets.
type FormalInventory struct {
	LeanGoblinFiles    int
	LeanGoblinLOC      int
	LeanAppsFiles      int
	LeanAppsLOC        int
	LeanTotalFiles     int
	LeanTotalLOC       int
	LeanGoblinAuthored int
	LeanGoblinPublic   int
	LeanGoblinPrivate  int
	LeanAppsAuthored   int
	LeanAppsPublic     int
	LeanAppsPrivate    int
	LeanTotalAuthored  int
	LeanTotalPublic    int
	LeanTotalPrivate   int
	LeanGoblinKernel   int
	LeanAppsKernel     int
	LeanTotalKernel    int
	AxiomLocks         int
	LeanModules        []LeanModuleStats
	ProdCertFiles      []GoFileStats
	ProdCertLOC        int
	FormalTestFiles    []GoFileStats
	FormalTestLOC      int
	CertVIPRTestFiles  []GoFileStats
	CertVIPRTestLOC    int
	TotalGoTestFiles   int
	TotalGoTestLOC     int
	FuzzTargets        []FuzzTargetInfo
	DefaultFuzzCount   int
	CBCFuzzCount       int
	TotalFuzzCount     int
	FuzzPackageCounts  map[string]int
	FuzzBinaryCounts   map[string]int
	TSVActiveRowCount  int
}

// leanKernelTheoremCounts tracks per-module Lean kernel TheoremInfo declarations checked
// by Lean.collectAxioms in scripts/verify_formal.sh without spawning Lake during go test.
var leanKernelTheoremCounts = map[string]int{
	"GoblinFormal.Common.Basic": 98,
	"GoblinFormal.Common.Json":  188,
	"GoblinFormal.Track1":       110,
	"GoblinFormal.Track2A":      314,
	"GoblinFormal.Track2B":      97,
	"GoblinFormal.Track2C":      334,
	"GoblinFormal.Track3":       72,
	"GoblinFormal.Track3B":      203,
	"GoblinFormal.Track4":       690,
	"GoblinFormal.Track5A":      648,
	"GoblinFormal.Track5B":      408,
	"GoblinFormal.Track5C":      185,
	"GoblinFormal.Track5D":      377,
	"GoblinFormal.Track5E":      2119,
	"Apps.Common":               4,
	"Apps.BinCheck":             3,
	"Apps.CutsCheck":            0,
	"Apps.LpCheck":              3,
	"Apps.LsapCheck":            21,
	"Apps.RulesCheck":           2,
	"Apps.SimdCfgCheck":         0,
	"Apps.ViprCheck":            0,
}

var formalGeneratedBlocks = map[string]bool{
	"README_FORMAL_SUMMARY":     true,
	"INDEX_FORMAL_SUMMARY":      true,
	"BENCHMARKS_FORMAL_SUMMARY": true,
	"FORMAL_OVERVIEW_STATS":     true,
	"FORMAL_GO_STATS":           true,
	"FORMAL_MODULE_MATRIX":      true,
	"FORMAL_FUZZ_INVENTORY":     true,
}

func findRepoRoot(start string) (string, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err == nil && !info.IsDir() {
		abs = filepath.Dir(abs)
	}
	for cur := abs; ; {
		goMod := filepath.Join(cur, "go.mod")
		lakeToml := filepath.Join(cur, "formal", "lakefile.toml")
		if _, err1 := os.Stat(goMod); err1 == nil {
			if _, err2 := os.Stat(lakeToml); err2 == nil {
				return cur, nil
			}
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fmt.Errorf("could not locate repository root from %s", start)
		}
		cur = parent
	}
}

func countFileLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	lines := bytes.Count(data, []byte{'\n'})
	if data[len(data)-1] != '\n' {
		lines++
	}
	return lines
}

func formatCommaInt(n int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		if neg {
			return "-" + s
		}
		return s
	}
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	rem := len(s) % 3
	if rem > 0 {
		b.WriteString(s[:rem])
		if len(s) > rem {
			b.WriteByte(',')
		}
	}
	for i := rem; i < len(s); i += 3 {
		if i > rem {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

func stripLeanComments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	depth := 0
	inStr := false
	for i := 0; i < len(src); {
		if depth > 0 {
			if strings.HasPrefix(src[i:], "/-") {
				depth++
				i += 2
				continue
			}
			if strings.HasPrefix(src[i:], "-/") {
				depth--
				i += 2
				continue
			}
			if src[i] == '\n' {
				b.WriteByte('\n')
			}
			i++
			continue
		}
		if inStr {
			b.WriteByte(src[i])
			if src[i] == '\\' && i+1 < len(src) {
				i++
				b.WriteByte(src[i])
				i++
				continue
			}
			if src[i] == '"' {
				inStr = false
			}
			i++
			continue
		}
		if strings.HasPrefix(src[i:], "/-") {
			depth++
			i += 2
			continue
		}
		if strings.HasPrefix(src[i:], "--") {
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		if src[i] == '"' {
			inStr = true
		}
		b.WriteByte(src[i])
		i++
	}
	return b.String()
}

func countLeanTheoremDecls(stripped string) (pub, priv int) {
	modifiers := []string{"private ", "private\t", "protected ", "protected\t", "noncomputable ", "noncomputable\t", "unsafe ", "unsafe\t", "partial ", "partial\t"}
	for line := range strings.SplitSeq(stripped, "\n") {
		s := strings.TrimLeft(line, " \t")
		for strings.HasPrefix(s, "@[") {
			end := strings.IndexByte(s, ']')
			if end < 0 {
				break
			}
			s = strings.TrimLeft(s[end+1:], " \t")
		}
		isPriv := false
		for {
			matched := false
			for _, mod := range modifiers {
				if strings.HasPrefix(s, mod) {
					if strings.HasPrefix(mod, "private") {
						isPriv = true
					}
					s = strings.TrimLeft(s[len(mod):], " \t")
					matched = true
					break
				}
			}
			if !matched {
				break
			}
		}
		if s == "theorem" || s == "lemma" ||
			strings.HasPrefix(s, "theorem ") || strings.HasPrefix(s, "theorem\t") ||
			strings.HasPrefix(s, "lemma ") || strings.HasPrefix(s, "lemma\t") {
			if isPriv {
				priv++
			} else {
				pub++
			}
		}
	}
	return pub, priv
}

func countAxiomAuditLocks(stripped string) (total int, byModule map[string]int, err error) {
	byModule = make(map[string]int)
	sawGuardMsgs := false
	for line := range strings.SplitSeq(stripped, "\n") {
		s := strings.TrimSpace(line)
		if s == "" {
			continue
		}
		if strings.HasPrefix(s, "#guard_msgs") {
			sawGuardMsgs = true
			continue
		}
		if strings.HasPrefix(s, "#print") {
			fields := strings.Fields(s)
			if len(fields) >= 3 && fields[0] == "#print" && fields[1] == "axioms" {
				if !sawGuardMsgs {
					return 0, nil, fmt.Errorf("unguarded #print axioms %s in AxiomAudit.lean", fields[2])
				}
				total++
				sym := fields[2]
				modKey := "Common"
				if rest, ok := strings.CutPrefix(sym, "GoblinFormal."); ok {
					if strings.HasPrefix(rest, "Track") {
						if dot := strings.IndexByte(rest, '.'); dot > 0 {
							modKey = rest[:dot]
						}
					}
				}
				byModule[modKey]++
			}
		}
		sawGuardMsgs = false
	}
	return total, byModule, nil
}

func leanRelToModuleName(relSlash string) string {
	noExt := strings.TrimSuffix(relSlash, ".lean")
	return strings.ReplaceAll(noExt, "/", ".")
}

func leanModuleGroup(modName string) string {
	if strings.HasPrefix(modName, "Apps.") {
		return "Apps"
	}
	if modName == "GoblinFormal.Common.Basic" || modName == "GoblinFormal.Common.Json" {
		return "Common"
	}
	if rest, ok := strings.CutPrefix(modName, "GoblinFormal."); ok {
		if strings.HasPrefix(rest, "Track") {
			return rest
		}
	}
	return ""
}

func collectLeanInventory(root string, inv *FormalInventory) error {
	formalDir := filepath.Join(root, "formal")
	moduleOrder := []string{
		"Common", "Track1", "Track2A", "Track2B", "Track2C",
		"Track3", "Track3B", "Track4", "Track5A", "Track5B", "Track5C", "Track5D", "Track5E", "Apps",
	}
	modStats := make(map[string]*LeanModuleStats, len(moduleOrder))
	for _, name := range moduleOrder {
		modStats[name] = &LeanModuleStats{Name: name}
	}

	var axiomAuditStripped string
	err := filepath.WalkDir(formalDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		// Skip .lake whether directory or symlink to shared build cache.
		if d.Name() == ".lake" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".lean") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(formalDir, path)
		if err != nil {
			return err
		}
		relSlash := filepath.ToSlash(rel)
		loc := countFileLines(data)
		stripped := stripLeanComments(string(data))
		pub, priv := countLeanTheoremDecls(stripped)

		modName := leanRelToModuleName(relSlash)
		if modName == "GoblinFormal.AxiomAudit" {
			axiomAuditStripped = stripped
		}
		kernelCount := 0
		if modName != "GoblinFormal" && modName != "GoblinFormal.AxiomAudit" {
			cnt, ok := leanKernelTheoremCounts[modName]
			if !ok {
				return fmt.Errorf("lean module %s missing from leanKernelTheoremCounts manifest", modName)
			}
			kernelCount = cnt
		}

		isApps := strings.HasPrefix(relSlash, "Apps/")
		if isApps {
			inv.LeanAppsFiles++
			inv.LeanAppsLOC += loc
			inv.LeanAppsPublic += pub
			inv.LeanAppsPrivate += priv
			inv.LeanAppsAuthored += pub + priv
			inv.LeanAppsKernel += kernelCount
		} else {
			inv.LeanGoblinFiles++
			inv.LeanGoblinLOC += loc
			inv.LeanGoblinPublic += pub
			inv.LeanGoblinPrivate += priv
			inv.LeanGoblinAuthored += pub + priv
			inv.LeanGoblinKernel += kernelCount
		}

		if grp := leanModuleGroup(modName); grp != "" {
			st := modStats[grp]
			st.Files++
			st.LOC += loc
			st.PublicDecls += pub
			st.PrivateDecls += priv
			st.AuthoredDecls += pub + priv
			st.KernelDecls += kernelCount
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk formal Lean files: %w", err)
	}
	if axiomAuditStripped == "" {
		return errors.New("missing formal/GoblinFormal/AxiomAudit.lean")
	}
	locks, locksByMod, err := countAxiomAuditLocks(axiomAuditStripped)
	if err != nil {
		return err
	}
	inv.AxiomLocks = locks
	for modKey, count := range locksByMod {
		if st, ok := modStats[modKey]; ok {
			st.AxiomLocks += count
		}
	}

	inv.LeanTotalFiles = inv.LeanGoblinFiles + inv.LeanAppsFiles
	inv.LeanTotalLOC = inv.LeanGoblinLOC + inv.LeanAppsLOC
	inv.LeanTotalPublic = inv.LeanGoblinPublic + inv.LeanAppsPublic
	inv.LeanTotalPrivate = inv.LeanGoblinPrivate + inv.LeanAppsPrivate
	inv.LeanTotalAuthored = inv.LeanGoblinAuthored + inv.LeanAppsAuthored
	inv.LeanTotalKernel = inv.LeanGoblinKernel + inv.LeanAppsKernel

	inv.LeanModules = make([]LeanModuleStats, 0, len(moduleOrder))
	for _, name := range moduleOrder {
		inv.LeanModules = append(inv.LeanModules, *modStats[name])
	}
	return nil
}

func isRepoSkipDir(name string) bool {
	switch name {
	case ".git", ".lake", ".cache", "third_party", "testdata":
		return true
	default:
		return false
	}
}

func collectGoFormalInventory(root string, inv *FormalInventory) error {
	prodRelPaths := []string{
		"exact_polish.go",
		"vipr_json.go",
		"vipr_trace.go",
		"certified.go",
		filepath.Join("assign", "certified.go"),
		filepath.Join("internal", "mad", "certified.go"),
	}
	for _, rel := range prodRelPaths {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return fmt.Errorf("read %s: %w", rel, err)
		}
		loc := countFileLines(data)
		inv.ProdCertFiles = append(inv.ProdCertFiles, GoFileStats{
			RelPath: filepath.ToSlash(rel),
			LOC:     loc,
		})
		inv.ProdCertLOC += loc
	}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if path != root && isRepoSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == ".lake" {
			return nil
		}
		if !strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relSlash := filepath.ToSlash(rel)
		base := d.Name()
		isFormal := strings.HasPrefix(base, "formal")
		isCertOrVIPR := strings.Contains(base, "certified") ||
			strings.Contains(base, "vipr") ||
			strings.Contains(base, "exact_polish") ||
			strings.Contains(base, "tcb")
		if !isFormal && !isCertOrVIPR {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		loc := countFileLines(data)
		st := GoFileStats{RelPath: relSlash, LOC: loc}
		if isFormal {
			inv.FormalTestFiles = append(inv.FormalTestFiles, st)
			inv.FormalTestLOC += loc
		} else {
			inv.CertVIPRTestFiles = append(inv.CertVIPRTestFiles, st)
			inv.CertVIPRTestLOC += loc
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk Go formal test files: %w", err)
	}
	inv.TotalGoTestFiles = len(inv.FormalTestFiles) + len(inv.CertVIPRTestFiles)
	inv.TotalGoTestLOC = inv.FormalTestLOC + inv.CertVIPRTestLOC
	return nil
}

type fuzzTSVRow struct {
	binary  string
	workdir string
	target  string
	class   string
}

func parseTargetsTSV(path string) (map[string]fuzzTSVRow, int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	rows := make(map[string]fuzzTSVRow)
	count := 0
	for lineNo, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 6 {
			return nil, 0, fmt.Errorf("%s:%d expected 6 tab-separated fields, got %d", path, lineNo+1, len(fields))
		}
		row := fuzzTSVRow{
			binary:  fields[1],
			workdir: fields[2],
			target:  fields[3],
			class:   fields[4],
		}
		if _, dup := rows[row.target]; dup {
			return nil, 0, fmt.Errorf("%s:%d duplicate target %q", path, lineNo+1, row.target)
		}
		rows[row.target] = row
		count++
	}
	return rows, count, nil
}

func fileRequiresCBCTag(fileAST *ast.File) (bool, error) {
	for _, cg := range fileAST.Comments {
		if cg.Pos() > fileAST.Package {
			break
		}
		for _, c := range cg.List {
			if constraint.IsGoBuild(c.Text) || constraint.IsPlusBuild(c.Text) {
				expr, err := constraint.Parse(c.Text)
				if err != nil {
					return false, err
				}
				defaultOK := expr.Eval(func(tag string) bool { return tag == "cgo" })
				cbcOK := expr.Eval(func(tag string) bool { return tag == "cbc" || tag == "cgo" })
				if !defaultOK && cbcOK {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func collectFuzzTargetsInventory(root string, inv *FormalInventory) error {
	inv.FuzzPackageCounts = make(map[string]int)
	inv.FuzzBinaryCounts = make(map[string]int)

	tsvPath := filepath.Join(root, "scripts", "gcp-fuzz", "targets.tsv")
	tsvRows, tsvCount, err := parseTargetsTSV(tsvPath)
	if err != nil {
		return fmt.Errorf("parse targets.tsv: %w", err)
	}
	inv.TSVActiveRowCount = tsvCount

	fset := token.NewFileSet()
	discovered := make(map[string]FuzzTargetInfo)
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if path != root && isRepoSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == ".lake" || !strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		fileAST, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			return fmt.Errorf("parse %s: %w", path, parseErr)
		}
		isCBC, tagErr := fileRequiresCBCTag(fileAST)
		if tagErr != nil {
			return fmt.Errorf("parse build tag in %s: %w", path, tagErr)
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		relSlash := filepath.ToSlash(rel)
		pkgDir := filepath.ToSlash(filepath.Dir(rel))
		for _, decl := range fileAST.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Fuzz") {
				continue
			}
			name := fn.Name.Name
			if prev, dup := discovered[name]; dup {
				return fmt.Errorf("duplicate Fuzz declaration %s in %s and %s", name, prev.RelPath, relSlash)
			}
			discovered[name] = FuzzTargetInfo{
				Name:    name,
				RelPath: relSlash,
				PkgDir:  pkgDir,
				IsCBC:   isCBC,
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk Fuzz declarations: %w", err)
	}

	var missingInTSV []string
	for name, info := range discovered {
		tsvRow, ok := tsvRows[name]
		if !ok {
			missingInTSV = append(missingInTSV, name)
			continue
		}
		wantWorkdir := "/app/work"
		if info.PkgDir != "." {
			wantWorkdir = "/app/work/" + info.PkgDir
		}
		if tsvRow.workdir != wantWorkdir {
			return fmt.Errorf("targets.tsv workdir mismatch for %s: got %q, want %q", name, tsvRow.workdir, wantWorkdir)
		}
		tsvCBC := strings.HasSuffix(tsvRow.binary, "_cbc.test")
		if info.IsCBC != tsvCBC {
			return fmt.Errorf("targets.tsv cbc binary mismatch for %s: binary=%s vs go:build cbc=%v", name, tsvRow.binary, info.IsCBC)
		}
		info.TSVBinary = tsvRow.binary
		info.TSVWorkdir = tsvRow.workdir
		info.TSVClass = tsvRow.class
		inv.FuzzTargets = append(inv.FuzzTargets, info)
		if info.IsCBC {
			inv.CBCFuzzCount++
		} else {
			inv.DefaultFuzzCount++
		}
		inv.FuzzPackageCounts[info.PkgDir]++
		inv.FuzzBinaryCounts[filepath.Base(info.TSVBinary)]++
	}
	if len(missingInTSV) > 0 {
		slices.Sort(missingInTSV)
		return fmt.Errorf("fuzz declarations missing from scripts/gcp-fuzz/targets.tsv: %v", missingInTSV)
	}

	var missingInGo []string
	for name := range tsvRows {
		if _, ok := discovered[name]; !ok {
			missingInGo = append(missingInGo, name)
		}
	}
	if len(missingInGo) > 0 {
		slices.Sort(missingInGo)
		return fmt.Errorf("scripts/gcp-fuzz/targets.tsv targets missing from Go *_test.go AST: %v", missingInGo)
	}

	slices.SortFunc(inv.FuzzTargets, func(a, b FuzzTargetInfo) int {
		return strings.Compare(a.Name, b.Name)
	})
	inv.TotalFuzzCount = inv.DefaultFuzzCount + inv.CBCFuzzCount
	return nil
}

// CollectFormalInventory deterministically computes Lean 4 proof counts, Go cert/VIPR
// code counts, and Go AST Fuzz* target counts cross-checked against targets.tsv.
func CollectFormalInventory(root string) (*FormalInventory, error) {
	inv := &FormalInventory{}
	if err := collectLeanInventory(root, inv); err != nil {
		return nil, err
	}
	if err := collectGoFormalInventory(root, inv); err != nil {
		return nil, err
	}
	if err := collectFuzzTargetsInventory(root, inv); err != nil {
		return nil, err
	}
	return inv, nil
}

func renderFormalBlock(blockName string, inv *FormalInventory) (string, error) {
	var b strings.Builder
	switch blockName {
	case "README_FORMAL_SUMMARY":
		fmt.Fprintf(&b,
			"An isolated offline Lean 4 workspace ([`formal/`](formal/lakefile.toml), pinned to `v4.36.0` with `batteries` `v4.36.0` as `lintDriver`) with zero `sorry` or custom axioms proves soundness for the standalone CLI certificate checkers, abstract algebraic presolve and cutting-plane models, and compiled `arm64`/`amd64` zero-allocation kernels across `%d` Lean files (`%s` LOC: `%s` authored `theorem` and `lemma` declarations with `%s` in `GoblinFormal` plus `%d` in `Apps/LsapCheck.lean`, `%s` kernel `TheoremInfo` declarations including compiler-synthesized helper, equation, and splitter lemmas, and `%d` `#print axioms` locks), paired with `%d` pure-Go certification files (`%s` LOC), `%d` formal and certified Go test files (`%s` LOC), and `%d` `Fuzz*` targets (`%d` default-build and `%d` `-tags=cbc`, matching [`scripts/gcp-fuzz/targets.tsv`](scripts/gcp-fuzz/targets.tsv)). Hosted CI ([`.github/workflows/ci.yml`](.github/workflows/ci.yml)) runs Go test matrices, codegen invariants, and Lean verification (`lake build --wfail && lake lint` plus Track 5 ELF checks), [`scripts/pre-commit.sh`](scripts/pre-commit.sh) runs those checks locally (emitting a warning and skipping Lean if `lake` is absent unless `GOBLIN_STRICT_GATES=1`), [`scripts/verify_formal.sh`](scripts/verify_formal.sh) runs full formal suites and all `%d` default-build `^Fuzz` seed corpora, and plain `go test ./...` skips Lean subprocess checks.",
			inv.LeanTotalFiles,
			formatCommaInt(inv.LeanTotalLOC),
			formatCommaInt(inv.LeanTotalAuthored),
			formatCommaInt(inv.LeanGoblinAuthored),
			inv.LeanAppsAuthored,
			formatCommaInt(inv.LeanTotalKernel),
			inv.AxiomLocks,
			len(inv.ProdCertFiles),
			formatCommaInt(inv.ProdCertLOC),
			inv.TotalGoTestFiles,
			formatCommaInt(inv.TotalGoTestLOC),
			inv.TotalFuzzCount,
			inv.DefaultFuzzCount,
			inv.CBCFuzzCount,
			inv.DefaultFuzzCount,
		)
	case "INDEX_FORMAL_SUMMARY":
		fmt.Fprintf(&b,
			"4. **[Formal Verification Architecture (Lean 4 Prove Lane)](formal-verification.md)**: Industry precedents, anti-translation-gap boundaries, pure-Go runtime checking (`WithCertified`, `WithCertifiedWitness`, and `assign.WithCertified`, which never invoke Lean at runtime and carry no formal proof of their Go code), and offline Lean 4 CLI equivalence validated under `GOBLIN_FORMAL=1`. Hosted CI ([`.github/workflows/ci.yml`](../.github/workflows/ci.yml)) enforces Go tests and Lean checks, [`scripts/pre-commit.sh`](../scripts/pre-commit.sh) runs incremental `lake build --wfail` and `lake lint` alongside Track 5 ELF verification locally (emitting a warning and skipping Lean gates if `lake` is missing unless `GOBLIN_STRICT_GATES=1`), [`scripts/verify_formal.sh`](../scripts/verify_formal.sh) runs all `%d` default-build `^Fuzz` seed corpora across `./...` alongside `%d` `-tags=cbc` targets (`%d` total in `scripts/gcp-fuzz/targets.tsv`), and plain `go test ./...` skips Lean subprocess checks. Across `%d` Lean files (`%s` LOC), the suite contains `%s` authored `theorem` and `lemma` declarations (`%s` in `formal/GoblinFormal` plus `%d` private lemma in `Apps/LsapCheck.lean`), while Lean checks `%s` kernel declarations including compiler-synthesized helper, equation, and splitter lemmas (`%d` axiom locks plus `#verify_all_goblin_axioms` and `Apps.*` audits in `formal/GoblinFormal/AxiomAudit.lean`, with `0` `sorry` or custom axioms). Standalone Lean CLI checkers prove certificate validity (`assign`, `simplex.go`, `exact_polish.go`, `certified.go`, `vipr_trace.go`), Track 2 rules, Track 3B LP dual postsolve, and Track 4 B&B state invariants formalize abstract standalone Lean models with no mechanical link to imperative Go files (`presolve.go`, `cuts.go`, `bab.go`), and Track 5 checks `14` `arm64` ELF routines (`7` `-gcflags=-l` standalone builds of functions inlined in production) plus `6` `amd64` AVX kernels for uncertified solves only (assuming panic-free comparison guards by trace hypothesis on guarded branches and trusting an `18`-symbol Zero-GC allow-list up to the first external call).",
			inv.DefaultFuzzCount,
			inv.CBCFuzzCount,
			inv.TotalFuzzCount,
			inv.LeanTotalFiles,
			formatCommaInt(inv.LeanTotalLOC),
			formatCommaInt(inv.LeanTotalAuthored),
			formatCommaInt(inv.LeanGoblinAuthored),
			inv.LeanAppsAuthored,
			formatCommaInt(inv.LeanTotalKernel),
			inv.AxiomLocks,
		)
	case "BENCHMARKS_FORMAL_SUMMARY":
		fmt.Fprintf(&b,
			"Automated formal inventory across the workspace covers `%d` Lean files (`%s` LOC: `%s` authored `theorem` and `lemma` declarations, `%s` kernel `TheoremInfo` entries, and `%d` axiom locks), `%d` Go certification files (`%s` LOC) paired with `%d` formal and certified test files (`%s` LOC), and `%d` `Fuzz*` targets (`%d` default-build across `./...` plus `%d` `-tags=cbc`, matching [`scripts/gcp-fuzz/targets.tsv`](../scripts/gcp-fuzz/targets.tsv)).",
			inv.LeanTotalFiles,
			formatCommaInt(inv.LeanTotalLOC),
			formatCommaInt(inv.LeanTotalAuthored),
			formatCommaInt(inv.LeanTotalKernel),
			inv.AxiomLocks,
			len(inv.ProdCertFiles),
			formatCommaInt(inv.ProdCertLOC),
			inv.TotalGoTestFiles,
			formatCommaInt(inv.TotalGoTestLOC),
			inv.TotalFuzzCount,
			inv.DefaultFuzzCount,
			inv.CBCFuzzCount,
		)
	case "FORMAL_OVERVIEW_STATS":
		fmt.Fprintf(&b,
			"All proofs and standalone verifiers live in [`formal/`](../formal/lakefile.toml), pinned to Lean `v4.36.0` with `batteries` `v4.36.0` as `lintDriver`, compiling across 78 Lake build jobs with zero warnings (`%d` Lean files, `%s` LOC: `%d` modules in `GoblinFormal` totaling `%s` LOC and `%d` files in `Apps/`, comprising shared `Apps/Common.lean` plus `7` CLI executables, totaling `%s` LOC, paired with `%d` formal, certified, and VIPR Go files totaling `%s` LOC). Across that codebase, the suite contains `%s` authored `theorem` and `lemma` declarations (`%s` in `GoblinFormal`: `%s` public and `%s` private, plus `%d` private bounds lemma in `Apps/LsapCheck.lean`). Programmatic `Lean.collectAxioms` inspection via `#verify_all_goblin_axioms` (`GoblinFormal` and `Apps.Common` at build time) and `auditAllAppsModules` across all `%d` `Apps.*` modules checks `%s` kernel `TheoremInfo` declarations (`%s` in `GoblinFormal` plus `%s` in `Apps.*`, a total that includes compiler-synthesized helper, equation, and splitter lemmas) alongside `%d` `#guard_msgs in #print axioms` locks in [`AxiomAudit.lean`](../formal/GoblinFormal/AxiomAudit.lean).",
			inv.LeanTotalFiles,
			formatCommaInt(inv.LeanTotalLOC),
			inv.LeanGoblinFiles,
			formatCommaInt(inv.LeanGoblinLOC),
			inv.LeanAppsFiles,
			formatCommaInt(inv.LeanAppsLOC),
			len(inv.ProdCertFiles)+inv.TotalGoTestFiles,
			formatCommaInt(inv.ProdCertLOC+inv.TotalGoTestLOC),
			formatCommaInt(inv.LeanTotalAuthored),
			formatCommaInt(inv.LeanGoblinAuthored),
			formatCommaInt(inv.LeanGoblinPublic),
			formatCommaInt(inv.LeanGoblinPrivate),
			inv.LeanAppsAuthored,
			inv.LeanAppsFiles,
			formatCommaInt(inv.LeanTotalKernel),
			formatCommaInt(inv.LeanGoblinKernel),
			formatCommaInt(inv.LeanAppsKernel),
			inv.AxiomLocks,
		)
	case "FORMAL_GO_STATS":
		byRel := make(map[string]int, len(inv.ProdCertFiles))
		for _, f := range inv.ProdCertFiles {
			byRel[f.RelPath] = f.LOC
		}
		fmt.Fprintf(&b,
			"On the Go side, `%d` production files ([`exact_polish.go`](../exact_polish.go) at `%s` LOC, [`vipr_json.go`](../vipr_json.go) at `%s` LOC, [`vipr_trace.go`](../vipr_trace.go) at `%s` LOC, [`certified.go`](../certified.go) at `%s` LOC, [`assign/certified.go`](../assign/certified.go) at `%s` LOC, and [`internal/mad/certified.go`](../internal/mad/certified.go) at `%s` LOC; `%s` LOC combined) implement exact `*big.Rat` basis polishing, LP/MILP unboundedness extraction, zero-reflection Mini-VIPR JSON codecs, presolve-enveloped and orbital-symmetry `WithVIPRTrace` proof recording, and opt-in fail-closed `WithCertified` / `assign.WithCertified` runtime verification in pure standard-library Go (never linking or invoking Lean CLI binaries at runtime) with zero hot-path overhead when disabled, while `%d` formal, certified, and VIPR test files (`%s` LOC total: `%d` `formal*` test files at `%s` LOC plus `%d` certified, VIPR, exact-polish, and TCB test files at `%s` LOC) house negative controls, subprocess pools, linker dead-code-elimination checks, and live observer bridges.",
			len(inv.ProdCertFiles),
			formatCommaInt(byRel["exact_polish.go"]),
			formatCommaInt(byRel["vipr_json.go"]),
			formatCommaInt(byRel["vipr_trace.go"]),
			formatCommaInt(byRel["certified.go"]),
			formatCommaInt(byRel["assign/certified.go"]),
			formatCommaInt(byRel["internal/mad/certified.go"]),
			formatCommaInt(inv.ProdCertLOC),
			inv.TotalGoTestFiles,
			formatCommaInt(inv.TotalGoTestLOC),
			len(inv.FormalTestFiles),
			formatCommaInt(inv.FormalTestLOC),
			len(inv.CertVIPRTestFiles),
			formatCommaInt(inv.CertVIPRTestLOC),
		)
	case "FORMAL_MODULE_MATRIX":
		renderFormalModuleMatrix(&b, inv)
	case "FORMAL_FUZZ_INVENTORY":
		renderFormalFuzzInventory(&b, inv)
	default:
		return "", fmt.Errorf("unknown formal generated block %q", blockName)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func renderFormalModuleMatrix(b *strings.Builder, inv *FormalInventory) {
	type rowMeta struct {
		component string
		module    string
		theorems  string
		harness   string
	}
	metaByGroup := map[string]rowMeta{
		"Common": {
			component: "**Common**",
			module:    "[`Common/Basic.lean`](../formal/GoblinFormal/Common/Basic.lean), [`Common/Json.lean`](../formal/GoblinFormal/Common/Json.lean)",
			theorems:  "`finSum_inj_le`, `finSum_inj_eq`, `intFinSum_dvd`, `float64BitsToRat_isNone_iff`, `float64BitsToRat_eq_ieee754`",
			harness:   "[`formal_common_test.go`](../formal_common_test.go), [`formal_oracle_pool_test.go`](../formal_oracle_pool_test.go)",
		},
		"Track1": {
			component: "**Track 1 (LSAP)**",
			module:    "[`Track1.lean`](../formal/GoblinFormal/Track1.lean), `lsap-check`",
			theorems:  "`checkLSAP_sound`, `checkLSAPTranspose_sound`, `checkLSAPForbidden_sound`, `checkHallInfeasible_sound`, `safe_objective_within_tol`",
			harness:   "[`assign/certified.go`](../assign/certified.go), [`assign/formal_lsap_test.go`](../assign/formal_lsap_test.go), [`assign/formal_fuzz_bridge_test.go`](../assign/formal_fuzz_bridge_test.go)",
		},
		"Track2A": {
			component: "**Track 2A (Presolve)**",
			module:    "[`Track2A.lean`](../formal/GoblinFormal/Track2A.lean), `rules-check`",
			theorems:  "`divCeilSpec_spec`, `hasInfeasibleIntEqSpec_sound`, `evalActivitySummarySpec_sound`, `checkBinStrengthenFromRowSpec_sound`, `verifyOrbitSymmetrySpec_sound`, `orbit_chain_ordering_forced_zero_sound`",
			harness:   "[`formal_track2a_test.go`](../formal_track2a_test.go), [`formal_export_test.go`](../formal_export_test.go), [`formal_adversarial_regression_test.go`](../formal_adversarial_regression_test.go)",
		},
		"Track2B": {
			component: "**Track 2B (Core Cuts)**",
			module:    "[`Track2B.lean`](../formal/GoblinFormal/Track2B.lean), `cuts-check`",
			theorems:  "`gmi_cut_sound`, `checkTableauGMI_sound`, `checkRowAgg_sound`, `cmir_safe_rounded_sound`, `checkCMIRCertificate_sound`, `checkVUBDisagg_sound`",
			harness:   "[`formal_track2b_test.go`](../formal_track2b_test.go), [`formal_track2_live_export_test.go`](../formal_track2_live_export_test.go), [`formal_export_test.go`](../formal_export_test.go)",
		},
		"Track2C": {
			component: "**Track 2C (Adv. Cuts)**",
			module:    "[`Track2C.lean`](../formal/GoblinFormal/Track2C.lean), `cuts-check` (`gmi-check`, `vub-check`, `cover-check`, `lotsizing-check`, `agg-cmir-check`, `drop-check`)",
			theorems:  "`checkStructuralGMICert_sound`, `checkStructuralGMICertFast_sound`, `checkStructuralVUBCert_sound`, `checkKnapsackCoverCert_sound`, `checkLotSizingCutCert_sound`, `checkAggCMIRCert_sound`, `checkFinalizedCutCert_sound`",
			harness:   "[`formal_track2c_test.go`](../formal_track2c_test.go), [`formal_track2_live_export_test.go`](../formal_track2_live_export_test.go), [`formal_parallel_adversarial_test.go`](../formal_parallel_adversarial_test.go)",
		},
		"Track3": {
			component: "**Track 3 (LP/Farkas)**",
			module:    "[`Track3.lean`](../formal/GoblinFormal/Track3.lean), `lp-check`",
			theorems:  "`checkPrimalFeasible_sound`, `safeDualBound_sound`, `checkLPOptimal_sound`, `checkFarkas_sound`, `checkBoundConflict_sound`",
			harness:   "[`certified.go`](../certified.go), [`formal_track3_test.go`](../formal_track3_test.go), [`formal_fuzz_bridge_test.go`](../formal_fuzz_bridge_test.go)",
		},
		"Track3B": {
			component: "**Track 3B (Rays/Postsolve)**",
			module:    "[`Track3B.lean`](../formal/GoblinFormal/Track3B.lean), `lp-check` (`unbounded`, `milp-unbounded`, `lp-presolve`), `rules-check` (`gcd-div-check`)",
			theorems:  "`checkLPUnbounded_sound`, `checkMILPUnbounded_sound`, `gcd_row_div_sound`, `evalGcdRowDiv_sound`, `lp_presolve_primal_lift`, `lp_presolve_chain_dual_sound`",
			harness:   "[`formal_track3b_test.go`](../formal_track3b_test.go), [`exact_polish.go`](../exact_polish.go), [`formal_export_test.go`](../formal_export_test.go)",
		},
		"Track4": {
			component: "**Track 4 (B&B/VIPR)**",
			module:    "[`Track4.lean`](../formal/GoblinFormal/Track4.lean), `vipr-check`",
			theorems:  "`bestBound_global_valid`, `exactGapClosedSpec_sound`, `checkOrbitAutomorphism_sound`, `runPresolveSteps_sound`, `checkMiniVIPR_sound`, `checkVIPRWithPresolve_optimal_same_n_sound`, `checkVIPRWithPresolve_eps_optimal_sound`",
			harness:   "[`certified.go`](../certified.go), [`vipr_trace.go`](../vipr_trace.go), [`vipr_trace_test.go`](../vipr_trace_test.go), [`certified_test.go`](../certified_test.go)",
		},
		"Track5A": {
			component: "**Track 5A (ARM64 Leaf)**",
			module:    "[`Track5A.lean`](../formal/GoblinFormal/Track5A.lean), `bin-check`",
			theorems:  "`splitmix64_arm64_equiv`, `canonicalFloatBits_arm64_equiv`, `divCeilInt64_arm64_equiv`, `gcdInt64_arm64_equiv`",
			harness:   "[`formal_track5_test.go`](../formal_track5_test.go), [`scripts/extract_elf_track5.py`](../scripts/extract_elf_track5.py)",
		},
		"Track5B": {
			component: "**Track 5B (ARM64 Kernel)**",
			module:    "[`Track5B.lean`](../formal/GoblinFormal/Track5B.lean), `bin-check`",
			theorems:  "`checkZeroGC_run_sound`, `checkStackGuard_sound`, `checkPanicBoundsReachability_run_safe`, `checkPanicBoundsReachability_run_unconditional`, `hasInfeasibleIntegerEqRow_elf_safe`, `augmentRowScalar_elf_safe`",
			harness:   "[`formal_track5_test.go`](../formal_track5_test.go), [`scripts/extract_elf_track5.py`](../scripts/extract_elf_track5.py)",
		},
		"Track5C": {
			component: "**Track 5C (SIMD CFG)**",
			module:    "[`Track5C.lean`](../formal/GoblinFormal/Track5C.lean), `simd-cfg-check`",
			theorems:  "`checkVectorCFG_sound`, `unguardedSucc_fallthroughGuarded_keeps_target`, `unguardedSucc_targetGuarded_keeps_fallthrough`, `unguardedSucc_ambiguous_keeps_both`",
			harness:   "[`formal_track5_test.go`](../formal_track5_test.go), [`cmd/goblin-optcheck/main.go`](../cmd/goblin-optcheck/main.go)",
		},
		"Track5D": {
			component: "**Track 5D (LSAP ELF/SIMD)**",
			module:    "[`Track5D.lean`](../formal/GoblinFormal/Track5D.lean), `bin-check`",
			theorems:  "`twoSumAssign_arm64_equiv`, `exactReducedCost_and_leaves_elf_safe`, `exactReducedCostNoinline_elf_safe`, `hasPotentialCancellation_elf_safe`, `vminpdLane_signed_zero_asymmetry`, `simdTree4Reduce_eq_scalarMin2Fold`, `simdTree8Reduce_eq_scalarMin2Fold`",
			harness:   "[`formal_track5d_test.go`](../formal_track5d_test.go), [`scripts/extract_elf_track5.py`](../scripts/extract_elf_track5.py), [`internal/simd/assign_test.go`](../internal/simd/assign_test.go)",
		},
		"Track5E": {
			component: "**Track 5E (AMD64 SIMD)**",
			module:    "[`Track5E.lean`](../formal/GoblinFormal/Track5E.lean)",
			theorems:  "`checkDecodedAbiSafety_sound`, `checkDecodedFrameAbi_sound`, `byteRangeDisjoint64_of_aligned_ne`, `allAbsLE64AVX2_aligned_byteDisjoint_equiv`, `allAbsLE64_avx2_eq_avx512_mult8`, `minNoNaN64_avx2_eq_avx512`, `initReducedRow64_avx2_eq_avx512_mult8`",
			harness:   "[`formal_track5e_test.go`](../formal_track5e_test.go), [`scripts/extract_amd64_track5e.py`](../scripts/extract_amd64_track5e.py)",
		},
		"Apps": {
			component: "**Apps (8 CLI Modules)**",
			module:    "[`Apps/Common.lean`](../formal/Apps/Common.lean) + `7` CLI executables",
			theorems:  "Private bounds lemma in `Apps/LsapCheck.lean`; all `8` `Apps.*` modules checked by `auditAllAppsModules`",
			harness:   "[`scripts/verify_formal.sh`](../scripts/verify_formal.sh), [`formal_oracle_pool_test.go`](../formal_oracle_pool_test.go)",
		},
	}

	b.WriteString("| Component | Module / App | Decls (`Auth / Pub / Kernel`, Locks; Kernel includes synthesized helpers) | Key Top-Level Theorems | Go Test & Oracle Harness |\n")
	b.WriteString("| :--- | :--- | :--- | :--- | :--- |\n")
	for _, mod := range inv.LeanModules {
		m := metaByGroup[mod.Name]
		fmt.Fprintf(b, "| %s | %s | `%s / %s / %s` (`%d`) | %s | %s |\n",
			m.component,
			m.module,
			formatCommaInt(mod.AuthoredDecls),
			formatCommaInt(mod.PublicDecls),
			formatCommaInt(mod.KernelDecls),
			mod.AxiomLocks,
			m.theorems,
			m.harness,
		)
	}
	fmt.Fprintf(b,
		"| **Total / Axiom Lock** | [`AxiomAudit.lean`](../formal/GoblinFormal/AxiomAudit.lean) (`%d` Lean files, `%s` LOC) | **`%s / %s / %s` (`%d`)** | `%s` authored `theorem`/`lemma` decls (`%s` in `GoblinFormal` + `%d` in `Apps/LsapCheck.lean`); `%s` kernel `TheoremInfo` entries include synthesized helpers; `%d` axiom locks + `#verify_all_goblin_axioms` + `auditAllAppsModules`; `0` `sorryAx` or custom axioms | Enforced by [`.github/workflows/ci.yml`](../.github/workflows/ci.yml), [`scripts/pre-commit.sh`](../scripts/pre-commit.sh), `lake build --wfail && lake lint` (`78/78` jobs), `auditAllAppsModules`, and `lake env leanchecker --fresh GoblinFormal` |\n",
		inv.LeanTotalFiles,
		formatCommaInt(inv.LeanTotalLOC),
		formatCommaInt(inv.LeanTotalAuthored),
		formatCommaInt(inv.LeanTotalPublic),
		formatCommaInt(inv.LeanTotalKernel),
		inv.AxiomLocks,
		formatCommaInt(inv.LeanTotalAuthored),
		formatCommaInt(inv.LeanGoblinAuthored),
		inv.LeanAppsAuthored,
		formatCommaInt(inv.LeanTotalKernel),
		inv.AxiomLocks,
	)
}

func renderFormalFuzzInventory(b *strings.Builder, inv *FormalInventory) {
	b.WriteString("| Build Profile | Binary (`targets.tsv`) | Package | Targets | Execution Scope |\n")
	b.WriteString("| :--- | :--- | :--- | ---: | :--- |\n")
	rows := []struct {
		profile string
		bin     string
		pkg     string
		scope   string
	}{
		{"Default (`GOBLIN_FORMAL=1` + Pure Go)", "`assign.test`", "`./assign`", "LSAP `--jsonl` differential oracles (`lsap-check`), dyadic2304, and `assign.WithCertified`"},
		{"Default (`GOBLIN_FORMAL=1` + Pure Go)", "`goblin.test`", "`.`", "LP/MILP/MAD `--jsonl` differential oracles (`lp-check`, `rules-check`, `cuts-check`, `vipr-check`), `WithCertified`, `VerifyVIPRTrace`, LU, and `PuLP` JSON"},
		{"Assert (`-tags=goblin_assert`)", "`goblin_assert.test`", "`.`", "Structural, public-corpus mutation, metamorphic, presolve parity, and incremental LP state checks"},
		{"Race (`-race`)", "`goblin_race.test`", "`.`", "Parallel Branch-and-Bound race detection (`FuzzParallelBABRace`)"},
		{"Default Parser", "`format.test`", "`./format`", "Streaming `PuLP` JSON parity and MPS round-trip invariance"},
		{"Default SIMD", "`simd.test`", "`./internal/simd`", "Scalar vs. AVX2 vs. AVX-512 tier bit-parity (`FuzzSIMDKernelTierParity`)"},
		{"Optional CBC (`-tags=cbc`)", "`goblin_cbc.test`", "`.`", "Pure-Go vs. COIN-OR CBC differential LP/MILP/MAD and incremental LP checks"},
		{"Optional CBC (`-tags=cbc`)", "`format_cbc.test`", "`./format`", "MPS export parity against COIN-OR CBC (`FuzzWriteMPSCBCParity`)"},
	}
	for _, r := range rows {
		binKey := strings.Trim(r.bin, "`")
		cnt := inv.FuzzBinaryCounts[binKey]
		fmt.Fprintf(b, "| %s | %s | %s | `%d` | %s |\n", r.profile, r.bin, r.pkg, cnt, r.scope)
	}
	fmt.Fprintf(b, "| **Total Active Fuzz Targets** | **`targets.tsv` (`%d` rows)** | **`4` Go packages** | **`%d`** | **`%d` default-build (`go test -run '^Fuzz' ./...`) + `%d` `-tags=cbc`** |\n",
		inv.TSVActiveRowCount,
		inv.TotalFuzzCount,
		inv.DefaultFuzzCount,
		inv.CBCFuzzCount,
	)
}

// SyncFormalDocs updates or verifies formal verification and fuzz inventory blocks
// across README.md, docs/index.md, docs/benchmarks.md, and docs/formal-verification.md.
func SyncFormalDocs(root string, checkOnly bool) error {
	inv, err := CollectFormalInventory(root)
	if err != nil {
		return err
	}
	var tsBundle *TargetSiliconBundle
	tsDir := filepath.Join(root, "bench", "results", "target_silicon")
	if _, statErr := os.Stat(filepath.Join(tsDir, "amd64_avx512.json")); statErr == nil {
		loadedTS, tsErr := LoadTargetSiliconBundle(tsDir)
		if tsErr != nil {
			return tsErr
		}
		tsBundle = loadedTS
	}
	var dc docgenCorpus
	dc.repoRoot = root
	dc.formalInv = inv
	dc.targetSilicon = tsBundle
	docs := []string{
		"README.md",
		filepath.Join("docs", "index.md"),
		filepath.Join("docs", "benchmarks.md"),
		filepath.Join("docs", "formal-verification.md"),
	}
	for _, rel := range docs {
		path := filepath.Join(root, rel)
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		orig := string(raw)
		synced, replaced, repErr := replaceGeneratedBlocks(
			orig,
			func(blockName string) bool {
				if formalGeneratedBlocks[blockName] {
					return true
				}
				return blockName == "TARGET_SILICON_SUMMARY" && tsBundle != nil
			},
			func(blockName string) (string, error) {
				if blockName == "TARGET_SILICON_SUMMARY" {
					var tbBuf bytes.Buffer
					if renderErr := renderTargetSiliconSummary(&tbBuf, dc.milp(), dc.assign(), tsBundle); renderErr != nil {
						return "", renderErr
					}
					return strings.TrimRight(tbBuf.String(), "\n"), nil
				}
				return renderFormalBlock(blockName, inv)
			},
		)
		if repErr != nil {
			return fmt.Errorf("%s: %w", rel, repErr)
		}
		if replaced == 0 {
			return fmt.Errorf("%s has no formal generated block markers", rel)
		}
		if checkOnly {
			if orig != synced {
				return fmt.Errorf("%s formal inventory block is out of sync with repository sources", rel)
			}
			continue
		}
		if orig != synced {
			if writeErr := os.WriteFile(path, []byte(synced), syncedDocPerm); writeErr != nil {
				return writeErr
			}
		}
	}
	return nil
}

// TargetSiliconTestRun records one executed test configuration on a target-silicon instance.
type TargetSiliconTestRun struct {
	Profile             string `json:"profile"`
	ArchLevel           string `json:"arch_level"`
	SIMDEnabled         bool   `json:"simd_enabled"`
	SIMDMaxTier         string `json:"simd_max_tier"`
	ActiveSIMDTier      string `json:"active_simd_tier"`
	PackagesPassed      int    `json:"packages_passed"`
	ExplicitTestsPassed int    `json:"explicit_tests_passed"`
	ElapsedSec          int    `json:"elapsed_sec"`
}

// TargetSiliconTestSummary summarizes test matrix execution across packages and ISA tiers.
type TargetSiliconTestSummary struct {
	ProfilesVerified          int                    `json:"profiles_verified"`
	GOAMD64Levels             []string               `json:"goamd64_levels,omitempty"`
	SIMDMaxTiers              []string               `json:"simd_max_tiers,omitempty"`
	Packages                  []string               `json:"packages"`
	ExplicitFormalAndFMATests []string               `json:"explicit_formal_and_fma_tests"`
	TotalFailures             int                    `json:"total_failures"`
	Runs                      []TargetSiliconTestRun `json:"runs"`
}

// TargetSiliconKernelEntry stores 6-round interleaved AB/BA nanosecond timings for one kernel and size.
type TargetSiliconKernelEntry struct {
	Kernel         string    `json:"kernel"`
	Size           int       `json:"size"`
	ScalarRoundsNS []float64 `json:"scalar_rounds_ns"`
	AVX2RoundsNS   []float64 `json:"avx2_rounds_ns"`
	AVX512RoundsNS []float64 `json:"avx512_rounds_ns"`
}

// TargetSiliconKernelABBA holds interleaved vector kernel timings from simd.test on AVX-512 silicon.
type TargetSiliconKernelABBA struct {
	Rounds    int                        `json:"rounds"`
	OrderOdd  []string                   `json:"order_odd"`
	OrderEven []string                   `json:"order_even"`
	Entries   []TargetSiliconKernelEntry `json:"entries"`
}

// TargetSiliconMILPEntry stores per-family LP/MILP solve and total trial timings on target silicon.
type TargetSiliconMILPEntry struct {
	Tier           string    `json:"tier"`
	NumVars        int       `json:"num_vars"`
	NumConstraints int       `json:"num_constraints"`
	Status         string    `json:"status"`
	Objective      float64   `json:"objective"`
	RootLPObj      float64   `json:"root_lp_obj"`
	TSolveMS       float64   `json:"t_solve_ms"`
	TTotalMS       float64   `json:"t_total_ms"`
	TSolveTrialsMS []float64 `json:"t_solve_trials_ms"`
	TTotalTrialsMS []float64 `json:"t_total_trials_ms"`
}

// TargetSiliconAssignEntry stores per-family LSAP solve and total trial timings on target silicon.
type TargetSiliconAssignEntry struct {
	Tier           string    `json:"tier"`
	Rows           int       `json:"rows"`
	Cols           int       `json:"cols"`
	Family         string    `json:"family"`
	Objective      float64   `json:"objective"`
	TSolveMS       float64   `json:"t_solve_ms"`
	TTotalMS       float64   `json:"t_total_ms"`
	TSolveTrialsMS []float64 `json:"t_solve_trials_ms"`
	TTotalTrialsMS []float64 `json:"t_total_trials_ms"`
}

// TargetSiliconAMD64MILPCorpus holds 31-family LP/MILP telemetry across scalar, AVX2, and AVX-512 tiers.
type TargetSiliconAMD64MILPCorpus struct {
	Families         int                               `json:"families"`
	Trials           int                               `json:"trials"`
	ParityMismatches int                               `json:"parity_mismatches"`
	DefaultScalar    map[string]TargetSiliconMILPEntry `json:"default_scalar"`
	SIMDAVX2         map[string]TargetSiliconMILPEntry `json:"simd_avx2"`
	SIMDAVX512       map[string]TargetSiliconMILPEntry `json:"simd_avx512"`
}

// TargetSiliconAMD64AssignCorpus holds 22-family LSAP telemetry across scalar, AVX2, and AVX-512 tiers.
type TargetSiliconAMD64AssignCorpus struct {
	Families         int                                 `json:"families"`
	Trials           int                                 `json:"trials"`
	ParityMismatches int                                 `json:"parity_mismatches"`
	DefaultScalar    map[string]TargetSiliconAssignEntry `json:"default_scalar"`
	SIMDAVX2         map[string]TargetSiliconAssignEntry `json:"simd_avx2"`
	SIMDAVX512       map[string]TargetSiliconAssignEntry `json:"simd_avx512"`
}

// TargetSiliconAMD64Report captures sanitized hardware telemetry from an n4-standard-4 AVX-512 run.
type TargetSiliconAMD64Report struct {
	SchemaVersion      int                            `json:"schema_version"`
	PlatformID         string                         `json:"platform_id"`
	MachineType        string                         `json:"machine_type"`
	OSImageFamily      string                         `json:"os_image_family"`
	BootDiskType       string                         `json:"boot_disk_type"`
	GOOS               string                         `json:"goos"`
	GOARCH             string                         `json:"goarch"`
	UnameSRM           string                         `json:"uname_srm"`
	CPUModel           string                         `json:"cpu_model"`
	VCPUs              int                            `json:"vcpus"`
	CoresPerSocket     int                            `json:"cores_per_socket"`
	ThreadsPerCore     int                            `json:"threads_per_core"`
	LSCPUSummary       map[string]string              `json:"lscpu_summary"`
	AVX512Flags        []string                       `json:"avx512_flags"`
	HardwareSIMDTier   string                         `json:"hardware_simd_tier"`
	ActiveSIMDTier     string                         `json:"active_simd_tier"`
	GitCommit          string                         `json:"git_commit"`
	SourceFingerprint  string                         `json:"source_fingerprint"`
	DefaultGoVersion   string                         `json:"default_go_version"`
	SIMDGoVersion      string                         `json:"simd_go_version"`
	CPUAffinity        string                         `json:"cpu_affinity"`
	PreTestCPUBusyPct  float64                        `json:"pre_test_cpu_busy_pct"`
	PreBenchCPUBusyPct float64                        `json:"pre_bench_cpu_busy_pct"`
	MeasuredAt         string                         `json:"measured_at"`
	TestVerification   TargetSiliconTestSummary       `json:"test_verification"`
	KernelABBA         TargetSiliconKernelABBA        `json:"kernel_abba"`
	MILPCorpus         TargetSiliconAMD64MILPCorpus   `json:"milp_corpus"`
	AssignCorpus       TargetSiliconAMD64AssignCorpus `json:"assign_corpus"`
}

// TargetSiliconARM64MILPCorpus holds 31-family LP/MILP telemetry on linux/arm64.
type TargetSiliconARM64MILPCorpus struct {
	Families         int                               `json:"families"`
	Trials           int                               `json:"trials"`
	ParityMismatches int                               `json:"parity_mismatches"`
	Default          map[string]TargetSiliconMILPEntry `json:"default"`
}

// TargetSiliconARM64AssignCorpus holds 22-family LSAP telemetry on linux/arm64.
type TargetSiliconARM64AssignCorpus struct {
	Families         int                                 `json:"families"`
	Trials           int                                 `json:"trials"`
	ParityMismatches int                                 `json:"parity_mismatches"`
	Default          map[string]TargetSiliconAssignEntry `json:"default"`
}

// TargetSiliconARM64Report captures sanitized hardware telemetry from a linux/arm64 GCE run.
type TargetSiliconARM64Report struct {
	SchemaVersion              int                            `json:"schema_version"`
	PlatformID                 string                         `json:"platform_id"`
	MachineType                string                         `json:"machine_type"`
	OSImageFamily              string                         `json:"os_image_family"`
	BootDiskType               string                         `json:"boot_disk_type"`
	GOOS                       string                         `json:"goos"`
	GOARCH                     string                         `json:"goarch"`
	GOARM64                    string                         `json:"goarm64"`
	UnameSRM                   string                         `json:"uname_srm"`
	CPUModel                   string                         `json:"cpu_model"`
	VCPUs                      int                            `json:"vcpus"`
	CoresPerSocket             int                            `json:"cores_per_socket"`
	ThreadsPerCore             int                            `json:"threads_per_core"`
	LSCPUSummary               map[string]string              `json:"lscpu_summary"`
	CPUFeatures                []string                       `json:"cpu_features"`
	HardwareSIMDTier           string                         `json:"hardware_simd_tier"`
	ActiveSIMDTier             string                         `json:"active_simd_tier"`
	GoExperimentSIMDCompiles   bool                           `json:"goexperiment_simd_compiles"`
	GoExperimentSIMDSuitesPass bool                           `json:"goexperiment_simd_suites_passed"`
	GitCommit                  string                         `json:"git_commit"`
	SourceFingerprint          string                         `json:"source_fingerprint"`
	DefaultGoVersion           string                         `json:"default_go_version"`
	SIMDGoVersion              string                         `json:"simd_go_version,omitempty"`
	CPUAffinity                string                         `json:"cpu_affinity"`
	PreTestCPUBusyPct          float64                        `json:"pre_test_cpu_busy_pct"`
	PreBenchCPUBusyPct         float64                        `json:"pre_bench_cpu_busy_pct"`
	MeasuredAt                 string                         `json:"measured_at"`
	TestVerification           TargetSiliconTestSummary       `json:"test_verification"`
	MILPCorpus                 TargetSiliconARM64MILPCorpus   `json:"milp_corpus"`
	AssignCorpus               TargetSiliconARM64AssignCorpus `json:"assign_corpus"`
}

// TargetSiliconBundle pairs sanitized amd64 AVX-512 and linux/arm64 target-silicon telemetry.
type TargetSiliconBundle struct {
	AMD64 TargetSiliconAMD64Report
	ARM64 TargetSiliconARM64Report
}

var (
	reHygieneEmail = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	reHygieneIPv4  = regexp.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`)
	reHygieneZone  = regexp.MustCompile(`\b(?:us|europe|asia|australia|northamerica|southamerica|me|africa)-[a-z]+[0-9](?:-[a-z])?\b`)
)

// ValidateTargetSiliconHygiene rejects strings containing private cloud identifiers, emails,
// IPv4 addresses, GCP regions/zones, deprecated storage CLI names, or local user home paths.
func ValidateTargetSiliconHygiene(label, raw string, allowInstancePrefix bool) error {
	if m := reHygieneEmail.FindString(raw); m != "" {
		return fmt.Errorf("%s contains forbidden email pattern %q", label, m)
	}
	if m := reHygieneIPv4.FindString(raw); m != "" && m != "0.0."+"0.0" {
		return fmt.Errorf("%s contains forbidden IPv4 literal %q", label, m)
	}
	if m := reHygieneZone.FindString(raw); m != "" {
		return fmt.Errorf("%s contains forbidden GCP region/zone literal %q", label, m)
	}
	forbiddenTokens := []string{
		"/" + "ho" + "me/",
		"/" + "Users/",
		"gs" + "util",
		"gs:" + "//",
		"compute." + "internal",
		"gserviceaccount." + "com",
	}
	if !allowInstancePrefix {
		forbiddenTokens = append(forbiddenTokens, "goblin-"+"silicon-")
	}
	for _, tok := range forbiddenTokens {
		if strings.Contains(raw, tok) {
			return fmt.Errorf("%s contains forbidden token %q", label, tok)
		}
	}
	return nil
}

// LoadTargetSiliconBundle reads and hygiene-checks amd64_avx512.json and linux_arm64.json from dir.
func LoadTargetSiliconBundle(dir string) (*TargetSiliconBundle, error) {
	amd64Path := filepath.Join(dir, "amd64_avx512.json")
	rawAMD64, err := os.ReadFile(amd64Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", amd64Path, err)
	}
	if err := ValidateTargetSiliconHygiene(amd64Path, string(rawAMD64), false); err != nil {
		return nil, err
	}
	var amd64Rep TargetSiliconAMD64Report
	if err := json.Unmarshal(rawAMD64, &amd64Rep); err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", amd64Path, err)
	}

	arm64Path := filepath.Join(dir, "linux_arm64.json")
	rawARM64, err := os.ReadFile(arm64Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", arm64Path, err)
	}
	if err := ValidateTargetSiliconHygiene(arm64Path, string(rawARM64), false); err != nil {
		return nil, err
	}
	var arm64Rep TargetSiliconARM64Report
	if err := json.Unmarshal(rawARM64, &arm64Rep); err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", arm64Path, err)
	}

	return &TargetSiliconBundle{
		AMD64: amd64Rep,
		ARM64: arm64Rep,
	}, nil
}

var canonicalTargetSiliconKernels = []string{
	"AllAbsLE64",
	"MinNoNaN64",
	"InitReducedRow64",
	"Min2ReducedRow64",
	"RelaxRow64",
	"Dot64",
	"Axpy64",
	"SelectDualLeaveRow64",
	"ScanDualRatioCand64",
}

func validateTargetSiliconMILPMap(label string, cases []CorpusCase, m map[string]TargetSiliconMILPEntry) error {
	if len(m) != len(cases) {
		return fmt.Errorf("%s has %d MILP families, want %d", label, len(m), len(cases))
	}
	for _, c := range cases {
		e, ok := m[c.ID]
		if !ok {
			return fmt.Errorf("%s missing MILP family %q", label, c.ID)
		}
		if e.TSolveMS <= 0 || e.TTotalMS <= 0 || len(e.TSolveTrialsMS) == 0 || len(e.TTotalTrialsMS) != len(e.TSolveTrialsMS) {
			return fmt.Errorf("%s family %q has invalid trial timings", label, c.ID)
		}
	}
	return nil
}

func validateTargetSiliconAssignMap(label string, assignCases []AssignCorpusCase, m map[string]TargetSiliconAssignEntry) error {
	if len(m) != len(assignCases) {
		return fmt.Errorf("%s has %d LSAP families, want %d", label, len(m), len(assignCases))
	}
	for _, ac := range assignCases {
		e, ok := m[ac.ID]
		if !ok {
			return fmt.Errorf("%s missing LSAP family %q", label, ac.ID)
		}
		if e.Rows != ac.Rows || e.Cols != ac.Cols {
			return fmt.Errorf("%s family %q shape (%d,%d) != (%d,%d)", label, ac.ID, e.Rows, e.Cols, ac.Rows, ac.Cols)
		}
		if e.TSolveMS <= 0 || e.TTotalMS <= 0 || len(e.TSolveTrialsMS) == 0 || len(e.TTotalTrialsMS) != len(e.TSolveTrialsMS) {
			return fmt.Errorf("%s family %q has invalid trial timings", label, ac.ID)
		}
	}
	return nil
}

// ValidateTargetSiliconTelemetry verifies structural invariants, hardware tier detection,
// quiescence bounds, 6-round kernel AB/BA completeness, and corpus parity across both reports.
func ValidateTargetSiliconTelemetry(cases []CorpusCase, assignCases []AssignCorpusCase, ts *TargetSiliconBundle) error {
	if ts == nil {
		return errors.New("nil target-silicon telemetry bundle")
	}
	a := &ts.AMD64
	if a.SchemaVersion != 1 || a.PlatformID != "linux_amd64_avx512" || a.MachineType != "n4-standard-4" || a.GOARCH != "amd64" {
		return fmt.Errorf("amd64_avx512 metadata mismatch: schema=%d platform=%q machine=%q arch=%q",
			a.SchemaVersion, a.PlatformID, a.MachineType, a.GOARCH)
	}
	if a.HardwareSIMDTier != "avx512" || a.ActiveSIMDTier != "avx512" {
		return fmt.Errorf("amd64_avx512 expected hardware/active tier avx512, got %q/%q", a.HardwareSIMDTier, a.ActiveSIMDTier)
	}
	if len(a.GitCommit) < 7 || len(a.SourceFingerprint) != 16 || a.CPUModel == "" || a.UnameSRM == "" {
		return errors.New("amd64_avx512 missing provenance or hardware identity fields")
	}
	for _, reqFlag := range []string{"avx512f", "avx512dq", "avx512bw", "avx512vl"} {
		if !slices.Contains(a.AVX512Flags, reqFlag) {
			return fmt.Errorf("amd64_avx512 missing required CPU flag %q", reqFlag)
		}
	}
	if a.PreBenchCPUBusyPct >= 5.0 || a.PreBenchCPUBusyPct < 0 {
		return fmt.Errorf("amd64_avx512 pre_bench_cpu_busy_pct %.2f outside [0, 5.0)", a.PreBenchCPUBusyPct)
	}
	if a.TestVerification.TotalFailures != 0 || a.TestVerification.ProfilesVerified < 10 || len(a.TestVerification.Runs) != a.TestVerification.ProfilesVerified {
		return fmt.Errorf("amd64_avx512 invalid test verification summary: failures=%d profiles=%d",
			a.TestVerification.TotalFailures, a.TestVerification.ProfilesVerified)
	}
	for _, r := range a.TestVerification.Runs {
		if r.PackagesPassed != 4 || r.ExplicitTestsPassed != 5 {
			return fmt.Errorf("amd64_avx512 profile %q passed (%d pkgs, %d explicit), want (4, 5)",
				r.Profile, r.PackagesPassed, r.ExplicitTestsPassed)
		}
	}
	if a.KernelABBA.Rounds != 6 {
		return fmt.Errorf("amd64_avx512 kernel_abba rounds=%d, want 6", a.KernelABBA.Rounds)
	}
	seenKernels := make(map[string]bool, len(canonicalTargetSiliconKernels))
	for _, ke := range a.KernelABBA.Entries {
		if len(ke.ScalarRoundsNS) != 6 || len(ke.AVX2RoundsNS) != 6 || len(ke.AVX512RoundsNS) != 6 {
			return fmt.Errorf("amd64_avx512 kernel %s N=%d does not carry 6 rounds per tier", ke.Kernel, ke.Size)
		}
		seenKernels[ke.Kernel] = true
	}
	for _, wantK := range canonicalTargetSiliconKernels {
		if !seenKernels[wantK] {
			return fmt.Errorf("amd64_avx512 kernel_abba missing kernel %q", wantK)
		}
	}
	if a.MILPCorpus.ParityMismatches != 0 || a.AssignCorpus.ParityMismatches != 0 {
		return fmt.Errorf("amd64_avx512 parity mismatches: milp=%d assign=%d", a.MILPCorpus.ParityMismatches, a.AssignCorpus.ParityMismatches)
	}
	for _, sub := range []struct {
		lbl string
		m   map[string]TargetSiliconMILPEntry
	}{
		{"amd64.milp.default_scalar", a.MILPCorpus.DefaultScalar},
		{"amd64.milp.simd_avx2", a.MILPCorpus.SIMDAVX2},
		{"amd64.milp.simd_avx512", a.MILPCorpus.SIMDAVX512},
	} {
		if err := validateTargetSiliconMILPMap(sub.lbl, cases, sub.m); err != nil {
			return err
		}
	}
	for _, sub := range []struct {
		lbl string
		m   map[string]TargetSiliconAssignEntry
	}{
		{"amd64.assign.default_scalar", a.AssignCorpus.DefaultScalar},
		{"amd64.assign.simd_avx2", a.AssignCorpus.SIMDAVX2},
		{"amd64.assign.simd_avx512", a.AssignCorpus.SIMDAVX512},
	} {
		if err := validateTargetSiliconAssignMap(sub.lbl, assignCases, sub.m); err != nil {
			return err
		}
	}

	r := &ts.ARM64
	if r.SchemaVersion != 1 || r.PlatformID != "linux_arm64" || r.GOARCH != "arm64" {
		return fmt.Errorf("linux_arm64 metadata mismatch: schema=%d platform=%q arch=%q",
			r.SchemaVersion, r.PlatformID, r.GOARCH)
	}
	if r.MachineType != "c4a-standard-4" && r.MachineType != "t2a-standard-4" {
		return fmt.Errorf("linux_arm64 unexpected machine_type %q", r.MachineType)
	}
	if r.HardwareSIMDTier != "scalar" || r.ActiveSIMDTier != "scalar" {
		return fmt.Errorf("linux_arm64 expected scalar tier, got %q/%q", r.HardwareSIMDTier, r.ActiveSIMDTier)
	}
	if len(r.GitCommit) < 7 || len(r.SourceFingerprint) != 16 || r.CPUModel == "" || r.UnameSRM == "" {
		return errors.New("linux_arm64 missing provenance or hardware identity fields")
	}
	if r.PreBenchCPUBusyPct >= 5.0 || r.PreBenchCPUBusyPct < 0 {
		return fmt.Errorf("linux_arm64 pre_bench_cpu_busy_pct %.2f outside [0, 5.0)", r.PreBenchCPUBusyPct)
	}
	if r.TestVerification.TotalFailures != 0 || r.TestVerification.ProfilesVerified < 1 || len(r.TestVerification.Runs) != r.TestVerification.ProfilesVerified {
		return fmt.Errorf("linux_arm64 invalid test verification summary: failures=%d profiles=%d",
			r.TestVerification.TotalFailures, r.TestVerification.ProfilesVerified)
	}
	if r.GoExperimentSIMDCompiles && !r.GoExperimentSIMDSuitesPass {
		return errors.New("linux_arm64 compiled GOEXPERIMENT=simd but did not pass GOEXPERIMENT=simd test suites")
	}
	if r.MILPCorpus.ParityMismatches != 0 || r.AssignCorpus.ParityMismatches != 0 {
		return fmt.Errorf("linux_arm64 parity mismatches: milp=%d assign=%d", r.MILPCorpus.ParityMismatches, r.AssignCorpus.ParityMismatches)
	}
	if err := validateTargetSiliconMILPMap("arm64.milp.default", cases, r.MILPCorpus.Default); err != nil {
		return err
	}
	if err := validateTargetSiliconAssignMap("arm64.assign.default", assignCases, r.AssignCorpus.Default); err != nil {
		return err
	}
	return nil
}

func summarizeTargetSubset[C any](
	items []C,
	filter func(C) bool,
	trialsOf func(C) (sc, v2, v4, ar []float64),
) (nFams, nTrials int, scSum, v2Sum, v4Sum, armSum, gmScV4, loScV4, hiScV4, gmV2V4, loV2V4, hiV2V4 float64) {
	var famScV4, famV2V4 []float64
	for _, item := range items {
		if !filter(item) {
			continue
		}
		nFams++
		sc, v2, v4, ar := trialsOf(item)
		n := min(min(len(sc), len(v2)), min(len(v4), len(ar)))
		nTrials += n
		for k := range n {
			scSum += sc[k]
			v2Sum += v2[k]
			v4Sum += v4[k]
			armSum += ar[k]
		}
		if fgm, _, _, _ := pairedRatio(sc[:n], v4[:n]); !math.IsNaN(fgm) {
			famScV4 = append(famScV4, fgm)
		}
		if fgm2, _, _, _ := pairedRatio(v2[:n], v4[:n]); !math.IsNaN(fgm2) {
			famV2V4 = append(famV2V4, fgm2)
		}
	}
	gmScV4, loScV4, hiScV4 = familyClusteredRatio(famScV4)
	gmV2V4, loV2V4, hiV2V4 = familyClusteredRatio(famV2V4)
	return
}

func formatKernelNS(ns float64) string {
	if ns < 10.0 {
		return fmt.Sprintf("%.2f", ns)
	}
	if ns < 100.0 {
		return fmt.Sprintf("%.1f", ns)
	}
	return fmt.Sprintf("%.0f", ns)
}

func renderTargetSiliconSummary(b *bytes.Buffer, cases []CorpusCase, assignCases []AssignCorpusCase, ts *TargetSiliconBundle) error {
	if err := ValidateTargetSiliconTelemetry(cases, assignCases, ts); err != nil {
		return err
	}
	a := &ts.AMD64
	r := &ts.ARM64
	shortCommitA := a.GitCommit[:min(7, len(a.GitCommit))]
	shortCommitR := r.GitCommit[:min(7, len(r.GitCommit))]

	fmt.Fprintf(b,
		"- **Native `linux/amd64` AVX-512 host (`bench/results/target_silicon/amd64_avx512.json`)**: GCE `%s` (`%s`, `%s`, `%s`, `%d` vCPUs, `%s`, pinned CPU `%s`, pre-benchmark host load `%.2f%%`), `%d` `avx512*` ISA flags (`avx512f`, `avx512dq`, `avx512bw`, `avx512vl`, `avx512_vbmi2`, `avx512_vpopcntdq`), `simd.HardwareSIMDTier() == %q`, `simd.ActiveSIMDTier() == %q`, fingerprint `%s` (commit `%s`), Go `%s` / `%s`. Passed `%d/%d` test configurations (`0` failures across `.`, `./internal/simd`, `./assign`, and `./format` for default `GOAMD64=v1` and `GOEXPERIMENT=simd` `GOAMD64=v1`, `v3`, `v4` across `GOBLIN_SIMD_MAX_TIER={scalar,avx2,avx512}`, including explicit `TestFormalTrack5D_SIMDParityAndEdgeAudit`, `TestFormalTrack5E_LiveSIMDDifferential`, `TestFormalTrack5E_ZeroAllocContracts`, `TestFormalTrack5E_HardwareNaNParity`, and `TestTwoSumTwoProdFMASafety`).\n",
		a.MachineType, a.OSImageFamily, a.BootDiskType, a.CPUModel, a.VCPUs, a.UnameSRM, a.CPUAffinity, a.PreBenchCPUBusyPct,
		len(a.AVX512Flags), a.HardwareSIMDTier, a.ActiveSIMDTier, a.SourceFingerprint, shortCommitA, a.DefaultGoVersion, a.SIMDGoVersion,
		a.TestVerification.ProfilesVerified, a.TestVerification.ProfilesVerified,
	)

	armSIMDNote := "default scalar build verified"
	if r.GoExperimentSIMDCompiles && r.GoExperimentSIMDSuitesPass {
		armSIMDNote = fmt.Sprintf("both default (`%s`) and `GOEXPERIMENT=simd` (`%s`) compile and pass all suites cleanly on `linux/arm64`, dispatching `!amd64` 4-way unrolled scalar kernels (`HardwareSIMDTier() == %q`, `ActiveSIMDTier() == %q`)",
			r.DefaultGoVersion, r.SIMDGoVersion, r.HardwareSIMDTier, r.ActiveSIMDTier)
	}
	fmt.Fprintf(b,
		"- **Native `linux/arm64` host (`bench/results/target_silicon/linux_arm64.json`)**: GCE `%s` (`%s`, `%s`, `%s`, `%d` vCPUs, `%s`, `GOARM64=%s`, pinned CPU `%s`, pre-benchmark host load `%.2f%%`), fingerprint `%s` (commit `%s`), `%d/%d` test configurations passed (`0` failures across `.`, `./internal/simd`, `./assign`, and `./format`, including hardware FMA non-contraction in `TestTwoSumTwoProdFMASafety` and Track 5D/5E IEEE-754 parity; %s).\n\n",
		r.MachineType, r.OSImageFamily, r.BootDiskType, r.CPUModel, r.VCPUs, r.UnameSRM, r.GOARM64, r.CPUAffinity, r.PreBenchCPUBusyPct,
		r.SourceFingerprint, shortCommitR, r.TestVerification.ProfilesVerified, r.TestVerification.ProfilesVerified, armSIMDNote,
	)

	type kernelKey struct {
		name string
		size int
	}
	byKey := make(map[kernelKey]TargetSiliconKernelEntry, len(a.KernelABBA.Entries))
	for _, ke := range a.KernelABBA.Entries {
		byKey[kernelKey{ke.Kernel, ke.Size}] = ke
	}
	selectedKernels := []kernelKey{
		{"AllAbsLE64", 64}, {"AllAbsLE64", 1024},
		{"MinNoNaN64", 64}, {"MinNoNaN64", 1024},
		{"InitReducedRow64", 64}, {"InitReducedRow64", 1024},
		{"Min2ReducedRow64", 128}, {"Min2ReducedRow64", 1024},
		{"RelaxRow64", 64}, {"RelaxRow64", 1024},
		{"Dot64", 256}, {"Dot64", 2048},
		{"Axpy64", 256}, {"Axpy64", 2048},
		{"SelectDualLeaveRow64", 256}, {"SelectDualLeaveRow64", 2048},
		{"ScanDualRatioCand64", 1024}, {"ScanDualRatioCand64", 8192},
	}

	b.WriteString("#### Emerald Rapids 6-Round Interleaved AB/BA Vector Kernel Crossover (`simd.test`, CPU 1)\n\n")
	b.WriteString("| Kernel | $N$ | Scalar Med (ns/op) | AVX2 Med (ns/op) | AVX-512 Med (ns/op) | Paired `Scalar / AVX2` [95% CI] | Paired `Scalar / AVX-512` [95% CI] | Paired `AVX2 / AVX-512` [95% CI] |\n")
	b.WriteString("| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, sk := range selectedKernels {
		ke, ok := byKey[sk]
		if !ok {
			return fmt.Errorf("target-silicon kernel table missing %s N=%d", sk.name, sk.size)
		}
		scMed, _, _ := medianMinMax(ke.ScalarRoundsNS)
		v2Med, _, _ := medianMinMax(ke.AVX2RoundsNS)
		v4Med, _, _ := medianMinMax(ke.AVX512RoundsNS)
		gm12, lo12, hi12, _ := pairedRatio(ke.ScalarRoundsNS, ke.AVX2RoundsNS)
		gm14, lo14, hi14, _ := pairedRatio(ke.ScalarRoundsNS, ke.AVX512RoundsNS)
		gm24, lo24, hi24, _ := pairedRatio(ke.AVX2RoundsNS, ke.AVX512RoundsNS)
		fmt.Fprintf(b, "| `%s` | %d | %s | %s | %s | %.2fx [%.2fx-%.2fx] | **%.2fx** [%.2fx-%.2fx] | %.2fx [%.2fx-%.2fx] |\n",
			sk.name, sk.size,
			formatKernelNS(scMed), formatKernelNS(v2Med), formatKernelNS(v4Med),
			gm12, lo12, hi12,
			gm14, lo14, hi14,
			gm24, lo24, hi24)
	}

	b.WriteString("\n#### Target-Silicon End-to-End LP/MILP and LSAP Corpus Summary (`goblin-bench`, CPU 1)\n\n")
	b.WriteString("| Workload Subset | Families (Trials) | `n4` Scalar Solve Sum (ms) | `n4` AVX2 Solve Sum (ms) | `n4` AVX-512 Solve Sum (ms) | `n4` Equal-Family `Scalar / AVX-512` [95% CI] | `n4` Equal-Family `AVX2 / AVX-512` [95% CI] | `arm64` Default Solve Sum (ms) | Parity |\n")
	b.WriteString("| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |\n")

	milpRows := []subsetSpec[CorpusCase]{
		{"LP/MILP Tier 2+3 non-planted (solver-dominated)", func(c CorpusCase) bool {
			return c.Tier != "tier1_canonical" && !isPlantedMAD(c.ID)
		}},
		{"LP/MILP All 31 synthetic families", func(_ CorpusCase) bool { return true }},
	}
	for _, mr := range milpRows {
		nF, nT, scS, v2S, v4S, armS, gmSV4, loSV4, hiSV4, gm24, lo24, hi24 := summarizeTargetSubset(
			cases, mr.filter,
			func(c CorpusCase) (sc, v2, v4, ar []float64) {
				return a.MILPCorpus.DefaultScalar[c.ID].TSolveTrialsMS,
					a.MILPCorpus.SIMDAVX2[c.ID].TSolveTrialsMS,
					a.MILPCorpus.SIMDAVX512[c.ID].TSolveTrialsMS,
					r.MILPCorpus.Default[c.ID].TSolveTrialsMS
			},
		)
		fmt.Fprintf(b, "| %s | %d (%d) | %.1f | %.1f | %.1f | **%.2fx** [%.2fx-%.2fx] | %.2fx [%.2fx-%.2fx] | %.1f | `OK` |\n",
			mr.label, nF, nT, scS, v2S, v4S, gmSV4, loSV4, hiSV4, gm24, lo24, hi24, armS)
	}

	assignRows := []subsetSpec[AssignCorpusCase]{
		{"LSAP Tier 1: Micro/Small (`N <= 32`)", func(c AssignCorpusCase) bool { return c.Tier == "tier1_small" }},
		{"LSAP Tier 2: Medium (`64 <= N <= 256`)", func(c AssignCorpusCase) bool { return c.Tier == "tier2_medium" }},
		{"LSAP Tier 3: Large (`500 <= N <= 2000`)", func(c AssignCorpusCase) bool { return c.Tier == "tier3_large" }},
		{"**LSAP All 22 synthetic families**", func(_ AssignCorpusCase) bool { return true }},
	}
	for _, ar := range assignRows {
		nF, nT, scS, v2S, v4S, armS, gmSV4, loSV4, hiSV4, gm24, lo24, hi24 := summarizeTargetSubset(
			assignCases, ar.filter,
			func(c AssignCorpusCase) (sc, v2, v4, arm []float64) {
				return a.AssignCorpus.DefaultScalar[c.ID].TSolveTrialsMS,
					a.AssignCorpus.SIMDAVX2[c.ID].TSolveTrialsMS,
					a.AssignCorpus.SIMDAVX512[c.ID].TSolveTrialsMS,
					r.AssignCorpus.Default[c.ID].TSolveTrialsMS
			},
		)
		fmt.Fprintf(b, "| %s | %d (%d) | %s | %s | %s | **%.2fx** [%.2fx-%.2fx] | %.2fx [%.2fx-%.2fx] | %s | `OK` |\n",
			ar.label, nF, nT,
			formatAssignMS(scS), formatAssignMS(v2S), formatAssignMS(v4S),
			gmSV4, loSV4, hiSV4, gm24, lo24, hi24,
			formatAssignMS(armS))
	}
	return nil
}
