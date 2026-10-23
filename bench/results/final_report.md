# `gopt` vs. Python (`PuLP + COIN-OR CBC`) Benchmark Report

## 1. Experimental Setup and Provenance

- **CPU**: `13th Gen Intel(R) Core(TM) i9-13900K` (x86_64, 30 logical CPUs exposed by `lscpu`, AVX2 + FMA)
- **Default build**: `go build` with the pure-Go scalar kernels. All headline timings use this build.
- **Experimental build**: `GOEXPERIMENT=simd`, which dispatches AVX2 kernels only for slices of length 256 or more; see section 4.
- **Python baseline**: Python 3.14 + `PuLP` + `COIN-OR CBC` subprocess (`threads=1`). Its solve time includes MPS writing, process spawn, and `.sol` parsing.
- **Corpus**: 31 deterministic LP and MILP families across 3 tiers (`testdata/corpus/*.json` and `testdata/corpus/trials/*.jsonl`; 11 randomized trial instances per family for Tiers 1-2 and 5 for Tier 3, 293 instances in total), measured in warm state.

<!-- BEGIN GENERATED:PROVENANCE -->
- **gopt telemetry source**: fingerprint `dfc05e06279c1488` (working tree on top of commit `5c12aa5e7e2c3070bfeebe8df5fd14d8df1c1be0`), `go1.27.1-X:nodwarf5`; the documentation gate rejects telemetry whose fingerprint differs from the checked-in sources
- **CPU affinity**: gopt `2`; Python + CBC `2`
- **Python reference**: PuLP `4.2.0`, CBC `2.10.3` (`cbc`)
- **CBC subprocess spawn floor**: 2.09 ms median (`prob.solve()` on a 1-variable MILP); the "Net of Spawn" columns subtract it
- **goblin**: COIN-OR CBC/CLP `master` (commit `c1c171ec`, built in `../Cbc`), linked in-process through `cgo`. goblin maps gopt's default options onto CBC with two deliberate differences: `MaxCutPasses = 12` becomes `passC = 30` and `MaxNodes = 50000` becomes unlimited. It is therefore not a like-for-like configuration and not the CBC 2.10.3 binary used by the Python baseline.
<!-- END GENERATED:PROVENANCE -->

## 2. Aggregate Workload Tier Summary

The planted-gap MAD family is constructed so that a root heuristic closes the gap at node 1; the bold row excludes it.

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families | Py Total GeoMean (ms) | Py Net-of-Spawn GeoMean (ms) | goblin GeoMean (ms) | gopt GeoMean (ms) | `Py / gopt` GeoMean | `Py / gopt` Median [Min-Max] | `Py net / gopt` GeoMean | `gopt / goblin` |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical (`N <= 22`) | 10 | 3.86 | 0.38 | 1.23 | 0.04 | **91.3x** | 150.2x [7.7x-313.6x] | 9.0x | 0.03x |
| Tier 2: Target p90 (`30 <= N <= 76`) | 13 | 14.37 | 11.37 | 25.63 | 2.08 | **6.9x** | 8.7x [1.3x-27.4x] | 5.5x | 0.08x |
| Tier 3: Large Tail (`135 <= N <= 4028`) | 8 | 101.99 | 97.98 | 133.67 | 44.89 | **2.3x** | 1.7x [0.4x-11.4x] | 2.2x | 0.34x |
| Tier 3 non-MAD MILPs | 4 | 120.68 | 118.18 | 214.01 | 23.44 | **5.1x** | 8.3x [1.0x-11.4x] | 5.0x | 0.11x |
| Planted-gap MAD family (zero integrality gap by construction) | 8 | 23.99 | 19.16 | 24.24 | 7.43 | **3.2x** | 2.2x [0.4x-27.4x] | 2.6x | 0.31x |
| Random-weight MAD (negative controls) | 3 | 44.33 | 40.97 | 73.38 | 18.67 | **2.4x** | 1.4x [1.3x-7.7x] | 2.2x | 0.25x |
| **All families except planted-gap MAD** | 23 | 13.42 | 4.59 | 12.39 | 0.71 | **18.8x** | 11.4x [1.0x-313.6x] | 6.4x | 0.06x |
| All corpus families | 31 | 15.59 | 6.64 | 14.73 | 1.31 | **11.9x** | 9.5x [0.4x-313.6x] | 5.1x | 0.09x |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

## 3. End-to-End Pipeline Comparison (`JSON -> Solution`)

Pipeline stages measured per instance (median across trials after warmup):
1. **Prep (`t_parse + t_build + t_format/t_compile`)**: JSON unmarshaling, model construction, and MPS formatting (Python) or `Model.Compile()` matrix compilation (Go).
2. **Solve (`t_solve`)**: CBC subprocess execution and `.sol` parsing (Python) vs. in-memory `gopt.SolveCompiled` (Go).
3. **Total (`t_total`)**: Complete end-to-end latency from raw JSON bytes to verified primal solution.

The Parity column is computed by `bench.CheckParity`: status, canonical objective, and every per-trial objective of both gopt builds (and goblin) must match CBC within a relative tolerance of 1e-5, and `ValidateTelemetryAgainstCorpus` rejects the telemetry otherwise.

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | PyPrep (ms) | GoPrep (ms) | PySolve (ms) | PySolve Net of Spawn (ms) | GoSolve (ms) | PyTotal (ms) | GoTotal (ms) | `Py / gopt` | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | 0.087 | 0.008 | 1.86 | 0.00 | 0.00 | 1.92 | 0.01 | **174.2x** | `OK` |
| `t1_std_lp_max` | 3 | 3 | 0.091 | 0.008 | 1.73 | 0.00 | 0.00 | 1.79 | 0.01 | **170.2x** | `OK` |
| `t1_std_mip` | 3 | 3 | 0.122 | 0.008 | 3.35 | 1.25 | 0.00 | 3.45 | 0.01 | **304.1x** | `OK` |
| `t1_mip_float_obj` | 3 | 3 | 0.129 | 0.008 | 3.21 | 1.12 | 0.00 | 3.30 | 0.01 | **313.6x** | `OK` |
| `t1_whiskas` | 6 | 5 | 0.184 | 0.017 | 2.76 | 0.67 | 0.00 | 2.91 | 0.02 | **130.2x** | `OK` |
| `t1_furniture` | 2 | 3 | 0.087 | 0.012 | 2.92 | 0.83 | 0.02 | 2.98 | 0.03 | **87.7x** | `OK` |
| `t1_knapsack_12` | 12 | 3 | 0.142 | 0.037 | 4.87 | 2.77 | 0.09 | 4.98 | 0.14 | **36.6x** | `OK` |
| `t1_infeasible_mip` | 2 | 1 | 0.095 | 0.008 | 2.21 | 0.11 | 0.00 | 2.27 | 0.01 | **244.6x** | `OK` |
| `t1_mad_g2_i4` | 22 | 30 | 0.291 | 0.130 | 5.23 | 3.13 | 0.16 | 5.45 | 0.30 | **18.2x** | `OK` |
| `t1_mad_rand_g2_i4` | 22 | 32 | 0.443 | 0.176 | 34.90 | 32.80 | 4.31 | 35.23 | 4.60 | **7.7x** | `OK` |
| `t2_mkp_30` | 30 | 6 | 0.377 | 0.126 | 37.88 | 35.79 | 4.67 | 38.23 | 4.82 | **7.9x** | `OK` |
| `t2_mkp_50` | 50 | 8 | 0.585 | 0.203 | 14.52 | 12.43 | 8.06 | 14.82 | 8.27 | **1.8x** | `OK` |
| `t2_cflp_36` | 36 | 11 | 0.420 | 0.109 | 6.08 | 3.98 | 0.26 | 6.47 | 0.37 | **17.4x** | `OK` |
| `t2_cflp_49` | 49 | 13 | 0.491 | 0.146 | 6.66 | 4.57 | 0.64 | 7.09 | 0.81 | **8.7x** | `OK` |
| `t2_gap_45` | 45 | 13 | 0.345 | 0.135 | 10.44 | 8.35 | 1.00 | 10.81 | 1.14 | **9.5x** | `OK` |
| `t2_lotsizing_48` | 48 | 32 | 0.550 | 0.188 | 58.84 | 56.74 | 6.33 | 59.24 | 6.50 | **9.1x** | `OK` |
| `t2_binpack_44` | 44 | 14 | 0.473 | 0.131 | 10.99 | 8.89 | 0.55 | 11.38 | 0.68 | **16.7x** | `OK` |
| `t2_setcover_50` | 50 | 20 | 0.418 | 0.130 | 6.20 | 4.11 | 0.14 | 6.51 | 0.28 | **23.6x** | `OK` |
| `t2_mad_g2_i8` | 30 | 34 | 0.626 | 0.107 | 4.94 | 2.85 | 0.09 | 5.48 | 0.20 | **27.4x** | `OK` |
| `t2_mad_g3_i9` | 48 | 48 | 0.961 | 0.331 | 7.57 | 5.48 | 0.53 | 8.33 | 0.96 | **8.7x** | `OK` |
| `t2_mad_g4_i12` | 76 | 64 | 1.298 | 0.555 | 7.32 | 5.23 | 2.27 | 7.99 | 2.95 | **2.7x** | `OK` |
| `t2_mad_rand_g2_i8` | 30 | 36 | 0.692 | 0.236 | 13.37 | 11.28 | 9.85 | 13.93 | 10.10 | **1.4x** | `OK` |
| `t2_mad_rand_g3_i9` | 48 | 51 | 0.599 | 0.382 | 177.04 | 174.95 | 139.79 | 177.47 | 140.19 | **1.3x** | `OK` |
| `t3_mkp_200` | 200 | 8 | 1.071 | 0.572 | 51.71 | 49.62 | 50.67 | 52.38 | 51.24 | **1.0x** | `OK` |
| `t3_cflp_300` | 300 | 34 | 1.362 | 0.806 | 259.76 | 257.67 | 22.18 | 261.58 | 22.98 | **11.4x** | `OK` |
| `t3_lotsizing_504` | 504 | 336 | 2.514 | 0.882 | 153.28 | 151.18 | 27.83 | 155.39 | 28.70 | **5.4x** | `OK` |
| `t3_sched_1000` | 1000 | 100 | 6.842 | 2.548 | 95.76 | 93.67 | 7.37 | 99.61 | 8.94 | **11.1x** | `OK` |
| `t3_mad_g5_i20` | 135 | 85 | 2.720 | 1.151 | 12.85 | 10.76 | 7.72 | 14.90 | 8.93 | **1.7x** | `OK` |
| `t3_mad_g6_i30` | 222 | 108 | 2.982 | 1.557 | 28.08 | 25.99 | 16.63 | 30.12 | 18.19 | **1.7x** | `OK` |
| `t3_mad_g10_i100` | 1070 | 230 | 16.389 | 7.145 | 183.31 | 181.21 | 546.52 | 195.76 | 553.86 | **0.4x** | `OK` |
| `t3_mad_g4_i1000` | 4028 | 1052 | 77.944 | 26.438 | 560.97 | 558.88 | 577.18 | 628.55 | 606.41 | **1.0x** | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

## 4. Scalar vs. `GOEXPERIMENT=simd` Build

Ratios are geometric means of per-trial `scalar / SIMD` solve times over the same instances, with 95% t-intervals on the log ratios. "Reachable" is a necessary condition only: the AVX2 `Dot64`/`Axpy64` paths need a slice of length at least 256, which requires `n + m >= 256`.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 256`, no AVX2 kernel can run): 20 of 25 have a 95% interval excluding 1. These differences come from the two binaries (compiler output and code layout differ under `GOEXPERIMENT=simd`) and from run-to-run effects, not from SIMD kernels. Intervals of similar size on the AVX2-capable instances are therefore not evidence of a SIMD effect.
- **Instances where AVX2 can run**: 1 faster, 2 slower, 3 with no detectable effect, out of 6.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Scalar Solve Median (ms) | SIMD Solve Median (ms) | Paired `scalar / SIMD` GeoMean [95% CI] | Trials | Verdict |
| :--- | ---: | :---: | ---: | ---: | ---: | ---: | :--- |
| `t1_std_lp` | 6 | no (control) | 0.001 | 0.001 | 1.10x [1.05x-1.15x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t1_std_lp_max` | 6 | no (control) | 0.001 | 0.001 | 1.28x [1.10x-1.49x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t1_std_mip` | 6 | no (control) | 0.003 | 0.003 | 1.08x [1.02x-1.14x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t1_mip_float_obj` | 6 | no (control) | 0.002 | 0.002 | 1.06x [1.01x-1.11x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t1_whiskas` | 11 | no (control) | 0.003 | 0.002 | 1.16x [1.10x-1.22x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t1_furniture` | 5 | no (control) | 0.019 | 0.016 | 1.21x [1.12x-1.30x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t1_knapsack_12` | 15 | no (control) | 0.086 | 0.082 | 1.09x [1.01x-1.17x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t1_infeasible_mip` | 3 | no (control) | 0.000 | 0.000 | 1.65x [1.47x-1.85x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t1_mad_g2_i4` | 52 | no (control) | 0.155 | 0.107 | 1.36x [1.17x-1.58x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t1_mad_rand_g2_i4` | 54 | no (control) | 4.306 | 3.424 | 1.28x [1.10x-1.49x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t2_mkp_30` | 36 | no (control) | 4.666 | 3.191 | 1.41x [1.30x-1.53x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t2_mkp_50` | 58 | no (control) | 8.058 | 6.142 | 1.34x [1.21x-1.48x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t2_cflp_36` | 47 | no (control) | 0.261 | 0.173 | 1.50x [1.44x-1.56x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t2_cflp_49` | 62 | no (control) | 0.641 | 0.449 | 1.39x [1.33x-1.46x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t2_gap_45` | 58 | no (control) | 1.000 | 1.000 | 1.25x [1.08x-1.44x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t2_lotsizing_48` | 80 | no (control) | 6.326 | 5.917 | 1.14x [0.98x-1.32x] | 11 | identical source |
| `t2_binpack_44` | 58 | no (control) | 0.553 | 0.743 | 0.83x [0.75x-0.92x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t2_setcover_50` | 70 | no (control) | 0.143 | 0.163 | 0.85x [0.74x-0.97x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t2_mad_g2_i8` | 64 | no (control) | 0.094 | 0.116 | 0.74x [0.63x-0.87x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t2_mad_g3_i9` | 96 | no (control) | 0.527 | 0.375 | 1.34x [1.24x-1.44x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t2_mad_g4_i12` | 140 | no (control) | 2.271 | 2.208 | 1.00x [0.97x-1.03x] | 11 | identical source |
| `t2_mad_rand_g2_i8` | 66 | no (control) | 9.853 | 8.402 | 1.10x [0.95x-1.27x] | 11 | identical source |
| `t2_mad_rand_g3_i9` | 99 | no (control) | 139.789 | 132.912 | 1.02x [0.97x-1.09x] | 11 | identical source |
| `t3_mkp_200` | 208 | no (control) | 50.666 | 55.941 | 0.98x [0.92x-1.04x] | 5 | identical source |
| `t3_cflp_300` | 334 | possible | 22.179 | 21.627 | 1.02x [1.00x-1.04x] | 5 | no detectable effect |
| `t3_lotsizing_504` | 840 | possible | 27.827 | 45.915 | 0.74x [0.59x-0.93x] | 5 | SIMD slower |
| `t3_sched_1000` | 1100 | possible | 7.373 | 8.275 | 0.87x [0.78x-0.96x] | 5 | SIMD slower |
| `t3_mad_g5_i20` | 220 | no (control) | 7.720 | 4.318 | 1.50x [1.13x-1.99x] | 5 | identical source; interval excludes 1 (build/run effect) |
| `t3_mad_g6_i30` | 330 | possible | 16.634 | 9.630 | 1.37x [1.05x-1.79x] | 5 | SIMD faster |
| `t3_mad_g10_i100` | 1300 | possible | 546.516 | 522.943 | 1.04x [0.91x-1.19x] | 5 | no detectable effect |
| `t3_mad_g4_i1000` | 5080 | possible | 577.182 | 611.589 | 1.08x [0.97x-1.21x] | 5 | no detectable effect |
<!-- END GENERATED:SIMD_EFFECT_TABLE -->

## 5. Solver-Core Checkpoint Progression (default build)

Progression across checkpoints on pre-compiled `CompiledProblem` instances:
- **CP2/3 (Root LP)**: Revised Dual and Two-Phase Primal Simplex.
- **CP4 (Base B&B)**: Branch-and-Bound with pseudocost/strong branching (`Presolve=off, Cuts=off, Heuristics=off`).
- **CP5 (+Presolve)**: Savelsbergh bound tightening, GCD reduction, and node propagation (`Presolve=on, Cuts=off, Heuristics=off`).
- **CP6 (+Cuts)**: GMI, lifted 0-1 knapsack covers, flow/VUB and $(l, S)$ cuts, and single-row MIR cuts (`Presolve=on, Cuts=on, Heuristics=off`).
- **CP7 (Full `gopt`)**: Full Branch-and-Cut with Propagated Rounding, Root Sub-MIP, and Fractional Diving (`Presolve=on, Cuts=on, Heuristics=on`).

<!-- BEGIN GENERATED:FULL_CHECKPOINT_TABLE -->
| Problem ID | Root LP Obj | Optimal Obj | Root LP (ms) | CP4 Base (ms / nodes) | CP5 +Presolve (ms / nodes) | CP6 +Cuts (ms / nodes / cuts) | CP7 Full (ms / nodes / cuts) |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 54.00 | 54.00 | 0.001 | 0.00 (1) | 0.00 (1) | 0.00 (1 / 0) | **0.00** (1 / 0) |
| `t1_std_lp_max` | 80.00 | 80.00 | 0.001 | 0.00 (1) | 0.00 (1) | 0.00 (1 / 0) | **0.00** (1 / 0) |
| `t1_std_mip` | 58.00 | 64.00 | 0.001 | 0.00 (3) | 0.00 (1) | 0.00 (1 / 0) | **0.00** (1 / 0) |
| `t1_mip_float_obj` | 58.90 | 64.95 | 0.001 | 0.00 (3) | 0.00 (1) | 0.00 (1 / 0) | **0.00** (1 / 0) |
| `t1_whiskas` | 0.52 | 0.52 | 0.004 | 0.01 (1) | 0.00 (1) | 0.00 (1 / 0) | **0.00** (1 / 0) |
| `t1_furniture` | 1075.00 | 1075.00 | 0.002 | 0.01 (1) | 0.02 (1) | 0.01 (1 / 0) | **0.02** (1 / 0) |
| `t1_knapsack_12` | 578.65 | 513.24 | 0.005 | 0.12 (31) | 0.08 (24) | 0.10 (2 / 15) | **0.09** (1 / 15) |
| `t1_infeasible_mip` | 1.50 | 0.00 | 0.001 | 0.00 (5) | 0.00 (0) | 0.00 (0 / 0) | **0.00** (0 / 0) |
| `t1_mad_g2_i4` | 2160.00 | 2160.00 | 0.068 | 0.34 (4) | 0.24 (4) | 0.44 (4 / 2) | **0.16** (1 / 0) |
| `t1_mad_rand_g2_i4` | 4257.00 | 5249.00 | 0.090 | 0.67 (9) | 0.70 (9) | 0.89 (11 / 4) | **4.31** (11 / 4) |
| `t2_mkp_30` | 1966.92 | 1917.24 | 0.027 | 1.06 (77) | 1.29 (77) | 2.77 (60 / 9) | **4.67** (29 / 9) |
| `t2_mkp_50` | 2648.37 | 2620.01 | 0.046 | 4.91 (119) | 6.19 (132) | 5.85 (2 / 5) | **8.06** (1 / 5) |
| `t2_cflp_36` | 781.97 | 801.17 | 0.031 | 0.19 (7) | 0.20 (7) | 1.39 (3 / 2) | **0.26** (1 / 2) |
| `t2_cflp_49` | 827.30 | 909.02 | 0.048 | 0.46 (9) | 0.46 (9) | 2.35 (4 / 20) | **0.64** (1 / 20) |
| `t2_gap_45` | 178.04 | 195.18 | 0.042 | 2.64 (61) | 2.27 (61) | 0.35 (1 / 13) | **1.00** (1 / 13) |
| `t2_lotsizing_48` | 2052.34 | 2245.81 | 0.108 | 3.87 (174) | 2.84 (198) | 7.65 (33 / 40) | **6.33** (11 / 40) |
| `t2_binpack_44` | 29.30 | 30.32 | 0.047 | 28.97 (2400) | 24.45 (1895) | 1.31 (18 / 12) | **0.55** (1 / 12) |
| `t2_setcover_50` | 56.84 | 58.10 | 0.041 | 0.68 (9) | 0.70 (9) | 0.28 (2 / 6) | **0.14** (1 / 6) |
| `t2_mad_g2_i8` | 2160.00 | 2160.00 | 0.061 | 0.36 (6) | 0.41 (7) | 0.80 (6 / 2) | **0.09** (1 / 0) |
| `t2_mad_g3_i9` | 4320.00 | 4320.00 | 0.348 | 2.54 (8) | 2.39 (18) | 2.02 (13 / 2) | **0.53** (1 / 0) |
| `t2_mad_g4_i12` | 7200.00 | 7200.00 | 0.720 | 4.04 (30) | 3.99 (12) | 5.14 (10 / 7) | **2.27** (1 / 0) |
| `t2_mad_rand_g2_i8` | 5894.00 | 6696.00 | 0.163 | 2.20 (31) | 1.72 (33) | 2.25 (32 / 5) | **9.85** (35 / 5) |
| `t2_mad_rand_g3_i9` | 6060.00 | 7958.00 | 0.416 | 83.56 (181) | 66.65 (364) | 81.89 (262 / 11) | **139.79** (339 / 11) |
| `t3_mkp_200` | 12955.46 | 12906.80 | 0.194 | 125.71 (5000, `NodeLimit`) | 151.19 (5000, `NodeLimit`) | 78.58 (739 / 21) | **50.67** (557 / 21) |
| `t3_cflp_300` | 2081.53 | 2187.30 | 0.218 | 9.27 (134) | 7.24 (134) | 25.67 (39 / 37) | **22.18** (9 / 37) |
| `t3_lotsizing_504` | 10518.99 | 13120.44 | 2.344 | 250.04 (3327, `TimeLimit`) | 250.06 (4215, `TimeLimit`) | 126.15 (365 / 165) | **27.83** (11 / 165) |
| `t3_sched_1000` | 46971.09 | 46818.28 | 0.177 | 240.72 (5000, `NodeLimit`) | 250.04 (3164, `TimeLimit`) | 19.31 (96 / 50) | **7.37** (17 / 50) |
| `t3_mad_g5_i20` | 10800.00 | 10800.00 | 6.360 | 33.96 (91) | 9.72 (47) | 9.54 (19 / 4) | **7.72** (1 / 0) |
| `t3_mad_g6_i30` | 15120.00 | 15120.00 | 11.389 | 134.73 (266) | 52.98 (154) | 29.98 (32 / 4) | **16.63** (1 / 0) |
| `t3_mad_g10_i100` | 79200.00 | 79200.00 | 221.955 | 250.10 (2, `TimeLimit`) | 250.13 (29, `TimeLimit`) | 250.08 (1 / 5, `TimeLimit`) | **546.52** (1 / 0) |
| `t3_mad_g4_i1000` | 187200.00 | 187200.00 | 571.212 | 251.53 (0, `TimeLimit`) | 252.57 (0, `TimeLimit`) | 250.79 (0 / 0, `TimeLimit`) | **577.18** (1 / 0) |
<!-- END GENERATED:FULL_CHECKPOINT_TABLE -->
