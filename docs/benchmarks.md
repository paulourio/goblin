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
- **Unified telemetry source**: fingerprint `a33eec8017a28703` (working tree on top of commit `d73197b773a8357bb4b58c816d30b2788628f346`), `go1.27.3-X:nodwarf5`, Linux kernel `6.18.63-gentoo-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 7.37 ms median on a 1-variable MILP and 7.08 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

- **Single-threaded, sequential, pinned execution**: every solve runs single-threaded (`threads=1` in CBC, one goroutine in `gb.SolveCompiled`), all harnesses run with `workers=1` in a single session on the same Linux kernel, and each harness is pinned with `taskset` to one performance core. The i9-13900K mixes 5.5 GHz P-cores (CPUs 0-13) with 4.3 GHz E-cores (CPUs 14-29); unpinned runs migrate between them and introduce bimodal variance.
- **Warm state**: each harness runs an untimed worker warmup (`_init_worker` in [`bench/python_baseline.py`](../bench/python_baseline.py), `warmWorker` in [`cmd/goblin-bench/main.go`](../cmd/goblin-bench/main.go)), then 2 untimed warmup solves per family on Tiers 1-2 and 1 on Tier 3.
- **Synthetic corpus and held-out public benchmarks**: each synthetic family in `testdata/corpus/trials/<id>.jsonl` holds 11 distinct seeded problem instances (Tiers 1-2) or 5 (Tier 3), 293 instances in total; `WriteCorpusJSON` rejects duplicate trial payloads. Trial $k$ is the identical problem instance in every harness, enabling paired per-instance ratios rather than repeated runs on a fixed matrix. Complementing the synthetic timing corpus, [`public_benchmarks_test.go`](../public_benchmarks_test.go) verifies optimal status, objective parity, and primal feasibility across 16 held-out public Netlib LP and MIPLIB/COIN-OR MILP instances in `testdata/public/`, plus a valid dual lower bound under a 25-node budget (`go_max_nodes: 25`) on `coinor_capplan1`.
- **Metrics**: per randomized instance, prep time (`t_parse + t_build + t_format/t_compile`), solve time (`t_solve`), and end-to-end time (`t_total`). Aggregate tables report paired per-instance geometric means with 95% Student-$t$ intervals on log ratios, total wall-clock sums across all instances in each subset, and full per-instance `[Min-Max]` ranges without floor clamping.
- **Parity**: the benchmark gate requires both `goblin` pure-Go builds and `goblin (CBC)` to match CBC's canonical and per-trial statuses and objectives across all 293 instances within a relative tolerance of `1e-5` under matching `1e-6` MIP gap tolerances.

---

## 2. Aggregate Workload Summary

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Instances) | Paired `Py / Go` GeoMean [95% CI] | Paired `Go / CBC` | `Go` Sum (ms) | `CBC` Sum (ms) | `Py` Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **348.04x** [282.34x-429.05x] | 0.05x | 14.1 | 212.9 | 1223.0 | 553.33x [13.45x-2062.02x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **18.38x** [15.19x-22.24x] | 0.17x | 849.9 | 2094.6 | 4692.0 | 23.70x [0.19x-135.19x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **7.93x** [5.58x-11.27x] | 0.26x | 4731.0 | 3317.2 | 6704.6 | 10.05x [0.06x-40.73x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **11.77x** [9.48x-14.60x] | 0.23x | 5247.2 | 3413.1 | 7315.0 | 12.02x [0.06x-135.19x] |
| Tier 3 non-MAD MILPs | 4 (20) | **4.68x** [2.55x-8.61x] | 0.44x | 4406.4 | 1412.7 | 3006.1 | 6.70x [0.06x-19.75x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **34.77x** [28.57x-42.31x] | 0.10x | 335.2 | 2025.9 | 4202.8 | 40.66x [3.25x-109.71x] |
| Random-weight MAD (negative controls) | 3 (33) | **13.76x** [9.08x-20.84x] | 0.18x | 462.2 | 1300.9 | 2438.0 | 12.73x [1.67x-101.06x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **54.54x** [41.46x-71.74x] | 0.12x | 5259.9 | 3598.8 | 8416.8 | 48.12x [0.06x-2062.02x] |
| All 31 synthetic corpus families | 31 (293) | **49.43x** [39.69x-61.56x] | 0.11x | 5595.1 | 5624.7 | 12619.6 | 41.46x [0.06x-2062.02x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

How to read the aggregate table:
1. **Tier 1 micro-models are dominated by subprocess cost.** On 2-to-26-variable models, Python is bounded by the CBC subprocess spawn and `.sol` parsing floor reported in the provenance block above, whereas pure-Go executes in microseconds in memory.
2. **The planted-gap MAD family is zero-gap by construction.** Its instances have `RootLPBound == OptimalObj` by design and close at the root without branching via presolve orbit fixing and root heuristics. The bold row (`Tier 2+3 non-planted MILPs`) excludes both Tier-1 subprocess micro-models and planted-gap MAD to isolate solver-dominated MILP performance.
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
- Every structured instance `t*_mad_g*` has a **zero root LP gap** (`RootLPBound == OptimalObj` with $r_g = 0$), which the root heuristics (`tryPropagatedRounding`, `tryRootSubMIP`, and `tryFractionalDiving`) close at the root without branching (1 explored node, or 2 when `tryRootSubMIP` also explores 1 sub-MIP root node on `t3_mad_g10_i100`).
- [`BuildMADRandom`](../bench/corpus.go) (`t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`, and `TestRandomMADNegativeControls` in [`solver_test.go`](../solver_test.go)) uses the same `BuildMADFromWeights` constraint matrix and assigns dense pseudorandom weights $W_{t,i} \in [100, 2000)$ to every item. Those negative controls have a strict LP-to-MIP gap, the root heuristic does not close it, and the solver must branch.

### MAD Results (`G` Groups $\times$ `I` Items, $T = 6$)

<!-- BEGIN GENERATED:MAD_PIPELINE_TABLE -->
| Problem ID | $G \times I$ | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Trial 0 Obj | Parity |
| :--- | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **82.2x** [64.9x-109.7x] | 0.13 | 11.01 | 0.07 | 10.76 | 0.052 | 0.315 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **49.0x** [13.4x-101.1x] | 0.62 | 38.79 | 0.59 | 38.53 | 0.032 | 0.325 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **67.0x** [43.6x-79.0x] | 0.14 | 10.26 | 0.11 | 9.96 | 0.039 | 0.387 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **45.9x** [35.7x-67.4x] | 0.27 | 11.08 | 0.15 | 10.66 | 0.094 | 0.562 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **32.6x** [23.7x-42.4x] | 0.38 | 12.83 | 0.27 | 12.19 | 0.102 | 0.847 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **10.8x** [3.3x-41.8x] | 4.23 | 51.39 | 4.18 | 51.04 | 0.049 | 0.419 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **4.9x** [1.7x-24.3x] | 31.09 | 178.44 | 31.01 | 177.95 | 0.088 | 0.603 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **27.5x** [20.5x-40.7x] | 0.82 | 21.05 | 0.57 | 19.83 | 0.221 | 1.471 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **13.7x** [8.1x-21.6x] | 1.52 | 21.27 | 1.15 | 19.36 | 0.370 | 2.462 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **10.7x** [10.0x-11.8x] | 6.64 | 70.19 | 5.04 | 61.62 | 1.283 | 11.629 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **8.1x** [3.2x-22.0x] | 51.06 | 394.23 | 44.58 | 358.74 | 5.466 | 46.777 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Cross-Instance Latency Dispersion Across Randomized Instances (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.14 \pm 0.02$ ($\pm 0.01$) | $11.02 \pm 0.52$ ($\pm 0.35$) | $0.12 \pm 0.01$ ($\pm 0.01$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.96 \pm 0.86$ ($\pm 0.58$) | $35.07 \pm 10.94$ ($\pm 7.35$) | $0.98 \pm 0.86$ ($\pm 0.58$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.16 \pm 0.05$ ($\pm 0.03$) | $10.54 \pm 0.62$ ($\pm 0.42$) | $0.15 \pm 0.02$ ($\pm 0.01$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.25 \pm 0.06$ ($\pm 0.04$) | $11.15 \pm 0.20$ ($\pm 0.14$) | $0.22 \pm 0.05$ ($\pm 0.04$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.41 \pm 0.08$ ($\pm 0.05$) | $13.12 \pm 0.85$ ($\pm 0.57$) | $0.40 \pm 0.11$ ($\pm 0.08$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $4.17 \pm 2.22$ ($\pm 1.49$) | $38.73 \pm 18.21$ ($\pm 12.23$) | $4.27 \pm 2.27$ ($\pm 1.52$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $36.89 \pm 30.37$ ($\pm 20.40$) | $147.84 \pm 67.52$ ($\pm 45.36$) | $37.18 \pm 30.80$ ($\pm 20.69$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.77 \pm 0.14$ ($\pm 0.18$) | $20.85 \pm 2.61$ ($\pm 3.24$) | $0.77 \pm 0.13$ ($\pm 0.16$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.65 \pm 0.62$ ($\pm 0.77$) | $21.36 \pm 0.27$ ($\pm 0.34$) | $1.57 \pm 0.56$ ($\pm 0.70$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $6.49 \pm 0.24$ ($\pm 0.30$) | $69.32 \pm 4.34$ ($\pm 5.39$) | $6.48 \pm 0.34$ ($\pm 0.42$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $56.03 \pm 15.18$ ($\pm 18.85$) | $628.17 \pm 673.03$ ($\pm 835.54$) | $53.39 \pm 17.09$ ($\pm 21.22$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | **0.07** (1 / 0) | 0.06 (1 / 0) | 0.08 (1) | 0.36 (3) | 0.071 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.59** (5 / 0) | 0.58 (12 / 0) | 0.49 (12) | 0.86 (84) | 0.070 | 4601.00 | 4257.00 |
| `t2_mad_g2_i8` | **0.11** (1 / 0) | 0.44 (9 / 0) | 0.41 (9) | 0.20 (2) | 0.086 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.15** (1 / 0) | 0.11 (1 / 0) | 0.14 (1) | 1.28 (136) | 0.211 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.27** (1 / 0) | 1.34 (15 / 0) | 1.28 (15) | 5.59 (47) | 0.269 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **4.18** (43 / 0) | 2.31 (41 / 0) | 3.06 (41) | 3.82 (126) | 0.118 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **31.01** (137 / 0) | 26.66 (201 / 0) | 12.03 (201) | 205.95 (1912) | 0.242 | 7720.00 | 6060.00 |
| `t3_mad_g5_i20` | **0.57** (1 / 0) | 1.96 (14 / 0) | 1.84 (14) | 1.40 (1) | 0.909 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **1.15** (1 / 0) | 2.72 (12 / 0) | 2.61 (12) | 6.30 (27) | 1.428 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **5.04** (1 / 0) | 51.78 (130 / 0) | 50.78 (130) | 207.66 (270) | 15.561 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **44.58** (1 / 0) | 44.26 (2 / 0) | 44.98 (2) | 250.18 (132, `TimeLimit`) | 116.617 | 208000.00 | 208000.00 |
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
| `t1_std_lp` | 3 | 3 | **826.5x** [548.6x-1243.1x] | 0.01 | 7.02 | 0.00 | 6.97 | 0.006 | 0.076 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **895.4x** [598.3x-1232.8x] | 0.01 | 7.28 | 0.00 | 7.22 | 0.006 | 0.081 | `OK` |
| `t1_std_mip` | 3 | 3 | **605.0x** [245.9x-952.8x] | 0.01 | 8.50 | 0.01 | 8.43 | 0.006 | 0.090 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **675.4x** [502.6x-972.5x] | 0.01 | 8.52 | 0.01 | 8.46 | 0.006 | 0.089 | `OK` |
| `t1_whiskas` | 6 | 5 | **494.0x** [354.1x-712.9x] | 0.02 | 7.43 | 0.00 | 7.34 | 0.011 | 0.118 | `OK` |
| `t1_furniture` | 2 | 3 | **311.4x** [100.7x-636.9x] | 0.03 | 9.14 | 0.02 | 9.07 | 0.005 | 0.089 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **140.8x** [67.9x-268.8x] | 0.08 | 10.48 | 0.07 | 10.38 | 0.013 | 0.138 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **988.3x** [69.4x-2062.0x] | 0.01 | 7.04 | 0.00 | 6.99 | 0.005 | 0.073 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **82.2x** [64.9x-109.7x] | 0.13 | 11.01 | 0.07 | 10.76 | 0.052 | 0.315 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **49.0x** [13.4x-101.1x] | 0.62 | 38.79 | 0.59 | 38.53 | 0.032 | 0.325 | `OK` |
| `t2_mkp_30` | 30 | 6 | **8.3x** [4.3x-16.9x] | 2.95 | 30.21 | 2.92 | 30.03 | 0.032 | 0.252 | `OK` |
| `t2_mkp_50` | 50 | 8 | **5.5x** [2.5x-55.9x] | 4.97 | 19.33 | 4.91 | 19.06 | 0.049 | 0.368 | `OK` |
| `t2_cflp_36` | 36 | 11 | **32.6x** [4.7x-110.4x] | 0.25 | 10.85 | 0.20 | 10.65 | 0.038 | 0.250 | `OK` |
| `t2_cflp_49` | 49 | 13 | **28.7x** [10.7x-62.4x] | 0.38 | 11.72 | 0.35 | 11.49 | 0.030 | 0.292 | `OK` |
| `t2_gap_45` | 45 | 13 | **12.2x** [2.4x-32.5x] | 1.02 | 16.24 | 0.99 | 16.02 | 0.034 | 0.282 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **10.8x** [3.9x-30.3x] | 4.85 | 74.28 | 4.77 | 74.02 | 0.043 | 0.326 | `OK` |
| `t2_binpack_44` | 44 | 14 | **15.1x** [0.2x-88.0x] | 0.46 | 15.02 | 0.43 | 14.82 | 0.028 | 0.282 | `OK` |
| `t2_setcover_50` | 50 | 20 | **59.8x** [5.6x-135.2x] | 0.16 | 12.11 | 0.12 | 11.76 | 0.044 | 0.435 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **67.0x** [43.6x-79.0x] | 0.14 | 10.26 | 0.11 | 9.96 | 0.039 | 0.387 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **45.9x** [35.7x-67.4x] | 0.27 | 11.08 | 0.15 | 10.66 | 0.094 | 0.562 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **32.6x** [23.7x-42.4x] | 0.38 | 12.83 | 0.27 | 12.19 | 0.102 | 0.847 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **10.8x** [3.3x-41.8x] | 4.23 | 51.39 | 4.18 | 51.04 | 0.049 | 0.419 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **4.9x** [1.7x-24.3x] | 31.09 | 178.44 | 31.01 | 177.95 | 0.088 | 0.603 | `OK` |
| `t3_mkp_200` | 200 | 8 | **3.2x** [2.8x-3.9x] | 37.55 | 124.49 | 37.44 | 123.85 | 0.111 | 0.846 | `OK` |
| `t3_cflp_300` | 300 | 34 | **13.1x** [9.5x-19.7x] | 15.38 | 265.30 | 15.20 | 264.35 | 0.180 | 1.239 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **1.0x** [0.1x-2.5x] | 100.35 | 167.99 | 99.84 | 165.97 | 0.366 | 2.460 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **11.5x** [9.6x-14.1x] | 8.02 | 85.72 | 7.59 | 82.61 | 0.445 | 3.680 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **27.5x** [20.5x-40.7x] | 0.82 | 21.05 | 0.57 | 19.83 | 0.221 | 1.471 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **13.7x** [8.1x-21.6x] | 1.52 | 21.27 | 1.15 | 19.36 | 0.370 | 2.462 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **10.7x** [10.0x-11.8x] | 6.64 | 70.19 | 5.04 | 61.62 | 1.283 | 11.629 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **8.1x** [3.2x-22.0x] | 51.06 | 394.23 | 44.58 | 358.74 | 5.466 | 46.777 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## 5. Default Build vs. `GOEXPERIMENT=simd`

In [`internal/simd/kernels_amd64.go`](../internal/simd/kernels_amd64.go), AVX2 kernels (gated at runtime on CPU `AVX2` and `FMA` support) spill 4-lane accumulators to the stack via `StoreArray` and execute `archsimd.ClearAVXUpperBits()` (`VZEROUPPER`) before scalar tail or caller operations, eliminating AVX-to-SSE dirty-upper `YMM` transition stalls while preserving `NaN` propagation parity. From the kernel dispatch thresholds upward (`minAVX2Len = 32` for `Dot64`, `Axpy64`, `Scale64`, `AbsMax64`, `RowActivityBoundsFinite`, `MinReducedSlice64`, and `InitReducedRow64`; $N \ge 64$ for `Min2ReducedRow64`; and caller-gated at $N \ge 256$ in `assign/solver.go` for `AllAbsLE64`), the 256-bit AVX2 loops outperform the 4-way slice-stepped scalar loops; below those lengths, call and `VZEROUPPER` overhead dominates, so shorter slices stay on the scalar path.

At the full-solver level, each ratio below is the geometric mean of per-trial `scalar / SIMD` solve times over the same instances, with a 95% $t$-interval on the log ratios; "no detectable effect" means the interval contains 1. "Reachable" is a necessary condition: the AVX2 `Dot64`/`Axpy64`/`Scale64` paths require a slice of length 32 or more (`n + m >= 32`). Below that size no AVX2 kernel can run, so those 8 families serve as negative controls where intervals excluding 1 reflect binary code-layout or run-to-run variance rather than SIMD execution. Lowering `minAVX2Len` from 256 to 32 enables AVX2 across dense tableau row accumulation (`ComputeTableauRow`) for medium models with $n \le 256$ (`DenseThreshold`) and dual multiplier/reduced-cost updates inside `runDualSimplexLoop`, while `ComputeTableauRow` dispatches `Axpy64` on contiguous CSR column blocks in large models ($n > 256$, such as `t3_mad_g4_i1000`).

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 0 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 1 faster, 1 slower, 21 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 0.98x [0.94x-1.02x] | 0.002 | 0.002 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 1.02x [0.99x-1.05x] | 0.002 | 0.002 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 1.07x [0.99x-1.15x] | 0.006 | 0.006 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source | 1.00x [0.96x-1.05x] | 0.005 | 0.005 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 0.99x [0.94x-1.04x] | 0.003 | 0.003 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 0.98x [0.94x-1.03x] | 0.019 | 0.017 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source | 1.01x [0.99x-1.03x] | 0.066 | 0.065 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 1.18x [0.32x-4.36x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 1.02x [0.93x-1.11x] | 0.066 | 0.065 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 0.97x [0.92x-1.03x] | 0.591 | 0.587 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 0.98x [0.92x-1.04x] | 2.916 | 3.050 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 1.01x [0.91x-1.12x] | 4.908 | 4.963 | 11 |
| `t2_cflp_36` | 47 | possible | SIMD faster | 1.21x [1.16x-1.25x] | 0.205 | 0.165 | 11 |
| `t2_cflp_49` | 62 | possible | SIMD slower | 0.91x [0.86x-0.97x] | 0.347 | 0.419 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 1.01x [0.93x-1.10x] | 0.986 | 1.149 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 1.01x [0.97x-1.05x] | 4.771 | 4.870 | 11 |
| `t2_binpack_44` | 58 | possible | no detectable effect | 0.98x [0.92x-1.04x] | 0.430 | 0.437 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 0.90x [0.75x-1.07x] | 0.118 | 0.134 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 1.07x [0.95x-1.20x] | 0.105 | 0.103 | 11 |
| `t2_mad_g3_i9` | 108 | possible | no detectable effect | 1.11x [0.86x-1.44x] | 0.151 | 0.119 | 11 |
| `t2_mad_g4_i12` | 156 | possible | no detectable effect | 0.99x [0.81x-1.22x] | 0.266 | 0.253 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 0.98x [0.92x-1.06x] | 4.178 | 4.350 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 1.03x [0.95x-1.11x] | 31.015 | 31.371 | 11 |
| `t3_mkp_200` | 208 | possible | no detectable effect | 1.04x [0.98x-1.10x] | 37.443 | 36.625 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.00x [0.99x-1.02x] | 15.197 | 14.925 | 5 |
| `t3_lotsizing_504` | 840 | possible | no detectable effect | 0.99x [0.99x-1.00x] | 99.840 | 100.231 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 0.89x [0.71x-1.12x] | 7.592 | 8.236 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 1.04x [0.78x-1.37x] | 0.572 | 0.543 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 1.04x [0.85x-1.27x] | 1.150 | 1.115 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 1.01x [0.88x-1.15x] | 5.035 | 4.751 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | no detectable effect | 1.07x [0.97x-1.17x] | 44.578 | 39.380 | 5 |
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
| `t1_knapsack_12` | **0.07** (1 / 8) | 0.08 (2 / 8) | 0.09 (24) | 0.13 (31) | 0.005 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.07** (1 / 0) | 0.06 (1 / 0) | 0.08 (1) | 0.36 (3) | 0.071 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.59** (5 / 0) | 0.58 (12 / 0) | 0.49 (12) | 0.86 (84) | 0.070 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.92** (29 / 8) | 2.17 (60 / 8) | 1.00 (77) | 0.87 (77) | 0.020 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **4.91** (1 / 4) | 4.45 (2 / 4) | 4.36 (132) | 3.66 (119) | 0.032 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.20** (1 / 2) | 1.09 (3 / 2) | 0.15 (7) | 0.18 (7) | 0.021 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.35** (1 / 20) | 1.65 (5 / 20) | 0.31 (9) | 0.33 (9) | 0.028 | 909.02 | 827.30 |
| `t2_gap_45` | **0.99** (1 / 8) | 0.27 (1 / 8) | 1.66 (61) | 2.23 (61) | 0.027 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **4.77** (3 / 35) | 5.84 (33 / 35) | 2.10 (198) | 2.12 (174) | 0.052 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **0.43** (1 / 5) | 0.96 (13 / 5) | 15.36 (1838) | 26.17 (2450) | 0.026 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.12** (1 / 6) | 0.28 (2 / 6) | 0.86 (9) | 0.87 (9) | 0.033 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.11** (1 / 0) | 0.44 (9 / 0) | 0.41 (9) | 0.20 (2) | 0.086 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.15** (1 / 0) | 0.11 (1 / 0) | 0.14 (1) | 1.28 (136) | 0.211 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.27** (1 / 0) | 1.34 (15 / 0) | 1.28 (15) | 5.59 (47) | 0.269 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **4.18** (43 / 0) | 2.31 (41 / 0) | 3.06 (41) | 3.82 (126) | 0.118 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **31.01** (137 / 0) | 26.66 (201 / 0) | 12.03 (201) | 205.95 (1912) | 0.242 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **37.44** (594 / 14) | 45.29 (701 / 14) | 97.51 (5000, `NodeLimit`) | 76.48 (5000, `NodeLimit`) | 0.122 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **15.20** (48 / 31) | 22.93 (63 / 31) | 6.59 (134) | 6.84 (134) | 0.160 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **99.84** (27 / 203) | 122.63 (416 / 203) | 243.02 (5000, `NodeLimit`) | 223.43 (5000, `NodeLimit`) | 0.845 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **7.59** (35 / 47) | 12.64 (96 / 47) | 247.87 (5000, `NodeLimit`) | 174.80 (5000, `NodeLimit`) | 0.154 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.57** (1 / 0) | 1.96 (14 / 0) | 1.84 (14) | 1.40 (1) | 0.909 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **1.15** (1 / 0) | 2.72 (12 / 0) | 2.61 (12) | 6.30 (27) | 1.428 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **5.04** (1 / 0) | 51.78 (130 / 0) | 50.78 (130) | 207.66 (270) | 15.561 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **44.58** (1 / 0) | 44.26 (2 / 0) | 44.98 (2) | 250.18 (132, `TimeLimit`) | 116.617 | 208000.00 | 208000.00 |
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
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.69x** [1.56x-1.84x] | 4.82x | 0.217 | 0.339 | 1.69x [0.62x-3.64x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **3.15x** [2.78x-3.57x] | 3.05x | 10.499 | 35.227 | 2.73x [0.84x-17.40x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.87x** [2.47x-3.33x] | 3.70x | 260.127 | 1063.404 | 2.88x [0.96x-5.98x] |
| Square families ($m = n$) | 16 (146) | **2.79x** [2.52x-3.10x] | 3.93x | 265.980 | 1087.341 | 2.55x [0.62x-17.40x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.92x** [1.76x-2.08x] | 3.04x | 4.863 | 11.629 | 2.09x [0.82x-3.28x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.47x** [2.03x-3.01x] | 2.90x | 37.402 | 69.561 | 1.94x [0.81x-17.40x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.52x** [2.32x-2.74x] | 3.67x | 270.843 | 1098.970 | 2.27x [0.62x-17.40x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 7.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **2.40x** | 0.0005 | 0.0013 | 0.0006 | 0.0206 | $0.0008 \pm 0.0009$ | $0.0014 \pm 0.0003$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.75x** | 0.0014 | 0.0024 | 0.0017 | 0.0106 | $0.0014 \pm 0.0004$ | $0.0024 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.45x** | 0.0042 | 0.0065 | 0.0061 | 0.0159 | $0.0045 \pm 0.0016$ | $0.0064 \pm 0.0017$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.44x** | 0.0029 | 0.0042 | 0.0045 | 0.0148 | $0.0030 \pm 0.0011$ | $0.0041 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.55x** | 0.0031 | 0.0051 | 0.0049 | 0.0160 | $0.0034 \pm 0.0012$ | $0.0052 \pm 0.0019$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.72x** | 0.0064 | 0.0115 | 0.0116 | 0.0345 | $0.0067 \pm 0.0021$ | $0.0112 \pm 0.0021$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.80x** | 0.0288 | 0.0753 | 0.0395 | 0.102 | $0.0267 \pm 0.0061$ | $0.0734 \pm 0.0109$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.92x** | 0.0554 | 0.186 | 0.0735 | 0.238 | $0.0540 \pm 0.0053$ | $0.242 \pm 0.189$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.32x** | 0.0464 | 0.0999 | 0.0703 | 0.151 | $0.0437 \pm 0.0042$ | $0.102 \pm 0.0146$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.67x** | 0.145 | 0.233 | 0.153 | 0.274 | $0.154 \pm 0.0459$ | $0.252 \pm 0.0488$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **13.47x** | 0.0574 | 0.721 | 0.0799 | 0.800 | $0.0544 \pm 0.0072$ | $0.726 \pm 0.0187$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **2.23x** | 0.0332 | 0.0737 | 0.0577 | 0.152 | $0.0357 \pm 0.0128$ | $0.0775 \pm 0.0124$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.11x** | 0.0400 | 0.0845 | 0.0671 | 0.163 | $0.0403 \pm 0.0030$ | $0.0851 \pm 0.0078$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.24x** | 0.0875 | 0.299 | 0.118 | 0.382 | $0.0938 \pm 0.0151$ | $0.302 \pm 0.0321$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.46x** | 0.334 | 1.348 | 0.445 | 1.815 | $0.452 \pm 0.384$ | $1.342 \pm 0.130$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.16x** | 1.323 | 5.552 | 1.755 | 8.231 | $1.356 \pm 0.145$ | $5.629 \pm 0.405$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.69x** | 0.836 | 1.566 | 1.417 | 4.602 | $0.975 \pm 0.364$ | $1.579 \pm 0.0676$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.70x** | 5.325 | 9.132 | 5.593 | 10.215 | $5.926 \pm 2.089$ | $9.917 \pm 2.859$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.85x** | 0.325 | 0.879 | 0.728 | 3.962 | $0.318 \pm 0.0137$ | $0.905 \pm 0.0630$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.21x** | 0.480 | 1.042 | 0.869 | 4.044 | $0.474 \pm 0.0480$ | $1.042 \pm 0.0360$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.84x** | 6.604 | 32.980 | 8.477 | 43.616 | $6.690 \pm 0.489$ | $32.521 \pm 4.219$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.43x** | 36.174 | 150.660 | 43.823 | 198.987 | $36.287 \pm 2.528$ | $161.087 \pm 18.550$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 7.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSlice64`, `InitReducedRow64`, and `Min2ReducedRow64`)

In [`assign/solver.go`](../assign/solver.go), [`simd.AllAbsLE64With`](../internal/simd/kernels_amd64.go) validates finite cost bounds across the input matrix via 8-lane AVX2 `Abs().LessEqual` checks when `len(cost) >= 256`, [`simd.MinReducedSlice64With`](../internal/simd/kernels_amd64.go) accelerates reduction-transfer slack scans in `preprocessSquareJV`, [`simd.Min2ReducedRow64With`](../internal/simd/kernels_amd64.go) accelerates the first/second reduced-minimum scans in `preprocessRectangularBV` and `augmentingRowReduction` via 8-way AVX2 `VSUBPD` and `VCMPPD`/`VPAND`/`VMOVMSKPD` bitmask filtering when $\max(m, n) \ge 64$, and [`simd.InitReducedRow64With`](../internal/simd/kernels_amd64.go) runs on a slice of length $\max(m, n)$ at the start of each Dijkstra augmentation when $\max(m, n) \ge 32$. All vector kernels clean upper `YMM` state via `StoreArray` + `ClearAVXUpperBits()` (`VZEROUPPER`) before scalar operations; exact per-family verdicts appear in the generated table below.

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32`, no AVX2 row-reduction kernel runs): 3 of 4 have a 95% interval excluding 1.
- **Instances where AVX2 row kernels run** (`max(rows, cols) >= 32`): 6 faster, 6 slower, 6 with no detectable effect, out of 18.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 row kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source | 1.04x [0.96x-1.12x] | 0.0005 | 0.0004 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.13x [1.09x-1.17x] | 0.0014 | 0.0013 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.10x [1.08x-1.13x] | 0.0042 | 0.0038 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | SIMD slower | 0.95x [0.93x-0.98x] | 0.0029 | 0.0031 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | no detectable effect | 0.97x [0.93x-1.01x] | 0.0031 | 0.0032 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.93x [0.88x-0.97x] | 0.0064 | 0.0068 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD slower | 0.81x [0.79x-0.83x] | 0.0288 | 0.0343 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD slower | 0.85x [0.82x-0.88x] | 0.0554 | 0.0668 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD slower | 0.88x [0.87x-0.90x] | 0.0464 | 0.0530 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD slower | 0.97x [0.95x-0.98x] | 0.145 | 0.152 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | no detectable effect | 1.04x [0.99x-1.09x] | 0.0574 | 0.0570 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.13x [1.09x-1.19x] | 0.0332 | 0.0292 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | no detectable effect | 1.01x [0.96x-1.06x] | 0.0400 | 0.0409 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD slower | 0.85x [0.78x-0.92x] | 0.0875 | 0.104 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | no detectable effect | 1.11x [0.88x-1.42x] | 0.334 | 0.334 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.10x [1.05x-1.16x] | 1.323 | 1.173 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | no detectable effect | 1.15x [0.57x-2.34x] | 0.836 | 0.678 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | no detectable effect | 1.03x [0.98x-1.09x] | 5.325 | 5.316 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.64x [1.29x-2.09x] | 0.325 | 0.203 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.23x [1.03x-1.47x] | 0.480 | 0.372 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.12x [1.02x-1.23x] | 6.604 | 5.848 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.14x [1.11x-1.16x] | 36.174 | 31.348 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

---

## 8. Keeping Documentation and Benchmarks Synchronized

1. **Regenerate telemetry** from a clean, committed tree in a single pinned session so that all seven telemetry files record identical `git_commit`, `source_fingerprint`, and `uname_release`: run the commands in the [README](../README.md#verification-benchmarking-and-documentation-sync-commands).
2. **Regenerate tables**: `go run ./cmd/goblin-bench -sync-docs`.
3. **Verify**: `go run ./cmd/goblin-bench -check-docs`, or `go test ./...`, which runs `TestDocumentationAndBenchmarkSync`. The gate validates corpus dimensions, parity, symbol identifiers, and provenance, and checks every generated block in `README.md`, `docs/benchmarks.md`, and `bench/results/final_report.md`.
