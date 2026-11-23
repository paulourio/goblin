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
- **Unified telemetry source**: fingerprint `42200995483aab66` (commit `54139b90d0894cb1295ac695699fa8a7fe881b20`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.50 ms median on a 1-variable MILP and 6.39 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
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
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **331.22x** [165.04x-664.72x] | 331.22x | 0.05x | 8.7 (0.8) | 195.0 (19.4) | 1073.2 (101.2) | 488.95x [43.00x-1768.59x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **17.31x** [9.36x-32.00x] | 17.31x | 0.17x | 724.1 (44.8) | 1991.3 (176.8) | 4329.3 (418.8) | 22.58x [0.14x-166.42x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **10.40x** [5.26x-20.58x] | 10.40x | 0.20x | 783.2 (140.2) | 2912.6 (560.7) | 6210.7 (1068.9) | 10.32x [1.13x-66.36x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **11.44x** [6.52x-20.08x] | 12.20x | 0.22x | 1198.2 (140.3) | 3148.6 (401.1) | 6798.3 (988.4) | 12.11x [0.14x-166.42x] |
| Tier 3 non-MAD MILPs | 4 (20) | **8.08x** [1.79x-36.45x] | 8.08x | 0.27x | 482.5 (96.3) | 1249.5 (232.3) | 2815.3 (600.5) | 6.03x [1.88x-66.36x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **26.81x** [12.30x-58.41x] | 34.79x | 0.11x | 310.2 (44.8) | 1780.1 (338.8) | 3848.5 (509.0) | 35.83x [1.13x-132.66x] |
| Random-weight MAD (negative controls) | 3 (33) | **16.82x** [0.82x-345.60x] | 16.82x | 0.14x | 296.2 (20.5) | 1218.3 (119.1) | 2257.8 (249.7) | 16.27x [1.31x-119.01x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **44.93x** [19.08x-105.80x] | 53.78x | 0.12x | 1205.8 (141.0) | 3318.8 (418.0) | 7764.7 (1079.8) | 43.00x [0.14x-1768.59x] |
| All 31 synthetic corpus families | 31 (293) | **39.33x** [20.55x-75.26x] | 48.90x | 0.12x | 1516.1 (185.8) | 5098.9 (756.9) | 11613.3 (1588.8) | 40.95x [0.14x-1768.59x] |
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
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **102.8x** [83.6x-132.7x] | 0.09 | 9.72 | 0.06 | 9.48 | 0.032 | 0.231 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **67.8x** [43.0x-119.0x] | 0.49 | 34.01 | 0.46 | 33.75 | 0.028 | 0.238 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **58.7x** [45.3x-70.6x] | 0.16 | 9.21 | 0.13 | 8.94 | 0.033 | 0.274 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **42.7x** [31.6x-53.0x] | 0.23 | 10.14 | 0.15 | 9.74 | 0.071 | 0.414 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **32.2x** [26.7x-35.8x] | 0.36 | 11.56 | 0.29 | 10.94 | 0.072 | 0.616 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **9.8x** [3.4x-27.8x] | 3.51 | 48.33 | 3.47 | 48.01 | 0.040 | 0.313 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **7.2x** [1.3x-29.6x] | 16.50 | 167.40 | 16.44 | 166.97 | 0.064 | 0.437 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **30.6x** [23.5x-35.9x] | 0.58 | 17.70 | 0.44 | 16.58 | 0.135 | 1.079 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **17.1x** [10.9x-23.5x] | 1.01 | 18.71 | 0.81 | 16.97 | 0.204 | 1.736 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **6.0x** [1.1x-13.7x] | 8.36 | 60.94 | 7.36 | 52.95 | 1.095 | 8.243 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **10.2x** [4.4x-24.6x] | 34.01 | 371.06 | 30.06 | 337.05 | 4.367 | 34.505 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Cross-Instance Latency Dispersion Across Randomized Instances (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.10 \pm 0.02$ ($\pm 0.01$) | $9.71 \pm 0.45$ ($\pm 0.30$) | $0.09 \pm 0.01$ ($\pm 0.01$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.49 \pm 0.23$ ($\pm 0.15$) | $30.88 \pm 9.57$ ($\pm 6.43$) | $0.49 \pm 0.22$ ($\pm 0.15$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.16 \pm 0.01$ ($\pm 0.01$) | $9.49 \pm 0.70$ ($\pm 0.47$) | $0.16 \pm 0.01$ ($\pm 0.01$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.24 \pm 0.04$ ($\pm 0.03$) | $10.20 \pm 0.23$ ($\pm 0.15$) | $0.21 \pm 0.06$ ($\pm 0.04$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.37 \pm 0.02$ ($\pm 0.01$) | $11.79 \pm 0.68$ ($\pm 0.46$) | $0.38 \pm 0.04$ ($\pm 0.02$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $4.18 \pm 3.12$ ($\pm 2.09$) | $35.80 \pm 16.98$ ($\pm 11.41$) | $4.18 \pm 3.21$ ($\pm 2.15$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $22.26 \pm 21.97$ ($\pm 14.76$) | $138.57 \pm 63.45$ ($\pm 42.62$) | $22.24 \pm 22.16$ ($\pm 14.89$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.58 \pm 0.02$ ($\pm 0.03$) | $17.99 \pm 2.39$ ($\pm 2.97$) | $0.61 \pm 0.01$ ($\pm 0.02$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.14 \pm 0.38$ ($\pm 0.47$) | $18.74 \pm 0.43$ ($\pm 0.53$) | $1.08 \pm 0.36$ ($\pm 0.45$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $16.43 \pm 20.74$ ($\pm 25.74$) | $63.01 \pm 3.29$ ($\pm 4.08$) | $16.59 \pm 20.18$ ($\pm 25.05$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $41.99 \pm 14.70$ ($\pm 18.25$) | $579.34 \pm 610.67$ ($\pm 758.12$) | $39.01 \pm 14.07$ ($\pm 17.47$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | **0.06** (1 / 0) | 0.04 (1 / 0) | 0.06 (1) | 0.53 (4) | 0.083 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.46** (2 / 0) | 0.56 (12 / 0) | 0.55 (12) | 0.77 (109) | 0.101 | 4601.00 | 4257.00 |
| `t2_mad_g2_i8` | **0.13** (1 / 0) | 0.76 (8 / 0) | 0.74 (8) | 0.73 (9) | 0.100 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.15** (1 / 0) | 0.13 (2 / 0) | 0.15 (2) | 2.73 (13) | 0.263 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.29** (1 / 0) | 3.22 (22 / 0) | 3.49 (22) | 5.20 (23) | 0.360 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.47** (43 / 0) | 2.91 (112 / 0) | 2.85 (112) | 3.64 (45) | 0.152 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **16.44** (201 / 0) | 29.97 (188 / 0) | 29.88 (188) | 91.10 (539) | 0.285 | 7720.00 | 6060.00 |
| `t3_mad_g5_i20` | **0.44** (1 / 0) | 3.25 (17 / 0) | 2.61 (17) | 0.52 (1) | 0.653 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.81** (1 / 0) | 8.10 (58 / 0) | 7.62 (58) | 13.06 (74) | 1.557 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.36** (2 / 0) | 21.31 (25 / 0) | 19.15 (25) | 250.38 (152, `TimeLimit`) | 2.658 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **30.06** (1 / 0) | 32.37 (2 / 0) | 31.04 (2) | 250.29 (6, `TimeLimit`) | 56.329 | 208000.00 | 208000.00 |
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
| `t1_std_lp` | 3 | 3 | **601.7x** [259.8x-842.8x] | 0.01 | 6.40 | 0.00 | 6.35 | 0.005 | 0.055 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **660.7x** [361.4x-1019.8x] | 0.01 | 6.23 | 0.00 | 6.18 | 0.005 | 0.051 | `OK` |
| `t1_std_mip` | 3 | 3 | **661.4x** [514.5x-821.2x] | 0.01 | 7.41 | 0.01 | 7.36 | 0.005 | 0.057 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **739.9x** [612.5x-1073.0x] | 0.01 | 7.46 | 0.00 | 7.40 | 0.005 | 0.058 | `OK` |
| `t1_whiskas` | 6 | 5 | **457.2x** [327.6x-632.9x] | 0.01 | 6.34 | 0.01 | 6.27 | 0.007 | 0.074 | `OK` |
| `t1_furniture` | 2 | 3 | **253.7x** [76.5x-677.3x] | 0.02 | 7.97 | 0.02 | 7.92 | 0.005 | 0.059 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **103.6x** [49.0x-310.2x] | 0.11 | 9.38 | 0.09 | 9.29 | 0.010 | 0.093 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **975.7x** [125.2x-1768.6x] | 0.01 | 6.23 | 0.00 | 6.19 | 0.005 | 0.044 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **102.8x** [83.6x-132.7x] | 0.09 | 9.72 | 0.06 | 9.48 | 0.032 | 0.231 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **67.8x** [43.0x-119.0x] | 0.49 | 34.01 | 0.46 | 33.75 | 0.028 | 0.238 | `OK` |
| `t2_mkp_30` | 30 | 6 | **11.0x** [6.2x-16.9x] | 2.30 | 27.74 | 2.27 | 27.55 | 0.022 | 0.181 | `OK` |
| `t2_mkp_50` | 50 | 8 | **6.0x** [3.2x-31.9x] | 4.28 | 16.63 | 4.24 | 16.37 | 0.033 | 0.263 | `OK` |
| `t2_cflp_36` | 36 | 11 | **56.0x** [17.9x-166.4x] | 0.16 | 9.49 | 0.14 | 9.32 | 0.018 | 0.172 | `OK` |
| `t2_cflp_49` | 49 | 13 | **30.3x** [18.3x-82.0x] | 0.40 | 10.26 | 0.35 | 10.07 | 0.035 | 0.212 | `OK` |
| `t2_gap_45` | 45 | 13 | **22.8x** [4.7x-79.5x] | 0.81 | 14.01 | 0.78 | 13.80 | 0.022 | 0.206 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **11.9x** [5.3x-28.1x] | 4.12 | 69.04 | 4.10 | 68.80 | 0.030 | 0.247 | `OK` |
| `t2_binpack_44` | 44 | 14 | **1.9x** [0.1x-29.6x] | 11.65 | 13.84 | 11.62 | 13.63 | 0.028 | 0.209 | `OK` |
| `t2_setcover_50` | 50 | 20 | **37.5x** [6.5x-102.2x] | 0.27 | 11.11 | 0.23 | 10.81 | 0.039 | 0.304 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **58.7x** [45.3x-70.6x] | 0.16 | 9.21 | 0.13 | 8.94 | 0.033 | 0.274 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **42.7x** [31.6x-53.0x] | 0.23 | 10.14 | 0.15 | 9.74 | 0.071 | 0.414 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **32.2x** [26.7x-35.8x] | 0.36 | 11.56 | 0.29 | 10.94 | 0.072 | 0.616 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.8x** [3.4x-27.8x] | 3.51 | 48.33 | 3.47 | 48.01 | 0.040 | 0.313 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **7.2x** [1.3x-29.6x] | 16.50 | 167.40 | 16.44 | 166.97 | 0.064 | 0.437 | `OK` |
| `t3_mkp_200` | 200 | 8 | **4.9x** [4.2x-6.2x] | 18.69 | 115.02 | 18.61 | 114.42 | 0.083 | 0.608 | `OK` |
| `t3_cflp_300` | 300 | 34 | **26.4x** [8.0x-66.4x] | 8.85 | 248.26 | 8.74 | 247.35 | 0.112 | 0.909 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **3.0x** [1.9x-4.9x] | 62.13 | 157.06 | 61.90 | 155.19 | 0.212 | 1.871 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **10.8x** [5.3x-17.7x] | 6.63 | 80.16 | 6.33 | 77.23 | 0.301 | 2.786 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **30.6x** [23.5x-35.9x] | 0.58 | 17.70 | 0.44 | 16.58 | 0.135 | 1.079 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **17.1x** [10.9x-23.5x] | 1.01 | 18.71 | 0.81 | 16.97 | 0.204 | 1.736 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **6.0x** [1.1x-13.7x] | 8.36 | 60.94 | 7.36 | 52.95 | 1.095 | 8.243 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **10.2x** [4.4x-24.6x] | 34.01 | 371.06 | 30.06 | 337.05 | 4.367 | 34.505 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## 5. Default Build vs. `GOEXPERIMENT=simd`

In [`internal/simd/kernels_amd64.go`](../internal/simd/kernels_amd64.go) and [`internal/simd/kernels_avx512_amd64.go`](../internal/simd/kernels_avx512_amd64.go), runtime CPUID dispatch selects AVX-512, AVX2, or 4-way scalar loops from a single portable `CGO_ENABLED=0` binary (`GOAMD64=v3` FMA-safe). Each vector kernel spills accumulators via `StoreArray` and executes `archsimd.ClearAVXUpperBits()` (`VZEROUPPER`) before scalar tail or caller operations, eliminating AVX-to-SSE transition stalls while preserving IEEE-754 `NaN` propagation parity. AVX2 activates at `minAVX2Len = 32` (`Dot64`, `Axpy64`, `Scale64`, `AbsMax64`, `RowActivityBoundsFinite`, `SelectDualLeaveRow64With`, `MinReducedSkip64With`, `InitReducedRow64`, `AllAbsLE64With`, `ValidateAndNegate64With`), `rows >= 8 && cols >= 8` for `Transpose64With`, `minDualRatioSIMDChunks = 4` (256 cols for `ScanDualRatioCand64With`), and `minAVX2Min2Len = 256` (`Min2ReducedRow64`); on AVX-512 hosts, dense LSAP loops dispatch AVX-512 at `minAVX512Len = 64` and `minAVX512Min2Len = 128` while sparse LP kernels gate AVX-512 at `minAVX512LPLen = 2048` / `minLeaveRowAVX512 = 2048` / `minDualRatioAVX512Chunks = 128` (8,192 cols) to avoid Intel Emerald Rapids License-2 clock down-binning across surrounding scalar LU code.

At the full-solver level, each ratio below is the geometric mean of per-trial `scalar / SIMD` solve times over the same instances on the local AVX2 host (`i9-13900K`), with a 95% $t$-interval on the log ratios; "no detectable effect" means the interval contains 1. "Reachable" is a necessary condition: the AVX2 LP kernels require `n + m >= 32`. Below that size no vector kernel can run, so those small models serve as negative controls where intervals excluding 1 reflect binary code-layout or run-to-run variance rather than SIMD execution. Per-family paired speedups and 95% intervals for larger models appear in the generated table below.

### 5.1 Scope and Pending Target-Hardware Verification

All telemetry in this document is measured locally on the AVX2-only `13th Gen Intel(R) Core(TM) i9-13900K` host (`x86_64`, Linux). Portable `GOOS=linux GOARCH=arm64 CGO_ENABLED=0` and `GOAMD64=v3`/`v4` builds compile and pass local build checks, whereas fresh physical AVX-512 and `linux/arm64` runtime test and benchmark re-verification on the current tree remain pending target hardware access.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 0 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 4 faster, 1 slower, 18 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 1.12x [0.77x-1.63x] | 0.004 | 0.004 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 0.97x [0.93x-1.00x] | 0.004 | 0.004 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 1.00x [0.97x-1.04x] | 0.006 | 0.006 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source | 1.00x [0.99x-1.02x] | 0.004 | 0.004 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 1.01x [0.99x-1.03x] | 0.006 | 0.005 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 0.99x [0.92x-1.06x] | 0.017 | 0.017 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source | 0.99x [0.97x-1.00x] | 0.093 | 0.094 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 0.84x [0.66x-1.07x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 1.12x [0.96x-1.30x] | 0.060 | 0.055 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 1.00x [0.99x-1.01x] | 0.458 | 0.456 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 0.97x [0.84x-1.13x] | 2.274 | 2.108 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 1.00x [0.98x-1.02x] | 4.245 | 4.311 | 11 |
| `t2_cflp_36` | 47 | possible | no detectable effect | 1.01x [0.99x-1.03x] | 0.143 | 0.140 | 11 |
| `t2_cflp_49` | 62 | possible | no detectable effect | 1.01x [0.97x-1.05x] | 0.354 | 0.360 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 1.00x [0.96x-1.04x] | 0.782 | 0.766 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 0.99x [0.98x-1.00x] | 4.098 | 4.147 | 11 |
| `t2_binpack_44` | 58 | possible | no detectable effect | 0.98x [0.96x-1.01x] | 11.617 | 11.660 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 1.03x [0.92x-1.14x] | 0.234 | 0.225 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 1.00x [0.96x-1.04x] | 0.126 | 0.127 | 11 |
| `t2_mad_g3_i9` | 108 | possible | SIMD faster | 1.14x [1.04x-1.25x] | 0.148 | 0.130 | 11 |
| `t2_mad_g4_i12` | 156 | possible | no detectable effect | 0.97x [0.91x-1.02x] | 0.288 | 0.297 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.01x [0.96x-1.07x] | 3.471 | 2.964 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 1.01x [0.99x-1.02x] | 16.438 | 16.310 | 11 |
| `t3_mkp_200` | 208 | possible | SIMD faster | 1.06x [1.02x-1.11x] | 18.612 | 17.485 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.04x [0.96x-1.12x] | 8.737 | 8.584 | 5 |
| `t3_lotsizing_504` | 840 | possible | SIMD faster | 1.03x [1.02x-1.04x] | 61.904 | 60.738 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 1.03x [0.89x-1.19x] | 6.328 | 6.178 | 5 |
| `t3_mad_g5_i20` | 240 | possible | SIMD slower | 0.96x [0.94x-0.98x] | 0.441 | 0.465 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 1.05x [0.95x-1.15x] | 0.813 | 0.815 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 0.97x [0.92x-1.02x] | 7.363 | 7.873 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | SIMD faster | 1.08x [1.05x-1.11x] | 30.056 | 27.489 | 5 |
<!-- END GENERATED:SIMD_EFFECT_TABLE -->

---

## 6. Full Corpus Solver-Core Checkpoint Progression

<!-- BEGIN GENERATED:FULL_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_std_lp` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 54.00 | 54.00 |
| `t1_std_lp_max` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 80.00 | 80.00 |
| `t1_std_mip` | **0.01** (1 / 0) | 0.00 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.002 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.002 | 64.95 | 58.90 |
| `t1_whiskas` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.003 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.02 (1) | 0.02 (1) | 0.002 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.09** (1 / 8) | 0.11 (2 / 8) | 0.14 (18) | 0.23 (35) | 0.007 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.002 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.06** (1 / 0) | 0.04 (1 / 0) | 0.06 (1) | 0.53 (4) | 0.083 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.46** (2 / 0) | 0.56 (12 / 0) | 0.55 (12) | 0.77 (109) | 0.101 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.27** (5 / 9) | 2.66 (40 / 9) | 1.04 (51) | 0.96 (61) | 0.025 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **4.24** (1 / 4) | 11.50 (2 / 4) | 4.72 (118) | 4.29 (111) | 0.039 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.14** (1 / 2) | 0.21 (3 / 2) | 0.16 (7) | 0.17 (7) | 0.027 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.35** (1 / 7) | 0.65 (3 / 7) | 0.32 (9) | 0.32 (9) | 0.037 | 909.02 | 827.30 |
| `t2_gap_45` | **0.78** (1 / 7) | 0.41 (1 / 7) | 1.89 (41) | 2.03 (60) | 0.025 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **4.10** (2 / 37) | 7.76 (25 / 37) | 3.01 (162) | 3.30 (199) | 0.059 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **11.62** (274 / 12) | 13.85 (298 / 12) | 8.61 (772) | 26.64 (1941) | 0.026 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.23** (1 / 6) | 0.35 (2 / 6) | 0.67 (6) | 0.80 (6) | 0.033 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.13** (1 / 0) | 0.76 (8 / 0) | 0.74 (8) | 0.73 (9) | 0.100 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.15** (1 / 0) | 0.13 (2 / 0) | 0.15 (2) | 2.73 (13) | 0.263 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.29** (1 / 0) | 3.22 (22 / 0) | 3.49 (22) | 5.20 (23) | 0.360 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.47** (43 / 0) | 2.91 (112 / 0) | 2.85 (112) | 3.64 (45) | 0.152 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **16.44** (201 / 0) | 29.97 (188 / 0) | 29.88 (188) | 91.10 (539) | 0.285 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **18.61** (250 / 14) | 81.32 (1135 / 16) | 165.00 (5000, `NodeLimit`) | 148.05 (5000, `NodeLimit`) | 0.119 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **8.74** (1 / 28) | 18.13 (54 / 34) | 8.93 (116) | 8.64 (118) | 0.169 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **61.90** (2 / 206) | 79.20 (376 / 206) | 250.03 (3682, `TimeLimit`) | 250.02 (3792, `TimeLimit`) | 0.712 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **6.33** (31 / 55) | 19.69 (87 / 53) | 250.11 (2561, `TimeLimit`) | 250.10 (2591, `TimeLimit`) | 0.182 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.44** (1 / 0) | 3.25 (17 / 0) | 2.61 (17) | 0.52 (1) | 0.653 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.81** (1 / 0) | 8.10 (58 / 0) | 7.62 (58) | 13.06 (74) | 1.557 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.36** (2 / 0) | 21.31 (25 / 0) | 19.15 (25) | 250.38 (152, `TimeLimit`) | 2.658 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **30.06** (1 / 0) | 32.37 (2 / 0) | 31.04 (2) | 250.29 (6, `TimeLimit`) | 56.329 | 208000.00 | 208000.00 |
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
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.35x** [1.14x-1.60x] | 1.35x | 4.28x | 0.222 (0.0199) | 0.313 (0.0287) | 1.44x [0.25x-2.44x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **3.03x** [1.83x-5.02x] | 3.03x | 3.06x | 9.060 (0.802) | 34.044 (3.089) | 2.62x [0.87x-18.10x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.84x** [1.85x-4.36x] | 2.84x | 3.55x | 222.382 (44.234) | 938.451 (188.031) | 2.44x [1.42x-6.16x] |
| Square families ($m = n$) | 16 (146) | **2.68x** [1.90x-3.77x] | 2.61x | 3.70x | 226.881 (44.186) | 962.001 (189.202) | 2.23x [0.25x-18.10x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.74x** [1.25x-2.44x] | 1.63x | 3.06x | 4.783 (0.870) | 10.807 (1.946) | 1.78x [0.87x-2.77x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.36x** [1.13x-4.93x] | 2.51x | 2.96x | 35.990 (6.351) | 66.145 (11.095) | 1.80x [1.04x-18.10x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.38x** [1.83x-3.10x] | 2.29x | 3.52x | 231.664 (45.056) | 972.808 (191.148) | 2.01x [0.25x-18.10x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 7.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.25x** | 0.0005 | 0.0009 | 0.0006 | 0.0120 | $0.0010 \pm 0.0010$ | $0.0009 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.26x** | 0.0017 | 0.0023 | 0.0020 | 0.0102 | $0.0019 \pm 0.0008$ | $0.0023 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.67x** | 0.0036 | 0.0063 | 0.0047 | 0.0155 | $0.0036 \pm 0.0008$ | $0.0061 \pm 0.0017$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.14x** | 0.0037 | 0.0039 | 0.0046 | 0.0142 | $0.0035 \pm 0.0007$ | $0.0040 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.23x** | 0.0039 | 0.0044 | 0.0049 | 0.0152 | $0.0037 \pm 0.0008$ | $0.0045 \pm 0.0009$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.65x** | 0.0063 | 0.0108 | 0.0102 | 0.0329 | $0.0065 \pm 0.0016$ | $0.0107 \pm 0.0018$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.39x** | 0.0286 | 0.0711 | 0.0359 | 0.0963 | $0.0301 \pm 0.0107$ | $0.0693 \pm 0.0108$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.51x** | 0.0526 | 0.181 | 0.0677 | 0.231 | $0.0519 \pm 0.0046$ | $0.183 \pm 0.0262$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.23x** | 0.0476 | 0.101 | 0.0709 | 0.159 | $0.0463 \pm 0.0042$ | $0.104 \pm 0.0135$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.68x** | 0.137 | 0.228 | 0.151 | 0.266 | $0.148 \pm 0.0497$ | $0.241 \pm 0.0449$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **14.11x** | 0.0597 | 0.786 | 0.0774 | 0.862 | $0.0561 \pm 0.0088$ | $0.783 \pm 0.0343$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.89x** | 0.0365 | 0.0691 | 0.0630 | 0.142 | $0.0387 \pm 0.0125$ | $0.0710 \pm 0.0081$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **1.85x** | 0.0422 | 0.0793 | 0.0660 | 0.152 | $0.0436 \pm 0.0074$ | $0.0799 \pm 0.0070$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.56x** | 0.0825 | 0.281 | 0.106 | 0.356 | $0.0800 \pm 0.0080$ | $0.285 \pm 0.0290$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.91x** | 0.315 | 1.292 | 0.438 | 1.684 | $0.329 \pm 0.0476$ | $1.279 \pm 0.126$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.88x** | 1.221 | 5.190 | 1.638 | 7.729 | $1.408 \pm 0.436$ | $5.314 \pm 0.379$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.71x** | 0.864 | 1.478 | 1.468 | 4.119 | $0.867 \pm 0.0347$ | $1.479 \pm 0.0524$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.64x** | 5.232 | 8.486 | 5.816 | 9.371 | $5.758 \pm 2.231$ | $9.231 \pm 2.638$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.50x** | 0.345 | 0.823 | 0.738 | 3.628 | $0.337 \pm 0.0284$ | $0.844 \pm 0.0594$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.29x** | 0.439 | 0.966 | 0.812 | 3.821 | $0.423 \pm 0.0293$ | $0.967 \pm 0.0263$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.98x** | 5.792 | 27.988 | 7.496 | 36.841 | $5.731 \pm 0.352$ | $28.559 \pm 2.240$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.74x** | 30.341 | 143.101 | 39.483 | 185.731 | $29.953 \pm 3.650$ | $141.297 \pm 8.937$ | 16450.52 | `OK` |
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
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source | 1.00x [0.97x-1.03x] | 0.0005 | 0.0005 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.03x [1.00x-1.06x] | 0.0017 | 0.0017 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source | 1.01x [0.99x-1.04x] | 0.0036 | 0.0035 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | SIMD faster | 1.12x [1.10x-1.14x] | 0.0037 | 0.0033 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | SIMD faster | 1.17x [1.14x-1.20x] | 0.0039 | 0.0033 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | no detectable effect | 0.89x [0.74x-1.08x] | 0.0063 | 0.0069 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.22x [1.19x-1.26x] | 0.0286 | 0.0227 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.32x [1.25x-1.39x] | 0.0526 | 0.0400 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.70x [1.58x-1.82x] | 0.0476 | 0.0295 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.68x [1.59x-1.77x] | 0.137 | 0.0803 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.30x [1.25x-1.36x] | 0.0597 | 0.0452 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.30x [1.20x-1.39x] | 0.0365 | 0.0276 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.44x [1.35x-1.54x] | 0.0422 | 0.0308 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.38x [1.33x-1.44x] | 0.0825 | 0.0569 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.59x [1.49x-1.70x] | 0.315 | 0.196 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.86x [1.54x-2.25x] | 1.221 | 0.743 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.51x [2.36x-2.67x] | 0.864 | 0.347 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.64x [2.50x-2.79x] | 5.232 | 1.879 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.85x [1.55x-2.21x] | 0.345 | 0.191 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.56x [1.31x-1.86x] | 0.439 | 0.280 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.61x [1.52x-1.69x] | 5.792 | 3.549 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.52x [1.42x-1.62x] | 30.341 | 19.693 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

---

## 8. Production Binary Footprint and Linker Dead-Code Elimination

The `goblin-bench -binary-size` mode compiles four standalone `CGO_ENABLED=0` programs in an isolated temporary module against the live workspace across both default `go build` and `GOEXPERIMENT=simd`, saving compiled artifact sizes and `go tool nm -size` symbol counts to `bench/results/goblin_binary_size.json`. [`ValidateBinarySizeTelemetry`](../bench/docgen.go) rejects the telemetry bundle if `assign_default` links any `math/big`, `encoding/json`, `VIPR`, or `Certif` symbols, if `goblin_default` links any `encoding/json`, `VIPR`, or `Certif` symbols, or if any profile links `encoding/json`:

<!-- BEGIN GENERATED:BINARY_SIZE_TABLE -->
| Profile | Default Raw / Stripped (KiB) | `GOEXPERIMENT=simd` Raw / Stripped (KiB) | Stripped Delta vs. Default | `goblin` Text (KiB) | `math/big` Syms | `encoding/json` Syms | `VIPR` Syms | `Certif` Syms |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `assign` default (`LinearSum` + `SolveInto`) | 2784.8 / **1740.2** | 2813.7 / **1804.2** | baseline | 114.4 | `0` | `0` | `0` | `0` |
| `assign` certified (`+ assign.WithCertified(true)`) | 2871.2 / **1796.2** | 2898.1 / **1860.2** | +56.0 KiB (+3.2%) | 154.8 | `0` | `0` | `0` | `16` |
| `goblin` default (`Model.Solve`, `DefaultOptions`) | 5324.5 / **3392.2** | 5221.6 / **3412.2** | baseline | 919.2 | `130` | `0` | `0` | `0` |
| `goblin` certified + VIPR (`+ WithCertified + WithVIPRTrace`) | 6472.9 / **4164.2** | 6338.6 / **4184.2** | +772.0 KiB (+22.8%) | 1360.0 | `153` | `0` | `197` | `10` |
<!-- END GENERATED:BINARY_SIZE_TABLE -->

---

## 9. Pure-Go vs. In-Process C++ CBC Performance Analysis

Comparing isolated unpresolved `Root LP` timings, paired trial ratios, and solver-core checkpoints (`CP5 +Presolve`, `CP6 +Cuts`, `CP7 Full`) between `BackendGo` and in-process `BackendCBC` (`c1c171ec`) highlights four structural trade-offs on large degenerate LPs and heavy-tail branch-and-cut models.

### 9.1 MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families ($N = G(I + T + 3)$ variables, $M = G(2T + 3) + I$ constraints) separate solver behavior into three scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Paired `Go / CBC` | Go Total Med (ms) | CBC Total Med (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.04x** | 0.09 | 2.48 | 0.60x | 0.06 (1 / 19) | 2.45 (0 / 0) | 0.083 (28) | 0.139 (19) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.05x** | 0.49 | 11.16 | 0.70x | 0.46 (2 / 58) | 11.13 (6 / 83) | 0.101 (30) | 0.143 (18) |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.07x** | 0.16 | 2.04 | 0.62x | 0.13 (1 / 26) | 2.00 (0 / 0) | 0.100 (33) | 0.162 (21) |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.09x** | 0.23 | 2.62 | 0.94x | 0.15 (1 / 40) | 2.56 (0 / 0) | 0.263 (58) | 0.280 (40) |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.10x** | 0.36 | 3.26 | 0.91x | 0.29 (1 / 51) | 3.19 (0 / 0) | 0.360 (80) | 0.396 (63) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.25x** | 3.51 | 16.69 | 1.00x | 3.47 (43 / 437) | 16.64 (12 / 216) | 0.152 (38) | 0.152 (24) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.24x** | 16.50 | 91.29 | 1.00x | 16.44 (201 / 2102) | 91.22 (198 / 1300) | 0.285 (65) | 0.286 (54) |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.07x** | 0.58 | 8.11 | 1.03x | 0.44 (1 / 75) | 7.98 (0 / 0) | 0.653 (91) | 0.635 (94) |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.15x** | 1.01 | 7.50 | 1.50x | 0.81 (1 / 98) | 7.30 (0 / 0) | 1.557 (244) | 1.040 (149) |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **0.29x** | 8.36 | 35.78 | 0.73x | 7.36 (2 / 364) | 34.81 (0 / 0) | 2.658 (224) | 3.627 (319) |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **0.17x** | 34.01 | 277.04 | 1.59x | 30.06 (1 / 2439) | 273.22 (0 / 0) | 56.329 (1900) | 35.319 (3128) |
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
| `t3_mkp_200` | $200 \times 8$ | **0.48x** | $21.67 \pm 13.40$ | $40.38 \pm 19.15$ | 18.61 (250 / 2447) | 49.38 (154 / 1145) | 81.32 (1135 / 16) | 165.00 (5000, `NodeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **0.35x** | $5.90 \pm 6.23$ | $12.69 \pm 9.49$ | 4.24 (1 / 19) | 9.27 (0 / 8) | 11.50 (2 / 4) | 4.72 (118) |
| `t2_binpack_44` | $44 \times 14$ | **1.12x** | $21.96 \pm 41.53$ | $6.88 \pm 2.25$ | 11.62 (274 / 1988) | 6.29 (0 / 8) | 13.85 (298 / 12) | 8.61 (772) |
| `t3_cflp_300` | $300 \times 34$ | **0.16x** | $10.63 \pm 7.32$ | $55.27 \pm 11.73$ | 8.74 (1 / 105) | 50.52 (18 / 483) | 18.13 (54 / 34) | 8.93 (116) |
| `t3_lotsizing_504` | $504 \times 336$ | **0.55x** | $55.87 \pm 34.49$ | $92.83 \pm 56.97$ | 61.90 (2 / 1061) | 71.19 (10 / 377) | 79.20 (376 / 206) | 250.03 (3682, `TimeLimit`) |
| `t3_sched_1000` | $1000 \times 100$ | **0.12x** | $8.34 \pm 4.24$ | $61.42 \pm 4.69$ | 6.33 (31 / 474) | 60.41 (0 / 76) | 19.69 (87 / 53) | 250.11 (2561, `TimeLimit`) |
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

# 11. Lean 4 Prove Lane build (78 jobs), arm64 + amd64 ELF verification, CLI/JSONL checks, and 19 formal Fuzz corpora
./scripts/verify_formal.sh
PYTHONDONTWRITEBYTECODE=1 python3 scripts/extract_elf_track5.py --verify
PYTHONDONTWRITEBYTECODE=1 python3 scripts/extract_amd64_track5e.py --verify
```

Running `go test ./...` also executes `TestDocumentationAndBenchmarkSync`, which validates corpus dimensions, parity, symbol identifiers, provenance, and every generated block across `README.md`, `docs/benchmarks.md`, and `bench/results/final_report.md`.
