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
- **Unified telemetry source**: fingerprint `f9ba58e5cc1cece2` (commit `dd90c5cac35e6b8587bf6e627d1169d4100c822a`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.62 ms median on a 1-variable MILP and 5.86 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

## 2. Aggregate Workload Tier Summary

Tier 1 micro-models are dominated by Python subprocess spawn overhead, and the planted-gap MAD family is constructed so that root heuristics close the gap without branching; the bold row isolates solver-dominated Tier 2 and Tier 3 non-planted MILPs across paired trial instances.

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `Py / Go` GeoMean [Clustered 95% CI] | Pooled Trial `Py / Go` | Equal-Family `Go / CBC` | `Go` Trial (Med) Sum (ms) | `CBC` Trial (Med) Sum (ms) | `Py` Trial (Med) Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **310.94x** [147.76x-654.33x] | 310.94x | 0.05x | 9.5 (0.8) | 200.8 (19.9) | 1065.1 (100.0) | 482.17x [36.75x-2272.39x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **15.15x** [7.82x-29.36x] | 15.15x | 0.19x | 881.5 (53.5) | 1974.6 (171.9) | 4283.5 (414.8) | 20.63x [0.12x-115.53x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **9.31x** [4.40x-19.70x] | 9.31x | 0.22x | 936.8 (173.4) | 2897.2 (561.2) | 6168.3 (1061.5) | 9.21x [1.21x-72.24x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **9.48x** [5.23x-17.20x] | 10.14x | 0.27x | 1519.3 (183.3) | 3131.3 (395.5) | 6735.9 (981.4) | 9.98x [0.12x-115.53x] |
| Tier 3 non-MAD MILPs | 4 (20) | **6.60x** [1.21x-36.12x] | 6.60x | 0.33x | 645.9 (130.5) | 1244.8 (231.1) | 2792.9 (597.1) | 4.65x [1.21x-72.24x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **26.51x** [12.41x-56.60x] | 34.49x | 0.11x | 300.1 (43.7) | 1766.3 (340.2) | 3818.9 (504.3) | 38.41x [1.29x-118.35x] |
| Random-weight MAD (negative controls) | 3 (33) | **15.84x** [0.74x-340.44x] | 15.84x | 0.15x | 340.9 (21.9) | 1220.8 (115.8) | 2245.1 (249.0) | 16.10x [1.31x-116.01x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **39.17x** [16.03x-95.71x] | 47.20x | 0.14x | 1527.7 (184.1) | 3306.3 (412.8) | 7698.0 (1072.0) | 39.17x [0.12x-2272.39x] |
| All 31 synthetic corpus families | 31 (293) | **35.41x** [18.09x-69.32x] | 44.08x | 0.13x | 1827.8 (227.7) | 5072.6 (753.0) | 11516.9 (1576.3) | 39.17x [0.12x-2272.39x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2.1 Pure-Go vs. In-Process C++ CBC Aggregate Summary

Comparing `BackendGo` directly against in-process `BackendCBC` removes Python MPS formatting and subprocess spawn overhead across the same five workload subsets:

<!-- BEGIN GENERATED:CBC_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `BackendCBC / BackendGo` Total [Clustered 95% CI] | Equal-Family `BackendCBC / BackendGo` Solve [Clustered 95% CI] | `BackendGo` Trial (Med) Sum (ms) | `BackendCBC` Trial (Med) Sum (ms) | Per-Instance `CBC / Go` Total Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro-models (`N <= 26`) | 10 (110) | **20.03x** [8.76x-45.83x] | **41.98x** [13.51x-130.46x] | 9.5 (0.8) | 200.8 (19.9) | 22.82x [2.43x-124.09x] |
| Tier 2: Medium MILPs (`30 <= N <= 84`) | 13 (143) | **5.14x** [3.09x-8.56x] | **5.65x** [3.23x-9.89x] | 881.5 (53.5) | 1974.6 (171.9) | 6.63x [0.07x-39.28x] |
| Tier 3: Large MILPs (`145 <= N <= 4036`) | 8 (40) | **4.45x** [2.33x-8.47x] | **4.88x** [2.40x-9.93x] | 936.8 (173.4) | 2897.2 (561.2) | 5.82x [0.65x-16.41x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **3.76x** [2.35x-6.01x] | **3.91x** [2.38x-6.42x] | 1519.3 (183.3) | 3131.3 (395.5) | 4.11x [0.07x-39.28x] |
| All 31 synthetic families | 31 (293) | **7.68x** [5.07x-11.63x] | **10.39x** [6.01x-17.96x] | 1827.8 (227.7) | 5072.6 (753.0) | 8.49x [0.07x-124.09x] |
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
| `t1_std_lp` | 3 | 3 | **609.8x** [486.4x-738.3x] | 0.01 | 6.21 | 0.00 | 6.15 | 0.005 | 0.051 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **638.2x** [406.8x-905.8x] | 0.01 | 6.10 | 0.00 | 6.05 | 0.005 | 0.051 | `OK` |
| `t1_std_mip` | 3 | 3 | **614.0x** [478.0x-768.4x] | 0.01 | 7.56 | 0.01 | 7.48 | 0.005 | 0.060 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **650.0x** [502.1x-890.6x] | 0.01 | 7.12 | 0.00 | 7.05 | 0.005 | 0.052 | `OK` |
| `t1_whiskas` | 6 | 5 | **416.8x** [338.8x-580.0x] | 0.02 | 6.40 | 0.01 | 6.32 | 0.007 | 0.076 | `OK` |
| `t1_furniture` | 2 | 3 | **225.4x** [57.2x-807.9x] | 0.03 | 7.74 | 0.02 | 7.68 | 0.005 | 0.053 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **80.1x** [36.8x-285.8x] | 0.14 | 9.04 | 0.13 | 8.95 | 0.010 | 0.095 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **1198.1x** [486.8x-2272.4x] | 0.00 | 5.93 | 0.00 | 5.89 | 0.004 | 0.038 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **92.7x** [54.7x-118.4x] | 0.09 | 9.40 | 0.06 | 9.18 | 0.037 | 0.217 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **65.1x** [42.4x-116.0x] | 0.51 | 34.49 | 0.48 | 34.23 | 0.027 | 0.257 | `OK` |
| `t2_mkp_30` | 30 | 6 | **8.1x** [3.9x-12.0x] | 2.84 | 27.18 | 2.79 | 27.01 | 0.023 | 0.181 | `OK` |
| `t2_mkp_50` | 50 | 8 | **4.0x** [1.8x-27.5x] | 6.15 | 16.85 | 6.13 | 16.58 | 0.028 | 0.269 | `OK` |
| `t2_cflp_36` | 36 | 11 | **41.7x** [12.5x-115.5x] | 0.23 | 9.46 | 0.19 | 9.28 | 0.020 | 0.172 | `OK` |
| `t2_cflp_49` | 49 | 13 | **28.8x** [17.0x-59.4x] | 0.36 | 9.75 | 0.34 | 9.55 | 0.026 | 0.197 | `OK` |
| `t2_gap_45` | 45 | 13 | **18.3x** [4.0x-69.0x] | 0.91 | 14.34 | 0.89 | 14.13 | 0.021 | 0.211 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **9.9x** [4.5x-22.8x] | 5.34 | 68.42 | 5.32 | 68.18 | 0.029 | 0.251 | `OK` |
| `t2_binpack_44` | 44 | 14 | **1.6x** [0.1x-24.3x] | 15.20 | 13.28 | 15.17 | 13.07 | 0.027 | 0.203 | `OK` |
| `t2_setcover_50` | 50 | 20 | **37.2x** [5.2x-115.3x] | 0.30 | 10.51 | 0.27 | 10.20 | 0.034 | 0.302 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **49.5x** [32.4x-60.4x] | 0.18 | 9.25 | 0.14 | 8.95 | 0.034 | 0.289 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **55.1x** [42.9x-72.8x] | 0.17 | 9.87 | 0.13 | 9.44 | 0.045 | 0.420 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **32.4x** [25.2x-39.9x] | 0.36 | 11.36 | 0.29 | 10.72 | 0.067 | 0.595 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.2x** [3.3x-26.5x] | 3.31 | 47.30 | 3.28 | 46.98 | 0.038 | 0.314 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **6.6x** [1.3x-28.4x] | 18.13 | 167.22 | 18.07 | 166.77 | 0.064 | 0.455 | `OK` |
| `t3_mkp_200` | 200 | 8 | **3.5x** [2.9x-4.7x] | 25.41 | 114.80 | 25.34 | 114.19 | 0.075 | 0.613 | `OK` |
| `t3_cflp_300` | 300 | 34 | **25.8x** [8.1x-72.2x] | 9.57 | 244.00 | 9.42 | 243.12 | 0.104 | 0.901 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **2.3x** [1.2x-4.3x] | 86.58 | 158.05 | 86.38 | 156.28 | 0.217 | 1.895 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **8.9x** [4.0x-14.5x] | 8.97 | 80.20 | 8.66 | 77.52 | 0.301 | 2.759 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **27.7x** [21.9x-32.8x] | 0.63 | 18.00 | 0.47 | 16.90 | 0.159 | 1.065 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **16.9x** [11.1x-22.2x] | 1.04 | 18.69 | 0.85 | 16.88 | 0.198 | 1.733 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **6.1x** [1.3x-15.3x] | 8.20 | 59.52 | 7.27 | 50.88 | 0.943 | 8.162 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **10.4x** [4.6x-24.4x] | 33.03 | 368.22 | 29.10 | 334.77 | 4.135 | 35.477 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

## 4. Scalar vs. `GOEXPERIMENT=simd` Build

Ratios are geometric means of per-trial `scalar / SIMD` solve times over the same instances, with 95% t-intervals on the log ratios. "Reachable" is a necessary condition only: the AVX2 `Dot64`/`Axpy64`/`Scale64` paths need a slice of length at least 32, which requires `n + m >= 32`.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 4 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 2 faster, 3 slower, 18 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.67x [0.48x-0.94x] | 0.004 | 0.006 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 0.85x [0.72x-1.00x] | 0.004 | 0.005 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.72x [0.51x-1.00x] | 0.007 | 0.008 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.81x [0.74x-0.88x] | 0.005 | 0.006 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 0.83x [0.63x-1.10x] | 0.007 | 0.008 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 0.88x [0.75x-1.03x] | 0.017 | 0.020 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.08x [1.02x-1.13x] | 0.132 | 0.121 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 0.85x [0.50x-1.43x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | SIMD faster | 1.32x [1.19x-1.45x] | 0.059 | 0.046 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 0.95x [0.89x-1.01x] | 0.480 | 0.488 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 1.03x [1.00x-1.06x] | 2.785 | 2.757 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 0.98x [0.95x-1.01x] | 6.127 | 6.222 | 11 |
| `t2_cflp_36` | 47 | possible | no detectable effect | 0.96x [0.72x-1.27x] | 0.186 | 0.242 | 11 |
| `t2_cflp_49` | 62 | possible | no detectable effect | 1.05x [0.96x-1.16x] | 0.336 | 0.377 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 0.94x [0.87x-1.02x] | 0.891 | 0.937 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 0.97x [0.92x-1.01x] | 5.316 | 4.993 | 11 |
| `t2_binpack_44` | 58 | possible | no detectable effect | 0.97x [0.93x-1.02x] | 15.173 | 15.001 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 0.97x [0.92x-1.01x] | 0.266 | 0.282 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 0.98x [0.91x-1.05x] | 0.138 | 0.136 | 11 |
| `t2_mad_g3_i9` | 108 | possible | SIMD slower | 0.95x [0.91x-0.99x] | 0.127 | 0.131 | 11 |
| `t2_mad_g4_i12` | 156 | possible | SIMD slower | 0.95x [0.92x-0.98x] | 0.290 | 0.305 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 0.97x [0.93x-1.01x] | 3.275 | 3.133 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | SIMD slower | 0.98x [0.96x-1.00x] | 18.066 | 18.029 | 11 |
| `t3_mkp_200` | 208 | possible | no detectable effect | 1.03x [1.00x-1.07x] | 25.337 | 24.174 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.00x [0.96x-1.04x] | 9.415 | 9.315 | 5 |
| `t3_lotsizing_504` | 840 | possible | SIMD faster | 1.02x [1.01x-1.03x] | 86.379 | 83.827 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 1.01x [0.95x-1.07x] | 8.655 | 7.996 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 0.98x [0.93x-1.03x] | 0.469 | 0.475 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 1.02x [0.94x-1.10x] | 0.851 | 0.854 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 1.00x [0.94x-1.06x] | 7.268 | 7.145 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | no detectable effect | 1.02x [0.98x-1.05x] | 29.097 | 29.171 | 5 |
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
| `t1_std_mip` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.002 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.00** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.002 | 64.95 | 58.90 |
| `t1_whiskas` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.004 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.02 (1) | 0.02 (1) | 0.003 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.13** (1 / 8) | 0.17 (2 / 8) | 0.17 (18) | 0.26 (35) | 0.009 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.06** (1 / 0) | 0.04 (1 / 0) | 0.05 (1) | 0.56 (4) | 0.096 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.48** (2 / 0) | 0.60 (12 / 0) | 0.60 (12) | 0.83 (109) | 0.101 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.79** (5 / 9) | 3.77 (40 / 9) | 1.29 (51) | 1.24 (61) | 0.030 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **6.13** (1 / 4) | 19.33 (2 / 4) | 6.62 (118) | 6.24 (111) | 0.048 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.19** (1 / 2) | 0.32 (3 / 2) | 0.19 (7) | 0.21 (7) | 0.033 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.34** (1 / 7) | 0.60 (3 / 7) | 0.33 (9) | 0.32 (9) | 0.035 | 909.02 | 827.30 |
| `t2_gap_45` | **0.89** (1 / 7) | 0.49 (1 / 7) | 2.13 (41) | 2.33 (60) | 0.030 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **5.32** (2 / 37) | 9.49 (25 / 37) | 3.42 (162) | 3.91 (199) | 0.062 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **15.17** (296 / 12) | 17.60 (288 / 12) | 9.01 (772) | 29.34 (1941) | 0.030 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.27** (1 / 6) | 0.43 (2 / 6) | 0.72 (6) | 0.67 (6) | 0.034 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.14** (1 / 0) | 0.77 (8 / 0) | 0.78 (8) | 0.75 (9) | 0.103 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.13** (1 / 0) | 0.16 (2 / 0) | 0.14 (2) | 2.81 (13) | 0.245 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.29** (1 / 0) | 3.33 (22 / 0) | 3.22 (22) | 5.41 (23) | 0.353 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.28** (43 / 0) | 3.11 (317 / 0) | 3.34 (317) | 4.23 (45) | 0.159 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **18.07** (196 / 0) | 26.26 (188 / 0) | 25.20 (188) | 97.82 (663) | 0.288 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **25.34** (250 / 14) | 126.48 (1135 / 16) | 227.30 (5000, `NodeLimit`) | 211.52 (5000, `NodeLimit`) | 0.131 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **9.42** (1 / 28) | 19.32 (54 / 34) | 8.99 (116) | 10.07 (118) | 0.169 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **86.38** (2 / 206) | 119.26 (376 / 206) | 250.08 (2826, `TimeLimit`) | 250.06 (2954, `TimeLimit`) | 0.744 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **8.66** (31 / 55) | 28.13 (87 / 53) | 250.08 (1747, `TimeLimit`) | 250.05 (1744, `TimeLimit`) | 0.228 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.47** (1 / 0) | 3.05 (17 / 0) | 2.73 (17) | 0.50 (1) | 0.670 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.85** (1 / 0) | 9.07 (58 / 0) | 8.00 (58) | 23.37 (134) | 1.540 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.27** (2 / 0) | 20.93 (20 / 0) | 18.81 (20) | 250.11 (164, `TimeLimit`) | 2.741 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **29.10** (1 / 0) | 32.72 (2 / 0) | 33.55 (2) | 250.03 (6, `TimeLimit`) | 56.802 | 208000.00 | 208000.00 |
<!-- END GENERATED:FULL_CHECKPOINT_TABLE -->

## 6. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus (`testdata/assign_corpus/manifest.json`; 11 randomized trials per family for Tiers 1-2 and 5 for Tier 3, 200 instances in total) compares pure-Go `assign.LinearSumFlat` against SciPy's C++ `rectangular_lsap` extension in warm state on bit-identical `float64` cost matrices, timing `linear_sum_assignment` without post-solve NumPy array gather overhead.

### 6.1 Aggregate LSAP Tier Summary

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `SciPy Solve / goblin` GeoMean [Clustered 95% CI] | Pooled Trial `SciPy Solve / goblin` | Equal-Family `SciPy Total / goblin` | `goblin` Solve Trial (Med) Sum (ms) | SciPy Solve Trial (Med) Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.24x** [0.97x-1.60x] | 1.24x | 3.99x | 0.254 (0.0201) | 0.313 (0.0285) | 1.45x [0.10x-2.67x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **3.07x** [1.96x-4.83x] | 3.07x | 3.12x | 9.188 (0.771) | 32.503 (2.947) | 2.65x [0.74x-17.81x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.89x** [1.89x-4.43x] | 2.89x | 3.68x | 223.587 (45.088) | 932.252 (186.792) | 2.61x [1.47x-5.95x] |
| Square families ($m = n$) | 16 (146) | **2.60x** [1.84x-3.67x] | 2.51x | 3.68x | 228.488 (45.078) | 954.183 (187.807) | 2.20x [0.10x-17.81x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.82x** [1.18x-2.79x] | 1.69x | 3.13x | 4.541 (0.801) | 10.885 (1.961) | 2.15x [0.66x-3.02x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.27x** [1.14x-4.53x] | 2.40x | 2.88x | 35.508 (6.161) | 65.053 (11.038) | 1.80x [0.94x-17.81x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.36x** [1.80x-3.08x] | 2.26x | 3.52x | 233.029 (45.879) | 965.069 (189.768) | 2.19x [0.10x-17.81x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 6.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.30x** | 0.0005 | 0.0009 | 0.0006 | 0.0118 | $0.0010 \pm 0.0012$ | $0.0009 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **0.92x** | 0.0018 | 0.0024 | 0.0025 | 0.0102 | $0.0037 \pm 0.0052$ | $0.0024 \pm 0.0004$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.62x** | 0.0037 | 0.0062 | 0.0054 | 0.0152 | $0.0037 \pm 0.0009$ | $0.0061 \pm 0.0017$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.03x** | 0.0039 | 0.0039 | 0.0056 | 0.0152 | $0.0039 \pm 0.0012$ | $0.0039 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.13x** | 0.0039 | 0.0042 | 0.0047 | 0.0152 | $0.0041 \pm 0.0012$ | $0.0046 \pm 0.0010$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.65x** | 0.0064 | 0.0109 | 0.0108 | 0.0325 | $0.0065 \pm 0.0018$ | $0.0106 \pm 0.0019$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.66x** | 0.0294 | 0.0709 | 0.0355 | 0.0963 | $0.0264 \pm 0.0063$ | $0.0689 \pm 0.0107$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.36x** | 0.0529 | 0.182 | 0.0696 | 0.235 | $0.0535 \pm 0.0062$ | $0.180 \pm 0.0243$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.07x** | 0.0456 | 0.0939 | 0.0683 | 0.146 | $0.0470 \pm 0.0110$ | $0.0961 \pm 0.0141$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.62x** | 0.134 | 0.218 | 0.141 | 0.255 | $0.143 \pm 0.0455$ | $0.227 \pm 0.0457$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **12.21x** | 0.0577 | 0.687 | 0.0777 | 0.764 | $0.0581 \pm 0.0147$ | $0.690 \pm 0.0205$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **2.19x** | 0.0328 | 0.0749 | 0.0541 | 0.153 | $0.0380 \pm 0.0195$ | $0.0778 \pm 0.0069$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.28x** | 0.0365 | 0.0845 | 0.0584 | 0.156 | $0.0371 \pm 0.0029$ | $0.0850 \pm 0.0086$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.63x** | 0.0770 | 0.289 | 0.105 | 0.370 | $0.0808 \pm 0.0160$ | $0.290 \pm 0.0290$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.68x** | 0.305 | 1.247 | 0.453 | 1.693 | $0.351 \pm 0.132$ | $1.240 \pm 0.133$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.14x** | 1.252 | 5.139 | 1.620 | 7.704 | $1.273 \pm 0.161$ | $5.248 \pm 0.319$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.73x** | 0.849 | 1.490 | 1.424 | 4.346 | $0.861 \pm 0.0708$ | $1.484 \pm 0.0518$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.66x** | 5.065 | 8.532 | 5.259 | 9.413 | $5.671 \pm 2.096$ | $9.261 \pm 2.727$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.59x** | 0.330 | 0.813 | 0.698 | 3.799 | $0.326 \pm 0.0197$ | $0.847 \pm 0.0823$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.39x** | 0.395 | 0.980 | 0.762 | 3.646 | $0.399 \pm 0.0182$ | $0.953 \pm 0.0535$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **5.00x** | 5.695 | 28.050 | 7.543 | 36.300 | $5.714 \pm 0.293$ | $28.616 \pm 2.366$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.61x** | 31.502 | 141.788 | 38.485 | 186.234 | $30.473 \pm 3.826$ | $140.042 \pm 11.866$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 6.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSkip64`, `InitReducedRow64`, and `Min2ReducedRow64`)

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` with no `>= 8x8` tall transpose and either fused square `Minimize` non-sparse path or `rows * cols < 32`, so no AVX2 kernel runs): 1 of 3 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32`, tall `rows > cols` with `>= 8x8`, or fallback `AllAbsLE64`/`ValidateAndNegate64` at `rows * cols >= 32`): 17 faster, 1 slower, 1 with no detectable effect, out of 19.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.91x [0.86x-0.95x] | 0.0005 | 0.0006 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source | 1.02x [0.98x-1.07x] | 0.0018 | 0.0018 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source | 0.92x [0.78x-1.08x] | 0.0037 | 0.0038 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | no detectable effect | 1.01x [0.84x-1.21x] | 0.0039 | 0.0036 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | SIMD faster | 1.07x [1.05x-1.09x] | 0.0039 | 0.0036 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | SIMD slower | 0.93x [0.90x-0.97x] | 0.0064 | 0.0066 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.22x [1.18x-1.26x] | 0.0294 | 0.0231 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.29x [1.18x-1.42x] | 0.0529 | 0.0427 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.47x [1.15x-1.88x] | 0.0456 | 0.0307 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.61x [1.54x-1.68x] | 0.134 | 0.0827 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.24x [1.13x-1.37x] | 0.0577 | 0.0475 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.21x [1.16x-1.26x] | 0.0328 | 0.0274 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.23x [1.16x-1.30x] | 0.0365 | 0.0300 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.25x [1.20x-1.31x] | 0.0770 | 0.0589 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.62x [1.41x-1.86x] | 0.305 | 0.202 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.47x [1.02x-2.10x] | 1.252 | 0.808 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.38x [2.30x-2.46x] | 0.849 | 0.349 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.64x [2.52x-2.76x] | 5.065 | 1.913 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.69x [1.58x-1.81x] | 0.330 | 0.206 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.55x [1.49x-1.62x] | 0.395 | 0.250 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.64x [1.54x-1.74x] | 5.695 | 3.482 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.46x [1.29x-1.64x] | 31.502 | 19.689 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

## 7. Production Binary Footprint and Linker Dead-Code Elimination

Standalone `CGO_ENABLED=0` binaries compiled across the four canonical `goblin/assign` and `goblin` build profiles (`bench/results/goblin_binary_size.json`), verifying that unreferenced `WithCertified`, `WithVIPRTrace`, `encoding/json`, and (in `goblin/assign`) `math/big` symbols are eliminated by the Go linker:

<!-- BEGIN GENERATED:BINARY_SIZE_TABLE -->
| Profile | Default Raw / Stripped (KiB) | `GOEXPERIMENT=simd` Raw / Stripped (KiB) | Stripped Delta vs. Default | `goblin` Text (KiB) | `math/big` Syms | `encoding/json` Syms | `VIPR` Syms | `Certif` Syms |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `assign` default (`LinearSum` + `SolveInto`) | 2783.7 / **1744.2** | 2820.2 / **1808.2** | baseline | 118.4 | `0` | `0` | `0` | `0` |
| `assign` certified (`+ assign.WithCertified(true)`) | 2907.5 / **1824.2** | 2944.3 / **1892.2** | +80.0 KiB (+4.6%) | 176.9 | `0` | `0` | `0` | `17` |
| `goblin` default (`Model.Solve`, `DefaultOptions`) | 5403.3 / **3448.2** | 5306.0 / **3468.2** | baseline | 950.2 | `130` | `0` | `0` | `0` |
| `goblin` certified + VIPR (`+ WithCertified + WithVIPRTrace`) | 6622.6 / **4260.2** | 6482.5 / **4276.2** | +812.0 KiB (+23.5%) | 1413.7 | `155` | `0` | `215` | `14` |
<!-- END GENERATED:BINARY_SIZE_TABLE -->
