# `goblin` vs. Python (`PuLP + COIN-OR CBC`) Benchmark Report

## 1. Experimental Setup and Provenance

```bash
go run ./cmd/goblin-bench -check-docs
```

- **CPU**: `13th Gen Intel(R) Core(TM) i9-13900K` (x86_64, 30 logical CPUs exposed by `lscpu`, AVX2 + FMA).
- **Default pure-Go backend**: `goblin` default (`BackendGo`, scalar kernels or `WithoutSIMD()`). All headline `goblin (Go)` timings use this setup.
- **Experimental SIMD build**: `GOEXPERIMENT=simd`, which dispatches AVX2 kernels for slices of length 32 or more; see section 4.
- **Optional C++ backend**: `gb.WithCBC()` (`BackendCBC`, built with `-tags cbc`), driving in-process COIN-OR CBC/CLP through `cgo` under the same unified `goblin` API.
- **Python baseline**: Python 3.14 + `PuLP` + `COIN-OR CBC` subprocess (`threads=1`). Its solve time includes MPS writing, process spawn, and `.sol` parsing.
- **Corpus**: 31 deterministic LP and MILP families across 3 tiers (`testdata/corpus/*.json` and `testdata/corpus/trials/*.jsonl`; 11 randomized trial instances per family for Tiers 1-2 and 5 for Tier 3, 293 instances in total), measured in warm state.

<!-- BEGIN GENERATED:PROVENANCE -->
- **goblin telemetry source**: fingerprint `80d7e0984eddd6e7` (working tree on top of commit `a1011bc8fdb9c25f0d0d595087e3b66ed1b32021`), `go1.27.3-X:nodwarf5`; the documentation gate rejects telemetry whose fingerprint differs from the checked-in sources.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, CBC `2.10.3` (`cbc`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 1.53 ms median (`prob.solve()` on a 1-variable MILP); the "Net of Spawn" columns subtract it.
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `master` (commit `c1c171ec`, vendored in `third_party/Cbc`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). The C++ backend maps default settings onto CBC with two deliberate differences: `MaxCutPasses = 12` becomes `passC = 30` and `MaxNodes = 50000` becomes unlimited. It is therefore not a like-for-like setup and not the CBC 2.10.3 binary used by the Python baseline.
<!-- END GENERATED:PROVENANCE -->

## 2. Aggregate Workload Tier Summary

The planted-gap MAD family is constructed so that a root heuristic closes the gap at node 1; the bold row excludes it.

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families | `Py / goblin (Go)` GeoMean | `Py net / goblin (Go)` | `goblin (Go) / CBC` | `goblin (Go)` (ms) | `goblin (CBC)` (ms) | Py Total (ms) | Py Net (ms) | `Py / goblin (Go)` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical (`N <= 26`) | 10 | **40.6x** | 1.3x | 0.07x | 0.06 | 0.87 | 2.50 | 0.08 | 55.2x [11.9x-87.5x] |
| Tier 2: Target p90 (`30 <= N <= 84`) | 13 | **6.6x** | 5.0x | 0.17x | 1.68 | 10.14 | 11.03 | 8.43 | 6.5x [1.5x-12.5x] |
| Tier 3: Large Tail (`145 <= N <= 4036`) | 8 | **2.9x** | 2.8x | 0.29x | 20.52 | 71.38 | 59.90 | 56.86 | 4.0x [0.7x-8.2x] |
| Tier 3 non-MAD MILPs | 4 | **2.2x** | 2.1x | 0.45x | 42.98 | 96.17 | 92.48 | 90.63 | 2.7x [0.7x-8.2x] |
| Planted-gap MAD family (zero integrality gap by construction) | 8 | **5.8x** | 4.4x | 0.14x | 2.17 | 15.03 | 12.66 | 9.57 | 4.5x [3.6x-11.9x] |
| Random-weight MAD (negative controls) | 3 | **7.8x** | 7.5x | 0.18x | 7.57 | 42.97 | 58.88 | 56.80 | 6.4x [4.5x-16.5x] |
| **All families except planted-gap MAD** | 23 | **11.4x** | 2.4x | 0.15x | 0.87 | 5.98 | 9.93 | 2.07 | 9.5x [0.7x-87.5x] |
| All corpus families | 31 | **9.6x** | 2.8x | 0.15x | 1.10 | 7.59 | 10.57 | 3.08 | 8.2x [0.7x-87.5x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

## 3. End-to-End Pipeline Comparison (`JSON -> Solution`)

Pipeline stages measured per instance (median across trials after warmup):
1. **Prep (`t_parse + t_build + t_format/t_compile`)**: JSON unmarshaling, model construction, and MPS formatting (Python) or `Model.Compile()` matrix assembly (Go).
2. **Solve (`t_solve`)**: CBC subprocess execution and `.sol` parsing (Python) vs. in-memory `gb.SolveCompiled` (Go).
3. **Total (`t_total`)**: Complete end-to-end latency from raw JSON bytes to verified primal solution.

The Parity column is computed by `bench.CheckParity`: status, canonical objective, and every per-trial objective of all `goblin` builds (`scalar`, `simd`, and `cbc`) must match CBC within a relative tolerance of 1e-5, and `ValidateTelemetryAgainstCorpus` rejects the telemetry otherwise.

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | `Py / goblin (Go)` | GoTotal (ms) | PyTotal (ms) | GoSolve (ms) | PySolve (ms) | PySolve Net (ms) | GoPrep (ms) | PyPrep (ms) | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | **65.8x** | 0.02 | 1.34 | 0.00 | 1.30 | 0.00 | 0.017 | 0.056 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **63.3x** | 0.02 | 1.34 | 0.00 | 1.26 | 0.00 | 0.017 | 0.056 | `OK` |
| `t1_std_mip` | 3 | 3 | **87.5x** | 0.02 | 2.01 | 0.01 | 1.96 | 0.43 | 0.017 | 0.059 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **66.0x** | 0.03 | 1.91 | 0.01 | 1.87 | 0.33 | 0.020 | 0.057 | `OK` |
| `t1_whiskas` | 6 | 5 | **28.4x** | 0.05 | 1.39 | 0.01 | 1.33 | 0.00 | 0.043 | 0.082 | `OK` |
| `t1_furniture` | 2 | 3 | **47.2x** | 0.05 | 2.23 | 0.02 | 2.18 | 0.65 | 0.018 | 0.057 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **23.0x** | 0.14 | 3.21 | 0.09 | 3.14 | 1.61 | 0.042 | 0.092 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **83.1x** | 0.02 | 1.45 | 0.00 | 1.42 | 0.00 | 0.015 | 0.046 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **11.9x** | 0.32 | 3.82 | 0.11 | 3.63 | 2.10 | 0.207 | 0.246 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **16.5x** | 1.50 | 24.75 | 1.21 | 24.53 | 22.99 | 0.225 | 0.281 | `OK` |
| `t2_mkp_30` | 30 | 6 | **6.5x** | 4.28 | 27.67 | 4.12 | 27.50 | 25.97 | 0.158 | 0.208 | `OK` |
| `t2_mkp_50` | 50 | 8 | **1.5x** | 7.79 | 11.47 | 7.53 | 11.23 | 9.69 | 0.251 | 0.324 | `OK` |
| `t2_cflp_36` | 36 | 11 | **9.5x** | 0.37 | 3.56 | 0.25 | 3.39 | 1.86 | 0.105 | 0.191 | `OK` |
| `t2_cflp_49` | 49 | 13 | **7.4x** | 0.56 | 4.15 | 0.43 | 3.96 | 2.42 | 0.125 | 0.259 | `OK` |
| `t2_gap_45` | 45 | 13 | **6.1x** | 1.39 | 8.48 | 1.23 | 8.28 | 6.75 | 0.164 | 0.241 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **5.7x** | 6.98 | 39.67 | 6.84 | 39.44 | 37.91 | 0.198 | 0.290 | `OK` |
| `t2_binpack_44` | 44 | 14 | **12.5x** | 0.54 | 6.73 | 0.45 | 6.57 | 5.04 | 0.090 | 0.212 | `OK` |
| `t2_setcover_50` | 50 | 20 | **11.2x** | 0.48 | 5.37 | 0.16 | 5.11 | 3.57 | 0.323 | 0.353 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **10.0x** | 0.34 | 3.43 | 0.14 | 3.18 | 1.65 | 0.168 | 0.329 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **9.8x** | 0.43 | 4.21 | 0.16 | 3.83 | 2.30 | 0.233 | 0.486 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **4.7x** | 1.13 | 5.28 | 0.48 | 4.71 | 3.17 | 0.653 | 0.740 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **6.4x** | 6.78 | 43.22 | 6.50 | 42.91 | 41.38 | 0.276 | 0.382 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **4.5x** | 42.57 | 190.87 | 42.21 | 190.46 | 188.93 | 0.418 | 0.542 | `OK` |
| `t3_mkp_200` | 200 | 8 | **0.7x** | 57.28 | 41.23 | 56.68 | 40.62 | 39.09 | 0.666 | 0.782 | `OK` |
| `t3_cflp_300` | 300 | 34 | **8.2x** | 25.71 | 211.02 | 24.74 | 210.13 | 208.60 | 1.012 | 1.147 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **0.8x** | 160.41 | 128.49 | 158.30 | 126.64 | 125.11 | 1.714 | 2.268 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **4.5x** | 14.44 | 65.45 | 11.83 | 62.22 | 60.69 | 2.750 | 3.667 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **4.4x** | 2.24 | 9.85 | 0.86 | 8.88 | 7.35 | 1.108 | 1.308 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **3.8x** | 3.37 | 12.68 | 1.50 | 10.99 | 9.45 | 1.867 | 2.243 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **3.6x** | 13.00 | 46.51 | 7.06 | 37.92 | 36.39 | 6.773 | 10.812 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **4.2x** | 93.67 | 389.97 | 61.25 | 345.93 | 344.40 | 30.001 | 44.695 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

## 4. Scalar vs. `GOEXPERIMENT=simd` Build

Ratios are geometric means of per-trial `scalar / SIMD` solve times over the same instances, with 95% t-intervals on the log ratios. "Reachable" is a necessary condition only: the AVX2 `Dot64`/`Axpy64`/`Scale64` paths need a slice of length at least 32, which requires `n + m >= 32`.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 4 of 8 have a 95% interval excluding 1. These differences come from the two binaries (compiler output and code layout differ under `GOEXPERIMENT=simd`) and from run-to-run effects, not from SIMD kernels. Intervals of similar size on the AVX2-capable instances are therefore not evidence of a SIMD effect.
- **Instances where AVX2 can run**: 6 faster, 4 slower, 13 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.73x [0.60x-0.90x] | 0.003 | 0.004 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 0.92x [0.77x-1.10x] | 0.003 | 0.003 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 0.83x [0.60x-1.15x] | 0.006 | 0.006 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.55x [1.31x-1.82x] | 0.008 | 0.006 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 1.10x [0.99x-1.22x] | 0.006 | 0.006 | 11 |
| `t1_furniture` | 5 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.13x [1.07x-1.20x] | 0.023 | 0.018 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source | 0.91x [0.80x-1.03x] | 0.094 | 0.097 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.52x [1.40x-1.65x] | 0.001 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 1.10x [0.94x-1.29x] | 0.111 | 0.097 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 1.08x [0.90x-1.31x] | 1.208 | 1.009 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 0.99x [0.88x-1.12x] | 4.121 | 4.523 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 1.00x [0.88x-1.13x] | 7.532 | 8.198 | 11 |
| `t2_cflp_36` | 47 | possible | SIMD slower | 0.83x [0.73x-0.95x] | 0.252 | 0.270 | 11 |
| `t2_cflp_49` | 62 | possible | SIMD slower | 0.88x [0.79x-0.99x] | 0.430 | 0.594 | 11 |
| `t2_gap_45` | 58 | possible | SIMD faster | 1.16x [1.02x-1.32x] | 1.234 | 1.138 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 0.92x [0.81x-1.04x] | 6.843 | 7.037 | 11 |
| `t2_binpack_44` | 58 | possible | SIMD slower | 0.71x [0.64x-0.78x] | 0.452 | 0.731 | 11 |
| `t2_setcover_50` | 70 | possible | SIMD faster | 1.60x [1.42x-1.79x] | 0.157 | 0.109 | 11 |
| `t2_mad_g2_i8` | 72 | possible | SIMD faster | 1.51x [1.20x-1.90x] | 0.143 | 0.093 | 11 |
| `t2_mad_g3_i9` | 108 | possible | SIMD faster | 1.33x [1.11x-1.59x] | 0.160 | 0.117 | 11 |
| `t2_mad_g4_i12` | 156 | possible | SIMD faster | 1.32x [1.11x-1.57x] | 0.478 | 0.384 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | SIMD faster | 1.18x [1.02x-1.35x] | 6.502 | 6.295 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 0.95x [0.85x-1.07x] | 42.208 | 53.020 | 11 |
| `t3_mkp_200` | 208 | possible | no detectable effect | 1.06x [0.91x-1.22x] | 56.678 | 49.802 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.04x [0.99x-1.10x] | 24.739 | 24.351 | 5 |
| `t3_lotsizing_504` | 840 | possible | no detectable effect | 1.05x [0.98x-1.12x] | 158.299 | 148.219 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 0.87x [0.72x-1.06x] | 11.827 | 13.097 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 1.22x [0.67x-2.22x] | 0.857 | 0.709 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 0.94x [0.82x-1.08x] | 1.503 | 1.564 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | SIMD slower | 0.76x [0.61x-0.93x] | 7.056 | 9.358 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | no detectable effect | 0.91x [0.77x-1.08x] | 61.248 | 66.617 | 5 |
<!-- END GENERATED:SIMD_EFFECT_TABLE -->

## 5. Solver-Core Checkpoint Progression (default pure-Go backend)

Progression across checkpoints on pre-compiled `CompiledProblem` instances:
- **CP2/3 (Root LP)**: Revised Dual and Two-Phase Primal Simplex.
- **CP4 (Base B&B)**: Branch-and-Bound with pseudocost/strong branching (`Presolve=off, Cuts=off, Heuristics=off`).
- **CP5 (+Presolve)**: Savelsbergh bound tightening, GCD reduction, and node propagation (`Presolve=on, Cuts=off, Heuristics=off`).
- **CP6 (+Cuts)**: GMI, lifted 0-1 knapsack covers, flow/VUB and $(l, S)$ cuts, and single-row MIR cuts (`Presolve=on, Cuts=on, Heuristics=off`).
- **CP7 (Full `goblin`)**: Full Branch-and-Cut with Propagated Rounding, Root Sub-MIP, and Fractional Diving (`Presolve=on, Cuts=on, Heuristics=on`).

<!-- BEGIN GENERATED:FULL_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Optimal Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_std_lp` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 54.00 | 54.00 |
| `t1_std_lp_max` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 80.00 | 80.00 |
| `t1_std_mip` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.003 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.003 | 64.95 | 58.90 |
| `t1_whiskas` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.004 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.02 (1) | 0.02 (1) | 0.003 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.09** (1 / 8) | 0.14 (2 / 8) | 0.19 (24) | 0.27 (31) | 0.008 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.01 (5) | 0.002 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.11** (1 / 0) | 0.07 (1 / 0) | 0.09 (1) | 0.56 (3) | 0.119 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **1.21** (5 / 0) | 0.84 (12 / 0) | 0.84 (12) | 0.99 (84) | 0.095 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **4.12** (29 / 8) | 3.32 (60 / 8) | 1.15 (77) | 1.01 (77) | 0.023 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **7.53** (1 / 4) | 5.64 (2 / 4) | 8.62 (132) | 5.86 (119) | 0.051 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.25** (1 / 2) | 1.63 (3 / 2) | 0.16 (7) | 0.17 (7) | 0.033 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.43** (1 / 20) | 1.88 (4 / 20) | 0.34 (9) | 0.34 (9) | 0.030 | 909.02 | 827.30 |
| `t2_gap_45` | **1.23** (1 / 8) | 0.37 (1 / 8) | 2.53 (61) | 2.58 (61) | 0.025 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **6.84** (3 / 35) | 7.02 (33 / 35) | 3.02 (198) | 3.43 (174) | 0.086 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **0.45** (1 / 5) | 1.53 (13 / 5) | 24.20 (1838) | 31.58 (2450) | 0.040 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.16** (1 / 6) | 0.35 (2 / 6) | 0.88 (9) | 0.88 (9) | 0.046 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.14** (1 / 0) | 0.52 (9 / 0) | 0.44 (9) | 0.22 (2) | 0.095 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.16** (1 / 0) | 0.23 (1 / 0) | 0.22 (1) | 2.05 (136) | 0.297 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.48** (1 / 0) | 2.22 (15 / 0) | 2.38 (15) | 7.57 (47) | 0.399 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **6.50** (43 / 0) | 4.44 (41 / 0) | 3.39 (41) | 5.48 (126) | 0.192 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **42.21** (137 / 0) | 41.85 (201 / 0) | 20.57 (201) | 250.07 (1426, `TimeLimit`) | 0.395 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **56.68** (594 / 14) | 68.28 (701 / 14) | 161.53 (5000, `NodeLimit`) | 117.07 (5000, `NodeLimit`) | 0.178 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **24.74** (48 / 31) | 28.17 (63 / 31) | 10.96 (134) | 12.55 (134) | 0.238 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **158.30** (27 / 203) | 198.38 (416 / 203) | 250.02 (3083, `TimeLimit`) | 250.11 (3673, `TimeLimit`) | 1.167 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **11.83** (35 / 47) | 13.81 (96 / 47) | 250.04 (3400, `TimeLimit`) | 250.04 (4246, `TimeLimit`) | 0.206 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.86** (1 / 0) | 3.16 (14 / 0) | 2.08 (14) | 1.55 (1) | 1.548 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **1.50** (1 / 0) | 3.51 (12 / 0) | 5.41 (12) | 9.82 (27) | 2.399 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.06** (1 / 0) | 55.48 (130 / 0) | 52.47 (130) | 250.31 (201, `TimeLimit`) | 16.753 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **61.25** (1 / 0) | 76.75 (2 / 0) | 58.17 (2) | 251.43 (40, `TimeLimit`) | 190.467 | 208000.00 | 208000.00 |
<!-- END GENERATED:FULL_CHECKPOINT_TABLE -->

## 6. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family Linear Sum Assignment corpus (`testdata/assign_corpus/manifest.json`; 11 randomized trials per family for Tiers 1-2 and 5 for Tier 3, 200 instances in total) compares pure-Go `assign.LinearSumFlat` against SciPy's C++ `rectangular_lsap` extension in warm state on bit-identical `float64` cost matrices.

### 6.1 Aggregate LSAP Tier Summary

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families | `SciPy Solve / goblin` GeoMean | `SciPy Total / goblin` GeoMean | `goblin` Solve (ms) | SciPy Solve (ms) | `goblin` Total (ms) | SciPy Total (ms) | `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 | **1.84x** | 4.52x | 0.0026 | 0.0047 | 0.0034 | 0.0156 | 1.59x [1.38x-3.11x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 | **1.77x** | 1.63x | 0.104 | 0.183 | 0.159 | 0.260 | 1.61x [1.02x-6.92x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 | **1.37x** | 1.43x | 4.055 | 5.553 | 6.256 | 8.946 | 1.27x [0.89x-2.09x] |
| Square families ($m = n$) | 16 | **1.84x** | 2.29x | 0.164 | 0.302 | 0.220 | 0.504 | 1.82x [0.89x-6.92x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 | **1.23x** | 1.57x | 0.0542 | 0.0667 | 0.105 | 0.165 | 1.23x [1.00x-1.53x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 | **1.59x** | 1.87x | 0.124 | 0.198 | 0.170 | 0.318 | 1.38x [0.89x-6.92x] |
| **All 22 LSAP corpus families** | 22 | **1.65x** | 2.07x | 0.121 | 0.200 | 0.180 | 0.372 | 1.57x [0.89x-6.92x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 6.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | `SciPy Solve / goblin` | GoSolve (ms) | SciPySolve (ms) | GoTotal (ms) | SciPyTotal (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Optimal Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **3.11x** | 0.0006 | 0.0019 | 0.0007 | 0.0122 | $0.0006 \pm 0.0002$ | $0.0020 \pm 0.0003$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **2.44x** | 0.0013 | 0.0032 | 0.0017 | 0.0106 | $0.0015 \pm 0.0005$ | $0.0032 \pm 0.0004$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.64x** | 0.0043 | 0.0070 | 0.0053 | 0.0157 | $0.0043 \pm 0.0011$ | $0.0071 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.49x** | 0.0031 | 0.0047 | 0.0042 | 0.0143 | $0.0029 \pm 0.0007$ | $0.0047 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.53x** | 0.0032 | 0.0050 | 0.0044 | 0.0152 | $0.0033 \pm 0.0008$ | $0.0051 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.38x** | 0.0082 | 0.0113 | 0.0138 | 0.0321 | $0.0087 \pm 0.0029$ | $0.0112 \pm 0.0018$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **1.86x** | 0.0362 | 0.0673 | 0.0445 | 0.0915 | $0.0352 \pm 0.0066$ | $0.0664 \pm 0.0100$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **2.24x** | 0.0747 | 0.167 | 0.0943 | 0.218 | $0.0761 \pm 0.0139$ | $0.168 \pm 0.0258$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **1.51x** | 0.0596 | 0.0902 | 0.0883 | 0.135 | $0.0581 \pm 0.0069$ | $0.0929 \pm 0.0137$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.13x** | 0.187 | 0.210 | 0.205 | 0.249 | $0.214 \pm 0.0645$ | $0.227 \pm 0.0413$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **6.92x** | 0.0934 | 0.647 | 0.134 | 0.722 | $0.0912 \pm 0.0120$ | $0.654 \pm 0.0259$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.19x** | 0.0558 | 0.0664 | 0.146 | 0.135 | $0.0554 \pm 0.0055$ | $0.0685 \pm 0.0072$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **1.02x** | 0.0757 | 0.0769 | 0.166 | 0.148 | $0.0765 \pm 0.0051$ | $0.0791 \pm 0.0090$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **1.61x** | 0.170 | 0.273 | 0.260 | 0.355 | $0.177 \pm 0.0335$ | $0.279 \pm 0.0320$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **1.78x** | 0.680 | 1.208 | 1.024 | 1.565 | $0.671 \pm 0.0991$ | $1.216 \pm 0.127$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **1.91x** | 2.647 | 5.065 | 3.588 | 6.730 | $2.420 \pm 0.613$ | $5.136 \pm 0.361$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.07x** | 1.378 | 1.481 | 2.308 | 2.800 | $1.374 \pm 0.247$ | $1.474 \pm 0.0683$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **0.89x** | 9.184 | 8.160 | 9.885 | 9.560 | $10.058 \pm 3.526$ | $8.896 \pm 2.588$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **1.27x** | 0.619 | 0.787 | 1.601 | 2.142 | $0.629 \pm 0.0345$ | $0.800 \pm 0.0535$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **1.00x** | 0.953 | 0.949 | 1.900 | 2.179 | $1.029 \pm 0.232$ | $0.948 \pm 0.0520$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **2.09x** | 12.997 | 27.110 | 16.486 | 33.193 | $13.073 \pm 2.413$ | $27.297 \pm 2.163$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **1.87x** | 70.197 | 131.378 | 91.393 | 164.230 | $71.853 \pm 5.861$ | $131.911 \pm 8.430$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 6.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSlice64`, `InitReducedRow64`, and `Min2ReducedRow64`)

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32`, no AVX2 row-reduction kernel runs): 3 of 4 have a 95% interval excluding 1.
- **Instances where AVX2 row kernels run** (`max(rows, cols) >= 32`): 9 faster, 5 slower, 4 with no detectable effect, out of 18.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 row kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source | 1.01x [0.90x-1.13x] | 0.0006 | 0.0006 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.82x [0.75x-0.90x] | 0.0013 | 0.0017 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.72x [0.67x-0.78x] | 0.0043 | 0.0058 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | SIMD slower | 0.69x [0.64x-0.74x] | 0.0031 | 0.0043 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | SIMD slower | 0.64x [0.58x-0.70x] | 0.0032 | 0.0048 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.78x [0.71x-0.86x] | 0.0082 | 0.0103 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD slower | 0.80x [0.77x-0.83x] | 0.0362 | 0.0462 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD slower | 0.89x [0.81x-0.97x] | 0.0747 | 0.0865 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD slower | 0.94x [0.90x-0.99x] | 0.0596 | 0.0652 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | no detectable effect | 0.92x [0.79x-1.07x] | 0.187 | 0.206 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.15x [1.11x-1.20x] | 0.0934 | 0.0858 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.37x [1.25x-1.50x] | 0.0558 | 0.0423 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.09x [1.05x-1.13x] | 0.0757 | 0.0708 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.19x [1.08x-1.32x] | 0.170 | 0.146 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.36x [1.28x-1.45x] | 0.680 | 0.488 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | no detectable effect | 1.16x [0.89x-1.50x] | 2.647 | 1.932 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.41x [1.15x-1.71x] | 1.378 | 0.945 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | no detectable effect | 1.05x [0.90x-1.23x] | 9.184 | 8.195 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 2.06x [1.81x-2.35x] | 0.619 | 0.295 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | no detectable effect | 1.27x [0.99x-1.63x] | 0.953 | 0.798 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.66x [1.14x-2.41x] | 12.997 | 8.010 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.22x [1.16x-1.29x] | 70.197 | 58.095 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->
