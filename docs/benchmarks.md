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
- **Unified telemetry source**: fingerprint `907d83c35a5be855` (working tree on top of commit `1ba2236d5b5ba160c2f4ba70990b6799b866b2a4`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 7.17 ms median on a 1-variable MILP and 6.75 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
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
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **358.29x** [164.27x-781.50x] | 358.29x | 0.05x | 9.2 (0.8) | 150.9 (14.1) | 1104.5 (103.9) | 552.87x [41.05x-1766.43x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **19.38x** [11.65x-32.21x] | 19.38x | 0.18x | 559.2 (36.0) | 1712.9 (152.1) | 4446.3 (429.2) | 23.03x [1.26x-149.47x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **9.56x** [4.65x-19.67x] | 9.56x | 0.27x | 921.1 (174.5) | 3047.1 (608.1) | 6560.1 (1123.2) | 9.30x [1.06x-67.66x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **12.47x** [7.37x-21.12x] | 13.69x | 0.22x | 1145.9 (161.6) | 2903.0 (379.1) | 7107.3 (1038.3) | 12.56x [1.26x-149.47x] |
| Tier 3 non-MAD MILPs | 4 (20) | **7.55x** [1.30x-43.80x] | 7.55x | 0.31x | 595.8 (126.4) | 1249.3 (232.1) | 3020.2 (641.4) | 6.08x [1.44x-67.66x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **25.57x** [11.45x-57.10x] | 33.84x | 0.17x | 335.4 (49.0) | 1869.4 (382.2) | 4004.6 (523.8) | 34.65x [1.06x-137.54x] |
| Random-weight MAD (negative controls) | 3 (33) | **16.07x** [0.77x-333.30x] | 16.07x | 0.19x | 328.3 (21.7) | 968.7 (97.1) | 2328.4 (255.3) | 14.90x [1.26x-112.41x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **48.92x** [20.74x-115.43x] | 59.51x | 0.12x | 1154.1 (162.3) | 3041.4 (392.1) | 8106.3 (1132.6) | 46.53x [1.26x-1766.43x] |
| All 31 synthetic corpus families | 31 (293) | **41.38x** [21.50x-79.66x] | 52.61x | 0.13x | 1489.6 (211.3) | 4910.8 (774.3) | 12110.9 (1656.4) | 42.38x [1.06x-1766.43x] |
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
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **106.5x** [58.9x-137.5x] | 0.08 | 9.63 | 0.05 | 9.40 | 0.029 | 0.223 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **64.9x** [41.0x-112.4x] | 0.50 | 35.61 | 0.47 | 35.30 | 0.029 | 0.252 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **52.9x** [34.0x-73.8x] | 0.17 | 9.91 | 0.14 | 9.59 | 0.033 | 0.312 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **53.8x** [42.4x-63.7x] | 0.19 | 10.76 | 0.13 | 10.32 | 0.053 | 0.442 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **28.0x** [21.5x-34.8x] | 0.42 | 11.68 | 0.33 | 11.05 | 0.088 | 0.628 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **9.4x** [3.5x-25.1x] | 3.25 | 47.12 | 3.19 | 46.82 | 0.042 | 0.316 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **6.8x** [1.3x-28.6x] | 17.92 | 172.55 | 17.86 | 172.09 | 0.064 | 0.461 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **23.5x** [19.1x-31.3x] | 0.77 | 18.39 | 0.52 | 17.28 | 0.284 | 1.073 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **15.9x** [8.2x-23.3x] | 1.11 | 19.20 | 0.89 | 17.34 | 0.292 | 1.850 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **5.8x** [1.1x-15.0x] | 9.61 | 63.04 | 8.38 | 54.50 | 1.056 | 8.571 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **10.0x** [4.4x-23.7x] | 36.68 | 381.17 | 31.79 | 346.44 | 4.619 | 36.323 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Cross-Instance Latency Dispersion Across Randomized Instances (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.09 \pm 0.03$ ($\pm 0.02$) | $9.58 \pm 0.43$ ($\pm 0.29$) | $0.10 \pm 0.03$ ($\pm 0.02$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.53 \pm 0.25$ ($\pm 0.17$) | $31.90 \pm 9.95$ ($\pm 6.68$) | $0.53 \pm 0.25$ ($\pm 0.17$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.20 \pm 0.05$ ($\pm 0.03$) | $10.16 \pm 0.72$ ($\pm 0.48$) | $0.19 \pm 0.01$ ($\pm 0.01$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.20 \pm 0.03$ ($\pm 0.02$) | $10.66 \pm 0.41$ ($\pm 0.28$) | $0.30 \pm 0.07$ ($\pm 0.05$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.43 \pm 0.07$ ($\pm 0.04$) | $11.84 \pm 0.55$ ($\pm 0.37$) | $0.38 \pm 0.01$ ($\pm 0.01$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $4.49 \pm 3.56$ ($\pm 2.39$) | $36.57 \pm 17.56$ ($\pm 11.80$) | $4.49 \pm 3.59$ ($\pm 2.41$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $24.83 \pm 25.15$ ($\pm 16.90$) | $143.20 \pm 65.45$ ($\pm 43.97$) | $24.94 \pm 25.11$ ($\pm 16.87$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.79 \pm 0.11$ ($\pm 0.13$) | $18.51 \pm 2.47$ ($\pm 3.06$) | $0.58 \pm 0.01$ ($\pm 0.01$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.31 \pm 0.59$ ($\pm 0.74$) | $19.36 \pm 0.30$ ($\pm 0.38$) | $1.17 \pm 0.37$ ($\pm 0.46$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $18.13 \pm 23.14$ ($\pm 28.73$) | $65.35 \pm 3.88$ ($\pm 4.82$) | $18.30 \pm 24.00$ ($\pm 29.80$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $44.85 \pm 15.78$ ($\pm 19.59$) | $604.75 \pm 632.22$ ($\pm 784.88$) | $41.87 \pm 16.63$ ($\pm 20.65$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | **0.05** (1 / 0) | 0.06 (1 / 0) | 0.07 (1) | 0.64 (4) | 0.091 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.47** (2 / 0) | 0.60 (12 / 0) | 0.58 (12) | 0.82 (109) | 0.109 | 4601.00 | 4257.00 |
| `t2_mad_g2_i8` | **0.14** (1 / 0) | 0.87 (8 / 0) | 0.77 (8) | 0.77 (9) | 0.109 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.13** (1 / 0) | 0.23 (2 / 0) | 0.13 (2) | 2.89 (13) | 0.261 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.33** (1 / 0) | 3.60 (22 / 0) | 3.26 (22) | 5.56 (23) | 0.366 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.19** (43 / 0) | 3.30 (112 / 0) | 3.20 (112) | 3.99 (45) | 0.169 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **17.86** (196 / 0) | 34.21 (188 / 0) | 33.40 (188) | 98.83 (539) | 0.307 | 7720.00 | 6060.00 |
| `t3_mad_g5_i20` | **0.52** (1 / 0) | 3.34 (17 / 0) | 2.99 (17) | 0.54 (1) | 0.724 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.89** (1 / 0) | 9.57 (58 / 0) | 8.89 (58) | 14.90 (74) | 1.740 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **8.38** (2 / 0) | 22.66 (25 / 0) | 20.63 (25) | 250.26 (145, `TimeLimit`) | 3.022 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **31.79** (1 / 0) | 34.99 (2 / 0) | 33.67 (2) | 251.05 (6, `TimeLimit`) | 60.313 | 208000.00 | 208000.00 |
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
| `t1_std_lp` | 3 | 3 | **865.2x** [604.4x-1015.3x] | 0.01 | 6.51 | 0.00 | 6.46 | 0.004 | 0.054 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **943.0x** [655.8x-1200.5x] | 0.01 | 6.36 | 0.00 | 6.31 | 0.003 | 0.051 | `OK` |
| `t1_std_mip` | 3 | 3 | **868.2x** [564.4x-1115.1x] | 0.01 | 7.51 | 0.00 | 7.46 | 0.004 | 0.057 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **668.4x** [540.2x-878.0x] | 0.01 | 7.84 | 0.01 | 7.78 | 0.006 | 0.065 | `OK` |
| `t1_whiskas` | 6 | 5 | **383.6x** [176.1x-541.3x] | 0.02 | 6.50 | 0.01 | 6.42 | 0.008 | 0.079 | `OK` |
| `t1_furniture` | 2 | 3 | **236.2x** [71.2x-695.0x] | 0.03 | 7.95 | 0.02 | 7.90 | 0.006 | 0.055 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **96.0x** [46.5x-285.9x] | 0.12 | 9.62 | 0.11 | 9.53 | 0.012 | 0.095 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **1224.2x** [786.6x-1766.4x] | 0.00 | 6.41 | 0.00 | 6.37 | 0.005 | 0.046 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **106.5x** [58.9x-137.5x] | 0.08 | 9.63 | 0.05 | 9.40 | 0.029 | 0.223 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **64.9x** [41.0x-112.4x] | 0.50 | 35.61 | 0.47 | 35.30 | 0.029 | 0.252 | `OK` |
| `t2_mkp_30` | 30 | 6 | **10.0x** [5.4x-14.7x] | 2.43 | 29.00 | 2.41 | 28.82 | 0.021 | 0.186 | `OK` |
| `t2_mkp_50` | 50 | 8 | **5.0x** [2.5x-31.7x] | 5.24 | 16.84 | 5.21 | 16.56 | 0.032 | 0.267 | `OK` |
| `t2_cflp_36` | 36 | 11 | **47.9x** [20.0x-149.5x] | 0.18 | 9.24 | 0.16 | 9.06 | 0.020 | 0.169 | `OK` |
| `t2_cflp_49` | 49 | 13 | **35.0x** [21.5x-91.7x] | 0.36 | 10.97 | 0.33 | 10.73 | 0.025 | 0.215 | `OK` |
| `t2_gap_45` | 45 | 13 | **18.1x** [4.3x-68.6x] | 0.93 | 15.18 | 0.90 | 14.98 | 0.026 | 0.214 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **11.5x** [4.9x-30.4x] | 3.65 | 71.21 | 3.62 | 70.95 | 0.031 | 0.251 | `OK` |
| `t2_binpack_44` | 44 | 14 | **12.9x** [5.2x-23.0x] | 1.05 | 13.86 | 1.03 | 13.65 | 0.023 | 0.205 | `OK` |
| `t2_setcover_50` | 50 | 20 | **47.4x** [6.5x-128.9x] | 0.19 | 10.91 | 0.16 | 10.61 | 0.037 | 0.306 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **52.9x** [34.0x-73.8x] | 0.17 | 9.91 | 0.14 | 9.59 | 0.033 | 0.312 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **53.8x** [42.4x-63.7x] | 0.19 | 10.76 | 0.13 | 10.32 | 0.053 | 0.442 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **28.0x** [21.5x-34.8x] | 0.42 | 11.68 | 0.33 | 11.05 | 0.088 | 0.628 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.4x** [3.5x-25.1x] | 3.25 | 47.12 | 3.19 | 46.82 | 0.042 | 0.316 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **6.8x** [1.3x-28.6x] | 17.92 | 172.55 | 17.86 | 172.09 | 0.064 | 0.461 | `OK` |
| `t3_mkp_200` | 200 | 8 | **4.8x** [3.6x-6.2x] | 24.92 | 136.59 | 24.83 | 135.50 | 0.096 | 0.857 | `OK` |
| `t3_cflp_300` | 300 | 34 | **32.6x** [7.4x-67.7x] | 6.56 | 259.17 | 6.46 | 258.19 | 0.107 | 0.988 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **2.4x** [1.4x-4.3x] | 86.47 | 161.80 | 86.22 | 159.87 | 0.245 | 1.949 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **8.6x** [3.8x-12.6x] | 8.42 | 83.88 | 8.09 | 80.79 | 0.349 | 2.900 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **23.5x** [19.1x-31.3x] | 0.77 | 18.39 | 0.52 | 17.28 | 0.284 | 1.073 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **15.9x** [8.2x-23.3x] | 1.11 | 19.20 | 0.89 | 17.34 | 0.292 | 1.850 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.8x** [1.1x-15.0x] | 9.61 | 63.04 | 8.38 | 54.50 | 1.056 | 8.571 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **10.0x** [4.4x-23.7x] | 36.68 | 381.17 | 31.79 | 346.44 | 4.619 | 36.323 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## 5. Default Build vs. `GOEXPERIMENT=simd`

In [`internal/simd/kernels_amd64.go`](../internal/simd/kernels_amd64.go) and [`internal/simd/kernels_avx512_amd64.go`](../internal/simd/kernels_avx512_amd64.go), runtime CPUID dispatch selects AVX-512, AVX2, or 4-way scalar loops from a single portable `CGO_ENABLED=0` binary (`GOAMD64=v3` FMA-safe). Each vector kernel spills accumulators via `StoreArray` and executes `archsimd.ClearAVXUpperBits()` (`VZEROUPPER`) before scalar tail or caller operations, eliminating AVX-to-SSE transition stalls while preserving IEEE-754 `NaN` propagation parity. AVX2 activates at `minAVX2Len = 32` (`Dot64`, `Axpy64`, `Scale64`, `AbsMax64`, `RowActivityBoundsFinite`, `SelectDualLeaveRow64With`, `MinReducedSkip64With`, `InitReducedRow64`, `AllAbsLE64With`, `ValidateAndNegate64With`), `rows >= 8 && cols >= 8` for `Transpose64With`, `minDualRatioSIMDChunks = 4` (256 cols for `ScanDualRatioCand64With`), and `minAVX2Min2Len = 256` (`Min2ReducedRow64`); on AVX-512 hosts, dense LSAP loops dispatch AVX-512 at `minAVX512Len = 64` and `minAVX512Min2Len = 128` while sparse LP kernels gate AVX-512 at `minAVX512LPLen = 2048` / `minLeaveRowAVX512 = 2048` / `minDualRatioAVX512Chunks = 128` (8,192 cols) to avoid Intel Emerald Rapids License-2 clock down-binning across surrounding scalar LU code.

At the full-solver level, each ratio below is the geometric mean of per-trial `scalar / SIMD` solve times over the same instances on the local AVX2 host (`i9-13900K`), with a 95% $t$-interval on the log ratios; "no detectable effect" means the interval contains 1. "Reachable" is a necessary condition: the AVX2 LP kernels require `n + m >= 32`. Below that size no vector kernel can run, so those small models serve as negative controls where intervals excluding 1 reflect binary code-layout or run-to-run variance rather than SIMD execution. Per-family paired speedups and 95% intervals for larger models appear in the generated table below.

### 5.1 Scope and Pending Target-Hardware Verification

All telemetry in this document is measured locally on the AVX2-only `13th Gen Intel(R) Core(TM) i9-13900K` host (`x86_64`, Linux). Portable `GOOS=linux GOARCH=arm64 CGO_ENABLED=0` and `GOAMD64=v3`/`v4` builds compile and pass local build checks, whereas fresh physical AVX-512 and `linux/arm64` runtime test and benchmark re-verification on the current tree remain pending target hardware access.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 3 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 3 faster, 4 slower, 16 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 1.00x [0.98x-1.02x] | 0.004 | 0.004 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 0.97x [0.93x-1.01x] | 0.003 | 0.003 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 0.93x [0.82x-1.05x] | 0.005 | 0.005 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source | 1.07x [0.97x-1.19x] | 0.005 | 0.005 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 1.03x [0.90x-1.19x] | 0.006 | 0.006 | 11 |
| `t1_furniture` | 5 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.04x [1.01x-1.08x] | 0.018 | 0.017 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.80x [0.76x-0.84x] | 0.109 | 0.151 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.73x [0.67x-0.80x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 0.86x [0.65x-1.13x] | 0.050 | 0.061 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 0.99x [0.97x-1.02x] | 0.471 | 0.498 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 0.98x [0.92x-1.05x] | 2.408 | 2.478 | 11 |
| `t2_mkp_50` | 58 | possible | SIMD slower | 0.92x [0.84x-1.00x] | 5.207 | 5.388 | 11 |
| `t2_cflp_36` | 47 | possible | no detectable effect | 1.02x [0.96x-1.08x] | 0.161 | 0.165 | 11 |
| `t2_cflp_49` | 62 | possible | SIMD slower | 0.90x [0.85x-0.96x] | 0.333 | 0.382 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 1.01x [0.99x-1.03x] | 0.903 | 0.926 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 0.99x [0.93x-1.06x] | 3.623 | 3.930 | 11 |
| `t2_binpack_44` | 58 | possible | SIMD slower | 0.98x [0.97x-1.00x] | 1.030 | 1.036 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 0.98x [0.93x-1.03x] | 0.160 | 0.157 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 1.01x [0.91x-1.12x] | 0.135 | 0.141 | 11 |
| `t2_mad_g3_i9` | 108 | possible | SIMD slower | 0.77x [0.72x-0.83x] | 0.129 | 0.170 | 11 |
| `t2_mad_g4_i12` | 156 | possible | SIMD faster | 1.07x [1.01x-1.15x] | 0.330 | 0.298 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.00x [0.98x-1.03x] | 3.194 | 3.295 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 1.01x [0.98x-1.03x] | 17.858 | 17.992 | 11 |
| `t3_mkp_200` | 208 | possible | no detectable effect | 1.03x [0.98x-1.08x] | 24.831 | 23.071 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 0.97x [0.87x-1.09x] | 6.455 | 6.343 | 5 |
| `t3_lotsizing_504` | 840 | possible | no detectable effect | 1.02x [0.98x-1.07x] | 86.218 | 80.058 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 0.98x [0.83x-1.16x] | 8.090 | 7.908 | 5 |
| `t3_mad_g5_i20` | 240 | possible | SIMD faster | 1.21x [1.07x-1.36x] | 0.515 | 0.438 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 1.03x [0.96x-1.11x] | 0.888 | 0.855 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 1.05x [0.97x-1.14x] | 8.378 | 7.516 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | SIMD faster | 1.10x [1.05x-1.15x] | 31.787 | 28.790 | 5 |
<!-- END GENERATED:SIMD_EFFECT_TABLE -->

---

## 6. Full Corpus Solver-Core Checkpoint Progression

<!-- BEGIN GENERATED:FULL_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_std_lp` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 54.00 | 54.00 |
| `t1_std_lp_max` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 80.00 | 80.00 |
| `t1_std_mip` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.01 (3) | 0.002 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.002 | 64.95 | 58.90 |
| `t1_whiskas` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.004 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.02 (1) | 0.02 (1) | 0.003 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.11** (1 / 8) | 0.12 (2 / 8) | 0.19 (18) | 0.30 (35) | 0.008 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.05** (1 / 0) | 0.06 (1 / 0) | 0.07 (1) | 0.64 (4) | 0.091 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.47** (2 / 0) | 0.60 (12 / 0) | 0.58 (12) | 0.82 (109) | 0.109 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.41** (5 / 9) | 3.34 (40 / 9) | 1.23 (51) | 1.16 (61) | 0.030 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **5.21** (1 / 4) | 15.51 (2 / 4) | 5.93 (118) | 5.44 (111) | 0.047 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.16** (1 / 2) | 0.25 (3 / 2) | 0.19 (7) | 0.19 (7) | 0.028 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.33** (1 / 10) | 0.77 (4 / 10) | 0.39 (9) | 0.32 (9) | 0.037 | 909.02 | 827.30 |
| `t2_gap_45` | **0.90** (1 / 9) | 0.55 (1 / 9) | 1.89 (41) | 2.23 (60) | 0.030 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **3.62** (2 / 37) | 7.58 (25 / 37) | 3.47 (162) | 3.91 (199) | 0.065 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **1.03** (2 / 6) | 2.39 (5 / 6) | 9.58 (772) | 30.95 (1941) | 0.029 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.16** (1 / 6) | 0.37 (2 / 6) | 0.69 (6) | 0.67 (6) | 0.031 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.14** (1 / 0) | 0.87 (8 / 0) | 0.77 (8) | 0.77 (9) | 0.109 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.13** (1 / 0) | 0.23 (2 / 0) | 0.13 (2) | 2.89 (13) | 0.261 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.33** (1 / 0) | 3.60 (22 / 0) | 3.26 (22) | 5.56 (23) | 0.366 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.19** (43 / 0) | 3.30 (112 / 0) | 3.20 (112) | 3.99 (45) | 0.169 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **17.86** (196 / 0) | 34.21 (188 / 0) | 33.40 (188) | 98.83 (539) | 0.307 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **24.83** (250 / 14) | 115.55 (1135 / 16) | 237.43 (5000, `NodeLimit`) | 216.64 (5000, `NodeLimit`) | 0.141 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **6.46** (1 / 28) | 19.83 (54 / 34) | 9.45 (116) | 9.75 (118) | 0.184 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **86.22** (2 / 206) | 124.00 (376 / 206) | 250.07 (2733, `TimeLimit`) | 250.10 (2797, `TimeLimit`) | 0.798 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **8.09** (31 / 55) | 28.67 (87 / 53) | 250.09 (1656, `TimeLimit`) | 250.02 (1694, `TimeLimit`) | 0.237 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.52** (1 / 0) | 3.34 (17 / 0) | 2.99 (17) | 0.54 (1) | 0.724 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.89** (1 / 0) | 9.57 (58 / 0) | 8.89 (58) | 14.90 (74) | 1.740 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **8.38** (2 / 0) | 22.66 (25 / 0) | 20.63 (25) | 250.26 (145, `TimeLimit`) | 3.022 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **31.79** (1 / 0) | 34.99 (2 / 0) | 33.67 (2) | 251.05 (6, `TimeLimit`) | 60.313 | 208000.00 | 208000.00 |
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
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.72x** [1.36x-2.18x] | 1.72x | 6.09x | 0.262 (0.0232) | 0.430 (0.0392) | 1.70x [1.00x-4.10x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **4.12x** [2.24x-7.57x] | 4.12x | 4.23x | 10.995 (0.982) | 86.235 (4.261) | 3.11x [0.92x-127.48x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **3.03x** [1.82x-5.04x] | 3.03x | 3.98x | 272.003 (51.521) | 1544.486 (301.623) | 2.63x [1.45x-9.44x] |
| Square families ($m = n$) | 16 (146) | **3.47x** [2.36x-5.10x] | 3.49x | 5.19x | 276.379 (51.326) | 1615.434 (303.104) | 2.64x [1.00x-127.48x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.91x** [1.44x-2.52x] | 1.82x | 3.29x | 6.881 (1.200) | 15.718 (2.819) | 1.99x [0.92x-2.85x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.96x** [1.18x-7.44x] | 3.27x | 4.04x | 46.958 (8.398) | 116.861 (14.855) | 2.01x [1.00x-127.48x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.94x** [2.17x-3.99x] | 2.93x | 4.58x | 283.261 (52.527) | 1631.152 (305.923) | 2.34x [0.92x-127.48x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 7.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **2.48x** | 0.0006 | 0.0016 | 0.0008 | 0.0230 | $0.0007 \pm 0.0001$ | $0.0016 \pm 0.0003$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.70x** | 0.0021 | 0.0037 | 0.0024 | 0.0189 | $0.0021 \pm 0.0005$ | $0.0035 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.81x** | 0.0043 | 0.0080 | 0.0061 | 0.0277 | $0.0045 \pm 0.0010$ | $0.0082 \pm 0.0018$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.34x** | 0.0043 | 0.0055 | 0.0068 | 0.0266 | $0.0041 \pm 0.0007$ | $0.0055 \pm 0.0010$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.38x** | 0.0044 | 0.0062 | 0.0069 | 0.0275 | $0.0046 \pm 0.0012$ | $0.0063 \pm 0.0009$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.85x** | 0.0075 | 0.0142 | 0.0136 | 0.0533 | $0.0078 \pm 0.0025$ | $0.0140 \pm 0.0020$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.94x** | 0.0343 | 0.0966 | 0.0514 | 0.140 | $0.0324 \pm 0.0078$ | $0.0936 \pm 0.0151$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.97x** | 0.0625 | 0.245 | 0.100 | 0.338 | $0.0612 \pm 0.0055$ | $0.245 \pm 0.0349$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.39x** | 0.0573 | 0.133 | 0.100 | 0.211 | $0.0558 \pm 0.0078$ | $0.134 \pm 0.0255$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **3.14x** | 0.163 | 0.324 | 0.199 | 0.416 | $0.175 \pm 0.0621$ | $1.326 \pm 2.500$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **26.71x** | 0.0712 | 1.018 | 0.0975 | 1.254 | $0.0675 \pm 0.0102$ | $2.547 \pm 2.547$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **2.09x** | 0.0446 | 0.0972 | 0.105 | 0.214 | $0.0487 \pm 0.0142$ | $0.0992 \pm 0.0094$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.21x** | 0.0475 | 0.112 | 0.0953 | 0.218 | $0.0522 \pm 0.0140$ | $0.113 \pm 0.0110$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **5.93x** | 0.0988 | 0.394 | 0.139 | 0.556 | $0.101 \pm 0.0207$ | $0.945 \pm 1.213$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **5.30x** | 0.404 | 1.840 | 0.591 | 5.827 | $0.406 \pm 0.0772$ | $2.337 \pm 1.264$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.89x** | 1.633 | 7.137 | 2.377 | 11.752 | $1.971 \pm 0.592$ | $7.439 \pm 0.508$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.80x** | 1.156 | 2.022 | 1.835 | 6.299 | $1.141 \pm 0.0692$ | $2.051 \pm 0.0944$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.67x** | 6.940 | 11.335 | 7.371 | 12.821 | $7.567 \pm 2.548$ | $12.455 \pm 3.651$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.52x** | 0.490 | 1.206 | 1.108 | 5.837 | $0.489 \pm 0.0492$ | $1.230 \pm 0.0566$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.21x** | 0.610 | 1.393 | 1.302 | 5.940 | $0.646 \pm 0.0883$ | $1.420 \pm 0.0565$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **5.08x** | 8.256 | 40.424 | 11.019 | 56.279 | $8.134 \pm 0.478$ | $41.380 \pm 3.934$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **7.09x** | 32.437 | 238.107 | 46.091 | 301.374 | $34.453 \pm 6.390$ | $242.922 \pm 34.311$ | 16450.52 | `OK` |
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
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source | 0.84x [0.54x-1.31x] | 0.0006 | 0.0006 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source | 1.07x [0.83x-1.39x] | 0.0021 | 0.0017 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.16x [1.04x-1.29x] | 0.0043 | 0.0038 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | SIMD faster | 1.19x [1.02x-1.40x] | 0.0043 | 0.0034 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | SIMD faster | 1.26x [1.08x-1.48x] | 0.0044 | 0.0038 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | SIMD faster | 1.16x [1.09x-1.24x] | 0.0075 | 0.0065 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | no detectable effect | 1.11x [0.62x-1.99x] | 0.0343 | 0.0233 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.45x [1.37x-1.52x] | 0.0625 | 0.0424 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.75x [1.48x-2.08x] | 0.0573 | 0.0314 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.95x [1.83x-2.07x] | 0.163 | 0.0833 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.40x [1.35x-1.45x] | 0.0712 | 0.0503 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.60x [1.18x-2.16x] | 0.0446 | 0.0280 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.57x [1.40x-1.76x] | 0.0475 | 0.0327 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.58x [1.41x-1.77x] | 0.0988 | 0.0584 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.81x [1.68x-1.96x] | 0.404 | 0.211 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.21x [1.35x-3.61x] | 1.633 | 0.844 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 3.08x [2.71x-3.49x] | 1.156 | 0.386 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 3.32x [3.02x-3.65x] | 6.940 | 2.002 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 2.49x [2.02x-3.08x] | 0.490 | 0.193 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 2.16x [1.67x-2.78x] | 0.610 | 0.309 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 2.10x [1.93x-2.28x] | 8.256 | 3.873 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.44x [1.25x-1.66x] | 32.437 | 24.185 | 5 |
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
| Tier 1: Micro-models (`N <= 26`) | 10 (110) | **19.67x** [8.83x-43.83x] | **41.09x** [13.12x-128.67x] | 9.2 (0.8) | 150.9 (14.1) | 15.40x [1.69x-137.96x] |
| Tier 2: Medium MILPs (`30 <= N <= 84`) | 13 (143) | **5.54x** [4.01x-7.66x] | **6.13x** [4.22x-8.90x] | 559.2 (36.0) | 1712.9 (152.1) | 5.84x [0.53x-41.35x] |
| Tier 3: Large MILPs (`145 <= N <= 4036`) | 8 (40) | **3.73x** [2.12x-6.57x] | **4.15x** [2.24x-7.72x] | 921.1 (174.5) | 3047.1 (608.1) | 4.15x [0.41x-15.21x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **4.55x** [3.07x-6.76x] | **4.77x** [3.11x-7.29x] | 1145.9 (161.6) | 2903.0 (379.1) | 5.08x [0.53x-41.35x] |
| All 31 synthetic families | 31 (293) | **7.53x** [5.14x-11.02x] | **10.24x** [6.08x-17.25x] | 1489.6 (211.3) | 4910.8 (774.3) | 7.38x [0.41x-137.96x] |
<!-- END GENERATED:CBC_SUMMARY_TABLE -->

### 9.1 MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families ($N = G(I + T + 3)$ variables, $M = G(2T + 3) + I$ constraints) separate solver behavior into three scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Paired `Go / CBC` | Go Total Med (ms) | CBC Total Med (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.08x** | 0.08 | 1.14 | 0.65x | 0.05 (1 / 19) | 1.09 (0 / 0) | 0.091 (28) | 0.139 (19) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.07x** | 0.50 | 7.22 | 0.74x | 0.47 (2 / 72) | 7.17 (14 / 44) | 0.109 (30) | 0.147 (18) |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.15x** | 0.17 | 1.25 | 0.78x | 0.14 (1 / 26) | 1.21 (0 / 0) | 0.109 (33) | 0.140 (21) |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.12x** | 0.19 | 1.67 | 0.97x | 0.13 (1 / 40) | 1.60 (0 / 0) | 0.261 (58) | 0.269 (40) |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.18x** | 0.42 | 2.17 | 0.88x | 0.33 (1 / 51) | 2.08 (0 / 0) | 0.366 (80) | 0.414 (63) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.32x** | 3.25 | 11.07 | 1.08x | 3.19 (43 / 483) | 11.01 (28 / 146) | 0.169 (38) | 0.157 (24) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.33x** | 17.92 | 78.77 | 1.05x | 17.86 (196 / 2289) | 78.68 (216 / 1352) | 0.307 (65) | 0.293 (54) |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.14x** | 0.77 | 5.97 | 1.13x | 0.52 (1 / 75) | 5.83 (0 / 0) | 0.724 (91) | 0.642 (94) |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.30x** | 1.11 | 4.00 | 1.61x | 0.89 (1 / 98) | 3.77 (0 / 0) | 1.740 (244) | 1.077 (149) |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **0.42x** | 9.61 | 27.68 | 0.82x | 8.38 (2 / 377) | 26.15 (0 / 0) | 3.022 (224) | 3.703 (319) |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **0.17x** | 36.68 | 338.34 | 1.01x | 31.79 (1 / 2439) | 328.97 (0 / 0) | 60.313 (1900) | 59.511 (3128) |
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
| `t3_mkp_200` | $200 \times 8$ | **0.58x** | $25.00 \pm 13.70$ | $39.53 \pm 18.66$ | 24.83 (250 / 2860) | 47.50 (154 / 1145) | 115.55 (1135 / 16) | 237.43 (5000, `NodeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **0.44x** | $7.40 \pm 7.91$ | $12.41 \pm 9.11$ | 5.21 (1 / 20) | 8.36 (0 / 8) | 15.51 (2 / 4) | 5.93 (118) |
| `t2_binpack_44` | $44 \times 14$ | **0.18x** | $1.33 \pm 0.88$ | $6.56 \pm 2.13$ | 1.03 (2 / 43) | 6.01 (0 / 8) | 2.39 (5 / 6) | 9.58 (772) |
| `t3_cflp_300` | $300 \times 34$ | **0.13x** | $8.76 \pm 6.39$ | $55.64 \pm 11.56$ | 6.46 (1 / 119) | 52.03 (18 / 483) | 19.83 (54 / 34) | 9.45 (116) |
| `t3_lotsizing_504` | $504 \times 336$ | **0.72x** | $74.44 \pm 47.60$ | $93.20 \pm 58.53$ | 86.22 (2 / 1351) | 70.75 (10 / 377) | 124.00 (376 / 206) | 250.07 (2733, `TimeLimit`) |
| `t3_sched_1000` | $1000 \times 100$ | **0.16x** | $10.95 \pm 6.46$ | $61.49 \pm 5.19$ | 8.09 (31 / 917) | 61.07 (0 / 76) | 28.67 (87 / 53) | 250.09 (1656, `TimeLimit`) |
<!-- END GENERATED:NON_MAD_CBC_GAP_TABLE -->

<!-- BEGIN GENERATED:NON_MAD_CBC_NOTES -->
- **Bin-packing tail behavior on `t2_binpack_44`**: pure-Go `BackendGo` ($1.33 \pm 0.88$ ms mean, 1.05 ms median) leads in-process `BackendCBC` ($6.56 \pm 2.13$ ms mean, 6.03 ms median) with a **5.68x** `BackendCBC / BackendGo` geometric-mean speedup (`Go / CBC = 0.18x`), running slower on 0 of 11 trials (peaking at 3.13 ms vs. 11.78 ms), with `CP5 +Presolve` at 9.58 ms vs. `CP6 +Cuts` at 2.39 ms.
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
