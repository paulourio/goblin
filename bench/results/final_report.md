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
- **Unified telemetry source**: fingerprint `81fe97d73f995897` (working tree on top of commit `2848940e3a824499dd909f9d87664a23101c1a95`), `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.05 ms median on a 1-variable MILP and 5.74 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

## 2. Aggregate Workload Tier Summary

Tier 1 micro-models are dominated by Python subprocess spawn overhead, and the planted-gap MAD family is constructed so that root heuristics close the gap without branching; the bold row isolates solver-dominated Tier 2 and Tier 3 non-planted MILPs across paired trial instances.

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `Py / Go` GeoMean [Clustered 95% CI] | Pooled Trial `Py / Go` | Equal-Family `Go / CBC` | `Go` Trial (Med) Sum (ms) | `CBC` Trial (Med) Sum (ms) | `Py` Trial (Med) Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **363.28x** [175.82x-750.58x] | 363.28x | 0.04x | 8.3 (0.7) | 189.1 (18.8) | 981.0 (92.8) | 582.64x [36.41x-1156.20x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **18.98x** [10.94x-32.94x] | 18.98x | 0.15x | 739.6 (32.3) | 1927.6 (170.3) | 4098.1 (398.1) | 25.75x [0.11x-145.61x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **9.68x** [4.43x-21.16x] | 9.68x | 0.21x | 800.3 (160.4) | 2853.9 (551.4) | 6044.5 (1040.3) | 9.69x [1.10x-77.34x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **12.18x** [6.90x-21.52x] | 13.31x | 0.20x | 1229.1 (146.2) | 3065.9 (390.3) | 6518.5 (954.6) | 15.26x [0.11x-145.61x] |
| Tier 3 non-MAD MILPs | 4 (20) | **7.54x** [1.11x-51.26x] | 7.54x | 0.28x | 497.2 (114.6) | 1223.8 (227.2) | 2734.9 (584.9) | 5.19x [2.09x-77.34x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **26.16x** [11.61x-58.95x] | 34.58x | 0.10x | 311.7 (46.5) | 1739.7 (333.9) | 3717.8 (492.6) | 35.67x [1.10x-146.46x] |
| Random-weight MAD (negative controls) | 3 (33) | **16.09x** [0.83x-310.20x] | 16.09x | 0.14x | 321.2 (18.2) | 1200.9 (116.5) | 2173.0 (240.8) | 17.64x [1.40x-92.30x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **48.47x** [20.36x-115.37x] | 58.91x | 0.11x | 1236.5 (146.8) | 3230.8 (406.6) | 7405.9 (1038.7) | 44.65x [0.11x-1156.20x] |
| All 31 synthetic corpus families | 31 (293) | **41.34x** [21.35x-80.04x] | 52.44x | 0.11x | 1548.1 (193.4) | 4970.5 (740.5) | 11123.7 (1531.2) | 41.83x [0.11x-1156.20x] |
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
| `t1_std_lp` | 3 | 3 | **668.0x** [449.4x-1018.9x] | 0.01 | 5.86 | 0.00 | 5.82 | 0.005 | 0.061 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **967.5x** [697.2x-1073.9x] | 0.01 | 5.82 | 0.00 | 5.78 | 0.003 | 0.061 | `OK` |
| `t1_std_mip` | 3 | 3 | **590.1x** [267.9x-697.9x] | 0.01 | 6.81 | 0.00 | 6.76 | 0.006 | 0.061 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **918.7x** [789.5x-1027.6x] | 0.01 | 6.85 | 0.00 | 6.80 | 0.003 | 0.062 | `OK` |
| `t1_whiskas` | 6 | 5 | **541.1x** [451.3x-623.8x] | 0.01 | 5.92 | 0.00 | 5.85 | 0.006 | 0.087 | `OK` |
| `t1_furniture` | 2 | 3 | **320.3x** [78.7x-975.2x] | 0.02 | 7.04 | 0.01 | 7.00 | 0.003 | 0.057 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **111.9x** [42.5x-320.2x] | 0.08 | 8.31 | 0.07 | 8.23 | 0.008 | 0.099 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **860.8x** [595.2x-1156.2x] | 0.01 | 5.75 | 0.00 | 5.71 | 0.005 | 0.050 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **109.1x** [79.5x-146.5x] | 0.08 | 8.77 | 0.04 | 8.55 | 0.029 | 0.250 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **62.7x** [36.4x-92.3x] | 0.49 | 31.69 | 0.46 | 31.49 | 0.027 | 0.259 | `OK` |
| `t2_mkp_30` | 30 | 6 | **7.7x** [4.1x-11.9x] | 2.52 | 25.93 | 2.50 | 25.79 | 0.020 | 0.186 | `OK` |
| `t2_mkp_50` | 50 | 8 | **3.9x** [1.6x-32.5x] | 6.35 | 15.44 | 6.32 | 15.23 | 0.033 | 0.302 | `OK` |
| `t2_cflp_36` | 36 | 11 | **56.5x** [25.7x-145.6x] | 0.16 | 8.41 | 0.13 | 8.26 | 0.019 | 0.192 | `OK` |
| `t2_cflp_49` | 49 | 13 | **33.4x** [21.0x-95.3x] | 0.30 | 9.28 | 0.27 | 9.09 | 0.029 | 0.235 | `OK` |
| `t2_gap_45` | 45 | 13 | **19.3x** [2.8x-59.2x] | 0.91 | 13.05 | 0.88 | 12.87 | 0.023 | 0.223 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **12.8x** [5.4x-30.3x] | 2.99 | 65.99 | 2.96 | 65.78 | 0.029 | 0.270 | `OK` |
| `t2_binpack_44` | 44 | 14 | **10.9x** [0.1x-40.9x] | 0.50 | 12.47 | 0.48 | 12.30 | 0.024 | 0.218 | `OK` |
| `t2_setcover_50` | 50 | 20 | **47.9x** [6.2x-118.1x] | 0.16 | 10.05 | 0.13 | 9.78 | 0.037 | 0.355 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **62.6x** [56.8x-72.5x] | 0.14 | 8.84 | 0.11 | 8.57 | 0.033 | 0.336 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **45.3x** [33.5x-58.0x] | 0.21 | 9.15 | 0.14 | 8.77 | 0.052 | 0.491 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **29.8x** [24.2x-35.4x] | 0.34 | 10.36 | 0.26 | 9.79 | 0.078 | 0.749 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.7x** [2.3x-28.7x] | 2.65 | 45.68 | 2.62 | 45.42 | 0.037 | 0.343 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **6.9x** [1.4x-26.4x] | 15.04 | 163.45 | 14.96 | 163.02 | 0.059 | 0.540 | `OK` |
| `t3_mkp_200` | 200 | 8 | **2.7x** [2.1x-3.2x] | 37.82 | 112.16 | 37.73 | 111.57 | 0.091 | 0.778 | `OK` |
| `t3_cflp_300` | 300 | 34 | **35.4x** [7.9x-77.3x] | 5.77 | 242.23 | 5.65 | 241.34 | 0.111 | 1.174 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **3.2x** [2.3x-5.2x] | 63.71 | 153.18 | 63.50 | 151.37 | 0.234 | 2.239 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **10.8x** [5.1x-16.3x] | 7.31 | 77.29 | 6.80 | 74.44 | 0.358 | 3.414 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **26.6x** [17.0x-34.0x] | 0.53 | 16.74 | 0.40 | 15.70 | 0.139 | 1.289 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **15.7x** [8.9x-20.7x] | 0.99 | 16.96 | 0.76 | 15.25 | 0.249 | 2.208 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.8x** [1.1x-13.9x] | 8.33 | 58.65 | 7.17 | 50.88 | 1.368 | 10.777 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **9.9x** [4.4x-23.3x] | 35.92 | 363.11 | 28.68 | 330.22 | 5.300 | 43.026 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

## 4. Scalar vs. `GOEXPERIMENT=simd` Build

Ratios are geometric means of per-trial `scalar / SIMD` solve times over the same instances, with 95% t-intervals on the log ratios. "Reachable" is a necessary condition only: the AVX2 `Dot64`/`Axpy64`/`Scale64` paths need a slice of length at least 32, which requires `n + m >= 32`.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 1 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 2 faster, 1 slower, 20 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 0.75x [0.53x-1.06x] | 0.003 | 0.004 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 0.90x [0.76x-1.07x] | 0.002 | 0.002 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 0.87x [0.63x-1.22x] | 0.004 | 0.006 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.86x [0.76x-0.97x] | 0.004 | 0.004 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 0.96x [0.87x-1.06x] | 0.004 | 0.005 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 0.91x [0.75x-1.10x] | 0.012 | 0.012 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source | 1.06x [0.91x-1.24x] | 0.073 | 0.075 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 1.02x [0.96x-1.07x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 1.10x [0.98x-1.23x] | 0.044 | 0.042 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 1.01x [0.96x-1.06x] | 0.460 | 0.445 | 11 |
| `t2_mkp_30` | 36 | possible | SIMD slower | 0.99x [0.99x-1.00x] | 2.495 | 2.497 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 0.99x [0.97x-1.01x] | 6.318 | 6.414 | 11 |
| `t2_cflp_36` | 47 | possible | no detectable effect | 1.02x [0.95x-1.11x] | 0.128 | 0.128 | 11 |
| `t2_cflp_49` | 62 | possible | no detectable effect | 0.93x [0.80x-1.09x] | 0.273 | 0.287 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 1.00x [0.95x-1.06x] | 0.883 | 0.890 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 0.99x [0.98x-1.00x] | 2.961 | 2.951 | 11 |
| `t2_binpack_44` | 58 | possible | no detectable effect | 0.97x [0.94x-1.00x] | 0.483 | 0.482 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 1.01x [0.93x-1.09x] | 0.127 | 0.142 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 0.98x [0.90x-1.05x] | 0.107 | 0.108 | 11 |
| `t2_mad_g3_i9` | 108 | possible | no detectable effect | 1.02x [0.94x-1.10x] | 0.141 | 0.136 | 11 |
| `t2_mad_g4_i12` | 156 | possible | no detectable effect | 0.97x [0.88x-1.06x] | 0.265 | 0.264 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.00x [0.98x-1.02x] | 2.616 | 2.643 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 0.99x [0.98x-1.01x] | 14.959 | 14.944 | 11 |
| `t3_mkp_200` | 208 | possible | SIMD faster | 1.07x [1.03x-1.11x] | 37.727 | 34.683 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 0.98x [0.92x-1.05x] | 5.651 | 6.298 | 5 |
| `t3_lotsizing_504` | 840 | possible | no detectable effect | 1.03x [0.98x-1.08x] | 63.504 | 58.350 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 1.04x [0.99x-1.09x] | 6.801 | 6.245 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 1.01x [0.97x-1.05x] | 0.398 | 0.391 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 1.00x [0.95x-1.05x] | 0.762 | 0.753 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 1.00x [0.95x-1.05x] | 7.169 | 6.913 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | SIMD faster | 1.10x [1.04x-1.17x] | 28.677 | 25.591 | 5 |
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
| `t1_std_lp_max` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.001 | 80.00 | 80.00 |
| `t1_std_mip` | **0.00** (1 / 0) | 0.01 (1 / 0) | 0.00 (1) | 0.01 (3) | 0.002 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.01 (3) | 0.002 | 64.95 | 58.90 |
| `t1_whiskas` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.003 | 0.52 | 0.52 |
| `t1_furniture` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.002 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.07** (1 / 8) | 0.10 (2 / 8) | 0.14 (24) | 0.22 (35) | 0.006 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.04** (1 / 0) | 0.04 (1 / 0) | 0.05 (1) | 0.54 (4) | 0.076 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.46** (2 / 0) | 0.58 (12 / 0) | 0.55 (12) | 0.79 (34) | 0.090 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.50** (11 / 9) | 2.65 (48 / 9) | 1.34 (77) | 1.22 (77) | 0.019 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **6.32** (1 / 4) | 15.18 (2 / 4) | 7.10 (132) | 6.25 (119) | 0.035 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.13** (1 / 2) | 0.21 (3 / 2) | 0.18 (7) | 0.17 (7) | 0.022 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.27** (1 / 10) | 0.55 (4 / 10) | 0.38 (9) | 0.37 (9) | 0.028 | 909.02 | 827.30 |
| `t2_gap_45` | **0.88** (1 / 9) | 0.35 (1 / 9) | 1.89 (61) | 2.05 (61) | 0.025 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **2.96** (2 / 37) | 6.09 (28 / 37) | 2.91 (187) | 3.51 (199) | 0.057 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **0.48** (2 / 8) | 0.94 (5 / 8) | 21.91 (1424) | 35.45 (2375) | 0.023 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.13** (1 / 6) | 0.33 (2 / 6) | 0.65 (6) | 0.63 (6) | 0.028 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.11** (1 / 0) | 0.68 (9 / 0) | 0.58 (9) | 0.33 (4) | 0.096 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.14** (1 / 0) | 0.13 (3 / 0) | 0.13 (3) | 2.39 (14) | 0.234 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.26** (1 / 0) | 1.94 (20 / 0) | 1.65 (20) | 5.33 (23) | 0.340 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **2.62** (21 / 0) | 2.59 (125 / 0) | 2.53 (125) | 6.10 (182) | 0.141 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **14.96** (220 / 0) | 35.86 (191 / 0) | 35.83 (191) | 123.68 (975) | 0.270 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **37.73** (448 / 16) | 83.49 (1144 / 16) | 177.89 (5000, `NodeLimit`) | 164.81 (5000, `NodeLimit`) | 0.115 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **5.65** (1 / 28) | 17.84 (46 / 34) | 9.21 (134) | 9.65 (134) | 0.164 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **63.50** (2 / 206) | 80.73 (376 / 206) | 250.03 (3380, `TimeLimit`) | 250.04 (3538, `TimeLimit`) | 0.701 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **6.80** (32 / 55) | 20.99 (87 / 53) | 250.06 (2209, `TimeLimit`) | 250.09 (2220, `TimeLimit`) | 0.202 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.40** (1 / 0) | 1.98 (10 / 0) | 1.64 (10) | 0.51 (1) | 0.635 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.76** (1 / 0) | 6.02 (44 / 0) | 5.45 (44) | 13.22 (74) | 1.515 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.17** (2 / 0) | 22.23 (25 / 0) | 20.94 (25) | 250.44 (146, `TimeLimit`) | 2.631 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **28.68** (1 / 0) | 32.36 (2 / 0) | 30.89 (2) | 238.61 (49) | 57.863 | 208000.00 | 208000.00 |
<!-- END GENERATED:FULL_CHECKPOINT_TABLE -->

## 6. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus (`testdata/assign_corpus/manifest.json`; 11 randomized trials per family for Tiers 1-2 and 5 for Tier 3, 200 instances in total) compares pure-Go `assign.LinearSumFlat` against SciPy's C++ `rectangular_lsap` extension in warm state on bit-identical `float64` cost matrices, timing `linear_sum_assignment` without post-solve NumPy array gather overhead.

### 6.1 Aggregate LSAP Tier Summary

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `SciPy Solve / goblin` GeoMean [Clustered 95% CI] | Pooled Trial `SciPy Solve / goblin` | Equal-Family `SciPy Total / goblin` | `goblin` Solve Trial (Med) Sum (ms) | SciPy Solve Trial (Med) Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **2.03x** [1.89x-2.17x] | 2.03x | 6.21x | 0.165 (0.0137) | 0.323 (0.0294) | 2.08x [1.10x-4.05x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **3.33x** [2.18x-5.08x] | 3.33x | 3.30x | 9.393 (0.741) | 32.971 (2.955) | 3.05x [0.60x-19.82x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **3.24x** [2.16x-4.88x] | 3.24x | 3.78x | 195.894 (38.595) | 898.005 (179.108) | 3.10x [1.56x-6.40x] |
| Square families ($m = n$) | 16 (146) | **3.08x** [2.33x-4.08x] | 3.03x | 4.26x | 201.661 (38.680) | 920.541 (180.163) | 2.76x [0.60x-19.82x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **2.41x** [1.93x-3.00x] | 2.30x | 3.68x | 3.791 (0.670) | 10.759 (1.929) | 2.45x [1.04x-3.43x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.51x** [1.36x-4.65x] | 2.65x | 3.14x | 34.495 (5.832) | 64.648 (10.934) | 2.08x [0.60x-19.82x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.88x** [2.34x-3.55x] | 2.81x | 4.10x | 205.452 (39.350) | 931.300 (182.093) | 2.61x [0.60x-19.82x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 6.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **2.15x** | 0.0004 | 0.0009 | 0.0005 | 0.0118 | $0.0004 \pm 0.0001$ | $0.0009 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **2.04x** | 0.0013 | 0.0023 | 0.0015 | 0.0102 | $0.0012 \pm 0.0004$ | $0.0023 \pm 0.0004$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **2.21x** | 0.0024 | 0.0063 | 0.0034 | 0.0153 | $0.0030 \pm 0.0011$ | $0.0064 \pm 0.0020$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.91x** | 0.0022 | 0.0043 | 0.0031 | 0.0149 | $0.0023 \pm 0.0006$ | $0.0043 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.85x** | 0.0026 | 0.0046 | 0.0035 | 0.0156 | $0.0026 \pm 0.0006$ | $0.0048 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **2.02x** | 0.0047 | 0.0110 | 0.0077 | 0.0320 | $0.0055 \pm 0.0021$ | $0.0106 \pm 0.0019$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **3.07x** | 0.0236 | 0.0698 | 0.0297 | 0.0957 | $0.0229 \pm 0.0056$ | $0.0688 \pm 0.0104$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **4.26x** | 0.0482 | 0.183 | 0.0630 | 0.234 | $0.0469 \pm 0.0047$ | $0.216 \pm 0.120$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.02x** | 0.0425 | 0.0934 | 0.0656 | 0.144 | $0.0493 \pm 0.0152$ | $0.0964 \pm 0.0131$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.74x** | 0.129 | 0.219 | 0.137 | 0.255 | $0.139 \pm 0.0460$ | $0.235 \pm 0.0472$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **11.19x** | 0.0496 | 0.689 | 0.0647 | 0.765 | $0.147 \pm 0.334$ | $0.687 \pm 0.0187$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **2.47x** | 0.0276 | 0.0710 | 0.0522 | 0.143 | $0.0299 \pm 0.0113$ | $0.0710 \pm 0.0079$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.56x** | 0.0314 | 0.0797 | 0.0539 | 0.152 | $0.0313 \pm 0.0037$ | $0.0798 \pm 0.0073$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.81x** | 0.0749 | 0.280 | 0.101 | 0.355 | $0.0744 \pm 0.0079$ | $0.284 \pm 0.0317$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **4.05x** | 0.315 | 1.271 | 0.407 | 1.658 | $0.313 \pm 0.0510$ | $1.260 \pm 0.120$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.41x** | 1.145 | 5.229 | 1.520 | 7.585 | $1.212 \pm 0.164$ | $5.321 \pm 0.383$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **2.10x** | 0.714 | 1.479 | 1.311 | 3.935 | $0.706 \pm 0.0218$ | $1.482 \pm 0.0542$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.71x** | 4.891 | 8.436 | 5.095 | 9.773 | $5.437 \pm 1.928$ | $9.169 \pm 2.652$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **3.16x** | 0.270 | 0.819 | 0.644 | 3.500 | $0.266 \pm 0.0196$ | $0.840 \pm 0.0609$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.77x** | 0.336 | 0.951 | 0.728 | 3.611 | $0.347 \pm 0.0176$ | $0.960 \pm 0.0320$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **5.24x** | 5.473 | 27.714 | 7.304 | 34.743 | $5.383 \pm 0.262$ | $28.259 \pm 2.294$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **5.21x** | 25.766 | 134.479 | 34.702 | 177.775 | $25.828 \pm 3.749$ | $133.570 \pm 8.626$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 6.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSlice64`, `InitReducedRow64`, and `Min2ReducedRow64`)

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` and `rows * cols < 256`, no AVX2 kernel runs): 0 of 2 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32` or `AllAbsLE64` at `rows * cols >= 256`): 13 faster, 0 slower, 7 with no detectable effect, out of 20.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source | 1.04x [0.88x-1.22x] | 0.0004 | 0.0004 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source | 1.10x [0.85x-1.43x] | 0.0013 | 0.0011 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | possible | no detectable effect | 0.83x [0.57x-1.22x] | 0.0024 | 0.0037 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | no detectable effect | 1.00x [0.77x-1.30x] | 0.0022 | 0.0022 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | no detectable effect | 1.00x [0.77x-1.30x] | 0.0026 | 0.0026 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | no detectable effect | 0.97x [0.69x-1.38x] | 0.0047 | 0.0058 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.26x [1.16x-1.37x] | 0.0236 | 0.0195 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.38x [1.30x-1.48x] | 0.0482 | 0.0338 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.51x [1.26x-1.80x] | 0.0425 | 0.0285 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.62x [1.44x-1.82x] | 0.129 | 0.0811 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | no detectable effect | 1.75x [0.92x-3.35x] | 0.0496 | 0.0344 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | no detectable effect | 1.24x [0.92x-1.68x] | 0.0276 | 0.0207 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | no detectable effect | 1.18x [0.98x-1.43x] | 0.0314 | 0.0255 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.43x [1.25x-1.64x] | 0.0749 | 0.0473 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.58x [1.47x-1.70x] | 0.315 | 0.174 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.72x [1.52x-1.94x] | 1.145 | 0.722 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.96x [1.81x-2.13x] | 0.714 | 0.364 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.57x [2.46x-2.70x] | 4.891 | 1.822 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.84x [1.47x-2.29x] | 0.270 | 0.150 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.65x [1.40x-1.95x] | 0.336 | 0.199 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.61x [1.43x-1.81x] | 5.473 | 3.338 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.58x [1.50x-1.66x] | 25.766 | 15.997 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->
