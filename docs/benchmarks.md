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
- **goblin telemetry source**: fingerprint `f409647e4eddfdee` (commit `ff6e5230482dce42de75d010b475c088846121a8`), `go1.27.1-X:nodwarf5`; the documentation gate rejects telemetry whose fingerprint differs from the checked-in sources.
- **CPU affinity**: goblin `2`; Python + CBC `2`.
- **Python reference**: PuLP `4.2.0`, CBC `2.10.3` (`cbc`).
- **CBC subprocess spawn floor**: 1.53 ms median (`prob.solve()` on a 1-variable MILP); the "Net of Spawn" columns subtract it.
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `master` (commit `c1c171ec`, vendored in `third_party/Cbc`), linked in-process through `cgo` (`goblin.WithCBC()`). The C++ backend maps default settings onto CBC with two deliberate differences: `MaxCutPasses = 12` becomes `passC = 30` and `MaxNodes = 50000` becomes unlimited. It is therefore not a like-for-like setup and not the CBC 2.10.3 binary used by the Python baseline.
<!-- END GENERATED:PROVENANCE -->

- **Single-threaded, sequential, pinned execution**: every solve runs single-threaded (`threads=1` in CBC, one goroutine in `goblin.SolveCompiled`), all harnesses run with `workers=1`, and each harness is pinned with `taskset` to one performance core. The i9-13900K mixes 5.5 GHz P-cores (CPUs 0-13) with 4.3 GHz E-cores (CPUs 14-29); unpinned runs migrate between them and produced spurious 2x differences between builds on models where no AVX2 kernel runs. The recorded affinity appears in the provenance block.
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
| Tier 1: Canonical (`N <= 26`) | 10 | **54.8x** | 1.8x | 0.09x | 0.05 | 0.51 | 2.50 | 0.08 | 88.8x [6.0x-161.2x] |
| Tier 2: Target p90 (`30 <= N <= 84`) | 13 | **5.8x** | 4.5x | 0.31x | 1.89 | 6.03 | 11.03 | 8.43 | 6.7x [0.9x-18.3x] |
| Tier 3: Large Tail (`145 <= N <= 4036`) | 8 | **1.8x** | 1.7x | 0.84x | 33.78 | 40.26 | 59.90 | 56.86 | 1.6x [0.5x-7.4x] |
| Tier 3 non-MAD MILPs | 4 | **2.2x** | 2.2x | 0.73x | 41.21 | 56.63 | 92.48 | 90.63 | 2.4x [0.7x-7.4x] |
| Planted-gap MAD family (zero integrality gap by construction) | 8 | **3.8x** | 2.9x | 0.41x | 3.29 | 8.11 | 12.66 | 9.57 | 4.9x [0.5x-20.8x] |
| Random-weight MAD (negative controls) | 3 | **2.6x** | 2.5x | 0.92x | 22.34 | 24.36 | 58.88 | 56.80 | 3.6x [0.9x-6.0x] |
| **All families except planted-gap MAD** | 23 | **11.8x** | 2.5x | 0.23x | 0.84 | 3.60 | 9.93 | 2.07 | 9.5x [0.7x-161.2x] |
| All corpus families | 31 | **8.8x** | 2.6x | 0.27x | 1.20 | 4.44 | 10.57 | 3.08 | 7.4x [0.5x-161.2x] |
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
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **20.8x** | 0.18 | 3.82 | 0.08 | 3.63 | 2.10 | 0.110 | 0.246 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **6.0x** | 4.12 | 24.75 | 3.99 | 24.53 | 22.99 | 0.097 | 0.281 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **16.6x** | 0.21 | 3.43 | 0.09 | 3.18 | 1.65 | 0.116 | 0.329 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **6.7x** | 0.63 | 4.21 | 0.38 | 3.83 | 2.30 | 0.225 | 0.486 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **5.3x** | 0.99 | 5.28 | 0.60 | 4.71 | 3.17 | 0.293 | 0.740 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **3.6x** | 12.14 | 43.22 | 12.00 | 42.91 | 41.38 | 0.132 | 0.382 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **0.9x** | 222.98 | 190.87 | 222.77 | 190.46 | 188.93 | 0.203 | 0.542 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **4.5x** | 2.19 | 9.85 | 1.66 | 8.88 | 7.35 | 0.537 | 1.308 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **1.7x** | 7.41 | 12.68 | 6.62 | 10.99 | 9.45 | 0.894 | 2.243 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **0.5x** | 88.16 | 46.51 | 82.95 | 37.92 | 36.39 | 4.763 | 10.812 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **1.0x** | 409.76 | 389.97 | 395.05 | 345.93 | 344.40 | 14.753 | 44.695 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Multi-Trial Latency Dispersion (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | $0.22 \pm 0.05$ ($\pm 0.03$) | $3.75 \pm 0.22$ ($\pm 0.15$) | $0.22 \pm 0.05$ ($\pm 0.03$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | $3.45 \pm 1.90$ ($\pm 1.28$) | $22.50 \pm 8.42$ ($\pm 5.66$) | $3.48 \pm 1.92$ ($\pm 1.29$) |
| `t2_mad_g2_i8` | $2 \times 8$ | $0.23 \pm 0.04$ ($\pm 0.03$) | $3.59 \pm 0.32$ ($\pm 0.22$) | $0.24 \pm 0.04$ ($\pm 0.02$) |
| `t2_mad_g3_i9` | $3 \times 9$ | $0.66 \pm 0.16$ ($\pm 0.11$) | $4.20 \pm 0.05$ ($\pm 0.03$) | $0.61 \pm 0.12$ ($\pm 0.08$) |
| `t2_mad_g4_i12` | $4 \times 12$ | $0.97 \pm 0.22$ ($\pm 0.15$) | $5.30 \pm 0.09$ ($\pm 0.06$) | $0.96 \pm 0.29$ ($\pm 0.20$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | $11.58 \pm 6.59$ ($\pm 4.43$) | $31.51 \pm 18.28$ ($\pm 12.28$) | $11.48 \pm 6.55$ ($\pm 4.40$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | $252.88 \pm 190.78$ ($\pm 128.16$) | $166.14 \pm 82.17$ ($\pm 55.20$) | $252.27 \pm 189.42$ ($\pm 127.25$) |
| `t3_mad_g5_i20` | $5 \times 20$ | $2.16 \pm 0.29$ ($\pm 0.36$) | $9.31 \pm 1.08$ ($\pm 1.34$) | $2.22 \pm 0.15$ ($\pm 0.19$) |
| `t3_mad_g6_i30` | $6 \times 30$ | $6.11 \pm 2.67$ ($\pm 3.31$) | $13.63 \pm 2.27$ ($\pm 2.82$) | $6.29 \pm 2.86$ ($\pm 3.55$) |
| `t3_mad_g10_i100` | $10 \times 100$ | $88.80 \pm 21.08$ ($\pm 26.17$) | $47.36 \pm 3.41$ ($\pm 4.24$) | $88.79 \pm 20.25$ ($\pm 25.13$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | $430.75 \pm 110.14$ ($\pm 136.73$) | $463.02 \pm 195.32$ ($\pm 242.49$) | $421.81 \pm 102.95$ ($\pm 127.81$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Optimal Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | **0.08** (1 / 0) | 0.24 (2 / 2) | 0.31 (3) | 0.31 (3) | 0.058 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **3.99** (11 / 2) | 0.99 (21 / 2) | 0.79 (84) | 0.70 (84) | 0.058 | 4601.00 | 4257.00 |
| `t2_mad_g2_i8` | **0.09** (1 / 0) | 0.09 (1 / 0) | 0.08 (1) | 0.07 (1) | 0.071 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.38** (1 / 0) | 1.72 (35 / 3) | 1.66 (18) | 1.28 (15) | 0.179 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.60** (1 / 0) | 3.31 (82 / 4) | 3.58 (11) | 2.85 (26) | 0.269 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **12.00** (47 / 6) | 8.96 (846 / 6) | 2.99 (415) | 4.37 (126) | 0.095 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **222.77** (2101 / 15) | 178.56 (3069 / 15) | 89.87 (1899) | 205.72 (3177) | 0.211 | 7720.00 | 6060.00 |
| `t3_mad_g5_i20` | **1.66** (1 / 0) | 21.73 (123 / 9) | 21.85 (213) | 13.08 (39) | 0.698 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **6.62** (1 / 0) | 24.17 (106 / 7) | 42.60 (136) | 163.73 (180) | 5.039 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **82.95** (1 / 0) | 250.29 (3 / 6, `TimeLimit`) | 250.46 (8, `TimeLimit`) | 250.02 (4, `TimeLimit`) | 79.513 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **395.05** (1 / 0) | 251.59 (0 / 0, `TimeLimit`) | 251.19 (0, `TimeLimit`) | 251.99 (0, `TimeLimit`) | 367.698 | 208000.00 | 208000.00 |
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
| `t1_std_lp` | 3 | 3 | **98.9x** | 0.01 | 1.34 | 0.00 | 1.30 | 0.00 | 0.011 | 0.056 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **90.2x** | 0.01 | 1.34 | 0.00 | 1.26 | 0.00 | 0.011 | 0.056 | `OK` |
| `t1_std_mip` | 3 | 3 | **106.6x** | 0.02 | 2.01 | 0.01 | 1.96 | 0.43 | 0.012 | 0.059 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **100.4x** | 0.02 | 1.91 | 0.00 | 1.87 | 0.33 | 0.012 | 0.057 | `OK` |
| `t1_whiskas` | 6 | 5 | **50.3x** | 0.03 | 1.39 | 0.00 | 1.33 | 0.00 | 0.023 | 0.082 | `OK` |
| `t1_furniture` | 2 | 3 | **87.4x** | 0.03 | 2.23 | 0.02 | 2.18 | 0.65 | 0.011 | 0.057 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **28.8x** | 0.11 | 3.21 | 0.08 | 3.14 | 1.61 | 0.031 | 0.092 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **161.2x** | 0.01 | 1.45 | 0.00 | 1.42 | 0.00 | 0.009 | 0.046 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **20.8x** | 0.18 | 3.82 | 0.08 | 3.63 | 2.10 | 0.110 | 0.246 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **6.0x** | 4.12 | 24.75 | 3.99 | 24.53 | 22.99 | 0.097 | 0.281 | `OK` |
| `t2_mkp_30` | 30 | 6 | **6.3x** | 4.37 | 27.67 | 4.31 | 27.50 | 25.97 | 0.063 | 0.208 | `OK` |
| `t2_mkp_50` | 50 | 8 | **1.0x** | 10.97 | 11.47 | 10.82 | 11.23 | 9.69 | 0.109 | 0.324 | `OK` |
| `t2_cflp_36` | 36 | 11 | **10.9x** | 0.33 | 3.56 | 0.26 | 3.39 | 1.86 | 0.060 | 0.191 | `OK` |
| `t2_cflp_49` | 49 | 13 | **4.0x** | 1.03 | 4.15 | 0.93 | 3.96 | 2.42 | 0.078 | 0.259 | `OK` |
| `t2_gap_45` | 45 | 13 | **9.5x** | 0.90 | 8.48 | 0.81 | 8.28 | 6.75 | 0.077 | 0.241 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **8.4x** | 4.74 | 39.67 | 4.66 | 39.44 | 37.91 | 0.092 | 0.290 | `OK` |
| `t2_binpack_44` | 44 | 14 | **12.3x** | 0.55 | 6.73 | 0.44 | 6.57 | 5.04 | 0.093 | 0.212 | `OK` |
| `t2_setcover_50` | 50 | 20 | **18.3x** | 0.29 | 5.37 | 0.13 | 5.11 | 3.57 | 0.142 | 0.353 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **16.6x** | 0.21 | 3.43 | 0.09 | 3.18 | 1.65 | 0.116 | 0.329 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **6.7x** | 0.63 | 4.21 | 0.38 | 3.83 | 2.30 | 0.225 | 0.486 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **5.3x** | 0.99 | 5.28 | 0.60 | 4.71 | 3.17 | 0.293 | 0.740 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **3.6x** | 12.14 | 43.22 | 12.00 | 42.91 | 41.38 | 0.132 | 0.382 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **0.9x** | 222.98 | 190.87 | 222.77 | 190.46 | 188.93 | 0.203 | 0.542 | `OK` |
| `t3_mkp_200` | 200 | 8 | **0.7x** | 61.18 | 41.23 | 60.88 | 40.62 | 39.09 | 0.312 | 0.782 | `OK` |
| `t3_cflp_300` | 300 | 34 | **7.4x** | 28.36 | 211.02 | 27.74 | 210.13 | 208.60 | 0.440 | 1.147 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **3.4x** | 38.18 | 128.49 | 37.40 | 126.64 | 125.11 | 0.764 | 2.268 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **1.5x** | 43.52 | 65.45 | 42.30 | 62.22 | 60.69 | 1.240 | 3.667 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **4.5x** | 2.19 | 9.85 | 1.66 | 8.88 | 7.35 | 0.537 | 1.308 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **1.7x** | 7.41 | 12.68 | 6.62 | 10.99 | 9.45 | 0.894 | 2.243 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **0.5x** | 88.16 | 46.51 | 82.95 | 37.92 | 36.39 | 4.763 | 10.812 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **1.0x** | 409.76 | 389.97 | 395.05 | 345.93 | 344.40 | 14.753 | 44.695 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## 5. Default Build vs. `GOEXPERIMENT=simd`

Each ratio is the geometric mean of per-trial `scalar / SIMD` solve times over the same instances, with a 95% $t$-interval on the log ratios; "no detectable effect" means the interval contains 1. "Reachable" is only a necessary condition: the AVX2 `Dot64`/`Axpy64` paths need a slice of length 256 or more, which requires `n + m >= 256`. Below that size no AVX2 kernel can run, so those instances are negative controls: an interval there that excludes 1 measures differences between the two binaries or between runs, not SIMD.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 256`, no AVX2 kernel can run): 2 of 25 have a 95% interval excluding 1. These differences come from the two binaries (compiler output and code layout differ under `GOEXPERIMENT=simd`) and from run-to-run effects, not from SIMD kernels. Intervals of similar size on the AVX2-capable instances are therefore not evidence of a SIMD effect.
- **Instances where AVX2 can run**: 0 faster, 0 slower, 6 with no detectable effect, out of 6.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 1.02x [0.86x-1.22x] | 0.002 | 0.002 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 1.04x [0.93x-1.17x] | 0.001 | 0.001 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 0.98x [0.92x-1.05x] | 0.006 | 0.006 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source | 1.08x [0.87x-1.34x] | 0.004 | 0.005 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 1.12x [0.98x-1.27x] | 0.003 | 0.003 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 0.95x [0.88x-1.02x] | 0.015 | 0.015 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source | 1.00x [0.94x-1.07x] | 0.083 | 0.085 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 1.00x [0.96x-1.05x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | no (control) | identical source | 1.01x [0.98x-1.04x] | 0.077 | 0.077 | 11 |
| `t1_mad_rand_g2_i4` | 60 | no (control) | identical source | 1.01x [0.97x-1.04x] | 3.989 | 4.137 | 11 |
| `t2_mkp_30` | 36 | no (control) | identical source | 0.93x [0.84x-1.02x] | 4.306 | 4.467 | 11 |
| `t2_mkp_50` | 58 | no (control) | identical source | 0.99x [0.95x-1.04x] | 10.819 | 10.386 | 11 |
| `t2_cflp_36` | 47 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.93x [0.86x-1.00x] | 0.263 | 0.270 | 11 |
| `t2_cflp_49` | 62 | no (control) | identical source | 0.95x [0.80x-1.14x] | 0.934 | 0.904 | 11 |
| `t2_gap_45` | 58 | no (control) | identical source | 1.01x [0.93x-1.10x] | 0.807 | 0.654 | 11 |
| `t2_lotsizing_48` | 80 | no (control) | identical source | 0.99x [0.92x-1.08x] | 4.662 | 4.652 | 11 |
| `t2_binpack_44` | 58 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.13x [1.03x-1.24x] | 0.443 | 0.384 | 11 |
| `t2_setcover_50` | 70 | no (control) | identical source | 1.04x [0.98x-1.10x] | 0.131 | 0.121 | 11 |
| `t2_mad_g2_i8` | 72 | no (control) | identical source | 0.98x [0.93x-1.03x] | 0.090 | 0.089 | 11 |
| `t2_mad_g3_i9` | 108 | no (control) | identical source | 1.06x [0.98x-1.15x] | 0.380 | 0.362 | 11 |
| `t2_mad_g4_i12` | 156 | no (control) | identical source | 1.00x [0.91x-1.11x] | 0.601 | 0.604 | 11 |
| `t2_mad_rand_g2_i8` | 72 | no (control) | identical source | 1.01x [0.99x-1.03x] | 12.001 | 11.506 | 11 |
| `t2_mad_rand_g3_i9` | 108 | no (control) | identical source | 1.00x [0.99x-1.02x] | 222.769 | 218.190 | 11 |
| `t3_mkp_200` | 208 | no (control) | identical source | 0.99x [0.96x-1.02x] | 60.879 | 63.419 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.05x [0.94x-1.18x] | 27.740 | 27.245 | 5 |
| `t3_lotsizing_504` | 840 | possible | no detectable effect | 0.99x [0.96x-1.02x] | 37.396 | 38.821 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 1.03x [0.96x-1.09x] | 42.299 | 41.388 | 5 |
| `t3_mad_g5_i20` | 240 | no (control) | identical source | 0.97x [0.81x-1.17x] | 1.663 | 1.635 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 0.97x [0.94x-1.01x] | 6.617 | 7.009 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 0.99x [0.98x-1.01x] | 82.954 | 83.404 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | no detectable effect | 1.02x [1.00x-1.05x] | 395.052 | 382.775 | 5 |
<!-- END GENERATED:SIMD_EFFECT_TABLE -->

---

## 6. Full Corpus Solver-Core Checkpoint Progression

<!-- BEGIN GENERATED:FULL_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Optimal Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_std_lp` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.001 | 54.00 | 54.00 |
| `t1_std_lp_max` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.001 | 80.00 | 80.00 |
| `t1_std_mip` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.00 (3) | 0.001 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.00** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.00 (3) | 0.001 | 64.95 | 58.90 |
| `t1_whiskas` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.02 (1) | 0.01 (1) | 0.001 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.08** (1 / 15) | 0.07 (2 / 15) | 0.09 (24) | 0.12 (31) | 0.006 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (5) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.08** (1 / 0) | 0.24 (2 / 2) | 0.31 (3) | 0.31 (3) | 0.058 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **3.99** (11 / 2) | 0.99 (21 / 2) | 0.79 (84) | 0.70 (84) | 0.058 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **4.31** (29 / 9) | 1.73 (60 / 9) | 0.82 (77) | 0.72 (77) | 0.020 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **10.82** (1 / 5) | 4.23 (2 / 5) | 3.89 (132) | 3.28 (119) | 0.031 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.26** (1 / 2) | 0.88 (3 / 2) | 0.13 (7) | 0.13 (7) | 0.021 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.93** (1 / 20) | 1.41 (4 / 20) | 0.31 (9) | 0.34 (9) | 0.032 | 909.02 | 827.30 |
| `t2_gap_45` | **0.81** (1 / 13) | 0.20 (1 / 13) | 1.49 (61) | 1.79 (61) | 0.029 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **4.66** (11 / 40) | 4.98 (33 / 40) | 1.75 (198) | 2.07 (174) | 0.062 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **0.44** (1 / 12) | 1.20 (18 / 12) | 16.11 (1895) | 24.02 (2400) | 0.029 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.13** (1 / 6) | 0.23 (2 / 6) | 0.71 (9) | 0.70 (9) | 0.032 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.09** (1 / 0) | 0.09 (1 / 0) | 0.08 (1) | 0.07 (1) | 0.071 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.38** (1 / 0) | 1.72 (35 / 3) | 1.66 (18) | 1.28 (15) | 0.179 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.60** (1 / 0) | 3.31 (82 / 4) | 3.58 (11) | 2.85 (26) | 0.269 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **12.00** (47 / 6) | 8.96 (846 / 6) | 2.99 (415) | 4.37 (126) | 0.095 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **222.77** (2101 / 15) | 178.56 (3069 / 15) | 89.87 (1899) | 205.72 (3177) | 0.211 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **60.88** (540 / 21) | 53.54 (739 / 21) | 97.86 (5000, `NodeLimit`) | 80.03 (5000, `NodeLimit`) | 0.147 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **27.74** (9 / 37) | 15.22 (39 / 37) | 5.93 (134) | 6.07 (134) | 0.133 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **37.40** (11 / 165) | 86.96 (365 / 165) | 220.83 (5000, `NodeLimit`) | 212.94 (5000, `NodeLimit`) | 1.291 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **42.30** (17 / 50) | 10.52 (96 / 50) | 214.09 (5000, `NodeLimit`) | 156.11 (5000, `NodeLimit`) | 0.157 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **1.66** (1 / 0) | 21.73 (123 / 9) | 21.85 (213) | 13.08 (39) | 0.698 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **6.62** (1 / 0) | 24.17 (106 / 7) | 42.60 (136) | 163.73 (180) | 5.039 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **82.95** (1 / 0) | 250.29 (3 / 6, `TimeLimit`) | 250.46 (8, `TimeLimit`) | 250.02 (4, `TimeLimit`) | 79.513 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **395.05** (1 / 0) | 251.59 (0 / 0, `TimeLimit`) | 251.19 (0, `TimeLimit`) | 251.99 (0, `TimeLimit`) | 367.698 | 208000.00 | 208000.00 |
<!-- END GENERATED:FULL_CHECKPOINT_TABLE -->

---

## 7. Keeping Documentation and Benchmarks Synchronized

1. **Regenerate telemetry** from a clean, committed tree so that every entry records its `git_commit`: run the commands in the [README](../README.md#verification-benchmarking-and-documentation-sync-commands).
2. **Regenerate tables**: `go run ./cmd/goblin-bench -sync-docs`.
3. **Verify**: `go run ./cmd/goblin-bench -check-docs`, or `go test ./...`, which runs `TestDocumentationAndBenchmarkSync`. The gate validates corpus dimensions, parity, and provenance, and checks every generated block in `README.md`, `docs/benchmarks.md`, and `bench/results/final_report.md`.
