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
- **Unified telemetry source**: fingerprint `06fa79b9f6ebed61` (commit `56a3b68d01d84505bb6a45fe00028f2c7ffaccbb`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 8.27 ms median on a 1-variable MILP and 7.93 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

## 2. Aggregate Workload Tier Summary

Tier 1 micro-models are dominated by Python subprocess spawn overhead, and the planted-gap MAD family is constructed so that root heuristics close the gap without branching; the bold row isolates solver-dominated Tier 2 and Tier 3 non-planted MILPs across paired trial instances.

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `Py / Go` GeoMean [Clustered 95% CI] | Pooled Trial `Py / Go` | Equal-Family `Go / CBC` | `Go` Trial (Med) Sum (ms) | `CBC` Trial (Med) Sum (ms) | `Py` Trial (Med) Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **361.58x** [173.38x-754.06x] | 361.58x | 0.06x | 10.5 (0.9) | 155.9 (14.5) | 1338.9 (125.9) | 427.49x [32.02x-2302.22x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **19.89x** [11.73x-33.75x] | 19.89x | 0.17x | 609.9 (41.0) | 2034.1 (191.1) | 4931.1 (477.0) | 24.90x [1.36x-177.43x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **8.99x** [4.08x-19.78x] | 8.99x | 0.31x | 1051.6 (203.9) | 2649.0 (536.1) | 6964.5 (1201.2) | 9.39x [0.94x-74.85x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **12.44x** [6.95x-22.26x] | 13.91x | 0.22x | 1292.2 (190.8) | 3262.3 (426.0) | 7703.0 (1120.8) | 12.35x [1.36x-177.43x] |
| Tier 3 non-MAD MILPs | 4 (20) | **6.80x** [0.99x-46.87x] | 6.80x | 0.34x | 692.6 (150.7) | 1310.1 (241.9) | 3185.0 (680.9) | 4.29x [1.39x-74.85x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **26.38x** [10.96x-63.47x] | 35.58x | 0.18x | 370.1 (54.2) | 1433.4 (302.2) | 4331.9 (570.2) | 33.77x [0.94x-189.74x] |
| Random-weight MAD (negative controls) | 3 (33) | **15.91x** [0.79x-320.97x] | 15.91x | 0.18x | 357.7 (24.3) | 1190.1 (125.6) | 2515.9 (276.4) | 14.99x [1.36x-106.98x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **48.27x** [20.36x-114.42x] | 59.27x | 0.13x | 1301.8 (191.6) | 3405.6 (439.4) | 8902.6 (1233.9) | 52.46x [1.36x-2302.22x] |
| All 31 synthetic corpus families | 31 (293) | **41.30x** [21.32x-80.01x] | 53.02x | 0.14x | 1671.9 (245.8) | 4839.0 (741.6) | 13234.5 (1804.1) | 46.04x [0.94x-2302.22x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2.1 Pure-Go vs. In-Process C++ CBC Aggregate Summary

Comparing `BackendGo` directly against in-process `BackendCBC` removes Python MPS formatting and subprocess spawn overhead across the same five workload subsets:

<!-- BEGIN GENERATED:CBC_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `BackendCBC / BackendGo` Total [Clustered 95% CI] | Equal-Family `BackendCBC / BackendGo` Solve [Clustered 95% CI] | `BackendGo` Trial (Med) Sum (ms) | `BackendCBC` Trial (Med) Sum (ms) | Per-Instance `CBC / Go` Total Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro-models (`N <= 26`) | 10 (110) | **16.55x** [6.98x-39.28x] | **35.76x** [11.05x-115.75x] | 10.5 (0.9) | 155.9 (14.5) | 14.91x [2.13x-107.62x] |
| Tier 2: Medium MILPs (`30 <= N <= 84`) | 13 (143) | **5.94x** [4.10x-8.60x] | **6.60x** [4.32x-10.09x] | 609.9 (41.0) | 2034.1 (191.1) | 6.29x [0.63x-45.15x] |
| Tier 3: Large MILPs (`145 <= N <= 4036`) | 8 (40) | **3.26x** [1.79x-5.97x] | **3.62x** [1.90x-6.90x] | 1051.6 (203.9) | 2649.0 (536.1) | 3.42x [0.36x-14.29x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **4.59x** [2.91x-7.23x] | **4.80x** [2.95x-7.82x] | 1292.2 (190.8) | 3262.3 (426.0) | 5.14x [0.63x-45.15x] |
| All 31 synthetic families | 31 (293) | **7.08x** [4.80x-10.45x] | **9.75x** [5.78x-16.45x] | 1671.9 (245.8) | 4839.0 (741.6) | 7.30x [0.36x-107.62x] |
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
| `t1_std_lp` | 3 | 3 | **678.7x** [428.6x-842.9x] | 0.01 | 7.58 | 0.01 | 7.51 | 0.006 | 0.072 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **714.3x** [347.2x-1196.9x] | 0.01 | 7.77 | 0.00 | 7.68 | 0.005 | 0.077 | `OK` |
| `t1_std_mip` | 3 | 3 | **753.8x** [533.8x-978.3x] | 0.01 | 9.53 | 0.01 | 9.45 | 0.006 | 0.080 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **832.8x** [631.8x-1044.0x] | 0.01 | 9.69 | 0.00 | 9.61 | 0.006 | 0.079 | `OK` |
| `t1_whiskas` | 6 | 5 | **340.0x** [257.0x-426.4x] | 0.02 | 8.70 | 0.01 | 8.59 | 0.013 | 0.113 | `OK` |
| `t1_furniture` | 2 | 3 | **241.0x** [76.7x-731.8x] | 0.03 | 9.76 | 0.02 | 9.68 | 0.007 | 0.072 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **105.9x** [56.1x-275.3x] | 0.15 | 12.94 | 0.14 | 12.83 | 0.013 | 0.126 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **1477.5x** [666.5x-2302.2x] | 0.01 | 8.82 | 0.00 | 8.75 | 0.005 | 0.080 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **153.8x** [114.1x-189.7x] | 0.08 | 12.85 | 0.05 | 12.60 | 0.030 | 0.261 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **63.6x** [32.0x-107.0x] | 0.58 | 38.29 | 0.53 | 38.03 | 0.036 | 0.255 | `OK` |
| `t2_mkp_30` | 30 | 6 | **10.0x** [5.3x-14.6x] | 2.74 | 32.61 | 2.69 | 32.41 | 0.029 | 0.207 | `OK` |
| `t2_mkp_50` | 50 | 8 | **5.2x** [2.5x-31.3x] | 5.96 | 19.82 | 5.93 | 19.53 | 0.038 | 0.298 | `OK` |
| `t2_cflp_36` | 36 | 11 | **63.4x** [26.2x-177.4x] | 0.18 | 12.11 | 0.16 | 11.91 | 0.018 | 0.196 | `OK` |
| `t2_cflp_49` | 49 | 13 | **34.5x** [14.9x-102.8x] | 0.39 | 12.53 | 0.37 | 12.18 | 0.026 | 0.231 | `OK` |
| `t2_gap_45` | 45 | 13 | **20.2x** [4.5x-69.3x] | 0.96 | 17.22 | 0.92 | 16.99 | 0.030 | 0.240 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **12.4x** [5.3x-28.0x] | 4.22 | 77.92 | 4.19 | 77.63 | 0.034 | 0.260 | `OK` |
| `t2_binpack_44` | 44 | 14 | **10.4x** [3.6x-19.0x] | 1.62 | 16.38 | 1.59 | 16.12 | 0.039 | 0.237 | `OK` |
| `t2_setcover_50` | 50 | 20 | **53.2x** [6.8x-175.8x] | 0.23 | 13.27 | 0.18 | 12.91 | 0.038 | 0.356 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **57.4x** [46.0x-67.0x] | 0.20 | 11.38 | 0.15 | 11.07 | 0.043 | 0.339 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **42.3x** [32.5x-51.9x] | 0.28 | 12.14 | 0.16 | 11.58 | 0.096 | 0.478 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **31.6x** [29.6x-33.4x] | 0.42 | 13.55 | 0.34 | 12.88 | 0.086 | 0.677 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.2x** [3.3x-24.6x] | 3.51 | 52.70 | 3.41 | 52.36 | 0.060 | 0.324 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **6.9x** [1.4x-29.0x] | 20.25 | 185.36 | 20.18 | 184.91 | 0.071 | 0.459 | `OK` |
| `t3_mkp_200` | 200 | 8 | **3.2x** [2.3x-4.0x] | 38.94 | 128.83 | 38.80 | 128.18 | 0.147 | 0.675 | `OK` |
| `t3_cflp_300` | 300 | 34 | **35.0x** [9.1x-74.9x] | 7.24 | 290.71 | 7.13 | 289.67 | 0.118 | 1.046 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **2.4x** [1.4x-4.6x] | 94.65 | 172.18 | 94.35 | 170.14 | 0.253 | 2.055 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **8.2x** [3.9x-12.3x] | 9.88 | 89.15 | 9.52 | 86.07 | 0.506 | 3.087 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **24.6x** [15.6x-32.8x] | 0.82 | 20.94 | 0.63 | 19.63 | 0.210 | 1.114 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **14.9x** [9.9x-19.8x] | 1.35 | 20.76 | 1.00 | 18.66 | 0.268 | 1.936 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.5x** [0.9x-13.7x] | 9.75 | 66.25 | 8.65 | 57.64 | 1.306 | 9.423 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **9.8x** [4.4x-23.4x] | 41.28 | 412.36 | 34.73 | 371.23 | 5.086 | 40.852 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

## 4. Scalar vs. `GOEXPERIMENT=simd` Build

Ratios are geometric means of per-trial `scalar / SIMD` solve times over the same instances, with 95% t-intervals on the log ratios. "Reachable" is a necessary condition only: the AVX2 `Dot64`/`Axpy64`/`Scale64` paths need a slice of length at least 32, which requires `n + m >= 32`.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 5 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 5 faster, 2 slower, 16 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.13x [1.01x-1.26x] | 0.005 | 0.004 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.87x [0.78x-0.97x] | 0.004 | 0.004 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 0.96x [0.85x-1.09x] | 0.006 | 0.006 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source | 0.81x [0.66x-1.00x] | 0.005 | 0.006 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.60x [1.50x-1.70x] | 0.010 | 0.006 | 11 |
| `t1_furniture` | 5 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.27x [1.06x-1.50x] | 0.024 | 0.017 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.15x [1.02x-1.31x] | 0.139 | 0.110 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 1.14x [0.99x-1.31x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 0.98x [0.75x-1.27x] | 0.051 | 0.047 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 0.96x [0.88x-1.04x] | 0.532 | 0.545 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 1.05x [0.93x-1.17x] | 2.695 | 2.705 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 0.95x [0.90x-1.00x] | 5.926 | 5.724 | 11 |
| `t2_cflp_36` | 47 | possible | SIMD slower | 0.91x [0.84x-0.99x] | 0.162 | 0.189 | 11 |
| `t2_cflp_49` | 62 | possible | no detectable effect | 1.11x [0.80x-1.53x] | 0.368 | 0.334 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 0.97x [0.91x-1.04x] | 0.922 | 1.049 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 0.97x [0.92x-1.04x] | 4.188 | 3.973 | 11 |
| `t2_binpack_44` | 58 | possible | SIMD faster | 1.19x [1.11x-1.28x] | 1.589 | 1.268 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 0.91x [0.78x-1.06x] | 0.180 | 0.186 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 0.99x [0.87x-1.12x] | 0.153 | 0.150 | 11 |
| `t2_mad_g3_i9` | 108 | possible | no detectable effect | 1.00x [0.87x-1.16x] | 0.164 | 0.195 | 11 |
| `t2_mad_g4_i12` | 156 | possible | no detectable effect | 1.01x [0.91x-1.12x] | 0.341 | 0.328 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.02x [0.98x-1.05x] | 3.410 | 3.618 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | SIMD slower | 0.93x [0.88x-0.98x] | 20.179 | 20.469 | 11 |
| `t3_mkp_200` | 208 | possible | SIMD faster | 1.36x [1.17x-1.60x] | 38.797 | 26.652 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 0.92x [0.79x-1.07x] | 7.129 | 7.176 | 5 |
| `t3_lotsizing_504` | 840 | possible | SIMD faster | 1.06x [1.01x-1.12x] | 94.350 | 87.420 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 0.97x [0.82x-1.15x] | 9.523 | 8.875 | 5 |
| `t3_mad_g5_i20` | 240 | possible | SIMD faster | 1.28x [1.05x-1.55x] | 0.628 | 0.466 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 1.11x [0.85x-1.46x] | 1.003 | 0.850 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 1.03x [0.94x-1.13x] | 8.647 | 8.807 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | SIMD faster | 1.11x [1.01x-1.21x] | 34.732 | 33.726 | 5 |
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
| `t1_std_lp` | **0.01** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 54.00 | 54.00 |
| `t1_std_lp_max` | **0.00** (1 / 0) | 0.01 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 80.00 | 80.00 |
| `t1_std_mip` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.002 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.00** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.002 | 64.95 | 58.90 |
| `t1_whiskas` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.006 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.02 (1) | 0.02 (1) | 0.003 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.14** (1 / 8) | 0.13 (2 / 8) | 0.19 (18) | 0.28 (35) | 0.009 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.002 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.05** (1 / 0) | 0.05 (1 / 0) | 0.06 (1) | 0.66 (4) | 0.086 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.53** (2 / 0) | 0.81 (12 / 0) | 0.63 (12) | 0.85 (109) | 0.118 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.69** (5 / 9) | 3.66 (40 / 9) | 1.34 (51) | 1.17 (61) | 0.030 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **5.93** (1 / 4) | 16.25 (2 / 4) | 6.35 (118) | 5.73 (111) | 0.047 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.16** (1 / 2) | 0.31 (3 / 2) | 0.19 (7) | 0.20 (7) | 0.028 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.37** (1 / 10) | 0.65 (4 / 10) | 0.34 (9) | 0.34 (9) | 0.037 | 909.02 | 827.30 |
| `t2_gap_45` | **0.92** (1 / 9) | 0.45 (1 / 9) | 1.94 (41) | 2.13 (60) | 0.028 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **4.19** (2 / 37) | 11.80 (25 / 37) | 5.75 (162) | 4.24 (199) | 0.069 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **1.59** (2 / 6) | 1.87 (5 / 6) | 10.19 (772) | 32.95 (1941) | 0.035 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.18** (1 / 6) | 0.38 (2 / 6) | 0.73 (6) | 0.78 (6) | 0.032 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.15** (1 / 0) | 0.93 (8 / 0) | 0.83 (8) | 0.81 (9) | 0.113 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.16** (1 / 0) | 0.14 (2 / 0) | 0.14 (2) | 3.15 (13) | 0.276 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.34** (1 / 0) | 3.77 (22 / 0) | 3.50 (22) | 6.08 (23) | 0.398 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.41** (43 / 0) | 5.07 (112 / 0) | 5.07 (112) | 6.18 (45) | 0.158 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **20.18** (196 / 0) | 40.50 (188 / 0) | 36.32 (188) | 93.97 (682) | 0.302 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **38.80** (250 / 14) | 121.40 (1135 / 16) | 247.87 (5000, `NodeLimit`) | 248.43 (5000, `NodeLimit`) | 0.203 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **7.13** (1 / 28) | 20.86 (54 / 34) | 10.62 (116) | 10.54 (118) | 0.177 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **94.35** (2 / 206) | 140.89 (376 / 206) | 250.09 (2333, `TimeLimit`) | 250.08 (2387, `TimeLimit`) | 0.906 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **9.52** (31 / 55) | 33.10 (87 / 53) | 250.09 (1462, `TimeLimit`) | 250.13 (1551, `TimeLimit`) | 0.285 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.63** (1 / 0) | 3.88 (17 / 0) | 3.33 (17) | 0.58 (1) | 0.764 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **1.00** (1 / 0) | 10.67 (58 / 0) | 9.58 (58) | 15.79 (74) | 1.845 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **8.65** (2 / 0) | 24.17 (25 / 0) | 22.62 (25) | 250.42 (153, `TimeLimit`) | 3.168 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **34.73** (1 / 0) | 37.90 (2 / 0) | 35.04 (2) | 250.13 (6, `TimeLimit`) | 66.318 | 208000.00 | 208000.00 |
<!-- END GENERATED:FULL_CHECKPOINT_TABLE -->

## 6. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus (`testdata/assign_corpus/manifest.json`; 11 randomized trials per family for Tiers 1-2 and 5 for Tier 3, 200 instances in total) compares pure-Go `assign.LinearSumFlat` against SciPy's C++ `rectangular_lsap` extension in warm state on bit-identical `float64` cost matrices, timing `linear_sum_assignment` without post-solve NumPy array gather overhead.

### 6.1 Aggregate LSAP Tier Summary

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `SciPy Solve / goblin` GeoMean [Clustered 95% CI] | Pooled Trial `SciPy Solve / goblin` | Equal-Family `SciPy Total / goblin` | `goblin` Solve Trial (Med) Sum (ms) | SciPy Solve Trial (Med) Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.42x** [1.11x-1.82x] | 1.42x | 4.31x | 0.261 (0.0224) | 0.347 (0.0308) | 1.39x [0.75x-2.69x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **2.92x** [1.82x-4.69x] | 2.92x | 2.97x | 9.877 (0.843) | 34.860 (3.157) | 2.44x [0.50x-15.74x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.88x** [1.89x-4.39x] | 2.88x | 3.65x | 241.439 (46.387) | 1003.216 (197.847) | 2.58x [1.51x-6.16x] |
| Square families ($m = n$) | 16 (146) | **2.69x** [1.96x-3.69x] | 2.62x | 3.79x | 246.561 (46.376) | 1026.909 (198.966) | 2.24x [0.50x-15.74x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.74x** [1.18x-2.55x] | 1.62x | 2.87x | 5.016 (0.877) | 11.515 (2.069) | 1.94x [0.75x-2.62x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.25x** [1.10x-4.58x] | 2.36x | 2.84x | 37.294 (6.679) | 69.566 (11.661) | 1.78x [0.79x-15.74x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.39x** [1.86x-3.08x] | 2.30x | 3.51x | 251.577 (47.253) | 1038.424 (201.035) | 2.07x [0.50x-15.74x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 6.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **2.01x** | 0.0005 | 0.0010 | 0.0005 | 0.0127 | $0.0005 \pm 0.0001$ | $0.0010 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.52x** | 0.0016 | 0.0026 | 0.0019 | 0.0109 | $0.0017 \pm 0.0004$ | $0.0025 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.61x** | 0.0037 | 0.0065 | 0.0048 | 0.0161 | $0.0041 \pm 0.0012$ | $0.0065 \pm 0.0017$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.03x** | 0.0040 | 0.0043 | 0.0056 | 0.0151 | $0.0043 \pm 0.0016$ | $0.0042 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.19x** | 0.0038 | 0.0047 | 0.0066 | 0.0160 | $0.0041 \pm 0.0012$ | $0.0048 \pm 0.0009$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.36x** | 0.0088 | 0.0117 | 0.0143 | 0.0355 | $0.0091 \pm 0.0022$ | $0.0125 \pm 0.0039$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.37x** | 0.0326 | 0.0756 | 0.0402 | 0.103 | $0.0317 \pm 0.0065$ | $0.0742 \pm 0.0122$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **2.73x** | 0.0546 | 0.199 | 0.0721 | 0.253 | $0.0924 \pm 0.111$ | $0.192 \pm 0.0295$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.13x** | 0.0501 | 0.100 | 0.0747 | 0.156 | $0.0476 \pm 0.0052$ | $0.102 \pm 0.0153$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.64x** | 0.145 | 0.237 | 0.154 | 0.274 | $0.156 \pm 0.0508$ | $0.249 \pm 0.0436$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **12.56x** | 0.0632 | 0.733 | 0.0813 | 0.812 | $0.0592 \pm 0.0087$ | $0.737 \pm 0.0213$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.96x** | 0.0373 | 0.0736 | 0.0663 | 0.153 | $0.0389 \pm 0.0091$ | $0.0753 \pm 0.0084$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **1.98x** | 0.0417 | 0.0850 | 0.0671 | 0.166 | $0.0435 \pm 0.0086$ | $0.0852 \pm 0.0074$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.53x** | 0.0847 | 0.298 | 0.117 | 0.380 | $0.0863 \pm 0.0106$ | $0.304 \pm 0.0353$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.97x** | 0.334 | 1.356 | 0.450 | 1.814 | $0.342 \pm 0.0498$ | $1.350 \pm 0.121$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.19x** | 1.276 | 5.529 | 2.029 | 8.425 | $1.353 \pm 0.159$ | $5.659 \pm 0.412$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.76x** | 0.905 | 1.591 | 1.476 | 4.400 | $0.903 \pm 0.0475$ | $1.590 \pm 0.0637$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.70x** | 5.503 | 8.981 | 6.109 | 10.047 | $5.949 \pm 2.201$ | $9.888 \pm 2.809$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.50x** | 0.362 | 0.882 | 0.763 | 3.981 | $0.361 \pm 0.0293$ | $0.899 \pm 0.0639$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.33x** | 0.428 | 1.020 | 0.976 | 4.094 | $0.443 \pm 0.0339$ | $1.031 \pm 0.0452$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.97x** | 6.241 | 29.266 | 8.000 | 38.259 | $6.103 \pm 0.365$ | $30.385 \pm 2.387$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.58x** | 31.672 | 150.578 | 44.533 | 199.172 | $33.177 \pm 4.231$ | $151.191 \pm 11.530$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 6.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSkip64`, `InitReducedRow64`, and `Min2ReducedRow64`)

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` with no `>= 8x8` tall transpose and either fused square `Minimize` non-sparse path or `rows * cols < 32`, so no AVX2 kernel runs): 1 of 3 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32`, tall `rows > cols` with `>= 8x8`, or fallback `AllAbsLE64`/`ValidateAndNegate64` at `rows * cols >= 32`): 19 faster, 0 slower, 0 with no detectable effect, out of 19.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source | 0.98x [0.94x-1.03x] | 0.0005 | 0.0005 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.06x [1.04x-1.08x] | 0.0016 | 0.0015 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source | 1.07x [0.99x-1.16x] | 0.0037 | 0.0038 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | SIMD faster | 1.28x [1.13x-1.46x] | 0.0040 | 0.0034 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | SIMD faster | 1.16x [1.11x-1.22x] | 0.0038 | 0.0033 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | SIMD faster | 1.27x [1.22x-1.32x] | 0.0088 | 0.0067 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.43x [1.22x-1.67x] | 0.0326 | 0.0244 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.63x [1.03x-2.58x] | 0.0546 | 0.0426 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.68x [1.64x-1.73x] | 0.0501 | 0.0308 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.70x [1.64x-1.77x] | 0.145 | 0.0898 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.23x [1.17x-1.30x] | 0.0632 | 0.0495 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.31x [1.07x-1.60x] | 0.0373 | 0.0288 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.27x [1.05x-1.54x] | 0.0417 | 0.0322 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.39x [1.31x-1.47x] | 0.0847 | 0.0603 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.57x [1.53x-1.61x] | 0.334 | 0.212 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.72x [1.57x-1.89x] | 1.276 | 0.811 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.25x [1.88x-2.69x] | 0.905 | 0.376 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.61x [2.47x-2.75x] | 5.503 | 2.019 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.78x [1.65x-1.92x] | 0.362 | 0.215 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.53x [1.32x-1.78x] | 0.428 | 0.293 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.70x [1.58x-1.82x] | 6.241 | 3.619 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.40x [1.30x-1.50x] | 31.672 | 25.063 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

## 7. Production Binary Footprint and Linker Dead-Code Elimination

Standalone `CGO_ENABLED=0` binaries compiled across the four canonical `goblin/assign` and `goblin` build profiles (`bench/results/goblin_binary_size.json`), verifying that unreferenced `WithCertified`, `WithVIPRTrace`, `encoding/json`, and (in `goblin/assign`) `math/big` symbols are eliminated by the Go linker:

<!-- BEGIN GENERATED:BINARY_SIZE_TABLE -->
| Profile | Default Raw / Stripped (KiB) | `GOEXPERIMENT=simd` Raw / Stripped (KiB) | Stripped Delta vs. Default | `goblin` Text (KiB) | `math/big` Syms | `encoding/json` Syms | `VIPR` Syms | `Certif` Syms |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `assign` default (`LinearSum` + `SolveInto`) | 2822.7 / **1768.2** | 2858.1 / **1832.2** | baseline | 118.7 | `0` | `0` | `0` | `0` |
| `assign` certified (`+ assign.WithCertified(true)`) | 2942.9 / **1844.2** | 2980.6 / **1916.2** | +76.0 KiB (+4.3%) | 177.2 | `0` | `0` | `0` | `17` |
| `goblin` default (`Model.Solve`, `DefaultOptions`) | 5915.6 / **3776.2** | 5798.6 / **3796.2** | baseline | 1168.5 | `130` | `0` | `0` | `0` |
| `goblin` certified + VIPR (`+ WithCertified + WithVIPRTrace`) | 7353.3 / **4752.2** | 7215.9 / **4772.2** | +976.0 KiB (+25.8%) | 1743.7 | `159` | `0` | `220` | `20` |
<!-- END GENERATED:BINARY_SIZE_TABLE -->

## 8. Combinatorial Group-Item Allocation (Automatic `Model.Solve` Dispatch vs. Pure Branch-and-Cut)

The 13-family combinatorial MAD corpus (`testdata/mad_corpus/manifest.json`; 131 randomized trials across the `pmad_base_g10_i2` base instance with exact Trial-0 optimum `24137`, 8 PMAD families across $G \in \{6, 10, 20\}$, $I \in \{4..12\}$, and $\lambda \in \{0.25, 1.0\}$, plus 4 random-weight controls) evaluates default `Model.Solve` (automatic algebraic MAD detection routed to `internal/mad`, alongside internal `StrategyDP` and `StrategyBruteForce` breakdowns when DFS states are at most `250,000`) against 250 ms-capped pure Branch-and-Cut (`WithCombinatorialSearch(false)` on `BackendGo`, in-process `BackendCBC`, and Python `PuLP + CBC` subprocess execution) with `1e-9` combinatorial parity and `[BestBound, Incumbent]` containment.

### 8.1 Aggregate Combinatorial MAD Summary

<!-- BEGIN GENERATED:MAD_COMBINATORIAL_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Auto Trial (Med) Sum (ms) | DP Trial (Med) Sum (ms) | DFS Trial (Med) Sum (ms) | MILP Go Trial (Med) Sum (ms) [Opt] | In-Process CBC Trial (Med) Sum (ms) [Opt] | Python PuLP+CBC Trial (Med) Sum (ms) [Opt] | Equal-Family `MILP Go / Auto` [95% CI] | Equal-Family `CBC / Auto` [95% CI] | Equal-Family `Python / Auto` [95% CI] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| PMAD base (`G = 10, I = 2`) | 1 (11) | 0.423 (0.0360) | 0.113 (0.0099) | 1.148 (0.102) | 947.2 (16.8) [8/11] | 925.3 (82.9) [11/11] | 1749.4 (197.4) [11/11] | **809.5x** | **1962.6x** | **3467.8x** |
| PMAD small & medium (`I <= 10`) | 6 (66) | 155.849 (14.941) | 147.768 (12.930) | 499.020 (47.386) [2/6] | 15914.2 (1500.9) [7/66] | 13427.6 (1213.0) [22/66] | 16446.2 (1492.8) [21/66] | **332.4x** [51.4x-2147.6x] | **257.6x** [55.0x-1207.4x] | **340.4x** [58.9x-1967.3x] |
| PMAD large (`I = 12`) | 2 (10) | 354.478 (68.240) | 350.818 (68.981) | skip (>250k) | 2502.0 (500.4) [0/10] | 2547.5 (509.2) [0/10] | 2775.6 (555.0) [0/10] | **7.3x** [0.2x-218.2x] | **7.5x** [0.3x-194.7x] | **8.1x** [0.4x-149.5x] |
| Random-weight MAD controls (`I <= 12`) | 4 (44) | 5.593 (0.511) | 4.746 (0.426) | 314.123 (28.276) | 2326.0 (277.8) [38/44] | 4260.6 (415.3) [34/44] | 5280.3 (542.5) [33/44] | **118.3x** [21.5x-652.1x] | **589.2x** [284.9x-1218.2x] | **1059.4x** [464.7x-2415.3x] |
| **All 13 combinatorial MAD families** | 13 (131) | 516.344 (83.727) | 503.446 (82.347) | 814.291 (75.764) [7/13] | 21689.4 (2295.9) [53/131] | 21160.9 (2220.3) [67/131] | 26251.6 (2787.7) [65/131] | **144.1x** [45.0x-461.1x] | **225.3x** [71.5x-710.0x] | **324.9x** [91.0x-1160.9x] |
<!-- END GENERATED:MAD_COMBINATORIAL_SUMMARY_TABLE -->

### 8.2 Full 13-Family Combinatorial MAD Pipeline Comparison

<!-- BEGIN GENERATED:MAD_COMBINATORIAL_PIPELINE_TABLE -->
| Problem ID | $G \times I \times T$ | $\lambda$ | Auto Strategy | Auto Med (ms) | DP Med (ms) | DFS Med (ms) | MILP Go 250 ms | In-Process CBC 250 ms | Python PuLP+CBC 250 ms | Trial 0 Obj | Parity |
| :--- | :---: | ---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `pmad_base_g10_i2` | $10 \times 2 \times 6$ | 1.00 | `DP` | 0.0360 | 0.0099 | 0.102 | 16.83 (8/11 opt) | 82.87 (`Optimal`) | 197.40 (`Optimal`) | 24137.0 | `OK` |
| `pmad_g6_i6_lam025` | $6 \times 6 \times 6$ | 0.25 | `DP` | 0.131 | 0.0946 | 36.525 | 250.05 (4/11 opt) | 126.69 (`Optimal`) | 201.48 (9/11 opt) | 13259.5 | `OK` |
| `pmad_g6_i10_lam10` | $6 \times 10 \times 6$ | 1.00 | `DP` | 4.927 | 4.063 | skip (>250k) | 250.14 (`TimeLimit`) | 252.91 (`TimeLimit`) | 275.42 (`TimeLimit`) | 14018.0 | `OK` |
| `pmad_g6_i12_lam025` | $6 \times 12 \times 6$ | 0.25 | `DP` | 23.697 | 24.788 | skip (>250k) | 250.10 (`TimeLimit`) | 252.04 (`TimeLimit`) | 266.85 (`TimeLimit`) | 9787.0 | `OK` |
| `pmad_g10_i4_lam025` | $10 \times 4 \times 6$ | 0.25 | `DP` | 0.0852 | 0.0448 | 10.861 | 250.05 (3/11 opt) | 70.76 (`Optimal`) | 161.44 (`Optimal`) | 23954.0 | `OK` |
| `pmad_g10_i8_lam10` | $10 \times 8 \times 6$ | 1.00 | `DP` | 1.013 | 0.956 | skip (>250k) | 250.13 (`TimeLimit`) | 253.94 (`TimeLimit`) | 283.76 (`TimeLimit`) | 21928.0 | `OK` |
| `pmad_g10_i10_lam025` | $10 \times 10 \times 6$ | 0.25 | `DP` | 8.455 | 7.492 | skip (>250k) | 250.32 (`TimeLimit`) | 253.74 (`TimeLimit`) | 283.70 (1/11 opt) | 22570.0 | `OK` |
| `pmad_g20_i6_lam10` | $20 \times 6 \times 6$ | 1.00 | `DP` | 0.330 | 0.281 | skip (>250k) | 250.17 (`TimeLimit`) | 254.91 (`TimeLimit`) | 287.02 (`TimeLimit`) | 43530.0 | `OK` |
| `pmad_g20_i12_lam10` | $20 \times 12 \times 6$ | 1.00 | `DP` | 44.542 | 44.193 | skip (>250k) | 250.28 (`TimeLimit`) | 257.18 (`TimeLimit`) | 288.10 (`TimeLimit`) | 38256.0 | `OK` |
| `mad_comb_rand_g2_i4` | $2 \times 4 \times 6$ | 0.00 | `BruteForce` | 0.0176 | 0.0042 | 0.0024 | 0.60 (`Optimal`) | 11.74 (`Optimal`) | 38.22 (`Optimal`) | 4601.0 | `OK` |
| `mad_comb_rand_g2_i8` | $2 \times 8 \times 6$ | 0.00 | `BruteForce` | 0.0535 | 0.0489 | 0.0285 | 5.06 (`Optimal`) | 20.26 (`Optimal`) | 51.27 (`Optimal`) | 5975.0 | `OK` |
| `mad_comb_rand_g3_i9` | $3 \times 9 \times 6$ | 0.00 | `DP` | 0.0830 | 0.0510 | 0.353 | 22.14 (`Optimal`) | 131.56 (`Optimal`) | 188.07 (10/11 opt) | 7720.0 | `OK` |
| `mad_comb_rand_g4_i12` | $4 \times 12 \times 6$ | 0.00 | `DP` | 0.357 | 0.322 | 27.892 | 250.04 (5/11 opt) | 251.73 (1/11 opt) | 264.92 (1/11 opt) | 7905.0 | `OK` |
<!-- END GENERATED:MAD_COMBINATORIAL_PIPELINE_TABLE -->
