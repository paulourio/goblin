# `goblin` vs. Python (`PuLP + COIN-OR CBC`) Benchmark Report

## 1. Experimental Setup and Provenance

```bash
go run ./cmd/goblin-bench -check-docs
```

- **CPU**: `13th Gen Intel(R) Core(TM) i9-13900K` (x86_64, 30 logical CPUs exposed by `lscpu`, AVX2 + FMA).
- **Default pure-Go backend**: `goblin` default (`BackendGo`, scalar kernels or `WithoutSIMD()`). All headline `goblin (Go)` timings use this setup.
- **Experimental SIMD build**: `GOEXPERIMENT=simd`, which dispatches AVX2 kernels only for slices of length 256 or more; see section 4.
- **Optional C++ backend**: `goblin.WithCBC()` (`BackendCBC`), driving in-process COIN-OR CBC/CLP through `cgo` under the same unified `goblin` API.
- **Python baseline**: Python 3.14 + `PuLP` + `COIN-OR CBC` subprocess (`threads=1`). Its solve time includes MPS writing, process spawn, and `.sol` parsing.
- **Corpus**: 31 deterministic LP and MILP families across 3 tiers (`testdata/corpus/*.json` and `testdata/corpus/trials/*.jsonl`; 11 randomized trial instances per family for Tiers 1-2 and 5 for Tier 3, 293 instances in total), measured in warm state.

<!-- BEGIN GENERATED:PROVENANCE -->
- **goblin telemetry source**: fingerprint `f409647e4eddfdee` (working tree on top of commit `1b1a98018251d5283ec57c65e12dc0707f2d55f5`), `go1.27.1-X:nodwarf5`; the documentation gate rejects telemetry whose fingerprint differs from the checked-in sources.
- **CPU affinity**: goblin `2`; Python + CBC `2`.
- **Python reference**: PuLP `4.2.0`, CBC `2.10.3` (`cbc`).
- **CBC subprocess spawn floor**: 1.53 ms median (`prob.solve()` on a 1-variable MILP); the "Net of Spawn" columns subtract it.
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `master` (commit `c1c171ec`, vendored in `third_party/Cbc`), linked in-process through `cgo` (`goblin.WithCBC()`). The C++ backend maps default settings onto CBC with two deliberate differences: `MaxCutPasses = 12` becomes `passC = 30` and `MaxNodes = 50000` becomes unlimited. It is therefore not a like-for-like setup and not the CBC 2.10.3 binary used by the Python baseline.
<!-- END GENERATED:PROVENANCE -->

## 2. Aggregate Workload Tier Summary

The planted-gap MAD family is constructed so that a root heuristic closes the gap at node 1; the bold row excludes it.

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families | Py Total GeoMean (ms) | Py Net-of-Spawn GeoMean (ms) | `goblin (CBC)` GeoMean (ms) | `goblin (Go)` GeoMean (ms) | `Py / goblin (Go)` GeoMean | `Py / goblin (Go)` Median [Min-Max] | `Py net / goblin (Go)` GeoMean | `goblin (Go) / CBC` |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical (`N <= 26`) | 10 | 2.50 | 0.08 | 0.51 | 0.04 | **68.2x** | 124.9x [6.2x-172.0x] | 2.2x | 0.07x |
| Tier 2: Target p90 (`30 <= N <= 84`) | 13 | 11.03 | 8.43 | 6.09 | 1.77 | **6.2x** | 7.0x [0.9x-17.9x] | 4.8x | 0.29x |
| Tier 3: Large Tail (`145 <= N <= 4036`) | 8 | 59.90 | 56.86 | 39.83 | 34.04 | **1.8x** | 1.6x [0.5x-7.7x] | 1.7x | 0.85x |
| Tier 3 non-MAD MILPs | 4 | 92.48 | 90.63 | 55.63 | 41.54 | **2.2x** | 2.4x [0.7x-7.7x] | 2.2x | 0.75x |
| Planted-gap MAD family (zero integrality gap by construction) | 8 | 12.66 | 9.57 | 8.05 | 3.29 | **3.8x** | 5.2x [0.5x-18.0x] | 2.9x | 0.41x |
| Random-weight MAD (negative controls) | 3 | 58.88 | 56.80 | 24.56 | 21.86 | **2.7x** | 3.7x [0.9x-6.2x] | 2.6x | 0.89x |
| **All families except planted-gap MAD** | 23 | 9.93 | 2.07 | 3.63 | 0.74 | **13.4x** | 10.9x [0.7x-172.0x] | 2.8x | 0.20x |
| All corpus families | 31 | 10.57 | 3.08 | 4.46 | 1.09 | **9.7x** | 7.7x [0.5x-172.0x] | 2.8x | 0.24x |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

## 3. End-to-End Pipeline Comparison (`JSON -> Solution`)

Pipeline stages measured per instance (median across trials after warmup):
1. **Prep (`t_parse + t_build + t_format/t_compile`)**: JSON unmarshaling, model construction, and MPS formatting (Python) or `Model.Compile()` matrix assembly (Go).
2. **Solve (`t_solve`)**: CBC subprocess execution and `.sol` parsing (Python) vs. in-memory `goblin.SolveCompiled` (Go).
3. **Total (`t_total`)**: Complete end-to-end latency from raw JSON bytes to verified primal solution.

The Parity column is computed by `bench.CheckParity`: status, canonical objective, and every per-trial objective of all `goblin` builds (`scalar`, `simd`, and `cbc`) must match CBC within a relative tolerance of 1e-5, and `ValidateTelemetryAgainstCorpus` rejects the telemetry otherwise.

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | PyPrep (ms) | GoPrep (ms) | PySolve (ms) | PySolve Net of Spawn (ms) | GoSolve (ms) | PyTotal (ms) | GoTotal (ms) | `Py / goblin (Go)` | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | 0.056 | 0.008 | 1.30 | 0.00 | 0.00 | 1.34 | 0.01 | **142.4x** | `OK` |
| `t1_std_lp_max` | 3 | 3 | 0.056 | 0.007 | 1.26 | 0.00 | 0.00 | 1.34 | 0.01 | **152.1x** | `OK` |
| `t1_std_mip` | 3 | 3 | 0.059 | 0.008 | 1.96 | 0.43 | 0.00 | 2.01 | 0.01 | **172.0x** | `OK` |
| `t1_mip_float_obj` | 3 | 3 | 0.057 | 0.008 | 1.87 | 0.33 | 0.00 | 1.91 | 0.01 | **162.5x** | `OK` |
| `t1_whiskas` | 6 | 5 | 0.082 | 0.016 | 1.33 | 0.00 | 0.00 | 1.39 | 0.02 | **65.7x** | `OK` |
| `t1_furniture` | 2 | 3 | 0.057 | 0.008 | 2.18 | 0.65 | 0.01 | 2.23 | 0.02 | **107.4x** | `OK` |
| `t1_knapsack_12` | 12 | 3 | 0.092 | 0.026 | 3.14 | 1.61 | 0.09 | 3.21 | 0.11 | **28.3x** | `OK` |
| `t1_infeasible_mip` | 2 | 1 | 0.046 | 0.008 | 1.42 | 0.00 | 0.00 | 1.45 | 0.01 | **160.9x** | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | 0.246 | 0.097 | 3.63 | 2.10 | 0.07 | 3.82 | 0.21 | **18.0x** | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | 0.281 | 0.095 | 24.53 | 22.99 | 3.89 | 24.75 | 3.98 | **6.2x** | `OK` |
| `t2_mkp_30` | 30 | 6 | 0.208 | 0.066 | 27.50 | 25.97 | 4.00 | 27.67 | 4.07 | **6.8x** | `OK` |
| `t2_mkp_50` | 50 | 8 | 0.324 | 0.114 | 11.23 | 9.69 | 11.55 | 11.47 | 11.66 | **1.0x** | `OK` |
| `t2_cflp_36` | 36 | 11 | 0.191 | 0.059 | 3.39 | 1.86 | 0.26 | 3.56 | 0.33 | **10.9x** | `OK` |
| `t2_cflp_49` | 49 | 13 | 0.259 | 0.081 | 3.96 | 2.42 | 0.67 | 4.15 | 0.77 | **5.4x** | `OK` |
| `t2_gap_45` | 45 | 13 | 0.241 | 0.078 | 8.28 | 6.75 | 0.67 | 8.48 | 0.76 | **11.2x** | `OK` |
| `t2_lotsizing_48` | 48 | 32 | 0.290 | 0.098 | 39.44 | 37.91 | 4.87 | 39.67 | 4.96 | **8.0x** | `OK` |
| `t2_binpack_44` | 44 | 14 | 0.212 | 0.074 | 6.57 | 5.04 | 0.37 | 6.73 | 0.45 | **15.0x** | `OK` |
| `t2_setcover_50` | 50 | 20 | 0.353 | 0.124 | 5.11 | 3.57 | 0.16 | 5.37 | 0.30 | **17.9x** | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | 0.329 | 0.112 | 3.18 | 1.65 | 0.09 | 3.43 | 0.20 | **17.0x** | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | 0.486 | 0.177 | 3.83 | 2.30 | 0.37 | 4.21 | 0.60 | **7.0x** | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | 0.740 | 0.299 | 4.71 | 3.17 | 0.53 | 5.28 | 0.89 | **5.9x** | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | 0.382 | 0.143 | 42.91 | 41.38 | 11.59 | 43.22 | 11.71 | **3.7x** | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | 0.542 | 0.207 | 190.46 | 188.93 | 224.03 | 190.87 | 224.25 | **0.9x** | `OK` |
| `t3_mkp_200` | 200 | 8 | 0.782 | 0.294 | 40.62 | 39.09 | 61.92 | 41.23 | 62.21 | **0.7x** | `OK` |
| `t3_cflp_300` | 300 | 34 | 1.147 | 0.437 | 210.13 | 208.60 | 26.88 | 211.02 | 27.44 | **7.7x** | `OK` |
| `t3_lotsizing_504` | 504 | 336 | 2.268 | 0.771 | 126.64 | 125.11 | 38.39 | 128.49 | 39.16 | **3.3x** | `OK` |
| `t3_sched_1000` | 1000 | 100 | 3.667 | 1.264 | 62.22 | 60.69 | 43.29 | 65.45 | 44.56 | **1.5x** | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | 1.308 | 0.520 | 8.88 | 7.35 | 1.64 | 9.85 | 2.22 | **4.4x** | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | 2.243 | 0.828 | 10.99 | 9.45 | 6.81 | 12.68 | 7.60 | **1.7x** | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | 10.812 | 4.834 | 37.92 | 36.39 | 84.36 | 46.51 | 89.45 | **0.5x** | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | 44.695 | 16.110 | 345.93 | 344.40 | 385.49 | 389.97 | 400.91 | **1.0x** | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

## 4. Scalar vs. `GOEXPERIMENT=simd` Build

Ratios are geometric means of per-trial `scalar / SIMD` solve times over the same instances, with 95% t-intervals on the log ratios. "Reachable" is a necessary condition only: the AVX2 `Dot64`/`Axpy64` paths need a slice of length at least 256, which requires `n + m >= 256`.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 256`, no AVX2 kernel can run): 4 of 25 have a 95% interval excluding 1. These differences come from the two binaries (compiler output and code layout differ under `GOEXPERIMENT=simd`) and from run-to-run effects, not from SIMD kernels. Intervals of similar size on the AVX2-capable instances are therefore not evidence of a SIMD effect.
- **Instances where AVX2 can run**: 0 faster, 1 slower, 5 with no detectable effect, out of 6.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Scalar Solve Median (ms) | SIMD Solve Median (ms) | Paired `scalar / SIMD` GeoMean [95% CI] | Trials | Verdict |
| :--- | ---: | :---: | ---: | ---: | ---: | ---: | :--- |
| `t1_std_lp` | 6 | no (control) | 0.001 | 0.001 | 0.92x [0.76x-1.11x] | 11 | identical source |
| `t1_std_lp_max` | 6 | no (control) | 0.001 | 0.001 | 1.35x [0.68x-2.70x] | 11 | identical source |
| `t1_std_mip` | 6 | no (control) | 0.003 | 0.003 | 1.26x [1.09x-1.45x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t1_mip_float_obj` | 6 | no (control) | 0.003 | 0.004 | 0.75x [0.44x-1.31x] | 11 | identical source |
| `t1_whiskas` | 11 | no (control) | 0.002 | 0.003 | 0.73x [0.59x-0.91x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t1_furniture` | 5 | no (control) | 0.009 | 0.017 | 0.54x [0.45x-0.64x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t1_knapsack_12` | 15 | no (control) | 0.087 | 0.067 | 1.16x [0.93x-1.45x] | 11 | identical source |
| `t1_infeasible_mip` | 3 | no (control) | 0.000 | 0.000 | 1.94x [0.81x-4.69x] | 11 | identical source |
| `t1_mad_g2_i4` | 60 | no (control) | 0.071 | 0.075 | 1.06x [0.92x-1.22x] | 11 | identical source |
| `t1_mad_rand_g2_i4` | 60 | no (control) | 3.887 | 3.791 | 1.05x [0.94x-1.18x] | 11 | identical source |
| `t2_mkp_30` | 36 | no (control) | 4.003 | 4.441 | 0.98x [0.90x-1.06x] | 11 | identical source |
| `t2_mkp_50` | 58 | no (control) | 11.546 | 11.208 | 1.01x [0.98x-1.05x] | 11 | identical source |
| `t2_cflp_36` | 47 | no (control) | 0.260 | 0.268 | 0.98x [0.96x-1.01x] | 11 | identical source |
| `t2_cflp_49` | 62 | no (control) | 0.668 | 0.636 | 1.02x [0.95x-1.09x] | 11 | identical source |
| `t2_gap_45` | 58 | no (control) | 0.666 | 0.777 | 1.00x [0.93x-1.06x] | 11 | identical source |
| `t2_lotsizing_48` | 80 | no (control) | 4.867 | 4.638 | 1.01x [0.97x-1.04x] | 11 | identical source |
| `t2_binpack_44` | 58 | no (control) | 0.370 | 0.376 | 1.09x [0.99x-1.20x] | 11 | identical source |
| `t2_setcover_50` | 70 | no (control) | 0.157 | 0.118 | 1.07x [0.91x-1.25x] | 11 | identical source |
| `t2_mad_g2_i8` | 72 | no (control) | 0.088 | 0.089 | 1.00x [0.98x-1.02x] | 11 | identical source |
| `t2_mad_g3_i9` | 108 | no (control) | 0.368 | 0.367 | 0.99x [0.87x-1.12x] | 11 | identical source |
| `t2_mad_g4_i12` | 156 | no (control) | 0.535 | 0.632 | 1.00x [0.90x-1.11x] | 11 | identical source |
| `t2_mad_rand_g2_i8` | 72 | no (control) | 11.585 | 12.608 | 1.01x [0.93x-1.08x] | 11 | identical source |
| `t2_mad_rand_g3_i9` | 108 | no (control) | 224.034 | 218.819 | 1.02x [1.01x-1.03x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t3_mkp_200` | 208 | no (control) | 61.918 | 62.860 | 0.98x [0.95x-1.01x] | 5 | identical source |
| `t3_cflp_300` | 334 | possible | 26.882 | 27.184 | 0.97x [0.94x-1.01x] | 5 | no detectable effect |
| `t3_lotsizing_504` | 840 | possible | 38.391 | 38.886 | 0.98x [0.97x-1.00x] | 5 | no detectable effect |
| `t3_sched_1000` | 1100 | possible | 43.287 | 42.567 | 0.98x [0.90x-1.06x] | 5 | no detectable effect |
| `t3_mad_g5_i20` | 240 | no (control) | 1.639 | 1.625 | 1.02x [0.90x-1.15x] | 5 | identical source |
| `t3_mad_g6_i30` | 354 | possible | 6.807 | 7.138 | 0.96x [0.88x-1.06x] | 5 | no detectable effect |
| `t3_mad_g10_i100` | 1340 | possible | 84.358 | 86.862 | 0.97x [0.96x-0.99x] | 5 | SIMD slower |
| `t3_mad_g4_i1000` | 5096 | possible | 385.490 | 381.853 | 1.00x [0.97x-1.02x] | 5 | no detectable effect |
<!-- END GENERATED:SIMD_EFFECT_TABLE -->

## 5. Solver-Core Checkpoint Progression (default pure-Go backend)

Progression across checkpoints on pre-compiled `CompiledProblem` instances:
- **CP2/3 (Root LP)**: Revised Dual and Two-Phase Primal Simplex.
- **CP4 (Base B&B)**: Branch-and-Bound with pseudocost/strong branching (`Presolve=off, Cuts=off, Heuristics=off`).
- **CP5 (+Presolve)**: Savelsbergh bound tightening, GCD reduction, and node propagation (`Presolve=on, Cuts=off, Heuristics=off`).
- **CP6 (+Cuts)**: GMI, lifted 0-1 knapsack covers, flow/VUB and $(l, S)$ cuts, and single-row MIR cuts (`Presolve=on, Cuts=on, Heuristics=off`).
- **CP7 (Full `goblin`)**: Full Branch-and-Cut with Propagated Rounding, Root Sub-MIP, and Fractional Diving (`Presolve=on, Cuts=on, Heuristics=on`).

<!-- BEGIN GENERATED:FULL_CHECKPOINT_TABLE -->
| Problem ID | Root LP Obj | Optimal Obj | Root LP (ms) | CP4 Base (ms / nodes) | CP5 +Presolve (ms / nodes) | CP6 +Cuts (ms / nodes / cuts) | CP7 Full (ms / nodes / cuts) |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 54.00 | 54.00 | 0.001 | 0.00 (1) | 0.00 (1) | 0.00 (1 / 0) | **0.00** (1 / 0) |
| `t1_std_lp_max` | 80.00 | 80.00 | 0.001 | 0.00 (1) | 0.00 (1) | 0.00 (1 / 0) | **0.00** (1 / 0) |
| `t1_std_mip` | 58.00 | 64.00 | 0.001 | 0.00 (3) | 0.00 (1) | 0.00 (1 / 0) | **0.00** (1 / 0) |
| `t1_mip_float_obj` | 58.90 | 64.95 | 0.001 | 0.00 (3) | 0.00 (1) | 0.00 (1 / 0) | **0.00** (1 / 0) |
| `t1_whiskas` | 0.52 | 0.52 | 0.002 | 0.00 (1) | 0.00 (1) | 0.00 (1 / 0) | **0.00** (1 / 0) |
| `t1_furniture` | 1075.00 | 1075.00 | 0.001 | 0.01 (1) | 0.01 (1) | 0.01 (1 / 0) | **0.01** (1 / 0) |
| `t1_knapsack_12` | 578.65 | 513.24 | 0.005 | 0.12 (31) | 0.09 (24) | 0.09 (2 / 15) | **0.09** (1 / 15) |
| `t1_infeasible_mip` | 1.50 | 0.00 | 0.001 | 0.00 (5) | 0.00 (0) | 0.00 (0 / 0) | **0.00** (0 / 0) |
| `t1_mad_g2_i4` | 2400.00 | 2400.00 | 0.054 | 0.26 (3) | 0.26 (3) | 0.23 (2 / 2) | **0.07** (1 / 0) |
| `t1_mad_rand_g2_i4` | 4257.00 | 4601.00 | 0.063 | 0.79 (84) | 0.91 (84) | 1.14 (21 / 2) | **3.89** (11 / 2) |
| `t2_mkp_30` | 1966.92 | 1917.24 | 0.019 | 0.72 (77) | 0.93 (77) | 1.89 (60 / 9) | **4.00** (29 / 9) |
| `t2_mkp_50` | 2648.37 | 2620.01 | 0.030 | 3.28 (119) | 3.85 (132) | 3.76 (2 / 5) | **11.55** (1 / 5) |
| `t2_cflp_36` | 781.97 | 801.17 | 0.021 | 0.13 (7) | 0.14 (7) | 0.87 (3 / 2) | **0.26** (1 / 2) |
| `t2_cflp_49` | 827.30 | 909.02 | 0.033 | 0.29 (9) | 0.29 (9) | 1.31 (4 / 20) | **0.67** (1 / 20) |
| `t2_gap_45` | 178.04 | 195.18 | 0.029 | 1.58 (61) | 1.48 (61) | 0.20 (1 / 13) | **0.67** (1 / 13) |
| `t2_lotsizing_48` | 2052.34 | 2245.81 | 0.065 | 2.06 (174) | 1.73 (198) | 4.97 (33 / 40) | **4.87** (11 / 40) |
| `t2_binpack_44` | 29.30 | 30.32 | 0.036 | 24.80 (2400) | 16.45 (1895) | 1.24 (18 / 12) | **0.37** (1 / 12) |
| `t2_setcover_50` | 56.84 | 58.10 | 0.029 | 0.64 (9) | 0.64 (9) | 0.19 (2 / 6) | **0.16** (1 / 6) |
| `t2_mad_g2_i8` | 2400.00 | 2400.00 | 0.072 | 0.07 (1) | 0.08 (1) | 0.08 (1 / 0) | **0.09** (1 / 0) |
| `t2_mad_g3_i9` | 4800.00 | 4800.00 | 0.178 | 1.24 (15) | 1.66 (18) | 1.72 (35 / 3) | **0.37** (1 / 0) |
| `t2_mad_g4_i12` | 8000.00 | 8000.00 | 0.268 | 2.84 (26) | 3.71 (11) | 3.63 (82 / 4) | **0.53** (1 / 0) |
| `t2_mad_rand_g2_i8` | 5894.00 | 5975.00 | 0.097 | 4.36 (126) | 3.43 (415) | 8.89 (846 / 6) | **11.59** (47 / 6) |
| `t2_mad_rand_g3_i9` | 6060.00 | 7720.00 | 0.217 | 207.06 (3177) | 97.61 (1899) | 180.40 (3069 / 15) | **224.03** (2101 / 15) |
| `t3_mkp_200` | 12955.46 | 12906.80 | 0.170 | 79.56 (5000, `NodeLimit`) | 98.33 (5000, `NodeLimit`) | 52.66 (739 / 21) | **61.92** (540 / 21) |
| `t3_cflp_300` | 2081.53 | 2187.30 | 0.139 | 6.25 (134) | 5.66 (134) | 15.21 (39 / 37) | **26.88** (9 / 37) |
| `t3_lotsizing_504` | 10518.99 | 13120.44 | 1.307 | 210.05 (5000, `NodeLimit`) | 215.56 (5000, `NodeLimit`) | 85.45 (365 / 165) | **38.39** (11 / 165) |
| `t3_sched_1000` | 46971.09 | 46818.28 | 0.162 | 160.16 (5000, `NodeLimit`) | 218.76 (5000, `NodeLimit`) | 10.37 (96 / 50) | **43.29** (17 / 50) |
| `t3_mad_g5_i20` | 12000.00 | 12000.00 | 0.673 | 13.07 (39) | 21.03 (213) | 21.43 (123 / 9) | **1.64** (1 / 0) |
| `t3_mad_g6_i30` | 16800.00 | 16800.00 | 4.972 | 162.28 (180) | 41.89 (136) | 24.47 (106 / 7) | **6.81** (1 / 0) |
| `t3_mad_g10_i100` | 88000.00 | 88000.00 | 78.835 | 250.14 (4, `TimeLimit`) | 250.48 (8, `TimeLimit`) | 250.20 (3 / 6, `TimeLimit`) | **84.36** (1 / 0) |
| `t3_mad_g4_i1000` | 208000.00 | 208000.00 | 373.324 | 250.19 (0, `TimeLimit`) | 250.02 (0, `TimeLimit`) | 251.56 (0 / 0, `TimeLimit`) | **385.49** (1 / 0) |
<!-- END GENERATED:FULL_CHECKPOINT_TABLE -->
