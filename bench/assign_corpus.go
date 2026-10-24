package bench

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/paulourio/goblin/assign"
)

// AssignCorpusCase describes a single benchmark family in the 3-tier Linear Sum Assignment corpus.
type AssignCorpusCase struct {
	ID          string       `json:"id"`
	Tier        string       `json:"tier"` // "tier1_small", "tier2_medium", "tier3_large"
	Rows        int          `json:"rows"`
	Cols        int          `json:"cols"`
	Sense       assign.Sense `json:"sense"`
	Family      string       `json:"family"`
	Description string       `json:"description"`
	BaseSeed    uint64       `json:"base_seed"`
	Trials      int          `json:"trials"`
	TrialSeeds  []uint64     `json:"trial_seeds"`
	Trial0SHA   string       `json:"trial0_sha256"`
}

// AssignCorpusManifest is serialized to testdata/assign_corpus/manifest.json so that both Go
// and Python (scipy.optimize.linear_sum_assignment) generate and verify bit-identical float64
// cost matrices across all trials without checking hundreds of megabytes into git.
type AssignCorpusManifest struct {
	Cases []AssignCorpusCase `json:"cases"`
}

func splitMix64At(seed uint64, idx int) uint64 {
	z := seed + uint64(idx+1)*0x9e3779b97f4a7c15
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

func assignTrialSeed(baseSeed uint64, trial int) uint64 {
	return baseSeed + uint64(trial)*100003
}

// MatrixSHA256 returns the first 16 hex characters of the SHA-256 digest over the
// little-endian IEEE-754 binary representation of mat.
func MatrixSHA256(mat []float64) string {
	h := sha256.New()
	var buf [8]byte
	for _, v := range mat {
		binary.LittleEndian.PutUint64(buf[:], math.Float64bits(v))
		_, _ = h.Write(buf[:])
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// GenerateAssignMatrix deterministically constructs the flat row-major cost matrix
// for the given AssignCorpusCase and trial index. All values are exact dyadic floats
// (integer * 2^-k) so Go and Python/NumPy produce bit-identical IEEE-754 matrices.
func GenerateAssignMatrix(c AssignCorpusCase, trial int) []float64 {
	seed := assignTrialSeed(c.BaseSeed, trial)
	if trial >= 0 && trial < len(c.TrialSeeds) {
		seed = c.TrialSeeds[trial]
	}
	rows, cols := c.Rows, c.Cols
	total := rows * cols
	cost := make([]float64, total)

	switch c.Family {
	case "canonical":
		base := [16]float64{
			90, 75, 75, 80,
			35, 85, 55, 65,
			125, 95, 90, 105,
			45, 110, 95, 115,
		}
		for idx := range total {
			val := base[idx%16]
			if trial > 0 {
				val += float64(int(splitMix64At(seed, idx)%21) - 10)
			}
			cost[idx] = val
		}
	case "uniform", "wide", "tall", "maximize":
		// Exact dyadic rationals in [0, 9765.625): (u % 10_000_000) * 2^-10.
		const scale = 1.0 / 1024.0
		for idx := range total {
			cost[idx] = float64(splitMix64At(seed, idx)%10000000) * scale
		}
	case "integer_ties":
		mod := uint64(16)
		switch c.Tier {
		case "tier1_small":
			mod = 8
		case "tier3_large":
			mod = 32
		}
		for idx := range total {
			cost[idx] = float64(splitMix64At(seed, idx) % mod)
		}
	case "geometric":
		// 2D Euclidean squared distances on exact 2^-7 grid points.
		const coordScale = 1.0 / 128.0
		px := make([]float64, rows)
		py := make([]float64, rows)
		qx := make([]float64, cols)
		qy := make([]float64, cols)
		for r := range rows {
			px[r] = float64(splitMix64At(seed, r)%100000) * coordScale
			py[r] = float64(splitMix64At(seed, rows+r)%100000) * coordScale
		}
		for col := range cols {
			qx[col] = float64(splitMix64At(seed, 2*rows+col)%100000) * coordScale
			qy[col] = float64(splitMix64At(seed, 2*rows+cols+col)%100000) * coordScale
		}
		for r := range rows {
			prx, pry := px[r], py[r]
			rowSlice := cost[r*cols : (r+1)*cols]
			for col := range cols {
				dx := prx - qx[col]
				dy := pry - qy[col]
				rowSlice[col] = dx*dx + dy*dy
			}
		}
	case "rank1":
		// Outer product a[r]*b[c] + exact 2^-6 perturbation.
		const noiseScale = 1.0 / 64.0
		a := make([]float64, rows)
		b := make([]float64, cols)
		for r := range rows {
			a[r] = float64(100 + splitMix64At(seed, r)%900)
		}
		for col := range cols {
			b[col] = float64(100 + splitMix64At(seed, rows+col)%900)
		}
		baseOffset := rows + cols
		for r := range rows {
			ar := a[r]
			rowSlice := cost[r*cols : (r+1)*cols]
			for col := range cols {
				idx := baseOffset + r*cols + col
				noise := float64(splitMix64At(seed, idx)%64) * noiseScale
				rowSlice[col] = ar*b[col] + noise
			}
		}
	case "sparse":
		// Forbidden +Inf edges outside guaranteed feasible cyclic bands.
		inf := math.Inf(1)
		for r := range rows {
			for col := range cols {
				idx := r*cols + col
				band := (col - r + cols) % cols
				if band > 2 && splitMix64At(seed, idx)%10 < 6 {
					cost[idx] = inf
				} else {
					cost[idx] = float64(10 + splitMix64At(seed, idx)%500)
				}
			}
		}
	default:
		panic(fmt.Sprintf("unknown assign corpus family %q", c.Family))
	}
	return cost
}

// BuildAssignCorpus constructs the deterministic 22-family, 3-tier Linear Sum Assignment corpus.
func BuildAssignCorpus() []AssignCorpusCase {
	raw := []AssignCorpusCase{
		// Tier 1: Small / Micro Suite (N <= 32, 11 trials)
		{
			ID:          "a1_canon_4x4",
			Tier:        "tier1_small",
			Rows:        4,
			Cols:        4,
			Sense:       assign.Minimize,
			Family:      "canonical",
			Description: "4x4 canonical Hungarian cost matrix",
			BaseSeed:    1001,
		},
		{
			ID:          "a1_unif_10x10",
			Tier:        "tier1_small",
			Rows:        10,
			Cols:        10,
			Sense:       assign.Minimize,
			Family:      "uniform",
			Description: "10x10 uniform continuous assignment",
			BaseSeed:    1002,
		},
		{
			ID:          "a1_ties_20x20",
			Tier:        "tier1_small",
			Rows:        20,
			Cols:        20,
			Sense:       assign.Minimize,
			Family:      "integer_ties",
			Description: "20x20 high-tie small integer {0..7} assignment",
			BaseSeed:    1003,
		},
		{
			ID:          "a1_wide_16x32",
			Tier:        "tier1_small",
			Rows:        16,
			Cols:        32,
			Sense:       assign.Minimize,
			Family:      "wide",
			Description: "16x32 rectangular wide assignment (m < n)",
			BaseSeed:    1004,
		},
		{
			ID:          "a1_tall_32x16",
			Tier:        "tier1_small",
			Rows:        32,
			Cols:        16,
			Sense:       assign.Minimize,
			Family:      "tall",
			Description: "32x16 rectangular tall assignment (m > n)",
			BaseSeed:    1005,
		},
		{
			ID:          "a1_sparse_24x24",
			Tier:        "tier1_small",
			Rows:        24,
			Cols:        24,
			Sense:       assign.Minimize,
			Family:      "sparse",
			Description: "24x24 sparse assignment with +Inf forbidden edges",
			BaseSeed:    1006,
		},

		// Tier 2: Medium / Target p90 Suite (64 <= N <= 256, 11 trials)
		{
			ID:          "a2_unif_64x64",
			Tier:        "tier2_medium",
			Rows:        64,
			Cols:        64,
			Sense:       assign.Minimize,
			Family:      "uniform",
			Description: "64x64 uniform continuous assignment",
			BaseSeed:    2001,
		},
		{
			ID:          "a2_unif_100x100",
			Tier:        "tier2_medium",
			Rows:        100,
			Cols:        100,
			Sense:       assign.Minimize,
			Family:      "uniform",
			Description: "100x100 uniform continuous assignment",
			BaseSeed:    2002,
		},
		{
			ID:          "a2_ties_100x100",
			Tier:        "tier2_medium",
			Rows:        100,
			Cols:        100,
			Sense:       assign.Minimize,
			Family:      "integer_ties",
			Description: "100x100 degenerate integer {0..15} assignment",
			BaseSeed:    2003,
		},
		{
			ID:          "a2_geom_100x100",
			Tier:        "tier2_medium",
			Rows:        100,
			Cols:        100,
			Sense:       assign.Minimize,
			Family:      "geometric",
			Description: "100x100 2D Euclidean squared-distance assignment",
			BaseSeed:    2004,
		},
		{
			ID:          "a2_rank1_100x100",
			Tier:        "tier2_medium",
			Rows:        100,
			Cols:        100,
			Sense:       assign.Minimize,
			Family:      "rank1",
			Description: "100x100 pathological rank-1 outer-product assignment",
			BaseSeed:    2005,
		},
		{
			ID:          "a2_wide_80x200",
			Tier:        "tier2_medium",
			Rows:        80,
			Cols:        200,
			Sense:       assign.Minimize,
			Family:      "wide",
			Description: "80x200 rectangular wide assignment (m < n)",
			BaseSeed:    2006,
		},
		{
			ID:          "a2_tall_200x80",
			Tier:        "tier2_medium",
			Rows:        200,
			Cols:        80,
			Sense:       assign.Minimize,
			Family:      "tall",
			Description: "200x80 rectangular tall assignment (m > n)",
			BaseSeed:    2007,
		},
		{
			ID:          "a2_max_128x128",
			Tier:        "tier2_medium",
			Rows:        128,
			Cols:        128,
			Sense:       assign.Maximize,
			Family:      "maximize",
			Description: "128x128 maximum-weight assignment",
			BaseSeed:    2008,
		},
		{
			ID:          "a2_unif_256x256",
			Tier:        "tier2_medium",
			Rows:        256,
			Cols:        256,
			Sense:       assign.Minimize,
			Family:      "uniform",
			Description: "256x256 uniform continuous assignment (AVX2 dispatch boundary)",
			BaseSeed:    2009,
		},

		// Tier 3: Large Tail Suite (500 <= N <= 2000, 5 trials)
		{
			ID:          "a3_unif_500x500",
			Tier:        "tier3_large",
			Rows:        500,
			Cols:        500,
			Sense:       assign.Minimize,
			Family:      "uniform",
			Description: "500x500 uniform continuous assignment",
			BaseSeed:    3001,
		},
		{
			ID:          "a3_ties_500x500",
			Tier:        "tier3_large",
			Rows:        500,
			Cols:        500,
			Sense:       assign.Minimize,
			Family:      "integer_ties",
			Description: "500x500 degenerate integer {0..31} assignment",
			BaseSeed:    3002,
		},
		{
			ID:          "a3_geom_500x500",
			Tier:        "tier3_large",
			Rows:        500,
			Cols:        500,
			Sense:       assign.Minimize,
			Family:      "geometric",
			Description: "500x500 2D Euclidean squared-distance assignment",
			BaseSeed:    3003,
		},
		{
			ID:          "a3_wide_250x1000",
			Tier:        "tier3_large",
			Rows:        250,
			Cols:        1000,
			Sense:       assign.Minimize,
			Family:      "wide",
			Description: "250x1000 rectangular wide assignment (m < n)",
			BaseSeed:    3004,
		},
		{
			ID:          "a3_tall_1000x250",
			Tier:        "tier3_large",
			Rows:        1000,
			Cols:        250,
			Sense:       assign.Minimize,
			Family:      "tall",
			Description: "1000x250 rectangular tall assignment (m > n)",
			BaseSeed:    3005,
		},
		{
			ID:          "a3_unif_1000x1000",
			Tier:        "tier3_large",
			Rows:        1000,
			Cols:        1000,
			Sense:       assign.Minimize,
			Family:      "uniform",
			Description: "1000x1000 uniform continuous assignment (10^6 entries)",
			BaseSeed:    3006,
		},
		{
			ID:          "a3_unif_2000x2000",
			Tier:        "tier3_large",
			Rows:        2000,
			Cols:        2000,
			Sense:       assign.Minimize,
			Family:      "uniform",
			Description: "2000x2000 uniform continuous assignment (4x10^6 entries)",
			BaseSeed:    3007,
		},
	}

	for i := range raw {
		nTrials := DefaultTrials(raw[i].Tier)
		raw[i].Trials = nTrials
		seeds := make([]uint64, nTrials)
		for k := range nTrials {
			seeds[k] = assignTrialSeed(raw[i].BaseSeed, k)
		}
		raw[i].TrialSeeds = seeds
		mat0 := GenerateAssignMatrix(raw[i], 0)
		raw[i].Trial0SHA = MatrixSHA256(mat0)
	}
	return raw
}

// WriteAssignCorpusManifest writes testdata/assign_corpus/manifest.json containing all
// 22 assignment corpus definitions, per-trial seeds, and canonical Trial-0 matrix SHA-256 digests.
func WriteAssignCorpusManifest(dir string) ([]AssignCorpusCase, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	cases := BuildAssignCorpus()
	manifest := AssignCorpusManifest{Cases: cases}
	rawJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	rawJSON = append(rawJSON, '\n')
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), rawJSON, 0o644); err != nil {
		return nil, err
	}
	return cases, nil
}
