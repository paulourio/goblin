# `goblin`: Pure-Go LP, MILP, and Linear Sum Assignment Solver with Optional C++ CBC Backend

The `goblin` module provides an in-memory Linear Programming (LP) and Mixed-Integer Linear Programming (MILP) modeling engine and optional parallel Branch-and-Cut solver (`WithWorkers`) alongside the [`goblin/assign`](assign/assign.go) Linear Sum Assignment Problem (`LSAP`) solver, written in Go 1.27. It compiles algebraic models directly into encapsulated Dual-Index Compressed Sparse Row and Column (`CSR`/`CSC`) matrices (`CompiledProblem`) and streams standard `PuLP` JSON payloads directly into `Model` (`FromPuLPJSONBytes`), eliminating intermediate AST allocations, text-file I/O, and subprocess spawn costs. By default it builds as pure Go with zero `cgo` dependency (`BackendGo`), and nested module isolation (`third_party/go.mod`) keeps `go get github.com/paulourio/goblin` lightweight; callers who build with `-tags cbc` (`cgo` enabled, or `-tags "cbc,cbc_system"` against system COIN-OR packages) can also select the in-process C++ COIN-OR CBC/CLP backend (`WithCBC()` / `WithBackend(BackendCBC)`) or configure solver flags (`WithoutSIMD()`, `WithWorkers(...)`, `WithTimeLimit(...)`, etc.) through functional arguments on `Solve` and `SolveCompiled`.

## Quickstart

Install the module with `go get github.com/paulourio/goblin`.

### 1. Mixed-Integer Linear Programming (`goblin`)

Import `github.com/paulourio/goblin` as `gb`, construct a model with [`NewModel`](model.go), add continuous, integer, or binary variables, build terms and constraints with `x.T(coeff)` and `expr.Le(rhs)` (or `gb.T` / `gb.Le`), and call `m.Solve(ctx, ...)`:

```go
package main

import (
	"context"
	"fmt"
	"math"
	"time"

	gb "github.com/paulourio/goblin"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	m := gb.NewModel("std_mip", gb.Minimize)

	x := m.AddContVar("x", 0, 4)
	y := m.AddContVar("y", -1, 1)
	z := m.AddIntVar("z", 0, math.Inf(1))

	m.SetObjective(gb.NewExpr(
		x.T(1),
		y.T(4),
		z.T(9),
	))

	m.AddConstraint("c1", gb.NewExpr(x.T(1), y.T(1)).Le(5))
	m.AddConstraint("c2", gb.NewExpr(x.T(1), z.T(1)).Ge(10))
	m.AddConstraint("c3", gb.NewExpr(y.T(-1), z.T(1)).Eq(7.5))

	// Default pure-Go backend:
	res, err := m.Solve(ctx)
	if err != nil {
		panic(err)
	}
	fmt.Printf("status=%v objective=%.2f x=%.1f y=%.1f z=%.1f\n",
		res.Status, res.Objective, res.Value(x), res.Value(y), res.Value(z))

	// Or enable parallel B&B workers, fail-closed exact rational certification, CBC (-tags cbc), or scalar mode:
	// resPar, err := m.Solve(ctx, gb.WithWorkers(4))
	// resCert, err := m.Solve(ctx, gb.WithCertified(true))
	// resCBC, err := m.Solve(ctx, gb.WithCBC())
	// resScalar, err := m.Solve(ctx, gb.WithoutSIMD())
}
```

### 2. Linear Sum Assignment (`goblin/assign`)

Import `github.com/paulourio/goblin/assign` and call [`assign.LinearSum`](assign/assign.go) (or [`assign.LinearSumFlat`](assign/assign.go) for a contiguous row-major `[]float64` slice) to solve a square or rectangular bipartite assignment problem without constructing an algebraic `Model`:

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/paulourio/goblin/assign"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	res, err := assign.LinearSum(ctx, [][]float64{
		{90, 75, 75, 80},
		{35, 85, 55, 65},
		{125, 95, 90, 105},
		{45, 110, 95, 115},
	}, assign.Minimize)
	if err != nil {
		panic(err)
	}
	fmt.Printf("cost=%.0f rows=%v cols=%v\n", res.Objective, res.RowInd, res.ColInd)
}
```

---

## Architecture and Core Guarantees

1. **Zero-Subprocess In-Memory Execution**: Modeling (`Model`), matrix checks and assembly (`Model.Validate()`, `Model.CompileChecked()`), presolve (`Presolve`), root and node LPs (`LPSolver`), and Branch-and-Cut (`SolveCompiled`) execute in-process in pure Go using pooled workspace buffers (`AcquireWorkspace` / `ReleaseWorkspace`) with zero `cgo` dependency by default, plus an optional `-tags cbc` in-process `cgo` bridge to COIN-OR CBC/CLP (`cbc_cgo.go`, serialized process-wide across goroutines to guard unsynchronized upstream `Cgl`/`Cbc` static state).
2. **Schema Compatibility with Python `PuLP` and Free-Format MPS Export**: The `goblin/format` package reads and writes `PuLP`'s native `toDict()` / `fromDict()` JSON schema ([`format/pulp_json.go`](format/pulp_json.go)) via direct streaming ingestion ([`FromPuLPJSONBytes`](format/pulp_json.go)) with automatic AST fallback, and writes free-format MPS files ([`format/mps.go`](format/mps.go)).
3. **Optional Multi-Tier SIMD Kernels**: With `GOEXPERIMENT=simd` on `amd64` (gated at runtime on CPU `AVX-512`, `AVX2`, and `FMA` support), [`internal/simd/kernels_amd64.go`](internal/simd/kernels_amd64.go) and [`internal/simd/kernels_avx512_amd64.go`](internal/simd/kernels_avx512_amd64.go) dispatch 512-bit AVX-512 or 256-bit AVX2 vector loops (`Dot64`, `Axpy64`, `SelectDualLeaveRow64With`, `ScanDualRatioCand64With`, `MinReducedSkip64`, `InitReducedRow64`, `Min2ReducedRow64`, `AllAbsLE64`, and `RelaxRow64`) with `StoreArray` + `VZEROUPPER` cleanup and IEEE-754 `NaN` propagation parity across scalar and vector paths, and can be toggled at runtime via `WithoutSIMD()` or `WithSIMD(false)`. See the paired comparison in [`docs/benchmarks.md`](docs/benchmarks.md#5-default-build-vs-goexperimentsimd).
4. **Verified Parity on Synthetic and Public Suites**: [`solver_test.go`](solver_test.go) re-solves all 293 distinct randomized trial instances of the 31 synthetic benchmark families (`testdata/corpus/trials/*.jsonl`), compares status and objective with `PuLP + COIN-OR CBC` (`c1c171ec`) and `BackendCBC`, and verifies primal and row feasibility on every result. [`public_benchmarks_test.go`](public_benchmarks_test.go) verifies optimal status, objective parity, and primal feasibility across 20 held-out public Netlib LP and MIPLIB 3.0 MILP instances (`testdata/public/`) plus a valid dual lower bound under a 25-node budget with cuts and heuristics disabled (`WithCuts(false)`, `WithHeuristics(false)`, `WithMaxNodes(25)`) on `coinor_capplan1`, while [`regression_test.go`](regression_test.go) tests big-M fixed-charge models against enumerated ground truth and Farkas-certified LP scaling edge cases. `TimeLimit`, `MaxNodes`, and `MaxIter` return `StatusTimeLimit`, `StatusNodeLimit`, or `StatusIterationLimit` with a valid global bound once the root LP is solved; a node that cannot be evaluated is never discarded silently.

---

## Verification Architecture, Formalization Strength, and Binary Footprint

### 1. Continuous Four-Pillar Tracking

Every commit is validated against four automated engineering invariants:

1. **Performance benchmarks**: paired warm-state latency and status/objective parity across `31` LP and MILP families (`293` randomized trials) against `PuLP + COIN-OR CBC` (`c1c171ec`) and in-process `BackendCBC`, plus `22` Linear Sum Assignment families (`200` randomized trials) against `scipy.optimize.linear_sum_assignment`.
2. **SIMD correctness**: `12` bounds-check-free loop locks in [`cmd/goblin-optcheck`](cmd/goblin-optcheck/main.go), paired scalar-vs-`GOEXPERIMENT=simd` corpus parity, `Track5C` `VZEROUPPER` and EVEX control-flow-graph verification (`simd-cfg-check`), and `Track5D`/`Track5E` IEEE-754 lane reduction and decoded `amd64` AVX2/AVX-512 machine-code equivalence.
3. **Binary soundness**: `14` `arm64` and `6` `amd64` compiled ELF proofs covering zero-GC execution, stack-split headroom, `runtime.panicBounds` unreachability, and `VZEROUPPER` ABI safety, cross-checked against fresh Go builds by [`scripts/extract_elf_track5.py`](scripts/extract_elf_track5.py) and [`scripts/extract_amd64_track5e.py`](scripts/extract_amd64_track5e.py).
4. **Production binary size**: continuous `CGO_ENABLED=0` binary-size and `go tool nm -size` symbol telemetry (`bench/results/goblin_binary_size.json`) enforcing zero linker leakage from unreferenced certification and proof-tracing subsystems.

### 2. Lean 4 Formalization Structure (`formal/`, Lean `v4.36.0`)

The isolated Lake workspace in [`formal/`](formal/lakefile.toml) has zero external package dependencies (`23` Lean files, `1,188` authored theorems, `4,674` kernel `TheoremInfo` declarations, `162` `#guard_msgs in #print axioms` locks in [`AxiomAudit.lean`](formal/GoblinFormal/AxiomAudit.lean), `0` `sorryAx`, `0` `Lean.trustCompiler`, and `0` custom axioms) and spans three verification layers:

- **Result certificate checkers (Tracks 1, 3, 4)**: standalone `lsap-check`, `lp-check`, and `vipr-check` CLIs paired with opt-in in-process `gb.WithCertified(true)`, `assign.WithCertified(true)`, and presolve-enveloped plus orbital-symmetry `gb.WithVIPRTrace`.
- **Algebraic rule proofs and differential fuzzing (Tracks 2A-2C, 3B)**: machine-checked `rules-check` and `cuts-check` soundness theorems for presolve tightenings, Euclidean GCD row reductions, LP dual postsolve, static/dynamic orbital symmetry, GMI, lifted knapsack covers, lot-sizing `(l, S)` chains, aggregated cMIR, and sparse cut finalization, exercised by zero-escape-hatch `--jsonl` oracle pools across `18` solver fuzz targets (`19` total fuzzers).
- **Decoded ELF machine-code leaf equivalence (Tracks 5A-5E)**: machine-checked proofs directly over `14` stock `arm64` ELF routines and `6` decoded `amd64` AVX2/AVX-512 vector kernels (`530` x86-64/VEX/EVEX instructions).

### 3. Honest Assessment of Verification Strength and Scope Boundaries

- **Strong guarantees**: When `gb.WithCertified(true)`, `assign.WithCertified(true)`, or the Lean CLI checkers accept an output, global optimality, infeasibility, unboundedness, or dual bounds hold over exact rational arithmetic $\mathbb{Q}$ on the caller's original problem independent of floating-point rounding or heuristic bugs. The trusted computing base is the Lean 4 kernel plus JSON serialization for CLI checks, or the Go runtime plus `math/big` / `dyadic2304` and the in-process verifier ([`certified.go`](certified.go), [`assign/certified.go`](assign/certified.go)). On the `20` verified `arm64` and `amd64` leaf kernels, functional equivalence and ABI, bounds, and zero-GC safety hold on Go 1.27's emitted machine bytes.
- **Explicit limitations**: Default uncertified solves (`gb.WithCertified(false)`) execute roughly 60,000 lines of imperative floating-point Go code ([`lu.go`](lu.go), [`simplex.go`](simplex.go), [`presolve.go`](presolve.go), [`cuts.go`](cuts.go), [`bab.go`](bab.go), [`heuristics.go`](heuristics.go)) without Lean source lifting. Track 2 proves mathematical rule formulas rather than full imperative Go state transitions, and `--jsonl` differential fuzzing samples bounded regimes rather than exhausting all `float64` inputs. Sparse Markowitz `LU`, Forrest-Tomlin updates, simplex anti-cycling and termination, parallel workers (`WithWorkers > 1`), the Go runtime and garbage collector, and vendored C++ CBC (`BackendCBC`) remain outside formal proof scope. Passing `gb.WithCertified(true)` guarantees **soundness** (failing closed with `StatusNumericalError` and `ErrCertificationFailed` whenever an exact witness cannot be extracted or verified), whereas severely ill-conditioned instances can reject witness extraction rather than achieving completeness.

### 4. Production Binary Footprint and Linker Dead-Code Elimination

Keeping [`formal/`](formal/lakefile.toml) in a standalone Lake workspace and empirical bridges inside `*_test.go` adds `0` bytes to Go builds. In production packages, `gb.WithCertified`, `assign.WithCertified`, and `gb.WithVIPRTrace` install unexported hook pointers on `Options`, while [`vipr_json.go`](vipr_json.go) and 2304-bit fixed-point `dyadic2304` arithmetic in [`assign/certified.go`](assign/certified.go) avoid `encoding/json/v2` and `math/big` package-init dependencies. Consequently, Go's linker eliminates `100%` of [`certified.go`](certified.go), [`assign/certified.go`](assign/certified.go), [`vipr_trace.go`](vipr_trace.go), `PresolveForVIPR`, `activeOrbitWithWitnesses`, and `encoding/json` (plus `math/big` in `goblin/assign`) from binaries that do not call certified or VIPR options:

<!-- BEGIN GENERATED:BINARY_SIZE_TABLE -->
| Profile | Default Raw / Stripped (KiB) | `GOEXPERIMENT=simd` Raw / Stripped (KiB) | Stripped Delta vs. Default | `goblin` Text (KiB) | `math/big` Syms | `encoding/json` Syms | `VIPR` Syms | `Certif` Syms |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `assign` default (`LinearSum` + `SolveInto`) | 2784.8 / **1740.2** | 2813.7 / **1804.2** | baseline | 114.4 | `0` | `0` | `0` | `0` |
| `assign` certified (`+ assign.WithCertified(true)`) | 2858.9 / **1788.2** | 2891.1 / **1856.2** | +48.0 KiB (+2.8%) | 149.5 | `0` | `0` | `0` | `15` |
| `goblin` default (`Model.Solve`, `DefaultOptions`) | 5186.2 / **3300.2** | 5083.1 / **3320.2** | baseline | 862.1 | `130` | `0` | `0` | `0` |
| `goblin` certified + VIPR (`+ WithCertified + WithVIPRTrace`) | 6274.2 / **4032.2** | 6142.0 / **4052.2** | +732.0 KiB (+22.2%) | 1278.3 | `153` | `0` | `181` | `10` |
<!-- END GENERATED:BINARY_SIZE_TABLE -->

---

## Documentation Index

Full technical guides live in the [`docs/`](docs/index.md) directory:

- **[Architecture and Design Overview](docs/index.md)**: Module boundaries, data flow from algebraic expressions to encapsulated `CSR`/`CSC` matrices, and memory pooling invariants.
- **[Modeling API and Serialization Formats](docs/modeling-and-formats.md)**: Constructing variables, linear expressions, and constraints, configuring solver flags and backends, validating and compiling models, and exchanging `PuLP` JSON and MPS payloads.
- **[Solver Internals and Mathematical Specification](docs/solver-internals.md)**: Three-stage singleton and sparse Markowitz LU with `FTRANSparseHint`/`BTRANSparseHint`, `BTRANExact`, and in-place Forrest-Tomlin $U$ updates, Revised Dual Simplex with Bound-Flipping Ratio Test (`selectDualBFRT`) and degenerate cost perturbation, LP and MILP presolve with static and dynamic orbital symmetry breaking, root and in-tree cutting planes (single-row and multi-row MIR, `GMI`, lifted covers, Flow/VUB), primal heuristics, optional parallel Branch-and-Cut (`WithWorkers`), and post-ARR sparse-core LSAP pricing.
- **[Benchmark Methodology and Corpus Results](docs/benchmarks.md)**: Hardware setup, single-session comparative telemetry against Python `PuLP + CBC` (`c1c171ec`) and in-process `BackendCBC`, paired scalar-vs-SIMD statistics, structural analysis of the synthetic Mean Absolute Deviation (`MAD`) `G x I` allocation family vs. random-weight negative controls (`BuildMADRandom`), and full checkpoint telemetry.
- **[Formal Verification Architecture (Lean 4 Prove Lane)](docs/formal-verification.md)**: Industry precedents, anti-translation-gap rules, opt-in fail-closed runtime rational certification ([`certified.go`](certified.go), [`assign/certified.go`](assign/certified.go), dead-code-eliminated to `0` linked bytes along with [`vipr_trace.go`](vipr_trace.go) and `encoding/json` when unreferenced), and the five-track Lean 4 verification suite (`23` Lean files, `1,188` authored theorems, `4,674` kernel-checked `TheoremInfo` declarations, `162` `AxiomAudit.lean` locks, `0` `sorryAx`) covering LSAP/LP/Farkas/Meyer unboundedness and presolve-enveloped + orbital-symmetry `WithVIPRTrace` Branch-and-Cut certification ([`exact_polish.go`](exact_polish.go), [`vipr_trace.go`](vipr_trace.go)), MILP presolve/symmetry/cut/LP-postsolve soundness proofs (`Track2A`-`Track2C`, `Track3B`, `Track4`), and binary/SIMD equivalence across `14` compiled `arm64` ELF routines and `6` decoded `amd64` AVX2/AVX-512 ELF kernels (`Track5A`-`Track5E`).

---

## Benchmarks

All benchmark tables in `README.md`, [`docs/benchmarks.md`](docs/benchmarks.md), and [`bench/results/final_report.md`](bench/results/final_report.md) are rendered deterministically from the eight JSON telemetry files in `bench/results/` (`python_baseline.json`, `goblin_cbc.json`, `goblin_scalar.json`, `goblin_simd.json`, `scipy_assign_baseline.json`, `goblin_assign_scalar.json`, `goblin_assign_simd.json`, `goblin_binary_size.json`). [`doc_sync_test.go`](doc_sync_test.go) fails `go test ./...` if any table drifts from the telemetry, if any `goblin` build disagrees with CBC or SciPy on a status or objective, or if any telemetry file differs in git commit, source/corpus fingerprint, or host kernel. Headline timings use the default pure-Go scalar build (`BackendGo`).

<!-- BEGIN GENERATED:PROVENANCE -->
- **Unified telemetry source**: fingerprint `e8f74b711ec92a3a` (commit `4bc6c2b5d36f1e76ea2231cdf0a1a1c4cf5f0054`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.24 ms median on a 1-variable MILP and 6.26 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

Read the synthetic corpus ratios with three methodological notes. First, Tier 1 micro-models (`N <= 26`) are dominated by Python's CBC subprocess spawn floor reported in the provenance block above. Second, the planted-gap MAD family has a zero root LP gap by construction and closes at the root without branching. Consequently, the bold row (`Tier 2+3 non-planted MILPs`) isolates solver-dominated MILPs across paired randomized instances, alongside total subset wall times and full per-instance `[Min-Max]` ranges.

### 1. Aggregate Synthetic Workload Summary (Paired Per-Instance Geometric Mean and Wall-Clock Sums)

<!-- BEGIN GENERATED:TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `Py / Go` GeoMean [Clustered 95% CI] | Pooled Trial `Py / Go` | Equal-Family `Go / CBC` | `Go` Trial (Med) Sum (ms) | `CBC` Trial (Med) Sum (ms) | `Py` Trial (Med) Sum (ms) | Per-Instance `Py / Go` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Canonical micro-models (`N <= 26`, spawn-dominated) | 10 (110) | **330.78x** [157.60x-694.26x] | 330.78x | 0.05x | 9.3 (0.7) | 195.5 (19.3) | 1052.1 (99.1) | 456.78x [26.06x-3002.05x] |
| Tier 2: Target p90 MILPs (`30 <= N <= 84`) | 13 (143) | **20.43x** [12.53x-33.32x] | 20.43x | 0.15x | 648.3 (32.3) | 1946.5 (170.3) | 4299.7 (415.6) | 26.26x [0.13x-165.89x] |
| Tier 3: Large tail MILPs (`145 <= N <= 4036`) | 8 (40) | **10.43x** [5.05x-21.53x] | 10.43x | 0.20x | 882.8 (136.9) | 2893.3 (555.7) | 6186.9 (1067.2) | 10.47x [1.10x-74.97x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | **13.33x** [8.00x-22.21x] | 14.66x | 0.19x | 1222.2 (124.7) | 3100.7 (392.8) | 6759.1 (984.5) | 14.58x [0.13x-165.89x] |
| Tier 3 non-MAD MILPs | 4 (20) | **7.95x** [1.60x-39.62x] | 7.95x | 0.27x | 582.3 (93.2) | 1241.2 (229.9) | 2803.8 (599.5) | 6.49x [1.12x-74.97x] |
| Planted-gap MAD family (zero root LP gap by design) | 8 (64) | **27.55x** [12.32x-61.58x] | 35.81x | 0.10x | 309.8 (44.5) | 1764.1 (335.7) | 3827.1 (507.6) | 38.03x [1.10x-164.93x] |
| Random-weight MAD (negative controls) | 3 (33) | **15.99x** [0.92x-277.81x] | 15.99x | 0.15x | 300.0 (20.2) | 1207.2 (115.7) | 2241.7 (247.7) | 14.70x [1.34x-119.27x] |
| All non-planted families (Tiers 1-3) | 23 (229) | **48.91x** [21.53x-111.10x] | 59.17x | 0.11x | 1230.6 (125.4) | 3271.2 (409.6) | 7711.6 (1074.3) | 50.00x [0.13x-3002.05x] |
| All 31 synthetic corpus families | 31 (293) | **42.17x** [22.53x-78.94x] | 53.02x | 0.11x | 1540.4 (170.0) | 5035.3 (745.3) | 11538.6 (1581.9) | 42.69x [0.13x-3002.05x] |
<!-- END GENERATED:TIER_SUMMARY_TABLE -->

### 2. Synthetic MAD Group-Item Allocation Family (`G` Groups $\times$ `I` Items, $T = 6$ Periods)

The synthetic `MAD` group-item allocation family ([`BuildMADFromWeights`](bench/corpus.go)) assigns $I$ items across $G$ groups over $T = 6$ periods to minimize total L1 distance from each group's central level $m_g$ plus the roundness penalty $r_g = |m_g - 100 n_g|$ relative to an integer multiple $n_g \in \mathbb{Z}_{\ge 1}$. In the planted-gap generator, items in $S = \{0, \dots, I-G-1\}$ carry orthogonal curve signatures cancelled by the $G$ designated items $D = \{I-G, \dots, I-1\}$ with roundness-aligned period sums, yielding a zero root LP gap that root heuristics close without branching; unstructured random-weight negative controls (`BuildMADRandom`: `t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`) are included directly in the corpus and detailed in [`docs/benchmarks.md`](docs/benchmarks.md).

<!-- BEGIN GENERATED:MAD_PIPELINE_TABLE -->
| Problem ID | $G \times I$ | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Trial 0 Obj | Parity |
| :--- | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 26 | 34 | **122.0x** [42.6x-164.9x] | 0.06 | 9.30 | 0.04 | 9.10 | 0.022 | 0.206 | 2400.0 | `OK` |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 26 | 34 | **59.5x** [26.1x-119.3x] | 0.49 | 33.73 | 0.46 | 33.49 | 0.029 | 0.241 | 4601.0 | `OK` |
| `t2_mad_g2_i8` | $2 \times 8$ | 34 | 38 | **51.7x** [44.1x-64.0x] | 0.18 | 9.17 | 0.14 | 8.89 | 0.044 | 0.271 | 2400.0 | `OK` |
| `t2_mad_g3_i9` | $3 \times 9$ | 54 | 54 | **46.5x** [33.7x-57.0x] | 0.21 | 10.15 | 0.14 | 9.72 | 0.065 | 0.416 | 4800.0 | `OK` |
| `t2_mad_g4_i12` | $4 \times 12$ | 84 | 72 | **32.2x** [29.1x-38.5x] | 0.36 | 11.28 | 0.29 | 10.67 | 0.070 | 0.597 | 8000.0 | `OK` |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 34 | 38 | **9.7x** [3.5x-27.8x] | 3.14 | 46.89 | 3.10 | 46.60 | 0.043 | 0.298 | 5975.0 | `OK` |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 54 | 54 | **7.1x** [1.3x-29.5x] | 16.59 | 167.04 | 16.53 | 166.61 | 0.055 | 0.433 | 7720.0 | `OK` |
| `t3_mad_g5_i20` | $5 \times 20$ | 145 | 95 | **33.2x** [26.5x-40.3x] | 0.53 | 18.08 | 0.42 | 16.97 | 0.110 | 1.105 | 12000.0 | `OK` |
| `t3_mad_g6_i30` | $6 \times 30$ | 234 | 120 | **17.3x** [10.5x-23.1x] | 1.04 | 17.89 | 0.84 | 16.22 | 0.205 | 1.695 | 16800.0 | `OK` |
| `t3_mad_g10_i100` | $10 \times 100$ | 1090 | 250 | **6.0x** [1.1x-14.7x] | 8.17 | 61.09 | 7.36 | 53.12 | 1.061 | 8.153 | 88000.0 | `OK` |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 4036 | 1060 | **10.2x** [4.4x-24.8x] | 33.98 | 370.66 | 30.28 | 336.00 | 3.785 | 34.091 | 208000.0 | `OK` |
<!-- END GENERATED:MAD_PIPELINE_TABLE -->

Cross-instance total latency dispersion across the randomized instances in each family (mean $\pm$ sample SD and two-sided 95% Student-$t$ interval after warmup):

<!-- BEGIN GENERATED:MAD_STATS_TABLE -->
| Problem ID | $G \times I$ | Trials | `goblin (Go)` Scalar Total (Mean $\pm$ SD [95% CI], ms) | Python `PuLP + CBC` Total (Mean $\pm$ SD [95% CI], ms) | `goblin (Go)` SIMD Total (Mean $\pm$ SD [95% CI], ms) |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | $0.08 \pm 0.05$ ($\pm 0.03$) | $9.06 \pm 0.46$ ($\pm 0.31$) | $0.07 \pm 0.01$ ($\pm 0.01$) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | $0.55 \pm 0.32$ ($\pm 0.21$) | $30.33 \pm 9.63$ ($\pm 6.47$) | $0.51 \pm 0.28$ ($\pm 0.19$) |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | $0.18 \pm 0.01$ ($\pm 0.01$) | $9.34 \pm 0.53$ ($\pm 0.36$) | $0.15 \pm 0.01$ ($\pm 0.00$) |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | $0.22 \pm 0.04$ ($\pm 0.03$) | $10.27 \pm 0.35$ ($\pm 0.23$) | $0.22 \pm 0.04$ ($\pm 0.03$) |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | $0.36 \pm 0.01$ ($\pm 0.01$) | $11.68 \pm 0.85$ ($\pm 0.57$) | $0.35 \pm 0.01$ ($\pm 0.01$) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | $4.17 \pm 3.21$ ($\pm 2.16$) | $35.21 \pm 16.87$ ($\pm 11.33$) | $4.16 \pm 3.24$ ($\pm 2.18$) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | $22.54 \pm 22.08$ ($\pm 14.83$) | $138.26 \pm 63.47$ ($\pm 42.63$) | $22.20 \pm 21.89$ ($\pm 14.71$) |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | $0.54 \pm 0.02$ ($\pm 0.02$) | $17.88 \pm 2.40$ ($\pm 2.98$) | $0.54 \pm 0.02$ ($\pm 0.02$) |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | $1.08 \pm 0.36$ ($\pm 0.44$) | $18.11 \pm 0.44$ ($\pm 0.55$) | $1.57 \pm 0.78$ ($\pm 0.97$) |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | $16.63 \pm 21.26$ ($\pm 26.39$) | $62.80 \pm 3.69$ ($\pm 4.58$) | $18.31 \pm 23.14$ ($\pm 28.72$) |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | $41.84 \pm 14.38$ ($\pm 17.85$) | $577.82 \pm 610.64$ ($\pm 758.09$) | $39.00 \pm 16.12$ ($\pm 20.01$) |
<!-- END GENERATED:MAD_STATS_TABLE -->

### 3. Full 31-Family Synthetic Benchmark Corpus (`JSON -> Solution`)

<!-- BEGIN GENERATED:FULL_PIPELINE_TABLE -->
| Problem ID | Vars | Rows | Paired `Py / Go` [Min-Max] | GoTotal Med (ms) | PyTotal Med (ms) | GoSolve Med (ms) | PySolve Med (ms) | GoPrep (ms) | PyPrep (ms) | Parity |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `t1_std_lp` | 3 | 3 | **397.3x** [100.5x-727.4x] | 0.01 | 6.30 | 0.01 | 6.24 | 0.007 | 0.063 | `OK` |
| `t1_std_lp_max` | 3 | 3 | **371.2x** [183.6x-631.3x] | 0.01 | 6.15 | 0.00 | 6.10 | 0.008 | 0.049 | `OK` |
| `t1_std_mip` | 3 | 3 | **486.6x** [122.2x-1203.6x] | 0.01 | 7.30 | 0.01 | 7.23 | 0.005 | 0.058 | `OK` |
| `t1_mip_float_obj` | 3 | 3 | **758.0x** [628.4x-984.8x] | 0.01 | 7.14 | 0.00 | 7.09 | 0.004 | 0.051 | `OK` |
| `t1_whiskas` | 6 | 5 | **606.9x** [446.5x-737.2x] | 0.01 | 6.25 | 0.00 | 6.18 | 0.005 | 0.072 | `OK` |
| `t1_furniture` | 2 | 3 | **288.9x** [79.3x-1058.7x] | 0.02 | 7.57 | 0.02 | 7.52 | 0.003 | 0.051 | `OK` |
| `t1_knapsack_12` | 12 | 3 | **111.8x** [50.0x-331.7x] | 0.09 | 9.20 | 0.09 | 9.10 | 0.007 | 0.090 | `OK` |
| `t1_infeasible_mip` | 2 | 1 | **2026.2x** [1170.8x-3002.1x] | 0.00 | 6.17 | 0.00 | 6.12 | 0.002 | 0.042 | `OK` |
| `t1_mad_g2_i4` | 26 | 34 | **122.0x** [42.6x-164.9x] | 0.06 | 9.30 | 0.04 | 9.10 | 0.022 | 0.206 | `OK` |
| `t1_mad_rand_g2_i4` | 26 | 34 | **59.5x** [26.1x-119.3x] | 0.49 | 33.73 | 0.46 | 33.49 | 0.029 | 0.241 | `OK` |
| `t2_mkp_30` | 30 | 6 | **11.6x** [6.2x-16.8x] | 2.12 | 27.98 | 2.10 | 27.80 | 0.018 | 0.173 | `OK` |
| `t2_mkp_50` | 50 | 8 | **6.0x** [3.1x-34.4x] | 4.10 | 16.77 | 4.07 | 16.51 | 0.029 | 0.254 | `OK` |
| `t2_cflp_36` | 36 | 11 | **57.4x** [24.4x-165.9x] | 0.15 | 8.84 | 0.14 | 8.68 | 0.017 | 0.153 | `OK` |
| `t2_cflp_49` | 49 | 13 | **33.2x** [19.9x-93.1x] | 0.35 | 10.48 | 0.32 | 10.29 | 0.028 | 0.215 | `OK` |
| `t2_gap_45` | 45 | 13 | **20.6x** [4.9x-66.2x] | 0.98 | 14.15 | 0.96 | 13.93 | 0.020 | 0.207 | `OK` |
| `t2_lotsizing_48` | 48 | 32 | **13.1x** [5.8x-34.0x] | 3.32 | 68.74 | 3.29 | 68.50 | 0.027 | 0.244 | `OK` |
| `t2_binpack_44` | 44 | 14 | **11.4x** [0.1x-37.5x] | 0.61 | 13.36 | 0.55 | 13.15 | 0.021 | 0.192 | `OK` |
| `t2_setcover_50` | 50 | 20 | **49.5x** [6.4x-143.6x] | 0.20 | 10.75 | 0.14 | 10.48 | 0.035 | 0.289 | `OK` |
| `t2_mad_g2_i8` | 34 | 38 | **51.7x** [44.1x-64.0x] | 0.18 | 9.17 | 0.14 | 8.89 | 0.044 | 0.271 | `OK` |
| `t2_mad_g3_i9` | 54 | 54 | **46.5x** [33.7x-57.0x] | 0.21 | 10.15 | 0.14 | 9.72 | 0.065 | 0.416 | `OK` |
| `t2_mad_g4_i12` | 84 | 72 | **32.2x** [29.1x-38.5x] | 0.36 | 11.28 | 0.29 | 10.67 | 0.070 | 0.597 | `OK` |
| `t2_mad_rand_g2_i8` | 34 | 38 | **9.7x** [3.5x-27.8x] | 3.14 | 46.89 | 3.10 | 46.60 | 0.043 | 0.298 | `OK` |
| `t2_mad_rand_g3_i9` | 54 | 54 | **7.1x** [1.3x-29.5x] | 16.59 | 167.04 | 16.53 | 166.61 | 0.055 | 0.433 | `OK` |
| `t3_mkp_200` | 200 | 8 | **5.3x** [4.0x-6.5x] | 18.65 | 114.81 | 18.58 | 114.19 | 0.071 | 0.617 | `OK` |
| `t3_cflp_300` | 300 | 34 | **27.9x** [7.7x-75.0x] | 6.09 | 247.18 | 5.99 | 246.28 | 0.103 | 0.902 | `OK` |
| `t3_lotsizing_504` | 504 | 336 | **2.6x** [1.1x-4.9x] | 61.81 | 156.92 | 61.62 | 155.06 | 0.207 | 1.852 | `OK` |
| `t3_sched_1000` | 1000 | 100 | **10.4x** [5.0x-14.5x] | 6.64 | 80.58 | 6.36 | 77.70 | 0.310 | 2.741 | `OK` |
| `t3_mad_g5_i20` | 145 | 95 | **33.2x** [26.5x-40.3x] | 0.53 | 18.08 | 0.42 | 16.97 | 0.110 | 1.105 | `OK` |
| `t3_mad_g6_i30` | 234 | 120 | **17.3x** [10.5x-23.1x] | 1.04 | 17.89 | 0.84 | 16.22 | 0.205 | 1.695 | `OK` |
| `t3_mad_g10_i100` | 1090 | 250 | **6.0x** [1.1x-14.7x] | 8.17 | 61.09 | 7.36 | 53.12 | 1.061 | 8.153 | `OK` |
| `t3_mad_g4_i1000` | 4036 | 1060 | **10.2x** [4.4x-24.8x] | 33.98 | 370.66 | 30.28 | 336.00 | 3.785 | 34.091 | `OK` |
<!-- END GENERATED:FULL_PIPELINE_TABLE -->

### 4. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus ([`bench/assign_corpus.go`](bench/assign_corpus.go), 200 randomized instances across square, rectangular wide/tall, degenerate integer-tie, Euclidean geometric, rank-1 outer-product, sparse `+Inf`, and maximization families) compares [`assign.LinearSumFlat`](assign/assign.go) against SciPy's C++ `rectangular_lsap` extension (`scipy.optimize.linear_sum_assignment`) in warm state on bit-identical `float64` cost matrices:

<!-- BEGIN GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->
| Workload Subset | Families (Trials) | Equal-Family `SciPy Solve / goblin` GeoMean [Clustered 95% CI] | Pooled Trial `SciPy Solve / goblin` | Equal-Family `SciPy Total / goblin` | `goblin` Solve Trial (Med) Sum (ms) | SciPy Solve Trial (Med) Sum (ms) | Per-Instance `SciPy Solve / goblin` Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro/Small (`N <= 32`) | 6 (66) | **1.44x** [1.22x-1.70x] | 1.44x | 4.68x | 0.219 (0.0195) | 0.322 (0.0295) | 1.51x [0.34x-2.60x] |
| Tier 2: Medium / Target p90 (`64 <= N <= 256`) | 9 (99) | **2.97x** [1.86x-4.75x] | 2.97x | 3.02x | 9.012 (0.799) | 32.592 (2.941) | 2.59x [0.90x-15.96x] |
| Tier 3: Large Tail (`500 <= N <= 2000`) | 7 (35) | **2.83x** [1.85x-4.32x] | 2.83x | 3.50x | 221.630 (43.639) | 931.076 (186.558) | 2.48x [1.46x-5.71x] |
| Square families ($m = n$) | 16 (146) | **2.67x** [1.94x-3.68x] | 2.60x | 3.73x | 226.097 (43.619) | 953.161 (187.586) | 2.32x [0.34x-15.96x] |
| Rectangular families ($m \neq n$, wide and tall) | 6 (54) | **1.80x** [1.31x-2.48x] | 1.69x | 3.16x | 4.765 (0.839) | 10.829 (1.943) | 1.96x [0.90x-2.58x] |
| Degenerate / structured (ties, rank-1, geometric, sparse) | 7 (65) | **2.31x** [1.16x-4.61x] | 2.45x | 2.89x | 35.432 (6.161) | 64.733 (10.913) | 1.80x [1.09x-15.96x] |
| **All 22 synthetic LSAP families** | 22 (200) | **2.40x** [1.87x-3.07x] | 2.32x | 3.57x | 230.861 (44.458) | 963.990 (189.529) | 2.04x [0.34x-15.96x] |
<!-- END GENERATED:ASSIGN_TIER_SUMMARY_TABLE -->

<!-- BEGIN GENERATED:ASSIGN_PIPELINE_TABLE -->
| Problem ID | Shape ($m \times n$) | Family | Paired `SciPy Solve / goblin` | GoSolve Med (ms) | SciPySolve Med (ms) | GoTotal Med (ms) | SciPyTotal Med (ms) | Go Solve Mean $\pm$ SD (ms) | SciPy Solve Mean $\pm$ SD (ms) | Trial 0 Obj | Parity |
| :--- | :---: | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: |
| `a1_canon_4x4` | $4 \times 4$ | `canonical` | **1.54x** | 0.0005 | 0.0010 | 0.0006 | 0.0138 | $0.0010 \pm 0.0010$ | $0.0011 \pm 0.0002$ | 275.00 | `OK` |
| `a1_unif_10x10` | $10 \times 10$ | `uniform` | **1.29x** | 0.0017 | 0.0024 | 0.0020 | 0.0104 | $0.0019 \pm 0.0008$ | $0.0024 \pm 0.0005$ | 18107.93 | `OK` |
| `a1_ties_20x20` | $20 \times 20$ | `integer_ties` | **1.69x** | 0.0037 | 0.0062 | 0.0046 | 0.0153 | $0.0035 \pm 0.0007$ | $0.0061 \pm 0.0016$ | 4.00 | `OK` |
| `a1_wide_16x32` | $16 \times 32$ | `wide` | **1.16x** | 0.0037 | 0.0041 | 0.0045 | 0.0143 | $0.0035 \pm 0.0007$ | $0.0040 \pm 0.0008$ | 5044.66 | `OK` |
| `a1_tall_32x16` | $32 \times 16$ | `tall` | **1.34x** | 0.0039 | 0.0050 | 0.0047 | 0.0156 | $0.0037 \pm 0.0008$ | $0.0051 \pm 0.0018$ | 2840.65 | `OK` |
| `a1_sparse_24x24` | $24 \times 24$ | `sparse` | **1.70x** | 0.0060 | 0.0109 | 0.0100 | 0.0329 | $0.0064 \pm 0.0016$ | $0.0107 \pm 0.0017$ | 2261.00 | `OK` |
| `a2_unif_64x64` | $64 \times 64$ | `uniform` | **2.39x** | 0.0284 | 0.0703 | 0.0360 | 0.0954 | $0.0300 \pm 0.0109$ | $0.0690 \pm 0.0102$ | 16825.55 | `OK` |
| `a2_unif_100x100` | $100 \times 100$ | `uniform` | **3.28x** | 0.0533 | 0.174 | 0.0679 | 0.224 | $0.0537 \pm 0.0072$ | $0.177 \pm 0.0261$ | 15794.03 | `OK` |
| `a2_ties_100x100` | $100 \times 100$ | `integer_ties` | **2.13x** | 0.0470 | 0.0933 | 0.0693 | 0.145 | $0.0449 \pm 0.0038$ | $0.0961 \pm 0.0138$ | 1.00 | `OK` |
| `a2_geom_100x100` | $100 \times 100$ | `geometric` | **1.63x** | 0.136 | 0.218 | 0.147 | 0.257 | $0.146 \pm 0.0459$ | $0.233 \pm 0.0480$ | 610058.93 | `OK` |
| `a2_rank1_100x100` | $100 \times 100$ | `rank1` | **12.38x** | 0.0591 | 0.682 | 0.0766 | 0.759 | $0.0561 \pm 0.0084$ | $0.687 \pm 0.0235$ | 20173201.47 | `OK` |
| `a2_wide_80x200` | $80 \times 200$ | `wide` | **1.95x** | 0.0350 | 0.0690 | 0.0597 | 0.142 | $0.0375 \pm 0.0121$ | $0.0710 \pm 0.0078$ | 3410.92 | `OK` |
| `a2_tall_200x80` | $200 \times 80$ | `tall` | **2.00x** | 0.0393 | 0.0771 | 0.0621 | 0.150 | $0.0398 \pm 0.0037$ | $0.0797 \pm 0.0073$ | 4329.48 | `OK` |
| `a2_max_128x128` | $128 \times 128$ | `maximize` | **3.56x** | 0.0795 | 0.279 | 0.102 | 0.354 | $0.0797 \pm 0.0080$ | $0.284 \pm 0.0324$ | 1233885.41 | `OK` |
| `a2_unif_256x256` | $256 \times 256$ | `uniform` | **3.83x** | 0.321 | 1.279 | 0.439 | 1.694 | $0.332 \pm 0.0454$ | $1.266 \pm 0.120$ | 14858.73 | `OK` |
| `a3_unif_500x500` | $500 \times 500$ | `uniform` | **3.89x** | 1.213 | 5.208 | 1.661 | 7.899 | $1.400 \pm 0.408$ | $5.304 \pm 0.374$ | 16720.22 | `OK` |
| `a3_ties_500x500` | $500 \times 500$ | `integer_ties` | **1.72x** | 0.864 | 1.493 | 1.464 | 4.012 | $0.864 \pm 0.0293$ | $1.486 \pm 0.0573$ | 0.00 | `OK` |
| `a3_geom_500x500` | $500 \times 500$ | `geometric` | **1.66x** | 5.044 | 8.410 | 5.553 | 9.251 | $5.657 \pm 2.181$ | $9.188 \pm 2.685$ | 1396753.69 | `OK` |
| `a3_wide_250x1000` | $250 \times 1000$ | `wide` | **2.50x** | 0.340 | 0.816 | 0.718 | 3.628 | $0.335 \pm 0.0242$ | $0.835 \pm 0.0573$ | 2515.07 | `OK` |
| `a3_tall_1000x250` | $1000 \times 250$ | `tall` | **2.27x** | 0.418 | 0.972 | 0.865 | 3.699 | $0.433 \pm 0.0424$ | $0.979 \pm 0.0344$ | 2394.37 | `OK` |
| `a3_unif_1000x1000` | $1000 \times 1000$ | `uniform` | **4.89x** | 5.798 | 27.873 | 7.491 | 36.444 | $5.794 \pm 0.196$ | $28.384 \pm 2.251$ | 16203.23 | `OK` |
| `a3_unif_2000x2000` | $2000 \times 2000$ | `uniform` | **4.71x** | 29.962 | 141.786 | 38.331 | 184.352 | $29.844 \pm 3.780$ | $140.039 \pm 10.069$ | 16450.52 | `OK` |
<!-- END GENERATED:ASSIGN_PIPELINE_TABLE -->

---

## Pure-Go vs. C++ CBC Performance Gaps and MAD Scaling Analysis

Comparing isolated unpresolved `Root LP` timings, paired trial ratios, and solver-core checkpoints (`CP5 +Presolve`, `CP6 +Cuts`, `CP7 Full`) between `BackendGo` and in-process `BackendCBC` (`c1c171ec`) highlights four structural trade-offs on large degenerate LPs and heavy-tail branch-and-cut models.

### 1. MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families (`N = G(I + T + 3)` variables, `M = G(2T + 3) + I` constraints) separate solver performance into three distinct scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Paired `Go / CBC` | Go Total Med (ms) | CBC Total Med (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.03x** | 0.06 | 2.47 | 0.62x | 0.04 (1 / 19) | 2.44 (0 / 0) | 0.084 (28) | 0.136 (19) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.05x** | 0.49 | 11.22 | 0.70x | 0.46 (2 / 58) | 11.19 (6 / 83) | 0.103 (30) | 0.148 (18) |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.09x** | 0.18 | 1.80 | 0.68x | 0.14 (1 / 26) | 1.75 (0 / 0) | 0.104 (33) | 0.155 (21) |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.09x** | 0.21 | 2.42 | 1.00x | 0.14 (1 / 40) | 2.37 (0 / 0) | 0.242 (58) | 0.242 (40) |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.10x** | 0.36 | 3.24 | 0.87x | 0.29 (1 / 51) | 3.16 (0 / 0) | 0.352 (80) | 0.405 (63) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.25x** | 3.14 | 16.58 | 1.00x | 3.10 (43 / 437) | 16.54 (12 / 216) | 0.152 (38) | 0.153 (24) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.25x** | 16.59 | 87.94 | 0.98x | 16.53 (201 / 2102) | 87.88 (198 / 1300) | 0.281 (65) | 0.288 (54) |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.07x** | 0.53 | 8.10 | 1.06x | 0.42 (1 / 75) | 7.98 (0 / 0) | 0.667 (91) | 0.628 (94) |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.14x** | 1.04 | 7.50 | 1.49x | 0.84 (1 / 98) | 7.30 (0 / 0) | 1.547 (244) | 1.040 (149) |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **0.29x** | 8.17 | 35.48 | 0.73x | 7.36 (2 / 364) | 34.65 (0 / 0) | 2.651 (224) | 3.621 (319) |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **0.17x** | 33.98 | 274.68 | 1.63x | 30.28 (1 / 2439) | 270.87 (0 / 0) | 57.486 (1900) | 35.230 (3128) |
<!-- END GENERATED:MAD_CBC_GAP_TABLE -->

1. **Small-to-Medium Planted-Gap MAD (`t1_mad_g2_i4` through `t3_mad_g5_i20`)**: Every structured `t*_mad_g*` instance has a zero root LP gap by design (`RootLPBound == OptimalObj`). Pure-Go closes those models at node 1 through presolve orbit fixing and root heuristics (`tryPropagatedRounding`, `tryRootSubMIP`, and `tryFractionalDiving` in [`heuristics.go`](heuristics.go)) without C++ `CbcModel` and `CglPreProcess` setup costs.
2. **Large Planted-Gap MAD (`t3_mad_g6_i30`, `t3_mad_g10_i100`, `t3_mad_g4_i1000`)**: Pure-Go `Go Solve` closes these models at the root (exact node counts and solve times in the generated table above) because [`Presolve`](presolve.go) verifies the $S_G$ group-permutation automorphism (`applyOrbitSymmetryBreaking`), fixes the $G \times G$ designated-item bijection to $I_G$, and eliminates $G^2$ binary variables and $2G$ assignment equations before the root LP runs, while [`FTRANSparseHint`](lu.go) and [`BTRANSparseHint`](lu.go) restrict basis triangular sweeps to active index windows. On the unpresolved `Root LP` column:
   - **Equality Crash Basis (`t3_mad_g10_i100`)**: [`crashEqualityBasis`](simplex.go) builds a lower-triangular, dual-feasible equality basis with analytical DSE weights (`LU`, `FTRAN`/`BTRAN`, and ratio times appear in the generated table above).
   - **Dense-Row Coupling and Forrest-Tomlin Updates (`t3_mad_g4_i1000`)**: On 1,000-row bases, [`crashEqualityBasis`](simplex.go) skips columns incident to coupling rows wider than 128 nonzeros so BFRT bound flips retain sparse unit slacks, while [`lu.go`](lu.go) performs in-place mutated-$U$ Forrest-Tomlin updates (`PushEtaSparse`) inside pre-allocated headroom arenas up to `maxEtaUpdates = 56`.
3. **Symmetric Random-Weight MAD (`t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`)**: Assigning dense random weights (`BuildMADRandom`) removes the zero-gap structure and forces Branch-and-Cut to prove optimality:
   - [`Presolve`](presolve.go) verifies exact $S_G$ group symmetry and fixes the $G \times G$ designated-item bijection to $I_G$, [`bab.go`](bab.go) exits after round 1 when root cuts yield zero bound gain and purges slack root cuts via [`PurgeSlackCuts`](simplex.go), and [`tryRootSubMIP`](heuristics.go) enforces a 50% RINS integer-fixing gate on zero-cut roots.
   - Across [`docs/benchmarks.md`](docs/benchmarks.md#mad-solver-core-checkpoint-progression), comparing `CP7 Full` against `CP5 +Presolve` (`Presolve=on, Cuts=off, Heuristics=off`) isolates the effect of cutting planes and primal heuristics on random-weight MAD search.

### 2. Non-MAD Corpus Comparison, Dispersion, and Checkpoint Trade-Offs

Across the non-MAD families, multi-row MIR cut aggregation (`separateAggregatedMIRCuts`), bound-cutoff strong-branching domain reductions (`selectBranchVarCore`), and globally valid in-tree cuts (`generateInTreeCutRound`) accelerate Branch-and-Cut convergence, while the generated `Go Total Mean +- SD` and `CBC Total Mean +- SD` columns capture seed-to-seed variance (for example outlier seeds in `t2_binpack_44`):

<!-- BEGIN GENERATED:NON_MAD_CBC_GAP_TABLE -->
| Problem ID | $N \times M$ | Paired `Go / CBC` | Go Total Mean $\pm$ SD (ms) | CBC Total Mean $\pm$ SD (ms) | Go CP7 Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go CP6 +Cuts (ms / nodes / cuts) | Go CP5 +Presolve (ms / nodes) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t3_mkp_200` | $200 \times 8$ | **0.45x** | $19.05 \pm 10.09$ | $40.13 \pm 18.99$ | 18.58 (250 / 2447) | 48.92 (154 / 1145) | 82.00 (1135 / 16) | 165.47 (5000, `NodeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **0.36x** | $5.86 \pm 6.21$ | $12.26 \pm 8.98$ | 4.07 (1 / 19) | 8.57 (0 / 8) | 11.55 (2 / 4) | 4.69 (118) |
| `t2_binpack_44` | $44 \times 14$ | **0.19x** | $15.33 \pm 47.14$ | $6.59 \pm 2.10$ | 0.55 (2 / 51) | 6.11 (0 / 8) | 1.00 (5 / 8) | 8.23 (772) |
| `t3_cflp_300` | $300 \times 34$ | **0.15x** | $10.89 \pm 9.13$ | $55.11 \pm 11.57$ | 5.99 (1 / 106) | 50.08 (18 / 483) | 19.07 (54 / 34) | 8.59 (116) |
| `t3_lotsizing_504` | $504 \times 336$ | **0.65x** | $78.01 \pm 75.90$ | $92.31 \pm 56.97$ | 61.62 (2 / 1061) | 70.91 (10 / 377) | 79.06 (376 / 206) | 250.04 (3736, `TimeLimit`) |
| `t3_sched_1000` | $1000 \times 100$ | **0.13x** | $8.52 \pm 4.43$ | $60.69 \pm 4.69$ | 6.36 (31 / 474) | 59.31 (0 / 76) | 19.40 (87 / 53) | 250.05 (2574, `TimeLimit`) |
<!-- END GENERATED:NON_MAD_CBC_GAP_TABLE -->

---

## Verification Scope and Architectural History

The current codebase has passed full pure-Go, `GOEXPERIMENT=simd`, `-tags cbc`, race-detector, linter, differential-test, and `GOOS=linux GOARCH=arm64` / `GOAMD64=v3`/`v4` cross-compilation gates on the local AVX2 host (`i9-13900K`). Physical AVX-512 and `linux/arm64` target runtime re-verification remain pending target hardware access.

### Architectural Context and Rejected Designs

1. **LU Updates and Row-Singleton Stability (`lu.go`)**: The factorizer maintains elbow-room $U$ arenas (`uRowStart`/`uColStart`) paired with rank-free in-place mutated-$U$ Forrest-Tomlin updates (`PushEta`/`PushEtaSparse`) up to `maxEtaUpdates = 56`, avoiding extra branch checks during `FTRAN` and `BTRAN`. Two alternatives were tested and rejected earlier: (a) a lazy-rank Forrest-Tomlin prototype with `maxEtaUpdates = 128` regressed `miplib_p0201` and inflated `netlib_brandy` pivots from `uRank` branch mispredictions; (b) removing the `luMarkowitzTol * colMax` check on Stage 2 row singletons caused unscaled `1000x` multipliers in $L$ and numerical stalling on `t3_mad_g4_i1000`.
2. **Root LP Crash Basis and Dense Coupling (`simplex.go`)**: Dual Phase I (`installDualPhase1Bounds`) keeps the public Netlib LPs inside sparse dual simplex without falling back to primal simplex, and [`crashEqualityBasis`](simplex.go) accelerates `t3_mad_g10_i100` unpresolved Root LP setup vs. slack bases. Crashing columns incident to dense coupling rows (`> 128` nonzeros) was rejected because displacing 1-NNZ artificial slacks on `t3_mad_g4_i1000` increased $U$ fill-in and solve latency.
3. **Cut Separation, Symmetry, and Zero-Cut Sub-MIP Gating (`cuts.go`, `presolve.go`, `bab.go`, `heuristics.go`)**: Root cut rounds enforce GMI/MIR density caps (`maxCutNNZ`), exit after round 1 on zero bound gain, and purge inactive slack cuts both before branching and periodically in-tree via [`PurgeSlackCuts`](simplex.go). On zero-cut roots, a blanket `< 48`-integer sub-MIP ban helped median latency on small seeds at the cost of mean and tail regressions; requiring 50% fixed integers (RINS criterion) improved both median and tail behavior across all trials.
4. **Architectural Summary**:
   - **Numerical and Bridge Hardening**: Multi-scale three-bucket Shewchuk summation (`wideDotAcc`, `assignWideAcc`), compensated residual verification and Farkas exact-dot certificates (`BTRANExact`), exact `*big.Rat` basis dual polishing and LP/MILP Meyer unboundedness certification (`ExactBasisDuals`, `ExtractUnboundedCertificate` in [`exact_polish.go`](exact_polish.go)), opt-in fail-closed runtime rational verification (`WithCertified`, `VerifyVIPRTrace` in [`certified.go`](certified.go), and `assign.WithCertified` in [`assign/certified.go`](assign/certified.go)), presolve-enveloped and orbital-symmetry Branch-and-Cut proof generation (`WithVIPRTrace` in [`vipr_trace.go`](vipr_trace.go)), strict `lb > ub` validation across presolve and simplex, and CBC tiny-coefficient, quotient-bound, scaling, and drift guards (`cbc_cgo.go`, `cbc_bridge.cpp`).
   - **Memory Pooling & Serialization**: Pointer-free inline `wideDotAcc`, flat LU nucleus arenas with intrusive `int32` Markowitz buckets, copy-on-write `Presolve`, lazy CBC bridge slices, and direct streaming `PuLP` JSON ingestion (`format/pulp_json_fast.go`).
   - **Multi-Tier SIMD & Search**: Runtime AVX-512/AVX2/Scalar dispatch (`internal/simd/`), in-register DSE leaving-row scan (`SelectDualLeaveRow64With`), batched dual ratio screening (`ScanDualRatioCand64With`), multi-row MIR aggregation, globally valid in-tree cuts (`generateInTreeCutRound`), dynamic node orbital branching, strong-branching bound-cutoff domain reduction, post-ARR Volgenant sparse-core LSAP pricing, and optional parallel Branch-and-Cut via `WithWorkers`.

---

## Verification, Benchmarking, and Documentation Sync Commands

Run all commands from the `goblin/` directory:

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

# 11. Lean 4 Prove Lane build (74 jobs), arm64 + amd64 ELF verification, CLI/JSONL checks, and 19 formal Fuzz corpora
./scripts/verify_formal.sh
PYTHONDONTWRITEBYTECODE=1 python3 scripts/extract_elf_track5.py --verify
PYTHONDONTWRITEBYTECODE=1 python3 scripts/extract_amd64_track5e.py --verify
```

---

## License

The `goblin` module and its pure-Go default build (`BackendGo`, `goblin/assign`, `goblin/format`) are distributed under the [MIT License](LICENSE).

Optional vendored COIN-OR sources under `third_party/` (`Cbc`, `Cgl`, `Clp`, `CoinUtils`, and `Osi`, pinned in [`third_party/VERSIONS`](third_party/VERSIONS) and excluded from standard `go get` module downloads via `third_party/go.mod`) and adapted COIN-OR test fixtures in [`cbc_ported_test.go`](cbc_ported_test.go) are governed by the Eclipse Public License 2.0 (`EPL-2.0`; license texts in `third_party/*/LICENSE`). When distributing binaries compiled with `-tags cbc` that link those COIN-OR libraries, recipients may obtain the corresponding COIN-OR source code from `third_party/` in the upstream repository at `https://github.com/paulourio/goblin` (or from `https://github.com/coin-or`) under the terms of `EPL-2.0` Section 3.
