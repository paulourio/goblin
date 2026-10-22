# `gopt`: Pure-Go LP and MILP Modeling and Branch-and-Cut Solver

The `gopt` package provides an in-memory Linear Programming (LP) and Mixed-Integer Linear Programming (MILP) modeling engine and Branch-and-Cut solver written in Go 1.27. It compiles algebraic models directly into Dual-Index Compressed Sparse Row and Column (`CSR`/`CSC`) matrices (`CompiledProblem`), eliminating intermediate text-file serialization and subprocess execution overhead.

## Architecture and Core Guarantees

1. **Zero-Subprocess In-Memory Execution**: Modeling (`Model`), matrix validation and compilation (`Model.Validate()`, `Model.CompileChecked()`), presolve (`Presolve`), LP relaxation (`LPSolver`), and Branch-and-Cut (`SolveCompiled`) execute entirely in-process using pooled workspace buffers (`AcquireWorkspace` / `ReleaseWorkspace`).
2. **Exact Schema Compatibility with Python `PuLP`**: The `gopt/format` package reads and writes `PuLP`'s native `toDict()` / `fromDict()` JSON schema ([`format/pulp_json.go`](format/pulp_json.go)) and standard fixed/free MPS files ([`format/mps.go`](format/mps.go)).
3. **Optional SIMD Kernels**: With `GOEXPERIMENT=simd` on `amd64`, [`internal/simd/kernels_amd64.go`](internal/simd/kernels_amd64.go) dispatches 256-bit AVX2 `archsimd.Float64x4` instructions (`VMULPD` + `VADDPD`) for slices of length 256 or more. Results are bit-identical to the scalar kernels ([`internal/simd/kernels_scalar.go`](internal/simd/kernels_scalar.go)) on every `GOAMD64` level, which the kernel tests verify on both sides of the dispatch threshold. On the benchmark corpus the build shows no measurable speedup (most models never reach the AVX2 path); see the paired comparison in [`docs/benchmarks.md`](docs/benchmarks.md#5-default-build-vs-goexperimentsimd).
4. **Verified Parity and Resource-Limit Contracts**: `TestAllTrialParity` in [`solver_test.go`](solver_test.go) re-solves all 293 randomized trial instances of the 31 benchmark families (`testdata/corpus/trials/*.jsonl`), compares status and objective with the frozen `PuLP + COIN-OR CBC 2.10.3` and goblin results, and checks every gopt solution for feasibility. [`regression_test.go`](regression_test.go) adds big-M fixed-charge models with enumerated ground truth and replays fuzz instances on which gopt or CBC failed. `TimeLimit`, `MaxNodes`, and `MaxIter` (all in `Options`) return `StatusTimeLimit`, `StatusNodeLimit`, or `StatusIterationLimit` with a valid global bound once the root LP is solved; a node that cannot be evaluated is never discarded silently.

---

## Documentation Index

Full technical specifications and architectural guides live in the [`docs/`](docs/index.md) directory:

- **[Architecture and Design Overview](docs/index.md)**: Module boundaries, data flow from algebraic expressions to `CSR`/`CSC` matrices, and memory pooling invariants.
- **[Modeling API and Serialization Formats](docs/modeling-and-formats.md)**: Constructing variables, linear expressions, and constraints, validating and compiling models, and exchanging `PuLP` JSON and MPS payloads.
- **[Solver Internals and Mathematical Specification](docs/solver-internals.md)**: Three-stage singleton + dense-nucleus LU factorization with PFI column-spike eta updates, Revised Dual and Two-Phase Primal Simplex with sparse `TabRowIdx` tableau rows, MIP presolve, cutting planes (`GMI`, Sequentially Lifted Knapsack Covers, Flow/VUB, `c-MIR`), primal heuristics, and Branch-and-Cut search.
- **[Benchmark Methodology and Corpus Results](docs/benchmarks.md)**: Hardware configuration, comparative telemetry against Python `PuLP + CBC` and in-process `goblin`, CBC spawn-floor separation, paired scalar-vs-SIMD statistics, structural analysis of the Mean Absolute Deviation (`MAD`) `G x I` allocation family vs. random-weight negative controls (`BuildMADRandom`), and full checkpoint telemetry.

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

All benchmark tables in this `README.md`, [`docs/benchmarks.md`](docs/benchmarks.md), and [`bench/results/final_report.md`](bench/results/final_report.md) are rendered from the JSON telemetry in `bench/results/` (`python_baseline.json`, `goblin_cbc.json`, `gopt_scalar.json`, `gopt_simd.json`). [`doc_sync_test.go`](doc_sync_test.go) fails `go test ./...` if a table drifts from the telemetry, if any gopt build or goblin disagrees with CBC on a status or objective, or if the telemetry lacks git provenance. Timings use the default (scalar) build.

<!-- BEGIN GENERATED:PROVENANCE -->
- **gopt telemetry source**: fingerprint `313e00cf5a6fffb3` (working tree on top of commit `652204b7fb6e729dabef1707fb6cc2440593d9dc`), `go1.27.1-X:nodwarf5`; the documentation gate rejects telemetry whose fingerprint differs from the checked-in sources
- **CPU affinity**: gopt `2`; Python + CBC `2`
- **Python reference**: PuLP `4.2.0`, CBC `2.10.3` (`cbc`)
- **CBC subprocess spawn floor**: 2.09 ms median (`prob.solve()` on a 1-variable MILP); the "Net of Spawn" columns subtract it
- **goblin**: COIN-OR CBC/CLP `master` (commit `c1c171ec`, built in `../Cbc`), linked in-process through `cgo`. goblin maps gopt's default options onto CBC with two deliberate differences: `MaxCutPasses = 12` becomes `passC = 30` and `MaxNodes = 50000` becomes unlimited. It is therefore not a like-for-like configuration and not the CBC 2.10.3 binary used by the Python baseline.
<!-- END GENERATED:PROVENANCE -->

Read the ratios with three caveats. Python's solve time includes a fixed CBC process-spawn cost, reported separately as "Net of Spawn". goblin runs a different CBC build with different options. The planted-gap MAD family is solved at the root by construction, so the bold row excludes it.

### 1. Aggregate Workload Summary (Geometric Mean and Median `[Min-Max]` Total Latency)

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families | Py Total GeoMean (ms) | Py Net-of-Spawn GeoMean (ms) | goblin GeoMean (ms) | gopt GeoMean (ms) | `Py / gopt` GeoMean | `Py / gopt` Median [Min-Max] | `Py net / gopt` GeoMean | `gopt / goblin` |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical (`N <= 22`) | 10 | 3.86 | 0.38 | 1.23 | 0.06 | **69.9x** | 84.4x [7.5x-387.4x] | 6.9x | 0.04x |
| Tier 2: Target p90 (`30 <= N <= 76`) | 13 | 14.37 | 11.37 | 25.63 | 1.75 | **8.2x** | 11.3x [1.3x-35.7x] | 6.5x | 0.07x |
| Tier 3: Large Tail (`135 <= N <= 4028`) | 8 | 101.99 | 97.98 | 133.67 | 48.63 | **2.1x** | 1.7x [0.2x-10.8x] | 2.0x | 0.36x |
| Tier 3 non-MAD MILPs | 4 | 120.68 | 118.18 | 214.01 | 24.75 | **4.9x** | 6.8x [1.5x-10.8x] | 4.8x | 0.12x |
| Planted-gap MAD family (zero integrality gap by construction) | 8 | 23.99 | 19.16 | 24.24 | 7.37 | **3.3x** | 3.3x [0.2x-30.1x] | 2.6x | 0.30x |
| Random-weight MAD (negative controls) | 3 | 44.33 | 40.97 | 73.38 | 18.57 | **2.4x** | 1.4x [1.3x-7.5x] | 2.2x | 0.25x |
| **All families except planted-gap MAD** | 23 | 13.42 | 4.59 | 12.39 | 0.75 | **17.9x** | 15.2x [1.3x-387.4x] | 6.1x | 0.06x |
| All corpus families | 31 | 15.59 | 6.64 | 14.73 | 1.36 | **11.5x** | 11.3x [0.2x-387.4x] | 4.9x | 0.09x |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2. MAD Group-Item Allocation Family (`G` Groups $\times$ `I` Items, $T = 6$ Periods)

The Mean Absolute Deviation (`MAD`) allocation family ([`buildMADAllocation`](bench/corpus.go)) assigns $I$ items across $G$ groups over $T = 6$ periods to minimize total absolute deviation from each group's central level $m_g$. In `buildMADAllocation`, only the $G$ designated items $D = \{I-G, \dots, I-1\}$ carry nonzero orthogonal period weights ($\pm 360$), producing a zero integrality gap that `tryRootSubMIP` closes at node 1; unstructured random-weight negative controls (`BuildMADRandom`: `t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`) are included directly in the corpus and detailed in [`docs/benchmarks.md`](docs/benchmarks.md).

<!-- BEGIN GENERATED:MAD_PIPELINE_TABLE -->
| Problem ID | $G$ | $I$ | $T$ | Vars | Rows | Optimal Obj | PyPrep (ms) | GoPrep (ms) | PySolve (ms) | PySolve Net of Spawn (ms) | GoSolve (ms) | PyTotal (ms) | GoTotal (ms) | `Py / gopt` | Parity |
| :--- | --: | ---: | --: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_mad_g2_i4` | 2 | 4 | 6 | 22 | 30 | 2160.0 | 0.291 | 0.082 | 5.23 | 3.13 | 0.10 | 5.45 | 0.18 | **30.1x** | `OK` |
| `t1_mad_rand_g2_i4` | 2 | 4 | 6 | 22 | 32 | 5249.0 | 0.443 | 0.168 | 34.90 | 32.80 | 4.62 | 35.23 | 4.72 | **7.5x** | `OK` |
| `t2_mad_g2_i8` | 2 | 8 | 6 | 30 | 34 | 2160.0 | 0.626 | 0.191 | 4.94 | 2.85 | 0.22 | 5.48 | 0.41 | **13.3x** | `OK` |
| `t2_mad_g3_i9` | 3 | 9 | 6 | 48 | 48 | 4320.0 | 0.961 | 0.312 | 7.57 | 5.48 | 0.52 | 8.33 | 0.82 | **10.1x** | `OK` |
| `t2_mad_g4_i12` | 4 | 12 | 6 | 76 | 64 | 7200.0 | 1.298 | 0.511 | 7.32 | 5.23 | 1.37 | 7.99 | 1.69 | **4.7x** | `OK` |
| `t2_mad_rand_g2_i8` | 2 | 8 | 6 | 30 | 36 | 6696.0 | 0.692 | 0.264 | 13.37 | 11.28 | 10.49 | 13.93 | 10.79 | **1.3x** | `OK` |
| `t2_mad_rand_g3_i9` | 3 | 9 | 6 | 48 | 51 | 7958.0 | 0.599 | 0.355 | 177.04 | 174.95 | 125.30 | 177.47 | 125.71 | **1.4x** | `OK` |
| `t3_mad_g5_i20` | 5 | 20 | 6 | 135 | 85 | 10800.0 | 2.720 | 0.684 | 12.85 | 10.76 | 8.61 | 14.90 | 9.17 | **1.6x** | `OK` |
| `t3_mad_g6_i30` | 6 | 30 | 6 | 222 | 108 | 15120.0 | 2.982 | 1.545 | 28.08 | 25.99 | 15.37 | 30.12 | 16.91 | **1.8x** | `OK` |
| `t3_mad_g10_i100` | 10 | 100 | 6 | 1070 | 230 | 79200.0 | 16.389 | 5.695 | 183.31 | 181.21 | 902.33 | 195.76 | 908.02 | **0.2x** | `OK` |
| `t3_mad_g4_i1000` | 4 | 1000 | 6 | 4028 | 1052 | 187200.0 | 77.944 | 25.776 | 560.97 | 558.88 | 575.51 | 628.55 | 591.73 | **1.1x** | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

Multi-trial total latency dispersion (mean $\pm$ sample standard deviation and two-sided 95% Student-$t$ confidence interval across measured trials after warmup):

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `gopt` Scalar Total (Mean $\pm$ SD [95% CI], ms) | `gopt` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | $5.14 \pm 1.05$ ($\pm 0.70$) | $0.42 \pm 0.78$ ($\pm 0.52$) | $0.31 \pm 0.02$ ($\pm 0.01$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | $28.09 \pm 14.34$ ($\pm 9.63$) | $4.02 \pm 2.36$ ($\pm 1.58$) | $4.35 \pm 2.81$ ($\pm 1.89$) |
| `t2_mad_g2_i8` | $2 \times 8$ | $5.63 \pm 1.12$ ($\pm 0.75$) | $0.42 \pm 0.05$ ($\pm 0.03$) | $0.45 \pm 0.08$ ($\pm 0.05$) |
| `t2_mad_g3_i9` | $3 \times 9$ | $7.66 \pm 1.10$ ($\pm 0.74$) | $0.87 \pm 0.15$ ($\pm 0.10$) | $0.86 \pm 0.12$ ($\pm 0.08$) |
| `t2_mad_g4_i12` | $4 \times 12$ | $8.24 \pm 0.88$ ($\pm 0.59$) | $1.95 \pm 0.64$ ($\pm 0.43$) | $2.22 \pm 0.66$ ($\pm 0.44$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | $34.80 \pm 30.02$ ($\pm 20.17$) | $7.70 \pm 6.84$ ($\pm 4.60$) | $5.89 \pm 5.71$ ($\pm 3.84$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | $175.93 \pm 35.71$ ($\pm 23.99$) | $127.27 \pm 27.24$ ($\pm 18.30$) | $135.36 \pm 23.63$ ($\pm 15.88$) |
| `t3_mad_g5_i20` | $5 \times 20$ | $14.21 \pm 1.40$ ($\pm 1.74$) | $7.35 \pm 3.86$ ($\pm 4.80$) | $7.47 \pm 3.99$ ($\pm 4.95$) |
| `t3_mad_g6_i30` | $6 \times 30$ | $35.69 \pm 23.51$ ($\pm 29.19$) | $15.31 \pm 4.73$ ($\pm 5.87$) | $12.17 \pm 2.65$ ($\pm 3.30$) |
| `t3_mad_g10_i100` | $10 \times 100$ | $231.54 \pm 90.64$ ($\pm 112.53$) | $784.42 \pm 665.56$ ($\pm 826.27$) | $820.26 \pm 700.15$ ($\pm 869.21$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | $608.55 \pm 62.25$ ($\pm 77.29$) | $578.77 \pm 253.21$ ($\pm 314.36$) | $561.21 \pm 209.27$ ($\pm 259.80$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### 3. Full 31-Family Benchmark Corpus (`JSON -> Solution`)

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | PyPrep (ms) | GoPrep (ms) | PySolve (ms) | PySolve Net of Spawn (ms) | GoSolve (ms) | PyTotal (ms) | GoTotal (ms) | `Py / gopt` | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | 0.087 | 0.020 | 1.86 | 0.00 | 0.00 | 1.92 | 0.02 | **88.8x** | `OK` |
| `t1_std_lp_max` | 3 | 3 | 0.091 | 0.019 | 1.73 | 0.00 | 0.00 | 1.79 | 0.02 | **80.0x** | `OK` |
| `t1_std_mip` | 3 | 3 | 0.122 | 0.019 | 3.35 | 1.25 | 0.01 | 3.45 | 0.03 | **114.1x** | `OK` |
| `t1_mip_float_obj` | 3 | 3 | 0.129 | 0.020 | 3.21 | 1.12 | 0.01 | 3.30 | 0.03 | **113.1x** | `OK` |
| `t1_whiskas` | 6 | 5 | 0.184 | 0.024 | 2.76 | 0.67 | 0.00 | 2.91 | 0.03 | **100.7x** | `OK` |
| `t1_furniture` | 2 | 3 | 0.087 | 0.017 | 2.92 | 0.83 | 0.02 | 2.98 | 0.04 | **77.9x** | `OK` |
| `t1_knapsack_12` | 12 | 3 | 0.142 | 0.031 | 4.87 | 2.77 | 0.07 | 4.98 | 0.11 | **44.5x** | `OK` |
| `t1_infeasible_mip` | 2 | 1 | 0.095 | 0.006 | 2.21 | 0.11 | 0.00 | 2.27 | 0.01 | **387.4x** | `OK` |
| `t1_mad_g2_i4` | 22 | 30 | 0.291 | 0.082 | 5.23 | 3.13 | 0.10 | 5.45 | 0.18 | **30.1x** | `OK` |
| `t1_mad_rand_g2_i4` | 22 | 32 | 0.443 | 0.168 | 34.90 | 32.80 | 4.62 | 35.23 | 4.72 | **7.5x** | `OK` |
| `t2_mkp_30` | 30 | 6 | 0.377 | 0.077 | 37.88 | 35.79 | 3.34 | 38.23 | 3.40 | **11.2x** | `OK` |
| `t2_mkp_50` | 50 | 8 | 0.585 | 0.138 | 14.52 | 12.43 | 5.80 | 14.82 | 5.98 | **2.5x** | `OK` |
| `t2_cflp_36` | 36 | 11 | 0.420 | 0.059 | 6.08 | 3.98 | 0.12 | 6.47 | 0.18 | **35.7x** | `OK` |
| `t2_cflp_49` | 49 | 13 | 0.491 | 0.083 | 6.66 | 4.57 | 0.41 | 7.09 | 0.52 | **13.5x** | `OK` |
| `t2_gap_45` | 45 | 13 | 0.345 | 0.074 | 10.44 | 8.35 | 0.64 | 10.81 | 0.71 | **15.2x** | `OK` |
| `t2_lotsizing_48` | 48 | 32 | 0.550 | 0.108 | 58.84 | 56.74 | 5.17 | 59.24 | 5.26 | **11.3x** | `OK` |
| `t2_binpack_44` | 44 | 14 | 0.473 | 0.151 | 10.99 | 8.89 | 0.48 | 11.38 | 0.62 | **18.3x** | `OK` |
| `t2_setcover_50` | 50 | 20 | 0.418 | 0.225 | 6.20 | 4.11 | 0.18 | 6.51 | 0.42 | **15.5x** | `OK` |
| `t2_mad_g2_i8` | 30 | 34 | 0.626 | 0.191 | 4.94 | 2.85 | 0.22 | 5.48 | 0.41 | **13.3x** | `OK` |
| `t2_mad_g3_i9` | 48 | 48 | 0.961 | 0.312 | 7.57 | 5.48 | 0.52 | 8.33 | 0.82 | **10.1x** | `OK` |
| `t2_mad_g4_i12` | 76 | 64 | 1.298 | 0.511 | 7.32 | 5.23 | 1.37 | 7.99 | 1.69 | **4.7x** | `OK` |
| `t2_mad_rand_g2_i8` | 30 | 36 | 0.692 | 0.264 | 13.37 | 11.28 | 10.49 | 13.93 | 10.79 | **1.3x** | `OK` |
| `t2_mad_rand_g3_i9` | 48 | 51 | 0.599 | 0.355 | 177.04 | 174.95 | 125.30 | 177.47 | 125.71 | **1.4x** | `OK` |
| `t3_mkp_200` | 200 | 8 | 1.071 | 0.437 | 51.71 | 49.62 | 35.07 | 52.38 | 35.56 | **1.5x** | `OK` |
| `t3_cflp_300` | 300 | 34 | 1.362 | 0.632 | 259.76 | 257.67 | 23.55 | 261.58 | 24.25 | **10.8x** | `OK` |
| `t3_lotsizing_504` | 504 | 336 | 2.514 | 0.836 | 153.28 | 151.18 | 42.70 | 155.39 | 43.57 | **3.6x** | `OK` |
| `t3_sched_1000` | 1000 | 100 | 6.842 | 1.967 | 95.76 | 93.67 | 8.28 | 99.61 | 9.99 | **10.0x** | `OK` |
| `t3_mad_g5_i20` | 135 | 85 | 2.720 | 0.684 | 12.85 | 10.76 | 8.61 | 14.90 | 9.17 | **1.6x** | `OK` |
| `t3_mad_g6_i30` | 222 | 108 | 2.982 | 1.545 | 28.08 | 25.99 | 15.37 | 30.12 | 16.91 | **1.8x** | `OK` |
| `t3_mad_g10_i100` | 1070 | 230 | 16.389 | 5.695 | 183.31 | 181.21 | 902.33 | 195.76 | 908.02 | **0.2x** | `OK` |
| `t3_mad_g4_i1000` | 4028 | 1052 | 77.944 | 25.776 | 560.97 | 558.88 | 575.51 | 628.55 | 591.73 | **1.1x** | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## Verification, Benchmarking, and Documentation Sync Commands

Run all commands from the `gopt/` directory:

```bash
# 1. Unit, regression, 293-trial CBC/goblin parity, and documentation gates (both builds)
go test ./...
GOEXPERIMENT=simd go test ./...

# 2. Regenerate the 31-family PuLP JSON and multi-trial JSONL corpus in testdata/corpus/
go run ./cmd/gopt-bench -generate-corpus

# 3. Commit first: gopt-bench records `git rev-parse HEAD` in every telemetry entry.
git status --short   # must be empty for clean provenance

# 4. Warm-state Python PuLP + COIN-OR CBC baseline, pinned to one performance core (also measures the CBC spawn floor)
CBC_BIN=/path/to/cbc-2.10.3 taskset -c 2 python3 bench/python_baseline.py --corpus-dir testdata/corpus --out-json bench/results/python_baseline.json --workers 1

# 5. Warm-state gopt benchmarks, pinned to the same performance core (CPU 2 on the i9-13900K)
go build -o /tmp/gopt-bench-scalar ./cmd/gopt-bench && taskset -c 2 /tmp/gopt-bench-scalar -workers 1 -out bench/results/gopt_scalar.json
GOEXPERIMENT=simd go build -o /tmp/gopt-bench-simd ./cmd/gopt-bench && taskset -c 2 /tmp/gopt-bench-simd -workers 1 -out bench/results/gopt_simd.json

# 6. Render README.md, docs/benchmarks.md, and bench/results/final_report.md from the telemetry
go run ./cmd/gopt-bench -sync-docs

# 7. Verify documentation, parity, and provenance (exits non-zero on any failure)
go run ./cmd/gopt-bench -check-docs

# 8. Differential test against CBC on freshly generated random MILPs (needs PuLP with CBC)
python3 bench/difftest_cbc.py --instances 400 --seed 1
```
