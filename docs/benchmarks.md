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
- **Unified telemetry source**: fingerprint `75d553b9f304f608` (commit `b1716d8ee2f1b74a49a6d01870c52d03e0d82a58`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.85 ms median on a 1-variable MILP and 6.77 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
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
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **318.68x** [158.53x-640.61x] | 318.68x | 0.05x | 9.0 (0.8) | 195.2 (19.5) | 1103.2 (103.8) | 367.35x [42.80x-3046.45x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **17.56x** [9.47x-32.54x] | 17.56x | 0.18x | 727.6 (45.0) | 1960.5 (170.7) | 4393.2 (425.5) | 24.65x [0.14x-143.03x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **10.24x** [5.27x-19.86x] | 10.24x | 0.21x | 788.0 (142.6) | 2914.6 (561.8) | 6246.2 (1076.5) | 10.32x [1.14x-74.59x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **11.69x** [6.60x-20.70x] | 12.49x | 0.22x | 1202.8 (142.5) | 3130.6 (396.1) | 6858.4 (999.0) | 12.04x [0.14x-143.03x] |
| Tier 3 non-MAD MILPs | 4 (20) | **8.16x** [1.78x-37.43x] | 8.16x | 0.27x | 484.6 (98.3) | 1256.6 (232.9) | 2832.9 (606.5) | 5.96x [1.85x-74.59x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **27.33x** [11.63x-64.25x] | 36.29x | 0.11x | 313.5 (45.3) | 1769.4 (338.8) | 3889.9 (513.1) | 43.96x [1.14x-182.27x] |
| Random-weight MAD (negative controls) | 3 (33) | **16.48x** [0.91x-297.20x] | 16.48x | 0.14x | 297.5 (20.7) | 1215.2 (115.9) | 2263.1 (250.2) | 16.18x [1.39x-95.88x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **44.00x** [18.94x-102.20x] | 52.50x | 0.13x | 1211.1 (143.2) | 3301.0 (413.2) | 7852.7 (1092.7) | 46.07x [0.14x-3046.45x] |
| All 31 synthetic corpus families | 31 (293) | **38.91x** [20.45x-74.02x] | 48.43x | 0.12x | 1524.6 (188.5) | 5070.4 (752.0) | 11742.6 (1605.9) | 44.35x [0.14x-3046.45x] |
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
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **152.7x** [116.7x-182.3x] | 0.07 | 10.05 | 0.04 | 9.77 | 0.022 | 0.237 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **62.4x** [42.8x-95.9x] | 0.54 | 33.63 | 0.51 | 33.38 | 0.027 | 0.237 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **45.8x** [43.9x-51.6x] | 0.22 | 10.00 | 0.16 | 9.70 | 0.056 | 0.303 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **57.7x** [46.0x-66.4x] | 0.18 | 10.88 | 0.13 | 10.45 | 0.047 | 0.441 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **28.4x** [25.8x-33.9x] | 0.44 | 12.17 | 0.33 | 11.54 | 0.110 | 0.638 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **9.9x** [3.6x-27.2x] | 3.30 | 48.27 | 3.19 | 47.96 | 0.043 | 0.306 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **7.2x** [1.4x-29.5x] | 16.88 | 168.25 | 16.81 | 167.79 | 0.066 | 0.447 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **26.2x** [21.0x-31.8x] | 0.69 | 18.45 | 0.52 | 17.20 | 0.176 | 1.102 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **17.1x** [10.1x-23.7x] | 1.04 | 18.98 | 0.84 | 17.23 | 0.206 | 1.751 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **5.9x** [1.1x-14.2x] | 8.27 | 61.20 | 7.29 | 53.39 | 1.084 | 8.302 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **10.2x** [4.4x-24.9x] | 34.36 | 371.41 | 30.48 | 337.17 | 4.312 | 35.019 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Cross-Instance Latency Dispersion Across Randomized Instances (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.07 \pm 0.01$ ($\pm 0.01$) | $9.91 \pm 0.64$ ($\pm 0.43$) | $0.08 \pm 0.01$ ($\pm 0.00$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.52 \pm 0.22$ ($\pm 0.15$) | $30.51 \pm 9.49$ ($\pm 6.38$) | $0.57 \pm 0.25$ ($\pm 0.17$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.22 \pm 0.01$ ($\pm 0.01$) | $10.04 \pm 0.49$ ($\pm 0.33$) | $0.16 \pm 0.01$ ($\pm 0.01$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.19 \pm 0.03$ ($\pm 0.02$) | $10.89 \pm 0.10$ ($\pm 0.06$) | $0.21 \pm 0.06$ ($\pm 0.04$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.44 \pm 0.03$ ($\pm 0.02$) | $12.50 \pm 0.67$ ($\pm 0.45$) | $0.35 \pm 0.01$ ($\pm 0.01$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $4.16 \pm 3.20$ ($\pm 2.15$) | $36.05 \pm 17.18$ ($\pm 11.54$) | $4.05 \pm 3.07$ ($\pm 2.06$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $22.36 \pm 22.09$ ($\pm 14.84$) | $139.17 \pm 63.70$ ($\pm 42.79$) | $22.22 \pm 21.73$ ($\pm 14.59$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.70 \pm 0.03$ ($\pm 0.03$) | $18.37 \pm 2.51$ ($\pm 3.12$) | $0.65 \pm 0.06$ ($\pm 0.07$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.16 \pm 0.42$ ($\pm 0.52$) | $18.96 \pm 0.16$ ($\pm 0.20$) | $1.23 \pm 0.48$ ($\pm 0.59$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $16.67 \pm 20.85$ ($\pm 25.89$) | $63.64 \pm 3.74$ ($\pm 4.64$) | $16.66 \pm 20.61$ ($\pm 25.58$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $42.15 \pm 14.25$ ($\pm 17.69$) | $581.69 \pm 613.11$ ($\pm 761.15$) | $40.78 \pm 17.28$ ($\pm 21.45$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | **0.04** (1 / 0) | 0.04 (1 / 0) | 0.05 (1) | 0.52 (4) | 0.082 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.51** (2 / 0) | 0.57 (12 / 0) | 0.57 (12) | 0.79 (109) | 0.107 | 4601.00 | 4257.00 |
| `t2_mad_g2_i8` | **0.16** (1 / 0) | 0.76 (8 / 0) | 0.73 (8) | 0.74 (9) | 0.106 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.13** (1 / 0) | 0.13 (2 / 0) | 0.13 (2) | 2.71 (13) | 0.245 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.33** (1 / 0) | 3.21 (22 / 0) | 2.92 (22) | 5.26 (23) | 0.379 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.19** (43 / 0) | 2.89 (112 / 0) | 2.84 (112) | 3.60 (45) | 0.160 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **16.81** (201 / 0) | 30.36 (188 / 0) | 30.29 (188) | 92.38 (539) | 0.278 | 7720.00 | 6060.00 |
| `t3_mad_g5_i20` | **0.52** (1 / 0) | 2.99 (17 / 0) | 2.66 (17) | 0.52 (1) | 0.654 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.84** (1 / 0) | 8.65 (58 / 0) | 7.53 (58) | 13.03 (74) | 1.584 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.29** (2 / 0) | 21.27 (25 / 0) | 20.02 (25) | 250.38 (152, `TimeLimit`) | 2.658 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **30.48** (1 / 0) | 32.86 (2 / 0) | 32.22 (2) | 250.08 (6, `TimeLimit`) | 57.530 | 208000.00 | 208000.00 |
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
| `t1_std_lp` | 3 | 3 | **441.1x** [94.5x-817.7x] | 0.01 | 6.80 | 0.00 | 6.74 | 0.006 | 0.065 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **411.8x** [209.6x-921.3x] | 0.02 | 6.69 | 0.01 | 6.62 | 0.008 | 0.063 | `OK` |
| `t1_std_mip` | 3 | 3 | **440.2x** [264.2x-621.9x] | 0.01 | 7.96 | 0.01 | 7.90 | 0.007 | 0.066 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **405.5x** [192.3x-779.9x] | 0.02 | 7.47 | 0.01 | 7.41 | 0.007 | 0.064 | `OK` |
| `t1_whiskas` | 6 | 5 | **438.6x** [109.5x-760.8x] | 0.01 | 6.68 | 0.01 | 6.57 | 0.006 | 0.089 | `OK` |
| `t1_furniture` | 2 | 3 | **291.0x** [84.8x-824.1x] | 0.02 | 8.28 | 0.02 | 8.22 | 0.003 | 0.065 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **117.9x** [55.3x-378.9x] | 0.10 | 9.63 | 0.09 | 9.53 | 0.008 | 0.105 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **2322.0x** [1127.8x-3046.5x] | 0.00 | 6.60 | 0.00 | 6.55 | 0.002 | 0.056 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **152.7x** [116.7x-182.3x] | 0.07 | 10.05 | 0.04 | 9.77 | 0.022 | 0.237 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **62.4x** [42.8x-95.9x] | 0.54 | 33.63 | 0.51 | 33.38 | 0.027 | 0.237 | `OK` |
| `t2_mkp_30` | 30 | 6 | **12.1x** [6.3x-17.8x] | 2.13 | 28.54 | 2.10 | 28.36 | 0.018 | 0.183 | `OK` |
| `t2_mkp_50` | 50 | 8 | **5.9x** [3.1x-32.6x] | 4.13 | 17.08 | 4.10 | 16.83 | 0.033 | 0.268 | `OK` |
| `t2_cflp_36` | 36 | 11 | **50.8x** [11.1x-143.0x] | 0.18 | 9.83 | 0.16 | 9.65 | 0.022 | 0.187 | `OK` |
| `t2_cflp_49` | 49 | 13 | **35.0x** [21.1x-89.0x] | 0.38 | 10.84 | 0.34 | 10.57 | 0.033 | 0.229 | `OK` |
| `t2_gap_45` | 45 | 13 | **23.9x** [4.9x-85.0x] | 0.79 | 14.80 | 0.76 | 14.59 | 0.021 | 0.212 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **11.5x** [5.6x-23.2x] | 4.29 | 69.65 | 4.26 | 69.40 | 0.035 | 0.248 | `OK` |
| `t2_binpack_44` | 44 | 14 | **1.9x** [0.1x-25.6x] | 11.84 | 14.37 | 11.81 | 14.14 | 0.030 | 0.221 | `OK` |
| `t2_setcover_50` | 50 | 20 | **41.5x** [7.0x-127.0x] | 0.26 | 10.86 | 0.23 | 10.50 | 0.033 | 0.305 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **45.8x** [43.9x-51.6x] | 0.22 | 10.00 | 0.16 | 9.70 | 0.056 | 0.303 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **57.7x** [46.0x-66.4x] | 0.18 | 10.88 | 0.13 | 10.45 | 0.047 | 0.441 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **28.4x** [25.8x-33.9x] | 0.44 | 12.17 | 0.33 | 11.54 | 0.110 | 0.638 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.9x** [3.6x-27.2x] | 3.30 | 48.27 | 3.19 | 47.96 | 0.043 | 0.306 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **7.2x** [1.4x-29.5x] | 16.88 | 168.25 | 16.81 | 167.79 | 0.066 | 0.447 | `OK` |
| `t3_mkp_200` | 200 | 8 | **4.9x** [4.2x-6.3x] | 18.47 | 117.14 | 18.39 | 116.54 | 0.081 | 0.605 | `OK` |
| `t3_cflp_300` | 300 | 34 | **27.0x** [8.3x-74.6x] | 9.37 | 251.05 | 9.22 | 250.15 | 0.110 | 0.921 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **3.1x** [1.9x-4.9x] | 63.09 | 157.44 | 62.86 | 155.59 | 0.218 | 1.865 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **11.0x** [5.2x-17.5x] | 7.35 | 80.85 | 7.06 | 77.89 | 0.302 | 2.814 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **26.2x** [21.0x-31.8x] | 0.69 | 18.45 | 0.52 | 17.20 | 0.176 | 1.102 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **17.1x** [10.1x-23.7x] | 1.04 | 18.98 | 0.84 | 17.23 | 0.206 | 1.751 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.9x** [1.1x-14.2x] | 8.27 | 61.20 | 7.29 | 53.39 | 1.084 | 8.302 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **10.2x** [4.4x-24.9x] | 34.36 | 371.41 | 30.48 | 337.17 | 4.312 | 35.019 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## 5. Default Build vs. `GOEXPERIMENT=simd`

In [`internal/simd/kernels_amd64.go`](../internal/simd/kernels_amd64.go) and [`internal/simd/kernels_avx512_amd64.go`](../internal/simd/kernels_avx512_amd64.go), runtime CPUID dispatch selects AVX-512, AVX2, or 4-way scalar loops from a single portable `CGO_ENABLED=0` binary (`GOAMD64=v3` FMA-safe). Each vector kernel spills accumulators via `StoreArray` and executes `archsimd.ClearAVXUpperBits()` (`VZEROUPPER`) before scalar tail or caller operations, eliminating AVX-to-SSE transition stalls while preserving IEEE-754 `NaN` propagation parity. AVX2 activates at `minAVX2Len = 32` (`Dot64`, `Axpy64`, `Scale64`, `AbsMax64`, `RowActivityBoundsFinite`, `SelectDualLeaveRow64With`, `MinReducedSkip64With`, `InitReducedRow64`, `AllAbsLE64With`, `ValidateAndNegate64With`), `rows >= 8 && cols >= 8` for `Transpose64With`, `minDualRatioSIMDChunks = 4` (256 cols for `ScanDualRatioCand64With`), and `minAVX2Min2Len = 256` (`Min2ReducedRow64`); on AVX-512 hosts, dense LSAP loops dispatch AVX-512 at `minAVX512Len = 64` and `minAVX512Min2Len = 128` while sparse LP kernels gate AVX-512 at `minAVX512LPLen = 2048` / `minLeaveRowAVX512 = 2048` / `minDualRatioAVX512Chunks = 128` (8,192 cols) to avoid Intel Emerald Rapids License-2 clock down-binning across surrounding scalar LU code.

At the full-solver level, each ratio below is the geometric mean of per-trial `scalar / SIMD` solve times over the same instances on the local AVX2 host (`i9-13900K`), with a 95% $t$-interval on the log ratios; "no detectable effect" means the interval contains 1. "Reachable" is a necessary condition: the AVX2 LP kernels require `n + m >= 32`. Below that size no vector kernel can run, so those small models serve as negative controls where intervals excluding 1 reflect binary code-layout or run-to-run variance rather than SIMD execution. Per-family paired speedups and 95% intervals for larger models appear in the generated table below.

### 5.1 Scope and Pending Target-Hardware Verification

All telemetry in this document is measured locally on the AVX2-only `13th Gen Intel(R) Core(TM) i9-13900K` host (`x86_64`, Linux). Portable `GOOS=linux GOARCH=arm64 CGO_ENABLED=0` and `GOAMD64=v3`/`v4` builds compile and pass local build checks, whereas fresh physical AVX-512 and `linux/arm64` runtime test and benchmark re-verification on the current tree remain pending target hardware access.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 2 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 6 faster, 2 slower, 15 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 1.31x [0.74x-2.31x] | 0.005 | 0.004 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 1.37x [0.87x-2.17x] | 0.006 | 0.004 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.47x [1.12x-1.92x] | 0.007 | 0.005 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.51x [1.15x-2.00x] | 0.006 | 0.004 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 0.98x [0.88x-1.09x] | 0.006 | 0.006 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 1.04x [0.93x-1.16x] | 0.015 | 0.015 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source | 0.96x [0.92x-1.01x] | 0.091 | 0.092 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source | 1.39x [0.81x-2.38x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | SIMD slower | 0.90x [0.86x-0.95x] | 0.041 | 0.047 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 0.93x [0.83x-1.04x] | 0.505 | 0.501 | 11 |
| `t2_mkp_30` | 36 | possible | SIMD slower | 0.99x [0.98x-0.99x] | 2.105 | 2.163 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 0.99x [0.91x-1.08x] | 4.101 | 4.054 | 11 |
| `t2_cflp_36` | 47 | possible | SIMD faster | 1.11x [1.00x-1.24x] | 0.157 | 0.139 | 11 |
| `t2_cflp_49` | 62 | possible | SIMD faster | 1.13x [1.09x-1.17x] | 0.342 | 0.291 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 0.87x [0.67x-1.13x] | 0.760 | 0.765 | 11 |
| `t2_lotsizing_48` | 80 | possible | no detectable effect | 1.01x [0.95x-1.07x] | 4.262 | 4.152 | 11 |
| `t2_binpack_44` | 58 | possible | no detectable effect | 1.02x [0.97x-1.06x] | 11.811 | 11.881 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 0.92x [0.85x-1.01x] | 0.228 | 0.234 | 11 |
| `t2_mad_g2_i8` | 72 | possible | SIMD faster | 1.27x [1.23x-1.31x] | 0.157 | 0.124 | 11 |
| `t2_mad_g3_i9` | 108 | possible | no detectable effect | 0.96x [0.89x-1.03x] | 0.131 | 0.127 | 11 |
| `t2_mad_g4_i12` | 156 | possible | SIMD faster | 1.17x [1.12x-1.22x] | 0.328 | 0.281 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.02x [1.00x-1.03x] | 3.194 | 2.943 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 1.00x [0.99x-1.01x] | 16.806 | 16.354 | 11 |
| `t3_mkp_200` | 208 | possible | SIMD faster | 1.07x [1.04x-1.09x] | 18.387 | 17.040 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.02x [0.99x-1.04x] | 9.224 | 9.115 | 5 |
| `t3_lotsizing_504` | 840 | possible | SIMD faster | 1.03x [1.02x-1.04x] | 62.864 | 61.695 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 0.99x [0.88x-1.12x] | 7.059 | 6.263 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 1.07x [0.96x-1.18x] | 0.515 | 0.463 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 1.03x [0.98x-1.08x] | 0.844 | 0.817 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 1.00x [0.97x-1.04x] | 7.291 | 7.445 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | no detectable effect | 1.06x [0.97x-1.16x] | 30.481 | 27.266 | 5 |
<!-- END GENERATED:SIMD_EFFECT_TABLE -->

---

## 6. Full Corpus Solver-Core Checkpoint Progression

<!-- BEGIN GENERATED:FULL_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_std_lp` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 54.00 | 54.00 |
| `t1_std_lp_max` | **0.01** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.002 | 80.00 | 80.00 |
| `t1_std_mip` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.004 | 64.00 | 58.00 |
| `t1_mip_float_obj` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (3) | 0.003 | 64.95 | 58.90 |
| `t1_whiskas` | **0.01** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.003 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.002 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.09** (1 / 8) | 0.10 (2 / 8) | 0.15 (18) | 0.21 (35) | 0.007 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.04** (1 / 0) | 0.04 (1 / 0) | 0.05 (1) | 0.52 (4) | 0.082 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.51** (2 / 0) | 0.57 (12 / 0) | 0.57 (12) | 0.79 (109) | 0.107 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.10** (5 / 9) | 2.60 (40 / 9) | 1.07 (51) | 0.94 (61) | 0.024 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **4.10** (1 / 4) | 11.53 (2 / 4) | 4.73 (118) | 4.20 (111) | 0.038 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.16** (1 / 2) | 0.23 (3 / 2) | 0.18 (7) | 0.18 (7) | 0.028 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.34** (1 / 7) | 0.60 (3 / 7) | 0.31 (9) | 0.28 (9) | 0.037 | 909.02 | 827.30 |
| `t2_gap_45` | **0.76** (1 / 7) | 0.41 (1 / 7) | 1.81 (41) | 1.98 (60) | 0.026 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **4.26** (2 / 37) | 7.80 (25 / 37) | 3.03 (162) | 3.26 (199) | 0.059 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **11.81** (274 / 12) | 14.80 (298 / 12) | 8.38 (772) | 27.29 (1941) | 0.028 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.23** (1 / 6) | 0.33 (2 / 6) | 0.67 (6) | 0.65 (6) | 0.031 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.16** (1 / 0) | 0.76 (8 / 0) | 0.73 (8) | 0.74 (9) | 0.106 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.13** (1 / 0) | 0.13 (2 / 0) | 0.13 (2) | 2.71 (13) | 0.245 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.33** (1 / 0) | 3.21 (22 / 0) | 2.92 (22) | 5.26 (23) | 0.379 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.19** (43 / 0) | 2.89 (112 / 0) | 2.84 (112) | 3.60 (45) | 0.160 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **16.81** (201 / 0) | 30.36 (188 / 0) | 30.29 (188) | 92.38 (539) | 0.278 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **18.39** (250 / 14) | 81.83 (1135 / 16) | 166.07 (5000, `NodeLimit`) | 147.99 (5000, `NodeLimit`) | 0.119 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **9.22** (1 / 28) | 18.11 (54 / 34) | 9.48 (116) | 8.72 (118) | 0.173 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **62.86** (2 / 206) | 80.47 (376 / 206) | 250.03 (3691, `TimeLimit`) | 250.04 (3806, `TimeLimit`) | 0.721 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **7.06** (31 / 55) | 19.21 (87 / 53) | 250.02 (2594, `TimeLimit`) | 250.04 (2632, `TimeLimit`) | 0.191 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.52** (1 / 0) | 2.99 (17 / 0) | 2.66 (17) | 0.52 (1) | 0.654 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.84** (1 / 0) | 8.65 (58 / 0) | 7.53 (58) | 13.03 (74) | 1.584 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.29** (2 / 0) | 21.27 (25 / 0) | 20.02 (25) | 250.38 (152, `TimeLimit`) | 2.658 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **30.48** (1 / 0) | 32.86 (2 / 0) | 32.22 (2) | 250.08 (6, `TimeLimit`) | 57.530 | 208000.00 | 208000.00 |
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
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.50x** [1.23x-1.83x] | 1.50x | 4.67x | 0.214 (0.0194) | 0.311 (0.0287) | 1.55x [0.88x-2.43x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **2.98x** [1.86x-4.75x] | 2.98x | 2.81x | 9.078 (0.804) | 32.713 (2.963) | 2.52x [0.87x-15.52x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.84x** [1.86x-4.34x] | 2.84x | 3.67x | 225.109 (44.447) | 950.469 (190.022) | 2.53x [1.51x-5.97x] |
| Square families ($m = n$) | 16 (146) | **2.74x** [2.01x-3.72x] | 2.68x | 3.74x | 229.619 (44.413) | 972.672 (191.065) | 2.15x [0.89x-15.52x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.77x** [1.27x-2.47x] | 1.66x | 2.97x | 4.783 (0.858) | 10.821 (1.949) | 1.94x [0.87x-2.74x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.27x** [1.15x-4.50x] | 2.40x | 2.75x | 35.471 (6.267) | 64.825 (10.928) | 1.80x [0.89x-15.52x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.43x** [1.90x-3.10x] | 2.35x | 3.51x | 234.401 (45.271) | 983.493 (193.014) | 2.04x [0.87x-15.52x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 7.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.90x** | 0.0005 | 0.0009 | 0.0005 | 0.0116 | $0.0005 \pm 0.0001$ | $0.0009 \pm 0.0001$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.47x** | 0.0016 | 0.0024 | 0.0019 | 0.0102 | $0.0016 \pm 0.0004$ | $0.0024 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.71x** | 0.0036 | 0.0061 | 0.0044 | 0.0151 | $0.0035 \pm 0.0007$ | $0.0060 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.20x** | 0.0036 | 0.0040 | 0.0047 | 0.0141 | $0.0033 \pm 0.0007$ | $0.0040 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.20x** | 0.0037 | 0.0043 | 0.0053 | 0.0151 | $0.0038 \pm 0.0012$ | $0.0045 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.62x** | 0.0065 | 0.0111 | 0.0113 | 0.0327 | $0.0067 \pm 0.0022$ | $0.0106 \pm 0.0020$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.69x** | 0.0300 | 0.0707 | 0.0430 | 0.0959 | $0.0265 \pm 0.0065$ | $0.0697 \pm 0.0104$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.44x** | 0.0532 | 0.177 | 0.0834 | 0.227 | $0.0522 \pm 0.0050$ | $0.180 \pm 0.0229$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.05x** | 0.0485 | 0.0952 | 0.0861 | 0.144 | $0.0469 \pm 0.0056$ | $0.0963 \pm 0.0139$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.60x** | 0.139 | 0.220 | 0.171 | 0.257 | $0.148 \pm 0.0437$ | $0.233 \pm 0.0443$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **12.00x** | 0.0617 | 0.688 | 0.0818 | 0.768 | $0.0580 \pm 0.0082$ | $0.689 \pm 0.0157$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.95x** | 0.0355 | 0.0722 | 0.0624 | 0.145 | $0.0387 \pm 0.0136$ | $0.0731 \pm 0.0082$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **1.93x** | 0.0391 | 0.0788 | 0.0665 | 0.153 | $0.0427 \pm 0.0126$ | $0.0802 \pm 0.0068$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.43x** | 0.0829 | 0.282 | 0.113 | 0.358 | $0.0842 \pm 0.0150$ | $0.286 \pm 0.0316$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.90x** | 0.314 | 1.280 | 0.428 | 1.694 | $0.328 \pm 0.0582$ | $1.266 \pm 0.120$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.88x** | 1.280 | 5.258 | 1.649 | 7.992 | $1.397 \pm 0.285$ | $5.347 \pm 0.406$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.74x** | 0.850 | 1.480 | 1.446 | 4.215 | $0.855 \pm 0.0321$ | $1.484 \pm 0.0515$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.65x** | 5.157 | 8.428 | 5.329 | 9.310 | $5.659 \pm 2.006$ | $9.203 \pm 2.632$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.48x** | 0.343 | 0.829 | 0.740 | 3.688 | $0.337 \pm 0.0152$ | $0.839 \pm 0.0581$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.28x** | 0.433 | 0.961 | 0.815 | 3.806 | $0.424 \pm 0.0163$ | $0.969 \pm 0.0319$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.95x** | 5.743 | 28.345 | 8.058 | 37.258 | $5.834 \pm 0.391$ | $28.886 \pm 2.229$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.72x** | 30.642 | 144.721 | 39.014 | 189.812 | $30.514 \pm 4.074$ | $143.365 \pm 8.427$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 7.3 LSAP Scalar vs. `GOEXPERIMENT=simd`

In [`assign/solver.go`](../assign/solver.go), [`simd.AllAbsLE64With`](../internal/simd/kernels_amd64.go) and [`simd.ValidateAndNegate64With`](../internal/simd/assign_amd64.go) validate finite cost bounds via AVX2 checks whenever `len(cost) >= 32` (while the fused `colReduceValidateSquare` fast path skips `AllAbsLE64With` on finite square `Minimize` matrices and dispatches `ColReduceValidateRows64With` only for $n \ge 32$), [`simd.Transpose64With`](../internal/simd/assign_amd64.go) handles tall transposes with $\text{rows} \ge 8$ and $\text{cols} \ge 8$, [`simd.MinReducedSkip64With`](../internal/simd/assign_amd64.go) accelerates reduction-transfer slack scans in `preprocessSquareJV` ($n \ge 32$), [`simd.Min2ReducedRow64With`](../internal/simd/kernels_amd64.go) accelerates first/second reduced-minimum scans in `preprocessRectangularBV` and `augmentingRowReduction` at `minAVX2Min2Len = 256` (`minAVX512Min2Len = 128`), and [`simd.InitReducedRow64With`](../internal/simd/kernels_amd64.go) runs on a slice of length $\max(m, n) \ge 32$ at the start of each Dijkstra augmentation. All vector kernels clean upper `YMM` state via `StoreArray` + `ClearAVXUpperBits()` (`VZEROUPPER`) before scalar operations; exact per-family verdicts appear in the generated table below.

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` with no `>= 8x8` tall transpose and either fused square `Minimize` non-sparse path or `rows * cols < 32`, so no AVX2 kernel runs): 2 of 3 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32`, tall `rows > cols` with `>= 8x8`, or fallback `AllAbsLE64`/`ValidateAndNegate64` at `rows * cols >= 32`): 16 faster, 1 slower, 2 with no detectable effect, out of 19.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.62x [0.41x-0.95x] | 0.0005 | 0.0006 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source | 0.78x [0.55x-1.10x] | 0.0016 | 0.0018 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.87x [0.75x-1.00x] | 0.0036 | 0.0037 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | no detectable effect | 0.95x [0.82x-1.10x] | 0.0036 | 0.0035 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | no detectable effect | 0.97x [0.79x-1.19x] | 0.0037 | 0.0039 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | SIMD slower | 0.94x [0.88x-1.00x] | 0.0065 | 0.0071 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.22x [1.16x-1.30x] | 0.0300 | 0.0225 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.28x [1.21x-1.35x] | 0.0532 | 0.0413 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.58x [1.28x-1.95x] | 0.0485 | 0.0293 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.76x [1.62x-1.91x] | 0.139 | 0.0804 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.32x [1.26x-1.38x] | 0.0617 | 0.0466 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.32x [1.02x-1.71x] | 0.0355 | 0.0271 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.41x [1.24x-1.61x] | 0.0391 | 0.0294 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.40x [1.20x-1.63x] | 0.0829 | 0.0556 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.58x [1.47x-1.69x] | 0.314 | 0.196 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.87x [1.52x-2.31x] | 1.280 | 0.740 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.49x [2.35x-2.64x] | 0.850 | 0.349 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.57x [2.36x-2.80x] | 5.157 | 1.936 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.82x [1.73x-1.91x] | 0.343 | 0.186 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.60x [1.49x-1.72x] | 0.433 | 0.273 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.62x [1.50x-1.76x] | 5.743 | 3.663 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.42x [1.33x-1.53x] | 30.642 | 20.479 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

---

## 8. Production Binary Footprint and Linker Dead-Code Elimination

The `goblin-bench -binary-size` mode compiles four standalone `CGO_ENABLED=0` programs in an isolated temporary module against the live workspace across both default `go build` and `GOEXPERIMENT=simd`, saving compiled artifact sizes and `go tool nm -size` symbol counts to `bench/results/goblin_binary_size.json`. [`ValidateBinarySizeTelemetry`](../bench/docgen.go) rejects the telemetry bundle if `assign_default` links any `math/big`, `encoding/json`, `VIPR`, or `Certif` symbols, if `goblin_default` links any `encoding/json`, `VIPR`, or `Certif` symbols, or if any profile links `encoding/json`:

<!-- BEGIN GENERATED:BINARY_SIZE_TABLE -->
| Profile | Default Raw / Stripped (KiB) | `GOEXPERIMENT=simd` Raw / Stripped (KiB) | Stripped Delta vs. Default | `goblin` Text (KiB) | `math/big` Syms | `encoding/json` Syms | `VIPR` Syms | `Certif` Syms |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `assign` default (`LinearSum` + `SolveInto`) | 2780.5 / **1740.2** | 2816.4 / **1804.2** | baseline | 117.0 | `0` | `0` | `0` | `0` |
| `assign` certified (`+ assign.WithCertified(true)`) | 2872.0 / **1800.2** | 2909.4 / **1868.2** | +60.0 KiB (+3.4%) | 158.0 | `0` | `0` | `0` | `17` |
| `goblin` default (`Model.Solve`, `DefaultOptions`) | 5278.5 / **3356.2** | 5173.6 / **3376.2** | baseline | 886.4 | `130` | `0` | `0` | `0` |
| `goblin` certified + VIPR (`+ WithCertified + WithVIPRTrace`) | 6387.6 / **4100.2** | 6254.2 / **4124.2** | +744.0 KiB (+22.2%) | 1304.6 | `155` | `0` | `203` | `13` |
<!-- END GENERATED:BINARY_SIZE_TABLE -->

---

## 9. Pure-Go vs. In-Process C++ CBC Performance Analysis

Comparing isolated unpresolved `Root LP` timings, paired trial ratios, and solver-core checkpoints (`CP5 +Presolve`, `CP6 +Cuts`, `CP7 Full`) between `BackendGo` and in-process `BackendCBC` (`c1c171ec`) highlights four structural trade-offs on large degenerate LPs and heavy-tail branch-and-cut models.

### 9.1 MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families ($N = G(I + T + 3)$ variables, $M = G(2T + 3) + I$ constraints) separate solver behavior into three scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Paired `Go / CBC` | Go Total Med (ms) | CBC Total Med (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.03x** | 0.07 | 2.46 | 0.60x | 0.04 (1 / 19) | 2.43 (0 / 0) | 0.082 (28) | 0.136 (19) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.05x** | 0.54 | 11.29 | 0.71x | 0.51 (2 / 58) | 11.25 (6 / 83) | 0.107 (30) | 0.149 (18) |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.11x** | 0.22 | 1.79 | 0.70x | 0.16 (1 / 26) | 1.74 (0 / 0) | 0.106 (33) | 0.151 (21) |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.08x** | 0.18 | 2.46 | 0.95x | 0.13 (1 / 40) | 2.40 (0 / 0) | 0.245 (58) | 0.257 (40) |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.13x** | 0.44 | 3.16 | 0.93x | 0.33 (1 / 51) | 3.08 (0 / 0) | 0.379 (80) | 0.406 (63) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.25x** | 3.30 | 16.73 | 1.03x | 3.19 (43 / 437) | 16.68 (12 / 216) | 0.160 (38) | 0.155 (24) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.25x** | 16.88 | 87.87 | 0.98x | 16.81 (201 / 2102) | 87.80 (198 / 1300) | 0.278 (65) | 0.282 (54) |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.09x** | 0.69 | 8.15 | 1.03x | 0.52 (1 / 75) | 8.02 (0 / 0) | 0.654 (91) | 0.633 (94) |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.15x** | 1.04 | 7.51 | 1.50x | 0.84 (1 / 98) | 7.31 (0 / 0) | 1.584 (244) | 1.057 (149) |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **0.29x** | 8.27 | 36.37 | 0.74x | 7.29 (2 / 364) | 35.35 (0 / 0) | 2.658 (224) | 3.579 (319) |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **0.17x** | 34.36 | 276.93 | 1.62x | 30.48 (1 / 2439) | 272.81 (0 / 0) | 57.530 (1900) | 35.458 (3128) |
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
| `t3_mkp_200` | $200 \times 8$ | **0.48x** | $21.70 \pm 13.40$ | $40.70 \pm 19.29$ | 18.39 (250 / 2447) | 49.77 (154 / 1145) | 81.83 (1135 / 16) | 166.07 (5000, `NodeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **0.37x** | $5.93 \pm 6.08$ | $12.47 \pm 9.19$ | 4.10 (1 / 19) | 8.67 (0 / 8) | 11.53 (2 / 4) | 4.73 (118) |
| `t2_binpack_44` | $44 \times 14$ | **1.19x** | $22.14 \pm 41.75$ | $6.73 \pm 2.20$ | 11.81 (274 / 1988) | 6.14 (0 / 8) | 14.80 (298 / 12) | 8.38 (772) |
| `t3_cflp_300` | $300 \times 34$ | **0.15x** | $10.60 \pm 7.35$ | $55.22 \pm 11.80$ | 9.22 (1 / 105) | 50.20 (18 / 483) | 18.11 (54 / 34) | 9.48 (116) |
| `t3_lotsizing_504` | $504 \times 336$ | **0.55x** | $56.37 \pm 35.09$ | $93.81 \pm 57.97$ | 62.86 (2 / 1061) | 71.85 (10 / 377) | 80.47 (376 / 206) | 250.03 (3691, `TimeLimit`) |
| `t3_sched_1000` | $1000 \times 100$ | **0.12x** | $8.25 \pm 4.46$ | $61.59 \pm 4.78$ | 7.06 (31 / 474) | 60.02 (0 / 76) | 19.21 (87 / 53) | 250.02 (2594, `TimeLimit`) |
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
