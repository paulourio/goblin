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
- **goblin telemetry source**: fingerprint `dd247e9560bd5139` (commit `b362b3890e081d9e18b37955c5dc58638a28627a`), `go1.27.3-X:nodwarf5`; the documentation gate rejects telemetry whose fingerprint differs from the checked-in sources.
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
| Tier 1: Canonical (`N <= 26`) | 10 | **67.3x** | 2.2x | 0.07x | 0.04 | 0.52 | 2.50 | 0.08 | 106.7x [8.5x-287.0x] |
| Tier 2: Target p90 (`30 <= N <= 84`) | 13 | **7.7x** | 5.9x | 0.23x | 1.43 | 6.22 | 11.03 | 8.43 | 8.6x [1.4x-21.7x] |
| Tier 3: Large Tail (`145 <= N <= 4036`) | 8 | **4.3x** | 4.1x | 0.34x | 13.95 | 40.99 | 59.90 | 56.86 | 4.5x [1.3x-16.0x] |
| Tier 3 non-MAD MILPs | 4 | **5.4x** | 5.3x | 0.29x | 17.14 | 58.26 | 92.48 | 90.63 | 6.7x [1.3x-16.0x] |
| Planted-gap MAD family (zero integrality gap by construction) | 8 | **6.1x** | 4.6x | 0.25x | 2.06 | 8.25 | 12.66 | 9.57 | 6.6x [1.7x-16.4x] |
| Random-weight MAD (negative controls) | 3 | **3.7x** | 3.6x | 0.61x | 15.86 | 26.06 | 58.88 | 56.80 | 4.2x [1.4x-8.5x] |
| **All families except planted-gap MAD** | 23 | **17.5x** | 3.6x | 0.15x | 0.57 | 3.69 | 9.93 | 2.07 | 12.2x [1.3x-287.0x] |
| All corpus families | 31 | **13.3x** | 3.9x | 0.17x | 0.79 | 4.54 | 10.57 | 3.08 | 8.8x [1.3x-287.0x] |
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
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **15.9x** | 0.24 | 3.82 | 0.09 | 3.63 | 2.10 | 0.133 | 0.246 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **8.5x** | 2.92 | 24.75 | 2.81 | 24.53 | 22.99 | 0.111 | 0.281 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **16.4x** | 0.21 | 3.43 | 0.09 | 3.18 | 1.65 | 0.117 | 0.329 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **8.6x** | 0.49 | 4.21 | 0.28 | 3.83 | 2.30 | 0.213 | 0.486 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **6.5x** | 0.81 | 5.28 | 0.49 | 4.71 | 3.17 | 0.293 | 0.740 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **4.2x** | 10.27 | 43.22 | 10.15 | 42.91 | 41.38 | 0.125 | 0.382 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **1.4x** | 132.89 | 190.87 | 132.67 | 190.46 | 188.93 | 0.212 | 0.542 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **6.6x** | 1.50 | 9.85 | 0.86 | 8.88 | 7.35 | 0.528 | 1.308 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **4.1x** | 3.06 | 12.68 | 1.95 | 10.99 | 9.45 | 0.924 | 2.243 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **1.7x** | 27.85 | 46.51 | 24.07 | 37.92 | 36.39 | 3.847 | 10.812 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **3.0x** | 130.07 | 389.97 | 115.36 | 345.93 | 344.40 | 14.626 | 44.695 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Multi-Trial Latency Dispersion (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | $0.24 \pm 0.07$ ($\pm 0.05$) | $3.75 \pm 0.22$ ($\pm 0.15$) | $0.22 \pm 0.07$ ($\pm 0.05$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | $3.04 \pm 1.79$ ($\pm 1.21$) | $22.50 \pm 8.42$ ($\pm 5.66$) | $3.17 \pm 1.95$ ($\pm 1.31$) |
| `t2_mad_g2_i8` | $2 \times 8$ | $0.21 \pm 0.02$ ($\pm 0.01$) | $3.59 \pm 0.32$ ($\pm 0.22$) | $0.25 \pm 0.04$ ($\pm 0.03$) |
| `t2_mad_g3_i9` | $3 \times 9$ | $0.56 \pm 0.17$ ($\pm 0.11$) | $4.20 \pm 0.05$ ($\pm 0.03$) | $0.44 \pm 0.06$ ($\pm 0.04$) |
| `t2_mad_g4_i12` | $4 \times 12$ | $0.84 \pm 0.23$ ($\pm 0.15$) | $5.30 \pm 0.09$ ($\pm 0.06$) | $0.80 \pm 0.19$ ($\pm 0.13$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | $9.46 \pm 5.12$ ($\pm 3.44$) | $31.51 \pm 18.28$ ($\pm 12.28$) | $9.36 \pm 5.27$ ($\pm 3.54$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | $433.62 \pm 895.82$ ($\pm 601.78$) | $166.14 \pm 82.17$ ($\pm 55.20$) | $435.23 \pm 900.05$ ($\pm 604.62$) |
| `t3_mad_g5_i20` | $5 \times 20$ | $2.01 \pm 1.13$ ($\pm 1.40$) | $9.31 \pm 1.08$ ($\pm 1.34$) | $1.97 \pm 1.12$ ($\pm 1.39$) |
| `t3_mad_g6_i30` | $6 \times 30$ | $2.98 \pm 0.64$ ($\pm 0.80$) | $13.63 \pm 2.27$ ($\pm 2.82$) | $2.94 \pm 0.46$ ($\pm 0.57$) |
| `t3_mad_g10_i100` | $10 \times 100$ | $65.69 \pm 70.42$ ($\pm 87.43$) | $47.36 \pm 3.41$ ($\pm 4.24$) | $67.01 \pm 73.45$ ($\pm 91.19$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | $156.35 \pm 84.22$ ($\pm 104.56$) | $463.02 \pm 195.32$ ($\pm 242.49$) | $146.39 \pm 77.24$ ($\pm 95.88$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Optimal Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | **0.09** (1 / 0) | 0.25 (2 / 2) | 0.43 (3) | 0.34 (3) | 0.067 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **2.81** (11 / 2) | 1.37 (21 / 2) | 0.87 (84) | 0.77 (84) | 0.058 | 4601.00 | 4257.00 |
| `t2_mad_g2_i8` | **0.09** (1 / 0) | 0.13 (3 / 1) | 0.17 (2) | 0.17 (2) | 0.072 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.28** (1 / 0) | 1.70 (5 / 3) | 1.46 (19) | 1.10 (19) | 0.176 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.49** (1 / 0) | 3.16 (12 / 4) | 2.31 (17) | 3.23 (16) | 0.235 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **10.15** (77 / 9) | 7.06 (312 / 9) | 3.08 (55) | 2.92 (149) | 0.097 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **132.67** (1161 / 15) | 104.25 (1172 / 15) | 67.42 (3407) | 226.70 (2064) | 0.199 | 7720.00 | 6060.00 |
| `t3_mad_g5_i20` | **0.86** (1 / 0) | 0.79 (1 / 0) | 0.80 (1) | 0.78 (1) | 0.715 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **1.95** (1 / 0) | 15.75 (59 / 14) | 11.85 (83) | 5.45 (27) | 1.180 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **24.07** (1 / 0) | 128.96 (397 / 14) | 93.26 (170) | 129.94 (311) | 10.680 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **115.36** (1 / 0) | 158.20 (10 / 4) | 95.14 (5) | 250.32 (323, `TimeLimit`) | 101.409 | 208000.00 | 208000.00 |
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
| `t1_std_lp` | 3 | 3 | **112.4x** | 0.01 | 1.34 | 0.00 | 1.30 | 0.00 | 0.010 | 0.056 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **116.9x** | 0.01 | 1.34 | 0.00 | 1.26 | 0.00 | 0.010 | 0.056 | `OK` |
| `t1_std_mip` | 3 | 3 | **130.9x** | 0.02 | 2.01 | 0.00 | 1.96 | 0.43 | 0.011 | 0.059 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **101.4x** | 0.02 | 1.91 | 0.01 | 1.87 | 0.33 | 0.012 | 0.057 | `OK` |
| `t1_whiskas` | 6 | 5 | **52.9x** | 0.03 | 1.39 | 0.00 | 1.33 | 0.00 | 0.022 | 0.082 | `OK` |
| `t1_furniture` | 2 | 3 | **111.9x** | 0.02 | 2.23 | 0.01 | 2.18 | 0.65 | 0.008 | 0.057 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **48.0x** | 0.07 | 3.21 | 0.05 | 3.14 | 1.61 | 0.022 | 0.092 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **287.0x** | 0.01 | 1.45 | 0.00 | 1.42 | 0.00 | 0.005 | 0.046 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **15.9x** | 0.24 | 3.82 | 0.09 | 3.63 | 2.10 | 0.133 | 0.246 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **8.5x** | 2.92 | 24.75 | 2.81 | 24.53 | 22.99 | 0.111 | 0.281 | `OK` |
| `t2_mkp_30` | 30 | 6 | **8.5x** | 3.26 | 27.67 | 3.20 | 27.50 | 25.97 | 0.060 | 0.208 | `OK` |
| `t2_mkp_50` | 50 | 8 | **2.5x** | 4.63 | 11.47 | 4.50 | 11.23 | 9.69 | 0.110 | 0.324 | `OK` |
| `t2_cflp_36` | 36 | 11 | **13.1x** | 0.27 | 3.56 | 0.21 | 3.39 | 1.86 | 0.066 | 0.191 | `OK` |
| `t2_cflp_49` | 49 | 13 | **11.0x** | 0.38 | 4.15 | 0.30 | 3.96 | 2.42 | 0.078 | 0.259 | `OK` |
| `t2_gap_45` | 45 | 13 | **8.8x** | 0.97 | 8.48 | 0.85 | 8.28 | 6.75 | 0.072 | 0.241 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **8.4x** | 4.72 | 39.67 | 4.63 | 39.44 | 37.91 | 0.097 | 0.290 | `OK` |
| `t2_binpack_44` | 44 | 14 | **12.2x** | 0.55 | 6.73 | 0.46 | 6.57 | 5.04 | 0.086 | 0.212 | `OK` |
| `t2_setcover_50` | 50 | 20 | **21.7x** | 0.25 | 5.37 | 0.10 | 5.11 | 3.57 | 0.144 | 0.353 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **16.4x** | 0.21 | 3.43 | 0.09 | 3.18 | 1.65 | 0.117 | 0.329 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **8.6x** | 0.49 | 4.21 | 0.28 | 3.83 | 2.30 | 0.213 | 0.486 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **6.5x** | 0.81 | 5.28 | 0.49 | 4.71 | 3.17 | 0.293 | 0.740 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **4.2x** | 10.27 | 43.22 | 10.15 | 42.91 | 41.38 | 0.125 | 0.382 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **1.4x** | 132.89 | 190.87 | 132.67 | 190.46 | 188.93 | 0.212 | 0.542 | `OK` |
| `t3_mkp_200` | 200 | 8 | **1.3x** | 31.79 | 41.23 | 31.52 | 40.62 | 39.09 | 0.294 | 0.782 | `OK` |
| `t3_cflp_300` | 300 | 34 | **16.0x** | 13.20 | 211.02 | 12.82 | 210.13 | 208.60 | 0.415 | 1.147 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **4.8x** | 26.89 | 128.49 | 26.13 | 126.64 | 125.11 | 0.770 | 2.268 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **8.6x** | 7.64 | 65.45 | 6.44 | 62.22 | 60.69 | 1.257 | 3.667 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **6.6x** | 1.50 | 9.85 | 0.86 | 8.88 | 7.35 | 0.528 | 1.308 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **4.1x** | 3.06 | 12.68 | 1.95 | 10.99 | 9.45 | 0.924 | 2.243 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **1.7x** | 27.85 | 46.51 | 24.07 | 37.92 | 36.39 | 3.847 | 10.812 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **3.0x** | 130.07 | 389.97 | 115.36 | 345.93 | 344.40 | 14.626 | 44.695 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## 5. Default Build vs. `GOEXPERIMENT=simd`

In [`internal/simd/kernels_amd64.go`](../internal/simd/kernels_amd64.go), AVX2 kernels spill 4-lane accumulators to the stack via `StoreArray` and execute `archsimd.ClearAVXUpperBits()` (`VZEROUPPER`) before scalar tail or caller operations, eliminating AVX-to-SSE dirty-upper `YMM` transition stalls. From the dispatch threshold `minAVX2Len = 32` upward, the 256-bit AVX2 kernels (`Dot64`, `Axpy64`, `Scale64`, `AbsMax64`, `AllAbsLE64`, `RowActivityBoundsFinite`, `MinReducedSlice64`, `InitReducedRow64`, and `Min2ReducedRow64` at $N \ge 64$) outperform the 4-way slice-stepped scalar loops; below $N = 32$, call and `VZEROUPPER` overhead dominates, so slices of length $< 32$ remain on the scalar path.

At the full-solver level, each ratio below is the geometric mean of per-trial `scalar / SIMD` solve times over the same instances, with a 95% $t$-interval on the log ratios; "no detectable effect" means the interval contains 1. "Reachable" is a necessary condition: the AVX2 `Dot64`/`Axpy64`/`Scale64` paths require a slice of length 32 or more (`n + m >= 32`). Below that size no AVX2 kernel can run, so those 8 families serve as negative controls where intervals excluding 1 reflect binary code-layout or run-to-run variance rather than SIMD execution. Lowering `minAVX2Len` from 256 to 32 enables AVX2 across dense tableau row accumulation (`ComputeTableauRow`) and dual reduced-cost updates (`solveDual`) for medium models with $n \le 256$ (`DenseThreshold`), while `ComputeTableauRow` dispatches `Axpy64` on contiguous CSR column blocks in large models ($n > 256$, such as `t3_mad_g4_i1000`).

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 6 of 8 have a 95% interval excluding 1. These differences come from the two binaries (compiler output and code layout differ under `GOEXPERIMENT=simd`) and from run-to-run effects, not from SIMD kernels. Intervals of similar size on the AVX2-capable instances are therefore not evidence of a SIMD effect.
- **Instances where AVX2 can run**: 3 faster, 1 slower, 19 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.20x [1.05x-1.37x] | 0.002 | 0.002 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.25x [1.08x-1.44x] | 0.001 | 0.001 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 2.03x [1.15x-3.58x] | 0.005 | 0.003 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.66x [1.22x-2.27x] | 0.006 | 0.004 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.21x [1.09x-1.35x] | 0.004 | 0.002 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 0.96x [0.86x-1.08x] | 0.012 | 0.011 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.72x [0.56x-0.92x] | 0.047 | 0.063 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 1.03x [0.93x-1.16x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 1.04x [0.76x-1.40x] | 0.095 | 0.089 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 1.00x [0.92x-1.09x] | 2.814 | 3.530 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 0.99x [0.94x-1.05x] | 3.196 | 3.208 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 0.93x [0.84x-1.02x] | 4.499 | 4.799 | 11 |
| `t2_cflp_36` | 47 | possible | no detectable effect | 1.02x [0.83x-1.26x] | 0.210 | 0.159 | 11 |
| `t2_cflp_49` | 62 | possible | no detectable effect | 0.98x [0.89x-1.07x] | 0.303 | 0.285 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 1.00x [0.94x-1.07x] | 0.846 | 0.674 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 1.00x [0.96x-1.05x] | 4.632 | 4.241 | 11 |
| `t2_binpack_44` | 58 | possible | no detectable effect | 1.05x [0.95x-1.16x] | 0.461 | 0.381 | 11 |
| `t2_setcover_50` | 70 | possible | SIMD faster | 1.19x [1.06x-1.34x] | 0.097 | 0.085 | 11 |
| `t2_mad_g2_i8` | 72 | possible | SIMD slower | 0.95x [0.91x-0.99x] | 0.090 | 0.098 | 11 |
| `t2_mad_g3_i9` | 108 | possible | SIMD faster | 1.18x [1.04x-1.35x] | 0.278 | 0.229 | 11 |
| `t2_mad_g4_i12` | 156 | possible | no detectable effect | 1.03x [1.00x-1.07x] | 0.492 | 0.471 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.02x [0.97x-1.07x] | 10.145 | 10.040 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 0.99x [0.99x-1.00x] | 132.674 | 132.502 | 11 |
| `t3_mkp_200` | 208 | possible | no detectable effect | 1.05x [1.00x-1.11x] | 31.522 | 30.101 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.02x [0.98x-1.07x] | 12.816 | 12.179 | 5 |
| `t3_lotsizing_504` | 840 | possible | no detectable effect | 1.00x [0.96x-1.04x] | 26.129 | 26.117 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 1.01x [0.95x-1.07x] | 6.436 | 6.529 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 0.99x [0.87x-1.12x] | 0.861 | 1.005 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 1.03x [0.96x-1.10x] | 1.954 | 1.913 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 1.01x [0.93x-1.10x] | 24.074 | 23.102 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | SIMD faster | 1.07x [1.05x-1.09x] | 115.357 | 106.843 | 5 |
<!-- END GENERATED:SIMD_EFFECT_TABLE -->

---

## 6. Full Corpus Solver-Core Checkpoint Progression

<!-- BEGIN GENERATED:FULL_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Optimal Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_std_lp` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.001 | 54.00 | 54.00 |
| `t1_std_lp_max` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.001 | 80.00 | 80.00 |
| `t1_std_mip` | **0.00** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.001 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.01** (1 / 0) | 0.00 (1 / 0) | 0.01 (1) | 0.00 (3) | 0.001 | 64.95 | 58.90 |
| `t1_whiskas` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 0.52 | 0.52 |
| `t1_furniture` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.001 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.05** (1 / 15) | 0.05 (2 / 15) | 0.08 (24) | 0.10 (31) | 0.004 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (5) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.09** (1 / 0) | 0.25 (2 / 2) | 0.43 (3) | 0.34 (3) | 0.067 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **2.81** (11 / 2) | 1.37 (21 / 2) | 0.87 (84) | 0.77 (84) | 0.058 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **3.20** (29 / 9) | 2.59 (60 / 9) | 1.19 (77) | 0.86 (77) | 0.020 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **4.50** (1 / 5) | 3.76 (2 / 5) | 3.87 (132) | 3.53 (119) | 0.027 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.21** (1 / 2) | 1.06 (3 / 2) | 0.15 (7) | 0.13 (7) | 0.021 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.30** (1 / 20) | 1.36 (4 / 20) | 0.27 (9) | 0.27 (9) | 0.024 | 909.02 | 827.30 |
| `t2_gap_45` | **0.85** (1 / 13) | 0.19 (1 / 13) | 1.39 (61) | 1.54 (61) | 0.022 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **4.63** (11 / 40) | 5.33 (33 / 40) | 1.74 (198) | 2.61 (174) | 0.060 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **0.46** (1 / 12) | 2.67 (20 / 12) | 16.15 (1873) | 20.33 (2416) | 0.021 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.10** (1 / 6) | 0.19 (2 / 6) | 0.67 (9) | 0.66 (9) | 0.026 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.09** (1 / 0) | 0.13 (3 / 1) | 0.17 (2) | 0.17 (2) | 0.072 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.28** (1 / 0) | 1.70 (5 / 3) | 1.46 (19) | 1.10 (19) | 0.176 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.49** (1 / 0) | 3.16 (12 / 4) | 2.31 (17) | 3.23 (16) | 0.235 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **10.15** (77 / 9) | 7.06 (312 / 9) | 3.08 (55) | 2.92 (149) | 0.097 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **132.67** (1161 / 15) | 104.25 (1172 / 15) | 67.42 (3407) | 226.70 (2064) | 0.199 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **31.52** (590 / 21) | 46.66 (739 / 21) | 85.96 (5000, `NodeLimit`) | 65.97 (5000, `NodeLimit`) | 0.099 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **12.82** (47 / 37) | 19.54 (47 / 37) | 5.93 (134) | 5.77 (134) | 0.132 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **26.13** (11 / 165) | 87.74 (365 / 165) | 238.55 (5000, `NodeLimit`) | 228.22 (5000, `NodeLimit`) | 1.186 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **6.44** (35 / 50) | 11.10 (96 / 50) | 216.23 (5000, `NodeLimit`) | 159.54 (5000, `NodeLimit`) | 0.151 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.86** (1 / 0) | 0.79 (1 / 0) | 0.80 (1) | 0.78 (1) | 0.715 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **1.95** (1 / 0) | 15.75 (59 / 14) | 11.85 (83) | 5.45 (27) | 1.180 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **24.07** (1 / 0) | 128.96 (397 / 14) | 93.26 (170) | 129.94 (311) | 10.680 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **115.36** (1 / 0) | 158.20 (10 / 4) | 95.14 (5) | 250.32 (323, `TimeLimit`) | 101.409 | 208000.00 | 208000.00 |
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
| Tier 1: Micro/Small (`N <= 32`) | 6 | **2.26x** | 5.68x | 0.0021 | 0.0047 | 0.0027 | 0.0156 | 1.92x [1.63x-4.46x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 | **2.79x** | 2.63x | 0.0656 | 0.183 | 0.0988 | 0.260 | 2.33x [1.46x-12.17x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 | **2.69x** | 2.90x | 2.065 | 5.553 | 3.084 | 8.946 | 2.65x [1.63x-3.91x] |
| Square families ($m = n$) | 16 | **2.83x** | 3.57x | 0.107 | 0.302 | 0.141 | 0.504 | 2.86x [1.46x-12.17x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 | **2.08x** | 2.82x | 0.0321 | 0.0667 | 0.0585 | 0.165 | 2.12x [1.63x-2.65x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 | **2.33x** | 2.82x | 0.0847 | 0.198 | 0.113 | 0.318 | 1.93x [1.46x-12.17x] |
| **All 22 LSAP corpus families** | 22 | **2.60x** | 3.35x | 0.0768 | 0.200 | 0.111 | 0.372 | 2.29x [1.46x-12.17x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 7.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | `SciPy Solve / goblin` | GoSolve (ms) | SciPySolve (ms) | GoTotal (ms) | SciPyTotal (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Optimal Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **4.46x** | 0.0004 | 0.0019 | 0.0005 | 0.0122 | $0.0004 \pm 0.0001$ | $0.0020 \pm 0.0003$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **2.84x** | 0.0011 | 0.0032 | 0.0014 | 0.0106 | $0.0012 \pm 0.0004$ | $0.0032 \pm 0.0004$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.98x** | 0.0035 | 0.0070 | 0.0044 | 0.0157 | $0.0035 \pm 0.0010$ | $0.0071 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.86x** | 0.0025 | 0.0047 | 0.0033 | 0.0143 | $0.0024 \pm 0.0006$ | $0.0047 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.63x** | 0.0031 | 0.0050 | 0.0039 | 0.0152 | $0.0030 \pm 0.0008$ | $0.0051 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.75x** | 0.0065 | 0.0113 | 0.0105 | 0.0321 | $0.0062 \pm 0.0016$ | $0.0112 \pm 0.0018$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.33x** | 0.0289 | 0.0673 | 0.0356 | 0.0915 | $0.0269 \pm 0.0059$ | $0.0664 \pm 0.0100$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **2.92x** | 0.0574 | 0.167 | 0.0743 | 0.218 | $0.0577 \pm 0.0072$ | $0.168 \pm 0.0258$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **1.94x** | 0.0464 | 0.0902 | 0.0688 | 0.135 | $0.0444 \pm 0.0042$ | $0.0929 \pm 0.0137$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.46x** | 0.144 | 0.210 | 0.152 | 0.249 | $0.149 \pm 0.0420$ | $0.227 \pm 0.0413$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **12.17x** | 0.0531 | 0.647 | 0.0699 | 0.722 | $0.0515 \pm 0.0096$ | $0.654 \pm 0.0259$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **2.24x** | 0.0296 | 0.0664 | 0.0729 | 0.135 | $0.0292 \pm 0.0029$ | $0.0685 \pm 0.0072$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.02x** | 0.0381 | 0.0769 | 0.0833 | 0.148 | $0.0386 \pm 0.0037$ | $0.0791 \pm 0.0090$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **2.87x** | 0.0952 | 0.273 | 0.140 | 0.355 | $0.0971 \pm 0.0143$ | $0.279 \pm 0.0320$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.38x** | 0.357 | 1.208 | 0.543 | 1.565 | $0.370 \pm 0.0462$ | $1.216 \pm 0.127$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.62x** | 1.397 | 5.065 | 2.061 | 6.730 | $1.439 \pm 0.152$ | $5.136 \pm 0.361$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.93x** | 0.767 | 1.481 | 1.317 | 2.800 | $0.779 \pm 0.0268$ | $1.474 \pm 0.0683$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.63x** | 5.011 | 8.160 | 5.216 | 9.560 | $5.905 \pm 1.930$ | $8.896 \pm 2.588$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.65x** | 0.298 | 0.787 | 0.655 | 2.142 | $0.294 \pm 0.0170$ | $0.800 \pm 0.0535$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.23x** | 0.426 | 0.949 | 0.771 | 2.179 | $0.412 \pm 0.0290$ | $0.948 \pm 0.0520$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **3.91x** | 6.942 | 27.110 | 8.780 | 33.193 | $6.765 \pm 0.572$ | $27.297 \pm 2.163$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **3.88x** | 33.849 | 131.378 | 42.247 | 164.230 | $34.224 \pm 1.613$ | $131.911 \pm 8.430$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 7.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSlice64`, `InitReducedRow64`, and `Min2ReducedRow64`)

In [`assign/solver.go`](../assign/solver.go), [`simd.AllAbsLE64With`](../internal/simd/kernels_amd64.go) validates finite cost bounds across the input matrix via 8-lane AVX2 `Abs().LessEqual` checks, [`simd.MinReducedSlice64With`](../internal/simd/kernels_amd64.go) accelerates reduction-transfer slack scans in `preprocessSquareJV`, [`simd.Min2ReducedRow64With`](../internal/simd/kernels_amd64.go) accelerates the first/second reduced-minimum scans in `preprocessRectangularBV` and `augmentingRowReduction` via 8-way AVX2 `VSUBPD` and `VMINPD`/`VCMPPD`/`VMOVMSKPD` bitmask filtering when $\max(m, n) \ge 64$, and [`simd.InitReducedRow64With`](../internal/simd/kernels_amd64.go) runs on a slice of length $\max(m, n)$ at the start of each Dijkstra augmentation when $\max(m, n) \ge 32$. All vector kernels clean upper `YMM` state via `StoreArray` + `ClearAVXUpperBits()` (`VZEROUPPER`) before scalar operations, yielding consistent end-to-end speedups across medium, large, and rectangular instances.

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32`, no AVX2 row-reduction kernel runs): 1 of 4 have a 95% interval excluding 1.
- **Instances where AVX2 row kernels run** (`max(rows, cols) >= 32`): 17 faster, 0 slower, 1 with no detectable effect, out of 18.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 row kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.15x [1.07x-1.23x] | 0.0004 | 0.0003 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source | 1.12x [0.99x-1.25x] | 0.0011 | 0.0011 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source | 0.98x [0.83x-1.17x] | 0.0035 | 0.0034 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | SIMD faster | 1.05x [1.02x-1.09x] | 0.0025 | 0.0023 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | no detectable effect | 1.00x [0.81x-1.24x] | 0.0031 | 0.0028 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | no (control) | identical source | 0.95x [0.88x-1.02x] | 0.0065 | 0.0065 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.06x [1.02x-1.09x] | 0.0289 | 0.0272 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.21x [1.17x-1.25x] | 0.0574 | 0.0484 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.33x [1.28x-1.38x] | 0.0464 | 0.0362 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.10x [1.06x-1.14x] | 0.144 | 0.125 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.11x [1.04x-1.18x] | 0.0531 | 0.0482 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.22x [1.09x-1.37x] | 0.0296 | 0.0254 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.06x [1.02x-1.11x] | 0.0381 | 0.0367 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.21x [1.14x-1.28x] | 0.0952 | 0.0793 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.35x [1.29x-1.43x] | 0.357 | 0.258 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.41x [1.33x-1.50x] | 1.397 | 0.986 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.47x [1.38x-1.56x] | 0.767 | 0.538 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.20x [1.01x-1.43x] | 5.011 | 4.439 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.89x [1.81x-1.98x] | 0.298 | 0.158 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.14x [1.10x-1.19x] | 0.426 | 0.366 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.34x [1.27x-1.41x] | 6.942 | 5.291 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.24x [1.17x-1.31x] | 33.849 | 28.560 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

---

## 8. Keeping Documentation and Benchmarks Synchronized

1. **Regenerate telemetry** from a clean, committed tree so that every entry records its `git_commit`: run the commands in the [README](../README.md#verification-benchmarking-and-documentation-sync-commands).
2. **Regenerate tables**: `go run ./cmd/goblin-bench -sync-docs`.
3. **Verify**: `go run ./cmd/goblin-bench -check-docs`, or `go test ./...`, which runs `TestDocumentationAndBenchmarkSync`. The gate validates corpus dimensions, parity, and provenance, and checks every generated block in `README.md`, `docs/benchmarks.md`, and `bench/results/final_report.md`.
