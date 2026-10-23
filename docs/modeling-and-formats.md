# Modeling API and Serialization Formats

The `goblin` modeling layer provides a strongly typed Go API for constructing Linear Programs (`LP`) and Mixed-Integer Linear Programs (`MILP`), compiling them into sparse matrices (`CompiledProblem`), and exchanging problem definitions with Python `PuLP` and standard MPS solvers.

---

## 1. Constructing Models (`Model`, `Var`, `Expr`, `Constraint`)

### Variables and Bounds
Create a model with [`NewModel(name, sense)`](../model.go), where `sense` is `Minimize` (`1`) or `Maximize` (`-1`). Variables are lightweight 64-bit handles (`type Var uint64`, encoding the owning `Model` ID in the upper 32 bits and the 0-based column index in the lower 32 bits) registered via four methods on [`Model`](../model.go):
- `AddContVar(name string, lb, ub float64) Var`: Continuous variable $x_j \in [\text{lb}, \text{ub}]$. Use `math.Inf(-1)` and `math.Inf(1)` for unbounded sides.
- `AddIntVar(name string, lb, ub float64) Var`: General integer variable $x_j \in [\lceil \text{lb} \rceil, \lfloor \text{ub} \rfloor] \cap \mathbb{Z}$.
- `AddBinaryVar(name string) Var`: Binary variable $x_j \in \{0, 1\}$.
- `AddVar(name string, lb, ub float64, cat VarCategory) Var`: Generic constructor accepting `Continuous`, `Integer`, or `Binary`.

```go
m := gb.NewModel("knapsack", gb.Maximize)
x := m.AddBinaryVar("x")
y := m.AddIntVar("y", 0, 10)
m.SetObjective(gb.NewExpr(x.T(12), y.T(7)))
m.AddConstraint("cap", gb.NewExpr(x.T(4), y.T(3)).Le(25))
```

### Linear Expressions and Constraints
By convention, callers can import `github.com/paulourio/goblin` as `gb` (`import gb "github.com/paulourio/goblin"`) for concise expression construction. Linear expressions (`Expr`) combine variable-coefficient pairs (`Term`) with an optional scalar constant:
- The method `v.T(coeff float64) Term` (or `T(v Var, coeff float64) Term`) constructs a single term $c_j x_j$.
- The function `NewExpr(terms ...Term) Expr` builds a linear expression $\sum_k c_k x_{v_k}$.
- The methods `expr.Le(rhs)`, `expr.Ge(rhs)`, and `expr.Eq(rhs)` (or `Le(expr, rhs)`, `Ge(expr, rhs)`, and `Eq(expr, rhs)`) construct `LessEqual` ($\le$), `GreaterEqual` ($\ge$), and `Equal` ($=$) constraints. Any constant on `expr.Constant` shifts automatically to the right-hand side (`RHS - expr.Constant`) during `AddConstraint`.

---

## 2. Model Validation and Sparse Matrix Compilation (`Model.Compile`)

Calling [`Model.Validate()`](../model.go) or [`Model.CompileChecked()`](../compiled.go) checks that all variable bounds, objective terms, and constraint terms reference `Var` handles owned by the receiver `Model`, contain finite (`non-NaN`, `non-Inf`) coefficients, use unique variable names, and use a valid `ConstraintSense` (`LessEqual`, `Equal`, or `GreaterEqual`). If an invalid model is passed directly to `Solve()` or `SolveCompiled()`, the solver returns `StatusUndefined`. Bounds that become empty after integer rounding (for example an integer variable on `[0.5, 0.7]`) are not a validation error; the solver reports `StatusInfeasible`.

Calling [`Model.Compile()`](../compiled.go) freezes the algebraic model into an immutable [`CompiledProblem`](../compiled.go):
1. **Term Deduplication and Cancellation**: Within each constraint row, duplicate references to the same `Var` are summed in preallocated accumulator slices (`rowAcc`, `touched`, `inTouched`), and entries with $|a_{i,j}| \le 10^{-15}$ are dropped.
2. **Sorted `CSR` Construction**: Nonzero column indices within each row $i$ are sorted in ascending order and packed into contiguous `RowPtr`, `ColIdx`, and `RowVal` arrays.
3. **Transposed `CSC` Construction**: A two-pass counting transpose builds `ColPtr`, `RowIdx`, and `ColVal` in $O(\text{nnz} + n + m)$ time.
4. **Integer Column Indexing**: All `Binary` and `Integer` variable indices are collected into `IntCols`, setting `IsMIP = len(IntCols) > 0`.

Once compiled, a single `*CompiledProblem` can be solved repeatedly with different solver settings or across concurrent goroutines via [`SolveCompiled(p, opts...)`](../solve.go).

### Functional Solver Settings and Backends

Both `Model.Solve(opts...)` and `SolveCompiled(p, opts...)` start from the default pure-Go settings ([`solve.go`](../solve.go): `BackendGo`, presolve, cuts, heuristics, and SIMD enabled) and apply functional [`Option`](../solve.go) values in order:

- **Backend choice**: `WithPureGo()` (`WithBackend(BackendGo)`, default) runs the pure-Go simplex and Branch-and-Cut engine; `WithCBC()` (`WithBackend(BackendCBC)`) dispatches the compiled `CSC` matrix in-process to COIN-OR CBC/CLP via `cgo`.
- **SIMD control**: `WithSIMD(bool)` and `WithoutSIMD()` enable or disable AVX2 SIMD kernels on the pure-Go backend.
- **Search and tolerance limits**: `WithTimeLimit(d)`, `WithMaxNodes(n)`, `WithMaxIter(n)`, `WithPresolve(bool)`, `WithCuts(bool)`, `WithHeuristics(bool)`, `WithMaxCutPasses(n)`, `WithRelGapTol(tol)`, and `WithAbsGapTol(tol)`.

```go
res := gb.SolveCompiled(p,
	gb.WithPureGo(),
	gb.WithTimeLimit(5*time.Second),
	gb.WithMaxNodes(10000),   // StatusNodeLimit when exceeded
	gb.WithMaxIter(200000),   // total simplex iterations; StatusIterationLimit when exceeded
)
```

Whenever a limit stops a MILP search after the root LP has been solved, `Result.BestBound` is a valid global bound and `Result.Values` holds the best incumbent found, if any.

---

## 3. `PuLP` JSON Interoperability (`goblin/format`)

The [`format/pulp_json.go`](../format/pulp_json.go) file reads and writes Python `PuLP`'s `LpProblem.toJson()` / `LpProblem.fromJson()` schema:
- **Constraint Sign Rule**: In `PuLP`'s JSON dictionary, each constraint stores `coefficients` and a scalar `constant` representing $\sum_j a_{i,j} x_j + \text{constant} \ \{\le, =, \ge\}\ 0$. The function [`BuildModelFromPuLPJSON`](../format/pulp_json.go) maps `RHS = -c.Constant` and maps `sense` codes (`-1` $\to$ `LessEqual`, `0` $\to$ `Equal`, `1` $\to$ `GreaterEqual`).
- **Binary Bounds**: `PuLP` serializes binary variables as `"cat": "Integer"` with `"lowBound": 0, "upBound": 1`. During [`BuildModelFromPuLPJSON`](../format/pulp_json.go), integer variables with bounds $[0, 1]$ become `goblin.Binary` so that 0-1 knapsack cover lifting and binary bound tightening activate automatically.
- **Separated Parse and Build Stages**: [`ParsePuLPJSON(data)`](../format/pulp_json.go) unmarshals raw JSON bytes into `*PuLPJSON`, and [`BuildModelFromPuLPJSON(doc)`](../format/pulp_json.go) constructs the `*goblin.Model`, allowing the benchmark harness to time `t_parse` and `t_build` independently.

---

## 4. MPS Serialization (`format.WriteMPS`)

The function [`format.WriteMPS(p *CompiledProblem, w io.Writer)`](../format/mps.go) emits standard MPS output from a `CompiledProblem`:
- Writes `*SENSE:<sense>`, `NAME`, `ROWS` (`N` objective, `L`/`G`/`E` constraints), `COLUMNS` (iterating directly over `CSC` slices `ColPtr`/`RowIdx`/`ColVal` and bracketing integer blocks with `'MARKER' 'INTORG'` and `'INTEND'`), `RHS`, `BOUNDS`, and `ENDATA`.
