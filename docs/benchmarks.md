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
- **Unified telemetry source**: fingerprint `2e436740e97c6e65` (commit `a2bc4d40b277efdcbd17d9195414ae4b25a453c8`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `5`; Python + CBC `5`; Python + SciPy `5`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.65 ms median on a 1-variable MILP and 6.88 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
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
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **336.13x** [147.71x-764.90x] | 336.13x | 0.06x | 9.9 (0.9) | 158.3 (14.8) | 1150.4 (106.4) | 454.45x [31.67x-1522.91x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **19.87x** [11.95x-33.04x] | 19.87x | 0.17x | 548.2 (36.3) | 1844.1 (162.8) | 4433.0 (425.5) | 24.35x [1.37x-166.10x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **9.64x** [4.62x-20.09x] | 9.64x | 0.26x | 889.6 (165.5) | 2633.0 (537.5) | 6312.5 (1090.9) | 10.34x [1.12x-60.19x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **12.44x** [7.40x-20.94x] | 13.71x | 0.21x | 1118.7 (154.3) | 3117.3 (405.9) | 6936.5 (1007.6) | 13.09x [1.37x-166.10x] |
| Tier 3 non-MAD MILPs | 4 (20) | **7.35x** [1.29x-41.78x] | 7.35x | 0.28x | 578.8 (118.6) | 1333.7 (248.3) | 2865.1 (614.6) | 5.40x [1.42x-60.19x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **26.44x** [12.08x-57.86x] | 34.88x | 0.17x | 320.2 (47.7) | 1373.1 (295.6) | 3914.0 (518.3) | 38.98x [1.12x-148.52x] |
| Random-weight MAD (negative controls) | 3 (33) | **16.21x** [0.82x-320.54x] | 16.21x | 0.18x | 319.9 (22.1) | 1059.5 (104.0) | 2297.3 (248.4) | 15.45x [1.37x-115.18x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **47.83x** [20.40x-112.16x] | 58.21x | 0.12x | 1127.4 (155.0) | 3262.4 (419.6) | 7982.0 (1104.5) | 41.90x [1.37x-1522.91x] |
| All 31 synthetic corpus families | 31 (293) | **41.05x** [21.47x-78.48x] | 52.05x | 0.13x | 1447.6 (202.7) | 4635.5 (715.1) | 11896.0 (1622.8) | 41.14x [1.12x-1522.91x] |
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
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **91.3x** [66.4x-148.5x] | 0.11 | 9.51 | 0.06 | 9.28 | 0.042 | 0.228 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **64.1x** [39.8x-115.2x] | 0.51 | 33.61 | 0.48 | 33.37 | 0.028 | 0.243 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **60.9x** [56.7x-67.0x] | 0.16 | 9.92 | 0.13 | 9.61 | 0.035 | 0.319 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **50.9x** [36.8x-65.6x] | 0.20 | 10.56 | 0.13 | 10.12 | 0.050 | 0.436 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **33.1x** [30.0x-36.1x] | 0.36 | 12.06 | 0.29 | 11.41 | 0.074 | 0.628 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **9.5x** [3.7x-26.1x] | 3.07 | 48.08 | 3.03 | 47.74 | 0.041 | 0.328 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **7.0x** [1.4x-30.4x] | 18.49 | 166.68 | 18.37 | 166.24 | 0.066 | 0.458 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **24.3x** [17.7x-29.0x] | 0.78 | 19.40 | 0.50 | 18.23 | 0.279 | 1.099 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **18.1x** [10.9x-24.0x] | 1.06 | 19.42 | 0.84 | 17.57 | 0.231 | 1.793 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **5.7x** [1.1x-13.8x] | 8.81 | 63.59 | 7.68 | 55.39 | 1.167 | 8.681 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **10.2x** [4.3x-24.3x] | 36.19 | 373.83 | 30.70 | 336.58 | 4.586 | 36.208 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Cross-Instance Latency Dispersion Across Randomized Instances (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.11 \pm 0.02$ ($\pm 0.02$) | $9.54 \pm 0.43$ ($\pm 0.29$) | $0.09 \pm 0.02$ ($\pm 0.02$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.52 \pm 0.24$ ($\pm 0.16$) | $31.13 \pm 9.79$ ($\pm 6.58$) | $0.52 \pm 0.24$ ($\pm 0.16$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.16 \pm 0.01$ ($\pm 0.00$) | $10.02 \pm 0.53$ ($\pm 0.36$) | $0.17 \pm 0.01$ ($\pm 0.01$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.22 \pm 0.05$ ($\pm 0.03$) | $10.71 \pm 0.33$ ($\pm 0.22$) | $0.26 \pm 0.03$ ($\pm 0.02$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.37 \pm 0.01$ ($\pm 0.01$) | $12.15 \pm 0.55$ ($\pm 0.37$) | $0.39 \pm 0.02$ ($\pm 0.01$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $4.36 \pm 3.42$ ($\pm 2.29$) | $36.24 \pm 17.38$ ($\pm 11.67$) | $4.39 \pm 3.42$ ($\pm 2.30$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $24.20 \pm 25.19$ ($\pm 16.92$) | $141.48 \pm 64.54$ ($\pm 43.36$) | $24.61 \pm 24.51$ ($\pm 16.47$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.77 \pm 0.06$ ($\pm 0.07$) | $18.68 \pm 2.62$ ($\pm 3.25$) | $0.70 \pm 0.07$ ($\pm 0.09$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.11 \pm 0.39$ ($\pm 0.49$) | $19.35 \pm 0.33$ ($\pm 0.40$) | $1.14 \pm 0.34$ ($\pm 0.43$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $17.65 \pm 21.88$ ($\pm 27.16$) | $64.84 \pm 3.03$ ($\pm 3.76$) | $18.67 \pm 23.39$ ($\pm 29.03$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $42.63 \pm 15.46$ ($\pm 19.19$) | $586.62 \pm 620.34$ ($\pm 770.13$) | $42.80 \pm 16.57$ ($\pm 20.57$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | **0.06** (1 / 0) | 0.04 (1 / 0) | 0.05 (1) | 0.53 (4) | 0.085 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.48** (2 / 0) | 0.57 (12 / 0) | 0.57 (12) | 0.82 (109) | 0.111 | 4601.00 | 4257.00 |
| `t2_mad_g2_i8` | **0.13** (1 / 0) | 0.89 (8 / 0) | 0.80 (8) | 0.77 (9) | 0.109 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.13** (1 / 0) | 0.17 (2 / 0) | 0.15 (2) | 2.84 (13) | 0.266 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.29** (1 / 0) | 3.47 (22 / 0) | 3.10 (22) | 5.38 (23) | 0.363 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.03** (43 / 0) | 3.21 (112 / 0) | 3.14 (112) | 3.74 (45) | 0.158 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **18.37** (196 / 0) | 35.21 (188 / 0) | 32.98 (188) | 99.62 (539) | 0.284 | 7720.00 | 6060.00 |
| `t3_mad_g5_i20` | **0.50** (1 / 0) | 3.32 (17 / 0) | 2.86 (17) | 0.55 (1) | 0.699 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.84** (1 / 0) | 8.80 (58 / 0) | 8.40 (58) | 15.11 (74) | 1.502 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.68** (2 / 0) | 22.18 (25 / 0) | 20.78 (25) | 250.49 (145, `TimeLimit`) | 2.686 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **30.70** (1 / 0) | 33.27 (2 / 0) | 32.50 (2) | 250.26 (6, `TimeLimit`) | 58.578 | 208000.00 | 208000.00 |
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
| `t1_std_lp` | 3 | 3 | **1036.5x** [695.0x-1251.6x] | 0.01 | 8.11 | 0.00 | 8.03 | 0.004 | 0.078 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **1064.1x** [729.7x-1522.9x] | 0.01 | 7.80 | 0.00 | 7.72 | 0.004 | 0.078 | `OK` |
| `t1_std_mip` | 3 | 3 | **861.7x** [537.2x-1077.3x] | 0.01 | 7.85 | 0.00 | 7.79 | 0.004 | 0.070 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **962.2x** [682.4x-1224.7x] | 0.01 | 8.94 | 0.00 | 8.87 | 0.004 | 0.078 | `OK` |
| `t1_whiskas` | 6 | 5 | **404.7x** [305.5x-540.9x] | 0.02 | 6.53 | 0.01 | 6.45 | 0.008 | 0.079 | `OK` |
| `t1_furniture` | 2 | 3 | **244.7x** [80.3x-611.4x] | 0.02 | 8.25 | 0.02 | 8.19 | 0.006 | 0.057 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **68.0x** [31.7x-157.0x] | 0.16 | 9.29 | 0.13 | 9.18 | 0.018 | 0.096 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **510.2x** [180.3x-1130.3x] | 0.01 | 6.55 | 0.00 | 6.50 | 0.008 | 0.052 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **91.3x** [66.4x-148.5x] | 0.11 | 9.51 | 0.06 | 9.28 | 0.042 | 0.228 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **64.1x** [39.8x-115.2x] | 0.51 | 33.61 | 0.48 | 33.37 | 0.028 | 0.243 | `OK` |
| `t2_mkp_30` | 30 | 6 | **10.4x** [4.7x-15.9x] | 2.40 | 29.03 | 2.38 | 28.84 | 0.020 | 0.181 | `OK` |
| `t2_mkp_50` | 50 | 8 | **5.0x** [2.4x-27.8x] | 5.32 | 17.59 | 5.29 | 17.32 | 0.034 | 0.270 | `OK` |
| `t2_cflp_36` | 36 | 11 | **55.4x** [21.8x-166.1x] | 0.17 | 9.52 | 0.15 | 9.35 | 0.019 | 0.184 | `OK` |
| `t2_cflp_49` | 49 | 13 | **30.4x** [19.1x-76.6x] | 0.45 | 11.22 | 0.40 | 10.99 | 0.039 | 0.225 | `OK` |
| `t2_gap_45` | 45 | 13 | **19.6x** [4.4x-68.1x] | 0.89 | 14.79 | 0.85 | 14.56 | 0.024 | 0.213 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **11.9x** [5.1x-31.9x] | 3.57 | 69.73 | 3.53 | 69.48 | 0.032 | 0.259 | `OK` |
| `t2_binpack_44` | 44 | 14 | **13.3x** [5.4x-25.0x] | 1.03 | 14.74 | 1.01 | 14.54 | 0.025 | 0.208 | `OK` |
| `t2_setcover_50` | 50 | 20 | **40.2x** [5.9x-114.4x] | 0.24 | 11.58 | 0.16 | 11.27 | 0.071 | 0.316 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **60.9x** [56.7x-67.0x] | 0.16 | 9.92 | 0.13 | 9.61 | 0.035 | 0.319 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **50.9x** [36.8x-65.6x] | 0.20 | 10.56 | 0.13 | 10.12 | 0.050 | 0.436 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **33.1x** [30.0x-36.1x] | 0.36 | 12.06 | 0.29 | 11.41 | 0.074 | 0.628 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.5x** [3.7x-26.1x] | 3.07 | 48.08 | 3.03 | 47.74 | 0.041 | 0.328 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **7.0x** [1.4x-30.4x] | 18.49 | 166.68 | 18.37 | 166.24 | 0.066 | 0.458 | `OK` |
| `t3_mkp_200` | 200 | 8 | **4.3x** [3.2x-5.6x] | 24.56 | 118.89 | 24.47 | 118.29 | 0.093 | 0.607 | `OK` |
| `t3_cflp_300` | 300 | 34 | **30.6x** [7.9x-60.2x] | 6.58 | 254.26 | 6.43 | 253.38 | 0.136 | 0.921 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **2.4x** [1.4x-4.4x] | 79.76 | 159.21 | 79.50 | 157.33 | 0.247 | 1.863 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **9.1x** [4.0x-14.0x] | 7.75 | 82.25 | 7.44 | 79.31 | 0.342 | 2.899 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **24.3x** [17.7x-29.0x] | 0.78 | 19.40 | 0.50 | 18.23 | 0.279 | 1.099 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **18.1x** [10.9x-24.0x] | 1.06 | 19.42 | 0.84 | 17.57 | 0.231 | 1.793 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **5.7x** [1.1x-13.8x] | 8.81 | 63.59 | 7.68 | 55.39 | 1.167 | 8.681 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **10.2x** [4.3x-24.3x] | 36.19 | 373.83 | 30.70 | 336.58 | 4.586 | 36.208 | `OK` |
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
- **Instances where AVX2 can run**: 2 faster, 5 slower, 16 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 1.05x [0.97x-1.14x] | 0.004 | 0.004 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source | 1.09x [0.99x-1.20x] | 0.003 | 0.003 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 0.98x [0.86x-1.11x] | 0.005 | 0.005 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.92x [0.87x-0.98x] | 0.004 | 0.005 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 0.97x [0.89x-1.06x] | 0.006 | 0.007 | 11 |
| `t1_furniture` | 5 | no (control) | identical source | 1.06x [0.98x-1.14x] | 0.018 | 0.017 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.24x [1.07x-1.44x] | 0.135 | 0.107 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.52x [1.10x-2.09x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | no detectable effect | 1.06x [0.77x-1.44x] | 0.060 | 0.050 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | no detectable effect | 1.00x [0.97x-1.03x] | 0.478 | 0.480 | 11 |
| `t2_mkp_30` | 36 | possible | no detectable effect | 0.93x [0.86x-1.02x] | 2.378 | 2.501 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 1.00x [0.97x-1.03x] | 5.287 | 5.322 | 11 |
| `t2_cflp_36` | 47 | possible | SIMD slower | 0.98x [0.96x-1.00x] | 0.154 | 0.164 | 11 |
| `t2_cflp_49` | 62 | possible | SIMD faster | 1.16x [1.11x-1.22x] | 0.400 | 0.324 | 11 |
| `t2_gap_45` | 58 | possible | no detectable effect | 0.97x [0.88x-1.07x] | 0.849 | 0.859 | 11 |
| `t2_lotsizing_48` | 80 | possible | SIMD slower | 0.93x [0.89x-0.98x] | 3.532 | 3.695 | 11 |
| `t2_binpack_44` | 58 | possible | SIMD slower | 0.96x [0.94x-0.98x] | 1.007 | 1.024 | 11 |
| `t2_setcover_50` | 70 | possible | SIMD faster | 1.08x [1.01x-1.16x] | 0.159 | 0.172 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 0.99x [0.94x-1.03x] | 0.128 | 0.132 | 11 |
| `t2_mad_g3_i9` | 108 | possible | no detectable effect | 0.90x [0.79x-1.02x] | 0.129 | 0.152 | 11 |
| `t2_mad_g4_i12` | 156 | possible | SIMD slower | 0.95x [0.93x-0.98x] | 0.289 | 0.301 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.00x [0.95x-1.05x] | 3.028 | 3.320 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | SIMD slower | 0.97x [0.95x-0.99x] | 18.374 | 17.870 | 11 |
| `t3_mkp_200` | 208 | possible | no detectable effect | 1.01x [0.96x-1.05x] | 24.467 | 24.236 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 0.99x [0.92x-1.06x] | 6.431 | 6.411 | 5 |
| `t3_lotsizing_504` | 840 | possible | no detectable effect | 0.98x [0.93x-1.03x] | 79.504 | 86.022 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 0.94x [0.84x-1.06x] | 7.439 | 9.097 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 0.99x [0.90x-1.08x] | 0.499 | 0.503 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 0.96x [0.90x-1.04x] | 0.835 | 0.833 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 0.97x [0.90x-1.04x] | 7.682 | 7.598 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | no detectable effect | 1.01x [0.94x-1.09x] | 30.704 | 29.187 | 5 |
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
| `t1_whiskas` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.00 (1) | 0.003 | 0.52 | 0.52 |
| `t1_furniture` | **0.02** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.003 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.13** (1 / 8) | 0.16 (2 / 8) | 0.20 (18) | 0.30 (35) | 0.013 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.002 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.06** (1 / 0) | 0.04 (1 / 0) | 0.05 (1) | 0.53 (4) | 0.085 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.48** (2 / 0) | 0.57 (12 / 0) | 0.57 (12) | 0.82 (109) | 0.111 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.38** (5 / 9) | 3.40 (40 / 9) | 1.21 (51) | 1.09 (61) | 0.028 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **5.29** (1 / 4) | 15.59 (2 / 4) | 5.96 (118) | 5.43 (111) | 0.047 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.15** (1 / 2) | 0.25 (3 / 2) | 0.18 (7) | 0.19 (7) | 0.026 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.40** (1 / 10) | 0.69 (4 / 10) | 0.34 (9) | 0.32 (9) | 0.038 | 909.02 | 827.30 |
| `t2_gap_45` | **0.85** (1 / 9) | 0.46 (1 / 9) | 1.87 (41) | 2.23 (60) | 0.029 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **3.53** (2 / 37) | 7.29 (25 / 37) | 3.26 (162) | 3.86 (199) | 0.073 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **1.01** (2 / 6) | 1.51 (5 / 6) | 9.43 (772) | 29.91 (1941) | 0.031 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.16** (1 / 6) | 0.37 (2 / 6) | 0.89 (6) | 0.69 (6) | 0.034 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.13** (1 / 0) | 0.89 (8 / 0) | 0.80 (8) | 0.77 (9) | 0.109 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.13** (1 / 0) | 0.17 (2 / 0) | 0.15 (2) | 2.84 (13) | 0.266 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.29** (1 / 0) | 3.47 (22 / 0) | 3.10 (22) | 5.38 (23) | 0.363 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.03** (43 / 0) | 3.21 (112 / 0) | 3.14 (112) | 3.74 (45) | 0.158 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **18.37** (196 / 0) | 35.21 (188 / 0) | 32.98 (188) | 99.62 (539) | 0.284 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **24.47** (250 / 14) | 109.67 (1135 / 16) | 230.55 (5000, `NodeLimit`) | 211.34 (5000, `NodeLimit`) | 0.131 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **6.43** (1 / 28) | 20.02 (54 / 34) | 9.23 (116) | 9.31 (118) | 0.195 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **79.50** (2 / 206) | 119.77 (376 / 206) | 250.04 (2798, `TimeLimit`) | 250.01 (2982, `TimeLimit`) | 0.759 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **7.44** (31 / 55) | 27.79 (87 / 53) | 250.12 (1712, `TimeLimit`) | 250.06 (1727, `TimeLimit`) | 0.225 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.50** (1 / 0) | 3.32 (17 / 0) | 2.86 (17) | 0.55 (1) | 0.699 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.84** (1 / 0) | 8.80 (58 / 0) | 8.40 (58) | 15.11 (74) | 1.502 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.68** (2 / 0) | 22.18 (25 / 0) | 20.78 (25) | 250.49 (145, `TimeLimit`) | 2.686 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **30.70** (1 / 0) | 33.27 (2 / 0) | 32.50 (2) | 250.26 (6, `TimeLimit`) | 58.578 | 208000.00 | 208000.00 |
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
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.54x** [1.22x-1.94x] | 1.54x | 4.73x | 0.213 (0.0191) | 0.325 (0.0295) | 1.59x [0.74x-2.59x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **2.94x** [1.90x-4.55x] | 2.94x | 2.99x | 9.409 (0.820) | 32.909 (2.988) | 2.62x [0.92x-15.54x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.88x** [1.84x-4.51x] | 2.88x | 3.66x | 212.751 (42.357) | 957.121 (188.913) | 2.58x [1.46x-6.32x] |
| Square families ($m = n$) | 16 (146) | **2.76x** [2.06x-3.71x] | 2.70x | 3.87x | 217.577 (42.352) | 979.246 (189.944) | 2.30x [0.92x-15.54x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.77x** [1.25x-2.52x] | 1.65x | 3.00x | 4.796 (0.843) | 11.109 (1.987) | 1.88x [0.74x-2.65x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.33x** [1.22x-4.42x] | 2.48x | 2.93x | 35.860 (6.556) | 65.222 (11.145) | 1.90x [0.98x-15.54x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.45x** [1.93x-3.10x] | 2.37x | 3.61x | 222.373 (43.196) | 990.355 (191.931) | 2.07x [0.74x-15.54x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 7.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.91x** | 0.0005 | 0.0009 | 0.0005 | 0.0116 | $0.0005 \pm 0.0001$ | $0.0009 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.55x** | 0.0016 | 0.0025 | 0.0019 | 0.0108 | $0.0016 \pm 0.0004$ | $0.0024 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.85x** | 0.0035 | 0.0064 | 0.0045 | 0.0155 | $0.0034 \pm 0.0007$ | $0.0064 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.17x** | 0.0036 | 0.0039 | 0.0044 | 0.0144 | $0.0034 \pm 0.0006$ | $0.0039 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.19x** | 0.0037 | 0.0045 | 0.0058 | 0.0154 | $0.0040 \pm 0.0013$ | $0.0046 \pm 0.0009$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.76x** | 0.0062 | 0.0114 | 0.0102 | 0.0339 | $0.0065 \pm 0.0019$ | $0.0112 \pm 0.0019$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.64x** | 0.0299 | 0.0701 | 0.0364 | 0.0959 | $0.0270 \pm 0.0065$ | $0.0699 \pm 0.0112$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.32x** | 0.0539 | 0.193 | 0.0713 | 0.244 | $0.0563 \pm 0.0155$ | $0.184 \pm 0.0268$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.26x** | 0.0469 | 0.0975 | 0.0697 | 0.150 | $0.0456 \pm 0.0041$ | $0.104 \pm 0.0223$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.66x** | 0.139 | 0.221 | 0.149 | 0.264 | $0.146 \pm 0.0482$ | $0.236 \pm 0.0445$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **11.01x** | 0.0649 | 0.685 | 0.0820 | 0.764 | $0.0643 \pm 0.0148$ | $0.691 \pm 0.0246$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.94x** | 0.0367 | 0.0699 | 0.0634 | 0.149 | $0.0384 \pm 0.0104$ | $0.0731 \pm 0.0081$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **1.94x** | 0.0398 | 0.0773 | 0.0653 | 0.152 | $0.0417 \pm 0.0081$ | $0.0800 \pm 0.0078$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.14x** | 0.0820 | 0.278 | 0.109 | 0.359 | $0.104 \pm 0.0749$ | $0.290 \pm 0.0375$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.82x** | 0.326 | 1.296 | 0.436 | 1.703 | $0.332 \pm 0.0483$ | $1.264 \pm 0.135$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.90x** | 1.315 | 5.211 | 1.762 | 7.747 | $1.336 \pm 0.112$ | $5.207 \pm 0.366$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.66x** | 0.895 | 1.494 | 1.462 | 4.158 | $0.882 \pm 0.0511$ | $1.464 \pm 0.0578$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.66x** | 5.400 | 8.630 | 5.599 | 10.457 | $5.705 \pm 2.089$ | $9.275 \pm 2.724$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.52x** | 0.355 | 0.826 | 0.748 | 3.871 | $0.343 \pm 0.0234$ | $0.864 \pm 0.0694$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.37x** | 0.405 | 1.006 | 0.828 | 3.975 | $0.424 \pm 0.0285$ | $1.003 \pm 0.0189$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **5.00x** | 5.917 | 29.055 | 7.949 | 38.047 | $5.878 \pm 0.348$ | $29.441 \pm 2.501$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **5.18x** | 28.070 | 142.692 | 37.202 | 189.971 | $27.984 \pm 3.921$ | $144.172 \pm 9.629$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 7.3 LSAP Scalar vs. `GOEXPERIMENT=simd`

In [`assign/solver.go`](../assign/solver.go), [`simd.AllAbsLE64With`](../internal/simd/kernels_amd64.go) and [`simd.ValidateAndNegate64With`](../internal/simd/assign_amd64.go) validate finite cost bounds via AVX2 checks whenever `len(cost) >= 32` (while the fused `colReduceValidateSquare` fast path skips `AllAbsLE64With` on finite square `Minimize` matrices and dispatches `ColReduceValidateRows64With` only for $n \ge 32$), [`simd.Transpose64With`](../internal/simd/assign_amd64.go) handles tall transposes with $\text{rows} \ge 8$ and $\text{cols} \ge 8$, [`simd.MinReducedSkip64With`](../internal/simd/assign_amd64.go) accelerates reduction-transfer slack scans in `preprocessSquareJV` ($n \ge 32$), [`simd.Min2ReducedRow64With`](../internal/simd/kernels_amd64.go) accelerates first/second reduced-minimum scans in `preprocessRectangularBV` and `augmentingRowReduction` at `minAVX2Min2Len = 256` (`minAVX512Min2Len = 128`), and [`simd.InitReducedRow64With`](../internal/simd/kernels_amd64.go) runs on a slice of length $\max(m, n) \ge 32$ at the start of each Dijkstra augmentation. All vector kernels clean upper `YMM` state via `StoreArray` + `ClearAVXUpperBits()` (`VZEROUPPER`) before scalar operations; exact per-family verdicts appear in the generated table below.

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` with no `>= 8x8` tall transpose and either fused square `Minimize` non-sparse path or `rows * cols < 32`, so no AVX2 kernel runs): 2 of 3 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32`, tall `rows > cols` with `>= 8x8`, or fallback `AllAbsLE64`/`ValidateAndNegate64` at `rows * cols >= 32`): 17 faster, 1 slower, 1 with no detectable effect, out of 19.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.11x [1.02x-1.22x] | 0.0005 | 0.0004 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.05x [1.04x-1.07x] | 0.0016 | 0.0015 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source | 0.98x [0.90x-1.06x] | 0.0035 | 0.0035 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | SIMD faster | 1.11x [1.09x-1.13x] | 0.0036 | 0.0033 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | SIMD faster | 1.18x [1.13x-1.23x] | 0.0037 | 0.0032 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | SIMD slower | 0.96x [0.94x-0.99x] | 0.0062 | 0.0065 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.24x [1.17x-1.31x] | 0.0299 | 0.0232 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.35x [1.24x-1.48x] | 0.0539 | 0.0420 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.54x [1.29x-1.83x] | 0.0469 | 0.0303 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.63x [1.46x-1.82x] | 0.139 | 0.0794 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.43x [1.28x-1.60x] | 0.0649 | 0.0454 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | no detectable effect | 1.20x [0.90x-1.59x] | 0.0367 | 0.0267 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | SIMD faster | 1.33x [1.09x-1.63x] | 0.0398 | 0.0295 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.57x [1.13x-2.19x] | 0.0820 | 0.0547 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.65x [1.54x-1.76x] | 0.326 | 0.191 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 1.86x [1.71x-2.02x] | 1.315 | 0.734 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.12x [1.14x-3.94x] | 0.895 | 0.335 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.68x [2.56x-2.80x] | 5.400 | 1.899 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.89x [1.77x-2.02x] | 0.355 | 0.188 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.83x [1.56x-2.15x] | 0.405 | 0.223 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.79x [1.68x-1.91x] | 5.917 | 3.271 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.43x [1.38x-1.49x] | 28.070 | 18.821 | 5 |
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
| Tier 1: Micro-models (`N <= 26`) | 10 (110) | **18.06x** [8.24x-39.60x] | **39.45x** [13.23x-117.68x] | 9.9 (0.9) | 158.3 (14.8) | 13.04x [2.92x-146.63x] |
| Tier 2: Medium MILPs (`30 <= N <= 84`) | 13 (143) | **5.90x** [4.32x-8.06x] | **6.61x** [4.56x-9.58x] | 548.2 (36.3) | 1844.1 (162.8) | 6.04x [0.62x-58.38x] |
| Tier 3: Large MILPs (`145 <= N <= 4036`) | 8 (40) | **3.85x** [2.24x-6.63x] | **4.30x** [2.36x-7.84x] | 889.6 (165.5) | 2633.0 (537.5) | 3.89x [0.43x-13.14x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **4.85x** [3.32x-7.10x] | **5.15x** [3.38x-7.84x] | 1118.7 (154.3) | 3117.3 (405.9) | 5.52x [0.62x-58.38x] |
| All 31 synthetic families | 31 (293) | **7.59x** [5.29x-10.88x] | **10.53x** [6.38x-17.37x] | 1447.6 (202.7) | 4635.5 (715.1) | 7.67x [0.43x-146.63x] |
<!-- END GENERATED:CBC_SUMMARY_TABLE -->

### 9.1 MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families ($N = G(I + T + 3)$ variables, $M = G(2T + 3) + I$ constraints) separate solver behavior into three scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Paired `Go / CBC` | Go Total Med (ms) | CBC Total Med (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.09x** | 0.11 | 1.20 | 0.51x | 0.06 (1 / 19) | 1.16 (0 / 0) | 0.085 (28) | 0.166 (19) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.06x** | 0.51 | 7.67 | 0.70x | 0.48 (2 / 72) | 7.62 (14 / 44) | 0.111 (30) | 0.159 (18) |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.13x** | 0.16 | 1.30 | 0.73x | 0.13 (1 / 26) | 1.24 (0 / 0) | 0.109 (33) | 0.150 (21) |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.12x** | 0.20 | 1.69 | 1.01x | 0.13 (1 / 40) | 1.63 (0 / 0) | 0.266 (58) | 0.263 (40) |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.15x** | 0.36 | 2.12 | 0.84x | 0.29 (1 / 51) | 2.00 (0 / 0) | 0.363 (80) | 0.430 (63) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.31x** | 3.07 | 11.22 | 1.00x | 3.03 (43 / 483) | 11.15 (28 / 146) | 0.158 (38) | 0.157 (24) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.29x** | 18.49 | 85.10 | 0.92x | 18.37 (196 / 2289) | 85.03 (216 / 1352) | 0.284 (65) | 0.307 (54) |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.13x** | 0.78 | 6.13 | 1.06x | 0.50 (1 / 75) | 5.97 (0 / 0) | 0.699 (91) | 0.659 (94) |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.27x** | 1.06 | 4.02 | 1.33x | 0.84 (1 / 98) | 3.79 (0 / 0) | 1.502 (244) | 1.130 (149) |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **0.43x** | 8.81 | 28.07 | 0.71x | 7.68 (2 / 377) | 25.91 (0 / 0) | 2.686 (224) | 3.809 (319) |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **0.22x** | 36.19 | 251.04 | 1.59x | 30.70 (1 / 2439) | 246.42 (0 / 0) | 58.578 (1900) | 36.765 (3128) |
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
| `t3_mkp_200` | $200 \times 8$ | **0.52x** | $24.12 \pm 12.85$ | $43.51 \pm 20.60$ | 24.47 (250 / 2860) | 51.70 (154 / 1145) | 109.67 (1135 / 16) | 230.55 (5000, `NodeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **0.42x** | $7.60 \pm 8.41$ | $12.97 \pm 9.36$ | 5.29 (1 / 20) | 9.03 (0 / 8) | 15.59 (2 / 4) | 5.96 (118) |
| `t2_binpack_44` | $44 \times 14$ | **0.16x** | $1.29 \pm 0.86$ | $6.92 \pm 2.33$ | 1.01 (2 / 43) | 6.39 (0 / 8) | 1.51 (5 / 6) | 9.43 (772) |
| `t3_cflp_300` | $300 \times 34$ | **0.12x** | $8.56 \pm 5.57$ | $60.51 \pm 12.09$ | 6.43 (1 / 119) | 57.90 (18 / 483) | 20.02 (54 / 34) | 9.23 (116) |
| `t3_lotsizing_504` | $504 \times 336$ | **0.65x** | $72.63 \pm 47.29$ | $99.19 \pm 61.22$ | 79.50 (2 / 1351) | 76.52 (10 / 377) | 119.77 (376 / 206) | 250.04 (2798, `TimeLimit`) |
| `t3_sched_1000` | $1000 \times 100$ | **0.15x** | $10.44 \pm 5.97$ | $63.53 \pm 4.71$ | 7.44 (31 / 917) | 61.36 (0 / 76) | 27.79 (87 / 53) | 250.12 (1712, `TimeLimit`) |
<!-- END GENERATED:NON_MAD_CBC_GAP_TABLE -->

<!-- BEGIN GENERATED:NON_MAD_CBC_NOTES -->
- **Bin-packing tail behavior on `t2_binpack_44`**: pure-Go `BackendGo` ($1.29 \pm 0.86$ ms mean, 1.03 ms median) leads in-process `BackendCBC` ($6.92 \pm 2.33$ ms mean, 6.43 ms median) with a **6.11x** `BackendCBC / BackendGo` geometric-mean speedup (`Go / CBC = 0.16x`), running slower on 0 of 11 trials (peaking at 3.17 ms vs. 12.63 ms), with `CP5 +Presolve` at 9.43 ms vs. `CP6 +Cuts` at 1.51 ms.
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
| PMAD base (`G = 10, I = 2`) | 1 (11) | 0.264 (0.0245) | 0.103 (0.0094) | 909.5 (15.8) [8/11] | 737.1 (64.6) [11/11] | **1151.4x** | **2533.4x** |
| PMAD small & medium (`I <= 10`) | 6 (66) | 99.973 (9.057) | 99.178 (8.962) | 15727.5 (1500.7) [8/66] | 12783.7 (1169.8) [27/66] | **440.4x** [73.7x-2632.2x] | **317.4x** [80.8x-1246.2x] |
| PMAD large (`I = 12`) | 2 (10) | 476.537 (81.711) | 460.353 (79.088) | 2502.5 (500.4) [0/10] | 2561.4 (510.2) [0/10] | **6.6x** [0.0x-30069.4x] | **6.8x** [0.0x-25474.6x] |
| Random-weight MAD controls (`I <= 12`) | 4 (44) | 6.311 (0.543) | 6.005 (0.522) | 2594.6 (278.1) [37/44] | 4485.9 (418.0) [33/44] | **156.2x** [45.0x-541.5x] | **799.1x** [349.6x-1826.5x] |
| **All 13 combinatorial MAD families** | 13 (131) | 583.086 (91.336) | 565.640 (88.581) | 21734.2 (2295.0) [53/131] | 20568.1 (2162.6) [71/131] | **180.7x** [54.4x-600.3x] | **273.7x** [82.6x-907.3x] |
<!-- END GENERATED:MAD_COMBINATORIAL_SUMMARY_TABLE -->

<!-- BEGIN GENERATED:MAD_COMBINATORIAL_PIPELINE_TABLE -->
| Problem ID | $G \times I \times T$ | $\lambda$ | Auto Strategy | Auto Med (ms) | DP Med (ms) | DFS Med (ms) | MILP Go 250 ms | MILP CBC 250 ms | Trial 0 Obj | Parity |
| :--- | :---: | ---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `pmad_base_g10_i2` | $10 \times 2 \times 6$ | 1.00 | `DP` | 0.0245 | 0.0094 | 0.0922 | 15.76 (8/11 opt) | 64.60 (`Optimal`) | 24137.0 | `OK` |
| `pmad_g6_i6_lam025` | $6 \times 6 \times 6$ | 0.25 | `DP` | 0.101 | 0.0872 | 27.061 | 250.04 (4/11 opt) | 103.73 (`Optimal`) | 13259.5 | `OK` |
| `pmad_g6_i10_lam10` | $6 \times 10 \times 6$ | 1.00 | `DP` | 2.890 | 2.918 | skip (>250k) | 250.07 (`TimeLimit`) | 251.93 (1/11 opt) | 14018.0 | `OK` |
| `pmad_g6_i12_lam025` | $6 \times 12 \times 6$ | 0.25 | `DP` | 19.322 | 19.407 | skip (>250k) | 250.15 (`TimeLimit`) | 251.93 (`TimeLimit`) | 9787.0 | `OK` |
| `pmad_g10_i4_lam025` | $10 \times 4 \times 6$ | 0.25 | `DP` | 0.0490 | 0.0344 | 8.814 | 250.09 (4/11 opt) | 53.79 (`Optimal`) | 23954.0 | `OK` |
| `pmad_g10_i8_lam10` | $10 \times 8 \times 6$ | 1.00 | `DP` | 0.763 | 0.744 | skip (>250k) | 250.11 (`TimeLimit`) | 252.58 (3/11 opt) | 21928.0 | `OK` |
| `pmad_g10_i10_lam025` | $10 \times 10 \times 6$ | 0.25 | `DP` | 4.884 | 4.835 | skip (>250k) | 250.17 (`TimeLimit`) | 252.42 (1/11 opt) | 22570.0 | `OK` |
| `pmad_g20_i6_lam10` | $20 \times 6 \times 6$ | 1.00 | `DP` | 0.370 | 0.342 | skip (>250k) | 250.20 (`TimeLimit`) | 255.33 (`TimeLimit`) | 43530.0 | `OK` |
| `pmad_g20_i12_lam10` | $20 \times 12 \times 6$ | 1.00 | `DP` | 62.389 | 59.681 | skip (>250k) | 250.27 (`TimeLimit`) | 258.28 (`TimeLimit`) | 38256.0 | `OK` |
| `mad_comb_rand_g2_i4` | $2 \times 4 \times 6$ | 0.00 | `BruteForce` | 0.0115 | 0.0053 | 0.0033 | 0.70 (`Optimal`) | 14.33 (`Optimal`) | 4601.0 | `OK` |
| `mad_comb_rand_g2_i8` | $2 \times 8 \times 6$ | 0.00 | `BruteForce` | 0.0358 | 0.0413 | 0.0247 | 4.48 (`Optimal`) | 18.52 (`Optimal`) | 5975.0 | `OK` |
| `mad_comb_rand_g3_i9` | $3 \times 9 \times 6$ | 0.00 | `DP` | 0.0691 | 0.0615 | 0.354 | 22.83 (`Optimal`) | 133.95 (`Optimal`) | 7720.0 | `OK` |
| `mad_comb_rand_g4_i12` | $4 \times 12 \times 6$ | 0.00 | `DP` | 0.426 | 0.414 | 36.549 | 250.09 (4/11 opt) | 251.18 (`TimeLimit`) | 7905.0 | `OK` |
<!-- END GENERATED:MAD_COMBINATORIAL_PIPELINE_TABLE -->
