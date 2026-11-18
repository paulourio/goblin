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
- **Unified telemetry source**: fingerprint `f2b8d69d4aef8b31` (working tree on top of commit `759089c34649faad2e8fade211b398f598437c4b`), `go1.27.3-X:nodwarf5`, Linux kernel `6.18.63-gentoo-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 7.33 ms median on a 1-variable MILP and 6.88 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
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
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **336.05x** [273.46x-412.96x] | 0.05x | 14.2 | 206.7 | 1208.1 | 524.55x [13.11x-1540.99x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **18.94x** [15.55x-23.06x] | 0.16x | 858.6 | 2121.3 | 4706.1 | 24.22x [0.19x-165.84x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **8.02x** [5.63x-11.44x] | 0.26x | 4749.4 | 3119.2 | 6677.5 | 9.68x [0.06x-32.40x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **11.73x** [9.44x-14.58x] | 0.23x | 5279.6 | 3366.6 | 7344.5 | 12.10x [0.06x-165.84x] |
| Tier 3 non-MAD MILPs | 4 (20) | **4.66x** [2.54x-8.55x] | 0.47x | 4429.1 | 1340.3 | 3028.6 | 6.64x [0.06x-19.43x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **37.63x** [30.75x-46.05x] | 0.09x | 329.8 | 1900.1 | 4160.4 | 44.11x [3.41x-99.95x] |
| Random-weight MAD (negative controls) | 3 (33) | **13.70x** [9.09x-20.65x] | 0.18x | 467.3 | 1317.4 | 2425.4 | 12.62x [1.62x-89.94x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **53.56x** [40.79x-70.32x] | 0.12x | 5292.4 | 3547.0 | 8431.4 | 45.06x [0.06x-1540.99x] |
| All 31 synthetic corpus families | 31 (293) | **49.59x** [39.88x-61.65x] | 0.11x | 5622.2 | 5447.1 | 12591.7 | 44.77x [0.06x-1540.99x] |
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
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **81.3x** [67.1x-100.0x] | 0.13 | 11.02 | 0.08 | 10.79 | 0.049 | 0.313 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **46.8x** [13.1x-89.9x] | 0.61 | 36.65 | 0.59 | 36.40 | 0.030 | 0.322 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **67.6x** [37.3x-85.7x] | 0.14 | 10.66 | 0.10 | 10.34 | 0.037 | 0.392 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **66.4x** [62.8x-74.8x] | 0.18 | 11.51 | 0.11 | 11.08 | 0.057 | 0.574 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **34.0x** [24.2x-44.4x] | 0.36 | 12.96 | 0.26 | 12.33 | 0.090 | 0.849 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **10.8x** [3.2x-39.8x] | 4.47 | 51.51 | 4.42 | 51.18 | 0.051 | 0.421 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **5.1x** [1.6x-24.3x] | 32.00 | 179.21 | 31.93 | 178.73 | 0.074 | 0.596 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **29.1x** [26.3x-32.4x] | 0.68 | 19.95 | 0.48 | 18.76 | 0.185 | 1.420 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **14.9x** [6.5x-20.2x] | 1.16 | 20.61 | 0.88 | 18.65 | 0.296 | 2.439 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **10.2x** [9.4x-11.5x] | 6.77 | 65.16 | 4.95 | 56.75 | 1.419 | 11.509 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **8.2x** [3.4x-21.1x] | 49.00 | 393.64 | 42.51 | 357.50 | 6.324 | 47.502 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Cross-Instance Latency Dispersion Across Randomized Instances (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.14 \pm 0.02$ ($\pm 0.01$) | $11.02 \pm 0.24$ ($\pm 0.16$) | $0.13 \pm 0.03$ ($\pm 0.02$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.96 \pm 0.87$ ($\pm 0.58$) | $33.54 \pm 10.29$ ($\pm 6.91$) | $0.98 \pm 0.89$ ($\pm 0.59$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.17 \pm 0.06$ ($\pm 0.04$) | $10.72 \pm 0.68$ ($\pm 0.46$) | $0.18 \pm 0.06$ ($\pm 0.04$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.17 \pm 0.01$ ($\pm 0.01$) | $11.55 \pm 0.36$ ($\pm 0.24$) | $0.23 \pm 0.06$ ($\pm 0.04$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.40 \pm 0.08$ ($\pm 0.06$) | $13.20 \pm 0.58$ ($\pm 0.39$) | $0.42 \pm 0.10$ ($\pm 0.07$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $4.21 \pm 2.27$ ($\pm 1.53$) | $38.50 \pm 18.27$ ($\pm 12.27$) | $4.16 \pm 2.32$ ($\pm 1.56$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $37.31 \pm 30.76$ ($\pm 20.66$) | $148.44 \pm 67.74$ ($\pm 45.50$) | $38.08 \pm 32.59$ ($\pm 21.89$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.67 \pm 0.04$ ($\pm 0.04$) | $19.66 \pm 2.41$ ($\pm 2.99$) | $0.77 \pm 0.22$ ($\pm 0.27$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.52 \pm 0.91$ ($\pm 1.12$) | $20.50 \pm 0.22$ ($\pm 0.27$) | $1.49 \pm 0.77$ ($\pm 0.96$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $6.65 \pm 0.63$ ($\pm 0.79$) | $67.61 \pm 4.02$ ($\pm 4.98$) | $6.72 \pm 0.75$ ($\pm 0.93$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $55.21 \pm 16.74$ ($\pm 20.78$) | $622.01 \pm 660.27$ ($\pm 819.70$) | $53.83 \pm 16.28$ ($\pm 20.21$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | **0.08** (1 / 0) | 0.06 (1 / 0) | 0.08 (1) | 0.36 (3) | 0.072 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.59** (5 / 0) | 0.57 (12 / 0) | 0.56 (12) | 0.84 (84) | 0.070 | 4601.00 | 4257.00 |
| `t2_mad_g2_i8` | **0.10** (1 / 0) | 0.53 (9 / 0) | 0.47 (9) | 0.22 (2) | 0.095 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.11** (1 / 0) | 0.11 (1 / 0) | 0.12 (1) | 1.91 (136) | 0.209 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.26** (1 / 0) | 1.67 (15 / 0) | 1.31 (15) | 5.73 (47) | 0.278 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **4.42** (43 / 0) | 2.27 (41 / 0) | 2.10 (41) | 3.45 (126) | 0.118 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **31.93** (137 / 0) | 26.83 (201 / 0) | 12.13 (201) | 208.75 (1912) | 0.246 | 7720.00 | 6060.00 |
| `t3_mad_g5_i20` | **0.48** (1 / 0) | 1.97 (14 / 0) | 2.17 (14) | 1.00 (1) | 0.989 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.88** (1 / 0) | 2.81 (12 / 0) | 2.68 (12) | 6.34 (27) | 1.431 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **4.95** (1 / 0) | 54.09 (130 / 0) | 51.37 (130) | 209.78 (270) | 15.575 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **42.51** (1 / 0) | 44.42 (2 / 0) | 44.61 (2) | 250.14 (134, `TimeLimit`) | 116.443 | 208000.00 | 208000.00 |
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
| `t1_std_lp` | 3 | 3 | **789.8x** [480.0x-1338.1x] | 0.01 | 7.19 | 0.00 | 7.13 | 0.006 | 0.084 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **855.0x** [376.6x-1171.9x] | 0.01 | 7.26 | 0.00 | 7.20 | 0.005 | 0.086 | `OK` |
| `t1_std_mip` | 3 | 3 | **660.9x** [483.4x-999.8x] | 0.01 | 8.64 | 0.01 | 8.57 | 0.006 | 0.092 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **660.5x** [523.4x-806.2x] | 0.01 | 8.33 | 0.01 | 8.27 | 0.006 | 0.089 | `OK` |
| `t1_whiskas` | 6 | 5 | **457.7x** [288.8x-608.2x] | 0.02 | 7.49 | 0.00 | 7.40 | 0.010 | 0.124 | `OK` |
| `t1_furniture` | 2 | 3 | **295.1x** [108.5x-564.0x] | 0.03 | 9.17 | 0.02 | 9.10 | 0.005 | 0.090 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **141.9x** [66.2x-293.2x] | 0.08 | 10.34 | 0.06 | 10.24 | 0.012 | 0.133 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **854.7x** [74.3x-1541.0x] | 0.01 | 7.01 | 0.00 | 6.96 | 0.005 | 0.076 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **81.3x** [67.1x-100.0x] | 0.13 | 11.02 | 0.08 | 10.79 | 0.049 | 0.313 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **46.8x** [13.1x-89.9x] | 0.61 | 36.65 | 0.59 | 36.40 | 0.030 | 0.322 | `OK` |
| `t2_mkp_30` | 30 | 6 | **8.3x** [4.3x-15.8x] | 2.97 | 30.43 | 2.90 | 30.24 | 0.031 | 0.245 | `OK` |
| `t2_mkp_50` | 50 | 8 | **5.4x** [2.5x-52.4x] | 5.03 | 18.86 | 4.98 | 18.59 | 0.050 | 0.363 | `OK` |
| `t2_cflp_36` | 36 | 11 | **31.6x** [4.4x-109.6x] | 0.23 | 10.13 | 0.19 | 9.96 | 0.037 | 0.229 | `OK` |
| `t2_cflp_49` | 49 | 13 | **27.5x** [10.9x-47.6x] | 0.38 | 11.22 | 0.35 | 11.01 | 0.029 | 0.287 | `OK` |
| `t2_gap_45` | 45 | 13 | **11.8x** [2.4x-30.8x] | 1.05 | 16.04 | 1.00 | 15.83 | 0.036 | 0.280 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **10.9x** [4.0x-29.8x] | 4.82 | 74.34 | 4.74 | 74.07 | 0.038 | 0.330 | `OK` |
| `t2_binpack_44` | 44 | 14 | **15.7x** [0.2x-93.3x] | 0.46 | 15.37 | 0.43 | 15.15 | 0.028 | 0.283 | `OK` |
| `t2_setcover_50` | 50 | 20 | **61.9x** [4.8x-165.8x] | 0.17 | 12.56 | 0.12 | 12.25 | 0.053 | 0.440 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **67.6x** [37.3x-85.7x] | 0.14 | 10.66 | 0.10 | 10.34 | 0.037 | 0.392 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **66.4x** [62.8x-74.8x] | 0.18 | 11.51 | 0.11 | 11.08 | 0.057 | 0.574 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **34.0x** [24.2x-44.4x] | 0.36 | 12.96 | 0.26 | 12.33 | 0.090 | 0.849 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **10.8x** [3.2x-39.8x] | 4.47 | 51.51 | 4.42 | 51.18 | 0.051 | 0.421 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **5.1x** [1.6x-24.3x] | 32.00 | 179.21 | 31.93 | 178.73 | 0.074 | 0.596 | `OK` |
| `t3_mkp_200` | 200 | 8 | **3.3x** [2.8x-3.8x] | 38.15 | 124.27 | 38.06 | 123.62 | 0.130 | 0.854 | `OK` |
| `t3_cflp_300` | 300 | 34 | **13.0x** [9.6x-19.4x] | 15.52 | 266.84 | 15.33 | 265.89 | 0.189 | 1.250 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **1.0x** [0.1x-2.4x] | 102.07 | 169.55 | 101.56 | 167.58 | 0.314 | 2.469 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **11.4x** [9.5x-13.8x] | 8.15 | 86.71 | 7.72 | 83.57 | 0.478 | 3.836 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **29.1x** [26.3x-32.4x] | 0.68 | 19.95 | 0.48 | 18.76 | 0.185 | 1.420 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **14.9x** [6.5x-20.2x] | 1.16 | 20.61 | 0.88 | 18.65 | 0.296 | 2.439 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **10.2x** [9.4x-11.5x] | 6.77 | 65.16 | 4.95 | 56.75 | 1.419 | 11.509 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **8.2x** [3.4x-21.1x] | 49.00 | 393.64 | 42.51 | 357.50 | 6.324 | 47.502 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## 5. Default Build vs. `GOEXPERIMENT=simd`

In [`internal/simd/kernels_amd64.go`](../internal/simd/kernels_amd64.go), AVX2 kernels spill 4-lane accumulators to the stack via `StoreArray` and execute `archsimd.ClearAVXUpperBits()` (`VZEROUPPER`) before scalar tail or caller operations, eliminating AVX-to-SSE dirty-upper `YMM` transition stalls while preserving `NaN` propagation parity. From the dispatch threshold `minAVX2Len = 32` upward, the 256-bit AVX2 kernels (`Dot64`, `Axpy64`, `Scale64`, `AbsMax64`, `AllAbsLE64`, `RowActivityBoundsFinite`, `MinReducedSlice64`, `InitReducedRow64`, and `Min2ReducedRow64` at $N \ge 64$) outperform the 4-way slice-stepped scalar loops; below $N = 32$, call and `VZEROUPPER` overhead dominates, so slices of length $< 32$ remain on the scalar path.

At the full-solver level, each ratio below is the geometric mean of per-trial `scalar / SIMD` solve times over the same instances, with a 95% $t$-interval on the log ratios; "no detectable effect" means the interval contains 1. "Reachable" is a necessary condition: the AVX2 `Dot64`/`Axpy64`/`Scale64` paths require a slice of length 32 or more (`n + m >= 32`). Below that size no AVX2 kernel can run, so those 8 families serve as negative controls where intervals excluding 1 reflect binary code-layout or run-to-run variance rather than SIMD execution. Lowering `minAVX2Len` from 256 to 32 enables AVX2 across dense tableau row accumulation (`ComputeTableauRow`) and dual reduced-cost updates (`computeDualsAndReducedCosts`) for medium models with $n \le 256$ (`DenseThreshold`), while `ComputeTableauRow` dispatches `Axpy64` on contiguous CSR column blocks in large models ($n > 256$, such as `t3_mad_g4_i1000`).

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 0 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 2 faster, 1 slower, 20 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 1.01x [0.96x-1.07x] | 0.002 | 0.002 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 1.00x [0.97x-1.04x] | 0.002 | 0.002 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 0.99x [0.88x-1.11x] | 0.006 | 0.006 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source | 1.06x [0.98x-1.16x] | 0.006 | 0.005 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 1.20x [0.95x-1.52x] | 0.003 | 0.003 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 0.92x [0.82x-1.02x] | 0.019 | 0.018 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source | 1.00x [0.98x-1.03x] | 0.064 | 0.066 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 1.34x [0.74x-2.43x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | SIMD faster | 1.13x [1.02x-1.26x] | 0.079 | 0.068 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 0.99x [0.96x-1.02x] | 0.585 | 0.639 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 0.99x [0.98x-1.00x] | 2.897 | 3.001 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 1.01x [0.99x-1.03x] | 4.982 | 4.813 | 11 |
| `t2_cflp_36` | 47 | possible | no detectable effect | 0.98x [0.84x-1.13x] | 0.186 | 0.205 | 11 |
| `t2_cflp_49` | 62 | possible | no detectable effect | 0.99x [0.98x-1.00x] | 0.347 | 0.348 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 1.05x [1.00x-1.11x] | 1.002 | 0.980 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 0.99x [0.97x-1.02x] | 4.738 | 4.910 | 11 |
| `t2_binpack_44` | 58 | possible | no detectable effect | 0.99x [0.97x-1.01x] | 0.431 | 0.426 | 11 |
| `t2_setcover_50` | 70 | possible | SIMD faster | 1.11x [1.05x-1.19x] | 0.120 | 0.100 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 0.93x [0.75x-1.15x] | 0.103 | 0.099 | 11 |
| `t2_mad_g3_i9` | 108 | possible | SIMD slower | 0.80x [0.69x-0.93x] | 0.114 | 0.151 | 11 |
| `t2_mad_g4_i12` | 156 | possible | no detectable effect | 0.97x [0.80x-1.18x] | 0.258 | 0.268 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.00x [0.96x-1.05x] | 4.422 | 4.300 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 0.99x [0.96x-1.01x] | 31.934 | 32.664 | 11 |
| `t3_mkp_200` | 208 | possible | no detectable effect | 1.00x [0.92x-1.09x] | 38.057 | 36.598 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.01x [0.97x-1.06x] | 15.335 | 15.841 | 5 |
| `t3_lotsizing_504` | 840 | possible | no detectable effect | 1.00x [0.97x-1.04x] | 101.564 | 103.085 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 1.03x [0.93x-1.15x] | 7.717 | 7.057 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 0.88x [0.59x-1.31x] | 0.483 | 0.472 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 0.99x [0.89x-1.11x] | 0.876 | 0.863 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 0.99x [0.71x-1.37x] | 4.952 | 5.315 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | no detectable effect | 1.02x [0.99x-1.05x] | 42.515 | 40.031 | 5 |
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
| `t1_knapsack_12` | **0.06** (1 / 8) | 0.08 (2 / 8) | 0.09 (24) | 0.13 (31) | 0.005 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.08** (1 / 0) | 0.06 (1 / 0) | 0.08 (1) | 0.36 (3) | 0.072 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.59** (5 / 0) | 0.57 (12 / 0) | 0.56 (12) | 0.84 (84) | 0.070 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.90** (29 / 8) | 2.14 (60 / 8) | 0.99 (77) | 0.86 (77) | 0.020 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **4.98** (1 / 4) | 4.45 (2 / 4) | 4.39 (132) | 3.70 (119) | 0.032 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.19** (1 / 2) | 1.19 (3 / 2) | 0.15 (7) | 0.17 (7) | 0.022 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.35** (1 / 20) | 1.69 (5 / 20) | 0.32 (9) | 0.32 (9) | 0.028 | 909.02 | 827.30 |
| `t2_gap_45` | **1.00** (1 / 8) | 0.24 (1 / 8) | 1.67 (61) | 1.78 (61) | 0.027 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **4.74** (3 / 35) | 5.82 (33 / 35) | 2.20 (198) | 2.42 (174) | 0.053 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **0.43** (1 / 5) | 0.96 (13 / 5) | 14.47 (1838) | 27.99 (2450) | 0.026 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.12** (1 / 6) | 0.23 (2 / 6) | 0.78 (9) | 0.77 (9) | 0.030 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.10** (1 / 0) | 0.53 (9 / 0) | 0.47 (9) | 0.22 (2) | 0.095 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.11** (1 / 0) | 0.11 (1 / 0) | 0.12 (1) | 1.91 (136) | 0.209 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.26** (1 / 0) | 1.67 (15 / 0) | 1.31 (15) | 5.73 (47) | 0.278 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **4.42** (43 / 0) | 2.27 (41 / 0) | 2.10 (41) | 3.45 (126) | 0.118 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **31.93** (137 / 0) | 26.83 (201 / 0) | 12.13 (201) | 208.75 (1912) | 0.246 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **38.06** (594 / 14) | 45.45 (701 / 14) | 97.78 (5000, `NodeLimit`) | 76.66 (5000, `NodeLimit`) | 0.116 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **15.33** (48 / 31) | 23.10 (63 / 31) | 6.71 (134) | 6.95 (134) | 0.158 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **101.56** (27 / 203) | 124.05 (416 / 203) | 250.03 (4828, `TimeLimit`) | 228.63 (5000, `NodeLimit`) | 0.993 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **7.72** (35 / 47) | 11.98 (96 / 47) | 244.17 (5000, `NodeLimit`) | 172.94 (5000, `NodeLimit`) | 0.137 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.48** (1 / 0) | 1.97 (14 / 0) | 2.17 (14) | 1.00 (1) | 0.989 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.88** (1 / 0) | 2.81 (12 / 0) | 2.68 (12) | 6.34 (27) | 1.431 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **4.95** (1 / 0) | 54.09 (130 / 0) | 51.37 (130) | 209.78 (270) | 15.575 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **42.51** (1 / 0) | 44.42 (2 / 0) | 44.61 (2) | 250.14 (134, `TimeLimit`) | 116.443 | 208000.00 | 208000.00 |
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
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.58x** [1.43x-1.74x] | 4.65x | 0.247 | 0.335 | 1.67x [0.31x-2.88x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **3.12x** [2.76x-3.52x] | 2.97x | 9.564 | 35.559 | 2.76x [1.03x-21.16x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.81x** [2.43x-3.24x] | 3.59x | 257.406 | 1008.693 | 2.70x [0.87x-5.81x] |
| Square families ($m = n$) | 16 (146) | **2.69x** [2.42x-3.00x] | 3.78x | 262.425 | 1033.008 | 2.54x [0.31x-21.16x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.88x** [1.74x-2.04x] | 3.02x | 4.792 | 11.580 | 2.09x [0.84x-2.81x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.41x** [1.96x-2.97x] | 2.91x | 38.682 | 70.303 | 1.93x [0.31x-21.16x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.45x** [2.25x-2.66x] | 3.56x | 267.217 | 1044.588 | 2.25x [0.31x-21.16x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 7.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.47x** | 0.0005 | 0.0009 | 0.0006 | 0.0113 | $0.0009 \pm 0.0011$ | $0.0009 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **2.17x** | 0.0012 | 0.0025 | 0.0015 | 0.0106 | $0.0011 \pm 0.0003$ | $0.0025 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.68x** | 0.0036 | 0.0066 | 0.0055 | 0.0159 | $0.0039 \pm 0.0013$ | $0.0064 \pm 0.0018$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.43x** | 0.0029 | 0.0042 | 0.0045 | 0.0157 | $0.0030 \pm 0.0011$ | $0.0041 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.38x** | 0.0031 | 0.0044 | 0.0048 | 0.0162 | $0.0035 \pm 0.0013$ | $0.0046 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.47x** | 0.0070 | 0.0116 | 0.0123 | 0.0338 | $0.0100 \pm 0.0102$ | $0.0119 \pm 0.0033$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.74x** | 0.0286 | 0.0759 | 0.0410 | 0.103 | $0.0284 \pm 0.0083$ | $0.0755 \pm 0.0106$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.47x** | 0.0552 | 0.191 | 0.0729 | 0.247 | $0.0539 \pm 0.0059$ | $0.188 \pm 0.0288$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.06x** | 0.0467 | 0.0996 | 0.0734 | 0.150 | $0.0513 \pm 0.0170$ | $0.102 \pm 0.0145$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.69x** | 0.145 | 0.234 | 0.154 | 0.282 | $0.154 \pm 0.0464$ | $0.256 \pm 0.0529$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **13.40x** | 0.0587 | 0.726 | 0.0810 | 0.805 | $0.0585 \pm 0.0128$ | $0.781 \pm 0.173$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **2.36x** | 0.0324 | 0.0740 | 0.0583 | 0.151 | $0.0320 \pm 0.0028$ | $0.0756 \pm 0.0085$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.08x** | 0.0412 | 0.0853 | 0.0705 | 0.163 | $0.0411 \pm 0.0032$ | $0.0855 \pm 0.0076$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.31x** | 0.0904 | 0.303 | 0.118 | 0.387 | $0.0929 \pm 0.0105$ | $0.307 \pm 0.0331$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.84x** | 0.353 | 1.391 | 0.463 | 1.876 | $0.357 \pm 0.0638$ | $1.361 \pm 0.139$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.07x** | 1.358 | 5.571 | 1.781 | 8.399 | $1.398 \pm 0.145$ | $5.685 \pm 0.431$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.67x** | 0.825 | 1.591 | 1.432 | 4.250 | $1.009 \pm 0.438$ | $1.590 \pm 0.0567$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.64x** | 5.560 | 9.139 | 5.802 | 10.065 | $6.115 \pm 2.107$ | $9.924 \pm 2.923$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.68x** | 0.334 | 0.873 | 0.753 | 3.848 | $0.333 \pm 0.0236$ | $0.893 \pm 0.0605$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.33x** | 0.445 | 1.052 | 0.859 | 3.854 | $0.451 \pm 0.0169$ | $1.050 \pm 0.0434$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.58x** | 6.983 | 30.078 | 9.451 | 41.198 | $6.696 \pm 0.526$ | $30.664 \pm 2.457$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.28x** | 35.137 | 150.915 | 42.497 | 208.605 | $35.479 \pm 2.662$ | $151.933 \pm 9.947$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 7.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSlice64`, `InitReducedRow64`, and `Min2ReducedRow64`)

In [`assign/solver.go`](../assign/solver.go), [`simd.AllAbsLE64With`](../internal/simd/kernels_amd64.go) validates finite cost bounds across the input matrix via 8-lane AVX2 `Abs().LessEqual` checks, [`simd.MinReducedSlice64With`](../internal/simd/kernels_amd64.go) accelerates reduction-transfer slack scans in `preprocessSquareJV`, [`simd.Min2ReducedRow64With`](../internal/simd/kernels_amd64.go) accelerates the first/second reduced-minimum scans in `preprocessRectangularBV` and `augmentingRowReduction` via 8-way AVX2 `VSUBPD` and `VMINPD`/`VCMPPD`/`VMOVMSKPD` bitmask filtering when $\max(m, n) \ge 64$, and [`simd.InitReducedRow64With`](../internal/simd/kernels_amd64.go) runs on a slice of length $\max(m, n)$ at the start of each Dijkstra augmentation when $\max(m, n) \ge 32$. All vector kernels clean upper `YMM` state via `StoreArray` + `ClearAVXUpperBits()` (`VZEROUPPER`) before scalar operations, yielding consistent end-to-end speedups across medium, large, and rectangular instances.

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32`, no AVX2 row-reduction kernel runs): 3 of 4 have a 95% interval excluding 1.
- **Instances where AVX2 row kernels run** (`max(rows, cols) >= 32`): 5 faster, 5 slower, 8 with no detectable effect, out of 18.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 row kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.13x [1.03x-1.23x] | 0.0005 | 0.0004 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.94x [0.91x-0.97x] | 0.0012 | 0.0013 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.96x [0.94x-0.99x] | 0.0036 | 0.0037 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | no detectable effect | 0.97x [0.95x-1.00x] | 0.0029 | 0.0030 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | no detectable effect | 1.02x [0.93x-1.11x] | 0.0031 | 0.0031 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | no (control) | identical source | 0.95x [0.56x-1.61x] | 0.0070 | 0.0074 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD slower | 0.85x [0.79x-0.92x] | 0.0286 | 0.0351 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD slower | 0.85x [0.83x-0.87x] | 0.0552 | 0.0653 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | no detectable effect | 0.98x [0.82x-1.18x] | 0.0467 | 0.0522 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD slower | 0.95x [0.93x-0.98x] | 0.145 | 0.150 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | no detectable effect | 1.07x [0.99x-1.16x] | 0.0587 | 0.0592 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | no detectable effect | 0.98x [0.82x-1.17x] | 0.0324 | 0.0303 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | no detectable effect | 1.00x [0.97x-1.02x] | 0.0412 | 0.0413 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD slower | 0.87x [0.85x-0.89x] | 0.0904 | 0.104 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.03x [1.00x-1.05x] | 0.353 | 0.326 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.16x [1.08x-1.24x] | 1.358 | 1.175 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | no detectable effect | 1.40x [0.91x-2.16x] | 0.825 | 0.687 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.07x [1.03x-1.11x] | 5.560 | 5.209 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.74x [1.56x-1.94x] | 0.334 | 0.200 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.15x [1.10x-1.20x] | 0.445 | 0.385 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | no detectable effect | 1.10x [0.99x-1.21x] | 6.983 | 6.258 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD slower | 0.88x [0.78x-1.00x] | 35.137 | 36.992 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

---

## 8. Keeping Documentation and Benchmarks Synchronized

1. **Regenerate telemetry** from a clean, committed tree in a single pinned session so that all seven telemetry files record identical `git_commit`, `source_fingerprint`, and `uname_release`: run the commands in the [README](../README.md#verification-benchmarking-and-documentation-sync-commands).
2. **Regenerate tables**: `go run ./cmd/goblin-bench -sync-docs`.
3. **Verify**: `go run ./cmd/goblin-bench -check-docs`, or `go test ./...`, which runs `TestDocumentationAndBenchmarkSync`. The gate validates corpus dimensions, parity, symbol identifiers, and provenance, and checks every generated block in `README.md`, `docs/benchmarks.md`, and `bench/results/final_report.md`.
