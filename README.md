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
- **Unified telemetry source**: fingerprint `8870ee172dd85c23` (working tree on top of commit `c15436234b905cfb89496cd75752329cd42b129d`), `go1.27.3-X:simd`, Linux kernel `6.18.63-gentoo-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, or kernel differs.
- **CPU affinity**: goblin `6`; Python + CBC `6`; Python + SciPy `6`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.66 ms median on a 1-variable MILP and 6.47 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

Read the synthetic corpus ratios with three methodological notes. First, Tier 1 micro-models (`N <= 26`) are dominated by Python's CBC subprocess spawn floor reported in the provenance block above. Second, the planted-gap MAD family has a zero root LP gap by construction and closes at the root without branching. Consequently, the bold row (`Tier 2+3 non-planted MILPs`) isolates solver-dominated MILPs across paired randomized instances, alongside total subset wall times and full per-instance `[Min-Max]` ranges.

### 1. Aggregate Synthetic Workload Summary (Paired Per-Instance Geometric Mean and Wall-Clock Sums)

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `Py / Go` GeoMean [Clustered 95% CI] | Pooled Trial `Py / Go` | Equal-Family `Go / CBC` | `Go` Trial (Med) Sum (ms) | `CBC` Trial (Med) Sum (ms) | `Py` Trial (Med) Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **283.65x** [122.02x-659.40x] | 283.65x | 0.06x | 20.1 (1.9) | 198.0 (19.5) | 1119.7 (105.4) | 413.05x [10.66x-2170.48x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **12.37x** [6.79x-22.56x] | 12.37x | 0.25x | 1346.8 (70.7) | 1991.3 (176.1) | 4444.5 (428.1) | 15.53x [0.09x-151.42x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **4.62x** [1.75x-12.18x] | 4.62x | 0.45x | 8844.7 (332.5) | 2940.1 (567.6) | 6259.3 (1077.5) | 5.01x [0.03x-27.94x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **6.07x** [3.20x-11.54x] | 7.24x | 0.43x | 9686.3 (339.1) | 3164.3 (401.7) | 6907.8 (1000.4) | 6.46x [0.03x-151.42x] |
| Tier 3 non-MAD MILPs | 4 (20) | **2.34x** [0.35x-15.61x] | 2.34x | 0.92x | 8350.5 (269.3) | 1262.1 (233.3) | 2827.1 (604.8) | 2.36x [0.03x-14.41x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **20.41x** [8.57x-48.61x] | 27.62x | 0.14x | 506.5 (64.2) | 1792.3 (344.5) | 3907.1 (515.4) | 31.95x [3.33x-106.90x] |
| Random-weight MAD (negative controls) | 3 (33) | **7.83x** [0.54x-114.08x] | 7.83x | 0.30x | 672.3 (46.2) | 1239.0 (120.5) | 2288.0 (252.0) | 8.02x [0.71x-75.81x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **28.81x** [10.78x-76.99x] | 37.47x | 0.20x | 9705.1 (340.9) | 3337.0 (418.7) | 7916.5 (1095.6) | 28.83x [0.03x-2170.48x] |
| All 31 synthetic corpus families | 31 (293) | **26.36x** [12.58x-55.21x] | 35.06x | 0.18x | 10211.6 (405.1) | 5129.4 (763.2) | 11823.6 (1610.9) | 29.79x [0.03x-2170.48x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2. Synthetic MAD Group-Item Allocation Family (`G` Groups $\times$ `I` Items, $T = 6$ Periods)

The synthetic `MAD` group-item allocation family ([`BuildMADFromWeights`](bench/corpus.go)) assigns $I$ items across $G$ groups over $T = 6$ periods to minimize total L1 distance from each group's central level $m_g$ plus the roundness penalty $r_g = |m_g - 100 n_g|$ relative to an integer multiple $n_g \in \mathbb{Z}_{\ge 1}$. In the planted-gap generator, items in $S = \{0, \dots, I-G-1\}$ carry orthogonal curve signatures cancelled by the $G$ designated items $D = \{I-G, \dots, I-1\}$ with roundness-aligned period sums, yielding a zero root LP gap that root heuristics close without branching; unstructured random-weight negative controls (`BuildMADRandom`: `t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`) are included directly in the corpus and detailed in [`docs/benchmarks.md`](docs/benchmarks.md).

<!-- BEGIN GENERATED:MAD_PIPELINE_TABLE -->
| Problem ID | $G \times I$ | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Trial 0 Obj | Parity |
| :--- | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **84.5x** [67.5x-106.9x] | 0.12 | 10.14 | 0.08 | 9.82 | 0.046 | 0.295 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **26.4x** [10.7x-75.8x] | 1.63 | 34.59 | 1.59 | 34.36 | 0.037 | 0.301 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **50.3x** [33.9x-65.4x] | 0.18 | 9.92 | 0.15 | 9.64 | 0.037 | 0.385 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **45.6x** [29.3x-55.8x] | 0.22 | 10.78 | 0.15 | 10.37 | 0.062 | 0.541 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **22.6x** [13.0x-31.5x] | 0.48 | 11.77 | 0.39 | 11.17 | 0.113 | 0.797 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **5.4x** [2.2x-15.8x] | 7.20 | 48.53 | 7.12 | 48.23 | 0.048 | 0.394 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **3.4x** [0.7x-13.3x] | 37.42 | 168.88 | 37.32 | 168.45 | 0.080 | 0.565 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **22.0x** [17.7x-27.9x] | 0.79 | 18.60 | 0.59 | 17.47 | 0.184 | 1.412 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **11.1x** [7.4x-16.9x] | 2.13 | 19.61 | 1.70 | 17.88 | 0.295 | 2.291 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **4.6x** [4.0x-5.0x] | 13.75 | 61.85 | 12.44 | 54.18 | 1.176 | 10.950 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **6.2x** [3.3x-8.2x] | 46.54 | 372.69 | 41.83 | 337.85 | 4.578 | 45.389 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

Cross-instance total latency dispersion across the randomized instances in each family (mean $\pm$ sample SD and two-sided 95% Student-$t$ interval after warmup):

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.12 \pm 0.02$ ($\pm 0.01$) | $10.10 \pm 0.54$ ($\pm 0.36$) | $0.15 \pm 0.09$ ($\pm 0.06$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $1.48 \pm 0.87$ ($\pm 0.59$) | $31.41 \pm 9.59$ ($\pm 6.44$) | $1.51 \pm 0.89$ ($\pm 0.60$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.21 \pm 0.04$ ($\pm 0.03$) | $10.15 \pm 0.77$ ($\pm 0.52$) | $0.21 \pm 0.05$ ($\pm 0.03$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.24 \pm 0.06$ ($\pm 0.04$) | $10.72 \pm 0.22$ ($\pm 0.15$) | $0.23 \pm 0.05$ ($\pm 0.03$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.55 \pm 0.13$ ($\pm 0.09$) | $12.21 \pm 0.95$ ($\pm 0.64$) | $0.53 \pm 0.07$ ($\pm 0.05$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $8.25 \pm 6.13$ ($\pm 4.12$) | $36.26 \pm 17.18$ ($\pm 11.54$) | $8.19 \pm 6.19$ ($\pm 4.16$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $51.38 \pm 44.70$ ($\pm 30.03$) | $140.33 \pm 63.80$ ($\pm 42.86$) | $51.63 \pm 44.81$ ($\pm 30.11$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.83 \pm 0.10$ ($\pm 0.13$) | $18.31 \pm 2.26$ ($\pm 2.81$) | $0.87 \pm 0.13$ ($\pm 0.16$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.86 \pm 0.63$ ($\pm 0.78$) | $19.59 \pm 0.35$ ($\pm 0.43$) | $1.83 \pm 0.61$ ($\pm 0.76$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $14.01 \pm 1.07$ ($\pm 1.32$) | $63.84 \pm 3.63$ ($\pm 4.51$) | $14.25 \pm 1.14$ ($\pm 1.41$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $82.15 \pm 68.83$ ($\pm 85.45$) | $584.70 \pm 618.21$ ($\pm 767.48$) | $83.69 \pm 69.59$ ($\pm 86.39$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### 3. Full 31-Family Synthetic Benchmark Corpus (`JSON -> Solution`)

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | **777.6x** [388.9x-1077.3x] | 0.01 | 6.64 | 0.00 | 6.57 | 0.005 | 0.074 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **743.1x** [578.6x-1030.4x] | 0.01 | 6.73 | 0.00 | 6.67 | 0.005 | 0.079 | `OK` |
| `t1_std_mip` | 3 | 3 | **380.9x** [188.5x-552.8x] | 0.02 | 7.82 | 0.01 | 7.76 | 0.008 | 0.083 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **337.7x** [256.4x-443.5x] | 0.02 | 7.95 | 0.01 | 7.88 | 0.007 | 0.088 | `OK` |
| `t1_whiskas` | 6 | 5 | **534.1x** [321.7x-701.0x] | 0.01 | 6.70 | 0.00 | 6.61 | 0.007 | 0.104 | `OK` |
| `t1_furniture` | 2 | 3 | **297.9x** [69.6x-795.4x] | 0.02 | 8.37 | 0.02 | 8.31 | 0.003 | 0.084 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **108.5x** [45.1x-311.4x] | 0.10 | 9.86 | 0.09 | 9.76 | 0.009 | 0.123 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **1177.9x** [628.1x-2170.5x] | 0.01 | 6.57 | 0.00 | 6.52 | 0.005 | 0.065 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **84.5x** [67.5x-106.9x] | 0.12 | 10.14 | 0.08 | 9.82 | 0.046 | 0.295 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **26.4x** [10.7x-75.8x] | 1.63 | 34.59 | 1.59 | 34.36 | 0.037 | 0.301 | `OK` |
| `t2_mkp_30` | 30 | 6 | **4.5x** [2.8x-8.4x] | 4.63 | 28.72 | 4.60 | 28.54 | 0.029 | 0.234 | `OK` |
| `t2_mkp_50` | 50 | 8 | **2.7x** [1.1x-22.6x] | 10.14 | 17.78 | 10.10 | 17.52 | 0.048 | 0.349 | `OK` |
| `t2_cflp_36` | 36 | 11 | **20.9x** [1.7x-151.4x] | 0.44 | 9.91 | 0.41 | 9.74 | 0.032 | 0.227 | `OK` |
| `t2_cflp_49` | 49 | 13 | **16.2x** [6.5x-34.2x] | 0.61 | 10.77 | 0.58 | 10.55 | 0.032 | 0.278 | `OK` |
| `t2_gap_45` | 45 | 13 | **13.7x** [1.4x-67.0x] | 1.10 | 14.54 | 1.08 | 14.33 | 0.026 | 0.264 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **7.6x** [3.5x-18.7x] | 7.46 | 69.68 | 7.43 | 69.44 | 0.038 | 0.315 | `OK` |
| `t2_binpack_44` | 44 | 14 | **9.8x** [0.1x-36.5x] | 0.57 | 14.21 | 0.55 | 13.93 | 0.033 | 0.266 | `OK` |
| `t2_setcover_50` | 50 | 20 | **40.7x** [2.9x-115.4x] | 0.23 | 12.59 | 0.16 | 12.28 | 0.049 | 0.421 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **50.3x** [33.9x-65.4x] | 0.18 | 9.92 | 0.15 | 9.64 | 0.037 | 0.385 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **45.6x** [29.3x-55.8x] | 0.22 | 10.78 | 0.15 | 10.37 | 0.062 | 0.541 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **22.6x** [13.0x-31.5x] | 0.48 | 11.77 | 0.39 | 11.17 | 0.113 | 0.797 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **5.4x** [2.2x-15.8x] | 7.20 | 48.53 | 7.12 | 48.23 | 0.048 | 0.394 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **3.4x** [0.7x-13.3x] | 37.42 | 168.88 | 37.32 | 168.45 | 0.080 | 0.565 | `OK` |
| `t3_mkp_200` | 200 | 8 | **1.2x** [1.0x-1.3x] | 92.10 | 116.12 | 92.00 | 115.52 | 0.111 | 0.804 | `OK` |
| `t3_cflp_300` | 300 | 34 | **9.5x** [6.1x-14.4x] | 22.16 | 249.58 | 22.00 | 248.66 | 0.144 | 1.176 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **0.7x** [0.0x-2.0x] | 135.29 | 157.44 | 135.02 | 155.51 | 0.269 | 2.291 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **4.0x** [2.7x-5.0x] | 19.75 | 81.63 | 19.38 | 78.69 | 0.363 | 3.492 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **22.0x** [17.7x-27.9x] | 0.79 | 18.60 | 0.59 | 17.47 | 0.184 | 1.412 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **11.1x** [7.4x-16.9x] | 2.13 | 19.61 | 1.70 | 17.88 | 0.295 | 2.291 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **4.6x** [4.0x-5.0x] | 13.75 | 61.85 | 12.44 | 54.18 | 1.176 | 10.950 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **6.2x** [3.3x-8.2x] | 46.54 | 372.69 | 41.83 | 337.85 | 4.578 | 45.389 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

### 4. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus ([`bench/assign_corpus.go`](bench/assign_corpus.go), 200 randomized instances across square, rectangular wide/tall, degenerate integer-tie, Euclidean geometric, rank-1 outer-product, sparse `+Inf`, and maximization families) compares [`assign.LinearSumFlat`](assign/assign.go) against SciPy's C++ `rectangular_lsap` extension (`scipy.optimize.linear_sum_assignment`) in warm state on bit-identical `float64` cost matrices:

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `SciPy Solve / goblin` GeoMean [Clustered 95% CI] | Pooled Trial `SciPy Solve / goblin` | Equal-Family `SciPy Total / goblin` | `goblin` Solve Trial (Med) Sum (ms) | SciPy Solve Trial (Med) Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.89x** [1.40x-2.56x] | 1.89x | 5.14x | 0.186 (0.0150) | 0.797 (0.0288) | 1.78x [0.65x-381.54x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **3.31x** [2.00x-5.46x] | 3.31x | 3.20x | 8.576 (0.735) | 32.804 (2.949) | 2.99x [0.58x-19.83x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **3.09x** [2.07x-4.63x] | 3.09x | 3.91x | 214.180 (42.060) | 938.177 (186.652) | 3.12x [1.23x-6.48x] |
| Square families ($m = n$) | 16 (146) | **3.02x** [2.20x-4.15x] | 3.00x | 4.08x | 219.061 (42.109) | 960.930 (187.683) | 2.69x [0.58x-381.54x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **2.23x** [1.61x-3.08x] | 2.09x | 3.40x | 3.881 (0.702) | 10.848 (1.947) | 2.28x [0.80x-3.32x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.70x** [1.28x-5.70x] | 2.91x | 3.26x | 34.696 (5.962) | 65.832 (11.148) | 2.07x [0.58x-381.54x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.78x** [2.19x-3.54x] | 2.72x | 3.88x | 222.942 (42.811) | 971.778 (189.630) | 2.52x [0.58x-381.54x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **2.02x** | 0.0005 | 0.0010 | 0.0006 | 0.0139 | $0.0005 \pm 0.0001$ | $0.0011 \pm 0.0003$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.58x** | 0.0015 | 0.0023 | 0.0019 | 0.0101 | $0.0017 \pm 0.0009$ | $0.0024 \pm 0.0004$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **3.21x** | 0.0026 | 0.0062 | 0.0044 | 0.0151 | $0.0032 \pm 0.0014$ | $0.0499 \pm 0.144$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.47x** | 0.0026 | 0.0040 | 0.0042 | 0.0144 | $0.0028 \pm 0.0011$ | $0.0039 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.60x** | 0.0028 | 0.0044 | 0.0044 | 0.0149 | $0.0029 \pm 0.0011$ | $0.0045 \pm 0.0009$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.93x** | 0.0050 | 0.0109 | 0.0092 | 0.0326 | $0.0058 \pm 0.0025$ | $0.0106 \pm 0.0020$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.83x** | 0.0240 | 0.0702 | 0.0313 | 0.0985 | $0.0251 \pm 0.0081$ | $0.0688 \pm 0.0104$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.56x** | 0.0483 | 0.175 | 0.0635 | 0.224 | $0.0501 \pm 0.0101$ | $0.177 \pm 0.0254$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.17x** | 0.0426 | 0.0937 | 0.0648 | 0.142 | $0.0458 \pm 0.0137$ | $0.0965 \pm 0.0142$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.56x** | 0.132 | 0.219 | 0.140 | 0.257 | $0.158 \pm 0.0676$ | $0.233 \pm 0.0430$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **15.58x** | 0.0486 | 0.686 | 0.0642 | 0.760 | $0.0457 \pm 0.0070$ | $0.707 \pm 0.0623$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **2.52x** | 0.0271 | 0.0693 | 0.0510 | 0.146 | $0.0297 \pm 0.0119$ | $0.0716 \pm 0.0077$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.36x** | 0.0328 | 0.0801 | 0.0578 | 0.154 | $0.0350 \pm 0.0102$ | $0.0804 \pm 0.0068$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.68x** | 0.0760 | 0.280 | 0.0989 | 0.361 | $0.0776 \pm 0.0089$ | $0.285 \pm 0.0335$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **4.07x** | 0.304 | 1.276 | 0.424 | 1.780 | $0.313 \pm 0.0534$ | $1.263 \pm 0.123$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.84x** | 1.170 | 5.166 | 1.580 | 7.902 | $1.438 \pm 0.476$ | $5.329 \pm 0.346$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.89x** | 0.722 | 1.480 | 1.273 | 4.028 | $0.804 \pm 0.225$ | $1.482 \pm 0.0494$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.69x** | 5.009 | 8.653 | 5.202 | 9.525 | $5.567 \pm 1.983$ | $9.271 \pm 2.618$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **3.20x** | 0.270 | 0.826 | 0.639 | 3.770 | $0.264 \pm 0.0207$ | $0.844 \pm 0.0594$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.73x** | 0.367 | 0.963 | 0.736 | 3.849 | $0.357 \pm 0.0270$ | $0.972 \pm 0.0333$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **5.13x** | 5.858 | 28.301 | 7.543 | 38.371 | $5.629 \pm 0.380$ | $28.885 \pm 2.501$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.92x** | 28.665 | 141.263 | 38.715 | 186.836 | $28.777 \pm 3.776$ | $140.852 \pm 8.948$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

---

## Pure-Go vs. C++ CBC Performance Gaps and MAD Scaling Analysis

Comparing isolated unpresolved `Root LP` timings, paired trial ratios, and solver-core checkpoints (`CP5 +Presolve`, `CP6 +Cuts`, `CP7 Full`) between `BackendGo` and in-process `BackendCBC` (`c1c171ec`) highlights four structural trade-offs on large degenerate LPs and heavy-tail branch-and-cut models.

### 1. MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families (`N = G(I + T + 3)` variables, `M = G(2T + 3) + I` constraints) separate solver performance into three distinct scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Paired `Go / CBC` | Go Total Med (ms) | CBC Total Med (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.05x** | 0.12 | 2.47 | 0.63x | 0.08 (1 / 19) | 2.43 (0 / 0) | 0.087 (28) | 0.139 (19) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.12x** | 1.63 | 11.37 | 0.76x | 1.59 (10 / 232) | 11.32 (6 / 83) | 0.110 (30) | 0.144 (18) |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.10x** | 0.18 | 1.82 | 0.72x | 0.15 (1 / 26) | 1.78 (0 / 0) | 0.107 (33) | 0.148 (21) |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.09x** | 0.22 | 2.51 | 0.98x | 0.15 (1 / 39) | 2.45 (0 / 0) | 0.256 (59) | 0.260 (40) |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.15x** | 0.48 | 3.38 | 0.85x | 0.39 (1 / 51) | 3.29 (0 / 0) | 0.357 (68) | 0.421 (63) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.45x** | 7.20 | 16.90 | 0.91x | 7.12 (71 / 722) | 16.83 (12 / 216) | 0.144 (38) | 0.159 (24) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.52x** | 37.42 | 92.26 | 1.10x | 37.32 (262 / 3126) | 92.16 (198 / 1300) | 0.319 (66) | 0.289 (54) |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.10x** | 0.79 | 8.13 | 1.44x | 0.59 (1 / 75) | 7.96 (0 / 0) | 0.941 (132) | 0.654 (94) |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.21x** | 2.13 | 7.85 | 1.42x | 1.70 (1 / 105) | 7.59 (0 / 0) | 1.541 (423) | 1.086 (149) |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **0.38x** | 13.75 | 37.96 | 3.66x | 12.44 (2 / 374) | 36.33 (0 / 0) | 13.551 (1031) | 3.701 (319) |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **0.28x** | 46.54 | 280.38 | 3.23x | 41.83 (1 / 2439) | 275.56 (0 / 0) | 115.984 (1864) | 35.870 (3128) |
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
| `t3_mkp_200` | $200 \times 8$ | **2.03x** | $86.85 \pm 43.78$ | $40.76 \pm 19.36$ | 92.00 (636 / 5584) | 49.71 (154 / 1145) | 119.44 (701 / 12) | 250.03 (4414, `TimeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **0.85x** | $15.22 \pm 16.93$ | $12.51 \pm 9.15$ | 10.10 (1 / 24) | 8.65 (0 / 8) | 26.75 (2 / 4) | 10.61 (132) |
| `t2_binpack_44` | $44 \times 14$ | **0.23x** | $23.88 \pm 66.33$ | $6.75 \pm 2.18$ | 0.55 (2 / 69) | 6.15 (0 / 8) | 1.72 (6 / 7) | 34.39 (1848) |
| `t3_cflp_300` | $300 \times 34$ | **0.43x** | $26.03 \pm 13.90$ | $55.83 \pm 11.87$ | 22.00 (38 / 1551) | 50.65 (18 / 483) | 21.55 (38 / 31) | 13.23 (134) |
| `t3_lotsizing_504` | $504 \times 336$ | **2.46x** | $1536.22 \pm 3223.48$ | $93.80 \pm 57.78$ | 135.02 (40 / 1582) | 71.54 (10 / 377) | 171.89 (420 / 205) | 250.03 (1694, `TimeLimit`) |
| `t3_sched_1000` | $1000 \times 100$ | **0.33x** | $20.99 \pm 5.27$ | $62.03 \pm 4.85$ | 19.38 (64 / 507) | 60.54 (0 / 76) | 62.11 (206 / 46) | 250.03 (1370, `TimeLimit`) |
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
