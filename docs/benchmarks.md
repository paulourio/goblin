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
- **Unified telemetry source**: fingerprint `6a0ff4872f1c417c` (working tree on top of commit `85d57a31dbae83971e3c933fa305d29684ef5a14`), `go1.27.3-X:simd`, Linux kernel `6.18.63-gentoo-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, or kernel differs.
- **CPU affinity**: goblin `6`; Python + CBC `6`; Python + SciPy `6`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 7.17 ms median on a 1-variable MILP and 6.66 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
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
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **361.22x** [143.92x-906.59x] | 361.22x | 0.05x | 21.1 (2.0) | 193.2 (19.3) | 1134.3 (106.7) | 604.64x [10.19x-2026.59x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **11.19x** [5.90x-21.20x] | 11.19x | 0.27x | 1545.5 (83.3) | 1995.0 (177.8) | 4439.3 (427.2) | 13.52x [0.07x-152.38x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **4.30x** [1.44x-12.87x] | 4.30x | 0.49x | 10742.5 (469.1) | 2924.2 (566.0) | 6281.9 (1079.2) | 5.73x [0.03x-28.65x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **5.17x** [2.61x-10.23x] | 6.22x | 0.51x | 11786.0 (488.0) | 3152.2 (402.4) | 6893.3 (998.2) | 6.06x [0.03x-152.38x] |
| Tier 3 non-MAD MILPs | 4 (20) | **1.90x** [0.22x-16.03x] | 1.90x | 1.14x | 10250.9 (405.5) | 1253.5 (232.8) | 2824.2 (604.2) | 1.68x [0.03x-13.02x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **22.38x** [9.16x-54.63x] | 30.53x | 0.13x | 503.0 (64.5) | 1791.4 (343.9) | 3939.7 (518.5) | 32.85x [3.18x-147.17x] |
| Random-weight MAD (negative controls) | 3 (33) | **7.38x** [0.50x-109.13x] | 7.38x | 0.33x | 729.5 (53.8) | 1236.3 (121.2) | 2287.0 (250.2) | 7.39x [0.48x-76.34x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **28.56x** [9.73x-83.81x] | 37.95x | 0.20x | 11806.1 (489.9) | 3321.0 (419.2) | 7915.8 (1094.7) | 24.41x [0.03x-2026.59x] |
| All 31 synthetic corpus families | 31 (293) | **26.82x** [11.98x-60.01x] | 36.19x | 0.18x | 12309.0 (554.4) | 5112.3 (763.1) | 11855.5 (1613.2) | 28.65x [0.03x-2026.59x] |
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
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **120.0x** [57.9x-147.2x] | 0.08 | 10.26 | 0.05 | 10.03 | 0.024 | 0.296 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **24.8x** [10.2x-76.3x] | 1.71 | 34.24 | 1.67 | 34.00 | 0.030 | 0.305 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **50.6x** [35.5x-62.2x] | 0.18 | 10.25 | 0.15 | 9.93 | 0.035 | 0.391 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **45.5x** [32.5x-57.7x] | 0.21 | 10.74 | 0.16 | 10.29 | 0.056 | 0.552 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **25.0x** [20.5x-30.3x] | 0.48 | 12.20 | 0.38 | 11.58 | 0.091 | 0.812 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **5.2x** [2.2x-19.0x] | 7.22 | 48.64 | 7.16 | 48.32 | 0.058 | 0.402 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **3.1x** [0.5x-12.6x] | 44.85 | 167.37 | 44.77 | 166.94 | 0.082 | 0.569 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **22.2x** [14.5x-28.7x] | 0.78 | 19.20 | 0.60 | 18.02 | 0.196 | 1.382 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **11.8x** [8.9x-20.0x] | 1.76 | 19.26 | 1.52 | 17.50 | 0.282 | 2.305 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **5.6x** [5.0x-6.5x] | 11.17 | 62.28 | 10.07 | 54.60 | 1.221 | 10.838 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **6.1x** [3.2x-7.9x] | 49.82 | 374.33 | 43.67 | 339.00 | 4.601 | 45.480 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Cross-Instance Latency Dispersion Across Randomized Instances (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.09 \pm 0.03$ ($\pm 0.02$) | $10.17 \pm 0.53$ ($\pm 0.36$) | $0.09 \pm 0.01$ ($\pm 0.01$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $1.62 \pm 1.00$ ($\pm 0.67$) | $31.19 \pm 9.51$ ($\pm 6.39$) | $1.61 \pm 0.97$ ($\pm 0.65$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.21 \pm 0.04$ ($\pm 0.03$) | $10.44 \pm 0.60$ ($\pm 0.40$) | $0.23 \pm 0.07$ ($\pm 0.05$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.24 \pm 0.05$ ($\pm 0.04$) | $10.75 \pm 0.18$ ($\pm 0.12$) | $0.25 \pm 0.07$ ($\pm 0.05$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.50 \pm 0.06$ ($\pm 0.04$) | $12.47 \pm 0.68$ ($\pm 0.46$) | $0.52 \pm 0.08$ ($\pm 0.05$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $8.89 \pm 6.68$ ($\pm 4.49$) | $36.29 \pm 17.18$ ($\pm 11.54$) | $8.74 \pm 6.61$ ($\pm 4.44$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $55.80 \pm 46.72$ ($\pm 31.38$) | $140.43 \pm 64.13$ ($\pm 43.08$) | $56.36 \pm 47.08$ ($\pm 31.63$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.84 \pm 0.14$ ($\pm 0.17$) | $18.71 \pm 2.35$ ($\pm 2.92$) | $0.78 \pm 0.09$ ($\pm 0.11$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.70 \pm 0.49$ ($\pm 0.60$) | $19.29 \pm 0.10$ ($\pm 0.12$) | $1.60 \pm 0.51$ ($\pm 0.63$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $11.39 \pm 1.50$ ($\pm 1.87$) | $63.97 \pm 3.40$ ($\pm 4.23$) | $11.25 \pm 0.60$ ($\pm 0.74$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $84.37 \pm 72.50$ ($\pm 90.00$) | $589.57 \pm 615.72$ ($\pm 764.39$) | $85.82 \pm 76.74$ ($\pm 95.28$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | **0.05** (1 / 0) | 0.06 (1 / 0) | 0.07 (1) | 1.01 (4) | 0.083 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **1.67** (10 / 0) | 1.10 (12 / 0) | 1.04 (12) | 2.22 (84) | 0.101 | 4601.00 | 4257.00 |
| `t2_mad_g2_i8` | **0.15** (1 / 0) | 1.18 (9 / 0) | 1.52 (9) | 1.25 (9) | 0.103 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.16** (1 / 0) | 0.21 (1 / 0) | 0.21 (1) | 4.14 (5) | 0.272 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.38** (1 / 0) | 7.52 (25 / 0) | 7.48 (25) | 7.20 (19) | 0.350 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **7.16** (65 / 0) | 4.02 (66 / 0) | 3.67 (66) | 9.19 (333) | 0.160 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **44.77** (254 / 0) | 26.56 (209 / 0) | 26.54 (209) | 250.03 (1664, `TimeLimit`) | 0.303 | 7720.00 | 6060.00 |
| `t3_mad_g5_i20` | **0.60** (1 / 0) | 4.48 (12 / 0) | 5.20 (12) | 3.24 (3) | 0.977 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **1.52** (1 / 0) | 10.99 (12 / 0) | 10.31 (12) | 39.85 (81) | 1.595 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **10.07** (2 / 0) | 250.43 (245 / 0, `TimeLimit`) | 250.11 (248, `TimeLimit`) | 156.41 (105) | 13.926 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **43.67** (1 / 0) | 68.87 (2 / 0) | 67.16 (2) | 209.90 (9) | 114.463 | 208000.00 | 208000.00 |
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
| `t1_std_lp` | 3 | 3 | **802.0x** [524.9x-1203.9x] | 0.01 | 6.92 | 0.00 | 6.86 | 0.005 | 0.080 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **1026.2x** [641.8x-1275.0x] | 0.01 | 6.69 | 0.00 | 6.64 | 0.003 | 0.087 | `OK` |
| `t1_std_mip` | 3 | 3 | **838.1x** [760.2x-969.4x] | 0.01 | 8.07 | 0.01 | 8.01 | 0.003 | 0.090 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **855.0x** [686.5x-999.5x] | 0.01 | 7.94 | 0.01 | 7.87 | 0.003 | 0.090 | `OK` |
| `t1_whiskas` | 6 | 5 | **569.2x** [401.9x-694.5x] | 0.01 | 7.10 | 0.00 | 7.00 | 0.007 | 0.122 | `OK` |
| `t1_furniture` | 2 | 3 | **285.8x** [67.5x-773.1x] | 0.02 | 8.69 | 0.02 | 8.62 | 0.003 | 0.092 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **105.3x** [43.2x-260.7x] | 0.10 | 10.17 | 0.09 | 10.06 | 0.010 | 0.135 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **1256.5x** [392.0x-2026.6x] | 0.00 | 6.64 | 0.00 | 6.60 | 0.003 | 0.070 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **120.0x** [57.9x-147.2x] | 0.08 | 10.26 | 0.05 | 10.03 | 0.024 | 0.296 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **24.8x** [10.2x-76.3x] | 1.71 | 34.24 | 1.67 | 34.00 | 0.030 | 0.305 | `OK` |
| `t2_mkp_30` | 30 | 6 | **4.2x** [2.4x-7.1x] | 4.82 | 28.64 | 4.80 | 28.46 | 0.030 | 0.239 | `OK` |
| `t2_mkp_50` | 50 | 8 | **2.3x** [1.0x-24.4x] | 12.51 | 17.78 | 12.46 | 17.52 | 0.058 | 0.354 | `OK` |
| `t2_cflp_36` | 36 | 11 | **23.0x** [2.2x-152.4x] | 0.32 | 10.04 | 0.30 | 9.85 | 0.024 | 0.236 | `OK` |
| `t2_cflp_49` | 49 | 13 | **14.5x** [5.5x-31.7x] | 0.70 | 11.00 | 0.67 | 10.77 | 0.028 | 0.285 | `OK` |
| `t2_gap_45` | 45 | 13 | **7.5x** [1.2x-21.6x] | 1.87 | 15.24 | 1.82 | 15.02 | 0.035 | 0.273 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **6.3x** [3.0x-13.6x] | 9.15 | 69.09 | 9.08 | 68.85 | 0.046 | 0.317 | `OK` |
| `t2_binpack_44` | 44 | 14 | **7.7x** [0.1x-33.1x] | 0.78 | 14.52 | 0.76 | 14.31 | 0.030 | 0.270 | `OK` |
| `t2_setcover_50` | 50 | 20 | **39.7x** [2.5x-107.8x] | 0.21 | 11.75 | 0.16 | 11.44 | 0.047 | 0.412 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **50.6x** [35.5x-62.2x] | 0.18 | 10.25 | 0.15 | 9.93 | 0.035 | 0.391 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **45.5x** [32.5x-57.7x] | 0.21 | 10.74 | 0.16 | 10.29 | 0.056 | 0.552 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **25.0x** [20.5x-30.3x] | 0.48 | 12.20 | 0.38 | 11.58 | 0.091 | 0.812 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **5.2x** [2.2x-19.0x] | 7.22 | 48.64 | 7.16 | 48.32 | 0.058 | 0.402 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **3.1x** [0.5x-12.6x] | 44.85 | 167.37 | 44.77 | 166.94 | 0.082 | 0.569 | `OK` |
| `t3_mkp_200` | 200 | 8 | **1.0x** [1.0x-1.1x] | 104.91 | 115.78 | 104.80 | 115.17 | 0.123 | 0.806 | `OK` |
| `t3_cflp_300` | 300 | 34 | **8.7x** [5.7x-13.0x] | 24.25 | 249.05 | 23.98 | 248.13 | 0.141 | 1.188 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **0.4x** [0.0x-1.1x] | 253.45 | 157.90 | 253.20 | 156.06 | 0.268 | 2.307 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **3.5x** [2.3x-4.5x] | 22.93 | 81.45 | 22.55 | 78.54 | 0.383 | 3.519 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **22.2x** [14.5x-28.7x] | 0.78 | 19.20 | 0.60 | 18.02 | 0.196 | 1.382 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **11.8x** [8.9x-20.0x] | 1.76 | 19.26 | 1.52 | 17.50 | 0.282 | 2.305 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.6x** [5.0x-6.5x] | 11.17 | 62.28 | 10.07 | 54.60 | 1.221 | 10.838 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **6.1x** [3.2x-7.9x] | 49.82 | 374.33 | 43.67 | 339.00 | 4.601 | 45.480 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## 5. Default Build vs. `GOEXPERIMENT=simd`

In [`internal/simd/kernels_amd64.go`](../internal/simd/kernels_amd64.go), AVX2 kernels (gated at runtime on CPU `AVX2` and `FMA` support) spill 4-lane accumulators to the stack via `StoreArray` and execute `archsimd.ClearAVXUpperBits()` (`VZEROUPPER`) before scalar tail or caller operations, eliminating AVX-to-SSE dirty-upper `YMM` transition stalls while preserving `NaN` propagation parity. From the kernel dispatch thresholds upward (`minAVX2Len = 32` for `Dot64`, `Axpy64`, `Scale64`, `AbsMax64`, `RowActivityBoundsFinite`, `MinReducedSlice64`, and `InitReducedRow64`; $N \ge 256$ for `Min2ReducedRow64`; and caller-gated at $N \ge 256$ in `assign/solver.go` for `AllAbsLE64`), the 256-bit AVX2 loops outperform the 4-way slice-stepped scalar loops; below those lengths, call and `VZEROUPPER` overhead dominates, so shorter slices stay on the scalar path.

At the full-solver level, each ratio below is the geometric mean of per-trial `scalar / SIMD` solve times over the same instances, with a 95% $t$-interval on the log ratios; "no detectable effect" means the interval contains 1. "Reachable" is a necessary condition: the AVX2 `Dot64`/`Axpy64`/`Scale64` paths require a slice of length 32 or more (`n + m >= 32`). Below that size no AVX2 kernel can run, so those 8 families serve as negative controls where intervals excluding 1 reflect binary code-layout or run-to-run variance rather than SIMD execution. Lowering `minAVX2Len` from 256 to 32 enables AVX2 across dense tableau row accumulation (`ComputeTableauRow`) for medium models with $n \le 256$ (`DenseThreshold`) and dual multiplier/reduced-cost updates inside `runDualSimplexLoop`, while `ComputeTableauRow` dispatches `Axpy64` on contiguous CSR column blocks in large models ($n > 256$, such as `t3_mad_g4_i1000`).

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 0 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 0 faster, 2 slower, 21 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 0.86x [0.57x-1.32x] | 0.003 | 0.003 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 0.98x [0.89x-1.10x] | 0.002 | 0.002 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 0.96x [0.83x-1.12x] | 0.005 | 0.006 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source | 1.00x [0.90x-1.11x] | 0.005 | 0.005 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 0.92x [0.79x-1.08x] | 0.005 | 0.006 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 0.93x [0.77x-1.13x] | 0.016 | 0.020 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source | 0.98x [0.82x-1.17x] | 0.094 | 0.105 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 0.93x [0.81x-1.07x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 0.96x [0.87x-1.06x] | 0.054 | 0.058 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 0.99x [0.94x-1.05x] | 1.674 | 1.709 | 11 |
| `t2_mkp_30` | 36 | possible | SIMD slower | 0.98x [0.97x-1.00x] | 4.796 | 4.827 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 1.01x [0.99x-1.03x] | 12.458 | 12.869 | 11 |
| `t2_cflp_36` | 47 | possible | no detectable effect | 1.01x [0.92x-1.11x] | 0.297 | 0.284 | 11 |
| `t2_cflp_49` | 62 | possible | no detectable effect | 1.00x [0.96x-1.03x] | 0.672 | 0.667 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 1.01x [0.95x-1.08x] | 1.815 | 1.525 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 0.99x [0.95x-1.03x] | 9.078 | 9.050 | 11 |
| `t2_binpack_44` | 58 | possible | no detectable effect | 1.05x [0.96x-1.15x] | 0.757 | 0.757 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 1.05x [0.97x-1.13x] | 0.159 | 0.164 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 0.89x [0.74x-1.09x] | 0.148 | 0.167 | 11 |
| `t2_mad_g3_i9` | 108 | possible | SIMD slower | 0.95x [0.90x-0.99x] | 0.156 | 0.170 | 11 |
| `t2_mad_g4_i12` | 156 | possible | no detectable effect | 0.98x [0.92x-1.03x] | 0.381 | 0.386 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.02x [0.99x-1.05x] | 7.164 | 7.123 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 0.99x [0.97x-1.01x] | 44.766 | 44.438 | 11 |
| `t3_mkp_200` | 208 | possible | no detectable effect | 1.00x [0.99x-1.02x] | 104.795 | 105.349 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 0.99x [0.97x-1.01x] | 23.981 | 23.906 | 5 |
| `t3_lotsizing_504` | 840 | possible | no detectable effect | 0.99x [0.98x-1.01x] | 253.200 | 255.039 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 0.99x [0.95x-1.04x] | 22.554 | 21.593 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 1.04x [0.93x-1.15x] | 0.603 | 0.574 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 1.05x [0.89x-1.23x] | 1.515 | 1.450 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 1.03x [0.90x-1.17x] | 10.065 | 9.939 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | no detectable effect | 1.01x [0.96x-1.07x] | 43.667 | 41.484 | 5 |
<!-- END GENERATED:SIMD_EFFECT_TABLE -->

---

## 6. Full Corpus Solver-Core Checkpoint Progression

<!-- BEGIN GENERATED:FULL_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_std_lp` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 54.00 | 54.00 |
| `t1_std_lp_max` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 80.00 | 80.00 |
| `t1_std_mip` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.002 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.002 | 64.95 | 58.90 |
| `t1_whiskas` | **0.00** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.00 (1) | 0.004 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.02 (1) | 0.02 (1) | 0.003 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.09** (1 / 8) | 0.15 (2 / 8) | 0.25 (24) | 0.35 (31) | 0.009 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.002 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.05** (1 / 0) | 0.06 (1 / 0) | 0.07 (1) | 1.01 (4) | 0.083 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **1.67** (10 / 0) | 1.10 (12 / 0) | 1.04 (12) | 2.22 (84) | 0.101 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **4.80** (27 / 9) | 5.15 (48 / 9) | 2.16 (77) | 2.10 (77) | 0.033 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **12.46** (1 / 4) | 28.71 (2 / 4) | 12.94 (132) | 10.41 (119) | 0.047 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.30** (1 / 2) | 1.81 (3 / 2) | 0.28 (7) | 0.24 (7) | 0.026 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.67** (1 / 20) | 2.13 (4 / 20) | 0.60 (9) | 0.55 (9) | 0.031 | 909.02 | 827.30 |
| `t2_gap_45` | **1.82** (1 / 9) | 0.46 (1 / 9) | 2.94 (61) | 2.74 (61) | 0.031 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **9.08** (5 / 37) | 5.42 (8 / 37) | 4.86 (198) | 5.49 (174) | 0.060 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **0.76** (2 / 7) | 1.69 (6 / 7) | 39.14 (1848) | 50.77 (2401) | 0.032 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.16** (1 / 6) | 0.53 (2 / 6) | 1.70 (9) | 1.43 (9) | 0.039 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.15** (1 / 0) | 1.18 (9 / 0) | 1.52 (9) | 1.25 (9) | 0.103 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.16** (1 / 0) | 0.21 (1 / 0) | 0.21 (1) | 4.14 (5) | 0.272 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.38** (1 / 0) | 7.52 (25 / 0) | 7.48 (25) | 7.20 (19) | 0.350 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **7.16** (65 / 0) | 4.02 (66 / 0) | 3.67 (66) | 9.19 (333) | 0.160 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **44.77** (254 / 0) | 26.56 (209 / 0) | 26.54 (209) | 250.03 (1664, `TimeLimit`) | 0.303 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **104.80** (590 / 12) | 131.15 (701 / 12) | 250.05 (3500, `TimeLimit`) | 250.04 (4686, `TimeLimit`) | 0.136 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **23.98** (38 / 31) | 22.81 (38 / 31) | 15.96 (134) | 14.00 (134) | 0.196 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **253.20** (35 / 205) | 243.23 (420 / 205) | 250.03 (1329, `TimeLimit`) | 250.05 (1520, `TimeLimit`) | 0.875 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **22.55** (64 / 46) | 75.06 (206 / 46) | 250.05 (1066, `TimeLimit`) | 250.02 (1523, `TimeLimit`) | 0.252 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.60** (1 / 0) | 4.48 (12 / 0) | 5.20 (12) | 3.24 (3) | 0.977 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **1.52** (1 / 0) | 10.99 (12 / 0) | 10.31 (12) | 39.85 (81) | 1.595 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **10.07** (2 / 0) | 250.43 (245 / 0, `TimeLimit`) | 250.11 (248, `TimeLimit`) | 156.41 (105) | 13.926 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **43.67** (1 / 0) | 68.87 (2 / 0) | 67.16 (2) | 209.90 (9) | 114.463 | 208000.00 | 208000.00 |
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
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.56x** [1.34x-1.82x] | 1.56x | 4.47x | 0.203 (0.0165) | 0.315 (0.0290) | 1.60x [0.38x-3.52x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **3.32x** [2.10x-5.25x] | 3.32x | 3.36x | 8.932 (0.782) | 35.399 (3.147) | 3.04x [1.04x-15.57x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **3.04x** [2.10x-4.42x] | 3.04x | 3.94x | 246.561 (48.219) | 962.516 (190.164) | 2.95x [1.49x-6.06x] |
| Square families ($m = n$) | 16 (146) | **2.91x** [2.14x-3.96x] | 2.86x | 4.08x | 251.072 (48.190) | 986.028 (191.101) | 2.63x [0.38x-15.57x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **2.00x** [1.40x-2.87x] | 1.86x | 3.20x | 4.623 (0.827) | 12.201 (2.239) | 1.99x [0.77x-3.38x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.54x** [1.29x-5.00x] | 2.69x | 3.14x | 34.740 (5.998) | 66.837 (11.198) | 2.07x [0.56x-15.57x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.63x** [2.07x-3.34x] | 2.55x | 3.82x | 255.695 (49.017) | 998.229 (193.340) | 2.36x [0.38x-15.57x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 7.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.48x** | 0.0006 | 0.0011 | 0.0007 | 0.0137 | $0.0009 \pm 0.0008$ | $0.0011 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.61x** | 0.0015 | 0.0024 | 0.0017 | 0.0101 | $0.0015 \pm 0.0003$ | $0.0023 \pm 0.0004$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.79x** | 0.0028 | 0.0062 | 0.0056 | 0.0153 | $0.0037 \pm 0.0023$ | $0.0061 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.30x** | 0.0029 | 0.0040 | 0.0047 | 0.0142 | $0.0031 \pm 0.0010$ | $0.0039 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.39x** | 0.0032 | 0.0044 | 0.0048 | 0.0151 | $0.0033 \pm 0.0011$ | $0.0045 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.88x** | 0.0055 | 0.0110 | 0.0086 | 0.0327 | $0.0060 \pm 0.0022$ | $0.0108 \pm 0.0018$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **3.01x** | 0.0265 | 0.0849 | 0.0335 | 0.116 | $0.0300 \pm 0.0142$ | $0.0847 \pm 0.0135$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.82x** | 0.0526 | 0.191 | 0.0736 | 0.253 | $0.0514 \pm 0.0044$ | $0.198 \pm 0.0356$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.50x** | 0.0453 | 0.111 | 0.0706 | 0.171 | $0.0440 \pm 0.0090$ | $0.109 \pm 0.0177$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.80x** | 0.133 | 0.239 | 0.150 | 0.280 | $0.143 \pm 0.0413$ | $0.255 \pm 0.0563$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **13.03x** | 0.0565 | 0.730 | 0.0739 | 0.825 | $0.0578 \pm 0.0063$ | $0.752 \pm 0.0694$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **2.36x** | 0.0316 | 0.0706 | 0.0547 | 0.144 | $0.0305 \pm 0.0036$ | $0.0719 \pm 0.0081$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **1.94x** | 0.0382 | 0.0791 | 0.0645 | 0.153 | $0.0423 \pm 0.0108$ | $0.0802 \pm 0.0073$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.94x** | 0.0813 | 0.313 | 0.108 | 0.416 | $0.0830 \pm 0.0081$ | $0.330 \pm 0.0580$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **4.07x** | 0.316 | 1.328 | 0.418 | 1.870 | $0.330 \pm 0.0498$ | $1.337 \pm 0.171$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.43x** | 1.212 | 5.645 | 1.616 | 8.483 | $1.284 \pm 0.144$ | $5.701 \pm 0.766$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **2.06x** | 0.788 | 1.667 | 1.352 | 4.484 | $0.791 \pm 0.0554$ | $1.625 \pm 0.104$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.69x** | 4.966 | 8.434 | 5.584 | 9.611 | $5.596 \pm 2.063$ | $9.250 \pm 2.532$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **3.01x** | 0.314 | 0.937 | 0.689 | 3.879 | $0.309 \pm 0.0227$ | $0.930 \pm 0.0795$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.62x** | 0.437 | 1.143 | 0.897 | 4.204 | $0.441 \pm 0.0165$ | $1.157 \pm 0.0633$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.82x** | 5.935 | 28.779 | 8.189 | 39.582 | $6.218 \pm 0.519$ | $30.045 \pm 3.134$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.14x** | 34.566 | 143.559 | 41.442 | 193.083 | $34.672 \pm 1.851$ | $143.795 \pm 9.652$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 7.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSlice64`, `InitReducedRow64`, and `Min2ReducedRow64`)

In [`assign/solver.go`](../assign/solver.go), [`simd.AllAbsLE64With`](../internal/simd/kernels_amd64.go) validates finite cost bounds across the input matrix via 8-lane AVX2 `Abs().LessEqual` checks when `len(cost) >= 256`, [`simd.MinReducedSlice64With`](../internal/simd/kernels_amd64.go) accelerates reduction-transfer slack scans in `preprocessSquareJV`, [`simd.Min2ReducedRow64With`](../internal/simd/kernels_amd64.go) accelerates the first/second reduced-minimum scans in `preprocessRectangularBV` and `augmentingRowReduction` via 8-way AVX2 `VSUBPD` and `VCMPPD`/`VPAND`/`VMOVMSKPD` bitmask filtering when $\max(m, n) \ge 256$, and [`simd.InitReducedRow64With`](../internal/simd/kernels_amd64.go) runs on a slice of length $\max(m, n)$ at the start of each Dijkstra augmentation when $\max(m, n) \ge 32$. All vector kernels clean upper `YMM` state via `StoreArray` + `ClearAVXUpperBits()` (`VZEROUPPER`) before scalar operations; exact per-family verdicts appear in the generated table below.

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` and `rows * cols < 256`, no AVX2 kernel runs): 0 of 2 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32` or `AllAbsLE64` at `rows * cols >= 256`): 5 faster, 3 slower, 12 with no detectable effect, out of 20.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source | 1.01x [0.64x-1.60x] | 0.0006 | 0.0006 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source | 0.91x [0.74x-1.13x] | 0.0015 | 0.0015 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | possible | no detectable effect | 0.94x [0.67x-1.31x] | 0.0028 | 0.0038 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | no detectable effect | 0.84x [0.68x-1.04x] | 0.0029 | 0.0036 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | no detectable effect | 0.95x [0.71x-1.28x] | 0.0032 | 0.0034 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | no detectable effect | 0.93x [0.69x-1.26x] | 0.0055 | 0.0064 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | no detectable effect | 1.01x [0.77x-1.33x] | 0.0265 | 0.0285 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD slower | 0.91x [0.86x-0.96x] | 0.0526 | 0.0574 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD slower | 0.84x [0.74x-0.95x] | 0.0453 | 0.0489 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | no detectable effect | 0.79x [0.52x-1.22x] | 0.133 | 0.138 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | no detectable effect | 1.05x [0.96x-1.15x] | 0.0565 | 0.0545 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | no detectable effect | 1.08x [0.91x-1.28x] | 0.0316 | 0.0267 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | no detectable effect | 1.04x [0.90x-1.22x] | 0.0382 | 0.0394 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD slower | 0.86x [0.80x-0.92x] | 0.0813 | 0.0964 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | no detectable effect | 1.03x [0.98x-1.08x] | 0.316 | 0.302 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.09x [1.07x-1.11x] | 1.212 | 1.111 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.24x [1.12x-1.37x] | 0.788 | 0.621 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.05x [1.00x-1.10x] | 4.966 | 4.889 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.84x [1.55x-2.17x] | 0.314 | 0.170 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | no detectable effect | 1.08x [0.96x-1.22x] | 0.437 | 0.400 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.16x [1.10x-1.22x] | 5.935 | 5.513 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | no detectable effect | 1.03x [0.96x-1.10x] | 34.566 | 32.946 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

---

## 8. Keeping Documentation and Benchmarks Synchronized

1. **Regenerate telemetry** from a clean, committed tree in a single pinned session so that all seven telemetry files record identical `git_commit`, `source_fingerprint`, and `uname_release`: run the commands in the [README](../README.md#verification-benchmarking-and-documentation-sync-commands).
2. **Regenerate tables**: `go run ./cmd/goblin-bench -sync-docs`.
3. **Verify**: `go run ./cmd/goblin-bench -check-docs`, or `go test ./...`, which runs `TestDocumentationAndBenchmarkSync`. The gate validates corpus dimensions, parity, symbol identifiers, and provenance, and checks every generated block in `README.md`, `docs/benchmarks.md`, and `bench/results/final_report.md`.
