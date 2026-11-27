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
- **Unified telemetry source**: fingerprint `06fa79b9f6ebed61` (working tree on top of commit `91f68c4c7f9073819337d0bc87b1acaaee4638ae`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 7.02 ms median on a 1-variable MILP and 7.03 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

## 2. Aggregate Workload Tier Summary

Tier 1 micro-models are dominated by Python subprocess spawn overhead, and the planted-gap MAD family is constructed so that root heuristics close the gap without branching; the bold row isolates solver-dominated Tier 2 and Tier 3 non-planted MILPs across paired trial instances.

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `Py / Go` GeoMean [Clustered 95% CI] | Pooled Trial `Py / Go` | Equal-Family `Go / CBC` | `Go` Trial (Med) Sum (ms) | `CBC` Trial (Med) Sum (ms) | `Py` Trial (Med) Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **399.24x** [157.83x-1009.86x] | 399.24x | 0.05x | 10.5 (0.9) | 159.1 (14.9) | 1206.6 (112.4) | 672.24x [37.46x-2577.67x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **20.91x** [12.23x-35.76x] | 20.91x | 0.17x | 576.2 (37.8) | 1817.4 (164.3) | 4829.7 (464.9) | 25.36x [1.37x-165.07x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **9.32x** [4.48x-19.40x] | 9.32x | 0.28x | 963.4 (186.8) | 2661.7 (539.1) | 6623.7 (1143.7) | 8.80x [1.03x-71.61x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **12.99x** [7.49x-22.52x] | 14.41x | 0.21x | 1169.0 (164.7) | 3072.1 (403.2) | 7356.2 (1064.2) | 12.79x [1.37x-165.07x] |
| Tier 3 non-MAD MILPs | 4 (20) | **7.39x** [1.27x-42.90x] | 7.39x | 0.29x | 603.0 (127.8) | 1318.1 (244.3) | 2987.3 (639.9) | 5.45x [1.45x-71.61x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **24.61x** [11.31x-53.56x] | 32.46x | 0.20x | 372.6 (60.1) | 1420.2 (301.5) | 4212.6 (554.8) | 39.32x [1.03x-159.69x] |
| Random-weight MAD (negative controls) | 3 (33) | **16.38x** [0.78x-345.09x] | 16.38x | 0.19x | 337.3 (22.6) | 1010.7 (101.0) | 2396.6 (262.7) | 15.59x [1.37x-120.08x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **53.79x** [21.86x-132.35x] | 66.23x | 0.11x | 1177.5 (165.4) | 3218.0 (416.9) | 8447.5 (1166.3) | 46.44x [1.37x-2577.67x] |
| All 31 synthetic corpus families | 31 (293) | **43.96x** [22.13x-87.33x] | 56.67x | 0.13x | 1550.2 (225.5) | 4638.2 (718.3) | 12660.0 (1721.1) | 42.46x [1.03x-2577.67x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2.1 Pure-Go vs. In-Process C++ CBC Aggregate Summary

Comparing `BackendGo` directly against in-process `BackendCBC` removes Python MPS formatting and subprocess spawn overhead across the same five workload subsets:

<!-- BEGIN GENERATED:CBC_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `BackendCBC / BackendGo` Total [Clustered 95% CI] | Equal-Family `BackendCBC / BackendGo` Solve [Clustered 95% CI] | `BackendGo` Trial (Med) Sum (ms) | `BackendCBC` Trial (Med) Sum (ms) | Per-Instance `CBC / Go` Total Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro-models (`N <= 26`) | 10 (110) | **20.62x** [8.37x-50.78x] | **39.40x** [12.16x-127.71x] | 10.5 (0.9) | 159.1 (14.9) | 15.95x [3.49x-136.79x] |
| Tier 2: Medium MILPs (`30 <= N <= 84`) | 13 (143) | **5.73x** [4.07x-8.06x] | **6.40x** [4.34x-9.42x] | 576.2 (37.8) | 1817.4 (164.3) | 5.76x [0.59x-45.05x] |
| Tier 3: Large MILPs (`145 <= N <= 4036`) | 8 (40) | **3.59x** [2.06x-6.25x] | **3.99x** [2.19x-7.25x] | 963.4 (186.8) | 2661.7 (539.1) | 3.57x [0.37x-14.49x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **4.81x** [3.20x-7.23x] | **5.03x** [3.25x-7.78x] | 1169.0 (164.7) | 3072.1 (403.2) | 5.20x [0.59x-45.05x] |
| All 31 synthetic families | 31 (293) | **7.67x** [5.11x-11.52x] | **10.18x** [6.03x-17.17x] | 1550.2 (225.5) | 4638.2 (718.3) | 6.87x [0.37x-136.79x] |
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
| `t1_std_lp` | 3 | 3 | **788.9x** [403.7x-1165.3x] | 0.01 | 7.13 | 0.00 | 7.06 | 0.004 | 0.068 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **1040.3x** [711.9x-1583.2x] | 0.01 | 7.43 | 0.00 | 7.37 | 0.004 | 0.068 | `OK` |
| `t1_std_mip` | 3 | 3 | **881.3x** [655.5x-1076.2x] | 0.01 | 8.57 | 0.00 | 8.48 | 0.004 | 0.072 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **1035.6x** [810.2x-1233.2x] | 0.01 | 8.59 | 0.00 | 8.52 | 0.004 | 0.079 | `OK` |
| `t1_whiskas` | 6 | 5 | **572.4x** [403.0x-710.3x] | 0.01 | 8.19 | 0.01 | 8.09 | 0.007 | 0.103 | `OK` |
| `t1_furniture` | 2 | 3 | **260.1x** [71.2x-1060.0x] | 0.03 | 8.74 | 0.02 | 8.67 | 0.006 | 0.066 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **93.5x** [37.5x-350.0x] | 0.12 | 10.51 | 0.11 | 10.40 | 0.009 | 0.105 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **2367.8x** [2024.6x-2577.7x] | 0.00 | 6.78 | 0.00 | 6.74 | 0.003 | 0.055 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **62.6x** [38.7x-159.7x] | 0.19 | 10.30 | 0.09 | 10.06 | 0.096 | 0.243 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **66.6x** [40.6x-120.1x] | 0.52 | 36.17 | 0.49 | 35.93 | 0.027 | 0.248 | `OK` |
| `t2_mkp_30` | 30 | 6 | **9.8x** [4.9x-14.4x] | 2.78 | 30.69 | 2.76 | 30.51 | 0.028 | 0.190 | `OK` |
| `t2_mkp_50` | 50 | 8 | **5.0x** [2.5x-29.8x] | 5.38 | 19.06 | 5.35 | 18.77 | 0.034 | 0.272 | `OK` |
| `t2_cflp_36` | 36 | 11 | **57.9x** [24.4x-165.1x] | 0.19 | 10.92 | 0.17 | 10.73 | 0.019 | 0.194 | `OK` |
| `t2_cflp_49` | 49 | 13 | **33.7x** [22.5x-55.5x] | 0.37 | 11.79 | 0.35 | 11.56 | 0.026 | 0.233 | `OK` |
| `t2_gap_45` | 45 | 13 | **19.5x** [4.6x-67.4x] | 0.96 | 15.82 | 0.93 | 15.60 | 0.024 | 0.219 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **13.3x** [6.0x-33.5x] | 3.85 | 80.86 | 3.82 | 80.59 | 0.036 | 0.272 | `OK` |
| `t2_binpack_44` | 44 | 14 | **14.4x** [5.8x-28.7x] | 1.06 | 15.65 | 1.04 | 15.43 | 0.022 | 0.222 | `OK` |
| `t2_setcover_50` | 50 | 20 | **54.8x** [7.1x-145.1x] | 0.20 | 12.95 | 0.15 | 12.62 | 0.035 | 0.337 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **53.0x** [40.9x-94.5x] | 0.26 | 12.86 | 0.17 | 12.38 | 0.078 | 0.407 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **70.2x** [44.9x-94.0x] | 0.19 | 13.36 | 0.14 | 12.82 | 0.051 | 0.588 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **30.2x** [26.6x-34.1x] | 0.47 | 14.39 | 0.35 | 13.68 | 0.122 | 0.731 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.7x** [3.5x-25.3x] | 3.22 | 49.80 | 3.18 | 49.47 | 0.042 | 0.326 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **6.8x** [1.4x-29.0x] | 18.88 | 176.76 | 18.78 | 176.29 | 0.066 | 0.461 | `OK` |
| `t3_mkp_200` | 200 | 8 | **4.3x** [3.2x-5.7x] | 25.52 | 123.20 | 25.38 | 122.56 | 0.085 | 0.639 | `OK` |
| `t3_cflp_300` | 300 | 34 | **32.3x** [8.0x-71.6x] | 6.97 | 264.92 | 6.85 | 263.96 | 0.114 | 0.972 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **2.5x** [1.4x-4.4x] | 86.74 | 165.84 | 86.47 | 163.87 | 0.243 | 1.995 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **8.6x** [3.9x-13.3x] | 8.56 | 85.94 | 8.22 | 83.09 | 0.357 | 2.934 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **25.9x** [20.7x-37.9x] | 0.84 | 19.53 | 0.57 | 18.40 | 0.268 | 1.129 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **14.6x** [9.3x-17.6x] | 1.15 | 19.87 | 0.91 | 18.05 | 0.361 | 1.838 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.7x** [1.0x-14.1x] | 8.89 | 65.96 | 7.79 | 58.02 | 1.115 | 8.820 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **8.9x** [4.3x-24.0x] | 48.15 | 398.48 | 42.78 | 361.95 | 5.632 | 36.711 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

## 4. Scalar vs. `GOEXPERIMENT=simd` Build

Ratios are geometric means of per-trial `scalar / SIMD` solve times over the same instances, with 95% t-intervals on the log ratios. "Reachable" is a necessary condition only: the AVX2 `Dot64`/`Axpy64`/`Scale64` paths need a slice of length at least 32, which requires `n + m >= 32`.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 5 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 0 faster, 19 slower, 4 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 1.23x [0.93x-1.63x] | 0.004 | 0.003 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 0.89x [0.72x-1.10x] | 0.003 | 0.004 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.59x [0.46x-0.75x] | 0.005 | 0.008 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.66x [0.60x-0.73x] | 0.004 | 0.007 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.81x [0.71x-0.93x] | 0.006 | 0.008 | 11 |
| `t1_furniture` | 5 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.82x [0.73x-0.94x] | 0.019 | 0.025 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source | 0.91x [0.81x-1.03x] | 0.108 | 0.126 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.74x [0.69x-0.79x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 1.26x [0.85x-1.86x] | 0.090 | 0.066 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | SIMD slower | 0.75x [0.72x-0.79x] | 0.491 | 0.651 | 11 |
| `t2_mkp_30` | 36 | possible | SIMD slower | 0.92x [0.88x-0.96x] | 2.760 | 2.892 | 11 |
| `t2_mkp_50` | 58 | possible | SIMD slower | 0.84x [0.81x-0.88x] | 5.348 | 5.728 | 11 |
| `t2_cflp_36` | 47 | possible | SIMD slower | 0.64x [0.62x-0.66x] | 0.171 | 0.257 | 11 |
| `t2_cflp_49` | 62 | possible | SIMD slower | 0.58x [0.46x-0.74x] | 0.346 | 0.589 | 11 |
| `t2_gap_45` | 58 | possible | SIMD slower | 0.88x [0.84x-0.91x] | 0.931 | 1.105 | 11 |
| `t2_lotsizing_48` | 80 | possible | SIMD slower | 0.89x [0.84x-0.95x] | 3.819 | 4.542 | 11 |
| `t2_binpack_44` | 58 | possible | no detectable effect | 0.99x [0.98x-1.00x] | 1.036 | 1.052 | 11 |
| `t2_setcover_50` | 70 | possible | SIMD slower | 0.81x [0.77x-0.85x] | 0.154 | 0.202 | 11 |
| `t2_mad_g2_i8` | 72 | possible | SIMD slower | 0.78x [0.68x-0.90x] | 0.169 | 0.242 | 11 |
| `t2_mad_g3_i9` | 108 | possible | SIMD slower | 0.89x [0.83x-0.95x] | 0.135 | 0.164 | 11 |
| `t2_mad_g4_i12` | 156 | possible | SIMD slower | 0.81x [0.73x-0.89x] | 0.347 | 0.430 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | SIMD slower | 0.84x [0.80x-0.87x] | 3.176 | 3.881 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | SIMD slower | 0.72x [0.67x-0.79x] | 18.780 | 21.374 | 11 |
| `t3_mkp_200` | 208 | possible | SIMD slower | 0.68x [0.57x-0.82x] | 25.381 | 36.639 | 5 |
| `t3_cflp_300` | 334 | possible | SIMD slower | 0.88x [0.79x-0.98x] | 6.845 | 7.249 | 5 |
| `t3_lotsizing_504` | 840 | possible | SIMD slower | 0.76x [0.66x-0.87x] | 86.471 | 108.840 | 5 |
| `t3_sched_1000` | 1100 | possible | SIMD slower | 0.82x [0.77x-0.87x] | 8.223 | 10.412 | 5 |
| `t3_mad_g5_i20` | 240 | possible | SIMD slower | 0.90x [0.84x-0.96x] | 0.567 | 0.591 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 0.96x [0.72x-1.28x] | 0.912 | 1.075 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | SIMD slower | 0.79x [0.71x-0.88x] | 7.793 | 10.475 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | no detectable effect | 0.92x [0.74x-1.16x] | 42.777 | 44.276 | 5 |
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
| `t1_whiskas` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.004 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.003 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.11** (1 / 8) | 0.14 (2 / 8) | 0.18 (18) | 0.26 (35) | 0.009 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.09** (1 / 0) | 0.04 (1 / 0) | 0.06 (1) | 0.58 (4) | 0.088 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.49** (2 / 0) | 0.60 (12 / 0) | 0.59 (12) | 0.87 (109) | 0.113 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.76** (5 / 9) | 3.44 (40 / 9) | 1.27 (51) | 1.11 (61) | 0.028 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **5.35** (1 / 4) | 16.52 (2 / 4) | 6.17 (118) | 5.61 (111) | 0.049 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.17** (1 / 2) | 0.30 (3 / 2) | 0.21 (7) | 0.20 (7) | 0.030 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.35** (1 / 10) | 0.66 (4 / 10) | 0.34 (9) | 0.31 (9) | 0.038 | 909.02 | 827.30 |
| `t2_gap_45` | **0.93** (1 / 9) | 0.44 (1 / 9) | 1.92 (41) | 2.35 (60) | 0.031 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **3.82** (2 / 37) | 7.70 (25 / 37) | 3.83 (162) | 4.02 (199) | 0.067 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **1.04** (2 / 6) | 1.57 (5 / 6) | 9.57 (772) | 31.71 (1941) | 0.030 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.15** (1 / 6) | 0.39 (2 / 6) | 0.72 (6) | 0.70 (6) | 0.032 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.17** (1 / 0) | 0.92 (8 / 0) | 0.82 (8) | 0.81 (9) | 0.109 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.14** (1 / 0) | 0.14 (2 / 0) | 0.14 (2) | 2.94 (13) | 0.261 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.35** (1 / 0) | 3.74 (22 / 0) | 3.58 (22) | 5.67 (23) | 0.382 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.18** (43 / 0) | 3.45 (112 / 0) | 3.26 (112) | 4.11 (45) | 0.170 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **18.78** (196 / 0) | 33.92 (188 / 0) | 34.92 (188) | 91.27 (682) | 0.304 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **25.38** (250 / 14) | 117.86 (1135 / 16) | 241.71 (5000, `NodeLimit`) | 225.12 (5000, `NodeLimit`) | 0.138 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **6.85** (1 / 28) | 20.37 (54 / 34) | 10.37 (116) | 9.87 (118) | 0.187 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **86.47** (2 / 206) | 126.38 (376 / 206) | 250.02 (2652, `TimeLimit`) | 250.07 (2737, `TimeLimit`) | 0.769 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **8.22** (31 / 55) | 29.66 (87 / 53) | 250.02 (1613, `TimeLimit`) | 250.12 (1633, `TimeLimit`) | 0.235 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.57** (1 / 0) | 3.50 (17 / 0) | 3.10 (17) | 0.55 (1) | 0.706 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.91** (1 / 0) | 10.75 (58 / 0) | 8.99 (58) | 15.22 (74) | 1.708 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.79** (2 / 0) | 23.86 (25 / 0) | 20.95 (25) | 250.34 (153, `TimeLimit`) | 2.822 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **42.78** (1 / 0) | 37.46 (2 / 0) | 42.93 (2) | 251.92 (5, `TimeLimit`) | 81.818 | 208000.00 | 208000.00 |
<!-- END GENERATED:FULL_CHECKPOINT_TABLE -->

## 6. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus (`testdata/assign_corpus/manifest.json`; 11 randomized trials per family for Tiers 1-2 and 5 for Tier 3, 200 instances in total) compares pure-Go `assign.LinearSumFlat` against SciPy's C++ `rectangular_lsap` extension in warm state on bit-identical `float64` cost matrices, timing `linear_sum_assignment` without post-solve NumPy array gather overhead.

### 6.1 Aggregate LSAP Tier Summary

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `SciPy Solve / goblin` GeoMean [Clustered 95% CI] | Pooled Trial `SciPy Solve / goblin` | Equal-Family `SciPy Total / goblin` | `goblin` Solve Trial (Med) Sum (ms) | SciPy Solve Trial (Med) Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.46x** [1.14x-1.87x] | 1.46x | 4.26x | 0.232 (0.0206) | 0.333 (0.0307) | 1.39x [0.79x-5.79x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **2.99x** [1.87x-4.80x] | 2.99x | 3.00x | 9.534 (0.851) | 34.300 (3.095) | 2.65x [1.14x-15.60x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.82x** [1.86x-4.28x] | 2.82x | 3.64x | 251.283 (50.051) | 1003.123 (202.872) | 2.55x [1.50x-6.29x] |
| Square families ($m = n$) | 16 (146) | **2.71x** [1.98x-3.72x] | 2.66x | 3.72x | 256.008 (49.991) | 1026.277 (203.922) | 2.20x [0.89x-15.60x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.77x** [1.25x-2.50x] | 1.65x | 2.99x | 5.041 (0.931) | 11.479 (2.075) | 1.89x [0.79x-2.69x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.25x** [1.12x-4.52x] | 2.37x | 2.67x | 37.112 (6.544) | 67.773 (11.338) | 1.80x [0.89x-15.60x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.41x** [1.88x-3.10x] | 2.34x | 3.51x | 261.049 (50.923) | 1037.756 (205.998) | 2.05x [0.79x-15.60x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 6.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **2.12x** | 0.0005 | 0.0011 | 0.0006 | 0.0147 | $0.0006 \pm 0.0001$ | $0.0013 \pm 0.0006$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.26x** | 0.0019 | 0.0026 | 0.0023 | 0.0106 | $0.0020 \pm 0.0004$ | $0.0025 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.53x** | 0.0039 | 0.0065 | 0.0058 | 0.0159 | $0.0042 \pm 0.0013$ | $0.0063 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.15x** | 0.0039 | 0.0042 | 0.0052 | 0.0149 | $0.0036 \pm 0.0007$ | $0.0041 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.20x** | 0.0036 | 0.0047 | 0.0053 | 0.0161 | $0.0040 \pm 0.0012$ | $0.0047 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.71x** | 0.0067 | 0.0117 | 0.0116 | 0.0348 | $0.0068 \pm 0.0018$ | $0.0114 \pm 0.0019$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.74x** | 0.0299 | 0.0755 | 0.0362 | 0.103 | $0.0276 \pm 0.0067$ | $0.0740 \pm 0.0114$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.30x** | 0.0561 | 0.185 | 0.0722 | 0.238 | $0.0571 \pm 0.0125$ | $0.187 \pm 0.0266$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.06x** | 0.0500 | 0.0978 | 0.0758 | 0.147 | $0.0483 \pm 0.0038$ | $0.100 \pm 0.0144$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.54x** | 0.150 | 0.228 | 0.158 | 0.268 | $0.160 \pm 0.0503$ | $0.242 \pm 0.0454$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **12.24x** | 0.0631 | 0.713 | 0.0858 | 0.792 | $0.0593 \pm 0.0089$ | $0.719 \pm 0.0244$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.95x** | 0.0370 | 0.0740 | 0.0632 | 0.152 | $0.0389 \pm 0.0102$ | $0.0745 \pm 0.0078$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **1.99x** | 0.0411 | 0.0838 | 0.0645 | 0.162 | $0.0432 \pm 0.0097$ | $0.0847 \pm 0.0070$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.68x** | 0.0849 | 0.304 | 0.110 | 0.383 | $0.0847 \pm 0.0085$ | $0.312 \pm 0.0378$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.84x** | 0.339 | 1.335 | 0.453 | 1.775 | $0.347 \pm 0.0494$ | $1.325 \pm 0.125$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.08x** | 1.264 | 5.479 | 1.727 | 8.435 | $1.381 \pm 0.225$ | $5.591 \pm 0.380$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.71x** | 0.910 | 1.563 | 1.514 | 4.618 | $0.918 \pm 0.0515$ | $1.570 \pm 0.0598$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.66x** | 5.360 | 8.718 | 5.940 | 9.697 | $5.891 \pm 2.170$ | $9.611 \pm 2.784$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.48x** | 0.373 | 0.873 | 0.763 | 4.032 | $0.359 \pm 0.0262$ | $0.890 \pm 0.0607$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.30x** | 0.473 | 1.036 | 1.003 | 4.270 | $0.452 \pm 0.0345$ | $1.037 \pm 0.0429$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.92x** | 6.309 | 30.356 | 7.906 | 39.481 | $6.141 \pm 0.479$ | $30.245 \pm 2.595$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.35x** | 35.362 | 154.846 | 42.551 | 203.602 | $35.115 \pm 5.075$ | $151.681 \pm 10.062$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 6.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSkip64`, `InitReducedRow64`, and `Min2ReducedRow64`)

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` with no `>= 8x8` tall transpose and either fused square `Minimize` non-sparse path or `rows * cols < 32`, so no AVX2 kernel runs): 2 of 3 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32`, tall `rows > cols` with `>= 8x8`, or fallback `AllAbsLE64`/`ValidateAndNegate64` at `rows * cols >= 32`): 18 faster, 1 slower, 0 with no detectable effect, out of 19.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.09x [1.04x-1.15x] | 0.0005 | 0.0005 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.21x [1.18x-1.25x] | 0.0019 | 0.0016 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source | 1.11x [0.97x-1.27x] | 0.0039 | 0.0038 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | SIMD faster | 1.09x [1.05x-1.13x] | 0.0039 | 0.0035 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | SIMD faster | 1.15x [1.08x-1.22x] | 0.0036 | 0.0034 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | SIMD slower | 0.97x [0.95x-1.00x] | 0.0067 | 0.0066 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.16x [1.04x-1.30x] | 0.0299 | 0.0264 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.29x [1.24x-1.35x] | 0.0561 | 0.0433 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.72x [1.58x-1.88x] | 0.0500 | 0.0298 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.79x [1.69x-1.89x] | 0.150 | 0.0846 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.22x [1.17x-1.28x] | 0.0631 | 0.0501 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.24x [1.00x-1.54x] | 0.0370 | 0.0288 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.30x [1.08x-1.56x] | 0.0411 | 0.0310 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.35x [1.23x-1.47x] | 0.0849 | 0.0608 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.60x [1.50x-1.70x] | 0.339 | 0.212 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.76x [1.63x-1.89x] | 1.264 | 0.788 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.42x [2.10x-2.78x] | 0.910 | 0.364 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.40x [2.32x-2.48x] | 5.360 | 2.337 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.60x [1.40x-1.81x] | 0.373 | 0.232 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.48x [1.20x-1.81x] | 0.473 | 0.293 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.59x [1.51x-1.67x] | 6.309 | 3.954 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.45x [1.34x-1.56x] | 35.362 | 24.284 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

## 7. Production Binary Footprint and Linker Dead-Code Elimination

Standalone `CGO_ENABLED=0` binaries compiled across the four canonical `goblin/assign` and `goblin` build profiles (`bench/results/goblin_binary_size.json`), verifying that unreferenced `WithCertified`, `WithVIPRTrace`, `encoding/json`, and (in `goblin/assign`) `math/big` symbols are eliminated by the Go linker:

<!-- BEGIN GENERATED:BINARY_SIZE_TABLE -->
| Profile | Default Raw / Stripped (KiB) | `GOEXPERIMENT=simd` Raw / Stripped (KiB) | Stripped Delta vs. Default | `goblin` Text (KiB) | `math/big` Syms | `encoding/json` Syms | `VIPR` Syms | `Certif` Syms |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `assign` default (`LinearSum` + `SolveInto`) | 2822.8 / **1768.2** | 2858.1 / **1832.2** | baseline | 118.7 | `0` | `0` | `0` | `0` |
| `assign` certified (`+ assign.WithCertified(true)`) | 2942.9 / **1844.2** | 2980.7 / **1916.2** | +76.0 KiB (+4.3%) | 177.2 | `0` | `0` | `0` | `17` |
| `goblin` default (`Model.Solve`, `DefaultOptions`) | 5915.4 / **3776.2** | 5798.6 / **3796.2** | baseline | 1168.5 | `130` | `0` | `0` | `0` |
| `goblin` certified + VIPR (`+ WithCertified + WithVIPRTrace`) | 7353.0 / **4752.2** | 7215.7 / **4772.2** | +976.0 KiB (+25.8%) | 1743.7 | `159` | `0` | `220` | `20` |
<!-- END GENERATED:BINARY_SIZE_TABLE -->

## 8. Combinatorial Group-Item Allocation (Automatic `Model.Solve` Dispatch vs. Pure Branch-and-Cut)

The 13-family combinatorial MAD corpus (`testdata/mad_corpus/manifest.json`; 131 randomized trials across the `pmad_base_g10_i2` base instance with exact Trial-0 optimum `24137`, 8 PMAD families across $G \in \{6, 10, 20\}$, $I \in \{4..12\}$, and $\lambda \in \{0.25, 1.0\}$, plus 4 random-weight controls) evaluates default `Model.Solve` (automatic algebraic MAD detection routed to `internal/mad`, alongside internal `StrategyDP` and `StrategyBruteForce` breakdowns when DFS states are at most `250,000`) against 250 ms-capped pure Branch-and-Cut (`WithCombinatorialSearch(false)` on `BackendGo`, in-process `BackendCBC`, and Python `PuLP + CBC` subprocess execution) with `1e-9` combinatorial parity and `[BestBound, Incumbent]` containment.

### 8.1 Aggregate Combinatorial MAD Summary

<!-- BEGIN GENERATED:MAD_COMBINATORIAL_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Auto Trial (Med) Sum (ms) | DP Trial (Med) Sum (ms) | DFS Trial (Med) Sum (ms) | MILP Go Trial (Med) Sum (ms) [Opt] | In-Process CBC Trial (Med) Sum (ms) [Opt] | Python PuLP+CBC Trial (Med) Sum (ms) [Opt] | Equal-Family `MILP Go / Auto` [95% CI] | Equal-Family `CBC / Auto` [95% CI] | Equal-Family `Python / Auto` [95% CI] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| PMAD base (`G = 10, I = 2`) | 1 (11) | 0.410 (0.0371) | 0.106 (0.0096) | 1.049 (0.0952) | 919.3 (17.2) [8/11] | 765.9 (67.6) [11/11] | 2094.1 (245.4) [7/11] | **781.9x** | **1708.9x** | **4204.2x** |
| PMAD small & medium (`I <= 10`) | 6 (66) | 132.700 (11.157) | 129.826 (11.398) | 453.464 (40.054) [2/6] | 15843.8 (1500.8) [7/66] | 13190.2 (1200.6) [25/66] | 16966.0 (1541.9) [18/66] | **351.9x** [58.8x-2105.3x] | **266.5x** [63.6x-1116.5x] | **377.6x** [66.8x-2134.7x] |
| PMAD large (`I = 12`) | 2 (10) | 456.963 (83.890) | 439.485 (88.727) | skip (>250k) | 2502.8 (500.6) [0/10] | 2550.6 (510.2) [0/10] | 2796.5 (560.6) [0/10] | **6.0x** [0.0x-1048.4x] | **6.1x** [0.0x-961.1x] | **6.7x** [0.1x-723.6x] |
| Random-weight MAD controls (`I <= 12`) | 4 (44) | 7.102 (0.674) | 6.059 (0.581) | 404.910 (35.691) | 2457.3 (273.7) [38/44] | 4323.9 (413.5) [33/44] | 5730.0 (573.7) [33/44] | **113.1x** [22.8x-561.6x] | **541.8x** [252.9x-1160.7x] | **1034.4x** [397.9x-2689.0x] |
| **All 13 combinatorial MAD families** | 13 (131) | 597.175 (95.759) | 575.476 (100.715) | 859.423 (75.841) [7/13] | 21723.1 (2292.2) [53/131] | 20830.7 (2191.9) [69/131] | 27586.6 (2921.6) [58/131] | **141.1x** [43.2x-460.4x] | **214.0x** [67.6x-677.6x] | **333.3x** [89.1x-1246.3x] |
<!-- END GENERATED:MAD_COMBINATORIAL_SUMMARY_TABLE -->

### 8.2 Full 13-Family Combinatorial MAD Pipeline Comparison

<!-- BEGIN GENERATED:MAD_COMBINATORIAL_PIPELINE_TABLE -->
| Problem ID | $G \times I \times T$ | $\lambda$ | Auto Strategy | Auto Med (ms) | DP Med (ms) | DFS Med (ms) | MILP Go 250 ms | In-Process CBC 250 ms | Python PuLP+CBC 250 ms | Trial 0 Obj | Parity |
| :--- | :---: | ---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `pmad_base_g10_i2` | $10 \times 2 \times 6$ | 1.00 | `DP` | 0.0371 | 0.0096 | 0.0952 | 17.16 (8/11 opt) | 67.60 (`Optimal`) | 245.39 (7/11 opt) | 24137.0 | `OK` |
| `pmad_g6_i6_lam025` | $6 \times 6 \times 6$ | 0.25 | `DP` | 0.118 | 0.0931 | 29.289 | 250.04 (4/11 opt) | 121.90 (`Optimal`) | 220.34 (7/11 opt) | 13259.5 | `OK` |
| `pmad_g6_i10_lam10` | $6 \times 10 \times 6$ | 1.00 | `DP` | 3.963 | 4.142 | skip (>250k) | 250.12 (`TimeLimit`) | 252.52 (`TimeLimit`) | 274.18 (`TimeLimit`) | 14018.0 | `OK` |
| `pmad_g6_i12_lam025` | $6 \times 12 \times 6$ | 0.25 | `DP` | 26.430 | 29.123 | skip (>250k) | 250.15 (`TimeLimit`) | 252.35 (`TimeLimit`) | 268.80 (`TimeLimit`) | 9787.0 | `OK` |
| `pmad_g10_i4_lam025` | $10 \times 4 \times 6$ | 0.25 | `DP` | 0.0699 | 0.0389 | 10.766 | 250.12 (3/11 opt) | 65.12 (`Optimal`) | 189.94 (`Optimal`) | 23954.0 | `OK` |
| `pmad_g10_i8_lam10` | $10 \times 8 \times 6$ | 1.00 | `DP` | 0.803 | 0.772 | skip (>250k) | 250.10 (`TimeLimit`) | 252.77 (2/11 opt) | 282.59 (`TimeLimit`) | 21928.0 | `OK` |
| `pmad_g10_i10_lam025` | $10 \times 10 \times 6$ | 0.25 | `DP` | 5.727 | 5.942 | skip (>250k) | 250.22 (`TimeLimit`) | 252.96 (1/11 opt) | 283.33 (`TimeLimit`) | 22570.0 | `OK` |
| `pmad_g20_i6_lam10` | $20 \times 6 \times 6$ | 1.00 | `DP` | 0.477 | 0.410 | skip (>250k) | 250.22 (`TimeLimit`) | 255.31 (`TimeLimit`) | 291.53 (`TimeLimit`) | 43530.0 | `OK` |
| `pmad_g20_i12_lam10` | $20 \times 12 \times 6$ | 1.00 | `DP` | 57.460 | 59.604 | skip (>250k) | 250.45 (`TimeLimit`) | 257.86 (`TimeLimit`) | 291.78 (`TimeLimit`) | 38256.0 | `OK` |
| `mad_comb_rand_g2_i4` | $2 \times 4 \times 6$ | 0.00 | `BruteForce` | 0.0258 | 0.0056 | 0.0037 | 0.75 (`Optimal`) | 15.46 (`Optimal`) | 42.62 (`Optimal`) | 4601.0 | `OK` |
| `mad_comb_rand_g2_i8` | $2 \times 8 \times 6$ | 0.00 | `BruteForce` | 0.0475 | 0.0433 | 0.0261 | 3.78 (`Optimal`) | 16.21 (`Optimal`) | 60.05 (`Optimal`) | 5975.0 | `OK` |
| `mad_comb_rand_g3_i9` | $3 \times 9 \times 6$ | 0.00 | `DP` | 0.0850 | 0.0584 | 0.327 | 19.08 (`Optimal`) | 130.26 (`Optimal`) | 204.01 (10/11 opt) | 7720.0 | `OK` |
| `mad_comb_rand_g4_i12` | $4 \times 12 \times 6$ | 0.00 | `DP` | 0.516 | 0.473 | 35.334 | 250.04 (5/11 opt) | 251.54 (`TimeLimit`) | 267.06 (1/11 opt) | 7905.0 | `OK` |
<!-- END GENERATED:MAD_COMBINATORIAL_PIPELINE_TABLE -->
