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
- **Unified telemetry source**: fingerprint `e8f74b711ec92a3a` (commit `4bc6c2b5d36f1e76ea2231cdf0a1a1c4cf5f0054`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.24 ms median on a 1-variable MILP and 6.26 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
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
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **330.78x** [157.60x-694.26x] | 330.78x | 0.05x | 9.3 (0.7) | 195.5 (19.3) | 1052.1 (99.1) | 456.78x [26.06x-3002.05x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **20.43x** [12.53x-33.32x] | 20.43x | 0.15x | 648.3 (32.3) | 1946.5 (170.3) | 4299.7 (415.6) | 26.26x [0.13x-165.89x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **10.43x** [5.05x-21.53x] | 10.43x | 0.20x | 882.8 (136.9) | 2893.3 (555.7) | 6186.9 (1067.2) | 10.47x [1.10x-74.97x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **13.33x** [8.00x-22.21x] | 14.66x | 0.19x | 1222.2 (124.7) | 3100.7 (392.8) | 6759.1 (984.5) | 14.58x [0.13x-165.89x] |
| Tier 3 non-MAD MILPs | 4 (20) | **7.95x** [1.60x-39.62x] | 7.95x | 0.27x | 582.3 (93.2) | 1241.2 (229.9) | 2803.8 (599.5) | 6.49x [1.12x-74.97x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **27.55x** [12.32x-61.58x] | 35.81x | 0.10x | 309.8 (44.5) | 1764.1 (335.7) | 3827.1 (507.6) | 38.03x [1.10x-164.93x] |
| Random-weight MAD (negative controls) | 3 (33) | **15.99x** [0.92x-277.81x] | 15.99x | 0.15x | 300.0 (20.2) | 1207.2 (115.7) | 2241.7 (247.7) | 14.70x [1.34x-119.27x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **48.91x** [21.53x-111.10x] | 59.17x | 0.11x | 1230.6 (125.4) | 3271.2 (409.6) | 7711.6 (1074.3) | 50.00x [0.13x-3002.05x] |
| All 31 synthetic corpus families | 31 (293) | **42.17x** [22.53x-78.94x] | 53.02x | 0.11x | 1540.4 (170.0) | 5035.3 (745.3) | 11538.6 (1581.9) | 42.69x [0.13x-3002.05x] |
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
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **122.0x** [42.6x-164.9x] | 0.06 | 9.30 | 0.04 | 9.10 | 0.022 | 0.206 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **59.5x** [26.1x-119.3x] | 0.49 | 33.73 | 0.46 | 33.49 | 0.029 | 0.241 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **51.7x** [44.1x-64.0x] | 0.18 | 9.17 | 0.14 | 8.89 | 0.044 | 0.271 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **46.5x** [33.7x-57.0x] | 0.21 | 10.15 | 0.14 | 9.72 | 0.065 | 0.416 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **32.2x** [29.1x-38.5x] | 0.36 | 11.28 | 0.29 | 10.67 | 0.070 | 0.597 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **9.7x** [3.5x-27.8x] | 3.14 | 46.89 | 3.10 | 46.60 | 0.043 | 0.298 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **7.1x** [1.3x-29.5x] | 16.59 | 167.04 | 16.53 | 166.61 | 0.055 | 0.433 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **33.2x** [26.5x-40.3x] | 0.53 | 18.08 | 0.42 | 16.97 | 0.110 | 1.105 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **17.3x** [10.5x-23.1x] | 1.04 | 17.89 | 0.84 | 16.22 | 0.205 | 1.695 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **6.0x** [1.1x-14.7x] | 8.17 | 61.09 | 7.36 | 53.12 | 1.061 | 8.153 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **10.2x** [4.4x-24.8x] | 33.98 | 370.66 | 30.28 | 336.00 | 3.785 | 34.091 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Cross-Instance Latency Dispersion Across Randomized Instances (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.08 \pm 0.05$ ($\pm 0.03$) | $9.06 \pm 0.46$ ($\pm 0.31$) | $0.07 \pm 0.01$ ($\pm 0.01$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.55 \pm 0.32$ ($\pm 0.21$) | $30.33 \pm 9.63$ ($\pm 6.47$) | $0.51 \pm 0.28$ ($\pm 0.19$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.18 \pm 0.01$ ($\pm 0.01$) | $9.34 \pm 0.53$ ($\pm 0.36$) | $0.15 \pm 0.01$ ($\pm 0.00$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.22 \pm 0.04$ ($\pm 0.03$) | $10.27 \pm 0.35$ ($\pm 0.23$) | $0.22 \pm 0.04$ ($\pm 0.03$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.36 \pm 0.01$ ($\pm 0.01$) | $11.68 \pm 0.85$ ($\pm 0.57$) | $0.35 \pm 0.01$ ($\pm 0.01$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $4.17 \pm 3.21$ ($\pm 2.16$) | $35.21 \pm 16.87$ ($\pm 11.33$) | $4.16 \pm 3.24$ ($\pm 2.18$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $22.54 \pm 22.08$ ($\pm 14.83$) | $138.26 \pm 63.47$ ($\pm 42.63$) | $22.20 \pm 21.89$ ($\pm 14.71$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.54 \pm 0.02$ ($\pm 0.02$) | $17.88 \pm 2.40$ ($\pm 2.98$) | $0.54 \pm 0.02$ ($\pm 0.02$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.08 \pm 0.36$ ($\pm 0.44$) | $18.11 \pm 0.44$ ($\pm 0.55$) | $1.57 \pm 0.78$ ($\pm 0.97$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $16.63 \pm 21.26$ ($\pm 26.39$) | $62.80 \pm 3.69$ ($\pm 4.58$) | $18.31 \pm 23.14$ ($\pm 28.72$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $41.84 \pm 14.38$ ($\pm 17.85$) | $577.82 \pm 610.64$ ($\pm 758.09$) | $39.00 \pm 16.12$ ($\pm 20.01$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | **0.04** (1 / 0) | 0.05 (1 / 0) | 0.06 (1) | 0.53 (4) | 0.084 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.46** (2 / 0) | 0.56 (12 / 0) | 0.55 (12) | 0.78 (109) | 0.103 | 4601.00 | 4257.00 |
| `t2_mad_g2_i8` | **0.14** (1 / 0) | 0.81 (8 / 0) | 0.73 (8) | 0.73 (9) | 0.104 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.14** (1 / 0) | 0.13 (2 / 0) | 0.13 (2) | 2.75 (13) | 0.242 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.29** (1 / 0) | 3.23 (22 / 0) | 2.95 (22) | 5.36 (23) | 0.352 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.10** (43 / 0) | 3.09 (112 / 0) | 2.86 (112) | 3.71 (45) | 0.152 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **16.53** (201 / 0) | 30.37 (188 / 0) | 30.17 (188) | 92.68 (539) | 0.281 | 7720.00 | 6060.00 |
| `t3_mad_g5_i20` | **0.42** (1 / 0) | 2.96 (17 / 0) | 3.50 (17) | 0.53 (1) | 0.667 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.84** (1 / 0) | 8.19 (58 / 0) | 7.57 (58) | 14.13 (74) | 1.547 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.36** (2 / 0) | 21.55 (25 / 0) | 20.12 (25) | 250.36 (147, `TimeLimit`) | 2.651 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **30.28** (1 / 0) | 32.84 (2 / 0) | 31.73 (2) | 251.28 (6, `TimeLimit`) | 57.486 | 208000.00 | 208000.00 |
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
| `t1_std_lp` | 3 | 3 | **397.3x** [100.5x-727.4x] | 0.01 | 6.30 | 0.01 | 6.24 | 0.007 | 0.063 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **371.2x** [183.6x-631.3x] | 0.01 | 6.15 | 0.00 | 6.10 | 0.008 | 0.049 | `OK` |
| `t1_std_mip` | 3 | 3 | **486.6x** [122.2x-1203.6x] | 0.01 | 7.30 | 0.01 | 7.23 | 0.005 | 0.058 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **758.0x** [628.4x-984.8x] | 0.01 | 7.14 | 0.00 | 7.09 | 0.004 | 0.051 | `OK` |
| `t1_whiskas` | 6 | 5 | **606.9x** [446.5x-737.2x] | 0.01 | 6.25 | 0.00 | 6.18 | 0.005 | 0.072 | `OK` |
| `t1_furniture` | 2 | 3 | **288.9x** [79.3x-1058.7x] | 0.02 | 7.57 | 0.02 | 7.52 | 0.003 | 0.051 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **111.8x** [50.0x-331.7x] | 0.09 | 9.20 | 0.09 | 9.10 | 0.007 | 0.090 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **2026.2x** [1170.8x-3002.1x] | 0.00 | 6.17 | 0.00 | 6.12 | 0.002 | 0.042 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **122.0x** [42.6x-164.9x] | 0.06 | 9.30 | 0.04 | 9.10 | 0.022 | 0.206 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **59.5x** [26.1x-119.3x] | 0.49 | 33.73 | 0.46 | 33.49 | 0.029 | 0.241 | `OK` |
| `t2_mkp_30` | 30 | 6 | **11.6x** [6.2x-16.8x] | 2.12 | 27.98 | 2.10 | 27.80 | 0.018 | 0.173 | `OK` |
| `t2_mkp_50` | 50 | 8 | **6.0x** [3.1x-34.4x] | 4.10 | 16.77 | 4.07 | 16.51 | 0.029 | 0.254 | `OK` |
| `t2_cflp_36` | 36 | 11 | **57.4x** [24.4x-165.9x] | 0.15 | 8.84 | 0.14 | 8.68 | 0.017 | 0.153 | `OK` |
| `t2_cflp_49` | 49 | 13 | **33.2x** [19.9x-93.1x] | 0.35 | 10.48 | 0.32 | 10.29 | 0.028 | 0.215 | `OK` |
| `t2_gap_45` | 45 | 13 | **20.6x** [4.9x-66.2x] | 0.98 | 14.15 | 0.96 | 13.93 | 0.020 | 0.207 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **13.1x** [5.8x-34.0x] | 3.32 | 68.74 | 3.29 | 68.50 | 0.027 | 0.244 | `OK` |
| `t2_binpack_44` | 44 | 14 | **11.4x** [0.1x-37.5x] | 0.61 | 13.36 | 0.55 | 13.15 | 0.021 | 0.192 | `OK` |
| `t2_setcover_50` | 50 | 20 | **49.5x** [6.4x-143.6x] | 0.20 | 10.75 | 0.14 | 10.48 | 0.035 | 0.289 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **51.7x** [44.1x-64.0x] | 0.18 | 9.17 | 0.14 | 8.89 | 0.044 | 0.271 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **46.5x** [33.7x-57.0x] | 0.21 | 10.15 | 0.14 | 9.72 | 0.065 | 0.416 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **32.2x** [29.1x-38.5x] | 0.36 | 11.28 | 0.29 | 10.67 | 0.070 | 0.597 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.7x** [3.5x-27.8x] | 3.14 | 46.89 | 3.10 | 46.60 | 0.043 | 0.298 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **7.1x** [1.3x-29.5x] | 16.59 | 167.04 | 16.53 | 166.61 | 0.055 | 0.433 | `OK` |
| `t3_mkp_200` | 200 | 8 | **5.3x** [4.0x-6.5x] | 18.65 | 114.81 | 18.58 | 114.19 | 0.071 | 0.617 | `OK` |
| `t3_cflp_300` | 300 | 34 | **27.9x** [7.7x-75.0x] | 6.09 | 247.18 | 5.99 | 246.28 | 0.103 | 0.902 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **2.6x** [1.1x-4.9x] | 61.81 | 156.92 | 61.62 | 155.06 | 0.207 | 1.852 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **10.4x** [5.0x-14.5x] | 6.64 | 80.58 | 6.36 | 77.70 | 0.310 | 2.741 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **33.2x** [26.5x-40.3x] | 0.53 | 18.08 | 0.42 | 16.97 | 0.110 | 1.105 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **17.3x** [10.5x-23.1x] | 1.04 | 17.89 | 0.84 | 16.22 | 0.205 | 1.695 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **6.0x** [1.1x-14.7x] | 8.17 | 61.09 | 7.36 | 53.12 | 1.061 | 8.153 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **10.2x** [4.4x-24.8x] | 33.98 | 370.66 | 30.28 | 336.00 | 3.785 | 34.091 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## 5. Default Build vs. `GOEXPERIMENT=simd`

In [`internal/simd/kernels_amd64.go`](../internal/simd/kernels_amd64.go) and [`internal/simd/kernels_avx512_amd64.go`](../internal/simd/kernels_avx512_amd64.go), runtime CPUID dispatch selects AVX-512, AVX2, or 4-way scalar loops from a single portable `CGO_ENABLED=0` binary (`GOAMD64=v3` FMA-safe). Each vector kernel spills accumulators via `StoreArray` and executes `archsimd.ClearAVXUpperBits()` (`VZEROUPPER`) before scalar tail or caller operations, eliminating AVX-to-SSE transition stalls while preserving IEEE-754 `NaN` propagation parity. AVX2 activates at `minAVX2Len = 32` (`Dot64`, `Axpy64`, `Scale64`, `AbsMax64`, `RowActivityBoundsFinite`, `SelectDualLeaveRow64With`, `MinReducedSkip64With`, `InitReducedRow64`, `AllAbsLE64With`, `ValidateAndNegate64With`), `rows >= 8 && cols >= 8` for `Transpose64With`, `minDualRatioSIMDChunks = 4` (256 cols for `ScanDualRatioCand64With`), and `minAVX2Min2Len = 256` (`Min2ReducedRow64`); on AVX-512 hosts, dense LSAP loops dispatch AVX-512 at `minAVX512Len = 64` and `minAVX512Min2Len = 128` while sparse LP kernels gate AVX-512 at `minAVX512LPLen = 2048` / `minLeaveRowAVX512 = 2048` / `minDualRatioAVX512Chunks = 128` (8,192 cols) to avoid Intel Emerald Rapids License-2 clock down-binning across surrounding scalar LU code.

At the full-solver level, each ratio below is the geometric mean of per-trial `scalar / SIMD` solve times over the same instances on the local AVX2 host (`i9-13900K`), with a 95% $t$-interval on the log ratios; "no detectable effect" means the interval contains 1. "Reachable" is a necessary condition: the AVX2 LP kernels require `n + m >= 32`. Below that size no vector kernel can run, so those small models serve as negative controls where intervals excluding 1 reflect binary code-layout or run-to-run variance rather than SIMD execution. Per-family paired speedups and 95% intervals for larger models appear in the generated table below.

### 5.1 Scope and Pending Target-Hardware Verification

All telemetry in this document is measured locally on the AVX2-only `13th Gen Intel(R) Core(TM) i9-13900K` host (`x86_64`, Linux). Portable `GOOS=linux GOARCH=arm64 CGO_ENABLED=0` and `GOAMD64=v3`/`v4` builds compile and pass local build checks, whereas fresh physical AVX-512 and `linux/arm64` runtime test and benchmark re-verification on the current tree remain pending target hardware access.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 5 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 7 faster, 0 slower, 16 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.31x [1.12x-1.54x] | 0.006 | 0.004 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.47x [1.07x-2.02x] | 0.005 | 0.004 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 1.00x [0.81x-1.23x] | 0.006 | 0.006 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source | 1.05x [0.81x-1.35x] | 0.004 | 0.004 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.85x [0.76x-0.95x] | 0.004 | 0.005 | 11 |
| `t1_furniture` | 5 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.92x [0.86x-0.99x] | 0.016 | 0.016 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.82x [0.77x-0.87x] | 0.088 | 0.109 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 1.17x [0.93x-1.47x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 1.01x [0.97x-1.06x] | 0.041 | 0.040 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | SIMD faster | 1.07x [1.03x-1.12x] | 0.461 | 0.443 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 1.00x [0.99x-1.01x] | 2.097 | 2.094 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 1.01x [0.99x-1.02x] | 4.074 | 4.023 | 11 |
| `t2_cflp_36` | 47 | possible | no detectable effect | 1.00x [0.97x-1.04x] | 0.135 | 0.136 | 11 |
| `t2_cflp_49` | 62 | possible | no detectable effect | 1.06x [0.98x-1.14x] | 0.323 | 0.310 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 1.00x [0.96x-1.03x] | 0.960 | 0.960 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 0.97x [0.92x-1.02x] | 3.295 | 3.658 | 11 |
| `t2_binpack_44` | 58 | possible | SIMD faster | 1.04x [1.01x-1.06x] | 0.549 | 0.522 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 0.95x [0.88x-1.03x] | 0.144 | 0.158 | 11 |
| `t2_mad_g2_i8` | 72 | possible | SIMD faster | 1.12x [1.08x-1.17x] | 0.136 | 0.122 | 11 |
| `t2_mad_g3_i9` | 108 | possible | no detectable effect | 0.99x [0.88x-1.11x] | 0.144 | 0.152 | 11 |
| `t2_mad_g4_i12` | 156 | possible | SIMD faster | 1.03x [1.00x-1.06x] | 0.287 | 0.279 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.00x [0.97x-1.04x] | 3.096 | 3.039 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 1.01x [0.99x-1.04x] | 16.530 | 16.491 | 11 |
| `t3_mkp_200` | 208 | possible | SIMD faster | 1.07x [1.02x-1.11x] | 18.584 | 17.029 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.04x [0.99x-1.08x] | 5.991 | 5.895 | 5 |
| `t3_lotsizing_504` | 840 | possible | SIMD faster | 1.03x [1.00x-1.06x] | 61.620 | 60.926 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 1.07x [0.93x-1.24x] | 6.358 | 6.442 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 1.00x [0.98x-1.03x] | 0.420 | 0.424 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 0.77x [0.55x-1.06x] | 0.838 | 0.919 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 0.91x [0.78x-1.07x] | 7.364 | 7.165 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | SIMD faster | 1.11x [1.03x-1.19x] | 30.279 | 26.390 | 5 |
<!-- END GENERATED:SIMD_EFFECT_TABLE -->

---

## 6. Full Corpus Solver-Core Checkpoint Progression

<!-- BEGIN GENERATED:FULL_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_std_lp` | **0.01** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 54.00 | 54.00 |
| `t1_std_lp_max` | **0.00** (1 / 0) | 0.01 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 80.00 | 80.00 |
| `t1_std_mip` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.002 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.01 (3) | 0.002 | 64.95 | 58.90 |
| `t1_whiskas` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.003 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.002 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.09** (1 / 8) | 0.11 (2 / 8) | 0.14 (18) | 0.21 (35) | 0.007 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.04** (1 / 0) | 0.05 (1 / 0) | 0.06 (1) | 0.53 (4) | 0.084 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.46** (2 / 0) | 0.56 (12 / 0) | 0.55 (12) | 0.78 (109) | 0.103 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.10** (5 / 9) | 3.22 (40 / 9) | 1.03 (51) | 0.95 (61) | 0.025 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **4.07** (1 / 4) | 11.55 (2 / 4) | 4.69 (118) | 4.23 (111) | 0.038 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.14** (1 / 2) | 0.23 (3 / 2) | 0.18 (7) | 0.21 (7) | 0.025 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.32** (1 / 10) | 0.57 (4 / 10) | 0.38 (9) | 0.29 (9) | 0.036 | 909.02 | 827.30 |
| `t2_gap_45` | **0.96** (1 / 9) | 0.38 (1 / 9) | 2.15 (41) | 2.27 (60) | 0.027 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **3.29** (2 / 37) | 6.43 (25 / 37) | 2.98 (162) | 3.26 (199) | 0.061 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **0.55** (2 / 8) | 1.00 (5 / 8) | 8.23 (772) | 27.53 (1953) | 0.027 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.14** (1 / 6) | 0.36 (2 / 6) | 0.70 (6) | 0.64 (6) | 0.031 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.14** (1 / 0) | 0.81 (8 / 0) | 0.73 (8) | 0.73 (9) | 0.104 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.14** (1 / 0) | 0.13 (2 / 0) | 0.13 (2) | 2.75 (13) | 0.242 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.29** (1 / 0) | 3.23 (22 / 0) | 2.95 (22) | 5.36 (23) | 0.352 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.10** (43 / 0) | 3.09 (112 / 0) | 2.86 (112) | 3.71 (45) | 0.152 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **16.53** (201 / 0) | 30.37 (188 / 0) | 30.17 (188) | 92.68 (539) | 0.281 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **18.58** (250 / 14) | 82.00 (1135 / 16) | 165.47 (5000, `NodeLimit`) | 147.81 (5000, `NodeLimit`) | 0.118 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **5.99** (1 / 28) | 19.07 (54 / 34) | 8.59 (116) | 8.71 (118) | 0.171 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **61.62** (2 / 206) | 79.06 (376 / 206) | 250.04 (3736, `TimeLimit`) | 250.02 (3851, `TimeLimit`) | 0.709 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **6.36** (31 / 55) | 19.40 (87 / 53) | 250.05 (2574, `TimeLimit`) | 250.04 (2614, `TimeLimit`) | 0.183 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.42** (1 / 0) | 2.96 (17 / 0) | 3.50 (17) | 0.53 (1) | 0.667 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.84** (1 / 0) | 8.19 (58 / 0) | 7.57 (58) | 14.13 (74) | 1.547 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.36** (2 / 0) | 21.55 (25 / 0) | 20.12 (25) | 250.36 (147, `TimeLimit`) | 2.651 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **30.28** (1 / 0) | 32.84 (2 / 0) | 31.73 (2) | 251.28 (6, `TimeLimit`) | 57.486 | 208000.00 | 208000.00 |
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
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.44x** [1.22x-1.70x] | 1.44x | 4.68x | 0.219 (0.0195) | 0.322 (0.0295) | 1.51x [0.34x-2.60x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **2.97x** [1.86x-4.75x] | 2.97x | 3.02x | 9.012 (0.799) | 32.592 (2.941) | 2.59x [0.90x-15.96x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.83x** [1.85x-4.32x] | 2.83x | 3.50x | 221.630 (43.639) | 931.076 (186.558) | 2.48x [1.46x-5.71x] |
| Square families ($m = n$) | 16 (146) | **2.67x** [1.94x-3.68x] | 2.60x | 3.73x | 226.097 (43.619) | 953.161 (187.586) | 2.32x [0.34x-15.96x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.80x** [1.31x-2.48x] | 1.69x | 3.16x | 4.765 (0.839) | 10.829 (1.943) | 1.96x [0.90x-2.58x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.31x** [1.16x-4.61x] | 2.45x | 2.89x | 35.432 (6.161) | 64.733 (10.913) | 1.80x [1.09x-15.96x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.40x** [1.87x-3.07x] | 2.32x | 3.57x | 230.861 (44.458) | 963.990 (189.529) | 2.04x [0.34x-15.96x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 7.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.54x** | 0.0005 | 0.0010 | 0.0006 | 0.0138 | $0.0010 \pm 0.0010$ | $0.0011 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.29x** | 0.0017 | 0.0024 | 0.0020 | 0.0104 | $0.0019 \pm 0.0008$ | $0.0024 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.69x** | 0.0037 | 0.0062 | 0.0046 | 0.0153 | $0.0035 \pm 0.0007$ | $0.0061 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.16x** | 0.0037 | 0.0041 | 0.0045 | 0.0143 | $0.0035 \pm 0.0007$ | $0.0040 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.34x** | 0.0039 | 0.0050 | 0.0047 | 0.0156 | $0.0037 \pm 0.0008$ | $0.0051 \pm 0.0018$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.70x** | 0.0060 | 0.0109 | 0.0100 | 0.0329 | $0.0064 \pm 0.0016$ | $0.0107 \pm 0.0017$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.39x** | 0.0284 | 0.0703 | 0.0360 | 0.0954 | $0.0300 \pm 0.0109$ | $0.0690 \pm 0.0102$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.28x** | 0.0533 | 0.174 | 0.0679 | 0.224 | $0.0537 \pm 0.0072$ | $0.177 \pm 0.0261$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.13x** | 0.0470 | 0.0933 | 0.0693 | 0.145 | $0.0449 \pm 0.0038$ | $0.0961 \pm 0.0138$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.63x** | 0.136 | 0.218 | 0.147 | 0.257 | $0.146 \pm 0.0459$ | $0.233 \pm 0.0480$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **12.38x** | 0.0591 | 0.682 | 0.0766 | 0.759 | $0.0561 \pm 0.0084$ | $0.687 \pm 0.0235$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.95x** | 0.0350 | 0.0690 | 0.0597 | 0.142 | $0.0375 \pm 0.0121$ | $0.0710 \pm 0.0078$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.00x** | 0.0393 | 0.0771 | 0.0621 | 0.150 | $0.0398 \pm 0.0037$ | $0.0797 \pm 0.0073$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.56x** | 0.0795 | 0.279 | 0.102 | 0.354 | $0.0797 \pm 0.0080$ | $0.284 \pm 0.0324$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.83x** | 0.321 | 1.279 | 0.439 | 1.694 | $0.332 \pm 0.0454$ | $1.266 \pm 0.120$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.89x** | 1.213 | 5.208 | 1.661 | 7.899 | $1.400 \pm 0.408$ | $5.304 \pm 0.374$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.72x** | 0.864 | 1.493 | 1.464 | 4.012 | $0.864 \pm 0.0293$ | $1.486 \pm 0.0573$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.66x** | 5.044 | 8.410 | 5.553 | 9.251 | $5.657 \pm 2.181$ | $9.188 \pm 2.685$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.50x** | 0.340 | 0.816 | 0.718 | 3.628 | $0.335 \pm 0.0242$ | $0.835 \pm 0.0573$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.27x** | 0.418 | 0.972 | 0.865 | 3.699 | $0.433 \pm 0.0424$ | $0.979 \pm 0.0344$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.89x** | 5.798 | 27.873 | 7.491 | 36.444 | $5.794 \pm 0.196$ | $28.384 \pm 2.251$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.71x** | 29.962 | 141.786 | 38.331 | 184.352 | $29.844 \pm 3.780$ | $140.039 \pm 10.069$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 7.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSkip64`, `InitReducedRow64`, and `Min2ReducedRow64`)

In [`assign/solver.go`](../assign/solver.go), [`simd.AllAbsLE64With`](../internal/simd/kernels_amd64.go) and [`simd.ValidateAndNegate64With`](../internal/simd/assign_amd64.go) validate finite cost bounds via AVX2 checks whenever `len(cost) >= 32` (while the fused `colReduceValidateSquare` fast path skips `AllAbsLE64With` on finite square `Minimize` matrices and dispatches `ColReduceValidateRows64With` only for $n \ge 32$), [`simd.Transpose64With`](../internal/simd/assign_amd64.go) handles tall transposes with $\text{rows} \ge 8$ and $\text{cols} \ge 8$, [`simd.MinReducedSkip64With`](../internal/simd/assign_amd64.go) accelerates reduction-transfer slack scans in `preprocessSquareJV` ($n \ge 32$), [`simd.Min2ReducedRow64With`](../internal/simd/kernels_amd64.go) accelerates first/second reduced-minimum scans in `preprocessRectangularBV` and `augmentingRowReduction` at `minAVX2Min2Len = 256` (`minAVX512Min2Len = 128`), and [`simd.InitReducedRow64With`](../internal/simd/kernels_amd64.go) runs on a slice of length $\max(m, n) \ge 32$ at the start of each Dijkstra augmentation. All vector kernels clean upper `YMM` state via `StoreArray` + `ClearAVXUpperBits()` (`VZEROUPPER`) before scalar operations; exact per-family verdicts appear in the generated table below.

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` with no `>= 8x8` tall transpose and either fused square `Minimize` non-sparse path or `rows * cols < 32`, so no AVX2 kernel runs): 2 of 3 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32`, tall `rows > cols` with `>= 8x8`, or fallback `AllAbsLE64`/`ValidateAndNegate64` at `rows * cols >= 32`): 17 faster, 1 slower, 1 with no detectable effect, out of 19.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.97x [0.94x-1.00x] | 0.0005 | 0.0005 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.03x [1.00x-1.06x] | 0.0017 | 0.0017 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source | 0.99x [0.96x-1.02x] | 0.0037 | 0.0036 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | SIMD faster | 1.13x [1.10x-1.15x] | 0.0037 | 0.0032 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | SIMD faster | 1.16x [1.13x-1.20x] | 0.0039 | 0.0034 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | SIMD slower | 0.94x [0.91x-0.97x] | 0.0060 | 0.0064 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.21x [1.18x-1.24x] | 0.0284 | 0.0229 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.34x [1.27x-1.42x] | 0.0533 | 0.0402 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.60x [1.58x-1.63x] | 0.0470 | 0.0292 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.69x [1.63x-1.76x] | 0.136 | 0.0794 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.32x [1.24x-1.42x] | 0.0591 | 0.0437 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.24x [1.14x-1.35x] | 0.0350 | 0.0273 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.32x [1.25x-1.40x] | 0.0393 | 0.0294 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.39x [1.34x-1.43x] | 0.0795 | 0.0559 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | no detectable effect | 1.32x [0.90x-1.95x] | 0.321 | 0.219 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.82x [1.54x-2.15x] | 1.213 | 0.752 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.53x [2.32x-2.74x] | 0.864 | 0.348 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.59x [2.49x-2.69x] | 5.044 | 1.890 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.80x [1.56x-2.08x] | 0.340 | 0.191 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.70x [1.41x-2.05x] | 0.418 | 0.248 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.66x [1.52x-1.80x] | 5.798 | 3.509 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.52x [1.42x-1.64x] | 29.962 | 19.793 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

---

## 8. Production Binary Footprint and Linker Dead-Code Elimination

The `goblin-bench -binary-size` mode compiles four standalone `CGO_ENABLED=0` programs in an isolated temporary module against the live workspace across both default `go build` and `GOEXPERIMENT=simd`, saving compiled artifact sizes and `go tool nm -size` symbol counts to `bench/results/goblin_binary_size.json`. [`ValidateBinarySizeTelemetry`](../bench/docgen.go) rejects the telemetry bundle if `assign_default` links any `math/big`, `encoding/json`, `VIPR`, or `Certif` symbols, if `goblin_default` links any `encoding/json`, `VIPR`, or `Certif` symbols, or if any profile links `encoding/json`:

<!-- BEGIN GENERATED:BINARY_SIZE_TABLE -->
| Profile | Default Raw / Stripped (KiB) | `GOEXPERIMENT=simd` Raw / Stripped (KiB) | Stripped Delta vs. Default | `goblin` Text (KiB) | `math/big` Syms | `encoding/json` Syms | `VIPR` Syms | `Certif` Syms |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `assign` default (`LinearSum` + `SolveInto`) | 2784.8 / **1740.2** | 2813.7 / **1804.2** | baseline | 114.4 | `0` | `0` | `0` | `0` |
| `assign` certified (`+ assign.WithCertified(true)`) | 2858.9 / **1788.2** | 2891.1 / **1856.2** | +48.0 KiB (+2.8%) | 149.5 | `0` | `0` | `0` | `15` |
| `goblin` default (`Model.Solve`, `DefaultOptions`) | 5186.2 / **3300.2** | 5083.1 / **3320.2** | baseline | 862.1 | `130` | `0` | `0` | `0` |
| `goblin` certified + VIPR (`+ WithCertified + WithVIPRTrace`) | 6274.2 / **4032.2** | 6142.0 / **4052.2** | +732.0 KiB (+22.2%) | 1278.3 | `153` | `0` | `181` | `10` |
<!-- END GENERATED:BINARY_SIZE_TABLE -->

---

## 9. Keeping Documentation and Benchmarks Synchronized

1. **Regenerate telemetry** from a clean, committed tree in a single pinned session so that all eight telemetry files record identical `git_commit`, `source_fingerprint`, and `uname_release`: run the commands in the [README](../README.md#verification-benchmarking-and-documentation-sync-commands).
2. **Regenerate tables**: `go run ./cmd/goblin-bench -sync-docs`.
3. **Verify**: `go run ./cmd/goblin-bench -check-docs`, or `go test ./...`, which runs `TestDocumentationAndBenchmarkSync`. The gate validates corpus dimensions, parity, symbol identifiers, and provenance, and checks every generated block in `README.md`, `docs/benchmarks.md`, and `bench/results/final_report.md`.
