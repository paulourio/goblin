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
- **Unified telemetry source**: fingerprint `743b1e77616c6db6` (commit `bf1fe0d7d320fac6cfefd176a27efe3bef0f291f`), `go1.27.3-X:simd`, Linux kernel `6.18.63-gentoo-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, or kernel differs.
- **CPU affinity**: goblin `6`; Python + CBC `6`; Python + SciPy `6`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 7.21 ms median on a 1-variable MILP and 6.81 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
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
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **424.01x** [330.79x-543.51x] | 0.04x | 19.1 | 200.4 | 1147.5 | 669.67x [10.95x-2723.98x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **15.22x** [12.41x-18.66x] | 0.21x | 1024.2 | 1956.9 | 4466.3 | 17.69x [0.10x-174.55x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **5.72x** [3.87x-8.45x] | 0.33x | 7431.1 | 3574.5 | 6334.9 | 7.30x [0.04x-36.01x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **8.93x** [7.11x-11.22x] | 0.31x | 8080.7 | 3105.7 | 6973.4 | 8.50x [0.04x-174.55x] |
| Tier 3 non-MAD MILPs | 4 (20) | **2.98x** [1.58x-5.63x] | 0.75x | 7065.5 | 1246.8 | 2893.3 | 3.13x [0.04x-19.99x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **35.52x** [27.77x-45.44x] | 0.09x | 375.4 | 2452.7 | 3942.5 | 40.92x [2.09x-166.44x] |
| Random-weight MAD (negative controls) | 3 (33) | **9.89x** [6.84x-14.30x] | 0.24x | 475.0 | 1208.0 | 2282.9 | 9.37x [0.55x-81.37x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **50.04x** [36.75x-68.15x] | 0.13x | 8098.9 | 3279.1 | 8006.2 | 29.05x [0.04x-2723.98x] |
| All 31 synthetic corpus families | 31 (293) | **46.43x** [36.25x-59.48x] | 0.12x | 8474.3 | 5731.8 | 11948.7 | 36.01x [0.04x-2723.98x] |
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
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **136.6x** [89.0x-166.4x] | 0.07 | 10.69 | 0.04 | 10.46 | 0.024 | 0.293 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **27.4x** [11.0x-81.4x] | 1.38 | 35.30 | 1.34 | 35.06 | 0.040 | 0.304 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **56.4x** [41.4x-93.4x] | 0.19 | 10.38 | 0.13 | 10.09 | 0.043 | 0.380 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **53.2x** [35.9x-69.7x] | 0.20 | 11.42 | 0.13 | 11.00 | 0.063 | 0.554 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **32.9x** [21.9x-40.5x] | 0.37 | 12.96 | 0.29 | 12.34 | 0.083 | 0.817 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **8.3x** [3.1x-29.0x] | 5.19 | 49.02 | 5.11 | 48.72 | 0.045 | 0.392 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **4.3x** [0.6x-16.4x] | 34.10 | 167.82 | 33.93 | 167.39 | 0.083 | 0.566 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **25.3x** [18.9x-36.0x] | 0.76 | 19.39 | 0.57 | 18.31 | 0.190 | 1.408 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **10.5x** [2.1x-19.7x] | 1.47 | 19.89 | 1.20 | 18.14 | 0.276 | 2.295 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **7.1x** [6.6x-7.6x] | 9.15 | 61.25 | 7.73 | 53.71 | 1.233 | 10.698 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **7.6x** [3.5x-13.5x] | 43.51 | 375.64 | 38.53 | 341.71 | 4.837 | 44.357 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Cross-Instance Latency Dispersion Across Randomized Instances (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.08 \pm 0.02$ ($\pm 0.01$) | $10.42 \pm 0.65$ ($\pm 0.44$) | $0.08 \pm 0.02$ ($\pm 0.01$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $1.48 \pm 1.03$ ($\pm 0.69$) | $31.73 \pm 9.52$ ($\pm 6.40$) | $1.43 \pm 0.99$ ($\pm 0.66$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.19 \pm 0.05$ ($\pm 0.03$) | $10.41 \pm 0.59$ ($\pm 0.40$) | $0.18 \pm 0.04$ ($\pm 0.03$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.22 \pm 0.05$ ($\pm 0.03$) | $11.55 \pm 0.27$ ($\pm 0.18$) | $0.22 \pm 0.05$ ($\pm 0.03$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.41 \pm 0.08$ ($\pm 0.05$) | $13.15 \pm 0.59$ ($\pm 0.40$) | $0.43 \pm 0.09$ ($\pm 0.06$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $5.34 \pm 3.34$ ($\pm 2.24$) | $36.67 \pm 17.08$ ($\pm 11.48$) | $5.58 \pm 3.44$ ($\pm 2.31$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $36.35 \pm 25.34$ ($\pm 17.02$) | $139.14 \pm 63.32$ ($\pm 42.54$) | $36.31 \pm 24.64$ ($\pm 16.55$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.76 \pm 0.13$ ($\pm 0.16$) | $19.18 \pm 2.39$ ($\pm 2.97$) | $0.68 \pm 0.07$ ($\pm 0.09$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $2.90 \pm 3.59$ ($\pm 4.46$) | $19.91 \pm 0.54$ ($\pm 0.67$) | $2.83 \pm 3.47$ ($\pm 4.31$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $8.96 \pm 0.49$ ($\pm 0.61$) | $63.78 \pm 3.83$ ($\pm 4.76$) | $8.94 \pm 1.13$ ($\pm 1.40$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $60.49 \pm 35.69$ ($\pm 44.31$) | $585.45 \pm 613.38$ ($\pm 761.49$) | $60.89 \pm 34.79$ ($\pm 43.19$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | **0.04** (1 / 0) | 0.07 (1 / 0) | 0.08 (1) | 0.91 (3) | 0.083 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **1.34** (10 / 0) | 0.82 (12 / 0) | 0.82 (12) | 1.75 (84) | 0.086 | 4601.00 | 4257.00 |
| `t2_mad_g2_i8` | **0.13** (1 / 0) | 0.84 (9 / 0) | 0.75 (9) | 0.38 (2) | 0.091 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.13** (1 / 0) | 0.12 (1 / 0) | 0.13 (1) | 3.07 (5) | 0.229 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.29** (1 / 0) | 5.50 (53 / 0) | 5.46 (53) | 6.46 (21) | 0.306 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **5.11** (54 / 0) | 4.87 (84 / 0) | 4.15 (84) | 21.78 (334) | 0.144 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **33.93** (250 / 0) | 20.20 (219 / 0) | 20.30 (219) | 206.87 (1370) | 0.262 | 7720.00 | 6060.00 |
| `t3_mad_g5_i20` | **0.57** (1 / 0) | 3.50 (14 / 0) | 4.05 (14) | 6.75 (32) | 0.907 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **1.20** (1 / 0) | 7.21 (12 / 0) | 7.59 (12) | 64.15 (130) | 1.464 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.73** (2 / 0) | 87.45 (107 / 0) | 86.50 (107) | 110.81 (124) | 15.042 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **38.53** (1 / 0) | 54.22 (2 / 0) | 49.56 (2) | 57.85 (0, `NumericalError`) | 111.432 | 208000.00 | 0.00 |
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
| `t1_std_lp` | 3 | 3 | **860.2x** [609.2x-1326.3x] | 0.01 | 6.94 | 0.00 | 6.82 | 0.005 | 0.087 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **942.8x** [442.8x-1404.0x] | 0.01 | 6.76 | 0.00 | 6.70 | 0.004 | 0.074 | `OK` |
| `t1_std_mip` | 3 | 3 | **962.4x** [678.3x-1175.6x] | 0.01 | 8.56 | 0.00 | 8.50 | 0.003 | 0.088 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **1018.8x** [880.6x-1197.5x] | 0.01 | 7.98 | 0.00 | 7.92 | 0.004 | 0.091 | `OK` |
| `t1_whiskas` | 6 | 5 | **599.6x** [406.6x-726.5x] | 0.01 | 6.93 | 0.00 | 6.83 | 0.006 | 0.111 | `OK` |
| `t1_furniture` | 2 | 3 | **343.2x** [89.6x-958.5x] | 0.02 | 8.62 | 0.01 | 8.55 | 0.003 | 0.082 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **127.1x** [56.0x-291.1x] | 0.07 | 10.22 | 0.07 | 10.13 | 0.009 | 0.130 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **2412.7x** [1852.4x-2724.0x] | 0.00 | 6.72 | 0.00 | 6.66 | 0.002 | 0.073 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **136.6x** [89.0x-166.4x] | 0.07 | 10.69 | 0.04 | 10.46 | 0.024 | 0.293 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **27.4x** [11.0x-81.4x] | 1.38 | 35.30 | 1.34 | 35.06 | 0.040 | 0.304 | `OK` |
| `t2_mkp_30` | 30 | 6 | **6.8x** [3.9x-12.7x] | 3.31 | 30.18 | 3.29 | 30.00 | 0.023 | 0.237 | `OK` |
| `t2_mkp_50` | 50 | 8 | **4.0x** [1.8x-32.8x] | 6.77 | 17.57 | 6.71 | 17.31 | 0.056 | 0.347 | `OK` |
| `t2_cflp_36` | 36 | 11 | **31.1x** [2.9x-174.5x] | 0.28 | 10.65 | 0.25 | 10.47 | 0.023 | 0.239 | `OK` |
| `t2_cflp_49` | 49 | 13 | **17.6x** [7.6x-29.0x] | 0.69 | 11.37 | 0.64 | 11.15 | 0.034 | 0.286 | `OK` |
| `t2_gap_45` | 45 | 13 | **9.6x** [1.3x-30.3x] | 1.24 | 14.93 | 1.22 | 14.73 | 0.028 | 0.271 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **8.4x** [4.4x-21.0x] | 7.03 | 69.97 | 6.96 | 69.73 | 0.046 | 0.310 | `OK` |
| `t2_binpack_44` | 44 | 14 | **11.7x** [0.1x-36.1x] | 0.63 | 14.72 | 0.61 | 14.51 | 0.035 | 0.267 | `OK` |
| `t2_setcover_50` | 50 | 20 | **48.3x** [3.4x-132.5x] | 0.20 | 11.90 | 0.13 | 11.59 | 0.048 | 0.410 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **56.4x** [41.4x-93.4x] | 0.19 | 10.38 | 0.13 | 10.09 | 0.043 | 0.380 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **53.2x** [35.9x-69.7x] | 0.20 | 11.42 | 0.13 | 11.00 | 0.063 | 0.554 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **32.9x** [21.9x-40.5x] | 0.37 | 12.96 | 0.29 | 12.34 | 0.083 | 0.817 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **8.3x** [3.1x-29.0x] | 5.19 | 49.02 | 5.11 | 48.72 | 0.045 | 0.392 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **4.3x** [0.6x-16.4x] | 34.10 | 167.82 | 33.93 | 167.39 | 0.083 | 0.566 | `OK` |
| `t3_mkp_200` | 200 | 8 | **2.2x** [2.0x-2.5x] | 55.46 | 116.02 | 55.36 | 115.41 | 0.106 | 0.808 | `OK` |
| `t3_cflp_300` | 300 | 34 | **11.6x** [7.3x-20.0x] | 18.88 | 256.93 | 18.62 | 256.02 | 0.156 | 1.178 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **0.6x** [0.0x-1.4x] | 180.47 | 176.57 | 180.18 | 174.75 | 0.279 | 2.289 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **5.3x** [3.8x-6.9x] | 15.57 | 81.53 | 15.11 | 78.64 | 0.427 | 3.464 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **25.3x** [18.9x-36.0x] | 0.76 | 19.39 | 0.57 | 18.31 | 0.190 | 1.408 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **10.5x** [2.1x-19.7x] | 1.47 | 19.89 | 1.20 | 18.14 | 0.276 | 2.295 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **7.1x** [6.6x-7.6x] | 9.15 | 61.25 | 7.73 | 53.71 | 1.233 | 10.698 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **7.6x** [3.5x-13.5x] | 43.51 | 375.64 | 38.53 | 341.71 | 4.837 | 44.357 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## 5. Default Build vs. `GOEXPERIMENT=simd`

In [`internal/simd/kernels_amd64.go`](../internal/simd/kernels_amd64.go), AVX2 kernels (gated at runtime on CPU `AVX2` and `FMA` support) spill 4-lane accumulators to the stack via `StoreArray` and execute `archsimd.ClearAVXUpperBits()` (`VZEROUPPER`) before scalar tail or caller operations, eliminating AVX-to-SSE dirty-upper `YMM` transition stalls while preserving `NaN` propagation parity. From the kernel dispatch thresholds upward (`minAVX2Len = 32` for `Dot64`, `Axpy64`, `Scale64`, `AbsMax64`, `RowActivityBoundsFinite`, `MinReducedSlice64`, and `InitReducedRow64`; $N \ge 64$ for `Min2ReducedRow64`; and caller-gated at $N \ge 256$ in `assign/solver.go` for `AllAbsLE64`), the 256-bit AVX2 loops outperform the 4-way slice-stepped scalar loops; below those lengths, call and `VZEROUPPER` overhead dominates, so shorter slices stay on the scalar path.

At the full-solver level, each ratio below is the geometric mean of per-trial `scalar / SIMD` solve times over the same instances, with a 95% $t$-interval on the log ratios; "no detectable effect" means the interval contains 1. "Reachable" is a necessary condition: the AVX2 `Dot64`/`Axpy64`/`Scale64` paths require a slice of length 32 or more (`n + m >= 32`). Below that size no AVX2 kernel can run, so those 8 families serve as negative controls where intervals excluding 1 reflect binary code-layout or run-to-run variance rather than SIMD execution. Lowering `minAVX2Len` from 256 to 32 enables AVX2 across dense tableau row accumulation (`ComputeTableauRow`) for medium models with $n \le 256$ (`DenseThreshold`) and dual multiplier/reduced-cost updates inside `runDualSimplexLoop`, while `ComputeTableauRow` dispatches `Axpy64` on contiguous CSR column blocks in large models ($n > 256$, such as `t3_mad_g4_i1000`).

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 1 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 0 faster, 1 slower, 22 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 0.89x [0.77x-1.02x] | 0.002 | 0.002 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 0.96x [0.79x-1.16x] | 0.002 | 0.002 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 0.99x [0.82x-1.19x] | 0.004 | 0.004 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source | 0.99x [0.89x-1.10x] | 0.004 | 0.004 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 1.00x [0.84x-1.20x] | 0.004 | 0.004 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 0.95x [0.83x-1.08x] | 0.014 | 0.015 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source | 1.18x [0.84x-1.68x] | 0.065 | 0.071 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.88x [0.78x-1.00x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 1.01x [0.94x-1.10x] | 0.045 | 0.046 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 1.05x [0.99x-1.11x] | 1.342 | 1.338 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 0.98x [0.95x-1.01x] | 3.287 | 3.637 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 1.00x [0.96x-1.04x] | 6.712 | 7.089 | 11 |
| `t2_cflp_36` | 47 | possible | no detectable effect | 1.03x [0.94x-1.13x] | 0.253 | 0.245 | 11 |
| `t2_cflp_49` | 62 | possible | no detectable effect | 0.98x [0.94x-1.02x] | 0.643 | 0.660 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 0.99x [0.96x-1.03x] | 1.218 | 1.239 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 0.99x [0.95x-1.03x] | 6.963 | 6.887 | 11 |
| `t2_binpack_44` | 58 | possible | no detectable effect | 0.93x [0.84x-1.04x] | 0.605 | 0.638 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 1.00x [0.92x-1.10x] | 0.135 | 0.152 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 1.00x [0.97x-1.04x] | 0.130 | 0.134 | 11 |
| `t2_mad_g3_i9` | 108 | possible | no detectable effect | 1.06x [0.97x-1.16x] | 0.126 | 0.137 | 11 |
| `t2_mad_g4_i12` | 156 | possible | no detectable effect | 0.96x [0.85x-1.07x] | 0.288 | 0.286 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | SIMD slower | 0.94x [0.88x-1.00x] | 5.109 | 5.104 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 0.99x [0.96x-1.01x] | 33.934 | 33.921 | 11 |
| `t3_mkp_200` | 208 | possible | no detectable effect | 1.00x [0.99x-1.02x] | 55.358 | 54.902 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.00x [0.98x-1.01x] | 18.618 | 18.425 | 5 |
| `t3_lotsizing_504` | 840 | possible | no detectable effect | 1.00x [0.97x-1.02x] | 180.175 | 179.597 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 1.00x [0.97x-1.02x] | 15.111 | 14.974 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 1.13x [0.92x-1.38x] | 0.570 | 0.519 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 1.01x [0.89x-1.15x] | 1.200 | 1.156 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 1.01x [0.88x-1.15x] | 7.725 | 7.246 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | no detectable effect | 1.00x [0.94x-1.06x] | 38.535 | 37.316 | 5 |
<!-- END GENERATED:SIMD_EFFECT_TABLE -->

---

## 6. Full Corpus Solver-Core Checkpoint Progression

<!-- BEGIN GENERATED:FULL_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_std_lp` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 54.00 | 54.00 |
| `t1_std_lp_max` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 80.00 | 80.00 |
| `t1_std_mip` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.01 (3) | 0.002 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.01 (3) | 0.002 | 64.95 | 58.90 |
| `t1_whiskas` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.003 | 0.52 | 0.52 |
| `t1_furniture` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.002 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.07** (1 / 8) | 0.10 (2 / 8) | 0.19 (24) | 0.24 (31) | 0.006 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.04** (1 / 0) | 0.07 (1 / 0) | 0.08 (1) | 0.91 (3) | 0.083 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **1.34** (10 / 0) | 0.82 (12 / 0) | 0.82 (12) | 1.75 (84) | 0.086 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **3.29** (27 / 9) | 2.77 (48 / 9) | 1.41 (77) | 1.31 (77) | 0.020 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **6.71** (1 / 4) | 13.97 (2 / 4) | 7.37 (132) | 6.84 (119) | 0.035 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.25** (1 / 2) | 1.34 (3 / 2) | 0.23 (7) | 0.22 (7) | 0.025 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.64** (1 / 20) | 2.03 (4 / 20) | 0.45 (9) | 0.45 (9) | 0.033 | 909.02 | 827.30 |
| `t2_gap_45` | **1.22** (1 / 9) | 0.45 (1 / 9) | 2.25 (61) | 2.22 (61) | 0.026 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **6.96** (5 / 37) | 4.20 (8 / 37) | 3.33 (198) | 3.68 (174) | 0.054 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **0.61** (2 / 7) | 1.39 (6 / 7) | 26.75 (1848) | 42.74 (2401) | 0.028 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.13** (1 / 6) | 0.42 (2 / 6) | 1.22 (9) | 1.14 (9) | 0.033 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.13** (1 / 0) | 0.84 (9 / 0) | 0.75 (9) | 0.38 (2) | 0.091 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.13** (1 / 0) | 0.12 (1 / 0) | 0.13 (1) | 3.07 (5) | 0.229 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.29** (1 / 0) | 5.50 (53 / 0) | 5.46 (53) | 6.46 (21) | 0.306 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **5.11** (54 / 0) | 4.87 (84 / 0) | 4.15 (84) | 21.78 (334) | 0.144 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **33.93** (250 / 0) | 20.20 (219 / 0) | 20.30 (219) | 206.87 (1370) | 0.262 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **55.36** (590 / 12) | 62.98 (701 / 12) | 192.67 (5000, `NodeLimit`) | 171.18 (5000, `NodeLimit`) | 0.119 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **18.62** (38 / 31) | 18.65 (38 / 31) | 11.04 (134) | 10.93 (134) | 0.152 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **180.18** (35 / 205) | 177.94 (420 / 205) | 250.03 (2267, `TimeLimit`) | 250.04 (1822, `TimeLimit`) | 0.825 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **15.11** (64 / 46) | 52.06 (206 / 46) | 250.03 (1879, `TimeLimit`) | 250.03 (2198, `TimeLimit`) | 0.194 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.57** (1 / 0) | 3.50 (14 / 0) | 4.05 (14) | 6.75 (32) | 0.907 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **1.20** (1 / 0) | 7.21 (12 / 0) | 7.59 (12) | 64.15 (130) | 1.464 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.73** (2 / 0) | 87.45 (107 / 0) | 86.50 (107) | 110.81 (124) | 15.042 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **38.53** (1 / 0) | 54.22 (2 / 0) | 49.56 (2) | 57.85 (0, `NumericalError`) | 111.432 | 208000.00 | 0.00 |
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
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.79x** [1.61x-2.00x] | 4.63x | 0.182 | 0.315 | 1.88x [0.30x-3.95x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **2.49x** [2.19x-2.84x] | 2.33x | 11.756 | 33.625 | 2.25x [0.97x-14.58x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.89x** [2.54x-3.29x] | 3.80x | 249.878 | 961.292 | 2.81x [1.58x-5.56x] |
| Square families ($m = n$) | 16 (146) | **2.61x** [2.37x-2.88x] | 3.56x | 256.869 | 984.013 | 2.39x [0.30x-14.58x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.61x** [1.49x-1.75x] | 2.35x | 4.947 | 11.220 | 1.53x [0.87x-3.73x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.44x** [2.04x-2.91x] | 2.79x | 35.252 | 66.485 | 2.00x [0.60x-14.58x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.30x** [2.12x-2.49x] | 3.18x | 261.816 | 995.233 | 2.15x [0.30x-14.58x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 7.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.83x** | 0.0004 | 0.0009 | 0.0005 | 0.0113 | $0.0007 \pm 0.0009$ | $0.0009 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.95x** | 0.0012 | 0.0024 | 0.0016 | 0.0100 | $0.0013 \pm 0.0006$ | $0.0024 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **2.03x** | 0.0027 | 0.0063 | 0.0042 | 0.0150 | $0.0033 \pm 0.0022$ | $0.0061 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.58x** | 0.0024 | 0.0038 | 0.0039 | 0.0139 | $0.0026 \pm 0.0010$ | $0.0039 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.43x** | 0.0029 | 0.0042 | 0.0047 | 0.0150 | $0.0032 \pm 0.0009$ | $0.0044 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **2.03x** | 0.0051 | 0.0111 | 0.0086 | 0.0324 | $0.0055 \pm 0.0019$ | $0.0109 \pm 0.0022$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.72x** | 0.0275 | 0.0709 | 0.0347 | 0.0964 | $0.0258 \pm 0.0066$ | $0.0687 \pm 0.0105$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.58x** | 0.0527 | 0.192 | 0.0703 | 0.259 | $0.0550 \pm 0.0131$ | $0.195 \pm 0.0295$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.38x** | 0.0478 | 0.117 | 0.0791 | 0.180 | $0.0495 \pm 0.0136$ | $0.116 \pm 0.0248$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.19x** | 0.192 | 0.221 | 0.207 | 0.267 | $0.202 \pm 0.0541$ | $0.237 \pm 0.0437$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **9.58x** | 0.0776 | 0.685 | 0.111 | 0.760 | $0.0792 \pm 0.0235$ | $0.740 \pm 0.0954$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.51x** | 0.0476 | 0.0694 | 0.101 | 0.143 | $0.0472 \pm 0.0038$ | $0.0713 \pm 0.0078$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **1.29x** | 0.0621 | 0.0798 | 0.113 | 0.156 | $0.0621 \pm 0.0052$ | $0.0799 \pm 0.0066$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **2.33x** | 0.125 | 0.282 | 0.178 | 0.358 | $0.122 \pm 0.0163$ | $0.284 \pm 0.0308$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.13x** | 0.377 | 1.276 | 0.615 | 1.752 | $0.426 \pm 0.152$ | $1.265 \pm 0.122$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.96x** | 1.182 | 5.185 | 1.571 | 8.084 | $1.378 \pm 0.395$ | $5.317 \pm 0.396$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.93x** | 0.752 | 1.491 | 1.352 | 4.275 | $0.774 \pm 0.0616$ | $1.492 \pm 0.0574$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.72x** | 4.947 | 8.444 | 5.185 | 9.447 | $5.530 \pm 1.968$ | $9.362 \pm 2.830$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.98x** | 0.308 | 0.835 | 0.679 | 3.907 | $0.305 \pm 0.0248$ | $0.915 \pm 0.159$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.27x** | 0.428 | 0.973 | 0.830 | 4.103 | $0.432 \pm 0.0198$ | $0.978 \pm 0.0296$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.59x** | 6.263 | 28.084 | 8.112 | 37.978 | $6.246 \pm 0.547$ | $28.630 \pm 2.234$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.13x** | 35.107 | 147.154 | 44.454 | 195.534 | $35.311 \pm 2.746$ | $145.565 \pm 8.628$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 7.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSlice64`, `InitReducedRow64`, and `Min2ReducedRow64`)

In [`assign/solver.go`](../assign/solver.go), [`simd.AllAbsLE64With`](../internal/simd/kernels_amd64.go) validates finite cost bounds across the input matrix via 8-lane AVX2 `Abs().LessEqual` checks when `len(cost) >= 256`, [`simd.MinReducedSlice64With`](../internal/simd/kernels_amd64.go) accelerates reduction-transfer slack scans in `preprocessSquareJV`, [`simd.Min2ReducedRow64With`](../internal/simd/kernels_amd64.go) accelerates the first/second reduced-minimum scans in `preprocessRectangularBV` and `augmentingRowReduction` via 8-way AVX2 `VSUBPD` and `VCMPPD`/`VPAND`/`VMOVMSKPD` bitmask filtering when $\max(m, n) \ge 64$, and [`simd.InitReducedRow64With`](../internal/simd/kernels_amd64.go) runs on a slice of length $\max(m, n)$ at the start of each Dijkstra augmentation when $\max(m, n) \ge 32$. All vector kernels clean upper `YMM` state via `StoreArray` + `ClearAVXUpperBits()` (`VZEROUPPER`) before scalar operations; exact per-family verdicts appear in the generated table below.

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32`, no AVX2 row-reduction kernel runs): 0 of 4 have a 95% interval excluding 1.
- **Instances where AVX2 row kernels run** (`max(rows, cols) >= 32`): 7 faster, 2 slower, 9 with no detectable effect, out of 18.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 row kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source | 1.03x [0.59x-1.81x] | 0.0004 | 0.0004 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source | 1.00x [0.62x-1.61x] | 0.0012 | 0.0011 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source | 0.97x [0.71x-1.33x] | 0.0027 | 0.0033 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | no detectable effect | 0.89x [0.69x-1.15x] | 0.0024 | 0.0028 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | no detectable effect | 1.06x [0.89x-1.26x] | 0.0029 | 0.0029 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | no (control) | identical source | 0.92x [0.68x-1.26x] | 0.0051 | 0.0059 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD slower | 0.90x [0.84x-0.97x] | 0.0275 | 0.0301 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | no detectable effect | 0.94x [0.85x-1.04x] | 0.0527 | 0.0591 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | no detectable effect | 1.00x [0.87x-1.16x] | 0.0478 | 0.0478 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | no detectable effect | 0.84x [0.59x-1.21x] | 0.192 | 0.197 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.16x [1.01x-1.32x] | 0.0776 | 0.0694 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.25x [1.01x-1.54x] | 0.0476 | 0.0352 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | no detectable effect | 1.02x [0.90x-1.15x] | 0.0621 | 0.0583 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD slower | 0.86x [0.82x-0.92x] | 0.125 | 0.142 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | no detectable effect | 1.10x [0.95x-1.28x] | 0.377 | 0.352 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.20x [1.02x-1.40x] | 1.182 | 1.061 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.20x [1.12x-1.29x] | 0.752 | 0.633 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.07x [1.05x-1.09x] | 4.947 | 4.693 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.88x [1.57x-2.25x] | 0.308 | 0.162 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | no detectable effect | 1.14x [1.00x-1.30x] | 0.428 | 0.387 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.13x [1.03x-1.24x] | 6.263 | 5.431 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | no detectable effect | 1.09x [0.98x-1.22x] | 35.107 | 31.939 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

---

## 8. Keeping Documentation and Benchmarks Synchronized

1. **Regenerate telemetry** from a clean, committed tree in a single pinned session so that all seven telemetry files record identical `git_commit`, `source_fingerprint`, and `uname_release`: run the commands in the [README](../README.md#verification-benchmarking-and-documentation-sync-commands).
2. **Regenerate tables**: `go run ./cmd/goblin-bench -sync-docs`.
3. **Verify**: `go run ./cmd/goblin-bench -check-docs`, or `go test ./...`, which runs `TestDocumentationAndBenchmarkSync`. The gate validates corpus dimensions, parity, symbol identifiers, and provenance, and checks every generated block in `README.md`, `docs/benchmarks.md`, and `bench/results/final_report.md`.
