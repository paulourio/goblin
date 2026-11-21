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
- **Unified telemetry source**: fingerprint `5b410dffd8dc05c7` (commit `2b7305d20ada300b230498a89f1e7f64aa619fc4`), `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, or kernel differs.
- **CPU affinity**: goblin `6`; Python + CBC `6`; Python + SciPy `6`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 7.21 ms median on a 1-variable MILP and 6.70 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
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
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **419.70x** [184.06x-957.03x] | 419.70x | 0.04x | 9.8 (0.8) | 193.5 (19.0) | 1077.7 (100.8) | 649.99x [31.36x-2522.44x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **18.65x** [11.03x-31.52x] | 18.65x | 0.16x | 755.9 (33.8) | 2010.2 (173.7) | 4323.0 (419.2) | 24.85x [0.15x-161.18x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **8.79x** [3.58x-21.54x] | 8.79x | 0.23x | 3436.6 (164.7) | 2968.2 (577.6) | 6186.9 (1063.7) | 10.85x [0.09x-36.20x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **10.14x** [5.80x-17.73x] | 11.69x | 0.25x | 3936.3 (155.7) | 3204.4 (407.0) | 6774.1 (986.1) | 12.01x [0.09x-161.18x] |
| Tier 3 non-MAD MILPs | 4 (20) | **4.69x** [0.75x-29.44x] | 4.69x | 0.45x | 3188.0 (122.7) | 1283.5 (240.8) | 2805.5 (598.8) | 4.61x [0.09x-25.05x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **31.84x** [15.59x-65.06x] | 40.77x | 0.09x | 257.2 (42.9) | 1798.0 (346.6) | 3839.7 (506.3) | 44.28x [4.26x-146.82x] |
| Random-weight MAD (negative controls) | 3 (33) | **15.89x** [1.01x-249.75x] | 15.89x | 0.15x | 361.6 (17.8) | 1243.8 (117.6) | 2245.8 (247.8) | 13.39x [1.40x-111.50x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **46.14x** [18.00x-118.29x] | 58.64x | 0.12x | 3945.1 (156.4) | 3373.9 (423.7) | 7747.9 (1077.4) | 41.42x [0.09x-2522.44x] |
| All 31 synthetic corpus families | 31 (293) | **41.93x** [20.76x-84.69x] | 54.17x | 0.11x | 4202.3 (199.3) | 5172.0 (770.3) | 11587.6 (1583.7) | 43.11x [0.09x-2522.44x] |
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
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **109.5x** [66.9x-146.8x] | 0.08 | 9.50 | 0.04 | 9.29 | 0.032 | 0.285 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **55.4x** [31.4x-111.5x] | 0.58 | 33.73 | 0.55 | 33.50 | 0.027 | 0.307 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **68.5x** [62.7x-79.6x] | 0.14 | 9.72 | 0.11 | 9.35 | 0.035 | 0.384 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **58.4x** [43.5x-79.7x] | 0.19 | 10.15 | 0.10 | 9.74 | 0.065 | 0.542 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **32.8x** [27.9x-39.5x] | 0.39 | 11.95 | 0.26 | 11.33 | 0.098 | 0.805 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **10.8x** [3.9x-33.4x] | 3.12 | 47.21 | 3.07 | 46.84 | 0.040 | 0.396 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **6.7x** [1.4x-26.0x] | 14.09 | 166.84 | 13.99 | 166.41 | 0.067 | 0.573 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **31.6x** [22.3x-36.2x] | 0.54 | 18.02 | 0.37 | 16.93 | 0.156 | 1.384 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **21.0x** [18.7x-24.5x] | 0.88 | 18.20 | 0.65 | 16.48 | 0.235 | 2.260 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **11.1x** [9.3x-12.7x] | 5.29 | 60.48 | 3.65 | 52.55 | 1.431 | 10.796 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **10.0x** [4.3x-26.0x] | 35.34 | 368.23 | 29.18 | 334.19 | 4.823 | 43.997 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Cross-Instance Latency Dispersion Across Randomized Instances (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.09 \pm 0.02$ ($\pm 0.02$) | $9.44 \pm 0.49$ ($\pm 0.33$) | $0.08 \pm 0.01$ ($\pm 0.01$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.64 \pm 0.33$ ($\pm 0.22$) | $30.72 \pm 9.55$ ($\pm 6.42$) | $0.65 \pm 0.34$ ($\pm 0.23$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.14 \pm 0.01$ ($\pm 0.01$) | $9.81 \pm 0.62$ ($\pm 0.42$) | $0.15 \pm 0.02$ ($\pm 0.02$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.18 \pm 0.04$ ($\pm 0.03$) | $10.39 \pm 0.59$ ($\pm 0.39$) | $0.18 \pm 0.04$ ($\pm 0.02$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.37 \pm 0.04$ ($\pm 0.03$) | $12.02 \pm 0.77$ ($\pm 0.52$) | $0.37 \pm 0.04$ ($\pm 0.02$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $4.46 \pm 4.07$ ($\pm 2.74$) | $35.37 \pm 16.90$ ($\pm 11.35$) | $4.42 \pm 3.99$ ($\pm 2.68$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $27.77 \pm 29.49$ ($\pm 19.81$) | $138.08 \pm 63.42$ ($\pm 42.60$) | $27.55 \pm 29.00$ ($\pm 19.48$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.55 \pm 0.04$ ($\pm 0.05$) | $17.53 \pm 2.29$ ($\pm 2.85$) | $0.51 \pm 0.02$ ($\pm 0.02$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $0.88 \pm 0.11$ ($\pm 0.14$) | $18.30 \pm 0.35$ ($\pm 0.44$) | $0.91 \pm 0.10$ ($\pm 0.12$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $5.67 \pm 0.76$ ($\pm 0.94$) | $62.69 \pm 3.62$ ($\pm 4.49$) | $5.24 \pm 1.02$ ($\pm 1.26$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $42.62 \pm 13.13$ ($\pm 16.30$) | $577.78 \pm 610.57$ ($\pm 758.00$) | $38.73 \pm 14.09$ ($\pm 17.49$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | **0.04** (1 / 0) | 0.04 (1 / 0) | 0.04 (1) | 0.45 (4) | 0.064 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.55** (5 / 0) | 0.52 (12 / 0) | 0.50 (12) | 0.94 (34) | 0.074 | 4601.00 | 4257.00 |
| `t2_mad_g2_i8` | **0.11** (1 / 0) | 0.63 (9 / 0) | 0.55 (9) | 0.51 (10) | 0.100 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.10** (1 / 0) | 0.09 (1 / 0) | 0.11 (1) | 2.41 (23) | 0.200 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.26** (1 / 0) | 1.54 (26 / 0) | 1.41 (26) | 3.48 (14) | 0.270 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.07** (39 / 0) | 2.51 (325 / 0) | 2.43 (325) | 6.80 (111) | 0.114 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **13.99** (220 / 0) | 30.73 (191 / 0) | 29.94 (191) | 236.43 (2029) | 0.231 | 7720.00 | 6060.00 |
| `t3_mad_g5_i20` | **0.37** (1 / 0) | 1.57 (10 / 0) | 1.52 (10) | 6.14 (16) | 0.728 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.65** (1 / 0) | 0.53 (1 / 0) | 0.53 (1) | 2.86 (9) | 1.418 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **3.65** (1 / 0) | 9.78 (4 / 0) | 8.58 (4) | 250.10 (313, `TimeLimit`) | 2.426 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **29.18** (1 / 0) | 34.49 (2 / 0) | 32.61 (2) | 256.76 (52, `TimeLimit`) | 105.384 | 208000.00 | 208000.00 |
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
| `t1_std_lp` | 3 | 3 | **827.7x** [471.9x-1192.1x] | 0.01 | 6.69 | 0.00 | 6.63 | 0.005 | 0.087 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **1161.1x** [937.5x-1302.1x] | 0.01 | 6.26 | 0.00 | 6.20 | 0.003 | 0.079 | `OK` |
| `t1_std_mip` | 3 | 3 | **653.1x** [293.5x-830.5x] | 0.01 | 7.35 | 0.00 | 7.30 | 0.004 | 0.079 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **638.6x** [273.8x-776.3x] | 0.01 | 7.41 | 0.01 | 7.35 | 0.004 | 0.090 | `OK` |
| `t1_whiskas` | 6 | 5 | **635.2x** [504.9x-710.4x] | 0.01 | 6.33 | 0.00 | 6.23 | 0.006 | 0.099 | `OK` |
| `t1_furniture` | 2 | 3 | **371.9x** [85.5x-1201.4x] | 0.02 | 8.07 | 0.01 | 8.00 | 0.004 | 0.087 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **135.4x** [54.4x-385.2x] | 0.08 | 9.18 | 0.07 | 9.08 | 0.008 | 0.131 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **2179.5x** [1481.9x-2522.4x] | 0.00 | 6.29 | 0.00 | 6.23 | 0.002 | 0.063 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **109.5x** [66.9x-146.8x] | 0.08 | 9.50 | 0.04 | 9.29 | 0.032 | 0.285 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **55.4x** [31.4x-111.5x] | 0.58 | 33.73 | 0.55 | 33.50 | 0.027 | 0.307 | `OK` |
| `t2_mkp_30` | 30 | 6 | **7.7x** [4.4x-13.8x] | 2.53 | 28.01 | 2.50 | 27.82 | 0.021 | 0.235 | `OK` |
| `t2_mkp_50` | 50 | 8 | **4.3x** [1.8x-32.4x] | 6.45 | 17.40 | 6.41 | 17.14 | 0.041 | 0.346 | `OK` |
| `t2_cflp_36` | 36 | 11 | **32.9x** [4.0x-161.2x] | 0.24 | 9.38 | 0.22 | 9.21 | 0.019 | 0.221 | `OK` |
| `t2_cflp_49` | 49 | 13 | **20.1x** [9.6x-34.1x] | 0.51 | 10.15 | 0.46 | 9.96 | 0.026 | 0.276 | `OK` |
| `t2_gap_45` | 45 | 13 | **20.9x** [4.1x-62.1x] | 0.72 | 14.60 | 0.70 | 14.37 | 0.025 | 0.269 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **11.8x** [5.0x-28.6x] | 4.70 | 68.15 | 4.67 | 67.91 | 0.030 | 0.310 | `OK` |
| `t2_binpack_44` | 44 | 14 | **13.8x** [0.2x-50.9x] | 0.51 | 14.19 | 0.49 | 13.98 | 0.024 | 0.267 | `OK` |
| `t2_setcover_50` | 50 | 20 | **46.3x** [4.5x-123.8x] | 0.17 | 11.39 | 0.13 | 11.07 | 0.042 | 0.417 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **68.5x** [62.7x-79.6x] | 0.14 | 9.72 | 0.11 | 9.35 | 0.035 | 0.384 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **58.4x** [43.5x-79.7x] | 0.19 | 10.15 | 0.10 | 9.74 | 0.065 | 0.542 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **32.8x** [27.9x-39.5x] | 0.39 | 11.95 | 0.26 | 11.33 | 0.098 | 0.805 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **10.8x** [3.9x-33.4x] | 3.12 | 47.21 | 3.07 | 46.84 | 0.040 | 0.396 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **6.7x** [1.4x-26.0x] | 14.09 | 166.84 | 13.99 | 166.41 | 0.067 | 0.573 | `OK` |
| `t3_mkp_200` | 200 | 8 | **2.5x** [1.7x-4.3x] | 38.24 | 114.62 | 38.13 | 114.02 | 0.096 | 0.808 | `OK` |
| `t3_cflp_300` | 300 | 34 | **17.7x** [10.2x-25.1x] | 10.50 | 247.14 | 10.39 | 246.23 | 0.121 | 1.185 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **1.3x** [0.1x-3.5x] | 64.78 | 156.21 | 64.54 | 154.38 | 0.248 | 2.287 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **8.1x** [4.9x-10.9x] | 9.14 | 80.79 | 8.81 | 78.06 | 0.342 | 3.484 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **31.6x** [22.3x-36.2x] | 0.54 | 18.02 | 0.37 | 16.93 | 0.156 | 1.384 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **21.0x** [18.7x-24.5x] | 0.88 | 18.20 | 0.65 | 16.48 | 0.235 | 2.260 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **11.1x** [9.3x-12.7x] | 5.29 | 60.48 | 3.65 | 52.55 | 1.431 | 10.796 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **10.0x** [4.3x-26.0x] | 35.34 | 368.23 | 29.18 | 334.19 | 4.823 | 43.997 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## 5. Default Build vs. `GOEXPERIMENT=simd`

In [`internal/simd/kernels_amd64.go`](../internal/simd/kernels_amd64.go) and [`internal/simd/kernels_avx512_amd64.go`](../internal/simd/kernels_avx512_amd64.go), runtime CPUID dispatch selects AVX-512, AVX2, or 4-way scalar loops from a single portable `CGO_ENABLED=0` binary (`GOAMD64=v3` FMA-safe). Each vector kernel spills accumulators via `StoreArray` and executes `archsimd.ClearAVXUpperBits()` (`VZEROUPPER`) before scalar tail or caller operations, eliminating AVX-to-SSE transition stalls while preserving IEEE-754 `NaN` propagation parity. AVX2 activates at `minAVX2Len = 32` (`Dot64`, `Axpy64`, `Scale64`, `AbsMax64`, `RowActivityBoundsFinite`, `SelectDualLeaveRow64With`, `MinReducedSlice64With`, `InitReducedRow64`), `minDualRatioSIMDChunks = 4` (256 cols for `ScanDualRatioCand64With`), and $N \ge 256$ (`Min2ReducedRow64`, `AllAbsLE64`); on AVX-512 hosts, dense LSAP loops dispatch AVX-512 at `minAVX512Len = 64` while sparse LP kernels gate AVX-512 at `minAVX512LPLen = 2048` / `minLeaveRowAVX512 = 2048` / `minDualRatioAVX512Chunks = 128` (8,192 cols) to avoid Intel Emerald Rapids License-2 clock down-binning across surrounding scalar LU code.

At the full-solver level, each ratio below is the geometric mean of per-trial `scalar / SIMD` solve times over the same instances on the local AVX2 host (`i9-13900K`), with a 95% $t$-interval on the log ratios; "no detectable effect" means the interval contains 1. "Reachable" is a necessary condition: the AVX2 LP kernels require `n + m >= 32`. Below that size no vector kernel can run, so those 8 families serve as negative controls where intervals excluding 1 reflect binary code-layout or run-to-run variance rather than SIMD execution. On larger models (`ComputeTableauRow` contiguous CSR blocks, `SelectDualLeaveRow64With`, and `ScanDualRatioCand64With`), `t3_mkp_200` and `t3_lotsizing_504` achieve statistically significant AVX2 speedups with 0 regressions across all 23 reachable families, while `t3_mad_g4_i1000` records a 1.12x paired geomean whose 5-trial 95% interval spans 0.98x-1.27x.

### 5.1 Cloud Target-Hardware Verification (GCP `n4` Emerald Rapids & `e2` Broadwell)

Separate single-binary (`CGO_ENABLED=0`, `GOAMD64=v3`, `GOEXPERIMENT=simd`) verification runs on ephemeral GCP `n4-standard-4` (`Intel(R) Xeon(R) Platinum 8581C`, Emerald Rapids AVX-512) and `e2-standard-4` (`Intel(R) Xeon(R) CPU @ 2.20GHz`, Broadwell AVX2) instances in `us-central1-a` verified 22/22 `internal/simd` bit-parity tests (including `SelectDualLeaveRow64` and `ScanDualRatioCand64` on real AVX-512 silicon), `goblin/assign` tests, full `goblin` + Netlib/MIPLIB parity tests, and 0 end-to-end objective mismatches:
- **LSAP paired SIMD vs. scalar (`-trials=7`)**: `n4` AVX-512 achieved 1.380x geomean across all 22 families and 2.086x on Tier 3 (vs. 1.290x / 1.826x under AVX2); `e2` AVX2 achieved 1.379x across all 22 families and 2.104x on Tier 3.
- **Wave 3 MAD/MILP gain vs. pre-MAD Run 2 (`-trials=5`, identical silicon)**: `n4` achieved 1.522x geomean across 11 MAD families (1.334x across all 31 MILPs; `t3_mad_g10_i100` 3.335x solve / 4.896x Root LP); `e2` achieved 1.557x across 11 MAD families (1.283x across all 31 MILPs; `t3_mad_g10_i100` 2.906x solve / 4.746x Root LP, `t3_mad_g4_i1000` 1.322x).
- **Pending GCP re-measurement**: Raising `minDualRatioAVX512Chunks` from 32 to 128 after Run 3 (to eliminate the 9% Emerald Rapids License-2 clock down-binning observed on `t3_mad_g4_i1000` Solve under AVX-512 vs. AVX2's 1.08x to 1.12x gain on 4 of 5 trials) is locally verified and pending a fresh `n4` measurement.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 0 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 2 faster, 0 slower, 21 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 0.96x [0.78x-1.18x] | 0.002 | 0.003 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 0.99x [0.80x-1.22x] | 0.002 | 0.002 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 0.88x [0.71x-1.10x] | 0.005 | 0.005 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source | 0.76x [0.48x-1.21x] | 0.005 | 0.005 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 1.01x [0.89x-1.13x] | 0.004 | 0.004 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 0.94x [0.81x-1.09x] | 0.012 | 0.011 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source | 1.05x [0.91x-1.21x] | 0.070 | 0.069 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 1.00x [0.92x-1.09x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 1.12x [0.98x-1.29x] | 0.040 | 0.038 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 0.99x [0.94x-1.04x] | 0.553 | 0.559 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 0.97x [0.93x-1.02x] | 2.505 | 3.179 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 1.00x [0.96x-1.03x] | 6.411 | 6.002 | 11 |
| `t2_cflp_36` | 47 | possible | no detectable effect | 1.04x [0.91x-1.18x] | 0.218 | 0.200 | 11 |
| `t2_cflp_49` | 62 | possible | no detectable effect | 0.98x [0.94x-1.02x] | 0.455 | 0.462 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 1.01x [0.96x-1.06x] | 0.696 | 0.716 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 1.00x [0.97x-1.02x] | 4.670 | 4.781 | 11 |
| `t2_binpack_44` | 58 | possible | no detectable effect | 1.00x [0.95x-1.05x] | 0.487 | 0.467 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 1.02x [0.93x-1.11x] | 0.126 | 0.130 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 1.02x [0.97x-1.08x] | 0.106 | 0.105 | 11 |
| `t2_mad_g3_i9` | 108 | possible | no detectable effect | 1.02x [0.91x-1.14x] | 0.102 | 0.101 | 11 |
| `t2_mad_g4_i12` | 156 | possible | no detectable effect | 1.00x [0.94x-1.07x] | 0.258 | 0.255 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 0.99x [0.96x-1.02x] | 3.071 | 3.101 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 0.99x [0.96x-1.02x] | 13.993 | 14.097 | 11 |
| `t3_mkp_200` | 208 | possible | SIMD faster | 1.05x [1.03x-1.08x] | 38.132 | 36.648 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.05x [1.00x-1.10x] | 10.391 | 10.487 | 5 |
| `t3_lotsizing_504` | 840 | possible | SIMD faster | 1.08x [1.02x-1.15x] | 64.541 | 63.446 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 0.99x [0.94x-1.03x] | 8.812 | 9.341 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 1.05x [0.99x-1.12x] | 0.372 | 0.359 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 1.01x [0.99x-1.02x] | 0.649 | 0.652 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 1.03x [0.96x-1.11x] | 3.646 | 3.363 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | no detectable effect | 1.12x [0.98x-1.27x] | 29.177 | 26.737 | 5 |
<!-- END GENERATED:SIMD_EFFECT_TABLE -->

---

## 6. Full Corpus Solver-Core Checkpoint Progression

<!-- BEGIN GENERATED:FULL_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_std_lp` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 54.00 | 54.00 |
| `t1_std_lp_max` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.001 | 80.00 | 80.00 |
| `t1_std_mip` | **0.00** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.002 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.00 (1) | 0.01 (3) | 0.002 | 64.95 | 58.90 |
| `t1_whiskas` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.003 | 0.52 | 0.52 |
| `t1_furniture` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.002 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.07** (1 / 8) | 0.09 (2 / 8) | 0.14 (24) | 0.22 (35) | 0.006 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.04** (1 / 0) | 0.04 (1 / 0) | 0.04 (1) | 0.45 (4) | 0.064 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.55** (5 / 0) | 0.52 (12 / 0) | 0.50 (12) | 0.94 (34) | 0.074 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.50** (27 / 9) | 2.51 (48 / 9) | 1.23 (77) | 1.09 (77) | 0.023 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **6.41** (1 / 4) | 14.59 (2 / 4) | 6.87 (132) | 6.64 (119) | 0.035 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.22** (1 / 2) | 0.97 (3 / 2) | 0.20 (7) | 0.20 (7) | 0.021 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.46** (1 / 20) | 1.52 (5 / 20) | 0.35 (9) | 0.34 (9) | 0.025 | 909.02 | 827.30 |
| `t2_gap_45` | **0.70** (1 / 9) | 0.36 (1 / 9) | 1.67 (61) | 1.76 (61) | 0.022 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **4.67** (4 / 37) | 5.16 (28 / 37) | 2.52 (187) | 3.07 (199) | 0.051 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **0.49** (2 / 7) | 0.86 (5 / 7) | 21.02 (1496) | 36.92 (2402) | 0.023 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.13** (1 / 6) | 0.31 (2 / 6) | 0.72 (9) | 0.75 (9) | 0.030 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.11** (1 / 0) | 0.63 (9 / 0) | 0.55 (9) | 0.51 (10) | 0.100 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.10** (1 / 0) | 0.09 (1 / 0) | 0.11 (1) | 2.41 (23) | 0.200 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.26** (1 / 0) | 1.54 (26 / 0) | 1.41 (26) | 3.48 (14) | 0.270 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.07** (39 / 0) | 2.51 (325 / 0) | 2.43 (325) | 6.80 (111) | 0.114 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **13.99** (220 / 0) | 30.73 (191 / 0) | 29.94 (191) | 236.43 (2029) | 0.231 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **38.13** (598 / 9) | 101.73 (1552 / 9) | 180.97 (5000, `NodeLimit`) | 166.58 (5000, `NodeLimit`) | 0.110 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **10.39** (53 / 20) | 12.25 (59 / 20) | 9.07 (134) | 9.41 (134) | 0.151 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **64.54** (30 / 205) | 106.51 (488 / 205) | 250.03 (3282, `TimeLimit`) | 250.02 (3523, `TimeLimit`) | 0.664 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **8.81** (64 / 46) | 35.78 (206 / 46) | 250.02 (2157, `TimeLimit`) | 250.04 (2202, `TimeLimit`) | 0.203 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.37** (1 / 0) | 1.57 (10 / 0) | 1.52 (10) | 6.14 (16) | 0.728 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.65** (1 / 0) | 0.53 (1 / 0) | 0.53 (1) | 2.86 (9) | 1.418 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **3.65** (1 / 0) | 9.78 (4 / 0) | 8.58 (4) | 250.10 (313, `TimeLimit`) | 2.426 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **29.18** (1 / 0) | 34.49 (2 / 0) | 32.61 (2) | 256.76 (52, `TimeLimit`) | 105.384 | 208000.00 | 208000.00 |
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
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.78x** [1.43x-2.23x] | 1.78x | 5.39x | 0.194 (0.0159) | 0.318 (0.0292) | 1.80x [0.59x-3.40x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **3.11x** [1.88x-5.16x] | 3.11x | 3.09x | 10.055 (0.780) | 33.057 (2.944) | 3.04x [0.19x-18.84x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **3.16x** [2.17x-4.62x] | 3.16x | 3.91x | 208.854 (41.183) | 942.443 (187.380) | 3.04x [1.59x-6.29x] |
| Square families ($m = n$) | 16 (146) | **2.91x** [2.13x-3.98x] | 2.85x | 4.07x | 215.120 (41.308) | 964.934 (188.379) | 2.63x [0.19x-18.84x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **2.17x** [1.55x-3.03x] | 2.02x | 3.40x | 3.983 (0.672) | 10.884 (1.974) | 2.23x [0.75x-3.54x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.37x** [1.14x-4.93x] | 2.46x | 3.03x | 33.859 (5.954) | 65.982 (11.445) | 1.87x [0.61x-18.84x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.69x** [2.12x-3.41x] | 2.60x | 3.88x | 219.103 (41.979) | 975.818 (190.353) | 2.43x [0.19x-18.84x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 7.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **2.37x** | 0.0004 | 0.0009 | 0.0005 | 0.0116 | $0.0004 \pm 0.0001$ | $0.0010 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.93x** | 0.0012 | 0.0024 | 0.0015 | 0.0103 | $0.0013 \pm 0.0007$ | $0.0024 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.37x** | 0.0037 | 0.0062 | 0.0050 | 0.0151 | $0.0047 \pm 0.0023$ | $0.0060 \pm 0.0015$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.43x** | 0.0028 | 0.0043 | 0.0042 | 0.0145 | $0.0029 \pm 0.0006$ | $0.0041 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.74x** | 0.0029 | 0.0044 | 0.0037 | 0.0153 | $0.0027 \pm 0.0006$ | $0.0047 \pm 0.0009$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **2.05x** | 0.0050 | 0.0111 | 0.0078 | 0.0329 | $0.0055 \pm 0.0021$ | $0.0107 \pm 0.0017$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **3.06x** | 0.0232 | 0.0713 | 0.0311 | 0.0973 | $0.0231 \pm 0.0056$ | $0.0695 \pm 0.0111$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **4.27x** | 0.0471 | 0.176 | 0.0633 | 0.224 | $0.0463 \pm 0.0045$ | $0.220 \pm 0.148$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **1.79x** | 0.0561 | 0.0941 | 0.0825 | 0.142 | $0.0553 \pm 0.0159$ | $0.0963 \pm 0.0144$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.61x** | 0.135 | 0.219 | 0.149 | 0.270 | $0.149 \pm 0.0515$ | $0.233 \pm 0.0431$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **13.86x** | 0.0512 | 0.674 | 0.0670 | 0.762 | $0.0512 \pm 0.0153$ | $0.687 \pm 0.0227$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **2.64x** | 0.0270 | 0.0688 | 0.0522 | 0.143 | $0.0268 \pm 0.0029$ | $0.0707 \pm 0.0081$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **1.79x** | 0.0380 | 0.0790 | 0.0605 | 0.153 | $0.0505 \pm 0.0294$ | $0.0803 \pm 0.0086$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **2.87x** | 0.0773 | 0.288 | 0.103 | 0.372 | $0.180 \pm 0.338$ | $0.285 \pm 0.0327$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.85x** | 0.325 | 1.273 | 0.421 | 1.668 | $0.332 \pm 0.0607$ | $1.263 \pm 0.122$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.69x** | 1.537 | 5.181 | 1.958 | 7.867 | $1.467 \pm 0.356$ | $5.294 \pm 0.397$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **2.11x** | 0.722 | 1.501 | 1.271 | 4.188 | $0.705 \pm 0.0490$ | $1.487 \pm 0.0476$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.75x** | 4.981 | 8.940 | 5.179 | 10.606 | $5.481 \pm 1.983$ | $9.437 \pm 2.730$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **3.11x** | 0.264 | 0.821 | 0.665 | 3.636 | $0.271 \pm 0.0371$ | $0.840 \pm 0.0557$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.88x** | 0.337 | 0.997 | 0.699 | 3.859 | $0.343 \pm 0.0139$ | $0.985 \pm 0.0332$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **5.09x** | 5.648 | 28.195 | 7.490 | 36.776 | $5.610 \pm 0.241$ | $28.621 \pm 2.264$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **5.11x** | 27.694 | 141.746 | 37.189 | 184.189 | $27.893 \pm 3.697$ | $141.824 \pm 11.451$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 7.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSlice64`, `InitReducedRow64`, and `Min2ReducedRow64`)

In [`assign/solver.go`](../assign/solver.go), [`simd.AllAbsLE64With`](../internal/simd/kernels_amd64.go) validates finite cost bounds across the input matrix via 8-lane AVX2 `Abs().LessEqual` checks when `len(cost) >= 256`, [`simd.MinReducedSlice64With`](../internal/simd/kernels_amd64.go) accelerates reduction-transfer slack scans in `preprocessSquareJV`, [`simd.Min2ReducedRow64With`](../internal/simd/kernels_amd64.go) accelerates the first/second reduced-minimum scans in `preprocessRectangularBV` and `augmentingRowReduction` via 8-way AVX2 `VSUBPD` and `VCMPPD`/`VPAND`/`VMOVMSKPD` bitmask filtering when $\max(m, n) \ge 256$, and [`simd.InitReducedRow64With`](../internal/simd/kernels_amd64.go) runs on a slice of length $\max(m, n)$ at the start of each Dijkstra augmentation when $\max(m, n) \ge 32$. All vector kernels clean upper `YMM` state via `StoreArray` + `ClearAVXUpperBits()` (`VZEROUPPER`) before scalar operations; exact per-family verdicts appear in the generated table below.

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` and `rows * cols < 256`, no AVX2 kernel runs): 0 of 2 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32` or `AllAbsLE64` at `rows * cols >= 256`): 15 faster, 0 slower, 5 with no detectable effect, out of 20.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source | 1.00x [0.86x-1.15x] | 0.0004 | 0.0004 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source | 1.16x [0.86x-1.56x] | 0.0012 | 0.0011 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | possible | no detectable effect | 1.10x [0.77x-1.58x] | 0.0037 | 0.0039 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | no detectable effect | 0.93x [0.67x-1.30x] | 0.0028 | 0.0028 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | no detectable effect | 1.03x [0.85x-1.26x] | 0.0029 | 0.0026 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | no detectable effect | 0.97x [0.70x-1.34x] | 0.0050 | 0.0057 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.27x [1.17x-1.38x] | 0.0232 | 0.0191 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.36x [1.28x-1.44x] | 0.0471 | 0.0353 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.44x [1.26x-1.65x] | 0.0561 | 0.0388 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.62x [1.49x-1.77x] | 0.135 | 0.0807 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.37x [1.21x-1.56x] | 0.0512 | 0.0363 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | no detectable effect | 1.19x [0.94x-1.51x] | 0.0270 | 0.0213 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.62x [1.32x-1.98x] | 0.0380 | 0.0258 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.99x [1.12x-3.55x] | 0.0773 | 0.0484 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.58x [1.43x-1.76x] | 0.325 | 0.181 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.04x [1.39x-3.00x] | 1.537 | 0.692 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.07x [1.68x-2.55x] | 0.722 | 0.338 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.56x [2.44x-2.69x] | 4.981 | 1.827 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.84x [1.16x-2.94x] | 0.264 | 0.125 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.62x [1.38x-1.90x] | 0.337 | 0.209 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.71x [1.57x-1.86x] | 5.648 | 3.347 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.48x [1.35x-1.64x] | 27.694 | 19.322 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

---

## 8. Keeping Documentation and Benchmarks Synchronized

1. **Regenerate telemetry** from a clean, committed tree in a single pinned session so that all seven telemetry files record identical `git_commit`, `source_fingerprint`, and `uname_release`: run the commands in the [README](../README.md#verification-benchmarking-and-documentation-sync-commands).
2. **Regenerate tables**: `go run ./cmd/goblin-bench -sync-docs`.
3. **Verify**: `go run ./cmd/goblin-bench -check-docs`, or `go test ./...`, which runs `TestDocumentationAndBenchmarkSync`. The gate validates corpus dimensions, parity, symbol identifiers, and provenance, and checks every generated block in `README.md`, `docs/benchmarks.md`, and `bench/results/final_report.md`.
