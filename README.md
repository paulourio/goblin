# `gopt`: Pure-Go LP and MILP Modeling and Branch-and-Cut Solver

The `gopt` package provides an in-memory Linear Programming (LP) and Mixed-Integer Linear Programming (MILP) modeling engine and Branch-and-Cut solver written in Go 1.27. It compiles algebraic models directly into Dual-Index Compressed Sparse Row and Column (`CSR`/`CSC`) matrices (`CompiledProblem`), eliminating intermediate text-file serialization and subprocess execution overhead.

## Architecture and Core Guarantees

1. **Zero-Subprocess In-Memory Execution**: Modeling (`Model`), matrix validation and compilation (`Model.Validate()`, `Model.CompileChecked()`), presolve (`Presolve`), LP relaxation (`LPSolver`), and Branch-and-Cut (`SolveCompiled`) execute entirely in-process using pooled workspace buffers (`AcquireWorkspace` / `ReleaseWorkspace`).
2. **Exact Schema Compatibility with Python `PuLP`**: The `gopt/format` package reads and writes `PuLP`'s native `toDict()` / `fromDict()` JSON schema ([`format/pulp_json.go`](format/pulp_json.go)) and standard fixed/free MPS files ([`format/mps.go`](format/mps.go)).
3. **Hardware SIMD Vector Kernels**: When compiled with `GOEXPERIMENT=simd` on `amd64`, [`internal/simd/kernels_amd64.go`](internal/simd/kernels_amd64.go) dispatches 256-bit AVX2 `archsimd.Float64x4` instructions (`VMULPD` + `VADDPD`) on dense slices ($n \ge 256$) while preserving IEEE-754 pairwise reduction parity with the scalar fallback ([`internal/simd/kernels_scalar.go`](internal/simd/kernels_scalar.go)).
4. **Verified Optimal Parity and Sound Resource-Limit Contracts**: Every instance across all 31 benchmark families (293 randomized trials in `testdata/corpus/*.json` and `testdata/corpus/trials/*.jsonl`) is verified against `PuLP + COIN-OR CBC 2.10.3` and in-memory C++ [`goblin`](../goblin) in [`solver_test.go`](solver_test.go). Resource-limited runs (`TimeLimit`, `MaxNodes`, `MaxIter`) return honest termination statuses (`StatusTimeLimit`, `StatusNodeLimit`, `StatusIterationLimit`) with global open-tree dual bounds.

---

## Documentation Index

Full technical specifications and architectural guides live in the [`docs/`](docs/index.md) directory:

- **[Architecture and Design Overview](docs/index.md)**: Module boundaries, data flow from algebraic expressions to `CSR`/`CSC` matrices, and memory pooling invariants.
- **[Modeling API and Serialization Formats](docs/modeling-and-formats.md)**: Constructing variables, linear expressions, and constraints, validating and compiling models, and exchanging `PuLP` JSON and MPS payloads.
- **[Solver Internals and Mathematical Specification](docs/solver-internals.md)**: Three-stage singleton + dense-nucleus LU factorization with PFI column-spike eta updates, Revised Dual and Two-Phase Primal Simplex with sparse `TabRowIdx` tableau rows, MIP presolve, cutting planes (`GMI`, Sequentially Lifted Knapsack Covers, Flow/VUB, `c-MIR`), primal heuristics, and Branch-and-Cut search.
- **[Benchmark Methodology and Corpus Results](docs/benchmarks.md)**: Hardware configuration, 4-way comparative telemetry against Python `PuLP + CBC` and in-memory C++ `goblin`, structural analysis of the Mean Absolute Deviation (`MAD`) `G x I` allocation family vs. random-weight negative controls (`BuildMADRandom`), and full checkpoint telemetry.

---

## Quickstart

Construct a Mixed-Integer Linear Program with [`NewModel`](model.go), add continuous, integer, or binary variables, and call `Solve()`:

```go
package main

import (
	"fmt"
	"math"

	"gopt"
)

func main() {
	m := gopt.NewModel("std_mip", gopt.Minimize)

	x := m.AddContVar("x", 0, 4)
	y := m.AddContVar("y", -1, 1)
	z := m.AddIntVar("z", 0, math.Inf(1))

	m.SetObjective(gopt.NewExpr(
		gopt.T(x, 1),
		gopt.T(y, 4),
		gopt.T(z, 9),
	))

	m.AddConstraint("c1", gopt.Le(gopt.NewExpr(gopt.T(x, 1), gopt.T(y, 1)), 5))
	m.AddConstraint("c2", gopt.Ge(gopt.NewExpr(gopt.T(x, 1), gopt.T(z, 1)), 10))
	m.AddConstraint("c3", gopt.Eq(gopt.NewExpr(gopt.T(y, -1), gopt.T(z, 1)), 7.5))

	res := m.Solve()
	fmt.Printf("status=%v objective=%.2f x=%.1f y=%.1f z=%.1f\n",
		res.Status, res.Objective, res.Value(x), res.Value(y), res.Value(z))
}
```

---

## Benchmarks

All benchmark tables in this `README.md`, [`docs/benchmarks.md`](docs/benchmarks.md), and [`bench/results/final_report.md`](bench/results/final_report.md) derive deterministically from the frozen JSON telemetry files (`bench/results/python_baseline.json`, `bench/results/goblin_cbc.json`, `bench/results/gopt_scalar.json`, and `bench/results/gopt_simd.json`). Automated gate tests in [`doc_sync_test.go`](doc_sync_test.go) fail `go test ./...` if any table drifts from the canonical JSON data.

### 1. Aggregate Workload Tier Summary (Geometric Mean and Median `[Min-Max]` Total Latency)

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | PyTotal GeoMean (ms) | GoblinTotal GeoMean (ms) | GoptScalar GeoMean (ms) | GoptSIMD GeoMean (ms) | `Py / gopt SIMD` GeoMean | `Py / gopt SIMD` Median [Min-Max] | `gopt SIMD / goblin` |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical (`N <= 22`, 10 families) | 4.09 | 1.23 | 0.04 | 0.05 | **85.3x** | 144.8x [8.1x-264.7x] | **0.04x** |
| Tier 2: Target p90 (`30 <= N <= 76`, 13 families) | 16.49 | 25.63 | 2.18 | 1.72 | **9.6x** | 13.3x [1.5x-25.4x] | **0.07x** |
| Tier 3: Large Tail (`135 <= N <= 4028`, 8 families) | 113.12 | 133.67 | 55.29 | 54.33 | **2.1x** | 1.3x [0.5x-11.9x] | **0.41x** |
| Tier 3 Non-MAD MILPs (`200 <= N <= 1000`, 4 families) | 145.04 | 214.01 | 31.26 | 33.43 | **4.3x** | 6.2x [1.2x-11.9x] | **0.16x** |
| MAD `G x I` Family (`22 <= N <= 4028`, 11 families) | 29.78 | 32.79 | 10.80 | 8.58 | **3.5x** | 1.8x [0.5x-20.8x] | **0.26x** |
| Unstructured Random MAD (`22 <= N <= 48`, 3 families) | 43.26 | 73.38 | 21.15 | 15.61 | **2.8x** | 1.8x [1.5x-8.1x] | **0.21x** |
| **All 31 Corpus Families (293 Random Trials)** | 17.29 | 14.73 | 1.35 | 1.32 | **13.1x** | 13.3x [0.5x-264.7x] | **0.09x** |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2. MAD Group-Item Allocation Family (`G` Groups $\times$ `I` Items, $T = 6$ Periods)

The Mean Absolute Deviation (`MAD`) allocation family ([`buildMADAllocation`](bench/corpus.go)) assigns $I$ items across $G$ groups over $T = 6$ periods to minimize total absolute deviation from each group's central level $m_g$. In `buildMADAllocation`, only the $G$ designated items $D = \{I-G, \dots, I-1\}$ carry nonzero orthogonal period weights ($\pm 360$), producing a zero integrality gap that `tryRootSubMIP` closes at node 1; unstructured random-weight negative controls (`BuildMADRandom`: `t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`) are included directly in the corpus and detailed in [`docs/benchmarks.md`](docs/benchmarks.md).

<!-- BEGIN GENERATED:MAD_PIPELINE_TABLE -->
| Problem ID | $G$ | $I$ | $T$ | Vars | Rows | Optimal Obj | PyPrep (ms) | GoPrep (ms) | PySolve (ms) | GoSolve Scalar (ms) | GoSolve SIMD (ms) | PyTotal (ms) | GoTotal SIMD (ms) | Speedup (SIMD) | Parity |
| :--- | --: | ---: | --: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_mad_g2_i4` | 2 | 4 | 6 | 22 | 30 | 2160.0 | 0.733 | 0.144 | 5.11 | 0.12 | 0.18 | 5.60 | 0.32 | **17.4x** | `OK` |
| `t1_mad_rand_g2_i4` | 2 | 4 | 6 | 22 | 32 | 5249.0 | 0.704 | 0.102 | 31.70 | 4.54 | 3.82 | 32.02 | 3.95 | **8.1x** | `OK` |
| `t2_mad_g2_i8` | 2 | 8 | 6 | 30 | 34 | 2160.0 | 1.019 | 0.166 | 7.42 | 0.25 | 0.23 | 8.09 | 0.39 | **20.8x** | `OK` |
| `t2_mad_g3_i9` | 3 | 9 | 6 | 48 | 48 | 4320.0 | 1.403 | 0.172 | 6.14 | 0.69 | 0.27 | 7.09 | 0.45 | **15.9x** | `OK` |
| `t2_mad_g4_i12` | 4 | 12 | 6 | 76 | 64 | 7200.0 | 1.901 | 0.274 | 8.86 | 2.26 | 1.15 | 10.39 | 1.43 | **7.2x** | `OK` |
| `t2_mad_rand_g2_i8` | 2 | 8 | 6 | 30 | 36 | 6696.0 | 1.012 | 0.134 | 12.25 | 11.08 | 7.24 | 12.92 | 7.36 | **1.8x** | `OK` |
| `t2_mad_rand_g3_i9` | 3 | 9 | 6 | 48 | 51 | 7958.0 | 1.413 | 0.243 | 194.77 | 176.43 | 130.31 | 195.77 | 130.70 | **1.5x** | `OK` |
| `t3_mad_g5_i20` | 5 | 20 | 6 | 135 | 85 | 10800.0 | 3.556 | 0.988 | 12.77 | 10.87 | 10.74 | 15.39 | 12.26 | **1.3x** | `OK` |
| `t3_mad_g6_i30` | 6 | 30 | 6 | 222 | 108 | 15120.0 | 6.451 | 1.240 | 20.50 | 18.12 | 16.41 | 23.01 | 17.50 | **1.3x** | `OK` |
| `t3_mad_g10_i100` | 10 | 100 | 6 | 1070 | 230 | 79200.0 | 27.655 | 6.492 | 172.11 | 494.18 | 410.36 | 189.47 | 417.25 | **0.5x** | `OK` |
| `t3_mad_g4_i1000` | 4 | 1000 | 6 | 4028 | 1052 | 187200.0 | 116.841 | 24.004 | 840.98 | 806.07 | 650.89 | 903.03 | 679.14 | **1.3x** | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

Multi-trial total latency dispersion (mean $\pm$ sample standard deviation and two-sided 95% Student-$t$ confidence interval across measured trials after warmup):

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `gopt` Scalar Total (Mean $\pm$ SD [95% CI], ms) | `gopt` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | $5.94 \pm 1.53$ ($\pm 1.03$) | $0.22 \pm 0.05$ ($\pm 0.03$) | $0.34 \pm 0.06$ ($\pm 0.04$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | $28.46 \pm 14.95$ ($\pm 10.05$) | $4.78 \pm 3.01$ ($\pm 2.02$) | $3.62 \pm 2.46$ ($\pm 1.65$) |
| `t2_mad_g2_i8` | $2 \times 8$ | $8.01 \pm 0.77$ ($\pm 0.52$) | $0.73 \pm 0.92$ ($\pm 0.62$) | $0.41 \pm 0.05$ ($\pm 0.03$) |
| `t2_mad_g3_i9` | $3 \times 9$ | $6.94 \pm 0.90$ ($\pm 0.60$) | $1.09 \pm 0.30$ ($\pm 0.20$) | $0.49 \pm 0.08$ ($\pm 0.05$) |
| `t2_mad_g4_i12` | $4 \times 12$ | $10.70 \pm 2.51$ ($\pm 1.68$) | $2.69 \pm 0.65$ ($\pm 0.44$) | $1.38 \pm 0.38$ ($\pm 0.25$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | $33.39 \pm 27.78$ ($\pm 18.66$) | $8.65 \pm 7.75$ ($\pm 5.20$) | $4.82 \pm 4.29$ ($\pm 2.88$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | $196.90 \pm 55.63$ ($\pm 37.37$) | $177.50 \pm 35.41$ ($\pm 23.78$) | $142.13 \pm 33.37$ ($\pm 22.41$) |
| `t3_mad_g5_i20` | $5 \times 20$ | $14.05 \pm 2.90$ ($\pm 3.60$) | $9.16 \pm 4.90$ ($\pm 6.08$) | $9.04 \pm 4.62$ ($\pm 5.73$) |
| `t3_mad_g6_i30` | $6 \times 30$ | $25.37 \pm 6.28$ ($\pm 7.80$) | $17.90 \pm 3.97$ ($\pm 4.93$) | $16.65 \pm 4.06$ ($\pm 5.05$) |
| `t3_mad_g10_i100` | $10 \times 100$ | $204.03 \pm 75.44$ ($\pm 93.66$) | $919.13 \pm 987.97$ ($\pm 1226.53$) | $833.47 \pm 954.80$ ($\pm 1185.35$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | $866.48 \pm 190.57$ ($\pm 236.59$) | $656.10 \pm 301.70$ ($\pm 374.55$) | $692.62 \pm 249.88$ ($\pm 310.21$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### 3. Full 31-Family Benchmark Corpus (`JSON -> Solution`)

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | PyPrep (ms) | GoPrep (ms) | PySolve (ms) | GoSolve Scalar (ms) | GoSolve SIMD (ms) | PyTotal (ms) | GoTotal SIMD (ms) | Speedup (SIMD) | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | 0.181 | 0.011 | 2.35 | 0.00 | 0.00 | 2.46 | 0.01 | **188.1x** | `OK` |
| `t1_std_lp_max` | 3 | 3 | 0.175 | 0.009 | 2.41 | 0.00 | 0.00 | 2.51 | 0.01 | **221.9x** | `OK` |
| `t1_std_mip` | 3 | 3 | 0.155 | 0.009 | 2.95 | 0.00 | 0.00 | 3.04 | 0.01 | **252.4x** | `OK` |
| `t1_mip_float_obj` | 3 | 3 | 0.161 | 0.009 | 3.09 | 0.00 | 0.00 | 3.18 | 0.01 | **264.7x** | `OK` |
| `t1_whiskas` | 6 | 5 | 0.223 | 0.020 | 2.43 | 0.00 | 0.00 | 2.57 | 0.03 | **101.6x** | `OK` |
| `t1_furniture` | 2 | 3 | 0.158 | 0.018 | 3.87 | 0.01 | 0.02 | 3.97 | 0.05 | **87.0x** | `OK` |
| `t1_knapsack_12` | 12 | 3 | 0.262 | 0.047 | 5.07 | 0.08 | 0.12 | 5.24 | 0.18 | **29.9x** | `OK` |
| `t1_infeasible_mip` | 2 | 1 | 0.126 | 0.012 | 2.20 | 0.00 | 0.00 | 2.27 | 0.01 | **195.2x** | `OK` |
| `t1_mad_g2_i4` | 22 | 30 | 0.733 | 0.144 | 5.11 | 0.12 | 0.18 | 5.60 | 0.32 | **17.4x** | `OK` |
| `t1_mad_rand_g2_i4` | 22 | 32 | 0.704 | 0.102 | 31.70 | 4.54 | 3.82 | 32.02 | 3.95 | **8.1x** | `OK` |
| `t2_mkp_30` | 30 | 6 | 0.556 | 0.107 | 44.98 | 5.05 | 3.27 | 45.35 | 3.40 | **13.3x** | `OK` |
| `t2_mkp_50` | 50 | 8 | 0.823 | 0.118 | 15.41 | 8.05 | 5.82 | 16.01 | 5.92 | **2.7x** | `OK` |
| `t2_cflp_36` | 36 | 11 | 0.568 | 0.090 | 6.59 | 0.13 | 0.20 | 6.95 | 0.29 | **23.7x** | `OK` |
| `t2_cflp_49` | 49 | 13 | 0.700 | 0.124 | 6.70 | 0.59 | 0.54 | 7.16 | 0.66 | **10.8x** | `OK` |
| `t2_gap_45` | 45 | 13 | 0.657 | 0.121 | 12.75 | 0.91 | 0.86 | 13.23 | 0.98 | **13.5x** | `OK` |
| `t2_lotsizing_48` | 48 | 32 | 0.822 | 0.105 | 71.53 | 10.05 | 5.94 | 72.10 | 6.03 | **12.0x** | `OK` |
| `t2_binpack_44` | 44 | 14 | 0.648 | 0.102 | 12.37 | 0.31 | 0.41 | 12.77 | 0.50 | **25.3x** | `OK` |
| `t2_setcover_50` | 50 | 20 | 1.161 | 0.195 | 9.36 | 0.18 | 0.17 | 10.09 | 0.40 | **25.4x** | `OK` |
| `t2_mad_g2_i8` | 30 | 34 | 1.019 | 0.166 | 7.42 | 0.25 | 0.23 | 8.09 | 0.39 | **20.8x** | `OK` |
| `t2_mad_g3_i9` | 48 | 48 | 1.403 | 0.172 | 6.14 | 0.69 | 0.27 | 7.09 | 0.45 | **15.9x** | `OK` |
| `t2_mad_g4_i12` | 76 | 64 | 1.901 | 0.274 | 8.86 | 2.26 | 1.15 | 10.39 | 1.43 | **7.2x** | `OK` |
| `t2_mad_rand_g2_i8` | 30 | 36 | 1.012 | 0.134 | 12.25 | 11.08 | 7.24 | 12.92 | 7.36 | **1.8x** | `OK` |
| `t2_mad_rand_g3_i9` | 48 | 51 | 1.413 | 0.243 | 194.77 | 176.43 | 130.31 | 195.77 | 130.70 | **1.5x** | `OK` |
| `t3_mkp_200` | 200 | 8 | 2.387 | 0.502 | 62.74 | 52.93 | 54.00 | 63.72 | 54.52 | **1.2x** | `OK` |
| `t3_cflp_300` | 300 | 34 | 3.486 | 0.490 | 356.28 | 23.50 | 29.66 | 358.45 | 30.13 | **11.9x** | `OK` |
| `t3_lotsizing_504` | 504 | 336 | 7.283 | 1.451 | 177.33 | 70.80 | 68.84 | 182.84 | 70.27 | **2.6x** | `OK` |
| `t3_sched_1000` | 1000 | 100 | 10.245 | 1.978 | 99.20 | 8.36 | 9.38 | 105.98 | 10.82 | **9.8x** | `OK` |
| `t3_mad_g5_i20` | 135 | 85 | 3.556 | 0.988 | 12.77 | 10.87 | 10.74 | 15.39 | 12.26 | **1.3x** | `OK` |
| `t3_mad_g6_i30` | 222 | 108 | 6.451 | 1.240 | 20.50 | 18.12 | 16.41 | 23.01 | 17.50 | **1.3x** | `OK` |
| `t3_mad_g10_i100` | 1070 | 230 | 27.655 | 6.492 | 172.11 | 494.18 | 410.36 | 189.47 | 417.25 | **0.5x** | `OK` |
| `t3_mad_g4_i1000` | 4028 | 1052 | 116.841 | 24.004 | 840.98 | 806.07 | 650.89 | 903.03 | 679.14 | **1.3x** | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## Verification, Benchmarking, and Documentation Sync Commands

Run all commands from the `gopt/` directory:

```bash
# 1. Run unit tests, CBC parity suite, and documentation sync gate (scalar and SIMD)
go test -v ./...
GOEXPERIMENT=simd go test -v ./...

# 2. Regenerate the 31-family PuLP JSON and multi-trial JSONL corpus in testdata/corpus/
go run ./cmd/gopt-bench -generate-corpus

# 3. Run the warm-state Python PuLP + COIN-OR CBC baseline across multi-seed trials
python3 bench/python_baseline.py --corpus-dir testdata/corpus --out-json bench/results/python_baseline.json --workers 1

# 4. Run warm-state gopt benchmarks in scalar and AVX2 SIMD modes
go run ./cmd/gopt-bench -workers 1 -out bench/results/gopt_scalar.json
GOEXPERIMENT=simd go run ./cmd/gopt-bench -workers 1 -out bench/results/gopt_simd.json

# 5. Synchronize README.md, docs/benchmarks.md, and bench/results/final_report.md from JSON telemetry
GOEXPERIMENT=simd go run ./cmd/gopt-bench -sync-docs

# 6. Verify that all documentation tables match canonical JSON telemetry (exits non-zero on drift)
go run ./cmd/gopt-bench -check-docs
```
