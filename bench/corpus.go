// Package bench provides the deterministic 3-tier MILP benchmark corpus
// and comparative timing harness for gopt vs. Python PuLP + COIN-OR CBC.
package bench

import (
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"

	"gopt"
	"gopt/format"
)

// CorpusCase describes a single benchmark instance in the 3-tier corpus.
type CorpusCase struct {
	ID          string
	Tier        string // "tier1_canonical", "tier2_p90", "tier3_large"
	Description string
	Model       *gopt.Model
	BuildTrial  func(trial int) *gopt.Model
}

// TrialModel returns the deterministic problem instance for trial k (k == 0 returns the canonical Model).
func (c CorpusCase) TrialModel(trial int) *gopt.Model {
	if trial <= 0 || c.BuildTrial == nil {
		return c.Model
	}
	return c.BuildTrial(trial)
}

// DefaultTrials returns the number of randomized benchmark trials for the given tier.
func DefaultTrials(tier string) int {
	if tier == "tier3_large" {
		return 5
	}
	return 11
}

func trialSeed(base uint64, trial int) uint64 {
	return base + uint64(trial)*1009
}

// BuildCorpus constructs the deterministic 3-tier benchmark suite covering
// canonical PuLP unit tests (Tier 1, N <= 15), p90 target workloads (Tier 2, 15 <= N <= 50),
// and large tail workloads (Tier 3, 100 <= N <= 1000).
func BuildCorpus() []CorpusCase {
	var cases []CorpusCase

	// =========================================================================
	// Tier 1: Canonical & Edge-Case Suite (N <= 22)
	// =========================================================================
	cases = append(cases,
		CorpusCase{
			ID:          "t1_std_lp",
			Tier:        "tier1_canonical",
			Description: "PuLP canonical continuous LP (3 vars, 3 rows)",
			Model:       buildPuLPStdProblem("t1_std_lp", gopt.Minimize, false, false),
			BuildTrial: func(trial int) *gopt.Model {
				return buildPuLPStdProblemTrial("t1_std_lp", gopt.Minimize, false, false, trial)
			},
		},
		CorpusCase{
			ID:          "t1_std_lp_max",
			Tier:        "tier1_canonical",
			Description: "PuLP canonical continuous LP maximize (3 vars, 3 rows)",
			Model:       buildPuLPStdProblem("t1_std_lp_max", gopt.Maximize, false, false),
			BuildTrial: func(trial int) *gopt.Model {
				return buildPuLPStdProblemTrial("t1_std_lp_max", gopt.Maximize, false, false, trial)
			},
		},
		CorpusCase{
			ID:          "t1_std_mip",
			Tier:        "tier1_canonical",
			Description: "PuLP canonical mixed-integer problem (3 vars, 3 rows)",
			Model:       buildPuLPStdProblem("t1_std_mip", gopt.Minimize, true, false),
			BuildTrial: func(trial int) *gopt.Model {
				return buildPuLPStdProblemTrial("t1_std_mip", gopt.Minimize, true, false, trial)
			},
		},
		CorpusCase{
			ID:          "t1_mip_float_obj",
			Tier:        "tier1_canonical",
			Description: "PuLP test_mip_floats_objective (3 vars, 3 rows, obj=64.95)",
			Model:       buildPuLPStdProblem("t1_mip_float_obj", gopt.Minimize, true, true),
			BuildTrial: func(trial int) *gopt.Model {
				return buildPuLPStdProblemTrial("t1_mip_float_obj", gopt.Minimize, true, true, trial)
			},
		},
		CorpusCase{
			ID:          "t1_whiskas",
			Tier:        "tier1_canonical",
			Description: "PuLP Whiskas blending problem (6 vars, 5 rows)",
			Model:       buildWhiskasProblem(),
			BuildTrial:  buildWhiskasProblemTrial,
		},
		CorpusCase{
			ID:          "t1_furniture",
			Tier:        "tier1_canonical",
			Description: "PuLP integer furniture production (2 integer vars, 3 rows)",
			Model:       buildFurnitureProblem(),
			BuildTrial:  buildFurnitureProblemTrial,
		},
		CorpusCase{
			ID:          "t1_knapsack_12",
			Tier:        "tier1_canonical",
			Description: "Mixed 0-1 and integer knapsack (12 vars, 3 rows)",
			Model:       buildMixedKnapsack("t1_knapsack_12", 12, 3, 101),
			BuildTrial: func(trial int) *gopt.Model {
				return buildMixedKnapsack("t1_knapsack_12", 12, 3, trialSeed(101, trial))
			},
		},
		CorpusCase{
			ID:          "t1_infeasible_mip",
			Tier:        "tier1_canonical",
			Description: "LP-feasible but integer-infeasible parity check (2 binary vars, 1 row)",
			Model:       buildInfeasibleMIP(),
			BuildTrial:  buildInfeasibleMIPTrial,
		},
		CorpusCase{
			ID:          "t1_mad_g2_i4",
			Tier:        "tier1_canonical",
			Description: "22-var MAD group-item allocation (G=2 groups, I=4 items, T=6 periods)",
			Model:       buildMADAllocation("t1_mad_g2_i4", 2, 4, 6, 109),
			BuildTrial: func(trial int) *gopt.Model {
				return buildMADAllocation("t1_mad_g2_i4", 2, 4, 6, trialSeed(109, trial))
			},
		},
		CorpusCase{
			ID:          "t1_mad_rand_g2_i4",
			Tier:        "tier1_canonical",
			Description: "22-var unstructured random-weight MAD allocation (G=2 groups, I=4 items, T=6 periods)",
			Model:       BuildMADRandom("t1_mad_rand_g2_i4", 2, 4, 6, 7000),
			BuildTrial: func(trial int) *gopt.Model {
				return BuildMADRandom("t1_mad_rand_g2_i4", 2, 4, 6, trialSeed(7000, trial))
			},
		},
	)

	// =========================================================================
	// Tier 2: Target p90 Workload (15 <= N <= 76, mostly 0-1 & Integer + Continuous)
	// =========================================================================
	cases = append(cases,
		CorpusCase{
			ID:          "t2_mkp_30",
			Tier:        "tier2_p90",
			Description: "30-var mixed 0-1 & general integer multidimensional knapsack (6 rows)",
			Model:       buildMixedKnapsack("t2_mkp_30", 30, 6, 201),
			BuildTrial: func(trial int) *gopt.Model {
				return buildMixedKnapsack("t2_mkp_30", 30, 6, trialSeed(201, trial))
			},
		},
		CorpusCase{
			ID:          "t2_mkp_50",
			Tier:        "tier2_p90",
			Description: "50-var mixed 0-1 & general integer multidimensional knapsack (8 rows)",
			Model:       buildMixedKnapsack("t2_mkp_50", 50, 8, 202),
			BuildTrial: func(trial int) *gopt.Model {
				return buildMixedKnapsack("t2_mkp_50", 50, 8, trialSeed(202, trial))
			},
		},
		CorpusCase{
			ID:          "t2_cflp_36",
			Tier:        "tier2_p90",
			Description: "36-var Capacitated Facility Location (6 binary open + 30 mixed flows)",
			Model:       buildFacilityLocation("t2_cflp_36", 6, 5, 203),
			BuildTrial: func(trial int) *gopt.Model {
				return buildFacilityLocation("t2_cflp_36", 6, 5, trialSeed(203, trial))
			},
		},
		CorpusCase{
			ID:          "t2_cflp_49",
			Tier:        "tier2_p90",
			Description: "49-var Capacitated Facility Location (7 binary open + 42 mixed flows)",
			Model:       buildFacilityLocation("t2_cflp_49", 7, 6, 204),
			BuildTrial: func(trial int) *gopt.Model {
				return buildFacilityLocation("t2_cflp_49", 7, 6, trialSeed(204, trial))
			},
		},
		CorpusCase{
			ID:          "t2_gap_45",
			Tier:        "tier2_p90",
			Description: "45-var Generalized Assignment with integer overtime (5 agents, 8 tasks)",
			Model:       buildGeneralizedAssignment("t2_gap_45", 5, 8, 205),
			BuildTrial: func(trial int) *gopt.Model {
				return buildGeneralizedAssignment("t2_gap_45", 5, 8, trialSeed(205, trial))
			},
		},
		CorpusCase{
			ID:          "t2_lotsizing_48",
			Tier:        "tier2_p90",
			Description: "48-var Capacitated Lot-Sizing (16 periods: binary setup + int prod + cont inv)",
			Model:       buildLotSizing("t2_lotsizing_48", 16, 206),
			BuildTrial: func(trial int) *gopt.Model {
				return buildLotSizing("t2_lotsizing_48", 16, trialSeed(206, trial))
			},
		},
		CorpusCase{
			ID:          "t2_binpack_44",
			Tier:        "tier2_p90",
			Description: "44-var Bin Packing with general integer slack (4 bins, 10 items)",
			Model:       buildBinPacking("t2_binpack_44", 4, 10, 207),
			BuildTrial: func(trial int) *gopt.Model {
				return buildBinPacking("t2_binpack_44", 4, 10, trialSeed(207, trial))
			},
		},
		CorpusCase{
			ID:          "t2_setcover_50",
			Tier:        "tier2_p90",
			Description: "50-var Set Covering with 0-1 selection and integer penalty slacks (20 rows)",
			Model:       buildSetCovering("t2_setcover_50", 50, 20, 208),
			BuildTrial: func(trial int) *gopt.Model {
				return buildSetCovering("t2_setcover_50", 50, 20, trialSeed(208, trial))
			},
		},
		CorpusCase{
			ID:          "t2_mad_g2_i8",
			Tier:        "tier2_p90",
			Description: "30-var MAD group-item allocation (G=2 groups, I=8 items, T=6 periods)",
			Model:       buildMADAllocation("t2_mad_g2_i8", 2, 8, 6, 209),
			BuildTrial: func(trial int) *gopt.Model {
				return buildMADAllocation("t2_mad_g2_i8", 2, 8, 6, trialSeed(209, trial))
			},
		},
		CorpusCase{
			ID:          "t2_mad_g3_i9",
			Tier:        "tier2_p90",
			Description: "48-var MAD group-item allocation (G=3 groups, I=9 items, T=6 periods)",
			Model:       buildMADAllocation("t2_mad_g3_i9", 3, 9, 6, 210),
			BuildTrial: func(trial int) *gopt.Model {
				return buildMADAllocation("t2_mad_g3_i9", 3, 9, 6, trialSeed(210, trial))
			},
		},
		CorpusCase{
			ID:          "t2_mad_g4_i12",
			Tier:        "tier2_p90",
			Description: "76-var MAD group-item allocation (G=4 groups, I=12 items, T=6 periods)",
			Model:       buildMADAllocation("t2_mad_g4_i12", 4, 12, 6, 211),
			BuildTrial: func(trial int) *gopt.Model {
				return buildMADAllocation("t2_mad_g4_i12", 4, 12, 6, trialSeed(211, trial))
			},
		},
		CorpusCase{
			ID:          "t2_mad_rand_g2_i8",
			Tier:        "tier2_p90",
			Description: "30-var unstructured random-weight MAD allocation (G=2 groups, I=8 items, T=6 periods)",
			Model:       BuildMADRandom("t2_mad_rand_g2_i8", 2, 8, 6, 7001),
			BuildTrial: func(trial int) *gopt.Model {
				return BuildMADRandom("t2_mad_rand_g2_i8", 2, 8, 6, trialSeed(7001, trial))
			},
		},
		CorpusCase{
			ID:          "t2_mad_rand_g3_i9",
			Tier:        "tier2_p90",
			Description: "48-var unstructured random-weight MAD allocation (G=3 groups, I=9 items, T=6 periods)",
			Model:       BuildMADRandom("t2_mad_rand_g3_i9", 3, 9, 6, 7002),
			BuildTrial: func(trial int) *gopt.Model {
				return BuildMADRandom("t2_mad_rand_g3_i9", 3, 9, 6, trialSeed(7002, trial))
			},
		},
	)

	// =========================================================================
	// Tier 3: Large Tail Workload (100 <= N <= 4028)
	// =========================================================================
	cases = append(cases,
		CorpusCase{
			ID:          "t3_mkp_200",
			Tier:        "tier3_large",
			Description: "200-var mixed 0-1/integer/continuous multidimensional knapsack (8 rows)",
			Model:       buildMixedKnapsack("t3_mkp_200", 200, 8, 301),
			BuildTrial: func(trial int) *gopt.Model {
				return buildMixedKnapsack("t3_mkp_200", 200, 8, trialSeed(301, trial))
			},
		},
		CorpusCase{
			ID:          "t3_cflp_300",
			Tier:        "tier3_large",
			Description: "300-var Capacitated Facility Location (15 binary facilities, 19 customers)",
			Model:       buildFacilityLocation("t3_cflp_300", 15, 19, 302),
			BuildTrial: func(trial int) *gopt.Model {
				return buildFacilityLocation("t3_cflp_300", 15, 19, trialSeed(302, trial))
			},
		},
		CorpusCase{
			ID:          "t3_lotsizing_504",
			Tier:        "tier3_large",
			Description: "504-var Multi-Item Capacitated Lot-Sizing (42 items x 4 periods x 3 vars)",
			Model:       buildMultiItemLotSizing("t3_lotsizing_504", 42, 4, 303),
			BuildTrial: func(trial int) *gopt.Model {
				return buildMultiItemLotSizing("t3_lotsizing_504", 42, 4, trialSeed(303, trial))
			},
		},
		CorpusCase{
			ID:          "t3_sched_1000",
			Tier:        "tier3_large",
			Description: "1000-var Block Resource Allocation & Scheduling (50 blocks x 20 vars, 100 rows)",
			Model:       buildBlockScheduling("t3_sched_1000", 50, 20, 304),
			BuildTrial: func(trial int) *gopt.Model {
				return buildBlockScheduling("t3_sched_1000", 50, 20, trialSeed(304, trial))
			},
		},
		CorpusCase{
			ID:          "t3_mad_g5_i20",
			Tier:        "tier3_large",
			Description: "135-var MAD group-item allocation (G=5 groups, I=20 items, T=6 periods)",
			Model:       buildMADAllocation("t3_mad_g5_i20", 5, 20, 6, 305),
			BuildTrial: func(trial int) *gopt.Model {
				return buildMADAllocation("t3_mad_g5_i20", 5, 20, 6, trialSeed(305, trial))
			},
		},
		CorpusCase{
			ID:          "t3_mad_g6_i30",
			Tier:        "tier3_large",
			Description: "222-var MAD group-item allocation (G=6 groups, I=30 items, T=6 periods)",
			Model:       buildMADAllocation("t3_mad_g6_i30", 6, 30, 6, 306),
			BuildTrial: func(trial int) *gopt.Model {
				return buildMADAllocation("t3_mad_g6_i30", 6, 30, 6, trialSeed(306, trial))
			},
		},
		CorpusCase{
			ID:          "t3_mad_g10_i100",
			Tier:        "tier3_large",
			Description: "1070-var MAD group-item allocation (G=10 groups, I=100 items, T=6 periods)",
			Model:       buildMADAllocation("t3_mad_g10_i100", 10, 100, 6, 307),
			BuildTrial: func(trial int) *gopt.Model {
				return buildMADAllocation("t3_mad_g10_i100", 10, 100, 6, trialSeed(307, trial))
			},
		},
		CorpusCase{
			ID:          "t3_mad_g4_i1000",
			Tier:        "tier3_large",
			Description: "4028-var MAD group-item allocation (G=4 groups, I=1000 items, T=6 periods)",
			Model:       buildMADAllocation("t3_mad_g4_i1000", 4, 1000, 6, 308),
			BuildTrial: func(trial int) *gopt.Model {
				return buildMADAllocation("t3_mad_g4_i1000", 4, 1000, 6, trialSeed(308, trial))
			},
		},
	)

	return cases
}

// WriteCorpusJSON writes all benchmark corpus problems as PuLP-compatible JSON files into dir,
// plus multi-seed randomized trial ensembles into dir/trials/<id>.jsonl.
func WriteCorpusJSON(dir string) ([]CorpusCase, error) {
	trialsDir := filepath.Join(dir, "trials")
	if err := os.MkdirAll(trialsDir, 0o755); err != nil {
		return nil, err
	}
	cases := BuildCorpus()
	for _, c := range cases {
		p := c.Model.Compile()
		path := filepath.Join(dir, c.ID+".json")
		f, err := os.Create(path)
		if err != nil {
			return nil, err
		}
		err = format.ToPuLPJSON(p, f)
		closeErr := f.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}

		numTrials := DefaultTrials(c.Tier)
		tf, err := os.Create(filepath.Join(trialsDir, c.ID+".jsonl"))
		if err != nil {
			return nil, err
		}
		for k := range numTrials {
			tp := c.TrialModel(k).Compile()
			if err := format.ToPuLPJSONCompact(tp, tf); err != nil {
				_ = tf.Close()
				return nil, err
			}
		}
		if err := tf.Close(); err != nil {
			return nil, err
		}
	}
	return cases, nil
}

func buildPuLPStdProblem(name string, sense gopt.Sense, mip bool, floatObj bool) *gopt.Model {
	return buildPuLPStdProblemTrial(name, sense, mip, floatObj, 0)
}

func buildPuLPStdProblemTrial(name string, sense gopt.Sense, mip bool, floatObj bool, trial int) *gopt.Model {
	m := gopt.NewModel(name, sense)
	cat := gopt.Continuous
	if mip {
		cat = gopt.Integer
	}
	x := m.AddContVar("x", 0, 4)
	y := m.AddContVar("y", -1, 1)
	z := m.AddVar("z", 0, 100, cat)

	cx, cy, cz := 1.0, 4.0, 9.0
	if floatObj {
		cx, cy, cz = 1.1, 4.1, 9.1
	}
	rhs1 := 5.0
	rhs2 := 10.0
	rhs3 := 7.0
	if mip {
		rhs3 = 7.5
	}
	if trial > 0 {
		seed := trialSeed(11, trial)
		rng := rand.New(rand.NewPCG(seed, seed^0x517cc1b727220a95))
		cx += 0.1 * float64(1+rng.IntN(15))
		cy += 0.1 * float64(1+rng.IntN(15))
		cz += 0.1 * float64(1+rng.IntN(20))
		rhs1 = 4.25 + 0.25*float64(rng.IntN(6))
		rhs2 = 9.0 + 0.5*float64(rng.IntN(5))
		rhs3 += float64(rng.IntN(5))
	}
	m.SetObjectiveNamed("obj", gopt.NewExpr(gopt.T(x, cx), gopt.T(y, cy), gopt.T(z, cz)))
	m.AddConstraint("c1", gopt.Le(gopt.NewExpr(gopt.T(x, 1), gopt.T(y, 1)), rhs1))
	m.AddConstraint("c2", gopt.Ge(gopt.NewExpr(gopt.T(x, 1), gopt.T(z, 1)), rhs2))
	m.AddConstraint("c3", gopt.Eq(gopt.NewExpr(gopt.T(y, -1), gopt.T(z, 1)), rhs3))
	return m
}

func buildWhiskasProblem() *gopt.Model {
	return buildWhiskasProblemTrial(0)
}

func buildWhiskasProblemTrial(trial int) *gopt.Model {
	m := gopt.NewModel("t1_whiskas", gopt.Minimize)
	x1 := m.AddContVar("Chicken", 0, 100)
	x2 := m.AddContVar("Beef", 0, 100)
	x3 := m.AddContVar("Mutton", 0, 100)
	x4 := m.AddContVar("Rice", 0, 100)
	x5 := m.AddContVar("Wheat", 0, 100)
	x6 := m.AddContVar("Gel", 0, 100)

	c1, c2, c3, c4, c5, c6 := 0.013, 0.008, 0.010, 0.002, 0.005, 0.001
	protRHS, fatRHS, fibreRHS, saltRHS := 8.0, 6.0, 2.0, 0.4
	if trial > 0 {
		seed := trialSeed(59, trial)
		rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
		c1 = 0.011 + 0.0005*float64(rng.IntN(9))
		c2 = 0.006 + 0.0005*float64(rng.IntN(9))
		c3 = 0.008 + 0.0005*float64(rng.IntN(9))
		c4 = 0.0015 + 0.0002*float64(rng.IntN(6))
		c5 = 0.0035 + 0.0003*float64(rng.IntN(6))
		c6 = 0.0008 + 0.0001*float64(rng.IntN(5))
		protRHS = 7.5 + 0.2*float64(rng.IntN(6))
		fatRHS = 5.5 + 0.2*float64(rng.IntN(6))
		fibreRHS = 1.8 + 0.1*float64(rng.IntN(5))
		saltRHS = 0.36 + 0.02*float64(rng.IntN(5))
	}

	m.SetObjectiveNamed("Cost", gopt.NewExpr(
		gopt.T(x1, c1), gopt.T(x2, c2), gopt.T(x3, c3),
		gopt.T(x4, c4), gopt.T(x5, c5), gopt.T(x6, c6),
	))
	m.AddConstraint("Sum", gopt.Eq(gopt.SumVars(x1, x2, x3, x4, x5, x6), 100))
	m.AddConstraint("Protein", gopt.Ge(gopt.NewExpr(
		gopt.T(x1, 0.100), gopt.T(x2, 0.200), gopt.T(x3, 0.150),
		gopt.T(x4, 0.000), gopt.T(x5, 0.040), gopt.T(x6, 0.000),
	), protRHS))
	m.AddConstraint("Fat", gopt.Ge(gopt.NewExpr(
		gopt.T(x1, 0.080), gopt.T(x2, 0.100), gopt.T(x3, 0.110),
		gopt.T(x4, 0.010), gopt.T(x5, 0.010), gopt.T(x6, 0.000),
	), fatRHS))
	m.AddConstraint("Fibre", gopt.Le(gopt.NewExpr(
		gopt.T(x1, 0.001), gopt.T(x2, 0.005), gopt.T(x3, 0.003),
		gopt.T(x4, 0.100), gopt.T(x5, 0.150), gopt.T(x6, 0.000),
	), fibreRHS))
	m.AddConstraint("Salt", gopt.Le(gopt.NewExpr(
		gopt.T(x1, 0.002), gopt.T(x2, 0.005), gopt.T(x3, 0.007),
		gopt.T(x4, 0.002), gopt.T(x5, 0.008), gopt.T(x6, 0.000),
	), saltRHS))
	return m
}

func buildFurnitureProblem() *gopt.Model {
	return buildFurnitureProblemTrial(0)
}

func buildFurnitureProblemTrial(trial int) *gopt.Model {
	m := gopt.NewModel("t1_furniture", gopt.Maximize)
	tables := m.AddIntVar("Tables", 0, 100)
	chairs := m.AddIntVar("Chairs", 0, 100)
	pTab, pCh := 45.0, 20.0
	woodRHS, laborRHS, demRHS := 400.0, 450.0, 10.0
	if trial > 0 {
		seed := trialSeed(67, trial)
		rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
		pTab = float64(35 + rng.IntN(25))
		pCh = float64(15 + rng.IntN(15))
		woodRHS = float64(340 + 10*rng.IntN(15))
		laborRHS = float64(390 + 15*rng.IntN(12))
		demRHS = float64(8 + rng.IntN(6))
	}
	m.SetObjectiveNamed("Profit", gopt.NewExpr(gopt.T(tables, pTab), gopt.T(chairs, pCh)))
	m.AddConstraint("Wood", gopt.Le(gopt.NewExpr(gopt.T(tables, 20), gopt.T(chairs, 5)), woodRHS))
	m.AddConstraint("Labor", gopt.Le(gopt.NewExpr(gopt.T(tables, 10), gopt.T(chairs, 15)), laborRHS))
	m.AddConstraint("Demand", gopt.Ge(gopt.NewExpr(gopt.T(tables, 1), gopt.T(chairs, 1)), demRHS))
	return m
}

func buildInfeasibleMIP() *gopt.Model {
	return buildInfeasibleMIPTrial(0)
}

func buildInfeasibleMIPTrial(trial int) *gopt.Model {
	m := gopt.NewModel("t1_infeasible_mip", gopt.Minimize)
	x := m.AddBinaryVar("x")
	y := m.AddBinaryVar("y")
	cx, cy := 1.0, 1.0
	rhs := 3.0
	if trial > 0 {
		seed := trialSeed(79, trial)
		rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
		cx = float64(1 + rng.IntN(9))
		cy = float64(1 + rng.IntN(9))
		if trial%2 == 1 {
			rhs = 1.0
		}
	}
	m.SetObjectiveNamed("obj", gopt.NewExpr(gopt.T(x, cx), gopt.T(y, cy)))
	m.AddConstraint("parity", gopt.Eq(gopt.NewExpr(gopt.T(x, 2), gopt.T(y, 2)), rhs))
	return m
}

// buildMixedKnapsack builds a realistic mixed-integer resource allocation problem where
// ~70% of variables are 0-1 binary, ~20% are general integers in [0, 5], and ~10% are continuous.
func buildMixedKnapsack(name string, nVars, nRows int, seed uint64) *gopt.Model {
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	m := gopt.NewModel(name, gopt.Maximize)

	vars := make([]gopt.Var, nVars)
	objTerms := make([]gopt.Term, nVars)
	for j := range nVars {
		vname := fmt.Sprintf("x_%04d", j)
		rem := j % 10
		if rem < 7 {
			vars[j] = m.AddBinaryVar(vname)
		} else if rem < 9 {
			vars[j] = m.AddIntVar(vname, 0, 5)
		} else {
			vars[j] = m.AddContVar(vname, 0, 10)
		}
		// Add small jitter so objective optima are unique
		val := float64(10+rng.IntN(90)) + float64(j+1)*1e-3
		objTerms[j] = gopt.T(vars[j], val)
	}
	m.SetObjectiveNamed("total_value", gopt.Expr{Terms: objTerms})

	density := 0.45
	if nVars > 100 {
		density = 0.15
	}
	for i := range nRows {
		var terms []gopt.Term
		rowSum := 0.0
		for j := range nVars {
			if rng.Float64() < density || (j%nRows == i) {
				w := float64(1 + rng.IntN(25))
				terms = append(terms, gopt.T(vars[j], w))
				_, ub := m.VarBounds(vars[j])
				rowSum += w * min(ub, 2.0)
			}
		}
		capRHS := math.Floor(0.38 * rowSum)
		if capRHS < 10 {
			capRHS = 10
		}
		m.AddConstraint(fmt.Sprintf("res_%03d", i), gopt.Le(gopt.Expr{Terms: terms}, capRHS))
	}
	return m
}

// buildFacilityLocation creates a Capacitated Facility Location Problem (CFLP) with
// nFac binary facility-opening variables and nFac*nCust allocation variables (mixed integer and continuous).
func buildFacilityLocation(name string, nFac, nCust int, seed uint64) *gopt.Model {
	rng := rand.New(rand.NewPCG(seed, seed^0xdeadbeef))
	m := gopt.NewModel(name, gopt.Minimize)

	openVars := make([]gopt.Var, nFac)
	flowVars := make([][]gopt.Var, nFac)
	var objTerms []gopt.Term

	for f := range nFac {
		openVars[f] = m.AddBinaryVar(fmt.Sprintf("open_%02d", f))
		fixedCost := float64(120+rng.IntN(150)) + float64(f)*0.01
		objTerms = append(objTerms, gopt.T(openVars[f], fixedCost))
		flowVars[f] = make([]gopt.Var, nCust)
	}

	demands := make([]float64, nCust)
	totalDemand := 0.0
	for c := range nCust {
		demands[c] = float64(5 + rng.IntN(12))
		totalDemand += demands[c]
	}
	capPerFac := math.Ceil(totalDemand / float64(max(1, nFac/2)))

	for f := range nFac {
		for c := range nCust {
			vname := fmt.Sprintf("flow_%02d_%02d", f, c)
			if (f+c)%3 == 0 {
				// Continuous fractional split flow
				flowVars[f][c] = m.AddContVar(vname, 0, demands[c])
			} else {
				// Integer unit shipment
				flowVars[f][c] = m.AddIntVar(vname, 0, demands[c])
			}
			unitCost := float64(3+rng.IntN(20)) + float64(f*nCust+c)*1e-3
			objTerms = append(objTerms, gopt.T(flowVars[f][c], unitCost))
		}
	}
	m.SetObjectiveNamed("total_cost", gopt.Expr{Terms: objTerms})

	// Demand satisfaction constraints for each customer
	for c := range nCust {
		terms := make([]gopt.Term, nFac)
		for f := range nFac {
			terms[f] = gopt.T(flowVars[f][c], 1.0)
		}
		m.AddConstraint(fmt.Sprintf("dem_%02d", c), gopt.Ge(gopt.Expr{Terms: terms}, demands[c]))
	}

	// Capacity linking constraints: sum_c flow[f][c] - capPerFac * open[f] <= 0
	for f := range nFac {
		terms := make([]gopt.Term, 0, nCust+1)
		for c := range nCust {
			terms = append(terms, gopt.T(flowVars[f][c], 1.0))
		}
		terms = append(terms, gopt.T(openVars[f], -capPerFac))
		m.AddConstraint(fmt.Sprintf("cap_%02d", f), gopt.Le(gopt.Expr{Terms: terms}, 0))
	}

	return m
}

// buildGeneralizedAssignment creates a Generalized Assignment Problem with nAgents * nTasks
// 0-1 assignment variables plus nAgents general integer overtime variables.
func buildGeneralizedAssignment(name string, nAgents, nTasks int, seed uint64) *gopt.Model {
	rng := rand.New(rand.NewPCG(seed, seed^0xabcdef01))
	m := gopt.NewModel(name, gopt.Minimize)

	assign := make([][]gopt.Var, nAgents)
	overtime := make([]gopt.Var, nAgents)
	var objTerms []gopt.Term

	for a := range nAgents {
		assign[a] = make([]gopt.Var, nTasks)
		for t := range nTasks {
			assign[a][t] = m.AddBinaryVar(fmt.Sprintf("x_%d_%d", a, t))
			cost := float64(10+rng.IntN(40)) + float64(a*nTasks+t)*1e-3
			objTerms = append(objTerms, gopt.T(assign[a][t], cost))
		}
	}
	for a := range nAgents {
		overtime[a] = m.AddIntVar(fmt.Sprintf("ot_%d", a), 0, 15)
		objTerms = append(objTerms, gopt.T(overtime[a], 25.0+float64(a)*0.1))
	}
	m.SetObjectiveNamed("cost", gopt.Expr{Terms: objTerms})

	// Each task assigned to exactly 1 agent
	for t := range nTasks {
		terms := make([]gopt.Term, nAgents)
		for a := range nAgents {
			terms[a] = gopt.T(assign[a][t], 1.0)
		}
		m.AddConstraint(fmt.Sprintf("task_%d", t), gopt.Eq(gopt.Expr{Terms: terms}, 1.0))
	}

	// Agent capacity with integer overtime
	for a := range nAgents {
		terms := make([]gopt.Term, 0, nTasks+1)
		totalReq := 0.0
		for t := range nTasks {
			req := float64(4 + rng.IntN(10))
			totalReq += req
			terms = append(terms, gopt.T(assign[a][t], req))
		}
		terms = append(terms, gopt.T(overtime[a], -1.0))
		regCap := math.Floor(totalReq / float64(nAgents) * 0.85)
		m.AddConstraint(fmt.Sprintf("agent_cap_%d", a), gopt.Le(gopt.Expr{Terms: terms}, regCap))
	}
	return m
}

// buildLotSizing builds a Capacitated Lot-Sizing problem with 3 variables per period:
// binary setup y_t, integer production batch p_t, and continuous inventory s_t.
func buildLotSizing(name string, nPeriods int, seed uint64) *gopt.Model {
	rng := rand.New(rand.NewPCG(seed, seed^0x12345678))
	m := gopt.NewModel(name, gopt.Minimize)

	setup := make([]gopt.Var, nPeriods)
	prod := make([]gopt.Var, nPeriods)
	inv := make([]gopt.Var, nPeriods)
	var objTerms []gopt.Term

	maxBatch := 25.0
	for t := range nPeriods {
		setup[t] = m.AddBinaryVar(fmt.Sprintf("setup_%03d", t))
		prod[t] = m.AddIntVar(fmt.Sprintf("prod_%03d", t), 0, maxBatch)
		inv[t] = m.AddContVar(fmt.Sprintf("inv_%03d", t), 0, 100)

		setupCost := float64(80+rng.IntN(60)) + float64(t)*1e-3
		prodCost := float64(4 + rng.IntN(6))
		holdCost := 1.5 + float64(t%3)*0.25
		objTerms = append(objTerms,
			gopt.T(setup[t], setupCost),
			gopt.T(prod[t], prodCost),
			gopt.T(inv[t], holdCost),
		)
	}
	m.SetObjectiveNamed("total_lot_cost", gopt.Expr{Terms: objTerms})

	for t := range nPeriods {
		demand := float64(6 + rng.IntN(10))
		// Flow balance: inv[t-1] + prod[t] - inv[t] == demand
		var balTerms []gopt.Term
		if t > 0 {
			balTerms = append(balTerms, gopt.T(inv[t-1], 1.0))
		}
		balTerms = append(balTerms, gopt.T(prod[t], 1.0), gopt.T(inv[t], -1.0))
		m.AddConstraint(fmt.Sprintf("bal_%03d", t), gopt.Eq(gopt.Expr{Terms: balTerms}, demand))

		// Setup linking: prod[t] - maxBatch * setup[t] <= 0
		m.AddConstraint(fmt.Sprintf("link_%03d", t), gopt.Le(
			gopt.NewExpr(gopt.T(prod[t], 1.0), gopt.T(setup[t], -maxBatch)),
			0,
		))
	}
	return m
}

// buildMultiItemLotSizing builds a Multi-Item Capacitated Lot-Sizing problem with
// nItems * nPeriods * 3 variables (binary setup, integer production, continuous inventory).
func buildMultiItemLotSizing(name string, nItems, nPeriods int, seed uint64) *gopt.Model {
	rng := rand.New(rand.NewPCG(seed, seed^0x33557799))
	m := gopt.NewModel(name, gopt.Minimize)

	var objTerms []gopt.Term
	maxBatch := 25.0

	for item := range nItems {
		setup := make([]gopt.Var, nPeriods)
		prod := make([]gopt.Var, nPeriods)
		inv := make([]gopt.Var, nPeriods)

		for t := range nPeriods {
			setup[t] = m.AddBinaryVar(fmt.Sprintf("setup_%02d_%02d", item, t))
			prod[t] = m.AddIntVar(fmt.Sprintf("prod_%02d_%02d", item, t), 0, maxBatch)
			inv[t] = m.AddContVar(fmt.Sprintf("inv_%02d_%02d", item, t), 0, 100)

			setupCost := float64(50+rng.IntN(40)) + float64(item*nPeriods+t)*1e-3
			prodCost := float64(3 + rng.IntN(5))
			holdCost := 1.0 + float64((item+t)%3)*0.25
			objTerms = append(objTerms,
				gopt.T(setup[t], setupCost),
				gopt.T(prod[t], prodCost),
				gopt.T(inv[t], holdCost),
			)
		}

		for t := range nPeriods {
			demand := float64(5 + rng.IntN(8))
			var balTerms []gopt.Term
			if t > 0 {
				balTerms = append(balTerms, gopt.T(inv[t-1], 1.0))
			}
			balTerms = append(balTerms, gopt.T(prod[t], 1.0), gopt.T(inv[t], -1.0))
			m.AddConstraint(fmt.Sprintf("bal_%02d_%02d", item, t), gopt.Eq(gopt.Expr{Terms: balTerms}, demand))

			m.AddConstraint(fmt.Sprintf("link_%02d_%02d", item, t), gopt.Le(
				gopt.NewExpr(gopt.T(prod[t], 1.0), gopt.T(setup[t], -maxBatch)),
				0,
			))
		}
	}
	m.SetObjectiveNamed("multi_lot_cost", gopt.Expr{Terms: objTerms})
	return m
}

// buildBlockScheduling builds a 1000-variable mixed binary/integer/continuous scheduling
// problem with nBlocks * varsPerBlock variables and 2 * nBlocks constraints.
func buildBlockScheduling(name string, nBlocks, varsPerBlock int, seed uint64) *gopt.Model {
	rng := rand.New(rand.NewPCG(seed, seed^0x778899aa))
	m := gopt.NewModel(name, gopt.Maximize)

	var objTerms []gopt.Term
	for b := range nBlocks {
		bVars := make([]gopt.Var, varsPerBlock)
		var capTerms, covTerms []gopt.Term
		rowSum := 0.0
		for j := range varsPerBlock {
			idx := b*varsPerBlock + j
			vname := fmt.Sprintf("x_%04d", idx)
			rem := j % 10
			if rem < 7 {
				bVars[j] = m.AddBinaryVar(vname)
			} else if rem < 9 {
				bVars[j] = m.AddIntVar(vname, 0, 4)
			} else {
				bVars[j] = m.AddContVar(vname, 0, 8)
			}
			val := float64(15+rng.IntN(70)) + float64(idx+1)*1e-3
			objTerms = append(objTerms, gopt.T(bVars[j], val))

			w := float64(2 + rng.IntN(12))
			capTerms = append(capTerms, gopt.T(bVars[j], w))
			rowSum += w
			if j < varsPerBlock/2 {
				covTerms = append(covTerms, gopt.T(bVars[j], 1.0))
			}
		}
		m.AddConstraint(fmt.Sprintf("bcap_%03d", b), gopt.Le(gopt.Expr{Terms: capTerms}, math.Floor(0.45*rowSum)))
		m.AddConstraint(fmt.Sprintf("bcov_%03d", b), gopt.Ge(gopt.Expr{Terms: covTerms}, 2.0))
	}
	m.SetObjectiveNamed("sched_val", gopt.Expr{Terms: objTerms})
	return m
}

// buildBinPacking builds a Bin Packing problem with nBins binary bin-open variables
// and nBins*nItems binary item-to-bin assignment variables.
func buildBinPacking(name string, nBins, nItems int, seed uint64) *gopt.Model {
	rng := rand.New(rand.NewPCG(seed, seed^0x55aa55aa))
	m := gopt.NewModel(name, gopt.Minimize)

	binOpen := make([]gopt.Var, nBins)
	assign := make([][]gopt.Var, nBins)
	var objTerms []gopt.Term

	for b := range nBins {
		// Break symmetry slightly with progressive bin cost
		binOpen[b] = m.AddBinaryVar(fmt.Sprintf("bin_%02d", b))
		objTerms = append(objTerms, gopt.T(binOpen[b], 10.0+float64(b)*0.1))
		assign[b] = make([]gopt.Var, nItems)
		for i := range nItems {
			assign[b][i] = m.AddBinaryVar(fmt.Sprintf("x_%02d_%02d", b, i))
			objTerms = append(objTerms, gopt.T(assign[b][i], float64(b+1)*1e-3))
		}
	}
	m.SetObjectiveNamed("bins_used", gopt.Expr{Terms: objTerms})

	weights := make([]float64, nItems)
	totalW := 0.0
	for i := range nItems {
		weights[i] = float64(4 + rng.IntN(9))
		totalW += weights[i]
	}
	binCap := math.Ceil(totalW / float64(nBins) * 1.35)

	for i := range nItems {
		terms := make([]gopt.Term, nBins)
		for b := range nBins {
			terms[b] = gopt.T(assign[b][i], 1.0)
		}
		m.AddConstraint(fmt.Sprintf("item_%02d", i), gopt.Eq(gopt.Expr{Terms: terms}, 1.0))
	}

	for b := range nBins {
		terms := make([]gopt.Term, 0, nItems+1)
		for i := range nItems {
			terms = append(terms, gopt.T(assign[b][i], weights[i]))
		}
		terms = append(terms, gopt.T(binOpen[b], -binCap))
		m.AddConstraint(fmt.Sprintf("bincap_%02d", b), gopt.Le(gopt.Expr{Terms: terms}, 0))
	}
	return m
}

// buildSetCovering builds a Set Covering problem with nVars variables (40 binary + 10 integer)
// and nRows covering constraints.
func buildSetCovering(name string, nVars, nRows int, seed uint64) *gopt.Model {
	rng := rand.New(rand.NewPCG(seed, seed^0x87654321))
	m := gopt.NewModel(name, gopt.Minimize)

	vars := make([]gopt.Var, nVars)
	objTerms := make([]gopt.Term, nVars)
	for j := range nVars {
		if j%5 == 4 {
			vars[j] = m.AddIntVar(fmt.Sprintf("s_%03d", j), 0, 3)
		} else {
			vars[j] = m.AddBinaryVar(fmt.Sprintf("s_%03d", j))
		}
		cost := float64(5+rng.IntN(30)) + float64(j)*1e-3
		objTerms[j] = gopt.T(vars[j], cost)
	}
	m.SetObjectiveNamed("cover_cost", gopt.Expr{Terms: objTerms})

	for i := range nRows {
		var terms []gopt.Term
		for j := range nVars {
			if rng.Float64() < 0.25 || (j%nRows == i) {
				terms = append(terms, gopt.T(vars[j], 1.0))
			}
		}
		rhs := 1.0
		if i%4 == 0 {
			rhs = 2.0
		}
		m.AddConstraint(fmt.Sprintf("cov_%03d", i), gopt.Ge(gopt.Expr{Terms: terms}, rhs))
	}
	return m
}

// buildMADAllocation builds a Mean Absolute Deviation (MAD) group-item allocation problem
// with nG groups (G), nI items (I), and nT periods (T).
// Subset D = {nI - nG, ..., nI - 1} consists of nG designated items where each group g
// receives exactly one item from D, while every item i in {0, ..., nI - 1} is assigned
// to exactly one group g.
func buildMADAllocation(name string, nG, nI, nT int, seed uint64) *gopt.Model {
	rng := rand.New(rand.NewPCG(seed, seed^0x4d41445f4749))
	m := gopt.NewModel(name, gopt.Minimize)

	// Generate deterministic non-negative period weight matrix W[t][i] (size nT x nI).
	// Each group g has a zero-mean signature vector u[g] in the subspace orthogonal to both
	// the constant vector 1 and the L1 subgradient sign(eta), parameterized along the 4D
	// moment curve so group cones are extreme. Items in S = {0 .. nI-nG-1} carry positive
	// multiples of u[g*(i)], and designated items in D = {nI-nG .. nI-1} carry the
	// complementary hedge plus a co-monotonic macro trajectory (g+1)*eta[t].
	w := make([][]float64, nT)
	for t := range nT {
		w[t] = make([]float64, nI)
	}

	e1 := []int{1, -1, 0, 0, 0, 0}
	e2 := []int{1, 1, -2, 0, 0, 0}
	e3 := []int{0, 0, 0, 1, -1, 0}
	e4 := []int{0, 0, 0, 1, 1, -2}

	u := make([][]int, nG)
	for g := range nG {
		tau := float64(g+1) / float64(nG+1)
		c1 := int(math.Round(60.0 * tau))
		c2 := int(math.Round(60.0 * (tau * tau)))
		c3 := int(math.Round(60.0 * (tau * tau * tau)))
		c4 := int(math.Round(60.0*(tau*tau*tau*tau))) + (g + 1)
		u[g] = make([]int, nT)
		for t := range nT {
			tMod := t % 6
			u[g][t] = c1*e1[tMod] + c2*e2[tMod] + c3*e3[tMod] + c4*e4[tMod]
		}
	}

	dStart := nI - nG
	sumA := make([]int, nG)
	for i := range dStart {
		gStar := i % nG
		aI := 4
		sumA[gStar] += aI
		minU := 0
		for _, v := range u[gStar] {
			if v < minU {
				minU = v
			}
		}
		baseI := 600 + rng.IntN(901) - minU*aI
		for t := range nT {
			w[t][i] = float64(baseI + aI*u[gStar][t])
		}
	}

	scaleEta := max(1, (nI/(nG*10))+1)
	baseEta := []float64{-180.0, -120.0, -60.0, 60.0, 120.0, 180.0}
	eta := make([]float64, nT)
	for t := range nT {
		eta[t] = float64(scaleEta) * baseEta[t%6]
	}

	for g := range nG {
		iD := dStart + g
		alphaG := float64(g + 1)
		minTerm := 0.0
		for t := range nT {
			term := float64(-sumA[g]*u[g][t]) + alphaG*eta[t]
			if term < minTerm {
				minTerm = term
			}
		}
		baseD := float64(5000+1000*g) - minTerm
		for t := range nT {
			w[t][iD] = math.Round(baseD + float64(-sumA[g]*u[g][t]) + alphaG*eta[t])
		}
	}

	x := make([][]gopt.Var, nG)
	z := make([][]gopt.Var, nG)
	central := make([]gopt.Var, nG)
	objTerms := make([]gopt.Term, 0, nG*nT)

	for g := range nG {
		x[g] = make([]gopt.Var, nI)
		for i := range nI {
			x[g][i] = m.AddBinaryVar(fmt.Sprintf("x_%02d_%02d", g, i))
		}
		z[g] = make([]gopt.Var, nT)
		for t := range nT {
			z[g][t] = m.AddContVar(fmt.Sprintf("z_%02d_%02d", g, t), 0, math.Inf(1))
			objTerms = append(objTerms, gopt.T(z[g][t], 1.0))
		}
		central[g] = m.AddContVar(fmt.Sprintf("m_%02d", g), 0, math.Inf(1))
	}
	m.SetObjectiveNamed("total_mad", gopt.Expr{Terms: objTerms})

	// 1. Absolute deviation linearization for each group g and period t:
	//    z[g][t] >= sum_i W[t][i]*x[g][i] - m[g]  <=>  z[g][t] - sum_i W[t][i]*x[g][i] + m[g] >= 0
	//    z[g][t] >= m[g] - sum_i W[t][i]*x[g][i]  <=>  z[g][t] + sum_i W[t][i]*x[g][i] - m[g] >= 0
	for g := range nG {
		for t := range nT {
			posTerms := make([]gopt.Term, 0, nI+2)
			negTerms := make([]gopt.Term, 0, nI+2)
			posTerms = append(posTerms, gopt.T(z[g][t], 1.0), gopt.T(central[g], 1.0))
			negTerms = append(negTerms, gopt.T(z[g][t], 1.0), gopt.T(central[g], -1.0))
			for i := range nI {
				wt := w[t][i]
				if wt != 0 {
					posTerms = append(posTerms, gopt.T(x[g][i], -wt))
					negTerms = append(negTerms, gopt.T(x[g][i], wt))
				}
			}
			m.AddConstraint(fmt.Sprintf("dev_pos_%02d_%02d", g, t), gopt.Ge(gopt.Expr{Terms: posTerms}, 0))
			m.AddConstraint(fmt.Sprintf("dev_neg_%02d_%02d", g, t), gopt.Ge(gopt.Expr{Terms: negTerms}, 0))
		}
	}

	// 2. Each item i must be assigned to exactly one group g:
	//    sum_{g=0..nG-1} x[g][i] == 1
	for i := range nI {
		terms := make([]gopt.Term, nG)
		for g := range nG {
			terms[g] = gopt.T(x[g][i], 1.0)
		}
		m.AddConstraint(fmt.Sprintf("item_%02d", i), gopt.Eq(gopt.Expr{Terms: terms}, 1.0))
	}

	// 3. Each group g must receive exactly one item from subset D = {nI-nG .. nI-1}:
	//    sum_{i in D} x[g][i] == 1
	for g := range nG {
		terms := make([]gopt.Term, 0, nG)
		for i := dStart; i < nI; i++ {
			terms = append(terms, gopt.T(x[g][i], 1.0))
		}
		m.AddConstraint(fmt.Sprintf("group_d_%02d", g), gopt.Eq(gopt.Expr{Terms: terms}, 1.0))
	}

	return m
}

// BuildMADRandom constructs an unstructured random-weight Mean Absolute Deviation (MAD)
// group-item allocation MILP (negative control contrasting with the engineered orthogonal-signature
// buildMADAllocation instances where the LP relaxation is tight at the root node).
func BuildMADRandom(name string, nG, nI, nT int, seed uint64) *gopt.Model {
	rng := rand.New(rand.NewPCG(seed, seed^0x77))
	m := gopt.NewModel(name, gopt.Minimize)

	w := make([][]float64, nT)
	for t := range nT {
		w[t] = make([]float64, nI)
		for i := range nI {
			w[t][i] = float64(100 + rng.IntN(1900))
		}
	}

	x := make([][]gopt.Var, nG)
	z := make([][]gopt.Var, nG)
	cen := make([]gopt.Var, nG)
	obj := make([]gopt.Term, 0, nG*nT)

	for g := range nG {
		x[g] = make([]gopt.Var, nI)
		for i := range nI {
			x[g][i] = m.AddBinaryVar(fmt.Sprintf("x_%02d_%02d", g, i))
		}
		z[g] = make([]gopt.Var, nT)
		for t := range nT {
			z[g][t] = m.AddContVar(fmt.Sprintf("z_%02d_%02d", g, t), 0, math.Inf(1))
			obj = append(obj, gopt.T(z[g][t], 1.0))
		}
		cen[g] = m.AddContVar(fmt.Sprintf("m_%02d", g), 0, math.Inf(1))
	}
	m.SetObjectiveNamed("total_mad", gopt.Expr{Terms: obj})

	for g := range nG {
		for t := range nT {
			pos := make([]gopt.Term, 0, nI+2)
			neg := make([]gopt.Term, 0, nI+2)
			pos = append(pos, gopt.T(z[g][t], 1.0), gopt.T(cen[g], 1.0))
			neg = append(neg, gopt.T(z[g][t], 1.0), gopt.T(cen[g], -1.0))
			for i := range nI {
				pos = append(pos, gopt.T(x[g][i], -w[t][i]))
				neg = append(neg, gopt.T(x[g][i], w[t][i]))
			}
			m.AddConstraint(fmt.Sprintf("dev_pos_%02d_%02d", g, t), gopt.Ge(gopt.Expr{Terms: pos}, 0))
			m.AddConstraint(fmt.Sprintf("dev_neg_%02d_%02d", g, t), gopt.Ge(gopt.Expr{Terms: neg}, 0))
		}
	}

	for i := range nI {
		terms := make([]gopt.Term, nG)
		for g := range nG {
			terms[g] = gopt.T(x[g][i], 1.0)
		}
		m.AddConstraint(fmt.Sprintf("item_%02d", i), gopt.Eq(gopt.Expr{Terms: terms}, 1.0))
	}

	lo := float64(nI / nG)
	hi := math.Ceil(float64(nI) / float64(nG))
	for g := range nG {
		terms := make([]gopt.Term, nI)
		for i := range nI {
			terms[i] = gopt.T(x[g][i], 1.0)
		}
		m.AddConstraint(fmt.Sprintf("lo_%02d", g), gopt.Ge(gopt.Expr{Terms: terms}, lo))
		m.AddConstraint(fmt.Sprintf("hi_%02d", g), gopt.Le(gopt.Expr{Terms: terms}, hi))
	}
	return m
}
