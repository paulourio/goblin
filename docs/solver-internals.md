# Solver Internals and Mathematical Specification

The `gopt` engine combines a revised dual and two-phase primal simplex LP solver ([`lu.go`](../lu.go), [`simplex.go`](../simplex.go)) with a branch-and-cut MILP framework ([`presolve.go`](../presolve.go), [`cuts.go`](../cuts.go), [`heuristics.go`](../heuristics.go), [`bab.go`](../bab.go)). This document specifies the formulations and the invariants each subsystem maintains.

---

## 1. Three-Stage Basis Factorization and PFI Eta Updates (`lu.go`)

The LP solver maintains a nonsingular basis $B \in \mathbb{R}^{m \times m}$ whose columns are drawn from the $n$ structural columns and the $m$ unit slack columns.

### Singleton Peeling and Sparse Markowitz LU
[`LUFactorization.FactorizeSparse`](../lu.go) permutes $B$ into the block form

$$P B Q = \begin{pmatrix} U_{11} & B_{12} & B_{13} \\ 0 & L_{22} & 0 \\ 0 & B_{32} & N_{33} \end{pmatrix}$$

1. **Stage 1, column singletons** (positions $0 \dots s_1 - 1$): repeatedly pivots on a column with one nonzero among the remaining rows (`|pivVal| >= 1e-7`). Those columns have no entries in later rows, so $U_{11}$ is upper triangular and $L_{11} = I$.
2. **Stage 2, row singletons** (positions $s_1 \dots s_2 - 1$): repeatedly pivots on a row with one remaining column whose entry satisfies both the floor $|p| \ge 10^{-7}$ and the column threshold $|p| \ge 0.1 \max_i |B_{i,q}|$. Because the block to the right of $L_{22}$ is zero, factoring $\begin{pmatrix} L_{22} \\ B_{32} \end{pmatrix} = \begin{pmatrix} \hat{L}_{22} \\ \hat{L}_{32} \end{pmatrix} D_2$ places the unit multipliers in $\hat{L}$ and the pivot values $D_2$ on the diagonal of $U_{22}$.
3. **Stage 3, sparse Markowitz nucleus** ($n_{\text{nuc}} = m - s_2$): [`factorizeNucleusSparse`](../lu.go) maintains active sparse rows (`nucRowIdx`/`nucRowVal`) and columns (`nucColIdx`/`nucColVal`) with dynamic degree arrays. At each step it checks for a threshold-stable row singleton, a column singleton, or the entry minimizing the Markowitz product $(r_i - 1)(c_j - 1)$ among candidates satisfying $|u_{ij}| \ge 0.1 \max_r |u_{rj}|$ and $|u_{ij}| \ge 10^{-11}$, then updates the active submatrix by sparse outer-product elimination.

After factorization, all three stages are assembled into unified pre-permuted CSR and CSC factors (`lRowPtr`/`lColPtr` for unit lower-triangular $L$, `uRowPtr`/`uColPtr` and `uInvDiag` for upper-triangular $U$) indexed directly in elimination order $0 \dots m-1$.

### `FTRAN`, `BTRAN`, and Hyper-Sparse Solves
- **`FTRAN` ($B x = b$) and `FTRANSparse`**: permutes $b$ into elimination order, solves $L y = P b$ by column-wise zero-skipping elimination (`lColPtr`), solves $U z = y$ by backward column sweep (`uColPtr` and `uInvDiag`), unpermutes $x = Q z$, and applies the PFI eta file in order. When the initial right-hand side has at most $m / 4$ nonzeros, `FTRANSparse` tracks the nonzero support and returns `FColIdx` so callers update and clear only touched rows.
- **`BTRAN` ($B^T \pi = c$) and `BTRANSparse`**: applies the eta file in reverse, permutes into column-elimination order, solves $U^T z = Q^T c$ by forward column sweep (`uColPtr`), solves $L^T w = z$ by backward row sweep (`lRowPtr`), and unpermutes $\pi = P^T w$. `BTRANSparse` tracks the nonzero support and returns `RhoIdx`.
- **Updates**: [`PushEtaSparse`](../lu.go) appends a product-form (PFI) eta column directly from `FColIdx`. After `maxEtaUpdates = 56` updates, or when a pivot fails, the basis is refactorized.

---

## 2. Revised Dual and Two-Phase Primal Simplex (`simplex.go`)

Each row $i$ becomes $a_i^T x + s_i = b_i$ with slack bounds $s_i \in [0, \infty)$ for `LessEqual`, $(-\infty, 0]$ for `GreaterEqual`, and $[0, 0]$ for `Equal`. Cutting planes are stored as extra `LessEqual` rows.

### Feasibility Tolerances
A basic variable $x_k$ is primal infeasible when it leaves $[l_k, u_k]$ by more than its tolerance. Slack columns use $\epsilon_p = 10^{-7}$. A structural column uses $\epsilon_p / \max(1, \max_i |a_{ik}|)$, floored at $10^{-12}$, because a value error $e$ in $x_k$ shifts some row by up to $e \max_i |a_{ik}|$. With an absolute tolerance, a big-M indicator resting $5 \cdot 10^{-8}$ outside its bound would shift its row by $0.05$ once reported.

### Dual-First Dispatch
Nonbasic structurals start at the bound favoured by their cost, with the slack basis $B = I$. When the reduced costs are dual feasible within $\epsilon_d = 10^{-7}$, [`LPSolver.Solve`](../simplex.go) runs [`runDualSimplexLoop`](../simplex.go); otherwise, or if the dual loop ends without a verified optimum, it runs [`solveTwoPhasePrimalWithLimit`](../simplex.go).

### Dual Simplex
1. **Leaving row**: the infeasible basic row maximizing $\text{infeas}_r^2 / w_r$, with dual steepest-edge weights $w_r$ updated over `FColIdx` by the Forrest-Goldfarb recurrence (`FTRAN(Tau)` is skipped when `len(FColIdx) <= 1`) and clamped to $[10^{-4}, 10^{12}]$. Appending cut rows keeps existing weights, since the existing rows of $B^{-1}$ do not change when a basic slack row is added.
2. **Pivot row** (`ComputeTableauRow`): computes $\rho = B^{-T} e_r$ via `BTRANSparse(Rho, RhoIdx)`, then accumulates $\bar a_{r} = \rho^T [A \; I]$ over `RhoIdx` and records the nonbasic support in `TabRowIdx`.
3. **Ratio test and Bound-Flipping Ratio Test (`selectDualBFRT`)**: Pass 1 computes $\theta_{\max} = \min (|d_k| + 10^{-9}) / |\bar a_{r,k}|$; Pass 2 picks, among candidates within $\theta_{\max}$, the largest $|\bar a_{r,k}|$, preferring columns whose bound span absorbs the leaving row's violation and breaking near-ties (`1e-11`) by minimum basis row load (`colBasisLoad` via `RowBasicDeg` on non-degenerate steps, or static column degree and index on degenerate steps). When the Pass-2 winner is boxed, cannot absorb the violation alone, and has a positive dual step (`cands[0].ratio > 1e-7`), `selectDualBFRT` sorts breakpoints and flips boxed nonbasic variables not in equality rows (`!colHasEq[k]`, up to 64 per pivot) to their opposite bounds, updating $x_B$ via one batched `FTRANSparse(Tau, TauIdx)`.
4. **Infeasibility**: if no column is eligible after a fresh refactorization, the LP is reported infeasible.

### Two-Phase Primal Simplex
Phase I minimizes the sum of bound violations of basic variables with a composite cost; Phase II uses Dantzig pricing. In Phase I, an improving column that no basic variable blocks cannot occur in exact arithmetic, so it is reported as `StatusNumericalError`, never as an infeasibility proof. Both loops honour `LPSolver.MaxIter` (shared by both phases) and `LPSolver.Deadline`.

---

## 3. SIMD Kernels (`internal/simd`)

With `GOEXPERIMENT=simd` on `amd64`, [`kernels_amd64.go`](../internal/simd/kernels_amd64.go) implements `Dot64` and `Axpy64` with `archsimd.Float64x4` for slices of length 256 or more; shorter slices run the same 4-way scalar loops as [`kernels_scalar.go`](../internal/simd/kernels_scalar.go).

- **Bit-identical results**: the vector code uses separate `VMULPD` and `VADDPD`, never FMA. The scalar code converts each product to `float64` before adding it, which by the Go specification forbids FMA contraction. Both reduce four lanes over $i \bmod 4$ as $(s_0 + s_1) + (s_2 + s_3)$. [`kernels_test.go`](../internal/simd/kernels_test.go) checks bit equality on both sides of the 256 threshold, and it passes with `GOAMD64=v3`, where FMA is available.
- **Reach**: dense tableau rows exist only for $n \le 256$, and sparse dual iterations update `Y` and `Dj` over `RhoIdx` and `TabRowIdx` when the support is sparse, so AVX2 calls occur only on dense dual updates of length 256 or more. Below `n + m = 256` both builds run the same scalar kernels, so timing differences there come from the two binaries and from run-to-run variation. The paired timing comparison is in [`benchmarks.md`](benchmarks.md#5-default-build-vs-goexperimentsimd).

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

- **LP updates and primal fallback**: while basis factorization uses a three-stage singleton and sparse Markowitz LU with `FTRANSparse`/`BTRANSparse` and dual bound flipping (`selectDualBFRT`), basis updates between refactorizations use a product-form (PFI) eta file rather than Forrest-Tomlin $U$-updates, and the Phase II primal fallback uses full Dantzig pricing.
- **Scaling**: there is no row or column equilibration; only the primal feasibility tolerance is normalized by column magnitude.
- **Cuts at the root only**: cuts are separated at the root node, using global bounds, and kept for the whole tree.
- **Reference solver**: parity is checked against CBC 2.10.3, which itself returns wrong answers on some instances (see [`testdata/regression`](../testdata/regression)); `bench/difftest_cbc.py` verifies disagreements independently before blaming either solver.
