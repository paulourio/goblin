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
- **Unified telemetry source**: fingerprint `f2b8d69d4aef8b31` (working tree on top of commit `759089c34649faad2e8fade211b398f598437c4b`), `go1.27.3-X:nodwarf5`, Linux kernel `6.18.63-gentoo-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 7.33 ms median on a 1-variable MILP and 6.88 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

Read the synthetic corpus ratios with three methodological notes. First, Tier 1 micro-models (`N <= 26`) are dominated by Python's ~1.3 to 2.0 ms CBC subprocess spawn cost. Second, the planted-gap MAD family has a zero root LP gap by construction and closes at node 1. Consequently, the bold row (`Tier 2+3 non-planted MILPs`) isolates solver-dominated MILPs across paired randomized instances, alongside total subset wall times and full per-instance `[Min-Max]` ranges.

### 1. Aggregate Synthetic Workload Summary (Paired Per-Instance Geometric Mean and Wall-Clock Sums)

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Instances) | Paired `Py / Go` GeoMean [95% CI] | Paired `Go / CBC` | `Go` Sum (ms) | `CBC` Sum (ms) | `Py` Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **336.05x** [273.46x-412.96x] | 0.05x | 14.2 | 206.7 | 1208.1 | 524.55x [13.11x-1540.99x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **18.94x** [15.55x-23.06x] | 0.16x | 858.6 | 2121.3 | 4706.1 | 24.22x [0.19x-165.84x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **8.02x** [5.63x-11.44x] | 0.26x | 4749.4 | 3119.2 | 6677.5 | 9.68x [0.06x-32.40x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **11.73x** [9.44x-14.58x] | 0.23x | 5279.6 | 3366.6 | 7344.5 | 12.10x [0.06x-165.84x] |
| Tier 3 non-MAD MILPs | 4 (20) | **4.66x** [2.54x-8.55x] | 0.47x | 4429.1 | 1340.3 | 3028.6 | 6.64x [0.06x-19.43x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **37.63x** [30.75x-46.05x] | 0.09x | 329.8 | 1900.1 | 4160.4 | 44.11x [3.41x-99.95x] |
| Random-weight MAD (negative controls) | 3 (33) | **13.70x** [9.09x-20.65x] | 0.18x | 467.3 | 1317.4 | 2425.4 | 12.62x [1.62x-89.94x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **53.56x** [40.79x-70.32x] | 0.12x | 5292.4 | 3547.0 | 8431.4 | 45.06x [0.06x-1540.99x] |
| All 31 synthetic corpus families | 31 (293) | **49.59x** [39.88x-61.65x] | 0.11x | 5622.2 | 5447.1 | 12591.7 | 44.77x [0.06x-1540.99x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2. Synthetic MAD Group-Item Allocation Family (`G` Groups $\times$ `I` Items, $T = 6$ Periods)

The synthetic `MAD` group-item allocation family ([`BuildMADFromWeights`](bench/corpus.go)) assigns $I$ items across $G$ groups over $T = 6$ periods to minimize total L1 distance from each group's central level $m_g$ plus the roundness penalty $r_g = |m_g - 100 n_g|$ relative to an integer multiple $n_g \in \mathbb{Z}_{\ge 1}$. In the planted-gap generator, items in $S = \{0, \dots, I-G-1\}$ carry orthogonal curve signatures cancelled by the $G$ designated items $D = \{I-G, \dots, I-1\}$ with roundness-aligned period sums, yielding a zero root LP gap that root heuristics close at node 1; unstructured random-weight negative controls (`BuildMADRandom`: `t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`) are included directly in the corpus and detailed in [`docs/benchmarks.md`](docs/benchmarks.md).

<!-- BEGIN GENERATED:MAD_PIPELINE_TABLE -->
| Problem ID | $G \times I$ | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Trial 0 Obj | Parity |
| :--- | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **81.3x** [67.1x-100.0x] | 0.13 | 11.02 | 0.08 | 10.79 | 0.049 | 0.313 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **46.8x** [13.1x-89.9x] | 0.61 | 36.65 | 0.59 | 36.40 | 0.030 | 0.322 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **67.6x** [37.3x-85.7x] | 0.14 | 10.66 | 0.10 | 10.34 | 0.037 | 0.392 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **66.4x** [62.8x-74.8x] | 0.18 | 11.51 | 0.11 | 11.08 | 0.057 | 0.574 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **34.0x** [24.2x-44.4x] | 0.36 | 12.96 | 0.26 | 12.33 | 0.090 | 0.849 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **10.8x** [3.2x-39.8x] | 4.47 | 51.51 | 4.42 | 51.18 | 0.051 | 0.421 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **5.1x** [1.6x-24.3x] | 32.00 | 179.21 | 31.93 | 178.73 | 0.074 | 0.596 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **29.1x** [26.3x-32.4x] | 0.68 | 19.95 | 0.48 | 18.76 | 0.185 | 1.420 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **14.9x** [6.5x-20.2x] | 1.16 | 20.61 | 0.88 | 18.65 | 0.296 | 2.439 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **10.2x** [9.4x-11.5x] | 6.77 | 65.16 | 4.95 | 56.75 | 1.419 | 11.509 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **8.2x** [3.4x-21.1x] | 49.00 | 393.64 | 42.51 | 357.50 | 6.324 | 47.502 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

Cross-instance total latency dispersion across the randomized instances in each family (mean $\pm$ sample SD and two-sided 95% Student-$t$ interval after warmup):

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.14 \pm 0.02$ ($\pm 0.01$) | $11.02 \pm 0.24$ ($\pm 0.16$) | $0.13 \pm 0.03$ ($\pm 0.02$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.96 \pm 0.87$ ($\pm 0.58$) | $33.54 \pm 10.29$ ($\pm 6.91$) | $0.98 \pm 0.89$ ($\pm 0.59$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.17 \pm 0.06$ ($\pm 0.04$) | $10.72 \pm 0.68$ ($\pm 0.46$) | $0.18 \pm 0.06$ ($\pm 0.04$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.17 \pm 0.01$ ($\pm 0.01$) | $11.55 \pm 0.36$ ($\pm 0.24$) | $0.23 \pm 0.06$ ($\pm 0.04$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.40 \pm 0.08$ ($\pm 0.06$) | $13.20 \pm 0.58$ ($\pm 0.39$) | $0.42 \pm 0.10$ ($\pm 0.07$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $4.21 \pm 2.27$ ($\pm 1.53$) | $38.50 \pm 18.27$ ($\pm 12.27$) | $4.16 \pm 2.32$ ($\pm 1.56$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $37.31 \pm 30.76$ ($\pm 20.66$) | $148.44 \pm 67.74$ ($\pm 45.50$) | $38.08 \pm 32.59$ ($\pm 21.89$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.67 \pm 0.04$ ($\pm 0.04$) | $19.66 \pm 2.41$ ($\pm 2.99$) | $0.77 \pm 0.22$ ($\pm 0.27$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.52 \pm 0.91$ ($\pm 1.12$) | $20.50 \pm 0.22$ ($\pm 0.27$) | $1.49 \pm 0.77$ ($\pm 0.96$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $6.65 \pm 0.63$ ($\pm 0.79$) | $67.61 \pm 4.02$ ($\pm 4.98$) | $6.72 \pm 0.75$ ($\pm 0.93$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $55.21 \pm 16.74$ ($\pm 20.78$) | $622.01 \pm 660.27$ ($\pm 819.70$) | $53.83 \pm 16.28$ ($\pm 20.21$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### 3. Full 31-Family Synthetic Benchmark Corpus (`JSON -> Solution`)

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | **789.8x** [480.0x-1338.1x] | 0.01 | 7.19 | 0.00 | 7.13 | 0.006 | 0.084 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **855.0x** [376.6x-1171.9x] | 0.01 | 7.26 | 0.00 | 7.20 | 0.005 | 0.086 | `OK` |
| `t1_std_mip` | 3 | 3 | **660.9x** [483.4x-999.8x] | 0.01 | 8.64 | 0.01 | 8.57 | 0.006 | 0.092 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **660.5x** [523.4x-806.2x] | 0.01 | 8.33 | 0.01 | 8.27 | 0.006 | 0.089 | `OK` |
| `t1_whiskas` | 6 | 5 | **457.7x** [288.8x-608.2x] | 0.02 | 7.49 | 0.00 | 7.40 | 0.010 | 0.124 | `OK` |
| `t1_furniture` | 2 | 3 | **295.1x** [108.5x-564.0x] | 0.03 | 9.17 | 0.02 | 9.10 | 0.005 | 0.090 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **141.9x** [66.2x-293.2x] | 0.08 | 10.34 | 0.06 | 10.24 | 0.012 | 0.133 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **854.7x** [74.3x-1541.0x] | 0.01 | 7.01 | 0.00 | 6.96 | 0.005 | 0.076 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **81.3x** [67.1x-100.0x] | 0.13 | 11.02 | 0.08 | 10.79 | 0.049 | 0.313 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **46.8x** [13.1x-89.9x] | 0.61 | 36.65 | 0.59 | 36.40 | 0.030 | 0.322 | `OK` |
| `t2_mkp_30` | 30 | 6 | **8.3x** [4.3x-15.8x] | 2.97 | 30.43 | 2.90 | 30.24 | 0.031 | 0.245 | `OK` |
| `t2_mkp_50` | 50 | 8 | **5.4x** [2.5x-52.4x] | 5.03 | 18.86 | 4.98 | 18.59 | 0.050 | 0.363 | `OK` |
| `t2_cflp_36` | 36 | 11 | **31.6x** [4.4x-109.6x] | 0.23 | 10.13 | 0.19 | 9.96 | 0.037 | 0.229 | `OK` |
| `t2_cflp_49` | 49 | 13 | **27.5x** [10.9x-47.6x] | 0.38 | 11.22 | 0.35 | 11.01 | 0.029 | 0.287 | `OK` |
| `t2_gap_45` | 45 | 13 | **11.8x** [2.4x-30.8x] | 1.05 | 16.04 | 1.00 | 15.83 | 0.036 | 0.280 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **10.9x** [4.0x-29.8x] | 4.82 | 74.34 | 4.74 | 74.07 | 0.038 | 0.330 | `OK` |
| `t2_binpack_44` | 44 | 14 | **15.7x** [0.2x-93.3x] | 0.46 | 15.37 | 0.43 | 15.15 | 0.028 | 0.283 | `OK` |
| `t2_setcover_50` | 50 | 20 | **61.9x** [4.8x-165.8x] | 0.17 | 12.56 | 0.12 | 12.25 | 0.053 | 0.440 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **67.6x** [37.3x-85.7x] | 0.14 | 10.66 | 0.10 | 10.34 | 0.037 | 0.392 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **66.4x** [62.8x-74.8x] | 0.18 | 11.51 | 0.11 | 11.08 | 0.057 | 0.574 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **34.0x** [24.2x-44.4x] | 0.36 | 12.96 | 0.26 | 12.33 | 0.090 | 0.849 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **10.8x** [3.2x-39.8x] | 4.47 | 51.51 | 4.42 | 51.18 | 0.051 | 0.421 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **5.1x** [1.6x-24.3x] | 32.00 | 179.21 | 31.93 | 178.73 | 0.074 | 0.596 | `OK` |
| `t3_mkp_200` | 200 | 8 | **3.3x** [2.8x-3.8x] | 38.15 | 124.27 | 38.06 | 123.62 | 0.130 | 0.854 | `OK` |
| `t3_cflp_300` | 300 | 34 | **13.0x** [9.6x-19.4x] | 15.52 | 266.84 | 15.33 | 265.89 | 0.189 | 1.250 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **1.0x** [0.1x-2.4x] | 102.07 | 169.55 | 101.56 | 167.58 | 0.314 | 2.469 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **11.4x** [9.5x-13.8x] | 8.15 | 86.71 | 7.72 | 83.57 | 0.478 | 3.836 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **29.1x** [26.3x-32.4x] | 0.68 | 19.95 | 0.48 | 18.76 | 0.185 | 1.420 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **14.9x** [6.5x-20.2x] | 1.16 | 20.61 | 0.88 | 18.65 | 0.296 | 2.439 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **10.2x** [9.4x-11.5x] | 6.77 | 65.16 | 4.95 | 56.75 | 1.419 | 11.509 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **8.2x** [3.4x-21.1x] | 49.00 | 393.64 | 42.51 | 357.50 | 6.324 | 47.502 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

### 4. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus ([`bench/assign_corpus.go`](bench/assign_corpus.go), 200 randomized instances across square, rectangular wide/tall, degenerate integer-tie, Euclidean geometric, rank-1 outer-product, sparse `+Inf`, and maximization families) compares [`assign.LinearSumFlat`](assign/assign.go) against SciPy's C++ `rectangular_lsap` extension (`scipy.optimize.linear_sum_assignment`) in warm state on bit-identical `float64` cost matrices:

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Instances) | Paired `SciPy Solve / goblin` GeoMean [95% CI] | Paired `SciPy Total / goblin` GeoMean | `goblin` Solve Sum (ms) | SciPy Solve Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.58x** [1.43x-1.74x] | 4.65x | 0.247 | 0.335 | 1.67x [0.31x-2.88x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **3.12x** [2.76x-3.52x] | 2.97x | 9.564 | 35.559 | 2.76x [1.03x-21.16x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.81x** [2.43x-3.24x] | 3.59x | 257.406 | 1008.693 | 2.70x [0.87x-5.81x] |
| Square families ($m = n$) | 16 (146) | **2.69x** [2.42x-3.00x] | 3.78x | 262.425 | 1033.008 | 2.54x [0.31x-21.16x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.88x** [1.74x-2.04x] | 3.02x | 4.792 | 11.580 | 2.09x [0.84x-2.81x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.41x** [1.96x-2.97x] | 2.91x | 38.682 | 70.303 | 1.93x [0.31x-21.16x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.45x** [2.25x-2.66x] | 3.56x | 267.217 | 1044.588 | 2.25x [0.31x-21.16x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.47x** | 0.0005 | 0.0009 | 0.0006 | 0.0113 | $0.0009 \pm 0.0011$ | $0.0009 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **2.17x** | 0.0012 | 0.0025 | 0.0015 | 0.0106 | $0.0011 \pm 0.0003$ | $0.0025 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.68x** | 0.0036 | 0.0066 | 0.0055 | 0.0159 | $0.0039 \pm 0.0013$ | $0.0064 \pm 0.0018$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.43x** | 0.0029 | 0.0042 | 0.0045 | 0.0157 | $0.0030 \pm 0.0011$ | $0.0041 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.38x** | 0.0031 | 0.0044 | 0.0048 | 0.0162 | $0.0035 \pm 0.0013$ | $0.0046 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.47x** | 0.0070 | 0.0116 | 0.0123 | 0.0338 | $0.0100 \pm 0.0102$ | $0.0119 \pm 0.0033$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.74x** | 0.0286 | 0.0759 | 0.0410 | 0.103 | $0.0284 \pm 0.0083$ | $0.0755 \pm 0.0106$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.47x** | 0.0552 | 0.191 | 0.0729 | 0.247 | $0.0539 \pm 0.0059$ | $0.188 \pm 0.0288$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.06x** | 0.0467 | 0.0996 | 0.0734 | 0.150 | $0.0513 \pm 0.0170$ | $0.102 \pm 0.0145$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.69x** | 0.145 | 0.234 | 0.154 | 0.282 | $0.154 \pm 0.0464$ | $0.256 \pm 0.0529$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **13.40x** | 0.0587 | 0.726 | 0.0810 | 0.805 | $0.0585 \pm 0.0128$ | $0.781 \pm 0.173$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **2.36x** | 0.0324 | 0.0740 | 0.0583 | 0.151 | $0.0320 \pm 0.0028$ | $0.0756 \pm 0.0085$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.08x** | 0.0412 | 0.0853 | 0.0705 | 0.163 | $0.0411 \pm 0.0032$ | $0.0855 \pm 0.0076$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.31x** | 0.0904 | 0.303 | 0.118 | 0.387 | $0.0929 \pm 0.0105$ | $0.307 \pm 0.0331$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.84x** | 0.353 | 1.391 | 0.463 | 1.876 | $0.357 \pm 0.0638$ | $1.361 \pm 0.139$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.07x** | 1.358 | 5.571 | 1.781 | 8.399 | $1.398 \pm 0.145$ | $5.685 \pm 0.431$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.67x** | 0.825 | 1.591 | 1.432 | 4.250 | $1.009 \pm 0.438$ | $1.590 \pm 0.0567$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.64x** | 5.560 | 9.139 | 5.802 | 10.065 | $6.115 \pm 2.107$ | $9.924 \pm 2.923$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.68x** | 0.334 | 0.873 | 0.753 | 3.848 | $0.333 \pm 0.0236$ | $0.893 \pm 0.0605$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.33x** | 0.445 | 1.052 | 0.859 | 3.854 | $0.451 \pm 0.0169$ | $1.050 \pm 0.0434$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.58x** | 6.983 | 30.078 | 9.451 | 41.198 | $6.696 \pm 0.526$ | $30.664 \pm 2.457$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.28x** | 35.137 | 150.915 | 42.497 | 208.605 | $35.479 \pm 2.662$ | $151.933 \pm 9.947$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

---

## Pure-Go vs. C++ CBC Performance Gaps and MAD Scaling Analysis

Comparing isolated unpresolved `Root LP` timings, paired trial ratios, and solver-core checkpoints (`CP5 +Presolve`, `CP6 +Cuts`, `CP7 Full`) between `BackendGo` and in-process `BackendCBC` (`c1c171ec`) highlights four structural trade-offs on large degenerate LPs and heavy-tail branch-and-cut models.

### 1. MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families (`N = G(I + T + 3)` variables, `M = G(2T + 3) + I` constraints) separate solver performance into three distinct scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Paired `Go / CBC` | Go Total Med (ms) | CBC Total Med (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.06x** | 0.13 | 2.60 | 0.62x | 0.08 (1 / 19) | 2.55 (0 / 0) | 0.072 (28) | 0.117 (19) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.07x** | 0.61 | 11.94 | 0.52x | 0.59 (5 / 124) | 11.90 (6 / 83) | 0.070 (30) | 0.134 (18) |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.07x** | 0.14 | 1.92 | 0.74x | 0.10 (1 / 26) | 1.86 (0 / 0) | 0.095 (33) | 0.128 (21) |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.07x** | 0.18 | 2.67 | 0.93x | 0.11 (1 / 39) | 2.58 (0 / 0) | 0.209 (59) | 0.226 (40) |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.10x** | 0.36 | 3.54 | 0.74x | 0.26 (1 / 51) | 3.42 (0 / 0) | 0.278 (68) | 0.378 (63) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.23x** | 4.47 | 17.76 | 0.85x | 4.42 (43 / 580) | 17.71 (12 / 216) | 0.118 (38) | 0.138 (24) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.35x** | 32.00 | 98.57 | 0.93x | 31.93 (137 / 1910) | 98.50 (198 / 1300) | 0.246 (66) | 0.263 (54) |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.08x** | 0.68 | 8.62 | 1.68x | 0.48 (1 / 75) | 8.46 (0 / 0) | 0.989 (138) | 0.587 (94) |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.17x** | 1.16 | 7.98 | 1.47x | 0.88 (1 / 105) | 7.71 (0 / 0) | 1.431 (246) | 0.976 (149) |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **0.17x** | 6.77 | 39.52 | 4.78x | 4.95 (1 / 255) | 37.93 (0 / 0) | 15.575 (962) | 3.258 (319) |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **0.21x** | 49.00 | 298.36 | 3.32x | 42.51 (1 / 2439) | 292.67 (0 / 0) | 116.443 (2314) | 35.023 (3128) |
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
| `t3_mkp_200` | $200 \times 8$ | **0.72x** | $32.96 \pm 17.03$ | $43.23 \pm 20.42$ | 38.06 (594 / 5502) | 52.92 (154 / 1145) | 45.45 (701 / 14) | 97.78 (5000, `NodeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **0.40x** | $8.08 \pm 9.64$ | $13.45 \pm 9.65$ | 4.98 (1 / 24) | 9.74 (0 / 8) | 4.45 (2 / 4) | 4.39 (132) |
| `t2_binpack_44` | $44 \times 14$ | **0.15x** | $12.18 \pm 34.83$ | $7.21 \pm 2.40$ | 0.43 (1 / 45) | 6.45 (0 / 8) | 0.96 (13 / 5) | 14.47 (1838) |
| `t3_cflp_300` | $300 \times 34$ | **0.32x** | $19.79 \pm 8.39$ | $59.35 \pm 12.64$ | 15.33 (48 / 2131) | 53.86 (18 / 483) | 23.10 (63 / 31) | 6.71 (134) |
| `t3_lotsizing_504` | $504 \times 336$ | **1.74x** | $825.26 \pm 1672.84$ | $99.76 \pm 61.70$ | 101.56 (27 / 1284) | 76.43 (10 / 377) | 124.05 (416 / 203) | 250.03 (4828, `TimeLimit`) |
| `t3_sched_1000` | $1000 \times 100$ | **0.12x** | $7.81 \pm 1.18$ | $65.71 \pm 5.16$ | 7.72 (35 / 431) | 63.95 (0 / 76) | 11.98 (96 / 47) | 244.17 (5000, `NodeLimit`) |
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
