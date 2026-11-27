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
- **Unified telemetry source**: fingerprint `dbe2691169aaaa14` (commit `1e3538c047b8cb771a9e55d39d0abff97e2ac84d`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 7.05 ms median on a 1-variable MILP and 6.15 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

## 2. Aggregate Workload Tier Summary

Tier 1 micro-models are dominated by Python subprocess spawn overhead, and the planted-gap MAD family is constructed so that root heuristics close the gap without branching; the bold row isolates solver-dominated Tier 2 and Tier 3 non-planted MILPs across paired trial instances.

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `Py / Go` GeoMean [Clustered 95% CI] | Pooled Trial `Py / Go` | Equal-Family `Go / CBC` | `Go` Trial (Med) Sum (ms) | `CBC` Trial (Med) Sum (ms) | `Py` Trial (Med) Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **372.01x** [167.24x-827.47x] | 372.01x | 0.05x | 9.4 (0.8) | 149.5 (14.1) | 1077.1 (100.6) | 538.91x [37.74x-2629.14x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **19.85x** [12.02x-32.77x] | 19.85x | 0.18x | 549.2 (35.5) | 1832.6 (172.3) | 4486.9 (433.7) | 24.90x [1.33x-134.79x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **9.26x** [4.59x-18.69x] | 9.26x | 0.25x | 907.7 (164.2) | 2960.1 (583.4) | 6322.3 (1091.2) | 9.44x [1.07x-66.44x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **12.67x** [7.50x-21.39x] | 13.95x | 0.21x | 1124.4 (152.2) | 3213.1 (436.0) | 6994.6 (1016.5) | 13.25x [1.33x-134.79x] |
| Tier 3 non-MAD MILPs | 4 (20) | **7.51x** [1.31x-42.95x] | 7.51x | 0.25x | 584.2 (117.5) | 1439.1 (268.7) | 2878.0 (616.0) | 5.12x [1.43x-66.44x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **25.62x** [10.94x-59.97x] | 34.68x | 0.17x | 333.3 (47.6) | 1592.0 (320.8) | 3916.0 (517.5) | 37.83x [1.07x-174.75x] |
| Random-weight MAD (negative controls) | 3 (33) | **15.68x** [0.95x-259.97x] | 15.68x | 0.19x | 323.9 (21.2) | 1076.1 (115.8) | 2307.2 (256.2) | 17.04x [1.33x-88.70x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **49.82x** [21.00x-118.15x] | 60.74x | 0.11x | 1132.9 (152.9) | 3350.2 (449.0) | 7970.3 (1108.0) | 46.39x [1.33x-2629.14x] |
| All 31 synthetic corpus families | 31 (293) | **41.96x** [21.65x-81.32x] | 53.75x | 0.13x | 1466.2 (200.5) | 4942.2 (769.8) | 11886.2 (1625.5) | 44.04x [1.07x-2629.14x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2.1 Pure-Go vs. In-Process C++ CBC Aggregate Summary

Comparing `BackendGo` directly against in-process `BackendCBC` removes Python MPS formatting and subprocess spawn overhead across the same five workload subsets:

<!-- BEGIN GENERATED:CBC_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `BackendCBC / BackendGo` Total [Clustered 95% CI] | Equal-Family `BackendCBC / BackendGo` Solve [Clustered 95% CI] | `BackendGo` Trial (Med) Sum (ms) | `BackendCBC` Trial (Med) Sum (ms) | Per-Instance `CBC / Go` Total Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro-models (`N <= 26`) | 10 (110) | **21.04x** [8.73x-50.69x] | **40.46x** [12.91x-126.80x] | 9.4 (0.8) | 149.5 (14.1) | 18.17x [3.09x-134.08x] |
| Tier 2: Medium MILPs (`30 <= N <= 84`) | 13 (143) | **5.60x** [4.15x-7.55x] | **6.28x** [4.39x-8.98x] | 549.2 (35.5) | 1832.6 (172.3) | 5.99x [0.59x-37.88x] |
| Tier 3: Large MILPs (`145 <= N <= 4036`) | 8 (40) | **4.08x** [2.44x-6.83x] | **4.56x** [2.59x-8.00x] | 907.7 (164.2) | 2960.1 (583.4) | 3.90x [0.49x-17.35x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **4.86x** [3.40x-6.96x] | **5.14x** [3.46x-7.65x] | 1124.4 (152.2) | 3213.1 (436.0) | 5.31x [0.59x-37.88x] |
| All 31 synthetic families | 31 (293) | **7.91x** [5.36x-11.68x] | **10.54x** [6.35x-17.52x] | 1466.2 (200.5) | 4942.2 (769.8) | 7.21x [0.49x-134.08x] |
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
| `t1_std_lp` | 3 | 3 | **670.8x** [472.6x-950.4x] | 0.01 | 6.84 | 0.00 | 6.78 | 0.005 | 0.066 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **671.1x** [435.3x-1060.6x] | 0.01 | 6.23 | 0.00 | 6.19 | 0.005 | 0.051 | `OK` |
| `t1_std_mip` | 3 | 3 | **611.6x** [471.5x-773.4x] | 0.01 | 7.34 | 0.01 | 7.29 | 0.005 | 0.051 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **731.8x** [534.9x-904.6x] | 0.01 | 7.22 | 0.00 | 7.17 | 0.005 | 0.053 | `OK` |
| `t1_whiskas` | 6 | 5 | **541.9x** [410.8x-755.1x] | 0.01 | 6.44 | 0.01 | 6.37 | 0.006 | 0.074 | `OK` |
| `t1_furniture` | 2 | 3 | **274.1x** [85.6x-1015.6x] | 0.02 | 7.66 | 0.02 | 7.61 | 0.004 | 0.050 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **103.3x** [46.7x-328.4x] | 0.11 | 9.11 | 0.10 | 9.01 | 0.008 | 0.088 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **2276.3x** [1941.0x-2629.1x] | 0.00 | 6.03 | 0.00 | 5.99 | 0.002 | 0.038 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **126.7x** [63.0x-174.7x] | 0.06 | 9.14 | 0.04 | 8.93 | 0.022 | 0.211 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **56.9x** [37.7x-88.7x] | 0.55 | 34.59 | 0.49 | 34.34 | 0.042 | 0.239 | `OK` |
| `t2_mkp_30` | 30 | 6 | **10.3x** [5.2x-14.7x] | 2.44 | 28.31 | 2.43 | 28.13 | 0.019 | 0.182 | `OK` |
| `t2_mkp_50` | 50 | 8 | **4.9x** [2.5x-28.7x] | 5.11 | 16.36 | 5.09 | 16.10 | 0.029 | 0.259 | `OK` |
| `t2_cflp_36` | 36 | 11 | **47.9x** [23.5x-134.8x] | 0.21 | 10.57 | 0.17 | 10.37 | 0.025 | 0.188 | `OK` |
| `t2_cflp_49` | 49 | 13 | **36.0x** [21.6x-85.0x] | 0.38 | 11.41 | 0.33 | 11.19 | 0.024 | 0.230 | `OK` |
| `t2_gap_45` | 45 | 13 | **21.1x** [4.5x-75.3x] | 0.88 | 15.30 | 0.85 | 15.09 | 0.021 | 0.219 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **12.2x** [5.1x-30.5x] | 3.81 | 71.89 | 3.77 | 71.63 | 0.032 | 0.254 | `OK` |
| `t2_binpack_44` | 44 | 14 | **13.2x** [5.5x-26.3x] | 0.98 | 13.67 | 0.96 | 13.49 | 0.021 | 0.195 | `OK` |
| `t2_setcover_50` | 50 | 20 | **43.0x** [6.8x-134.6x] | 0.23 | 11.40 | 0.18 | 11.06 | 0.038 | 0.316 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **48.7x** [33.3x-60.7x] | 0.18 | 9.46 | 0.14 | 9.15 | 0.035 | 0.290 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **57.3x** [43.0x-67.2x] | 0.18 | 11.11 | 0.13 | 10.66 | 0.050 | 0.453 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **30.9x** [24.0x-37.8x] | 0.41 | 12.67 | 0.31 | 12.01 | 0.096 | 0.663 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.8x** [3.5x-27.2x] | 3.17 | 50.52 | 3.13 | 50.20 | 0.041 | 0.321 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **6.9x** [1.3x-30.4x] | 17.48 | 171.05 | 17.39 | 170.63 | 0.059 | 0.447 | `OK` |
| `t3_mkp_200` | 200 | 8 | **4.2x** [3.2x-5.1x] | 23.23 | 114.66 | 23.15 | 114.05 | 0.078 | 0.622 | `OK` |
| `t3_cflp_300` | 300 | 34 | **32.7x** [7.5x-66.4x] | 6.21 | 247.83 | 6.11 | 246.93 | 0.100 | 0.922 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **2.6x** [1.4x-4.8x] | 80.17 | 172.18 | 79.95 | 168.98 | 0.226 | 2.090 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **8.7x** [4.1x-14.8x] | 7.89 | 81.35 | 7.59 | 78.41 | 0.341 | 2.896 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **21.4x** [13.1x-32.2x] | 0.90 | 19.13 | 0.55 | 17.96 | 0.340 | 1.042 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **14.5x** [8.4x-20.0x] | 1.19 | 19.07 | 0.91 | 17.20 | 0.368 | 1.825 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.6x** [1.1x-12.6x] | 8.25 | 63.13 | 7.39 | 54.18 | 1.227 | 8.545 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **9.9x** [4.3x-23.3x] | 36.38 | 373.82 | 31.83 | 339.07 | 4.391 | 35.477 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

## 4. Scalar vs. `GOEXPERIMENT=simd` Build

Ratios are geometric means of per-trial `scalar / SIMD` solve times over the same instances, with 95% t-intervals on the log ratios. "Reachable" is a necessary condition only: the AVX2 `Dot64`/`Axpy64`/`Scale64` paths need a slice of length at least 32, which requires `n + m >= 32`.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 5 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 6 faster, 2 slower, 15 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.21x [1.07x-1.38x] | 0.005 | 0.003 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.21x [1.08x-1.36x] | 0.004 | 0.003 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 1.25x [1.00x-1.56x] | 0.006 | 0.004 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.07x [1.03x-1.11x] | 0.004 | 0.004 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 1.04x [1.00x-1.08x] | 0.005 | 0.005 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 1.03x [0.94x-1.13x] | 0.017 | 0.018 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.79x [0.74x-0.84x] | 0.099 | 0.131 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.66x [0.46x-0.96x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 0.81x [0.57x-1.16x] | 0.042 | 0.063 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | SIMD faster | 1.12x [1.08x-1.16x] | 0.494 | 0.468 | 11 |
| `t2_mkp_30` | 36 | possible | SIMD slower | 0.96x [0.92x-1.00x] | 2.425 | 2.389 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 1.02x [0.96x-1.08x] | 5.085 | 5.076 | 11 |
| `t2_cflp_36` | 47 | possible | SIMD faster | 1.11x [1.07x-1.14x] | 0.174 | 0.163 | 11 |
| `t2_cflp_49` | 62 | possible | no detectable effect | 1.04x [0.95x-1.13x] | 0.326 | 0.320 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 0.98x [0.95x-1.00x] | 0.855 | 0.861 | 11 |
| `t2_lotsizing_48` | 80 | possible | SIMD faster | 1.07x [1.04x-1.10x] | 3.772 | 3.542 | 11 |
| `t2_binpack_44` | 58 | possible | SIMD slower | 0.91x [0.84x-0.98x] | 0.964 | 1.027 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 1.00x [0.92x-1.09x] | 0.185 | 0.159 | 11 |
| `t2_mad_g2_i8` | 72 | possible | SIMD faster | 1.14x [1.06x-1.22x] | 0.141 | 0.129 | 11 |
| `t2_mad_g3_i9` | 108 | possible | no detectable effect | 1.02x [0.99x-1.06x] | 0.130 | 0.125 | 11 |
| `t2_mad_g4_i12` | 156 | possible | no detectable effect | 0.98x [0.91x-1.04x] | 0.310 | 0.325 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.02x [0.98x-1.05x] | 3.126 | 3.150 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 1.00x [0.98x-1.01x] | 17.391 | 18.085 | 11 |
| `t3_mkp_200` | 208 | possible | SIMD faster | 1.03x [1.00x-1.06x] | 23.153 | 23.075 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.01x [0.92x-1.11x] | 6.110 | 6.113 | 5 |
| `t3_lotsizing_504` | 840 | possible | no detectable effect | 1.02x [0.99x-1.05x] | 79.951 | 79.235 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 1.02x [0.92x-1.13x] | 7.587 | 7.650 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 1.21x [0.98x-1.50x] | 0.547 | 0.457 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 1.08x [0.98x-1.19x] | 0.912 | 0.875 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 0.97x [0.90x-1.05x] | 7.387 | 7.596 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | SIMD faster | 1.09x [1.02x-1.17x] | 31.826 | 28.490 | 5 |
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
| `t1_std_mip` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.00 (1) | 0.01 (3) | 0.002 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.01 (3) | 0.002 | 64.95 | 58.90 |
| `t1_whiskas` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.00 (1) | 0.004 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.003 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.10** (1 / 8) | 0.12 (2 / 8) | 0.17 (18) | 0.25 (35) | 0.008 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.04** (1 / 0) | 0.04 (1 / 0) | 0.07 (1) | 0.59 (4) | 0.090 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.49** (2 / 0) | 0.59 (12 / 0) | 0.57 (12) | 0.78 (109) | 0.109 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.43** (5 / 9) | 3.27 (40 / 9) | 1.18 (51) | 1.13 (61) | 0.029 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **5.09** (1 / 4) | 15.21 (2 / 4) | 5.76 (118) | 5.36 (111) | 0.044 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.17** (1 / 2) | 0.28 (3 / 2) | 0.20 (7) | 0.22 (7) | 0.029 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.33** (1 / 10) | 0.61 (4 / 10) | 0.32 (9) | 0.30 (9) | 0.036 | 909.02 | 827.30 |
| `t2_gap_45` | **0.85** (1 / 9) | 0.42 (1 / 9) | 1.83 (41) | 2.22 (60) | 0.028 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **3.77** (2 / 37) | 8.13 (25 / 37) | 3.40 (162) | 3.81 (199) | 0.063 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **0.96** (2 / 6) | 1.55 (5 / 6) | 9.25 (772) | 30.02 (1941) | 0.029 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.18** (1 / 6) | 0.45 (2 / 6) | 0.74 (6) | 0.71 (6) | 0.051 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.14** (1 / 0) | 0.89 (8 / 0) | 0.79 (8) | 0.81 (9) | 0.110 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.13** (1 / 0) | 0.17 (2 / 0) | 0.19 (2) | 2.83 (13) | 0.257 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.31** (1 / 0) | 3.63 (22 / 0) | 3.24 (22) | 5.64 (23) | 0.371 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.13** (43 / 0) | 3.09 (112 / 0) | 3.02 (112) | 4.77 (45) | 0.166 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **17.39** (196 / 0) | 32.99 (188 / 0) | 33.19 (188) | 87.13 (682) | 0.280 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **23.15** (250 / 14) | 109.90 (1135 / 16) | 232.19 (5000, `NodeLimit`) | 214.54 (5000, `NodeLimit`) | 0.136 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **6.11** (1 / 28) | 19.52 (54 / 34) | 9.62 (116) | 9.59 (118) | 0.203 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **79.95** (2 / 206) | 121.97 (376 / 206) | 250.02 (2748, `TimeLimit`) | 250.10 (2895, `TimeLimit`) | 0.748 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **7.59** (31 / 55) | 28.57 (87 / 53) | 250.13 (1706, `TimeLimit`) | 250.07 (1666, `TimeLimit`) | 0.254 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.55** (1 / 0) | 3.48 (17 / 0) | 3.13 (17) | 0.55 (1) | 0.705 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.91** (1 / 0) | 9.30 (58 / 0) | 8.38 (58) | 14.42 (74) | 1.704 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.39** (2 / 0) | 23.30 (25 / 0) | 21.14 (25) | 250.23 (153, `TimeLimit`) | 2.766 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **31.83** (1 / 0) | 34.31 (2 / 0) | 30.75 (2) | 251.03 (9, `TimeLimit`) | 60.432 | 208000.00 | 208000.00 |
<!-- END GENERATED:FULL_CHECKPOINT_TABLE -->

## 6. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus (`testdata/assign_corpus/manifest.json`; 11 randomized trials per family for Tiers 1-2 and 5 for Tier 3, 200 instances in total) compares pure-Go `assign.LinearSumFlat` against SciPy's C++ `rectangular_lsap` extension in warm state on bit-identical `float64` cost matrices, timing `linear_sum_assignment` without post-solve NumPy array gather overhead.

### 6.1 Aggregate LSAP Tier Summary

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `SciPy Solve / goblin` GeoMean [Clustered 95% CI] | Pooled Trial `SciPy Solve / goblin` | Equal-Family `SciPy Total / goblin` | `goblin` Solve Trial (Med) Sum (ms) | SciPy Solve Trial (Med) Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.59x** [1.29x-1.97x] | 1.59x | 4.41x | 0.215 (0.0191) | 0.332 (0.0293) | 1.61x [0.93x-4.58x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **2.88x** [1.79x-4.64x] | 2.88x | 2.73x | 9.296 (0.826) | 32.747 (2.972) | 2.55x [0.98x-15.77x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.86x** [1.87x-4.36x] | 2.86x | 3.67x | 224.218 (46.276) | 943.525 (189.431) | 2.63x [1.47x-5.81x] |
| Square families ($m = n$) | 16 (146) | **2.74x** [2.02x-3.70x] | 2.68x | 3.59x | 228.965 (46.299) | 965.398 (190.398) | 2.23x [0.93x-15.77x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.81x** [1.30x-2.51x] | 1.68x | 2.99x | 4.765 (0.823) | 11.206 (2.034) | 1.88x [0.94x-2.77x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.22x** [1.13x-4.40x] | 2.35x | 2.60x | 36.206 (6.327) | 65.612 (11.354) | 1.75x [0.93x-15.77x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.44x** [1.93x-3.10x] | 2.36x | 3.42x | 233.729 (47.122) | 976.604 (192.432) | 2.05x [0.93x-15.77x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 6.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **2.07x** | 0.0004 | 0.0009 | 0.0005 | 0.0114 | $0.0005 \pm 0.0001$ | $0.0009 \pm 0.0001$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.75x** | 0.0016 | 0.0024 | 0.0018 | 0.0106 | $0.0016 \pm 0.0004$ | $0.0031 \pm 0.0023$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.70x** | 0.0036 | 0.0066 | 0.0054 | 0.0159 | $0.0037 \pm 0.0011$ | $0.0064 \pm 0.0017$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.23x** | 0.0037 | 0.0041 | 0.0053 | 0.0148 | $0.0035 \pm 0.0007$ | $0.0042 \pm 0.0007$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.26x** | 0.0035 | 0.0044 | 0.0056 | 0.0159 | $0.0037 \pm 0.0009$ | $0.0046 \pm 0.0009$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.68x** | 0.0062 | 0.0109 | 0.0109 | 0.0326 | $0.0066 \pm 0.0019$ | $0.0110 \pm 0.0022$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.60x** | 0.0307 | 0.0696 | 0.0431 | 0.0952 | $0.0277 \pm 0.0071$ | $0.0702 \pm 0.0105$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.25x** | 0.0550 | 0.174 | 0.0877 | 0.224 | $0.0549 \pm 0.0048$ | $0.180 \pm 0.0301$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **1.80x** | 0.0505 | 0.0937 | 0.0900 | 0.142 | $0.0545 \pm 0.0160$ | $0.0961 \pm 0.0141$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.59x** | 0.140 | 0.224 | 0.168 | 0.271 | $0.150 \pm 0.0483$ | $0.232 \pm 0.0449$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **11.81x** | 0.0607 | 0.675 | 0.0825 | 0.750 | $0.0583 \pm 0.0096$ | $0.679 \pm 0.0188$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.84x** | 0.0358 | 0.0679 | 0.0633 | 0.139 | $0.0389 \pm 0.0114$ | $0.0699 \pm 0.0082$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **1.93x** | 0.0401 | 0.0800 | 0.0649 | 0.156 | $0.0431 \pm 0.0118$ | $0.0813 \pm 0.0087$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.51x** | 0.0811 | 0.279 | 0.109 | 0.357 | $0.0822 \pm 0.0081$ | $0.291 \pm 0.0472$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.83x** | 0.332 | 1.308 | 0.435 | 1.710 | $0.336 \pm 0.0501$ | $1.278 \pm 0.117$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.84x** | 1.296 | 5.224 | 1.910 | 7.801 | $1.400 \pm 0.275$ | $5.308 \pm 0.417$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.67x** | 0.883 | 1.448 | 1.446 | 4.079 | $0.877 \pm 0.0483$ | $1.462 \pm 0.0491$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.67x** | 5.183 | 8.896 | 5.737 | 9.914 | $5.764 \pm 2.136$ | $9.407 \pm 2.625$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.57x** | 0.338 | 0.870 | 0.725 | 3.796 | $0.338 \pm 0.0225$ | $0.870 \pm 0.0455$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.44x** | 0.401 | 1.007 | 0.807 | 3.898 | $0.419 \pm 0.0274$ | $1.020 \pm 0.0501$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.92x** | 5.937 | 28.225 | 8.075 | 36.956 | $5.875 \pm 0.269$ | $28.940 \pm 2.247$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.73x** | 32.238 | 143.761 | 39.144 | 189.482 | $30.171 \pm 4.088$ | $141.699 \pm 8.279$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 6.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSkip64`, `InitReducedRow64`, and `Min2ReducedRow64`)

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` with no `>= 8x8` tall transpose and either fused square `Minimize` non-sparse path or `rows * cols < 32`, so no AVX2 kernel runs): 0 of 3 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32`, tall `rows > cols` with `>= 8x8`, or fallback `AllAbsLE64`/`ValidateAndNegate64` at `rows * cols >= 32`): 17 faster, 1 slower, 1 with no detectable effect, out of 19.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source | 0.95x [0.90x-1.01x] | 0.0004 | 0.0005 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source | 1.01x [0.98x-1.04x] | 0.0016 | 0.0016 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source | 0.99x [0.97x-1.01x] | 0.0036 | 0.0037 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | SIMD faster | 1.10x [1.07x-1.14x] | 0.0037 | 0.0034 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | no detectable effect | 1.04x [0.97x-1.11x] | 0.0035 | 0.0033 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | SIMD slower | 0.94x [0.89x-0.98x] | 0.0062 | 0.0065 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.28x [1.24x-1.31x] | 0.0307 | 0.0232 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.35x [1.26x-1.45x] | 0.0550 | 0.0429 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.75x [1.35x-2.27x] | 0.0505 | 0.0298 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.59x [1.47x-1.72x] | 0.140 | 0.0906 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.25x [1.19x-1.31x] | 0.0607 | 0.0473 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.34x [1.07x-1.68x] | 0.0358 | 0.0279 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.29x [1.22x-1.38x] | 0.0401 | 0.0312 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.39x [1.35x-1.44x] | 0.0811 | 0.0581 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.61x [1.57x-1.65x] | 0.332 | 0.207 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.83x [1.32x-2.52x] | 1.296 | 0.752 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.44x [2.25x-2.64x] | 0.883 | 0.353 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.56x [2.48x-2.64x] | 5.183 | 1.959 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.61x [1.23x-2.10x] | 0.338 | 0.209 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.60x [1.53x-1.66x] | 0.401 | 0.260 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.63x [1.48x-1.78x] | 5.937 | 3.529 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.50x [1.29x-1.75x] | 32.238 | 18.965 | 5 |
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
| PMAD base (`G = 10, I = 2`) | 1 (11) | 0.397 (0.0368) | 0.102 (0.0094) | 1.037 (0.0916) | 915.0 (16.4) [8/11] | 735.4 (66.8) [11/11] | **779.9x** | **1678.8x** |
| PMAD small & medium (`I <= 10`) | 6 (66) | 127.791 (10.531) | 126.682 (11.064) | 422.318 (36.580) [2/6] | 15788.7 (1500.8) [8/66] | 13075.2 (1175.4) [25/66] | **358.2x** [62.2x-2063.4x] | **266.8x** [67.2x-1059.5x] |
| PMAD large (`I = 12`) | 2 (10) | 437.644 (87.168) | 417.787 (87.018) | skip (>250k) | 2503.0 (500.7) [0/10] | 2550.9 (510.4) [0/10] | **6.2x** [0.0x-917.3x] | **6.3x** [0.0x-850.4x] |
| Random-weight MAD controls (`I <= 12`) | 4 (44) | 5.769 (0.489) | 4.829 (0.416) | 345.812 (29.447) | 2268.5 (272.6) [38/44] | 3993.6 (379.5) [34/44] | **116.1x** [21.2x-634.8x] | **566.7x** [268.2x-1197.6x] |
| **All 13 combinatorial MAD families** | 13 (131) | 571.601 (98.224) | 549.400 (98.507) | 769.167 (66.119) [7/13] | 21475.2 (2290.5) [54/131] | 20355.2 (2132.1) [70/131] | **144.1x** [44.5x-467.0x] | **218.0x** [69.7x-681.4x] |
<!-- END GENERATED:MAD_COMBINATORIAL_SUMMARY_TABLE -->

### 8.2 Full 13-Family Combinatorial MAD Pipeline Comparison

<!-- BEGIN GENERATED:MAD_COMBINATORIAL_PIPELINE_TABLE -->
| Problem ID | $G \times I \times T$ | $\lambda$ | Auto Strategy | Auto Med (ms) | DP Med (ms) | DFS Med (ms) | MILP Go 250 ms | MILP CBC 250 ms | Trial 0 Obj | Parity |
| :--- | :---: | ---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `pmad_base_g10_i2` | $10 \times 2 \times 6$ | 1.00 | `DP` | 0.0368 | 0.0094 | 0.0916 | 16.43 (8/11 opt) | 66.82 (`Optimal`) | 24137.0 | `OK` |
| `pmad_g6_i6_lam025` | $6 \times 6 \times 6$ | 0.25 | `DP` | 0.114 | 0.0873 | 26.989 | 250.03 (5/11 opt) | 104.07 (`Optimal`) | 13259.5 | `OK` |
| `pmad_g6_i10_lam10` | $6 \times 10 \times 6$ | 1.00 | `DP` | 3.205 | 3.159 | skip (>250k) | 250.09 (`TimeLimit`) | 252.39 (`TimeLimit`) | 14018.0 | `OK` |
| `pmad_g6_i12_lam025` | $6 \times 12 \times 6$ | 0.25 | `DP` | 29.059 | 25.939 | skip (>250k) | 250.29 (`TimeLimit`) | 253.00 (`TimeLimit`) | 9787.0 | `OK` |
| `pmad_g10_i4_lam025` | $10 \times 4 \times 6$ | 0.25 | `DP` | 0.0850 | 0.0369 | 9.591 | 250.07 (3/11 opt) | 58.55 (`Optimal`) | 23954.0 | `OK` |
| `pmad_g10_i8_lam10` | $10 \times 8 \times 6$ | 1.00 | `DP` | 0.797 | 0.751 | skip (>250k) | 250.14 (`TimeLimit`) | 252.57 (3/11 opt) | 21928.0 | `OK` |
| `pmad_g10_i10_lam025` | $10 \times 10 \times 6$ | 0.25 | `DP` | 5.854 | 6.659 | skip (>250k) | 250.20 (`TimeLimit`) | 252.56 (`TimeLimit`) | 22570.0 | `OK` |
| `pmad_g20_i6_lam10` | $20 \times 6 \times 6$ | 1.00 | `DP` | 0.476 | 0.371 | skip (>250k) | 250.23 (`TimeLimit`) | 255.28 (`TimeLimit`) | 43530.0 | `OK` |
| `pmad_g20_i12_lam10` | $20 \times 12 \times 6$ | 1.00 | `DP` | 58.109 | 61.080 | skip (>250k) | 250.41 (`TimeLimit`) | 257.40 (`TimeLimit`) | 38256.0 | `OK` |
| `mad_comb_rand_g2_i4` | $2 \times 4 \times 6$ | 0.00 | `BruteForce` | 0.0208 | 0.0046 | 0.0024 | 0.56 (`Optimal`) | 11.95 (`Optimal`) | 4601.0 | `OK` |
| `mad_comb_rand_g2_i8` | $2 \times 8 \times 6$ | 0.00 | `BruteForce` | 0.0397 | 0.0328 | 0.0197 | 4.23 (`Optimal`) | 14.67 (`Optimal`) | 5975.0 | `OK` |
| `mad_comb_rand_g3_i9` | $3 \times 9 \times 6$ | 0.00 | `DP` | 0.0676 | 0.0456 | 0.261 | 17.76 (`Optimal`) | 101.10 (`Optimal`) | 7720.0 | `OK` |
| `mad_comb_rand_g4_i12` | $4 \times 12 \times 6$ | 0.00 | `DP` | 0.361 | 0.333 | 29.164 | 250.05 (5/11 opt) | 251.73 (1/11 opt) | 7905.0 | `OK` |
<!-- END GENERATED:MAD_COMBINATORIAL_PIPELINE_TABLE -->
