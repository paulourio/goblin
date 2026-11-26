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
- **Unified telemetry source**: fingerprint `8c76940984c7433e` (working tree on top of commit `e8e555d75547739838f16e72cb3e16a5c628106f`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.07 ms median on a 1-variable MILP and 5.66 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

## 2. Aggregate Workload Tier Summary

Tier 1 micro-models are dominated by Python subprocess spawn overhead, and the planted-gap MAD family is constructed so that root heuristics close the gap without branching; the bold row isolates solver-dominated Tier 2 and Tier 3 non-planted MILPs across paired trial instances.

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `Py / Go` GeoMean [Clustered 95% CI] | Pooled Trial `Py / Go` | Equal-Family `Go / CBC` | `Go` Trial (Med) Sum (ms) | `CBC` Trial (Med) Sum (ms) | `Py` Trial (Med) Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **341.25x** [149.23x-780.36x] | 341.25x | 0.05x | 9.1 (0.8) | 144.2 (13.5) | 980.4 (92.6) | 567.01x [34.38x-1747.57x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **18.64x** [11.37x-30.54x] | 18.64x | 0.18x | 526.1 (33.8) | 1633.5 (145.6) | 4184.9 (406.5) | 23.29x [1.26x-144.07x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **9.07x** [4.53x-18.18x] | 9.07x | 0.28x | 857.1 (160.0) | 2424.9 (496.2) | 5924.1 (1024.6) | 8.91x [1.04x-56.87x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **11.93x** [7.19x-19.82x] | 13.09x | 0.22x | 1070.8 (147.0) | 2769.2 (362.7) | 6564.8 (957.7) | 12.44x [1.26x-144.07x] |
| Tier 3 non-MAD MILPs | 4 (20) | **7.25x** [1.31x-40.15x] | 7.25x | 0.30x | 552.6 (113.8) | 1192.6 (221.9) | 2694.9 (579.4) | 5.25x [1.49x-56.87x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **23.70x** [11.08x-50.72x] | 31.23x | 0.18x | 313.8 (46.9) | 1301.4 (280.2) | 3642.8 (482.5) | 35.48x [1.04x-127.46x] |
| Random-weight MAD (negative controls) | 3 (33) | **16.15x** [0.85x-306.12x] | 16.15x | 0.19x | 306.6 (20.2) | 916.7 (91.7) | 2247.5 (244.6) | 15.03x [1.26x-112.26x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **47.24x** [19.95x-111.86x] | 57.50x | 0.12x | 1078.6 (147.6) | 2901.2 (375.0) | 7446.6 (1041.2) | 39.98x [1.26x-1747.57x] |
| All 31 synthetic corpus families | 31 (293) | **39.54x** [20.51x-76.21x] | 50.32x | 0.13x | 1392.4 (194.6) | 4202.5 (655.2) | 11089.4 (1523.7) | 38.96x [1.04x-1747.57x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2.1 Pure-Go vs. In-Process C++ CBC Aggregate Summary

Comparing `BackendGo` directly against in-process `BackendCBC` removes Python MPS formatting and subprocess spawn overhead across the same five workload subsets:

<!-- BEGIN GENERATED:CBC_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `BackendCBC / BackendGo` Total [Clustered 95% CI] | Equal-Family `BackendCBC / BackendGo` Solve [Clustered 95% CI] | `BackendGo` Trial (Med) Sum (ms) | `BackendCBC` Trial (Med) Sum (ms) | Per-Instance `CBC / Go` Total Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro-models (`N <= 26`) | 10 (110) | **20.13x** [8.57x-47.29x] | **38.81x** [12.99x-115.95x] | 9.1 (0.8) | 144.2 (13.5) | 15.18x [2.98x-128.22x] |
| Tier 2: Medium MILPs (`30 <= N <= 84`) | 13 (143) | **5.63x** [4.10x-7.74x] | **6.29x** [4.32x-9.17x] | 526.1 (33.8) | 1633.5 (145.6) | 6.37x [0.54x-41.33x] |
| Tier 3: Large MILPs (`145 <= N <= 4036`) | 8 (40) | **3.58x** [2.07x-6.18x] | **4.02x** [2.20x-7.35x] | 857.1 (160.0) | 2424.9 (496.2) | 3.64x [0.39x-12.10x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **4.57x** [3.12x-6.71x] | **4.85x** [3.17x-7.40x] | 1070.8 (147.0) | 2769.2 (362.7) | 5.02x [0.54x-41.33x] |
| All 31 synthetic families | 31 (293) | **7.56x** [5.10x-11.19x] | **10.08x** [6.07x-16.73x] | 1392.4 (194.6) | 4202.5 (655.2) | 7.37x [0.39x-128.22x] |
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
| `t1_std_lp` | 3 | 3 | **769.9x** [484.1x-983.6x] | 0.01 | 5.73 | 0.00 | 5.69 | 0.003 | 0.044 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **788.9x** [523.1x-1144.6x] | 0.01 | 5.72 | 0.00 | 5.68 | 0.004 | 0.043 | `OK` |
| `t1_std_mip` | 3 | 3 | **719.2x** [562.9x-875.4x] | 0.01 | 6.75 | 0.00 | 6.70 | 0.003 | 0.046 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **807.3x** [650.2x-960.1x] | 0.01 | 6.69 | 0.00 | 6.64 | 0.004 | 0.048 | `OK` |
| `t1_whiskas` | 6 | 5 | **455.8x** [314.1x-601.5x] | 0.01 | 5.78 | 0.01 | 5.72 | 0.006 | 0.065 | `OK` |
| `t1_furniture` | 2 | 3 | **248.0x** [69.0x-866.7x] | 0.02 | 7.01 | 0.02 | 6.97 | 0.004 | 0.043 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **78.0x** [34.4x-239.4x] | 0.13 | 8.33 | 0.11 | 8.25 | 0.011 | 0.077 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **1396.1x** [1075.5x-1747.6x] | 0.00 | 5.62 | 0.00 | 5.59 | 0.003 | 0.036 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **78.8x** [42.5x-127.5x] | 0.11 | 9.14 | 0.06 | 8.94 | 0.048 | 0.206 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **62.6x** [40.0x-112.3x] | 0.48 | 31.81 | 0.46 | 31.56 | 0.027 | 0.239 | `OK` |
| `t2_mkp_30` | 30 | 6 | **9.7x** [4.8x-14.1x] | 2.42 | 29.38 | 2.39 | 29.21 | 0.024 | 0.172 | `OK` |
| `t2_mkp_50` | 50 | 8 | **4.8x** [2.5x-29.9x] | 4.85 | 15.92 | 4.82 | 15.67 | 0.031 | 0.241 | `OK` |
| `t2_cflp_36` | 36 | 11 | **50.8x** [21.4x-144.1x] | 0.17 | 8.52 | 0.15 | 8.36 | 0.018 | 0.150 | `OK` |
| `t2_cflp_49` | 49 | 13 | **29.8x** [17.6x-73.0x] | 0.38 | 9.49 | 0.34 | 9.30 | 0.034 | 0.187 | `OK` |
| `t2_gap_45` | 45 | 13 | **18.9x** [4.2x-60.2x] | 0.86 | 13.32 | 0.83 | 13.11 | 0.021 | 0.182 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **11.9x** [4.8x-31.0x] | 3.44 | 65.91 | 3.41 | 65.68 | 0.030 | 0.239 | `OK` |
| `t2_binpack_44` | 44 | 14 | **11.3x** [5.0x-23.2x] | 1.03 | 12.96 | 1.01 | 12.77 | 0.037 | 0.189 | `OK` |
| `t2_setcover_50` | 50 | 20 | **35.8x** [5.7x-79.4x] | 0.25 | 10.02 | 0.16 | 9.76 | 0.066 | 0.272 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **56.1x** [48.4x-64.1x] | 0.16 | 8.78 | 0.12 | 8.51 | 0.033 | 0.259 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **46.4x** [33.1x-54.7x] | 0.19 | 9.01 | 0.13 | 8.62 | 0.052 | 0.373 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **29.2x** [26.0x-33.2x] | 0.36 | 10.36 | 0.29 | 9.82 | 0.074 | 0.552 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.6x** [3.6x-25.3x] | 2.97 | 44.51 | 2.93 | 44.24 | 0.037 | 0.296 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **7.0x** [1.3x-29.7x] | 16.77 | 168.32 | 16.69 | 167.85 | 0.067 | 0.451 | `OK` |
| `t3_mkp_200` | 200 | 8 | **4.3x** [3.1x-5.5x] | 22.54 | 111.50 | 22.46 | 110.87 | 0.087 | 0.620 | `OK` |
| `t3_cflp_300` | 300 | 34 | **30.0x** [7.9x-56.9x] | 6.37 | 236.56 | 6.27 | 235.67 | 0.110 | 0.889 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **2.5x** [1.5x-4.6x] | 77.31 | 155.08 | 77.08 | 153.27 | 0.217 | 1.799 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **8.6x** [4.0x-14.5x] | 7.60 | 76.22 | 7.30 | 73.37 | 0.304 | 2.617 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **21.0x** [16.9x-26.1x] | 0.78 | 16.84 | 0.50 | 15.82 | 0.266 | 0.969 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **14.5x** [8.4x-21.4x] | 1.05 | 16.82 | 0.83 | 15.23 | 0.352 | 1.606 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.6x** [1.0x-13.9x] | 8.02 | 58.52 | 7.05 | 50.26 | 1.093 | 7.974 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **9.8x** [4.3x-23.6x] | 36.29 | 353.02 | 30.00 | 320.57 | 4.069 | 31.882 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

## 4. Scalar vs. `GOEXPERIMENT=simd` Build

Ratios are geometric means of per-trial `scalar / SIMD` solve times over the same instances, with 95% t-intervals on the log ratios. "Reachable" is a necessary condition only: the AVX2 `Dot64`/`Axpy64`/`Scale64` paths need a slice of length at least 32, which requires `n + m >= 32`.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 5 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 7 faster, 3 slower, 13 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 0.93x [0.84x-1.02x] | 0.003 | 0.004 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 1.01x [0.97x-1.05x] | 0.003 | 0.003 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 1.00x [0.94x-1.07x] | 0.005 | 0.005 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.04x [1.02x-1.05x] | 0.004 | 0.004 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.84x [0.74x-0.97x] | 0.006 | 0.007 | 11 |
| `t1_furniture` | 5 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.91x [0.85x-0.98x] | 0.016 | 0.018 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.94x [0.91x-0.98x] | 0.115 | 0.125 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.48x [1.07x-2.06x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | SIMD faster | 1.64x [1.26x-2.14x] | 0.063 | 0.038 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 0.97x [0.94x-1.01x] | 0.456 | 0.486 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 0.98x [0.92x-1.05x] | 2.391 | 2.349 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 0.99x [0.95x-1.02x] | 4.822 | 5.006 | 11 |
| `t2_cflp_36` | 47 | possible | SIMD slower | 0.96x [0.94x-0.99x] | 0.153 | 0.155 | 11 |
| `t2_cflp_49` | 62 | possible | SIMD faster | 1.09x [1.02x-1.16x] | 0.342 | 0.316 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 0.98x [0.94x-1.02x] | 0.829 | 0.813 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 0.98x [0.93x-1.03x] | 3.407 | 3.745 | 11 |
| `t2_binpack_44` | 58 | possible | SIMD faster | 1.12x [1.07x-1.17x] | 1.008 | 0.938 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 0.98x [0.91x-1.05x] | 0.164 | 0.162 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 0.98x [0.95x-1.01x] | 0.124 | 0.126 | 11 |
| `t2_mad_g3_i9` | 108 | possible | SIMD faster | 1.08x [1.02x-1.14x] | 0.130 | 0.121 | 11 |
| `t2_mad_g4_i12` | 156 | possible | SIMD slower | 0.90x [0.84x-0.96x] | 0.287 | 0.322 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | SIMD slower | 0.96x [0.93x-1.00x] | 2.934 | 3.077 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | SIMD faster | 1.02x [1.00x-1.04x] | 16.694 | 17.404 | 11 |
| `t3_mkp_200` | 208 | possible | SIMD faster | 1.06x [1.02x-1.10x] | 22.461 | 21.269 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.03x [0.95x-1.11x] | 6.265 | 6.196 | 5 |
| `t3_lotsizing_504` | 840 | possible | SIMD faster | 1.03x [1.01x-1.06x] | 77.076 | 74.931 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 1.00x [0.97x-1.03x] | 7.302 | 7.436 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 1.04x [0.84x-1.29x] | 0.504 | 0.509 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 1.03x [1.00x-1.07x] | 0.832 | 0.805 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 0.99x [0.92x-1.07x] | 7.051 | 6.902 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | no detectable effect | 1.08x [0.98x-1.20x] | 29.995 | 26.584 | 5 |
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
| `t1_whiskas` | **0.01** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.003 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.003 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.11** (1 / 8) | 0.13 (2 / 8) | 0.18 (18) | 0.26 (35) | 0.009 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.002 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.06** (1 / 0) | 0.04 (1 / 0) | 0.05 (1) | 0.52 (4) | 0.085 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.46** (2 / 0) | 0.57 (12 / 0) | 0.56 (12) | 0.91 (109) | 0.106 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.39** (5 / 9) | 3.14 (40 / 9) | 1.14 (51) | 1.07 (61) | 0.027 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **4.82** (1 / 4) | 14.47 (2 / 4) | 5.56 (118) | 5.16 (111) | 0.044 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.15** (1 / 2) | 0.23 (3 / 2) | 0.18 (7) | 0.17 (7) | 0.026 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.34** (1 / 10) | 0.64 (4 / 10) | 0.34 (9) | 0.32 (9) | 0.036 | 909.02 | 827.30 |
| `t2_gap_45` | **0.83** (1 / 9) | 0.91 (1 / 9) | 1.71 (41) | 2.03 (60) | 0.028 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **3.41** (2 / 37) | 7.02 (25 / 37) | 3.25 (162) | 3.77 (199) | 0.059 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **1.01** (2 / 6) | 1.43 (5 / 6) | 8.78 (772) | 28.66 (1941) | 0.029 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.16** (1 / 6) | 0.36 (2 / 6) | 0.66 (6) | 0.67 (6) | 0.033 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.12** (1 / 0) | 0.85 (8 / 0) | 0.75 (8) | 0.73 (9) | 0.103 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.13** (1 / 0) | 0.14 (2 / 0) | 0.14 (2) | 2.91 (13) | 0.240 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.29** (1 / 0) | 3.30 (22 / 0) | 3.07 (22) | 5.37 (23) | 0.340 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **2.93** (43 / 0) | 2.97 (112 / 0) | 2.93 (112) | 4.24 (45) | 0.161 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **16.69** (196 / 0) | 31.44 (188 / 0) | 31.38 (188) | 90.99 (539) | 0.271 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **22.46** (250 / 14) | 115.77 (1135 / 16) | 214.91 (5000, `NodeLimit`) | 201.49 (5000, `NodeLimit`) | 0.129 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **6.27** (1 / 28) | 18.54 (54 / 34) | 8.80 (116) | 9.17 (118) | 0.174 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **77.08** (2 / 206) | 114.65 (376 / 206) | 250.02 (2933, `TimeLimit`) | 250.02 (2968, `TimeLimit`) | 0.725 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **7.30** (31 / 55) | 26.82 (87 / 53) | 250.11 (1801, `TimeLimit`) | 250.11 (1782, `TimeLimit`) | 0.238 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.50** (1 / 0) | 3.15 (17 / 0) | 2.84 (17) | 0.51 (1) | 0.725 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.83** (1 / 0) | 8.90 (58 / 0) | 8.29 (58) | 13.88 (74) | 1.557 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.05** (2 / 0) | 21.08 (25 / 0) | 19.96 (25) | 250.52 (145, `TimeLimit`) | 2.563 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **30.00** (1 / 0) | 31.45 (2 / 0) | 30.02 (2) | 250.18 (6, `TimeLimit`) | 56.601 | 208000.00 | 208000.00 |
<!-- END GENERATED:FULL_CHECKPOINT_TABLE -->

## 6. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus (`testdata/assign_corpus/manifest.json`; 11 randomized trials per family for Tiers 1-2 and 5 for Tier 3, 200 instances in total) compares pure-Go `assign.LinearSumFlat` against SciPy's C++ `rectangular_lsap` extension in warm state on bit-identical `float64` cost matrices, timing `linear_sum_assignment` without post-solve NumPy array gather overhead.

### 6.1 Aggregate LSAP Tier Summary

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `SciPy Solve / goblin` GeoMean [Clustered 95% CI] | Pooled Trial `SciPy Solve / goblin` | Equal-Family `SciPy Total / goblin` | `goblin` Solve Trial (Med) Sum (ms) | SciPy Solve Trial (Med) Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.55x** [1.24x-1.95x] | 1.55x | 4.98x | 0.202 (0.0184) | 0.309 (0.0279) | 1.59x [1.02x-3.34x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **2.83x** [1.77x-4.53x] | 2.83x | 2.66x | 9.031 (0.793) | 31.191 (2.829) | 2.50x [1.07x-14.57x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.86x** [1.83x-4.47x] | 2.86x | 3.65x | 207.176 (41.358) | 892.875 (179.306) | 2.64x [1.49x-6.17x] |
| Square families ($m = n$) | 16 (146) | **2.70x** [1.99x-3.67x] | 2.64x | 3.64x | 211.808 (41.357) | 913.540 (180.203) | 2.35x [1.07x-14.57x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.78x** [1.26x-2.51x] | 1.65x | 3.13x | 4.600 (0.812) | 10.835 (1.960) | 1.78x [1.02x-3.11x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.23x** [1.14x-4.36x] | 2.36x | 2.71x | 35.180 (6.054) | 61.633 (10.396) | 1.78x [1.07x-14.57x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.41x** [1.89x-3.07x] | 2.33x | 3.49x | 216.409 (42.169) | 924.375 (182.163) | 1.97x [1.02x-14.57x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 6.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **2.07x** | 0.0004 | 0.0009 | 0.0005 | 0.0113 | $0.0005 \pm 0.0001$ | $0.0010 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.46x** | 0.0016 | 0.0023 | 0.0018 | 0.0098 | $0.0016 \pm 0.0004$ | $0.0023 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.82x** | 0.0033 | 0.0060 | 0.0042 | 0.0147 | $0.0034 \pm 0.0007$ | $0.0063 \pm 0.0021$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.20x** | 0.0035 | 0.0038 | 0.0042 | 0.0139 | $0.0033 \pm 0.0007$ | $0.0039 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.25x** | 0.0035 | 0.0042 | 0.0044 | 0.0145 | $0.0035 \pm 0.0007$ | $0.0044 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.69x** | 0.0060 | 0.0107 | 0.0098 | 0.0315 | $0.0062 \pm 0.0015$ | $0.0104 \pm 0.0019$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.64x** | 0.0279 | 0.0684 | 0.0402 | 0.0943 | $0.0257 \pm 0.0061$ | $0.0667 \pm 0.0103$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.07x** | 0.0537 | 0.168 | 0.0844 | 0.216 | $0.0546 \pm 0.0079$ | $0.168 \pm 0.0259$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **1.96x** | 0.0481 | 0.0933 | 0.0873 | 0.145 | $0.0479 \pm 0.0081$ | $0.0935 \pm 0.0135$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.52x** | 0.139 | 0.204 | 0.173 | 0.242 | $0.150 \pm 0.0576$ | $0.220 \pm 0.0421$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **11.44x** | 0.0605 | 0.663 | 0.0793 | 0.735 | $0.0586 \pm 0.0102$ | $0.661 \pm 0.0206$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.79x** | 0.0358 | 0.0654 | 0.0644 | 0.135 | $0.0409 \pm 0.0170$ | $0.0698 \pm 0.0123$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **1.86x** | 0.0377 | 0.0751 | 0.0639 | 0.144 | $0.0409 \pm 0.0083$ | $0.0751 \pm 0.0066$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.38x** | 0.0795 | 0.270 | 0.112 | 0.340 | $0.0820 \pm 0.0120$ | $0.277 \pm 0.0349$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.79x** | 0.310 | 1.221 | 0.430 | 1.614 | $0.320 \pm 0.0496$ | $1.204 \pm 0.116$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.83x** | 1.268 | 4.935 | 1.631 | 7.279 | $1.337 \pm 0.273$ | $5.047 \pm 0.352$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.63x** | 0.856 | 1.411 | 1.465 | 3.910 | $0.871 \pm 0.0377$ | $1.421 \pm 0.0641$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.59x** | 4.940 | 8.007 | 5.088 | 9.424 | $5.578 \pm 1.947$ | $8.725 \pm 2.545$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.60x** | 0.315 | 0.806 | 0.694 | 3.360 | $0.319 \pm 0.0157$ | $0.832 \pm 0.0861$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.46x** | 0.416 | 1.005 | 0.784 | 4.148 | $0.406 \pm 0.0235$ | $0.998 \pm 0.0218$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **5.06x** | 5.602 | 28.806 | 7.336 | 36.724 | $5.548 \pm 0.342$ | $28.083 \pm 2.269$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.91x** | 27.960 | 134.335 | 35.904 | 174.463 | $27.376 \pm 3.645$ | $133.469 \pm 6.739$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 6.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSkip64`, `InitReducedRow64`, and `Min2ReducedRow64`)

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` with no `>= 8x8` tall transpose and either fused square `Minimize` non-sparse path or `rows * cols < 32`, so no AVX2 kernel runs): 2 of 3 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32`, tall `rows > cols` with `>= 8x8`, or fallback `AllAbsLE64`/`ValidateAndNegate64` at `rows * cols >= 32`): 17 faster, 1 slower, 1 with no detectable effect, out of 19.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.92x [0.87x-0.97x] | 0.0004 | 0.0005 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.05x [1.02x-1.08x] | 0.0016 | 0.0015 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source | 0.93x [0.85x-1.01x] | 0.0033 | 0.0035 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | SIMD faster | 1.08x [1.04x-1.11x] | 0.0035 | 0.0032 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | SIMD faster | 1.07x [1.01x-1.14x] | 0.0035 | 0.0031 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | SIMD slower | 0.94x [0.90x-0.99x] | 0.0060 | 0.0062 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.23x [1.19x-1.27x] | 0.0279 | 0.0223 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.36x [1.29x-1.43x] | 0.0537 | 0.0415 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.70x [1.53x-1.89x] | 0.0481 | 0.0295 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.68x [1.60x-1.76x] | 0.139 | 0.0816 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.25x [1.18x-1.33x] | 0.0605 | 0.0488 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | no detectable effect | 1.32x [0.99x-1.75x] | 0.0358 | 0.0286 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.33x [1.13x-1.57x] | 0.0377 | 0.0288 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.40x [1.24x-1.57x] | 0.0795 | 0.0539 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.54x [1.46x-1.63x] | 0.310 | 0.214 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.76x [1.35x-2.30x] | 1.268 | 0.691 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.09x [1.19x-3.66x] | 0.856 | 0.338 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.65x [2.54x-2.76x] | 4.940 | 1.859 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.67x [1.36x-2.04x] | 0.315 | 0.207 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.67x [1.52x-1.82x] | 0.416 | 0.240 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.65x [1.51x-1.80x] | 5.602 | 3.360 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.49x [1.41x-1.57x] | 27.960 | 17.987 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

## 7. Production Binary Footprint and Linker Dead-Code Elimination

Standalone `CGO_ENABLED=0` binaries compiled across the four canonical `goblin/assign` and `goblin` build profiles (`bench/results/goblin_binary_size.json`), verifying that unreferenced `WithCertified`, `WithVIPRTrace`, `encoding/json`, and (in `goblin/assign`) `math/big` symbols are eliminated by the Go linker:

<!-- BEGIN GENERATED:BINARY_SIZE_TABLE -->
| Profile | Default Raw / Stripped (KiB) | `GOEXPERIMENT=simd` Raw / Stripped (KiB) | Stripped Delta vs. Default | `goblin` Text (KiB) | `math/big` Syms | `encoding/json` Syms | `VIPR` Syms | `Certif` Syms |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `assign` default (`LinearSum` + `SolveInto`) | 2822.8 / **1768.2** | 2858.1 / **1832.2** | baseline | 118.7 | `0` | `0` | `0` | `0` |
| `assign` certified (`+ assign.WithCertified(true)`) | 2942.9 / **1844.2** | 2980.7 / **1916.2** | +76.0 KiB (+4.3%) | 177.2 | `0` | `0` | `0` | `17` |
| `goblin` default (`Model.Solve`, `DefaultOptions`) | 5586.5 / **3560.2** | 5472.0 / **3576.2** | baseline | 1023.4 | `130` | `0` | `0` | `0` |
| `goblin` certified + VIPR (`+ WithCertified + WithVIPRTrace`) | 6833.1 / **4400.2** | 6697.0 / **4420.2** | +840.0 KiB (+23.6%) | 1503.1 | `155` | `0` | `220` | `14` |
<!-- END GENERATED:BINARY_SIZE_TABLE -->
