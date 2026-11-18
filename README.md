# `goblin`: Pure-Go LP, MILP, and Linear Sum Assignment Solver with Optional C++ CBC Backend

The `goblin` module provides an in-memory Linear Programming (LP) and Mixed-Integer Linear Programming (MILP) modeling engine and Branch-and-Cut solver alongside the [`goblin/assign`](assign/assign.go) Linear Sum Assignment Problem (`LSAP`) solver, written in Go 1.27. It compiles algebraic models directly into Dual-Index Compressed Sparse Row and Column (`CSR`/`CSC`) matrices (`CompiledProblem`), eliminating intermediate text-file I/O and subprocess spawn costs. By default it builds as pure Go with no `cgo` dependency (`BackendGo`); callers who build with `-tags cbc` (`cgo` enabled) can also select the in-process C++ COIN-OR CBC/CLP backend (`WithCBC()` / `WithBackend(BackendCBC)`) or configure solver flags (`WithoutSIMD()`, `WithTimeLimit(...)`, etc.) through functional arguments on `Solve` and `SolveCompiled`.

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
3. **Optional SIMD Kernels**: With `GOEXPERIMENT=simd` on `amd64`, [`internal/simd/kernels_amd64.go`](internal/simd/kernels_amd64.go) dispatches 256-bit AVX2 `archsimd.Float64x4` instructions (`VFMADD213PD` in `Dot64`; `VMULPD` + `VADDPD` and `VSUBPD` + `VMINPD` in `Axpy64`, `MinReducedSlice64`, `InitReducedRow64`, and `Min2ReducedRow64`; 8-lane `Abs().LessEqual` in `AllAbsLE64`) with `VZEROUPPER` cleanup for slices of length 32 or more, and can be toggled at runtime via `WithoutSIMD()` or `WithSIMD(false)`. See the paired comparison in [`docs/benchmarks.md`](docs/benchmarks.md#5-default-build-vs-goexperimentsimd).
4. **Verified Parity and Resource-Limit Contracts**: The suite in [`solver_test.go`](solver_test.go) re-solves all 293 randomized trial instances of the 31 benchmark families (`testdata/corpus/trials/*.jsonl`), compares status and objective with the frozen `PuLP + COIN-OR CBC 2.10.3` and `BackendCBC` results, and verifies that every `goblin` solution satisfies all bounds and rows. [`regression_test.go`](regression_test.go) adds big-M fixed-charge models with enumerated ground truth and replays fuzz instances on which `goblin` or CBC failed. `TimeLimit`, `MaxNodes`, and `MaxIter` return `StatusTimeLimit`, `StatusNodeLimit`, or `StatusIterationLimit` with a valid global bound once the root LP is solved; a node that cannot be evaluated is never discarded silently.

---

## Documentation Index

Full technical guides live in the [`docs/`](docs/index.md) directory:

- **[Architecture and Design Overview](docs/index.md)**: Module boundaries, data flow from algebraic expressions to `CSR`/`CSC` matrices, and memory pooling invariants.
- **[Modeling API and Serialization Formats](docs/modeling-and-formats.md)**: Constructing variables, linear expressions, and constraints, configuring solver flags and backends, validating and compiling models, and exchanging `PuLP` JSON and MPS payloads.
- **[Solver Internals and Mathematical Specification](docs/solver-internals.md)**: Three-stage singleton and sparse Markowitz LU with `FTRANSparseHint`/`BTRANSparseHint` and PFI eta updates, Revised Dual Simplex with Bound-Flipping Ratio Test (`selectDualBFRT`) and sparse `TabRowIdx` tableau rows, Two-Phase Primal Simplex, MIP presolve with algebraic orbit symmetry breaking, cutting planes (`GMI`, Sequentially Lifted Knapsack Covers, Flow/VUB, one-sided `c-MIR`) with slack-cut purging, primal heuristics, and Branch-and-Cut search.
- **[Benchmark Methodology and Corpus Results](docs/benchmarks.md)**: Hardware setup, comparative telemetry against Python `PuLP + CBC` and in-process `BackendCBC`, CBC spawn-floor separation, paired scalar-vs-SIMD statistics, structural analysis of the Mean Absolute Deviation (`MAD`) `G x I` allocation family vs. random-weight negative controls (`BuildMADRandom`), and full checkpoint telemetry.

---

## Benchmarks

All benchmark tables in this `README.md`, [`docs/benchmarks.md`](docs/benchmarks.md), and [`bench/results/final_report.md`](bench/results/final_report.md) are rendered from the JSON telemetry in `bench/results/` (`python_baseline.json`, `goblin_cbc.json`, `goblin_scalar.json`, `goblin_simd.json`). [`doc_sync_test.go`](doc_sync_test.go) fails `go test ./...` if a table drifts from the telemetry, if any `goblin` build disagrees with CBC on a status or objective, or if the telemetry lacks git provenance. Timings use the default (scalar) build.

<!-- BEGIN GENERATED:PROVENANCE -->
- **goblin telemetry source**: fingerprint `6c061f8c469be0a6` (working tree on top of commit `41d23542f0d77ed11e80d949681eda9ee33ffd8d`), `go1.27.3-X:nodwarf5`; the documentation gate rejects telemetry whose fingerprint differs from the checked-in sources.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, CBC `2.10.3` (`cbc`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 1.53 ms median (`prob.solve()` on a 1-variable MILP); the "Net of Spawn" columns subtract it.
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `master` (commit `c1c171ec`, vendored in `third_party/Cbc`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). The C++ backend maps default settings onto CBC with two deliberate differences: `MaxCutPasses = 12` becomes `passC = 30` and `MaxNodes = 50000` becomes unlimited. It is therefore not a like-for-like setup and not the CBC 2.10.3 binary used by the Python baseline.
<!-- END GENERATED:PROVENANCE -->

Read the ratios with three caveats. Python's solve time includes a fixed CBC process-spawn cost, reported separately as "Net of Spawn". The `goblin (CBC)` column runs a different CBC build with different settings. The planted-gap MAD family is solved at the root by construction, so the bold row excludes it.

### 1. Aggregate Workload Summary (Geometric Mean and Median `[Min-Max]` Total Latency)

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families | `Py / goblin (Go)` GeoMean | `Py net / goblin (Go)` | `goblin (Go) / CBC` | `goblin (Go)` (ms) | `goblin (CBC)` (ms) | Py Total (ms) | Py Net (ms) | `Py / goblin (Go)` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical (`N <= 26`) | 10 | **95.0x** | 3.1x | 0.04x | 0.03 | 0.60 | 2.50 | 0.08 | 110.5x [27.3x-289.4x] |
| Tier 2: Target p90 (`30 <= N <= 84`) | 13 | **10.4x** | 8.0x | 0.15x | 1.06 | 7.25 | 11.03 | 8.43 | 10.4x [2.1x-32.1x] |
| Tier 3: Large Tail (`145 <= N <= 4036`) | 8 | **5.7x** | 5.4x | 0.22x | 10.59 | 47.12 | 59.90 | 56.86 | 7.8x [1.0x-14.7x] |
| Tier 3 non-MAD MILPs | 4 | **3.3x** | 3.2x | 0.42x | 28.08 | 67.53 | 92.48 | 90.63 | 4.5x [1.0x-12.3x] |
| Planted-gap MAD family (zero integrality gap by construction) | 8 | **13.5x** | 10.2x | 0.10x | 0.94 | 9.58 | 12.66 | 9.57 | 13.8x [6.8x-27.3x] |
| Random-weight MAD (negative controls) | 3 | **12.4x** | 11.9x | 0.16x | 4.76 | 29.61 | 58.88 | 56.80 | 9.9x [5.4x-35.0x] |
| **All families except planted-gap MAD** | 23 | **20.1x** | 4.2x | 0.12x | 0.49 | 4.27 | 9.93 | 2.07 | 13.1x [1.0x-289.4x] |
| All corpus families | 31 | **18.1x** | 5.3x | 0.11x | 0.58 | 5.26 | 10.57 | 3.08 | 13.2x [1.0x-289.4x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2. MAD Group-Item Allocation Family (`G` Groups $\times$ `I` Items, $T = 6$ Periods)

The `MAD` group-item allocation family ([`BuildMADFromWeights`](bench/corpus.go)) assigns $I$ items across $G$ groups over $T = 6$ periods to minimize total L1 distance from each group's central level $m_g$ plus the roundness penalty $r_g = |m_g - 100 n_g|$ relative to an integer multiple $n_g \in \mathbb{Z}_{\ge 1}$. In the planted-gap generator, items in $S = \{0, \dots, I-G-1\}$ carry orthogonal curve signatures cancelled by the $G$ designated items $D = \{I-G, \dots, I-1\}$ with roundness-aligned period sums, yielding a zero root LP gap that root heuristics close at node 1; unstructured random-weight negative controls (`BuildMADRandom`: `t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`) are included directly in the corpus and detailed in [`docs/benchmarks.md`](docs/benchmarks.md).

<!-- BEGIN GENERATED:MAD_PIPELINE_TABLE -->
| Problem ID | $G \times I$ | Vars | Rows | `Py / goblin (Go)` | GoTotal (ms) | PyTotal (ms) | GoSolve (ms) | PySolve (ms) | PySolve Net (ms) | GoPrep (ms) | PyPrep (ms) | Optimal Obj | Parity |
| :--- | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **27.3x** | 0.14 | 3.82 | 0.08 | 3.63 | 2.10 | 0.056 | 0.246 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **35.0x** | 0.71 | 24.75 | 0.68 | 24.53 | 22.99 | 0.032 | 0.281 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **14.4x** | 0.24 | 3.43 | 0.15 | 3.18 | 1.65 | 0.058 | 0.329 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **23.6x** | 0.18 | 4.21 | 0.13 | 3.83 | 2.30 | 0.058 | 0.486 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **13.2x** | 0.40 | 5.28 | 0.27 | 4.71 | 3.17 | 0.113 | 0.740 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **9.9x** | 4.34 | 43.22 | 4.30 | 42.91 | 41.38 | 0.045 | 0.382 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **5.4x** | 35.04 | 190.87 | 34.85 | 190.46 | 188.93 | 0.074 | 0.542 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **14.7x** | 0.67 | 9.85 | 0.46 | 8.88 | 7.35 | 0.201 | 1.308 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **11.3x** | 1.13 | 12.68 | 0.86 | 10.99 | 9.45 | 0.281 | 2.243 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **6.8x** | 6.82 | 46.51 | 5.19 | 37.92 | 36.39 | 1.473 | 10.812 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **7.9x** | 49.55 | 389.97 | 43.06 | 345.93 | 344.40 | 5.456 | 44.695 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

Multi-trial total latency spread (mean $\pm$ sample SD and two-sided 95% Student-$t$ confidence interval across measured trials after warmup):

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | $0.15 \pm 0.02$ ($\pm 0.02$) | $3.75 \pm 0.22$ ($\pm 0.15$) | $0.14 \pm 0.02$ ($\pm 0.01$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | $0.98 \pm 0.80$ ($\pm 0.54$) | $22.50 \pm 8.42$ ($\pm 5.66$) | $0.96 \pm 0.81$ ($\pm 0.54$) |
| `t2_mad_g2_i8` | $2 \times 8$ | $0.27 \pm 0.17$ ($\pm 0.11$) | $3.59 \pm 0.32$ ($\pm 0.22$) | $0.19 \pm 0.08$ ($\pm 0.05$) |
| `t2_mad_g3_i9` | $3 \times 9$ | $0.21 \pm 0.05$ ($\pm 0.03$) | $4.20 \pm 0.05$ ($\pm 0.03$) | $0.24 \pm 0.13$ ($\pm 0.08$) |
| `t2_mad_g4_i12` | $4 \times 12$ | $0.42 \pm 0.09$ ($\pm 0.06$) | $5.30 \pm 0.09$ ($\pm 0.06$) | $0.41 \pm 0.09$ ($\pm 0.06$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | $4.31 \pm 2.40$ ($\pm 1.61$) | $31.51 \pm 18.28$ ($\pm 12.28$) | $4.38 \pm 2.47$ ($\pm 1.66$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | $41.03 \pm 36.00$ ($\pm 24.19$) | $166.14 \pm 82.17$ ($\pm 55.20$) | $40.46 \pm 35.94$ ($\pm 24.14$) |
| `t3_mad_g5_i20` | $5 \times 20$ | $0.84 \pm 0.33$ ($\pm 0.41$) | $9.31 \pm 1.08$ ($\pm 1.34$) | $0.75 \pm 0.10$ ($\pm 0.13$) |
| `t3_mad_g6_i30` | $6 \times 30$ | $1.62 \pm 1.28$ ($\pm 1.59$) | $13.63 \pm 2.27$ ($\pm 2.82$) | $1.60 \pm 0.73$ ($\pm 0.91$) |
| `t3_mad_g10_i100` | $10 \times 100$ | $6.98 \pm 0.70$ ($\pm 0.87$) | $47.36 \pm 3.41$ ($\pm 4.24$) | $6.77 \pm 0.62$ ($\pm 0.77$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | $58.13 \pm 17.32$ ($\pm 21.50$) | $463.02 \pm 195.32$ ($\pm 242.49$) | $55.86 \pm 16.61$ ($\pm 20.63$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### 3. Full 31-Family Benchmark Corpus (`JSON -> Solution`)

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | `Py / goblin (Go)` | GoTotal (ms) | PyTotal (ms) | GoSolve (ms) | PySolve (ms) | PySolve Net (ms) | GoPrep (ms) | PyPrep (ms) | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | **130.7x** | 0.01 | 1.34 | 0.00 | 1.30 | 0.00 | 0.008 | 0.056 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **244.3x** | 0.01 | 1.34 | 0.00 | 1.26 | 0.00 | 0.004 | 0.056 | `OK` |
| `t1_std_mip` | 3 | 3 | **223.1x** | 0.01 | 2.01 | 0.00 | 1.96 | 0.43 | 0.004 | 0.059 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **134.8x** | 0.01 | 1.91 | 0.01 | 1.87 | 0.33 | 0.005 | 0.057 | `OK` |
| `t1_whiskas` | 6 | 5 | **90.4x** | 0.02 | 1.39 | 0.00 | 1.33 | 0.00 | 0.010 | 0.082 | `OK` |
| `t1_furniture` | 2 | 3 | **67.4x** | 0.03 | 2.23 | 0.02 | 2.18 | 0.65 | 0.006 | 0.057 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **36.9x** | 0.09 | 3.21 | 0.07 | 3.14 | 1.61 | 0.013 | 0.092 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **289.4x** | 0.01 | 1.45 | 0.00 | 1.42 | 0.00 | 0.004 | 0.046 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **27.3x** | 0.14 | 3.82 | 0.08 | 3.63 | 2.10 | 0.056 | 0.246 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **35.0x** | 0.71 | 24.75 | 0.68 | 24.53 | 22.99 | 0.032 | 0.281 | `OK` |
| `t2_mkp_30` | 30 | 6 | **9.2x** | 2.99 | 27.67 | 2.96 | 27.50 | 25.97 | 0.026 | 0.208 | `OK` |
| `t2_mkp_50` | 50 | 8 | **2.1x** | 5.39 | 11.47 | 5.35 | 11.23 | 9.69 | 0.042 | 0.324 | `OK` |
| `t2_cflp_36` | 36 | 11 | **13.1x** | 0.27 | 3.56 | 0.21 | 3.39 | 1.86 | 0.029 | 0.191 | `OK` |
| `t2_cflp_49` | 49 | 13 | **8.8x** | 0.47 | 4.15 | 0.44 | 3.96 | 2.42 | 0.028 | 0.259 | `OK` |
| `t2_gap_45` | 45 | 13 | **10.4x** | 0.81 | 8.48 | 0.79 | 8.28 | 6.75 | 0.028 | 0.241 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **6.9x** | 5.76 | 39.67 | 5.72 | 39.44 | 37.91 | 0.037 | 0.290 | `OK` |
| `t2_binpack_44` | 44 | 14 | **13.3x** | 0.50 | 6.73 | 0.47 | 6.57 | 5.04 | 0.034 | 0.212 | `OK` |
| `t2_setcover_50` | 50 | 20 | **32.1x** | 0.17 | 5.37 | 0.11 | 5.11 | 3.57 | 0.047 | 0.353 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **14.4x** | 0.24 | 3.43 | 0.15 | 3.18 | 1.65 | 0.058 | 0.329 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **23.6x** | 0.18 | 4.21 | 0.13 | 3.83 | 2.30 | 0.058 | 0.486 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **13.2x** | 0.40 | 5.28 | 0.27 | 4.71 | 3.17 | 0.113 | 0.740 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.9x** | 4.34 | 43.22 | 4.30 | 42.91 | 41.38 | 0.045 | 0.382 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **5.4x** | 35.04 | 190.87 | 34.85 | 190.46 | 188.93 | 0.074 | 0.542 | `OK` |
| `t3_mkp_200` | 200 | 8 | **1.0x** | 39.36 | 41.23 | 39.26 | 40.62 | 39.09 | 0.101 | 0.782 | `OK` |
| `t3_cflp_300` | 300 | 34 | **12.3x** | 17.10 | 211.02 | 16.97 | 210.13 | 208.60 | 0.144 | 1.147 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **1.2x** | 110.35 | 128.49 | 109.83 | 126.64 | 125.11 | 0.442 | 2.268 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **7.8x** | 8.37 | 65.45 | 7.98 | 62.22 | 60.69 | 0.397 | 3.667 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **14.7x** | 0.67 | 9.85 | 0.46 | 8.88 | 7.35 | 0.201 | 1.308 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **11.3x** | 1.13 | 12.68 | 0.86 | 10.99 | 9.45 | 0.281 | 2.243 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **6.8x** | 6.82 | 46.51 | 5.19 | 37.92 | 36.39 | 1.473 | 10.812 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **7.9x** | 49.55 | 389.97 | 43.06 | 345.93 | 344.40 | 5.456 | 44.695 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

### 4. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family Linear Sum Assignment corpus ([`bench/assign_corpus.go`](bench/assign_corpus.go), 200 randomized instances across square, rectangular wide/tall, degenerate integer-tie, Euclidean geometric, rank-1 outer-product, sparse `+Inf`, and maximization families) compares [`assign.LinearSumFlat`](assign/assign.go) against SciPy's C++ `rectangular_lsap` extension (`scipy.optimize.linear_sum_assignment`) in warm state on bit-identical `float64` cost matrices:

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families | `SciPy Solve / goblin` GeoMean | `SciPy Total / goblin` GeoMean | `goblin` Solve (ms) | SciPy Solve (ms) | `goblin` Total (ms) | SciPy Total (ms) | `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 | **2.16x** | 5.47x | 0.0022 | 0.0047 | 0.0028 | 0.0156 | 1.75x [1.53x-5.02x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 | **2.48x** | 2.36x | 0.0738 | 0.183 | 0.110 | 0.260 | 2.04x [1.32x-10.91x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 | **2.43x** | 2.56x | 2.290 | 5.553 | 3.490 | 8.946 | 2.43x [1.42x-3.64x] |
| Square families ($m = n$) | 16 | **2.59x** | 3.25x | 0.116 | 0.302 | 0.155 | 0.504 | 2.55x [1.32x-10.91x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 | **1.87x** | 2.57x | 0.0356 | 0.0667 | 0.0643 | 0.165 | 1.88x [1.53x-2.43x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 | **2.10x** | 2.52x | 0.0940 | 0.198 | 0.126 | 0.318 | 1.67x [1.32x-10.91x] |
| **All 22 LSAP corpus families** | 22 | **2.37x** | 3.05x | 0.0842 | 0.200 | 0.122 | 0.372 | 2.03x [1.32x-10.91x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | `SciPy Solve / goblin` | GoSolve (ms) | SciPySolve (ms) | GoTotal (ms) | SciPyTotal (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Optimal Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **5.02x** | 0.0004 | 0.0019 | 0.0005 | 0.0122 | $0.0004 \pm 0.0001$ | $0.0020 \pm 0.0003$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **2.68x** | 0.0012 | 0.0032 | 0.0014 | 0.0106 | $0.0012 \pm 0.0003$ | $0.0032 \pm 0.0004$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.84x** | 0.0038 | 0.0070 | 0.0047 | 0.0157 | $0.0037 \pm 0.0009$ | $0.0071 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.63x** | 0.0029 | 0.0047 | 0.0037 | 0.0143 | $0.0027 \pm 0.0006$ | $0.0047 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.53x** | 0.0033 | 0.0050 | 0.0042 | 0.0152 | $0.0032 \pm 0.0007$ | $0.0051 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.67x** | 0.0068 | 0.0113 | 0.0111 | 0.0321 | $0.0071 \pm 0.0023$ | $0.0112 \pm 0.0018$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.04x** | 0.0330 | 0.0673 | 0.0399 | 0.0915 | $0.0304 \pm 0.0068$ | $0.0664 \pm 0.0100$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **2.52x** | 0.0665 | 0.167 | 0.0830 | 0.218 | $0.0645 \pm 0.0063$ | $0.168 \pm 0.0258$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **1.73x** | 0.0522 | 0.0902 | 0.0771 | 0.135 | $0.0501 \pm 0.0051$ | $0.0929 \pm 0.0137$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.32x** | 0.159 | 0.210 | 0.169 | 0.249 | $0.166 \pm 0.0467$ | $0.227 \pm 0.0413$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **10.91x** | 0.0592 | 0.647 | 0.0780 | 0.722 | $0.0557 \pm 0.0091$ | $0.654 \pm 0.0259$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.95x** | 0.0340 | 0.0664 | 0.0814 | 0.135 | $0.0332 \pm 0.0033$ | $0.0685 \pm 0.0072$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **1.80x** | 0.0426 | 0.0769 | 0.0901 | 0.148 | $0.0450 \pm 0.0049$ | $0.0791 \pm 0.0090$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **2.57x** | 0.106 | 0.273 | 0.156 | 0.355 | $0.105 \pm 0.0088$ | $0.279 \pm 0.0320$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.08x** | 0.391 | 1.208 | 0.612 | 1.565 | $0.411 \pm 0.0513$ | $1.216 \pm 0.127$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.31x** | 1.532 | 5.065 | 2.270 | 6.730 | $1.592 \pm 0.172$ | $5.136 \pm 0.361$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.67x** | 0.886 | 1.481 | 1.529 | 2.800 | $0.886 \pm 0.0512$ | $1.474 \pm 0.0683$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.42x** | 5.750 | 8.160 | 6.167 | 9.560 | $6.565 \pm 2.147$ | $8.896 \pm 2.588$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.43x** | 0.324 | 0.787 | 0.721 | 2.142 | $0.317 \pm 0.0152$ | $0.800 \pm 0.0535$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.02x** | 0.471 | 0.949 | 0.867 | 2.179 | $0.472 \pm 0.0370$ | $0.948 \pm 0.0520$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **3.52x** | 7.700 | 27.110 | 9.846 | 33.193 | $7.527 \pm 0.529$ | $27.297 \pm 2.163$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **3.64x** | 36.059 | 131.378 | 47.836 | 164.230 | $36.388 \pm 2.315$ | $131.911 \pm 8.430$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

---

## Pure-Go vs. C++ CBC Performance Gaps and MAD Scaling Analysis

While pure-Go `goblin` (`BackendGo`) achieves lower median total latency than Python `PuLP + CBC` across all 31 corpus families and lower median total latency than in-process `goblin (CBC)` (`BackendCBC`) across 30 of 31 families (including all 11 MAD families), comparing isolated unpresolved `Root LP` timings and solver-core checkpoints (`CP5 +Presolve`, `CP6 +Cuts`, `CP7 Full`) against `BackendCBC` and `COIN-OR CLP` highlights four remaining structural trade-offs on large degenerate LPs and branch-and-cut models.

### 1. MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families (`N = G(I + T + 3)` variables, `M = G(2T + 3) + I` constraints) separate solver performance into three distinct scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Total (`Go / CBC`) | Go Total (ms) | CBC Total (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) | Py Net Solve (ms) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.06x** | 0.14 | 2.54 | 0.58x | 0.08 (1 / 19) | 2.50 (0 / 0) | 0.072 (28) | 0.124 (19) | 2.10 |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.05x** | 0.71 | 13.03 | 0.63x | 0.68 (5 / 124) | 12.99 (6 / 83) | 0.083 (30) | 0.130 (18) | 22.99 |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.11x** | 0.24 | 2.15 | 0.69x | 0.15 (1 / 26) | 2.10 (0 / 0) | 0.087 (33) | 0.127 (21) | 1.65 |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.07x** | 0.18 | 2.72 | 0.96x | 0.13 (1 / 39) | 2.66 (0 / 0) | 0.224 (59) | 0.234 (40) | 2.30 |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.10x** | 0.40 | 4.09 | 0.79x | 0.27 (1 / 51) | 3.66 (0 / 0) | 0.296 (68) | 0.377 (63) | 3.17 |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.22x** | 4.34 | 19.54 | 0.86x | 4.30 (43 / 580) | 19.49 (12 / 216) | 0.118 (38) | 0.137 (24) | 41.38 |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.34x** | 35.04 | 101.98 | 0.90x | 34.85 (137 / 1910) | 101.87 (198 / 1300) | 0.240 (66) | 0.266 (54) | 188.93 |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.07x** | 0.67 | 9.97 | 1.64x | 0.46 (1 / 75) | 9.81 (0 / 0) | 0.988 (138) | 0.603 (94) | 7.35 |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.13x** | 1.13 | 8.36 | 1.49x | 0.86 (1 / 105) | 8.12 (0 / 0) | 1.479 (246) | 0.991 (149) | 9.45 |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **0.15x** | 6.82 | 44.32 | 3.06x | 5.19 (1 / 255) | 42.46 (0 / 0) | 16.510 (962) | 5.403 (319) | 36.39 |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **0.16x** | 49.55 | 316.31 | 3.28x | 43.06 (1 / 2439) | 311.87 (0 / 0) | 123.747 (2314) | 37.726 (3128) | 344.40 |
<!-- END GENERATED:MAD_CBC_GAP_TABLE -->

1. **Small-to-Medium Planted-Gap MAD (`t1_mad_g2_i4` through `t3_mad_g5_i20`, $26 \le N \le 145$)**: Every structured `t*_mad_g*` instance has a zero root LP gap by design (`RootLPBound == OptimalObj`). Pure-Go closes these models at node 1 through presolve orbit fixing and root heuristics (`tryPropagatedRounding`, `tryRootSubMIP`, and `tryFractionalDiving` in [`heuristics.go`](heuristics.go)) without C++ `CbcModel` and `CglPreProcess` setup costs.
2. **Large Planted-Gap MAD (`t3_mad_g6_i30`, `t3_mad_g10_i100`, `t3_mad_g4_i1000`)**: Pure-Go `Go Solve` finishes faster than its own unpresolved `Go Root LP` because [`Presolve`](presolve.go) detects and verifies the $S_G$ group-permutation automorphism (`applyOrbitSymmetryBreaking`), fixes the $G \times G$ designated-item bijection to $I_G$, and eliminates $G^2$ binary variables and $2G$ assignment equations before the root LP runs, while [`FTRANSparseHint`](lu.go) and [`BTRANSparseHint`](lu.go) restrict basis triangular sweeps to active index windows. On the unpresolved `Root LP` column, however, `COIN-OR CLP` remains faster for two reasons:
   - **Degenerate Simplex Iteration Growth (`t3_mad_g10_i100`)**: Partitioning 100 items across $G = 10$ identical groups creates severe primal and dual degeneracy. `COIN-OR CLP` mitigates this via LP presolve (`ClpPresolve`), geometric-mean matrix scaling, anti-degeneracy cost shifts, and crash basis assembly (`Idiot` / triangular crash). [`LPSolver`](simplex.go) starts from an unscaled all-slack basis without LP presolve or cost shifts, and its Phase II primal fallback uses full Dantzig pricing.
   - **Per-Pivot PFI Eta Update Cost (`t3_mad_g4_i1000`)**: On 1,000-row bases, pure-Go costs more per pivot on the unpresolved Root LP because the basis factorizer ([`lu.go`](lu.go)) appends a Product-Form of the Inverse (PFI) eta file (`PushEtaSparse`) up to `maxEtaUpdates = 56` before rebuilding factors from scratch, whereas `CLP` updates the $U$ factor in place via Forrest-Tomlin updates.
3. **Symmetric Random-Weight MAD (`t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`)**: Assigning dense random weights (`BuildMADRandom`) removes the zero-gap structure and forces Branch-and-Cut to prove optimality:
   - [`Presolve`](presolve.go) verifies exact $S_G$ group symmetry and fixes the $G \times G$ designated-item bijection to $I_G$, [`bab.go`](bab.go) rolls back trailing cut rounds that fail to improve the root bound and purges slack root cuts via [`PurgeSlackCuts`](simplex.go) before branching, and post-cut root heuristics are skipped when cuts add zero bound gain. Consequently, pure-Go finishes well below `BackendCBC` across all three random-weight families (`t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, and `t2_mad_rand_g3_i9`), exploring fewer nodes than `BackendCBC` on canonical `t2_mad_rand_g3_i9` with zero retained slack cuts.
   - Even so, [`docs/benchmarks.md`](docs/benchmarks.md#mad-solver-core-checkpoint-progression) shows that `CP5 +Presolve` (`Presolve=on, Cuts=off, Heuristics=off`) finishes faster than `CP6 +Cuts` and `CP7 Full` on `t2_mad_rand_g3_i9` because trial root cut separation (`CP6`) and pre-cut root diving and sub-MIP LPs (`CP7`) pay root LP work without closing the random-weight integrality gap.

### 2. Non-MAD Corpus Comparison and Checkpoint Trade-Offs

Across the 20 non-MAD families, pure-Go achieves lower median total latency than `BackendCBC` on all five diagnostic models below, while exhibiting multi-trial variance on outlier seeds of `t2_binpack_44` and checkpoint trade-offs where `CP5 +Presolve` (`t3_cflp_300`, `t2_mkp_50`) finishes slightly faster than `CP7 Full`:

<!-- BEGIN GENERATED:NON_MAD_CBC_GAP_TABLE -->
| Problem ID | $N \times M$ | Total Median (`Go / CBC`) | Go Total Mean $\pm$ SD (ms) | CBC Total Mean $\pm$ SD (ms) | Go CP7 Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go CP6 +Cuts (ms / nodes / cuts) | Go CP5 +Presolve (ms / nodes) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t3_mkp_200` | $200 \times 8$ | **0.68x** | $34.48 \pm 17.74$ | $47.11 \pm 22.30$ | 39.26 (594 / 5502) | 57.53 (154 / 1145) | 47.84 (701 / 14) | 102.93 (5000, `NodeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **0.53x** | $8.60 \pm 10.57$ | $14.56 \pm 10.62$ | 5.35 (1 / 24) | 10.15 (0 / 8) | 5.13 (2 / 4) | 4.77 (132) |
| `t2_binpack_44` | $44 \times 14$ | **0.07x** | $12.90 \pm 36.70$ | $7.64 \pm 2.39$ | 0.47 (1 / 45) | 7.07 (0 / 8) | 0.97 (13 / 5) | 15.59 (1838) |
| `t3_cflp_300` | $300 \times 34$ | **0.29x** | $20.48 \pm 8.15$ | $64.28 \pm 13.29$ | 16.97 (48 / 2131) | 57.93 (18 / 483) | 23.14 (63 / 31) | 7.53 (134) |
| `t3_sched_1000` | $1000 \times 100$ | **0.11x** | $7.96 \pm 1.40$ | $74.64 \pm 7.91$ | 7.98 (35 / 431) | 73.63 (0 / 76) | 12.30 (96 / 47) | 250.02 (4768, `TimeLimit`) |
<!-- END GENERATED:NON_MAD_CBC_GAP_TABLE -->

### 3. Summary of Architectural Bottlenecks

1. **PFI Eta Updates and Unscaled LP Cold Starts ([`lu.go`](lu.go), [`simplex.go`](simplex.go))**: Scanning up to `maxEtaUpdates = 56` PFI eta vectors between full Markowitz factor rebuilds increases per-pivot cost on large unpresolved bases (`t3_mad_g4_i1000`), while omitting LP presolve, matrix scaling, and anti-degeneracy cost shifts inflates pivot counts on degenerate unpresolved assignment LPs (`t3_mad_g10_i100`).
2. **Root-Only Cut Separation Without Multi-Row Aggregation ([`cuts.go`](cuts.go))**: Although [`bab.go`](bab.go) rolls back zero-gain cut tails and [`PurgeSlackCuts`](simplex.go) removes slack root cuts before branching, cutting planes are still separated only at the root node from single rows using global bounds. Multi-item lot-sizing (`t3_lotsizing_504`) and dense multi-constraint knapsacks (`t3_mkp_200`) require multi-row aggregation and in-tree cuts to match CBC's bound tightening.
3. **Static Presolve Symmetry Breaking Only ([`presolve.go`](presolve.go), [`bab.go`](bab.go))**: [`Presolve`](presolve.go) detects and verifies static $S_G$ group automorphisms (fixing designated bijections, upper-triangular partitioning/packing orbitopes, and monotone-cost block-opening indicators), which reduces multi-trial dispersion on `t2_mad_rand_g3_i9` and `t2_binpack_44`, whereas [`bab.go`](bab.go) does not perform dynamic orbital branching for symmetries that emerge only after branching decisions on identical-weight items.
4. **Trial Root Cut and Pre-Cut Heuristic Overhead ([`heuristics.go`](heuristics.go), [`bab.go`](bab.go))**: Post-cut heuristics are skipped when root cuts do not improve the LP bound, yet trial root cut passes (`CP6`) and pre-cut diving and sub-MIP solves (`CP7`) still add root LP overhead relative to `CP5 +Presolve` on models whose integrality gap must be closed by branching (`t2_mad_rand_g3_i9`, `t3_cflp_300`).

---

## Verification, Benchmarking, and Documentation Sync Commands

Run all commands from the `goblin/` directory:

```bash
# 0. Build the vendored COIN-OR CBC/CLP C++ stack into third_party/Cbc/dist (once per checkout)
./scripts/build_cbc.sh

# 1. Unit, regression, 293-trial CBC parity, and documentation gates (default pure-Go, SIMD, and opt-in CBC)
go test ./...
GOEXPERIMENT=simd go test ./...
go test -tags=cbc ./...

# 2. Regenerate the 31-family PuLP JSON and multi-trial JSONL corpus in testdata/corpus/
go run ./cmd/goblin-bench -generate-corpus

# 3. Commit first: goblin-bench records `git rev-parse HEAD` in every telemetry entry.
git status --short   # must be empty for clean provenance

# 4. Warm-state Python PuLP + COIN-OR CBC baseline, pinned to one performance core (also measures the CBC spawn floor)
CBC_BIN=/path/to/cbc-2.10.3 taskset -c 2 python3 bench/python_baseline.py --corpus-dir testdata/corpus --out-json bench/results/python_baseline.json --workers 1

# 5. Warm-state goblin benchmarks across Pure-Go (scalar and SIMD) and C++ CBC backends, pinned to the same performance core (CPU 2 on the i9-13900K)
go build -o /tmp/goblin-bench-scalar ./cmd/goblin-bench && taskset -c 2 /tmp/goblin-bench-scalar -backend go -no-simd -workers 1 -out bench/results/goblin_scalar.json
GOEXPERIMENT=simd go build -o /tmp/goblin-bench-simd ./cmd/goblin-bench && taskset -c 2 /tmp/goblin-bench-simd -backend go -workers 1 -out bench/results/goblin_simd.json
go build -tags=cbc -o /tmp/goblin-bench-cbc ./cmd/goblin-bench && taskset -c 2 /tmp/goblin-bench-cbc -backend cbc -workers 1 -out bench/results/goblin_cbc.json

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

---

## License

The `goblin` module is distributed under the [MIT License](LICENSE). Optional vendored COIN-OR sources under `third_party/` (`Cbc`, `Cgl`, `Clp`, `CoinUtils`, and `Osi`, linked only when building with `-tags cbc`) remain under their upstream `EPL-2.0` licenses.
