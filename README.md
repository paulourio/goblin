# goblin

The `goblin` module is a pure-Go solver for Linear Programming (LP), Mixed-Integer Linear Programming (MILP), and Linear Sum Assignment Problems (LSAP). It runs inside your Go process with zero `cgo` dependencies, no Python runtime, and no external solver binaries. You can build algebraic models in Go, stream Python `PuLP` JSON payloads directly into memory, or export standard MPS files.

- **Pure Go by default**: Compiles cleanly with `CGO_ENABLED=0` and reuses pooled memory workspaces across solves. No subprocess launches or temporary files sit between your application and the solver.
- **Two focused packages**: General LP and MILP modeling lives in the root package (`gb`), while dense and sparse bipartite assignment matrices run directly through [`goblin/assign`](assign/assign.go).
- **Opt-in exact checking**: Passing `gb.WithCertified(true)`, `gb.WithCertifiedWitness`, or `assign.WithCertified(true)` checks results in-process using pure-Go exact `*big.Rat` and `dyadic2304` checkers before returning (Lean is never invoked at runtime; those pure-Go checkers carry no formal proof of their Go code and are validated offline against Lean 4 CLI checkers). Unused certification code disappears completely at link time.
- **Optional COIN-OR CBC backend**: Need to compare against or delegate to C++ COIN-OR CBC/CLP? Build with `-tags cbc` and pass `gb.WithCBC()` to invoke CBC in process.

---

## Quickstart

Install the module with Go 1.27 or newer:

```bash
go get github.com/paulourio/goblin
```

### Mixed-Integer Linear Programming

Import `github.com/paulourio/goblin` as `gb`, register continuous, integer, or binary variables, attach linear constraints, and solve the model in memory:

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

	res, err := m.Solve(ctx)
	if err != nil {
		panic(err)
	}
	fmt.Printf("status=%v objective=%.2f x=%.1f y=%.1f z=%.1f\n",
		res.Status, res.Objective, res.Value(x), res.Value(y), res.Value(z))

	// Optional solver controls:
	// resPar, err := m.Solve(ctx, gb.WithWorkers(4))
	// resCert, err := m.Solve(ctx, gb.WithCertified(true))
	// resCBC, err := m.Solve(ctx, gb.WithCBC())
}
```

See [Modeling API and Serialization Formats](docs/modeling-and-formats.md) for `PuLP` JSON streaming, MPS export, reusable compiled models, and all solver options.

### Linear Sum Assignment

Import `github.com/paulourio/goblin/assign` to match rows to columns on square or rectangular cost matrices without building an algebraic model:

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

For contiguous slices and zero-allocation loops, call `assign.LinearSumFlat` or `assign.SolveInto`. Full details appear in [Linear Sum Assignment API](docs/modeling-and-formats.md#5-linear-sum-assignment-api-goblinassign).

---

## Performance

Every table below renders automatically from canonical JSON telemetry in `bench/results/` and is checked on every test run by [`doc_sync_test.go`](doc_sync_test.go). All measurements compare the default pure-Go build against Python `PuLP + COIN-OR CBC` (for LP and MILP) and `SciPy` (`linear_sum_assignment`, for LSAP), pinned single-threaded to one performance core (`i9-13900K`) after warmup, with verified objective parity on every trial.

### LP and MILP Overview

Keep two corpus properties in mind when reading LP and MILP speedups against the Python `PuLP + cbc` subprocess baseline. First, 10 Tier 1 micro-models are dominated by Python's CBC subprocess startup cost. Second, 8 of the 31 synthetic families are planted-gap MAD models with a zero root LP gap by construction that close without branching. The bold `Tier 2+3 non-planted MILPs` row excludes both groups to isolate solver-dominated branching workloads, while Section 9 of [Benchmark Methodology and Corpus Results](docs/benchmarks.md#9-pure-go-vs-in-process-c-cbc-performance-analysis) compares pure-Go directly against in-process `BackendCBC`. Columns report cumulative end-to-end wall time (`JSON -> solution`) summed across all randomized trials in each subset alongside equal-family geometric-mean speedup (`Python / Go`).

<!-- BEGIN GENERATED:README_CBC_HEADLINE -->
Across the 14 solver-dominated Tier 2+3 non-planted MILP families, pure-Go `BackendGo` achieves a **10.27x** [5.65x-18.66x] equal-family geometric-mean total speedup against the Python `PuLP + cbc` subprocess baseline and **3.87x** [2.42x-6.17x] total (**4.05x** [2.47x-6.66x] solve) against in-process C++ `BackendCBC`; Python MPS serialization and process spawn account for the difference between those two ratios.
<!-- END GENERATED:README_CBC_HEADLINE -->

<!-- BEGIN GENERATED:README_MILP_SUMMARY_TABLE -->
| Workload | Families (Trials) | Go Sum (ms) | Python Sum (ms) | Speedup (GeoMean [95% CI]) | Per-Instance Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro-models (`N <= 26`, spawn-dominated) | 10 (110) | 9.4 | 1077.3 | **281.29x** [139.64x-566.64x] | 304.42x [40.78x-2969.91x] |
| Tier 2: Medium MILPs (`30 <= N <= 84`) | 13 (143) | 833.1 | 4410.3 | **16.47x** [8.54x-31.74x] | 22.82x [0.11x-151.93x] |
| Tier 3: Large MILPs (`145 <= N <= 4036`) | 8 (40) | 937.3 | 6338.8 | **9.41x** [4.55x-19.47x] | 9.69x [1.05x-63.74x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | 1447.7 | 6920.0 | **10.27x** [5.65x-18.66x] | 11.19x [0.11x-151.93x] |
| All 31 synthetic families | 31 (293) | 1779.8 | 11826.3 | **35.60x** [18.68x-67.84x] | 41.82x [0.11x-2969.91x] |
<!-- END GENERATED:README_MILP_SUMMARY_TABLE -->

Full provenance, per-family breakdowns across all 31 families, solver-core checkpoints, and pure-Go vs. in-process C++ CBC analysis live in [Benchmark Methodology and Corpus Results](docs/benchmarks.md).

### Group-Item Allocation (MAD)

The synthetic Mean Absolute Deviation (`MAD`) family assigns $I$ items across $G$ groups over 6 periods to keep each group close to a rounded target level. Structured instances (`t*_mad_g*`) carry a zero root LP gap by design; random-weight controls (`*_rand_*`) remove that structure so branch-and-cut must search the tree. Each row reports median end-to-end times (`JSON -> solution`) across randomized trials and paired geometric-mean speedup (`Python / Go`):

<!-- BEGIN GENERATED:README_MAD_SUMMARY_TABLE -->
| Problem | $G \times I$ | Trials | Go (ms) | Python (ms) | Speedup |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | 0.06 | 9.49 | **145.9x** |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | 0.58 | 33.56 | **62.1x** |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | 0.16 | 9.80 | **59.4x** |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | 0.20 | 10.37 | **49.1x** |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | 0.38 | 12.63 | **33.4x** |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | 3.40 | 48.79 | **9.2x** |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | 18.50 | 169.97 | **6.8x** |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | 0.66 | 18.54 | **28.0x** |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | 1.06 | 19.01 | **17.0x** |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | 8.58 | 63.51 | **5.9x** |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | 35.37 | 377.98 | **10.1x** |
<!-- END GENERATED:README_MAD_SUMMARY_TABLE -->

**Note on SIMD**: Experimental `GOEXPERIMENT=simd` builds deliver little overall speedup on MAD and other sparse LP/MILP workloads because scalar matrix factorization dominates runtime. Full dispersion (`Mean ± SD`), prep/solve splits, checkpoint progression, and paired scalar-vs-SIMD benchmarks appear in [MAD Benchmarks](docs/benchmarks.md#3-mad-group-item-allocation-formulation) and [Default Build vs. `GOEXPERIMENT=simd`](docs/benchmarks.md#5-default-build-vs-goexperimentsimd).

### Linear Sum Assignment

The `goblin/assign` corpus compares in-memory pure-Go execution directly against SciPy's compiled C++ `rectangular_lsap` extension across 22 square, rectangular, geometric, tied, rank-1, and sparse matrix families (cumulative solver wall times and paired `SciPy / Go` solve speedups):

<!-- BEGIN GENERATED:README_ASSIGN_SUMMARY_TABLE -->
| Workload | Families (Trials) | Go Sum (ms) | SciPy Sum (ms) | Speedup (GeoMean [95% CI]) | Per-Instance Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Small (`N <= 32`) | 6 (66) | 0.210 | 0.319 | **1.56x** [1.29x-1.87x] | 1.59x [1.12x-2.72x] |
| Tier 2: Medium (`64 <= N <= 256`) | 9 (99) | 9.446 | 33.060 | **2.91x** [1.80x-4.71x] | 2.58x [0.96x-16.11x] |
| Tier 3: Large (`500 <= N <= 2000`) | 7 (35) | 222.911 | 958.728 | **2.84x** [1.84x-4.37x] | 2.53x [1.46x-6.04x] |
| **All 22 synthetic LSAP families** | 22 (200) | 232.567 | 992.106 | **2.43x** [1.91x-3.10x] | 2.08x [0.96x-16.11x] |
<!-- END GENERATED:README_ASSIGN_SUMMARY_TABLE -->

Unlike sparse MILPs, dense assignment matrices benefit substantially from AVX2 and AVX-512 vectorization under `GOEXPERIMENT=simd`. Per-family results, square vs. rectangular splits, and scalar-vs-SIMD tables live in [Linear Sum Assignment Benchmarks](docs/benchmarks.md#71-aggregate-lsap-workload-summary) and [LSAP SIMD Analysis](docs/benchmarks.md#73-lsap-scalar-vs-goexperimentsimd).

---

## Correctness and Guarantees

Optimization solvers balance numerical speed against mathematical certainty. The `goblin` module separates fast default execution from opt-in exact verification so callers pay for certification only when needed.

### Default Solver Guarantees

- **Verified parity**: Automated tests check status, objective, and feasibility parity across 31 synthetic LP/MILP families (293 trials, including 8 planted-gap MAD families and 10 spawn-dominated micro-models) against Python `PuLP + cbc` subprocess execution and in-process `BackendCBC`, 14 held-out Netlib LPs and 6 MIPLIB 3.0 MIPs solved to optimality plus a 25-node cuts/heuristics-off dual-bound check on `coinor_capplan1` (`21` fixtures in [`testdata/public/manifest.json`](testdata/public/manifest.json)), and 22 synthetic LSAP families (200 trials) against SciPy, complemented by differential fuzzing against CBC and SciPy.
- **Default float semantics**: Default uncertified mode (`WithCertified(false)` without `WithVIPRTrace`) evaluates all constraints uniformly using compiled IEEE-754 `float64(RHS - Expr.Constant)` and solver tolerances, without compile-time sub-unit pure-integer RHS tightening or exact-dyadic equality rejection, preserving parity with `PuLP`/`CBC`, `format.WriteMPS`, and `format.ToPuLPJSON`.
- **Fail-safe resource limits**: Stopping early on `WithTimeLimit`, `WithMaxNodes`, or `WithMaxIter` returns a valid global `BestBound` once the root LP solves, alongside the best feasible solution (incumbent) found; pre-root or unestablished limit exits return `-Inf` (`Minimize`) or `+Inf` (`Maximize`) in `BestBound`. Unresolved nodes never vanish silently.

### Opt-In Exact Certification

- **Exact rational witness vs. `float64` projection**: Passing `gb.WithCertified(true)` proves optimality and bounds over exact rationals $\mathbb{Q}$, and callers needing unrounded `*big.Rat` witnesses can pass `gb.WithCertifiedWitness`. Meanwhile, `Result.Values` (for continuous variables), `Duals`, `ReducedCosts`, and `Objective` hold those exact rationals rounded to nearest `float64` ($\le 0.5\text{ ULP}$; integer coordinates are exact, `BestBound` is rounded outward via `floor` for `Minimize` or `ceil` for `Maximize`, and `assign` `Result.U`/`V` are floored finite `float64` duals so $U_i + V_j \le C_{ij}$ holds over $\mathbb{Q}$). Rounded continuous `float64` `Result.Values` alone can show up to `0.5`-ULP constraint residuals when re-evaluated in `float64` even when the internal exact witness satisfies the model with zero violation.
- **Exact binary-dyadic semantics and deterministic fallback budgets**: Exact binary-dyadic $\mathbb{Q}$ semantics (including pure-integer fractional RHS tightening via `floor`/`ceil`, raw unmerged duplicate terms, exact `RHS - Expr.Constant`, and exact-$\mathbb{Q}$ equality checks) activate under `WithCertified(true)` or `WithVIPRTrace`. Non-dyadic decimal equalities (such as `-7.2*x1 - 4.1*x2 = -1` or `miplib_flugpl` row `ANZ3`) can therefore be feasible within default float tolerances yet infeasible in exact $\mathbb{Q}$. Calling `VerifyVIPRTrace` bounds fallback anchor and unboundedness checks with deterministic `100,000` LP-iteration and `5,000` B&B-node budgets instead of a wall-clock timeout. If an exact witness cannot be built or checked, the solver clears unsafe outputs and returns `ErrCertificationFailed`.
- **Unproved pure-Go runtime checkers (FlashProve boundary)**: The pure-Go runtime verifiers (`certified.go`, `exact_polish.go`, `vipr_trace.go`, `assign/certified.go`) carry no formal proof of their Go code and are validated instead by differential testing and negative controls against the standalone Lean 4 CLI checkers (including lossy `exactOrig` models via `ExactProblemRatsForTest`).
- **Zero binary tax when unused**: Functional option hooks keep checking, proof tracing, and JSON formatting decoupled from the default solve path. Go's linker strips all certification and proof-tracing symbols out of binaries that do not call those options (see [Production Binary Footprint](docs/benchmarks.md#8-production-binary-footprint-and-linker-dead-code-elimination)).

### Lean 4 Proofs and Honest Scope Boundaries

<!-- BEGIN GENERATED:README_FORMAL_SUMMARY -->
An isolated offline Lean 4 workspace ([`formal/`](formal/lakefile.toml), pinned to `v4.36.0` with `batteries` `v4.36.0` as `lintDriver`) with zero `sorry` or custom axioms proves soundness for the standalone CLI certificate checkers, abstract algebraic presolve and cutting-plane models, and compiled `arm64`/`amd64` zero-allocation kernels across `24` Lean files (`53,872` LOC: `1,325` authored `theorem` and `lemma` declarations with `1,324` in `GoblinFormal` plus `1` in `Apps/LsapCheck.lean`, `5,876` kernel `TheoremInfo` declarations including compiler-synthesized helper, equation, and splitter lemmas, and `220` `#print axioms` locks), paired with `5` pure-Go certification files (`18,254` LOC), `30` formal and certified Go test files (`33,960` LOC), and `45` `Fuzz*` targets (`37` default-build and `8` `-tags=cbc`, matching [`scripts/gcp-fuzz/targets.tsv`](scripts/gcp-fuzz/targets.tsv)). Hosted CI ([`.github/workflows/ci.yml`](.github/workflows/ci.yml)) runs Go test matrices, codegen invariants, and Lean verification (`lake build --wfail && lake lint` plus Track 5 ELF checks), [`scripts/pre-commit.sh`](scripts/pre-commit.sh) runs those checks locally (emitting a warning and skipping Lean if `lake` is absent unless `GOBLIN_STRICT_GATES=1`), [`scripts/verify_formal.sh`](scripts/verify_formal.sh) runs full formal suites and all `37` default-build `^Fuzz` seed corpora, and plain `go test ./...` skips Lean subprocess checks.
<!-- END GENERATED:README_FORMAL_SUMMARY -->

Several parts of the system stay outside formal proof scope by design:

- **Unproved Go runtime and solver code**: Default uncertified solves execute floating-point Go code without source-to-Lean lifting, and runtime `WithCertified(true)` runs unproved pure-Go rational checkers (`certified.go`, `exact_polish.go`, `vipr_trace.go`, `assign/certified.go`) rather than Lean executables.
- **Abstract standalone Lean models**: Track 2 rules, Track 3B `postsolveLPDual`, and Track 4 `BabState` formalize abstract standalone Lean models with no mechanical link to imperative `presolve.go`, `cuts.go`, or `bab.go`.
- **Out-of-scope internals**: Sparse Markowitz `LU` factorization, Forrest-Tomlin updates, simplex termination, parallel search (`WithWorkers > 1`), the Go runtime and garbage collector, and the optional C++ CBC backend are not formally proven.
- **Track 5 binary proof boundaries**: Machine-code checks run against hand-authored Lean ARM64 and x86-64 ISA decoders/operational models (trusted computing base). The `14` ARM64 routines (`7` standalone `-gcflags=-l` builds of functions inlined in production, while benchmarks run on `amd64`) and `6` `GOEXPERIMENT=simd` `amd64` AVX kernels add assurance only for uncertified solves; for certified results they add no extra soundness assurance because post-solve exact rational checking makes kernel bugs irrelevant. The `BoundsGuardsHold` hypothesis assumes panic-free runtime comparisons by hypothesis (syntactic CFG guard mediation: every edge to `runtime.panicBounds` traverses a `CMP` plus conditional branch, not value-level bounds safety; only the `4` zero-`panicBounds` ARM64 leaves prove unconditional unreachability). Zero-GC checks direct call targets up to the first external `BL` against the trusted `18`-symbol `allowedZeroGCSyms` allow-list without verifying those callees. Track 5E AVX2/AVX-512 cross-ISA proofs for `allAbsLE64` and `initReducedRow64` cover the `n % 8 == 0` vector body (outer Go wrappers handle scalar tails and `-0.0 -> +0.0` normalization, while `minNoNaN64` covers its scalar tail inline).
- **Soundness over completeness**: Certified mode ensures accepted results hold over exact rationals against the caller's original model, including continuous-variable vertex polishing and 1-ULP presolve-bound repair on mixed-integer traces. Severely ill-conditioned instances can still fail closed instead of producing a witness.

Read [Formal Verification Architecture](docs/formal-verification.md#verification-strength-and-scope-boundaries) for the full five-track proof inventory, trusted computing base, soundness bug ledger, and Lean commands.

---

## Optional Backends and Build Flags

- **Default pure-Go build (`BackendGo`)**: Requires no `cgo` and runs single-threaded by default. Pass `gb.WithWorkers(n)` for parallel Branch-and-Cut.
- **Experimental SIMD (`GOEXPERIMENT=simd`)**: Enables runtime-dispatched AVX2 and AVX-512 kernels on `amd64`, with `gb.WithoutSIMD()` available at runtime. Most measurable wins occur in dense `goblin/assign` workloads.
- **Optional COIN-OR CBC (`-tags cbc`)**: Vendored COIN-OR sources sit behind a nested `third_party/go.mod`, keeping plain `go get` downloads small. Run `./scripts/build_cbc.sh` and build with `-tags cbc` (or `-tags "cbc,cbc_system"` against system COIN-OR libraries), then pass `gb.WithCBC()`.

Full configuration flags and backend semantics are documented in [Solver Settings and Backends](docs/modeling-and-formats.md#functional-solver-settings-and-backends).

---

## Documentation

| Guide | Contents |
| :--- | :--- |
| [Architecture Overview](docs/index.md) | Module layers, dual CSR/CSC sparse matrix layout, and pooled workspaces. |
| [Modeling API and Formats](docs/modeling-and-formats.md) | Variables, expressions, solver options, `PuLP` JSON streaming, MPS export, and `goblin/assign`. |
| [Solver Internals](docs/solver-internals.md) | Sparse Markowitz LU, dual/primal simplex, presolve, cuts, heuristics, rejected designs, and known limits. |
| [Benchmarks and Telemetry](docs/benchmarks.md) | Hardware provenance, all 31 MILP and 22 LSAP families, SIMD tables, Go vs. CBC analysis, binary size, and reproduction commands. |
| [Formal Verification](docs/formal-verification.md) | Five-track Lean 4 proof suite, trust boundaries, formal fuzzing, soundness bug ledger, and verification protocol. |

---

## Development and Testing

Run core pure-Go test suites from the `goblin/` directory (`go test ./...` skips Lean subprocess checks unless `GOBLIN_FORMAL=1` is set):

```bash
go test ./...
GOEXPERIMENT=simd go test ./...
go test -tags=cbc ./...
go run ./cmd/goblin-bench -check-docs
```

Hosted CI ([`.github/workflows/ci.yml`](.github/workflows/ci.yml)) runs Go linting, compiler invariant verification, multi-configuration tests, Lean 4 builds (`lake build --wfail && lake lint`), Track 5 ELF checks, and formal differential fuzz corpora. Locally, running `./scripts/pre-commit.sh` adds `lake build --wfail && lake lint`, Track 5 ELF binary checks, and `GOBLIN_FORMAL=1 go test -short -run '^TestFormal' . ./assign` whenever `lake` is installed (emitting a warning and skipping Lean gates if `lake` is absent unless `GOBLIN_STRICT_GATES=1`). Complete benchmark regeneration, telemetry sync, differential testing, and manual full Lean 4 commands (`./scripts/verify_formal.sh`) live in [Reproducing Benchmarks](docs/benchmarks.md#10-reproducing-benchmarks-and-syncing-documentation) and [Formal Verification Protocol](docs/formal-verification.md#5-verification-protocol).

---

## License

The `goblin` module and its pure-Go default build (`BackendGo`, `goblin/assign`, `goblin/format`) are licensed under the [MIT License](LICENSE).

Optional vendored COIN-OR sources under `third_party/` (`Cbc`, `Cgl`, `Clp`, `CoinUtils`, and `Osi`, pinned in [`third_party/VERSIONS`](third_party/VERSIONS) and excluded from standard `go get` downloads via `third_party/go.mod`) and adapted COIN-OR fixtures in [`cbc_ported_test.go`](cbc_ported_test.go) are licensed under the Eclipse Public License 2.0 (`EPL-2.0`; see `third_party/*/LICENSE`). When distributing binaries compiled with `-tags cbc` that link those libraries, recipients may obtain the corresponding COIN-OR source code from `third_party/` in `https://github.com/paulourio/goblin` (or from `https://github.com/coin-or`) under `EPL-2.0` Section 3.
