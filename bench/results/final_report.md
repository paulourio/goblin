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
- **Unified telemetry source**: fingerprint `2e436740e97c6e65` (working tree on top of commit `daa87923d9d36aed42936df0f9f5ffdad63acbf9`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `1`; Python + CBC `1`; Python + SciPy `1`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.84 ms median on a 1-variable MILP and 6.78 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

## 2. Aggregate Workload Tier Summary

Tier 1 micro-models are dominated by Python subprocess spawn overhead, and the planted-gap MAD family is constructed so that root heuristics close the gap without branching; the bold row isolates solver-dominated Tier 2 and Tier 3 non-planted MILPs across paired trial instances.

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `Py / Go` GeoMean [Clustered 95% CI] | Pooled Trial `Py / Go` | Equal-Family `Go / CBC` | `Go` Trial (Med) Sum (ms) | `CBC` Trial (Med) Sum (ms) | `Py` Trial (Med) Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **330.40x** [147.30x-741.11x] | 330.40x | 0.06x | 10.6 (0.9) | 155.4 (14.5) | 1129.5 (105.9) | 535.29x [30.76x-1224.21x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **18.74x** [11.40x-30.83x] | 18.74x | 0.18x | 571.2 (38.5) | 1753.9 (157.0) | 4506.1 (436.7) | 22.01x [1.33x-163.35x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **9.65x** [4.61x-20.18x] | 9.65x | 0.27x | 906.6 (169.7) | 2614.0 (524.3) | 6446.5 (1110.1) | 9.46x [1.04x-69.88x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **12.23x** [7.20x-20.79x] | 13.46x | 0.22x | 1147.1 (160.0) | 2984.2 (389.7) | 7056.0 (1020.2) | 12.47x [1.33x-163.35x] |
| Tier 3 non-MAD MILPs | 4 (20) | **7.27x** [1.27x-41.52x] | 7.27x | 0.30x | 585.9 (122.4) | 1293.2 (238.2) | 2904.6 (615.2) | 5.23x [1.49x-69.88x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **24.34x** [11.95x-49.60x] | 30.98x | 0.18x | 332.1 (48.3) | 1397.7 (292.7) | 4001.2 (536.4) | 34.18x [1.04x-126.48x] |
| Random-weight MAD (negative controls) | 3 (33) | **15.62x** [0.83x-293.71x] | 15.62x | 0.20x | 329.0 (23.6) | 972.5 (97.5) | 2349.5 (259.4) | 14.03x [1.33x-118.20x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **47.30x** [20.16x-111.01x] | 57.56x | 0.12x | 1156.3 (160.8) | 3125.6 (403.0) | 8081.0 (1116.4) | 42.37x [1.33x-1224.21x] |
| All 31 synthetic corpus families | 31 (293) | **39.85x** [20.87x-76.09x] | 50.27x | 0.14x | 1488.4 (209.0) | 4523.3 (695.7) | 12082.2 (1652.7) | 41.18x [1.04x-1224.21x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2.1 Pure-Go vs. In-Process C++ CBC Aggregate Summary

Comparing `BackendGo` directly against in-process `BackendCBC` removes Python MPS formatting and subprocess spawn overhead across the same five workload subsets:

<!-- BEGIN GENERATED:CBC_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `BackendCBC / BackendGo` Total [Clustered 95% CI] | Equal-Family `BackendCBC / BackendGo` Solve [Clustered 95% CI] | `BackendGo` Trial (Med) Sum (ms) | `BackendCBC` Trial (Med) Sum (ms) | Per-Instance `CBC / Go` Total Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro-models (`N <= 26`) | 10 (110) | **17.84x** [7.86x-40.50x] | **37.20x** [12.26x-112.89x] | 10.6 (0.9) | 155.4 (14.5) | 13.00x [2.08x-144.41x] |
| Tier 2: Medium MILPs (`30 <= N <= 84`) | 13 (143) | **5.55x** [3.97x-7.76x] | **6.22x** [4.22x-9.14x] | 571.2 (38.5) | 1753.9 (157.0) | 5.61x [0.56x-43.80x] |
| Tier 3: Large MILPs (`145 <= N <= 4036`) | 8 (40) | **3.75x** [2.16x-6.51x] | **4.15x** [2.27x-7.57x] | 906.6 (169.7) | 2614.0 (524.3) | 3.80x [0.38x-14.54x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **4.62x** [3.10x-6.89x] | **4.86x** [3.16x-7.49x] | 1147.1 (160.0) | 2984.2 (389.7) | 4.98x [0.56x-43.80x] |
| All 31 synthetic families | 31 (293) | **7.31x** [5.03x-10.61x] | **9.97x** [6.03x-16.50x] | 1488.4 (209.0) | 4523.3 (695.7) | 7.20x [0.38x-144.41x] |
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
| `t1_std_lp` | 3 | 3 | **789.0x** [592.9x-941.0x] | 0.01 | 6.53 | 0.00 | 6.48 | 0.004 | 0.055 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **823.8x** [416.0x-1173.5x] | 0.01 | 6.59 | 0.00 | 6.54 | 0.004 | 0.055 | `OK` |
| `t1_std_mip` | 3 | 3 | **823.9x** [610.0x-1101.4x] | 0.01 | 7.47 | 0.00 | 7.41 | 0.003 | 0.057 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **928.3x** [708.6x-1111.7x] | 0.01 | 7.79 | 0.00 | 7.73 | 0.004 | 0.064 | `OK` |
| `t1_whiskas` | 6 | 5 | **480.7x** [352.4x-636.4x] | 0.01 | 6.51 | 0.01 | 6.44 | 0.006 | 0.076 | `OK` |
| `t1_furniture` | 2 | 3 | **259.9x** [70.5x-573.2x] | 0.02 | 8.26 | 0.02 | 8.20 | 0.004 | 0.054 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **74.2x** [30.8x-161.9x] | 0.15 | 9.91 | 0.14 | 9.81 | 0.017 | 0.099 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **712.7x** [301.9x-1224.2x] | 0.01 | 7.88 | 0.00 | 7.81 | 0.009 | 0.065 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **77.9x** [44.5x-126.5x] | 0.13 | 9.75 | 0.07 | 9.55 | 0.056 | 0.217 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **60.5x** [36.8x-118.2x] | 0.55 | 35.23 | 0.52 | 34.98 | 0.030 | 0.252 | `OK` |
| `t2_mkp_30` | 30 | 6 | **10.2x** [5.0x-14.6x] | 2.48 | 29.70 | 2.46 | 29.50 | 0.020 | 0.185 | `OK` |
| `t2_mkp_50` | 50 | 8 | **4.8x** [2.5x-29.6x] | 5.56 | 18.87 | 5.53 | 18.57 | 0.038 | 0.292 | `OK` |
| `t2_cflp_36` | 36 | 11 | **53.7x** [22.0x-163.3x] | 0.19 | 9.96 | 0.16 | 9.78 | 0.021 | 0.179 | `OK` |
| `t2_cflp_49` | 49 | 13 | **29.0x** [17.3x-89.5x] | 0.44 | 10.68 | 0.39 | 10.46 | 0.039 | 0.214 | `OK` |
| `t2_gap_45` | 45 | 13 | **20.0x** [4.4x-61.7x] | 0.87 | 14.93 | 0.84 | 14.73 | 0.024 | 0.212 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **11.6x** [4.6x-31.1x] | 3.68 | 70.91 | 3.64 | 70.66 | 0.033 | 0.256 | `OK` |
| `t2_binpack_44` | 44 | 14 | **11.5x** [4.4x-20.4x] | 1.14 | 14.03 | 1.11 | 13.83 | 0.042 | 0.214 | `OK` |
| `t2_setcover_50` | 50 | 20 | **46.8x** [5.5x-125.6x] | 0.21 | 11.72 | 0.16 | 11.40 | 0.040 | 0.329 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **53.5x** [46.0x-65.7x] | 0.18 | 9.80 | 0.14 | 9.51 | 0.047 | 0.300 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **38.0x** [11.7x-50.2x] | 0.24 | 10.46 | 0.15 | 10.02 | 0.088 | 0.429 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **29.0x** [25.0x-35.5x] | 0.40 | 11.48 | 0.32 | 10.89 | 0.087 | 0.603 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.1x** [3.2x-23.6x] | 3.53 | 49.50 | 3.23 | 49.18 | 0.051 | 0.309 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **6.9x** [1.3x-28.5x] | 19.51 | 174.66 | 19.45 | 174.21 | 0.068 | 0.459 | `OK` |
| `t3_mkp_200` | 200 | 8 | **4.3x** [3.2x-5.5x] | 23.49 | 120.50 | 23.36 | 119.87 | 0.098 | 0.652 | `OK` |
| `t3_cflp_300` | 300 | 34 | **31.5x** [7.6x-69.9x] | 6.86 | 250.29 | 6.75 | 249.40 | 0.118 | 0.960 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **2.5x** [1.5x-4.2x] | 83.48 | 163.60 | 83.23 | 161.69 | 0.252 | 1.921 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **8.3x** [3.8x-12.0x] | 8.53 | 80.81 | 8.18 | 77.85 | 0.358 | 2.937 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **25.6x** [18.8x-33.9x] | 0.77 | 19.11 | 0.53 | 17.93 | 0.236 | 1.072 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **17.3x** [10.7x-22.6x] | 1.08 | 19.09 | 0.86 | 17.21 | 0.243 | 1.812 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.9x** [1.0x-14.5x] | 8.90 | 67.87 | 7.80 | 58.26 | 1.161 | 9.495 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **10.3x** [4.5x-24.5x] | 36.57 | 388.80 | 31.13 | 349.32 | 4.402 | 37.146 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

## 4. Scalar vs. `GOEXPERIMENT=simd` Build

Ratios are geometric means of per-trial `scalar / SIMD` solve times over the same instances, with 95% t-intervals on the log ratios. "Reachable" is a necessary condition only: the AVX2 `Dot64`/`Axpy64`/`Scale64` paths need a slice of length at least 32, which requires `n + m >= 32`.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 3 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 5 faster, 2 slower, 16 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 0.99x [0.80x-1.21x] | 0.004 | 0.004 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.12x [1.03x-1.21x] | 0.003 | 0.003 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 0.99x [0.95x-1.03x] | 0.005 | 0.005 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source | 0.91x [0.81x-1.01x] | 0.004 | 0.004 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 0.93x [0.81x-1.06x] | 0.007 | 0.007 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 0.95x [0.78x-1.15x] | 0.016 | 0.019 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.14x [1.07x-1.22x] | 0.137 | 0.117 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.60x [1.18x-2.18x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 1.17x [0.80x-1.70x] | 0.066 | 0.051 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 1.05x [0.97x-1.14x] | 0.521 | 0.498 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 0.97x [0.90x-1.05x] | 2.464 | 2.476 | 11 |
| `t2_mkp_50` | 58 | possible | SIMD faster | 1.08x [1.03x-1.14x] | 5.527 | 5.252 | 11 |
| `t2_cflp_36` | 47 | possible | no detectable effect | 0.98x [0.95x-1.01x] | 0.163 | 0.166 | 11 |
| `t2_cflp_49` | 62 | possible | SIMD faster | 1.12x [1.03x-1.22x] | 0.392 | 0.349 | 11 |
| `t2_gap_45` | 58 | possible | SIMD slower | 0.87x [0.79x-0.97x] | 0.844 | 0.872 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 0.99x [0.94x-1.04x] | 3.644 | 4.225 | 11 |
| `t2_binpack_44` | 58 | possible | SIMD faster | 1.12x [1.04x-1.21x] | 1.110 | 1.079 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 0.97x [0.85x-1.10x] | 0.161 | 0.200 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 0.96x [0.91x-1.01x] | 0.139 | 0.143 | 11 |
| `t2_mad_g3_i9` | 108 | possible | no detectable effect | 0.98x [0.79x-1.21x] | 0.152 | 0.167 | 11 |
| `t2_mad_g4_i12` | 156 | possible | SIMD faster | 1.04x [1.01x-1.07x] | 0.316 | 0.302 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 0.97x [0.92x-1.02x] | 3.226 | 3.358 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | SIMD slower | 0.97x [0.95x-0.99x] | 19.446 | 19.454 | 11 |
| `t3_mkp_200` | 208 | possible | no detectable effect | 1.02x [0.95x-1.11x] | 23.358 | 23.088 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 0.96x [0.82x-1.13x] | 6.747 | 6.352 | 5 |
| `t3_lotsizing_504` | 840 | possible | no detectable effect | 1.01x [0.96x-1.07x] | 83.229 | 82.724 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 1.02x [0.97x-1.07x] | 8.181 | 8.280 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 1.12x [0.99x-1.27x] | 0.528 | 0.453 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 0.95x [0.85x-1.06x] | 0.865 | 0.836 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 1.01x [0.98x-1.05x] | 7.797 | 8.016 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | SIMD faster | 1.07x [1.02x-1.12x] | 31.126 | 28.784 | 5 |
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
| `t1_whiskas` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.00 (1) | 0.004 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.003 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.14** (1 / 8) | 0.15 (2 / 8) | 0.22 (18) | 0.29 (35) | 0.010 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.002 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.07** (1 / 0) | 0.04 (1 / 0) | 0.05 (1) | 0.57 (4) | 0.087 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.52** (2 / 0) | 0.63 (12 / 0) | 0.60 (12) | 0.82 (109) | 0.111 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.46** (5 / 9) | 3.28 (40 / 9) | 1.27 (51) | 1.10 (61) | 0.029 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **5.53** (1 / 4) | 16.76 (2 / 4) | 6.17 (118) | 5.73 (111) | 0.050 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.16** (1 / 2) | 0.26 (3 / 2) | 0.18 (7) | 0.19 (7) | 0.028 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.39** (1 / 10) | 0.72 (4 / 10) | 0.34 (9) | 0.32 (9) | 0.040 | 909.02 | 827.30 |
| `t2_gap_45` | **0.84** (1 / 9) | 0.51 (1 / 9) | 2.06 (41) | 2.15 (60) | 0.030 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **3.64** (2 / 37) | 8.29 (25 / 37) | 3.39 (162) | 3.77 (199) | 0.064 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **1.11** (2 / 6) | 1.98 (5 / 6) | 9.51 (772) | 31.62 (1941) | 0.032 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.16** (1 / 6) | 0.40 (2 / 6) | 0.70 (6) | 0.67 (6) | 0.035 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.14** (1 / 0) | 0.91 (8 / 0) | 0.78 (8) | 0.78 (9) | 0.118 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.15** (1 / 0) | 0.14 (2 / 0) | 0.14 (2) | 2.87 (13) | 0.261 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.32** (1 / 0) | 3.54 (22 / 0) | 3.24 (22) | 6.61 (23) | 0.379 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.23** (43 / 0) | 3.18 (112 / 0) | 3.21 (112) | 4.08 (45) | 0.169 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **19.45** (196 / 0) | 33.53 (188 / 0) | 33.22 (188) | 98.95 (539) | 0.283 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **23.36** (250 / 14) | 112.35 (1135 / 16) | 236.86 (5000, `NodeLimit`) | 220.11 (5000, `NodeLimit`) | 0.144 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **6.75** (1 / 28) | 19.46 (54 / 34) | 9.45 (116) | 9.51 (118) | 0.178 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **83.23** (2 / 206) | 122.13 (376 / 206) | 250.04 (2780, `TimeLimit`) | 250.04 (2858, `TimeLimit`) | 0.782 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **8.18** (31 / 55) | 28.55 (87 / 53) | 250.10 (1664, `TimeLimit`) | 250.02 (1635, `TimeLimit`) | 0.224 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.53** (1 / 0) | 3.26 (17 / 0) | 2.98 (17) | 0.53 (1) | 0.705 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.86** (1 / 0) | 9.80 (58 / 0) | 8.60 (58) | 14.78 (74) | 1.628 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.80** (2 / 0) | 21.69 (25 / 0) | 20.35 (25) | 250.17 (145, `TimeLimit`) | 2.799 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **31.13** (1 / 0) | 34.09 (2 / 0) | 33.02 (2) | 250.11 (6, `TimeLimit`) | 58.692 | 208000.00 | 208000.00 |
<!-- END GENERATED:FULL_CHECKPOINT_TABLE -->

## 6. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus (`testdata/assign_corpus/manifest.json`; 11 randomized trials per family for Tiers 1-2 and 5 for Tier 3, 200 instances in total) compares pure-Go `assign.LinearSumFlat` against SciPy's C++ `rectangular_lsap` extension in warm state on bit-identical `float64` cost matrices, timing `linear_sum_assignment` without post-solve NumPy array gather overhead.

### 6.1 Aggregate LSAP Tier Summary

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `SciPy Solve / goblin` GeoMean [Clustered 95% CI] | Pooled Trial `SciPy Solve / goblin` | Equal-Family `SciPy Total / goblin` | `goblin` Solve Trial (Med) Sum (ms) | SciPy Solve Trial (Med) Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.43x** [1.23x-1.67x] | 1.43x | 4.17x | 0.230 (0.0201) | 0.331 (0.0304) | 1.46x [0.79x-2.63x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **2.92x** [1.81x-4.70x] | 2.92x | 2.79x | 9.507 (0.852) | 33.838 (3.071) | 2.51x [1.07x-15.72x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.79x** [1.83x-4.25x] | 2.79x | 3.62x | 228.003 (43.880) | 960.618 (190.933) | 2.48x [1.51x-5.73x] |
| Square families ($m = n$) | 16 (146) | **2.65x** [1.93x-3.64x] | 2.59x | 3.56x | 232.730 (43.895) | 983.645 (191.981) | 2.15x [0.79x-15.72x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.75x** [1.27x-2.42x] | 1.64x | 2.95x | 5.010 (0.857) | 11.141 (2.053) | 1.60x [0.89x-2.72x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.25x** [1.13x-4.49x] | 2.37x | 2.71x | 36.367 (6.367) | 67.141 (11.233) | 1.81x [0.79x-15.72x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.37x** [1.85x-3.03x] | 2.29x | 3.38x | 237.740 (44.752) | 994.786 (194.035) | 2.01x [0.79x-15.72x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 6.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.54x** | 0.0007 | 0.0009 | 0.0008 | 0.0130 | $0.0007 \pm 0.0001$ | $0.0010 \pm 0.0003$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.38x** | 0.0017 | 0.0026 | 0.0021 | 0.0112 | $0.0018 \pm 0.0005$ | $0.0025 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.70x** | 0.0036 | 0.0064 | 0.0053 | 0.0159 | $0.0038 \pm 0.0014$ | $0.0063 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.21x** | 0.0036 | 0.0040 | 0.0051 | 0.0147 | $0.0034 \pm 0.0006$ | $0.0041 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.20x** | 0.0038 | 0.0050 | 0.0055 | 0.0158 | $0.0040 \pm 0.0009$ | $0.0047 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.63x** | 0.0067 | 0.0115 | 0.0116 | 0.0340 | $0.0072 \pm 0.0026$ | $0.0114 \pm 0.0020$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.62x** | 0.0313 | 0.0730 | 0.0438 | 0.0993 | $0.0286 \pm 0.0082$ | $0.0728 \pm 0.0121$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.20x** | 0.0582 | 0.182 | 0.0898 | 0.235 | $0.0578 \pm 0.0053$ | $0.186 \pm 0.0274$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **1.89x** | 0.0514 | 0.102 | 0.0909 | 0.158 | $0.0554 \pm 0.0132$ | $0.103 \pm 0.0147$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.63x** | 0.141 | 0.233 | 0.168 | 0.272 | $0.152 \pm 0.0458$ | $0.243 \pm 0.0455$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **12.18x** | 0.0617 | 0.713 | 0.0825 | 0.791 | $0.0589 \pm 0.0082$ | $0.711 \pm 0.0108$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.84x** | 0.0369 | 0.0729 | 0.0789 | 0.155 | $0.0425 \pm 0.0125$ | $0.0757 \pm 0.0087$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **1.94x** | 0.0419 | 0.0843 | 0.0697 | 0.163 | $0.0442 \pm 0.0101$ | $0.0844 \pm 0.0076$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.57x** | 0.0837 | 0.291 | 0.109 | 0.380 | $0.0824 \pm 0.0093$ | $0.294 \pm 0.0317$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.82x** | 0.346 | 1.321 | 0.454 | 1.796 | $0.343 \pm 0.0450$ | $1.305 \pm 0.125$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.67x** | 1.357 | 5.407 | 2.088 | 7.981 | $1.500 \pm 0.291$ | $5.446 \pm 0.433$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.66x** | 0.916 | 1.524 | 1.478 | 4.404 | $0.909 \pm 0.0432$ | $1.512 \pm 0.0561$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.69x** | 5.187 | 8.644 | 5.832 | 9.534 | $5.755 \pm 2.079$ | $9.551 \pm 2.834$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.51x** | 0.356 | 0.881 | 0.751 | 3.820 | $0.346 \pm 0.0313$ | $0.867 \pm 0.0611$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.22x** | 0.414 | 1.006 | 0.837 | 3.910 | $0.449 \pm 0.0558$ | $0.990 \pm 0.0293$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.84x** | 5.841 | 28.686 | 7.703 | 38.136 | $6.050 \pm 0.327$ | $29.337 \pm 2.374$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.74x** | 29.809 | 144.784 | 39.429 | 197.365 | $30.592 \pm 3.548$ | $144.420 \pm 9.864$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 6.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSkip64`, `InitReducedRow64`, and `Min2ReducedRow64`)

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` with no `>= 8x8` tall transpose and either fused square `Minimize` non-sparse path or `rows * cols < 32`, so no AVX2 kernel runs): 2 of 3 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32`, tall `rows > cols` with `>= 8x8`, or fallback `AllAbsLE64`/`ValidateAndNegate64` at `rows * cols >= 32`): 19 faster, 0 slower, 0 with no detectable effect, out of 19.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.38x [1.22x-1.55x] | 0.0007 | 0.0005 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.19x [1.02x-1.38x] | 0.0017 | 0.0016 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source | 1.01x [0.94x-1.08x] | 0.0036 | 0.0036 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | SIMD faster | 1.10x [1.05x-1.14x] | 0.0036 | 0.0033 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | SIMD faster | 1.15x [1.09x-1.22x] | 0.0038 | 0.0033 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | SIMD faster | 1.04x [1.00x-1.08x] | 0.0067 | 0.0065 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.33x [1.28x-1.39x] | 0.0313 | 0.0229 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.44x [1.34x-1.54x] | 0.0582 | 0.0414 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.78x [1.37x-2.32x] | 0.0514 | 0.0301 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.62x [1.33x-1.97x] | 0.141 | 0.0871 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.26x [1.18x-1.33x] | 0.0617 | 0.0487 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.45x [1.25x-1.68x] | 0.0369 | 0.0282 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.45x [1.26x-1.67x] | 0.0419 | 0.0292 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.37x [1.21x-1.55x] | 0.0837 | 0.0556 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.65x [1.55x-1.76x] | 0.346 | 0.199 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.97x [1.41x-2.74x] | 1.357 | 0.722 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.59x [2.46x-2.72x] | 0.916 | 0.356 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.64x [2.42x-2.88x] | 5.187 | 1.851 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.82x [1.67x-1.98x] | 0.356 | 0.192 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.58x [1.35x-1.83x] | 0.414 | 0.285 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.62x [1.49x-1.77x] | 5.841 | 3.602 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.42x [1.32x-1.52x] | 29.809 | 22.445 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

## 7. Production Binary Footprint and Linker Dead-Code Elimination

Standalone `CGO_ENABLED=0` binaries compiled across the six canonical `goblin/assign`, `goblin/mad`, and `goblin` build profiles (`bench/results/goblin_binary_size.json`), verifying that unreferenced `WithCertified`, `WithVIPRTrace`, `encoding/json`, and (in `goblin/assign` and both `goblin/mad` profiles) `math/big` symbols are eliminated by the Go linker:

<!-- BEGIN GENERATED:BINARY_SIZE_TABLE -->
| Profile | Default Raw / Stripped (KiB) | `GOEXPERIMENT=simd` Raw / Stripped (KiB) | Stripped Delta vs. Default | `goblin` Text (KiB) | `math/big` Syms | `encoding/json` Syms | `VIPR` Syms | `Certif` Syms |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `assign` default (`LinearSum` + `SolveInto`) | 2822.8 / **1768.2** | 2858.1 / **1832.2** | baseline | 118.7 | `0` | `0` | `0` | `0` |
| `assign` certified (`+ assign.WithCertified(true)`) | 2942.9 / **1844.2** | 2980.7 / **1916.2** | +76.0 KiB (+4.3%) | 177.2 | `0` | `0` | `0` | `17` |
| `goblin/mad` default (`mad.Solve`, `StrategyAuto`) | 2783.9 / **1760.2** | 2714.5 / **1760.2** | baseline | 117.8 | `0` | `0` | `0` | `0` |
| `goblin/mad` certified (`+ mad.WithCertified(true)`) | 2978.2 / **1888.2** | 2900.4 / **1888.2** | +128.0 KiB (+7.3%) | 201.2 | `0` | `0` | `0` | `5` |
| `goblin` default (`Model.Solve`, `DefaultOptions`) | 5586.4 / **3560.2** | 5472.3 / **3576.2** | baseline | 1023.4 | `130` | `0` | `0` | `0` |
| `goblin` certified + VIPR (`+ WithCertified + WithVIPRTrace`) | 6831.2 / **4400.2** | 6697.1 / **4420.2** | +840.0 KiB (+23.6%) | 1503.1 | `155` | `0` | `220` | `14` |
<!-- END GENERATED:BINARY_SIZE_TABLE -->

## 8. Combinatorial Group-Item Allocation (`goblin/mad` vs. Algebraic MILP)

The 13-family combinatorial MAD corpus (`testdata/mad_corpus/manifest.json`; 131 randomized trials across the `pmad_base_g10_i2` base instance with exact Trial-0 optimum `24137`, 8 PMAD families across $G \in \{6, 10, 20\}$, $I \in \{4..12\}$, and $\lambda \in \{0.25, 1.0\}$, plus 4 random-weight controls) evaluates `gb.SolveMAD` (`StrategyAuto`, `StrategyDP`, and `StrategyBruteForce` when DFS states are at most `250,000`) against 250 ms-capped `BuildMADModel` `Model.Solve` (`BackendGo` and `BackendCBC`) with `1e-9` combinatorial parity and `[BestBound, Incumbent]` containment.

### 8.1 Aggregate Combinatorial MAD Summary

<!-- BEGIN GENERATED:MAD_COMBINATORIAL_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Auto Trial (Med) Sum (ms) | DP Trial (Med) Sum (ms) | MILP Go Trial (Med) Sum (ms) [Opt] | MILP CBC Trial (Med) Sum (ms) [Opt] | Equal-Family `MILP Go / Auto` [95% CI] | Equal-Family `MILP CBC / Auto` [95% CI] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| PMAD base (`G = 10, I = 2`) | 1 (11) | 0.265 (0.0229) | 0.108 (0.0099) | 918.8 (16.5) [8/11] | 761.4 (71.6) [11/11] | **1201.9x** | **2625.7x** |
| PMAD small & medium (`I <= 10`) | 6 (66) | 100.005 (9.016) | 98.141 (8.912) | 15762.5 (1500.7) [8/66] | 12841.4 (1172.5) [26/66] | **460.7x** [75.6x-2809.4x] | **334.2x** [81.1x-1377.4x] |
| PMAD large (`I = 12`) | 2 (10) | 308.667 (61.413) | 307.559 (61.469) | 2502.5 (500.3) [0/10] | 2551.4 (510.7) [0/10] | **8.6x** [0.1x-849.2x] | **8.8x** [0.1x-785.4x] |
| Random-weight MAD controls (`I <= 12`) | 4 (44) | 4.633 (0.419) | 4.459 (0.404) | 2318.3 (271.9) [38/44] | 3936.3 (374.4) [34/44] | **171.6x** [46.8x-629.4x] | **842.1x** [401.3x-1767.1x] |
| **All 13 combinatorial MAD families** | 13 (131) | 413.569 (70.871) | 410.267 (70.795) | 21502.0 (2289.4) [54/131] | 20090.6 (2129.1) [71/131] | **198.5x** [62.1x-634.3x] | **297.5x** [93.6x-945.4x] |
<!-- END GENERATED:MAD_COMBINATORIAL_SUMMARY_TABLE -->

### 8.2 Full 13-Family Combinatorial MAD Pipeline Comparison

<!-- BEGIN GENERATED:MAD_COMBINATORIAL_PIPELINE_TABLE -->
| Problem ID | $G \times I \times T$ | $\lambda$ | Auto Strategy | Auto Med (ms) | DP Med (ms) | DFS Med (ms) | MILP Go 250 ms | MILP CBC 250 ms | Trial 0 Obj | Parity |
| :--- | :---: | ---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `pmad_base_g10_i2` | $10 \times 2 \times 6$ | 1.00 | `DP` | 0.0229 | 0.0099 | 0.0989 | 16.48 (8/11 opt) | 71.60 (`Optimal`) | 24137.0 | `OK` |
| `pmad_g6_i6_lam025` | $6 \times 6 \times 6$ | 0.25 | `DP` | 0.100 | 0.0899 | 27.881 | 250.04 (4/11 opt) | 105.37 (`Optimal`) | 13259.5 | `OK` |
| `pmad_g6_i10_lam10` | $6 \times 10 \times 6$ | 1.00 | `DP` | 2.943 | 2.982 | skip (>250k) | 250.11 (`TimeLimit`) | 251.97 (`TimeLimit`) | 14018.0 | `OK` |
| `pmad_g6_i12_lam025` | $6 \times 12 \times 6$ | 0.25 | `DP` | 19.963 | 19.852 | skip (>250k) | 250.11 (`TimeLimit`) | 253.70 (`TimeLimit`) | 9787.0 | `OK` |
| `pmad_g10_i4_lam025` | $10 \times 4 \times 6$ | 0.25 | `DP` | 0.0499 | 0.0333 | 8.727 | 250.07 (4/11 opt) | 55.35 (`Optimal`) | 23954.0 | `OK` |
| `pmad_g10_i8_lam10` | $10 \times 8 \times 6$ | 1.00 | `DP` | 0.765 | 0.743 | skip (>250k) | 250.12 (`TimeLimit`) | 252.84 (3/11 opt) | 21928.0 | `OK` |
| `pmad_g10_i10_lam025` | $10 \times 10 \times 6$ | 0.25 | `DP` | 4.875 | 4.794 | skip (>250k) | 250.21 (`TimeLimit`) | 252.67 (1/11 opt) | 22570.0 | `OK` |
| `pmad_g20_i6_lam10` | $20 \times 6 \times 6$ | 1.00 | `DP` | 0.283 | 0.270 | skip (>250k) | 250.18 (`TimeLimit`) | 254.26 (`TimeLimit`) | 43530.0 | `OK` |
| `pmad_g20_i12_lam10` | $20 \times 12 \times 6$ | 1.00 | `DP` | 41.451 | 41.617 | skip (>250k) | 250.21 (`TimeLimit`) | 256.98 (`TimeLimit`) | 38256.0 | `OK` |
| `mad_comb_rand_g2_i4` | $2 \times 4 \times 6$ | 0.00 | `BruteForce` | 0.0071 | 0.0039 | 0.0023 | 0.53 (`Optimal`) | 11.16 (`Optimal`) | 4601.0 | `OK` |
| `mad_comb_rand_g2_i8` | $2 \times 8 \times 6$ | 0.00 | `BruteForce` | 0.0255 | 0.0310 | 0.0186 | 3.19 (`Optimal`) | 12.55 (`Optimal`) | 5975.0 | `OK` |
| `mad_comb_rand_g3_i9` | $3 \times 9 \times 6$ | 0.00 | `DP` | 0.0532 | 0.0458 | 0.261 | 18.10 (`Optimal`) | 99.67 (`Optimal`) | 7720.0 | `OK` |
| `mad_comb_rand_g4_i12` | $4 \times 12 \times 6$ | 0.00 | `DP` | 0.333 | 0.323 | 27.843 | 250.04 (5/11 opt) | 251.02 (1/11 opt) | 7905.0 | `OK` |
<!-- END GENERATED:MAD_COMBINATORIAL_PIPELINE_TABLE -->
