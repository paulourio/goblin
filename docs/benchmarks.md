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
- **Unified telemetry source**: fingerprint `8c76940984c7433e` (commit `7900285032b0a01380777bde12851e1d6d5cb5ba`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.09 ms median on a 1-variable MILP and 5.90 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
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
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **387.88x** [168.21x-894.43x] | 387.88x | 0.04x | 8.4 (0.7) | 144.2 (13.3) | 985.1 (92.9) | 590.94x [30.60x-2598.92x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **18.54x** [11.47x-29.96x] | 18.54x | 0.18x | 522.6 (35.5) | 1652.5 (148.6) | 4169.2 (398.3) | 22.23x [1.33x-143.31x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **9.91x** [4.63x-21.18x] | 9.91x | 0.26x | 851.5 (160.3) | 2412.1 (488.9) | 6015.5 (1050.7) | 9.35x [1.05x-72.32x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **12.30x** [7.33x-20.61x] | 13.47x | 0.21x | 1064.1 (151.3) | 2789.9 (363.1) | 6623.4 (970.4) | 12.07x [1.33x-143.31x] |
| Tier 3 non-MAD MILPs | 4 (20) | **7.49x** [1.29x-43.62x] | 7.49x | 0.30x | 550.7 (116.6) | 1195.2 (219.5) | 2774.5 (600.8) | 5.27x [1.43x-72.32x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **26.09x** [11.45x-59.46x] | 33.78x | 0.16x | 310.6 (44.6) | 1287.6 (275.5) | 3654.4 (487.2) | 35.30x [1.05x-153.76x] |
| Random-weight MAD (negative controls) | 3 (33) | **16.06x** [0.80x-323.22x] | 16.06x | 0.19x | 302.6 (20.7) | 913.7 (92.7) | 2179.3 (236.1) | 14.83x [1.33x-111.22x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **49.65x** [20.59x-119.76x] | 60.54x | 0.11x | 1071.8 (152.0) | 2921.1 (375.4) | 7515.4 (1054.7) | 42.89x [1.33x-2598.92x] |
| All 31 synthetic corpus families | 31 (293) | **42.06x** [21.49x-82.28x] | 53.30x | 0.12x | 1382.5 (196.6) | 4208.7 (650.8) | 11169.8 (1541.9) | 38.45x [1.05x-2598.92x] |
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
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **137.0x** [109.1x-153.8x] | 0.06 | 8.59 | 0.04 | 8.40 | 0.022 | 0.191 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **64.4x** [42.7x-111.2x] | 0.49 | 32.18 | 0.46 | 31.94 | 0.025 | 0.234 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **39.6x** [35.1x-46.5x] | 0.22 | 8.55 | 0.15 | 8.29 | 0.073 | 0.257 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **52.4x** [41.0x-60.9x] | 0.17 | 9.31 | 0.12 | 8.92 | 0.047 | 0.386 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **25.6x** [22.2x-30.5x] | 0.42 | 10.78 | 0.32 | 10.21 | 0.111 | 0.571 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **9.1x** [3.3x-23.9x] | 3.74 | 45.03 | 3.63 | 44.72 | 0.038 | 0.276 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **7.1x** [1.3x-31.2x] | 16.49 | 158.88 | 16.42 | 158.44 | 0.057 | 0.436 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **29.9x** [23.0x-34.9x] | 0.55 | 16.87 | 0.42 | 15.79 | 0.123 | 0.966 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **17.1x** [10.3x-22.3x] | 0.98 | 17.04 | 0.78 | 15.43 | 0.220 | 1.610 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **5.8x** [1.1x-13.9x] | 8.04 | 62.34 | 7.11 | 53.78 | 1.147 | 8.007 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **9.9x** [4.3x-23.8x] | 34.15 | 353.73 | 29.35 | 319.90 | 4.429 | 32.994 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Cross-Instance Latency Dispersion Across Randomized Instances (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.06 \pm 0.00$ ($\pm 0.00$) | $8.46 \pm 0.35$ ($\pm 0.24$) | $0.09 \pm 0.02$ ($\pm 0.02$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.49 \pm 0.23$ ($\pm 0.15$) | $29.24 \pm 9.46$ ($\pm 6.35$) | $0.49 \pm 0.23$ ($\pm 0.15$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.22 \pm 0.01$ ($\pm 0.01$) | $8.76 \pm 0.48$ ($\pm 0.33$) | $0.17 \pm 0.02$ ($\pm 0.01$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.18 \pm 0.03$ ($\pm 0.02$) | $9.37 \pm 0.20$ ($\pm 0.14$) | $0.20 \pm 0.06$ ($\pm 0.04$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.43 \pm 0.04$ ($\pm 0.03$) | $10.99 \pm 0.63$ ($\pm 0.43$) | $0.35 \pm 0.02$ ($\pm 0.01$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $4.14 \pm 3.13$ ($\pm 2.11$) | $33.16 \pm 16.23$ ($\pm 10.90$) | $4.14 \pm 3.32$ ($\pm 2.23$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $22.89 \pm 23.44$ ($\pm 15.75$) | $135.72 \pm 62.14$ ($\pm 41.74$) | $23.32 \pm 23.38$ ($\pm 15.71$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.54 \pm 0.02$ ($\pm 0.02$) | $16.41 \pm 2.35$ ($\pm 2.92$) | $0.54 \pm 0.01$ ($\pm 0.02$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.04 \pm 0.35$ ($\pm 0.44$) | $17.13 \pm 0.24$ ($\pm 0.30$) | $1.10 \pm 0.38$ ($\pm 0.47$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $17.43 \pm 22.44$ ($\pm 27.86$) | $63.79 \pm 3.53$ ($\pm 4.38$) | $17.03 \pm 21.60$ ($\pm 26.82$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $41.14 \pm 14.42$ ($\pm 17.90$) | $550.87 \pm 581.68$ ($\pm 722.14$) | $40.39 \pm 15.24$ ($\pm 18.92$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | **0.04** (1 / 0) | 0.04 (1 / 0) | 0.05 (1) | 0.50 (4) | 0.082 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.46** (2 / 0) | 0.61 (12 / 0) | 0.53 (12) | 0.76 (109) | 0.102 | 4601.00 | 4257.00 |
| `t2_mad_g2_i8` | **0.15** (1 / 0) | 0.83 (8 / 0) | 0.80 (8) | 0.78 (9) | 0.110 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.12** (1 / 0) | 0.12 (2 / 0) | 0.12 (2) | 2.69 (13) | 0.240 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.32** (1 / 0) | 3.30 (22 / 0) | 3.00 (22) | 5.35 (23) | 0.373 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.63** (43 / 0) | 2.99 (112 / 0) | 2.98 (112) | 3.82 (45) | 0.154 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **16.42** (196 / 0) | 31.32 (188 / 0) | 31.41 (188) | 92.24 (539) | 0.274 | 7720.00 | 6060.00 |
| `t3_mad_g5_i20` | **0.42** (1 / 0) | 3.02 (17 / 0) | 2.90 (17) | 0.49 (1) | 0.646 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.78** (1 / 0) | 9.40 (58 / 0) | 8.07 (58) | 13.49 (74) | 1.587 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.11** (2 / 0) | 21.47 (25 / 0) | 19.35 (25) | 250.62 (146, `TimeLimit`) | 2.571 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **29.35** (1 / 0) | 32.09 (2 / 0) | 31.32 (2) | 250.21 (6, `TimeLimit`) | 75.872 | 208000.00 | 208000.00 |
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
| `t1_std_lp` | 3 | 3 | **778.5x** [471.5x-1091.8x] | 0.01 | 5.91 | 0.00 | 5.86 | 0.004 | 0.044 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **891.1x** [627.3x-1149.3x] | 0.01 | 5.85 | 0.00 | 5.81 | 0.003 | 0.045 | `OK` |
| `t1_std_mip` | 3 | 3 | **746.7x** [505.9x-895.3x] | 0.01 | 6.66 | 0.00 | 6.61 | 0.003 | 0.044 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **878.7x** [631.6x-1066.9x] | 0.01 | 6.89 | 0.00 | 6.84 | 0.003 | 0.048 | `OK` |
| `t1_whiskas` | 6 | 5 | **500.1x** [397.9x-626.1x] | 0.01 | 5.85 | 0.00 | 5.78 | 0.005 | 0.065 | `OK` |
| `t1_furniture` | 2 | 3 | **273.9x** [72.6x-963.1x] | 0.02 | 7.06 | 0.01 | 7.02 | 0.003 | 0.041 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **71.3x** [30.6x-240.8x] | 0.14 | 8.29 | 0.12 | 8.22 | 0.014 | 0.074 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **1968.0x** [878.0x-2598.9x] | 0.00 | 5.68 | 0.00 | 5.65 | 0.002 | 0.035 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **137.0x** [109.1x-153.8x] | 0.06 | 8.59 | 0.04 | 8.40 | 0.022 | 0.191 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **64.4x** [42.7x-111.2x] | 0.49 | 32.18 | 0.46 | 31.94 | 0.025 | 0.234 | `OK` |
| `t2_mkp_30` | 30 | 6 | **10.3x** [6.7x-15.5x] | 2.29 | 29.08 | 2.26 | 28.90 | 0.021 | 0.176 | `OK` |
| `t2_mkp_50` | 50 | 8 | **5.2x** [2.7x-32.9x] | 5.65 | 16.62 | 5.60 | 16.34 | 0.031 | 0.275 | `OK` |
| `t2_cflp_36` | 36 | 11 | **49.7x** [20.8x-143.3x] | 0.17 | 8.79 | 0.15 | 8.63 | 0.018 | 0.149 | `OK` |
| `t2_cflp_49` | 49 | 13 | **27.4x** [16.0x-72.6x] | 0.43 | 9.90 | 0.39 | 9.68 | 0.047 | 0.189 | `OK` |
| `t2_gap_45` | 45 | 13 | **18.3x** [4.1x-63.3x] | 0.86 | 13.49 | 0.83 | 13.27 | 0.024 | 0.184 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **11.4x** [4.8x-26.0x] | 3.91 | 65.19 | 3.89 | 64.97 | 0.029 | 0.235 | `OK` |
| `t2_binpack_44` | 44 | 14 | **12.2x** [5.1x-23.0x] | 0.98 | 12.34 | 0.96 | 12.17 | 0.022 | 0.173 | `OK` |
| `t2_setcover_50` | 50 | 20 | **47.6x** [6.8x-125.1x] | 0.18 | 10.29 | 0.14 | 10.00 | 0.034 | 0.285 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **39.6x** [35.1x-46.5x] | 0.22 | 8.55 | 0.15 | 8.29 | 0.073 | 0.257 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **52.4x** [41.0x-60.9x] | 0.17 | 9.31 | 0.12 | 8.92 | 0.047 | 0.386 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **25.6x** [22.2x-30.5x] | 0.42 | 10.78 | 0.32 | 10.21 | 0.111 | 0.571 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.1x** [3.3x-23.9x] | 3.74 | 45.03 | 3.63 | 44.72 | 0.038 | 0.276 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **7.1x** [1.3x-31.2x] | 16.49 | 158.88 | 16.42 | 158.44 | 0.057 | 0.436 | `OK` |
| `t3_mkp_200` | 200 | 8 | **4.3x** [3.1x-5.6x] | 23.45 | 111.06 | 23.38 | 110.49 | 0.076 | 0.612 | `OK` |
| `t3_cflp_300` | 300 | 34 | **32.5x** [7.8x-72.3x] | 7.27 | 252.11 | 6.99 | 251.24 | 0.123 | 0.878 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **2.5x** [1.4x-4.6x] | 78.22 | 159.87 | 78.00 | 156.50 | 0.262 | 1.871 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **9.0x** [4.4x-14.9x] | 7.66 | 77.73 | 7.27 | 75.13 | 0.296 | 2.634 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **29.9x** [23.0x-34.9x] | 0.55 | 16.87 | 0.42 | 15.79 | 0.123 | 0.966 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **17.1x** [10.3x-22.3x] | 0.98 | 17.04 | 0.78 | 15.43 | 0.220 | 1.610 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.8x** [1.1x-13.9x] | 8.04 | 62.34 | 7.11 | 53.78 | 1.147 | 8.007 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **9.9x** [4.3x-23.8x] | 34.15 | 353.73 | 29.35 | 319.90 | 4.429 | 32.994 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## 5. Default Build vs. `GOEXPERIMENT=simd`

In [`internal/simd/kernels_amd64.go`](../internal/simd/kernels_amd64.go) and [`internal/simd/kernels_avx512_amd64.go`](../internal/simd/kernels_avx512_amd64.go), runtime CPUID dispatch selects AVX-512, AVX2, or 4-way scalar loops from a single portable `CGO_ENABLED=0` binary (`GOAMD64=v3` FMA-safe). Each vector kernel spills accumulators via `StoreArray` and executes `archsimd.ClearAVXUpperBits()` (`VZEROUPPER`) before scalar tail or caller operations, eliminating AVX-to-SSE transition stalls while preserving IEEE-754 `NaN` propagation parity. AVX2 activates at `minAVX2Len = 32` (`Dot64`, `Axpy64`, `Scale64`, `AbsMax64`, `RowActivityBoundsFinite`, `SelectDualLeaveRow64With`, `MinReducedSkip64With`, `InitReducedRow64`, `AllAbsLE64With`, `ValidateAndNegate64With`), `rows >= 8 && cols >= 8` for `Transpose64With`, `minDualRatioSIMDChunks = 4` (256 cols for `ScanDualRatioCand64With`), and `minAVX2Min2Len = 256` (`Min2ReducedRow64`); on AVX-512 hosts, dense LSAP loops dispatch AVX-512 at `minAVX512Len = 64` and `minAVX512Min2Len = 128` while sparse LP kernels gate AVX-512 at `minAVX512LPLen = 2048` / `minLeaveRowAVX512 = 2048` / `minDualRatioAVX512Chunks = 128` (8,192 cols) to avoid Intel Emerald Rapids License-2 clock down-binning across surrounding scalar LU code.

At the full-solver level, each ratio below is the geometric mean of per-trial `scalar / SIMD` solve times over the same instances on the local AVX2 host (`i9-13900K`), with a 95% $t$-interval on the log ratios; "no detectable effect" means the interval contains 1. "Reachable" is a necessary condition: the AVX2 LP kernels require `n + m >= 32`. Below that size no vector kernel can run, so those small models serve as negative controls where intervals excluding 1 reflect binary code-layout or run-to-run variance rather than SIMD execution. Per-family paired speedups and 95% intervals for larger models appear in the generated table below.

### 5.1 Target-Silicon Runtime Verification (AVX-512 and `linux/arm64`)

Primary CBC and SciPy differential comparison tables in Sections 2 to 4 and 6 to 9 run locally on the AVX2 `13th Gen Intel(R) Core(TM) i9-13900K` host (`x86_64`, Linux). Supplementary point-in-time target-silicon verification ([`scripts/verify_target_silicon.sh`](../scripts/verify_target_silicon.sh)) cross-compiles static `CGO_ENABLED=0` binaries and executes the test matrix, 6-round interleaved AB/BA vector kernel crossover benchmarks, and full 31-family LP/MILP plus 22-family LSAP suites on ephemeral Compute Engine instances (`n4-standard-4` Intel Xeon Emerald Rapids AVX-512 and `c4a-standard-4` or `t2a-standard-4` `linux/arm64`), rendering directly from `bench/results/target_silicon/{amd64_avx512,linux_arm64}.json` without coupling routine `go test` runs to cloud access:

<!-- BEGIN GENERATED:TARGET_SILICON_SUMMARY -->
- **Native `linux/amd64` AVX-512 host (`bench/results/target_silicon/amd64_avx512.json`)**: GCE `n4-standard-4` (`debian-12`, `hyperdisk-balanced`, `INTEL(R) XEON(R) PLATINUM 8581C CPU @ 2.10GHz`, `4` vCPUs, `Linux 6.1.0-56-cloud-amd64 x86_64`, pinned CPU `1`, pre-benchmark host load `0.12%`), `13` `avx512*` ISA flags (`avx512f`, `avx512dq`, `avx512bw`, `avx512vl`, `avx512_vbmi2`, `avx512_vpopcntdq`), `simd.HardwareSIMDTier() == "avx512"`, `simd.ActiveSIMDTier() == "avx512"`, fingerprint `379bd473ec8716c8` (commit `e8e555d`), Go `go1.27.3-X:nodwarf5` / `go1.27.3-X:simd`. Passed `10/10` test configurations (`0` failures across `.`, `./internal/simd`, `./assign`, and `./format` for default `GOAMD64=v1` and `GOEXPERIMENT=simd` `GOAMD64=v1`, `v3`, `v4` across `GOBLIN_SIMD_MAX_TIER={scalar,avx2,avx512}`, including explicit `TestFormalTrack5D_SIMDParityAndEdgeAudit`, `TestFormalTrack5E_LiveSIMDDifferential`, `TestFormalTrack5E_ZeroAllocContracts`, `TestFormalTrack5E_HardwareNaNParity`, and `TestTwoSumTwoProdFMASafety`).
- **Native `linux/arm64` host (`bench/results/target_silicon/linux_arm64.json`)**: GCE `c4a-standard-4` (`debian-12-arm64`, `hyperdisk-balanced`, `ARM Neoverse-V2`, `4` vCPUs, `Linux 6.1.0-56-cloud-arm64 aarch64`, `GOARM64=v8.0`, pinned CPU `1`, pre-benchmark host load `0.00%`), fingerprint `379bd473ec8716c8` (commit `e8e555d`), `2/2` test configurations passed (`0` failures across `.`, `./internal/simd`, `./assign`, and `./format`, including hardware FMA non-contraction in `TestTwoSumTwoProdFMASafety` and Track 5D/5E IEEE-754 parity; both default (`go1.27.3-X:nodwarf5`) and `GOEXPERIMENT=simd` (`go1.27.3-X:simd`) compile and pass all suites cleanly on `linux/arm64`, dispatching `!amd64` 4-way unrolled scalar kernels (`HardwareSIMDTier() == "scalar"`, `ActiveSIMDTier() == "scalar"`)).

#### Emerald Rapids 6-Round Interleaved AB/BA Vector Kernel Crossover (`simd.test`, CPU 1)

| Kernel | $N$ | Scalar Med (ns/op) | AVX2 Med (ns/op) | AVX-512 Med (ns/op) | Paired `Scalar / AVX2` [95% CI] | Paired `Scalar / AVX-512` [95% CI] | Paired `AVX2 / AVX-512` [95% CI] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `AllAbsLE64` | 64 | 45.4 | 12.0 | 9.58 | 3.76x [3.67x-3.85x] | **4.75x** [4.68x-4.83x] | 1.26x [1.23x-1.30x] |
| `AllAbsLE64` | 1024 | 719 | 103 | 76.1 | 6.99x [6.96x-7.02x] | **9.44x** [9.38x-9.50x] | 1.35x [1.34x-1.36x] |
| `MinNoNaN64` | 64 | 27.9 | 18.1 | 16.4 | 1.54x [1.53x-1.56x] | **1.71x** [1.67x-1.74x] | 1.11x [1.09x-1.13x] |
| `MinNoNaN64` | 1024 | 419 | 206 | 91.6 | 2.03x [2.02x-2.03x] | **4.55x** [4.48x-4.62x] | 2.24x [2.21x-2.28x] |
| `InitReducedRow64` | 64 | 58.7 | 20.7 | 18.0 | 2.85x [2.82x-2.88x] | **3.28x** [3.24x-3.32x] | 1.15x [1.15x-1.15x] |
| `InitReducedRow64` | 1024 | 1111 | 234 | 131 | 4.75x [4.74x-4.77x] | **8.49x** [8.47x-8.52x] | 1.79x [1.78x-1.79x] |
| `Min2ReducedRow64` | 128 | 67.4 | 68.3 | 50.8 | 0.99x [0.98x-0.99x] | **1.32x** [1.31x-1.33x] | 1.34x [1.33x-1.36x] |
| `Min2ReducedRow64` | 1024 | 435 | 180 | 118 | 2.41x [2.39x-2.44x] | **3.68x** [3.66x-3.69x] | 1.52x [1.51x-1.54x] |
| `RelaxRow64` | 64 | 39.9 | 37.2 | 17.5 | 1.07x [1.06x-1.08x] | **2.28x** [2.27x-2.29x] | 2.13x [2.11x-2.15x] |
| `RelaxRow64` | 1024 | 550 | 515 | 191 | 1.07x [1.06x-1.08x] | **2.89x** [2.86x-2.91x] | 2.70x [2.68x-2.72x] |
| `Dot64` | 256 | 74.2 | 34.8 | 35.2 | 2.13x [2.12x-2.14x] | **2.10x** [2.10x-2.11x] | 0.99x [0.99x-0.99x] |
| `Dot64` | 2048 | 582 | 249 | 138 | 2.33x [2.32x-2.35x] | **4.20x** [4.14x-4.27x] | 1.80x [1.78x-1.82x] |
| `Axpy64` | 256 | 92.1 | 37.8 | 38.8 | 2.36x [2.20x-2.53x] | **2.36x** [2.23x-2.49x] | 1.00x [0.92x-1.09x] |
| `Axpy64` | 2048 | 714 | 259 | 152 | 2.75x [2.75x-2.76x] | **4.71x** [4.64x-4.79x] | 1.71x [1.68x-1.74x] |
| `SelectDualLeaveRow64` | 256 | 551 | 92.1 | 92.0 | 5.99x [5.93x-6.04x] | **5.98x** [5.92x-6.03x] | 1.00x [0.99x-1.01x] |
| `SelectDualLeaveRow64` | 2048 | 4118 | 780 | 515 | 5.24x [5.15x-5.33x] | **7.96x** [7.81x-8.11x] | 1.52x [1.51x-1.53x] |
| `ScanDualRatioCand64` | 1024 | 1158 | 589 | 589 | 1.97x [1.94x-1.99x] | **1.97x** [1.94x-2.00x] | 1.00x [0.99x-1.01x] |
| `ScanDualRatioCand64` | 8192 | 9120 | 5093 | 3948 | 1.79x [1.77x-1.81x] | **2.31x** [2.28x-2.34x] | 1.29x [1.28x-1.29x] |

#### Target-Silicon End-to-End LP/MILP and LSAP Corpus Summary (`goblin-bench`, CPU 1)

| Workload Subset | Families (Trials) | `n4` Scalar Solve Sum (ms) | `n4` AVX2 Solve Sum (ms) | `n4` AVX-512 Solve Sum (ms) | `n4` Equal-Family `Scalar / AVX-512` [95% CI] | `n4` Equal-Family `AVX2 / AVX-512` [95% CI] | `arm64` Default Solve Sum (ms) | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| LP/MILP Tier 2+3 non-planted (solver-dominated) | 14 (130) | 1900.6 | 1901.8 | 1903.5 | **1.00x** [0.98x-1.03x] | 1.00x [0.97x-1.04x] | 1168.7 | `OK` |
| LP/MILP All 31 synthetic families | 31 (293) | 2403.4 | 2405.2 | 2405.5 | **1.01x** [0.98x-1.04x] | 1.04x [1.01x-1.08x] | 1432.4 | `OK` |
| LSAP Tier 1: Micro/Small (`N <= 32`) | 6 (66) | 0.373 | 0.383 | 0.355 | **1.07x** [0.96x-1.20x] | 1.08x [0.99x-1.18x] | 0.230 | `OK` |
| LSAP Tier 2: Medium (`64 <= N <= 256`) | 9 (99) | 15.577 | 11.024 | 10.373 | **1.42x** [1.29x-1.56x] | 1.03x [0.97x-1.10x] | 10.784 | `OK` |
| LSAP Tier 3: Large (`500 <= N <= 2000`) | 7 (35) | 361.436 | 224.782 | 172.349 | **2.05x** [1.77x-2.37x] | 1.16x [1.06x-1.27x] | 299.944 | `OK` |
| **LSAP All 22 synthetic families** | 22 (200) | 377.386 | 236.189 | 183.077 | **1.48x** [1.30x-1.68x] | 1.08x [1.04x-1.13x] | 310.957 | `OK` |
<!-- END GENERATED:TARGET_SILICON_SUMMARY -->

### 5.2 Local AVX2 Host (`i9-13900K`) Scalar vs. `GOEXPERIMENT=simd`

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 5 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 3 faster, 4 slower, 16 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 0.96x [0.87x-1.05x] | 0.003 | 0.004 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.76x [0.68x-0.86x] | 0.003 | 0.004 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.84x [0.78x-0.91x] | 0.004 | 0.005 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source | 0.90x [0.81x-1.00x] | 0.004 | 0.005 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.85x [0.76x-0.94x] | 0.005 | 0.006 | 11 |
| `t1_furniture` | 5 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.88x [0.79x-0.97x] | 0.015 | 0.018 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.14x [1.09x-1.20x] | 0.125 | 0.102 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 1.02x [0.94x-1.11x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | SIMD slower | 0.69x [0.56x-0.85x] | 0.038 | 0.048 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 0.97x [0.94x-1.01x] | 0.460 | 0.452 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 0.96x [0.81x-1.14x] | 2.261 | 2.456 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 1.00x [0.97x-1.03x] | 5.596 | 5.340 | 11 |
| `t2_cflp_36` | 47 | possible | no detectable effect | 0.97x [0.93x-1.00x] | 0.151 | 0.164 | 11 |
| `t2_cflp_49` | 62 | possible | no detectable effect | 1.00x [0.86x-1.17x] | 0.391 | 0.343 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 1.02x [0.97x-1.09x] | 0.833 | 0.872 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 1.01x [0.93x-1.09x] | 3.887 | 3.309 | 11 |
| `t2_binpack_44` | 58 | possible | no detectable effect | 1.01x [0.97x-1.06x] | 0.957 | 0.976 | 11 |
| `t2_setcover_50` | 70 | possible | SIMD slower | 0.90x [0.84x-0.95x] | 0.142 | 0.161 | 11 |
| `t2_mad_g2_i8` | 72 | possible | SIMD faster | 1.12x [1.04x-1.20x] | 0.151 | 0.129 | 11 |
| `t2_mad_g3_i9` | 108 | possible | no detectable effect | 0.94x [0.83x-1.07x] | 0.122 | 0.127 | 11 |
| `t2_mad_g4_i12` | 156 | possible | SIMD faster | 1.12x [1.07x-1.18x] | 0.318 | 0.278 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.02x [0.98x-1.06x] | 3.626 | 3.029 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | SIMD slower | 0.97x [0.95x-1.00x] | 16.416 | 16.552 | 11 |
| `t3_mkp_200` | 208 | possible | no detectable effect | 1.03x [0.97x-1.10x] | 23.379 | 21.339 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.00x [0.87x-1.15x] | 6.985 | 5.953 | 5 |
| `t3_lotsizing_504` | 840 | possible | SIMD faster | 1.02x [1.01x-1.03x] | 78.000 | 77.434 | 5 |
| `t3_sched_1000` | 1100 | possible | SIMD slower | 0.96x [0.93x-1.00x] | 7.269 | 7.493 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 1.00x [0.99x-1.02x] | 0.417 | 0.411 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 0.97x [0.90x-1.05x] | 0.776 | 0.818 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 1.02x [0.93x-1.11x] | 7.105 | 7.143 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | no detectable effect | 1.03x [0.91x-1.16x] | 29.348 | 31.261 | 5 |
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
| `t1_whiskas` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.004 | 0.52 | 0.52 |
| `t1_furniture` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.002 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.12** (1 / 8) | 0.11 (2 / 8) | 0.17 (18) | 0.24 (35) | 0.010 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.04** (1 / 0) | 0.04 (1 / 0) | 0.05 (1) | 0.50 (4) | 0.082 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.46** (2 / 0) | 0.61 (12 / 0) | 0.53 (12) | 0.76 (109) | 0.102 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.26** (5 / 9) | 3.11 (40 / 9) | 1.16 (51) | 1.03 (61) | 0.027 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **5.60** (1 / 4) | 14.58 (2 / 4) | 5.64 (118) | 5.39 (111) | 0.042 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.15** (1 / 2) | 0.30 (3 / 2) | 0.17 (7) | 0.18 (7) | 0.025 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.39** (1 / 10) | 0.58 (4 / 10) | 0.30 (9) | 0.28 (9) | 0.034 | 909.02 | 827.30 |
| `t2_gap_45` | **0.83** (1 / 9) | 0.43 (1 / 9) | 1.72 (41) | 2.09 (60) | 0.028 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **3.89** (2 / 37) | 6.89 (25 / 37) | 3.18 (162) | 3.82 (199) | 0.069 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **0.96** (2 / 6) | 1.41 (5 / 6) | 9.68 (772) | 28.62 (1941) | 0.029 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.14** (1 / 6) | 0.35 (2 / 6) | 0.65 (6) | 0.62 (6) | 0.031 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.15** (1 / 0) | 0.83 (8 / 0) | 0.80 (8) | 0.78 (9) | 0.110 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.12** (1 / 0) | 0.12 (2 / 0) | 0.12 (2) | 2.69 (13) | 0.240 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.32** (1 / 0) | 3.30 (22 / 0) | 3.00 (22) | 5.35 (23) | 0.373 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.63** (43 / 0) | 2.99 (112 / 0) | 2.98 (112) | 3.82 (45) | 0.154 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **16.42** (196 / 0) | 31.32 (188 / 0) | 31.41 (188) | 92.24 (539) | 0.274 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **23.38** (250 / 14) | 107.70 (1135 / 16) | 216.09 (5000, `NodeLimit`) | 201.45 (5000, `NodeLimit`) | 0.124 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **6.99** (1 / 28) | 18.74 (54 / 34) | 8.88 (116) | 9.50 (118) | 0.173 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **78.00** (2 / 206) | 114.52 (376 / 206) | 250.03 (2914, `TimeLimit`) | 250.08 (3043, `TimeLimit`) | 0.697 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **7.27** (31 / 55) | 26.84 (87 / 53) | 250.08 (1797, `TimeLimit`) | 250.11 (1783, `TimeLimit`) | 0.233 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.42** (1 / 0) | 3.02 (17 / 0) | 2.90 (17) | 0.49 (1) | 0.646 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.78** (1 / 0) | 9.40 (58 / 0) | 8.07 (58) | 13.49 (74) | 1.587 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.11** (2 / 0) | 21.47 (25 / 0) | 19.35 (25) | 250.62 (146, `TimeLimit`) | 2.571 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **29.35** (1 / 0) | 32.09 (2 / 0) | 31.32 (2) | 250.21 (6, `TimeLimit`) | 75.872 | 208000.00 | 208000.00 |
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
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.21x** [0.93x-1.58x] | 1.21x | 3.93x | 0.253 (0.0204) | 0.307 (0.0274) | 1.37x [0.09x-2.38x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **2.95x** [1.86x-4.70x] | 2.95x | 3.00x | 8.715 (0.767) | 31.126 (2.815) | 2.51x [1.17x-15.44x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.89x** [1.84x-4.53x] | 2.89x | 3.63x | 201.623 (39.453) | 882.783 (175.604) | 2.49x [1.50x-6.33x] |
| Square families ($m = n$) | 16 (146) | **2.57x** [1.81x-3.65x] | 2.47x | 3.61x | 206.113 (39.449) | 903.950 (176.588) | 2.14x [0.09x-15.44x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.70x** [1.11x-2.61x] | 1.58x | 2.99x | 4.478 (0.791) | 10.265 (1.857) | 1.94x [0.59x-2.56x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.22x** [1.11x-4.44x] | 2.34x | 2.79x | 34.252 (6.069) | 62.106 (10.542) | 1.76x [0.94x-15.44x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.30x** [1.75x-3.03x] | 2.19x | 3.43x | 210.592 (40.240) | 914.215 (178.446) | 2.03x [0.09x-15.44x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 7.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.31x** | 0.0006 | 0.0010 | 0.0007 | 0.0130 | $0.0010 \pm 0.0010$ | $0.0010 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **0.90x** | 0.0018 | 0.0022 | 0.0019 | 0.0096 | $0.0038 \pm 0.0057$ | $0.0022 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.52x** | 0.0036 | 0.0058 | 0.0050 | 0.0149 | $0.0038 \pm 0.0010$ | $0.0058 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.01x** | 0.0039 | 0.0039 | 0.0050 | 0.0138 | $0.0038 \pm 0.0008$ | $0.0037 \pm 0.0007$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.02x** | 0.0044 | 0.0041 | 0.0054 | 0.0144 | $0.0043 \pm 0.0011$ | $0.0043 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.70x** | 0.0062 | 0.0104 | 0.0102 | 0.0317 | $0.0063 \pm 0.0014$ | $0.0108 \pm 0.0023$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.61x** | 0.0284 | 0.0700 | 0.0345 | 0.0950 | $0.0263 \pm 0.0066$ | $0.0673 \pm 0.0107$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.33x** | 0.0507 | 0.168 | 0.0661 | 0.216 | $0.0504 \pm 0.0041$ | $0.169 \pm 0.0253$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.00x** | 0.0467 | 0.0915 | 0.0690 | 0.139 | $0.0465 \pm 0.0072$ | $0.0933 \pm 0.0138$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.56x** | 0.135 | 0.207 | 0.141 | 0.240 | $0.145 \pm 0.0436$ | $0.220 \pm 0.0420$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **11.97x** | 0.0566 | 0.650 | 0.0775 | 0.721 | $0.0552 \pm 0.0072$ | $0.655 \pm 0.0208$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **2.02x** | 0.0331 | 0.0715 | 0.0548 | 0.141 | $0.0355 \pm 0.0096$ | $0.0704 \pm 0.0076$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.04x** | 0.0375 | 0.0756 | 0.0602 | 0.146 | $0.0375 \pm 0.0036$ | $0.0765 \pm 0.0074$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.30x** | 0.0795 | 0.264 | 0.109 | 0.334 | $0.0815 \pm 0.0119$ | $0.268 \pm 0.0301$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.87x** | 0.300 | 1.216 | 0.425 | 1.612 | $0.315 \pm 0.0536$ | $1.209 \pm 0.119$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.21x** | 1.140 | 4.992 | 1.498 | 7.352 | $1.209 \pm 0.163$ | $5.062 \pm 0.386$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.69x** | 0.831 | 1.402 | 1.380 | 3.866 | $0.840 \pm 0.0427$ | $1.421 \pm 0.0541$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.65x** | 4.990 | 8.175 | 5.523 | 8.873 | $5.445 \pm 1.906$ | $8.831 \pm 2.506$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.45x** | 0.332 | 0.779 | 0.686 | 3.364 | $0.325 \pm 0.0196$ | $0.796 \pm 0.0527$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.34x** | 0.381 | 0.923 | 0.759 | 3.498 | $0.393 \pm 0.0197$ | $0.917 \pm 0.0242$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **5.02x** | 5.526 | 26.579 | 7.130 | 34.682 | $5.500 \pm 0.334$ | $27.655 \pm 2.831$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.98x** | 26.253 | 132.754 | 35.031 | 174.627 | $26.612 \pm 3.492$ | $131.875 \pm 7.902$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 7.3 LSAP Scalar vs. `GOEXPERIMENT=simd`

In [`assign/solver.go`](../assign/solver.go), [`simd.AllAbsLE64With`](../internal/simd/kernels_amd64.go) and [`simd.ValidateAndNegate64With`](../internal/simd/assign_amd64.go) validate finite cost bounds via AVX2 checks whenever `len(cost) >= 32` (while the fused `colReduceValidateSquare` fast path skips `AllAbsLE64With` on finite square `Minimize` matrices and dispatches `ColReduceValidateRows64With` only for $n \ge 32$), [`simd.Transpose64With`](../internal/simd/assign_amd64.go) handles tall transposes with $\text{rows} \ge 8$ and $\text{cols} \ge 8$, [`simd.MinReducedSkip64With`](../internal/simd/assign_amd64.go) accelerates reduction-transfer slack scans in `preprocessSquareJV` ($n \ge 32$), [`simd.Min2ReducedRow64With`](../internal/simd/kernels_amd64.go) accelerates first/second reduced-minimum scans in `preprocessRectangularBV` and `augmentingRowReduction` at `minAVX2Min2Len = 256` (`minAVX512Min2Len = 128`), and [`simd.InitReducedRow64With`](../internal/simd/kernels_amd64.go) runs on a slice of length $\max(m, n) \ge 32$ at the start of each Dijkstra augmentation. All vector kernels clean upper `YMM` state via `StoreArray` + `ClearAVXUpperBits()` (`VZEROUPPER`) before scalar operations; exact per-family verdicts appear in the generated table below.

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` with no `>= 8x8` tall transpose and either fused square `Minimize` non-sparse path or `rows * cols < 32`, so no AVX2 kernel runs): 3 of 3 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32`, tall `rows > cols` with `>= 8x8`, or fallback `AllAbsLE64`/`ValidateAndNegate64` at `rows * cols >= 32`): 17 faster, 0 slower, 2 with no detectable effect, out of 19.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.78x [1.11x-2.84x] | 0.0006 | 0.0004 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.74x [1.02x-2.97x] | 0.0018 | 0.0015 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.12x [1.01x-1.25x] | 0.0036 | 0.0034 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | SIMD faster | 1.29x [1.13x-1.46x] | 0.0039 | 0.0031 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | SIMD faster | 1.32x [1.10x-1.58x] | 0.0044 | 0.0030 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | no detectable effect | 1.01x [0.98x-1.05x] | 0.0062 | 0.0059 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.32x [1.27x-1.38x] | 0.0284 | 0.0213 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.29x [1.22x-1.37x] | 0.0507 | 0.0404 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.75x [1.55x-1.97x] | 0.0467 | 0.0287 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.73x [1.67x-1.79x] | 0.135 | 0.0777 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.20x [1.10x-1.30x] | 0.0566 | 0.0464 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | no detectable effect | 1.21x [0.96x-1.52x] | 0.0331 | 0.0277 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.20x [1.08x-1.34x] | 0.0375 | 0.0313 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.35x [1.22x-1.50x] | 0.0795 | 0.0568 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.58x [1.47x-1.69x] | 0.300 | 0.190 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.73x [1.62x-1.84x] | 1.140 | 0.692 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.45x [2.26x-2.65x] | 0.831 | 0.325 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.65x [2.53x-2.77x] | 4.990 | 1.817 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.69x [1.61x-1.76x] | 0.332 | 0.198 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.53x [1.32x-1.76x] | 0.381 | 0.238 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.65x [1.51x-1.81x] | 5.526 | 3.290 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.51x [1.43x-1.60x] | 26.253 | 18.174 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

---

## 8. Production Binary Footprint and Linker Dead-Code Elimination

The `goblin-bench -binary-size` mode compiles four standalone `CGO_ENABLED=0` programs in an isolated temporary module against the live workspace across both default `go build` and `GOEXPERIMENT=simd`, saving compiled artifact sizes and `go tool nm -size` symbol counts to `bench/results/goblin_binary_size.json`. [`ValidateBinarySizeTelemetry`](../bench/docgen.go) rejects the telemetry bundle if `assign_default` links any `math/big`, `encoding/json`, `VIPR`, or `Certif` symbols, if `goblin_default` links any `encoding/json`, `VIPR`, or `Certif` symbols, or if any profile links `encoding/json`:

<!-- BEGIN GENERATED:BINARY_SIZE_TABLE -->
| Profile | Default Raw / Stripped (KiB) | `GOEXPERIMENT=simd` Raw / Stripped (KiB) | Stripped Delta vs. Default | `goblin` Text (KiB) | `math/big` Syms | `encoding/json` Syms | `VIPR` Syms | `Certif` Syms |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `assign` default (`LinearSum` + `SolveInto`) | 2822.8 / **1768.2** | 2858.1 / **1832.2** | baseline | 118.7 | `0` | `0` | `0` | `0` |
| `assign` certified (`+ assign.WithCertified(true)`) | 2942.9 / **1844.2** | 2980.7 / **1916.2** | +76.0 KiB (+4.3%) | 177.2 | `0` | `0` | `0` | `17` |
| `goblin` default (`Model.Solve`, `DefaultOptions`) | 5586.5 / **3560.2** | 5472.0 / **3576.2** | baseline | 1023.4 | `130` | `0` | `0` | `0` |
| `goblin` certified + VIPR (`+ WithCertified + WithVIPRTrace`) | 6833.1 / **4400.2** | 6697.0 / **4420.2** | +840.0 KiB (+23.6%) | 1503.1 | `155` | `0` | `220` | `14` |
<!-- END GENERATED:BINARY_SIZE_TABLE -->

---

## 9. Pure-Go vs. In-Process C++ CBC Performance Analysis

Comparing isolated unpresolved `Root LP` timings, paired trial ratios, and solver-core checkpoints (`CP5 +Presolve`, `CP6 +Cuts`, `CP7 Full`) between `BackendGo` and in-process `BackendCBC` (`c1c171ec`) removes Python MPS formatting and OS process-spawn overhead across all 31 synthetic families and highlights four structural trade-offs on large degenerate LPs and heavy-tail branch-and-cut models:

<!-- BEGIN GENERATED:CBC_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `BackendCBC / BackendGo` Total [Clustered 95% CI] | Equal-Family `BackendCBC / BackendGo` Solve [Clustered 95% CI] | `BackendGo` Trial (Med) Sum (ms) | `BackendCBC` Trial (Med) Sum (ms) | Per-Instance `CBC / Go` Total Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro-models (`N <= 26`) | 10 (110) | **22.99x** [9.67x-54.69x] | **44.22x** [14.23x-137.43x] | 8.4 (0.7) | 144.2 (13.3) | 18.30x [3.66x-152.34x] |
| Tier 2: Medium MILPs (`30 <= N <= 84`) | 13 (143) | **5.68x** [4.03x-8.01x] | **6.39x** [4.31x-9.47x] | 522.6 (35.5) | 1652.5 (148.6) | 5.73x [0.55x-45.59x] |
| Tier 3: Large MILPs (`145 <= N <= 4036`) | 8 (40) | **3.88x** [2.11x-7.11x] | **4.23x** [2.22x-8.07x] | 851.5 (160.3) | 2412.1 (488.9) | 3.78x [0.37x-14.31x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **4.74x** [3.15x-7.14x] | **4.98x** [3.21x-7.75x] | 1064.1 (151.3) | 2789.9 (363.1) | 5.39x [0.55x-45.59x] |
| All 31 synthetic families | 31 (293) | **8.08x** [5.36x-12.19x] | **10.72x** [6.32x-18.19x] | 1382.5 (196.6) | 4208.7 (650.8) | 7.28x [0.37x-152.34x] |
<!-- END GENERATED:CBC_SUMMARY_TABLE -->

### 9.1 MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families ($N = G(I + T + 3)$ variables, $M = G(2T + 3) + I$ constraints) separate solver behavior into three scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Paired `Go / CBC` | Go Total Med (ms) | CBC Total Med (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.05x** | 0.06 | 1.10 | 0.62x | 0.04 (1 / 19) | 1.06 (0 / 0) | 0.082 (28) | 0.133 (19) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.07x** | 0.49 | 6.79 | 0.72x | 0.46 (2 / 72) | 6.75 (14 / 44) | 0.102 (30) | 0.142 (18) |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.17x** | 0.22 | 1.29 | 0.70x | 0.15 (1 / 26) | 1.25 (0 / 0) | 0.110 (33) | 0.157 (21) |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.10x** | 0.17 | 1.71 | 0.84x | 0.12 (1 / 40) | 1.65 (0 / 0) | 0.240 (58) | 0.284 (40) |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.19x** | 0.42 | 1.99 | 0.93x | 0.32 (1 / 51) | 1.91 (0 / 0) | 0.373 (80) | 0.400 (63) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.31x** | 3.74 | 10.80 | 0.87x | 3.63 (43 / 483) | 10.76 (28 / 146) | 0.154 (38) | 0.177 (24) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.32x** | 16.49 | 75.07 | 0.99x | 16.42 (196 / 2289) | 75.00 (216 / 1352) | 0.274 (65) | 0.276 (54) |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.10x** | 0.55 | 5.95 | 0.98x | 0.42 (1 / 75) | 5.82 (0 / 0) | 0.646 (91) | 0.661 (94) |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.26x** | 0.98 | 3.77 | 1.46x | 0.78 (1 / 98) | 3.55 (0 / 0) | 1.587 (244) | 1.088 (149) |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **0.46x** | 8.04 | 23.91 | 0.73x | 7.11 (2 / 377) | 22.72 (0 / 0) | 2.571 (224) | 3.529 (319) |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **0.23x** | 34.15 | 235.78 | 2.23x | 29.35 (1 / 2439) | 231.83 (0 / 0) | 75.872 (1900) | 34.020 (3128) |
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
| `t3_mkp_200` | $200 \times 8$ | **0.56x** | $23.09 \pm 12.65$ | $37.81 \pm 17.86$ | 23.38 (250 / 2860) | 46.11 (154 / 1145) | 107.70 (1135 / 16) | 216.09 (5000, `NodeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **0.44x** | $7.20 \pm 7.86$ | $12.15 \pm 9.25$ | 5.60 (1 / 20) | 8.99 (0 / 8) | 14.58 (2 / 4) | 5.64 (118) |
| `t2_binpack_44` | $44 \times 14$ | **0.17x** | $1.26 \pm 0.88$ | $6.43 \pm 2.06$ | 0.96 (2 / 43) | 5.93 (0 / 8) | 1.41 (5 / 6) | 9.68 (772) |
| `t3_cflp_300` | $300 \times 34$ | **0.13x** | $8.22 \pm 5.55$ | $52.82 \pm 11.90$ | 6.99 (1 / 119) | 47.66 (18 / 483) | 18.74 (54 / 34) | 8.88 (116) |
| `t3_lotsizing_504` | $504 \times 336$ | **0.70x** | $69.05 \pm 44.96$ | $88.41 \pm 55.18$ | 78.00 (2 / 1351) | 67.30 (10 / 377) | 114.52 (376 / 206) | 250.03 (2914, `TimeLimit`) |
| `t3_sched_1000` | $1000 \times 100$ | **0.15x** | $9.78 \pm 5.53$ | $60.00 \pm 5.12$ | 7.27 (31 / 917) | 57.70 (0 / 76) | 26.84 (87 / 53) | 250.08 (1797, `TimeLimit`) |
<!-- END GENERATED:NON_MAD_CBC_GAP_TABLE -->

<!-- BEGIN GENERATED:NON_MAD_CBC_NOTES -->
- **Bin-packing tail behavior on `t2_binpack_44`**: pure-Go `BackendGo` ($1.26 \pm 0.88$ ms mean, 0.98 ms median) leads in-process `BackendCBC` ($6.43 \pm 2.06$ ms mean, 5.96 ms median) with a **5.92x** `BackendCBC / BackendGo` geometric-mean speedup (`Go / CBC = 0.17x`), running slower on 0 of 11 trials (peaking at 3.27 ms vs. 11.54 ms), with `CP5 +Presolve` at 9.68 ms vs. `CP6 +Cuts` at 1.41 ms.
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

# 11. Lean 4 Prove Lane build (78 jobs), arm64 + amd64 ELF verification, CLI/JSONL checks, and 40 default-build Fuzz corpora
./scripts/verify_formal.sh
PYTHONDONTWRITEBYTECODE=1 python3 scripts/extract_elf_track5.py --verify
PYTHONDONTWRITEBYTECODE=1 python3 scripts/extract_amd64_track5e.py --verify

# 12. Optional on-demand target-silicon verification (ephemeral GCE n4-standard-4 AVX-512 + linux/arm64)
./scripts/verify_target_silicon.sh --dry-run
./scripts/verify_target_silicon.sh
```

Running `go test ./...` also executes `TestDocumentationAndBenchmarkSync`, which validates corpus dimensions, parity, symbol identifiers, provenance, formal inventory blocks, and every generated table across `README.md`, `docs/index.md`, `docs/benchmarks.md`, `docs/formal-verification.md`, and `bench/results/final_report.md`.

<!-- BEGIN GENERATED:BENCHMARKS_FORMAL_SUMMARY -->
Automated formal inventory across the workspace covers `24` Lean files (`53,872` LOC: `1,325` authored `theorem` and `lemma` declarations, `5,876` kernel `TheoremInfo` entries, and `220` axiom locks), `5` Go certification files (`18,489` LOC) paired with `30` formal and certified test files (`34,044` LOC), and `48` `Fuzz*` targets (`40` default-build across `./...` plus `8` `-tags=cbc`, matching [`scripts/gcp-fuzz/targets.tsv`](../scripts/gcp-fuzz/targets.tsv)).
<!-- END GENERATED:BENCHMARKS_FORMAL_SUMMARY -->
