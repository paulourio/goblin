# Solver Internals and Mathematical Specification

The `goblin` engine combines a revised dual and two-phase primal simplex LP solver ([`lu.go`](../lu.go), [`simplex.go`](../simplex.go)) with a branch-and-cut MILP framework ([`presolve.go`](../presolve.go), [`cuts.go`](../cuts.go), [`heuristics.go`](../heuristics.go), [`bab.go`](../bab.go)). This document states the equations and the invariants each subsystem maintains.

---

## 1. Three-Stage Basis Factorization and PFI Eta Updates (`lu.go`)

The LP solver maintains a nonsingular basis $B \in \mathbb{R}^{m \times m}$ whose columns are drawn from the $n$ structural columns and the $m$ unit slack columns.

### Singleton Peeling and Sparse Markowitz LU
The sparse factorizer [`FactorizeSparse`](../lu.go) permutes $B$ into the block form

$$P B Q = \begin{pmatrix} U_{11} & B_{12} & B_{13} \\ 0 & L_{22} & 0 \\ 0 & B_{32} & N_{33} \end{pmatrix}$$

1. **Stage 1, column singletons** (positions $0 \dots s_1 - 1$): repeatedly pivots on a column with one nonzero among the remaining rows (`|pivVal| >= 1e-7`). Those columns have no entries in later rows, so $U_{11}$ is upper triangular and $L_{11} = I$.
2. **Stage 2, row singletons** (positions $s_1 \dots s_2 - 1$): repeatedly pivots on a row with one remaining column whose entry satisfies both the floor $|p| \ge 10^{-7}$ and the column threshold $|p| \ge 0.1 \max_i |B_{i,q}|$. Because the block to the right of $L_{22}$ is zero, factoring $\begin{pmatrix} L_{22} \\ B_{32} \end{pmatrix} = \begin{pmatrix} \hat{L}_{22} \\ \hat{L}_{32} \end{pmatrix} D_2$ places the unit multipliers in $\hat{L}$ and the pivot values $D_2$ on the diagonal of $U_{22}$.
3. **Stage 3, sparse Markowitz nucleus** ($n_{\text{nuc}} = m - s_2$): [`factorizeNucleusSparse`](../lu.go) maintains active sparse rows (`nucRowIdx`/`nucRowVal`) and columns (`nucColIdx`/`nucColVal`) with dynamic degree arrays. At each step it checks for a threshold-stable row singleton, a column singleton, or the entry minimizing the Markowitz product $(r_i - 1)(c_j - 1)$ among candidates satisfying $|u_{ij}| \ge 0.1 \max_r |u_{rj}|$ and $|u_{ij}| \ge 10^{-11}$, then updates the active submatrix by sparse outer-product Gaussian steps.

Once factored, all three stages are assembled into unified pre-permuted CSR and CSC factors (`lRowPtr`/`lColPtr` for unit lower-triangular $L$, `uRowPtr`/`uColPtr` and `uInvDiag` for upper-triangular $U$) indexed directly in pivot order $0 \dots m-1$.

### `FTRAN`, `BTRAN`, and Hyper-Sparse Solves
- **`FTRAN` ($B x = b$) and `FTRANSparse`**: permutes $b$ into pivot order, solves $L y = P b$ by column-wise zero-skipping forward sweeps (`lColPtr`), solves $U z = y$ by backward column sweep (`uColPtr` and `uInvDiag`), unpermutes $x = Q z$, and applies the PFI eta file in order. When the initial right-hand side has at most $m / 4$ nonzeros, `FTRANSparse` tracks the nonzero support and returns `FColIdx` so callers update and clear only touched rows.
- **`BTRAN` ($B^T \pi = c$) and `BTRANSparse`**: applies the eta file in reverse, permutes into column-pivot order, solves $U^T z = Q^T c$ by forward column sweep (`uColPtr`), solves $L^T w = z$ by backward row sweep (`lRowPtr`), and unpermutes $\pi = P^T w$. The sparse variant `BTRANSparse` tracks the nonzero support and returns `RhoIdx`.
- **Updates**: [`PushEtaSparse`](../lu.go) appends a product-form (PFI) eta column directly from `FColIdx`. After `maxEtaUpdates = 56` updates, or when a pivot fails, the basis is refactored.

---

## 2. Revised Dual and Two-Phase Primal Simplex (`simplex.go`)

Each row $i$ becomes $a_i^T x + s_i = b_i$ with slack bounds $s_i \in [0, \infty)$ for `LessEqual`, $(-\infty, 0]$ for `GreaterEqual`, and $[0, 0]$ for `Equal`. Cutting planes are stored as extra `LessEqual` rows.

### Feasibility Tolerances
A basic variable $x_k$ is primal infeasible when it leaves $[l_k, u_k]$ by more than its tolerance. Slack columns use $\epsilon_p = 10^{-7}$. A structural column uses $\epsilon_p / \max(1, \max_i |a_{ik}|)$, floored at $10^{-12}$, because a value error $e$ in $x_k$ shifts some row by up to $e \max_i |a_{ik}|$. With an absolute tolerance, a big-M indicator resting $5 \cdot 10^{-8}$ outside its bound would shift its row by $0.05$ once reported.

### Dual-First Dispatch
Nonbasic structurals start at the bound favoured by their cost, with the slack basis $B = I$. When the reduced costs are dual feasible within $\epsilon_d = 10^{-7}$, [`LPSolver.Solve`](../simplex.go) runs [`runDualSimplexLoop`](../simplex.go); otherwise, or if the dual loop ends without a verified optimum, it runs [`solveTwoPhasePrimalWithLimit`](../simplex.go).

### Dual Simplex
1. **Leaving row**: the infeasible basic row maximizing $\text{infeas}_r^2 / w_r$, with dual steepest-edge weights $w_r$ updated over `FColIdx` by the Forrest-Goldfarb recurrence (`FTRAN(Tau)` is skipped when `len(FColIdx) <= 1`) and clamped to $[10^{-4}, 10^{12}]$. Appending cut rows keeps existing weights, since the existing rows of $B^{-1}$ do not change when a basic slack row is added.
2. **Pivot row** (`ComputeTableauRow`): computes $\rho = B^{-T} e_r$ via `BTRANSparse(Rho, RhoIdx)`, then accumulates $\bar a_{r} = \rho^T [A \; I]$ over `RhoIdx`. When the total structural nonzeros across touched rows stay below 64, `ComputeTableauRow` tracks the nonbasic support in `TabRowIdx`; otherwise it tracks touched 64-column blocks (`TabRowChunks`) and dispatches `axpy64` over contiguous CSR column runs of length 32 or more.
3. **Ratio test and Bound-Flipping Ratio Test (`selectDualBFRT`)**: Pass 1 pre-filters columns via $|d_k| \le (\theta_{\max} + 2 \cdot 10^{-12}) |\bar a_{r,k}|$ before dividing to update $\theta_{\max} = \min (|d_k| + 10^{-9}) / |\bar a_{r,k}|$; Pass 2 picks, among candidates within $\theta_{\max}$, the largest pivot magnitude ($|\bar a_{r,k}|$ for $n \le 100$, or column-normalized $|\bar a_{r,k}| \cdot \text{feasTol}(k)$ for $n > 100$ so large-weight binary columns do not starve unit-scaled continuous columns), preferring columns whose bound span absorbs the leaving row's violation and breaking near-ties (`1e-11`) by minimum basis row load (`colBasisLoad` via `RowBasicDeg` on non-degenerate steps, or static column degree and index on degenerate steps). When the Pass-2 winner is boxed, cannot absorb the violation alone, and at least one candidate has a positive dual step (`ratio > 1e-7`), `selectDualBFRT` sorts breakpoints and flips boxed nonbasic variables not in equality rows (`!colHasEq[k]`, up to 64 per pivot) to their opposite bounds, updating $x_B$ via one batched `FTRANSparse(Tau, TauIdx)`.
4. **Unbounded dual ray**: if no column is eligible after a fresh basis rebuild, the LP is reported infeasible.

### Two-Phase Primal Simplex
Phase I minimizes the sum of bound violations of basic variables with a composite cost; Phase II uses Dantzig pricing. In Phase I, an improving column that no basic variable blocks cannot occur in exact arithmetic, so it is reported as `StatusNumericalError`, never as an infeasible status. Both loops honour `LPSolver.MaxIter` (shared by both phases) and `LPSolver.Deadline`.

---

## 3. SIMD Kernels (`internal/simd`)

Under the `simd` build experiment on `amd64`, [`kernels_amd64.go`](../internal/simd/kernels_amd64.go) computes `Dot64`, `Axpy64`, `Scale64`, `AbsMax64`, `AllAbsLE64`, `RowActivityBoundsFinite`, `MinReducedSlice64`, `InitReducedRow64`, and `Min2ReducedRow64` with `archsimd.Float64x4` and `archsimd.ClearAVXUpperBits()` (`VZEROUPPER`) for slices of length `minAVX2Len = 32` (`64` for `Min2ReducedRow64`) or more; shorter slices run the same 4-way slice-stepped scalar loops as [`kernels_scalar.go`](../internal/simd/kernels_scalar.go).

- **Microarchitectural transition cleanup (`StoreArray` + `VZEROUPPER`)**: Go 1.27's `GOEXPERIMENT=simd` emits 256-bit VEX `YMM` instructions for `archsimd.Float64x4` while emitting legacy non-VEX SSE instructions (`MOVSD`, `ADDSD`, `MULSD`, `SUBSD`, `MINSD`, `UCOMISD`) for scalar `float64` operations. Returning from an AVX2 loop or executing scalar tail operations with dirty upper `YMM` bits incurs an ~80–85 ns AVX-to-SSE state-transition stall per boundary, and extracting lanes via `GetLo()`/`GetHi()`/`GetElem()` adds `VEXTRACTF128`/`VPEXTRQ` cross-domain stalls while allowing the SSA scheduler to reorder `ClearAVXUpperBits()` above lane extraction. Each reduction kernel therefore spills its 4-lane accumulator to a stack array via `StoreArray(&lanes)` (enforcing an SSA memory dependency), calls `archsimd.ClearAVXUpperBits()` immediately after the vector loop, and reduces the four lanes in scalar registers before executing any tail loop.
- **Sign-symmetric rounding in `Axpy64` and dual-accumulator FMA in `Dot64`**: `Dot64` uses dual 256-bit AVX2/FMA accumulators (`MulAdd`, `VFMADD213PD`) to hide 4-cycle FMA pipeline latency. By contrast, `Axpy64`, `MinReducedSlice64`, `InitReducedRow64`, and `Min2ReducedRow64` use separate `VMULPD`, `VADDPD`, `VSUBPD`, and `VMINPD` instructions (and explicit `float64` product conversion in scalar loops) so that opposite-sign row updates in `ComputeTableauRow` cancel to exact zero ($\text{fl}(y a) + \text{fl}(-y a) = 0$) and reduced-cost scans remain bit-identical across scalar and AVX2 paths.
- **Reach and Amdahl's-law boundary**: in the LP solver, dense tableau rows (`DenseRows`) exist for $n \le 256$ (`DenseThreshold`), so AVX2 `Axpy64` runs on dense tableau row accumulation (`ComputeTableauRow`, for rows with $> 8$ nonzeros) for $32 \le n \le 256$ and on dense dual reduced-cost updates (`solveDual`) when $n \le 256$ and $n + m \ge 32$; on larger models ($n > 256$), `ComputeTableauRow` also dispatches `Axpy64` whenever a CSR row's structural columns form a contiguous index block of length 32 or more (such as capacity or group-balance rows in `t3_mad_g10_i100` and `t3_mad_g4_i1000`). In `goblin/assign`, `AllAbsLE64` validates finite cost bounds at matrix entry, `MinReducedSlice64` computes reduction-transfer slacks in `preprocessSquareJV`, `Min2ReducedRow64` accelerates preprocessing row scans (`preprocessRectangularBV` and `augmentingRowReduction`) via 8-way AVX2 `VSUBPD` + `VMINPD`/`VCMPPD`/`VMOVMSKPD` fast-skipping of non-competitive columns, and `InitReducedRow64` runs at the start of each Dijkstra augmentation when $\max(m, n) \ge 32$. The kernel crossover and paired solver comparisons are in [`benchmarks.md`](benchmarks.md#5-default-build-vs-goexperimentsimd).

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

At the root, [`generateCutRound`](../cuts.go) separates four families, keeps candidates with efficacy at least $10^{-3}$ and coefficient dynamism at most $10^5$, rejects candidates whose inner-product cosine exceeds $0.92$ against an already chosen cut, and adds up to $\min(60, \max(15, m/3))$ cuts per round as one batch. Every cut is normalized and its right-hand side relaxed by $10^{-7}$. Coefficients of magnitude at most $10^{-11}$ are dropped only after adding the largest possible value of the dropped term over the variable's global bounds to the right-hand side; a cut whose dropped term is unbounded is rejected. If the LP fails to re-solve to an optimal basis after a round (infeasible, pivot or time limit, numerical error), the round is rolled back to the last optimal LP state and cutting stops.

1. **Gomory mixed-integer cuts** from up to 25 tableau rows whose basic integer variable has fractional part $f_0 \in [0.01, 0.99]$. With nonbasic variables shifted to their active bounds ($\tilde x_j \ge 0$):
   $$\sum_{j \in \mathcal{I}: f_j \le f_0} \frac{f_j}{f_0} \tilde{x}_j + \sum_{j \in \mathcal{I}: f_j > f_0} \frac{1 - f_j}{1 - f_0} \tilde{x}_j + \sum_{j \in \mathcal{C}: \bar{a}_j > 0} \frac{\bar{a}_j}{f_0} \tilde{x}_j - \sum_{j \in \mathcal{C}: \bar{a}_j < 0} \frac{\bar{a}_j}{1 - f_0} \tilde{x}_j \ge 1$$
   Integer columns with fractional active bounds, and all slacks, are treated as continuous.
2. **Lifted knapsack covers**: on each inequality row relaxed to a binary knapsack (non-binary terms moved to their worst bound, negative binaries complemented), finds a minimal cover $C$ and lifts the remaining items sequentially with a dynamic program over the minimum weight needed to reach each cut activity, giving $\sum_{j \in C} x_j + \sum_{j \notin C} \alpha_j x_j \le |C| - 1$.
3. **VUB disaggregation and $(l, S)$ inequalities**: from rows $\sum_k a_k x_k - C y \le 0$ with $x_k \ge 0$ and binary $y$, derives $x_k \le \min(u_k, C / a_k)\, y$; from chains of flow-balance rows $s_{t-1} + x_t - s_t = d_t$ with VUB-linked production, separates $s_{t-1} + \sum_{\tau \in [t,q] \setminus S} x_\tau + \sum_{\tau \in S} d_{\tau q} y_\tau \ge d_{tq}$.
4. **Single-row mixed-integer rounding**: on inequality rows with finite bounds, shifts each variable to its nearer bound, divides by $\delta = |a_{ik}|$ for up to four fractional integer variables, and applies the MIR function. No multi-row combinations are formed.

---

## 6. Primal Heuristics and Branch-and-Cut (`heuristics.go`, `bab.go`)

### Primal Heuristics
- **Propagated rounding** (`tryPropagatedRounding`): fixes integral columns, then rounds the fractional ones in order of increasing fractional part with queue-based bound tightening (`propagateFromCol`), trying nearest and objective-directed rounding. On mixed models with more than 32 fractional columns it is skipped.
- **Fixed-integer LP** (`solveFixedIntContinuousLP`): with integer decisions fixed, tightens bounds via two passes of `propagateBounds`, solves L1/epigraph continuous and rounding-linked subproblems (`solveEpigraphContinuousLP`) as well as separable single-row bounded continuous knapsacks in closed form, and otherwise re-solves the continuous part warm-started on the full model or as a compact LP over the free columns.
- **Root sub-MIP** (`tryRootSubMIP`): with 1 to 120 fractional integers at the root on models with equality constraints or continuous-linked integer columns (`findContLinkedIntCols`), frees the integer columns of short equality rows that contain a fractional variable (or two basic variables when $n \le 200$), fixes the rest, and solves the reduced model seeded with the outer incumbent cutoff (at most 800 nodes, stopping early after 48 nodes without incumbent improvement, and at most 1.5 s). Before branch-and-bound begins, `tightenUnboundedIntCols` propagates objective-implied upper and lower bounds from the incumbent onto unbounded integer columns so node relaxations remain boxed and dual-feasible across branch switches.
- **Fractional diving** (`tryFractionalDiving`): up to 18 steps of fixing a fractional variable from pure-integer equality rows, propagating, and re-solving warm-started.

### Branching and Search
- **Branching column choice** (`selectBranchVar`): pseudocost product scores $\max(f \bar\psi^-, 10^{-4}) \cdot \max((1-f) \bar\psi^+, 10^{-4})$. At depth 2 or less it strong-branches on up to 5 unreliable candidates with 25 dual pivots per child. Only optimal child solves update pseudocosts; an infeasible child scores high; a pivot-limited child falls back to its pseudocost estimate.
- **Pseudocosts**: each node records its branching side and step length. Gains are stored per unit of step length; steps shorter than $10^{-3}$ are ignored.
- **Node choice**: best bound with plunging into the preferred child, switching to the queue every 8 nodes.

### Incumbents and Near-Integral Nodes
An LP point is integral when every integer variable is within $10^{-6}$ of an integer. It becomes an incumbent only if, after snapping those values, the point violates no bound, row, or integer requirement by more than $10^{-5}$. Otherwise the node branches on the non-fixed integer variable furthest from an integer, even below $10^{-6}$, so its subtree is still explored. A node that cannot be branched keeps its LP bound in the global bound and makes the final status `NumericalError` rather than `Optimal`.

### Limits and Bounds
- `TimeLimit`, `MaxNodes`, and `MaxIter` (total simplex pivots in `Result.Iterations`) stop the search. A node that could not be evaluated because of a limit goes back to the queue, so its bound stays in the global bound.
- A node LP that fails twice (warm start, then slack basis) keeps its parent bound in the global bound.
- **The `BestBound` field**: the minimum over open nodes, unresolved nodes, and the incumbent; at an early root exit it is the root LP or root cut bound. Reporting `StatusOptimal` requires the gap between `BestBound` and the incumbent to close within `RelGapTol`/`AbsGapTol`.

---

## 7. Linear Sum Assignment (`goblin/assign`)

The subpackage [`goblin/assign`](../assign/solver.go) solves the $m \times n$ Linear Sum Assignment Problem (`LSAP`) by maintaining dual potentials $u \in \mathbb{R}^m$ and $v \in \mathbb{R}^n$ that satisfy complementary slackness on the normalized dimensions $(m, n) = (\min(\text{rows}, \text{cols}), \max(\text{rows}, \text{cols}))$:

1. **Single-Pass Validation, Exponent Scaling, and Cache-Tiled Transpose** (`validateAndPrepareCost`): validates finite entries and forbidden $+\infty$ (or $-\infty$ when `sense == Maximize`) costs in one pass. When `rows > cols`, the matrix is transposed in $32 \times 32$ tiles into `ws.workCost`; when `sense == Maximize`, finite costs are negated in the same pass. If any finite entry exceeds $10^{290}$ in magnitude, all finite costs are scaled by $2^{-64}$ (`0x1p-64`) during reduction and augmentation so intermediate differences $(C_{i, j} - v_j) - u_i$ cannot overflow IEEE-754 `float64`, and final dual potentials are rescaled by $2^{64}$ (`0x1p64`).
2. **Dual Preprocessing**:
   - **Square ($m = n$, [`preprocessSquareJV`](../assign/solver.go))**: executes Jonker-Volgenant (1987) row-major column reduction and reduction transfer over uniquely matched rows. If nearly all rows remain unassigned (`numFree >= n - max(2, n/32)`), [`isCollinearSquare`](../assign/solver.go) tests whether column increments across unassigned rows are sign-concordant: collinear (rank-1 or Monge) matrices run a greedy-chain warm start ordered by extreme-column spread $C_{i, j_{\max}} - C_{i, j_{\min}}$, whereas non-collinear row-dominated matrices normalize row scales via $u_i = \min_j C_{i, j}$ and repeat column reduction and reduction transfer on $C_{i, j} - u_i$. Finally, [`augmentingRowReduction`](../assign/solver.go) executes two passes of chain-following augmenting row reduction.
   - **Rectangular ($m < n$, [`preprocessRectangularBV`](../assign/solver.go))**: executes Bijsterbosch-Volgenant (2010) row reduction keeping $v_j = 0$ on unassigned columns (`row4col[j] < 0`), followed by two passes of [`augmentingRowReduction`](../assign/solver.go).
3. **3-Way Partitioned Shortest Augmenting Path** ([`augmentRow`](../assign/solver.go)): for each remaining unassigned row $i^*$, initializes $d_j = (C_{i^*, j} - v_j) - u_{i^*}$ and $\min = \min_j d_j$ via [`simd.InitReducedRow64With`](../internal/simd/kernels_amd64.go), then partitions columns achieving $d_j = \min$ into the initial frontier `remaining[0:up]`, returning immediately (`1-hop` fast path) if an unassigned column (`row4col[j] == -1`) achieves $d_j = \min$. Otherwise it maintains the Jonker-Volgenant 3-way column permutation `remaining[0:low]` (permanently labeled), `remaining[low:up]` (current minimum-$d$ frontier), and `remaining[up:n]` (unscanned) in lockstep with `d[0:n]`, so `d[k]` always stores the label distance of column `remaining[k]` without indirect index gathers. Whenever the frontier is exhausted (`low == up`), a 4-way unrolled contiguous scan over `d[up:n]` finds the next minimum $\min$ and partitions its tie group into `remaining[low:up]`; popping each frontier column $j = \text{remaining}[\text{low}]$ relaxes outgoing edges from its matched row $i = \text{row4col}[j]$ across `remaining[up:n]`, promoting newly tight columns into `remaining[low:up]` or returning immediately when an unassigned column reaches $d = \min$. Dual potentials then update in $O(\text{low})$ over `remaining[0:low]` and `d[0:low]`, and the primal matching augments along `pred`.

---

## Known Limits of the Pure-Go Engine

- **LP updates and primal fallback**: while the basis solver uses a three-stage singleton and sparse Markowitz LU with `FTRANSparse`/`BTRANSparse` and dual bound flipping (`selectDualBFRT`), basis updates between full factor rebuilds use a product-form (PFI) eta file rather than Forrest-Tomlin $U$-updates, and the Phase II primal fallback uses full Dantzig pricing.
- **Scaling**: there is no row or column scaling; only the primal bound tolerance is normalized by column magnitude.
- **Cuts at the root only**: cuts are separated at the root node, using global bounds, and kept for the whole tree.
- **Reference solver**: parity is checked against CBC 2.10.3, which itself returns wrong answers on some instances (see [`testdata/regression`](../testdata/regression)); `bench/difftest_cbc.py` verifies mismatches independently before blaming either solver.
