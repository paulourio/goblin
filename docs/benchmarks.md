# Benchmark Methodology and Corpus Results

The benchmark protocol below evaluates pure-Go `goblin` (`BackendGo`, zero `cgo` by default), Python `PuLP` driving the `COIN-OR CBC` (`c1c171ec`) command-line binary, and `goblin (CBC)` (`BackendCBC`, built with `-tags cbc`), which calls the same `c1c171ec` CBC/CLP stack in-process through `cgo`. Every timing and binary-size measurement in this document comes from a generated block rendered by [`bench/docgen.go`](../bench/docgen.go) from the JSON telemetry in `bench/results/`; the prose contains no hardcoded timings. The test [`doc_sync_test.go`](../doc_sync_test.go) fails `go test ./...` when a block drifts from the telemetry, when a parity check fails, or when any of the eight telemetry files lacks uniform git, source-fingerprint, and host-kernel provenance.

---

## 1. Environment, Provenance, and Protocol

```bash
go run ./cmd/goblin-bench -check-docs
```

- **CPU**: `13th Gen Intel(R) Core(TM) i9-13900K` (`x86_64`, 30 logical CPUs exposed by `lscpu`, AVX2 + FMA).
- **Default build**: `go build` with the pure-Go kernels (`BackendGo`). All headline timings use that build.
- **Experimental build**: `GOEXPERIMENT=simd`, compared to the default build only in section 5.

<!-- BEGIN GENERATED:PROVENANCE -->
- **Unified telemetry source**: fingerprint `b47f24a9055c678c` (commit `88ea00e84d8db9a8821a12f65e2acbdbb78e73ad`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.74 ms median on a 1-variable MILP and 6.34 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

- **Single-threaded, sequential, pinned execution**: every solve runs single-threaded (`threads=1` in CBC, one goroutine in `gb.SolveCompiled`), all harnesses run with `workers=1` in a single session on the same Linux kernel, and each harness is pinned with `taskset` to one performance core. The i9-13900K mixes 5.5 GHz P-cores (CPUs 0-13) with 4.3 GHz E-cores (CPUs 14-29); unpinned runs migrate between them and introduce bimodal variance.
- **Warm state**: each harness runs an untimed worker warmup (`_init_worker` in [`bench/python_baseline.py`](../bench/python_baseline.py), `warmWorker` in [`cmd/goblin-bench/main.go`](../cmd/goblin-bench/main.go)), then 2 untimed warmup solves per family on Tiers 1-2 and 1 on Tier 3.
- **Synthetic corpus and held-out public benchmarks**: each synthetic family in `testdata/corpus/trials/<id>.jsonl` holds 11 distinct seeded problem instances (Tiers 1-2) or 5 (Tier 3), 293 instances across 31 synthetic families (including 8 planted-gap MAD families and 10 spawn-dominated Tier 1 micro-models, compared against Python `PuLP`+`cbc` subprocess in Sections 2 to 4 and in-process `BackendCBC` in Section 9); `WriteCorpusJSON` rejects duplicate trial payloads. Trial $k$ is the identical problem instance in every harness, enabling paired per-instance ratios rather than repeated runs on a fixed matrix. Complementing the synthetic timing corpus, [`public_benchmarks_test.go`](../public_benchmarks_test.go) verifies optimal status, objective parity, and primal feasibility across 14 held-out Netlib LPs and 6 MIPLIB 3.0 MIPs in `testdata/public/manifest.json`, plus a valid dual lower bound under a 25-node budget with cuts and heuristics disabled (`WithCuts(false)`, `WithHeuristics(false)`, `WithMaxNodes(25)`) on `coinor_capplan1` (`21` public instances total).
- **Metrics**: per randomized instance, `PyPrep` sums JSON parse and PuLP model build (`t_parse + t_build`, while MPS formatting `t_format` is recorded separately and included inside `t_solve`), whereas `GoPrep` sums `FromPuLPJSONBytes` parse/build (`t_parse + t_build`, with `t_build = 0`) and `Model.Compile()` (`t_compile`). Aggregate tables report equal-family geometric means with family-clustered 95% Student-$t_{F-1}$ intervals alongside pooled trial geometric means, both multi-trial and family-median wall-clock sums, and full per-instance `[Min-Max]` ranges without floor clamping.
- **Parity**: the benchmark gate requires both `goblin` pure-Go builds and `goblin (CBC)` to match CBC's canonical and per-trial statuses, objectives, and root LP relaxation objectives across all 293 instances within a relative tolerance of `1e-5` under matching `1e-6` MIP gap tolerances.

---

## 2. Aggregate Workload Summary

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `Py / Go` GeoMean [Clustered 95% CI] | Pooled Trial `Py / Go` | Equal-Family `Go / CBC` | `Go` Trial (Med) Sum (ms) | `CBC` Trial (Med) Sum (ms) | `Py` Trial (Med) Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **436.86x** [185.87x-1026.79x] | 436.86x | 0.04x | 8.7 (0.8) | 194.4 (19.4) | 1058.9 (99.4) | 716.29x [39.58x-2961.15x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **17.46x** [9.33x-32.69x] | 17.46x | 0.16x | 724.5 (44.6) | 1978.5 (173.7) | 4302.6 (417.6) | 23.75x [0.14x-154.57x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **10.39x** [5.24x-20.61x] | 10.39x | 0.20x | 785.6 (142.4) | 2891.3 (558.7) | 6178.4 (1065.2) | 10.36x [1.13x-66.00x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **11.53x** [6.50x-20.46x] | 12.33x | 0.22x | 1201.5 (142.6) | 3130.4 (396.2) | 6762.7 (986.2) | 12.36x [0.14x-154.57x] |
| Tier 3 non-MAD MILPs | 4 (20) | **8.02x** [1.79x-35.86x] | 8.02x | 0.27x | 485.9 (98.8) | 1244.8 (230.4) | 2805.7 (599.8) | 6.19x [1.85x-66.00x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **27.10x** [12.14x-60.49x] | 35.22x | 0.10x | 309.5 (44.5) | 1763.9 (338.6) | 3819.9 (505.8) | 38.16x [1.13x-137.99x] |
| Random-weight MAD (negative controls) | 3 (33) | **16.13x** [0.88x-296.73x] | 16.13x | 0.15x | 298.0 (20.2) | 1205.6 (116.2) | 2248.8 (248.4) | 15.11x [1.39x-106.49x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **50.73x** [19.98x-128.78x] | 61.55x | 0.11x | 1209.3 (143.3) | 3300.4 (413.2) | 7720.0 (1076.5) | 43.41x [0.14x-2961.15x] |
| All 31 synthetic corpus families | 31 (293) | **43.15x** [21.31x-87.35x] | 54.48x | 0.11x | 1518.8 (187.7) | 5064.2 (751.8) | 11539.9 (1582.3) | 42.19x [0.14x-2961.15x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

How to read the aggregate table:
1. **Tier 1 micro-models are dominated by subprocess cost.** On 2-to-26-variable models, Python is bounded by the CBC subprocess spawn and `.sol` parsing floor reported in the provenance block above, whereas pure-Go executes in microseconds in memory.
2. **The planted-gap MAD family is zero-gap by construction.** Its instances have `RootLPBound == OptimalObj` by design and close at the root without branching via presolve orbit fixing and root heuristics. The bold row (`Tier 2+3 non-planted MILPs`) excludes both Tier-1 subprocess micro-models and planted-gap MAD to isolate solver-dominated MILP performance.
3. **Heavy-tail dispersion is visible in wall-clock sums and `[Min-Max]` ranges.** Reporting `Go` Trial (Med) Sum, `CBC` Trial (Med) Sum, `Py` Trial (Med) Sum, and per-instance `[Min-Max]` alongside paired geometric means exposes outlier seed variance (for example, in the non-MAD checkpoint table below).
4. **Large degenerate LPs and Forrest-Tomlin updates.** Pure-Go factors the basis with a three-stage singleton and sparse Markowitz LU, `FTRANSparseHint`/`BTRANSparseHint`, `BTRANExact`, `presolveLPWithDeadline` LP presolve, dual cost perturbation, dual bound flipping (`selectDualBFRT`), conditional power-of-two Ruiz equilibration, and in-place mutated-$U$ Forrest-Tomlin updates; see [`solver-internals.md`](solver-internals.md#known-limits-of-the-pure-go-engine).

---

## 3. MAD Group-Item Allocation Formulation

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
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **112.3x** [70.8x-138.0x] | 0.08 | 9.21 | 0.05 | 9.00 | 0.028 | 0.208 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **61.7x** [39.6x-106.5x] | 0.53 | 33.82 | 0.48 | 33.57 | 0.032 | 0.241 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **57.0x** [37.4x-72.2x] | 0.16 | 10.06 | 0.13 | 9.80 | 0.032 | 0.294 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **52.4x** [38.9x-60.7x] | 0.18 | 9.81 | 0.13 | 9.40 | 0.048 | 0.405 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **26.4x** [19.1x-35.8x] | 0.45 | 11.27 | 0.34 | 10.68 | 0.098 | 0.596 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **9.6x** [3.5x-24.8x] | 3.15 | 47.39 | 3.09 | 47.07 | 0.043 | 0.313 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **7.1x** [1.4x-27.5x] | 16.49 | 167.22 | 16.42 | 166.77 | 0.062 | 0.444 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **30.8x** [24.0x-35.4x] | 0.57 | 18.18 | 0.44 | 17.10 | 0.126 | 1.035 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **17.6x** [10.6x-23.0x] | 1.02 | 18.36 | 0.83 | 16.51 | 0.196 | 1.726 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **6.0x** [1.1x-14.5x] | 8.11 | 60.57 | 7.25 | 52.96 | 0.888 | 8.155 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **10.2x** [4.5x-24.7x] | 33.91 | 368.31 | 30.10 | 334.66 | 4.094 | 34.039 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Cross-Instance Latency Dispersion Across Randomized Instances (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.08 \pm 0.02$ ($\pm 0.01$) | $9.23 \pm 0.51$ ($\pm 0.34$) | $0.08 \pm 0.01$ ($\pm 0.01$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.53 \pm 0.24$ ($\pm 0.16$) | $30.66 \pm 9.50$ ($\pm 6.38$) | $0.58 \pm 0.25$ ($\pm 0.17$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.18 \pm 0.04$ ($\pm 0.03$) | $9.97 \pm 0.72$ ($\pm 0.49$) | $0.17 \pm 0.01$ ($\pm 0.01$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.19 \pm 0.03$ ($\pm 0.02$) | $9.85 \pm 0.18$ ($\pm 0.12$) | $0.25 \pm 0.22$ ($\pm 0.14$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.44 \pm 0.07$ ($\pm 0.05$) | $11.60 \pm 0.74$ ($\pm 0.50$) | $0.42 \pm 0.03$ ($\pm 0.02$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $4.14 \pm 3.05$ ($\pm 2.05$) | $35.31 \pm 16.96$ ($\pm 11.39$) | $4.10 \pm 3.04$ ($\pm 2.04$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $22.42 \pm 22.05$ ($\pm 14.81$) | $138.47 \pm 63.40$ ($\pm 42.59$) | $22.26 \pm 22.06$ ($\pm 14.82$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.56 \pm 0.01$ ($\pm 0.02$) | $17.50 \pm 2.41$ ($\pm 2.99$) | $0.66 \pm 0.06$ ($\pm 0.07$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.09 \pm 0.37$ ($\pm 0.46$) | $18.32 \pm 0.32$ ($\pm 0.39$) | $1.20 \pm 0.38$ ($\pm 0.47$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $16.51 \pm 20.81$ ($\pm 25.83$) | $62.89 \pm 3.61$ ($\pm 4.48$) | $16.50 \pm 20.49$ ($\pm 25.43$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $41.77 \pm 14.44$ ($\pm 17.93$) | $575.84 \pm 607.59$ ($\pm 754.30$) | $39.73 \pm 14.17$ ($\pm 17.59$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | **0.05** (1 / 0) | 0.04 (1 / 0) | 0.06 (1) | 0.54 (4) | 0.085 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.48** (2 / 0) | 0.57 (12 / 0) | 0.55 (12) | 0.80 (109) | 0.106 | 4601.00 | 4257.00 |
| `t2_mad_g2_i8` | **0.13** (1 / 0) | 0.76 (8 / 0) | 0.76 (8) | 0.72 (9) | 0.101 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.13** (1 / 0) | 0.13 (2 / 0) | 0.13 (2) | 2.75 (13) | 0.243 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.34** (1 / 0) | 3.22 (22 / 0) | 2.96 (22) | 5.27 (23) | 0.357 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.09** (43 / 0) | 2.97 (112 / 0) | 2.98 (112) | 3.60 (45) | 0.152 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **16.42** (196 / 0) | 30.15 (188 / 0) | 30.26 (188) | 92.20 (539) | 0.274 | 7720.00 | 6060.00 |
| `t3_mad_g5_i20` | **0.44** (1 / 0) | 3.18 (17 / 0) | 2.61 (17) | 0.52 (1) | 0.654 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.83** (1 / 0) | 8.09 (58 / 0) | 7.83 (58) | 13.21 (74) | 1.550 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.25** (2 / 0) | 21.54 (25 / 0) | 20.03 (25) | 250.09 (147, `TimeLimit`) | 2.622 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **30.10** (1 / 0) | 32.35 (2 / 0) | 32.15 (2) | 251.60 (6, `TimeLimit`) | 57.124 | 208000.00 | 208000.00 |
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
| `t1_std_lp` | 3 | 3 | **945.8x** [665.3x-1096.0x] | 0.01 | 6.23 | 0.00 | 6.18 | 0.003 | 0.052 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **925.8x** [480.8x-1317.9x] | 0.01 | 6.31 | 0.00 | 6.27 | 0.003 | 0.051 | `OK` |
| `t1_std_mip` | 3 | 3 | **921.7x** [671.4x-1086.9x] | 0.01 | 7.11 | 0.00 | 7.06 | 0.003 | 0.052 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **1010.6x** [813.2x-1154.2x] | 0.01 | 7.26 | 0.00 | 7.20 | 0.003 | 0.055 | `OK` |
| `t1_whiskas` | 6 | 5 | **613.9x** [453.4x-744.4x] | 0.01 | 6.32 | 0.00 | 6.24 | 0.005 | 0.072 | `OK` |
| `t1_furniture` | 2 | 3 | **305.9x** [83.7x-985.2x] | 0.02 | 8.29 | 0.02 | 8.24 | 0.003 | 0.054 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **109.3x** [50.8x-349.3x] | 0.10 | 8.76 | 0.09 | 8.68 | 0.008 | 0.083 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **2182.1x** [1052.7x-2961.1x] | 0.00 | 6.13 | 0.00 | 6.09 | 0.002 | 0.043 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **112.3x** [70.8x-138.0x] | 0.08 | 9.21 | 0.05 | 9.00 | 0.028 | 0.208 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **61.7x** [39.6x-106.5x] | 0.53 | 33.82 | 0.48 | 33.57 | 0.032 | 0.241 | `OK` |
| `t2_mkp_30` | 30 | 6 | **11.7x** [6.2x-16.0x] | 2.14 | 28.17 | 2.11 | 28.00 | 0.019 | 0.176 | `OK` |
| `t2_mkp_50` | 50 | 8 | **5.9x** [3.1x-31.7x] | 4.23 | 16.81 | 4.20 | 16.57 | 0.032 | 0.255 | `OK` |
| `t2_cflp_36` | 36 | 11 | **54.3x** [17.4x-154.6x] | 0.15 | 9.10 | 0.14 | 8.93 | 0.017 | 0.158 | `OK` |
| `t2_cflp_49` | 49 | 13 | **34.2x** [20.2x-87.9x] | 0.37 | 10.32 | 0.32 | 10.11 | 0.027 | 0.212 | `OK` |
| `t2_gap_45` | 45 | 13 | **23.4x** [4.6x-81.5x] | 0.79 | 14.57 | 0.76 | 14.36 | 0.020 | 0.205 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **11.9x** [5.3x-28.2x] | 4.12 | 68.65 | 4.09 | 68.40 | 0.027 | 0.254 | `OK` |
| `t2_binpack_44` | 44 | 14 | **1.9x** [0.1x-29.1x] | 12.07 | 13.47 | 12.02 | 13.29 | 0.026 | 0.197 | `OK` |
| `t2_setcover_50` | 50 | 20 | **39.4x** [7.1x-121.4x] | 0.28 | 10.77 | 0.25 | 10.49 | 0.031 | 0.287 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **57.0x** [37.4x-72.2x] | 0.16 | 10.06 | 0.13 | 9.80 | 0.032 | 0.294 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **52.4x** [38.9x-60.7x] | 0.18 | 9.81 | 0.13 | 9.40 | 0.048 | 0.405 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **26.4x** [19.1x-35.8x] | 0.45 | 11.27 | 0.34 | 10.68 | 0.098 | 0.596 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.6x** [3.5x-24.8x] | 3.15 | 47.39 | 3.09 | 47.07 | 0.043 | 0.313 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **7.1x** [1.4x-27.5x] | 16.49 | 167.22 | 16.42 | 166.77 | 0.062 | 0.444 | `OK` |
| `t3_mkp_200` | 200 | 8 | **4.9x** [4.0x-6.5x] | 18.97 | 115.38 | 18.89 | 114.76 | 0.077 | 0.612 | `OK` |
| `t3_cflp_300` | 300 | 34 | **26.0x** [7.8x-66.0x] | 8.96 | 247.55 | 8.86 | 246.63 | 0.106 | 0.914 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **3.0x** [1.9x-5.0x] | 63.06 | 157.05 | 62.85 | 155.21 | 0.204 | 1.864 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **10.8x** [5.1x-16.6x] | 7.79 | 79.79 | 7.45 | 76.89 | 0.302 | 2.757 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **30.8x** [24.0x-35.4x] | 0.57 | 18.18 | 0.44 | 17.10 | 0.126 | 1.035 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **17.6x** [10.6x-23.0x] | 1.02 | 18.36 | 0.83 | 16.51 | 0.196 | 1.726 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **6.0x** [1.1x-14.5x] | 8.11 | 60.57 | 7.25 | 52.96 | 0.888 | 8.155 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **10.2x** [4.5x-24.7x] | 33.91 | 368.31 | 30.10 | 334.66 | 4.094 | 34.039 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## 5. Default Build vs. `GOEXPERIMENT=simd`

In [`internal/simd/kernels_amd64.go`](../internal/simd/kernels_amd64.go) and [`internal/simd/kernels_avx512_amd64.go`](../internal/simd/kernels_avx512_amd64.go), runtime CPUID dispatch selects AVX-512, AVX2, or 4-way scalar loops from a single portable `CGO_ENABLED=0` binary (`GOAMD64=v3` FMA-safe). Each vector kernel spills accumulators via `StoreArray` and executes `archsimd.ClearAVXUpperBits()` (`VZEROUPPER`) before scalar tail or caller operations, eliminating AVX-to-SSE transition stalls while preserving IEEE-754 `NaN` propagation parity. AVX2 activates at `minAVX2Len = 32` (`Dot64`, `Axpy64`, `Scale64`, `AbsMax64`, `RowActivityBoundsFinite`, `SelectDualLeaveRow64With`, `MinReducedSkip64With`, `InitReducedRow64`, `AllAbsLE64With`, `ValidateAndNegate64With`), `rows >= 8 && cols >= 8` for `Transpose64With`, `minDualRatioSIMDChunks = 4` (256 cols for `ScanDualRatioCand64With`), and `minAVX2Min2Len = 256` (`Min2ReducedRow64`); on AVX-512 hosts, dense LSAP loops dispatch AVX-512 at `minAVX512Len = 64` and `minAVX512Min2Len = 128` while sparse LP kernels gate AVX-512 at `minAVX512LPLen = 2048` / `minLeaveRowAVX512 = 2048` / `minDualRatioAVX512Chunks = 128` (8,192 cols) to avoid Intel Emerald Rapids License-2 clock down-binning across surrounding scalar LU code.

At the full-solver level, each ratio below is the geometric mean of per-trial `scalar / SIMD` solve times over the same instances on the local AVX2 host (`i9-13900K`), with a 95% $t$-interval on the log ratios; "no detectable effect" means the interval contains 1. "Reachable" is a necessary condition: the AVX2 LP kernels require `n + m >= 32`. Below that size no vector kernel can run, so those small models serve as negative controls where intervals excluding 1 reflect binary code-layout or run-to-run variance rather than SIMD execution. Per-family paired speedups and 95% intervals for larger models appear in the generated table below.

### 5.1 Scope and Pending Target-Hardware Verification

All telemetry in this document is measured locally on the AVX2-only `13th Gen Intel(R) Core(TM) i9-13900K` host (`x86_64`, Linux). Portable `GOOS=linux GOARCH=arm64 CGO_ENABLED=0` and `GOAMD64=v3`/`v4` builds compile and pass local build checks, whereas fresh physical AVX-512 and `linux/arm64` runtime test and benchmark re-verification on the current tree remain pending target hardware access.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 6 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 4 faster, 2 slower, 17 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.73x [0.61x-0.87x] | 0.003 | 0.005 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.75x [0.60x-0.92x] | 0.003 | 0.004 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.81x [0.71x-0.92x] | 0.004 | 0.005 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.88x [0.80x-0.97x] | 0.004 | 0.004 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.88x [0.79x-0.97x] | 0.005 | 0.005 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 0.96x [0.87x-1.06x] | 0.016 | 0.016 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.96x [0.94x-0.97x] | 0.089 | 0.095 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 0.96x [0.87x-1.06x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 1.03x [0.95x-1.12x] | 0.050 | 0.048 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | SIMD slower | 0.90x [0.86x-0.95x] | 0.479 | 0.518 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 1.00x [0.99x-1.00x] | 2.115 | 2.154 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 0.99x [0.96x-1.03x] | 4.205 | 4.037 | 11 |
| `t2_cflp_36` | 47 | possible | no detectable effect | 0.99x [0.96x-1.01x] | 0.137 | 0.140 | 11 |
| `t2_cflp_49` | 62 | possible | SIMD faster | 1.10x [1.06x-1.15x] | 0.322 | 0.295 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 0.94x [0.88x-1.00x] | 0.762 | 0.755 | 11 |
| `t2_lotsizing_48` | 80 | possible | SIMD slower | 0.96x [0.92x-1.00x] | 4.090 | 4.353 | 11 |
| `t2_binpack_44` | 58 | possible | no detectable effect | 0.97x [0.93x-1.02x] | 12.016 | 11.847 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 0.90x [0.74x-1.09x] | 0.250 | 0.237 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 1.07x [0.96x-1.20x] | 0.126 | 0.127 | 11 |
| `t2_mad_g3_i9` | 108 | possible | no detectable effect | 0.87x [0.64x-1.19x] | 0.126 | 0.127 | 11 |
| `t2_mad_g4_i12` | 156 | possible | no detectable effect | 1.05x [0.96x-1.16x] | 0.337 | 0.322 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.01x [0.97x-1.06x] | 3.086 | 2.915 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 1.01x [0.98x-1.04x] | 16.424 | 16.447 | 11 |
| `t3_mkp_200` | 208 | possible | SIMD faster | 1.07x [1.06x-1.08x] | 18.891 | 17.581 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.05x [0.97x-1.14x] | 8.863 | 8.953 | 5 |
| `t3_lotsizing_504` | 840 | possible | SIMD faster | 1.02x [1.01x-1.04x] | 62.853 | 61.158 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 0.99x [0.84x-1.18x] | 7.454 | 6.238 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 0.92x [0.85x-1.01x] | 0.439 | 0.492 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 0.95x [0.91x-1.01x] | 0.829 | 0.815 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 1.00x [0.96x-1.04x] | 7.246 | 7.253 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | SIMD faster | 1.07x [1.01x-1.15x] | 30.096 | 28.926 | 5 |
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
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.002 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.09** (1 / 8) | 0.10 (2 / 8) | 0.14 (18) | 0.21 (35) | 0.007 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.05** (1 / 0) | 0.04 (1 / 0) | 0.06 (1) | 0.54 (4) | 0.085 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.48** (2 / 0) | 0.57 (12 / 0) | 0.55 (12) | 0.80 (109) | 0.106 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.11** (5 / 9) | 2.65 (40 / 9) | 1.03 (51) | 0.92 (61) | 0.025 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **4.20** (1 / 4) | 11.59 (2 / 4) | 4.76 (118) | 4.19 (111) | 0.038 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.14** (1 / 2) | 0.21 (3 / 2) | 0.16 (7) | 0.18 (7) | 0.025 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.32** (1 / 7) | 0.66 (3 / 7) | 0.32 (9) | 0.30 (9) | 0.036 | 909.02 | 827.30 |
| `t2_gap_45` | **0.76** (1 / 7) | 0.40 (1 / 7) | 1.82 (41) | 1.97 (60) | 0.025 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **4.09** (2 / 37) | 8.41 (25 / 37) | 3.04 (162) | 3.30 (199) | 0.059 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **12.02** (274 / 12) | 14.01 (298 / 12) | 9.13 (772) | 27.35 (1941) | 0.027 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.25** (1 / 6) | 0.37 (2 / 6) | 0.72 (6) | 0.67 (6) | 0.034 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.13** (1 / 0) | 0.76 (8 / 0) | 0.76 (8) | 0.72 (9) | 0.101 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.13** (1 / 0) | 0.13 (2 / 0) | 0.13 (2) | 2.75 (13) | 0.243 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.34** (1 / 0) | 3.22 (22 / 0) | 2.96 (22) | 5.27 (23) | 0.357 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.09** (43 / 0) | 2.97 (112 / 0) | 2.98 (112) | 3.60 (45) | 0.152 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **16.42** (196 / 0) | 30.15 (188 / 0) | 30.26 (188) | 92.20 (539) | 0.274 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **18.89** (250 / 14) | 81.82 (1135 / 16) | 166.00 (5000, `NodeLimit`) | 147.89 (5000, `NodeLimit`) | 0.117 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **8.86** (1 / 28) | 18.16 (54 / 34) | 8.76 (116) | 8.69 (118) | 0.169 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **62.85** (2 / 206) | 80.71 (376 / 206) | 250.04 (3710, `TimeLimit`) | 250.05 (3800, `TimeLimit`) | 0.751 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **7.45** (31 / 55) | 19.68 (87 / 53) | 250.03 (2585, `TimeLimit`) | 250.01 (2621, `TimeLimit`) | 0.179 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.44** (1 / 0) | 3.18 (17 / 0) | 2.61 (17) | 0.52 (1) | 0.654 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.83** (1 / 0) | 8.09 (58 / 0) | 7.83 (58) | 13.21 (74) | 1.550 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.25** (2 / 0) | 21.54 (25 / 0) | 20.03 (25) | 250.09 (147, `TimeLimit`) | 2.622 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **30.10** (1 / 0) | 32.35 (2 / 0) | 32.15 (2) | 251.60 (6, `TimeLimit`) | 57.124 | 208000.00 | 208000.00 |
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
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.56x** [1.17x-2.08x] | 1.56x | 4.71x | 0.214 (0.0195) | 0.319 (0.0291) | 1.52x [0.87x-3.43x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **2.94x** [1.85x-4.68x] | 2.94x | 2.80x | 9.129 (0.807) | 32.743 (2.960) | 2.53x [0.87x-14.88x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.88x** [1.87x-4.44x] | 2.88x | 3.59x | 215.897 (43.067) | 931.056 (185.628) | 2.65x [1.47x-6.14x] |
| Square families ($m = n$) | 16 (146) | **2.78x** [2.05x-3.77x] | 2.72x | 3.68x | 220.516 (43.046) | 953.326 (186.674) | 2.37x [0.93x-14.88x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.77x** [1.26x-2.49x] | 1.65x | 3.04x | 4.723 (0.848) | 10.792 (1.943) | 1.93x [0.87x-2.80x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.27x** [1.16x-4.46x] | 2.39x | 2.70x | 35.653 (6.153) | 64.945 (10.913) | 1.79x [0.93x-14.88x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.46x** [1.93x-3.13x] | 2.38x | 3.49x | 225.239 (43.893) | 964.118 (188.617) | 2.08x [0.87x-14.88x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 7.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **2.46x** | 0.0005 | 0.0013 | 0.0006 | 0.0181 | $0.0005 \pm 0.0001$ | $0.0012 \pm 0.0003$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.44x** | 0.0016 | 0.0024 | 0.0019 | 0.0102 | $0.0016 \pm 0.0003$ | $0.0024 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.72x** | 0.0036 | 0.0062 | 0.0051 | 0.0151 | $0.0035 \pm 0.0007$ | $0.0060 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.20x** | 0.0035 | 0.0040 | 0.0043 | 0.0140 | $0.0033 \pm 0.0006$ | $0.0039 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.19x** | 0.0039 | 0.0043 | 0.0051 | 0.0151 | $0.0039 \pm 0.0012$ | $0.0045 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.66x** | 0.0064 | 0.0110 | 0.0112 | 0.0328 | $0.0067 \pm 0.0020$ | $0.0110 \pm 0.0024$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.64x** | 0.0290 | 0.0708 | 0.0409 | 0.0966 | $0.0268 \pm 0.0065$ | $0.0693 \pm 0.0112$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.35x** | 0.0530 | 0.175 | 0.0832 | 0.225 | $0.0522 \pm 0.0049$ | $0.176 \pm 0.0263$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **1.94x** | 0.0482 | 0.0945 | 0.0868 | 0.143 | $0.0499 \pm 0.0098$ | $0.0963 \pm 0.0144$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.63x** | 0.137 | 0.217 | 0.166 | 0.258 | $0.147 \pm 0.0472$ | $0.234 \pm 0.0440$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **11.82x** | 0.0617 | 0.686 | 0.0823 | 0.760 | $0.0589 \pm 0.0092$ | $0.689 \pm 0.0277$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.92x** | 0.0358 | 0.0704 | 0.0621 | 0.144 | $0.0384 \pm 0.0133$ | $0.0715 \pm 0.0073$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **1.95x** | 0.0391 | 0.0794 | 0.0672 | 0.154 | $0.0426 \pm 0.0128$ | $0.0809 \pm 0.0089$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.38x** | 0.0837 | 0.280 | 0.110 | 0.369 | $0.0846 \pm 0.0157$ | $0.284 \pm 0.0317$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.90x** | 0.319 | 1.287 | 0.433 | 1.696 | $0.330 \pm 0.0558$ | $1.276 \pm 0.119$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.04x** | 1.312 | 5.201 | 1.868 | 7.696 | $1.318 \pm 0.159$ | $5.312 \pm 0.395$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.75x** | 0.847 | 1.491 | 1.437 | 4.075 | $0.849 \pm 0.0245$ | $1.483 \pm 0.0516$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.66x** | 5.049 | 8.407 | 5.202 | 10.090 | $5.697 \pm 2.158$ | $9.226 \pm 2.613$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.53x** | 0.334 | 0.819 | 0.726 | 3.569 | $0.330 \pm 0.0180$ | $0.836 \pm 0.0590$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.30x** | 0.431 | 0.966 | 0.807 | 3.721 | $0.421 \pm 0.0207$ | $0.969 \pm 0.0346$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.96x** | 5.724 | 27.884 | 7.593 | 36.537 | $5.721 \pm 0.432$ | $28.366 \pm 2.273$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.88x** | 29.371 | 140.860 | 37.152 | 184.940 | $28.844 \pm 3.923$ | $140.019 \pm 8.963$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 7.3 LSAP Scalar vs. `GOEXPERIMENT=simd`

In [`assign/solver.go`](../assign/solver.go), [`simd.AllAbsLE64With`](../internal/simd/kernels_amd64.go) and [`simd.ValidateAndNegate64With`](../internal/simd/assign_amd64.go) validate finite cost bounds via AVX2 checks whenever `len(cost) >= 32` (while the fused `colReduceValidateSquare` fast path skips `AllAbsLE64With` on finite square `Minimize` matrices and dispatches `ColReduceValidateRows64With` only for $n \ge 32$), [`simd.Transpose64With`](../internal/simd/assign_amd64.go) handles tall transposes with $\text{rows} \ge 8$ and $\text{cols} \ge 8$, [`simd.MinReducedSkip64With`](../internal/simd/assign_amd64.go) accelerates reduction-transfer slack scans in `preprocessSquareJV` ($n \ge 32$), [`simd.Min2ReducedRow64With`](../internal/simd/kernels_amd64.go) accelerates first/second reduced-minimum scans in `preprocessRectangularBV` and `augmentingRowReduction` at `minAVX2Min2Len = 256` (`minAVX512Min2Len = 128`), and [`simd.InitReducedRow64With`](../internal/simd/kernels_amd64.go) runs on a slice of length $\max(m, n) \ge 32$ at the start of each Dijkstra augmentation. All vector kernels clean upper `YMM` state via `StoreArray` + `ClearAVXUpperBits()` (`VZEROUPPER`) before scalar operations; exact per-family verdicts appear in the generated table below.

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` with no `>= 8x8` tall transpose and either fused square `Minimize` non-sparse path or `rows * cols < 32`, so no AVX2 kernel runs): 1 of 3 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32`, tall `rows > cols` with `>= 8x8`, or fallback `AllAbsLE64`/`ValidateAndNegate64` at `rows * cols >= 32`): 18 faster, 0 slower, 1 with no detectable effect, out of 19.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source | 0.99x [0.94x-1.04x] | 0.0005 | 0.0005 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.05x [1.02x-1.08x] | 0.0016 | 0.0016 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source | 0.99x [0.95x-1.03x] | 0.0036 | 0.0036 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | SIMD faster | 1.08x [1.06x-1.10x] | 0.0035 | 0.0032 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | SIMD faster | 1.13x [1.09x-1.17x] | 0.0039 | 0.0033 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | no detectable effect | 0.97x [0.95x-1.00x] | 0.0064 | 0.0066 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.27x [1.22x-1.31x] | 0.0290 | 0.0232 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.33x [1.25x-1.41x] | 0.0530 | 0.0409 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.71x [1.56x-1.87x] | 0.0482 | 0.0304 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.72x [1.65x-1.79x] | 0.137 | 0.0800 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.30x [1.18x-1.42x] | 0.0617 | 0.0474 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.28x [1.00x-1.65x] | 0.0358 | 0.0287 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.29x [1.05x-1.60x] | 0.0391 | 0.0299 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.40x [1.20x-1.63x] | 0.0837 | 0.0567 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.59x [1.54x-1.64x] | 0.319 | 0.202 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.76x [1.48x-2.09x] | 1.312 | 0.727 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.46x [2.27x-2.67x] | 0.847 | 0.351 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.62x [2.58x-2.66x] | 5.049 | 1.900 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.84x [1.79x-1.89x] | 0.334 | 0.179 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.55x [1.44x-1.67x] | 0.431 | 0.281 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.69x [1.54x-1.85x] | 5.724 | 3.396 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.54x [1.51x-1.57x] | 29.371 | 19.131 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

---

## 8. Production Binary Footprint and Linker Dead-Code Elimination

The `goblin-bench -binary-size` mode compiles four standalone `CGO_ENABLED=0` programs in an isolated temporary module against the live workspace across both default `go build` and `GOEXPERIMENT=simd`, saving compiled artifact sizes and `go tool nm -size` symbol counts to `bench/results/goblin_binary_size.json`. [`ValidateBinarySizeTelemetry`](../bench/docgen.go) rejects the telemetry bundle if `assign_default` links any `math/big`, `encoding/json`, `VIPR`, or `Certif` symbols, if `goblin_default` links any `encoding/json`, `VIPR`, or `Certif` symbols, or if any profile links `encoding/json`:

<!-- BEGIN GENERATED:BINARY_SIZE_TABLE -->
| Profile | Default Raw / Stripped (KiB) | `GOEXPERIMENT=simd` Raw / Stripped (KiB) | Stripped Delta vs. Default | `goblin` Text (KiB) | `math/big` Syms | `encoding/json` Syms | `VIPR` Syms | `Certif` Syms |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `assign` default (`LinearSum` + `SolveInto`) | 2786.7 / **1740.2** | 2816.6 / **1804.2** | baseline | 117.3 | `0` | `0` | `0` | `0` |
| `assign` certified (`+ assign.WithCertified(true)`) | 2906.9 / **1824.2** | 2940.0 / **1888.2** | +84.0 KiB (+4.8%) | 175.5 | `0` | `0` | `0` | `17` |
| `goblin` default (`Model.Solve`, `DefaultOptions`) | 5300.2 / **3372.2** | 5189.0 / **3388.2** | baseline | 893.0 | `130` | `0` | `0` | `0` |
| `goblin` certified + VIPR (`+ WithCertified + WithVIPRTrace`) | 6465.4 / **4160.2** | 6338.8 / **4180.2** | +788.0 KiB (+23.4%) | 1343.7 | `155` | `0` | `212` | `14` |
<!-- END GENERATED:BINARY_SIZE_TABLE -->

---

## 9. Pure-Go vs. In-Process C++ CBC Performance Analysis

Comparing isolated unpresolved `Root LP` timings, paired trial ratios, and solver-core checkpoints (`CP5 +Presolve`, `CP6 +Cuts`, `CP7 Full`) between `BackendGo` and in-process `BackendCBC` (`c1c171ec`) removes Python MPS formatting and OS process-spawn overhead across all 31 synthetic families and highlights four structural trade-offs on large degenerate LPs and heavy-tail branch-and-cut models:

<!-- BEGIN GENERATED:CBC_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `BackendCBC / BackendGo` Total [Clustered 95% CI] | Equal-Family `BackendCBC / BackendGo` Solve [Clustered 95% CI] | `BackendGo` Trial (Med) Sum (ms) | `BackendCBC` Trial (Med) Sum (ms) | Per-Instance `CBC / Go` Total Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro-models (`N <= 26`) | 10 (110) | **27.38x** [11.73x-63.94x] | **51.11x** [16.82x-155.27x] | 8.7 (0.8) | 194.4 (19.4) | 26.08x [2.78x-154.20x] |
| Tier 2: Medium MILPs (`30 <= N <= 84`) | 13 (143) | **6.11x** [3.76x-9.94x] | **6.73x** [3.95x-11.48x] | 724.5 (44.6) | 1978.5 (173.7) | 7.19x [0.08x-52.68x] |
| Tier 3: Large MILPs (`145 <= N <= 4036`) | 8 (40) | **5.01x** [2.75x-9.15x] | **5.48x** [2.84x-10.56x] | 785.6 (142.4) | 2891.3 (558.7) | 6.11x [0.62x-22.20x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **4.65x** [2.94x-7.35x] | **4.85x** [2.99x-7.86x] | 1201.5 (142.6) | 3130.4 (396.2) | 5.26x [0.08x-52.68x] |
| All 31 synthetic families | 31 (293) | **9.42x** [6.13x-14.47x] | **12.28x** [7.14x-21.12x] | 1518.8 (187.7) | 5064.2 (751.8) | 9.19x [0.08x-154.20x] |
<!-- END GENERATED:CBC_SUMMARY_TABLE -->

### 9.1 MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families ($N = G(I + T + 3)$ variables, $M = G(2T + 3) + I$ constraints) separate solver behavior into three scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Paired `Go / CBC` | Go Total Med (ms) | CBC Total Med (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.04x** | 0.08 | 2.44 | 0.62x | 0.05 (1 / 19) | 2.41 (0 / 0) | 0.085 (28) | 0.137 (19) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.05x** | 0.53 | 11.28 | 0.73x | 0.48 (2 / 72) | 11.25 (6 / 83) | 0.106 (30) | 0.145 (18) |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.08x** | 0.16 | 2.02 | 0.61x | 0.13 (1 / 26) | 1.98 (0 / 0) | 0.101 (33) | 0.168 (21) |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.07x** | 0.18 | 2.55 | 0.87x | 0.13 (1 / 40) | 2.50 (0 / 0) | 0.243 (58) | 0.281 (40) |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.12x** | 0.45 | 3.31 | 0.89x | 0.34 (1 / 51) | 3.23 (0 / 0) | 0.357 (80) | 0.401 (63) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.25x** | 3.15 | 16.58 | 1.00x | 3.09 (43 / 483) | 16.53 (12 / 216) | 0.152 (38) | 0.153 (24) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.25x** | 16.49 | 88.39 | 0.97x | 16.42 (196 / 2299) | 88.32 (198 / 1300) | 0.274 (65) | 0.282 (54) |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.07x** | 0.57 | 8.11 | 1.04x | 0.44 (1 / 75) | 7.98 (0 / 0) | 0.654 (91) | 0.629 (94) |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.14x** | 1.02 | 7.37 | 1.45x | 0.83 (1 / 98) | 7.18 (0 / 0) | 1.550 (244) | 1.069 (149) |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **0.29x** | 8.11 | 36.97 | 0.73x | 7.25 (2 / 377) | 35.66 (0 / 0) | 2.622 (224) | 3.585 (319) |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **0.17x** | 33.91 | 275.88 | 1.62x | 30.10 (1 / 2439) | 271.13 (0 / 0) | 57.124 (1900) | 35.239 (3128) |
<!-- END GENERATED:MAD_CBC_GAP_TABLE -->

1. **Small-to-Medium Planted-Gap MAD (`t1_mad_g2_i4` through `t3_mad_g5_i20`)**: Every structured `t*_mad_g*` instance has a zero root LP gap by design (`RootLPBound == OptimalObj`). Pure-Go closes those models at node 1 through presolve orbit fixing and root heuristics (`tryPropagatedRounding`, `tryRootSubMIP`, and `tryFractionalDiving` in [`heuristics.go`](../heuristics.go)) without C++ `CbcModel` and `CglPreProcess` setup costs.
2. **Large Planted-Gap MAD (`t3_mad_g6_i30`, `t3_mad_g10_i100`, `t3_mad_g4_i1000`)**: Pure-Go `Go Solve` closes these models at the root (exact node counts and solve times appear in the generated table above) because [`Presolve`](../presolve.go) verifies the $S_G$ group-permutation automorphism (`applyOrbitSymmetryBreaking`), fixes the $G \times G$ designated-item bijection to $I_G$, and eliminates $G^2$ binary variables and $2G$ assignment equations before the root LP runs, while [`FTRANSparseHint`](../lu.go) and [`BTRANSparseHint`](../lu.go) restrict basis triangular sweeps to active index windows. On the unpresolved `Root LP` column:
   - **Equality Crash Basis (`t3_mad_g10_i100`)**: [`crashEqualityBasis`](../simplex.go) builds a lower-triangular, dual-feasible equality basis with analytical DSE weights.
   - **Dense-Row Coupling and Forrest-Tomlin Updates (`t3_mad_g4_i1000`)**: On 1,000-row bases, [`crashEqualityBasis`](../simplex.go) skips columns incident to coupling rows wider than 128 nonzeros so BFRT bound flips retain sparse unit slacks, while [`lu.go`](../lu.go) performs in-place mutated-$U$ Forrest-Tomlin updates (`PushEtaSparse`) inside pre-allocated headroom arenas up to `maxEtaUpdates = 56`.
3. **Symmetric Random-Weight MAD (`t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`)**: Assigning dense random weights (`BuildMADRandom`) removes the zero-gap structure and forces Branch-and-Cut to prove optimality. In these models, [`Presolve`](../presolve.go) verifies exact $S_G$ group symmetry and fixes the $G \times G$ designated-item bijection to $I_G$, [`bab.go`](../bab.go) exits after round 1 when root cuts yield zero bound gain and purges slack root cuts via [`PurgeSlackCuts`](../simplex.go), and [`tryRootSubMIP`](../heuristics.go) enforces a 50% RINS integer-fixing gate on zero-cut roots. Comparing `CP7 Full` against `CP5 +Presolve` (`Presolve=on, Cuts=off, Heuristics=off`) in [Section 3](#mad-solver-core-checkpoint-progression) isolates the effect of cutting planes and primal heuristics on random-weight MAD search.

### 9.2 Non-MAD Corpus Comparison, Dispersion, and Checkpoint Trade-Offs

Across the non-MAD families, multi-row MIR cut aggregation (`separateAggregatedMIRCuts`), bound-cutoff strong-branching domain reductions (`selectBranchVar`), and globally valid in-tree cuts (`generateInTreeCutRound`) accelerate Branch-and-Cut convergence, while the generated `Go Total Mean +- SD` and `CBC Total Mean +- SD` columns capture seed-to-seed variance (for example outlier seeds in `t2_binpack_44`):

<!-- BEGIN GENERATED:NON_MAD_CBC_GAP_TABLE -->
| Problem ID | $N \times M$ | Paired `Go / CBC` | Go Total Mean $\pm$ SD (ms) | CBC Total Mean $\pm$ SD (ms) | Go CP7 Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go CP6 +Cuts (ms / nodes / cuts) | Go CP5 +Presolve (ms / nodes) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t3_mkp_200` | $200 \times 8$ | **0.49x** | $21.87 \pm 13.62$ | $40.41 \pm 19.20$ | 18.89 (250 / 2860) | 49.34 (154 / 1145) | 81.82 (1135 / 16) | 166.00 (5000, `NodeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **0.35x** | $5.90 \pm 6.17$ | $12.77 \pm 9.51$ | 4.20 (1 / 20) | 9.61 (0 / 8) | 11.59 (2 / 4) | 4.76 (118) |
| `t2_binpack_44` | $44 \times 14$ | **1.14x** | $22.07 \pm 41.62$ | $6.80 \pm 2.20$ | 12.02 (274 / 1988) | 6.20 (0 / 8) | 14.01 (298 / 12) | 9.13 (772) |
| `t3_cflp_300` | $300 \times 34$ | **0.16x** | $10.72 \pm 7.32$ | $55.13 \pm 11.81$ | 8.86 (1 / 119) | 49.82 (18 / 483) | 18.16 (54 / 34) | 8.76 (116) |
| `t3_lotsizing_504` | $504 \times 336$ | **0.56x** | $56.24 \pm 34.91$ | $92.49 \pm 57.09$ | 62.85 (2 / 1351) | 70.88 (10 / 377) | 80.71 (376 / 206) | 250.04 (3710, `TimeLimit`) |
| `t3_sched_1000` | $1000 \times 100$ | **0.12x** | $8.35 \pm 4.41$ | $60.92 \pm 4.63$ | 7.45 (31 / 917) | 59.65 (0 / 76) | 19.68 (87 / 53) | 250.03 (2585, `TimeLimit`) |
<!-- END GENERATED:NON_MAD_CBC_GAP_TABLE -->

<!-- BEGIN GENERATED:NON_MAD_CBC_NOTES -->
- **Heavy-tail regression on `t2_binpack_44`**: pure-Go `BackendGo` ($22.07 \pm 41.62$ ms mean, 12.07 ms median) trails in-process `BackendCBC` ($6.80 \pm 2.20$ ms mean, 6.23 ms median) with a **0.87x** `BackendCBC / BackendGo` geometric-mean ratio (`Go / CBC = 1.14x`), running slower on 8 of 11 trials (peaking at 145.92 ms vs. 12.30 ms) while root cuts increase `CP5 +Presolve` (9.13 ms) to `CP6 +Cuts` (14.01 ms).
<!-- END GENERATED:NON_MAD_CBC_NOTES -->
- **Bounded node budget on `coinor_capplan1`**: on the largest held-out public MILP in `testdata/public/manifest.json` (`coinor_capplan1`, 2,172 variables and 726 constraints), [`public_benchmarks_test.go`](../public_benchmarks_test.go) runs `BackendGo` under a bounded 25-node budget with cuts and heuristics disabled (`WithCuts(false)`, `WithHeuristics(false)`, `WithMaxNodes(25)`) to verify a valid global dual lower bound at `StatusNodeLimit`, whereas in-process `BackendCBC` solves the same instance to `StatusOptimal` without node, cut, or heuristic caps.

---

## 10. Reproducing Benchmarks and Syncing Documentation

Run all commands from the `goblin/` repository root on a quiet host so that all eight telemetry files record uniform `git_commit`, `source_fingerprint`, and `uname_release`:

```bash
# 0. Build the vendored COIN-OR CBC/CLP C++ stack with -DNDEBUG into third_party/Cbc/dist (once per checkout)
./scripts/build_cbc.sh

# 1. Unit, regression, 293-trial CBC parity, 21-instance public suite, and documentation gates
go test ./...
GOEXPERIMENT=simd go test ./...
go test -tags=cbc ./...

# 2. Regenerate the deduplicated 31-family PuLP JSON and multi-trial JSONL corpus in testdata/corpus/
go run ./cmd/goblin-bench -generate-corpus

# 3. Commit code changes first so every telemetry file records clean git provenance.
git status --short   # must be empty for git_dirty=false provenance

# 4. Warm-state Python PuLP + COIN-OR CBC (c1c171ec) baseline, pinned to one performance core
CBC_BIN=./third_party/Cbc/dist/bin/cbc taskset -c 2 python3 bench/python_baseline.py --corpus-dir testdata/corpus --out-json bench/results/python_baseline.json --workers 1

# 5. Warm-state goblin benchmarks across separate Default, GOEXPERIMENT=simd, and C++ CBC builds, pinned to CPU 2
go build -o /tmp/goblin-bench-default ./cmd/goblin-bench
GOEXPERIMENT=simd go build -o /tmp/goblin-bench-simd ./cmd/goblin-bench
go build -tags=cbc -o /tmp/goblin-bench-cbc ./cmd/goblin-bench
taskset -c 2 /tmp/goblin-bench-default -backend go -workers 1 -out bench/results/goblin_scalar.json
taskset -c 2 /tmp/goblin-bench-simd -backend go -workers 1 -out bench/results/goblin_simd.json
taskset -c 2 /tmp/goblin-bench-cbc -backend cbc -workers 1 -out bench/results/goblin_cbc.json

# 6. Warm-state SciPy and goblin/assign LSAP benchmarks across separate Default & SIMD builds, pinned to CPU 2
taskset -c 2 python3 bench/scipy_assign_baseline.py --corpus-dir testdata/assign_corpus --out-json bench/results/scipy_assign_baseline.json
taskset -c 2 /tmp/goblin-bench-default -assign -out bench/results/goblin_assign_scalar.json
taskset -c 2 /tmp/goblin-bench-simd -assign -out bench/results/goblin_assign_simd.json
taskset -c 2 /tmp/goblin-bench-default -binary-size -out bench/results/goblin_binary_size.json
rm -f /tmp/goblin-bench-default /tmp/goblin-bench-simd /tmp/goblin-bench-cbc

# 7. Render README.md, docs/benchmarks.md, and bench/results/final_report.md from the telemetry
go run ./cmd/goblin-bench -sync-docs

# 8. Verify documentation, parity, symbol identifiers, and provenance (exits non-zero on any failure)
go run ./cmd/goblin-bench -check-docs

# 9. Differential tests against CBC and SciPy on freshly generated random instances
python3 bench/difftest_cbc.py --instances 400 --seed 1
python3 bench/difftest_assign.py --instances 500 --seed 1

# 10. Hot-path compiler, disassembly, and Amdahl's Law profile audit (scalar and SIMD)
go run ./cmd/goblin-optcheck -benchtime=200ms
go run ./cmd/goblin-optcheck -simd -benchtime=200ms

# 11. Lean 4 Prove Lane build (78 jobs), arm64 + amd64 ELF verification, CLI/JSONL checks, and 21 default-build Fuzz corpora
./scripts/verify_formal.sh
PYTHONDONTWRITEBYTECODE=1 python3 scripts/extract_elf_track5.py --verify
PYTHONDONTWRITEBYTECODE=1 python3 scripts/extract_amd64_track5e.py --verify
```

Running `go test ./...` also executes `TestDocumentationAndBenchmarkSync`, which validates corpus dimensions, parity, symbol identifiers, provenance, and every generated block across `README.md`, `docs/benchmarks.md`, and `bench/results/final_report.md`.
