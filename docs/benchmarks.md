# Benchmark Methodology and Corpus Results

This document specifies the experimental protocol, hardware configuration, benchmark problem formulations, and empirical results comparing pure-Go `gopt` (Scalar and `GOEXPERIMENT=simd`) against Python `PuLP` (`COIN-OR CBC 2.10.3` CLI subprocess) and [`goblin`](../../goblin) (in-memory C++ `COIN-OR CBC/CLP` via `cgo`). Every table in this document is synchronized automatically from the canonical JSON telemetry files (`bench/results/python_baseline.json`, `bench/results/goblin_cbc.json`, `bench/results/gopt_scalar.json`, and `bench/results/gopt_simd.json`) via [`bench/docgen.go`](../bench/docgen.go) and verified by [`doc_sync_test.go`](../doc_sync_test.go).

---

## 1. Hardware Environment and Multi-Trial Protocol

- **CPU**: `13th Gen Intel(R) Core(TM) i9-13900K` (`x86_64`, 30 logical CPUs exposed in container environment, AVX2 + FMA).
- **Go Toolchain**: `go1.27.1-X:nodwarf5 linux/amd64` tested in both pure-Go scalar mode and `GOEXPERIMENT=simd` mode (`simd/archsimd.Float64x4`).
- **In-Memory C++ Baseline (`goblin`)**: `go1.27.1-X:nodwarf5` + `g++ 16.2.1 -O3` linked in-process via `cgo` against `COIN-OR CBC 2.10 / CLP` (`OsiClpSolverInterface` and `CbcSolver`).
- **Python Subprocess Baseline**: Python 3.14 + `PuLP 4.2.0` + `COIN-OR CBC 2.10.3` CLI (`threads=1`), with `pulp_version`, `cbc_path`, and `cbc_version` recorded in every entry of `bench/results/python_baseline.json`.
- **Single-Threaded Uncontended Execution (`workers=1`)**: Each individual LP or MILP solve executes strictly single-threaded (`threads=1` in CBC and single-goroutine in `gopt.SolveCompiled`). All three benchmark harnesses (`python_baseline.py`, `gopt-bench`, and `goblin-bench`) execute sequentially with `workers=1` so that every timed trial runs in an uncontended CPU state without L3 cache contention or C++ `cgo` bridge mutex queueing.
- **Warm-State Initialization and Multi-Seed Randomized Ensembles**:
  - **Process and Worker Warmup**: Before any problem family is timed, each harness executes an untimed worker initialization pass (`_init_worker` in [`bench/python_baseline.py`](../bench/python_baseline.py) and `warmWorker` in [`cmd/gopt-bench/main.go`](../cmd/gopt-bench/main.go) / `cmd/goblin-bench/main.go`) that pre-loads modules, warms dynamic allocators, `sync.Pool` `Workspace` buffers, C++ `COIN-OR` code paths, and persistent `/dev/shm` or temporary directories.
  - **Per-Family and Per-Checkpoint Warmup**: Every problem family executes untimed warmup solves (`2` warmup runs on Tiers 1 and 2, `1` warmup run on Tier 3) before measured trials begin, and every solver-core checkpoint (`Root LP`, `CP4 Base`, `CP5 +Presolve`, `CP6 +Cuts`, `CP7 Full`) runs an untimed warmup solve prior to timing.
  - **Multi-Seed Randomized Trials (`293` Total Trials across `31` Families)**: Each parameterized problem family in `testdata/corpus/trials/<id>.jsonl` is evaluated across $K = 11$ independent random seeds (`trial = 0..10`) on Tiers 1 and 2 and $K = 5$ independent random seeds (`trial = 0..4`) on Tier 3. Every trial's optimal objective is verified across `PuLP + CBC`, `goblin`, `gopt` Scalar, and `gopt` SIMD (`TrialObjectives` parity check).
  - **Reported Metrics**: Each trial records preparation time (`t_parse + t_build + t_format/t_compile`), solver execution time (`t_solve`), and end-to-end latency (`t_total`), reporting the sample median alongside mean $\pm$ sample standard deviation (`SD`), two-sided 95% Student-$t$ confidence intervals (`[95% CI]`), and tier-level geometric mean and `Median [Min-Max]` spread.

---

## 2. Aggregate Workload Tier Summary and Architectural Crossover

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | PyTotal GeoMean (ms) | GoblinTotal GeoMean (ms) | GoptScalar GeoMean (ms) | GoptSIMD GeoMean (ms) | `Py / gopt SIMD` GeoMean | `Py / gopt SIMD` Median [Min-Max] | `gopt SIMD / goblin` |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical (`N <= 22`, 10 families) | 4.09 | 1.23 | 0.04 | 0.05 | **85.3x** | 144.8x [8.1x-264.7x] | **0.04x** |
| Tier 2: Target p90 (`30 <= N <= 76`, 13 families) | 16.49 | 25.63 | 2.18 | 1.72 | **9.6x** | 13.3x [1.5x-25.4x] | **0.07x** |
| Tier 3: Large Tail (`135 <= N <= 4028`, 8 families) | 113.12 | 133.67 | 55.29 | 54.33 | **2.1x** | 1.3x [0.5x-11.9x] | **0.41x** |
| Tier 3 Non-MAD MILPs (`200 <= N <= 1000`, 4 families) | 145.04 | 214.01 | 31.26 | 33.43 | **4.3x** | 6.2x [1.2x-11.9x] | **0.16x** |
| MAD `G x I` Family (`22 <= N <= 4028`, 11 families) | 29.78 | 32.79 | 10.80 | 8.58 | **3.5x** | 1.8x [0.5x-20.8x] | **0.26x** |
| Unstructured Random MAD (`22 <= N <= 48`, 3 families) | 43.26 | 73.38 | 21.15 | 15.61 | **2.8x** | 1.8x [1.5x-8.1x] | **0.21x** |
| **All 31 Corpus Families (293 Random Trials)** | 17.29 | 14.73 | 1.35 | 1.32 | **13.1x** | 13.3x [0.5x-264.7x] | **0.09x** |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### Disentangling Subprocess IPC Overhead from Solver Engine Speed
Comparing Python `PuLP + CBC`, in-memory C++ `goblin`, and pure-Go `gopt` separates serialization/IPC overhead from core algorithmic scaling:
1. **Subprocess IPC vs. In-Memory Execution (`Tier 1`, $N \le 22$, 10 families)**: Python `PuLP + CBC` incurs `2.20ms` to `3.87ms` per call on canonical small instances (`4.09ms` Tier 1 geometric mean) to format an `.mps` file, spawn the `cbc` binary, and parse the `.sol` output. Calling `COIN-OR CBC/CLP` in-memory via `goblin` eliminates this IPC tax (**3.3x** geometric mean speedup over Python, `1.23ms` vs. `4.09ms`), while pure-Go `gopt` (`GOEXPERIMENT=simd`) eliminates the C++ `CbcSolver` / `CglPreProcess` object-graph initialization overhead by reusing pooled `Workspace` buffers (**85.3x** faster than Python at `0.05ms` geometric mean, `144.8x [8.1x-264.7x]` median, and `gopt SIMD / goblin = 0.04x`).
2. **Medium and Large Combinatorial MILPs (`Tier 2` and `Tier 3 Non-MAD`)**: On `Tier 2` ($30 \le N \le 76$, 13 families) and the 4 non-MAD `Tier 3` combinatorial MILPs (`t3_mkp_200`, `t3_cflp_300`, `t3_lotsizing_504`, `t3_sched_1000`), `gopt` SIMD achieves a **9.6x** (`1.72ms` vs. `16.49ms`) and **4.3x** (`33.43ms` vs. `145.04ms`) geometric-mean speedup over Python `PuLP + CBC` (`gopt SIMD / goblin = 0.07x` and `0.16x` relative to in-memory `goblin` at `25.63ms` and `214.01ms`), driven by root `GMI`/cover/`VUB`/`c-MIR` cutting planes and compact fixed-integer LP heuristics.
3. **Simplex LU Scaling vs. C++ `COIN-OR CLP`**: `gopt`'s `LUFactorization` peels column and row singletons and runs dense partial-pivoting elimination ($O(n_{\text{nuc}}^3)$) on the remaining nucleus, whereas C++ `COIN-OR CLP` uses a full hyper-sparse Markowitz LU factorization with Forrest-Tomlin updates. On `t3_mad_g10_i100` (230 rows) and `t3_mad_g4_i1000` (1,052 rows), `goblin`'s root LP (`15.583ms` and `34.797ms`) is **8.6x to 11.5x faster** than `gopt`'s root LP (`133.811ms` and `400.346ms`), and `goblin` achieves lower total latency on both `t3_mad_g10_i100` (`168.24ms` vs. `417.25ms`, `gopt / goblin = 2.48x`) and `t3_mad_g4_i1000` (`446.42ms` vs. `679.14ms`, `gopt / goblin = 1.52x`), illustrating the asymptotic crossover where hyper-sparse Markowitz LU overtakes dense-nucleus elimination on larger bases.

---

## 3. MAD Group-Item Allocation Formulation (`G` Groups $\times$ `I` Items)

The Mean Absolute Deviation (`MAD`) allocation family ([`buildMADAllocation`](../bench/corpus.go)) models multi-period balanced allocation across $G$ groups, $I$ items, and $T = 6$ time periods.

### Decision Variables
- $x_{g,i} \in \{0, 1\}$ for $g \in \{0, \dots, G-1\}$ and $i \in \{0, \dots, I-1\}$: Binary indicator equal to $1$ if item $i$ is assigned to group $g$.
- $z_{g,t} \ge 0$ for $g \in \{0, \dots, G-1\}$ and $t \in \{0, \dots, T-1\}$: Continuous absolute deviation of group $g$'s aggregate weight in period $t$ from its central level $m_g$.
- $m_g \ge 0$ for $g \in \{0, \dots, G-1\}$: Continuous central weight level for group $g$.

Total variables: $N = G \cdot I + G \cdot T + G = G(I + T + 1)$.

### Objective and Constraints
$$\min \sum_{g=0}^{G-1} \sum_{t=0}^{T-1} z_{g,t}$$

subject to:
1. **Linearized Absolute Deviation** ($2 \cdot G \cdot T$ rows):
   $$z_{g,t} - \sum_{i=0}^{I-1} W_{t,i} x_{g,i} + m_g \ge 0 \quad \forall g, t$$
   $$z_{g,t} + \sum_{i=0}^{I-1} W_{t,i} x_{g,i} - m_g \ge 0 \quad \forall g, t$$
2. **Item Partitioning** ($I$ equality rows):
   $$\sum_{g=0}^{G-1} x_{g,i} = 1 \quad \forall i \in \{0, \dots, I-1\}$$
3. **Designated Subset Assignment** ($G$ equality rows over $D = \{I-G, \dots, I-1\}$):
   $$\sum_{i \in D} x_{g,i} = 1 \quad \forall g \in \{0, \dots, G-1\}$$

Total constraints: $M = 2 \cdot G \cdot T + I + G$.

### Structural Disclosure: Orthogonal Designated Signatures vs. Random-Weight Negative Controls
In [`buildMADAllocation`](../bench/corpus.go), only the $G$ designated items $i \in D = \{I-G, \dots, I-1\}$ carry nonzero period weights ($\pm 360$ in two periods per item), while the remaining $I - G$ items have $W_{t,i} = 0$ and appear only in the partitioning constraints $\sum_{g} x_{g,i} = 1$. Consequently:
- Every structured corpus instance `t*_mad_*` has a **zero integrality gap** (`RootLPBound == OptimalObj`). Although the fractional LP relaxation leaves the designated binary variables at $1/G$, `tryRootSubMIP` fixes the already-integral zero-weight items and solves the remaining $G \times G$ permutation sub-MIP to proven optimality at node 1 without branching.
- To evaluate solver behavior when this zero-weight/orthogonal structure is absent, [`BuildMADRandom`](../bench/corpus.go) (`t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, and `t2_mad_rand_g3_i9` in the corpus, plus `TestRandomMADNegativeControls` in [`solver_test.go`](../solver_test.go)) assigns dense pseudorandom positive weights $W_{t,i} \in [100, 2000)$ (`float64(100 + rng.IntN(1900))`) to all $I$ items across 11 random seeds per family. On these unstructured random-weight instances, the root LP relaxation exhibits a strict integrality gap (`RootLPBound < OptimalObj`), `tryRootSubMIP` does not close the root gap, and `gopt` executes multi-node Branch-and-Cut search (`Nodes > 1`) across the $G!$ group-permutation symmetries, achieving a **2.8x** geometric-mean speedup over Python `PuLP + CBC` (`15.61ms` vs. `43.26ms`, and `gopt SIMD / goblin = 0.21x` vs. `73.38ms`).

### MAD Benchmark Results (`G` Groups $\times$ `I` Items, $T = 6$)

<!-- BEGIN GENERATED:MAD_PIPELINE_TABLE -->
| Problem ID | $G$ | $I$ | $T$ | Vars | Rows | Optimal Obj | PyPrep (ms) | GoPrep (ms) | PySolve (ms) | GoSolve Scalar (ms) | GoSolve SIMD (ms) | PyTotal (ms) | GoTotal SIMD (ms) | Speedup (SIMD) | Parity |
| :--- | --: | ---: | --: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_mad_g2_i4` | 2 | 4 | 6 | 22 | 30 | 2160.0 | 0.733 | 0.144 | 5.11 | 0.12 | 0.18 | 5.60 | 0.32 | **17.4x** | `OK` |
| `t1_mad_rand_g2_i4` | 2 | 4 | 6 | 22 | 32 | 5249.0 | 0.704 | 0.102 | 31.70 | 4.54 | 3.82 | 32.02 | 3.95 | **8.1x** | `OK` |
| `t2_mad_g2_i8` | 2 | 8 | 6 | 30 | 34 | 2160.0 | 1.019 | 0.166 | 7.42 | 0.25 | 0.23 | 8.09 | 0.39 | **20.8x** | `OK` |
| `t2_mad_g3_i9` | 3 | 9 | 6 | 48 | 48 | 4320.0 | 1.403 | 0.172 | 6.14 | 0.69 | 0.27 | 7.09 | 0.45 | **15.9x** | `OK` |
| `t2_mad_g4_i12` | 4 | 12 | 6 | 76 | 64 | 7200.0 | 1.901 | 0.274 | 8.86 | 2.26 | 1.15 | 10.39 | 1.43 | **7.2x** | `OK` |
| `t2_mad_rand_g2_i8` | 2 | 8 | 6 | 30 | 36 | 6696.0 | 1.012 | 0.134 | 12.25 | 11.08 | 7.24 | 12.92 | 7.36 | **1.8x** | `OK` |
| `t2_mad_rand_g3_i9` | 3 | 9 | 6 | 48 | 51 | 7958.0 | 1.413 | 0.243 | 194.77 | 176.43 | 130.31 | 195.77 | 130.70 | **1.5x** | `OK` |
| `t3_mad_g5_i20` | 5 | 20 | 6 | 135 | 85 | 10800.0 | 3.556 | 0.988 | 12.77 | 10.87 | 10.74 | 15.39 | 12.26 | **1.3x** | `OK` |
| `t3_mad_g6_i30` | 6 | 30 | 6 | 222 | 108 | 15120.0 | 6.451 | 1.240 | 20.50 | 18.12 | 16.41 | 23.01 | 17.50 | **1.3x** | `OK` |
| `t3_mad_g10_i100` | 10 | 100 | 6 | 1070 | 230 | 79200.0 | 27.655 | 6.492 | 172.11 | 494.18 | 410.36 | 189.47 | 417.25 | **0.5x** | `OK` |
| `t3_mad_g4_i1000` | 4 | 1000 | 6 | 4028 | 1052 | 187200.0 | 116.841 | 24.004 | 840.98 | 806.07 | 650.89 | 903.03 | 679.14 | **1.3x** | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Multi-Trial Latency Dispersion (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `gopt` Scalar Total (Mean $\pm$ SD [95% CI], ms) | `gopt` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | $5.94 \pm 1.53$ ($\pm 1.03$) | $0.22 \pm 0.05$ ($\pm 0.03$) | $0.34 \pm 0.06$ ($\pm 0.04$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | $28.46 \pm 14.95$ ($\pm 10.05$) | $4.78 \pm 3.01$ ($\pm 2.02$) | $3.62 \pm 2.46$ ($\pm 1.65$) |
| `t2_mad_g2_i8` | $2 \times 8$ | $8.01 \pm 0.77$ ($\pm 0.52$) | $0.73 \pm 0.92$ ($\pm 0.62$) | $0.41 \pm 0.05$ ($\pm 0.03$) |
| `t2_mad_g3_i9` | $3 \times 9$ | $6.94 \pm 0.90$ ($\pm 0.60$) | $1.09 \pm 0.30$ ($\pm 0.20$) | $0.49 \pm 0.08$ ($\pm 0.05$) |
| `t2_mad_g4_i12` | $4 \times 12$ | $10.70 \pm 2.51$ ($\pm 1.68$) | $2.69 \pm 0.65$ ($\pm 0.44$) | $1.38 \pm 0.38$ ($\pm 0.25$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | $33.39 \pm 27.78$ ($\pm 18.66$) | $8.65 \pm 7.75$ ($\pm 5.20$) | $4.82 \pm 4.29$ ($\pm 2.88$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | $196.90 \pm 55.63$ ($\pm 37.37$) | $177.50 \pm 35.41$ ($\pm 23.78$) | $142.13 \pm 33.37$ ($\pm 22.41$) |
| `t3_mad_g5_i20` | $5 \times 20$ | $14.05 \pm 2.90$ ($\pm 3.60$) | $9.16 \pm 4.90$ ($\pm 6.08$) | $9.04 \pm 4.62$ ($\pm 5.73$) |
| `t3_mad_g6_i30` | $6 \times 30$ | $25.37 \pm 6.28$ ($\pm 7.80$) | $17.90 \pm 3.97$ ($\pm 4.93$) | $16.65 \pm 4.06$ ($\pm 5.05$) |
| `t3_mad_g10_i100` | $10 \times 100$ | $204.03 \pm 75.44$ ($\pm 93.66$) | $919.13 \pm 987.97$ ($\pm 1226.53$) | $833.47 \pm 954.80$ ($\pm 1185.35$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | $866.48 \pm 190.57$ ($\pm 236.59$) | $656.10 \pm 301.70$ ($\pm 374.55$) | $692.62 \pm 249.88$ ($\pm 310.21$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression (`GOEXPERIMENT=simd`)

Checkpoints `CP4 Base` (pure Branch-and-Bound without presolve, cuts, or heuristics) and `CP5 +Presolve` run under a `250ms` per-checkpoint time limit and `5,000`-node cap (`bench/corpus.go`). Checkpoints that hit a resource limit before proving optimality are annotated with `[TimeLimit]` or `[NodeLimit]`.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | Root LP Obj | Optimal Obj | Root LP (ms) | CP4 Base (ms / nodes) | CP5 +Presolve (ms / nodes) | CP6 +Cuts (ms / nodes / cuts) | CP7 Full (ms / nodes / cuts) |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | 2160.00 | 2160.00 | 0.108 | 0.38 (4) | 0.44 (4) | 0.22 (3 / 2) | **0.18** (1 / 0) |
| `t1_mad_rand_g2_i4` | 4257.00 | 5249.00 | 0.090 | 0.50 (9) | 0.58 (9) | 1.01 (7 / 27) | **3.82** (7 / 27) |
| `t2_mad_g2_i8` | 2160.00 | 2160.00 | 0.124 | 0.51 (8) | 0.43 (8) | 0.49 (10 / 3) | **0.23** (1 / 0) |
| `t2_mad_g3_i9` | 4320.00 | 4320.00 | 0.192 | 1.13 (10) | 1.05 (6) | 1.50 (11 / 3) | **0.27** (1 / 0) |
| `t2_mad_g4_i12` | 7200.00 | 7200.00 | 0.900 | 3.20 (16) | 2.83 (13) | 3.06 (30 / 10) | **1.15** (1 / 0) |
| `t2_mad_rand_g2_i8` | 5894.00 | 6696.00 | 0.105 | 1.50 (43) | 1.25 (39) | 1.29 (41 / 6) | **7.24** (41 / 6) |
| `t2_mad_rand_g3_i9` | 6060.00 | 7958.00 | 0.433 | 78.17 (183) | 52.88 (235) | 68.45 (383 / 18) | **130.31** (293 / 18) |
| `t3_mad_g5_i20` | 10800.00 | 10800.00 | 6.976 | 20.03 (54) | 11.98 (36) | 22.85 (76 / 6) | **10.74** (1 / 0) |
| `t3_mad_g6_i30` | 15120.00 | 15120.00 | 7.769 | 55.69 (90) | 47.91 (102) | 50.46 (70 / 20) | **16.41** (1 / 0) |
| `t3_mad_g10_i100` | 79200.00 | 79200.00 | 133.811 | 250.66 (4, `TimeLimit`) | 250.32 (29, `TimeLimit`) | 250.90 (36 / 20, `TimeLimit`) | **410.36** (1 / 0) |
| `t3_mad_g4_i1000` | 187200.00 | 187200.00 | 400.346 | 250.04 (0, `TimeLimit`) | 250.22 (0, `TimeLimit`) | 252.53 (0 / 0, `TimeLimit`) | **650.89** (1 / 0) |
<!-- END GENERATED:MAD_CHECKPOINT_TABLE -->

---

## 4. Full 31-Family Corpus End-to-End Comparison

The complete benchmark corpus spans 31 problem families (293 randomized trials across `testdata/corpus/*.json` and `testdata/corpus/trials/*.jsonl`):
- **Tier 1 (`tier1_unit`, 10 families)**: Canonical `PuLP` unit tests (`t1_std_lp`, `t1_std_lp_max`, `t1_std_mip`, `t1_mip_float_obj`, `t1_whiskas`, `t1_furniture`, `t1_knapsack_12`, `t1_infeasible_mip`), structured `t1_mad_g2_i4`, and unstructured random-weight `t1_mad_rand_g2_i4`.
- **Tier 2 (`tier2_p90`, 13 families)**: Medium-scale combinatorial MILPs (`30` to `76` variables, 11 random seeds each) covering Multidimensional Knapsack (`mkp`), Capacitated Facility Location (`cflp`), Generalized Assignment (`gap`), Multi-Item Lot-Sizing (`lotsizing`), Bin Packing (`binpack`), Set Cover (`setcover`), structured MAD allocation (`t2_mad_g2_i8`, `t2_mad_g3_i9`, `t2_mad_g4_i12`), and unstructured random-weight MAD (`t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`).
- **Tier 3 (`tier3_large`, 8 families)**: Large-scale MILPs (`135` to `4,028` variables and up to `1,052` constraints, 5 random seeds each), comprising 4 classic combinatorial MILPs (`t3_mkp_200`, `t3_cflp_300`, `t3_lotsizing_504`, `t3_sched_1000`) and 4 MAD allocation families (`t3_mad_g5_i20`, `t3_mad_g6_i30`, `t3_mad_g10_i100`, `t3_mad_g4_i1000`).

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | PyPrep (ms) | GoPrep (ms) | PySolve (ms) | GoSolve Scalar (ms) | GoSolve SIMD (ms) | PyTotal (ms) | GoTotal SIMD (ms) | Speedup (SIMD) | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | 0.181 | 0.011 | 2.35 | 0.00 | 0.00 | 2.46 | 0.01 | **188.1x** | `OK` |
| `t1_std_lp_max` | 3 | 3 | 0.175 | 0.009 | 2.41 | 0.00 | 0.00 | 2.51 | 0.01 | **221.9x** | `OK` |
| `t1_std_mip` | 3 | 3 | 0.155 | 0.009 | 2.95 | 0.00 | 0.00 | 3.04 | 0.01 | **252.4x** | `OK` |
| `t1_mip_float_obj` | 3 | 3 | 0.161 | 0.009 | 3.09 | 0.00 | 0.00 | 3.18 | 0.01 | **264.7x** | `OK` |
| `t1_whiskas` | 6 | 5 | 0.223 | 0.020 | 2.43 | 0.00 | 0.00 | 2.57 | 0.03 | **101.6x** | `OK` |
| `t1_furniture` | 2 | 3 | 0.158 | 0.018 | 3.87 | 0.01 | 0.02 | 3.97 | 0.05 | **87.0x** | `OK` |
| `t1_knapsack_12` | 12 | 3 | 0.262 | 0.047 | 5.07 | 0.08 | 0.12 | 5.24 | 0.18 | **29.9x** | `OK` |
| `t1_infeasible_mip` | 2 | 1 | 0.126 | 0.012 | 2.20 | 0.00 | 0.00 | 2.27 | 0.01 | **195.2x** | `OK` |
| `t1_mad_g2_i4` | 22 | 30 | 0.733 | 0.144 | 5.11 | 0.12 | 0.18 | 5.60 | 0.32 | **17.4x** | `OK` |
| `t1_mad_rand_g2_i4` | 22 | 32 | 0.704 | 0.102 | 31.70 | 4.54 | 3.82 | 32.02 | 3.95 | **8.1x** | `OK` |
| `t2_mkp_30` | 30 | 6 | 0.556 | 0.107 | 44.98 | 5.05 | 3.27 | 45.35 | 3.40 | **13.3x** | `OK` |
| `t2_mkp_50` | 50 | 8 | 0.823 | 0.118 | 15.41 | 8.05 | 5.82 | 16.01 | 5.92 | **2.7x** | `OK` |
| `t2_cflp_36` | 36 | 11 | 0.568 | 0.090 | 6.59 | 0.13 | 0.20 | 6.95 | 0.29 | **23.7x** | `OK` |
| `t2_cflp_49` | 49 | 13 | 0.700 | 0.124 | 6.70 | 0.59 | 0.54 | 7.16 | 0.66 | **10.8x** | `OK` |
| `t2_gap_45` | 45 | 13 | 0.657 | 0.121 | 12.75 | 0.91 | 0.86 | 13.23 | 0.98 | **13.5x** | `OK` |
| `t2_lotsizing_48` | 48 | 32 | 0.822 | 0.105 | 71.53 | 10.05 | 5.94 | 72.10 | 6.03 | **12.0x** | `OK` |
| `t2_binpack_44` | 44 | 14 | 0.648 | 0.102 | 12.37 | 0.31 | 0.41 | 12.77 | 0.50 | **25.3x** | `OK` |
| `t2_setcover_50` | 50 | 20 | 1.161 | 0.195 | 9.36 | 0.18 | 0.17 | 10.09 | 0.40 | **25.4x** | `OK` |
| `t2_mad_g2_i8` | 30 | 34 | 1.019 | 0.166 | 7.42 | 0.25 | 0.23 | 8.09 | 0.39 | **20.8x** | `OK` |
| `t2_mad_g3_i9` | 48 | 48 | 1.403 | 0.172 | 6.14 | 0.69 | 0.27 | 7.09 | 0.45 | **15.9x** | `OK` |
| `t2_mad_g4_i12` | 76 | 64 | 1.901 | 0.274 | 8.86 | 2.26 | 1.15 | 10.39 | 1.43 | **7.2x** | `OK` |
| `t2_mad_rand_g2_i8` | 30 | 36 | 1.012 | 0.134 | 12.25 | 11.08 | 7.24 | 12.92 | 7.36 | **1.8x** | `OK` |
| `t2_mad_rand_g3_i9` | 48 | 51 | 1.413 | 0.243 | 194.77 | 176.43 | 130.31 | 195.77 | 130.70 | **1.5x** | `OK` |
| `t3_mkp_200` | 200 | 8 | 2.387 | 0.502 | 62.74 | 52.93 | 54.00 | 63.72 | 54.52 | **1.2x** | `OK` |
| `t3_cflp_300` | 300 | 34 | 3.486 | 0.490 | 356.28 | 23.50 | 29.66 | 358.45 | 30.13 | **11.9x** | `OK` |
| `t3_lotsizing_504` | 504 | 336 | 7.283 | 1.451 | 177.33 | 70.80 | 68.84 | 182.84 | 70.27 | **2.6x** | `OK` |
| `t3_sched_1000` | 1000 | 100 | 10.245 | 1.978 | 99.20 | 8.36 | 9.38 | 105.98 | 10.82 | **9.8x** | `OK` |
| `t3_mad_g5_i20` | 135 | 85 | 3.556 | 0.988 | 12.77 | 10.87 | 10.74 | 15.39 | 12.26 | **1.3x** | `OK` |
| `t3_mad_g6_i30` | 222 | 108 | 6.451 | 1.240 | 20.50 | 18.12 | 16.41 | 23.01 | 17.50 | **1.3x** | `OK` |
| `t3_mad_g10_i100` | 1070 | 230 | 27.655 | 6.492 | 172.11 | 494.18 | 410.36 | 189.47 | 417.25 | **0.5x** | `OK` |
| `t3_mad_g4_i1000` | 4028 | 1052 | 116.841 | 24.004 | 840.98 | 806.07 | 650.89 | 903.03 | 679.14 | **1.3x** | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## 5. Full 31-Family Solver-Core Checkpoint Progression (`GOEXPERIMENT=simd`)

<!-- BEGIN GENERATED:FULL_CHECKPOINT_TABLE -->
| Problem ID | Root LP Obj | Optimal Obj | Root LP (ms) | CP4 Base (ms / nodes) | CP5 +Presolve (ms / nodes) | CP6 +Cuts (ms / nodes / cuts) | CP7 Full (ms / nodes / cuts) |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 54.00 | 54.00 | 0.001 | 0.00 (1) | 0.00 (1) | 0.00 (1 / 0) | **0.00** (1 / 0) |
| `t1_std_lp_max` | 80.00 | 80.00 | 0.001 | 0.00 (1) | 0.00 (1) | 0.00 (1 / 0) | **0.00** (1 / 0) |
| `t1_std_mip` | 58.00 | 64.00 | 0.001 | 0.00 (3) | 0.00 (1) | 0.00 (1 / 0) | **0.00** (1 / 0) |
| `t1_mip_float_obj` | 58.90 | 64.95 | 0.001 | 0.00 (3) | 0.00 (1) | 0.00 (1 / 0) | **0.00** (1 / 0) |
| `t1_whiskas` | 0.52 | 0.52 | 0.003 | 0.00 (1) | 0.00 (1) | 0.00 (1 / 0) | **0.00** (1 / 0) |
| `t1_furniture` | 1075.00 | 1075.00 | 0.002 | 0.01 (1) | 0.02 (1) | 0.02 (1 / 0) | **0.02** (1 / 0) |
| `t1_knapsack_12` | 578.65 | 513.24 | 0.008 | 0.18 (31) | 0.14 (24) | 0.10 (2 / 15) | **0.12** (1 / 15) |
| `t1_infeasible_mip` | 1.50 | 0.00 | 0.001 | 0.01 (5) | 0.00 (0) | 0.00 (0 / 0) | **0.00** (0 / 0) |
| `t1_mad_g2_i4` | 2160.00 | 2160.00 | 0.108 | 0.38 (4) | 0.44 (4) | 0.22 (3 / 2) | **0.18** (1 / 0) |
| `t1_mad_rand_g2_i4` | 4257.00 | 5249.00 | 0.090 | 0.50 (9) | 0.58 (9) | 1.01 (7 / 27) | **3.82** (7 / 27) |
| `t2_mkp_30` | 1966.92 | 1917.24 | 0.043 | 1.21 (77) | 1.33 (77) | 2.88 (66 / 9) | **3.27** (29 / 9) |
| `t2_mkp_50` | 2648.37 | 2620.01 | 0.104 | 3.63 (133) | 4.11 (146) | 3.91 (2 / 5) | **5.82** (1 / 5) |
| `t2_cflp_36` | 781.97 | 801.17 | 0.022 | 0.18 (7) | 0.14 (7) | 1.17 (3 / 2) | **0.20** (1 / 2) |
| `t2_cflp_49` | 827.30 | 909.02 | 0.044 | 0.41 (9) | 0.41 (9) | 2.19 (4 / 20) | **0.54** (1 / 20) |
| `t2_gap_45` | 178.04 | 195.18 | 0.023 | 1.36 (61) | 2.28 (61) | 0.35 (1 / 13) | **0.86** (1 / 13) |
| `t2_lotsizing_48` | 2052.34 | 2245.81 | 0.062 | 2.18 (178) | 2.33 (177) | 10.46 (16 / 42) | **5.94** (15 / 42) |
| `t2_binpack_44` | 29.30 | 30.32 | 0.047 | 28.49 (2442) | 26.46 (1935) | 1.36 (21 / 12) | **0.41** (1 / 12) |
| `t2_setcover_50` | 56.84 | 58.10 | 0.038 | 1.02 (9) | 1.02 (9) | 0.32 (2 / 6) | **0.17** (1 / 6) |
| `t2_mad_g2_i8` | 2160.00 | 2160.00 | 0.124 | 0.51 (8) | 0.43 (8) | 0.49 (10 / 3) | **0.23** (1 / 0) |
| `t2_mad_g3_i9` | 4320.00 | 4320.00 | 0.192 | 1.13 (10) | 1.05 (6) | 1.50 (11 / 3) | **0.27** (1 / 0) |
| `t2_mad_g4_i12` | 7200.00 | 7200.00 | 0.900 | 3.20 (16) | 2.83 (13) | 3.06 (30 / 10) | **1.15** (1 / 0) |
| `t2_mad_rand_g2_i8` | 5894.00 | 6696.00 | 0.105 | 1.50 (43) | 1.25 (39) | 1.29 (41 / 6) | **7.24** (41 / 6) |
| `t2_mad_rand_g3_i9` | 6060.00 | 7958.00 | 0.433 | 78.17 (183) | 52.88 (235) | 68.45 (383 / 18) | **130.31** (293 / 18) |
| `t3_mkp_200` | 12955.46 | 12906.80 | 0.412 | 131.40 (5000, `NodeLimit`) | 150.55 (5000, `NodeLimit`) | 109.74 (1031 / 21) | **54.00** (566 / 21) |
| `t3_cflp_300` | 2081.53 | 2187.30 | 0.159 | 4.94 (112) | 4.90 (112) | 23.90 (60 / 39) | **29.66** (23 / 39) |
| `t3_lotsizing_504` | 10518.99 | 13120.44 | 3.147 | 250.18 (3213, `TimeLimit`) | 250.05 (3837, `TimeLimit`) | 238.20 (381 / 165) | **68.84** (11 / 165) |
| `t3_sched_1000` | 46971.09 | 46818.28 | 2.644 | 250.39 (3774, `TimeLimit`) | 250.04 (3469, `TimeLimit`) | 26.89 (96 / 50) | **9.38** (17 / 50) |
| `t3_mad_g5_i20` | 10800.00 | 10800.00 | 6.976 | 20.03 (54) | 11.98 (36) | 22.85 (76 / 6) | **10.74** (1 / 0) |
| `t3_mad_g6_i30` | 15120.00 | 15120.00 | 7.769 | 55.69 (90) | 47.91 (102) | 50.46 (70 / 20) | **16.41** (1 / 0) |
| `t3_mad_g10_i100` | 79200.00 | 79200.00 | 133.811 | 250.66 (4, `TimeLimit`) | 250.32 (29, `TimeLimit`) | 250.90 (36 / 20, `TimeLimit`) | **410.36** (1 / 0) |
| `t3_mad_g4_i1000` | 187200.00 | 187200.00 | 400.346 | 250.04 (0, `TimeLimit`) | 250.22 (0, `TimeLimit`) | 252.53 (0 / 0, `TimeLimit`) | **650.89** (1 / 0) |
<!-- END GENERATED:FULL_CHECKPOINT_TABLE -->

---

## 6. Keeping Documentation and Benchmarks Synchronized

To prevent divergence between code, benchmark JSON files, and markdown tables:
1. **Regenerate Tables**: Run `GOEXPERIMENT=simd go run ./cmd/gopt-bench -sync-docs` after updating any benchmark JSON file in `bench/results/`.
2. **Verify Sync via CLI**: Run `go run ./cmd/gopt-bench -check-docs` to assert zero diff across `README.md`, `docs/benchmarks.md`, and `bench/results/final_report.md`.
3. **Automated Test Gate**: Running `go test ./...` executes `TestDocumentationAndBenchmarkSync` in [`doc_sync_test.go`](../doc_sync_test.go), which validates corpus dimensions, verifies byte-for-byte parity with `../goblin/bench/results/*.json` when present, and checks that every generated block in `README.md`, `docs/benchmarks.md`, and `bench/results/final_report.md` matches the canonical JSON telemetry character-for-character.
