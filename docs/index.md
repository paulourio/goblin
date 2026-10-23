# `gopt` Architecture and Design Specification

The `gopt` solver separates algebraic model construction, sparse matrix compilation, numerical optimization, and file-format serialization into four isolated layers. Each layer communicates through immutable data containers (`CompiledProblem`, `Result`, `LPResult`) so that concurrent workers can solve independent problem instances without shared mutable state.

---

## 1. Architectural Layer Separation

```mermaid
flowchart LR
    A["format.FromPuLPJSON / Model API"] --> B["Model.Compile()"]
    B --> C["CompiledProblem (CSR + CSC)"]
    C --> D["Presolve (GCD + Savelsbergh)"]
    D --> E["LPSolver (Dual / Primal Simplex + LU)"]
    E --> F["solveBranchAndCut (Cuts + Heuristics + B&B)"]
    F --> G["Result (Primal + Dual + Telemetry)"]
```

| Layer | Source Files | Core Responsibility |
| :--- | :--- | :--- |
| **1. Modeling & Compilation** | [`model.go`](../model.go), [`expr.go`](../expr.go), [`compiled.go`](../compiled.go) | Collects variables, linear expressions, and constraints, validates variable ownership and finite coefficients, deduplicates terms, and compiles immutable `CSR` and `CSC` sparse matrices (`CompiledProblem`). |
| **2. Linear Algebra & LP Engine** | [`lu.go`](../lu.go), [`simplex.go`](../simplex.go), [`workspace.go`](../workspace.go), [`internal/simd/`](../internal/simd/) | Executes singleton-preordered sparse Markowitz `LU` factorization with PFI eta updates, Revised Dual Simplex with `FTRANSparse`/`BTRANSparse` and bound flipping, and Two-Phase Primal Simplex over pooled `Workspace` buffers. |
| **3. MILP Branch-and-Cut** | [`presolve.go`](../presolve.go), [`cuts.go`](../cuts.go), [`heuristics.go`](../heuristics.go), [`bab.go`](../bab.go), [`solve.go`](../solve.go) | Performs MIP presolve, multi-family cutting-plane generation, primal heuristics, and pseudocost/strong-branching search. |
| **4. Serialization & Benchmarking** | [`format/pulp_json.go`](../format/pulp_json.go), [`format/mps.go`](../format/mps.go), [`bench/corpus.go`](../bench/corpus.go), [`bench/docgen.go`](../bench/docgen.go) | Parses and emits `PuLP` JSON and MPS formats, builds the 31-family multi-seed benchmark corpus (293 randomized trials), and synchronizes markdown documentation from JSON telemetry. |

---

## 2. Core Data Invariants

### Dual-Index Sparse Representation (`CompiledProblem`)
The function [`Model.Compile()`](../compiled.go) produces an immutable [`CompiledProblem`](../compiled.go) containing both row-major (`CSR`) and column-major (`CSC`) representations of the constraint matrix $A \in \mathbb{R}^{m \times n}$:
- **Row-Major (`CSR`)**: `RowPtr` ($m+1$), `ColIdx` ($\text{nnz}$), and `RowVal` ($\text{nnz}$), sorted by ascending column index within each row. Presolve row propagation (`propagateRow`), cutting-plane generation (`generateCutRound`), and primal constraint verification (`MaxViolation`) scan `CSR` slices sequentially.
- **Column-Major (`CSC`)**: `ColPtr` ($n+1$), `RowIdx` ($\text{nnz}$), and `ColVal` ($\text{nnz}$), sorted by ascending row index within each column. Simplex column pricing, basis matrix assembly (`factorizeBasis`), and `FTRAN` spike updates scan `CSC` slices sequentially.
- **Internal Minimization Normalization**: Every problem is normalized internally to minimization via `ObjMin[j] = Sense * ObjCoeffs[j]`. User-facing objectives and bounds convert back through `ObjectiveFromMin` and `EvaluateObjective`.

### Pooled Solver Workspaces (`Workspace`)
To reuse numerical buffers across solves and Branch-and-Bound node re-optimizations, [`AcquireWorkspace`](../workspace.go) and [`ReleaseWorkspace`](../workspace.go) manage a `sync.Pool` of pre-allocated [`Workspace`](../workspace.go) structs. Each `Workspace` owns the basis arrays (`Basis`, `BasicRow`, `Status`, `RowBasicDeg`), working bounds and costs (`LB`, `UB`, `Cost`, `WorkCost`), primal/dual iterate vectors (`X`, `XB`, `Y`, `Dj`, `DSE`), tableau and sparse transformation buffers (`Rho`, `RhoIdx`, `FCol`, `FColIdx`, `Tau`, `TauIdx`, `TabRow`, `TabRowIdx`, `TabRowMark`, `FlipCols`, `BFRTCands`), and the reusable [`LUFactorization`](../lu.go).

---

## 3. Detailed Documentation Guides

1. **[Modeling API and Serialization Formats](modeling-and-formats.md)**: Complete reference for `Model`, `Var`, `Expr`, `Constraint`, `CompiledProblem`, `PuLP` JSON, and MPS I/O.
2. **[Solver Internals and Mathematical Specification](solver-internals.md)**: Mathematical formulations and implementation details for `LU` factorization, Dual and Primal Simplex, SIMD vector kernels, presolve, cuts, heuristics, and Branch-and-Cut.
3. **[Benchmark Methodology and Corpus Results](benchmarks.md)**: Reproducible benchmark protocol, hardware specifications, MAD `G x I` problem formulation, and synchronized telemetry tables.
