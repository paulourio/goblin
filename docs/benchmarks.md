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
- **Unified telemetry source**: fingerprint `e8f74b711ec92a3a` (working tree on top of commit `42bb45704ce9f8882b4a9927442adb1db22857bd`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.46 ms median on a 1-variable MILP and 6.26 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
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
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **326.69x** [163.01x-654.73x] | 326.69x | 0.05x | 8.8 (0.8) | 195.9 (19.6) | 1050.7 (99.4) | 502.89x [30.67x-1789.59x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **20.39x** [12.48x-33.31x] | 20.39x | 0.15x | 646.8 (32.5) | 1952.8 (170.7) | 4352.7 (422.4) | 25.71x [0.13x-149.03x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **10.36x** [5.09x-21.09x] | 10.36x | 0.20x | 881.1 (138.7) | 2932.3 (565.2) | 6225.4 (1069.3) | 10.15x [1.13x-68.62x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **13.25x** [8.02x-21.88x] | 14.53x | 0.20x | 1218.7 (126.2) | 3112.8 (394.4) | 6828.0 (994.6) | 14.56x [0.13x-149.03x] |
| Tier 3 non-MAD MILPs | 4 (20) | **8.02x** [1.64x-39.14x] | 8.02x | 0.27x | 580.2 (94.4) | 1247.8 (231.1) | 2820.4 (603.1) | 6.55x [1.13x-68.62x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **26.95x** [12.27x-59.15x] | 35.04x | 0.11x | 310.2 (45.0) | 1797.1 (344.0) | 3854.3 (506.6) | 36.62x [1.14x-123.61x] |
| Random-weight MAD (negative controls) | 3 (33) | **16.89x** [0.82x-347.04x] | 16.89x | 0.14x | 298.1 (20.2) | 1209.4 (115.9) | 2263.3 (251.2) | 14.89x [1.32x-120.63x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **48.84x** [21.66x-110.10x] | 59.02x | 0.11x | 1226.5 (126.9) | 3283.7 (411.4) | 7774.6 (1084.5) | 46.40x [0.13x-1789.59x] |
| All 31 synthetic corpus families | 31 (293) | **41.89x** [22.51x-77.96x] | 52.66x | 0.11x | 1536.7 (172.0) | 5080.9 (755.5) | 11628.8 (1591.1) | 43.79x [0.13x-1789.59x] |
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
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **102.2x** [73.0x-123.6x] | 0.09 | 9.56 | 0.05 | 9.35 | 0.036 | 0.214 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **68.0x** [30.7x-120.6x] | 0.47 | 34.25 | 0.45 | 33.98 | 0.024 | 0.244 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **59.6x** [44.2x-72.9x] | 0.15 | 9.43 | 0.13 | 9.15 | 0.030 | 0.284 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **43.4x** [33.1x-51.3x] | 0.23 | 10.10 | 0.15 | 9.69 | 0.062 | 0.408 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **32.8x** [29.5x-37.2x] | 0.35 | 11.35 | 0.28 | 10.74 | 0.068 | 0.598 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **9.9x** [3.6x-25.7x] | 3.16 | 47.99 | 3.11 | 47.67 | 0.040 | 0.305 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **7.2x** [1.3x-29.5x] | 16.55 | 169.01 | 16.46 | 168.59 | 0.059 | 0.437 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **31.9x** [24.7x-36.8x] | 0.56 | 17.76 | 0.44 | 16.65 | 0.115 | 1.021 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **16.7x** [10.2x-22.6x] | 1.12 | 18.54 | 0.87 | 16.71 | 0.245 | 1.720 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **5.9x** [1.1x-14.2x] | 8.61 | 60.68 | 7.54 | 52.74 | 1.129 | 8.127 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **10.2x** [4.5x-25.1x] | 33.93 | 369.20 | 30.27 | 335.55 | 4.025 | 34.453 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Cross-Instance Latency Dispersion Across Randomized Instances (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.09 \pm 0.01$ ($\pm 0.01$) | $9.46 \pm 0.38$ ($\pm 0.26$) | $0.07 \pm 0.01$ ($\pm 0.01$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.50 \pm 0.28$ ($\pm 0.19$) | $30.69 \pm 9.56$ ($\pm 6.42$) | $0.51 \pm 0.28$ ($\pm 0.19$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.16 \pm 0.02$ ($\pm 0.01$) | $9.66 \pm 0.55$ ($\pm 0.37$) | $0.16 \pm 0.01$ ($\pm 0.01$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.24 \pm 0.04$ ($\pm 0.03$) | $10.09 \pm 0.14$ ($\pm 0.09$) | $0.23 \pm 0.04$ ($\pm 0.03$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.35 \pm 0.02$ ($\pm 0.01$) | $11.63 \pm 0.52$ ($\pm 0.35$) | $0.37 \pm 0.05$ ($\pm 0.03$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $4.15 \pm 3.18$ ($\pm 2.14$) | $35.81 \pm 17.04$ ($\pm 11.45$) | $4.11 \pm 3.03$ ($\pm 2.04$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $22.45 \pm 22.06$ ($\pm 14.82$) | $139.26 \pm 63.80$ ($\pm 42.86$) | $22.12 \pm 21.89$ ($\pm 14.71$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.55 \pm 0.02$ ($\pm 0.02$) | $17.84 \pm 2.43$ ($\pm 3.02$) | $0.58 \pm 0.07$ ($\pm 0.08$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.16 \pm 0.39$ ($\pm 0.48$) | $18.69 \pm 0.37$ ($\pm 0.45$) | $1.19 \pm 0.35$ ($\pm 0.43$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $16.60 \pm 20.59$ ($\pm 25.56$) | $63.15 \pm 3.93$ ($\pm 4.88$) | $16.44 \pm 20.71$ ($\pm 25.71$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $41.87 \pm 14.22$ ($\pm 17.66$) | $581.33 \pm 615.01$ ($\pm 763.52$) | $38.43 \pm 14.56$ ($\pm 18.08$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | **0.05** (1 / 0) | 0.04 (1 / 0) | 0.05 (1) | 0.52 (4) | 0.085 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.45** (2 / 0) | 0.58 (12 / 0) | 0.56 (12) | 0.80 (109) | 0.102 | 4601.00 | 4257.00 |
| `t2_mad_g2_i8` | **0.13** (1 / 0) | 0.82 (8 / 0) | 0.73 (8) | 0.73 (9) | 0.102 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.15** (1 / 0) | 0.13 (2 / 0) | 0.13 (2) | 2.76 (13) | 0.253 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.28** (1 / 0) | 3.22 (22 / 0) | 2.95 (22) | 5.43 (23) | 0.349 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.11** (43 / 0) | 3.05 (112 / 0) | 2.85 (112) | 3.64 (45) | 0.151 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **16.46** (201 / 0) | 30.70 (188 / 0) | 30.04 (188) | 91.42 (539) | 0.274 | 7720.00 | 6060.00 |
| `t3_mad_g5_i20` | **0.44** (1 / 0) | 3.01 (17 / 0) | 2.67 (17) | 0.52 (1) | 0.650 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.87** (1 / 0) | 8.17 (58 / 0) | 7.57 (58) | 13.13 (74) | 1.557 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.54** (2 / 0) | 21.60 (25 / 0) | 19.43 (25) | 250.06 (150, `TimeLimit`) | 2.651 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **30.27** (1 / 0) | 32.89 (2 / 0) | 31.61 (2) | 250.18 (6, `TimeLimit`) | 57.228 | 208000.00 | 208000.00 |
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
| `t1_std_lp` | 3 | 3 | **651.7x** [424.0x-858.0x] | 0.01 | 6.13 | 0.00 | 6.08 | 0.004 | 0.048 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **628.4x** [320.6x-1053.0x] | 0.01 | 6.25 | 0.00 | 6.20 | 0.005 | 0.052 | `OK` |
| `t1_std_mip` | 3 | 3 | **658.9x** [486.7x-858.2x] | 0.01 | 7.27 | 0.01 | 7.22 | 0.004 | 0.051 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **730.1x** [576.6x-1076.4x] | 0.01 | 7.15 | 0.00 | 7.10 | 0.005 | 0.052 | `OK` |
| `t1_whiskas` | 6 | 5 | **469.1x** [329.4x-670.4x] | 0.01 | 6.23 | 0.01 | 6.15 | 0.007 | 0.074 | `OK` |
| `t1_furniture` | 2 | 3 | **247.8x** [72.1x-727.4x] | 0.02 | 7.65 | 0.02 | 7.60 | 0.005 | 0.052 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **98.2x** [45.6x-330.6x] | 0.11 | 8.83 | 0.10 | 8.74 | 0.011 | 0.084 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **885.2x** [110.3x-1789.6x] | 0.01 | 6.10 | 0.00 | 6.06 | 0.005 | 0.042 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **102.2x** [73.0x-123.6x] | 0.09 | 9.56 | 0.05 | 9.35 | 0.036 | 0.214 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **68.0x** [30.7x-120.6x] | 0.47 | 34.25 | 0.45 | 33.98 | 0.024 | 0.244 | `OK` |
| `t2_mkp_30` | 30 | 6 | **11.6x** [6.4x-17.4x] | 2.11 | 28.30 | 2.09 | 28.12 | 0.020 | 0.179 | `OK` |
| `t2_mkp_50` | 50 | 8 | **6.1x** [3.2x-36.4x] | 4.37 | 16.76 | 4.33 | 16.52 | 0.029 | 0.257 | `OK` |
| `t2_cflp_36` | 36 | 11 | **58.4x** [25.0x-149.0x] | 0.15 | 9.58 | 0.14 | 9.40 | 0.017 | 0.175 | `OK` |
| `t2_cflp_49` | 49 | 13 | **30.6x** [17.5x-84.3x] | 0.39 | 10.48 | 0.34 | 10.28 | 0.035 | 0.207 | `OK` |
| `t2_gap_45` | 45 | 13 | **21.0x** [4.7x-67.5x] | 0.96 | 14.35 | 0.95 | 14.15 | 0.020 | 0.204 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **13.2x** [5.8x-34.1x] | 3.30 | 69.52 | 3.28 | 69.26 | 0.028 | 0.256 | `OK` |
| `t2_binpack_44` | 44 | 14 | **10.7x** [0.1x-34.1x] | 0.62 | 14.05 | 0.60 | 13.84 | 0.035 | 0.204 | `OK` |
| `t2_setcover_50` | 50 | 20 | **46.9x** [6.7x-122.3x] | 0.19 | 11.49 | 0.16 | 11.19 | 0.038 | 0.314 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **59.6x** [44.2x-72.9x] | 0.15 | 9.43 | 0.13 | 9.15 | 0.030 | 0.284 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **43.4x** [33.1x-51.3x] | 0.23 | 10.10 | 0.15 | 9.69 | 0.062 | 0.408 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **32.8x** [29.5x-37.2x] | 0.35 | 11.35 | 0.28 | 10.74 | 0.068 | 0.598 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.9x** [3.6x-25.7x] | 3.16 | 47.99 | 3.11 | 47.67 | 0.040 | 0.305 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **7.2x** [1.3x-29.5x] | 16.55 | 169.01 | 16.46 | 168.59 | 0.059 | 0.437 | `OK` |
| `t3_mkp_200` | 200 | 8 | **5.4x** [4.1x-6.6x] | 18.83 | 115.99 | 18.75 | 115.37 | 0.073 | 0.612 | `OK` |
| `t3_cflp_300` | 300 | 34 | **27.2x** [7.9x-68.6x] | 6.09 | 249.14 | 5.99 | 248.22 | 0.111 | 0.903 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **2.6x** [1.1x-5.0x] | 62.89 | 157.60 | 62.70 | 155.73 | 0.267 | 1.871 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **10.8x** [5.2x-17.7x] | 6.62 | 80.35 | 6.35 | 77.43 | 0.284 | 2.795 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **31.9x** [24.7x-36.8x] | 0.56 | 17.76 | 0.44 | 16.65 | 0.115 | 1.021 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **16.7x** [10.2x-22.6x] | 1.12 | 18.54 | 0.87 | 16.71 | 0.245 | 1.720 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.9x** [1.1x-14.2x] | 8.61 | 60.68 | 7.54 | 52.74 | 1.129 | 8.127 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **10.2x** [4.5x-25.1x] | 33.93 | 369.20 | 30.27 | 335.55 | 4.025 | 34.453 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## 5. Default Build vs. `GOEXPERIMENT=simd`

In [`internal/simd/kernels_amd64.go`](../internal/simd/kernels_amd64.go) and [`internal/simd/kernels_avx512_amd64.go`](../internal/simd/kernels_avx512_amd64.go), runtime CPUID dispatch selects AVX-512, AVX2, or 4-way scalar loops from a single portable `CGO_ENABLED=0` binary (`GOAMD64=v3` FMA-safe). Each vector kernel spills accumulators via `StoreArray` and executes `archsimd.ClearAVXUpperBits()` (`VZEROUPPER`) before scalar tail or caller operations, eliminating AVX-to-SSE transition stalls while preserving IEEE-754 `NaN` propagation parity. AVX2 activates at `minAVX2Len = 32` (`Dot64`, `Axpy64`, `Scale64`, `AbsMax64`, `RowActivityBoundsFinite`, `SelectDualLeaveRow64With`, `MinReducedSkip64With`, `InitReducedRow64`, `AllAbsLE64With`, `ValidateAndNegate64With`), `rows >= 8 && cols >= 8` for `Transpose64With`, `minDualRatioSIMDChunks = 4` (256 cols for `ScanDualRatioCand64With`), and `minAVX2Min2Len = 256` (`Min2ReducedRow64`); on AVX-512 hosts, dense LSAP loops dispatch AVX-512 at `minAVX512Len = 64` and `minAVX512Min2Len = 128` while sparse LP kernels gate AVX-512 at `minAVX512LPLen = 2048` / `minLeaveRowAVX512 = 2048` / `minDualRatioAVX512Chunks = 128` (8,192 cols) to avoid Intel Emerald Rapids License-2 clock down-binning across surrounding scalar LU code.

At the full-solver level, each ratio below is the geometric mean of per-trial `scalar / SIMD` solve times over the same instances on the local AVX2 host (`i9-13900K`), with a 95% $t$-interval on the log ratios; "no detectable effect" means the interval contains 1. "Reachable" is a necessary condition: the AVX2 LP kernels require `n + m >= 32`. Below that size no vector kernel can run, so those small models serve as negative controls where intervals excluding 1 reflect binary code-layout or run-to-run variance rather than SIMD execution. Per-family paired speedups and 95% intervals for larger models appear in the generated table below.

### 5.1 Scope and Pending Target-Hardware Verification

All telemetry in this document is measured locally on the AVX2-only `13th Gen Intel(R) Core(TM) i9-13900K` host (`x86_64`, Linux). Portable `GOOS=linux GOARCH=arm64 CGO_ENABLED=0` and `GOAMD64=v3`/`v4` builds compile and pass local build checks, whereas fresh physical AVX-512 and `linux/arm64` runtime test and benchmark re-verification on the current tree remain pending target hardware access.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 1 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 7 faster, 2 slower, 14 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 1.02x [0.97x-1.08x] | 0.004 | 0.004 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 1.15x [0.88x-1.50x] | 0.004 | 0.003 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 0.98x [0.89x-1.08x] | 0.006 | 0.006 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source | 1.00x [0.97x-1.03x] | 0.004 | 0.004 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 0.99x [0.92x-1.06x] | 0.005 | 0.006 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 1.01x [1.00x-1.03x] | 0.016 | 0.016 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.88x [0.82x-0.94x] | 0.097 | 0.110 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 0.79x [0.54x-1.16x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | SIMD faster | 1.28x [1.17x-1.40x] | 0.051 | 0.043 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 0.99x [0.96x-1.01x] | 0.449 | 0.449 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 0.98x [0.92x-1.03x] | 2.091 | 2.273 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 1.02x [1.00x-1.04x] | 4.329 | 4.252 | 11 |
| `t2_cflp_36` | 47 | possible | SIMD slower | 0.88x [0.85x-0.91x] | 0.139 | 0.168 | 11 |
| `t2_cflp_49` | 62 | possible | SIMD faster | 1.21x [1.17x-1.25x] | 0.339 | 0.288 | 11 |
| `t2_gap_45` | 58 | possible | SIMD slower | 0.95x [0.90x-1.00x] | 0.946 | 1.037 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 0.98x [0.94x-1.02x] | 3.279 | 3.505 | 11 |
| `t2_binpack_44` | 58 | possible | SIMD faster | 1.12x [1.06x-1.19x] | 0.600 | 0.602 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 1.04x [0.96x-1.11x] | 0.160 | 0.169 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 1.03x [0.95x-1.12x] | 0.125 | 0.123 | 11 |
| `t2_mad_g3_i9` | 108 | possible | no detectable effect | 1.09x [0.99x-1.20x] | 0.149 | 0.146 | 11 |
| `t2_mad_g4_i12` | 156 | possible | no detectable effect | 0.99x [0.92x-1.06x] | 0.283 | 0.277 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.00x [0.97x-1.04x] | 3.110 | 3.094 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | SIMD faster | 1.02x [1.00x-1.03x] | 16.455 | 16.893 | 11 |
| `t3_mkp_200` | 208 | possible | SIMD faster | 1.06x [1.02x-1.11x] | 18.754 | 16.748 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.04x [0.97x-1.12x] | 5.991 | 5.866 | 5 |
| `t3_lotsizing_504` | 840 | possible | SIMD faster | 1.03x [1.01x-1.05x] | 62.697 | 59.434 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 1.01x [0.97x-1.04x] | 6.347 | 6.171 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 1.01x [0.92x-1.11x] | 0.438 | 0.420 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 1.04x [0.95x-1.14x] | 0.868 | 0.795 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 1.03x [0.99x-1.06x] | 7.540 | 7.115 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | SIMD faster | 1.12x [1.06x-1.18x] | 30.271 | 26.619 | 5 |
<!-- END GENERATED:SIMD_EFFECT_TABLE -->

---

## 6. Full Corpus Solver-Core Checkpoint Progression

<!-- BEGIN GENERATED:FULL_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_std_lp` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 54.00 | 54.00 |
| `t1_std_lp_max` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 80.00 | 80.00 |
| `t1_std_mip` | **0.01** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.01 (3) | 0.002 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.01 (3) | 0.002 | 64.95 | 58.90 |
| `t1_whiskas` | **0.01** (1 / 0) | 0.00 (1 / 0) | 0.01 (1) | 0.00 (1) | 0.003 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.02 (1) | 0.02 (1) | 0.002 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.10** (1 / 8) | 0.10 (2 / 8) | 0.14 (18) | 0.21 (35) | 0.007 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.002 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.05** (1 / 0) | 0.04 (1 / 0) | 0.05 (1) | 0.52 (4) | 0.085 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.45** (2 / 0) | 0.58 (12 / 0) | 0.56 (12) | 0.80 (109) | 0.102 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.09** (5 / 9) | 2.60 (40 / 9) | 1.03 (51) | 0.92 (61) | 0.024 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **4.33** (1 / 4) | 11.52 (2 / 4) | 4.69 (118) | 4.15 (111) | 0.038 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.14** (1 / 2) | 0.22 (3 / 2) | 0.17 (7) | 0.18 (7) | 0.026 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.34** (1 / 10) | 0.57 (4 / 10) | 0.30 (9) | 0.28 (9) | 0.035 | 909.02 | 827.30 |
| `t2_gap_45` | **0.95** (1 / 9) | 0.37 (1 / 9) | 1.71 (41) | 2.21 (60) | 0.025 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **3.28** (2 / 37) | 6.40 (25 / 37) | 2.98 (162) | 3.24 (199) | 0.061 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **0.60** (2 / 8) | 1.02 (5 / 8) | 8.19 (772) | 26.38 (1953) | 0.027 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.16** (1 / 6) | 0.33 (2 / 6) | 0.66 (6) | 0.64 (6) | 0.031 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.13** (1 / 0) | 0.82 (8 / 0) | 0.73 (8) | 0.73 (9) | 0.102 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.15** (1 / 0) | 0.13 (2 / 0) | 0.13 (2) | 2.76 (13) | 0.253 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.28** (1 / 0) | 3.22 (22 / 0) | 2.95 (22) | 5.43 (23) | 0.349 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.11** (43 / 0) | 3.05 (112 / 0) | 2.85 (112) | 3.64 (45) | 0.151 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **16.46** (201 / 0) | 30.70 (188 / 0) | 30.04 (188) | 91.42 (539) | 0.274 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **18.75** (250 / 14) | 81.94 (1135 / 16) | 165.12 (5000, `NodeLimit`) | 147.57 (5000, `NodeLimit`) | 0.120 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **5.99** (1 / 28) | 17.82 (54 / 34) | 8.56 (116) | 8.60 (118) | 0.169 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **62.70** (2 / 206) | 78.82 (376 / 206) | 250.03 (3737, `TimeLimit`) | 250.04 (3867, `TimeLimit`) | 0.724 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **6.35** (31 / 55) | 19.62 (87 / 53) | 250.02 (2595, `TimeLimit`) | 250.05 (2630, `TimeLimit`) | 0.193 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.44** (1 / 0) | 3.01 (17 / 0) | 2.67 (17) | 0.52 (1) | 0.650 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.87** (1 / 0) | 8.17 (58 / 0) | 7.57 (58) | 13.13 (74) | 1.557 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.54** (2 / 0) | 21.60 (25 / 0) | 19.43 (25) | 250.06 (150, `TimeLimit`) | 2.651 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **30.27** (1 / 0) | 32.89 (2 / 0) | 31.61 (2) | 250.18 (6, `TimeLimit`) | 57.228 | 208000.00 | 208000.00 |
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
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.37x** [1.14x-1.63x] | 1.37x | 4.43x | 0.219 (0.0195) | 0.315 (0.0286) | 1.42x [0.25x-2.47x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **2.96x** [1.81x-4.83x] | 2.96x | 3.02x | 10.067 (0.794) | 33.524 (2.973) | 2.56x [0.68x-18.89x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.78x** [1.78x-4.35x] | 2.78x | 3.57x | 232.189 (45.684) | 949.892 (188.524) | 2.41x [0.90x-6.10x] |
| Square families ($m = n$) | 16 (146) | **2.61x** [1.85x-3.67x] | 2.54x | 3.68x | 237.633 (45.618) | 972.876 (189.579) | 2.13x [0.25x-18.89x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.78x** [1.29x-2.45x] | 1.68x | 3.17x | 4.842 (0.879) | 10.855 (1.947) | 1.96x [0.68x-2.59x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.28x** [1.10x-4.73x] | 2.44x | 2.88x | 36.340 (6.415) | 65.183 (10.995) | 1.83x [0.90x-18.89x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.35x** [1.81x-3.05x] | 2.27x | 3.53x | 242.475 (46.498) | 983.731 (191.526) | 2.00x [0.25x-18.89x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 7.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.22x** | 0.0005 | 0.0008 | 0.0006 | 0.0111 | $0.0009 \pm 0.0009$ | $0.0009 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.25x** | 0.0017 | 0.0023 | 0.0021 | 0.0104 | $0.0019 \pm 0.0007$ | $0.0023 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.70x** | 0.0035 | 0.0062 | 0.0045 | 0.0153 | $0.0035 \pm 0.0006$ | $0.0061 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.16x** | 0.0036 | 0.0039 | 0.0047 | 0.0140 | $0.0034 \pm 0.0007$ | $0.0039 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.29x** | 0.0039 | 0.0044 | 0.0048 | 0.0152 | $0.0037 \pm 0.0008$ | $0.0047 \pm 0.0014$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.67x** | 0.0062 | 0.0110 | 0.0102 | 0.0326 | $0.0065 \pm 0.0017$ | $0.0107 \pm 0.0018$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.35x** | 0.0288 | 0.0704 | 0.0361 | 0.0961 | $0.0304 \pm 0.0109$ | $0.0690 \pm 0.0105$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.42x** | 0.0520 | 0.178 | 0.0672 | 0.226 | $0.0513 \pm 0.0051$ | $0.176 \pm 0.0257$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.06x** | 0.0479 | 0.0941 | 0.0704 | 0.141 | $0.0464 \pm 0.0061$ | $0.0959 \pm 0.0143$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.62x** | 0.139 | 0.220 | 0.153 | 0.256 | $0.148 \pm 0.0456$ | $0.234 \pm 0.0453$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **13.48x** | 0.0587 | 0.690 | 0.0768 | 0.779 | $0.0546 \pm 0.0084$ | $0.731 \pm 0.0834$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.90x** | 0.0352 | 0.0693 | 0.0592 | 0.143 | $0.0397 \pm 0.0191$ | $0.0713 \pm 0.0086$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.04x** | 0.0387 | 0.0792 | 0.0629 | 0.155 | $0.0393 \pm 0.0037$ | $0.0803 \pm 0.0072$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.53x** | 0.0796 | 0.278 | 0.103 | 0.356 | $0.0804 \pm 0.0087$ | $0.284 \pm 0.0317$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.50x** | 0.314 | 1.294 | 0.439 | 1.875 | $0.425 \pm 0.324$ | $1.306 \pm 0.145$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.12x** | 1.221 | 5.191 | 1.631 | 7.865 | $1.291 \pm 0.152$ | $5.304 \pm 0.397$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.55x** | 0.839 | 1.483 | 1.481 | 4.308 | $0.991 \pm 0.326$ | $1.486 \pm 0.0509$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.63x** | 5.321 | 8.491 | 5.611 | 9.412 | $5.707 \pm 2.007$ | $9.179 \pm 2.651$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.41x** | 0.354 | 0.817 | 0.753 | 3.716 | $0.349 \pm 0.0298$ | $0.839 \pm 0.0597$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.28x** | 0.444 | 0.973 | 0.818 | 3.845 | $0.430 \pm 0.0248$ | $0.979 \pm 0.0275$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.96x** | 6.003 | 28.630 | 8.562 | 38.265 | $5.867 \pm 0.340$ | $29.131 \pm 2.053$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.52x** | 31.502 | 142.939 | 41.900 | 188.974 | $31.802 \pm 3.856$ | $143.060 \pm 9.735$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 7.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSkip64`, `InitReducedRow64`, and `Min2ReducedRow64`)

In [`assign/solver.go`](../assign/solver.go), [`simd.AllAbsLE64With`](../internal/simd/kernels_amd64.go) and [`simd.ValidateAndNegate64With`](../internal/simd/assign_amd64.go) validate finite cost bounds via AVX2 checks whenever `len(cost) >= 32` (while the fused `colReduceValidateSquare` fast path skips `AllAbsLE64With` on finite square `Minimize` matrices and dispatches `ColReduceValidateRows64With` only for $n \ge 32$), [`simd.Transpose64With`](../internal/simd/assign_amd64.go) handles tall transposes with $\text{rows} \ge 8$ and $\text{cols} \ge 8$, [`simd.MinReducedSkip64With`](../internal/simd/assign_amd64.go) accelerates reduction-transfer slack scans in `preprocessSquareJV` ($n \ge 32$), [`simd.Min2ReducedRow64With`](../internal/simd/kernels_amd64.go) accelerates first/second reduced-minimum scans in `preprocessRectangularBV` and `augmentingRowReduction` at `minAVX2Min2Len = 256` (`minAVX512Min2Len = 128`), and [`simd.InitReducedRow64With`](../internal/simd/kernels_amd64.go) runs on a slice of length $\max(m, n) \ge 32$ at the start of each Dijkstra augmentation. All vector kernels clean upper `YMM` state via `StoreArray` + `ClearAVXUpperBits()` (`VZEROUPPER`) before scalar operations; exact per-family verdicts appear in the generated table below.

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` with no `>= 8x8` tall transpose and either fused square `Minimize` non-sparse path or `rows * cols < 32`, so no AVX2 kernel runs): 1 of 3 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32`, tall `rows > cols` with `>= 8x8`, or fallback `AllAbsLE64`/`ValidateAndNegate64` at `rows * cols >= 32`): 18 faster, 0 slower, 1 with no detectable effect, out of 19.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source | 0.99x [0.97x-1.01x] | 0.0005 | 0.0006 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.04x [1.01x-1.07x] | 0.0017 | 0.0017 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source | 0.99x [0.96x-1.02x] | 0.0035 | 0.0037 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | SIMD faster | 1.09x [1.06x-1.13x] | 0.0036 | 0.0032 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | SIMD faster | 1.12x [1.10x-1.15x] | 0.0039 | 0.0034 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | no detectable effect | 0.97x [0.94x-1.01x] | 0.0062 | 0.0063 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.22x [1.17x-1.28x] | 0.0288 | 0.0231 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.28x [1.22x-1.35x] | 0.0520 | 0.0413 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.67x [1.60x-1.73x] | 0.0479 | 0.0296 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.70x [1.63x-1.76x] | 0.139 | 0.0834 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.26x [1.23x-1.30x] | 0.0587 | 0.0462 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.29x [1.22x-1.37x] | 0.0352 | 0.0274 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.32x [1.30x-1.34x] | 0.0387 | 0.0295 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.38x [1.30x-1.46x] | 0.0796 | 0.0575 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.78x [1.35x-2.35x] | 0.314 | 0.196 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.72x [1.61x-1.83x] | 1.221 | 0.746 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.80x [2.05x-3.83x] | 0.839 | 0.330 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.64x [2.50x-2.79x] | 5.321 | 1.924 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.85x [1.56x-2.20x] | 0.354 | 0.195 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.54x [1.49x-1.59x] | 0.444 | 0.283 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.66x [1.60x-1.73x] | 6.003 | 3.691 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.65x [1.56x-1.75x] | 31.502 | 18.834 | 5 |
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
