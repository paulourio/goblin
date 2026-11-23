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
- **Unified telemetry source**: fingerprint `42200995483aab66` (commit `54139b90d0894cb1295ac695699fa8a7fe881b20`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.50 ms median on a 1-variable MILP and 6.39 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

## 2. Aggregate Workload Tier Summary

Tier 1 micro-models are dominated by Python subprocess spawn overhead, and the planted-gap MAD family is constructed so that root heuristics close the gap without branching; the bold row isolates solver-dominated Tier 2 and Tier 3 non-planted MILPs across paired trial instances.

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `Py / Go` GeoMean [Clustered 95% CI] | Pooled Trial `Py / Go` | Equal-Family `Go / CBC` | `Go` Trial (Med) Sum (ms) | `CBC` Trial (Med) Sum (ms) | `Py` Trial (Med) Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **331.22x** [165.04x-664.72x] | 331.22x | 0.05x | 8.7 (0.8) | 195.0 (19.4) | 1073.2 (101.2) | 488.95x [43.00x-1768.59x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **17.31x** [9.36x-32.00x] | 17.31x | 0.17x | 724.1 (44.8) | 1991.3 (176.8) | 4329.3 (418.8) | 22.58x [0.14x-166.42x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **10.40x** [5.26x-20.58x] | 10.40x | 0.20x | 783.2 (140.2) | 2912.6 (560.7) | 6210.7 (1068.9) | 10.32x [1.13x-66.36x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **11.44x** [6.52x-20.08x] | 12.20x | 0.22x | 1198.2 (140.3) | 3148.6 (401.1) | 6798.3 (988.4) | 12.11x [0.14x-166.42x] |
| Tier 3 non-MAD MILPs | 4 (20) | **8.08x** [1.79x-36.45x] | 8.08x | 0.27x | 482.5 (96.3) | 1249.5 (232.3) | 2815.3 (600.5) | 6.03x [1.88x-66.36x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **26.81x** [12.30x-58.41x] | 34.79x | 0.11x | 310.2 (44.8) | 1780.1 (338.8) | 3848.5 (509.0) | 35.83x [1.13x-132.66x] |
| Random-weight MAD (negative controls) | 3 (33) | **16.82x** [0.82x-345.60x] | 16.82x | 0.14x | 296.2 (20.5) | 1218.3 (119.1) | 2257.8 (249.7) | 16.27x [1.31x-119.01x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **44.93x** [19.08x-105.80x] | 53.78x | 0.12x | 1205.8 (141.0) | 3318.8 (418.0) | 7764.7 (1079.8) | 43.00x [0.14x-1768.59x] |
| All 31 synthetic corpus families | 31 (293) | **39.33x** [20.55x-75.26x] | 48.90x | 0.12x | 1516.1 (185.8) | 5098.9 (756.9) | 11613.3 (1588.8) | 40.95x [0.14x-1768.59x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

## 3. End-to-End Pipeline Comparison (`JSON -> Solution`)

Pipeline stages measured per instance (median across randomized trials after warmup, plus paired per-trial speedup range):
1. **Prep (`t_parse + t_build` vs. `t_parse + t_compile`)**: JSON unmarshaling and PuLP model construction in Python (MPS writing `t_format` is recorded separately and timed inside `t_solve`) vs. streaming `FromPuLPJSONBytes` plus `Model.Compile()` matrix assembly in Go.
2. **Solve (`t_solve`)**: CBC subprocess execution and `.sol` parsing (Python) vs. in-memory `gb.SolveCompiled` (Go).
3. **Total (`t_total`)**: Complete end-to-end latency from raw JSON bytes to verified primal solution.

The Parity column is computed by `bench.CheckParity`: canonical and per-trial statuses and objectives across all 293 instances for `scalar`, `simd`, and `cbc` must match CBC within a relative tolerance of 1e-5, and `ValidateTelemetryAgainstCorpus` rejects the telemetry otherwise.

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | **601.7x** [259.8x-842.8x] | 0.01 | 6.40 | 0.00 | 6.35 | 0.005 | 0.055 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **660.7x** [361.4x-1019.8x] | 0.01 | 6.23 | 0.00 | 6.18 | 0.005 | 0.051 | `OK` |
| `t1_std_mip` | 3 | 3 | **661.4x** [514.5x-821.2x] | 0.01 | 7.41 | 0.01 | 7.36 | 0.005 | 0.057 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **739.9x** [612.5x-1073.0x] | 0.01 | 7.46 | 0.00 | 7.40 | 0.005 | 0.058 | `OK` |
| `t1_whiskas` | 6 | 5 | **457.2x** [327.6x-632.9x] | 0.01 | 6.34 | 0.01 | 6.27 | 0.007 | 0.074 | `OK` |
| `t1_furniture` | 2 | 3 | **253.7x** [76.5x-677.3x] | 0.02 | 7.97 | 0.02 | 7.92 | 0.005 | 0.059 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **103.6x** [49.0x-310.2x] | 0.11 | 9.38 | 0.09 | 9.29 | 0.010 | 0.093 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **975.7x** [125.2x-1768.6x] | 0.01 | 6.23 | 0.00 | 6.19 | 0.005 | 0.044 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **102.8x** [83.6x-132.7x] | 0.09 | 9.72 | 0.06 | 9.48 | 0.032 | 0.231 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **67.8x** [43.0x-119.0x] | 0.49 | 34.01 | 0.46 | 33.75 | 0.028 | 0.238 | `OK` |
| `t2_mkp_30` | 30 | 6 | **11.0x** [6.2x-16.9x] | 2.30 | 27.74 | 2.27 | 27.55 | 0.022 | 0.181 | `OK` |
| `t2_mkp_50` | 50 | 8 | **6.0x** [3.2x-31.9x] | 4.28 | 16.63 | 4.24 | 16.37 | 0.033 | 0.263 | `OK` |
| `t2_cflp_36` | 36 | 11 | **56.0x** [17.9x-166.4x] | 0.16 | 9.49 | 0.14 | 9.32 | 0.018 | 0.172 | `OK` |
| `t2_cflp_49` | 49 | 13 | **30.3x** [18.3x-82.0x] | 0.40 | 10.26 | 0.35 | 10.07 | 0.035 | 0.212 | `OK` |
| `t2_gap_45` | 45 | 13 | **22.8x** [4.7x-79.5x] | 0.81 | 14.01 | 0.78 | 13.80 | 0.022 | 0.206 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **11.9x** [5.3x-28.1x] | 4.12 | 69.04 | 4.10 | 68.80 | 0.030 | 0.247 | `OK` |
| `t2_binpack_44` | 44 | 14 | **1.9x** [0.1x-29.6x] | 11.65 | 13.84 | 11.62 | 13.63 | 0.028 | 0.209 | `OK` |
| `t2_setcover_50` | 50 | 20 | **37.5x** [6.5x-102.2x] | 0.27 | 11.11 | 0.23 | 10.81 | 0.039 | 0.304 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **58.7x** [45.3x-70.6x] | 0.16 | 9.21 | 0.13 | 8.94 | 0.033 | 0.274 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **42.7x** [31.6x-53.0x] | 0.23 | 10.14 | 0.15 | 9.74 | 0.071 | 0.414 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **32.2x** [26.7x-35.8x] | 0.36 | 11.56 | 0.29 | 10.94 | 0.072 | 0.616 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.8x** [3.4x-27.8x] | 3.51 | 48.33 | 3.47 | 48.01 | 0.040 | 0.313 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **7.2x** [1.3x-29.6x] | 16.50 | 167.40 | 16.44 | 166.97 | 0.064 | 0.437 | `OK` |
| `t3_mkp_200` | 200 | 8 | **4.9x** [4.2x-6.2x] | 18.69 | 115.02 | 18.61 | 114.42 | 0.083 | 0.608 | `OK` |
| `t3_cflp_300` | 300 | 34 | **26.4x** [8.0x-66.4x] | 8.85 | 248.26 | 8.74 | 247.35 | 0.112 | 0.909 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **3.0x** [1.9x-4.9x] | 62.13 | 157.06 | 61.90 | 155.19 | 0.212 | 1.871 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **10.8x** [5.3x-17.7x] | 6.63 | 80.16 | 6.33 | 77.23 | 0.301 | 2.786 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **30.6x** [23.5x-35.9x] | 0.58 | 17.70 | 0.44 | 16.58 | 0.135 | 1.079 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **17.1x** [10.9x-23.5x] | 1.01 | 18.71 | 0.81 | 16.97 | 0.204 | 1.736 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **6.0x** [1.1x-13.7x] | 8.36 | 60.94 | 7.36 | 52.95 | 1.095 | 8.243 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **10.2x** [4.4x-24.6x] | 34.01 | 371.06 | 30.06 | 337.05 | 4.367 | 34.505 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

## 4. Scalar vs. `GOEXPERIMENT=simd` Build

Ratios are geometric means of per-trial `scalar / SIMD` solve times over the same instances, with 95% t-intervals on the log ratios. "Reachable" is a necessary condition only: the AVX2 `Dot64`/`Axpy64`/`Scale64` paths need a slice of length at least 32, which requires `n + m >= 32`.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 0 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 4 faster, 1 slower, 18 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 1.12x [0.77x-1.63x] | 0.004 | 0.004 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 0.97x [0.93x-1.00x] | 0.004 | 0.004 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 1.00x [0.97x-1.04x] | 0.006 | 0.006 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source | 1.00x [0.99x-1.02x] | 0.004 | 0.004 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 1.01x [0.99x-1.03x] | 0.006 | 0.005 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 0.99x [0.92x-1.06x] | 0.017 | 0.017 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source | 0.99x [0.97x-1.00x] | 0.093 | 0.094 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 0.84x [0.66x-1.07x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 1.12x [0.96x-1.30x] | 0.060 | 0.055 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 1.00x [0.99x-1.01x] | 0.458 | 0.456 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 0.97x [0.84x-1.13x] | 2.274 | 2.108 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 1.00x [0.98x-1.02x] | 4.245 | 4.311 | 11 |
| `t2_cflp_36` | 47 | possible | no detectable effect | 1.01x [0.99x-1.03x] | 0.143 | 0.140 | 11 |
| `t2_cflp_49` | 62 | possible | no detectable effect | 1.01x [0.97x-1.05x] | 0.354 | 0.360 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 1.00x [0.96x-1.04x] | 0.782 | 0.766 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 0.99x [0.98x-1.00x] | 4.098 | 4.147 | 11 |
| `t2_binpack_44` | 58 | possible | no detectable effect | 0.98x [0.96x-1.01x] | 11.617 | 11.660 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 1.03x [0.92x-1.14x] | 0.234 | 0.225 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 1.00x [0.96x-1.04x] | 0.126 | 0.127 | 11 |
| `t2_mad_g3_i9` | 108 | possible | SIMD faster | 1.14x [1.04x-1.25x] | 0.148 | 0.130 | 11 |
| `t2_mad_g4_i12` | 156 | possible | no detectable effect | 0.97x [0.91x-1.02x] | 0.288 | 0.297 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.01x [0.96x-1.07x] | 3.471 | 2.964 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 1.01x [0.99x-1.02x] | 16.438 | 16.310 | 11 |
| `t3_mkp_200` | 208 | possible | SIMD faster | 1.06x [1.02x-1.11x] | 18.612 | 17.485 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.04x [0.96x-1.12x] | 8.737 | 8.584 | 5 |
| `t3_lotsizing_504` | 840 | possible | SIMD faster | 1.03x [1.02x-1.04x] | 61.904 | 60.738 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 1.03x [0.89x-1.19x] | 6.328 | 6.178 | 5 |
| `t3_mad_g5_i20` | 240 | possible | SIMD slower | 0.96x [0.94x-0.98x] | 0.441 | 0.465 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 1.05x [0.95x-1.15x] | 0.813 | 0.815 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 0.97x [0.92x-1.02x] | 7.363 | 7.873 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | SIMD faster | 1.08x [1.05x-1.11x] | 30.056 | 27.489 | 5 |
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
| `t1_std_mip` | **0.01** (1 / 0) | 0.00 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.002 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.002 | 64.95 | 58.90 |
| `t1_whiskas` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.003 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.02 (1) | 0.02 (1) | 0.002 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.09** (1 / 8) | 0.11 (2 / 8) | 0.14 (18) | 0.23 (35) | 0.007 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.002 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.06** (1 / 0) | 0.04 (1 / 0) | 0.06 (1) | 0.53 (4) | 0.083 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.46** (2 / 0) | 0.56 (12 / 0) | 0.55 (12) | 0.77 (109) | 0.101 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.27** (5 / 9) | 2.66 (40 / 9) | 1.04 (51) | 0.96 (61) | 0.025 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **4.24** (1 / 4) | 11.50 (2 / 4) | 4.72 (118) | 4.29 (111) | 0.039 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.14** (1 / 2) | 0.21 (3 / 2) | 0.16 (7) | 0.17 (7) | 0.027 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.35** (1 / 7) | 0.65 (3 / 7) | 0.32 (9) | 0.32 (9) | 0.037 | 909.02 | 827.30 |
| `t2_gap_45` | **0.78** (1 / 7) | 0.41 (1 / 7) | 1.89 (41) | 2.03 (60) | 0.025 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **4.10** (2 / 37) | 7.76 (25 / 37) | 3.01 (162) | 3.30 (199) | 0.059 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **11.62** (274 / 12) | 13.85 (298 / 12) | 8.61 (772) | 26.64 (1941) | 0.026 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.23** (1 / 6) | 0.35 (2 / 6) | 0.67 (6) | 0.80 (6) | 0.033 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.13** (1 / 0) | 0.76 (8 / 0) | 0.74 (8) | 0.73 (9) | 0.100 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.15** (1 / 0) | 0.13 (2 / 0) | 0.15 (2) | 2.73 (13) | 0.263 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.29** (1 / 0) | 3.22 (22 / 0) | 3.49 (22) | 5.20 (23) | 0.360 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.47** (43 / 0) | 2.91 (112 / 0) | 2.85 (112) | 3.64 (45) | 0.152 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **16.44** (201 / 0) | 29.97 (188 / 0) | 29.88 (188) | 91.10 (539) | 0.285 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **18.61** (250 / 14) | 81.32 (1135 / 16) | 165.00 (5000, `NodeLimit`) | 148.05 (5000, `NodeLimit`) | 0.119 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **8.74** (1 / 28) | 18.13 (54 / 34) | 8.93 (116) | 8.64 (118) | 0.169 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **61.90** (2 / 206) | 79.20 (376 / 206) | 250.03 (3682, `TimeLimit`) | 250.02 (3792, `TimeLimit`) | 0.712 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **6.33** (31 / 55) | 19.69 (87 / 53) | 250.11 (2561, `TimeLimit`) | 250.10 (2591, `TimeLimit`) | 0.182 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.44** (1 / 0) | 3.25 (17 / 0) | 2.61 (17) | 0.52 (1) | 0.653 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.81** (1 / 0) | 8.10 (58 / 0) | 7.62 (58) | 13.06 (74) | 1.557 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.36** (2 / 0) | 21.31 (25 / 0) | 19.15 (25) | 250.38 (152, `TimeLimit`) | 2.658 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **30.06** (1 / 0) | 32.37 (2 / 0) | 31.04 (2) | 250.29 (6, `TimeLimit`) | 56.329 | 208000.00 | 208000.00 |
<!-- END GENERATED:FULL_CHECKPOINT_TABLE -->

## 6. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus (`testdata/assign_corpus/manifest.json`; 11 randomized trials per family for Tiers 1-2 and 5 for Tier 3, 200 instances in total) compares pure-Go `assign.LinearSumFlat` against SciPy's C++ `rectangular_lsap` extension in warm state on bit-identical `float64` cost matrices, timing `linear_sum_assignment` without post-solve NumPy array gather overhead.

### 6.1 Aggregate LSAP Tier Summary

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `SciPy Solve / goblin` GeoMean [Clustered 95% CI] | Pooled Trial `SciPy Solve / goblin` | Equal-Family `SciPy Total / goblin` | `goblin` Solve Trial (Med) Sum (ms) | SciPy Solve Trial (Med) Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.35x** [1.14x-1.60x] | 1.35x | 4.28x | 0.222 (0.0199) | 0.313 (0.0287) | 1.44x [0.25x-2.44x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **3.03x** [1.83x-5.02x] | 3.03x | 3.06x | 9.060 (0.802) | 34.044 (3.089) | 2.62x [0.87x-18.10x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.84x** [1.85x-4.36x] | 2.84x | 3.55x | 222.382 (44.234) | 938.451 (188.031) | 2.44x [1.42x-6.16x] |
| Square families ($m = n$) | 16 (146) | **2.68x** [1.90x-3.77x] | 2.61x | 3.70x | 226.881 (44.186) | 962.001 (189.202) | 2.23x [0.25x-18.10x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.74x** [1.25x-2.44x] | 1.63x | 3.06x | 4.783 (0.870) | 10.807 (1.946) | 1.78x [0.87x-2.77x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.36x** [1.13x-4.93x] | 2.51x | 2.96x | 35.990 (6.351) | 66.145 (11.095) | 1.80x [1.04x-18.10x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.38x** [1.83x-3.10x] | 2.29x | 3.52x | 231.664 (45.056) | 972.808 (191.148) | 2.01x [0.25x-18.10x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 6.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.25x** | 0.0005 | 0.0009 | 0.0006 | 0.0120 | $0.0010 \pm 0.0010$ | $0.0009 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.26x** | 0.0017 | 0.0023 | 0.0020 | 0.0102 | $0.0019 \pm 0.0008$ | $0.0023 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.67x** | 0.0036 | 0.0063 | 0.0047 | 0.0155 | $0.0036 \pm 0.0008$ | $0.0061 \pm 0.0017$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.14x** | 0.0037 | 0.0039 | 0.0046 | 0.0142 | $0.0035 \pm 0.0007$ | $0.0040 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.23x** | 0.0039 | 0.0044 | 0.0049 | 0.0152 | $0.0037 \pm 0.0008$ | $0.0045 \pm 0.0009$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.65x** | 0.0063 | 0.0108 | 0.0102 | 0.0329 | $0.0065 \pm 0.0016$ | $0.0107 \pm 0.0018$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.39x** | 0.0286 | 0.0711 | 0.0359 | 0.0963 | $0.0301 \pm 0.0107$ | $0.0693 \pm 0.0108$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.51x** | 0.0526 | 0.181 | 0.0677 | 0.231 | $0.0519 \pm 0.0046$ | $0.183 \pm 0.0262$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.23x** | 0.0476 | 0.101 | 0.0709 | 0.159 | $0.0463 \pm 0.0042$ | $0.104 \pm 0.0135$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.68x** | 0.137 | 0.228 | 0.151 | 0.266 | $0.148 \pm 0.0497$ | $0.241 \pm 0.0449$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **14.11x** | 0.0597 | 0.786 | 0.0774 | 0.862 | $0.0561 \pm 0.0088$ | $0.783 \pm 0.0343$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.89x** | 0.0365 | 0.0691 | 0.0630 | 0.142 | $0.0387 \pm 0.0125$ | $0.0710 \pm 0.0081$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **1.85x** | 0.0422 | 0.0793 | 0.0660 | 0.152 | $0.0436 \pm 0.0074$ | $0.0799 \pm 0.0070$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.56x** | 0.0825 | 0.281 | 0.106 | 0.356 | $0.0800 \pm 0.0080$ | $0.285 \pm 0.0290$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.91x** | 0.315 | 1.292 | 0.438 | 1.684 | $0.329 \pm 0.0476$ | $1.279 \pm 0.126$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.88x** | 1.221 | 5.190 | 1.638 | 7.729 | $1.408 \pm 0.436$ | $5.314 \pm 0.379$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.71x** | 0.864 | 1.478 | 1.468 | 4.119 | $0.867 \pm 0.0347$ | $1.479 \pm 0.0524$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.64x** | 5.232 | 8.486 | 5.816 | 9.371 | $5.758 \pm 2.231$ | $9.231 \pm 2.638$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.50x** | 0.345 | 0.823 | 0.738 | 3.628 | $0.337 \pm 0.0284$ | $0.844 \pm 0.0594$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.29x** | 0.439 | 0.966 | 0.812 | 3.821 | $0.423 \pm 0.0293$ | $0.967 \pm 0.0263$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.98x** | 5.792 | 27.988 | 7.496 | 36.841 | $5.731 \pm 0.352$ | $28.559 \pm 2.240$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.74x** | 30.341 | 143.101 | 39.483 | 185.731 | $29.953 \pm 3.650$ | $141.297 \pm 8.937$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 6.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSkip64`, `InitReducedRow64`, and `Min2ReducedRow64`)

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` with no `>= 8x8` tall transpose and either fused square `Minimize` non-sparse path or `rows * cols < 32`, so no AVX2 kernel runs): 1 of 3 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32`, tall `rows > cols` with `>= 8x8`, or fallback `AllAbsLE64`/`ValidateAndNegate64` at `rows * cols >= 32`): 18 faster, 0 slower, 1 with no detectable effect, out of 19.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source | 1.00x [0.97x-1.03x] | 0.0005 | 0.0005 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.03x [1.00x-1.06x] | 0.0017 | 0.0017 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source | 1.01x [0.99x-1.04x] | 0.0036 | 0.0035 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | SIMD faster | 1.12x [1.10x-1.14x] | 0.0037 | 0.0033 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | SIMD faster | 1.17x [1.14x-1.20x] | 0.0039 | 0.0033 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | no detectable effect | 0.89x [0.74x-1.08x] | 0.0063 | 0.0069 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.22x [1.19x-1.26x] | 0.0286 | 0.0227 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.32x [1.25x-1.39x] | 0.0526 | 0.0400 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.70x [1.58x-1.82x] | 0.0476 | 0.0295 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.68x [1.59x-1.77x] | 0.137 | 0.0803 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.30x [1.25x-1.36x] | 0.0597 | 0.0452 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.30x [1.20x-1.39x] | 0.0365 | 0.0276 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.44x [1.35x-1.54x] | 0.0422 | 0.0308 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.38x [1.33x-1.44x] | 0.0825 | 0.0569 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.59x [1.49x-1.70x] | 0.315 | 0.196 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.86x [1.54x-2.25x] | 1.221 | 0.743 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.51x [2.36x-2.67x] | 0.864 | 0.347 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.64x [2.50x-2.79x] | 5.232 | 1.879 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.85x [1.55x-2.21x] | 0.345 | 0.191 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.56x [1.31x-1.86x] | 0.439 | 0.280 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.61x [1.52x-1.69x] | 5.792 | 3.549 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.52x [1.42x-1.62x] | 30.341 | 19.693 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

## 7. Production Binary Footprint and Linker Dead-Code Elimination

Standalone `CGO_ENABLED=0` binaries compiled across the four canonical `goblin/assign` and `goblin` build profiles (`bench/results/goblin_binary_size.json`), verifying that unreferenced `WithCertified`, `WithVIPRTrace`, `encoding/json`, and (in `goblin/assign`) `math/big` symbols are eliminated by the Go linker:

<!-- BEGIN GENERATED:BINARY_SIZE_TABLE -->
| Profile | Default Raw / Stripped (KiB) | `GOEXPERIMENT=simd` Raw / Stripped (KiB) | Stripped Delta vs. Default | `goblin` Text (KiB) | `math/big` Syms | `encoding/json` Syms | `VIPR` Syms | `Certif` Syms |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `assign` default (`LinearSum` + `SolveInto`) | 2784.8 / **1740.2** | 2813.7 / **1804.2** | baseline | 114.4 | `0` | `0` | `0` | `0` |
| `assign` certified (`+ assign.WithCertified(true)`) | 2871.2 / **1796.2** | 2898.1 / **1860.2** | +56.0 KiB (+3.2%) | 154.8 | `0` | `0` | `0` | `16` |
| `goblin` default (`Model.Solve`, `DefaultOptions`) | 5324.5 / **3392.2** | 5221.6 / **3412.2** | baseline | 919.2 | `130` | `0` | `0` | `0` |
| `goblin` certified + VIPR (`+ WithCertified + WithVIPRTrace`) | 6472.9 / **4164.2** | 6338.6 / **4184.2** | +772.0 KiB (+22.8%) | 1360.0 | `153` | `0` | `197` | `10` |
<!-- END GENERATED:BINARY_SIZE_TABLE -->
