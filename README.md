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
- **Unified telemetry source**: fingerprint `9c208e08cb0fabf1` (working tree on top of commit `a26ec8e26bcfd733dba753235c218bf493636d61`), `go1.27.3-X:simd`, Linux kernel `6.18.63-gentoo-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, or kernel differs.
- **CPU affinity**: goblin `6`; Python + CBC `6`; Python + SciPy `6`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 8.06 ms median on a 1-variable MILP and 7.52 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

Read the synthetic corpus ratios with three methodological notes. First, Tier 1 micro-models (`N <= 26`) are dominated by Python's CBC subprocess spawn floor reported in the provenance block above. Second, the planted-gap MAD family has a zero root LP gap by construction and closes at the root without branching. Consequently, the bold row (`Tier 2+3 non-planted MILPs`) isolates solver-dominated MILPs across paired randomized instances, alongside total subset wall times and full per-instance `[Min-Max]` ranges.

### 1. Aggregate Synthetic Workload Summary (Paired Per-Instance Geometric Mean and Wall-Clock Sums)

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `Py / Go` GeoMean [Clustered 95% CI] | Pooled Trial `Py / Go` | Equal-Family `Go / CBC` | `Go` Trial (Med) Sum (ms) | `CBC` Trial (Med) Sum (ms) | `Py` Trial (Med) Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **360.89x** [124.63x-1044.99x] | 360.89x | 0.06x | 41.1 (3.6) | 212.5 (20.8) | 1452.9 (134.1) | 630.33x [5.46x-2913.22x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **7.29x** [3.73x-14.23x] | 7.29x | 0.40x | 3083.7 (206.3) | 2304.5 (193.6) | 5485.9 (500.9) | 7.80x [0.05x-146.06x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **3.54x** [1.19x-10.50x] | 3.54x | 0.56x | 19772.5 (808.8) | 4019.2 (747.5) | 8487.6 (1443.8) | 4.92x [0.02x-32.49x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **3.72x** [1.87x-7.41x] | 4.31x | 0.69x | 22047.5 (915.4) | 3838.5 (475.4) | 8932.5 (1277.7) | 4.61x [0.02x-146.06x] |
| Tier 3 non-MAD MILPs | 4 (20) | **1.68x** [0.16x-17.27x] | 1.68x | 1.34x | 18985.3 (710.8) | 1645.1 (291.2) | 3878.0 (813.7) | 1.48x [0.02x-13.48x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **16.20x** [6.50x-40.38x] | 21.67x | 0.17x | 810.2 (99.9) | 2511.3 (468.4) | 5203.8 (683.5) | 18.21x [3.11x-139.33x] |
| Random-weight MAD (negative controls) | 3 (33) | **3.82x** [0.15x-95.73x] | 3.82x | 0.70x | 1596.5 (157.4) | 1401.4 (129.9) | 2922.1 (286.5) | 3.08x [0.25x-77.60x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **23.42x** [7.35x-74.65x] | 30.87x | 0.26x | 22087.2 (918.9) | 4024.9 (493.6) | 10222.5 (1395.3) | 18.88x [0.02x-2913.22x] |
| All 31 synthetic corpus families | 31 (293) | **21.29x** [8.95x-50.63x] | 28.57x | 0.23x | 22897.4 (1018.7) | 6536.2 (962.0) | 15426.3 (2078.9) | 18.76x [0.02x-2913.22x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2. Synthetic MAD Group-Item Allocation Family (`G` Groups $\times$ `I` Items, $T = 6$ Periods)

The synthetic `MAD` group-item allocation family ([`BuildMADFromWeights`](bench/corpus.go)) assigns $I$ items across $G$ groups over $T = 6$ periods to minimize total L1 distance from each group's central level $m_g$ plus the roundness penalty $r_g = |m_g - 100 n_g|$ relative to an integer multiple $n_g \in \mathbb{Z}_{\ge 1}$. In the planted-gap generator, items in $S = \{0, \dots, I-G-1\}$ carry orthogonal curve signatures cancelled by the $G$ designated items $D = \{I-G, \dots, I-1\}$ with roundness-aligned period sums, yielding a zero root LP gap that root heuristics close without branching; unstructured random-weight negative controls (`BuildMADRandom`: `t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`) are included directly in the corpus and detailed in [`docs/benchmarks.md`](docs/benchmarks.md).

<!-- BEGIN GENERATED:MAD_PIPELINE_TABLE -->
| Problem ID | $G \times I$ | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Trial 0 Obj | Parity |
| :--- | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **114.6x** [88.9x-139.3x] | 0.13 | 16.42 | 0.10 | 16.18 | 0.030 | 0.494 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **17.0x** [5.5x-77.6x] | 3.24 | 43.87 | 3.19 | 43.61 | 0.044 | 0.349 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **24.8x** [15.5x-41.2x] | 0.42 | 11.37 | 0.36 | 11.05 | 0.071 | 0.416 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **42.5x** [29.2x-60.4x] | 0.26 | 12.12 | 0.20 | 11.68 | 0.060 | 0.583 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **12.7x** [7.5x-16.3x] | 0.99 | 13.53 | 0.82 | 12.88 | 0.186 | 0.859 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **2.0x** [0.7x-6.4x] | 24.10 | 50.40 | 23.97 | 50.09 | 0.094 | 0.417 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **1.6x** [0.2x-4.7x] | 130.10 | 192.26 | 129.92 | 191.82 | 0.121 | 1.013 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **16.8x** [9.8x-32.5x] | 1.33 | 22.53 | 0.93 | 21.24 | 0.263 | 1.683 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **6.9x** [4.0x-10.8x] | 3.04 | 19.93 | 2.70 | 18.11 | 0.526 | 2.330 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **4.8x** [3.3x-8.2x] | 18.83 | 87.46 | 16.19 | 74.62 | 1.992 | 20.478 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **5.6x** [3.1x-9.0x] | 74.86 | 500.16 | 69.77 | 434.20 | 5.134 | 54.538 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

Cross-instance total latency dispersion across the randomized instances in each family (mean $\pm$ sample SD and two-sided 95% Student-$t$ interval after warmup):

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.13 \pm 0.04$ ($\pm 0.03$) | $14.80 \pm 3.48$ ($\pm 2.34$) | $0.13 \pm 0.03$ ($\pm 0.02$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $3.33 \pm 2.09$ ($\pm 1.40$) | $41.90 \pm 12.75$ ($\pm 8.57$) | $3.10 \pm 1.96$ ($\pm 1.32$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.52 \pm 0.14$ ($\pm 0.10$) | $12.78 \pm 2.37$ ($\pm 1.59$) | $0.56 \pm 0.15$ ($\pm 0.10$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.30 \pm 0.07$ ($\pm 0.05$) | $12.22 \pm 0.53$ ($\pm 0.36$) | $0.26 \pm 0.05$ ($\pm 0.03$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $1.14 \pm 0.28$ ($\pm 0.19$) | $14.22 \pm 1.84$ ($\pm 1.24$) | $1.34 \pm 0.45$ ($\pm 0.31$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $24.08 \pm 15.97$ ($\pm 10.73$) | $40.48 \pm 19.53$ ($\pm 13.12$) | $24.50 \pm 17.61$ ($\pm 11.83$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $117.74 \pm 73.20$ ($\pm 49.17$) | $183.27 \pm 91.09$ ($\pm 61.19$) | $129.14 \pm 90.41$ ($\pm 60.73$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $1.38 \pm 0.31$ ($\pm 0.38$) | $23.33 \pm 6.08$ ($\pm 7.55$) | $0.93 \pm 0.05$ ($\pm 0.07$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $3.29 \pm 1.28$ ($\pm 1.59$) | $21.32 \pm 3.41$ ($\pm 4.24$) | $3.57 \pm 1.13$ ($\pm 1.40$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $20.09 \pm 6.09$ ($\pm 7.56$) | $93.70 \pm 17.11$ ($\pm 21.24$) | $21.35 \pm 2.45$ ($\pm 3.04$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $132.68 \pm 132.77$ ($\pm 164.83$) | $783.55 \pm 813.38$ ($\pm 1009.78$) | $135.22 \pm 132.15$ ($\pm 164.07$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### 3. Full 31-Family Synthetic Benchmark Corpus (`JSON -> Solution`)

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | **1187.2x** [1022.8x-1295.8x] | 0.01 | 8.20 | 0.00 | 8.13 | 0.003 | 0.102 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **1119.6x** [588.1x-1550.7x] | 0.01 | 7.67 | 0.00 | 7.60 | 0.003 | 0.098 | `OK` |
| `t1_std_mip` | 3 | 3 | **635.0x** [102.4x-1008.6x] | 0.01 | 8.79 | 0.01 | 8.71 | 0.004 | 0.102 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **661.2x** [267.0x-1172.5x] | 0.01 | 9.37 | 0.01 | 9.29 | 0.005 | 0.116 | `OK` |
| `t1_whiskas` | 6 | 5 | **582.2x** [172.6x-905.8x] | 0.02 | 11.62 | 0.01 | 11.48 | 0.008 | 0.194 | `OK` |
| `t1_furniture` | 2 | 3 | **284.9x** [53.9x-792.6x] | 0.02 | 9.37 | 0.02 | 9.30 | 0.003 | 0.094 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **87.8x** [32.7x-260.2x] | 0.12 | 10.94 | 0.11 | 10.82 | 0.011 | 0.144 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **2372.9x** [1809.8x-2913.2x] | 0.00 | 7.85 | 0.00 | 7.79 | 0.003 | 0.091 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **114.6x** [88.9x-139.3x] | 0.13 | 16.42 | 0.10 | 16.18 | 0.030 | 0.494 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **17.0x** [5.5x-77.6x] | 3.24 | 43.87 | 3.19 | 43.61 | 0.044 | 0.349 | `OK` |
| `t2_mkp_30` | 30 | 6 | **4.6x** [1.9x-7.8x] | 7.34 | 39.39 | 7.29 | 39.20 | 0.043 | 0.401 | `OK` |
| `t2_mkp_50` | 50 | 8 | **1.6x** [0.8x-25.1x] | 21.55 | 19.73 | 21.45 | 19.44 | 0.082 | 0.379 | `OK` |
| `t2_cflp_36` | 36 | 11 | **17.5x** [1.8x-146.1x] | 0.53 | 11.23 | 0.48 | 11.03 | 0.034 | 0.253 | `OK` |
| `t2_cflp_49` | 49 | 13 | **11.1x** [4.1x-27.8x] | 0.99 | 12.24 | 0.93 | 12.01 | 0.047 | 0.301 | `OK` |
| `t2_gap_45` | 45 | 13 | **4.4x** [0.7x-21.1x] | 3.62 | 20.16 | 3.56 | 19.94 | 0.057 | 0.286 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **4.0x** [1.8x-11.7x] | 14.74 | 89.13 | 14.66 | 88.85 | 0.084 | 0.343 | `OK` |
| `t2_binpack_44` | 44 | 14 | **5.3x** [0.1x-19.0x] | 1.37 | 16.43 | 1.28 | 16.19 | 0.051 | 0.300 | `OK` |
| `t2_setcover_50` | 50 | 20 | **27.5x** [1.8x-89.5x] | 0.35 | 12.96 | 0.29 | 12.63 | 0.080 | 0.442 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **24.8x** [15.5x-41.2x] | 0.42 | 11.37 | 0.36 | 11.05 | 0.071 | 0.416 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **42.5x** [29.2x-60.4x] | 0.26 | 12.12 | 0.20 | 11.68 | 0.060 | 0.583 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **12.7x** [7.5x-16.3x] | 0.99 | 13.53 | 0.82 | 12.88 | 0.186 | 0.859 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **2.0x** [0.7x-6.4x] | 24.10 | 50.40 | 23.97 | 50.09 | 0.094 | 0.417 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **1.6x** [0.2x-4.7x] | 130.10 | 192.26 | 129.92 | 191.82 | 0.121 | 1.013 | `OK` |
| `t3_mkp_200` | 200 | 8 | **0.8x** [0.7x-1.0x] | 173.53 | 121.97 | 173.20 | 121.28 | 0.204 | 0.903 | `OK` |
| `t3_cflp_300` | 300 | 34 | **9.0x** [5.3x-13.5x] | 32.12 | 379.91 | 31.89 | 378.91 | 0.178 | 1.292 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **0.3x** [0.0x-1.0x] | 473.01 | 211.62 | 472.70 | 209.65 | 0.336 | 3.808 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **3.2x** [2.0x-5.1x] | 32.09 | 100.24 | 31.39 | 94.46 | 0.466 | 6.033 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **16.8x** [9.8x-32.5x] | 1.33 | 22.53 | 0.93 | 21.24 | 0.263 | 1.683 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **6.9x** [4.0x-10.8x] | 3.04 | 19.93 | 2.70 | 18.11 | 0.526 | 2.330 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **4.8x** [3.3x-8.2x] | 18.83 | 87.46 | 16.19 | 74.62 | 1.992 | 20.478 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **5.6x** [3.1x-9.0x] | 74.86 | 500.16 | 69.77 | 434.20 | 5.134 | 54.538 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

### 4. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus ([`bench/assign_corpus.go`](bench/assign_corpus.go), 200 randomized instances across square, rectangular wide/tall, degenerate integer-tie, Euclidean geometric, rank-1 outer-product, sparse `+Inf`, and maximization families) compares [`assign.LinearSumFlat`](assign/assign.go) against SciPy's C++ `rectangular_lsap` extension (`scipy.optimize.linear_sum_assignment`) in warm state on bit-identical `float64` cost matrices:

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `SciPy Solve / goblin` GeoMean [Clustered 95% CI] | Pooled Trial `SciPy Solve / goblin` | Equal-Family `SciPy Total / goblin` | `goblin` Solve Trial (Med) Sum (ms) | SciPy Solve Trial (Med) Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **2.31x** [1.92x-2.77x] | 2.31x | 7.46x | 0.211 (0.0166) | 0.461 (0.0420) | 2.35x [0.56x-4.76x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **4.45x** [2.82x-7.01x] | 4.45x | 4.46x | 8.911 (0.786) | 47.629 (4.297) | 4.07x [1.55x-19.66x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **3.73x** [2.70x-5.16x] | 3.73x | 4.73x | 325.979 (59.628) | 1422.781 (284.864) | 3.71x [2.11x-8.21x] |
| Square families ($m = n$) | 16 (146) | **3.84x** [2.89x-5.10x] | 3.85x | 5.51x | 330.347 (59.576) | 1454.134 (286.216) | 3.72x [0.60x-19.66x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **2.79x** [1.96x-3.98x] | 2.62x | 4.56x | 4.754 (0.855) | 16.737 (2.987) | 2.99x [0.56x-4.97x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **3.42x** [1.73x-6.78x] | 3.66x | 4.40x | 35.229 (6.180) | 95.179 (15.403) | 2.78x [0.84x-19.66x] |
| **All 22 synthetic LSAP families** | 22 (200) | **3.52x** [2.82x-4.39x] | 3.47x | 5.23x | 335.101 (60.430) | 1470.871 (289.203) | 3.27x [0.56x-19.66x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **2.42x** | 0.0006 | 0.0016 | 0.0007 | 0.0229 | $0.0008 \pm 0.0007$ | $0.0017 \pm 0.0004$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **2.57x** | 0.0014 | 0.0036 | 0.0017 | 0.0191 | $0.0014 \pm 0.0003$ | $0.0036 \pm 0.0006$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **2.65x** | 0.0029 | 0.0090 | 0.0046 | 0.0265 | $0.0037 \pm 0.0023$ | $0.0088 \pm 0.0019$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.89x** | 0.0031 | 0.0059 | 0.0048 | 0.0250 | $0.0033 \pm 0.0010$ | $0.0060 \pm 0.0010$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.81x** | 0.0034 | 0.0065 | 0.0046 | 0.0259 | $0.0040 \pm 0.0024$ | $0.0066 \pm 0.0010$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **2.66x** | 0.0052 | 0.0154 | 0.0092 | 0.0538 | $0.0060 \pm 0.0023$ | $0.0152 \pm 0.0020$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **3.72x** | 0.0262 | 0.0986 | 0.0332 | 0.138 | $0.0269 \pm 0.0057$ | $0.0989 \pm 0.0148$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **4.91x** | 0.0526 | 0.254 | 0.0717 | 0.327 | $0.0513 \pm 0.0044$ | $0.254 \pm 0.0381$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **3.17x** | 0.0458 | 0.134 | 0.0704 | 0.205 | $0.0442 \pm 0.0089$ | $0.139 \pm 0.0204$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **2.37x** | 0.137 | 0.309 | 0.145 | 0.375 | $0.143 \pm 0.0445$ | $0.331 \pm 0.0632$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **18.02x** | 0.0562 | 1.044 | 0.0743 | 1.170 | $0.0582 \pm 0.0065$ | $1.044 \pm 0.0252$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **3.45x** | 0.0317 | 0.106 | 0.0558 | 0.211 | $0.0309 \pm 0.0034$ | $0.106 \pm 0.0089$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.94x** | 0.0380 | 0.118 | 0.0626 | 0.231 | $0.0422 \pm 0.0106$ | $0.122 \pm 0.0121$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **4.90x** | 0.0821 | 0.402 | 0.109 | 0.527 | $0.0832 \pm 0.0075$ | $0.409 \pm 0.0482$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **5.56x** | 0.317 | 1.833 | 0.428 | 2.512 | $0.330 \pm 0.0497$ | $1.827 \pm 0.188$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.76x** | 1.215 | 5.677 | 1.633 | 9.173 | $1.256 \pm 0.134$ | $5.990 \pm 0.779$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **2.38x** | 0.784 | 1.902 | 1.349 | 5.335 | $0.775 \pm 0.0503$ | $1.856 \pm 0.231$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **2.43x** | 5.149 | 11.990 | 5.428 | 14.452 | $5.709 \pm 1.944$ | $13.797 \pm 4.572$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **4.17x** | 0.318 | 1.263 | 0.777 | 5.491 | $0.317 \pm 0.0246$ | $1.325 \pm 0.161$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **3.27x** | 0.460 | 1.488 | 0.859 | 5.895 | $0.457 \pm 0.0237$ | $1.494 \pm 0.0387$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **6.06x** | 6.735 | 43.854 | 8.877 | 57.750 | $6.720 \pm 0.361$ | $41.428 \pm 8.349$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.43x** | 44.966 | 218.690 | 55.004 | 279.255 | $49.962 \pm 10.389$ | $218.666 \pm 24.285$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

---

## Pure-Go vs. C++ CBC Performance Gaps and MAD Scaling Analysis

Comparing isolated unpresolved `Root LP` timings, paired trial ratios, and solver-core checkpoints (`CP5 +Presolve`, `CP6 +Cuts`, `CP7 Full`) between `BackendGo` and in-process `BackendCBC` (`c1c171ec`) highlights four structural trade-offs on large degenerate LPs and heavy-tail branch-and-cut models.

### 1. MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families (`N = G(I + T + 3)` variables, `M = G(2T + 3) + I` constraints) separate solver performance into three distinct scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Paired `Go / CBC` | Go Total Med (ms) | CBC Total Med (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.05x** | 0.13 | 2.63 | 0.77x | 0.10 (1 / 19) | 2.59 (0 / 0) | 0.111 (28) | 0.145 (19) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.23x** | 3.24 | 12.29 | 0.80x | 3.19 (10 / 232) | 12.24 (6 / 83) | 0.127 (30) | 0.159 (18) |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.16x** | 0.42 | 2.94 | 0.88x | 0.36 (1 / 26) | 2.85 (0 / 0) | 0.210 (33) | 0.240 (21) |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.10x** | 0.26 | 2.83 | 1.20x | 0.20 (1 / 39) | 2.74 (0 / 0) | 0.332 (59) | 0.277 (40) |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.28x** | 0.99 | 3.65 | 1.68x | 0.82 (1 / 51) | 3.53 (0 / 0) | 0.709 (68) | 0.422 (63) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **1.20x** | 24.10 | 17.45 | 0.93x | 23.97 (65 / 714) | 17.40 (12 / 216) | 0.222 (38) | 0.239 (24) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **1.27x** | 130.10 | 100.14 | 1.98x | 129.92 (254 / 3052) | 100.06 (198 / 1300) | 0.600 (66) | 0.304 (54) |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.12x** | 1.33 | 10.05 | 1.00x | 0.93 (1 / 75) | 9.87 (0 / 0) | 1.092 (132) | 1.096 (94) |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.23x** | 3.04 | 13.56 | 1.44x | 2.70 (1 / 105) | 13.10 (0 / 0) | 2.524 (423) | 1.748 (149) |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **0.33x** | 18.83 | 58.37 | 3.03x | 16.19 (2 / 357) | 55.89 (0 / 0) | 19.578 (1031) | 6.453 (319) |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **0.31x** | 74.86 | 374.38 | 4.11x | 69.77 (1 / 2439) | 368.23 (0 / 0) | 181.752 (1864) | 44.213 (3128) |
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
| `t3_mkp_200` | $200 \times 8$ | **2.34x** | $157.69 \pm 83.35$ | $61.10 \pm 27.37$ | 173.20 (590 / 5308) | 63.79 (154 / 1145) | 202.35 (701 / 12) | 250.06 (2040, `TimeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **1.37x** | $26.94 \pm 26.11$ | $15.12 \pm 13.09$ | 21.45 (1 / 24) | 9.51 (0 / 8) | 36.35 (2 / 4) | 15.10 (132) |
| `t2_binpack_44` | $44 \times 14$ | **0.46x** | $64.30 \pm 186.41$ | $8.25 \pm 3.91$ | 1.28 (2 / 69) | 6.31 (0 / 8) | 2.36 (6 / 7) | 71.63 (1848) |
| `t3_cflp_300` | $300 \times 34$ | **0.56x** | $38.56 \pm 22.06$ | $63.14 \pm 14.55$ | 31.89 (38 / 1551) | 59.44 (18 / 483) | 39.91 (38 / 31) | 24.56 (134) |
| `t3_lotsizing_504` | $504 \times 336$ | **5.34x** | $3566.20 \pm 7280.96$ | $131.80 \pm 92.59$ | 472.70 (35 / 1615) | 101.38 (10 / 377) | 250.44 (242 / 205, `TimeLimit`) | 250.23 (421, `TimeLimit`) |
| `t3_sched_1000` | $1000 \times 100$ | **0.46x** | $34.63 \pm 10.28$ | $72.97 \pm 14.77$ | 31.39 (64 / 507) | 65.61 (0 / 76) | 93.79 (206 / 46) | 250.17 (618, `TimeLimit`) |
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
