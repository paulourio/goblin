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
- **Unified telemetry source**: fingerprint `6a8a322d4f9ce0be` (working tree on top of commit `98ebb1c888513674ef7d5c1a4dc61cbd479baa75`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.57 ms median on a 1-variable MILP and 6.23 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
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
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **329.11x** [162.95x-664.72x] | 329.11x | 0.05x | 9.0 (0.8) | 197.7 (19.7) | 1089.1 (102.3) | 536.17x [29.94x-1235.03x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **20.66x** [12.62x-33.80x] | 20.66x | 0.15x | 650.8 (32.2) | 1954.4 (170.5) | 4355.1 (421.5) | 25.82x [0.13x-174.46x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **10.33x** [5.03x-21.22x] | 10.33x | 0.20x | 895.2 (139.7) | 2915.1 (561.5) | 6252.4 (1081.3) | 10.14x [1.13x-75.96x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **13.58x** [8.08x-22.82x] | 14.95x | 0.19x | 1234.4 (127.3) | 3120.7 (395.6) | 6839.1 (998.1) | 15.38x [0.13x-174.46x] |
| Tier 3 non-MAD MILPs | 4 (20) | **8.07x** [1.58x-41.20x] | 8.07x | 0.27x | 592.5 (95.9) | 1253.8 (232.5) | 2832.8 (608.0) | 6.65x [1.13x-75.96x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **26.27x** [12.02x-57.44x] | 33.99x | 0.11x | 312.7 (44.8) | 1773.8 (339.0) | 3873.7 (514.3) | 38.72x [1.14x-124.90x] |
| Random-weight MAD (negative controls) | 3 (33) | **16.92x** [0.82x-350.77x] | 16.92x | 0.14x | 300.6 (20.0) | 1216.7 (116.4) | 2266.0 (250.4) | 16.28x [1.36x-119.47x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **49.75x** [22.04x-112.29x] | 60.20x | 0.11x | 1242.3 (128.0) | 3293.3 (412.8) | 7822.8 (1090.8) | 49.44x [0.13x-1235.03x] |
| All 31 synthetic corpus families | 31 (293) | **42.19x** [22.63x-78.66x] | 53.13x | 0.11x | 1555.0 (172.7) | 5067.2 (751.7) | 11696.5 (1605.1) | 43.33x [0.13x-1235.03x] |
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
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **102.3x** [86.1x-124.9x] | 0.09 | 9.62 | 0.06 | 9.39 | 0.035 | 0.224 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **68.3x** [29.9x-119.5x] | 0.48 | 34.12 | 0.45 | 33.86 | 0.025 | 0.247 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **44.4x** [33.6x-61.7x] | 0.22 | 9.66 | 0.15 | 9.39 | 0.061 | 0.276 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **55.3x** [42.3x-66.5x] | 0.18 | 10.24 | 0.12 | 9.82 | 0.046 | 0.411 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **29.6x** [23.8x-38.4x] | 0.42 | 11.46 | 0.32 | 10.85 | 0.080 | 0.612 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **10.0x** [3.5x-25.8x] | 2.98 | 47.85 | 2.94 | 47.56 | 0.039 | 0.300 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **7.1x** [1.4x-29.2x] | 16.56 | 168.44 | 16.49 | 168.01 | 0.064 | 0.435 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **31.3x** [25.0x-36.4x] | 0.56 | 18.26 | 0.45 | 17.04 | 0.112 | 1.014 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **16.6x** [10.2x-20.8x] | 1.01 | 18.82 | 0.82 | 16.99 | 0.261 | 1.778 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **5.8x** [1.1x-13.3x] | 8.44 | 61.14 | 7.52 | 53.28 | 1.075 | 8.316 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **10.3x** [4.5x-24.7x] | 33.84 | 375.12 | 30.22 | 340.11 | 3.936 | 35.304 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Cross-Instance Latency Dispersion Across Randomized Instances (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.09 \pm 0.01$ ($\pm 0.01$) | $9.58 \pm 0.43$ ($\pm 0.29$) | $0.09 \pm 0.01$ ($\pm 0.01$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.50 \pm 0.28$ ($\pm 0.19$) | $31.01 \pm 9.64$ ($\pm 6.47$) | $0.50 \pm 0.27$ ($\pm 0.18$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.22 \pm 0.03$ ($\pm 0.02$) | $9.73 \pm 0.63$ ($\pm 0.43$) | $0.16 \pm 0.01$ ($\pm 0.00$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.19 \pm 0.03$ ($\pm 0.02$) | $10.20 \pm 0.30$ ($\pm 0.20$) | $0.25 \pm 0.04$ ($\pm 0.03$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.40 \pm 0.04$ ($\pm 0.03$) | $11.77 \pm 0.88$ ($\pm 0.59$) | $0.36 \pm 0.02$ ($\pm 0.01$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $4.12 \pm 3.12$ ($\pm 2.10$) | $35.91 \pm 17.20$ ($\pm 11.56$) | $4.12 \pm 3.14$ ($\pm 2.11$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $22.71 \pm 22.51$ ($\pm 15.12$) | $139.08 \pm 63.73$ ($\pm 42.81$) | $22.36 \pm 21.97$ ($\pm 14.76$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.56 \pm 0.03$ ($\pm 0.03$) | $17.78 \pm 2.40$ ($\pm 2.98$) | $0.56 \pm 0.02$ ($\pm 0.02$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.18 \pm 0.37$ ($\pm 0.46$) | $18.84 \pm 0.40$ ($\pm 0.50$) | $1.16 \pm 0.38$ ($\pm 0.47$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $16.68 \pm 20.40$ ($\pm 25.32$) | $63.19 \pm 3.53$ ($\pm 4.38$) | $16.06 \pm 20.09$ ($\pm 24.95$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $42.14 \pm 14.72$ ($\pm 18.27$) | $584.11 \pm 614.36$ ($\pm 762.71$) | $39.55 \pm 15.72$ ($\pm 19.51$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | **0.06** (1 / 0) | 0.04 (1 / 0) | 0.05 (1) | 0.52 (4) | 0.090 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.45** (2 / 0) | 0.57 (12 / 0) | 0.55 (12) | 0.81 (109) | 0.105 | 4601.00 | 4257.00 |
| `t2_mad_g2_i8` | **0.15** (1 / 0) | 0.83 (8 / 0) | 0.73 (8) | 0.72 (9) | 0.103 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.12** (1 / 0) | 0.13 (2 / 0) | 0.14 (2) | 2.73 (13) | 0.244 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.32** (1 / 0) | 3.23 (22 / 0) | 3.07 (22) | 5.29 (23) | 0.356 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **2.94** (43 / 0) | 2.98 (112 / 0) | 2.88 (112) | 3.77 (45) | 0.153 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **16.49** (201 / 0) | 31.02 (188 / 0) | 30.19 (188) | 92.91 (539) | 0.275 | 7720.00 | 6060.00 |
| `t3_mad_g5_i20` | **0.45** (1 / 0) | 3.03 (17 / 0) | 2.66 (17) | 0.52 (1) | 0.653 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.82** (1 / 0) | 8.21 (58 / 0) | 7.58 (58) | 13.24 (74) | 1.607 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.52** (2 / 0) | 21.66 (25 / 0) | 19.38 (25) | 250.18 (145, `TimeLimit`) | 2.736 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **30.22** (1 / 0) | 33.54 (2 / 0) | 31.09 (2) | 250.58 (6, `TimeLimit`) | 58.441 | 208000.00 | 208000.00 |
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
| `t1_std_lp` | 3 | 3 | **602.9x** [311.8x-850.4x] | 0.01 | 6.36 | 0.00 | 6.30 | 0.005 | 0.055 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **631.9x** [321.8x-1129.7x] | 0.01 | 6.31 | 0.00 | 6.25 | 0.005 | 0.053 | `OK` |
| `t1_std_mip` | 3 | 3 | **700.3x** [587.3x-929.0x] | 0.01 | 7.71 | 0.00 | 7.64 | 0.005 | 0.064 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **747.3x** [554.4x-1093.3x] | 0.01 | 7.39 | 0.00 | 7.34 | 0.006 | 0.058 | `OK` |
| `t1_whiskas` | 6 | 5 | **559.6x** [383.6x-816.4x] | 0.01 | 7.24 | 0.01 | 7.15 | 0.007 | 0.099 | `OK` |
| `t1_furniture` | 2 | 3 | **252.0x** [85.2x-643.1x] | 0.03 | 7.90 | 0.02 | 7.85 | 0.006 | 0.055 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **92.1x** [40.5x-297.7x] | 0.12 | 9.41 | 0.11 | 9.31 | 0.013 | 0.100 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **824.5x** [300.0x-1235.0x] | 0.01 | 6.26 | 0.00 | 6.21 | 0.005 | 0.048 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **102.3x** [86.1x-124.9x] | 0.09 | 9.62 | 0.06 | 9.39 | 0.035 | 0.224 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **68.3x** [29.9x-119.5x] | 0.48 | 34.12 | 0.45 | 33.86 | 0.025 | 0.247 | `OK` |
| `t2_mkp_30` | 30 | 6 | **11.4x** [6.0x-16.3x] | 2.14 | 28.00 | 2.12 | 27.83 | 0.018 | 0.171 | `OK` |
| `t2_mkp_50` | 50 | 8 | **5.9x** [3.1x-35.4x] | 4.14 | 16.28 | 4.11 | 16.05 | 0.032 | 0.248 | `OK` |
| `t2_cflp_36` | 36 | 11 | **60.6x** [25.7x-174.5x] | 0.16 | 9.42 | 0.14 | 9.25 | 0.016 | 0.173 | `OK` |
| `t2_cflp_49` | 49 | 13 | **33.1x** [20.9x-92.7x] | 0.38 | 10.54 | 0.35 | 10.33 | 0.030 | 0.211 | `OK` |
| `t2_gap_45` | 45 | 13 | **21.5x** [4.8x-76.1x] | 0.98 | 14.74 | 0.96 | 14.53 | 0.020 | 0.209 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **13.2x** [5.7x-33.1x] | 3.30 | 69.49 | 3.28 | 69.25 | 0.029 | 0.247 | `OK` |
| `t2_binpack_44` | 44 | 14 | **12.3x** [0.1x-44.6x] | 0.58 | 14.45 | 0.56 | 14.23 | 0.022 | 0.211 | `OK` |
| `t2_setcover_50` | 50 | 20 | **51.6x** [6.9x-143.6x] | 0.19 | 10.92 | 0.15 | 10.63 | 0.032 | 0.304 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **44.4x** [33.6x-61.7x] | 0.22 | 9.66 | 0.15 | 9.39 | 0.061 | 0.276 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **55.3x** [42.3x-66.5x] | 0.18 | 10.24 | 0.12 | 9.82 | 0.046 | 0.411 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **29.6x** [23.8x-38.4x] | 0.42 | 11.46 | 0.32 | 10.85 | 0.080 | 0.612 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **10.0x** [3.5x-25.8x] | 2.98 | 47.85 | 2.94 | 47.56 | 0.039 | 0.300 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **7.1x** [1.4x-29.2x] | 16.56 | 168.44 | 16.49 | 168.01 | 0.064 | 0.435 | `OK` |
| `t3_mkp_200` | 200 | 8 | **5.4x** [4.1x-6.9x] | 19.33 | 121.10 | 19.24 | 120.48 | 0.082 | 0.625 | `OK` |
| `t3_cflp_300` | 300 | 34 | **28.3x** [8.2x-76.0x] | 6.16 | 248.24 | 6.07 | 247.29 | 0.103 | 0.929 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **2.6x** [1.1x-4.9x] | 63.06 | 157.79 | 62.87 | 155.86 | 0.199 | 1.928 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **10.9x** [5.4x-17.4x] | 7.35 | 80.86 | 6.92 | 77.94 | 0.302 | 2.820 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **31.3x** [25.0x-36.4x] | 0.56 | 18.26 | 0.45 | 17.04 | 0.112 | 1.014 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **16.6x** [10.2x-20.8x] | 1.01 | 18.82 | 0.82 | 16.99 | 0.261 | 1.778 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.8x** [1.1x-13.3x] | 8.44 | 61.14 | 7.52 | 53.28 | 1.075 | 8.316 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **10.3x** [4.5x-24.7x] | 33.84 | 375.12 | 30.22 | 340.11 | 3.936 | 35.304 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## 5. Default Build vs. `GOEXPERIMENT=simd`

In [`internal/simd/kernels_amd64.go`](../internal/simd/kernels_amd64.go) and [`internal/simd/kernels_avx512_amd64.go`](../internal/simd/kernels_avx512_amd64.go), runtime CPUID dispatch selects AVX-512, AVX2, or 4-way scalar loops from a single portable `CGO_ENABLED=0` binary (`GOAMD64=v3` FMA-safe). Each vector kernel spills accumulators via `StoreArray` and executes `archsimd.ClearAVXUpperBits()` (`VZEROUPPER`) before scalar tail or caller operations, eliminating AVX-to-SSE transition stalls while preserving IEEE-754 `NaN` propagation parity. AVX2 activates at `minAVX2Len = 32` (`Dot64`, `Axpy64`, `Scale64`, `AbsMax64`, `RowActivityBoundsFinite`, `SelectDualLeaveRow64With`, `MinReducedSkip64With`, `InitReducedRow64`, `AllAbsLE64With`, `ValidateAndNegate64With`), `rows >= 8 && cols >= 8` for `Transpose64With`, `minDualRatioSIMDChunks = 4` (256 cols for `ScanDualRatioCand64With`), and `minAVX2Min2Len = 256` (`Min2ReducedRow64`); on AVX-512 hosts, dense LSAP loops dispatch AVX-512 at `minAVX512Len = 64` and `minAVX512Min2Len = 128` while sparse LP kernels gate AVX-512 at `minAVX512LPLen = 2048` / `minLeaveRowAVX512 = 2048` / `minDualRatioAVX512Chunks = 128` (8,192 cols) to avoid Intel Emerald Rapids License-2 clock down-binning across surrounding scalar LU code.

At the full-solver level, each ratio below is the geometric mean of per-trial `scalar / SIMD` solve times over the same instances on the local AVX2 host (`i9-13900K`), with a 95% $t$-interval on the log ratios; "no detectable effect" means the interval contains 1. "Reachable" is a necessary condition: the AVX2 LP kernels require `n + m >= 32`. Below that size no vector kernel can run, so those small models serve as negative controls where intervals excluding 1 reflect binary code-layout or run-to-run variance rather than SIMD execution. Per-family paired speedups and 95% intervals for larger models appear in the generated table below.

### 5.1 Scope and Pending Target-Hardware Verification

All telemetry in this document is measured locally on the AVX2-only `13th Gen Intel(R) Core(TM) i9-13900K` host (`x86_64`, Linux). Portable `GOOS=linux GOARCH=arm64 CGO_ENABLED=0` and `GOAMD64=v3`/`v4` builds compile and pass local build checks, whereas fresh physical AVX-512 and `linux/arm64` runtime test and benchmark re-verification on the current tree remain pending target hardware access.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 0 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 6 faster, 2 slower, 15 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 1.15x [0.99x-1.33x] | 0.004 | 0.004 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 1.13x [0.93x-1.37x] | 0.003 | 0.003 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 1.00x [0.99x-1.01x] | 0.005 | 0.005 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source | 0.99x [0.94x-1.05x] | 0.004 | 0.004 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 0.99x [0.96x-1.02x] | 0.006 | 0.006 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 0.99x [0.95x-1.03x] | 0.016 | 0.017 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source | 0.98x [0.94x-1.02x] | 0.105 | 0.104 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 0.97x [0.92x-1.03x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 1.02x [0.98x-1.06x] | 0.055 | 0.054 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 1.00x [0.99x-1.01x] | 0.451 | 0.449 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 0.98x [0.95x-1.02x] | 2.116 | 2.140 | 11 |
| `t2_mkp_50` | 58 | possible | SIMD faster | 1.01x [1.00x-1.03x] | 4.111 | 4.084 | 11 |
| `t2_cflp_36` | 47 | possible | no detectable effect | 1.00x [0.99x-1.01x] | 0.139 | 0.134 | 11 |
| `t2_cflp_49` | 62 | possible | no detectable effect | 1.01x [0.98x-1.04x] | 0.346 | 0.335 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 0.99x [0.96x-1.01x] | 0.962 | 0.968 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 1.00x [0.99x-1.02x] | 3.275 | 3.295 | 11 |
| `t2_binpack_44` | 58 | possible | SIMD slower | 0.91x [0.87x-0.95x] | 0.556 | 0.607 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 1.02x [1.00x-1.04x] | 0.146 | 0.140 | 11 |
| `t2_mad_g2_i8` | 72 | possible | SIMD faster | 1.23x [1.11x-1.35x] | 0.152 | 0.127 | 11 |
| `t2_mad_g3_i9` | 108 | possible | SIMD slower | 0.84x [0.81x-0.88x] | 0.123 | 0.155 | 11 |
| `t2_mad_g4_i12` | 156 | possible | SIMD faster | 1.09x [1.02x-1.17x] | 0.319 | 0.285 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.01x [0.98x-1.04x] | 2.943 | 3.093 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 1.01x [0.99x-1.03x] | 16.493 | 16.384 | 11 |
| `t3_mkp_200` | 208 | possible | SIMD faster | 1.07x [1.05x-1.08x] | 19.244 | 17.790 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.02x [0.99x-1.05x] | 6.067 | 6.150 | 5 |
| `t3_lotsizing_504` | 840 | possible | no detectable effect | 1.05x [0.99x-1.11x] | 62.866 | 61.202 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 1.01x [0.88x-1.16x] | 6.920 | 6.069 | 5 |
| `t3_mad_g5_i20` | 240 | possible | SIMD faster | 1.05x [1.01x-1.09x] | 0.451 | 0.425 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 1.03x [0.95x-1.12x] | 0.817 | 0.851 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 1.06x [0.98x-1.16x] | 7.520 | 7.319 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | SIMD faster | 1.09x [1.03x-1.17x] | 30.220 | 27.634 | 5 |
<!-- END GENERATED:SIMD_EFFECT_TABLE -->

---

## 6. Full Corpus Solver-Core Checkpoint Progression

<!-- BEGIN GENERATED:FULL_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_std_lp` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 54.00 | 54.00 |
| `t1_std_lp_max` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 80.00 | 80.00 |
| `t1_std_mip` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.01 (3) | 0.002 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.00** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.002 | 64.95 | 58.90 |
| `t1_whiskas` | **0.01** (1 / 0) | 0.00 (1 / 0) | 0.01 (1) | 0.00 (1) | 0.003 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.02 (1) | 0.002 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.11** (1 / 8) | 0.12 (2 / 8) | 0.16 (18) | 0.24 (35) | 0.009 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.002 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.06** (1 / 0) | 0.04 (1 / 0) | 0.05 (1) | 0.52 (4) | 0.090 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.45** (2 / 0) | 0.57 (12 / 0) | 0.55 (12) | 0.81 (109) | 0.105 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.12** (5 / 9) | 2.64 (40 / 9) | 1.03 (51) | 0.99 (61) | 0.026 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **4.11** (1 / 4) | 11.61 (2 / 4) | 4.76 (118) | 4.19 (111) | 0.038 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.14** (1 / 2) | 0.23 (3 / 2) | 0.20 (7) | 0.17 (7) | 0.026 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.35** (1 / 10) | 0.59 (4 / 10) | 0.34 (9) | 0.30 (9) | 0.036 | 909.02 | 827.30 |
| `t2_gap_45` | **0.96** (1 / 9) | 0.38 (1 / 9) | 1.85 (41) | 2.22 (60) | 0.026 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **3.28** (2 / 37) | 6.44 (25 / 37) | 2.99 (162) | 3.26 (199) | 0.067 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **0.56** (2 / 8) | 1.02 (5 / 8) | 8.79 (772) | 26.78 (1953) | 0.027 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.15** (1 / 6) | 0.32 (2 / 6) | 0.65 (6) | 0.64 (6) | 0.031 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.15** (1 / 0) | 0.83 (8 / 0) | 0.73 (8) | 0.72 (9) | 0.103 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.12** (1 / 0) | 0.13 (2 / 0) | 0.14 (2) | 2.73 (13) | 0.244 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.32** (1 / 0) | 3.23 (22 / 0) | 3.07 (22) | 5.29 (23) | 0.356 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **2.94** (43 / 0) | 2.98 (112 / 0) | 2.88 (112) | 3.77 (45) | 0.153 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **16.49** (201 / 0) | 31.02 (188 / 0) | 30.19 (188) | 92.91 (539) | 0.275 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **19.24** (250 / 14) | 82.79 (1135 / 16) | 165.09 (5000, `NodeLimit`) | 149.73 (5000, `NodeLimit`) | 0.118 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **6.07** (1 / 28) | 17.96 (54 / 34) | 8.68 (116) | 8.71 (118) | 0.170 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **62.87** (2 / 206) | 79.69 (376 / 206) | 250.08 (3697, `TimeLimit`) | 250.04 (3471, `TimeLimit`) | 1.178 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **6.92** (31 / 55) | 19.28 (87 / 53) | 250.07 (2550, `TimeLimit`) | 250.02 (2604, `TimeLimit`) | 0.180 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.45** (1 / 0) | 3.03 (17 / 0) | 2.66 (17) | 0.52 (1) | 0.653 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.82** (1 / 0) | 8.21 (58 / 0) | 7.58 (58) | 13.24 (74) | 1.607 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.52** (2 / 0) | 21.66 (25 / 0) | 19.38 (25) | 250.18 (145, `TimeLimit`) | 2.736 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **30.22** (1 / 0) | 33.54 (2 / 0) | 31.09 (2) | 250.58 (6, `TimeLimit`) | 58.441 | 208000.00 | 208000.00 |
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
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.43x** [1.05x-1.96x] | 1.43x | 4.75x | 0.242 (0.0213) | 0.313 (0.0287) | 1.54x [0.46x-3.42x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **2.88x** [1.76x-4.73x] | 2.88x | 2.90x | 10.232 (0.801) | 32.691 (2.942) | 2.55x [0.17x-15.94x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.82x** [1.83x-4.32x] | 2.82x | 3.48x | 219.915 (42.870) | 933.611 (185.209) | 2.59x [1.46x-5.88x] |
| Square families ($m = n$) | 16 (146) | **2.69x** [1.96x-3.70x] | 2.63x | 3.79x | 225.544 (42.866) | 955.807 (186.248) | 2.28x [0.17x-15.94x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.68x** [1.11x-2.54x] | 1.55x | 2.87x | 4.844 (0.826) | 10.808 (1.931) | 1.90x [0.46x-2.65x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.16x** [1.05x-4.44x] | 2.27x | 2.74x | 36.951 (6.268) | 64.975 (11.013) | 1.77x [0.17x-15.94x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.37x** [1.83x-3.06x] | 2.28x | 3.51x | 230.388 (43.692) | 966.615 (188.179) | 2.02x [0.17x-15.94x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 7.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **2.23x** | 0.0004 | 0.0009 | 0.0005 | 0.0107 | $0.0004 \pm 0.0001$ | $0.0010 \pm 0.0003$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.51x** | 0.0016 | 0.0024 | 0.0019 | 0.0103 | $0.0016 \pm 0.0004$ | $0.0024 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.67x** | 0.0036 | 0.0062 | 0.0045 | 0.0153 | $0.0037 \pm 0.0010$ | $0.0061 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **0.97x** | 0.0039 | 0.0041 | 0.0050 | 0.0143 | $0.0043 \pm 0.0016$ | $0.0039 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.11x** | 0.0043 | 0.0043 | 0.0052 | 0.0149 | $0.0041 \pm 0.0012$ | $0.0044 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.44x** | 0.0075 | 0.0109 | 0.0120 | 0.0331 | $0.0079 \pm 0.0038$ | $0.0106 \pm 0.0018$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.71x** | 0.0279 | 0.0704 | 0.0345 | 0.0964 | $0.0263 \pm 0.0057$ | $0.0704 \pm 0.0123$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.22x** | 0.0536 | 0.179 | 0.0693 | 0.228 | $0.0546 \pm 0.0072$ | $0.177 \pm 0.0262$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.00x** | 0.0474 | 0.0935 | 0.0714 | 0.141 | $0.0488 \pm 0.0143$ | $0.0959 \pm 0.0139$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.32x** | 0.140 | 0.217 | 0.147 | 0.253 | $0.246 \pm 0.319$ | $0.235 \pm 0.0422$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **12.33x** | 0.0596 | 0.683 | 0.0759 | 0.765 | $0.0572 \pm 0.0106$ | $0.696 \pm 0.0252$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.89x** | 0.0346 | 0.0692 | 0.0626 | 0.143 | $0.0394 \pm 0.0139$ | $0.0716 \pm 0.0078$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **1.97x** | 0.0383 | 0.0788 | 0.0633 | 0.155 | $0.0419 \pm 0.0123$ | $0.0803 \pm 0.0069$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.42x** | 0.0796 | 0.285 | 0.105 | 0.363 | $0.0841 \pm 0.0161$ | $0.285 \pm 0.0314$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.84x** | 0.320 | 1.265 | 0.444 | 1.677 | $0.332 \pm 0.0551$ | $1.262 \pm 0.119$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.80x** | 1.262 | 5.189 | 1.860 | 7.724 | $1.409 \pm 0.279$ | $5.291 \pm 0.393$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.70x** | 0.872 | 1.490 | 1.484 | 4.088 | $0.874 \pm 0.0414$ | $1.483 \pm 0.0529$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.65x** | 5.137 | 8.512 | 5.350 | 10.263 | $5.716 \pm 2.150$ | $9.218 \pm 2.626$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.57x** | 0.310 | 0.817 | 0.727 | 3.578 | $0.326 \pm 0.0266$ | $0.837 \pm 0.0590$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.20x** | 0.435 | 0.957 | 0.838 | 3.672 | $0.445 \pm 0.0598$ | $0.972 \pm 0.0360$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.86x** | 5.969 | 27.954 | 8.856 | 36.545 | $5.848 \pm 0.253$ | $28.466 \pm 2.249$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.80x** | 28.884 | 140.290 | 41.337 | 188.961 | $29.365 \pm 3.423$ | $140.456 \pm 9.698$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 7.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSkip64`, `InitReducedRow64`, and `Min2ReducedRow64`)

In [`assign/solver.go`](../assign/solver.go), [`simd.AllAbsLE64With`](../internal/simd/kernels_amd64.go) and [`simd.ValidateAndNegate64With`](../internal/simd/assign_amd64.go) validate finite cost bounds via AVX2 checks whenever `len(cost) >= 32` (while the fused `colReduceValidateSquare` fast path skips `AllAbsLE64With` on finite square `Minimize` matrices and dispatches `ColReduceValidateRows64With` only for $n \ge 32$), [`simd.Transpose64With`](../internal/simd/assign_amd64.go) handles tall transposes with $\text{rows} \ge 8$ and $\text{cols} \ge 8$, [`simd.MinReducedSkip64With`](../internal/simd/assign_amd64.go) accelerates reduction-transfer slack scans in `preprocessSquareJV` ($n \ge 32$), [`simd.Min2ReducedRow64With`](../internal/simd/kernels_amd64.go) accelerates first/second reduced-minimum scans in `preprocessRectangularBV` and `augmentingRowReduction` at `minAVX2Min2Len = 256` (`minAVX512Min2Len = 128`), and [`simd.InitReducedRow64With`](../internal/simd/kernels_amd64.go) runs on a slice of length $\max(m, n) \ge 32$ at the start of each Dijkstra augmentation. All vector kernels clean upper `YMM` state via `StoreArray` + `ClearAVXUpperBits()` (`VZEROUPPER`) before scalar operations; exact per-family verdicts appear in the generated table below.

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` with no `>= 8x8` tall transpose and either fused square `Minimize` non-sparse path or `rows * cols < 32`, so no AVX2 kernel runs): 1 of 3 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32`, tall `rows > cols` with `>= 8x8`, or fallback `AllAbsLE64`/`ValidateAndNegate64` at `rows * cols >= 32`): 16 faster, 0 slower, 3 with no detectable effect, out of 19.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source | 1.00x [0.96x-1.05x] | 0.0004 | 0.0004 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source | 1.03x [0.99x-1.07x] | 0.0016 | 0.0015 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.98x [0.96x-0.99x] | 0.0036 | 0.0037 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | no detectable effect | 1.17x [0.98x-1.40x] | 0.0039 | 0.0034 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | SIMD faster | 1.10x [1.08x-1.12x] | 0.0043 | 0.0038 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | no detectable effect | 1.04x [0.95x-1.15x] | 0.0075 | 0.0066 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.22x [1.14x-1.32x] | 0.0279 | 0.0225 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.31x [1.25x-1.37x] | 0.0536 | 0.0413 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.66x [1.51x-1.82x] | 0.0474 | 0.0293 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 2.08x [1.39x-3.13x] | 0.140 | 0.0849 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.30x [1.24x-1.36x] | 0.0596 | 0.0456 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.37x [1.23x-1.52x] | 0.0346 | 0.0270 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.30x [1.26x-1.35x] | 0.0383 | 0.0293 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.41x [1.31x-1.51x] | 0.0796 | 0.0555 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.62x [1.56x-1.68x] | 0.320 | 0.199 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.86x [1.44x-2.41x] | 1.262 | 0.723 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | no detectable effect | 1.90x [0.86x-4.19x] | 0.872 | 0.353 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.56x [2.40x-2.73x] | 5.137 | 2.181 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.79x [1.44x-2.23x] | 0.310 | 0.184 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.66x [1.49x-1.85x] | 0.435 | 0.281 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.63x [1.45x-1.83x] | 5.969 | 3.607 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.48x [1.32x-1.66x] | 28.884 | 19.804 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

---

## 8. Keeping Documentation and Benchmarks Synchronized

1. **Regenerate telemetry** from a clean, committed tree in a single pinned session so that all seven telemetry files record identical `git_commit`, `source_fingerprint`, and `uname_release`: run the commands in the [README](../README.md#verification-benchmarking-and-documentation-sync-commands).
2. **Regenerate tables**: `go run ./cmd/goblin-bench -sync-docs`.
3. **Verify**: `go run ./cmd/goblin-bench -check-docs`, or `go test ./...`, which runs `TestDocumentationAndBenchmarkSync`. The gate validates corpus dimensions, parity, symbol identifiers, and provenance, and checks every generated block in `README.md`, `docs/benchmarks.md`, and `bench/results/final_report.md`.
