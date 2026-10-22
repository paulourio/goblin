# Solver Internals and Mathematical Specification

The `gopt` optimization engine combines a Revised Dual and Two-Phase Primal Simplex LP solver ([`lu.go`](../lu.go), [`simplex.go`](../simplex.go)) with a Branch-and-Cut MILP framework ([`presolve.go`](../presolve.go), [`cuts.go`](../cuts.go), [`heuristics.go`](../heuristics.go), [`bab.go`](../bab.go)). This specification details the mathematical formulations and implementation invariants of each subsystem.

---

## 1. Three-Stage Basis Factorization and PFI Eta Updates (`lu.go`)

The LP solver maintains a nonsingular basis matrix $B \in \mathbb{R}^{m \times m}$ consisting of $m$ basic columns selected from the $n$ structural variables and $m$ row-slack variables.

### Singleton Peeling and Dense-Nucleus Partial-Pivoting LU
The method [`LUFactorization.Factorize`](../lu.go) decomposes $B$ in three stages before dense elimination:
1. **Identity Fast Path**: When the basis consists solely of unit slack columns with unit diagonal entries, `Factorize` sets `isIdentity = true` and skips elimination ($O(m)$ check).
2. **Triangular Singleton Elimination**:
   - **Stage 1 (Column Singletons)**: Iteratively peels basic columns containing a single remaining active nonzero entry, recording the causal row/column permutation in `prePerm` ($0 \le k < f_{\text{start}}$).
   - **Stage 2 (Row Singletons)**: Iteratively peels active rows containing a single remaining nonzero entry, recording the reverse-order permutation in `postPerm` ($r_{\text{end}} \le k < m$).
3. **Dense-Nucleus Gaussian Elimination ($O(n_{\text{nuc}}^3)$)**:
   - The remaining active submatrix of dimension $n_{\text{nuc}} = r_{\text{end}} - f_{\text{start}}$ is scattered into an 8-element-aligned row-major buffer (`nucStride = (nNuc + 7) &^ 7`).
   - For each nucleus step $k \in \{0, \dots, n_{\text{nuc}}-1\}$, partial pivoting swaps in the row $i \ge k$ maximizing $|U_{i,k}|$ (returning a singular-basis error if $\max_{i \ge k} |U_{i,k}| < 10^{-11}$).
   - Each nucleus row $i$ tracks its active column envelope in $L$ (`lFirst[i]`, `lLast[i]`) and $U$ (`uFirst[i]`, `uLast[i]`), restricting elimination updates `simd.Axpy64` to $[\text{firstCol}, \text{lastCol}]$.

### Forward and Backward Transformation (`FTRAN` and `BTRAN`)
- **`FTRAN` ($B x = b$)**: Solves Stage 1 column singletons forward, solves the dense nucleus $L y = P b$ and $U x_{\text{nuc}} = y$ using `simd.Dot64` over active row envelopes, solves Stage 2 row singletons backward, and applies accumulated Product Form of the Inverse (`PFI`) column-spike eta vectors (`etaVector`).
- **`BTRAN` ($B^T \pi = c_B$)**: Applies PFI column-spike eta vectors in reverse order, propagates through Stage 2 row singletons, solves $U^T z = \tilde{c}_B$ via `simd.Axpy64` and $L^T w = z$ on the nucleus, propagates through Stage 1 column singletons, and unpermutes $\pi$.
- **Refactorization Cadence**: Up to `maxEtaUpdates = 56` basis changes are appended as sparse PFI column-spike eta vectors before `LU.NeedsRefactor()` triggers a fresh `Factorize`.

---

## 2. Revised Dual and Two-Phase Primal Simplex (`simplex.go`)

Every constraint row $i$ is converted into an equality $a_i^T x + s_i = b_i$ by introducing a slack variable $s_i = b_i - a_i^T x$ with bounds:
- $s_i \in [0, +\infty)$ for `LessEqual` ($\le$),
- $s_i \in (-\infty, 0]$ for `GreaterEqual` ($\ge$),
- $s_i \in [0, 0]$ for `Equal` ($=$).

### Crash Basis and Dual-First Dispatch
At initialization, nonbasic structural variables are placed at their most favorable finite bound according to reduced cost sign (`StatusAtLower` if $c_j \ge 0$, `StatusAtUpper` if $c_j < 0$), and row slacks form the identity basis $B = I_m$. When all nonbasic variables are dual-feasible ($d_j \ge -\epsilon_{\text{dual}}$ at lower bound, $d_j \le \epsilon_{\text{dual}}$ at upper bound), [`LPSolver.Solve`](../simplex.go) enters [`runDualSimplexLoop`](../simplex.go) directly.

### Dual Simplex with Dual Steepest-Edge (`DSE`) Pricing and Sparse Tableau Rows
1. **Dual Steepest-Edge Leaving Row Selection**: Selects the basic row $r$ maximizing the weighted primal violation score $\text{infeas}_r^2 / w_r$, where the steepest-edge weight `ws.DSE[r]` is initialized to $1.0$, updated at each pivot via the Forrest-Goldfarb recurrence alongside `UpdateBTRAN`, and clamped to $[10^{-4}, 10^{12}]$ to prevent overflow across deep Branch-and-Bound subtrees.
2. **Sparse Pivot Row Computation (`ComputeTableauRow`)**: Computes $\rho = B^{-T} e_r$ (`BTRAN`) and accumulates nonbasic inner products $\bar{a}_{r,j} = \rho^T A_{\cdot,j}$ by traversing the `CSC` matrix (`ColPtr`, `RowIdx`, `ColVal`) only for rows $i$ with $|\rho_i| > 10^{-13}$. Nonzero column indices are recorded in `ws.TabRowIdx` (`ws.TabRowMark`), so ratio tests and dual updates inspect only $O(\text{nnz}(\bar{a}_r))$ columns rather than scanning all $n + m$ variables.
3. **Harris Two-Pass Ratio Test and Bounded-Span Priority**:
   - Pass 1 computes the Harris tolerance threshold $\theta_{\max} = \min_{k \in \mathcal{E}} (|d_k| + \epsilon_{\text{dual}}) / |\bar{a}_{r,k}|$ over eligible nonbasic columns in `TabRowIdx`.
   - Pass 2 selects among eligible candidates ($|d_k| / |\bar{a}_{r,k}| \le \theta_{\max} + 10^{-12}$) the column maximizing pivot magnitude $|\bar{a}_{r,k}|$ (with deterministic lowest-index tie-breaking), prioritizing candidates whose bound span $\text{UB}_k - \text{LB}_k$ can absorb the leaving row's primal violation in a single pivot ($\text{infeas}_r \le |\bar{a}_{r,k}| (\text{UB}_k - \text{LB}_k)$) over tightly boxed variables that would trigger degenerate bound-to-bound flips.
4. **Two-Phase Primal Fallback and Resource Limits**:
   - If the initial crash basis is not dual-feasible or if Dual Simplex encounters numerical stall, [`solveTwoPhasePrimalWithLimit`](../simplex.go) minimizes Phase I artificial bound violations before running Phase II primal simplex (`runPrimalSimplex`).
   - Both Dual Simplex and Two-Phase Primal Simplex enforce `LPSolver.MaxIter` (shared across Phase I and Phase II) and `LPSolver.Deadline`. Exhausting either budget returns `StatusIterationLimit` or `StatusTimeLimit` immediately; an unconverged Phase I termination is never misclassified as `StatusInfeasible`.

---

## 3. Hardware SIMD Vector Kernels (`internal/simd`)

When built with `GOEXPERIMENT=simd` on `amd64`, [`internal/simd/kernels_amd64.go`](../internal/simd/kernels_amd64.go) uses Go 1.27's `simd/archsimd` package (`archsimd.Float64x4`):
- **Non-FMA Instruction Selection and Pairwise Reduction Parity**: Both `Dot64` and `Axpy64` use separate vector multiplication (`Float64x4.Mul`, emitting `VMULPD`) and vector addition (`Float64x4.Add`, emitting `VADDPD`) rather than fused multiply-add (`VFMADD231PD`), preserving identical two-rounding IEEE-754 arithmetic with the scalar fallback (`kernels_scalar.go`). Partial sums are reduced via `(s0 + s1) + (s2 + s3)`, matching `Float64x4` horizontal extraction `(lo.GetElem(0) + lo.GetElem(1)) + (hi.GetElem(0) + hi.GetElem(1))` so scalar and SIMD builds follow identical simplex pivot paths.
- **Length-Gated AVX2 Dispatch (`n >= 256`) and Sparse Corpus Regime**: In Go 1.27, functions touching 256-bit `YMM` registers emit `VZEROUPPER` on return. For short and medium slices (`n < 256`), `Dot64` and `Axpy64` execute 4-way unrolled scalar loops on 128-bit `XMM` registers without `YMM` transition overhead; slices with `n >= 256` dispatch to `dot64AVX2` and `axpy64AVX2`. Because the 31-family benchmark corpus consists of sparse LP/MILP matrices whose post-singleton LU nucleus rows have active spans shorter than 256 nonzeros, scalar and `GOEXPERIMENT=simd` builds exhibit near-identical (`~1.02x`) end-to-end latency on the corpus, while `n >= 256` dense microbenchmarks verify the AVX2 kernels directly.

---

## 4. MIP Presolve (`presolve.go`)

Before solving the root LP relaxation, [`presolveWithDeadline`](../presolve.go) executes up to `maxPasses` rounds of four scalar sparse-matrix reductions (checking the solve `deadline` at each pass):
1. **Euclidean GCD Reduction**: On pure-integer constraint rows, divides all coefficients by $g = \gcd(|a_{i,j_1}|, \dots, |a_{i,j_k}|) > 1$ and rounds the scaled right-hand side ($\lfloor b_i / g \rfloor$ for $\le$, $\lceil b_i / g \rceil$ for $\ge$, or flags `Infeasible` if $b_i / g \notin \mathbb{Z}$ for $=$).
2. **Savelsbergh Activity Bound Tightening**: Computes minimum and maximum row activities $\underline{\alpha}_i, \overline{\alpha}_i$ over the sparse `CSR` row slices and tightens variable bounds $l_j, u_j$ using residual row slack.
3. **Redundant Row Removal**: Eliminates `LessEqual` rows with $\overline{\alpha}_i \le b_i + \epsilon$ and `GreaterEqual` rows with $\underline{\alpha}_i \ge b_i - \epsilon$.
4. **Binary Coefficient Strengthening**: For binary variables $x_j \in \{0, 1\}$ in $\le$ rows where $\overline{\alpha}_i - |a_{i,j}| < b_i < \overline{\alpha}_i$, reduces $|a_{i,j}|$ and $b_i$ by $\delta = \overline{\alpha}_i - b_i$.

---

## 5. Cutting-Plane Generators (`cuts.go`)

At the root node, [`generateCutRound`](../cuts.go) separates violated inequalities across four cut families, filters candidates by efficacy and cosine parallelism ($\frac{|a^T a'|}{\|a\| \|a'\|} \le 0.92$, `maxOrthoCosine = 0.92`), and appends up to `maxCutsPerRound = min(60, max(15, m/3))` cuts per round to the LP relaxation via `LPSolver.AddCut`:
1. **Gomory Mixed-Integer (`GMI`) Cuts**: Extracted from up to 25 tableau rows $r$ per round where a basic integer variable $x_{B_r}$ takes a fractional value $f_0 = x_{B_r}^* - \lfloor x_{B_r}^* \rfloor \in [0.01, 0.99]$:
   $$\sum_{j \in \mathcal{I}: f_j \le f_0} \frac{f_j}{f_0} \tilde{x}_j + \sum_{j \in \mathcal{I}: f_j > f_0} \frac{1 - f_j}{1 - f_0} \tilde{x}_j + \sum_{j \in \mathcal{C}: \bar{a}_j > 0} \frac{\bar{a}_j}{f_0} \tilde{x}_j - \sum_{j \in \mathcal{C}: \bar{a}_j < 0} \frac{\bar{a}_j}{1 - f_0} \tilde{x}_j \ge 1$$
2. **Sequentially Lifted 0-1 Knapsack Cover Cuts**: Identifies minimal covers $C$ on binary knapsack constraints $\sum_{j \in C} w_j x_j > b$ (after complementing upper-bounded variables) and performs sequential dynamic-programming up-lifting. A DP table `minWeight[0..|C|-1]` computes the exact minimum weight achievable by any subset of $r$ cover items; each non-cover binary variable $j \notin C$ is lifted with coefficient $\alpha_j = |C| - 1 - r^*$ where $r^*$ is the largest cardinality satisfying $\text{minWeight}[r^*] + w_j \le b$, yielding $\sum_{j \in C} x_j + \sum_{j \notin C} \alpha_j x_j \le |C| - 1$.
3. **Flow Cover and Variable Upper Bound (`VUB`) $(l, S)$ Cuts**: Detects single-node flow structures containing continuous variables bounded by binary indicators ($0 \le y_j \le u_j x_j$) and generates disaggregated VUB and strengthened $(l, S)$ inequalities.
4. **Single-Row Complemented Mixed-Integer Rounding (`c-MIR`) Cuts**: On inequality rows where all variables have finite simple bounds (`VarLB`, `VarUB`), shifts each variable to its closer bound (`lpPrimal[col] - l <= u - lpPrimal[col]`), scales the row by divisor $\delta = |a_{i,k}|$ for up to `4` fractional integer variables (`triedDivs < 4`) in the row, and applies the mixed-integer rounding function.

---

## 6. Primal Heuristics and Branch-and-Cut (`heuristics.go`, `bab.go`)

### Root and Node Primal Heuristics
- **Propagated Rounding (`tryPropagatedRounding`)**: Ranks integer variables by ascending distance to the nearest integer (`|v - round(v)|`), fixes already-integral columns, and skips mixed-integer nodes with more than `32` fractional integer variables (`hasCont && len(fracCands) > 32`) to avoid expensive continuous LP solves on loosely relaxed nodes. Evaluates two rounding strategies (`strat = 0` nearest-integer rounding, `strat = 1` objective-directed rounding when `|v - round(v)| > 0.15`) coupled with incremental queue-based constraint propagation (`propagateFromCol`).
- **Compact Fixed-Integer Continuous LP (`solveFixedIntContinuousLP`)**: Whenever all integer variables are fixed to candidate integer values on a mixed-integer problem, first attempts warm-started Dual Simplex (`auxSolver.Solve()`) on the full model; if that does not yield an optimal feasible solution, constructs and solves a compact continuous-only LP (`__cont_lp__`) over only the $n_{\text{cont}}$ unfixed continuous variables.
- **Equality-Expanded Root Sub-MIP (`tryRootSubMIP`)**: Activates when the root LP has $1 \le \text{baseFrac} \le 120$ fractional integer variables, expands the free set across equality rows of length $\text{nnz}_i \le 32$ that contain a fractional variable or at least two basic variables (`basicInRow >= 2`), and solves the reduced neighborhood sub-MIP (`__submip__`, `MaxNodes = 800`, `TimeLimit = min(1.5s, remaining)`) when the expanded free integer count satisfies $1 \le n_{\text{frac}} \le 240$ and $n_{\text{frac}} < |\text{IntCols}|$. These complexity bounds prevent sub-MIP stalls on large dense instances while closing structured partitioning gaps at node 1.
- **Fractional Diving (`tryFractionalDiving`)**: Identifies equality-support integer variables in pure-integer equality rows of length $2 \le \text{nnz}_i \le 32$ and plunges up to `maxDiveSteps = min(18, len(IntCols))` steps by fixing a fractional integer variable, propagating bounds, and re-optimizing with warm-started Dual Simplex (`diveSolver.Solve()`), checking `diveSolver.timeExpired()` at every step.

### Branching, Feasibility Verification, and Termination Guarantees
- **Strong Branching + Pseudocost Scoring (`selectBranchVar`)**: At shallow tree depths (`depth <= 2`), evaluates up to `5` most-fractional candidates with bounded Dual Simplex lookahead (`MaxIter = 25`) to update pseudocosts (`downSum`, `upSum`), switching to pure pseudocost product scoring $\max(\Delta^-, 10^{-4}) \cdot \max(\Delta^+, 10^{-4})$ deeper in the tree.
- **Hybrid Plunging and Best-Bound Queue (`nodeQueue`)**: Alternates between depth-first child plunging (preserving warm basis states) and best-bound priority-queue selection every 8 nodes.
- **Full-Row Feasibility Verification**: Every candidate integer solution — including snapped near-integral LP solutions at the root node, after cut rounds, or at B&B tree nodes — is verified against the original problem constraints and bounds via `p.MaxViolation(cand, true) <= PrimalTol` before updating the incumbent objective `bestObjMin`.
- **Sound Global Dual Bound and Limit Statuses**:
  - If a node LP terminates with `StatusTimeLimit`, the node is pushed back onto `nodeQueue` and search halts immediately. If a node LP hits `StatusIterationLimit` or `StatusNumericalError` even after a fresh crash-basis retry, its parent lower bound is recorded in `unsolvedMinBound` rather than pruning the subtree.
  - Upon loop exit, `bestBoundMin` is computed as the minimum bound across all remaining open nodes (`pq`, `plungeNode`, and `unsolvedMinBound`). `StatusOptimal` is returned only when `isGapClosed(bestBoundMin, bestObjMin, RelGapTol, AbsGapTol)` holds (`Gap == 0` when no open nodes remain). Otherwise, `solveBranchAndCut` returns `StatusTimeLimit`, `StatusNodeLimit`, or `StatusIterationLimit` with the honest open `BestBound` and relative `Gap`.
