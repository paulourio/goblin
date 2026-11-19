# `goblin` Architecture and Design Specification

The `goblin` solver separates algebraic model building, sparse matrix assembly, numerical search, and file-format I/O into five isolated layers. Each layer communicates through encapsulated data containers (`CompiledProblem`, `Result`, `LPResult`) so that concurrent workers can solve independent problem instances without shared mutable state, using either the default pure-Go engine (`BackendGo`) or the optional in-process C++ COIN-OR CBC/CLP backend (`BackendCBC`).

---

## 1. Architectural Layer Separation

```mermaid
flowchart LR
    A["format.FromPuLPJSON / Model API"] --> B["Model.Compile()"]
    B --> C["CompiledProblem (CSR + CSC)"]
    C --> D["Presolve (GCD + Savelsbergh + Orbit Symmetry)"]
    D --> E["LPSolver (Dual / Primal Simplex + LU)"]
    E --> F["solveBranchAndCutWithDone (Cuts + Heuristics + B&B)"]
    F --> G["Result (Primal + Dual + Telemetry)"]
```

| Layer | Source Files | Core Responsibility |
| :--- | :--- | :--- |
| **1. Modeling & Compilation** | [`model.go`](../model.go), [`expr.go`](../expr.go), [`compiled.go`](../compiled.go) | Collects variables, linear expressions, and constraints, validates variable ownership and finite coefficients, deduplicates terms, and compiles encapsulated `CSR` and `CSC` sparse matrices (`CompiledProblem`). |
| **2. Linear Algebra & LP Engine** | [`lu.go`](../lu.go), [`simplex.go`](../simplex.go), [`workspace.go`](../workspace.go), [`internal/simd/`](../internal/simd/) | Executes singleton-preordered sparse Markowitz `LU` factorization with `BTRANExact` Farkas verification and PFI eta updates, Revised Dual Simplex with `FTRANSparseHint`/`BTRANSparseHint` and bound flipping, and Two-Phase Primal Simplex over pooled `Workspace` buffers. |
| **3. MILP Branch-and-Cut & CBC Bridge** | [`presolve.go`](../presolve.go), [`cuts.go`](../cuts.go), [`heuristics.go`](../heuristics.go), [`bab.go`](../bab.go), [`solve.go`](../solve.go), [`cbc_cgo.go`](../cbc_cgo.go) | Performs MIP presolve, multi-family cutting-plane generation, primal heuristics, and pseudocost/strong-branching search in pure Go (zero `cgo` by default), or dispatches the compiled `CSC` matrix to COIN-OR CBC/CLP when built with `-tags cbc` and `WithCBC()` is selected. |
| **4. Combinatorial Assignment (`goblin/assign`)** | [`assign/assign.go`](../assign/assign.go), [`assign/solver.go`](../assign/solver.go), [`assign/workspace.go`](../assign/workspace.go), [`assign_alias.go`](../assign_alias.go) | Solves square and rectangular Linear Sum Assignment Problems (`LSAP`) in $O(\min(m, n)^2 \max(m, n))$ time via Jonker-Volgenant and Bijsterbosch-Volgenant dual reduction and 3-way partitioned Dijkstra shortest augmenting paths over pooled workspaces. |
| **5. Serialization & Benchmarking** | [`format/pulp_json.go`](../format/pulp_json.go), [`format/mps.go`](../format/mps.go), [`bench/corpus.go`](../bench/corpus.go), [`bench/assign_corpus.go`](../bench/assign_corpus.go), [`bench/docgen.go`](../bench/docgen.go), [`cmd/goblin-optcheck/main.go`](../cmd/goblin-optcheck/main.go) | Parses and emits `PuLP` JSON and MPS formats, builds the 31-family synthetic MILP corpus (293 distinct trials) and 22-family synthetic LSAP corpus (200 trials), synchronizes markdown documentation from JSON telemetry, and audits hot-path compiler and disassembly diagnostics. |

---

## 2. Core Data Invariants

### Dual-Index Sparse Representation (`CompiledProblem`)
The function [`Model.Compile()`](../compiled.go) produces an immutable [`CompiledProblem`](../compiled.go) whose internal `CSR` and `CSC` slices are unexported so external callers cannot violate matrix or C++ ABI invariants, while read-only accessors (`NumVars`, `NumConstraints`, `NNZ`, `VarBounds`, `VarCategory`, `RowRHS`, `ForEachRowTerm`, `ForEachColTerm`) expose structural inspection:
- **Row-Major (`CSR`)**: `rowPtr` ($m+1$), `colIdx` ($\text{nnz}$), and `rowVal` ($\text{nnz}$), sorted by ascending column index within each row. Node bound propagation (`propagateBounds` and `propagateRow`), cutting-plane rounds (`generateCutRound`), and primal residual checks (`MaxViolation`) scan `CSR` slices sequentially.
- **Column-Major (`CSC`)**: `colPtr` ($n+1$), `rowIdx` ($\text{nnz}$), and `colVal` ($\text{nnz}$), sorted by ascending row index within each column, paired with 32-bit `colPtr32` and `rowIdx32` views validated before zero-copy C++ `loadProblem` calls. Simplex column pricing, basis matrix assembly (`factorizeBasis`), and `FTRAN` spike updates scan `CSC` slices sequentially.
- **Internal Objective Sense**: Every problem is normalized internally to minimize `objMin[j] = sense * objCoeffs[j]`. User-facing objectives and bounds convert back through `ObjectiveFromMin` and `EvaluateObjective`.

### Pooled Solver Workspaces (`Workspace`)
To reuse numerical buffers across solves and Branch-and-Bound node re-solves, [`AcquireWorkspace`](../workspace.go) and [`ReleaseWorkspace`](../workspace.go) manage a `sync.Pool` of pre-allocated [`Workspace`](../workspace.go) structs guarded against double release by an atomic `inPool` flag. Each `Workspace` encapsulates basis arrays (`basis`, `basicRow`, `status`, `rowBasicDeg`), working bounds and costs (`lb`, `ub`, `cost`, `workCost`), primal/dual iterate vectors (`x`, `xb`, `y`, `dj`, `dse`), tableau and sparse transformation buffers (`rho`, `rhoIdx`, `fCol`, `fColIdx`, `tau`, `tauIdx`, `tabRow`, `tabRowIdx`, `tabRowMark`, `tabRowChunks`, `tabRowChunkMark`, `flipCols`, `bfrtCands`), and the reusable [`LUFactorization`](../lu.go).

---

## 3. Detailed Documentation Guides

1. **[Modeling API and Serialization Formats](modeling-and-formats.md)**: Complete reference for `Model`, `Var`, `Expr`, `Constraint`, `CompiledProblem`, `PuLP` JSON, and MPS I/O.
2. **[Solver Internals and Mathematical Specification](solver-internals.md)**: Equations and algorithmic details for `LU` factorization, Dual and Primal Simplex, SIMD vector kernels, presolve, cuts, heuristics, and Branch-and-Cut.
3. **[Benchmark Methodology and Corpus Results](benchmarks.md)**: Reproducible benchmark protocol, hardware specs, MAD `G x I` model structure, and synchronized telemetry tables.
