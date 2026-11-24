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
- **Unified telemetry source**: fingerprint `b47f24a9055c678c` (working tree on top of commit `33f8c6a4005da0c924bbebb77666f4092185d807`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.53 ms median on a 1-variable MILP and 6.28 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
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
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **407.29x** [174.09x-952.90x] | 407.29x | 0.04x | 9.5 (0.8) | 196.7 (19.5) | 1074.1 (101.2) | 690.29x [37.43x-2334.72x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **17.17x** [9.19x-32.08x] | 17.17x | 0.17x | 739.5 (45.5) | 1986.7 (174.2) | 4384.3 (421.5) | 24.09x [0.13x-148.30x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **10.13x** [5.24x-19.61x] | 10.13x | 0.21x | 800.4 (143.7) | 2942.1 (566.1) | 6292.1 (1085.2) | 10.01x [1.15x-77.02x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **11.40x** [6.44x-20.19x] | 12.14x | 0.22x | 1223.2 (143.3) | 3164.0 (400.9) | 6890.4 (1000.4) | 12.01x [0.13x-148.30x] |
| Tier 3 non-MAD MILPs | 4 (20) | **8.11x** [1.79x-36.70x] | 8.11x | 0.27x | 492.4 (98.6) | 1266.0 (234.3) | 2855.3 (610.2) | 6.34x [1.87x-77.02x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **25.72x** [11.90x-55.62x] | 33.56x | 0.11x | 317.8 (45.9) | 1789.7 (341.9) | 3889.3 (515.8) | 36.23x [1.15x-119.14x] |
| Random-weight MAD (negative controls) | 3 (33) | **15.69x** [0.97x-254.92x] | 15.69x | 0.15x | 303.5 (21.0) | 1232.0 (118.9) | 2296.6 (251.7) | 16.07x [1.30x-95.37x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **49.20x** [19.60x-123.49x] | 59.43x | 0.11x | 1231.6 (144.0) | 3335.8 (417.9) | 7861.3 (1092.1) | 44.08x [0.13x-2334.72x] |
| All 31 synthetic corpus families | 31 (293) | **41.62x** [20.75x-83.49x] | 52.46x | 0.11x | 1549.3 (189.9) | 5125.5 (759.8) | 11750.5 (1607.9) | 40.78x [0.13x-2334.72x] |
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
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **95.4x** [51.5x-119.1x] | 0.09 | 9.43 | 0.06 | 9.22 | 0.035 | 0.210 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **56.8x** [37.4x-95.4x] | 0.54 | 34.38 | 0.49 | 34.15 | 0.046 | 0.240 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **46.6x** [35.1x-70.4x] | 0.22 | 9.66 | 0.16 | 9.36 | 0.046 | 0.289 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **55.0x** [42.6x-71.2x] | 0.18 | 10.28 | 0.13 | 9.86 | 0.047 | 0.411 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **30.5x** [23.0x-35.7x] | 0.37 | 11.47 | 0.29 | 10.84 | 0.075 | 0.622 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **9.5x** [3.4x-25.1x] | 3.57 | 48.22 | 3.50 | 47.93 | 0.042 | 0.309 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **7.2x** [1.3x-29.3x] | 16.87 | 169.12 | 16.80 | 168.67 | 0.059 | 0.443 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **26.7x** [18.2x-34.0x] | 0.66 | 18.51 | 0.49 | 17.38 | 0.162 | 1.095 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **16.2x** [10.1x-21.6x] | 1.03 | 19.00 | 0.84 | 17.07 | 0.209 | 1.813 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **5.8x** [1.1x-13.4x] | 8.55 | 62.43 | 7.49 | 54.04 | 1.155 | 8.684 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **10.1x** [4.4x-24.4x] | 34.81 | 375.00 | 30.16 | 339.86 | 4.391 | 35.748 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Cross-Instance Latency Dispersion Across Randomized Instances (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.10 \pm 0.03$ ($\pm 0.02$) | $9.39 \pm 0.51$ ($\pm 0.34$) | $0.08 \pm 0.02$ ($\pm 0.01$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.58 \pm 0.25$ ($\pm 0.17$) | $30.96 \pm 9.56$ ($\pm 6.42$) | $0.50 \pm 0.23$ ($\pm 0.15$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.21 \pm 0.04$ ($\pm 0.03$) | $9.75 \pm 0.84$ ($\pm 0.57$) | $0.16 \pm 0.01$ ($\pm 0.01$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.19 \pm 0.03$ ($\pm 0.02$) | $10.25 \pm 0.25$ ($\pm 0.17$) | $0.21 \pm 0.05$ ($\pm 0.03$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.39 \pm 0.05$ ($\pm 0.03$) | $11.74 \pm 0.72$ ($\pm 0.48$) | $0.37 \pm 0.03$ ($\pm 0.02$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $4.25 \pm 3.10$ ($\pm 2.08$) | $35.95 \pm 17.26$ ($\pm 11.59$) | $4.24 \pm 3.14$ ($\pm 2.11$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $22.76 \pm 22.45$ ($\pm 15.08$) | $141.87 \pm 65.38$ ($\pm 43.92$) | $22.45 \pm 22.26$ ($\pm 14.96$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.66 \pm 0.08$ ($\pm 0.09$) | $17.74 \pm 2.32$ ($\pm 2.88$) | $0.55 \pm 0.02$ ($\pm 0.02$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.22 \pm 0.40$ ($\pm 0.50$) | $19.04 \pm 0.29$ ($\pm 0.36$) | $1.24 \pm 0.37$ ($\pm 0.45$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $16.93 \pm 21.03$ ($\pm 26.10$) | $64.38 \pm 3.43$ ($\pm 4.25$) | $16.80 \pm 20.79$ ($\pm 25.81$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $42.79 \pm 14.96$ ($\pm 18.57$) | $586.19 \pm 617.63$ ($\pm 766.76$) | $40.36 \pm 17.04$ ($\pm 21.16$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | **0.06** (1 / 0) | 0.05 (1 / 0) | 0.06 (1) | 0.55 (4) | 0.089 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.49** (2 / 0) | 0.57 (12 / 0) | 0.57 (12) | 0.82 (109) | 0.112 | 4601.00 | 4257.00 |
| `t2_mad_g2_i8` | **0.16** (1 / 0) | 0.76 (8 / 0) | 0.75 (8) | 0.75 (9) | 0.103 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.13** (1 / 0) | 0.15 (2 / 0) | 0.15 (2) | 2.88 (13) | 0.242 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.29** (1 / 0) | 3.26 (22 / 0) | 3.65 (22) | 5.26 (23) | 0.356 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.50** (43 / 0) | 2.93 (112 / 0) | 2.91 (112) | 3.91 (45) | 0.164 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **16.80** (196 / 0) | 30.64 (188 / 0) | 30.69 (188) | 92.82 (539) | 0.274 | 7720.00 | 6060.00 |
| `t3_mad_g5_i20` | **0.49** (1 / 0) | 2.99 (17 / 0) | 2.60 (17) | 0.52 (1) | 0.769 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.84** (1 / 0) | 8.33 (58 / 0) | 7.47 (58) | 13.51 (74) | 1.564 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.49** (2 / 0) | 22.82 (25 / 0) | 19.44 (25) | 250.24 (145, `TimeLimit`) | 2.658 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **30.16** (1 / 0) | 33.10 (2 / 0) | 31.56 (2) | 250.45 (6, `TimeLimit`) | 58.527 | 208000.00 | 208000.00 |
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
| `t1_std_lp` | 3 | 3 | **934.7x** [672.1x-1195.5x] | 0.01 | 6.37 | 0.00 | 6.31 | 0.003 | 0.051 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **941.5x** [555.9x-1271.6x] | 0.01 | 6.27 | 0.00 | 6.22 | 0.003 | 0.048 | `OK` |
| `t1_std_mip` | 3 | 3 | **978.7x** [801.8x-1145.9x] | 0.01 | 7.36 | 0.00 | 7.31 | 0.003 | 0.057 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **1039.1x** [651.3x-1242.9x] | 0.01 | 7.58 | 0.00 | 7.52 | 0.003 | 0.065 | `OK` |
| `t1_whiskas` | 6 | 5 | **622.4x** [472.0x-723.8x] | 0.01 | 6.33 | 0.00 | 6.25 | 0.005 | 0.074 | `OK` |
| `t1_furniture` | 2 | 3 | **302.0x** [86.2x-1102.0x] | 0.02 | 7.78 | 0.02 | 7.72 | 0.003 | 0.051 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **100.9x** [50.1x-287.0x] | 0.11 | 9.34 | 0.10 | 9.25 | 0.010 | 0.089 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **1366.1x** [751.7x-2334.7x] | 0.00 | 6.33 | 0.00 | 6.29 | 0.004 | 0.046 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **95.4x** [51.5x-119.1x] | 0.09 | 9.43 | 0.06 | 9.22 | 0.035 | 0.210 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **56.8x** [37.4x-95.4x] | 0.54 | 34.38 | 0.49 | 34.15 | 0.046 | 0.240 | `OK` |
| `t2_mkp_30` | 30 | 6 | **11.4x** [6.4x-15.8x] | 2.17 | 27.95 | 2.14 | 27.76 | 0.020 | 0.179 | `OK` |
| `t2_mkp_50` | 50 | 8 | **5.8x** [3.1x-34.0x] | 4.39 | 17.07 | 4.36 | 16.80 | 0.033 | 0.265 | `OK` |
| `t2_cflp_36` | 36 | 11 | **56.2x** [17.6x-148.3x] | 0.15 | 9.22 | 0.14 | 9.04 | 0.017 | 0.166 | `OK` |
| `t2_cflp_49` | 49 | 13 | **31.0x** [9.3x-86.5x] | 0.33 | 10.05 | 0.30 | 9.86 | 0.029 | 0.205 | `OK` |
| `t2_gap_45` | 45 | 13 | **23.0x** [4.7x-80.8x] | 0.77 | 14.43 | 0.75 | 14.21 | 0.021 | 0.208 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **11.5x** [5.6x-25.9x] | 4.21 | 69.52 | 4.18 | 69.27 | 0.033 | 0.254 | `OK` |
| `t2_binpack_44` | 44 | 14 | **1.8x** [0.1x-25.4x] | 11.96 | 13.28 | 11.93 | 13.08 | 0.026 | 0.197 | `OK` |
| `t2_setcover_50` | 50 | 20 | **38.3x** [6.9x-125.1x] | 0.26 | 11.25 | 0.24 | 10.94 | 0.034 | 0.306 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **46.6x** [35.1x-70.4x] | 0.22 | 9.66 | 0.16 | 9.36 | 0.046 | 0.289 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **55.0x** [42.6x-71.2x] | 0.18 | 10.28 | 0.13 | 9.86 | 0.047 | 0.411 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **30.5x** [23.0x-35.7x] | 0.37 | 11.47 | 0.29 | 10.84 | 0.075 | 0.622 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.5x** [3.4x-25.1x] | 3.57 | 48.22 | 3.50 | 47.93 | 0.042 | 0.309 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **7.2x** [1.3x-29.3x] | 16.87 | 169.12 | 16.80 | 168.67 | 0.059 | 0.443 | `OK` |
| `t3_mkp_200` | 200 | 8 | **5.0x** [4.1x-6.7x] | 18.69 | 117.88 | 18.62 | 117.24 | 0.075 | 0.649 | `OK` |
| `t3_cflp_300` | 300 | 34 | **26.5x** [8.3x-77.0x] | 8.87 | 252.32 | 8.76 | 251.40 | 0.103 | 0.912 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **3.0x** [1.9x-4.8x] | 64.54 | 158.67 | 64.33 | 156.82 | 0.223 | 1.870 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **10.8x** [5.4x-17.7x] | 6.55 | 81.37 | 6.25 | 78.38 | 0.293 | 2.837 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **26.7x** [18.2x-34.0x] | 0.66 | 18.51 | 0.49 | 17.38 | 0.162 | 1.095 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **16.2x** [10.1x-21.6x] | 1.03 | 19.00 | 0.84 | 17.07 | 0.209 | 1.813 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.8x** [1.1x-13.4x] | 8.55 | 62.43 | 7.49 | 54.04 | 1.155 | 8.684 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **10.1x** [4.4x-24.4x] | 34.81 | 375.00 | 30.16 | 339.86 | 4.391 | 35.748 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## 5. Default Build vs. `GOEXPERIMENT=simd`

In [`internal/simd/kernels_amd64.go`](../internal/simd/kernels_amd64.go) and [`internal/simd/kernels_avx512_amd64.go`](../internal/simd/kernels_avx512_amd64.go), runtime CPUID dispatch selects AVX-512, AVX2, or 4-way scalar loops from a single portable `CGO_ENABLED=0` binary (`GOAMD64=v3` FMA-safe). Each vector kernel spills accumulators via `StoreArray` and executes `archsimd.ClearAVXUpperBits()` (`VZEROUPPER`) before scalar tail or caller operations, eliminating AVX-to-SSE transition stalls while preserving IEEE-754 `NaN` propagation parity. AVX2 activates at `minAVX2Len = 32` (`Dot64`, `Axpy64`, `Scale64`, `AbsMax64`, `RowActivityBoundsFinite`, `SelectDualLeaveRow64With`, `MinReducedSkip64With`, `InitReducedRow64`, `AllAbsLE64With`, `ValidateAndNegate64With`), `rows >= 8 && cols >= 8` for `Transpose64With`, `minDualRatioSIMDChunks = 4` (256 cols for `ScanDualRatioCand64With`), and `minAVX2Min2Len = 256` (`Min2ReducedRow64`); on AVX-512 hosts, dense LSAP loops dispatch AVX-512 at `minAVX512Len = 64` and `minAVX512Min2Len = 128` while sparse LP kernels gate AVX-512 at `minAVX512LPLen = 2048` / `minLeaveRowAVX512 = 2048` / `minDualRatioAVX512Chunks = 128` (8,192 cols) to avoid Intel Emerald Rapids License-2 clock down-binning across surrounding scalar LU code.

At the full-solver level, each ratio below is the geometric mean of per-trial `scalar / SIMD` solve times over the same instances on the local AVX2 host (`i9-13900K`), with a 95% $t$-interval on the log ratios; "no detectable effect" means the interval contains 1. "Reachable" is a necessary condition: the AVX2 LP kernels require `n + m >= 32`. Below that size no vector kernel can run, so those small models serve as negative controls where intervals excluding 1 reflect binary code-layout or run-to-run variance rather than SIMD execution. Per-family paired speedups and 95% intervals for larger models appear in the generated table below.

### 5.1 Scope and Pending Target-Hardware Verification

All telemetry in this document is measured locally on the AVX2-only `13th Gen Intel(R) Core(TM) i9-13900K` host (`x86_64`, Linux). Portable `GOOS=linux GOARCH=arm64 CGO_ENABLED=0` and `GOAMD64=v3`/`v4` builds compile and pass local build checks, whereas fresh physical AVX-512 and `linux/arm64` runtime test and benchmark re-verification on the current tree remain pending target hardware access.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 6 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 7 faster, 1 slower, 15 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.83x [0.75x-0.93x] | 0.003 | 0.004 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.78x [0.65x-0.94x] | 0.003 | 0.004 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.83x [0.75x-0.93x] | 0.004 | 0.005 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.89x [0.81x-0.98x] | 0.004 | 0.004 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.87x [0.78x-0.98x] | 0.005 | 0.005 | 11 |
| `t1_furniture` | 5 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.86x [0.76x-0.98x] | 0.015 | 0.019 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source | 0.92x [0.84x-1.01x] | 0.095 | 0.098 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 1.00x [0.97x-1.03x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 1.09x [0.90x-1.31x] | 0.056 | 0.049 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | SIMD faster | 1.14x [1.09x-1.18x] | 0.485 | 0.450 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 0.94x [0.81x-1.08x] | 2.142 | 2.241 | 11 |
| `t2_mkp_50` | 58 | possible | SIMD faster | 1.05x [1.00x-1.10x] | 4.361 | 4.313 | 11 |
| `t2_cflp_36` | 47 | possible | no detectable effect | 1.01x [0.99x-1.04x] | 0.138 | 0.141 | 11 |
| `t2_cflp_49` | 62 | possible | no detectable effect | 0.97x [0.78x-1.22x] | 0.296 | 0.376 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 1.00x [0.95x-1.04x] | 0.747 | 0.772 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 1.03x [0.99x-1.07x] | 4.181 | 4.107 | 11 |
| `t2_binpack_44` | 58 | possible | no detectable effect | 1.02x [0.98x-1.06x] | 11.931 | 11.858 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 1.05x [0.95x-1.17x] | 0.235 | 0.233 | 11 |
| `t2_mad_g2_i8` | 72 | possible | SIMD faster | 1.19x [1.07x-1.32x] | 0.159 | 0.127 | 11 |
| `t2_mad_g3_i9` | 108 | possible | SIMD slower | 0.90x [0.81x-1.00x] | 0.131 | 0.136 | 11 |
| `t2_mad_g4_i12` | 156 | possible | no detectable effect | 1.09x [0.99x-1.19x] | 0.293 | 0.280 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.02x [0.97x-1.07x] | 3.504 | 2.979 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 1.02x [1.00x-1.04x] | 16.804 | 16.604 | 11 |
| `t3_mkp_200` | 208 | possible | SIMD faster | 1.05x [1.03x-1.07x] | 18.617 | 18.103 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.03x [0.95x-1.12x] | 8.760 | 8.998 | 5 |
| `t3_lotsizing_504` | 840 | possible | SIMD faster | 1.04x [1.02x-1.07x] | 64.333 | 61.965 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 1.00x [0.83x-1.20x] | 6.246 | 7.040 | 5 |
| `t3_mad_g5_i20` | 240 | possible | SIMD faster | 1.17x [1.05x-1.30x] | 0.494 | 0.434 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 1.03x [0.77x-1.38x] | 0.840 | 0.810 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 1.02x [0.99x-1.04x] | 7.493 | 7.443 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | SIMD faster | 1.09x [1.01x-1.17x] | 30.156 | 27.113 | 5 |
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
| `t1_whiskas` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.003 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.02 (1) | 0.01 (1) | 0.002 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.10** (1 / 8) | 0.11 (2 / 8) | 0.14 (18) | 0.22 (35) | 0.007 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.06** (1 / 0) | 0.05 (1 / 0) | 0.06 (1) | 0.55 (4) | 0.089 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.49** (2 / 0) | 0.57 (12 / 0) | 0.57 (12) | 0.82 (109) | 0.112 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.14** (5 / 9) | 2.65 (40 / 9) | 1.05 (51) | 0.92 (61) | 0.025 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **4.36** (1 / 4) | 11.88 (2 / 4) | 4.84 (118) | 4.27 (111) | 0.039 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.14** (1 / 2) | 0.21 (3 / 2) | 0.17 (7) | 0.17 (7) | 0.025 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.30** (1 / 7) | 0.61 (3 / 7) | 0.30 (9) | 0.31 (9) | 0.033 | 909.02 | 827.30 |
| `t2_gap_45` | **0.75** (1 / 7) | 0.39 (1 / 7) | 1.74 (41) | 1.96 (60) | 0.025 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **4.18** (2 / 37) | 7.75 (25 / 37) | 3.05 (162) | 3.50 (199) | 0.068 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **11.93** (274 / 12) | 14.22 (298 / 12) | 8.60 (772) | 27.75 (1941) | 0.028 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.24** (1 / 6) | 0.42 (2 / 6) | 0.67 (6) | 0.64 (6) | 0.031 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.16** (1 / 0) | 0.76 (8 / 0) | 0.75 (8) | 0.75 (9) | 0.103 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.13** (1 / 0) | 0.15 (2 / 0) | 0.15 (2) | 2.88 (13) | 0.242 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.29** (1 / 0) | 3.26 (22 / 0) | 3.65 (22) | 5.26 (23) | 0.356 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.50** (43 / 0) | 2.93 (112 / 0) | 2.91 (112) | 3.91 (45) | 0.164 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **16.80** (196 / 0) | 30.64 (188 / 0) | 30.69 (188) | 92.82 (539) | 0.274 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **18.62** (250 / 14) | 84.59 (1135 / 16) | 169.34 (5000, `NodeLimit`) | 150.65 (5000, `NodeLimit`) | 0.117 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **8.76** (1 / 28) | 18.93 (54 / 34) | 8.82 (116) | 8.94 (118) | 0.187 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **64.33** (2 / 206) | 81.92 (376 / 206) | 250.06 (3625, `TimeLimit`) | 250.01 (3699, `TimeLimit`) | 0.725 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **6.25** (31 / 55) | 19.83 (87 / 53) | 250.02 (2555, `TimeLimit`) | 250.06 (2586, `TimeLimit`) | 0.194 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.49** (1 / 0) | 2.99 (17 / 0) | 2.60 (17) | 0.52 (1) | 0.769 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.84** (1 / 0) | 8.33 (58 / 0) | 7.47 (58) | 13.51 (74) | 1.564 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.49** (2 / 0) | 22.82 (25 / 0) | 19.44 (25) | 250.24 (145, `TimeLimit`) | 2.658 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **30.16** (1 / 0) | 33.10 (2 / 0) | 31.56 (2) | 250.45 (6, `TimeLimit`) | 58.527 | 208000.00 | 208000.00 |
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
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.48x** [1.18x-1.86x] | 1.48x | 4.49x | 0.217 (0.0198) | 0.318 (0.0286) | 1.50x [0.84x-2.65x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **2.98x** [1.89x-4.71x] | 2.98x | 2.83x | 9.242 (0.794) | 32.915 (2.980) | 2.66x [0.85x-15.17x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.87x** [1.87x-4.41x] | 2.87x | 3.60x | 216.804 (42.828) | 940.999 (186.774) | 2.59x [1.50x-6.16x] |
| Square families ($m = n$) | 16 (146) | **2.75x** [2.03x-3.73x] | 2.69x | 3.71x | 221.504 (42.811) | 963.195 (187.811) | 2.29x [0.84x-15.17x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.76x** [1.23x-2.52x] | 1.64x | 2.90x | 4.759 (0.832) | 11.036 (1.972) | 1.90x [0.85x-2.86x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.30x** [1.18x-4.48x] | 2.43x | 2.75x | 35.619 (6.153) | 65.175 (11.019) | 1.85x [0.84x-15.17x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.44x** [1.91x-3.11x] | 2.35x | 3.47x | 226.263 (43.642) | 974.231 (189.783) | 2.08x [0.84x-15.17x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 7.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.93x** | 0.0005 | 0.0009 | 0.0006 | 0.0122 | $0.0005 \pm 0.0001$ | $0.0009 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.41x** | 0.0016 | 0.0023 | 0.0019 | 0.0102 | $0.0016 \pm 0.0004$ | $0.0023 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.73x** | 0.0037 | 0.0062 | 0.0054 | 0.0157 | $0.0036 \pm 0.0008$ | $0.0063 \pm 0.0017$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.15x** | 0.0037 | 0.0039 | 0.0052 | 0.0141 | $0.0034 \pm 0.0007$ | $0.0039 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.17x** | 0.0036 | 0.0042 | 0.0054 | 0.0148 | $0.0038 \pm 0.0011$ | $0.0044 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.68x** | 0.0067 | 0.0111 | 0.0114 | 0.0315 | $0.0068 \pm 0.0022$ | $0.0111 \pm 0.0025$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.80x** | 0.0292 | 0.0740 | 0.0418 | 0.0995 | $0.0267 \pm 0.0064$ | $0.0737 \pm 0.0120$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.38x** | 0.0530 | 0.175 | 0.0848 | 0.225 | $0.0528 \pm 0.0058$ | $0.179 \pm 0.0238$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.18x** | 0.0480 | 0.107 | 0.0864 | 0.156 | $0.0464 \pm 0.0052$ | $0.102 \pm 0.0157$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.61x** | 0.139 | 0.219 | 0.182 | 0.255 | $0.147 \pm 0.0457$ | $0.232 \pm 0.0458$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **11.67x** | 0.0614 | 0.688 | 0.0829 | 0.763 | $0.0597 \pm 0.0089$ | $0.690 \pm 0.0208$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.92x** | 0.0370 | 0.0708 | 0.0632 | 0.146 | $0.0389 \pm 0.0098$ | $0.0739 \pm 0.0116$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **1.92x** | 0.0390 | 0.0794 | 0.0660 | 0.155 | $0.0430 \pm 0.0134$ | $0.0805 \pm 0.0074$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.45x** | 0.0796 | 0.282 | 0.103 | 0.359 | $0.0835 \pm 0.0179$ | $0.285 \pm 0.0341$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.79x** | 0.309 | 1.285 | 0.420 | 1.756 | $0.342 \pm 0.0739$ | $1.276 \pm 0.131$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.90x** | 1.223 | 5.279 | 1.892 | 7.826 | $1.393 \pm 0.264$ | $5.376 \pm 0.377$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.70x** | 0.879 | 1.491 | 1.443 | 4.191 | $0.882 \pm 0.0678$ | $1.498 \pm 0.0505$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.66x** | 5.016 | 8.497 | 5.537 | 10.084 | $5.662 \pm 2.033$ | $9.247 \pm 2.660$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.51x** | 0.345 | 0.833 | 0.713 | 3.625 | $0.336 \pm 0.0233$ | $0.845 \pm 0.0577$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.40x** | 0.404 | 0.981 | 0.804 | 3.763 | $0.419 \pm 0.0386$ | $1.004 \pm 0.0691$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.89x** | 6.031 | 28.257 | 7.675 | 37.236 | $5.892 \pm 0.385$ | $28.842 \pm 2.332$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.94x** | 28.931 | 141.436 | 38.242 | 183.557 | $28.777 \pm 3.730$ | $141.388 \pm 9.161$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 7.3 LSAP Scalar vs. `GOEXPERIMENT=simd`

In [`assign/solver.go`](../assign/solver.go), [`simd.AllAbsLE64With`](../internal/simd/kernels_amd64.go) and [`simd.ValidateAndNegate64With`](../internal/simd/assign_amd64.go) validate finite cost bounds via AVX2 checks whenever `len(cost) >= 32` (while the fused `colReduceValidateSquare` fast path skips `AllAbsLE64With` on finite square `Minimize` matrices and dispatches `ColReduceValidateRows64With` only for $n \ge 32$), [`simd.Transpose64With`](../internal/simd/assign_amd64.go) handles tall transposes with $\text{rows} \ge 8$ and $\text{cols} \ge 8$, [`simd.MinReducedSkip64With`](../internal/simd/assign_amd64.go) accelerates reduction-transfer slack scans in `preprocessSquareJV` ($n \ge 32$), [`simd.Min2ReducedRow64With`](../internal/simd/kernels_amd64.go) accelerates first/second reduced-minimum scans in `preprocessRectangularBV` and `augmentingRowReduction` at `minAVX2Min2Len = 256` (`minAVX512Min2Len = 128`), and [`simd.InitReducedRow64With`](../internal/simd/kernels_amd64.go) runs on a slice of length $\max(m, n) \ge 32$ at the start of each Dijkstra augmentation. All vector kernels clean upper `YMM` state via `StoreArray` + `ClearAVXUpperBits()` (`VZEROUPPER`) before scalar operations; exact per-family verdicts appear in the generated table below.

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` with no `>= 8x8` tall transpose and either fused square `Minimize` non-sparse path or `rows * cols < 32`, so no AVX2 kernel runs): 0 of 3 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32`, tall `rows > cols` with `>= 8x8`, or fallback `AllAbsLE64`/`ValidateAndNegate64` at `rows * cols >= 32`): 15 faster, 0 slower, 4 with no detectable effect, out of 19.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source | 0.65x [0.42x-1.01x] | 0.0005 | 0.0006 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source | 0.57x [0.32x-1.01x] | 0.0016 | 0.0020 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source | 0.94x [0.86x-1.02x] | 0.0037 | 0.0038 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | no detectable effect | 0.99x [0.85x-1.15x] | 0.0037 | 0.0035 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | no detectable effect | 1.03x [0.89x-1.20x] | 0.0036 | 0.0035 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | no detectable effect | 0.98x [0.95x-1.00x] | 0.0067 | 0.0063 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.25x [1.16x-1.35x] | 0.0292 | 0.0226 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.33x [1.25x-1.41x] | 0.0530 | 0.0403 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.53x [1.28x-1.82x] | 0.0480 | 0.0291 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.72x [1.64x-1.82x] | 0.139 | 0.0850 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.33x [1.23x-1.44x] | 0.0614 | 0.0468 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | no detectable effect | 1.36x [0.98x-1.89x] | 0.0370 | 0.0270 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.41x [1.22x-1.62x] | 0.0390 | 0.0292 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.37x [1.20x-1.56x] | 0.0796 | 0.0554 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.50x [1.24x-1.81x] | 0.309 | 0.195 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.88x [1.53x-2.31x] | 1.223 | 0.724 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.53x [2.14x-2.98x] | 0.879 | 0.352 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.54x [2.28x-2.82x] | 5.016 | 1.903 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.88x [1.57x-2.25x] | 0.345 | 0.179 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.47x [1.08x-2.00x] | 0.404 | 0.270 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.67x [1.56x-1.78x] | 6.031 | 3.722 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.46x [1.36x-1.56x] | 28.931 | 20.011 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

---

## 8. Production Binary Footprint and Linker Dead-Code Elimination

The `goblin-bench -binary-size` mode compiles four standalone `CGO_ENABLED=0` programs in an isolated temporary module against the live workspace across both default `go build` and `GOEXPERIMENT=simd`, saving compiled artifact sizes and `go tool nm -size` symbol counts to `bench/results/goblin_binary_size.json`. [`ValidateBinarySizeTelemetry`](../bench/docgen.go) rejects the telemetry bundle if `assign_default` links any `math/big`, `encoding/json`, `VIPR`, or `Certif` symbols, if `goblin_default` links any `encoding/json`, `VIPR`, or `Certif` symbols, or if any profile links `encoding/json`:

<!-- BEGIN GENERATED:BINARY_SIZE_TABLE -->
| Profile | Default Raw / Stripped (KiB) | `GOEXPERIMENT=simd` Raw / Stripped (KiB) | Stripped Delta vs. Default | `goblin` Text (KiB) | `math/big` Syms | `encoding/json` Syms | `VIPR` Syms | `Certif` Syms |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `assign` default (`LinearSum` + `SolveInto`) | 2786.7 / **1740.2** | 2816.6 / **1804.2** | baseline | 117.3 | `0` | `0` | `0` | `0` |
| `assign` certified (`+ assign.WithCertified(true)`) | 2906.9 / **1824.2** | 2940.0 / **1888.2** | +84.0 KiB (+4.8%) | 175.5 | `0` | `0` | `0` | `17` |
| `goblin` default (`Model.Solve`, `DefaultOptions`) | 5300.2 / **3372.2** | 5189.0 / **3388.2** | baseline | 893.0 | `130` | `0` | `0` | `0` |
| `goblin` certified + VIPR (`+ WithCertified + WithVIPRTrace`) | 6465.4 / **4160.2** | 6338.8 / **4180.2** | +788.0 KiB (+23.4%) | 1343.7 | `155` | `0` | `212` | `14` |
<!-- END GENERATED:BINARY_SIZE_TABLE -->

---

## 9. Pure-Go vs. In-Process C++ CBC Performance Analysis

Comparing isolated unpresolved `Root LP` timings, paired trial ratios, and solver-core checkpoints (`CP5 +Presolve`, `CP6 +Cuts`, `CP7 Full`) between `BackendGo` and in-process `BackendCBC` (`c1c171ec`) removes Python MPS formatting and OS process-spawn overhead across all 31 synthetic families and highlights four structural trade-offs on large degenerate LPs and heavy-tail branch-and-cut models:

<!-- BEGIN GENERATED:CBC_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `BackendCBC / BackendGo` Total [Clustered 95% CI] | Equal-Family `BackendCBC / BackendGo` Solve [Clustered 95% CI] | `BackendGo` Trial (Med) Sum (ms) | `BackendCBC` Trial (Med) Sum (ms) | Per-Instance `CBC / Go` Total Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro-models (`N <= 26`) | 10 (110) | **26.26x** [12.00x-57.48x] | **52.19x** [17.44x-156.15x] | 9.5 (0.8) | 196.7 (19.5) | 21.96x [4.54x-157.45x] |
| Tier 2: Medium MILPs (`30 <= N <= 84`) | 13 (143) | **5.76x** [3.58x-9.24x] | **6.38x** [3.78x-10.77x] | 739.5 (45.5) | 1986.7 (174.2) | 7.14x [0.08x-48.37x] |
| Tier 3: Large MILPs (`145 <= N <= 4036`) | 8 (40) | **4.84x** [2.76x-8.49x] | **5.30x** [2.87x-9.79x] | 800.4 (143.7) | 2942.1 (566.1) | 5.39x [0.64x-15.98x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **4.47x** [2.86x-7.00x] | **4.67x** [2.91x-7.50x] | 1223.2 (143.3) | 3164.0 (400.9) | 4.91x [0.08x-48.37x] |
| All 31 synthetic families | 31 (293) | **8.98x** [5.93x-13.61x] | **11.98x** [6.95x-20.64x] | 1549.3 (189.9) | 5125.5 (759.8) | 9.13x [0.08x-157.45x] |
<!-- END GENERATED:CBC_SUMMARY_TABLE -->

### 9.1 MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families ($N = G(I + T + 3)$ variables, $M = G(2T + 3) + I$ constraints) separate solver behavior into three scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Paired `Go / CBC` | Go Total Med (ms) | CBC Total Med (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.04x** | 0.09 | 2.47 | 0.68x | 0.06 (1 / 19) | 2.43 (0 / 0) | 0.089 (28) | 0.132 (19) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.05x** | 0.54 | 11.30 | 0.79x | 0.49 (2 / 72) | 11.26 (6 / 83) | 0.112 (30) | 0.142 (18) |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.10x** | 0.22 | 1.87 | 0.72x | 0.16 (1 / 26) | 1.83 (0 / 0) | 0.103 (33) | 0.143 (21) |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.07x** | 0.18 | 2.50 | 0.96x | 0.13 (1 / 40) | 2.44 (0 / 0) | 0.242 (58) | 0.253 (40) |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.11x** | 0.37 | 3.22 | 0.89x | 0.29 (1 / 51) | 3.14 (0 / 0) | 0.356 (80) | 0.401 (63) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.26x** | 3.57 | 16.83 | 1.06x | 3.50 (43 / 483) | 16.79 (12 / 216) | 0.164 (38) | 0.155 (24) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.25x** | 16.87 | 90.72 | 0.98x | 16.80 (196 / 2299) | 90.64 (198 / 1300) | 0.274 (65) | 0.279 (54) |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.08x** | 0.66 | 8.02 | 1.21x | 0.49 (1 / 75) | 7.90 (0 / 0) | 0.769 (91) | 0.635 (94) |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.15x** | 1.03 | 7.69 | 1.49x | 0.84 (1 / 98) | 7.49 (0 / 0) | 1.564 (244) | 1.049 (149) |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **0.30x** | 8.55 | 37.32 | 0.74x | 7.49 (2 / 377) | 36.04 (0 / 0) | 2.658 (224) | 3.612 (319) |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **0.17x** | 34.81 | 278.82 | 1.62x | 30.16 (1 / 2439) | 275.15 (0 / 0) | 58.527 (1900) | 36.178 (3128) |
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
| `t3_mkp_200` | $200 \times 8$ | **0.48x** | $21.78 \pm 13.60$ | $40.70 \pm 19.17$ | 18.62 (250 / 2860) | 49.79 (154 / 1145) | 84.59 (1135 / 16) | 169.34 (5000, `NodeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **0.37x** | $6.08 \pm 6.31$ | $12.74 \pm 9.48$ | 4.36 (1 / 20) | 8.79 (0 / 8) | 11.88 (2 / 4) | 4.84 (118) |
| `t2_binpack_44` | $44 \times 14$ | **1.19x** | $22.48 \pm 42.32$ | $6.70 \pm 2.20$ | 11.93 (274 / 1988) | 6.06 (0 / 8) | 14.22 (298 / 12) | 8.60 (772) |
| `t3_cflp_300` | $300 \times 34$ | **0.15x** | $10.86 \pm 7.73$ | $56.15 \pm 11.83$ | 8.76 (1 / 119) | 51.09 (18 / 483) | 18.93 (54 / 34) | 8.82 (116) |
| `t3_lotsizing_504` | $504 \times 336$ | **0.56x** | $57.41 \pm 35.34$ | $93.89 \pm 57.56$ | 64.33 (2 / 1351) | 71.73 (10 / 377) | 81.92 (376 / 206) | 250.06 (3625, `TimeLimit`) |
| `t3_sched_1000` | $1000 \times 100$ | **0.12x** | $8.43 \pm 4.33$ | $62.45 \pm 4.83$ | 6.25 (31 / 917) | 60.80 (0 / 76) | 19.83 (87 / 53) | 250.02 (2555, `TimeLimit`) |
<!-- END GENERATED:NON_MAD_CBC_GAP_TABLE -->

<!-- BEGIN GENERATED:NON_MAD_CBC_NOTES -->
- **Heavy-tail regression on `t2_binpack_44`**: pure-Go `BackendGo` ($22.48 \pm 42.32$ ms mean, 11.96 ms median) trails in-process `BackendCBC` ($6.70 \pm 2.20$ ms mean, 6.09 ms median) with a **0.84x** `BackendCBC / BackendGo` geometric-mean ratio (`Go / CBC = 1.19x`), running slower on 8 of 11 trials (peaking at 148.31 ms vs. 12.27 ms) while root cuts increase `CP5 +Presolve` (8.60 ms) to `CP6 +Cuts` (14.22 ms).
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

# 11. Lean 4 Prove Lane build (78 jobs), arm64 + amd64 ELF verification, CLI/JSONL checks, and 21 default-build Fuzz corpora
./scripts/verify_formal.sh
PYTHONDONTWRITEBYTECODE=1 python3 scripts/extract_elf_track5.py --verify
PYTHONDONTWRITEBYTECODE=1 python3 scripts/extract_amd64_track5e.py --verify
```

Running `go test ./...` also executes `TestDocumentationAndBenchmarkSync`, which validates corpus dimensions, parity, symbol identifiers, provenance, and every generated block across `README.md`, `docs/benchmarks.md`, and `bench/results/final_report.md`.
