/-
GoblinFormal.Track3B: Verified LP/MILP unboundedness certificates (Meyer's theorem),
Euclidean GCD integer row-coefficient division, and LP presolve / dual postsolve
reconstruction over exact `Rat`.
-/

import GoblinFormal.Common.Basic
import GoblinFormal.Common.Json
import GoblinFormal.Track3

namespace GoblinFormal.Track3B

open GoblinFormal
open GoblinFormal.Track3

/-! ### Section 0: Algebraic, FinSum & Sparse-Row Helpers -/

private theorem rat_mul_nonpos_of_nonneg_of_nonpos {a b : Rat} (ha : 0 ≤ a) (hb : b ≤ 0) :
    a * b ≤ 0 := by
  have hm := Rat.mul_nonneg ha (rat_neg_nonneg hb)
  rw [Rat.mul_neg] at hm
  simpa using Rat.neg_le_neg_iff.mpr hm

private theorem rat_mul_nonpos_of_nonpos_of_nonneg {a b : Rat} (ha : a ≤ 0) (hb : 0 ≤ b) :
    a * b ≤ 0 := by
  rw [Rat.mul_comm]
  exact rat_mul_nonpos_of_nonneg_of_nonpos hb ha

private theorem rat_mul_nonneg_of_nonpos_of_nonpos {a b : Rat} (ha : a ≤ 0) (hb : b ≤ 0) :
    0 ≤ a * b := by
  have hm := Rat.mul_nonneg (rat_neg_nonneg ha) (rat_neg_nonneg hb)
  rwa [Rat.neg_mul, Rat.mul_neg, Rat.neg_neg] at hm

private theorem sumOptFin_congr {n : Nat} {f g : Fin n → Option Rat} (h : ∀ j, f j = g j) :
    sumOptFin n f = sumOptFin n g := by
  induction n with
  | zero => rfl
  | succ n ih =>
    simp only [sumOptFin, ih (fun i => h i.castSucc), h (Fin.last n)]

private theorem sumOptFin_split_at {n : Nat} (f : Fin n → Option Rat) (k : Fin n)
    (vk : Rat) (hfk : f k = some vk) :
    sumOptFin n f =
      match sumOptFin n (fun j => if j = k then some 0 else f j) with
      | none => none
      | some s => some (s + vk) := by
  induction n with
  | zero => exact Fin.elim0 k
  | succ n ih =>
    simp only [sumOptFin]
    by_cases hk : k = Fin.last n
    · subst hk
      have hleft :
          sumOptFin n (fun i : Fin n => if i.castSucc = Fin.last n then some 0 else f i.castSucc) =
          sumOptFin n (fun i : Fin n => f i.castSucc) := by
        apply sumOptFin_congr
        intro i
        have hne : i.castSucc ≠ Fin.last n := by
          intro heq
          have hv := congrArg Fin.val heq
          simp only [Fin.val_castSucc, Fin.val_last] at hv
          omega
        simp [hne]
      rw [hleft, hfk]
      cases sumOptFin n (fun i => f i.castSucc) with
      | none => simp
      | some a => simp [Rat.add_zero]
    · have hklt : k.val < n := by
        have hklt' := k.isLt
        have hne : k.val ≠ n := fun hv => hk (Fin.ext hv)
        omega
      let k0 : Fin n := ⟨k.val, hklt⟩
      have hk0 : k = k0.castSucc := Fin.ext rfl
      have hfk0 : f k0.castSucc = some vk := by rwa [← hk0]
      have hih := ih (fun i => f i.castSucc) k0 hfk0
      have hleft :
          sumOptFin n (fun i : Fin n => if i.castSucc = k then some 0 else f i.castSucc) =
          sumOptFin n (fun i : Fin n => if i = k0 then some 0 else f i.castSucc) := by
        apply sumOptFin_congr
        intro i
        have hiff : i.castSucc = k ↔ i = k0 := by
          rw [hk0]
          exact ⟨fun h => Fin.ext (by simpa [Fin.val_castSucc] using congrArg Fin.val h),
                 fun h => h ▸ rfl⟩
        simp [hiff]
      have hlast : (if Fin.last n = k then some (0 : Rat) else f (Fin.last n)) = f (Fin.last n) := by
        simp [Ne.symm hk]
      rw [hih, hleft, hlast]
      cases sumOptFin n (fun i => if i = k0 then some 0 else f i.castSucc) with
      | none => simp
      | some a =>
        cases f (Fin.last n) with
        | none => simp
        | some b =>
          simp only
          rw [Rat.add_assoc, Rat.add_comm vk b, ← Rat.add_assoc]

private def allFinAux3B (n : Nat) (p : Fin n → Bool) (i : Nat) (_hi : i ≤ n) : Bool :=
  if h : i < n then
    if p ⟨i, h⟩ then allFinAux3B n p (i + 1) h else false
  else
    true
termination_by n - i

private theorem allFinAux3B_iff (n : Nat) (p : Fin n → Bool) :
    ∀ (k i : Nat) (hi : i ≤ n), n - i = k →
      (allFinAux3B n p i hi = true ↔ ∀ j : Fin n, i ≤ j.val → p j = true) := by
  intro k
  induction k with
  | zero =>
    intro i _ _
    have hge : ¬i < n := by omega
    unfold allFinAux3B
    simp only [hge, ↓reduceDIte, true_iff]
    intro j hj
    have hjlt := j.isLt
    omega
  | succ k ih =>
    intro i _ hdiff
    have hlt : i < n := by omega
    unfold allFinAux3B
    simp only [hlt, ↓reduceDIte]
    cases hp : p ⟨i, hlt⟩
    · simp only [Bool.false_eq_true, ↓reduceIte, false_iff]
      intro hforall
      have hbad := hforall ⟨i, hlt⟩ (Nat.le_refl i)
      rw [hp] at hbad
      cases hbad
    · simp only [↓reduceIte]
      rw [ih (i + 1) hlt (by omega)]
      constructor
      · intro hrest j hj
        by_cases heq : i = j.val
        · have hjEq : j = ⟨i, hlt⟩ := Fin.ext heq.symm
          rw [hjEq, hp]
        · exact hrest j (by omega)
      · intro hforall j hj
        exact hforall j (by omega)

/-- Total universal quantifier over `Fin n`. -/
def allFin3B (n : Nat) (p : Fin n → Bool) : Bool :=
  allFinAux3B n p 0 (Nat.zero_le n)

theorem allFin3B_iff {n : Nat} {p : Fin n → Bool} :
    allFin3B n p = true ↔ ∀ j : Fin n, p j = true := by
  unfold allFin3B
  rw [allFinAux3B_iff n p n 0 (Nat.zero_le n) rfl]
  exact ⟨fun h j => h j (Nat.zero_le j.val), fun h j _ => h j⟩

private def anyFinAux3B (n : Nat) (p : Fin n → Bool) (i : Nat) (_hi : i ≤ n) : Bool :=
  if h : i < n then
    if p ⟨i, h⟩ then true else anyFinAux3B n p (i + 1) h
  else
    false
termination_by n - i

private theorem anyFinAux3B_iff (n : Nat) (p : Fin n → Bool) :
    ∀ (k i : Nat) (hi : i ≤ n), n - i = k →
      (anyFinAux3B n p i hi = true ↔ ∃ j : Fin n, i ≤ j.val ∧ p j = true) := by
  intro k
  induction k with
  | zero =>
    intro i _ _
    have hge : ¬i < n := by omega
    unfold anyFinAux3B
    simp only [hge, ↓reduceDIte, Bool.false_eq_true, false_iff]
    rintro ⟨j, hj, _⟩
    have hjlt := j.isLt
    omega
  | succ k ih =>
    intro i _ hdiff
    have hlt : i < n := by omega
    unfold anyFinAux3B
    simp only [hlt, ↓reduceDIte]
    cases hp : p ⟨i, hlt⟩
    · simp only [Bool.false_eq_true, ↓reduceIte]
      rw [ih (i + 1) hlt (by omega)]
      constructor
      · rintro ⟨j, hj, hpj⟩
        exact ⟨j, by omega, hpj⟩
      · rintro ⟨j, hj, hpj⟩
        by_cases heq : i = j.val
        · have hjEq : j = ⟨i, hlt⟩ := Fin.ext heq.symm
          rw [hjEq, hp] at hpj
          cases hpj
        · exact ⟨j, by omega, hpj⟩
    · simp only [↓reduceIte, true_iff]
      exact ⟨⟨i, hlt⟩, Nat.le_refl i, hp⟩

/-- Total existential quantifier over `Fin n`. -/
def anyFin3B (n : Nat) (p : Fin n → Bool) : Bool :=
  anyFinAux3B n p 0 (Nat.zero_le n)

theorem anyFin3B_iff {n : Nat} {p : Fin n → Bool} :
    anyFin3B n p = true ↔ ∃ j : Fin n, p j = true := by
  unfold anyFin3B
  rw [anyFinAux3B_iff n p n 0 (Nat.zero_le n) rfl]
  exact ⟨fun ⟨j, _, hp⟩ => ⟨j, hp⟩, fun ⟨j, hp⟩ => ⟨j, Nat.zero_le j.val, hp⟩⟩

theorem checkVarBounds_iff {lb ub : Option Rat} {xj : Rat} :
    checkVarBounds lb ub xj = true ↔ SatisfiesVarBounds lb ub xj := by
  unfold checkVarBounds SatisfiesVarBounds
  rw [Bool.and_eq_true]
  constructor
  · intro ⟨h1, h2⟩
    refine ⟨?_, ?_⟩
    · cases lb with
      | none => trivial
      | some l => exact decide_eq_true_eq.mp h1
    · cases ub with
      | none => trivial
      | some u => exact decide_eq_true_eq.mp h2
  · intro ⟨h1, h2⟩
    refine ⟨?_, ?_⟩
    · cases lb with
      | none => rfl
      | some l => exact decide_eq_true_eq.mpr h1
    · cases ub with
      | none => rfl
      | some u => exact decide_eq_true_eq.mpr h2

theorem checkRowConstraint_iff {sense : ConstraintSense} {act rhs : Rat} :
    checkRowConstraint sense act rhs = true ↔ SatisfiesRow sense act rhs := by
  cases sense with
  | le => simp [checkRowConstraint, SatisfiesRow]
  | ge => simp [checkRowConstraint, SatisfiesRow]
  | eq => simp [checkRowConstraint, SatisfiesRow, beq_iff_eq]

/-- Standard basis vector `e_k` over `Fin n → Rat`. -/
def unitVec (n : Nat) (k : Fin n) : Fin n → Rat :=
  fun j => if j = k then 1 else 0

/-- Net sparse coefficient of column `k` in constraint row `i` of `LP`. -/
def rowCoeff (LP : LPInstance) (i : Fin LP.numRows) (k : Fin LP.numVars) : Rat :=
  rowActivity LP (unitVec LP.numVars k) i

theorem evalTerms_eq_finSum {n : Nat} (x : Fin n → Rat) (ts : List (SparseTerm n)) :
    evalTerms x ts = finSum n (fun j => evalTerms (unitVec n j) ts * x j) := by
  induction ts with
  | nil =>
    simp only [evalTerms, Rat.zero_mul, finSum_zero]
  | cons t rest ih =>
    simp only [evalTerms]
    have hcongr :
        ∀ j : Fin n,
          (t.coeff * unitVec n j t.col + evalTerms (unitVec n j) rest) * x j =
            (if j = t.col then t.coeff * x t.col else 0) +
              evalTerms (unitVec n j) rest * x j := by
      intro j
      rw [Rat.add_mul]
      unfold unitVec
      by_cases hj : j = t.col
      · subst hj
        simp [Rat.mul_one]
      · have hne : t.col ≠ j := fun h => hj h.symm
        simp [hj, hne, Rat.mul_zero, Rat.zero_mul]
    rw [finSum_congr hcongr, finSum_add, finSum_single, ih]

theorem rowActivity_eq_finSum (LP : LPInstance) (x : Fin LP.numVars → Rat) (i : Fin LP.numRows) :
    rowActivity LP x i = finSum LP.numVars (fun j => rowCoeff LP i j * x j) :=
  evalTerms_eq_finSum x (LP.rows i)

theorem computeColDotsArr_get_eq (LP : LPInstance) (y : Fin LP.numRows → Rat)
    (k : Fin LP.numVars) :
    let arr := computeColDotsArr LP y
    arr.1[k.val]'(by rw [arr.2]; exact k.isLt) =
      finSum LP.numRows (fun i => y i * rowCoeff LP i k) := by
  dsimp only
  let arr := computeColDotsArr LP y
  have hdot := computeColDotsArr_dot_eq LP y (unitVec LP.numVars k)
  dsimp only at hdot
  have hlhs :
      finSum LP.numVars (fun j => arr.1[j.val]'(by rw [arr.2]; exact j.isLt) * unitVec LP.numVars k j) =
        arr.1[k.val]'(by rw [arr.2]; exact k.isLt) := by
    have hc :
        ∀ j : Fin LP.numVars,
          arr.1[j.val]'(by rw [arr.2]; exact j.isLt) * unitVec LP.numVars k j =
            if j = k then arr.1[k.val]'(by rw [arr.2]; exact k.isLt) else 0 := by
      intro j
      unfold unitVec
      by_cases hj : j = k
      · subst hj; simp [Rat.mul_one]
      · simp [hj, Rat.mul_zero]
    rw [finSum_congr hc, finSum_single]
  rw [← hlhs, hdot]
  rfl

/-! ### Section 1: LP Unboundedness Certificate (`checkLPUnbounded`) -/

/-- Point `x0 + t • d` along ray `d` from anchor `x0`. -/
def rayPoint {n : Nat} (x0 d : Fin n → Rat) (t : Rat) : Fin n → Rat :=
  fun j => x0 j + t * d j

/-- Linear objective slope `c^T d` along ray `d` (excluding `objConst`). -/
def rayObjSlope (LP : LPInstance) (d : Fin LP.numVars → Rat) : Rat :=
  finSum LP.numVars (fun j => LP.obj j * d j)

/-- Propositional variable-bound compatibility for a recession ray entry `dj`. -/
def SatisfiesRayVarBound (lb ub : Option Rat) (dj : Rat) : Prop :=
  (0 < dj → ub = none) ∧ (dj < 0 → lb = none)

/-- Executable checker for variable-bound compatibility of a recession ray entry `dj`. -/
def checkRayVarBound (lb ub : Option Rat) (dj : Rat) : Bool :=
  (if 0 < dj then ub.isNone else true) &&
  (if dj < 0 then lb.isNone else true)

theorem checkRayVarBound_iff {lb ub : Option Rat} {dj : Rat} :
    checkRayVarBound lb ub dj = true ↔ SatisfiesRayVarBound lb ub dj := by
  unfold checkRayVarBound SatisfiesRayVarBound
  rw [Bool.and_eq_true]
  constructor
  · intro ⟨h1, h2⟩
    refine ⟨?_, ?_⟩
    · intro hpos
      simp only [hpos, ↓reduceIte, Option.isNone_iff_eq_none] at h1
      exact h1
    · intro hneg
      simp only [hneg, ↓reduceIte, Option.isNone_iff_eq_none] at h2
      exact h2
  · intro ⟨h1, h2⟩
    refine ⟨?_, ?_⟩
    · by_cases hpos : 0 < dj
      · simp [hpos, h1 hpos]
      · simp [hpos]
    · by_cases hneg : dj < 0
      · simp [hneg, h2 hneg]
      · simp [hneg]

/-- Propositional homogeneous row condition `A_i d ⋄ 0` for a recession ray. -/
def SatisfiesRayRow (sense : ConstraintSense) (act : Rat) : Prop :=
  SatisfiesRow sense act 0

/-- Executable checker for homogeneous row condition `A_i d ⋄ 0`. -/
def checkRayRow (sense : ConstraintSense) (act : Rat) : Bool :=
  checkRowConstraint sense act 0

/-- Propositional LP recession ray specification (`c^T d < 0`). -/
def IsRecessionRay (LP : LPInstance) (d : Fin LP.numVars → Rat) : Prop :=
  (∀ j : Fin LP.numVars, SatisfiesRayVarBound (LP.lb j) (LP.ub j) (d j)) ∧
  (∀ i : Fin LP.numRows, SatisfiesRayRow (LP.sense i) (rowActivity LP d i)) ∧
  rayObjSlope LP d < 0

/-- Executable LP recession ray checker. -/
def checkRecessionRay (LP : LPInstance) (d : Fin LP.numVars → Rat) : Bool :=
  allFin3B LP.numVars (fun j => checkRayVarBound (LP.lb j) (LP.ub j) (d j)) &&
  allFin3B LP.numRows (fun i => checkRayRow (LP.sense i) (rowActivity LP d i)) &&
  decide (rayObjSlope LP d < 0)

theorem checkRecessionRay_sound {LP : LPInstance} {d : Fin LP.numVars → Rat}
    (hok : checkRecessionRay LP d = true) : IsRecessionRay LP d := by
  unfold checkRecessionRay at hok
  rw [Bool.and_eq_true, Bool.and_eq_true, allFin3B_iff, allFin3B_iff, decide_eq_true_eq] at hok
  rcases hok with ⟨⟨hb, hr⟩, hobj⟩
  exact ⟨fun j => checkRayVarBound_iff.mp (hb j),
         fun i => checkRowConstraint_iff.mp (hr i),
         hobj⟩

/-- Executable LP unboundedness certificate checker (`(x0, d)`). -/
def checkLPUnbounded (LP : LPInstance) (x0 d : Fin LP.numVars → Rat) : Bool :=
  checkPrimalFeasible LP x0 && checkRecessionRay LP d

theorem evalTerms_rayPoint {n : Nat} (x0 d : Fin n → Rat) (t : Rat)
    (ts : List (SparseTerm n)) :
    evalTerms (rayPoint x0 d t) ts = evalTerms x0 ts + t * evalTerms d ts := by
  induction ts with
  | nil =>
    simp [evalTerms, Rat.mul_zero, Rat.add_zero]
  | cons hd tl ih =>
    simp only [evalTerms, rayPoint, ih]
    have hmul : hd.coeff * (t * d hd.col) = t * (hd.coeff * d hd.col) := by
      rw [← Rat.mul_assoc, Rat.mul_comm hd.coeff t, Rat.mul_assoc]
    rw [Rat.mul_add, hmul, Rat.mul_add]
    rw [Rat.add_assoc, ← Rat.add_assoc (t * (hd.coeff * d hd.col)),
        Rat.add_comm (t * (hd.coeff * d hd.col)) (evalTerms x0 tl),
        Rat.add_assoc, ← Rat.add_assoc]

theorem rowActivity_rayPoint (LP : LPInstance) (x0 d : Fin LP.numVars → Rat)
    (t : Rat) (i : Fin LP.numRows) :
    rowActivity LP (rayPoint x0 d t) i = rowActivity LP x0 i + t * rowActivity LP d i :=
  evalTerms_rayPoint x0 d t (LP.rows i)

theorem LPObjective_rayPoint (LP : LPInstance) (x0 d : Fin LP.numVars → Rat) (t : Rat) :
    LPObjective LP (rayPoint x0 d t) = LPObjective LP x0 + t * rayObjSlope LP d := by
  unfold LPObjective rayPoint rayObjSlope
  have hcongr :
      ∀ j : Fin LP.numVars,
        LP.obj j * (x0 j + t * d j) = LP.obj j * x0 j + t * (LP.obj j * d j) := by
    intro j
    rw [Rat.mul_add, ← Rat.mul_assoc, Rat.mul_comm (LP.obj j) t, Rat.mul_assoc]
  rw [finSum_congr hcongr, finSum_add, finSum_const_mul, ← Rat.add_assoc]

/-- Every non-negative step `0 ≤ t` along a valid recession ray preserves primal feasibility. -/
theorem rayPoint_primalFeasible {LP : LPInstance} {x0 d : Fin LP.numVars → Rat} {t : Rat}
    (hfeas : PrimalFeasible LP x0) (hray : IsRecessionRay LP d) (ht : 0 ≤ t) :
    PrimalFeasible LP (rayPoint x0 d t) := by
  refine ⟨?_, ?_⟩
  · intro j
    have hx := hfeas.1 j
    have hd := hray.1 j
    unfold SatisfiesVarBounds rayPoint
    refine ⟨?_, ?_⟩
    · cases hlb : LP.lb j with
      | none => trivial
      | some l =>
        have hlx : l ≤ x0 j := by
          have hx1 := hx.1
          rwa [hlb] at hx1
        have hdj_nonneg : 0 ≤ d j := by
          apply Rat.not_lt.mp
          intro hneg
          have hnone := hd.2 hneg
          rw [hlb] at hnone
          cases hnone
        have hprod : 0 ≤ t * d j := Rat.mul_nonneg ht hdj_nonneg
        have hsum := rat_add_le_add hlx hprod
        rwa [Rat.add_zero] at hsum
    · cases hub : LP.ub j with
      | none => trivial
      | some u =>
        have hux : x0 j ≤ u := by
          have hx2 := hx.2
          rwa [hub] at hx2
        have hdj_nonpos : d j ≤ 0 := by
          apply Rat.not_lt.mp
          intro hpos
          have hnone := hd.1 hpos
          rw [hub] at hnone
          cases hnone
        have hprod : t * d j ≤ 0 := rat_mul_nonpos_of_nonneg_of_nonpos ht hdj_nonpos
        have hsum := rat_add_le_add hux hprod
        rwa [Rat.add_zero] at hsum
  · intro i
    rw [rowActivity_rayPoint]
    have hri := hfeas.2 i
    have hdi := hray.2.1 i
    unfold SatisfiesRayRow at hdi
    cases hs : LP.sense i with
    | le =>
      rw [hs] at hri hdi
      unfold SatisfiesRow at hri hdi ⊢
      have hprod : t * rowActivity LP d i ≤ 0 :=
        rat_mul_nonpos_of_nonneg_of_nonpos ht hdi
      have hsum := rat_add_le_add hri hprod
      rwa [Rat.add_zero] at hsum
    | ge =>
      rw [hs] at hri hdi
      unfold SatisfiesRow at hri hdi ⊢
      have hprod : 0 ≤ t * rowActivity LP d i := Rat.mul_nonneg ht hdi
      have hsum := rat_add_le_add hri hprod
      rwa [Rat.add_zero] at hsum
    | eq =>
      rw [hs] at hri hdi
      unfold SatisfiesRow at hri hdi ⊢
      rw [hri, hdi, Rat.mul_zero, Rat.add_zero]

/-- Soundness of LP unboundedness certificates: for every target threshold `M : Rat`,
there exists a primal-feasible vector `x` with `LPObjective LP x ≤ M`. -/
theorem checkLPUnbounded_sound {LP : LPInstance} {x0 d : Fin LP.numVars → Rat}
    (hok : checkLPUnbounded LP x0 d = true) :
    PrimalFeasible LP x0 ∧
    IsRecessionRay LP d ∧
    ∀ M : Rat, ∃ x : Fin LP.numVars → Rat, PrimalFeasible LP x ∧ LPObjective LP x ≤ M := by
  unfold checkLPUnbounded at hok
  rw [Bool.and_eq_true] at hok
  have hfeas : PrimalFeasible LP x0 := checkPrimalFeasible_sound hok.1
  have hray : IsRecessionRay LP d := checkRecessionRay_sound hok.2
  refine ⟨hfeas, hray, ?_⟩
  intro M
  let obj0 := LPObjective LP x0
  let s := rayObjSlope LP d
  have hs : s < 0 := hray.2.2
  rcases Rat.le_total (a := obj0) (b := M) with hle | hge
  · refine ⟨rayPoint x0 d 0, rayPoint_primalFeasible hfeas hray (Rat.le_refl), ?_⟩
    rw [LPObjective_rayPoint, Rat.zero_mul, Rat.add_zero]
    exact hle
  · have hns_pos : 0 < -s := by
      have h := Rat.neg_lt_neg_iff.mpr hs
      simpa using h
    have hdiff_nonneg : 0 ≤ obj0 - M := (Rat.le_iff_sub_nonneg M obj0).mp hge
    let t : Rat := (obj0 - M) / (-s)
    have ht_nonneg : 0 ≤ t := by
      unfold t
      rw [Rat.div_def]
      exact Rat.mul_nonneg hdiff_nonneg (Rat.le_of_lt (Rat.inv_pos.mpr hns_pos))
    refine ⟨rayPoint x0 d t, rayPoint_primalFeasible hfeas hray ht_nonneg, ?_⟩
    rw [LPObjective_rayPoint]
    have hts : t * s = M - obj0 := by
      have hs_eq : s = -(-s) := (Rat.neg_neg s).symm
      calc
        t * s = ((obj0 - M) / (-s)) * (-(-s)) := by rw [← hs_eq]
        _ = -(((obj0 - M) / (-s)) * (-s)) := Rat.mul_neg _ _
        _ = -(obj0 - M) := by rw [Rat.div_mul_cancel (Rat.ne_of_gt hns_pos)]
        _ = M - obj0 := Rat.neg_sub obj0 M
    change obj0 + t * s ≤ M
    rw [hts, Rat.add_comm obj0 (M - obj0), Rat.sub_add_cancel]
    exact Rat.le_refl

/-! ### Section 2: MILP Unboundedness Certificate via Meyer's Theorem (`checkMILPUnbounded`) -/

/-- Integrality predicate on variables marked `isInt j = true`. -/
def SatisfiesIntegrality {n : Nat} (isInt : Fin n → Bool) (x : Fin n → Rat) : Prop :=
  ∀ j : Fin n, isInt j = true → (x j).den = 1

/-- Executable integrality checker on `isInt`. -/
def checkIntegrality {n : Nat} (isInt : Fin n → Bool) (x : Fin n → Rat) : Bool :=
  allFin3B n (fun j => if isInt j then decide ((x j).den = 1) else true)

theorem checkIntegrality_sound {n : Nat} {isInt : Fin n → Bool} {x : Fin n → Rat}
    (hok : checkIntegrality isInt x = true) : SatisfiesIntegrality isInt x := by
  unfold checkIntegrality at hok
  rw [allFin3B_iff] at hok
  intro j hj
  have h := hok j
  simp only [hj, ↓reduceIte, decide_eq_true_eq] at h
  exact h

/-- Executable MILP unboundedness certificate checker (integer-feasible anchor `x0`
plus integer-on-`isInt` recession ray `d`). -/
def checkMILPUnbounded (LP : LPInstance) (isInt : Fin LP.numVars → Bool)
    (x0 d : Fin LP.numVars → Rat) : Bool :=
  checkLPUnbounded LP x0 d && checkIntegrality isInt x0 && checkIntegrality isInt d

/-- Stepping by any `k : Nat` along an integer-on-`isInt` ray preserves integrality. -/
theorem rayPoint_nat_integrality {n : Nat} {isInt : Fin n → Bool} {x0 d : Fin n → Rat}
    (hx0 : SatisfiesIntegrality isInt x0) (hd : SatisfiesIntegrality isInt d) (k : Nat) :
    SatisfiesIntegrality isInt (rayPoint x0 d (k : Rat)) := by
  intro j hj
  unfold rayPoint
  have hx := rat_eq_num_of_den_one (hx0 j hj)
  have hdr := rat_eq_num_of_den_one (hd j hj)
  have hk : (k : Rat) = ((k : Int) : Rat) := rfl
  rw [hx, hdr, hk, ← Rat.intCast_mul, ← Rat.intCast_add]
  rfl

/-- Soundness of MILP unboundedness certificates (Meyer's theorem witness family):
every natural-number step `x_k = x0 + k • d` is primal-feasible and integer-feasible on `isInt`,
and for every `M : Rat` there exists `k : Nat` with `LPObjective LP x_k ≤ M`. -/
theorem checkMILPUnbounded_sound {LP : LPInstance} {isInt : Fin LP.numVars → Bool}
    {x0 d : Fin LP.numVars → Rat}
    (hok : checkMILPUnbounded LP isInt x0 d = true) :
    (∀ k : Nat,
      PrimalFeasible LP (rayPoint x0 d (k : Rat)) ∧
      SatisfiesIntegrality isInt (rayPoint x0 d (k : Rat))) ∧
    (∀ M : Rat, ∃ k : Nat,
      PrimalFeasible LP (rayPoint x0 d (k : Rat)) ∧
      SatisfiesIntegrality isInt (rayPoint x0 d (k : Rat)) ∧
      LPObjective LP (rayPoint x0 d (k : Rat)) ≤ M) := by
  unfold checkMILPUnbounded at hok
  rw [Bool.and_eq_true, Bool.and_eq_true] at hok
  rcases hok with ⟨⟨hlp, hxInt⟩, hdInt⟩
  rcases checkLPUnbounded_sound hlp with ⟨hfeas, hray, _⟩
  have hint0 : SatisfiesIntegrality isInt x0 := checkIntegrality_sound hxInt
  have hintd : SatisfiesIntegrality isInt d := checkIntegrality_sound hdInt
  have hstep : ∀ k : Nat,
      PrimalFeasible LP (rayPoint x0 d (k : Rat)) ∧
      SatisfiesIntegrality isInt (rayPoint x0 d (k : Rat)) := by
    intro k
    have hk_nonneg : (0 : Rat) ≤ (k : Rat) := Rat.intCast_nonneg.mpr (Int.natCast_nonneg k)
    exact ⟨rayPoint_primalFeasible hfeas hray hk_nonneg, rayPoint_nat_integrality hint0 hintd k⟩
  refine ⟨hstep, ?_⟩
  intro M
  let obj0 := LPObjective LP x0
  let s := rayObjSlope LP d
  have hs : s < 0 := hray.2.2
  rcases Rat.le_total (a := obj0) (b := M) with hle | hge
  · refine ⟨0, (hstep 0).1, (hstep 0).2, ?_⟩
    rw [LPObjective_rayPoint]
    simpa [Rat.zero_mul, Rat.add_zero] using hle
  · have hns_pos : 0 < -s := by
      have h := Rat.neg_lt_neg_iff.mpr hs
      simpa using h
    have hdiff_nonneg : 0 ≤ obj0 - M := (Rat.le_iff_sub_nonneg M obj0).mp hge
    let t : Rat := (obj0 - M) / (-s)
    have ht_nonneg : 0 ≤ t := by
      unfold t
      rw [Rat.div_def]
      exact Rat.mul_nonneg hdiff_nonneg (Rat.le_of_lt (Rat.inv_pos.mpr hns_pos))
    let z : Int := t.ceil
    have htz : t ≤ (z : Rat) := Rat.le_ceil
    have hz_rat_nonneg : (0 : Rat) ≤ (z : Rat) := Rat.le_trans ht_nonneg htz
    have hz_nonneg : 0 ≤ z := Rat.intCast_nonneg.mp hz_rat_nonneg
    let k : Nat := z.toNat
    have hk_eq : (k : Rat) = (z : Rat) := by
      have hk_int : (k : Int) = z := Int.toNat_of_nonneg hz_nonneg
      exact congrArg (fun m : Int => (m : Rat)) hk_int
    have htk : t ≤ (k : Rat) := by rwa [hk_eq]
    have hmul : (k : Rat) * s ≤ t * s :=
      rat_mul_le_mul_of_nonpos_right htk (Rat.le_of_lt hs)
    have hts : t * s = M - obj0 := by
      have hs_eq : s = -(-s) := (Rat.neg_neg s).symm
      calc
        t * s = ((obj0 - M) / (-s)) * (-(-s)) := by rw [← hs_eq]
        _ = -(((obj0 - M) / (-s)) * (-s)) := Rat.mul_neg _ _
        _ = -(obj0 - M) := by rw [Rat.div_mul_cancel (Rat.ne_of_gt hns_pos)]
        _ = M - obj0 := Rat.neg_sub obj0 M
    refine ⟨k, (hstep k).1, (hstep k).2, ?_⟩
    rw [LPObjective_rayPoint]
    have hadd := (Rat.add_le_add_left (c := obj0)).mpr hmul
    rw [hts, Rat.add_comm obj0 (M - obj0), Rat.sub_add_cancel] at hadd
    exact hadd

/-! ### Section 3: MIP Euclidean GCD Row-Coefficient Division (`checkGcdRowDiv`) -/

/-- Non-negative GCD over `Int`. -/
def gcdInt3B (a b : Int) : Int :=
  (Int.gcd a b : Int)

/-- Euclidean GCD of `|a_0|, ..., |a_{n-1}|` over `Fin n → Int`. -/
def rowCoeffsGcd : (n : Nat) → (Fin n → Int) → Int
  | 0, _ => 0
  | n + 1, a => gcdInt3B (rowCoeffsGcd n (fun i => a i.castSucc)) (a (Fin.last n))

theorem rowCoeffsGcd_nonneg (n : Nat) (a : Fin n → Int) : 0 ≤ rowCoeffsGcd n a := by
  induction n with
  | zero => exact Int.le_refl 0
  | succ _ _ => exact Int.natCast_nonneg _

theorem rowCoeffsGcd_dvd (n : Nat) (a : Fin n → Int) (j : Fin n) :
    rowCoeffsGcd n a ∣ a j := by
  induction n with
  | zero => exact Fin.elim0 j
  | succ n ih =>
    simp only [rowCoeffsGcd, gcdInt3B]
    by_cases hj : j = Fin.last n
    · subst hj
      exact Int.gcd_dvd_right _ _
    · have hjlt : j.val < n := by
        have hlt := j.isLt
        have hne : j.val ≠ n := fun hv => hj (Fin.ext hv)
        omega
      let j0 : Fin n := ⟨j.val, hjlt⟩
      have hj0 : j = j0.castSucc := Fin.ext rfl
      rw [hj0]
      exact Int.dvd_trans (Int.gcd_dvd_left _ _) (ih (fun i => a i.castSucc) j0)

/-- Outcome of dividing a pure-integer row `∑ a_j x_j ⋄ rhs` by `d > 0`. -/
inductive GcdRowDivOutcome (n : Nat) where
  | reduced (a' : Fin n → Int) (rhs' : Int)
  | infeasible

/-- Computes the canonical GCD row division outcome when `rowCoeffsGcd n a > 0`. -/
def evalGcdRowDiv {n : Nat} (sense : ConstraintSense) (a : Fin n → Int) (rhs : Rat) :
    Option (Int × GcdRowDivOutcome n) :=
  let d := rowCoeffsGcd n a
  if 0 < d then
    let q := rhs / (d : Rat)
    let a' : Fin n → Int := fun j => Int.tdiv (a j) d
    match sense with
    | .le => some (d, .reduced a' q.floor)
    | .ge => some (d, .reduced a' q.ceil)
    | .eq =>
      if q.den == 1 then
        some (d, .reduced a' q.num)
      else
        some (d, .infeasible)
  else
    none

/-- Executable checker for Euclidean GCD row-coefficient division over `Rat` RHS
(covering both integer RHS `b : Int` via `(b : Rat)` and general rational RHS). -/
def checkGcdRowDiv {n : Nat} (sense : ConstraintSense) (a : Fin n → Int) (rhs : Rat)
    (d : Int) (out : GcdRowDivOutcome n) : Bool :=
  decide (0 < d) &&
  allFin3B n (fun j => decide (Int.tmod (a j) d = 0)) &&
  match out with
  | .reduced a' rhs' =>
    allFin3B n (fun j => decide (a j = d * a' j)) &&
    match sense with
    | .le => rhs' == (rhs / (d : Rat)).floor
    | .ge => rhs' == (rhs / (d : Rat)).ceil
    | .eq => (rhs / (d : Rat)).den == 1 && rhs' == (rhs / (d : Rat)).num
  | .infeasible =>
    sense == .eq && (rhs / (d : Rat)).den != 1

/-- Soundness and exact solution-set equivalence of MIP GCD row-coefficient division:
- `.reduced a' rhs'` preserves exact integer solutions (`↔`) for `<=` (`floor(rhs/d)`),
  `>=` (`ceil(rhs/d)`), and `=` (`rhs/d` integral).
- `.infeasible` certifies that no integer vector satisfies the equality row. -/
theorem gcd_row_div_sound {n : Nat} {sense : ConstraintSense} {a : Fin n → Int}
    {rhs : Rat} {d : Int} {out : GcdRowDivOutcome n}
    (hok : checkGcdRowDiv sense a rhs d out = true) :
    match out with
    | .reduced a' rhs' =>
      ∀ x : Fin n → Int,
        SatisfiesRow sense ((intFinSum n (fun j => a j * x j) : Int) : Rat) rhs ↔
        SatisfiesRow sense ((intFinSum n (fun j => a' j * x j) : Int) : Rat) (rhs' : Rat)
    | .infeasible =>
      ¬ ∃ x : Fin n → Int,
        SatisfiesRow sense ((intFinSum n (fun j => a j * x j) : Int) : Rat) rhs := by
  unfold checkGcdRowDiv at hok
  rw [Bool.and_eq_true, Bool.and_eq_true, decide_eq_true_eq, allFin3B_iff] at hok
  rcases hok with ⟨⟨hd_pos, hmod⟩, hout⟩
  have hd_rat_pos : (0 : Rat) < (d : Rat) := Rat.intCast_lt_intCast.mpr hd_pos
  have hd_rat_ne : (d : Rat) ≠ 0 := Rat.ne_of_gt hd_rat_pos
  cases out with
  | reduced a' rhs' =>
    dsimp only at hout ⊢
    rw [Bool.and_eq_true, allFin3B_iff] at hout
    rcases hout with ⟨hfactor, hrhs⟩
    intro x
    let S' : Int := intFinSum n (fun j => a' j * x j)
    have hsum_eq : intFinSum n (fun j => a j * x j) = S' * d := by
      have hc : ∀ j : Fin n, a j * x j = d * (a' j * x j) := by
        intro j
        rw [decide_eq_true_eq.mp (hfactor j), Int.mul_assoc]
      rw [intFinSum_congr hc, intFinSum_const_mul, Int.mul_comm]
    have hsum_rat : ((intFinSum n (fun j => a j * x j) : Int) : Rat) = (S' : Rat) * (d : Rat) := by
      rw [hsum_eq, Rat.intCast_mul]
    cases sense with
    | le =>
      simp only [beq_iff_eq] at hrhs
      unfold SatisfiesRow
      rw [hsum_rat, hrhs]
      constructor
      · intro hle
        have hdiv : (S' : Rat) ≤ rhs / (d : Rat) := (rat_le_div_iff hd_rat_pos).mpr hle
        exact Rat.intCast_le_intCast.mpr (Rat.le_floor_iff.mpr hdiv)
      · intro hle
        have hdiv : (S' : Rat) ≤ rhs / (d : Rat) :=
          Rat.le_floor_iff.mp (Rat.intCast_le_intCast.mp hle)
        exact (rat_le_div_iff hd_rat_pos).mp hdiv
    | ge =>
      simp only [beq_iff_eq] at hrhs
      unfold SatisfiesRow
      rw [hsum_rat, hrhs]
      constructor
      · intro hge
        have hdiv : rhs / (d : Rat) ≤ (S' : Rat) := (rat_div_le_iff hd_rat_pos).mpr hge
        exact Rat.intCast_le_intCast.mpr (Rat.ceil_le_iff.mpr hdiv)
      · intro hge
        have hdiv : rhs / (d : Rat) ≤ (S' : Rat) :=
          Rat.ceil_le_iff.mp (Rat.intCast_le_intCast.mp hge)
        exact (rat_div_le_iff hd_rat_pos).mp hdiv
    | eq =>
      rw [Bool.and_eq_true, beq_iff_eq, beq_iff_eq] at hrhs
      rcases hrhs with ⟨hden, hrhs_num⟩
      unfold SatisfiesRow
      rw [hsum_rat]
      have hq_eq : rhs / (d : Rat) = (rhs' : Rat) := by
        rw [hrhs_num]
        exact rat_eq_num_of_den_one hden
      have hrhs_eq : rhs = (rhs' : Rat) * (d : Rat) := by
        have hm := congrArg (· * (d : Rat)) hq_eq
        rwa [Rat.div_mul_cancel hd_rat_ne] at hm
      rw [hrhs_eq]
      constructor
      · intro heq
        have hm := congrArg (· / (d : Rat)) heq
        rw [Rat.div_def, Rat.mul_assoc, Rat.mul_inv_cancel (d : Rat) hd_rat_ne, Rat.mul_one,
            Rat.div_def, Rat.mul_assoc, Rat.mul_inv_cancel (d : Rat) hd_rat_ne, Rat.mul_one] at hm
        exact hm
      · intro heq
        rw [heq]
  | infeasible =>
    dsimp only at hout ⊢
    rw [Bool.and_eq_true, bne_iff_ne] at hout
    rcases hout with ⟨hsense_b, hden_ne⟩
    have hsense : sense = .eq := by
      cases sense <;> first | rfl | cases hsense_b
    subst hsense
    rintro ⟨x, hrow⟩
    unfold SatisfiesRow at hrow
    have hdvd_all : ∀ j : Fin n, d ∣ a j := by
      intro j
      exact Int.dvd_of_tmod_eq_zero (decide_eq_true_eq.mp (hmod j))
    have hdvd_sum : d ∣ intFinSum n (fun j => a j * x j) := intFinSum_dvd hdvd_all
    rcases hdvd_sum with ⟨k, hk⟩
    have hq_eq : rhs / (d : Rat) = (k : Rat) := by
      rw [← hrow, hk, Int.mul_comm d k, Rat.intCast_mul, Rat.div_def, Rat.mul_assoc,
          Rat.mul_inv_cancel (d : Rat) hd_rat_ne, Rat.mul_one]
    have hden1 : (rhs / (d : Rat)).den = 1 := by
      rw [hq_eq]
      rfl
    exact hden_ne hden1

/-- `evalGcdRowDiv` always returns a divisor and outcome that pass `checkGcdRowDiv`. -/
theorem evalGcdRowDiv_checkGcdRowDiv {n : Nat} {sense : ConstraintSense} {a : Fin n → Int}
    {rhs : Rat} {d : Int} {out : GcdRowDivOutcome n}
    (heval : evalGcdRowDiv sense a rhs = some (d, out)) :
    checkGcdRowDiv sense a rhs d out = true := by
  unfold evalGcdRowDiv at heval
  dsimp only at heval
  split at heval
  · next hd_pos =>
    have hd_ne : rowCoeffsGcd n a ≠ 0 := Int.ne_of_gt hd_pos
    have hdiv_props :
        ∀ j : Fin n,
          Int.tmod (a j) (rowCoeffsGcd n a) = 0 ∧
          a j = rowCoeffsGcd n a * Int.tdiv (a j) (rowCoeffsGcd n a) := by
      intro j
      rcases rowCoeffsGcd_dvd n a j with ⟨k, hk⟩
      have htdiv : Int.tdiv (a j) (rowCoeffsGcd n a) = k := by
        rw [hk, Int.mul_tdiv_cancel_left k hd_ne]
      have hmul : a j = rowCoeffsGcd n a * Int.tdiv (a j) (rowCoeffsGcd n a) := by
        rw [htdiv, hk]
      have hmod : Int.tmod (a j) (rowCoeffsGcd n a) = 0 :=
        Int.tmod_eq_zero_of_dvd (rowCoeffsGcd_dvd n a j)
      exact ⟨hmod, hmul⟩
    cases sense with
    | le =>
      injection heval with hpair
      injection hpair with hd_eq hout_eq
      subst hd_eq; subst hout_eq
      unfold checkGcdRowDiv
      simp only [Bool.and_eq_true, decide_eq_true_eq, allFin3B_iff, beq_self_eq_true, and_true]
      exact ⟨⟨hd_pos, fun j => (hdiv_props j).1⟩, fun j => (hdiv_props j).2⟩
    | ge =>
      injection heval with hpair
      injection hpair with hd_eq hout_eq
      subst hd_eq; subst hout_eq
      unfold checkGcdRowDiv
      simp only [Bool.and_eq_true, decide_eq_true_eq, allFin3B_iff, beq_self_eq_true, and_true]
      exact ⟨⟨hd_pos, fun j => (hdiv_props j).1⟩, fun j => (hdiv_props j).2⟩
    | eq =>
      dsimp only at heval
      cases hden : (rhs / (rowCoeffsGcd n a : Rat)).den == 1 with
      | true =>
        simp only [hden, ↓reduceIte] at heval
        injection heval with hpair
        injection hpair with hd_eq hout_eq
        subst hd_eq; subst hout_eq
        unfold checkGcdRowDiv
        simp only [Bool.and_eq_true, decide_eq_true_eq, allFin3B_iff, beq_self_eq_true, and_true]
        exact ⟨⟨hd_pos, fun j => (hdiv_props j).1⟩, fun j => (hdiv_props j).2, hden⟩
      | false =>
        simp only [hden, Bool.false_eq_true, ↓reduceIte] at heval
        injection heval with hpair
        injection hpair with hd_eq hout_eq
        subst hd_eq; subst hout_eq
        unfold checkGcdRowDiv
        simp only [Bool.and_eq_true, decide_eq_true_eq, allFin3B_iff, bne_iff_ne]
        refine ⟨⟨hd_pos, fun j => (hdiv_props j).1⟩, rfl, ?_⟩
        intro h1
        rw [h1] at hden
        simp at hden
  · cases heval

/-- Soundness and exact solution-set equivalence of `evalGcdRowDiv`, derived from `gcd_row_div_sound`. -/
theorem evalGcdRowDiv_sound {n : Nat} {sense : ConstraintSense} {a : Fin n → Int}
    {rhs : Rat} {d : Int} {out : GcdRowDivOutcome n}
    (heval : evalGcdRowDiv sense a rhs = some (d, out)) :
    match out with
    | .reduced a' rhs' =>
      ∀ x : Fin n → Int,
        SatisfiesRow sense ((intFinSum n (fun j => a j * x j) : Int) : Rat) rhs ↔
        SatisfiesRow sense ((intFinSum n (fun j => a' j * x j) : Int) : Rat) (rhs' : Rat)
    | .infeasible =>
      ¬ ∃ x : Fin n → Int,
        SatisfiesRow sense ((intFinSum n (fun j => a j * x j) : Int) : Rat) rhs := by
  have hchk := evalGcdRowDiv_checkGcdRowDiv heval
  cases out with
  | reduced a' rhs' => exact gcd_row_div_sound hchk
  | infeasible => exact gcd_row_div_sound hchk

/-! ### Section 4: LP Presolve Step & Postsolve Dual Reconstruction (`checkLPPresolveStep`) -/

/-- Verified LP presolve reduction step witnessing dual-exact transformation `LP ↦ LP'`. -/
inductive LPPresolveStep where
  /-- Eliminates fixed variable `col` (`lb = ub = some val`) from rows while adjusting RHS. -/
  | fixedSubst (col : Nat) (val : Rat)
  /-- Absorbs singleton row `origRow` on variable `col` into `LP'` bounds, mapping remaining
      rows of `LP'` injectively via `origRowIdx` into `LP` rows (`≠ origRow`). -/
  | singletonRow (origRow : Nat) (col : Nat) (origRowIdx : Array Nat)
  /-- Fixes unblocked / dual-dominated column `col` to its lower bound (`toLower = true`)
      or upper bound (`toLower = false`). -/
  | dualColFix (col : Nat) (toLower : Bool)
  /-- Fixes a zero-objective column (`obj col = 0`) with zero coefficients in all rows
      to a feasible constant `val`. -/
  | zeroObjEmptyColFix (col : Nat) (val : Rat)
  /-- Drops a redundant row `droppedRow`, keeping remaining rows via `origRowIdx`. -/
  | dropRedundantRow (droppedRow : Nat) (origRowIdx : Array Nat)
  deriving Repr

/-- Looks up the reduced-row index `q` such that `origRowIdx[q] == origRow`, if any. -/
def findReducedRow? (mRed : Nat) (origRowIdx : Array Nat) (origRow : Nat) : Option (Fin mRed) :=
  loop 0
where
  loop (q : Nat) : Option (Fin mRed) :=
    if hq : q < mRed then
      if hsz : q < origRowIdx.size then
        if origRowIdx[q] == origRow then some ⟨q, hq⟩
        else loop (q + 1)
      else none
    else none
  termination_by mRed - q

private theorem findReducedRow_loop_some {mRed : Nat} {origRowIdx : Array Nat} {origRow : Nat}
    (hsz : origRowIdx.size = mRed) :
    ∀ (k q : Nat), mRed - q = k →
      ∀ r : Fin mRed, findReducedRow?.loop mRed origRowIdx origRow q = some r →
        origRowIdx[r.val]'(by rw [hsz]; exact r.isLt) = origRow := by
  intro k
  induction k with
  | zero =>
    intro q hdiff r hloop
    have hge : ¬q < mRed := by omega
    unfold findReducedRow?.loop at hloop
    simp [hge] at hloop
  | succ k ih =>
    intro q hdiff r hloop
    have hlt : q < mRed := by omega
    have hltSz : q < origRowIdx.size := by rw [hsz]; exact hlt
    unfold findReducedRow?.loop at hloop
    simp only [hlt, hltSz, ↓reduceDIte] at hloop
    by_cases heq : origRowIdx[q] = origRow
    · simp only [heq, beq_self_eq_true, ↓reduceIte] at hloop
      injection hloop with hr
      subst hr
      exact heq
    · have hbeq : (origRowIdx[q] == origRow) = false := beq_false_of_ne heq
      simp only [hbeq, Bool.false_eq_true, ↓reduceIte] at hloop
      exact ih (q + 1) (by omega) r hloop

private theorem findReducedRow_loop_complete {mRed : Nat} {origRowIdx : Array Nat} {origRow : Nat}
    (hsz : origRowIdx.size = mRed)
    (hinj : ∀ q1 q2 : Fin mRed,
      origRowIdx[q1.val]'(by rw [hsz]; exact q1.isLt) =
      origRowIdx[q2.val]'(by rw [hsz]; exact q2.isLt) → q1 = q2) :
    ∀ (k q : Nat), mRed - q = k →
      ∀ target : Fin mRed, q ≤ target.val →
        origRowIdx[target.val]'(by rw [hsz]; exact target.isLt) = origRow →
        findReducedRow?.loop mRed origRowIdx origRow q = some target := by
  intro k
  induction k with
  | zero =>
    intro q _ target hle _
    have := target.isLt
    omega
  | succ k ih =>
    intro q _ target hle htarget
    have hlt : q < mRed := by omega
    have hltSz : q < origRowIdx.size := by rw [hsz]; exact hlt
    unfold findReducedRow?.loop
    simp only [hlt, hltSz, ↓reduceDIte]
    by_cases heq : origRowIdx[q] = origRow
    · have hsame := hinj ⟨q, hlt⟩ target (heq.trans htarget.symm)
      simp [heq, hsame]
    · have hbeq : (origRowIdx[q] == origRow) = false := beq_false_of_ne heq
      have hne_idx : q ≠ target.val := by
        intro hq
        have hfin : (⟨q, hlt⟩ : Fin mRed) = target := Fin.ext hq
        have htq : origRowIdx[q]'hltSz = origRow := by
          rw [← hfin] at htarget
          exact htarget
        exact heq htq
      simp only [hbeq, Bool.false_eq_true, ↓reduceIte]
      exact ih (q + 1) (by omega) target (by omega) htarget

private theorem findReducedRow_none_of_forall_ne {mRed : Nat} {origRowIdx : Array Nat} {origRow : Nat}
    (hsz : origRowIdx.size = mRed)
    (hne : ∀ q : Fin mRed, origRowIdx[q.val]'(by rw [hsz]; exact q.isLt) ≠ origRow) :
    findReducedRow? mRed origRowIdx origRow = none := by
  cases h : findReducedRow? mRed origRowIdx origRow with
  | none => rfl
  | some r =>
    have heq := findReducedRow_loop_some hsz mRed 0 (by omega) r h
    exact False.elim (hne r heq)

private theorem findReducedRow_some_of_mem {mRed : Nat} {origRowIdx : Array Nat}
    (hsz : origRowIdx.size = mRed)
    (hinj : ∀ q1 q2 : Fin mRed,
      origRowIdx[q1.val]'(by rw [hsz]; exact q1.isLt) =
      origRowIdx[q2.val]'(by rw [hsz]; exact q2.isLt) → q1 = q2)
    (q : Fin mRed) :
    findReducedRow? mRed origRowIdx (origRowIdx[q.val]'(by rw [hsz]; exact q.isLt)) = some q :=
  findReducedRow_loop_complete hsz hinj mRed 0 (by omega) q (Nat.zero_le _) rfl

/-- Executable postsolve dual reconstruction mapping `yRed` on `LP'` to `yOrig` on `LP`. -/
def postsolveLPDual (LP LP' : LPInstance) (step : LPPresolveStep)
    (yRed : Fin LP'.numRows → Rat) : Fin LP.numRows → Rat :=
  match step with
  | .fixedSubst _ _ | .dualColFix _ _ | .zeroObjEmptyColFix _ _ =>
    if hRows : LP'.numRows = LP.numRows then
      fun i => yRed ⟨i.val, by rw [hRows]; exact i.isLt⟩
    else
      fun _ => 0
  | .dropRedundantRow _ origRowIdx =>
    fun i =>
      match findReducedRow? LP'.numRows origRowIdx i.val with
      | some q => projDual (LP'.sense q) (yRed q)
      | none => 0
  | .singletonRow origRow col origRowIdx =>
    if hRow : origRow < LP.numRows then
      if hCol : col < LP.numVars then
        let rFin : Fin LP.numRows := ⟨origRow, hRow⟩
        let cFin : Fin LP.numVars := ⟨col, hCol⟩
        let yBase : Fin LP.numRows → Rat := fun i =>
          match findReducedRow? LP'.numRows origRowIdx i.val with
          | some q => projDual (LP'.sense q) (yRed q)
          | none => 0
        let a := rowCoeff LP rFin cFin
        let rcBase := LP.obj cFin - finSum LP.numRows (fun i => yBase i * rowCoeff LP i cFin)
        let sngSense := LP.sense rFin
        let isUpper := (sngSense == .le && decide (0 < a)) || (sngSense == .ge && decide (a < 0))
        let ySing : Rat :=
          if sngSense == .eq then
            rcBase / a
          else if isUpper then
            if rcBase < 0 then rcBase / a else 0
          else
            if 0 < rcBase then rcBase / a else 0
        fun i => if i = rFin then ySing else yBase i
      else
        fun _ => 0
    else
      fun _ => 0

/-- Checks that `origRowIdx` maps `Fin LP'.numRows` injectively onto all rows of `LP` except `skipRow`,
preserving row senses, RHS values, and sparse coefficients. -/
def checkRowSubmap (LP LP' : LPInstance) (hVars : LP'.numVars = LP.numVars)
    (skipRow : Fin LP.numRows) (origRowIdx : Array Nat) : Bool :=
  if hSz : origRowIdx.size = LP'.numRows then
    allFin3B LP'.numRows (fun q =>
      let origIdx := origRowIdx[q.val]'(by rw [hSz]; exact q.isLt)
      if hOrig : origIdx < LP.numRows then
        let iOrig : Fin LP.numRows := ⟨origIdx, hOrig⟩
        (origIdx != skipRow.val) &&
        allFin3B LP'.numRows (fun q2 =>
          if q == q2 then true
          else origRowIdx[q2.val]'(by rw [hSz]; exact q2.isLt) != origIdx) &&
        (LP'.sense q == LP.sense iOrig) &&
        (LP'.rhs q == LP.rhs iOrig) &&
        allFin3B LP.numVars (fun j =>
          let j' : Fin LP'.numVars := ⟨j.val, by rw [hVars]; exact j.isLt⟩
          rowCoeff LP' q j' == rowCoeff LP iOrig j)
      else
        false) &&
    allFin3B LP.numRows (fun i =>
      if i == skipRow then true
      else anyFin3B LP'.numRows (fun q =>
        origRowIdx[q.val]'(by rw [hSz]; exact q.isLt) == i.val))
  else
    false

/-- Executable checker verifying that `step` is a sound primal-dual LP presolve reduction `LP ↦ LP'`. -/
def checkLPPresolveStep (LP LP' : LPInstance) (step : LPPresolveStep) : Bool :=
  if hVars : LP'.numVars = LP.numVars then
    match step with
    | .fixedSubst col val =>
      if hRows : LP'.numRows = LP.numRows then
        if hCol : col < LP.numVars then
          let cFin : Fin LP.numVars := ⟨col, hCol⟩
          let cFin' : Fin LP'.numVars := ⟨col, by rw [hVars]; exact hCol⟩
          (LP.lb cFin == some val) &&
          (LP.ub cFin == some val) &&
          allFin3B LP.numVars (fun j =>
            let j' : Fin LP'.numVars := ⟨j.val, by rw [hVars]; exact j.isLt⟩
            (LP'.lb j' == LP.lb j) && (LP'.ub j' == LP.ub j)) &&
          allFin3B LP.numVars (fun j =>
            if j == cFin then true
            else
              let j' : Fin LP'.numVars := ⟨j.val, by rw [hVars]; exact j.isLt⟩
              LP'.obj j' == LP.obj j) &&
          (LP'.objConst + LP'.obj cFin' * val == LP.objConst + LP.obj cFin * val) &&
          allFin3B LP.numRows (fun i =>
            let i' : Fin LP'.numRows := ⟨i.val, by rw [hRows]; exact i.isLt⟩
            (LP'.sense i' == LP.sense i) &&
            (rowCoeff LP' i' cFin' == 0) &&
            (LP'.rhs i' == LP.rhs i - rowCoeff LP i cFin * val) &&
            allFin3B LP.numVars (fun j =>
              if j == cFin then true
              else
                let j' : Fin LP'.numVars := ⟨j.val, by rw [hVars]; exact j.isLt⟩
                rowCoeff LP' i' j' == rowCoeff LP i j))
        else false
      else false
    | .singletonRow origRow col origRowIdx =>
      if hRow : origRow < LP.numRows then
        if hCol : col < LP.numVars then
          let rFin : Fin LP.numRows := ⟨origRow, hRow⟩
          let cFin : Fin LP.numVars := ⟨col, hCol⟩
          let cFin' : Fin LP'.numVars := ⟨col, by rw [hVars]; exact hCol⟩
          let a := rowCoeff LP rFin cFin
          let b := LP.rhs rFin
          let imp := b / a
          (a != 0) &&
          allFin3B LP.numVars (fun j =>
            if j == cFin then true else rowCoeff LP rFin j == 0) &&
          checkRowSubmap LP LP' hVars rFin origRowIdx &&
          (LP'.objConst == LP.objConst) &&
          allFin3B LP.numVars (fun j =>
            let j' : Fin LP'.numVars := ⟨j.val, by rw [hVars]; exact j.isLt⟩
            LP'.obj j' == LP.obj j) &&
          allFin3B LP.numVars (fun j =>
            if j == cFin then true
            else
              let j' : Fin LP'.numVars := ⟨j.val, by rw [hVars]; exact j.isLt⟩
              (LP'.lb j' == LP.lb j) && (LP'.ub j' == LP.ub j)) &&
          match LP.sense rFin with
          | .eq =>
            checkVarBounds (LP.lb cFin) (LP.ub cFin) imp &&
            (LP'.lb cFin' == some imp) &&
            (LP'.ub cFin' == some imp)
          | .le =>
            if 0 < a then
              checkVarBounds (LP.lb cFin) (LP.ub cFin) imp &&
              (LP'.lb cFin' == LP.lb cFin) &&
              (LP'.ub cFin' == some imp)
            else
              checkVarBounds (LP.lb cFin) (LP.ub cFin) imp &&
              (LP'.lb cFin' == some imp) &&
              (LP'.ub cFin' == LP.ub cFin)
          | .ge =>
            if 0 < a then
              checkVarBounds (LP.lb cFin) (LP.ub cFin) imp &&
              (LP'.lb cFin' == some imp) &&
              (LP'.ub cFin' == LP.ub cFin)
            else
              checkVarBounds (LP.lb cFin) (LP.ub cFin) imp &&
              (LP'.lb cFin' == LP.lb cFin) &&
              (LP'.ub cFin' == some imp)
        else false
      else false
    | .dualColFix col toLower =>
      if hRows : LP'.numRows = LP.numRows then
        if hCol : col < LP.numVars then
          let cFin : Fin LP.numVars := ⟨col, hCol⟩
          let cFin' : Fin LP'.numVars := ⟨col, by rw [hVars]; exact hCol⟩
          (LP'.objConst == LP.objConst) &&
          allFin3B LP.numVars (fun j =>
            let j' : Fin LP'.numVars := ⟨j.val, by rw [hVars]; exact j.isLt⟩
            LP'.obj j' == LP.obj j) &&
          allFin3B LP.numVars (fun j =>
            if j == cFin then true
            else
              let j' : Fin LP'.numVars := ⟨j.val, by rw [hVars]; exact j.isLt⟩
              (LP'.lb j' == LP.lb j) && (LP'.ub j' == LP.ub j)) &&
          allFin3B LP.numRows (fun i =>
            let i' : Fin LP'.numRows := ⟨i.val, by rw [hRows]; exact i.isLt⟩
            (LP'.sense i' == LP.sense i) &&
            (LP'.rhs i' == LP.rhs i) &&
            allFin3B LP.numVars (fun j =>
              let j' : Fin LP'.numVars := ⟨j.val, by rw [hVars]; exact j.isLt⟩
              rowCoeff LP' i' j' == rowCoeff LP i j)) &&
          if toLower then
            match LP.lb cFin with
            | none => false
            | some l =>
              checkVarBounds (LP.lb cFin) (LP.ub cFin) l &&
              (LP'.lb cFin' == some l) &&
              (LP'.ub cFin' == some l) &&
              decide (0 ≤ LP.obj cFin) &&
              allFin3B LP.numRows (fun i =>
                let a := rowCoeff LP i cFin
                match LP.sense i with
                | .le => decide (0 ≤ a)
                | .ge => decide (a ≤ 0)
                | .eq => a == 0)
          else
            match LP.ub cFin with
            | none => false
            | some u =>
              checkVarBounds (LP.lb cFin) (LP.ub cFin) u &&
              (LP'.lb cFin' == some u) &&
              (LP'.ub cFin' == some u) &&
              decide (LP.obj cFin ≤ 0) &&
              allFin3B LP.numRows (fun i =>
                let a := rowCoeff LP i cFin
                match LP.sense i with
                | .le => decide (a ≤ 0)
                | .ge => decide (0 ≤ a)
                | .eq => a == 0)
        else false
      else false
    | .zeroObjEmptyColFix col val =>
      if hRows : LP'.numRows = LP.numRows then
        if hCol : col < LP.numVars then
          let cFin : Fin LP.numVars := ⟨col, hCol⟩
          let cFin' : Fin LP'.numVars := ⟨col, by rw [hVars]; exact hCol⟩
          checkVarBounds (LP.lb cFin) (LP.ub cFin) val &&
          (LP.obj cFin == 0) &&
          (LP'.lb cFin' == some val) &&
          (LP'.ub cFin' == some val) &&
          (LP'.objConst == LP.objConst) &&
          allFin3B LP.numVars (fun j =>
            let j' : Fin LP'.numVars := ⟨j.val, by rw [hVars]; exact j.isLt⟩
            LP'.obj j' == LP.obj j) &&
          allFin3B LP.numVars (fun j =>
            if j == cFin then true
            else
              let j' : Fin LP'.numVars := ⟨j.val, by rw [hVars]; exact j.isLt⟩
              (LP'.lb j' == LP.lb j) && (LP'.ub j' == LP.ub j)) &&
          allFin3B LP.numRows (fun i =>
            let i' : Fin LP'.numRows := ⟨i.val, by rw [hRows]; exact i.isLt⟩
            (rowCoeff LP i cFin == 0) &&
            (LP'.sense i' == LP.sense i) &&
            (LP'.rhs i' == LP.rhs i) &&
            allFin3B LP.numVars (fun j =>
              let j' : Fin LP'.numVars := ⟨j.val, by rw [hVars]; exact j.isLt⟩
              rowCoeff LP' i' j' == rowCoeff LP i j))
        else false
      else false
    | .dropRedundantRow droppedRow origRowIdx =>
      if hDrop : droppedRow < LP.numRows then
        let dFin : Fin LP.numRows := ⟨droppedRow, hDrop⟩
        checkRowSubmap LP LP' hVars dFin origRowIdx &&
        (LP'.objConst == LP.objConst) &&
        allFin3B LP.numVars (fun j =>
          let j' : Fin LP'.numVars := ⟨j.val, by rw [hVars]; exact j.isLt⟩
          (LP'.obj j' == LP.obj j) &&
          (LP'.lb j' == LP.lb j) &&
          (LP'.ub j' == LP.ub j)) &&
        -- Redundancy witness: dropped row is implied by variable bounds over `[lb, ub]`.
        match LP.sense dFin with
        | .eq =>
          allFin3B LP.numVars (fun j => rowCoeff LP dFin j == 0) && (LP.rhs dFin == 0)
        | .le =>
          match sumOptFin LP.numVars (fun j =>
            farkasTermUpper (rowCoeff LP dFin j) (LP.lb j) (LP.ub j)) with
          | none => false
          | some maxAct => decide (maxAct ≤ LP.rhs dFin)
        | .ge =>
          match sumOptFin LP.numVars (fun j =>
            rcTermBound (rowCoeff LP dFin j) (LP.lb j) (LP.ub j)) with
          | none => false
          | some minAct => decide (LP.rhs dFin ≤ minAct)
      else false
  else false

private theorem rcTermBound_fixed (r v : Rat) :
    rcTermBound r (some v) (some v) = some (r * v) := by
  unfold rcTermBound
  by_cases hpos : 0 < r
  · simp [hpos]
  · simp only [hpos, ↓reduceIte]
    by_cases hneg : r < 0
    · simp [hneg]
    · simp only [hneg, ↓reduceIte]
      have hr0 : r = 0 := Rat.le_antisymm (Rat.not_lt.mp hpos) (Rat.not_lt.mp hneg)
      simp [hr0, Rat.zero_mul]

private theorem constraintSense_beq_iff {a b : ConstraintSense} :
    (a == b) = true ↔ a = b := by
  cases a <;> cases b <;> decide

private theorem projDual_idem (sense : ConstraintSense) (y : Rat) :
    projDual sense (projDual sense y) = projDual sense y := by
  cases sense with
  | le =>
    unfold projDual
    dsimp only
    by_cases hy : y ≤ 0
    · simp [hy]
    · simp [hy]
  | ge =>
    unfold projDual
    dsimp only
    by_cases hy : 0 ≤ y
    · simp [hy]
    · simp [hy]
  | eq => rfl

private theorem projDual_zero (sense : ConstraintSense) : projDual sense 0 = 0 := by
  cases sense <;> simp [projDual]

private theorem projDual_le_nonpos (y : Rat) : projDual .le y ≤ 0 := by
  unfold projDual
  dsimp only
  split
  · assumption
  · exact Rat.le_refl

private theorem projDual_ge_nonneg (y : Rat) : 0 ≤ projDual .ge y := by
  unfold projDual
  dsimp only
  split
  · assumption
  · exact Rat.le_refl

private theorem checkRowSubmap_props {LP LP' : LPInstance} {hVars : LP'.numVars = LP.numVars}
    {skipRow : Fin LP.numRows} {origRowIdx : Array Nat}
    (hsub : checkRowSubmap LP LP' hVars skipRow origRowIdx = true) :
    ∃ (hSz : origRowIdx.size = LP'.numRows) (mapFn : Fin LP'.numRows → Fin LP.numRows),
      (∀ q, (mapFn q).val = origRowIdx[q.val]'(by rw [hSz]; exact q.isLt)) ∧
      (∀ q, mapFn q ≠ skipRow) ∧
      (∀ q1 q2, mapFn q1 = mapFn q2 → q1 = q2) ∧
      (∀ i : Fin LP.numRows, i ≠ skipRow → ∃ q, mapFn q = i) ∧
      (∀ q, LP'.sense q = LP.sense (mapFn q)) ∧
      (∀ q, LP'.rhs q = LP.rhs (mapFn q)) ∧
      (∀ q (j : Fin LP.numVars),
        rowCoeff LP' q ⟨j.val, by rw [hVars]; exact j.isLt⟩ = rowCoeff LP (mapFn q) j) := by
  unfold checkRowSubmap at hsub
  split at hsub
  · next hSz =>
    rw [Bool.and_eq_true, allFin3B_iff, allFin3B_iff] at hsub
    rcases hsub with ⟨hqAll, hsurjAll⟩
    have hltOrig : ∀ q : Fin LP'.numRows,
        origRowIdx[q.val]'(by rw [hSz]; exact q.isLt) < LP.numRows := by
      intro q
      have hq := hqAll q
      dsimp only at hq
      split at hq
      · assumption
      · cases hq
    let mapFn : Fin LP'.numRows → Fin LP.numRows :=
      fun q => ⟨origRowIdx[q.val]'(by rw [hSz]; exact q.isLt), hltOrig q⟩
    refine ⟨hSz, mapFn, (fun _ => rfl), ?_, ?_, ?_, ?_, ?_, ?_⟩
    · intro q heq
      have hq := hqAll q
      dsimp only at hq
      have hlt := hltOrig q
      simp only [hlt, ↓reduceDIte, Bool.and_eq_true] at hq
      have hne := bne_iff_ne.mp hq.1.1.1.1
      exact hne (congrArg Fin.val heq)
    · intro q1 q2 heq
      by_cases hqq : q1 = q2
      · exact hqq
      · have hq := hqAll q1
        dsimp only at hq
        have hlt := hltOrig q1
        simp only [hlt, ↓reduceDIte, Bool.and_eq_true, allFin3B_iff] at hq
        have hinj2 := hq.1.1.1.2 q2
        have hbeq : (q1 == q2) = false := beq_false_of_ne hqq
        simp only [hbeq, Bool.false_eq_true, ↓reduceIte, bne_iff_ne] at hinj2
        exact False.elim (hinj2 (congrArg Fin.val heq).symm)
    · intro i hne
      have hi := hsurjAll i
      have hbeq : (i == skipRow) = false := beq_false_of_ne hne
      simp only [hbeq, Bool.false_eq_true, ↓reduceIte, anyFin3B_iff, beq_iff_eq] at hi
      rcases hi with ⟨q, hqval⟩
      exact ⟨q, Fin.ext hqval⟩
    · intro q
      have hq := hqAll q
      dsimp only at hq
      have hlt := hltOrig q
      simp only [hlt, ↓reduceDIte, Bool.and_eq_true] at hq
      exact constraintSense_beq_iff.mp hq.1.1.2
    · intro q
      have hq := hqAll q
      dsimp only at hq
      have hlt := hltOrig q
      simp only [hlt, ↓reduceDIte, Bool.and_eq_true, beq_iff_eq] at hq
      exact hq.1.2
    · intro q j
      have hq := hqAll q
      dsimp only at hq
      have hlt := hltOrig q
      simp only [hlt, ↓reduceDIte, Bool.and_eq_true, allFin3B_iff, beq_iff_eq] at hq
      exact hq.2 j
  · cases hsub

private def lookupValOrZero {α : Type} (o : Option α) (g : α → Rat) : Rat :=
  match o with
  | some q => g q
  | none => 0

private theorem finSum_lookup_bijection {mOrig mRed : Nat}
    (mapFn : Fin mRed → Fin mOrig)
    (lookup : Fin mOrig → Option (Fin mRed))
    (hlookup_of_map : ∀ q : Fin mRed, lookup (mapFn q) = some q)
    (hmap_of_lookup : ∀ (i : Fin mOrig) (q : Fin mRed), lookup i = some q → mapFn q = i)
    (g : Fin mRed → Rat) :
    finSum mOrig (fun i => lookupValOrZero (lookup i) g) = finSum mRed g := by
  induction mRed with
  | zero =>
    simp only [finSum_zero_n]
    have hzero : ∀ i : Fin mOrig, lookupValOrZero (lookup i) g = 0 := by
      intro i
      cases lookup i with
      | none => rfl
      | some q => exact Fin.elim0 q
    rw [finSum_congr hzero, finSum_zero]
  | succ k ih =>
    rw [finSum_succ]
    let lastRow : Fin mOrig := mapFn (Fin.last k)
    rw [finSum_split_at _ lastRow]
    have hlast_val : lookupValOrZero (lookup lastRow) g = g (Fin.last k) := by
      dsimp only [lookupValOrZero, lastRow]
      rw [hlookup_of_map (Fin.last k)]
    rw [hlast_val]
    let mapFn' : Fin k → Fin mOrig := fun p => mapFn p.castSucc
    let lookup' : Fin mOrig → Option (Fin k) := fun i =>
      if i = lastRow then
        none
      else
        match lookup i with
        | none => none
        | some q => if hlt : q.val < k then some ⟨q.val, hlt⟩ else none
    have hlookup'_of_map : ∀ p : Fin k, lookup' (mapFn' p) = some p := by
      intro p
      have hne_last : mapFn p.castSucc ≠ lastRow := by
        intro heq
        have h1 := hlookup_of_map p.castSucc
        rw [heq, hlookup_of_map (Fin.last k)] at h1
        injection h1 with hfin
        have hv := congrArg Fin.val hfin
        simp only [Fin.val_castSucc, Fin.val_last] at hv
        omega
      dsimp only [lookup', mapFn']
      simp only [hne_last, ↓reduceIte, hlookup_of_map p.castSucc, Fin.val_castSucc, p.isLt,
        ↓reduceDIte, Fin.eta]
    have hmap'_of_lookup : ∀ (i : Fin mOrig) (p : Fin k), lookup' i = some p → mapFn' p = i := by
      intro i p hl
      dsimp only [lookup'] at hl
      by_cases hi : i = lastRow
      · simp [hi] at hl
      · simp only [hi, ↓reduceIte] at hl
        cases hli : lookup i with
        | none => simp [hli] at hl
        | some q =>
          simp only [hli] at hl
          by_cases hlt : q.val < k
          · simp only [hlt, ↓reduceDIte] at hl
            injection hl with hp
            have hqc : p.castSucc = q := by
              rw [← hp]
              exact Fin.ext rfl
            dsimp only [mapFn']
            rw [hqc]
            exact hmap_of_lookup i q hli
          · simp [hlt] at hl
    have hih := ih mapFn' lookup' hlookup'_of_map hmap'_of_lookup (fun p => g p.castSucc)
    have hpoint :
        ∀ i : Fin mOrig,
          (if i = lastRow then (0 : Rat) else lookupValOrZero (lookup i) g) =
          lookupValOrZero (lookup' i) (fun p => g p.castSucc) := by
      intro i
      dsimp only [lookup']
      by_cases hi : i = lastRow
      · simp [hi, lookupValOrZero]
      · simp only [hi, ↓reduceIte]
        cases hli : lookup i with
        | none => rfl
        | some q =>
          by_cases hlt : q.val < k
          · simp only [hlt, ↓reduceDIte, lookupValOrZero]
            have hqc : (⟨q.val, hlt⟩ : Fin k).castSucc = q := Fin.ext rfl
            rw [hqc]
          · have hqlast : q = Fin.last k := by
              have hqlt := q.isLt
              exact Fin.ext (by simp only [Fin.val_last]; omega)
            have hmapq := hmap_of_lookup i q hli
            rw [hqlast] at hmapq
            exact False.elim (hi hmapq.symm)
    rw [finSum_congr hpoint, hih]

/-- Casts a primal solution of `LP'` back to `LP` when `LP'.numVars = LP.numVars`. -/
def liftLPPresolvePrimal (LP LP' : LPInstance) (x' : Fin LP'.numVars → Rat) :
    Fin LP.numVars → Rat :=
  if hVars : LP'.numVars = LP.numVars then
    fun j => x' ⟨j.val, by rw [hVars]; exact j.isLt⟩
  else
    fun _ => 0

private def colReducedCost (LP : LPInstance) (yProj : Fin LP.numRows → Rat)
    (j : Fin LP.numVars) : Rat :=
  LP.obj j - finSum LP.numRows (fun i => yProj i * rowCoeff LP i j)

private theorem safeDualBound_eq_spec (LP : LPInstance) (y : Fin LP.numRows → Rat) :
    safeDualBound LP y =
      let y' := fun i => projDual (LP.sense i) (y i)
      match sumOptFin LP.numVars (fun j =>
        rcTermBound (colReducedCost LP y' j) (LP.lb j) (LP.ub j)) with
      | none => none
      | some rcSum =>
        some (LP.objConst + finSum LP.numRows (fun i => y' i * LP.rhs i) + rcSum) := by
  unfold safeDualBound colReducedCost
  dsimp only
  have hc :
      ∀ j : Fin LP.numVars,
        rcTermBound
          (LP.obj j - (computeColDotsArr LP (fun i => projDual (LP.sense i) (y i))).1[j.val]'(by
            rw [(computeColDotsArr LP (fun i => projDual (LP.sense i) (y i))).2]; exact j.isLt))
          (LP.lb j) (LP.ub j) =
        rcTermBound
          (LP.obj j - finSum LP.numRows (fun i => projDual (LP.sense i) (y i) * rowCoeff LP i j))
          (LP.lb j) (LP.ub j) := by
    intro j
    rw [computeColDotsArr_get_eq LP (fun i => projDual (LP.sense i) (y i)) j]
  rw [sumOptFin_congr hc]
  rfl

private theorem varBounds_of_imp_upper {lb ub : Option Rat} {imp x : Rat}
    (himp : SatisfiesVarBounds lb ub imp) (hb : SatisfiesVarBounds lb (some imp) x) :
    SatisfiesVarBounds lb ub x := by
  refine ⟨hb.1, ?_⟩
  cases hub : ub with
  | none => trivial
  | some u =>
    have himp2 := himp.2
    rw [hub] at himp2
    exact Rat.le_trans hb.2 himp2

private theorem varBounds_of_imp_lower {lb ub : Option Rat} {imp x : Rat}
    (himp : SatisfiesVarBounds lb ub imp) (hb : SatisfiesVarBounds (some imp) ub x) :
    SatisfiesVarBounds lb ub x := by
  refine ⟨?_, hb.2⟩
  cases hlb : lb with
  | none => trivial
  | some l =>
    have himp1 := himp.1
    rw [hlb] at himp1
    exact Rat.le_trans himp1 hb.1

/-- Primal lifting soundness: every primal-feasible solution of `LP'` lifts to a
primal-feasible solution of `LP` with identical objective value. -/
theorem lp_presolve_primal_lift {LP LP' : LPInstance} {step : LPPresolveStep}
    {x' : Fin LP'.numVars → Rat}
    (hstep : checkLPPresolveStep LP LP' step = true)
    (hfeas' : PrimalFeasible LP' x') :
    PrimalFeasible LP (liftLPPresolvePrimal LP LP' x') ∧
    LPObjective LP (liftLPPresolvePrimal LP LP' x') = LPObjective LP' x' := by
  cases LP with
  | mk n m objC obj sense rows rhs lb ub =>
    cases LP' with
    | mk n' m' objC' obj' sense' rows' rhs' lb' ub' =>
      let LP0 : LPInstance := ⟨n, m, objC, obj, sense, rows, rhs, lb, ub⟩
      let LP1 : LPInstance := ⟨n', m', objC', obj', sense', rows', rhs', lb', ub'⟩
      change checkLPPresolveStep LP0 LP1 step = true at hstep
      unfold checkLPPresolveStep at hstep
      by_cases hVars : n' = n
      · subst n'
        dsimp only [LP0, LP1] at hstep
        simp only [↓reduceDIte] at hstep
        have hlift_eq : liftLPPresolvePrimal LP0 ⟨n, m', objC', obj', sense', rows', rhs', lb', ub'⟩ x' = x' := by
          funext j
          unfold liftLPPresolvePrimal
          simp [LP0]
        change PrimalFeasible LP0 _ ∧ LPObjective LP0 _ = _
        rw [hlift_eq]
        cases step with
        | fixedSubst col val =>
          by_cases hRows : m' = m
          · subst m'
            by_cases hCol : col < n
            · simp only [↓reduceDIte, hCol, Bool.and_eq_true, allFin3B_iff, beq_iff_eq] at hstep
              rcases hstep with ⟨⟨⟨⟨⟨hlb_c, hub_c⟩, hbnds⟩, hobj_other⟩, hobj_const⟩, hrows⟩
              let cFin : Fin n := ⟨col, hCol⟩
              have hbnds_orig : ∀ j : Fin n, SatisfiesVarBounds (lb j) (ub j) (x' j) := by
                intro j
                have hbj := hbnds j
                have hf1 := hfeas'.1 j
                simpa [LP1, hbj.1, hbj.2] using hf1
              have hxc : x' cFin = val := by
                have hbc := hbnds_orig cFin
                unfold SatisfiesVarBounds at hbc
                rw [hlb_c, hub_c] at hbc
                exact Rat.le_antisymm hbc.2 hbc.1
              refine ⟨⟨hbnds_orig, ?_⟩, ?_⟩
              · intro i
                rcases hrows i with ⟨⟨⟨hsense_b, hrc_zero⟩, hrhs_eq⟩, hrc_other⟩
                have hsense_i : sense' i = sense i := constraintSense_beq_iff.mp hsense_b
                let LP_red : LPInstance := ⟨n, m, objC', obj', sense', rows', rhs', lb', ub'⟩
                have hact_orig :
                    rowActivity LP0 x' i =
                      finSum n (fun j => if j = cFin then 0 else rowCoeff LP0 i j * x' j) +
                        rowCoeff LP0 i cFin * val := by
                  rw [rowActivity_eq_finSum LP0 x' i, finSum_split_at _ cFin, hxc]
                have hact_red :
                    rowActivity LP_red x' i =
                      finSum n (fun j => if j = cFin then 0 else rowCoeff LP0 i j * x' j) := by
                  rw [rowActivity_eq_finSum LP_red x' i, finSum_split_at _ cFin]
                  have hz : rowCoeff LP_red i cFin * x' cFin = 0 := by
                    change rowCoeff _ _ _ * _ = 0
                    rw [hrc_zero, Rat.zero_mul]
                  rw [hz, Rat.add_zero]
                  apply finSum_congr
                  intro j
                  by_cases hj : j = cFin
                  · simp [hj]
                  · have ho := hrc_other j
                    have hj' : j ≠ ⟨col, hCol⟩ := hj
                    simp only [hj', ↓reduceIte, beq_iff_eq] at ho
                    simp only [hj, ↓reduceIte]
                    exact congrArg (· * x' j) ho
                have hact_rel :
                    rowActivity LP0 x' i = rowActivity LP_red x' i + rowCoeff LP0 i cFin * val := by
                  rw [hact_orig, hact_red]
                have hrhs_rel : rhs i = rhs' i + rowCoeff LP0 i cFin * val := by
                  rw [hrhs_eq, Rat.sub_add_cancel]
                have hri' := hfeas'.2 i
                change SatisfiesRow (sense' i) (rowActivity LP_red x' i) (rhs' i) at hri'
                rw [hsense_i] at hri'
                change SatisfiesRow (sense i) (rowActivity LP0 x' i) (rhs i)
                rw [hact_rel, hrhs_rel]
                cases hs_i : sense i with
                | le =>
                  rw [hs_i] at hri'
                  exact Rat.add_le_add_right.mpr hri'
                | ge =>
                  rw [hs_i] at hri'
                  exact Rat.add_le_add_right.mpr hri'
                | eq =>
                  rw [hs_i] at hri'
                  unfold SatisfiesRow at hri' ⊢
                  rw [hri']
              · unfold LPObjective
                dsimp only [LP0]
                rw [finSum_split_at (fun j => obj j * x' j) cFin,
                    finSum_split_at (fun j => obj' j * x' j) cFin, hxc]
                have hsum_other :
                    finSum n (fun j => if j = cFin then 0 else obj j * x' j) =
                    finSum n (fun j => if j = cFin then 0 else obj' j * x' j) := by
                  apply finSum_congr
                  intro j
                  by_cases hj : j = cFin
                  · simp [hj]
                  · have ho := hobj_other j
                    have hj' : j ≠ ⟨col, hCol⟩ := hj
                    simp only [hj', ↓reduceIte, beq_iff_eq] at ho
                    simp only [hj, ↓reduceIte]
                    exact congrArg (· * x' j) ho.symm
                rw [hsum_other]
                have hcomm1 :
                    objC + (finSum n (fun j => if j = cFin then 0 else obj' j * x' j) + obj cFin * val) =
                    (objC + obj cFin * val) + finSum n (fun j => if j = cFin then 0 else obj' j * x' j) := by
                  rw [Rat.add_comm (finSum n (fun j => if j = cFin then 0 else obj' j * x' j)) (obj cFin * val),
                      ← Rat.add_assoc]
                have hcomm2 :
                    objC' + (finSum n (fun j => if j = cFin then 0 else obj' j * x' j) + obj' cFin * val) =
                    (objC' + obj' cFin * val) + finSum n (fun j => if j = cFin then 0 else obj' j * x' j) := by
                  rw [Rat.add_comm (finSum n (fun j => if j = cFin then 0 else obj' j * x' j)) (obj' cFin * val),
                      ← Rat.add_assoc]
                rw [hcomm1, hcomm2, hobj_const]
            · simp [hCol] at hstep
          · simp [hRows] at hstep
        | singletonRow origRow col origRowIdx =>
          by_cases hRow : origRow < m
          · by_cases hCol : col < n
            · simp only [hRow, hCol, ↓reduceDIte, Bool.and_eq_true] at hstep
              rcases hstep with ⟨⟨⟨⟨⟨⟨ha_ne_b, hsng_other_b⟩, hsub⟩, hobjC_b⟩, hobj_b⟩, hbnds_other_b⟩, hsng_bnd⟩
              let rFin : Fin m := ⟨origRow, hRow⟩
              let cFin : Fin n := ⟨col, hCol⟩
              let LP_red : LPInstance := ⟨n, m', objC', obj', sense', rows', rhs', lb', ub'⟩
              have ha_ne : rowCoeff LP0 rFin cFin ≠ 0 := bne_iff_ne.mp ha_ne_b
              have hsng_other : ∀ j : Fin n, j ≠ cFin → rowCoeff LP0 rFin j = 0 := by
                intro j hj
                have ho := (allFin3B_iff.mp hsng_other_b) j
                simp [cFin, hj] at ho
                exact ho
              rcases checkRowSubmap_props (LP := LP0) (LP' := LP_red) (hVars := rfl) hsub with
                ⟨_, mapFn, _, _, _, hsurj, hsense_map, hrhs_map, hrc_map⟩
              have hobjC_eq : objC' = objC := beq_iff_eq.mp hobjC_b
              have hobj_eq : ∀ j : Fin n, obj' j = obj j := fun j =>
                beq_iff_eq.mp ((allFin3B_iff.mp hobj_b) j)
              have hbnds_other : ∀ j : Fin n, j ≠ cFin → lb' j = lb j ∧ ub' j = ub j := by
                intro j hj
                have ho := (allFin3B_iff.mp hbnds_other_b) j
                simp [cFin, hj] at ho
                exact ho
              let a := rowCoeff LP0 rFin cFin
              let b := rhs rFin
              let imp := b / a
              have hact_sng : rowActivity LP0 x' rFin = a * x' cFin := by
                rw [rowActivity_eq_finSum LP0 x' rFin, finSum_split_at _ cFin]
                have hz : finSum n (fun j => if j = cFin then 0 else rowCoeff LP0 rFin j * x' j) = 0 := by
                  have hc : ∀ j : Fin n,
                      (if j = cFin then 0 else rowCoeff LP0 rFin j * x' j) = 0 := by
                    intro j
                    by_cases hj : j = cFin
                    · simp [hj]
                    · simp [hj, hsng_other j hj, Rat.zero_mul]
                  rw [finSum_congr hc, finSum_zero]
                rw [hz, Rat.zero_add]
              have hcBnd_and_sngRow :
                  SatisfiesVarBounds (lb cFin) (ub cFin) (x' cFin) ∧
                  SatisfiesRow (sense rFin) (rowActivity LP0 x' rFin) (rhs rFin) := by
                have hf_c := hfeas'.1 cFin
                change SatisfiesVarBounds (lb' cFin) (ub' cFin) (x' cFin) at hf_c
                have ha_mul_imp : a * imp = b := by
                  dsimp only [imp]
                  rw [Rat.mul_comm a (b / a), Rat.div_mul_cancel ha_ne]
                cases hs_r : sense rFin with
                | eq =>
                  dsimp only [rFin] at hs_r
                  rw [hs_r] at hsng_bnd
                  simp only [Bool.and_eq_true, beq_iff_eq] at hsng_bnd
                  rcases hsng_bnd with ⟨⟨himp_vb, hlb'_eq⟩, hub'_eq⟩
                  have himp_ok : SatisfiesVarBounds (lb cFin) (ub cFin) imp :=
                    checkVarBounds_iff.mp himp_vb
                  unfold SatisfiesVarBounds at hf_c
                  rw [hlb'_eq, hub'_eq] at hf_c
                  have hx_eq : x' cFin = imp := Rat.le_antisymm hf_c.2 hf_c.1
                  refine ⟨hx_eq ▸ himp_ok, ?_⟩
                  rw [hact_sng, hx_eq, ha_mul_imp]
                  rfl
                | le =>
                  dsimp only [rFin] at hs_r
                  rw [hs_r] at hsng_bnd
                  dsimp only at hsng_bnd
                  by_cases hpos : 0 < a
                  · have hpos' : 0 < rowCoeff LP0 ⟨origRow, hRow⟩ ⟨col, hCol⟩ := hpos
                    dsimp only [LP0] at hpos'
                    simp only [hpos', ↓reduceIte, Bool.and_eq_true, beq_iff_eq] at hsng_bnd
                    rcases hsng_bnd with ⟨⟨himp_vb, hlb'_eq⟩, hub'_eq⟩
                    have himp_ok : SatisfiesVarBounds (lb cFin) (ub cFin) imp :=
                      checkVarBounds_iff.mp himp_vb
                    rw [hlb'_eq, hub'_eq] at hf_c
                    refine ⟨varBounds_of_imp_upper himp_ok hf_c, ?_⟩
                    rw [hact_sng]
                    unfold SatisfiesRow
                    have hm := Rat.mul_le_mul_of_nonneg_left hf_c.2 (Rat.le_of_lt hpos)
                    rwa [ha_mul_imp] at hm
                  · have hpos' : ¬(0 < rowCoeff LP0 ⟨origRow, hRow⟩ ⟨col, hCol⟩) := hpos
                    dsimp only [LP0] at hpos'
                    simp only [hpos', ↓reduceIte, Bool.and_eq_true, beq_iff_eq] at hsng_bnd
                    rcases hsng_bnd with ⟨⟨himp_vb, hlb'_eq⟩, hub'_eq⟩
                    have himp_ok : SatisfiesVarBounds (lb cFin) (ub cFin) imp :=
                      checkVarBounds_iff.mp himp_vb
                    rw [hlb'_eq, hub'_eq] at hf_c
                    have ha_le : a ≤ 0 := Rat.not_lt.mp hpos
                    refine ⟨varBounds_of_imp_lower himp_ok hf_c, ?_⟩
                    rw [hact_sng]
                    unfold SatisfiesRow
                    have hm := rat_mul_le_mul_of_nonpos_left hf_c.1 ha_le
                    rwa [ha_mul_imp] at hm
                | ge =>
                  dsimp only [rFin] at hs_r
                  rw [hs_r] at hsng_bnd
                  dsimp only at hsng_bnd
                  by_cases hpos : 0 < a
                  · have hpos' : 0 < rowCoeff LP0 ⟨origRow, hRow⟩ ⟨col, hCol⟩ := hpos
                    dsimp only [LP0] at hpos'
                    simp only [hpos', ↓reduceIte, Bool.and_eq_true, beq_iff_eq] at hsng_bnd
                    rcases hsng_bnd with ⟨⟨himp_vb, hlb'_eq⟩, hub'_eq⟩
                    have himp_ok : SatisfiesVarBounds (lb cFin) (ub cFin) imp :=
                      checkVarBounds_iff.mp himp_vb
                    rw [hlb'_eq, hub'_eq] at hf_c
                    refine ⟨varBounds_of_imp_lower himp_ok hf_c, ?_⟩
                    rw [hact_sng]
                    unfold SatisfiesRow
                    have hm := Rat.mul_le_mul_of_nonneg_left hf_c.1 (Rat.le_of_lt hpos)
                    rwa [ha_mul_imp] at hm
                  · have hpos' : ¬(0 < rowCoeff LP0 ⟨origRow, hRow⟩ ⟨col, hCol⟩) := hpos
                    dsimp only [LP0] at hpos'
                    simp only [hpos', ↓reduceIte, Bool.and_eq_true, beq_iff_eq] at hsng_bnd
                    rcases hsng_bnd with ⟨⟨himp_vb, hlb'_eq⟩, hub'_eq⟩
                    have himp_ok : SatisfiesVarBounds (lb cFin) (ub cFin) imp :=
                      checkVarBounds_iff.mp himp_vb
                    rw [hlb'_eq, hub'_eq] at hf_c
                    have ha_le : a ≤ 0 := Rat.not_lt.mp hpos
                    refine ⟨varBounds_of_imp_upper himp_ok hf_c, ?_⟩
                    rw [hact_sng]
                    unfold SatisfiesRow
                    have hm := rat_mul_le_mul_of_nonpos_left hf_c.2 ha_le
                    rwa [ha_mul_imp] at hm
              refine ⟨⟨?_, ?_⟩, ?_⟩
              · intro j
                by_cases hj : j = cFin
                · subst hj; exact hcBnd_and_sngRow.1
                · rcases hbnds_other j hj with ⟨hl_eq, hu_eq⟩
                  have hfj := hfeas'.1 j
                  simpa [hl_eq, hu_eq] using hfj
              · intro i
                by_cases hi : i = rFin
                · subst hi; exact hcBnd_and_sngRow.2
                · rcases hsurj i hi with ⟨q, hmap_q⟩
                  have hfq := hfeas'.2 q
                  change SatisfiesRow (LP_red.sense q) (rowActivity LP_red x' q) (LP_red.rhs q) at hfq
                  have hact_eq : rowActivity LP_red x' q = rowActivity LP0 x' i := by
                    rw [rowActivity_eq_finSum LP_red x' q, rowActivity_eq_finSum LP0 x' i]
                    apply finSum_congr
                    intro j
                    have hrc := hrc_map q j
                    rw [hmap_q] at hrc
                    simp [hrc]
                  have hs_eq := hsense_map q
                  have hr_eq := hrhs_map q
                  rw [hmap_q] at hs_eq hr_eq
                  rwa [hs_eq, hr_eq, hact_eq] at hfq
              · unfold LPObjective
                dsimp only [LP0]
                rw [hobjC_eq]
                apply congrArg (objC + ·)
                exact finSum_congr (fun j => congrArg (· * x' j) (hobj_eq j).symm)
            · simp [hRow, hCol] at hstep
          · simp [hRow] at hstep
        | dualColFix col toLower =>
          by_cases hRows : m' = m
          · subst m'
            by_cases hCol : col < n
            · simp only [hCol, ↓reduceDIte, Bool.and_eq_true] at hstep
              rcases hstep with ⟨⟨⟨⟨hobjC_b, hobj_b⟩, hbnds_other_b⟩, hrows_b⟩, hcol_fix⟩
              let cFin : Fin n := ⟨col, hCol⟩
              let LP_red : LPInstance := ⟨n, m, objC', obj', sense', rows', rhs', lb', ub'⟩
              have hobjC_eq : objC' = objC := beq_iff_eq.mp hobjC_b
              have hobj_eq : ∀ j : Fin n, obj' j = obj j := fun j =>
                beq_iff_eq.mp ((allFin3B_iff.mp hobj_b) j)
              have hbnds_other : ∀ j : Fin n, j ≠ cFin → lb' j = lb j ∧ ub' j = ub j := by
                intro j hj
                have ho := (allFin3B_iff.mp hbnds_other_b) j
                simp [cFin, hj] at ho
                exact ho
              have hcBnd : SatisfiesVarBounds (lb cFin) (ub cFin) (x' cFin) := by
                have hf_c := hfeas'.1 cFin
                change SatisfiesVarBounds (lb' cFin) (ub' cFin) (x' cFin) at hf_c
                cases toLower with
                | true =>
                  simp only [ite_true] at hcol_fix
                  cases hlb_c : lb cFin with
                  | none =>
                    dsimp only [cFin] at hlb_c
                    simp [hlb_c] at hcol_fix
                  | some l =>
                    dsimp only [cFin] at hlb_c
                    rw [hlb_c] at hcol_fix
                    simp only [Bool.and_eq_true, beq_iff_eq] at hcol_fix
                    rcases hcol_fix with ⟨⟨⟨⟨hl_vb, hlb'_eq⟩, hub'_eq⟩, _⟩, _⟩
                    unfold SatisfiesVarBounds at hf_c
                    rw [hlb'_eq, hub'_eq] at hf_c
                    have hx_eq : x' cFin = l := Rat.le_antisymm hf_c.2 hf_c.1
                    have hl_ok : SatisfiesVarBounds (some l) (ub cFin) l :=
                      checkVarBounds_iff.mp hl_vb
                    rwa [hx_eq]
                | false =>
                  simp only [Bool.false_eq_true, ↓reduceIte] at hcol_fix
                  cases hub_c : ub cFin with
                  | none =>
                    dsimp only [cFin] at hub_c
                    simp [hub_c] at hcol_fix
                  | some u =>
                    dsimp only [cFin] at hub_c
                    rw [hub_c] at hcol_fix
                    simp only [Bool.and_eq_true, beq_iff_eq] at hcol_fix
                    rcases hcol_fix with ⟨⟨⟨⟨hu_vb, hlb'_eq⟩, hub'_eq⟩, _⟩, _⟩
                    unfold SatisfiesVarBounds at hf_c
                    rw [hlb'_eq, hub'_eq] at hf_c
                    have hx_eq : x' cFin = u := Rat.le_antisymm hf_c.2 hf_c.1
                    have hu_ok : SatisfiesVarBounds (lb cFin) (some u) u :=
                      checkVarBounds_iff.mp hu_vb
                    rwa [hx_eq]
              refine ⟨⟨?_, ?_⟩, ?_⟩
              · intro j
                by_cases hj : j = cFin
                · subst hj; exact hcBnd
                · rcases hbnds_other j hj with ⟨hl_eq, hu_eq⟩
                  simpa [hl_eq, hu_eq] using hfeas'.1 j
              · intro i
                have hri := (allFin3B_iff.mp hrows_b) i
                simp only [Bool.and_eq_true, allFin3B_iff] at hri
                rcases hri with ⟨⟨hs_b, hrhs_b⟩, hrc_all⟩
                have hs_eq : sense' i = sense i := constraintSense_beq_iff.mp hs_b
                have hrhs_eq : rhs' i = rhs i := beq_iff_eq.mp hrhs_b
                have hact_eq : rowActivity LP_red x' i = rowActivity LP0 x' i := by
                  rw [rowActivity_eq_finSum LP_red x' i, rowActivity_eq_finSum LP0 x' i]
                  exact finSum_congr (fun j => congrArg (· * x' j) (beq_iff_eq.mp (hrc_all j)))
                have hfi := hfeas'.2 i
                change SatisfiesRow (sense' i) (rowActivity LP_red x' i) (rhs' i) at hfi
                change SatisfiesRow (sense i) (rowActivity LP0 x' i) (rhs i)
                rwa [hs_eq, hrhs_eq, hact_eq] at hfi
              · unfold LPObjective
                dsimp only [LP0]
                rw [hobjC_eq]
                apply congrArg (objC + ·)
                exact finSum_congr (fun j => congrArg (· * x' j) (hobj_eq j).symm)
            · simp [hCol] at hstep
          · simp [hRows] at hstep
        | zeroObjEmptyColFix col val =>
          by_cases hRows : m' = m
          · subst m'
            by_cases hCol : col < n
            · simp only [hCol, ↓reduceDIte, Bool.and_eq_true] at hstep
              rcases hstep with ⟨⟨⟨⟨⟨⟨⟨hval_vb, _⟩, hlb'_b⟩, hub'_b⟩, hobjC_b⟩, hobj_b⟩, hbnds_other_b⟩, hrows_b⟩
              let cFin : Fin n := ⟨col, hCol⟩
              let LP_red : LPInstance := ⟨n, m, objC', obj', sense', rows', rhs', lb', ub'⟩
              have hval_ok : SatisfiesVarBounds (lb cFin) (ub cFin) val :=
                checkVarBounds_iff.mp hval_vb
              have hlb'_eq : lb' cFin = some val := beq_iff_eq.mp hlb'_b
              have hub'_eq : ub' cFin = some val := beq_iff_eq.mp hub'_b
              have hobjC_eq : objC' = objC := beq_iff_eq.mp hobjC_b
              have hobj_eq : ∀ j : Fin n, obj' j = obj j := fun j =>
                beq_iff_eq.mp ((allFin3B_iff.mp hobj_b) j)
              have hbnds_other : ∀ j : Fin n, j ≠ cFin → lb' j = lb j ∧ ub' j = ub j := by
                intro j hj
                have ho := (allFin3B_iff.mp hbnds_other_b) j
                simp [cFin, hj] at ho
                exact ho
              refine ⟨⟨?_, ?_⟩, ?_⟩
              · intro j
                by_cases hj : j = cFin
                · subst hj
                  have hf_c := hfeas'.1 cFin
                  change SatisfiesVarBounds (lb' cFin) (ub' cFin) (x' cFin) at hf_c
                  unfold SatisfiesVarBounds at hf_c
                  rw [hlb'_eq, hub'_eq] at hf_c
                  have hx_eq : x' cFin = val := Rat.le_antisymm hf_c.2 hf_c.1
                  rwa [hx_eq]
                · rcases hbnds_other j hj with ⟨hl_eq, hu_eq⟩
                  simpa [hl_eq, hu_eq] using hfeas'.1 j
              · intro i
                have hri := (allFin3B_iff.mp hrows_b) i
                simp only [Bool.and_eq_true, allFin3B_iff] at hri
                rcases hri with ⟨⟨⟨_, hs_b⟩, hrhs_b⟩, hrc_all⟩
                have hs_eq : sense' i = sense i := constraintSense_beq_iff.mp hs_b
                have hrhs_eq : rhs' i = rhs i := beq_iff_eq.mp hrhs_b
                have hact_eq : rowActivity LP_red x' i = rowActivity LP0 x' i := by
                  rw [rowActivity_eq_finSum LP_red x' i, rowActivity_eq_finSum LP0 x' i]
                  exact finSum_congr (fun j => congrArg (· * x' j) (beq_iff_eq.mp (hrc_all j)))
                have hfi := hfeas'.2 i
                change SatisfiesRow (sense' i) (rowActivity LP_red x' i) (rhs' i) at hfi
                change SatisfiesRow (sense i) (rowActivity LP0 x' i) (rhs i)
                rwa [hs_eq, hrhs_eq, hact_eq] at hfi
              · unfold LPObjective
                dsimp only [LP0]
                rw [hobjC_eq]
                apply congrArg (objC + ·)
                exact finSum_congr (fun j => congrArg (· * x' j) (hobj_eq j).symm)
            · simp [hCol] at hstep
          · simp [hRows] at hstep
        | dropRedundantRow droppedRow origRowIdx =>
          by_cases hDrop : droppedRow < m
          · simp only [hDrop, ↓reduceDIte, Bool.and_eq_true] at hstep
            rcases hstep with ⟨⟨⟨hsub, hobjC_b⟩, hvars_b⟩, hred_row⟩
            let dFin : Fin m := ⟨droppedRow, hDrop⟩
            let LP_red : LPInstance := ⟨n, m', objC', obj', sense', rows', rhs', lb', ub'⟩
            rcases checkRowSubmap_props (LP := LP0) (LP' := LP_red) (hVars := rfl) hsub with
              ⟨_, mapFn, _, _, _, hsurj, hsense_map, hrhs_map, hrc_map⟩
            have hobjC_eq : objC' = objC := beq_iff_eq.mp hobjC_b
            have hvars_all := allFin3B_iff.mp hvars_b
            have hbnds_orig : ∀ j : Fin n, SatisfiesVarBounds (lb j) (ub j) (x' j) := by
              intro j
              have hj := hvars_all j
              rw [Bool.and_eq_true, Bool.and_eq_true, beq_iff_eq, beq_iff_eq, beq_iff_eq] at hj
              simpa [hj.1.2, hj.2] using hfeas'.1 j
            refine ⟨⟨hbnds_orig, ?_⟩, ?_⟩
            · intro i
              by_cases hi : i = dFin
              · subst hi
                rw [rowActivity_eq_finSum LP0 x' dFin]
                change SatisfiesRow (sense dFin)
                  (finSum n fun j => rowCoeff LP0 dFin j * x' j) (rhs dFin)
                cases hs_d : sense dFin with
                | eq =>
                  dsimp only [dFin] at hs_d
                  rw [hs_d] at hred_row
                  simp only [Bool.and_eq_true, allFin3B_iff, beq_iff_eq] at hred_row
                  rcases hred_row with ⟨hrc0, hrhs0⟩
                  unfold SatisfiesRow
                  have hz : finSum n (fun j => rowCoeff LP0 dFin j * x' j) = 0 := by
                    rw [finSum_congr (fun j => by rw [hrc0 j, Rat.zero_mul]), finSum_zero]
                  rw [hz, hrhs0]
                | le =>
                  dsimp only [dFin] at hs_d
                  rw [hs_d] at hred_row
                  dsimp only [SatisfiesRow] at hred_row ⊢
                  cases hsum : sumOptFin n (fun j => farkasTermUpper (rowCoeff LP0 dFin j) (lb j) (ub j)) with
                  | none =>
                    change (match sumOptFin n (fun j => farkasTermUpper (rowCoeff LP0 dFin j) (lb j) (ub j)) with
                      | none => false | some maxAct => decide (maxAct ≤ rhs dFin)) = true at hred_row
                    rw [hsum] at hred_row
                    cases hred_row
                  | some maxAct =>
                    change (match sumOptFin n (fun j => farkasTermUpper (rowCoeff LP0 dFin j) (lb j) (ub j)) with
                      | none => false | some maxAct => decide (maxAct ≤ rhs dFin)) = true at hred_row
                    rw [hsum, decide_eq_true_eq] at hred_row
                    have hact_le : finSum n (fun j => rowCoeff LP0 dFin j * x' j) ≤ maxAct :=
                      finSum_le_sumOptFin (fun j b hb => farkasTermUpper_ge (hbnds_orig j) hb) hsum
                    exact Rat.le_trans hact_le hred_row
                | ge =>
                  dsimp only [dFin] at hs_d
                  rw [hs_d] at hred_row
                  dsimp only [SatisfiesRow] at hred_row ⊢
                  cases hsum : sumOptFin n (fun j => rcTermBound (rowCoeff LP0 dFin j) (lb j) (ub j)) with
                  | none =>
                    change (match sumOptFin n (fun j => rcTermBound (rowCoeff LP0 dFin j) (lb j) (ub j)) with
                      | none => false | some minAct => decide (rhs dFin ≤ minAct)) = true at hred_row
                    rw [hsum] at hred_row
                    cases hred_row
                  | some minAct =>
                    change (match sumOptFin n (fun j => rcTermBound (rowCoeff LP0 dFin j) (lb j) (ub j)) with
                      | none => false | some minAct => decide (rhs dFin ≤ minAct)) = true at hred_row
                    rw [hsum, decide_eq_true_eq] at hred_row
                    have hact_ge : minAct ≤ finSum n (fun j => rowCoeff LP0 dFin j * x' j) :=
                      sumOptFin_le_finSum (fun j b hb => rcTermBound_le (hbnds_orig j) hb) hsum
                    exact Rat.le_trans hred_row hact_ge
              · rcases hsurj i hi with ⟨q, hmap_q⟩
                have hfq := hfeas'.2 q
                change SatisfiesRow (LP_red.sense q) (rowActivity LP_red x' q) (LP_red.rhs q) at hfq
                have hact_eq : rowActivity LP_red x' q = rowActivity LP0 x' i := by
                  rw [rowActivity_eq_finSum LP_red x' q, rowActivity_eq_finSum LP0 x' i]
                  apply finSum_congr
                  intro j
                  have hrc := hrc_map q j
                  rw [hmap_q] at hrc
                  simp [hrc]
                have hs_eq := hsense_map q
                have hr_eq := hrhs_map q
                rw [hmap_q] at hs_eq hr_eq
                rwa [hs_eq, hr_eq, hact_eq] at hfq
            · unfold LPObjective
              dsimp only [LP0]
              rw [hobjC_eq]
              apply congrArg (objC + ·)
              apply finSum_congr
              intro j
              have hj := hvars_all j
              rw [Bool.and_eq_true, Bool.and_eq_true, beq_iff_eq] at hj
              exact congrArg (· * x' j) hj.1.1.symm
          · simp [hDrop] at hstep
      · dsimp only [LP0, LP1] at hstep
        simp [hVars] at hstep

private theorem rat_sub_add_eq_sub_sub (a b c : Rat) : a - (b + c) = a - b - c := by
  simp only [Rat.sub_eq_add_neg, Rat.neg_add, ← Rat.add_assoc]

private theorem rat_rearr_sng (a b x s : Rat) :
    a + (b + x) + (s + 0) = a + b + (s + x) := by
  rw [Rat.add_zero]
  calc
    a + (b + x) + s = a + ((b + x) + s) := Rat.add_assoc a (b + x) s
    _ = a + (b + (x + s)) := by rw [Rat.add_assoc b x s]
    _ = a + (b + (s + x)) := by rw [Rat.add_comm x s]
    _ = a + b + (s + x) := (Rat.add_assoc a b (s + x)).symm

private theorem rat_rearr_fixed_lhs (a b c d e : Rat) :
    a + b + (c + (d - e)) = (a + d) + b + c - e := by
  simp only [Rat.sub_eq_add_neg]
  calc
    a + b + (c + (d + -e)) = (a + b + (c + d)) + -e := by
      rw [← Rat.add_assoc c d (-e), ← Rat.add_assoc (a + b) (c + d) (-e)]
    _ = (a + ((b + c) + d)) + -e := by
      rw [Rat.add_assoc a b (c + d), ← Rat.add_assoc b c d]
    _ = (a + (d + (b + c))) + -e := by rw [Rat.add_comm (b + c) d]
    _ = (a + d + b + c) + -e := by
      rw [← Rat.add_assoc a d (b + c), ← Rat.add_assoc (a + d) b c]

private theorem rat_rearr_fixed_rhs (a b c d e : Rat) :
    a + (b - e) + (c + d) = (a + d) + b + c - e := by
  have h1 : a + (b - e) + (c + d) = a + b + (c + (d - e)) := by
    simp only [Rat.sub_eq_add_neg]
    calc
      a + (b + -e) + (c + d) = a + ((b + -e) + (c + d)) := Rat.add_assoc a _ _
      _ = a + (b + (-e + (c + d))) := by rw [Rat.add_assoc b (-e) (c + d)]
      _ = a + (b + ((c + d) + -e)) := by rw [Rat.add_comm (-e) (c + d)]
      _ = a + b + (c + (d + -e)) := by
        rw [Rat.add_assoc c d (-e), ← Rat.add_assoc a b (c + (d + -e))]
  rw [h1]
  exact rat_rearr_fixed_lhs a b c d e

@[simp] private theorem constraintSense_beq_eq_eq : (ConstraintSense.eq == .eq) = true := rfl
@[simp] private theorem constraintSense_beq_le_eq : (ConstraintSense.le == .eq) = false := rfl
@[simp] private theorem constraintSense_beq_ge_eq : (ConstraintSense.ge == .eq) = false := rfl
@[simp] private theorem constraintSense_beq_le_le : (ConstraintSense.le == .le) = true := rfl
@[simp] private theorem constraintSense_beq_le_ge : (ConstraintSense.le == .ge) = false := rfl
@[simp] private theorem constraintSense_beq_ge_le : (ConstraintSense.ge == .le) = false := rfl
@[simp] private theorem constraintSense_beq_ge_ge : (ConstraintSense.ge == .ge) = true := rfl

private theorem finSum_nonpos {n : Nat} {f : Fin n → Rat} (h : ∀ i, f i ≤ 0) :
    finSum n f ≤ 0 := by
  have h0 : finSum n f ≤ finSum n (fun _ => (0 : Rat)) := finSum_le_finSum h
  simpa [finSum_zero] using h0

private theorem rcTermBound_zero (lb ub : Option Rat) :
    rcTermBound 0 lb ub = some 0 := by
  unfold rcTermBound
  simp

private theorem rcTermBound_nonneg_ub_irrel {r : Rat} (hr : 0 ≤ r) (lb ub1 ub2 : Option Rat) :
    rcTermBound r lb ub1 = rcTermBound r lb ub2 := by
  unfold rcTermBound
  by_cases hpos : 0 < r
  · simp [hpos]
  · have hnotneg : ¬(r < 0) := Rat.not_lt.mpr hr
    simp [hpos, hnotneg]

private theorem rcTermBound_nonpos_lb_irrel {r : Rat} (hr : r ≤ 0) (lb1 lb2 ub : Option Rat) :
    rcTermBound r lb1 ub = rcTermBound r lb2 ub := by
  unfold rcTermBound
  have hnotpos : ¬(0 < r) := Rat.not_lt.mpr hr
  by_cases hneg : r < 0
  · simp [hnotpos, hneg]
  · simp [hnotpos, hneg]

private theorem rcTermBound_of_neg {r u : Rat} (hr : r < 0) (lb : Option Rat) :
    rcTermBound r lb (some u) = some (r * u) := by
  unfold rcTermBound
  have hnotpos : ¬(0 < r) := Rat.not_lt.mpr (Rat.le_of_lt hr)
  simp [hnotpos, hr]

private theorem rcTermBound_of_pos {r l : Rat} (hr : 0 < r) (ub : Option Rat) :
    rcTermBound r (some l) ub = some (r * l) := by
  unfold rcTermBound
  simp [hr]

private theorem projDual_le_of_nonpos {y : Rat} (hy : y ≤ 0) : projDual .le y = y := by
  unfold projDual
  simp [hy]

private theorem projDual_ge_of_nonneg {y : Rat} (hy : 0 ≤ y) : projDual .ge y = y := by
  unfold projDual
  simp [hy]

private theorem rat_inv_nonpos_of_neg {a : Rat} (ha : a < 0) : a⁻¹ ≤ 0 := by
  by_cases h : a⁻¹ ≤ 0
  · exact h
  · have hpos : 0 ≤ a⁻¹ := Rat.le_of_lt (Rat.not_le.mp h)
    have hm := rat_mul_nonpos_of_nonpos_of_nonneg (Rat.le_of_lt ha) hpos
    rw [Rat.mul_inv_cancel a (Rat.ne_of_lt ha)] at hm
    exact False.elim ((by decide : ¬((1 : Rat) ≤ 0)) hm)

private theorem rat_div_nonpos_of_nonpos_of_pos {r a : Rat} (hr : r ≤ 0) (ha : 0 < a) :
    r / a ≤ 0 :=
  (rat_div_le_iff ha).mpr (by rw [Rat.zero_mul]; exact hr)

private theorem rat_div_nonneg_of_nonneg_of_pos {r a : Rat} (hr : 0 ≤ r) (ha : 0 < a) :
    0 ≤ r / a :=
  (rat_le_div_iff ha).mpr (by rw [Rat.zero_mul]; exact hr)

private theorem rat_div_nonpos_of_nonneg_of_neg {r a : Rat} (hr : 0 ≤ r) (ha : a < 0) :
    r / a ≤ 0 := by
  rw [Rat.div_def]
  exact rat_mul_nonpos_of_nonneg_of_nonpos hr (rat_inv_nonpos_of_neg ha)

private theorem rat_div_nonneg_of_nonpos_of_neg {r a : Rat} (hr : r ≤ 0) (ha : a < 0) :
    0 ≤ r / a := by
  rw [Rat.div_def]
  exact rat_mul_nonneg_of_nonpos_of_nonpos hr (rat_inv_nonpos_of_neg ha)

private def submapDualVec (LP' : LPInstance) (origRowIdx : Array Nat)
    (yRed : Fin LP'.numRows → Rat) (i : Nat) : Rat :=
  match findReducedRow? LP'.numRows origRowIdx i with
  | some q => projDual (LP'.sense q) (yRed q)
  | none => 0

private theorem rowSubmap_yBase_props {LP LP' : LPInstance} {hVars : LP'.numVars = LP.numVars}
    {skipRow : Fin LP.numRows} {origRowIdx : Array Nat}
    (hsub : checkRowSubmap LP LP' hVars skipRow origRowIdx = true)
    (yRed : Fin LP'.numRows → Rat) :
    ∃ mapFn : Fin LP'.numRows → Fin LP.numRows,
      submapDualVec LP' origRowIdx yRed skipRow.val = 0 ∧
      (∀ i : Fin LP.numRows,
        projDual (LP.sense i) (submapDualVec LP' origRowIdx yRed i.val) =
        submapDualVec LP' origRowIdx yRed i.val) ∧
      (∀ q : Fin LP'.numRows, LP'.rhs q = LP.rhs (mapFn q)) ∧
      (∀ q : Fin LP'.numRows, ∀ j : Fin LP.numVars,
        rowCoeff LP' q ⟨j.val, by rw [hVars]; exact j.isLt⟩ = rowCoeff LP (mapFn q) j) ∧
      (∀ g : Fin LP.numRows → Rat,
        finSum LP.numRows (fun i => submapDualVec LP' origRowIdx yRed i.val * g i) =
        finSum LP'.numRows (fun q => projDual (LP'.sense q) (yRed q) * g (mapFn q))) := by
  rcases checkRowSubmap_props hsub with
    ⟨hSz, mapFn, hval, hne_skip, hinj, _, hsense_map, hrhs_map, hrc_map⟩
  let lookup : Fin LP.numRows → Option (Fin LP'.numRows) :=
    fun i => findReducedRow? LP'.numRows origRowIdx i.val
  have hinj_val : ∀ q1 q2 : Fin LP'.numRows,
      origRowIdx[q1.val]'(by rw [hSz]; exact q1.isLt) =
      origRowIdx[q2.val]'(by rw [hSz]; exact q2.isLt) → q1 = q2 := by
    intro q1 q2 hv
    apply hinj
    apply Fin.ext
    rw [hval q1, hval q2, hv]
  have hlookup_of_map : ∀ q : Fin LP'.numRows, lookup (mapFn q) = some q := by
    intro q
    dsimp only [lookup]
    rw [hval q]
    exact findReducedRow_some_of_mem hSz hinj_val q
  have hmap_of_lookup : ∀ (i : Fin LP.numRows) (q : Fin LP'.numRows),
      lookup i = some q → mapFn q = i := by
    intro i q hl
    apply Fin.ext
    rw [hval q]
    exact findReducedRow_loop_some hSz LP'.numRows 0 (by omega) q hl
  have hlookup_skip : lookup skipRow = none := by
    apply findReducedRow_none_of_forall_ne hSz
    intro q heq
    apply hne_skip q
    apply Fin.ext
    rw [hval q, heq]
  refine ⟨mapFn, ?_, ?_, hrhs_map, hrc_map, ?_⟩
  · unfold submapDualVec
    change (match lookup skipRow with | some q => projDual (LP'.sense q) (yRed q) | none => 0) = 0
    rw [hlookup_skip]
  · intro i
    unfold submapDualVec
    cases hl : findReducedRow? LP'.numRows origRowIdx i.val with
    | none => exact projDual_zero (LP.sense i)
    | some q =>
      have hm : mapFn q = i := hmap_of_lookup i q hl
      rw [← hm, ← hsense_map q]
      exact projDual_idem (LP'.sense q) (yRed q)
  · intro g
    have hpoint :
        ∀ i : Fin LP.numRows,
          submapDualVec LP' origRowIdx yRed i.val * g i =
            lookupValOrZero (lookup i) (fun q => projDual (LP'.sense q) (yRed q) * g (mapFn q)) := by
      intro i
      unfold submapDualVec
      dsimp only [lookup, lookupValOrZero]
      cases hl : findReducedRow? LP'.numRows origRowIdx i.val with
      | none => simp [Rat.zero_mul]
      | some q =>
        have hm : mapFn q = i := hmap_of_lookup i q hl
        dsimp only
        rw [hm]
    rw [finSum_congr hpoint]
    exact finSum_lookup_bijection mapFn lookup
      hlookup_of_map hmap_of_lookup (fun q => projDual (LP'.sense q) (yRed q) * g (mapFn q))

private theorem singleton_ySing_proj (sngSense : ConstraintSense) (a rcBase : Rat)
    (ha_ne : a ≠ 0) :
    let isUpper := (sngSense == .le && decide (0 < a)) || (sngSense == .ge && decide (a < 0))
    let ySing : Rat :=
      if sngSense == .eq then
        rcBase / a
      else if isUpper then
        if rcBase < 0 then rcBase / a else 0
      else
        if 0 < rcBase then rcBase / a else 0
    projDual sngSense ySing = ySing := by
  dsimp only
  cases sngSense with
  | eq => rfl
  | le =>
    by_cases hpos : 0 < a
    · simp only [constraintSense_beq_le_eq, constraintSense_beq_le_le, constraintSense_beq_le_ge,
        hpos, decide_true, Bool.and_self, Bool.true_or, Bool.false_eq_true, ↓reduceIte]
      by_cases hrc : rcBase < 0
      · simp only [hrc, ↓reduceIte]
        exact projDual_le_of_nonpos (rat_div_nonpos_of_nonpos_of_pos (Rat.le_of_lt hrc) hpos)
      · simp only [hrc, ↓reduceIte, projDual_zero]
    · have hneg : a < 0 := Rat.lt_of_le_of_ne (Rat.not_lt.mp hpos) ha_ne
      simp only [constraintSense_beq_le_eq, constraintSense_beq_le_le, constraintSense_beq_le_ge,
        hpos, decide_false, Bool.and_false, Bool.false_and, Bool.false_or, Bool.false_eq_true, ↓reduceIte]
      by_cases hrc : 0 < rcBase
      · simp only [hrc, ↓reduceIte]
        exact projDual_le_of_nonpos (rat_div_nonpos_of_nonneg_of_neg (Rat.le_of_lt hrc) hneg)
      · simp only [hrc, ↓reduceIte, projDual_zero]
  | ge =>
    by_cases hpos : 0 < a
    · have hnotneg : ¬(a < 0) := Rat.not_lt.mpr (Rat.le_of_lt hpos)
      simp only [constraintSense_beq_ge_eq, constraintSense_beq_ge_le, constraintSense_beq_ge_ge,
        hnotneg, decide_false, Bool.and_false, Bool.false_and, Bool.or_false, Bool.false_eq_true, ↓reduceIte]
      by_cases hrc : 0 < rcBase
      · simp only [hrc, ↓reduceIte]
        exact projDual_ge_of_nonneg (rat_div_nonneg_of_nonneg_of_pos (Rat.le_of_lt hrc) hpos)
      · simp only [hrc, ↓reduceIte, projDual_zero]
    · have hneg : a < 0 := Rat.lt_of_le_of_ne (Rat.not_lt.mp hpos) ha_ne
      simp only [constraintSense_beq_ge_eq, constraintSense_beq_ge_le, constraintSense_beq_ge_ge,
        hneg, decide_true, Bool.and_self, Bool.or_true, Bool.false_eq_true, ↓reduceIte]
      by_cases hrc : rcBase < 0
      · simp only [hrc, ↓reduceIte]
        exact projDual_ge_of_nonneg (rat_div_nonneg_of_nonpos_of_neg (Rat.le_of_lt hrc) hneg)
      · simp only [hrc, ↓reduceIte, projDual_zero]

private theorem singleton_active_option_eq {n : Nat} (cFin : Fin n)
    (f g : Fin n → Option Rat)
    (objC rowSum' rcBase a b : Rat) (ha_ne : a ≠ 0)
    (hother : ∀ j : Fin n, j ≠ cFin → f j = g j)
    (hfc : f cFin = some 0)
    (hgc : g cFin = some (rcBase * (b / a))) :
    (match sumOptFin n f with
      | none => none
      | some rcSum => some (objC + (rowSum' + (rcBase / a) * b) + rcSum)) =
    (match sumOptFin n g with
      | none => none
      | some rcSum' => some (objC + rowSum' + rcSum')) := by
  have _ := ha_ne
  rw [sumOptFin_split_at f cFin 0 hfc, sumOptFin_split_at g cFin (rcBase * (b / a)) hgc]
  have hother_eq :
      sumOptFin n (fun j => if j = cFin then some 0 else f j) =
      sumOptFin n (fun j => if j = cFin then some 0 else g j) := by
    apply sumOptFin_congr
    intro j
    by_cases hj : j = cFin
    · simp [hj]
    · simp [hj, hother j hj]
  rw [hother_eq]
  cases sumOptFin n (fun j => if j = cFin then some 0 else g j) with
  | none => rfl
  | some sOther =>
    dsimp only
    apply congrArg some
    have hmul : (rcBase / a) * b = rcBase * (b / a) := by
      rw [Rat.div_def, Rat.div_def, Rat.mul_assoc, Rat.mul_comm a⁻¹ b]
    rw [hmul]
    exact rat_rearr_sng objC rowSum' (rcBase * (b / a)) sOther

/-- Exact safe dual bound preservation across any certified LP presolve step:
reconstructing dual multipliers via `postsolveLPDual` yields the exact same
Neumaier-Shcherbina safe dual bound on `LP` as `yRed` yields on `LP'`. -/
theorem lp_presolve_safeDualBound_eq {LP LP' : LPInstance} {step : LPPresolveStep}
    (hstep : checkLPPresolveStep LP LP' step = true)
    (yRed : Fin LP'.numRows → Rat) :
    safeDualBound LP (postsolveLPDual LP LP' step yRed) = safeDualBound LP' yRed := by
  cases LP with
  | mk n m objC obj sense rows rhs lb ub =>
    cases LP' with
    | mk n' m' objC' obj' sense' rows' rhs' lb' ub' =>
      let LP0 : LPInstance := ⟨n, m, objC, obj, sense, rows, rhs, lb, ub⟩
      let LP1 : LPInstance := ⟨n', m', objC', obj', sense', rows', rhs', lb', ub'⟩
      change checkLPPresolveStep LP0 LP1 step = true at hstep
      change safeDualBound LP0 (postsolveLPDual LP0 LP1 step yRed) = safeDualBound LP1 yRed
      unfold checkLPPresolveStep at hstep
      by_cases hVars : n' = n
      · subst n'
        dsimp only [LP0, LP1] at hstep
        simp only [↓reduceDIte] at hstep
        cases step with
        | fixedSubst col val =>
          by_cases hRows : m' = m
          · subst m'
            by_cases hCol : col < n
            · simp only [↓reduceDIte, hCol, Bool.and_eq_true, allFin3B_iff, beq_iff_eq] at hstep
              rcases hstep with ⟨⟨⟨⟨⟨hlb_c, hub_c⟩, hbnds⟩, hobj_other⟩, hobj_const⟩, hrows⟩
              let cFin : Fin n := ⟨col, hCol⟩
              let LP_red : LPInstance := ⟨n, m, objC', obj', sense', rows', rhs', lb', ub'⟩
              have hpost_eq : postsolveLPDual LP0 LP_red (.fixedSubst col val) yRed = yRed := by
                funext i
                unfold postsolveLPDual
                simp [LP0, LP_red]
              change safeDualBound LP0 (postsolveLPDual LP0 LP_red (.fixedSubst col val) yRed) =
                safeDualBound LP_red yRed
              rw [hpost_eq, safeDualBound_eq_spec LP0 yRed, safeDualBound_eq_spec LP_red yRed]
              dsimp only [LP0, LP_red]
              let y0 : Fin m → Rat := fun i => projDual (sense i) (yRed i)
              let y1 : Fin m → Rat := fun i => projDual (sense' i) (yRed i)
              have hy_eq : ∀ i : Fin m, y1 i = y0 i := by
                intro i
                have hs := constraintSense_beq_iff.mp (hrows i).1.1.1
                dsimp only [y0, y1]
                rw [hs]
              let f : Fin n → Option Rat :=
                fun j => rcTermBound (colReducedCost LP0 y0 j) (lb j) (ub j)
              let g : Fin n → Option Rat :=
                fun j => rcTermBound (colReducedCost LP_red y1 j) (lb' j) (ub' j)
              have hrc_red_c : colReducedCost LP_red y1 cFin = obj' cFin := by
                unfold colReducedCost
                dsimp only [LP_red]
                have hz : finSum m (fun i => y1 i * rowCoeff LP_red i cFin) = 0 := by
                  rw [finSum_congr (fun i => by
                    have h0 := (hrows i).1.1.2
                    change y1 i * rowCoeff LP_red i cFin = 0
                    rw [h0, Rat.mul_zero]), finSum_zero]
                rw [hz, rat_sub_zero]
              have hfc : f cFin = some (colReducedCost LP0 y0 cFin * val) := by
                dsimp only [f]
                rw [hlb_c, hub_c]
                exact rcTermBound_fixed _ val
              have hgc : g cFin = some (obj' cFin * val) := by
                dsimp only [g]
                rw [(hbnds cFin).1, (hbnds cFin).2, hlb_c, hub_c, hrc_red_c]
                exact rcTermBound_fixed _ val
              have hfg_other : ∀ j : Fin n, j ≠ cFin → f j = g j := by
                intro j hj
                dsimp only [f, g]
                rw [(hbnds j).1, (hbnds j).2]
                have ho_obj := hobj_other j
                have hj' : j ≠ ⟨col, hCol⟩ := hj
                simp only [hj', ↓reduceIte, beq_iff_eq] at ho_obj
                have hrc_eq : colReducedCost LP0 y0 j = colReducedCost LP_red y1 j := by
                  unfold colReducedCost
                  dsimp only [LP0, LP_red]
                  rw [ho_obj]
                  apply congrArg (obj j - ·)
                  apply finSum_congr
                  intro i
                  have ho_rc := (hrows i).2 j
                  simp only [hj', ↓reduceIte, beq_iff_eq] at ho_rc
                  rw [hy_eq i, ho_rc]
                rw [hrc_eq]
              change (match sumOptFin n f with
                  | none => none
                  | some rcSum => some (objC + finSum m (fun i => y0 i * rhs i) + rcSum)) =
                (match sumOptFin n g with
                  | none => none
                  | some rcSum' => some (objC' + finSum m (fun i => y1 i * rhs' i) + rcSum'))
              rw [sumOptFin_split_at f cFin _ hfc, sumOptFin_split_at g cFin _ hgc]
              have hother_eq :
                  sumOptFin n (fun j => if j = cFin then some 0 else f j) =
                  sumOptFin n (fun j => if j = cFin then some 0 else g j) := by
                apply sumOptFin_congr
                intro j
                by_cases hj : j = cFin
                · simp [hj]
                · simp [hj, hfg_other j hj]
              rw [hother_eq]
              cases sumOptFin n (fun j => if j = cFin then some 0 else g j) with
              | none => rfl
              | some sOther =>
                dsimp only
                apply congrArg some
                let dotC := finSum m (fun i => y0 i * rowCoeff LP0 i cFin)
                have hrhs_sum :
                    finSum m (fun i => y1 i * rhs' i) =
                    finSum m (fun i => y0 i * rhs i) - dotC * val := by
                  have hp : ∀ i : Fin m,
                      y1 i * rhs' i = y0 i * rhs i - (y0 i * rowCoeff LP0 i cFin) * val := by
                    intro i
                    rw [hy_eq i, (hrows i).1.2, rat_mul_sub, Rat.mul_assoc]
                  rw [finSum_congr hp, finSum_sub, finSum_mul_const]
                have hrc0_val :
                    colReducedCost LP0 y0 cFin * val = obj cFin * val - dotC * val := by
                  unfold colReducedCost
                  dsimp only [LP0]
                  exact rat_sub_mul (obj cFin) dotC val
                rw [hrhs_sum, hrc0_val]
                let rowSum := finSum m (fun i => y0 i * rhs i)
                rw [rat_rearr_fixed_lhs objC rowSum sOther (obj cFin * val) (dotC * val),
                    rat_rearr_fixed_rhs objC' rowSum sOther (obj' cFin * val) (dotC * val),
                    hobj_const]
            · simp [hCol] at hstep
          · simp [hRows] at hstep
        | dropRedundantRow droppedRow origRowIdx =>
          by_cases hDrop : droppedRow < m
          · simp only [hDrop, ↓reduceDIte, Bool.and_eq_true] at hstep
            rcases hstep with ⟨⟨⟨hsub, hobjC_b⟩, hvars_b⟩, _⟩
            have hobjC_eq : objC' = objC := beq_iff_eq.mp hobjC_b
            subst objC'
            let dFin : Fin m := ⟨droppedRow, hDrop⟩
            let LP_red : LPInstance := ⟨n, m', objC, obj', sense', rows', rhs', lb', ub'⟩
            rcases rowSubmap_yBase_props (LP := LP0) (LP' := LP_red) (hVars := rfl) hsub yRed with
              ⟨mapFn, _, hyBase_proj, hrhs_map, hrc_map, hsum_map⟩
            have hvars_all := allFin3B_iff.mp hvars_b
            have hpost_sub :
                postsolveLPDual LP0 LP_red (.dropRedundantRow droppedRow origRowIdx) yRed =
                (fun i => submapDualVec LP_red origRowIdx yRed i.val) := rfl
            change safeDualBound LP0 (postsolveLPDual LP0 LP_red (.dropRedundantRow droppedRow origRowIdx) yRed) =
              safeDualBound LP_red yRed
            rw [hpost_sub, safeDualBound_eq_spec LP0 _, safeDualBound_eq_spec LP_red yRed]
            have hrhs_sum :
                finSum m (fun i =>
                  projDual (LP0.sense i) (submapDualVec LP_red origRowIdx yRed i.val) *
                    LP0.rhs i) =
                finSum m' (fun q => projDual (LP_red.sense q) (yRed q) * LP_red.rhs q) := by
              rw [finSum_congr (fun i => congrArg (· * LP0.rhs i) (hyBase_proj i)),
                  hsum_map LP0.rhs]
              exact finSum_congr (fun q => congrArg (projDual (LP_red.sense q) (yRed q) * ·) (hrhs_map q).symm)
            have hterms_eq :
                ∀ j : Fin n,
                  rcTermBound
                    (colReducedCost LP0
                      (fun i => projDual (LP0.sense i) (submapDualVec LP_red origRowIdx yRed i.val)) j)
                    (LP0.lb j) (LP0.ub j) =
                  rcTermBound
                    (colReducedCost LP_red (fun q => projDual (LP_red.sense q) (yRed q)) j)
                    (LP_red.lb j) (LP_red.ub j) := by
              intro j
              have hj := hvars_all j
              rw [Bool.and_eq_true, Bool.and_eq_true, beq_iff_eq, beq_iff_eq, beq_iff_eq] at hj
              have hrc_j :
                  colReducedCost LP0
                    (fun i => projDual (LP0.sense i) (submapDualVec LP_red origRowIdx yRed i.val)) j =
                  colReducedCost LP_red (fun q => projDual (LP_red.sense q) (yRed q)) j := by
                unfold colReducedCost
                have hdot_j :
                    finSum m (fun i =>
                      projDual (LP0.sense i) (submapDualVec LP_red origRowIdx yRed i.val) *
                        rowCoeff LP0 i j) =
                    finSum m' (fun q => projDual (LP_red.sense q) (yRed q) * rowCoeff LP_red q j) := by
                  rw [finSum_congr (fun i => congrArg (· * rowCoeff LP0 i j) (hyBase_proj i)),
                      hsum_map (fun i => rowCoeff LP0 i j)]
                  exact finSum_congr (fun q =>
                    congrArg (projDual (LP_red.sense q) (yRed q) * ·) (hrc_map q j).symm)
                dsimp only [LP0, LP_red]
                rw [hj.1.1, hdot_j]
              rw [hrc_j]
              dsimp only [LP0, LP_red]
              rw [hj.1.2, hj.2]
            dsimp only [LP0, LP_red] at hrhs_sum hterms_eq ⊢
            rw [sumOptFin_congr hterms_eq, hrhs_sum]
          · simp [hDrop] at hstep
        | zeroObjEmptyColFix col val =>
          by_cases hRows : m' = m
          · subst m'
            by_cases hCol : col < n
            · simp only [hCol, ↓reduceDIte, Bool.and_eq_true] at hstep
              rcases hstep with ⟨⟨⟨⟨⟨⟨⟨_, hobj_c0⟩, _⟩, _⟩, hobjC_b⟩, hobj_b⟩, hbnds_other_b⟩, hrows_b⟩
              have hobjC_eq : objC' = objC := beq_iff_eq.mp hobjC_b
              subst objC'
              let cFin : Fin n := ⟨col, hCol⟩
              let LP_red : LPInstance := ⟨n, m, objC, obj', sense', rows', rhs', lb', ub'⟩
              have hpost_eq : postsolveLPDual LP0 LP_red (.zeroObjEmptyColFix col val) yRed = yRed := by
                funext i
                unfold postsolveLPDual
                simp [LP0, LP_red]
              change safeDualBound LP0 (postsolveLPDual LP0 LP_red (.zeroObjEmptyColFix col val) yRed) =
                safeDualBound LP_red yRed
              rw [hpost_eq, safeDualBound_eq_spec LP0 yRed, safeDualBound_eq_spec LP_red yRed]
              have hobj_eq : ∀ j : Fin n, obj' j = obj j := fun j =>
                beq_iff_eq.mp ((allFin3B_iff.mp hobj_b) j)
              have hrows_all := allFin3B_iff.mp hrows_b
              have hy_eq : ∀ i : Fin m, projDual (sense' i) (yRed i) = projDual (sense i) (yRed i) := by
                intro i
                have hri := hrows_all i
                simp only [Bool.and_eq_true] at hri
                rw [constraintSense_beq_iff.mp hri.1.1.2]
              have hrhs_sum :
                  finSum m (fun i => projDual (sense i) (yRed i) * rhs i) =
                  finSum m (fun i => projDual (sense' i) (yRed i) * rhs' i) := by
                apply finSum_congr
                intro i
                have hri := hrows_all i
                simp only [Bool.and_eq_true] at hri
                rw [hy_eq i, beq_iff_eq.mp hri.1.2]
              have hrc_all_eq :
                  ∀ j : Fin n,
                    colReducedCost LP0 (fun i => projDual (sense i) (yRed i)) j =
                    colReducedCost LP_red (fun i => projDual (sense' i) (yRed i)) j := by
                intro j
                unfold colReducedCost
                dsimp only [LP0, LP_red]
                rw [hobj_eq j]
                apply congrArg (obj j - ·)
                apply finSum_congr
                intro i
                have hri := hrows_all i
                simp only [Bool.and_eq_true, allFin3B_iff] at hri
                rw [hy_eq i, beq_iff_eq.mp (hri.2 j)]
              have hterms_eq :
                  ∀ j : Fin n,
                    rcTermBound (colReducedCost LP0 (fun i => projDual (sense i) (yRed i)) j) (lb j) (ub j) =
                    rcTermBound (colReducedCost LP_red (fun i => projDual (sense' i) (yRed i)) j) (lb' j) (ub' j) := by
                intro j
                rw [← hrc_all_eq j]
                by_cases hj : j = cFin
                · subst hj
                  have hrc0 : colReducedCost LP0 (fun i => projDual (sense i) (yRed i)) cFin = 0 := by
                    unfold colReducedCost
                    dsimp only [LP0]
                    have ho0 : obj cFin = 0 := beq_iff_eq.mp hobj_c0
                    have hz : finSum m (fun i => projDual (sense i) (yRed i) * rowCoeff LP0 i cFin) = 0 := by
                      rw [finSum_congr (fun i => by
                        have hri := hrows_all i
                        simp only [Bool.and_eq_true] at hri
                        have ha0 : rowCoeff LP0 i cFin = 0 := beq_iff_eq.mp hri.1.1.1
                        rw [ha0, Rat.mul_zero]), finSum_zero]
                    rw [ho0, hz, rat_sub_zero]
                  rw [hrc0, rcTermBound_zero, rcTermBound_zero]
                · have ho := (allFin3B_iff.mp hbnds_other_b) j
                  simp [cFin, hj] at ho
                  rw [ho.1, ho.2]
              dsimp only [LP0, LP_red]
              rw [sumOptFin_congr hterms_eq, hrhs_sum]
            · simp [hCol] at hstep
          · simp [hRows] at hstep
        | dualColFix col toLower =>
          by_cases hRows : m' = m
          · subst m'
            by_cases hCol : col < n
            · simp only [hCol, ↓reduceDIte, Bool.and_eq_true] at hstep
              rcases hstep with ⟨⟨⟨⟨hobjC_b, hobj_b⟩, hbnds_other_b⟩, hrows_b⟩, hcol_fix⟩
              have hobjC_eq : objC' = objC := beq_iff_eq.mp hobjC_b
              subst objC'
              let cFin : Fin n := ⟨col, hCol⟩
              let LP_red : LPInstance := ⟨n, m, objC, obj', sense', rows', rhs', lb', ub'⟩
              have hpost_eq : postsolveLPDual LP0 LP_red (.dualColFix col toLower) yRed = yRed := by
                funext i
                unfold postsolveLPDual
                simp [LP0, LP_red]
              change safeDualBound LP0 (postsolveLPDual LP0 LP_red (.dualColFix col toLower) yRed) =
                safeDualBound LP_red yRed
              rw [hpost_eq, safeDualBound_eq_spec LP0 yRed, safeDualBound_eq_spec LP_red yRed]
              have hobj_eq : ∀ j : Fin n, obj' j = obj j := fun j =>
                beq_iff_eq.mp ((allFin3B_iff.mp hobj_b) j)
              have hrows_all := allFin3B_iff.mp hrows_b
              have hy_eq : ∀ i : Fin m, projDual (sense' i) (yRed i) = projDual (sense i) (yRed i) := by
                intro i
                have hri := hrows_all i
                simp only [Bool.and_eq_true] at hri
                rw [constraintSense_beq_iff.mp hri.1.1]
              have hrhs_sum :
                  finSum m (fun i => projDual (sense i) (yRed i) * rhs i) =
                  finSum m (fun i => projDual (sense' i) (yRed i) * rhs' i) := by
                apply finSum_congr
                intro i
                have hri := hrows_all i
                simp only [Bool.and_eq_true] at hri
                rw [hy_eq i, beq_iff_eq.mp hri.1.2]
              have hrc_all_eq :
                  ∀ j : Fin n,
                    colReducedCost LP0 (fun i => projDual (sense i) (yRed i)) j =
                    colReducedCost LP_red (fun i => projDual (sense' i) (yRed i)) j := by
                intro j
                unfold colReducedCost
                dsimp only [LP0, LP_red]
                rw [hobj_eq j]
                apply congrArg (obj j - ·)
                apply finSum_congr
                intro i
                have hri := hrows_all i
                simp only [Bool.and_eq_true, allFin3B_iff] at hri
                rw [hy_eq i, beq_iff_eq.mp (hri.2 j)]
              have hterms_eq :
                  ∀ j : Fin n,
                    rcTermBound (colReducedCost LP0 (fun i => projDual (sense i) (yRed i)) j) (lb j) (ub j) =
                    rcTermBound (colReducedCost LP_red (fun i => projDual (sense' i) (yRed i)) j) (lb' j) (ub' j) := by
                intro j
                rw [← hrc_all_eq j]
                by_cases hj : j = cFin
                · subst hj
                  cases toLower with
                  | true =>
                    simp only [ite_true] at hcol_fix
                    cases hlb_c : lb cFin with
                    | none =>
                      dsimp only [cFin] at hlb_c
                      simp [hlb_c] at hcol_fix
                    | some l =>
                      dsimp only [cFin] at hlb_c
                      rw [hlb_c] at hcol_fix
                      simp only [Bool.and_eq_true, beq_iff_eq, decide_eq_true_eq, allFin3B_iff] at hcol_fix
                      rcases hcol_fix with ⟨⟨⟨⟨_, hlb'_eq⟩, hub'_eq⟩, hobj_nonneg⟩, hcol_signs⟩
                      have hdot_nonpos :
                          finSum m (fun i => projDual (sense i) (yRed i) * rowCoeff LP0 i cFin) ≤ 0 := by
                        apply finSum_nonpos
                        intro i
                        have hsi := hcol_signs i
                        cases hs : sense i with
                        | le =>
                          rw [hs] at hsi
                          simp only [decide_eq_true_eq] at hsi
                          exact rat_mul_nonpos_of_nonpos_of_nonneg (projDual_le_nonpos _) hsi
                        | ge =>
                          rw [hs] at hsi
                          simp only [decide_eq_true_eq] at hsi
                          exact rat_mul_nonpos_of_nonneg_of_nonpos (projDual_ge_nonneg _) hsi
                        | eq =>
                          rw [hs] at hsi
                          simp only [beq_iff_eq] at hsi
                          change projDual .eq (yRed i) * rowCoeff LP0 i ⟨col, hCol⟩ ≤ 0
                          rw [hsi, Rat.mul_zero]
                          exact Rat.le_refl
                      have hrc_nonneg :
                          0 ≤ colReducedCost LP0 (fun i => projDual (sense i) (yRed i)) cFin := by
                        unfold colReducedCost
                        dsimp only [LP0]
                        rw [Rat.sub_eq_add_neg]
                        have hneg_nonneg :
                            0 ≤ -finSum m (fun i => projDual (sense i) (yRed i) * rowCoeff LP0 i cFin) := by
                          have h := Rat.neg_le_neg_iff.mpr hdot_nonpos
                          simpa using h
                        exact Rat.add_nonneg hobj_nonneg hneg_nonneg
                      dsimp only [cFin] at hlb'_eq hub'_eq ⊢
                      rw [hlb'_eq, hub'_eq]
                      exact rcTermBound_nonneg_ub_irrel hrc_nonneg (some l) (ub ⟨col, hCol⟩) (some l)
                  | false =>
                    simp only [Bool.false_eq_true, ↓reduceIte] at hcol_fix
                    cases hub_c : ub cFin with
                    | none =>
                      dsimp only [cFin] at hub_c
                      simp [hub_c] at hcol_fix
                    | some u =>
                      dsimp only [cFin] at hub_c
                      rw [hub_c] at hcol_fix
                      simp only [Bool.and_eq_true, beq_iff_eq, decide_eq_true_eq, allFin3B_iff] at hcol_fix
                      rcases hcol_fix with ⟨⟨⟨⟨_, hlb'_eq⟩, hub'_eq⟩, hobj_nonpos⟩, hcol_signs⟩
                      have hdot_nonneg :
                          0 ≤ finSum m (fun i => projDual (sense i) (yRed i) * rowCoeff LP0 i cFin) := by
                        apply finSum_nonneg
                        intro i
                        have hsi := hcol_signs i
                        cases hs : sense i with
                        | le =>
                          rw [hs] at hsi
                          simp only [decide_eq_true_eq] at hsi
                          exact rat_mul_nonneg_of_nonpos_of_nonpos (projDual_le_nonpos _) hsi
                        | ge =>
                          rw [hs] at hsi
                          simp only [decide_eq_true_eq] at hsi
                          exact Rat.mul_nonneg (projDual_ge_nonneg _) hsi
                        | eq =>
                          rw [hs] at hsi
                          simp only [beq_iff_eq] at hsi
                          change 0 ≤ projDual .eq (yRed i) * rowCoeff LP0 i ⟨col, hCol⟩
                          rw [hsi, Rat.mul_zero]
                          exact Rat.le_refl
                      have hrc_nonpos :
                          colReducedCost LP0 (fun i => projDual (sense i) (yRed i)) cFin ≤ 0 := by
                        unfold colReducedCost
                        dsimp only [LP0]
                        rw [Rat.sub_eq_add_neg]
                        have hneg_nonpos :
                            -finSum m (fun i => projDual (sense i) (yRed i) * rowCoeff LP0 i cFin) ≤ 0 := by
                          have h := Rat.neg_le_neg_iff.mpr hdot_nonneg
                          simpa using h
                        have hsum := rat_add_le_add hobj_nonpos hneg_nonpos
                        rwa [Rat.add_zero] at hsum
                      dsimp only [cFin] at hlb'_eq hub'_eq ⊢
                      rw [hlb'_eq, hub'_eq]
                      exact rcTermBound_nonpos_lb_irrel hrc_nonpos (lb ⟨col, hCol⟩) (some u) (some u)
                · have ho := (allFin3B_iff.mp hbnds_other_b) j
                  simp [cFin, hj] at ho
                  rw [ho.1, ho.2]
              dsimp only [LP0, LP_red]
              rw [sumOptFin_congr hterms_eq, hrhs_sum]
            · simp [hCol] at hstep
          · simp [hRows] at hstep
        | singletonRow origRow col origRowIdx =>
          by_cases hRow : origRow < m
          · by_cases hCol : col < n
            · simp only [hRow, hCol, ↓reduceDIte, Bool.and_eq_true] at hstep
              rcases hstep with ⟨⟨⟨⟨⟨⟨ha_ne_b, hsng_other_b⟩, hsub⟩, hobjC_b⟩, hobj_b⟩, hbnds_other_b⟩, hsng_bnd⟩
              have hobjC_eq : objC' = objC := beq_iff_eq.mp hobjC_b
              subst objC'
              let rFin : Fin m := ⟨origRow, hRow⟩
              let cFin : Fin n := ⟨col, hCol⟩
              let LP_red : LPInstance := ⟨n, m', objC, obj', sense', rows', rhs', lb', ub'⟩
              have ha_ne : rowCoeff LP0 rFin cFin ≠ 0 := bne_iff_ne.mp ha_ne_b
              have hsng_other : ∀ j : Fin n, j ≠ cFin → rowCoeff LP0 rFin j = 0 := by
                intro j hj
                have ho := (allFin3B_iff.mp hsng_other_b) j
                simp [cFin, hj] at ho
                exact ho
              rcases rowSubmap_yBase_props (LP := LP0) (LP' := LP_red) (hVars := rfl) hsub yRed with
                ⟨mapFn, hyBase_rFin, hyBase_proj, hrhs_map, hrc_map, hsum_map⟩
              have hobj_eq : ∀ j : Fin n, obj' j = obj j := fun j =>
                beq_iff_eq.mp ((allFin3B_iff.mp hobj_b) j)
              have hbnds_other : ∀ j : Fin n, j ≠ cFin → lb' j = lb j ∧ ub' j = ub j := by
                intro j hj
                have ho := (allFin3B_iff.mp hbnds_other_b) j
                simp [cFin, hj] at ho
                exact ho
              let yBase : Fin m → Rat := fun i => submapDualVec LP_red origRowIdx yRed i.val
              let a := rowCoeff LP0 rFin cFin
              let b := rhs rFin
              let imp := b / a
              let rcBase := obj cFin - finSum m (fun i => yBase i * rowCoeff LP0 i cFin)
              let sngSense := sense rFin
              let isUpper := (sngSense == .le && decide (0 < a)) || (sngSense == .ge && decide (a < 0))
              let ySing : Rat :=
                if sngSense == .eq then
                  rcBase / a
                else if isUpper then
                  if rcBase < 0 then rcBase / a else 0
                else
                  if 0 < rcBase then rcBase / a else 0
              have hpost_eq :
                  postsolveLPDual LP0 LP_red (.singletonRow origRow col origRowIdx) yRed =
                  (fun i => if i = rFin then ySing else yBase i) := by
                funext i
                unfold postsolveLPDual
                dsimp only [LP0, LP_red]
                simp only [hRow, hCol, ↓reduceDIte]
                rfl
              have hySing_proj : projDual (sense rFin) ySing = ySing :=
                singleton_ySing_proj (sense rFin) a rcBase ha_ne
              let y0 : Fin m → Rat := fun i => if i = rFin then ySing else yBase i
              let y1 : Fin m' → Rat := fun q => projDual (LP_red.sense q) (yRed q)
              have hy0_proj : ∀ i : Fin m, projDual (sense i) (y0 i) = y0 i := by
                intro i
                dsimp only [y0]
                by_cases hi : i = rFin
                · subst hi; simp [hySing_proj]
                · simp only [hi, ↓reduceIte]
                  exact hyBase_proj i
              have hy0_split : ∀ g : Fin m → Rat,
                  finSum m (fun i => y0 i * g i) =
                  finSum m (fun i => yBase i * g i) + ySing * g rFin := by
                intro g
                have hp : ∀ i : Fin m,
                    y0 i * g i = yBase i * g i + (if i = rFin then ySing * g rFin else 0) := by
                  intro i
                  dsimp only [y0]
                  by_cases hi : i = rFin
                  · subst hi
                    have hb0 : yBase rFin = 0 := hyBase_rFin
                    simp [hb0, Rat.zero_mul, Rat.zero_add]
                  · simp [hi, Rat.add_zero]
                rw [finSum_congr hp, finSum_add, finSum_single]
              have hrhs_base :
                  finSum m (fun i => yBase i * rhs i) = finSum m' (fun q => y1 q * rhs' q) := by
                dsimp only [yBase]
                rw [hsum_map LP0.rhs]
                exact finSum_congr (fun q => congrArg (y1 q * ·) (hrhs_map q).symm)
              have hrc_red_eq : ∀ j : Fin n,
                  colReducedCost LP_red y1 j =
                  obj j - finSum m (fun i => yBase i * rowCoeff LP0 i j) := by
                intro j
                unfold colReducedCost
                have hdot_j :
                    finSum m (fun i => yBase i * rowCoeff LP0 i j) =
                    finSum m' (fun q => y1 q * rowCoeff LP_red q j) := by
                  dsimp only [yBase]
                  rw [hsum_map (fun i => rowCoeff LP0 i j)]
                  exact finSum_congr (fun q => congrArg (y1 q * ·) (hrc_map q j).symm)
                dsimp only [LP_red]
                rw [hobj_eq j, ← hdot_j]
              have hrc0_other : ∀ j : Fin n, j ≠ cFin →
                  colReducedCost LP0 y0 j = colReducedCost LP_red y1 j := by
                intro j hj
                rw [hrc_red_eq j]
                unfold colReducedCost
                dsimp only [LP0]
                rw [hy0_split (fun i => rowCoeff LP0 i j), hsng_other j hj, Rat.mul_zero, Rat.add_zero]
              have hrc0_cFin :
                  colReducedCost LP0 y0 cFin = rcBase - ySing * a := by
                unfold colReducedCost
                dsimp only [LP0, rcBase, a]
                rw [hy0_split (fun i => rowCoeff LP0 i cFin)]
                exact rat_sub_add_eq_sub_sub _ _ _
              change safeDualBound LP0 (postsolveLPDual LP0 LP_red (.singletonRow origRow col origRowIdx) yRed) =
                safeDualBound LP_red yRed
              rw [hpost_eq, safeDualBound_eq_spec LP0 y0, safeDualBound_eq_spec LP_red yRed]
              have hy0_fun : (fun i => projDual (LP0.sense i) (y0 i)) = y0 := funext hy0_proj
              rw [hy0_fun]
              let f : Fin n → Option Rat :=
                fun j => rcTermBound (colReducedCost LP0 y0 j) (lb j) (ub j)
              let g : Fin n → Option Rat :=
                fun j => rcTermBound (colReducedCost LP_red y1 j) (lb' j) (ub' j)
              have hfg_other : ∀ j : Fin n, j ≠ cFin → f j = g j := by
                intro j hj
                dsimp only [f, g]
                rw [hrc0_other j hj, (hbnds_other j hj).1, (hbnds_other j hj).2]
              have hrhs_y0 :
                  finSum m (fun i => y0 i * LP0.rhs i) =
                  finSum m' (fun q => y1 q * LP_red.rhs q) + ySing * b := by
                rw [hy0_split LP0.rhs, hrhs_base]
              change (match sumOptFin n f with
                  | none => none
                  | some rcSum => some (objC + finSum m (fun i => y0 i * LP0.rhs i) + rcSum)) =
                (match sumOptFin n g with
                  | none => none
                  | some rcSum' => some (objC + finSum m' (fun q => y1 q * LP_red.rhs q) + rcSum'))
              rw [hrhs_y0]
              have hactive :
                  ySing = rcBase / a →
                  g cFin = some (rcBase * imp) →
                  (match sumOptFin n f with
                    | none => none
                    | some rcSum => some (objC + (finSum m' (fun q => y1 q * LP_red.rhs q) + ySing * b) + rcSum)) =
                  (match sumOptFin n g with
                    | none => none
                    | some rcSum' => some (objC + finSum m' (fun q => y1 q * LP_red.rhs q) + rcSum')) := by
                intro hyS hgc
                rw [hyS]
                have hfc : f cFin = some 0 := by
                  dsimp only [f]
                  rw [hrc0_cFin, hyS, Rat.div_mul_cancel ha_ne, Rat.sub_self]
                  exact rcTermBound_zero _ _
                exact singleton_active_option_eq cFin f g objC
                  (finSum m' (fun q => y1 q * LP_red.rhs q)) rcBase a b ha_ne hfg_other hfc hgc
              have hinactive :
                  ySing = 0 →
                  f cFin = g cFin →
                  (match sumOptFin n f with
                    | none => none
                    | some rcSum => some (objC + (finSum m' (fun q => y1 q * LP_red.rhs q) + ySing * b) + rcSum)) =
                  (match sumOptFin n g with
                    | none => none
                    | some rcSum' => some (objC + finSum m' (fun q => y1 q * LP_red.rhs q) + rcSum')) := by
                intro hyS hfc_eq
                rw [hyS, Rat.zero_mul, Rat.add_zero]
                have hfg_all : ∀ j : Fin n, f j = g j := by
                  intro j
                  by_cases hj : j = cFin
                  · subst hj; exact hfc_eq
                  · exact hfg_other j hj
                rw [sumOptFin_congr hfg_all]
              cases hs_r : sense rFin with
              | eq =>
                dsimp only [rFin] at hs_r
                rw [hs_r] at hsng_bnd
                simp only [Bool.and_eq_true, beq_iff_eq] at hsng_bnd
                rcases hsng_bnd with ⟨⟨_, hlb'_eq⟩, hub'_eq⟩
                have hyS : ySing = rcBase / a := by
                  dsimp only [ySing, sngSense, rFin]
                  simp [hs_r]
                have hgc : g cFin = some (rcBase * imp) := by
                  dsimp only [g]
                  rw [hrc_red_eq cFin, hlb'_eq, hub'_eq]
                  exact rcTermBound_fixed rcBase imp
                exact hactive hyS hgc
              | le =>
                dsimp only [rFin] at hs_r
                rw [hs_r] at hsng_bnd
                dsimp only at hsng_bnd
                by_cases hpos : 0 < a
                · have hpos' : 0 < rowCoeff LP0 ⟨origRow, hRow⟩ ⟨col, hCol⟩ := hpos
                  dsimp only [LP0] at hpos'
                  simp only [hpos', ↓reduceIte, Bool.and_eq_true, beq_iff_eq] at hsng_bnd
                  rcases hsng_bnd with ⟨⟨_, hlb'_eq⟩, hub'_eq⟩
                  by_cases hrc : rcBase < 0
                  · have hyS : ySing = rcBase / a := by
                      dsimp only [ySing, isUpper, sngSense, rFin]
                      simp [hs_r, hpos, hrc]
                    have hgc : g cFin = some (rcBase * imp) := by
                      dsimp only [g]
                      rw [hrc_red_eq cFin, hub'_eq]
                      exact rcTermBound_of_neg hrc (lb' cFin)
                    exact hactive hyS hgc
                  · have hyS : ySing = 0 := by
                      dsimp only [ySing, isUpper, sngSense, rFin]
                      simp [hs_r, hpos, hrc]
                    have hfc_eq : f cFin = g cFin := by
                      dsimp only [f, g]
                      rw [hrc0_cFin, hyS, Rat.zero_mul, rat_sub_zero, hrc_red_eq cFin, hlb'_eq]
                      exact rcTermBound_nonneg_ub_irrel (Rat.not_lt.mp hrc) (lb cFin) (ub cFin) (ub' cFin)
                    exact hinactive hyS hfc_eq
                · have hpos' : ¬(0 < rowCoeff LP0 ⟨origRow, hRow⟩ ⟨col, hCol⟩) := hpos
                  dsimp only [LP0] at hpos'
                  simp only [hpos', ↓reduceIte, Bool.and_eq_true, beq_iff_eq] at hsng_bnd
                  rcases hsng_bnd with ⟨⟨_, hlb'_eq⟩, hub'_eq⟩
                  by_cases hrc : 0 < rcBase
                  · have hyS : ySing = rcBase / a := by
                      dsimp only [ySing, isUpper, sngSense, rFin]
                      simp [hs_r, hpos, hrc]
                    have hgc : g cFin = some (rcBase * imp) := by
                      dsimp only [g]
                      rw [hrc_red_eq cFin, hlb'_eq]
                      exact rcTermBound_of_pos hrc (ub' cFin)
                    exact hactive hyS hgc
                  · have hyS : ySing = 0 := by
                      dsimp only [ySing, isUpper, sngSense, rFin]
                      simp [hs_r, hpos, hrc]
                    have hfc_eq : f cFin = g cFin := by
                      dsimp only [f, g]
                      rw [hrc0_cFin, hyS, Rat.zero_mul, rat_sub_zero, hrc_red_eq cFin, hub'_eq]
                      exact rcTermBound_nonpos_lb_irrel (Rat.not_lt.mp hrc) (lb cFin) (lb' cFin) (ub cFin)
                    exact hinactive hyS hfc_eq
              | ge =>
                dsimp only [rFin] at hs_r
                rw [hs_r] at hsng_bnd
                dsimp only at hsng_bnd
                by_cases hpos : 0 < a
                · have hpos' : 0 < rowCoeff LP0 ⟨origRow, hRow⟩ ⟨col, hCol⟩ := hpos
                  dsimp only [LP0] at hpos'
                  simp only [hpos', ↓reduceIte, Bool.and_eq_true, beq_iff_eq] at hsng_bnd
                  rcases hsng_bnd with ⟨⟨_, hlb'_eq⟩, hub'_eq⟩
                  have hnotneg : ¬(a < 0) := Rat.not_lt.mpr (Rat.le_of_lt hpos)
                  by_cases hrc : 0 < rcBase
                  · have hyS : ySing = rcBase / a := by
                      dsimp only [ySing, isUpper, sngSense, rFin]
                      simp [hs_r, hnotneg, hrc]
                    have hgc : g cFin = some (rcBase * imp) := by
                      dsimp only [g]
                      rw [hrc_red_eq cFin, hlb'_eq]
                      exact rcTermBound_of_pos hrc (ub' cFin)
                    exact hactive hyS hgc
                  · have hyS : ySing = 0 := by
                      dsimp only [ySing, isUpper, sngSense, rFin]
                      simp [hs_r, hnotneg, hrc]
                    have hfc_eq : f cFin = g cFin := by
                      dsimp only [f, g]
                      rw [hrc0_cFin, hyS, Rat.zero_mul, rat_sub_zero, hrc_red_eq cFin, hub'_eq]
                      exact rcTermBound_nonpos_lb_irrel (Rat.not_lt.mp hrc) (lb cFin) (lb' cFin) (ub cFin)
                    exact hinactive hyS hfc_eq
                · have hpos' : ¬(0 < rowCoeff LP0 ⟨origRow, hRow⟩ ⟨col, hCol⟩) := hpos
                  dsimp only [LP0] at hpos'
                  simp only [hpos', ↓reduceIte, Bool.and_eq_true, beq_iff_eq] at hsng_bnd
                  rcases hsng_bnd with ⟨⟨_, hlb'_eq⟩, hub'_eq⟩
                  have hneg : a < 0 := Rat.lt_of_le_of_ne (Rat.not_lt.mp hpos) ha_ne
                  by_cases hrc : rcBase < 0
                  · have hyS : ySing = rcBase / a := by
                      dsimp only [ySing, isUpper, sngSense, rFin]
                      simp [hs_r, hneg, hrc]
                    have hgc : g cFin = some (rcBase * imp) := by
                      dsimp only [g]
                      rw [hrc_red_eq cFin, hub'_eq]
                      exact rcTermBound_of_neg hrc (lb' cFin)
                    exact hactive hyS hgc
                  · have hyS : ySing = 0 := by
                      dsimp only [ySing, isUpper, sngSense, rFin]
                      simp [hs_r, hneg, hrc]
                    have hfc_eq : f cFin = g cFin := by
                      dsimp only [f, g]
                      rw [hrc0_cFin, hyS, Rat.zero_mul, rat_sub_zero, hrc_red_eq cFin, hlb'_eq]
                      exact rcTermBound_nonneg_ub_irrel (Rat.not_lt.mp hrc) (lb cFin) (ub cFin) (ub' cFin)
                    exact hinactive hyS hfc_eq
            · simp [hRow, hCol] at hstep
          · simp [hRow] at hstep
      · dsimp only [LP0, LP1] at hstep
        simp [hVars] at hstep

/-- Dual bound lifting soundness: any valid safe dual lower bound `B` on `LP'`
lifts via `postsolveLPDual` to a valid lower bound `B ≤ LPObjective LP x` on `LP`. -/
theorem lp_presolve_dual_bound_sound {LP LP' : LPInstance} {step : LPPresolveStep}
    {yRed : Fin LP'.numRows → Rat} {B : Rat}
    (hstep : checkLPPresolveStep LP LP' step = true)
    (hbound : safeDualBound LP' yRed = some B) :
    safeDualBound LP (postsolveLPDual LP LP' step yRed) = some B ∧
      ∀ x : Fin LP.numVars → Rat, PrimalFeasible LP x → B ≤ LPObjective LP x := by
  have heq : safeDualBound LP (postsolveLPDual LP LP' step yRed) = some B := by
    rw [lp_presolve_safeDualBound_eq hstep yRed]
    exact hbound
  exact ⟨heq, fun _ hx => safeDualBound_sound heq hx⟩

/-- Full primal-dual LP presolve & postsolve optimality soundness:
given a certified reduction `LP ↦ LP'` and a certified primal-dual optimal pair `(x', y')` on `LP'`,
the lifted primal `liftLPPresolvePrimal LP LP' x'` and postsolved dual `postsolveLPDual LP LP' step y'`
certify zero-duality-gap global optimality on the original instance `LP`. -/
theorem lp_presolve_dual_sound {LP LP' : LPInstance} {step : LPPresolveStep}
    {x' : Fin LP'.numVars → Rat} {y' : Fin LP'.numRows → Rat}
    (hstep : checkLPPresolveStep LP LP' step = true)
    (hopt' : checkLPOptimal LP' x' y' = true) :
    let x := liftLPPresolvePrimal LP LP' x'
    let y := postsolveLPDual LP LP' step y'
    PrimalFeasible LP x ∧
      LPObjective LP x = LPObjective LP' x' ∧
      safeDualBound LP y = some (LPObjective LP x) ∧
      (∀ xAny : Fin LP.numVars → Rat, PrimalFeasible LP xAny →
        LPObjective LP x ≤ LPObjective LP xAny) := by
  intro x y
  unfold checkLPOptimal at hopt'
  rw [Bool.and_eq_true] at hopt'
  have hfeas' : PrimalFeasible LP' x' := checkPrimalFeasible_sound hopt'.1
  rcases lp_presolve_primal_lift hstep hfeas' with ⟨hfeas, hobj_eq⟩
  cases hsb' : safeDualBound LP' y' with
  | none =>
    have hg := hopt'.2
    rw [hsb'] at hg
    cases hg
  | some lbVal =>
    have hg := hopt'.2
    rw [hsb', beq_iff_eq] at hg
    have hsb_orig : safeDualBound LP y = some (LPObjective LP x) := by
      dsimp only [y, x]
      rw [lp_presolve_safeDualBound_eq hstep y', hsb', ← hg, hobj_eq]
    exact ⟨hfeas, hobj_eq, hsb_orig, fun _ hxAny => safeDualBound_sound hsb_orig hxAny⟩

/-- Verifies a multi-step sequence of LP presolve reductions
`LP_0 ↦_{(step_1, LP_1)} ⋯ ↦_{(step_k, LP_k)}` ending at `LPFinal`. -/
def checkLPPresolveChain : LPInstance → List (LPPresolveStep × LPInstance) → LPInstance → Bool
  | lpCur, [], lpFinal =>
    if hV : lpFinal.numVars = lpCur.numVars then
      if hR : lpFinal.numRows = lpCur.numRows then
        (lpFinal.objConst == lpCur.objConst) &&
        allFin3B lpCur.numVars (fun j =>
          let j' : Fin lpFinal.numVars := ⟨j.val, by rw [hV]; exact j.isLt⟩
          (lpFinal.obj j' == lpCur.obj j) &&
          (lpFinal.lb j' == lpCur.lb j) &&
          (lpFinal.ub j' == lpCur.ub j)) &&
        allFin3B lpCur.numRows (fun i =>
          let i' : Fin lpFinal.numRows := ⟨i.val, by rw [hR]; exact i.isLt⟩
          (lpFinal.sense i' == lpCur.sense i) &&
          (lpFinal.rhs i' == lpCur.rhs i) &&
          allFin3B lpCur.numVars (fun j =>
            let j' : Fin lpFinal.numVars := ⟨j.val, by rw [hV]; exact j.isLt⟩
            rowCoeff lpFinal i' j' == rowCoeff lpCur i j))
      else false
    else false
  | lpCur, (step, lpNext) :: rest, lpFinal =>
    checkLPPresolveStep lpCur lpNext step && checkLPPresolveChain lpNext rest lpFinal

/-- Lifts a primal vector across a multi-step LP presolve chain. -/
def liftLPPresolvePrimalChain :
    (lpCur : LPInstance) → List (LPPresolveStep × LPInstance) → (lpFinal : LPInstance) →
    (Fin lpFinal.numVars → Rat) → (Fin lpCur.numVars → Rat)
  | lpCur, [], lpFinal, xFinal => liftLPPresolvePrimal lpCur lpFinal xFinal
  | lpCur, (_, lpNext) :: rest, lpFinal, xFinal =>
    liftLPPresolvePrimal lpCur lpNext (liftLPPresolvePrimalChain lpNext rest lpFinal xFinal)

/-- Reconstructs dual multipliers across a multi-step LP presolve chain (reverse order). -/
def postsolveLPDualChain :
    (lpCur : LPInstance) → List (LPPresolveStep × LPInstance) → (lpFinal : LPInstance) →
    (Fin lpFinal.numRows → Rat) → (Fin lpCur.numRows → Rat)
  | lpCur, [], lpFinal, yFinal =>
    if hR : lpFinal.numRows = lpCur.numRows then
      fun i => yFinal ⟨i.val, by rw [hR]; exact i.isLt⟩
    else
      fun _ => 0
  | lpCur, (step, lpNext) :: rest, lpFinal, yFinal =>
    postsolveLPDual lpCur lpNext step (postsolveLPDualChain lpNext rest lpFinal yFinal)

private theorem nil_chain_primal_and_dual {LP LP' : LPInstance}
    (hchain : checkLPPresolveChain LP [] LP' = true) :
    (∀ x' : Fin LP'.numVars → Rat, PrimalFeasible LP' x' →
      PrimalFeasible LP (liftLPPresolvePrimal LP LP' x') ∧
      LPObjective LP (liftLPPresolvePrimal LP LP' x') = LPObjective LP' x') ∧
    (∀ y' : Fin LP'.numRows → Rat,
      safeDualBound LP (postsolveLPDualChain LP [] LP' y') = safeDualBound LP' y') := by
  cases LP with
  | mk n m objC obj sense rows rhs lb ub =>
    cases LP' with
    | mk n' m' objC' obj' sense' rows' rhs' lb' ub' =>
      let LP0 : LPInstance := ⟨n, m, objC, obj, sense, rows, rhs, lb, ub⟩
      let LP1 : LPInstance := ⟨n', m', objC', obj', sense', rows', rhs', lb', ub'⟩
      change checkLPPresolveChain LP0 [] LP1 = true at hchain
      unfold checkLPPresolveChain at hchain
      by_cases hV : n' = n
      · subst n'
        by_cases hR : m' = m
        · subst m'
          dsimp only [LP0, LP1] at hchain
          simp only [↓reduceDIte, Bool.and_eq_true, allFin3B_iff, beq_iff_eq] at hchain
          rcases hchain with ⟨⟨hobjC, hvars⟩, hrows⟩
          subst objC'
          let LP_red : LPInstance := ⟨n, m, objC, obj', sense', rows', rhs', lb', ub'⟩
          refine ⟨?_, ?_⟩
          · intro x' hfeas'
            have hlift : liftLPPresolvePrimal LP0 LP_red x' = x' := by
              funext j; unfold liftLPPresolvePrimal; simp [LP0, LP_red]
            change PrimalFeasible LP0 (liftLPPresolvePrimal LP0 LP_red x') ∧
              LPObjective LP0 (liftLPPresolvePrimal LP0 LP_red x') = LPObjective LP_red x'
            rw [hlift]
            refine ⟨⟨?_, ?_⟩, ?_⟩
            · intro j
              have hf1 : SatisfiesVarBounds (lb' j) (ub' j) (x' j) := hfeas'.1 j
              dsimp only [LP0]
              rwa [(hvars j).1.2, (hvars j).2] at hf1
            · intro i
              have hri := hrows i
              have hs_eq : sense' i = sense i := constraintSense_beq_iff.mp hri.1.1
              have hact_eq : rowActivity LP_red x' i = rowActivity LP0 x' i := by
                rw [rowActivity_eq_finSum LP_red x' i, rowActivity_eq_finSum LP0 x' i]
                exact finSum_congr (fun j => congrArg (· * x' j) (hri.2 j))
              have hfi := hfeas'.2 i
              change SatisfiesRow (sense' i) (rowActivity LP_red x' i) (rhs' i) at hfi
              change SatisfiesRow (sense i) (rowActivity LP0 x' i) (rhs i)
              rwa [hs_eq, hri.1.2, hact_eq] at hfi
            · unfold LPObjective
              dsimp only [LP0, LP_red]
              apply congrArg (objC + ·)
              exact finSum_congr (fun j => congrArg (· * x' j) (hvars j).1.1.symm)
          · intro y'
            have hpost : postsolveLPDualChain LP0 [] LP_red y' = y' := by
              funext i; unfold postsolveLPDualChain; simp [LP0, LP_red]
            change safeDualBound LP0 (postsolveLPDualChain LP0 [] LP_red y') = safeDualBound LP_red y'
            rw [hpost, safeDualBound_eq_spec LP0 y', safeDualBound_eq_spec LP_red y']
            have hy_eq : ∀ i : Fin m, projDual (sense' i) (y' i) = projDual (sense i) (y' i) := by
              intro i
              rw [constraintSense_beq_iff.mp (hrows i).1.1]
            have hrhs_eq :
                finSum m (fun i => projDual (sense i) (y' i) * rhs i) =
                finSum m (fun i => projDual (sense' i) (y' i) * rhs' i) := by
              apply finSum_congr
              intro i
              rw [hy_eq i, (hrows i).1.2]
            have hterms_eq :
                ∀ j : Fin n,
                  rcTermBound (colReducedCost LP0 (fun i => projDual (sense i) (y' i)) j) (lb j) (ub j) =
                  rcTermBound (colReducedCost LP_red (fun i => projDual (sense' i) (y' i)) j) (lb' j) (ub' j) := by
              intro j
              have hrc_j :
                  colReducedCost LP0 (fun i => projDual (sense i) (y' i)) j =
                  colReducedCost LP_red (fun i => projDual (sense' i) (y' i)) j := by
                unfold colReducedCost
                dsimp only [LP0, LP_red]
                rw [(hvars j).1.1]
                apply congrArg (obj j - ·)
                exact finSum_congr (fun i => by rw [hy_eq i, (hrows i).2 j])
              rw [hrc_j, (hvars j).1.2, (hvars j).2]
            dsimp only [LP0, LP_red]
            rw [sumOptFin_congr hterms_eq, hrhs_eq]
        · dsimp only [LP0, LP1] at hchain
          simp [hR] at hchain
      · dsimp only [LP0, LP1] at hchain
        simp [hV] at hchain

/-- Multi-step LP presolve chain soundness: primal feasibility, objective equality,
and exact Neumaier-Shcherbina safe dual bound preservation hold across arbitrary chains. -/
theorem lp_presolve_chain_dual_sound {LP LPFinal : LPInstance}
    {steps : List (LPPresolveStep × LPInstance)}
    {xFinal : Fin LPFinal.numVars → Rat} {yFinal : Fin LPFinal.numRows → Rat}
    (hchain : checkLPPresolveChain LP steps LPFinal = true)
    (hopt : checkLPOptimal LPFinal xFinal yFinal = true) :
    let x := liftLPPresolvePrimalChain LP steps LPFinal xFinal
    let y := postsolveLPDualChain LP steps LPFinal yFinal
    PrimalFeasible LP x ∧
      LPObjective LP x = LPObjective LPFinal xFinal ∧
      safeDualBound LP y = some (LPObjective LP x) ∧
      (∀ xAny : Fin LP.numVars → Rat, PrimalFeasible LP xAny →
        LPObjective LP x ≤ LPObjective LP xAny) := by
  have hgen :
      ∀ (lpCur : LPInstance) (st : List (LPPresolveStep × LPInstance)),
        checkLPPresolveChain lpCur st LPFinal = true →
        (∀ xf : Fin LPFinal.numVars → Rat, PrimalFeasible LPFinal xf →
          PrimalFeasible lpCur (liftLPPresolvePrimalChain lpCur st LPFinal xf) ∧
          LPObjective lpCur (liftLPPresolvePrimalChain lpCur st LPFinal xf) = LPObjective LPFinal xf) ∧
        (∀ yf : Fin LPFinal.numRows → Rat,
          safeDualBound lpCur (postsolveLPDualChain lpCur st LPFinal yf) = safeDualBound LPFinal yf) := by
    intro lpCur st
    induction st generalizing lpCur with
    | nil =>
      intro hc
      exact nil_chain_primal_and_dual hc
    | cons hd tl ih =>
      rcases hd with ⟨step, lpNext⟩
      intro hc
      unfold checkLPPresolveChain at hc
      rw [Bool.and_eq_true] at hc
      rcases ih lpNext hc.2 with ⟨hprimal_tl, hdual_tl⟩
      refine ⟨?_, ?_⟩
      · intro xf hxf
        rcases hprimal_tl xf hxf with ⟨hf_next, hobj_next⟩
        rcases lp_presolve_primal_lift hc.1 hf_next with ⟨hf_cur, hobj_cur⟩
        exact ⟨hf_cur, Eq.trans hobj_cur hobj_next⟩
      · intro yf
        unfold postsolveLPDualChain
        rw [lp_presolve_safeDualBound_eq hc.1 _, hdual_tl yf]
  intro x y
  rcases hgen LP steps hchain with ⟨hprimal, hdual⟩
  unfold checkLPOptimal at hopt
  rw [Bool.and_eq_true] at hopt
  have hfeasFinal : PrimalFeasible LPFinal xFinal := checkPrimalFeasible_sound hopt.1
  rcases hprimal xFinal hfeasFinal with ⟨hfeas, hobj_eq⟩
  cases hsb : safeDualBound LPFinal yFinal with
  | none =>
    have hg := hopt.2
    rw [hsb] at hg
    cases hg
  | some lbVal =>
    have hg := hopt.2
    rw [hsb, beq_iff_eq] at hg
    have hsb_orig : safeDualBound LP y = some (LPObjective LP x) := by
      dsimp only [y, x]
      rw [hdual yFinal, hsb, ← hg, hobj_eq]
    exact ⟨hfeas, hobj_eq, hsb_orig, fun _ hxAny => safeDualBound_sound hsb_orig hxAny⟩

/-! ### Section 5: JSON-Ready Certificate Parsers & Verifiers (Wave 2 CLI Bridges) -/

private def parseDecimalNat3B? (chars : List Char) : Option Nat :=
  if chars.isEmpty then none
  else
    chars.foldl (fun acc c =>
      match acc with
      | none => none
      | some n =>
        if '0' ≤ c && c ≤ '9' then some (n * 10 + (c.toNat - '0'.toNat))
        else none) (some 0)

private def parseHexNat3B? (chars : List Char) : Option Nat :=
  if chars.isEmpty then none
  else
    chars.foldl (fun acc c =>
      match acc with
      | none => none
      | some n =>
        if '0' ≤ c && c ≤ '9' then some (n * 16 + (c.toNat - '0'.toNat))
        else if 'a' ≤ c && c ≤ 'f' then some (n * 16 + 10 + (c.toNat - 'a'.toNat))
        else if 'A' ≤ c && c ≤ 'F' then some (n * 16 + 10 + (c.toNat - 'A'.toNat))
        else none) (some 0)

/-- Parses an IEEE-754 `UInt64` bit-pattern from a JSON integer or decimal/hex string. -/
def asUInt64BitsJson (v : JsonVal) : Except String UInt64 :=
  match v with
  | .num _ _ _ => v.asUInt64
  | .str s =>
    let parsed : Option Nat :=
      match s.toList with
      | '0' :: 'x' :: rest => parseHexNat3B? rest
      | '0' :: 'X' :: rest => parseHexNat3B? rest
      | chars => parseDecimalNat3B? chars
    match parsed with
    | none => .error s!"invalid UInt64 bitstring '{s}'"
    | some n =>
      if n < UInt64.size then .ok (UInt64.ofNat n)
      else .error s!"bitstring {s} exceeds UInt64 range"
  | _ => .error "expected UInt64 integer or bitstring"

/-- Parses an exact `Rat` from `{"bits": ...}` or any standard `JsonVal.asRat` encoding. -/
def asRatOrBitsJson (v : JsonVal) : Except String Rat :=
  match v.getField? "bits" with
  | some bv => do
    let bits ← asUInt64BitsJson bv
    match float64BitsToRat bits with
    | some r => .ok r
    | none => .error "non-finite float64 bits (Inf/NaN)"
  | none => v.asRat

/-- Parses a constraint sense string (`"le"`, `"<="`, `"ge"`, `">="`, `"eq"`, `"="`, `"=="`). -/
def parseSenseJson (s : String) : Except String ConstraintSense :=
  match s with
  | "le" | "<=" => .ok .le
  | "ge" | ">=" => .ok .ge
  | "eq" | "=" | "==" => .ok .eq
  | _ => .error s!"invalid constraint sense '{s}'"

private def getOptNonNullField (root : JsonVal) (names : List String) : Option JsonVal :=
  names.findSome? fun name =>
    match root.getField? name with
    | some .null | none => none
    | some v => some v

/-- Parses a fixed-length exact `Rat` vector from named rational or IEEE-754 bits array fields. -/
def parseRatVecJson (root : JsonVal) (ratNames bitsNames : List String)
    (expectedLen : Nat) (label : String) : Except String (Array Rat) := do
  match getOptNonNullField root ratNames with
  | some v =>
    let arr ← v.asArray
    if arr.size != expectedLen then
      throw s!"'{label}' length {arr.size} != expected {expectedLen}"
    arr.mapM asRatOrBitsJson
  | none =>
    match getOptNonNullField root bitsNames with
    | some v =>
      let arr ← v.asArray
      if arr.size != expectedLen then
        throw s!"'{label}Bits' length {arr.size} != expected {expectedLen}"
      arr.mapM fun elem => do
        let bits ← asUInt64BitsJson elem
        match float64BitsToRat bits with
        | some r => .ok r
        | none => .error s!"'{label}Bits' encodes Inf or NaN"
    | none =>
      throw s!"missing required vector field '{label}'"

private def parseOptRatVecJson (arr : Array JsonVal) (expectedLen : Nat) (label : String) :
    Except String (Array (Option Rat)) := do
  if arr.size != expectedLen then
    throw s!"'{label}' length {arr.size} != expected {expectedLen}"
  arr.mapM fun v =>
    match v with
    | .null => .ok none
    | _ => some <$> asRatOrBitsJson v

private def parseRowTermsJson (numVars : Nat) (arr : Array JsonVal) (rowIdx : Nat) :
    Except String (List (SparseTerm numVars)) := do
  let mapped ← arr.mapM fun termVal => do
    let col ← termVal.getNat "col"
    if h : col < numVars then
      let cv ← termVal.getField "coeff"
      let coeff ← asRatOrBitsJson cv
      pure ({ col := ⟨col, h⟩, coeff := coeff } : SparseTerm numVars)
    else
      throw s!"row {rowIdx} column index {col} >= numVars {numVars}"
  return mapped.toList

/-- Parses an `LPInstance` from JSON matching `Apps/LpCheck.lean` schema. -/
def parseLPInstanceJson (root : JsonVal) : Except String LPInstance := do
  let numVars ← root.getNat "numVars"
  let numRows ← root.getNat "numRows"
  let objConst ←
    match root.getField? "objConst" with
    | none | some .null => pure (0 : Rat)
    | some v => asRatOrBitsJson v
  let objArr ← parseRatVecJson root ["obj"] ["objBits"] numVars "obj"
  let lbArr ← parseOptRatVecJson (← root.getArray "lb") numVars "lb"
  let ubArr ← parseOptRatVecJson (← root.getArray "ub") numVars "ub"
  let rowsJson ← root.getArray "rows"
  if rowsJson.size != numRows then
    throw s!"'rows' length {rowsJson.size} != numRows {numRows}"
  let senseArr ← rowsJson.mapM fun rv => rv.getString "sense" >>= parseSenseJson
  let rhsArr ← rowsJson.mapM fun rv => rv.getField "rhs" >>= asRatOrBitsJson
  let termsList ← (List.range numRows).mapM fun i =>
    if hi : i < rowsJson.size then
      rowsJson[i].getArray "terms" >>= fun tj => parseRowTermsJson numVars tj i
    else
      throw "internal row index out of range"
  let termsArr := termsList.toArray
  if hObj : objArr.size = numVars then
    if hLb : lbArr.size = numVars then
      if hUb : ubArr.size = numVars then
        if hSense : senseArr.size = numRows then
          if hTerms : termsArr.size = numRows then
            if hRhs : rhsArr.size = numRows then
              return {
                numVars := numVars
                numRows := numRows
                objConst := objConst
                obj := fun j => objArr[j.val]'(by rw [hObj]; exact j.isLt)
                sense := fun i => senseArr[i.val]'(by rw [hSense]; exact i.isLt)
                rows := fun i => termsArr[i.val]'(by rw [hTerms]; exact i.isLt)
                rhs := fun i => rhsArr[i.val]'(by rw [hRhs]; exact i.isLt)
                lb := fun j => lbArr[j.val]'(by rw [hLb]; exact j.isLt)
                ub := fun j => ubArr[j.val]'(by rw [hUb]; exact j.isLt)
              }
            else throw "internal rhs array size mismatch"
          else throw "internal terms array size mismatch"
        else throw "internal sense array size mismatch"
      else throw "internal ub array size mismatch"
    else throw "internal lb array size mismatch"
  else throw "internal obj array size mismatch"

/-- Verifies an LP unboundedness certificate `(x0, d)` against `lp` from JSON fields. -/
def verifyLPUnboundedJson (lp : LPInstance) (root : JsonVal) : Except String Unit := do
  let xArr ← parseRatVecJson root ["x0", "x", "anchor", "primal"]
    ["x0Bits", "xBits", "anchorBits", "primalBits"] lp.numVars "x0"
  let dArr ← parseRatVecJson root ["d", "ray", "direction"]
    ["dBits", "rayBits", "directionBits"] lp.numVars "d"
  if hx : xArr.size = lp.numVars then
    if hd : dArr.size = lp.numVars then
      let x0 : Fin lp.numVars → Rat := fun j => xArr[j.val]'(by rw [hx]; exact j.isLt)
      let d : Fin lp.numVars → Rat := fun j => dArr[j.val]'(by rw [hd]; exact j.isLt)
      if checkLPUnbounded lp x0 d then
        return ()
      else if !checkPrimalFeasible lp x0 then
        throw "LP unboundedness certificate rejected: anchor x0 is not primal-feasible"
      else if !(allFin3B lp.numVars (fun j => checkRayVarBound (lp.lb j) (lp.ub j) (d j))) then
        throw "LP unboundedness certificate rejected: ray d violates a finite variable bound"
      else if !(allFin3B lp.numRows (fun i => checkRayRow (lp.sense i) (rowActivity lp d i))) then
        throw "LP unboundedness certificate rejected: ray d violates homogeneous row recession"
      else
        throw "LP unboundedness certificate rejected: objective directional derivative c·d >= 0"
    else throw "internal ray vector size mismatch"
  else throw "internal anchor vector size mismatch"

/-- Standalone root JSON verifier for `unbounded` mode. -/
def verifyLPUnboundedRootJson (root : JsonVal) : Except String Unit := do
  let lp ← parseLPInstanceJson root
  verifyLPUnboundedJson lp root

private def parseIsIntMaskField (numVars : Nat) (v : JsonVal) (label : String) :
    Except String (Array Bool) := do
  let arr ← v.asArray
  if arr.size != numVars then
    throw s!"'{label}' length {arr.size} != numVars {numVars}"
  arr.mapM JsonVal.asBool

private def parseIntColsField (numVars : Nat) (v : JsonVal) : Except String (Array Bool) := do
  let colsJson ← v.asArray
  let cols ← colsJson.mapM JsonVal.asNat
  for c in cols do
    if c ≥ numVars then
      throw s!"integer column index {c} >= numVars {numVars}"
  return Array.ofFn (fun j : Fin numVars => cols.contains j.val)

private def parseIsIntFlagsJson (numVars : Nat) (root : JsonVal) : Except String (Array Bool) := do
  let maskOpt? : Option (Array Bool) ←
    match getOptNonNullField root ["isInt"], getOptNonNullField root ["intMask"] with
    | some v1, some v2 =>
      let m1 ← parseIsIntMaskField numVars v1 "isInt"
      let m2 ← parseIsIntMaskField numVars v2 "intMask"
      if m1 != m2 then
        throw "conflicting integer-column specifications: 'isInt' and 'intMask' differ"
      pure (some m1)
    | some v1, none => some <$> parseIsIntMaskField numVars v1 "isInt"
    | none, some v2 => some <$> parseIsIntMaskField numVars v2 "intMask"
    | none, none => pure none
  let colsOpt? : Option (Array Bool) ←
    match getOptNonNullField root ["intCols"], getOptNonNullField root ["integerCols"] with
    | some v1, some v2 =>
      let c1 ← parseIntColsField numVars v1
      let c2 ← parseIntColsField numVars v2
      if c1 != c2 then
        throw "conflicting integer-column specifications: 'intCols' and 'integerCols' differ"
      pure (some c1)
    | some v1, none => some <$> parseIntColsField numVars v1
    | none, some v2 => some <$> parseIntColsField numVars v2
    | none, none => pure none
  let flags ←
    match maskOpt?, colsOpt? with
    | some m, some c =>
      if m != c then
        throw "conflicting integer-column specifications: 'isInt' and 'intCols' denote different column sets"
      pure m
    | some m, none => pure m
    | none, some c => pure c
    | none, none =>
      throw "missing required integer-column specification ('isInt' or 'intCols')"
  if !flags.any id then
    throw "milp-unbounded requires at least one integer column; use 'unbounded' mode for all-continuous LPs"
  return flags

/-- Verifies a MILP unboundedness certificate `(isInt, x0, d)` against `lp` from JSON fields. -/
def verifyMILPUnboundedJson (lp : LPInstance) (root : JsonVal) : Except String Unit := do
  let isIntArr ← parseIsIntFlagsJson lp.numVars root
  let xArr ← parseRatVecJson root ["x0", "x", "anchor", "primal"]
    ["x0Bits", "xBits", "anchorBits", "primalBits"] lp.numVars "x0"
  let dArr ← parseRatVecJson root ["d", "ray", "direction"]
    ["dBits", "rayBits", "directionBits"] lp.numVars "d"
  if hi : isIntArr.size = lp.numVars then
    if hx : xArr.size = lp.numVars then
      if hd : dArr.size = lp.numVars then
        let isInt : Fin lp.numVars → Bool := fun j => isIntArr[j.val]'(by rw [hi]; exact j.isLt)
        let x0 : Fin lp.numVars → Rat := fun j => xArr[j.val]'(by rw [hx]; exact j.isLt)
        let d : Fin lp.numVars → Rat := fun j => dArr[j.val]'(by rw [hd]; exact j.isLt)
        if checkMILPUnbounded lp isInt x0 d then
          return ()
        else if !checkLPUnbounded lp x0 d then
          verifyLPUnboundedJson lp root
        else if !checkIntegrality isInt x0 then
          throw "MILP unboundedness certificate rejected: anchor x0 is fractional on an integer column"
        else
          throw "MILP unboundedness certificate rejected: ray d is fractional on an integer column"
      else throw "internal ray vector size mismatch"
    else throw "internal anchor vector size mismatch"
  else throw "internal isInt vector size mismatch"

/-- Standalone root JSON verifier for `milp-unbounded` mode. -/
def verifyMILPUnboundedRootJson (root : JsonVal) : Except String Unit := do
  let lp ← parseLPInstanceJson root
  verifyMILPUnboundedJson lp root

/-- Parses an `LPPresolveStep` from JSON. -/
def parseLPPresolveStepJson (v : JsonVal) : Except String LPPresolveStep := do
  let kind ←
    match getOptNonNullField v ["kind", "rule", "type", "step"] with
    | some kv => kv.asString
    | none => throw "LP presolve step missing 'kind' field"
  match kind with
  | "fixedSubst" | "fixed-subst" | "fixed_subst" =>
    let col ← v.getNat "col"
    let val ← v.getField "val" >>= asRatOrBitsJson
    return .fixedSubst col val
  | "singletonRow" | "singleton-row" | "singleton_row" =>
    let origRow ←
      match getOptNonNullField v ["origRow", "row"] with
      | some rv => rv.asNat
      | none => throw "singletonRow step missing 'origRow'"
    let col ← v.getNat "col"
    let mapJson ← v.getArray "origRowIdx"
    let origRowIdx ← mapJson.mapM JsonVal.asNat
    return .singletonRow origRow col origRowIdx
  | "dualColFix" | "dual-col-fix" | "dual_col_fix" =>
    let col ← v.getNat "col"
    let toLower ← v.getBool "toLower"
    return .dualColFix col toLower
  | "zeroObjEmptyColFix" | "zero-obj-empty-col-fix" | "zero_obj_empty_col_fix" =>
    let col ← v.getNat "col"
    let val ← v.getField "val" >>= asRatOrBitsJson
    return .zeroObjEmptyColFix col val
  | "dropRedundantRow" | "drop-redundant-row" | "drop_redundant_row" =>
    let droppedRow ←
      match getOptNonNullField v ["droppedRow", "origRow", "row"] with
      | some rv => rv.asNat
      | none => throw "dropRedundantRow step missing 'droppedRow'"
    let mapJson ← v.getArray "origRowIdx"
    let origRowIdx ← mapJson.mapM JsonVal.asNat
    return .dropRedundantRow droppedRow origRowIdx
  | other =>
    throw s!"unsupported LP presolve step kind '{other}'"

/-- Verifies a single-step or multi-step LP presolve reduction (and optional reduced-LP
primal-dual optimality / safe dual postsolve bound) from JSON. -/
def verifyLPPresolveJson (root : JsonVal) : Except String (Option Rat) := do
  let lpOrig ←
    match getOptNonNullField root ["orig", "lpOrig"] with
    | some ov => parseLPInstanceJson ov
    | none => parseLPInstanceJson root
  let lpRedVal ←
    match getOptNonNullField root ["reduced", "lpReduced", "presolved"] with
    | some rv => pure rv
    | none => throw "missing required field 'reduced' (presolved LPInstance)"
  let lpRed ← parseLPInstanceJson lpRedVal
  let chain ←
    match getOptNonNullField root ["steps", "chain"] with
    | some cv =>
      let arr ← cv.asArray
      let parsed ← arr.mapM fun item => do
        let st ← parseLPPresolveStepJson item
        let nextVal ←
          match getOptNonNullField item ["after", "lpAfter", "reduced"] with
          | some nv => pure nv
          | none => pure lpRedVal
        let lpNext ← parseLPInstanceJson nextVal
        pure (st, lpNext)
      pure parsed.toList
    | none =>
      match getOptNonNullField root ["step"] with
      | some sv =>
        let st ← parseLPPresolveStepJson sv
        pure [(st, lpRed)]
      | none =>
        pure []
  if !checkLPPresolveChain lpOrig chain lpRed then
    throw "LP presolve reduction certificate failed checkLPPresolveChain"
  let hasX := (getOptNonNullField root ["xReduced", "xRed", "x", "xReducedBits", "xRedBits", "xBits"]).isSome
  let hasY := (getOptNonNullField root ["yReduced", "yRed", "y", "yReducedBits", "yRedBits", "yBits"]).isSome
  let xRedOpt : Option (Fin lpRed.numVars → Rat) ←
    if hasX then
      let xArr ← parseRatVecJson root ["xReduced", "xRed", "x"]
        ["xReducedBits", "xRedBits", "xBits"] lpRed.numVars "xReduced"
      if hx : xArr.size = lpRed.numVars then
        let xRed : Fin lpRed.numVars → Rat := fun j => xArr[j.val]'(by rw [hx]; exact j.isLt)
        if !checkPrimalFeasible lpRed xRed then
          throw "LP presolve certificate rejected: xReduced is not primal-feasible on reduced LP"
        let xOrig := liftLPPresolvePrimalChain lpOrig chain lpRed xRed
        if !checkPrimalFeasible lpOrig xOrig then
          throw "LP presolve certificate rejected: lifted primal vector is not primal-feasible on original LP"
        pure (some xRed)
      else throw "internal xReduced size mismatch"
    else
      pure none
  if !hasY then
    return none
  let yArr ← parseRatVecJson root ["yReduced", "yRed", "y"]
    ["yReducedBits", "yRedBits", "yBits"] lpRed.numRows "yReduced"
  if hy : yArr.size = lpRed.numRows then
    let yRed : Fin lpRed.numRows → Rat := fun i => yArr[i.val]'(by rw [hy]; exact i.isLt)
    let yOrig := postsolveLPDualChain lpOrig chain lpRed yRed
    match xRedOpt with
    | some xRed =>
      if !checkLPOptimal lpRed xRed yRed then
        throw "LP presolve certificate rejected: (xReduced, yReduced) is not optimal on reduced LP"
      match safeDualBound lpOrig yOrig with
      | some b => return some b
      | none => throw "postsolved dual vector failed safeDualBound on original LP"
    | none =>
      match safeDualBound lpOrig yOrig with
      | some b => return some b
      | none => throw "postsolved dual vector failed safeDualBound on original LP"
  else throw "internal yReduced size mismatch"

private def parseSignedIntFlexJson (v : JsonVal) : Except String Int :=
  match v with
  | .num _ _ _ => v.asInt
  | .str _ => do
    let r ← v.asRat
    if r.den == 1 then .ok r.num
    else .error s!"expected integer string, got rational {r.num}/{r.den}"
  | _ => .error "expected integer or integer string"

/-- JSON handler for `gcd-div-check` in `Apps/RulesCheck.lean`: evaluates and verifies
Euclidean GCD integer row-coefficient division over `Rat` rhs and `Int` coefficients. -/
def handleGcdRowDivJson (v : JsonVal) : Except String String := do
  let coeffsJson ← v.getArray "coeffs"
  let coeffs ← coeffsJson.mapM parseSignedIntFlexJson
  let sense ← v.getString "sense" >>= parseSenseJson
  let rhs ← v.getField "rhs" >>= asRatOrBitsJson
  let n := coeffs.size
  if hc : coeffs.size = n then
    let cFn : Fin n → Int := fun j => coeffs[j.val]'(by rw [hc]; exact j.isLt)
    let g := rowCoeffsGcd n cFn
    match evalGcdRowDiv sense cFn rhs with
    | none =>
      throw "gcd-div-check requires at least one non-zero integer coefficient (gcd > 0)"
    | some (gEval, out) =>
      if !checkGcdRowDiv sense cFn rhs gEval out then
        throw "internal gcd-div-check validation failed"
      let resInfeasible :=
        match out with
        | .infeasible => true
        | .reduced _ _ => false
      let resDivRhs : Int :=
        match out with
        | .infeasible => 0
        | .reduced _ r' => r'
      let resDivCoeffs : Fin n → Int :=
        match out with
        | .infeasible => fun _ => 0
        | .reduced a' _ => a'
      let divisorOpt? : Option Int ←
        match getOptNonNullField v ["divisor", "d", "candidate_g"] with
        | some dv => some <$> parseSignedIntFlexJson dv
        | none => pure none
      let divisor := divisorOpt?.getD gEval
      let validateProposedDiv := fun (dcVal drVal : JsonVal) => do
        let dcJson ← dcVal.asArray
        if dcJson.size != n then
          throw s!"'div_coeffs' length {dcJson.size} != coeffs length {n}"
        let dcArr ← dcJson.mapM parseSignedIntFlexJson
        let dr ← parseSignedIntFlexJson drVal
        if hdc : dcArr.size = n then
          let dcFn : Fin n → Int := fun j => dcArr[j.val]'(by rw [hdc]; exact j.isLt)
          if !checkGcdRowDiv sense cFn rhs divisor (.reduced dcFn dr) then
            throw "unsound gcd-div-check: divided coefficients or tightened RHS mismatch"
        else throw "internal div_coeffs size mismatch"
      match getOptNonNullField v ["expect_infeasible", "infeasible"] with
      | some iv =>
        let expInf ← iv.asBool
        if expInf then
          if !checkGcdRowDiv sense cFn rhs divisor .infeasible then
            throw s!"unsound gcd-div-check: claimed infeasible on feasible equality rhs={rhs.num}/{rhs.den} (d={divisor})"
        else
          match getOptNonNullField v ["div_coeffs", "divCoeffs"], getOptNonNullField v ["div_rhs", "divRhs"] with
          | some dcVal, some drVal => validateProposedDiv dcVal drVal
          | some _, none => throw "gcd-div-check: 'div_coeffs' requires 'div_rhs'"
          | none, some _ => throw "gcd-div-check: 'div_rhs' requires 'div_coeffs'"
          | none, none =>
            if resInfeasible then
              throw s!"gcd-div-check mismatch: expected feasible, but equality rhs={rhs.num}/{rhs.den} is not divisible by gcd={g}"
            if let some dCand := divisorOpt? then
              if !(decide (0 < dCand) && allFin3B n (fun j => decide (Int.tmod (cFn j) dCand = 0))) then
                throw s!"unsound gcd-div-check: divisor {dCand} does not divide all integer coefficients"
      | none =>
        match getOptNonNullField v ["div_coeffs", "divCoeffs"], getOptNonNullField v ["div_rhs", "divRhs"] with
        | some dcVal, some drVal => validateProposedDiv dcVal drVal
        | some _, none => throw "gcd-div-check: 'div_coeffs' requires 'div_rhs'"
        | none, some _ => throw "gcd-div-check: 'div_rhs' requires 'div_coeffs'"
        | none, none =>
          if let some dCand := divisorOpt? then
            if !(decide (0 < dCand) && allFin3B n (fun j => decide (Int.tmod (cFn j) dCand = 0))) then
              throw s!"unsound gcd-div-check: divisor {dCand} does not divide all integer coefficients"
      let divList := (List.range n).map fun j =>
        if hj : j < n then toString (resDivCoeffs ⟨j, hj⟩) else "0"
      let infeasStr := if resInfeasible then "true" else "false"
      return "{\"gcd\":" ++ toString g ++
        ",\"infeasible\":" ++ infeasStr ++
        ",\"div_rhs\":" ++ toString resDivRhs ++
        ",\"div_coeffs\":[" ++ String.intercalate "," divList ++ "]}"
  else
    throw "internal coeffs size mismatch"

end GoblinFormal.Track3B
