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
- **Unified telemetry source**: fingerprint `61682e72894e7404` (commit `9262f513905304ba6067db9778bc29b1dfdd7842`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.32 ms median on a 1-variable MILP and 6.31 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

- **Single-threaded, sequential, pinned execution**: every solve runs single-threaded (`threads=1` in CBC, one goroutine in `gb.SolveCompiled`), all harnesses run with `workers=1` in a single session on the same Linux kernel, and each harness is pinned with `taskset` to one performance core. The i9-13900K mixes 5.5 GHz P-cores (CPUs 0-13) with 4.3 GHz E-cores (CPUs 14-29); unpinned runs migrate between them and introduce bimodal variance.
- **Warm state**: each harness runs an untimed worker warmup (`_init_worker` in [`bench/python_baseline.py`](../bench/python_baseline.py), `warmWorker` in [`cmd/goblin-bench/main.go`](../cmd/goblin-bench/main.go)), then 2 untimed warmup solves per family on Tiers 1-2 and 1 on Tier 3.
- **Synthetic corpus and held-out public benchmarks**: each synthetic family in `testdata/corpus/trials/<id>.jsonl` holds 11 distinct seeded problem instances (Tiers 1-2) or 5 (Tier 3), 293 instances in total; `WriteCorpusJSON` rejects duplicate trial payloads. Trial $k$ is the identical problem instance in every harness, enabling paired per-instance ratios rather than repeated runs on a fixed matrix. Complementing the synthetic timing corpus, [`public_benchmarks_test.go`](../public_benchmarks_test.go) verifies optimal status, objective parity, and primal feasibility across 20 held-out public Netlib LP and MIPLIB 3.0 MILP instances in `testdata/public/`, plus a valid dual lower bound under a 25-node budget with cuts and heuristics disabled (`WithCuts(false)`, `WithHeuristics(false)`, `WithMaxNodes(25)`) on `coinor_capplan1`.
- **Metrics**: per randomized instance, `PyPrep` sums JSON parse and PuLP model build (`t_parse + t_build`, while MPS formatting `t_format` is recorded separately and included inside `t_solve`), whereas `GoPrep` sums `FromPuLPJSONBytes` parse/build (`t_parse + t_build`, with `t_build = 0`) and `Model.Compile()` (`t_compile`). Aggregate tables report equal-family geometric means with family-clustered 95% Student-$t_{F-1}$ intervals alongside pooled trial geometric means, both multi-trial and family-median wall-clock sums, and full per-instance `[Min-Max]` ranges without floor clamping.
- **Parity**: the benchmark gate requires both `goblin` pure-Go builds and `goblin (CBC)` to match CBC's canonical and per-trial statuses, objectives, and root LP relaxation objectives across all 293 instances within a relative tolerance of `1e-5` under matching `1e-6` MIP gap tolerances.

---

## 2. Aggregate Workload Summary

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `Py / Go` GeoMean [Clustered 95% CI] | Pooled Trial `Py / Go` | Equal-Family `Go / CBC` | `Go` Trial (Med) Sum (ms) | `CBC` Trial (Med) Sum (ms) | `Py` Trial (Med) Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **333.55x** [159.25x-698.63x] | 333.55x | 0.05x | 9.0 (0.8) | 194.9 (19.3) | 1045.8 (99.1) | 508.83x [32.61x-1675.45x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **21.50x** [12.97x-35.65x] | 21.50x | 0.14x | 547.1 (31.3) | 1956.3 (170.9) | 4317.9 (417.3) | 27.19x [0.18x-164.03x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **10.90x** [5.19x-22.90x] | 10.90x | 0.19x | 846.5 (134.4) | 2918.1 (560.2) | 6189.1 (1066.7) | 10.87x [1.13x-68.92x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **14.08x** [8.32x-23.84x] | 15.42x | 0.18x | 1095.8 (122.9) | 3121.6 (393.9) | 6771.2 (987.5) | 15.53x [0.18x-164.03x] |
| Tier 3 non-MAD MILPs | 4 (20) | **8.62x** [1.51x-49.10x] | 8.62x | 0.25x | 557.0 (92.3) | 1252.0 (230.4) | 2811.3 (601.1) | 6.51x [1.13x-68.92x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **27.81x** [12.66x-61.09x] | 36.17x | 0.10x | 298.8 (43.0) | 1778.3 (339.7) | 3838.5 (506.0) | 43.20x [1.18x-129.24x] |
| Random-weight MAD (negative controls) | 3 (33) | **17.19x** [1.04x-283.03x] | 17.19x | 0.14x | 246.7 (19.3) | 1212.7 (116.6) | 2246.2 (248.7) | 15.56x [1.45x-112.59x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **51.14x** [22.61x-115.67x] | 61.63x | 0.11x | 1103.7 (123.5) | 3291.0 (410.7) | 7714.4 (1077.1) | 50.43x [0.18x-1675.45x] |
| All 31 synthetic corpus families | 31 (293) | **43.70x** [23.42x-81.55x] | 54.86x | 0.11x | 1402.6 (166.5) | 5069.3 (750.4) | 11552.9 (1583.1) | 45.89x [0.18x-1675.45x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

How to read the aggregate table:
1. **Tier 1 micro-models are dominated by subprocess cost.** On 2-to-26-variable models, Python is bounded by the CBC subprocess spawn and `.sol` parsing floor reported in the provenance block above, whereas pure-Go executes in microseconds in memory.
2. **The planted-gap MAD family is zero-gap by construction.** Its instances have `RootLPBound == OptimalObj` by design and close at the root without branching via presolve orbit fixing and root heuristics. The bold row (`Tier 2+3 non-planted MILPs`) excludes both Tier-1 subprocess micro-models and planted-gap MAD to isolate solver-dominated MILP performance.
3. **Heavy-tail dispersion is visible in wall-clock sums and `[Min-Max]` ranges.** Reporting `Go` Trial (Med) Sum, `CBC` Trial (Med) Sum, `Py` Trial (Med) Sum, and per-instance `[Min-Max]` alongside paired geometric means exposes outlier seed variance (for example, in the non-MAD checkpoint table below).
4. **Large degenerate LPs and Forrest-Tomlin updates.** Pure-Go factors the basis with a three-stage singleton and sparse Markowitz LU, `FTRANSparseHint`/`BTRANSparseHint`, `BTRANExact`, `presolveLPWithDeadline` LP presolve, dual cost perturbation, dual bound flipping (`selectDualBFRT`), conditional power-of-two Ruiz equilibration, and in-place mutated-$U$ Forrest-Tomlin updates; see [`solver-internals.md`](solver-internals.md#known-limits-of-the-pure-go-engine).

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
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **102.9x** [85.8x-129.2x] | 0.09 | 9.43 | 0.05 | 9.22 | 0.034 | 0.211 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **62.7x** [38.7x-112.6x] | 0.49 | 33.97 | 0.44 | 33.73 | 0.032 | 0.238 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **59.6x** [49.2x-83.5x] | 0.18 | 9.76 | 0.14 | 9.46 | 0.038 | 0.307 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **54.8x** [41.6x-62.8x] | 0.17 | 9.83 | 0.12 | 9.42 | 0.045 | 0.400 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **29.4x** [27.2x-33.8x] | 0.39 | 11.31 | 0.30 | 10.71 | 0.092 | 0.604 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **10.1x** [3.6x-30.6x] | 3.03 | 47.19 | 2.98 | 46.90 | 0.038 | 0.307 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **8.0x** [1.4x-30.0x] | 15.76 | 167.54 | 15.69 | 167.11 | 0.058 | 0.431 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **32.3x** [26.1x-39.0x] | 0.53 | 17.94 | 0.41 | 16.80 | 0.112 | 1.017 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **17.2x** [11.2x-23.1x] | 1.04 | 18.30 | 0.82 | 16.58 | 0.220 | 1.719 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **6.2x** [1.2x-15.1x] | 7.78 | 60.43 | 6.83 | 52.61 | 0.994 | 8.137 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **10.5x** [4.6x-25.5x] | 32.79 | 368.98 | 29.21 | 335.58 | 3.905 | 34.109 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Cross-Instance Latency Dispersion Across Randomized Instances (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.09 \pm 0.01$ ($\pm 0.01$) | $9.33 \pm 0.49$ ($\pm 0.33$) | $0.09 \pm 0.00$ ($\pm 0.00$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.51 \pm 0.22$ ($\pm 0.15$) | $30.39 \pm 9.49$ ($\pm 6.38$) | $0.46 \pm 0.20$ ($\pm 0.14$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.18 \pm 0.02$ ($\pm 0.01$) | $11.01 \pm 2.36$ ($\pm 1.59$) | $0.15 \pm 0.01$ ($\pm 0.01$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.18 \pm 0.03$ ($\pm 0.02$) | $9.87 \pm 0.16$ ($\pm 0.10$) | $0.21 \pm 0.04$ ($\pm 0.02$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.40 \pm 0.01$ ($\pm 0.01$) | $11.67 \pm 0.62$ ($\pm 0.41$) | $0.33 \pm 0.01$ ($\pm 0.01$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $3.95 \pm 2.90$ ($\pm 1.95$) | $35.42 \pm 17.10$ ($\pm 11.49$) | $3.98 \pm 3.03$ ($\pm 2.04$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $17.97 \pm 11.79$ ($\pm 7.92$) | $138.39 \pm 63.36$ ($\pm 42.56$) | $17.93 \pm 11.46$ ($\pm 7.70$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.54 \pm 0.05$ ($\pm 0.06$) | $17.65 \pm 2.40$ ($\pm 2.98$) | $0.57 \pm 0.09$ ($\pm 0.12$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.10 \pm 0.32$ ($\pm 0.40$) | $18.37 \pm 0.22$ ($\pm 0.27$) | $1.05 \pm 0.33$ ($\pm 0.41$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $15.69 \pm 19.67$ ($\pm 24.42$) | $62.54 \pm 3.90$ ($\pm 4.84$) | $15.96 \pm 19.61$ ($\pm 24.34$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $40.55 \pm 14.11$ ($\pm 17.52$) | $577.01 \pm 608.27$ ($\pm 755.15$) | $38.30 \pm 14.00$ ($\pm 17.38$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | **0.05** (1 / 0) | 0.05 (1 / 0) | 0.06 (1) | 0.56 (4) | 0.089 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.44** (2 / 0) | 0.56 (12 / 0) | 0.54 (12) | 0.77 (114) | 0.100 | 4601.00 | 4257.00 |
| `t2_mad_g2_i8` | **0.14** (1 / 0) | 0.74 (8 / 0) | 0.72 (8) | 0.77 (9) | 0.109 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.12** (1 / 0) | 0.13 (2 / 0) | 0.13 (2) | 2.79 (13) | 0.246 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.30** (1 / 0) | 3.18 (22 / 0) | 2.93 (22) | 5.22 (23) | 0.354 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **2.98** (43 / 0) | 2.85 (112 / 0) | 2.82 (112) | 3.59 (45) | 0.152 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **15.69** (288 / 0) | 29.10 (186 / 0) | 29.22 (186) | 91.56 (539) | 0.274 | 7720.00 | 6060.00 |
| `t3_mad_g5_i20` | **0.41** (1 / 0) | 2.92 (17 / 0) | 2.58 (17) | 0.53 (1) | 0.667 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.82** (1 / 0) | 8.25 (58 / 0) | 7.35 (58) | 13.24 (74) | 1.550 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **6.83** (2 / 0) | 21.19 (25 / 0) | 19.43 (25) | 250.25 (150, `TimeLimit`) | 2.643 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **29.21** (1 / 0) | 32.04 (2 / 0) | 30.74 (2) | 251.32 (6, `TimeLimit`) | 57.331 | 208000.00 | 208000.00 |
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
| `t1_std_lp` | 3 | 3 | **786.0x** [491.6x-1203.0x] | 0.01 | 6.28 | 0.00 | 6.22 | 0.004 | 0.051 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **668.8x** [287.0x-1088.2x] | 0.01 | 6.16 | 0.00 | 6.11 | 0.005 | 0.050 | `OK` |
| `t1_std_mip` | 3 | 3 | **690.1x** [465.2x-943.3x] | 0.01 | 7.32 | 0.00 | 7.27 | 0.005 | 0.054 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **741.5x** [556.9x-1122.4x] | 0.01 | 7.32 | 0.00 | 7.27 | 0.005 | 0.055 | `OK` |
| `t1_whiskas` | 6 | 5 | **485.4x** [374.2x-583.8x] | 0.01 | 6.24 | 0.01 | 6.17 | 0.007 | 0.072 | `OK` |
| `t1_furniture` | 2 | 3 | **237.3x** [77.5x-600.0x] | 0.02 | 7.43 | 0.02 | 7.38 | 0.005 | 0.048 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **90.1x** [32.6x-283.6x] | 0.11 | 8.80 | 0.10 | 8.72 | 0.013 | 0.084 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **945.4x** [456.2x-1675.5x] | 0.01 | 6.13 | 0.00 | 6.08 | 0.005 | 0.041 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **102.9x** [85.8x-129.2x] | 0.09 | 9.43 | 0.05 | 9.22 | 0.034 | 0.211 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **62.7x** [38.7x-112.6x] | 0.49 | 33.97 | 0.44 | 33.73 | 0.032 | 0.238 | `OK` |
| `t2_mkp_30` | 30 | 6 | **11.6x** [6.3x-16.7x] | 2.08 | 28.31 | 2.06 | 28.14 | 0.017 | 0.173 | `OK` |
| `t2_mkp_50` | 50 | 8 | **5.9x** [2.0x-31.9x] | 4.03 | 16.44 | 4.00 | 16.20 | 0.032 | 0.244 | `OK` |
| `t2_cflp_36` | 36 | 11 | **58.6x** [24.4x-164.0x] | 0.15 | 8.93 | 0.14 | 8.77 | 0.017 | 0.158 | `OK` |
| `t2_cflp_49` | 49 | 13 | **38.4x** [23.6x-103.4x] | 0.31 | 10.14 | 0.29 | 9.93 | 0.022 | 0.200 | `OK` |
| `t2_gap_45` | 45 | 13 | **20.4x** [3.3x-61.4x] | 0.81 | 14.64 | 0.78 | 14.43 | 0.024 | 0.207 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **13.1x** [6.2x-31.2x] | 3.67 | 68.71 | 3.60 | 68.45 | 0.032 | 0.249 | `OK` |
| `t2_binpack_44` | 44 | 14 | **12.1x** [0.2x-44.4x] | 0.55 | 13.58 | 0.52 | 13.38 | 0.021 | 0.196 | `OK` |
| `t2_setcover_50` | 50 | 20 | **54.4x** [7.2x-144.9x] | 0.18 | 10.93 | 0.13 | 10.63 | 0.031 | 0.299 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **59.6x** [49.2x-83.5x] | 0.18 | 9.76 | 0.14 | 9.46 | 0.038 | 0.307 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **54.8x** [41.6x-62.8x] | 0.17 | 9.83 | 0.12 | 9.42 | 0.045 | 0.400 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **29.4x** [27.2x-33.8x] | 0.39 | 11.31 | 0.30 | 10.71 | 0.092 | 0.604 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **10.1x** [3.6x-30.6x] | 3.03 | 47.19 | 2.98 | 46.90 | 0.038 | 0.307 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **8.0x** [1.4x-30.0x] | 15.76 | 167.54 | 15.69 | 167.11 | 0.058 | 0.431 | `OK` |
| `t3_mkp_200` | 200 | 8 | **5.5x** [4.1x-6.7x] | 17.47 | 115.94 | 17.39 | 115.34 | 0.082 | 0.602 | `OK` |
| `t3_cflp_300` | 300 | 34 | **34.9x** [8.5x-68.9x] | 5.93 | 248.79 | 5.78 | 247.88 | 0.099 | 0.914 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **2.7x** [1.1x-5.0x] | 61.96 | 156.47 | 61.76 | 154.61 | 0.207 | 1.845 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **10.8x** [5.2x-16.3x] | 6.91 | 79.88 | 6.60 | 76.97 | 0.308 | 2.765 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **32.3x** [26.1x-39.0x] | 0.53 | 17.94 | 0.41 | 16.80 | 0.112 | 1.017 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **17.2x** [11.2x-23.1x] | 1.04 | 18.30 | 0.82 | 16.58 | 0.220 | 1.719 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **6.2x** [1.2x-15.1x] | 7.78 | 60.43 | 6.83 | 52.61 | 0.994 | 8.137 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **10.5x** [4.6x-25.5x] | 32.79 | 368.98 | 29.21 | 335.58 | 3.905 | 34.109 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## 5. Default Build vs. `GOEXPERIMENT=simd`

In [`internal/simd/kernels_amd64.go`](../internal/simd/kernels_amd64.go) and [`internal/simd/kernels_avx512_amd64.go`](../internal/simd/kernels_avx512_amd64.go), runtime CPUID dispatch selects AVX-512, AVX2, or 4-way scalar loops from a single portable `CGO_ENABLED=0` binary (`GOAMD64=v3` FMA-safe). Each vector kernel spills accumulators via `StoreArray` and executes `archsimd.ClearAVXUpperBits()` (`VZEROUPPER`) before scalar tail or caller operations, eliminating AVX-to-SSE transition stalls while preserving IEEE-754 `NaN` propagation parity. AVX2 activates at `minAVX2Len = 32` (`Dot64`, `Axpy64`, `Scale64`, `AbsMax64`, `RowActivityBoundsFinite`, `SelectDualLeaveRow64With`, `MinReducedSkip64With`, `InitReducedRow64`, `AllAbsLE64With`, `ValidateAndNegate64With`), `rows >= 8 && cols >= 8` for `Transpose64With`, `minDualRatioSIMDChunks = 4` (256 cols for `ScanDualRatioCand64With`), and `minAVX2Min2Len = 256` (`Min2ReducedRow64`); on AVX-512 hosts, dense LSAP loops dispatch AVX-512 at `minAVX512Len = 64` and `minAVX512Min2Len = 128` while sparse LP kernels gate AVX-512 at `minAVX512LPLen = 2048` / `minLeaveRowAVX512 = 2048` / `minDualRatioAVX512Chunks = 128` (8,192 cols) to avoid Intel Emerald Rapids License-2 clock down-binning across surrounding scalar LU code.

At the full-solver level, each ratio below is the geometric mean of per-trial `scalar / SIMD` solve times over the same instances on the local AVX2 host (`i9-13900K`), with a 95% $t$-interval on the log ratios; "no detectable effect" means the interval contains 1. "Reachable" is a necessary condition: the AVX2 LP kernels require `n + m >= 32`. Below that size no vector kernel can run, so those small models serve as negative controls where intervals excluding 1 reflect binary code-layout or run-to-run variance rather than SIMD execution. Per-family paired speedups and 95% intervals for larger models appear in the generated table below.

### 5.1 Scope and Pending Target-Hardware Verification

All telemetry in this document is measured locally on the AVX2-only `13th Gen Intel(R) Core(TM) i9-13900K` host (`x86_64`, Linux). Portable `GOOS=linux GOARCH=arm64 CGO_ENABLED=0` and `GOAMD64=v3`/`v4` builds compile and pass local build checks, whereas fresh physical AVX-512 and `linux/arm64` runtime test and benchmark re-verification on the current tree remain pending target hardware access.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 1 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 6 faster, 3 slower, 14 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.89x [0.83x-0.96x] | 0.003 | 0.004 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 1.05x [0.92x-1.20x] | 0.003 | 0.003 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 0.98x [0.91x-1.06x] | 0.005 | 0.005 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source | 0.96x [0.90x-1.02x] | 0.004 | 0.004 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 1.00x [0.89x-1.13x] | 0.006 | 0.005 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 0.96x [0.86x-1.07x] | 0.017 | 0.018 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source | 0.89x [0.70x-1.12x] | 0.096 | 0.101 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 1.05x [0.68x-1.62x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 0.98x [0.91x-1.05x] | 0.052 | 0.054 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | SIMD faster | 1.08x [1.04x-1.13x] | 0.441 | 0.432 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 0.98x [0.90x-1.07x] | 2.058 | 2.032 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 1.05x [0.94x-1.16x] | 4.000 | 3.974 | 11 |
| `t2_cflp_36` | 47 | possible | no detectable effect | 0.98x [0.93x-1.02x] | 0.135 | 0.139 | 11 |
| `t2_cflp_49` | 62 | possible | SIMD slower | 0.91x [0.89x-0.93x] | 0.287 | 0.313 | 11 |
| `t2_gap_45` | 58 | possible | SIMD faster | 1.07x [1.01x-1.12x] | 0.783 | 0.789 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 1.03x [0.97x-1.09x] | 3.604 | 3.148 | 11 |
| `t2_binpack_44` | 58 | possible | SIMD slower | 0.89x [0.85x-0.93x] | 0.517 | 0.545 | 11 |
| `t2_setcover_50` | 70 | possible | SIMD slower | 0.85x [0.79x-0.91x] | 0.132 | 0.165 | 11 |
| `t2_mad_g2_i8` | 72 | possible | SIMD faster | 1.14x [1.05x-1.25x] | 0.136 | 0.117 | 11 |
| `t2_mad_g3_i9` | 108 | possible | no detectable effect | 0.91x [0.81x-1.02x] | 0.123 | 0.135 | 11 |
| `t2_mad_g4_i12` | 156 | possible | SIMD faster | 1.15x [1.11x-1.19x] | 0.305 | 0.263 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.01x [0.98x-1.04x] | 2.981 | 2.900 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 0.99x [0.98x-1.01x] | 15.692 | 15.774 | 11 |
| `t3_mkp_200` | 208 | possible | SIMD faster | 1.07x [1.05x-1.09x] | 17.387 | 16.307 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.04x [0.94x-1.16x] | 5.782 | 5.908 | 5 |
| `t3_lotsizing_504` | 840 | possible | no detectable effect | 1.03x [0.99x-1.07x] | 61.756 | 58.576 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 1.04x [0.90x-1.20x] | 6.599 | 6.050 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 0.98x [0.90x-1.07x] | 0.415 | 0.408 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 1.05x [0.94x-1.17x] | 0.825 | 0.780 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 0.97x [0.87x-1.07x] | 6.828 | 6.969 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | SIMD faster | 1.08x [1.05x-1.11x] | 29.212 | 26.617 | 5 |
<!-- END GENERATED:SIMD_EFFECT_TABLE -->

---

## 6. Full Corpus Solver-Core Checkpoint Progression

<!-- BEGIN GENERATED:FULL_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_std_lp` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 54.00 | 54.00 |
| `t1_std_lp_max` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.001 | 80.00 | 80.00 |
| `t1_std_mip` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.01 (3) | 0.002 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.01 (3) | 0.002 | 64.95 | 58.90 |
| `t1_whiskas` | **0.01** (1 / 0) | 0.00 (1 / 0) | 0.01 (1) | 0.00 (1) | 0.003 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.02 (1) | 0.01 (1) | 0.002 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.10** (1 / 8) | 0.12 (2 / 8) | 0.15 (18) | 0.23 (35) | 0.009 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.002 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.05** (1 / 0) | 0.05 (1 / 0) | 0.06 (1) | 0.56 (4) | 0.089 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.44** (2 / 0) | 0.56 (12 / 0) | 0.54 (12) | 0.77 (114) | 0.100 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.06** (5 / 9) | 2.89 (40 / 9) | 1.50 (51) | 1.36 (61) | 0.036 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **4.00** (1 / 4) | 11.68 (2 / 4) | 4.66 (118) | 4.14 (111) | 0.038 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.14** (1 / 2) | 0.21 (3 / 2) | 0.16 (7) | 0.17 (7) | 0.025 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.29** (1 / 10) | 0.55 (4 / 10) | 0.30 (9) | 0.28 (9) | 0.034 | 909.02 | 827.30 |
| `t2_gap_45` | **0.78** (1 / 9) | 0.37 (1 / 9) | 1.68 (41) | 2.05 (60) | 0.025 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **3.60** (2 / 37) | 6.31 (25 / 37) | 2.88 (162) | 3.22 (199) | 0.059 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **0.52** (2 / 8) | 0.98 (5 / 8) | 8.85 (772) | 26.05 (1953) | 0.027 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.13** (1 / 6) | 0.32 (2 / 6) | 0.65 (6) | 0.63 (6) | 0.031 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.14** (1 / 0) | 0.74 (8 / 0) | 0.72 (8) | 0.77 (9) | 0.109 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.12** (1 / 0) | 0.13 (2 / 0) | 0.13 (2) | 2.79 (13) | 0.246 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.30** (1 / 0) | 3.18 (22 / 0) | 2.93 (22) | 5.22 (23) | 0.354 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **2.98** (43 / 0) | 2.85 (112 / 0) | 2.82 (112) | 3.59 (45) | 0.152 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **15.69** (288 / 0) | 29.10 (186 / 0) | 29.22 (186) | 91.56 (539) | 0.274 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **17.39** (250 / 14) | 81.71 (1135 / 16) | 163.35 (5000, `NodeLimit`) | 148.83 (5000, `NodeLimit`) | 0.118 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **5.78** (1 / 28) | 16.93 (44 / 34) | 8.50 (116) | 8.64 (118) | 0.184 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **61.76** (2 / 206) | 78.12 (376 / 206) | 250.02 (3810, `TimeLimit`) | 250.04 (3925, `TimeLimit`) | 0.731 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **6.60** (31 / 55) | 19.31 (87 / 53) | 250.03 (2540, `TimeLimit`) | 250.05 (2587, `TimeLimit`) | 0.183 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.41** (1 / 0) | 2.92 (17 / 0) | 2.58 (17) | 0.53 (1) | 0.667 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.82** (1 / 0) | 8.25 (58 / 0) | 7.35 (58) | 13.24 (74) | 1.550 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **6.83** (2 / 0) | 21.19 (25 / 0) | 19.43 (25) | 250.25 (150, `TimeLimit`) | 2.643 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **29.21** (1 / 0) | 32.04 (2 / 0) | 30.74 (2) | 251.32 (6, `TimeLimit`) | 57.331 | 208000.00 | 208000.00 |
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
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.68x** [1.45x-1.95x] | 1.68x | 5.21x | 0.196 (0.0178) | 0.310 (0.0287) | 1.65x [0.84x-3.15x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **3.29x** [2.09x-5.19x] | 3.29x | 3.14x | 9.447 (0.736) | 32.770 (2.958) | 2.99x [1.05x-19.44x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **3.27x** [2.20x-4.88x] | 3.27x | 3.85x | 209.168 (41.734) | 948.956 (188.200) | 3.44x [1.51x-6.45x] |
| Square families ($m = n$) | 16 (146) | **2.96x** [2.18x-4.03x] | 2.88x | 4.00x | 214.957 (41.805) | 971.096 (189.222) | 2.42x [1.05x-19.44x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **2.22x** [1.55x-3.16x] | 2.07x | 3.46x | 3.855 (0.683) | 10.940 (1.964) | 2.39x [0.84x-3.63x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.45x** [1.20x-4.97x] | 2.55x | 2.94x | 33.955 (5.880) | 65.164 (10.968) | 1.91x [1.12x-19.44x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.74x** [2.16x-3.47x] | 2.63x | 3.85x | 218.812 (42.488) | 982.036 (191.187) | 2.41x [0.84x-19.44x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 7.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **2.05x** | 0.0004 | 0.0009 | 0.0005 | 0.0108 | $0.0004 \pm 0.0001$ | $0.0009 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.90x** | 0.0012 | 0.0023 | 0.0015 | 0.0102 | $0.0013 \pm 0.0003$ | $0.0023 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.68x** | 0.0036 | 0.0062 | 0.0054 | 0.0152 | $0.0036 \pm 0.0010$ | $0.0061 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.52x** | 0.0027 | 0.0040 | 0.0037 | 0.0147 | $0.0026 \pm 0.0006$ | $0.0039 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.41x** | 0.0030 | 0.0044 | 0.0042 | 0.0153 | $0.0032 \pm 0.0011$ | $0.0044 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.62x** | 0.0069 | 0.0109 | 0.0110 | 0.0325 | $0.0067 \pm 0.0017$ | $0.0106 \pm 0.0019$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.92x** | 0.0255 | 0.0703 | 0.0330 | 0.0969 | $0.0239 \pm 0.0051$ | $0.0689 \pm 0.0101$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.56x** | 0.0467 | 0.175 | 0.0638 | 0.226 | $0.0505 \pm 0.0136$ | $0.177 \pm 0.0267$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.22x** | 0.0410 | 0.0935 | 0.0630 | 0.142 | $0.0444 \pm 0.0120$ | $0.0968 \pm 0.0155$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.70x** | 0.134 | 0.217 | 0.149 | 0.264 | $0.142 \pm 0.0474$ | $0.234 \pm 0.0432$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **13.56x** | 0.0543 | 0.680 | 0.0712 | 0.755 | $0.0518 \pm 0.0110$ | $0.687 \pm 0.0213$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **2.49x** | 0.0281 | 0.0701 | 0.0539 | 0.142 | $0.0307 \pm 0.0117$ | $0.0734 \pm 0.0095$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.45x** | 0.0321 | 0.0851 | 0.0585 | 0.159 | $0.0356 \pm 0.0111$ | $0.0847 \pm 0.0074$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.79x** | 0.0754 | 0.282 | 0.103 | 0.358 | $0.0761 \pm 0.0155$ | $0.286 \pm 0.0323$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.66x** | 0.299 | 1.284 | 0.405 | 1.713 | $0.404 \pm 0.336$ | $1.271 \pm 0.123$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.53x** | 1.117 | 5.231 | 1.509 | 7.867 | $1.181 \pm 0.156$ | $5.332 \pm 0.392$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **2.22x** | 0.659 | 1.495 | 1.256 | 4.279 | $0.671 \pm 0.0275$ | $1.489 \pm 0.0529$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.70x** | 4.981 | 8.466 | 5.213 | 10.206 | $5.573 \pm 2.087$ | $9.267 \pm 2.705$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **3.29x** | 0.258 | 0.828 | 0.663 | 3.826 | $0.256 \pm 0.0167$ | $0.843 \pm 0.0556$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.76x** | 0.359 | 0.973 | 0.740 | 3.926 | $0.356 \pm 0.0301$ | $0.979 \pm 0.0339$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **5.10x** | 5.820 | 28.221 | 8.499 | 37.172 | $5.623 \pm 0.389$ | $28.697 \pm 2.281$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **5.11x** | 28.539 | 142.987 | 37.713 | 188.809 | $28.172 \pm 4.094$ | $143.185 \pm 11.272$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 7.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSkip64`, `InitReducedRow64`, and `Min2ReducedRow64`)

In [`assign/solver.go`](../assign/solver.go), [`simd.AllAbsLE64With`](../internal/simd/kernels_amd64.go) and [`simd.ValidateAndNegate64With`](../internal/simd/assign_amd64.go) validate finite cost bounds via AVX2 checks whenever `len(cost) >= 32` (while the fused `colReduceValidateSquare` fast path skips `AllAbsLE64With` on finite square `Minimize` matrices and dispatches `ColReduceValidateRows64With` only for $n \ge 32$), [`simd.Transpose64With`](../internal/simd/assign_amd64.go) handles tall transposes with $\text{rows} \ge 8$ and $\text{cols} \ge 8$, [`simd.MinReducedSkip64With`](../internal/simd/assign_amd64.go) accelerates reduction-transfer slack scans in `preprocessSquareJV` ($n \ge 32$), [`simd.Min2ReducedRow64With`](../internal/simd/kernels_amd64.go) accelerates first/second reduced-minimum scans in `preprocessRectangularBV` and `augmentingRowReduction` at `minAVX2Min2Len = 256` (`minAVX512Min2Len = 128`), and [`simd.InitReducedRow64With`](../internal/simd/kernels_amd64.go) runs on a slice of length $\max(m, n) \ge 32$ at the start of each Dijkstra augmentation. All vector kernels clean upper `YMM` state via `StoreArray` + `ClearAVXUpperBits()` (`VZEROUPPER`) before scalar operations; exact per-family verdicts appear in the generated table below.

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` with no `>= 8x8` tall transpose and either fused square `Minimize` non-sparse path or `rows * cols < 32`, so no AVX2 kernel runs): 1 of 3 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32`, tall `rows > cols` with `>= 8x8`, or fallback `AllAbsLE64`/`ValidateAndNegate64` at `rows * cols >= 32`): 19 faster, 0 slower, 0 with no detectable effect, out of 19.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source | 1.01x [0.97x-1.06x] | 0.0004 | 0.0004 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.96x [0.95x-0.98x] | 0.0012 | 0.0012 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source | 1.00x [0.98x-1.02x] | 0.0036 | 0.0035 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | SIMD faster | 1.06x [1.03x-1.09x] | 0.0027 | 0.0026 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | SIMD faster | 1.10x [1.07x-1.12x] | 0.0030 | 0.0028 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | SIMD faster | 1.02x [1.00x-1.03x] | 0.0069 | 0.0067 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.29x [1.24x-1.34x] | 0.0255 | 0.0194 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.28x [1.20x-1.37x] | 0.0467 | 0.0367 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.40x [1.30x-1.51x] | 0.0410 | 0.0288 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.75x [1.70x-1.80x] | 0.134 | 0.0769 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.34x [1.27x-1.41x] | 0.0543 | 0.0405 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.19x [1.10x-1.28x] | 0.0281 | 0.0231 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.26x [1.19x-1.33x] | 0.0321 | 0.0254 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.39x [1.31x-1.48x] | 0.0754 | 0.0499 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.76x [1.32x-2.35x] | 0.299 | 0.192 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.68x [1.57x-1.80x] | 1.117 | 0.695 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.99x [1.84x-2.15x] | 0.659 | 0.349 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.45x [2.09x-2.88x] | 4.981 | 1.860 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.82x [1.69x-1.96x] | 0.258 | 0.139 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.49x [1.44x-1.54x] | 0.359 | 0.247 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.63x [1.54x-1.72x] | 5.820 | 3.583 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.45x [1.39x-1.52x] | 28.539 | 19.327 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

---

## 8. Keeping Documentation and Benchmarks Synchronized

1. **Regenerate telemetry** from a clean, committed tree in a single pinned session so that all seven telemetry files record identical `git_commit`, `source_fingerprint`, and `uname_release`: run the commands in the [README](../README.md#verification-benchmarking-and-documentation-sync-commands).
2. **Regenerate tables**: `go run ./cmd/goblin-bench -sync-docs`.
3. **Verify**: `go run ./cmd/goblin-bench -check-docs`, or `go test ./...`, which runs `TestDocumentationAndBenchmarkSync`. The gate validates corpus dimensions, parity, symbol identifiers, and provenance, and checks every generated block in `README.md`, `docs/benchmarks.md`, and `bench/results/final_report.md`.
