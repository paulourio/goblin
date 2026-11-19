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
3. **Optional SIMD Kernels**: With `GOEXPERIMENT=simd` on `amd64` (gated at runtime on CPU `AVX2` and `FMA` support), [`internal/simd/kernels_amd64.go`](internal/simd/kernels_amd64.go) dispatches 256-bit AVX2 `archsimd.Float64x4` instructions (`VFMADD213PD` in `Dot64`; `VMULPD` + `VADDPD` in `Axpy64`; `VSUBPD` + `VMINPD` in `MinReducedSlice64` and `InitReducedRow64`; `VSUBPD` + `VCMPPD`/`VPAND`/`VMOVMSKPD` in `Min2ReducedRow64` at length 64 or more; 8-lane `Abs().LessEqual` in `AllAbsLE64`, gated at 256 elements in `assign/solver.go`) with `VZEROUPPER` cleanup and IEEE-754 `NaN` propagation parity across scalar and vector paths, and can be toggled at runtime via `WithoutSIMD()` or `WithSIMD(false)`. See the paired comparison in [`docs/benchmarks.md`](docs/benchmarks.md#5-default-build-vs-goexperimentsimd).
4. **Verified Parity on Synthetic and Public Suites**: [`solver_test.go`](solver_test.go) re-solves all 293 distinct randomized trial instances of the 31 synthetic benchmark families (`testdata/corpus/trials/*.jsonl`), compares status and objective with `PuLP + COIN-OR CBC` (`c1c171ec`) and `BackendCBC`, and verifies primal and row feasibility on every result. [`public_benchmarks_test.go`](public_benchmarks_test.go) verifies optimal status, objective parity, and primal feasibility across 16 held-out public Netlib LP and MIPLIB/COIN-OR MILP instances (`testdata/public/`) plus a valid dual lower bound under a 25-node budget (`go_max_nodes: 25`) on `coinor_capplan1`, while [`regression_test.go`](regression_test.go) tests big-M fixed-charge models against enumerated ground truth and Farkas-certified LP scaling edge cases. `TimeLimit`, `MaxNodes`, and `MaxIter` return `StatusTimeLimit`, `StatusNodeLimit`, or `StatusIterationLimit` with a valid global bound once the root LP is solved; a node that cannot be evaluated is never discarded silently.

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
- **Unified telemetry source**: fingerprint `743b1e77616c6db6` (commit `bf1fe0d7d320fac6cfefd176a27efe3bef0f291f`), `go1.27.3-X:simd`, Linux kernel `6.18.63-gentoo-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, or kernel differs.
- **CPU affinity**: goblin `6`; Python + CBC `6`; Python + SciPy `6`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 7.21 ms median on a 1-variable MILP and 6.81 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

Read the synthetic corpus ratios with three methodological notes. First, Tier 1 micro-models (`N <= 26`) are dominated by Python's CBC subprocess spawn floor reported in the provenance block above. Second, the planted-gap MAD family has a zero root LP gap by construction and closes at the root without branching. Consequently, the bold row (`Tier 2+3 non-planted MILPs`) isolates solver-dominated MILPs across paired randomized instances, alongside total subset wall times and full per-instance `[Min-Max]` ranges.

### 1. Aggregate Synthetic Workload Summary (Paired Per-Instance Geometric Mean and Wall-Clock Sums)

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Instances) | Paired `Py / Go` GeoMean [95% CI] | Paired `Go / CBC` | `Go` Sum (ms) | `CBC` Sum (ms) | `Py` Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **424.01x** [330.79x-543.51x] | 0.04x | 19.1 | 200.4 | 1147.5 | 669.67x [10.95x-2723.98x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **15.22x** [12.41x-18.66x] | 0.21x | 1024.2 | 1956.9 | 4466.3 | 17.69x [0.10x-174.55x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **5.72x** [3.87x-8.45x] | 0.33x | 7431.1 | 3574.5 | 6334.9 | 7.30x [0.04x-36.01x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **8.93x** [7.11x-11.22x] | 0.31x | 8080.7 | 3105.7 | 6973.4 | 8.50x [0.04x-174.55x] |
| Tier 3 non-MAD MILPs | 4 (20) | **2.98x** [1.58x-5.63x] | 0.75x | 7065.5 | 1246.8 | 2893.3 | 3.13x [0.04x-19.99x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **35.52x** [27.77x-45.44x] | 0.09x | 375.4 | 2452.7 | 3942.5 | 40.92x [2.09x-166.44x] |
| Random-weight MAD (negative controls) | 3 (33) | **9.89x** [6.84x-14.30x] | 0.24x | 475.0 | 1208.0 | 2282.9 | 9.37x [0.55x-81.37x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **50.04x** [36.75x-68.15x] | 0.13x | 8098.9 | 3279.1 | 8006.2 | 29.05x [0.04x-2723.98x] |
| All 31 synthetic corpus families | 31 (293) | **46.43x** [36.25x-59.48x] | 0.12x | 8474.3 | 5731.8 | 11948.7 | 36.01x [0.04x-2723.98x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2. Synthetic MAD Group-Item Allocation Family (`G` Groups $\times$ `I` Items, $T = 6$ Periods)

The synthetic `MAD` group-item allocation family ([`BuildMADFromWeights`](bench/corpus.go)) assigns $I$ items across $G$ groups over $T = 6$ periods to minimize total L1 distance from each group's central level $m_g$ plus the roundness penalty $r_g = |m_g - 100 n_g|$ relative to an integer multiple $n_g \in \mathbb{Z}_{\ge 1}$. In the planted-gap generator, items in $S = \{0, \dots, I-G-1\}$ carry orthogonal curve signatures cancelled by the $G$ designated items $D = \{I-G, \dots, I-1\}$ with roundness-aligned period sums, yielding a zero root LP gap that root heuristics close without branching; unstructured random-weight negative controls (`BuildMADRandom`: `t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`) are included directly in the corpus and detailed in [`docs/benchmarks.md`](docs/benchmarks.md).

<!-- BEGIN GENERATED:MAD_PIPELINE_TABLE -->
| Problem ID | $G \times I$ | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Trial 0 Obj | Parity |
| :--- | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **136.6x** [89.0x-166.4x] | 0.07 | 10.69 | 0.04 | 10.46 | 0.024 | 0.293 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **27.4x** [11.0x-81.4x] | 1.38 | 35.30 | 1.34 | 35.06 | 0.040 | 0.304 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **56.4x** [41.4x-93.4x] | 0.19 | 10.38 | 0.13 | 10.09 | 0.043 | 0.380 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **53.2x** [35.9x-69.7x] | 0.20 | 11.42 | 0.13 | 11.00 | 0.063 | 0.554 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **32.9x** [21.9x-40.5x] | 0.37 | 12.96 | 0.29 | 12.34 | 0.083 | 0.817 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **8.3x** [3.1x-29.0x] | 5.19 | 49.02 | 5.11 | 48.72 | 0.045 | 0.392 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **4.3x** [0.6x-16.4x] | 34.10 | 167.82 | 33.93 | 167.39 | 0.083 | 0.566 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **25.3x** [18.9x-36.0x] | 0.76 | 19.39 | 0.57 | 18.31 | 0.190 | 1.408 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **10.5x** [2.1x-19.7x] | 1.47 | 19.89 | 1.20 | 18.14 | 0.276 | 2.295 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **7.1x** [6.6x-7.6x] | 9.15 | 61.25 | 7.73 | 53.71 | 1.233 | 10.698 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **7.6x** [3.5x-13.5x] | 43.51 | 375.64 | 38.53 | 341.71 | 4.837 | 44.357 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

Cross-instance total latency dispersion across the randomized instances in each family (mean $\pm$ sample SD and two-sided 95% Student-$t$ interval after warmup):

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.08 \pm 0.02$ ($\pm 0.01$) | $10.42 \pm 0.65$ ($\pm 0.44$) | $0.08 \pm 0.02$ ($\pm 0.01$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $1.48 \pm 1.03$ ($\pm 0.69$) | $31.73 \pm 9.52$ ($\pm 6.40$) | $1.43 \pm 0.99$ ($\pm 0.66$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.19 \pm 0.05$ ($\pm 0.03$) | $10.41 \pm 0.59$ ($\pm 0.40$) | $0.18 \pm 0.04$ ($\pm 0.03$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.22 \pm 0.05$ ($\pm 0.03$) | $11.55 \pm 0.27$ ($\pm 0.18$) | $0.22 \pm 0.05$ ($\pm 0.03$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.41 \pm 0.08$ ($\pm 0.05$) | $13.15 \pm 0.59$ ($\pm 0.40$) | $0.43 \pm 0.09$ ($\pm 0.06$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $5.34 \pm 3.34$ ($\pm 2.24$) | $36.67 \pm 17.08$ ($\pm 11.48$) | $5.58 \pm 3.44$ ($\pm 2.31$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $36.35 \pm 25.34$ ($\pm 17.02$) | $139.14 \pm 63.32$ ($\pm 42.54$) | $36.31 \pm 24.64$ ($\pm 16.55$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.76 \pm 0.13$ ($\pm 0.16$) | $19.18 \pm 2.39$ ($\pm 2.97$) | $0.68 \pm 0.07$ ($\pm 0.09$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $2.90 \pm 3.59$ ($\pm 4.46$) | $19.91 \pm 0.54$ ($\pm 0.67$) | $2.83 \pm 3.47$ ($\pm 4.31$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $8.96 \pm 0.49$ ($\pm 0.61$) | $63.78 \pm 3.83$ ($\pm 4.76$) | $8.94 \pm 1.13$ ($\pm 1.40$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $60.49 \pm 35.69$ ($\pm 44.31$) | $585.45 \pm 613.38$ ($\pm 761.49$) | $60.89 \pm 34.79$ ($\pm 43.19$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### 3. Full 31-Family Synthetic Benchmark Corpus (`JSON -> Solution`)

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | **860.2x** [609.2x-1326.3x] | 0.01 | 6.94 | 0.00 | 6.82 | 0.005 | 0.087 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **942.8x** [442.8x-1404.0x] | 0.01 | 6.76 | 0.00 | 6.70 | 0.004 | 0.074 | `OK` |
| `t1_std_mip` | 3 | 3 | **962.4x** [678.3x-1175.6x] | 0.01 | 8.56 | 0.00 | 8.50 | 0.003 | 0.088 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **1018.8x** [880.6x-1197.5x] | 0.01 | 7.98 | 0.00 | 7.92 | 0.004 | 0.091 | `OK` |
| `t1_whiskas` | 6 | 5 | **599.6x** [406.6x-726.5x] | 0.01 | 6.93 | 0.00 | 6.83 | 0.006 | 0.111 | `OK` |
| `t1_furniture` | 2 | 3 | **343.2x** [89.6x-958.5x] | 0.02 | 8.62 | 0.01 | 8.55 | 0.003 | 0.082 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **127.1x** [56.0x-291.1x] | 0.07 | 10.22 | 0.07 | 10.13 | 0.009 | 0.130 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **2412.7x** [1852.4x-2724.0x] | 0.00 | 6.72 | 0.00 | 6.66 | 0.002 | 0.073 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **136.6x** [89.0x-166.4x] | 0.07 | 10.69 | 0.04 | 10.46 | 0.024 | 0.293 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **27.4x** [11.0x-81.4x] | 1.38 | 35.30 | 1.34 | 35.06 | 0.040 | 0.304 | `OK` |
| `t2_mkp_30` | 30 | 6 | **6.8x** [3.9x-12.7x] | 3.31 | 30.18 | 3.29 | 30.00 | 0.023 | 0.237 | `OK` |
| `t2_mkp_50` | 50 | 8 | **4.0x** [1.8x-32.8x] | 6.77 | 17.57 | 6.71 | 17.31 | 0.056 | 0.347 | `OK` |
| `t2_cflp_36` | 36 | 11 | **31.1x** [2.9x-174.5x] | 0.28 | 10.65 | 0.25 | 10.47 | 0.023 | 0.239 | `OK` |
| `t2_cflp_49` | 49 | 13 | **17.6x** [7.6x-29.0x] | 0.69 | 11.37 | 0.64 | 11.15 | 0.034 | 0.286 | `OK` |
| `t2_gap_45` | 45 | 13 | **9.6x** [1.3x-30.3x] | 1.24 | 14.93 | 1.22 | 14.73 | 0.028 | 0.271 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **8.4x** [4.4x-21.0x] | 7.03 | 69.97 | 6.96 | 69.73 | 0.046 | 0.310 | `OK` |
| `t2_binpack_44` | 44 | 14 | **11.7x** [0.1x-36.1x] | 0.63 | 14.72 | 0.61 | 14.51 | 0.035 | 0.267 | `OK` |
| `t2_setcover_50` | 50 | 20 | **48.3x** [3.4x-132.5x] | 0.20 | 11.90 | 0.13 | 11.59 | 0.048 | 0.410 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **56.4x** [41.4x-93.4x] | 0.19 | 10.38 | 0.13 | 10.09 | 0.043 | 0.380 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **53.2x** [35.9x-69.7x] | 0.20 | 11.42 | 0.13 | 11.00 | 0.063 | 0.554 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **32.9x** [21.9x-40.5x] | 0.37 | 12.96 | 0.29 | 12.34 | 0.083 | 0.817 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **8.3x** [3.1x-29.0x] | 5.19 | 49.02 | 5.11 | 48.72 | 0.045 | 0.392 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **4.3x** [0.6x-16.4x] | 34.10 | 167.82 | 33.93 | 167.39 | 0.083 | 0.566 | `OK` |
| `t3_mkp_200` | 200 | 8 | **2.2x** [2.0x-2.5x] | 55.46 | 116.02 | 55.36 | 115.41 | 0.106 | 0.808 | `OK` |
| `t3_cflp_300` | 300 | 34 | **11.6x** [7.3x-20.0x] | 18.88 | 256.93 | 18.62 | 256.02 | 0.156 | 1.178 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **0.6x** [0.0x-1.4x] | 180.47 | 176.57 | 180.18 | 174.75 | 0.279 | 2.289 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **5.3x** [3.8x-6.9x] | 15.57 | 81.53 | 15.11 | 78.64 | 0.427 | 3.464 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **25.3x** [18.9x-36.0x] | 0.76 | 19.39 | 0.57 | 18.31 | 0.190 | 1.408 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **10.5x** [2.1x-19.7x] | 1.47 | 19.89 | 1.20 | 18.14 | 0.276 | 2.295 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **7.1x** [6.6x-7.6x] | 9.15 | 61.25 | 7.73 | 53.71 | 1.233 | 10.698 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **7.6x** [3.5x-13.5x] | 43.51 | 375.64 | 38.53 | 341.71 | 4.837 | 44.357 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

### 4. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus ([`bench/assign_corpus.go`](bench/assign_corpus.go), 200 randomized instances across square, rectangular wide/tall, degenerate integer-tie, Euclidean geometric, rank-1 outer-product, sparse `+Inf`, and maximization families) compares [`assign.LinearSumFlat`](assign/assign.go) against SciPy's C++ `rectangular_lsap` extension (`scipy.optimize.linear_sum_assignment`) in warm state on bit-identical `float64` cost matrices:

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Instances) | Paired `SciPy Solve / goblin` GeoMean [95% CI] | Paired `SciPy Total / goblin` GeoMean | `goblin` Solve Sum (ms) | SciPy Solve Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.79x** [1.61x-2.00x] | 4.63x | 0.182 | 0.315 | 1.88x [0.30x-3.95x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **2.49x** [2.19x-2.84x] | 2.33x | 11.756 | 33.625 | 2.25x [0.97x-14.58x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.89x** [2.54x-3.29x] | 3.80x | 249.878 | 961.292 | 2.81x [1.58x-5.56x] |
| Square families ($m = n$) | 16 (146) | **2.61x** [2.37x-2.88x] | 3.56x | 256.869 | 984.013 | 2.39x [0.30x-14.58x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.61x** [1.49x-1.75x] | 2.35x | 4.947 | 11.220 | 1.53x [0.87x-3.73x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.44x** [2.04x-2.91x] | 2.79x | 35.252 | 66.485 | 2.00x [0.60x-14.58x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.30x** [2.12x-2.49x] | 3.18x | 261.816 | 995.233 | 2.15x [0.30x-14.58x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.83x** | 0.0004 | 0.0009 | 0.0005 | 0.0113 | $0.0007 \pm 0.0009$ | $0.0009 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.95x** | 0.0012 | 0.0024 | 0.0016 | 0.0100 | $0.0013 \pm 0.0006$ | $0.0024 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **2.03x** | 0.0027 | 0.0063 | 0.0042 | 0.0150 | $0.0033 \pm 0.0022$ | $0.0061 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.58x** | 0.0024 | 0.0038 | 0.0039 | 0.0139 | $0.0026 \pm 0.0010$ | $0.0039 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.43x** | 0.0029 | 0.0042 | 0.0047 | 0.0150 | $0.0032 \pm 0.0009$ | $0.0044 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **2.03x** | 0.0051 | 0.0111 | 0.0086 | 0.0324 | $0.0055 \pm 0.0019$ | $0.0109 \pm 0.0022$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.72x** | 0.0275 | 0.0709 | 0.0347 | 0.0964 | $0.0258 \pm 0.0066$ | $0.0687 \pm 0.0105$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.58x** | 0.0527 | 0.192 | 0.0703 | 0.259 | $0.0550 \pm 0.0131$ | $0.195 \pm 0.0295$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.38x** | 0.0478 | 0.117 | 0.0791 | 0.180 | $0.0495 \pm 0.0136$ | $0.116 \pm 0.0248$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.19x** | 0.192 | 0.221 | 0.207 | 0.267 | $0.202 \pm 0.0541$ | $0.237 \pm 0.0437$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **9.58x** | 0.0776 | 0.685 | 0.111 | 0.760 | $0.0792 \pm 0.0235$ | $0.740 \pm 0.0954$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.51x** | 0.0476 | 0.0694 | 0.101 | 0.143 | $0.0472 \pm 0.0038$ | $0.0713 \pm 0.0078$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **1.29x** | 0.0621 | 0.0798 | 0.113 | 0.156 | $0.0621 \pm 0.0052$ | $0.0799 \pm 0.0066$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **2.33x** | 0.125 | 0.282 | 0.178 | 0.358 | $0.122 \pm 0.0163$ | $0.284 \pm 0.0308$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.13x** | 0.377 | 1.276 | 0.615 | 1.752 | $0.426 \pm 0.152$ | $1.265 \pm 0.122$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.96x** | 1.182 | 5.185 | 1.571 | 8.084 | $1.378 \pm 0.395$ | $5.317 \pm 0.396$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.93x** | 0.752 | 1.491 | 1.352 | 4.275 | $0.774 \pm 0.0616$ | $1.492 \pm 0.0574$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.72x** | 4.947 | 8.444 | 5.185 | 9.447 | $5.530 \pm 1.968$ | $9.362 \pm 2.830$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.98x** | 0.308 | 0.835 | 0.679 | 3.907 | $0.305 \pm 0.0248$ | $0.915 \pm 0.159$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.27x** | 0.428 | 0.973 | 0.830 | 4.103 | $0.432 \pm 0.0198$ | $0.978 \pm 0.0296$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.59x** | 6.263 | 28.084 | 8.112 | 37.978 | $6.246 \pm 0.547$ | $28.630 \pm 2.234$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.13x** | 35.107 | 147.154 | 44.454 | 195.534 | $35.311 \pm 2.746$ | $145.565 \pm 8.628$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

---

## Pure-Go vs. C++ CBC Performance Gaps and MAD Scaling Analysis

Comparing isolated unpresolved `Root LP` timings, paired trial ratios, and solver-core checkpoints (`CP5 +Presolve`, `CP6 +Cuts`, `CP7 Full`) between `BackendGo` and in-process `BackendCBC` (`c1c171ec`) highlights four structural trade-offs on large degenerate LPs and heavy-tail branch-and-cut models.

### 1. MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families (`N = G(I + T + 3)` variables, `M = G(2T + 3) + I` constraints) separate solver performance into three distinct scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Paired `Go / CBC` | Go Total Med (ms) | CBC Total Med (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.03x** | 0.07 | 2.67 | 0.59x | 0.04 (1 / 19) | 2.62 (0 / 42) | 0.083 (28) | 0.139 (19) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.11x** | 1.38 | 11.40 | 0.61x | 1.34 (10 / 232) | 11.36 (6 / 128) | 0.086 (30) | 0.140 (18) |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.09x** | 0.19 | 1.95 | 0.67x | 0.13 (1 / 26) | 1.91 (0 / 48) | 0.091 (33) | 0.136 (21) |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.08x** | 0.20 | 2.77 | 0.93x | 0.13 (1 / 39) | 2.71 (0 / 81) | 0.229 (59) | 0.246 (40) |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.10x** | 0.37 | 3.57 | 0.76x | 0.29 (1 / 51) | 3.48 (0 / 97) | 0.306 (68) | 0.404 (131) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.30x** | 5.19 | 16.69 | 1.02x | 5.11 (54 / 642) | 16.64 (12 / 269) | 0.144 (38) | 0.142 (24) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.42x** | 34.10 | 87.76 | 0.93x | 33.93 (250 / 3142) | 87.66 (198 / 1388) | 0.262 (66) | 0.281 (133) |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.08x** | 0.76 | 9.12 | 1.30x | 0.57 (1 / 75) | 8.97 (0 / 166) | 0.907 (132) | 0.695 (94) |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.20x** | 1.47 | 9.11 | 0.55x | 1.20 (1 / 105) | 8.88 (0 / 464) | 1.464 (423) | 2.645 (149) |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **0.18x** | 9.15 | 48.89 | 1.28x | 7.73 (2 / 357) | 47.72 (0 / 1099) | 15.042 (1031) | 11.712 (2079) |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **0.16x** | 43.51 | 442.69 | 0.18x | 38.53 (1 / 2439) | 436.72 (0 / 1890) | 111.432 (1865) | 634.085 (4993) |
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
| `t3_mkp_200` | $200 \times 8$ | **1.13x** | $46.62 \pm 23.03$ | $40.00 \pm 19.02$ | 55.36 (590 / 5308) | 49.01 (154 / 1145) | 62.98 (701 / 12) | 192.67 (5000, `NodeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **0.57x** | $10.09 \pm 11.66$ | $12.31 \pm 9.07$ | 6.71 (1 / 24) | 8.48 (0 / 8) | 13.97 (2 / 4) | 7.37 (132) |
| `t2_binpack_44` | $44 \times 14$ | **0.21x** | $21.33 \pm 64.43$ | $6.55 \pm 2.14$ | 0.61 (2 / 88) | 5.99 (0 / 8) | 1.39 (6 / 7) | 26.75 (1848) |
| `t3_cflp_300` | $300 \times 34$ | **0.36x** | $21.65 \pm 11.56$ | $55.02 \pm 11.72$ | 18.62 (38 / 1622) | 49.74 (18 / 483) | 18.65 (38 / 31) | 11.04 (134) |
| `t3_lotsizing_504` | $504 \times 336$ | **3.00x** | $1329.15 \pm 2688.98$ | $93.38 \pm 58.51$ | 180.18 (35 / 1615) | 70.88 (10 / 377) | 177.94 (420 / 205) | 250.03 (2267, `TimeLimit`) |
| `t3_sched_1000` | $1000 \times 100$ | **0.25x** | $15.68 \pm 3.75$ | $60.97 \pm 4.66$ | 15.11 (64 / 507) | 59.99 (0 / 76) | 52.06 (206 / 46) | 250.03 (1879, `TimeLimit`) |
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
