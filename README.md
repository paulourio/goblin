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
2. **SIMD correctness**: `12` bounds-check-free loop locks in [`cmd/goblin-optcheck`](cmd/goblin-optcheck/main.go), paired scalar-vs-`GOEXPERIMENT=simd` corpus parity, `Track5C` `VZEROUPPER`, x86-prefix, and CPU-feature-guarded EVEX control-flow-graph verification (`simd-cfg-check`, `50` vector functions across `2,178` deduplicated module functions), and `Track5D`/`Track5E` IEEE-754 lane reduction plus decoded `amd64` AVX2/AVX-512 machine-code equivalence with silicon-verified x86-64 NaN semantics.
3. **Binary soundness**: `14` `arm64` (`-gcflags=-l` standalone leaves, an inline-lowering snippet, and single-frame solver kernels up to external `BL` boundaries) and `6` `amd64` compiled ELF proofs covering allow-listed zero-GC execution, stack-split headroom, structural bounds-guard mediation (`BoundsGuardsHold`), and `VZEROUPPER`/frame ABI safety, cross-checked against fresh Go builds (`.text`, `.rodata`, `.data`, and fallback callees) by [`scripts/extract_elf_track5.py`](scripts/extract_elf_track5.py) and [`scripts/extract_amd64_track5e.py`](scripts/extract_amd64_track5e.py).
4. **Production binary size**: continuous `CGO_ENABLED=0` binary-size and `go tool nm -size` symbol telemetry (`bench/results/goblin_binary_size.json`) enforcing zero linker leakage from unreferenced certification and proof-tracing subsystems.

### 2. Lean 4 Formalization Structure (`formal/`, Lean `v4.36.0`)

The isolated Lake workspace in [`formal/`](formal/lakefile.toml) has zero external package dependencies (`23` Lean files, `52,743` LOC, `1,335` authored theorems in `GoblinFormal`, `5,809` kernel `TheoremInfo` declarations, `207` `#guard_msgs in #print axioms` locks plus `#verify_all_goblin_axioms` in [`AxiomAudit.lean`](formal/GoblinFormal/AxiomAudit.lean), `0` `sorryAx`, `0` `Lean.trustCompiler`, `0` `_native.bv_decide` or `_native.native_decide` axioms, and `0` custom axioms, independently verified by `leanchecker` and `nanoda_bin`) and spans three verification layers:

- **Result certificate checkers (Tracks 1, 3, 4)**: standalone `lsap-check`, `lp-check`, and `vipr-check` CLIs (`parseJsonStrict`, `10^12` OOM-safe guards, native `Option Rat` forbidden edges, Hall infeasibility, and `bound-conflict`) paired with opt-in pure-Go in-process `gb.WithCertified(true)` (exact zero default gap unless overridden via `WithRelGapTol`/`WithAbsGapTol`), `assign.WithCertified(true)` (2304-bit dyadic verification with dual-feasible `float64` `Result.U`/`Result.V`), and presolve-enveloped plus orbital-symmetry `gb.WithVIPRTrace` (`VerifyVIPRTrace`).
- **Algebraic rule proofs and live observer fuzzing (Tracks 2A-2C, 3B)**: machine-checked `rules-check` and `cuts-check` soundness theorems over abstract specifications for presolve tightenings, Euclidean GCD row reductions, reference LP dual postsolve (`postsolveLPDual`), static/dynamic $S_G$ orbital symmetry, structural GMI/VUB, lifted knapsack covers, lot-sizing `(l, S)` chains, aggregated cMIR, and sparse cut finalization, fed live from `presolveWithObserver` and `cutGenWorkspace.certObs` across `18` formal-wired solver fuzz targets (`19` default-build fuzzers).
- **Decoded ELF machine-code leaf equivalence (Tracks 5A-5E)**: machine-checked proofs directly over `14` `arm64` ELF routines and `6` decoded `amd64` AVX2/AVX-512 vector kernels (`530` x86-64/VEX/EVEX instructions).

### 3. Honest Assessment of Verification Strength and Scope Boundaries

- **Strong guarantees**: When `gb.WithCertified(true)`, `assign.WithCertified(true)`, or the Lean CLI checkers accept an output, global optimality (at exact zero rational gap by default), infeasibility, unboundedness, or dual bounds hold over exact rational arithmetic $\mathbb{Q}$ on the caller's original problem (with exact `Ceil`/`Floor` integer and binary bound tightening in `Model`) independent of floating-point rounding or heuristic bugs. Runtime `WithCertified` and `VerifyVIPRTrace` run pure-Go `*big.Rat` / `dyadic2304` verifiers mirroring the Lean specs rather than linking Lean binaries into Go; the trusted computing base is the Lean 4 kernel (and `leanchecker`/`nanoda_bin`) plus JSON serialization for CLI checks, or the Go runtime plus `math/big` / `dyadic2304` and [`certified.go`](certified.go) / [`assign/certified.go`](assign/certified.go) in process. On the `20` verified `arm64` and `amd64` leaf and single-frame kernels, modeled equivalence, frame/register ABI preservation, structural guard mediation, and allow-listed zero-GC safety hold on Go 1.27's emitted machine bytes with zero non-standard axioms (`{propext, Classical.choice, Quot.sound}` only).
- **Explicit limitations**: Default uncertified solves (`gb.WithCertified(false)`) execute imperative floating-point Go code ([`lu.go`](lu.go), [`simplex.go`](simplex.go), [`presolve.go`](presolve.go), [`cuts.go`](cuts.go), [`bab.go`](bab.go), [`heuristics.go`](heuristics.go)) without Lean source lifting. Track 2 rules, Track 3B `postsolveLPDual`, and Track 4's `BabState` (`activeBounds` and linear-objective gap predicates) prove abstract reference specifications rather than imperative Go state transitions; `--jsonl` differential fuzzing samples bounded regimes rather than exhausting all `float64` inputs; Track 5 `runArm64` stops at external `BL` boundaries, `checkPanicBounds` proves structural guard mediation (`BoundsGuardsHold`) rather than value-level slice-index unreachability, and `Track5B` `*_exec` theorems are single-input concrete overlay tests. Sparse Markowitz `LU`, Forrest-Tomlin updates, simplex anti-cycling and termination, parallel workers (`WithWorkers > 1`), the Go runtime and garbage collector, and vendored C++ CBC (`BackendCBC`) remain outside formal proof scope. Passing `gb.WithCertified(true)` guarantees **soundness** (failing closed with `StatusNumericalError` and `ErrCertificationFailed` whenever an exact witness cannot be extracted or verified), whereas severely ill-conditioned instances can reject witness extraction rather than achieving completeness.

### 4. Production Binary Footprint and Linker Dead-Code Elimination

Keeping [`formal/`](formal/lakefile.toml) in a standalone Lake workspace and empirical bridges inside `*_test.go` adds `0` bytes to Go builds. In production packages, `gb.WithCertified`, `assign.WithCertified`, and `gb.WithVIPRTrace` install unexported hook pointers on `Options`, while [`vipr_json.go`](vipr_json.go) and 2304-bit fixed-point `dyadic2304` arithmetic in [`assign/certified.go`](assign/certified.go) avoid `encoding/json/v2` and `math/big` package-init dependencies. Consequently, Go's linker eliminates `100%` of [`certified.go`](certified.go), [`assign/certified.go`](assign/certified.go), [`vipr_trace.go`](vipr_trace.go), `PresolveForVIPR`, `activeOrbitWithWitnesses`, and `encoding/json` (plus `math/big` in `goblin/assign`) from binaries that do not call certified or VIPR options:

<!-- BEGIN GENERATED:BINARY_SIZE_TABLE -->
| Profile | Default Raw / Stripped (KiB) | `GOEXPERIMENT=simd` Raw / Stripped (KiB) | Stripped Delta vs. Default | `goblin` Text (KiB) | `math/big` Syms | `encoding/json` Syms | `VIPR` Syms | `Certif` Syms |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `assign` default (`LinearSum` + `SolveInto`) | 2784.8 / **1740.2** | 2813.7 / **1804.2** | baseline | 114.4 | `0` | `0` | `0` | `0` |
| `assign` certified (`+ assign.WithCertified(true)`) | 2871.3 / **1796.2** | 2898.1 / **1860.2** | +56.0 KiB (+3.2%) | 154.8 | `0` | `0` | `0` | `16` |
| `goblin` default (`Model.Solve`, `DefaultOptions`) | 5276.5 / **3360.2** | 5167.2 / **3376.2** | baseline | 899.1 | `130` | `0` | `0` | `0` |
| `goblin` certified + VIPR (`+ WithCertified + WithVIPRTrace`) | 6362.0 / **4092.2** | 6236.7 / **4116.2** | +732.0 KiB (+21.8%) | 1317.0 | `153` | `0` | `183` | `10` |
<!-- END GENERATED:BINARY_SIZE_TABLE -->

---

## Documentation Index

Full technical guides live in the [`docs/`](docs/index.md) directory:

- **[Architecture and Design Overview](docs/index.md)**: Module boundaries, data flow from algebraic expressions to encapsulated `CSR`/`CSC` matrices, and memory pooling invariants.
- **[Modeling API and Serialization Formats](docs/modeling-and-formats.md)**: Constructing variables, linear expressions, and constraints, configuring solver flags and backends, validating and compiling models, and exchanging `PuLP` JSON and MPS payloads.
- **[Solver Internals and Mathematical Specification](docs/solver-internals.md)**: Three-stage singleton and sparse Markowitz LU with `FTRANSparseHint`/`BTRANSparseHint`, `BTRANExact`, and in-place Forrest-Tomlin $U$ updates, Revised Dual Simplex with Bound-Flipping Ratio Test (`selectDualBFRT`) and degenerate cost perturbation, LP and MILP presolve with static and dynamic orbital symmetry breaking, root and in-tree cutting planes (single-row and multi-row MIR, `GMI`, lifted covers, Flow/VUB), primal heuristics, optional parallel Branch-and-Cut (`WithWorkers`), and post-ARR sparse-core LSAP pricing.
- **[Benchmark Methodology and Corpus Results](docs/benchmarks.md)**: Hardware setup, single-session comparative telemetry against Python `PuLP + CBC` (`c1c171ec`) and in-process `BackendCBC`, paired scalar-vs-SIMD statistics, structural analysis of the synthetic Mean Absolute Deviation (`MAD`) `G x I` allocation family vs. random-weight negative controls (`BuildMADRandom`), and full checkpoint telemetry.
- **[Formal Verification Architecture (Lean 4 Prove Lane)](docs/formal-verification.md)**: Industry precedents, anti-translation-gap rules, opt-in fail-closed pure-Go runtime rational certification ([`certified.go`](certified.go), [`assign/certified.go`](assign/certified.go), dead-code-eliminated to `0` linked bytes along with [`vipr_trace.go`](vipr_trace.go) and `encoding/json` when unreferenced), and the five-track Lean 4 verification suite (`23` Lean files, `52,743` LOC, `1,335` authored theorems in `GoblinFormal`, `5,809` kernel-checked `TheoremInfo` declarations, `207` `AxiomAudit.lean` locks + `#verify_all_goblin_axioms`, `0` `sorryAx` and `0` `bv_decide`/native/custom axioms) covering LSAP/Hall/LP/Farkas/bound-conflict/Meyer unboundedness and presolve-enveloped + orbital-symmetry `WithVIPRTrace` Branch-and-Cut certification ([`exact_polish.go`](exact_polish.go), [`vipr_trace.go`](vipr_trace.go)), live-observed MILP presolve/symmetry/cut/LP-postsolve soundness proofs (`Track2A`-`Track2C`, `Track3B`, `Track4`), and binary/SIMD equivalence across `14` compiled `arm64` ELF routines and `6` decoded `amd64` AVX2/AVX-512 ELF kernels (`Track5A`-`Track5E`).

---

## Benchmarks

All benchmark tables in `README.md`, [`docs/benchmarks.md`](docs/benchmarks.md), and [`bench/results/final_report.md`](bench/results/final_report.md) are rendered deterministically from the eight JSON telemetry files in `bench/results/` (`python_baseline.json`, `goblin_cbc.json`, `goblin_scalar.json`, `goblin_simd.json`, `scipy_assign_baseline.json`, `goblin_assign_scalar.json`, `goblin_assign_simd.json`, `goblin_binary_size.json`). [`doc_sync_test.go`](doc_sync_test.go) fails `go test ./...` if any table drifts from the telemetry, if any `goblin` build disagrees with CBC or SciPy on a status or objective, or if any telemetry file differs in git commit, source/corpus fingerprint, or host kernel. Headline timings use the default pure-Go scalar build (`BackendGo`).

<!-- BEGIN GENERATED:PROVENANCE -->
- **Unified telemetry source**: fingerprint `f9016b5ee7cf4cd3` (working tree on top of commit `98fc925725e2d5fd4a0c84f266e02e80f8e2ece3`), Go toolchains default `go1.27.3-X:nodwarf5` and SIMD `go1.27.3-X:simd`, Linux kernel `6.18.64-gentoo-r1-x86_64`; the documentation gate rejects any telemetry file whose commit, fingerprint, toolchain SIMD mode, or kernel differs.
- **CPU affinity**: goblin `2`; Python + CBC `2`; Python + SciPy `2`.
- **Python MILP reference**: PuLP `4.2.0`, `CBC devel (git:c1c171ec)` (`cbc`, SHA-256 `0be2af228dd0056d`, `ratioGap=1e-6`, `allowableGap=1e-6`).
- **Python LSAP reference**: SciPy `1.18.1` (`scipy.optimize.linear_sum_assignment`, C++ `rectangular_lsap`), NumPy `2.5.4`.
- **CBC subprocess spawn floor**: 6.92 ms median on a 1-variable MILP and 6.84 ms median on a 1-variable LP (measured separately for provenance; summary tables report unadjusted wall times and paired ratios without floor subtraction).
- **goblin `WithCBC()` backend**: COIN-OR CBC/CLP `devel` (commit `c1c171ec`, vendored in `third_party/Cbc` and built with `-O3 -DNDEBUG`), linked in-process through `cgo` when built with `-tags cbc` (`gb.WithCBC()`). Both Python `PuLP + CBC` and in-process `BackendCBC` run the same `c1c171ec` solver stack at matching `1e-6` relative and absolute MIP gap tolerances; untouched default settings map `MaxCutPasses` to `passC = 30` and `MaxNodes` to unlimited on `BackendCBC`, while honoring explicit `WithMaxNodes` and `WithMaxCutPasses` overrides.
<!-- END GENERATED:PROVENANCE -->

Read the synthetic corpus ratios with three methodological notes. First, Tier 1 micro-models (`N <= 26`) are dominated by Python's CBC subprocess spawn floor reported in the provenance block above. Second, the planted-gap MAD family has a zero root LP gap by construction and closes at the root without branching. Consequently, the bold row (`Tier 2+3 non-planted MILPs`) isolates solver-dominated MILPs across paired randomized instances, alongside total subset wall times and full per-instance `[Min-Max]` ranges.

### 1. Aggregate Synthetic Workload Summary (Paired Per-Instance Geometric Mean and Wall-Clock Sums)

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

### 2. Synthetic MAD Group-Item Allocation Family (`G` Groups $\times$ `I` Items, $T = 6$ Periods)

The synthetic `MAD` group-item allocation family ([`BuildMADFromWeights`](bench/corpus.go)) assigns $I$ items across $G$ groups over $T = 6$ periods to minimize total L1 distance from each group's central level $m_g$ plus the roundness penalty $r_g = |m_g - 100 n_g|$ relative to an integer multiple $n_g \in \mathbb{Z}_{\ge 1}$. In the planted-gap generator, items in $S = \{0, \dots, I-G-1\}$ carry orthogonal curve signatures cancelled by the $G$ designated items $D = \{I-G, \dots, I-1\}$ with roundness-aligned period sums, yielding a zero root LP gap that root heuristics close without branching; unstructured random-weight negative controls (`BuildMADRandom`: `t1_mad_rand_g2_i4`, `t2_mad_rand_g2_i8`, `t2_mad_rand_g3_i9`) are included directly in the corpus and detailed in [`docs/benchmarks.md`](docs/benchmarks.md).

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

Cross-instance total latency dispersion across the randomized instances in each family (mean $\pm$ sample SD and two-sided 95% Student-$t$ interval after warmup):

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

### 3. Full 31-Family Synthetic Benchmark Corpus (`JSON -> Solution`)

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

### 4. Linear Sum Assignment (`goblin/assign` vs. `scipy.optimize.linear_sum_assignment`)

The 22-family synthetic Linear Sum Assignment corpus ([`bench/assign_corpus.go`](bench/assign_corpus.go), 200 randomized instances across square, rectangular wide/tall, degenerate integer-tie, Euclidean geometric, rank-1 outer-product, sparse `+Inf`, and maximization families) compares [`assign.LinearSumFlat`](assign/assign.go) against SciPy's C++ `rectangular_lsap` extension (`scipy.optimize.linear_sum_assignment`) in warm state on bit-identical `float64` cost matrices:

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

---

## Pure-Go vs. C++ CBC Performance Gaps and MAD Scaling Analysis

Comparing isolated unpresolved `Root LP` timings, paired trial ratios, and solver-core checkpoints (`CP5 +Presolve`, `CP6 +Cuts`, `CP7 Full`) between `BackendGo` and in-process `BackendCBC` (`c1c171ec`) highlights four structural trade-offs on large degenerate LPs and heavy-tail branch-and-cut models.

### 1. MAD Group-Item Allocation (`BackendGo` vs. `BackendCBC` and `COIN-OR CLP`)

The 11 MAD corpus families (`N = G(I + T + 3)` variables, `M = G(2T + 3) + I` constraints) separate solver performance into three distinct scaling regimes:

<!-- BEGIN GENERATED:MAD_CBC_GAP_TABLE -->
| Problem ID | $G \times I$ ($N \times M$) | Paired `Go / CBC` | Go Total Med (ms) | CBC Total Med (ms) | Root LP (`Go / CBC`) | Go Solve (ms / nodes / iters) | CBC Solve (ms / nodes / iters) | Go Root LP (ms / iters) | CBC Root LP (ms / iters) |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.03x** | 0.06 | 2.46 | 0.63x | 0.04 (1 / 19) | 2.43 (0 / 0) | 0.086 (28) | 0.137 (19) |
| `t1_mad_rand_g2_i4` | $2 \times 4$ ($26 \times 34$) | **0.05x** | 0.49 | 11.17 | 0.69x | 0.46 (2 / 58) | 11.13 (6 / 83) | 0.101 (30) | 0.146 (18) |
| `t2_mad_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.08x** | 0.16 | 1.80 | 0.71x | 0.13 (1 / 26) | 1.75 (0 / 0) | 0.101 (33) | 0.141 (21) |
| `t2_mad_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.08x** | 0.17 | 2.47 | 0.97x | 0.13 (1 / 40) | 2.42 (0 / 0) | 0.244 (58) | 0.252 (40) |
| `t2_mad_g4_i12` | $4 \times 12$ ($84 \times 72$) | **0.12x** | 0.43 | 3.17 | 0.88x | 0.32 (1 / 51) | 3.10 (0 / 0) | 0.365 (80) | 0.414 (63) |
| `t2_mad_rand_g2_i8` | $2 \times 8$ ($34 \times 38$) | **0.25x** | 3.27 | 16.75 | 0.93x | 3.21 (43 / 437) | 16.71 (12 / 216) | 0.151 (38) | 0.162 (24) |
| `t2_mad_rand_g3_i9` | $3 \times 9$ ($54 \times 54$) | **0.25x** | 16.43 | 90.33 | 0.96x | 16.37 (201 / 2102) | 90.27 (198 / 1300) | 0.276 (65) | 0.286 (54) |
| `t3_mad_g5_i20` | $5 \times 20$ ($145 \times 95$) | **0.08x** | 0.62 | 8.15 | 1.04x | 0.47 (1 / 75) | 8.03 (0 / 0) | 0.671 (91) | 0.647 (94) |
| `t3_mad_g6_i30` | $6 \times 30$ ($234 \times 120$) | **0.14x** | 1.02 | 7.40 | 1.59x | 0.81 (1 / 98) | 7.21 (0 / 0) | 1.643 (244) | 1.033 (149) |
| `t3_mad_g10_i100` | $10 \times 100$ ($1090 \times 250$) | **0.29x** | 8.16 | 36.70 | 0.78x | 7.24 (2 / 364) | 35.61 (0 / 0) | 2.812 (224) | 3.600 (319) |
| `t3_mad_g4_i1000` | $4 \times 1000$ ($4036 \times 1060$) | **0.17x** | 33.44 | 275.85 | 1.59x | 29.77 (1 / 2439) | 272.30 (0 / 0) | 56.479 (1900) | 35.470 (3128) |
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
| `t3_mkp_200` | $200 \times 8$ | **0.47x** | $21.61 \pm 13.62$ | $41.19 \pm 19.30$ | 17.83 (250 / 2447) | 51.81 (154 / 1145) | 81.36 (1135 / 16) | 163.52 (5000, `NodeLimit`) |
| `t2_mkp_50` | $50 \times 8$ | **0.36x** | $5.89 \pm 6.05$ | $12.47 \pm 9.09$ | 4.06 (1 / 19) | 8.76 (0 / 8) | 11.51 (2 / 4) | 4.69 (118) |
| `t2_binpack_44` | $44 \times 14$ | **1.17x** | $21.95 \pm 41.60$ | $6.63 \pm 2.15$ | 11.56 (274 / 1988) | 6.07 (0 / 8) | 13.84 (298 / 12) | 8.28 (772) |
| `t3_cflp_300` | $300 \times 34$ | **0.15x** | $10.66 \pm 7.40$ | $55.17 \pm 11.63$ | 9.25 (1 / 105) | 50.62 (18 / 483) | 18.77 (54 / 34) | 8.54 (116) |
| `t3_lotsizing_504` | $504 \times 336$ | **0.55x** | $55.81 \pm 34.74$ | $92.65 \pm 57.46$ | 62.93 (2 / 1061) | 71.24 (10 / 377) | 80.28 (376 / 206) | 250.06 (3650, `TimeLimit`) |
| `t3_sched_1000` | $1000 \times 100$ | **0.12x** | $8.39 \pm 4.38$ | $61.57 \pm 4.85$ | 6.34 (31 / 474) | 60.53 (0 / 76) | 19.41 (87 / 53) | 250.07 (2565, `TimeLimit`) |
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

# 11. Lean 4 Prove Lane build (75 jobs), arm64 + amd64 ELF verification, CLI/JSONL checks, and 19 formal Fuzz corpora
./scripts/verify_formal.sh
PYTHONDONTWRITEBYTECODE=1 python3 scripts/extract_elf_track5.py --verify
PYTHONDONTWRITEBYTECODE=1 python3 scripts/extract_amd64_track5e.py --verify
```

---

## License

The `goblin` module and its pure-Go default build (`BackendGo`, `goblin/assign`, `goblin/format`) are distributed under the [MIT License](LICENSE).

Optional vendored COIN-OR sources under `third_party/` (`Cbc`, `Cgl`, `Clp`, `CoinUtils`, and `Osi`, pinned in [`third_party/VERSIONS`](third_party/VERSIONS) and excluded from standard `go get` module downloads via `third_party/go.mod`) and adapted COIN-OR test fixtures in [`cbc_ported_test.go`](cbc_ported_test.go) are governed by the Eclipse Public License 2.0 (`EPL-2.0`; license texts in `third_party/*/LICENSE`). When distributing binaries compiled with `-tags cbc` that link those COIN-OR libraries, recipients may obtain the corresponding COIN-OR source code from `third_party/` in the upstream repository at `https://github.com/paulourio/goblin` (or from `https://github.com/coin-or`) under the terms of `EPL-2.0` Section 3.
