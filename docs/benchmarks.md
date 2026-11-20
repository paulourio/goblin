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
- **Unified telemetry source**: fingerprint `8870ee172dd85c23` (working tree on top of commit `c15436234b905cfb89496cd75752329cd42b129d`), `go1.27.3-X:simd`, Linux kernel `6.18.63-gentoo-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, or kernel differs.
- **CPU affinity**: goblin `6`; Python + CBC `6`; Python + SciPy `6`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.66 ms median on a 1-variable MILP and 6.47 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
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
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **283.65x** [122.02x-659.40x] | 283.65x | 0.06x | 20.1 (1.9) | 198.0 (19.5) | 1119.7 (105.4) | 413.05x [10.66x-2170.48x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **12.37x** [6.79x-22.56x] | 12.37x | 0.25x | 1346.8 (70.7) | 1991.3 (176.1) | 4444.5 (428.1) | 15.53x [0.09x-151.42x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **4.62x** [1.75x-12.18x] | 4.62x | 0.45x | 8844.7 (332.5) | 2940.1 (567.6) | 6259.3 (1077.5) | 5.01x [0.03x-27.94x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **6.07x** [3.20x-11.54x] | 7.24x | 0.43x | 9686.3 (339.1) | 3164.3 (401.7) | 6907.8 (1000.4) | 6.46x [0.03x-151.42x] |
| Tier 3 non-MAD MILPs | 4 (20) | **2.34x** [0.35x-15.61x] | 2.34x | 0.92x | 8350.5 (269.3) | 1262.1 (233.3) | 2827.1 (604.8) | 2.36x [0.03x-14.41x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **20.41x** [8.57x-48.61x] | 27.62x | 0.14x | 506.5 (64.2) | 1792.3 (344.5) | 3907.1 (515.4) | 31.95x [3.33x-106.90x] |
| Random-weight MAD (negative controls) | 3 (33) | **7.83x** [0.54x-114.08x] | 7.83x | 0.30x | 672.3 (46.2) | 1239.0 (120.5) | 2288.0 (252.0) | 8.02x [0.71x-75.81x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **28.81x** [10.78x-76.99x] | 37.47x | 0.20x | 9705.1 (340.9) | 3337.0 (418.7) | 7916.5 (1095.6) | 28.83x [0.03x-2170.48x] |
| All 31 synthetic corpus families | 31 (293) | **26.36x** [12.58x-55.21x] | 35.06x | 0.18x | 10211.6 (405.1) | 5129.4 (763.2) | 11823.6 (1610.9) | 29.79x [0.03x-2170.48x] |
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
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **84.5x** [67.5x-106.9x] | 0.12 | 10.14 | 0.08 | 9.82 | 0.046 | 0.295 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **26.4x** [10.7x-75.8x] | 1.63 | 34.59 | 1.59 | 34.36 | 0.037 | 0.301 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **50.3x** [33.9x-65.4x] | 0.18 | 9.92 | 0.15 | 9.64 | 0.037 | 0.385 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **45.6x** [29.3x-55.8x] | 0.22 | 10.78 | 0.15 | 10.37 | 0.062 | 0.541 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **22.6x** [13.0x-31.5x] | 0.48 | 11.77 | 0.39 | 11.17 | 0.113 | 0.797 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **5.4x** [2.2x-15.8x] | 7.20 | 48.53 | 7.12 | 48.23 | 0.048 | 0.394 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **3.4x** [0.7x-13.3x] | 37.42 | 168.88 | 37.32 | 168.45 | 0.080 | 0.565 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **22.0x** [17.7x-27.9x] | 0.79 | 18.60 | 0.59 | 17.47 | 0.184 | 1.412 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **11.1x** [7.4x-16.9x] | 2.13 | 19.61 | 1.70 | 17.88 | 0.295 | 2.291 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **4.6x** [4.0x-5.0x] | 13.75 | 61.85 | 12.44 | 54.18 | 1.176 | 10.950 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **6.2x** [3.3x-8.2x] | 46.54 | 372.69 | 41.83 | 337.85 | 4.578 | 45.389 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Cross-Instance Latency Dispersion Across Randomized Instances (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.12 \pm 0.02$ ($\pm 0.01$) | $10.10 \pm 0.54$ ($\pm 0.36$) | $0.15 \pm 0.09$ ($\pm 0.06$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $1.48 \pm 0.87$ ($\pm 0.59$) | $31.41 \pm 9.59$ ($\pm 6.44$) | $1.51 \pm 0.89$ ($\pm 0.60$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.21 \pm 0.04$ ($\pm 0.03$) | $10.15 \pm 0.77$ ($\pm 0.52$) | $0.21 \pm 0.05$ ($\pm 0.03$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.24 \pm 0.06$ ($\pm 0.04$) | $10.72 \pm 0.22$ ($\pm 0.15$) | $0.23 \pm 0.05$ ($\pm 0.03$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.55 \pm 0.13$ ($\pm 0.09$) | $12.21 \pm 0.95$ ($\pm 0.64$) | $0.53 \pm 0.07$ ($\pm 0.05$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $8.25 \pm 6.13$ ($\pm 4.12$) | $36.26 \pm 17.18$ ($\pm 11.54$) | $8.19 \pm 6.19$ ($\pm 4.16$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $51.38 \pm 44.70$ ($\pm 30.03$) | $140.33 \pm 63.80$ ($\pm 42.86$) | $51.63 \pm 44.81$ ($\pm 30.11$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.83 \pm 0.10$ ($\pm 0.13$) | $18.31 \pm 2.26$ ($\pm 2.81$) | $0.87 \pm 0.13$ ($\pm 0.16$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.86 \pm 0.63$ ($\pm 0.78$) | $19.59 \pm 0.35$ ($\pm 0.43$) | $1.83 \pm 0.61$ ($\pm 0.76$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $14.01 \pm 1.07$ ($\pm 1.32$) | $63.84 \pm 3.63$ ($\pm 4.51$) | $14.25 \pm 1.14$ ($\pm 1.41$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $82.15 \pm 68.83$ ($\pm 85.45$) | $584.70 \pm 618.21$ ($\pm 767.48$) | $83.69 \pm 69.59$ ($\pm 86.39$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | **0.08** (1 / 0) | 0.08 (1 / 0) | 0.09 (1) | 1.09 (4) | 0.087 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **1.59** (10 / 0) | 1.03 (12 / 0) | 1.00 (12) | 2.14 (84) | 0.110 | 4601.00 | 4257.00 |
| `t2_mad_g2_i8` | **0.15** (1 / 0) | 1.12 (9 / 0) | 1.05 (9) | 1.20 (9) | 0.107 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.15** (1 / 0) | 0.14 (1 / 0) | 0.20 (1) | 4.50 (5) | 0.256 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.39** (1 / 0) | 7.10 (25 / 0) | 7.08 (25) | 6.47 (19) | 0.357 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **7.12** (71 / 0) | 3.67 (66 / 0) | 4.45 (66) | 9.26 (333) | 0.144 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **37.32** (262 / 0) | 26.11 (209 / 0) | 25.03 (209) | 247.34 (1682) | 0.319 | 7720.00 | 6060.00 |
| `t3_mad_g5_i20` | **0.59** (1 / 0) | 4.16 (12 / 0) | 4.84 (12) | 3.13 (3) | 0.941 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **1.70** (1 / 0) | 10.68 (12 / 0) | 10.10 (12) | 39.50 (81) | 1.541 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **12.44** (2 / 0) | 250.03 (265 / 0, `TimeLimit`) | 250.16 (267, `TimeLimit`) | 155.79 (105) | 13.551 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **41.83** (1 / 0) | 67.66 (2 / 0) | 63.69 (2) | 216.26 (9) | 115.984 | 208000.00 | 208000.00 |
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
| `t1_std_lp` | 3 | 3 | **777.6x** [388.9x-1077.3x] | 0.01 | 6.64 | 0.00 | 6.57 | 0.005 | 0.074 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **743.1x** [578.6x-1030.4x] | 0.01 | 6.73 | 0.00 | 6.67 | 0.005 | 0.079 | `OK` |
| `t1_std_mip` | 3 | 3 | **380.9x** [188.5x-552.8x] | 0.02 | 7.82 | 0.01 | 7.76 | 0.008 | 0.083 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **337.7x** [256.4x-443.5x] | 0.02 | 7.95 | 0.01 | 7.88 | 0.007 | 0.088 | `OK` |
| `t1_whiskas` | 6 | 5 | **534.1x** [321.7x-701.0x] | 0.01 | 6.70 | 0.00 | 6.61 | 0.007 | 0.104 | `OK` |
| `t1_furniture` | 2 | 3 | **297.9x** [69.6x-795.4x] | 0.02 | 8.37 | 0.02 | 8.31 | 0.003 | 0.084 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **108.5x** [45.1x-311.4x] | 0.10 | 9.86 | 0.09 | 9.76 | 0.009 | 0.123 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **1177.9x** [628.1x-2170.5x] | 0.01 | 6.57 | 0.00 | 6.52 | 0.005 | 0.065 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **84.5x** [67.5x-106.9x] | 0.12 | 10.14 | 0.08 | 9.82 | 0.046 | 0.295 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **26.4x** [10.7x-75.8x] | 1.63 | 34.59 | 1.59 | 34.36 | 0.037 | 0.301 | `OK` |
| `t2_mkp_30` | 30 | 6 | **4.5x** [2.8x-8.4x] | 4.63 | 28.72 | 4.60 | 28.54 | 0.029 | 0.234 | `OK` |
| `t2_mkp_50` | 50 | 8 | **2.7x** [1.1x-22.6x] | 10.14 | 17.78 | 10.10 | 17.52 | 0.048 | 0.349 | `OK` |
| `t2_cflp_36` | 36 | 11 | **20.9x** [1.7x-151.4x] | 0.44 | 9.91 | 0.41 | 9.74 | 0.032 | 0.227 | `OK` |
| `t2_cflp_49` | 49 | 13 | **16.2x** [6.5x-34.2x] | 0.61 | 10.77 | 0.58 | 10.55 | 0.032 | 0.278 | `OK` |
| `t2_gap_45` | 45 | 13 | **13.7x** [1.4x-67.0x] | 1.10 | 14.54 | 1.08 | 14.33 | 0.026 | 0.264 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **7.6x** [3.5x-18.7x] | 7.46 | 69.68 | 7.43 | 69.44 | 0.038 | 0.315 | `OK` |
| `t2_binpack_44` | 44 | 14 | **9.8x** [0.1x-36.5x] | 0.57 | 14.21 | 0.55 | 13.93 | 0.033 | 0.266 | `OK` |
| `t2_setcover_50` | 50 | 20 | **40.7x** [2.9x-115.4x] | 0.23 | 12.59 | 0.16 | 12.28 | 0.049 | 0.421 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **50.3x** [33.9x-65.4x] | 0.18 | 9.92 | 0.15 | 9.64 | 0.037 | 0.385 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **45.6x** [29.3x-55.8x] | 0.22 | 10.78 | 0.15 | 10.37 | 0.062 | 0.541 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **22.6x** [13.0x-31.5x] | 0.48 | 11.77 | 0.39 | 11.17 | 0.113 | 0.797 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **5.4x** [2.2x-15.8x] | 7.20 | 48.53 | 7.12 | 48.23 | 0.048 | 0.394 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **3.4x** [0.7x-13.3x] | 37.42 | 168.88 | 37.32 | 168.45 | 0.080 | 0.565 | `OK` |
| `t3_mkp_200` | 200 | 8 | **1.2x** [1.0x-1.3x] | 92.10 | 116.12 | 92.00 | 115.52 | 0.111 | 0.804 | `OK` |
| `t3_cflp_300` | 300 | 34 | **9.5x** [6.1x-14.4x] | 22.16 | 249.58 | 22.00 | 248.66 | 0.144 | 1.176 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **0.7x** [0.0x-2.0x] | 135.29 | 157.44 | 135.02 | 155.51 | 0.269 | 2.291 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **4.0x** [2.7x-5.0x] | 19.75 | 81.63 | 19.38 | 78.69 | 0.363 | 3.492 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **22.0x** [17.7x-27.9x] | 0.79 | 18.60 | 0.59 | 17.47 | 0.184 | 1.412 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **11.1x** [7.4x-16.9x] | 2.13 | 19.61 | 1.70 | 17.88 | 0.295 | 2.291 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **4.6x** [4.0x-5.0x] | 13.75 | 61.85 | 12.44 | 54.18 | 1.176 | 10.950 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **6.2x** [3.3x-8.2x] | 46.54 | 372.69 | 41.83 | 337.85 | 4.578 | 45.389 | `OK` |
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
| `t1_std_lp` | 6 | no (control) | identical source | 0.99x [0.78x-1.25x] | 0.003 | 0.003 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 1.05x [0.80x-1.38x] | 0.002 | 0.002 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 0.84x [0.57x-1.22x] | 0.009 | 0.011 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source | 1.26x [0.91x-1.75x] | 0.014 | 0.010 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 1.00x [0.86x-1.16x] | 0.005 | 0.005 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 0.98x [0.86x-1.11x] | 0.015 | 0.014 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source | 1.00x [0.90x-1.11x] | 0.088 | 0.088 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 0.82x [0.51x-1.33x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 0.86x [0.66x-1.13x] | 0.075 | 0.073 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 1.00x [0.97x-1.04x] | 1.595 | 1.629 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 1.05x [0.96x-1.14x] | 4.602 | 4.733 | 11 |
| `t2_mkp_50` | 58 | possible | SIMD slower | 0.98x [0.97x-0.99x] | 10.099 | 10.225 | 11 |
| `t2_cflp_36` | 47 | possible | no detectable effect | 1.03x [0.92x-1.14x] | 0.407 | 0.388 | 11 |
| `t2_cflp_49` | 62 | possible | no detectable effect | 1.00x [0.97x-1.02x] | 0.583 | 0.595 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 0.97x [0.93x-1.01x] | 1.075 | 1.061 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 0.98x [0.96x-1.00x] | 7.428 | 7.449 | 11 |
| `t2_binpack_44` | 58 | possible | no detectable effect | 1.02x [0.95x-1.09x] | 0.548 | 0.542 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 1.05x [0.92x-1.19x] | 0.157 | 0.155 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 1.02x [0.99x-1.05x] | 0.146 | 0.148 | 11 |
| `t2_mad_g3_i9` | 108 | possible | no detectable effect | 1.03x [0.94x-1.12x] | 0.155 | 0.147 | 11 |
| `t2_mad_g4_i12` | 156 | possible | no detectable effect | 1.03x [0.90x-1.17x] | 0.390 | 0.393 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.04x [0.95x-1.14x] | 7.124 | 7.208 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 0.98x [0.97x-1.00x] | 37.319 | 38.698 | 11 |
| `t3_mkp_200` | 208 | possible | SIMD faster | 1.01x [1.00x-1.02x] | 92.000 | 90.007 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 0.98x [0.92x-1.04x] | 21.997 | 21.743 | 5 |
| `t3_lotsizing_504` | 840 | possible | no detectable effect | 1.00x [0.98x-1.01x] | 135.018 | 134.055 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 1.02x [0.96x-1.09x] | 19.384 | 19.481 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 0.96x [0.73x-1.26x] | 0.589 | 0.693 | 5 |
| `t3_mad_g6_i30` | 354 | possible | SIMD faster | 1.05x [1.01x-1.09x] | 1.698 | 1.578 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 0.98x [0.92x-1.06x] | 12.439 | 13.483 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | no detectable effect | 0.99x [0.95x-1.03x] | 41.830 | 43.235 | 5 |
<!-- END GENERATED:SIMD_EFFECT_TABLE -->

---

## 6. Full Corpus Solver-Core Checkpoint Progression

<!-- BEGIN GENERATED:FULL_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_std_lp` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 54.00 | 54.00 |
| `t1_std_lp_max` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 80.00 | 80.00 |
| `t1_std_mip` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.002 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.02 (3) | 0.002 | 64.95 | 58.90 |
| `t1_whiskas` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.004 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.02 (1) | 0.01 (1) | 0.003 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.09** (1 / 8) | 0.12 (2 / 8) | 0.20 (24) | 0.26 (31) | 0.007 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.08** (1 / 0) | 0.08 (1 / 0) | 0.09 (1) | 1.09 (4) | 0.087 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **1.59** (10 / 0) | 1.03 (12 / 0) | 1.00 (12) | 2.14 (84) | 0.110 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **4.60** (27 / 9) | 4.44 (48 / 9) | 1.92 (77) | 1.69 (77) | 0.025 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **10.10** (1 / 4) | 26.75 (2 / 4) | 10.61 (132) | 8.95 (119) | 0.045 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.41** (1 / 2) | 1.68 (3 / 2) | 0.25 (7) | 0.23 (7) | 0.025 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.58** (1 / 20) | 1.79 (4 / 20) | 0.54 (9) | 0.51 (9) | 0.028 | 909.02 | 827.30 |
| `t2_gap_45` | **1.08** (1 / 9) | 0.41 (1 / 9) | 2.59 (61) | 2.50 (61) | 0.028 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **7.43** (4 / 37) | 4.92 (8 / 37) | 3.73 (198) | 4.33 (174) | 0.051 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **0.55** (2 / 7) | 1.72 (6 / 7) | 34.39 (1848) | 46.78 (2401) | 0.031 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.16** (1 / 6) | 0.50 (2 / 6) | 1.40 (9) | 1.35 (9) | 0.055 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.15** (1 / 0) | 1.12 (9 / 0) | 1.05 (9) | 1.20 (9) | 0.107 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.15** (1 / 0) | 0.14 (1 / 0) | 0.20 (1) | 4.50 (5) | 0.256 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.39** (1 / 0) | 7.10 (25 / 0) | 7.08 (25) | 6.47 (19) | 0.357 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **7.12** (71 / 0) | 3.67 (66 / 0) | 4.45 (66) | 9.26 (333) | 0.144 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **37.32** (262 / 0) | 26.11 (209 / 0) | 25.03 (209) | 247.34 (1682) | 0.319 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **92.00** (636 / 12) | 119.44 (701 / 12) | 250.03 (4414, `TimeLimit`) | 239.46 (5000, `NodeLimit`) | 0.140 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **22.00** (38 / 31) | 21.55 (38 / 31) | 13.23 (134) | 12.20 (134) | 0.158 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **135.02** (40 / 205) | 171.89 (420 / 205) | 250.03 (1694, `TimeLimit`) | 250.03 (1687, `TimeLimit`) | 0.846 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **19.38** (64 / 46) | 62.11 (206 / 46) | 250.03 (1370, `TimeLimit`) | 250.03 (1707, `TimeLimit`) | 0.234 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.59** (1 / 0) | 4.16 (12 / 0) | 4.84 (12) | 3.13 (3) | 0.941 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **1.70** (1 / 0) | 10.68 (12 / 0) | 10.10 (12) | 39.50 (81) | 1.541 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **12.44** (2 / 0) | 250.03 (265 / 0, `TimeLimit`) | 250.16 (267, `TimeLimit`) | 155.79 (105) | 13.551 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **41.83** (1 / 0) | 67.66 (2 / 0) | 63.69 (2) | 216.26 (9) | 115.984 | 208000.00 | 208000.00 |
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
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.89x** [1.40x-2.56x] | 1.89x | 5.14x | 0.186 (0.0150) | 0.797 (0.0288) | 1.78x [0.65x-381.54x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **3.31x** [2.00x-5.46x] | 3.31x | 3.20x | 8.576 (0.735) | 32.804 (2.949) | 2.99x [0.58x-19.83x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **3.09x** [2.07x-4.63x] | 3.09x | 3.91x | 214.180 (42.060) | 938.177 (186.652) | 3.12x [1.23x-6.48x] |
| Square families ($m = n$) | 16 (146) | **3.02x** [2.20x-4.15x] | 3.00x | 4.08x | 219.061 (42.109) | 960.930 (187.683) | 2.69x [0.58x-381.54x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **2.23x** [1.61x-3.08x] | 2.09x | 3.40x | 3.881 (0.702) | 10.848 (1.947) | 2.28x [0.80x-3.32x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.70x** [1.28x-5.70x] | 2.91x | 3.26x | 34.696 (5.962) | 65.832 (11.148) | 2.07x [0.58x-381.54x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.78x** [2.19x-3.54x] | 2.72x | 3.88x | 222.942 (42.811) | 971.778 (189.630) | 2.52x [0.58x-381.54x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 7.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **2.02x** | 0.0005 | 0.0010 | 0.0006 | 0.0139 | $0.0005 \pm 0.0001$ | $0.0011 \pm 0.0003$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.58x** | 0.0015 | 0.0023 | 0.0019 | 0.0101 | $0.0017 \pm 0.0009$ | $0.0024 \pm 0.0004$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **3.21x** | 0.0026 | 0.0062 | 0.0044 | 0.0151 | $0.0032 \pm 0.0014$ | $0.0499 \pm 0.144$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.47x** | 0.0026 | 0.0040 | 0.0042 | 0.0144 | $0.0028 \pm 0.0011$ | $0.0039 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.60x** | 0.0028 | 0.0044 | 0.0044 | 0.0149 | $0.0029 \pm 0.0011$ | $0.0045 \pm 0.0009$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.93x** | 0.0050 | 0.0109 | 0.0092 | 0.0326 | $0.0058 \pm 0.0025$ | $0.0106 \pm 0.0020$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.83x** | 0.0240 | 0.0702 | 0.0313 | 0.0985 | $0.0251 \pm 0.0081$ | $0.0688 \pm 0.0104$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.56x** | 0.0483 | 0.175 | 0.0635 | 0.224 | $0.0501 \pm 0.0101$ | $0.177 \pm 0.0254$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.17x** | 0.0426 | 0.0937 | 0.0648 | 0.142 | $0.0458 \pm 0.0137$ | $0.0965 \pm 0.0142$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.56x** | 0.132 | 0.219 | 0.140 | 0.257 | $0.158 \pm 0.0676$ | $0.233 \pm 0.0430$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **15.58x** | 0.0486 | 0.686 | 0.0642 | 0.760 | $0.0457 \pm 0.0070$ | $0.707 \pm 0.0623$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **2.52x** | 0.0271 | 0.0693 | 0.0510 | 0.146 | $0.0297 \pm 0.0119$ | $0.0716 \pm 0.0077$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.36x** | 0.0328 | 0.0801 | 0.0578 | 0.154 | $0.0350 \pm 0.0102$ | $0.0804 \pm 0.0068$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.68x** | 0.0760 | 0.280 | 0.0989 | 0.361 | $0.0776 \pm 0.0089$ | $0.285 \pm 0.0335$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **4.07x** | 0.304 | 1.276 | 0.424 | 1.780 | $0.313 \pm 0.0534$ | $1.263 \pm 0.123$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.84x** | 1.170 | 5.166 | 1.580 | 7.902 | $1.438 \pm 0.476$ | $5.329 \pm 0.346$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.89x** | 0.722 | 1.480 | 1.273 | 4.028 | $0.804 \pm 0.225$ | $1.482 \pm 0.0494$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.69x** | 5.009 | 8.653 | 5.202 | 9.525 | $5.567 \pm 1.983$ | $9.271 \pm 2.618$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **3.20x** | 0.270 | 0.826 | 0.639 | 3.770 | $0.264 \pm 0.0207$ | $0.844 \pm 0.0594$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.73x** | 0.367 | 0.963 | 0.736 | 3.849 | $0.357 \pm 0.0270$ | $0.972 \pm 0.0333$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **5.13x** | 5.858 | 28.301 | 7.543 | 38.371 | $5.629 \pm 0.380$ | $28.885 \pm 2.501$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.92x** | 28.665 | 141.263 | 38.715 | 186.836 | $28.777 \pm 3.776$ | $140.852 \pm 8.948$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 7.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSlice64`, `InitReducedRow64`, and `Min2ReducedRow64`)

In [`assign/solver.go`](../assign/solver.go), [`simd.AllAbsLE64With`](../internal/simd/kernels_amd64.go) validates finite cost bounds across the input matrix via 8-lane AVX2 `Abs().LessEqual` checks when `len(cost) >= 256`, [`simd.MinReducedSlice64With`](../internal/simd/kernels_amd64.go) accelerates reduction-transfer slack scans in `preprocessSquareJV`, [`simd.Min2ReducedRow64With`](../internal/simd/kernels_amd64.go) accelerates the first/second reduced-minimum scans in `preprocessRectangularBV` and `augmentingRowReduction` via 8-way AVX2 `VSUBPD` and `VCMPPD`/`VPAND`/`VMOVMSKPD` bitmask filtering when $\max(m, n) \ge 256$, and [`simd.InitReducedRow64With`](../internal/simd/kernels_amd64.go) runs on a slice of length $\max(m, n)$ at the start of each Dijkstra augmentation when $\max(m, n) \ge 32$. All vector kernels clean upper `YMM` state via `StoreArray` + `ClearAVXUpperBits()` (`VZEROUPPER`) before scalar operations; exact per-family verdicts appear in the generated table below.

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` and `rows * cols < 256`, no AVX2 kernel runs): 0 of 2 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32` or `AllAbsLE64` at `rows * cols >= 256`): 16 faster, 0 slower, 4 with no detectable effect, out of 20.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source | 0.72x [0.47x-1.12x] | 0.0005 | 0.0005 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source | 0.95x [0.56x-1.60x] | 0.0015 | 0.0012 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | possible | no detectable effect | 0.82x [0.57x-1.19x] | 0.0026 | 0.0037 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | no detectable effect | 1.08x [0.80x-1.46x] | 0.0026 | 0.0026 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | no detectable effect | 1.04x [0.77x-1.42x] | 0.0028 | 0.0026 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | no detectable effect | 0.97x [0.70x-1.36x] | 0.0050 | 0.0060 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.35x [1.21x-1.51x] | 0.0240 | 0.0194 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.38x [1.22x-1.55x] | 0.0483 | 0.0353 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.29x [1.04x-1.59x] | 0.0426 | 0.0286 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.84x [1.49x-2.27x] | 0.132 | 0.0758 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.27x [1.17x-1.39x] | 0.0486 | 0.0351 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.34x [1.08x-1.66x] | 0.0271 | 0.0210 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.41x [1.21x-1.65x] | 0.0328 | 0.0247 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.51x [1.38x-1.64x] | 0.0760 | 0.0494 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.43x [1.34x-1.53x] | 0.304 | 0.221 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.82x [1.13x-2.92x] | 1.170 | 0.792 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.15x [1.47x-3.15x] | 0.722 | 0.365 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.34x [2.21x-2.48x] | 5.009 | 2.055 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.59x [1.25x-2.03x] | 0.270 | 0.163 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.46x [1.22x-1.75x] | 0.367 | 0.248 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.54x [1.39x-1.69x] | 5.858 | 3.679 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.50x [1.41x-1.59x] | 28.665 | 18.772 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

---

## 8. Keeping Documentation and Benchmarks Synchronized

1. **Regenerate telemetry** from a clean, committed tree in a single pinned session so that all seven telemetry files record identical `git_commit`, `source_fingerprint`, and `uname_release`: run the commands in the [README](../README.md#verification-benchmarking-and-documentation-sync-commands).
2. **Regenerate tables**: `go run ./cmd/goblin-bench -sync-docs`.
3. **Verify**: `go run ./cmd/goblin-bench -check-docs`, or `go test ./...`, which runs `TestDocumentationAndBenchmarkSync`. The gate validates corpus dimensions, parity, symbol identifiers, and provenance, and checks every generated block in `README.md`, `docs/benchmarks.md`, and `bench/results/final_report.md`.
