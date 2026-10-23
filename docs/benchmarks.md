# Benchmark Methodology and Corpus Results

This document describes the benchmark protocol and results for pure-Go `gopt`, Python `PuLP` driving the `COIN-OR CBC 2.10.3` command-line binary, and [`goblin`](../../goblin), which calls a `CBC` build in-process through `cgo`. Every number in this document comes from a generated block rendered by [`bench/docgen.go`](../bench/docgen.go) from the JSON telemetry in `bench/results/`; the prose deliberately contains no measured values. [`doc_sync_test.go`](../doc_sync_test.go) fails `go test ./...` when a block drifts from the telemetry, when a parity check fails, or when telemetry lacks provenance.

---

## 1. Environment, Provenance, and Protocol

- **CPU**: `13th Gen Intel(R) Core(TM) i9-13900K` (`x86_64`, 30 logical CPUs exposed by `lscpu`, AVX2 + FMA).
- **Default build**: `go build` with the pure-Go kernels. All headline timings use this build.
- **Experimental build**: `GOEXPERIMENT=simd`, compared to the default build only in section 5.

<!-- BEGIN GENERATED:PROVENANCE -->
- **gopt telemetry source**: fingerprint `dfc05e06279c1488` (commit `4118cec7487c3731bdf8ced77d263ec3ffdef785`), `go1.27.1-X:nodwarf5`; the documentation gate rejects telemetry whose fingerprint differs from the checked-in sources
- **CPU affinity**: gopt `2`; Python + CBC `2`
- **Python reference**: PuLP `4.2.0`, CBC `2.10.3` (`cbc`)
- **CBC subprocess spawn floor**: 2.09 ms median (`prob.solve()` on a 1-variable MILP); the "Net of Spawn" columns subtract it
- **goblin**: COIN-OR CBC/CLP `master` (commit `c1c171ec`, built in `../Cbc`), linked in-process through `cgo`. goblin maps gopt's default options onto CBC with two deliberate differences: `MaxCutPasses = 12` becomes `passC = 30` and `MaxNodes = 50000` becomes unlimited. It is therefore not a like-for-like configuration and not the CBC 2.10.3 binary used by the Python baseline.
<!-- END GENERATED:PROVENANCE -->

- **Single-threaded, sequential, pinned execution**: every solve runs single-threaded (`threads=1` in CBC, one goroutine in `gopt.SolveCompiled`), all harnesses run with `workers=1`, and each harness is pinned with `taskset` to one performance core. The i9-13900K mixes 5.5 GHz P-cores (CPUs 0-13) with 4.3 GHz E-cores (CPUs 14-29); unpinned runs migrate between them and produced spurious 2x differences between builds on models where no AVX2 kernel runs. The recorded affinity appears in the provenance block.
- **Warm state**: each harness runs an untimed worker warmup (`_init_worker` in [`bench/python_baseline.py`](../bench/python_baseline.py), `warmWorker` in [`cmd/gopt-bench/main.go`](../cmd/gopt-bench/main.go)), then 2 untimed warmup solves per family on Tiers 1-2 and 1 on Tier 3.
- **Randomized trials**: each family in `testdata/corpus/trials/<id>.jsonl` holds 11 seeded instances (Tiers 1-2) or 5 (Tier 3), 293 instances in total. Trial $k$ is the same instance in every harness, which enables paired comparisons.
- **Metrics**: per trial, preparation time (`t_parse + t_build + t_format/t_compile`), solve time (`t_solve`), and end-to-end time (`t_total`). Tables report medians; dispersion tables report mean $\pm$ SD with two-sided 95% Student-$t$ intervals.
- **CBC spawn floor**: the Python harness also times `prob.solve()` on a 1-variable MILP. That fixed cost (MPS write, process spawn, CBC start-up, `.sol` parsing) is subtracted in the "Net of Spawn" columns to separate interface overhead from solver work.
- **Parity**: `bench.CheckParity` requires both gopt builds, and goblin, to match CBC's status, canonical objective, and every per-trial objective within a relative tolerance of 1e-5. `TestAllTrialParity` re-solves all 293 instances and additionally checks each gopt solution for feasibility.

---

## 2. Aggregate Workload Summary

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families | Py Total GeoMean (ms) | Py Net-of-Spawn GeoMean (ms) | goblin GeoMean (ms) | gopt GeoMean (ms) | `Py / gopt` GeoMean | `Py / gopt` Median [Min-Max] | `Py net / gopt` GeoMean | `gopt / goblin` |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical (`N <= 22`) | 10 | 3.86 | 0.38 | 1.23 | 0.04 | **98.7x** | 164.1x [6.9x-307.4x] | 9.8x | 0.03x |
| Tier 2: Target p90 (`30 <= N <= 76`) | 13 | 14.37 | 11.37 | 25.63 | 2.10 | **6.8x** | 9.2x [1.5x-23.7x] | 5.4x | 0.08x |
| Tier 3: Large Tail (`135 <= N <= 4028`) | 8 | 101.99 | 97.98 | 133.67 | 39.47 | **2.6x** | 2.6x [0.4x-13.0x] | 2.5x | 0.30x |
| Tier 3 non-MAD MILPs | 4 | 120.68 | 118.18 | 214.01 | 23.18 | **5.2x** | 8.0x [1.2x-13.0x] | 5.1x | 0.11x |
| Planted-gap MAD family (zero integrality gap by construction) | 8 | 23.99 | 19.16 | 24.24 | 6.85 | **3.5x** | 3.1x [0.4x-18.7x] | 2.8x | 0.28x |
| Random-weight MAD (negative controls) | 3 | 44.33 | 40.97 | 73.38 | 16.76 | **2.6x** | 1.8x [1.5x-6.9x] | 2.4x | 0.23x |
| **All families except planted-gap MAD** | 23 | 13.42 | 4.59 | 12.39 | 0.68 | **19.7x** | 13.0x [1.2x-307.4x] | 6.7x | 0.06x |
| All corpus families | 31 | 15.59 | 6.64 | 14.73 | 1.24 | **12.6x** | 12.0x [0.4x-307.4x] | 5.4x | 0.08x |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

How to read this table:
1. **Small models are dominated by interface cost.** For Tier 1, most of the Python time is the CBC spawn floor. Compare the "Py net / gopt" column with "Py / gopt" to see how much of the ratio is process overhead rather than solver speed.
2. **The planted-gap MAD family is not representative.** Its instances have a zero integrality gap by construction, and `tryRootSubMIP` closes them at node 1. The bold row excludes that family and is the fairer aggregate.
3. **goblin is not a like-for-like CBC baseline.** It links a different CBC build with different options (see the provenance block), so the `gopt / goblin` column compares configurations, not engines.
4. **Large degenerate LPs and PFI updates.** `gopt` factorizes the basis with a three-stage singleton and sparse Markowitz LU, `FTRANSparse`/`BTRANSparse`, and dual bound flipping (`selectDualBFRT`), but updates the basis with a product-form (PFI) eta file rather than Forrest-Tomlin $U$-updates and lacks matrix equilibration; see [`solver-internals.md`](solver-internals.md#known-limitations).

---

## 3. MAD Group-Item Allocation Formulation (`G` Groups $\times$ `I` Items)

The Mean Absolute Deviation (`MAD`) allocation family ([`buildMADAllocation`](../bench/corpus.go)) models multi-period balanced allocation across $G$ groups, $I$ items, and $T = 6$ time periods.

### Decision Variables
- $x_{g,i} \in \{0, 1\}$ for $g \in \{0, \dots, G-1\}$ and $i \in \{0, \dots, I-1\}$: Binary indicator equal to $1$ if item $i$ is assigned to group $g$.
- $z_{g,t} \ge 0$ for $g \in \{0, \dots, G-1\}$ and $t \in \{0, \dots, T-1\}$: Continuous absolute deviation of group $g$'s aggregate weight in period $t$ from its central level $m_g$.
- $m_g \ge 0$ for $g \in \{0, \dots, G-1\}$: Continuous central weight level for group $g$.

Total variables: $N = G \cdot I + G \cdot T + G = G(I + T + 1)$.

### Objective and Constraints
$$\min \sum_{g=0}^{G-1} \sum_{t=0}^{T-1} z_{g,t}$$

subject to:
1. **Linearized Absolute Deviation** ($2 \cdot G \cdot T$ rows):
   $$z_{g,t} - \sum_{i=0}^{I-1} W_{t,i} x_{g,i} + m_g \ge 0 \quad \forall g, t$$
   $$z_{g,t} + \sum_{i=0}^{I-1} W_{t,i} x_{g,i} - m_g \ge 0 \quad \forall g, t$$
2. **Item Partitioning** ($I$ equality rows):
   $$\sum_{g=0}^{G-1} x_{g,i} = 1 \quad \forall i \in \{0, \dots, I-1\}$$
3. **Designated Subset Assignment** ($G$ equality rows over $D = \{I-G, \dots, I-1\}$):
   $$\sum_{i \in D} x_{g,i} = 1 \quad \forall g \in \{0, \dots, G-1\}$$

Total constraints: $M = 2 \cdot G \cdot T + I + G$.
### Structural Disclosure: Planted-Gap Instances vs. Random-Weight Negative Controls

In [`buildMADAllocation`](../bench/corpus.go), only the $G$ designated items $i \in D = \{I-G, \dots, I-1\}$ carry nonzero period weights ($\pm 360$ in two periods per item); the remaining $I - G$ items have $W_{t,i} = 0$ and appear only in the partitioning constraints $\sum_{g} x_{g,i} = 1$. Consequently:
- Every structured instance `t*_mad_g*` has a **zero integrality gap** (`RootLPBound == OptimalObj`). The LP relaxation leaves the designated binaries at $1/G$; `tryRootSubMIP` fixes the integral zero-weight items and solves the remaining $G \times G$ permutation sub-MIP at node 1.
- [`BuildMADRandom`](../bench/corpus.go) (`t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`, and `TestRandomMADNegativeControls` in [`solver_test.go`](../solver_test.go)) assigns dense pseudorandom weights $W_{t,i} \in [100, 2000)$ to every item. These negative controls have a strict integrality gap, the root heuristic does not close it, and the solver must branch.

### MAD Results (`G` Groups $\times$ `I` Items, $T = 6$)

<!-- BEGIN GENERATED:MAD_PIPELINE_TABLE -->
| Problem ID | $G$ | $I$ | $T$ | Vars | Rows | Optimal Obj | PyPrep (ms) | GoPrep (ms) | PySolve (ms) | PySolve Net of Spawn (ms) | GoSolve (ms) | PyTotal (ms) | GoTotal (ms) | `Py / gopt` | Parity |
| :--- | --: | ---: | --: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_mad_g2_i4` | 2 | 4 | 6 | 22 | 30 | 2160.0 | 0.291 | 0.133 | 5.23 | 3.13 | 0.17 | 5.45 | 0.29 | **18.7x** | `OK` |
| `t1_mad_rand_g2_i4` | 2 | 4 | 6 | 22 | 32 | 5249.0 | 0.443 | 0.179 | 34.90 | 32.80 | 4.96 | 35.23 | 5.14 | **6.9x** | `OK` |
| `t2_mad_g2_i8` | 2 | 8 | 6 | 30 | 34 | 2160.0 | 0.626 | 0.196 | 4.94 | 2.85 | 0.16 | 5.48 | 0.36 | **15.0x** | `OK` |
| `t2_mad_g3_i9` | 3 | 9 | 6 | 48 | 48 | 4320.0 | 0.961 | 0.323 | 7.57 | 5.48 | 0.53 | 8.33 | 0.84 | **9.9x** | `OK` |
| `t2_mad_g4_i12` | 4 | 12 | 6 | 76 | 64 | 7200.0 | 1.298 | 0.568 | 7.32 | 5.23 | 2.19 | 7.99 | 2.68 | **3.0x** | `OK` |
| `t2_mad_rand_g2_i8` | 2 | 8 | 6 | 30 | 36 | 6696.0 | 0.692 | 0.150 | 13.37 | 11.28 | 7.56 | 13.93 | 7.77 | **1.8x** | `OK` |
| `t2_mad_rand_g3_i9` | 3 | 9 | 6 | 48 | 51 | 7958.0 | 0.599 | 0.328 | 177.04 | 174.95 | 117.55 | 177.47 | 117.89 | **1.5x** | `OK` |
| `t3_mad_g5_i20` | 5 | 20 | 6 | 135 | 85 | 10800.0 | 2.720 | 0.551 | 12.85 | 10.76 | 4.26 | 14.90 | 4.78 | **3.1x** | `OK` |
| `t3_mad_g6_i30` | 6 | 30 | 6 | 222 | 108 | 15120.0 | 2.982 | 1.362 | 28.08 | 25.99 | 13.40 | 30.12 | 14.51 | **2.1x** | `OK` |
| `t3_mad_g10_i100` | 10 | 100 | 6 | 1070 | 230 | 79200.0 | 16.389 | 7.261 | 183.31 | 181.21 | 464.65 | 195.76 | 471.57 | **0.4x** | `OK` |
| `t3_mad_g4_i1000` | 4 | 1000 | 6 | 4028 | 1052 | 187200.0 | 77.944 | 25.815 | 560.97 | 558.88 | 599.24 | 628.55 | 624.69 | **1.0x** | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Multi-Trial Latency Dispersion (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `gopt` Scalar Total (Mean $\pm$ SD [95% CI], ms) | `gopt` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | $5.14 \pm 1.05$ ($\pm 0.70$) | $0.33 \pm 0.08$ ($\pm 0.06$) | $0.32 \pm 0.07$ ($\pm 0.05$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | $28.09 \pm 14.34$ ($\pm 9.63$) | $4.97 \pm 3.15$ ($\pm 2.12$) | $4.11 \pm 2.62$ ($\pm 1.76$) |
| `t2_mad_g2_i8` | $2 \times 8$ | $5.63 \pm 1.12$ ($\pm 0.75$) | $0.47 \pm 0.29$ ($\pm 0.20$) | $0.30 \pm 0.07$ ($\pm 0.05$) |
| `t2_mad_g3_i9` | $3 \times 9$ | $7.66 \pm 1.10$ ($\pm 0.74$) | $0.91 \pm 0.10$ ($\pm 0.07$) | $0.89 \pm 0.06$ ($\pm 0.04$) |
| `t2_mad_g4_i12` | $4 \times 12$ | $8.24 \pm 0.88$ ($\pm 0.59$) | $2.99 \pm 0.89$ ($\pm 0.60$) | $2.22 \pm 0.46$ ($\pm 0.31$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | $34.80 \pm 30.02$ ($\pm 20.17$) | $5.61 \pm 4.64$ ($\pm 3.12$) | $7.39 \pm 6.07$ ($\pm 4.08$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | $175.93 \pm 35.71$ ($\pm 23.99$) | $121.24 \pm 19.00$ ($\pm 12.76$) | $130.50 \pm 22.58$ ($\pm 15.17$) |
| `t3_mad_g5_i20` | $5 \times 20$ | $14.21 \pm 1.40$ ($\pm 1.74$) | $5.21 \pm 1.89$ ($\pm 2.34$) | $5.48 \pm 0.86$ ($\pm 1.07$) |
| `t3_mad_g6_i30` | $6 \times 30$ | $35.69 \pm 23.51$ ($\pm 29.19$) | $16.29 \pm 2.78$ ($\pm 3.45$) | $14.66 \pm 2.87$ ($\pm 3.56$) |
| `t3_mad_g10_i100` | $10 \times 100$ | $231.54 \pm 90.64$ ($\pm 112.53$) | $991.69 \pm 1253.82$ ($\pm 1556.57$) | $1002.61 \pm 1233.68$ ($\pm 1531.57$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | $608.55 \pm 62.25$ ($\pm 77.29$) | $664.29 \pm 88.47$ ($\pm 109.84$) | $647.87 \pm 77.61$ ($\pm 96.35$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | Root LP Obj | Optimal Obj | Root LP (ms) | CP4 Base (ms / nodes) | CP5 +Presolve (ms / nodes) | CP6 +Cuts (ms / nodes / cuts) | CP7 Full (ms / nodes / cuts) |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | 2160.00 | 2160.00 | 0.101 | 0.40 (4) | 0.39 (4) | 0.51 (4 / 2) | **0.17** (1 / 0) |
| `t1_mad_rand_g2_i4` | 4257.00 | 5249.00 | 0.093 | 0.63 (9) | 0.75 (9) | 0.88 (11 / 4) | **4.96** (11 / 4) |
| `t2_mad_g2_i8` | 2160.00 | 2160.00 | 0.094 | 0.55 (6) | 0.53 (7) | 0.82 (6 / 2) | **0.16** (1 / 0) |
| `t2_mad_g3_i9` | 4320.00 | 4320.00 | 0.388 | 1.98 (8) | 1.96 (18) | 2.06 (13 / 2) | **0.53** (1 / 0) |
| `t2_mad_g4_i12` | 7200.00 | 7200.00 | 1.155 | 5.62 (30) | 4.11 (12) | 4.05 (10 / 7) | **2.19** (1 / 0) |
| `t2_mad_rand_g2_i8` | 5894.00 | 6696.00 | 0.107 | 1.85 (31) | 1.19 (33) | 2.29 (32 / 5) | **7.56** (35 / 5) |
| `t2_mad_rand_g3_i9` | 6060.00 | 7958.00 | 0.235 | 84.05 (181) | 71.43 (364) | 68.07 (262 / 11) | **117.55** (339 / 11) |
| `t3_mad_g5_i20` | 10800.00 | 10800.00 | 5.700 | 34.69 (91) | 8.13 (47) | 7.14 (19 / 4) | **4.26** (1 / 0) |
| `t3_mad_g6_i30` | 15120.00 | 15120.00 | 11.144 | 133.70 (266) | 51.86 (154) | 29.74 (32 / 4) | **13.40** (1 / 0) |
| `t3_mad_g10_i100` | 79200.00 | 79200.00 | 180.597 | 250.44 (3, `TimeLimit`) | 250.03 (2, `TimeLimit`) | 250.10 (2 / 10, `TimeLimit`) | **464.65** (1 / 0) |
| `t3_mad_g4_i1000` | 187200.00 | 187200.00 | 544.282 | 252.18 (0, `TimeLimit`) | 251.72 (0, `TimeLimit`) | 250.02 (0 / 0, `TimeLimit`) | **599.24** (1 / 0) |
<!-- END GENERATED:MAD_CHECKPOINT_TABLE -->

---

## 4. Full Corpus End-to-End Comparison

The corpus spans 31 families in three tiers:
- **Tier 1 (10 families)**: canonical `PuLP` unit models (`t1_std_lp`, `t1_std_lp_max`, `t1_std_mip`, `t1_mip_float_obj`, `t1_whiskas`, `t1_furniture`, `t1_knapsack_12`, `t1_infeasible_mip`), `t1_mad_g2_i4`, and `t1_mad_rand_g2_i4`.
- **Tier 2 (13 families)**: 30 to 76 variables: multidimensional knapsack, capacitated facility location, generalized assignment, multi-item lot sizing, bin packing, set cover, three planted-gap MAD and two random-weight MAD families.
- **Tier 3 (8 families)**: 135 to 4,028 variables: `t3_mkp_200`, `t3_cflp_300`, `t3_lotsizing_504`, `t3_sched_1000`, and four planted-gap MAD families.

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | PyPrep (ms) | GoPrep (ms) | PySolve (ms) | PySolve Net of Spawn (ms) | GoSolve (ms) | PyTotal (ms) | GoTotal (ms) | `Py / gopt` | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | 0.087 | 0.008 | 1.86 | 0.00 | 0.00 | 1.92 | 0.01 | **184.3x** | `OK` |
| `t1_std_lp_max` | 3 | 3 | 0.091 | 0.008 | 1.73 | 0.00 | 0.00 | 1.79 | 0.01 | **184.8x** | `OK` |
| `t1_std_mip` | 3 | 3 | 0.122 | 0.008 | 3.35 | 1.25 | 0.00 | 3.45 | 0.01 | **301.7x** | `OK` |
| `t1_mip_float_obj` | 3 | 3 | 0.129 | 0.008 | 3.21 | 1.12 | 0.00 | 3.30 | 0.01 | **307.4x** | `OK` |
| `t1_whiskas` | 6 | 5 | 0.184 | 0.017 | 2.76 | 0.67 | 0.00 | 2.91 | 0.02 | **136.4x** | `OK` |
| `t1_furniture` | 2 | 3 | 0.087 | 0.008 | 2.92 | 0.83 | 0.01 | 2.98 | 0.02 | **143.8x** | `OK` |
| `t1_knapsack_12` | 12 | 3 | 0.142 | 0.028 | 4.87 | 2.77 | 0.08 | 4.98 | 0.11 | **46.2x** | `OK` |
| `t1_infeasible_mip` | 2 | 1 | 0.095 | 0.008 | 2.21 | 0.11 | 0.00 | 2.27 | 0.01 | **239.7x** | `OK` |
| `t1_mad_g2_i4` | 22 | 30 | 0.291 | 0.133 | 5.23 | 3.13 | 0.17 | 5.45 | 0.29 | **18.7x** | `OK` |
| `t1_mad_rand_g2_i4` | 22 | 32 | 0.443 | 0.179 | 34.90 | 32.80 | 4.96 | 35.23 | 5.14 | **6.9x** | `OK` |
| `t2_mkp_30` | 30 | 6 | 0.377 | 0.141 | 37.88 | 35.79 | 5.16 | 38.23 | 5.43 | **7.0x** | `OK` |
| `t2_mkp_50` | 50 | 8 | 0.585 | 0.222 | 14.52 | 12.43 | 8.70 | 14.82 | 8.92 | **1.7x** | `OK` |
| `t2_cflp_36` | 36 | 11 | 0.420 | 0.125 | 6.08 | 3.98 | 0.33 | 6.47 | 0.46 | **14.1x** | `OK` |
| `t2_cflp_49` | 49 | 13 | 0.491 | 0.102 | 6.66 | 4.57 | 0.46 | 7.09 | 0.59 | **12.0x** | `OK` |
| `t2_gap_45` | 45 | 13 | 0.345 | 0.138 | 10.44 | 8.35 | 1.04 | 10.81 | 1.18 | **9.2x** | `OK` |
| `t2_lotsizing_48` | 48 | 32 | 0.550 | 0.183 | 58.84 | 56.74 | 6.54 | 59.24 | 6.71 | **8.8x** | `OK` |
| `t2_binpack_44` | 44 | 14 | 0.473 | 0.079 | 10.99 | 8.89 | 0.41 | 11.38 | 0.48 | **23.7x** | `OK` |
| `t2_setcover_50` | 50 | 20 | 0.418 | 0.239 | 6.20 | 4.11 | 0.16 | 6.51 | 0.41 | **15.8x** | `OK` |
| `t2_mad_g2_i8` | 30 | 34 | 0.626 | 0.196 | 4.94 | 2.85 | 0.16 | 5.48 | 0.36 | **15.0x** | `OK` |
| `t2_mad_g3_i9` | 48 | 48 | 0.961 | 0.323 | 7.57 | 5.48 | 0.53 | 8.33 | 0.84 | **9.9x** | `OK` |
| `t2_mad_g4_i12` | 76 | 64 | 1.298 | 0.568 | 7.32 | 5.23 | 2.19 | 7.99 | 2.68 | **3.0x** | `OK` |
| `t2_mad_rand_g2_i8` | 30 | 36 | 0.692 | 0.150 | 13.37 | 11.28 | 7.56 | 13.93 | 7.77 | **1.8x** | `OK` |
| `t2_mad_rand_g3_i9` | 48 | 51 | 0.599 | 0.328 | 177.04 | 174.95 | 117.55 | 177.47 | 117.89 | **1.5x** | `OK` |
| `t3_mkp_200` | 200 | 8 | 1.071 | 0.497 | 51.71 | 49.62 | 43.55 | 52.38 | 43.84 | **1.2x** | `OK` |
| `t3_cflp_300` | 300 | 34 | 1.362 | 0.787 | 259.76 | 257.67 | 21.03 | 261.58 | 21.81 | **12.0x** | `OK` |
| `t3_lotsizing_504` | 504 | 336 | 2.514 | 0.844 | 153.28 | 151.18 | 38.44 | 155.39 | 39.30 | **4.0x** | `OK` |
| `t3_sched_1000` | 1000 | 100 | 6.842 | 1.684 | 95.76 | 93.67 | 6.22 | 99.61 | 7.68 | **13.0x** | `OK` |
| `t3_mad_g5_i20` | 135 | 85 | 2.720 | 0.551 | 12.85 | 10.76 | 4.26 | 14.90 | 4.78 | **3.1x** | `OK` |
| `t3_mad_g6_i30` | 222 | 108 | 2.982 | 1.362 | 28.08 | 25.99 | 13.40 | 30.12 | 14.51 | **2.1x** | `OK` |
| `t3_mad_g10_i100` | 1070 | 230 | 16.389 | 7.261 | 183.31 | 181.21 | 464.65 | 195.76 | 471.57 | **0.4x** | `OK` |
| `t3_mad_g4_i1000` | 4028 | 1052 | 77.944 | 25.815 | 560.97 | 558.88 | 599.24 | 628.55 | 624.69 | **1.0x** | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## 5. Default Build vs. `GOEXPERIMENT=simd`

Each ratio is the geometric mean of per-trial `scalar / SIMD` solve times over the same instances, with a 95% $t$-interval on the log ratios; "no detectable effect" means the interval contains 1. "Reachable" is only a necessary condition: the AVX2 `Dot64`/`Axpy64` paths need a slice of length 256 or more, which requires `n + m >= 256`. Below that size no AVX2 kernel can run, so those instances are negative controls: an interval there that excludes 1 measures differences between the two binaries or between runs, not SIMD.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 256`, no AVX2 kernel can run): 13 of 25 have a 95% interval excluding 1. These differences come from the two binaries (compiler output and code layout differ under `GOEXPERIMENT=simd`) and from run-to-run effects, not from SIMD kernels. Intervals of similar size on the AVX2-capable instances are therefore not evidence of a SIMD effect.
- **Instances where AVX2 can run**: 0 faster, 1 slower, 5 with no detectable effect, out of 6.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Scalar Solve Median (ms) | SIMD Solve Median (ms) | Paired `scalar / SIMD` GeoMean [95% CI] | Trials | Verdict |
| :--- | ---: | :---: | ---: | ---: | ---: | ---: | :--- |
| `t1_std_lp` | 6 | no (control) | 0.002 | 0.002 | 0.95x [0.87x-1.05x] | 11 | identical source |
| `t1_std_lp_max` | 6 | no (control) | 0.001 | 0.001 | 1.07x [1.02x-1.13x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t1_std_mip` | 6 | no (control) | 0.003 | 0.003 | 0.83x [0.67x-1.02x] | 11 | identical source |
| `t1_mip_float_obj` | 6 | no (control) | 0.003 | 0.003 | 0.78x [0.64x-0.95x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t1_whiskas` | 11 | no (control) | 0.003 | 0.004 | 0.71x [0.67x-0.75x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t1_furniture` | 5 | no (control) | 0.011 | 0.015 | 0.69x [0.63x-0.75x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t1_knapsack_12` | 15 | no (control) | 0.076 | 0.092 | 0.78x [0.72x-0.84x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t1_infeasible_mip` | 3 | no (control) | 0.000 | 0.000 | 0.78x [0.56x-1.10x] | 11 | identical source |
| `t1_mad_g2_i4` | 52 | no (control) | 0.170 | 0.179 | 1.04x [0.82x-1.32x] | 11 | identical source |
| `t1_mad_rand_g2_i4` | 54 | no (control) | 4.957 | 4.331 | 1.19x [1.07x-1.33x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t2_mkp_30` | 36 | no (control) | 5.164 | 3.441 | 1.47x [1.31x-1.65x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t2_mkp_50` | 58 | no (control) | 8.697 | 5.444 | 1.58x [1.45x-1.72x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t2_cflp_36` | 47 | no (control) | 0.328 | 0.279 | 1.07x [0.95x-1.20x] | 11 | identical source |
| `t2_cflp_49` | 62 | no (control) | 0.459 | 0.572 | 0.85x [0.79x-0.92x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t2_gap_45` | 58 | no (control) | 1.043 | 0.920 | 0.96x [0.84x-1.10x] | 11 | identical source |
| `t2_lotsizing_48` | 80 | no (control) | 6.535 | 4.375 | 1.28x [1.11x-1.48x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t2_binpack_44` | 58 | no (control) | 0.408 | 0.552 | 0.86x [0.74x-1.00x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t2_setcover_50` | 70 | no (control) | 0.161 | 0.203 | 0.93x [0.74x-1.18x] | 11 | identical source |
| `t2_mad_g2_i8` | 64 | no (control) | 0.162 | 0.136 | 1.50x [0.98x-2.30x] | 11 | identical source |
| `t2_mad_g3_i9` | 96 | no (control) | 0.529 | 0.533 | 1.01x [0.96x-1.06x] | 11 | identical source |
| `t2_mad_g4_i12` | 140 | no (control) | 2.187 | 1.790 | 1.32x [1.17x-1.49x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t2_mad_rand_g2_i8` | 66 | no (control) | 7.562 | 10.164 | 0.77x [0.71x-0.83x] | 11 | identical source; interval excludes 1 (build/run effect) |
| `t2_mad_rand_g3_i9` | 99 | no (control) | 117.549 | 128.359 | 0.93x [0.82x-1.05x] | 11 | identical source |
| `t3_mkp_200` | 208 | no (control) | 43.551 | 37.777 | 1.01x [0.74x-1.36x] | 5 | identical source |
| `t3_cflp_300` | 334 | possible | 21.027 | 19.816 | 0.97x [0.78x-1.20x] | 5 | no detectable effect |
| `t3_lotsizing_504` | 840 | possible | 38.439 | 47.310 | 0.73x [0.57x-0.92x] | 5 | SIMD slower |
| `t3_sched_1000` | 1100 | possible | 6.223 | 5.206 | 1.07x [0.78x-1.45x] | 5 | no detectable effect |
| `t3_mad_g5_i20` | 220 | no (control) | 4.256 | 5.187 | 0.92x [0.69x-1.23x] | 5 | identical source |
| `t3_mad_g6_i30` | 330 | possible | 13.396 | 12.400 | 1.12x [0.96x-1.29x] | 5 | no detectable effect |
| `t3_mad_g10_i100` | 1300 | possible | 464.647 | 484.760 | 0.97x [0.91x-1.04x] | 5 | no detectable effect |
| `t3_mad_g4_i1000` | 5080 | possible | 599.243 | 610.371 | 1.02x [0.96x-1.07x] | 5 | no detectable effect |
<!-- END GENERATED:SIMD_EFFECT_TABLE -->

---

## 6. Full Corpus Solver-Core Checkpoint Progression

<!-- BEGIN GENERATED:FULL_CHECKPOINT_TABLE -->
| Problem ID | Root LP Obj | Optimal Obj | Root LP (ms) | CP4 Base (ms / nodes) | CP5 +Presolve (ms / nodes) | CP6 +Cuts (ms / nodes / cuts) | CP7 Full (ms / nodes / cuts) |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 54.00 | 54.00 | 0.001 | 0.00 (1) | 0.00 (1) | 0.00 (1 / 0) | **0.00** (1 / 0) |
| `t1_std_lp_max` | 80.00 | 80.00 | 0.001 | 0.00 (1) | 0.00 (1) | 0.00 (1 / 0) | **0.00** (1 / 0) |
| `t1_std_mip` | 58.00 | 64.00 | 0.001 | 0.00 (3) | 0.00 (1) | 0.00 (1 / 0) | **0.00** (1 / 0) |
| `t1_mip_float_obj` | 58.90 | 64.95 | 0.002 | 0.01 (3) | 0.00 (1) | 0.00 (1 / 0) | **0.00** (1 / 0) |
| `t1_whiskas` | 0.52 | 0.52 | 0.002 | 0.00 (1) | 0.00 (1) | 0.00 (1 / 0) | **0.00** (1 / 0) |
| `t1_furniture` | 1075.00 | 1075.00 | 0.001 | 0.01 (1) | 0.01 (1) | 0.01 (1 / 0) | **0.01** (1 / 0) |
| `t1_knapsack_12` | 578.65 | 513.24 | 0.005 | 0.11 (31) | 0.12 (24) | 0.12 (2 / 15) | **0.08** (1 / 15) |
| `t1_infeasible_mip` | 1.50 | 0.00 | 0.001 | 0.01 (5) | 0.00 (0) | 0.00 (0 / 0) | **0.00** (0 / 0) |
| `t1_mad_g2_i4` | 2160.00 | 2160.00 | 0.101 | 0.40 (4) | 0.39 (4) | 0.51 (4 / 2) | **0.17** (1 / 0) |
| `t1_mad_rand_g2_i4` | 4257.00 | 5249.00 | 0.093 | 0.63 (9) | 0.75 (9) | 0.88 (11 / 4) | **4.96** (11 / 4) |
| `t2_mkp_30` | 1966.92 | 1917.24 | 0.023 | 1.14 (77) | 1.33 (77) | 2.96 (60 / 9) | **5.16** (29 / 9) |
| `t2_mkp_50` | 2648.37 | 2620.01 | 0.046 | 4.96 (119) | 6.88 (132) | 6.15 (2 / 5) | **8.70** (1 / 5) |
| `t2_cflp_36` | 781.97 | 801.17 | 0.035 | 0.22 (7) | 0.21 (7) | 1.32 (3 / 2) | **0.33** (1 / 2) |
| `t2_cflp_49` | 827.30 | 909.02 | 0.033 | 0.34 (9) | 0.34 (9) | 1.57 (4 / 20) | **0.46** (1 / 20) |
| `t2_gap_45` | 178.04 | 195.18 | 0.042 | 2.17 (61) | 2.37 (61) | 0.35 (1 / 13) | **1.04** (1 / 13) |
| `t2_lotsizing_48` | 2052.34 | 2245.81 | 0.112 | 3.17 (174) | 2.61 (198) | 6.01 (33 / 40) | **6.54** (11 / 40) |
| `t2_binpack_44` | 29.30 | 30.32 | 0.051 | 31.22 (2400) | 23.40 (1895) | 1.69 (18 / 12) | **0.41** (1 / 12) |
| `t2_setcover_50` | 56.84 | 58.10 | 0.042 | 0.96 (9) | 0.69 (9) | 0.26 (2 / 6) | **0.16** (1 / 6) |
| `t2_mad_g2_i8` | 2160.00 | 2160.00 | 0.094 | 0.55 (6) | 0.53 (7) | 0.82 (6 / 2) | **0.16** (1 / 0) |
| `t2_mad_g3_i9` | 4320.00 | 4320.00 | 0.388 | 1.98 (8) | 1.96 (18) | 2.06 (13 / 2) | **0.53** (1 / 0) |
| `t2_mad_g4_i12` | 7200.00 | 7200.00 | 1.155 | 5.62 (30) | 4.11 (12) | 4.05 (10 / 7) | **2.19** (1 / 0) |
| `t2_mad_rand_g2_i8` | 5894.00 | 6696.00 | 0.107 | 1.85 (31) | 1.19 (33) | 2.29 (32 / 5) | **7.56** (35 / 5) |
| `t2_mad_rand_g3_i9` | 6060.00 | 7958.00 | 0.235 | 84.05 (181) | 71.43 (364) | 68.07 (262 / 11) | **117.55** (339 / 11) |
| `t3_mkp_200` | 12955.46 | 12906.80 | 0.194 | 105.08 (5000, `NodeLimit`) | 127.75 (5000, `NodeLimit`) | 74.91 (739 / 21) | **43.55** (557 / 21) |
| `t3_cflp_300` | 2081.53 | 2187.30 | 0.143 | 9.54 (134) | 8.05 (134) | 16.82 (39 / 37) | **21.03** (9 / 37) |
| `t3_lotsizing_504` | 10518.99 | 13120.44 | 1.329 | 250.03 (4883, `TimeLimit`) | 250.05 (3969, `TimeLimit`) | 140.97 (365 / 165) | **38.44** (11 / 165) |
| `t3_sched_1000` | 46971.09 | 46818.28 | 0.282 | 250.05 (4976, `TimeLimit`) | 250.02 (3643, `TimeLimit`) | 19.66 (96 / 50) | **6.22** (17 / 50) |
| `t3_mad_g5_i20` | 10800.00 | 10800.00 | 5.700 | 34.69 (91) | 8.13 (47) | 7.14 (19 / 4) | **4.26** (1 / 0) |
| `t3_mad_g6_i30` | 15120.00 | 15120.00 | 11.144 | 133.70 (266) | 51.86 (154) | 29.74 (32 / 4) | **13.40** (1 / 0) |
| `t3_mad_g10_i100` | 79200.00 | 79200.00 | 180.597 | 250.44 (3, `TimeLimit`) | 250.03 (2, `TimeLimit`) | 250.10 (2 / 10, `TimeLimit`) | **464.65** (1 / 0) |
| `t3_mad_g4_i1000` | 187200.00 | 187200.00 | 544.282 | 252.18 (0, `TimeLimit`) | 251.72 (0, `TimeLimit`) | 250.02 (0 / 0, `TimeLimit`) | **599.24** (1 / 0) |
<!-- END GENERATED:FULL_CHECKPOINT_TABLE -->

---

## 7. Keeping Documentation and Benchmarks Synchronized

1. **Regenerate telemetry** from a clean, committed tree so that every entry records its `git_commit`: run the commands in the [README](../README.md#verification-benchmarking-and-documentation-sync-commands).
2. **Regenerate tables**: `go run ./cmd/gopt-bench -sync-docs`.
3. **Verify**: `go run ./cmd/gopt-bench -check-docs`, or `go test ./...`, which runs `TestDocumentationAndBenchmarkSync`. The gate validates corpus dimensions, parity, and provenance, and checks every generated block in `README.md`, `docs/benchmarks.md`, and `bench/results/final_report.md`.
