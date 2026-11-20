# `goblin` vs. Python (`PuLP + COIN-OR CBC`) Benchmark Report

## 1. Experimental Setup and Provenance

```bash
go run ./cmd/goblin-bench -check-docs
```

- **CPU**: `13th Gen Intel(R) Core(TM) i9-13900K` (x86_64, 30 logical CPUs exposed by `lscpu`, AVX2 + FMA).
- **Default pure-Go backend**: `goblin` default (`BackendGo`, scalar kernels or `WithoutSIMD()`). All headline `goblin (Go)` timings use that build.
- **Experimental SIMD build**: `GOEXPERIMENT=simd`, which dispatches AVX2 kernels for slices of length 32 or more; see section 4.
- **Optional C++ backend**: `gb.WithCBC()` (`BackendCBC`, built with `-tags cbc`), driving in-process COIN-OR CBC/CLP (`c1c171ec`) through `cgo`.
- **Python baseline**: Python + `PuLP` + `COIN-OR CBC` (`c1c171ec`, `threads=1`, `ratioGap=1e-6`, `allowableGap=1e-6`) subprocess. Its solve time includes MPS writing, process spawn, and `.sol` parsing.
- **Synthetic corpus and public validation**: 31 synthetic LP and MILP families across 3 tiers (`testdata/corpus/*.json` and `testdata/corpus/trials/*.jsonl`; 11 distinct randomized trial instances per family for Tiers 1-2 and 5 for Tier 3, 293 instances in total), measured in warm state, complemented by 20 held-out public Netlib LP and MIPLIB 3.0 MILP instances solved to optimality plus a 25-node dual-bound check on `coinor_capplan1` (`public_benchmarks_test.go`).

<!-- BEGIN GENERATED:PROVENANCE -->
- **Unified telemetry source**: fingerprint `e204cda60eec4c2c` (commit `99a49c0d04e2f9edf0a4c6466a80b49b8a54b088`), `go1.27.3-X:simd`, Linux kernel `6.18.63-gentoo-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, or kernel differs.
- **CPU affinity**: goblin `6`; Python + CBC `6`; Python + SciPy `6`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.56 ms median on a 1-variable MILP and 6.27 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

## 2. Aggregate Workload Tier Summary

Tier 1 micro-models are dominated by Python subprocess spawn overhead, and the planted-gap MAD family is constructed so that root heuristics close the gap without branching; the bold row isolates solver-dominated Tier 2 and Tier 3 non-planted MILPs across paired trial instances.

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `Py / Go` GeoMean [Clustered 95% CI] | Pooled Trial `Py / Go` | Equal-Family `Go / CBC` | `Go` Trial (Med) Sum (ms) | `CBC` Trial (Med) Sum (ms) | `Py` Trial (Med) Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **272.73x** [123.43x-602.64x] | 272.73x | 0.06x | 16.5 (1.5) | 193.2 (19.3) | 1065.3 (100.2) | 365.74x [10.99x-2701.65x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **14.56x** [8.16x-26.00x] | 14.56x | 0.20x | 1097.0 (53.3) | 1959.1 (170.9) | 4307.9 (416.9) | 18.67x [0.10x-145.71x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **6.28x** [2.57x-15.35x] | 6.28x | 0.33x | 4368.5 (210.4) | 2882.3 (557.8) | 6174.8 (1064.9) | 7.42x [0.07x-32.39x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **7.96x** [4.39x-14.44x] | 9.09x | 0.32x | 5080.1 (207.2) | 3103.3 (392.6) | 6761.3 (985.0) | 9.19x [0.07x-145.71x] |
| Tier 3 non-MAD MILPs | 4 (20) | **3.88x** [0.56x-27.01x] | 3.88x | 0.56x | 3991.9 (154.7) | 1238.2 (229.7) | 2800.5 (599.2) | 3.62x [0.07x-23.01x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **24.09x** [9.33x-62.19x] | 33.29x | 0.12x | 386.2 (56.5) | 1762.5 (338.6) | 3826.0 (506.4) | 34.64x [3.77x-156.02x] |
| Random-weight MAD (negative controls) | 3 (33) | **9.64x** [0.71x-130.32x] | 9.64x | 0.25x | 539.4 (34.2) | 1210.6 (115.5) | 2242.0 (247.7) | 8.72x [0.97x-69.31x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **32.62x** [13.25x-80.35x] | 40.78x | 0.17x | 5095.9 (208.7) | 3272.1 (409.4) | 7722.0 (1075.6) | 31.40x [0.07x-2701.65x] |
| All 31 synthetic corpus families | 31 (293) | **30.17x** [15.18x-59.95x] | 39.01x | 0.15x | 5482.1 (265.2) | 5034.6 (748.0) | 11548.0 (1582.0) | 32.97x [0.07x-2701.65x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

## 3. End-to-End Pipeline Comparison (`JSON -> Solution`)

Pipeline stages measured per instance (median across randomized trials after warmup, plus paired per-trial speedup range):
1. **Prep (`t_parse + t_build + t_format/t_compile`)**: JSON unmarshaling, model construction, and MPS formatting (Python) or `Model.Compile()` matrix assembly (Go).
2. **Solve (`t_solve`)**: CBC subprocess execution and `.sol` parsing (Python) vs. in-memory `gb.SolveCompiled` (Go).
3. **Total (`t_total`)**: Complete end-to-end latency from raw JSON bytes to verified primal solution.

The Parity column is computed by `bench.CheckParity`: canonical and per-trial statuses and objectives across all 293 instances for `scalar`, `simd`, and `cbc` must match CBC within a relative tolerance of 1e-5, and `ValidateTelemetryAgainstCorpus` rejects the telemetry otherwise.

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | **395.9x** [119.7x-602.2x] | 0.01 | 6.21 | 0.00 | 6.17 | 0.008 | 0.064 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **456.4x** [209.0x-622.5x] | 0.01 | 6.26 | 0.00 | 6.21 | 0.009 | 0.066 | `OK` |
| `t1_std_mip` | 3 | 3 | **453.6x** [177.6x-713.4x] | 0.02 | 7.37 | 0.01 | 7.32 | 0.008 | 0.069 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **506.2x** [401.8x-672.4x] | 0.01 | 7.68 | 0.01 | 7.63 | 0.007 | 0.076 | `OK` |
| `t1_whiskas` | 6 | 5 | **330.7x** [231.9x-425.8x] | 0.02 | 6.50 | 0.01 | 6.41 | 0.013 | 0.100 | `OK` |
| `t1_furniture` | 2 | 3 | **197.9x** [52.2x-580.6x] | 0.03 | 7.71 | 0.02 | 7.66 | 0.007 | 0.064 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **97.4x** [27.4x-319.3x] | 0.09 | 9.21 | 0.08 | 9.12 | 0.009 | 0.112 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **1889.7x** [1042.9x-2701.7x] | 0.00 | 6.22 | 0.00 | 6.18 | 0.002 | 0.053 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **144.8x** [134.1x-156.0x] | 0.07 | 9.65 | 0.04 | 9.44 | 0.023 | 0.265 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **31.5x** [11.0x-69.3x] | 1.25 | 33.39 | 1.22 | 33.14 | 0.031 | 0.309 | `OK` |
| `t2_mkp_30` | 30 | 6 | **6.0x** [3.4x-10.2x] | 3.07 | 27.83 | 3.05 | 27.66 | 0.023 | 0.224 | `OK` |
| `t2_mkp_50` | 50 | 8 | **3.1x** [1.2x-25.2x] | 8.66 | 16.66 | 8.62 | 16.42 | 0.040 | 0.326 | `OK` |
| `t2_cflp_36` | 36 | 11 | **28.2x** [3.7x-145.7x] | 0.27 | 9.24 | 0.25 | 9.07 | 0.021 | 0.206 | `OK` |
| `t2_cflp_49` | 49 | 13 | **17.2x** [9.2x-31.3x] | 0.58 | 10.30 | 0.55 | 10.08 | 0.025 | 0.254 | `OK` |
| `t2_gap_45` | 45 | 13 | **15.8x** [2.0x-67.9x] | 1.02 | 14.27 | 1.00 | 14.08 | 0.024 | 0.245 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **9.9x** [4.4x-22.8x] | 5.19 | 68.18 | 5.15 | 67.94 | 0.032 | 0.314 | `OK` |
| `t2_binpack_44` | 44 | 14 | **9.5x** [0.1x-42.1x] | 0.58 | 13.62 | 0.56 | 13.43 | 0.030 | 0.238 | `OK` |
| `t2_setcover_50` | 50 | 20 | **47.2x** [3.6x-142.7x] | 0.20 | 11.44 | 0.16 | 11.15 | 0.037 | 0.399 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **46.2x** [34.9x-64.6x] | 0.23 | 9.53 | 0.16 | 9.26 | 0.042 | 0.362 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **55.0x** [33.0x-65.0x] | 0.18 | 10.05 | 0.12 | 9.65 | 0.054 | 0.515 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **28.8x** [23.2x-34.4x] | 0.39 | 11.50 | 0.31 | 10.89 | 0.082 | 0.785 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **6.7x** [2.8x-17.9x] | 5.04 | 47.32 | 4.98 | 47.02 | 0.045 | 0.388 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **4.3x** [1.0x-16.4x] | 27.90 | 167.03 | 27.84 | 166.57 | 0.070 | 0.577 | `OK` |
| `t3_mkp_200` | 200 | 8 | **1.8x** [1.2x-3.2x] | 52.40 | 115.68 | 52.28 | 115.07 | 0.107 | 0.799 | `OK` |
| `t3_cflp_300` | 300 | 34 | **16.7x** [10.1x-23.0x] | 11.44 | 246.63 | 11.32 | 245.71 | 0.119 | 1.179 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **1.1x** [0.1x-3.4x] | 78.84 | 156.63 | 78.59 | 154.79 | 0.250 | 2.291 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **6.5x** [3.9x-9.5x] | 12.03 | 80.23 | 11.69 | 77.33 | 0.331 | 3.481 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **27.9x** [22.7x-32.4x] | 0.64 | 17.79 | 0.48 | 16.67 | 0.156 | 1.361 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **10.5x** [9.8x-11.0x] | 1.78 | 18.71 | 1.50 | 16.96 | 0.263 | 2.286 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **4.8x** [3.8x-6.0x] | 12.68 | 60.66 | 11.36 | 52.91 | 1.607 | 10.821 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **7.7x** [3.8x-13.5x] | 40.56 | 368.54 | 34.94 | 334.66 | 4.737 | 43.643 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

## 4. Scalar vs. `GOEXPERIMENT=simd` Build

Ratios are geometric means of per-trial `scalar / SIMD` solve times over the same instances, with 95% t-intervals on the log ratios. "Reachable" is a necessary condition only: the AVX2 `Dot64`/`Axpy64`/`Scale64` paths need a slice of length at least 32, which requires `n + m >= 32`.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 1 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 0 faster, 1 slower, 22 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 0.77x [0.59x-1.01x] | 0.003 | 0.004 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 1.13x [0.84x-1.52x] | 0.004 | 0.003 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 0.86x [0.72x-1.04x] | 0.005 | 0.006 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.33x [1.11x-1.60x] | 0.007 | 0.005 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 0.93x [0.67x-1.29x] | 0.006 | 0.007 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 0.94x [0.75x-1.16x] | 0.018 | 0.018 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source | 1.11x [0.92x-1.33x] | 0.084 | 0.087 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 1.07x [0.87x-1.31x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 1.01x [0.98x-1.04x] | 0.042 | 0.042 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 0.99x [0.92x-1.06x] | 1.220 | 1.302 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 1.00x [0.98x-1.01x] | 3.048 | 3.068 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 1.00x [0.99x-1.01x] | 8.623 | 8.626 | 11 |
| `t2_cflp_36` | 47 | possible | no detectable effect | 1.03x [0.95x-1.12x] | 0.253 | 0.227 | 11 |
| `t2_cflp_49` | 62 | possible | no detectable effect | 1.02x [0.94x-1.11x] | 0.554 | 0.559 | 11 |
| `t2_gap_45` | 58 | possible | SIMD slower | 0.97x [0.94x-0.99x] | 0.998 | 0.990 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 1.01x [0.95x-1.07x] | 5.154 | 5.267 | 11 |
| `t2_binpack_44` | 58 | possible | no detectable effect | 1.02x [0.98x-1.07x] | 0.559 | 0.528 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 1.02x [0.94x-1.12x] | 0.164 | 0.150 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 1.02x [0.92x-1.13x] | 0.161 | 0.157 | 11 |
| `t2_mad_g3_i9` | 108 | possible | no detectable effect | 1.04x [0.94x-1.14x] | 0.118 | 0.121 | 11 |
| `t2_mad_g4_i12` | 156 | possible | no detectable effect | 1.00x [0.95x-1.05x] | 0.312 | 0.313 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.00x [0.98x-1.02x] | 4.979 | 4.936 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 1.00x [0.97x-1.03x] | 27.840 | 27.856 | 11 |
| `t3_mkp_200` | 208 | possible | no detectable effect | 1.01x [0.99x-1.03x] | 52.282 | 52.605 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 0.97x [0.94x-1.00x] | 11.321 | 12.132 | 5 |
| `t3_lotsizing_504` | 840 | possible | no detectable effect | 0.99x [0.97x-1.01x] | 78.592 | 79.985 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 1.00x [0.92x-1.08x] | 11.690 | 10.839 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 1.00x [0.98x-1.01x] | 0.478 | 0.480 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 0.95x [0.85x-1.05x] | 1.497 | 1.541 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 1.03x [0.96x-1.10x] | 11.361 | 11.322 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | no detectable effect | 1.02x [0.98x-1.05x] | 34.941 | 34.866 | 5 |
<!-- END GENERATED:SIMD_EFFECT_TABLE -->

## 5. Solver-Core Checkpoint Progression (default pure-Go backend)

Progression across checkpoints on pre-compiled `CompiledProblem` instances:
- **CP2/3 (Root LP)**: Revised Dual and Two-Phase Primal Simplex.
- **CP4 (Base B&B)**: Branch-and-Bound with pseudocost/strong branching (`Presolve=off, Cuts=off, Heuristics=off`).
- **CP5 (+Presolve)**: Savelsbergh bound tightening, GCD reduction, and node propagation (`Presolve=on, Cuts=off, Heuristics=off`).
- **CP6 (+Cuts)**: GMI, lifted 0-1 knapsack covers, flow/VUB and $(l, S)$ cuts, and single-row MIR cuts (`Presolve=on, Cuts=on, Heuristics=off`).
- **CP7 (Full `goblin`)**: Full Branch-and-Cut with Propagated Rounding, Root Sub-MIP, and Fractional Diving (`Presolve=on, Cuts=on, Heuristics=on`).

<!-- BEGIN GENERATED:FULL_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_std_lp` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 54.00 | 54.00 |
| `t1_std_lp_max` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 80.00 | 80.00 |
| `t1_std_mip` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.00 (1) | 0.01 (3) | 0.002 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.002 | 64.95 | 58.90 |
| `t1_whiskas` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.00 (1) | 0.005 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.02 (1) | 0.02 (1) | 0.003 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.08** (1 / 8) | 0.10 (2 / 8) | 0.16 (24) | 0.24 (35) | 0.007 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.04** (1 / 0) | 0.04 (1 / 0) | 0.05 (1) | 0.57 (4) | 0.068 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **1.22** (10 / 0) | 0.68 (12 / 0) | 0.64 (12) | 1.25 (34) | 0.084 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **3.05** (27 / 9) | 3.46 (48 / 9) | 1.54 (77) | 1.34 (77) | 0.022 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **8.62** (1 / 4) | 22.62 (2 / 4) | 9.16 (132) | 8.45 (119) | 0.041 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.25** (1 / 2) | 1.04 (3 / 2) | 0.22 (7) | 0.22 (7) | 0.020 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.55** (1 / 20) | 1.62 (5 / 20) | 0.38 (9) | 0.36 (9) | 0.026 | 909.02 | 827.30 |
| `t2_gap_45` | **1.00** (1 / 9) | 0.41 (1 / 9) | 1.83 (61) | 1.87 (61) | 0.026 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **5.15** (4 / 37) | 5.83 (28 / 37) | 2.78 (187) | 3.50 (199) | 0.048 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **0.56** (2 / 7) | 1.05 (7 / 7) | 25.19 (1372) | 38.80 (2327) | 0.030 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.16** (1 / 6) | 0.36 (2 / 6) | 0.74 (9) | 0.71 (9) | 0.029 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.16** (1 / 0) | 0.93 (9 / 0) | 0.79 (9) | 0.69 (10) | 0.108 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.12** (1 / 0) | 0.11 (1 / 0) | 0.12 (1) | 4.09 (23) | 0.230 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.31** (1 / 0) | 2.30 (26 / 0) | 2.28 (26) | 5.01 (14) | 0.324 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **4.98** (45 / 0) | 3.31 (325 / 0) | 3.41 (325) | 9.89 (111) | 0.142 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **27.84** (220 / 0) | 40.35 (191 / 0) | 39.98 (191) | 250.01 (1798, `TimeLimit`) | 0.247 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **52.28** (598 / 9) | 143.49 (1552 / 9) | 250.02 (4957, `TimeLimit`) | 233.91 (5000, `NodeLimit`) | 0.123 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **11.32** (53 / 20) | 12.72 (59 / 20) | 10.11 (134) | 10.39 (134) | 0.158 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **78.59** (30 / 205) | 133.60 (488 / 205) | 250.05 (2653, `TimeLimit`) | 250.01 (2874, `TimeLimit`) | 0.694 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **11.69** (64 / 46) | 46.61 (206 / 46) | 250.12 (1786, `TimeLimit`) | 250.03 (1797, `TimeLimit`) | 0.227 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.48** (1 / 0) | 2.63 (10 / 0) | 2.62 (10) | 3.99 (5) | 0.906 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **1.50** (1 / 0) | 6.26 (12 / 0) | 6.77 (12) | 70.94 (116) | 1.482 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **11.36** (2 / 0) | 106.27 (145 / 0) | 103.97 (145) | 250.28 (212, `TimeLimit`) | 12.048 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **34.94** (1 / 0) | 39.88 (2 / 0) | 38.68 (2) | 251.70 (18, `TimeLimit`) | 111.759 | 208000.00 | 208000.00 |
<!-- END GENERATED:FULL_CHECKPOINT_TABLE -->

## 6. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus (`testdata/assign_corpus/manifest.json`; 11 randomized trials per family for Tiers 1-2 and 5 for Tier 3, 200 instances in total) compares pure-Go `assign.LinearSumFlat` against SciPy's C++ `rectangular_lsap` extension in warm state on bit-identical `float64` cost matrices, timing `linear_sum_assignment` without post-solve NumPy array gather overhead.

### 6.1 Aggregate LSAP Tier Summary

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `SciPy Solve / goblin` GeoMean [Clustered 95% CI] | Pooled Trial `SciPy Solve / goblin` | Equal-Family `SciPy Total / goblin` | `goblin` Solve Trial (Med) Sum (ms) | SciPy Solve Trial (Med) Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.76x** [1.55x-1.99x] | 1.76x | 4.85x | 0.189 (0.0151) | 0.316 (0.0290) | 1.77x [0.73x-3.95x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **3.38x** [2.05x-5.58x] | 3.38x | 3.23x | 9.261 (0.727) | 32.592 (2.952) | 3.07x [0.17x-20.23x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **3.23x** [2.15x-4.84x] | 3.23x | 3.93x | 206.122 (40.593) | 922.564 (184.208) | 3.27x [1.48x-6.39x] |
| Square families ($m = n$) | 16 (146) | **2.98x** [2.15x-4.13x] | 2.92x | 3.96x | 211.837 (40.650) | 944.623 (185.258) | 2.73x [0.17x-20.23x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **2.33x** [1.65x-3.29x] | 2.20x | 3.54x | 3.736 (0.685) | 10.850 (1.932) | 2.56x [0.85x-3.54x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.50x** [1.18x-5.26x] | 2.63x | 3.01x | 35.183 (6.056) | 64.709 (10.919) | 2.07x [0.17x-20.23x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.79x** [2.18x-3.56x] | 2.70x | 3.84x | 215.572 (41.335) | 955.473 (187.189) | 2.61x [0.17x-20.23x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 6.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.85x** | 0.0005 | 0.0009 | 0.0006 | 0.0115 | $0.0005 \pm 0.0001$ | $0.0009 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.75x** | 0.0013 | 0.0024 | 0.0017 | 0.0103 | $0.0016 \pm 0.0010$ | $0.0025 \pm 0.0006$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **2.04x** | 0.0026 | 0.0064 | 0.0044 | 0.0153 | $0.0032 \pm 0.0016$ | $0.0062 \pm 0.0017$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.52x** | 0.0025 | 0.0040 | 0.0042 | 0.0150 | $0.0027 \pm 0.0008$ | $0.0040 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.55x** | 0.0028 | 0.0043 | 0.0044 | 0.0157 | $0.0030 \pm 0.0014$ | $0.0044 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.89x** | 0.0054 | 0.0109 | 0.0089 | 0.0325 | $0.0061 \pm 0.0028$ | $0.0108 \pm 0.0017$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.96x** | 0.0239 | 0.0711 | 0.0337 | 0.0964 | $0.0238 \pm 0.0051$ | $0.0695 \pm 0.0098$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.72x** | 0.0482 | 0.177 | 0.0651 | 0.227 | $0.0472 \pm 0.0042$ | $0.176 \pm 0.0251$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.04x** | 0.0428 | 0.0930 | 0.0675 | 0.141 | $0.0491 \pm 0.0169$ | $0.0960 \pm 0.0141$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.43x** | 0.132 | 0.220 | 0.139 | 0.263 | $0.236 \pm 0.323$ | $0.236 \pm 0.0435$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **15.13x** | 0.0493 | 0.687 | 0.0651 | 0.761 | $0.0457 \pm 0.0074$ | $0.682 \pm 0.0149$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **2.81x** | 0.0277 | 0.0732 | 0.0507 | 0.147 | $0.0268 \pm 0.0035$ | $0.0752 \pm 0.0078$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.67x** | 0.0309 | 0.0837 | 0.0536 | 0.156 | $0.0315 \pm 0.0032$ | $0.0841 \pm 0.0071$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.83x** | 0.0747 | 0.279 | 0.0976 | 0.355 | $0.0742 \pm 0.0084$ | $0.284 \pm 0.0321$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **4.12x** | 0.297 | 1.268 | 0.410 | 1.683 | $0.307 \pm 0.0500$ | $1.259 \pm 0.121$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.41x** | 1.138 | 5.217 | 1.516 | 7.699 | $1.210 \pm 0.158$ | $5.318 \pm 0.392$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **2.11x** | 0.717 | 1.504 | 1.319 | 4.088 | $0.709 \pm 0.0293$ | $1.495 \pm 0.0535$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.68x** | 5.107 | 8.397 | 5.312 | 9.232 | $5.579 \pm 2.066$ | $9.177 \pm 2.644$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **3.23x** | 0.264 | 0.810 | 0.636 | 3.699 | $0.259 \pm 0.0131$ | $0.835 \pm 0.0581$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.78x** | 0.357 | 0.956 | 0.725 | 3.655 | $0.348 \pm 0.0228$ | $0.966 \pm 0.0348$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **5.16x** | 5.513 | 27.708 | 7.369 | 35.599 | $5.467 \pm 0.335$ | $28.233 \pm 2.217$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **5.04x** | 27.496 | 139.615 | 38.809 | 181.954 | $27.653 \pm 4.068$ | $138.488 \pm 9.926$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 6.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSlice64`, `InitReducedRow64`, and `Min2ReducedRow64`)

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` and `rows * cols < 256`, no AVX2 kernel runs): 0 of 2 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32` or `AllAbsLE64` at `rows * cols >= 256`): 14 faster, 0 slower, 6 with no detectable effect, out of 20.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source | 0.72x [0.44x-1.17x] | 0.0005 | 0.0005 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source | 1.24x [0.95x-1.62x] | 0.0013 | 0.0012 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | possible | no detectable effect | 0.89x [0.66x-1.21x] | 0.0026 | 0.0036 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | no detectable effect | 1.03x [0.79x-1.33x] | 0.0025 | 0.0027 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | no detectable effect | 1.01x [0.72x-1.43x] | 0.0028 | 0.0027 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | no detectable effect | 1.03x [0.72x-1.47x] | 0.0054 | 0.0059 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.33x [1.24x-1.42x] | 0.0239 | 0.0189 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.25x [1.07x-1.46x] | 0.0482 | 0.0358 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.51x [1.29x-1.77x] | 0.0428 | 0.0286 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 2.07x [1.31x-3.26x] | 0.132 | 0.0758 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.28x [1.19x-1.38x] | 0.0493 | 0.0352 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | no detectable effect | 1.17x [0.91x-1.51x] | 0.0277 | 0.0210 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | no detectable effect | 1.19x [0.98x-1.46x] | 0.0309 | 0.0255 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.40x [1.24x-1.59x] | 0.0747 | 0.0478 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.43x [1.35x-1.52x] | 0.297 | 0.219 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.58x [1.53x-1.63x] | 1.138 | 0.731 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.89x [1.72x-2.07x] | 0.717 | 0.375 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.37x [2.24x-2.52x] | 5.107 | 2.014 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.51x [1.28x-1.79x] | 0.264 | 0.173 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.52x [1.26x-1.84x] | 0.357 | 0.223 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.55x [1.49x-1.61x] | 5.513 | 3.526 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.47x [1.39x-1.56x] | 27.496 | 18.441 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->
