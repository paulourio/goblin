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
- **Unified telemetry source**: fingerprint `9c208e08cb0fabf1` (working tree on top of commit `a26ec8e26bcfd733dba753235c218bf493636d61`), `go1.27.3-X:simd`, Linux kernel `6.18.63-gentoo-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, or kernel differs.
- **CPU affinity**: goblin `6`; Python + CBC `6`; Python + SciPy `6`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 8.06 ms median on a 1-variable MILP and 7.52 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

- **Single-threaded, sequential, pinned execution**: every solve runs single-threaded (`threads=1` in CBC, one goroutine in `gb.SolveCompiled`), all harnesses run with `workers=1` in a single session on the same Linux kernel, and each harness is pinned with `taskset` to one performance core. The i9-13900K mixes 5.5 GHz P-cores (CPUs 0-13) with 4.3 GHz E-cores (CPUs 14-29); unpinned runs migrate between them and introduce bimodal variance.
- **Warm state**: each harness runs an untimed worker warmup (`_init_worker` in [`bench/python_baseline.py`](../bench/python_baseline.py), `warmWorker` in [`cmd/goblin-bench/main.go`](../cmd/goblin-bench/main.go)), then 2 untimed warmup solves per family on Tiers 1-2 and 1 on Tier 3.
- **Synthetic corpus and held-out public benchmarks**: each synthetic family in `testdata/corpus/trials/<id>.jsonl` holds 11 distinct seeded problem instances (Tiers 1-2) or 5 (Tier 3), 293 instances in total; `WriteCorpusJSON` rejects duplicate trial payloads. Trial $k$ is the identical problem instance in every harness, enabling paired per-instance ratios rather than repeated runs on a fixed matrix. Complementing the synthetic timing corpus, [`public_benchmarks_test.go`](../public_benchmarks_test.go) verifies optimal status, objective parity, and primal feasibility across 20 held-out public Netlib LP and MIPLIB 3.0 MILP instances in `testdata/public/`, plus a valid dual lower bound under a 25-node budget (`go_max_nodes: 25`) on `coinor_capplan1`.
- **Metrics**: per randomized instance, prep time (`t_parse + t_build + t_format/t_compile`), solve time (`t_solve`), and end-to-end time (`t_total`). Aggregate tables report equal-family geometric means with family-clustered 95% Student-$t_{F-1}$ intervals alongside pooled trial geometric means, both multi-trial and family-median wall-clock sums, and full per-instance `[Min-Max]` ranges without floor clamping.
- **Parity**: the benchmark gate requires both `goblin` pure-Go builds and `goblin (CBC)` to match CBC's canonical and per-trial statuses, objectives, and root LP relaxation objectives across all 293 instances within a relative tolerance of `1e-5` under matching `1e-6` MIP gap tolerances.

---

## 2. Aggregate Workload Summary

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `Py / Go` GeoMean [Clustered 95% CI] | Pooled Trial `Py / Go` | Equal-Family `Go / CBC` | `Go` Trial (Med) Sum (ms) | `CBC` Trial (Med) Sum (ms) | `Py` Trial (Med) Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **360.89x** [124.63x-1044.99x] | 360.89x | 0.06x | 41.1 (3.6) | 212.5 (20.8) | 1452.9 (134.1) | 630.33x [5.46x-2913.22x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **7.29x** [3.73x-14.23x] | 7.29x | 0.40x | 3083.7 (206.3) | 2304.5 (193.6) | 5485.9 (500.9) | 7.80x [0.05x-146.06x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **3.54x** [1.19x-10.50x] | 3.54x | 0.56x | 19772.5 (808.8) | 4019.2 (747.5) | 8487.6 (1443.8) | 4.92x [0.02x-32.49x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **3.72x** [1.87x-7.41x] | 4.31x | 0.69x | 22047.5 (915.4) | 3838.5 (475.4) | 8932.5 (1277.7) | 4.61x [0.02x-146.06x] |
| Tier 3 non-MAD MILPs | 4 (20) | **1.68x** [0.16x-17.27x] | 1.68x | 1.34x | 18985.3 (710.8) | 1645.1 (291.2) | 3878.0 (813.7) | 1.48x [0.02x-13.48x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **16.20x** [6.50x-40.38x] | 21.67x | 0.17x | 810.2 (99.9) | 2511.3 (468.4) | 5203.8 (683.5) | 18.21x [3.11x-139.33x] |
| Random-weight MAD (negative controls) | 3 (33) | **3.82x** [0.15x-95.73x] | 3.82x | 0.70x | 1596.5 (157.4) | 1401.4 (129.9) | 2922.1 (286.5) | 3.08x [0.25x-77.60x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **23.42x** [7.35x-74.65x] | 30.87x | 0.26x | 22087.2 (918.9) | 4024.9 (493.6) | 10222.5 (1395.3) | 18.88x [0.02x-2913.22x] |
| All 31 synthetic corpus families | 31 (293) | **21.29x** [8.95x-50.63x] | 28.57x | 0.23x | 22897.4 (1018.7) | 6536.2 (962.0) | 15426.3 (2078.9) | 18.76x [0.02x-2913.22x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

How to read the aggregate table:
1. **Tier 1 micro-models are dominated by subprocess cost.** On 2-to-26-variable models, Python is bounded by the CBC subprocess spawn and `.sol` parsing floor reported in the provenance block above, whereas pure-Go executes in microseconds in memory.
2. **The planted-gap MAD family is zero-gap by construction.** Its instances have `RootLPBound == OptimalObj` by design and close at the root without branching via presolve orbit fixing and root heuristics. The bold row (`Tier 2+3 non-planted MILPs`) excludes both Tier-1 subprocess micro-models and planted-gap MAD to isolate solver-dominated MILP performance.
3. **Heavy-tail dispersion is visible in wall-clock sums and `[Min-Max]` ranges.** Reporting `Go` Trial (Med) Sum, `CBC` Trial (Med) Sum, `Py` Trial (Med) Sum, and per-instance `[Min-Max]` alongside paired geometric means exposes individual hard seeds (such as heavy-tail trials of `t3_lotsizing_504` where root-only single-row cuts leave a wider tree than CBC's multi-row aggregation).
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
- Every structured instance `t*_mad_g*` has a **zero root LP gap** (`RootLPBound == OptimalObj` with $r_g = 0$), which the root heuristics (`tryPropagatedRounding`, `tryRootSubMIP`, and `tryFractionalDiving`) close at the root without branching (1 explored node, or 2 when `tryRootSubMIP` also explores 1 sub-MIP root node on `t3_mad_g10_i100`).
- [`BuildMADRandom`](../bench/corpus.go) (`t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`, and `TestRandomMADNegativeControls` in [`solver_test.go`](../solver_test.go)) uses the same `BuildMADFromWeights` constraint matrix and assigns dense pseudorandom weights $W_{t,i} \in [100, 2000)$ to every item. Those negative controls have a strict LP-to-MIP gap, the root heuristic does not close it, and the solver must branch.

### MAD Results (`G` Groups $\times$ `I` Items, $T = 6$)

<!-- BEGIN GENERATED:MAD_PIPELINE_TABLE -->
| Problem ID | $G \times I$ | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Trial 0 Obj | Parity |
| :--- | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **114.6x** [88.9x-139.3x] | 0.13 | 16.42 | 0.10 | 16.18 | 0.030 | 0.494 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **17.0x** [5.5x-77.6x] | 3.24 | 43.87 | 3.19 | 43.61 | 0.044 | 0.349 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **24.8x** [15.5x-41.2x] | 0.42 | 11.37 | 0.36 | 11.05 | 0.071 | 0.416 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **42.5x** [29.2x-60.4x] | 0.26 | 12.12 | 0.20 | 11.68 | 0.060 | 0.583 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **12.7x** [7.5x-16.3x] | 0.99 | 13.53 | 0.82 | 12.88 | 0.186 | 0.859 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **2.0x** [0.7x-6.4x] | 24.10 | 50.40 | 23.97 | 50.09 | 0.094 | 0.417 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **1.6x** [0.2x-4.7x] | 130.10 | 192.26 | 129.92 | 191.82 | 0.121 | 1.013 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **16.8x** [9.8x-32.5x] | 1.33 | 22.53 | 0.93 | 21.24 | 0.263 | 1.683 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **6.9x** [4.0x-10.8x] | 3.04 | 19.93 | 2.70 | 18.11 | 0.526 | 2.330 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **4.8x** [3.3x-8.2x] | 18.83 | 87.46 | 16.19 | 74.62 | 1.992 | 20.478 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **5.6x** [3.1x-9.0x] | 74.86 | 500.16 | 69.77 | 434.20 | 5.134 | 54.538 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Cross-Instance Latency Dispersion Across Randomized Instances (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.13 \pm 0.04$ ($\pm 0.03$) | $14.80 \pm 3.48$ ($\pm 2.34$) | $0.13 \pm 0.03$ ($\pm 0.02$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $3.33 \pm 2.09$ ($\pm 1.40$) | $41.90 \pm 12.75$ ($\pm 8.57$) | $3.10 \pm 1.96$ ($\pm 1.32$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.52 \pm 0.14$ ($\pm 0.10$) | $12.78 \pm 2.37$ ($\pm 1.59$) | $0.56 \pm 0.15$ ($\pm 0.10$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.30 \pm 0.07$ ($\pm 0.05$) | $12.22 \pm 0.53$ ($\pm 0.36$) | $0.26 \pm 0.05$ ($\pm 0.03$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $1.14 \pm 0.28$ ($\pm 0.19$) | $14.22 \pm 1.84$ ($\pm 1.24$) | $1.34 \pm 0.45$ ($\pm 0.31$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $24.08 \pm 15.97$ ($\pm 10.73$) | $40.48 \pm 19.53$ ($\pm 13.12$) | $24.50 \pm 17.61$ ($\pm 11.83$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $117.74 \pm 73.20$ ($\pm 49.17$) | $183.27 \pm 91.09$ ($\pm 61.19$) | $129.14 \pm 90.41$ ($\pm 60.73$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $1.38 \pm 0.31$ ($\pm 0.38$) | $23.33 \pm 6.08$ ($\pm 7.55$) | $0.93 \pm 0.05$ ($\pm 0.07$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $3.29 \pm 1.28$ ($\pm 1.59$) | $21.32 \pm 3.41$ ($\pm 4.24$) | $3.57 \pm 1.13$ ($\pm 1.40$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $20.09 \pm 6.09$ ($\pm 7.56$) | $93.70 \pm 17.11$ ($\pm 21.24$) | $21.35 \pm 2.45$ ($\pm 3.04$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $132.68 \pm 132.77$ ($\pm 164.83$) | $783.55 \pm 813.38$ ($\pm 1009.78$) | $135.22 \pm 132.15$ ($\pm 164.07$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | **0.10** (1 / 0) | 0.06 (1 / 0) | 0.08 (1) | 1.56 (4) | 0.111 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **3.19** (10 / 0) | 2.31 (12 / 0) | 2.83 (12) | 4.73 (84) | 0.127 | 4601.00 | 4257.00 |
| `t2_mad_g2_i8` | **0.36** (1 / 0) | 3.33 (9 / 0) | 2.32 (9) | 2.83 (9) | 0.210 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.20** (1 / 0) | 0.22 (1 / 0) | 0.25 (1) | 6.54 (5) | 0.332 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.82** (1 / 0) | 12.86 (25 / 0) | 10.30 (25) | 16.78 (19) | 0.709 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **23.97** (65 / 0) | 9.26 (66 / 0) | 7.23 (66) | 21.48 (333) | 0.222 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **129.92** (254 / 0) | 36.28 (209 / 0) | 36.82 (209) | 250.01 (1253, `TimeLimit`) | 0.600 | 7720.00 | 6060.00 |
| `t3_mad_g5_i20` | **0.93** (1 / 0) | 7.16 (12 / 0) | 6.68 (12) | 5.33 (3) | 1.092 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **2.70** (1 / 0) | 17.68 (12 / 0) | 17.06 (12) | 51.50 (81) | 2.524 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **16.19** (2 / 0) | 250.11 (129 / 0, `TimeLimit`) | 250.63 (174, `TimeLimit`) | 251.12 (88, `TimeLimit`) | 19.578 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **69.77** (1 / 0) | 93.17 (2 / 0) | 111.35 (2) | 259.63 (3, `TimeLimit`) | 181.752 | 208000.00 | 208000.00 |
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
| `t1_std_lp` | 3 | 3 | **1187.2x** [1022.8x-1295.8x] | 0.01 | 8.20 | 0.00 | 8.13 | 0.003 | 0.102 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **1119.6x** [588.1x-1550.7x] | 0.01 | 7.67 | 0.00 | 7.60 | 0.003 | 0.098 | `OK` |
| `t1_std_mip` | 3 | 3 | **635.0x** [102.4x-1008.6x] | 0.01 | 8.79 | 0.01 | 8.71 | 0.004 | 0.102 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **661.2x** [267.0x-1172.5x] | 0.01 | 9.37 | 0.01 | 9.29 | 0.005 | 0.116 | `OK` |
| `t1_whiskas` | 6 | 5 | **582.2x** [172.6x-905.8x] | 0.02 | 11.62 | 0.01 | 11.48 | 0.008 | 0.194 | `OK` |
| `t1_furniture` | 2 | 3 | **284.9x** [53.9x-792.6x] | 0.02 | 9.37 | 0.02 | 9.30 | 0.003 | 0.094 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **87.8x** [32.7x-260.2x] | 0.12 | 10.94 | 0.11 | 10.82 | 0.011 | 0.144 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **2372.9x** [1809.8x-2913.2x] | 0.00 | 7.85 | 0.00 | 7.79 | 0.003 | 0.091 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **114.6x** [88.9x-139.3x] | 0.13 | 16.42 | 0.10 | 16.18 | 0.030 | 0.494 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **17.0x** [5.5x-77.6x] | 3.24 | 43.87 | 3.19 | 43.61 | 0.044 | 0.349 | `OK` |
| `t2_mkp_30` | 30 | 6 | **4.6x** [1.9x-7.8x] | 7.34 | 39.39 | 7.29 | 39.20 | 0.043 | 0.401 | `OK` |
| `t2_mkp_50` | 50 | 8 | **1.6x** [0.8x-25.1x] | 21.55 | 19.73 | 21.45 | 19.44 | 0.082 | 0.379 | `OK` |
| `t2_cflp_36` | 36 | 11 | **17.5x** [1.8x-146.1x] | 0.53 | 11.23 | 0.48 | 11.03 | 0.034 | 0.253 | `OK` |
| `t2_cflp_49` | 49 | 13 | **11.1x** [4.1x-27.8x] | 0.99 | 12.24 | 0.93 | 12.01 | 0.047 | 0.301 | `OK` |
| `t2_gap_45` | 45 | 13 | **4.4x** [0.7x-21.1x] | 3.62 | 20.16 | 3.56 | 19.94 | 0.057 | 0.286 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **4.0x** [1.8x-11.7x] | 14.74 | 89.13 | 14.66 | 88.85 | 0.084 | 0.343 | `OK` |
| `t2_binpack_44` | 44 | 14 | **5.3x** [0.1x-19.0x] | 1.37 | 16.43 | 1.28 | 16.19 | 0.051 | 0.300 | `OK` |
| `t2_setcover_50` | 50 | 20 | **27.5x** [1.8x-89.5x] | 0.35 | 12.96 | 0.29 | 12.63 | 0.080 | 0.442 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **24.8x** [15.5x-41.2x] | 0.42 | 11.37 | 0.36 | 11.05 | 0.071 | 0.416 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **42.5x** [29.2x-60.4x] | 0.26 | 12.12 | 0.20 | 11.68 | 0.060 | 0.583 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **12.7x** [7.5x-16.3x] | 0.99 | 13.53 | 0.82 | 12.88 | 0.186 | 0.859 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **2.0x** [0.7x-6.4x] | 24.10 | 50.40 | 23.97 | 50.09 | 0.094 | 0.417 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **1.6x** [0.2x-4.7x] | 130.10 | 192.26 | 129.92 | 191.82 | 0.121 | 1.013 | `OK` |
| `t3_mkp_200` | 200 | 8 | **0.8x** [0.7x-1.0x] | 173.53 | 121.97 | 173.20 | 121.28 | 0.204 | 0.903 | `OK` |
| `t3_cflp_300` | 300 | 34 | **9.0x** [5.3x-13.5x] | 32.12 | 379.91 | 31.89 | 378.91 | 0.178 | 1.292 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **0.3x** [0.0x-1.0x] | 473.01 | 211.62 | 472.70 | 209.65 | 0.336 | 3.808 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **3.2x** [2.0x-5.1x] | 32.09 | 100.24 | 31.39 | 94.46 | 0.466 | 6.033 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **16.8x** [9.8x-32.5x] | 1.33 | 22.53 | 0.93 | 21.24 | 0.263 | 1.683 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **6.9x** [4.0x-10.8x] | 3.04 | 19.93 | 2.70 | 18.11 | 0.526 | 2.330 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **4.8x** [3.3x-8.2x] | 18.83 | 87.46 | 16.19 | 74.62 | 1.992 | 20.478 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **5.6x** [3.1x-9.0x] | 74.86 | 500.16 | 69.77 | 434.20 | 5.134 | 54.538 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## 5. Default Build vs. `GOEXPERIMENT=simd`

In [`internal/simd/kernels_amd64.go`](../internal/simd/kernels_amd64.go), AVX2 kernels (gated at runtime on CPU `AVX2` and `FMA` support) spill 4-lane accumulators to the stack via `StoreArray` and execute `archsimd.ClearAVXUpperBits()` (`VZEROUPPER`) before scalar tail or caller operations, eliminating AVX-to-SSE dirty-upper `YMM` transition stalls while preserving `NaN` propagation parity. From the kernel dispatch thresholds upward (`minAVX2Len = 32` for `Dot64`, `Axpy64`, `Scale64`, `AbsMax64`, `RowActivityBoundsFinite`, `MinReducedSlice64`, and `InitReducedRow64`; $N \ge 256$ for `Min2ReducedRow64`; and caller-gated at $N \ge 256$ in `assign/solver.go` for `AllAbsLE64`), the 256-bit AVX2 loops outperform the 4-way slice-stepped scalar loops; below those lengths, call and `VZEROUPPER` overhead dominates, so shorter slices stay on the scalar path.

At the full-solver level, each ratio below is the geometric mean of per-trial `scalar / SIMD` solve times over the same instances, with a 95% $t$-interval on the log ratios; "no detectable effect" means the interval contains 1. "Reachable" is a necessary condition: the AVX2 `Dot64`/`Axpy64`/`Scale64` paths require a slice of length 32 or more (`n + m >= 32`). Below that size no AVX2 kernel can run, so those 8 families serve as negative controls where intervals excluding 1 reflect binary code-layout or run-to-run variance rather than SIMD execution. Lowering `minAVX2Len` from 256 to 32 enables AVX2 across dense tableau row accumulation (`ComputeTableauRow`) for medium models with $n \le 256$ (`DenseThreshold`) and dual multiplier/reduced-cost updates inside `runDualSimplexLoop`, while `ComputeTableauRow` dispatches `Axpy64` on contiguous CSR column blocks in large models ($n > 256$, such as `t3_mad_g4_i1000`).

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 0 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 2 faster, 1 slower, 20 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 0.87x [0.72x-1.05x] | 0.003 | 0.003 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 0.95x [0.84x-1.09x] | 0.003 | 0.003 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 1.06x [0.84x-1.36x] | 0.007 | 0.006 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source | 0.82x [0.54x-1.24x] | 0.007 | 0.008 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 1.04x [0.94x-1.15x] | 0.009 | 0.009 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 0.93x [0.78x-1.11x] | 0.018 | 0.019 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source | 1.01x [0.87x-1.18x] | 0.106 | 0.107 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 0.87x [0.62x-1.22x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 1.08x [0.94x-1.23x] | 0.102 | 0.092 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 1.06x [0.98x-1.15x] | 3.191 | 3.168 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 1.07x [0.96x-1.20x] | 7.288 | 6.659 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 1.02x [0.90x-1.14x] | 21.445 | 22.093 | 11 |
| `t2_cflp_36` | 47 | possible | no detectable effect | 0.96x [0.80x-1.15x] | 0.480 | 0.555 | 11 |
| `t2_cflp_49` | 62 | possible | no detectable effect | 0.87x [0.74x-1.01x] | 0.931 | 1.418 | 11 |
| `t2_gap_45` | 58 | possible | SIMD faster | 1.19x [1.01x-1.40x] | 3.557 | 2.173 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 1.02x [0.92x-1.13x] | 14.660 | 18.650 | 11 |
| `t2_binpack_44` | 58 | possible | no detectable effect | 1.08x [0.95x-1.23x] | 1.282 | 1.160 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 1.00x [0.85x-1.18x] | 0.292 | 0.285 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 0.93x [0.81x-1.07x] | 0.357 | 0.389 | 11 |
| `t2_mad_g3_i9` | 108 | possible | no detectable effect | 1.14x [0.98x-1.31x] | 0.198 | 0.190 | 11 |
| `t2_mad_g4_i12` | 156 | possible | no detectable effect | 0.83x [0.63x-1.09x] | 0.815 | 0.859 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.08x [0.94x-1.24x] | 23.967 | 23.016 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 0.95x [0.89x-1.03x] | 129.920 | 139.484 | 11 |
| `t3_mkp_200` | 208 | possible | SIMD faster | 1.16x [1.04x-1.28x] | 173.205 | 153.087 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 0.86x [0.67x-1.11x] | 31.888 | 38.704 | 5 |
| `t3_lotsizing_504` | 840 | possible | SIMD slower | 0.85x [0.77x-0.95x] | 472.699 | 524.572 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 1.02x [0.78x-1.33x] | 31.390 | 28.571 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 1.25x [0.97x-1.62x] | 0.927 | 0.767 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 0.88x [0.60x-1.31x] | 2.700 | 3.122 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 0.90x [0.69x-1.17x] | 16.189 | 19.534 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | no detectable effect | 1.00x [0.73x-1.37x] | 69.765 | 65.975 | 5 |
<!-- END GENERATED:SIMD_EFFECT_TABLE -->

---

## 6. Full Corpus Solver-Core Checkpoint Progression

<!-- BEGIN GENERATED:FULL_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_std_lp` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.003 | 54.00 | 54.00 |
| `t1_std_lp_max` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.003 | 80.00 | 80.00 |
| `t1_std_mip` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.02 (3) | 0.003 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.02 (3) | 0.005 | 64.95 | 58.90 |
| `t1_whiskas` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.006 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.02 (1 / 0) | 0.03 (1) | 0.02 (1) | 0.004 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.11** (1 / 8) | 0.22 (2 / 8) | 0.27 (24) | 0.34 (31) | 0.008 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.002 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.10** (1 / 0) | 0.06 (1 / 0) | 0.08 (1) | 1.56 (4) | 0.111 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **3.19** (10 / 0) | 2.31 (12 / 0) | 2.83 (12) | 4.73 (84) | 0.127 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **7.29** (27 / 9) | 8.10 (48 / 9) | 2.76 (77) | 3.24 (77) | 0.047 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **21.45** (1 / 4) | 36.35 (2 / 4) | 15.10 (132) | 12.66 (119) | 0.053 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.48** (1 / 2) | 4.11 (3 / 2) | 0.90 (7) | 0.49 (7) | 0.051 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.93** (1 / 20) | 3.16 (4 / 20) | 1.19 (9) | 0.91 (9) | 0.042 | 909.02 | 827.30 |
| `t2_gap_45` | **3.56** (1 / 9) | 0.80 (1 / 9) | 6.93 (61) | 4.42 (61) | 0.038 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **14.66** (5 / 37) | 8.78 (8 / 37) | 11.53 (198) | 17.07 (174) | 0.197 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **1.28** (2 / 7) | 2.36 (6 / 7) | 71.63 (1848) | 98.28 (2401) | 0.043 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.29** (1 / 6) | 0.96 (2 / 6) | 1.95 (9) | 2.09 (9) | 0.063 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.36** (1 / 0) | 3.33 (9 / 0) | 2.32 (9) | 2.83 (9) | 0.210 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.20** (1 / 0) | 0.22 (1 / 0) | 0.25 (1) | 6.54 (5) | 0.332 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.82** (1 / 0) | 12.86 (25 / 0) | 10.30 (25) | 16.78 (19) | 0.709 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **23.97** (65 / 0) | 9.26 (66 / 0) | 7.23 (66) | 21.48 (333) | 0.222 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **129.92** (254 / 0) | 36.28 (209 / 0) | 36.82 (209) | 250.01 (1253, `TimeLimit`) | 0.600 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **173.20** (590 / 12) | 202.35 (701 / 12) | 250.06 (2040, `TimeLimit`) | 250.09 (3790, `TimeLimit`) | 0.230 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **31.89** (38 / 31) | 39.91 (38 / 31) | 24.56 (134) | 19.98 (134) | 0.219 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **472.70** (35 / 205) | 250.44 (242 / 205, `TimeLimit`) | 250.23 (421, `TimeLimit`) | 250.02 (495, `TimeLimit`) | 2.422 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **31.39** (64 / 46) | 93.79 (206 / 46) | 250.17 (618, `TimeLimit`) | 250.26 (1000, `TimeLimit`) | 0.271 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.93** (1 / 0) | 7.16 (12 / 0) | 6.68 (12) | 5.33 (3) | 1.092 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **2.70** (1 / 0) | 17.68 (12 / 0) | 17.06 (12) | 51.50 (81) | 2.524 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **16.19** (2 / 0) | 250.11 (129 / 0, `TimeLimit`) | 250.63 (174, `TimeLimit`) | 251.12 (88, `TimeLimit`) | 19.578 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **69.77** (1 / 0) | 93.17 (2 / 0) | 111.35 (2) | 259.63 (3, `TimeLimit`) | 181.752 | 208000.00 | 208000.00 |
<!-- END GENERATED:FULL_CHECKPOINT_TABLE -->

---

## 7. Linear Sum Assignment Benchmarks (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus ([`bench/assign_corpus.go`](../bench/assign_corpus.go), manifest in `testdata/assign_corpus/manifest.json`) evaluates pure-Go [`assign.LinearSumFlat`](../assign/assign.go) against SciPy's compiled C++ `rectangular_lsap` extension (`scipy.optimize.linear_sum_assignment`) across 200 randomized instances in warm state:
- **Bit-identical matrices**: every entry is generated from a 64-bit `SplitMix64` stream (`splitMix64At`) scaled by exact dyadic rationals ($2^{-k}$), and [`bench/scipy_assign_baseline.py`](../bench/scipy_assign_baseline.py) verifies the IEEE-754 SHA-256 digest of every family's canonical matrix against `manifest.json` before timing.
- **Pure in-process solver timing**: `scipy.optimize.linear_sum_assignment` calls its C++ extension in-process on a contiguous `float64` NumPy array, and [`bench/scipy_assign_baseline.py`](../bench/scipy_assign_baseline.py) stops the solve timer immediately upon return before computing the NumPy objective sum, so `SciPy Solve / goblin` compares in-memory solver execution directly.

### 7.1 Aggregate LSAP Workload Summary

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `SciPy Solve / goblin` GeoMean [Clustered 95% CI] | Pooled Trial `SciPy Solve / goblin` | Equal-Family `SciPy Total / goblin` | `goblin` Solve Trial (Med) Sum (ms) | SciPy Solve Trial (Med) Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **2.31x** [1.92x-2.77x] | 2.31x | 7.46x | 0.211 (0.0166) | 0.461 (0.0420) | 2.35x [0.56x-4.76x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **4.45x** [2.82x-7.01x] | 4.45x | 4.46x | 8.911 (0.786) | 47.629 (4.297) | 4.07x [1.55x-19.66x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **3.73x** [2.70x-5.16x] | 3.73x | 4.73x | 325.979 (59.628) | 1422.781 (284.864) | 3.71x [2.11x-8.21x] |
| Square families ($m = n$) | 16 (146) | **3.84x** [2.89x-5.10x] | 3.85x | 5.51x | 330.347 (59.576) | 1454.134 (286.216) | 3.72x [0.60x-19.66x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **2.79x** [1.96x-3.98x] | 2.62x | 4.56x | 4.754 (0.855) | 16.737 (2.987) | 2.99x [0.56x-4.97x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **3.42x** [1.73x-6.78x] | 3.66x | 4.40x | 35.229 (6.180) | 95.179 (15.403) | 2.78x [0.84x-19.66x] |
| **All 22 synthetic LSAP families** | 22 (200) | **3.52x** [2.82x-4.39x] | 3.47x | 5.23x | 335.101 (60.430) | 1470.871 (289.203) | 3.27x [0.56x-19.66x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 7.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **2.42x** | 0.0006 | 0.0016 | 0.0007 | 0.0229 | $0.0008 \pm 0.0007$ | $0.0017 \pm 0.0004$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **2.57x** | 0.0014 | 0.0036 | 0.0017 | 0.0191 | $0.0014 \pm 0.0003$ | $0.0036 \pm 0.0006$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **2.65x** | 0.0029 | 0.0090 | 0.0046 | 0.0265 | $0.0037 \pm 0.0023$ | $0.0088 \pm 0.0019$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.89x** | 0.0031 | 0.0059 | 0.0048 | 0.0250 | $0.0033 \pm 0.0010$ | $0.0060 \pm 0.0010$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.81x** | 0.0034 | 0.0065 | 0.0046 | 0.0259 | $0.0040 \pm 0.0024$ | $0.0066 \pm 0.0010$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **2.66x** | 0.0052 | 0.0154 | 0.0092 | 0.0538 | $0.0060 \pm 0.0023$ | $0.0152 \pm 0.0020$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **3.72x** | 0.0262 | 0.0986 | 0.0332 | 0.138 | $0.0269 \pm 0.0057$ | $0.0989 \pm 0.0148$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **4.91x** | 0.0526 | 0.254 | 0.0717 | 0.327 | $0.0513 \pm 0.0044$ | $0.254 \pm 0.0381$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **3.17x** | 0.0458 | 0.134 | 0.0704 | 0.205 | $0.0442 \pm 0.0089$ | $0.139 \pm 0.0204$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **2.37x** | 0.137 | 0.309 | 0.145 | 0.375 | $0.143 \pm 0.0445$ | $0.331 \pm 0.0632$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **18.02x** | 0.0562 | 1.044 | 0.0743 | 1.170 | $0.0582 \pm 0.0065$ | $1.044 \pm 0.0252$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **3.45x** | 0.0317 | 0.106 | 0.0558 | 0.211 | $0.0309 \pm 0.0034$ | $0.106 \pm 0.0089$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.94x** | 0.0380 | 0.118 | 0.0626 | 0.231 | $0.0422 \pm 0.0106$ | $0.122 \pm 0.0121$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **4.90x** | 0.0821 | 0.402 | 0.109 | 0.527 | $0.0832 \pm 0.0075$ | $0.409 \pm 0.0482$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **5.56x** | 0.317 | 1.833 | 0.428 | 2.512 | $0.330 \pm 0.0497$ | $1.827 \pm 0.188$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.76x** | 1.215 | 5.677 | 1.633 | 9.173 | $1.256 \pm 0.134$ | $5.990 \pm 0.779$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **2.38x** | 0.784 | 1.902 | 1.349 | 5.335 | $0.775 \pm 0.0503$ | $1.856 \pm 0.231$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **2.43x** | 5.149 | 11.990 | 5.428 | 14.452 | $5.709 \pm 1.944$ | $13.797 \pm 4.572$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **4.17x** | 0.318 | 1.263 | 0.777 | 5.491 | $0.317 \pm 0.0246$ | $1.325 \pm 0.161$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **3.27x** | 0.460 | 1.488 | 0.859 | 5.895 | $0.457 \pm 0.0237$ | $1.494 \pm 0.0387$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **6.06x** | 6.735 | 43.854 | 8.877 | 57.750 | $6.720 \pm 0.361$ | $41.428 \pm 8.349$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.43x** | 44.966 | 218.690 | 55.004 | 279.255 | $49.962 \pm 10.389$ | $218.666 \pm 24.285$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 7.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSlice64`, `InitReducedRow64`, and `Min2ReducedRow64`)

In [`assign/solver.go`](../assign/solver.go), [`simd.AllAbsLE64With`](../internal/simd/kernels_amd64.go) validates finite cost bounds across the input matrix via 8-lane AVX2 `Abs().LessEqual` checks when `len(cost) >= 256`, [`simd.MinReducedSlice64With`](../internal/simd/kernels_amd64.go) accelerates reduction-transfer slack scans in `preprocessSquareJV`, [`simd.Min2ReducedRow64With`](../internal/simd/kernels_amd64.go) accelerates the first/second reduced-minimum scans in `preprocessRectangularBV` and `augmentingRowReduction` via 8-way AVX2 `VSUBPD` and `VCMPPD`/`VPAND`/`VMOVMSKPD` bitmask filtering when $\max(m, n) \ge 256$, and [`simd.InitReducedRow64With`](../internal/simd/kernels_amd64.go) runs on a slice of length $\max(m, n)$ at the start of each Dijkstra augmentation when $\max(m, n) \ge 32$. All vector kernels clean upper `YMM` state via `StoreArray` + `ClearAVXUpperBits()` (`VZEROUPPER`) before scalar operations; exact per-family verdicts appear in the generated table below.

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` and `rows * cols < 256`, no AVX2 kernel runs): 0 of 2 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32` or `AllAbsLE64` at `rows * cols >= 256`): 2 faster, 4 slower, 14 with no detectable effect, out of 20.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source | 1.01x [0.62x-1.64x] | 0.0006 | 0.0006 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source | 0.92x [0.74x-1.16x] | 0.0014 | 0.0015 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | possible | no detectable effect | 0.95x [0.68x-1.32x] | 0.0029 | 0.0040 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | no detectable effect | 1.00x [0.83x-1.21x] | 0.0031 | 0.0031 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | no detectable effect | 0.99x [0.64x-1.55x] | 0.0034 | 0.0035 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | no detectable effect | 0.93x [0.66x-1.30x] | 0.0052 | 0.0065 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | no detectable effect | 0.97x [0.81x-1.16x] | 0.0262 | 0.0271 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD slower | 0.92x [0.87x-0.97x] | 0.0526 | 0.0561 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD slower | 0.85x [0.75x-0.97x] | 0.0458 | 0.0466 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD slower | 0.96x [0.92x-0.99x] | 0.137 | 0.137 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | no detectable effect | 1.05x [0.97x-1.13x] | 0.0562 | 0.0559 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | no detectable effect | 1.10x [0.92x-1.30x] | 0.0317 | 0.0263 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | no detectable effect | 1.05x [0.90x-1.22x] | 0.0380 | 0.0396 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD slower | 0.86x [0.80x-0.92x] | 0.0821 | 0.0957 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | no detectable effect | 1.03x [0.98x-1.08x] | 0.317 | 0.324 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | no detectable effect | 0.95x [0.62x-1.45x] | 1.215 | 1.161 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.21x [1.14x-1.28x] | 0.784 | 0.631 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | no detectable effect | 1.04x [0.99x-1.10x] | 5.149 | 4.842 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.54x [1.23x-1.92x] | 0.318 | 0.206 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | no detectable effect | 1.16x [0.98x-1.37x] | 0.460 | 0.386 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | no detectable effect | 1.08x [0.88x-1.31x] | 6.735 | 6.529 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | no detectable effect | 1.07x [0.92x-1.25x] | 44.966 | 43.330 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

---

## 8. Keeping Documentation and Benchmarks Synchronized

1. **Regenerate telemetry** from a clean, committed tree in a single pinned session so that all seven telemetry files record identical `git_commit`, `source_fingerprint`, and `uname_release`: run the commands in the [README](../README.md#verification-benchmarking-and-documentation-sync-commands).
2. **Regenerate tables**: `go run ./cmd/goblin-bench -sync-docs`.
3. **Verify**: `go run ./cmd/goblin-bench -check-docs`, or `go test ./...`, which runs `TestDocumentationAndBenchmarkSync`. The gate validates corpus dimensions, parity, symbol identifiers, and provenance, and checks every generated block in `README.md`, `docs/benchmarks.md`, and `bench/results/final_report.md`.
