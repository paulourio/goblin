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
3. **Optional Multi-Tier SIMD Kernels**: With `GOEXPERIMENT=simd` on `amd64` (gated at runtime on CPU `AVX-512`, `AVX2`, and `FMA` support), [`internal/simd/kernels_amd64.go`](internal/simd/kernels_amd64.go) and [`internal/simd/kernels_avx512_amd64.go`](internal/simd/kernels_avx512_amd64.go) dispatch 512-bit AVX-512 or 256-bit AVX2 vector loops (`Dot64`, `Axpy64`, `SelectDualLeaveRow64With`, `ScanDualRatioCand64With`, `MinReducedSlice64`, `InitReducedRow64`, `Min2ReducedRow64`, `AllAbsLE64`, and `RelaxRow64`) with `StoreArray` + `VZEROUPPER` cleanup and IEEE-754 `NaN` propagation parity across scalar and vector paths, and can be toggled at runtime via `WithoutSIMD()` or `WithSIMD(false)`. See the paired comparison in [`docs/benchmarks.md`](docs/benchmarks.md#5-default-build-vs-goexperimentsimd).
4. **Verified Parity on Synthetic and Public Suites**: [`solver_test.go`](solver_test.go) re-solves all 293 distinct randomized trial instances of the 31 synthetic benchmark families (`testdata/corpus/trials/*.jsonl`), compares status and objective with `PuLP + COIN-OR CBC` (`c1c171ec`) and `BackendCBC`, and verifies primal and row feasibility on every result. [`public_benchmarks_test.go`](public_benchmarks_test.go) verifies optimal status, objective parity, and primal feasibility across 20 held-out public Netlib LP and MIPLIB 3.0 MILP instances (`testdata/public/`) plus a valid dual lower bound under a 25-node budget (`go_max_nodes: 25`) on `coinor_capplan1`, while [`regression_test.go`](regression_test.go) tests big-M fixed-charge models against enumerated ground truth and Farkas-certified LP scaling edge cases. `TimeLimit`, `MaxNodes`, and `MaxIter` return `StatusTimeLimit`, `StatusNodeLimit`, or `StatusIterationLimit` with a valid global bound once the root LP is solved; a node that cannot be evaluated is never discarded silently.

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
- **Unified telemetry source**: fingerprint `81fe97d73f995897` (working tree on top of commit `2848940e3a824499dd909f9d87664a23101c1a95`), `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.05 ms median on a 1-variable MILP and 5.74 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

Read the synthetic corpus ratios with three methodological notes. First, Tier 1 micro-models (`N <= 26`) are dominated by Python's CBC subprocess spawn floor reported in the provenance block above. Second, the planted-gap MAD family has a zero root LP gap by construction and closes at the root without branching. Consequently, the bold row (`Tier 2+3 non-planted MILPs`) isolates solver-dominated MILPs across paired randomized instances, alongside total subset wall times and full per-instance `[Min-Max]` ranges.

### 1. Aggregate Synthetic Workload Summary (Paired Per-Instance Geometric Mean and Wall-Clock Sums)

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `Py / Go` GeoMean [Clustered 95% CI] | Pooled Trial `Py / Go` | Equal-Family `Go / CBC` | `Go` Trial (Med) Sum (ms) | `CBC` Trial (Med) Sum (ms) | `Py` Trial (Med) Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **363.28x** [175.82x-750.58x] | 363.28x | 0.04x | 8.3 (0.7) | 189.1 (18.8) | 981.0 (92.8) | 582.64x [36.41x-1156.20x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **18.98x** [10.94x-32.94x] | 18.98x | 0.15x | 739.6 (32.3) | 1927.6 (170.3) | 4098.1 (398.1) | 25.75x [0.11x-145.61x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **9.68x** [4.43x-21.16x] | 9.68x | 0.21x | 800.3 (160.4) | 2853.9 (551.4) | 6044.5 (1040.3) | 9.69x [1.10x-77.34x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **12.18x** [6.90x-21.52x] | 13.31x | 0.20x | 1229.1 (146.2) | 3065.9 (390.3) | 6518.5 (954.6) | 15.26x [0.11x-145.61x] |
| Tier 3 non-MAD MILPs | 4 (20) | **7.54x** [1.11x-51.26x] | 7.54x | 0.28x | 497.2 (114.6) | 1223.8 (227.2) | 2734.9 (584.9) | 5.19x [2.09x-77.34x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **26.16x** [11.61x-58.95x] | 34.58x | 0.10x | 311.7 (46.5) | 1739.7 (333.9) | 3717.8 (492.6) | 35.67x [1.10x-146.46x] |
| Random-weight MAD (negative controls) | 3 (33) | **16.09x** [0.83x-310.20x] | 16.09x | 0.14x | 321.2 (18.2) | 1200.9 (116.5) | 2173.0 (240.8) | 17.64x [1.40x-92.30x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **48.47x** [20.36x-115.37x] | 58.91x | 0.11x | 1236.5 (146.8) | 3230.8 (406.6) | 7405.9 (1038.7) | 44.65x [0.11x-1156.20x] |
| All 31 synthetic corpus families | 31 (293) | **41.34x** [21.35x-80.04x] | 52.44x | 0.11x | 1548.1 (193.4) | 4970.5 (740.5) | 11123.7 (1531.2) | 41.83x [0.11x-1156.20x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2. Synthetic MAD Group-Item Allocation Family (`G` Groups $\times$ `I` Items, $T = 6$ Periods)

The synthetic `MAD` group-item allocation family ([`BuildMADFromWeights`](bench/corpus.go)) assigns $I$ items across $G$ groups over $T = 6$ periods to minimize total L1 distance from each group's central level $m_g$ plus the roundness penalty $r_g = |m_g - 100 n_g|$ relative to an integer multiple $n_g \in \mathbb{Z}_{\ge 1}$. In the planted-gap generator, items in $S = \{0, \dots, I-G-1\}$ carry orthogonal curve signatures cancelled by the $G$ designated items $D = \{I-G, \dots, I-1\}$ with roundness-aligned period sums, yielding a zero root LP gap that root heuristics close without branching; unstructured random-weight negative controls (`BuildMADRandom`: `t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`) are included directly in the corpus and detailed in [`docs/benchmarks.md`](docs/benchmarks.md).

<!-- BEGIN GENERATED:MAD_PIPELINE_TABLE -->
| Problem ID | $G \times I$ | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Trial 0 Obj | Parity |
| :--- | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **109.1x** [79.5x-146.5x] | 0.08 | 8.77 | 0.04 | 8.55 | 0.029 | 0.250 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **62.7x** [36.4x-92.3x] | 0.49 | 31.69 | 0.46 | 31.49 | 0.027 | 0.259 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **62.6x** [56.8x-72.5x] | 0.14 | 8.84 | 0.11 | 8.57 | 0.033 | 0.336 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **45.3x** [33.5x-58.0x] | 0.21 | 9.15 | 0.14 | 8.77 | 0.052 | 0.491 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **29.8x** [24.2x-35.4x] | 0.34 | 10.36 | 0.26 | 9.79 | 0.078 | 0.749 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **9.7x** [2.3x-28.7x] | 2.65 | 45.68 | 2.62 | 45.42 | 0.037 | 0.343 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **6.9x** [1.4x-26.4x] | 15.04 | 163.45 | 14.96 | 163.02 | 0.059 | 0.540 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **26.6x** [17.0x-34.0x] | 0.53 | 16.74 | 0.40 | 15.70 | 0.139 | 1.289 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **15.7x** [8.9x-20.7x] | 0.99 | 16.96 | 0.76 | 15.25 | 0.249 | 2.208 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **5.8x** [1.1x-13.9x] | 8.33 | 58.65 | 7.17 | 50.88 | 1.368 | 10.777 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **9.9x** [4.4x-23.3x] | 35.92 | 363.11 | 28.68 | 330.22 | 5.300 | 43.026 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

Cross-instance total latency dispersion across the randomized instances in each family (mean $\pm$ sample SD and two-sided 95% Student-$t$ interval after warmup):

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.08 \pm 0.02$ ($\pm 0.01$) | $8.51 \pm 0.37$ ($\pm 0.25$) | $0.08 \pm 0.01$ ($\pm 0.01$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.50 \pm 0.24$ ($\pm 0.16$) | $28.89 \pm 9.28$ ($\pm 6.24$) | $0.50 \pm 0.24$ ($\pm 0.16$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.14 \pm 0.01$ ($\pm 0.00$) | $8.81 \pm 0.46$ ($\pm 0.31$) | $0.14 \pm 0.02$ ($\pm 0.01$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.21 \pm 0.04$ ($\pm 0.03$) | $9.15 \pm 0.07$ ($\pm 0.05$) | $0.21 \pm 0.07$ ($\pm 0.04$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.36 \pm 0.03$ ($\pm 0.02$) | $10.64 \pm 0.64$ ($\pm 0.43$) | $0.37 \pm 0.07$ ($\pm 0.05$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $4.58 \pm 4.72$ ($\pm 3.17$) | $33.72 \pm 16.74$ ($\pm 11.24$) | $4.52 \pm 4.64$ ($\pm 3.11$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $24.12 \pm 22.11$ ($\pm 14.86$) | $134.94 \pm 62.38$ ($\pm 41.90$) | $24.18 \pm 21.95$ ($\pm 14.74$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.61 \pm 0.12$ ($\pm 0.14$) | $16.27 \pm 2.27$ ($\pm 2.82$) | $0.58 \pm 0.09$ ($\pm 0.12$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.13 \pm 0.42$ ($\pm 0.52$) | $16.99 \pm 0.29$ ($\pm 0.35$) | $1.12 \pm 0.39$ ($\pm 0.48$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $16.36 \pm 20.45$ ($\pm 25.39$) | $60.99 \pm 4.01$ ($\pm 4.98$) | $16.39 \pm 20.28$ ($\pm 25.17$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $42.50 \pm 15.55$ ($\pm 19.31$) | $567.68 \pm 600.90$ ($\pm 746.00$) | $38.81 \pm 16.04$ ($\pm 19.92$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### 3. Full 31-Family Synthetic Benchmark Corpus (`JSON -> Solution`)

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | **668.0x** [449.4x-1018.9x] | 0.01 | 5.86 | 0.00 | 5.82 | 0.005 | 0.061 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **967.5x** [697.2x-1073.9x] | 0.01 | 5.82 | 0.00 | 5.78 | 0.003 | 0.061 | `OK` |
| `t1_std_mip` | 3 | 3 | **590.1x** [267.9x-697.9x] | 0.01 | 6.81 | 0.00 | 6.76 | 0.006 | 0.061 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **918.7x** [789.5x-1027.6x] | 0.01 | 6.85 | 0.00 | 6.80 | 0.003 | 0.062 | `OK` |
| `t1_whiskas` | 6 | 5 | **541.1x** [451.3x-623.8x] | 0.01 | 5.92 | 0.00 | 5.85 | 0.006 | 0.087 | `OK` |
| `t1_furniture` | 2 | 3 | **320.3x** [78.7x-975.2x] | 0.02 | 7.04 | 0.01 | 7.00 | 0.003 | 0.057 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **111.9x** [42.5x-320.2x] | 0.08 | 8.31 | 0.07 | 8.23 | 0.008 | 0.099 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **860.8x** [595.2x-1156.2x] | 0.01 | 5.75 | 0.00 | 5.71 | 0.005 | 0.050 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **109.1x** [79.5x-146.5x] | 0.08 | 8.77 | 0.04 | 8.55 | 0.029 | 0.250 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **62.7x** [36.4x-92.3x] | 0.49 | 31.69 | 0.46 | 31.49 | 0.027 | 0.259 | `OK` |
| `t2_mkp_30` | 30 | 6 | **7.7x** [4.1x-11.9x] | 2.52 | 25.93 | 2.50 | 25.79 | 0.020 | 0.186 | `OK` |
| `t2_mkp_50` | 50 | 8 | **3.9x** [1.6x-32.5x] | 6.35 | 15.44 | 6.32 | 15.23 | 0.033 | 0.302 | `OK` |
| `t2_cflp_36` | 36 | 11 | **56.5x** [25.7x-145.6x] | 0.16 | 8.41 | 0.13 | 8.26 | 0.019 | 0.192 | `OK` |
| `t2_cflp_49` | 49 | 13 | **33.4x** [21.0x-95.3x] | 0.30 | 9.28 | 0.27 | 9.09 | 0.029 | 0.235 | `OK` |
| `t2_gap_45` | 45 | 13 | **19.3x** [2.8x-59.2x] | 0.91 | 13.05 | 0.88 | 12.87 | 0.023 | 0.223 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **12.8x** [5.4x-30.3x] | 2.99 | 65.99 | 2.96 | 65.78 | 0.029 | 0.270 | `OK` |
| `t2_binpack_44` | 44 | 14 | **10.9x** [0.1x-40.9x] | 0.50 | 12.47 | 0.48 | 12.30 | 0.024 | 0.218 | `OK` |
| `t2_setcover_50` | 50 | 20 | **47.9x** [6.2x-118.1x] | 0.16 | 10.05 | 0.13 | 9.78 | 0.037 | 0.355 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **62.6x** [56.8x-72.5x] | 0.14 | 8.84 | 0.11 | 8.57 | 0.033 | 0.336 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **45.3x** [33.5x-58.0x] | 0.21 | 9.15 | 0.14 | 8.77 | 0.052 | 0.491 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **29.8x** [24.2x-35.4x] | 0.34 | 10.36 | 0.26 | 9.79 | 0.078 | 0.749 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.7x** [2.3x-28.7x] | 2.65 | 45.68 | 2.62 | 45.42 | 0.037 | 0.343 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **6.9x** [1.4x-26.4x] | 15.04 | 163.45 | 14.96 | 163.02 | 0.059 | 0.540 | `OK` |
| `t3_mkp_200` | 200 | 8 | **2.7x** [2.1x-3.2x] | 37.82 | 112.16 | 37.73 | 111.57 | 0.091 | 0.778 | `OK` |
| `t3_cflp_300` | 300 | 34 | **35.4x** [7.9x-77.3x] | 5.77 | 242.23 | 5.65 | 241.34 | 0.111 | 1.174 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **3.2x** [2.3x-5.2x] | 63.71 | 153.18 | 63.50 | 151.37 | 0.234 | 2.239 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **10.8x** [5.1x-16.3x] | 7.31 | 77.29 | 6.80 | 74.44 | 0.358 | 3.414 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **26.6x** [17.0x-34.0x] | 0.53 | 16.74 | 0.40 | 15.70 | 0.139 | 1.289 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **15.7x** [8.9x-20.7x] | 0.99 | 16.96 | 0.76 | 15.25 | 0.249 | 2.208 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.8x** [1.1x-13.9x] | 8.33 | 58.65 | 7.17 | 50.88 | 1.368 | 10.777 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **9.9x** [4.4x-23.3x] | 35.92 | 363.11 | 28.68 | 330.22 | 5.300 | 43.026 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

### 4. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus ([`bench/assign_corpus.go`](bench/assign_corpus.go), 200 randomized instances across square, rectangular wide/tall, degenerate integer-tie, Euclidean geometric, rank-1 outer-product, sparse `+Inf`, and maximization families) compares [`assign.LinearSumFlat`](assign/assign.go) against SciPy's C++ `rectangular_lsap` extension (`scipy.optimize.linear_sum_assignment`) in warm state on bit-identical `float64` cost matrices:

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `SciPy Solve / goblin` GeoMean [Clustered 95% CI] | Pooled Trial `SciPy Solve / goblin` | Equal-Family `SciPy Total / goblin` | `goblin` Solve Trial (Med) Sum (ms) | SciPy Solve Trial (Med) Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **2.03x** [1.89x-2.17x] | 2.03x | 6.21x | 0.165 (0.0137) | 0.323 (0.0294) | 2.08x [1.10x-4.05x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **3.33x** [2.18x-5.08x] | 3.33x | 3.30x | 9.393 (0.741) | 32.971 (2.955) | 3.05x [0.60x-19.82x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **3.24x** [2.16x-4.88x] | 3.24x | 3.78x | 195.894 (38.595) | 898.005 (179.108) | 3.10x [1.56x-6.40x] |
| Square families ($m = n$) | 16 (146) | **3.08x** [2.33x-4.08x] | 3.03x | 4.26x | 201.661 (38.680) | 920.541 (180.163) | 2.76x [0.60x-19.82x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **2.41x** [1.93x-3.00x] | 2.30x | 3.68x | 3.791 (0.670) | 10.759 (1.929) | 2.45x [1.04x-3.43x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.51x** [1.36x-4.65x] | 2.65x | 3.14x | 34.495 (5.832) | 64.648 (10.934) | 2.08x [0.60x-19.82x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.88x** [2.34x-3.55x] | 2.81x | 4.10x | 205.452 (39.350) | 931.300 (182.093) | 2.61x [0.60x-19.82x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **2.15x** | 0.0004 | 0.0009 | 0.0005 | 0.0118 | $0.0004 \pm 0.0001$ | $0.0009 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **2.04x** | 0.0013 | 0.0023 | 0.0015 | 0.0102 | $0.0012 \pm 0.0004$ | $0.0023 \pm 0.0004$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **2.21x** | 0.0024 | 0.0063 | 0.0034 | 0.0153 | $0.0030 \pm 0.0011$ | $0.0064 \pm 0.0020$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.91x** | 0.0022 | 0.0043 | 0.0031 | 0.0149 | $0.0023 \pm 0.0006$ | $0.0043 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.85x** | 0.0026 | 0.0046 | 0.0035 | 0.0156 | $0.0026 \pm 0.0006$ | $0.0048 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **2.02x** | 0.0047 | 0.0110 | 0.0077 | 0.0320 | $0.0055 \pm 0.0021$ | $0.0106 \pm 0.0019$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **3.07x** | 0.0236 | 0.0698 | 0.0297 | 0.0957 | $0.0229 \pm 0.0056$ | $0.0688 \pm 0.0104$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **4.26x** | 0.0482 | 0.183 | 0.0630 | 0.234 | $0.0469 \pm 0.0047$ | $0.216 \pm 0.120$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.02x** | 0.0425 | 0.0934 | 0.0656 | 0.144 | $0.0493 \pm 0.0152$ | $0.0964 \pm 0.0131$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.74x** | 0.129 | 0.219 | 0.137 | 0.255 | $0.139 \pm 0.0460$ | $0.235 \pm 0.0472$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **11.19x** | 0.0496 | 0.689 | 0.0647 | 0.765 | $0.147 \pm 0.334$ | $0.687 \pm 0.0187$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **2.47x** | 0.0276 | 0.0710 | 0.0522 | 0.143 | $0.0299 \pm 0.0113$ | $0.0710 \pm 0.0079$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.56x** | 0.0314 | 0.0797 | 0.0539 | 0.152 | $0.0313 \pm 0.0037$ | $0.0798 \pm 0.0073$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.81x** | 0.0749 | 0.280 | 0.101 | 0.355 | $0.0744 \pm 0.0079$ | $0.284 \pm 0.0317$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **4.05x** | 0.315 | 1.271 | 0.407 | 1.658 | $0.313 \pm 0.0510$ | $1.260 \pm 0.120$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.41x** | 1.145 | 5.229 | 1.520 | 7.585 | $1.212 \pm 0.164$ | $5.321 \pm 0.383$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **2.10x** | 0.714 | 1.479 | 1.311 | 3.935 | $0.706 \pm 0.0218$ | $1.482 \pm 0.0542$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.71x** | 4.891 | 8.436 | 5.095 | 9.773 | $5.437 \pm 1.928$ | $9.169 \pm 2.652$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **3.16x** | 0.270 | 0.819 | 0.644 | 3.500 | $0.266 \pm 0.0196$ | $0.840 \pm 0.0609$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.77x** | 0.336 | 0.951 | 0.728 | 3.611 | $0.347 \pm 0.0176$ | $0.960 \pm 0.0320$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **5.24x** | 5.473 | 27.714 | 7.304 | 34.743 | $5.383 \pm 0.262$ | $28.259 \pm 2.294$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **5.21x** | 25.766 | 134.479 | 34.702 | 177.775 | $25.828 \pm 3.749$ | $133.570 \pm 8.626$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

---

## Pure-Go vs. C++ CBC Performance Gaps and MAD Scaling Analysis

Comparing isolated unpresolved `Root LP` timings, paired trial ratios, and solver-core checkpoints (`CP5 +Presolve`, `CP6 +Cuts`, `CP7 Full`) between `BackendGo` and in-process `BackendCBC` (`c1c171ec`) highlights four structural trade-offs on large degenerate LPs and heavy-tail branch-and-cut models.

### 1. MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families (`N = G(I + T + 3)` variables, `M = G(2T + 3) + I` constraints) separate solver performance into three distinct scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Paired `Go / CBC` | Go Total Med (ms) | CBC Total Med (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.04x** | 0.08 | 2.43 | 0.55x | 0.04 (1 / 19) | 2.39 (0 / 0) | 0.076 (28) | 0.139 (19) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.05x** | 0.49 | 10.99 | 0.65x | 0.46 (2 / 58) | 10.96 (6 / 83) | 0.090 (30) | 0.138 (18) |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.07x** | 0.14 | 1.75 | 0.67x | 0.11 (1 / 26) | 1.71 (0 / 0) | 0.096 (33) | 0.143 (21) |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.08x** | 0.21 | 2.40 | 0.95x | 0.14 (1 / 40) | 2.34 (0 / 0) | 0.234 (58) | 0.246 (40) |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.10x** | 0.34 | 3.13 | 0.86x | 0.26 (1 / 51) | 3.04 (0 / 0) | 0.340 (80) | 0.398 (63) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.24x** | 2.65 | 16.33 | 0.93x | 2.62 (21 / 324) | 16.29 (12 / 216) | 0.141 (38) | 0.151 (24) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.25x** | 15.04 | 89.19 | 0.96x | 14.96 (220 / 2314) | 89.12 (198 / 1300) | 0.270 (65) | 0.282 (54) |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.08x** | 0.53 | 7.97 | 1.02x | 0.40 (1 / 75) | 7.82 (0 / 0) | 0.635 (91) | 0.624 (94) |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.15x** | 0.99 | 7.35 | 1.47x | 0.76 (1 / 98) | 7.11 (0 / 0) | 1.515 (244) | 1.033 (149) |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **0.30x** | 8.33 | 36.40 | 0.74x | 7.17 (2 / 364) | 34.92 (0 / 0) | 2.631 (224) | 3.563 (319) |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **0.18x** | 35.92 | 272.49 | 1.66x | 28.68 (1 / 2439) | 267.24 (0 / 0) | 57.863 (1905) | 34.929 (3128) |
<!-- END GENERATED:MAD_CBC_GAP_TABLE -->

1. **Small-to-Medium Planted-Gap MAD (`t1_mad_g2_i4` through `t3_mad_g5_i20`, $26 \le N \le 145$)**: Every structured `t*_mad_g*` instance has a zero root LP gap by design (`RootLPBound == OptimalObj`). Pure-Go closes those models at node 1 through presolve orbit fixing and root heuristics (`tryPropagatedRounding`, `tryRootSubMIP`, and `tryFractionalDiving` in [`heuristics.go`](heuristics.go)) without C++ `CbcModel` and `CglPreProcess` setup costs.
2. **Large Planted-Gap MAD (`t3_mad_g6_i30`, `t3_mad_g10_i100`, `t3_mad_g4_i1000`)**: Pure-Go `Go Solve` closes all three at node 1 (or 2 with root sub-MIP) because [`Presolve`](presolve.go) verifies the $S_G$ group-permutation automorphism (`applyOrbitSymmetryBreaking`), fixes the $G \times G$ designated-item bijection to $I_G$, and eliminates $G^2$ binary variables and $2G$ assignment equations before the root LP runs, while [`FTRANSparseHint`](lu.go) and [`BTRANSparseHint`](lu.go) restrict basis triangular sweeps to active index windows. On the unpresolved `Root LP` column:
   - **Equality Crash Basis (`t3_mad_g10_i100`)**: [`crashEqualityBasis`](simplex.go) builds a lower-triangular, dual-feasible equality basis with analytical DSE weights (`2.631 ms` / `224` iters vs. `COIN-OR CLP`'s `3.563 ms` / `319` iters, **`0.74x`**).
   - **Dense-Row Coupling and Forrest-Tomlin Updates (`t3_mad_g4_i1000`)**: On 1,000-row bases (`57.863 ms` / `1905` iters vs. `CLP`'s `34.929 ms` / `3128` iters, **`1.66x`**), [`crashEqualityBasis`](simplex.go) skips columns incident to coupling rows wider than 128 nonzeros so BFRT bound flips retain sparse unit slacks, while [`lu.go`](lu.go) performs in-place mutated-$U$ Forrest-Tomlin updates (`PushEtaSparse`) inside pre-allocated headroom arenas up to `maxEtaUpdates = 56`.
3. **Symmetric Random-Weight MAD (`t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`)**: Assigning dense random weights (`BuildMADRandom`) removes the zero-gap structure and forces Branch-and-Cut to prove optimality:
   - [`Presolve`](presolve.go) verifies exact $S_G$ group symmetry and fixes the $G \times G$ designated-item bijection to $I_G$, [`bab.go`](bab.go) exits after round 1 when root cuts yield zero bound gain and purges slack root cuts via [`PurgeSlackCuts`](simplex.go), and [`tryRootSubMIP`](heuristics.go) enforces a 50% RINS integer-fixing gate on zero-cut roots.
   - Across [`docs/benchmarks.md`](docs/benchmarks.md#mad-solver-core-checkpoint-progression), `CP5 +Presolve` (`Presolve=on, Cuts=off, Heuristics=off`) remains slightly faster than `CP7 Full` on tiny `t2_mad_rand_g2_i8` trees (`2.53 ms` vs. `2.62 ms`) due to root trial separation, whereas `CP7 Full` runs more than twice as fast as `CP5 +Presolve` on `t2_mad_rand_g3_i9` (`14.96 ms` vs. `35.83 ms`).

### 2. Non-MAD Corpus Comparison, Dispersion, and Checkpoint Trade-Offs

Across the non-MAD families, multi-row MIR cut aggregation (`separateAggregatedMIRCuts`) and selective in-tree GMI separation reduce `t3_lotsizing_504` mean latency from Wave 3's `576.76 ms` down to `49.65 ± 25.88 ms` (`0.53x` vs. CBC), and `CP7 Full` now outperforms `CP5 +Presolve` on `t3_cflp_300` (`5.65 ms` at 1 node vs. `9.21 ms`), while outlier trials of `t2_binpack_44` retain higher variance (`17.05 ± 49.76 ms` mean vs. `0.48 ms` median solve):

<!-- BEGIN GENERATED:NON_MAD_CBC_GAP_TABLE -->
| Problem ID | $N \times M$ | Paired `Go / CBC` | Go Total Mean $\pm$ SD (ms) | CBC Total Mean $\pm$ SD (ms) | Go CP7 Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go CP6 +Cuts (ms / nodes / cuts) | Go CP5 +Presolve (ms / nodes) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t3_mkp_200` | $200 \times 8$ | **0.87x** | $34.27 \pm 16.04$ | $39.50 \pm 18.71$ | 37.73 (448 / 3584) | 48.42 (154 / 1145) | 83.49 (1144 / 16) | 177.89 (5000, `NodeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **0.51x** | $9.64 \pm 11.89$ | $12.18 \pm 8.90$ | 6.32 (1 / 19) | 8.97 (0 / 8) | 15.18 (2 / 4) | 7.10 (132) |
| `t2_binpack_44` | $44 \times 14$ | **0.19x** | $17.05 \pm 49.76$ | $6.50 \pm 2.13$ | 0.48 (2 / 43) | 5.95 (0 / 8) | 0.94 (5 / 8) | 21.91 (1424) |
| `t3_cflp_300` | $300 \times 34$ | **0.12x** | $7.55 \pm 5.74$ | $54.09 \pm 11.59$ | 5.65 (1 / 105) | 49.11 (18 / 483) | 17.84 (46 / 34) | 9.21 (134) |
| `t3_lotsizing_504` | $504 \times 336$ | **0.53x** | $49.65 \pm 25.88$ | $91.26 \pm 56.52$ | 63.50 (2 / 1054) | 70.25 (10 / 377) | 80.73 (376 / 206) | 250.03 (3380, `TimeLimit`) |
| `t3_sched_1000` | $1000 \times 100$ | **0.12x** | $7.98 \pm 4.26$ | $59.91 \pm 4.89$ | 6.80 (32 / 435) | 58.68 (0 / 76) | 20.99 (87 / 53) | 250.06 (2209, `TimeLimit`) |
<!-- END GENERATED:NON_MAD_CBC_GAP_TABLE -->

---

## Roadmap Status and Architectural History

All **P0–P3** Wave 4 roadmap items across `BackendGo`, `BackendCBC`, `goblin/assign`, and `goblin/format` have landed and passed verification on `amd64` (AVX2 & GCP `n4` Emerald Rapids AVX-512) and `linux/arm64` (GCP `c4a` Neoverse-V2). No open P0–P3 gaps remain.

### Architectural Context and Rejected Designs

1. **LU Updates and Row-Singleton Stability (`lu.go`)**: The factorizer maintains elbow-room $U$ arenas (`uRowStart`/`uColStart`) paired with rank-free in-place mutated-$U$ Forrest-Tomlin updates (`PushEta`/`PushEtaSparse`) up to `maxEtaUpdates = 56`, avoiding extra branch checks during `FTRAN` and `BTRAN`. Two alternatives were tested and rejected earlier: (a) a lazy-rank Forrest-Tomlin prototype with `maxEtaUpdates = 128` regressed `miplib_p0201` (0.62x) and inflated `netlib_brandy` iterations (380 to 2,722) from `uRank` branch mispredictions; (b) removing the `luMarkowitzTol * colMax` check on Stage 2 row singletons caused unscaled `1000x` multipliers in $L$ and numerical stalling on `t3_mad_g4_i1000`.
2. **Root LP Crash Basis and Dense Coupling (`simplex.go`)**: Dual Phase I (`installDualPhase1Bounds`) keeps all 14 public Netlib LPs inside sparse dual simplex with zero primal fallbacks, and [`crashEqualityBasis`](simplex.go) cuts `t3_mad_g10_i100` unpresolved Root LP to `2.631 ms` vs. CLP `3.563 ms` (`0.74x`). Crashing columns incident to dense coupling rows (`> 128` nonzeros) was rejected because displacing 1-NNZ artificial slacks on `t3_mad_g4_i1000` doubled $U$ fill-in and slowed solve latency by 2.5x.
3. **Cut Separation, Symmetry, and Zero-Cut Sub-MIP Gating (`cuts.go`, `presolve.go`, `bab.go`, `heuristics.go`)**: Root cut rounds enforce GMI/MIR density caps (`maxCutNNZ`), exit after round 1 on zero bound gain, and purge inactive slack cuts both before branching and periodically in-tree via [`PurgeSlackCuts`](simplex.go). On zero-cut roots, a blanket `< 48`-integer sub-MIP ban improved 7-trial median on `t2_mad_rand_g3_i9` but inflated 11-trial mean and tail; requiring 50% fixed integers (RINS criterion) improved median, mean, tail, and node counts across all 11 trials.
4. **Completed Milestones**:
   - **Numerical and Bridge Hardening (`3f52ee8`, `9a6b3f5`, `85d57a3`)**: Multi-scale three-bucket Shewchuk summation (`wideDotAcc`, `assignWideAcc`), CBC tiny-coefficient and cutoff guards, MIPLIB 3.0 public benchmark fixtures, and family-clustered Student-$t$ telemetry intervals.
   - **Wave 0 (`facbd10`)**: Pointer-free inline `wideDotAcc` (zero heap escapes), per-row `rowMaxTerm` residual triggers, and stack `dot2Acc` fast paths.
   - **Wave 1 (`bd0bdf4` to `fe40685`)**: Elimination of Go 1.27 `archsimd` AVX2-to-SSE `VZEROUPPER` traps, scale-relative `assign.hasPotentialCancellation` (0 allocs/op warm `SolveInto`), branchless AVX2 Dijkstra relaxation (`assign_amd64.go`), zero-pin SWAR JSON parsing and buffered `WriteMPS`, lazy CSC indexing on `extraRows`, batch-fixed `solveFixedIntContinuousLP`, and bounded post-cut root heuristics.
   - **Wave 2 (`c7cb1a4` to `99a49c0`)**: Flat single-value LU nucleus arenas with intrusive `int32` Markowitz buckets (0 allocs/op steady-state LU), EMA-gated iterative Gilbert-Peierls reach DFS, copy-on-write `Presolve` with direct `CompiledProblem` compilation, lazy `cbcOnce` CBC slices, Dual Phase I, packed contiguous `basicLB`/`basicUB`/`basicTol`, GMI/MIR density caps, in-tree slack-cut aging, and incremental CSC node propagation (`propagateFromCols`).
   - **Wave 3 (`8bcb706` to `2b7305d`, Multi-Tier AVX-512/AVX2 SIMD & MAD Solver Optimization, verified on GCP `n4` Emerald Rapids & `e2` Broadwell)**: Runtime AVX-512/AVX2/Scalar CPUID dispatch in a single `CGO_ENABLED=0` binary (`GOAMD64=v3` FMA-safe `twoProd`/`twoSum` error-free transforms), 4-accumulator scalar `Axpy64Scalar` for Broadwell, in-register DSE leaving-row scan (`SelectDualLeaveRow64With`), batched dual ratio screening (`ScanDualRatioCand64With`), zero-alloc equality crash basis (`crashEqualityBasis`), certified unrefined residual fast paths (`evalRowRes`), dirty-row presolve tightening and zero-sort $O(G \cdot \text{rowLen})$ orbit checks, decoupled epigraph rounding with fractional-only sort, zero-cut 50% RINS sub-MIP gating, and root-LP re-solve plus CBC primal polishing (`FuzzMADDifferentialCBC`).
   - **Wave 4 (P0–P3 Roadmap Completion, verified on GCP `n4` Emerald Rapids AVX-512 & `c4a` Axion `linux/arm64`)**: P0 `linux/arm64` native parity and `cbcLPDualWithinTol` basic-column guard ([`cbc_cgo.go`](cbc_cgo.go), [`internal/simd/kernels_test.go`](internal/simd/kernels_test.go)); P1.1 in-place mutated-$U$ Forrest-Tomlin updates ([`lu.go`](lu.go), cutting `t3_mad_g4_i1000` unpresolved Root LP from `2.90x` to `1.66x` vs. CLP); P1.2 dual cost perturbation and dual-exact LP presolve ([`simplex.go`](simplex.go), [`presolve.go`](presolve.go)); P1.3 multi-row MIR cut aggregation and selective in-tree GMI cuts ([`cuts.go`](cuts.go), [`bab.go`](bab.go), cutting `t3_lotsizing_504` mean latency 11.6x to `49.65 ms`, `0.53x` vs. CBC); P2.1 dynamic node-level orbital branching ([`bab.go`](bab.go), [`presolve.go`](presolve.go)); P2.2 strong-branching bound-cutoff domain reduction (`t3_cflp_300` median solve `5.65 ms` vs. `10.39 ms`); P3.1 post-ARR Volgenant sparse-core LSAP pricing ([`assign/solver.go`](assign/solver.go), [`assign/workspace.go`](assign/workspace.go)); P3.2 direct streaming `PuLP` JSON ingestion ([`format/pulp_json_fast.go`](format/pulp_json_fast.go)); and P3.3 optional parallel Branch-and-Bound via `WithWorkers` ([`solve.go`](solve.go), [`bab.go`](bab.go), achieving 4.05x 4-vCPU speedup on `miplib_p0201` on `c4a` and 3.45x on `n4`, with deterministic single-threaded default).

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

# 5. Warm-state goblin benchmarks across Pure-Go (AB/BA paired scalar & SIMD) and C++ CBC backends, pinned to CPU 2
GOEXPERIMENT=simd go build -o /tmp/goblin-bench-simd ./cmd/goblin-bench
taskset -c 2 /tmp/goblin-bench-simd -backend go -workers 1 -paired-scalar-out bench/results/goblin_scalar.json -out bench/results/goblin_simd.json
go build -tags=cbc -o /tmp/goblin-bench-cbc ./cmd/goblin-bench && taskset -c 2 /tmp/goblin-bench-cbc -backend cbc -workers 1 -out bench/results/goblin_cbc.json

# 6. Warm-state SciPy and goblin/assign LSAP benchmarks (AB/BA paired scalar & SIMD), pinned to CPU 2
taskset -c 2 python3 bench/scipy_assign_baseline.py --corpus-dir testdata/assign_corpus --out-json bench/results/scipy_assign_baseline.json
taskset -c 2 /tmp/goblin-bench-simd -assign -paired-scalar-out bench/results/goblin_assign_scalar.json -out bench/results/goblin_assign_simd.json

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
