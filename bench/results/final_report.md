# `gopt` vs. Python (`PuLP + COIN-OR CBC`) Benchmark Report

## 1. Experimental Setup and Hardware Environment

- **CPU**: `13th Gen Intel(R) Core(TM) i9-13900K` (x86_64, 24 physical cores / 32 hardware threads; 30 logical CPUs exposed in container environment, AVX2 + FMA)
- **Go Toolchain**: `go1.27.1-X:nodwarf5 linux/amd64` with `GOEXPERIMENT=simd` (`simd/archsimd.Float64x4`)
- **Python Baseline**: Python 3.14 + `PuLP` + `COIN-OR CBC` subprocess (`threads=1`)
- **In-Process C++ Reference (`goblin`)**: In-memory `cgo` bindings to `COIN-OR CBC/CLP` (`OsiClpSolverInterface` / `CbcSolver`), isolating Python/MPS/subprocess IPC overhead from solver-core execution time.
- **Corpus**: 31 deterministic LP and MILP problem families across 3 tiers (`testdata/corpus/*.json` and `testdata/corpus/trials/*.jsonl`, 11 randomized trial instances per family for Tiers 1-2 and 5 per family for Tier 3 = 293 total instances), evaluated strictly in warm state after worker-level and per-problem warmup passes.

## 2. Aggregate Workload Tier Summary

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

## 3. End-to-End Pipeline Comparison (`JSON -> Solution`)

Pipeline stages measured per instance (median across trials after warmup):
1. **Prep (`t_parse + t_build + t_format/t_compile`)**: JSON unmarshaling, model construction, and MPS formatting (Python) or `Model.Compile()` matrix compilation (Go).
2. **Solve (`t_solve`)**: CBC subprocess execution and `.sol` parsing (Python) vs. in-memory `gopt.SolveCompiled` (Go).
3. **Total (`t_total`)**: Complete end-to-end latency from raw JSON bytes to verified primal solution.

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

## 4. Solver-Core Checkpoint Progression (`GOEXPERIMENT=simd`)

Progression across checkpoints on pre-compiled `CompiledProblem` instances:
- **CP2/3 (Root LP)**: Revised Dual & Two-Phase Primal Simplex with `simd/archsimd` kernels.
- **CP4 (Base B&B)**: Branch-and-Bound with pseudocost/strong branching (`Presolve=off, Cuts=off, Heuristics=off`).
- **CP5 (+Presolve)**: Savelsbergh bound tightening, GCD reduction, and node propagation (`Presolve=on, Cuts=off, Heuristics=off`).
- **CP6 (+Cuts)**: GMI, Lifted 0-1 Knapsack Covers, Flow/VUB $(l, S)$ cuts, and Marchand-Wolsey c-MIR cuts (`Presolve=on, Cuts=on, Heuristics=off`).
- **CP7 (Full `gopt`)**: Full Branch-and-Cut with Propagated Rounding, Root Sub-MIP, and Fractional Diving (`Presolve=on, Cuts=on, Heuristics=on`).

<!-- BEGIN GENERATED:FULL_CHECKPOINT_TABLE -->
| Problem ID | Root LP Obj | Optimal Obj | Root LP (ms) | CP4 Base (ms / nodes) | CP5 +Presolve (ms / nodes) | CP6 +Cuts (ms / nodes / cuts) | CP7 Full (ms / nodes / cuts) |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 54.00 | 54.00 | 0.001 | 0.00 (1) | 0.00 (1) | 0.00 (1 / 0) | **0.00** (1 / 0) |
| `t1_std_lp_max` | 80.00 | 80.00 | 0.001 | 0.00 (1) | 0.00 (1) | 0.00 (1 / 0) | **0.00** (1 / 0) |
| `t1_std_mip` | 58.00 | 64.00 | 0.001 | 0.00 (3) | 0.00 (1) | 0.00 (1 / 0) | **0.00** (1 / 0) |
| `t1_mip_float_obj` | 58.90 | 64.95 | 0.001 | 0.00 (3) | 0.00 (1) | 0.00 (1 / 0) | **0.00** (1 / 0) |
| `t1_whiskas` | 0.52 | 0.52 | 0.003 | 0.00 (1) | 0.00 (1) | 0.00 (1 / 0) | **0.00** (1 / 0) |
| `t1_furniture` | 1075.00 | 1075.00 | 0.002 | 0.01 (1) | 0.02 (1) | 0.02 (1 / 0) | **0.02** (1 / 0) |
| `t1_knapsack_12` | 578.65 | 513.24 | 0.008 | 0.18 (31) | 0.14 (24) | 0.10 (2 / 15) | **0.12** (1 / 15) |
| `t1_infeasible_mip` | 1.50 | 0.00 | 0.001 | 0.01 (5) | 0.00 (0) | 0.00 (0 / 0) | **0.00** (0 / 0) |
| `t1_mad_g2_i4` | 2160.00 | 2160.00 | 0.108 | 0.38 (4) | 0.44 (4) | 0.22 (3 / 2) | **0.18** (1 / 0) |
| `t1_mad_rand_g2_i4` | 4257.00 | 5249.00 | 0.090 | 0.50 (9) | 0.58 (9) | 1.01 (7 / 27) | **3.82** (7 / 27) |
| `t2_mkp_30` | 1966.92 | 1917.24 | 0.043 | 1.21 (77) | 1.33 (77) | 2.88 (66 / 9) | **3.27** (29 / 9) |
| `t2_mkp_50` | 2648.37 | 2620.01 | 0.104 | 3.63 (133) | 4.11 (146) | 3.91 (2 / 5) | **5.82** (1 / 5) |
| `t2_cflp_36` | 781.97 | 801.17 | 0.022 | 0.18 (7) | 0.14 (7) | 1.17 (3 / 2) | **0.20** (1 / 2) |
| `t2_cflp_49` | 827.30 | 909.02 | 0.044 | 0.41 (9) | 0.41 (9) | 2.19 (4 / 20) | **0.54** (1 / 20) |
| `t2_gap_45` | 178.04 | 195.18 | 0.023 | 1.36 (61) | 2.28 (61) | 0.35 (1 / 13) | **0.86** (1 / 13) |
| `t2_lotsizing_48` | 2052.34 | 2245.81 | 0.062 | 2.18 (178) | 2.33 (177) | 10.46 (16 / 42) | **5.94** (15 / 42) |
| `t2_binpack_44` | 29.30 | 30.32 | 0.047 | 28.49 (2442) | 26.46 (1935) | 1.36 (21 / 12) | **0.41** (1 / 12) |
| `t2_setcover_50` | 56.84 | 58.10 | 0.038 | 1.02 (9) | 1.02 (9) | 0.32 (2 / 6) | **0.17** (1 / 6) |
| `t2_mad_g2_i8` | 2160.00 | 2160.00 | 0.124 | 0.51 (8) | 0.43 (8) | 0.49 (10 / 3) | **0.23** (1 / 0) |
| `t2_mad_g3_i9` | 4320.00 | 4320.00 | 0.192 | 1.13 (10) | 1.05 (6) | 1.50 (11 / 3) | **0.27** (1 / 0) |
| `t2_mad_g4_i12` | 7200.00 | 7200.00 | 0.900 | 3.20 (16) | 2.83 (13) | 3.06 (30 / 10) | **1.15** (1 / 0) |
| `t2_mad_rand_g2_i8` | 5894.00 | 6696.00 | 0.105 | 1.50 (43) | 1.25 (39) | 1.29 (41 / 6) | **7.24** (41 / 6) |
| `t2_mad_rand_g3_i9` | 6060.00 | 7958.00 | 0.433 | 78.17 (183) | 52.88 (235) | 68.45 (383 / 18) | **130.31** (293 / 18) |
| `t3_mkp_200` | 12955.46 | 12906.80 | 0.412 | 131.40 (5000, `NodeLimit`) | 150.55 (5000, `NodeLimit`) | 109.74 (1031 / 21) | **54.00** (566 / 21) |
| `t3_cflp_300` | 2081.53 | 2187.30 | 0.159 | 4.94 (112) | 4.90 (112) | 23.90 (60 / 39) | **29.66** (23 / 39) |
| `t3_lotsizing_504` | 10518.99 | 13120.44 | 3.147 | 250.18 (3213, `TimeLimit`) | 250.05 (3837, `TimeLimit`) | 238.20 (381 / 165) | **68.84** (11 / 165) |
| `t3_sched_1000` | 46971.09 | 46818.28 | 2.644 | 250.39 (3774, `TimeLimit`) | 250.04 (3469, `TimeLimit`) | 26.89 (96 / 50) | **9.38** (17 / 50) |
| `t3_mad_g5_i20` | 10800.00 | 10800.00 | 6.976 | 20.03 (54) | 11.98 (36) | 22.85 (76 / 6) | **10.74** (1 / 0) |
| `t3_mad_g6_i30` | 15120.00 | 15120.00 | 7.769 | 55.69 (90) | 47.91 (102) | 50.46 (70 / 20) | **16.41** (1 / 0) |
| `t3_mad_g10_i100` | 79200.00 | 79200.00 | 133.811 | 250.66 (4, `TimeLimit`) | 250.32 (29, `TimeLimit`) | 250.90 (36 / 20, `TimeLimit`) | **410.36** (1 / 0) |
| `t3_mad_g4_i1000` | 187200.00 | 187200.00 | 400.346 | 250.04 (0, `TimeLimit`) | 250.22 (0, `TimeLimit`) | 252.53 (0 / 0, `TimeLimit`) | **650.89** (1 / 0) |
<!-- END GENERATED:FULL_CHECKPOINT_TABLE -->
