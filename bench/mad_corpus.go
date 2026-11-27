package bench

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"math"
	"math/bits"
	"math/rand/v2"
	"os"
	"path/filepath"

	"github.com/paulourio/goblin/mad"
)

// MADCorpusCase describes one MAD family.
type MADCorpusCase struct {
	ID              string   `json:"id"`
	Tier            string   `json:"tier"`
	Family          string   `json:"family"`
	Description     string   `json:"description"`
	Groups          int      `json:"groups"`
	Items           int      `json:"items"`
	Periods         int      `json:"periods"`
	HasBaselines    bool     `json:"has_baselines"`
	HasDesignated   bool     `json:"has_designated"`
	Step            float64  `json:"step"`
	Lambda          float64  `json:"lambda"`
	RoundWeight     float64  `json:"round_weight"`
	AllowZeroStep   bool     `json:"allow_zero_step"`
	DesignatedItems []int    `json:"designated_items,omitempty"`
	BaseSeed        uint64   `json:"base_seed"`
	Trials          int      `json:"trials"`
	TrialSeeds      []uint64 `json:"trial_seeds"`
	Trial0SHA       string   `json:"trial0_sha256"`
}

// MADCorpusManifest is written to testdata/mad_corpus/manifest.json so benchmark runs
// and verification tests cross-check deterministic dimensions and Trial-0 digests.
type MADCorpusManifest struct {
	Cases []MADCorpusCase `json:"cases"`
}

// MADMILPMetrics summarizes capped MILP runs.
type MADMILPMetrics struct {
	Status         string    `json:"status"`
	TrialStatuses  []string  `json:"trial_statuses,omitempty"`
	HasIncumbent   bool      `json:"has_incumbent"`
	HasBestBound   bool      `json:"has_best_bound"`
	Objective      *float64  `json:"objective,omitempty"`
	BestBound      float64   `json:"best_bound"`
	Nodes          int       `json:"nodes"`
	Iterations     int       `json:"iterations"`
	TimeMS         float64   `json:"time_ms"`
	MeanMS         float64   `json:"mean_ms"`
	SDMS           float64   `json:"sd_ms"`
	TrialsMS       []float64 `json:"trials_ms,omitempty"`
	OptimalCount   int       `json:"optimal_count"`
	TimeLimitCount int       `json:"time_limit_count"`
}

// MADBenchResult stores MAD family telemetry.
type MADBenchResult struct {
	ID                 string          `json:"id"`
	Tier               string          `json:"tier"`
	Family             string          `json:"family"`
	Groups             int             `json:"groups"`
	Items              int             `json:"items"`
	Periods            int             `json:"periods"`
	HasBaselines       bool            `json:"has_baselines"`
	HasDesignated      bool            `json:"has_designated"`
	Step               float64         `json:"step"`
	Lambda             float64         `json:"lambda"`
	RoundWeight        float64         `json:"round_weight"`
	Trials             int             `json:"trials"`
	DFSStates          int64           `json:"dfs_states"`
	DPStates           int64           `json:"dp_states"`
	DPWork             int64           `json:"dp_work"`
	BruteForceRan      bool            `json:"brute_force_ran"`
	CBCAvailable       bool            `json:"cbc_available"`
	SIMDEnabled        bool            `json:"simd_enabled"`
	SIMDTier           string          `json:"simd_tier,omitempty"`
	PairedABBA         bool            `json:"paired_ab_ba,omitzero"`
	CPUModel           string          `json:"cpu_model,omitempty"`
	GOAMD64            string          `json:"goamd64,omitempty"`
	Objective          float64         `json:"objective"`
	TrialObjectives    []float64       `json:"trial_objectives,omitempty"`
	ParityOK           bool            `json:"parity_ok"`
	AutoStrategy       string          `json:"auto_strategy"`
	AutoTimeMS         float64         `json:"auto_time_ms"`
	AutoMeanMS         float64         `json:"auto_mean_ms"`
	AutoSDMS           float64         `json:"auto_sd_ms"`
	AutoTrialsMS       []float64       `json:"auto_trials_ms,omitempty"`
	DPTimeMS           float64         `json:"dp_time_ms"`
	DPMeanMS           float64         `json:"dp_mean_ms"`
	DPSDMS             float64         `json:"dp_sd_ms"`
	DPTrialsMS         []float64       `json:"dp_trials_ms,omitempty"`
	BruteForceTimeMS   float64         `json:"brute_force_time_ms,omitzero"`
	BruteForceMeanMS   float64         `json:"brute_force_mean_ms,omitzero"`
	BruteForceSDMS     float64         `json:"brute_force_sd_ms,omitzero"`
	BruteForceTrialsMS []float64       `json:"brute_force_trials_ms,omitempty"`
	MILPGo             MADMILPMetrics  `json:"milp_go"`
	MILPCBC            *MADMILPMetrics `json:"milp_cbc,omitempty"`
	GitCommit          string          `json:"git_commit,omitempty"`
	GitDirty           bool            `json:"git_dirty,omitzero"`
	GoVersion          string          `json:"go_version,omitempty"`
	SourceFingerprint  string          `json:"source_fingerprint,omitempty"`
	CPUAffinity        string          `json:"cpu_affinity,omitempty"`
	UnameRelease       string          `json:"uname_release,omitempty"`
	MeasuredAt         string          `json:"measured_at,omitempty"`
}

func madCorpusTrialSeed(baseSeed uint64, trial int) uint64 {
	return baseSeed + uint64(trial)*1009
}

// MADCorpusSpecs returns the deterministic 13-family combinatorial MAD benchmark corpus
// definitions and per-trial seeds without generating Trial-0 matrices or digests.
func MADCorpusSpecs() []MADCorpusCase {
	raw := []MADCorpusCase{
		{
			ID:            "pmad_base_g10_i2",
			Tier:          "tier1_small",
			Family:        "pmad_base",
			Description:   "Canonical PMAD base instance (G=10, I=2, T=6, lambda=1.0; Trial 0 exact 24137)",
			Groups:        10,
			Items:         2,
			Periods:       madTimeHorizon,
			HasBaselines:  true,
			HasDesignated: false,
			Step:          madRoundingBase,
			Lambda:        1.0,
			RoundWeight:   float64(madTimeHorizon) * 1.0,
			BaseSeed:      4000,
		},
		{
			ID:            "pmad_g6_i6_lam025",
			Tier:          "tier1_small",
			Family:        "pmad",
			Description:   "PMAD baselines+items (G=6, I=6, T=6, lambda=0.25, RoundWeight=1.5)",
			Groups:        6,
			Items:         6,
			Periods:       madTimeHorizon,
			HasBaselines:  true,
			HasDesignated: false,
			Step:          madRoundingBase,
			Lambda:        0.25,
			RoundWeight:   float64(madTimeHorizon) * 0.25,
			BaseSeed:      4101,
		},
		{
			ID:            "pmad_g6_i10_lam10",
			Tier:          "tier2_medium",
			Family:        "pmad",
			Description:   "PMAD baselines+items (G=6, I=10, T=6, lambda=1.0, RoundWeight=6.0)",
			Groups:        6,
			Items:         10,
			Periods:       madTimeHorizon,
			HasBaselines:  true,
			HasDesignated: false,
			Step:          madRoundingBase,
			Lambda:        1.0,
			RoundWeight:   float64(madTimeHorizon) * 1.0,
			BaseSeed:      4102,
		},
		{
			ID:            "pmad_g6_i12_lam025",
			Tier:          "tier3_large",
			Family:        "pmad",
			Description:   "PMAD baselines+items (G=6, I=12, T=6, lambda=0.25, RoundWeight=1.5)",
			Groups:        6,
			Items:         12,
			Periods:       madTimeHorizon,
			HasBaselines:  true,
			HasDesignated: false,
			Step:          madRoundingBase,
			Lambda:        0.25,
			RoundWeight:   float64(madTimeHorizon) * 0.25,
			BaseSeed:      4103,
		},
		{
			ID:            "pmad_g10_i4_lam025",
			Tier:          "tier1_small",
			Family:        "pmad",
			Description:   "PMAD baselines+items (G=10, I=4, T=6, lambda=0.25, RoundWeight=1.5)",
			Groups:        10,
			Items:         4,
			Periods:       madTimeHorizon,
			HasBaselines:  true,
			HasDesignated: false,
			Step:          madRoundingBase,
			Lambda:        0.25,
			RoundWeight:   float64(madTimeHorizon) * 0.25,
			BaseSeed:      4201,
		},
		{
			ID:            "pmad_g10_i8_lam10",
			Tier:          "tier2_medium",
			Family:        "pmad",
			Description:   "PMAD baselines+items (G=10, I=8, T=6, lambda=1.0, RoundWeight=6.0)",
			Groups:        10,
			Items:         8,
			Periods:       madTimeHorizon,
			HasBaselines:  true,
			HasDesignated: false,
			Step:          madRoundingBase,
			Lambda:        1.0,
			RoundWeight:   float64(madTimeHorizon) * 1.0,
			BaseSeed:      4202,
		},
		{
			ID:            "pmad_g10_i10_lam025",
			Tier:          "tier2_medium",
			Family:        "pmad",
			Description:   "PMAD baselines+items (G=10, I=10, T=6, lambda=0.25, RoundWeight=1.5)",
			Groups:        10,
			Items:         10,
			Periods:       madTimeHorizon,
			HasBaselines:  true,
			HasDesignated: false,
			Step:          madRoundingBase,
			Lambda:        0.25,
			RoundWeight:   float64(madTimeHorizon) * 0.25,
			BaseSeed:      4203,
		},
		{
			ID:            "pmad_g20_i6_lam10",
			Tier:          "tier2_medium",
			Family:        "pmad",
			Description:   "PMAD baselines+items (G=20, I=6, T=6, lambda=1.0, RoundWeight=6.0)",
			Groups:        20,
			Items:         6,
			Periods:       madTimeHorizon,
			HasBaselines:  true,
			HasDesignated: false,
			Step:          madRoundingBase,
			Lambda:        1.0,
			RoundWeight:   float64(madTimeHorizon) * 1.0,
			BaseSeed:      4301,
		},
		{
			ID:            "pmad_g20_i12_lam10",
			Tier:          "tier3_large",
			Family:        "pmad",
			Description:   "PMAD baselines+items (G=20, I=12, T=6, lambda=1.0, RoundWeight=6.0)",
			Groups:        20,
			Items:         12,
			Periods:       madTimeHorizon,
			HasBaselines:  true,
			HasDesignated: false,
			Step:          madRoundingBase,
			Lambda:        1.0,
			RoundWeight:   float64(madTimeHorizon) * 1.0,
			BaseSeed:      4302,
		},
		{
			ID:            "mad_comb_rand_g2_i4",
			Tier:          "tier1_small",
			Family:        "mad_comb_rand",
			Description:   "Random-weight MAD control matching t1_mad_rand_g2_i4 (G=2, I=4, T=6, DesignatedItems=[2..3])",
			Groups:        2,
			Items:         4,
			Periods:       madTimeHorizon,
			HasBaselines:  false,
			HasDesignated: true,
			Step:          madRoundingBase,
			Lambda:        0,
			RoundWeight:   1.0,
			BaseSeed:      7000,
		},
		{
			ID:            "mad_comb_rand_g2_i8",
			Tier:          "tier2_medium",
			Family:        "mad_comb_rand",
			Description:   "Random-weight MAD control matching t2_mad_rand_g2_i8 (G=2, I=8, T=6, DesignatedItems=[6..7])",
			Groups:        2,
			Items:         8,
			Periods:       madTimeHorizon,
			HasBaselines:  false,
			HasDesignated: true,
			Step:          madRoundingBase,
			Lambda:        0,
			RoundWeight:   1.0,
			BaseSeed:      7001,
		},
		{
			ID:            "mad_comb_rand_g3_i9",
			Tier:          "tier2_medium",
			Family:        "mad_comb_rand",
			Description:   "Random-weight MAD control matching t2_mad_rand_g3_i9 (G=3, I=9, T=6, DesignatedItems=[6..8])",
			Groups:        3,
			Items:         9,
			Periods:       madTimeHorizon,
			HasBaselines:  false,
			HasDesignated: true,
			Step:          madRoundingBase,
			Lambda:        0,
			RoundWeight:   1.0,
			BaseSeed:      7002,
		},
		{
			ID:            "mad_comb_rand_g4_i12",
			Tier:          "tier2_medium",
			Family:        "mad_comb_rand",
			Description:   "Random-weight MAD control (G=4, I=12, T=6, DesignatedItems=[8..11])",
			Groups:        4,
			Items:         12,
			Periods:       madTimeHorizon,
			HasBaselines:  false,
			HasDesignated: true,
			Step:          madRoundingBase,
			Lambda:        0,
			RoundWeight:   1.0,
			BaseSeed:      7003,
		},
	}

	for i := range raw {
		nTrials := DefaultTrials(raw[i].Tier)
		raw[i].Trials = nTrials
		seeds := make([]uint64, nTrials)
		for k := range nTrials {
			seeds[k] = madCorpusTrialSeed(raw[i].BaseSeed, k)
		}
		raw[i].TrialSeeds = seeds
		if raw[i].HasDesignated {
			des := make([]int, raw[i].Groups)
			dStart := raw[i].Items - raw[i].Groups
			for g := range raw[i].Groups {
				des[g] = dStart + g
			}
			raw[i].DesignatedItems = des
		}
	}
	return raw
}

func pmadBaseTrial0Problem() mad.Problem {
	return mad.Problem{
		Groups: 10,
		Baselines: [][]float64{
			{508, 465, 706, 1213, 704, 238},
			{1360, 379, 1778, 763, 386, 243},
			{1428, 1832, 1301, 1388, 1021, 832},
			{542, 1930, 1721, 1019, 240, 838},
			{1585, 111, 1967, 886, 880, 781},
			{1216, 494, 1417, 1804, 300, 1037},
			{1370, 1761, 314, 637, 457, 244},
			{1614, 1452, 1244, 736, 1645, 1701},
			{396, 1810, 656, 659, 1201, 1528},
			{1009, 210, 447, 1748, 1746, 655},
		},
		Items: [][]float64{
			{149, 566, 1479, 128, 1950, 1713},
			{491, 1179, 1737, 1103, 990, 1847},
		},
		Step:          madRoundingBase,
		RoundWeight:   float64(madTimeHorizon),
		AllowZeroStep: false,
	}
}

// GenerateMADProblem deterministically builds the mad.Problem for corpus family c and trial index.
func GenerateMADProblem(c MADCorpusCase, trial int) mad.Problem {
	seed := madCorpusTrialSeed(c.BaseSeed, trial)
	if trial >= 0 && trial < len(c.TrialSeeds) {
		seed = c.TrialSeeds[trial]
	}

	switch c.Family {
	case "pmad_base":
		if trial == 0 {
			return pmadBaseTrial0Problem()
		}
		fallthrough
	case "pmad":
		rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
		baselines := make([][]float64, c.Groups)
		for g := range c.Groups {
			row := make([]float64, c.Periods)
			for t := range c.Periods {
				row[t] = float64(100 + rng.IntN(1900))
			}
			baselines[g] = row
		}
		items := make([][]float64, c.Items)
		for i := range c.Items {
			row := make([]float64, c.Periods)
			for t := range c.Periods {
				row[t] = float64(100 + rng.IntN(1900))
			}
			items[i] = row
		}
		return mad.Problem{
			Groups:        c.Groups,
			Baselines:     baselines,
			Items:         items,
			Step:          c.Step,
			RoundWeight:   c.RoundWeight,
			AllowZeroStep: c.AllowZeroStep,
		}
	case "mad_comb_rand":
		rng := rand.New(rand.NewPCG(seed, seed^0x77))
		w := make([][]float64, c.Periods)
		for t := range c.Periods {
			w[t] = make([]float64, c.Items)
			for i := range c.Items {
				w[t][i] = float64(100 + rng.IntN(1900))
			}
		}
		items, err := mad.TransposePeriodWeights(w)
		if err != nil {
			panic(err)
		}
		var designated []int
		if len(c.DesignatedItems) > 0 {
			designated = append([]int(nil), c.DesignatedItems...)
		} else if c.HasDesignated {
			designated = make([]int, c.Groups)
			dStart := c.Items - c.Groups
			for g := range c.Groups {
				designated[g] = dStart + g
			}
		}
		return mad.Problem{
			Groups:          c.Groups,
			Items:           items,
			Step:            c.Step,
			RoundWeight:     c.RoundWeight,
			AllowZeroStep:   c.AllowZeroStep,
			DesignatedItems: designated,
		}
	default:
		panic(fmt.Sprintf("unknown mad corpus family %q", c.Family))
	}
}

// MADProblemSHA256 returns the first 16 hex characters of the SHA-256 digest over the
// canonical dimensions, scalar parameters, designated indices, baselines, and items of prob.
func MADProblemSHA256(prob mad.Problem) string {
	h := sha256.New()
	var buf [8]byte
	writeInt := func(v int) {
		binary.LittleEndian.PutUint64(buf[:], uint64(v))
		_, _ = h.Write(buf[:])
	}
	writeFloat := func(v float64) {
		binary.LittleEndian.PutUint64(buf[:], math.Float64bits(v))
		_, _ = h.Write(buf[:])
	}

	writeInt(prob.Groups)
	writeInt(len(prob.Baselines))
	writeInt(len(prob.Items))
	var periods int
	if len(prob.Items) > 0 {
		periods = len(prob.Items[0])
	} else if len(prob.Baselines) > 0 {
		periods = len(prob.Baselines[0])
	}
	writeInt(periods)
	writeFloat(prob.Step)
	writeFloat(prob.RoundWeight)
	if prob.AllowZeroStep {
		writeInt(1)
	} else {
		writeInt(0)
	}
	writeInt(len(prob.DesignatedItems))
	for _, idx := range prob.DesignatedItems {
		writeInt(idx)
	}
	for _, row := range prob.Baselines {
		for _, v := range row {
			writeFloat(v)
		}
	}
	for _, row := range prob.Items {
		for _, v := range row {
			writeFloat(v)
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

const madSatLimit = math.MaxInt64 / 2

func madSatAdd(a, b int64) int64 {
	if a >= madSatLimit || b >= madSatLimit || a > madSatLimit-b {
		return madSatLimit
	}
	return a + b
}

func madSatMul(a, b int64) int64 {
	if a <= 0 || b <= 0 {
		return 0
	}
	if a >= madSatLimit || b >= madSatLimit || a > madSatLimit/b {
		return madSatLimit
	}
	return a * b
}

func madSatPow(base int64, exp int) int64 {
	if exp <= 0 {
		return 1
	}
	if base <= 0 {
		return 0
	}
	res := int64(1)
	b := base
	e := exp
	for e > 0 {
		if e&1 != 0 {
			res = madSatMul(res, b)
		}
		e >>= 1
		if e > 0 {
			b = madSatMul(b, b)
		}
	}
	return res
}

func madSatFact(n int) int64 {
	res := int64(1)
	for i := 2; i <= n; i++ {
		res = madSatMul(res, int64(i))
		if res >= madSatLimit {
			return madSatLimit
		}
	}
	return res
}

func madSatStirling2BellSum(n, maxK int) int64 {
	if n <= 0 || maxK <= 0 {
		return 1
	}
	kLim := min(n, maxK)
	prev := make([]int64, kLim+1)
	cur := make([]int64, kLim+1)
	prev[0] = 1
	for i := 1; i <= n; i++ {
		clear(cur)
		up := min(i, kLim)
		for k := 1; k <= up; k++ {
			cur[k] = madSatAdd(prev[k-1], madSatMul(int64(k), prev[k]))
		}
		prev, cur = cur, prev
	}
	var total int64
	for k := 1; k <= kLim; k++ {
		total = madSatAdd(total, prev[k])
	}
	return total
}

func madSatBinom(n, k int) int64 {
	if k < 0 || k > n {
		return 0
	}
	if k == 0 || k == n {
		return 1
	}
	if k > n-k {
		k = n - k
	}
	res := int64(1)
	for i := 1; i <= k; i++ {
		num := int64(n - k + i)
		if res >= madSatLimit/num {
			return madSatLimit
		}
		res = (res * num) / int64(i)
	}
	return res
}

func madUniqueBaselines(baselines [][]float64) int {
	if baselines == nil {
		return 1
	}
	var reps [][]float64
	for _, row := range baselines {
		matched := false
		for _, rep := range reps {
			if len(row) != len(rep) {
				continue
			}
			eq := true
			for t := range row {
				if row[t] != rep[t] {
					eq = false
					break
				}
			}
			if eq {
				matched = true
				break
			}
		}
		if !matched {
			reps = append(reps, row)
		}
	}
	return max(1, len(reps))
}

// MADPlanMetrics computes the DFS state count, stored DP subset state count, and estimated
// DP work matching mad/solver.go's buildCombPlanBudget planner.
func MADPlanMetrics(prob mad.Problem) (dfsStates, dpStates, dpWork int64) {
	numGroups := prob.Groups
	if prob.Baselines != nil {
		numGroups = len(prob.Baselines)
	}
	numItems := len(prob.Items)
	var numPeriods int
	if numItems > 0 {
		numPeriods = len(prob.Items[0])
	} else if len(prob.Baselines) > 0 {
		numPeriods = len(prob.Baselines[0])
	}
	if numGroups <= 0 {
		return 0, 0, 0
	}

	uCount := madUniqueBaselines(prob.Baselines)
	hasD := len(prob.DesignatedItems) == numGroups
	canPinD := hasD && uCount == 1

	switch {
	case !hasD && prob.Baselines == nil:
		dfsStates = madSatStirling2BellSum(numItems, numGroups)
	case canPinD:
		dfsStates = madSatPow(int64(numGroups), numItems-numGroups)
	case hasD && !canPinD:
		dfsStates = madSatMul(madSatFact(numGroups), madSatPow(int64(numGroups), numItems-numGroups))
	default:
		dfsStates = madSatPow(int64(numGroups), numItems)
	}

	dpBits := numItems
	if canPinD {
		dpBits = numItems - numGroups
	}
	if dpBits > mad.MaxDPCeiling {
		return dfsStates, 0, madSatLimit
	}

	numMasks := int64(1) << uint(dpBits)
	streamingDP := !hasD && uCount > 1 && numGroups > numItems+1
	var oracleEvals, dpTransWork int64

	switch {
	case !hasD && uCount == 1:
		effG := min(numGroups, numItems)
		oracleEvals = numMasks
		dpTransWork = madSatMul(int64(effG), madSatPow(3, dpBits))
		dpStates = numMasks
	case !hasD && !streamingDP:
		oracleEvals = madSatMul(int64(uCount), numMasks)
		dpTransWork = madSatMul(int64(numGroups), madSatPow(3, dpBits))
		dpStates = oracleEvals
	case !hasD && streamingDP:
		oracleEvals = madSatMul(int64(uCount), numMasks)
		var candPairs int64
		for k := 1; k <= numItems; k++ {
			b := madSatBinom(numItems, k)
			capK := int64(min(numGroups, numItems-k+1))
			candPairs = madSatAdd(candPairs, madSatMul(b, capK))
			remPow := int64(1) << uint(numItems-k)
			dpTransWork = madSatAdd(dpTransWork, madSatMul(madSatMul(b, capK), remPow))
		}
		dpStates = madSatAdd(int64(uCount), candPairs)
	case canPinD:
		oracleEvals = madSatMul(int64(numGroups), numMasks)
		dpTransWork = madSatMul(int64(numGroups), madSatPow(3, dpBits))
		dpStates = oracleEvals
	default:
		freeMasks := int64(1) << uint(numItems-numGroups)
		oracleEvals = madSatMul(int64(uCount), madSatMul(int64(numGroups), freeMasks))
		dSubsetHalf := int64(1) << uint(numGroups-1)
		dpTransWork = madSatMul(int64(numGroups), madSatMul(dSubsetHalf, madSatPow(3, numItems-numGroups)))
		dpStates = oracleEvals
	}

	evalCost := madSatMul(int64(numPeriods), int64(max(1, bits.Len(uint(numPeriods)))))
	finalEvalWork := madSatMul(int64(numGroups), evalCost)
	oracleWork := madSatMul(oracleEvals, evalCost)
	postEvalWork := finalEvalWork
	if prob.Baselines == nil {
		postEvalWork = madSatAdd(int64(numGroups), madSatMul(int64(1+min(numGroups, numItems)), evalCost))
	}
	dpWork = madSatAdd(madSatAdd(oracleWork, dpTransWork), postEvalWork)
	return dfsStates, dpStates, dpWork
}

// MADDFSStates returns DFS state count.
func MADDFSStates(prob mad.Problem) int64 {
	dfsStates, _, _ := MADPlanMetrics(prob)
	return dfsStates
}

// BuildMADCorpus constructs the deterministic 13-family MAD combinatorial corpus
// including canonical Trial-0 SHA-256 digests.
func BuildMADCorpus() []MADCorpusCase {
	cases := MADCorpusSpecs()
	for i := range cases {
		cases[i].Trial0SHA = MADProblemSHA256(GenerateMADProblem(cases[i], 0))
	}
	return cases
}

// WriteMADCorpusManifest writes testdata/mad_corpus/manifest.json containing all
// 13 MAD combinatorial corpus definitions, per-trial seeds, and canonical Trial-0 SHA-256 digests so
// downstream benchmark runners and doc-sync tests verify identical problem generation across runs.
func WriteMADCorpusManifest(dir string) ([]MADCorpusCase, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	cases := BuildMADCorpus()
	manifest := MADCorpusManifest{Cases: cases}
	rawJSON, err := json.Marshal(&manifest, jsontext.WithIndent("  "))
	if err != nil {
		return nil, err
	}
	rawJSON = append(rawJSON, '\n')
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), rawJSON, 0o644); err != nil {
		return nil, err
	}
	return cases, nil
}
