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
- **Unified telemetry source**: fingerprint `6a0ff4872f1c417c` (working tree on top of commit `85d57a31dbae83971e3c933fa305d29684ef5a14`), `go1.27.3-X:simd`, Linux kernel `6.18.63-gentoo-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, or kernel differs.
- **CPU affinity**: goblin `6`; Python + CBC `6`; Python + SciPy `6`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 7.17 ms median on a 1-variable MILP and 6.66 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

Read the synthetic corpus ratios with three methodological notes. First, Tier 1 micro-models (`N <= 26`) are dominated by Python's CBC subprocess spawn floor reported in the provenance block above. Second, the planted-gap MAD family has a zero root LP gap by construction and closes at the root without branching. Consequently, the bold row (`Tier 2+3 non-planted MILPs`) isolates solver-dominated MILPs across paired randomized instances, alongside total subset wall times and full per-instance `[Min-Max]` ranges.

### 1. Aggregate Synthetic Workload Summary (Paired Per-Instance Geometric Mean and Wall-Clock Sums)

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `Py / Go` GeoMean [Clustered 95% CI] | Pooled Trial `Py / Go` | Equal-Family `Go / CBC` | `Go` Trial (Med) Sum (ms) | `CBC` Trial (Med) Sum (ms) | `Py` Trial (Med) Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **361.22x** [143.92x-906.59x] | 361.22x | 0.05x | 21.1 (2.0) | 193.2 (19.3) | 1134.3 (106.7) | 604.64x [10.19x-2026.59x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **11.19x** [5.90x-21.20x] | 11.19x | 0.27x | 1545.5 (83.3) | 1995.0 (177.8) | 4439.3 (427.2) | 13.52x [0.07x-152.38x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **4.30x** [1.44x-12.87x] | 4.30x | 0.49x | 10742.5 (469.1) | 2924.2 (566.0) | 6281.9 (1079.2) | 5.73x [0.03x-28.65x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **5.17x** [2.61x-10.23x] | 6.22x | 0.51x | 11786.0 (488.0) | 3152.2 (402.4) | 6893.3 (998.2) | 6.06x [0.03x-152.38x] |
| Tier 3 non-MAD MILPs | 4 (20) | **1.90x** [0.22x-16.03x] | 1.90x | 1.14x | 10250.9 (405.5) | 1253.5 (232.8) | 2824.2 (604.2) | 1.68x [0.03x-13.02x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **22.38x** [9.16x-54.63x] | 30.53x | 0.13x | 503.0 (64.5) | 1791.4 (343.9) | 3939.7 (518.5) | 32.85x [3.18x-147.17x] |
| Random-weight MAD (negative controls) | 3 (33) | **7.38x** [0.50x-109.13x] | 7.38x | 0.33x | 729.5 (53.8) | 1236.3 (121.2) | 2287.0 (250.2) | 7.39x [0.48x-76.34x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **28.56x** [9.73x-83.81x] | 37.95x | 0.20x | 11806.1 (489.9) | 3321.0 (419.2) | 7915.8 (1094.7) | 24.41x [0.03x-2026.59x] |
| All 31 synthetic corpus families | 31 (293) | **26.82x** [11.98x-60.01x] | 36.19x | 0.18x | 12309.0 (554.4) | 5112.3 (763.1) | 11855.5 (1613.2) | 28.65x [0.03x-2026.59x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2. Synthetic MAD Group-Item Allocation Family (`G` Groups $\times$ `I` Items, $T = 6$ Periods)

The synthetic `MAD` group-item allocation family ([`BuildMADFromWeights`](bench/corpus.go)) assigns $I$ items across $G$ groups over $T = 6$ periods to minimize total L1 distance from each group's central level $m_g$ plus the roundness penalty $r_g = |m_g - 100 n_g|$ relative to an integer multiple $n_g \in \mathbb{Z}_{\ge 1}$. In the planted-gap generator, items in $S = \{0, \dots, I-G-1\}$ carry orthogonal curve signatures cancelled by the $G$ designated items $D = \{I-G, \dots, I-1\}$ with roundness-aligned period sums, yielding a zero root LP gap that root heuristics close without branching; unstructured random-weight negative controls (`BuildMADRandom`: `t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`) are included directly in the corpus and detailed in [`docs/benchmarks.md`](docs/benchmarks.md).

<!-- BEGIN GENERATED:MAD_PIPELINE_TABLE -->
| Problem ID | $G \times I$ | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Trial 0 Obj | Parity |
| :--- | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **120.0x** [57.9x-147.2x] | 0.08 | 10.26 | 0.05 | 10.03 | 0.024 | 0.296 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **24.8x** [10.2x-76.3x] | 1.71 | 34.24 | 1.67 | 34.00 | 0.030 | 0.305 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **50.6x** [35.5x-62.2x] | 0.18 | 10.25 | 0.15 | 9.93 | 0.035 | 0.391 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **45.5x** [32.5x-57.7x] | 0.21 | 10.74 | 0.16 | 10.29 | 0.056 | 0.552 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **25.0x** [20.5x-30.3x] | 0.48 | 12.20 | 0.38 | 11.58 | 0.091 | 0.812 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **5.2x** [2.2x-19.0x] | 7.22 | 48.64 | 7.16 | 48.32 | 0.058 | 0.402 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **3.1x** [0.5x-12.6x] | 44.85 | 167.37 | 44.77 | 166.94 | 0.082 | 0.569 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **22.2x** [14.5x-28.7x] | 0.78 | 19.20 | 0.60 | 18.02 | 0.196 | 1.382 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **11.8x** [8.9x-20.0x] | 1.76 | 19.26 | 1.52 | 17.50 | 0.282 | 2.305 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **5.6x** [5.0x-6.5x] | 11.17 | 62.28 | 10.07 | 54.60 | 1.221 | 10.838 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **6.1x** [3.2x-7.9x] | 49.82 | 374.33 | 43.67 | 339.00 | 4.601 | 45.480 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

Cross-instance total latency dispersion across the randomized instances in each family (mean $\pm$ sample SD and two-sided 95% Student-$t$ interval after warmup):

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.09 \pm 0.03$ ($\pm 0.02$) | $10.17 \pm 0.53$ ($\pm 0.36$) | $0.09 \pm 0.01$ ($\pm 0.01$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $1.62 \pm 1.00$ ($\pm 0.67$) | $31.19 \pm 9.51$ ($\pm 6.39$) | $1.61 \pm 0.97$ ($\pm 0.65$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.21 \pm 0.04$ ($\pm 0.03$) | $10.44 \pm 0.60$ ($\pm 0.40$) | $0.23 \pm 0.07$ ($\pm 0.05$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.24 \pm 0.05$ ($\pm 0.04$) | $10.75 \pm 0.18$ ($\pm 0.12$) | $0.25 \pm 0.07$ ($\pm 0.05$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.50 \pm 0.06$ ($\pm 0.04$) | $12.47 \pm 0.68$ ($\pm 0.46$) | $0.52 \pm 0.08$ ($\pm 0.05$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $8.89 \pm 6.68$ ($\pm 4.49$) | $36.29 \pm 17.18$ ($\pm 11.54$) | $8.74 \pm 6.61$ ($\pm 4.44$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $55.80 \pm 46.72$ ($\pm 31.38$) | $140.43 \pm 64.13$ ($\pm 43.08$) | $56.36 \pm 47.08$ ($\pm 31.63$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.84 \pm 0.14$ ($\pm 0.17$) | $18.71 \pm 2.35$ ($\pm 2.92$) | $0.78 \pm 0.09$ ($\pm 0.11$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.70 \pm 0.49$ ($\pm 0.60$) | $19.29 \pm 0.10$ ($\pm 0.12$) | $1.60 \pm 0.51$ ($\pm 0.63$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $11.39 \pm 1.50$ ($\pm 1.87$) | $63.97 \pm 3.40$ ($\pm 4.23$) | $11.25 \pm 0.60$ ($\pm 0.74$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $84.37 \pm 72.50$ ($\pm 90.00$) | $589.57 \pm 615.72$ ($\pm 764.39$) | $85.82 \pm 76.74$ ($\pm 95.28$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### 3. Full 31-Family Synthetic Benchmark Corpus (`JSON -> Solution`)

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | **802.0x** [524.9x-1203.9x] | 0.01 | 6.92 | 0.00 | 6.86 | 0.005 | 0.080 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **1026.2x** [641.8x-1275.0x] | 0.01 | 6.69 | 0.00 | 6.64 | 0.003 | 0.087 | `OK` |
| `t1_std_mip` | 3 | 3 | **838.1x** [760.2x-969.4x] | 0.01 | 8.07 | 0.01 | 8.01 | 0.003 | 0.090 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **855.0x** [686.5x-999.5x] | 0.01 | 7.94 | 0.01 | 7.87 | 0.003 | 0.090 | `OK` |
| `t1_whiskas` | 6 | 5 | **569.2x** [401.9x-694.5x] | 0.01 | 7.10 | 0.00 | 7.00 | 0.007 | 0.122 | `OK` |
| `t1_furniture` | 2 | 3 | **285.8x** [67.5x-773.1x] | 0.02 | 8.69 | 0.02 | 8.62 | 0.003 | 0.092 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **105.3x** [43.2x-260.7x] | 0.10 | 10.17 | 0.09 | 10.06 | 0.010 | 0.135 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **1256.5x** [392.0x-2026.6x] | 0.00 | 6.64 | 0.00 | 6.60 | 0.003 | 0.070 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **120.0x** [57.9x-147.2x] | 0.08 | 10.26 | 0.05 | 10.03 | 0.024 | 0.296 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **24.8x** [10.2x-76.3x] | 1.71 | 34.24 | 1.67 | 34.00 | 0.030 | 0.305 | `OK` |
| `t2_mkp_30` | 30 | 6 | **4.2x** [2.4x-7.1x] | 4.82 | 28.64 | 4.80 | 28.46 | 0.030 | 0.239 | `OK` |
| `t2_mkp_50` | 50 | 8 | **2.3x** [1.0x-24.4x] | 12.51 | 17.78 | 12.46 | 17.52 | 0.058 | 0.354 | `OK` |
| `t2_cflp_36` | 36 | 11 | **23.0x** [2.2x-152.4x] | 0.32 | 10.04 | 0.30 | 9.85 | 0.024 | 0.236 | `OK` |
| `t2_cflp_49` | 49 | 13 | **14.5x** [5.5x-31.7x] | 0.70 | 11.00 | 0.67 | 10.77 | 0.028 | 0.285 | `OK` |
| `t2_gap_45` | 45 | 13 | **7.5x** [1.2x-21.6x] | 1.87 | 15.24 | 1.82 | 15.02 | 0.035 | 0.273 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **6.3x** [3.0x-13.6x] | 9.15 | 69.09 | 9.08 | 68.85 | 0.046 | 0.317 | `OK` |
| `t2_binpack_44` | 44 | 14 | **7.7x** [0.1x-33.1x] | 0.78 | 14.52 | 0.76 | 14.31 | 0.030 | 0.270 | `OK` |
| `t2_setcover_50` | 50 | 20 | **39.7x** [2.5x-107.8x] | 0.21 | 11.75 | 0.16 | 11.44 | 0.047 | 0.412 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **50.6x** [35.5x-62.2x] | 0.18 | 10.25 | 0.15 | 9.93 | 0.035 | 0.391 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **45.5x** [32.5x-57.7x] | 0.21 | 10.74 | 0.16 | 10.29 | 0.056 | 0.552 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **25.0x** [20.5x-30.3x] | 0.48 | 12.20 | 0.38 | 11.58 | 0.091 | 0.812 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **5.2x** [2.2x-19.0x] | 7.22 | 48.64 | 7.16 | 48.32 | 0.058 | 0.402 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **3.1x** [0.5x-12.6x] | 44.85 | 167.37 | 44.77 | 166.94 | 0.082 | 0.569 | `OK` |
| `t3_mkp_200` | 200 | 8 | **1.0x** [1.0x-1.1x] | 104.91 | 115.78 | 104.80 | 115.17 | 0.123 | 0.806 | `OK` |
| `t3_cflp_300` | 300 | 34 | **8.7x** [5.7x-13.0x] | 24.25 | 249.05 | 23.98 | 248.13 | 0.141 | 1.188 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **0.4x** [0.0x-1.1x] | 253.45 | 157.90 | 253.20 | 156.06 | 0.268 | 2.307 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **3.5x** [2.3x-4.5x] | 22.93 | 81.45 | 22.55 | 78.54 | 0.383 | 3.519 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **22.2x** [14.5x-28.7x] | 0.78 | 19.20 | 0.60 | 18.02 | 0.196 | 1.382 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **11.8x** [8.9x-20.0x] | 1.76 | 19.26 | 1.52 | 17.50 | 0.282 | 2.305 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.6x** [5.0x-6.5x] | 11.17 | 62.28 | 10.07 | 54.60 | 1.221 | 10.838 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **6.1x** [3.2x-7.9x] | 49.82 | 374.33 | 43.67 | 339.00 | 4.601 | 45.480 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

### 4. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus ([`bench/assign_corpus.go`](bench/assign_corpus.go), 200 randomized instances across square, rectangular wide/tall, degenerate integer-tie, Euclidean geometric, rank-1 outer-product, sparse `+Inf`, and maximization families) compares [`assign.LinearSumFlat`](assign/assign.go) against SciPy's C++ `rectangular_lsap` extension (`scipy.optimize.linear_sum_assignment`) in warm state on bit-identical `float64` cost matrices:

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `SciPy Solve / goblin` GeoMean [Clustered 95% CI] | Pooled Trial `SciPy Solve / goblin` | Equal-Family `SciPy Total / goblin` | `goblin` Solve Trial (Med) Sum (ms) | SciPy Solve Trial (Med) Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.56x** [1.34x-1.82x] | 1.56x | 4.47x | 0.203 (0.0165) | 0.315 (0.0290) | 1.60x [0.38x-3.52x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **3.32x** [2.10x-5.25x] | 3.32x | 3.36x | 8.932 (0.782) | 35.399 (3.147) | 3.04x [1.04x-15.57x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **3.04x** [2.10x-4.42x] | 3.04x | 3.94x | 246.561 (48.219) | 962.516 (190.164) | 2.95x [1.49x-6.06x] |
| Square families ($m = n$) | 16 (146) | **2.91x** [2.14x-3.96x] | 2.86x | 4.08x | 251.072 (48.190) | 986.028 (191.101) | 2.63x [0.38x-15.57x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **2.00x** [1.40x-2.87x] | 1.86x | 3.20x | 4.623 (0.827) | 12.201 (2.239) | 1.99x [0.77x-3.38x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.54x** [1.29x-5.00x] | 2.69x | 3.14x | 34.740 (5.998) | 66.837 (11.198) | 2.07x [0.56x-15.57x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.63x** [2.07x-3.34x] | 2.55x | 3.82x | 255.695 (49.017) | 998.229 (193.340) | 2.36x [0.38x-15.57x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.48x** | 0.0006 | 0.0011 | 0.0007 | 0.0137 | $0.0009 \pm 0.0008$ | $0.0011 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.61x** | 0.0015 | 0.0024 | 0.0017 | 0.0101 | $0.0015 \pm 0.0003$ | $0.0023 \pm 0.0004$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.79x** | 0.0028 | 0.0062 | 0.0056 | 0.0153 | $0.0037 \pm 0.0023$ | $0.0061 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.30x** | 0.0029 | 0.0040 | 0.0047 | 0.0142 | $0.0031 \pm 0.0010$ | $0.0039 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.39x** | 0.0032 | 0.0044 | 0.0048 | 0.0151 | $0.0033 \pm 0.0011$ | $0.0045 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.88x** | 0.0055 | 0.0110 | 0.0086 | 0.0327 | $0.0060 \pm 0.0022$ | $0.0108 \pm 0.0018$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **3.01x** | 0.0265 | 0.0849 | 0.0335 | 0.116 | $0.0300 \pm 0.0142$ | $0.0847 \pm 0.0135$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.82x** | 0.0526 | 0.191 | 0.0736 | 0.253 | $0.0514 \pm 0.0044$ | $0.198 \pm 0.0356$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.50x** | 0.0453 | 0.111 | 0.0706 | 0.171 | $0.0440 \pm 0.0090$ | $0.109 \pm 0.0177$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.80x** | 0.133 | 0.239 | 0.150 | 0.280 | $0.143 \pm 0.0413$ | $0.255 \pm 0.0563$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **13.03x** | 0.0565 | 0.730 | 0.0739 | 0.825 | $0.0578 \pm 0.0063$ | $0.752 \pm 0.0694$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **2.36x** | 0.0316 | 0.0706 | 0.0547 | 0.144 | $0.0305 \pm 0.0036$ | $0.0719 \pm 0.0081$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **1.94x** | 0.0382 | 0.0791 | 0.0645 | 0.153 | $0.0423 \pm 0.0108$ | $0.0802 \pm 0.0073$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.94x** | 0.0813 | 0.313 | 0.108 | 0.416 | $0.0830 \pm 0.0081$ | $0.330 \pm 0.0580$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **4.07x** | 0.316 | 1.328 | 0.418 | 1.870 | $0.330 \pm 0.0498$ | $1.337 \pm 0.171$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.43x** | 1.212 | 5.645 | 1.616 | 8.483 | $1.284 \pm 0.144$ | $5.701 \pm 0.766$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **2.06x** | 0.788 | 1.667 | 1.352 | 4.484 | $0.791 \pm 0.0554$ | $1.625 \pm 0.104$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.69x** | 4.966 | 8.434 | 5.584 | 9.611 | $5.596 \pm 2.063$ | $9.250 \pm 2.532$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **3.01x** | 0.314 | 0.937 | 0.689 | 3.879 | $0.309 \pm 0.0227$ | $0.930 \pm 0.0795$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.62x** | 0.437 | 1.143 | 0.897 | 4.204 | $0.441 \pm 0.0165$ | $1.157 \pm 0.0633$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.82x** | 5.935 | 28.779 | 8.189 | 39.582 | $6.218 \pm 0.519$ | $30.045 \pm 3.134$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.14x** | 34.566 | 143.559 | 41.442 | 193.083 | $34.672 \pm 1.851$ | $143.795 \pm 9.652$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

---

## Pure-Go vs. C++ CBC Performance Gaps and MAD Scaling Analysis

Comparing isolated unpresolved `Root LP` timings, paired trial ratios, and solver-core checkpoints (`CP5 +Presolve`, `CP6 +Cuts`, `CP7 Full`) between `BackendGo` and in-process `BackendCBC` (`c1c171ec`) highlights four structural trade-offs on large degenerate LPs and heavy-tail branch-and-cut models.

### 1. MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families (`N = G(I + T + 3)` variables, `M = G(2T + 3) + I` constraints) separate solver performance into three distinct scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Paired `Go / CBC` | Go Total Med (ms) | CBC Total Med (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.04x** | 0.08 | 2.44 | 0.60x | 0.05 (1 / 19) | 2.41 (0 / 0) | 0.083 (28) | 0.138 (19) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.13x** | 1.71 | 11.15 | 0.69x | 1.67 (10 / 232) | 11.11 (6 / 83) | 0.101 (30) | 0.146 (18) |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.09x** | 0.18 | 2.07 | 0.63x | 0.15 (1 / 26) | 2.01 (0 / 0) | 0.103 (33) | 0.163 (21) |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.09x** | 0.21 | 2.61 | 0.95x | 0.16 (1 / 39) | 2.55 (0 / 0) | 0.272 (59) | 0.287 (40) |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.13x** | 0.48 | 3.54 | 0.85x | 0.38 (1 / 51) | 3.41 (0 / 0) | 0.350 (68) | 0.412 (63) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.47x** | 7.22 | 16.64 | 1.03x | 7.16 (65 / 714) | 16.59 (12 / 216) | 0.160 (38) | 0.155 (24) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.58x** | 44.85 | 93.36 | 1.05x | 44.77 (254 / 3052) | 92.89 (198 / 1300) | 0.303 (66) | 0.288 (54) |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.10x** | 0.78 | 8.19 | 1.50x | 0.60 (1 / 75) | 8.03 (0 / 0) | 0.977 (132) | 0.654 (94) |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.21x** | 1.76 | 7.57 | 1.52x | 1.52 (1 / 105) | 7.32 (0 / 0) | 1.595 (423) | 1.052 (149) |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **0.30x** | 11.17 | 37.69 | 3.80x | 10.07 (2 / 357) | 35.55 (0 / 0) | 13.926 (1031) | 3.662 (319) |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **0.29x** | 49.82 | 279.84 | 3.19x | 43.67 (1 / 2439) | 273.91 (0 / 0) | 114.463 (1864) | 35.829 (3128) |
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
| `t3_mkp_200` | $200 \times 8$ | **2.31x** | $98.05 \pm 52.49$ | $40.22 \pm 19.09$ | 104.80 (590 / 5308) | 49.38 (154 / 1145) | 131.15 (701 / 12) | 250.05 (3500, `TimeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **1.00x** | $17.88 \pm 19.87$ | $12.38 \pm 9.05$ | 12.46 (1 / 24) | 8.68 (0 / 8) | 28.71 (2 / 4) | 12.94 (132) |
| `t2_binpack_44` | $44 \times 14$ | **0.29x** | $30.95 \pm 88.38$ | $6.93 \pm 2.19$ | 0.76 (2 / 69) | 6.30 (0 / 8) | 1.69 (6 / 7) | 39.14 (1848) |
| `t3_cflp_300` | $300 \times 34$ | **0.47x** | $28.09 \pm 14.62$ | $55.22 \pm 11.63$ | 23.98 (38 / 1551) | 49.95 (18 / 483) | 22.81 (38 / 31) | 15.96 (134) |
| `t3_lotsizing_504` | $504 \times 336$ | **4.08x** | $1899.67 \pm 3862.58$ | $93.09 \pm 56.92$ | 253.20 (35 / 1615) | 72.01 (10 / 377) | 243.23 (420 / 205) | 250.03 (1329, `TimeLimit`) |
| `t3_sched_1000` | $1000 \times 100$ | **0.38x** | $24.38 \pm 6.89$ | $62.16 \pm 5.19$ | 22.55 (64 / 507) | 60.17 (0 / 76) | 75.06 (206 / 46) | 250.05 (1066, `TimeLimit`) |
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
