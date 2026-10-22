# `gopt` vs. Python (`PuLP + COIN-OR CBC`) Benchmark Report

## 1. Experimental Setup and Provenance

- **CPU**: `13th Gen Intel(R) Core(TM) i9-13900K` (x86_64, 30 logical CPUs exposed by `lscpu`, AVX2 + FMA)
- **Default build**: `go build` with the pure-Go scalar kernels. All headline timings use this build.
- **Experimental build**: `GOEXPERIMENT=simd`, which dispatches AVX2 kernels only for slices of length 256 or more; see section 4.
- **Python baseline**: Python 3.14 + `PuLP` + `COIN-OR CBC` subprocess (`threads=1`). Its solve time includes MPS writing, process spawn, and `.sol` parsing.
- **Corpus**: 31 deterministic LP and MILP families across 3 tiers (`testdata/corpus/*.json` and `testdata/corpus/trials/*.jsonl`; 11 randomized trial instances per family for Tiers 1-2 and 5 for Tier 3, 293 instances in total), measured in warm state.

<!-- BEGIN GENERATED:PROVENANCE -->
- **gopt telemetry source**: fingerprint `313e00cf5a6fffb3` (working tree on top of commit `652204b7fb6e729dabef1707fb6cc2440593d9dc`), `go1.27.1-X:nodwarf5`; the documentation gate rejects telemetry whose fingerprint differs from the checked-in sources
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
| Tier 1: Canonical (`N <= 22`) | 10 | 3.86 | 0.38 | 1.23 | 0.06 | **69.9x** | 84.4x [7.5x-387.4x] | 6.9x | 0.04x |
| Tier 2: Target p90 (`30 <= N <= 76`) | 13 | 14.37 | 11.37 | 25.63 | 1.75 | **8.2x** | 11.3x [1.3x-35.7x] | 6.5x | 0.07x |
| Tier 3: Large Tail (`135 <= N <= 4028`) | 8 | 101.99 | 97.98 | 133.67 | 48.63 | **2.1x** | 1.7x [0.2x-10.8x] | 2.0x | 0.36x |
| Tier 3 non-MAD MILPs | 4 | 120.68 | 118.18 | 214.01 | 24.75 | **4.9x** | 6.8x [1.5x-10.8x] | 4.8x | 0.12x |
| Planted-gap MAD family (zero integrality gap by construction) | 8 | 23.99 | 19.16 | 24.24 | 7.37 | **3.3x** | 3.3x [0.2x-30.1x] | 2.6x | 0.30x |
| Random-weight MAD (negative controls) | 3 | 44.33 | 40.97 | 73.38 | 18.57 | **2.4x** | 1.4x [1.3x-7.5x] | 2.2x | 0.25x |
| **All families except planted-gap MAD** | 23 | 13.42 | 4.59 | 12.39 | 0.75 | **17.9x** | 15.2x [1.3x-387.4x] | 6.1x | 0.06x |
| All corpus families | 31 | 15.59 | 6.64 | 14.73 | 1.36 | **11.5x** | 11.3x [0.2x-387.4x] | 4.9x | 0.09x |
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

## 4. Scalar vs. `GOEXPERIMENT=simd` Build

Ratios are geometric means of per-trial `scalar / SIMD` solve times over the same instances, with 95% t-intervals on the log ratios. "Reachable" is a necessary condition only: the AVX2 `Dot64`/`Axpy64` paths need a slice of length at least 256, which requires `n + m >= 256`.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 256`, no AVX2 kernel can run): 9 of 25 have a 95% interval excluding 1. These differences come from the two binaries (compiler output and code layout differ under `GOEXPERIMENT=simd`) and from run-to-run effects, not from SIMD kernels. Intervals of similar size on the AVX2-capable instances are therefore not evidence of a SIMD effect.
- **Instances where AVX2 can run**: 0 faster, 1 slower, 5 with no detectable effect, out of 6.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Scalar Solve Median (ms) | SIMD Solve Median (ms) | Paired `scalar / SIMD` GeoMean [95% CI] | Trials | Verdict |
| :--- | ---: | :---: | ---: | ---: | ---: | ---: | :--- |
| `t1_std_lp` | 6 | no (control) | 0.002 | 0.002 | 1.04x [0.94x-1.15x] | 11 | identical source |
| `t1_std_lp_max` | 6 | no (control) | 0.002 | 0.003 | 0.97x [0.93x-1.02x] | 11 | identical source |
| `t1_std_mip` | 6 | no (control) | 0.009 | 0.009 | 0.96x [0.84x-1.08x] | 11 | identical source |
| `t1_mip_float_obj` | 6 | no (control) | 0.009 | 0.009 | 1.15x [0.97x-1.36x] | 11 | identical source |
| `t1_whiskas` | 11 | no (control) | 0.003 | 0.004 | 0.78x [0.65x-0.93x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t1_furniture` | 5 | no (control) | 0.022 | 0.022 | 0.96x [0.84x-1.08x] | 11 | identical source |
| `t1_knapsack_12` | 15 | no (control) | 0.069 | 0.118 | 0.87x [0.45x-1.68x] | 11 | identical source |
| `t1_infeasible_mip` | 3 | no (control) | 0.000 | 0.000 | 0.71x [0.66x-0.77x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t1_mad_g2_i4` | 52 | no (control) | 0.100 | 0.163 | 0.62x [0.58x-0.66x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t1_mad_rand_g2_i4` | 54 | no (control) | 4.616 | 3.727 | 0.95x [0.78x-1.17x] | 11 | identical source |
| `t2_mkp_30` | 36 | no (control) | 3.342 | 4.295 | 0.87x [0.78x-0.98x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t2_mkp_50` | 58 | no (control) | 5.801 | 8.174 | 0.75x [0.67x-0.83x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t2_cflp_36` | 47 | no (control) | 0.121 | 0.206 | 0.81x [0.71x-0.92x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t2_cflp_49` | 62 | no (control) | 0.406 | 0.417 | 0.95x [0.78x-1.16x] | 11 | identical source |
| `t2_gap_45` | 58 | no (control) | 0.636 | 0.956 | 0.65x [0.57x-0.74x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t2_lotsizing_48` | 80 | no (control) | 5.175 | 8.254 | 0.68x [0.62x-0.73x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t2_binpack_44` | 58 | no (control) | 0.481 | 0.377 | 1.13x [0.94x-1.37x] | 11 | identical source |
| `t2_setcover_50` | 70 | no (control) | 0.184 | 0.172 | 0.99x [0.85x-1.15x] | 11 | identical source |
| `t2_mad_g2_i8` | 64 | no (control) | 0.224 | 0.219 | 1.04x [0.90x-1.20x] | 11 | identical source |
| `t2_mad_g3_i9` | 96 | no (control) | 0.521 | 0.448 | 1.03x [0.93x-1.14x] | 11 | identical source |
| `t2_mad_g4_i12` | 140 | no (control) | 1.374 | 1.565 | 0.84x [0.66x-1.06x] | 11 | identical source |
| `t2_mad_rand_g2_i8` | 66 | no (control) | 10.492 | 5.513 | 1.44x [1.17x-1.77x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t2_mad_rand_g3_i9` | 99 | no (control) | 125.296 | 138.034 | 0.93x [0.87x-1.00x] | 11 | identical source |
| `t3_mkp_200` | 208 | no (control) | 35.068 | 41.023 | 0.91x [0.79x-1.04x] | 5 | identical source |
| `t3_cflp_300` | 334 | possible | 23.551 | 16.232 | 1.29x [0.99x-1.69x] | 5 | no detectable effect |
| `t3_lotsizing_504` | 840 | possible | 42.700 | 52.025 | 0.91x [0.74x-1.11x] | 5 | no detectable effect |
| `t3_sched_1000` | 1100 | possible | 8.276 | 10.753 | 0.77x [0.56x-1.05x] | 5 | no detectable effect |
| `t3_mad_g5_i20` | 220 | no (control) | 8.610 | 7.968 | 0.96x [0.72x-1.28x] | 5 | identical source |
| `t3_mad_g6_i30` | 330 | possible | 15.372 | 11.340 | 1.25x [0.93x-1.69x] | 5 | no detectable effect |
| `t3_mad_g10_i100` | 1300 | possible | 902.328 | 921.484 | 0.95x [0.93x-0.98x] | 5 | SIMD slower |
| `t3_mad_g4_i1000` | 5080 | possible | 575.514 | 550.541 | 0.98x [0.79x-1.20x] | 5 | no detectable effect |
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
| `t1_std_lp` | 54.00 | 54.00 | 0.002 | 0.00 (1) | 0.00 (1) | 0.00 (1 / 0) | **0.00** (1 / 0) |
| `t1_std_lp_max` | 80.00 | 80.00 | 0.002 | 0.00 (1) | 0.00 (1) | 0.00 (1 / 0) | **0.00** (1 / 0) |
| `t1_std_mip` | 58.00 | 64.00 | 0.002 | 0.01 (3) | 0.01 (1) | 0.01 (1 / 0) | **0.01** (1 / 0) |
| `t1_mip_float_obj` | 58.90 | 64.95 | 0.002 | 0.01 (3) | 0.01 (1) | 0.01 (1 / 0) | **0.01** (1 / 0) |
| `t1_whiskas` | 0.52 | 0.52 | 0.002 | 0.00 (1) | 0.00 (1) | 0.00 (1 / 0) | **0.00** (1 / 0) |
| `t1_furniture` | 1075.00 | 1075.00 | 0.002 | 0.02 (1) | 0.02 (1) | 0.02 (1 / 0) | **0.02** (1 / 0) |
| `t1_knapsack_12` | 578.65 | 513.24 | 0.006 | 0.10 (31) | 0.08 (24) | 0.05 (2 / 15) | **0.07** (1 / 15) |
| `t1_infeasible_mip` | 1.50 | 0.00 | 0.001 | 0.00 (5) | 0.00 (0) | 0.00 (0 / 0) | **0.00** (0 / 0) |
| `t1_mad_g2_i4` | 2160.00 | 2160.00 | 0.060 | 0.23 (4) | 0.23 (4) | 0.32 (4 / 2) | **0.10** (1 / 0) |
| `t1_mad_rand_g2_i4` | 4257.00 | 5249.00 | 0.053 | 0.30 (9) | 0.35 (9) | 0.55 (7 / 24) | **4.62** (7 / 24) |
| `t2_mkp_30` | 1966.92 | 1917.24 | 0.028 | 0.78 (77) | 0.86 (77) | 1.76 (60 / 9) | **3.34** (29 / 9) |
| `t2_mkp_50` | 2648.37 | 2620.01 | 0.096 | 3.32 (119) | 4.31 (132) | 4.92 (2 / 5) | **5.80** (1 / 5) |
| `t2_cflp_36` | 781.97 | 801.17 | 0.019 | 0.12 (7) | 0.12 (7) | 0.95 (3 / 2) | **0.12** (1 / 2) |
| `t2_cflp_49` | 827.30 | 909.02 | 0.028 | 0.32 (9) | 0.29 (9) | 2.41 (4 / 20) | **0.41** (1 / 20) |
| `t2_gap_45` | 178.04 | 195.18 | 0.024 | 1.53 (61) | 1.88 (61) | 0.21 (1 / 13) | **0.64** (1 / 13) |
| `t2_lotsizing_48` | 2052.34 | 2245.81 | 0.062 | 2.57 (174) | 1.75 (198) | 6.18 (39 / 36) | **5.17** (15 / 36) |
| `t2_binpack_44` | 29.30 | 30.32 | 0.025 | 31.64 (2469) | 20.70 (1881) | 1.12 (5 / 18) | **0.48** (1 / 18) |
| `t2_setcover_50` | 56.84 | 58.10 | 0.037 | 0.97 (9) | 1.02 (9) | 0.29 (2 / 6) | **0.18** (1 / 6) |
| `t2_mad_g2_i8` | 2160.00 | 2160.00 | 0.116 | 0.52 (8) | 0.49 (8) | 0.74 (10 / 3) | **0.22** (1 / 0) |
| `t2_mad_g3_i9` | 4320.00 | 4320.00 | 0.217 | 1.88 (10) | 1.73 (6) | 1.76 (9 / 3) | **0.52** (1 / 0) |
| `t2_mad_g4_i12` | 7200.00 | 7200.00 | 1.212 | 5.18 (16) | 4.93 (13) | 4.19 (17 / 5) | **1.37** (1 / 0) |
| `t2_mad_rand_g2_i8` | 5894.00 | 6696.00 | 0.164 | 2.37 (43) | 2.08 (39) | 2.01 (43 / 6) | **10.49** (43 / 6) |
| `t2_mad_rand_g3_i9` | 6060.00 | 7958.00 | 0.414 | 96.29 (198) | 38.02 (217) | 70.99 (358 / 20) | **125.30** (325 / 20) |
| `t3_mkp_200` | 12955.46 | 12906.80 | 0.586 | 85.62 (5000, `NodeLimit`) | 133.06 (5000, `NodeLimit`) | 75.08 (719 / 21) | **35.07** (506 / 21) |
| `t3_cflp_300` | 2081.53 | 2187.30 | 0.228 | 8.38 (134) | 8.91 (134) | 22.65 (53 / 38) | **23.55** (17 / 38) |
| `t3_lotsizing_504` | 10518.99 | 13120.44 | 3.037 | 250.05 (3169, `TimeLimit`) | 250.03 (4082, `TimeLimit`) | 156.73 (352 / 163) | **42.70** (11 / 163) |
| `t3_sched_1000` | 46971.09 | 46818.28 | 2.444 | 212.56 (5000, `NodeLimit`) | 250.04 (3571, `TimeLimit`) | 18.85 (96 / 50) | **8.28** (17 / 50) |
| `t3_mad_g5_i20` | 10800.00 | 10800.00 | 5.804 | 17.77 (54) | 11.96 (36) | 16.81 (69 / 3) | **8.61** (1 / 0) |
| `t3_mad_g6_i30` | 15120.00 | 15120.00 | 8.012 | 51.78 (173) | 33.23 (112) | 17.48 (26 / 2) | **15.37** (1 / 0) |
| `t3_mad_g10_i100` | 79200.00 | 79200.00 | 138.508 | 250.02 (10, `TimeLimit`) | 236.82 (210) | 250.07 (208 / 7, `TimeLimit`) | **902.33** (1 / 0) |
| `t3_mad_g4_i1000` | 187200.00 | 187200.00 | 457.171 | 252.53 (0, `TimeLimit`) | 252.44 (0, `TimeLimit`) | 252.83 (0 / 0, `TimeLimit`) | **575.51** (1 / 0) |
<!-- END GENERATED:FULL_CHECKPOINT_TABLE -->
