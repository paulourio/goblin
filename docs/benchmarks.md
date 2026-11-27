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
- **Unified telemetry source**: fingerprint `06fa79b9f6ebed61` (commit `56a3b68d01d84505bb6a45fe00028f2c7ffaccbb`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 8.27 ms median on a 1-variable MILP and 7.93 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
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
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **361.58x** [173.38x-754.06x] | 361.58x | 0.06x | 10.5 (0.9) | 155.9 (14.5) | 1338.9 (125.9) | 427.49x [32.02x-2302.22x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **19.89x** [11.73x-33.75x] | 19.89x | 0.17x | 609.9 (41.0) | 2034.1 (191.1) | 4931.1 (477.0) | 24.90x [1.36x-177.43x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **8.99x** [4.08x-19.78x] | 8.99x | 0.31x | 1051.6 (203.9) | 2649.0 (536.1) | 6964.5 (1201.2) | 9.39x [0.94x-74.85x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **12.44x** [6.95x-22.26x] | 13.91x | 0.22x | 1292.2 (190.8) | 3262.3 (426.0) | 7703.0 (1120.8) | 12.35x [1.36x-177.43x] |
| Tier 3 non-MAD MILPs | 4 (20) | **6.80x** [0.99x-46.87x] | 6.80x | 0.34x | 692.6 (150.7) | 1310.1 (241.9) | 3185.0 (680.9) | 4.29x [1.39x-74.85x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **26.38x** [10.96x-63.47x] | 35.58x | 0.18x | 370.1 (54.2) | 1433.4 (302.2) | 4331.9 (570.2) | 33.77x [0.94x-189.74x] |
| Random-weight MAD (negative controls) | 3 (33) | **15.91x** [0.79x-320.97x] | 15.91x | 0.18x | 357.7 (24.3) | 1190.1 (125.6) | 2515.9 (276.4) | 14.99x [1.36x-106.98x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **48.27x** [20.36x-114.42x] | 59.27x | 0.13x | 1301.8 (191.6) | 3405.6 (439.4) | 8902.6 (1233.9) | 52.46x [1.36x-2302.22x] |
| All 31 synthetic corpus families | 31 (293) | **41.30x** [21.32x-80.01x] | 53.02x | 0.14x | 1671.9 (245.8) | 4839.0 (741.6) | 13234.5 (1804.1) | 46.04x [0.94x-2302.22x] |
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
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **153.8x** [114.1x-189.7x] | 0.08 | 12.85 | 0.05 | 12.60 | 0.030 | 0.261 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **63.6x** [32.0x-107.0x] | 0.58 | 38.29 | 0.53 | 38.03 | 0.036 | 0.255 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **57.4x** [46.0x-67.0x] | 0.20 | 11.38 | 0.15 | 11.07 | 0.043 | 0.339 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **42.3x** [32.5x-51.9x] | 0.28 | 12.14 | 0.16 | 11.58 | 0.096 | 0.478 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **31.6x** [29.6x-33.4x] | 0.42 | 13.55 | 0.34 | 12.88 | 0.086 | 0.677 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **9.2x** [3.3x-24.6x] | 3.51 | 52.70 | 3.41 | 52.36 | 0.060 | 0.324 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **6.9x** [1.4x-29.0x] | 20.25 | 185.36 | 20.18 | 184.91 | 0.071 | 0.459 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **24.6x** [15.6x-32.8x] | 0.82 | 20.94 | 0.63 | 19.63 | 0.210 | 1.114 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **14.9x** [9.9x-19.8x] | 1.35 | 20.76 | 1.00 | 18.66 | 0.268 | 1.936 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **5.5x** [0.9x-13.7x] | 9.75 | 66.25 | 8.65 | 57.64 | 1.306 | 9.423 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **9.8x** [4.4x-23.4x] | 41.28 | 412.36 | 34.73 | 371.23 | 5.086 | 40.852 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Cross-Instance Latency Dispersion Across Randomized Instances (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.08 \pm 0.01$ ($\pm 0.01$) | $12.67 \pm 0.71$ ($\pm 0.48$) | $0.08 \pm 0.02$ ($\pm 0.02$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.60 \pm 0.30$ ($\pm 0.20$) | $35.00 \pm 10.75$ ($\pm 7.22$) | $0.62 \pm 0.28$ ($\pm 0.19$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.20 \pm 0.02$ ($\pm 0.02$) | $11.59 \pm 0.83$ ($\pm 0.56$) | $0.21 \pm 0.03$ ($\pm 0.02$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.29 \pm 0.05$ ($\pm 0.03$) | $12.14 \pm 0.46$ ($\pm 0.31$) | $0.29 \pm 0.10$ ($\pm 0.07$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.44 \pm 0.04$ ($\pm 0.03$) | $13.81 \pm 0.84$ ($\pm 0.57$) | $0.44 \pm 0.05$ ($\pm 0.03$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $4.91 \pm 3.81$ ($\pm 2.56$) | $39.77 \pm 18.95$ ($\pm 12.73$) | $4.87 \pm 3.81$ ($\pm 2.56$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $27.00 \pm 27.50$ ($\pm 18.47$) | $153.95 \pm 70.32$ ($\pm 47.24$) | $28.24 \pm 28.90$ ($\pm 19.41$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.84 \pm 0.14$ ($\pm 0.18$) | $20.64 \pm 2.81$ ($\pm 3.49$) | $0.61 \pm 0.03$ ($\pm 0.03$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.44 \pm 0.41$ ($\pm 0.51$) | $20.92 \pm 0.46$ ($\pm 0.58$) | $1.35 \pm 0.47$ ($\pm 0.59$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $20.48 \pm 27.14$ ($\pm 33.70$) | $68.99 \pm 4.67$ ($\pm 5.79$) | $19.74 \pm 25.60$ ($\pm 31.78$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $49.03 \pm 16.53$ ($\pm 20.52$) | $645.37 \pm 668.83$ ($\pm 830.33$) | $44.35 \pm 15.55$ ($\pm 19.30$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | **0.05** (1 / 0) | 0.05 (1 / 0) | 0.06 (1) | 0.66 (4) | 0.086 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.53** (2 / 0) | 0.81 (12 / 0) | 0.63 (12) | 0.85 (109) | 0.118 | 4601.00 | 4257.00 |
| `t2_mad_g2_i8` | **0.15** (1 / 0) | 0.93 (8 / 0) | 0.83 (8) | 0.81 (9) | 0.113 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.16** (1 / 0) | 0.14 (2 / 0) | 0.14 (2) | 3.15 (13) | 0.276 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.34** (1 / 0) | 3.77 (22 / 0) | 3.50 (22) | 6.08 (23) | 0.398 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.41** (43 / 0) | 5.07 (112 / 0) | 5.07 (112) | 6.18 (45) | 0.158 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **20.18** (196 / 0) | 40.50 (188 / 0) | 36.32 (188) | 93.97 (682) | 0.302 | 7720.00 | 6060.00 |
| `t3_mad_g5_i20` | **0.63** (1 / 0) | 3.88 (17 / 0) | 3.33 (17) | 0.58 (1) | 0.764 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **1.00** (1 / 0) | 10.67 (58 / 0) | 9.58 (58) | 15.79 (74) | 1.845 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **8.65** (2 / 0) | 24.17 (25 / 0) | 22.62 (25) | 250.42 (153, `TimeLimit`) | 3.168 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **34.73** (1 / 0) | 37.90 (2 / 0) | 35.04 (2) | 250.13 (6, `TimeLimit`) | 66.318 | 208000.00 | 208000.00 |
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
| `t1_std_lp` | 3 | 3 | **678.7x** [428.6x-842.9x] | 0.01 | 7.58 | 0.01 | 7.51 | 0.006 | 0.072 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **714.3x** [347.2x-1196.9x] | 0.01 | 7.77 | 0.00 | 7.68 | 0.005 | 0.077 | `OK` |
| `t1_std_mip` | 3 | 3 | **753.8x** [533.8x-978.3x] | 0.01 | 9.53 | 0.01 | 9.45 | 0.006 | 0.080 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **832.8x** [631.8x-1044.0x] | 0.01 | 9.69 | 0.00 | 9.61 | 0.006 | 0.079 | `OK` |
| `t1_whiskas` | 6 | 5 | **340.0x** [257.0x-426.4x] | 0.02 | 8.70 | 0.01 | 8.59 | 0.013 | 0.113 | `OK` |
| `t1_furniture` | 2 | 3 | **241.0x** [76.7x-731.8x] | 0.03 | 9.76 | 0.02 | 9.68 | 0.007 | 0.072 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **105.9x** [56.1x-275.3x] | 0.15 | 12.94 | 0.14 | 12.83 | 0.013 | 0.126 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **1477.5x** [666.5x-2302.2x] | 0.01 | 8.82 | 0.00 | 8.75 | 0.005 | 0.080 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **153.8x** [114.1x-189.7x] | 0.08 | 12.85 | 0.05 | 12.60 | 0.030 | 0.261 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **63.6x** [32.0x-107.0x] | 0.58 | 38.29 | 0.53 | 38.03 | 0.036 | 0.255 | `OK` |
| `t2_mkp_30` | 30 | 6 | **10.0x** [5.3x-14.6x] | 2.74 | 32.61 | 2.69 | 32.41 | 0.029 | 0.207 | `OK` |
| `t2_mkp_50` | 50 | 8 | **5.2x** [2.5x-31.3x] | 5.96 | 19.82 | 5.93 | 19.53 | 0.038 | 0.298 | `OK` |
| `t2_cflp_36` | 36 | 11 | **63.4x** [26.2x-177.4x] | 0.18 | 12.11 | 0.16 | 11.91 | 0.018 | 0.196 | `OK` |
| `t2_cflp_49` | 49 | 13 | **34.5x** [14.9x-102.8x] | 0.39 | 12.53 | 0.37 | 12.18 | 0.026 | 0.231 | `OK` |
| `t2_gap_45` | 45 | 13 | **20.2x** [4.5x-69.3x] | 0.96 | 17.22 | 0.92 | 16.99 | 0.030 | 0.240 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **12.4x** [5.3x-28.0x] | 4.22 | 77.92 | 4.19 | 77.63 | 0.034 | 0.260 | `OK` |
| `t2_binpack_44` | 44 | 14 | **10.4x** [3.6x-19.0x] | 1.62 | 16.38 | 1.59 | 16.12 | 0.039 | 0.237 | `OK` |
| `t2_setcover_50` | 50 | 20 | **53.2x** [6.8x-175.8x] | 0.23 | 13.27 | 0.18 | 12.91 | 0.038 | 0.356 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **57.4x** [46.0x-67.0x] | 0.20 | 11.38 | 0.15 | 11.07 | 0.043 | 0.339 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **42.3x** [32.5x-51.9x] | 0.28 | 12.14 | 0.16 | 11.58 | 0.096 | 0.478 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **31.6x** [29.6x-33.4x] | 0.42 | 13.55 | 0.34 | 12.88 | 0.086 | 0.677 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.2x** [3.3x-24.6x] | 3.51 | 52.70 | 3.41 | 52.36 | 0.060 | 0.324 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **6.9x** [1.4x-29.0x] | 20.25 | 185.36 | 20.18 | 184.91 | 0.071 | 0.459 | `OK` |
| `t3_mkp_200` | 200 | 8 | **3.2x** [2.3x-4.0x] | 38.94 | 128.83 | 38.80 | 128.18 | 0.147 | 0.675 | `OK` |
| `t3_cflp_300` | 300 | 34 | **35.0x** [9.1x-74.9x] | 7.24 | 290.71 | 7.13 | 289.67 | 0.118 | 1.046 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **2.4x** [1.4x-4.6x] | 94.65 | 172.18 | 94.35 | 170.14 | 0.253 | 2.055 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **8.2x** [3.9x-12.3x] | 9.88 | 89.15 | 9.52 | 86.07 | 0.506 | 3.087 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **24.6x** [15.6x-32.8x] | 0.82 | 20.94 | 0.63 | 19.63 | 0.210 | 1.114 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **14.9x** [9.9x-19.8x] | 1.35 | 20.76 | 1.00 | 18.66 | 0.268 | 1.936 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.5x** [0.9x-13.7x] | 9.75 | 66.25 | 8.65 | 57.64 | 1.306 | 9.423 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **9.8x** [4.4x-23.4x] | 41.28 | 412.36 | 34.73 | 371.23 | 5.086 | 40.852 | `OK` |
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
- **Instances where AVX2 can run**: 5 faster, 2 slower, 16 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.13x [1.01x-1.26x] | 0.005 | 0.004 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.87x [0.78x-0.97x] | 0.004 | 0.004 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 0.96x [0.85x-1.09x] | 0.006 | 0.006 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source | 0.81x [0.66x-1.00x] | 0.005 | 0.006 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.60x [1.50x-1.70x] | 0.010 | 0.006 | 11 |
| `t1_furniture` | 5 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.27x [1.06x-1.50x] | 0.024 | 0.017 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.15x [1.02x-1.31x] | 0.139 | 0.110 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 1.14x [0.99x-1.31x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 0.98x [0.75x-1.27x] | 0.051 | 0.047 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 0.96x [0.88x-1.04x] | 0.532 | 0.545 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 1.05x [0.93x-1.17x] | 2.695 | 2.705 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 0.95x [0.90x-1.00x] | 5.926 | 5.724 | 11 |
| `t2_cflp_36` | 47 | possible | SIMD slower | 0.91x [0.84x-0.99x] | 0.162 | 0.189 | 11 |
| `t2_cflp_49` | 62 | possible | no detectable effect | 1.11x [0.80x-1.53x] | 0.368 | 0.334 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 0.97x [0.91x-1.04x] | 0.922 | 1.049 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 0.97x [0.92x-1.04x] | 4.188 | 3.973 | 11 |
| `t2_binpack_44` | 58 | possible | SIMD faster | 1.19x [1.11x-1.28x] | 1.589 | 1.268 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 0.91x [0.78x-1.06x] | 0.180 | 0.186 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 0.99x [0.87x-1.12x] | 0.153 | 0.150 | 11 |
| `t2_mad_g3_i9` | 108 | possible | no detectable effect | 1.00x [0.87x-1.16x] | 0.164 | 0.195 | 11 |
| `t2_mad_g4_i12` | 156 | possible | no detectable effect | 1.01x [0.91x-1.12x] | 0.341 | 0.328 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.02x [0.98x-1.05x] | 3.410 | 3.618 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | SIMD slower | 0.93x [0.88x-0.98x] | 20.179 | 20.469 | 11 |
| `t3_mkp_200` | 208 | possible | SIMD faster | 1.36x [1.17x-1.60x] | 38.797 | 26.652 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 0.92x [0.79x-1.07x] | 7.129 | 7.176 | 5 |
| `t3_lotsizing_504` | 840 | possible | SIMD faster | 1.06x [1.01x-1.12x] | 94.350 | 87.420 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 0.97x [0.82x-1.15x] | 9.523 | 8.875 | 5 |
| `t3_mad_g5_i20` | 240 | possible | SIMD faster | 1.28x [1.05x-1.55x] | 0.628 | 0.466 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 1.11x [0.85x-1.46x] | 1.003 | 0.850 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 1.03x [0.94x-1.13x] | 8.647 | 8.807 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | SIMD faster | 1.11x [1.01x-1.21x] | 34.732 | 33.726 | 5 |
<!-- END GENERATED:SIMD_EFFECT_TABLE -->

---

## 6. Full Corpus Solver-Core Checkpoint Progression

<!-- BEGIN GENERATED:FULL_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_std_lp` | **0.01** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 54.00 | 54.00 |
| `t1_std_lp_max` | **0.00** (1 / 0) | 0.01 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 80.00 | 80.00 |
| `t1_std_mip` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.002 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.00** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.002 | 64.95 | 58.90 |
| `t1_whiskas` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.006 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.02 (1) | 0.02 (1) | 0.003 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.14** (1 / 8) | 0.13 (2 / 8) | 0.19 (18) | 0.28 (35) | 0.009 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.002 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.05** (1 / 0) | 0.05 (1 / 0) | 0.06 (1) | 0.66 (4) | 0.086 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.53** (2 / 0) | 0.81 (12 / 0) | 0.63 (12) | 0.85 (109) | 0.118 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.69** (5 / 9) | 3.66 (40 / 9) | 1.34 (51) | 1.17 (61) | 0.030 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **5.93** (1 / 4) | 16.25 (2 / 4) | 6.35 (118) | 5.73 (111) | 0.047 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.16** (1 / 2) | 0.31 (3 / 2) | 0.19 (7) | 0.20 (7) | 0.028 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.37** (1 / 10) | 0.65 (4 / 10) | 0.34 (9) | 0.34 (9) | 0.037 | 909.02 | 827.30 |
| `t2_gap_45` | **0.92** (1 / 9) | 0.45 (1 / 9) | 1.94 (41) | 2.13 (60) | 0.028 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **4.19** (2 / 37) | 11.80 (25 / 37) | 5.75 (162) | 4.24 (199) | 0.069 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **1.59** (2 / 6) | 1.87 (5 / 6) | 10.19 (772) | 32.95 (1941) | 0.035 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.18** (1 / 6) | 0.38 (2 / 6) | 0.73 (6) | 0.78 (6) | 0.032 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.15** (1 / 0) | 0.93 (8 / 0) | 0.83 (8) | 0.81 (9) | 0.113 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.16** (1 / 0) | 0.14 (2 / 0) | 0.14 (2) | 3.15 (13) | 0.276 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.34** (1 / 0) | 3.77 (22 / 0) | 3.50 (22) | 6.08 (23) | 0.398 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.41** (43 / 0) | 5.07 (112 / 0) | 5.07 (112) | 6.18 (45) | 0.158 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **20.18** (196 / 0) | 40.50 (188 / 0) | 36.32 (188) | 93.97 (682) | 0.302 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **38.80** (250 / 14) | 121.40 (1135 / 16) | 247.87 (5000, `NodeLimit`) | 248.43 (5000, `NodeLimit`) | 0.203 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **7.13** (1 / 28) | 20.86 (54 / 34) | 10.62 (116) | 10.54 (118) | 0.177 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **94.35** (2 / 206) | 140.89 (376 / 206) | 250.09 (2333, `TimeLimit`) | 250.08 (2387, `TimeLimit`) | 0.906 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **9.52** (31 / 55) | 33.10 (87 / 53) | 250.09 (1462, `TimeLimit`) | 250.13 (1551, `TimeLimit`) | 0.285 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.63** (1 / 0) | 3.88 (17 / 0) | 3.33 (17) | 0.58 (1) | 0.764 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **1.00** (1 / 0) | 10.67 (58 / 0) | 9.58 (58) | 15.79 (74) | 1.845 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **8.65** (2 / 0) | 24.17 (25 / 0) | 22.62 (25) | 250.42 (153, `TimeLimit`) | 3.168 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **34.73** (1 / 0) | 37.90 (2 / 0) | 35.04 (2) | 250.13 (6, `TimeLimit`) | 66.318 | 208000.00 | 208000.00 |
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
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.42x** [1.11x-1.82x] | 1.42x | 4.31x | 0.261 (0.0224) | 0.347 (0.0308) | 1.39x [0.75x-2.69x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **2.92x** [1.82x-4.69x] | 2.92x | 2.97x | 9.877 (0.843) | 34.860 (3.157) | 2.44x [0.50x-15.74x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.88x** [1.89x-4.39x] | 2.88x | 3.65x | 241.439 (46.387) | 1003.216 (197.847) | 2.58x [1.51x-6.16x] |
| Square families ($m = n$) | 16 (146) | **2.69x** [1.96x-3.69x] | 2.62x | 3.79x | 246.561 (46.376) | 1026.909 (198.966) | 2.24x [0.50x-15.74x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.74x** [1.18x-2.55x] | 1.62x | 2.87x | 5.016 (0.877) | 11.515 (2.069) | 1.94x [0.75x-2.62x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.25x** [1.10x-4.58x] | 2.36x | 2.84x | 37.294 (6.679) | 69.566 (11.661) | 1.78x [0.79x-15.74x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.39x** [1.86x-3.08x] | 2.30x | 3.51x | 251.577 (47.253) | 1038.424 (201.035) | 2.07x [0.50x-15.74x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 7.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **2.01x** | 0.0005 | 0.0010 | 0.0005 | 0.0127 | $0.0005 \pm 0.0001$ | $0.0010 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.52x** | 0.0016 | 0.0026 | 0.0019 | 0.0109 | $0.0017 \pm 0.0004$ | $0.0025 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.61x** | 0.0037 | 0.0065 | 0.0048 | 0.0161 | $0.0041 \pm 0.0012$ | $0.0065 \pm 0.0017$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.03x** | 0.0040 | 0.0043 | 0.0056 | 0.0151 | $0.0043 \pm 0.0016$ | $0.0042 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.19x** | 0.0038 | 0.0047 | 0.0066 | 0.0160 | $0.0041 \pm 0.0012$ | $0.0048 \pm 0.0009$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.36x** | 0.0088 | 0.0117 | 0.0143 | 0.0355 | $0.0091 \pm 0.0022$ | $0.0125 \pm 0.0039$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.37x** | 0.0326 | 0.0756 | 0.0402 | 0.103 | $0.0317 \pm 0.0065$ | $0.0742 \pm 0.0122$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **2.73x** | 0.0546 | 0.199 | 0.0721 | 0.253 | $0.0924 \pm 0.111$ | $0.192 \pm 0.0295$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.13x** | 0.0501 | 0.100 | 0.0747 | 0.156 | $0.0476 \pm 0.0052$ | $0.102 \pm 0.0153$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.64x** | 0.145 | 0.237 | 0.154 | 0.274 | $0.156 \pm 0.0508$ | $0.249 \pm 0.0436$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **12.56x** | 0.0632 | 0.733 | 0.0813 | 0.812 | $0.0592 \pm 0.0087$ | $0.737 \pm 0.0213$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.96x** | 0.0373 | 0.0736 | 0.0663 | 0.153 | $0.0389 \pm 0.0091$ | $0.0753 \pm 0.0084$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **1.98x** | 0.0417 | 0.0850 | 0.0671 | 0.166 | $0.0435 \pm 0.0086$ | $0.0852 \pm 0.0074$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.53x** | 0.0847 | 0.298 | 0.117 | 0.380 | $0.0863 \pm 0.0106$ | $0.304 \pm 0.0353$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.97x** | 0.334 | 1.356 | 0.450 | 1.814 | $0.342 \pm 0.0498$ | $1.350 \pm 0.121$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.19x** | 1.276 | 5.529 | 2.029 | 8.425 | $1.353 \pm 0.159$ | $5.659 \pm 0.412$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.76x** | 0.905 | 1.591 | 1.476 | 4.400 | $0.903 \pm 0.0475$ | $1.590 \pm 0.0637$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.70x** | 5.503 | 8.981 | 6.109 | 10.047 | $5.949 \pm 2.201$ | $9.888 \pm 2.809$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.50x** | 0.362 | 0.882 | 0.763 | 3.981 | $0.361 \pm 0.0293$ | $0.899 \pm 0.0639$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.33x** | 0.428 | 1.020 | 0.976 | 4.094 | $0.443 \pm 0.0339$ | $1.031 \pm 0.0452$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.97x** | 6.241 | 29.266 | 8.000 | 38.259 | $6.103 \pm 0.365$ | $30.385 \pm 2.387$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.58x** | 31.672 | 150.578 | 44.533 | 199.172 | $33.177 \pm 4.231$ | $151.191 \pm 11.530$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 7.3 LSAP Scalar vs. `GOEXPERIMENT=simd`

In [`assign/solver.go`](../assign/solver.go), [`simd.AllAbsLE64With`](../internal/simd/kernels_amd64.go) and [`simd.ValidateAndNegate64With`](../internal/simd/assign_amd64.go) validate finite cost bounds via AVX2 checks whenever `len(cost) >= 32` (while the fused `colReduceValidateSquare` fast path skips `AllAbsLE64With` on finite square `Minimize` matrices and dispatches `ColReduceValidateRows64With` only for $n \ge 32$), [`simd.Transpose64With`](../internal/simd/assign_amd64.go) handles tall transposes with $\text{rows} \ge 8$ and $\text{cols} \ge 8$, [`simd.MinReducedSkip64With`](../internal/simd/assign_amd64.go) accelerates reduction-transfer slack scans in `preprocessSquareJV` ($n \ge 32$), [`simd.Min2ReducedRow64With`](../internal/simd/kernels_amd64.go) accelerates first/second reduced-minimum scans in `preprocessRectangularBV` and `augmentingRowReduction` at `minAVX2Min2Len = 256` (`minAVX512Min2Len = 128`), and [`simd.InitReducedRow64With`](../internal/simd/kernels_amd64.go) runs on a slice of length $\max(m, n) \ge 32$ at the start of each Dijkstra augmentation. All vector kernels clean upper `YMM` state via `StoreArray` + `ClearAVXUpperBits()` (`VZEROUPPER`) before scalar operations; exact per-family verdicts appear in the generated table below.

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` with no `>= 8x8` tall transpose and either fused square `Minimize` non-sparse path or `rows * cols < 32`, so no AVX2 kernel runs): 1 of 3 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32`, tall `rows > cols` with `>= 8x8`, or fallback `AllAbsLE64`/`ValidateAndNegate64` at `rows * cols >= 32`): 19 faster, 0 slower, 0 with no detectable effect, out of 19.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source | 0.98x [0.94x-1.03x] | 0.0005 | 0.0005 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.06x [1.04x-1.08x] | 0.0016 | 0.0015 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source | 1.07x [0.99x-1.16x] | 0.0037 | 0.0038 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | SIMD faster | 1.28x [1.13x-1.46x] | 0.0040 | 0.0034 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | SIMD faster | 1.16x [1.11x-1.22x] | 0.0038 | 0.0033 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | SIMD faster | 1.27x [1.22x-1.32x] | 0.0088 | 0.0067 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.43x [1.22x-1.67x] | 0.0326 | 0.0244 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.63x [1.03x-2.58x] | 0.0546 | 0.0426 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.68x [1.64x-1.73x] | 0.0501 | 0.0308 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.70x [1.64x-1.77x] | 0.145 | 0.0898 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.23x [1.17x-1.30x] | 0.0632 | 0.0495 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.31x [1.07x-1.60x] | 0.0373 | 0.0288 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.27x [1.05x-1.54x] | 0.0417 | 0.0322 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.39x [1.31x-1.47x] | 0.0847 | 0.0603 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.57x [1.53x-1.61x] | 0.334 | 0.212 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.72x [1.57x-1.89x] | 1.276 | 0.811 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.25x [1.88x-2.69x] | 0.905 | 0.376 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.61x [2.47x-2.75x] | 5.503 | 2.019 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.78x [1.65x-1.92x] | 0.362 | 0.215 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.53x [1.32x-1.78x] | 0.428 | 0.293 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.70x [1.58x-1.82x] | 6.241 | 3.619 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.40x [1.30x-1.50x] | 31.672 | 25.063 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

---

## 8. Production Binary Footprint and Linker Dead-Code Elimination

The `goblin-bench -binary-size` mode compiles four standalone `CGO_ENABLED=0` programs in an isolated temporary module against the live workspace across both default `go build` and `GOEXPERIMENT=simd`, saving compiled artifact sizes and `go tool nm -size` symbol counts to `bench/results/goblin_binary_size.json`. [`ValidateBinarySizeTelemetry`](../bench/docgen.go) rejects the telemetry bundle if `assign_default` links any `math/big`, `encoding/json`, `VIPR`, or `Certif` symbols, if `goblin_default` links any `encoding/json`, `VIPR`, or `Certif` symbols, or if any profile links `encoding/json`:

<!-- BEGIN GENERATED:BINARY_SIZE_TABLE -->
| Profile | Default Raw / Stripped (KiB) | `GOEXPERIMENT=simd` Raw / Stripped (KiB) | Stripped Delta vs. Default | `goblin` Text (KiB) | `math/big` Syms | `encoding/json` Syms | `VIPR` Syms | `Certif` Syms |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `assign` default (`LinearSum` + `SolveInto`) | 2822.7 / **1768.2** | 2858.1 / **1832.2** | baseline | 118.7 | `0` | `0` | `0` | `0` |
| `assign` certified (`+ assign.WithCertified(true)`) | 2942.9 / **1844.2** | 2980.6 / **1916.2** | +76.0 KiB (+4.3%) | 177.2 | `0` | `0` | `0` | `17` |
| `goblin` default (`Model.Solve`, `DefaultOptions`) | 5915.6 / **3776.2** | 5798.6 / **3796.2** | baseline | 1168.5 | `130` | `0` | `0` | `0` |
| `goblin` certified + VIPR (`+ WithCertified + WithVIPRTrace`) | 7353.3 / **4752.2** | 7215.9 / **4772.2** | +976.0 KiB (+25.8%) | 1743.7 | `159` | `0` | `220` | `20` |
<!-- END GENERATED:BINARY_SIZE_TABLE -->

---

## 9. Pure-Go vs. In-Process C++ CBC Performance Analysis

Comparing isolated unpresolved `Root LP` timings, paired trial ratios, and solver-core checkpoints (`CP5 +Presolve`, `CP6 +Cuts`, `CP7 Full`) between `BackendGo` and in-process `BackendCBC` (`c1c171ec`) removes Python MPS formatting and OS process-spawn overhead across all 31 synthetic families and highlights four structural trade-offs on large degenerate LPs and heavy-tail branch-and-cut models:

<!-- BEGIN GENERATED:CBC_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `BackendCBC / BackendGo` Total [Clustered 95% CI] | Equal-Family `BackendCBC / BackendGo` Solve [Clustered 95% CI] | `BackendGo` Trial (Med) Sum (ms) | `BackendCBC` Trial (Med) Sum (ms) | Per-Instance `CBC / Go` Total Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro-models (`N <= 26`) | 10 (110) | **16.55x** [6.98x-39.28x] | **35.76x** [11.05x-115.75x] | 10.5 (0.9) | 155.9 (14.5) | 14.91x [2.13x-107.62x] |
| Tier 2: Medium MILPs (`30 <= N <= 84`) | 13 (143) | **5.94x** [4.10x-8.60x] | **6.60x** [4.32x-10.09x] | 609.9 (41.0) | 2034.1 (191.1) | 6.29x [0.63x-45.15x] |
| Tier 3: Large MILPs (`145 <= N <= 4036`) | 8 (40) | **3.26x** [1.79x-5.97x] | **3.62x** [1.90x-6.90x] | 1051.6 (203.9) | 2649.0 (536.1) | 3.42x [0.36x-14.29x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **4.59x** [2.91x-7.23x] | **4.80x** [2.95x-7.82x] | 1292.2 (190.8) | 3262.3 (426.0) | 5.14x [0.63x-45.15x] |
| All 31 synthetic families | 31 (293) | **7.08x** [4.80x-10.45x] | **9.75x** [5.78x-16.45x] | 1671.9 (245.8) | 4839.0 (741.6) | 7.30x [0.36x-107.62x] |
<!-- END GENERATED:CBC_SUMMARY_TABLE -->

### 9.1 MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families ($N = G(I + T + 3)$ variables, $M = G(2T + 3) + I$ constraints) separate solver behavior into three scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Paired `Go / CBC` | Go Total Med (ms) | CBC Total Med (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.07x** | 0.08 | 1.14 | 0.59x | 0.05 (1 / 19) | 1.11 (0 / 0) | 0.086 (28) | 0.145 (19) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.07x** | 0.58 | 7.34 | 0.77x | 0.53 (2 / 72) | 7.30 (14 / 44) | 0.118 (30) | 0.154 (18) |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.14x** | 0.20 | 1.44 | 0.58x | 0.15 (1 / 26) | 1.39 (0 / 0) | 0.113 (33) | 0.197 (21) |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.14x** | 0.28 | 2.09 | 0.84x | 0.16 (1 / 40) | 2.00 (0 / 0) | 0.276 (58) | 0.329 (40) |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.12x** | 0.42 | 3.39 | 0.60x | 0.34 (1 / 51) | 3.23 (0 / 0) | 0.398 (80) | 0.665 (63) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.29x** | 3.51 | 13.15 | 0.52x | 3.41 (43 / 483) | 13.09 (28 / 146) | 0.158 (38) | 0.303 (24) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.28x** | 20.25 | 105.13 | 0.85x | 20.18 (196 / 2289) | 105.04 (216 / 1352) | 0.302 (65) | 0.355 (54) |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.14x** | 0.82 | 6.09 | 1.09x | 0.63 (1 / 75) | 5.95 (0 / 0) | 0.764 (91) | 0.702 (94) |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.35x** | 1.35 | 4.06 | 1.54x | 1.00 (1 / 98) | 3.83 (0 / 0) | 1.845 (244) | 1.195 (149) |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **0.47x** | 9.75 | 27.71 | 0.82x | 8.65 (2 / 377) | 26.24 (0 / 0) | 3.168 (224) | 3.875 (319) |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **0.25x** | 41.28 | 256.29 | 1.74x | 34.73 (1 / 2439) | 251.89 (0 / 0) | 66.318 (1900) | 38.201 (3128) |
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
| `t3_mkp_200` | $200 \times 8$ | **0.81x** | $35.37 \pm 18.03$ | $41.84 \pm 20.02$ | 38.80 (250 / 2860) | 50.77 (154 / 1145) | 121.40 (1135 / 16) | 247.87 (5000, `NodeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **0.44x** | $8.07 \pm 8.67$ | $13.44 \pm 9.83$ | 5.93 (1 / 20) | 9.65 (0 / 8) | 16.25 (2 / 4) | 6.35 (118) |
| `t2_binpack_44` | $44 \times 14$ | **0.24x** | $1.89 \pm 1.18$ | $7.11 \pm 2.31$ | 1.59 (2 / 43) | 6.41 (0 / 8) | 1.87 (5 / 6) | 10.19 (772) |
| `t3_cflp_300` | $300 \times 34$ | **0.13x** | $9.01 \pm 6.29$ | $58.66 \pm 12.85$ | 7.13 (1 / 119) | 53.31 (18 / 483) | 20.86 (54 / 34) | 10.62 (116) |
| `t3_lotsizing_504` | $504 \times 336$ | **0.75x** | $81.79 \pm 52.40$ | $97.14 \pm 60.42$ | 94.35 (2 / 1351) | 74.31 (10 / 377) | 140.89 (376 / 206) | 250.09 (2333, `TimeLimit`) |
| `t3_sched_1000` | $1000 \times 100$ | **0.17x** | $12.35 \pm 6.60$ | $64.37 \pm 5.31$ | 9.52 (31 / 917) | 62.73 (0 / 76) | 33.10 (87 / 53) | 250.09 (1462, `TimeLimit`) |
<!-- END GENERATED:NON_MAD_CBC_GAP_TABLE -->

<!-- BEGIN GENERATED:NON_MAD_CBC_NOTES -->
- **Bin-packing tail behavior on `t2_binpack_44`**: pure-Go `BackendGo` ($1.89 \pm 1.18$ ms mean, 1.62 ms median) leads in-process `BackendCBC` ($7.11 \pm 2.31$ ms mean, 6.44 ms median) with a **4.24x** `BackendCBC / BackendGo` geometric-mean speedup (`Go / CBC = 0.24x`), running slower on 0 of 11 trials (peaking at 3.97 ms vs. 12.82 ms), with `CP5 +Presolve` at 10.19 ms vs. `CP6 +Cuts` at 1.87 ms.
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
| PMAD base (`G = 10, I = 2`) | 1 (11) | 0.423 (0.0360) | 0.113 (0.0099) | 1.148 (0.102) | 947.2 (16.8) [8/11] | 925.3 (82.9) [11/11] | 1749.4 (197.4) [11/11] | **809.5x** | **1962.6x** | **3467.8x** |
| PMAD small & medium (`I <= 10`) | 6 (66) | 155.849 (14.941) | 147.768 (12.930) | 499.020 (47.386) [2/6] | 15914.2 (1500.9) [7/66] | 13427.6 (1213.0) [22/66] | 16446.2 (1492.8) [21/66] | **332.4x** [51.4x-2147.6x] | **257.6x** [55.0x-1207.4x] | **340.4x** [58.9x-1967.3x] |
| PMAD large (`I = 12`) | 2 (10) | 354.478 (68.240) | 350.818 (68.981) | skip (>250k) | 2502.0 (500.4) [0/10] | 2547.5 (509.2) [0/10] | 2775.6 (555.0) [0/10] | **7.3x** [0.2x-218.2x] | **7.5x** [0.3x-194.7x] | **8.1x** [0.4x-149.5x] |
| Random-weight MAD controls (`I <= 12`) | 4 (44) | 5.593 (0.511) | 4.746 (0.426) | 314.123 (28.276) | 2326.0 (277.8) [38/44] | 4260.6 (415.3) [34/44] | 5280.3 (542.5) [33/44] | **118.3x** [21.5x-652.1x] | **589.2x** [284.9x-1218.2x] | **1059.4x** [464.7x-2415.3x] |
| **All 13 combinatorial MAD families** | 13 (131) | 516.344 (83.727) | 503.446 (82.347) | 814.291 (75.764) [7/13] | 21689.4 (2295.9) [53/131] | 21160.9 (2220.3) [67/131] | 26251.6 (2787.7) [65/131] | **144.1x** [45.0x-461.1x] | **225.3x** [71.5x-710.0x] | **324.9x** [91.0x-1160.9x] |
<!-- END GENERATED:MAD_COMBINATORIAL_SUMMARY_TABLE -->

<!-- BEGIN GENERATED:MAD_COMBINATORIAL_PIPELINE_TABLE -->
| Problem ID | $G \times I \times T$ | $\lambda$ | Auto Strategy | Auto Med (ms) | DP Med (ms) | DFS Med (ms) | MILP Go 250 ms | In-Process CBC 250 ms | Python PuLP+CBC 250 ms | Trial 0 Obj | Parity |
| :--- | :---: | ---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `pmad_base_g10_i2` | $10 \times 2 \times 6$ | 1.00 | `DP` | 0.0360 | 0.0099 | 0.102 | 16.83 (8/11 opt) | 82.87 (`Optimal`) | 197.40 (`Optimal`) | 24137.0 | `OK` |
| `pmad_g6_i6_lam025` | $6 \times 6 \times 6$ | 0.25 | `DP` | 0.131 | 0.0946 | 36.525 | 250.05 (4/11 opt) | 126.69 (`Optimal`) | 201.48 (9/11 opt) | 13259.5 | `OK` |
| `pmad_g6_i10_lam10` | $6 \times 10 \times 6$ | 1.00 | `DP` | 4.927 | 4.063 | skip (>250k) | 250.14 (`TimeLimit`) | 252.91 (`TimeLimit`) | 275.42 (`TimeLimit`) | 14018.0 | `OK` |
| `pmad_g6_i12_lam025` | $6 \times 12 \times 6$ | 0.25 | `DP` | 23.697 | 24.788 | skip (>250k) | 250.10 (`TimeLimit`) | 252.04 (`TimeLimit`) | 266.85 (`TimeLimit`) | 9787.0 | `OK` |
| `pmad_g10_i4_lam025` | $10 \times 4 \times 6$ | 0.25 | `DP` | 0.0852 | 0.0448 | 10.861 | 250.05 (3/11 opt) | 70.76 (`Optimal`) | 161.44 (`Optimal`) | 23954.0 | `OK` |
| `pmad_g10_i8_lam10` | $10 \times 8 \times 6$ | 1.00 | `DP` | 1.013 | 0.956 | skip (>250k) | 250.13 (`TimeLimit`) | 253.94 (`TimeLimit`) | 283.76 (`TimeLimit`) | 21928.0 | `OK` |
| `pmad_g10_i10_lam025` | $10 \times 10 \times 6$ | 0.25 | `DP` | 8.455 | 7.492 | skip (>250k) | 250.32 (`TimeLimit`) | 253.74 (`TimeLimit`) | 283.70 (1/11 opt) | 22570.0 | `OK` |
| `pmad_g20_i6_lam10` | $20 \times 6 \times 6$ | 1.00 | `DP` | 0.330 | 0.281 | skip (>250k) | 250.17 (`TimeLimit`) | 254.91 (`TimeLimit`) | 287.02 (`TimeLimit`) | 43530.0 | `OK` |
| `pmad_g20_i12_lam10` | $20 \times 12 \times 6$ | 1.00 | `DP` | 44.542 | 44.193 | skip (>250k) | 250.28 (`TimeLimit`) | 257.18 (`TimeLimit`) | 288.10 (`TimeLimit`) | 38256.0 | `OK` |
| `mad_comb_rand_g2_i4` | $2 \times 4 \times 6$ | 0.00 | `BruteForce` | 0.0176 | 0.0042 | 0.0024 | 0.60 (`Optimal`) | 11.74 (`Optimal`) | 38.22 (`Optimal`) | 4601.0 | `OK` |
| `mad_comb_rand_g2_i8` | $2 \times 8 \times 6$ | 0.00 | `BruteForce` | 0.0535 | 0.0489 | 0.0285 | 5.06 (`Optimal`) | 20.26 (`Optimal`) | 51.27 (`Optimal`) | 5975.0 | `OK` |
| `mad_comb_rand_g3_i9` | $3 \times 9 \times 6$ | 0.00 | `DP` | 0.0830 | 0.0510 | 0.353 | 22.14 (`Optimal`) | 131.56 (`Optimal`) | 188.07 (10/11 opt) | 7720.0 | `OK` |
| `mad_comb_rand_g4_i12` | $4 \times 12 \times 6$ | 0.00 | `DP` | 0.357 | 0.322 | 27.892 | 250.04 (5/11 opt) | 251.73 (1/11 opt) | 264.92 (1/11 opt) | 7905.0 | `OK` |
<!-- END GENERATED:MAD_COMBINATORIAL_PIPELINE_TABLE -->
