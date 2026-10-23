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
- **[Solver Internals and Mathematical Specification](docs/solver-internals.md)**: Three-stage singleton and sparse Markowitz LU factorization with `FTRANSparse`/`BTRANSparse` and PFI eta updates, Revised Dual Simplex with Bound-Flipping Ratio Test (`selectDualBFRT`) and sparse `TabRowIdx` tableau rows, Two-Phase Primal Simplex, MIP presolve, cutting planes (`GMI`, Sequentially Lifted Knapsack Covers, Flow/VUB, `c-MIR`), primal heuristics, and Branch-and-Cut search.
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
- **gopt telemetry source**: fingerprint `dfc05e06279c1488` (commit `4118cec7487c3731bdf8ced77d263ec3ffdef785`), `go1.27.1-X:nodwarf5`; the documentation gate rejects telemetry whose fingerprint differs from the checked-in sources
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
| Tier 1: Canonical (`N <= 22`) | 10 | 3.86 | 0.38 | 1.23 | 0.04 | **98.7x** | 164.1x [6.9x-307.4x] | 9.8x | 0.03x |
| Tier 2: Target p90 (`30 <= N <= 76`) | 13 | 14.37 | 11.37 | 25.63 | 2.10 | **6.8x** | 9.2x [1.5x-23.7x] | 5.4x | 0.08x |
| Tier 3: Large Tail (`135 <= N <= 4028`) | 8 | 101.99 | 97.98 | 133.67 | 39.47 | **2.6x** | 2.6x [0.4x-13.0x] | 2.5x | 0.30x |
| Tier 3 non-MAD MILPs | 4 | 120.68 | 118.18 | 214.01 | 23.18 | **5.2x** | 8.0x [1.2x-13.0x] | 5.1x | 0.11x |
| Planted-gap MAD family (zero integrality gap by construction) | 8 | 23.99 | 19.16 | 24.24 | 6.85 | **3.5x** | 3.1x [0.4x-18.7x] | 2.8x | 0.28x |
| Random-weight MAD (negative controls) | 3 | 44.33 | 40.97 | 73.38 | 16.76 | **2.6x** | 1.8x [1.5x-6.9x] | 2.4x | 0.23x |
| **All families except planted-gap MAD** | 23 | 13.42 | 4.59 | 12.39 | 0.68 | **19.7x** | 13.0x [1.2x-307.4x] | 6.7x | 0.06x |
| All corpus families | 31 | 15.59 | 6.64 | 14.73 | 1.24 | **12.6x** | 12.0x [0.4x-307.4x] | 5.4x | 0.08x |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2. MAD Group-Item Allocation Family (`G` Groups $\times$ `I` Items, $T = 6$ Periods)

The Mean Absolute Deviation (`MAD`) allocation family ([`buildMADAllocation`](bench/corpus.go)) assigns $I$ items across $G$ groups over $T = 6$ periods to minimize total absolute deviation from each group's central level $m_g$. In `buildMADAllocation`, only the $G$ designated items $D = \{I-G, \dots, I-1\}$ carry nonzero orthogonal period weights ($\pm 360$), producing a zero integrality gap that `tryRootSubMIP` closes at node 1; unstructured random-weight negative controls (`BuildMADRandom`: `t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`) are included directly in the corpus and detailed in [`docs/benchmarks.md`](docs/benchmarks.md).

<!-- BEGIN GENERATED:MAD_PIPELINE_TABLE -->
| Problem ID | $G$ | $I$ | $T$ | Vars | Rows | Optimal Obj | PyPrep (ms) | GoPrep (ms) | PySolve (ms) | PySolve Net of Spawn (ms) | GoSolve (ms) | PyTotal (ms) | GoTotal (ms) | `Py / gopt` | Parity |
| :--- | --: | ---: | --: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_mad_g2_i4` | 2 | 4 | 6 | 22 | 30 | 2160.0 | 0.291 | 0.133 | 5.23 | 3.13 | 0.17 | 5.45 | 0.29 | **18.7x** | `OK` |
| `t1_mad_rand_g2_i4` | 2 | 4 | 6 | 22 | 32 | 5249.0 | 0.443 | 0.179 | 34.90 | 32.80 | 4.96 | 35.23 | 5.14 | **6.9x** | `OK` |
| `t2_mad_g2_i8` | 2 | 8 | 6 | 30 | 34 | 2160.0 | 0.626 | 0.196 | 4.94 | 2.85 | 0.16 | 5.48 | 0.36 | **15.0x** | `OK` |
| `t2_mad_g3_i9` | 3 | 9 | 6 | 48 | 48 | 4320.0 | 0.961 | 0.323 | 7.57 | 5.48 | 0.53 | 8.33 | 0.84 | **9.9x** | `OK` |
| `t2_mad_g4_i12` | 4 | 12 | 6 | 76 | 64 | 7200.0 | 1.298 | 0.568 | 7.32 | 5.23 | 2.19 | 7.99 | 2.68 | **3.0x** | `OK` |
| `t2_mad_rand_g2_i8` | 2 | 8 | 6 | 30 | 36 | 6696.0 | 0.692 | 0.150 | 13.37 | 11.28 | 7.56 | 13.93 | 7.77 | **1.8x** | `OK` |
| `t2_mad_rand_g3_i9` | 3 | 9 | 6 | 48 | 51 | 7958.0 | 0.599 | 0.328 | 177.04 | 174.95 | 117.55 | 177.47 | 117.89 | **1.5x** | `OK` |
| `t3_mad_g5_i20` | 5 | 20 | 6 | 135 | 85 | 10800.0 | 2.720 | 0.551 | 12.85 | 10.76 | 4.26 | 14.90 | 4.78 | **3.1x** | `OK` |
| `t3_mad_g6_i30` | 6 | 30 | 6 | 222 | 108 | 15120.0 | 2.982 | 1.362 | 28.08 | 25.99 | 13.40 | 30.12 | 14.51 | **2.1x** | `OK` |
| `t3_mad_g10_i100` | 10 | 100 | 6 | 1070 | 230 | 79200.0 | 16.389 | 7.261 | 183.31 | 181.21 | 464.65 | 195.76 | 471.57 | **0.4x** | `OK` |
| `t3_mad_g4_i1000` | 4 | 1000 | 6 | 4028 | 1052 | 187200.0 | 77.944 | 25.815 | 560.97 | 558.88 | 599.24 | 628.55 | 624.69 | **1.0x** | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

Multi-trial total latency dispersion (mean $\pm$ sample standard deviation and two-sided 95% Student-$t$ confidence interval across measured trials after warmup):

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `gopt` Scalar Total (Mean $\pm$ SD [95% CI], ms) | `gopt` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | $5.14 \pm 1.05$ ($\pm 0.70$) | $0.33 \pm 0.08$ ($\pm 0.06$) | $0.32 \pm 0.07$ ($\pm 0.05$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | $28.09 \pm 14.34$ ($\pm 9.63$) | $4.97 \pm 3.15$ ($\pm 2.12$) | $4.11 \pm 2.62$ ($\pm 1.76$) |
| `t2_mad_g2_i8` | $2 \times 8$ | $5.63 \pm 1.12$ ($\pm 0.75$) | $0.47 \pm 0.29$ ($\pm 0.20$) | $0.30 \pm 0.07$ ($\pm 0.05$) |
| `t2_mad_g3_i9` | $3 \times 9$ | $7.66 \pm 1.10$ ($\pm 0.74$) | $0.91 \pm 0.10$ ($\pm 0.07$) | $0.89 \pm 0.06$ ($\pm 0.04$) |
| `t2_mad_g4_i12` | $4 \times 12$ | $8.24 \pm 0.88$ ($\pm 0.59$) | $2.99 \pm 0.89$ ($\pm 0.60$) | $2.22 \pm 0.46$ ($\pm 0.31$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | $34.80 \pm 30.02$ ($\pm 20.17$) | $5.61 \pm 4.64$ ($\pm 3.12$) | $7.39 \pm 6.07$ ($\pm 4.08$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | $175.93 \pm 35.71$ ($\pm 23.99$) | $121.24 \pm 19.00$ ($\pm 12.76$) | $130.50 \pm 22.58$ ($\pm 15.17$) |
| `t3_mad_g5_i20` | $5 \times 20$ | $14.21 \pm 1.40$ ($\pm 1.74$) | $5.21 \pm 1.89$ ($\pm 2.34$) | $5.48 \pm 0.86$ ($\pm 1.07$) |
| `t3_mad_g6_i30` | $6 \times 30$ | $35.69 \pm 23.51$ ($\pm 29.19$) | $16.29 \pm 2.78$ ($\pm 3.45$) | $14.66 \pm 2.87$ ($\pm 3.56$) |
| `t3_mad_g10_i100` | $10 \times 100$ | $231.54 \pm 90.64$ ($\pm 112.53$) | $991.69 \pm 1253.82$ ($\pm 1556.57$) | $1002.61 \pm 1233.68$ ($\pm 1531.57$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | $608.55 \pm 62.25$ ($\pm 77.29$) | $664.29 \pm 88.47$ ($\pm 109.84$) | $647.87 \pm 77.61$ ($\pm 96.35$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### 3. Full 31-Family Benchmark Corpus (`JSON -> Solution`)

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | PyPrep (ms) | GoPrep (ms) | PySolve (ms) | PySolve Net of Spawn (ms) | GoSolve (ms) | PyTotal (ms) | GoTotal (ms) | `Py / gopt` | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | 0.087 | 0.008 | 1.86 | 0.00 | 0.00 | 1.92 | 0.01 | **184.3x** | `OK` |
| `t1_std_lp_max` | 3 | 3 | 0.091 | 0.008 | 1.73 | 0.00 | 0.00 | 1.79 | 0.01 | **184.8x** | `OK` |
| `t1_std_mip` | 3 | 3 | 0.122 | 0.008 | 3.35 | 1.25 | 0.00 | 3.45 | 0.01 | **301.7x** | `OK` |
| `t1_mip_float_obj` | 3 | 3 | 0.129 | 0.008 | 3.21 | 1.12 | 0.00 | 3.30 | 0.01 | **307.4x** | `OK` |
| `t1_whiskas` | 6 | 5 | 0.184 | 0.017 | 2.76 | 0.67 | 0.00 | 2.91 | 0.02 | **136.4x** | `OK` |
| `t1_furniture` | 2 | 3 | 0.087 | 0.008 | 2.92 | 0.83 | 0.01 | 2.98 | 0.02 | **143.8x** | `OK` |
| `t1_knapsack_12` | 12 | 3 | 0.142 | 0.028 | 4.87 | 2.77 | 0.08 | 4.98 | 0.11 | **46.2x** | `OK` |
| `t1_infeasible_mip` | 2 | 1 | 0.095 | 0.008 | 2.21 | 0.11 | 0.00 | 2.27 | 0.01 | **239.7x** | `OK` |
| `t1_mad_g2_i4` | 22 | 30 | 0.291 | 0.133 | 5.23 | 3.13 | 0.17 | 5.45 | 0.29 | **18.7x** | `OK` |
| `t1_mad_rand_g2_i4` | 22 | 32 | 0.443 | 0.179 | 34.90 | 32.80 | 4.96 | 35.23 | 5.14 | **6.9x** | `OK` |
| `t2_mkp_30` | 30 | 6 | 0.377 | 0.141 | 37.88 | 35.79 | 5.16 | 38.23 | 5.43 | **7.0x** | `OK` |
| `t2_mkp_50` | 50 | 8 | 0.585 | 0.222 | 14.52 | 12.43 | 8.70 | 14.82 | 8.92 | **1.7x** | `OK` |
| `t2_cflp_36` | 36 | 11 | 0.420 | 0.125 | 6.08 | 3.98 | 0.33 | 6.47 | 0.46 | **14.1x** | `OK` |
| `t2_cflp_49` | 49 | 13 | 0.491 | 0.102 | 6.66 | 4.57 | 0.46 | 7.09 | 0.59 | **12.0x** | `OK` |
| `t2_gap_45` | 45 | 13 | 0.345 | 0.138 | 10.44 | 8.35 | 1.04 | 10.81 | 1.18 | **9.2x** | `OK` |
| `t2_lotsizing_48` | 48 | 32 | 0.550 | 0.183 | 58.84 | 56.74 | 6.54 | 59.24 | 6.71 | **8.8x** | `OK` |
| `t2_binpack_44` | 44 | 14 | 0.473 | 0.079 | 10.99 | 8.89 | 0.41 | 11.38 | 0.48 | **23.7x** | `OK` |
| `t2_setcover_50` | 50 | 20 | 0.418 | 0.239 | 6.20 | 4.11 | 0.16 | 6.51 | 0.41 | **15.8x** | `OK` |
| `t2_mad_g2_i8` | 30 | 34 | 0.626 | 0.196 | 4.94 | 2.85 | 0.16 | 5.48 | 0.36 | **15.0x** | `OK` |
| `t2_mad_g3_i9` | 48 | 48 | 0.961 | 0.323 | 7.57 | 5.48 | 0.53 | 8.33 | 0.84 | **9.9x** | `OK` |
| `t2_mad_g4_i12` | 76 | 64 | 1.298 | 0.568 | 7.32 | 5.23 | 2.19 | 7.99 | 2.68 | **3.0x** | `OK` |
| `t2_mad_rand_g2_i8` | 30 | 36 | 0.692 | 0.150 | 13.37 | 11.28 | 7.56 | 13.93 | 7.77 | **1.8x** | `OK` |
| `t2_mad_rand_g3_i9` | 48 | 51 | 0.599 | 0.328 | 177.04 | 174.95 | 117.55 | 177.47 | 117.89 | **1.5x** | `OK` |
| `t3_mkp_200` | 200 | 8 | 1.071 | 0.497 | 51.71 | 49.62 | 43.55 | 52.38 | 43.84 | **1.2x** | `OK` |
| `t3_cflp_300` | 300 | 34 | 1.362 | 0.787 | 259.76 | 257.67 | 21.03 | 261.58 | 21.81 | **12.0x** | `OK` |
| `t3_lotsizing_504` | 504 | 336 | 2.514 | 0.844 | 153.28 | 151.18 | 38.44 | 155.39 | 39.30 | **4.0x** | `OK` |
| `t3_sched_1000` | 1000 | 100 | 6.842 | 1.684 | 95.76 | 93.67 | 6.22 | 99.61 | 7.68 | **13.0x** | `OK` |
| `t3_mad_g5_i20` | 135 | 85 | 2.720 | 0.551 | 12.85 | 10.76 | 4.26 | 14.90 | 4.78 | **3.1x** | `OK` |
| `t3_mad_g6_i30` | 222 | 108 | 2.982 | 1.362 | 28.08 | 25.99 | 13.40 | 30.12 | 14.51 | **2.1x** | `OK` |
| `t3_mad_g10_i100` | 1070 | 230 | 16.389 | 7.261 | 183.31 | 181.21 | 464.65 | 195.76 | 471.57 | **0.4x** | `OK` |
| `t3_mad_g4_i1000` | 4028 | 1052 | 77.944 | 25.815 | 560.97 | 558.88 | 599.24 | 628.55 | 624.69 | **1.0x** | `OK` |
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
