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
- **goblin telemetry source**: fingerprint `f409647e4eddfdee` (working tree on top of commit `1b1a98018251d5283ec57c65e12dc0707f2d55f5`), `go1.27.1-X:nodwarf5`; the documentation gate rejects telemetry whose fingerprint differs from the checked-in sources.
- **CPU affinity**: goblin `2`; Python + CBC `2`.
- **Python reference**: PuLP `4.2.0`, CBC `2.10.3` (`cbc`).
- **CBC subprocess spawn floor**: 1.53 ms median (`prob.solve()` on a 1-variable MILP); the "Net of Spawn" columns subtract it.
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `master` (commit `c1c171ec`, vendored in `third_party/Cbc`), linked in-process through `cgo` (`goblin.WithCBC()`). The C++ backend maps default settings onto CBC with two deliberate differences: `MaxCutPasses = 12` becomes `passC = 30` and `MaxNodes = 50000` becomes unlimited. It is therefore not a like-for-like setup and not the CBC 2.10.3 binary used by the Python baseline.
<!-- END GENERATED:PROVENANCE -->

Read the ratios with three caveats. Python's solve time includes a fixed CBC process-spawn cost, reported separately as "Net of Spawn". The `goblin (CBC)` column runs a different CBC build with different settings. The planted-gap MAD family is solved at the root by construction, so the bold row excludes it.

### 1. Aggregate Workload Summary (Geometric Mean and Median `[Min-Max]` Total Latency)

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families | Py Total GeoMean (ms) | Py Net-of-Spawn GeoMean (ms) | `goblin (CBC)` GeoMean (ms) | `goblin (Go)` GeoMean (ms) | `Py / goblin (Go)` GeoMean | `Py / goblin (Go)` Median [Min-Max] | `Py net / goblin (Go)` GeoMean | `goblin (Go) / CBC` |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical (`N <= 26`) | 10 | 2.50 | 0.08 | 0.51 | 0.04 | **68.2x** | 124.9x [6.2x-172.0x] | 2.2x | 0.07x |
| Tier 2: Target p90 (`30 <= N <= 84`) | 13 | 11.03 | 8.43 | 6.09 | 1.77 | **6.2x** | 7.0x [0.9x-17.9x] | 4.8x | 0.29x |
| Tier 3: Large Tail (`145 <= N <= 4036`) | 8 | 59.90 | 56.86 | 39.83 | 34.04 | **1.8x** | 1.6x [0.5x-7.7x] | 1.7x | 0.85x |
| Tier 3 non-MAD MILPs | 4 | 92.48 | 90.63 | 55.63 | 41.54 | **2.2x** | 2.4x [0.7x-7.7x] | 2.2x | 0.75x |
| Planted-gap MAD family (zero integrality gap by construction) | 8 | 12.66 | 9.57 | 8.05 | 3.29 | **3.8x** | 5.2x [0.5x-18.0x] | 2.9x | 0.41x |
| Random-weight MAD (negative controls) | 3 | 58.88 | 56.80 | 24.56 | 21.86 | **2.7x** | 3.7x [0.9x-6.2x] | 2.6x | 0.89x |
| **All families except planted-gap MAD** | 23 | 9.93 | 2.07 | 3.63 | 0.74 | **13.4x** | 10.9x [0.7x-172.0x] | 2.8x | 0.20x |
| All corpus families | 31 | 10.57 | 3.08 | 4.46 | 1.09 | **9.7x** | 7.7x [0.5x-172.0x] | 2.8x | 0.24x |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2. MAD Group-Item Allocation Family (`G` Groups $\times$ `I` Items, $T = 6$ Periods)

The `MAD` group-item allocation family ([`BuildMADFromWeights`](bench/corpus.go)) assigns $I$ items across $G$ groups over $T = 6$ periods to minimize total L1 distance from each group's central level $m_g$ plus the roundness penalty $r_g = |m_g - 100 n_g|$ relative to an integer multiple $n_g \in \mathbb{Z}_{\ge 1}$. In the planted-gap generator, items in $S = \{0, \dots, I-G-1\}$ carry orthogonal curve signatures cancelled by the $G$ designated items $D = \{I-G, \dots, I-1\}$ with roundness-aligned period sums, yielding a zero root LP gap that root heuristics close at node 1; unstructured random-weight negative controls (`BuildMADRandom`: `t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`) are included directly in the corpus and detailed in [`docs/benchmarks.md`](docs/benchmarks.md).

<!-- BEGIN GENERATED:MAD_PIPELINE_TABLE -->
| Problem ID | $G$ | $I$ | $T$ | Vars | Rows | Optimal Obj | PyPrep (ms) | GoPrep (ms) | PySolve (ms) | PySolve Net of Spawn (ms) | GoSolve (ms) | PyTotal (ms) | GoTotal (ms) | `Py / goblin (Go)` | Parity |
| :--- | --: | ---: | --: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_mad_g2_i4` | 2 | 4 | 6 | 26 | 34 | 2400.0 | 0.246 | 0.097 | 3.63 | 2.10 | 0.07 | 3.82 | 0.21 | **18.0x** | `OK` |
| `t1_mad_rand_g2_i4` | 2 | 4 | 6 | 26 | 34 | 4601.0 | 0.281 | 0.095 | 24.53 | 22.99 | 3.89 | 24.75 | 3.98 | **6.2x** | `OK` |
| `t2_mad_g2_i8` | 2 | 8 | 6 | 34 | 38 | 2400.0 | 0.329 | 0.112 | 3.18 | 1.65 | 0.09 | 3.43 | 0.20 | **17.0x** | `OK` |
| `t2_mad_g3_i9` | 3 | 9 | 6 | 54 | 54 | 4800.0 | 0.486 | 0.177 | 3.83 | 2.30 | 0.37 | 4.21 | 0.60 | **7.0x** | `OK` |
| `t2_mad_g4_i12` | 4 | 12 | 6 | 84 | 72 | 8000.0 | 0.740 | 0.299 | 4.71 | 3.17 | 0.53 | 5.28 | 0.89 | **5.9x** | `OK` |
| `t2_mad_rand_g2_i8` | 2 | 8 | 6 | 34 | 38 | 5975.0 | 0.382 | 0.143 | 42.91 | 41.38 | 11.59 | 43.22 | 11.71 | **3.7x** | `OK` |
| `t2_mad_rand_g3_i9` | 3 | 9 | 6 | 54 | 54 | 7720.0 | 0.542 | 0.207 | 190.46 | 188.93 | 224.03 | 190.87 | 224.25 | **0.9x** | `OK` |
| `t3_mad_g5_i20` | 5 | 20 | 6 | 145 | 95 | 12000.0 | 1.308 | 0.520 | 8.88 | 7.35 | 1.64 | 9.85 | 2.22 | **4.4x** | `OK` |
| `t3_mad_g6_i30` | 6 | 30 | 6 | 234 | 120 | 16800.0 | 2.243 | 0.828 | 10.99 | 9.45 | 6.81 | 12.68 | 7.60 | **1.7x** | `OK` |
| `t3_mad_g10_i100` | 10 | 100 | 6 | 1090 | 250 | 88000.0 | 10.812 | 4.834 | 37.92 | 36.39 | 84.36 | 46.51 | 89.45 | **0.5x** | `OK` |
| `t3_mad_g4_i1000` | 4 | 1000 | 6 | 4036 | 1060 | 208000.0 | 44.695 | 16.110 | 345.93 | 344.40 | 385.49 | 389.97 | 400.91 | **1.0x** | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

Multi-trial total latency spread (mean $\pm$ sample SD and two-sided 95% Student-$t$ confidence interval across measured trials after warmup):

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | $3.75 \pm 0.22$ ($\pm 0.15$) | $0.22 \pm 0.07$ ($\pm 0.05$) | $0.19 \pm 0.04$ ($\pm 0.03$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | $22.50 \pm 8.42$ ($\pm 5.66$) | $3.43 \pm 1.86$ ($\pm 1.25$) | $3.42 \pm 1.88$ ($\pm 1.26$) |
| `t2_mad_g2_i8` | $2 \times 8$ | $3.59 \pm 0.32$ ($\pm 0.22$) | $0.22 \pm 0.03$ ($\pm 0.02$) | $0.23 \pm 0.04$ ($\pm 0.03$) |
| `t2_mad_g3_i9` | $3 \times 9$ | $4.20 \pm 0.05$ ($\pm 0.03$) | $0.63 \pm 0.17$ ($\pm 0.12$) | $0.63 \pm 0.13$ ($\pm 0.08$) |
| `t2_mad_g4_i12` | $4 \times 12$ | $5.30 \pm 0.09$ ($\pm 0.06$) | $0.98 \pm 0.19$ ($\pm 0.13$) | $0.97 \pm 0.22$ ($\pm 0.15$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | $31.51 \pm 18.28$ ($\pm 12.28$) | $11.73 \pm 6.74$ ($\pm 4.53$) | $11.99 \pm 6.82$ ($\pm 4.58$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | $166.14 \pm 82.17$ ($\pm 55.20$) | $254.88 \pm 193.46$ ($\pm 129.96$) | $251.13 \pm 189.44$ ($\pm 127.26$) |
| `t3_mad_g5_i20` | $5 \times 20$ | $9.31 \pm 1.08$ ($\pm 1.34$) | $2.19 \pm 0.18$ ($\pm 0.23$) | $2.19 \pm 0.14$ ($\pm 0.17$) |
| `t3_mad_g6_i30` | $6 \times 30$ | $13.63 \pm 2.27$ ($\pm 2.82$) | $6.06 \pm 2.73$ ($\pm 3.39$) | $6.33 \pm 2.91$ ($\pm 3.61$) |
| `t3_mad_g10_i100` | $10 \times 100$ | $47.36 \pm 3.41$ ($\pm 4.24$) | $89.72 \pm 21.80$ ($\pm 27.06$) | $92.04 \pm 22.36$ ($\pm 27.76$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | $463.02 \pm 195.32$ ($\pm 242.49$) | $427.98 \pm 109.60$ ($\pm 136.06$) | $427.70 \pm 106.52$ ($\pm 132.24$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### 3. Full 31-Family Benchmark Corpus (`JSON -> Solution`)

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | PyPrep (ms) | GoPrep (ms) | PySolve (ms) | PySolve Net of Spawn (ms) | GoSolve (ms) | PyTotal (ms) | GoTotal (ms) | `Py / goblin (Go)` | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | 0.056 | 0.008 | 1.30 | 0.00 | 0.00 | 1.34 | 0.01 | **142.4x** | `OK` |
| `t1_std_lp_max` | 3 | 3 | 0.056 | 0.007 | 1.26 | 0.00 | 0.00 | 1.34 | 0.01 | **152.1x** | `OK` |
| `t1_std_mip` | 3 | 3 | 0.059 | 0.008 | 1.96 | 0.43 | 0.00 | 2.01 | 0.01 | **172.0x** | `OK` |
| `t1_mip_float_obj` | 3 | 3 | 0.057 | 0.008 | 1.87 | 0.33 | 0.00 | 1.91 | 0.01 | **162.5x** | `OK` |
| `t1_whiskas` | 6 | 5 | 0.082 | 0.016 | 1.33 | 0.00 | 0.00 | 1.39 | 0.02 | **65.7x** | `OK` |
| `t1_furniture` | 2 | 3 | 0.057 | 0.008 | 2.18 | 0.65 | 0.01 | 2.23 | 0.02 | **107.4x** | `OK` |
| `t1_knapsack_12` | 12 | 3 | 0.092 | 0.026 | 3.14 | 1.61 | 0.09 | 3.21 | 0.11 | **28.3x** | `OK` |
| `t1_infeasible_mip` | 2 | 1 | 0.046 | 0.008 | 1.42 | 0.00 | 0.00 | 1.45 | 0.01 | **160.9x** | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | 0.246 | 0.097 | 3.63 | 2.10 | 0.07 | 3.82 | 0.21 | **18.0x** | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | 0.281 | 0.095 | 24.53 | 22.99 | 3.89 | 24.75 | 3.98 | **6.2x** | `OK` |
| `t2_mkp_30` | 30 | 6 | 0.208 | 0.066 | 27.50 | 25.97 | 4.00 | 27.67 | 4.07 | **6.8x** | `OK` |
| `t2_mkp_50` | 50 | 8 | 0.324 | 0.114 | 11.23 | 9.69 | 11.55 | 11.47 | 11.66 | **1.0x** | `OK` |
| `t2_cflp_36` | 36 | 11 | 0.191 | 0.059 | 3.39 | 1.86 | 0.26 | 3.56 | 0.33 | **10.9x** | `OK` |
| `t2_cflp_49` | 49 | 13 | 0.259 | 0.081 | 3.96 | 2.42 | 0.67 | 4.15 | 0.77 | **5.4x** | `OK` |
| `t2_gap_45` | 45 | 13 | 0.241 | 0.078 | 8.28 | 6.75 | 0.67 | 8.48 | 0.76 | **11.2x** | `OK` |
| `t2_lotsizing_48` | 48 | 32 | 0.290 | 0.098 | 39.44 | 37.91 | 4.87 | 39.67 | 4.96 | **8.0x** | `OK` |
| `t2_binpack_44` | 44 | 14 | 0.212 | 0.074 | 6.57 | 5.04 | 0.37 | 6.73 | 0.45 | **15.0x** | `OK` |
| `t2_setcover_50` | 50 | 20 | 0.353 | 0.124 | 5.11 | 3.57 | 0.16 | 5.37 | 0.30 | **17.9x** | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | 0.329 | 0.112 | 3.18 | 1.65 | 0.09 | 3.43 | 0.20 | **17.0x** | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | 0.486 | 0.177 | 3.83 | 2.30 | 0.37 | 4.21 | 0.60 | **7.0x** | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | 0.740 | 0.299 | 4.71 | 3.17 | 0.53 | 5.28 | 0.89 | **5.9x** | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | 0.382 | 0.143 | 42.91 | 41.38 | 11.59 | 43.22 | 11.71 | **3.7x** | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | 0.542 | 0.207 | 190.46 | 188.93 | 224.03 | 190.87 | 224.25 | **0.9x** | `OK` |
| `t3_mkp_200` | 200 | 8 | 0.782 | 0.294 | 40.62 | 39.09 | 61.92 | 41.23 | 62.21 | **0.7x** | `OK` |
| `t3_cflp_300` | 300 | 34 | 1.147 | 0.437 | 210.13 | 208.60 | 26.88 | 211.02 | 27.44 | **7.7x** | `OK` |
| `t3_lotsizing_504` | 504 | 336 | 2.268 | 0.771 | 126.64 | 125.11 | 38.39 | 128.49 | 39.16 | **3.3x** | `OK` |
| `t3_sched_1000` | 1000 | 100 | 3.667 | 1.264 | 62.22 | 60.69 | 43.29 | 65.45 | 44.56 | **1.5x** | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | 1.308 | 0.520 | 8.88 | 7.35 | 1.64 | 9.85 | 2.22 | **4.4x** | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | 2.243 | 0.828 | 10.99 | 9.45 | 6.81 | 12.68 | 7.60 | **1.7x** | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | 10.812 | 4.834 | 37.92 | 36.39 | 84.36 | 46.51 | 89.45 | **0.5x** | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | 44.695 | 16.110 | 345.93 | 344.40 | 385.49 | 389.97 | 400.91 | **1.0x** | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## Pure-Go vs. C++ CBC Performance Gaps and MAD Scaling Analysis

While pure-Go `goblin` (`BackendGo`) achieves lower latency than both Python `PuLP + CBC` and in-process `goblin (CBC)` (`BackendCBC`) on small and medium models, direct comparison against `BackendCBC` and `COIN-OR CLP` exposes four structural performance gaps on large or symmetric instances.

### 1. MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families (`N = G(I + T + 3)` variables, `M = G(2T + 3) + I` constraints) separate solver performance into three distinct scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Py Net Solve (ms) | Go Total (ms) | CBC Total (ms) | Total (`Go / CBC`) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | 0.054 (28) | 0.120 (19) | 0.45x | 0.07 (1 / 28) | 1.81 (0 / 0) | 2.10 | 0.21 | 1.90 | **0.11x** |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | 0.063 (30) | 0.121 (18) | 0.52x | 3.89 (11 / 244) | 10.76 (6 / 83) | 22.99 | 3.98 | 10.87 | **0.37x** |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | 0.072 (34) | 0.116 (21) | 0.62x | 0.09 (1 / 34) | 1.66 (0 / 0) | 1.65 | 0.20 | 1.78 | **0.11x** |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | 0.178 (60) | 0.201 (40) | 0.88x | 0.37 (1 / 60) | 2.27 (0 / 0) | 2.30 | 0.60 | 2.46 | **0.24x** |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | 0.268 (68) | 0.343 (63) | 0.78x | 0.53 (1 / 68) | 2.94 (0 / 0) | 3.17 | 0.89 | 3.22 | **0.28x** |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | 0.097 (38) | 0.121 (24) | 0.80x | 11.59 (47 / 693) | 15.87 (12 / 216) | 41.38 | 11.71 | 16.00 | **0.73x** |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | 0.217 (66) | 0.239 (54) | 0.91x | 224.03 (2101 / 25012) | 85.00 (198 / 1300) | 188.93 | 224.25 | 85.23 | **2.63x** |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | 0.673 (110) | 0.522 (94) | 1.29x | 1.64 (1 / 110) | 7.68 (0 / 0) | 7.35 | 2.22 | 8.20 | **0.27x** |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | 4.972 (416) | 0.872 (149) | 5.70x | 6.81 (1 / 416) | 6.87 (0 / 0) | 9.45 | 7.60 | 7.69 | **0.99x** |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | 78.835 (2774) | 2.914 (319) | 27.05x | 84.36 (1 / 2774) | 33.47 (0 / 0) | 36.39 | 89.45 | 37.33 | **2.40x** |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | 373.324 (4139) | 33.509 (3128) | 11.14x | 385.49 (1 / 4139) | 266.96 (0 / 0) | 344.40 | 400.91 | 280.86 | **1.43x** |
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
| Problem ID | $N \times M$ | Go CP5 +Presolve (ms / nodes) | Go CP6 +Cuts (ms / nodes / cuts) | Go CP7 Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Total Mean $\pm$ SD (ms) | CBC Total Mean $\pm$ SD (ms) | Total Median (`Go / CBC`) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t3_mkp_200` | $200 \times 8$ | 98.33 (5000, `NodeLimit`) | 52.66 (739 / 21) | 61.92 (540 / 5328) | 47.79 (154 / 1145) | $61.60 \pm 30.98$ | $40.52 \pm 19.95$ | **1.29x** |
| `t2_mkp_50` | $50 \times 8$ | 3.85 (132) | 3.76 (2 / 5) | 11.55 (1 / 24) | 8.36 (0 / 8) | $13.60 \pm 14.01$ | $12.07 \pm 8.80$ | **1.37x** |
| `t2_binpack_44` | $44 \times 14$ | 16.45 (1895) | 1.24 (18 / 12) | 0.37 (1 / 44) | 5.83 (0 / 8) | $26.26 \pm 81.36$ | $6.42 \pm 2.11$ | **0.08x** |
| `t3_cflp_300` | $300 \times 34$ | 5.66 (134) | 15.21 (39 / 37) | 26.88 (9 / 910) | 47.97 (18 / 483) | $31.64 \pm 12.03$ | $54.58 \pm 11.81$ | **0.57x** |
| `t3_sched_1000` | $1000 \times 100$ | 218.76 (5000, `NodeLimit`) | 10.37 (96 / 50) | 43.29 (17 / 366) | 57.85 (0 / 76) | $41.24 \pm 12.16$ | $60.21 \pm 4.59$ | **0.75x** |
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
