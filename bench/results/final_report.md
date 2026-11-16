# `goblin` vs. Python (`PuLP + COIN-OR CBC`) Benchmark Report

## 1. Experimental Setup and Provenance

```bash
go run ./cmd/goblin-bench -check-docs
```

- **CPU**: `13th Gen Intel(R) Core(TM) i9-13900K` (x86_64, 30 logical CPUs exposed by `lscpu`, AVX2 + FMA).
- **Default pure-Go backend**: `goblin` default (`BackendGo`, scalar kernels or `WithoutSIMD()`). All headline `goblin (Go)` timings use this setup.
- **Experimental SIMD build**: `GOEXPERIMENT=simd`, which dispatches AVX2 kernels for slices of length 32 or more; see section 4.
- **Optional C++ backend**: `gb.WithCBC()` (`BackendCBC`), driving in-process COIN-OR CBC/CLP through `cgo` under the same unified `goblin` API.
- **Python baseline**: Python 3.14 + `PuLP` + `COIN-OR CBC` subprocess (`threads=1`). Its solve time includes MPS writing, process spawn, and `.sol` parsing.
- **Corpus**: 31 deterministic LP and MILP families across 3 tiers (`testdata/corpus/*.json` and `testdata/corpus/trials/*.jsonl`; 11 randomized trial instances per family for Tiers 1-2 and 5 for Tier 3, 293 instances in total), measured in warm state.

<!-- BEGIN GENERATED:PROVENANCE -->
- **goblin telemetry source**: fingerprint `fabcfbe970b8684b` (commit `2cc6f5ec22cbdb3b87bd475899ea4276b91959de`), `go1.27.3-X:nodwarf5`; the documentation gate rejects telemetry whose fingerprint differs from the checked-in sources.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, CBC `2.10.3` (`cbc`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 1.53 ms median (`prob.solve()` on a 1-variable MILP); the "Net of Spawn" columns subtract it.
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `master` (commit `c1c171ec`, vendored in `third_party/Cbc`), linked in-process through `cgo` (`gb.WithCBC()`). The C++ backend maps default settings onto CBC with two deliberate differences: `MaxCutPasses = 12` becomes `passC = 30` and `MaxNodes = 50000` becomes unlimited. It is therefore not a like-for-like setup and not the CBC 2.10.3 binary used by the Python baseline.
<!-- END GENERATED:PROVENANCE -->

## 2. Aggregate Workload Tier Summary

The planted-gap MAD family is constructed so that a root heuristic closes the gap at node 1; the bold row excludes it.

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families | `Py / goblin (Go)` GeoMean | `Py net / goblin (Go)` | `goblin (Go) / CBC` | `goblin (Go)` (ms) | `goblin (CBC)` (ms) | Py Total (ms) | Py Net (ms) | `Py / goblin (Go)` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical (`N <= 26`) | 10 | **66.4x** | 2.1x | 0.07x | 0.04 | 0.54 | 2.50 | 0.08 | 80.6x [23.2x-182.0x] |
| Tier 2: Target p90 (`30 <= N <= 84`) | 13 | **10.6x** | 8.1x | 0.17x | 1.05 | 6.24 | 11.03 | 8.43 | 10.8x [2.3x-22.8x] |
| Tier 3: Large Tail (`145 <= N <= 4036`) | 8 | **5.2x** | 5.0x | 0.28x | 11.45 | 40.65 | 59.90 | 56.86 | 6.7x [1.2x-15.3x] |
| Tier 3 non-MAD MILPs | 4 | **3.9x** | 3.8x | 0.42x | 23.78 | 56.56 | 92.48 | 90.63 | 5.1x [1.2x-15.3x] |
| Planted-gap MAD family (zero integrality gap by construction) | 8 | **10.0x** | 7.5x | 0.15x | 1.27 | 8.32 | 12.66 | 9.57 | 10.4x [5.3x-23.2x] |
| Random-weight MAD (negative controls) | 3 | **14.1x** | 13.6x | 0.17x | 4.19 | 25.19 | 58.88 | 56.80 | 11.2x [6.8x-36.7x] |
| **All families except planted-gap MAD** | 23 | **18.8x** | 3.9x | 0.14x | 0.53 | 3.73 | 9.93 | 2.07 | 15.3x [1.2x-182.0x] |
| All corpus families | 31 | **15.9x** | 4.6x | 0.14x | 0.66 | 4.59 | 10.57 | 3.08 | 11.3x [1.2x-182.0x] |
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
| `t1_std_lp` | 3 | 3 | **89.3x** | 0.02 | 1.34 | 0.00 | 1.30 | 0.00 | 0.012 | 0.056 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **87.5x** | 0.02 | 1.34 | 0.00 | 1.26 | 0.00 | 0.012 | 0.056 | `OK` |
| `t1_std_mip` | 3 | 3 | **98.7x** | 0.02 | 2.01 | 0.01 | 1.96 | 0.43 | 0.014 | 0.059 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **108.6x** | 0.02 | 1.91 | 0.01 | 1.87 | 0.33 | 0.012 | 0.057 | `OK` |
| `t1_whiskas` | 6 | 5 | **52.5x** | 0.03 | 1.39 | 0.00 | 1.33 | 0.00 | 0.021 | 0.082 | `OK` |
| `t1_furniture` | 2 | 3 | **73.7x** | 0.03 | 2.23 | 0.02 | 2.18 | 0.65 | 0.011 | 0.057 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **33.2x** | 0.10 | 3.21 | 0.07 | 3.14 | 1.61 | 0.032 | 0.092 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **182.0x** | 0.01 | 1.45 | 0.00 | 1.42 | 0.00 | 0.007 | 0.046 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **23.2x** | 0.16 | 3.82 | 0.06 | 3.63 | 2.10 | 0.109 | 0.246 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **36.7x** | 0.67 | 24.75 | 0.59 | 24.53 | 22.99 | 0.094 | 0.281 | `OK` |
| `t2_mkp_30` | 30 | 6 | **10.8x** | 2.57 | 27.67 | 2.50 | 27.50 | 25.97 | 0.071 | 0.208 | `OK` |
| `t2_mkp_50` | 50 | 8 | **2.3x** | 4.93 | 11.47 | 4.77 | 11.23 | 9.69 | 0.110 | 0.324 | `OK` |
| `t2_cflp_36` | 36 | 11 | **18.2x** | 0.20 | 3.56 | 0.14 | 3.39 | 1.86 | 0.059 | 0.191 | `OK` |
| `t2_cflp_49` | 49 | 13 | **10.0x** | 0.42 | 4.15 | 0.31 | 3.96 | 2.42 | 0.077 | 0.259 | `OK` |
| `t2_gap_45` | 45 | 13 | **10.6x** | 0.80 | 8.48 | 0.70 | 8.28 | 6.75 | 0.088 | 0.241 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **9.3x** | 4.25 | 39.67 | 4.16 | 39.44 | 37.91 | 0.093 | 0.290 | `OK` |
| `t2_binpack_44` | 44 | 14 | **15.1x** | 0.45 | 6.73 | 0.37 | 6.57 | 5.04 | 0.074 | 0.212 | `OK` |
| `t2_setcover_50` | 50 | 20 | **22.8x** | 0.24 | 5.37 | 0.11 | 5.11 | 3.57 | 0.130 | 0.353 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **11.3x** | 0.30 | 3.43 | 0.13 | 3.18 | 1.65 | 0.150 | 0.329 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **14.6x** | 0.29 | 4.21 | 0.11 | 3.83 | 2.30 | 0.175 | 0.486 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **10.3x** | 0.51 | 5.28 | 0.22 | 4.71 | 3.17 | 0.278 | 0.740 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **11.2x** | 3.86 | 43.22 | 3.73 | 42.91 | 41.38 | 0.129 | 0.382 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **6.8x** | 28.19 | 190.87 | 27.97 | 190.46 | 188.93 | 0.217 | 0.542 | `OK` |
| `t3_mkp_200` | 200 | 8 | **1.2x** | 33.23 | 41.23 | 32.95 | 40.62 | 39.09 | 0.280 | 0.782 | `OK` |
| `t3_cflp_300` | 300 | 34 | **15.3x** | 13.79 | 211.02 | 13.37 | 210.13 | 208.60 | 0.430 | 1.147 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **1.4x** | 94.14 | 128.49 | 92.95 | 126.64 | 125.11 | 0.877 | 2.268 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **8.8x** | 7.41 | 65.45 | 6.17 | 62.22 | 60.69 | 1.265 | 3.667 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **10.6x** | 0.93 | 9.85 | 0.42 | 8.88 | 7.35 | 0.509 | 1.308 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **6.0x** | 2.10 | 12.68 | 0.89 | 10.99 | 9.45 | 1.066 | 2.243 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.3x** | 8.85 | 46.51 | 4.46 | 37.92 | 36.39 | 3.868 | 10.812 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **7.3x** | 53.11 | 389.97 | 34.79 | 345.93 | 344.40 | 15.540 | 44.695 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

## 4. Scalar vs. `GOEXPERIMENT=simd` Build

Ratios are geometric means of per-trial `scalar / SIMD` solve times over the same instances, with 95% t-intervals on the log ratios. "Reachable" is a necessary condition only: the AVX2 `Dot64`/`Axpy64`/`Scale64` paths need a slice of length at least 32, which requires `n + m >= 32`.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 2 of 8 have a 95% interval excluding 1. These differences come from the two binaries (compiler output and code layout differ under `GOEXPERIMENT=simd`) and from run-to-run effects, not from SIMD kernels. Intervals of similar size on the AVX2-capable instances are therefore not evidence of a SIMD effect.
- **Instances where AVX2 can run**: 2 faster, 0 slower, 21 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 0.98x [0.92x-1.03x] | 0.002 | 0.002 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 1.02x [0.93x-1.13x] | 0.002 | 0.002 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.30x [1.04x-1.62x] | 0.008 | 0.006 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.05x [1.00x-1.10x] | 0.005 | 0.005 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 0.97x [0.93x-1.02x] | 0.004 | 0.004 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 0.90x [0.78x-1.04x] | 0.017 | 0.016 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source | 1.00x [0.97x-1.03x] | 0.065 | 0.066 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 0.99x [0.95x-1.04x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 0.89x [0.62x-1.28x] | 0.057 | 0.059 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 1.05x [0.98x-1.12x] | 0.588 | 0.547 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 1.01x [0.96x-1.06x] | 2.502 | 2.445 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 1.02x [0.99x-1.06x] | 4.773 | 4.694 | 11 |
| `t2_cflp_36` | 47 | possible | no detectable effect | 1.07x [0.94x-1.22x] | 0.138 | 0.138 | 11 |
| `t2_cflp_49` | 62 | possible | no detectable effect | 0.92x [0.82x-1.03x] | 0.310 | 0.307 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 1.03x [0.98x-1.09x] | 0.700 | 0.690 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 0.96x [0.90x-1.02x] | 4.162 | 4.654 | 11 |
| `t2_binpack_44` | 58 | possible | no detectable effect | 0.99x [0.95x-1.02x] | 0.372 | 0.366 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 1.07x [0.99x-1.17x] | 0.108 | 0.092 | 11 |
| `t2_mad_g2_i8` | 72 | possible | SIMD faster | 1.41x [1.12x-1.77x] | 0.133 | 0.087 | 11 |
| `t2_mad_g3_i9` | 108 | possible | no detectable effect | 0.82x [0.63x-1.07x] | 0.106 | 0.139 | 11 |
| `t2_mad_g4_i12` | 156 | possible | no detectable effect | 0.92x [0.81x-1.05x] | 0.221 | 0.265 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.02x [0.99x-1.05x] | 3.727 | 3.573 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 1.03x [0.97x-1.08x] | 27.968 | 28.553 | 11 |
| `t3_mkp_200` | 208 | possible | no detectable effect | 1.01x [0.98x-1.05x] | 32.954 | 31.514 | 5 |
| `t3_cflp_300` | 334 | possible | SIMD faster | 1.02x [1.01x-1.04x] | 13.365 | 13.026 | 5 |
| `t3_lotsizing_504` | 840 | possible | no detectable effect | 1.01x [0.99x-1.03x] | 92.950 | 94.124 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 0.98x [0.87x-1.10x] | 6.174 | 6.260 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 0.94x [0.68x-1.31x] | 0.418 | 0.464 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 1.02x [0.97x-1.08x] | 0.886 | 0.823 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 1.04x [0.94x-1.14x] | 4.458 | 4.386 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | no detectable effect | 1.03x [0.99x-1.07x] | 34.794 | 34.345 | 5 |
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
| `t1_std_lp` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.001 | 54.00 | 54.00 |
| `t1_std_lp_max` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.001 | 80.00 | 80.00 |
| `t1_std_mip` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.002 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.00 (3) | 0.001 | 64.95 | 58.90 |
| `t1_whiskas` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.02 (1) | 0.01 (1) | 0.002 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.07** (1 / 8) | 0.07 (2 / 8) | 0.09 (24) | 0.13 (31) | 0.005 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (5) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.06** (1 / 0) | 0.03 (1 / 0) | 0.04 (1) | 0.28 (3) | 0.058 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.59** (5 / 0) | 0.64 (12 / 0) | 0.43 (12) | 0.76 (84) | 0.065 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.50** (29 / 8) | 2.90 (60 / 8) | 0.86 (77) | 0.84 (77) | 0.018 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **4.77** (1 / 4) | 4.04 (2 / 4) | 3.88 (132) | 3.89 (119) | 0.028 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.14** (1 / 2) | 1.19 (3 / 2) | 0.14 (7) | 0.14 (7) | 0.020 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.31** (1 / 20) | 1.45 (4 / 20) | 0.27 (9) | 0.33 (9) | 0.024 | 909.02 | 827.30 |
| `t2_gap_45` | **0.70** (1 / 8) | 0.22 (1 / 8) | 1.59 (61) | 1.52 (61) | 0.020 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **4.16** (3 / 35) | 5.14 (33 / 35) | 1.88 (198) | 2.16 (174) | 0.048 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **0.37** (1 / 5) | 0.84 (13 / 5) | 13.53 (1838) | 24.12 (2450) | 0.021 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.11** (1 / 6) | 0.20 (2 / 6) | 0.77 (9) | 0.72 (9) | 0.031 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.13** (1 / 0) | 0.39 (9 / 0) | 0.35 (9) | 0.17 (2) | 0.078 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.11** (1 / 0) | 0.09 (1 / 0) | 0.10 (1) | 1.15 (136) | 0.195 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.22** (1 / 0) | 1.37 (15 / 0) | 1.33 (15) | 5.08 (47) | 0.237 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.73** (43 / 0) | 1.99 (41 / 0) | 1.93 (41) | 3.10 (126) | 0.101 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **27.97** (137 / 0) | 24.27 (201 / 0) | 10.91 (201) | 249.27 (2064) | 0.212 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **32.95** (594 / 14) | 40.38 (701 / 14) | 85.50 (5000, `NodeLimit`) | 67.62 (5000, `NodeLimit`) | 0.105 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **13.37** (48 / 31) | 18.91 (63 / 31) | 5.90 (134) | 5.95 (134) | 0.126 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **92.95** (27 / 203) | 109.69 (416 / 203) | 220.96 (5000, `NodeLimit`) | 199.59 (5000, `NodeLimit`) | 0.829 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **6.17** (35 / 47) | 9.82 (96 / 47) | 213.18 (5000, `NodeLimit`) | 153.69 (5000, `NodeLimit`) | 0.116 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.42** (1 / 0) | 1.72 (14 / 0) | 1.74 (14) | 0.84 (1) | 0.904 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.89** (1 / 0) | 3.01 (12 / 0) | 2.29 (12) | 5.64 (27) | 1.246 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **4.46** (1 / 0) | 47.22 (130 / 0) | 46.19 (130) | 185.18 (270) | 13.832 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **34.79** (1 / 0) | 39.27 (2 / 0) | 37.33 (2) | 250.56 (170, `TimeLimit`) | 100.322 | 208000.00 | 208000.00 |
<!-- END GENERATED:FULL_CHECKPOINT_TABLE -->

## 6. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family Linear Sum Assignment corpus (`testdata/assign_corpus/manifest.json`; 11 randomized trials per family for Tiers 1-2 and 5 for Tier 3, 200 instances in total) compares pure-Go `assign.LinearSumFlat` against SciPy's C++ `rectangular_lsap` extension in warm state on bit-identical `float64` cost matrices.

### 6.1 Aggregate LSAP Tier Summary

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families | `SciPy Solve / goblin` GeoMean | `SciPy Total / goblin` GeoMean | `goblin` Solve (ms) | SciPy Solve (ms) | `goblin` Total (ms) | SciPy Total (ms) | `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 | **2.42x** | 5.58x | 0.0020 | 0.0047 | 0.0028 | 0.0156 | 1.97x [1.67x-5.73x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 | **2.74x** | 2.75x | 0.0669 | 0.183 | 0.0946 | 0.260 | 2.29x [1.41x-11.77x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 | **2.65x** | 2.79x | 2.097 | 5.553 | 3.207 | 8.946 | 2.68x [1.45x-3.89x] |
| Square families ($m = n$) | 16 | **2.84x** | 3.52x | 0.106 | 0.302 | 0.143 | 0.504 | 2.84x [1.41x-11.77x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 | **2.11x** | 2.94x | 0.0316 | 0.0667 | 0.0562 | 0.165 | 2.10x [1.67x-2.68x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 | **2.26x** | 2.64x | 0.0874 | 0.198 | 0.120 | 0.318 | 1.86x [1.41x-11.77x] |
| **All 22 LSAP corpus families** | 22 | **2.62x** | 3.35x | 0.0764 | 0.200 | 0.111 | 0.372 | 2.30x [1.41x-11.77x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 6.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | `SciPy Solve / goblin` | GoSolve (ms) | SciPySolve (ms) | GoTotal (ms) | SciPyTotal (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Optimal Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **5.73x** | 0.0003 | 0.0019 | 0.0004 | 0.0122 | $0.0003 \pm 0.0001$ | $0.0020 \pm 0.0003$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **3.06x** | 0.0010 | 0.0032 | 0.0013 | 0.0106 | $0.0011 \pm 0.0003$ | $0.0032 \pm 0.0004$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **2.03x** | 0.0035 | 0.0070 | 0.0050 | 0.0157 | $0.0036 \pm 0.0012$ | $0.0071 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.92x** | 0.0024 | 0.0047 | 0.0039 | 0.0143 | $0.0028 \pm 0.0015$ | $0.0047 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.67x** | 0.0030 | 0.0050 | 0.0045 | 0.0152 | $0.0031 \pm 0.0009$ | $0.0051 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.74x** | 0.0065 | 0.0113 | 0.0108 | 0.0321 | $0.0088 \pm 0.0056$ | $0.0112 \pm 0.0018$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.29x** | 0.0294 | 0.0673 | 0.0383 | 0.0915 | $0.0279 \pm 0.0061$ | $0.0664 \pm 0.0100$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **2.78x** | 0.0601 | 0.167 | 0.0757 | 0.218 | $0.0629 \pm 0.0130$ | $0.168 \pm 0.0258$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **1.91x** | 0.0472 | 0.0902 | 0.0693 | 0.135 | $0.0446 \pm 0.0047$ | $0.0929 \pm 0.0137$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.41x** | 0.149 | 0.210 | 0.157 | 0.249 | $0.161 \pm 0.0570$ | $0.227 \pm 0.0413$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **11.77x** | 0.0549 | 0.647 | 0.0718 | 0.722 | $0.0504 \pm 0.0083$ | $0.654 \pm 0.0259$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **2.19x** | 0.0303 | 0.0664 | 0.0548 | 0.135 | $0.0310 \pm 0.0033$ | $0.0685 \pm 0.0072$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.02x** | 0.0380 | 0.0769 | 0.0646 | 0.148 | $0.0383 \pm 0.0031$ | $0.0791 \pm 0.0090$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **2.89x** | 0.0946 | 0.273 | 0.138 | 0.355 | $0.0943 \pm 0.0085$ | $0.279 \pm 0.0320$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.33x** | 0.363 | 1.208 | 0.545 | 1.565 | $0.369 \pm 0.0438$ | $1.216 \pm 0.127$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.69x** | 1.373 | 5.065 | 2.032 | 6.730 | $1.492 \pm 0.221$ | $5.136 \pm 0.361$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.86x** | 0.797 | 1.481 | 1.382 | 2.800 | $0.798 \pm 0.0432$ | $1.474 \pm 0.0683$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.45x** | 5.640 | 8.160 | 6.246 | 9.560 | $5.724 \pm 1.864$ | $8.896 \pm 2.588$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.68x** | 0.294 | 0.787 | 0.653 | 2.142 | $0.291 \pm 0.0231$ | $0.800 \pm 0.0535$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.32x** | 0.410 | 0.949 | 0.787 | 2.179 | $0.445 \pm 0.0751$ | $0.948 \pm 0.0520$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **3.82x** | 7.101 | 27.110 | 8.700 | 33.193 | $6.779 \pm 0.508$ | $27.297 \pm 2.163$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **3.89x** | 33.783 | 131.378 | 44.429 | 164.230 | $33.512 \pm 2.068$ | $131.911 \pm 8.430$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 6.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSlice64`, `InitReducedRow64`, and `Min2ReducedRow64`)

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32`, no AVX2 row-reduction kernel runs): 0 of 4 have a 95% interval excluding 1.
- **Instances where AVX2 row kernels run** (`max(rows, cols) >= 32`): 14 faster, 0 slower, 4 with no detectable effect, out of 18.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 row kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source | 0.79x [0.53x-1.17x] | 0.0003 | 0.0003 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source | 0.97x [0.94x-1.01x] | 0.0010 | 0.0011 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source | 1.04x [0.96x-1.12x] | 0.0035 | 0.0034 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | no detectable effect | 1.08x [0.88x-1.33x] | 0.0024 | 0.0025 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | SIMD faster | 1.15x [1.08x-1.24x] | 0.0030 | 0.0026 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | no (control) | identical source | 1.25x [0.88x-1.79x] | 0.0065 | 0.0064 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.10x [1.04x-1.16x] | 0.0294 | 0.0273 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.31x [1.17x-1.46x] | 0.0601 | 0.0487 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.36x [1.28x-1.44x] | 0.0472 | 0.0362 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.20x [1.10x-1.32x] | 0.149 | 0.125 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.07x [1.04x-1.11x] | 0.0549 | 0.0493 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | no detectable effect | 1.19x [0.98x-1.44x] | 0.0303 | 0.0262 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | no detectable effect | 1.03x [0.98x-1.09x] | 0.0380 | 0.0374 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.17x [1.15x-1.19x] | 0.0946 | 0.0803 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.35x [1.30x-1.40x] | 0.363 | 0.268 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.45x [1.36x-1.55x] | 1.373 | 0.993 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | no detectable effect | 1.18x [0.68x-2.06x] | 0.797 | 0.554 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.16x [1.04x-1.30x] | 5.640 | 4.416 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.77x [1.48x-2.12x] | 0.294 | 0.166 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.15x [1.05x-1.27x] | 0.410 | 0.363 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.36x [1.29x-1.44x] | 7.101 | 4.937 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.26x [1.23x-1.30x] | 33.783 | 26.084 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->
