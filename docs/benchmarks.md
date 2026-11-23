# Benchmark Methodology and Corpus Results

The benchmark protocol below evaluates pure-Go `goblin` (`BackendGo`, zero `cgo` by default), Python `PuLP` driving the `COIN-OR CBC` (`c1c171ec`) command-line binary, and `goblin (CBC)` (`BackendCBC`, built with `-tags cbc`), which calls the same `c1c171ec` CBC/CLP stack in-process through `cgo`. Every number in this document comes from a generated block rendered by [`bench/docgen.go`](../bench/docgen.go) from the JSON telemetry in `bench/results/`; the prose contains no hardcoded measurements. [`doc_sync_test.go`](../doc_sync_test.go) fails `go test ./...` when a block drifts from the telemetry, when a parity check fails, or when any of the eight telemetry files lacks uniform git, source-fingerprint, and host-kernel provenance.

---

## 1. Environment, Provenance, and Protocol

```bash
go run ./cmd/goblin-bench -check-docs
```

- **CPU**: `13th Gen Intel(R) Core(TM) i9-13900K` (`x86_64`, 30 logical CPUs exposed by `lscpu`, AVX2 + FMA).
- **Default build**: `go build` with the pure-Go kernels (`BackendGo`). All headline timings use that build.
- **Experimental build**: `GOEXPERIMENT=simd`, compared to the default build only in section 5.

<!-- BEGIN GENERATED:PROVENANCE -->
- **Unified telemetry source**: fingerprint `f9016b5ee7cf4cd3` (commit `6a7604718c89aaae1eba6e95f44439410a9df314`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.80 ms median on a 1-variable MILP and 6.34 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
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
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **354.04x** [169.35x-740.14x] | 354.04x | 0.05x | 8.8 (0.7) | 195.2 (19.4) | 1093.8 (102.8) | 470.29x [38.16x-2964.39x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **17.77x** [9.45x-33.40x] | 17.77x | 0.17x | 727.6 (44.9) | 1983.9 (173.5) | 4352.8 (423.8) | 24.48x [0.14x-171.70x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **10.58x** [5.30x-21.14x] | 10.58x | 0.20x | 778.9 (141.8) | 2905.0 (559.5) | 6251.8 (1074.1) | 10.29x [1.15x-73.24x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **11.69x** [6.58x-20.77x] | 12.48x | 0.22x | 1200.1 (141.7) | 3140.5 (396.8) | 6841.6 (995.9) | 12.29x [0.14x-171.70x] |
| Tier 3 non-MAD MILPs | 4 (20) | **8.20x** [1.82x-37.01x] | 8.20x | 0.27x | 480.7 (97.6) | 1246.3 (231.1) | 2829.2 (602.7) | 5.98x [1.88x-73.24x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **28.88x** [12.13x-68.75x] | 38.24x | 0.10x | 307.1 (45.0) | 1773.1 (338.5) | 3871.8 (512.0) | 41.86x [1.15x-187.77x] |
| Random-weight MAD (negative controls) | 3 (33) | **16.50x** [0.85x-320.69x] | 16.50x | 0.14x | 298.5 (20.4) | 1237.1 (118.4) | 2260.9 (250.7) | 14.82x [1.40x-120.34x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **46.02x** [19.30x-109.75x] | 55.14x | 0.12x | 1208.2 (142.4) | 3311.0 (413.8) | 7826.7 (1088.8) | 45.64x [0.14x-2964.39x] |
| All 31 synthetic corpus families | 31 (293) | **40.81x** [21.05x-79.10x] | 50.91x | 0.12x | 1515.3 (187.4) | 5084.1 (752.3) | 11698.5 (1600.7) | 44.42x [0.14x-2964.39x] |
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
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **155.9x** [125.1x-187.8x] | 0.06 | 9.96 | 0.04 | 9.73 | 0.020 | 0.231 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **64.9x** [38.2x-120.3x] | 0.48 | 34.19 | 0.46 | 33.94 | 0.024 | 0.253 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **56.8x** [51.1x-67.2x] | 0.16 | 9.26 | 0.13 | 8.96 | 0.038 | 0.282 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **54.7x** [41.7x-67.9x] | 0.17 | 9.91 | 0.13 | 9.51 | 0.043 | 0.403 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **28.7x** [25.0x-32.8x] | 0.41 | 11.42 | 0.32 | 10.83 | 0.077 | 0.611 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **9.6x** [3.6x-25.1x] | 3.52 | 48.07 | 3.45 | 47.77 | 0.044 | 0.305 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **7.2x** [1.4x-29.4x] | 16.45 | 168.49 | 16.38 | 168.05 | 0.058 | 0.441 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **32.9x** [25.0x-37.8x] | 0.55 | 18.96 | 0.43 | 17.71 | 0.111 | 1.057 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **17.0x** [10.1x-21.5x] | 1.01 | 18.77 | 0.83 | 16.97 | 0.230 | 1.809 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **6.0x** [1.1x-15.3x] | 8.44 | 60.50 | 7.38 | 52.45 | 1.012 | 8.311 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **10.4x** [4.4x-25.2x] | 34.20 | 373.18 | 30.52 | 338.69 | 4.117 | 35.147 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Cross-Instance Latency Dispersion Across Randomized Instances (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.06 \pm 0.01$ ($\pm 0.00$) | $9.89 \pm 0.58$ ($\pm 0.39$) | $0.07 \pm 0.01$ ($\pm 0.01$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.52 \pm 0.25$ ($\pm 0.17$) | $31.05 \pm 9.65$ ($\pm 6.48$) | $0.54 \pm 0.25$ ($\pm 0.17$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.17 \pm 0.01$ ($\pm 0.01$) | $9.44 \pm 0.61$ ($\pm 0.41$) | $0.16 \pm 0.01$ ($\pm 0.01$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.18 \pm 0.03$ ($\pm 0.02$) | $9.96 \pm 0.14$ ($\pm 0.09$) | $0.20 \pm 0.05$ ($\pm 0.03$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.40 \pm 0.04$ ($\pm 0.02$) | $11.55 \pm 0.55$ ($\pm 0.37$) | $0.35 \pm 0.01$ ($\pm 0.00$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $4.18 \pm 3.05$ ($\pm 2.05$) | $35.86 \pm 17.10$ ($\pm 11.49$) | $4.07 \pm 3.08$ ($\pm 2.07$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $22.44 \pm 22.42$ ($\pm 15.06$) | $138.63 \pm 63.45$ ($\pm 42.63$) | $22.20 \pm 21.79$ ($\pm 14.64$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.56 \pm 0.02$ ($\pm 0.03$) | $18.39 \pm 2.38$ ($\pm 2.96$) | $0.56 \pm 0.02$ ($\pm 0.02$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.15 \pm 0.40$ ($\pm 0.49$) | $18.78 \pm 0.16$ ($\pm 0.20$) | $1.20 \pm 0.33$ ($\pm 0.41$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $16.37 \pm 20.38$ ($\pm 25.31$) | $62.83 \pm 3.73$ ($\pm 4.63$) | $16.35 \pm 20.21$ ($\pm 25.09$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $41.56 \pm 14.44$ ($\pm 17.93$) | $584.51 \pm 613.10$ ($\pm 761.14$) | $40.49 \pm 15.98$ ($\pm 19.84$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | **0.04** (1 / 0) | 0.05 (1 / 0) | 0.08 (1) | 0.53 (4) | 0.086 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.46** (2 / 0) | 0.58 (12 / 0) | 0.54 (12) | 0.77 (109) | 0.100 | 4601.00 | 4257.00 |
| `t2_mad_g2_i8` | **0.13** (1 / 0) | 0.76 (8 / 0) | 0.73 (8) | 0.72 (9) | 0.103 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.13** (1 / 0) | 0.14 (2 / 0) | 0.14 (2) | 2.73 (13) | 0.247 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.32** (1 / 0) | 3.22 (22 / 0) | 2.96 (22) | 5.23 (23) | 0.364 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.45** (43 / 0) | 2.90 (112 / 0) | 2.86 (112) | 3.67 (45) | 0.155 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **16.38** (201 / 0) | 30.31 (188 / 0) | 30.06 (188) | 91.48 (539) | 0.279 | 7720.00 | 6060.00 |
| `t3_mad_g5_i20` | **0.43** (1 / 0) | 3.04 (17 / 0) | 2.69 (17) | 0.52 (1) | 0.655 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.83** (1 / 0) | 8.09 (58 / 0) | 7.62 (58) | 13.10 (74) | 1.602 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.38** (2 / 0) | 21.38 (25 / 0) | 19.12 (25) | 250.08 (151, `TimeLimit`) | 2.673 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **30.52** (1 / 0) | 32.48 (2 / 0) | 31.03 (2) | 251.27 (6, `TimeLimit`) | 56.410 | 208000.00 | 208000.00 |
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
| `t1_std_lp` | 3 | 3 | **397.2x** [140.3x-635.6x] | 0.01 | 6.76 | 0.01 | 6.69 | 0.007 | 0.064 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **392.5x** [207.9x-658.5x] | 0.01 | 6.50 | 0.01 | 6.44 | 0.007 | 0.060 | `OK` |
| `t1_std_mip` | 3 | 3 | **524.1x** [97.6x-1092.1x] | 0.01 | 7.45 | 0.01 | 7.40 | 0.006 | 0.063 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **974.8x** [733.2x-1347.8x] | 0.01 | 8.03 | 0.00 | 7.95 | 0.003 | 0.075 | `OK` |
| `t1_whiskas` | 6 | 5 | **508.5x** [354.5x-660.1x] | 0.01 | 6.42 | 0.01 | 6.33 | 0.006 | 0.079 | `OK` |
| `t1_furniture` | 2 | 3 | **290.9x** [85.7x-823.1x] | 0.02 | 7.66 | 0.01 | 7.60 | 0.003 | 0.054 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **116.1x** [51.9x-358.2x] | 0.09 | 9.47 | 0.09 | 9.37 | 0.007 | 0.101 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **2236.0x** [1453.3x-2964.4x] | 0.00 | 6.43 | 0.00 | 6.37 | 0.002 | 0.050 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **155.9x** [125.1x-187.8x] | 0.06 | 9.96 | 0.04 | 9.73 | 0.020 | 0.231 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **64.9x** [38.2x-120.3x] | 0.48 | 34.19 | 0.46 | 33.94 | 0.024 | 0.253 | `OK` |
| `t2_mkp_30` | 30 | 6 | **11.9x** [5.7x-16.6x] | 2.14 | 28.65 | 2.12 | 28.46 | 0.018 | 0.184 | `OK` |
| `t2_mkp_50` | 50 | 8 | **6.0x** [3.1x-33.4x] | 4.28 | 17.84 | 4.24 | 17.58 | 0.030 | 0.260 | `OK` |
| `t2_cflp_36` | 36 | 11 | **58.7x** [17.7x-171.7x] | 0.16 | 10.09 | 0.14 | 9.89 | 0.017 | 0.180 | `OK` |
| `t2_cflp_49` | 49 | 13 | **32.9x** [18.4x-88.3x] | 0.39 | 10.69 | 0.35 | 10.45 | 0.034 | 0.224 | `OK` |
| `t2_gap_45` | 45 | 13 | **23.0x** [4.7x-79.7x] | 0.79 | 14.33 | 0.76 | 14.11 | 0.021 | 0.208 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **11.5x** [5.6x-24.5x] | 4.36 | 69.30 | 4.33 | 69.04 | 0.031 | 0.249 | `OK` |
| `t2_binpack_44` | 44 | 14 | **1.9x** [0.1x-24.8x] | 11.76 | 14.15 | 11.73 | 13.95 | 0.029 | 0.208 | `OK` |
| `t2_setcover_50` | 50 | 20 | **40.7x** [6.6x-141.2x] | 0.27 | 11.61 | 0.24 | 11.28 | 0.033 | 0.328 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **56.8x** [51.1x-67.2x] | 0.16 | 9.26 | 0.13 | 8.96 | 0.038 | 0.282 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **54.7x** [41.7x-67.9x] | 0.17 | 9.91 | 0.13 | 9.51 | 0.043 | 0.403 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **28.7x** [25.0x-32.8x] | 0.41 | 11.42 | 0.32 | 10.83 | 0.077 | 0.611 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.6x** [3.6x-25.1x] | 3.52 | 48.07 | 3.45 | 47.77 | 0.044 | 0.305 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **7.2x** [1.4x-29.4x] | 16.45 | 168.49 | 16.38 | 168.05 | 0.058 | 0.441 | `OK` |
| `t3_mkp_200` | 200 | 8 | **4.9x** [4.2x-6.3x] | 18.38 | 115.39 | 18.30 | 114.78 | 0.077 | 0.626 | `OK` |
| `t3_cflp_300` | 300 | 34 | **26.8x** [8.0x-73.2x] | 9.34 | 248.44 | 9.24 | 247.51 | 0.102 | 0.921 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **3.1x** [1.9x-5.0x] | 62.90 | 158.24 | 62.70 | 156.37 | 0.322 | 1.893 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **11.0x** [5.4x-17.9x] | 7.02 | 80.59 | 6.56 | 77.59 | 0.286 | 2.805 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **32.9x** [25.0x-37.8x] | 0.55 | 18.96 | 0.43 | 17.71 | 0.111 | 1.057 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **17.0x** [10.1x-21.5x] | 1.01 | 18.77 | 0.83 | 16.97 | 0.230 | 1.809 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **6.0x** [1.1x-15.3x] | 8.44 | 60.50 | 7.38 | 52.45 | 1.012 | 8.311 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **10.4x** [4.4x-25.2x] | 34.20 | 373.18 | 30.52 | 338.69 | 4.117 | 35.147 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## 5. Default Build vs. `GOEXPERIMENT=simd`

In [`internal/simd/kernels_amd64.go`](../internal/simd/kernels_amd64.go) and [`internal/simd/kernels_avx512_amd64.go`](../internal/simd/kernels_avx512_amd64.go), runtime CPUID dispatch selects AVX-512, AVX2, or 4-way scalar loops from a single portable `CGO_ENABLED=0` binary (`GOAMD64=v3` FMA-safe). Each vector kernel spills accumulators via `StoreArray` and executes `archsimd.ClearAVXUpperBits()` (`VZEROUPPER`) before scalar tail or caller operations, eliminating AVX-to-SSE transition stalls while preserving IEEE-754 `NaN` propagation parity. AVX2 activates at `minAVX2Len = 32` (`Dot64`, `Axpy64`, `Scale64`, `AbsMax64`, `RowActivityBoundsFinite`, `SelectDualLeaveRow64With`, `MinReducedSkip64With`, `InitReducedRow64`, `AllAbsLE64With`, `ValidateAndNegate64With`), `rows >= 8 && cols >= 8` for `Transpose64With`, `minDualRatioSIMDChunks = 4` (256 cols for `ScanDualRatioCand64With`), and `minAVX2Min2Len = 256` (`Min2ReducedRow64`); on AVX-512 hosts, dense LSAP loops dispatch AVX-512 at `minAVX512Len = 64` and `minAVX512Min2Len = 128` while sparse LP kernels gate AVX-512 at `minAVX512LPLen = 2048` / `minLeaveRowAVX512 = 2048` / `minDualRatioAVX512Chunks = 128` (8,192 cols) to avoid Intel Emerald Rapids License-2 clock down-binning across surrounding scalar LU code.

At the full-solver level, each ratio below is the geometric mean of per-trial `scalar / SIMD` solve times over the same instances on the local AVX2 host (`i9-13900K`), with a 95% $t$-interval on the log ratios; "no detectable effect" means the interval contains 1. "Reachable" is a necessary condition: the AVX2 LP kernels require `n + m >= 32`. Below that size no vector kernel can run, so those small models serve as negative controls where intervals excluding 1 reflect binary code-layout or run-to-run variance rather than SIMD execution. Per-family paired speedups and 95% intervals for larger models appear in the generated table below.

### 5.1 Scope and Pending Target-Hardware Verification

All telemetry in this document is measured locally on the AVX2-only `13th Gen Intel(R) Core(TM) i9-13900K` host (`x86_64`, Linux). Portable `GOOS=linux GOARCH=arm64 CGO_ENABLED=0` and `GOAMD64=v3`/`v4` builds compile and pass local build checks, whereas fresh physical AVX-512 and `linux/arm64` runtime test and benchmark re-verification on the current tree remain pending target hardware access.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 2 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 4 faster, 1 slower, 18 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 0.90x [0.62x-1.31x] | 0.006 | 0.005 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.10x [1.00x-1.22x] | 0.006 | 0.005 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 1.22x [0.75x-1.96x] | 0.007 | 0.006 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.97x [0.95x-1.00x] | 0.004 | 0.004 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 1.02x [0.92x-1.13x] | 0.005 | 0.004 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 1.01x [0.96x-1.06x] | 0.015 | 0.016 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source | 1.00x [0.98x-1.02x] | 0.088 | 0.090 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 0.95x [0.75x-1.20x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 0.97x [0.94x-1.01x] | 0.039 | 0.042 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 0.98x [0.85x-1.14x] | 0.457 | 0.459 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 1.01x [0.98x-1.03x] | 2.121 | 2.103 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 0.96x [0.90x-1.02x] | 4.240 | 4.141 | 11 |
| `t2_cflp_36` | 47 | possible | SIMD slower | 0.89x [0.83x-0.95x] | 0.141 | 0.165 | 11 |
| `t2_cflp_49` | 62 | possible | SIMD faster | 1.09x [1.01x-1.17x] | 0.354 | 0.293 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 1.02x [0.99x-1.05x] | 0.764 | 0.755 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 1.04x [1.00x-1.08x] | 4.326 | 4.081 | 11 |
| `t2_binpack_44` | 58 | possible | no detectable effect | 1.02x [0.97x-1.07x] | 11.730 | 11.626 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 1.02x [0.97x-1.08x] | 0.237 | 0.239 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 1.02x [0.96x-1.09x] | 0.128 | 0.123 | 11 |
| `t2_mad_g3_i9` | 108 | possible | no detectable effect | 0.96x [0.84x-1.08x] | 0.129 | 0.134 | 11 |
| `t2_mad_g4_i12` | 156 | possible | SIMD faster | 1.11x [1.06x-1.16x] | 0.317 | 0.281 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | SIMD faster | 1.04x [1.00x-1.07x] | 3.454 | 2.997 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 1.00x [0.99x-1.01x] | 16.377 | 16.416 | 11 |
| `t3_mkp_200` | 208 | possible | SIMD faster | 1.06x [1.01x-1.11x] | 18.299 | 17.633 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.03x [0.96x-1.10x] | 9.237 | 8.431 | 5 |
| `t3_lotsizing_504` | 840 | possible | no detectable effect | 1.01x [0.97x-1.05x] | 62.702 | 60.084 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 1.00x [0.95x-1.06x] | 6.555 | 6.179 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 1.03x [0.97x-1.10x] | 0.432 | 0.428 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 0.94x [0.81x-1.09x] | 0.828 | 0.861 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 1.00x [0.95x-1.04x] | 7.380 | 7.465 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | no detectable effect | 1.05x [0.98x-1.13x] | 30.517 | 30.385 | 5 |
<!-- END GENERATED:SIMD_EFFECT_TABLE -->

---

## 6. Full Corpus Solver-Core Checkpoint Progression

<!-- BEGIN GENERATED:FULL_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_std_lp` | **0.01** (1 / 0) | 0.00 (1 / 0) | 0.01 (1) | 0.00 (1) | 0.004 | 54.00 | 54.00 |
| `t1_std_lp_max` | **0.01** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 80.00 | 80.00 |
| `t1_std_mip` | **0.01** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.01 (3) | 0.002 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.01 (3) | 0.002 | 64.95 | 58.90 |
| `t1_whiskas` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.00 (1) | 0.003 | 0.52 | 0.52 |
| `t1_furniture` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.002 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.09** (1 / 8) | 0.11 (2 / 8) | 0.15 (18) | 0.21 (35) | 0.008 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.04** (1 / 0) | 0.05 (1 / 0) | 0.08 (1) | 0.53 (4) | 0.086 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.46** (2 / 0) | 0.58 (12 / 0) | 0.54 (12) | 0.77 (109) | 0.100 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.12** (5 / 9) | 2.62 (40 / 9) | 1.05 (51) | 0.92 (61) | 0.025 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **4.24** (1 / 4) | 11.65 (2 / 4) | 4.77 (118) | 4.17 (111) | 0.038 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.14** (1 / 2) | 0.27 (3 / 2) | 0.23 (7) | 0.17 (7) | 0.024 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.35** (1 / 7) | 0.68 (3 / 7) | 0.33 (9) | 0.31 (9) | 0.036 | 909.02 | 827.30 |
| `t2_gap_45` | **0.76** (1 / 7) | 0.41 (1 / 7) | 1.80 (41) | 2.01 (60) | 0.025 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **4.33** (2 / 37) | 7.74 (25 / 37) | 3.00 (162) | 3.29 (199) | 0.060 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **11.73** (274 / 12) | 13.91 (298 / 12) | 8.27 (772) | 27.34 (1941) | 0.028 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.24** (1 / 6) | 0.34 (2 / 6) | 0.67 (6) | 0.65 (6) | 0.032 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.13** (1 / 0) | 0.76 (8 / 0) | 0.73 (8) | 0.72 (9) | 0.103 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.13** (1 / 0) | 0.14 (2 / 0) | 0.14 (2) | 2.73 (13) | 0.247 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.32** (1 / 0) | 3.22 (22 / 0) | 2.96 (22) | 5.23 (23) | 0.364 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.45** (43 / 0) | 2.90 (112 / 0) | 2.86 (112) | 3.67 (45) | 0.155 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **16.38** (201 / 0) | 30.31 (188 / 0) | 30.06 (188) | 91.48 (539) | 0.279 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **18.30** (250 / 14) | 81.17 (1135 / 16) | 163.68 (5000, `NodeLimit`) | 149.09 (5000, `NodeLimit`) | 0.119 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **9.24** (1 / 28) | 18.82 (54 / 34) | 8.55 (116) | 8.71 (118) | 0.170 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **62.70** (2 / 206) | 79.83 (376 / 206) | 250.03 (3651, `TimeLimit`) | 250.04 (3750, `TimeLimit`) | 0.758 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **6.56** (31 / 55) | 19.96 (87 / 53) | 250.03 (2570, `TimeLimit`) | 250.02 (2604, `TimeLimit`) | 0.182 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.43** (1 / 0) | 3.04 (17 / 0) | 2.69 (17) | 0.52 (1) | 0.655 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.83** (1 / 0) | 8.09 (58 / 0) | 7.62 (58) | 13.10 (74) | 1.602 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.38** (2 / 0) | 21.38 (25 / 0) | 19.12 (25) | 250.08 (151, `TimeLimit`) | 2.673 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **30.52** (1 / 0) | 32.48 (2 / 0) | 31.03 (2) | 251.27 (6, `TimeLimit`) | 56.410 | 208000.00 | 208000.00 |
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
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.37x** [1.15x-1.64x] | 1.37x | 4.39x | 0.218 (0.0194) | 0.314 (0.0285) | 1.39x [0.27x-2.60x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **2.97x** [1.84x-4.80x] | 2.97x | 2.96x | 9.012 (0.792) | 32.558 (2.966) | 2.55x [0.89x-16.56x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.83x** [1.80x-4.45x] | 2.83x | 3.58x | 224.030 (43.576) | 942.220 (188.915) | 2.70x [1.01x-6.13x] |
| Square families ($m = n$) | 16 (146) | **2.62x** [1.87x-3.68x] | 2.55x | 3.62x | 228.548 (43.545) | 964.180 (189.946) | 2.19x [0.27x-16.56x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.81x** [1.29x-2.53x] | 1.69x | 3.21x | 4.711 (0.843) | 10.912 (1.964) | 1.95x [0.89x-2.85x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.27x** [1.12x-4.61x] | 2.42x | 2.82x | 36.442 (6.488) | 64.808 (10.988) | 1.79x [1.01x-16.56x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.37x** [1.83x-3.07x] | 2.28x | 3.50x | 233.260 (44.388) | 975.092 (191.910) | 2.04x [0.27x-16.56x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 7.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.28x** | 0.0006 | 0.0008 | 0.0006 | 0.0118 | $0.0010 \pm 0.0010$ | $0.0009 \pm 0.0003$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.23x** | 0.0017 | 0.0023 | 0.0020 | 0.0102 | $0.0019 \pm 0.0007$ | $0.0023 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.68x** | 0.0036 | 0.0061 | 0.0045 | 0.0149 | $0.0035 \pm 0.0007$ | $0.0060 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.20x** | 0.0036 | 0.0040 | 0.0045 | 0.0141 | $0.0034 \pm 0.0007$ | $0.0040 \pm 0.0007$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.24x** | 0.0039 | 0.0043 | 0.0048 | 0.0151 | $0.0037 \pm 0.0008$ | $0.0045 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.73x** | 0.0060 | 0.0109 | 0.0099 | 0.0333 | $0.0064 \pm 0.0017$ | $0.0108 \pm 0.0015$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.31x** | 0.0294 | 0.0711 | 0.0388 | 0.0962 | $0.0311 \pm 0.0112$ | $0.0691 \pm 0.0106$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.37x** | 0.0535 | 0.182 | 0.0683 | 0.233 | $0.0525 \pm 0.0055$ | $0.177 \pm 0.0248$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.07x** | 0.0475 | 0.0941 | 0.0703 | 0.143 | $0.0460 \pm 0.0045$ | $0.0959 \pm 0.0143$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.60x** | 0.137 | 0.220 | 0.148 | 0.259 | $0.149 \pm 0.0471$ | $0.232 \pm 0.0437$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **12.72x** | 0.0579 | 0.688 | 0.0780 | 0.767 | $0.0547 \pm 0.0088$ | $0.687 \pm 0.0173$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.96x** | 0.0347 | 0.0693 | 0.0600 | 0.141 | $0.0375 \pm 0.0123$ | $0.0713 \pm 0.0077$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.02x** | 0.0383 | 0.0794 | 0.0616 | 0.154 | $0.0399 \pm 0.0041$ | $0.0805 \pm 0.0070$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.57x** | 0.0804 | 0.283 | 0.103 | 0.359 | $0.0798 \pm 0.0083$ | $0.285 \pm 0.0351$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.86x** | 0.314 | 1.278 | 0.431 | 1.674 | $0.329 \pm 0.0563$ | $1.261 \pm 0.120$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.18x** | 1.213 | 5.231 | 1.616 | 7.735 | $1.279 \pm 0.146$ | $5.332 \pm 0.430$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.55x** | 0.847 | 1.477 | 1.470 | 4.077 | $0.988 \pm 0.312$ | $1.482 \pm 0.0562$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.63x** | 5.389 | 8.491 | 5.641 | 9.349 | $5.730 \pm 1.964$ | $9.209 \pm 2.685$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.57x** | 0.332 | 0.826 | 0.727 | 3.862 | $0.329 \pm 0.0225$ | $0.845 \pm 0.0585$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.31x** | 0.431 | 0.981 | 0.820 | 4.068 | $0.427 \pm 0.0382$ | $0.985 \pm 0.0319$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.90x** | 5.885 | 28.204 | 7.792 | 37.185 | $5.862 \pm 0.395$ | $28.760 \pm 2.284$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.72x** | 29.480 | 143.705 | 40.202 | 186.631 | $30.191 \pm 3.938$ | $141.831 \pm 9.622$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 7.3 LSAP Scalar vs. `GOEXPERIMENT=simd`

In [`assign/solver.go`](../assign/solver.go), [`simd.AllAbsLE64With`](../internal/simd/kernels_amd64.go) and [`simd.ValidateAndNegate64With`](../internal/simd/assign_amd64.go) validate finite cost bounds via AVX2 checks whenever `len(cost) >= 32` (while the fused `colReduceValidateSquare` fast path skips `AllAbsLE64With` on finite square `Minimize` matrices and dispatches `ColReduceValidateRows64With` only for $n \ge 32$), [`simd.Transpose64With`](../internal/simd/assign_amd64.go) handles tall transposes with $\text{rows} \ge 8$ and $\text{cols} \ge 8$, [`simd.MinReducedSkip64With`](../internal/simd/assign_amd64.go) accelerates reduction-transfer slack scans in `preprocessSquareJV` ($n \ge 32$), [`simd.Min2ReducedRow64With`](../internal/simd/kernels_amd64.go) accelerates first/second reduced-minimum scans in `preprocessRectangularBV` and `augmentingRowReduction` at `minAVX2Min2Len = 256` (`minAVX512Min2Len = 128`), and [`simd.InitReducedRow64With`](../internal/simd/kernels_amd64.go) runs on a slice of length $\max(m, n) \ge 32$ at the start of each Dijkstra augmentation. All vector kernels clean upper `YMM` state via `StoreArray` + `ClearAVXUpperBits()` (`VZEROUPPER`) before scalar operations; exact per-family verdicts appear in the generated table below.

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` with no `>= 8x8` tall transpose and either fused square `Minimize` non-sparse path or `rows * cols < 32`, so no AVX2 kernel runs): 1 of 3 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32`, tall `rows > cols` with `>= 8x8`, or fallback `AllAbsLE64`/`ValidateAndNegate64` at `rows * cols >= 32`): 17 faster, 1 slower, 1 with no detectable effect, out of 19.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source | 1.00x [0.99x-1.02x] | 0.0006 | 0.0005 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source | 0.71x [0.38x-1.34x] | 0.0017 | 0.0019 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.97x [0.95x-1.00x] | 0.0036 | 0.0037 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | no detectable effect | 1.02x [0.88x-1.18x] | 0.0036 | 0.0032 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | SIMD faster | 1.15x [1.12x-1.18x] | 0.0039 | 0.0034 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | SIMD slower | 0.94x [0.89x-0.99x] | 0.0060 | 0.0065 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.24x [1.16x-1.33x] | 0.0294 | 0.0232 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.30x [1.23x-1.38x] | 0.0535 | 0.0401 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.68x [1.57x-1.79x] | 0.0475 | 0.0294 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.71x [1.64x-1.78x] | 0.137 | 0.0806 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.28x [1.22x-1.34x] | 0.0579 | 0.0448 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.27x [1.20x-1.34x] | 0.0347 | 0.0271 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.34x [1.26x-1.42x] | 0.0383 | 0.0293 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.38x [1.31x-1.45x] | 0.0804 | 0.0566 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.55x [1.45x-1.64x] | 0.314 | 0.205 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.70x [1.59x-1.81x] | 1.213 | 0.744 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.77x [2.05x-3.73x] | 0.847 | 0.357 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.62x [2.46x-2.80x] | 5.389 | 1.902 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.88x [1.71x-2.07x] | 0.332 | 0.171 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.53x [1.49x-1.57x] | 0.431 | 0.291 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.65x [1.59x-1.70x] | 5.885 | 3.644 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.47x [1.38x-1.57x] | 29.480 | 19.822 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

---

## 8. Production Binary Footprint and Linker Dead-Code Elimination

The `goblin-bench -binary-size` mode compiles four standalone `CGO_ENABLED=0` programs in an isolated temporary module against the live workspace across both default `go build` and `GOEXPERIMENT=simd`, saving compiled artifact sizes and `go tool nm -size` symbol counts to `bench/results/goblin_binary_size.json`. [`ValidateBinarySizeTelemetry`](../bench/docgen.go) rejects the telemetry bundle if `assign_default` links any `math/big`, `encoding/json`, `VIPR`, or `Certif` symbols, if `goblin_default` links any `encoding/json`, `VIPR`, or `Certif` symbols, or if any profile links `encoding/json`:

<!-- BEGIN GENERATED:BINARY_SIZE_TABLE -->
| Profile | Default Raw / Stripped (KiB) | `GOEXPERIMENT=simd` Raw / Stripped (KiB) | Stripped Delta vs. Default | `goblin` Text (KiB) | `math/big` Syms | `encoding/json` Syms | `VIPR` Syms | `Certif` Syms |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `assign` default (`LinearSum` + `SolveInto`) | 2784.8 / **1740.2** | 2813.7 / **1804.2** | baseline | 114.4 | `0` | `0` | `0` | `0` |
| `assign` certified (`+ assign.WithCertified(true)`) | 2871.3 / **1796.2** | 2898.1 / **1860.2** | +56.0 KiB (+3.2%) | 154.8 | `0` | `0` | `0` | `16` |
| `goblin` default (`Model.Solve`, `DefaultOptions`) | 5276.5 / **3360.2** | 5167.2 / **3376.2** | baseline | 899.1 | `130` | `0` | `0` | `0` |
| `goblin` certified + VIPR (`+ WithCertified + WithVIPRTrace`) | 6362.1 / **4092.2** | 6236.9 / **4116.2** | +732.0 KiB (+21.8%) | 1317.0 | `153` | `0` | `183` | `10` |
<!-- END GENERATED:BINARY_SIZE_TABLE -->

---

## 9. Pure-Go vs. In-Process C++ CBC Performance Analysis

Comparing isolated unpresolved `Root LP` timings, paired trial ratios, and solver-core checkpoints (`CP5 +Presolve`, `CP6 +Cuts`, `CP7 Full`) between `BackendGo` and in-process `BackendCBC` (`c1c171ec`) highlights four structural trade-offs on large degenerate LPs and heavy-tail branch-and-cut models.

### 9.1 MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families ($N = G(I + T + 3)$ variables, $M = G(2T + 3) + I$ constraints) separate solver behavior into three scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Paired `Go / CBC` | Go Total Med (ms) | CBC Total Med (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.03x** | 0.06 | 2.44 | 0.64x | 0.04 (1 / 19) | 2.41 (0 / 0) | 0.086 (28) | 0.136 (19) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.05x** | 0.48 | 11.14 | 0.69x | 0.46 (2 / 58) | 11.10 (6 / 83) | 0.100 (30) | 0.144 (18) |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.09x** | 0.16 | 1.81 | 0.69x | 0.13 (1 / 26) | 1.77 (0 / 0) | 0.103 (33) | 0.148 (21) |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.07x** | 0.17 | 2.57 | 0.87x | 0.13 (1 / 40) | 2.52 (0 / 0) | 0.247 (58) | 0.283 (40) |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.11x** | 0.41 | 3.32 | 0.88x | 0.32 (1 / 51) | 3.24 (0 / 0) | 0.364 (80) | 0.412 (63) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.25x** | 3.52 | 16.86 | 0.99x | 3.45 (43 / 437) | 16.81 (12 / 216) | 0.155 (38) | 0.156 (24) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.24x** | 16.45 | 90.37 | 0.99x | 16.38 (201 / 2102) | 90.31 (198 / 1300) | 0.279 (65) | 0.281 (54) |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.07x** | 0.55 | 8.19 | 1.04x | 0.43 (1 / 75) | 8.06 (0 / 0) | 0.655 (91) | 0.627 (94) |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.15x** | 1.01 | 7.42 | 1.53x | 0.83 (1 / 98) | 7.22 (0 / 0) | 1.602 (244) | 1.044 (149) |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **0.29x** | 8.44 | 35.87 | 0.74x | 7.38 (2 / 364) | 35.07 (0 / 0) | 2.673 (224) | 3.610 (319) |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **0.17x** | 34.20 | 276.89 | 1.60x | 30.52 (1 / 2439) | 273.32 (0 / 0) | 56.410 (1900) | 35.168 (3128) |
<!-- END GENERATED:MAD_CBC_GAP_TABLE -->

1. **Small-to-Medium Planted-Gap MAD (`t1_mad_g2_i4` through `t3_mad_g5_i20`)**: Every structured `t*_mad_g*` instance has a zero root LP gap by design (`RootLPBound == OptimalObj`). Pure-Go closes those models at node 1 through presolve orbit fixing and root heuristics (`tryPropagatedRounding`, `tryRootSubMIP`, and `tryFractionalDiving` in [`heuristics.go`](../heuristics.go)) without C++ `CbcModel` and `CglPreProcess` setup costs.
2. **Large Planted-Gap MAD (`t3_mad_g6_i30`, `t3_mad_g10_i100`, `t3_mad_g4_i1000`)**: Pure-Go `Go Solve` closes these models at the root (exact node counts and solve times appear in the generated table above) because [`Presolve`](../presolve.go) verifies the $S_G$ group-permutation automorphism (`applyOrbitSymmetryBreaking`), fixes the $G \times G$ designated-item bijection to $I_G$, and eliminates $G^2$ binary variables and $2G$ assignment equations before the root LP runs, while [`FTRANSparseHint`](../lu.go) and [`BTRANSparseHint`](../lu.go) restrict basis triangular sweeps to active index windows. On the unpresolved `Root LP` column:
   - **Equality Crash Basis (`t3_mad_g10_i100`)**: [`crashEqualityBasis`](../simplex.go) builds a lower-triangular, dual-feasible equality basis with analytical DSE weights.
   - **Dense-Row Coupling and Forrest-Tomlin Updates (`t3_mad_g4_i1000`)**: On 1,000-row bases, [`crashEqualityBasis`](../simplex.go) skips columns incident to coupling rows wider than 128 nonzeros so BFRT bound flips retain sparse unit slacks, while [`lu.go`](../lu.go) performs in-place mutated-$U$ Forrest-Tomlin updates (`PushEtaSparse`) inside pre-allocated headroom arenas up to `maxEtaUpdates = 56`.
3. **Symmetric Random-Weight MAD (`t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`)**: Assigning dense random weights (`BuildMADRandom`) removes the zero-gap structure and forces Branch-and-Cut to prove optimality. In these models, [`Presolve`](../presolve.go) verifies exact $S_G$ group symmetry and fixes the $G \times G$ designated-item bijection to $I_G$, [`bab.go`](../bab.go) exits after round 1 when root cuts yield zero bound gain and purges slack root cuts via [`PurgeSlackCuts`](../simplex.go), and [`tryRootSubMIP`](../heuristics.go) enforces a 50% RINS integer-fixing gate on zero-cut roots. Comparing `CP7 Full` against `CP5 +Presolve` (`Presolve=on, Cuts=off, Heuristics=off`) in [Section 3](#mad-solver-core-checkpoint-progression) isolates the effect of cutting planes and primal heuristics on random-weight MAD search.

### 9.2 Non-MAD Corpus Comparison, Dispersion, and Checkpoint Trade-Offs

Across the non-MAD families, multi-row MIR cut aggregation (`separateAggregatedMIRCuts`), bound-cutoff strong-branching domain reductions (`selectBranchVarCore`), and globally valid in-tree cuts (`generateInTreeCutRound`) accelerate Branch-and-Cut convergence, while the generated `Go Total Mean +- SD` and `CBC Total Mean +- SD` columns capture seed-to-seed variance (for example outlier seeds in `t2_binpack_44`):

<!-- BEGIN GENERATED:NON_MAD_CBC_GAP_TABLE -->
| Problem ID | $N \times M$ | Paired `Go / CBC` | Go Total Mean $\pm$ SD (ms) | CBC Total Mean $\pm$ SD (ms) | Go CP7 Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go CP6 +Cuts (ms / nodes / cuts) | Go CP5 +Presolve (ms / nodes) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t3_mkp_200` | $200 \times 8$ | **0.49x** | $21.63 \pm 13.45$ | $40.01 \pm 18.88$ | 18.30 (250 / 2447) | 49.03 (154 / 1145) | 81.17 (1135 / 16) | 163.68 (5000, `NodeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **0.35x** | $5.90 \pm 6.19$ | $12.50 \pm 9.17$ | 4.24 (1 / 19) | 8.76 (0 / 8) | 11.65 (2 / 4) | 4.77 (118) |
| `t2_binpack_44` | $44 \times 14$ | **1.19x** | $22.14 \pm 41.75$ | $6.69 \pm 2.15$ | 11.73 (274 / 1988) | 6.22 (0 / 8) | 13.91 (298 / 12) | 8.27 (772) |
| `t3_cflp_300` | $300 \times 34$ | **0.15x** | $10.66 \pm 7.39$ | $55.13 \pm 11.82$ | 9.24 (1 / 105) | 50.10 (18 / 483) | 18.82 (54 / 34) | 8.55 (116) |
| `t3_lotsizing_504` | $504 \times 336$ | **0.55x** | $55.64 \pm 34.85$ | $92.90 \pm 57.41$ | 62.70 (2 / 1061) | 71.64 (10 / 377) | 79.83 (376 / 206) | 250.03 (3651, `TimeLimit`) |
| `t3_sched_1000` | $1000 \times 100$ | **0.12x** | $8.22 \pm 4.30$ | $61.22 \pm 4.57$ | 6.56 (31 / 474) | 59.55 (0 / 76) | 19.96 (87 / 53) | 250.03 (2570, `TimeLimit`) |
<!-- END GENERATED:NON_MAD_CBC_GAP_TABLE -->

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

# 11. Lean 4 Prove Lane build (75 jobs), arm64 + amd64 ELF verification, CLI/JSONL checks, and 19 formal Fuzz corpora
./scripts/verify_formal.sh
PYTHONDONTWRITEBYTECODE=1 python3 scripts/extract_elf_track5.py --verify
PYTHONDONTWRITEBYTECODE=1 python3 scripts/extract_amd64_track5e.py --verify
```

Running `go test ./...` also executes `TestDocumentationAndBenchmarkSync`, which validates corpus dimensions, parity, symbol identifiers, provenance, and every generated block across `README.md`, `docs/benchmarks.md`, and `bench/results/final_report.md`.
