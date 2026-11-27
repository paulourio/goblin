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
- **Unified telemetry source**: fingerprint `dbe2691169aaaa14` (working tree on top of commit `e6622a2893bc5c96904a03afa0de07570accacac`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.32 ms median on a 1-variable MILP and 6.26 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

## 2. Aggregate Workload Tier Summary

Tier 1 micro-models are dominated by Python subprocess spawn overhead, and the planted-gap MAD family is constructed so that root heuristics close the gap without branching; the bold row isolates solver-dominated Tier 2 and Tier 3 non-planted MILPs across paired trial instances.

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `Py / Go` GeoMean [Clustered 95% CI] | Pooled Trial `Py / Go` | Equal-Family `Go / CBC` | `Go` Trial (Med) Sum (ms) | `CBC` Trial (Med) Sum (ms) | `Py` Trial (Med) Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **408.54x** [174.31x-957.53x] | 408.54x | 0.04x | 10.0 (0.9) | 154.6 (14.3) | 1123.1 (104.7) | 582.44x [33.22x-3072.19x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **18.66x** [11.22x-31.04x] | 18.66x | 0.17x | 569.4 (37.2) | 1757.2 (155.9) | 4404.3 (420.3) | 23.17x [1.30x-133.08x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **9.94x** [4.59x-21.50x] | 9.94x | 0.24x | 904.1 (166.6) | 2839.4 (580.0) | 6471.4 (1101.6) | 10.13x [1.04x-69.57x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **11.98x** [7.16x-20.03x] | 13.08x | 0.21x | 1149.6 (157.5) | 3058.9 (403.2) | 6960.5 (1007.5) | 13.05x [1.30x-133.08x] |
| Tier 3 non-MAD MILPs | 4 (20) | **7.43x** [1.32x-42.02x] | 7.43x | 0.28x | 588.9 (121.0) | 1365.1 (252.7) | 2905.2 (618.4) | 5.47x [1.44x-69.57x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **27.70x** [11.78x-65.11x] | 36.49x | 0.15x | 324.8 (46.4) | 1550.6 (333.9) | 4025.0 (524.4) | 36.66x [1.04x-161.41x] |
| Random-weight MAD (negative controls) | 3 (33) | **15.47x** [0.93x-256.45x] | 15.47x | 0.20x | 332.4 (21.7) | 973.3 (96.7) | 2324.8 (252.0) | 16.14x [1.30x-87.75x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **49.98x** [20.37x-122.62x] | 61.02x | 0.11x | 1158.7 (158.3) | 3200.6 (416.4) | 7973.8 (1102.2) | 41.17x [1.30x-3072.19x] |
| All 31 synthetic corpus families | 31 (293) | **42.92x** [21.67x-85.01x] | 54.54x | 0.12x | 1483.5 (204.7) | 4751.2 (750.3) | 11998.8 (1626.7) | 37.36x [1.04x-3072.19x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2.1 Pure-Go vs. In-Process C++ CBC Aggregate Summary

Comparing `BackendGo` directly against in-process `BackendCBC` removes Python MPS formatting and subprocess spawn overhead across the same five workload subsets:

<!-- BEGIN GENERATED:CBC_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `BackendCBC / BackendGo` Total [Clustered 95% CI] | Equal-Family `BackendCBC / BackendGo` Solve [Clustered 95% CI] | `BackendGo` Trial (Med) Sum (ms) | `BackendCBC` Trial (Med) Sum (ms) | Per-Instance `CBC / Go` Total Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro-models (`N <= 26`) | 10 (110) | **22.55x** [8.98x-56.58x] | **42.20x** [13.32x-133.71x] | 10.0 (0.9) | 154.6 (14.3) | 17.86x [2.63x-144.39x] |
| Tier 2: Medium MILPs (`30 <= N <= 84`) | 13 (143) | **5.74x** [4.07x-8.09x] | **6.38x** [4.28x-9.51x] | 569.4 (37.2) | 1757.2 (155.9) | 6.15x [0.57x-48.33x] |
| Tier 3: Large MILPs (`145 <= N <= 4036`) | 8 (40) | **4.11x** [2.21x-7.65x] | **4.52x** [2.35x-8.67x] | 904.1 (166.6) | 2839.4 (580.0) | 3.86x [0.39x-18.00x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **4.72x** [3.18x-7.01x] | **4.99x** [3.23x-7.71x] | 1149.6 (157.5) | 3058.9 (403.2) | 5.31x [0.57x-48.33x] |
| All 31 synthetic families | 31 (293) | **8.19x** [5.41x-12.39x] | **10.73x** [6.36x-18.12x] | 1483.5 (204.7) | 4751.2 (750.3) | 7.56x [0.39x-144.39x] |
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
| `t1_std_lp` | 3 | 3 | **675.2x** [392.1x-934.4x] | 0.01 | 7.50 | 0.00 | 7.43 | 0.006 | 0.070 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **864.6x** [488.4x-1317.9x] | 0.01 | 6.76 | 0.00 | 6.69 | 0.003 | 0.067 | `OK` |
| `t1_std_mip` | 3 | 3 | **888.4x** [638.3x-1017.6x] | 0.01 | 7.80 | 0.00 | 7.74 | 0.003 | 0.061 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **947.4x** [591.8x-1131.9x] | 0.01 | 7.82 | 0.00 | 7.75 | 0.003 | 0.067 | `OK` |
| `t1_whiskas` | 6 | 5 | **494.0x** [280.6x-639.7x] | 0.01 | 6.43 | 0.01 | 6.35 | 0.006 | 0.076 | `OK` |
| `t1_furniture` | 2 | 3 | **268.7x** [76.1x-950.8x] | 0.02 | 7.76 | 0.02 | 7.70 | 0.004 | 0.052 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **101.8x** [48.6x-322.5x] | 0.11 | 9.70 | 0.11 | 9.61 | 0.008 | 0.100 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **2544.3x** [2089.9x-3072.2x] | 0.00 | 6.59 | 0.00 | 6.54 | 0.002 | 0.048 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **137.3x** [117.4x-161.4x] | 0.07 | 9.98 | 0.04 | 9.76 | 0.024 | 0.230 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **55.9x** [33.2x-87.7x] | 0.62 | 34.41 | 0.58 | 34.16 | 0.037 | 0.244 | `OK` |
| `t2_mkp_30` | 30 | 6 | **9.5x** [4.4x-13.7x] | 2.57 | 28.50 | 2.55 | 28.31 | 0.023 | 0.181 | `OK` |
| `t2_mkp_50` | 50 | 8 | **4.4x** [2.2x-26.3x] | 6.08 | 16.65 | 6.00 | 16.39 | 0.039 | 0.252 | `OK` |
| `t2_cflp_36` | 36 | 11 | **49.8x** [20.6x-133.1x] | 0.18 | 9.09 | 0.16 | 8.91 | 0.019 | 0.159 | `OK` |
| `t2_cflp_49` | 49 | 13 | **31.8x** [18.2x-84.8x] | 0.40 | 11.21 | 0.35 | 10.98 | 0.032 | 0.214 | `OK` |
| `t2_gap_45` | 45 | 13 | **18.1x** [4.4x-47.9x] | 0.92 | 14.06 | 0.89 | 13.85 | 0.027 | 0.211 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **11.8x** [5.0x-29.7x] | 3.95 | 67.92 | 3.89 | 67.69 | 0.032 | 0.242 | `OK` |
| `t2_binpack_44` | 44 | 14 | **11.9x** [4.6x-23.2x] | 1.06 | 13.22 | 1.04 | 13.03 | 0.025 | 0.192 | `OK` |
| `t2_setcover_50` | 50 | 20 | **36.9x** [5.4x-80.2x] | 0.24 | 10.88 | 0.18 | 10.59 | 0.066 | 0.296 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **57.3x** [49.0x-66.5x] | 0.17 | 9.74 | 0.13 | 9.44 | 0.034 | 0.294 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **48.9x** [35.6x-60.0x] | 0.18 | 10.11 | 0.13 | 9.68 | 0.048 | 0.404 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **29.0x** [21.5x-34.9x] | 0.40 | 11.37 | 0.32 | 10.69 | 0.077 | 0.607 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **10.0x** [3.8x-28.0x] | 3.05 | 49.45 | 3.01 | 49.13 | 0.039 | 0.319 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **6.7x** [1.3x-26.9x] | 18.01 | 168.13 | 17.88 | 167.69 | 0.066 | 0.447 | `OK` |
| `t3_mkp_200` | 200 | 8 | **4.5x** [3.4x-5.6x] | 23.03 | 122.36 | 22.95 | 121.70 | 0.077 | 0.660 | `OK` |
| `t3_cflp_300` | 300 | 34 | **31.0x** [7.0x-69.6x] | 6.87 | 252.55 | 6.76 | 251.58 | 0.113 | 0.960 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **2.5x** [1.4x-4.3x] | 82.43 | 158.80 | 82.19 | 156.93 | 0.231 | 1.934 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **9.0x** [4.0x-15.4x] | 8.69 | 84.65 | 8.10 | 81.52 | 0.325 | 2.925 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **32.5x** [25.2x-36.0x] | 0.58 | 19.63 | 0.46 | 18.49 | 0.122 | 1.081 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **16.8x** [11.3x-24.9x] | 1.10 | 20.11 | 0.88 | 18.27 | 0.228 | 1.993 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.4x** [1.0x-12.3x] | 8.97 | 61.37 | 7.98 | 53.53 | 1.593 | 8.412 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **10.6x** [4.6x-24.8x] | 34.92 | 382.14 | 30.54 | 346.13 | 4.319 | 37.022 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

## 4. Scalar vs. `GOEXPERIMENT=simd` Build

Ratios are geometric means of per-trial `scalar / SIMD` solve times over the same instances, with 95% t-intervals on the log ratios. "Reachable" is a necessary condition only: the AVX2 `Dot64`/`Axpy64`/`Scale64` paths need a slice of length at least 32, which requires `n + m >= 32`.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 2 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 9 faster, 0 slower, 14 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 1.04x [0.91x-1.19x] | 0.004 | 0.004 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 0.88x [0.76x-1.02x] | 0.003 | 0.004 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.84x [0.73x-0.98x] | 0.005 | 0.006 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.87x [0.78x-0.96x] | 0.004 | 0.005 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 0.94x [0.82x-1.09x] | 0.006 | 0.006 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 0.95x [0.86x-1.04x] | 0.016 | 0.019 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source | 0.94x [0.82x-1.08x] | 0.107 | 0.112 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 0.95x [0.89x-1.02x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 0.99x [0.93x-1.06x] | 0.045 | 0.044 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | SIMD faster | 1.13x [1.05x-1.21x] | 0.575 | 0.478 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 1.02x [0.98x-1.06x] | 2.545 | 2.433 | 11 |
| `t2_mkp_50` | 58 | possible | SIMD faster | 1.06x [1.02x-1.11x] | 6.004 | 5.649 | 11 |
| `t2_cflp_36` | 47 | possible | no detectable effect | 1.00x [0.96x-1.04x] | 0.159 | 0.156 | 11 |
| `t2_cflp_49` | 62 | possible | no detectable effect | 0.99x [0.91x-1.08x] | 0.355 | 0.346 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 1.07x [1.00x-1.16x] | 0.887 | 0.858 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 0.97x [0.91x-1.03x] | 3.894 | 3.664 | 11 |
| `t2_binpack_44` | 58 | possible | SIMD faster | 1.03x [1.01x-1.06x] | 1.042 | 1.057 | 11 |
| `t2_setcover_50` | 70 | possible | SIMD faster | 1.05x [1.02x-1.08x] | 0.184 | 0.189 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 1.02x [0.93x-1.13x] | 0.133 | 0.128 | 11 |
| `t2_mad_g3_i9` | 108 | possible | no detectable effect | 1.00x [0.82x-1.21x] | 0.129 | 0.151 | 11 |
| `t2_mad_g4_i12` | 156 | possible | SIMD faster | 1.09x [1.01x-1.19x] | 0.315 | 0.294 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.01x [0.97x-1.05x] | 3.014 | 3.261 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 1.00x [0.97x-1.03x] | 17.877 | 17.282 | 11 |
| `t3_mkp_200` | 208 | possible | SIMD faster | 1.03x [1.01x-1.06x] | 22.949 | 22.710 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.03x [0.94x-1.14x] | 6.762 | 6.350 | 5 |
| `t3_lotsizing_504` | 840 | possible | SIMD faster | 1.04x [1.02x-1.06x] | 82.190 | 81.170 | 5 |
| `t3_sched_1000` | 1100 | possible | SIMD faster | 1.04x [1.01x-1.06x] | 8.100 | 8.035 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 1.02x [0.95x-1.10x] | 0.455 | 0.439 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 1.03x [0.96x-1.11x] | 0.880 | 0.873 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 1.02x [0.93x-1.12x] | 7.982 | 7.666 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | SIMD faster | 1.09x [1.04x-1.14x] | 30.540 | 28.307 | 5 |
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
| `t1_whiskas` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.00 (1) | 0.003 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.003 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.11** (1 / 8) | 0.13 (2 / 8) | 0.18 (18) | 0.29 (35) | 0.009 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.04** (1 / 0) | 0.04 (1 / 0) | 0.06 (1) | 0.57 (4) | 0.092 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.58** (2 / 0) | 0.63 (12 / 0) | 0.61 (12) | 0.90 (109) | 0.113 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.55** (5 / 9) | 3.68 (40 / 9) | 1.29 (51) | 1.30 (61) | 0.030 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **6.00** (1 / 4) | 15.91 (2 / 4) | 6.09 (118) | 5.72 (111) | 0.046 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.16** (1 / 2) | 0.24 (3 / 2) | 0.19 (7) | 0.19 (7) | 0.027 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.35** (1 / 10) | 0.71 (4 / 10) | 0.36 (9) | 0.34 (9) | 0.040 | 909.02 | 827.30 |
| `t2_gap_45` | **0.89** (1 / 9) | 0.44 (1 / 9) | 1.87 (41) | 2.23 (60) | 0.030 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **3.89** (2 / 37) | 7.79 (25 / 37) | 3.50 (162) | 3.73 (199) | 0.063 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **1.04** (2 / 6) | 1.61 (5 / 6) | 9.83 (772) | 32.17 (1941) | 0.031 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.18** (1 / 6) | 0.44 (2 / 6) | 0.78 (6) | 0.71 (6) | 0.035 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.13** (1 / 0) | 0.91 (8 / 0) | 0.78 (8) | 0.78 (9) | 0.112 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.13** (1 / 0) | 0.14 (2 / 0) | 0.16 (2) | 3.70 (13) | 0.256 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.32** (1 / 0) | 3.56 (22 / 0) | 3.25 (22) | 5.61 (23) | 0.388 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.01** (43 / 0) | 3.25 (112 / 0) | 3.24 (112) | 4.13 (45) | 0.175 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **17.88** (196 / 0) | 33.52 (188 / 0) | 33.27 (188) | 88.20 (682) | 0.312 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **22.95** (250 / 14) | 116.35 (1135 / 16) | 232.14 (5000, `NodeLimit`) | 219.99 (5000, `NodeLimit`) | 0.129 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **6.76** (1 / 28) | 21.69 (54 / 34) | 9.70 (116) | 9.88 (118) | 0.178 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **82.19** (2 / 206) | 125.38 (376 / 206) | 250.02 (2676, `TimeLimit`) | 250.07 (2736, `TimeLimit`) | 0.822 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **8.10** (31 / 55) | 29.17 (87 / 53) | 250.02 (1665, `TimeLimit`) | 250.05 (1646, `TimeLimit`) | 0.241 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.46** (1 / 0) | 3.21 (17 / 0) | 2.95 (17) | 0.51 (1) | 0.669 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.88** (1 / 0) | 9.22 (58 / 0) | 8.75 (58) | 14.73 (74) | 1.640 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.98** (2 / 0) | 22.19 (25 / 0) | 19.99 (25) | 250.27 (153, `TimeLimit`) | 2.799 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **30.54** (1 / 0) | 34.41 (2 / 0) | 31.44 (2) | 250.36 (9, `TimeLimit`) | 59.674 | 208000.00 | 208000.00 |
<!-- END GENERATED:FULL_CHECKPOINT_TABLE -->

## 6. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus (`testdata/assign_corpus/manifest.json`; 11 randomized trials per family for Tiers 1-2 and 5 for Tier 3, 200 instances in total) compares pure-Go `assign.LinearSumFlat` against SciPy's C++ `rectangular_lsap` extension in warm state on bit-identical `float64` cost matrices, timing `linear_sum_assignment` without post-solve NumPy array gather overhead.

### 6.1 Aggregate LSAP Tier Summary

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `SciPy Solve / goblin` GeoMean [Clustered 95% CI] | Pooled Trial `SciPy Solve / goblin` | Equal-Family `SciPy Total / goblin` | `goblin` Solve Trial (Med) Sum (ms) | SciPy Solve Trial (Med) Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.31x** [1.02x-1.68x] | 1.31x | 4.15x | 0.310 (0.0247) | 0.364 (0.0337) | 1.30x [0.16x-2.58x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **2.63x** [1.60x-4.30x] | 2.63x | 2.35x | 12.072 (1.056) | 35.715 (3.211) | 2.32x [1.04x-15.68x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.77x** [1.64x-4.66x] | 2.77x | 3.39x | 259.821 (51.779) | 1115.857 (221.138) | 3.10x [1.09x-6.34x] |
| Square families ($m = n$) | 16 (146) | **2.44x** [1.74x-3.42x] | 2.38x | 3.22x | 266.061 (51.831) | 1136.383 (221.487) | 2.07x [0.39x-15.68x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.70x** [1.03x-2.80x] | 1.52x | 2.74x | 6.143 (1.028) | 15.553 (2.896) | 1.47x [0.16x-3.85x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.01x** [0.95x-4.26x] | 2.17x | 2.45x | 46.164 (8.789) | 74.679 (13.129) | 1.62x [0.39x-15.68x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.21x** [1.69x-2.89x] | 2.11x | 3.08x | 272.204 (52.859) | 1151.936 (224.383) | 1.92x [0.16x-15.68x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 6.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.89x** | 0.0007 | 0.0012 | 0.0008 | 0.0181 | $0.0007 \pm 0.0001$ | $0.0013 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.19x** | 0.0020 | 0.0024 | 0.0024 | 0.0107 | $0.0021 \pm 0.0004$ | $0.0024 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.40x** | 0.0044 | 0.0070 | 0.0064 | 0.0166 | $0.0054 \pm 0.0038$ | $0.0070 \pm 0.0021$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.18x** | 0.0046 | 0.0051 | 0.0067 | 0.0215 | $0.0043 \pm 0.0008$ | $0.0051 \pm 0.0009$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **0.94x** | 0.0053 | 0.0053 | 0.0075 | 0.0210 | $0.0074 \pm 0.0080$ | $0.0055 \pm 0.0009$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.44x** | 0.0076 | 0.0127 | 0.0135 | 0.0401 | $0.0084 \pm 0.0028$ | $0.0119 \pm 0.0025$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.88x** | 0.0314 | 0.0840 | 0.0492 | 0.115 | $0.0311 \pm 0.0096$ | $0.0891 \pm 0.0297$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.10x** | 0.0671 | 0.212 | 0.109 | 0.273 | $0.0668 \pm 0.0059$ | $0.209 \pm 0.0326$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **1.90x** | 0.0573 | 0.111 | 0.102 | 0.169 | $0.0595 \pm 0.0139$ | $0.112 \pm 0.0166$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.57x** | 0.161 | 0.252 | 0.205 | 0.299 | $0.180 \pm 0.0580$ | $0.275 \pm 0.0504$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **12.35x** | 0.0715 | 0.829 | 0.0987 | 0.922 | $0.0687 \pm 0.0107$ | $0.839 \pm 0.0379$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.93x** | 0.0437 | 0.0879 | 0.0860 | 0.175 | $0.0471 \pm 0.0117$ | $0.0892 \pm 0.0098$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **1.40x** | 0.0579 | 0.0809 | 0.105 | 0.154 | $0.0585 \pm 0.0097$ | $0.0813 \pm 0.0076$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **2.48x** | 0.117 | 0.283 | 0.162 | 0.360 | $0.115 \pm 0.0108$ | $0.286 \pm 0.0339$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **2.70x** | 0.449 | 1.271 | 0.697 | 1.676 | $0.470 \pm 0.0704$ | $1.265 \pm 0.132$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.37x** | 1.755 | 6.447 | 2.438 | 8.462 | $1.872 \pm 0.256$ | $6.375 \pm 1.315$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.21x** | 1.277 | 1.510 | 2.033 | 4.228 | $1.243 \pm 0.0673$ | $1.503 \pm 0.0609$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.49x** | 7.209 | 10.407 | 7.637 | 11.383 | $7.281 \pm 2.520$ | $10.693 \pm 2.927$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.32x** | 0.502 | 1.266 | 1.257 | 5.165 | $0.549 \pm 0.107$ | $1.259 \pm 0.0889$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **3.45x** | 0.415 | 1.451 | 0.820 | 5.463 | $0.421 \pm 0.0237$ | $1.454 \pm 0.0580$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **5.15x** | 5.827 | 29.831 | 7.527 | 39.099 | $5.871 \pm 0.421$ | $30.262 \pm 2.779$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.95x** | 34.793 | 170.226 | 47.207 | 223.672 | $34.727 \pm 6.372$ | $171.626 \pm 30.937$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 6.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSkip64`, `InitReducedRow64`, and `Min2ReducedRow64`)

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` with no `>= 8x8` tall transpose and either fused square `Minimize` non-sparse path or `rows * cols < 32`, so no AVX2 kernel runs): 3 of 3 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32`, tall `rows > cols` with `>= 8x8`, or fallback `AllAbsLE64`/`ValidateAndNegate64` at `rows * cols >= 32`): 18 faster, 0 slower, 1 with no detectable effect, out of 19.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.31x [1.25x-1.38x] | 0.0007 | 0.0005 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.30x [1.24x-1.35x] | 0.0020 | 0.0016 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.28x [1.09x-1.50x] | 0.0044 | 0.0038 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | SIMD faster | 1.37x [1.32x-1.41x] | 0.0046 | 0.0033 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | SIMD faster | 1.69x [1.15x-2.47x] | 0.0053 | 0.0033 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | SIMD faster | 1.20x [1.15x-1.25x] | 0.0076 | 0.0066 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.45x [1.30x-1.61x] | 0.0314 | 0.0230 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.65x [1.54x-1.77x] | 0.0671 | 0.0420 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.78x [1.30x-2.44x] | 0.0573 | 0.0300 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 2.01x [1.77x-2.28x] | 0.161 | 0.0835 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.40x [1.28x-1.52x] | 0.0715 | 0.0491 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.58x [1.30x-1.92x] | 0.0437 | 0.0287 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.78x [1.51x-2.10x] | 0.0579 | 0.0309 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.88x [1.73x-2.05x] | 0.117 | 0.0580 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 2.24x [2.02x-2.48x] | 0.449 | 0.197 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.53x [2.35x-2.73x] | 1.755 | 0.726 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.88x [1.14x-3.08x] | 1.277 | 0.574 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.93x [1.67x-2.22x] | 7.209 | 3.369 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.90x [1.39x-2.58x] | 0.502 | 0.298 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | no detectable effect | 1.04x [0.83x-1.31x] | 0.415 | 0.392 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.28x [1.06x-1.56x] | 5.827 | 4.511 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.62x [1.45x-1.82x] | 34.793 | 21.587 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

## 7. Production Binary Footprint and Linker Dead-Code Elimination

Standalone `CGO_ENABLED=0` binaries compiled across the four canonical `goblin/assign` and `goblin` build profiles (`bench/results/goblin_binary_size.json`), verifying that unreferenced `WithCertified`, `WithVIPRTrace`, `encoding/json`, and (in `goblin/assign`) `math/big` symbols are eliminated by the Go linker:

<!-- BEGIN GENERATED:BINARY_SIZE_TABLE -->
| Profile | Default Raw / Stripped (KiB) | `GOEXPERIMENT=simd` Raw / Stripped (KiB) | Stripped Delta vs. Default | `goblin` Text (KiB) | `math/big` Syms | `encoding/json` Syms | `VIPR` Syms | `Certif` Syms |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `assign` default (`LinearSum` + `SolveInto`) | 2822.7 / **1768.2** | 2858.1 / **1832.2** | baseline | 118.7 | `0` | `0` | `0` | `0` |
| `assign` certified (`+ assign.WithCertified(true)`) | 2942.9 / **1844.2** | 2980.7 / **1916.2** | +76.0 KiB (+4.3%) | 177.2 | `0` | `0` | `0` | `17` |
| `goblin` default (`Model.Solve`, `DefaultOptions`) | 5915.4 / **3776.2** | 5798.6 / **3796.2** | baseline | 1168.5 | `130` | `0` | `0` | `0` |
| `goblin` certified + VIPR (`+ WithCertified + WithVIPRTrace`) | 7353.0 / **4752.2** | 7215.7 / **4772.2** | +976.0 KiB (+25.8%) | 1743.7 | `159` | `0` | `220` | `20` |
<!-- END GENERATED:BINARY_SIZE_TABLE -->

## 8. Combinatorial Group-Item Allocation (Automatic `Model.Solve` Dispatch vs. Pure Branch-and-Cut)

The 13-family combinatorial MAD corpus (`testdata/mad_corpus/manifest.json`; 131 randomized trials across the `pmad_base_g10_i2` base instance with exact Trial-0 optimum `24137`, 8 PMAD families across $G \in \{6, 10, 20\}$, $I \in \{4..12\}$, and $\lambda \in \{0.25, 1.0\}$, plus 4 random-weight controls) evaluates default `Model.Solve` (automatic algebraic MAD detection routed to `internal/mad`, alongside internal `StrategyDP` and `StrategyBruteForce` breakdowns when DFS states are at most `250,000`) against 250 ms-capped pure Branch-and-Cut (`WithCombinatorialSearch(false)` on `BackendGo`, and `BackendCBC`) with `1e-9` combinatorial parity and `[BestBound, Incumbent]` containment.

### 8.1 Aggregate Combinatorial MAD Summary

<!-- BEGIN GENERATED:MAD_COMBINATORIAL_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Auto Trial (Med) Sum (ms) | DP Trial (Med) Sum (ms) | DFS Trial (Med) Sum (ms) | MILP Go Trial (Med) Sum (ms) [Opt] | MILP CBC Trial (Med) Sum (ms) [Opt] | Equal-Family `MILP Go / Auto` [95% CI] | Equal-Family `MILP CBC / Auto` [95% CI] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| PMAD base (`G = 10, I = 2`) | 1 (11) | 0.442 (0.0427) | 0.116 (0.0096) | 1.271 (0.116) | 951.0 (21.3) [8/11] | 938.7 (90.8) [11/11] | **789.3x** | **1933.5x** |
| PMAD small & medium (`I <= 10`) | 6 (66) | 148.637 (13.476) | 141.070 (12.891) | 529.869 (48.748) [2/6] | 15977.9 (1501.0) [7/66] | 13468.7 (1219.7) [23/66] | **323.0x** [52.7x-1980.6x] | **250.8x** [55.7x-1130.2x] |
| PMAD large (`I = 12`) | 2 (10) | 370.223 (73.595) | 383.314 (77.677) | skip (>250k) | 2502.5 (500.4) [0/10] | 2542.4 (507.2) [0/10] | **6.9x** [0.9x-53.0x] | **7.0x** [1.0x-48.1x] |
| Random-weight MAD controls (`I <= 12`) | 4 (44) | 5.171 (0.467) | 4.311 (0.388) | 302.085 (27.384) | 2246.4 (268.5) [39/44] | 3917.4 (374.2) [35/44] | **125.6x** [24.7x-638.7x] | **610.3x** [297.8x-1250.8x] |
| **All 13 combinatorial MAD families** | 13 (131) | 524.472 (87.581) | 528.811 (90.966) | 833.225 (76.248) [7/13] | 21677.8 (2291.2) [54/131] | 20867.3 (2192.0) [69/131] | **143.1x** [45.3x-452.2x] | **222.4x** [70.2x-705.0x] |
<!-- END GENERATED:MAD_COMBINATORIAL_SUMMARY_TABLE -->

### 8.2 Full 13-Family Combinatorial MAD Pipeline Comparison

<!-- BEGIN GENERATED:MAD_COMBINATORIAL_PIPELINE_TABLE -->
| Problem ID | $G \times I \times T$ | $\lambda$ | Auto Strategy | Auto Med (ms) | DP Med (ms) | DFS Med (ms) | MILP Go 250 ms | MILP CBC 250 ms | Trial 0 Obj | Parity |
| :--- | :---: | ---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `pmad_base_g10_i2` | $10 \times 2 \times 6$ | 1.00 | `DP` | 0.0427 | 0.0096 | 0.116 | 21.34 (8/11 opt) | 90.85 (`Optimal`) | 24137.0 | `OK` |
| `pmad_g6_i6_lam025` | $6 \times 6 \times 6$ | 0.25 | `DP` | 0.154 | 0.123 | 35.700 | 250.06 (4/11 opt) | 134.39 (`Optimal`) | 13259.5 | `OK` |
| `pmad_g6_i10_lam10` | $6 \times 10 \times 6$ | 1.00 | `DP` | 4.375 | 4.341 | skip (>250k) | 250.17 (`TimeLimit`) | 252.36 (`TimeLimit`) | 14018.0 | `OK` |
| `pmad_g6_i12_lam025` | $6 \times 12 \times 6$ | 0.25 | `DP` | 30.854 | 35.539 | skip (>250k) | 250.13 (`TimeLimit`) | 251.74 (`TimeLimit`) | 9787.0 | `OK` |
| `pmad_g10_i4_lam025` | $10 \times 4 \times 6$ | 0.25 | `DP` | 0.0957 | 0.0509 | 13.048 | 250.12 (3/11 opt) | 72.01 (`Optimal`) | 23954.0 | `OK` |
| `pmad_g10_i8_lam10` | $10 \times 8 \times 6$ | 1.00 | `DP` | 1.059 | 1.016 | skip (>250k) | 250.16 (`TimeLimit`) | 253.67 (`TimeLimit`) | 21928.0 | `OK` |
| `pmad_g10_i10_lam025` | $10 \times 10 \times 6$ | 0.25 | `DP` | 7.484 | 7.091 | skip (>250k) | 250.27 (`TimeLimit`) | 253.10 (1/11 opt) | 22570.0 | `OK` |
| `pmad_g20_i6_lam10` | $20 \times 6 \times 6$ | 1.00 | `DP` | 0.308 | 0.269 | skip (>250k) | 250.18 (`TimeLimit`) | 254.17 (`TimeLimit`) | 43530.0 | `OK` |
| `pmad_g20_i12_lam10` | $20 \times 12 \times 6$ | 1.00 | `DP` | 42.741 | 42.139 | skip (>250k) | 250.28 (`TimeLimit`) | 255.47 (`TimeLimit`) | 38256.0 | `OK` |
| `mad_comb_rand_g2_i4` | $2 \times 4 \times 6$ | 0.00 | `BruteForce` | 0.0145 | 0.0037 | 0.0023 | 0.56 (`Optimal`) | 10.72 (`Optimal`) | 4601.0 | `OK` |
| `mad_comb_rand_g2_i8` | $2 \times 8 \times 6$ | 0.00 | `BruteForce` | 0.0356 | 0.0311 | 0.0185 | 3.09 (`Optimal`) | 12.42 (`Optimal`) | 5975.0 | `OK` |
| `mad_comb_rand_g3_i9` | $3 \times 9 \times 6$ | 0.00 | `DP` | 0.0680 | 0.0448 | 0.254 | 17.36 (`Optimal`) | 100.02 (`Optimal`) | 7720.0 | `OK` |
| `mad_comb_rand_g4_i12` | $4 \times 12 \times 6$ | 0.00 | `DP` | 0.349 | 0.308 | 27.109 | 247.47 (6/11 opt) | 251.07 (2/11 opt) | 7905.0 | `OK` |
<!-- END GENERATED:MAD_COMBINATORIAL_PIPELINE_TABLE -->
