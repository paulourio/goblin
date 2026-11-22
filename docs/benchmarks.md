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
- **Unified telemetry source**: fingerprint `81fe97d73f995897` (commit `57a94d9275d4806331d57a52231a5b8f89d65947`), `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.02 ms median on a 1-variable MILP and 5.74 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
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
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **362.95x** [164.25x-802.01x] | 362.95x | 0.04x | 8.4 (0.7) | 191.8 (19.2) | 979.5 (93.0) | 533.76x [37.08x-2309.73x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **18.45x** [10.75x-31.68x] | 18.45x | 0.15x | 738.8 (32.5) | 1951.4 (170.9) | 4103.2 (398.4) | 24.18x [0.11x-141.11x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **9.66x** [4.46x-20.94x] | 9.66x | 0.21x | 795.7 (158.3) | 2854.3 (551.7) | 6048.1 (1040.6) | 9.86x [1.08x-77.00x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **11.92x** [6.84x-20.78x] | 12.99x | 0.20x | 1225.7 (143.4) | 3086.1 (390.5) | 6528.7 (956.4) | 15.26x [0.11x-141.11x] |
| Tier 3 non-MAD MILPs | 4 (20) | **7.49x** [1.13x-49.52x] | 7.49x | 0.29x | 494.9 (111.6) | 1225.6 (227.4) | 2737.4 (585.9) | 5.20x [2.18x-77.00x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **25.47x** [11.67x-55.59x] | 33.31x | 0.10x | 309.7 (47.5) | 1743.9 (334.6) | 3716.2 (491.3) | 38.76x [1.08x-119.22x] |
| Random-weight MAD (negative controls) | 3 (33) | **16.10x** [0.83x-310.97x] | 16.10x | 0.14x | 320.6 (18.2) | 1196.8 (114.6) | 2177.7 (241.5) | 17.47x [1.42x-94.27x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **48.10x** [19.99x-115.73x] | 58.45x | 0.11x | 1233.1 (144.1) | 3253.5 (407.2) | 7414.6 (1040.7) | 45.22x [0.11x-2309.73x] |
| All 31 synthetic corpus families | 31 (293) | **40.82x** [20.95x-79.53x] | 51.69x | 0.11x | 1542.9 (191.5) | 4997.5 (741.8) | 11130.9 (1532.0) | 42.55x [0.11x-2309.73x] |
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
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **95.3x** [64.6x-119.2x] | 0.09 | 8.71 | 0.05 | 8.51 | 0.033 | 0.252 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **62.8x** [37.1x-94.3x] | 0.49 | 31.87 | 0.46 | 31.67 | 0.027 | 0.257 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **56.9x** [53.4x-63.9x] | 0.15 | 8.46 | 0.11 | 8.20 | 0.036 | 0.332 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **48.3x** [27.9x-60.5x] | 0.18 | 9.10 | 0.12 | 8.72 | 0.051 | 0.488 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **28.1x** [23.2x-35.0x] | 0.38 | 10.33 | 0.27 | 9.77 | 0.099 | 0.733 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **9.7x** [2.3x-28.3x] | 2.68 | 45.47 | 2.64 | 45.22 | 0.038 | 0.341 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **6.8x** [1.4x-26.2x] | 15.07 | 164.12 | 15.01 | 163.71 | 0.060 | 0.536 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **26.4x** [17.5x-30.8x] | 0.61 | 16.55 | 0.45 | 15.51 | 0.143 | 1.269 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **15.7x** [9.3x-20.6x] | 1.00 | 16.83 | 0.77 | 15.17 | 0.244 | 2.170 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **5.8x** [1.1x-14.6x] | 8.08 | 58.85 | 6.97 | 51.09 | 1.286 | 10.632 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **9.9x** [4.5x-24.7x] | 36.99 | 362.47 | 32.32 | 329.87 | 4.584 | 42.721 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Cross-Instance Latency Dispersion Across Randomized Instances (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.09 \pm 0.02$ ($\pm 0.01$) | $8.51 \pm 0.33$ ($\pm 0.22$) | $0.09 \pm 0.01$ ($\pm 0.01$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.50 \pm 0.23$ ($\pm 0.16$) | $28.77 \pm 9.21$ ($\pm 6.18$) | $0.51 \pm 0.24$ ($\pm 0.16$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.15 \pm 0.01$ ($\pm 0.00$) | $8.64 \pm 0.47$ ($\pm 0.32$) | $0.15 \pm 0.01$ ($\pm 0.01$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.19 \pm 0.05$ ($\pm 0.03$) | $9.11 \pm 0.06$ ($\pm 0.04$) | $0.18 \pm 0.03$ ($\pm 0.02$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.38 \pm 0.04$ ($\pm 0.03$) | $10.61 \pm 0.60$ ($\pm 0.40$) | $0.37 \pm 0.04$ ($\pm 0.03$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $4.55 \pm 4.67$ ($\pm 3.14$) | $33.78 \pm 16.76$ ($\pm 11.26$) | $4.58 \pm 4.66$ ($\pm 3.13$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $24.10 \pm 21.97$ ($\pm 14.76$) | $135.42 \pm 62.60$ ($\pm 42.05$) | $24.12 \pm 21.93$ ($\pm 14.73$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.62 \pm 0.09$ ($\pm 0.11$) | $16.33 \pm 2.21$ ($\pm 2.74$) | $0.61 \pm 0.10$ ($\pm 0.12$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.12 \pm 0.40$ ($\pm 0.49$) | $16.87 \pm 0.08$ ($\pm 0.09$) | $1.15 \pm 0.37$ ($\pm 0.46$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $16.46 \pm 20.99$ ($\pm 26.06$) | $60.77 \pm 3.52$ ($\pm 4.37$) | $16.14 \pm 20.08$ ($\pm 24.93$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $41.95 \pm 13.73$ ($\pm 17.05$) | $568.18 \pm 601.51$ ($\pm 746.75$) | $39.97 \pm 14.83$ ($\pm 18.42$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | **0.05** (1 / 0) | 0.05 (1 / 0) | 0.06 (1) | 0.57 (4) | 0.078 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.46** (2 / 0) | 0.56 (12 / 0) | 0.55 (12) | 0.79 (34) | 0.087 | 4601.00 | 4257.00 |
| `t2_mad_g2_i8` | **0.11** (1 / 0) | 0.66 (9 / 0) | 0.58 (9) | 0.33 (4) | 0.092 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.12** (1 / 0) | 0.14 (3 / 0) | 0.13 (3) | 2.39 (14) | 0.239 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.27** (1 / 0) | 1.99 (20 / 0) | 1.66 (20) | 5.28 (23) | 0.352 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **2.64** (21 / 0) | 2.59 (125 / 0) | 3.34 (125) | 6.11 (182) | 0.144 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **15.01** (220 / 0) | 35.86 (191 / 0) | 36.41 (191) | 123.71 (975) | 0.270 | 7720.00 | 6060.00 |
| `t3_mad_g5_i20` | **0.45** (1 / 0) | 1.97 (10 / 0) | 1.66 (10) | 0.50 (1) | 0.635 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.77** (1 / 0) | 6.09 (44 / 0) | 5.52 (44) | 13.25 (74) | 1.518 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **6.97** (2 / 0) | 23.21 (25 / 0) | 20.15 (25) | 250.58 (145, `TimeLimit`) | 2.795 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **32.32** (1 / 0) | 32.53 (2 / 0) | 30.97 (2) | 241.32 (49) | 57.713 | 208000.00 | 208000.00 |
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
| `t1_std_lp` | 3 | 3 | **737.1x** [431.1x-982.8x] | 0.01 | 5.85 | 0.00 | 5.80 | 0.003 | 0.061 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **1010.4x** [703.3x-1127.9x] | 0.01 | 5.84 | 0.00 | 5.79 | 0.003 | 0.061 | `OK` |
| `t1_std_mip` | 3 | 3 | **436.5x** [245.0x-705.3x] | 0.01 | 6.85 | 0.01 | 6.80 | 0.006 | 0.062 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **590.7x** [234.6x-728.0x] | 0.01 | 6.86 | 0.01 | 6.81 | 0.005 | 0.063 | `OK` |
| `t1_whiskas` | 6 | 5 | **530.9x** [407.0x-612.1x] | 0.01 | 5.90 | 0.00 | 5.83 | 0.006 | 0.087 | `OK` |
| `t1_furniture` | 2 | 3 | **319.6x** [81.8x-977.7x] | 0.02 | 7.09 | 0.01 | 7.04 | 0.003 | 0.059 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **107.5x** [42.0x-289.8x] | 0.08 | 8.27 | 0.07 | 8.20 | 0.009 | 0.097 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **1893.0x** [1110.0x-2309.7x] | 0.00 | 5.76 | 0.00 | 5.72 | 0.002 | 0.050 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **95.3x** [64.6x-119.2x] | 0.09 | 8.71 | 0.05 | 8.51 | 0.033 | 0.252 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **62.8x** [37.1x-94.3x] | 0.49 | 31.87 | 0.46 | 31.67 | 0.027 | 0.257 | `OK` |
| `t2_mkp_30` | 30 | 6 | **7.6x** [4.1x-11.8x] | 2.52 | 26.03 | 2.49 | 25.90 | 0.021 | 0.185 | `OK` |
| `t2_mkp_50` | 50 | 8 | **3.9x** [1.6x-28.4x] | 6.36 | 15.48 | 6.33 | 15.26 | 0.034 | 0.299 | `OK` |
| `t2_cflp_36` | 36 | 11 | **52.0x** [23.4x-141.1x] | 0.20 | 8.31 | 0.15 | 8.17 | 0.019 | 0.191 | `OK` |
| `t2_cflp_49` | 49 | 13 | **35.7x** [21.6x-103.9x] | 0.32 | 9.20 | 0.29 | 9.01 | 0.025 | 0.234 | `OK` |
| `t2_gap_45` | 45 | 13 | **18.4x** [2.9x-69.6x] | 0.94 | 13.36 | 0.92 | 13.18 | 0.029 | 0.223 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **12.9x** [5.5x-30.0x] | 2.97 | 65.86 | 2.95 | 65.66 | 0.028 | 0.268 | `OK` |
| `t2_binpack_44` | 44 | 14 | **10.5x** [0.1x-39.5x] | 0.55 | 12.59 | 0.50 | 12.41 | 0.032 | 0.220 | `OK` |
| `t2_setcover_50` | 50 | 20 | **41.4x** [5.8x-105.2x] | 0.19 | 10.07 | 0.13 | 9.80 | 0.043 | 0.354 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **56.9x** [53.4x-63.9x] | 0.15 | 8.46 | 0.11 | 8.20 | 0.036 | 0.332 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **48.3x** [27.9x-60.5x] | 0.18 | 9.10 | 0.12 | 8.72 | 0.051 | 0.488 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **28.1x** [23.2x-35.0x] | 0.38 | 10.33 | 0.27 | 9.77 | 0.099 | 0.733 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.7x** [2.3x-28.3x] | 2.68 | 45.47 | 2.64 | 45.22 | 0.038 | 0.341 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **6.8x** [1.4x-26.2x] | 15.07 | 164.12 | 15.01 | 163.71 | 0.060 | 0.536 | `OK` |
| `t3_mkp_200` | 200 | 8 | **2.7x** [2.2x-3.3x] | 36.94 | 112.10 | 36.86 | 111.51 | 0.089 | 0.775 | `OK` |
| `t3_cflp_300` | 300 | 34 | **34.3x** [7.8x-77.0x] | 5.96 | 243.03 | 5.82 | 242.13 | 0.121 | 1.181 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **3.2x** [2.3x-5.1x] | 61.67 | 153.33 | 61.44 | 151.53 | 0.218 | 2.243 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **10.7x** [5.3x-15.3x] | 7.05 | 77.45 | 6.57 | 74.62 | 0.346 | 3.403 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **26.4x** [17.5x-30.8x] | 0.61 | 16.55 | 0.45 | 15.51 | 0.143 | 1.269 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **15.7x** [9.3x-20.6x] | 1.00 | 16.83 | 0.77 | 15.17 | 0.244 | 2.170 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.8x** [1.1x-14.6x] | 8.08 | 58.85 | 6.97 | 51.09 | 1.286 | 10.632 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **9.9x** [4.5x-24.7x] | 36.99 | 362.47 | 32.32 | 329.87 | 4.584 | 42.721 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## 5. Default Build vs. `GOEXPERIMENT=simd`

In [`internal/simd/kernels_amd64.go`](../internal/simd/kernels_amd64.go) and [`internal/simd/kernels_avx512_amd64.go`](../internal/simd/kernels_avx512_amd64.go), runtime CPUID dispatch selects AVX-512, AVX2, or 4-way scalar loops from a single portable `CGO_ENABLED=0` binary (`GOAMD64=v3` FMA-safe). Each vector kernel spills accumulators via `StoreArray` and executes `archsimd.ClearAVXUpperBits()` (`VZEROUPPER`) before scalar tail or caller operations, eliminating AVX-to-SSE transition stalls while preserving IEEE-754 `NaN` propagation parity. AVX2 activates at `minAVX2Len = 32` (`Dot64`, `Axpy64`, `Scale64`, `AbsMax64`, `RowActivityBoundsFinite`, `SelectDualLeaveRow64With`, `MinReducedSkip64With`, `InitReducedRow64`, `AllAbsLE64With`, `ValidateAndNegate64With`), `rows >= 8 && cols >= 8` for `Transpose64With`, `minDualRatioSIMDChunks = 4` (256 cols for `ScanDualRatioCand64With`), and `minAVX2Min2Len = 256` (`Min2ReducedRow64`); on AVX-512 hosts, dense LSAP loops dispatch AVX-512 at `minAVX512Len = 64` and `minAVX512Min2Len = 128` while sparse LP kernels gate AVX-512 at `minAVX512LPLen = 2048` / `minLeaveRowAVX512 = 2048` / `minDualRatioAVX512Chunks = 128` (8,192 cols) to avoid Intel Emerald Rapids License-2 clock down-binning across surrounding scalar LU code.

At the full-solver level, each ratio below is the geometric mean of per-trial `scalar / SIMD` solve times over the same instances on the local AVX2 host (`i9-13900K`), with a 95% $t$-interval on the log ratios; "no detectable effect" means the interval contains 1. "Reachable" is a necessary condition: the AVX2 LP kernels require `n + m >= 32`. Below that size no vector kernel can run, so those small models serve as negative controls where intervals excluding 1 reflect binary code-layout or run-to-run variance rather than SIMD execution. Per-family paired speedups and 95% intervals for larger models appear in the generated table below.

### 5.1 Scope and Pending Target-Hardware Verification

All telemetry in this document is measured locally on the AVX2-only `13th Gen Intel(R) Core(TM) i9-13900K` host (`x86_64`, Linux). Portable `GOOS=linux GOARCH=arm64 CGO_ENABLED=0` and `GOAMD64=v3`/`v4` builds compile and pass local build checks, whereas fresh physical AVX-512 and `linux/arm64` runtime test and benchmark re-verification on the current tree remain pending target hardware access.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 1 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 3 faster, 1 slower, 19 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 1.08x [0.89x-1.32x] | 0.003 | 0.003 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 0.94x [0.82x-1.07x] | 0.002 | 0.002 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 1.20x [0.77x-1.89x] | 0.006 | 0.005 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source | 0.99x [0.95x-1.02x] | 0.005 | 0.005 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 0.95x [0.85x-1.07x] | 0.004 | 0.004 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 0.96x [0.83x-1.11x] | 0.012 | 0.012 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source | 1.04x [0.90x-1.21x] | 0.075 | 0.077 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.91x [0.86x-0.97x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 1.01x [0.83x-1.23x] | 0.045 | 0.048 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 0.98x [0.94x-1.03x] | 0.463 | 0.448 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 1.00x [0.98x-1.01x] | 2.492 | 2.580 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 0.94x [0.84x-1.06x] | 6.330 | 6.499 | 11 |
| `t2_cflp_36` | 47 | possible | no detectable effect | 0.97x [0.83x-1.13x] | 0.148 | 0.162 | 11 |
| `t2_cflp_49` | 62 | possible | no detectable effect | 1.03x [0.98x-1.07x] | 0.288 | 0.268 | 11 |
| `t2_gap_45` | 58 | possible | SIMD slower | 0.97x [0.95x-0.99x] | 0.916 | 0.922 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 0.99x [0.97x-1.00x] | 2.945 | 2.939 | 11 |
| `t2_binpack_44` | 58 | possible | no detectable effect | 0.98x [0.94x-1.04x] | 0.501 | 0.500 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 1.00x [0.91x-1.10x] | 0.128 | 0.156 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 1.02x [0.99x-1.05x] | 0.115 | 0.110 | 11 |
| `t2_mad_g3_i9` | 108 | possible | SIMD faster | 1.07x [1.02x-1.11x] | 0.118 | 0.108 | 11 |
| `t2_mad_g4_i12` | 156 | possible | no detectable effect | 1.02x [0.99x-1.04x] | 0.269 | 0.267 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 0.99x [0.98x-1.00x] | 2.641 | 2.623 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 0.99x [0.96x-1.01x] | 15.008 | 15.050 | 11 |
| `t3_mkp_200` | 208 | possible | SIMD faster | 1.06x [1.02x-1.10x] | 36.860 | 35.486 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.01x [0.97x-1.07x] | 5.818 | 5.880 | 5 |
| `t3_lotsizing_504` | 840 | possible | no detectable effect | 1.04x [0.99x-1.10x] | 61.445 | 60.503 | 5 |
| `t3_sched_1000` | 1100 | possible | SIMD faster | 1.07x [1.01x-1.14x] | 6.568 | 5.885 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 1.06x [0.95x-1.17x] | 0.449 | 0.397 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 0.99x [0.94x-1.04x] | 0.771 | 0.761 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 0.99x [0.94x-1.04x] | 6.968 | 6.898 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | no detectable effect | 1.08x [0.97x-1.21x] | 32.322 | 28.698 | 5 |
<!-- END GENERATED:SIMD_EFFECT_TABLE -->

---

## 6. Full Corpus Solver-Core Checkpoint Progression

<!-- BEGIN GENERATED:FULL_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_std_lp` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 54.00 | 54.00 |
| `t1_std_lp_max` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.001 | 80.00 | 80.00 |
| `t1_std_mip` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.002 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.01** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (3) | 0.002 | 64.95 | 58.90 |
| `t1_whiskas` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.003 | 0.52 | 0.52 |
| `t1_furniture` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.002 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.07** (1 / 8) | 0.10 (2 / 8) | 0.14 (24) | 0.22 (35) | 0.006 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.05** (1 / 0) | 0.05 (1 / 0) | 0.06 (1) | 0.57 (4) | 0.078 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.46** (2 / 0) | 0.56 (12 / 0) | 0.55 (12) | 0.79 (34) | 0.087 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.49** (11 / 9) | 2.67 (48 / 9) | 1.35 (77) | 1.15 (77) | 0.021 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **6.33** (1 / 4) | 15.83 (2 / 4) | 7.08 (132) | 6.24 (119) | 0.038 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.15** (1 / 2) | 0.21 (3 / 2) | 0.18 (7) | 0.17 (7) | 0.023 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.29** (1 / 10) | 0.55 (4 / 10) | 0.38 (9) | 0.37 (9) | 0.029 | 909.02 | 827.30 |
| `t2_gap_45` | **0.92** (1 / 9) | 0.35 (1 / 9) | 1.87 (61) | 2.12 (61) | 0.027 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **2.95** (2 / 37) | 6.59 (28 / 37) | 2.84 (187) | 3.25 (199) | 0.057 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **0.50** (2 / 8) | 0.95 (5 / 8) | 22.07 (1424) | 35.38 (2375) | 0.024 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.13** (1 / 6) | 0.33 (2 / 6) | 0.65 (6) | 0.63 (6) | 0.028 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.11** (1 / 0) | 0.66 (9 / 0) | 0.58 (9) | 0.33 (4) | 0.092 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.12** (1 / 0) | 0.14 (3 / 0) | 0.13 (3) | 2.39 (14) | 0.239 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.27** (1 / 0) | 1.99 (20 / 0) | 1.66 (20) | 5.28 (23) | 0.352 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **2.64** (21 / 0) | 2.59 (125 / 0) | 3.34 (125) | 6.11 (182) | 0.144 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **15.01** (220 / 0) | 35.86 (191 / 0) | 36.41 (191) | 123.71 (975) | 0.270 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **36.86** (448 / 16) | 82.85 (1144 / 16) | 178.80 (5000, `NodeLimit`) | 165.76 (5000, `NodeLimit`) | 0.112 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **5.82** (1 / 28) | 17.09 (46 / 34) | 9.16 (134) | 9.86 (134) | 0.177 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **61.44** (2 / 206) | 80.97 (376 / 206) | 250.02 (3412, `TimeLimit`) | 250.01 (3573, `TimeLimit`) | 0.711 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **6.57** (32 / 55) | 20.96 (87 / 53) | 250.08 (2222, `TimeLimit`) | 250.01 (2235, `TimeLimit`) | 0.195 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.45** (1 / 0) | 1.97 (10 / 0) | 1.66 (10) | 0.50 (1) | 0.635 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.77** (1 / 0) | 6.09 (44 / 0) | 5.52 (44) | 13.25 (74) | 1.518 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **6.97** (2 / 0) | 23.21 (25 / 0) | 20.15 (25) | 250.58 (145, `TimeLimit`) | 2.795 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **32.32** (1 / 0) | 32.53 (2 / 0) | 30.97 (2) | 241.32 (49) | 57.713 | 208000.00 | 208000.00 |
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
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.94x** [1.72x-2.18x] | 1.94x | 5.95x | 0.174 (0.0140) | 0.317 (0.0286) | 2.01x [0.59x-5.76x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **3.39x** [2.09x-5.48x] | 3.39x | 3.29x | 8.254 (0.713) | 32.506 (2.930) | 3.04x [1.18x-19.93x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **3.21x** [2.16x-4.76x] | 3.21x | 3.80x | 196.472 (38.600) | 905.234 (180.241) | 3.07x [1.55x-6.37x] |
| Square families ($m = n$) | 16 (146) | **3.09x** [2.28x-4.18x] | 3.04x | 4.18x | 201.096 (38.654) | 927.270 (181.260) | 2.75x [0.59x-19.93x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **2.33x** [1.78x-3.04x] | 2.20x | 3.72x | 3.804 (0.672) | 10.788 (1.940) | 2.37x [1.18x-3.94x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.60x** [1.27x-5.32x] | 2.75x | 3.19x | 33.230 (5.889) | 64.481 (10.902) | 2.07x [0.59x-19.93x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.86x** [2.28x-3.59x] | 2.79x | 4.05x | 204.900 (39.326) | 938.058 (183.200) | 2.54x [0.59x-19.93x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 7.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **2.31x** | 0.0004 | 0.0009 | 0.0005 | 0.0117 | $0.0004 \pm 0.0001$ | $0.0010 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.94x** | 0.0013 | 0.0024 | 0.0014 | 0.0103 | $0.0013 \pm 0.0007$ | $0.0024 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.99x** | 0.0024 | 0.0062 | 0.0036 | 0.0153 | $0.0036 \pm 0.0027$ | $0.0065 \pm 0.0027$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.70x** | 0.0023 | 0.0039 | 0.0030 | 0.0141 | $0.0023 \pm 0.0006$ | $0.0039 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.73x** | 0.0025 | 0.0044 | 0.0032 | 0.0149 | $0.0026 \pm 0.0007$ | $0.0044 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **2.01x** | 0.0051 | 0.0109 | 0.0078 | 0.0321 | $0.0055 \pm 0.0020$ | $0.0106 \pm 0.0020$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **3.07x** | 0.0231 | 0.0700 | 0.0311 | 0.0958 | $0.0229 \pm 0.0059$ | $0.0687 \pm 0.0103$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.75x** | 0.0479 | 0.175 | 0.0634 | 0.224 | $0.0466 \pm 0.0042$ | $0.176 \pm 0.0262$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.15x** | 0.0423 | 0.0935 | 0.0648 | 0.141 | $0.0461 \pm 0.0141$ | $0.0961 \pm 0.0137$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.71x** | 0.128 | 0.217 | 0.135 | 0.254 | $0.139 \pm 0.0443$ | $0.232 \pm 0.0464$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **14.87x** | 0.0495 | 0.683 | 0.0680 | 0.757 | $0.0467 \pm 0.0074$ | $0.686 \pm 0.0190$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **2.53x** | 0.0275 | 0.0692 | 0.0514 | 0.143 | $0.0297 \pm 0.0118$ | $0.0726 \pm 0.0123$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.39x** | 0.0313 | 0.0792 | 0.0555 | 0.153 | $0.0346 \pm 0.0107$ | $0.0804 \pm 0.0091$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.72x** | 0.0758 | 0.279 | 0.102 | 0.358 | $0.0766 \pm 0.0120$ | $0.284 \pm 0.0333$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **4.13x** | 0.287 | 1.264 | 0.421 | 1.653 | $0.308 \pm 0.0600$ | $1.259 \pm 0.118$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.93x** | 1.145 | 5.195 | 1.815 | 7.545 | $1.396 \pm 0.435$ | $5.311 \pm 0.352$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **2.12x** | 0.717 | 1.480 | 1.295 | 3.891 | $0.699 \pm 0.0394$ | $1.481 \pm 0.0519$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.72x** | 4.945 | 8.411 | 5.126 | 9.151 | $5.418 \pm 1.981$ | $9.145 \pm 2.660$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **3.19x** | 0.269 | 0.827 | 0.638 | 3.496 | $0.263 \pm 0.0202$ | $0.839 \pm 0.0603$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.79x** | 0.340 | 0.956 | 0.728 | 3.628 | $0.345 \pm 0.0137$ | $0.964 \pm 0.0333$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **5.17x** | 5.486 | 27.668 | 8.201 | 34.881 | $5.453 \pm 0.309$ | $28.213 \pm 2.282$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **5.28x** | 25.699 | 135.704 | 32.419 | 178.710 | $25.721 \pm 3.652$ | $135.094 \pm 9.387$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 7.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSkip64`, `InitReducedRow64`, and `Min2ReducedRow64`)

In [`assign/solver.go`](../assign/solver.go), [`simd.AllAbsLE64With`](../internal/simd/kernels_amd64.go) and [`simd.ValidateAndNegate64With`](../internal/simd/assign_amd64.go) validate finite cost bounds via AVX2 checks whenever `len(cost) >= 32` (while the fused `colReduceValidateSquare` fast path skips `AllAbsLE64With` on finite square `Minimize` matrices and dispatches `ColReduceValidateRows64With` only for $n \ge 32$), [`simd.Transpose64With`](../internal/simd/assign_amd64.go) handles tall transposes with $\text{rows} \ge 8$ and $\text{cols} \ge 8$, [`simd.MinReducedSkip64With`](../internal/simd/assign_amd64.go) accelerates reduction-transfer slack scans in `preprocessSquareJV` ($n \ge 32$), [`simd.Min2ReducedRow64With`](../internal/simd/kernels_amd64.go) accelerates first/second reduced-minimum scans in `preprocessRectangularBV` and `augmentingRowReduction` at `minAVX2Min2Len = 256` (`minAVX512Min2Len = 128`), and [`simd.InitReducedRow64With`](../internal/simd/kernels_amd64.go) runs on a slice of length $\max(m, n) \ge 32$ at the start of each Dijkstra augmentation. All vector kernels clean upper `YMM` state via `StoreArray` + `ClearAVXUpperBits()` (`VZEROUPPER`) before scalar operations; exact per-family verdicts appear in the generated table below.

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` and `rows * cols < 256`, no AVX2 kernel runs): 0 of 2 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32` or `AllAbsLE64` at `rows * cols >= 256`): 15 faster, 0 slower, 5 with no detectable effect, out of 20.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source | 1.01x [0.87x-1.16x] | 0.0004 | 0.0004 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source | 1.15x [0.85x-1.55x] | 0.0013 | 0.0011 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | possible | no detectable effect | 0.92x [0.56x-1.51x] | 0.0024 | 0.0037 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | no detectable effect | 1.00x [0.76x-1.31x] | 0.0023 | 0.0024 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | no detectable effect | 0.98x [0.69x-1.41x] | 0.0025 | 0.0026 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | no detectable effect | 0.98x [0.70x-1.36x] | 0.0051 | 0.0057 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.25x [1.16x-1.35x] | 0.0231 | 0.0195 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.23x [1.05x-1.45x] | 0.0479 | 0.0364 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.30x [1.06x-1.59x] | 0.0423 | 0.0290 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.76x [1.70x-1.81x] | 0.128 | 0.0742 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.29x [1.20x-1.40x] | 0.0495 | 0.0359 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.39x [1.15x-1.67x] | 0.0275 | 0.0208 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.39x [1.20x-1.63x] | 0.0313 | 0.0248 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | no detectable effect | 1.13x [0.58x-2.22x] | 0.0758 | 0.0498 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.57x [1.49x-1.66x] | 0.287 | 0.180 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.95x [1.38x-2.73x] | 1.145 | 0.686 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.01x [1.81x-2.24x] | 0.717 | 0.354 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.48x [2.22x-2.78x] | 4.945 | 1.832 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.83x [1.50x-2.24x] | 0.269 | 0.145 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.65x [1.43x-1.90x] | 0.340 | 0.202 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.72x [1.65x-1.79x] | 5.486 | 3.253 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.59x [1.51x-1.67x] | 25.699 | 15.463 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

---

## 8. Keeping Documentation and Benchmarks Synchronized

1. **Regenerate telemetry** from a clean, committed tree in a single pinned session so that all seven telemetry files record identical `git_commit`, `source_fingerprint`, and `uname_release`: run the commands in the [README](../README.md#verification-benchmarking-and-documentation-sync-commands).
2. **Regenerate tables**: `go run ./cmd/goblin-bench -sync-docs`.
3. **Verify**: `go run ./cmd/goblin-bench -check-docs`, or `go test ./...`, which runs `TestDocumentationAndBenchmarkSync`. The gate validates corpus dimensions, parity, symbol identifiers, and provenance, and checks every generated block in `README.md`, `docs/benchmarks.md`, and `bench/results/final_report.md`.
