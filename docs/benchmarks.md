# Benchmark Methodology and Corpus Results

This document describes the benchmark protocol and results for pure-Go `goblin` (`BackendGo`), Python `PuLP` driving the `COIN-OR CBC 2.10.3` command-line binary, and `goblin (CBC)` (`BackendCBC`), which calls a `CBC` build in-process through `cgo`. Every number in this document comes from a generated block rendered by [`bench/docgen.go`](../bench/docgen.go) from the JSON telemetry in `bench/results/`; the prose deliberately contains no measured values. [`doc_sync_test.go`](../doc_sync_test.go) fails `go test ./...` when a block drifts from the telemetry, when a parity check fails, or when telemetry lacks provenance.

---

## 1. Environment, Provenance, and Protocol

```bash
go run ./cmd/goblin-bench -check-docs
```

- **CPU**: `13th Gen Intel(R) Core(TM) i9-13900K` (`x86_64`, 30 logical CPUs exposed by `lscpu`, AVX2 + FMA).
- **Default build**: `go build` with the pure-Go kernels (`BackendGo`). All headline timings use this build.
- **Experimental build**: `GOEXPERIMENT=simd`, compared to the default build only in section 5.

<!-- BEGIN GENERATED:PROVENANCE -->
- **goblin telemetry source**: fingerprint `912de0c86eda99b6` (commit `5dc6cab27c56a240c1a7e6aa282a29edfb537e02`), `go1.27.3-X:nodwarf5`; the documentation gate rejects telemetry whose fingerprint differs from the checked-in sources.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, CBC `2.10.3` (`cbc`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 1.53 ms median (`prob.solve()` on a 1-variable MILP); the "Net of Spawn" columns subtract it.
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `master` (commit `c1c171ec`, vendored in `third_party/Cbc`), linked in-process through `cgo` (`gb.WithCBC()`). The C++ backend maps default settings onto CBC with two deliberate differences: `MaxCutPasses = 12` becomes `passC = 30` and `MaxNodes = 50000` becomes unlimited. It is therefore not a like-for-like setup and not the CBC 2.10.3 binary used by the Python baseline.
<!-- END GENERATED:PROVENANCE -->

- **Single-threaded, sequential, pinned execution**: every solve runs single-threaded (`threads=1` in CBC, one goroutine in `gb.SolveCompiled`), all harnesses run with `workers=1`, and each harness is pinned with `taskset` to one performance core. The i9-13900K mixes 5.5 GHz P-cores (CPUs 0-13) with 4.3 GHz E-cores (CPUs 14-29); unpinned runs migrate between them and produced spurious 2x differences between builds on models where no AVX2 kernel runs. The recorded affinity appears in the provenance block.
- **Warm state**: each harness runs an untimed worker warmup (`_init_worker` in [`bench/python_baseline.py`](../bench/python_baseline.py), `warmWorker` in [`cmd/goblin-bench/main.go`](../cmd/goblin-bench/main.go)), then 2 untimed warmup solves per family on Tiers 1-2 and 1 on Tier 3.
- **Randomized trials**: each family in `testdata/corpus/trials/<id>.jsonl` holds 11 seeded instances (Tiers 1-2) or 5 (Tier 3), 293 instances in total. Trial $k$ is the same instance in every harness, which enables paired comparisons.
- **Metrics**: per trial, prep time (`t_parse + t_build + t_format/t_compile`), solve time (`t_solve`), and end-to-end time (`t_total`). Tables report medians; spread tables report mean $\pm$ SD with two-sided 95% Student-$t$ intervals.
- **CBC spawn floor**: the Python harness also times `prob.solve()` on a 1-variable MILP. That fixed cost (MPS write, process spawn, CBC start-up, `.sol` parsing) is subtracted in the "Net of Spawn" columns to separate interface overhead from solver work.
- **Parity**: the benchmark gate requires both `goblin` pure-Go builds, and `goblin (CBC)`, to match CBC's status, canonical objective, and every per-trial objective within a relative tolerance of 1e-5. The test suite in [`solver_test.go`](../solver_test.go) re-solves all 293 instances and checks that every `goblin` solution satisfies all bounds and rows.

---

## 2. Aggregate Workload Summary

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families | `Py / goblin (Go)` GeoMean | `Py net / goblin (Go)` | `goblin (Go) / CBC` | `goblin (Go)` (ms) | `goblin (CBC)` (ms) | Py Total (ms) | Py Net (ms) | `Py / goblin (Go)` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical (`N <= 26`) | 10 | **86.3x** | 2.8x | 0.06x | 0.03 | 0.53 | 2.50 | 0.08 | 109.6x [24.4x-249.2x] |
| Tier 2: Target p90 (`30 <= N <= 84`) | 13 | **10.8x** | 8.2x | 0.17x | 1.02 | 6.14 | 11.03 | 8.43 | 11.1x [2.4x-22.1x] |
| Tier 3: Large Tail (`145 <= N <= 4036`) | 8 | **5.4x** | 5.1x | 0.27x | 11.14 | 41.84 | 59.90 | 56.86 | 7.7x [1.2x-15.6x] |
| Tier 3 non-MAD MILPs | 4 | **4.0x** | 3.9x | 0.40x | 23.28 | 58.79 | 92.48 | 90.63 | 5.2x [1.2x-15.6x] |
| Planted-gap MAD family (zero integrality gap by construction) | 8 | **10.6x** | 8.0x | 0.14x | 1.19 | 8.24 | 12.66 | 9.57 | 9.4x [5.4x-24.4x] |
| Random-weight MAD (negative controls) | 3 | **14.4x** | 13.8x | 0.17x | 4.10 | 24.78 | 58.88 | 56.80 | 11.1x [6.9x-38.5x] |
| **All families except planted-gap MAD** | 23 | **21.0x** | 4.4x | 0.13x | 0.47 | 3.71 | 9.93 | 2.07 | 15.6x [1.2x-249.2x] |
| All corpus families | 31 | **17.6x** | 5.1x | 0.13x | 0.60 | 4.56 | 10.57 | 3.08 | 14.2x [1.2x-249.2x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

How to read this table:
1. **Small models are dominated by interface cost.** For Tier 1, most of the Python time is the CBC spawn floor. Compare the "Py net / goblin" column with "Py / goblin" to see how much of the ratio is process overhead rather than solver speed.
2. **The planted-gap MAD family is not representative.** Its instances have a zero root LP gap by design, and `tryRootSubMIP` closes them at node 1. The bold row excludes that family and is the fairer aggregate.
3. **`goblin (CBC)` is not a like-for-like CBC baseline.** It links a different CBC build with different settings (see the provenance block), so the `goblin / goblin (CBC)` column compares setups, not engines.
4. **Large degenerate LPs and PFI updates.** The pure-Go `goblin` engine factors the basis with a three-stage singleton and sparse Markowitz LU, `FTRANSparse`/`BTRANSparse`, and dual bound flipping (`selectDualBFRT`), but updates the basis with a product-form (PFI) eta file rather than Forrest-Tomlin $U$-updates and omits matrix scaling; see [`solver-internals.md`](solver-internals.md#known-limits-of-the-pure-go-engine).

---

## 3. MAD Group-Item Allocation Formulation (`G` Groups $\times$ `I` Items)

The `MAD` group-item allocation family ([`BuildMADFromWeights`](../bench/corpus.go)) models multi-period balanced assignment across $G$ groups, $I$ items, and $T = 6$ time periods with central-level roundness penalties.

### Decision Variables
- Binary assignment variables $x_{g,i} \in \{0, 1\}$ for $g \in \{0, \dots, G-1\}$ and $i \in \{0, \dots, I-1\}$, equal to $1$ if item $i$ is assigned to group $g$.
- Continuous L1 residual variables $z_{g,t} \ge 0$ for $g \in \{0, \dots, G-1\}$ and $t \in \{0, \dots, T-1\}$, tracking the absolute distance of group $g$'s aggregate weight in period $t$ from its central level $m_g$.
- Continuous level variables $m_g \ge 0$ for $g \in \{0, \dots, G-1\}$, representing the central weight level for group $g$.
- General integer multiple variables $n_g \in \mathbb{Z}_{\ge 1}$ for $g \in \{0, \dots, G-1\}$, representing the positive integer multiple of the rounding base $B = 100$.
- Continuous roundness penalty variables $r_g \ge 0$ for $g \in \{0, \dots, G-1\}$, tracking $|m_g - 100 n_g|$.

Total variables: $N = G \cdot I + G \cdot T + 3G = G(I + T + 3)$.

### Objective and Constraints
$$\min \sum_{g=0}^{G-1} \sum_{t=0}^{T-1} z_{g,t} + \sum_{g=0}^{G-1} r_g$$

subject to:
1. **Linearized L1 Residual** ($2 \cdot G \cdot T$ rows):
   $$z_{g,t} - \sum_{i=0}^{I-1} W_{t,i} x_{g,i} + m_g \ge 0 \quad \forall g, t$$
   $$z_{g,t} + \sum_{i=0}^{I-1} W_{t,i} x_{g,i} - m_g \ge 0 \quad \forall g, t$$
2. **Linearized Roundness Penalty** ($2 \cdot G$ rows, with $B = 100$):
   $$r_g - m_g + 100 n_g \ge 0 \quad \forall g$$
   $$r_g + m_g - 100 n_g \ge 0 \quad \forall g$$
3. **Item Partitioning** ($I$ equality rows):
   $$\sum_{g=0}^{G-1} x_{g,i} = 1 \quad \forall i \in \{0, \dots, I-1\}$$
4. **Designated Subset Assignment** ($G$ equality rows over $D = \{I-G, \dots, I-1\}$):
   $$\sum_{i \in D} x_{g,i} = 1 \quad \forall g \in \{0, \dots, G-1\}$$

Total constraints: $M = G(2T + 3) + I$.

### Structural Disclosure: Planted-Gap Instances vs. Random-Weight Negative Controls

In the planted-gap generator ([`bench/corpus.go`](../bench/corpus.go)), regular items $i \in \{0, \dots, I-G-1\}$ carry group signatures along a 4D polynomial curve in the subspace orthogonal to both the constant vector and $\operatorname{sign}(\eta)$, while designated items $i \in D = \{I-G, \dots, I-1\}$ carry the complementary hedge plus a co-monotonic macro trajectory $(g+1)\eta_t$ with period sums aligned to multiples of $B = 100$. Consequently:
- Every structured instance `t*_mad_g*` has a **zero root LP gap** (`RootLPBound == OptimalObj` with $r_g = 0$), which the root heuristics (`tryPropagatedRounding`, `tryRootSubMIP`, and `tryFractionalDiving`) close at node 1.
- [`BuildMADRandom`](../bench/corpus.go) (`t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`, and `TestRandomMADNegativeControls` in [`solver_test.go`](../solver_test.go)) uses the same `BuildMADFromWeights` constraint matrix and assigns dense pseudorandom weights $W_{t,i} \in [100, 2000)$ to every item. These negative controls have a strict LP-to-MIP gap, the root heuristic does not close it, and the solver must branch.

### MAD Results (`G` Groups $\times$ `I` Items, $T = 6$)

<!-- BEGIN GENERATED:MAD_PIPELINE_TABLE -->
| Problem ID | $G \times I$ | Vars | Rows | `Py / goblin (Go)` | GoTotal (ms) | PyTotal (ms) | GoSolve (ms) | PySolve (ms) | PySolve Net (ms) | GoPrep (ms) | PyPrep (ms) | Optimal Obj | Parity |
| :--- | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **24.4x** | 0.16 | 3.82 | 0.05 | 3.63 | 2.10 | 0.104 | 0.246 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **38.5x** | 0.64 | 24.75 | 0.55 | 24.53 | 22.99 | 0.088 | 0.281 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **16.6x** | 0.21 | 3.43 | 0.09 | 3.18 | 1.65 | 0.115 | 0.329 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **14.2x** | 0.30 | 4.21 | 0.11 | 3.83 | 2.30 | 0.187 | 0.486 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **10.1x** | 0.52 | 5.28 | 0.22 | 4.71 | 3.17 | 0.287 | 0.740 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **11.1x** | 3.90 | 43.22 | 3.77 | 42.91 | 41.38 | 0.130 | 0.382 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **6.9x** | 27.51 | 190.87 | 27.30 | 190.46 | 188.93 | 0.209 | 0.542 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **8.8x** | 1.11 | 9.85 | 0.49 | 8.88 | 7.35 | 0.650 | 1.308 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **7.7x** | 1.65 | 12.68 | 0.77 | 10.99 | 9.45 | 0.820 | 2.243 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **5.4x** | 8.60 | 46.51 | 4.58 | 37.92 | 36.39 | 3.905 | 10.812 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **7.6x** | 51.06 | 389.97 | 34.88 | 345.93 | 344.40 | 15.777 | 44.695 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Multi-Trial Latency Dispersion (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | $0.16 \pm 0.04$ ($\pm 0.02$) | $3.75 \pm 0.22$ ($\pm 0.15$) | $0.19 \pm 0.09$ ($\pm 0.06$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | $0.86 \pm 0.70$ ($\pm 0.47$) | $22.50 \pm 8.42$ ($\pm 5.66$) | $0.89 \pm 0.76$ ($\pm 0.51$) |
| `t2_mad_g2_i8` | $2 \times 8$ | $0.21 \pm 0.02$ ($\pm 0.01$) | $3.59 \pm 0.32$ ($\pm 0.22$) | $0.25 \pm 0.05$ ($\pm 0.03$) |
| `t2_mad_g3_i9` | $3 \times 9$ | $0.32 \pm 0.05$ ($\pm 0.04$) | $4.20 \pm 0.05$ ($\pm 0.03$) | $0.28 \pm 0.05$ ($\pm 0.03$) |
| `t2_mad_g4_i12` | $4 \times 12$ | $0.55 \pm 0.07$ ($\pm 0.05$) | $5.30 \pm 0.09$ ($\pm 0.06$) | $0.54 \pm 0.08$ ($\pm 0.06$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | $3.73 \pm 2.01$ ($\pm 1.35$) | $31.51 \pm 18.28$ ($\pm 12.28$) | $3.63 \pm 1.98$ ($\pm 1.33$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | $33.38 \pm 29.64$ ($\pm 19.91$) | $166.14 \pm 82.17$ ($\pm 55.20$) | $33.39 \pm 29.52$ ($\pm 19.83$) |
| `t3_mad_g5_i20` | $5 \times 20$ | $1.09 \pm 0.11$ ($\pm 0.13$) | $9.31 \pm 1.08$ ($\pm 1.34$) | $1.02 \pm 0.13$ ($\pm 0.16$) |
| `t3_mad_g6_i30` | $6 \times 30$ | $1.86 \pm 0.79$ ($\pm 0.98$) | $13.63 \pm 2.27$ ($\pm 2.82$) | $2.92 \pm 0.99$ ($\pm 1.23$) |
| `t3_mad_g10_i100` | $10 \times 100$ | $8.64 \pm 0.63$ ($\pm 0.78$) | $47.36 \pm 3.41$ ($\pm 4.24$) | $8.45 \pm 0.45$ ($\pm 0.56$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | $57.45 \pm 13.56$ ($\pm 16.84$) | $463.02 \pm 195.32$ ($\pm 242.49$) | $54.81 \pm 13.97$ ($\pm 17.34$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Optimal Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | **0.05** (1 / 0) | 0.06 (1 / 0) | 0.07 (1) | 0.32 (3) | 0.062 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.55** (5 / 0) | 0.49 (12 / 0) | 0.41 (12) | 0.74 (84) | 0.062 | 4601.00 | 4257.00 |
| `t2_mad_g2_i8` | **0.09** (1 / 0) | 0.47 (9 / 0) | 0.34 (9) | 0.17 (2) | 0.077 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.11** (1 / 0) | 0.09 (1 / 0) | 0.09 (1) | 1.12 (136) | 0.182 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.22** (1 / 0) | 1.19 (15 / 0) | 1.00 (15) | 4.97 (47) | 0.256 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.77** (43 / 0) | 1.99 (41 / 0) | 2.17 (41) | 2.99 (126) | 0.104 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **27.30** (137 / 0) | 23.27 (201 / 0) | 12.08 (201) | 242.97 (2064) | 0.229 | 7720.00 | 6060.00 |
| `t3_mad_g5_i20` | **0.49** (1 / 0) | 1.93 (14 / 0) | 1.65 (14) | 0.84 (1) | 0.837 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.77** (1 / 0) | 2.40 (12 / 0) | 2.65 (12) | 5.78 (27) | 1.247 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **4.58** (1 / 0) | 60.65 (130 / 0) | 44.59 (130) | 180.04 (270) | 13.145 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **34.88** (1 / 0) | 38.55 (2 / 0) | 36.25 (2) | 250.39 (177, `TimeLimit`) | 98.820 | 208000.00 | 208000.00 |
<!-- END GENERATED:MAD_CHECKPOINT_TABLE -->

---

## 4. Full Corpus End-to-End Comparison

The corpus spans 31 families in three tiers:
- **Tier 1 (10 families)**: canonical `PuLP` unit models (`t1_std_lp`, `t1_std_lp_max`, `t1_std_mip`, `t1_mip_float_obj`, `t1_whiskas`, `t1_furniture`, `t1_knapsack_12`, `t1_infeasible_mip`), `t1_mad_g2_i4`, and `t1_mad_rand_g2_i4`.
- **Tier 2 (13 families)**: 30 to 84 variables: multidimensional knapsack, capacitated depot siting, generalized assignment, multi-item lot sizing, bin packing, set cover, three planted-gap MAD and two random-weight MAD families.
- **Tier 3 (8 families)**: 145 to 4,036 variables: `t3_mkp_200`, `t3_cflp_300`, `t3_lotsizing_504`, `t3_sched_1000`, and four planted-gap MAD families.

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | `Py / goblin (Go)` | GoTotal (ms) | PyTotal (ms) | GoSolve (ms) | PySolve (ms) | PySolve Net (ms) | GoPrep (ms) | PyPrep (ms) | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | **117.8x** | 0.01 | 1.34 | 0.00 | 1.30 | 0.00 | 0.010 | 0.056 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **119.7x** | 0.01 | 1.34 | 0.00 | 1.26 | 0.00 | 0.009 | 0.056 | `OK` |
| `t1_std_mip` | 3 | 3 | **179.4x** | 0.01 | 2.01 | 0.00 | 1.96 | 0.43 | 0.008 | 0.059 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **159.9x** | 0.01 | 1.91 | 0.00 | 1.87 | 0.33 | 0.008 | 0.057 | `OK` |
| `t1_whiskas` | 6 | 5 | **60.4x** | 0.02 | 1.39 | 0.00 | 1.33 | 0.00 | 0.020 | 0.082 | `OK` |
| `t1_furniture` | 2 | 3 | **101.4x** | 0.02 | 2.23 | 0.01 | 2.18 | 0.65 | 0.008 | 0.057 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **39.3x** | 0.08 | 3.21 | 0.06 | 3.14 | 1.61 | 0.024 | 0.092 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **249.2x** | 0.01 | 1.45 | 0.00 | 1.42 | 0.00 | 0.006 | 0.046 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **24.4x** | 0.16 | 3.82 | 0.05 | 3.63 | 2.10 | 0.104 | 0.246 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **38.5x** | 0.64 | 24.75 | 0.55 | 24.53 | 22.99 | 0.088 | 0.281 | `OK` |
| `t2_mkp_30` | 30 | 6 | **11.1x** | 2.50 | 27.67 | 2.44 | 27.50 | 25.97 | 0.066 | 0.208 | `OK` |
| `t2_mkp_50` | 50 | 8 | **2.4x** | 4.80 | 11.47 | 4.65 | 11.23 | 9.69 | 0.108 | 0.324 | `OK` |
| `t2_cflp_36` | 36 | 11 | **17.5x** | 0.20 | 3.56 | 0.14 | 3.39 | 1.86 | 0.059 | 0.191 | `OK` |
| `t2_cflp_49` | 49 | 13 | **10.2x** | 0.41 | 4.15 | 0.30 | 3.96 | 2.42 | 0.098 | 0.259 | `OK` |
| `t2_gap_45` | 45 | 13 | **11.3x** | 0.75 | 8.48 | 0.67 | 8.28 | 6.75 | 0.075 | 0.241 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **8.4x** | 4.73 | 39.67 | 4.62 | 39.44 | 37.91 | 0.096 | 0.290 | `OK` |
| `t2_binpack_44` | 44 | 14 | **14.7x** | 0.46 | 6.73 | 0.37 | 6.57 | 5.04 | 0.076 | 0.212 | `OK` |
| `t2_setcover_50` | 50 | 20 | **22.1x** | 0.24 | 5.37 | 0.11 | 5.11 | 3.57 | 0.154 | 0.353 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **16.6x** | 0.21 | 3.43 | 0.09 | 3.18 | 1.65 | 0.115 | 0.329 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **14.2x** | 0.30 | 4.21 | 0.11 | 3.83 | 2.30 | 0.187 | 0.486 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **10.1x** | 0.52 | 5.28 | 0.22 | 4.71 | 3.17 | 0.287 | 0.740 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **11.1x** | 3.90 | 43.22 | 3.77 | 42.91 | 41.38 | 0.130 | 0.382 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **6.9x** | 27.51 | 190.87 | 27.30 | 190.46 | 188.93 | 0.209 | 0.542 | `OK` |
| `t3_mkp_200` | 200 | 8 | **1.2x** | 33.05 | 41.23 | 32.55 | 40.62 | 39.09 | 0.320 | 0.782 | `OK` |
| `t3_cflp_300` | 300 | 34 | **15.6x** | 13.49 | 211.02 | 13.06 | 210.13 | 208.60 | 0.440 | 1.147 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **1.4x** | 89.53 | 128.49 | 88.47 | 126.64 | 125.11 | 0.824 | 2.268 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **8.9x** | 7.35 | 65.45 | 5.99 | 62.22 | 60.69 | 1.222 | 3.667 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **8.8x** | 1.11 | 9.85 | 0.49 | 8.88 | 7.35 | 0.650 | 1.308 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **7.7x** | 1.65 | 12.68 | 0.77 | 10.99 | 9.45 | 0.820 | 2.243 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.4x** | 8.60 | 46.51 | 4.58 | 37.92 | 36.39 | 3.905 | 10.812 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **7.6x** | 51.06 | 389.97 | 34.88 | 345.93 | 344.40 | 15.777 | 44.695 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## 5. Default Build vs. `GOEXPERIMENT=simd`

In [`internal/simd/kernels_amd64.go`](../internal/simd/kernels_amd64.go), AVX2 kernels spill 4-lane accumulators to the stack via `StoreArray` and execute `archsimd.ClearAVXUpperBits()` (`VZEROUPPER`) before scalar tail or caller operations, eliminating AVX-to-SSE dirty-upper `YMM` transition stalls. From the dispatch threshold `minAVX2Len = 32` upward, the 256-bit AVX2 kernels (`Dot64`, `Axpy64`, `Scale64`, `AbsMax64`, `AllAbsLE64`, `RowActivityBoundsFinite`, `MinReducedSlice64`, `InitReducedRow64`, and `Min2ReducedRow64` at $N \ge 64$) outperform the 4-way slice-stepped scalar loops; below $N = 32$, call and `VZEROUPPER` overhead dominates, so slices of length $< 32$ remain on the scalar path.

At the full-solver level, each ratio below is the geometric mean of per-trial `scalar / SIMD` solve times over the same instances, with a 95% $t$-interval on the log ratios; "no detectable effect" means the interval contains 1. "Reachable" is a necessary condition: the AVX2 `Dot64`/`Axpy64`/`Scale64` paths require a slice of length 32 or more (`n + m >= 32`). Below that size no AVX2 kernel can run, so those 8 families serve as negative controls where intervals excluding 1 reflect binary code-layout or run-to-run variance rather than SIMD execution. Lowering `minAVX2Len` from 256 to 32 enables AVX2 across dense tableau row accumulation (`ComputeTableauRow`) and dual reduced-cost updates (`solveDual`) for medium models with $n \le 256$ (`DenseThreshold`), while `ComputeTableauRow` dispatches `Axpy64` on contiguous CSR column blocks in large models ($n > 256$, such as `t3_mad_g4_i1000`).

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 0 of 8 have a 95% interval excluding 1. These differences come from the two binaries (compiler output and code layout differ under `GOEXPERIMENT=simd`) and from run-to-run effects, not from SIMD kernels. Intervals of similar size on the AVX2-capable instances are therefore not evidence of a SIMD effect.
- **Instances where AVX2 can run**: 3 faster, 2 slower, 18 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 0.99x [0.86x-1.14x] | 0.002 | 0.002 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 1.03x [0.96x-1.09x] | 0.001 | 0.001 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 0.96x [0.81x-1.15x] | 0.003 | 0.003 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source | 1.05x [0.86x-1.28x] | 0.004 | 0.003 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 1.04x [0.98x-1.10x] | 0.002 | 0.002 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 1.03x [0.94x-1.12x] | 0.012 | 0.011 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source | 0.99x [0.95x-1.03x] | 0.059 | 0.060 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 0.96x [0.90x-1.04x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 0.93x [0.81x-1.07x] | 0.050 | 0.054 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 0.98x [0.93x-1.03x] | 0.550 | 0.502 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 1.04x [0.97x-1.11x] | 2.442 | 2.402 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 1.05x [0.99x-1.11x] | 4.649 | 4.233 | 11 |
| `t2_cflp_36` | 47 | possible | no detectable effect | 1.01x [0.93x-1.10x] | 0.143 | 0.137 | 11 |
| `t2_cflp_49` | 62 | possible | SIMD faster | 1.13x [1.06x-1.20x] | 0.304 | 0.317 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 0.97x [0.90x-1.04x] | 0.674 | 0.730 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 1.02x [0.97x-1.07x] | 4.624 | 4.601 | 11 |
| `t2_binpack_44` | 58 | possible | no detectable effect | 1.05x [1.00x-1.11x] | 0.373 | 0.367 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 1.06x [0.95x-1.17x] | 0.106 | 0.091 | 11 |
| `t2_mad_g2_i8` | 72 | possible | SIMD slower | 0.80x [0.64x-0.99x] | 0.086 | 0.112 | 11 |
| `t2_mad_g3_i9` | 108 | possible | SIMD faster | 1.15x [1.00x-1.32x] | 0.110 | 0.096 | 11 |
| `t2_mad_g4_i12` | 156 | possible | no detectable effect | 0.97x [0.82x-1.14x] | 0.218 | 0.219 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.03x [0.97x-1.10x] | 3.768 | 3.490 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 0.98x [0.96x-1.01x] | 27.301 | 27.183 | 11 |
| `t3_mkp_200` | 208 | possible | SIMD faster | 1.04x [1.00x-1.07x] | 32.554 | 31.933 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.01x [0.95x-1.07x] | 13.061 | 12.262 | 5 |
| `t3_lotsizing_504` | 840 | possible | no detectable effect | 1.00x [0.99x-1.01x] | 88.467 | 89.254 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 1.00x [0.93x-1.08x] | 5.994 | 5.973 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 1.01x [0.73x-1.40x] | 0.486 | 0.477 | 5 |
| `t3_mad_g6_i30` | 354 | possible | SIMD slower | 0.65x [0.47x-0.89x] | 0.768 | 1.257 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 1.02x [0.95x-1.10x] | 4.576 | 4.634 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | no detectable effect | 1.01x [0.98x-1.03x] | 34.879 | 33.794 | 5 |
<!-- END GENERATED:SIMD_EFFECT_TABLE -->

---

## 6. Full Corpus Solver-Core Checkpoint Progression

<!-- BEGIN GENERATED:FULL_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Optimal Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_std_lp` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.001 | 54.00 | 54.00 |
| `t1_std_lp_max` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.001 | 80.00 | 80.00 |
| `t1_std_mip` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (3) | 0.001 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (3) | 0.001 | 64.95 | 58.90 |
| `t1_whiskas` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 0.52 | 0.52 |
| `t1_furniture` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.001 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.06** (1 / 8) | 0.06 (2 / 8) | 0.08 (24) | 0.11 (31) | 0.004 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (5) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.05** (1 / 0) | 0.06 (1 / 0) | 0.07 (1) | 0.32 (3) | 0.062 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.55** (5 / 0) | 0.49 (12 / 0) | 0.41 (12) | 0.74 (84) | 0.062 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.44** (29 / 8) | 1.86 (60 / 8) | 0.91 (77) | 0.75 (77) | 0.018 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **4.65** (1 / 4) | 3.96 (2 / 4) | 3.86 (132) | 3.70 (119) | 0.028 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.14** (1 / 2) | 0.98 (3 / 2) | 0.12 (7) | 0.12 (7) | 0.018 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.30** (1 / 20) | 1.42 (4 / 20) | 0.29 (9) | 0.27 (9) | 0.024 | 909.02 | 827.30 |
| `t2_gap_45` | **0.67** (1 / 8) | 0.22 (1 / 8) | 1.43 (61) | 1.48 (61) | 0.020 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **4.62** (3 / 35) | 5.56 (33 / 35) | 1.61 (198) | 1.84 (174) | 0.047 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **0.37** (1 / 5) | 0.84 (13 / 5) | 13.23 (1838) | 24.78 (2450) | 0.025 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.11** (1 / 6) | 0.19 (2 / 6) | 0.69 (9) | 0.75 (9) | 0.028 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.09** (1 / 0) | 0.47 (9 / 0) | 0.34 (9) | 0.17 (2) | 0.077 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.11** (1 / 0) | 0.09 (1 / 0) | 0.09 (1) | 1.12 (136) | 0.182 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.22** (1 / 0) | 1.19 (15 / 0) | 1.00 (15) | 4.97 (47) | 0.256 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.77** (43 / 0) | 1.99 (41 / 0) | 2.17 (41) | 2.99 (126) | 0.104 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **27.30** (137 / 0) | 23.27 (201 / 0) | 12.08 (201) | 242.97 (2064) | 0.229 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **32.55** (594 / 14) | 39.69 (701 / 14) | 84.58 (5000, `NodeLimit`) | 65.76 (5000, `NodeLimit`) | 0.101 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **13.06** (48 / 31) | 18.41 (63 / 31) | 6.08 (134) | 5.84 (134) | 0.127 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **88.47** (27 / 203) | 115.57 (416 / 203) | 209.87 (5000, `NodeLimit`) | 192.63 (5000, `NodeLimit`) | 0.780 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **5.99** (35 / 47) | 9.64 (96 / 47) | 205.99 (5000, `NodeLimit`) | 147.99 (5000, `NodeLimit`) | 0.121 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.49** (1 / 0) | 1.93 (14 / 0) | 1.65 (14) | 0.84 (1) | 0.837 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.77** (1 / 0) | 2.40 (12 / 0) | 2.65 (12) | 5.78 (27) | 1.247 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **4.58** (1 / 0) | 60.65 (130 / 0) | 44.59 (130) | 180.04 (270) | 13.145 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **34.88** (1 / 0) | 38.55 (2 / 0) | 36.25 (2) | 250.39 (177, `TimeLimit`) | 98.820 | 208000.00 | 208000.00 |
<!-- END GENERATED:FULL_CHECKPOINT_TABLE -->

---

## 7. Linear Sum Assignment Benchmarks (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family Linear Sum Assignment corpus ([`bench/assign_corpus.go`](../bench/assign_corpus.go), manifest in `testdata/assign_corpus/manifest.json`) evaluates pure-Go [`assign.LinearSumFlat`](../assign/assign.go) against SciPy's compiled C++ `rectangular_lsap` extension (`scipy.optimize.linear_sum_assignment`) across 200 randomized instances in warm state:
- **Bit-identical matrices**: every entry is generated from a 64-bit `SplitMix64` stream (`splitMix64At`) scaled by exact dyadic rationals ($2^{-k}$), and [`bench/scipy_assign_baseline.py`](../bench/scipy_assign_baseline.py) verifies the IEEE-754 SHA-256 digest of every family's canonical matrix against `manifest.json` before timing.
- **No subprocess floor**: unlike `PuLP + CBC`, `scipy.optimize.linear_sum_assignment` calls its C++ extension in-process on a contiguous `float64` NumPy array, so `SciPy Solve / goblin` compares in-memory solver execution directly (with ~1 to 2 $\mu\text{s}$ of Python wrapper validation and index-array allocation included on micro Tier-1 instances).

### 7.1 Aggregate LSAP Workload Summary

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families | `SciPy Solve / goblin` GeoMean | `SciPy Total / goblin` GeoMean | `goblin` Solve (ms) | SciPy Solve (ms) | `goblin` Total (ms) | SciPy Total (ms) | `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 | **2.39x** | 5.52x | 0.0020 | 0.0047 | 0.0028 | 0.0156 | 1.89x [1.73x-5.63x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 | **2.78x** | 2.78x | 0.0659 | 0.183 | 0.0935 | 0.260 | 2.30x [1.44x-11.98x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 | **2.73x** | 2.95x | 2.037 | 5.553 | 3.030 | 8.946 | 2.71x [1.64x-3.95x] |
| Square families ($m = n$) | 16 | **2.89x** | 3.60x | 0.104 | 0.302 | 0.140 | 0.504 | 2.90x [1.44x-11.98x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 | **2.10x** | 2.96x | 0.0317 | 0.0667 | 0.0557 | 0.165 | 2.13x [1.73x-2.71x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 | **2.33x** | 2.74x | 0.0848 | 0.198 | 0.116 | 0.318 | 1.91x [1.44x-11.98x] |
| **All 22 LSAP corpus families** | 22 | **2.65x** | 3.42x | 0.0754 | 0.200 | 0.109 | 0.372 | 2.29x [1.44x-11.98x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 7.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | `SciPy Solve / goblin` | GoSolve (ms) | SciPySolve (ms) | GoTotal (ms) | SciPyTotal (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Optimal Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **5.63x** | 0.0003 | 0.0019 | 0.0004 | 0.0122 | $0.0003 \pm 0.0001$ | $0.0020 \pm 0.0003$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **3.01x** | 0.0011 | 0.0032 | 0.0013 | 0.0106 | $0.0011 \pm 0.0003$ | $0.0032 \pm 0.0004$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **2.01x** | 0.0035 | 0.0070 | 0.0050 | 0.0157 | $0.0035 \pm 0.0009$ | $0.0071 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.78x** | 0.0026 | 0.0047 | 0.0041 | 0.0143 | $0.0024 \pm 0.0005$ | $0.0047 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.73x** | 0.0029 | 0.0050 | 0.0044 | 0.0152 | $0.0030 \pm 0.0009$ | $0.0051 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.76x** | 0.0064 | 0.0113 | 0.0110 | 0.0321 | $0.0066 \pm 0.0025$ | $0.0112 \pm 0.0018$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.30x** | 0.0293 | 0.0673 | 0.0395 | 0.0915 | $0.0272 \pm 0.0058$ | $0.0664 \pm 0.0100$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **2.92x** | 0.0573 | 0.167 | 0.0745 | 0.218 | $0.0568 \pm 0.0050$ | $0.168 \pm 0.0258$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **1.91x** | 0.0473 | 0.0902 | 0.0695 | 0.135 | $0.0447 \pm 0.0045$ | $0.0929 \pm 0.0137$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.44x** | 0.146 | 0.210 | 0.154 | 0.249 | $0.153 \pm 0.0415$ | $0.227 \pm 0.0413$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **11.98x** | 0.0540 | 0.647 | 0.0701 | 0.722 | $0.0528 \pm 0.0136$ | $0.654 \pm 0.0259$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **2.24x** | 0.0297 | 0.0664 | 0.0540 | 0.135 | $0.0293 \pm 0.0032$ | $0.0685 \pm 0.0072$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.03x** | 0.0379 | 0.0769 | 0.0632 | 0.148 | $0.0381 \pm 0.0032$ | $0.0791 \pm 0.0090$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **2.88x** | 0.0949 | 0.273 | 0.138 | 0.355 | $0.0941 \pm 0.0076$ | $0.279 \pm 0.0320$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.44x** | 0.351 | 1.208 | 0.527 | 1.565 | $0.362 \pm 0.0455$ | $1.216 \pm 0.127$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.65x** | 1.386 | 5.065 | 2.029 | 6.730 | $1.431 \pm 0.143$ | $5.136 \pm 0.361$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.95x** | 0.759 | 1.481 | 1.313 | 2.800 | $0.768 \pm 0.0224$ | $1.474 \pm 0.0683$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.64x** | 4.980 | 8.160 | 5.175 | 9.560 | $5.944 \pm 2.022$ | $8.896 \pm 2.588$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.71x** | 0.291 | 0.787 | 0.644 | 2.142 | $0.285 \pm 0.0140$ | $0.800 \pm 0.0535$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.28x** | 0.417 | 0.949 | 0.758 | 2.179 | $0.408 \pm 0.0234$ | $0.948 \pm 0.0520$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **3.95x** | 6.871 | 27.110 | 8.798 | 33.193 | $6.602 \pm 0.539$ | $27.297 \pm 2.163$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **3.94x** | 33.359 | 131.378 | 39.596 | 164.230 | $34.340 \pm 3.012$ | $131.911 \pm 8.430$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 7.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSlice64`, `InitReducedRow64`, and `Min2ReducedRow64`)

In [`assign/solver.go`](../assign/solver.go), [`simd.AllAbsLE64With`](../internal/simd/kernels_amd64.go) validates finite cost bounds across the input matrix via 8-lane AVX2 `Abs().LessEqual` checks, [`simd.MinReducedSlice64With`](../internal/simd/kernels_amd64.go) accelerates reduction-transfer slack scans in `preprocessSquareJV`, [`simd.Min2ReducedRow64With`](../internal/simd/kernels_amd64.go) accelerates the first/second reduced-minimum scans in `preprocessRectangularBV` and `augmentingRowReduction` via 8-way AVX2 `VSUBPD` and `VMINPD`/`VCMPPD`/`VMOVMSKPD` bitmask filtering when $\max(m, n) \ge 64$, and [`simd.InitReducedRow64With`](../internal/simd/kernels_amd64.go) runs on a slice of length $\max(m, n)$ at the start of each Dijkstra augmentation when $\max(m, n) \ge 32$. All vector kernels clean upper `YMM` state via `StoreArray` + `ClearAVXUpperBits()` (`VZEROUPPER`) before scalar operations, yielding consistent end-to-end speedups across medium, large, and rectangular instances.

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32`, no AVX2 row-reduction kernel runs): 0 of 4 have a 95% interval excluding 1.
- **Instances where AVX2 row kernels run** (`max(rows, cols) >= 32`): 17 faster, 0 slower, 1 with no detectable effect, out of 18.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 row kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source | 0.98x [0.93x-1.04x] | 0.0003 | 0.0003 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source | 1.02x [0.97x-1.07x] | 0.0011 | 0.0011 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source | 1.02x [0.98x-1.07x] | 0.0035 | 0.0034 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | no detectable effect | 1.00x [0.97x-1.02x] | 0.0026 | 0.0026 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | SIMD faster | 1.14x [1.06x-1.22x] | 0.0029 | 0.0026 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | no (control) | identical source | 0.98x [0.96x-1.01x] | 0.0064 | 0.0065 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.09x [1.05x-1.13x] | 0.0293 | 0.0272 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.18x [1.15x-1.21x] | 0.0573 | 0.0491 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.33x [1.24x-1.44x] | 0.0473 | 0.0368 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.17x [1.11x-1.24x] | 0.146 | 0.127 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.11x [1.02x-1.22x] | 0.0540 | 0.0501 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.20x [1.11x-1.29x] | 0.0297 | 0.0263 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.04x [1.03x-1.06x] | 0.0379 | 0.0365 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.18x [1.15x-1.21x] | 0.0949 | 0.0794 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.33x [1.28x-1.39x] | 0.351 | 0.265 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.36x [1.24x-1.48x] | 1.386 | 1.027 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.43x [1.34x-1.53x] | 0.759 | 0.520 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.20x [1.02x-1.40x] | 4.980 | 4.366 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.77x [1.67x-1.88x] | 0.291 | 0.161 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.15x [1.10x-1.19x] | 0.417 | 0.370 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.33x [1.23x-1.45x] | 6.871 | 4.825 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.24x [1.15x-1.34x] | 33.359 | 26.669 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

---

## 8. Keeping Documentation and Benchmarks Synchronized

1. **Regenerate telemetry** from a clean, committed tree so that every entry records its `git_commit`: run the commands in the [README](../README.md#verification-benchmarking-and-documentation-sync-commands).
2. **Regenerate tables**: `go run ./cmd/goblin-bench -sync-docs`.
3. **Verify**: `go run ./cmd/goblin-bench -check-docs`, or `go test ./...`, which runs `TestDocumentationAndBenchmarkSync`. The gate validates corpus dimensions, parity, and provenance, and checks every generated block in `README.md`, `docs/benchmarks.md`, and `bench/results/final_report.md`.
