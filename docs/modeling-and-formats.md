# Modeling API and Serialization Formats

The `gopt` modeling layer provides a strongly typed Go API for constructing Linear Programs (`LP`) and Mixed-Integer Linear Programs (`MILP`), compiling them into sparse matrices (`CompiledProblem`), and exchanging problem definitions with Python `PuLP` and standard MPS solvers.

---

## 1. Constructing Models (`Model`, `Var`, `Expr`, `Constraint`)

### Variables and Bounds
Create a model with [`NewModel(name, sense)`](../model.go), where `sense` is `Minimize` (`1`) or `Maximize` (`-1`). Variables are lightweight 64-bit handles (`type Var uint64`, encoding the owning `Model` ID in the upper 32 bits and the 0-based column index in the lower 32 bits) registered via four methods on [`Model`](../model.go):
- `AddContVar(name string, lb, ub float64) Var`: Continuous variable $x_j \in [\text{lb}, \text{ub}]$. Use `math.Inf(-1)` and `math.Inf(1)` for unbounded directions.
- `AddIntVar(name string, lb, ub float64) Var`: General integer variable $x_j \in [\lceil \text{lb} \rceil, \lfloor \text{ub} \rfloor] \cap \mathbb{Z}$.
- `AddBinaryVar(name string) Var`: Binary variable $x_j \in \{0, 1\}$.
- `AddVar(name string, lb, ub float64, cat VarCategory) Var`: Generic constructor accepting `Continuous`, `Integer`, or `Binary`.

### Linear Expressions and Constraints
Linear expressions (`Expr`) combine variable-coefficient pairs (`Term`) with an optional scalar constant:
- `T(v Var, coeff float64) Term` constructs a single term $c_j x_j$.
- `NewExpr(terms ...Term) Expr` builds a linear expression $\sum_k c_k x_{v_k}$.
- `Le(expr, rhs)`, `Ge(expr, rhs)`, and `Eq(expr, rhs)` construct `LessEqual` ($\le$), `GreaterEqual` ($\ge$), and `Equal` ($=$) constraints. Any constant on `expr.Constant` shifts automatically to the right-hand side (`RHS - expr.Constant`) during `AddConstraint`.

---

## 2. Model Validation and Sparse Matrix Compilation (`Model.Compile`)

Calling [`Model.Validate()`](../model.go) or [`Model.CompileChecked()`](../compiled.go) checks that all variable bounds, objective terms, and constraint terms reference `Var` handles owned by the receiver `Model`, contain finite (`non-NaN`, `non-Inf`) coefficients, use unique variable names, and use a valid `ConstraintSense` (`LessEqual`, `Equal`, or `GreaterEqual`). If an invalid model is passed directly to `Solve()` or `SolveCompiled()`, the solver returns `StatusUndefined`. Bounds that become empty after integer rounding (for example an integer variable on `[0.5, 0.7]`) are not a validation error; the solver reports `StatusInfeasible`.

Calling [`Model.Compile()`](../compiled.go) freezes the algebraic model into an immutable [`CompiledProblem`](../compiled.go):
1. **Term Deduplication and Cancellation**: Within each constraint row, duplicate references to the same `Var` are summed in preallocated accumulator slices (`rowAcc`, `touched`, `inTouched`), and entries with $|a_{i,j}| \le 10^{-15}$ are dropped.
2. **Sorted `CSR` Construction**: Nonzero column indices within each row $i$ are sorted in ascending order and packed into contiguous `RowPtr`, `ColIdx`, and `RowVal` arrays.
3. **Transposed `CSC` Construction**: A two-pass counting transpose builds `ColPtr`, `RowIdx`, and `ColVal` in $O(\text{nnz} + n + m)$ time.
4. **Integer Column Indexing**: All `Binary` and `Integer` variable indices are collected into `IntCols`, setting `IsMIP = len(IntCols) > 0`.

Once compiled, a single `*CompiledProblem` can be solved repeatedly with different `Options` or across concurrent goroutines via [`SolveCompiled(p, opts...)`](../solve.go).

### Solver Options

Start from [`DefaultOptions()`](../solve.go) and override fields. A zero-value `Options{}` is valid but minimal: presolve, cuts, and heuristics are off, the node and iteration budgets are unlimited, and both gap tolerances are zero.

```go
opts := gopt.DefaultOptions()
opts.TimeLimit = 5 * time.Second
opts.MaxNodes = 10000 // StatusNodeLimit when exceeded
opts.MaxIter = 200000 // total simplex iterations; StatusIterationLimit when exceeded
res := gopt.SolveCompiled(p, opts)
```

Whenever a limit stops a MILP search after the root LP has been solved, `Result.BestBound` is a valid global bound and `Result.Values` holds the best incumbent found, if any.

---

## 3. `PuLP` JSON Interoperability (`gopt/format`)

The [`format/pulp_json.go`](../format/pulp_json.go) file implements bidirectional serialization matching Python `PuLP`'s `LpProblem.toJson()` / `LpProblem.fromJson()` schema:
- **Constraint Sign Convention**: In `PuLP`'s JSON dictionary, each constraint stores `coefficients` and a scalar `constant` representing $\sum_j a_{i,j} x_j + \text{constant} \ \{\le, =, \ge\}\ 0$. The function [`BuildModelFromPuLPJSON`](../format/pulp_json.go) maps `RHS = -c.Constant` and maps `sense` codes (`-1` $\to$ `LessEqual`, `0` $\to$ `Equal`, `1` $\to$ `GreaterEqual`).
- **Category Promotion**: `PuLP` serializes binary variables as `"cat": "Integer"` with `"lowBound": 0, "upBound": 1`. During [`BuildModelFromPuLPJSON`](../format/pulp_json.go), integer variables with bounds $[0, 1]$ are promoted to `gopt.Binary` so that 0-1 knapsack cover lifting and binary bound propagation activate automatically.
- **Separated Parse and Build Stages**: [`ParsePuLPJSON(data)`](../format/pulp_json.go) unmarshals raw JSON bytes into `*PuLPJSON`, and [`BuildModelFromPuLPJSON(doc)`](../format/pulp_json.go) constructs the `*gopt.Model`, allowing the benchmark harness to time `t_parse` and `t_build` independently.

---

## 4. MPS Serialization (`format.WriteMPS`)

The function [`format.WriteMPS(p *CompiledProblem, w io.Writer)`](../format/mps.go) emits standard MPS output from a `CompiledProblem`:
- Writes `*SENSE:<sense>`, `NAME`, `ROWS` (`N` objective, `L`/`G`/`E` constraints), `COLUMNS` (iterating directly over `CSC` slices `ColPtr`/`RowIdx`/`ColVal` and bracketing integer blocks with `'MARKER' 'INTORG'` and `'INTEND'`), `RHS`, `BOUNDS`, and `ENDATA`.
