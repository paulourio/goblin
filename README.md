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
- **Unified telemetry source**: fingerprint `81fe97d73f995897` (commit `57a94d9275d4806331d57a52231a5b8f89d65947`), `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.02 ms median on a 1-variable MILP and 5.74 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

Read the synthetic corpus ratios with three methodological notes. First, Tier 1 micro-models (`N <= 26`) are dominated by Python's CBC subprocess spawn floor reported in the provenance block above. Second, the planted-gap MAD family has a zero root LP gap by construction and closes at the root without branching. Consequently, the bold row (`Tier 2+3 non-planted MILPs`) isolates solver-dominated MILPs across paired randomized instances, alongside total subset wall times and full per-instance `[Min-Max]` ranges.

### 1. Aggregate Synthetic Workload Summary (Paired Per-Instance Geometric Mean and Wall-Clock Sums)

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `Py / Go` GeoMean [Clustered 95% CI] | Pooled Trial `Py / Go` | Equal-Family `Go / CBC` | `Go` Trial (Med) Sum (ms) | `CBC` Trial (Med) Sum (ms) | `Py` Trial (Med) Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **362.95x** [164.25x-802.01x] | 362.95x | 0.04x | 8.4 (0.7) | 191.8 (19.2) | 979.5 (93.0) | 533.76x [37.08x-2309.73x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **18.45x** [10.75x-31.68x] | 18.45x | 0.15x | 738.8 (32.5) | 1951.4 (170.9) | 4103.2 (398.4) | 24.18x [0.11x-141.11x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **9.66x** [4.46x-20.94x] | 9.66x | 0.21x | 795.7 (158.3) | 2854.3 (551.7) | 6048.1 (1040.6) | 9.86x [1.08x-77.00x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **11.92x** [6.84x-20.78x] | 12.99x | 0.20x | 1225.7 (143.4) | 3086.1 (390.5) | 6528.7 (956.4) | 15.26x [0.11x-141.11x] |
| Tier 3 non-MAD MILPs | 4 (20) | **7.49x** [1.13x-49.52x] | 7.49x | 0.29x | 494.9 (111.6) | 1225.6 (227.4) | 2737.4 (585.9) | 5.20x [2.18x-77.00x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **25.47x** [11.67x-55.59x] | 33.31x | 0.10x | 309.7 (47.5) | 1743.9 (334.6) | 3716.2 (491.3) | 38.76x [1.08x-119.22x] |
| Random-weight MAD (negative controls) | 3 (33) | **16.10x** [0.83x-310.97x] | 16.10x | 0.14x | 320.6 (18.2) | 1196.8 (114.6) | 2177.7 (241.5) | 17.47x [1.42x-94.27x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **48.10x** [19.99x-115.73x] | 58.45x | 0.11x | 1233.1 (144.1) | 3253.5 (407.2) | 7414.6 (1040.7) | 45.22x [0.11x-2309.73x] |
| All 31 synthetic corpus families | 31 (293) | **40.82x** [20.95x-79.53x] | 51.69x | 0.11x | 1542.9 (191.5) | 4997.5 (741.8) | 11130.9 (1532.0) | 42.55x [0.11x-2309.73x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2. Synthetic MAD Group-Item Allocation Family (`G` Groups $\times$ `I` Items, $T = 6$ Periods)

The synthetic `MAD` group-item allocation family ([`BuildMADFromWeights`](bench/corpus.go)) assigns $I$ items across $G$ groups over $T = 6$ periods to minimize total L1 distance from each group's central level $m_g$ plus the roundness penalty $r_g = |m_g - 100 n_g|$ relative to an integer multiple $n_g \in \mathbb{Z}_{\ge 1}$. In the planted-gap generator, items in $S = \{0, \dots, I-G-1\}$ carry orthogonal curve signatures cancelled by the $G$ designated items $D = \{I-G, \dots, I-1\}$ with roundness-aligned period sums, yielding a zero root LP gap that root heuristics close without branching; unstructured random-weight negative controls (`BuildMADRandom`: `t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`) are included directly in the corpus and detailed in [`docs/benchmarks.md`](docs/benchmarks.md).

<!-- BEGIN GENERATED:MAD_PIPELINE_TABLE -->
| Problem ID | $G \times I$ | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Trial 0 Obj | Parity |
| :--- | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **95.3x** [64.6x-119.2x] | 0.09 | 8.71 | 0.05 | 8.51 | 0.033 | 0.252 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **62.8x** [37.1x-94.3x] | 0.49 | 31.87 | 0.46 | 31.67 | 0.027 | 0.257 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **56.9x** [53.4x-63.9x] | 0.15 | 8.46 | 0.11 | 8.20 | 0.036 | 0.332 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **48.3x** [27.9x-60.5x] | 0.18 | 9.10 | 0.12 | 8.72 | 0.051 | 0.488 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **28.1x** [23.2x-35.0x] | 0.38 | 10.33 | 0.27 | 9.77 | 0.099 | 0.733 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **9.7x** [2.3x-28.3x] | 2.68 | 45.47 | 2.64 | 45.22 | 0.038 | 0.341 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **6.8x** [1.4x-26.2x] | 15.07 | 164.12 | 15.01 | 163.71 | 0.060 | 0.536 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **26.4x** [17.5x-30.8x] | 0.61 | 16.55 | 0.45 | 15.51 | 0.143 | 1.269 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **15.7x** [9.3x-20.6x] | 1.00 | 16.83 | 0.77 | 15.17 | 0.244 | 2.170 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **5.8x** [1.1x-14.6x] | 8.08 | 58.85 | 6.97 | 51.09 | 1.286 | 10.632 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **9.9x** [4.5x-24.7x] | 36.99 | 362.47 | 32.32 | 329.87 | 4.584 | 42.721 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

Cross-instance total latency dispersion across the randomized instances in each family (mean $\pm$ sample SD and two-sided 95% Student-$t$ interval after warmup):

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.09 \pm 0.02$ ($\pm 0.01$) | $8.51 \pm 0.33$ ($\pm 0.22$) | $0.09 \pm 0.01$ ($\pm 0.01$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.50 \pm 0.23$ ($\pm 0.16$) | $28.77 \pm 9.21$ ($\pm 6.18$) | $0.51 \pm 0.24$ ($\pm 0.16$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.15 \pm 0.01$ ($\pm 0.00$) | $8.64 \pm 0.47$ ($\pm 0.32$) | $0.15 \pm 0.01$ ($\pm 0.01$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.19 \pm 0.05$ ($\pm 0.03$) | $9.11 \pm 0.06$ ($\pm 0.04$) | $0.18 \pm 0.03$ ($\pm 0.02$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.38 \pm 0.04$ ($\pm 0.03$) | $10.61 \pm 0.60$ ($\pm 0.40$) | $0.37 \pm 0.04$ ($\pm 0.03$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $4.55 \pm 4.67$ ($\pm 3.14$) | $33.78 \pm 16.76$ ($\pm 11.26$) | $4.58 \pm 4.66$ ($\pm 3.13$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $24.10 \pm 21.97$ ($\pm 14.76$) | $135.42 \pm 62.60$ ($\pm 42.05$) | $24.12 \pm 21.93$ ($\pm 14.73$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.62 \pm 0.09$ ($\pm 0.11$) | $16.33 \pm 2.21$ ($\pm 2.74$) | $0.61 \pm 0.10$ ($\pm 0.12$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.12 \pm 0.40$ ($\pm 0.49$) | $16.87 \pm 0.08$ ($\pm 0.09$) | $1.15 \pm 0.37$ ($\pm 0.46$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $16.46 \pm 20.99$ ($\pm 26.06$) | $60.77 \pm 3.52$ ($\pm 4.37$) | $16.14 \pm 20.08$ ($\pm 24.93$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $41.95 \pm 13.73$ ($\pm 17.05$) | $568.18 \pm 601.51$ ($\pm 746.75$) | $39.97 \pm 14.83$ ($\pm 18.42$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### 3. Full 31-Family Synthetic Benchmark Corpus (`JSON -> Solution`)

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | **737.1x** [431.1x-982.8x] | 0.01 | 5.85 | 0.00 | 5.80 | 0.003 | 0.061 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **1010.4x** [703.3x-1127.9x] | 0.01 | 5.84 | 0.00 | 5.79 | 0.003 | 0.061 | `OK` |
| `t1_std_mip` | 3 | 3 | **436.5x** [245.0x-705.3x] | 0.01 | 6.85 | 0.01 | 6.80 | 0.006 | 0.062 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **590.7x** [234.6x-728.0x] | 0.01 | 6.86 | 0.01 | 6.81 | 0.005 | 0.063 | `OK` |
| `t1_whiskas` | 6 | 5 | **530.9x** [407.0x-612.1x] | 0.01 | 5.90 | 0.00 | 5.83 | 0.006 | 0.087 | `OK` |
| `t1_furniture` | 2 | 3 | **319.6x** [81.8x-977.7x] | 0.02 | 7.09 | 0.01 | 7.04 | 0.003 | 0.059 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **107.5x** [42.0x-289.8x] | 0.08 | 8.27 | 0.07 | 8.20 | 0.009 | 0.097 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **1893.0x** [1110.0x-2309.7x] | 0.00 | 5.76 | 0.00 | 5.72 | 0.002 | 0.050 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **95.3x** [64.6x-119.2x] | 0.09 | 8.71 | 0.05 | 8.51 | 0.033 | 0.252 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **62.8x** [37.1x-94.3x] | 0.49 | 31.87 | 0.46 | 31.67 | 0.027 | 0.257 | `OK` |
| `t2_mkp_30` | 30 | 6 | **7.6x** [4.1x-11.8x] | 2.52 | 26.03 | 2.49 | 25.90 | 0.021 | 0.185 | `OK` |
| `t2_mkp_50` | 50 | 8 | **3.9x** [1.6x-28.4x] | 6.36 | 15.48 | 6.33 | 15.26 | 0.034 | 0.299 | `OK` |
| `t2_cflp_36` | 36 | 11 | **52.0x** [23.4x-141.1x] | 0.20 | 8.31 | 0.15 | 8.17 | 0.019 | 0.191 | `OK` |
| `t2_cflp_49` | 49 | 13 | **35.7x** [21.6x-103.9x] | 0.32 | 9.20 | 0.29 | 9.01 | 0.025 | 0.234 | `OK` |
| `t2_gap_45` | 45 | 13 | **18.4x** [2.9x-69.6x] | 0.94 | 13.36 | 0.92 | 13.18 | 0.029 | 0.223 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **12.9x** [5.5x-30.0x] | 2.97 | 65.86 | 2.95 | 65.66 | 0.028 | 0.268 | `OK` |
| `t2_binpack_44` | 44 | 14 | **10.5x** [0.1x-39.5x] | 0.55 | 12.59 | 0.50 | 12.41 | 0.032 | 0.220 | `OK` |
| `t2_setcover_50` | 50 | 20 | **41.4x** [5.8x-105.2x] | 0.19 | 10.07 | 0.13 | 9.80 | 0.043 | 0.354 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **56.9x** [53.4x-63.9x] | 0.15 | 8.46 | 0.11 | 8.20 | 0.036 | 0.332 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **48.3x** [27.9x-60.5x] | 0.18 | 9.10 | 0.12 | 8.72 | 0.051 | 0.488 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **28.1x** [23.2x-35.0x] | 0.38 | 10.33 | 0.27 | 9.77 | 0.099 | 0.733 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.7x** [2.3x-28.3x] | 2.68 | 45.47 | 2.64 | 45.22 | 0.038 | 0.341 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **6.8x** [1.4x-26.2x] | 15.07 | 164.12 | 15.01 | 163.71 | 0.060 | 0.536 | `OK` |
| `t3_mkp_200` | 200 | 8 | **2.7x** [2.2x-3.3x] | 36.94 | 112.10 | 36.86 | 111.51 | 0.089 | 0.775 | `OK` |
| `t3_cflp_300` | 300 | 34 | **34.3x** [7.8x-77.0x] | 5.96 | 243.03 | 5.82 | 242.13 | 0.121 | 1.181 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **3.2x** [2.3x-5.1x] | 61.67 | 153.33 | 61.44 | 151.53 | 0.218 | 2.243 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **10.7x** [5.3x-15.3x] | 7.05 | 77.45 | 6.57 | 74.62 | 0.346 | 3.403 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **26.4x** [17.5x-30.8x] | 0.61 | 16.55 | 0.45 | 15.51 | 0.143 | 1.269 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **15.7x** [9.3x-20.6x] | 1.00 | 16.83 | 0.77 | 15.17 | 0.244 | 2.170 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.8x** [1.1x-14.6x] | 8.08 | 58.85 | 6.97 | 51.09 | 1.286 | 10.632 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **9.9x** [4.5x-24.7x] | 36.99 | 362.47 | 32.32 | 329.87 | 4.584 | 42.721 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

### 4. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus ([`bench/assign_corpus.go`](bench/assign_corpus.go), 200 randomized instances across square, rectangular wide/tall, degenerate integer-tie, Euclidean geometric, rank-1 outer-product, sparse `+Inf`, and maximization families) compares [`assign.LinearSumFlat`](assign/assign.go) against SciPy's C++ `rectangular_lsap` extension (`scipy.optimize.linear_sum_assignment`) in warm state on bit-identical `float64` cost matrices:

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `SciPy Solve / goblin` GeoMean [Clustered 95% CI] | Pooled Trial `SciPy Solve / goblin` | Equal-Family `SciPy Total / goblin` | `goblin` Solve Trial (Med) Sum (ms) | SciPy Solve Trial (Med) Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.94x** [1.72x-2.18x] | 1.94x | 5.95x | 0.174 (0.0140) | 0.317 (0.0286) | 2.01x [0.59x-5.76x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **3.39x** [2.09x-5.48x] | 3.39x | 3.29x | 8.254 (0.713) | 32.506 (2.930) | 3.04x [1.18x-19.93x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **3.21x** [2.16x-4.76x] | 3.21x | 3.80x | 196.472 (38.600) | 905.234 (180.241) | 3.07x [1.55x-6.37x] |
| Square families ($m = n$) | 16 (146) | **3.09x** [2.28x-4.18x] | 3.04x | 4.18x | 201.096 (38.654) | 927.270 (181.260) | 2.75x [0.59x-19.93x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **2.33x** [1.78x-3.04x] | 2.20x | 3.72x | 3.804 (0.672) | 10.788 (1.940) | 2.37x [1.18x-3.94x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.60x** [1.27x-5.32x] | 2.75x | 3.19x | 33.230 (5.889) | 64.481 (10.902) | 2.07x [0.59x-19.93x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.86x** [2.28x-3.59x] | 2.79x | 4.05x | 204.900 (39.326) | 938.058 (183.200) | 2.54x [0.59x-19.93x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **2.31x** | 0.0004 | 0.0009 | 0.0005 | 0.0117 | $0.0004 \pm 0.0001$ | $0.0010 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.94x** | 0.0013 | 0.0024 | 0.0014 | 0.0103 | $0.0013 \pm 0.0007$ | $0.0024 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.99x** | 0.0024 | 0.0062 | 0.0036 | 0.0153 | $0.0036 \pm 0.0027$ | $0.0065 \pm 0.0027$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.70x** | 0.0023 | 0.0039 | 0.0030 | 0.0141 | $0.0023 \pm 0.0006$ | $0.0039 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.73x** | 0.0025 | 0.0044 | 0.0032 | 0.0149 | $0.0026 \pm 0.0007$ | $0.0044 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **2.01x** | 0.0051 | 0.0109 | 0.0078 | 0.0321 | $0.0055 \pm 0.0020$ | $0.0106 \pm 0.0020$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **3.07x** | 0.0231 | 0.0700 | 0.0311 | 0.0958 | $0.0229 \pm 0.0059$ | $0.0687 \pm 0.0103$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.75x** | 0.0479 | 0.175 | 0.0634 | 0.224 | $0.0466 \pm 0.0042$ | $0.176 \pm 0.0262$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.15x** | 0.0423 | 0.0935 | 0.0648 | 0.141 | $0.0461 \pm 0.0141$ | $0.0961 \pm 0.0137$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.71x** | 0.128 | 0.217 | 0.135 | 0.254 | $0.139 \pm 0.0443$ | $0.232 \pm 0.0464$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **14.87x** | 0.0495 | 0.683 | 0.0680 | 0.757 | $0.0467 \pm 0.0074$ | $0.686 \pm 0.0190$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **2.53x** | 0.0275 | 0.0692 | 0.0514 | 0.143 | $0.0297 \pm 0.0118$ | $0.0726 \pm 0.0123$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.39x** | 0.0313 | 0.0792 | 0.0555 | 0.153 | $0.0346 \pm 0.0107$ | $0.0804 \pm 0.0091$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.72x** | 0.0758 | 0.279 | 0.102 | 0.358 | $0.0766 \pm 0.0120$ | $0.284 \pm 0.0333$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **4.13x** | 0.287 | 1.264 | 0.421 | 1.653 | $0.308 \pm 0.0600$ | $1.259 \pm 0.118$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.93x** | 1.145 | 5.195 | 1.815 | 7.545 | $1.396 \pm 0.435$ | $5.311 \pm 0.352$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **2.12x** | 0.717 | 1.480 | 1.295 | 3.891 | $0.699 \pm 0.0394$ | $1.481 \pm 0.0519$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.72x** | 4.945 | 8.411 | 5.126 | 9.151 | $5.418 \pm 1.981$ | $9.145 \pm 2.660$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **3.19x** | 0.269 | 0.827 | 0.638 | 3.496 | $0.263 \pm 0.0202$ | $0.839 \pm 0.0603$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.79x** | 0.340 | 0.956 | 0.728 | 3.628 | $0.345 \pm 0.0137$ | $0.964 \pm 0.0333$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **5.17x** | 5.486 | 27.668 | 8.201 | 34.881 | $5.453 \pm 0.309$ | $28.213 \pm 2.282$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **5.28x** | 25.699 | 135.704 | 32.419 | 178.710 | $25.721 \pm 3.652$ | $135.094 \pm 9.387$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

---

## Pure-Go vs. C++ CBC Performance Gaps and MAD Scaling Analysis

Comparing isolated unpresolved `Root LP` timings, paired trial ratios, and solver-core checkpoints (`CP5 +Presolve`, `CP6 +Cuts`, `CP7 Full`) between `BackendGo` and in-process `BackendCBC` (`c1c171ec`) highlights four structural trade-offs on large degenerate LPs and heavy-tail branch-and-cut models.

### 1. MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families (`N = G(I + T + 3)` variables, `M = G(2T + 3) + I` constraints) separate solver performance into three distinct scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Paired `Go / CBC` | Go Total Med (ms) | CBC Total Med (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.04x** | 0.09 | 2.43 | 0.57x | 0.05 (1 / 19) | 2.39 (0 / 0) | 0.078 (28) | 0.136 (19) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.05x** | 0.49 | 11.18 | 0.61x | 0.46 (2 / 58) | 11.15 (6 / 83) | 0.087 (30) | 0.144 (18) |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.07x** | 0.15 | 1.97 | 0.59x | 0.11 (1 / 26) | 1.93 (0 / 0) | 0.092 (33) | 0.157 (21) |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.07x** | 0.18 | 2.55 | 0.87x | 0.12 (1 / 40) | 2.49 (0 / 0) | 0.239 (58) | 0.273 (40) |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.11x** | 0.38 | 3.36 | 0.88x | 0.27 (1 / 51) | 3.18 (0 / 0) | 0.352 (80) | 0.399 (63) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.24x** | 2.68 | 16.37 | 0.95x | 2.64 (21 / 324) | 16.33 (12 / 216) | 0.144 (38) | 0.152 (24) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.26x** | 15.07 | 87.09 | 0.97x | 15.01 (220 / 2314) | 87.03 (198 / 1300) | 0.270 (65) | 0.278 (54) |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.08x** | 0.61 | 7.94 | 1.01x | 0.45 (1 / 75) | 7.80 (0 / 0) | 0.635 (91) | 0.629 (94) |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.15x** | 1.00 | 7.33 | 1.43x | 0.77 (1 / 98) | 7.08 (0 / 0) | 1.518 (244) | 1.062 (149) |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **0.29x** | 8.08 | 34.99 | 0.77x | 6.97 (2 / 364) | 33.92 (0 / 0) | 2.795 (224) | 3.636 (319) |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **0.17x** | 36.99 | 273.98 | 1.65x | 32.32 (1 / 2439) | 268.91 (0 / 0) | 57.713 (1905) | 34.930 (3128) |
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
| `t3_mkp_200` | $200 \times 8$ | **0.86x** | $34.26 \pm 16.07$ | $39.75 \pm 18.80$ | 36.86 (448 / 3584) | 48.62 (154 / 1145) | 82.85 (1144 / 16) | 178.80 (5000, `NodeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **0.52x** | $9.55 \pm 11.62$ | $12.35 \pm 9.10$ | 6.33 (1 / 19) | 9.08 (0 / 8) | 15.83 (2 / 4) | 7.08 (132) |
| `t2_binpack_44` | $44 \times 14$ | **0.19x** | $17.04 \pm 49.58$ | $6.70 \pm 2.20$ | 0.50 (2 / 43) | 6.13 (0 / 8) | 0.95 (5 / 8) | 22.07 (1424) |
| `t3_cflp_300` | $300 \times 34$ | **0.12x** | $7.67 \pm 5.62$ | $54.09 \pm 11.60$ | 5.82 (1 / 105) | 49.06 (18 / 483) | 17.09 (46 / 34) | 9.16 (134) |
| `t3_lotsizing_504` | $504 \times 336$ | **0.52x** | $49.09 \pm 25.43$ | $91.20 \pm 56.27$ | 61.44 (2 / 1054) | 70.35 (10 / 377) | 80.97 (376 / 206) | 250.02 (3412, `TimeLimit`) |
| `t3_sched_1000` | $1000 \times 100$ | **0.12x** | $7.96 \pm 3.96$ | $60.08 \pm 4.84$ | 6.57 (32 / 435) | 58.65 (0 / 76) | 20.96 (87 / 53) | 250.08 (2222, `TimeLimit`) |
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
