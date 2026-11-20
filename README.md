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

1. **Zero-Subprocess In-Memory Execution**: Modeling (`Model`), matrix checks and assembly (`Model.Validate()`, `Model.CompileChecked()`), presolve (`Presolve`), root and node LPs (`LPSolver`), and Branch-and-Cut (`SolveCompiled`) execute in-process in pure Go using pooled workspace buffers (`AcquireWorkspace` / `ReleaseWorkspace`) with zero `cgo` dependency by default, plus an optional `-tags cbc` in-process `cgo` bridge to COIN-OR CBC/CLP (`cbc_cgo.go`, serialized process-wide across goroutines to guard unsynchronized upstream `Cgl`/`Cbc` static state).
2. **Schema Compatibility with Python `PuLP` and Free-Format MPS Export**: The `goblin/format` package reads and writes `PuLP`'s native `toDict()` / `fromDict()` JSON schema ([`format/pulp_json.go`](format/pulp_json.go)) and writes free-format MPS files ([`format/mps.go`](format/mps.go)).
3. **Optional SIMD Kernels**: With `GOEXPERIMENT=simd` on `amd64` (gated at runtime on CPU `AVX2` and `FMA` support), [`internal/simd/kernels_amd64.go`](internal/simd/kernels_amd64.go) dispatches 256-bit AVX2 `archsimd.Float64x4` instructions (`VFMADD213PD` in `Dot64`; `VMULPD` + `VADDPD` in `Axpy64`; `VSUBPD` + `VMINPD` in `MinReducedSlice64` and `InitReducedRow64`; `VSUBPD` + `VCMPPD`/`VPAND`/`VMOVMSKPD` in `Min2ReducedRow64` at length 256 or more; 8-lane `Abs().LessEqual` in `AllAbsLE64`, gated at 256 elements in `assign/solver.go`) with `VZEROUPPER` cleanup and IEEE-754 `NaN` propagation parity across scalar and vector paths, and can be toggled at runtime via `WithoutSIMD()` or `WithSIMD(false)`. See the paired comparison in [`docs/benchmarks.md`](docs/benchmarks.md#5-default-build-vs-goexperimentsimd).
4. **Verified Parity on Synthetic and Public Suites**: [`solver_test.go`](solver_test.go) re-solves all 293 distinct randomized trial instances of the 31 synthetic benchmark families (`testdata/corpus/trials/*.jsonl`), compares status and objective with `PuLP + COIN-OR CBC` (`c1c171ec`) and `BackendCBC`, and verifies primal and row feasibility on every result. [`public_benchmarks_test.go`](public_benchmarks_test.go) verifies optimal status, objective parity, and primal feasibility across 20 held-out public Netlib LP and MIPLIB 3.0 MILP instances (`testdata/public/`) plus a valid dual lower bound under a 25-node budget (`go_max_nodes: 25`) on `coinor_capplan1`, while [`regression_test.go`](regression_test.go) tests big-M fixed-charge models against enumerated ground truth and Farkas-certified LP scaling edge cases. `TimeLimit`, `MaxNodes`, and `MaxIter` return `StatusTimeLimit`, `StatusNodeLimit`, or `StatusIterationLimit` with a valid global bound once the root LP is solved; a node that cannot be evaluated is never discarded silently.

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
- **Unified telemetry source**: fingerprint `e204cda60eec4c2c` (commit `99a49c0d04e2f9edf0a4c6466a80b49b8a54b088`), `go1.27.3-X:simd`, Linux kernel `6.18.63-gentoo-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, or kernel differs.
- **CPU affinity**: goblin `6`; Python + CBC `6`; Python + SciPy `6`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.56 ms median on a 1-variable MILP and 6.27 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

Read the synthetic corpus ratios with three methodological notes. First, Tier 1 micro-models (`N <= 26`) are dominated by Python's CBC subprocess spawn floor reported in the provenance block above. Second, the planted-gap MAD family has a zero root LP gap by construction and closes at the root without branching. Consequently, the bold row (`Tier 2+3 non-planted MILPs`) isolates solver-dominated MILPs across paired randomized instances, alongside total subset wall times and full per-instance `[Min-Max]` ranges.

### 1. Aggregate Synthetic Workload Summary (Paired Per-Instance Geometric Mean and Wall-Clock Sums)

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `Py / Go` GeoMean [Clustered 95% CI] | Pooled Trial `Py / Go` | Equal-Family `Go / CBC` | `Go` Trial (Med) Sum (ms) | `CBC` Trial (Med) Sum (ms) | `Py` Trial (Med) Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **272.73x** [123.43x-602.64x] | 272.73x | 0.06x | 16.5 (1.5) | 193.2 (19.3) | 1065.3 (100.2) | 365.74x [10.99x-2701.65x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **14.56x** [8.16x-26.00x] | 14.56x | 0.20x | 1097.0 (53.3) | 1959.1 (170.9) | 4307.9 (416.9) | 18.67x [0.10x-145.71x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **6.28x** [2.57x-15.35x] | 6.28x | 0.33x | 4368.5 (210.4) | 2882.3 (557.8) | 6174.8 (1064.9) | 7.42x [0.07x-32.39x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **7.96x** [4.39x-14.44x] | 9.09x | 0.32x | 5080.1 (207.2) | 3103.3 (392.6) | 6761.3 (985.0) | 9.19x [0.07x-145.71x] |
| Tier 3 non-MAD MILPs | 4 (20) | **3.88x** [0.56x-27.01x] | 3.88x | 0.56x | 3991.9 (154.7) | 1238.2 (229.7) | 2800.5 (599.2) | 3.62x [0.07x-23.01x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **24.09x** [9.33x-62.19x] | 33.29x | 0.12x | 386.2 (56.5) | 1762.5 (338.6) | 3826.0 (506.4) | 34.64x [3.77x-156.02x] |
| Random-weight MAD (negative controls) | 3 (33) | **9.64x** [0.71x-130.32x] | 9.64x | 0.25x | 539.4 (34.2) | 1210.6 (115.5) | 2242.0 (247.7) | 8.72x [0.97x-69.31x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **32.62x** [13.25x-80.35x] | 40.78x | 0.17x | 5095.9 (208.7) | 3272.1 (409.4) | 7722.0 (1075.6) | 31.40x [0.07x-2701.65x] |
| All 31 synthetic corpus families | 31 (293) | **30.17x** [15.18x-59.95x] | 39.01x | 0.15x | 5482.1 (265.2) | 5034.6 (748.0) | 11548.0 (1582.0) | 32.97x [0.07x-2701.65x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2. Synthetic MAD Group-Item Allocation Family (`G` Groups $\times$ `I` Items, $T = 6$ Periods)

The synthetic `MAD` group-item allocation family ([`BuildMADFromWeights`](bench/corpus.go)) assigns $I$ items across $G$ groups over $T = 6$ periods to minimize total L1 distance from each group's central level $m_g$ plus the roundness penalty $r_g = |m_g - 100 n_g|$ relative to an integer multiple $n_g \in \mathbb{Z}_{\ge 1}$. In the planted-gap generator, items in $S = \{0, \dots, I-G-1\}$ carry orthogonal curve signatures cancelled by the $G$ designated items $D = \{I-G, \dots, I-1\}$ with roundness-aligned period sums, yielding a zero root LP gap that root heuristics close without branching; unstructured random-weight negative controls (`BuildMADRandom`: `t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`) are included directly in the corpus and detailed in [`docs/benchmarks.md`](docs/benchmarks.md).

<!-- BEGIN GENERATED:MAD_PIPELINE_TABLE -->
| Problem ID | $G \times I$ | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Trial 0 Obj | Parity |
| :--- | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **144.8x** [134.1x-156.0x] | 0.07 | 9.65 | 0.04 | 9.44 | 0.023 | 0.265 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **31.5x** [11.0x-69.3x] | 1.25 | 33.39 | 1.22 | 33.14 | 0.031 | 0.309 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **46.2x** [34.9x-64.6x] | 0.23 | 9.53 | 0.16 | 9.26 | 0.042 | 0.362 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **55.0x** [33.0x-65.0x] | 0.18 | 10.05 | 0.12 | 9.65 | 0.054 | 0.515 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **28.8x** [23.2x-34.4x] | 0.39 | 11.50 | 0.31 | 10.89 | 0.082 | 0.785 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **6.7x** [2.8x-17.9x] | 5.04 | 47.32 | 4.98 | 47.02 | 0.045 | 0.388 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **4.3x** [1.0x-16.4x] | 27.90 | 167.03 | 27.84 | 166.57 | 0.070 | 0.577 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **27.9x** [22.7x-32.4x] | 0.64 | 17.79 | 0.48 | 16.67 | 0.156 | 1.361 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **10.5x** [9.8x-11.0x] | 1.78 | 18.71 | 1.50 | 16.96 | 0.263 | 2.286 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **4.8x** [3.8x-6.0x] | 12.68 | 60.66 | 11.36 | 52.91 | 1.607 | 10.821 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **7.7x** [3.8x-13.5x] | 40.56 | 368.54 | 34.94 | 334.66 | 4.737 | 43.643 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

Cross-instance total latency dispersion across the randomized instances in each family (mean $\pm$ sample SD and two-sided 95% Student-$t$ interval after warmup):

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.07 \pm 0.00$ ($\pm 0.00$) | $9.51 \pm 0.38$ ($\pm 0.26$) | $0.07 \pm 0.00$ ($\pm 0.00$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $1.18 \pm 0.76$ ($\pm 0.51$) | $30.56 \pm 9.54$ ($\pm 6.41$) | $1.21 \pm 0.78$ ($\pm 0.52$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.21 \pm 0.04$ ($\pm 0.03$) | $9.78 \pm 0.74$ ($\pm 0.50$) | $0.21 \pm 0.02$ ($\pm 0.02$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.19 \pm 0.04$ ($\pm 0.03$) | $10.09 \pm 0.20$ ($\pm 0.14$) | $0.18 \pm 0.03$ ($\pm 0.02$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.41 \pm 0.04$ ($\pm 0.03$) | $11.69 \pm 0.70$ ($\pm 0.47$) | $0.41 \pm 0.04$ ($\pm 0.03$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $6.79 \pm 5.61$ ($\pm 3.77$) | $35.39 \pm 16.92$ ($\pm 11.37$) | $6.74 \pm 5.51$ ($\pm 3.70$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $41.08 \pm 40.63$ ($\pm 27.30$) | $137.87 \pm 63.08$ ($\pm 42.38$) | $41.19 \pm 41.02$ ($\pm 27.56$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.63 \pm 0.02$ ($\pm 0.02$) | $17.81 \pm 2.29$ ($\pm 2.85$) | $0.63 \pm 0.02$ ($\pm 0.02$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.76 \pm 0.07$ ($\pm 0.09$) | $18.49 \pm 0.47$ ($\pm 0.58$) | $1.88 \pm 0.19$ ($\pm 0.24$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $13.33 \pm 1.89$ ($\pm 2.35$) | $63.07 \pm 3.89$ ($\pm 4.83$) | $12.62 \pm 1.26$ ($\pm 1.57$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $59.59 \pm 35.75$ ($\pm 44.39$) | $575.49 \pm 606.99$ ($\pm 753.56$) | $58.07 \pm 33.82$ ($\pm 41.98$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### 3. Full 31-Family Synthetic Benchmark Corpus (`JSON -> Solution`)

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | **395.9x** [119.7x-602.2x] | 0.01 | 6.21 | 0.00 | 6.17 | 0.008 | 0.064 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **456.4x** [209.0x-622.5x] | 0.01 | 6.26 | 0.00 | 6.21 | 0.009 | 0.066 | `OK` |
| `t1_std_mip` | 3 | 3 | **453.6x** [177.6x-713.4x] | 0.02 | 7.37 | 0.01 | 7.32 | 0.008 | 0.069 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **506.2x** [401.8x-672.4x] | 0.01 | 7.68 | 0.01 | 7.63 | 0.007 | 0.076 | `OK` |
| `t1_whiskas` | 6 | 5 | **330.7x** [231.9x-425.8x] | 0.02 | 6.50 | 0.01 | 6.41 | 0.013 | 0.100 | `OK` |
| `t1_furniture` | 2 | 3 | **197.9x** [52.2x-580.6x] | 0.03 | 7.71 | 0.02 | 7.66 | 0.007 | 0.064 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **97.4x** [27.4x-319.3x] | 0.09 | 9.21 | 0.08 | 9.12 | 0.009 | 0.112 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **1889.7x** [1042.9x-2701.7x] | 0.00 | 6.22 | 0.00 | 6.18 | 0.002 | 0.053 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **144.8x** [134.1x-156.0x] | 0.07 | 9.65 | 0.04 | 9.44 | 0.023 | 0.265 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **31.5x** [11.0x-69.3x] | 1.25 | 33.39 | 1.22 | 33.14 | 0.031 | 0.309 | `OK` |
| `t2_mkp_30` | 30 | 6 | **6.0x** [3.4x-10.2x] | 3.07 | 27.83 | 3.05 | 27.66 | 0.023 | 0.224 | `OK` |
| `t2_mkp_50` | 50 | 8 | **3.1x** [1.2x-25.2x] | 8.66 | 16.66 | 8.62 | 16.42 | 0.040 | 0.326 | `OK` |
| `t2_cflp_36` | 36 | 11 | **28.2x** [3.7x-145.7x] | 0.27 | 9.24 | 0.25 | 9.07 | 0.021 | 0.206 | `OK` |
| `t2_cflp_49` | 49 | 13 | **17.2x** [9.2x-31.3x] | 0.58 | 10.30 | 0.55 | 10.08 | 0.025 | 0.254 | `OK` |
| `t2_gap_45` | 45 | 13 | **15.8x** [2.0x-67.9x] | 1.02 | 14.27 | 1.00 | 14.08 | 0.024 | 0.245 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **9.9x** [4.4x-22.8x] | 5.19 | 68.18 | 5.15 | 67.94 | 0.032 | 0.314 | `OK` |
| `t2_binpack_44` | 44 | 14 | **9.5x** [0.1x-42.1x] | 0.58 | 13.62 | 0.56 | 13.43 | 0.030 | 0.238 | `OK` |
| `t2_setcover_50` | 50 | 20 | **47.2x** [3.6x-142.7x] | 0.20 | 11.44 | 0.16 | 11.15 | 0.037 | 0.399 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **46.2x** [34.9x-64.6x] | 0.23 | 9.53 | 0.16 | 9.26 | 0.042 | 0.362 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **55.0x** [33.0x-65.0x] | 0.18 | 10.05 | 0.12 | 9.65 | 0.054 | 0.515 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **28.8x** [23.2x-34.4x] | 0.39 | 11.50 | 0.31 | 10.89 | 0.082 | 0.785 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **6.7x** [2.8x-17.9x] | 5.04 | 47.32 | 4.98 | 47.02 | 0.045 | 0.388 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **4.3x** [1.0x-16.4x] | 27.90 | 167.03 | 27.84 | 166.57 | 0.070 | 0.577 | `OK` |
| `t3_mkp_200` | 200 | 8 | **1.8x** [1.2x-3.2x] | 52.40 | 115.68 | 52.28 | 115.07 | 0.107 | 0.799 | `OK` |
| `t3_cflp_300` | 300 | 34 | **16.7x** [10.1x-23.0x] | 11.44 | 246.63 | 11.32 | 245.71 | 0.119 | 1.179 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **1.1x** [0.1x-3.4x] | 78.84 | 156.63 | 78.59 | 154.79 | 0.250 | 2.291 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **6.5x** [3.9x-9.5x] | 12.03 | 80.23 | 11.69 | 77.33 | 0.331 | 3.481 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **27.9x** [22.7x-32.4x] | 0.64 | 17.79 | 0.48 | 16.67 | 0.156 | 1.361 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **10.5x** [9.8x-11.0x] | 1.78 | 18.71 | 1.50 | 16.96 | 0.263 | 2.286 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **4.8x** [3.8x-6.0x] | 12.68 | 60.66 | 11.36 | 52.91 | 1.607 | 10.821 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **7.7x** [3.8x-13.5x] | 40.56 | 368.54 | 34.94 | 334.66 | 4.737 | 43.643 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

### 4. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus ([`bench/assign_corpus.go`](bench/assign_corpus.go), 200 randomized instances across square, rectangular wide/tall, degenerate integer-tie, Euclidean geometric, rank-1 outer-product, sparse `+Inf`, and maximization families) compares [`assign.LinearSumFlat`](assign/assign.go) against SciPy's C++ `rectangular_lsap` extension (`scipy.optimize.linear_sum_assignment`) in warm state on bit-identical `float64` cost matrices:

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `SciPy Solve / goblin` GeoMean [Clustered 95% CI] | Pooled Trial `SciPy Solve / goblin` | Equal-Family `SciPy Total / goblin` | `goblin` Solve Trial (Med) Sum (ms) | SciPy Solve Trial (Med) Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.76x** [1.55x-1.99x] | 1.76x | 4.85x | 0.189 (0.0151) | 0.316 (0.0290) | 1.77x [0.73x-3.95x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **3.38x** [2.05x-5.58x] | 3.38x | 3.23x | 9.261 (0.727) | 32.592 (2.952) | 3.07x [0.17x-20.23x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **3.23x** [2.15x-4.84x] | 3.23x | 3.93x | 206.122 (40.593) | 922.564 (184.208) | 3.27x [1.48x-6.39x] |
| Square families ($m = n$) | 16 (146) | **2.98x** [2.15x-4.13x] | 2.92x | 3.96x | 211.837 (40.650) | 944.623 (185.258) | 2.73x [0.17x-20.23x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **2.33x** [1.65x-3.29x] | 2.20x | 3.54x | 3.736 (0.685) | 10.850 (1.932) | 2.56x [0.85x-3.54x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.50x** [1.18x-5.26x] | 2.63x | 3.01x | 35.183 (6.056) | 64.709 (10.919) | 2.07x [0.17x-20.23x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.79x** [2.18x-3.56x] | 2.70x | 3.84x | 215.572 (41.335) | 955.473 (187.189) | 2.61x [0.17x-20.23x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.85x** | 0.0005 | 0.0009 | 0.0006 | 0.0115 | $0.0005 \pm 0.0001$ | $0.0009 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.75x** | 0.0013 | 0.0024 | 0.0017 | 0.0103 | $0.0016 \pm 0.0010$ | $0.0025 \pm 0.0006$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **2.04x** | 0.0026 | 0.0064 | 0.0044 | 0.0153 | $0.0032 \pm 0.0016$ | $0.0062 \pm 0.0017$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.52x** | 0.0025 | 0.0040 | 0.0042 | 0.0150 | $0.0027 \pm 0.0008$ | $0.0040 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.55x** | 0.0028 | 0.0043 | 0.0044 | 0.0157 | $0.0030 \pm 0.0014$ | $0.0044 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.89x** | 0.0054 | 0.0109 | 0.0089 | 0.0325 | $0.0061 \pm 0.0028$ | $0.0108 \pm 0.0017$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.96x** | 0.0239 | 0.0711 | 0.0337 | 0.0964 | $0.0238 \pm 0.0051$ | $0.0695 \pm 0.0098$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.72x** | 0.0482 | 0.177 | 0.0651 | 0.227 | $0.0472 \pm 0.0042$ | $0.176 \pm 0.0251$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.04x** | 0.0428 | 0.0930 | 0.0675 | 0.141 | $0.0491 \pm 0.0169$ | $0.0960 \pm 0.0141$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.43x** | 0.132 | 0.220 | 0.139 | 0.263 | $0.236 \pm 0.323$ | $0.236 \pm 0.0435$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **15.13x** | 0.0493 | 0.687 | 0.0651 | 0.761 | $0.0457 \pm 0.0074$ | $0.682 \pm 0.0149$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **2.81x** | 0.0277 | 0.0732 | 0.0507 | 0.147 | $0.0268 \pm 0.0035$ | $0.0752 \pm 0.0078$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.67x** | 0.0309 | 0.0837 | 0.0536 | 0.156 | $0.0315 \pm 0.0032$ | $0.0841 \pm 0.0071$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.83x** | 0.0747 | 0.279 | 0.0976 | 0.355 | $0.0742 \pm 0.0084$ | $0.284 \pm 0.0321$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **4.12x** | 0.297 | 1.268 | 0.410 | 1.683 | $0.307 \pm 0.0500$ | $1.259 \pm 0.121$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.41x** | 1.138 | 5.217 | 1.516 | 7.699 | $1.210 \pm 0.158$ | $5.318 \pm 0.392$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **2.11x** | 0.717 | 1.504 | 1.319 | 4.088 | $0.709 \pm 0.0293$ | $1.495 \pm 0.0535$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.68x** | 5.107 | 8.397 | 5.312 | 9.232 | $5.579 \pm 2.066$ | $9.177 \pm 2.644$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **3.23x** | 0.264 | 0.810 | 0.636 | 3.699 | $0.259 \pm 0.0131$ | $0.835 \pm 0.0581$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.78x** | 0.357 | 0.956 | 0.725 | 3.655 | $0.348 \pm 0.0228$ | $0.966 \pm 0.0348$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **5.16x** | 5.513 | 27.708 | 7.369 | 35.599 | $5.467 \pm 0.335$ | $28.233 \pm 2.217$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **5.04x** | 27.496 | 139.615 | 38.809 | 181.954 | $27.653 \pm 4.068$ | $138.488 \pm 9.926$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

---

## Pure-Go vs. C++ CBC Performance Gaps and MAD Scaling Analysis

Comparing isolated unpresolved `Root LP` timings, paired trial ratios, and solver-core checkpoints (`CP5 +Presolve`, `CP6 +Cuts`, `CP7 Full`) between `BackendGo` and in-process `BackendCBC` (`c1c171ec`) highlights four structural trade-offs on large degenerate LPs and heavy-tail branch-and-cut models.

### 1. MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families (`N = G(I + T + 3)` variables, `M = G(2T + 3) + I` constraints) separate solver performance into three distinct scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Paired `Go / CBC` | Go Total Med (ms) | CBC Total Med (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.03x** | 0.07 | 2.45 | 0.49x | 0.04 (1 / 19) | 2.41 (0 / 0) | 0.068 (28) | 0.140 (19) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.10x** | 1.25 | 11.08 | 0.58x | 1.22 (10 / 216) | 11.03 (6 / 83) | 0.084 (30) | 0.144 (18) |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.09x** | 0.23 | 2.03 | 0.66x | 0.16 (1 / 26) | 1.98 (0 / 0) | 0.108 (33) | 0.165 (21) |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.07x** | 0.18 | 2.60 | 0.80x | 0.12 (1 / 39) | 2.54 (0 / 0) | 0.230 (58) | 0.286 (40) |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.11x** | 0.39 | 3.33 | 0.81x | 0.31 (1 / 51) | 3.25 (0 / 0) | 0.324 (67) | 0.403 (63) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.36x** | 5.04 | 16.54 | 0.90x | 4.98 (45 / 582) | 16.50 (12 / 216) | 0.142 (38) | 0.158 (24) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.41x** | 27.90 | 87.87 | 0.86x | 27.84 (220 / 2391) | 87.80 (198 / 1300) | 0.247 (65) | 0.288 (54) |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.08x** | 0.64 | 8.10 | 1.44x | 0.48 (1 / 75) | 7.95 (0 / 0) | 0.906 (137) | 0.629 (94) |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.24x** | 1.78 | 7.42 | 1.39x | 1.50 (1 / 105) | 7.19 (0 / 0) | 1.482 (337) | 1.068 (149) |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **0.37x** | 12.68 | 35.58 | 3.30x | 11.36 (2 / 365) | 34.55 (0 / 0) | 12.048 (1203) | 3.654 (319) |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **0.23x** | 40.56 | 277.04 | 3.16x | 34.94 (1 / 2439) | 271.50 (0 / 0) | 111.759 (1905) | 35.324 (3128) |
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
| `t3_mkp_200` | $200 \times 8$ | **1.31x** | $49.99 \pm 21.11$ | $40.12 \pm 19.04$ | 52.28 (598 / 5126) | 49.15 (154 / 1145) | 143.49 (1552 / 9) | 250.02 (4957, `TimeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **0.69x** | $12.84 \pm 15.22$ | $12.28 \pm 8.97$ | 8.62 (1 / 19) | 8.50 (0 / 8) | 22.62 (2 / 4) | 9.16 (132) |
| `t2_binpack_44` | $44 \times 14$ | **0.23x** | $21.42 \pm 60.84$ | $6.82 \pm 2.24$ | 0.56 (2 / 67) | 6.19 (0 / 8) | 1.05 (7 / 7) | 25.19 (1372) |
| `t3_cflp_300` | $300 \times 34$ | **0.25x** | $14.45 \pm 7.20$ | $54.74 \pm 11.66$ | 11.32 (53 / 1213) | 49.76 (18 / 483) | 12.72 (59 / 20) | 10.11 (134) |
| `t3_lotsizing_504` | $504 \times 336$ | **1.48x** | $721.02 \pm 1478.43$ | $92.24 \pm 56.70$ | 78.59 (30 / 1534) | 70.63 (10 / 377) | 133.60 (488 / 205) | 250.05 (2653, `TimeLimit`) |
| `t3_sched_1000` | $1000 \times 100$ | **0.20x** | $12.92 \pm 4.61$ | $60.54 \pm 5.02$ | 11.69 (64 / 426) | 59.02 (0 / 76) | 46.61 (206 / 46) | 250.12 (1786, `TimeLimit`) |
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
