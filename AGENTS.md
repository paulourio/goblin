# Goblin Engineering and Codegen Invariants

Every change to solver, SIMD, assignment, or serialization hot paths MUST preserve the Go 1.27 compiler codegen invariants enforced by `cmd/goblin-optcheck -check-invariants`, pass `scripts/pre-commit.sh` across all build tags, and validate hot-path latency with controlled interleaved AB/BA benchmarks on a pinned core.

## 1. Go Compiler and Binary Codegen Invariants

Automated checks in `cmd/goblin-optcheck/invariants.go` and `TestCodegenInvariants` enforce these locks on every `go test ./...` and `GOEXPERIMENT=simd go test ./...` run.

### 1.1 Four-Way Unrolled Scalar Loops (`internal/simd/`)
- **Required idiom**: Re-slice inputs to equal lengths tied directly to the primary slice length, then iterate with `for ; j <= len(cSlice)-4; j += 4` followed by `for ; j < len(cSlice); j++`.
- **Rejected pattern**: Writing `lim := n - 3; for ; j < lim; j += 4` or `cTail := cSlice[j:]` after re-slicing breaks Go 1.27 SSA `prove` bounds-check elimination on re-sliced slices (`cSlice`, `vSlice`, `sSlice`, `rSlice`, `dstRow`). During benchmarking on `Min2ReducedRow64`, this pattern reintroduced four `IsInBounds` checks inside the unrolled loop and two `IsSliceInBounds` checks on the tail, regressing `Min2ReducedRow64` by 15% to 38% and downstream `assign` solves by 5% to 21%.
- **Guarded kernels**: `AllAbsLE64Scalar`, `Min2ReducedRow64Scalar` / `Min2ReducedRow64`, `RelaxRow64Scalar`, `MinNoNaN64Scalar`, `ValidateAndNegate64Scalar`, `ColReduceValidateRows64Scalar`, and `ColReduceRows64Scalar` must emit zero bounds checks inside their `+= 4` loop bodies.

### 1.2 Leaf Function Inlining Budget (`cost <= 80`)
- Keep these hot leaf helpers strictly within Go's inlining budget (`cost <= 80`):
  - `cuts.go`: `nextUp`, `nextDown`
  - `presolve.go`: `isFractionalFloat64`
  - `simplex.go`: `combineDualNoiseTol`
  - `lu.go`: `(*LUFactorization).removeURowEntry`, `(*LUFactorization).removeUColEntry`
  - `format/pulp_json_fast.go`: `(*pulpScanner).consumeByte`, `(*pulpScanner).readUnescapedBytes`
  - `internal/simd/kernels_amd64.go`: `evalDualLeaveRow`
- Replace `math.Nextafter` and `math.Trunc` on hot paths with `nextUp`, `nextDown`, and `isFractionalFloat64` to avoid non-inlined stdlib calls and bit-exact overhead.
- Avoid slice-capturing inner closures on hot paths (`selectDualLeaveRow64Vec` uses the top-level helper `evalDualLeaveRow` so the compiler inlines it cleanly without closure capture state).

### 1.3 Cold Wide-Accumulator Stack Frame Isolation (`//go:noinline`)
- Keep `wideDotAcc` and compensated fallback arrays inside dedicated `//go:noinline` helpers so fast paths do not reserve 1 KB to 3 KB stack frames on every call:
  - `simplex.go`: `recomputeWorkingRowResidualWide` (`recomputeWorkingRowResidual` stays `<= 512` B), `evalColDualDotWide` (`evalColDualDot` stays `<= 400` B)
  - `presolve.go`: `evalBinaryTightenRHSWide`, `evalRowActivityWide` (`evalRowActivitySummaryInto` stays `<= 256` B)
  - `heuristics.go`: `evalEpigraphCompObjWide`

### 1.4 Hot-Path Defer and `growslice` Avoidance
- Keep `defer` out of hot loops and high-frequency update paths. The method `(*LUFactorization).pushFTFromSpike` wraps `pushFTFromSpikeCore` with zero `runtime.deferreturn` calls and a frame size `<= 64` B.
- Pre-size pooled workspace slices (`workspace.go`) and use explicit write cursors (`outLen`) instead of `append` where disassembly reveals `runtime.growslice` calls. The method `(*LPSolver).ComputeTableauRow` must keep `runtime.growslice` calls at zero.

### 1.5 Vector ISA Portability (`GOAMD64=v1`) and CFG Safety
- Default binaries must run safely on `GOAMD64=v1` CPUs without AVX2 or AVX-512.
- Every EVEX (`%zmm`, opmask) instruction in `internal/simd` must live inside dedicated `*AVX512` helpers gated behind runtime CPU feature checks (`hasAVX512`).
- Every SIMD function touching 256-bit `YMM` or 512-bit `ZMM` registers must execute `vzeroupper` before returning or invoking normal calls, with zero legacy SSE (`%xmm` without `v` prefix) instructions while upper vector state is dirty. Verify with `go run ./cmd/goblin-optcheck -check-invariants -simd`.
- Preserve `twoProd` / `twoSum` floating-point non-contraction invariants and IEEE-754 `NaN` propagation parity across scalar, AVX2, and AVX-512 tiers.

## 2. Verification and Benchmarking Workflow

Execute these checks from the repository root before committing:

```bash
# 1. Verify compiler inlining, BCE, frame/defer/growslice, and vector CFG locks
go run ./cmd/goblin-optcheck -check-invariants
go run ./cmd/goblin-optcheck -check-invariants -simd

# 2. Run formatting, multi-tag lint, comment prose gate, and test suites
./scripts/pre-commit.sh
```

- **Host CPU quiescence gate before local benchmarks**: Before running any local wall-clock benchmark, CPU profile, or telemetry refresh (`cmd/goblin-bench`, `go test -bench`), inspect active CPU utilization (`top -bn1` or a 1s `/proc/stat` delta), `/proc/pressure/cpu`, and top CPU consumers (`ps -eo pid,pcpu,comm --sort=-pcpu`). Do not rely solely on raw `/proc/loadavg` because idle `io_uring` waiters can inflate it without real CPU contention. If the host is under significant CPU load (compilers, encoders, fuzzers, or thermal throttling), stop timing runs immediately, report the active consumers, and wait for the operator to confirm the machine is quiet before measuring. Deterministic checks (unit tests, `goblin-optcheck`, iteration counts) do not require an idle host.
- **Controlled AB/BA benchmarking**: Never judge a hot-path optimization from static BCE counts alone. Once the host is quiet, freeze baseline `.test` binaries in a scratch directory outside the repo, build post-edit `.test` binaries, and run at least 5 interleaved AB/BA rounds pinned to an idle performance core (`taskset -c <core> <bin> -test.cpu=1 -test.benchmem`).
- **Native AVX-512 runtime verification**: When testing or benchmarking AVX-512 kernels from a host CPU lacking `avx512f`, compile static `CGO_ENABLED=0 GOEXPERIMENT=simd` test binaries (`GOAMD64=v1`, `v3`, `v4`) and execute them on an ephemeral `n4-standard-4` (`hyperdisk-balanced`, `--no-service-account --no-scopes`) GCE VM, tearing down the VM via an `EXIT` trap upon completion. Resolve the GCP project, region, and zone strictly at runtime from environment variables (`GCP_PROJECT`, `GCP_REGION`, `GCP_ZONE`) or the operator's active `gcloud` CLI configuration (`gcloud config get-value project` and `compute/region`). Never hardcode project IDs, regions, zones, bucket names, service account emails, internal hosts, or local home paths in repository files, scripts, or documentation.
- **Telemetry provenance**: Commit all solver code changes before regenerating `bench/results/*.json` and running `go run ./cmd/goblin-bench -sync-docs`, keeping `git status --porcelain -- . :(exclude)bench/results` empty so every telemetry file records `git_dirty=false`.
- **Pre-commit hygiene**: Never commit `.test`, `.prof`, or `.cache/` artifacts into the repository, never bypass `.git/hooks/pre-commit` with `--no-verify`, and never introduce hardcoded user home paths or private cloud identifiers.
