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
- **Unified telemetry source**: fingerprint `5b410dffd8dc05c7` (commit `2b7305d20ada300b230498a89f1e7f64aa619fc4`), `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, or kernel differs.
- **CPU affinity**: goblin `6`; Python + CBC `6`; Python + SciPy `6`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 7.21 ms median on a 1-variable MILP and 6.70 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

Read the synthetic corpus ratios with three methodological notes. First, Tier 1 micro-models (`N <= 26`) are dominated by Python's CBC subprocess spawn floor reported in the provenance block above. Second, the planted-gap MAD family has a zero root LP gap by construction and closes at the root without branching. Consequently, the bold row (`Tier 2+3 non-planted MILPs`) isolates solver-dominated MILPs across paired randomized instances, alongside total subset wall times and full per-instance `[Min-Max]` ranges.

### 1. Aggregate Synthetic Workload Summary (Paired Per-Instance Geometric Mean and Wall-Clock Sums)

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `Py / Go` GeoMean [Clustered 95% CI] | Pooled Trial `Py / Go` | Equal-Family `Go / CBC` | `Go` Trial (Med) Sum (ms) | `CBC` Trial (Med) Sum (ms) | `Py` Trial (Med) Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **419.70x** [184.06x-957.03x] | 419.70x | 0.04x | 9.8 (0.8) | 193.5 (19.0) | 1077.7 (100.8) | 649.99x [31.36x-2522.44x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **18.65x** [11.03x-31.52x] | 18.65x | 0.16x | 755.9 (33.8) | 2010.2 (173.7) | 4323.0 (419.2) | 24.85x [0.15x-161.18x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **8.79x** [3.58x-21.54x] | 8.79x | 0.23x | 3436.6 (164.7) | 2968.2 (577.6) | 6186.9 (1063.7) | 10.85x [0.09x-36.20x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **10.14x** [5.80x-17.73x] | 11.69x | 0.25x | 3936.3 (155.7) | 3204.4 (407.0) | 6774.1 (986.1) | 12.01x [0.09x-161.18x] |
| Tier 3 non-MAD MILPs | 4 (20) | **4.69x** [0.75x-29.44x] | 4.69x | 0.45x | 3188.0 (122.7) | 1283.5 (240.8) | 2805.5 (598.8) | 4.61x [0.09x-25.05x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **31.84x** [15.59x-65.06x] | 40.77x | 0.09x | 257.2 (42.9) | 1798.0 (346.6) | 3839.7 (506.3) | 44.28x [4.26x-146.82x] |
| Random-weight MAD (negative controls) | 3 (33) | **15.89x** [1.01x-249.75x] | 15.89x | 0.15x | 361.6 (17.8) | 1243.8 (117.6) | 2245.8 (247.8) | 13.39x [1.40x-111.50x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **46.14x** [18.00x-118.29x] | 58.64x | 0.12x | 3945.1 (156.4) | 3373.9 (423.7) | 7747.9 (1077.4) | 41.42x [0.09x-2522.44x] |
| All 31 synthetic corpus families | 31 (293) | **41.93x** [20.76x-84.69x] | 54.17x | 0.11x | 4202.3 (199.3) | 5172.0 (770.3) | 11587.6 (1583.7) | 43.11x [0.09x-2522.44x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2. Synthetic MAD Group-Item Allocation Family (`G` Groups $\times$ `I` Items, $T = 6$ Periods)

The synthetic `MAD` group-item allocation family ([`BuildMADFromWeights`](bench/corpus.go)) assigns $I$ items across $G$ groups over $T = 6$ periods to minimize total L1 distance from each group's central level $m_g$ plus the roundness penalty $r_g = |m_g - 100 n_g|$ relative to an integer multiple $n_g \in \mathbb{Z}_{\ge 1}$. In the planted-gap generator, items in $S = \{0, \dots, I-G-1\}$ carry orthogonal curve signatures cancelled by the $G$ designated items $D = \{I-G, \dots, I-1\}$ with roundness-aligned period sums, yielding a zero root LP gap that root heuristics close without branching; unstructured random-weight negative controls (`BuildMADRandom`: `t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`) are included directly in the corpus and detailed in [`docs/benchmarks.md`](docs/benchmarks.md).

<!-- BEGIN GENERATED:MAD_PIPELINE_TABLE -->
| Problem ID | $G \times I$ | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Trial 0 Obj | Parity |
| :--- | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **109.5x** [66.9x-146.8x] | 0.08 | 9.50 | 0.04 | 9.29 | 0.032 | 0.285 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **55.4x** [31.4x-111.5x] | 0.58 | 33.73 | 0.55 | 33.50 | 0.027 | 0.307 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **68.5x** [62.7x-79.6x] | 0.14 | 9.72 | 0.11 | 9.35 | 0.035 | 0.384 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **58.4x** [43.5x-79.7x] | 0.19 | 10.15 | 0.10 | 9.74 | 0.065 | 0.542 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **32.8x** [27.9x-39.5x] | 0.39 | 11.95 | 0.26 | 11.33 | 0.098 | 0.805 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **10.8x** [3.9x-33.4x] | 3.12 | 47.21 | 3.07 | 46.84 | 0.040 | 0.396 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **6.7x** [1.4x-26.0x] | 14.09 | 166.84 | 13.99 | 166.41 | 0.067 | 0.573 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **31.6x** [22.3x-36.2x] | 0.54 | 18.02 | 0.37 | 16.93 | 0.156 | 1.384 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **21.0x** [18.7x-24.5x] | 0.88 | 18.20 | 0.65 | 16.48 | 0.235 | 2.260 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **11.1x** [9.3x-12.7x] | 5.29 | 60.48 | 3.65 | 52.55 | 1.431 | 10.796 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **10.0x** [4.3x-26.0x] | 35.34 | 368.23 | 29.18 | 334.19 | 4.823 | 43.997 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

Cross-instance total latency dispersion across the randomized instances in each family (mean $\pm$ sample SD and two-sided 95% Student-$t$ interval after warmup):

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.09 \pm 0.02$ ($\pm 0.02$) | $9.44 \pm 0.49$ ($\pm 0.33$) | $0.08 \pm 0.01$ ($\pm 0.01$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.64 \pm 0.33$ ($\pm 0.22$) | $30.72 \pm 9.55$ ($\pm 6.42$) | $0.65 \pm 0.34$ ($\pm 0.23$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.14 \pm 0.01$ ($\pm 0.01$) | $9.81 \pm 0.62$ ($\pm 0.42$) | $0.15 \pm 0.02$ ($\pm 0.02$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.18 \pm 0.04$ ($\pm 0.03$) | $10.39 \pm 0.59$ ($\pm 0.39$) | $0.18 \pm 0.04$ ($\pm 0.02$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.37 \pm 0.04$ ($\pm 0.03$) | $12.02 \pm 0.77$ ($\pm 0.52$) | $0.37 \pm 0.04$ ($\pm 0.02$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $4.46 \pm 4.07$ ($\pm 2.74$) | $35.37 \pm 16.90$ ($\pm 11.35$) | $4.42 \pm 3.99$ ($\pm 2.68$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $27.77 \pm 29.49$ ($\pm 19.81$) | $138.08 \pm 63.42$ ($\pm 42.60$) | $27.55 \pm 29.00$ ($\pm 19.48$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.55 \pm 0.04$ ($\pm 0.05$) | $17.53 \pm 2.29$ ($\pm 2.85$) | $0.51 \pm 0.02$ ($\pm 0.02$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $0.88 \pm 0.11$ ($\pm 0.14$) | $18.30 \pm 0.35$ ($\pm 0.44$) | $0.91 \pm 0.10$ ($\pm 0.12$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $5.67 \pm 0.76$ ($\pm 0.94$) | $62.69 \pm 3.62$ ($\pm 4.49$) | $5.24 \pm 1.02$ ($\pm 1.26$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $42.62 \pm 13.13$ ($\pm 16.30$) | $577.78 \pm 610.57$ ($\pm 758.00$) | $38.73 \pm 14.09$ ($\pm 17.49$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### 3. Full 31-Family Synthetic Benchmark Corpus (`JSON -> Solution`)

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | **827.7x** [471.9x-1192.1x] | 0.01 | 6.69 | 0.00 | 6.63 | 0.005 | 0.087 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **1161.1x** [937.5x-1302.1x] | 0.01 | 6.26 | 0.00 | 6.20 | 0.003 | 0.079 | `OK` |
| `t1_std_mip` | 3 | 3 | **653.1x** [293.5x-830.5x] | 0.01 | 7.35 | 0.00 | 7.30 | 0.004 | 0.079 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **638.6x** [273.8x-776.3x] | 0.01 | 7.41 | 0.01 | 7.35 | 0.004 | 0.090 | `OK` |
| `t1_whiskas` | 6 | 5 | **635.2x** [504.9x-710.4x] | 0.01 | 6.33 | 0.00 | 6.23 | 0.006 | 0.099 | `OK` |
| `t1_furniture` | 2 | 3 | **371.9x** [85.5x-1201.4x] | 0.02 | 8.07 | 0.01 | 8.00 | 0.004 | 0.087 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **135.4x** [54.4x-385.2x] | 0.08 | 9.18 | 0.07 | 9.08 | 0.008 | 0.131 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **2179.5x** [1481.9x-2522.4x] | 0.00 | 6.29 | 0.00 | 6.23 | 0.002 | 0.063 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **109.5x** [66.9x-146.8x] | 0.08 | 9.50 | 0.04 | 9.29 | 0.032 | 0.285 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **55.4x** [31.4x-111.5x] | 0.58 | 33.73 | 0.55 | 33.50 | 0.027 | 0.307 | `OK` |
| `t2_mkp_30` | 30 | 6 | **7.7x** [4.4x-13.8x] | 2.53 | 28.01 | 2.50 | 27.82 | 0.021 | 0.235 | `OK` |
| `t2_mkp_50` | 50 | 8 | **4.3x** [1.8x-32.4x] | 6.45 | 17.40 | 6.41 | 17.14 | 0.041 | 0.346 | `OK` |
| `t2_cflp_36` | 36 | 11 | **32.9x** [4.0x-161.2x] | 0.24 | 9.38 | 0.22 | 9.21 | 0.019 | 0.221 | `OK` |
| `t2_cflp_49` | 49 | 13 | **20.1x** [9.6x-34.1x] | 0.51 | 10.15 | 0.46 | 9.96 | 0.026 | 0.276 | `OK` |
| `t2_gap_45` | 45 | 13 | **20.9x** [4.1x-62.1x] | 0.72 | 14.60 | 0.70 | 14.37 | 0.025 | 0.269 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **11.8x** [5.0x-28.6x] | 4.70 | 68.15 | 4.67 | 67.91 | 0.030 | 0.310 | `OK` |
| `t2_binpack_44` | 44 | 14 | **13.8x** [0.2x-50.9x] | 0.51 | 14.19 | 0.49 | 13.98 | 0.024 | 0.267 | `OK` |
| `t2_setcover_50` | 50 | 20 | **46.3x** [4.5x-123.8x] | 0.17 | 11.39 | 0.13 | 11.07 | 0.042 | 0.417 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **68.5x** [62.7x-79.6x] | 0.14 | 9.72 | 0.11 | 9.35 | 0.035 | 0.384 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **58.4x** [43.5x-79.7x] | 0.19 | 10.15 | 0.10 | 9.74 | 0.065 | 0.542 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **32.8x** [27.9x-39.5x] | 0.39 | 11.95 | 0.26 | 11.33 | 0.098 | 0.805 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **10.8x** [3.9x-33.4x] | 3.12 | 47.21 | 3.07 | 46.84 | 0.040 | 0.396 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **6.7x** [1.4x-26.0x] | 14.09 | 166.84 | 13.99 | 166.41 | 0.067 | 0.573 | `OK` |
| `t3_mkp_200` | 200 | 8 | **2.5x** [1.7x-4.3x] | 38.24 | 114.62 | 38.13 | 114.02 | 0.096 | 0.808 | `OK` |
| `t3_cflp_300` | 300 | 34 | **17.7x** [10.2x-25.1x] | 10.50 | 247.14 | 10.39 | 246.23 | 0.121 | 1.185 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **1.3x** [0.1x-3.5x] | 64.78 | 156.21 | 64.54 | 154.38 | 0.248 | 2.287 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **8.1x** [4.9x-10.9x] | 9.14 | 80.79 | 8.81 | 78.06 | 0.342 | 3.484 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **31.6x** [22.3x-36.2x] | 0.54 | 18.02 | 0.37 | 16.93 | 0.156 | 1.384 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **21.0x** [18.7x-24.5x] | 0.88 | 18.20 | 0.65 | 16.48 | 0.235 | 2.260 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **11.1x** [9.3x-12.7x] | 5.29 | 60.48 | 3.65 | 52.55 | 1.431 | 10.796 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **10.0x** [4.3x-26.0x] | 35.34 | 368.23 | 29.18 | 334.19 | 4.823 | 43.997 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

### 4. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus ([`bench/assign_corpus.go`](bench/assign_corpus.go), 200 randomized instances across square, rectangular wide/tall, degenerate integer-tie, Euclidean geometric, rank-1 outer-product, sparse `+Inf`, and maximization families) compares [`assign.LinearSumFlat`](assign/assign.go) against SciPy's C++ `rectangular_lsap` extension (`scipy.optimize.linear_sum_assignment`) in warm state on bit-identical `float64` cost matrices:

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `SciPy Solve / goblin` GeoMean [Clustered 95% CI] | Pooled Trial `SciPy Solve / goblin` | Equal-Family `SciPy Total / goblin` | `goblin` Solve Trial (Med) Sum (ms) | SciPy Solve Trial (Med) Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.78x** [1.43x-2.23x] | 1.78x | 5.39x | 0.194 (0.0159) | 0.318 (0.0292) | 1.80x [0.59x-3.40x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **3.11x** [1.88x-5.16x] | 3.11x | 3.09x | 10.055 (0.780) | 33.057 (2.944) | 3.04x [0.19x-18.84x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **3.16x** [2.17x-4.62x] | 3.16x | 3.91x | 208.854 (41.183) | 942.443 (187.380) | 3.04x [1.59x-6.29x] |
| Square families ($m = n$) | 16 (146) | **2.91x** [2.13x-3.98x] | 2.85x | 4.07x | 215.120 (41.308) | 964.934 (188.379) | 2.63x [0.19x-18.84x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **2.17x** [1.55x-3.03x] | 2.02x | 3.40x | 3.983 (0.672) | 10.884 (1.974) | 2.23x [0.75x-3.54x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.37x** [1.14x-4.93x] | 2.46x | 3.03x | 33.859 (5.954) | 65.982 (11.445) | 1.87x [0.61x-18.84x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.69x** [2.12x-3.41x] | 2.60x | 3.88x | 219.103 (41.979) | 975.818 (190.353) | 2.43x [0.19x-18.84x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **2.37x** | 0.0004 | 0.0009 | 0.0005 | 0.0116 | $0.0004 \pm 0.0001$ | $0.0010 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.93x** | 0.0012 | 0.0024 | 0.0015 | 0.0103 | $0.0013 \pm 0.0007$ | $0.0024 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.37x** | 0.0037 | 0.0062 | 0.0050 | 0.0151 | $0.0047 \pm 0.0023$ | $0.0060 \pm 0.0015$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.43x** | 0.0028 | 0.0043 | 0.0042 | 0.0145 | $0.0029 \pm 0.0006$ | $0.0041 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.74x** | 0.0029 | 0.0044 | 0.0037 | 0.0153 | $0.0027 \pm 0.0006$ | $0.0047 \pm 0.0009$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **2.05x** | 0.0050 | 0.0111 | 0.0078 | 0.0329 | $0.0055 \pm 0.0021$ | $0.0107 \pm 0.0017$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **3.06x** | 0.0232 | 0.0713 | 0.0311 | 0.0973 | $0.0231 \pm 0.0056$ | $0.0695 \pm 0.0111$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **4.27x** | 0.0471 | 0.176 | 0.0633 | 0.224 | $0.0463 \pm 0.0045$ | $0.220 \pm 0.148$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **1.79x** | 0.0561 | 0.0941 | 0.0825 | 0.142 | $0.0553 \pm 0.0159$ | $0.0963 \pm 0.0144$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.61x** | 0.135 | 0.219 | 0.149 | 0.270 | $0.149 \pm 0.0515$ | $0.233 \pm 0.0431$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **13.86x** | 0.0512 | 0.674 | 0.0670 | 0.762 | $0.0512 \pm 0.0153$ | $0.687 \pm 0.0227$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **2.64x** | 0.0270 | 0.0688 | 0.0522 | 0.143 | $0.0268 \pm 0.0029$ | $0.0707 \pm 0.0081$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **1.79x** | 0.0380 | 0.0790 | 0.0605 | 0.153 | $0.0505 \pm 0.0294$ | $0.0803 \pm 0.0086$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **2.87x** | 0.0773 | 0.288 | 0.103 | 0.372 | $0.180 \pm 0.338$ | $0.285 \pm 0.0327$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.85x** | 0.325 | 1.273 | 0.421 | 1.668 | $0.332 \pm 0.0607$ | $1.263 \pm 0.122$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.69x** | 1.537 | 5.181 | 1.958 | 7.867 | $1.467 \pm 0.356$ | $5.294 \pm 0.397$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **2.11x** | 0.722 | 1.501 | 1.271 | 4.188 | $0.705 \pm 0.0490$ | $1.487 \pm 0.0476$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.75x** | 4.981 | 8.940 | 5.179 | 10.606 | $5.481 \pm 1.983$ | $9.437 \pm 2.730$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **3.11x** | 0.264 | 0.821 | 0.665 | 3.636 | $0.271 \pm 0.0371$ | $0.840 \pm 0.0557$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.88x** | 0.337 | 0.997 | 0.699 | 3.859 | $0.343 \pm 0.0139$ | $0.985 \pm 0.0332$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **5.09x** | 5.648 | 28.195 | 7.490 | 36.776 | $5.610 \pm 0.241$ | $28.621 \pm 2.264$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **5.11x** | 27.694 | 141.746 | 37.189 | 184.189 | $27.893 \pm 3.697$ | $141.824 \pm 11.451$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

---

## Pure-Go vs. C++ CBC Performance Gaps and MAD Scaling Analysis

Comparing isolated unpresolved `Root LP` timings, paired trial ratios, and solver-core checkpoints (`CP5 +Presolve`, `CP6 +Cuts`, `CP7 Full`) between `BackendGo` and in-process `BackendCBC` (`c1c171ec`) highlights four structural trade-offs on large degenerate LPs and heavy-tail branch-and-cut models.

### 1. MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families (`N = G(I + T + 3)` variables, `M = G(2T + 3) + I` constraints) separate solver performance into three distinct scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Paired `Go / CBC` | Go Total Med (ms) | CBC Total Med (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.04x** | 0.08 | 2.36 | 0.47x | 0.04 (1 / 19) | 2.32 (0 / 0) | 0.064 (28) | 0.135 (19) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.06x** | 0.58 | 11.02 | 0.52x | 0.55 (5 / 108) | 10.98 (6 / 83) | 0.074 (30) | 0.141 (18) |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.07x** | 0.14 | 1.82 | 0.71x | 0.11 (1 / 26) | 1.78 (0 / 0) | 0.100 (33) | 0.142 (21) |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.07x** | 0.19 | 2.46 | 0.79x | 0.10 (1 / 39) | 2.39 (0 / 0) | 0.200 (58) | 0.253 (40) |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.11x** | 0.39 | 3.16 | 0.67x | 0.26 (1 / 51) | 3.06 (0 / 0) | 0.270 (67) | 0.403 (63) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.22x** | 3.12 | 16.62 | 0.73x | 3.07 (39 / 362) | 16.57 (12 / 216) | 0.114 (38) | 0.155 (24) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.26x** | 14.09 | 89.97 | 0.75x | 13.99 (220 / 2391) | 89.89 (198 / 1300) | 0.231 (65) | 0.307 (54) |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.06x** | 0.54 | 8.48 | 1.09x | 0.37 (1 / 75) | 8.24 (0 / 0) | 0.728 (128) | 0.665 (94) |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.11x** | 0.88 | 7.96 | 1.25x | 0.65 (1 / 93) | 7.70 (0 / 0) | 1.418 (182) | 1.135 (149) |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **0.14x** | 5.29 | 39.72 | 0.62x | 3.65 (1 / 243) | 38.16 (0 / 0) | 2.426 (221) | 3.933 (319) |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **0.17x** | 35.34 | 280.64 | 2.90x | 29.18 (1 / 2439) | 275.44 (0 / 0) | 105.384 (1905) | 36.306 (3128) |
<!-- END GENERATED:MAD_CBC_GAP_TABLE -->

1. **Small-to-Medium Planted-Gap MAD (`t1_mad_g2_i4` through `t3_mad_g5_i20`, $26 \le N \le 145$)**: Every structured `t*_mad_g*` instance has a zero root LP gap by design (`RootLPBound == OptimalObj`). Pure-Go closes those models at node 1 through presolve orbit fixing and root heuristics (`tryPropagatedRounding`, `tryRootSubMIP`, and `tryFractionalDiving` in [`heuristics.go`](heuristics.go)) without C++ `CbcModel` and `CglPreProcess` setup costs.
2. **Large Planted-Gap MAD (`t3_mad_g6_i30`, `t3_mad_g10_i100`, `t3_mad_g4_i1000`)**: Pure-Go `Go Solve` closes all three at node 1 because [`Presolve`](presolve.go) verifies the $S_G$ group-permutation automorphism (`applyOrbitSymmetryBreaking`), fixes the $G \times G$ designated-item bijection to $I_G$, and eliminates $G^2$ binary variables and $2G$ assignment equations before the root LP runs, while [`FTRANSparseHint`](lu.go) and [`BTRANSparseHint`](lu.go) restrict basis triangular sweeps to active index windows. On the unpresolved `Root LP` column:
   - **Equality Crash Basis (`t3_mad_g10_i100`)**: [`crashEqualityBasis`](simplex.go) builds a lower-triangular, dual-feasible equality basis with exact analytical DSE weights (`2.426 ms` / `221` iters vs. `COIN-OR CLP`'s `3.933 ms` / `319` iters, **`0.62x`**).
   - **Dense-Row Coupling and PFI Eta Cost (`t3_mad_g4_i1000`)**: On 1,000-row bases (`2.90x` vs. `CLP` on unpresolved Root LP), [`crashEqualityBasis`](simplex.go) intentionally skips columns incident to coupling rows wider than 128 nonzeros so BFRT bound flips retain sparse unit slacks, and [`lu.go`](lu.go) appends a Product-Form of the Inverse (PFI) eta file (`PushEtaSparse`) up to `maxEtaUpdates = 56` before rebuilding factors from scratch, whereas `CLP` updates the $U$ factor in place via Forrest-Tomlin updates.
3. **Symmetric Random-Weight MAD (`t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`)**: Assigning dense random weights (`BuildMADRandom`) removes the zero-gap structure and forces Branch-and-Cut to prove optimality:
   - [`Presolve`](presolve.go) verifies exact $S_G$ group symmetry and fixes the $G \times G$ designated-item bijection to $I_G$, [`bab.go`](bab.go) bails out after round 1 when root cuts yield zero bound gain and purges slack root cuts via [`PurgeSlackCuts`](simplex.go), and [`tryRootSubMIP`](heuristics.go) enforces a 50% RINS integer-fixing gate on zero-cut roots.
   - Across [`docs/benchmarks.md`](docs/benchmarks.md#mad-solver-core-checkpoint-progression), `CP5 +Presolve` (`Presolve=on, Cuts=off, Heuristics=off`) remains marginally faster than `CP7 Full` on `t1_mad_rand_g2_i4` (0.50 ms vs. 0.55 ms) and `t2_mad_rand_g2_i8` (2.43 ms vs. 3.07 ms) due to trial cut-separation overhead on tiny trees, whereas `CP7 Full` runs more than twice as fast as `CP5 +Presolve` on `t2_mad_rand_g3_i9` (13.99 ms vs. 29.94 ms).

### 2. Non-MAD Corpus Comparison, Heavy-Tail Dispersion, and Checkpoint Trade-Offs

Across the non-MAD families, multi-item lot sizing (`t3_lotsizing_504`) and outlier seeds of `t2_binpack_44` exhibit heavy-tail tree variance where single-row root cuts leave a larger search tree than CBC's multi-row cut aggregation, while `CP5 +Presolve` remains slightly faster than `CP7 Full` on `t3_cflp_300` (9.07 ms vs. 10.39 ms):

<!-- BEGIN GENERATED:NON_MAD_CBC_GAP_TABLE -->
| Problem ID | $N \times M$ | Paired `Go / CBC` | Go Total Mean $\pm$ SD (ms) | CBC Total Mean $\pm$ SD (ms) | Go CP7 Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go CP6 +Cuts (ms / nodes / cuts) | Go CP5 +Presolve (ms / nodes) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t3_mkp_200` | $200 \times 8$ | **0.94x** | $36.60 \pm 16.16$ | $40.41 \pm 19.05$ | 38.13 (598 / 5126) | 49.08 (154 / 1145) | 101.73 (1552 / 9) | 180.97 (5000, `NodeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **0.48x** | $9.09 \pm 10.76$ | $12.86 \pm 9.13$ | 6.41 (1 / 19) | 9.45 (0 / 8) | 14.59 (2 / 4) | 6.87 (132) |
| `t2_binpack_44` | $44 \times 14$ | **0.16x** | $13.91 \pm 40.01$ | $6.81 \pm 2.42$ | 0.49 (2 / 43) | 6.12 (0 / 8) | 0.86 (5 / 7) | 21.02 (1496) |
| `t3_cflp_300` | $300 \times 34$ | **0.23x** | $13.74 \pm 6.96$ | $57.18 \pm 12.76$ | 10.39 (53 / 1213) | 53.50 (18 / 483) | 12.25 (59 / 20) | 9.07 (134) |
| `t3_lotsizing_504` | $504 \times 336$ | **1.20x** | $576.76 \pm 1176.14$ | $96.71 \pm 59.74$ | 64.54 (30 / 1534) | 75.36 (10 / 377) | 106.51 (488 / 205) | 250.03 (3282, `TimeLimit`) |
| `t3_sched_1000` | $1000 \times 100$ | **0.16x** | $10.49 \pm 3.61$ | $62.38 \pm 4.47$ | 8.81 (64 / 426) | 62.04 (0 / 76) | 35.78 (206 / 46) | 250.02 (2157, `TimeLimit`) |
<!-- END GENERATED:NON_MAD_CBC_GAP_TABLE -->

---

## Roadmap and Open Gaps

This section consolidates open correctness, platform, algorithmic, and performance gaps across `BackendGo`, `BackendCBC`, `goblin/assign`, and `goblin/format`.

| Priority | Track | Open Gap / Work Item | Primary Files | Target Workload / Outcome |
| :--- | :--- | :--- | :--- | :--- |
| **P0** | Correctness | Triage pre-existing `bigm347` differential mismatch (`goblin=39.4` vs. `cbc=35.0501`) | [`simplex.go`](simplex.go), [`bab.go`](bab.go), [`bench/difftest_cbc.py`](bench/difftest_cbc.py) | Zero differential mismatches across 400 random CBC test seeds |
| **P0** | Platform | Validate native `linux/arm64` execution, including `-tags=cbc` bridge tests | [`cbc_cgo.go`](cbc_cgo.go), [`scripts/build_cbc.sh`](scripts/build_cbc.sh) | Verified parity and race tests on `aarch64` hardware |
| **P1** | LP & LU | True in-place mutated-$U$ Forrest-Tomlin LU updates (without lazy-rank branch tax) | [`lu.go`](lu.go) | Cut per-pivot BTRAN/FTRAN cost on `t3_mad_g4_i1000` and dense Netlib/MIPLIB bases |
| **P1** | LP & LU | Dedicated LP presolve and anti-degeneracy cost perturbation for large unpresolved LPs | [`simplex.go`](simplex.go), [`presolve.go`](presolve.go) | Close the remaining 2.90x unpresolved Root LP gap vs. CLP on `t3_mad_g4_i1000` |
| **P1** | MIP Cuts | Multi-row cut aggregation and selective in-tree cut separation | [`cuts.go`](cuts.go), [`bab.go`](bab.go) | Reduce heavy-tail search trees on `t3_lotsizing_504`, `t3_mkp_200`, and `coinor_capplan1` |
| **P2** | MIP Search | Dynamic orbital branching for symmetries emerging after branching on identical items | [`bab.go`](bab.go), [`presolve.go`](presolve.go) | Extend static $S_G$ presolve orbit fixing into the B&B tree |
| **P2** | MIP Search | Strong-branching tuning (`miplib_lseu`) and trial-cut gating on small trees | [`bab.go`](bab.go), [`heuristics.go`](heuristics.go) | Eliminate remaining `CP5 +Presolve` vs. `CP7 Full` root overhead on `t3_cflp_300` and tiny random MAD |
| **P3** | LSAP | Volgenant sparse-core pricing (`K = 16..32` candidate edges per row) for `N >= 1000` | [`assign/solver.go`](assign/solver.go) | Keep large dense LSAP active graphs inside L2 cache |
| **P3** | Formats | Direct streaming `PuLP` JSON ingestion bypassing the intermediate `PuLPJSON` AST | [`format/pulp_json_fast.go`](format/pulp_json_fast.go) | Lower peak parse memory and prep latency on large JSON models |
| **P3** | MIP Search | Optional parallel branch-and-bound workers with deterministic single-threaded mode | [`bab.go`](bab.go) | Multi-core scaling on deep B&B trees without changing deterministic default behavior |

### Architectural Context and Rejected Designs

1. **LU Updates and Row-Singleton Stability (`lu.go`)**: The factorizer stores pointer-free 32-byte `int32` PFI eta records with `[minIdx, maxIdx]` range-skipped BTRAN up to `maxEtaUpdates = 56`. Two candidate changes were tested and rejected: (a) a lazy-rank Forrest-Tomlin prototype with `maxEtaUpdates = 128` regressed `miplib_p0201` (0.62x) and increased `netlib_brandy` iterations (380 to 2,722) due to `uRank` branch mispredictions; (b) removing the `luMarkowitzTol * colMax` check on Stage 2 row singletons caused unscaled `1000x` multipliers in $L$ and numerical stalling on `t3_mad_g4_i1000`.
2. **Root LP Crash Basis and Dense Coupling (`simplex.go`)**: Dual Phase I (`installDualPhase1Bounds`) keeps all 14 public Netlib LPs inside sparse dual simplex with zero primal fallbacks, and [`crashEqualityBasis`](simplex.go) cuts `t3_mad_g10_i100` unpresolved Root LP by 4.9x (`2.43 ms` vs. CLP `3.93 ms`). Crashing columns incident to dense coupling rows (`> 128` nonzeros) was rejected because displacing 1-NNZ artificial slacks on `t3_mad_g4_i1000` doubled $U$ fill-in and slowed solve latency by 2.5x.
3. **Cut Separation, Symmetry, and Zero-Cut Sub-MIP Gating (`cuts.go`, `presolve.go`, `bab.go`, `heuristics.go`)**: Root cut rounds enforce GMI/MIR density caps (`maxCutNNZ`), exit after round 1 on zero bound gain, and purge inactive slack cuts both before branching and periodically in-tree via [`PurgeSlackCuts`](simplex.go). On zero-cut roots, a blanket `< 48`-integer sub-MIP ban improved 7-trial median on `t2_mad_rand_g3_i9` but inflated 11-trial mean (`29.5` to `38.6 ms`) and tail (`96.9` to `121.6 ms`); requiring 50% fixed integers (RINS criterion) improved median, mean, tail, and node counts across all 11 trials.
4. **Recently Closed Milestones (Already Landed)**:
   - **Numerical and Bridge Hardening (`3f52ee8`, `9a6b3f5`, `85d57a3`)**: Multi-scale three-bucket Shewchuk summation (`wideDotAcc`, `assignWideAcc`), CBC tiny-coefficient and cutoff guards, MIPLIB 3.0 public benchmark fixtures, and family-clustered Student-$t$ telemetry intervals.
   - **Wave 0 (`facbd10`)**: Pointer-free inline `wideDotAcc` (zero heap escapes), per-row `rowMaxTerm` residual triggers, and stack `dot2Acc` fast paths.
   - **Wave 1 (`bd0bdf4` to `fe40685`)**: Elimination of Go 1.27 `archsimd` AVX2-to-SSE `VZEROUPPER` traps, scale-relative `assign.hasPotentialCancellation` (0 allocs/op warm `SolveInto`), branchless AVX2 Dijkstra relaxation (`assign_amd64.go`), zero-pin SWAR JSON parsing and buffered `WriteMPS`, lazy CSC indexing on `extraRows`, batch-fixed `solveFixedIntContinuousLP`, and bounded post-cut root heuristics.
   - **Wave 2 (`c7cb1a4` to `99a49c0`)**: Flat single-value LU nucleus arenas with intrusive `int32` Markowitz buckets (0 allocs/op steady-state LU), EMA-gated iterative Gilbert-Peierls reach DFS, copy-on-write `Presolve` with direct `CompiledProblem` compilation, lazy `cbcOnce` CBC slices, Dual Phase I, packed contiguous `basicLB`/`basicUB`/`basicTol`, GMI/MIR density caps, in-tree slack-cut aging, and incremental CSC node propagation (`propagateFromCols`).
   - **Wave 3 (`8bcb706` to `2b7305d`, Multi-Tier AVX-512/AVX2 SIMD & MAD Solver Optimization, verified on GCP `n4` Emerald Rapids & `e2` Broadwell)**: Runtime AVX-512/AVX2/Scalar CPUID dispatch in a single `CGO_ENABLED=0` binary (`GOAMD64=v3` FMA-safe `twoProd`/`twoSum` error-free transforms), 4-accumulator scalar `Axpy64Scalar` for Broadwell, in-register DSE leaving-row scan (`SelectDualLeaveRow64With`), batched dual ratio screening (`ScanDualRatioCand64With`), zero-alloc equality crash basis (`crashEqualityBasis`), certified unrefined residual fast paths (`evalRowRes`), dirty-row presolve tightening and zero-sort $O(G \cdot \text{rowLen})$ orbit checks, decoupled epigraph rounding with fractional-only sort, zero-cut 50% RINS sub-MIP gating, and root-LP re-solve plus CBC primal polishing (`FuzzMADDifferentialCBC`).

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
