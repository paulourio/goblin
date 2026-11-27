# Benchmark Methodology and Corpus Results

The benchmark protocol below evaluates pure-Go `goblin` (`BackendGo`, zero `cgo` by default), Python `PuLP` driving the `COIN-OR CBC` (`c1c171ec`) command-line binary, and `goblin (CBC)` (`BackendCBC`, built with `-tags cbc`), which calls the same `c1c171ec` CBC/CLP stack in-process through `cgo`. Every timing and binary-size measurement in this document comes from a generated block rendered by [`bench/docgen.go`](../bench/docgen.go) from the JSON telemetry in `bench/results/`; the prose contains no hardcoded timings. The test [`doc_sync_test.go`](../doc_sync_test.go) fails `go test ./...` when a block drifts from the telemetry, when a parity check fails, or when any of the nine telemetry files lacks uniform git, source-fingerprint, and host-kernel provenance.

---

## 1. Environment, Provenance, and Protocol

```bash
go run ./cmd/goblin-bench -check-docs
```

- **CPU**: `13th Gen Intel(R) Core(TM) i9-13900K` (`x86_64`, 30 logical CPUs exposed by `lscpu`, AVX2 + FMA).
- **Default build**: `go build` with the pure-Go kernels (`BackendGo`). All headline timings use that build.
- **Experimental build**: `GOEXPERIMENT=simd`, compared to the default build only in section 5.

<!-- BEGIN GENERATED:PROVENANCE -->
- **Unified telemetry source**: fingerprint `2e436740e97c6e65` (working tree on top of commit `daa87923d9d36aed42936df0f9f5ffdad63acbf9`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `1`; Python + CBC `1`; Python + SciPy `1`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.84 ms median on a 1-variable MILP and 6.78 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
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
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **330.40x** [147.30x-741.11x] | 330.40x | 0.06x | 10.6 (0.9) | 155.4 (14.5) | 1129.5 (105.9) | 535.29x [30.76x-1224.21x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **18.74x** [11.40x-30.83x] | 18.74x | 0.18x | 571.2 (38.5) | 1753.9 (157.0) | 4506.1 (436.7) | 22.01x [1.33x-163.35x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **9.65x** [4.61x-20.18x] | 9.65x | 0.27x | 906.6 (169.7) | 2614.0 (524.3) | 6446.5 (1110.1) | 9.46x [1.04x-69.88x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **12.23x** [7.20x-20.79x] | 13.46x | 0.22x | 1147.1 (160.0) | 2984.2 (389.7) | 7056.0 (1020.2) | 12.47x [1.33x-163.35x] |
| Tier 3 non-MAD MILPs | 4 (20) | **7.27x** [1.27x-41.52x] | 7.27x | 0.30x | 585.9 (122.4) | 1293.2 (238.2) | 2904.6 (615.2) | 5.23x [1.49x-69.88x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **24.34x** [11.95x-49.60x] | 30.98x | 0.18x | 332.1 (48.3) | 1397.7 (292.7) | 4001.2 (536.4) | 34.18x [1.04x-126.48x] |
| Random-weight MAD (negative controls) | 3 (33) | **15.62x** [0.83x-293.71x] | 15.62x | 0.20x | 329.0 (23.6) | 972.5 (97.5) | 2349.5 (259.4) | 14.03x [1.33x-118.20x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **47.30x** [20.16x-111.01x] | 57.56x | 0.12x | 1156.3 (160.8) | 3125.6 (403.0) | 8081.0 (1116.4) | 42.37x [1.33x-1224.21x] |
| All 31 synthetic corpus families | 31 (293) | **39.85x** [20.87x-76.09x] | 50.27x | 0.14x | 1488.4 (209.0) | 4523.3 (695.7) | 12082.2 (1652.7) | 41.18x [1.04x-1224.21x] |
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
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **77.9x** [44.5x-126.5x] | 0.13 | 9.75 | 0.07 | 9.55 | 0.056 | 0.217 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **60.5x** [36.8x-118.2x] | 0.55 | 35.23 | 0.52 | 34.98 | 0.030 | 0.252 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **53.5x** [46.0x-65.7x] | 0.18 | 9.80 | 0.14 | 9.51 | 0.047 | 0.300 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **38.0x** [11.7x-50.2x] | 0.24 | 10.46 | 0.15 | 10.02 | 0.088 | 0.429 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **29.0x** [25.0x-35.5x] | 0.40 | 11.48 | 0.32 | 10.89 | 0.087 | 0.603 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **9.1x** [3.2x-23.6x] | 3.53 | 49.50 | 3.23 | 49.18 | 0.051 | 0.309 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **6.9x** [1.3x-28.5x] | 19.51 | 174.66 | 19.45 | 174.21 | 0.068 | 0.459 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **25.6x** [18.8x-33.9x] | 0.77 | 19.11 | 0.53 | 17.93 | 0.236 | 1.072 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **17.3x** [10.7x-22.6x] | 1.08 | 19.09 | 0.86 | 17.21 | 0.243 | 1.812 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **5.9x** [1.0x-14.5x] | 8.90 | 67.87 | 7.80 | 58.26 | 1.161 | 9.495 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **10.3x** [4.5x-24.5x] | 36.57 | 388.80 | 31.13 | 349.32 | 4.402 | 37.146 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Cross-Instance Latency Dispersion Across Randomized Instances (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.13 \pm 0.04$ ($\pm 0.03$) | $9.51 \pm 0.60$ ($\pm 0.40$) | $0.10 \pm 0.03$ ($\pm 0.02$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.58 \pm 0.28$ ($\pm 0.19$) | $32.28 \pm 10.05$ ($\pm 6.75$) | $0.54 \pm 0.26$ ($\pm 0.17$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.19 \pm 0.01$ ($\pm 0.01$) | $9.94 \pm 0.65$ ($\pm 0.44$) | $0.19 \pm 0.01$ ($\pm 0.01$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.31 \pm 0.21$ ($\pm 0.14$) | $10.60 \pm 0.57$ ($\pm 0.38$) | $0.31 \pm 0.09$ ($\pm 0.06$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.40 \pm 0.03$ ($\pm 0.02$) | $11.71 \pm 0.80$ ($\pm 0.54$) | $0.38 \pm 0.01$ ($\pm 0.00$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $4.64 \pm 3.54$ ($\pm 2.38$) | $37.03 \pm 17.73$ ($\pm 11.91$) | $4.80 \pm 3.93$ ($\pm 2.64$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $24.69 \pm 25.49$ ($\pm 17.12$) | $144.28 \pm 65.31$ ($\pm 43.87$) | $25.59 \pm 26.35$ ($\pm 17.70$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.73 \pm 0.10$ ($\pm 0.12$) | $18.71 \pm 2.42$ ($\pm 3.00$) | $0.60 \pm 0.04$ ($\pm 0.05$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.15 \pm 0.38$ ($\pm 0.47$) | $19.06 \pm 0.45$ ($\pm 0.56$) | $1.26 \pm 0.39$ ($\pm 0.48$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $18.81 \pm 24.33$ ($\pm 30.20$) | $69.03 \pm 4.09$ ($\pm 5.07$) | $18.57 \pm 23.41$ ($\pm 29.06$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $43.46 \pm 15.13$ ($\pm 18.78$) | $601.58 \pm 630.02$ ($\pm 782.15$) | $41.03 \pm 15.92$ ($\pm 19.76$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | **0.07** (1 / 0) | 0.04 (1 / 0) | 0.05 (1) | 0.57 (4) | 0.087 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.52** (2 / 0) | 0.63 (12 / 0) | 0.60 (12) | 0.82 (109) | 0.111 | 4601.00 | 4257.00 |
| `t2_mad_g2_i8` | **0.14** (1 / 0) | 0.91 (8 / 0) | 0.78 (8) | 0.78 (9) | 0.118 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.15** (1 / 0) | 0.14 (2 / 0) | 0.14 (2) | 2.87 (13) | 0.261 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.32** (1 / 0) | 3.54 (22 / 0) | 3.24 (22) | 6.61 (23) | 0.379 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.23** (43 / 0) | 3.18 (112 / 0) | 3.21 (112) | 4.08 (45) | 0.169 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **19.45** (196 / 0) | 33.53 (188 / 0) | 33.22 (188) | 98.95 (539) | 0.283 | 7720.00 | 6060.00 |
| `t3_mad_g5_i20` | **0.53** (1 / 0) | 3.26 (17 / 0) | 2.98 (17) | 0.53 (1) | 0.705 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.86** (1 / 0) | 9.80 (58 / 0) | 8.60 (58) | 14.78 (74) | 1.628 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.80** (2 / 0) | 21.69 (25 / 0) | 20.35 (25) | 250.17 (145, `TimeLimit`) | 2.799 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **31.13** (1 / 0) | 34.09 (2 / 0) | 33.02 (2) | 250.11 (6, `TimeLimit`) | 58.692 | 208000.00 | 208000.00 |
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
| `t1_std_lp` | 3 | 3 | **789.0x** [592.9x-941.0x] | 0.01 | 6.53 | 0.00 | 6.48 | 0.004 | 0.055 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **823.8x** [416.0x-1173.5x] | 0.01 | 6.59 | 0.00 | 6.54 | 0.004 | 0.055 | `OK` |
| `t1_std_mip` | 3 | 3 | **823.9x** [610.0x-1101.4x] | 0.01 | 7.47 | 0.00 | 7.41 | 0.003 | 0.057 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **928.3x** [708.6x-1111.7x] | 0.01 | 7.79 | 0.00 | 7.73 | 0.004 | 0.064 | `OK` |
| `t1_whiskas` | 6 | 5 | **480.7x** [352.4x-636.4x] | 0.01 | 6.51 | 0.01 | 6.44 | 0.006 | 0.076 | `OK` |
| `t1_furniture` | 2 | 3 | **259.9x** [70.5x-573.2x] | 0.02 | 8.26 | 0.02 | 8.20 | 0.004 | 0.054 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **74.2x** [30.8x-161.9x] | 0.15 | 9.91 | 0.14 | 9.81 | 0.017 | 0.099 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **712.7x** [301.9x-1224.2x] | 0.01 | 7.88 | 0.00 | 7.81 | 0.009 | 0.065 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **77.9x** [44.5x-126.5x] | 0.13 | 9.75 | 0.07 | 9.55 | 0.056 | 0.217 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **60.5x** [36.8x-118.2x] | 0.55 | 35.23 | 0.52 | 34.98 | 0.030 | 0.252 | `OK` |
| `t2_mkp_30` | 30 | 6 | **10.2x** [5.0x-14.6x] | 2.48 | 29.70 | 2.46 | 29.50 | 0.020 | 0.185 | `OK` |
| `t2_mkp_50` | 50 | 8 | **4.8x** [2.5x-29.6x] | 5.56 | 18.87 | 5.53 | 18.57 | 0.038 | 0.292 | `OK` |
| `t2_cflp_36` | 36 | 11 | **53.7x** [22.0x-163.3x] | 0.19 | 9.96 | 0.16 | 9.78 | 0.021 | 0.179 | `OK` |
| `t2_cflp_49` | 49 | 13 | **29.0x** [17.3x-89.5x] | 0.44 | 10.68 | 0.39 | 10.46 | 0.039 | 0.214 | `OK` |
| `t2_gap_45` | 45 | 13 | **20.0x** [4.4x-61.7x] | 0.87 | 14.93 | 0.84 | 14.73 | 0.024 | 0.212 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **11.6x** [4.6x-31.1x] | 3.68 | 70.91 | 3.64 | 70.66 | 0.033 | 0.256 | `OK` |
| `t2_binpack_44` | 44 | 14 | **11.5x** [4.4x-20.4x] | 1.14 | 14.03 | 1.11 | 13.83 | 0.042 | 0.214 | `OK` |
| `t2_setcover_50` | 50 | 20 | **46.8x** [5.5x-125.6x] | 0.21 | 11.72 | 0.16 | 11.40 | 0.040 | 0.329 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **53.5x** [46.0x-65.7x] | 0.18 | 9.80 | 0.14 | 9.51 | 0.047 | 0.300 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **38.0x** [11.7x-50.2x] | 0.24 | 10.46 | 0.15 | 10.02 | 0.088 | 0.429 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **29.0x** [25.0x-35.5x] | 0.40 | 11.48 | 0.32 | 10.89 | 0.087 | 0.603 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.1x** [3.2x-23.6x] | 3.53 | 49.50 | 3.23 | 49.18 | 0.051 | 0.309 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **6.9x** [1.3x-28.5x] | 19.51 | 174.66 | 19.45 | 174.21 | 0.068 | 0.459 | `OK` |
| `t3_mkp_200` | 200 | 8 | **4.3x** [3.2x-5.5x] | 23.49 | 120.50 | 23.36 | 119.87 | 0.098 | 0.652 | `OK` |
| `t3_cflp_300` | 300 | 34 | **31.5x** [7.6x-69.9x] | 6.86 | 250.29 | 6.75 | 249.40 | 0.118 | 0.960 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **2.5x** [1.5x-4.2x] | 83.48 | 163.60 | 83.23 | 161.69 | 0.252 | 1.921 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **8.3x** [3.8x-12.0x] | 8.53 | 80.81 | 8.18 | 77.85 | 0.358 | 2.937 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **25.6x** [18.8x-33.9x] | 0.77 | 19.11 | 0.53 | 17.93 | 0.236 | 1.072 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **17.3x** [10.7x-22.6x] | 1.08 | 19.09 | 0.86 | 17.21 | 0.243 | 1.812 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.9x** [1.0x-14.5x] | 8.90 | 67.87 | 7.80 | 58.26 | 1.161 | 9.495 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **10.3x** [4.5x-24.5x] | 36.57 | 388.80 | 31.13 | 349.32 | 4.402 | 37.146 | `OK` |
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
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 3 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 5 faster, 2 slower, 16 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 0.99x [0.80x-1.21x] | 0.004 | 0.004 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.12x [1.03x-1.21x] | 0.003 | 0.003 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 0.99x [0.95x-1.03x] | 0.005 | 0.005 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source | 0.91x [0.81x-1.01x] | 0.004 | 0.004 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 0.93x [0.81x-1.06x] | 0.007 | 0.007 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 0.95x [0.78x-1.15x] | 0.016 | 0.019 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.14x [1.07x-1.22x] | 0.137 | 0.117 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.60x [1.18x-2.18x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 1.17x [0.80x-1.70x] | 0.066 | 0.051 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 1.05x [0.97x-1.14x] | 0.521 | 0.498 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 0.97x [0.90x-1.05x] | 2.464 | 2.476 | 11 |
| `t2_mkp_50` | 58 | possible | SIMD faster | 1.08x [1.03x-1.14x] | 5.527 | 5.252 | 11 |
| `t2_cflp_36` | 47 | possible | no detectable effect | 0.98x [0.95x-1.01x] | 0.163 | 0.166 | 11 |
| `t2_cflp_49` | 62 | possible | SIMD faster | 1.12x [1.03x-1.22x] | 0.392 | 0.349 | 11 |
| `t2_gap_45` | 58 | possible | SIMD slower | 0.87x [0.79x-0.97x] | 0.844 | 0.872 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 0.99x [0.94x-1.04x] | 3.644 | 4.225 | 11 |
| `t2_binpack_44` | 58 | possible | SIMD faster | 1.12x [1.04x-1.21x] | 1.110 | 1.079 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 0.97x [0.85x-1.10x] | 0.161 | 0.200 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 0.96x [0.91x-1.01x] | 0.139 | 0.143 | 11 |
| `t2_mad_g3_i9` | 108 | possible | no detectable effect | 0.98x [0.79x-1.21x] | 0.152 | 0.167 | 11 |
| `t2_mad_g4_i12` | 156 | possible | SIMD faster | 1.04x [1.01x-1.07x] | 0.316 | 0.302 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 0.97x [0.92x-1.02x] | 3.226 | 3.358 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | SIMD slower | 0.97x [0.95x-0.99x] | 19.446 | 19.454 | 11 |
| `t3_mkp_200` | 208 | possible | no detectable effect | 1.02x [0.95x-1.11x] | 23.358 | 23.088 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 0.96x [0.82x-1.13x] | 6.747 | 6.352 | 5 |
| `t3_lotsizing_504` | 840 | possible | no detectable effect | 1.01x [0.96x-1.07x] | 83.229 | 82.724 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 1.02x [0.97x-1.07x] | 8.181 | 8.280 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 1.12x [0.99x-1.27x] | 0.528 | 0.453 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 0.95x [0.85x-1.06x] | 0.865 | 0.836 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 1.01x [0.98x-1.05x] | 7.797 | 8.016 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | SIMD faster | 1.07x [1.02x-1.12x] | 31.126 | 28.784 | 5 |
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
| `t1_whiskas` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.00 (1) | 0.004 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.003 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.14** (1 / 8) | 0.15 (2 / 8) | 0.22 (18) | 0.29 (35) | 0.010 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.002 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.07** (1 / 0) | 0.04 (1 / 0) | 0.05 (1) | 0.57 (4) | 0.087 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.52** (2 / 0) | 0.63 (12 / 0) | 0.60 (12) | 0.82 (109) | 0.111 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.46** (5 / 9) | 3.28 (40 / 9) | 1.27 (51) | 1.10 (61) | 0.029 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **5.53** (1 / 4) | 16.76 (2 / 4) | 6.17 (118) | 5.73 (111) | 0.050 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.16** (1 / 2) | 0.26 (3 / 2) | 0.18 (7) | 0.19 (7) | 0.028 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.39** (1 / 10) | 0.72 (4 / 10) | 0.34 (9) | 0.32 (9) | 0.040 | 909.02 | 827.30 |
| `t2_gap_45` | **0.84** (1 / 9) | 0.51 (1 / 9) | 2.06 (41) | 2.15 (60) | 0.030 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **3.64** (2 / 37) | 8.29 (25 / 37) | 3.39 (162) | 3.77 (199) | 0.064 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **1.11** (2 / 6) | 1.98 (5 / 6) | 9.51 (772) | 31.62 (1941) | 0.032 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.16** (1 / 6) | 0.40 (2 / 6) | 0.70 (6) | 0.67 (6) | 0.035 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.14** (1 / 0) | 0.91 (8 / 0) | 0.78 (8) | 0.78 (9) | 0.118 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.15** (1 / 0) | 0.14 (2 / 0) | 0.14 (2) | 2.87 (13) | 0.261 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.32** (1 / 0) | 3.54 (22 / 0) | 3.24 (22) | 6.61 (23) | 0.379 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.23** (43 / 0) | 3.18 (112 / 0) | 3.21 (112) | 4.08 (45) | 0.169 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **19.45** (196 / 0) | 33.53 (188 / 0) | 33.22 (188) | 98.95 (539) | 0.283 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **23.36** (250 / 14) | 112.35 (1135 / 16) | 236.86 (5000, `NodeLimit`) | 220.11 (5000, `NodeLimit`) | 0.144 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **6.75** (1 / 28) | 19.46 (54 / 34) | 9.45 (116) | 9.51 (118) | 0.178 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **83.23** (2 / 206) | 122.13 (376 / 206) | 250.04 (2780, `TimeLimit`) | 250.04 (2858, `TimeLimit`) | 0.782 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **8.18** (31 / 55) | 28.55 (87 / 53) | 250.10 (1664, `TimeLimit`) | 250.02 (1635, `TimeLimit`) | 0.224 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.53** (1 / 0) | 3.26 (17 / 0) | 2.98 (17) | 0.53 (1) | 0.705 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.86** (1 / 0) | 9.80 (58 / 0) | 8.60 (58) | 14.78 (74) | 1.628 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.80** (2 / 0) | 21.69 (25 / 0) | 20.35 (25) | 250.17 (145, `TimeLimit`) | 2.799 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **31.13** (1 / 0) | 34.09 (2 / 0) | 33.02 (2) | 250.11 (6, `TimeLimit`) | 58.692 | 208000.00 | 208000.00 |
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
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.43x** [1.23x-1.67x] | 1.43x | 4.17x | 0.230 (0.0201) | 0.331 (0.0304) | 1.46x [0.79x-2.63x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **2.92x** [1.81x-4.70x] | 2.92x | 2.79x | 9.507 (0.852) | 33.838 (3.071) | 2.51x [1.07x-15.72x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.79x** [1.83x-4.25x] | 2.79x | 3.62x | 228.003 (43.880) | 960.618 (190.933) | 2.48x [1.51x-5.73x] |
| Square families ($m = n$) | 16 (146) | **2.65x** [1.93x-3.64x] | 2.59x | 3.56x | 232.730 (43.895) | 983.645 (191.981) | 2.15x [0.79x-15.72x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.75x** [1.27x-2.42x] | 1.64x | 2.95x | 5.010 (0.857) | 11.141 (2.053) | 1.60x [0.89x-2.72x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.25x** [1.13x-4.49x] | 2.37x | 2.71x | 36.367 (6.367) | 67.141 (11.233) | 1.81x [0.79x-15.72x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.37x** [1.85x-3.03x] | 2.29x | 3.38x | 237.740 (44.752) | 994.786 (194.035) | 2.01x [0.79x-15.72x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 7.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.54x** | 0.0007 | 0.0009 | 0.0008 | 0.0130 | $0.0007 \pm 0.0001$ | $0.0010 \pm 0.0003$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.38x** | 0.0017 | 0.0026 | 0.0021 | 0.0112 | $0.0018 \pm 0.0005$ | $0.0025 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.70x** | 0.0036 | 0.0064 | 0.0053 | 0.0159 | $0.0038 \pm 0.0014$ | $0.0063 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.21x** | 0.0036 | 0.0040 | 0.0051 | 0.0147 | $0.0034 \pm 0.0006$ | $0.0041 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.20x** | 0.0038 | 0.0050 | 0.0055 | 0.0158 | $0.0040 \pm 0.0009$ | $0.0047 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.63x** | 0.0067 | 0.0115 | 0.0116 | 0.0340 | $0.0072 \pm 0.0026$ | $0.0114 \pm 0.0020$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.62x** | 0.0313 | 0.0730 | 0.0438 | 0.0993 | $0.0286 \pm 0.0082$ | $0.0728 \pm 0.0121$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.20x** | 0.0582 | 0.182 | 0.0898 | 0.235 | $0.0578 \pm 0.0053$ | $0.186 \pm 0.0274$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **1.89x** | 0.0514 | 0.102 | 0.0909 | 0.158 | $0.0554 \pm 0.0132$ | $0.103 \pm 0.0147$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.63x** | 0.141 | 0.233 | 0.168 | 0.272 | $0.152 \pm 0.0458$ | $0.243 \pm 0.0455$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **12.18x** | 0.0617 | 0.713 | 0.0825 | 0.791 | $0.0589 \pm 0.0082$ | $0.711 \pm 0.0108$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.84x** | 0.0369 | 0.0729 | 0.0789 | 0.155 | $0.0425 \pm 0.0125$ | $0.0757 \pm 0.0087$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **1.94x** | 0.0419 | 0.0843 | 0.0697 | 0.163 | $0.0442 \pm 0.0101$ | $0.0844 \pm 0.0076$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.57x** | 0.0837 | 0.291 | 0.109 | 0.380 | $0.0824 \pm 0.0093$ | $0.294 \pm 0.0317$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.82x** | 0.346 | 1.321 | 0.454 | 1.796 | $0.343 \pm 0.0450$ | $1.305 \pm 0.125$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.67x** | 1.357 | 5.407 | 2.088 | 7.981 | $1.500 \pm 0.291$ | $5.446 \pm 0.433$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.66x** | 0.916 | 1.524 | 1.478 | 4.404 | $0.909 \pm 0.0432$ | $1.512 \pm 0.0561$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.69x** | 5.187 | 8.644 | 5.832 | 9.534 | $5.755 \pm 2.079$ | $9.551 \pm 2.834$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.51x** | 0.356 | 0.881 | 0.751 | 3.820 | $0.346 \pm 0.0313$ | $0.867 \pm 0.0611$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.22x** | 0.414 | 1.006 | 0.837 | 3.910 | $0.449 \pm 0.0558$ | $0.990 \pm 0.0293$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.84x** | 5.841 | 28.686 | 7.703 | 38.136 | $6.050 \pm 0.327$ | $29.337 \pm 2.374$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.74x** | 29.809 | 144.784 | 39.429 | 197.365 | $30.592 \pm 3.548$ | $144.420 \pm 9.864$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 7.3 LSAP Scalar vs. `GOEXPERIMENT=simd`

In [`assign/solver.go`](../assign/solver.go), [`simd.AllAbsLE64With`](../internal/simd/kernels_amd64.go) and [`simd.ValidateAndNegate64With`](../internal/simd/assign_amd64.go) validate finite cost bounds via AVX2 checks whenever `len(cost) >= 32` (while the fused `colReduceValidateSquare` fast path skips `AllAbsLE64With` on finite square `Minimize` matrices and dispatches `ColReduceValidateRows64With` only for $n \ge 32$), [`simd.Transpose64With`](../internal/simd/assign_amd64.go) handles tall transposes with $\text{rows} \ge 8$ and $\text{cols} \ge 8$, [`simd.MinReducedSkip64With`](../internal/simd/assign_amd64.go) accelerates reduction-transfer slack scans in `preprocessSquareJV` ($n \ge 32$), [`simd.Min2ReducedRow64With`](../internal/simd/kernels_amd64.go) accelerates first/second reduced-minimum scans in `preprocessRectangularBV` and `augmentingRowReduction` at `minAVX2Min2Len = 256` (`minAVX512Min2Len = 128`), and [`simd.InitReducedRow64With`](../internal/simd/kernels_amd64.go) runs on a slice of length $\max(m, n) \ge 32$ at the start of each Dijkstra augmentation. All vector kernels clean upper `YMM` state via `StoreArray` + `ClearAVXUpperBits()` (`VZEROUPPER`) before scalar operations; exact per-family verdicts appear in the generated table below.

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` with no `>= 8x8` tall transpose and either fused square `Minimize` non-sparse path or `rows * cols < 32`, so no AVX2 kernel runs): 2 of 3 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32`, tall `rows > cols` with `>= 8x8`, or fallback `AllAbsLE64`/`ValidateAndNegate64` at `rows * cols >= 32`): 19 faster, 0 slower, 0 with no detectable effect, out of 19.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.38x [1.22x-1.55x] | 0.0007 | 0.0005 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.19x [1.02x-1.38x] | 0.0017 | 0.0016 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source | 1.01x [0.94x-1.08x] | 0.0036 | 0.0036 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | SIMD faster | 1.10x [1.05x-1.14x] | 0.0036 | 0.0033 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | SIMD faster | 1.15x [1.09x-1.22x] | 0.0038 | 0.0033 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | SIMD faster | 1.04x [1.00x-1.08x] | 0.0067 | 0.0065 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.33x [1.28x-1.39x] | 0.0313 | 0.0229 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.44x [1.34x-1.54x] | 0.0582 | 0.0414 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.78x [1.37x-2.32x] | 0.0514 | 0.0301 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.62x [1.33x-1.97x] | 0.141 | 0.0871 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.26x [1.18x-1.33x] | 0.0617 | 0.0487 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.45x [1.25x-1.68x] | 0.0369 | 0.0282 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.45x [1.26x-1.67x] | 0.0419 | 0.0292 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.37x [1.21x-1.55x] | 0.0837 | 0.0556 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.65x [1.55x-1.76x] | 0.346 | 0.199 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.97x [1.41x-2.74x] | 1.357 | 0.722 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.59x [2.46x-2.72x] | 0.916 | 0.356 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.64x [2.42x-2.88x] | 5.187 | 1.851 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.82x [1.67x-1.98x] | 0.356 | 0.192 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.58x [1.35x-1.83x] | 0.414 | 0.285 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.62x [1.49x-1.77x] | 5.841 | 3.602 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.42x [1.32x-1.52x] | 29.809 | 22.445 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

---

## 8. Production Binary Footprint and Linker Dead-Code Elimination

The `goblin-bench -binary-size` mode compiles six standalone `CGO_ENABLED=0` programs in an isolated temporary module against the live workspace across both default `go build` and `GOEXPERIMENT=simd`, saving compiled artifact sizes and `go tool nm -size` symbol counts to `bench/results/goblin_binary_size.json`. [`ValidateBinarySizeTelemetry`](../bench/docgen.go) rejects the telemetry bundle if `assign_default` or `mad_default` links any `math/big`, `encoding/json`, `VIPR`, or `Certif` symbols, if `mad_certified` links any `math/big`, `encoding/json`, or `VIPR` symbols or omits `Certif` symbols, if `goblin_default` links any `encoding/json`, `VIPR`, or `Certif` symbols, or if any profile links `encoding/json`:

<!-- BEGIN GENERATED:BINARY_SIZE_TABLE -->
| Profile | Default Raw / Stripped (KiB) | `GOEXPERIMENT=simd` Raw / Stripped (KiB) | Stripped Delta vs. Default | `goblin` Text (KiB) | `math/big` Syms | `encoding/json` Syms | `VIPR` Syms | `Certif` Syms |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `assign` default (`LinearSum` + `SolveInto`) | 2822.8 / **1768.2** | 2858.1 / **1832.2** | baseline | 118.7 | `0` | `0` | `0` | `0` |
| `assign` certified (`+ assign.WithCertified(true)`) | 2942.9 / **1844.2** | 2980.7 / **1916.2** | +76.0 KiB (+4.3%) | 177.2 | `0` | `0` | `0` | `17` |
| `goblin/mad` default (`mad.Solve`, `StrategyAuto`) | 2783.9 / **1760.2** | 2714.5 / **1760.2** | baseline | 117.8 | `0` | `0` | `0` | `0` |
| `goblin/mad` certified (`+ mad.WithCertified(true)`) | 2978.2 / **1888.2** | 2900.4 / **1888.2** | +128.0 KiB (+7.3%) | 201.2 | `0` | `0` | `0` | `5` |
| `goblin` default (`Model.Solve`, `DefaultOptions`) | 5586.4 / **3560.2** | 5472.3 / **3576.2** | baseline | 1023.4 | `130` | `0` | `0` | `0` |
| `goblin` certified + VIPR (`+ WithCertified + WithVIPRTrace`) | 6831.2 / **4400.2** | 6697.1 / **4420.2** | +840.0 KiB (+23.6%) | 1503.1 | `155` | `0` | `220` | `14` |
<!-- END GENERATED:BINARY_SIZE_TABLE -->

---

## 9. Pure-Go vs. In-Process C++ CBC Performance Analysis

Comparing isolated unpresolved `Root LP` timings, paired trial ratios, and solver-core checkpoints (`CP5 +Presolve`, `CP6 +Cuts`, `CP7 Full`) between `BackendGo` and in-process `BackendCBC` (`c1c171ec`) removes Python MPS formatting and OS process-spawn overhead across all 31 synthetic families and highlights four structural trade-offs on large degenerate LPs and heavy-tail branch-and-cut models:

<!-- BEGIN GENERATED:CBC_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `BackendCBC / BackendGo` Total [Clustered 95% CI] | Equal-Family `BackendCBC / BackendGo` Solve [Clustered 95% CI] | `BackendGo` Trial (Med) Sum (ms) | `BackendCBC` Trial (Med) Sum (ms) | Per-Instance `CBC / Go` Total Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro-models (`N <= 26`) | 10 (110) | **17.84x** [7.86x-40.50x] | **37.20x** [12.26x-112.89x] | 10.6 (0.9) | 155.4 (14.5) | 13.00x [2.08x-144.41x] |
| Tier 2: Medium MILPs (`30 <= N <= 84`) | 13 (143) | **5.55x** [3.97x-7.76x] | **6.22x** [4.22x-9.14x] | 571.2 (38.5) | 1753.9 (157.0) | 5.61x [0.56x-43.80x] |
| Tier 3: Large MILPs (`145 <= N <= 4036`) | 8 (40) | **3.75x** [2.16x-6.51x] | **4.15x** [2.27x-7.57x] | 906.6 (169.7) | 2614.0 (524.3) | 3.80x [0.38x-14.54x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **4.62x** [3.10x-6.89x] | **4.86x** [3.16x-7.49x] | 1147.1 (160.0) | 2984.2 (389.7) | 4.98x [0.56x-43.80x] |
| All 31 synthetic families | 31 (293) | **7.31x** [5.03x-10.61x] | **9.97x** [6.03x-16.50x] | 1488.4 (209.0) | 4523.3 (695.7) | 7.20x [0.38x-144.41x] |
<!-- END GENERATED:CBC_SUMMARY_TABLE -->

### 9.1 MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families ($N = G(I + T + 3)$ variables, $M = G(2T + 3) + I$ constraints) separate solver behavior into three scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Paired `Go / CBC` | Go Total Med (ms) | CBC Total Med (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.10x** | 0.13 | 1.20 | 0.58x | 0.07 (1 / 19) | 1.16 (0 / 0) | 0.087 (28) | 0.151 (19) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.07x** | 0.55 | 7.44 | 0.72x | 0.52 (2 / 72) | 7.37 (14 / 44) | 0.111 (30) | 0.153 (18) |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.13x** | 0.18 | 1.43 | 0.66x | 0.14 (1 / 26) | 1.38 (0 / 0) | 0.118 (33) | 0.179 (21) |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.15x** | 0.24 | 1.81 | 0.86x | 0.15 (1 / 40) | 1.74 (0 / 0) | 0.261 (58) | 0.302 (40) |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.17x** | 0.40 | 2.19 | 0.82x | 0.32 (1 / 51) | 2.09 (0 / 0) | 0.379 (80) | 0.462 (63) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.33x** | 3.53 | 11.23 | 0.95x | 3.23 (43 / 483) | 11.18 (28 / 146) | 0.169 (38) | 0.177 (24) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.33x** | 19.51 | 78.80 | 0.92x | 19.45 (196 / 2289) | 78.73 (216 / 1352) | 0.283 (65) | 0.309 (54) |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.12x** | 0.77 | 6.13 | 1.05x | 0.53 (1 / 75) | 5.96 (0 / 0) | 0.705 (91) | 0.673 (94) |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.27x** | 1.08 | 4.19 | 1.47x | 0.86 (1 / 98) | 3.97 (0 / 0) | 1.628 (244) | 1.111 (149) |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **0.45x** | 8.90 | 27.53 | 0.74x | 7.80 (2 / 377) | 25.44 (0 / 0) | 2.799 (224) | 3.782 (319) |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **0.22x** | 36.57 | 248.26 | 1.65x | 31.13 (1 / 2439) | 243.41 (0 / 0) | 58.692 (1900) | 35.655 (3128) |
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
| `t3_mkp_200` | $200 \times 8$ | **0.55x** | $24.49 \pm 13.14$ | $42.02 \pm 19.95$ | 23.36 (250 / 2860) | 51.65 (154 / 1145) | 112.35 (1135 / 16) | 236.86 (5000, `NodeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **0.47x** | $8.06 \pm 8.55$ | $12.70 \pm 9.34$ | 5.53 (1 / 20) | 8.68 (0 / 8) | 16.76 (2 / 4) | 6.17 (118) |
| `t2_binpack_44` | $44 \times 14$ | **0.18x** | $1.49 \pm 0.88$ | $7.26 \pm 2.36$ | 1.11 (2 / 43) | 6.69 (0 / 8) | 1.98 (5 / 6) | 9.51 (772) |
| `t3_cflp_300` | $300 \times 34$ | **0.13x** | $8.79 \pm 6.10$ | $56.87 \pm 12.02$ | 6.75 (1 / 119) | 51.39 (18 / 483) | 19.46 (54 / 34) | 9.45 (116) |
| `t3_lotsizing_504` | $504 \times 336$ | **0.67x** | $72.79 \pm 46.85$ | $96.81 \pm 61.11$ | 83.23 (2 / 1351) | 72.99 (10 / 377) | 122.13 (376 / 206) | 250.04 (2780, `TimeLimit`) |
| `t3_sched_1000` | $1000 \times 100$ | **0.16x** | $11.10 \pm 6.15$ | $62.94 \pm 4.87$ | 8.18 (31 / 917) | 61.02 (0 / 76) | 28.55 (87 / 53) | 250.10 (1664, `TimeLimit`) |
<!-- END GENERATED:NON_MAD_CBC_GAP_TABLE -->

<!-- BEGIN GENERATED:NON_MAD_CBC_NOTES -->
- **Bin-packing tail behavior on `t2_binpack_44`**: pure-Go `BackendGo` ($1.49 \pm 0.88$ ms mean, 1.14 ms median) leads in-process `BackendCBC` ($7.26 \pm 2.36$ ms mean, 6.73 ms median) with a **5.41x** `BackendCBC / BackendGo` geometric-mean speedup (`Go / CBC = 0.18x`), running slower on 0 of 11 trials (peaking at 3.02 ms vs. 13.21 ms), with `CP5 +Presolve` at 9.51 ms vs. `CP6 +Cuts` at 1.98 ms.
<!-- END GENERATED:NON_MAD_CBC_NOTES -->
- **Bounded node budget on `coinor_capplan1`**: on the largest held-out public MILP in `testdata/public/manifest.json` (`coinor_capplan1`, 2,172 variables and 726 constraints), [`public_benchmarks_test.go`](../public_benchmarks_test.go) runs `BackendGo` under a bounded 25-node budget with cuts and heuristics disabled (`WithCuts(false)`, `WithHeuristics(false)`, `WithMaxNodes(25)`) to verify a valid global dual lower bound at `StatusNodeLimit`, whereas in-process `BackendCBC` solves the same instance to `StatusOptimal` without node, cut, or heuristic caps.

---

## 10. Reproducing Benchmarks and Syncing Documentation

Run all commands from the `goblin/` repository root on a quiet host so that all nine telemetry files record uniform `git_commit`, `source_fingerprint`, and `uname_release`:

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
Automated formal inventory across the workspace covers `24` Lean files (`53,872` LOC: `1,325` authored `theorem` and `lemma` declarations, `5,876` kernel `TheoremInfo` entries, and `220` axiom locks), `6` Go certification files (`20,349` LOC) paired with `31` formal and certified test files (`34,861` LOC), and `50` `Fuzz*` targets (`42` default-build across `./...` plus `8` `-tags=cbc`, matching [`scripts/gcp-fuzz/targets.tsv`](../scripts/gcp-fuzz/targets.tsv)).
<!-- END GENERATED:BENCHMARKS_FORMAL_SUMMARY -->

---

## 11. Combinatorial Group-Item Allocation Benchmarks (`goblin/mad` vs. Algebraic MILP)

The isolated 13-family combinatorial MAD corpus ([`bench/mad_corpus.go`](../bench/mad_corpus.go), manifest in `testdata/mad_corpus/manifest.json`) evaluates `gb.SolveMAD` (`MADAuto`, `MADDP`, and `MADBruteForce` when DFS states are at most `mad.MaxDFSStates = 250,000`) against 250 ms-capped algebraic MILP solves (`gb.BuildMADModel` via `Model.Solve` on `BackendGo` with `WithMaxNodes(0)` and on `BackendCBC`) across 131 randomized trials. Default `Model.Solve` and Section 3 remain 100% on pure Branch-and-Cut.

- **Corpus structure**: `pmad_base_g10_i2` defines the canonical 10-group, 2-item, 6-period PMAD base instance on Trial 0 (`Objective = 24137`, `Assignment = [6, 9]`), 8 PMAD families span $G \in \{6, 10, 20\}$, $I \in \{4..12\}$, and $\lambda \in \{0.25, 1.0\}$ (`RoundWeight = 6 * lambda`), and 4 `mad_comb_rand_*` families mirror `BuildMADRandom` weights with `DesignatedItems = [I-G..I-1]` and `RoundWeight = 1.0`.
- **Parity and containment**: `CheckMADParity` requires `1e-9` objective parity across `MADAuto`, `MADDP`, and `MADBruteForce`, and `[BestBound, Incumbent]` containment plus `1e-5` `StatusOptimal` parity on both 250 ms-capped MILP baselines.

<!-- BEGIN GENERATED:MAD_COMBINATORIAL_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Auto Trial (Med) Sum (ms) | DP Trial (Med) Sum (ms) | MILP Go Trial (Med) Sum (ms) [Opt] | MILP CBC Trial (Med) Sum (ms) [Opt] | Equal-Family `MILP Go / Auto` [95% CI] | Equal-Family `MILP CBC / Auto` [95% CI] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| PMAD base (`G = 10, I = 2`) | 1 (11) | 0.265 (0.0229) | 0.108 (0.0099) | 918.8 (16.5) [8/11] | 761.4 (71.6) [11/11] | **1201.9x** | **2625.7x** |
| PMAD small & medium (`I <= 10`) | 6 (66) | 100.005 (9.016) | 98.141 (8.912) | 15762.5 (1500.7) [8/66] | 12841.4 (1172.5) [26/66] | **460.7x** [75.6x-2809.4x] | **334.2x** [81.1x-1377.4x] |
| PMAD large (`I = 12`) | 2 (10) | 308.667 (61.413) | 307.559 (61.469) | 2502.5 (500.3) [0/10] | 2551.4 (510.7) [0/10] | **8.6x** [0.1x-849.2x] | **8.8x** [0.1x-785.4x] |
| Random-weight MAD controls (`I <= 12`) | 4 (44) | 4.633 (0.419) | 4.459 (0.404) | 2318.3 (271.9) [38/44] | 3936.3 (374.4) [34/44] | **171.6x** [46.8x-629.4x] | **842.1x** [401.3x-1767.1x] |
| **All 13 combinatorial MAD families** | 13 (131) | 413.569 (70.871) | 410.267 (70.795) | 21502.0 (2289.4) [54/131] | 20090.6 (2129.1) [71/131] | **198.5x** [62.1x-634.3x] | **297.5x** [93.6x-945.4x] |
<!-- END GENERATED:MAD_COMBINATORIAL_SUMMARY_TABLE -->

<!-- BEGIN GENERATED:MAD_COMBINATORIAL_PIPELINE_TABLE -->
| Problem ID | $G \times I \times T$ | $\lambda$ | Auto Strategy | Auto Med (ms) | DP Med (ms) | DFS Med (ms) | MILP Go 250 ms | MILP CBC 250 ms | Trial 0 Obj | Parity |
| :--- | :---: | ---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `pmad_base_g10_i2` | $10 \times 2 \times 6$ | 1.00 | `DP` | 0.0229 | 0.0099 | 0.0989 | 16.48 (8/11 opt) | 71.60 (`Optimal`) | 24137.0 | `OK` |
| `pmad_g6_i6_lam025` | $6 \times 6 \times 6$ | 0.25 | `DP` | 0.100 | 0.0899 | 27.881 | 250.04 (4/11 opt) | 105.37 (`Optimal`) | 13259.5 | `OK` |
| `pmad_g6_i10_lam10` | $6 \times 10 \times 6$ | 1.00 | `DP` | 2.943 | 2.982 | skip (>250k) | 250.11 (`TimeLimit`) | 251.97 (`TimeLimit`) | 14018.0 | `OK` |
| `pmad_g6_i12_lam025` | $6 \times 12 \times 6$ | 0.25 | `DP` | 19.963 | 19.852 | skip (>250k) | 250.11 (`TimeLimit`) | 253.70 (`TimeLimit`) | 9787.0 | `OK` |
| `pmad_g10_i4_lam025` | $10 \times 4 \times 6$ | 0.25 | `DP` | 0.0499 | 0.0333 | 8.727 | 250.07 (4/11 opt) | 55.35 (`Optimal`) | 23954.0 | `OK` |
| `pmad_g10_i8_lam10` | $10 \times 8 \times 6$ | 1.00 | `DP` | 0.765 | 0.743 | skip (>250k) | 250.12 (`TimeLimit`) | 252.84 (3/11 opt) | 21928.0 | `OK` |
| `pmad_g10_i10_lam025` | $10 \times 10 \times 6$ | 0.25 | `DP` | 4.875 | 4.794 | skip (>250k) | 250.21 (`TimeLimit`) | 252.67 (1/11 opt) | 22570.0 | `OK` |
| `pmad_g20_i6_lam10` | $20 \times 6 \times 6$ | 1.00 | `DP` | 0.283 | 0.270 | skip (>250k) | 250.18 (`TimeLimit`) | 254.26 (`TimeLimit`) | 43530.0 | `OK` |
| `pmad_g20_i12_lam10` | $20 \times 12 \times 6$ | 1.00 | `DP` | 41.451 | 41.617 | skip (>250k) | 250.21 (`TimeLimit`) | 256.98 (`TimeLimit`) | 38256.0 | `OK` |
| `mad_comb_rand_g2_i4` | $2 \times 4 \times 6$ | 0.00 | `BruteForce` | 0.0071 | 0.0039 | 0.0023 | 0.53 (`Optimal`) | 11.16 (`Optimal`) | 4601.0 | `OK` |
| `mad_comb_rand_g2_i8` | $2 \times 8 \times 6$ | 0.00 | `BruteForce` | 0.0255 | 0.0310 | 0.0186 | 3.19 (`Optimal`) | 12.55 (`Optimal`) | 5975.0 | `OK` |
| `mad_comb_rand_g3_i9` | $3 \times 9 \times 6$ | 0.00 | `DP` | 0.0532 | 0.0458 | 0.261 | 18.10 (`Optimal`) | 99.67 (`Optimal`) | 7720.0 | `OK` |
| `mad_comb_rand_g4_i12` | $4 \times 12 \times 6$ | 0.00 | `DP` | 0.333 | 0.323 | 27.843 | 250.04 (5/11 opt) | 251.02 (1/11 opt) | 7905.0 | `OK` |
<!-- END GENERATED:MAD_COMBINATORIAL_PIPELINE_TABLE -->
