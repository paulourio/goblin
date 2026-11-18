# `goblin`: Pure-Go LP, MILP, and Linear Sum Assignment Solver with Optional C++ CBC Backend

The `goblin` module provides an in-memory Linear Programming (LP) and Mixed-Integer Linear Programming (MILP) modeling engine and Branch-and-Cut solver alongside the [`goblin/assign`](assign/assign.go) Linear Sum Assignment Problem (`LSAP`) solver, written in Go 1.27. It compiles algebraic models directly into encapsulated Dual-Index Compressed Sparse Row and Column (`CSR`/`CSC`) matrices (`CompiledProblem`), eliminating intermediate text-file I/O and subprocess spawn costs. By default it builds as pure Go with zero `cgo` dependency (`BackendGo`), and nested module isolation (`third_party/go.mod`) keeps `go get github.com/paulourio/goblin` lightweight; callers who build with `-tags cbc` (`cgo` enabled, or `-tags "cbc,cbc_system"` against system COIN-OR packages) can also select the in-process C++ COIN-OR CBC/CLP backend (`WithCBC()` / `WithBackend(BackendCBC)`) or configure solver flags (`WithoutSIMD()`, `WithTimeLimit(...)`, etc.) through functional arguments on `Solve` and `SolveCompiled`.

## Quickstart

Install the module with `go get github.com/paulourio/goblin`.

### 1. Mixed-Integer Linear Programming (`goblin`)

Import `github.com/paulourio/goblin` as `gb`, construct a model with [`NewModel`](model.go), add continuous, integer, or binary variables, build terms and constraints with `x.T(coeff)` and `expr.Le(rhs)` (or `gb.T` / `gb.Le`), and call `m.Solve(ctx, ...)`:

```go
package main

import (
	"context"
	"fmt"
	"math"
	"time"

	gb "github.com/paulourio/goblin"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	m := gb.NewModel("std_mip", gb.Minimize)

	x := m.AddContVar("x", 0, 4)
	y := m.AddContVar("y", -1, 1)
	z := m.AddIntVar("z", 0, math.Inf(1))

	m.SetObjective(gb.NewExpr(
		x.T(1),
		y.T(4),
		z.T(9),
	))

	m.AddConstraint("c1", gb.NewExpr(x.T(1), y.T(1)).Le(5))
	m.AddConstraint("c2", gb.NewExpr(x.T(1), z.T(1)).Ge(10))
	m.AddConstraint("c3", gb.NewExpr(y.T(-1), z.T(1)).Eq(7.5))

	// Default pure-Go backend:
	res, err := m.Solve(ctx)
	if err != nil {
		panic(err)
	}
	fmt.Printf("status=%v objective=%.2f x=%.1f y=%.1f z=%.1f\n",
		res.Status, res.Objective, res.Value(x), res.Value(y), res.Value(z))

	// Or solve with the optional C++ COIN-OR CBC backend (requires -tags cbc) or with SIMD disabled:
	// resCBC, err := m.Solve(ctx, gb.WithCBC())
	// resScalar, err := m.Solve(ctx, gb.WithoutSIMD())
}
```

### 2. Linear Sum Assignment (`goblin/assign`)

Import `github.com/paulourio/goblin/assign` and call [`assign.LinearSum`](assign/assign.go) (or [`assign.LinearSumFlat`](assign/assign.go) for a contiguous row-major `[]float64` slice) to solve a square or rectangular bipartite assignment problem without constructing an algebraic `Model`:

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/paulourio/goblin/assign"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	res, err := assign.LinearSum(ctx, [][]float64{
		{90, 75, 75, 80},
		{35, 85, 55, 65},
		{125, 95, 90, 105},
		{45, 110, 95, 115},
	}, assign.Minimize)
	if err != nil {
		panic(err)
	}
	fmt.Printf("cost=%.0f rows=%v cols=%v\n", res.Objective, res.RowInd, res.ColInd)
}
```

---

## Architecture and Core Guarantees

1. **Zero-Subprocess In-Memory Execution**: Modeling (`Model`), matrix checks and assembly (`Model.Validate()`, `Model.CompileChecked()`), presolve (`Presolve`), root and node LPs (`LPSolver`), and Branch-and-Cut (`SolveCompiled`) execute in-process in pure Go using pooled workspace buffers (`AcquireWorkspace` / `ReleaseWorkspace`) with zero `cgo` dependency by default, plus an optional `-tags cbc` in-process `cgo` bridge to COIN-OR CBC/CLP (`cbc_cgo.go`).
2. **Schema Compatibility with Python `PuLP` and Free-Format MPS Export**: The `goblin/format` package reads and writes `PuLP`'s native `toDict()` / `fromDict()` JSON schema ([`format/pulp_json.go`](format/pulp_json.go)) and writes free-format MPS files ([`format/mps.go`](format/mps.go)).
3. **Optional SIMD Kernels**: With `GOEXPERIMENT=simd` on `amd64`, [`internal/simd/kernels_amd64.go`](internal/simd/kernels_amd64.go) dispatches 256-bit AVX2 `archsimd.Float64x4` instructions (`VFMADD213PD` in `Dot64`; `VMULPD` + `VADDPD` and `VSUBPD` + `VMINPD` in `Axpy64`, `MinReducedSlice64`, `InitReducedRow64`, and `Min2ReducedRow64`; 8-lane `Abs().LessEqual` in `AllAbsLE64`) with `VZEROUPPER` cleanup and IEEE-754 `NaN` propagation parity for slices of length 32 or more, and can be toggled at runtime via `WithoutSIMD()` or `WithSIMD(false)`. See the paired comparison in [`docs/benchmarks.md`](docs/benchmarks.md#5-default-build-vs-goexperimentsimd).
4. **Verified Parity on Synthetic and Public Suites**: [`solver_test.go`](solver_test.go) re-solves all 293 distinct randomized trial instances of the 31 synthetic benchmark families (`testdata/corpus/trials/*.jsonl`), compares status and objective with `PuLP + COIN-OR CBC` (`c1c171ec`) and `BackendCBC`, and verifies primal and row feasibility on every result. [`public_benchmarks_test.go`](public_benchmarks_test.go) verifies parity across 17 held-out public Netlib LP and MIPLIB/COIN-OR MILP instances (`testdata/public/`), while [`regression_test.go`](regression_test.go) tests big-M fixed-charge models against enumerated ground truth and Farkas-certified LP scaling edge cases. `TimeLimit`, `MaxNodes`, and `MaxIter` return `StatusTimeLimit`, `StatusNodeLimit`, or `StatusIterationLimit` with a valid global bound once the root LP is solved; a node that cannot be evaluated is never discarded silently.

---

## Documentation Index

Full technical guides live in the [`docs/`](docs/index.md) directory:

- **[Architecture and Design Overview](docs/index.md)**: Module boundaries, data flow from algebraic expressions to encapsulated `CSR`/`CSC` matrices, and memory pooling invariants.
- **[Modeling API and Serialization Formats](docs/modeling-and-formats.md)**: Constructing variables, linear expressions, and constraints, configuring solver flags and backends, validating and compiling models, and exchanging `PuLP` JSON and MPS payloads.
- **[Solver Internals and Mathematical Specification](docs/solver-internals.md)**: Three-stage singleton and sparse Markowitz LU with `FTRANSparseHint`/`BTRANSparseHint`, `BTRANExact`, and PFI eta updates, Revised Dual Simplex with Bound-Flipping Ratio Test (`selectDualBFRT`), Two-Phase Primal Simplex, MIP presolve with algebraic orbit symmetry breaking, cutting planes (`GMI`, Sequentially Lifted Knapsack Covers, Flow/VUB, one-sided `c-MIR`) with slack-cut purging, primal heuristics, and Branch-and-Cut search.
- **[Benchmark Methodology and Corpus Results](docs/benchmarks.md)**: Hardware setup, single-session comparative telemetry against Python `PuLP + CBC` (`c1c171ec`) and in-process `BackendCBC`, paired scalar-vs-SIMD statistics, structural analysis of the synthetic Mean Absolute Deviation (`MAD`) `G x I` allocation family vs. random-weight negative controls (`BuildMADRandom`), and full checkpoint telemetry.

---

## Benchmarks

All benchmark tables in `README.md`, [`docs/benchmarks.md`](docs/benchmarks.md), and [`bench/results/final_report.md`](bench/results/final_report.md) are rendered deterministically from the seven JSON telemetry files in `bench/results/` (`python_baseline.json`, `goblin_cbc.json`, `goblin_scalar.json`, `goblin_simd.json`, `scipy_assign_baseline.json`, `goblin_assign_scalar.json`, `goblin_assign_simd.json`). [`doc_sync_test.go`](doc_sync_test.go) fails `go test ./...` if any table drifts from the telemetry, if any `goblin` build disagrees with CBC or SciPy on a status or objective, or if any telemetry file differs in git commit, source/corpus fingerprint, or host kernel. Headline timings use the default pure-Go scalar build (`BackendGo`).

<!-- BEGIN GENERATED:PROVENANCE -->
- **Unified telemetry source**: fingerprint `a33eec8017a28703` (working tree on top of commit `4fd4211ec2eeeef813d830171c636489f7564cff`), `go1.27.3-X:nodwarf5`, Linux kernel `6.18.63-gentoo-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 7.32 ms median on a 1-variable MILP and 7.18 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

Read the synthetic corpus ratios with three methodological notes. First, Tier 1 micro-models (`N <= 26`) are dominated by Python's ~1.3 to 2.0 ms CBC subprocess spawn cost. Second, the planted-gap MAD family has a zero root LP gap by construction and closes at node 1. Consequently, the bold row (`Tier 2+3 non-planted MILPs`) isolates solver-dominated MILPs across paired randomized instances, alongside total subset wall times and full per-instance `[Min-Max]` ranges.

### 1. Aggregate Synthetic Workload Summary (Paired Per-Instance Geometric Mean and Wall-Clock Sums)

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Instances) | Paired `Py / Go` GeoMean [95% CI] | Paired `Go / CBC` | `Go` Sum (ms) | `CBC` Sum (ms) | `Py` Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **362.97x** [295.35x-446.09x] | 0.05x | 13.8 | 208.6 | 1221.7 | 552.09x [14.41x-2058.85x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **18.99x** [15.63x-23.06x] | 0.17x | 886.4 | 2109.6 | 4911.5 | 22.13x [0.17x-149.83x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **7.82x** [5.54x-11.05x] | 0.27x | 4780.0 | 3231.4 | 6752.3 | 9.51x [0.06x-33.43x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **12.14x** [9.75x-15.13x] | 0.23x | 5329.4 | 3359.2 | 7551.4 | 12.50x [0.06x-149.83x] |
| Tier 3 non-MAD MILPs | 4 (20) | **4.76x** [2.58x-8.77x] | 0.45x | 4452.3 | 1350.4 | 3038.2 | 6.53x [0.06x-19.66x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **35.54x** [28.86x-43.76x] | 0.09x | 338.3 | 2008.2 | 4234.3 | 42.52x [3.33x-106.87x] |
| Random-weight MAD (negative controls) | 3 (33) | **13.58x** [9.01x-20.46x] | 0.18x | 479.1 | 1296.7 | 2497.2 | 13.24x [1.56x-95.56x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **56.32x** [42.77x-74.15x] | 0.12x | 5341.9 | 3541.4 | 8651.2 | 52.62x [0.06x-2058.85x] |
| All 31 synthetic corpus families | 31 (293) | **50.93x** [40.84x-63.51x] | 0.11x | 5680.1 | 5549.6 | 12885.5 | 48.69x [0.06x-2058.85x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2. Synthetic MAD Group-Item Allocation Family (`G` Groups $\times$ `I` Items, $T = 6$ Periods)

The synthetic `MAD` group-item allocation family ([`BuildMADFromWeights`](bench/corpus.go)) assigns $I$ items across $G$ groups over $T = 6$ periods to minimize total L1 distance from each group's central level $m_g$ plus the roundness penalty $r_g = |m_g - 100 n_g|$ relative to an integer multiple $n_g \in \mathbb{Z}_{\ge 1}$. In the planted-gap generator, items in $S = \{0, \dots, I-G-1\}$ carry orthogonal curve signatures cancelled by the $G$ designated items $D = \{I-G, \dots, I-1\}$ with roundness-aligned period sums, yielding a zero root LP gap that root heuristics close at node 1; unstructured random-weight negative controls (`BuildMADRandom`: `t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`) are included directly in the corpus and detailed in [`docs/benchmarks.md`](docs/benchmarks.md).

<!-- BEGIN GENERATED:MAD_PIPELINE_TABLE -->
| Problem ID | $G \times I$ | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Trial 0 Obj | Parity |
| :--- | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **92.9x** [68.8x-106.9x] | 0.11 | 11.08 | 0.06 | 10.82 | 0.049 | 0.314 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **48.2x** [14.4x-95.6x] | 0.62 | 36.51 | 0.59 | 36.26 | 0.030 | 0.322 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **63.0x** [43.2x-94.8x] | 0.20 | 11.61 | 0.14 | 11.30 | 0.054 | 0.409 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **56.2x** [33.2x-72.4x] | 0.19 | 11.25 | 0.12 | 10.82 | 0.059 | 0.568 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **30.8x** [18.3x-41.9x] | 0.39 | 13.44 | 0.27 | 12.37 | 0.093 | 0.854 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **10.0x** [3.6x-38.2x] | 4.39 | 51.67 | 4.34 | 51.29 | 0.050 | 0.423 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **5.2x** [1.6x-23.7x] | 32.44 | 181.38 | 32.28 | 180.62 | 0.087 | 0.599 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **24.4x** [14.4x-33.4x] | 0.74 | 20.28 | 0.51 | 18.83 | 0.242 | 1.444 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **14.0x** [8.7x-20.6x] | 1.52 | 20.98 | 1.15 | 19.04 | 0.369 | 2.441 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **9.6x** [8.7x-11.6x] | 6.93 | 64.29 | 5.04 | 55.81 | 1.746 | 11.489 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **8.3x** [3.3x-21.6x] | 49.06 | 416.52 | 43.43 | 379.66 | 5.265 | 48.496 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

Cross-instance total latency dispersion across the randomized instances in each family (mean $\pm$ sample SD and two-sided 95% Student-$t$ interval after warmup):

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.12 \pm 0.02$ ($\pm 0.01$) | $11.08 \pm 0.46$ ($\pm 0.31$) | $0.15 \pm 0.03$ ($\pm 0.02$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.95 \pm 0.85$ ($\pm 0.57$) | $33.89 \pm 10.28$ ($\pm 6.90$) | $0.96 \pm 0.85$ ($\pm 0.57$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.19 \pm 0.05$ ($\pm 0.03$) | $11.67 \pm 0.45$ ($\pm 0.30$) | $0.20 \pm 0.06$ ($\pm 0.04$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.21 \pm 0.05$ ($\pm 0.03$) | $11.26 \pm 0.21$ ($\pm 0.14$) | $0.20 \pm 0.06$ ($\pm 0.04$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.45 \pm 0.15$ ($\pm 0.10$) | $13.28 \pm 0.58$ ($\pm 0.39$) | $0.40 \pm 0.17$ ($\pm 0.11$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $4.50 \pm 2.67$ ($\pm 1.79$) | $38.71 \pm 18.37$ ($\pm 12.34$) | $4.32 \pm 2.42$ ($\pm 1.63$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $38.10 \pm 31.61$ ($\pm 21.24$) | $154.41 \pm 69.88$ ($\pm 46.94$) | $37.75 \pm 31.06$ ($\pm 20.87$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.84 \pm 0.21$ ($\pm 0.26$) | $20.20 \pm 2.59$ ($\pm 3.21$) | $0.72 \pm 0.19$ ($\pm 0.23$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.65 \pm 0.65$ ($\pm 0.81$) | $21.94 \pm 1.86$ ($\pm 2.31$) | $1.50 \pm 0.84$ ($\pm 1.04$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $6.92 \pm 0.44$ ($\pm 0.55$) | $66.70 \pm 4.11$ ($\pm 5.10$) | $7.38 \pm 1.21$ ($\pm 1.51$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $56.11 \pm 15.45$ ($\pm 19.18$) | $633.98 \pm 652.45$ ($\pm 809.99$) | $53.84 \pm 15.57$ ($\pm 19.32$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### 3. Full 31-Family Synthetic Benchmark Corpus (`JSON -> Solution`)

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | **783.5x** [353.8x-1283.8x] | 0.01 | 7.12 | 0.00 | 7.06 | 0.006 | 0.077 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **957.4x** [681.8x-1310.4x] | 0.01 | 7.45 | 0.00 | 7.38 | 0.005 | 0.087 | `OK` |
| `t1_std_mip` | 3 | 3 | **683.5x** [543.3x-999.1x] | 0.01 | 8.74 | 0.01 | 8.67 | 0.006 | 0.097 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **647.3x** [358.2x-1015.5x] | 0.01 | 8.33 | 0.01 | 8.27 | 0.006 | 0.087 | `OK` |
| `t1_whiskas` | 6 | 5 | **503.0x** [362.8x-710.7x] | 0.02 | 7.20 | 0.00 | 7.11 | 0.010 | 0.113 | `OK` |
| `t1_furniture` | 2 | 3 | **305.2x** [100.7x-595.5x] | 0.03 | 8.79 | 0.02 | 8.72 | 0.005 | 0.079 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **143.4x** [60.5x-294.4x] | 0.08 | 10.67 | 0.06 | 10.56 | 0.013 | 0.134 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **1213.0x** [530.2x-2058.9x] | 0.01 | 7.55 | 0.00 | 7.50 | 0.005 | 0.082 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **92.9x** [68.8x-106.9x] | 0.11 | 11.08 | 0.06 | 10.82 | 0.049 | 0.314 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **48.2x** [14.4x-95.6x] | 0.62 | 36.51 | 0.59 | 36.26 | 0.030 | 0.322 | `OK` |
| `t2_mkp_30` | 30 | 6 | **8.0x** [4.3x-13.7x] | 2.94 | 30.21 | 2.91 | 30.02 | 0.031 | 0.246 | `OK` |
| `t2_mkp_50` | 50 | 8 | **5.7x** [2.5x-52.6x] | 4.92 | 19.81 | 4.88 | 19.52 | 0.048 | 0.372 | `OK` |
| `t2_cflp_36` | 36 | 11 | **36.1x** [5.3x-145.2x] | 0.25 | 11.64 | 0.21 | 11.45 | 0.040 | 0.260 | `OK` |
| `t2_cflp_49` | 49 | 13 | **33.7x** [15.3x-63.1x] | 0.38 | 12.81 | 0.35 | 12.59 | 0.029 | 0.302 | `OK` |
| `t2_gap_45` | 45 | 13 | **12.7x** [2.1x-36.8x] | 0.83 | 16.32 | 0.80 | 16.11 | 0.036 | 0.291 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **11.1x** [4.2x-23.3x] | 4.65 | 73.99 | 4.62 | 73.73 | 0.036 | 0.327 | `OK` |
| `t2_binpack_44` | 44 | 14 | **14.6x** [0.2x-66.1x] | 0.46 | 16.32 | 0.43 | 16.10 | 0.027 | 0.277 | `OK` |
| `t2_setcover_50` | 50 | 20 | **64.4x** [5.3x-149.8x] | 0.16 | 12.83 | 0.12 | 12.51 | 0.044 | 0.436 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **63.0x** [43.2x-94.8x] | 0.20 | 11.61 | 0.14 | 11.30 | 0.054 | 0.409 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **56.2x** [33.2x-72.4x] | 0.19 | 11.25 | 0.12 | 10.82 | 0.059 | 0.568 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **30.8x** [18.3x-41.9x] | 0.39 | 13.44 | 0.27 | 12.37 | 0.093 | 0.854 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **10.0x** [3.6x-38.2x] | 4.39 | 51.67 | 4.34 | 51.29 | 0.050 | 0.423 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **5.2x** [1.6x-23.7x] | 32.44 | 181.38 | 32.28 | 180.62 | 0.087 | 0.599 | `OK` |
| `t3_mkp_200` | 200 | 8 | **3.5x** [2.9x-3.9x] | 37.64 | 129.50 | 37.54 | 128.87 | 0.109 | 0.847 | `OK` |
| `t3_cflp_300` | 300 | 34 | **13.1x** [9.6x-19.7x] | 15.35 | 263.60 | 15.18 | 262.59 | 0.184 | 1.259 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **1.0x** [0.1x-2.4x] | 102.66 | 166.90 | 102.11 | 164.92 | 0.289 | 2.457 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **11.7x** [9.2x-16.3x] | 7.58 | 86.07 | 6.97 | 82.94 | 0.407 | 3.697 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **24.4x** [14.4x-33.4x] | 0.74 | 20.28 | 0.51 | 18.83 | 0.242 | 1.444 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **14.0x** [8.7x-20.6x] | 1.52 | 20.98 | 1.15 | 19.04 | 0.369 | 2.441 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **9.6x** [8.7x-11.6x] | 6.93 | 64.29 | 5.04 | 55.81 | 1.746 | 11.489 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **8.3x** [3.3x-21.6x] | 49.06 | 416.52 | 43.43 | 379.66 | 5.265 | 48.496 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

### 4. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus ([`bench/assign_corpus.go`](bench/assign_corpus.go), 200 randomized instances across square, rectangular wide/tall, degenerate integer-tie, Euclidean geometric, rank-1 outer-product, sparse `+Inf`, and maximization families) compares [`assign.LinearSumFlat`](assign/assign.go) against SciPy's C++ `rectangular_lsap` extension (`scipy.optimize.linear_sum_assignment`) in warm state on bit-identical `float64` cost matrices:

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Instances) | Paired `SciPy Solve / goblin` GeoMean [95% CI] | Paired `SciPy Total / goblin` GeoMean | `goblin` Solve Sum (ms) | SciPy Solve Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.65x** [1.51x-1.80x] | 4.67x | 0.217 | 0.332 | 1.72x [0.34x-2.88x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **3.04x** [2.66x-3.47x] | 2.99x | 10.588 | 35.227 | 2.70x [0.23x-26.41x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **3.00x** [2.64x-3.41x] | 3.93x | 285.197 | 1033.197 | 2.99x [1.47x-6.71x] |
| Square families ($m = n$) | 16 (146) | **2.73x** [2.45x-3.04x] | 3.85x | 291.115 | 1055.621 | 2.54x [0.23x-26.41x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.91x** [1.74x-2.09x] | 3.10x | 4.887 | 13.134 | 2.08x [0.78x-4.11x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.54x** [2.08x-3.11x] | 2.98x | 37.102 | 70.833 | 1.95x [0.97x-26.41x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.48x** [2.28x-2.70x] | 3.63x | 296.002 | 1068.755 | 2.28x [0.23x-26.41x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.78x** | 0.0004 | 0.0010 | 0.0005 | 0.0121 | $0.0008 \pm 0.0011$ | $0.0010 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **2.13x** | 0.0011 | 0.0026 | 0.0015 | 0.0109 | $0.0012 \pm 0.0003$ | $0.0025 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.62x** | 0.0037 | 0.0066 | 0.0055 | 0.0159 | $0.0040 \pm 0.0014$ | $0.0064 \pm 0.0018$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.40x** | 0.0029 | 0.0041 | 0.0044 | 0.0148 | $0.0031 \pm 0.0011$ | $0.0042 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.39x** | 0.0030 | 0.0045 | 0.0046 | 0.0160 | $0.0036 \pm 0.0016$ | $0.0047 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.68x** | 0.0071 | 0.0117 | 0.0122 | 0.0342 | $0.0070 \pm 0.0022$ | $0.0114 \pm 0.0020$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.75x** | 0.0279 | 0.0752 | 0.0416 | 0.102 | $0.0270 \pm 0.0061$ | $0.0732 \pm 0.0110$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.49x** | 0.0543 | 0.187 | 0.0722 | 0.238 | $0.0532 \pm 0.0056$ | $0.187 \pm 0.0275$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.16x** | 0.0470 | 0.100 | 0.0728 | 0.152 | $0.0478 \pm 0.0114$ | $0.102 \pm 0.0150$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.63x** | 0.150 | 0.233 | 0.159 | 0.285 | $0.155 \pm 0.0463$ | $0.249 \pm 0.0470$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **14.23x** | 0.0585 | 0.730 | 0.0803 | 0.807 | $0.0546 \pm 0.0086$ | $0.780 \pm 0.176$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **2.28x** | 0.0332 | 0.0734 | 0.0581 | 0.152 | $0.0329 \pm 0.0025$ | $0.0753 \pm 0.0083$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.08x** | 0.0395 | 0.0830 | 0.0665 | 0.162 | $0.0405 \pm 0.0030$ | $0.0844 \pm 0.0074$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **2.54x** | 0.0962 | 0.299 | 0.122 | 0.383 | $0.196 \pm 0.330$ | $0.309 \pm 0.0522$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.81x** | 0.339 | 1.350 | 0.447 | 1.814 | $0.355 \pm 0.0613$ | $1.343 \pm 0.127$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.06x** | 1.313 | 5.749 | 2.045 | 8.905 | $1.502 \pm 0.341$ | $5.995 \pm 0.517$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **2.23x** | 0.826 | 1.766 | 1.424 | 5.237 | $0.819 \pm 0.0499$ | $1.844 \pm 0.324$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.66x** | 5.362 | 9.002 | 5.971 | 10.041 | $6.010 \pm 2.240$ | $9.795 \pm 2.819$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **3.00x** | 0.331 | 0.890 | 0.732 | 3.826 | $0.326 \pm 0.0243$ | $0.999 \pm 0.264$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.64x** | 0.469 | 1.316 | 0.921 | 4.488 | $0.475 \pm 0.0401$ | $1.257 \pm 0.137$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.94x** | 6.715 | 32.955 | 8.661 | 46.971 | $6.682 \pm 0.716$ | $33.099 \pm 4.242$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **3.74x** | 40.362 | 157.877 | 49.244 | 207.146 | $41.226 \pm 4.701$ | $153.650 \pm 11.339$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

---

## Pure-Go vs. C++ CBC Performance Gaps and MAD Scaling Analysis

Comparing isolated unpresolved `Root LP` timings, paired trial ratios, and solver-core checkpoints (`CP5 +Presolve`, `CP6 +Cuts`, `CP7 Full`) between `BackendGo` and in-process `BackendCBC` (`c1c171ec`) highlights four structural trade-offs on large degenerate LPs and heavy-tail branch-and-cut models.

### 1. MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families (`N = G(I + T + 3)` variables, `M = G(2T + 3) + I` constraints) separate solver performance into three distinct scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Paired `Go / CBC` | Go Total Med (ms) | CBC Total Med (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.05x** | 0.11 | 2.66 | 0.55x | 0.06 (1 / 19) | 2.62 (0 / 0) | 0.070 (28) | 0.127 (19) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.07x** | 0.62 | 12.32 | 0.45x | 0.59 (5 / 124) | 12.28 (6 / 83) | 0.071 (30) | 0.158 (18) |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.08x** | 0.20 | 2.20 | 0.64x | 0.14 (1 / 26) | 2.16 (0 / 0) | 0.096 (33) | 0.151 (21) |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.07x** | 0.19 | 2.81 | 0.85x | 0.12 (1 / 39) | 2.74 (0 / 0) | 0.226 (59) | 0.266 (40) |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.11x** | 0.39 | 3.61 | 0.73x | 0.27 (1 / 51) | 3.52 (0 / 0) | 0.270 (68) | 0.369 (63) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.25x** | 4.39 | 17.57 | 0.88x | 4.34 (43 / 580) | 17.51 (12 / 216) | 0.117 (38) | 0.134 (24) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.36x** | 32.44 | 93.73 | 0.91x | 32.28 (137 / 1910) | 93.66 (198 / 1300) | 0.242 (66) | 0.265 (54) |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.10x** | 0.74 | 8.74 | 1.54x | 0.51 (1 / 75) | 8.57 (0 / 0) | 0.911 (138) | 0.591 (94) |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.20x** | 1.52 | 7.87 | 1.55x | 1.15 (1 / 105) | 7.61 (0 / 0) | 1.478 (246) | 0.957 (149) |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **0.18x** | 6.93 | 39.07 | 4.86x | 5.04 (1 / 255) | 37.88 (0 / 0) | 15.495 (962) | 3.187 (319) |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **0.21x** | 49.06 | 303.86 | 3.38x | 43.43 (1 / 2439) | 299.08 (0 / 0) | 117.723 (2314) | 34.820 (3128) |
<!-- END GENERATED:MAD_CBC_GAP_TABLE -->

1. **Small-to-Medium Planted-Gap MAD (`t1_mad_g2_i4` through `t3_mad_g5_i20`, $26 \le N \le 145$)**: Every structured `t*_mad_g*` instance has a zero root LP gap by design (`RootLPBound == OptimalObj`). Pure-Go closes those models at node 1 through presolve orbit fixing and root heuristics (`tryPropagatedRounding`, `tryRootSubMIP`, and `tryFractionalDiving` in [`heuristics.go`](heuristics.go)) without C++ `CbcModel` and `CglPreProcess` setup costs.
2. **Large Planted-Gap MAD (`t3_mad_g6_i30`, `t3_mad_g10_i100`, `t3_mad_g4_i1000`)**: Pure-Go `Go Solve` finishes faster than its own unpresolved `Go Root LP` because [`Presolve`](presolve.go) detects and verifies the $S_G$ group-permutation automorphism (`applyOrbitSymmetryBreaking`), fixes the $G \times G$ designated-item bijection to $I_G$, and eliminates $G^2$ binary variables and $2G$ assignment equations before the root LP runs, while [`FTRANSparseHint`](lu.go) and [`BTRANSparseHint`](lu.go) restrict basis triangular sweeps to active index windows. On the unpresolved `Root LP` column, however, `COIN-OR CLP` remains faster for two reasons:
   - **Degenerate Simplex Iteration Growth (`t3_mad_g10_i100`)**: Partitioning 100 items across $G = 10$ identical groups creates severe primal and dual degeneracy. `COIN-OR CLP` mitigates that degeneracy via LP presolve (`ClpPresolve`), geometric-mean matrix scaling, anti-degeneracy cost shifts, and crash basis assembly (`Idiot` / triangular crash). [`LPSolver`](simplex.go) starts from an all-slack basis without LP presolve or cost shifts, and its Phase II primal fallback uses full Dantzig pricing.
   - **Per-Pivot PFI Eta Update Cost (`t3_mad_g4_i1000`)**: On 1,000-row bases, pure-Go costs more per pivot on the unpresolved Root LP because the basis factorizer ([`lu.go`](lu.go)) appends a Product-Form of the Inverse (PFI) eta file (`PushEtaSparse`) up to `maxEtaUpdates = 56` before rebuilding factors from scratch, whereas `CLP` updates the $U$ factor in place via Forrest-Tomlin updates.
3. **Symmetric Random-Weight MAD (`t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`)**: Assigning dense random weights (`BuildMADRandom`) removes the zero-gap structure and forces Branch-and-Cut to prove optimality:
   - [`Presolve`](presolve.go) verifies exact $S_G$ group symmetry and fixes the $G \times G$ designated-item bijection to $I_G$, [`bab.go`](bab.go) rolls back trailing cut rounds that fail to improve the root bound and purges slack root cuts via [`PurgeSlackCuts`](simplex.go) before branching, and post-cut root heuristics are skipped when cuts add zero bound gain.
   - Even so, [`docs/benchmarks.md`](docs/benchmarks.md#mad-solver-core-checkpoint-progression) shows that `CP5 +Presolve` (`Presolve=on, Cuts=off, Heuristics=off`) finishes faster than `CP6 +Cuts` and `CP7 Full` on `t2_mad_rand_g3_i9` because trial root cut separation (`CP6`) and pre-cut root diving and sub-MIP LPs (`CP7`) pay root LP work without closing the random-weight integrality gap.

### 2. Non-MAD Corpus Comparison, Heavy-Tail Dispersion, and Checkpoint Trade-Offs

Across the non-MAD families, multi-item lot sizing (`t3_lotsizing_504`) and outlier seeds of `t2_binpack_44` exhibit heavy-tail tree variance where single-row root cuts leave a larger search tree than CBC's multi-row cut aggregation, while on `t3_cflp_300` and `t2_mkp_50` `CP5 +Presolve` finishes faster than `CP7 Full`:

<!-- BEGIN GENERATED:NON_MAD_CBC_GAP_TABLE -->
| Problem ID | $N \times M$ | Paired `Go / CBC` | Go Total Mean $\pm$ SD (ms) | CBC Total Mean $\pm$ SD (ms) | Go CP7 Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go CP6 +Cuts (ms / nodes / cuts) | Go CP5 +Presolve (ms / nodes) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t3_mkp_200` | $200 \times 8$ | **0.67x** | $32.63 \pm 16.80$ | $46.19 \pm 21.37$ | 37.54 (594 / 5502) | 56.94 (154 / 1145) | 45.49 (701 / 14) | 101.78 (5000, `NodeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **0.41x** | $8.22 \pm 10.18$ | $13.29 \pm 9.76$ | 4.88 (1 / 24) | 9.13 (0 / 8) | 4.79 (2 / 4) | 4.31 (132) |
| `t2_binpack_44` | $44 \times 14$ | **0.16x** | $13.31 \pm 38.53$ | $7.31 \pm 2.35$ | 0.43 (1 / 45) | 6.59 (0 / 8) | 0.99 (13 / 5) | 14.99 (1838) |
| `t3_cflp_300` | $300 \times 34$ | **0.31x** | $19.53 \pm 8.32$ | $59.96 \pm 12.77$ | 15.18 (48 / 2131) | 57.78 (18 / 483) | 24.38 (63 / 31) | 6.59 (134) |
| `t3_lotsizing_504` | $504 \times 336$ | **1.76x** | $830.64 \pm 1683.70$ | $98.64 \pm 60.95$ | 102.11 (27 / 1284) | 75.25 (10 / 377) | 126.52 (416 / 203) | 246.37 (5000, `NodeLimit`) |
| `t3_sched_1000` | $1000 \times 100$ | **0.12x** | $7.66 \pm 1.51$ | $65.29 \pm 5.24$ | 6.97 (35 / 431) | 64.25 (0 / 76) | 12.73 (96 / 47) | 250.02 (4997, `TimeLimit`) |
<!-- END GENERATED:NON_MAD_CBC_GAP_TABLE -->

### 3. Summary of Architectural Bottlenecks

1. **PFI Eta Updates and LP Cold Starts ([`lu.go`](lu.go), [`simplex.go`](simplex.go))**: Scanning up to `maxEtaUpdates = 56` PFI eta vectors between full Markowitz factor rebuilds increases per-pivot cost on large unpresolved bases (`t3_mad_g4_i1000`), while omitting LP presolve and anti-degeneracy cost shifts inflates pivot counts on degenerate unpresolved assignment LPs (`t3_mad_g10_i100`).
2. **Root-Only Cut Separation Without Multi-Row Aggregation ([`cuts.go`](cuts.go))**: Although [`bab.go`](bab.go) rolls back zero-gain cut tails and [`PurgeSlackCuts`](simplex.go) removes slack root cuts before branching, cutting planes are separated only at the root node from single rows using global bounds. Multi-item lot-sizing (`t3_lotsizing_504`, `coinor_capplan1`) and dense multi-constraint knapsacks (`t3_mkp_200`) require multi-row aggregation and in-tree cuts to match CBC's bound tightening across all seeds.
3. **Static Presolve Symmetry Breaking Only ([`presolve.go`](presolve.go), [`bab.go`](bab.go))**: [`Presolve`](presolve.go) detects and verifies static $S_G$ group automorphisms (fixing designated bijections, upper-triangular partitioning/packing orbitopes, and monotone-cost block-opening indicators), whereas [`bab.go`](bab.go) does not perform dynamic orbital branching for symmetries that emerge only after branching decisions on identical-weight items.
4. **Trial Root Cut and Pre-Cut Heuristic Overhead ([`heuristics.go`](heuristics.go), [`bab.go`](bab.go))**: Post-cut heuristics are skipped when root cuts do not improve the LP bound, yet trial root cut passes (`CP6`) and pre-cut diving and sub-MIP solves (`CP7`) still add root LP overhead relative to `CP5 +Presolve` on models whose integrality gap must be closed by branching (`t2_mad_rand_g3_i9`, `t3_cflp_300`).

---

## Verification, Benchmarking, and Documentation Sync Commands

Run all commands from the `goblin/` directory:

```bash
# 0. Build the vendored COIN-OR CBC/CLP C++ stack with -DNDEBUG into third_party/Cbc/dist (once per checkout)
./scripts/build_cbc.sh

# 1. Unit, regression, 293-trial CBC parity, 17-instance public suite, and documentation gates
go test ./...
GOEXPERIMENT=simd go test ./...
go test -tags=cbc ./...

# 2. Regenerate the deduplicated 31-family PuLP JSON and multi-trial JSONL corpus in testdata/corpus/
go run ./cmd/goblin-bench -generate-corpus

# 3. Commit code changes first so every telemetry file records clean git provenance.
git status --short   # must be empty for git_dirty=false provenance

# 4. Warm-state Python PuLP + COIN-OR CBC (c1c171ec) baseline, pinned to one performance core
CBC_BIN=./third_party/Cbc/dist/bin/cbc taskset -c 2 python3 bench/python_baseline.py --corpus-dir testdata/corpus --out-json bench/results/python_baseline.json --workers 1

# 5. Warm-state goblin benchmarks across Pure-Go (scalar and SIMD) and C++ CBC backends, pinned to CPU 2
go build -o /tmp/goblin-bench-scalar ./cmd/goblin-bench && taskset -c 2 /tmp/goblin-bench-scalar -backend go -no-simd -workers 1 -out bench/results/goblin_scalar.json
GOEXPERIMENT=simd go build -o /tmp/goblin-bench-simd ./cmd/goblin-bench && taskset -c 2 /tmp/goblin-bench-simd -backend go -workers 1 -out bench/results/goblin_simd.json
go build -tags=cbc -o /tmp/goblin-bench-cbc ./cmd/goblin-bench && taskset -c 2 /tmp/goblin-bench-cbc -backend cbc -workers 1 -out bench/results/goblin_cbc.json

# 6. Warm-state SciPy and goblin/assign LSAP benchmarks, pinned to CPU 2
taskset -c 2 python3 bench/scipy_assign_baseline.py --corpus-dir testdata/assign_corpus --out-json bench/results/scipy_assign_baseline.json
taskset -c 2 /tmp/goblin-bench-scalar -assign -no-simd -out bench/results/goblin_assign_scalar.json
taskset -c 2 /tmp/goblin-bench-simd -assign -out bench/results/goblin_assign_simd.json

# 7. Render README.md, docs/benchmarks.md, and bench/results/final_report.md from the telemetry
go run ./cmd/goblin-bench -sync-docs

# 8. Verify documentation, parity, symbol identifiers, and provenance (exits non-zero on any failure)
go run ./cmd/goblin-bench -check-docs

# 9. Differential tests against CBC and SciPy on freshly generated random instances
python3 bench/difftest_cbc.py --instances 400 --seed 1
python3 bench/difftest_assign.py --instances 500 --seed 1

# 10. Hot-path compiler, disassembly, and Amdahl's Law profile audit (scalar and SIMD)
go run ./cmd/goblin-optcheck -benchtime=200ms
go run ./cmd/goblin-optcheck -simd -benchtime=200ms
```

---

## License

The `goblin` module and its pure-Go default build (`BackendGo`, `goblin/assign`, `goblin/format`) are distributed under the [MIT License](LICENSE).

Optional vendored COIN-OR sources under `third_party/` (`Cbc`, `Cgl`, `Clp`, `CoinUtils`, and `Osi`, pinned in [`third_party/VERSIONS`](third_party/VERSIONS) and excluded from standard `go get` module downloads via `third_party/go.mod`) and adapted COIN-OR test fixtures in [`cbc_ported_test.go`](cbc_ported_test.go) are governed by the Eclipse Public License 2.0 (`EPL-2.0`; license texts in `third_party/*/LICENSE`). When distributing binaries compiled with `-tags cbc` that link those COIN-OR libraries, recipients may obtain the corresponding COIN-OR source code from `third_party/` in the upstream repository at `https://github.com/paulourio/goblin` (or from `https://github.com/coin-or`) under the terms of `EPL-2.0` Section 3.
