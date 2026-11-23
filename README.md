# goblin

The `goblin` module is a pure-Go solver for Linear Programming (LP), Mixed-Integer Linear Programming (MILP), and Linear Sum Assignment Problems (LSAP). It runs inside your Go process with zero `cgo` dependencies, no Python runtime, and no external solver binaries. You can build algebraic models in Go, stream Python `PuLP` JSON payloads directly into memory, or export standard MPS files.

- **Pure Go by default**: Compiles cleanly with `CGO_ENABLED=0` and reuses pooled memory workspaces across solves. No subprocess launches or temporary files sit between your application and the solver.
- **Two focused packages**: General LP and MILP modeling lives in the root package (`gb`), while dense and sparse bipartite assignment matrices run directly through [`goblin/assign`](assign/assign.go).
- **Opt-in exact verification**: Passing `gb.WithCertified(true)` or `assign.WithCertified(true)` validates results in exact rational arithmetic before returning. Unused certification code disappears completely at link time.
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

Keep two corpus properties in mind when reading LP and MILP speedups. First, Tier 1 micro-models are dominated by Python's CBC subprocess startup cost. Second, planted-gap MAD models have a zero root LP gap by construction and close without branching. The bold `Tier 2+3 non-planted MILPs` row excludes both groups to isolate solver-dominated branching workloads. Columns report cumulative end-to-end wall time (`JSON -> solution`) summed across all randomized trials in each subset alongside equal-family geometric-mean speedup (`Python / Go`):

<!-- BEGIN GENERATED:README_MILP_SUMMARY_TABLE -->
| Workload | Families (Trials) | Go Sum (ms) | Python Sum (ms) | Speedup (GeoMean [95% CI]) | Per-Instance Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Micro-models (`N <= 26`, spawn-dominated) | 10 (110) | 8.8 | 1093.8 | **354.04x** [169.35x-740.14x] | 470.29x [38.16x-2964.39x] |
| Tier 2: Medium MILPs (`30 <= N <= 84`) | 13 (143) | 727.6 | 4352.8 | **17.77x** [9.45x-33.40x] | 24.48x [0.14x-171.70x] |
| Tier 3: Large MILPs (`145 <= N <= 4036`) | 8 (40) | 778.9 | 6251.8 | **10.58x** [5.30x-21.14x] | 10.29x [1.15x-73.24x] |
| **Tier 2+3 non-planted MILPs (solver-dominated)** | 14 (130) | 1200.1 | 6841.6 | **11.69x** [6.58x-20.77x] | 12.29x [0.14x-171.70x] |
| All 31 synthetic families | 31 (293) | 1515.3 | 11698.5 | **40.81x** [21.05x-79.10x] | 44.42x [0.14x-2964.39x] |
<!-- END GENERATED:README_MILP_SUMMARY_TABLE -->

Full provenance, per-family breakdowns across all 31 families, solver-core checkpoints, and pure-Go vs. in-process C++ CBC analysis live in [Benchmark Methodology and Corpus Results](docs/benchmarks.md).

### Group-Item Allocation (MAD)

The synthetic Mean Absolute Deviation (`MAD`) family assigns $I$ items across $G$ groups over 6 periods to keep each group close to a rounded target level. Structured instances (`t*_mad_g*`) carry a zero root LP gap by design; random-weight controls (`*_rand_*`) remove that structure so branch-and-cut must search the tree. Each row reports median end-to-end times (`JSON -> solution`) across randomized trials and paired geometric-mean speedup (`Python / Go`):

<!-- BEGIN GENERATED:README_MAD_SUMMARY_TABLE -->
| Problem | $G \times I$ | Trials | Go (ms) | Python (ms) | Speedup |
| :--- | :---: | ---: | ---: | ---: | ---: |
| `t1_mad_g2_i4` | $2 \times 4$ | 11 | 0.06 | 9.96 | **155.9x** |
| `t1_mad_rand_g2_i4` | $2 \times 4$ | 11 | 0.48 | 34.19 | **64.9x** |
| `t2_mad_g2_i8` | $2 \times 8$ | 11 | 0.16 | 9.26 | **56.8x** |
| `t2_mad_g3_i9` | $3 \times 9$ | 11 | 0.17 | 9.91 | **54.7x** |
| `t2_mad_g4_i12` | $4 \times 12$ | 11 | 0.41 | 11.42 | **28.7x** |
| `t2_mad_rand_g2_i8` | $2 \times 8$ | 11 | 3.52 | 48.07 | **9.6x** |
| `t2_mad_rand_g3_i9` | $3 \times 9$ | 11 | 16.45 | 168.49 | **7.2x** |
| `t3_mad_g5_i20` | $5 \times 20$ | 5 | 0.55 | 18.96 | **32.9x** |
| `t3_mad_g6_i30` | $6 \times 30$ | 5 | 1.01 | 18.77 | **17.0x** |
| `t3_mad_g10_i100` | $10 \times 100$ | 5 | 8.44 | 60.50 | **6.0x** |
| `t3_mad_g4_i1000` | $4 \times 1000$ | 5 | 34.20 | 373.18 | **10.4x** |
<!-- END GENERATED:README_MAD_SUMMARY_TABLE -->

**Note on SIMD**: Experimental `GOEXPERIMENT=simd` builds deliver little overall speedup on MAD and other sparse LP/MILP workloads because scalar matrix factorization dominates runtime. Full dispersion (`Mean ± SD`), prep/solve splits, checkpoint progression, and paired scalar-vs-SIMD benchmarks appear in [MAD Benchmarks](docs/benchmarks.md#3-mad-group-item-allocation-formulation) and [Default Build vs. `GOEXPERIMENT=simd`](docs/benchmarks.md#5-default-build-vs-goexperimentsimd).

### Linear Sum Assignment

The `goblin/assign` corpus compares in-memory pure-Go execution directly against SciPy's compiled C++ `rectangular_lsap` extension across 22 square, rectangular, geometric, tied, rank-1, and sparse matrix families (cumulative solver wall times and paired `SciPy / Go` solve speedups):

<!-- BEGIN GENERATED:README_ASSIGN_SUMMARY_TABLE -->
| Workload | Families (Trials) | Go Sum (ms) | SciPy Sum (ms) | Speedup (GeoMean [95% CI]) | Per-Instance Median [Min-Max] |
| :--- | ---: | ---: | ---: | ---: | ---: |
| Tier 1: Small (`N <= 32`) | 6 (66) | 0.218 | 0.314 | **1.37x** [1.15x-1.64x] | 1.39x [0.27x-2.60x] |
| Tier 2: Medium (`64 <= N <= 256`) | 9 (99) | 9.012 | 32.558 | **2.97x** [1.84x-4.80x] | 2.55x [0.89x-16.56x] |
| Tier 3: Large (`500 <= N <= 2000`) | 7 (35) | 224.030 | 942.220 | **2.83x** [1.80x-4.45x] | 2.70x [1.01x-6.13x] |
| **All 22 synthetic LSAP families** | 22 (200) | 233.260 | 975.092 | **2.37x** [1.83x-3.07x] | 2.04x [0.27x-16.56x] |
<!-- END GENERATED:README_ASSIGN_SUMMARY_TABLE -->

Unlike sparse MILPs, dense assignment matrices benefit substantially from AVX2 and AVX-512 vectorization under `GOEXPERIMENT=simd`. Per-family results, square vs. rectangular splits, and scalar-vs-SIMD tables live in [Linear Sum Assignment Benchmarks](docs/benchmarks.md#71-aggregate-lsap-workload-summary) and [LSAP SIMD Analysis](docs/benchmarks.md#73-lsap-scalar-vs-goexperimentsimd).

---

## Correctness and Guarantees

Optimization solvers balance numerical speed against mathematical certainty. The `goblin` module separates fast default execution from opt-in exact verification so callers pay for certification only when needed.

### Default Solver Guarantees

- **Verified parity**: Automated tests check status, objective, and feasibility parity across 293 synthetic LP/MILP trials, 20 held-out public Netlib LP and MIPLIB 3.0 MILP instances, and 200 LSAP trials, complemented by differential fuzzing against CBC and SciPy.
- **Fail-safe resource limits**: Stopping early on `WithTimeLimit`, `WithMaxNodes`, or `WithMaxIter` returns a valid global `BestBound` once the root LP solves, alongside the best feasible solution (incumbent) found. Unresolved nodes never vanish silently.

### Opt-In Exact Certification

- **Fail-closed rational checking**: Passing `gb.WithCertified(true)` or `assign.WithCertified(true)` validates optimality (zero rational gap by default), infeasibility, unboundedness, or dual bounds in exact rational arithmetic directly against your original problem. If an exact witness cannot be built or verified, the solver clears unsafe outputs and returns `ErrCertificationFailed`.
- **Zero binary tax when unused**: Functional option hooks keep verification, proof tracing, and JSON formatting decoupled from the default solve path. Go's linker strips all certification and proof-tracing symbols out of binaries that do not call those options (see [Production Binary Footprint](docs/benchmarks.md#8-production-binary-footprint-and-linker-dead-code-elimination)).

### Lean 4 Proofs and Honest Scope Boundaries

An isolated Lean 4 workspace ([`formal/`](formal/lakefile.toml)) with zero external dependencies and zero `sorry` or custom axioms provides machine-checked proofs for the certificate checkers, abstract algebraic presolve and cutting-plane rules, and compiled `arm64`/`amd64` zero-allocation kernels.

Several parts of the system stay outside formal proof scope by design:

- **Default uncertified solves** execute floating-point Go code without source-to-Lean lifting.
- **Out-of-scope internals**: Sparse Markowitz `LU` factorization, Forrest-Tomlin updates, simplex termination, parallel search (`WithWorkers > 1`), the Go runtime and garbage collector, and the optional C++ CBC backend are not formally proven.
- **Soundness over completeness**: Certified mode guarantees that accepted results are mathematically valid over exact rationals. Severely ill-conditioned instances can still fail closed instead of producing a witness.

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

Run core test suites from the `goblin/` directory:

```bash
go test ./...
GOEXPERIMENT=simd go test ./...
go test -tags=cbc ./...
go run ./cmd/goblin-bench -check-docs
```

Complete benchmark regeneration, telemetry sync, differential testing, and Lean 4 verification commands live in [Reproducing Benchmarks](docs/benchmarks.md#10-reproducing-benchmarks-and-syncing-documentation) and [Formal Verification Protocol](docs/formal-verification.md#5-verification-protocol).

---

## License

The `goblin` module and its pure-Go default build (`BackendGo`, `goblin/assign`, `goblin/format`) are licensed under the [MIT License](LICENSE).

Optional vendored COIN-OR sources under `third_party/` (`Cbc`, `Cgl`, `Clp`, `CoinUtils`, and `Osi`, pinned in [`third_party/VERSIONS`](third_party/VERSIONS) and excluded from standard `go get` downloads via `third_party/go.mod`) and adapted COIN-OR fixtures in [`cbc_ported_test.go`](cbc_ported_test.go) are licensed under the Eclipse Public License 2.0 (`EPL-2.0`; see `third_party/*/LICENSE`). When distributing binaries compiled with `-tags cbc` that link those libraries, recipients may obtain the corresponding COIN-OR source code from `third_party/` in `https://github.com/paulourio/goblin` (or from `https://github.com/coin-or`) under `EPL-2.0` Section 3.
