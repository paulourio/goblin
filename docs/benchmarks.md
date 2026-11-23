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
- **Unified telemetry source**: fingerprint `f9016b5ee7cf4cd3` (working tree on top of commit `98fc925725e2d5fd4a0c84f266e02e80f8e2ece3`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.92 ms median on a 1-variable MILP and 6.84 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
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
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **392.08x** [185.01x-830.92x] | 392.08x | 0.04x | 8.7 (0.7) | 194.8 (19.4) | 1097.2 (103.1) | 570.60x [38.12x-3306.08x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **17.82x** [9.54x-33.30x] | 17.82x | 0.17x | 723.1 (44.0) | 1965.9 (173.1) | 4352.1 (420.6) | 24.81x [0.14x-148.88x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **10.38x** [5.32x-20.26x] | 10.38x | 0.20x | 781.4 (140.3) | 2905.2 (563.0) | 6224.2 (1067.3) | 10.30x [1.11x-73.14x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **11.66x** [6.61x-20.55x] | 12.47x | 0.22x | 1196.9 (140.2) | 3131.4 (400.6) | 6811.8 (989.3) | 12.41x [0.14x-148.88x] |
| Tier 3 non-MAD MILPs | 4 (20) | **8.11x** [1.80x-36.50x] | 8.11x | 0.27x | 482.3 (97.0) | 1252.9 (234.9) | 2811.2 (600.0) | 6.22x [1.87x-73.14x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **28.69x** [12.01x-68.56x] | 38.31x | 0.10x | 308.3 (44.1) | 1764.2 (338.0) | 3872.7 (508.7) | 41.29x [1.11x-189.75x] |
| Random-weight MAD (negative controls) | 3 (33) | **16.39x** [0.89x-301.71x] | 16.39x | 0.14x | 298.0 (20.2) | 1220.2 (118.3) | 2261.1 (248.8) | 15.16x [1.41x-122.28x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **47.98x** [19.70x-116.81x] | 57.80x | 0.12x | 1204.8 (140.9) | 3301.7 (417.5) | 7800.9 (1082.4) | 43.82x [0.14x-3306.08x] |
| All 31 synthetic corpus families | 31 (293) | **42.02x** [21.34x-82.72x] | 52.83x | 0.11x | 1513.2 (185.0) | 5066.0 (755.5) | 11673.6 (1591.0) | 41.74x [0.14x-3306.08x] |
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
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **159.6x** [123.8x-189.8x] | 0.06 | 9.97 | 0.04 | 9.72 | 0.020 | 0.234 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **62.8x** [38.1x-122.3x] | 0.49 | 33.57 | 0.46 | 33.34 | 0.031 | 0.241 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **59.7x** [53.4x-66.2x] | 0.16 | 9.47 | 0.13 | 9.18 | 0.034 | 0.288 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **54.8x** [40.8x-68.3x] | 0.17 | 10.08 | 0.13 | 9.65 | 0.045 | 0.413 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **28.3x** [24.7x-34.9x] | 0.43 | 11.78 | 0.32 | 11.16 | 0.100 | 0.622 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **9.7x** [3.6x-26.0x] | 3.27 | 47.85 | 3.21 | 47.55 | 0.043 | 0.308 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **7.2x** [1.4x-30.0x] | 16.43 | 167.34 | 16.37 | 166.89 | 0.059 | 0.435 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **27.8x** [21.2x-32.4x] | 0.62 | 18.31 | 0.47 | 17.21 | 0.154 | 1.020 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **17.8x** [11.1x-23.3x] | 1.02 | 18.52 | 0.81 | 16.80 | 0.210 | 1.744 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **6.0x** [1.1x-15.0x] | 8.16 | 60.64 | 7.24 | 52.79 | 1.001 | 8.259 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **10.4x** [4.6x-25.1x] | 33.44 | 369.88 | 29.77 | 335.98 | 3.871 | 35.031 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

### MAD Cross-Instance Latency Dispersion Across Randomized Instances (`Mean ± SD [95% CI]`)

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.06 \pm 0.01$ ($\pm 0.00$) | $9.83 \pm 0.54$ ($\pm 0.36$) | $0.07 \pm 0.01$ ($\pm 0.00$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.53 \pm 0.25$ ($\pm 0.17$) | $30.81 \pm 9.53$ ($\pm 6.40$) | $0.49 \pm 0.22$ ($\pm 0.15$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.16 \pm 0.01$ ($\pm 0.01$) | $9.67 \pm 0.46$ ($\pm 0.31$) | $0.16 \pm 0.01$ ($\pm 0.01$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.19 \pm 0.03$ ($\pm 0.02$) | $10.21 \pm 0.32$ ($\pm 0.21$) | $0.20 \pm 0.07$ ($\pm 0.04$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.43 \pm 0.02$ ($\pm 0.02$) | $12.08 \pm 0.66$ ($\pm 0.44$) | $0.38 \pm 0.05$ ($\pm 0.03$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $4.14 \pm 3.06$ ($\pm 2.06$) | $35.82 \pm 17.02$ ($\pm 11.43$) | $4.13 \pm 3.05$ ($\pm 2.05$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $22.42 \pm 22.08$ ($\pm 14.83$) | $138.93 \pm 63.64$ ($\pm 42.75$) | $22.25 \pm 22.07$ ($\pm 14.83$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.64 \pm 0.03$ ($\pm 0.03$) | $17.80 \pm 2.44$ ($\pm 3.03$) | $0.65 \pm 0.05$ ($\pm 0.06$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.08 \pm 0.35$ ($\pm 0.44$) | $18.53 \pm 0.13$ ($\pm 0.16$) | $1.06 \pm 0.37$ ($\pm 0.46$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $16.54 \pm 21.14$ ($\pm 26.24$) | $62.79 \pm 3.78$ ($\pm 4.70$) | $16.49 \pm 20.27$ ($\pm 25.17$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $41.56 \pm 14.74$ ($\pm 18.29$) | $583.48 \pm 618.09$ ($\pm 767.34$) | $39.15 \pm 14.85$ ($\pm 18.43$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### MAD Solver-Core Checkpoint Progression

Checkpoints `CP4 Base` (branch-and-bound without presolve, cuts, or heuristics), `CP5 +Presolve`, and `CP6 +Cuts` run under a 250 ms time limit and a 5,000-node cap; checkpoints that stop at a limit show the limit status.

<!-- BEGIN GENERATED:MAD_CHECKPOINT_TABLE -->
| Problem ID | CP7 Full (ms / nodes / cuts) | CP6 +Cuts (ms / nodes / cuts) | CP5 +Presolve (ms / nodes) | CP4 Base (ms / nodes) | Root LP (ms) | Trial 0 Obj | Root LP Obj |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | **0.04** (1 / 0) | 0.04 (1 / 0) | 0.06 (1) | 0.53 (4) | 0.086 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.46** (2 / 0) | 0.57 (12 / 0) | 0.55 (12) | 0.78 (109) | 0.101 | 4601.00 | 4257.00 |
| `t2_mad_g2_i8` | **0.13** (1 / 0) | 0.82 (8 / 0) | 0.73 (8) | 0.72 (9) | 0.101 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.13** (1 / 0) | 0.13 (2 / 0) | 0.14 (2) | 2.73 (13) | 0.244 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.32** (1 / 0) | 3.23 (22 / 0) | 2.99 (22) | 5.24 (23) | 0.365 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.21** (43 / 0) | 2.93 (112 / 0) | 2.95 (112) | 3.61 (45) | 0.151 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **16.37** (201 / 0) | 30.25 (188 / 0) | 30.22 (188) | 92.78 (539) | 0.276 | 7720.00 | 6060.00 |
| `t3_mad_g5_i20` | **0.47** (1 / 0) | 2.98 (17 / 0) | 2.65 (17) | 0.53 (1) | 0.671 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.81** (1 / 0) | 8.84 (58 / 0) | 7.66 (58) | 12.99 (74) | 1.643 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.24** (2 / 0) | 21.57 (25 / 0) | 19.26 (25) | 250.02 (151, `TimeLimit`) | 2.812 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **29.77** (1 / 0) | 32.37 (2 / 0) | 31.02 (2) | 250.05 (6, `TimeLimit`) | 56.479 | 208000.00 | 208000.00 |
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
| `t1_std_lp` | 3 | 3 | **508.6x** [141.8x-762.6x] | 0.01 | 6.34 | 0.00 | 6.29 | 0.005 | 0.054 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **504.2x** [265.9x-827.3x] | 0.01 | 6.32 | 0.00 | 6.27 | 0.004 | 0.053 | `OK` |
| `t1_std_mip` | 3 | 3 | **652.8x** [92.2x-1268.7x] | 0.01 | 7.96 | 0.01 | 7.89 | 0.004 | 0.067 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **913.3x** [600.7x-1102.6x] | 0.01 | 7.51 | 0.00 | 7.45 | 0.003 | 0.061 | `OK` |
| `t1_whiskas` | 6 | 5 | **627.9x** [496.7x-790.0x] | 0.01 | 6.73 | 0.00 | 6.63 | 0.005 | 0.089 | `OK` |
| `t1_furniture` | 2 | 3 | **337.0x** [98.9x-1114.8x] | 0.02 | 8.56 | 0.01 | 8.49 | 0.003 | 0.068 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **119.6x** [55.6x-396.3x] | 0.10 | 9.56 | 0.09 | 9.45 | 0.007 | 0.101 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **2214.3x** [1331.1x-3306.1x] | 0.00 | 6.54 | 0.00 | 6.49 | 0.002 | 0.053 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **159.6x** [123.8x-189.8x] | 0.06 | 9.97 | 0.04 | 9.72 | 0.020 | 0.234 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **62.8x** [38.1x-122.3x] | 0.49 | 33.57 | 0.46 | 33.34 | 0.031 | 0.241 | `OK` |
| `t2_mkp_30` | 30 | 6 | **11.9x** [6.3x-17.1x] | 2.15 | 28.20 | 2.13 | 28.02 | 0.018 | 0.180 | `OK` |
| `t2_mkp_50` | 50 | 8 | **6.0x** [3.0x-32.6x] | 4.09 | 17.19 | 4.06 | 16.94 | 0.030 | 0.257 | `OK` |
| `t2_cflp_36` | 36 | 11 | **50.9x** [16.9x-148.9x] | 0.20 | 9.72 | 0.15 | 9.54 | 0.020 | 0.185 | `OK` |
| `t2_cflp_49` | 49 | 13 | **34.1x** [19.3x-83.4x] | 0.33 | 10.41 | 0.30 | 10.20 | 0.034 | 0.218 | `OK` |
| `t2_gap_45` | 45 | 13 | **23.4x** [4.6x-79.8x] | 0.79 | 14.44 | 0.77 | 14.21 | 0.021 | 0.204 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **12.0x** [5.3x-28.2x] | 4.11 | 68.78 | 4.08 | 68.52 | 0.026 | 0.244 | `OK` |
| `t2_binpack_44` | 44 | 14 | **2.0x** [0.1x-29.5x] | 11.59 | 14.23 | 11.56 | 14.03 | 0.028 | 0.210 | `OK` |
| `t2_setcover_50` | 50 | 20 | **41.5x** [6.7x-140.2x] | 0.25 | 11.14 | 0.22 | 10.83 | 0.033 | 0.311 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **59.7x** [53.4x-66.2x] | 0.16 | 9.47 | 0.13 | 9.18 | 0.034 | 0.288 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **54.8x** [40.8x-68.3x] | 0.17 | 10.08 | 0.13 | 9.65 | 0.045 | 0.413 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **28.3x** [24.7x-34.9x] | 0.43 | 11.78 | 0.32 | 11.16 | 0.100 | 0.622 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.7x** [3.6x-26.0x] | 3.27 | 47.85 | 3.21 | 47.55 | 0.043 | 0.308 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **7.2x** [1.4x-30.0x] | 16.43 | 167.34 | 16.37 | 166.89 | 0.059 | 0.435 | `OK` |
| `t3_mkp_200` | 200 | 8 | **4.9x** [4.0x-6.6x] | 17.90 | 115.84 | 17.83 | 115.23 | 0.076 | 0.618 | `OK` |
| `t3_cflp_300` | 300 | 34 | **26.6x** [8.0x-73.1x] | 9.35 | 247.47 | 9.25 | 246.57 | 0.116 | 0.905 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **3.1x** [1.9x-5.0x] | 63.13 | 156.79 | 62.93 | 154.93 | 0.278 | 1.860 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **10.7x** [5.3x-17.5x] | 6.64 | 79.87 | 6.34 | 76.94 | 0.282 | 2.776 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **27.8x** [21.2x-32.4x] | 0.62 | 18.31 | 0.47 | 17.21 | 0.154 | 1.020 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **17.8x** [11.1x-23.3x] | 1.02 | 18.52 | 0.81 | 16.80 | 0.210 | 1.744 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **6.0x** [1.1x-15.0x] | 8.16 | 60.64 | 7.24 | 52.79 | 1.001 | 8.259 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **10.4x** [4.6x-25.1x] | 33.44 | 369.88 | 29.77 | 335.98 | 3.871 | 35.031 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

---

## 5. Default Build vs. `GOEXPERIMENT=simd`

In [`internal/simd/kernels_amd64.go`](../internal/simd/kernels_amd64.go) and [`internal/simd/kernels_avx512_amd64.go`](../internal/simd/kernels_avx512_amd64.go), runtime CPUID dispatch selects AVX-512, AVX2, or 4-way scalar loops from a single portable `CGO_ENABLED=0` binary (`GOAMD64=v3` FMA-safe). Each vector kernel spills accumulators via `StoreArray` and executes `archsimd.ClearAVXUpperBits()` (`VZEROUPPER`) before scalar tail or caller operations, eliminating AVX-to-SSE transition stalls while preserving IEEE-754 `NaN` propagation parity. AVX2 activates at `minAVX2Len = 32` (`Dot64`, `Axpy64`, `Scale64`, `AbsMax64`, `RowActivityBoundsFinite`, `SelectDualLeaveRow64With`, `MinReducedSkip64With`, `InitReducedRow64`, `AllAbsLE64With`, `ValidateAndNegate64With`), `rows >= 8 && cols >= 8` for `Transpose64With`, `minDualRatioSIMDChunks = 4` (256 cols for `ScanDualRatioCand64With`), and `minAVX2Min2Len = 256` (`Min2ReducedRow64`); on AVX-512 hosts, dense LSAP loops dispatch AVX-512 at `minAVX512Len = 64` and `minAVX512Min2Len = 128` while sparse LP kernels gate AVX-512 at `minAVX512LPLen = 2048` / `minLeaveRowAVX512 = 2048` / `minDualRatioAVX512Chunks = 128` (8,192 cols) to avoid Intel Emerald Rapids License-2 clock down-binning across surrounding scalar LU code.

At the full-solver level, each ratio below is the geometric mean of per-trial `scalar / SIMD` solve times over the same instances on the local AVX2 host (`i9-13900K`), with a 95% $t$-interval on the log ratios; "no detectable effect" means the interval contains 1. "Reachable" is a necessary condition: the AVX2 LP kernels require `n + m >= 32`. Below that size no vector kernel can run, so those small models serve as negative controls where intervals excluding 1 reflect binary code-layout or run-to-run variance rather than SIMD execution. Per-family paired speedups and 95% intervals for larger models appear in the generated table below.

### 5.1 Scope and Pending Target-Hardware Verification

All telemetry in this document is measured locally on the AVX2-only `13th Gen Intel(R) Core(TM) i9-13900K` host (`x86_64`, Linux). Portable `GOOS=linux GOARCH=arm64 CGO_ENABLED=0` and `GOAMD64=v3`/`v4` builds compile and pass local build checks, whereas fresh physical AVX-512 and `linux/arm64` runtime test and benchmark re-verification on the current tree remain pending target hardware access.

<!-- BEGIN GENERATED:SIMD_SUMMARY -->
- **Negative controls** (`n + m < 32`, no AVX2 kernel can run): 4 of 8 have a 95% interval excluding 1. Observed shifts on these micro-models reflect binary code layout and run-to-run variance under `GOEXPERIMENT=simd`, not vector execution.
- **Instances where AVX2 can run**: 4 faster, 6 slower, 13 with no detectable effect, out of 23.
<!-- END GENERATED:SIMD_SUMMARY -->

<!-- BEGIN GENERATED:SIMD_EFFECT_TABLE -->
| Problem ID | `n + m` | AVX2 `Dot64`/`Axpy64` reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `t1_std_lp` | 6 | no (control) | identical source | 1.33x [0.82x-2.16x] | 0.005 | 0.004 | 11 |
| `t1_std_lp_max` | 6 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.37x [1.10x-1.72x] | 0.005 | 0.003 | 11 |
| `t1_std_mip` | 6 | no (control) | identical source | 1.22x [0.72x-2.08x] | 0.006 | 0.006 | 11 |
| `t1_mip_float_obj` | 6 | no (control) | identical source | 0.97x [0.76x-1.25x] | 0.004 | 0.004 | 11 |
| `t1_whiskas` | 11 | no (control) | identical source | 0.93x [0.84x-1.02x] | 0.005 | 0.005 | 11 |
| `t1_furniture` | 5 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.82x [0.75x-0.90x] | 0.015 | 0.018 | 11 |
| `t1_knapsack_12` | 15 | no (control) | identical source; interval excludes 1 (build/run effect) | 0.79x [0.73x-0.86x] | 0.091 | 0.110 | 11 |
| `t1_infeasible_mip` | 3 | no (control) | identical source; interval excludes 1 (build/run effect) | 1.36x [1.03x-1.80x] | 0.000 | 0.000 | 11 |
| `t1_mad_g2_i4` | 60 | possible | SIMD slower | 0.92x [0.88x-0.96x] | 0.038 | 0.044 | 11 |
| `t1_mad_rand_g2_i4` | 60 | possible | SIMD faster | 1.05x [1.01x-1.09x] | 0.462 | 0.449 | 11 |
| `t2_mkp_30` | 36 | possible | SIMD slower | 0.95x [0.94x-0.96x] | 2.133 | 2.145 | 11 |
| `t2_mkp_50` | 58 | possible | no detectable effect | 1.00x [0.95x-1.05x] | 4.063 | 4.365 | 11 |
| `t2_cflp_36` | 47 | possible | SIMD slower | 0.86x [0.80x-0.93x] | 0.155 | 0.180 | 11 |
| `t2_cflp_49` | 62 | possible | no detectable effect | 1.08x [0.98x-1.19x] | 0.296 | 0.311 | 11 |
| `t2_gap_45` | 58 | possible | SIMD slower | 0.95x [0.91x-1.00x] | 0.765 | 0.751 | 11 |
| `t2_lotsizing_48` | 80 | possible | SIMD slower | 0.92x [0.88x-0.97x] | 4.084 | 4.427 | 11 |
| `t2_binpack_44` | 58 | possible | SIMD slower | 0.94x [0.90x-0.98x] | 11.563 | 11.938 | 11 |
| `t2_setcover_50` | 70 | possible | no detectable effect | 0.99x [0.92x-1.05x] | 0.225 | 0.225 | 11 |
| `t2_mad_g2_i8` | 72 | possible | no detectable effect | 1.00x [0.97x-1.03x] | 0.129 | 0.128 | 11 |
| `t2_mad_g3_i9` | 108 | possible | no detectable effect | 0.96x [0.86x-1.07x] | 0.127 | 0.128 | 11 |
| `t2_mad_g4_i12` | 156 | possible | SIMD faster | 1.09x [1.01x-1.16x] | 0.325 | 0.288 | 11 |
| `t2_mad_rand_g2_i8` | 72 | possible | no detectable effect | 1.00x [0.97x-1.04x] | 3.211 | 2.965 | 11 |
| `t2_mad_rand_g3_i9` | 108 | possible | no detectable effect | 1.01x [0.99x-1.02x] | 16.369 | 16.320 | 11 |
| `t3_mkp_200` | 208 | possible | SIMD faster | 1.06x [1.05x-1.07x] | 17.825 | 16.821 | 5 |
| `t3_cflp_300` | 334 | possible | no detectable effect | 1.00x [0.91x-1.11x] | 9.253 | 8.498 | 5 |
| `t3_lotsizing_504` | 840 | possible | no detectable effect | 1.02x [0.99x-1.05x] | 62.929 | 59.771 | 5 |
| `t3_sched_1000` | 1100 | possible | no detectable effect | 1.02x [0.99x-1.04x] | 6.344 | 6.237 | 5 |
| `t3_mad_g5_i20` | 240 | possible | no detectable effect | 0.97x [0.88x-1.08x] | 0.468 | 0.498 | 5 |
| `t3_mad_g6_i30` | 354 | possible | no detectable effect | 1.02x [0.99x-1.05x] | 0.810 | 0.817 | 5 |
| `t3_mad_g10_i100` | 1340 | possible | no detectable effect | 0.98x [0.89x-1.08x] | 7.245 | 7.416 | 5 |
| `t3_mad_g4_i1000` | 5096 | possible | SIMD faster | 1.09x [1.05x-1.12x] | 29.771 | 27.313 | 5 |
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
| `t1_whiskas` | **0.00** (1 / 0) | 0.00 (1 / 0) | 0.00 (1) | 0.00 (1) | 0.003 | 0.52 | 0.52 |
| `t1_furniture` | **0.01** (1 / 0) | 0.01 (1 / 0) | 0.01 (1) | 0.01 (1) | 0.002 | 1075.00 | 1075.00 |
| `t1_knapsack_12` | **0.09** (1 / 8) | 0.11 (2 / 8) | 0.14 (18) | 0.21 (35) | 0.007 | 513.24 | 578.65 |
| `t1_infeasible_mip` | **0.00** (0 / 0) | 0.00 (0 / 0) | 0.00 (0) | 0.00 (0) | 0.001 | 0.00 | 1.50 |
| `t1_mad_g2_i4` | **0.04** (1 / 0) | 0.04 (1 / 0) | 0.06 (1) | 0.53 (4) | 0.086 | 2400.00 | 2400.00 |
| `t1_mad_rand_g2_i4` | **0.46** (2 / 0) | 0.57 (12 / 0) | 0.55 (12) | 0.78 (109) | 0.101 | 4601.00 | 4257.00 |
| `t2_mkp_30` | **2.13** (5 / 9) | 2.92 (40 / 9) | 1.03 (51) | 0.92 (61) | 0.025 | 1917.24 | 1966.92 |
| `t2_mkp_50` | **4.06** (1 / 4) | 11.51 (2 / 4) | 4.69 (118) | 4.17 (111) | 0.038 | 2620.01 | 2648.37 |
| `t2_cflp_36` | **0.15** (1 / 2) | 0.23 (3 / 2) | 0.18 (7) | 0.19 (7) | 0.027 | 801.17 | 781.97 |
| `t2_cflp_49` | **0.30** (1 / 7) | 0.59 (3 / 7) | 0.30 (9) | 0.28 (9) | 0.033 | 909.02 | 827.30 |
| `t2_gap_45` | **0.77** (1 / 7) | 0.41 (1 / 7) | 1.82 (41) | 1.99 (60) | 0.025 | 195.18 | 178.04 |
| `t2_lotsizing_48` | **4.08** (2 / 37) | 7.83 (25 / 37) | 2.99 (162) | 3.28 (199) | 0.059 | 2245.81 | 2052.34 |
| `t2_binpack_44` | **11.56** (274 / 12) | 13.84 (298 / 12) | 8.28 (772) | 26.55 (1941) | 0.029 | 30.32 | 29.30 |
| `t2_setcover_50` | **0.22** (1 / 6) | 0.33 (2 / 6) | 0.67 (6) | 0.87 (6) | 0.031 | 58.10 | 56.84 |
| `t2_mad_g2_i8` | **0.13** (1 / 0) | 0.82 (8 / 0) | 0.73 (8) | 0.72 (9) | 0.101 | 2400.00 | 2400.00 |
| `t2_mad_g3_i9` | **0.13** (1 / 0) | 0.13 (2 / 0) | 0.14 (2) | 2.73 (13) | 0.244 | 4800.00 | 4800.00 |
| `t2_mad_g4_i12` | **0.32** (1 / 0) | 3.23 (22 / 0) | 2.99 (22) | 5.24 (23) | 0.365 | 8000.00 | 8000.00 |
| `t2_mad_rand_g2_i8` | **3.21** (43 / 0) | 2.93 (112 / 0) | 2.95 (112) | 3.61 (45) | 0.151 | 5975.00 | 5894.00 |
| `t2_mad_rand_g3_i9` | **16.37** (201 / 0) | 30.25 (188 / 0) | 30.22 (188) | 92.78 (539) | 0.276 | 7720.00 | 6060.00 |
| `t3_mkp_200` | **17.83** (250 / 14) | 81.36 (1135 / 16) | 163.52 (5000, `NodeLimit`) | 148.71 (5000, `NodeLimit`) | 0.117 | 12906.80 | 12955.46 |
| `t3_cflp_300` | **9.25** (1 / 28) | 18.77 (54 / 34) | 8.54 (116) | 8.68 (118) | 0.169 | 2187.30 | 2081.53 |
| `t3_lotsizing_504` | **62.93** (2 / 206) | 80.28 (376 / 206) | 250.06 (3650, `TimeLimit`) | 250.04 (3789, `TimeLimit`) | 0.707 | 13120.44 | 10518.99 |
| `t3_sched_1000` | **6.34** (31 / 55) | 19.41 (87 / 53) | 250.07 (2565, `TimeLimit`) | 250.09 (2585, `TimeLimit`) | 0.197 | 46818.28 | 46971.09 |
| `t3_mad_g5_i20` | **0.47** (1 / 0) | 2.98 (17 / 0) | 2.65 (17) | 0.53 (1) | 0.671 | 12000.00 | 12000.00 |
| `t3_mad_g6_i30` | **0.81** (1 / 0) | 8.84 (58 / 0) | 7.66 (58) | 12.99 (74) | 1.643 | 16800.00 | 16800.00 |
| `t3_mad_g10_i100` | **7.24** (2 / 0) | 21.57 (25 / 0) | 19.26 (25) | 250.02 (151, `TimeLimit`) | 2.812 | 88000.00 | 88000.00 |
| `t3_mad_g4_i1000` | **29.77** (1 / 0) | 32.37 (2 / 0) | 31.02 (2) | 250.05 (6, `TimeLimit`) | 56.479 | 208000.00 | 208000.00 |
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
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.35x** [1.13x-1.62x] | 1.35x | 4.47x | 0.221 (0.0196) | 0.310 (0.0287) | 1.44x [0.32x-2.45x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **3.01x** [1.91x-4.76x] | 3.01x | 3.08x | 10.051 (0.811) | 33.082 (3.008) | 2.55x [0.89x-16.32x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.89x** [1.90x-4.41x] | 2.89x | 3.69x | 220.823 (43.398) | 941.103 (186.677) | 2.56x [1.54x-6.09x] |
| Square families ($m = n$) | 16 (146) | **2.67x** [1.93x-3.69x] | 2.59x | 3.78x | 226.413 (43.391) | 963.582 (187.754) | 2.22x [0.32x-16.32x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.78x** [1.23x-2.58x] | 1.66x | 3.19x | 4.682 (0.837) | 10.913 (1.959) | 1.98x [0.90x-2.91x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.38x** [1.20x-4.73x] | 2.52x | 3.00x | 35.465 (6.217) | 66.333 (11.006) | 1.89x [1.04x-16.32x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.39x** [1.86x-3.08x] | 2.30x | 3.61x | 231.095 (44.228) | 974.495 (189.713) | 2.03x [0.32x-16.32x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

### 7.2 Full 22-Family LSAP Pipeline Comparison

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.30x** | 0.0006 | 0.0009 | 0.0006 | 0.0118 | $0.0010 \pm 0.0010$ | $0.0009 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.28x** | 0.0018 | 0.0024 | 0.0020 | 0.0103 | $0.0019 \pm 0.0007$ | $0.0024 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.66x** | 0.0037 | 0.0062 | 0.0045 | 0.0150 | $0.0036 \pm 0.0008$ | $0.0060 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.13x** | 0.0037 | 0.0040 | 0.0046 | 0.0141 | $0.0035 \pm 0.0007$ | $0.0039 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.18x** | 0.0040 | 0.0043 | 0.0048 | 0.0148 | $0.0038 \pm 0.0008$ | $0.0044 \pm 0.0008$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.67x** | 0.0059 | 0.0109 | 0.0098 | 0.0325 | $0.0064 \pm 0.0017$ | $0.0106 \pm 0.0019$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.38x** | 0.0280 | 0.0704 | 0.0358 | 0.0960 | $0.0300 \pm 0.0108$ | $0.0687 \pm 0.0105$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.40x** | 0.0523 | 0.181 | 0.0673 | 0.230 | $0.0517 \pm 0.0054$ | $0.177 \pm 0.0255$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.13x** | 0.0478 | 0.0935 | 0.0701 | 0.142 | $0.0451 \pm 0.0040$ | $0.0965 \pm 0.0136$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.94x** | 0.137 | 0.261 | 0.151 | 0.315 | $0.147 \pm 0.0475$ | $0.279 \pm 0.0600$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **12.66x** | 0.0583 | 0.686 | 0.0741 | 0.763 | $0.0550 \pm 0.0094$ | $0.687 \pm 0.0165$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.95x** | 0.0349 | 0.0712 | 0.0625 | 0.145 | $0.0376 \pm 0.0122$ | $0.0714 \pm 0.0075$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.08x** | 0.0386 | 0.0785 | 0.0620 | 0.152 | $0.0387 \pm 0.0033$ | $0.0807 \pm 0.0092$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.57x** | 0.0803 | 0.289 | 0.106 | 0.370 | $0.0798 \pm 0.0079$ | $0.286 \pm 0.0328$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.35x** | 0.334 | 1.278 | 0.436 | 1.667 | $0.428 \pm 0.324$ | $1.262 \pm 0.125$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **4.14x** | 1.229 | 5.167 | 1.609 | 7.909 | $1.284 \pm 0.174$ | $5.289 \pm 0.415$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.76x** | 0.835 | 1.485 | 1.436 | 4.249 | $0.842 \pm 0.0407$ | $1.483 \pm 0.0546$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.68x** | 5.130 | 8.463 | 5.407 | 9.556 | $5.685 \pm 2.066$ | $9.409 \pm 2.761$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.55x** | 0.326 | 0.831 | 0.746 | 3.932 | $0.333 \pm 0.0177$ | $0.848 \pm 0.0576$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.34x** | 0.430 | 0.969 | 0.802 | 3.986 | $0.420 \pm 0.0236$ | $0.981 \pm 0.0324$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.81x** | 6.116 | 28.105 | 8.827 | 37.572 | $5.939 \pm 0.412$ | $28.588 \pm 2.236$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.80x** | 29.332 | 141.656 | 39.242 | 189.361 | $29.663 \pm 4.053$ | $141.621 \pm 9.110$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

### 7.3 LSAP Scalar vs. `GOEXPERIMENT=simd` (`AllAbsLE64`, `MinReducedSkip64`, `InitReducedRow64`, and `Min2ReducedRow64`)

In [`assign/solver.go`](../assign/solver.go), [`simd.AllAbsLE64With`](../internal/simd/kernels_amd64.go) and [`simd.ValidateAndNegate64With`](../internal/simd/assign_amd64.go) validate finite cost bounds via AVX2 checks whenever `len(cost) >= 32` (while the fused `colReduceValidateSquare` fast path skips `AllAbsLE64With` on finite square `Minimize` matrices and dispatches `ColReduceValidateRows64With` only for $n \ge 32$), [`simd.Transpose64With`](../internal/simd/assign_amd64.go) handles tall transposes with $\text{rows} \ge 8$ and $\text{cols} \ge 8$, [`simd.MinReducedSkip64With`](../internal/simd/assign_amd64.go) accelerates reduction-transfer slack scans in `preprocessSquareJV` ($n \ge 32$), [`simd.Min2ReducedRow64With`](../internal/simd/kernels_amd64.go) accelerates first/second reduced-minimum scans in `preprocessRectangularBV` and `augmentingRowReduction` at `minAVX2Min2Len = 256` (`minAVX512Min2Len = 128`), and [`simd.InitReducedRow64With`](../internal/simd/kernels_amd64.go) runs on a slice of length $\max(m, n) \ge 32$ at the start of each Dijkstra augmentation. All vector kernels clean upper `YMM` state via `StoreArray` + `ClearAVXUpperBits()` (`VZEROUPPER`) before scalar operations; exact per-family verdicts appear in the generated table below.

<!-- BEGIN GENERATED:ASSIGN_SIMD_SUMMARY -->
- **Negative controls** (`max(rows, cols) < 32` with no `>= 8x8` tall transpose and either fused square `Minimize` non-sparse path or `rows * cols < 32`, so no AVX2 kernel runs): 0 of 3 have a 95% interval excluding 1.
- **Instances where AVX2 kernels run** (`max(rows, cols) >= 32`, tall `rows > cols` with `>= 8x8`, or fallback `AllAbsLE64`/`ValidateAndNegate64` at `rows * cols >= 32`): 16 faster, 1 slower, 2 with no detectable effect, out of 19.
<!-- END GENERATED:ASSIGN_SIMD_SUMMARY -->

<!-- BEGIN GENERATED:ASSIGN_SIMD_TABLE -->
| Problem ID | Shape ($m \times n$) | `max(m, n)` | AVX2 kernels reachable | Verdict | Paired `scalar / SIMD` GeoMean [95% CI] | Scalar Solve (ms) | SIMD Solve (ms) | Trials |
| :--- | :---: | ---: | :---: | :--- | ---: | ---: | ---: | ---: |
| `a1_canon_4x4` | $4 \times 4$ | 4 | no (control) | identical source | 0.98x [0.96x-1.01x] | 0.0006 | 0.0005 | 11 |
| `a1_unif_10x10` | $10 \times 10$ | 10 | no (control) | identical source | 1.03x [0.79x-1.34x] | 0.0018 | 0.0017 | 11 |
| `a1_ties_20x20` | $20 \times 20$ | 20 | no (control) | identical source | 0.98x [0.94x-1.01x] | 0.0037 | 0.0037 | 11 |
| `a1_wide_16x32` | $16 \times 32$ | 32 | possible | SIMD faster | 1.13x [1.10x-1.16x] | 0.0037 | 0.0032 | 11 |
| `a1_tall_32x16` | $32 \times 16$ | 32 | possible | SIMD faster | 1.18x [1.15x-1.22x] | 0.0040 | 0.0034 | 11 |
| `a1_sparse_24x24` | $24 \times 24$ | 24 | possible | SIMD slower | 0.95x [0.91x-0.99x] | 0.0059 | 0.0063 | 11 |
| `a2_unif_64x64` | $64 \times 64$ | 64 | possible | SIMD faster | 1.37x [1.06x-1.77x] | 0.0280 | 0.0221 | 11 |
| `a2_unif_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.29x [1.23x-1.34x] | 0.0523 | 0.0409 | 11 |
| `a2_ties_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.66x [1.55x-1.77x] | 0.0478 | 0.0297 | 11 |
| `a2_geom_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.68x [1.60x-1.77x] | 0.137 | 0.0841 | 11 |
| `a2_rank1_100x100` | $100 \times 100$ | 100 | possible | SIMD faster | 1.26x [1.19x-1.33x] | 0.0583 | 0.0453 | 11 |
| `a2_wide_80x200` | $80 \times 200$ | 200 | possible | SIMD faster | 1.30x [1.01x-1.68x] | 0.0349 | 0.0276 | 11 |
| `a2_tall_200x80` | $200 \times 80$ | 200 | possible | no detectable effect | 1.19x [1.00x-1.42x] | 0.0386 | 0.0302 | 11 |
| `a2_max_128x128` | $128 \times 128$ | 128 | possible | SIMD faster | 1.32x [1.16x-1.50x] | 0.0803 | 0.0566 | 11 |
| `a2_unif_256x256` | $256 \times 256$ | 256 | possible | SIMD faster | 1.78x [1.24x-2.56x] | 0.334 | 0.224 | 11 |
| `a3_unif_500x500` | $500 \times 500$ | 500 | possible | no detectable effect | 1.41x [0.84x-2.37x] | 1.229 | 0.748 | 5 |
| `a3_ties_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.36x [2.19x-2.54x] | 0.835 | 0.352 | 5 |
| `a3_geom_500x500` | $500 \times 500$ | 500 | possible | SIMD faster | 2.47x [2.18x-2.81x] | 5.130 | 1.906 | 5 |
| `a3_wide_250x1000` | $250 \times 1000$ | 1000 | possible | SIMD faster | 1.86x [1.72x-2.01x] | 0.326 | 0.169 | 5 |
| `a3_tall_1000x250` | $1000 \times 250$ | 1000 | possible | SIMD faster | 1.56x [1.42x-1.72x] | 0.430 | 0.273 | 5 |
| `a3_unif_1000x1000` | $1000 \times 1000$ | 1000 | possible | SIMD faster | 1.66x [1.55x-1.78x] | 6.116 | 3.772 | 5 |
| `a3_unif_2000x2000` | $2000 \times 2000$ | 2000 | possible | SIMD faster | 1.54x [1.46x-1.62x] | 29.332 | 18.666 | 5 |
<!-- END GENERATED:ASSIGN_SIMD_TABLE -->

---

## 8. Production Binary Footprint and Linker Dead-Code Elimination

The `goblin-bench -binary-size` mode compiles four standalone `CGO_ENABLED=0` programs in an isolated temporary module against the live workspace across both default `go build` and `GOEXPERIMENT=simd`, saving compiled artifact sizes and `go tool nm -size` symbol counts to `bench/results/goblin_binary_size.json`. [`ValidateBinarySizeTelemetry`](../bench/docgen.go) rejects the telemetry bundle if `assign_default` links any `math/big`, `encoding/json`, `VIPR`, or `Certif` symbols, if `goblin_default` links any `encoding/json`, `VIPR`, or `Certif` symbols, or if any profile links `encoding/json`:

<!-- BEGIN GENERATED:BINARY_SIZE_TABLE -->
| Profile | Default Raw / Stripped (KiB) | `GOEXPERIMENT=simd` Raw / Stripped (KiB) | Stripped Delta vs. Default | `goblin` Text (KiB) | `math/big` Syms | `encoding/json` Syms | `VIPR` Syms | `Certif` Syms |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `assign` default (`LinearSum` + `SolveInto`) | 2784.8 / **1740.2** | 2813.7 / **1804.2** | baseline | 114.4 | `0` | `0` | `0` | `0` |
| `assign` certified (`+ assign.WithCertified(true)`) | 2871.3 / **1796.2** | 2898.1 / **1860.2** | +56.0 KiB (+3.2%) | 154.8 | `0` | `0` | `0` | `16` |
| `goblin` default (`Model.Solve`, `DefaultOptions`) | 5276.5 / **3360.2** | 5167.2 / **3376.2** | baseline | 899.1 | `130` | `0` | `0` | `0` |
| `goblin` certified + VIPR (`+ WithCertified + WithVIPRTrace`) | 6362.0 / **4092.2** | 6236.7 / **4116.2** | +732.0 KiB (+21.8%) | 1317.0 | `153` | `0` | `183` | `10` |
<!-- END GENERATED:BINARY_SIZE_TABLE -->

---

## 9. Keeping Documentation and Benchmarks Synchronized

1. **Regenerate telemetry** from a clean, committed tree in a single pinned session so that all eight telemetry files record identical `git_commit`, `source_fingerprint`, and `uname_release`: run the commands in the [README](../README.md#verification-benchmarking-and-documentation-sync-commands).
2. **Regenerate tables**: `go run ./cmd/goblin-bench -sync-docs`.
3. **Verify**: `go run ./cmd/goblin-bench -check-docs`, or `go test ./...`, which runs `TestDocumentationAndBenchmarkSync`. The gate validates corpus dimensions, parity, symbol identifiers, and provenance, and checks every generated block in `README.md`, `docs/benchmarks.md`, and `bench/results/final_report.md`.
