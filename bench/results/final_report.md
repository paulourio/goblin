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
- **Unified telemetry source**: fingerprint `907d83c35a5be855` (commit `69bc7898a2e3f299552b742f6bce22deb3f724c0`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.48 ms median on a 1-variable MILP and 6.54 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

## 2. Aggregate Workload Tier Summary

Tier 1 micro-models are dominated by Python subprocess spawn overhead, and the planted-gap MAD family is constructed so that root heuristics close the gap without branching; the bold row isolates solver-dominated Tier 2 and Tier 3 non-planted MILPs across paired trial instances.

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `Py / Go` GeoMean [Clustered 95% CI] | Pooled Trial `Py / Go` | Equal-Family `Go / CBC` | `Go` Trial (Med) Sum (ms) | `CBC` Trial (Med) Sum (ms) | `Py` Trial (Med) Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **402.11x** [180.62x-895.25x] | 402.11x | 0.04x | 8.8 (0.8) | 153.0 (14.5) | 1098.1 (103.1) | 686.80x [40.42x-1803.14x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **19.49x** [11.70x-32.49x] | 19.49x | 0.18x | 552.4 (37.2) | 1750.7 (152.5) | 4442.1 (430.4) | 22.29x [1.36x-155.94x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **9.83x** [4.54x-21.29x] | 9.83x | 0.26x | 908.0 (175.4) | 2556.2 (519.0) | 6366.4 (1095.6) | 9.43x [1.04x-63.30x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **12.30x** [7.28x-20.77x] | 13.56x | 0.22x | 1132.9 (164.2) | 2961.0 (381.9) | 6954.5 (1013.0) | 12.98x [1.36x-155.94x] |
| Tier 3 non-MAD MILPs | 4 (20) | **7.23x** [1.26x-41.56x] | 7.23x | 0.30x | 589.7 (127.8) | 1268.9 (234.4) | 2880.5 (616.0) | 5.10x [1.48x-63.30x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **27.32x** [12.08x-61.79x] | 35.73x | 0.17x | 328.5 (48.5) | 1358.4 (290.7) | 3963.0 (522.9) | 44.77x [1.04x-140.93x] |
| Random-weight MAD (negative controls) | 3 (33) | **16.03x** [0.79x-324.29x] | 16.03x | 0.19x | 323.0 (22.7) | 993.1 (96.5) | 2297.7 (254.2) | 16.00x [1.36x-117.04x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **50.93x** [21.00x-123.55x] | 62.49x | 0.11x | 1140.7 (164.9) | 3101.4 (395.3) | 7943.6 (1106.2) | 44.43x [1.36x-1803.14x] |
| All 31 synthetic corpus families | 31 (293) | **43.37x** [22.10x-85.11x] | 55.31x | 0.13x | 1469.2 (213.4) | 4459.8 (686.0) | 11906.6 (1629.1) | 44.43x [1.04x-1803.14x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2.1 Pure-Go vs. In-Process C++ CBC Aggregate Summary

Comparing `BackendGo` directly against in-process `BackendCBC` removes Python MPS formatting and subprocess spawn overhead across the same five workload subsets:

<!-- BEGIN GENERATED:CBC_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `BackendCBC / BackendGo` Total [Clustered 95% CI] | Equal-Family `BackendCBC / BackendGo` Solve [Clustered 95% CI] | `BackendGo` Trial (Med) Sum (ms) | `BackendCBC` Trial (Med) Sum (ms) | Per-Instance `CBC / Go` Total Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro-models (`N <= 26`) | 10 (110) | **22.23x** [9.81x-50.38x] | **43.44x** [13.90x-135.74x] | 8.8 (0.8) | 153.0 (14.5) | 16.69x [3.84x-137.01x] |
| Tier 2: Medium MILPs (`30 <= N <= 84`) | 13 (143) | **5.59x** [4.09x-7.64x] | **6.23x** [4.31x-9.01x] | 552.4 (37.2) | 1750.7 (152.5) | 6.01x [0.58x-43.32x] |
| Tier 3: Large MILPs (`145 <= N <= 4036`) | 8 (40) | **3.80x** [2.09x-6.89x] | **4.14x** [2.20x-7.80x] | 908.0 (175.4) | 2556.2 (519.0) | 3.77x [0.39x-14.27x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **4.61x** [3.14x-6.76x] | **4.88x** [3.20x-7.45x] | 1132.9 (164.2) | 2961.0 (381.9) | 5.29x [0.58x-43.32x] |
| All 31 synthetic families | 31 (293) | **7.90x** [5.31x-11.74x] | **10.49x** [6.19x-17.77x] | 1469.2 (213.4) | 4459.8 (686.0) | 7.42x [0.39x-137.01x] |
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
| `t1_std_lp` | 3 | 3 | **854.9x** [572.2x-1015.7x] | 0.01 | 6.81 | 0.00 | 6.75 | 0.004 | 0.064 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **1028.3x** [703.5x-1452.3x] | 0.01 | 6.83 | 0.00 | 6.76 | 0.003 | 0.066 | `OK` |
| `t1_std_mip` | 3 | 3 | **826.8x** [443.3x-1026.5x] | 0.01 | 7.49 | 0.00 | 7.43 | 0.003 | 0.055 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **925.8x** [755.2x-1041.2x] | 0.01 | 7.30 | 0.00 | 7.24 | 0.003 | 0.055 | `OK` |
| `t1_whiskas` | 6 | 5 | **581.5x** [408.9x-725.9x] | 0.01 | 6.64 | 0.01 | 6.56 | 0.005 | 0.078 | `OK` |
| `t1_furniture` | 2 | 3 | **299.8x** [77.7x-1161.5x] | 0.02 | 8.16 | 0.02 | 8.10 | 0.003 | 0.060 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **101.4x** [46.8x-326.5x] | 0.11 | 9.28 | 0.10 | 9.17 | 0.008 | 0.098 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **1323.6x** [730.6x-1803.1x] | 0.00 | 6.41 | 0.00 | 6.36 | 0.004 | 0.044 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **109.5x** [63.5x-140.9x] | 0.08 | 9.95 | 0.05 | 9.71 | 0.030 | 0.239 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **64.1x** [40.4x-117.0x] | 0.50 | 34.20 | 0.48 | 33.96 | 0.027 | 0.244 | `OK` |
| `t2_mkp_30` | 30 | 6 | **10.3x** [5.2x-14.7x] | 2.47 | 28.60 | 2.45 | 28.42 | 0.021 | 0.187 | `OK` |
| `t2_mkp_50` | 50 | 8 | **4.9x** [2.5x-27.1x] | 5.28 | 17.40 | 5.25 | 17.14 | 0.033 | 0.264 | `OK` |
| `t2_cflp_36` | 36 | 11 | **52.7x** [22.3x-155.9x] | 0.19 | 9.14 | 0.17 | 8.96 | 0.019 | 0.173 | `OK` |
| `t2_cflp_49` | 49 | 13 | **35.8x** [20.1x-94.6x] | 0.35 | 10.64 | 0.33 | 10.40 | 0.024 | 0.225 | `OK` |
| `t2_gap_45` | 45 | 13 | **18.3x** [4.5x-53.7x] | 0.90 | 15.27 | 0.88 | 15.05 | 0.026 | 0.220 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **11.8x** [5.2x-31.2x] | 3.69 | 70.29 | 3.66 | 70.04 | 0.031 | 0.251 | `OK` |
| `t2_binpack_44` | 44 | 14 | **13.0x** [5.0x-24.3x] | 1.04 | 14.07 | 1.01 | 13.87 | 0.023 | 0.213 | `OK` |
| `t2_setcover_50` | 50 | 20 | **38.2x** [6.3x-100.7x] | 0.24 | 11.68 | 0.17 | 11.37 | 0.064 | 0.319 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **57.2x** [51.8x-64.4x] | 0.17 | 10.16 | 0.14 | 9.84 | 0.034 | 0.311 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **57.0x** [43.0x-67.9x] | 0.18 | 11.11 | 0.13 | 10.66 | 0.049 | 0.452 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **27.4x** [15.7x-37.2x] | 0.43 | 12.06 | 0.33 | 11.43 | 0.099 | 0.647 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.3x** [3.5x-24.4x] | 3.32 | 48.53 | 3.26 | 48.21 | 0.048 | 0.316 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **6.9x** [1.4x-29.1x] | 18.92 | 171.45 | 18.79 | 171.02 | 0.065 | 0.453 | `OK` |
| `t3_mkp_200` | 200 | 8 | **4.2x** [3.1x-5.4x] | 25.48 | 117.75 | 25.40 | 117.12 | 0.096 | 0.652 | `OK` |
| `t3_cflp_300` | 300 | 34 | **31.1x** [7.7x-63.3x] | 6.44 | 257.21 | 6.34 | 256.24 | 0.106 | 0.953 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **2.5x** [1.5x-4.5x] | 87.57 | 159.28 | 87.32 | 157.37 | 0.215 | 1.982 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **8.6x** [3.9x-14.7x] | 8.30 | 81.73 | 7.95 | 78.15 | 0.333 | 2.927 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **30.4x** [24.3x-37.7x] | 0.60 | 18.64 | 0.46 | 17.59 | 0.134 | 1.054 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **18.5x** [11.7x-25.6x] | 1.06 | 19.93 | 0.85 | 18.12 | 0.210 | 1.873 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.6x** [1.0x-13.4x] | 8.43 | 61.42 | 7.43 | 53.44 | 1.394 | 8.573 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **10.0x** [4.4x-23.8x] | 37.54 | 379.67 | 30.98 | 344.26 | 4.473 | 36.012 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

## 4. Scalar vs. `GOEXPERIMENT=simd` Build

Ratios are geometric means of per-trial `scalar / SIMD` solve times over the same instances, with 95% t-intervals on the log ratios. "Reachable" is a necessary condition only: the AVX2 `Dot64`/`Axpy64`/`Scale64` paths need a slice of length at least 32, which requires `n + m >= 32`.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 2 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 3 faster, 2 slower, 18 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 1.04x [1.00x-1.09x] | 0.004 | 0.003 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 1.01x [0.98x-1.04x] | 0.003 | 0.003 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 1.01x [0.97x-1.05x] | 0.005 | 0.005 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source | 0.97x [0.85x-1.11x] | 0.004 | 0.004 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.97x [0.95x-0.98x] | 0.005 | 0.005 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 0.97x [0.94x-1.01x] | 0.016 | 0.016 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.94x [0.91x-0.97x] | 0.099 | 0.102 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 1.03x [0.99x-1.07x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 1.04x [0.99x-1.09x] | 0.048 | 0.049 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 1.01x [0.99x-1.04x] | 0.476 | 0.464 | 11 |
| `t2_mkp_30` | 36 | possible | SIMD slower | 0.94x [0.88x-1.00x] | 2.452 | 2.435 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 1.01x [0.99x-1.03x] | 5.252 | 5.288 | 11 |
| `t2_cflp_36` | 47 | possible | no detectable effect | 0.99x [0.96x-1.01x] | 0.165 | 0.154 | 11 |
| `t2_cflp_49` | 62 | possible | SIMD slower | 0.96x [0.94x-0.98x] | 0.328 | 0.333 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 1.03x [0.97x-1.09x] | 0.881 | 0.884 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 0.97x [0.92x-1.02x] | 3.661 | 4.607 | 11 |
| `t2_binpack_44` | 58 | possible | no detectable effect | 1.02x [0.98x-1.06x] | 1.015 | 0.981 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 1.01x [0.90x-1.13x] | 0.169 | 0.184 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 1.06x [0.97x-1.15x] | 0.138 | 0.132 | 11 |
| `t2_mad_g3_i9` | 108 | possible | no detectable effect | 1.01x [0.97x-1.05x] | 0.130 | 0.130 | 11 |
| `t2_mad_g4_i12` | 156 | possible | SIMD faster | 1.15x [1.02x-1.29x] | 0.329 | 0.302 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.01x [0.98x-1.04x] | 3.263 | 3.116 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 0.99x [0.97x-1.01x] | 18.787 | 17.700 | 11 |
| `t3_mkp_200` | 208 | possible | SIMD faster | 1.07x [1.05x-1.09x] | 25.400 | 23.128 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.06x [0.97x-1.14x] | 6.336 | 6.242 | 5 |
| `t3_lotsizing_504` | 840 | possible | no detectable effect | 1.03x [0.99x-1.07x] | 87.316 | 82.188 | 5 |
| `t3_sched_1000` | 1100 | possible | SIMD faster | 1.03x [1.00x-1.05x] | 7.954 | 7.625 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 1.01x [0.94x-1.09x] | 0.458 | 0.444 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 0.90x [0.68x-1.20x] | 0.854 | 0.950 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 0.98x [0.95x-1.01x] | 7.431 | 7.430 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | no detectable effect | 1.05x [0.98x-1.13x] | 30.976 | 28.595 | 5 |
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
| `t1_whiskas` | **0.01** (1 / 0) | 0.00 (1 / 0) | 0.01 (1) | 0.00 (1) | 0.004 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.02 (1) | 0.02 (1) | 0.003 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.10** (1 / 8) | 0.12 (2 / 8) | 0.16 (18) | 0.26 (35) | 0.008 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.05** (1 / 0) | 0.06 (1 / 0) | 0.07 (1) | 0.62 (4) | 0.091 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.48** (2 / 0) | 0.59 (12 / 0) | 0.58 (12) | 0.82 (109) | 0.111 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.45** (5 / 9) | 3.44 (40 / 9) | 1.25 (51) | 1.14 (61) | 0.034 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **5.25** (1 / 4) | 15.34 (2 / 4) | 5.81 (118) | 5.56 (111) | 0.044 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.17** (1 / 2) | 0.26 (3 / 2) | 0.20 (7) | 0.19 (7) | 0.026 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.33** (1 / 10) | 0.70 (4 / 10) | 0.35 (9) | 0.31 (9) | 0.036 | 909.02 | 827.30 |
| `t2_gap_45` | **0.88** (1 / 9) | 0.42 (1 / 9) | 1.90 (41) | 2.10 (60) | 0.031 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **3.66** (2 / 37) | 7.42 (25 / 37) | 3.27 (162) | 3.87 (199) | 0.069 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **1.01** (2 / 6) | 1.51 (5 / 6) | 9.38 (772) | 30.25 (1941) | 0.032 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.17** (1 / 6) | 0.36 (2 / 6) | 0.73 (6) | 0.68 (6) | 0.032 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.14** (1 / 0) | 0.93 (8 / 0) | 0.78 (8) | 0.78 (9) | 0.111 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.13** (1 / 0) | 0.14 (2 / 0) | 0.14 (2) | 3.27 (13) | 0.261 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.33** (1 / 0) | 3.53 (22 / 0) | 3.20 (22) | 5.63 (23) | 0.378 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.26** (43 / 0) | 3.10 (112 / 0) | 3.21 (112) | 3.96 (45) | 0.159 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **18.79** (196 / 0) | 33.16 (188 / 0) | 34.70 (188) | 97.46 (539) | 0.299 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **25.40** (250 / 14) | 113.05 (1135 / 16) | 230.90 (5000, `NodeLimit`) | 212.14 (5000, `NodeLimit`) | 0.136 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **6.34** (1 / 28) | 19.82 (54 / 34) | 9.46 (116) | 9.62 (118) | 0.183 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **87.32** (2 / 206) | 123.28 (376 / 206) | 250.13 (2785, `TimeLimit`) | 250.08 (2797, `TimeLimit`) | 0.735 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **7.95** (31 / 55) | 28.64 (87 / 53) | 250.12 (1684, `TimeLimit`) | 250.02 (1702, `TimeLimit`) | 0.235 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.46** (1 / 0) | 3.56 (17 / 0) | 3.26 (17) | 0.53 (1) | 0.653 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.85** (1 / 0) | 9.17 (58 / 0) | 9.35 (58) | 14.50 (74) | 1.673 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.43** (2 / 0) | 22.54 (25 / 0) | 20.52 (25) | 250.30 (145, `TimeLimit`) | 2.645 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **30.98** (1 / 0) | 33.93 (2 / 0) | 32.19 (2) | 250.34 (6, `TimeLimit`) | 58.832 | 208000.00 | 208000.00 |
<!-- END GENERATED:FULL_CHECKPOINT_TABLE -->

## 6. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus (`testdata/assign_corpus/manifest.json`; 11 randomized trials per family for Tiers 1-2 and 5 for Tier 3, 200 instances in total) compares pure-Go `assign.LinearSumFlat` against SciPy's C++ `rectangular_lsap` extension in warm state on bit-identical `float64` cost matrices, timing `linear_sum_assignment` without post-solve NumPy array gather overhead.

### 6.1 Aggregate LSAP Tier Summary

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `SciPy Solve / goblin` GeoMean [Clustered 95% CI] | Pooled Trial `SciPy Solve / goblin` | Equal-Family `SciPy Total / goblin` | `goblin` Solve Trial (Med) Sum (ms) | SciPy Solve Trial (Med) Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.30x** [1.06x-1.61x] | 1.30x | 3.89x | 0.237 (0.0207) | 0.322 (0.0295) | 1.36x [0.29x-2.85x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **2.94x** [1.88x-4.61x] | 2.94x | 2.98x | 9.236 (0.829) | 32.796 (2.964) | 2.62x [0.96x-15.04x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.88x** [1.85x-4.50x] | 2.88x | 3.76x | 225.633 (44.535) | 997.660 (196.043) | 2.59x [1.45x-6.13x] |
| Square families ($m = n$) | 16 (146) | **2.63x** [1.90x-3.65x] | 2.54x | 3.69x | 230.249 (44.510) | 1019.740 (197.057) | 2.30x [0.29x-15.04x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.71x** [1.17x-2.51x] | 1.59x | 2.90x | 4.856 (0.874) | 11.038 (1.980) | 1.88x [0.62x-2.68x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.28x** [1.17x-4.45x] | 2.42x | 2.91x | 36.574 (6.434) | 65.666 (10.929) | 1.81x [1.00x-15.04x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.34x** [1.81x-3.03x] | 2.24x | 3.45x | 235.106 (45.384) | 1030.778 (199.037) | 2.01x [0.29x-15.04x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 6.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.29x** | 0.0006 | 0.0009 | 0.0007 | 0.0122 | $0.0010 \pm 0.0010$ | $0.0009 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.17x** | 0.0018 | 0.0024 | 0.0021 | 0.0102 | $0.0022 \pm 0.0011$ | $0.0024 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.63x** | 0.0038 | 0.0064 | 0.0055 | 0.0156 | $0.0039 \pm 0.0011$ | $0.0065 \pm 0.0020$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.09x** | 0.0037 | 0.0040 | 0.0046 | 0.0142 | $0.0036 \pm 0.0008$ | $0.0039 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.08x** | 0.0043 | 0.0044 | 0.0059 | 0.0151 | $0.0042 \pm 0.0011$ | $0.0044 \pm 0.0009$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.69x** | 0.0065 | 0.0114 | 0.0106 | 0.0338 | $0.0066 \pm 0.0014$ | $0.0111 \pm 0.0020$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.69x** | 0.0293 | 0.0731 | 0.0356 | 0.0990 | $0.0274 \pm 0.0067$ | $0.0722 \pm 0.0110$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.17x** | 0.0546 | 0.174 | 0.0747 | 0.224 | $0.0553 \pm 0.0058$ | $0.176 \pm 0.0254$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.09x** | 0.0474 | 0.0983 | 0.0702 | 0.148 | $0.0481 \pm 0.0114$ | $0.0995 \pm 0.0146$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.71x** | 0.136 | 0.228 | 0.143 | 0.267 | $0.146 \pm 0.0480$ | $0.244 \pm 0.0456$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **11.58x** | 0.0619 | 0.692 | 0.0775 | 0.776 | $0.0604 \pm 0.0103$ | $0.689 \pm 0.0192$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.90x** | 0.0357 | 0.0687 | 0.0596 | 0.141 | $0.0382 \pm 0.0105$ | $0.0713 \pm 0.0103$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **1.96x** | 0.0398 | 0.0780 | 0.0687 | 0.153 | $0.0406 \pm 0.0036$ | $0.0795 \pm 0.0068$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.29x** | 0.0837 | 0.279 | 0.117 | 0.358 | $0.0863 \pm 0.0140$ | $0.283 \pm 0.0327$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.78x** | 0.340 | 1.273 | 0.452 | 1.682 | $0.337 \pm 0.0557$ | $1.268 \pm 0.122$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.05x** | 1.255 | 5.293 | 1.727 | 7.899 | $1.333 \pm 0.208$ | $5.363 \pm 0.394$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.75x** | 0.872 | 1.513 | 1.426 | 4.728 | $0.880 \pm 0.0579$ | $1.542 \pm 0.115$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.62x** | 5.306 | 8.379 | 5.547 | 9.337 | $5.851 \pm 2.168$ | $9.282 \pm 2.724$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.47x** | 0.358 | 0.818 | 0.730 | 3.794 | $0.344 \pm 0.0331$ | $0.846 \pm 0.0628$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.31x** | 0.433 | 1.007 | 0.858 | 4.289 | $0.437 \pm 0.0237$ | $1.011 \pm 0.0505$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **5.04x** | 5.983 | 29.840 | 8.117 | 42.521 | $5.877 \pm 0.362$ | $29.641 \pm 2.554$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **5.01x** | 30.328 | 149.192 | 41.063 | 194.432 | $30.404 \pm 3.667$ | $151.847 \pm 14.537$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 6.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSkip64`, `InitReducedRow64`, and `Min2ReducedRow64`)

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` with no `>= 8x8` tall transpose and either fused square `Minimize` non-sparse path or `rows * cols < 32`, so no AVX2 kernel runs): 1 of 3 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32`, tall `rows > cols` with `>= 8x8`, or fallback `AllAbsLE64`/`ValidateAndNegate64` at `rows * cols >= 32`): 15 faster, 1 slower, 3 with no detectable effect, out of 19.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.86x [0.82x-0.91x] | 0.0006 | 0.0007 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source | 1.01x [0.98x-1.04x] | 0.0018 | 0.0017 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source | 0.95x [0.82x-1.10x] | 0.0038 | 0.0040 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | no detectable effect | 0.96x [0.83x-1.11x] | 0.0037 | 0.0036 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | no detectable effect | 1.03x [0.95x-1.12x] | 0.0043 | 0.0041 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | SIMD slower | 0.91x [0.83x-1.00x] | 0.0065 | 0.0072 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.26x [1.23x-1.29x] | 0.0293 | 0.0236 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | no detectable effect | 1.06x [0.64x-1.74x] | 0.0546 | 0.0422 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.58x [1.43x-1.73x] | 0.0474 | 0.0301 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.65x [1.57x-1.72x] | 0.136 | 0.0834 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.25x [1.18x-1.33x] | 0.0619 | 0.0486 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.27x [1.22x-1.32x] | 0.0357 | 0.0287 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.31x [1.27x-1.34x] | 0.0398 | 0.0313 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.32x [1.23x-1.41x] | 0.0837 | 0.0598 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.54x [1.48x-1.60x] | 0.340 | 0.212 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.73x [1.56x-1.93x] | 1.255 | 0.731 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.38x [2.00x-2.83x] | 0.872 | 0.331 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.55x [2.42x-2.68x] | 5.306 | 2.088 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.66x [1.51x-1.84x] | 0.358 | 0.214 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.59x [1.51x-1.68x] | 0.433 | 0.263 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.61x [1.44x-1.81x] | 5.983 | 3.526 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.51x [1.45x-1.56x] | 30.328 | 20.412 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

## 7. Production Binary Footprint and Linker Dead-Code Elimination

Standalone `CGO_ENABLED=0` binaries compiled across the four canonical `goblin/assign` and `goblin` build profiles (`bench/results/goblin_binary_size.json`), verifying that unreferenced `WithCertified`, `WithVIPRTrace`, `encoding/json`, and (in `goblin/assign`) `math/big` symbols are eliminated by the Go linker:

<!-- BEGIN GENERATED:BINARY_SIZE_TABLE -->
| Profile | Default Raw / Stripped (KiB) | `GOEXPERIMENT=simd` Raw / Stripped (KiB) | Stripped Delta vs. Default | `goblin` Text (KiB) | `math/big` Syms | `encoding/json` Syms | `VIPR` Syms | `Certif` Syms |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `assign` default (`LinearSum` + `SolveInto`) | 2822.8 / **1768.2** | 2858.1 / **1832.2** | baseline | 118.7 | `0` | `0` | `0` | `0` |
| `assign` certified (`+ assign.WithCertified(true)`) | 2942.9 / **1844.2** | 2980.7 / **1916.2** | +76.0 KiB (+4.3%) | 177.2 | `0` | `0` | `0` | `17` |
| `goblin` default (`Model.Solve`, `DefaultOptions`) | 5566.4 / **3548.2** | 5457.5 / **3568.2** | baseline | 1016.6 | `130` | `0` | `0` | `0` |
| `goblin` certified + VIPR (`+ WithCertified + WithVIPRTrace`) | 6821.4 / **4392.2** | 6679.1 / **4408.2** | +844.0 KiB (+23.8%) | 1496.3 | `155` | `0` | `220` | `14` |
<!-- END GENERATED:BINARY_SIZE_TABLE -->
