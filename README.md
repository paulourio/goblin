# `goblin`: Pure-Go LP and MILP Modeling and Branch-and-Cut Solver with Optional C++ CBC Backend

The `goblin` package provides an in-memory Linear Programming (LP) and Mixed-Integer Linear Programming (MILP) modeling engine and Branch-and-Cut solver written in Go 1.27. It compiles algebraic models directly into Dual-Index Compressed Sparse Row and Column (`CSR`/`CSC`) matrices (`CompiledProblem`), eliminating intermediate text-file I/O and subprocess spawn costs. By default it runs the pure-Go solver (`BackendGo`), and callers can select the in-process C++ COIN-OR CBC/CLP backend (`WithCBC()` / `WithBackend(BackendCBC)`) or configure solver flags (`WithoutSIMD()`, `WithTimeLimit(...)`, etc.) through functional arguments on `Solve` and `SolveCompiled`.

## Quickstart

Install the module with `go get github.com/paulourio/goblin`. Import `github.com/paulourio/goblin` as `gb`, construct a Mixed-Integer Linear Program with [`NewModel`](model.go), add continuous, integer, or binary variables, build terms and constraints with `x.T(coeff)` and `expr.Le(rhs)` (or `gb.T` / `gb.Le`), and call `m.Solve(...)`:

```go
package main

import (
	"fmt"
	"math"
	"time"

	gb "github.com/paulourio/goblin"
)

func main() {
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
	res := m.Solve(gb.WithTimeLimit(5 * time.Second))
	fmt.Printf("status=%v objective=%.2f x=%.1f y=%.1f z=%.1f\n",
		res.Status, res.Objective, res.Value(x), res.Value(y), res.Value(z))

	// Or solve with the in-process C++ COIN-OR CBC backend or with SIMD disabled:
	// resCBC := m.Solve(gb.WithCBC())
	// resScalar := m.Solve(gb.WithoutSIMD())
}
```

---

## Architecture and Core Guarantees

1. **Zero-Subprocess In-Memory Execution**: Modeling (`Model`), matrix checks and assembly (`Model.Validate()`, `Model.CompileChecked()`), presolve (`Presolve`), root and node LPs (`LPSolver`), and Branch-and-Cut (`SolveCompiled`) execute in-process using pooled workspace buffers (`AcquireWorkspace` / `ReleaseWorkspace`), with an optional in-process `cgo` bridge to COIN-OR CBC/CLP (`cbc_cgo.go`).
2. **Exact Schema Compatibility with Python `PuLP`**: The `goblin/format` package reads and writes `PuLP`'s native `toDict()` / `fromDict()` JSON schema ([`format/pulp_json.go`](format/pulp_json.go)) and standard fixed/free MPS files ([`format/mps.go`](format/mps.go)).
3. **Optional SIMD Kernels**: With `GOEXPERIMENT=simd` on `amd64`, [`internal/simd/kernels_amd64.go`](internal/simd/kernels_amd64.go) dispatches 256-bit AVX2 `archsimd.Float64x4` instructions (`VMULPD` + `VADDPD`) for slices of length 256 or more, and can be toggled at runtime via `WithoutSIMD()` or `WithSIMD(false)`. Results are bit-identical to the scalar kernels ([`internal/simd/kernels_scalar.go`](internal/simd/kernels_scalar.go)) on every `GOAMD64` level, which the kernel tests verify on both sides of the dispatch threshold. On the benchmark corpus the build shows no measurable speedup (most models never reach the AVX2 path); see the paired comparison in [`docs/benchmarks.md`](docs/benchmarks.md#5-default-build-vs-goexperimentsimd).
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
- **goblin telemetry source**: fingerprint `f409647e4eddfdee` (commit `ff6e5230482dce42de75d010b475c088846121a8`), `go1.27.1-X:nodwarf5`; the documentation gate rejects telemetry whose fingerprint differs from the checked-in sources.
- **CPU affinity**: goblin `2`; Python + CBC `2`.
- **Python reference**: PuLP `4.2.0`, CBC `2.10.3` (`cbc`).
- **CBC subprocess spawn floor**: 1.53 ms median (`prob.solve()` on a 1-variable MILP); the "Net of Spawn" columns subtract it.
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `master` (commit `c1c171ec`, vendored in `third_party/Cbc`), linked in-process through `cgo` (`goblin.WithCBC()`). The C++ backend maps default settings onto CBC with two deliberate differences: `MaxCutPasses = 12` becomes `passC = 30` and `MaxNodes = 50000` becomes unlimited. It is therefore not a like-for-like setup and not the CBC 2.10.3 binary used by the Python baseline.
<!-- END GENERATED:PROVENANCE -->

Read the ratios with three caveats. Python's solve time includes a fixed CBC process-spawn cost, reported separately as "Net of Spawn". The `goblin (CBC)` column runs a different CBC build with different settings. The planted-gap MAD family is solved at the root by construction, so the bold row excludes it.

### 1. Aggregate Workload Summary (Geometric Mean and Median `[Min-Max]` Total Latency)

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families | `Py / goblin (Go)` GeoMean | `Py net / goblin (Go)` | `goblin (Go) / CBC` | `goblin (Go)` (ms) | `goblin (CBC)` (ms) | Py Total (ms) | Py Net (ms) | `Py / goblin (Go)` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical (`N <= 26`) | 10 | **54.8x** | 1.8x | 0.09x | 0.05 | 0.51 | 2.50 | 0.08 | 88.8x [6.0x-161.2x] |
| Tier 2: Target p90 (`30 <= N <= 84`) | 13 | **5.8x** | 4.5x | 0.31x | 1.89 | 6.03 | 11.03 | 8.43 | 6.7x [0.9x-18.3x] |
| Tier 3: Large Tail (`145 <= N <= 4036`) | 8 | **1.8x** | 1.7x | 0.84x | 33.78 | 40.26 | 59.90 | 56.86 | 1.6x [0.5x-7.4x] |
| Tier 3 non-MAD MILPs | 4 | **2.2x** | 2.2x | 0.73x | 41.21 | 56.63 | 92.48 | 90.63 | 2.4x [0.7x-7.4x] |
| Planted-gap MAD family (zero integrality gap by construction) | 8 | **3.8x** | 2.9x | 0.41x | 3.29 | 8.11 | 12.66 | 9.57 | 4.9x [0.5x-20.8x] |
| Random-weight MAD (negative controls) | 3 | **2.6x** | 2.5x | 0.92x | 22.34 | 24.36 | 58.88 | 56.80 | 3.6x [0.9x-6.0x] |
| **All families except planted-gap MAD** | 23 | **11.8x** | 2.5x | 0.23x | 0.84 | 3.60 | 9.93 | 2.07 | 9.5x [0.7x-161.2x] |
| All corpus families | 31 | **8.8x** | 2.6x | 0.27x | 1.20 | 4.44 | 10.57 | 3.08 | 7.4x [0.5x-161.2x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2. MAD Group-Item Allocation Family (`G` Groups $\times$ `I` Items, $T = 6$ Periods)

The `MAD` group-item allocation family ([`BuildMADFromWeights`](bench/corpus.go)) assigns $I$ items across $G$ groups over $T = 6$ periods to minimize total L1 distance from each group's central level $m_g$ plus the roundness penalty $r_g = |m_g - 100 n_g|$ relative to an integer multiple $n_g \in \mathbb{Z}_{\ge 1}$. In the planted-gap generator, items in $S = \{0, \dots, I-G-1\}$ carry orthogonal curve signatures cancelled by the $G$ designated items $D = \{I-G, \dots, I-1\}$ with roundness-aligned period sums, yielding a zero root LP gap that root heuristics close at node 1; unstructured random-weight negative controls (`BuildMADRandom`: `t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`) are included directly in the corpus and detailed in [`docs/benchmarks.md`](docs/benchmarks.md).

<!-- BEGIN GENERATED:MAD_PIPELINE_TABLE -->
| Problem ID | $G \times I$ | Vars | Rows | `Py / goblin (Go)` | GoTotal (ms) | PyTotal (ms) | GoSolve (ms) | PySolve (ms) | PySolve Net (ms) | GoPrep (ms) | PyPrep (ms) | Optimal Obj | Parity |
| :--- | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **20.8x** | 0.18 | 3.82 | 0.08 | 3.63 | 2.10 | 0.110 | 0.246 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **6.0x** | 4.12 | 24.75 | 3.99 | 24.53 | 22.99 | 0.097 | 0.281 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **16.6x** | 0.21 | 3.43 | 0.09 | 3.18 | 1.65 | 0.116 | 0.329 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **6.7x** | 0.63 | 4.21 | 0.38 | 3.83 | 2.30 | 0.225 | 0.486 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **5.3x** | 0.99 | 5.28 | 0.60 | 4.71 | 3.17 | 0.293 | 0.740 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **3.6x** | 12.14 | 43.22 | 12.00 | 42.91 | 41.38 | 0.132 | 0.382 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **0.9x** | 222.98 | 190.87 | 222.77 | 190.46 | 188.93 | 0.203 | 0.542 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **4.5x** | 2.19 | 9.85 | 1.66 | 8.88 | 7.35 | 0.537 | 1.308 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **1.7x** | 7.41 | 12.68 | 6.62 | 10.99 | 9.45 | 0.894 | 2.243 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **0.5x** | 88.16 | 46.51 | 82.95 | 37.92 | 36.39 | 4.763 | 10.812 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **1.0x** | 409.76 | 389.97 | 395.05 | 345.93 | 344.40 | 14.753 | 44.695 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

Multi-trial total latency spread (mean $\pm$ sample SD and two-sided 95% Student-$t$ confidence interval across measured trials after warmup):

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | $0.22 \pm 0.05$ ($\pm 0.03$) | $3.75 \pm 0.22$ ($\pm 0.15$) | $0.22 \pm 0.05$ ($\pm 0.03$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | $3.45 \pm 1.90$ ($\pm 1.28$) | $22.50 \pm 8.42$ ($\pm 5.66$) | $3.48 \pm 1.92$ ($\pm 1.29$) |
| `t2_mad_g2_i8` | $2 \times 8$ | $0.23 \pm 0.04$ ($\pm 0.03$) | $3.59 \pm 0.32$ ($\pm 0.22$) | $0.24 \pm 0.04$ ($\pm 0.02$) |
| `t2_mad_g3_i9` | $3 \times 9$ | $0.66 \pm 0.16$ ($\pm 0.11$) | $4.20 \pm 0.05$ ($\pm 0.03$) | $0.61 \pm 0.12$ ($\pm 0.08$) |
| `t2_mad_g4_i12` | $4 \times 12$ | $0.97 \pm 0.22$ ($\pm 0.15$) | $5.30 \pm 0.09$ ($\pm 0.06$) | $0.96 \pm 0.29$ ($\pm 0.20$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | $11.58 \pm 6.59$ ($\pm 4.43$) | $31.51 \pm 18.28$ ($\pm 12.28$) | $11.48 \pm 6.55$ ($\pm 4.40$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | $252.88 \pm 190.78$ ($\pm 128.16$) | $166.14 \pm 82.17$ ($\pm 55.20$) | $252.27 \pm 189.42$ ($\pm 127.25$) |
| `t3_mad_g5_i20` | $5 \times 20$ | $2.16 \pm 0.29$ ($\pm 0.36$) | $9.31 \pm 1.08$ ($\pm 1.34$) | $2.22 \pm 0.15$ ($\pm 0.19$) |
| `t3_mad_g6_i30` | $6 \times 30$ | $6.11 \pm 2.67$ ($\pm 3.31$) | $13.63 \pm 2.27$ ($\pm 2.82$) | $6.29 \pm 2.86$ ($\pm 3.55$) |
| `t3_mad_g10_i100` | $10 \times 100$ | $88.80 \pm 21.08$ ($\pm 26.17$) | $47.36 \pm 3.41$ ($\pm 4.24$) | $88.79 \pm 20.25$ ($\pm 25.13$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | $430.75 \pm 110.14$ ($\pm 136.73$) | $463.02 \pm 195.32$ ($\pm 242.49$) | $421.81 \pm 102.95$ ($\pm 127.81$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### 3. Full 31-Family Benchmark Corpus (`JSON -> Solution`)

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | `Py / goblin (Go)` | GoTotal (ms) | PyTotal (ms) | GoSolve (ms) | PySolve (ms) | PySolve Net (ms) | GoPrep (ms) | PyPrep (ms) | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | **98.9x** | 0.01 | 1.34 | 0.00 | 1.30 | 0.00 | 0.011 | 0.056 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **90.2x** | 0.01 | 1.34 | 0.00 | 1.26 | 0.00 | 0.011 | 0.056 | `OK` |
| `t1_std_mip` | 3 | 3 | **106.6x** | 0.02 | 2.01 | 0.01 | 1.96 | 0.43 | 0.012 | 0.059 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **100.4x** | 0.02 | 1.91 | 0.00 | 1.87 | 0.33 | 0.012 | 0.057 | `OK` |
| `t1_whiskas` | 6 | 5 | **50.3x** | 0.03 | 1.39 | 0.00 | 1.33 | 0.00 | 0.023 | 0.082 | `OK` |
| `t1_furniture` | 2 | 3 | **87.4x** | 0.03 | 2.23 | 0.02 | 2.18 | 0.65 | 0.011 | 0.057 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **28.8x** | 0.11 | 3.21 | 0.08 | 3.14 | 1.61 | 0.031 | 0.092 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **161.2x** | 0.01 | 1.45 | 0.00 | 1.42 | 0.00 | 0.009 | 0.046 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **20.8x** | 0.18 | 3.82 | 0.08 | 3.63 | 2.10 | 0.110 | 0.246 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **6.0x** | 4.12 | 24.75 | 3.99 | 24.53 | 22.99 | 0.097 | 0.281 | `OK` |
| `t2_mkp_30` | 30 | 6 | **6.3x** | 4.37 | 27.67 | 4.31 | 27.50 | 25.97 | 0.063 | 0.208 | `OK` |
| `t2_mkp_50` | 50 | 8 | **1.0x** | 10.97 | 11.47 | 10.82 | 11.23 | 9.69 | 0.109 | 0.324 | `OK` |
| `t2_cflp_36` | 36 | 11 | **10.9x** | 0.33 | 3.56 | 0.26 | 3.39 | 1.86 | 0.060 | 0.191 | `OK` |
| `t2_cflp_49` | 49 | 13 | **4.0x** | 1.03 | 4.15 | 0.93 | 3.96 | 2.42 | 0.078 | 0.259 | `OK` |
| `t2_gap_45` | 45 | 13 | **9.5x** | 0.90 | 8.48 | 0.81 | 8.28 | 6.75 | 0.077 | 0.241 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **8.4x** | 4.74 | 39.67 | 4.66 | 39.44 | 37.91 | 0.092 | 0.290 | `OK` |
| `t2_binpack_44` | 44 | 14 | **12.3x** | 0.55 | 6.73 | 0.44 | 6.57 | 5.04 | 0.093 | 0.212 | `OK` |
| `t2_setcover_50` | 50 | 20 | **18.3x** | 0.29 | 5.37 | 0.13 | 5.11 | 3.57 | 0.142 | 0.353 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **16.6x** | 0.21 | 3.43 | 0.09 | 3.18 | 1.65 | 0.116 | 0.329 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **6.7x** | 0.63 | 4.21 | 0.38 | 3.83 | 2.30 | 0.225 | 0.486 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **5.3x** | 0.99 | 5.28 | 0.60 | 4.71 | 3.17 | 0.293 | 0.740 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **3.6x** | 12.14 | 43.22 | 12.00 | 42.91 | 41.38 | 0.132 | 0.382 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **0.9x** | 222.98 | 190.87 | 222.77 | 190.46 | 188.93 | 0.203 | 0.542 | `OK` |
| `t3_mkp_200` | 200 | 8 | **0.7x** | 61.18 | 41.23 | 60.88 | 40.62 | 39.09 | 0.312 | 0.782 | `OK` |
| `t3_cflp_300` | 300 | 34 | **7.4x** | 28.36 | 211.02 | 27.74 | 210.13 | 208.60 | 0.440 | 1.147 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **3.4x** | 38.18 | 128.49 | 37.40 | 126.64 | 125.11 | 0.764 | 2.268 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **1.5x** | 43.52 | 65.45 | 42.30 | 62.22 | 60.69 | 1.240 | 3.667 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **4.5x** | 2.19 | 9.85 | 1.66 | 8.88 | 7.35 | 0.537 | 1.308 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **1.7x** | 7.41 | 12.68 | 6.62 | 10.99 | 9.45 | 0.894 | 2.243 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **0.5x** | 88.16 | 46.51 | 82.95 | 37.92 | 36.39 | 4.763 | 10.812 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **1.0x** | 409.76 | 389.97 | 395.05 | 345.93 | 344.40 | 14.753 | 44.695 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## Pure-Go vs. C++ CBC Performance Gaps and MAD Scaling Analysis

While pure-Go `goblin` (`BackendGo`) achieves lower latency than both Python `PuLP + CBC` and in-process `goblin (CBC)` (`BackendCBC`) on small and medium models, direct comparison against `BackendCBC` and `COIN-OR CLP` exposes four structural performance gaps on large or symmetric instances.

### 1. MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families (`N = G(I + T + 3)` variables, `M = G(2T + 3) + I` constraints) separate solver performance into three distinct scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Total (`Go / CBC`) | Go Total (ms) | CBC Total (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) | Py Net Solve (ms) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.10x** | 0.18 | 1.88 | 0.50x | 0.08 (1 / 28) | 1.79 (0 / 0) | 0.058 (28) | 0.116 (19) | 2.10 |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.38x** | 4.12 | 10.72 | 0.49x | 3.99 (11 / 244) | 10.61 (6 / 83) | 0.058 (30) | 0.118 (18) | 22.99 |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.11x** | 0.21 | 1.81 | 0.61x | 0.09 (1 / 34) | 1.68 (0 / 0) | 0.071 (34) | 0.116 (21) | 1.65 |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.25x** | 0.63 | 2.50 | 0.88x | 0.38 (1 / 60) | 2.31 (0 / 0) | 0.179 (60) | 0.204 (40) | 2.30 |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.30x** | 0.99 | 3.30 | 0.79x | 0.60 (1 / 68) | 3.01 (0 / 0) | 0.269 (68) | 0.339 (63) | 3.17 |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.76x** | 12.14 | 15.93 | 0.79x | 12.00 (47 / 693) | 15.81 (12 / 216) | 0.095 (38) | 0.120 (24) | 41.38 |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **2.63x** | 222.98 | 84.72 | 0.88x | 222.77 (2101 / 25012) | 84.50 (198 / 1300) | 0.211 (66) | 0.240 (54) | 188.93 |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.27x** | 2.19 | 8.22 | 1.34x | 1.66 (1 / 110) | 7.72 (0 / 0) | 0.698 (110) | 0.521 (94) | 7.35 |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.95x** | 7.41 | 7.78 | 5.75x | 6.62 (1 / 416) | 6.94 (0 / 0) | 5.039 (416) | 0.876 (149) | 9.45 |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **2.36x** | 88.16 | 37.38 | 26.92x | 82.95 (1 / 2774) | 33.58 (0 / 0) | 79.513 (2774) | 2.954 (319) | 36.39 |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **1.46x** | 409.76 | 280.67 | 10.89x | 395.05 (1 / 4139) | 266.85 (0 / 0) | 367.698 (4139) | 33.753 (3128) | 344.40 |
<!-- END GENERATED:MAD_CBC_GAP_TABLE -->

1. **Small-to-Medium Planted-Gap MAD (`t1_mad_g2_i4` through `t3_mad_g5_i20`, $26 \le N \le 145$)**: Every structured `t*_mad_g*` instance has a zero root LP gap by design (`RootLPBound == OptimalObj`). Pure-Go closes these models at node 1 through root heuristics (`tryPropagatedRounding`, `tryRootSubMIP`, and `tryFractionalDiving` in [`heuristics.go`](heuristics.go)) without C++ `CbcModel` and `CglPreProcess` setup costs.
2. **Large Planted-Gap MAD Root LP Bottleneck (`t3_mad_g6_i30`, `t3_mad_g10_i100`, `t3_mad_g4_i1000`)**: Although pure-Go still closes the integer gap at node 1 without branching, virtually all solve time is consumed by the Root LP ([`simplex.go`](simplex.go) and [`lu.go`](lu.go)):
   - **Degenerate Simplex Iteration Growth (`t3_mad_g10_i100`)**: Partitioning 100 items across $G = 10$ identical groups creates severe primal and dual degeneracy. `COIN-OR CLP` mitigates this via LP presolve (`ClpPresolve`), geometric-mean matrix scaling, anti-degeneracy cost shifts, and crash basis assembly (`Idiot` / triangular crash). In contrast, [`LPSolver`](simplex.go) starts from an unscaled all-slack basis without LP presolve or cost shifts, and its Phase II primal fallback uses full Dantzig pricing rather than primal steepest-edge or Devex pricing.
   - **Per-Pivot PFI Eta Update Cost (`t3_mad_g4_i1000`)**: On 1,000-row bases where simplex pivot counts are comparable, pure-Go costs more per pivot because the basis factorizer ([`lu.go`](lu.go)) appends a Product-Form of the Inverse (PFI) eta file (`PushEtaSparse`) up to `maxEtaUpdates = 56` before rebuilding factors from scratch, whereas `CLP` updates the $U$ factor in place via Forrest-Tomlin updates.
3. **Multi-Group Symmetry and Root Cut/Heuristic Overhead on Random-Weight MAD (`t2_mad_rand_g3_i9`)**: Assigning dense random weights (`BuildMADRandom`) removes the zero-gap structure and forces Branch-and-Cut to prove the optimal bound across $G!$ group-permutation symmetries:
   - For $G = 2$ (`t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`), $2! = 2$ symmetry is small and pure-Go remains faster than `BackendCBC` despite exploring more nodes.
   - For $G = 3$ (`t2_mad_rand_g3_i9`, $3! = 6$ symmetry), pure-Go explores an order of magnitude more Branch-and-Bound nodes and simplex pivots than `BackendCBC`. Furthermore, root cuts (`CP6`) remain permanently in `ExtraRows` without cut aging or purging, and root heuristics (`CP7`) execute sub-MIP and diving LPs that do not break the $G!$ symmetry, making `CP7 Full` slower than `CP5 +Presolve` (`Presolve=on, Cuts=off, Heuristics=off`).

### 2. Non-MAD Corpus Gaps and Checkpoint Trade-Offs

Across the 20 non-MAD families, pure-Go trails `BackendCBC` on median latency, mean spread, or root heuristic overhead on five models:

<!-- BEGIN GENERATED:NON_MAD_CBC_GAP_TABLE -->
| Problem ID | $N \times M$ | Total Median (`Go / CBC`) | Go Total Mean $\pm$ SD (ms) | CBC Total Mean $\pm$ SD (ms) | Go CP7 Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go CP6 +Cuts (ms / nodes / cuts) | Go CP5 +Presolve (ms / nodes) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t3_mkp_200` | $200 \times 8$ | **1.28x** | $61.54 \pm 31.16$ | $39.73 \pm 19.09$ | 60.88 (540 / 5328) | 47.39 (154 / 1145) | 53.54 (739 / 21) | 97.86 (5000, `NodeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **1.31x** | $13.59 \pm 14.01$ | $11.91 \pm 8.69$ | 10.82 (1 / 24) | 8.25 (0 / 8) | 4.23 (2 / 5) | 3.89 (132) |
| `t2_binpack_44` | $44 \times 14$ | **0.09x** | $26.23 \pm 81.16$ | $6.41 \pm 2.09$ | 0.44 (1 / 44) | 5.76 (0 / 8) | 1.20 (18 / 12) | 16.11 (1895) |
| `t3_cflp_300` | $300 \times 34$ | **0.57x** | $33.04 \pm 10.61$ | $54.18 \pm 10.65$ | 27.74 (9 / 910) | 49.36 (18 / 483) | 15.22 (39 / 37) | 5.93 (134) |
| `t3_sched_1000` | $1000 \times 100$ | **0.72x** | $40.88 \pm 11.25$ | $61.43 \pm 4.82$ | 42.30 (17 / 366) | 59.12 (0 / 76) | 10.52 (96 / 50) | 214.09 (5000, `NodeLimit`) |
<!-- END GENERATED:NON_MAD_CBC_GAP_TABLE -->

### 3. Summary of Architectural Bottlenecks

1. **PFI Eta Updates and Unscaled LP Cold Starts ([`lu.go`](lu.go), [`simplex.go`](simplex.go))**: Scanning up to `maxEtaUpdates = 56` PFI eta vectors between full Markowitz factor rebuilds increases per-pivot cost on large bases (`t3_mad_g4_i1000`), while omitting LP presolve, matrix scaling, and anti-degeneracy cost shifts inflates pivot counts on degenerate assignment LPs (`t3_mad_g10_i100`).
2. **Root-Only Global Cuts Without Cut Aging ([`cuts.go`](cuts.go))**: Cutting planes are separated only at the root node using global bounds and retained for the entire tree. On multi-thousand-node trees (`t2_mad_rand_g3_i9`), unpurged cuts enlarge every node LP, while dense multi-constraint knapsacks (`t3_mkp_200`) require multi-row aggregation and in-tree cuts to match CBC's bound tightening.
3. **No Orbital Symmetry Breaking ([`bab.go`](bab.go))**: Permutation-symmetric models (`t2_mad_rand_g3_i9`, and outlier seed 9 of `t2_binpack_44`) force pure-Go to enumerate symmetric subtrees that CBC prunes.
4. **Unthrottled Root Diving and Sub-MIP Passes ([`heuristics.go`](heuristics.go))**: [`tryRootSubMIP`](heuristics.go) and [`tryFractionalDiving`](heuristics.go) run fixed pivot and node budgets at the root even when presolve (`CP5` on `t2_mad_rand_g3_i9` and `t3_cflp_300`) or root cuts (`CP6` on `t2_mkp_50` and `t3_sched_1000`) already solve the instance faster.

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

# 6. Render README.md, docs/benchmarks.md, and bench/results/final_report.md from the telemetry
go run ./cmd/goblin-bench -sync-docs

# 7. Verify documentation, parity, and provenance (exits non-zero on any failure)
go run ./cmd/goblin-bench -check-docs

# 8. Differential test against CBC on freshly generated random MILPs (needs PuLP with CBC)
python3 bench/difftest_cbc.py --instances 400 --seed 1
```
