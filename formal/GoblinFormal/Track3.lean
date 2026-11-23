/-
GoblinFormal.Track3: Verified LP primal-dual optimality, Neumaier-Shcherbina
safe dual bounding, and Farkas infeasibility checking over exact `Rat`.
-/

import GoblinFormal.Common.Basic
import GoblinFormal.Common.Json

namespace GoblinFormal.Track3

/-- Linear constraint relation sense. -/
inductive ConstraintSense where
  | le
  | ge
  | eq
  deriving Repr, DecidableEq, BEq

/-- Sparse nonzero matrix entry `(col, coeff)` in a constraint row. -/
structure SparseTerm (n : Nat) where
  col : Fin n
  coeff : Rat
  deriving Repr, DecidableEq, BEq

/-- Linear program in minimization canonical form `min objConst + ∑ obj_j x_j`. -/
structure LPInstance where
  numVars : Nat
  numRows : Nat
  objConst : Rat
  obj : Fin numVars → Rat
  sense : Fin numRows → ConstraintSense
  rows : Fin numRows → List (SparseTerm numVars)
  rhs : Fin numRows → Rat
  lb : Fin numVars → Option Rat
  ub : Fin numVars → Option Rat

/-- Evaluates a sparse linear expression `∑ (coeff * x[col])`. -/
def evalTerms {n : Nat} (x : Fin n → Rat) : List (SparseTerm n) → Rat
  | [] => 0
  | t :: ts => t.coeff * x t.col + evalTerms x ts

/-- Evaluates constraint row `i` activity `a_i · x`. -/
def rowActivity (LP : LPInstance) (x : Fin LP.numVars → Rat) (i : Fin LP.numRows) : Rat :=
  evalTerms x (LP.rows i)

/-- Evaluates the minimization objective `objConst + ∑ obj_j * x_j`. -/
def LPObjective (LP : LPInstance) (x : Fin LP.numVars → Rat) : Rat :=
  LP.objConst + finSum LP.numVars (fun j => LP.obj j * x j)

/-- Variable bound satisfaction (`none` means `∓∞`). -/
def SatisfiesVarBounds (lb ub : Option Rat) (xj : Rat) : Prop :=
  (match lb with | some l => l ≤ xj | none => True) ∧
  (match ub with | some u => xj ≤ u | none => True)

/-- Boolean checker for variable bounds. -/
def checkVarBounds (lb ub : Option Rat) (xj : Rat) : Bool :=
  (match lb with | some l => decide (l ≤ xj) | none => true) &&
  (match ub with | some u => decide (xj ≤ u) | none => true)

/-- Constraint row satisfaction (`≤`, `≥`, `=`). -/
def SatisfiesRow (sense : ConstraintSense) (act rhs : Rat) : Prop :=
  match sense with
  | .le => act ≤ rhs
  | .ge => rhs ≤ act
  | .eq => act = rhs

/-- Boolean checker for constraint row satisfaction. -/
def checkRowConstraint (sense : ConstraintSense) (act rhs : Rat) : Bool :=
  match sense with
  | .le => decide (act ≤ rhs)
  | .ge => decide (rhs ≤ act)
  | .eq => act == rhs

/-- Primal feasibility: `x` satisfies all variable bounds and all row constraints. -/
def PrimalFeasible (LP : LPInstance) (x : Fin LP.numVars → Rat) : Prop :=
  (∀ j : Fin LP.numVars, SatisfiesVarBounds (LP.lb j) (LP.ub j) (x j)) ∧
  (∀ i : Fin LP.numRows, SatisfiesRow (LP.sense i) (rowActivity LP x i) (LP.rhs i))

private def allFinAux (n : Nat) (p : Fin n → Bool) (i : Nat) (_hi : i ≤ n) : Bool :=
  if h : i < n then
    if p ⟨i, h⟩ then allFinAux n p (i + 1) h else false
  else
    true
termination_by n - i

private theorem allFinAux_iff_aux (n : Nat) (p : Fin n → Bool) :
    ∀ (k i : Nat) (hi : i ≤ n), n - i = k →
      (allFinAux n p i hi = true ↔ ∀ j : Fin n, i ≤ j.val → p j = true) := by
  intro k
  induction k with
  | zero =>
    intro i hi hdiff
    have hge : ¬i < n := by omega
    unfold allFinAux
    simp only [hge, ↓reduceDIte, true_iff]
    intro j hj
    have hjlt := j.isLt
    omega
  | succ k ih =>
    intro i hi hdiff
    have hlt : i < n := by omega
    unfold allFinAux
    simp only [hlt, ↓reduceDIte]
    cases hp : p ⟨i, hlt⟩
    · simp only [Bool.false_eq_true, ↓reduceIte, false_iff]
      intro hforall
      have hbad := hforall ⟨i, hlt⟩ (Nat.le_refl i)
      rw [hp] at hbad
      cases hbad
    · simp only [↓reduceIte]
      have hih := ih (i + 1) hlt (by omega)
      rw [hih]
      constructor
      · intro hrest j hj
        by_cases heq : i = j.val
        · have hjEq : j = ⟨i, hlt⟩ := Fin.ext heq.symm
          rw [hjEq, hp]
        · exact hrest j (by omega)
      · intro hforall j hj
        exact hforall j (by omega)

private def allFinBool (n : Nat) (p : Fin n → Bool) : Bool :=
  allFinAux n p 0 (Nat.zero_le n)

private theorem allFinBool_iff {n : Nat} {p : Fin n → Bool} :
    allFinBool n p = true ↔ ∀ j : Fin n, p j = true := by
  unfold allFinBool
  rw [allFinAux_iff_aux n p n 0 (Nat.zero_le n) rfl]
  constructor
  · intro h j
    exact h j (Nat.zero_le j.val)
  · intro h j _
    exact h j

/-- Executable primal feasibility checker. -/
def checkPrimalFeasible (LP : LPInstance) (x : Fin LP.numVars → Rat) : Bool :=
  allFinBool LP.numVars (fun j => checkVarBounds (LP.lb j) (LP.ub j) (x j)) &&
  allFinBool LP.numRows (fun i => checkRowConstraint (LP.sense i) (rowActivity LP x i) (LP.rhs i))

private theorem checkVarBounds_sound {lb ub : Option Rat} {xj : Rat}
    (h : checkVarBounds lb ub xj = true) : SatisfiesVarBounds lb ub xj := by
  unfold checkVarBounds at h
  rw [Bool.and_eq_true] at h
  refine ⟨?_, ?_⟩
  · cases lb with
    | none => trivial
    | some l =>
      simp only [decide_eq_true_eq] at h
      exact h.1
  · cases ub with
    | none => trivial
    | some u =>
      simp only [decide_eq_true_eq] at h
      exact h.2

private theorem checkRowConstraint_sound {sense : ConstraintSense} {act rhs : Rat}
    (h : checkRowConstraint sense act rhs = true) : SatisfiesRow sense act rhs := by
  cases sense with
  | le =>
    simp only [checkRowConstraint, decide_eq_true_eq] at h
    exact h
  | ge =>
    simp only [checkRowConstraint, decide_eq_true_eq] at h
    exact h
  | eq =>
    simp only [checkRowConstraint, beq_iff_eq] at h
    exact h

theorem checkPrimalFeasible_sound {LP : LPInstance} {x : Fin LP.numVars → Rat}
    (h : checkPrimalFeasible LP x = true) : PrimalFeasible LP x := by
  unfold checkPrimalFeasible at h
  rw [Bool.and_eq_true, allFinBool_iff, allFinBool_iff] at h
  exact ⟨fun j => checkVarBounds_sound (h.1 j), fun i => checkRowConstraint_sound (h.2 i)⟩

/-- Projects a dual multiplier onto the dual cone of a constraint row:
`le` -> `y ≤ 0`, `ge` -> `y ≥ 0`, `eq` -> unconstrained. -/
def projDual (sense : ConstraintSense) (y : Rat) : Rat :=
  match sense with
  | .le => if y ≤ 0 then y else 0
  | .ge => if 0 ≤ y then y else 0
  | .eq => y

theorem projDual_row_le {sense : ConstraintSense} {y act rhs : Rat}
    (hrow : SatisfiesRow sense act rhs) :
    projDual sense y * rhs ≤ projDual sense y * act := by
  cases sense with
  | le =>
    unfold projDual
    dsimp only
    split
    · next hy => exact rat_mul_le_mul_of_nonpos_left hrow hy
    · simp only [Rat.zero_mul]
      exact Rat.le_refl
  | ge =>
    unfold projDual
    dsimp only
    split
    · next hy => exact Rat.mul_le_mul_of_nonneg_left hrow hy
    · simp only [Rat.zero_mul]
      exact Rat.le_refl
  | eq =>
    unfold projDual
    dsimp only
    rw [hrow]
    exact Rat.le_refl

private def addTermsArr {n : Nat} (scale : Rat) (ts : List (SparseTerm n))
    (acc : Array Rat) (hsz : acc.size = n) : { a : Array Rat // a.size = n } :=
  match ts with
  | [] => ⟨acc, hsz⟩
  | t :: rest =>
    let idx := t.col.val
    have hlt : idx < acc.size := by rw [hsz]; exact t.col.isLt
    let acc' := acc.set idx (acc[idx] + scale * t.coeff) hlt
    have hsz' : acc'.size = n := by rw [Array.size_set]; exact hsz
    addTermsArr scale rest acc' hsz'

private theorem addTermsArr_step_eq {n : Nat} (scale : Rat) (t : SparseTerm n)
    (x : Fin n → Rat) (acc : Array Rat) (hsz : acc.size = n) :
    let hlt : t.col.val < acc.size := by rw [hsz]; exact t.col.isLt
    let acc' := acc.set t.col.val (acc[t.col.val] + scale * t.coeff) hlt
    have hsz' : acc'.size = n := by rw [Array.size_set]; exact hsz
    finSum n (fun j => acc'[j.val]'(by rw [hsz']; exact j.isLt) * x j) =
    finSum n (fun j => acc[j.val]'(by rw [hsz]; exact j.isLt) * x j) +
      scale * (t.coeff * x t.col) := by
  intro hlt acc' hsz'
  have hpoint :
      ∀ j : Fin n,
        acc'[j.val]'(by rw [hsz']; exact j.isLt) * x j =
        acc[j.val]'(by rw [hsz]; exact j.isLt) * x j +
          (if j = t.col then scale * (t.coeff * x t.col) else 0) := by
    intro j
    by_cases hj : j = t.col
    · subst hj
      simp only [acc', Array.getElem_set_self, ↓reduceIte]
      rw [Rat.add_mul, Rat.mul_assoc]
    · have hne : t.col.val ≠ j.val := fun heq => hj (Fin.ext heq.symm)
      have hjlt : j.val < acc.size := by rw [hsz]; exact j.isLt
      have hget : acc'[j.val]'(by rw [hsz']; exact j.isLt) = acc[j.val]'hjlt :=
        Array.getElem_set_ne hlt hjlt hne
      simp only [hj, ↓reduceIte, Rat.add_zero]
      rw [hget]
  rw [finSum_congr hpoint, finSum_add, finSum_single]

private theorem addTermsArr_dot_eq {n : Nat} (scale : Rat) (ts : List (SparseTerm n))
    (x : Fin n → Rat) :
    ∀ (acc : Array Rat) (hsz : acc.size = n),
      let res := addTermsArr scale ts acc hsz
      finSum n (fun j => res.1[j.val]'(by rw [res.2]; exact j.isLt) * x j) =
      finSum n (fun j => acc[j.val]'(by rw [hsz]; exact j.isLt) * x j) + scale * evalTerms x ts := by
  induction ts with
  | nil =>
    intro acc hsz
    simp only [addTermsArr, evalTerms, Rat.mul_zero, Rat.add_zero]
  | cons t rest ih =>
    intro acc hsz
    dsimp only [addTermsArr, evalTerms]
    let hlt : t.col.val < acc.size := by rw [hsz]; exact t.col.isLt
    let acc' := acc.set t.col.val (acc[t.col.val] + scale * t.coeff) hlt
    have hsz' : acc'.size = n := by rw [Array.size_set]; exact hsz
    have hih := ih acc' hsz'
    have hstep := addTermsArr_step_eq scale t x acc hsz
    rw [hih, hstep, Rat.add_assoc, Rat.mul_add]

private def addRowsArr (LP : LPInstance) (y : Fin LP.numRows → Rat)
    (i : Nat) (hi : i ≤ LP.numRows)
    (acc : Array Rat) (hsz : acc.size = LP.numVars) :
    { a : Array Rat // a.size = LP.numVars } :=
  match i with
  | 0 => ⟨acc, hsz⟩
  | k + 1 =>
    let res := addRowsArr LP y k (Nat.le_of_succ_le hi) acc hsz
    let ik : Fin LP.numRows := ⟨k, hi⟩
    let yi := y ik
    if yi == 0 then
      res
    else
      addTermsArr yi (LP.rows ik) res.1 res.2

private theorem addRowsArr_dot_eq (LP : LPInstance) (y : Fin LP.numRows → Rat)
    (x : Fin LP.numVars → Rat) (acc : Array Rat) (hsz : acc.size = LP.numVars) :
    ∀ (i : Nat) (hi : i ≤ LP.numRows),
      let res := addRowsArr LP y i hi acc hsz
      finSum LP.numVars (fun j => res.1[j.val]'(by rw [res.2]; exact j.isLt) * x j) =
      finSum LP.numVars (fun j => acc[j.val]'(by rw [hsz]; exact j.isLt) * x j) +
        finSum i (fun p : Fin i =>
          y ⟨p.val, Nat.lt_of_lt_of_le p.isLt hi⟩ *
            rowActivity LP x ⟨p.val, Nat.lt_of_lt_of_le p.isLt hi⟩) := by
  intro i
  induction i with
  | zero =>
    intro hi
    simp only [addRowsArr, finSum_zero_n, Rat.add_zero]
  | succ k ih =>
    intro hi
    dsimp only [addRowsArr]
    have hk : k ≤ LP.numRows := Nat.le_of_succ_le hi
    have hih := ih hk
    let prev := addRowsArr LP y k hk acc hsz
    let ik : Fin LP.numRows := ⟨k, hi⟩
    have hsplit :
        finSum (k + 1) (fun p : Fin (k + 1) =>
          y ⟨p.val, Nat.lt_of_lt_of_le p.isLt hi⟩ *
            rowActivity LP x ⟨p.val, Nat.lt_of_lt_of_le p.isLt hi⟩) =
        finSum k (fun p : Fin k =>
          y ⟨p.val, Nat.lt_of_lt_of_le p.isLt hk⟩ *
            rowActivity LP x ⟨p.val, Nat.lt_of_lt_of_le p.isLt hk⟩) +
          y ik * rowActivity LP x ik := by
      rw [finSum_succ]
      rfl
    split
    · next hy =>
      have hy0 : y ik = 0 := beq_iff_eq.mp hy
      have hz : y ik * rowActivity LP x ik = 0 := by rw [hy0, Rat.zero_mul]
      rw [hsplit, hz, Rat.add_zero]
      exact hih
    · have hterms := addTermsArr_dot_eq (y ik) (LP.rows ik) x prev.1 prev.2
      change finSum LP.numVars (fun j => (addTermsArr (y ik) (LP.rows ik) prev.1 prev.2).1[j.val]'(by
        rw [(addTermsArr (y ik) (LP.rows ik) prev.1 prev.2).2]; exact j.isLt) * x j) = _
      rw [hterms, hih, hsplit, Rat.add_assoc]
      rfl

/-- Computes `y^T A` as an `Array Rat` of length `LP.numVars` in `O(nnz + numVars)`. -/
def computeColDotsArr (LP : LPInstance) (y : Fin LP.numRows → Rat) :
    { a : Array Rat // a.size = LP.numVars } :=
  addRowsArr LP y LP.numRows (Nat.le_refl _)
    (Array.replicate LP.numVars (0 : Rat)) Array.size_replicate

/-- Functional view of `y^T A`. -/
def computeColDots (LP : LPInstance) (y : Fin LP.numRows → Rat) : Fin LP.numVars → Rat :=
  let arr := computeColDotsArr LP y
  fun j => arr.1[j.val]'(by rw [arr.2]; exact j.isLt)

theorem computeColDotsArr_dot_eq (LP : LPInstance) (y : Fin LP.numRows → Rat)
    (x : Fin LP.numVars → Rat) :
    let arr := computeColDotsArr LP y
    finSum LP.numVars (fun j => arr.1[j.val]'(by rw [arr.2]; exact j.isLt) * x j) =
      finSum LP.numRows (fun i => y i * rowActivity LP x i) := by
  unfold computeColDotsArr
  have h := addRowsArr_dot_eq LP y x (Array.replicate LP.numVars (0 : Rat))
    Array.size_replicate LP.numRows (Nat.le_refl _)
  have hinit :
      finSum LP.numVars (fun j =>
        (Array.replicate LP.numVars (0 : Rat))[j.val]'(by rw [Array.size_replicate]; exact j.isLt) * x j) = 0 := by
    have hz :
        finSum LP.numVars (fun j =>
          (Array.replicate LP.numVars (0 : Rat))[j.val]'(by rw [Array.size_replicate]; exact j.isLt) * x j) =
        finSum LP.numVars (fun _ => (0 : Rat)) := by
      apply finSum_congr
      intro j
      rw [Array.getElem_replicate, Rat.zero_mul]
    rw [hz, finSum_zero]
  dsimp only
  dsimp only at h
  rw [h, hinit, Rat.zero_add]

/-- Lower bound contribution of a single reduced-cost term `r * x` over `[lb, ub]`,
returning `none` if a required finite bound is missing. -/
def rcTermBound (r : Rat) (lb ub : Option Rat) : Option Rat :=
  if 0 < r then
    match lb with
    | some l => some (r * l)
    | none => none
  else if r < 0 then
    match ub with
    | some u => some (r * u)
    | none => none
  else
    some 0

theorem rcTermBound_le {r : Rat} {lb ub : Option Rat} {x b : Rat}
    (hx : SatisfiesVarBounds lb ub x) (hb : rcTermBound r lb ub = some b) :
    b ≤ r * x := by
  unfold rcTermBound at hb
  by_cases hpos : 0 < r
  · simp only [hpos, ↓reduceIte] at hb
    cases hlb : lb with
    | none =>
      rw [hlb] at hb
      contradiction
    | some l =>
      rw [hlb] at hb
      injection hb with heq
      rw [← heq]
      have hle : l ≤ x := by
        have hx1 := hx.1
        rw [hlb] at hx1
        exact hx1
      exact Rat.mul_le_mul_of_nonneg_left hle (Rat.le_of_lt hpos)
  · simp only [hpos, ↓reduceIte] at hb
    by_cases hneg : r < 0
    · simp only [hneg, ↓reduceIte] at hb
      cases hub : ub with
      | none =>
        rw [hub] at hb
        contradiction
      | some u =>
        rw [hub] at hb
        injection hb with heq
        rw [← heq]
        have hle : x ≤ u := by
          have hx2 := hx.2
          rw [hub] at hx2
          exact hx2
        exact rat_mul_le_mul_of_nonpos_left hle (Rat.le_of_lt hneg)
    · simp only [hneg, ↓reduceIte] at hb
      injection hb with heq
      have hr0 : r = 0 :=
        Rat.le_antisymm (Rat.not_lt.mp hpos) (Rat.not_lt.mp hneg)
      rw [← heq, hr0, Rat.zero_mul]
      exact Rat.le_refl

/-- Sums `Fin n → Option Rat`, returning `none` if any entry is `none`. -/
def sumOptFin : (n : Nat) → (Fin n → Option Rat) → Option Rat
  | 0, _ => some 0
  | n + 1, f =>
    match sumOptFin n (fun i => f i.castSucc), f (Fin.last n) with
    | some a, some b => some (a + b)
    | _, _ => none

theorem sumOptFin_le_finSum {n : Nat} {f : Fin n → Option Rat} {g : Fin n → Rat} {s : Rat}
    (hfg : ∀ j b, f j = some b → b ≤ g j)
    (hs : sumOptFin n f = some s) :
    s ≤ finSum n g := by
  induction n generalizing s with
  | zero =>
    simp only [sumOptFin] at hs
    injection hs with heq
    rw [← heq, finSum_zero_n]
    exact Rat.le_refl
  | succ n ih =>
    simp only [sumOptFin] at hs
    cases hleft : sumOptFin n (fun i => f i.castSucc) with
    | none =>
      rw [hleft] at hs
      contradiction
    | some a =>
      rw [hleft] at hs
      cases hright : f (Fin.last n) with
      | none =>
        rw [hright] at hs
        contradiction
      | some b =>
        rw [hright] at hs
        injection hs with heq
        rw [← heq, finSum_succ]
        have ha := ih (fun i bi hfi => hfg i.castSucc bi hfi) hleft
        have hb := hfg (Fin.last n) b hright
        exact rat_add_le_add ha hb

theorem finSum_le_sumOptFin {n : Nat} {f : Fin n → Option Rat} {g : Fin n → Rat} {s : Rat}
    (hfg : ∀ j b, f j = some b → g j ≤ b)
    (hs : sumOptFin n f = some s) :
    finSum n g ≤ s := by
  induction n generalizing s with
  | zero =>
    simp only [sumOptFin] at hs
    injection hs with heq
    rw [← heq, finSum_zero_n]
    exact Rat.le_refl
  | succ n ih =>
    simp only [sumOptFin] at hs
    cases hleft : sumOptFin n (fun i => f i.castSucc) with
    | none =>
      rw [hleft] at hs
      contradiction
    | some a =>
      rw [hleft] at hs
      cases hright : f (Fin.last n) with
      | none =>
        rw [hright] at hs
        contradiction
      | some b =>
        rw [hright] at hs
        injection hs with heq
        rw [← heq, finSum_succ]
        have ha := ih (fun i bi hfi => hfg i.castSucc bi hfi) hleft
        have hb := hfg (Fin.last n) b hright
        exact rat_add_le_add ha hb

/-- Neumaier-Shcherbina safe dual bound over exact `Rat`.
Projects `y` onto the dual cone, computes exact rational reduced costs `r_j = c_j - (y'^T A)_j`,
and bounds each `r_j * x_j` using finite variable bounds. -/
def safeDualBound (LP : LPInstance) (y : Fin LP.numRows → Rat) : Option Rat :=
  let y' : Fin LP.numRows → Rat := fun i => projDual (LP.sense i) (y i)
  let waArr := computeColDotsArr LP y'
  let rcSumOpt := sumOptFin LP.numVars (fun j =>
    let waj := waArr.1[j.val]'(by rw [waArr.2]; exact j.isLt)
    let rj := LP.obj j - waj
    rcTermBound rj (LP.lb j) (LP.ub j))
  match rcSumOpt with
  | none => none
  | some rcSum =>
    let yDotB := finSum LP.numRows (fun i => y' i * LP.rhs i)
    some (LP.objConst + yDotB + rcSum)

/-- Soundness of Neumaier-Shcherbina safe dual bounding:
any returned bound is a valid lower bound on `LPObjective LP x` for all primal-feasible `x`. -/
theorem safeDualBound_sound {LP : LPInstance} {y : Fin LP.numRows → Rat}
    {lbVal : Rat} {x : Fin LP.numVars → Rat}
    (hbound : safeDualBound LP y = some lbVal)
    (hfeas : PrimalFeasible LP x) :
    lbVal ≤ LPObjective LP x := by
  unfold safeDualBound at hbound
  dsimp only at hbound
  let y' : Fin LP.numRows → Rat := fun i => projDual (LP.sense i) (y i)
  let waArr := computeColDotsArr LP y'
  cases hrc : sumOptFin LP.numVars (fun j =>
      rcTermBound (LP.obj j - waArr.1[j.val]'(by rw [waArr.2]; exact j.isLt)) (LP.lb j) (LP.ub j)) with
  | none =>
    rw [hrc] at hbound
    contradiction
  | some rcSum =>
    rw [hrc] at hbound
    injection hbound with heq
    rw [← heq]
    have hrcLe :
        rcSum ≤ finSum LP.numVars (fun j =>
          (LP.obj j - waArr.1[j.val]'(by rw [waArr.2]; exact j.isLt)) * x j) :=
      sumOptFin_le_finSum (fun j b hb => rcTermBound_le (hfeas.1 j) hb) hrc
    have hrcSplit :
        finSum LP.numVars (fun j =>
          (LP.obj j - waArr.1[j.val]'(by rw [waArr.2]; exact j.isLt)) * x j) =
        finSum LP.numVars (fun j => LP.obj j * x j) -
          finSum LP.numVars (fun j =>
            waArr.1[j.val]'(by rw [waArr.2]; exact j.isLt) * x j) := by
      have hcongr :
          finSum LP.numVars (fun j =>
            (LP.obj j - waArr.1[j.val]'(by rw [waArr.2]; exact j.isLt)) * x j) =
          finSum LP.numVars (fun j =>
            LP.obj j * x j - waArr.1[j.val]'(by rw [waArr.2]; exact j.isLt) * x j) :=
        finSum_congr (fun j => rat_sub_mul _ _ _)
      rw [hcongr, finSum_sub]
    have hdot := computeColDotsArr_dot_eq LP y' x
    have hrow :
        finSum LP.numRows (fun i => y' i * LP.rhs i) ≤
        finSum LP.numRows (fun i => y' i * rowActivity LP x i) :=
      finSum_le_sumOptFin_rows (LP := LP) (y := y) (x := x) hfeas.2
    have hrowWa :
        finSum LP.numRows (fun i => y' i * LP.rhs i) ≤
        finSum LP.numVars (fun j =>
          waArr.1[j.val]'(by rw [waArr.2]; exact j.isLt) * x j) := by
      rw [← hdot] at hrow
      exact hrow
    have hsum :
        finSum LP.numRows (fun i => y' i * LP.rhs i) + rcSum ≤
        finSum LP.numVars (fun j => LP.obj j * x j) := by
      have hadd := rat_add_le_add hrowWa hrcLe
      rw [hrcSplit] at hadd
      have hcancel :
          finSum LP.numVars (fun j => waArr.1[j.val]'(by rw [waArr.2]; exact j.isLt) * x j) +
            (finSum LP.numVars (fun j => LP.obj j * x j) -
              finSum LP.numVars (fun j => waArr.1[j.val]'(by rw [waArr.2]; exact j.isLt) * x j)) =
          finSum LP.numVars (fun j => LP.obj j * x j) := by
        rw [Rat.sub_eq_add_neg, Rat.add_comm (finSum LP.numVars (fun j => LP.obj j * x j)),
          ← Rat.add_assoc, Rat.add_neg_cancel, Rat.zero_add]
      rw [hcancel] at hadd
      exact hadd
    unfold LPObjective
    rw [Rat.add_assoc]
    exact Rat.add_le_add_left.mpr hsum
where
  finSum_le_sumOptFin_rows {LP : LPInstance} {y : Fin LP.numRows → Rat}
      {x : Fin LP.numVars → Rat}
      (hrows : ∀ i : Fin LP.numRows, SatisfiesRow (LP.sense i) (rowActivity LP x i) (LP.rhs i)) :
      finSum LP.numRows (fun i => projDual (LP.sense i) (y i) * LP.rhs i) ≤
      finSum LP.numRows (fun i => projDual (LP.sense i) (y i) * rowActivity LP x i) :=
    finSum_le_finSum (fun i => projDual_row_le (hrows i))

/-- Exact primal-dual optimality checker: verifies primal feasibility, valid safe dual bound,
and zero duality gap (`LPObjective LP x == lbVal`). -/
def checkLPOptimal (LP : LPInstance) (x : Fin LP.numVars → Rat)
    (y : Fin LP.numRows → Rat) : Bool :=
  checkPrimalFeasible LP x &&
  match safeDualBound LP y with
  | none => false
  | some lbVal => LPObjective LP x == lbVal

/-- Soundness of exact LP optimality verification:
`x` is primal-feasible and achieves the global minimum objective among all primal-feasible vectors. -/
theorem checkLPOptimal_sound {LP : LPInstance}
    {x : Fin LP.numVars → Rat} {y : Fin LP.numRows → Rat}
    (hok : checkLPOptimal LP x y = true) :
    PrimalFeasible LP x ∧
      ∀ x' : Fin LP.numVars → Rat, PrimalFeasible LP x' →
        LPObjective LP x ≤ LPObjective LP x' := by
  unfold checkLPOptimal at hok
  rw [Bool.and_eq_true] at hok
  have hfeas : PrimalFeasible LP x := checkPrimalFeasible_sound hok.1
  refine ⟨hfeas, ?_⟩
  cases hsb : safeDualBound LP y with
  | none =>
    have hgap := hok.2
    rw [hsb] at hgap
    cases hgap
  | some lbVal =>
    have hgap := hok.2
    rw [hsb, beq_iff_eq] at hgap
    intro x' hx'
    rw [hgap]
    exact safeDualBound_sound hsb hx'

/-- Upper bound contribution of a single `wa * x` term over `[lb, ub]`,
returning `none` if a required finite bound is missing. -/
def farkasTermUpper (wa : Rat) (lb ub : Option Rat) : Option Rat :=
  if 0 < wa then
    match ub with
    | some u => some (wa * u)
    | none => none
  else if wa < 0 then
    match lb with
    | some l => some (wa * l)
    | none => none
  else
    some 0

theorem farkasTermUpper_ge {wa : Rat} {lb ub : Option Rat} {x b : Rat}
    (hx : SatisfiesVarBounds lb ub x) (hb : farkasTermUpper wa lb ub = some b) :
    wa * x ≤ b := by
  unfold farkasTermUpper at hb
  by_cases hpos : 0 < wa
  · simp only [hpos, ↓reduceIte] at hb
    cases hub : ub with
    | none =>
      rw [hub] at hb
      contradiction
    | some u =>
      rw [hub] at hb
      injection hb with heq
      rw [← heq]
      have hle : x ≤ u := by
        have hx2 := hx.2
        rw [hub] at hx2
        exact hx2
      exact Rat.mul_le_mul_of_nonneg_left hle (Rat.le_of_lt hpos)
  · simp only [hpos, ↓reduceIte] at hb
    by_cases hneg : wa < 0
    · simp only [hneg, ↓reduceIte] at hb
      cases hlb : lb with
      | none =>
        rw [hlb] at hb
        contradiction
      | some l =>
        rw [hlb] at hb
        injection hb with heq
        rw [← heq]
        have hle : l ≤ x := by
          have hx1 := hx.1
          rw [hlb] at hx1
          exact hx1
        exact rat_mul_le_mul_of_nonpos_left hle (Rat.le_of_lt hneg)
    · simp only [hneg, ↓reduceIte] at hb
      injection hb with heq
      have hwa0 : wa = 0 :=
        Rat.le_antisymm (Rat.not_lt.mp hpos) (Rat.not_lt.mp hneg)
      rw [← heq, hwa0, Rat.zero_mul]
      exact Rat.le_refl

/-- Exact Farkas infeasibility checker.
Projects `w` onto the dual cone (`le` -> `w ≤ 0`, `ge` -> `w ≥ 0`, `eq` -> any),
computes `wa = w'^T A`, checks finite bounds matching the signs of `wa_j`, and
verifies `max_{x ∈ [lb, ub]} w'^T A x < w'^T b`. -/
def checkFarkas (LP : LPInstance) (w : Fin LP.numRows → Rat) : Bool :=
  let w' : Fin LP.numRows → Rat := fun i => projDual (LP.sense i) (w i)
  let waArr := computeColDotsArr LP w'
  let maxLHSOpt := sumOptFin LP.numVars (fun j =>
    let waj := waArr.1[j.val]'(by rw [waArr.2]; exact j.isLt)
    farkasTermUpper waj (LP.lb j) (LP.ub j))
  match maxLHSOpt with
  | none => false
  | some maxLHS =>
    let wDotB := finSum LP.numRows (fun i => w' i * LP.rhs i)
    decide (maxLHS < wDotB)

/-- Soundness of Farkas infeasibility verification:
a passing dual ray certifies that no primal-feasible solution exists. -/
theorem checkFarkas_sound {LP : LPInstance} {w : Fin LP.numRows → Rat}
    (hok : checkFarkas LP w = true) :
    ¬ ∃ x : Fin LP.numVars → Rat, PrimalFeasible LP x := by
  intro ⟨x, hfeas⟩
  unfold checkFarkas at hok
  dsimp only at hok
  let w' : Fin LP.numRows → Rat := fun i => projDual (LP.sense i) (w i)
  let waArr := computeColDotsArr LP w'
  cases hmax : sumOptFin LP.numVars (fun j =>
      farkasTermUpper (waArr.1[j.val]'(by rw [waArr.2]; exact j.isLt)) (LP.lb j) (LP.ub j)) with
  | none =>
    rw [hmax] at hok
    cases hok
  | some maxLHS =>
    rw [hmax, decide_eq_true_eq] at hok
    have hcolLe :
        finSum LP.numVars (fun j =>
          waArr.1[j.val]'(by rw [waArr.2]; exact j.isLt) * x j) ≤ maxLHS :=
      finSum_le_sumOptFin (fun j b hb => farkasTermUpper_ge (hfeas.1 j) hb) hmax
    have hdot := computeColDotsArr_dot_eq LP w' x
    have hrow :
        finSum LP.numRows (fun i => w' i * LP.rhs i) ≤
        finSum LP.numRows (fun i => w' i * rowActivity LP x i) :=
      finSum_le_finSum (fun i => projDual_row_le (hfeas.2 i))
    rw [← hdot] at hrow
    have hcontra := Rat.le_trans hrow hcolLe
    exact (Rat.not_lt.mpr hcontra) hok

/-- Exact direct bound-conflict infeasibility checker:
verifies that column `col < LP.numVars` has finite bounds `lb col = some l` and
`ub col = some u` with `u < l` (`lb > ub`). -/
def checkBoundConflict (LP : LPInstance) (col : Nat) : Bool :=
  if hcol : col < LP.numVars then
    let j : Fin LP.numVars := ⟨col, hcol⟩
    match LP.lb j, LP.ub j with
    | some l, some u => decide (u < l)
    | _, _ => false
  else
    false

/-- Soundness of direct bound-conflict infeasibility verification:
a column with finite `lb > ub` certifies that no primal-feasible solution exists. -/
theorem checkBoundConflict_sound {LP : LPInstance} {col : Nat}
    (hok : checkBoundConflict LP col = true) :
    ¬ ∃ x : Fin LP.numVars → Rat, PrimalFeasible LP x := by
  intro ⟨x, hfeas⟩
  unfold checkBoundConflict at hok
  split at hok
  · next hcol =>
    let j : Fin LP.numVars := ⟨col, hcol⟩
    have hj := hfeas.1 j
    unfold SatisfiesVarBounds at hj
    cases hlb : LP.lb j with
    | none => simp [j, hlb] at hok
    | some l =>
      cases hub : LP.ub j with
      | none => simp [j, hlb, hub] at hok
      | some u =>
        simp [j, hlb, hub] at hok
        rw [hlb, hub] at hj
        have hlu : l ≤ u := Rat.le_trans hj.1 hj.2
        exact (Rat.not_lt.mpr hlu) hok
  · cases hok

end GoblinFormal.Track3
