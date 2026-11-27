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
- **Unified telemetry source**: fingerprint `2e436740e97c6e65` (commit `a2bc4d40b277efdcbd17d9195414ae4b25a453c8`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `5`; Python + CBC `5`; Python + SciPy `5`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.65 ms median on a 1-variable MILP and 6.88 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

## 2. Aggregate Workload Tier Summary

Tier 1 micro-models are dominated by Python subprocess spawn overhead, and the planted-gap MAD family is constructed so that root heuristics close the gap without branching; the bold row isolates solver-dominated Tier 2 and Tier 3 non-planted MILPs across paired trial instances.

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `Py / Go` GeoMean [Clustered 95% CI] | Pooled Trial `Py / Go` | Equal-Family `Go / CBC` | `Go` Trial (Med) Sum (ms) | `CBC` Trial (Med) Sum (ms) | `Py` Trial (Med) Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **336.13x** [147.71x-764.90x] | 336.13x | 0.06x | 9.9 (0.9) | 158.3 (14.8) | 1150.4 (106.4) | 454.45x [31.67x-1522.91x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **19.87x** [11.95x-33.04x] | 19.87x | 0.17x | 548.2 (36.3) | 1844.1 (162.8) | 4433.0 (425.5) | 24.35x [1.37x-166.10x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **9.64x** [4.62x-20.09x] | 9.64x | 0.26x | 889.6 (165.5) | 2633.0 (537.5) | 6312.5 (1090.9) | 10.34x [1.12x-60.19x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **12.44x** [7.40x-20.94x] | 13.71x | 0.21x | 1118.7 (154.3) | 3117.3 (405.9) | 6936.5 (1007.6) | 13.09x [1.37x-166.10x] |
| Tier 3 non-MAD MILPs | 4 (20) | **7.35x** [1.29x-41.78x] | 7.35x | 0.28x | 578.8 (118.6) | 1333.7 (248.3) | 2865.1 (614.6) | 5.40x [1.42x-60.19x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **26.44x** [12.08x-57.86x] | 34.88x | 0.17x | 320.2 (47.7) | 1373.1 (295.6) | 3914.0 (518.3) | 38.98x [1.12x-148.52x] |
| Random-weight MAD (negative controls) | 3 (33) | **16.21x** [0.82x-320.54x] | 16.21x | 0.18x | 319.9 (22.1) | 1059.5 (104.0) | 2297.3 (248.4) | 15.45x [1.37x-115.18x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **47.83x** [20.40x-112.16x] | 58.21x | 0.12x | 1127.4 (155.0) | 3262.4 (419.6) | 7982.0 (1104.5) | 41.90x [1.37x-1522.91x] |
| All 31 synthetic corpus families | 31 (293) | **41.05x** [21.47x-78.48x] | 52.05x | 0.13x | 1447.6 (202.7) | 4635.5 (715.1) | 11896.0 (1622.8) | 41.14x [1.12x-1522.91x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2.1 Pure-Go vs. In-Process C++ CBC Aggregate Summary

Comparing `BackendGo` directly against in-process `BackendCBC` removes Python MPS formatting and subprocess spawn overhead across the same five workload subsets:

<!-- BEGIN GENERATED:CBC_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `BackendCBC / BackendGo` Total [Clustered 95% CI] | Equal-Family `BackendCBC / BackendGo` Solve [Clustered 95% CI] | `BackendGo` Trial (Med) Sum (ms) | `BackendCBC` Trial (Med) Sum (ms) | Per-Instance `CBC / Go` Total Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro-models (`N <= 26`) | 10 (110) | **18.06x** [8.24x-39.60x] | **39.45x** [13.23x-117.68x] | 9.9 (0.9) | 158.3 (14.8) | 13.04x [2.92x-146.63x] |
| Tier 2: Medium MILPs (`30 <= N <= 84`) | 13 (143) | **5.90x** [4.32x-8.06x] | **6.61x** [4.56x-9.58x] | 548.2 (36.3) | 1844.1 (162.8) | 6.04x [0.62x-58.38x] |
| Tier 3: Large MILPs (`145 <= N <= 4036`) | 8 (40) | **3.85x** [2.24x-6.63x] | **4.30x** [2.36x-7.84x] | 889.6 (165.5) | 2633.0 (537.5) | 3.89x [0.43x-13.14x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **4.85x** [3.32x-7.10x] | **5.15x** [3.38x-7.84x] | 1118.7 (154.3) | 3117.3 (405.9) | 5.52x [0.62x-58.38x] |
| All 31 synthetic families | 31 (293) | **7.59x** [5.29x-10.88x] | **10.53x** [6.38x-17.37x] | 1447.6 (202.7) | 4635.5 (715.1) | 7.67x [0.43x-146.63x] |
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
| `t1_std_lp` | 3 | 3 | **1036.5x** [695.0x-1251.6x] | 0.01 | 8.11 | 0.00 | 8.03 | 0.004 | 0.078 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **1064.1x** [729.7x-1522.9x] | 0.01 | 7.80 | 0.00 | 7.72 | 0.004 | 0.078 | `OK` |
| `t1_std_mip` | 3 | 3 | **861.7x** [537.2x-1077.3x] | 0.01 | 7.85 | 0.00 | 7.79 | 0.004 | 0.070 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **962.2x** [682.4x-1224.7x] | 0.01 | 8.94 | 0.00 | 8.87 | 0.004 | 0.078 | `OK` |
| `t1_whiskas` | 6 | 5 | **404.7x** [305.5x-540.9x] | 0.02 | 6.53 | 0.01 | 6.45 | 0.008 | 0.079 | `OK` |
| `t1_furniture` | 2 | 3 | **244.7x** [80.3x-611.4x] | 0.02 | 8.25 | 0.02 | 8.19 | 0.006 | 0.057 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **68.0x** [31.7x-157.0x] | 0.16 | 9.29 | 0.13 | 9.18 | 0.018 | 0.096 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **510.2x** [180.3x-1130.3x] | 0.01 | 6.55 | 0.00 | 6.50 | 0.008 | 0.052 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **91.3x** [66.4x-148.5x] | 0.11 | 9.51 | 0.06 | 9.28 | 0.042 | 0.228 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **64.1x** [39.8x-115.2x] | 0.51 | 33.61 | 0.48 | 33.37 | 0.028 | 0.243 | `OK` |
| `t2_mkp_30` | 30 | 6 | **10.4x** [4.7x-15.9x] | 2.40 | 29.03 | 2.38 | 28.84 | 0.020 | 0.181 | `OK` |
| `t2_mkp_50` | 50 | 8 | **5.0x** [2.4x-27.8x] | 5.32 | 17.59 | 5.29 | 17.32 | 0.034 | 0.270 | `OK` |
| `t2_cflp_36` | 36 | 11 | **55.4x** [21.8x-166.1x] | 0.17 | 9.52 | 0.15 | 9.35 | 0.019 | 0.184 | `OK` |
| `t2_cflp_49` | 49 | 13 | **30.4x** [19.1x-76.6x] | 0.45 | 11.22 | 0.40 | 10.99 | 0.039 | 0.225 | `OK` |
| `t2_gap_45` | 45 | 13 | **19.6x** [4.4x-68.1x] | 0.89 | 14.79 | 0.85 | 14.56 | 0.024 | 0.213 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **11.9x** [5.1x-31.9x] | 3.57 | 69.73 | 3.53 | 69.48 | 0.032 | 0.259 | `OK` |
| `t2_binpack_44` | 44 | 14 | **13.3x** [5.4x-25.0x] | 1.03 | 14.74 | 1.01 | 14.54 | 0.025 | 0.208 | `OK` |
| `t2_setcover_50` | 50 | 20 | **40.2x** [5.9x-114.4x] | 0.24 | 11.58 | 0.16 | 11.27 | 0.071 | 0.316 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **60.9x** [56.7x-67.0x] | 0.16 | 9.92 | 0.13 | 9.61 | 0.035 | 0.319 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **50.9x** [36.8x-65.6x] | 0.20 | 10.56 | 0.13 | 10.12 | 0.050 | 0.436 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **33.1x** [30.0x-36.1x] | 0.36 | 12.06 | 0.29 | 11.41 | 0.074 | 0.628 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.5x** [3.7x-26.1x] | 3.07 | 48.08 | 3.03 | 47.74 | 0.041 | 0.328 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **7.0x** [1.4x-30.4x] | 18.49 | 166.68 | 18.37 | 166.24 | 0.066 | 0.458 | `OK` |
| `t3_mkp_200` | 200 | 8 | **4.3x** [3.2x-5.6x] | 24.56 | 118.89 | 24.47 | 118.29 | 0.093 | 0.607 | `OK` |
| `t3_cflp_300` | 300 | 34 | **30.6x** [7.9x-60.2x] | 6.58 | 254.26 | 6.43 | 253.38 | 0.136 | 0.921 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **2.4x** [1.4x-4.4x] | 79.76 | 159.21 | 79.50 | 157.33 | 0.247 | 1.863 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **9.1x** [4.0x-14.0x] | 7.75 | 82.25 | 7.44 | 79.31 | 0.342 | 2.899 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **24.3x** [17.7x-29.0x] | 0.78 | 19.40 | 0.50 | 18.23 | 0.279 | 1.099 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **18.1x** [10.9x-24.0x] | 1.06 | 19.42 | 0.84 | 17.57 | 0.231 | 1.793 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.7x** [1.1x-13.8x] | 8.81 | 63.59 | 7.68 | 55.39 | 1.167 | 8.681 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **10.2x** [4.3x-24.3x] | 36.19 | 373.83 | 30.70 | 336.58 | 4.586 | 36.208 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

## 4. Scalar vs. `GOEXPERIMENT=simd` Build

Ratios are geometric means of per-trial `scalar / SIMD` solve times over the same instances, with 95% t-intervals on the log ratios. "Reachable" is a necessary condition only: the AVX2 `Dot64`/`Axpy64`/`Scale64` paths need a slice of length at least 32, which requires `n + m >= 32`.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 3 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 2 faster, 5 slower, 16 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 1.05x [0.97x-1.14x] | 0.004 | 0.004 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 1.09x [0.99x-1.20x] | 0.003 | 0.003 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 0.98x [0.86x-1.11x] | 0.005 | 0.005 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.92x [0.87x-0.98x] | 0.004 | 0.005 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 0.97x [0.89x-1.06x] | 0.006 | 0.007 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 1.06x [0.98x-1.14x] | 0.018 | 0.017 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.24x [1.07x-1.44x] | 0.135 | 0.107 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.52x [1.10x-2.09x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 1.06x [0.77x-1.44x] | 0.060 | 0.050 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 1.00x [0.97x-1.03x] | 0.478 | 0.480 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 0.93x [0.86x-1.02x] | 2.378 | 2.501 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 1.00x [0.97x-1.03x] | 5.287 | 5.322 | 11 |
| `t2_cflp_36` | 47 | possible | SIMD slower | 0.98x [0.96x-1.00x] | 0.154 | 0.164 | 11 |
| `t2_cflp_49` | 62 | possible | SIMD faster | 1.16x [1.11x-1.22x] | 0.400 | 0.324 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 0.97x [0.88x-1.07x] | 0.849 | 0.859 | 11 |
| `t2_lotsizing_48` | 80 | possible | SIMD slower | 0.93x [0.89x-0.98x] | 3.532 | 3.695 | 11 |
| `t2_binpack_44` | 58 | possible | SIMD slower | 0.96x [0.94x-0.98x] | 1.007 | 1.024 | 11 |
| `t2_setcover_50` | 70 | possible | SIMD faster | 1.08x [1.01x-1.16x] | 0.159 | 0.172 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 0.99x [0.94x-1.03x] | 0.128 | 0.132 | 11 |
| `t2_mad_g3_i9` | 108 | possible | no detectable effect | 0.90x [0.79x-1.02x] | 0.129 | 0.152 | 11 |
| `t2_mad_g4_i12` | 156 | possible | SIMD slower | 0.95x [0.93x-0.98x] | 0.289 | 0.301 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.00x [0.95x-1.05x] | 3.028 | 3.320 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | SIMD slower | 0.97x [0.95x-0.99x] | 18.374 | 17.870 | 11 |
| `t3_mkp_200` | 208 | possible | no detectable effect | 1.01x [0.96x-1.05x] | 24.467 | 24.236 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 0.99x [0.92x-1.06x] | 6.431 | 6.411 | 5 |
| `t3_lotsizing_504` | 840 | possible | no detectable effect | 0.98x [0.93x-1.03x] | 79.504 | 86.022 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 0.94x [0.84x-1.06x] | 7.439 | 9.097 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 0.99x [0.90x-1.08x] | 0.499 | 0.503 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 0.96x [0.90x-1.04x] | 0.835 | 0.833 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 0.97x [0.90x-1.04x] | 7.682 | 7.598 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | no detectable effect | 1.01x [0.94x-1.09x] | 30.704 | 29.187 | 5 |
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
| `t1_mip_float_obj` | **0.00** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.002 | 64.95 | 58.90 |
| `t1_whiskas` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.00 (1) | 0.003 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.003 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.13** (1 / 8) | 0.16 (2 / 8) | 0.20 (18) | 0.30 (35) | 0.013 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.002 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.06** (1 / 0) | 0.04 (1 / 0) | 0.05 (1) | 0.53 (4) | 0.085 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.48** (2 / 0) | 0.57 (12 / 0) | 0.57 (12) | 0.82 (109) | 0.111 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.38** (5 / 9) | 3.40 (40 / 9) | 1.21 (51) | 1.09 (61) | 0.028 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **5.29** (1 / 4) | 15.59 (2 / 4) | 5.96 (118) | 5.43 (111) | 0.047 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.15** (1 / 2) | 0.25 (3 / 2) | 0.18 (7) | 0.19 (7) | 0.026 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.40** (1 / 10) | 0.69 (4 / 10) | 0.34 (9) | 0.32 (9) | 0.038 | 909.02 | 827.30 |
| `t2_gap_45` | **0.85** (1 / 9) | 0.46 (1 / 9) | 1.87 (41) | 2.23 (60) | 0.029 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **3.53** (2 / 37) | 7.29 (25 / 37) | 3.26 (162) | 3.86 (199) | 0.073 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **1.01** (2 / 6) | 1.51 (5 / 6) | 9.43 (772) | 29.91 (1941) | 0.031 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.16** (1 / 6) | 0.37 (2 / 6) | 0.89 (6) | 0.69 (6) | 0.034 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.13** (1 / 0) | 0.89 (8 / 0) | 0.80 (8) | 0.77 (9) | 0.109 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.13** (1 / 0) | 0.17 (2 / 0) | 0.15 (2) | 2.84 (13) | 0.266 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.29** (1 / 0) | 3.47 (22 / 0) | 3.10 (22) | 5.38 (23) | 0.363 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.03** (43 / 0) | 3.21 (112 / 0) | 3.14 (112) | 3.74 (45) | 0.158 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **18.37** (196 / 0) | 35.21 (188 / 0) | 32.98 (188) | 99.62 (539) | 0.284 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **24.47** (250 / 14) | 109.67 (1135 / 16) | 230.55 (5000, `NodeLimit`) | 211.34 (5000, `NodeLimit`) | 0.131 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **6.43** (1 / 28) | 20.02 (54 / 34) | 9.23 (116) | 9.31 (118) | 0.195 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **79.50** (2 / 206) | 119.77 (376 / 206) | 250.04 (2798, `TimeLimit`) | 250.01 (2982, `TimeLimit`) | 0.759 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **7.44** (31 / 55) | 27.79 (87 / 53) | 250.12 (1712, `TimeLimit`) | 250.06 (1727, `TimeLimit`) | 0.225 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.50** (1 / 0) | 3.32 (17 / 0) | 2.86 (17) | 0.55 (1) | 0.699 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.84** (1 / 0) | 8.80 (58 / 0) | 8.40 (58) | 15.11 (74) | 1.502 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.68** (2 / 0) | 22.18 (25 / 0) | 20.78 (25) | 250.49 (145, `TimeLimit`) | 2.686 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **30.70** (1 / 0) | 33.27 (2 / 0) | 32.50 (2) | 250.26 (6, `TimeLimit`) | 58.578 | 208000.00 | 208000.00 |
<!-- END GENERATED:FULL_CHECKPOINT_TABLE -->

## 6. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus (`testdata/assign_corpus/manifest.json`; 11 randomized trials per family for Tiers 1-2 and 5 for Tier 3, 200 instances in total) compares pure-Go `assign.LinearSumFlat` against SciPy's C++ `rectangular_lsap` extension in warm state on bit-identical `float64` cost matrices, timing `linear_sum_assignment` without post-solve NumPy array gather overhead.

### 6.1 Aggregate LSAP Tier Summary

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `SciPy Solve / goblin` GeoMean [Clustered 95% CI] | Pooled Trial `SciPy Solve / goblin` | Equal-Family `SciPy Total / goblin` | `goblin` Solve Trial (Med) Sum (ms) | SciPy Solve Trial (Med) Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.54x** [1.22x-1.94x] | 1.54x | 4.73x | 0.213 (0.0191) | 0.325 (0.0295) | 1.59x [0.74x-2.59x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **2.94x** [1.90x-4.55x] | 2.94x | 2.99x | 9.409 (0.820) | 32.909 (2.988) | 2.62x [0.92x-15.54x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.88x** [1.84x-4.51x] | 2.88x | 3.66x | 212.751 (42.357) | 957.121 (188.913) | 2.58x [1.46x-6.32x] |
| Square families ($m = n$) | 16 (146) | **2.76x** [2.06x-3.71x] | 2.70x | 3.87x | 217.577 (42.352) | 979.246 (189.944) | 2.30x [0.92x-15.54x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.77x** [1.25x-2.52x] | 1.65x | 3.00x | 4.796 (0.843) | 11.109 (1.987) | 1.88x [0.74x-2.65x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.33x** [1.22x-4.42x] | 2.48x | 2.93x | 35.860 (6.556) | 65.222 (11.145) | 1.90x [0.98x-15.54x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.45x** [1.93x-3.10x] | 2.37x | 3.61x | 222.373 (43.196) | 990.355 (191.931) | 2.07x [0.74x-15.54x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 6.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.91x** | 0.0005 | 0.0009 | 0.0005 | 0.0116 | $0.0005 \pm 0.0001$ | $0.0009 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.55x** | 0.0016 | 0.0025 | 0.0019 | 0.0108 | $0.0016 \pm 0.0004$ | $0.0024 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.85x** | 0.0035 | 0.0064 | 0.0045 | 0.0155 | $0.0034 \pm 0.0007$ | $0.0064 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.17x** | 0.0036 | 0.0039 | 0.0044 | 0.0144 | $0.0034 \pm 0.0006$ | $0.0039 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.19x** | 0.0037 | 0.0045 | 0.0058 | 0.0154 | $0.0040 \pm 0.0013$ | $0.0046 \pm 0.0009$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.76x** | 0.0062 | 0.0114 | 0.0102 | 0.0339 | $0.0065 \pm 0.0019$ | $0.0112 \pm 0.0019$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.64x** | 0.0299 | 0.0701 | 0.0364 | 0.0959 | $0.0270 \pm 0.0065$ | $0.0699 \pm 0.0112$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.32x** | 0.0539 | 0.193 | 0.0713 | 0.244 | $0.0563 \pm 0.0155$ | $0.184 \pm 0.0268$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.26x** | 0.0469 | 0.0975 | 0.0697 | 0.150 | $0.0456 \pm 0.0041$ | $0.104 \pm 0.0223$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.66x** | 0.139 | 0.221 | 0.149 | 0.264 | $0.146 \pm 0.0482$ | $0.236 \pm 0.0445$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **11.01x** | 0.0649 | 0.685 | 0.0820 | 0.764 | $0.0643 \pm 0.0148$ | $0.691 \pm 0.0246$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.94x** | 0.0367 | 0.0699 | 0.0634 | 0.149 | $0.0384 \pm 0.0104$ | $0.0731 \pm 0.0081$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **1.94x** | 0.0398 | 0.0773 | 0.0653 | 0.152 | $0.0417 \pm 0.0081$ | $0.0800 \pm 0.0078$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.14x** | 0.0820 | 0.278 | 0.109 | 0.359 | $0.104 \pm 0.0749$ | $0.290 \pm 0.0375$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.82x** | 0.326 | 1.296 | 0.436 | 1.703 | $0.332 \pm 0.0483$ | $1.264 \pm 0.135$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.90x** | 1.315 | 5.211 | 1.762 | 7.747 | $1.336 \pm 0.112$ | $5.207 \pm 0.366$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.66x** | 0.895 | 1.494 | 1.462 | 4.158 | $0.882 \pm 0.0511$ | $1.464 \pm 0.0578$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.66x** | 5.400 | 8.630 | 5.599 | 10.457 | $5.705 \pm 2.089$ | $9.275 \pm 2.724$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.52x** | 0.355 | 0.826 | 0.748 | 3.871 | $0.343 \pm 0.0234$ | $0.864 \pm 0.0694$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.37x** | 0.405 | 1.006 | 0.828 | 3.975 | $0.424 \pm 0.0285$ | $1.003 \pm 0.0189$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **5.00x** | 5.917 | 29.055 | 7.949 | 38.047 | $5.878 \pm 0.348$ | $29.441 \pm 2.501$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **5.18x** | 28.070 | 142.692 | 37.202 | 189.971 | $27.984 \pm 3.921$ | $144.172 \pm 9.629$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 6.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSkip64`, `InitReducedRow64`, and `Min2ReducedRow64`)

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` with no `>= 8x8` tall transpose and either fused square `Minimize` non-sparse path or `rows * cols < 32`, so no AVX2 kernel runs): 2 of 3 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32`, tall `rows > cols` with `>= 8x8`, or fallback `AllAbsLE64`/`ValidateAndNegate64` at `rows * cols >= 32`): 17 faster, 1 slower, 1 with no detectable effect, out of 19.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.11x [1.02x-1.22x] | 0.0005 | 0.0004 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.05x [1.04x-1.07x] | 0.0016 | 0.0015 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source | 0.98x [0.90x-1.06x] | 0.0035 | 0.0035 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | SIMD faster | 1.11x [1.09x-1.13x] | 0.0036 | 0.0033 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | SIMD faster | 1.18x [1.13x-1.23x] | 0.0037 | 0.0032 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | SIMD slower | 0.96x [0.94x-0.99x] | 0.0062 | 0.0065 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.24x [1.17x-1.31x] | 0.0299 | 0.0232 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.35x [1.24x-1.48x] | 0.0539 | 0.0420 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.54x [1.29x-1.83x] | 0.0469 | 0.0303 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.63x [1.46x-1.82x] | 0.139 | 0.0794 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.43x [1.28x-1.60x] | 0.0649 | 0.0454 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | no detectable effect | 1.20x [0.90x-1.59x] | 0.0367 | 0.0267 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.33x [1.09x-1.63x] | 0.0398 | 0.0295 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.57x [1.13x-2.19x] | 0.0820 | 0.0547 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.65x [1.54x-1.76x] | 0.326 | 0.191 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.86x [1.71x-2.02x] | 1.315 | 0.734 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.12x [1.14x-3.94x] | 0.895 | 0.335 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.68x [2.56x-2.80x] | 5.400 | 1.899 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.89x [1.77x-2.02x] | 0.355 | 0.188 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.83x [1.56x-2.15x] | 0.405 | 0.223 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.79x [1.68x-1.91x] | 5.917 | 3.271 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.43x [1.38x-1.49x] | 28.070 | 18.821 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

## 7. Production Binary Footprint and Linker Dead-Code Elimination

Standalone `CGO_ENABLED=0` binaries compiled across the six canonical `goblin/assign`, `goblin/mad`, and `goblin` build profiles (`bench/results/goblin_binary_size.json`), verifying that unreferenced `WithCertified`, `WithVIPRTrace`, `encoding/json`, and (in `goblin/assign` and both `goblin/mad` profiles) `math/big` symbols are eliminated by the Go linker:

<!-- BEGIN GENERATED:BINARY_SIZE_TABLE -->
| Profile | Default Raw / Stripped (KiB) | `GOEXPERIMENT=simd` Raw / Stripped (KiB) | Stripped Delta vs. Default | `goblin` Text (KiB) | `math/big` Syms | `encoding/json` Syms | `VIPR` Syms | `Certif` Syms |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `assign` default (`LinearSum` + `SolveInto`) | 2822.8 / **1768.2** | 2858.1 / **1832.2** | baseline | 118.7 | `0` | `0` | `0` | `0` |
| `assign` certified (`+ assign.WithCertified(true)`) | 2942.9 / **1844.2** | 2980.7 / **1916.2** | +76.0 KiB (+4.3%) | 177.2 | `0` | `0` | `0` | `17` |
| `goblin/mad` default (`mad.Solve`, `StrategyAuto`) | 2783.9 / **1760.2** | 2714.5 / **1760.2** | baseline | 117.8 | `0` | `0` | `0` | `0` |
| `goblin/mad` certified (`+ mad.WithCertified(true)`) | 2978.2 / **1888.2** | 2900.4 / **1888.2** | +128.0 KiB (+7.3%) | 201.2 | `0` | `0` | `0` | `5` |
| `goblin` default (`Model.Solve`, `DefaultOptions`) | 5586.4 / **3560.2** | 5472.3 / **3576.2** | baseline | 1023.4 | `130` | `0` | `0` | `0` |
| `goblin` certified + VIPR (`+ WithCertified + WithVIPRTrace`) | 6831.2 / **4400.2** | 6697.1 / **4420.2** | +840.0 KiB (+23.6%) | 1503.1 | `155` | `0` | `220` | `14` |
<!-- END GENERATED:BINARY_SIZE_TABLE -->

## 8. Combinatorial Group-Item Allocation (`goblin/mad` vs. Algebraic MILP)

The 13-family combinatorial MAD corpus (`testdata/mad_corpus/manifest.json`; 131 randomized trials across the `pmad_base_g10_i2` base instance with exact Trial-0 optimum `24137`, 8 PMAD families across $G \in \{6, 10, 20\}$, $I \in \{4..12\}$, and $\lambda \in \{0.25, 1.0\}$, plus 4 random-weight controls) evaluates `gb.SolveMAD` (`StrategyAuto`, `StrategyDP`, and `StrategyBruteForce` when DFS states are at most `250,000`) against 250 ms-capped `BuildMADModel` `Model.Solve` (`BackendGo` and `BackendCBC`) with `1e-9` combinatorial parity and `[BestBound, Incumbent]` containment.

### 8.1 Aggregate Combinatorial MAD Summary

<!-- BEGIN GENERATED:MAD_COMBINATORIAL_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Auto Trial (Med) Sum (ms) | DP Trial (Med) Sum (ms) | MILP Go Trial (Med) Sum (ms) [Opt] | MILP CBC Trial (Med) Sum (ms) [Opt] | Equal-Family `MILP Go / Auto` [95% CI] | Equal-Family `MILP CBC / Auto` [95% CI] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| PMAD base (`G = 10, I = 2`) | 1 (11) | 0.264 (0.0245) | 0.103 (0.0094) | 909.5 (15.8) [8/11] | 737.1 (64.6) [11/11] | **1151.4x** | **2533.4x** |
| PMAD small & medium (`I <= 10`) | 6 (66) | 99.973 (9.057) | 99.178 (8.962) | 15727.5 (1500.7) [8/66] | 12783.7 (1169.8) [27/66] | **440.4x** [73.7x-2632.2x] | **317.4x** [80.8x-1246.2x] |
| PMAD large (`I = 12`) | 2 (10) | 476.537 (81.711) | 460.353 (79.088) | 2502.5 (500.4) [0/10] | 2561.4 (510.2) [0/10] | **6.6x** [0.0x-30069.4x] | **6.8x** [0.0x-25474.6x] |
| Random-weight MAD controls (`I <= 12`) | 4 (44) | 6.311 (0.543) | 6.005 (0.522) | 2594.6 (278.1) [37/44] | 4485.9 (418.0) [33/44] | **156.2x** [45.0x-541.5x] | **799.1x** [349.6x-1826.5x] |
| **All 13 combinatorial MAD families** | 13 (131) | 583.086 (91.336) | 565.640 (88.581) | 21734.2 (2295.0) [53/131] | 20568.1 (2162.6) [71/131] | **180.7x** [54.4x-600.3x] | **273.7x** [82.6x-907.3x] |
<!-- END GENERATED:MAD_COMBINATORIAL_SUMMARY_TABLE -->

### 8.2 Full 13-Family Combinatorial MAD Pipeline Comparison

<!-- BEGIN GENERATED:MAD_COMBINATORIAL_PIPELINE_TABLE -->
| Problem ID | $G \times I \times T$ | $\lambda$ | Auto Strategy | Auto Med (ms) | DP Med (ms) | DFS Med (ms) | MILP Go 250 ms | MILP CBC 250 ms | Trial 0 Obj | Parity |
| :--- | :---: | ---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `pmad_base_g10_i2` | $10 \times 2 \times 6$ | 1.00 | `DP` | 0.0245 | 0.0094 | 0.0922 | 15.76 (8/11 opt) | 64.60 (`Optimal`) | 24137.0 | `OK` |
| `pmad_g6_i6_lam025` | $6 \times 6 \times 6$ | 0.25 | `DP` | 0.101 | 0.0872 | 27.061 | 250.04 (4/11 opt) | 103.73 (`Optimal`) | 13259.5 | `OK` |
| `pmad_g6_i10_lam10` | $6 \times 10 \times 6$ | 1.00 | `DP` | 2.890 | 2.918 | skip (>250k) | 250.07 (`TimeLimit`) | 251.93 (1/11 opt) | 14018.0 | `OK` |
| `pmad_g6_i12_lam025` | $6 \times 12 \times 6$ | 0.25 | `DP` | 19.322 | 19.407 | skip (>250k) | 250.15 (`TimeLimit`) | 251.93 (`TimeLimit`) | 9787.0 | `OK` |
| `pmad_g10_i4_lam025` | $10 \times 4 \times 6$ | 0.25 | `DP` | 0.0490 | 0.0344 | 8.814 | 250.09 (4/11 opt) | 53.79 (`Optimal`) | 23954.0 | `OK` |
| `pmad_g10_i8_lam10` | $10 \times 8 \times 6$ | 1.00 | `DP` | 0.763 | 0.744 | skip (>250k) | 250.11 (`TimeLimit`) | 252.58 (3/11 opt) | 21928.0 | `OK` |
| `pmad_g10_i10_lam025` | $10 \times 10 \times 6$ | 0.25 | `DP` | 4.884 | 4.835 | skip (>250k) | 250.17 (`TimeLimit`) | 252.42 (1/11 opt) | 22570.0 | `OK` |
| `pmad_g20_i6_lam10` | $20 \times 6 \times 6$ | 1.00 | `DP` | 0.370 | 0.342 | skip (>250k) | 250.20 (`TimeLimit`) | 255.33 (`TimeLimit`) | 43530.0 | `OK` |
| `pmad_g20_i12_lam10` | $20 \times 12 \times 6$ | 1.00 | `DP` | 62.389 | 59.681 | skip (>250k) | 250.27 (`TimeLimit`) | 258.28 (`TimeLimit`) | 38256.0 | `OK` |
| `mad_comb_rand_g2_i4` | $2 \times 4 \times 6$ | 0.00 | `BruteForce` | 0.0115 | 0.0053 | 0.0033 | 0.70 (`Optimal`) | 14.33 (`Optimal`) | 4601.0 | `OK` |
| `mad_comb_rand_g2_i8` | $2 \times 8 \times 6$ | 0.00 | `BruteForce` | 0.0358 | 0.0413 | 0.0247 | 4.48 (`Optimal`) | 18.52 (`Optimal`) | 5975.0 | `OK` |
| `mad_comb_rand_g3_i9` | $3 \times 9 \times 6$ | 0.00 | `DP` | 0.0691 | 0.0615 | 0.354 | 22.83 (`Optimal`) | 133.95 (`Optimal`) | 7720.0 | `OK` |
| `mad_comb_rand_g4_i12` | $4 \times 12 \times 6$ | 0.00 | `DP` | 0.426 | 0.414 | 36.549 | 250.09 (4/11 opt) | 251.18 (`TimeLimit`) | 7905.0 | `OK` |
<!-- END GENERATED:MAD_COMBINATORIAL_PIPELINE_TABLE -->
