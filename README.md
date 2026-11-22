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
- **[Formal Verification Architecture (Lean 4 Prove Lane)](docs/formal-verification.md)**: Industry precedents, anti-translation-gap rules, and the five-track Lean 4 verification suite (`22` Lean files, `809` authored theorems, `2,565` kernel-checked `TheoremInfo` declarations, `124` `AxiomAudit.lean` locks, `0` `sorryAx`) covering LSAP/LP/Farkas/Meyer unboundedness and zero-observer-effect `WithVIPRTrace` Branch-and-Cut certification ([`exact_polish.go`](exact_polish.go), [`vipr_trace.go`](vipr_trace.go)), MILP presolve/symmetry/cut/LP-postsolve soundness proofs (`Track2A`-`Track2C`, `Track3B`), and binary/SIMD equivalence across `14` compiled `arm64` ELF routines and `amd64` vector paths (`Track5A`-`Track5D`).

---

## Benchmarks

All benchmark tables in `README.md`, [`docs/benchmarks.md`](docs/benchmarks.md), and [`bench/results/final_report.md`](bench/results/final_report.md) are rendered deterministically from the seven JSON telemetry files in `bench/results/` (`python_baseline.json`, `goblin_cbc.json`, `goblin_scalar.json`, `goblin_simd.json`, `scipy_assign_baseline.json`, `goblin_assign_scalar.json`, `goblin_assign_simd.json`). [`doc_sync_test.go`](doc_sync_test.go) fails `go test ./...` if any table drifts from the telemetry, if any `goblin` build disagrees with CBC or SciPy on a status or objective, or if any telemetry file differs in git commit, source/corpus fingerprint, or host kernel. Headline timings use the default pure-Go scalar build (`BackendGo`).

<!-- BEGIN GENERATED:PROVENANCE -->
- **Unified telemetry source**: fingerprint `6a8a322d4f9ce0be` (working tree on top of commit `98ebb1c888513674ef7d5c1a4dc61cbd479baa75`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.57 ms median on a 1-variable MILP and 6.23 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

Read the synthetic corpus ratios with three methodological notes. First, Tier 1 micro-models (`N <= 26`) are dominated by Python's CBC subprocess spawn floor reported in the provenance block above. Second, the planted-gap MAD family has a zero root LP gap by construction and closes at the root without branching. Consequently, the bold row (`Tier 2+3 non-planted MILPs`) isolates solver-dominated MILPs across paired randomized instances, alongside total subset wall times and full per-instance `[Min-Max]` ranges.

### 1. Aggregate Synthetic Workload Summary (Paired Per-Instance Geometric Mean and Wall-Clock Sums)

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `Py / Go` GeoMean [Clustered 95% CI] | Pooled Trial `Py / Go` | Equal-Family `Go / CBC` | `Go` Trial (Med) Sum (ms) | `CBC` Trial (Med) Sum (ms) | `Py` Trial (Med) Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **329.11x** [162.95x-664.72x] | 329.11x | 0.05x | 9.0 (0.8) | 197.7 (19.7) | 1089.1 (102.3) | 536.17x [29.94x-1235.03x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **20.66x** [12.62x-33.80x] | 20.66x | 0.15x | 650.8 (32.2) | 1954.4 (170.5) | 4355.1 (421.5) | 25.82x [0.13x-174.46x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **10.33x** [5.03x-21.22x] | 10.33x | 0.20x | 895.2 (139.7) | 2915.1 (561.5) | 6252.4 (1081.3) | 10.14x [1.13x-75.96x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **13.58x** [8.08x-22.82x] | 14.95x | 0.19x | 1234.4 (127.3) | 3120.7 (395.6) | 6839.1 (998.1) | 15.38x [0.13x-174.46x] |
| Tier 3 non-MAD MILPs | 4 (20) | **8.07x** [1.58x-41.20x] | 8.07x | 0.27x | 592.5 (95.9) | 1253.8 (232.5) | 2832.8 (608.0) | 6.65x [1.13x-75.96x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **26.27x** [12.02x-57.44x] | 33.99x | 0.11x | 312.7 (44.8) | 1773.8 (339.0) | 3873.7 (514.3) | 38.72x [1.14x-124.90x] |
| Random-weight MAD (negative controls) | 3 (33) | **16.92x** [0.82x-350.77x] | 16.92x | 0.14x | 300.6 (20.0) | 1216.7 (116.4) | 2266.0 (250.4) | 16.28x [1.36x-119.47x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **49.75x** [22.04x-112.29x] | 60.20x | 0.11x | 1242.3 (128.0) | 3293.3 (412.8) | 7822.8 (1090.8) | 49.44x [0.13x-1235.03x] |
| All 31 synthetic corpus families | 31 (293) | **42.19x** [22.63x-78.66x] | 53.13x | 0.11x | 1555.0 (172.7) | 5067.2 (751.7) | 11696.5 (1605.1) | 43.33x [0.13x-1235.03x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2. Synthetic MAD Group-Item Allocation Family (`G` Groups $\times$ `I` Items, $T = 6$ Periods)

The synthetic `MAD` group-item allocation family ([`BuildMADFromWeights`](bench/corpus.go)) assigns $I$ items across $G$ groups over $T = 6$ periods to minimize total L1 distance from each group's central level $m_g$ plus the roundness penalty $r_g = |m_g - 100 n_g|$ relative to an integer multiple $n_g \in \mathbb{Z}_{\ge 1}$. In the planted-gap generator, items in $S = \{0, \dots, I-G-1\}$ carry orthogonal curve signatures cancelled by the $G$ designated items $D = \{I-G, \dots, I-1\}$ with roundness-aligned period sums, yielding a zero root LP gap that root heuristics close without branching; unstructured random-weight negative controls (`BuildMADRandom`: `t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`) are included directly in the corpus and detailed in [`docs/benchmarks.md`](docs/benchmarks.md).

<!-- BEGIN GENERATED:MAD_PIPELINE_TABLE -->
| Problem ID | $G \times I$ | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Trial 0 Obj | Parity |
| :--- | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **102.3x** [86.1x-124.9x] | 0.09 | 9.62 | 0.06 | 9.39 | 0.035 | 0.224 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **68.3x** [29.9x-119.5x] | 0.48 | 34.12 | 0.45 | 33.86 | 0.025 | 0.247 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **44.4x** [33.6x-61.7x] | 0.22 | 9.66 | 0.15 | 9.39 | 0.061 | 0.276 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **55.3x** [42.3x-66.5x] | 0.18 | 10.24 | 0.12 | 9.82 | 0.046 | 0.411 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **29.6x** [23.8x-38.4x] | 0.42 | 11.46 | 0.32 | 10.85 | 0.080 | 0.612 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **10.0x** [3.5x-25.8x] | 2.98 | 47.85 | 2.94 | 47.56 | 0.039 | 0.300 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **7.1x** [1.4x-29.2x] | 16.56 | 168.44 | 16.49 | 168.01 | 0.064 | 0.435 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **31.3x** [25.0x-36.4x] | 0.56 | 18.26 | 0.45 | 17.04 | 0.112 | 1.014 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **16.6x** [10.2x-20.8x] | 1.01 | 18.82 | 0.82 | 16.99 | 0.261 | 1.778 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **5.8x** [1.1x-13.3x] | 8.44 | 61.14 | 7.52 | 53.28 | 1.075 | 8.316 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **10.3x** [4.5x-24.7x] | 33.84 | 375.12 | 30.22 | 340.11 | 3.936 | 35.304 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

Cross-instance total latency dispersion across the randomized instances in each family (mean $\pm$ sample SD and two-sided 95% Student-$t$ interval after warmup):

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.09 \pm 0.01$ ($\pm 0.01$) | $9.58 \pm 0.43$ ($\pm 0.29$) | $0.09 \pm 0.01$ ($\pm 0.01$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.50 \pm 0.28$ ($\pm 0.19$) | $31.01 \pm 9.64$ ($\pm 6.47$) | $0.50 \pm 0.27$ ($\pm 0.18$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.22 \pm 0.03$ ($\pm 0.02$) | $9.73 \pm 0.63$ ($\pm 0.43$) | $0.16 \pm 0.01$ ($\pm 0.00$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.19 \pm 0.03$ ($\pm 0.02$) | $10.20 \pm 0.30$ ($\pm 0.20$) | $0.25 \pm 0.04$ ($\pm 0.03$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.40 \pm 0.04$ ($\pm 0.03$) | $11.77 \pm 0.88$ ($\pm 0.59$) | $0.36 \pm 0.02$ ($\pm 0.01$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $4.12 \pm 3.12$ ($\pm 2.10$) | $35.91 \pm 17.20$ ($\pm 11.56$) | $4.12 \pm 3.14$ ($\pm 2.11$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $22.71 \pm 22.51$ ($\pm 15.12$) | $139.08 \pm 63.73$ ($\pm 42.81$) | $22.36 \pm 21.97$ ($\pm 14.76$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.56 \pm 0.03$ ($\pm 0.03$) | $17.78 \pm 2.40$ ($\pm 2.98$) | $0.56 \pm 0.02$ ($\pm 0.02$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.18 \pm 0.37$ ($\pm 0.46$) | $18.84 \pm 0.40$ ($\pm 0.50$) | $1.16 \pm 0.38$ ($\pm 0.47$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $16.68 \pm 20.40$ ($\pm 25.32$) | $63.19 \pm 3.53$ ($\pm 4.38$) | $16.06 \pm 20.09$ ($\pm 24.95$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $42.14 \pm 14.72$ ($\pm 18.27$) | $584.11 \pm 614.36$ ($\pm 762.71$) | $39.55 \pm 15.72$ ($\pm 19.51$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### 3. Full 31-Family Synthetic Benchmark Corpus (`JSON -> Solution`)

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | **602.9x** [311.8x-850.4x] | 0.01 | 6.36 | 0.00 | 6.30 | 0.005 | 0.055 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **631.9x** [321.8x-1129.7x] | 0.01 | 6.31 | 0.00 | 6.25 | 0.005 | 0.053 | `OK` |
| `t1_std_mip` | 3 | 3 | **700.3x** [587.3x-929.0x] | 0.01 | 7.71 | 0.00 | 7.64 | 0.005 | 0.064 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **747.3x** [554.4x-1093.3x] | 0.01 | 7.39 | 0.00 | 7.34 | 0.006 | 0.058 | `OK` |
| `t1_whiskas` | 6 | 5 | **559.6x** [383.6x-816.4x] | 0.01 | 7.24 | 0.01 | 7.15 | 0.007 | 0.099 | `OK` |
| `t1_furniture` | 2 | 3 | **252.0x** [85.2x-643.1x] | 0.03 | 7.90 | 0.02 | 7.85 | 0.006 | 0.055 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **92.1x** [40.5x-297.7x] | 0.12 | 9.41 | 0.11 | 9.31 | 0.013 | 0.100 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **824.5x** [300.0x-1235.0x] | 0.01 | 6.26 | 0.00 | 6.21 | 0.005 | 0.048 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **102.3x** [86.1x-124.9x] | 0.09 | 9.62 | 0.06 | 9.39 | 0.035 | 0.224 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **68.3x** [29.9x-119.5x] | 0.48 | 34.12 | 0.45 | 33.86 | 0.025 | 0.247 | `OK` |
| `t2_mkp_30` | 30 | 6 | **11.4x** [6.0x-16.3x] | 2.14 | 28.00 | 2.12 | 27.83 | 0.018 | 0.171 | `OK` |
| `t2_mkp_50` | 50 | 8 | **5.9x** [3.1x-35.4x] | 4.14 | 16.28 | 4.11 | 16.05 | 0.032 | 0.248 | `OK` |
| `t2_cflp_36` | 36 | 11 | **60.6x** [25.7x-174.5x] | 0.16 | 9.42 | 0.14 | 9.25 | 0.016 | 0.173 | `OK` |
| `t2_cflp_49` | 49 | 13 | **33.1x** [20.9x-92.7x] | 0.38 | 10.54 | 0.35 | 10.33 | 0.030 | 0.211 | `OK` |
| `t2_gap_45` | 45 | 13 | **21.5x** [4.8x-76.1x] | 0.98 | 14.74 | 0.96 | 14.53 | 0.020 | 0.209 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **13.2x** [5.7x-33.1x] | 3.30 | 69.49 | 3.28 | 69.25 | 0.029 | 0.247 | `OK` |
| `t2_binpack_44` | 44 | 14 | **12.3x** [0.1x-44.6x] | 0.58 | 14.45 | 0.56 | 14.23 | 0.022 | 0.211 | `OK` |
| `t2_setcover_50` | 50 | 20 | **51.6x** [6.9x-143.6x] | 0.19 | 10.92 | 0.15 | 10.63 | 0.032 | 0.304 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **44.4x** [33.6x-61.7x] | 0.22 | 9.66 | 0.15 | 9.39 | 0.061 | 0.276 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **55.3x** [42.3x-66.5x] | 0.18 | 10.24 | 0.12 | 9.82 | 0.046 | 0.411 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **29.6x** [23.8x-38.4x] | 0.42 | 11.46 | 0.32 | 10.85 | 0.080 | 0.612 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **10.0x** [3.5x-25.8x] | 2.98 | 47.85 | 2.94 | 47.56 | 0.039 | 0.300 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **7.1x** [1.4x-29.2x] | 16.56 | 168.44 | 16.49 | 168.01 | 0.064 | 0.435 | `OK` |
| `t3_mkp_200` | 200 | 8 | **5.4x** [4.1x-6.9x] | 19.33 | 121.10 | 19.24 | 120.48 | 0.082 | 0.625 | `OK` |
| `t3_cflp_300` | 300 | 34 | **28.3x** [8.2x-76.0x] | 6.16 | 248.24 | 6.07 | 247.29 | 0.103 | 0.929 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **2.6x** [1.1x-4.9x] | 63.06 | 157.79 | 62.87 | 155.86 | 0.199 | 1.928 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **10.9x** [5.4x-17.4x] | 7.35 | 80.86 | 6.92 | 77.94 | 0.302 | 2.820 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **31.3x** [25.0x-36.4x] | 0.56 | 18.26 | 0.45 | 17.04 | 0.112 | 1.014 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **16.6x** [10.2x-20.8x] | 1.01 | 18.82 | 0.82 | 16.99 | 0.261 | 1.778 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.8x** [1.1x-13.3x] | 8.44 | 61.14 | 7.52 | 53.28 | 1.075 | 8.316 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **10.3x** [4.5x-24.7x] | 33.84 | 375.12 | 30.22 | 340.11 | 3.936 | 35.304 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

### 4. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus ([`bench/assign_corpus.go`](bench/assign_corpus.go), 200 randomized instances across square, rectangular wide/tall, degenerate integer-tie, Euclidean geometric, rank-1 outer-product, sparse `+Inf`, and maximization families) compares [`assign.LinearSumFlat`](assign/assign.go) against SciPy's C++ `rectangular_lsap` extension (`scipy.optimize.linear_sum_assignment`) in warm state on bit-identical `float64` cost matrices:

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `SciPy Solve / goblin` GeoMean [Clustered 95% CI] | Pooled Trial `SciPy Solve / goblin` | Equal-Family `SciPy Total / goblin` | `goblin` Solve Trial (Med) Sum (ms) | SciPy Solve Trial (Med) Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.43x** [1.05x-1.96x] | 1.43x | 4.75x | 0.242 (0.0213) | 0.313 (0.0287) | 1.54x [0.46x-3.42x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **2.88x** [1.76x-4.73x] | 2.88x | 2.90x | 10.232 (0.801) | 32.691 (2.942) | 2.55x [0.17x-15.94x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.82x** [1.83x-4.32x] | 2.82x | 3.48x | 219.915 (42.870) | 933.611 (185.209) | 2.59x [1.46x-5.88x] |
| Square families ($m = n$) | 16 (146) | **2.69x** [1.96x-3.70x] | 2.63x | 3.79x | 225.544 (42.866) | 955.807 (186.248) | 2.28x [0.17x-15.94x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.68x** [1.11x-2.54x] | 1.55x | 2.87x | 4.844 (0.826) | 10.808 (1.931) | 1.90x [0.46x-2.65x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.16x** [1.05x-4.44x] | 2.27x | 2.74x | 36.951 (6.268) | 64.975 (11.013) | 1.77x [0.17x-15.94x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.37x** [1.83x-3.06x] | 2.28x | 3.51x | 230.388 (43.692) | 966.615 (188.179) | 2.02x [0.17x-15.94x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **2.23x** | 0.0004 | 0.0009 | 0.0005 | 0.0107 | $0.0004 \pm 0.0001$ | $0.0010 \pm 0.0003$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.51x** | 0.0016 | 0.0024 | 0.0019 | 0.0103 | $0.0016 \pm 0.0004$ | $0.0024 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.67x** | 0.0036 | 0.0062 | 0.0045 | 0.0153 | $0.0037 \pm 0.0010$ | $0.0061 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **0.97x** | 0.0039 | 0.0041 | 0.0050 | 0.0143 | $0.0043 \pm 0.0016$ | $0.0039 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.11x** | 0.0043 | 0.0043 | 0.0052 | 0.0149 | $0.0041 \pm 0.0012$ | $0.0044 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.44x** | 0.0075 | 0.0109 | 0.0120 | 0.0331 | $0.0079 \pm 0.0038$ | $0.0106 \pm 0.0018$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.71x** | 0.0279 | 0.0704 | 0.0345 | 0.0964 | $0.0263 \pm 0.0057$ | $0.0704 \pm 0.0123$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.22x** | 0.0536 | 0.179 | 0.0693 | 0.228 | $0.0546 \pm 0.0072$ | $0.177 \pm 0.0262$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.00x** | 0.0474 | 0.0935 | 0.0714 | 0.141 | $0.0488 \pm 0.0143$ | $0.0959 \pm 0.0139$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.32x** | 0.140 | 0.217 | 0.147 | 0.253 | $0.246 \pm 0.319$ | $0.235 \pm 0.0422$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **12.33x** | 0.0596 | 0.683 | 0.0759 | 0.765 | $0.0572 \pm 0.0106$ | $0.696 \pm 0.0252$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.89x** | 0.0346 | 0.0692 | 0.0626 | 0.143 | $0.0394 \pm 0.0139$ | $0.0716 \pm 0.0078$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **1.97x** | 0.0383 | 0.0788 | 0.0633 | 0.155 | $0.0419 \pm 0.0123$ | $0.0803 \pm 0.0069$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.42x** | 0.0796 | 0.285 | 0.105 | 0.363 | $0.0841 \pm 0.0161$ | $0.285 \pm 0.0314$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.84x** | 0.320 | 1.265 | 0.444 | 1.677 | $0.332 \pm 0.0551$ | $1.262 \pm 0.119$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.80x** | 1.262 | 5.189 | 1.860 | 7.724 | $1.409 \pm 0.279$ | $5.291 \pm 0.393$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.70x** | 0.872 | 1.490 | 1.484 | 4.088 | $0.874 \pm 0.0414$ | $1.483 \pm 0.0529$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.65x** | 5.137 | 8.512 | 5.350 | 10.263 | $5.716 \pm 2.150$ | $9.218 \pm 2.626$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.57x** | 0.310 | 0.817 | 0.727 | 3.578 | $0.326 \pm 0.0266$ | $0.837 \pm 0.0590$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.20x** | 0.435 | 0.957 | 0.838 | 3.672 | $0.445 \pm 0.0598$ | $0.972 \pm 0.0360$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.86x** | 5.969 | 27.954 | 8.856 | 36.545 | $5.848 \pm 0.253$ | $28.466 \pm 2.249$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.80x** | 28.884 | 140.290 | 41.337 | 188.961 | $29.365 \pm 3.423$ | $140.456 \pm 9.698$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

---

## Pure-Go vs. C++ CBC Performance Gaps and MAD Scaling Analysis

Comparing isolated unpresolved `Root LP` timings, paired trial ratios, and solver-core checkpoints (`CP5 +Presolve`, `CP6 +Cuts`, `CP7 Full`) between `BackendGo` and in-process `BackendCBC` (`c1c171ec`) highlights four structural trade-offs on large degenerate LPs and heavy-tail branch-and-cut models.

### 1. MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families (`N = G(I + T + 3)` variables, `M = G(2T + 3) + I` constraints) separate solver performance into three distinct scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Paired `Go / CBC` | Go Total Med (ms) | CBC Total Med (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.04x** | 0.09 | 2.52 | 0.65x | 0.06 (1 / 19) | 2.49 (0 / 0) | 0.090 (28) | 0.140 (19) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.05x** | 0.48 | 11.38 | 0.72x | 0.45 (2 / 58) | 11.35 (6 / 83) | 0.105 (30) | 0.145 (18) |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.11x** | 0.22 | 1.78 | 0.72x | 0.15 (1 / 26) | 1.74 (0 / 0) | 0.103 (33) | 0.143 (21) |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.07x** | 0.18 | 2.48 | 0.98x | 0.12 (1 / 40) | 2.42 (0 / 0) | 0.244 (58) | 0.248 (40) |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.12x** | 0.42 | 3.19 | 0.90x | 0.32 (1 / 51) | 3.11 (0 / 0) | 0.356 (80) | 0.397 (63) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.25x** | 2.98 | 16.69 | 0.99x | 2.94 (43 / 437) | 16.64 (12 / 216) | 0.153 (38) | 0.154 (24) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.25x** | 16.56 | 88.28 | 0.98x | 16.49 (201 / 2102) | 88.21 (198 / 1300) | 0.275 (65) | 0.282 (54) |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.07x** | 0.56 | 8.20 | 1.00x | 0.45 (1 / 75) | 8.06 (0 / 0) | 0.653 (91) | 0.653 (94) |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.15x** | 1.01 | 7.43 | 1.49x | 0.82 (1 / 98) | 7.23 (0 / 0) | 1.607 (244) | 1.078 (149) |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **0.30x** | 8.44 | 36.17 | 0.75x | 7.52 (2 / 364) | 35.14 (0 / 0) | 2.736 (224) | 3.628 (319) |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **0.17x** | 33.84 | 277.18 | 1.66x | 30.22 (1 / 2439) | 273.41 (0 / 0) | 58.441 (1900) | 35.297 (3128) |
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
| `t3_mkp_200` | $200 \times 8$ | **0.44x** | $19.19 \pm 10.11$ | $40.81 \pm 19.27$ | 19.24 (250 / 2447) | 49.91 (154 / 1145) | 82.79 (1135 / 16) | 165.09 (5000, `NodeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **0.36x** | $5.92 \pm 6.24$ | $12.30 \pm 9.06$ | 4.11 (1 / 19) | 8.56 (0 / 8) | 11.61 (2 / 4) | 4.76 (118) |
| `t2_binpack_44` | $44 \times 14$ | **0.19x** | $15.24 \pm 46.88$ | $6.59 \pm 2.17$ | 0.56 (2 / 51) | 6.01 (0 / 8) | 1.02 (5 / 8) | 8.79 (772) |
| `t3_cflp_300` | $300 \times 34$ | **0.15x** | $10.79 \pm 9.12$ | $55.37 \pm 11.91$ | 6.07 (1 / 106) | 50.44 (18 / 483) | 17.96 (54 / 34) | 8.68 (116) |
| `t3_lotsizing_504` | $504 \times 336$ | **0.66x** | $80.17 \pm 75.94$ | $93.33 \pm 57.58$ | 62.87 (2 / 1061) | 71.51 (10 / 377) | 79.69 (376 / 206) | 250.08 (3697, `TimeLimit`) |
| `t3_sched_1000` | $1000 \times 100$ | **0.12x** | $8.34 \pm 4.29$ | $61.25 \pm 4.68$ | 6.92 (31 / 474) | 59.91 (0 / 76) | 19.28 (87 / 53) | 250.07 (2550, `TimeLimit`) |
<!-- END GENERATED:NON_MAD_CBC_GAP_TABLE -->

---

## Verification Scope and Architectural History

The current codebase has passed full pure-Go, `GOEXPERIMENT=simd`, `-tags cbc`, race-detector, linter, differential-test, and `GOOS=linux GOARCH=arm64` / `GOAMD64=v3`/`v4` cross-compilation gates on the local AVX2 host (`i9-13900K`). Physical AVX-512 and `linux/arm64` target runtime re-verification remain pending target hardware access.

### Architectural Context and Rejected Designs

1. **LU Updates and Row-Singleton Stability (`lu.go`)**: The factorizer maintains elbow-room $U$ arenas (`uRowStart`/`uColStart`) paired with rank-free in-place mutated-$U$ Forrest-Tomlin updates (`PushEta`/`PushEtaSparse`) up to `maxEtaUpdates = 56`, avoiding extra branch checks during `FTRAN` and `BTRAN`. Two alternatives were tested and rejected earlier: (a) a lazy-rank Forrest-Tomlin prototype with `maxEtaUpdates = 128` regressed `miplib_p0201` and inflated `netlib_brandy` pivots from `uRank` branch mispredictions; (b) removing the `luMarkowitzTol * colMax` check on Stage 2 row singletons caused unscaled `1000x` multipliers in $L$ and numerical stalling on `t3_mad_g4_i1000`.
2. **Root LP Crash Basis and Dense Coupling (`simplex.go`)**: Dual Phase I (`installDualPhase1Bounds`) keeps the public Netlib LPs inside sparse dual simplex without falling back to primal simplex, and [`crashEqualityBasis`](simplex.go) accelerates `t3_mad_g10_i100` unpresolved Root LP setup vs. slack bases. Crashing columns incident to dense coupling rows (`> 128` nonzeros) was rejected because displacing 1-NNZ artificial slacks on `t3_mad_g4_i1000` increased $U$ fill-in and solve latency.
3. **Cut Separation, Symmetry, and Zero-Cut Sub-MIP Gating (`cuts.go`, `presolve.go`, `bab.go`, `heuristics.go`)**: Root cut rounds enforce GMI/MIR density caps (`maxCutNNZ`), exit after round 1 on zero bound gain, and purge inactive slack cuts both before branching and periodically in-tree via [`PurgeSlackCuts`](simplex.go). On zero-cut roots, a blanket `< 48`-integer sub-MIP ban helped median latency on small seeds at the cost of mean and tail regressions; requiring 50% fixed integers (RINS criterion) improved both median and tail behavior across all trials.
4. **Architectural Summary**:
   - **Numerical and Bridge Hardening**: Multi-scale three-bucket Shewchuk summation (`wideDotAcc`, `assignWideAcc`), compensated residual verification and Farkas exact-dot certificates (`BTRANExact`), exact `*big.Rat` basis dual polishing and LP/MILP Meyer unboundedness certification (`ExactBasisDuals`, `ExtractUnboundedCertificate` in [`exact_polish.go`](exact_polish.go)), zero-observer-effect Branch-and-Cut proof generation (`WithVIPRTrace` in [`vipr_trace.go`](vipr_trace.go)), strict `lb > ub` validation across presolve and simplex, and CBC tiny-coefficient, quotient-bound, scaling, and drift guards (`cbc_cgo.go`, `cbc_bridge.cpp`).
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

# 11. Lean 4 Prove Lane build (72 jobs), CLI/JSONL subcommands, arm64 ELF verification, and 19 formal Fuzz corpora
./scripts/verify_formal.sh
PYTHONDONTWRITEBYTECODE=1 python3 scripts/extract_elf_track5.py --verify
```

---

## License

The `goblin` module and its pure-Go default build (`BackendGo`, `goblin/assign`, `goblin/format`) are distributed under the [MIT License](LICENSE).

Optional vendored COIN-OR sources under `third_party/` (`Cbc`, `Cgl`, `Clp`, `CoinUtils`, and `Osi`, pinned in [`third_party/VERSIONS`](third_party/VERSIONS) and excluded from standard `go get` module downloads via `third_party/go.mod`) and adapted COIN-OR test fixtures in [`cbc_ported_test.go`](cbc_ported_test.go) are governed by the Eclipse Public License 2.0 (`EPL-2.0`; license texts in `third_party/*/LICENSE`). When distributing binaries compiled with `-tags cbc` that link those COIN-OR libraries, recipients may obtain the corresponding COIN-OR source code from `third_party/` in the upstream repository at `https://github.com/paulourio/goblin` (or from `https://github.com/coin-or`) under the terms of `EPL-2.0` Section 3.
