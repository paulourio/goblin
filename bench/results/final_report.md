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
- **Unified telemetry source**: fingerprint `f88870a48211db9c` (commit `ad55a4dcc677a8239024a9ebbf50765fabeae902`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.45 ms median on a 1-variable MILP and 6.16 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

## 2. Aggregate Workload Tier Summary

Tier 1 micro-models are dominated by Python subprocess spawn overhead, and the planted-gap MAD family is constructed so that root heuristics close the gap without branching; the bold row isolates solver-dominated Tier 2 and Tier 3 non-planted MILPs across paired trial instances.

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `Py / Go` GeoMean [Clustered 95% CI] | Pooled Trial `Py / Go` | Equal-Family `Go / CBC` | `Go` Trial (Med) Sum (ms) | `CBC` Trial (Med) Sum (ms) | `Py` Trial (Med) Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **318.14x** [153.03x-661.40x] | 318.14x | 0.05x | 8.8 (0.8) | 190.8 (18.9) | 1045.0 (97.8) | 369.25x [39.20x-3099.07x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **16.22x** [8.52x-30.88x] | 16.22x | 0.18x | 810.8 (48.0) | 1931.8 (169.0) | 4259.5 (411.0) | 22.46x [0.11x-150.49x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **9.51x** [4.59x-19.74x] | 9.51x | 0.22x | 896.9 (162.9) | 2853.3 (549.6) | 6160.1 (1070.5) | 9.55x [1.03x-71.02x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **10.34x** [5.69x-18.78x] | 11.14x | 0.25x | 1389.0 (165.5) | 3080.8 (390.4) | 6702.4 (978.7) | 11.24x [0.11x-150.49x] |
| Tier 3 non-MAD MILPs | 4 (20) | **6.90x** [1.38x-34.58x] | 6.90x | 0.31x | 586.9 (118.2) | 1234.6 (228.7) | 2786.5 (599.0) | 5.28x [1.47x-71.02x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **27.61x** [11.84x-64.38x] | 36.51x | 0.11x | 319.5 (45.5) | 1728.8 (330.6) | 3818.2 (512.1) | 35.79x [1.03x-171.31x] |
| Random-weight MAD (negative controls) | 3 (33) | **15.58x** [0.73x-330.26x] | 15.58x | 0.15x | 322.8 (21.4) | 1191.3 (114.6) | 2217.7 (243.6) | 15.14x [1.24x-121.34x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **40.84x** [17.02x-98.03x] | 49.21x | 0.13x | 1397.1 (166.2) | 3247.1 (406.8) | 7646.4 (1067.2) | 41.99x [0.11x-3099.07x] |
| All 31 synthetic corpus families | 31 (293) | **36.92x** [19.01x-71.71x] | 46.10x | 0.13x | 1716.6 (211.6) | 4975.9 (737.5) | 11464.6 (1579.3) | 40.19x [0.11x-3099.07x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2.1 Pure-Go vs. In-Process C++ CBC Aggregate Summary

Comparing `BackendGo` directly against in-process `BackendCBC` removes Python MPS formatting and subprocess spawn overhead across the same five workload subsets:

<!-- BEGIN GENERATED:CBC_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `BackendCBC / BackendGo` Total [Clustered 95% CI] | Equal-Family `BackendCBC / BackendGo` Solve [Clustered 95% CI] | `BackendGo` Trial (Med) Sum (ms) | `BackendCBC` Trial (Med) Sum (ms) | Per-Instance `CBC / Go` Total Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro-models (`N <= 26`) | 10 (110) | **20.29x** [7.73x-53.22x] | **39.71x** [13.17x-119.70x] | 8.8 (0.8) | 190.8 (18.9) | 30.91x [1.01x-148.14x] |
| Tier 2: Medium MILPs (`30 <= N <= 84`) | 13 (143) | **5.43x** [3.34x-8.83x] | **6.02x** [3.51x-10.33x] | 810.8 (48.0) | 1931.8 (169.0) | 6.90x [0.07x-43.88x] |
| Tier 3: Large MILPs (`145 <= N <= 4036`) | 8 (40) | **4.50x** [2.44x-8.33x] | **4.95x** [2.51x-9.74x] | 896.9 (162.9) | 2853.3 (549.6) | 5.58x [0.56x-16.85x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **4.06x** [2.56x-6.46x] | **4.25x** [2.60x-6.94x] | 1389.0 (165.5) | 3080.8 (390.4) | 4.87x [0.07x-43.88x] |
| All 31 synthetic families | 31 (293) | **7.91x** [5.15x-12.16x] | **10.52x** [6.21x-17.81x] | 1716.6 (211.6) | 4975.9 (737.5) | 8.45x [0.07x-148.14x] |
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
| `t1_std_lp` | 3 | 3 | **428.0x** [115.2x-665.9x] | 0.01 | 5.92 | 0.00 | 5.87 | 0.006 | 0.045 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **455.2x** [173.9x-948.5x] | 0.01 | 6.07 | 0.00 | 6.03 | 0.007 | 0.047 | `OK` |
| `t1_std_mip` | 3 | 3 | **517.6x** [326.1x-894.1x] | 0.01 | 7.23 | 0.01 | 7.18 | 0.004 | 0.059 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **555.2x** [274.4x-736.3x] | 0.01 | 7.04 | 0.01 | 6.99 | 0.004 | 0.052 | `OK` |
| `t1_whiskas` | 6 | 5 | **320.5x** [118.9x-684.6x] | 0.02 | 6.31 | 0.01 | 6.22 | 0.007 | 0.075 | `OK` |
| `t1_furniture` | 2 | 3 | **273.1x** [79.1x-831.8x] | 0.02 | 7.59 | 0.02 | 7.54 | 0.003 | 0.049 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **96.3x** [45.1x-321.5x] | 0.11 | 8.69 | 0.10 | 8.61 | 0.008 | 0.088 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **2373.6x** [1377.9x-3099.1x] | 0.00 | 6.37 | 0.00 | 6.32 | 0.002 | 0.049 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **149.2x** [108.7x-171.3x] | 0.06 | 9.28 | 0.04 | 9.06 | 0.020 | 0.215 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **63.5x** [39.2x-121.3x] | 0.51 | 33.29 | 0.48 | 33.06 | 0.025 | 0.237 | `OK` |
| `t2_mkp_30` | 30 | 6 | **10.2x** [5.4x-14.2x] | 2.38 | 27.16 | 2.37 | 26.99 | 0.017 | 0.175 | `OK` |
| `t2_mkp_50` | 50 | 8 | **4.9x** [2.2x-29.8x] | 5.00 | 16.42 | 4.97 | 16.12 | 0.028 | 0.262 | `OK` |
| `t2_cflp_36` | 36 | 11 | **51.6x** [17.2x-150.5x] | 0.18 | 9.21 | 0.16 | 9.03 | 0.019 | 0.171 | `OK` |
| `t2_cflp_49` | 49 | 13 | **36.3x** [21.7x-96.0x] | 0.32 | 10.07 | 0.30 | 9.87 | 0.022 | 0.199 | `OK` |
| `t2_gap_45` | 45 | 13 | **19.8x** [4.0x-62.8x] | 0.82 | 14.23 | 0.80 | 14.02 | 0.023 | 0.200 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **10.7x** [4.8x-23.7x] | 4.55 | 67.80 | 4.52 | 67.57 | 0.030 | 0.246 | `OK` |
| `t2_binpack_44` | 44 | 14 | **1.7x** [0.1x-24.7x] | 12.89 | 13.75 | 12.87 | 13.53 | 0.025 | 0.201 | `OK` |
| `t2_setcover_50` | 50 | 20 | **35.0x** [6.0x-121.6x] | 0.27 | 10.72 | 0.24 | 10.41 | 0.049 | 0.306 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **53.1x** [35.6x-61.5x] | 0.17 | 8.91 | 0.13 | 8.66 | 0.033 | 0.276 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **44.0x** [14.9x-63.3x] | 0.21 | 10.04 | 0.14 | 9.63 | 0.047 | 0.408 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **32.7x** [27.3x-36.4x] | 0.36 | 12.38 | 0.29 | 11.73 | 0.069 | 0.619 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.1x** [3.0x-26.0x] | 3.18 | 46.65 | 3.15 | 46.35 | 0.040 | 0.305 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **6.5x** [1.2x-26.4x] | 17.66 | 163.63 | 17.57 | 163.14 | 0.060 | 0.443 | `OK` |
| `t3_mkp_200` | 200 | 8 | **4.0x** [3.1x-5.3x] | 24.12 | 114.76 | 24.04 | 114.15 | 0.079 | 0.617 | `OK` |
| `t3_cflp_300` | 300 | 34 | **25.9x** [7.9x-71.0x] | 9.57 | 248.09 | 9.42 | 247.19 | 0.105 | 0.906 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **2.6x** [1.5x-4.7x] | 76.31 | 157.05 | 76.09 | 155.18 | 0.226 | 1.834 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **8.6x** [3.8x-14.6x] | 8.19 | 79.13 | 7.88 | 76.20 | 0.326 | 2.753 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **28.6x** [22.4x-34.1x] | 0.60 | 18.46 | 0.46 | 17.30 | 0.154 | 1.082 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **17.8x** [10.5x-24.5x] | 0.99 | 18.28 | 0.80 | 16.60 | 0.196 | 1.717 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.8x** [1.0x-14.5x] | 8.39 | 60.23 | 7.53 | 52.69 | 1.100 | 8.405 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **10.0x** [4.4x-23.7x] | 34.70 | 374.49 | 30.24 | 340.24 | 4.162 | 34.349 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

## 4. Scalar vs. `GOEXPERIMENT=simd` Build

Ratios are geometric means of per-trial `scalar / SIMD` solve times over the same instances, with 95% t-intervals on the log ratios. "Reachable" is a necessary condition only: the AVX2 `Dot64`/`Axpy64`/`Scale64` paths need a slice of length at least 32, which requires `n + m >= 32`.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 1 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 10 faster, 3 slower, 10 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 0.96x [0.87x-1.06x] | 0.004 | 0.005 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 1.14x [0.98x-1.32x] | 0.004 | 0.004 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 1.07x [0.85x-1.34x] | 0.006 | 0.006 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.92x [0.85x-1.00x] | 0.006 | 0.006 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 0.80x [0.50x-1.26x] | 0.007 | 0.009 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 0.97x [0.92x-1.02x] | 0.016 | 0.017 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source | 1.00x [0.96x-1.04x] | 0.099 | 0.104 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 1.00x [0.95x-1.05x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | SIMD slower | 0.83x [0.80x-0.87x] | 0.038 | 0.046 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 0.98x [0.94x-1.02x] | 0.477 | 0.494 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 0.99x [0.98x-1.01x] | 2.366 | 2.395 | 11 |
| `t2_mkp_50` | 58 | possible | SIMD faster | 1.03x [1.02x-1.05x] | 4.972 | 5.001 | 11 |
| `t2_cflp_36` | 47 | possible | SIMD slower | 0.95x [0.91x-0.99x] | 0.159 | 0.156 | 11 |
| `t2_cflp_49` | 62 | possible | SIMD slower | 0.92x [0.85x-0.99x] | 0.302 | 0.363 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 1.05x [0.99x-1.11x] | 0.798 | 0.806 | 11 |
| `t2_lotsizing_48` | 80 | possible | SIMD faster | 1.07x [1.03x-1.12x] | 4.520 | 4.191 | 11 |
| `t2_binpack_44` | 58 | possible | no detectable effect | 0.99x [0.97x-1.02x] | 12.868 | 12.731 | 11 |
| `t2_setcover_50` | 70 | possible | SIMD faster | 1.09x [1.04x-1.14x] | 0.241 | 0.235 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 1.03x [0.93x-1.13x] | 0.133 | 0.134 | 11 |
| `t2_mad_g3_i9` | 108 | possible | no detectable effect | 1.07x [0.97x-1.18x] | 0.139 | 0.131 | 11 |
| `t2_mad_g4_i12` | 156 | possible | SIMD faster | 1.02x [1.01x-1.04x] | 0.294 | 0.293 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | SIMD faster | 1.03x [1.01x-1.06x] | 3.146 | 2.952 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | SIMD faster | 1.03x [1.01x-1.06x] | 17.568 | 17.600 | 11 |
| `t3_mkp_200` | 208 | possible | SIMD faster | 1.05x [1.02x-1.09x] | 24.041 | 22.373 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.00x [0.89x-1.12x] | 9.425 | 8.483 | 5 |
| `t3_lotsizing_504` | 840 | possible | no detectable effect | 1.04x [0.99x-1.08x] | 76.088 | 74.337 | 5 |
| `t3_sched_1000` | 1100 | possible | SIMD faster | 1.06x [1.02x-1.10x] | 7.877 | 7.251 | 5 |
| `t3_mad_g5_i20` | 240 | possible | SIMD faster | 1.11x [1.09x-1.13x] | 0.461 | 0.410 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 0.99x [0.91x-1.07x] | 0.801 | 0.842 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 0.99x [0.92x-1.07x] | 7.528 | 7.584 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | SIMD faster | 1.11x [1.07x-1.15x] | 30.236 | 28.186 | 5 |
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
| `t1_std_lp` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.01 (1) | 0.00 (1) | 0.002 | 54.00 | 54.00 |
| `t1_std_lp_max` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 80.00 | 80.00 |
| `t1_std_mip` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.003 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.002 | 64.95 | 58.90 |
| `t1_whiskas` | **0.01** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.004 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.003 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.10** (1 / 8) | 0.13 (2 / 8) | 0.16 (18) | 0.24 (35) | 0.008 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.04** (1 / 0) | 0.04 (1 / 0) | 0.05 (1) | 0.51 (4) | 0.081 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.48** (2 / 0) | 0.54 (12 / 0) | 0.59 (12) | 0.83 (109) | 0.107 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.37** (5 / 9) | 3.23 (40 / 9) | 1.23 (51) | 1.08 (61) | 0.030 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **4.97** (1 / 4) | 14.72 (2 / 4) | 6.24 (118) | 5.20 (111) | 0.043 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.16** (1 / 2) | 0.23 (3 / 2) | 0.18 (7) | 0.19 (7) | 0.025 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.30** (1 / 7) | 0.61 (3 / 7) | 0.31 (9) | 0.29 (9) | 0.033 | 909.02 | 827.30 |
| `t2_gap_45` | **0.80** (1 / 7) | 0.41 (1 / 7) | 1.73 (41) | 2.08 (60) | 0.029 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **4.52** (2 / 37) | 8.07 (25 / 37) | 3.27 (162) | 3.84 (199) | 0.075 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **12.87** (274 / 12) | 15.92 (298 / 12) | 9.31 (772) | 30.01 (1941) | 0.029 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.24** (1 / 6) | 0.37 (2 / 6) | 0.68 (6) | 0.71 (6) | 0.033 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.13** (1 / 0) | 0.80 (8 / 0) | 0.75 (8) | 0.77 (9) | 0.107 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.14** (1 / 0) | 0.12 (2 / 0) | 0.12 (2) | 2.71 (13) | 0.261 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.29** (1 / 0) | 3.48 (22 / 0) | 3.12 (22) | 5.56 (23) | 0.353 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.15** (43 / 0) | 3.12 (112 / 0) | 3.10 (112) | 3.92 (45) | 0.162 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **17.57** (196 / 0) | 34.69 (188 / 0) | 32.98 (188) | 99.02 (539) | 0.308 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **24.04** (250 / 14) | 107.67 (1135 / 16) | 230.47 (5000, `NodeLimit`) | 210.36 (5000, `NodeLimit`) | 0.147 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **9.42** (1 / 28) | 19.78 (54 / 34) | 9.87 (116) | 9.23 (118) | 0.175 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **76.09** (2 / 206) | 101.73 (376 / 206) | 250.06 (2902, `TimeLimit`) | 250.02 (3005, `TimeLimit`) | 0.773 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **7.88** (31 / 55) | 26.42 (87 / 53) | 250.04 (1678, `TimeLimit`) | 250.11 (1706, `TimeLimit`) | 0.225 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.46** (1 / 0) | 3.06 (17 / 0) | 2.80 (17) | 0.52 (1) | 0.661 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.80** (1 / 0) | 9.48 (58 / 0) | 8.27 (58) | 14.25 (74) | 1.511 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.53** (2 / 0) | 22.34 (25 / 0) | 20.78 (25) | 250.47 (145, `TimeLimit`) | 2.627 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **30.24** (1 / 0) | 32.85 (2 / 0) | 31.47 (2) | 250.71 (6, `TimeLimit`) | 57.320 | 208000.00 | 208000.00 |
<!-- END GENERATED:FULL_CHECKPOINT_TABLE -->

## 6. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus (`testdata/assign_corpus/manifest.json`; 11 randomized trials per family for Tiers 1-2 and 5 for Tier 3, 200 instances in total) compares pure-Go `assign.LinearSumFlat` against SciPy's C++ `rectangular_lsap` extension in warm state on bit-identical `float64` cost matrices, timing `linear_sum_assignment` without post-solve NumPy array gather overhead.

### 6.1 Aggregate LSAP Tier Summary

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `SciPy Solve / goblin` GeoMean [Clustered 95% CI] | Pooled Trial `SciPy Solve / goblin` | Equal-Family `SciPy Total / goblin` | `goblin` Solve Trial (Med) Sum (ms) | SciPy Solve Trial (Med) Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.25x** [0.99x-1.58x] | 1.25x | 3.99x | 0.253 (0.0202) | 0.311 (0.0284) | 1.41x [0.10x-2.49x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **2.89x** [1.80x-4.64x] | 2.89x | 2.96x | 9.763 (0.826) | 32.756 (2.975) | 2.46x [0.93x-17.79x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.85x** [1.86x-4.38x] | 2.85x | 3.60x | 220.156 (43.575) | 923.240 (185.591) | 2.57x [1.47x-5.80x] |
| Square families ($m = n$) | 16 (146) | **2.53x** [1.79x-3.58x] | 2.44x | 3.61x | 225.537 (43.592) | 945.518 (186.684) | 2.09x [0.10x-17.79x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.75x** [1.17x-2.60x] | 1.62x | 2.96x | 4.635 (0.829) | 10.789 (1.911) | 1.95x [0.68x-2.88x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.21x** [1.09x-4.47x] | 2.33x | 2.75x | 35.802 (6.392) | 64.181 (10.726) | 1.76x [0.91x-17.79x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.29x** [1.75x-2.99x] | 2.19x | 3.42x | 230.172 (44.421) | 956.307 (188.595) | 1.98x [0.10x-17.79x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 6.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.33x** | 0.0005 | 0.0008 | 0.0006 | 0.0107 | $0.0010 \pm 0.0011$ | $0.0010 \pm 0.0006$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **0.93x** | 0.0018 | 0.0025 | 0.0025 | 0.0101 | $0.0036 \pm 0.0050$ | $0.0024 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.56x** | 0.0037 | 0.0061 | 0.0054 | 0.0149 | $0.0039 \pm 0.0012$ | $0.0060 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.11x** | 0.0037 | 0.0040 | 0.0053 | 0.0144 | $0.0038 \pm 0.0012$ | $0.0040 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.07x** | 0.0042 | 0.0043 | 0.0057 | 0.0150 | $0.0042 \pm 0.0012$ | $0.0044 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.64x** | 0.0063 | 0.0107 | 0.0111 | 0.0327 | $0.0066 \pm 0.0020$ | $0.0105 \pm 0.0018$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.64x** | 0.0285 | 0.0704 | 0.0346 | 0.0968 | $0.0264 \pm 0.0066$ | $0.0682 \pm 0.0105$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.20x** | 0.0560 | 0.179 | 0.0717 | 0.231 | $0.0556 \pm 0.0047$ | $0.179 \pm 0.0267$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **1.87x** | 0.0511 | 0.0941 | 0.0759 | 0.145 | $0.0518 \pm 0.0116$ | $0.0962 \pm 0.0141$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.57x** | 0.144 | 0.217 | 0.151 | 0.249 | $0.151 \pm 0.0490$ | $0.231 \pm 0.0447$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **12.36x** | 0.0615 | 0.703 | 0.0791 | 0.780 | $0.0574 \pm 0.0093$ | $0.701 \pm 0.0228$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.91x** | 0.0351 | 0.0706 | 0.0588 | 0.145 | $0.0389 \pm 0.0128$ | $0.0720 \pm 0.0082$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.09x** | 0.0384 | 0.0793 | 0.0626 | 0.153 | $0.0384 \pm 0.0038$ | $0.0802 \pm 0.0085$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.33x** | 0.0815 | 0.274 | 0.110 | 0.350 | $0.0860 \pm 0.0163$ | $0.284 \pm 0.0365$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.49x** | 0.330 | 1.287 | 0.465 | 1.706 | $0.382 \pm 0.168$ | $1.266 \pm 0.139$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.20x** | 1.198 | 5.151 | 1.571 | 7.802 | $1.280 \pm 0.180$ | $5.345 \pm 0.400$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.71x** | 0.876 | 1.487 | 1.501 | 3.951 | $0.868 \pm 0.0498$ | $1.481 \pm 0.0503$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.61x** | 5.250 | 8.208 | 5.484 | 9.737 | $5.696 \pm 1.951$ | $9.057 \pm 2.581$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.57x** | 0.328 | 0.796 | 0.721 | 3.452 | $0.323 \pm 0.0180$ | $0.831 \pm 0.0643$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.34x** | 0.419 | 0.957 | 0.794 | 3.691 | $0.416 \pm 0.0110$ | $0.973 \pm 0.0280$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.65x** | 5.871 | 27.566 | 7.976 | 36.263 | $5.974 \pm 0.435$ | $27.801 \pm 2.231$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.75x** | 29.633 | 141.427 | 38.573 | 182.626 | $29.474 \pm 4.023$ | $139.159 \pm 9.892$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 6.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSkip64`, `InitReducedRow64`, and `Min2ReducedRow64`)

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` with no `>= 8x8` tall transpose and either fused square `Minimize` non-sparse path or `rows * cols < 32`, so no AVX2 kernel runs): 1 of 3 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32`, tall `rows > cols` with `>= 8x8`, or fallback `AllAbsLE64`/`ValidateAndNegate64` at `rows * cols >= 32`): 17 faster, 0 slower, 2 with no detectable effect, out of 19.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.92x [0.88x-0.97x] | 0.0005 | 0.0006 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source | 0.94x [0.81x-1.09x] | 0.0018 | 0.0018 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source | 0.95x [0.84x-1.09x] | 0.0037 | 0.0038 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | no detectable effect | 1.03x [0.88x-1.20x] | 0.0037 | 0.0035 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | SIMD faster | 1.15x [1.05x-1.26x] | 0.0042 | 0.0034 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | no detectable effect | 0.97x [0.90x-1.04x] | 0.0063 | 0.0064 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.24x [1.20x-1.29x] | 0.0285 | 0.0225 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.44x [1.35x-1.54x] | 0.0560 | 0.0390 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.69x [1.31x-2.18x] | 0.0511 | 0.0287 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.82x [1.75x-1.89x] | 0.144 | 0.0810 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.31x [1.25x-1.38x] | 0.0615 | 0.0456 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.34x [1.23x-1.47x] | 0.0351 | 0.0265 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.30x [1.26x-1.34x] | 0.0384 | 0.0287 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.38x [1.33x-1.42x] | 0.0815 | 0.0569 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.77x [1.53x-2.05x] | 0.330 | 0.192 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.60x [1.20x-2.13x] | 1.198 | 0.731 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.46x [2.21x-2.74x] | 0.876 | 0.354 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.64x [2.40x-2.90x] | 5.250 | 1.864 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.78x [1.67x-1.90x] | 0.328 | 0.185 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.55x [1.41x-1.69x] | 0.419 | 0.273 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.66x [1.56x-1.77x] | 5.871 | 3.672 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.55x [1.49x-1.61x] | 29.633 | 18.692 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

## 7. Production Binary Footprint and Linker Dead-Code Elimination

Standalone `CGO_ENABLED=0` binaries compiled across the four canonical `goblin/assign` and `goblin` build profiles (`bench/results/goblin_binary_size.json`), verifying that unreferenced `WithCertified`, `WithVIPRTrace`, `encoding/json`, and (in `goblin/assign`) `math/big` symbols are eliminated by the Go linker:

<!-- BEGIN GENERATED:BINARY_SIZE_TABLE -->
| Profile | Default Raw / Stripped (KiB) | `GOEXPERIMENT=simd` Raw / Stripped (KiB) | Stripped Delta vs. Default | `goblin` Text (KiB) | `math/big` Syms | `encoding/json` Syms | `VIPR` Syms | `Certif` Syms |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `assign` default (`LinearSum` + `SolveInto`) | 2783.7 / **1744.2** | 2820.2 / **1808.2** | baseline | 118.4 | `0` | `0` | `0` | `0` |
| `assign` certified (`+ assign.WithCertified(true)`) | 2912.6 / **1824.2** | 2939.8 / **1888.2** | +80.0 KiB (+4.6%) | 176.5 | `0` | `0` | `0` | `17` |
| `goblin` default (`Model.Solve`, `DefaultOptions`) | 5300.1 / **3372.2** | 5192.7 / **3392.2** | baseline | 899.0 | `130` | `0` | `0` | `0` |
| `goblin` certified + VIPR (`+ WithCertified + WithVIPRTrace`) | 6489.8 / **4180.2** | 6367.2 / **4200.2** | +808.0 KiB (+24.0%) | 1358.4 | `155` | `0` | `212` | `14` |
<!-- END GENERATED:BINARY_SIZE_TABLE -->
