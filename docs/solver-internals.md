# Solver Internals and Mathematical Specification

The `gopt` engine combines a revised dual and two-phase primal simplex LP solver ([`lu.go`](../lu.go), [`simplex.go`](../simplex.go)) with a branch-and-cut MILP framework ([`presolve.go`](../presolve.go), [`cuts.go`](../cuts.go), [`heuristics.go`](../heuristics.go), [`bab.go`](../bab.go)). This document specifies the formulations and the invariants each subsystem maintains.

---

## 1. Three-Stage Basis Factorization and PFI Eta Updates (`lu.go`)

The LP solver maintains a nonsingular basis $B \in \mathbb{R}^{m \times m}$ whose columns are drawn from the $n$ structural columns and the $m$ unit slack columns.

### Singleton Peeling and Dense-Nucleus LU
[`LUFactorization.FactorizeSparse`](../lu.go) permutes $B$ into the block form

$$P B Q = \begin{pmatrix} U_{11} & B_{12} & B_{13} \\ 0 & L_{22} & 0 \\ 0 & B_{32} & N_{33} \end{pmatrix}$$

1. **Stage 1, column singletons** (positions $0 \dots s_1 - 1$): repeatedly pivots on a column with exactly one nonzero among the remaining rows. Those columns have no entries in later rows, so $U_{11}$ is upper triangular.
2. **Stage 2, row singletons** (positions $s_1 \dots s_2 - 1$): repeatedly pivots on a row with exactly one remaining column. Those rows have no nucleus entries, so $L_{22}$ is lower triangular and the block to its right is zero.
3. **Stage 3, dense nucleus** ($n_{\text{nuc}} = m - s_2$): the remaining submatrix is scattered into a row-major buffer with stride `(nNuc + 7) &^ 7` and factorized by Gaussian elimination with partial pivoting, $O(n_{\text{nuc}}^3)$. Each row records the column envelope of its $L$ and $U$ entries (`lFirst`/`lLast`, `uFirst`/`uLast`) so that updates and solves touch only that span.

Singleton pivots smaller than $10^{-7}$ are deferred to the nucleus; a nucleus pivot below $10^{-11}$ reports a singular basis, after which the solver restarts from the slack basis.

### `FTRAN` and `BTRAN`
- **`FTRAN` ($B x = b$)**: forward substitution through $L_{22}$ (eliminating into $B_{32}$), the nucleus solves $L y = \tilde b$ and $U x = y$ with `simd.Dot64` over row envelopes, backward substitution through $U_{11}$, and finally the eta file in order.
- **`BTRAN` ($B^T \pi = c$)**: the eta file in reverse, forward substitution through $U_{11}^T$, the nucleus solves $U^T z = \tilde c$ and $L^T w = z$ with `simd.Axpy64`, and backward substitution through $L_{22}^T$.
- **Updates**: each basis change appends a product-form (PFI) eta column. After `maxEtaUpdates = 56` updates, or when a pivot fails, the basis is refactorized.

---

## 2. Revised Dual and Two-Phase Primal Simplex (`simplex.go`)

Each row $i$ becomes $a_i^T x + s_i = b_i$ with slack bounds $s_i \in [0, \infty)$ for `LessEqual`, $(-\infty, 0]$ for `GreaterEqual`, and $[0, 0]$ for `Equal`. Cutting planes are stored as extra `LessEqual` rows.

### Feasibility Tolerances
A basic variable $x_k$ is primal infeasible when it leaves $[l_k, u_k]$ by more than its tolerance. Slack columns use $\epsilon_p = 10^{-7}$. A structural column uses $\epsilon_p / \max(1, \max_i |a_{ik}|)$, floored at $10^{-12}$, because a value error $e$ in $x_k$ shifts some row by up to $e \max_i |a_{ik}|$. With an absolute tolerance, a big-M indicator resting $5 \cdot 10^{-8}$ outside its bound would shift its row by $0.05$ once reported.

### Dual-First Dispatch
Nonbasic structurals start at the bound favoured by their cost, with the slack basis $B = I$. When the reduced costs are dual feasible within $\epsilon_d = 10^{-7}$, [`LPSolver.Solve`](../simplex.go) runs [`runDualSimplexLoop`](../simplex.go); otherwise, or if the dual loop ends without a verified optimum, it runs [`solveTwoPhasePrimalWithLimit`](../simplex.go).

### Dual Simplex
1. **Leaving row**: the infeasible basic row maximizing $\text{infeas}_r^2 / w_r$, with dual steepest-edge weights $w_r$ updated by the Forrest-Goldfarb recurrence and clamped to $[10^{-4}, 10^{12}]$. Appending cut rows keeps existing weights, since the existing rows of $B^{-1}$ do not change when a basic slack row is added.
2. **Pivot row** (`ComputeTableauRow`): $\rho = B^{-T} e_r$, then $\bar a_{r} = \rho^T [A \; I]$. For $n \le 256$ the structural part uses the precompiled dense rows; otherwise it accumulates sparse CSR rows for which $|\rho_i| > 10^{-16}$ and records the touched columns in `TabRowIdx`.
3. **Ratio test**: a Harris two-pass test. Pass 1 computes $\theta_{\max} = \min (|d_k| + 10^{-9}) / |\bar a_{r,k}|$; pass 2 picks, among candidates within $\theta_{\max}$, the largest $|\bar a_{r,k}|$, preferring columns whose bound span can absorb the leaving row's violation.
4. **Infeasibility**: if no column is eligible after a fresh refactorization, the LP is reported infeasible.

### Two-Phase Primal Simplex
Phase I minimizes the sum of bound violations of basic variables with a composite cost; Phase II uses Dantzig pricing. In Phase I, an improving column that no basic variable blocks cannot occur in exact arithmetic, so it is reported as `StatusNumericalError`, never as an infeasibility proof. Both loops honour `LPSolver.MaxIter` (shared by both phases) and `LPSolver.Deadline`.

---

## 3. SIMD Kernels (`internal/simd`)

With `GOEXPERIMENT=simd` on `amd64`, [`kernels_amd64.go`](../internal/simd/kernels_amd64.go) implements `Dot64` and `Axpy64` with `archsimd.Float64x4` for slices of length 256 or more; shorter slices run the same 4-way scalar loops as [`kernels_scalar.go`](../internal/simd/kernels_scalar.go).

- **Bit-identical results**: the vector code uses separate `VMULPD` and `VADDPD`, never FMA. The scalar code converts each product to `float64` before adding it, which by the Go specification forbids FMA contraction. Both reduce four lanes over $i \bmod 4$ as $(s_0 + s_1) + (s_2 + s_3)$. [`kernels_test.go`](../internal/simd/kernels_test.go) checks bit equality on both sides of the 256 threshold, and it passes with `GOAMD64=v3`, where FMA is available.
- **Reach**: dense tableau rows exist only for $n \le 256$, so most AVX2 calls come from length-$m$ dual updates and nucleus rows of 256 or more. Below `n + m = 256` both builds run the same scalar kernels, so timing differences there come from the two binaries and from run-to-run variation. The paired timing comparison is in [`benchmarks.md`](benchmarks.md#5-default-build-vs-goexperimentsimd).

---

## 4. MIP Presolve (`presolve.go`)

[`presolveWithDeadline`](../presolve.go) runs on MILPs before the root LP and checks the deadline between passes:
1. **GCD reduction**: on rows whose variables and coefficients are all integral, divides by $g = \gcd_j |a_{ij}| > 1$ and rounds the right-hand side ($\lfloor b_i/g \rfloor$ for $\le$, $\lceil b_i/g \rceil$ for $\ge$; an equality with $b_i/g \notin \mathbb{Z}$ proves infeasibility). With $g = 1$ it still rounds integral rows' right-hand sides.
2. **Activity bound tightening** (up to `maxPasses` rounds): tightens $l_j, u_j$ from the residual minimum or maximum activity of each row, rounding integer bounds. Activities are computed once per row and pass; stale values only loosen the derived bounds, so every tightening is valid.
3. **Redundant row removal**: drops `LessEqual` rows with $\overline{\alpha}_i \le b_i$ and `GreaterEqual` rows with $\underline{\alpha}_i \ge b_i$ (within $10^{-9}$).
4. **Coefficient strengthening**: in a $\le$ row with maximum activity $\overline{\alpha}_i > b_i$, a binary with $a_{ij} > 0$ and $\overline{\alpha}_i - a_{ij} < b_i$ gets $a_{ij}$ and $b_i$ both reduced by $\delta = b_i - (\overline{\alpha}_i - a_{ij})$; a binary with $a_{ij} < 0$ and $\overline{\alpha}_i + a_{ij} < b_i$ gets $a_{ij}$ increased by $b_i - (\overline{\alpha}_i + a_{ij})$.

Node-level propagation (`propagateBounds`) applies the same activity rules to each node's bounds when presolve is enabled.

---

## 5. Cutting Planes (`cuts.go`)

At the root, [`generateCutRound`](../cuts.go) separates four families, keeps candidates with efficacy at least $10^{-3}$ and coefficient dynamism at most $10^5$, rejects candidates with cosine similarity above $0.92$ to an already selected cut, and adds up to $\min(60, \max(15, m/3))$ cuts per round as one batch. Every cut is normalized and its right-hand side relaxed by $10^{-7}$. Coefficients of magnitude at most $10^{-11}$ are dropped only after adding the largest possible value of the dropped term over the variable's global bounds to the right-hand side; a cut whose dropped term is unbounded is rejected. If the LP fails to re-solve to optimality after a round (infeasible, iteration or time limit, numerical failure), the round is rolled back to the last optimal relaxation and cutting stops.

1. **Gomory mixed-integer cuts** from up to 25 tableau rows whose basic integer variable has fractionality $f_0 \in [0.01, 0.99]$. With nonbasic variables shifted to their active bounds ($\tilde x_j \ge 0$):
   $$\sum_{j \in \mathcal{I}: f_j \le f_0} \frac{f_j}{f_0} \tilde{x}_j + \sum_{j \in \mathcal{I}: f_j > f_0} \frac{1 - f_j}{1 - f_0} \tilde{x}_j + \sum_{j \in \mathcal{C}: \bar{a}_j > 0} \frac{\bar{a}_j}{f_0} \tilde{x}_j - \sum_{j \in \mathcal{C}: \bar{a}_j < 0} \frac{\bar{a}_j}{1 - f_0} \tilde{x}_j \ge 1$$
   Integer columns with fractional active bounds, and all slacks, are treated as continuous.
2. **Lifted knapsack covers**: on each inequality row relaxed to a binary knapsack (non-binary terms moved to their worst bound, negative binaries complemented), finds a minimal cover $C$ and lifts the remaining items sequentially with a dynamic program over the minimum weight needed to reach each cut activity, giving $\sum_{j \in C} x_j + \sum_{j \notin C} \alpha_j x_j \le |C| - 1$.
3. **VUB disaggregation and $(l, S)$ inequalities**: from rows $\sum_k a_k x_k - C y \le 0$ with $x_k \ge 0$ and binary $y$, derives $x_k \le \min(u_k, C / a_k)\, y$; from chains of flow-balance rows $s_{t-1} + x_t - s_t = d_t$ with VUB-linked production, separates $s_{t-1} + \sum_{\tau \in [t,q] \setminus S} x_\tau + \sum_{\tau \in S} d_{\tau q} y_\tau \ge d_{tq}$.
4. **Single-row mixed-integer rounding**: on inequality rows with finite bounds, shifts each variable to its nearer bound, divides by $\delta = |a_{ik}|$ for up to four fractional integer variables, and applies the MIR function. There is no row aggregation.

---

## 6. Primal Heuristics and Branch-and-Cut (`heuristics.go`, `bab.go`)

### Primal Heuristics
- **Propagated rounding** (`tryPropagatedRounding`): fixes integral columns, then rounds the fractional ones in order of increasing fractionality with queue-based propagation (`propagateFromCol`), trying nearest and objective-directed rounding. On mixed models with more than 32 fractional columns it is skipped.
- **Fixed-integer LP** (`solveFixedIntContinuousLP`): with all integers fixed, re-solves the continuous part, first warm-started on the full model and otherwise as a compact LP over the free columns.
- **Root sub-MIP** (`tryRootSubMIP`): with 1 to 120 fractional integers at the root, frees the integer columns of short equality rows that contain a fractional variable or two basic variables, fixes the rest, and solves the reduced model (at most 800 nodes, at most 1.5 s).
- **Fractional diving** (`tryFractionalDiving`): up to 18 steps of fixing a fractional variable from pure-integer equality rows, propagating, and re-solving warm-started.

### Branching and Search
- **Variable selection** (`selectBranchVar`): pseudocost product scores $\max(f \bar\psi^-, 10^{-4}) \cdot \max((1-f) \bar\psi^+, 10^{-4})$. At depth 2 or less it strong-branches on up to 5 unreliable candidates with 25 dual iterations per child. Only optimal child solves update pseudocosts; an infeasible child scores high; an iteration-limited child falls back to its pseudocost estimate.
- **Pseudocosts**: each node records its branching direction and step length. Gains are stored per unit of step length; steps shorter than $10^{-3}$ are ignored.
- **Node selection**: best bound with plunging into the preferred child, switching to the queue every 8 nodes.

### Incumbents and Near-Integral Nodes
An LP point is integral when every integer variable is within $10^{-6}$ of an integer. It becomes an incumbent only if, after snapping those values, the point violates no bound, row, or integrality by more than $10^{-5}$. Otherwise the node branches on the non-fixed integer variable furthest from integrality, even below $10^{-6}$, so its subtree is still explored. A node that cannot be branched keeps its LP bound in the global bound and makes the final status `NumericalError` rather than `Optimal`.

### Limits and Bounds
- `TimeLimit`, `MaxNodes`, and `MaxIter` (total simplex iterations in `Result.Iterations`) stop the search. A node that could not be evaluated because of a limit goes back to the queue, so its bound stays in the global bound.
- A node LP that fails twice (warm start, then slack basis) keeps its parent bound in the global bound.
- `BestBound` is the minimum over open nodes, unresolved nodes, and the incumbent; at an early root exit it is the root LP or root cut bound. `StatusOptimal` requires the gap between `BestBound` and the incumbent to close within `RelGapTol`/`AbsGapTol`.

---

## Known Limitations

- **LP scale**: the dense nucleus factorization costs $O(n_{\text{nuc}}^2)$ memory and $O(n_{\text{nuc}}^3)$ time, the eta file grows by a dense column per pivot, Phase II uses full Dantzig pricing, and the dual ratio test has no bound flipping. LPs with thousands of rows and non-triangular bases are therefore markedly slower than in CLP or HiGHS.
- **Scaling**: there is no row or column equilibration; only the primal feasibility tolerance is normalized by column magnitude.
- **Cuts at the root only**: cuts are separated at the root node, using global bounds, and kept for the whole tree.
- **Reference solver**: parity is checked against CBC 2.10.3, which itself returns wrong answers on some instances (see [`testdata/regression`](../testdata/regression)); `bench/difftest_cbc.py` verifies disagreements independently before blaming either solver.
