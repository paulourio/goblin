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
- **Unified telemetry source**: fingerprint `8c76940984c7433e` (commit `7900285032b0a01380777bde12851e1d6d5cb5ba`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.09 ms median on a 1-variable MILP and 5.90 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

## 2. Aggregate Workload Tier Summary

Tier 1 micro-models are dominated by Python subprocess spawn overhead, and the planted-gap MAD family is constructed so that root heuristics close the gap without branching; the bold row isolates solver-dominated Tier 2 and Tier 3 non-planted MILPs across paired trial instances.

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `Py / Go` GeoMean [Clustered 95% CI] | Pooled Trial `Py / Go` | Equal-Family `Go / CBC` | `Go` Trial (Med) Sum (ms) | `CBC` Trial (Med) Sum (ms) | `Py` Trial (Med) Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **387.88x** [168.21x-894.43x] | 387.88x | 0.04x | 8.4 (0.7) | 144.2 (13.3) | 985.1 (92.9) | 590.94x [30.60x-2598.92x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **18.54x** [11.47x-29.96x] | 18.54x | 0.18x | 522.6 (35.5) | 1652.5 (148.6) | 4169.2 (398.3) | 22.23x [1.33x-143.31x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **9.91x** [4.63x-21.18x] | 9.91x | 0.26x | 851.5 (160.3) | 2412.1 (488.9) | 6015.5 (1050.7) | 9.35x [1.05x-72.32x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **12.30x** [7.33x-20.61x] | 13.47x | 0.21x | 1064.1 (151.3) | 2789.9 (363.1) | 6623.4 (970.4) | 12.07x [1.33x-143.31x] |
| Tier 3 non-MAD MILPs | 4 (20) | **7.49x** [1.29x-43.62x] | 7.49x | 0.30x | 550.7 (116.6) | 1195.2 (219.5) | 2774.5 (600.8) | 5.27x [1.43x-72.32x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **26.09x** [11.45x-59.46x] | 33.78x | 0.16x | 310.6 (44.6) | 1287.6 (275.5) | 3654.4 (487.2) | 35.30x [1.05x-153.76x] |
| Random-weight MAD (negative controls) | 3 (33) | **16.06x** [0.80x-323.22x] | 16.06x | 0.19x | 302.6 (20.7) | 913.7 (92.7) | 2179.3 (236.1) | 14.83x [1.33x-111.22x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **49.65x** [20.59x-119.76x] | 60.54x | 0.11x | 1071.8 (152.0) | 2921.1 (375.4) | 7515.4 (1054.7) | 42.89x [1.33x-2598.92x] |
| All 31 synthetic corpus families | 31 (293) | **42.06x** [21.49x-82.28x] | 53.30x | 0.12x | 1382.5 (196.6) | 4208.7 (650.8) | 11169.8 (1541.9) | 38.45x [1.05x-2598.92x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2.1 Pure-Go vs. In-Process C++ CBC Aggregate Summary

Comparing `BackendGo` directly against in-process `BackendCBC` removes Python MPS formatting and subprocess spawn overhead across the same five workload subsets:

<!-- BEGIN GENERATED:CBC_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `BackendCBC / BackendGo` Total [Clustered 95% CI] | Equal-Family `BackendCBC / BackendGo` Solve [Clustered 95% CI] | `BackendGo` Trial (Med) Sum (ms) | `BackendCBC` Trial (Med) Sum (ms) | Per-Instance `CBC / Go` Total Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro-models (`N <= 26`) | 10 (110) | **22.99x** [9.67x-54.69x] | **44.22x** [14.23x-137.43x] | 8.4 (0.7) | 144.2 (13.3) | 18.30x [3.66x-152.34x] |
| Tier 2: Medium MILPs (`30 <= N <= 84`) | 13 (143) | **5.68x** [4.03x-8.01x] | **6.39x** [4.31x-9.47x] | 522.6 (35.5) | 1652.5 (148.6) | 5.73x [0.55x-45.59x] |
| Tier 3: Large MILPs (`145 <= N <= 4036`) | 8 (40) | **3.88x** [2.11x-7.11x] | **4.23x** [2.22x-8.07x] | 851.5 (160.3) | 2412.1 (488.9) | 3.78x [0.37x-14.31x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **4.74x** [3.15x-7.14x] | **4.98x** [3.21x-7.75x] | 1064.1 (151.3) | 2789.9 (363.1) | 5.39x [0.55x-45.59x] |
| All 31 synthetic families | 31 (293) | **8.08x** [5.36x-12.19x] | **10.72x** [6.32x-18.19x] | 1382.5 (196.6) | 4208.7 (650.8) | 7.28x [0.37x-152.34x] |
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
| `t1_std_lp` | 3 | 3 | **778.5x** [471.5x-1091.8x] | 0.01 | 5.91 | 0.00 | 5.86 | 0.004 | 0.044 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **891.1x** [627.3x-1149.3x] | 0.01 | 5.85 | 0.00 | 5.81 | 0.003 | 0.045 | `OK` |
| `t1_std_mip` | 3 | 3 | **746.7x** [505.9x-895.3x] | 0.01 | 6.66 | 0.00 | 6.61 | 0.003 | 0.044 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **878.7x** [631.6x-1066.9x] | 0.01 | 6.89 | 0.00 | 6.84 | 0.003 | 0.048 | `OK` |
| `t1_whiskas` | 6 | 5 | **500.1x** [397.9x-626.1x] | 0.01 | 5.85 | 0.00 | 5.78 | 0.005 | 0.065 | `OK` |
| `t1_furniture` | 2 | 3 | **273.9x** [72.6x-963.1x] | 0.02 | 7.06 | 0.01 | 7.02 | 0.003 | 0.041 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **71.3x** [30.6x-240.8x] | 0.14 | 8.29 | 0.12 | 8.22 | 0.014 | 0.074 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **1968.0x** [878.0x-2598.9x] | 0.00 | 5.68 | 0.00 | 5.65 | 0.002 | 0.035 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **137.0x** [109.1x-153.8x] | 0.06 | 8.59 | 0.04 | 8.40 | 0.022 | 0.191 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **64.4x** [42.7x-111.2x] | 0.49 | 32.18 | 0.46 | 31.94 | 0.025 | 0.234 | `OK` |
| `t2_mkp_30` | 30 | 6 | **10.3x** [6.7x-15.5x] | 2.29 | 29.08 | 2.26 | 28.90 | 0.021 | 0.176 | `OK` |
| `t2_mkp_50` | 50 | 8 | **5.2x** [2.7x-32.9x] | 5.65 | 16.62 | 5.60 | 16.34 | 0.031 | 0.275 | `OK` |
| `t2_cflp_36` | 36 | 11 | **49.7x** [20.8x-143.3x] | 0.17 | 8.79 | 0.15 | 8.63 | 0.018 | 0.149 | `OK` |
| `t2_cflp_49` | 49 | 13 | **27.4x** [16.0x-72.6x] | 0.43 | 9.90 | 0.39 | 9.68 | 0.047 | 0.189 | `OK` |
| `t2_gap_45` | 45 | 13 | **18.3x** [4.1x-63.3x] | 0.86 | 13.49 | 0.83 | 13.27 | 0.024 | 0.184 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **11.4x** [4.8x-26.0x] | 3.91 | 65.19 | 3.89 | 64.97 | 0.029 | 0.235 | `OK` |
| `t2_binpack_44` | 44 | 14 | **12.2x** [5.1x-23.0x] | 0.98 | 12.34 | 0.96 | 12.17 | 0.022 | 0.173 | `OK` |
| `t2_setcover_50` | 50 | 20 | **47.6x** [6.8x-125.1x] | 0.18 | 10.29 | 0.14 | 10.00 | 0.034 | 0.285 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **39.6x** [35.1x-46.5x] | 0.22 | 8.55 | 0.15 | 8.29 | 0.073 | 0.257 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **52.4x** [41.0x-60.9x] | 0.17 | 9.31 | 0.12 | 8.92 | 0.047 | 0.386 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **25.6x** [22.2x-30.5x] | 0.42 | 10.78 | 0.32 | 10.21 | 0.111 | 0.571 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.1x** [3.3x-23.9x] | 3.74 | 45.03 | 3.63 | 44.72 | 0.038 | 0.276 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **7.1x** [1.3x-31.2x] | 16.49 | 158.88 | 16.42 | 158.44 | 0.057 | 0.436 | `OK` |
| `t3_mkp_200` | 200 | 8 | **4.3x** [3.1x-5.6x] | 23.45 | 111.06 | 23.38 | 110.49 | 0.076 | 0.612 | `OK` |
| `t3_cflp_300` | 300 | 34 | **32.5x** [7.8x-72.3x] | 7.27 | 252.11 | 6.99 | 251.24 | 0.123 | 0.878 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **2.5x** [1.4x-4.6x] | 78.22 | 159.87 | 78.00 | 156.50 | 0.262 | 1.871 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **9.0x** [4.4x-14.9x] | 7.66 | 77.73 | 7.27 | 75.13 | 0.296 | 2.634 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **29.9x** [23.0x-34.9x] | 0.55 | 16.87 | 0.42 | 15.79 | 0.123 | 0.966 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **17.1x** [10.3x-22.3x] | 0.98 | 17.04 | 0.78 | 15.43 | 0.220 | 1.610 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.8x** [1.1x-13.9x] | 8.04 | 62.34 | 7.11 | 53.78 | 1.147 | 8.007 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **9.9x** [4.3x-23.8x] | 34.15 | 353.73 | 29.35 | 319.90 | 4.429 | 32.994 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

## 4. Scalar vs. `GOEXPERIMENT=simd` Build

Ratios are geometric means of per-trial `scalar / SIMD` solve times over the same instances, with 95% t-intervals on the log ratios. "Reachable" is a necessary condition only: the AVX2 `Dot64`/`Axpy64`/`Scale64` paths need a slice of length at least 32, which requires `n + m >= 32`.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 5 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 3 faster, 4 slower, 16 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 0.96x [0.87x-1.05x] | 0.003 | 0.004 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.76x [0.68x-0.86x] | 0.003 | 0.004 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.84x [0.78x-0.91x] | 0.004 | 0.005 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source | 0.90x [0.81x-1.00x] | 0.004 | 0.005 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.85x [0.76x-0.94x] | 0.005 | 0.006 | 11 |
| `t1_furniture` | 5 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.88x [0.79x-0.97x] | 0.015 | 0.018 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.14x [1.09x-1.20x] | 0.125 | 0.102 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 1.02x [0.94x-1.11x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | SIMD slower | 0.69x [0.56x-0.85x] | 0.038 | 0.048 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 0.97x [0.94x-1.01x] | 0.460 | 0.452 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 0.96x [0.81x-1.14x] | 2.261 | 2.456 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 1.00x [0.97x-1.03x] | 5.596 | 5.340 | 11 |
| `t2_cflp_36` | 47 | possible | no detectable effect | 0.97x [0.93x-1.00x] | 0.151 | 0.164 | 11 |
| `t2_cflp_49` | 62 | possible | no detectable effect | 1.00x [0.86x-1.17x] | 0.391 | 0.343 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 1.02x [0.97x-1.09x] | 0.833 | 0.872 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 1.01x [0.93x-1.09x] | 3.887 | 3.309 | 11 |
| `t2_binpack_44` | 58 | possible | no detectable effect | 1.01x [0.97x-1.06x] | 0.957 | 0.976 | 11 |
| `t2_setcover_50` | 70 | possible | SIMD slower | 0.90x [0.84x-0.95x] | 0.142 | 0.161 | 11 |
| `t2_mad_g2_i8` | 72 | possible | SIMD faster | 1.12x [1.04x-1.20x] | 0.151 | 0.129 | 11 |
| `t2_mad_g3_i9` | 108 | possible | no detectable effect | 0.94x [0.83x-1.07x] | 0.122 | 0.127 | 11 |
| `t2_mad_g4_i12` | 156 | possible | SIMD faster | 1.12x [1.07x-1.18x] | 0.318 | 0.278 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.02x [0.98x-1.06x] | 3.626 | 3.029 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | SIMD slower | 0.97x [0.95x-1.00x] | 16.416 | 16.552 | 11 |
| `t3_mkp_200` | 208 | possible | no detectable effect | 1.03x [0.97x-1.10x] | 23.379 | 21.339 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.00x [0.87x-1.15x] | 6.985 | 5.953 | 5 |
| `t3_lotsizing_504` | 840 | possible | SIMD faster | 1.02x [1.01x-1.03x] | 78.000 | 77.434 | 5 |
| `t3_sched_1000` | 1100 | possible | SIMD slower | 0.96x [0.93x-1.00x] | 7.269 | 7.493 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 1.00x [0.99x-1.02x] | 0.417 | 0.411 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 0.97x [0.90x-1.05x] | 0.776 | 0.818 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 1.02x [0.93x-1.11x] | 7.105 | 7.143 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | no detectable effect | 1.03x [0.91x-1.16x] | 29.348 | 31.261 | 5 |
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
| `t1_std_lp` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 54.00 | 54.00 |
| `t1_std_lp_max` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 80.00 | 80.00 |
| `t1_std_mip` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.01 (3) | 0.002 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.01 (3) | 0.002 | 64.95 | 58.90 |
| `t1_whiskas` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.004 | 0.52 | 0.52 |
| `t1_furniture` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.002 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.12** (1 / 8) | 0.11 (2 / 8) | 0.17 (18) | 0.24 (35) | 0.010 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.04** (1 / 0) | 0.04 (1 / 0) | 0.05 (1) | 0.50 (4) | 0.082 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.46** (2 / 0) | 0.61 (12 / 0) | 0.53 (12) | 0.76 (109) | 0.102 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.26** (5 / 9) | 3.11 (40 / 9) | 1.16 (51) | 1.03 (61) | 0.027 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **5.60** (1 / 4) | 14.58 (2 / 4) | 5.64 (118) | 5.39 (111) | 0.042 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.15** (1 / 2) | 0.30 (3 / 2) | 0.17 (7) | 0.18 (7) | 0.025 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.39** (1 / 10) | 0.58 (4 / 10) | 0.30 (9) | 0.28 (9) | 0.034 | 909.02 | 827.30 |
| `t2_gap_45` | **0.83** (1 / 9) | 0.43 (1 / 9) | 1.72 (41) | 2.09 (60) | 0.028 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **3.89** (2 / 37) | 6.89 (25 / 37) | 3.18 (162) | 3.82 (199) | 0.069 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **0.96** (2 / 6) | 1.41 (5 / 6) | 9.68 (772) | 28.62 (1941) | 0.029 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.14** (1 / 6) | 0.35 (2 / 6) | 0.65 (6) | 0.62 (6) | 0.031 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.15** (1 / 0) | 0.83 (8 / 0) | 0.80 (8) | 0.78 (9) | 0.110 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.12** (1 / 0) | 0.12 (2 / 0) | 0.12 (2) | 2.69 (13) | 0.240 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.32** (1 / 0) | 3.30 (22 / 0) | 3.00 (22) | 5.35 (23) | 0.373 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.63** (43 / 0) | 2.99 (112 / 0) | 2.98 (112) | 3.82 (45) | 0.154 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **16.42** (196 / 0) | 31.32 (188 / 0) | 31.41 (188) | 92.24 (539) | 0.274 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **23.38** (250 / 14) | 107.70 (1135 / 16) | 216.09 (5000, `NodeLimit`) | 201.45 (5000, `NodeLimit`) | 0.124 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **6.99** (1 / 28) | 18.74 (54 / 34) | 8.88 (116) | 9.50 (118) | 0.173 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **78.00** (2 / 206) | 114.52 (376 / 206) | 250.03 (2914, `TimeLimit`) | 250.08 (3043, `TimeLimit`) | 0.697 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **7.27** (31 / 55) | 26.84 (87 / 53) | 250.08 (1797, `TimeLimit`) | 250.11 (1783, `TimeLimit`) | 0.233 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.42** (1 / 0) | 3.02 (17 / 0) | 2.90 (17) | 0.49 (1) | 0.646 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.78** (1 / 0) | 9.40 (58 / 0) | 8.07 (58) | 13.49 (74) | 1.587 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.11** (2 / 0) | 21.47 (25 / 0) | 19.35 (25) | 250.62 (146, `TimeLimit`) | 2.571 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **29.35** (1 / 0) | 32.09 (2 / 0) | 31.32 (2) | 250.21 (6, `TimeLimit`) | 75.872 | 208000.00 | 208000.00 |
<!-- END GENERATED:FULL_CHECKPOINT_TABLE -->

## 6. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus (`testdata/assign_corpus/manifest.json`; 11 randomized trials per family for Tiers 1-2 and 5 for Tier 3, 200 instances in total) compares pure-Go `assign.LinearSumFlat` against SciPy's C++ `rectangular_lsap` extension in warm state on bit-identical `float64` cost matrices, timing `linear_sum_assignment` without post-solve NumPy array gather overhead.

### 6.1 Aggregate LSAP Tier Summary

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `SciPy Solve / goblin` GeoMean [Clustered 95% CI] | Pooled Trial `SciPy Solve / goblin` | Equal-Family `SciPy Total / goblin` | `goblin` Solve Trial (Med) Sum (ms) | SciPy Solve Trial (Med) Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.21x** [0.93x-1.58x] | 1.21x | 3.93x | 0.253 (0.0204) | 0.307 (0.0274) | 1.37x [0.09x-2.38x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **2.95x** [1.86x-4.70x] | 2.95x | 3.00x | 8.715 (0.767) | 31.126 (2.815) | 2.51x [1.17x-15.44x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.89x** [1.84x-4.53x] | 2.89x | 3.63x | 201.623 (39.453) | 882.783 (175.604) | 2.49x [1.50x-6.33x] |
| Square families ($m = n$) | 16 (146) | **2.57x** [1.81x-3.65x] | 2.47x | 3.61x | 206.113 (39.449) | 903.950 (176.588) | 2.14x [0.09x-15.44x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.70x** [1.11x-2.61x] | 1.58x | 2.99x | 4.478 (0.791) | 10.265 (1.857) | 1.94x [0.59x-2.56x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.22x** [1.11x-4.44x] | 2.34x | 2.79x | 34.252 (6.069) | 62.106 (10.542) | 1.76x [0.94x-15.44x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.30x** [1.75x-3.03x] | 2.19x | 3.43x | 210.592 (40.240) | 914.215 (178.446) | 2.03x [0.09x-15.44x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 6.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.31x** | 0.0006 | 0.0010 | 0.0007 | 0.0130 | $0.0010 \pm 0.0010$ | $0.0010 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **0.90x** | 0.0018 | 0.0022 | 0.0019 | 0.0096 | $0.0038 \pm 0.0057$ | $0.0022 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.52x** | 0.0036 | 0.0058 | 0.0050 | 0.0149 | $0.0038 \pm 0.0010$ | $0.0058 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.01x** | 0.0039 | 0.0039 | 0.0050 | 0.0138 | $0.0038 \pm 0.0008$ | $0.0037 \pm 0.0007$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.02x** | 0.0044 | 0.0041 | 0.0054 | 0.0144 | $0.0043 \pm 0.0011$ | $0.0043 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.70x** | 0.0062 | 0.0104 | 0.0102 | 0.0317 | $0.0063 \pm 0.0014$ | $0.0108 \pm 0.0023$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.61x** | 0.0284 | 0.0700 | 0.0345 | 0.0950 | $0.0263 \pm 0.0066$ | $0.0673 \pm 0.0107$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.33x** | 0.0507 | 0.168 | 0.0661 | 0.216 | $0.0504 \pm 0.0041$ | $0.169 \pm 0.0253$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.00x** | 0.0467 | 0.0915 | 0.0690 | 0.139 | $0.0465 \pm 0.0072$ | $0.0933 \pm 0.0138$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.56x** | 0.135 | 0.207 | 0.141 | 0.240 | $0.145 \pm 0.0436$ | $0.220 \pm 0.0420$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **11.97x** | 0.0566 | 0.650 | 0.0775 | 0.721 | $0.0552 \pm 0.0072$ | $0.655 \pm 0.0208$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **2.02x** | 0.0331 | 0.0715 | 0.0548 | 0.141 | $0.0355 \pm 0.0096$ | $0.0704 \pm 0.0076$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.04x** | 0.0375 | 0.0756 | 0.0602 | 0.146 | $0.0375 \pm 0.0036$ | $0.0765 \pm 0.0074$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.30x** | 0.0795 | 0.264 | 0.109 | 0.334 | $0.0815 \pm 0.0119$ | $0.268 \pm 0.0301$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.87x** | 0.300 | 1.216 | 0.425 | 1.612 | $0.315 \pm 0.0536$ | $1.209 \pm 0.119$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.21x** | 1.140 | 4.992 | 1.498 | 7.352 | $1.209 \pm 0.163$ | $5.062 \pm 0.386$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.69x** | 0.831 | 1.402 | 1.380 | 3.866 | $0.840 \pm 0.0427$ | $1.421 \pm 0.0541$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.65x** | 4.990 | 8.175 | 5.523 | 8.873 | $5.445 \pm 1.906$ | $8.831 \pm 2.506$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.45x** | 0.332 | 0.779 | 0.686 | 3.364 | $0.325 \pm 0.0196$ | $0.796 \pm 0.0527$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.34x** | 0.381 | 0.923 | 0.759 | 3.498 | $0.393 \pm 0.0197$ | $0.917 \pm 0.0242$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **5.02x** | 5.526 | 26.579 | 7.130 | 34.682 | $5.500 \pm 0.334$ | $27.655 \pm 2.831$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.98x** | 26.253 | 132.754 | 35.031 | 174.627 | $26.612 \pm 3.492$ | $131.875 \pm 7.902$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 6.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSkip64`, `InitReducedRow64`, and `Min2ReducedRow64`)

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` with no `>= 8x8` tall transpose and either fused square `Minimize` non-sparse path or `rows * cols < 32`, so no AVX2 kernel runs): 3 of 3 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32`, tall `rows > cols` with `>= 8x8`, or fallback `AllAbsLE64`/`ValidateAndNegate64` at `rows * cols >= 32`): 17 faster, 0 slower, 2 with no detectable effect, out of 19.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.78x [1.11x-2.84x] | 0.0006 | 0.0004 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.74x [1.02x-2.97x] | 0.0018 | 0.0015 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.12x [1.01x-1.25x] | 0.0036 | 0.0034 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | SIMD faster | 1.29x [1.13x-1.46x] | 0.0039 | 0.0031 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | SIMD faster | 1.32x [1.10x-1.58x] | 0.0044 | 0.0030 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | no detectable effect | 1.01x [0.98x-1.05x] | 0.0062 | 0.0059 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.32x [1.27x-1.38x] | 0.0284 | 0.0213 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.29x [1.22x-1.37x] | 0.0507 | 0.0404 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.75x [1.55x-1.97x] | 0.0467 | 0.0287 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.73x [1.67x-1.79x] | 0.135 | 0.0777 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.20x [1.10x-1.30x] | 0.0566 | 0.0464 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | no detectable effect | 1.21x [0.96x-1.52x] | 0.0331 | 0.0277 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.20x [1.08x-1.34x] | 0.0375 | 0.0313 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.35x [1.22x-1.50x] | 0.0795 | 0.0568 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.58x [1.47x-1.69x] | 0.300 | 0.190 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.73x [1.62x-1.84x] | 1.140 | 0.692 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.45x [2.26x-2.65x] | 0.831 | 0.325 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.65x [2.53x-2.77x] | 4.990 | 1.817 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.69x [1.61x-1.76x] | 0.332 | 0.198 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.53x [1.32x-1.76x] | 0.381 | 0.238 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.65x [1.51x-1.81x] | 5.526 | 3.290 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.51x [1.43x-1.60x] | 26.253 | 18.174 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

## 7. Production Binary Footprint and Linker Dead-Code Elimination

Standalone `CGO_ENABLED=0` binaries compiled across the four canonical `goblin/assign` and `goblin` build profiles (`bench/results/goblin_binary_size.json`), verifying that unreferenced `WithCertified`, `WithVIPRTrace`, `encoding/json`, and (in `goblin/assign`) `math/big` symbols are eliminated by the Go linker:

<!-- BEGIN GENERATED:BINARY_SIZE_TABLE -->
| Profile | Default Raw / Stripped (KiB) | `GOEXPERIMENT=simd` Raw / Stripped (KiB) | Stripped Delta vs. Default | `goblin` Text (KiB) | `math/big` Syms | `encoding/json` Syms | `VIPR` Syms | `Certif` Syms |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `assign` default (`LinearSum` + `SolveInto`) | 2822.8 / **1768.2** | 2858.1 / **1832.2** | baseline | 118.7 | `0` | `0` | `0` | `0` |
| `assign` certified (`+ assign.WithCertified(true)`) | 2942.9 / **1844.2** | 2980.7 / **1916.2** | +76.0 KiB (+4.3%) | 177.2 | `0` | `0` | `0` | `17` |
| `goblin` default (`Model.Solve`, `DefaultOptions`) | 5586.5 / **3560.2** | 5472.0 / **3576.2** | baseline | 1023.4 | `130` | `0` | `0` | `0` |
| `goblin` certified + VIPR (`+ WithCertified + WithVIPRTrace`) | 6833.1 / **4400.2** | 6697.0 / **4420.2** | +840.0 KiB (+23.6%) | 1503.1 | `155` | `0` | `220` | `14` |
<!-- END GENERATED:BINARY_SIZE_TABLE -->
