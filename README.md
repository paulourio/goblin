# `goblin`: Pure-Go LP, MILP, and Linear Sum Assignment Solver with Optional C++ CBC Backend

The `goblin` module provides an in-memory Linear Programming (LP) and Mixed-Integer Linear Programming (MILP) modeling engine and Branch-and-Cut solver alongside the [`goblin/assign`](assign/assign.go) Linear Sum Assignment Problem (`LSAP`) solver, written in Go 1.27. It compiles algebraic models directly into Dual-Index Compressed Sparse Row and Column (`CSR`/`CSC`) matrices (`CompiledProblem`), eliminating intermediate text-file I/O and subprocess spawn costs. By default it runs the pure-Go solver (`BackendGo`), and callers can select the in-process C++ COIN-OR CBC/CLP backend (`WithCBC()` / `WithBackend(BackendCBC)`) or configure solver flags (`WithoutSIMD()`, `WithTimeLimit(...)`, etc.) through functional arguments on `Solve` and `SolveCompiled`.

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

	// Or solve with the in-process C++ COIN-OR CBC backend or with SIMD disabled:
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

1. **Zero-Subprocess In-Memory Execution**: Modeling (`Model`), matrix checks and assembly (`Model.Validate()`, `Model.CompileChecked()`), presolve (`Presolve`), root and node LPs (`LPSolver`), and Branch-and-Cut (`SolveCompiled`) execute in-process using pooled workspace buffers (`AcquireWorkspace` / `ReleaseWorkspace`), with an optional in-process `cgo` bridge to COIN-OR CBC/CLP (`cbc_cgo.go`).
2. **Exact Schema Compatibility with Python `PuLP`**: The `goblin/format` package reads and writes `PuLP`'s native `toDict()` / `fromDict()` JSON schema ([`format/pulp_json.go`](format/pulp_json.go)) and standard fixed/free MPS files ([`format/mps.go`](format/mps.go)).
3. **Optional SIMD Kernels**: With `GOEXPERIMENT=simd` on `amd64`, [`internal/simd/kernels_amd64.go`](internal/simd/kernels_amd64.go) dispatches 256-bit AVX2 `archsimd.Float64x4` instructions (`VFMADD213PD` in `Dot64`; `VMULPD` + `VADDPD` and `VSUBPD` + `VMINPD` in `Axpy64`, `MinReducedSlice64`, `InitReducedRow64`, and `Min2ReducedRow64`; 8-lane `Abs().LessEqual` in `AllAbsLE64`) with `VZEROUPPER` cleanup for slices of length 32 or more, and can be toggled at runtime via `WithoutSIMD()` or `WithSIMD(false)`. See the paired comparison in [`docs/benchmarks.md`](docs/benchmarks.md#5-default-build-vs-goexperimentsimd).
4. **Verified Parity and Resource-Limit Contracts**: The suite in [`solver_test.go`](solver_test.go) re-solves all 293 randomized trial instances of the 31 benchmark families (`testdata/corpus/trials/*.jsonl`), compares status and objective with the frozen `PuLP + COIN-OR CBC 2.10.3` and `BackendCBC` results, and verifies that every `goblin` solution satisfies all bounds and rows. [`regression_test.go`](regression_test.go) adds big-M fixed-charge models with enumerated ground truth and replays fuzz instances on which `goblin` or CBC failed. `TimeLimit`, `MaxNodes`, and `MaxIter` return `StatusTimeLimit`, `StatusNodeLimit`, or `StatusIterationLimit` with a valid global bound once the root LP is solved; a node that cannot be evaluated is never discarded silently.

---

## Documentation Index

Full technical guides live in the [`docs/`](docs/index.md) directory:

- **[Architecture and Design Overview](docs/index.md)**: Module boundaries, data flow from algebraic expressions to `CSR`/`CSC` matrices, and memory pooling invariants.
- **[Modeling API and Serialization Formats](docs/modeling-and-formats.md)**: Constructing variables, linear expressions, and constraints, configuring solver flags and backends, validating and compiling models, and exchanging `PuLP` JSON and MPS payloads.
- **[Solver Internals and Mathematical Specification](docs/solver-internals.md)**: Three-stage singleton and sparse Markowitz LU with `FTRANSparse`/`BTRANSparse` and PFI eta updates, Revised Dual Simplex with Bound-Flipping Ratio Test (`selectDualBFRT`) and sparse `TabRowIdx` tableau rows, Two-Phase Primal Simplex, MIP presolve, cutting planes (`GMI`, Sequentially Lifted Knapsack Covers, Flow/VUB, `c-MIR`), primal heuristics, and Branch-and-Cut search.
- **[Benchmark Methodology and Corpus Results](docs/benchmarks.md)**: Hardware setup, comparative telemetry against Python `PuLP + CBC` and in-process `BackendCBC`, CBC spawn-floor separation, paired scalar-vs-SIMD statistics, structural analysis of the Mean Absolute Deviation (`MAD`) `G x I` allocation family vs. random-weight negative controls (`BuildMADRandom`), and full checkpoint telemetry.

---

## Benchmarks

All benchmark tables in this `README.md`, [`docs/benchmarks.md`](docs/benchmarks.md), and [`bench/results/final_report.md`](bench/results/final_report.md) are rendered from the JSON telemetry in `bench/results/` (`python_baseline.json`, `goblin_cbc.json`, `goblin_scalar.json`, `goblin_simd.json`). [`doc_sync_test.go`](doc_sync_test.go) fails `go test ./...` if a table drifts from the telemetry, if any `goblin` build disagrees with CBC on a status or objective, or if the telemetry lacks git provenance. Timings use the default (scalar) build.

<!-- BEGIN GENERATED:PROVENANCE -->
- **goblin telemetry source**: fingerprint `dd247e9560bd5139` (commit `b362b3890e081d9e18b37955c5dc58638a28627a`), `go1.27.3-X:nodwarf5`; the documentation gate rejects telemetry whose fingerprint differs from the checked-in sources.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, CBC `2.10.3` (`cbc`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 1.53 ms median (`prob.solve()` on a 1-variable MILP); the "Net of Spawn" columns subtract it.
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `master` (commit `c1c171ec`, vendored in `third_party/Cbc`), linked in-process through `cgo` (`gb.WithCBC()`). The C++ backend maps default settings onto CBC with two deliberate differences: `MaxCutPasses = 12` becomes `passC = 30` and `MaxNodes = 50000` becomes unlimited. It is therefore not a like-for-like setup and not the CBC 2.10.3 binary used by the Python baseline.
<!-- END GENERATED:PROVENANCE -->

Read the ratios with three caveats. Python's solve time includes a fixed CBC process-spawn cost, reported separately as "Net of Spawn". The `goblin (CBC)` column runs a different CBC build with different settings. The planted-gap MAD family is solved at the root by construction, so the bold row excludes it.

### 1. Aggregate Workload Summary (Geometric Mean and Median `[Min-Max]` Total Latency)

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families | `Py / goblin (Go)` GeoMean | `Py net / goblin (Go)` | `goblin (Go) / CBC` | `goblin (Go)` (ms) | `goblin (CBC)` (ms) | Py Total (ms) | Py Net (ms) | `Py / goblin (Go)` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical (`N <= 26`) | 10 | **67.3x** | 2.2x | 0.07x | 0.04 | 0.52 | 2.50 | 0.08 | 106.7x [8.5x-287.0x] |
| Tier 2: Target p90 (`30 <= N <= 84`) | 13 | **7.7x** | 5.9x | 0.23x | 1.43 | 6.22 | 11.03 | 8.43 | 8.6x [1.4x-21.7x] |
| Tier 3: Large Tail (`145 <= N <= 4036`) | 8 | **4.3x** | 4.1x | 0.34x | 13.95 | 40.99 | 59.90 | 56.86 | 4.5x [1.3x-16.0x] |
| Tier 3 non-MAD MILPs | 4 | **5.4x** | 5.3x | 0.29x | 17.14 | 58.26 | 92.48 | 90.63 | 6.7x [1.3x-16.0x] |
| Planted-gap MAD family (zero integrality gap by construction) | 8 | **6.1x** | 4.6x | 0.25x | 2.06 | 8.25 | 12.66 | 9.57 | 6.6x [1.7x-16.4x] |
| Random-weight MAD (negative controls) | 3 | **3.7x** | 3.6x | 0.61x | 15.86 | 26.06 | 58.88 | 56.80 | 4.2x [1.4x-8.5x] |
| **All families except planted-gap MAD** | 23 | **17.5x** | 3.6x | 0.15x | 0.57 | 3.69 | 9.93 | 2.07 | 12.2x [1.3x-287.0x] |
| All corpus families | 31 | **13.3x** | 3.9x | 0.17x | 0.79 | 4.54 | 10.57 | 3.08 | 8.8x [1.3x-287.0x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2. MAD Group-Item Allocation Family (`G` Groups $\times$ `I` Items, $T = 6$ Periods)

The `MAD` group-item allocation family ([`BuildMADFromWeights`](bench/corpus.go)) assigns $I$ items across $G$ groups over $T = 6$ periods to minimize total L1 distance from each group's central level $m_g$ plus the roundness penalty $r_g = |m_g - 100 n_g|$ relative to an integer multiple $n_g \in \mathbb{Z}_{\ge 1}$. In the planted-gap generator, items in $S = \{0, \dots, I-G-1\}$ carry orthogonal curve signatures cancelled by the $G$ designated items $D = \{I-G, \dots, I-1\}$ with roundness-aligned period sums, yielding a zero root LP gap that root heuristics close at node 1; unstructured random-weight negative controls (`BuildMADRandom`: `t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`) are included directly in the corpus and detailed in [`docs/benchmarks.md`](docs/benchmarks.md).

<!-- BEGIN GENERATED:MAD_PIPELINE_TABLE -->
| Problem ID | $G \times I$ | Vars | Rows | `Py / goblin (Go)` | GoTotal (ms) | PyTotal (ms) | GoSolve (ms) | PySolve (ms) | PySolve Net (ms) | GoPrep (ms) | PyPrep (ms) | Optimal Obj | Parity |
| :--- | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **15.9x** | 0.24 | 3.82 | 0.09 | 3.63 | 2.10 | 0.133 | 0.246 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **8.5x** | 2.92 | 24.75 | 2.81 | 24.53 | 22.99 | 0.111 | 0.281 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **16.4x** | 0.21 | 3.43 | 0.09 | 3.18 | 1.65 | 0.117 | 0.329 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **8.6x** | 0.49 | 4.21 | 0.28 | 3.83 | 2.30 | 0.213 | 0.486 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **6.5x** | 0.81 | 5.28 | 0.49 | 4.71 | 3.17 | 0.293 | 0.740 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **4.2x** | 10.27 | 43.22 | 10.15 | 42.91 | 41.38 | 0.125 | 0.382 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **1.4x** | 132.89 | 190.87 | 132.67 | 190.46 | 188.93 | 0.212 | 0.542 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **6.6x** | 1.50 | 9.85 | 0.86 | 8.88 | 7.35 | 0.528 | 1.308 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **4.1x** | 3.06 | 12.68 | 1.95 | 10.99 | 9.45 | 0.924 | 2.243 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **1.7x** | 27.85 | 46.51 | 24.07 | 37.92 | 36.39 | 3.847 | 10.812 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **3.0x** | 130.07 | 389.97 | 115.36 | 345.93 | 344.40 | 14.626 | 44.695 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

Multi-trial total latency spread (mean $\pm$ sample SD and two-sided 95% Student-$t$ confidence interval across measured trials after warmup):

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | $0.24 \pm 0.07$ ($\pm 0.05$) | $3.75 \pm 0.22$ ($\pm 0.15$) | $0.22 \pm 0.07$ ($\pm 0.05$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | $3.04 \pm 1.79$ ($\pm 1.21$) | $22.50 \pm 8.42$ ($\pm 5.66$) | $3.17 \pm 1.95$ ($\pm 1.31$) |
| `t2_mad_g2_i8` | $2 \times 8$ | $0.21 \pm 0.02$ ($\pm 0.01$) | $3.59 \pm 0.32$ ($\pm 0.22$) | $0.25 \pm 0.04$ ($\pm 0.03$) |
| `t2_mad_g3_i9` | $3 \times 9$ | $0.56 \pm 0.17$ ($\pm 0.11$) | $4.20 \pm 0.05$ ($\pm 0.03$) | $0.44 \pm 0.06$ ($\pm 0.04$) |
| `t2_mad_g4_i12` | $4 \times 12$ | $0.84 \pm 0.23$ ($\pm 0.15$) | $5.30 \pm 0.09$ ($\pm 0.06$) | $0.80 \pm 0.19$ ($\pm 0.13$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | $9.46 \pm 5.12$ ($\pm 3.44$) | $31.51 \pm 18.28$ ($\pm 12.28$) | $9.36 \pm 5.27$ ($\pm 3.54$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | $433.62 \pm 895.82$ ($\pm 601.78$) | $166.14 \pm 82.17$ ($\pm 55.20$) | $435.23 \pm 900.05$ ($\pm 604.62$) |
| `t3_mad_g5_i20` | $5 \times 20$ | $2.01 \pm 1.13$ ($\pm 1.40$) | $9.31 \pm 1.08$ ($\pm 1.34$) | $1.97 \pm 1.12$ ($\pm 1.39$) |
| `t3_mad_g6_i30` | $6 \times 30$ | $2.98 \pm 0.64$ ($\pm 0.80$) | $13.63 \pm 2.27$ ($\pm 2.82$) | $2.94 \pm 0.46$ ($\pm 0.57$) |
| `t3_mad_g10_i100` | $10 \times 100$ | $65.69 \pm 70.42$ ($\pm 87.43$) | $47.36 \pm 3.41$ ($\pm 4.24$) | $67.01 \pm 73.45$ ($\pm 91.19$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | $156.35 \pm 84.22$ ($\pm 104.56$) | $463.02 \pm 195.32$ ($\pm 242.49$) | $146.39 \pm 77.24$ ($\pm 95.88$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### 3. Full 31-Family Benchmark Corpus (`JSON -> Solution`)

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | `Py / goblin (Go)` | GoTotal (ms) | PyTotal (ms) | GoSolve (ms) | PySolve (ms) | PySolve Net (ms) | GoPrep (ms) | PyPrep (ms) | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | **112.4x** | 0.01 | 1.34 | 0.00 | 1.30 | 0.00 | 0.010 | 0.056 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **116.9x** | 0.01 | 1.34 | 0.00 | 1.26 | 0.00 | 0.010 | 0.056 | `OK` |
| `t1_std_mip` | 3 | 3 | **130.9x** | 0.02 | 2.01 | 0.00 | 1.96 | 0.43 | 0.011 | 0.059 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **101.4x** | 0.02 | 1.91 | 0.01 | 1.87 | 0.33 | 0.012 | 0.057 | `OK` |
| `t1_whiskas` | 6 | 5 | **52.9x** | 0.03 | 1.39 | 0.00 | 1.33 | 0.00 | 0.022 | 0.082 | `OK` |
| `t1_furniture` | 2 | 3 | **111.9x** | 0.02 | 2.23 | 0.01 | 2.18 | 0.65 | 0.008 | 0.057 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **48.0x** | 0.07 | 3.21 | 0.05 | 3.14 | 1.61 | 0.022 | 0.092 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **287.0x** | 0.01 | 1.45 | 0.00 | 1.42 | 0.00 | 0.005 | 0.046 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **15.9x** | 0.24 | 3.82 | 0.09 | 3.63 | 2.10 | 0.133 | 0.246 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **8.5x** | 2.92 | 24.75 | 2.81 | 24.53 | 22.99 | 0.111 | 0.281 | `OK` |
| `t2_mkp_30` | 30 | 6 | **8.5x** | 3.26 | 27.67 | 3.20 | 27.50 | 25.97 | 0.060 | 0.208 | `OK` |
| `t2_mkp_50` | 50 | 8 | **2.5x** | 4.63 | 11.47 | 4.50 | 11.23 | 9.69 | 0.110 | 0.324 | `OK` |
| `t2_cflp_36` | 36 | 11 | **13.1x** | 0.27 | 3.56 | 0.21 | 3.39 | 1.86 | 0.066 | 0.191 | `OK` |
| `t2_cflp_49` | 49 | 13 | **11.0x** | 0.38 | 4.15 | 0.30 | 3.96 | 2.42 | 0.078 | 0.259 | `OK` |
| `t2_gap_45` | 45 | 13 | **8.8x** | 0.97 | 8.48 | 0.85 | 8.28 | 6.75 | 0.072 | 0.241 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **8.4x** | 4.72 | 39.67 | 4.63 | 39.44 | 37.91 | 0.097 | 0.290 | `OK` |
| `t2_binpack_44` | 44 | 14 | **12.2x** | 0.55 | 6.73 | 0.46 | 6.57 | 5.04 | 0.086 | 0.212 | `OK` |
| `t2_setcover_50` | 50 | 20 | **21.7x** | 0.25 | 5.37 | 0.10 | 5.11 | 3.57 | 0.144 | 0.353 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **16.4x** | 0.21 | 3.43 | 0.09 | 3.18 | 1.65 | 0.117 | 0.329 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **8.6x** | 0.49 | 4.21 | 0.28 | 3.83 | 2.30 | 0.213 | 0.486 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **6.5x** | 0.81 | 5.28 | 0.49 | 4.71 | 3.17 | 0.293 | 0.740 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **4.2x** | 10.27 | 43.22 | 10.15 | 42.91 | 41.38 | 0.125 | 0.382 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **1.4x** | 132.89 | 190.87 | 132.67 | 190.46 | 188.93 | 0.212 | 0.542 | `OK` |
| `t3_mkp_200` | 200 | 8 | **1.3x** | 31.79 | 41.23 | 31.52 | 40.62 | 39.09 | 0.294 | 0.782 | `OK` |
| `t3_cflp_300` | 300 | 34 | **16.0x** | 13.20 | 211.02 | 12.82 | 210.13 | 208.60 | 0.415 | 1.147 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **4.8x** | 26.89 | 128.49 | 26.13 | 126.64 | 125.11 | 0.770 | 2.268 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **8.6x** | 7.64 | 65.45 | 6.44 | 62.22 | 60.69 | 1.257 | 3.667 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **6.6x** | 1.50 | 9.85 | 0.86 | 8.88 | 7.35 | 0.528 | 1.308 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **4.1x** | 3.06 | 12.68 | 1.95 | 10.99 | 9.45 | 0.924 | 2.243 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **1.7x** | 27.85 | 46.51 | 24.07 | 37.92 | 36.39 | 3.847 | 10.812 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **3.0x** | 130.07 | 389.97 | 115.36 | 345.93 | 344.40 | 14.626 | 44.695 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

### 4. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family Linear Sum Assignment corpus ([`bench/assign_corpus.go`](bench/assign_corpus.go), 200 randomized instances across square, rectangular wide/tall, degenerate integer-tie, Euclidean geometric, rank-1 outer-product, sparse `+Inf`, and maximization families) compares [`assign.LinearSumFlat`](assign/assign.go) against SciPy's C++ `rectangular_lsap` extension (`scipy.optimize.linear_sum_assignment`) in warm state on bit-identical `float64` cost matrices:

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families | `SciPy Solve / goblin` GeoMean | `SciPy Total / goblin` GeoMean | `goblin` Solve (ms) | SciPy Solve (ms) | `goblin` Total (ms) | SciPy Total (ms) | `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 | **2.26x** | 5.68x | 0.0021 | 0.0047 | 0.0027 | 0.0156 | 1.92x [1.63x-4.46x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 | **2.79x** | 2.63x | 0.0656 | 0.183 | 0.0988 | 0.260 | 2.33x [1.46x-12.17x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 | **2.69x** | 2.90x | 2.065 | 5.553 | 3.084 | 8.946 | 2.65x [1.63x-3.91x] |
| Square families ($m = n$) | 16 | **2.83x** | 3.57x | 0.107 | 0.302 | 0.141 | 0.504 | 2.86x [1.46x-12.17x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 | **2.08x** | 2.82x | 0.0321 | 0.0667 | 0.0585 | 0.165 | 2.12x [1.63x-2.65x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 | **2.33x** | 2.82x | 0.0847 | 0.198 | 0.113 | 0.318 | 1.93x [1.46x-12.17x] |
| **All 22 LSAP corpus families** | 22 | **2.60x** | 3.35x | 0.0768 | 0.200 | 0.111 | 0.372 | 2.29x [1.46x-12.17x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | `SciPy Solve / goblin` | GoSolve (ms) | SciPySolve (ms) | GoTotal (ms) | SciPyTotal (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Optimal Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **4.46x** | 0.0004 | 0.0019 | 0.0005 | 0.0122 | $0.0004 \pm 0.0001$ | $0.0020 \pm 0.0003$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **2.84x** | 0.0011 | 0.0032 | 0.0014 | 0.0106 | $0.0012 \pm 0.0004$ | $0.0032 \pm 0.0004$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.98x** | 0.0035 | 0.0070 | 0.0044 | 0.0157 | $0.0035 \pm 0.0010$ | $0.0071 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.86x** | 0.0025 | 0.0047 | 0.0033 | 0.0143 | $0.0024 \pm 0.0006$ | $0.0047 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.63x** | 0.0031 | 0.0050 | 0.0039 | 0.0152 | $0.0030 \pm 0.0008$ | $0.0051 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.75x** | 0.0065 | 0.0113 | 0.0105 | 0.0321 | $0.0062 \pm 0.0016$ | $0.0112 \pm 0.0018$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.33x** | 0.0289 | 0.0673 | 0.0356 | 0.0915 | $0.0269 \pm 0.0059$ | $0.0664 \pm 0.0100$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **2.92x** | 0.0574 | 0.167 | 0.0743 | 0.218 | $0.0577 \pm 0.0072$ | $0.168 \pm 0.0258$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **1.94x** | 0.0464 | 0.0902 | 0.0688 | 0.135 | $0.0444 \pm 0.0042$ | $0.0929 \pm 0.0137$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.46x** | 0.144 | 0.210 | 0.152 | 0.249 | $0.149 \pm 0.0420$ | $0.227 \pm 0.0413$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **12.17x** | 0.0531 | 0.647 | 0.0699 | 0.722 | $0.0515 \pm 0.0096$ | $0.654 \pm 0.0259$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **2.24x** | 0.0296 | 0.0664 | 0.0729 | 0.135 | $0.0292 \pm 0.0029$ | $0.0685 \pm 0.0072$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.02x** | 0.0381 | 0.0769 | 0.0833 | 0.148 | $0.0386 \pm 0.0037$ | $0.0791 \pm 0.0090$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **2.87x** | 0.0952 | 0.273 | 0.140 | 0.355 | $0.0971 \pm 0.0143$ | $0.279 \pm 0.0320$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.38x** | 0.357 | 1.208 | 0.543 | 1.565 | $0.370 \pm 0.0462$ | $1.216 \pm 0.127$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.62x** | 1.397 | 5.065 | 2.061 | 6.730 | $1.439 \pm 0.152$ | $5.136 \pm 0.361$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.93x** | 0.767 | 1.481 | 1.317 | 2.800 | $0.779 \pm 0.0268$ | $1.474 \pm 0.0683$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.63x** | 5.011 | 8.160 | 5.216 | 9.560 | $5.905 \pm 1.930$ | $8.896 \pm 2.588$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.65x** | 0.298 | 0.787 | 0.655 | 2.142 | $0.294 \pm 0.0170$ | $0.800 \pm 0.0535$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.23x** | 0.426 | 0.949 | 0.771 | 2.179 | $0.412 \pm 0.0290$ | $0.948 \pm 0.0520$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **3.91x** | 6.942 | 27.110 | 8.780 | 33.193 | $6.765 \pm 0.572$ | $27.297 \pm 2.163$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **3.88x** | 33.849 | 131.378 | 42.247 | 164.230 | $34.224 \pm 1.613$ | $131.911 \pm 8.430$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

---

## Pure-Go vs. C++ CBC Performance Gaps and MAD Scaling Analysis

While pure-Go `goblin` (`BackendGo`) achieves lower latency than both Python `PuLP + CBC` and in-process `goblin (CBC)` (`BackendCBC`) on small and medium models, direct comparison against `BackendCBC` and `COIN-OR CLP` exposes four structural performance gaps on large or symmetric instances.

### 1. MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families (`N = G(I + T + 3)` variables, `M = G(2T + 3) + I` constraints) separate solver performance into three distinct scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Total (`Go / CBC`) | Go Total (ms) | CBC Total (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) | Py Net Solve (ms) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.12x** | 0.24 | 1.93 | 0.60x | 0.09 (1 / 28) | 1.83 (0 / 0) | 0.067 (28) | 0.112 (19) | 2.10 |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.26x** | 2.92 | 11.07 | 0.48x | 2.81 (11 / 247) | 10.96 (6 / 83) | 0.058 (30) | 0.121 (18) | 22.99 |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.12x** | 0.21 | 1.81 | 0.64x | 0.09 (1 / 33) | 1.67 (0 / 0) | 0.072 (33) | 0.112 (21) | 1.65 |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.19x** | 0.49 | 2.54 | 0.89x | 0.28 (1 / 60) | 2.34 (0 / 0) | 0.176 (60) | 0.199 (40) | 2.30 |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.23x** | 0.81 | 3.49 | 0.71x | 0.49 (1 / 68) | 3.21 (0 / 0) | 0.235 (68) | 0.332 (63) | 3.17 |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.59x** | 10.27 | 17.40 | 0.79x | 10.15 (77 / 959) | 17.26 (12 / 216) | 0.097 (38) | 0.122 (24) | 41.38 |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **1.45x** | 132.89 | 91.86 | 0.82x | 132.67 (1161 / 14152) | 91.63 (198 / 1300) | 0.199 (66) | 0.242 (54) | 188.93 |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.18x** | 1.50 | 8.28 | 1.32x | 0.86 (1 / 138) | 7.78 (0 / 0) | 0.715 (138) | 0.542 (94) | 7.35 |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.39x** | 3.06 | 7.83 | 1.34x | 1.95 (1 / 246) | 7.01 (0 / 0) | 1.180 (246) | 0.883 (149) | 9.45 |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **0.74x** | 27.85 | 37.76 | 3.66x | 24.07 (1 / 872) | 33.95 (0 / 0) | 10.680 (872) | 2.915 (319) | 36.39 |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **0.46x** | 130.07 | 282.42 | 3.03x | 115.36 (1 / 2028) | 266.68 (0 / 0) | 101.409 (2028) | 33.517 (3128) | 344.40 |
<!-- END GENERATED:MAD_CBC_GAP_TABLE -->

1. **Small-to-Medium Planted-Gap MAD (`t1_mad_g2_i4` through `t3_mad_g5_i20`, $26 \le N \le 145$)**: Every structured `t*_mad_g*` instance has a zero root LP gap by design (`RootLPBound == OptimalObj`). Pure-Go closes these models at node 1 through root heuristics (`tryPropagatedRounding`, `tryRootSubMIP`, and `tryFractionalDiving` in [`heuristics.go`](heuristics.go)) without C++ `CbcModel` and `CglPreProcess` setup costs.
2. **Large Planted-Gap MAD Root LP Bottleneck (`t3_mad_g6_i30`, `t3_mad_g10_i100`, `t3_mad_g4_i1000`)**: Although pure-Go closes the integer gap at node 1 and finishes faster end-to-end than `BackendCBC` across all three families (`0.39x` to `0.74x` total latency, avoiding C++ `CglPreProcess` and `CbcModel` setup), virtually all pure-Go solve time is consumed by the Root LP ([`simplex.go`](simplex.go) and [`lu.go`](lu.go)), where `COIN-OR CLP` remains faster (`1.34x` to `3.66x` on `Root LP`):
   - **Degenerate Simplex Iteration Growth (`t3_mad_g10_i100`)**: Partitioning 100 items across $G = 10$ identical groups creates severe primal and dual degeneracy. `COIN-OR CLP` mitigates this via LP presolve (`ClpPresolve`), geometric-mean matrix scaling, anti-degeneracy cost shifts, and crash basis assembly (`Idiot` / triangular crash). In contrast, [`LPSolver`](simplex.go) starts from an unscaled all-slack basis without LP presolve or cost shifts, and its Phase II primal fallback uses full Dantzig pricing rather than primal steepest-edge or Devex pricing.
   - **Per-Pivot PFI Eta Update Cost (`t3_mad_g4_i1000`)**: On 1,000-row bases, pure-Go costs more per pivot in the Root LP because the basis factorizer ([`lu.go`](lu.go)) appends a Product-Form of the Inverse (PFI) eta file (`PushEtaSparse`) up to `maxEtaUpdates = 56` before rebuilding factors from scratch, whereas `CLP` updates the $U$ factor in place via Forrest-Tomlin updates.
3. **Multi-Group Symmetry and Root Cut/Heuristic Overhead on Random-Weight MAD (`t2_mad_rand_g3_i9`)**: Assigning dense random weights (`BuildMADRandom`) removes the zero-gap structure and forces Branch-and-Cut to prove the optimal bound across $G!$ group-permutation symmetries:
   - For $G = 2$ (`t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`), $2! = 2$ symmetry is small and pure-Go remains faster than `BackendCBC` despite exploring more nodes.
   - For $G = 3$ (`t2_mad_rand_g3_i9`, $3! = 6$ symmetry), pure-Go explores an order of magnitude more Branch-and-Bound nodes and simplex pivots than `BackendCBC`. Furthermore, root cuts (`CP6`) remain permanently in `ExtraRows` without cut aging or purging, and root heuristics (`CP7`) execute sub-MIP and diving LPs that do not break the $G!$ symmetry, making `CP7 Full` slower than `CP5 +Presolve` (`Presolve=on, Cuts=off, Heuristics=off`).

### 2. Non-MAD Corpus Comparison and Checkpoint Trade-Offs

Across the 20 non-MAD families, pure-Go achieves lower median latency than `BackendCBC` on all five largest models (`0.09x` to `0.63x`), while exhibiting multi-trial variance on `t2_binpack_44` and checkpoint trade-offs where `CP5 +Presolve` (`t3_cflp_300`) or `CP6 +Cuts` (`t2_mkp_50`) finishes slightly faster than `CP7 Full`:

<!-- BEGIN GENERATED:NON_MAD_CBC_GAP_TABLE -->
| Problem ID | $N \times M$ | Total Median (`Go / CBC`) | Go Total Mean $\pm$ SD (ms) | CBC Total Mean $\pm$ SD (ms) | Go CP7 Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go CP6 +Cuts (ms / nodes / cuts) | Go CP5 +Presolve (ms / nodes) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t3_mkp_200` | $200 \times 8$ | **0.63x** | $29.53 \pm 15.48$ | $40.80 \pm 19.05$ | 31.52 (590 / 5535) | 50.19 (154 / 1145) | 46.66 (739 / 21) | 85.96 (5000, `NodeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **0.54x** | $7.21 \pm 8.57$ | $12.07 \pm 8.81$ | 4.50 (1 / 24) | 8.46 (0 / 8) | 3.76 (2 / 5) | 3.87 (132) |
| `t2_binpack_44` | $44 \times 14$ | **0.09x** | $15.65 \pm 47.40$ | $6.47 \pm 2.14$ | 0.46 (1 / 44) | 5.86 (0 / 8) | 2.67 (20 / 12) | 16.15 (1873) |
| `t3_cflp_300` | $300 \times 34$ | **0.27x** | $16.59 \pm 9.62$ | $53.10 \pm 11.21$ | 12.82 (47 / 2103) | 48.01 (18 / 483) | 19.54 (47 / 37) | 5.93 (134) |
| `t3_sched_1000` | $1000 \times 100$ | **0.12x** | $7.81 \pm 1.25$ | $64.60 \pm 4.17$ | 6.44 (35 / 445) | 63.04 (0 / 76) | 11.10 (96 / 50) | 216.23 (5000, `NodeLimit`) |
<!-- END GENERATED:NON_MAD_CBC_GAP_TABLE -->

### 3. Summary of Architectural Bottlenecks

1. **PFI Eta Updates and Unscaled LP Cold Starts ([`lu.go`](lu.go), [`simplex.go`](simplex.go))**: Scanning up to `maxEtaUpdates = 56` PFI eta vectors between full Markowitz factor rebuilds increases per-pivot cost on large bases (`t3_mad_g4_i1000`), while omitting LP presolve, matrix scaling, and anti-degeneracy cost shifts inflates pivot counts on degenerate assignment LPs (`t3_mad_g10_i100`).
2. **Root-Only Global Cuts Without Cut Aging ([`cuts.go`](cuts.go))**: Cutting planes are separated only at the root node using global bounds and retained for the entire tree. On multi-thousand-node trees (`t2_mad_rand_g3_i9`), unpurged cuts enlarge every node LP, while dense multi-constraint knapsacks (`t3_mkp_200`) require multi-row aggregation and in-tree cuts to match CBC's bound tightening.
3. **No Orbital Symmetry Breaking ([`bab.go`](bab.go))**: Permutation-symmetric models (`t2_mad_rand_g3_i9`, and outlier seed 9 of `t2_binpack_44`) force pure-Go to enumerate symmetric subtrees that CBC prunes.
4. **Root Diving and Sub-MIP Passes on Symmetric Equality Models ([`heuristics.go`](heuristics.go))**: Although [`tryRootSubMIP`](heuristics.go) is skipped on inequality-only models without continuous-linked integer columns, equality-constrained models (`t2_mad_rand_g3_i9`) and facility-location models (`t3_cflp_300`) still pay root sub-MIP and diving LP costs when `CP5 +Presolve` alone suffices.

---

## Verification, Benchmarking, and Documentation Sync Commands

Run all commands from the `goblin/` directory:

```bash
# 0. Build the vendored COIN-OR CBC/CLP C++ stack into third_party/Cbc/dist (once per checkout)
./scripts/build_cbc.sh

# 1. Unit, regression, 293-trial CBC parity, and documentation gates (both builds)
go test ./...
GOEXPERIMENT=simd go test ./...

# 2. Regenerate the 31-family PuLP JSON and multi-trial JSONL corpus in testdata/corpus/
go run ./cmd/goblin-bench -generate-corpus

# 3. Commit first: goblin-bench records `git rev-parse HEAD` in every telemetry entry.
git status --short   # must be empty for clean provenance

# 4. Warm-state Python PuLP + COIN-OR CBC baseline, pinned to one performance core (also measures the CBC spawn floor)
CBC_BIN=/path/to/cbc-2.10.3 taskset -c 2 python3 bench/python_baseline.py --corpus-dir testdata/corpus --out-json bench/results/python_baseline.json --workers 1

# 5. Warm-state goblin benchmarks across Pure-Go (scalar and SIMD) and C++ CBC backends, pinned to the same performance core (CPU 2 on the i9-13900K)
go build -o /tmp/goblin-bench-scalar ./cmd/goblin-bench && taskset -c 2 /tmp/goblin-bench-scalar -backend go -no-simd -workers 1 -out bench/results/goblin_scalar.json
GOEXPERIMENT=simd go build -o /tmp/goblin-bench-simd ./cmd/goblin-bench && taskset -c 2 /tmp/goblin-bench-simd -backend go -workers 1 -out bench/results/goblin_simd.json
taskset -c 2 /tmp/goblin-bench-scalar -backend cbc -workers 1 -out bench/results/goblin_cbc.json

# 6. Warm-state SciPy and goblin/assign LSAP benchmarks, pinned to the same performance core
taskset -c 2 python3 bench/scipy_assign_baseline.py --corpus-dir testdata/assign_corpus --out-json bench/results/scipy_assign_baseline.json
taskset -c 2 /tmp/goblin-bench-scalar -assign -no-simd -out bench/results/goblin_assign_scalar.json
taskset -c 2 /tmp/goblin-bench-simd -assign -out bench/results/goblin_assign_simd.json

# 7. Render README.md, docs/benchmarks.md, and bench/results/final_report.md from the telemetry
go run ./cmd/goblin-bench -sync-docs

# 8. Verify documentation, parity, and provenance (exits non-zero on any failure)
go run ./cmd/goblin-bench -check-docs

# 9. Differential tests against CBC and SciPy on freshly generated random instances
python3 bench/difftest_cbc.py --instances 400 --seed 1
python3 bench/difftest_assign.py --instances 500 --seed 1

# 10. Hot-path compiler, disassembly, and Amdahl's Law profile audit (scalar and SIMD)
go run ./cmd/goblin-optcheck -benchtime=200ms
go run ./cmd/goblin-optcheck -simd -benchtime=200ms
```
