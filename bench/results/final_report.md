# `goblin` vs. Python (`PuLP + COIN-OR CBC`) Benchmark Report

## 1. Experimental Setup and Provenance

```bash
go run ./cmd/goblin-bench -check-docs
```

- **CPU**: `13th Gen Intel(R) Core(TM) i9-13900K` (x86_64, 30 logical CPUs exposed by `lscpu`, AVX2 + FMA).
- **Default pure-Go backend**: default `go build` (`BackendGo`, scalar kernels). All headline `goblin (Go)` timings use that build.
- **Experimental SIMD build**: `GOEXPERIMENT=simd`, which dispatches AVX2 kernels for slices of length 32 or more; see section 4.
- **Optional C++ backend**: `gb.WithCBC()` (`BackendCBC`, built with `-tags cbc`), driving in-process COIN-OR CBC/CLP (`c1c171ec`) through `cgo`.
- **Python baseline**: Python + `PuLP` + `COIN-OR CBC` (`c1c171ec`, `threads=1`, `ratioGap=1e-6`, `allowableGap=1e-6`) subprocess. Its solve time includes MPS writing, process spawn, and `.sol` parsing.
- **Synthetic corpus and public validation**: 31 synthetic LP and MILP families across 3 tiers (`testdata/corpus/*.json` and `testdata/corpus/trials/*.jsonl`; 11 distinct randomized trial instances per family for Tiers 1-2 and 5 for Tier 3, 293 instances in total), measured in warm state, complemented by 20 held-out public Netlib LP and MIPLIB 3.0 MILP instances solved to optimality plus a 25-node dual-bound check on `coinor_capplan1` with cuts and heuristics disabled (`WithCuts(false)`, `WithHeuristics(false)`, `WithMaxNodes(25)`, `public_benchmarks_test.go`).

<!-- BEGIN GENERATED:PROVENANCE -->
- **Unified telemetry source**: fingerprint `c27969c2e5705bb2` (commit `67d5e540ce174d6b03c5389d1275179af0ebcdd6`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.62 ms median on a 1-variable MILP and 6.41 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

## 2. Aggregate Workload Tier Summary

Tier 1 micro-models are dominated by Python subprocess spawn overhead, and the planted-gap MAD family is constructed so that root heuristics close the gap without branching; the bold row isolates solver-dominated Tier 2 and Tier 3 non-planted MILPs across paired trial instances.

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `Py / Go` GeoMean [Clustered 95% CI] | Pooled Trial `Py / Go` | Equal-Family `Go / CBC` | `Go` Trial (Med) Sum (ms) | `CBC` Trial (Med) Sum (ms) | `Py` Trial (Med) Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **281.29x** [139.64x-566.64x] | 281.29x | 0.06x | 9.4 (0.9) | 150.8 (14.0) | 1077.3 (100.6) | 304.42x [40.78x-2969.91x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **16.47x** [8.54x-31.74x] | 16.47x | 0.21x | 833.1 (49.7) | 1718.5 (153.9) | 4410.3 (427.9) | 22.82x [0.11x-151.93x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **9.41x** [4.55x-19.47x] | 9.41x | 0.27x | 937.3 (168.8) | 2551.8 (515.8) | 6338.8 (1089.9) | 9.69x [1.05x-63.74x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **10.27x** [5.65x-18.66x] | 11.08x | 0.26x | 1447.7 (172.1) | 2928.9 (383.1) | 6920.0 (1005.9) | 11.19x [0.11x-151.93x] |
| Tier 3 non-MAD MILPs | 4 (20) | **6.83x** [1.35x-34.53x] | 6.83x | 0.32x | 622.9 (123.1) | 1268.4 (234.2) | 2871.0 (610.8) | 5.06x [1.29x-63.74x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **28.22x** [11.97x-66.56x] | 37.78x | 0.16x | 323.4 (46.5) | 1354.0 (287.7) | 3932.6 (521.3) | 38.02x [1.05x-172.00x] |
| Random-weight MAD (negative controls) | 3 (33) | **15.70x** [0.79x-310.01x] | 15.70x | 0.20x | 325.8 (22.5) | 960.7 (97.0) | 2292.4 (252.3) | 15.73x [1.34x-110.57x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **38.59x** [16.52x-90.19x] | 46.28x | 0.15x | 1456.5 (172.9) | 3067.1 (396.0) | 7893.7 (1097.0) | 42.69x [0.11x-2969.91x] |
| All 31 synthetic corpus families | 31 (293) | **35.60x** [18.68x-67.84x] | 44.27x | 0.15x | 1779.8 (219.4) | 4421.2 (683.7) | 11826.3 (1618.3) | 41.82x [0.11x-2969.91x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2.1 Pure-Go vs. In-Process C++ CBC Aggregate Summary

Comparing `BackendGo` directly against in-process `BackendCBC` removes Python MPS formatting and subprocess spawn overhead across the same five workload subsets:

<!-- BEGIN GENERATED:CBC_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `BackendCBC / BackendGo` Total [Clustered 95% CI] | Equal-Family `BackendCBC / BackendGo` Solve [Clustered 95% CI] | `BackendGo` Trial (Med) Sum (ms) | `BackendCBC` Trial (Med) Sum (ms) | Per-Instance `CBC / Go` Total Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro-models (`N <= 26`) | 10 (110) | **15.60x** [6.13x-39.71x] | **33.07x** [10.72x-102.07x] | 9.4 (0.9) | 150.8 (14.0) | 18.51x [0.80x-138.93x] |
| Tier 2: Medium MILPs (`30 <= N <= 84`) | 13 (143) | **4.75x** [3.00x-7.53x] | **5.25x** [3.16x-8.72x] | 833.1 (49.7) | 1718.5 (153.9) | 5.60x [0.07x-43.62x] |
| Tier 3: Large MILPs (`145 <= N <= 4036`) | 8 (40) | **3.66x** [2.08x-6.44x] | **4.00x** [2.17x-7.39x] | 937.3 (168.8) | 2551.8 (515.8) | 3.57x [0.40x-14.62x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **3.87x** [2.42x-6.17x] | **4.05x** [2.47x-6.66x] | 1447.7 (172.1) | 2928.9 (383.1) | 4.48x [0.07x-43.62x] |
| All 31 synthetic families | 31 (293) | **6.52x** [4.33x-9.80x] | **8.87x** [5.27x-14.93x] | 1779.8 (219.4) | 4421.2 (683.7) | 6.95x [0.07x-138.93x] |
<!-- END GENERATED:CBC_SUMMARY_TABLE -->

## 3. End-to-End Pipeline Comparison (`JSON -> Solution`)

Pipeline stages measured per instance (median across randomized trials after warmup, plus paired per-trial speedup range):
1. **Prep (`t_parse + t_build` vs. `t_parse + t_compile`)**: JSON unmarshaling and PuLP model construction in Python (MPS writing `t_format` is recorded separately and timed inside `t_solve`) vs. streaming `FromPuLPJSONBytes` plus `Model.Compile()` matrix assembly in Go.
2. **Solve (`t_solve`)**: CBC subprocess execution and `.sol` parsing (Python) vs. in-memory `gb.SolveCompiled` (Go).
3. **Total (`t_total`)**: Complete end-to-end latency from raw JSON bytes to verified primal solution.

The Parity column is computed by `bench.CheckParity`: canonical and per-trial statuses and objectives across all 293 instances for `scalar`, `simd`, and `cbc` must match CBC within a relative tolerance of 1e-5, and `ValidateTelemetryAgainstCorpus` rejects the telemetry otherwise.

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | **423.8x** [121.4x-663.7x] | 0.01 | 6.46 | 0.01 | 6.41 | 0.006 | 0.050 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **398.9x** [178.9x-913.4x] | 0.02 | 6.68 | 0.01 | 6.62 | 0.008 | 0.058 | `OK` |
| `t1_std_mip` | 3 | 3 | **391.8x** [223.7x-664.2x] | 0.02 | 7.67 | 0.01 | 7.61 | 0.009 | 0.065 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **364.4x** [189.6x-539.1x] | 0.02 | 7.34 | 0.01 | 7.28 | 0.008 | 0.055 | `OK` |
| `t1_whiskas` | 6 | 5 | **293.8x** [74.2x-648.5x] | 0.02 | 6.33 | 0.01 | 6.24 | 0.009 | 0.073 | `OK` |
| `t1_furniture` | 2 | 3 | **229.7x** [64.2x-511.6x] | 0.03 | 7.96 | 0.02 | 7.89 | 0.005 | 0.052 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **95.4x** [48.7x-304.7x] | 0.12 | 8.75 | 0.11 | 8.66 | 0.008 | 0.086 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **2202.3x** [1238.6x-2969.9x] | 0.00 | 6.33 | 0.00 | 6.29 | 0.002 | 0.044 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **145.9x** [109.2x-172.0x] | 0.06 | 9.49 | 0.04 | 9.25 | 0.022 | 0.225 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **62.1x** [40.8x-110.6x] | 0.58 | 33.56 | 0.51 | 33.31 | 0.027 | 0.235 | `OK` |
| `t2_mkp_30` | 30 | 6 | **10.2x** [5.1x-15.2x] | 2.42 | 28.53 | 2.40 | 28.34 | 0.019 | 0.183 | `OK` |
| `t2_mkp_50` | 50 | 8 | **4.9x** [2.2x-22.8x] | 5.20 | 17.17 | 5.17 | 16.91 | 0.031 | 0.275 | `OK` |
| `t2_cflp_36` | 36 | 11 | **51.6x** [16.7x-151.9x] | 0.18 | 9.50 | 0.16 | 9.34 | 0.020 | 0.166 | `OK` |
| `t2_cflp_49` | 49 | 13 | **29.9x** [13.2x-75.4x] | 0.39 | 11.26 | 0.36 | 11.03 | 0.033 | 0.228 | `OK` |
| `t2_gap_45` | 45 | 13 | **21.3x** [4.3x-76.6x] | 0.95 | 15.31 | 0.89 | 15.09 | 0.022 | 0.210 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **10.9x** [4.8x-27.0x] | 4.37 | 69.86 | 4.34 | 69.63 | 0.034 | 0.258 | `OK` |
| `t2_binpack_44` | 44 | 14 | **1.6x** [0.1x-22.9x] | 13.27 | 13.60 | 13.24 | 13.40 | 0.028 | 0.210 | `OK` |
| `t2_setcover_50` | 50 | 20 | **37.4x** [6.4x-129.1x] | 0.31 | 11.12 | 0.27 | 10.80 | 0.035 | 0.318 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **59.4x** [50.0x-65.9x] | 0.16 | 9.80 | 0.13 | 9.48 | 0.033 | 0.307 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **49.1x** [31.2x-63.8x] | 0.20 | 10.37 | 0.13 | 9.91 | 0.058 | 0.434 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **33.4x** [31.1x-35.7x] | 0.38 | 12.63 | 0.30 | 12.00 | 0.074 | 0.640 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.2x** [3.4x-24.7x] | 3.40 | 48.79 | 3.32 | 48.44 | 0.049 | 0.320 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **6.8x** [1.3x-27.9x] | 18.50 | 169.97 | 18.43 | 169.51 | 0.069 | 0.466 | `OK` |
| `t3_mkp_200` | 200 | 8 | **3.9x** [3.2x-5.1x] | 23.41 | 116.45 | 23.31 | 115.78 | 0.087 | 0.667 | `OK` |
| `t3_cflp_300` | 300 | 34 | **25.0x** [7.5x-63.7x] | 9.23 | 252.07 | 9.11 | 251.11 | 0.118 | 0.934 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **2.5x** [1.3x-4.4x] | 82.16 | 160.74 | 81.94 | 158.80 | 0.234 | 1.937 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **9.0x** [3.9x-13.6x] | 8.31 | 81.56 | 8.01 | 78.52 | 0.315 | 2.847 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **28.0x** [23.9x-32.6x] | 0.66 | 18.54 | 0.49 | 17.39 | 0.155 | 1.102 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **17.0x** [10.6x-22.7x] | 1.06 | 19.01 | 0.85 | 17.19 | 0.215 | 1.804 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.9x** [1.0x-15.3x] | 8.58 | 63.51 | 7.53 | 55.35 | 1.100 | 8.473 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **10.1x** [4.5x-23.8x] | 35.37 | 377.98 | 30.40 | 341.73 | 4.366 | 35.490 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

## 4. Scalar vs. `GOEXPERIMENT=simd` Build

Ratios are geometric means of per-trial `scalar / SIMD` solve times over the same instances, with 95% t-intervals on the log ratios. "Reachable" is a necessary condition only: the AVX2 `Dot64`/`Axpy64`/`Scale64` paths need a slice of length at least 32, which requires `n + m >= 32`.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 4 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 4 faster, 4 slower, 15 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 1.10x [0.93x-1.29x] | 0.005 | 0.004 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.61x [1.12x-2.32x] | 0.006 | 0.004 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.39x [1.13x-1.70x] | 0.008 | 0.006 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.60x [1.21x-2.11x] | 0.007 | 0.005 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 1.09x [0.93x-1.29x] | 0.008 | 0.007 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 0.94x [0.79x-1.11x] | 0.021 | 0.020 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.94x [0.90x-0.99x] | 0.107 | 0.108 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 1.35x [0.82x-2.24x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | SIMD slower | 0.68x [0.56x-0.81x] | 0.041 | 0.053 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | SIMD slower | 0.88x [0.85x-0.91x] | 0.511 | 0.563 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 0.99x [0.97x-1.02x] | 2.398 | 2.519 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 1.02x [0.94x-1.10x] | 5.170 | 5.136 | 11 |
| `t2_cflp_36` | 47 | possible | SIMD slower | 0.94x [0.91x-0.98x] | 0.156 | 0.161 | 11 |
| `t2_cflp_49` | 62 | possible | SIMD faster | 1.24x [1.06x-1.45x] | 0.359 | 0.313 | 11 |
| `t2_gap_45` | 58 | possible | SIMD slower | 0.90x [0.82x-0.99x] | 0.893 | 0.805 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 1.01x [0.97x-1.05x] | 4.338 | 4.432 | 11 |
| `t2_binpack_44` | 58 | possible | no detectable effect | 0.98x [0.96x-1.00x] | 13.242 | 13.737 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 0.98x [0.90x-1.07x] | 0.266 | 0.272 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 1.00x [0.98x-1.03x] | 0.132 | 0.133 | 11 |
| `t2_mad_g3_i9` | 108 | possible | SIMD faster | 1.09x [1.01x-1.17x] | 0.131 | 0.124 | 11 |
| `t2_mad_g4_i12` | 156 | possible | no detectable effect | 0.95x [0.86x-1.04x] | 0.301 | 0.295 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.02x [0.95x-1.10x] | 3.325 | 3.053 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 1.01x [0.99x-1.04x] | 18.426 | 17.487 | 11 |
| `t3_mkp_200` | 208 | possible | SIMD faster | 1.04x [1.01x-1.08x] | 23.310 | 22.324 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.08x [0.97x-1.21x] | 9.108 | 9.106 | 5 |
| `t3_lotsizing_504` | 840 | possible | no detectable effect | 1.02x [0.97x-1.07x] | 81.938 | 79.241 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 0.98x [0.93x-1.04x] | 8.010 | 8.513 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 0.98x [0.93x-1.03x] | 0.495 | 0.499 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 0.87x [0.62x-1.22x] | 0.854 | 0.826 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 1.01x [0.92x-1.10x] | 7.533 | 7.411 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | SIMD faster | 1.09x [1.01x-1.18x] | 30.399 | 28.133 | 5 |
<!-- END GENERATED:SIMD_EFFECT_TABLE -->

## 5. Solver-Core Checkpoint Progression (default pure-Go backend)

Progression across checkpoints on pre-compiled `CompiledProblem` instances:
- **CP2/3 (Root LP)**: Revised Dual and Two-Phase Primal Simplex.
- **CP4 (Base B&B)**: Branch-and-Bound with pseudocost/strong branching (`Presolve=off, Cuts=off, Heuristics=off`).
- **CP5 (+Presolve)**: Savelsbergh bound tightening, GCD reduction, and node propagation (`Presolve=on, Cuts=off, Heuristics=off`).
- **CP6 (+Cuts)**: GMI, lifted 0-1 knapsack covers, flow/VUB and $(l, S)$ cuts, single-row and multi-row MIR cuts, and globally valid in-tree cuts (`Presolve=on, Cuts=on, Heuristics=off`).
- **CP7 (Full `goblin`)**: Full Branch-and-Cut with Propagated Rounding, Root Sub-MIP, and Fractional Diving (`Presolve=on, Cuts=on, Heuristics=on`).

<!-- BEGIN GENERATED:FULL_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_std_lp` | **0.01** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.01 (1) | 0.004 | 54.00 | 54.00 |
| `t1_std_lp_max` | **0.01** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 80.00 | 80.00 |
| `t1_std_mip` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.003 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.003 | 64.95 | 58.90 |
| `t1_whiskas` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.00 (1) | 0.004 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.02 (1) | 0.003 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.11** (1 / 8) | 0.12 (2 / 8) | 0.17 (18) | 0.26 (35) | 0.009 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.04** (1 / 0) | 0.04 (1 / 0) | 0.05 (1) | 0.55 (4) | 0.093 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.51** (2 / 0) | 0.61 (12 / 0) | 0.60 (12) | 1.19 (109) | 0.115 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.40** (5 / 9) | 3.49 (40 / 9) | 1.21 (51) | 1.06 (61) | 0.028 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **5.17** (1 / 4) | 15.46 (2 / 4) | 6.01 (118) | 5.68 (111) | 0.047 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.16** (1 / 2) | 0.24 (3 / 2) | 0.18 (7) | 0.19 (7) | 0.027 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.36** (1 / 7) | 0.63 (3 / 7) | 0.32 (9) | 0.30 (9) | 0.035 | 909.02 | 827.30 |
| `t2_gap_45` | **0.89** (1 / 7) | 0.44 (1 / 7) | 1.85 (41) | 2.10 (60) | 0.027 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **4.34** (2 / 37) | 8.57 (25 / 37) | 3.61 (162) | 3.65 (199) | 0.062 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **13.24** (274 / 12) | 17.11 (298 / 12) | 9.36 (772) | 30.91 (1941) | 0.028 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.27** (1 / 6) | 0.42 (2 / 6) | 0.74 (6) | 0.69 (6) | 0.032 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.13** (1 / 0) | 0.84 (8 / 0) | 0.77 (8) | 0.76 (9) | 0.109 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.13** (1 / 0) | 0.14 (2 / 0) | 0.13 (2) | 2.82 (13) | 0.268 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.30** (1 / 0) | 3.65 (22 / 0) | 3.30 (22) | 5.74 (23) | 0.377 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.32** (43 / 0) | 3.14 (112 / 0) | 3.46 (112) | 3.90 (45) | 0.157 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **18.43** (196 / 0) | 34.93 (188 / 0) | 33.22 (188) | 97.15 (539) | 0.297 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **23.31** (250 / 14) | 110.88 (1135 / 16) | 229.75 (5000, `NodeLimit`) | 212.85 (5000, `NodeLimit`) | 0.129 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **9.11** (1 / 28) | 19.82 (54 / 34) | 9.51 (116) | 9.76 (118) | 0.185 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **81.94** (2 / 206) | 120.42 (376 / 206) | 250.02 (2793, `TimeLimit`) | 250.04 (2901, `TimeLimit`) | 0.747 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **8.01** (31 / 55) | 27.81 (87 / 53) | 250.13 (1697, `TimeLimit`) | 250.02 (1698, `TimeLimit`) | 0.230 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.49** (1 / 0) | 3.20 (17 / 0) | 2.90 (17) | 0.51 (1) | 0.657 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.85** (1 / 0) | 9.86 (58 / 0) | 8.51 (58) | 14.26 (74) | 1.638 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.53** (2 / 0) | 22.23 (25 / 0) | 20.12 (25) | 250.42 (145, `TimeLimit`) | 2.694 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **30.40** (1 / 0) | 32.91 (2 / 0) | 32.74 (2) | 250.60 (6, `TimeLimit`) | 58.664 | 208000.00 | 208000.00 |
<!-- END GENERATED:FULL_CHECKPOINT_TABLE -->

## 6. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus (`testdata/assign_corpus/manifest.json`; 11 randomized trials per family for Tiers 1-2 and 5 for Tier 3, 200 instances in total) compares pure-Go `assign.LinearSumFlat` against SciPy's C++ `rectangular_lsap` extension in warm state on bit-identical `float64` cost matrices, timing `linear_sum_assignment` without post-solve NumPy array gather overhead.

### 6.1 Aggregate LSAP Tier Summary

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `SciPy Solve / goblin` GeoMean [Clustered 95% CI] | Pooled Trial `SciPy Solve / goblin` | Equal-Family `SciPy Total / goblin` | `goblin` Solve Trial (Med) Sum (ms) | SciPy Solve Trial (Med) Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.56x** [1.29x-1.87x] | 1.56x | 5.01x | 0.210 (0.0193) | 0.319 (0.0292) | 1.59x [1.12x-2.72x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **2.91x** [1.80x-4.71x] | 2.91x | 2.75x | 9.446 (0.834) | 33.060 (2.987) | 2.58x [0.96x-16.11x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.84x** [1.84x-4.37x] | 2.84x | 3.61x | 222.911 (45.299) | 958.728 (192.780) | 2.53x [1.46x-6.04x] |
| Square families ($m = n$) | 16 (146) | **2.73x** [2.00x-3.72x] | 2.67x | 3.69x | 227.702 (45.297) | 981.076 (193.789) | 2.24x [1.12x-16.11x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.79x** [1.32x-2.44x] | 1.68x | 3.15x | 4.865 (0.854) | 11.030 (2.007) | 1.76x [0.96x-2.69x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.26x** [1.13x-4.55x] | 2.39x | 2.75x | 36.274 (6.414) | 65.747 (11.136) | 1.79x [1.12x-16.11x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.43x** [1.91x-3.10x] | 2.36x | 3.53x | 232.567 (46.151) | 992.106 (195.796) | 2.08x [0.96x-16.11x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 6.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.91x** | 0.0005 | 0.0009 | 0.0006 | 0.0117 | $0.0005 \pm 0.0001$ | $0.0009 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.59x** | 0.0015 | 0.0025 | 0.0018 | 0.0107 | $0.0016 \pm 0.0004$ | $0.0025 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.77x** | 0.0035 | 0.0064 | 0.0044 | 0.0157 | $0.0035 \pm 0.0007$ | $0.0063 \pm 0.0018$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.27x** | 0.0035 | 0.0042 | 0.0044 | 0.0149 | $0.0033 \pm 0.0006$ | $0.0042 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.25x** | 0.0035 | 0.0042 | 0.0045 | 0.0151 | $0.0036 \pm 0.0007$ | $0.0044 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.66x** | 0.0068 | 0.0109 | 0.0108 | 0.0327 | $0.0067 \pm 0.0017$ | $0.0108 \pm 0.0013$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.75x** | 0.0297 | 0.0765 | 0.0428 | 0.104 | $0.0275 \pm 0.0070$ | $0.0741 \pm 0.0118$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.14x** | 0.0566 | 0.175 | 0.0885 | 0.228 | $0.0571 \pm 0.0071$ | $0.180 \pm 0.0289$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **1.95x** | 0.0491 | 0.0935 | 0.0891 | 0.142 | $0.0509 \pm 0.0113$ | $0.0981 \pm 0.0145$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.52x** | 0.148 | 0.219 | 0.174 | 0.255 | $0.156 \pm 0.0493$ | $0.231 \pm 0.0418$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **12.46x** | 0.0608 | 0.700 | 0.0805 | 0.780 | $0.0569 \pm 0.0103$ | $0.698 \pm 0.0277$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.95x** | 0.0367 | 0.0742 | 0.0681 | 0.154 | $0.0396 \pm 0.0136$ | $0.0749 \pm 0.0087$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **1.88x** | 0.0410 | 0.0813 | 0.0685 | 0.158 | $0.0448 \pm 0.0155$ | $0.0816 \pm 0.0093$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.38x** | 0.0805 | 0.274 | 0.106 | 0.350 | $0.0844 \pm 0.0142$ | $0.284 \pm 0.0346$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.77x** | 0.332 | 1.294 | 0.450 | 1.744 | $0.342 \pm 0.0501$ | $1.284 \pm 0.139$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.82x** | 1.331 | 5.157 | 1.851 | 7.686 | $1.401 \pm 0.261$ | $5.295 \pm 0.375$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.68x** | 0.888 | 1.480 | 1.446 | 3.951 | $0.873 \pm 0.0522$ | $1.468 \pm 0.0673$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.66x** | 5.258 | 8.626 | 5.492 | 9.584 | $5.780 \pm 2.211$ | $9.383 \pm 2.660$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.50x** | 0.350 | 0.870 | 0.732 | 4.005 | $0.348 \pm 0.0248$ | $0.872 \pm 0.0571$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.29x** | 0.419 | 0.973 | 0.837 | 3.768 | $0.424 \pm 0.0156$ | $0.971 \pm 0.0409$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.96x** | 5.791 | 28.302 | 8.000 | 36.832 | $5.810 \pm 0.224$ | $28.846 \pm 2.161$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.87x** | 31.262 | 147.372 | 38.296 | 190.169 | $29.946 \pm 4.121$ | $144.910 \pm 9.670$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 6.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSkip64`, `InitReducedRow64`, and `Min2ReducedRow64`)

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` with no `>= 8x8` tall transpose and either fused square `Minimize` non-sparse path or `rows * cols < 32`, so no AVX2 kernel runs): 2 of 3 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32`, tall `rows > cols` with `>= 8x8`, or fallback `AllAbsLE64`/`ValidateAndNegate64` at `rows * cols >= 32`): 16 faster, 0 slower, 3 with no detectable effect, out of 19.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.61x [0.39x-0.96x] | 0.0005 | 0.0006 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source | 0.58x [0.33x-1.02x] | 0.0015 | 0.0018 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.89x [0.79x-0.99x] | 0.0035 | 0.0039 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | no detectable effect | 0.94x [0.80x-1.10x] | 0.0035 | 0.0035 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | no detectable effect | 0.94x [0.81x-1.10x] | 0.0035 | 0.0037 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | no detectable effect | 0.98x [0.91x-1.05x] | 0.0068 | 0.0066 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.27x [1.24x-1.30x] | 0.0297 | 0.0238 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.38x [1.25x-1.51x] | 0.0566 | 0.0419 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.62x [1.45x-1.80x] | 0.0491 | 0.0289 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.79x [1.72x-1.86x] | 0.148 | 0.0817 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.19x [1.12x-1.27x] | 0.0608 | 0.0486 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.27x [1.01x-1.60x] | 0.0367 | 0.0288 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.36x [1.16x-1.59x] | 0.0410 | 0.0323 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.32x [1.17x-1.49x] | 0.0805 | 0.0591 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.56x [1.43x-1.71x] | 0.332 | 0.210 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.63x [1.06x-2.53x] | 1.331 | 0.766 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.34x [2.19x-2.49x] | 0.888 | 0.373 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.55x [2.50x-2.61x] | 5.258 | 2.000 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.73x [1.65x-1.83x] | 0.350 | 0.208 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.61x [1.50x-1.73x] | 0.419 | 0.276 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.63x [1.48x-1.80x] | 5.791 | 3.585 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.48x [1.36x-1.60x] | 31.262 | 19.534 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

## 7. Production Binary Footprint and Linker Dead-Code Elimination

Standalone `CGO_ENABLED=0` binaries compiled across the four canonical `goblin/assign` and `goblin` build profiles (`bench/results/goblin_binary_size.json`), verifying that unreferenced `WithCertified`, `WithVIPRTrace`, `encoding/json`, and (in `goblin/assign`) `math/big` symbols are eliminated by the Go linker:

<!-- BEGIN GENERATED:BINARY_SIZE_TABLE -->
| Profile | Default Raw / Stripped (KiB) | `GOEXPERIMENT=simd` Raw / Stripped (KiB) | Stripped Delta vs. Default | `goblin` Text (KiB) | `math/big` Syms | `encoding/json` Syms | `VIPR` Syms | `Certif` Syms |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `assign` default (`LinearSum` + `SolveInto`) | 2822.8 / **1768.2** | 2858.1 / **1832.2** | baseline | 118.7 | `0` | `0` | `0` | `0` |
| `assign` certified (`+ assign.WithCertified(true)`) | 2942.9 / **1844.2** | 2980.7 / **1916.2** | +76.0 KiB (+4.3%) | 177.2 | `0` | `0` | `0` | `17` |
| `goblin` default (`Model.Solve`, `DefaultOptions`) | 5500.9 / **3508.2** | 5385.4 / **3520.2** | baseline | 988.0 | `130` | `0` | `0` | `0` |
| `goblin` certified + VIPR (`+ WithCertified + WithVIPRTrace`) | 6715.7 / **4332.2** | 6586.9 / **4348.2** | +824.0 KiB (+23.5%) | 1455.8 | `155` | `0` | `219` | `14` |
<!-- END GENERATED:BINARY_SIZE_TABLE -->
