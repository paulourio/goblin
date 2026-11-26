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
- **Unified telemetry source**: fingerprint `907d83c35a5be855` (commit `69bc7898a2e3f299552b742f6bce22deb3f724c0`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.48 ms median on a 1-variable MILP and 6.54 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
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
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **402.11x** [180.62x-895.25x] | 402.11x | 0.04x | 8.8 (0.8) | 153.0 (14.5) | 1098.1 (103.1) | 686.80x [40.42x-1803.14x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **19.49x** [11.70x-32.49x] | 19.49x | 0.18x | 552.4 (37.2) | 1750.7 (152.5) | 4442.1 (430.4) | 22.29x [1.36x-155.94x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **9.83x** [4.54x-21.29x] | 9.83x | 0.26x | 908.0 (175.4) | 2556.2 (519.0) | 6366.4 (1095.6) | 9.43x [1.04x-63.30x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **12.30x** [7.28x-20.77x] | 13.56x | 0.22x | 1132.9 (164.2) | 2961.0 (381.9) | 6954.5 (1013.0) | 12.98x [1.36x-155.94x] |
| Tier 3 non-MAD MILPs | 4 (20) | **7.23x** [1.26x-41.56x] | 7.23x | 0.30x | 589.7 (127.8) | 1268.9 (234.4) | 2880.5 (616.0) | 5.10x [1.48x-63.30x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **27.32x** [12.08x-61.79x] | 35.73x | 0.17x | 328.5 (48.5) | 1358.4 (290.7) | 3963.0 (522.9) | 44.77x [1.04x-140.93x] |
| Random-weight MAD (negative controls) | 3 (33) | **16.03x** [0.79x-324.29x] | 16.03x | 0.19x | 323.0 (22.7) | 993.1 (96.5) | 2297.7 (254.2) | 16.00x [1.36x-117.04x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **50.93x** [21.00x-123.55x] | 62.49x | 0.11x | 1140.7 (164.9) | 3101.4 (395.3) | 7943.6 (1106.2) | 44.43x [1.36x-1803.14x] |
| All 31 synthetic corpus families | 31 (293) | **43.37x** [22.10x-85.11x] | 55.31x | 0.13x | 1469.2 (213.4) | 4459.8 (686.0) | 11906.6 (1629.1) | 44.43x [1.04x-1803.14x] |
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
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **109.5x** [63.5x-140.9x] | 0.08 | 9.95 | 0.05 | 9.71 | 0.030 | 0.239 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **64.1x** [40.4x-117.0x] | 0.50 | 34.20 | 0.48 | 33.96 | 0.027 | 0.244 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **57.2x** [51.8x-64.4x] | 0.17 | 10.16 | 0.14 | 9.84 | 0.034 | 0.311 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **57.0x** [43.0x-67.9x] | 0.18 | 11.11 | 0.13 | 10.66 | 0.049 | 0.452 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **27.4x** [15.7x-37.2x] | 0.43 | 12.06 | 0.33 | 11.43 | 0.099 | 0.647 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **9.3x** [3.5x-24.4x] | 3.32 | 48.53 | 3.26 | 48.21 | 0.048 | 0.316 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **6.9x** [1.4x-29.1x] | 18.92 | 171.45 | 18.79 | 171.02 | 0.065 | 0.453 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **30.4x** [24.3x-37.7x] | 0.60 | 18.64 | 0.46 | 17.59 | 0.134 | 1.054 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **18.5x** [11.7x-25.6x] | 1.06 | 19.93 | 0.85 | 18.12 | 0.210 | 1.873 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **5.6x** [1.0x-13.4x] | 8.43 | 61.42 | 7.43 | 53.44 | 1.394 | 8.573 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **10.0x** [4.4x-23.8x] | 37.54 | 379.67 | 30.98 | 344.26 | 4.473 | 36.012 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Cross-Instance Latency Dispersion Across Randomized Instances (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.09 \pm 0.03$ ($\pm 0.02$) | $9.90 \pm 0.40$ ($\pm 0.27$) | $0.09 \pm 0.02$ ($\pm 0.02$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.52 \pm 0.24$ ($\pm 0.16$) | $30.91 \pm 9.65$ ($\pm 6.48$) | $0.52 \pm 0.24$ ($\pm 0.16$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.18 \pm 0.01$ ($\pm 0.01$) | $10.16 \pm 0.72$ ($\pm 0.48$) | $0.17 \pm 0.02$ ($\pm 0.01$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.20 \pm 0.03$ ($\pm 0.02$) | $11.05 \pm 0.35$ ($\pm 0.23$) | $0.20 \pm 0.04$ ($\pm 0.03$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.46 \pm 0.12$ ($\pm 0.08$) | $12.24 \pm 0.68$ ($\pm 0.45$) | $0.38 \pm 0.02$ ($\pm 0.01$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $4.47 \pm 3.61$ ($\pm 2.43$) | $36.18 \pm 17.21$ ($\pm 11.56$) | $4.38 \pm 3.44$ ($\pm 2.31$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $24.38 \pm 24.79$ ($\pm 16.65$) | $141.79 \pm 65.01$ ($\pm 43.67$) | $24.54 \pm 24.89$ ($\pm 16.72$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.60 \pm 0.03$ ($\pm 0.03$) | $18.29 \pm 2.61$ ($\pm 3.24$) | $0.60 \pm 0.03$ ($\pm 0.04$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.12 \pm 0.35$ ($\pm 0.43$) | $20.07 \pm 0.30$ ($\pm 0.37$) | $1.28 \pm 0.35$ ($\pm 0.44$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $17.99 \pm 22.98$ ($\pm 28.52$) | $64.40 \pm 4.43$ ($\pm 5.50$) | $18.19 \pm 22.87$ ($\pm 28.39$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $43.95 \pm 15.84$ ($\pm 19.67$) | $594.44 \pm 627.90$ ($\pm 779.52$) | $42.67 \pm 17.46$ ($\pm 21.68$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | **0.05** (1 / 0) | 0.06 (1 / 0) | 0.07 (1) | 0.62 (4) | 0.091 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.48** (2 / 0) | 0.59 (12 / 0) | 0.58 (12) | 0.82 (109) | 0.111 | 4601.00 | 4257.00 |
| `t2_mad_g2_i8` | **0.14** (1 / 0) | 0.93 (8 / 0) | 0.78 (8) | 0.78 (9) | 0.111 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.13** (1 / 0) | 0.14 (2 / 0) | 0.14 (2) | 3.27 (13) | 0.261 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.33** (1 / 0) | 3.53 (22 / 0) | 3.20 (22) | 5.63 (23) | 0.378 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.26** (43 / 0) | 3.10 (112 / 0) | 3.21 (112) | 3.96 (45) | 0.159 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **18.79** (196 / 0) | 33.16 (188 / 0) | 34.70 (188) | 97.46 (539) | 0.299 | 7720.00 | 6060.00 |
| `t3_mad_g5_i20` | **0.46** (1 / 0) | 3.56 (17 / 0) | 3.26 (17) | 0.53 (1) | 0.653 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.85** (1 / 0) | 9.17 (58 / 0) | 9.35 (58) | 14.50 (74) | 1.673 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.43** (2 / 0) | 22.54 (25 / 0) | 20.52 (25) | 250.30 (145, `TimeLimit`) | 2.645 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **30.98** (1 / 0) | 33.93 (2 / 0) | 32.19 (2) | 250.34 (6, `TimeLimit`) | 58.832 | 208000.00 | 208000.00 |
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
| `t1_std_lp` | 3 | 3 | **854.9x** [572.2x-1015.7x] | 0.01 | 6.81 | 0.00 | 6.75 | 0.004 | 0.064 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **1028.3x** [703.5x-1452.3x] | 0.01 | 6.83 | 0.00 | 6.76 | 0.003 | 0.066 | `OK` |
| `t1_std_mip` | 3 | 3 | **826.8x** [443.3x-1026.5x] | 0.01 | 7.49 | 0.00 | 7.43 | 0.003 | 0.055 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **925.8x** [755.2x-1041.2x] | 0.01 | 7.30 | 0.00 | 7.24 | 0.003 | 0.055 | `OK` |
| `t1_whiskas` | 6 | 5 | **581.5x** [408.9x-725.9x] | 0.01 | 6.64 | 0.01 | 6.56 | 0.005 | 0.078 | `OK` |
| `t1_furniture` | 2 | 3 | **299.8x** [77.7x-1161.5x] | 0.02 | 8.16 | 0.02 | 8.10 | 0.003 | 0.060 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **101.4x** [46.8x-326.5x] | 0.11 | 9.28 | 0.10 | 9.17 | 0.008 | 0.098 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **1323.6x** [730.6x-1803.1x] | 0.00 | 6.41 | 0.00 | 6.36 | 0.004 | 0.044 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **109.5x** [63.5x-140.9x] | 0.08 | 9.95 | 0.05 | 9.71 | 0.030 | 0.239 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **64.1x** [40.4x-117.0x] | 0.50 | 34.20 | 0.48 | 33.96 | 0.027 | 0.244 | `OK` |
| `t2_mkp_30` | 30 | 6 | **10.3x** [5.2x-14.7x] | 2.47 | 28.60 | 2.45 | 28.42 | 0.021 | 0.187 | `OK` |
| `t2_mkp_50` | 50 | 8 | **4.9x** [2.5x-27.1x] | 5.28 | 17.40 | 5.25 | 17.14 | 0.033 | 0.264 | `OK` |
| `t2_cflp_36` | 36 | 11 | **52.7x** [22.3x-155.9x] | 0.19 | 9.14 | 0.17 | 8.96 | 0.019 | 0.173 | `OK` |
| `t2_cflp_49` | 49 | 13 | **35.8x** [20.1x-94.6x] | 0.35 | 10.64 | 0.33 | 10.40 | 0.024 | 0.225 | `OK` |
| `t2_gap_45` | 45 | 13 | **18.3x** [4.5x-53.7x] | 0.90 | 15.27 | 0.88 | 15.05 | 0.026 | 0.220 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **11.8x** [5.2x-31.2x] | 3.69 | 70.29 | 3.66 | 70.04 | 0.031 | 0.251 | `OK` |
| `t2_binpack_44` | 44 | 14 | **13.0x** [5.0x-24.3x] | 1.04 | 14.07 | 1.01 | 13.87 | 0.023 | 0.213 | `OK` |
| `t2_setcover_50` | 50 | 20 | **38.2x** [6.3x-100.7x] | 0.24 | 11.68 | 0.17 | 11.37 | 0.064 | 0.319 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **57.2x** [51.8x-64.4x] | 0.17 | 10.16 | 0.14 | 9.84 | 0.034 | 0.311 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **57.0x** [43.0x-67.9x] | 0.18 | 11.11 | 0.13 | 10.66 | 0.049 | 0.452 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **27.4x** [15.7x-37.2x] | 0.43 | 12.06 | 0.33 | 11.43 | 0.099 | 0.647 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.3x** [3.5x-24.4x] | 3.32 | 48.53 | 3.26 | 48.21 | 0.048 | 0.316 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **6.9x** [1.4x-29.1x] | 18.92 | 171.45 | 18.79 | 171.02 | 0.065 | 0.453 | `OK` |
| `t3_mkp_200` | 200 | 8 | **4.2x** [3.1x-5.4x] | 25.48 | 117.75 | 25.40 | 117.12 | 0.096 | 0.652 | `OK` |
| `t3_cflp_300` | 300 | 34 | **31.1x** [7.7x-63.3x] | 6.44 | 257.21 | 6.34 | 256.24 | 0.106 | 0.953 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **2.5x** [1.5x-4.5x] | 87.57 | 159.28 | 87.32 | 157.37 | 0.215 | 1.982 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **8.6x** [3.9x-14.7x] | 8.30 | 81.73 | 7.95 | 78.15 | 0.333 | 2.927 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **30.4x** [24.3x-37.7x] | 0.60 | 18.64 | 0.46 | 17.59 | 0.134 | 1.054 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **18.5x** [11.7x-25.6x] | 1.06 | 19.93 | 0.85 | 18.12 | 0.210 | 1.873 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.6x** [1.0x-13.4x] | 8.43 | 61.42 | 7.43 | 53.44 | 1.394 | 8.573 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **10.0x** [4.4x-23.8x] | 37.54 | 379.67 | 30.98 | 344.26 | 4.473 | 36.012 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## 5. Default Build vs. `GOEXPERIMENT=simd`

In [`internal/simd/kernels_amd64.go`](../internal/simd/kernels_amd64.go) and [`internal/simd/kernels_avx512_amd64.go`](../internal/simd/kernels_avx512_amd64.go), runtime CPUID dispatch selects AVX-512, AVX2, or 4-way scalar loops from a single portable `CGO_ENABLED=0` binary (`GOAMD64=v3` FMA-safe). Each vector kernel spills accumulators via `StoreArray` and executes `archsimd.ClearAVXUpperBits()` (`VZEROUPPER`) before scalar tail or caller operations, eliminating AVX-to-SSE transition stalls while preserving IEEE-754 `NaN` propagation parity. AVX2 activates at `minAVX2Len = 32` (`Dot64`, `Axpy64`, `Scale64`, `AbsMax64`, `RowActivityBoundsFinite`, `SelectDualLeaveRow64With`, `MinReducedSkip64With`, `InitReducedRow64`, `AllAbsLE64With`, `ValidateAndNegate64With`), `rows >= 8 && cols >= 8` for `Transpose64With`, `minDualRatioSIMDChunks = 4` (256 cols for `ScanDualRatioCand64With`), and `minAVX2Min2Len = 256` (`Min2ReducedRow64`); on AVX-512 hosts, dense LSAP loops dispatch AVX-512 at `minAVX512Len = 64` and `minAVX512Min2Len = 128` while sparse LP kernels gate AVX-512 at `minAVX512LPLen = 2048` / `minLeaveRowAVX512 = 2048` / `minDualRatioAVX512Chunks = 128` (8,192 cols) to avoid Intel Emerald Rapids License-2 clock down-binning across surrounding scalar LU code.

At the full-solver level, each ratio below is the geometric mean of per-trial `scalar / SIMD` solve times over the same instances on the local AVX2 host (`i9-13900K`), with a 95% $t$-interval on the log ratios; "no detectable effect" means the interval contains 1. "Reachable" is a necessary condition: the AVX2 LP kernels require `n + m >= 32`. Below that size no vector kernel can run, so those small models serve as negative controls where intervals excluding 1 reflect binary code-layout or run-to-run variance rather than SIMD execution. Per-family paired speedups and 95% intervals for larger models appear in the generated table below.

### 5.1 Scope and Pending Target-Hardware Verification

All telemetry in this document is measured locally on the AVX2-only `13th Gen Intel(R) Core(TM) i9-13900K` host (`x86_64`, Linux). Portable `GOOS=linux GOARCH=arm64 CGO_ENABLED=0` and `GOAMD64=v3`/`v4` builds compile and pass local build checks, whereas fresh physical AVX-512 and `linux/arm64` runtime test and benchmark re-verification on the current tree remain pending target hardware access.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 2 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 3 faster, 2 slower, 18 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 1.04x [1.00x-1.09x] | 0.004 | 0.003 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 1.01x [0.98x-1.04x] | 0.003 | 0.003 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 1.01x [0.97x-1.05x] | 0.005 | 0.005 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source | 0.97x [0.85x-1.11x] | 0.004 | 0.004 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.97x [0.95x-0.98x] | 0.005 | 0.005 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 0.97x [0.94x-1.01x] | 0.016 | 0.016 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.94x [0.91x-0.97x] | 0.099 | 0.102 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 1.03x [0.99x-1.07x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 1.04x [0.99x-1.09x] | 0.048 | 0.049 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 1.01x [0.99x-1.04x] | 0.476 | 0.464 | 11 |
| `t2_mkp_30` | 36 | possible | SIMD slower | 0.94x [0.88x-1.00x] | 2.452 | 2.435 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 1.01x [0.99x-1.03x] | 5.252 | 5.288 | 11 |
| `t2_cflp_36` | 47 | possible | no detectable effect | 0.99x [0.96x-1.01x] | 0.165 | 0.154 | 11 |
| `t2_cflp_49` | 62 | possible | SIMD slower | 0.96x [0.94x-0.98x] | 0.328 | 0.333 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 1.03x [0.97x-1.09x] | 0.881 | 0.884 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 0.97x [0.92x-1.02x] | 3.661 | 4.607 | 11 |
| `t2_binpack_44` | 58 | possible | no detectable effect | 1.02x [0.98x-1.06x] | 1.015 | 0.981 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 1.01x [0.90x-1.13x] | 0.169 | 0.184 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 1.06x [0.97x-1.15x] | 0.138 | 0.132 | 11 |
| `t2_mad_g3_i9` | 108 | possible | no detectable effect | 1.01x [0.97x-1.05x] | 0.130 | 0.130 | 11 |
| `t2_mad_g4_i12` | 156 | possible | SIMD faster | 1.15x [1.02x-1.29x] | 0.329 | 0.302 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.01x [0.98x-1.04x] | 3.263 | 3.116 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 0.99x [0.97x-1.01x] | 18.787 | 17.700 | 11 |
| `t3_mkp_200` | 208 | possible | SIMD faster | 1.07x [1.05x-1.09x] | 25.400 | 23.128 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.06x [0.97x-1.14x] | 6.336 | 6.242 | 5 |
| `t3_lotsizing_504` | 840 | possible | no detectable effect | 1.03x [0.99x-1.07x] | 87.316 | 82.188 | 5 |
| `t3_sched_1000` | 1100 | possible | SIMD faster | 1.03x [1.00x-1.05x] | 7.954 | 7.625 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 1.01x [0.94x-1.09x] | 0.458 | 0.444 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 0.90x [0.68x-1.20x] | 0.854 | 0.950 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 0.98x [0.95x-1.01x] | 7.431 | 7.430 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | no detectable effect | 1.05x [0.98x-1.13x] | 30.976 | 28.595 | 5 |
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
| `t1_whiskas` | **0.01** (1 / 0) | 0.00 (1 / 0) | 0.01 (1) | 0.00 (1) | 0.004 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.02 (1) | 0.02 (1) | 0.003 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.10** (1 / 8) | 0.12 (2 / 8) | 0.16 (18) | 0.26 (35) | 0.008 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.05** (1 / 0) | 0.06 (1 / 0) | 0.07 (1) | 0.62 (4) | 0.091 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.48** (2 / 0) | 0.59 (12 / 0) | 0.58 (12) | 0.82 (109) | 0.111 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.45** (5 / 9) | 3.44 (40 / 9) | 1.25 (51) | 1.14 (61) | 0.034 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **5.25** (1 / 4) | 15.34 (2 / 4) | 5.81 (118) | 5.56 (111) | 0.044 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.17** (1 / 2) | 0.26 (3 / 2) | 0.20 (7) | 0.19 (7) | 0.026 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.33** (1 / 10) | 0.70 (4 / 10) | 0.35 (9) | 0.31 (9) | 0.036 | 909.02 | 827.30 |
| `t2_gap_45` | **0.88** (1 / 9) | 0.42 (1 / 9) | 1.90 (41) | 2.10 (60) | 0.031 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **3.66** (2 / 37) | 7.42 (25 / 37) | 3.27 (162) | 3.87 (199) | 0.069 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **1.01** (2 / 6) | 1.51 (5 / 6) | 9.38 (772) | 30.25 (1941) | 0.032 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.17** (1 / 6) | 0.36 (2 / 6) | 0.73 (6) | 0.68 (6) | 0.032 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.14** (1 / 0) | 0.93 (8 / 0) | 0.78 (8) | 0.78 (9) | 0.111 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.13** (1 / 0) | 0.14 (2 / 0) | 0.14 (2) | 3.27 (13) | 0.261 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.33** (1 / 0) | 3.53 (22 / 0) | 3.20 (22) | 5.63 (23) | 0.378 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.26** (43 / 0) | 3.10 (112 / 0) | 3.21 (112) | 3.96 (45) | 0.159 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **18.79** (196 / 0) | 33.16 (188 / 0) | 34.70 (188) | 97.46 (539) | 0.299 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **25.40** (250 / 14) | 113.05 (1135 / 16) | 230.90 (5000, `NodeLimit`) | 212.14 (5000, `NodeLimit`) | 0.136 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **6.34** (1 / 28) | 19.82 (54 / 34) | 9.46 (116) | 9.62 (118) | 0.183 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **87.32** (2 / 206) | 123.28 (376 / 206) | 250.13 (2785, `TimeLimit`) | 250.08 (2797, `TimeLimit`) | 0.735 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **7.95** (31 / 55) | 28.64 (87 / 53) | 250.12 (1684, `TimeLimit`) | 250.02 (1702, `TimeLimit`) | 0.235 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.46** (1 / 0) | 3.56 (17 / 0) | 3.26 (17) | 0.53 (1) | 0.653 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.85** (1 / 0) | 9.17 (58 / 0) | 9.35 (58) | 14.50 (74) | 1.673 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.43** (2 / 0) | 22.54 (25 / 0) | 20.52 (25) | 250.30 (145, `TimeLimit`) | 2.645 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **30.98** (1 / 0) | 33.93 (2 / 0) | 32.19 (2) | 250.34 (6, `TimeLimit`) | 58.832 | 208000.00 | 208000.00 |
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
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.30x** [1.06x-1.61x] | 1.30x | 3.89x | 0.237 (0.0207) | 0.322 (0.0295) | 1.36x [0.29x-2.85x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **2.94x** [1.88x-4.61x] | 2.94x | 2.98x | 9.236 (0.829) | 32.796 (2.964) | 2.62x [0.96x-15.04x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.88x** [1.85x-4.50x] | 2.88x | 3.76x | 225.633 (44.535) | 997.660 (196.043) | 2.59x [1.45x-6.13x] |
| Square families ($m = n$) | 16 (146) | **2.63x** [1.90x-3.65x] | 2.54x | 3.69x | 230.249 (44.510) | 1019.740 (197.057) | 2.30x [0.29x-15.04x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.71x** [1.17x-2.51x] | 1.59x | 2.90x | 4.856 (0.874) | 11.038 (1.980) | 1.88x [0.62x-2.68x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.28x** [1.17x-4.45x] | 2.42x | 2.91x | 36.574 (6.434) | 65.666 (10.929) | 1.81x [1.00x-15.04x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.34x** [1.81x-3.03x] | 2.24x | 3.45x | 235.106 (45.384) | 1030.778 (199.037) | 2.01x [0.29x-15.04x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 7.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.29x** | 0.0006 | 0.0009 | 0.0007 | 0.0122 | $0.0010 \pm 0.0010$ | $0.0009 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.17x** | 0.0018 | 0.0024 | 0.0021 | 0.0102 | $0.0022 \pm 0.0011$ | $0.0024 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.63x** | 0.0038 | 0.0064 | 0.0055 | 0.0156 | $0.0039 \pm 0.0011$ | $0.0065 \pm 0.0020$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.09x** | 0.0037 | 0.0040 | 0.0046 | 0.0142 | $0.0036 \pm 0.0008$ | $0.0039 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.08x** | 0.0043 | 0.0044 | 0.0059 | 0.0151 | $0.0042 \pm 0.0011$ | $0.0044 \pm 0.0009$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.69x** | 0.0065 | 0.0114 | 0.0106 | 0.0338 | $0.0066 \pm 0.0014$ | $0.0111 \pm 0.0020$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.69x** | 0.0293 | 0.0731 | 0.0356 | 0.0990 | $0.0274 \pm 0.0067$ | $0.0722 \pm 0.0110$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.17x** | 0.0546 | 0.174 | 0.0747 | 0.224 | $0.0553 \pm 0.0058$ | $0.176 \pm 0.0254$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.09x** | 0.0474 | 0.0983 | 0.0702 | 0.148 | $0.0481 \pm 0.0114$ | $0.0995 \pm 0.0146$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.71x** | 0.136 | 0.228 | 0.143 | 0.267 | $0.146 \pm 0.0480$ | $0.244 \pm 0.0456$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **11.58x** | 0.0619 | 0.692 | 0.0775 | 0.776 | $0.0604 \pm 0.0103$ | $0.689 \pm 0.0192$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.90x** | 0.0357 | 0.0687 | 0.0596 | 0.141 | $0.0382 \pm 0.0105$ | $0.0713 \pm 0.0103$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **1.96x** | 0.0398 | 0.0780 | 0.0687 | 0.153 | $0.0406 \pm 0.0036$ | $0.0795 \pm 0.0068$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.29x** | 0.0837 | 0.279 | 0.117 | 0.358 | $0.0863 \pm 0.0140$ | $0.283 \pm 0.0327$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.78x** | 0.340 | 1.273 | 0.452 | 1.682 | $0.337 \pm 0.0557$ | $1.268 \pm 0.122$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.05x** | 1.255 | 5.293 | 1.727 | 7.899 | $1.333 \pm 0.208$ | $5.363 \pm 0.394$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.75x** | 0.872 | 1.513 | 1.426 | 4.728 | $0.880 \pm 0.0579$ | $1.542 \pm 0.115$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.62x** | 5.306 | 8.379 | 5.547 | 9.337 | $5.851 \pm 2.168$ | $9.282 \pm 2.724$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.47x** | 0.358 | 0.818 | 0.730 | 3.794 | $0.344 \pm 0.0331$ | $0.846 \pm 0.0628$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.31x** | 0.433 | 1.007 | 0.858 | 4.289 | $0.437 \pm 0.0237$ | $1.011 \pm 0.0505$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **5.04x** | 5.983 | 29.840 | 8.117 | 42.521 | $5.877 \pm 0.362$ | $29.641 \pm 2.554$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **5.01x** | 30.328 | 149.192 | 41.063 | 194.432 | $30.404 \pm 3.667$ | $151.847 \pm 14.537$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 7.3 LSAP Scalar vs. `GOEXPERIMENT=simd`

In [`assign/solver.go`](../assign/solver.go), [`simd.AllAbsLE64With`](../internal/simd/kernels_amd64.go) and [`simd.ValidateAndNegate64With`](../internal/simd/assign_amd64.go) validate finite cost bounds via AVX2 checks whenever `len(cost) >= 32` (while the fused `colReduceValidateSquare` fast path skips `AllAbsLE64With` on finite square `Minimize` matrices and dispatches `ColReduceValidateRows64With` only for $n \ge 32$), [`simd.Transpose64With`](../internal/simd/assign_amd64.go) handles tall transposes with $\text{rows} \ge 8$ and $\text{cols} \ge 8$, [`simd.MinReducedSkip64With`](../internal/simd/assign_amd64.go) accelerates reduction-transfer slack scans in `preprocessSquareJV` ($n \ge 32$), [`simd.Min2ReducedRow64With`](../internal/simd/kernels_amd64.go) accelerates first/second reduced-minimum scans in `preprocessRectangularBV` and `augmentingRowReduction` at `minAVX2Min2Len = 256` (`minAVX512Min2Len = 128`), and [`simd.InitReducedRow64With`](../internal/simd/kernels_amd64.go) runs on a slice of length $\max(m, n) \ge 32$ at the start of each Dijkstra augmentation. All vector kernels clean upper `YMM` state via `StoreArray` + `ClearAVXUpperBits()` (`VZEROUPPER`) before scalar operations; exact per-family verdicts appear in the generated table below.

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` with no `>= 8x8` tall transpose and either fused square `Minimize` non-sparse path or `rows * cols < 32`, so no AVX2 kernel runs): 1 of 3 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32`, tall `rows > cols` with `>= 8x8`, or fallback `AllAbsLE64`/`ValidateAndNegate64` at `rows * cols >= 32`): 15 faster, 1 slower, 3 with no detectable effect, out of 19.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.86x [0.82x-0.91x] | 0.0006 | 0.0007 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source | 1.01x [0.98x-1.04x] | 0.0018 | 0.0017 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source | 0.95x [0.82x-1.10x] | 0.0038 | 0.0040 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | no detectable effect | 0.96x [0.83x-1.11x] | 0.0037 | 0.0036 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | no detectable effect | 1.03x [0.95x-1.12x] | 0.0043 | 0.0041 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | SIMD slower | 0.91x [0.83x-1.00x] | 0.0065 | 0.0072 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.26x [1.23x-1.29x] | 0.0293 | 0.0236 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | no detectable effect | 1.06x [0.64x-1.74x] | 0.0546 | 0.0422 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.58x [1.43x-1.73x] | 0.0474 | 0.0301 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.65x [1.57x-1.72x] | 0.136 | 0.0834 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.25x [1.18x-1.33x] | 0.0619 | 0.0486 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.27x [1.22x-1.32x] | 0.0357 | 0.0287 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.31x [1.27x-1.34x] | 0.0398 | 0.0313 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.32x [1.23x-1.41x] | 0.0837 | 0.0598 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.54x [1.48x-1.60x] | 0.340 | 0.212 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.73x [1.56x-1.93x] | 1.255 | 0.731 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.38x [2.00x-2.83x] | 0.872 | 0.331 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.55x [2.42x-2.68x] | 5.306 | 2.088 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.66x [1.51x-1.84x] | 0.358 | 0.214 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.59x [1.51x-1.68x] | 0.433 | 0.263 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.61x [1.44x-1.81x] | 5.983 | 3.526 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.51x [1.45x-1.56x] | 30.328 | 20.412 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

---

## 8. Production Binary Footprint and Linker Dead-Code Elimination

The `goblin-bench -binary-size` mode compiles four standalone `CGO_ENABLED=0` programs in an isolated temporary module against the live workspace across both default `go build` and `GOEXPERIMENT=simd`, saving compiled artifact sizes and `go tool nm -size` symbol counts to `bench/results/goblin_binary_size.json`. [`ValidateBinarySizeTelemetry`](../bench/docgen.go) rejects the telemetry bundle if `assign_default` links any `math/big`, `encoding/json`, `VIPR`, or `Certif` symbols, if `goblin_default` links any `encoding/json`, `VIPR`, or `Certif` symbols, or if any profile links `encoding/json`:

<!-- BEGIN GENERATED:BINARY_SIZE_TABLE -->
| Profile | Default Raw / Stripped (KiB) | `GOEXPERIMENT=simd` Raw / Stripped (KiB) | Stripped Delta vs. Default | `goblin` Text (KiB) | `math/big` Syms | `encoding/json` Syms | `VIPR` Syms | `Certif` Syms |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `assign` default (`LinearSum` + `SolveInto`) | 2822.8 / **1768.2** | 2858.1 / **1832.2** | baseline | 118.7 | `0` | `0` | `0` | `0` |
| `assign` certified (`+ assign.WithCertified(true)`) | 2942.9 / **1844.2** | 2980.7 / **1916.2** | +76.0 KiB (+4.3%) | 177.2 | `0` | `0` | `0` | `17` |
| `goblin` default (`Model.Solve`, `DefaultOptions`) | 5566.4 / **3548.2** | 5457.5 / **3568.2** | baseline | 1016.6 | `130` | `0` | `0` | `0` |
| `goblin` certified + VIPR (`+ WithCertified + WithVIPRTrace`) | 6821.4 / **4392.2** | 6679.1 / **4408.2** | +844.0 KiB (+23.8%) | 1496.3 | `155` | `0` | `220` | `14` |
<!-- END GENERATED:BINARY_SIZE_TABLE -->

---

## 9. Pure-Go vs. In-Process C++ CBC Performance Analysis

Comparing isolated unpresolved `Root LP` timings, paired trial ratios, and solver-core checkpoints (`CP5 +Presolve`, `CP6 +Cuts`, `CP7 Full`) between `BackendGo` and in-process `BackendCBC` (`c1c171ec`) removes Python MPS formatting and OS process-spawn overhead across all 31 synthetic families and highlights four structural trade-offs on large degenerate LPs and heavy-tail branch-and-cut models:

<!-- BEGIN GENERATED:CBC_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `BackendCBC / BackendGo` Total [Clustered 95% CI] | Equal-Family `BackendCBC / BackendGo` Solve [Clustered 95% CI] | `BackendGo` Trial (Med) Sum (ms) | `BackendCBC` Trial (Med) Sum (ms) | Per-Instance `CBC / Go` Total Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro-models (`N <= 26`) | 10 (110) | **22.23x** [9.81x-50.38x] | **43.44x** [13.90x-135.74x] | 8.8 (0.8) | 153.0 (14.5) | 16.69x [3.84x-137.01x] |
| Tier 2: Medium MILPs (`30 <= N <= 84`) | 13 (143) | **5.59x** [4.09x-7.64x] | **6.23x** [4.31x-9.01x] | 552.4 (37.2) | 1750.7 (152.5) | 6.01x [0.58x-43.32x] |
| Tier 3: Large MILPs (`145 <= N <= 4036`) | 8 (40) | **3.80x** [2.09x-6.89x] | **4.14x** [2.20x-7.80x] | 908.0 (175.4) | 2556.2 (519.0) | 3.77x [0.39x-14.27x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **4.61x** [3.14x-6.76x] | **4.88x** [3.20x-7.45x] | 1132.9 (164.2) | 2961.0 (381.9) | 5.29x [0.58x-43.32x] |
| All 31 synthetic families | 31 (293) | **7.90x** [5.31x-11.74x] | **10.49x** [6.19x-17.77x] | 1469.2 (213.4) | 4459.8 (686.0) | 7.42x [0.39x-137.01x] |
<!-- END GENERATED:CBC_SUMMARY_TABLE -->

### 9.1 MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families ($N = G(I + T + 3)$ variables, $M = G(2T + 3) + I$ constraints) separate solver behavior into three scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Paired `Go / CBC` | Go Total Med (ms) | CBC Total Med (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.08x** | 0.08 | 1.12 | 0.60x | 0.05 (1 / 19) | 1.08 (0 / 0) | 0.091 (28) | 0.150 (19) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.07x** | 0.50 | 7.54 | 0.71x | 0.48 (2 / 72) | 7.49 (14 / 44) | 0.111 (30) | 0.157 (18) |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.14x** | 0.17 | 1.27 | 0.70x | 0.14 (1 / 26) | 1.22 (0 / 0) | 0.111 (33) | 0.159 (21) |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.12x** | 0.18 | 1.68 | 0.97x | 0.13 (1 / 40) | 1.62 (0 / 0) | 0.261 (58) | 0.269 (40) |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.19x** | 0.43 | 2.04 | 0.89x | 0.33 (1 / 51) | 1.95 (0 / 0) | 0.378 (80) | 0.425 (63) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.32x** | 3.32 | 11.06 | 0.97x | 3.26 (43 / 483) | 11.01 (28 / 146) | 0.159 (38) | 0.164 (24) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.31x** | 18.92 | 77.90 | 0.97x | 18.79 (196 / 2289) | 77.82 (216 / 1352) | 0.299 (65) | 0.307 (54) |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.10x** | 0.60 | 6.12 | 1.00x | 0.46 (1 / 75) | 5.97 (0 / 0) | 0.653 (91) | 0.653 (94) |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.28x** | 1.06 | 4.01 | 1.53x | 0.85 (1 / 98) | 3.78 (0 / 0) | 1.673 (244) | 1.095 (149) |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **0.45x** | 8.43 | 26.78 | 0.71x | 7.43 (2 / 377) | 24.84 (0 / 0) | 2.645 (224) | 3.717 (319) |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **0.23x** | 37.54 | 247.69 | 1.63x | 30.98 (1 / 2439) | 243.20 (0 / 0) | 58.832 (1900) | 36.064 (3128) |
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
| `t3_mkp_200` | $200 \times 8$ | **0.57x** | $25.13 \pm 13.75$ | $40.83 \pm 19.36$ | 25.40 (250 / 2860) | 50.04 (154 / 1145) | 113.05 (1135 / 16) | 230.90 (5000, `NodeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **0.45x** | $7.48 \pm 7.89$ | $12.43 \pm 9.38$ | 5.25 (1 / 20) | 8.59 (0 / 8) | 15.34 (2 / 4) | 5.81 (118) |
| `t2_binpack_44` | $44 \times 14$ | **0.17x** | $1.33 \pm 0.86$ | $6.81 \pm 2.24$ | 1.01 (2 / 43) | 6.00 (0 / 8) | 1.51 (5 / 6) | 9.38 (772) |
| `t3_cflp_300` | $300 \times 34$ | **0.13x** | $8.59 \pm 5.75$ | $55.80 \pm 12.11$ | 6.34 (1 / 119) | 50.59 (18 / 483) | 19.82 (54 / 34) | 9.46 (116) |
| `t3_lotsizing_504` | $504 \times 336$ | **0.70x** | $73.29 \pm 46.66$ | $94.20 \pm 58.47$ | 87.32 (2 / 1351) | 71.83 (10 / 377) | 123.28 (376 / 206) | 250.13 (2785, `TimeLimit`) |
| `t3_sched_1000` | $1000 \times 100$ | **0.15x** | $10.92 \pm 6.11$ | $62.95 \pm 5.07$ | 7.95 (31 / 917) | 61.23 (0 / 76) | 28.64 (87 / 53) | 250.12 (1684, `TimeLimit`) |
<!-- END GENERATED:NON_MAD_CBC_GAP_TABLE -->

<!-- BEGIN GENERATED:NON_MAD_CBC_NOTES -->
- **Bin-packing tail behavior on `t2_binpack_44`**: pure-Go `BackendGo` ($1.33 \pm 0.86$ ms mean, 1.04 ms median) leads in-process `BackendCBC` ($6.81 \pm 2.24$ ms mean, 6.03 ms median) with a **5.88x** `BackendCBC / BackendGo` geometric-mean speedup (`Go / CBC = 0.17x`), running slower on 0 of 11 trials (peaking at 3.11 ms vs. 12.40 ms), with `CP5 +Presolve` at 9.38 ms vs. `CP6 +Cuts` at 1.51 ms.
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

# 11. Lean 4 Prove Lane build (78 jobs), arm64 + amd64 ELF verification, CLI/JSONL checks, and 37 default-build Fuzz corpora
./scripts/verify_formal.sh
PYTHONDONTWRITEBYTECODE=1 python3 scripts/extract_elf_track5.py --verify
PYTHONDONTWRITEBYTECODE=1 python3 scripts/extract_amd64_track5e.py --verify
```

Running `go test ./...` also executes `TestDocumentationAndBenchmarkSync`, which validates corpus dimensions, parity, symbol identifiers, provenance, formal inventory blocks, and every generated table across `README.md`, `docs/index.md`, `docs/benchmarks.md`, `docs/formal-verification.md`, and `bench/results/final_report.md`.

<!-- BEGIN GENERATED:BENCHMARKS_FORMAL_SUMMARY -->
Automated formal inventory across the workspace covers `24` Lean files (`53,872` LOC: `1,325` authored `theorem` and `lemma` declarations, `5,876` kernel `TheoremInfo` entries, and `220` axiom locks), `5` Go certification files (`18,475` LOC) paired with `30` formal and certified test files (`34,014` LOC), and `45` `Fuzz*` targets (`37` default-build across `./...` plus `8` `-tags=cbc`, matching [`scripts/gcp-fuzz/targets.tsv`](../scripts/gcp-fuzz/targets.tsv)).
<!-- END GENERATED:BENCHMARKS_FORMAL_SUMMARY -->
