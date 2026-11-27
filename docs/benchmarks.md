# Benchmark Methodology and Corpus Results

The benchmark protocol below evaluates pure-Go `goblin` (`BackendGo`, zero `cgo` by default), Python `PuLP` driving the `COIN-OR CBC` (`c1c171ec`) command-line binary, and `goblin (CBC)` (`BackendCBC`, built with `-tags cbc`), which calls the same `c1c171ec` CBC/CLP stack in-process through `cgo`. Every timing and binary-size measurement in this document comes from a generated block rendered by [`bench/docgen.go`](../bench/docgen.go) from the JSON telemetry in `bench/results/`; the prose contains no hardcoded timings. The test [`doc_sync_test.go`](../doc_sync_test.go) fails `go test ./...` when a block drifts from the telemetry, when a parity check fails, or when any of the ten telemetry files lacks uniform git, source-fingerprint, and host-kernel provenance.

---

## 1. Environment, Provenance, and Protocol

```bash
go run ./cmd/goblin-bench -check-docs
```

- **CPU**: `13th Gen Intel(R) Core(TM) i9-13900K` (`x86_64`, 30 logical CPUs exposed by `lscpu`, AVX2 + FMA).
- **Default build**: `go build` with the pure-Go kernels (`BackendGo`). All headline timings use that build.
- **Experimental build**: `GOEXPERIMENT=simd`, compared to the default build only in section 5.

<!-- BEGIN GENERATED:PROVENANCE -->
- **Unified telemetry source**: fingerprint `06fa79b9f6ebed61` (working tree on top of commit `91f68c4c7f9073819337d0bc87b1acaaee4638ae`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 7.02 ms median on a 1-variable MILP and 7.03 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
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
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **399.24x** [157.83x-1009.86x] | 399.24x | 0.05x | 10.5 (0.9) | 159.1 (14.9) | 1206.6 (112.4) | 672.24x [37.46x-2577.67x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **20.91x** [12.23x-35.76x] | 20.91x | 0.17x | 576.2 (37.8) | 1817.4 (164.3) | 4829.7 (464.9) | 25.36x [1.37x-165.07x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **9.32x** [4.48x-19.40x] | 9.32x | 0.28x | 963.4 (186.8) | 2661.7 (539.1) | 6623.7 (1143.7) | 8.80x [1.03x-71.61x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **12.99x** [7.49x-22.52x] | 14.41x | 0.21x | 1169.0 (164.7) | 3072.1 (403.2) | 7356.2 (1064.2) | 12.79x [1.37x-165.07x] |
| Tier 3 non-MAD MILPs | 4 (20) | **7.39x** [1.27x-42.90x] | 7.39x | 0.29x | 603.0 (127.8) | 1318.1 (244.3) | 2987.3 (639.9) | 5.45x [1.45x-71.61x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **24.61x** [11.31x-53.56x] | 32.46x | 0.20x | 372.6 (60.1) | 1420.2 (301.5) | 4212.6 (554.8) | 39.32x [1.03x-159.69x] |
| Random-weight MAD (negative controls) | 3 (33) | **16.38x** [0.78x-345.09x] | 16.38x | 0.19x | 337.3 (22.6) | 1010.7 (101.0) | 2396.6 (262.7) | 15.59x [1.37x-120.08x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **53.79x** [21.86x-132.35x] | 66.23x | 0.11x | 1177.5 (165.4) | 3218.0 (416.9) | 8447.5 (1166.3) | 46.44x [1.37x-2577.67x] |
| All 31 synthetic corpus families | 31 (293) | **43.96x** [22.13x-87.33x] | 56.67x | 0.13x | 1550.2 (225.5) | 4638.2 (718.3) | 12660.0 (1721.1) | 42.46x [1.03x-2577.67x] |
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
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **62.6x** [38.7x-159.7x] | 0.19 | 10.30 | 0.09 | 10.06 | 0.096 | 0.243 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **66.6x** [40.6x-120.1x] | 0.52 | 36.17 | 0.49 | 35.93 | 0.027 | 0.248 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **53.0x** [40.9x-94.5x] | 0.26 | 12.86 | 0.17 | 12.38 | 0.078 | 0.407 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **70.2x** [44.9x-94.0x] | 0.19 | 13.36 | 0.14 | 12.82 | 0.051 | 0.588 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **30.2x** [26.6x-34.1x] | 0.47 | 14.39 | 0.35 | 13.68 | 0.122 | 0.731 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **9.7x** [3.5x-25.3x] | 3.22 | 49.80 | 3.18 | 49.47 | 0.042 | 0.326 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **6.8x** [1.4x-29.0x] | 18.88 | 176.76 | 18.78 | 176.29 | 0.066 | 0.461 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **25.9x** [20.7x-37.9x] | 0.84 | 19.53 | 0.57 | 18.40 | 0.268 | 1.129 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **14.6x** [9.3x-17.6x] | 1.15 | 19.87 | 0.91 | 18.05 | 0.361 | 1.838 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **5.7x** [1.0x-14.1x] | 8.89 | 65.96 | 7.79 | 58.02 | 1.115 | 8.820 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **8.9x** [4.3x-24.0x] | 48.15 | 398.48 | 42.78 | 361.95 | 5.632 | 36.711 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Cross-Instance Latency Dispersion Across Randomized Instances (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.18 \pm 0.07$ ($\pm 0.05$) | $10.48 \pm 0.75$ ($\pm 0.50$) | $0.12 \pm 0.03$ ($\pm 0.02$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.54 \pm 0.26$ ($\pm 0.17$) | $33.05 \pm 10.01$ ($\pm 6.72$) | $0.72 \pm 0.32$ ($\pm 0.22$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.25 \pm 0.03$ ($\pm 0.02$) | $13.35 \pm 1.90$ ($\pm 1.28$) | $0.31 \pm 0.06$ ($\pm 0.04$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.21 \pm 0.04$ ($\pm 0.03$) | $14.35 \pm 1.80$ ($\pm 1.21$) | $0.24 \pm 0.04$ ($\pm 0.03$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.47 \pm 0.04$ ($\pm 0.03$) | $14.19 \pm 0.82$ ($\pm 0.55$) | $0.61 \pm 0.06$ ($\pm 0.04$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $4.59 \pm 3.73$ ($\pm 2.50$) | $38.12 \pm 18.23$ ($\pm 12.24$) | $5.34 \pm 4.02$ ($\pm 2.70$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $25.54 \pm 26.50$ ($\pm 17.80$) | $146.70 \pm 67.17$ ($\pm 45.12$) | $33.20 \pm 31.33$ ($\pm 21.04$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.77 \pm 0.16$ ($\pm 0.20$) | $19.81 \pm 2.46$ ($\pm 3.06$) | $0.96 \pm 0.15$ ($\pm 0.19$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.40 \pm 0.43$ ($\pm 0.53$) | $19.93 \pm 0.19$ ($\pm 0.23$) | $1.52 \pm 0.54$ ($\pm 0.67$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $18.83 \pm 24.21$ ($\pm 30.05$) | $67.21 \pm 3.91$ ($\pm 4.85$) | $22.94 \pm 28.99$ ($\pm 35.99$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $51.07 \pm 13.83$ ($\pm 17.17$) | $620.33 \pm 654.41$ ($\pm 812.43$) | $56.52 \pm 16.86$ ($\pm 20.93$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | **0.09** (1 / 0) | 0.04 (1 / 0) | 0.06 (1) | 0.58 (4) | 0.088 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.49** (2 / 0) | 0.60 (12 / 0) | 0.59 (12) | 0.87 (109) | 0.113 | 4601.00 | 4257.00 |
| `t2_mad_g2_i8` | **0.17** (1 / 0) | 0.92 (8 / 0) | 0.82 (8) | 0.81 (9) | 0.109 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.14** (1 / 0) | 0.14 (2 / 0) | 0.14 (2) | 2.94 (13) | 0.261 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.35** (1 / 0) | 3.74 (22 / 0) | 3.58 (22) | 5.67 (23) | 0.382 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.18** (43 / 0) | 3.45 (112 / 0) | 3.26 (112) | 4.11 (45) | 0.170 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **18.78** (196 / 0) | 33.92 (188 / 0) | 34.92 (188) | 91.27 (682) | 0.304 | 7720.00 | 6060.00 |
| `t3_mad_g5_i20` | **0.57** (1 / 0) | 3.50 (17 / 0) | 3.10 (17) | 0.55 (1) | 0.706 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.91** (1 / 0) | 10.75 (58 / 0) | 8.99 (58) | 15.22 (74) | 1.708 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.79** (2 / 0) | 23.86 (25 / 0) | 20.95 (25) | 250.34 (153, `TimeLimit`) | 2.822 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **42.78** (1 / 0) | 37.46 (2 / 0) | 42.93 (2) | 251.92 (5, `TimeLimit`) | 81.818 | 208000.00 | 208000.00 |
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
| `t1_std_lp` | 3 | 3 | **788.9x** [403.7x-1165.3x] | 0.01 | 7.13 | 0.00 | 7.06 | 0.004 | 0.068 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **1040.3x** [711.9x-1583.2x] | 0.01 | 7.43 | 0.00 | 7.37 | 0.004 | 0.068 | `OK` |
| `t1_std_mip` | 3 | 3 | **881.3x** [655.5x-1076.2x] | 0.01 | 8.57 | 0.00 | 8.48 | 0.004 | 0.072 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **1035.6x** [810.2x-1233.2x] | 0.01 | 8.59 | 0.00 | 8.52 | 0.004 | 0.079 | `OK` |
| `t1_whiskas` | 6 | 5 | **572.4x** [403.0x-710.3x] | 0.01 | 8.19 | 0.01 | 8.09 | 0.007 | 0.103 | `OK` |
| `t1_furniture` | 2 | 3 | **260.1x** [71.2x-1060.0x] | 0.03 | 8.74 | 0.02 | 8.67 | 0.006 | 0.066 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **93.5x** [37.5x-350.0x] | 0.12 | 10.51 | 0.11 | 10.40 | 0.009 | 0.105 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **2367.8x** [2024.6x-2577.7x] | 0.00 | 6.78 | 0.00 | 6.74 | 0.003 | 0.055 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **62.6x** [38.7x-159.7x] | 0.19 | 10.30 | 0.09 | 10.06 | 0.096 | 0.243 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **66.6x** [40.6x-120.1x] | 0.52 | 36.17 | 0.49 | 35.93 | 0.027 | 0.248 | `OK` |
| `t2_mkp_30` | 30 | 6 | **9.8x** [4.9x-14.4x] | 2.78 | 30.69 | 2.76 | 30.51 | 0.028 | 0.190 | `OK` |
| `t2_mkp_50` | 50 | 8 | **5.0x** [2.5x-29.8x] | 5.38 | 19.06 | 5.35 | 18.77 | 0.034 | 0.272 | `OK` |
| `t2_cflp_36` | 36 | 11 | **57.9x** [24.4x-165.1x] | 0.19 | 10.92 | 0.17 | 10.73 | 0.019 | 0.194 | `OK` |
| `t2_cflp_49` | 49 | 13 | **33.7x** [22.5x-55.5x] | 0.37 | 11.79 | 0.35 | 11.56 | 0.026 | 0.233 | `OK` |
| `t2_gap_45` | 45 | 13 | **19.5x** [4.6x-67.4x] | 0.96 | 15.82 | 0.93 | 15.60 | 0.024 | 0.219 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **13.3x** [6.0x-33.5x] | 3.85 | 80.86 | 3.82 | 80.59 | 0.036 | 0.272 | `OK` |
| `t2_binpack_44` | 44 | 14 | **14.4x** [5.8x-28.7x] | 1.06 | 15.65 | 1.04 | 15.43 | 0.022 | 0.222 | `OK` |
| `t2_setcover_50` | 50 | 20 | **54.8x** [7.1x-145.1x] | 0.20 | 12.95 | 0.15 | 12.62 | 0.035 | 0.337 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **53.0x** [40.9x-94.5x] | 0.26 | 12.86 | 0.17 | 12.38 | 0.078 | 0.407 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **70.2x** [44.9x-94.0x] | 0.19 | 13.36 | 0.14 | 12.82 | 0.051 | 0.588 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **30.2x** [26.6x-34.1x] | 0.47 | 14.39 | 0.35 | 13.68 | 0.122 | 0.731 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.7x** [3.5x-25.3x] | 3.22 | 49.80 | 3.18 | 49.47 | 0.042 | 0.326 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **6.8x** [1.4x-29.0x] | 18.88 | 176.76 | 18.78 | 176.29 | 0.066 | 0.461 | `OK` |
| `t3_mkp_200` | 200 | 8 | **4.3x** [3.2x-5.7x] | 25.52 | 123.20 | 25.38 | 122.56 | 0.085 | 0.639 | `OK` |
| `t3_cflp_300` | 300 | 34 | **32.3x** [8.0x-71.6x] | 6.97 | 264.92 | 6.85 | 263.96 | 0.114 | 0.972 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **2.5x** [1.4x-4.4x] | 86.74 | 165.84 | 86.47 | 163.87 | 0.243 | 1.995 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **8.6x** [3.9x-13.3x] | 8.56 | 85.94 | 8.22 | 83.09 | 0.357 | 2.934 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **25.9x** [20.7x-37.9x] | 0.84 | 19.53 | 0.57 | 18.40 | 0.268 | 1.129 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **14.6x** [9.3x-17.6x] | 1.15 | 19.87 | 0.91 | 18.05 | 0.361 | 1.838 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.7x** [1.0x-14.1x] | 8.89 | 65.96 | 7.79 | 58.02 | 1.115 | 8.820 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **8.9x** [4.3x-24.0x] | 48.15 | 398.48 | 42.78 | 361.95 | 5.632 | 36.711 | `OK` |
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
- **Instances where AVX2 can run**: 0 faster, 19 slower, 4 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 1.23x [0.93x-1.63x] | 0.004 | 0.003 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 0.89x [0.72x-1.10x] | 0.003 | 0.004 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.59x [0.46x-0.75x] | 0.005 | 0.008 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.66x [0.60x-0.73x] | 0.004 | 0.007 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.81x [0.71x-0.93x] | 0.006 | 0.008 | 11 |
| `t1_furniture` | 5 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.82x [0.73x-0.94x] | 0.019 | 0.025 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source | 0.91x [0.81x-1.03x] | 0.108 | 0.126 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.74x [0.69x-0.79x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 1.26x [0.85x-1.86x] | 0.090 | 0.066 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | SIMD slower | 0.75x [0.72x-0.79x] | 0.491 | 0.651 | 11 |
| `t2_mkp_30` | 36 | possible | SIMD slower | 0.92x [0.88x-0.96x] | 2.760 | 2.892 | 11 |
| `t2_mkp_50` | 58 | possible | SIMD slower | 0.84x [0.81x-0.88x] | 5.348 | 5.728 | 11 |
| `t2_cflp_36` | 47 | possible | SIMD slower | 0.64x [0.62x-0.66x] | 0.171 | 0.257 | 11 |
| `t2_cflp_49` | 62 | possible | SIMD slower | 0.58x [0.46x-0.74x] | 0.346 | 0.589 | 11 |
| `t2_gap_45` | 58 | possible | SIMD slower | 0.88x [0.84x-0.91x] | 0.931 | 1.105 | 11 |
| `t2_lotsizing_48` | 80 | possible | SIMD slower | 0.89x [0.84x-0.95x] | 3.819 | 4.542 | 11 |
| `t2_binpack_44` | 58 | possible | no detectable effect | 0.99x [0.98x-1.00x] | 1.036 | 1.052 | 11 |
| `t2_setcover_50` | 70 | possible | SIMD slower | 0.81x [0.77x-0.85x] | 0.154 | 0.202 | 11 |
| `t2_mad_g2_i8` | 72 | possible | SIMD slower | 0.78x [0.68x-0.90x] | 0.169 | 0.242 | 11 |
| `t2_mad_g3_i9` | 108 | possible | SIMD slower | 0.89x [0.83x-0.95x] | 0.135 | 0.164 | 11 |
| `t2_mad_g4_i12` | 156 | possible | SIMD slower | 0.81x [0.73x-0.89x] | 0.347 | 0.430 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | SIMD slower | 0.84x [0.80x-0.87x] | 3.176 | 3.881 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | SIMD slower | 0.72x [0.67x-0.79x] | 18.780 | 21.374 | 11 |
| `t3_mkp_200` | 208 | possible | SIMD slower | 0.68x [0.57x-0.82x] | 25.381 | 36.639 | 5 |
| `t3_cflp_300` | 334 | possible | SIMD slower | 0.88x [0.79x-0.98x] | 6.845 | 7.249 | 5 |
| `t3_lotsizing_504` | 840 | possible | SIMD slower | 0.76x [0.66x-0.87x] | 86.471 | 108.840 | 5 |
| `t3_sched_1000` | 1100 | possible | SIMD slower | 0.82x [0.77x-0.87x] | 8.223 | 10.412 | 5 |
| `t3_mad_g5_i20` | 240 | possible | SIMD slower | 0.90x [0.84x-0.96x] | 0.567 | 0.591 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 0.96x [0.72x-1.28x] | 0.912 | 1.075 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | SIMD slower | 0.79x [0.71x-0.88x] | 7.793 | 10.475 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | no detectable effect | 0.92x [0.74x-1.16x] | 42.777 | 44.276 | 5 |
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
| `t1_whiskas` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.004 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.003 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.11** (1 / 8) | 0.14 (2 / 8) | 0.18 (18) | 0.26 (35) | 0.009 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.09** (1 / 0) | 0.04 (1 / 0) | 0.06 (1) | 0.58 (4) | 0.088 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.49** (2 / 0) | 0.60 (12 / 0) | 0.59 (12) | 0.87 (109) | 0.113 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.76** (5 / 9) | 3.44 (40 / 9) | 1.27 (51) | 1.11 (61) | 0.028 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **5.35** (1 / 4) | 16.52 (2 / 4) | 6.17 (118) | 5.61 (111) | 0.049 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.17** (1 / 2) | 0.30 (3 / 2) | 0.21 (7) | 0.20 (7) | 0.030 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.35** (1 / 10) | 0.66 (4 / 10) | 0.34 (9) | 0.31 (9) | 0.038 | 909.02 | 827.30 |
| `t2_gap_45` | **0.93** (1 / 9) | 0.44 (1 / 9) | 1.92 (41) | 2.35 (60) | 0.031 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **3.82** (2 / 37) | 7.70 (25 / 37) | 3.83 (162) | 4.02 (199) | 0.067 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **1.04** (2 / 6) | 1.57 (5 / 6) | 9.57 (772) | 31.71 (1941) | 0.030 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.15** (1 / 6) | 0.39 (2 / 6) | 0.72 (6) | 0.70 (6) | 0.032 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.17** (1 / 0) | 0.92 (8 / 0) | 0.82 (8) | 0.81 (9) | 0.109 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.14** (1 / 0) | 0.14 (2 / 0) | 0.14 (2) | 2.94 (13) | 0.261 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.35** (1 / 0) | 3.74 (22 / 0) | 3.58 (22) | 5.67 (23) | 0.382 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.18** (43 / 0) | 3.45 (112 / 0) | 3.26 (112) | 4.11 (45) | 0.170 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **18.78** (196 / 0) | 33.92 (188 / 0) | 34.92 (188) | 91.27 (682) | 0.304 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **25.38** (250 / 14) | 117.86 (1135 / 16) | 241.71 (5000, `NodeLimit`) | 225.12 (5000, `NodeLimit`) | 0.138 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **6.85** (1 / 28) | 20.37 (54 / 34) | 10.37 (116) | 9.87 (118) | 0.187 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **86.47** (2 / 206) | 126.38 (376 / 206) | 250.02 (2652, `TimeLimit`) | 250.07 (2737, `TimeLimit`) | 0.769 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **8.22** (31 / 55) | 29.66 (87 / 53) | 250.02 (1613, `TimeLimit`) | 250.12 (1633, `TimeLimit`) | 0.235 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.57** (1 / 0) | 3.50 (17 / 0) | 3.10 (17) | 0.55 (1) | 0.706 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.91** (1 / 0) | 10.75 (58 / 0) | 8.99 (58) | 15.22 (74) | 1.708 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.79** (2 / 0) | 23.86 (25 / 0) | 20.95 (25) | 250.34 (153, `TimeLimit`) | 2.822 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **42.78** (1 / 0) | 37.46 (2 / 0) | 42.93 (2) | 251.92 (5, `TimeLimit`) | 81.818 | 208000.00 | 208000.00 |
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
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.46x** [1.14x-1.87x] | 1.46x | 4.26x | 0.232 (0.0206) | 0.333 (0.0307) | 1.39x [0.79x-5.79x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **2.99x** [1.87x-4.80x] | 2.99x | 3.00x | 9.534 (0.851) | 34.300 (3.095) | 2.65x [1.14x-15.60x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.82x** [1.86x-4.28x] | 2.82x | 3.64x | 251.283 (50.051) | 1003.123 (202.872) | 2.55x [1.50x-6.29x] |
| Square families ($m = n$) | 16 (146) | **2.71x** [1.98x-3.72x] | 2.66x | 3.72x | 256.008 (49.991) | 1026.277 (203.922) | 2.20x [0.89x-15.60x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.77x** [1.25x-2.50x] | 1.65x | 2.99x | 5.041 (0.931) | 11.479 (2.075) | 1.89x [0.79x-2.69x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.25x** [1.12x-4.52x] | 2.37x | 2.67x | 37.112 (6.544) | 67.773 (11.338) | 1.80x [0.89x-15.60x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.41x** [1.88x-3.10x] | 2.34x | 3.51x | 261.049 (50.923) | 1037.756 (205.998) | 2.05x [0.79x-15.60x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 7.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **2.12x** | 0.0005 | 0.0011 | 0.0006 | 0.0147 | $0.0006 \pm 0.0001$ | $0.0013 \pm 0.0006$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.26x** | 0.0019 | 0.0026 | 0.0023 | 0.0106 | $0.0020 \pm 0.0004$ | $0.0025 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.53x** | 0.0039 | 0.0065 | 0.0058 | 0.0159 | $0.0042 \pm 0.0013$ | $0.0063 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.15x** | 0.0039 | 0.0042 | 0.0052 | 0.0149 | $0.0036 \pm 0.0007$ | $0.0041 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.20x** | 0.0036 | 0.0047 | 0.0053 | 0.0161 | $0.0040 \pm 0.0012$ | $0.0047 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.71x** | 0.0067 | 0.0117 | 0.0116 | 0.0348 | $0.0068 \pm 0.0018$ | $0.0114 \pm 0.0019$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.74x** | 0.0299 | 0.0755 | 0.0362 | 0.103 | $0.0276 \pm 0.0067$ | $0.0740 \pm 0.0114$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.30x** | 0.0561 | 0.185 | 0.0722 | 0.238 | $0.0571 \pm 0.0125$ | $0.187 \pm 0.0266$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.06x** | 0.0500 | 0.0978 | 0.0758 | 0.147 | $0.0483 \pm 0.0038$ | $0.100 \pm 0.0144$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.54x** | 0.150 | 0.228 | 0.158 | 0.268 | $0.160 \pm 0.0503$ | $0.242 \pm 0.0454$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **12.24x** | 0.0631 | 0.713 | 0.0858 | 0.792 | $0.0593 \pm 0.0089$ | $0.719 \pm 0.0244$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.95x** | 0.0370 | 0.0740 | 0.0632 | 0.152 | $0.0389 \pm 0.0102$ | $0.0745 \pm 0.0078$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **1.99x** | 0.0411 | 0.0838 | 0.0645 | 0.162 | $0.0432 \pm 0.0097$ | $0.0847 \pm 0.0070$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.68x** | 0.0849 | 0.304 | 0.110 | 0.383 | $0.0847 \pm 0.0085$ | $0.312 \pm 0.0378$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.84x** | 0.339 | 1.335 | 0.453 | 1.775 | $0.347 \pm 0.0494$ | $1.325 \pm 0.125$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.08x** | 1.264 | 5.479 | 1.727 | 8.435 | $1.381 \pm 0.225$ | $5.591 \pm 0.380$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.71x** | 0.910 | 1.563 | 1.514 | 4.618 | $0.918 \pm 0.0515$ | $1.570 \pm 0.0598$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.66x** | 5.360 | 8.718 | 5.940 | 9.697 | $5.891 \pm 2.170$ | $9.611 \pm 2.784$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.48x** | 0.373 | 0.873 | 0.763 | 4.032 | $0.359 \pm 0.0262$ | $0.890 \pm 0.0607$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.30x** | 0.473 | 1.036 | 1.003 | 4.270 | $0.452 \pm 0.0345$ | $1.037 \pm 0.0429$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.92x** | 6.309 | 30.356 | 7.906 | 39.481 | $6.141 \pm 0.479$ | $30.245 \pm 2.595$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.35x** | 35.362 | 154.846 | 42.551 | 203.602 | $35.115 \pm 5.075$ | $151.681 \pm 10.062$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 7.3 LSAP Scalar vs. `GOEXPERIMENT=simd`

In [`assign/solver.go`](../assign/solver.go), [`simd.AllAbsLE64With`](../internal/simd/kernels_amd64.go) and [`simd.ValidateAndNegate64With`](../internal/simd/assign_amd64.go) validate finite cost bounds via AVX2 checks whenever `len(cost) >= 32` (while the fused `colReduceValidateSquare` fast path skips `AllAbsLE64With` on finite square `Minimize` matrices and dispatches `ColReduceValidateRows64With` only for $n \ge 32$), [`simd.Transpose64With`](../internal/simd/assign_amd64.go) handles tall transposes with $\text{rows} \ge 8$ and $\text{cols} \ge 8$, [`simd.MinReducedSkip64With`](../internal/simd/assign_amd64.go) accelerates reduction-transfer slack scans in `preprocessSquareJV` ($n \ge 32$), [`simd.Min2ReducedRow64With`](../internal/simd/kernels_amd64.go) accelerates first/second reduced-minimum scans in `preprocessRectangularBV` and `augmentingRowReduction` at `minAVX2Min2Len = 256` (`minAVX512Min2Len = 128`), and [`simd.InitReducedRow64With`](../internal/simd/kernels_amd64.go) runs on a slice of length $\max(m, n) \ge 32$ at the start of each Dijkstra augmentation. All vector kernels clean upper `YMM` state via `StoreArray` + `ClearAVXUpperBits()` (`VZEROUPPER`) before scalar operations; exact per-family verdicts appear in the generated table below.

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` with no `>= 8x8` tall transpose and either fused square `Minimize` non-sparse path or `rows * cols < 32`, so no AVX2 kernel runs): 2 of 3 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32`, tall `rows > cols` with `>= 8x8`, or fallback `AllAbsLE64`/`ValidateAndNegate64` at `rows * cols >= 32`): 18 faster, 1 slower, 0 with no detectable effect, out of 19.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.09x [1.04x-1.15x] | 0.0005 | 0.0005 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.21x [1.18x-1.25x] | 0.0019 | 0.0016 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source | 1.11x [0.97x-1.27x] | 0.0039 | 0.0038 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | SIMD faster | 1.09x [1.05x-1.13x] | 0.0039 | 0.0035 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | SIMD faster | 1.15x [1.08x-1.22x] | 0.0036 | 0.0034 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | SIMD slower | 0.97x [0.95x-1.00x] | 0.0067 | 0.0066 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.16x [1.04x-1.30x] | 0.0299 | 0.0264 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.29x [1.24x-1.35x] | 0.0561 | 0.0433 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.72x [1.58x-1.88x] | 0.0500 | 0.0298 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.79x [1.69x-1.89x] | 0.150 | 0.0846 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.22x [1.17x-1.28x] | 0.0631 | 0.0501 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.24x [1.00x-1.54x] | 0.0370 | 0.0288 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.30x [1.08x-1.56x] | 0.0411 | 0.0310 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.35x [1.23x-1.47x] | 0.0849 | 0.0608 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.60x [1.50x-1.70x] | 0.339 | 0.212 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.76x [1.63x-1.89x] | 1.264 | 0.788 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.42x [2.10x-2.78x] | 0.910 | 0.364 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.40x [2.32x-2.48x] | 5.360 | 2.337 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.60x [1.40x-1.81x] | 0.373 | 0.232 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.48x [1.20x-1.81x] | 0.473 | 0.293 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.59x [1.51x-1.67x] | 6.309 | 3.954 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.45x [1.34x-1.56x] | 35.362 | 24.284 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

---

## 8. Production Binary Footprint and Linker Dead-Code Elimination

The `goblin-bench -binary-size` mode compiles four standalone `CGO_ENABLED=0` programs in an isolated temporary module against the live workspace across both default `go build` and `GOEXPERIMENT=simd`, saving compiled artifact sizes and `go tool nm -size` symbol counts to `bench/results/goblin_binary_size.json`. [`ValidateBinarySizeTelemetry`](../bench/docgen.go) rejects the telemetry bundle if `assign_default` links any `math/big`, `encoding/json`, `VIPR`, or `Certif` symbols, if `goblin_default` links any `encoding/json`, `VIPR`, or `Certif` symbols, or if any profile links `encoding/json`:

<!-- BEGIN GENERATED:BINARY_SIZE_TABLE -->
| Profile | Default Raw / Stripped (KiB) | `GOEXPERIMENT=simd` Raw / Stripped (KiB) | Stripped Delta vs. Default | `goblin` Text (KiB) | `math/big` Syms | `encoding/json` Syms | `VIPR` Syms | `Certif` Syms |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `assign` default (`LinearSum` + `SolveInto`) | 2822.8 / **1768.2** | 2858.1 / **1832.2** | baseline | 118.7 | `0` | `0` | `0` | `0` |
| `assign` certified (`+ assign.WithCertified(true)`) | 2942.9 / **1844.2** | 2980.7 / **1916.2** | +76.0 KiB (+4.3%) | 177.2 | `0` | `0` | `0` | `17` |
| `goblin` default (`Model.Solve`, `DefaultOptions`) | 5915.4 / **3776.2** | 5798.6 / **3796.2** | baseline | 1168.5 | `130` | `0` | `0` | `0` |
| `goblin` certified + VIPR (`+ WithCertified + WithVIPRTrace`) | 7353.0 / **4752.2** | 7215.7 / **4772.2** | +976.0 KiB (+25.8%) | 1743.7 | `159` | `0` | `220` | `20` |
<!-- END GENERATED:BINARY_SIZE_TABLE -->

---

## 9. Pure-Go vs. In-Process C++ CBC Performance Analysis

Comparing isolated unpresolved `Root LP` timings, paired trial ratios, and solver-core checkpoints (`CP5 +Presolve`, `CP6 +Cuts`, `CP7 Full`) between `BackendGo` and in-process `BackendCBC` (`c1c171ec`) removes Python MPS formatting and OS process-spawn overhead across all 31 synthetic families and highlights four structural trade-offs on large degenerate LPs and heavy-tail branch-and-cut models:

<!-- BEGIN GENERATED:CBC_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `BackendCBC / BackendGo` Total [Clustered 95% CI] | Equal-Family `BackendCBC / BackendGo` Solve [Clustered 95% CI] | `BackendGo` Trial (Med) Sum (ms) | `BackendCBC` Trial (Med) Sum (ms) | Per-Instance `CBC / Go` Total Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro-models (`N <= 26`) | 10 (110) | **20.62x** [8.37x-50.78x] | **39.40x** [12.16x-127.71x] | 10.5 (0.9) | 159.1 (14.9) | 15.95x [3.49x-136.79x] |
| Tier 2: Medium MILPs (`30 <= N <= 84`) | 13 (143) | **5.73x** [4.07x-8.06x] | **6.40x** [4.34x-9.42x] | 576.2 (37.8) | 1817.4 (164.3) | 5.76x [0.59x-45.05x] |
| Tier 3: Large MILPs (`145 <= N <= 4036`) | 8 (40) | **3.59x** [2.06x-6.25x] | **3.99x** [2.19x-7.25x] | 963.4 (186.8) | 2661.7 (539.1) | 3.57x [0.37x-14.49x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **4.81x** [3.20x-7.23x] | **5.03x** [3.25x-7.78x] | 1169.0 (164.7) | 3072.1 (403.2) | 5.20x [0.59x-45.05x] |
| All 31 synthetic families | 31 (293) | **7.67x** [5.11x-11.52x] | **10.18x** [6.03x-17.17x] | 1550.2 (225.5) | 4638.2 (718.3) | 6.87x [0.37x-136.79x] |
<!-- END GENERATED:CBC_SUMMARY_TABLE -->

### 9.1 MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families ($N = G(I + T + 3)$ variables, $M = G(2T + 3) + I$ constraints) separate solver behavior into three scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Paired `Go / CBC` | Go Total Med (ms) | CBC Total Med (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.14x** | 0.19 | 1.19 | 0.60x | 0.09 (1 / 19) | 1.15 (0 / 0) | 0.088 (28) | 0.146 (19) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.07x** | 0.52 | 7.79 | 0.69x | 0.49 (2 / 72) | 7.75 (14 / 44) | 0.113 (30) | 0.163 (18) |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.17x** | 0.26 | 1.47 | 0.60x | 0.17 (1 / 26) | 1.42 (0 / 0) | 0.109 (33) | 0.183 (21) |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.11x** | 0.19 | 1.83 | 0.83x | 0.14 (1 / 40) | 1.76 (0 / 0) | 0.261 (58) | 0.314 (40) |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.19x** | 0.47 | 2.12 | 0.79x | 0.35 (1 / 51) | 2.03 (0 / 0) | 0.382 (80) | 0.485 (63) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.31x** | 3.22 | 12.45 | 0.87x | 3.18 (43 / 483) | 12.39 (28 / 146) | 0.170 (38) | 0.195 (24) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.32x** | 18.88 | 80.72 | 1.00x | 18.78 (196 / 2289) | 80.63 (216 / 1352) | 0.304 (65) | 0.303 (54) |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.13x** | 0.84 | 6.18 | 1.02x | 0.57 (1 / 75) | 6.04 (0 / 0) | 0.706 (91) | 0.690 (94) |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.33x** | 1.15 | 4.09 | 1.51x | 0.91 (1 / 98) | 3.87 (0 / 0) | 1.708 (244) | 1.128 (149) |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **0.45x** | 8.89 | 26.67 | 0.73x | 7.79 (2 / 377) | 25.08 (0 / 0) | 2.822 (224) | 3.875 (319) |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **0.26x** | 48.15 | 257.91 | 2.18x | 42.78 (1 / 2439) | 253.28 (0 / 0) | 81.818 (1900) | 37.475 (3128) |
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
| `t3_mkp_200` | $200 \times 8$ | **0.54x** | $25.12 \pm 13.72$ | $42.82 \pm 20.23$ | 25.38 (250 / 2860) | 53.04 (154 / 1145) | 117.86 (1135 / 16) | 241.71 (5000, `NodeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **0.43x** | $7.65 \pm 8.25$ | $13.08 \pm 9.60$ | 5.35 (1 / 20) | 9.09 (0 / 8) | 16.52 (2 / 4) | 6.17 (118) |
| `t2_binpack_44` | $44 \times 14$ | **0.17x** | $1.35 \pm 0.90$ | $7.02 \pm 2.30$ | 1.04 (2 / 43) | 6.46 (0 / 8) | 1.57 (5 / 6) | 9.57 (772) |
| `t3_cflp_300` | $300 \times 34$ | **0.13x** | $8.86 \pm 6.06$ | $59.00 \pm 12.14$ | 6.85 (1 / 119) | 53.07 (18 / 483) | 20.37 (54 / 34) | 10.37 (116) |
| `t3_lotsizing_504` | $504 \times 336$ | **0.68x** | $75.31 \pm 48.97$ | $97.41 \pm 59.33$ | 86.47 (2 / 1351) | 74.02 (10 / 377) | 126.38 (376 / 206) | 250.02 (2652, `TimeLimit`) |
| `t3_sched_1000` | $1000 \times 100$ | **0.16x** | $11.31 \pm 6.59$ | $64.40 \pm 5.69$ | 8.22 (31 / 917) | 63.37 (0 / 76) | 29.66 (87 / 53) | 250.02 (1613, `TimeLimit`) |
<!-- END GENERATED:NON_MAD_CBC_GAP_TABLE -->

<!-- BEGIN GENERATED:NON_MAD_CBC_NOTES -->
- **Bin-packing tail behavior on `t2_binpack_44`**: pure-Go `BackendGo` ($1.35 \pm 0.90$ ms mean, 1.06 ms median) leads in-process `BackendCBC` ($7.02 \pm 2.30$ ms mean, 6.50 ms median) with a **6.02x** `BackendCBC / BackendGo` geometric-mean speedup (`Go / CBC = 0.17x`), running slower on 0 of 11 trials (peaking at 3.25 ms vs. 12.83 ms), with `CP5 +Presolve` at 9.57 ms vs. `CP6 +Cuts` at 1.57 ms.
<!-- END GENERATED:NON_MAD_CBC_NOTES -->
- **Bounded node budget on `coinor_capplan1`**: on the largest held-out public MILP in `testdata/public/manifest.json` (`coinor_capplan1`, 2,172 variables and 726 constraints), [`public_benchmarks_test.go`](../public_benchmarks_test.go) runs `BackendGo` under a bounded 25-node budget with cuts and heuristics disabled (`WithCuts(false)`, `WithHeuristics(false)`, `WithMaxNodes(25)`) to verify a valid global dual lower bound at `StatusNodeLimit`, whereas in-process `BackendCBC` solves the same instance to `StatusOptimal` without node, cut, or heuristic caps.

---

## 10. Reproducing Benchmarks and Syncing Documentation

Run all commands from the `goblin/` repository root on a quiet host so that all ten telemetry files record uniform `git_commit`, `source_fingerprint`, and `uname_release`:

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

# 6. Warm-state SciPy, goblin/assign, binary-size, and isolated combinatorial MAD benchmarks, pinned to CPU 2
taskset -c 2 python3 bench/scipy_assign_baseline.py --corpus-dir testdata/assign_corpus --out-json bench/results/scipy_assign_baseline.json
taskset -c 2 /tmp/goblin-bench-default -assign -out bench/results/goblin_assign_scalar.json
taskset -c 2 /tmp/goblin-bench-simd -assign -out bench/results/goblin_assign_simd.json
taskset -c 2 /tmp/goblin-bench-default -binary-size -out bench/results/goblin_binary_size.json
taskset -c 2 /tmp/goblin-bench-cbc -mad -out bench/results/goblin_mad.json
CBC_BIN=./third_party/Cbc/dist/bin/cbc taskset -c 2 python3 bench/python_mad_baseline.py --corpus-dir testdata/mad_corpus --goblin-bench /tmp/goblin-bench-default --out-json bench/results/python_mad_baseline.json
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

# 11. Lean 4 Prove Lane build (78 jobs), arm64 + amd64 ELF verification, CLI/JSONL checks, and 42 default-build Fuzz corpora
./scripts/verify_formal.sh
PYTHONDONTWRITEBYTECODE=1 python3 scripts/extract_elf_track5.py --verify
PYTHONDONTWRITEBYTECODE=1 python3 scripts/extract_amd64_track5e.py --verify

# 12. Optional on-demand target-silicon verification (ephemeral GCE n4-standard-4 AVX-512 + linux/arm64)
./scripts/verify_target_silicon.sh --dry-run
./scripts/verify_target_silicon.sh
```

Running `go test ./...` also executes `TestDocumentationAndBenchmarkSync`, which validates corpus dimensions, parity, symbol identifiers, provenance, formal inventory blocks, and every generated table across `README.md`, `docs/index.md`, `docs/benchmarks.md`, `docs/formal-verification.md`, and `bench/results/final_report.md`.

<!-- BEGIN GENERATED:BENCHMARKS_FORMAL_SUMMARY -->
Automated formal inventory across the workspace covers `24` Lean files (`53,872` LOC: `1,325` authored `theorem` and `lemma` declarations, `5,876` kernel `TheoremInfo` entries, and `220` axiom locks), `6` Go certification files (`20,615` LOC) paired with `31` formal and certified test files (`34,873` LOC), and `50` `Fuzz*` targets (`42` default-build across `./...` plus `8` `-tags=cbc`, matching [`scripts/gcp-fuzz/targets.tsv`](../scripts/gcp-fuzz/targets.tsv)).
<!-- END GENERATED:BENCHMARKS_FORMAL_SUMMARY -->

---

## 11. Combinatorial Group-Item Allocation Benchmarks (`Model.Solve` vs. Pure Branch-and-Cut)

The isolated 13-family combinatorial MAD corpus ([`bench/mad_corpus.go`](../bench/mad_corpus.go), manifest in `testdata/mad_corpus/manifest.json`) evaluates default algebraic `Model.Solve` (automatic structural detection routed to `internal/mad`, alongside internal `StrategyDP` and `StrategyBruteForce` breakdowns when DFS states are at most `MaxDFSStates = 250,000`) against 250 ms-capped pure Branch-and-Cut solves (`Model.Solve` with `WithCombinatorialSearch(false)` and `WithMaxNodes(0)` on `BackendGo`, in-process C++ `BackendCBC`, and end-to-end Python `PuLP + CBC` subprocess execution via [`bench/python_mad_baseline.py`](../bench/python_mad_baseline.py)) across 131 randomized trials. Section 3's 31-family LP/MILP suite explicitly passes `WithCombinatorialSearch(false)` so pure Branch-and-Cut telemetry remains un-obfuscated.

- **Corpus structure**: `pmad_base_g10_i2` defines the canonical 10-group, 2-item, 6-period PMAD base instance on Trial 0 (`Objective = 24137`, `Assignment = [6, 9]`), 8 PMAD families span $G \in \{6, 10, 20\}$, $I \in \{4..12\}$, and $\lambda \in \{0.25, 1.0\}$ (`RoundWeight = 6 * lambda`), and 4 `mad_comb_rand_*` families mirror `BuildMADRandom` weights with `DesignatedItems = [I-G..I-1]` and `RoundWeight = 1.0`.
- **Parity and containment**: `CheckMADParity` requires `1e-9` objective parity across default `Model.Solve`, internal `StrategyDP`, and `StrategyBruteForce`, and `[BestBound, Incumbent]` containment plus `1e-5` `StatusOptimal` parity across the 250 ms-capped MILP baselines (`BackendGo`, in-process `BackendCBC`, and Python `PuLP + CBC`).

<!-- BEGIN GENERATED:MAD_COMBINATORIAL_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Auto Trial (Med) Sum (ms) | DP Trial (Med) Sum (ms) | DFS Trial (Med) Sum (ms) | MILP Go Trial (Med) Sum (ms) [Opt] | In-Process CBC Trial (Med) Sum (ms) [Opt] | Python PuLP+CBC Trial (Med) Sum (ms) [Opt] | Equal-Family `MILP Go / Auto` [95% CI] | Equal-Family `CBC / Auto` [95% CI] | Equal-Family `Python / Auto` [95% CI] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| PMAD base (`G = 10, I = 2`) | 1 (11) | 0.410 (0.0371) | 0.106 (0.0096) | 1.049 (0.0952) | 919.3 (17.2) [8/11] | 765.9 (67.6) [11/11] | 2094.1 (245.4) [7/11] | **781.9x** | **1708.9x** | **4204.2x** |
| PMAD small & medium (`I <= 10`) | 6 (66) | 132.700 (11.157) | 129.826 (11.398) | 453.464 (40.054) [2/6] | 15843.8 (1500.8) [7/66] | 13190.2 (1200.6) [25/66] | 16966.0 (1541.9) [18/66] | **351.9x** [58.8x-2105.3x] | **266.5x** [63.6x-1116.5x] | **377.6x** [66.8x-2134.7x] |
| PMAD large (`I = 12`) | 2 (10) | 456.963 (83.890) | 439.485 (88.727) | skip (>250k) | 2502.8 (500.6) [0/10] | 2550.6 (510.2) [0/10] | 2796.5 (560.6) [0/10] | **6.0x** [0.0x-1048.4x] | **6.1x** [0.0x-961.1x] | **6.7x** [0.1x-723.6x] |
| Random-weight MAD controls (`I <= 12`) | 4 (44) | 7.102 (0.674) | 6.059 (0.581) | 404.910 (35.691) | 2457.3 (273.7) [38/44] | 4323.9 (413.5) [33/44] | 5730.0 (573.7) [33/44] | **113.1x** [22.8x-561.6x] | **541.8x** [252.9x-1160.7x] | **1034.4x** [397.9x-2689.0x] |
| **All 13 combinatorial MAD families** | 13 (131) | 597.175 (95.759) | 575.476 (100.715) | 859.423 (75.841) [7/13] | 21723.1 (2292.2) [53/131] | 20830.7 (2191.9) [69/131] | 27586.6 (2921.6) [58/131] | **141.1x** [43.2x-460.4x] | **214.0x** [67.6x-677.6x] | **333.3x** [89.1x-1246.3x] |
<!-- END GENERATED:MAD_COMBINATORIAL_SUMMARY_TABLE -->

<!-- BEGIN GENERATED:MAD_COMBINATORIAL_PIPELINE_TABLE -->
| Problem ID | $G \times I \times T$ | $\lambda$ | Auto Strategy | Auto Med (ms) | DP Med (ms) | DFS Med (ms) | MILP Go 250 ms | In-Process CBC 250 ms | Python PuLP+CBC 250 ms | Trial 0 Obj | Parity |
| :--- | :---: | ---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `pmad_base_g10_i2` | $10 \times 2 \times 6$ | 1.00 | `DP` | 0.0371 | 0.0096 | 0.0952 | 17.16 (8/11 opt) | 67.60 (`Optimal`) | 245.39 (7/11 opt) | 24137.0 | `OK` |
| `pmad_g6_i6_lam025` | $6 \times 6 \times 6$ | 0.25 | `DP` | 0.118 | 0.0931 | 29.289 | 250.04 (4/11 opt) | 121.90 (`Optimal`) | 220.34 (7/11 opt) | 13259.5 | `OK` |
| `pmad_g6_i10_lam10` | $6 \times 10 \times 6$ | 1.00 | `DP` | 3.963 | 4.142 | skip (>250k) | 250.12 (`TimeLimit`) | 252.52 (`TimeLimit`) | 274.18 (`TimeLimit`) | 14018.0 | `OK` |
| `pmad_g6_i12_lam025` | $6 \times 12 \times 6$ | 0.25 | `DP` | 26.430 | 29.123 | skip (>250k) | 250.15 (`TimeLimit`) | 252.35 (`TimeLimit`) | 268.80 (`TimeLimit`) | 9787.0 | `OK` |
| `pmad_g10_i4_lam025` | $10 \times 4 \times 6$ | 0.25 | `DP` | 0.0699 | 0.0389 | 10.766 | 250.12 (3/11 opt) | 65.12 (`Optimal`) | 189.94 (`Optimal`) | 23954.0 | `OK` |
| `pmad_g10_i8_lam10` | $10 \times 8 \times 6$ | 1.00 | `DP` | 0.803 | 0.772 | skip (>250k) | 250.10 (`TimeLimit`) | 252.77 (2/11 opt) | 282.59 (`TimeLimit`) | 21928.0 | `OK` |
| `pmad_g10_i10_lam025` | $10 \times 10 \times 6$ | 0.25 | `DP` | 5.727 | 5.942 | skip (>250k) | 250.22 (`TimeLimit`) | 252.96 (1/11 opt) | 283.33 (`TimeLimit`) | 22570.0 | `OK` |
| `pmad_g20_i6_lam10` | $20 \times 6 \times 6$ | 1.00 | `DP` | 0.477 | 0.410 | skip (>250k) | 250.22 (`TimeLimit`) | 255.31 (`TimeLimit`) | 291.53 (`TimeLimit`) | 43530.0 | `OK` |
| `pmad_g20_i12_lam10` | $20 \times 12 \times 6$ | 1.00 | `DP` | 57.460 | 59.604 | skip (>250k) | 250.45 (`TimeLimit`) | 257.86 (`TimeLimit`) | 291.78 (`TimeLimit`) | 38256.0 | `OK` |
| `mad_comb_rand_g2_i4` | $2 \times 4 \times 6$ | 0.00 | `BruteForce` | 0.0258 | 0.0056 | 0.0037 | 0.75 (`Optimal`) | 15.46 (`Optimal`) | 42.62 (`Optimal`) | 4601.0 | `OK` |
| `mad_comb_rand_g2_i8` | $2 \times 8 \times 6$ | 0.00 | `BruteForce` | 0.0475 | 0.0433 | 0.0261 | 3.78 (`Optimal`) | 16.21 (`Optimal`) | 60.05 (`Optimal`) | 5975.0 | `OK` |
| `mad_comb_rand_g3_i9` | $3 \times 9 \times 6$ | 0.00 | `DP` | 0.0850 | 0.0584 | 0.327 | 19.08 (`Optimal`) | 130.26 (`Optimal`) | 204.01 (10/11 opt) | 7720.0 | `OK` |
| `mad_comb_rand_g4_i12` | $4 \times 12 \times 6$ | 0.00 | `DP` | 0.516 | 0.473 | 35.334 | 250.04 (5/11 opt) | 251.54 (`TimeLimit`) | 267.06 (1/11 opt) | 7905.0 | `OK` |
<!-- END GENERATED:MAD_COMBINATORIAL_PIPELINE_TABLE -->
