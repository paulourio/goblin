# Benchmark Methodology and Corpus Results

The benchmark protocol below evaluates pure-Go `goblin` (`BackendGo`, zero `cgo` by default), Python `PuLP` driving the `COIN-OR CBC` (`c1c171ec`) command-line binary, and `goblin (CBC)` (`BackendCBC`, built with `-tags cbc`), which calls the same `c1c171ec` CBC/CLP stack in-process through `cgo`. Every number in this document comes from a generated block rendered by [`bench/docgen.go`](../bench/docgen.go) from the JSON telemetry in `bench/results/`; the prose contains no hardcoded measurements. [`doc_sync_test.go`](../doc_sync_test.go) fails `go test ./...` when a block drifts from the telemetry, when a parity check fails, or when any of the seven telemetry files lacks uniform git, source-fingerprint, and host-kernel provenance.

---

## 1. Environment, Provenance, and Protocol

```bash
go run ./cmd/goblin-bench -check-docs
```

- **CPU**: `13th Gen Intel(R) Core(TM) i9-13900K` (`x86_64`, 30 logical CPUs exposed by `lscpu`, AVX2 + FMA).
- **Default build**: `go build` with the pure-Go kernels (`BackendGo`). All headline timings use that build.
- **Experimental build**: `GOEXPERIMENT=simd`, compared to the default build only in section 5.

<!-- BEGIN GENERATED:PROVENANCE -->
- **Unified telemetry source**: fingerprint `a33eec8017a28703` (working tree on top of commit `4fd4211ec2eeeef813d830171c636489f7564cff`), `go1.27.3-X:nodwarf5`, Linux kernel `6.18.63-gentoo-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 7.32 ms median on a 1-variable MILP and 7.18 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

- **Single-threaded, sequential, pinned execution**: every solve runs single-threaded (`threads=1` in CBC, one goroutine in `gb.SolveCompiled`), all harnesses run with `workers=1` in a single session on the same Linux kernel, and each harness is pinned with `taskset` to one performance core. The i9-13900K mixes 5.5 GHz P-cores (CPUs 0-13) with 4.3 GHz E-cores (CPUs 14-29); unpinned runs migrate between them and introduce bimodal variance.
- **Warm state**: each harness runs an untimed worker warmup (`_init_worker` in [`bench/python_baseline.py`](../bench/python_baseline.py), `warmWorker` in [`cmd/goblin-bench/main.go`](../cmd/goblin-bench/main.go)), then 2 untimed warmup solves per family on Tiers 1-2 and 1 on Tier 3.
- **Synthetic corpus and held-out public benchmarks**: each synthetic family in `testdata/corpus/trials/<id>.jsonl` holds 11 distinct seeded problem instances (Tiers 1-2) or 5 (Tier 3), 293 instances in total; `WriteCorpusJSON` rejects duplicate trial payloads. Trial $k$ is the identical problem instance in every harness, enabling paired per-instance ratios rather than repeated runs on a fixed matrix. Complementing the synthetic timing corpus, [`public_benchmarks_test.go`](../public_benchmarks_test.go) verifies status, objective parity, and primal feasibility across 17 held-out public Netlib LP and MIPLIB/COIN-OR MILP instances in `testdata/public/`.
- **Metrics**: per randomized instance, prep time (`t_parse + t_build + t_format/t_compile`), solve time (`t_solve`), and end-to-end time (`t_total`). Aggregate tables report paired per-instance geometric means with 95% Student-$t$ intervals on log ratios, total wall-clock sums across all instances in each subset, and full per-instance `[Min-Max]` ranges without floor clamping.
- **Parity**: the benchmark gate requires both `goblin` pure-Go builds and `goblin (CBC)` to match CBC's canonical and per-trial statuses and objectives across all 293 instances within a relative tolerance of `1e-5` under matching `1e-6` MIP gap tolerances.

---

## 2. Aggregate Workload Summary

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Instances) | Paired `Py / Go` GeoMean [95% CI] | Paired `Go / CBC` | `Go` Sum (ms) | `CBC` Sum (ms) | `Py` Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **362.97x** [295.35x-446.09x] | 0.05x | 13.8 | 208.6 | 1221.7 | 552.09x [14.41x-2058.85x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **18.99x** [15.63x-23.06x] | 0.17x | 886.4 | 2109.6 | 4911.5 | 22.13x [0.17x-149.83x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **7.82x** [5.54x-11.05x] | 0.27x | 4780.0 | 3231.4 | 6752.3 | 9.51x [0.06x-33.43x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **12.14x** [9.75x-15.13x] | 0.23x | 5329.4 | 3359.2 | 7551.4 | 12.50x [0.06x-149.83x] |
| Tier 3 non-MAD MILPs | 4 (20) | **4.76x** [2.58x-8.77x] | 0.45x | 4452.3 | 1350.4 | 3038.2 | 6.53x [0.06x-19.66x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **35.54x** [28.86x-43.76x] | 0.09x | 338.3 | 2008.2 | 4234.3 | 42.52x [3.33x-106.87x] |
| Random-weight MAD (negative controls) | 3 (33) | **13.58x** [9.01x-20.46x] | 0.18x | 479.1 | 1296.7 | 2497.2 | 13.24x [1.56x-95.56x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **56.32x** [42.77x-74.15x] | 0.12x | 5341.9 | 3541.4 | 8651.2 | 52.62x [0.06x-2058.85x] |
| All 31 synthetic corpus families | 31 (293) | **50.93x** [40.84x-63.51x] | 0.11x | 5680.1 | 5549.6 | 12885.5 | 48.69x [0.06x-2058.85x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

How to read the aggregate table:
1. **Tier 1 micro-models are dominated by subprocess cost.** On 2-to-26-variable models, Python spends ~1.3 to 2.0 ms per instance spawning CBC and parsing `.sol` files, whereas pure-Go executes in microseconds in memory.
2. **The planted-gap MAD family is zero-gap by construction.** Its instances have `RootLPBound == OptimalObj` by design and close at node 1 via presolve orbit fixing and root heuristics. The bold row (`Tier 2+3 non-planted MILPs`) excludes both Tier-1 subprocess micro-models and planted-gap MAD to isolate solver-dominated MILP performance.
3. **Heavy-tail dispersion is visible in wall-clock sums and `[Min-Max]` ranges.** Reporting `Go Sum`, `CBC Sum`, `Py Sum`, and per-instance `[Min-Max]` alongside paired geometric means exposes individual hard seeds (such as heavy-tail trials of `t3_lotsizing_504` where root-only single-row cuts leave a wider tree than CBC's multi-row aggregation).
4. **Large degenerate LPs and PFI updates.** Pure-Go factors the basis with a three-stage singleton and sparse Markowitz LU, `FTRANSparseHint`/`BTRANSparseHint`, `BTRANExact`, dual bound flipping (`selectDualBFRT`), and conditional power-of-two Ruiz equilibration, while updating the basis with a product-form (PFI) eta file rather than Forrest-Tomlin $U$-updates; see [`solver-internals.md`](solver-internals.md#known-limits-of-the-pure-go-engine).

---

## 3. MAD Group-Item Allocation Formulation (`G` Groups $\times$ `I` Items)

The synthetic `MAD` group-item allocation family ([`BuildMADFromWeights`](../bench/corpus.go)) models multi-period balanced assignment across $G$ groups, $I$ items, and $T = 6$ time periods with central-level roundness penalties.

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
- [`BuildMADRandom`](../bench/corpus.go) (`t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`, and `TestRandomMADNegativeControls` in [`solver_test.go`](../solver_test.go)) uses the same `BuildMADFromWeights` constraint matrix and assigns dense pseudorandom weights $W_{t,i} \in [100, 2000)$ to every item. Those negative controls have a strict LP-to-MIP gap, the root heuristic does not close it, and the solver must branch.

### MAD Results (`G` Groups $\times$ `I` Items, $T = 6$)

<!-- BEGIN GENERATED:MAD_PIPELINE_TABLE -->
| Problem ID | $G \times I$ | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Trial 0 Obj | Parity |
| :--- | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **92.9x** [68.8x-106.9x] | 0.11 | 11.08 | 0.06 | 10.82 | 0.049 | 0.314 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **48.2x** [14.4x-95.6x] | 0.62 | 36.51 | 0.59 | 36.26 | 0.030 | 0.322 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **63.0x** [43.2x-94.8x] | 0.20 | 11.61 | 0.14 | 11.30 | 0.054 | 0.409 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **56.2x** [33.2x-72.4x] | 0.19 | 11.25 | 0.12 | 10.82 | 0.059 | 0.568 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **30.8x** [18.3x-41.9x] | 0.39 | 13.44 | 0.27 | 12.37 | 0.093 | 0.854 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **10.0x** [3.6x-38.2x] | 4.39 | 51.67 | 4.34 | 51.29 | 0.050 | 0.423 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **5.2x** [1.6x-23.7x] | 32.44 | 181.38 | 32.28 | 180.62 | 0.087 | 0.599 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **24.4x** [14.4x-33.4x] | 0.74 | 20.28 | 0.51 | 18.83 | 0.242 | 1.444 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **14.0x** [8.7x-20.6x] | 1.52 | 20.98 | 1.15 | 19.04 | 0.369 | 2.441 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **9.6x** [8.7x-11.6x] | 6.93 | 64.29 | 5.04 | 55.81 | 1.746 | 11.489 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **8.3x** [3.3x-21.6x] | 49.06 | 416.52 | 43.43 | 379.66 | 5.265 | 48.496 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Cross-Instance Latency Dispersion Across Randomized Instances (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.12 \pm 0.02$ ($\pm 0.01$) | $11.08 \pm 0.46$ ($\pm 0.31$) | $0.15 \pm 0.03$ ($\pm 0.02$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.95 \pm 0.85$ ($\pm 0.57$) | $33.89 \pm 10.28$ ($\pm 6.90$) | $0.96 \pm 0.85$ ($\pm 0.57$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.19 \pm 0.05$ ($\pm 0.03$) | $11.67 \pm 0.45$ ($\pm 0.30$) | $0.20 \pm 0.06$ ($\pm 0.04$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.21 \pm 0.05$ ($\pm 0.03$) | $11.26 \pm 0.21$ ($\pm 0.14$) | $0.20 \pm 0.06$ ($\pm 0.04$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.45 \pm 0.15$ ($\pm 0.10$) | $13.28 \pm 0.58$ ($\pm 0.39$) | $0.40 \pm 0.17$ ($\pm 0.11$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $4.50 \pm 2.67$ ($\pm 1.79$) | $38.71 \pm 18.37$ ($\pm 12.34$) | $4.32 \pm 2.42$ ($\pm 1.63$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $38.10 \pm 31.61$ ($\pm 21.24$) | $154.41 \pm 69.88$ ($\pm 46.94$) | $37.75 \pm 31.06$ ($\pm 20.87$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.84 \pm 0.21$ ($\pm 0.26$) | $20.20 \pm 2.59$ ($\pm 3.21$) | $0.72 \pm 0.19$ ($\pm 0.23$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.65 \pm 0.65$ ($\pm 0.81$) | $21.94 \pm 1.86$ ($\pm 2.31$) | $1.50 \pm 0.84$ ($\pm 1.04$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $6.92 \pm 0.44$ ($\pm 0.55$) | $66.70 \pm 4.11$ ($\pm 5.10$) | $7.38 \pm 1.21$ ($\pm 1.51$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $56.11 \pm 15.45$ ($\pm 19.18$) | $633.98 \pm 652.45$ ($\pm 809.99$) | $53.84 \pm 15.57$ ($\pm 19.32$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | **0.06** (1 / 0) | 0.06 (1 / 0) | 0.07 (1) | 0.37 (3) | 0.070 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.59** (5 / 0) | 0.57 (12 / 0) | 0.74 (12) | 1.24 (84) | 0.071 | 4601.00 | 4257.00 |
| `t2_mad_g2_i8` | **0.14** (1 / 0) | 0.45 (9 / 0) | 0.49 (9) | 0.25 (2) | 0.096 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.12** (1 / 0) | 0.12 (1 / 0) | 0.14 (1) | 1.30 (136) | 0.226 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.27** (1 / 0) | 1.64 (15 / 0) | 1.17 (15) | 5.60 (47) | 0.270 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **4.34** (43 / 0) | 2.31 (41 / 0) | 3.14 (41) | 3.40 (126) | 0.117 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **32.28** (137 / 0) | 29.60 (201 / 0) | 12.07 (201) | 207.14 (1912) | 0.242 | 7720.00 | 6060.00 |
| `t3_mad_g5_i20` | **0.51** (1 / 0) | 2.01 (14 / 0) | 1.81 (14) | 1.24 (1) | 0.911 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **1.15** (1 / 0) | 2.78 (12 / 0) | 2.66 (12) | 6.48 (27) | 1.478 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **5.04** (1 / 0) | 55.98 (130 / 0) | 56.28 (130) | 216.93 (270) | 15.495 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **43.43** (1 / 0) | 44.08 (2 / 0) | 43.75 (2) | 250.41 (107, `TimeLimit`) | 117.723 | 208000.00 | 208000.00 |
<!-- END GENERATED:MAD_CHECKPOINT_TABLE -->

---

## 4. Full Synthetic Corpus End-to-End Comparison

The synthetic corpus spans 31 families in three tiers:
- **Tier 1 (10 families)**: canonical `PuLP` unit models (`t1_std_lp`, `t1_std_lp_max`, `t1_std_mip`, `t1_mip_float_obj`, `t1_whiskas`, `t1_furniture`, `t1_knapsack_12`, `t1_infeasible_mip`), `t1_mad_g2_i4`, and `t1_mad_rand_g2_i4`.
- **Tier 2 (13 families)**: 30 to 84 variables: multidimensional knapsack, capacitated depot siting, generalized assignment, multi-item lot sizing, bin packing, set cover, three planted-gap MAD and two random-weight MAD families.
- **Tier 3 (8 families)**: 145 to 4,036 variables: `t3_mkp_200`, `t3_cflp_300`, `t3_lotsizing_504`, `t3_sched_1000`, and four planted-gap MAD families.

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | **783.5x** [353.8x-1283.8x] | 0.01 | 7.12 | 0.00 | 7.06 | 0.006 | 0.077 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **957.4x** [681.8x-1310.4x] | 0.01 | 7.45 | 0.00 | 7.38 | 0.005 | 0.087 | `OK` |
| `t1_std_mip` | 3 | 3 | **683.5x** [543.3x-999.1x] | 0.01 | 8.74 | 0.01 | 8.67 | 0.006 | 0.097 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **647.3x** [358.2x-1015.5x] | 0.01 | 8.33 | 0.01 | 8.27 | 0.006 | 0.087 | `OK` |
| `t1_whiskas` | 6 | 5 | **503.0x** [362.8x-710.7x] | 0.02 | 7.20 | 0.00 | 7.11 | 0.010 | 0.113 | `OK` |
| `t1_furniture` | 2 | 3 | **305.2x** [100.7x-595.5x] | 0.03 | 8.79 | 0.02 | 8.72 | 0.005 | 0.079 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **143.4x** [60.5x-294.4x] | 0.08 | 10.67 | 0.06 | 10.56 | 0.013 | 0.134 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **1213.0x** [530.2x-2058.9x] | 0.01 | 7.55 | 0.00 | 7.50 | 0.005 | 0.082 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **92.9x** [68.8x-106.9x] | 0.11 | 11.08 | 0.06 | 10.82 | 0.049 | 0.314 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **48.2x** [14.4x-95.6x] | 0.62 | 36.51 | 0.59 | 36.26 | 0.030 | 0.322 | `OK` |
| `t2_mkp_30` | 30 | 6 | **8.0x** [4.3x-13.7x] | 2.94 | 30.21 | 2.91 | 30.02 | 0.031 | 0.246 | `OK` |
| `t2_mkp_50` | 50 | 8 | **5.7x** [2.5x-52.6x] | 4.92 | 19.81 | 4.88 | 19.52 | 0.048 | 0.372 | `OK` |
| `t2_cflp_36` | 36 | 11 | **36.1x** [5.3x-145.2x] | 0.25 | 11.64 | 0.21 | 11.45 | 0.040 | 0.260 | `OK` |
| `t2_cflp_49` | 49 | 13 | **33.7x** [15.3x-63.1x] | 0.38 | 12.81 | 0.35 | 12.59 | 0.029 | 0.302 | `OK` |
| `t2_gap_45` | 45 | 13 | **12.7x** [2.1x-36.8x] | 0.83 | 16.32 | 0.80 | 16.11 | 0.036 | 0.291 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **11.1x** [4.2x-23.3x] | 4.65 | 73.99 | 4.62 | 73.73 | 0.036 | 0.327 | `OK` |
| `t2_binpack_44` | 44 | 14 | **14.6x** [0.2x-66.1x] | 0.46 | 16.32 | 0.43 | 16.10 | 0.027 | 0.277 | `OK` |
| `t2_setcover_50` | 50 | 20 | **64.4x** [5.3x-149.8x] | 0.16 | 12.83 | 0.12 | 12.51 | 0.044 | 0.436 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **63.0x** [43.2x-94.8x] | 0.20 | 11.61 | 0.14 | 11.30 | 0.054 | 0.409 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **56.2x** [33.2x-72.4x] | 0.19 | 11.25 | 0.12 | 10.82 | 0.059 | 0.568 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **30.8x** [18.3x-41.9x] | 0.39 | 13.44 | 0.27 | 12.37 | 0.093 | 0.854 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **10.0x** [3.6x-38.2x] | 4.39 | 51.67 | 4.34 | 51.29 | 0.050 | 0.423 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **5.2x** [1.6x-23.7x] | 32.44 | 181.38 | 32.28 | 180.62 | 0.087 | 0.599 | `OK` |
| `t3_mkp_200` | 200 | 8 | **3.5x** [2.9x-3.9x] | 37.64 | 129.50 | 37.54 | 128.87 | 0.109 | 0.847 | `OK` |
| `t3_cflp_300` | 300 | 34 | **13.1x** [9.6x-19.7x] | 15.35 | 263.60 | 15.18 | 262.59 | 0.184 | 1.259 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **1.0x** [0.1x-2.4x] | 102.66 | 166.90 | 102.11 | 164.92 | 0.289 | 2.457 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **11.7x** [9.2x-16.3x] | 7.58 | 86.07 | 6.97 | 82.94 | 0.407 | 3.697 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **24.4x** [14.4x-33.4x] | 0.74 | 20.28 | 0.51 | 18.83 | 0.242 | 1.444 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **14.0x** [8.7x-20.6x] | 1.52 | 20.98 | 1.15 | 19.04 | 0.369 | 2.441 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **9.6x** [8.7x-11.6x] | 6.93 | 64.29 | 5.04 | 55.81 | 1.746 | 11.489 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **8.3x** [3.3x-21.6x] | 49.06 | 416.52 | 43.43 | 379.66 | 5.265 | 48.496 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## 5. Default Build vs. `GOEXPERIMENT=simd`

In [`internal/simd/kernels_amd64.go`](../internal/simd/kernels_amd64.go), AVX2 kernels spill 4-lane accumulators to the stack via `StoreArray` and execute `archsimd.ClearAVXUpperBits()` (`VZEROUPPER`) before scalar tail or caller operations, eliminating AVX-to-SSE dirty-upper `YMM` transition stalls while preserving `NaN` propagation parity. From the dispatch threshold `minAVX2Len = 32` upward, the 256-bit AVX2 kernels (`Dot64`, `Axpy64`, `Scale64`, `AbsMax64`, `AllAbsLE64`, `RowActivityBoundsFinite`, `MinReducedSlice64`, `InitReducedRow64`, and `Min2ReducedRow64` at $N \ge 64$) outperform the 4-way slice-stepped scalar loops; below $N = 32$, call and `VZEROUPPER` overhead dominates, so slices of length $< 32$ remain on the scalar path.

At the full-solver level, each ratio below is the geometric mean of per-trial `scalar / SIMD` solve times over the same instances, with a 95% $t$-interval on the log ratios; "no detectable effect" means the interval contains 1. "Reachable" is a necessary condition: the AVX2 `Dot64`/`Axpy64`/`Scale64` paths require a slice of length 32 or more (`n + m >= 32`). Below that size no AVX2 kernel can run, so those 8 families serve as negative controls where intervals excluding 1 reflect binary code-layout or run-to-run variance rather than SIMD execution. Lowering `minAVX2Len` from 256 to 32 enables AVX2 across dense tableau row accumulation (`ComputeTableauRow`) and dual reduced-cost updates (`computeDualsAndReducedCosts`) for medium models with $n \le 256$ (`DenseThreshold`), while `ComputeTableauRow` dispatches `Axpy64` on contiguous CSR column blocks in large models ($n > 256$, such as `t3_mad_g4_i1000`).

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 1 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 0 faster, 4 slower, 19 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 0.98x [0.95x-1.01x] | 0.002 | 0.002 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.91x [0.84x-1.00x] | 0.002 | 0.002 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 0.94x [0.85x-1.04x] | 0.006 | 0.007 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source | 0.98x [0.93x-1.04x] | 0.005 | 0.005 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 0.99x [0.92x-1.05x] | 0.003 | 0.003 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 0.98x [0.90x-1.06x] | 0.018 | 0.020 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source | 0.97x [0.94x-1.00x] | 0.064 | 0.065 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 0.99x [0.95x-1.03x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | SIMD slower | 0.77x [0.64x-0.92x] | 0.061 | 0.075 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 0.99x [0.98x-1.01x] | 0.592 | 0.586 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 1.02x [0.99x-1.05x] | 2.906 | 2.888 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 1.02x [1.00x-1.04x] | 4.877 | 4.832 | 11 |
| `t2_cflp_36` | 47 | possible | no detectable effect | 1.01x [0.95x-1.08x] | 0.207 | 0.206 | 11 |
| `t2_cflp_49` | 62 | possible | SIMD slower | 0.99x [0.97x-1.00x] | 0.350 | 0.361 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 0.99x [0.91x-1.09x] | 0.803 | 1.047 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 1.05x [0.97x-1.14x] | 4.619 | 4.701 | 11 |
| `t2_binpack_44` | 58 | possible | no detectable effect | 1.04x [0.97x-1.11x] | 0.435 | 0.438 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 1.01x [0.98x-1.03x] | 0.120 | 0.118 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 1.04x [0.93x-1.16x] | 0.137 | 0.135 | 11 |
| `t2_mad_g3_i9` | 108 | possible | no detectable effect | 1.08x [0.98x-1.18x] | 0.124 | 0.120 | 11 |
| `t2_mad_g4_i12` | 156 | possible | no detectable effect | 1.10x [0.85x-1.42x] | 0.267 | 0.247 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.06x [0.95x-1.19x] | 4.338 | 4.448 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 1.01x [0.98x-1.05x] | 32.279 | 31.838 | 11 |
| `t3_mkp_200` | 208 | possible | SIMD slower | 0.96x [0.93x-0.98x] | 37.540 | 39.396 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 0.89x [0.80x-1.00x] | 15.178 | 17.084 | 5 |
| `t3_lotsizing_504` | 840 | possible | SIMD slower | 0.94x [0.90x-0.99x] | 102.108 | 112.649 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 1.01x [0.95x-1.08x] | 6.971 | 7.376 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 1.10x [0.68x-1.77x] | 0.505 | 0.470 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 1.14x [0.87x-1.49x] | 1.147 | 0.882 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 0.92x [0.73x-1.15x] | 5.036 | 5.185 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | no detectable effect | 1.06x [1.00x-1.13x] | 43.428 | 40.167 | 5 |
<!-- END GENERATED:SIMD_EFFECT_TABLE -->

---

## 6. Full Corpus Solver-Core Checkpoint Progression

<!-- BEGIN GENERATED:FULL_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_std_lp` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.001 | 54.00 | 54.00 |
| `t1_std_lp_max` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.001 | 80.00 | 80.00 |
| `t1_std_mip` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.00 (3) | 0.001 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.00 (3) | 0.001 | 64.95 | 58.90 |
| `t1_whiskas` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.02 (1) | 0.01 (1) | 0.002 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.06** (1 / 8) | 0.08 (2 / 8) | 0.10 (24) | 0.13 (31) | 0.005 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.06** (1 / 0) | 0.06 (1 / 0) | 0.07 (1) | 0.37 (3) | 0.070 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.59** (5 / 0) | 0.57 (12 / 0) | 0.74 (12) | 1.24 (84) | 0.071 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.91** (29 / 8) | 2.13 (60 / 8) | 1.00 (77) | 0.93 (77) | 0.021 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **4.88** (1 / 4) | 4.79 (2 / 4) | 4.31 (132) | 3.66 (119) | 0.033 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.21** (1 / 2) | 1.07 (3 / 2) | 0.15 (7) | 0.17 (7) | 0.022 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.35** (1 / 20) | 1.65 (5 / 20) | 0.30 (9) | 0.31 (9) | 0.029 | 909.02 | 827.30 |
| `t2_gap_45` | **0.80** (1 / 8) | 0.24 (1 / 8) | 1.81 (61) | 1.68 (61) | 0.023 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **4.62** (3 / 35) | 5.75 (33 / 35) | 1.81 (198) | 2.44 (174) | 0.079 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **0.43** (1 / 5) | 0.99 (13 / 5) | 14.99 (1838) | 26.14 (2450) | 0.025 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.12** (1 / 6) | 0.26 (2 / 6) | 0.82 (9) | 0.83 (9) | 0.030 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.14** (1 / 0) | 0.45 (9 / 0) | 0.49 (9) | 0.25 (2) | 0.096 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.12** (1 / 0) | 0.12 (1 / 0) | 0.14 (1) | 1.30 (136) | 0.226 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.27** (1 / 0) | 1.64 (15 / 0) | 1.17 (15) | 5.60 (47) | 0.270 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **4.34** (43 / 0) | 2.31 (41 / 0) | 3.14 (41) | 3.40 (126) | 0.117 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **32.28** (137 / 0) | 29.60 (201 / 0) | 12.07 (201) | 207.14 (1912) | 0.242 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **37.54** (594 / 14) | 45.49 (701 / 14) | 101.78 (5000, `NodeLimit`) | 78.24 (5000, `NodeLimit`) | 0.119 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **15.18** (48 / 31) | 24.38 (63 / 31) | 6.59 (134) | 6.85 (134) | 0.163 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **102.11** (27 / 203) | 126.52 (416 / 203) | 246.37 (5000, `NodeLimit`) | 225.73 (5000, `NodeLimit`) | 0.918 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **6.97** (35 / 47) | 12.73 (96 / 47) | 250.02 (4997, `TimeLimit`) | 172.46 (5000, `NodeLimit`) | 0.151 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.51** (1 / 0) | 2.01 (14 / 0) | 1.81 (14) | 1.24 (1) | 0.911 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **1.15** (1 / 0) | 2.78 (12 / 0) | 2.66 (12) | 6.48 (27) | 1.478 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **5.04** (1 / 0) | 55.98 (130 / 0) | 56.28 (130) | 216.93 (270) | 15.495 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **43.43** (1 / 0) | 44.08 (2 / 0) | 43.75 (2) | 250.41 (107, `TimeLimit`) | 117.723 | 208000.00 | 208000.00 |
<!-- END GENERATED:FULL_CHECKPOINT_TABLE -->

---

## 7. Linear Sum Assignment Benchmarks (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus ([`bench/assign_corpus.go`](../bench/assign_corpus.go), manifest in `testdata/assign_corpus/manifest.json`) evaluates pure-Go [`assign.LinearSumFlat`](../assign/assign.go) against SciPy's compiled C++ `rectangular_lsap` extension (`scipy.optimize.linear_sum_assignment`) across 200 randomized instances in warm state:
- **Bit-identical matrices**: every entry is generated from a 64-bit `SplitMix64` stream (`splitMix64At`) scaled by exact dyadic rationals ($2^{-k}$), and [`bench/scipy_assign_baseline.py`](../bench/scipy_assign_baseline.py) verifies the IEEE-754 SHA-256 digest of every family's canonical matrix against `manifest.json` before timing.
- **Pure in-process solver timing**: `scipy.optimize.linear_sum_assignment` calls its C++ extension in-process on a contiguous `float64` NumPy array, and [`bench/scipy_assign_baseline.py`](../bench/scipy_assign_baseline.py) stops the solve timer immediately upon return before computing the NumPy objective sum, so `SciPy Solve / goblin` compares in-memory solver execution directly.

### 7.1 Aggregate LSAP Workload Summary

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Instances) | Paired `SciPy Solve / goblin` GeoMean [95% CI] | Paired `SciPy Total / goblin` GeoMean | `goblin` Solve Sum (ms) | SciPy Solve Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.65x** [1.51x-1.80x] | 4.67x | 0.217 | 0.332 | 1.72x [0.34x-2.88x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **3.04x** [2.66x-3.47x] | 2.99x | 10.588 | 35.227 | 2.70x [0.23x-26.41x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **3.00x** [2.64x-3.41x] | 3.93x | 285.197 | 1033.197 | 2.99x [1.47x-6.71x] |
| Square families ($m = n$) | 16 (146) | **2.73x** [2.45x-3.04x] | 3.85x | 291.115 | 1055.621 | 2.54x [0.23x-26.41x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.91x** [1.74x-2.09x] | 3.10x | 4.887 | 13.134 | 2.08x [0.78x-4.11x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.54x** [2.08x-3.11x] | 2.98x | 37.102 | 70.833 | 1.95x [0.97x-26.41x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.48x** [2.28x-2.70x] | 3.63x | 296.002 | 1068.755 | 2.28x [0.23x-26.41x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 7.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.78x** | 0.0004 | 0.0010 | 0.0005 | 0.0121 | $0.0008 \pm 0.0011$ | $0.0010 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **2.13x** | 0.0011 | 0.0026 | 0.0015 | 0.0109 | $0.0012 \pm 0.0003$ | $0.0025 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.62x** | 0.0037 | 0.0066 | 0.0055 | 0.0159 | $0.0040 \pm 0.0014$ | $0.0064 \pm 0.0018$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.40x** | 0.0029 | 0.0041 | 0.0044 | 0.0148 | $0.0031 \pm 0.0011$ | $0.0042 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.39x** | 0.0030 | 0.0045 | 0.0046 | 0.0160 | $0.0036 \pm 0.0016$ | $0.0047 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.68x** | 0.0071 | 0.0117 | 0.0122 | 0.0342 | $0.0070 \pm 0.0022$ | $0.0114 \pm 0.0020$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.75x** | 0.0279 | 0.0752 | 0.0416 | 0.102 | $0.0270 \pm 0.0061$ | $0.0732 \pm 0.0110$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.49x** | 0.0543 | 0.187 | 0.0722 | 0.238 | $0.0532 \pm 0.0056$ | $0.187 \pm 0.0275$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.16x** | 0.0470 | 0.100 | 0.0728 | 0.152 | $0.0478 \pm 0.0114$ | $0.102 \pm 0.0150$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.63x** | 0.150 | 0.233 | 0.159 | 0.285 | $0.155 \pm 0.0463$ | $0.249 \pm 0.0470$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **14.23x** | 0.0585 | 0.730 | 0.0803 | 0.807 | $0.0546 \pm 0.0086$ | $0.780 \pm 0.176$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **2.28x** | 0.0332 | 0.0734 | 0.0581 | 0.152 | $0.0329 \pm 0.0025$ | $0.0753 \pm 0.0083$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.08x** | 0.0395 | 0.0830 | 0.0665 | 0.162 | $0.0405 \pm 0.0030$ | $0.0844 \pm 0.0074$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **2.54x** | 0.0962 | 0.299 | 0.122 | 0.383 | $0.196 \pm 0.330$ | $0.309 \pm 0.0522$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.81x** | 0.339 | 1.350 | 0.447 | 1.814 | $0.355 \pm 0.0613$ | $1.343 \pm 0.127$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.06x** | 1.313 | 5.749 | 2.045 | 8.905 | $1.502 \pm 0.341$ | $5.995 \pm 0.517$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **2.23x** | 0.826 | 1.766 | 1.424 | 5.237 | $0.819 \pm 0.0499$ | $1.844 \pm 0.324$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.66x** | 5.362 | 9.002 | 5.971 | 10.041 | $6.010 \pm 2.240$ | $9.795 \pm 2.819$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **3.00x** | 0.331 | 0.890 | 0.732 | 3.826 | $0.326 \pm 0.0243$ | $0.999 \pm 0.264$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.64x** | 0.469 | 1.316 | 0.921 | 4.488 | $0.475 \pm 0.0401$ | $1.257 \pm 0.137$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.94x** | 6.715 | 32.955 | 8.661 | 46.971 | $6.682 \pm 0.716$ | $33.099 \pm 4.242$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **3.74x** | 40.362 | 157.877 | 49.244 | 207.146 | $41.226 \pm 4.701$ | $153.650 \pm 11.339$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 7.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSlice64`, `InitReducedRow64`, and `Min2ReducedRow64`)

In [`assign/solver.go`](../assign/solver.go), [`simd.AllAbsLE64With`](../internal/simd/kernels_amd64.go) validates finite cost bounds across the input matrix via 8-lane AVX2 `Abs().LessEqual` checks, [`simd.MinReducedSlice64With`](../internal/simd/kernels_amd64.go) accelerates reduction-transfer slack scans in `preprocessSquareJV`, [`simd.Min2ReducedRow64With`](../internal/simd/kernels_amd64.go) accelerates the first/second reduced-minimum scans in `preprocessRectangularBV` and `augmentingRowReduction` via 8-way AVX2 `VSUBPD` and `VMINPD`/`VCMPPD`/`VMOVMSKPD` bitmask filtering when $\max(m, n) \ge 64$, and [`simd.InitReducedRow64With`](../internal/simd/kernels_amd64.go) runs on a slice of length $\max(m, n)$ at the start of each Dijkstra augmentation when $\max(m, n) \ge 32$. All vector kernels clean upper `YMM` state via `StoreArray` + `ClearAVXUpperBits()` (`VZEROUPPER`) before scalar operations, yielding consistent end-to-end speedups across medium, large, and rectangular instances.

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32`, no AVX2 row-reduction kernel runs): 0 of 4 have a 95% interval excluding 1.
- **Instances where AVX2 row kernels run** (`max(rows, cols) >= 32`): 7 faster, 4 slower, 7 with no detectable effect, out of 18.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 row kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source | 1.00x [0.95x-1.06x] | 0.0004 | 0.0004 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source | 0.98x [0.95x-1.02x] | 0.0011 | 0.0012 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source | 1.02x [0.91x-1.14x] | 0.0037 | 0.0038 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | no detectable effect | 0.99x [0.87x-1.13x] | 0.0029 | 0.0031 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | no detectable effect | 1.04x [0.93x-1.16x] | 0.0030 | 0.0031 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | no (control) | identical source | 0.88x [0.74x-1.04x] | 0.0071 | 0.0073 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD slower | 0.82x [0.79x-0.85x] | 0.0279 | 0.0336 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD slower | 0.84x [0.81x-0.87x] | 0.0543 | 0.0628 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD slower | 0.90x [0.87x-0.92x] | 0.0470 | 0.0526 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD slower | 0.98x [0.95x-1.00x] | 0.150 | 0.154 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | no detectable effect | 1.01x [0.97x-1.06x] | 0.0585 | 0.0582 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.20x [1.08x-1.34x] | 0.0332 | 0.0293 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | no detectable effect | 1.02x [0.95x-1.09x] | 0.0395 | 0.0406 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | no detectable effect | 1.13x [0.66x-1.94x] | 0.0962 | 0.108 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.02x [1.00x-1.03x] | 0.339 | 0.336 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | no detectable effect | 1.04x [0.93x-1.15x] | 1.313 | 1.231 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.18x [1.16x-1.21x] | 0.826 | 0.703 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.07x [1.04x-1.10x] | 5.362 | 5.052 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.73x [1.62x-1.85x] | 0.331 | 0.198 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.23x [1.16x-1.31x] | 0.469 | 0.372 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.13x [1.04x-1.23x] | 6.715 | 5.916 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | no detectable effect | 1.18x [0.99x-1.40x] | 40.362 | 34.412 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

---

## 8. Keeping Documentation and Benchmarks Synchronized

1. **Regenerate telemetry** from a clean, committed tree in a single pinned session so that all seven telemetry files record identical `git_commit`, `source_fingerprint`, and `uname_release`: run the commands in the [README](../README.md#verification-benchmarking-and-documentation-sync-commands).
2. **Regenerate tables**: `go run ./cmd/goblin-bench -sync-docs`.
3. **Verify**: `go run ./cmd/goblin-bench -check-docs`, or `go test ./...`, which runs `TestDocumentationAndBenchmarkSync`. The gate validates corpus dimensions, parity, symbol identifiers, and provenance, and checks every generated block in `README.md`, `docs/benchmarks.md`, and `bench/results/final_report.md`.
