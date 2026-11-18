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
- **Unified telemetry source**: fingerprint `a33eec8017a28703` (working tree on top of commit `d73197b773a8357bb4b58c816d30b2788628f346`), `go1.27.3-X:nodwarf5`, Linux kernel `6.18.63-gentoo-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 7.37 ms median on a 1-variable MILP and 7.08 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

Read the synthetic corpus ratios with three methodological notes. First, Tier 1 micro-models (`N <= 26`) are dominated by Python's ~1.3 to 2.0 ms CBC subprocess spawn cost. Second, the planted-gap MAD family has a zero root LP gap by construction and closes at node 1. Consequently, the bold row (`Tier 2+3 non-planted MILPs`) isolates solver-dominated MILPs across paired randomized instances, alongside total subset wall times and full per-instance `[Min-Max]` ranges.

### 1. Aggregate Synthetic Workload Summary (Paired Per-Instance Geometric Mean and Wall-Clock Sums)

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Instances) | Paired `Py / Go` GeoMean [95% CI] | Paired `Go / CBC` | `Go` Sum (ms) | `CBC` Sum (ms) | `Py` Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **348.04x** [282.34x-429.05x] | 0.05x | 14.1 | 212.9 | 1223.0 | 553.33x [13.45x-2062.02x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **18.38x** [15.19x-22.24x] | 0.17x | 849.9 | 2094.6 | 4692.0 | 23.70x [0.19x-135.19x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **7.93x** [5.58x-11.27x] | 0.26x | 4731.0 | 3317.2 | 6704.6 | 10.05x [0.06x-40.73x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **11.77x** [9.48x-14.60x] | 0.23x | 5247.2 | 3413.1 | 7315.0 | 12.02x [0.06x-135.19x] |
| Tier 3 non-MAD MILPs | 4 (20) | **4.68x** [2.55x-8.61x] | 0.44x | 4406.4 | 1412.7 | 3006.1 | 6.70x [0.06x-19.75x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **34.77x** [28.57x-42.31x] | 0.10x | 335.2 | 2025.9 | 4202.8 | 40.66x [3.25x-109.71x] |
| Random-weight MAD (negative controls) | 3 (33) | **13.76x** [9.08x-20.84x] | 0.18x | 462.2 | 1300.9 | 2438.0 | 12.73x [1.67x-101.06x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **54.54x** [41.46x-71.74x] | 0.12x | 5259.9 | 3598.8 | 8416.8 | 48.12x [0.06x-2062.02x] |
| All 31 synthetic corpus families | 31 (293) | **49.43x** [39.69x-61.56x] | 0.11x | 5595.1 | 5624.7 | 12619.6 | 41.46x [0.06x-2062.02x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2. Synthetic MAD Group-Item Allocation Family (`G` Groups $\times$ `I` Items, $T = 6$ Periods)

The synthetic `MAD` group-item allocation family ([`BuildMADFromWeights`](bench/corpus.go)) assigns $I$ items across $G$ groups over $T = 6$ periods to minimize total L1 distance from each group's central level $m_g$ plus the roundness penalty $r_g = |m_g - 100 n_g|$ relative to an integer multiple $n_g \in \mathbb{Z}_{\ge 1}$. In the planted-gap generator, items in $S = \{0, \dots, I-G-1\}$ carry orthogonal curve signatures cancelled by the $G$ designated items $D = \{I-G, \dots, I-1\}$ with roundness-aligned period sums, yielding a zero root LP gap that root heuristics close at node 1; unstructured random-weight negative controls (`BuildMADRandom`: `t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`) are included directly in the corpus and detailed in [`docs/benchmarks.md`](docs/benchmarks.md).

<!-- BEGIN GENERATED:MAD_PIPELINE_TABLE -->
| Problem ID | $G \times I$ | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Trial 0 Obj | Parity |
| :--- | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **82.2x** [64.9x-109.7x] | 0.13 | 11.01 | 0.07 | 10.76 | 0.052 | 0.315 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **49.0x** [13.4x-101.1x] | 0.62 | 38.79 | 0.59 | 38.53 | 0.032 | 0.325 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **67.0x** [43.6x-79.0x] | 0.14 | 10.26 | 0.11 | 9.96 | 0.039 | 0.387 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **45.9x** [35.7x-67.4x] | 0.27 | 11.08 | 0.15 | 10.66 | 0.094 | 0.562 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **32.6x** [23.7x-42.4x] | 0.38 | 12.83 | 0.27 | 12.19 | 0.102 | 0.847 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **10.8x** [3.3x-41.8x] | 4.23 | 51.39 | 4.18 | 51.04 | 0.049 | 0.419 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **4.9x** [1.7x-24.3x] | 31.09 | 178.44 | 31.01 | 177.95 | 0.088 | 0.603 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **27.5x** [20.5x-40.7x] | 0.82 | 21.05 | 0.57 | 19.83 | 0.221 | 1.471 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **13.7x** [8.1x-21.6x] | 1.52 | 21.27 | 1.15 | 19.36 | 0.370 | 2.462 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **10.7x** [10.0x-11.8x] | 6.64 | 70.19 | 5.04 | 61.62 | 1.283 | 11.629 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **8.1x** [3.2x-22.0x] | 51.06 | 394.23 | 44.58 | 358.74 | 5.466 | 46.777 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

Cross-instance total latency dispersion across the randomized instances in each family (mean $\pm$ sample SD and two-sided 95% Student-$t$ interval after warmup):

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.14 \pm 0.02$ ($\pm 0.01$) | $11.02 \pm 0.52$ ($\pm 0.35$) | $0.12 \pm 0.01$ ($\pm 0.01$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.96 \pm 0.86$ ($\pm 0.58$) | $35.07 \pm 10.94$ ($\pm 7.35$) | $0.98 \pm 0.86$ ($\pm 0.58$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.16 \pm 0.05$ ($\pm 0.03$) | $10.54 \pm 0.62$ ($\pm 0.42$) | $0.15 \pm 0.02$ ($\pm 0.01$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.25 \pm 0.06$ ($\pm 0.04$) | $11.15 \pm 0.20$ ($\pm 0.14$) | $0.22 \pm 0.05$ ($\pm 0.04$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.41 \pm 0.08$ ($\pm 0.05$) | $13.12 \pm 0.85$ ($\pm 0.57$) | $0.40 \pm 0.11$ ($\pm 0.08$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $4.17 \pm 2.22$ ($\pm 1.49$) | $38.73 \pm 18.21$ ($\pm 12.23$) | $4.27 \pm 2.27$ ($\pm 1.52$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $36.89 \pm 30.37$ ($\pm 20.40$) | $147.84 \pm 67.52$ ($\pm 45.36$) | $37.18 \pm 30.80$ ($\pm 20.69$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.77 \pm 0.14$ ($\pm 0.18$) | $20.85 \pm 2.61$ ($\pm 3.24$) | $0.77 \pm 0.13$ ($\pm 0.16$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.65 \pm 0.62$ ($\pm 0.77$) | $21.36 \pm 0.27$ ($\pm 0.34$) | $1.57 \pm 0.56$ ($\pm 0.70$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $6.49 \pm 0.24$ ($\pm 0.30$) | $69.32 \pm 4.34$ ($\pm 5.39$) | $6.48 \pm 0.34$ ($\pm 0.42$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $56.03 \pm 15.18$ ($\pm 18.85$) | $628.17 \pm 673.03$ ($\pm 835.54$) | $53.39 \pm 17.09$ ($\pm 21.22$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### 3. Full 31-Family Synthetic Benchmark Corpus (`JSON -> Solution`)

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | **826.5x** [548.6x-1243.1x] | 0.01 | 7.02 | 0.00 | 6.97 | 0.006 | 0.076 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **895.4x** [598.3x-1232.8x] | 0.01 | 7.28 | 0.00 | 7.22 | 0.006 | 0.081 | `OK` |
| `t1_std_mip` | 3 | 3 | **605.0x** [245.9x-952.8x] | 0.01 | 8.50 | 0.01 | 8.43 | 0.006 | 0.090 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **675.4x** [502.6x-972.5x] | 0.01 | 8.52 | 0.01 | 8.46 | 0.006 | 0.089 | `OK` |
| `t1_whiskas` | 6 | 5 | **494.0x** [354.1x-712.9x] | 0.02 | 7.43 | 0.00 | 7.34 | 0.011 | 0.118 | `OK` |
| `t1_furniture` | 2 | 3 | **311.4x** [100.7x-636.9x] | 0.03 | 9.14 | 0.02 | 9.07 | 0.005 | 0.089 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **140.8x** [67.9x-268.8x] | 0.08 | 10.48 | 0.07 | 10.38 | 0.013 | 0.138 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **988.3x** [69.4x-2062.0x] | 0.01 | 7.04 | 0.00 | 6.99 | 0.005 | 0.073 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **82.2x** [64.9x-109.7x] | 0.13 | 11.01 | 0.07 | 10.76 | 0.052 | 0.315 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **49.0x** [13.4x-101.1x] | 0.62 | 38.79 | 0.59 | 38.53 | 0.032 | 0.325 | `OK` |
| `t2_mkp_30` | 30 | 6 | **8.3x** [4.3x-16.9x] | 2.95 | 30.21 | 2.92 | 30.03 | 0.032 | 0.252 | `OK` |
| `t2_mkp_50` | 50 | 8 | **5.5x** [2.5x-55.9x] | 4.97 | 19.33 | 4.91 | 19.06 | 0.049 | 0.368 | `OK` |
| `t2_cflp_36` | 36 | 11 | **32.6x** [4.7x-110.4x] | 0.25 | 10.85 | 0.20 | 10.65 | 0.038 | 0.250 | `OK` |
| `t2_cflp_49` | 49 | 13 | **28.7x** [10.7x-62.4x] | 0.38 | 11.72 | 0.35 | 11.49 | 0.030 | 0.292 | `OK` |
| `t2_gap_45` | 45 | 13 | **12.2x** [2.4x-32.5x] | 1.02 | 16.24 | 0.99 | 16.02 | 0.034 | 0.282 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **10.8x** [3.9x-30.3x] | 4.85 | 74.28 | 4.77 | 74.02 | 0.043 | 0.326 | `OK` |
| `t2_binpack_44` | 44 | 14 | **15.1x** [0.2x-88.0x] | 0.46 | 15.02 | 0.43 | 14.82 | 0.028 | 0.282 | `OK` |
| `t2_setcover_50` | 50 | 20 | **59.8x** [5.6x-135.2x] | 0.16 | 12.11 | 0.12 | 11.76 | 0.044 | 0.435 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **67.0x** [43.6x-79.0x] | 0.14 | 10.26 | 0.11 | 9.96 | 0.039 | 0.387 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **45.9x** [35.7x-67.4x] | 0.27 | 11.08 | 0.15 | 10.66 | 0.094 | 0.562 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **32.6x** [23.7x-42.4x] | 0.38 | 12.83 | 0.27 | 12.19 | 0.102 | 0.847 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **10.8x** [3.3x-41.8x] | 4.23 | 51.39 | 4.18 | 51.04 | 0.049 | 0.419 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **4.9x** [1.7x-24.3x] | 31.09 | 178.44 | 31.01 | 177.95 | 0.088 | 0.603 | `OK` |
| `t3_mkp_200` | 200 | 8 | **3.2x** [2.8x-3.9x] | 37.55 | 124.49 | 37.44 | 123.85 | 0.111 | 0.846 | `OK` |
| `t3_cflp_300` | 300 | 34 | **13.1x** [9.5x-19.7x] | 15.38 | 265.30 | 15.20 | 264.35 | 0.180 | 1.239 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **1.0x** [0.1x-2.5x] | 100.35 | 167.99 | 99.84 | 165.97 | 0.366 | 2.460 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **11.5x** [9.6x-14.1x] | 8.02 | 85.72 | 7.59 | 82.61 | 0.445 | 3.680 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **27.5x** [20.5x-40.7x] | 0.82 | 21.05 | 0.57 | 19.83 | 0.221 | 1.471 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **13.7x** [8.1x-21.6x] | 1.52 | 21.27 | 1.15 | 19.36 | 0.370 | 2.462 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **10.7x** [10.0x-11.8x] | 6.64 | 70.19 | 5.04 | 61.62 | 1.283 | 11.629 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **8.1x** [3.2x-22.0x] | 51.06 | 394.23 | 44.58 | 358.74 | 5.466 | 46.777 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

### 4. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus ([`bench/assign_corpus.go`](bench/assign_corpus.go), 200 randomized instances across square, rectangular wide/tall, degenerate integer-tie, Euclidean geometric, rank-1 outer-product, sparse `+Inf`, and maximization families) compares [`assign.LinearSumFlat`](assign/assign.go) against SciPy's C++ `rectangular_lsap` extension (`scipy.optimize.linear_sum_assignment`) in warm state on bit-identical `float64` cost matrices:

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Instances) | Paired `SciPy Solve / goblin` GeoMean [95% CI] | Paired `SciPy Total / goblin` GeoMean | `goblin` Solve Sum (ms) | SciPy Solve Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.69x** [1.56x-1.84x] | 4.82x | 0.217 | 0.339 | 1.69x [0.62x-3.64x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **3.15x** [2.78x-3.57x] | 3.05x | 10.499 | 35.227 | 2.73x [0.84x-17.40x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.87x** [2.47x-3.33x] | 3.70x | 260.127 | 1063.404 | 2.88x [0.96x-5.98x] |
| Square families ($m = n$) | 16 (146) | **2.79x** [2.52x-3.10x] | 3.93x | 265.980 | 1087.341 | 2.55x [0.62x-17.40x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.92x** [1.76x-2.08x] | 3.04x | 4.863 | 11.629 | 2.09x [0.82x-3.28x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.47x** [2.03x-3.01x] | 2.90x | 37.402 | 69.561 | 1.94x [0.81x-17.40x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.52x** [2.32x-2.74x] | 3.67x | 270.843 | 1098.970 | 2.27x [0.62x-17.40x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **2.40x** | 0.0005 | 0.0013 | 0.0006 | 0.0206 | $0.0008 \pm 0.0009$ | $0.0014 \pm 0.0003$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.75x** | 0.0014 | 0.0024 | 0.0017 | 0.0106 | $0.0014 \pm 0.0004$ | $0.0024 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.45x** | 0.0042 | 0.0065 | 0.0061 | 0.0159 | $0.0045 \pm 0.0016$ | $0.0064 \pm 0.0017$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.44x** | 0.0029 | 0.0042 | 0.0045 | 0.0148 | $0.0030 \pm 0.0011$ | $0.0041 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.55x** | 0.0031 | 0.0051 | 0.0049 | 0.0160 | $0.0034 \pm 0.0012$ | $0.0052 \pm 0.0019$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.72x** | 0.0064 | 0.0115 | 0.0116 | 0.0345 | $0.0067 \pm 0.0021$ | $0.0112 \pm 0.0021$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.80x** | 0.0288 | 0.0753 | 0.0395 | 0.102 | $0.0267 \pm 0.0061$ | $0.0734 \pm 0.0109$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.92x** | 0.0554 | 0.186 | 0.0735 | 0.238 | $0.0540 \pm 0.0053$ | $0.242 \pm 0.189$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.32x** | 0.0464 | 0.0999 | 0.0703 | 0.151 | $0.0437 \pm 0.0042$ | $0.102 \pm 0.0146$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.67x** | 0.145 | 0.233 | 0.153 | 0.274 | $0.154 \pm 0.0459$ | $0.252 \pm 0.0488$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **13.47x** | 0.0574 | 0.721 | 0.0799 | 0.800 | $0.0544 \pm 0.0072$ | $0.726 \pm 0.0187$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **2.23x** | 0.0332 | 0.0737 | 0.0577 | 0.152 | $0.0357 \pm 0.0128$ | $0.0775 \pm 0.0124$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.11x** | 0.0400 | 0.0845 | 0.0671 | 0.163 | $0.0403 \pm 0.0030$ | $0.0851 \pm 0.0078$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.24x** | 0.0875 | 0.299 | 0.118 | 0.382 | $0.0938 \pm 0.0151$ | $0.302 \pm 0.0321$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.46x** | 0.334 | 1.348 | 0.445 | 1.815 | $0.452 \pm 0.384$ | $1.342 \pm 0.130$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.16x** | 1.323 | 5.552 | 1.755 | 8.231 | $1.356 \pm 0.145$ | $5.629 \pm 0.405$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.69x** | 0.836 | 1.566 | 1.417 | 4.602 | $0.975 \pm 0.364$ | $1.579 \pm 0.0676$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.70x** | 5.325 | 9.132 | 5.593 | 10.215 | $5.926 \pm 2.089$ | $9.917 \pm 2.859$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.85x** | 0.325 | 0.879 | 0.728 | 3.962 | $0.318 \pm 0.0137$ | $0.905 \pm 0.0630$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.21x** | 0.480 | 1.042 | 0.869 | 4.044 | $0.474 \pm 0.0480$ | $1.042 \pm 0.0360$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.84x** | 6.604 | 32.980 | 8.477 | 43.616 | $6.690 \pm 0.489$ | $32.521 \pm 4.219$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.43x** | 36.174 | 150.660 | 43.823 | 198.987 | $36.287 \pm 2.528$ | $161.087 \pm 18.550$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

---

## Pure-Go vs. C++ CBC Performance Gaps and MAD Scaling Analysis

Comparing isolated unpresolved `Root LP` timings, paired trial ratios, and solver-core checkpoints (`CP5 +Presolve`, `CP6 +Cuts`, `CP7 Full`) between `BackendGo` and in-process `BackendCBC` (`c1c171ec`) highlights four structural trade-offs on large degenerate LPs and heavy-tail branch-and-cut models.

### 1. MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families (`N = G(I + T + 3)` variables, `M = G(2T + 3) + I` constraints) separate solver performance into three distinct scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Paired `Go / CBC` | Go Total Med (ms) | CBC Total Med (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.05x** | 0.13 | 2.70 | 0.56x | 0.07 (1 / 19) | 2.65 (0 / 0) | 0.071 (28) | 0.126 (19) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.07x** | 0.62 | 12.30 | 0.52x | 0.59 (5 / 124) | 12.25 (6 / 83) | 0.070 (30) | 0.134 (18) |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.08x** | 0.14 | 1.87 | 0.70x | 0.11 (1 / 26) | 1.83 (0 / 0) | 0.086 (33) | 0.124 (21) |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.09x** | 0.27 | 2.63 | 0.96x | 0.15 (1 / 39) | 2.55 (0 / 0) | 0.211 (59) | 0.220 (40) |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.11x** | 0.38 | 3.47 | 0.72x | 0.27 (1 / 51) | 3.35 (0 / 0) | 0.269 (68) | 0.371 (63) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.23x** | 4.23 | 17.60 | 0.87x | 4.18 (43 / 580) | 17.55 (12 / 216) | 0.118 (38) | 0.136 (24) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.36x** | 31.09 | 93.36 | 0.92x | 31.01 (137 / 1910) | 93.29 (198 / 1300) | 0.242 (66) | 0.264 (54) |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.09x** | 0.82 | 8.52 | 1.43x | 0.57 (1 / 75) | 8.36 (0 / 0) | 0.909 (138) | 0.637 (94) |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.19x** | 1.52 | 8.14 | 1.45x | 1.15 (1 / 105) | 7.72 (0 / 0) | 1.428 (246) | 0.982 (149) |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **0.17x** | 6.64 | 38.54 | 4.83x | 5.04 (1 / 255) | 37.35 (0 / 0) | 15.561 (962) | 3.223 (319) |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **0.21x** | 51.06 | 310.56 | 3.35x | 44.58 (1 / 2439) | 305.39 (0 / 0) | 116.617 (2314) | 34.848 (3128) |
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
| `t3_mkp_200` | $200 \times 8$ | **0.73x** | $32.72 \pm 16.35$ | $43.41 \pm 20.57$ | 37.44 (594 / 5502) | 52.19 (154 / 1145) | 45.29 (701 / 14) | 97.51 (5000, `NodeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **0.41x** | $8.00 \pm 9.52$ | $13.19 \pm 9.63$ | 4.91 (1 / 24) | 9.24 (0 / 8) | 4.45 (2 / 4) | 4.36 (132) |
| `t2_binpack_44` | $44 \times 14$ | **0.15x** | $12.13 \pm 34.71$ | $7.05 \pm 2.25$ | 0.43 (1 / 45) | 6.43 (0 / 8) | 0.96 (13 / 5) | 15.36 (1838) |
| `t3_cflp_300` | $300 \times 34$ | **0.25x** | $19.51 \pm 8.28$ | $73.55 \pm 17.69$ | 15.20 (48 / 2131) | 71.05 (18 / 483) | 22.93 (63 / 31) | 6.59 (134) |
| `t3_lotsizing_504` | $504 \times 336$ | **1.73x** | $821.47 \pm 1666.05$ | $98.89 \pm 61.18$ | 99.84 (27 / 1284) | 76.40 (10 / 377) | 122.63 (416 / 203) | 243.02 (5000, `NodeLimit`) |
| `t3_sched_1000` | $1000 \times 100$ | **0.11x** | $7.58 \pm 1.20$ | $66.69 \pm 3.48$ | 7.59 (35 / 431) | 65.61 (0 / 76) | 12.64 (96 / 47) | 247.87 (5000, `NodeLimit`) |
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
