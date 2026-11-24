# goblin

The `goblin` module is a pure-Go solver for Linear Programming (LP), Mixed-Integer Linear Programming (MILP), and Linear Sum Assignment Problems (LSAP). It runs inside your Go process with zero `cgo` dependencies, no Python runtime, and no external solver binaries. You can build algebraic models in Go, stream Python `PuLP` JSON payloads directly into memory, or export standard MPS files.

- **Pure Go by default**: Compiles cleanly with `CGO_ENABLED=0` and reuses pooled memory workspaces across solves. No subprocess launches or temporary files sit between your application and the solver.
- **Two focused packages**: General LP and MILP modeling lives in the root package (`gb`), while dense and sparse bipartite assignment matrices run directly through [`goblin/assign`](assign/assign.go).
- **Opt-in exact verification**: Passing `gb.WithCertified(true)` or `assign.WithCertified(true)` validates results in-process using pure-Go exact `*big.Rat` and `dyadic2304` checkers before returning (Lean is never invoked at runtime). Unused certification code disappears completely at link time.
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
Across the 14 solver-dominated Tier 2+3 non-planted MILP families, pure-Go `BackendGo` achieves a **11.40x** [6.44x-20.19x] equal-family geometric-mean total speedup against the Python `PuLP + cbc` subprocess baseline and **4.47x** [2.86x-7.00x] total (**4.67x** [2.91x-7.50x] solve) against in-process C++ `BackendCBC`; Python MPS serialization and process spawn account for the difference between those two ratios.
<!-- END GENERATED:README_CBC_HEADLINE -->

<!-- BEGIN GENERATED:README_MILP_SUMMARY_TABLE -->
| Workload | Families (Trials) | Go Sum (ms) | Python Sum (ms) | Speedup (GeoMean [95% CI]) | Per-Instance Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro-models (`N <= 26`, spawn-dominated) | 10 (110) | 9.5 | 1074.1 | **407.29x** [174.09x-952.90x] | 690.29x [37.43x-2334.72x] |
| Tier 2: Medium MILPs (`30 <= N <= 84`) | 13 (143) | 739.5 | 4384.3 | **17.17x** [9.19x-32.08x] | 24.09x [0.13x-148.30x] |
| Tier 3: Large MILPs (`145 <= N <= 4036`) | 8 (40) | 800.4 | 6292.1 | **10.13x** [5.24x-19.61x] | 10.01x [1.15x-77.02x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | 1223.2 | 6890.4 | **11.40x** [6.44x-20.19x] | 12.01x [0.13x-148.30x] |
| All 31 synthetic families | 31 (293) | 1549.3 | 11750.5 | **41.62x** [20.75x-83.49x] | 40.78x [0.13x-2334.72x] |
<!-- END GENERATED:README_MILP_SUMMARY_TABLE -->

Full provenance, per-family breakdowns across all 31 families, solver-core checkpoints, and pure-Go vs. in-process C++ CBC analysis live in [Benchmark Methodology and Corpus Results](docs/benchmarks.md).

### Group-Item Allocation (MAD)

The synthetic Mean Absolute Deviation (`MAD`) family assigns $I$ items across $G$ groups over 6 periods to keep each group close to a rounded target level. Structured instances (`t*_mad_g*`) carry a zero root LP gap by design; random-weight controls (`*_rand_*`) remove that structure so branch-and-cut must search the tree. Each row reports median end-to-end times (`JSON -> solution`) across randomized trials and paired geometric-mean speedup (`Python / Go`):

<!-- BEGIN GENERATED:README_MAD_SUMMARY_TABLE -->
| Problem | $G \times I$ | Trials | Go (ms) | Python (ms) | Speedup |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | 0.09 | 9.43 | **95.4x** |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | 0.54 | 34.38 | **56.8x** |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | 0.22 | 9.66 | **46.6x** |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | 0.18 | 10.28 | **55.0x** |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | 0.37 | 11.47 | **30.5x** |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | 3.57 | 48.22 | **9.5x** |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | 16.87 | 169.12 | **7.2x** |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | 0.66 | 18.51 | **26.7x** |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | 1.03 | 19.00 | **16.2x** |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | 8.55 | 62.43 | **5.8x** |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | 34.81 | 375.00 | **10.1x** |
<!-- END GENERATED:README_MAD_SUMMARY_TABLE -->

**Note on SIMD**: Experimental `GOEXPERIMENT=simd` builds deliver little overall speedup on MAD and other sparse LP/MILP workloads because scalar matrix factorization dominates runtime. Full dispersion (`Mean ± SD`), prep/solve splits, checkpoint progression, and paired scalar-vs-SIMD benchmarks appear in [MAD Benchmarks](docs/benchmarks.md#3-mad-group-item-allocation-formulation) and [Default Build vs. `GOEXPERIMENT=simd`](docs/benchmarks.md#5-default-build-vs-goexperimentsimd).

### Linear Sum Assignment

The `goblin/assign` corpus compares in-memory pure-Go execution directly against SciPy's compiled C++ `rectangular_lsap` extension across 22 square, rectangular, geometric, tied, rank-1, and sparse matrix families (cumulative solver wall times and paired `SciPy / Go` solve speedups):

<!-- BEGIN GENERATED:README_ASSIGN_SUMMARY_TABLE -->
| Workload | Families (Trials) | Go Sum (ms) | SciPy Sum (ms) | Speedup (GeoMean [95% CI]) | Per-Instance Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Small (`N <= 32`) | 6 (66) | 0.217 | 0.318 | **1.48x** [1.18x-1.86x] | 1.50x [0.84x-2.65x] |
| Tier 2: Medium (`64 <= N <= 256`) | 9 (99) | 9.242 | 32.915 | **2.98x** [1.89x-4.71x] | 2.66x [0.85x-15.17x] |
| Tier 3: Large (`500 <= N <= 2000`) | 7 (35) | 216.804 | 940.999 | **2.87x** [1.87x-4.41x] | 2.59x [1.50x-6.16x] |
| **All 22 synthetic LSAP families** | 22 (200) | 226.263 | 974.231 | **2.44x** [1.91x-3.11x] | 2.08x [0.84x-15.17x] |
<!-- END GENERATED:README_ASSIGN_SUMMARY_TABLE -->

Unlike sparse MILPs, dense assignment matrices benefit substantially from AVX2 and AVX-512 vectorization under `GOEXPERIMENT=simd`. Per-family results, square vs. rectangular splits, and scalar-vs-SIMD tables live in [Linear Sum Assignment Benchmarks](docs/benchmarks.md#71-aggregate-lsap-workload-summary) and [LSAP SIMD Analysis](docs/benchmarks.md#73-lsap-scalar-vs-goexperimentsimd).

---

## Correctness and Guarantees

Optimization solvers balance numerical speed against mathematical certainty. The `goblin` module separates fast default execution from opt-in exact verification so callers pay for certification only when needed.

### Default Solver Guarantees

- **Verified parity**: Automated tests check status, objective, and feasibility parity across 31 synthetic LP/MILP families (293 trials, including 8 planted-gap MAD families and 10 spawn-dominated micro-models) against Python `PuLP + cbc` subprocess execution and in-process `BackendCBC`, 14 held-out Netlib LPs and 6 MIPLIB 3.0 MIPs solved to optimality plus a 25-node cuts/heuristics-off dual-bound check on `coinor_capplan1` (`21` fixtures in [`testdata/public/manifest.json`](testdata/public/manifest.json)), and 22 synthetic LSAP families (200 trials) against SciPy, complemented by differential fuzzing against CBC and SciPy.
- **Fail-safe resource limits**: Stopping early on `WithTimeLimit`, `WithMaxNodes`, or `WithMaxIter` returns a valid global `BestBound` once the root LP solves, alongside the best feasible solution (incumbent) found; pre-root or unestablished limit exits return `-Inf` (`Minimize`) or `+Inf` (`Maximize`) in `BestBound`. Unresolved nodes never vanish silently.

### Opt-In Exact Certification

- **Fail-closed rational checking**: Passing `gb.WithCertified(true)` or `assign.WithCertified(true)` runs pure-Go `*big.Rat` or `dyadic2304` checkers in-process at runtime (Lean is never invoked or linked at runtime), validating optimality (zero rational gap by default), outward-rounded `BestBound` values, infeasibility, unboundedness, or dual bounds directly against your unmerged original `Model` terms and constants (tightening pure-integer rows with sub-unit fractional right-hand-side residuals at compile time, while mismatched `Dot` slice lengths return a deferred validation error). If an exact witness cannot be built or verified, the solver clears unsafe outputs and returns `ErrCertificationFailed`. Equivalence with the standalone Lean 4 CLI verifiers is validated offline and under `GOBLIN_FORMAL=1`.
- **Zero binary tax when unused**: Functional option hooks keep verification, proof tracing, and JSON formatting decoupled from the default solve path. Go's linker strips all certification and proof-tracing symbols out of binaries that do not call those options (see [Production Binary Footprint](docs/benchmarks.md#8-production-binary-footprint-and-linker-dead-code-elimination)).

### Lean 4 Proofs and Honest Scope Boundaries

An isolated offline Lean 4 workspace ([`formal/`](formal/lakefile.toml)) with zero external dependencies and zero `sorry` or custom axioms proves soundness for the standalone CLI certificate checkers, abstract algebraic presolve and cutting-plane rules, and compiled `arm64`/`amd64` zero-allocation kernels, validated against Go offline via `scripts/verify_formal.sh` and `GOBLIN_FORMAL=1` differential fuzzing (while `scripts/pre-commit.sh` runs `lake build`, Track 5 ELF verification, and `-short` `TestFormal*` tests when `lake` is present, and plain `go test ./...` skips Lean subprocess checks).

Several parts of the system stay outside formal proof scope by design:

- **Default uncertified solves** execute floating-point Go code without source-to-Lean lifting, and runtime `WithCertified(true)` runs pure-Go rational checkers rather than Lean executables.
- **Out-of-scope internals**: Sparse Markowitz `LU` factorization, Forrest-Tomlin updates, simplex termination, parallel search (`WithWorkers > 1`), the Go runtime and garbage collector, and the optional C++ CBC backend are not formally proven.
- **Track 5 binary proof boundaries**: Machine-code proofs check against hand-authored Lean ARM64 and x86-64 ISA decoders/operational models (part of the trusted computing base). All `14` ARM64 routines target `arm64` ELFs (not `amd64`), with `4` Track 5A helpers and `3` Track 5D leaf helpers proved standalone under `-gcflags=-l` whereas production builds inline them; `Zero-GC` checks direct calls against a minimal allow-list (`allowedZeroGCSyms`) including stack-split prologues (`runtime.morestack*`) and non-blocking cancel polls (`runtime.selectnbrecv`); panic-bounds proofs give unconditional unreachability only on zero-`panicBounds` ARM64 routines while routines retaining slice-entry guards prove structural CFG guard mediation (`checkPanicBoundsReachability_run_safe`); and Track 5E AVX2/AVX-512 cross-ISA proofs for `allAbsLE64` and `initReducedRow64` cover the `n % 8 == 0` vector body (outer Go wrappers handle scalar tails and `-0.0 -> +0.0` normalization, while `minNoNaN64` covers its scalar tail inline).
- **Soundness over completeness**: Certified mode guarantees that accepted results are mathematically valid over exact rationals against the caller's original model, including continuous-variable vertex polishing and 1-ULP presolve-bound repair on mixed-integer traces. Severely ill-conditioned instances can still fail closed instead of producing a witness.

Read [Formal Verification Architecture](docs/formal-verification.md#verification-strength-and-scope-boundaries) for the full five-track proof inventory, trusted computing base, soundness bug ledger, and Lean verification commands.

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

Running `./scripts/pre-commit.sh` adds `lake build --wfail`, Track 5 ELF binary verification, and `GOBLIN_FORMAL=1 go test -short -run '^TestFormal' . ./assign` whenever `lake` is installed (emitting a warning if `lake` is absent unless `GOBLIN_STRICT_GATES=1`). Complete benchmark regeneration, telemetry sync, differential testing, and full Lean 4 verification commands (`./scripts/verify_formal.sh`) live in [Reproducing Benchmarks](docs/benchmarks.md#10-reproducing-benchmarks-and-syncing-documentation) and [Formal Verification Protocol](docs/formal-verification.md#5-verification-protocol).

---

## License

The `goblin` module and its pure-Go default build (`BackendGo`, `goblin/assign`, `goblin/format`) are licensed under the [MIT License](LICENSE).

Optional vendored COIN-OR sources under `third_party/` (`Cbc`, `Cgl`, `Clp`, `CoinUtils`, and `Osi`, pinned in [`third_party/VERSIONS`](third_party/VERSIONS) and excluded from standard `go get` downloads via `third_party/go.mod`) and adapted COIN-OR fixtures in [`cbc_ported_test.go`](cbc_ported_test.go) are licensed under the Eclipse Public License 2.0 (`EPL-2.0`; see `third_party/*/LICENSE`). When distributing binaries compiled with `-tags cbc` that link those libraries, recipients may obtain the corresponding COIN-OR source code from `third_party/` in `https://github.com/paulourio/goblin` (or from `https://github.com/coin-or`) under `EPL-2.0` Section 3.
