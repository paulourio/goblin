# `goblin`: Pure-Go LP, MILP, and Linear Sum Assignment Solver with Optional C++ CBC Backend

The `goblin` module provides an in-memory Linear Programming (LP) and Mixed-Integer Linear Programming (MILP) modeling engine and optional parallel Branch-and-Cut solver (`WithWorkers`) alongside the [`goblin/assign`](assign/assign.go) Linear Sum Assignment Problem (`LSAP`) solver, written in Go 1.27. It compiles algebraic models directly into encapsulated Dual-Index Compressed Sparse Row and Column (`CSR`/`CSC`) matrices (`CompiledProblem`) and streams standard `PuLP` JSON payloads directly into `Model` (`FromPuLPJSONBytes`), eliminating intermediate AST allocations, text-file I/O, and subprocess spawn costs. By default it builds as pure Go with zero `cgo` dependency (`BackendGo`), and nested module isolation (`third_party/go.mod`) keeps `go get github.com/paulourio/goblin` lightweight; callers who build with `-tags cbc` (`cgo` enabled, or `-tags "cbc,cbc_system"` against system COIN-OR packages) can also select the in-process C++ COIN-OR CBC/CLP backend (`WithCBC()` / `WithBackend(BackendCBC)`) or configure solver flags (`WithoutSIMD()`, `WithWorkers(...)`, `WithTimeLimit(...)`, etc.) through functional arguments on `Solve` and `SolveCompiled`.

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

	// Or enable parallel B&B workers, select the C++ COIN-OR CBC backend (-tags cbc), or disable SIMD:
	// resPar, err := m.Solve(ctx, gb.WithWorkers(4))
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

1. **Zero-Subprocess In-Memory Execution**: Modeling (`Model`), matrix checks and assembly (`Model.Validate()`, `Model.CompileChecked()`), presolve (`Presolve`), root and node LPs (`LPSolver`), and Branch-and-Cut (`SolveCompiled`) execute in-process in pure Go using pooled workspace buffers (`AcquireWorkspace` / `ReleaseWorkspace`) with zero `cgo` dependency by default, plus an optional `-tags cbc` in-process `cgo` bridge to COIN-OR CBC/CLP (`cbc_cgo.go`, serialized process-wide across goroutines to guard unsynchronized upstream `Cgl`/`Cbc` static state).
2. **Schema Compatibility with Python `PuLP` and Free-Format MPS Export**: The `goblin/format` package reads and writes `PuLP`'s native `toDict()` / `fromDict()` JSON schema ([`format/pulp_json.go`](format/pulp_json.go)) via direct streaming ingestion ([`FromPuLPJSONBytes`](format/pulp_json.go)) with automatic AST fallback, and writes free-format MPS files ([`format/mps.go`](format/mps.go)).
3. **Optional Multi-Tier SIMD Kernels**: With `GOEXPERIMENT=simd` on `amd64` (gated at runtime on CPU `AVX-512`, `AVX2`, and `FMA` support), [`internal/simd/kernels_amd64.go`](internal/simd/kernels_amd64.go) and [`internal/simd/kernels_avx512_amd64.go`](internal/simd/kernels_avx512_amd64.go) dispatch 512-bit AVX-512 or 256-bit AVX2 vector loops (`Dot64`, `Axpy64`, `SelectDualLeaveRow64With`, `ScanDualRatioCand64With`, `MinReducedSkip64`, `InitReducedRow64`, `Min2ReducedRow64`, `AllAbsLE64`, and `RelaxRow64`) with `StoreArray` + `VZEROUPPER` cleanup and IEEE-754 `NaN` propagation parity across scalar and vector paths, and can be toggled at runtime via `WithoutSIMD()` or `WithSIMD(false)`. See the paired comparison in [`docs/benchmarks.md`](docs/benchmarks.md#5-default-build-vs-goexperimentsimd).
4. **Verified Parity on Synthetic and Public Suites**: [`solver_test.go`](solver_test.go) re-solves all 293 distinct randomized trial instances of the 31 synthetic benchmark families (`testdata/corpus/trials/*.jsonl`), compares status and objective with `PuLP + COIN-OR CBC` (`c1c171ec`) and `BackendCBC`, and verifies primal and row feasibility on every result. [`public_benchmarks_test.go`](public_benchmarks_test.go) verifies optimal status, objective parity, and primal feasibility across 20 held-out public Netlib LP and MIPLIB 3.0 MILP instances (`testdata/public/`) plus a valid dual lower bound under a 25-node budget with cuts and heuristics disabled (`WithCuts(false)`, `WithHeuristics(false)`, `WithMaxNodes(25)`) on `coinor_capplan1`, while [`regression_test.go`](regression_test.go) tests big-M fixed-charge models against enumerated ground truth and Farkas-certified LP scaling edge cases. `TimeLimit`, `MaxNodes`, and `MaxIter` return `StatusTimeLimit`, `StatusNodeLimit`, or `StatusIterationLimit` with a valid global bound once the root LP is solved; a node that cannot be evaluated is never discarded silently.

---

## Documentation Index

Full technical guides live in the [`docs/`](docs/index.md) directory:

- **[Architecture and Design Overview](docs/index.md)**: Module boundaries, data flow from algebraic expressions to encapsulated `CSR`/`CSC` matrices, and memory pooling invariants.
- **[Modeling API and Serialization Formats](docs/modeling-and-formats.md)**: Constructing variables, linear expressions, and constraints, configuring solver flags and backends, validating and compiling models, and exchanging `PuLP` JSON and MPS payloads.
- **[Solver Internals and Mathematical Specification](docs/solver-internals.md)**: Three-stage singleton and sparse Markowitz LU with `FTRANSparseHint`/`BTRANSparseHint`, `BTRANExact`, and in-place Forrest-Tomlin $U$ updates, Revised Dual Simplex with Bound-Flipping Ratio Test (`selectDualBFRT`) and degenerate cost perturbation, LP and MILP presolve with static and dynamic orbital symmetry breaking, root and in-tree cutting planes (single-row and multi-row MIR, `GMI`, lifted covers, Flow/VUB), primal heuristics, optional parallel Branch-and-Cut (`WithWorkers`), and post-ARR sparse-core LSAP pricing.
- **[Benchmark Methodology and Corpus Results](docs/benchmarks.md)**: Hardware setup, single-session comparative telemetry against Python `PuLP + CBC` (`c1c171ec`) and in-process `BackendCBC`, paired scalar-vs-SIMD statistics, structural analysis of the synthetic Mean Absolute Deviation (`MAD`) `G x I` allocation family vs. random-weight negative controls (`BuildMADRandom`), and full checkpoint telemetry.

---

## Benchmarks

All benchmark tables in `README.md`, [`docs/benchmarks.md`](docs/benchmarks.md), and [`bench/results/final_report.md`](bench/results/final_report.md) are rendered deterministically from the seven JSON telemetry files in `bench/results/` (`python_baseline.json`, `goblin_cbc.json`, `goblin_scalar.json`, `goblin_simd.json`, `scipy_assign_baseline.json`, `goblin_assign_scalar.json`, `goblin_assign_simd.json`). [`doc_sync_test.go`](doc_sync_test.go) fails `go test ./...` if any table drifts from the telemetry, if any `goblin` build disagrees with CBC or SciPy on a status or objective, or if any telemetry file differs in git commit, source/corpus fingerprint, or host kernel. Headline timings use the default pure-Go scalar build (`BackendGo`).

<!-- BEGIN GENERATED:PROVENANCE -->
- **Unified telemetry source**: fingerprint `61682e72894e7404` (commit `9262f513905304ba6067db9778bc29b1dfdd7842`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.32 ms median on a 1-variable MILP and 6.31 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

Read the synthetic corpus ratios with three methodological notes. First, Tier 1 micro-models (`N <= 26`) are dominated by Python's CBC subprocess spawn floor reported in the provenance block above. Second, the planted-gap MAD family has a zero root LP gap by construction and closes at the root without branching. Consequently, the bold row (`Tier 2+3 non-planted MILPs`) isolates solver-dominated MILPs across paired randomized instances, alongside total subset wall times and full per-instance `[Min-Max]` ranges.

### 1. Aggregate Synthetic Workload Summary (Paired Per-Instance Geometric Mean and Wall-Clock Sums)

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `Py / Go` GeoMean [Clustered 95% CI] | Pooled Trial `Py / Go` | Equal-Family `Go / CBC` | `Go` Trial (Med) Sum (ms) | `CBC` Trial (Med) Sum (ms) | `Py` Trial (Med) Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **333.55x** [159.25x-698.63x] | 333.55x | 0.05x | 9.0 (0.8) | 194.9 (19.3) | 1045.8 (99.1) | 508.83x [32.61x-1675.45x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **21.50x** [12.97x-35.65x] | 21.50x | 0.14x | 547.1 (31.3) | 1956.3 (170.9) | 4317.9 (417.3) | 27.19x [0.18x-164.03x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **10.90x** [5.19x-22.90x] | 10.90x | 0.19x | 846.5 (134.4) | 2918.1 (560.2) | 6189.1 (1066.7) | 10.87x [1.13x-68.92x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **14.08x** [8.32x-23.84x] | 15.42x | 0.18x | 1095.8 (122.9) | 3121.6 (393.9) | 6771.2 (987.5) | 15.53x [0.18x-164.03x] |
| Tier 3 non-MAD MILPs | 4 (20) | **8.62x** [1.51x-49.10x] | 8.62x | 0.25x | 557.0 (92.3) | 1252.0 (230.4) | 2811.3 (601.1) | 6.51x [1.13x-68.92x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **27.81x** [12.66x-61.09x] | 36.17x | 0.10x | 298.8 (43.0) | 1778.3 (339.7) | 3838.5 (506.0) | 43.20x [1.18x-129.24x] |
| Random-weight MAD (negative controls) | 3 (33) | **17.19x** [1.04x-283.03x] | 17.19x | 0.14x | 246.7 (19.3) | 1212.7 (116.6) | 2246.2 (248.7) | 15.56x [1.45x-112.59x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **51.14x** [22.61x-115.67x] | 61.63x | 0.11x | 1103.7 (123.5) | 3291.0 (410.7) | 7714.4 (1077.1) | 50.43x [0.18x-1675.45x] |
| All 31 synthetic corpus families | 31 (293) | **43.70x** [23.42x-81.55x] | 54.86x | 0.11x | 1402.6 (166.5) | 5069.3 (750.4) | 11552.9 (1583.1) | 45.89x [0.18x-1675.45x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2. Synthetic MAD Group-Item Allocation Family (`G` Groups $\times$ `I` Items, $T = 6$ Periods)

The synthetic `MAD` group-item allocation family ([`BuildMADFromWeights`](bench/corpus.go)) assigns $I$ items across $G$ groups over $T = 6$ periods to minimize total L1 distance from each group's central level $m_g$ plus the roundness penalty $r_g = |m_g - 100 n_g|$ relative to an integer multiple $n_g \in \mathbb{Z}_{\ge 1}$. In the planted-gap generator, items in $S = \{0, \dots, I-G-1\}$ carry orthogonal curve signatures cancelled by the $G$ designated items $D = \{I-G, \dots, I-1\}$ with roundness-aligned period sums, yielding a zero root LP gap that root heuristics close without branching; unstructured random-weight negative controls (`BuildMADRandom`: `t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`) are included directly in the corpus and detailed in [`docs/benchmarks.md`](docs/benchmarks.md).

<!-- BEGIN GENERATED:MAD_PIPELINE_TABLE -->
| Problem ID | $G \times I$ | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Trial 0 Obj | Parity |
| :--- | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **102.9x** [85.8x-129.2x] | 0.09 | 9.43 | 0.05 | 9.22 | 0.034 | 0.211 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **62.7x** [38.7x-112.6x] | 0.49 | 33.97 | 0.44 | 33.73 | 0.032 | 0.238 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **59.6x** [49.2x-83.5x] | 0.18 | 9.76 | 0.14 | 9.46 | 0.038 | 0.307 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **54.8x** [41.6x-62.8x] | 0.17 | 9.83 | 0.12 | 9.42 | 0.045 | 0.400 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **29.4x** [27.2x-33.8x] | 0.39 | 11.31 | 0.30 | 10.71 | 0.092 | 0.604 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **10.1x** [3.6x-30.6x] | 3.03 | 47.19 | 2.98 | 46.90 | 0.038 | 0.307 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **8.0x** [1.4x-30.0x] | 15.76 | 167.54 | 15.69 | 167.11 | 0.058 | 0.431 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **32.3x** [26.1x-39.0x] | 0.53 | 17.94 | 0.41 | 16.80 | 0.112 | 1.017 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **17.2x** [11.2x-23.1x] | 1.04 | 18.30 | 0.82 | 16.58 | 0.220 | 1.719 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **6.2x** [1.2x-15.1x] | 7.78 | 60.43 | 6.83 | 52.61 | 0.994 | 8.137 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **10.5x** [4.6x-25.5x] | 32.79 | 368.98 | 29.21 | 335.58 | 3.905 | 34.109 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

Cross-instance total latency dispersion across the randomized instances in each family (mean $\pm$ sample SD and two-sided 95% Student-$t$ interval after warmup):

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.09 \pm 0.01$ ($\pm 0.01$) | $9.33 \pm 0.49$ ($\pm 0.33$) | $0.09 \pm 0.00$ ($\pm 0.00$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.51 \pm 0.22$ ($\pm 0.15$) | $30.39 \pm 9.49$ ($\pm 6.38$) | $0.46 \pm 0.20$ ($\pm 0.14$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.18 \pm 0.02$ ($\pm 0.01$) | $11.01 \pm 2.36$ ($\pm 1.59$) | $0.15 \pm 0.01$ ($\pm 0.01$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.18 \pm 0.03$ ($\pm 0.02$) | $9.87 \pm 0.16$ ($\pm 0.10$) | $0.21 \pm 0.04$ ($\pm 0.02$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.40 \pm 0.01$ ($\pm 0.01$) | $11.67 \pm 0.62$ ($\pm 0.41$) | $0.33 \pm 0.01$ ($\pm 0.01$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $3.95 \pm 2.90$ ($\pm 1.95$) | $35.42 \pm 17.10$ ($\pm 11.49$) | $3.98 \pm 3.03$ ($\pm 2.04$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $17.97 \pm 11.79$ ($\pm 7.92$) | $138.39 \pm 63.36$ ($\pm 42.56$) | $17.93 \pm 11.46$ ($\pm 7.70$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.54 \pm 0.05$ ($\pm 0.06$) | $17.65 \pm 2.40$ ($\pm 2.98$) | $0.57 \pm 0.09$ ($\pm 0.12$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.10 \pm 0.32$ ($\pm 0.40$) | $18.37 \pm 0.22$ ($\pm 0.27$) | $1.05 \pm 0.33$ ($\pm 0.41$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $15.69 \pm 19.67$ ($\pm 24.42$) | $62.54 \pm 3.90$ ($\pm 4.84$) | $15.96 \pm 19.61$ ($\pm 24.34$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $40.55 \pm 14.11$ ($\pm 17.52$) | $577.01 \pm 608.27$ ($\pm 755.15$) | $38.30 \pm 14.00$ ($\pm 17.38$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### 3. Full 31-Family Synthetic Benchmark Corpus (`JSON -> Solution`)

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | **786.0x** [491.6x-1203.0x] | 0.01 | 6.28 | 0.00 | 6.22 | 0.004 | 0.051 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **668.8x** [287.0x-1088.2x] | 0.01 | 6.16 | 0.00 | 6.11 | 0.005 | 0.050 | `OK` |
| `t1_std_mip` | 3 | 3 | **690.1x** [465.2x-943.3x] | 0.01 | 7.32 | 0.00 | 7.27 | 0.005 | 0.054 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **741.5x** [556.9x-1122.4x] | 0.01 | 7.32 | 0.00 | 7.27 | 0.005 | 0.055 | `OK` |
| `t1_whiskas` | 6 | 5 | **485.4x** [374.2x-583.8x] | 0.01 | 6.24 | 0.01 | 6.17 | 0.007 | 0.072 | `OK` |
| `t1_furniture` | 2 | 3 | **237.3x** [77.5x-600.0x] | 0.02 | 7.43 | 0.02 | 7.38 | 0.005 | 0.048 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **90.1x** [32.6x-283.6x] | 0.11 | 8.80 | 0.10 | 8.72 | 0.013 | 0.084 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **945.4x** [456.2x-1675.5x] | 0.01 | 6.13 | 0.00 | 6.08 | 0.005 | 0.041 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **102.9x** [85.8x-129.2x] | 0.09 | 9.43 | 0.05 | 9.22 | 0.034 | 0.211 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **62.7x** [38.7x-112.6x] | 0.49 | 33.97 | 0.44 | 33.73 | 0.032 | 0.238 | `OK` |
| `t2_mkp_30` | 30 | 6 | **11.6x** [6.3x-16.7x] | 2.08 | 28.31 | 2.06 | 28.14 | 0.017 | 0.173 | `OK` |
| `t2_mkp_50` | 50 | 8 | **5.9x** [2.0x-31.9x] | 4.03 | 16.44 | 4.00 | 16.20 | 0.032 | 0.244 | `OK` |
| `t2_cflp_36` | 36 | 11 | **58.6x** [24.4x-164.0x] | 0.15 | 8.93 | 0.14 | 8.77 | 0.017 | 0.158 | `OK` |
| `t2_cflp_49` | 49 | 13 | **38.4x** [23.6x-103.4x] | 0.31 | 10.14 | 0.29 | 9.93 | 0.022 | 0.200 | `OK` |
| `t2_gap_45` | 45 | 13 | **20.4x** [3.3x-61.4x] | 0.81 | 14.64 | 0.78 | 14.43 | 0.024 | 0.207 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **13.1x** [6.2x-31.2x] | 3.67 | 68.71 | 3.60 | 68.45 | 0.032 | 0.249 | `OK` |
| `t2_binpack_44` | 44 | 14 | **12.1x** [0.2x-44.4x] | 0.55 | 13.58 | 0.52 | 13.38 | 0.021 | 0.196 | `OK` |
| `t2_setcover_50` | 50 | 20 | **54.4x** [7.2x-144.9x] | 0.18 | 10.93 | 0.13 | 10.63 | 0.031 | 0.299 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **59.6x** [49.2x-83.5x] | 0.18 | 9.76 | 0.14 | 9.46 | 0.038 | 0.307 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **54.8x** [41.6x-62.8x] | 0.17 | 9.83 | 0.12 | 9.42 | 0.045 | 0.400 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **29.4x** [27.2x-33.8x] | 0.39 | 11.31 | 0.30 | 10.71 | 0.092 | 0.604 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **10.1x** [3.6x-30.6x] | 3.03 | 47.19 | 2.98 | 46.90 | 0.038 | 0.307 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **8.0x** [1.4x-30.0x] | 15.76 | 167.54 | 15.69 | 167.11 | 0.058 | 0.431 | `OK` |
| `t3_mkp_200` | 200 | 8 | **5.5x** [4.1x-6.7x] | 17.47 | 115.94 | 17.39 | 115.34 | 0.082 | 0.602 | `OK` |
| `t3_cflp_300` | 300 | 34 | **34.9x** [8.5x-68.9x] | 5.93 | 248.79 | 5.78 | 247.88 | 0.099 | 0.914 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **2.7x** [1.1x-5.0x] | 61.96 | 156.47 | 61.76 | 154.61 | 0.207 | 1.845 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **10.8x** [5.2x-16.3x] | 6.91 | 79.88 | 6.60 | 76.97 | 0.308 | 2.765 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **32.3x** [26.1x-39.0x] | 0.53 | 17.94 | 0.41 | 16.80 | 0.112 | 1.017 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **17.2x** [11.2x-23.1x] | 1.04 | 18.30 | 0.82 | 16.58 | 0.220 | 1.719 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **6.2x** [1.2x-15.1x] | 7.78 | 60.43 | 6.83 | 52.61 | 0.994 | 8.137 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **10.5x** [4.6x-25.5x] | 32.79 | 368.98 | 29.21 | 335.58 | 3.905 | 34.109 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

### 4. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus ([`bench/assign_corpus.go`](bench/assign_corpus.go), 200 randomized instances across square, rectangular wide/tall, degenerate integer-tie, Euclidean geometric, rank-1 outer-product, sparse `+Inf`, and maximization families) compares [`assign.LinearSumFlat`](assign/assign.go) against SciPy's C++ `rectangular_lsap` extension (`scipy.optimize.linear_sum_assignment`) in warm state on bit-identical `float64` cost matrices:

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `SciPy Solve / goblin` GeoMean [Clustered 95% CI] | Pooled Trial `SciPy Solve / goblin` | Equal-Family `SciPy Total / goblin` | `goblin` Solve Trial (Med) Sum (ms) | SciPy Solve Trial (Med) Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.68x** [1.45x-1.95x] | 1.68x | 5.21x | 0.196 (0.0178) | 0.310 (0.0287) | 1.65x [0.84x-3.15x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **3.29x** [2.09x-5.19x] | 3.29x | 3.14x | 9.447 (0.736) | 32.770 (2.958) | 2.99x [1.05x-19.44x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **3.27x** [2.20x-4.88x] | 3.27x | 3.85x | 209.168 (41.734) | 948.956 (188.200) | 3.44x [1.51x-6.45x] |
| Square families ($m = n$) | 16 (146) | **2.96x** [2.18x-4.03x] | 2.88x | 4.00x | 214.957 (41.805) | 971.096 (189.222) | 2.42x [1.05x-19.44x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **2.22x** [1.55x-3.16x] | 2.07x | 3.46x | 3.855 (0.683) | 10.940 (1.964) | 2.39x [0.84x-3.63x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.45x** [1.20x-4.97x] | 2.55x | 2.94x | 33.955 (5.880) | 65.164 (10.968) | 1.91x [1.12x-19.44x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.74x** [2.16x-3.47x] | 2.63x | 3.85x | 218.812 (42.488) | 982.036 (191.187) | 2.41x [0.84x-19.44x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **2.05x** | 0.0004 | 0.0009 | 0.0005 | 0.0108 | $0.0004 \pm 0.0001$ | $0.0009 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.90x** | 0.0012 | 0.0023 | 0.0015 | 0.0102 | $0.0013 \pm 0.0003$ | $0.0023 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.68x** | 0.0036 | 0.0062 | 0.0054 | 0.0152 | $0.0036 \pm 0.0010$ | $0.0061 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.52x** | 0.0027 | 0.0040 | 0.0037 | 0.0147 | $0.0026 \pm 0.0006$ | $0.0039 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.41x** | 0.0030 | 0.0044 | 0.0042 | 0.0153 | $0.0032 \pm 0.0011$ | $0.0044 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.62x** | 0.0069 | 0.0109 | 0.0110 | 0.0325 | $0.0067 \pm 0.0017$ | $0.0106 \pm 0.0019$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.92x** | 0.0255 | 0.0703 | 0.0330 | 0.0969 | $0.0239 \pm 0.0051$ | $0.0689 \pm 0.0101$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.56x** | 0.0467 | 0.175 | 0.0638 | 0.226 | $0.0505 \pm 0.0136$ | $0.177 \pm 0.0267$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.22x** | 0.0410 | 0.0935 | 0.0630 | 0.142 | $0.0444 \pm 0.0120$ | $0.0968 \pm 0.0155$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.70x** | 0.134 | 0.217 | 0.149 | 0.264 | $0.142 \pm 0.0474$ | $0.234 \pm 0.0432$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **13.56x** | 0.0543 | 0.680 | 0.0712 | 0.755 | $0.0518 \pm 0.0110$ | $0.687 \pm 0.0213$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **2.49x** | 0.0281 | 0.0701 | 0.0539 | 0.142 | $0.0307 \pm 0.0117$ | $0.0734 \pm 0.0095$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.45x** | 0.0321 | 0.0851 | 0.0585 | 0.159 | $0.0356 \pm 0.0111$ | $0.0847 \pm 0.0074$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.79x** | 0.0754 | 0.282 | 0.103 | 0.358 | $0.0761 \pm 0.0155$ | $0.286 \pm 0.0323$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.66x** | 0.299 | 1.284 | 0.405 | 1.713 | $0.404 \pm 0.336$ | $1.271 \pm 0.123$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.53x** | 1.117 | 5.231 | 1.509 | 7.867 | $1.181 \pm 0.156$ | $5.332 \pm 0.392$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **2.22x** | 0.659 | 1.495 | 1.256 | 4.279 | $0.671 \pm 0.0275$ | $1.489 \pm 0.0529$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.70x** | 4.981 | 8.466 | 5.213 | 10.206 | $5.573 \pm 2.087$ | $9.267 \pm 2.705$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **3.29x** | 0.258 | 0.828 | 0.663 | 3.826 | $0.256 \pm 0.0167$ | $0.843 \pm 0.0556$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.76x** | 0.359 | 0.973 | 0.740 | 3.926 | $0.356 \pm 0.0301$ | $0.979 \pm 0.0339$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **5.10x** | 5.820 | 28.221 | 8.499 | 37.172 | $5.623 \pm 0.389$ | $28.697 \pm 2.281$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **5.11x** | 28.539 | 142.987 | 37.713 | 188.809 | $28.172 \pm 4.094$ | $143.185 \pm 11.272$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

---

## Pure-Go vs. C++ CBC Performance Gaps and MAD Scaling Analysis

Comparing isolated unpresolved `Root LP` timings, paired trial ratios, and solver-core checkpoints (`CP5 +Presolve`, `CP6 +Cuts`, `CP7 Full`) between `BackendGo` and in-process `BackendCBC` (`c1c171ec`) highlights four structural trade-offs on large degenerate LPs and heavy-tail branch-and-cut models.

### 1. MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families (`N = G(I + T + 3)` variables, `M = G(2T + 3) + I` constraints) separate solver performance into three distinct scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Paired `Go / CBC` | Go Total Med (ms) | CBC Total Med (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.04x** | 0.09 | 2.49 | 0.64x | 0.05 (1 / 19) | 2.46 (0 / 0) | 0.089 (28) | 0.139 (19) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.05x** | 0.49 | 11.24 | 0.69x | 0.44 (2 / 58) | 11.21 (6 / 83) | 0.100 (30) | 0.145 (18) |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.09x** | 0.18 | 1.79 | 0.74x | 0.14 (1 / 26) | 1.75 (0 / 0) | 0.109 (33) | 0.146 (21) |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.07x** | 0.17 | 2.48 | 0.98x | 0.12 (1 / 40) | 2.43 (0 / 0) | 0.246 (58) | 0.251 (40) |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.12x** | 0.39 | 3.15 | 0.88x | 0.30 (1 / 51) | 3.08 (0 / 0) | 0.354 (80) | 0.401 (63) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.23x** | 3.03 | 16.77 | 0.94x | 2.98 (43 / 431) | 16.72 (12 / 216) | 0.152 (38) | 0.163 (24) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.22x** | 15.76 | 88.64 | 0.96x | 15.69 (288 / 2620) | 88.58 (198 / 1300) | 0.274 (65) | 0.285 (54) |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.07x** | 0.53 | 8.09 | 1.03x | 0.41 (1 / 75) | 7.97 (0 / 0) | 0.667 (91) | 0.645 (94) |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.14x** | 1.04 | 7.44 | 1.50x | 0.82 (1 / 98) | 7.24 (0 / 0) | 1.550 (244) | 1.035 (149) |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **0.28x** | 7.78 | 35.94 | 0.74x | 6.83 (2 / 364) | 34.96 (0 / 0) | 2.643 (224) | 3.587 (319) |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **0.16x** | 32.79 | 278.31 | 1.61x | 29.21 (1 / 2439) | 274.62 (0 / 0) | 57.331 (1900) | 35.650 (3128) |
<!-- END GENERATED:MAD_CBC_GAP_TABLE -->

1. **Small-to-Medium Planted-Gap MAD (`t1_mad_g2_i4` through `t3_mad_g5_i20`)**: Every structured `t*_mad_g*` instance has a zero root LP gap by design (`RootLPBound == OptimalObj`). Pure-Go closes those models at node 1 through presolve orbit fixing and root heuristics (`tryPropagatedRounding`, `tryRootSubMIP`, and `tryFractionalDiving` in [`heuristics.go`](heuristics.go)) without C++ `CbcModel` and `CglPreProcess` setup costs.
2. **Large Planted-Gap MAD (`t3_mad_g6_i30`, `t3_mad_g10_i100`, `t3_mad_g4_i1000`)**: Pure-Go `Go Solve` closes these models at the root (exact node counts and solve times in the generated table above) because [`Presolve`](presolve.go) verifies the $S_G$ group-permutation automorphism (`applyOrbitSymmetryBreaking`), fixes the $G \times G$ designated-item bijection to $I_G$, and eliminates $G^2$ binary variables and $2G$ assignment equations before the root LP runs, while [`FTRANSparseHint`](lu.go) and [`BTRANSparseHint`](lu.go) restrict basis triangular sweeps to active index windows. On the unpresolved `Root LP` column:
   - **Equality Crash Basis (`t3_mad_g10_i100`)**: [`crashEqualityBasis`](simplex.go) builds a lower-triangular, dual-feasible equality basis with analytical DSE weights (`LU`, `FTRAN`/`BTRAN`, and ratio times appear in the generated table above).
   - **Dense-Row Coupling and Forrest-Tomlin Updates (`t3_mad_g4_i1000`)**: On 1,000-row bases, [`crashEqualityBasis`](simplex.go) skips columns incident to coupling rows wider than 128 nonzeros so BFRT bound flips retain sparse unit slacks, while [`lu.go`](lu.go) performs in-place mutated-$U$ Forrest-Tomlin updates (`PushEtaSparse`) inside pre-allocated headroom arenas up to `maxEtaUpdates = 56`.
3. **Symmetric Random-Weight MAD (`t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`)**: Assigning dense random weights (`BuildMADRandom`) removes the zero-gap structure and forces Branch-and-Cut to prove optimality:
   - [`Presolve`](presolve.go) verifies exact $S_G$ group symmetry and fixes the $G \times G$ designated-item bijection to $I_G$, [`bab.go`](bab.go) exits after round 1 when root cuts yield zero bound gain and purges slack root cuts via [`PurgeSlackCuts`](simplex.go), and [`tryRootSubMIP`](heuristics.go) enforces a 50% RINS integer-fixing gate on zero-cut roots.
   - Across [`docs/benchmarks.md`](docs/benchmarks.md#mad-solver-core-checkpoint-progression), comparing `CP7 Full` against `CP5 +Presolve` (`Presolve=on, Cuts=off, Heuristics=off`) isolates the effect of cutting planes and primal heuristics on random-weight MAD search.

### 2. Non-MAD Corpus Comparison, Dispersion, and Checkpoint Trade-Offs

Across the non-MAD families, multi-row MIR cut aggregation (`separateAggregatedMIRCuts`), bound-cutoff strong-branching domain reductions (`selectBranchVarCore`), and globally valid in-tree cuts (`generateInTreeCutRound`) accelerate Branch-and-Cut convergence, while the generated `Go Total Mean +- SD` and `CBC Total Mean +- SD` columns capture seed-to-seed variance (for example outlier seeds in `t2_binpack_44`):

<!-- BEGIN GENERATED:NON_MAD_CBC_GAP_TABLE -->
| Problem ID | $N \times M$ | Paired `Go / CBC` | Go Total Mean $\pm$ SD (ms) | CBC Total Mean $\pm$ SD (ms) | Go CP7 Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go CP6 +Cuts (ms / nodes / cuts) | Go CP5 +Presolve (ms / nodes) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t3_mkp_200` | $200 \times 8$ | **0.43x** | $18.74 \pm 10.18$ | $40.37 \pm 19.09$ | 17.39 (250 / 2447) | 49.31 (154 / 1145) | 81.71 (1135 / 16) | 163.35 (5000, `NodeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **0.36x** | $5.98 \pm 6.07$ | $12.45 \pm 9.09$ | 4.00 (1 / 19) | 8.72 (0 / 8) | 11.68 (2 / 4) | 4.66 (118) |
| `t2_binpack_44` | $44 \times 14$ | **0.18x** | $10.92 \pm 32.42$ | $6.62 \pm 2.17$ | 0.52 (2 / 43) | 6.12 (0 / 8) | 0.98 (5 / 8) | 8.85 (772) |
| `t3_cflp_300` | $300 \times 34$ | **0.12x** | $7.54 \pm 5.18$ | $55.26 \pm 11.92$ | 5.78 (1 / 105) | 49.93 (18 / 483) | 16.93 (44 / 34) | 8.50 (116) |
| `t3_lotsizing_504` | $504 \times 336$ | **0.63x** | $76.90 \pm 75.12$ | $93.43 \pm 57.28$ | 61.76 (2 / 1061) | 70.69 (10 / 377) | 78.12 (376 / 206) | 250.02 (3810, `TimeLimit`) |
| `t3_sched_1000` | $1000 \times 100$ | **0.12x** | $8.22 \pm 4.35$ | $61.34 \pm 4.94$ | 6.60 (31 / 474) | 59.74 (0 / 76) | 19.31 (87 / 53) | 250.03 (2540, `TimeLimit`) |
<!-- END GENERATED:NON_MAD_CBC_GAP_TABLE -->

---

## Verification Scope and Architectural History

The current codebase has passed full pure-Go, `GOEXPERIMENT=simd`, `-tags cbc`, race-detector, linter, differential-test, and `GOOS=linux GOARCH=arm64` / `GOAMD64=v3`/`v4` cross-compilation gates on the local AVX2 host (`i9-13900K`). Physical AVX-512 and `linux/arm64` target runtime re-verification remain pending target hardware access.

### Architectural Context and Rejected Designs

1. **LU Updates and Row-Singleton Stability (`lu.go`)**: The factorizer maintains elbow-room $U$ arenas (`uRowStart`/`uColStart`) paired with rank-free in-place mutated-$U$ Forrest-Tomlin updates (`PushEta`/`PushEtaSparse`) up to `maxEtaUpdates = 56`, avoiding extra branch checks during `FTRAN` and `BTRAN`. Two alternatives were tested and rejected earlier: (a) a lazy-rank Forrest-Tomlin prototype with `maxEtaUpdates = 128` regressed `miplib_p0201` and inflated `netlib_brandy` pivots from `uRank` branch mispredictions; (b) removing the `luMarkowitzTol * colMax` check on Stage 2 row singletons caused unscaled `1000x` multipliers in $L$ and numerical stalling on `t3_mad_g4_i1000`.
2. **Root LP Crash Basis and Dense Coupling (`simplex.go`)**: Dual Phase I (`installDualPhase1Bounds`) keeps the public Netlib LPs inside sparse dual simplex without falling back to primal simplex, and [`crashEqualityBasis`](simplex.go) accelerates `t3_mad_g10_i100` unpresolved Root LP setup vs. slack bases. Crashing columns incident to dense coupling rows (`> 128` nonzeros) was rejected because displacing 1-NNZ artificial slacks on `t3_mad_g4_i1000` increased $U$ fill-in and solve latency.
3. **Cut Separation, Symmetry, and Zero-Cut Sub-MIP Gating (`cuts.go`, `presolve.go`, `bab.go`, `heuristics.go`)**: Root cut rounds enforce GMI/MIR density caps (`maxCutNNZ`), exit after round 1 on zero bound gain, and purge inactive slack cuts both before branching and periodically in-tree via [`PurgeSlackCuts`](simplex.go). On zero-cut roots, a blanket `< 48`-integer sub-MIP ban helped median latency on small seeds at the cost of mean and tail regressions; requiring 50% fixed integers (RINS criterion) improved both median and tail behavior across all trials.
4. **Architectural Summary**:
   - **Numerical and Bridge Hardening**: Multi-scale three-bucket Shewchuk summation (`wideDotAcc`, `assignWideAcc`), compensated residual verification and Farkas exact-dot certificates (`BTRANExact`), strict `lb > ub` validation across presolve and simplex, and CBC tiny-coefficient, quotient-bound, scaling, and drift guards (`cbc_cgo.go`, `cbc_bridge.cpp`).
   - **Memory Pooling & Serialization**: Pointer-free inline `wideDotAcc`, flat LU nucleus arenas with intrusive `int32` Markowitz buckets, copy-on-write `Presolve`, lazy CBC bridge slices, and direct streaming `PuLP` JSON ingestion (`format/pulp_json_fast.go`).
   - **Multi-Tier SIMD & Search**: Runtime AVX-512/AVX2/Scalar dispatch (`internal/simd/`), in-register DSE leaving-row scan (`SelectDualLeaveRow64With`), batched dual ratio screening (`ScanDualRatioCand64With`), multi-row MIR aggregation, globally valid in-tree cuts (`generateInTreeCutRound`), dynamic node orbital branching, strong-branching bound-cutoff domain reduction, post-ARR Volgenant sparse-core LSAP pricing, and optional parallel Branch-and-Cut via `WithWorkers`.

---

## Verification, Benchmarking, and Documentation Sync Commands

Run all commands from the `goblin/` directory:

```bash
# 0. Build the vendored COIN-OR CBC/CLP C++ stack with -DNDEBUG into third_party/Cbc/dist (once per checkout)
./scripts/build_cbc.sh

# 1. Unit, regression, 293-trial CBC parity, 21-instance public suite, and documentation gates
go test ./...
GOEXPERIMENT=simd go test ./...
go test -tags=cbc ./...

# 2. Regenerate the deduplicated 31-family PuLP JSON and multi-trial JSONL corpus in testdata/corpus/
go run ./cmd/goblin-bench -generate-corpus

# 3. Commit code changes first so every telemetry file records clean git provenance.
git status --short   # must be empty for git_dirty=false provenance

# 4. Warm-state Python PuLP + COIN-OR CBC (c1c171ec) baseline, pinned to one performance core
CBC_BIN=./third_party/Cbc/dist/bin/cbc taskset -c 2 python3 bench/python_baseline.py --corpus-dir testdata/corpus --out-json bench/results/python_baseline.json --workers 1

# 5. Warm-state goblin benchmarks across separate Default, GOEXPERIMENT=simd, and C++ CBC builds, pinned to CPU 2
go build -o /tmp/goblin-bench-default ./cmd/goblin-bench
GOEXPERIMENT=simd go build -o /tmp/goblin-bench-simd ./cmd/goblin-bench
go build -tags=cbc -o /tmp/goblin-bench-cbc ./cmd/goblin-bench
taskset -c 2 /tmp/goblin-bench-default -backend go -workers 1 -out bench/results/goblin_scalar.json
taskset -c 2 /tmp/goblin-bench-simd -backend go -workers 1 -out bench/results/goblin_simd.json
taskset -c 2 /tmp/goblin-bench-cbc -backend cbc -workers 1 -out bench/results/goblin_cbc.json

# 6. Warm-state SciPy and goblin/assign LSAP benchmarks across separate Default & SIMD builds, pinned to CPU 2
taskset -c 2 python3 bench/scipy_assign_baseline.py --corpus-dir testdata/assign_corpus --out-json bench/results/scipy_assign_baseline.json
taskset -c 2 /tmp/goblin-bench-default -assign -out bench/results/goblin_assign_scalar.json
taskset -c 2 /tmp/goblin-bench-simd -assign -out bench/results/goblin_assign_simd.json
rm -f /tmp/goblin-bench-default /tmp/goblin-bench-simd /tmp/goblin-bench-cbc

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
