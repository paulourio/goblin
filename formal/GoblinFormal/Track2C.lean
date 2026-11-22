/-
GoblinFormal.Track2C: Universal Q soundness and executable certificate checkers for
lifted binary knapsack cover cuts, Barany-Van Roy-Wolsey (l, S) lot-sizing chain cuts,
multi-row aggregated & pairwise-surrogate cMIR cuts over optional bounds, and universal
`finalizeSparseCut` coefficient-drop and power-of-two scaling transformations.
-/

import GoblinFormal.Common.Basic
import GoblinFormal.Common.Json
import GoblinFormal.Track2B

namespace GoblinFormal.Track2C

open GoblinFormal
open GoblinFormal.Track2B

/-! ### 0. Pure `Init` Helpers: `Rat`, `Option Rat`, `allFin`, and Sparse Accumulation -/

private theorem rat_lt_of_lt_of_le {a b c : Rat} (hab : a < b) (hbc : b ≤ c) : a < c :=
  Rat.lt_iff_le_and_not_ge.mpr
    ⟨Rat.le_trans (Rat.le_of_lt hab) hbc, fun h => Rat.not_le.mpr hab (Rat.le_trans hbc h)⟩

private theorem rat_neg_nonneg {a : Rat} (ha : a ≤ 0) : 0 ≤ -a := by
  simpa using Rat.neg_le_neg ha

private theorem rat_sub_zero (a : Rat) : a - 0 = a := by
  rw [Rat.sub_eq_add_neg, Rat.neg_zero, Rat.add_zero]

private theorem rat_zero_sub (a : Rat) : 0 - a = -a := by
  rw [Rat.sub_eq_add_neg, Rat.zero_add]

private theorem rat_sub_mul (a b c : Rat) : (a - b) * c = a * c - b * c := by
  rw [Rat.sub_eq_add_neg, Rat.add_mul, Rat.neg_mul, ← Rat.sub_eq_add_neg]

private theorem rat_mul_sub (a b c : Rat) : a * (b - c) = a * b - a * c := by
  rw [Rat.sub_eq_add_neg, Rat.mul_add, Rat.mul_neg, ← Rat.sub_eq_add_neg]

private theorem rat_add_le_add {a b c d : Rat} (hab : a ≤ b) (hcd : c ≤ d) :
    a + c ≤ b + d :=
  Rat.le_trans (Rat.add_le_add_right.mpr hab) (Rat.add_le_add_left.mpr hcd)

private theorem rat_sub_le_sub {a b c d : Rat} (hab : a ≤ b) (hdc : d ≤ c) :
    a - c ≤ b - d := by
  rw [Rat.sub_eq_add_neg, Rat.sub_eq_add_neg]
  exact rat_add_le_add hab (Rat.neg_le_neg hdc)

private theorem rat_mul_le_mul_of_nonpos_right {a b c : Rat} (hab : a ≤ b) (hc : c ≤ 0) :
    b * c ≤ a * c := by
  have hc' : 0 ≤ -c := rat_neg_nonneg hc
  have h := Rat.mul_le_mul_of_nonneg_right hab hc'
  rw [Rat.mul_neg, Rat.mul_neg, Rat.neg_le_neg_iff] at h
  exact h

private theorem rat_mul_le_mul_of_nonpos_left {a b c : Rat} (hab : a ≤ b) (hc : c ≤ 0) :
    c * b ≤ c * a := by
  rw [Rat.mul_comm c b, Rat.mul_comm c a]
  exact rat_mul_le_mul_of_nonpos_right hab hc

private theorem rat_le_max_left {a b : Rat} : a ≤ max a b := by
  rw [Rat.max_def]
  split
  · assumption
  · exact Rat.le_refl

private theorem rat_le_max_right {a b : Rat} : b ≤ max a b := by
  rw [Rat.max_def]
  split
  · exact Rat.le_refl
  · next h => exact Rat.le_of_lt (Rat.not_le.mp h)

private theorem rat_abs_le_iff {a b : Rat} : a.abs ≤ b ↔ -b ≤ a ∧ a ≤ b := by
  rcases Rat.le_total (a := 0) (b := a) with ha | ha
  · rw [Rat.abs_of_nonneg ha]
    constructor
    · intro hab
      have hnb : -b ≤ 0 := by simpa using Rat.neg_le_neg (Rat.le_trans ha hab)
      exact ⟨Rat.le_trans hnb ha, hab⟩
    · exact And.right
  · rw [Rat.abs_of_nonpos ha]
    constructor
    · intro hnab
      have hba : -b ≤ a := Rat.neg_le_iff.mp hnab
      have hb0 : 0 ≤ b := Rat.le_trans (rat_neg_nonneg ha) hnab
      exact ⟨hba, Rat.le_trans ha hb0⟩
    · intro h
      exact Rat.neg_le_iff.mpr h.1

private theorem rat_le_abs (a : Rat) : a ≤ a.abs :=
  (rat_abs_le_iff.mp Rat.le_refl).2

private theorem rat_abs_mul (a b : Rat) : (a * b).abs = a.abs * b.abs := by
  rcases Rat.le_total (a := 0) (b := a) with ha | ha <;>
    rcases Rat.le_total (a := 0) (b := b) with hb | hb
  · rw [Rat.abs_of_nonneg ha, Rat.abs_of_nonneg hb, Rat.abs_of_nonneg (Rat.mul_nonneg ha hb)]
  · have hab : a * b ≤ 0 := by simpa [Rat.mul_zero] using Rat.mul_le_mul_of_nonneg_left hb ha
    rw [Rat.abs_of_nonneg ha, Rat.abs_of_nonpos hb, Rat.abs_of_nonpos hab, Rat.mul_neg]
  · have hab : a * b ≤ 0 := by simpa [Rat.zero_mul] using Rat.mul_le_mul_of_nonneg_right ha hb
    rw [Rat.abs_of_nonpos ha, Rat.abs_of_nonneg hb, Rat.abs_of_nonpos hab, Rat.neg_mul]
  · have hab : 0 ≤ a * b := by
      have ha' := rat_neg_nonneg ha
      have hb' := rat_neg_nonneg hb
      simpa [Rat.neg_mul, Rat.mul_neg, Rat.neg_neg] using Rat.mul_nonneg ha' hb'
    rw [Rat.abs_of_nonpos ha, Rat.abs_of_nonpos hb, Rat.abs_of_nonneg hab,
      Rat.neg_mul, Rat.mul_neg, Rat.neg_neg]

private theorem finSum_swap {m n : Nat} (f : Fin m → Fin n → Rat) :
    finSum m (fun i => finSum n (fun j => f i j)) =
      finSum n (fun j => finSum m (fun i => f i j)) := by
  induction m with
  | zero =>
    simp only [finSum_zero_n]
    exact (finSum_zero n).symm
  | succ m ih =>
    simp only [finSum_succ]
    rw [ih (fun i j => f i.castSucc j), ← finSum_add]

private def natFinSum : (n : Nat) → (Fin n → Nat) → Nat
  | 0, _ => 0
  | n + 1, f => natFinSum n (fun i => f i.castSucc) + f (Fin.last n)

private theorem natFinSum_eq_finSum (n : Nat) (f : Fin n → Nat) :
    (natFinSum n f : Rat) = finSum n (fun i => (f i : Rat)) := by
  induction n with
  | zero => rfl
  | succ n ih =>
    simp only [natFinSum, finSum_succ, Rat.natCast_add, ih (fun i => f i.castSucc)]

/-- Executable universal quantifier over `Fin n`. -/
def allFin : (n : Nat) → (Fin n → Bool) → Bool
  | 0, _ => true
  | n + 1, p => allFin n (fun i => p i.castSucc) && p (Fin.last n)

theorem allFin_eq_true_iff {n : Nat} {p : Fin n → Bool} :
    allFin n p = true ↔ ∀ i : Fin n, p i = true := by
  induction n with
  | zero =>
    constructor
    · intro _ i; exact Fin.elim0 i
    · intro _; rfl
  | succ n ih =>
    simp only [allFin, Bool.and_eq_true, ih]
    constructor
    · intro ⟨hprev, hlast⟩ i
      by_cases hi : i.val < n
      · have heq : i = (⟨i.val, hi⟩ : Fin n).castSucc := Fin.ext rfl
        rw [heq]
        exact hprev ⟨i.val, hi⟩
      · have hval : i.val = (Fin.last n).val := by
          simp only [Fin.val_last]
          have := i.isLt
          omega
        have heq : i = Fin.last n := Fin.ext hval
        rw [heq]
        exact hlast
    · intro hall
      exact ⟨fun i => hall i.castSucc, hall (Fin.last n)⟩

/-- Satisfaction of optional variable bounds `lb[j] ≤ x[j] ≤ ub[j]`. -/
def boundsSat {n : Nat} (lb ub : Fin n → Option Rat) (x : Fin n → Rat) : Prop :=
  ∀ j : Fin n, (∀ l, lb j = some l → l ≤ x j) ∧ (∀ u, ub j = some u → x j ≤ u)

/-- Integrality predicate on `Rat`. -/
def isIntVal (r : Rat) : Prop :=
  ∃ z : Int, r = (z : Rat)

/-- Binary `{0, 1}` predicate on `Rat`. -/
def isBinVal (r : Rat) : Prop :=
  r = 0 ∨ r = 1

/-- Tightened integer lower bound `ceil(l) = -floor(-l)`. -/
def effIntLB (l : Rat) : Int :=
  -((-l).floor)

/-- Tightened integer upper bound `floor(u)`. -/
def effIntUB (u : Rat) : Int :=
  u.floor

theorem effIntLB_le_of_le {l : Rat} {z : Int} (h : l ≤ (z : Rat)) :
    (effIntLB l : Rat) ≤ (z : Rat) := by
  have hneg : ((-z : Int) : Rat) ≤ -l := by
    rw [Rat.intCast_neg]
    exact Rat.neg_le_neg h
  have hfl : -z ≤ (-l).floor := Rat.le_floor_iff.mpr hneg
  have hle : -((-l).floor) ≤ z := by omega
  exact Rat.intCast_le_intCast.mpr hle

theorem le_effIntUB_of_le {u : Rat} {z : Int} (h : (z : Rat) ≤ u) :
    (z : Rat) ≤ (effIntUB u : Rat) :=
  Rat.intCast_le_intCast.mpr (Rat.le_floor_iff.mpr h)

/-- Sum of coefficients for column `j` in a sparse term list (handling duplicates). -/
def termsCoeff : List SparseTerm → Nat → Rat
  | [], _ => 0
  | t :: ts, j => (if t.col == j then t.val else 0) + termsCoeff ts j

/-- Evaluates a sparse term list at `x : Nat → Rat`. -/
def evalTermsList : List SparseTerm → (Nat → Rat) → Rat
  | [], _ => 0
  | t :: ts, x => t.val * x t.col + evalTermsList ts x

/-- Evaluates a `CutSpec` over `x : Fin n → Rat`. -/
def evalCutOnFin (n : Nat) (cut : CutSpec) (x : Fin n → Rat) : Rat :=
  evalTermsList cut.terms.toList (fun c => if h : c < n then x ⟨c, h⟩ else 0)

/-- Evaluates a `RowSpec` over `x : Fin n → Rat`. -/
def evalRowOnFin (n : Nat) (row : RowSpec) (x : Fin n → Rat) : Rat :=
  evalTermsList row.terms.toList (fun c => if h : c < n then x ⟨c, h⟩ else 0)

/-- Satisfaction of a `RowSpec` over `x : Fin n → Rat`. -/
def rowSatOnFin (n : Nat) (row : RowSpec) (x : Fin n → Rat) : Prop :=
  satisfiesSense (evalRowOnFin n row x) row.sense row.rhs

/-- Executable check that every column index in `terms` is `< n`. -/
def allTermsInBounds (n : Nat) (terms : Array SparseTerm) : Bool :=
  allFin terms.size (fun i => decide (terms[i.val].col < n))

private def guardExc (c : Bool) (msg : String) (k : Except String Unit) : Except String Unit :=
  if c then k else .error msg

private theorem guardExc_ok {c : Bool} {msg : String} {k : Except String Unit}
    (h : guardExc c msg k = .ok ()) : c = true ∧ k = .ok () := by
  unfold guardExc at h
  cases c
  · simp at h
  · exact ⟨rfl, h⟩

private theorem bool_eq_true_of_not_not {b : Bool} (h : ¬((!b) = true)) : b = true := by
  cases b
  · exfalso; exact h rfl
  · rfl

private theorem prop_of_not_not {P : Prop} [Decidable P] (h : ¬(¬P)) : P := by
  by_cases hp : P
  · exact hp
  · exact absurd hp h

theorem allTermsInBounds_iff {n : Nat} {terms : Array SparseTerm} :
    allTermsInBounds n terms = true ↔ ∀ t ∈ terms.toList, t.col < n := by
  unfold allTermsInBounds
  rw [allFin_eq_true_iff]
  constructor
  · intro hall t ht
    rw [List.mem_iff_getElem] at ht
    rcases ht with ⟨i, hi, rfl⟩
    have hi' : i < terms.size := by simpa using hi
    exact of_decide_eq_true (hall ⟨i, hi'⟩)
  · intro hall i
    have hmem : terms[i.val] ∈ terms.toList := by
      rw [List.mem_iff_getElem]
      exact ⟨i.val, by simp, rfl⟩
    exact decide_eq_true (hall _ hmem)

theorem evalTermsList_eq_finSum {n : Nat} (terms : List SparseTerm) (x : Fin n → Rat)
    (hcols : ∀ t ∈ terms, t.col < n) :
    evalTermsList terms (fun c => if h : c < n then x ⟨c, h⟩ else 0) =
      finSum n (fun j => termsCoeff terms j.val * x j) := by
  induction terms with
  | nil =>
    simp [evalTermsList, termsCoeff]
  | cons t ts ih =>
    have ht : t.col < n := hcols t (List.mem_cons_self ..)
    have hts : ∀ u ∈ ts, u.col < n := fun u hu => hcols u (List.mem_cons_of_mem t hu)
    simp only [evalTermsList, termsCoeff, ht, ↓reduceDIte]
    rw [ih hts]
    have hsingle :
        t.val * x ⟨t.col, ht⟩ =
          finSum n (fun j : Fin n => (if t.col == j.val then t.val else 0) * x j) := by
      have hcong :
          finSum n (fun j : Fin n => (if t.col == j.val then t.val else 0) * x j) =
            finSum n (fun j : Fin n => if j = ⟨t.col, ht⟩ then t.val * x ⟨t.col, ht⟩ else 0) := by
        apply finSum_congr
        intro j
        by_cases hj : j = ⟨t.col, ht⟩
        · subst hj; simp
        · have hne : (t.col == j.val) = false := by
            apply beq_false_of_ne
            intro heq
            exact hj (Fin.ext heq.symm)
          simp [hj, hne]
      rw [hcong, finSum_single]
    rw [hsingle, ← finSum_add]
    exact finSum_congr fun j => (Rat.add_mul _ _ _).symm

private def accumTermsAux (n : Nat) (acc : Array Rat) (hsz : acc.size = n) :
    List SparseTerm → { a : Array Rat // a.size = n }
  | [] => ⟨acc, hsz⟩
  | t :: ts =>
    if h : t.col < n then
      let acc' := acc.set t.col (acc[t.col]'(by omega) + t.val) (by omega)
      accumTermsAux n acc' (by simp [acc', hsz]) ts
    else
      accumTermsAux n acc hsz ts

private theorem accumTermsAux_getElem {n : Nat} (acc : Array Rat) (hsz : acc.size = n)
    (terms : List SparseTerm) (j : Fin n) :
    (accumTermsAux n acc hsz terms).1[j.val]'(by rw [(accumTermsAux n acc hsz terms).2]; exact j.isLt) =
      acc[j.val]'(by rw [hsz]; exact j.isLt) + termsCoeff terms j.val := by
  induction terms generalizing acc hsz with
  | nil =>
    simp [accumTermsAux, termsCoeff, Rat.add_zero]
  | cons t ts ih =>
    dsimp only [accumTermsAux]
    split
    · next h =>
      let acc' := acc.set t.col (acc[t.col]'(by omega) + t.val) (by omega)
      have hsz' : acc'.size = n := by simp [acc', hsz]
      rw [ih acc' hsz']
      simp only [termsCoeff]
      by_cases hjeq : t.col = j.val
      · have hbeq : (t.col == j.val) = true := beq_iff_eq.mpr hjeq
        simp only [hbeq, ↓reduceIte]
        have hget : acc'[j.val]'(by rw [hsz']; exact j.isLt) =
            acc[j.val]'(by rw [hsz]; exact j.isLt) + t.val := by
          simp [acc', hjeq]
        rw [hget, Rat.add_assoc]
      · have hbeq : (t.col == j.val) = false := beq_false_of_ne hjeq
        simp only [hbeq, Bool.false_eq_true, ↓reduceIte, Rat.zero_add]
        have hget : acc'[j.val]'(by rw [hsz']; exact j.isLt) =
            acc[j.val]'(by rw [hsz]; exact j.isLt) := by
          simp [acc', Array.getElem_set, hjeq]
        rw [hget]
    · next h =>
      have hne : t.col ≠ j.val := by omega
      have hbeq : (t.col == j.val) = false := beq_false_of_ne hne
      rw [ih acc hsz]
      simp [termsCoeff, hbeq, Rat.zero_add]

/-- Linear-time `O(n + nnz)` sparse-to-dense coefficient accumulator. -/
def accumTerms (n : Nat) (terms : Array SparseTerm) : { a : Array Rat // a.size = n } :=
  accumTermsAux n (Array.replicate n 0) (by simp) terms.toList

theorem accumTerms_getElem {n : Nat} (terms : Array SparseTerm) (j : Fin n) :
    (accumTerms n terms).1[j.val]'(by rw [(accumTerms n terms).2]; exact j.isLt) =
      termsCoeff terms.toList j.val := by
  unfold accumTerms
  rw [accumTermsAux_getElem]
  simp [Rat.zero_add]

theorem evalCutOnFin_eq_finSum_accum {n : Nat} (cut : CutSpec) (x : Fin n → Rat)
    (hcols : allTermsInBounds n cut.terms = true) :
    evalCutOnFin n cut x =
      finSum n (fun j => (accumTerms n cut.terms).1[j.val]'(by rw [(accumTerms n cut.terms).2]; exact j.isLt) * x j) := by
  unfold evalCutOnFin
  rw [evalTermsList_eq_finSum cut.terms.toList x (allTermsInBounds_iff.mp hcols)]
  exact finSum_congr fun j => by rw [accumTerms_getElem]

theorem evalRowOnFin_eq_finSum_accum {n : Nat} (row : RowSpec) (x : Fin n → Rat)
    (hcols : allTermsInBounds n row.terms = true) :
    evalRowOnFin n row x =
      finSum n (fun j => (accumTerms n row.terms).1[j.val]'(by rw [(accumTerms n row.terms).2]; exact j.isLt) * x j) := by
  unfold evalRowOnFin
  rw [evalTermsList_eq_finSum row.terms.toList x (allTermsInBounds_iff.mp hcols)]
  exact finSum_congr fun j => by rw [accumTerms_getElem]

/-! ### 1. Minimal & Lifted Binary Knapsack Cover Cuts -/

/-- Minimal binary knapsack cover cut soundness over `{0, 1}^n` in `Rat`. -/
theorem knapsack_cover_sound {n : Nat} {w z : Fin n → Rat} {inCover : Fin n → Bool} {b : Rat}
    (hw : ∀ j, 0 ≤ w j)
    (hz : ∀ j, isBinVal (z j))
    (hrow : finSum n (fun j => w j * z j) ≤ b)
    (hcover : b < finSum n (fun j => if inCover j then w j else 0)) :
    finSum n (fun j => (if inCover j then (1 : Rat) else 0) * z j) ≤
      finSum n (fun j => if inCover j then (1 : Rat) else 0) - 1 := by
  have hexZero : ∃ k : Fin n, inCover k = true ∧ z k = 0 := by
    by_cases h : ∃ k : Fin n, inCover k = true ∧ z k = 0
    · exact h
    · exfalso
      have hallOne : ∀ j : Fin n, inCover j = true → z j = 1 := by
        intro j hj
        rcases hz j with hz0 | hz1
        · exact absurd ⟨j, hj, hz0⟩ h
        · exact hz1
      have hcov_le : finSum n (fun j => if inCover j then w j else 0) ≤
          finSum n (fun j => w j * z j) := by
        apply finSum_le_finSum
        intro j
        cases hc : inCover j
        · simp only [Bool.false_eq_true, ↓reduceIte]
          rcases hz j with hz0 | hz1
          · rw [hz0, Rat.mul_zero]; exact Rat.le_refl
          · rw [hz1, Rat.mul_one]; exact hw j
        · simp only [↓reduceIte, hallOne j hc, Rat.mul_one]
          exact Rat.le_refl
      have hlt : b < b := rat_lt_of_lt_of_le hcover (Rat.le_trans hcov_le hrow)
      exact Rat.lt_irrefl hlt
  rcases hexZero with ⟨k, hkC, hzk0⟩
  have hterm : ∀ j : Fin n,
      (if inCover j then (1 : Rat) else 0) * z j ≤
        (if inCover j then (1 : Rat) else 0) - (if j = k then (1 : Rat) else 0) := by
    intro j
    by_cases hjk : j = k
    · subst hjk
      simp [hkC, hzk0, Rat.mul_zero, Rat.sub_self]
    · simp only [hjk, ↓reduceIte, rat_sub_zero]
      cases inCover j
      · simp
      · simp only [↓reduceIte, Rat.one_mul]
        rcases hz j with hz0 | hz1
        · rw [hz0]; decide
        · rw [hz1]; exact Rat.le_refl
  have hsum := finSum_le_finSum hterm
  rw [finSum_sub, finSum_single] at hsum
  exact hsum

/-- Exact minimum on `Option Rat` where `none` represents `+∞`. -/
def optRatMin : Option Rat → Option Rat → Option Rat
  | none, b => b
  | a, none => a
  | some a, some b => some (min a b)

private theorem optRatMin_le_left {a b : Option Rat} {r : Rat} (ha : a = some r) :
    ∃ r', optRatMin a b = some r' ∧ r' ≤ r := by
  subst ha
  cases b with
  | none => exact ⟨r, rfl, Rat.le_refl⟩
  | some s =>
    refine ⟨min r s, rfl, ?_⟩
    rw [Rat.min_def]
    split
    · exact Rat.le_refl
    · next h => exact Rat.le_of_lt (Rat.not_le.mp h)

private theorem optRatMin_le_right {a b : Option Rat} {r : Rat} (hb : b = some r) :
    ∃ r', optRatMin a b = some r' ∧ r' ≤ r := by
  subst hb
  cases a with
  | none => exact ⟨r, rfl, Rat.le_refl⟩
  | some s =>
    refine ⟨min s r, rfl, ?_⟩
    rw [Rat.min_def]
    split
    · assumption
    · exact Rat.le_refl

/-- Initial 0-1 knapsack lifting DP table for an empty set of processed items. -/
def coverDPInit (cap : Nat) : Fin (cap + 1) → Option Rat :=
  fun v => if v.val == 0 then some 0 else none

/-- Single 0-1 knapsack DP transition adding one item `(wk, ak)`. -/
def coverDPStep (cap : Nat) (dp : Fin (cap + 1) → Option Rat) (wk : Rat) (ak : Nat) :
    Fin (cap + 1) → Option Rat :=
  fun v =>
    let prev : Fin (cap + 1) := ⟨v.val - ak, by omega⟩
    let withK := (dp prev).map (fun r => r + wk)
    optRatMin (dp v) withK

/-- Exact DP lifting soundness test for adding an item `(wk, ak)` given table `dp`. -/
def coverDPLiftOk (cap : Nat) (b : Rat) (dp : Fin (cap + 1) → Option Rat) (wk : Rat) (ak : Nat) : Bool :=
  if h : ak == 0 then true
  else
    have hpos : 0 < ak := by
      have hne : ak ≠ 0 := fun h0 => by simp [h0] at h
      omega
    let vReq : Fin (cap + 1) := ⟨cap + 1 - ak, by omega⟩
    match dp vReq with
    | none => true
    | some r => decide (b < r + wk)

/-- Exact DP lower-bound invariant for processed items `S ⊆ Fin m`. -/
def CoverDPValid {m : Nat} (cap : Nat) (S : Fin m → Bool) (w : Fin m → Rat) (alpha : Fin m → Nat)
    (dp : Fin (cap + 1) → Option Rat) : Prop :=
  ∀ (z : Fin m → Bool) (v : Fin (cap + 1)),
    (v.val : Rat) ≤ finSum m (fun i => if S i && z i then (alpha i : Rat) else 0) →
    ∃ r : Rat, dp v = some r ∧ r ≤ finSum m (fun i => if S i && z i then w i else 0)

/-- Exact cut validity invariant for processed items `S ⊆ Fin m`. -/
def CoverCutValid {m : Nat} (cap : Nat) (b : Rat) (S : Fin m → Bool) (w : Fin m → Rat)
    (alpha : Fin m → Nat) : Prop :=
  ∀ z : Fin m → Bool,
    finSum m (fun i => if S i && z i then w i else 0) ≤ b →
    finSum m (fun i => if S i && z i then (alpha i : Rat) else 0) ≤ (cap : Rat)

private def setInsert {m : Nat} (S : Fin m → Bool) (k : Fin m) : Fin m → Bool :=
  fun i => S i || decide (i = k)

private theorem finSum_setInsert_split {m : Nat} (S : Fin m → Bool) (k : Fin m)
    (hk : S k = false) (z : Fin m → Bool) (f : Fin m → Rat) :
    finSum m (fun i => if setInsert S k i && z i then f i else 0) =
      finSum m (fun i => if S i && z i then f i else 0) + (if z k then f k else 0) := by
  have hsingle : (if z k then f k else 0) =
      finSum m (fun i : Fin m => if i = k then (if z k then f k else 0) else 0) :=
    (finSum_single k _).symm
  rw [hsingle, ← finSum_add]
  apply finSum_congr
  intro i
  by_cases hik : i = k
  · subst hik
    simp [setInsert, hk, Rat.zero_add]
  · simp [setInsert, hik, Rat.add_zero]

private theorem coverDPInit_valid {m : Nat} (cap : Nat) (w : Fin m → Rat) (alpha : Fin m → Nat) :
    CoverDPValid cap (fun _ => false) w alpha (coverDPInit cap) := by
  intro z v hv
  have hzero : finSum m (fun i => if false && z i then (alpha i : Rat) else 0) = 0 := by
    simp
  rw [hzero] at hv
  have hv0 : v.val = 0 := by
    by_cases h0 : v.val = 0
    · exact h0
    · have hpos : (0 : Rat) < (v.val : Rat) := by
        have h1 : (0 : Nat) < v.val := by omega
        exact Rat.natCast_lt_natCast.mpr h1
      exact absurd (rat_lt_of_lt_of_le hpos hv) (Rat.lt_irrefl)
  refine ⟨0, ?_, ?_⟩
  · simp [coverDPInit, hv0]
  · simp

private theorem coverDPStep_preserves {m : Nat} {cap : Nat} {S : Fin m → Bool}
    {w : Fin m → Rat} {alpha : Fin m → Nat} {dp : Fin (cap + 1) → Option Rat}
    (hdp : CoverDPValid cap S w alpha dp) (k : Fin m) (hk : S k = false) :
    CoverDPValid cap (setInsert S k) w alpha (coverDPStep cap dp (w k) (alpha k)) := by
  intro z v hv
  rw [finSum_setInsert_split S k hk z (fun i => (alpha i : Rat))] at hv
  rw [finSum_setInsert_split S k hk z w]
  cases hzk : z k
  · simp only [hzk, Bool.false_eq_true, ↓reduceIte, Rat.add_zero] at hv ⊢
    rcases hdp z v hv with ⟨r, hdr, hrle⟩
    rcases optRatMin_le_left (b := (dp ⟨v.val - alpha k, by omega⟩).map (· + w k)) hdr with ⟨r', hmin, hr'⟩
    exact ⟨r', hmin, Rat.le_trans hr' hrle⟩
  · simp only [hzk, ↓reduceIte] at hv ⊢
    have hprev_nat : ((v.val - alpha k : Nat) : Rat) ≤
        finSum m (fun i => if S i && z i then (alpha i : Rat) else 0) := by
      by_cases hle : v.val ≤ alpha k
      · have h0 : v.val - alpha k = 0 := by omega
        rw [h0]
        exact finSum_nonneg fun i => by
          split
          · exact Rat.natCast_nonneg
          · exact Rat.le_refl
      · have hcast : ((v.val - alpha k : Nat) : Rat) + (alpha k : Rat) = (v.val : Rat) := by
          rw [← Rat.natCast_add]
          congr 1
          omega
        have hadd : ((v.val - alpha k : Nat) : Rat) + (alpha k : Rat) ≤
            finSum m (fun i => if S i && z i then (alpha i : Rat) else 0) + (alpha k : Rat) := by
          rw [hcast]; exact hv
        exact Rat.add_le_add_right.mp hadd
    rcases hdp z ⟨v.val - alpha k, by omega⟩ hprev_nat with ⟨r, hdr, hrle⟩
    have hwith : (dp ⟨v.val - alpha k, by omega⟩).map (· + w k) = some (r + w k) := by
      simp [hdr]
    rcases optRatMin_le_right (a := dp v) hwith with ⟨r', hmin, hr'⟩
    exact ⟨r', hmin, Rat.le_trans hr' (Rat.add_le_add_right.mpr hrle)⟩

private theorem coverLiftStep_preserves {m : Nat} {cap : Nat} {b : Rat} {S : Fin m → Bool}
    {w : Fin m → Rat} {alpha : Fin m → Nat} {dp : Fin (cap + 1) → Option Rat}
    (hw : ∀ i, 0 ≤ w i)
    (hcut : CoverCutValid cap b S w alpha)
    (hdp : CoverDPValid cap S w alpha dp)
    (k : Fin m) (hk : S k = false)
    (hlift : coverDPLiftOk cap b dp (w k) (alpha k) = true) :
    CoverCutValid cap b (setInsert S k) w alpha := by
  intro z hrow
  rw [finSum_setInsert_split S k hk z w] at hrow
  rw [finSum_setInsert_split S k hk z (fun i => (alpha i : Rat))]
  cases hzk : z k
  · simp only [hzk, Bool.false_eq_true, ↓reduceIte, Rat.add_zero] at hrow ⊢
    exact hcut z hrow
  · simp only [hzk, ↓reduceIte] at hrow ⊢
    by_cases hak0 : alpha k = 0
    · rw [hak0]
      simp only [Rat.natCast_ofNat, Rat.add_zero]
      have hrowS : finSum m (fun i => if S i && z i then w i else 0) ≤ b := by
        have h1 : finSum m (fun i => if S i && z i then w i else 0) ≤
            finSum m (fun i => if S i && z i then w i else 0) + w k := by
          simpa [Rat.add_zero] using Rat.add_le_add_left (c := finSum m (fun i => if S i && z i then w i else 0)) |>.mpr (hw k)
        exact Rat.le_trans h1 hrow
      exact hcut z hrowS
    · let fNat : Fin m → Nat := fun i => if S i && z i then alpha i else 0
      let natSum : Nat := natFinSum m fNat
      have hnatSum_cast : (natSum : Rat) =
          finSum m (fun i => if S i && z i then (alpha i : Rat) else 0) := by
        dsimp only [natSum]
        rw [natFinSum_eq_finSum]
        apply finSum_congr
        intro i
        dsimp only [fNat]
        cases S i && z i <;> simp
      by_cases hok : natSum + alpha k ≤ cap
      · rw [← hnatSum_cast, ← Rat.natCast_add]
        exact Rat.natCast_le_natCast.mpr hok
      · exfalso
        have hge : cap + 1 - alpha k ≤ natSum := by omega
        have hge_rat : ((cap + 1 - alpha k : Nat) : Rat) ≤
            finSum m (fun i => if S i && z i then (alpha i : Rat) else 0) := by
          rw [← hnatSum_cast]
          exact Rat.natCast_le_natCast.mpr hge
        have hpos : 0 < alpha k := by omega
        rcases hdp z ⟨cap + 1 - alpha k, by omega⟩ hge_rat with ⟨r, hdr, hrle⟩
        unfold coverDPLiftOk at hlift
        have hbeq : (alpha k == 0) = false := beq_false_of_ne hak0
        simp only [hbeq, Bool.false_eq_true, ↓reduceDIte, hdr] at hlift
        have hblt : b < r + w k := of_decide_eq_true hlift
        have hle : r + w k ≤ b := Rat.le_trans (Rat.add_le_add_right.mpr hrle) hrow
        exact Rat.lt_irrefl (rat_lt_of_lt_of_le hblt hle)

/-- Prefix-indexed DP build over cover items `i < k`. -/
def coverBuildDP {m : Nat} (cap : Nat) (inCover : Fin m → Bool) (w : Fin m → Rat) :
    (k : Nat) → k ≤ m → (Fin (cap + 1) → Option Rat)
  | 0, _ => coverDPInit cap
  | k + 1, hk =>
    let dpPrev := coverBuildDP cap inCover w k (by omega)
    let idx : Fin m := ⟨k, hk⟩
    if inCover idx then coverDPStep cap dpPrev (w idx) 1 else dpPrev

private def coverPrefixSet {m : Nat} (inCover : Fin m → Bool) (k : Nat) : Fin m → Bool :=
  fun i => inCover i && decide (i.val < k)

private theorem coverBuildDP_valid {m : Nat} (cap : Nat) (inCover : Fin m → Bool)
    (w : Fin m → Rat) (alpha : Fin m → Nat) (hcovAlpha : ∀ i, inCover i = true → alpha i = 1)
    (k : Nat) (hk : k ≤ m) :
    CoverDPValid cap (coverPrefixSet inCover k) w alpha (coverBuildDP cap inCover w k hk) := by
  induction k with
  | zero =>
    have hset : coverPrefixSet inCover 0 = fun _ => false := by
      funext i; simp [coverPrefixSet]
    rw [hset]
    exact coverDPInit_valid cap w alpha
  | succ k ih =>
    dsimp only [coverBuildDP]
    let idx : Fin m := ⟨k, hk⟩
    have hnot : coverPrefixSet inCover k idx = false := by
      simp [coverPrefixSet, idx]
    cases hc : inCover idx
    · simp only [Bool.false_eq_true, ↓reduceIte]
      have hsame : coverPrefixSet inCover (k + 1) = coverPrefixSet inCover k := by
        funext i
        by_cases hi : i.val = k
        · have heq : i = idx := Fin.ext hi
          subst heq
          simp [coverPrefixSet, hc]
        · have hiff : (i.val < k + 1) ↔ (i.val < k) := by omega
          simp [coverPrefixSet, hiff]
      rw [hsame]
      exact ih (by omega)
    · simp only [↓reduceIte]
      have hsame : coverPrefixSet inCover (k + 1) = setInsert (coverPrefixSet inCover k) idx := by
        funext i
        by_cases hi : i.val = k
        · have heq : i = idx := Fin.ext hi
          subst heq
          simp [coverPrefixSet, setInsert, hc, idx]
        · have hne : i ≠ idx := fun h => hi (congrArg Fin.val h)
          have hiff : (i.val < k + 1) ↔ (i.val < k) := by omega
          simp [coverPrefixSet, setInsert, hne, hiff]
      rw [hsame, ← hcovAlpha idx hc]
      exact coverDPStep_preserves (ih (by omega)) idx hnot

/-- Executable sequential DP lift check over non-cover items `i < k`. -/
def coverLiftLoop {m : Nat} (cap : Nat) (b : Rat) (inCover : Fin m → Bool)
    (w : Fin m → Rat) (alpha : Fin m → Nat)
    (dp0 : Fin (cap + 1) → Option Rat) :
    (k : Nat) → k ≤ m → Option (Fin (cap + 1) → Option Rat)
  | 0, _ => some dp0
  | k + 1, hk =>
    match coverLiftLoop cap b inCover w alpha dp0 k (by omega) with
    | none => none
    | some dpPrev =>
      let idx : Fin m := ⟨k, hk⟩
      if inCover idx then
        some dpPrev
      else if coverDPLiftOk cap b dpPrev (w idx) (alpha idx) then
        some (coverDPStep cap dpPrev (w idx) (alpha idx))
      else
        none

private def liftedPrefixSet {m : Nat} (inCover : Fin m → Bool) (k : Nat) : Fin m → Bool :=
  fun i => inCover i || decide (i.val < k)

private theorem coverLiftLoop_sound {m : Nat} (cap : Nat) (b : Rat)
    (inCover : Fin m → Bool) (w : Fin m → Rat) (alpha : Fin m → Nat)
    (hw : ∀ i, 0 ≤ w i)
    (dp0 : Fin (cap + 1) → Option Rat)
    (hcut0 : CoverCutValid cap b inCover w alpha)
    (hdp0 : CoverDPValid cap inCover w alpha dp0)
    (k : Nat) (hk : k ≤ m) {dpOut : Fin (cap + 1) → Option Rat}
    (hrun : coverLiftLoop cap b inCover w alpha dp0 k hk = some dpOut) :
    CoverCutValid cap b (liftedPrefixSet inCover k) w alpha ∧
    CoverDPValid cap (liftedPrefixSet inCover k) w alpha dpOut := by
  induction k generalizing dpOut with
  | zero =>
    have hset : liftedPrefixSet inCover 0 = inCover := by
      funext i; simp [liftedPrefixSet]
    simp only [coverLiftLoop, Option.some.injEq] at hrun
    subst hrun
    rw [hset]
    exact ⟨hcut0, hdp0⟩
  | succ k ih =>
    dsimp only [coverLiftLoop] at hrun
    cases hprev : coverLiftLoop cap b inCover w alpha dp0 k (by omega) with
    | none => rw [hprev] at hrun; contradiction
    | some dpPrev =>
      rw [hprev] at hrun
      rcases ih (by omega) hprev with ⟨hcutPrev, hdpPrev⟩
      let idx : Fin m := ⟨k, hk⟩
      cases hc : inCover ⟨k, hk⟩
      · simp only [hc, Bool.false_eq_true, ↓reduceIte] at hrun
        cases hok : coverDPLiftOk cap b dpPrev (w ⟨k, hk⟩) (alpha ⟨k, hk⟩)
        · simp [hok] at hrun
        · simp only [hok, ↓reduceIte, Option.some.injEq] at hrun
          subst hrun
          have hnot : liftedPrefixSet inCover k idx = false := by
            simp [liftedPrefixSet, idx, hc]
          have hsame : liftedPrefixSet inCover (k + 1) = setInsert (liftedPrefixSet inCover k) idx := by
            funext i
            by_cases hi : i.val = k
            · have heq : i = idx := Fin.ext hi
              subst heq
              simp [liftedPrefixSet, setInsert, idx]
            · have hne : i ≠ idx := fun h => hi (congrArg Fin.val h)
              have hiff : (i.val < k + 1) ↔ (i.val < k) := by omega
              simp [liftedPrefixSet, setInsert, hne, hiff]
          rw [hsame]
          exact ⟨coverLiftStep_preserves hw hcutPrev hdpPrev idx hnot hok,
                 coverDPStep_preserves hdpPrev idx hnot⟩
      · simp only [hc, ↓reduceIte, Option.some.injEq] at hrun
        subst hrun
        have hsame : liftedPrefixSet inCover (k + 1) = liftedPrefixSet inCover k := by
          funext i
          by_cases hi : i.val = k
          · have heq : i = idx := Fin.ext hi
            subst heq
            simp [liftedPrefixSet, idx, hc]
          · have hiff : (i.val < k + 1) ↔ (i.val < k) := by omega
            simp [liftedPrefixSet, hiff]
        rw [hsame]
        exact ⟨hcutPrev, hdpPrev⟩

/-- Executable sequential DP lifting certificate predicate over `Fin m`. -/
def checkCoverLiftSeq {m : Nat} (cap : Nat) (b : Rat) (inCover : Fin m → Bool)
    (w : Fin m → Rat) (alpha : Fin m → Nat) : Bool :=
  let dp0 := coverBuildDP cap inCover w m (Nat.le_refl m)
  (coverLiftLoop cap b inCover w alpha dp0 m (Nat.le_refl m)).isSome

/-- Number of elements with `inCover i = true` as a `Nat`. -/
def countTrue : (m : Nat) → (Fin m → Bool) → Nat
  | 0, _ => 0
  | m + 1, p => countTrue m (fun i => p i.castSucc) + (if p (Fin.last m) then 1 else 0)

theorem countTrue_eq_finSum (m : Nat) (p : Fin m → Bool) :
    (countTrue m p : Rat) = finSum m (fun i => if p i then (1 : Rat) else 0) := by
  induction m with
  | zero => rfl
  | succ m ih =>
    simp only [countTrue, finSum_succ, Rat.natCast_add, ih (fun i => p i.castSucc)]
    cases p (Fin.last m) <;> simp

/-- Universal soundness of sequentially DP-lifted complemented binary knapsack cover cuts
derived from a general linear inequality with optional non-binary variable bounds. -/
theorem lifted_knapsack_cover_sound {n m : Nat}
    {rowA : Fin n → Rat} {lb ub : Fin n → Option Rat} {x : Fin n → Rat} {rawRHS : Rat}
    {col : Fin m → Fin n} {w : Fin m → Rat} {comp inCover : Fin m → Bool} {alpha : Fin m → Nat}
    {knapRHS cutRHS : Rat} {cutCoeff : Fin n → Rat}
    (hxbnd : boundsSat lb ub x)
    (hxBin : ∀ i : Fin m, isBinVal (x (col i)))
    (hrow : finSum n (fun j => rowA j * x j) ≤ rawRHS)
    (hw : ∀ i, 0 ≤ w i)
    (hcovPos : 1 ≤ countTrue m inCover)
    (hcovAlpha : ∀ i, inCover i = true → alpha i = 1)
    (hrelax : ∀ j : Fin n,
      let rj := rowA j - finSum m (fun i => if col i = j then (if comp i then -w i else w i) else 0)
      (0 < rj → (lb j).isSome = true) ∧ (rj < 0 → (ub j).isSome = true))
    (hknapRHS : rawRHS
      - finSum n (fun j =>
          let rj := rowA j - finSum m (fun i => if col i = j then (if comp i then -w i else w i) else 0)
          if 0 < rj then rj * (lb j).getD 0
          else if rj < 0 then rj * (ub j).getD 0
          else 0)
      + finSum m (fun i => if comp i then w i else 0) ≤ knapRHS)
    (hcover : knapRHS < finSum m (fun i => if inCover i then w i else 0))
    (hlift : checkCoverLiftSeq (countTrue m inCover - 1) knapRHS inCover w alpha = true)
    (hcutCoeff : ∀ j : Fin n,
      cutCoeff j = finSum m (fun i => if col i = j then (if comp i then -(alpha i : Rat) else (alpha i : Rat)) else 0))
    (hcutRHS : ((countTrue m inCover - 1 : Nat) : Rat) -
      finSum m (fun i => if comp i then (alpha i : Rat) else 0) ≤ cutRHS) :
    finSum n (fun j => cutCoeff j * x j) ≤ cutRHS := by
  let z : Fin m → Bool := fun i =>
    if comp i then decide (x (col i) == 0) else decide (x (col i) == 1)
  have hzVal : ∀ i : Fin m, (if z i then (1 : Rat) else 0) =
      if comp i then 1 - x (col i) else x (col i) := by
    intro i
    rcases hxBin i with hx0 | hx1
    · cases hc : comp i
      · simp [z, hc, hx0]
      · simp [z, hc, hx0, rat_sub_zero]
    · cases hc : comp i
      · simp [z, hc, hx1]
      · simp [z, hc, hx1, Rat.sub_self]
  have hitemSum :
      finSum n (fun j => finSum m (fun i => if col i = j then (if comp i then -w i else w i) else 0) * x j) =
        finSum m (fun i => if z i then w i else 0) - finSum m (fun i => if comp i then w i else 0) := by
    have h1 :
        finSum n (fun j => finSum m (fun i => if col i = j then (if comp i then -w i else w i) else 0) * x j) =
        finSum m (fun i => finSum n (fun j => (if col i = j then (if comp i then -w i else w i) else 0) * x j)) := by
      rw [← finSum_swap]
      exact finSum_congr fun j => by rw [finSum_mul_const]
    rw [h1]
    have h2 : ∀ i : Fin m,
        finSum n (fun j => (if col i = j then (if comp i then -w i else w i) else 0) * x j) =
          (if z i then w i else 0) - (if comp i then w i else 0) := by
      intro i
      have hcong :
          finSum n (fun j => (if col i = j then (if comp i then -w i else w i) else 0) * x j) =
          finSum n (fun j => if j = col i then (if comp i then -w i else w i) * x (col i) else 0) := by
        apply finSum_congr
        intro j
        by_cases hj : j = col i
        · subst hj; simp
        · have hne : col i ≠ j := Ne.symm hj
          simp [hj, hne]
      rw [hcong, finSum_single]
      have hzw : (if z i then w i else 0) = w i * (if z i then (1 : Rat) else 0) := by
        cases z i <;> simp
      rw [hzw, hzVal i]
      cases comp i
      · simp [rat_sub_zero]
      · simp only [↓reduceIte, rat_mul_sub, Rat.mul_one, Rat.neg_mul]
        rw [Rat.sub_eq_add_neg, Rat.sub_eq_add_neg, Rat.add_comm (w i), Rat.add_assoc,
          Rat.add_neg_cancel, Rat.add_zero]
    rw [finSum_congr h2, finSum_sub]
  have hrelax_le :
      finSum n (fun j =>
        let rj := rowA j - finSum m (fun i => if col i = j then (if comp i then -w i else w i) else 0)
        if 0 < rj then rj * (lb j).getD 0
        else if rj < 0 then rj * (ub j).getD 0
        else 0) ≤
      finSum n (fun j =>
        (rowA j - finSum m (fun i => if col i = j then (if comp i then -w i else w i) else 0)) * x j) := by
    apply finSum_le_finSum
    intro j
    dsimp only
    generalize hrj : rowA j - finSum m (fun i => if col i = j then (if comp i then -w i else w i) else 0) = rj
    have hrelj : (0 < rj → (lb j).isSome = true) ∧ (rj < 0 → (ub j).isSome = true) := by
      rw [← hrj]; exact hrelax j
    by_cases hpos : 0 < rj
    · simp only [hpos, ↓reduceIte]
      have hsome := hrelj.1 hpos
      cases hlb : lb j with
      | none => simp [hlb] at hsome
      | some l =>
        simp only [Option.getD_some]
        exact Rat.mul_le_mul_of_nonneg_left ((hxbnd j).1 l hlb) (Rat.le_of_lt hpos)
    · by_cases hneg : rj < 0
      · simp only [hpos, hneg, ↓reduceIte]
        have hsome := hrelj.2 hneg
        cases hub : ub j with
        | none => simp [hub] at hsome
        | some u =>
          simp only [Option.getD_some]
          exact rat_mul_le_mul_of_nonpos_left ((hxbnd j).2 u hub) (Rat.le_of_lt hneg)
      · have hr0 : rj = 0 :=
          Rat.le_antisymm (Rat.not_lt.mp hpos) (Rat.not_lt.mp hneg)
        simp [hr0]
  have hknap_row : finSum m (fun i => if z i then w i else 0) ≤ knapRHS := by
    have hsplit :
        finSum n (fun j => rowA j * x j) =
          finSum n (fun j => (rowA j - finSum m (fun i => if col i = j then (if comp i then -w i else w i) else 0)) * x j) +
          finSum n (fun j => finSum m (fun i => if col i = j then (if comp i then -w i else w i) else 0) * x j) := by
      rw [← finSum_add]
      apply finSum_congr
      intro j
      rw [← Rat.add_mul, Rat.sub_add_cancel]
    rw [hsplit, hitemSum] at hrow
    have hstep := Rat.le_trans (Rat.add_le_add_right.mpr hrelax_le) hrow
    have hstep2 := (Rat.le_sub_iff).mpr (by rwa [Rat.add_comm] at hstep)
    have hstep3 := (Rat.sub_right_le_iff_le_add).mp hstep2
    exact Rat.le_trans hstep3 hknapRHS
  let cap := countTrue m inCover - 1
  have hcut0 : CoverCutValid cap knapRHS inCover w alpha := by
    intro z' hz'
    have hrow_z' : finSum m (fun i => w i * (if inCover i && z' i then (1 : Rat) else 0)) ≤ knapRHS := by
      have heq : finSum m (fun i => w i * (if inCover i && z' i then (1 : Rat) else 0)) =
          finSum m (fun i => if inCover i && z' i then w i else 0) :=
        finSum_congr fun i => by cases inCover i && z' i <;> simp
      rwa [heq]
    have hbin_z' : ∀ i : Fin m, isBinVal (if inCover i && z' i then (1 : Rat) else 0) := by
      intro i; cases inCover i && z' i <;> simp [isBinVal]
    have hcov := knapsack_cover_sound hw hbin_z' hrow_z' hcover
    have hlhs :
        finSum m (fun i => if inCover i && z' i then (alpha i : Rat) else 0) =
        finSum m (fun i => (if inCover i then (1 : Rat) else 0) * (if inCover i && z' i then (1 : Rat) else 0)) := by
      apply finSum_congr
      intro i
      cases hc : inCover i
      · simp
      · cases z' i <;> simp [hcovAlpha i hc]
    have hrhs : finSum m (fun i => if inCover i then (1 : Rat) else 0) - 1 = (cap : Rat) := by
      rw [← countTrue_eq_finSum]
      have hsub : ((countTrue m inCover - 1 : Nat) : Rat) + ((1 : Nat) : Rat) = (countTrue m inCover : Rat) := by
        rw [← Rat.natCast_add]
        congr 1
        omega
      dsimp [cap]
      have hsub' : ((countTrue m inCover - 1 : Nat) : Rat) + 1 = (countTrue m inCover : Rat) := by
        simpa [Rat.natCast_ofNat] using hsub
      rw [← hsub', Rat.add_sub_cancel]
    rw [hlhs, ← hrhs]
    exact hcov
  have hdp0_raw := coverBuildDP_valid cap inCover w alpha hcovAlpha m (Nat.le_refl m)
  have hset0 : coverPrefixSet inCover m = inCover := by
    funext i; simp [coverPrefixSet, i.isLt]
  rw [hset0] at hdp0_raw
  unfold checkCoverLiftSeq at hlift
  change (coverLiftLoop cap knapRHS inCover w alpha (coverBuildDP cap inCover w m (Nat.le_refl m)) m (Nat.le_refl m)).isSome = true at hlift
  cases hloop : coverLiftLoop cap knapRHS inCover w alpha (coverBuildDP cap inCover w m (Nat.le_refl m)) m (Nat.le_refl m) with
  | none =>
    rw [hloop] at hlift
    simp at hlift
  | some dpOut =>
    have hfinal := (coverLiftLoop_sound cap knapRHS inCover w alpha hw _ hcut0 hdp0_raw m (Nat.le_refl m) hloop).1
    have hsetFull : liftedPrefixSet inCover m = fun _ => true := by
      funext i; simp [liftedPrefixSet, i.isLt]
    rw [hsetFull] at hfinal
    have hrow_full : finSum m (fun i => if true && z i then w i else 0) ≤ knapRHS := by
      simpa using hknap_row
    have hcut_z : finSum m (fun i => if z i then (alpha i : Rat) else 0) ≤ (cap : Rat) := by
      simpa using hfinal z hrow_full
    have hcut_lhs :
        finSum n (fun j => cutCoeff j * x j) =
          finSum m (fun i => if z i then (alpha i : Rat) else 0) -
          finSum m (fun i => if comp i then (alpha i : Rat) else 0) := by
      rw [finSum_congr (fun j => by rw [hcutCoeff j])]
      have h1 :
          finSum n (fun j => finSum m (fun i => if col i = j then (if comp i then -(alpha i : Rat) else (alpha i : Rat)) else 0) * x j) =
          finSum m (fun i => finSum n (fun j => (if col i = j then (if comp i then -(alpha i : Rat) else (alpha i : Rat)) else 0) * x j)) := by
        rw [← finSum_swap]
        exact finSum_congr fun j => by rw [finSum_mul_const]
      rw [h1]
      have h2 : ∀ i : Fin m,
          finSum n (fun j => (if col i = j then (if comp i then -(alpha i : Rat) else (alpha i : Rat)) else 0) * x j) =
            (if z i then (alpha i : Rat) else 0) - (if comp i then (alpha i : Rat) else 0) := by
        intro i
        have hcong :
            finSum n (fun j => (if col i = j then (if comp i then -(alpha i : Rat) else (alpha i : Rat)) else 0) * x j) =
            finSum n (fun j => if j = col i then (if comp i then -(alpha i : Rat) else (alpha i : Rat)) * x (col i) else 0) := by
          apply finSum_congr
          intro j
          by_cases hj : j = col i
          · subst hj; simp
          · have hne : col i ≠ j := Ne.symm hj
            simp [hj, hne]
        rw [hcong, finSum_single]
        have hza : (if z i then (alpha i : Rat) else 0) = (alpha i : Rat) * (if z i then (1 : Rat) else 0) := by
          cases z i <;> simp
        rw [hza, hzVal i]
        cases comp i
        · simp [rat_sub_zero]
        · simp only [↓reduceIte, rat_mul_sub, Rat.mul_one, Rat.neg_mul]
          rw [Rat.sub_eq_add_neg, Rat.sub_eq_add_neg, Rat.add_comm ((alpha i : Rat)), Rat.add_assoc,
            Rat.add_neg_cancel, Rat.add_zero]
      rw [finSum_congr h2, finSum_sub]
    rw [hcut_lhs]
    exact Rat.le_trans (rat_sub_le_sub hcut_z Rat.le_refl) hcutRHS

/-- Item entry in an executable knapsack cover lifting certificate. -/
structure CoverItemCert where
  col : Nat
  weight : Rat
  comp : Bool
  inCover : Bool
  alpha : Nat
  deriving Repr, Inhabited

/-- Complete certificate for a minimal/lifted binary knapsack cover cut (with optional positive scaling). -/
structure KnapsackCoverCert where
  numVars : Nat
  lb : Array (Option Rat)
  ub : Array (Option Rat)
  row : RowSpec
  sign : Rat
  items : Array CoverItemCert
  knapRHS : Rat
  scale : Rat := 1
  cut : CutSpec
  deriving Repr, Inhabited

/-- Executable verifier for `KnapsackCoverCert`. -/
def checkKnapsackCoverCert (cert : KnapsackCoverCert) : Except String Unit :=
  let n := cert.numVars
  let m := cert.items.size
  let inCoverFn : Fin m → Bool := fun i => cert.items[i.val].inCover
  let wFn : Fin m → Rat := fun i => cert.items[i.val].weight
  let compFn : Fin m → Bool := fun i => cert.items[i.val].comp
  let alphaFn : Fin m → Nat := fun i => cert.items[i.val].alpha
  let covCnt := countTrue m inCoverFn
  let cap := covCnt - 1
  let rowDense := accumTerms n cert.row.terms
  let cutDense := accumTerms n cert.cut.terms
  let itemRowTerms : Array SparseTerm := cert.items.map fun it =>
    { col := it.col, val := if it.comp then -it.weight else it.weight }
  let itemCutTerms : Array SparseTerm := cert.items.map fun it =>
    { col := it.col, val := if it.comp then -(it.alpha : Rat) else (it.alpha : Rat) }
  let itemRowDense := accumTerms n itemRowTerms
  let itemCutDense := accumTerms n itemCutTerms
  let itemsOk := allFin m fun i =>
    let it := cert.items[i.val]
    decide (it.col < n) && decide (0 ≤ it.weight) &&
      (!it.inCover || decide (it.alpha == 1))
  let relaxOk := allFin n fun j =>
    let rj := cert.sign * rowDense.1[j.val]'(by rw [rowDense.2]; exact j.isLt) -
      itemRowDense.1[j.val]'(by rw [itemRowDense.2]; exact j.isLt)
    if 0 < rj then (cert.lb[j.val]!).isSome
    else if rj < 0 then (cert.ub[j.val]!).isSome
    else true
  let relaxShift : Rat := finSum n fun j =>
    let rj := cert.sign * rowDense.1[j.val]'(by rw [rowDense.2]; exact j.isLt) -
      itemRowDense.1[j.val]'(by rw [itemRowDense.2]; exact j.isLt)
    if 0 < rj then rj * (cert.lb[j.val]!).getD 0
    else if rj < 0 then rj * (cert.ub[j.val]!).getD 0
    else 0
  let compShift : Rat := finSum m fun i => if compFn i then wFn i else 0
  let exactKnapRHS := cert.sign * cert.row.rhs - relaxShift + compShift
  let coverWeight : Rat := finSum m fun i => if inCoverFn i then wFn i else 0
  let cutCoeffOk := allFin n fun j =>
    decide (cutDense.1[j.val]'(by rw [cutDense.2]; exact j.isLt) =
      cert.scale * itemCutDense.1[j.val]'(by rw [itemCutDense.2]; exact j.isLt))
  let cutCompShift : Rat := finSum m fun i => if compFn i then (alphaFn i : Rat) else 0
  let exactCutRHS := cert.scale * ((cap : Rat) - cutCompShift)
  guardExc (cert.lb.size == n && cert.ub.size == n) s!"cover cert bound arrays must have length {n}" <|
  guardExc (allTermsInBounds n cert.row.terms) "cover cert row has out-of-bounds column index" <|
  guardExc (allTermsInBounds n cert.cut.terms) "cover cert cut has out-of-bounds column index" <|
  guardExc (decide (0 < cert.scale)) s!"cover cert scale={cert.scale} must be > 0" <|
  guardExc (rowLegalMuSignSpec cert.row.sense cert.sign) s!"cover cert sign={cert.sign} illegal for sense={repr cert.row.sense}" <|
  guardExc itemsOk "cover cert item has out-of-bounds col, negative weight, or cover alpha != 1" <|
  guardExc (decide (1 ≤ covCnt)) "cover cert must have at least one item in cover" <|
  guardExc relaxOk "cover cert non-binary relaxation missing required finite bound" <|
  guardExc (decide (exactKnapRHS ≤ cert.knapRHS)) s!"cover cert knapRHS={cert.knapRHS} < exact relaxed RHS {exactKnapRHS}" <|
  guardExc (decide (cert.knapRHS < coverWeight)) s!"cover cert coverWeight={coverWeight} <= knapRHS={cert.knapRHS}" <|
  guardExc (checkCoverLiftSeq cap cert.knapRHS inCoverFn wFn alphaFn) "cover cert failed sequential DP lifting validity check" <|
  guardExc cutCoeffOk "cover cert cut coefficients do not match scaled lifted cover terms" <|
  guardExc (decide (exactCutRHS ≤ cert.cut.rhs)) s!"cover cert cut.rhs={cert.cut.rhs} < exact scaled cut RHS {exactCutRHS}" <|
  .ok ()

private theorem accumTerms_map_eq_finSum {n : Nat} (items : Array CoverItemCert)
    (hcol : ∀ i : Fin items.size, items[i.val].col < n)
    (f : CoverItemCert → Rat) (j : Fin n) :
    (accumTerms n (items.map fun it => { col := it.col, val := f it })).1[j.val]'(by
      rw [(accumTerms n _).2]; exact j.isLt) =
    finSum items.size (fun i : Fin items.size =>
      if (⟨items[i.val].col, hcol i⟩ : Fin n) = j then f items[i.val] else 0) := by
  rw [accumTerms_getElem]
  have hlist : (items.map fun it => ({ col := it.col, val := f it } : SparseTerm)).toList =
      items.toList.map (fun it => ({ col := it.col, val := f it } : SparseTerm)) := by
    simp
  rw [hlist]
  suffices h : ∀ (k : Nat) (hk : k ≤ items.size),
      termsCoeff (items.toList.take k |>.map fun it => ({ col := it.col, val := f it } : SparseTerm)) j.val =
      finSum k (fun i : Fin k =>
        if (⟨items[i.val]'(by omega) |>.col, hcol ⟨i.val, by omega⟩⟩ : Fin n) = j then
          f (items[i.val]'(by omega))
        else 0) by
    have hfull := h items.size (Nat.le_refl _)
    have htake : items.toList.take items.size = items.toList := List.take_of_length_le (by simp)
    rw [htake] at hfull
    exact hfull
  intro k hk
  induction k with
  | zero => simp [termsCoeff]
  | succ k ih =>
    have hlt : k < items.toList.length := by simp; omega
    have htake_succ : items.toList.take (k + 1) = items.toList.take k ++ [items.toList[k]] :=
      List.take_succ_eq_append_getElem hlt
    rw [htake_succ, List.map_append]
    have happend : ∀ (l1 l2 : List SparseTerm),
        termsCoeff (l1 ++ l2) j.val = termsCoeff l1 j.val + termsCoeff l2 j.val := by
      intro l1 l2
      induction l1 with
      | nil => simp [termsCoeff, Rat.zero_add]
      | cons a as ih1 => simp [termsCoeff, ih1, Rat.add_assoc]
    rw [happend, ih (by omega), finSum_succ]
    congr 1
    simp only [List.map_cons, List.map_nil, termsCoeff, Rat.add_zero]
    have hget : items.toList[k] = items[k]'(by omega) := rfl
    rw [hget]
    by_cases heq : (items[k]'(by omega)).col = j.val
    · simp [heq]
    · have hfin : (⟨(items[k]'(by omega)).col, hcol ⟨k, hk⟩⟩ : Fin n) ≠ j :=
        fun h => heq (congrArg Fin.val h)
      have hbeq : ((items[k]'(by omega)).col == j.val) = false := beq_false_of_ne heq
      simp [hbeq, hfin]

/-- Universal soundness of `checkKnapsackCoverCert` over all feasible `x : Fin cert.numVars → Rat`. -/
theorem checkKnapsackCoverCert_sound (cert : KnapsackCoverCert)
    (hok : checkKnapsackCoverCert cert = .ok ())
    (hlbLen : cert.lb.size = cert.numVars)
    (hubLen : cert.ub.size = cert.numVars)
    (x : Fin cert.numVars → Rat)
    (hxbnd : boundsSat (fun j => cert.lb[j.val]'(by omega)) (fun j => cert.ub[j.val]'(by omega)) x)
    (hxBin : ∀ i : Fin cert.items.size, (h : cert.items[i.val].col < cert.numVars) →
      isBinVal (x ⟨cert.items[i.val].col, h⟩))
    (hrowSat : rowSatOnFin cert.numVars cert.row x) :
    evalCutOnFin cert.numVars cert.cut x ≤ cert.cut.rhs := by
  unfold checkKnapsackCoverCert at hok
  rcases guardExc_ok hok with ⟨_, hok⟩
  rcases guardExc_ok hok with ⟨hrowColsT, hok⟩
  rcases guardExc_ok hok with ⟨hcutColsT, hok⟩
  rcases guardExc_ok hok with ⟨hscaleD, hok⟩
  rcases guardExc_ok hok with ⟨hsignOk, hok⟩
  rcases guardExc_ok hok with ⟨hitemsT, hok⟩
  rcases guardExc_ok hok with ⟨hcovCntD, hok⟩
  rcases guardExc_ok hok with ⟨hrelaxT, hok⟩
  rcases guardExc_ok hok with ⟨hknapD, hok⟩
  rcases guardExc_ok hok with ⟨hcoverD, hok⟩
  rcases guardExc_ok hok with ⟨hliftT, hok⟩
  rcases guardExc_ok hok with ⟨hcutCoeffT, hok⟩
  rcases guardExc_ok hok with ⟨hcutRHSD, _⟩
  have hscalePos : 0 < cert.scale := of_decide_eq_true hscaleD
  have hitemsAll := allFin_eq_true_iff.mp hitemsT
  have hcolLt : ∀ i : Fin cert.items.size, cert.items[i.val].col < cert.numVars := by
    intro i
    have hi := hitemsAll i
    simp only [Bool.and_eq_true, decide_eq_true_eq] at hi
    exact hi.1.1
  have hwNN : ∀ i : Fin cert.items.size, 0 ≤ cert.items[i.val].weight := by
    intro i
    have hi := hitemsAll i
    simp only [Bool.and_eq_true, decide_eq_true_eq] at hi
    exact hi.1.2
  have hcovAlpha : ∀ i : Fin cert.items.size, cert.items[i.val].inCover = true → cert.items[i.val].alpha = 1 := by
    intro i hc
    have hi := hitemsAll i
    simp only [Bool.and_eq_true, Bool.or_eq_true, Bool.not_eq_true', decide_eq_true_eq] at hi
    rcases hi.2 with hfalse | heq
    · rw [hc] at hfalse; contradiction
    · simpa using heq
  have hcovPos : 1 ≤ countTrue cert.items.size (fun i => cert.items[i.val].inCover) :=
    of_decide_eq_true hcovCntD
  have hrelaxAll := allFin_eq_true_iff.mp hrelaxT
  have hknapLe := of_decide_eq_true hknapD
  have hcoverLt := of_decide_eq_true hcoverD
  have hcutCoeffAll := allFin_eq_true_iff.mp hcutCoeffT
  have hcutRHSLe := of_decide_eq_true hcutRHSD
  let n := cert.numVars
  let m := cert.items.size
  let rowDense := accumTerms n cert.row.terms
  let cutDense := accumTerms n cert.cut.terms
  let colFn : Fin m → Fin n := fun i => ⟨cert.items[i.val].col, hcolLt i⟩
  let wFn : Fin m → Rat := fun i => cert.items[i.val].weight
  let compFn : Fin m → Bool := fun i => cert.items[i.val].comp
  let inCoverFn : Fin m → Bool := fun i => cert.items[i.val].inCover
  let alphaFn : Fin m → Nat := fun i => cert.items[i.val].alpha
  let lbFn : Fin n → Option Rat := fun j => cert.lb[j.val]'(by omega)
  let ubFn : Fin n → Option Rat := fun j => cert.ub[j.val]'(by omega)
  have hrowLe : finSum n (fun j => (cert.sign * rowDense.1[j.val]'(by rw [rowDense.2]; exact j.isLt)) * x j) ≤
      cert.sign * cert.row.rhs := by
    have hsat : satisfiesSense (finSum n (fun j => rowDense.1[j.val]'(by rw [rowDense.2]; exact j.isLt) * x j))
        cert.row.sense cert.row.rhs := by
      unfold rowSatOnFin at hrowSat
      rwa [evalRowOnFin_eq_finSum_accum cert.row x hrowColsT] at hrowSat
    exact row_agg_legal_sound hsignOk hsat
  have hitemRowEq : ∀ j : Fin n,
      (accumTerms n (cert.items.map fun it => { col := it.col, val := if it.comp then -it.weight else it.weight })).1[j.val]'(by
        rw [(accumTerms n _).2]; exact j.isLt) =
      finSum m (fun i => if colFn i = j then (if compFn i then -wFn i else wFn i) else 0) :=
    fun j => accumTerms_map_eq_finSum cert.items hcolLt (fun it => if it.comp then -it.weight else it.weight) j
  have hitemCutEq : ∀ j : Fin n,
      (accumTerms n (cert.items.map fun it => { col := it.col, val := if it.comp then -(it.alpha : Rat) else (it.alpha : Rat) })).1[j.val]'(by
        rw [(accumTerms n _).2]; exact j.isLt) =
      finSum m (fun i => if colFn i = j then (if compFn i then -(alphaFn i : Rat) else (alphaFn i : Rat)) else 0) :=
    fun j => accumTerms_map_eq_finSum cert.items hcolLt (fun it => if it.comp then -(it.alpha : Rat) else (it.alpha : Rat)) j
  have hlbBang : ∀ j : Fin n, cert.lb[j.val]! = lbFn j := by
    intro j; exact getElem!_pos cert.lb j.val (by omega)
  have hubBang : ∀ j : Fin n, cert.ub[j.val]! = ubFn j := by
    intro j; exact getElem!_pos cert.ub j.val (by omega)
  have hrelaxProp : ∀ j : Fin n,
      let rj := cert.sign * rowDense.1[j.val]'(by rw [rowDense.2]; exact j.isLt) -
        finSum m (fun i => if colFn i = j then (if compFn i then -wFn i else wFn i) else 0)
      (0 < rj → (lbFn j).isSome = true) ∧ (rj < 0 → (ubFn j).isSome = true) := by
    intro j
    have hj := hrelaxAll j
    dsimp only at hj ⊢
    rw [hitemRowEq j, hlbBang j, hubBang j] at hj
    dsimp only [rowDense, n] at hj ⊢
    constructor
    · intro hpos
      rw [ite_eq_left_iff.mpr (fun h => absurd hpos h)] at hj
      exact hj
    · intro hneg
      have hnpos : ¬(0 < cert.sign * (accumTerms cert.numVars cert.row.terms).1[j.val]'(by rw [(accumTerms cert.numVars cert.row.terms).2]; exact j.isLt) -
          finSum m (fun i => if colFn i = j then (if compFn i then -wFn i else wFn i) else 0)) :=
        Rat.not_lt.mpr (Rat.le_of_lt hneg)
      rw [ite_eq_right_iff.mpr (fun h => absurd h hnpos), ite_eq_left_iff.mpr (fun h => absurd hneg h)] at hj
      exact hj
  have hknapLe' : cert.sign * cert.row.rhs
      - finSum n (fun j =>
          let rj := cert.sign * rowDense.1[j.val]'(by rw [rowDense.2]; exact j.isLt) -
            finSum m (fun i => if colFn i = j then (if compFn i then -wFn i else wFn i) else 0)
          if 0 < rj then rj * (lbFn j).getD 0
          else if rj < 0 then rj * (ubFn j).getD 0
          else 0)
      + finSum m (fun i => if compFn i then wFn i else 0) ≤ cert.knapRHS := by
    have hcong :
        finSum n (fun j =>
          let rj := cert.sign * rowDense.1[j.val]'(by rw [rowDense.2]; exact j.isLt) -
            (accumTerms n (cert.items.map fun it => { col := it.col, val := if it.comp then -it.weight else it.weight })).1[j.val]'(by
              rw [(accumTerms n _).2]; exact j.isLt)
          if 0 < rj then rj * (cert.lb[j.val]!).getD 0
          else if rj < 0 then rj * (cert.ub[j.val]!).getD 0
          else 0) =
        finSum n (fun j =>
          let rj := cert.sign * rowDense.1[j.val]'(by rw [rowDense.2]; exact j.isLt) -
            finSum m (fun i => if colFn i = j then (if compFn i then -wFn i else wFn i) else 0)
          if 0 < rj then rj * (lbFn j).getD 0
          else if rj < 0 then rj * (ubFn j).getD 0
          else 0) :=
      finSum_congr fun j => by rw [hitemRowEq j, hlbBang j, hubBang j]
    rwa [hcong] at hknapLe
  let unscaledCutCoeff : Fin n → Rat := fun j =>
    finSum m (fun i => if colFn i = j then (if compFn i then -(alphaFn i : Rat) else (alphaFn i : Rat)) else 0)
  have hunscaled := lifted_knapsack_cover_sound
    (rowA := fun j => cert.sign * rowDense.1[j.val]'(by rw [rowDense.2]; exact j.isLt))
    (lb := lbFn) (ub := ubFn) (x := x) (rawRHS := cert.sign * cert.row.rhs)
    (col := colFn) (w := wFn) (comp := compFn) (inCover := inCoverFn) (alpha := alphaFn)
    (knapRHS := cert.knapRHS)
    (cutRHS := ((countTrue m inCoverFn - 1 : Nat) : Rat) - finSum m (fun i => if compFn i then (alphaFn i : Rat) else 0))
    (cutCoeff := unscaledCutCoeff)
    hxbnd (fun i => hxBin i (hcolLt i)) hrowLe hwNN hcovPos hcovAlpha hrelaxProp hknapLe'
    hcoverLt hliftT (fun _ => rfl) Rat.le_refl
  have hscaled := Rat.mul_le_mul_of_nonneg_left hunscaled (Rat.le_of_lt hscalePos)
  rw [← finSum_const_mul] at hscaled
  rw [evalCutOnFin_eq_finSum_accum cert.cut x hcutColsT]
  have hmatch :
      finSum n (fun j => cutDense.1[j.val]'(by rw [cutDense.2]; exact j.isLt) * x j) =
      finSum n (fun j => cert.scale * (unscaledCutCoeff j * x j)) := by
    apply finSum_congr
    intro j
    have hj := of_decide_eq_true (hcutCoeffAll j)
    rw [hj, hitemCutEq j, Rat.mul_assoc]
  rw [hmatch]
  exact Rat.le_trans hscaled hcutRHSLe

/-! ### 2. Barany-Van Roy-Wolsey `(l, S)` Lot-Sizing Chain Inequalities -/

/-- Exact prefix demand `∑_{s < k} d_s`. -/
def prefixDemand {L : Nat} (d : Fin L → Rat) (k : Nat) : Rat :=
  finSum L (fun s => if s.val < k then d s else 0)

/-- Exact suffix demand `∑_{s ≥ τ} d_s`. -/
def suffixDemand {L : Nat} (d : Fin L → Rat) (τ : Fin L) : Rat :=
  finSum L (fun s => if τ.val ≤ s.val then d s else 0)

private theorem prefix_add_suffix_demand {L : Nat} (d : Fin L → Rat) (τ : Fin L) :
    prefixDemand d τ.val + suffixDemand d τ = finSum L d := by
  unfold prefixDemand suffixDemand
  rw [← finSum_add]
  apply finSum_congr
  intro s
  by_cases h : s.val < τ.val
  · have hge : ¬(τ.val ≤ s.val) := by omega
    simp [h, hge, Rat.add_zero]
  · have hge : τ.val ≤ s.val := by omega
    simp [h, hge, Rat.zero_add]

private theorem flow_chain_prefix_sum {L : Nat} {I0 : Rat} {x inv d : Fin L → Rat}
    (hflow : ∀ τ : Fin L,
      (if _h : τ.val = 0 then I0 else inv ⟨τ.val - 1, by omega⟩) + x τ - inv τ = d τ)
    (k : Nat) (hk : k ≤ L) :
    I0 + finSum L (fun s => if s.val < k then x s else 0) =
      prefixDemand d k + (if _h : k = 0 then I0 else inv ⟨k - 1, by omega⟩) := by
  induction k with
  | zero =>
    have hd0 : prefixDemand d 0 = 0 := by simp [prefixDemand]
    simp [hd0, Rat.add_zero, Rat.zero_add]
  | succ k ih =>
    let τ : Fin L := ⟨k, hk⟩
    have hxStep :
        finSum L (fun s => if s.val < k + 1 then x s else 0) =
          finSum L (fun s => if s.val < k then x s else 0) + x τ := by
      have hsingle : x τ = finSum L (fun s => if s = τ then x τ else 0) := (finSum_single τ _).symm
      rw [hsingle, ← finSum_add]
      apply finSum_congr
      intro s
      by_cases hs : s.val = k
      · have heq : s = τ := Fin.ext hs
        subst heq
        simp [τ, Rat.zero_add]
      · have hne : s ≠ τ := fun h => hs (congrArg Fin.val h)
        have hiff : (s.val < k + 1) ↔ (s.val < k) := by omega
        simp [hne, hiff, Rat.add_zero]
    have hdStep : prefixDemand d (k + 1) = prefixDemand d k + d τ := by
      unfold prefixDemand
      have hsingle : d τ = finSum L (fun s => if s = τ then d τ else 0) := (finSum_single τ _).symm
      rw [hsingle, ← finSum_add]
      apply finSum_congr
      intro s
      by_cases hs : s.val = k
      · have heq : s = τ := Fin.ext hs
        subst heq
        simp [τ, Rat.zero_add]
      · have hne : s ≠ τ := fun h => hs (congrArg Fin.val h)
        have hiff : (s.val < k + 1) ↔ (s.val < k) := by omega
        simp [hne, hiff, Rat.add_zero]
    rw [hxStep, ← Rat.add_assoc, ih (by omega), hdStep]
    have hf := hflow τ
    have hf' : (if _h : k = 0 then I0 else inv ⟨k - 1, by omega⟩) + x τ = d τ + inv τ := by
      have hsub := congrArg (· + inv τ) hf
      simpa [τ, Rat.sub_add_cancel] using hsub
    have hsucc : (if _h : k + 1 = 0 then I0 else inv ⟨k + 1 - 1, by omega⟩) = inv τ := by
      simp [τ]
    rw [hsucc, Rat.add_assoc, hf', ← Rat.add_assoc]

private theorem exists_min_fin {L : Nat} {P : Fin L → Prop} (h : ∃ τ : Fin L, P τ) :
    ∃ τ : Fin L, P τ ∧ ∀ τ' : Fin L, τ'.val < τ.val → ¬P τ' := by
  induction L with
  | zero =>
    rcases h with ⟨τ, _⟩
    exact Fin.elim0 τ
  | succ L ih =>
    by_cases hprev : ∃ i : Fin L, P i.castSucc
    · rcases ih hprev with ⟨i, hiP, hiMin⟩
      refine ⟨i.castSucc, hiP, ?_⟩
      intro τ' hlt
      have hlt' : τ'.val < L := Nat.lt_trans hlt i.isLt
      have heq : τ' = (⟨τ'.val, hlt'⟩ : Fin L).castSucc := Fin.ext rfl
      rw [heq]
      exact hiMin ⟨τ'.val, hlt'⟩ hlt
    · rcases h with ⟨τ, hτP⟩
      have hlast : τ = Fin.last L := by
        by_cases hlt : τ.val < L
        · exfalso
          exact hprev ⟨⟨τ.val, hlt⟩, by simpa using hτP⟩
        · have hval : τ.val = (Fin.last L).val := by
            simp only [Fin.val_last]
            have := τ.isLt
            omega
          exact Fin.ext hval
      subst hlast
      refine ⟨Fin.last L, hτP, ?_⟩
      intro τ' hlt hP'
      exact hprev ⟨⟨τ'.val, hlt⟩, by simpa using hP'⟩

/-- VUB linkage lemma: `x ≥ 0`, `x - cap * y ≤ rhs ≤ 0`, `y = 0` forces `x = 0`. -/
theorem vub_zero_forces_prod_zero {x cap rhs y : Rat}
    (hx : 0 ≤ x) (hrhs : rhs ≤ 0) (hvub : x - cap * y ≤ rhs) (hy0 : y = 0) :
    x = 0 := by
  rw [hy0, Rat.mul_zero, rat_sub_zero] at hvub
  exact Rat.le_antisymm (Rat.le_trans hvub hrhs) hx

/-- Universal soundness of Barany-Van Roy-Wolsey `(l, S)` lot-sizing flow-conservation
inequalities (including shared binary setup deduplication and outward-rounded demands). -/
theorem lot_sizing_lS_sound {L : Nat}
    {I0 : Rat} {x inv d : Fin L → Rat} {binCol : Fin L → Nat}
    {yVar : Nat → Rat} {inS : Fin L → Bool} {coeffY : Fin L → Rat} {cutDem : Rat}
    (hI0 : 0 ≤ I0)
    (hx : ∀ τ, 0 ≤ x τ)
    (hinv : ∀ τ, 0 ≤ inv τ)
    (hyBin : ∀ τ, isBinVal (yVar (binCol τ)))
    (hvub : ∀ τ, yVar (binCol τ) = 0 → x τ = 0)
    (hflow : ∀ τ : Fin L,
      (if _h : τ.val = 0 then I0 else inv ⟨τ.val - 1, by omega⟩) + x τ - inv τ = d τ)
    (hcoeffY_nn : ∀ τ, 0 ≤ coeffY τ)
    (hcoeffY_cov : ∀ τ : Fin L, inS τ = true →
      suffixDemand d τ ≤ coeffY τ ∨
      ∃ τ' : Fin L, τ'.val < τ.val ∧ inS τ' = true ∧ binCol τ' = binCol τ)
    (hcutDem : cutDem ≤ finSum L d) :
    cutDem ≤ I0 + finSum L (fun τ => if inS τ then 0 else x τ) +
      finSum L (fun τ => coeffY τ * yVar (binCol τ)) := by
  have hy_nn : ∀ τ, 0 ≤ yVar (binCol τ) := by
    intro τ
    rcases hyBin τ with h0 | h1
    · rw [h0]; exact Rat.le_refl
    · rw [h1]; decide
  have hyTerm_nn : ∀ τ, 0 ≤ coeffY τ * yVar (binCol τ) := fun τ =>
    Rat.mul_nonneg (hcoeffY_nn τ) (hy_nn τ)
  by_cases hex : ∃ τ : Fin L, inS τ = true ∧ yVar (binCol τ) = 1
  · rcases exists_min_fin hex with ⟨τ, ⟨hτS, hτY1⟩, hτMin⟩
    have hcoeff_ge : suffixDemand d τ ≤ coeffY τ := by
      rcases hcoeffY_cov τ hτS with hle | ⟨τ', hlt, hS', hsame⟩
      · exact hle
      · exfalso
        exact hτMin τ' hlt ⟨hS', by rwa [hsame]⟩
    have hprefix_eq := flow_chain_prefix_sum hflow τ.val (Nat.le_of_lt τ.isLt)
    let remInv : Rat := if _h : τ.val = 0 then I0 else inv ⟨τ.val - 1, by omega⟩
    have hinv_nn : 0 ≤ remInv := by
      dsimp only [remInv]
      split
      · exact hI0
      · exact hinv _
    have hprefix_le : prefixDemand d τ.val ≤ I0 + finSum L (fun s => if s.val < τ.val then x s else 0) := by
      rw [hprefix_eq]
      change prefixDemand d τ.val ≤ prefixDemand d τ.val + remInv
      have hadd := Rat.add_le_add_left (c := prefixDemand d τ.val) |>.mpr hinv_nn
      rwa [Rat.add_zero] at hadd
    have hpref_x_le : finSum L (fun s => if s.val < τ.val then x s else 0) ≤
        finSum L (fun s => if inS s then 0 else x s) := by
      apply finSum_le_finSum
      intro s
      by_cases hslt : s.val < τ.val
      · simp only [hslt, ↓reduceIte]
        cases hsS : inS s
        · simp
        · simp only [↓reduceIte]
          have hne1 : yVar (binCol s) ≠ 1 := fun h1 => hτMin s hslt ⟨hsS, h1⟩
          have hy0 : yVar (binCol s) = 0 := by
            rcases hyBin s with h0 | h1
            · exact h0
            · exact absurd h1 hne1
          rw [hvub s hy0]
          exact Rat.le_refl
      · simp only [hslt, ↓reduceIte]
        cases inS s
        · exact hx s
        · exact Rat.le_refl
    have hpart1 : prefixDemand d τ.val ≤ I0 + finSum L (fun s => if inS s then 0 else x s) :=
      Rat.le_trans hprefix_le (Rat.add_le_add_left.mpr hpref_x_le)
    have hpart2 : suffixDemand d τ ≤ finSum L (fun s => coeffY s * yVar (binCol s)) := by
      have h1 : suffixDemand d τ ≤ coeffY τ * yVar (binCol τ) := by
        rw [hτY1, Rat.mul_one]
        exact hcoeff_ge
      exact Rat.le_trans h1 (le_finSum_of_nonneg hyTerm_nn τ)
    have htot := rat_add_le_add hpart1 hpart2
    rw [prefix_add_suffix_demand d τ] at htot
    exact Rat.le_trans hcutDem htot
  · have hflowL := flow_chain_prefix_sum hflow L (Nat.le_refl L)
    let remInvL : Rat := if _h : L = 0 then I0 else inv ⟨L - 1, by omega⟩
    have hinvL_nn : 0 ≤ remInvL := by
      dsimp only [remInvL]
      split
      · exact hI0
      · exact hinv _
    have hallX : finSum L (fun s => if s.val < L then x s else 0) =
        finSum L (fun s => if inS s then 0 else x s) := by
      apply finSum_congr
      intro s
      simp only [s.isLt, ↓reduceIte]
      cases hsS : inS s
      · simp
      · simp only [↓reduceIte]
        have hy0 : yVar (binCol s) = 0 := by
          rcases hyBin s with h0 | h1
          · exact h0
          · exact absurd ⟨s, hsS, h1⟩ hex
        exact hvub s hy0
    have hallD : prefixDemand d L = finSum L d :=
      finSum_congr fun s => by simp [s.isLt]
    rw [hallX, hallD] at hflowL
    have hbase : finSum L d ≤ I0 + finSum L (fun s => if inS s then 0 else x s) := by
      rw [hflowL]
      change finSum L d ≤ finSum L d + remInvL
      have hadd := Rat.add_le_add_left (c := finSum L d) |>.mpr hinvL_nn
      rwa [Rat.add_zero] at hadd
    have haddY : I0 + finSum L (fun s => if inS s then 0 else x s) ≤
        I0 + finSum L (fun s => if inS s then 0 else x s) +
          finSum L (fun s => coeffY s * yVar (binCol s)) := by
      simpa [Rat.add_zero] using Rat.add_le_add_left
        (c := I0 + finSum L (fun s => if inS s then 0 else x s)) |>.mpr (finSum_nonneg hyTerm_nn)
    exact Rat.le_trans hcutDem (Rat.le_trans hbase haddY)

/-- Single period node in a Barany-Van Roy-Wolsey `(l, S)` lot-sizing chain certificate. -/
structure LotSizingNodeCert where
  prodCol : Nat
  invOutCol : Nat
  binCol : Nat
  demand : Rat
  vubCap : Rat
  vubRHS : Rat := 0
  inS : Bool
  coeffY : Rat
  deriving Repr, Inhabited

/-- Complete certificate for an `(l, S)` lot-sizing flow-conservation cut (with optional positive scaling). -/
structure LotSizingCutCert where
  numVars : Nat
  invInCol : Option Nat
  nodes : Array LotSizingNodeCert
  cutDem : Rat
  scale : Rat := 1
  cut : CutSpec
  deriving Repr, Inhabited

/-- Expected unscaled `(l, S)` cut coefficient for column `j`. -/
def lotSizingExpectedCoeff (cert : LotSizingCutCert) (j : Nat) : Rat :=
  (match cert.invInCol with
   | some c => if c == j then (-1 : Rat) else 0
   | none => 0) +
  finSum cert.nodes.size (fun τ =>
    let nd := cert.nodes[τ.val]
    (if nd.inS then 0 else if nd.prodCol == j then (-1 : Rat) else 0) +
    (if nd.binCol == j then -nd.coeffY else 0))

/-- Checks whether an earlier node `τ' < τ` in `S` already shares `binCol`. -/
def hasEarlierBinInS (nodes : Array LotSizingNodeCert) (τ : Nat) (binCol : Nat) : Bool :=
  allFin τ (fun i =>
    if _h : i.val < nodes.size then
      !(nodes[i.val].inS && decide (nodes[i.val].binCol == binCol))
    else true) == false

private theorem hasEarlierBinInS_true_iff {nodes : Array LotSizingNodeCert} {τ : Fin nodes.size}
    (h : hasEarlierBinInS nodes τ.val nodes[τ.val].binCol = true) :
    ∃ τ' : Fin nodes.size, τ'.val < τ.val ∧ nodes[τ'.val].inS = true ∧
      nodes[τ'.val].binCol = nodes[τ.val].binCol := by
  unfold hasEarlierBinInS at h
  have hfalse : allFin τ.val (fun i =>
      if _hlt : i.val < nodes.size then
        !(nodes[i.val].inS && decide (nodes[i.val].binCol == nodes[τ.val].binCol))
      else true) = false := by
    simpa using h
  exact Classical.byContradiction fun hnone => by
    have hall : ∀ i : Fin τ.val,
        (if _hlt : i.val < nodes.size then
          !(nodes[i.val].inS && decide (nodes[i.val].binCol == nodes[τ.val].binCol))
        else true) = true := by
      intro i
      have hlt : i.val < nodes.size := Nat.lt_trans i.isLt τ.isLt
      simp only [hlt, ↓reduceDIte, Bool.not_eq_true', Bool.and_eq_false_iff, decide_eq_false_iff_not]
      by_cases hS : nodes[i.val].inS = true
      · right
        intro heq
        exact hnone ⟨⟨i.val, hlt⟩, i.isLt, hS, by simpa using heq⟩
      · left
        cases h' : nodes[i.val].inS <;> simp_all
    have htrue := allFin_eq_true_iff.mpr hall
    rw [htrue] at hfalse
    contradiction

/-- Executable verifier for `LotSizingCutCert`. -/
def checkLotSizingCutCert (cert : LotSizingCutCert) : Except String Unit :=
  let n := cert.numVars
  let L := cert.nodes.size
  let invOk : Bool := match cert.invInCol with | none => true | some c => decide (c < n)
  let dFn : Fin L → Rat := fun τ => cert.nodes[τ.val].demand
  let nodesOk := allFin L fun τ =>
    let nd := cert.nodes[τ.val]
    let colOk := decide (nd.prodCol < n) && decide (nd.invOutCol < n) && decide (nd.binCol < n)
    let valOk := decide (0 ≤ nd.demand) && decide (0 ≤ nd.vubCap) &&
      decide (nd.vubRHS ≤ 0) && decide (0 ≤ nd.coeffY)
    let covOk := if nd.inS then
        decide (suffixDemand dFn τ ≤ nd.coeffY) || hasEarlierBinInS cert.nodes τ.val nd.binCol
      else
        decide (nd.coeffY == 0)
    colOk && valOk && covOk
  let totalDem := finSum L dFn
  let cutDense := accumTerms n cert.cut.terms
  let coeffOk := allFin n fun j =>
    decide (cutDense.1[j.val]'(by rw [cutDense.2]; exact j.isLt) =
      cert.scale * lotSizingExpectedCoeff cert j.val)
  guardExc (L != 0) "lot-sizing cert requires at least one flow node" <|
  guardExc (decide (0 < cert.scale)) s!"lot-sizing cert scale={cert.scale} must be > 0" <|
  guardExc (allTermsInBounds n cert.cut.terms) "lot-sizing cert cut has out-of-bounds column index" <|
  guardExc invOk "lot-sizing cert invInCol out of bounds" <|
  guardExc nodesOk "lot-sizing cert node failed bounds, VUB, or suffix-demand coverage check" <|
  guardExc (decide (cert.cutDem ≤ totalDem)) s!"lot-sizing cert cutDem={cert.cutDem} > totalDemand={totalDem}" <|
  guardExc coeffOk "lot-sizing cert cut coefficients do not match scaled (l, S) terms" <|
  guardExc (decide (cert.scale * (-cert.cutDem) ≤ cert.cut.rhs))
    s!"lot-sizing cert cut.rhs={cert.cut.rhs} < scaled -cutDem={cert.scale * (-cert.cutDem)}" <|
  .ok ()

/-- Universal soundness of `checkLotSizingCutCert` over all feasible `x : Fin cert.numVars → Rat`. -/
theorem checkLotSizingCutCert_sound (cert : LotSizingCutCert)
    (hok : checkLotSizingCutCert cert = .ok ())
    (x : Fin cert.numVars → Rat)
    (hInvIn : ∀ c, cert.invInCol = some c → (h : c < cert.numVars) → 0 ≤ x ⟨c, h⟩)
    (hNodesFeas : ∀ τ : Fin cert.nodes.size,
      (hp : cert.nodes[τ.val].prodCol < cert.numVars) →
      (hi : cert.nodes[τ.val].invOutCol < cert.numVars) →
      (hb : cert.nodes[τ.val].binCol < cert.numVars) →
      let nd := cert.nodes[τ.val]
      let prevInv : Rat :=
        if _h0 : τ.val = 0 then
          match cert.invInCol with
          | none => 0
          | some c => if hc : c < cert.numVars then x ⟨c, hc⟩ else 0
        else
          let prevCol := cert.nodes[τ.val - 1].invOutCol
          if hpv : prevCol < cert.numVars then x ⟨prevCol, hpv⟩ else 0
      0 ≤ x ⟨nd.prodCol, hp⟩ ∧
      0 ≤ x ⟨nd.invOutCol, hi⟩ ∧
      isBinVal (x ⟨nd.binCol, hb⟩) ∧
      x ⟨nd.prodCol, hp⟩ - nd.vubCap * x ⟨nd.binCol, hb⟩ ≤ nd.vubRHS ∧
      prevInv + x ⟨nd.prodCol, hp⟩ - x ⟨nd.invOutCol, hi⟩ = nd.demand) :
    evalCutOnFin cert.numVars cert.cut x ≤ cert.cut.rhs := by
  unfold checkLotSizingCutCert at hok
  rcases guardExc_ok hok with ⟨_, hok⟩
  rcases guardExc_ok hok with ⟨hscaleD, hok⟩
  rcases guardExc_ok hok with ⟨hcutColsT, hok⟩
  rcases guardExc_ok hok with ⟨hinvOkT, hok⟩
  rcases guardExc_ok hok with ⟨hnodesOkT, hok⟩
  rcases guardExc_ok hok with ⟨hcutDemD, hok⟩
  rcases guardExc_ok hok with ⟨hcoeffOkT, hok⟩
  rcases guardExc_ok hok with ⟨hcutRHSD, _⟩
  have hscalePos : 0 < cert.scale := of_decide_eq_true hscaleD
  have hInvInLt : ∀ c, cert.invInCol = some c → c < cert.numVars := by
    intro c hc
    simp only [hc] at hinvOkT
    exact of_decide_eq_true hinvOkT
  have hnodesAll := allFin_eq_true_iff.mp hnodesOkT
  have hcutDemLe : cert.cutDem ≤ finSum cert.nodes.size (fun τ => cert.nodes[τ.val].demand) :=
    of_decide_eq_true hcutDemD
  have hcoeffAll := allFin_eq_true_iff.mp hcoeffOkT
  have hcutRHSLe : cert.scale * (-cert.cutDem) ≤ cert.cut.rhs :=
    of_decide_eq_true hcutRHSD
  have hpLt : ∀ τ : Fin cert.nodes.size, cert.nodes[τ.val].prodCol < cert.numVars := by
    intro τ; have h := hnodesAll τ; simp only [Bool.and_eq_true, decide_eq_true_eq] at h; exact h.1.1.1.1
  have hiLt : ∀ τ : Fin cert.nodes.size, cert.nodes[τ.val].invOutCol < cert.numVars := by
    intro τ; have h := hnodesAll τ; simp only [Bool.and_eq_true, decide_eq_true_eq] at h; exact h.1.1.1.2
  have hbLt : ∀ τ : Fin cert.nodes.size, cert.nodes[τ.val].binCol < cert.numVars := by
    intro τ; have h := hnodesAll τ; simp only [Bool.and_eq_true, decide_eq_true_eq] at h; exact h.1.1.2
  have hvubRHSLe : ∀ τ : Fin cert.nodes.size, cert.nodes[τ.val].vubRHS ≤ 0 := by
    intro τ; have h := hnodesAll τ; simp only [Bool.and_eq_true, decide_eq_true_eq] at h; exact h.1.2.1.2
  have hcoeffYNN : ∀ τ : Fin cert.nodes.size, 0 ≤ cert.nodes[τ.val].coeffY := by
    intro τ; have h := hnodesAll τ; simp only [Bool.and_eq_true, decide_eq_true_eq] at h; exact h.1.2.2
  let xExt : Nat → Rat := fun c => if h : c < cert.numVars then x ⟨c, h⟩ else 0
  let I0 : Rat := match cert.invInCol with | none => 0 | some c => xExt c
  have hI0_nn : 0 ≤ I0 := by
    dsimp [I0, xExt]
    cases hI : cert.invInCol with
    | none => exact Rat.le_refl
    | some c =>
      have hc := hInvInLt c hI
      simp only [hc, ↓reduceDIte]
      exact hInvIn c hI hc
  let xProd : Fin cert.nodes.size → Rat := fun τ => x ⟨cert.nodes[τ.val].prodCol, hpLt τ⟩
  let xInv : Fin cert.nodes.size → Rat := fun τ => x ⟨cert.nodes[τ.val].invOutCol, hiLt τ⟩
  let dFn : Fin cert.nodes.size → Rat := fun τ => cert.nodes[τ.val].demand
  let binColFn : Fin cert.nodes.size → Nat := fun τ => cert.nodes[τ.val].binCol
  let inSFn : Fin cert.nodes.size → Bool := fun τ => cert.nodes[τ.val].inS
  let coeffYFn : Fin cert.nodes.size → Rat := fun τ => cert.nodes[τ.val].coeffY
  have hFeas : ∀ τ : Fin cert.nodes.size,
      0 ≤ xProd τ ∧ 0 ≤ xInv τ ∧ isBinVal (xExt (binColFn τ)) ∧
      (xExt (binColFn τ) = 0 → xProd τ = 0) ∧
      (if _h : τ.val = 0 then I0 else xInv ⟨τ.val - 1, by omega⟩) + xProd τ - xInv τ = dFn τ := by
    intro τ
    have hf := hNodesFeas τ (hpLt τ) (hiLt τ) (hbLt τ)
    dsimp only at hf
    rcases hf with ⟨hpx, hix, hbin, hvub, hfl⟩
    have hyExt : xExt (binColFn τ) = x ⟨cert.nodes[τ.val].binCol, hbLt τ⟩ := by
      simp [xExt, binColFn, hbLt τ]
    refine ⟨hpx, hix, by rwa [hyExt], fun hy0 => ?_, ?_⟩
    · rw [hyExt] at hy0
      exact vub_zero_forces_prod_zero hpx (hvubRHSLe τ) hvub hy0
    · have hprev : (if _h : τ.val = 0 then I0 else xInv ⟨τ.val - 1, by omega⟩) =
          (if h0 : τ.val = 0 then
            match cert.invInCol with
            | none => 0
            | some c => if hc : c < cert.numVars then x ⟨c, hc⟩ else 0
          else
            let prevCol := cert.nodes[τ.val - 1].invOutCol
            if hpv : prevCol < cert.numVars then x ⟨prevCol, hpv⟩ else 0) := by
        by_cases h0 : τ.val = 0
        · simp [h0, I0, xExt]
        · simp only [h0, ↓reduceDIte, xInv]
          have hprevLt := hiLt ⟨τ.val - 1, by omega⟩
          simp [hprevLt]
      rw [hprev]
      exact hfl
  have hcovCond : ∀ τ : Fin cert.nodes.size, inSFn τ = true →
      suffixDemand dFn τ ≤ coeffYFn τ ∨
      ∃ τ' : Fin cert.nodes.size, τ'.val < τ.val ∧ inSFn τ' = true ∧ binColFn τ' = binColFn τ := by
    intro τ hS
    have h := hnodesAll τ
    dsimp only [inSFn] at hS
    dsimp only at h
    simp only [Bool.and_eq_true] at h
    have hcov := h.2
    simp only [hS, ↓reduceIte, Bool.or_eq_true, decide_eq_true_eq] at hcov
    rcases hcov with hle | hearl
    · exact Or.inl hle
    · exact Or.inr (hasEarlierBinInS_true_iff hearl)
  have hls := lot_sizing_lS_sound
    (I0 := I0) (x := xProd) (inv := xInv) (d := dFn) (binCol := binColFn)
    (yVar := xExt) (inS := inSFn) (coeffY := coeffYFn) (cutDem := cert.cutDem)
    hI0_nn (fun τ => (hFeas τ).1) (fun τ => (hFeas τ).2.1)
    (fun τ => (hFeas τ).2.2.1) (fun τ => (hFeas τ).2.2.2.1) (fun τ => (hFeas τ).2.2.2.2)
    hcoeffYNN hcovCond hcutDemLe
  have hneg :
      -(I0 + finSum cert.nodes.size (fun τ => if inSFn τ then 0 else xProd τ) +
        finSum cert.nodes.size (fun τ => coeffYFn τ * xExt (binColFn τ))) ≤ -cert.cutDem :=
    Rat.neg_le_neg hls
  have hexpSum :
      finSum cert.numVars (fun j => lotSizingExpectedCoeff cert j.val * x j) =
        -(I0 + finSum cert.nodes.size (fun τ => if inSFn τ then 0 else xProd τ) +
          finSum cert.nodes.size (fun τ => coeffYFn τ * xExt (binColFn τ))) := by
    unfold lotSizingExpectedCoeff
    have hsplit1 :
        finSum cert.numVars (fun j =>
          ((match cert.invInCol with | some c => if c == j.val then (-1 : Rat) else 0 | none => 0) +
           finSum cert.nodes.size (fun τ =>
             (if cert.nodes[τ.val].inS then 0 else if cert.nodes[τ.val].prodCol == j.val then (-1 : Rat) else 0) +
             (if cert.nodes[τ.val].binCol == j.val then -cert.nodes[τ.val].coeffY else 0))) * x j) =
        finSum cert.numVars (fun j =>
          (match cert.invInCol with | some c => if c == j.val then (-1 : Rat) else 0 | none => 0) * x j) +
        finSum cert.numVars (fun j =>
          finSum cert.nodes.size (fun τ =>
            (if cert.nodes[τ.val].inS then 0 else if cert.nodes[τ.val].prodCol == j.val then (-1 : Rat) else 0) +
            (if cert.nodes[τ.val].binCol == j.val then -cert.nodes[τ.val].coeffY else 0)) * x j) := by
      rw [← finSum_add]
      exact finSum_congr fun j => Rat.add_mul _ _ _
    rw [hsplit1]
    have hpartI0 :
        finSum cert.numVars (fun j =>
          (match cert.invInCol with | some c => if c == j.val then (-1 : Rat) else 0 | none => 0) * x j) = -I0 := by
      dsimp [I0, xExt]
      cases hI : cert.invInCol with
      | none => simp
      | some c =>
        have hc := hInvInLt c hI
        simp only [hc, ↓reduceDIte]
        have hcong :
            finSum cert.numVars (fun j : Fin cert.numVars => (if c == j.val then (-1 : Rat) else 0) * x j) =
            finSum cert.numVars (fun j : Fin cert.numVars => if j = ⟨c, hc⟩ then -x ⟨c, hc⟩ else 0) := by
          apply finSum_congr
          intro j
          by_cases hj : j = ⟨c, hc⟩
          · subst hj; simp [Rat.neg_mul, Rat.one_mul]
          · have hne : (c == j.val) = false := beq_false_of_ne (fun h => hj (Fin.ext h.symm))
            simp [hj, hne]
        rw [hcong, finSum_single]
    have hpartNodes :
        finSum cert.numVars (fun j =>
          finSum cert.nodes.size (fun τ =>
            (if cert.nodes[τ.val].inS then 0 else if cert.nodes[τ.val].prodCol == j.val then (-1 : Rat) else 0) +
            (if cert.nodes[τ.val].binCol == j.val then -cert.nodes[τ.val].coeffY else 0)) * x j) =
        -(finSum cert.nodes.size (fun τ => if inSFn τ then 0 else xProd τ)) +
        -(finSum cert.nodes.size (fun τ => coeffYFn τ * xExt (binColFn τ))) := by
      have hswap :
          finSum cert.numVars (fun j =>
            finSum cert.nodes.size (fun τ =>
              (if cert.nodes[τ.val].inS then 0 else if cert.nodes[τ.val].prodCol == j.val then (-1 : Rat) else 0) +
              (if cert.nodes[τ.val].binCol == j.val then -cert.nodes[τ.val].coeffY else 0)) * x j) =
          finSum cert.nodes.size (fun τ =>
            finSum cert.numVars (fun j =>
              ((if cert.nodes[τ.val].inS then 0 else if cert.nodes[τ.val].prodCol == j.val then (-1 : Rat) else 0) +
               (if cert.nodes[τ.val].binCol == j.val then -cert.nodes[τ.val].coeffY else 0)) * x j)) := by
        rw [← finSum_swap]
        exact finSum_congr fun j => by rw [finSum_mul_const]
      rw [hswap, ← finSum_neg, ← finSum_neg, ← finSum_add]
      apply finSum_congr
      intro τ
      rw [finSum_congr (fun j => Rat.add_mul _ _ _), finSum_add]
      have h1 :
          finSum cert.numVars (fun j => (if cert.nodes[τ.val].inS then 0 else if cert.nodes[τ.val].prodCol == j.val then (-1 : Rat) else 0) * x j) =
            -(if inSFn τ then 0 else xProd τ) := by
        cases hS : cert.nodes[τ.val].inS
        · simp only [Bool.false_eq_true, ↓reduceIte, inSFn, hS, xProd]
          have hcong :
              finSum cert.numVars (fun j : Fin cert.numVars => (if cert.nodes[τ.val].prodCol == j.val then (-1 : Rat) else 0) * x j) =
              finSum cert.numVars (fun j : Fin cert.numVars => if j = ⟨cert.nodes[τ.val].prodCol, hpLt τ⟩ then -x ⟨cert.nodes[τ.val].prodCol, hpLt τ⟩ else 0) := by
            apply finSum_congr
            intro j
            by_cases hj : j = ⟨cert.nodes[τ.val].prodCol, hpLt τ⟩
            · subst hj; simp [Rat.neg_mul, Rat.one_mul]
            · have hne : (cert.nodes[τ.val].prodCol == j.val) = false := beq_false_of_ne (fun h => hj (Fin.ext h.symm))
              simp [hj, hne]
          rw [hcong, finSum_single]
        · simp [inSFn, hS]
      have h2 :
          finSum cert.numVars (fun j => (if cert.nodes[τ.val].binCol == j.val then -cert.nodes[τ.val].coeffY else 0) * x j) =
            -(coeffYFn τ * xExt (binColFn τ)) := by
        have hcong :
            finSum cert.numVars (fun j : Fin cert.numVars => (if cert.nodes[τ.val].binCol == j.val then -cert.nodes[τ.val].coeffY else 0) * x j) =
            finSum cert.numVars (fun j : Fin cert.numVars => if j = ⟨cert.nodes[τ.val].binCol, hbLt τ⟩ then -(coeffYFn τ * x ⟨cert.nodes[τ.val].binCol, hbLt τ⟩) else 0) := by
          apply finSum_congr
          intro j
          by_cases hj : j = ⟨cert.nodes[τ.val].binCol, hbLt τ⟩
          · subst hj; simp [coeffYFn, Rat.neg_mul]
          · have hne : (cert.nodes[τ.val].binCol == j.val) = false := beq_false_of_ne (fun h => hj (Fin.ext h.symm))
            simp [hj, hne]
        rw [hcong, finSum_single]
        simp [xExt, binColFn, hbLt τ]
      rw [h1, h2]
    rw [hpartI0, hpartNodes, Rat.neg_add, Rat.neg_add, Rat.add_assoc]
  rw [evalCutOnFin_eq_finSum_accum cert.cut x hcutColsT]
  have hmatch :
      finSum cert.numVars (fun j => (accumTerms cert.numVars cert.cut.terms).1[j.val]'(by rw [(accumTerms cert.numVars cert.cut.terms).2]; exact j.isLt) * x j) =
      cert.scale * finSum cert.numVars (fun j => lotSizingExpectedCoeff cert j.val * x j) := by
    rw [← finSum_const_mul]
    apply finSum_congr
    intro j
    have hj := of_decide_eq_true (hcoeffAll j)
    rw [hj, Rat.mul_assoc]
  rw [hmatch, hexpSum]
  exact Rat.le_trans (Rat.mul_le_mul_of_nonneg_left hneg (Rat.le_of_lt hscalePos)) hcutRHSLe

/-! ### 3. Multi-Row Aggregated cMIR & Pairwise Surrogate Cuts Over Optional Bounds -/

/-- Exact column error compensation term over optional bounds:
zero when `cv = exactCoeff` (valid for unbounded/semi-bounded columns!), otherwise
`|cv - exactCoeff| * max(|l|, |u|)` requiring both finite bounds. -/
def aggColErrTerm (cv exactCoeff : Rat) (lb ub : Option Rat) : Option Rat :=
  if cv == exactCoeff then some 0
  else
    match lb, ub with
    | some l, some u => some ((cv - exactCoeff).abs * max l.abs u.abs)
    | _, _ => none

/-- Soundness of error-bounded aggregated row materialization over arbitrary `Option Rat` bounds
(allowing unbounded and semi-bounded columns whenever `cv j = exactCoeff j`). -/
theorem row_agg_error_bound_opt_sound {n : Nat}
    {cv exactCoeff errTerm x : Fin n → Rat} {lb ub : Fin n → Option Rat}
    {exactRHS rhsErr aggRHS : Rat}
    (hexact : finSum n (fun j => exactCoeff j * x j) ≤ exactRHS)
    (hxbnd : boundsSat lb ub x)
    (herr : ∀ j, ∃ e, aggColErrTerm (cv j) (exactCoeff j) (lb j) (ub j) = some e ∧ e ≤ errTerm j)
    (hrhsErr : 0 ≤ rhsErr)
    (hrhs : exactRHS + rhsErr + finSum n errTerm ≤ aggRHS) :
    finSum n (fun j => cv j * x j) ≤ aggRHS := by
  have hterm : ∀ j : Fin n, cv j * x j ≤ exactCoeff j * x j + errTerm j := by
    intro j
    rcases herr j with ⟨e, heq, hele⟩
    unfold aggColErrTerm at heq
    by_cases hcv : cv j = exactCoeff j
    · have hbeq : (cv j == exactCoeff j) = true := beq_iff_eq.mpr hcv
      simp only [hbeq, ↓reduceIte, Option.some.injEq] at heq
      subst heq
      rw [hcv]
      simpa [Rat.add_zero] using Rat.add_le_add_left (c := exactCoeff j * x j) |>.mpr hele
    · have hbeq : (cv j == exactCoeff j) = false := beq_false_of_ne hcv
      simp only [hbeq, Bool.false_eq_true, ↓reduceIte] at heq
      cases hlb : lb j with
      | none => simp [hlb] at heq
      | some l =>
        cases hub : ub j with
        | none => simp [hlb, hub] at heq
        | some u =>
          simp only [hlb, hub, Option.some.injEq] at heq
          subst heq
          have hxabs : (x j).abs ≤ max l.abs u.abs := by
            rw [rat_abs_le_iff]
            have hlb_le : -max l.abs u.abs ≤ l := by
              have h := Rat.neg_le_neg (Rat.le_trans (rat_le_abs (-l))
                (by simpa [Rat.abs_neg] using rat_le_max_left (a := l.abs) (b := u.abs)))
              simpa [Rat.neg_neg] using h
            have hub_le : u ≤ max l.abs u.abs :=
              Rat.le_trans (rat_le_abs u) rat_le_max_right
            exact ⟨Rat.le_trans hlb_le ((hxbnd j).1 l hlb), Rat.le_trans ((hxbnd j).2 u hub) hub_le⟩
          have hdiff : (cv j - exactCoeff j) * x j ≤ ((cv j - exactCoeff j) * x j).abs := rat_le_abs _
          rw [rat_abs_mul] at hdiff
          have hmul : (cv j - exactCoeff j).abs * (x j).abs ≤ (cv j - exactCoeff j).abs * max l.abs u.abs :=
            Rat.mul_le_mul_of_nonneg_left hxabs Rat.abs_nonneg
          have hle : (cv j - exactCoeff j) * x j ≤ errTerm j :=
            Rat.le_trans (Rat.le_trans hdiff hmul) hele
          have hcv_eq : cv j * x j = exactCoeff j * x j + (cv j - exactCoeff j) * x j := by
            rw [rat_sub_mul, Rat.add_comm (exactCoeff j * x j), Rat.sub_add_cancel]
          rw [hcv_eq]
          exact Rat.add_le_add_left.mpr hle
  have h1 := finSum_le_finSum hterm
  rw [finSum_add] at h1
  have h2 := rat_add_le_add hexact (Rat.le_refl (a := finSum n errTerm))
  have h3 := Rat.le_trans h1 h2
  have h4 : exactRHS + finSum n errTerm ≤ exactRHS + rhsErr + finSum n errTerm := by
    have hle := Rat.add_le_add_left (c := exactRHS) |>.mpr hrhsErr
    rw [Rat.add_zero] at hle
    exact Rat.add_le_add_right.mpr hle
  exact Rat.le_trans (Rat.le_trans h3 h4) hrhs

/-- Effective lower bound for cMIR (tightening fractional bounds to `ceil(l)` on integer columns). -/
def cmirEffLB (isInt : Bool) (lb : Option Rat) : Option Rat :=
  match lb with
  | none => none
  | some l => some (if isInt then (effIntLB l : Rat) else l)

/-- Effective upper bound for cMIR (tightening fractional bounds to `floor(u)` on integer columns). -/
def cmirEffUB (isInt : Bool) (ub : Option Rat) : Option Rat :=
  match ub with
  | none => none
  | some u => some (if isInt then (effIntUB u : Rat) else u)

/-- Active chosen bound for column `j` in unified cMIR (`0` when `cv j = 0`). -/
def cmirActiveBound (isInt useLower : Bool) (cv : Rat) (lb ub : Option Rat) : Option Rat :=
  if cv == 0 then some 0
  else if useLower then cmirEffLB isInt lb
  else cmirEffUB isInt ub

/-- Unified multi-row aggregated (and pairwise-surrogate) cMIR soundness theorem over
arbitrary `Option Rat` variable domains, combining `Track2B.row_agg_sum_sound`,
`row_agg_error_bound_opt_sound`, and `Track2B.cmir_safe_rounded_sound`. -/
theorem agg_row_cmir_sound {m n : Nat}
    {A : Fin m → Fin n → Rat} {sense : Fin m → ConstraintSense}
    {rowRHS mu : Fin m → Rat}
    {isInt useLower : Fin n → Bool} {lb ub : Fin n → Option Rat} {x : Fin n → Rat}
    {cv errTerm alphaHat cInt cCont : Fin n → Rat}
    {rhsErr aggRHS delta betaHat cutRHS : Rat}
    (hmu : ∀ i, mu i = 0 ∨ rowLegalMuSignSpec (sense i) (mu i) = true)
    (hsat : ∀ i, satisfiesSense (finSum n (fun j => A i j * x j)) (sense i) (rowRHS i))
    (hxbnd : boundsSat lb ub x)
    (hxInt : ∀ j, isInt j = true → isIntVal (x j))
    (herr : ∀ j, ∃ e, aggColErrTerm (cv j) (finSum m (fun i => mu i * A i j)) (lb j) (ub j) = some e ∧ e ≤ errTerm j)
    (hrhsErr : 0 ≤ rhsErr)
    (haggRHS : finSum m (fun i => mu i * rowRHS i) + rhsErr + finSum n errTerm ≤ aggRHS)
    (hdelta : 0 < delta)
    (hactBnd : ∀ j, (cmirActiveBound (isInt j) (useLower j) (cv j) (lb j) (ub j)).isSome = true)
    (hbeta : (aggRHS - finSum n (fun j => cv j * (cmirActiveBound (isInt j) (useLower j) (cv j) (lb j) (ub j)).getD 0)) / delta ≤ betaHat)
    (halpha : ∀ j, isInt j = true → cv j ≠ 0 → alphaHat j ≤ cmirSignedCoeff (useLower j) (cv j) / delta)
    (hzero : ∀ j, cv j = 0 → cInt j = 0 ∧ cCont j = 0)
    (hcInt : ∀ j, isInt j = true → cv j ≠ 0 →
      ((cmirEffLB true (lb j)).isSome = true ∧ (cmirEffUB true (ub j)).isSome = true ∧
        (cmirEffLB true (lb j)).getD 0 = (cmirEffUB true (ub j)).getD 0 ∧ cInt j = 0) ∨
      cInt j ≤ mirIntCoeff (alphaHat j) (fracPart betaHat))
    (hcCont : ∀ j, isInt j = false → cv j ≠ 0 →
      max (-(cmirSignedCoeff (useLower j) (cv j))) 0 / (delta * (1 - fracPart betaHat)) ≤ cCont j)
    (hrhs : (betaHat.floor : Rat) +
      finSum n (fun j =>
        (if isInt j then cmirSignedCoeff (useLower j) (cInt j)
         else if useLower j then -cCont j else cCont j) *
        (cmirActiveBound (isInt j) (useLower j) (cv j) (lb j) (ub j)).getD 0) ≤ cutRHS) :
    finSum n (fun j =>
      (if isInt j then cmirSignedCoeff (useLower j) (cInt j)
       else if useLower j then -cCont j else cCont j) * x j) ≤ cutRHS := by
  have hexactAgg := row_agg_sum_sound hmu hsat
  have haggRow := row_agg_error_bound_opt_sound hexactAgg hxbnd herr hrhsErr haggRHS
  -- Embed unified `Fin n` columns into `Track2B.cmir_safe_rounded_sound` (`mI = n, mC = n`),
  -- routing inactive/cancelled (`cv j = 0`) and opposite-category slots to fixed `0`.
  let actI : Fin n → Bool := fun j => isInt j && decide (cv j ≠ 0)
  let actC : Fin n → Bool := fun j => (!isInt j) && decide (cv j ≠ 0)
  let aI : Fin n → Rat := fun j => if actI j then cv j else 0
  let xI : Fin n → Rat := fun j => if actI j then x j else 0
  let lI : Fin n → Rat := fun j => if actI j then (cmirEffLB true (lb j)).getD 0 else 0
  let uI : Fin n → Rat := fun j => if actI j then (cmirEffUB true (ub j)).getD 0 else 0
  let hasLI : Fin n → Bool := fun j => if actI j then (cmirEffLB true (lb j)).isSome else true
  let hasUI : Fin n → Bool := fun j => if actI j then (cmirEffUB true (ub j)).isSome else true
  let useLI : Fin n → Bool := fun j => if actI j then useLower j else true
  let alphaHI : Fin n → Rat := fun j => if actI j then alphaHat j else 0
  let cI' : Fin n → Rat := fun j => if actI j then cInt j else 0
  let aC : Fin n → Rat := fun j => if actC j then cv j else 0
  let xC : Fin n → Rat := fun j => if actC j then x j else 0
  let lC : Fin n → Rat := fun j => if actC j then (lb j).getD 0 else 0
  let uC : Fin n → Rat := fun j => if actC j then (ub j).getD 0 else 0
  let hasLC : Fin n → Bool := fun j => if actC j then (lb j).isSome else true
  let hasUC : Fin n → Bool := fun j => if actC j then (ub j).isSome else true
  let useLC : Fin n → Bool := fun j => if actC j then useLower j else true
  let cC' : Fin n → Rat := fun j => if actC j then cCont j else 0
  have hxI_ok : ∀ j, (∃ z : Int, xI j = (z : Rat)) ∧
      (hasLI j = true → lI j ≤ xI j) ∧ (hasUI j = true → xI j ≤ uI j) ∧
      (if useLI j then hasLI j = true ∧ ∃ z : Int, lI j = (z : Rat)
       else hasUI j = true ∧ ∃ z : Int, uI j = (z : Rat)) := by
    intro j
    cases ha : actI j
    · simp [xI, lI, uI, hasLI, hasUI, useLI, ha]
      exact ⟨0, rfl⟩
    · have hInt : isInt j = true := by
        dsimp [actI] at ha; simp only [Bool.and_eq_true] at ha; exact ha.1
      have hne0 : cv j ≠ 0 := by
        dsimp [actI] at ha; simp only [Bool.and_eq_true, decide_eq_true_eq] at ha; exact ha.2
      rcases hxInt j hInt with ⟨zx, hzx⟩
      simp only [xI, lI, uI, hasLI, hasUI, useLI, ha, ↓reduceIte]
      refine ⟨⟨zx, hzx⟩, ?_, ?_, ?_⟩
      · intro hL
        cases hlb : lb j with
        | none => simp [cmirEffLB, hlb] at hL
        | some l =>
          simp only [cmirEffLB, ↓reduceIte, Option.getD_some]
          rw [hzx]
          exact effIntLB_le_of_le (by rw [← hzx]; exact (hxbnd j).1 l hlb)
      · intro hU
        cases hub : ub j with
        | none => simp [cmirEffUB, hub] at hU
        | some u =>
          simp only [cmirEffUB, ↓reduceIte, Option.getD_some]
          rw [hzx]
          exact le_effIntUB_of_le (by rw [← hzx]; exact (hxbnd j).2 u hub)
      · have hact := hactBnd j
        unfold cmirActiveBound at hact
        have hbeq : (cv j == 0) = false := beq_false_of_ne hne0
        simp only [hbeq, Bool.false_eq_true, ↓reduceIte, hInt] at hact
        cases hu : useLower j
        · simp only [hu, Bool.false_eq_true, ↓reduceIte] at hact ⊢
          refine ⟨hact, ?_⟩
          cases hub : ub j with
          | none => simp [cmirEffUB, hub] at hact
          | some u => exact ⟨effIntUB u, by simp [cmirEffUB]⟩
        · simp only [hu, ↓reduceIte] at hact ⊢
          refine ⟨hact, ?_⟩
          cases hlb : lb j with
          | none => simp [cmirEffLB, hlb] at hact
          | some l => exact ⟨effIntLB l, by simp [cmirEffLB]⟩
  have hxC_ok : ∀ j, (hasLC j = true → lC j ≤ xC j) ∧ (hasUC j = true → xC j ≤ uC j) ∧
      (if useLC j then hasLC j = true else hasUC j = true) := by
    intro j
    cases ha : actC j
    · simp [xC, lC, uC, hasLC, hasUC, useLC, ha]
    · have hCont : isInt j = false := by
        dsimp [actC] at ha; simp only [Bool.and_eq_true, Bool.not_eq_true'] at ha; exact ha.1
      have hne0 : cv j ≠ 0 := by
        dsimp [actC] at ha; simp only [Bool.and_eq_true, decide_eq_true_eq] at ha; exact ha.2
      simp only [xC, lC, uC, hasLC, hasUC, useLC, ha, ↓reduceIte]
      refine ⟨?_, ?_, ?_⟩
      · intro hL
        cases hlb : lb j with
        | none => simp [hlb] at hL
        | some l => simp only [Option.getD_some]; exact (hxbnd j).1 l hlb
      · intro hU
        cases hub : ub j with
        | none => simp [hub] at hU
        | some u => simp only [Option.getD_some]; exact (hxbnd j).2 u hub
      · have hact := hactBnd j
        unfold cmirActiveBound at hact
        have hbeq : (cv j == 0) = false := beq_false_of_ne hne0
        simp only [hbeq, Bool.false_eq_true, ↓reduceIte, hCont] at hact
        cases hu : useLower j
        · simp only [hu, Bool.false_eq_true, ↓reduceIte] at hact ⊢
          cases hub : ub j with
          | none => simp [cmirEffUB, hub] at hact
          | some _ => rfl
        · simp only [hu, ↓reduceIte] at hact ⊢
          cases hlb : lb j with
          | none => simp [cmirEffLB, hlb] at hact
          | some _ => rfl
  have hrowSplit : finSum n (fun j => aI j * xI j) + finSum n (fun j => aC j * xC j) ≤ aggRHS := by
    rw [← finSum_add]
    have hcong : finSum n (fun j => aI j * xI j + aC j * xC j) = finSum n (fun j => cv j * x j) := by
      apply finSum_congr
      intro j
      by_cases h0 : cv j = 0
      · simp [aI, aC, actI, actC, h0, Rat.zero_mul, Rat.add_zero]
      · cases hi : isInt j <;> simp [aI, aC, xI, xC, actI, actC, hi, h0, Rat.add_zero, Rat.zero_add]
    rw [hcong]
    exact haggRow
  have hshiftEq :
      cmirShiftedRHS aI lI uI useLI aC lC uC useLC aggRHS =
        aggRHS - finSum n (fun j => cv j * (cmirActiveBound (isInt j) (useLower j) (cv j) (lb j) (ub j)).getD 0) := by
    unfold cmirShiftedRHS
    rw [Rat.sub_eq_add_neg, Rat.sub_eq_add_neg, Rat.add_assoc, ← Rat.neg_add, ← finSum_add, ← Rat.sub_eq_add_neg]
    congr 1
    apply finSum_congr
    intro j
    by_cases h0 : cv j = 0
    · simp [aI, aC, actI, actC, h0, Rat.zero_mul, Rat.add_zero]
    · have hbeq : (cv j == 0) = false := beq_false_of_ne h0
      cases hi : isInt j <;> cases hu : useLower j <;>
        simp [aI, aC, lI, uI, lC, uC, useLI, useLC, actI, actC, cmirBound, cmirActiveBound, cmirEffLB, cmirEffUB,
          hi, hu, h0, hbeq, Rat.add_zero, Rat.zero_add]
      · cases hub : ub j <;> simp
      · cases hlb : lb j <;> simp
  have hbeta' : cmirShiftedRHS aI lI uI useLI aC lC uC useLC aggRHS / delta ≤ betaHat := by
    rw [hshiftEq]; exact hbeta
  have halpha' : ∀ j, alphaHI j ≤ cmirSignedCoeff (useLI j) (aI j) / delta := by
    intro j
    cases ha : actI j
    · simp [alphaHI, aI, useLI, cmirSignedCoeff, ha, Rat.div_def, Rat.zero_mul]
    · have hInt : isInt j = true := by
        dsimp [actI] at ha; simp only [Bool.and_eq_true] at ha; exact ha.1
      have hne0 : cv j ≠ 0 := by
        dsimp [actI] at ha; simp only [Bool.and_eq_true, decide_eq_true_eq] at ha; exact ha.2
      simp only [alphaHI, aI, useLI, ha, ↓reduceIte]
      exact halpha j hInt hne0
  have hcI' : ∀ j, (hasLI j = true ∧ hasUI j = true ∧ lI j = uI j ∧ cI' j = 0) ∨
      cI' j ≤ mirIntCoeff (alphaHI j) (fracPart betaHat) := by
    intro j
    cases ha : actI j
    · left; simp [hasLI, hasUI, lI, uI, cI', ha]
    · have hInt : isInt j = true := by
        dsimp [actI] at ha; simp only [Bool.and_eq_true] at ha; exact ha.1
      have hne0 : cv j ≠ 0 := by
        dsimp [actI] at ha; simp only [Bool.and_eq_true, decide_eq_true_eq] at ha; exact ha.2
      simp only [hasLI, hasUI, lI, uI, cI', alphaHI, ha, ↓reduceIte]
      exact hcInt j hInt hne0
  have hcC' : ∀ j, max (-(cmirSignedCoeff (useLC j) (aC j))) 0 / (delta * (1 - fracPart betaHat)) ≤ cC' j := by
    intro j
    cases ha : actC j
    · simp only [aC, useLC, cC', ha, Bool.false_eq_true, ↓reduceIte, cmirSignedCoeff, Rat.neg_zero]
      have hm : max (0 : Rat) 0 = 0 := by simp [Rat.max_def]
      rw [hm, Rat.div_def, Rat.zero_mul]
      exact Rat.le_refl
    · have hCont : isInt j = false := by
        dsimp [actC] at ha; simp only [Bool.and_eq_true, Bool.not_eq_true'] at ha; exact ha.1
      have hne0 : cv j ≠ 0 := by
        dsimp [actC] at ha; simp only [Bool.and_eq_true, decide_eq_true_eq] at ha; exact ha.2
      simp only [aC, useLC, cC', ha, ↓reduceIte]
      exact hcCont j hCont hne0
  have hrhs' : (betaHat.floor : Rat) +
      finSum n (fun j => cmirSignedCoeff (useLI j) (cI' j) * cmirBound (useLI j) (lI j) (uI j)) +
      finSum n (fun j => (if useLC j then -cC' j else cC' j) * cmirBound (useLC j) (lC j) (uC j)) ≤ cutRHS := by
    rw [Rat.add_assoc, ← finSum_add]
    have hcong :
        finSum n (fun j =>
          cmirSignedCoeff (useLI j) (cI' j) * cmirBound (useLI j) (lI j) (uI j) +
          (if useLC j then -cC' j else cC' j) * cmirBound (useLC j) (lC j) (uC j)) =
        finSum n (fun j =>
          (if isInt j then cmirSignedCoeff (useLower j) (cInt j)
           else if useLower j then -cCont j else cCont j) *
          (cmirActiveBound (isInt j) (useLower j) (cv j) (lb j) (ub j)).getD 0) := by
      apply finSum_congr
      intro j
      by_cases h0 : cv j = 0
      · simp [actI, actC, cI', cC', useLI, useLC, lI, uI, lC, uC, cmirSignedCoeff, cmirBound,
          cmirActiveBound, h0, Rat.mul_zero, Rat.add_zero]
      · have hbeq : (cv j == 0) = false := beq_false_of_ne h0
        cases hi : isInt j <;> cases hu : useLower j <;>
          simp [actI, actC, cI', cC', useLI, useLC, lI, uI, lC, uC, cmirSignedCoeff, cmirBound,
            cmirActiveBound, cmirEffLB, cmirEffUB, hi, hu, h0, hbeq, Rat.add_zero, Rat.zero_add]
        · cases hub : ub j <;> simp
        · cases hlb : lb j <;> simp
    rw [hcong]
    exact hrhs
  have hcmir := cmir_safe_rounded_sound hdelta hxI_ok hxC_ok hrowSplit hbeta' halpha' hcI' hcC' hrhs'
  rw [← finSum_add] at hcmir
  have hlhsEq :
      finSum n (fun j => cmirSignedCoeff (useLI j) (cI' j) * xI j + (if useLC j then -cC' j else cC' j) * xC j) =
      finSum n (fun j =>
        (if isInt j then cmirSignedCoeff (useLower j) (cInt j)
         else if useLower j then -cCont j else cCont j) * x j) := by
    apply finSum_congr
    intro j
    by_cases h0 : cv j = 0
    · rcases hzero j h0 with ⟨hcI0, hcC0⟩
      cases hi : isInt j <;> cases hu : useLower j <;>
        simp [actI, actC, cI', cC', xI, xC, cmirSignedCoeff, hi, h0, hcI0, hcC0,
          Rat.zero_mul, Rat.mul_zero, Rat.add_zero]
    · cases hi : isInt j <;> simp [actI, actC, cI', cC', xI, xC, useLI, useLC, cmirSignedCoeff, hi, h0, Rat.add_zero, Rat.zero_add]
  rwa [hlhsEq] at hcmir

/-- Complete certificate for a multi-row aggregated (or pairwise surrogate) cMIR cut
over arbitrary optional bounds, with optional positive final cut scaling. -/
structure AggCMIRCert where
  numVars : Nat
  isInt : Array Bool
  lb : Array (Option Rat)
  ub : Array (Option Rat)
  rows : Array (RowSpec × Rat)
  aggTerms : Array SparseTerm
  aggRHS : Rat
  delta : Rat
  useLower : Array Bool
  betaHat : Option Rat := none
  alphaHat : Option (Array Rat) := none
  cInt : Option (Array Rat) := none
  cCont : Option (Array Rat) := none
  scale : Rat := 1
  cut : CutSpec
  deriving Repr, Inhabited

/-- Executable verifier for `AggCMIRCert`. -/
def checkAggCMIRCert (cert : AggCMIRCert) : Except String Unit :=
  let n := cert.numVars
  let m := cert.rows.size
  let aggDense := accumTerms n cert.aggTerms
  let cutDense := accumTerms n cert.cut.terms
  let rowsOk := allFin m fun i =>
    let (row, mu) := cert.rows[i.val]
    allTermsInBounds n row.terms && (decide (mu == 0) || rowLegalMuSignSpec row.sense mu)
  let exactAggCoeff : Fin n → Rat := fun j =>
    finSum m fun i =>
      let (row, mu) := cert.rows[i.val]
      mu * termsCoeff row.terms.toList j.val
  let exactAggRHS : Rat :=
    finSum m fun i =>
      let (row, mu) := cert.rows[i.val]
      mu * row.rhs
  let cvFn : Fin n → Rat := fun j => aggDense.1[j.val]'(by rw [aggDense.2]; exact j.isLt)
  let errOk := allFin n fun j =>
    (aggColErrTerm (cvFn j) (exactAggCoeff j) cert.lb[j.val]! cert.ub[j.val]!).isSome
  let errSum : Rat := finSum n fun j =>
    (aggColErrTerm (cvFn j) (exactAggCoeff j) cert.lb[j.val]! cert.ub[j.val]!).getD 0
  let actBndFn : Fin n → Option Rat := fun j =>
    cmirActiveBound cert.isInt[j.val]! cert.useLower[j.val]! (cvFn j) cert.lb[j.val]! cert.ub[j.val]!
  let actBndOk := allFin n fun j => (actBndFn j).isSome
  let exactBeta : Rat :=
    (cert.aggRHS - finSum n (fun j => cvFn j * (actBndFn j).getD 0)) / cert.delta
  let betaHat : Rat := cert.betaHat.getD exactBeta
  let f0 : Rat := fracPart betaHat
  let alphaHatFn : Fin n → Rat := fun j =>
    match cert.alphaHat with
    | none => cmirSignedCoeff cert.useLower[j.val]! (cvFn j) / cert.delta
    | some arr => arr[j.val]!
  let alphaLenOk : Bool := match cert.alphaHat with | none => true | some arr => arr.size == n
  let alphaOk := allFin n fun j =>
    if cert.isInt[j.val]! && decide (cvFn j ≠ 0) then
      decide (alphaHatFn j ≤ cmirSignedCoeff cert.useLower[j.val]! (cvFn j) / cert.delta)
    else true
  let cIntFn : Fin n → Rat := fun j =>
    if cvFn j == 0 then 0
    else match cert.cInt with
      | none => mirIntCoeff (alphaHatFn j) f0
      | some arr => arr[j.val]!
  let cIntLenOk : Bool := match cert.cInt with | none => true | some arr => arr.size == n
  let cIntOk := allFin n fun j =>
    if cert.isInt[j.val]! && decide (cvFn j ≠ 0) then
      let lOpt := cmirEffLB true cert.lb[j.val]!
      let uOpt := cmirEffUB true cert.ub[j.val]!
      let isFix := lOpt.isSome && uOpt.isSome && decide (lOpt.getD 0 = uOpt.getD 0) && decide (cIntFn j = 0)
      isFix || decide (cIntFn j ≤ mirIntCoeff (alphaHatFn j) f0)
    else true
  let cContFn : Fin n → Rat := fun j =>
    if cvFn j == 0 then 0
    else match cert.cCont with
      | none => max (-(cmirSignedCoeff cert.useLower[j.val]! (cvFn j))) 0 / (cert.delta * (1 - f0))
      | some arr => arr[j.val]!
  let cContLenOk : Bool := match cert.cCont with | none => true | some arr => arr.size == n
  let cContOk := allFin n fun j =>
    if !cert.isInt[j.val]! && decide (cvFn j ≠ 0) then
      decide (max (-(cmirSignedCoeff cert.useLower[j.val]! (cvFn j))) 0 / (cert.delta * (1 - f0)) ≤ cContFn j)
    else true
  let unscaledCutCoeff : Fin n → Rat := fun j =>
    if cert.isInt[j.val]! then cmirSignedCoeff cert.useLower[j.val]! (cIntFn j)
    else if cert.useLower[j.val]! then -cContFn j else cContFn j
  let cutCoeffOk := allFin n fun j =>
    decide (cutDense.1[j.val]'(by rw [cutDense.2]; exact j.isLt) = cert.scale * unscaledCutCoeff j)
  let exactCutRHS : Rat :=
    (betaHat.floor : Rat) + finSum n (fun j => unscaledCutCoeff j * (actBndFn j).getD 0)
  guardExc (cert.isInt.size == n && cert.lb.size == n && cert.ub.size == n && cert.useLower.size == n)
    s!"agg-cmir cert metadata arrays must have length {n}" <|
  guardExc (m != 0) "agg-cmir cert requires at least one row" <|
  guardExc (allTermsInBounds n cert.aggTerms) "agg-cmir cert aggTerms has out-of-bounds column" <|
  guardExc (allTermsInBounds n cert.cut.terms) "agg-cmir cert cut has out-of-bounds column" <|
  guardExc (decide (0 < cert.delta)) s!"agg-cmir cert delta={cert.delta} must be > 0" <|
  guardExc (decide (0 < cert.scale)) s!"agg-cmir cert scale={cert.scale} must be > 0" <|
  guardExc rowsOk "agg-cmir cert row has out-of-bounds col or illegal multiplier sign" <|
  guardExc errOk "agg-cmir cert inexact aggregated coefficient on unbounded column" <|
  guardExc (decide (exactAggRHS + errSum ≤ cert.aggRHS))
    s!"agg-cmir cert aggRHS={cert.aggRHS} < exactAggRHS + errSum={exactAggRHS + errSum}" <|
  guardExc actBndOk "agg-cmir cert active column missing chosen bound" <|
  guardExc (decide (exactBeta ≤ betaHat)) s!"agg-cmir cert betaHat={betaHat} < exactBeta={exactBeta}" <|
  guardExc (alphaLenOk && cIntLenOk && cContLenOk) "agg-cmir cert optional certificate array length mismatch" <|
  guardExc alphaOk "agg-cmir cert alphaHat exceeds exact scaled integer coefficient" <|
  guardExc cIntOk "agg-cmir cert cInt exceeds exact mirIntCoeff" <|
  guardExc cContOk "agg-cmir cert cCont below exact continuous MIR coefficient" <|
  guardExc cutCoeffOk "agg-cmir cert cut coefficients do not match scaled cMIR terms" <|
  guardExc (decide (cert.scale * exactCutRHS ≤ cert.cut.rhs))
    s!"agg-cmir cert cut.rhs={cert.cut.rhs} < scaled exactCutRHS={cert.scale * exactCutRHS}" <|
  .ok ()

/-- Universal soundness of `checkAggCMIRCert` over all feasible `x : Fin cert.numVars → Rat`
satisfying optional bounds, integrality, and all aggregated source rows. -/
theorem checkAggCMIRCert_sound (cert : AggCMIRCert)
    (hok : checkAggCMIRCert cert = .ok ())
    (hIntLen : cert.isInt.size = cert.numVars)
    (hlbLen : cert.lb.size = cert.numVars)
    (hubLen : cert.ub.size = cert.numVars)
    (huLen : cert.useLower.size = cert.numVars)
    (x : Fin cert.numVars → Rat)
    (hxbnd : boundsSat (fun j => cert.lb[j.val]'(by omega)) (fun j => cert.ub[j.val]'(by omega)) x)
    (hxInt : ∀ j : Fin cert.numVars, cert.isInt[j.val]'(by omega) = true → isIntVal (x j))
    (hrowsSat : ∀ i : Fin cert.rows.size, rowSatOnFin cert.numVars cert.rows[i.val].1 x) :
    evalCutOnFin cert.numVars cert.cut x ≤ cert.cut.rhs := by
  unfold checkAggCMIRCert at hok
  rcases guardExc_ok hok with ⟨_, hok⟩
  rcases guardExc_ok hok with ⟨_, hok⟩
  rcases guardExc_ok hok with ⟨_, hok⟩
  rcases guardExc_ok hok with ⟨hcutColsT, hok⟩
  rcases guardExc_ok hok with ⟨hdeltaD, hok⟩
  rcases guardExc_ok hok with ⟨hscaleD, hok⟩
  rcases guardExc_ok hok with ⟨hrowsOkT, hok⟩
  rcases guardExc_ok hok with ⟨herrOkT, hok⟩
  rcases guardExc_ok hok with ⟨haggRHSD, hok⟩
  rcases guardExc_ok hok with ⟨hactBndT, hok⟩
  rcases guardExc_ok hok with ⟨hbetaD, hok⟩
  rcases guardExc_ok hok with ⟨_, hok⟩
  rcases guardExc_ok hok with ⟨halphaT, hok⟩
  rcases guardExc_ok hok with ⟨hcIntT, hok⟩
  rcases guardExc_ok hok with ⟨hcContT, hok⟩
  rcases guardExc_ok hok with ⟨hcutCoeffT, hok⟩
  rcases guardExc_ok hok with ⟨hcutRHSD, _⟩
  let n := cert.numVars
  let m := cert.rows.size
  let isIntFn : Fin n → Bool := fun j => cert.isInt[j.val]'(by omega)
  let lbFn : Fin n → Option Rat := fun j => cert.lb[j.val]'(by omega)
  let ubFn : Fin n → Option Rat := fun j => cert.ub[j.val]'(by omega)
  let useLFn : Fin n → Bool := fun j => cert.useLower[j.val]'(by omega)
  have hisIntB : ∀ j : Fin n, cert.isInt[j.val]! = isIntFn j := fun j => getElem!_pos cert.isInt j.val (by omega)
  have hlbB : ∀ j : Fin n, cert.lb[j.val]! = lbFn j := fun j => getElem!_pos cert.lb j.val (by omega)
  have hubB : ∀ j : Fin n, cert.ub[j.val]! = ubFn j := fun j => getElem!_pos cert.ub j.val (by omega)
  have huLB : ∀ j : Fin n, cert.useLower[j.val]! = useLFn j := fun j => getElem!_pos cert.useLower j.val (by omega)
  have hdeltaPos : 0 < cert.delta := of_decide_eq_true hdeltaD
  have hscalePos : 0 < cert.scale := of_decide_eq_true hscaleD
  have hrowsAll := allFin_eq_true_iff.mp hrowsOkT
  have herrAll := allFin_eq_true_iff.mp herrOkT
  have hactAll := allFin_eq_true_iff.mp hactBndT
  have halphaAll := allFin_eq_true_iff.mp halphaT
  have hcIntAll := allFin_eq_true_iff.mp hcIntT
  have hcContAll := allFin_eq_true_iff.mp hcContT
  have hcutCoeffAll := allFin_eq_true_iff.mp hcutCoeffT
  let AFn : Fin m → Fin n → Rat := fun i j => termsCoeff cert.rows[i.val].1.terms.toList j.val
  let senseFn : Fin m → ConstraintSense := fun i => cert.rows[i.val].1.sense
  let rowRHSFn : Fin m → Rat := fun i => cert.rows[i.val].1.rhs
  let muFn : Fin m → Rat := fun i => cert.rows[i.val].2
  let aggDense := accumTerms n cert.aggTerms
  let cvFn : Fin n → Rat := fun j => aggDense.1[j.val]'(by rw [aggDense.2]; exact j.isLt)
  let errTermFn : Fin n → Rat := fun j =>
    (aggColErrTerm (cvFn j) (finSum m fun i => muFn i * AFn i j) (lbFn j) (ubFn j)).getD 0
  let betaHat : Rat := cert.betaHat.getD
    ((cert.aggRHS - finSum n (fun j => cvFn j * (cmirActiveBound (isIntFn j) (useLFn j) (cvFn j) (lbFn j) (ubFn j)).getD 0)) / cert.delta)
  let alphaHatFn : Fin n → Rat := fun j =>
    match cert.alphaHat with
    | none => cmirSignedCoeff (useLFn j) (cvFn j) / cert.delta
    | some arr => arr[j.val]!
  let cIntFn : Fin n → Rat := fun j =>
    if cvFn j == 0 then 0
    else match cert.cInt with
      | none => mirIntCoeff (alphaHatFn j) (fracPart betaHat)
      | some arr => arr[j.val]!
  let cContFn : Fin n → Rat := fun j =>
    if cvFn j == 0 then 0
    else match cert.cCont with
      | none => max (-(cmirSignedCoeff (useLFn j) (cvFn j))) 0 / (cert.delta * (1 - fracPart betaHat))
      | some arr => arr[j.val]!
  have hmu : ∀ i : Fin m, muFn i = 0 ∨ rowLegalMuSignSpec (senseFn i) (muFn i) = true := by
    intro i
    have hi := hrowsAll i
    dsimp only at hi
    simp only [Bool.and_eq_true, Bool.or_eq_true, decide_eq_true_eq] at hi
    rcases hi.2 with h0 | hleg
    · exact Or.inl (by simpa [muFn] using h0)
    · exact Or.inr hleg
  have hsat : ∀ i : Fin m, satisfiesSense (finSum n (fun j => AFn i j * x j)) (senseFn i) (rowRHSFn i) := by
    intro i
    have hi := hrowsAll i
    dsimp only at hi
    simp only [Bool.and_eq_true] at hi
    have hcols := allTermsInBounds_iff.mp hi.1
    have hrow := hrowsSat i
    unfold rowSatOnFin evalRowOnFin at hrow
    rwa [evalTermsList_eq_finSum cert.rows[i.val].1.terms.toList x hcols] at hrow
  have herr : ∀ j : Fin n, ∃ e,
      aggColErrTerm (cvFn j) (finSum m fun i => muFn i * AFn i j) (lbFn j) (ubFn j) = some e ∧ e ≤ errTermFn j := by
    intro j
    have hj := herrAll j
    dsimp only at hj
    rw [hlbB j, hubB j] at hj
    cases he : aggColErrTerm (cvFn j) (finSum m fun i => muFn i * AFn i j) (lbFn j) (ubFn j) with
    | none => rw [he] at hj; simp at hj
    | some e => exact ⟨e, rfl, by simp [errTermFn, he]⟩
  have haggRHSLe : finSum m (fun i => muFn i * rowRHSFn i) + 0 + finSum n errTermFn ≤ cert.aggRHS := by
    have h := of_decide_eq_true haggRHSD
    have hcong : finSum n (fun j => (aggColErrTerm (cvFn j) (finSum m fun i => muFn i * AFn i j) cert.lb[j.val]! cert.ub[j.val]!).getD 0) =
        finSum n errTermFn :=
      finSum_congr fun j => by dsimp only [errTermFn]; rw [hlbB j, hubB j]
    rw [Rat.add_zero]
    rwa [← hcong]
  have hactBnd : ∀ j : Fin n, (cmirActiveBound (isIntFn j) (useLFn j) (cvFn j) (lbFn j) (ubFn j)).isSome = true := by
    intro j
    have hj := hactAll j
    dsimp only at hj
    rwa [hisIntB j, huLB j, hlbB j, hubB j] at hj
  have hactBndEq : ∀ j : Fin n,
      cmirActiveBound cert.isInt[j.val]! cert.useLower[j.val]! (cvFn j) cert.lb[j.val]! cert.ub[j.val]! =
      cmirActiveBound (isIntFn j) (useLFn j) (cvFn j) (lbFn j) (ubFn j) := fun j => by
    rw [hisIntB j, huLB j, hlbB j, hubB j]
  have hbetaEq :
      (cert.aggRHS - finSum n (fun j => cvFn j * (cmirActiveBound cert.isInt[j.val]! cert.useLower[j.val]! (cvFn j) cert.lb[j.val]! cert.ub[j.val]!).getD 0)) / cert.delta =
      (cert.aggRHS - finSum n (fun j => cvFn j * (cmirActiveBound (isIntFn j) (useLFn j) (cvFn j) (lbFn j) (ubFn j)).getD 0)) / cert.delta := by
    congr 2
    exact finSum_congr fun j => by rw [hactBndEq j]
  have hbetaLe :
      (cert.aggRHS - finSum n (fun j => cvFn j * (cmirActiveBound (isIntFn j) (useLFn j) (cvFn j) (lbFn j) (ubFn j)).getD 0)) / cert.delta ≤ betaHat := by
    have h := of_decide_eq_true hbetaD
    dsimp only [betaHat]
    rwa [hbetaEq] at h
  have halphaEq : ∀ j : Fin n,
      (match cert.alphaHat with | none => cmirSignedCoeff cert.useLower[j.val]! (cvFn j) / cert.delta | some arr => arr[j.val]!) =
      alphaHatFn j := by
    intro j; dsimp only [alphaHatFn]; cases cert.alphaHat <;> simp [huLB j]
  have halphaProp : ∀ j : Fin n, isIntFn j = true → cvFn j ≠ 0 →
      alphaHatFn j ≤ cmirSignedCoeff (useLFn j) (cvFn j) / cert.delta := by
    intro j hInt hne0
    have hj := halphaAll j
    dsimp only at hj
    have hd : decide ((accumTerms cert.numVars cert.aggTerms).1[j.val]'(by rw [(accumTerms cert.numVars cert.aggTerms).2]; exact j.isLt) ≠ 0) = true :=
      decide_eq_true hne0
    simp only [hisIntB, huLB, hInt, hd, Bool.and_self, ↓reduceIte, decide_eq_true_eq] at hj
    exact hj
  have hzeroProp : ∀ j : Fin n, cvFn j = 0 → cIntFn j = 0 ∧ cContFn j = 0 := by
    intro j h0
    simp [cIntFn, cContFn, h0]
  have hbetaRaw :
      cert.betaHat.getD ((cert.aggRHS - finSum cert.numVars (fun k =>
        (accumTerms cert.numVars cert.aggTerms).1[k.val]'(by rw [(accumTerms cert.numVars cert.aggTerms).2]; exact k.isLt) *
        (cmirActiveBound cert.isInt[k.val]! cert.useLower[k.val]!
          ((accumTerms cert.numVars cert.aggTerms).1[k.val]'(by rw [(accumTerms cert.numVars cert.aggTerms).2]; exact k.isLt))
          cert.lb[k.val]! cert.ub[k.val]!).getD 0)) / cert.delta) =
      betaHat :=
    congrArg cert.betaHat.getD hbetaEq
  have hcIntEq : ∀ j : Fin n,
      (if (accumTerms cert.numVars cert.aggTerms).1[j.val]'(by rw [(accumTerms cert.numVars cert.aggTerms).2]; exact j.isLt) == 0 then 0 else match cert.cInt with
        | none => mirIntCoeff (match cert.alphaHat with | none => cmirSignedCoeff cert.useLower[j.val]! ((accumTerms cert.numVars cert.aggTerms).1[j.val]'(by rw [(accumTerms cert.numVars cert.aggTerms).2]; exact j.isLt)) / cert.delta | some arr => arr[j.val]!)
            (fracPart (cert.betaHat.getD ((cert.aggRHS - finSum cert.numVars (fun k =>
              (accumTerms cert.numVars cert.aggTerms).1[k.val]'(by rw [(accumTerms cert.numVars cert.aggTerms).2]; exact k.isLt) *
              (cmirActiveBound cert.isInt[k.val]! cert.useLower[k.val]!
                ((accumTerms cert.numVars cert.aggTerms).1[k.val]'(by rw [(accumTerms cert.numVars cert.aggTerms).2]; exact k.isLt))
                cert.lb[k.val]! cert.ub[k.val]!).getD 0)) / cert.delta)))
        | some arr => arr[j.val]!) = cIntFn j := by
    intro j
    rw [hbetaRaw]
    dsimp only [cIntFn, alphaHatFn, cvFn, aggDense, n]
    cases cert.cInt <;> cases cert.alphaHat <;> simp [huLB j]
  have hcContEq : ∀ j : Fin n,
      (if (accumTerms cert.numVars cert.aggTerms).1[j.val]'(by rw [(accumTerms cert.numVars cert.aggTerms).2]; exact j.isLt) == 0 then 0 else match cert.cCont with
        | none => max (-(cmirSignedCoeff cert.useLower[j.val]! ((accumTerms cert.numVars cert.aggTerms).1[j.val]'(by rw [(accumTerms cert.numVars cert.aggTerms).2]; exact j.isLt)))) 0 /
            (cert.delta * (1 - fracPart (cert.betaHat.getD ((cert.aggRHS - finSum cert.numVars (fun k =>
              (accumTerms cert.numVars cert.aggTerms).1[k.val]'(by rw [(accumTerms cert.numVars cert.aggTerms).2]; exact k.isLt) *
              (cmirActiveBound cert.isInt[k.val]! cert.useLower[k.val]!
                ((accumTerms cert.numVars cert.aggTerms).1[k.val]'(by rw [(accumTerms cert.numVars cert.aggTerms).2]; exact k.isLt))
                cert.lb[k.val]! cert.ub[k.val]!).getD 0)) / cert.delta))))
        | some arr => arr[j.val]!) = cContFn j := by
    intro j
    rw [hbetaRaw]
    dsimp only [cContFn, cvFn, aggDense, n]
    cases cert.cCont <;> simp [huLB j]
  have hcIntProp : ∀ j : Fin n, isIntFn j = true → cvFn j ≠ 0 →
      ((cmirEffLB true (lbFn j)).isSome = true ∧ (cmirEffUB true (ubFn j)).isSome = true ∧
        (cmirEffLB true (lbFn j)).getD 0 = (cmirEffUB true (ubFn j)).getD 0 ∧ cIntFn j = 0) ∨
      cIntFn j ≤ mirIntCoeff (alphaHatFn j) (fracPart betaHat) := by
    intro j hInt hne0
    have hj := hcIntAll j
    dsimp only at hj
    have hd : decide ((accumTerms cert.numVars cert.aggTerms).1[j.val]'(by rw [(accumTerms cert.numVars cert.aggTerms).2]; exact j.isLt) ≠ 0) = true :=
      decide_eq_true hne0
    simp only [hisIntB, huLB, hlbB, hubB, hInt, hd, Bool.and_self, ↓reduceIte,
      Bool.or_eq_true, Bool.and_eq_true, decide_eq_true_eq] at hj
    rcases hj with ⟨⟨⟨hL, hU⟩, hlu⟩, hc0⟩ | hle
    · exact Or.inl ⟨hL, hU, hlu, hc0⟩
    · exact Or.inr hle
  have hcContProp : ∀ j : Fin n, isIntFn j = false → cvFn j ≠ 0 →
      max (-(cmirSignedCoeff (useLFn j) (cvFn j))) 0 / (cert.delta * (1 - fracPart betaHat)) ≤ cContFn j := by
    intro j hCont hne0
    have hj := hcContAll j
    dsimp only at hj
    have hd : decide ((accumTerms cert.numVars cert.aggTerms).1[j.val]'(by rw [(accumTerms cert.numVars cert.aggTerms).2]; exact j.isLt) ≠ 0) = true :=
      decide_eq_true hne0
    simp only [hisIntB, huLB, hlbB, hubB, hCont, Bool.not_false, hd, Bool.and_self, ↓reduceIte,
      decide_eq_true_eq] at hj
    exact hj
  let unscaledCut : Fin n → Rat := fun j =>
    if isIntFn j then cmirSignedCoeff (useLFn j) (cIntFn j)
    else if useLFn j then -cContFn j else cContFn j
  let exactCutRHS : Rat := (betaHat.floor : Rat) +
    finSum n (fun j => unscaledCut j * (cmirActiveBound (isIntFn j) (useLFn j) (cvFn j) (lbFn j) (ubFn j)).getD 0)
  have hunscaled := agg_row_cmir_sound
    (A := AFn) (sense := senseFn) (rowRHS := rowRHSFn) (mu := muFn)
    (isInt := isIntFn) (useLower := useLFn) (lb := lbFn) (ub := ubFn) (x := x)
    (cv := cvFn) (errTerm := errTermFn) (alphaHat := alphaHatFn) (cInt := cIntFn) (cCont := cContFn)
    (rhsErr := 0) (aggRHS := cert.aggRHS) (delta := cert.delta) (betaHat := betaHat) (cutRHS := exactCutRHS)
    hmu hsat hxbnd hxInt herr Rat.le_refl haggRHSLe hdeltaPos hactBnd hbetaLe halphaProp
    hzeroProp hcIntProp hcContProp Rat.le_refl
  have hscaled := Rat.mul_le_mul_of_nonneg_left hunscaled (Rat.le_of_lt hscalePos)
  rw [← finSum_const_mul] at hscaled
  rw [evalCutOnFin_eq_finSum_accum cert.cut x hcutColsT]
  have hmatch :
      finSum n (fun j => (accumTerms n cert.cut.terms).1[j.val]'(by rw [(accumTerms n cert.cut.terms).2]; exact j.isLt) * x j) =
      finSum n (fun j => cert.scale * (unscaledCut j * x j)) := by
    apply finSum_congr
    intro j
    have hj := of_decide_eq_true (hcutCoeffAll j)
    dsimp only at hj
    rw [hcIntEq j, hcContEq j] at hj
    simp only [hisIntB, huLB] at hj
    change (accumTerms n cert.cut.terms).1[j.val]'(by rw [(accumTerms n cert.cut.terms).2]; exact j.isLt) =
      cert.scale * unscaledCut j at hj
    rw [hj, Rat.mul_assoc]
  have hrhsLe : cert.scale * exactCutRHS ≤ cert.cut.rhs := by
    have h := of_decide_eq_true hcutRHSD
    dsimp only at h
    have hsumEq :
        finSum cert.numVars (fun j =>
          (if cert.isInt[j.val]! then
             cmirSignedCoeff cert.useLower[j.val]! (if (accumTerms cert.numVars cert.aggTerms).1[j.val]'(by rw [(accumTerms cert.numVars cert.aggTerms).2]; exact j.isLt) == 0 then 0 else match cert.cInt with
               | none => mirIntCoeff (match cert.alphaHat with | none => cmirSignedCoeff cert.useLower[j.val]! ((accumTerms cert.numVars cert.aggTerms).1[j.val]'(by rw [(accumTerms cert.numVars cert.aggTerms).2]; exact j.isLt)) / cert.delta | some arr => arr[j.val]!)
                   (fracPart (cert.betaHat.getD ((cert.aggRHS - finSum cert.numVars (fun k =>
                     (accumTerms cert.numVars cert.aggTerms).1[k.val]'(by rw [(accumTerms cert.numVars cert.aggTerms).2]; exact k.isLt) *
                     (cmirActiveBound cert.isInt[k.val]! cert.useLower[k.val]!
                       ((accumTerms cert.numVars cert.aggTerms).1[k.val]'(by rw [(accumTerms cert.numVars cert.aggTerms).2]; exact k.isLt))
                       cert.lb[k.val]! cert.ub[k.val]!).getD 0)) / cert.delta)))
               | some arr => arr[j.val]!)
           else if cert.useLower[j.val]! then
             -(if (accumTerms cert.numVars cert.aggTerms).1[j.val]'(by rw [(accumTerms cert.numVars cert.aggTerms).2]; exact j.isLt) == 0 then 0 else match cert.cCont with
               | none => max (-(cmirSignedCoeff cert.useLower[j.val]! ((accumTerms cert.numVars cert.aggTerms).1[j.val]'(by rw [(accumTerms cert.numVars cert.aggTerms).2]; exact j.isLt)))) 0 /
                   (cert.delta * (1 - fracPart (cert.betaHat.getD ((cert.aggRHS - finSum cert.numVars (fun k =>
                     (accumTerms cert.numVars cert.aggTerms).1[k.val]'(by rw [(accumTerms cert.numVars cert.aggTerms).2]; exact k.isLt) *
                     (cmirActiveBound cert.isInt[k.val]! cert.useLower[k.val]!
                       ((accumTerms cert.numVars cert.aggTerms).1[k.val]'(by rw [(accumTerms cert.numVars cert.aggTerms).2]; exact k.isLt))
                       cert.lb[k.val]! cert.ub[k.val]!).getD 0)) / cert.delta))))
               | some arr => arr[j.val]!)
           else
             (if (accumTerms cert.numVars cert.aggTerms).1[j.val]'(by rw [(accumTerms cert.numVars cert.aggTerms).2]; exact j.isLt) == 0 then 0 else match cert.cCont with
               | none => max (-(cmirSignedCoeff cert.useLower[j.val]! ((accumTerms cert.numVars cert.aggTerms).1[j.val]'(by rw [(accumTerms cert.numVars cert.aggTerms).2]; exact j.isLt)))) 0 /
                   (cert.delta * (1 - fracPart (cert.betaHat.getD ((cert.aggRHS - finSum cert.numVars (fun k =>
                     (accumTerms cert.numVars cert.aggTerms).1[k.val]'(by rw [(accumTerms cert.numVars cert.aggTerms).2]; exact k.isLt) *
                     (cmirActiveBound cert.isInt[k.val]! cert.useLower[k.val]!
                       ((accumTerms cert.numVars cert.aggTerms).1[k.val]'(by rw [(accumTerms cert.numVars cert.aggTerms).2]; exact k.isLt))
                       cert.lb[k.val]! cert.ub[k.val]!).getD 0)) / cert.delta))))
               | some arr => arr[j.val]!)) *
          (cmirActiveBound cert.isInt[j.val]! cert.useLower[j.val]!
            ((accumTerms cert.numVars cert.aggTerms).1[j.val]'(by rw [(accumTerms cert.numVars cert.aggTerms).2]; exact j.isLt))
            cert.lb[j.val]! cert.ub[j.val]!).getD 0) =
        finSum n (fun j => unscaledCut j * (cmirActiveBound (isIntFn j) (useLFn j) (cvFn j) (lbFn j) (ubFn j)).getD 0) := by
      apply finSum_congr
      intro j
      rw [hcIntEq j, hcContEq j]
      simp only [hisIntB, huLB, hlbB, hubB, unscaledCut, cvFn, aggDense, n]
    rw [hsumEq, hbetaRaw] at h
    exact h
  rw [hmatch]
  exact Rat.le_trans hscaled hrhsLe

/-! ### 4. Universal `finalizeSparseCut` Coefficient-Drop & Scaling Soundness -/

/-- Exact minimum value of `d * x` over optional bounds `[lb, ub]`:
`some 0` when `d = 0` (even if unbounded!), `lb.map (d * ·)` when `d > 0`
(allowing semi-bounded `ub = none`), and `ub.map (d * ·)` when `d < 0`
(allowing semi-bounded `lb = none`). -/
def dropMinTerm (d : Rat) (lb ub : Option Rat) : Option Rat :=
  if d == 0 then some 0
  else if 0 < d then lb.map (fun l => d * l)
  else ub.map (fun u => d * u)

theorem dropMinTerm_le {d x lo : Rat} {lb ub : Option Rat}
    (hlo : dropMinTerm d lb ub = some lo)
    (hL : ∀ l, lb = some l → l ≤ x)
    (hU : ∀ u, ub = some u → x ≤ u) :
    lo ≤ d * x := by
  unfold dropMinTerm at hlo
  by_cases hd0 : d = 0
  · have hbeq : (d == 0) = true := beq_iff_eq.mpr hd0
    simp only [hbeq, ↓reduceIte, Option.some.injEq] at hlo
    subst hlo
    simp [hd0]
  · have hbeq : (d == 0) = false := beq_false_of_ne hd0
    simp only [hbeq, Bool.false_eq_true, ↓reduceIte] at hlo
    by_cases hpos : 0 < d
    · simp only [hpos, ↓reduceIte] at hlo
      cases hlb : lb with
      | none => simp [hlb] at hlo
      | some l =>
        simp only [hlb, Option.map_some, Option.some.injEq] at hlo
        subst hlo
        exact Rat.mul_le_mul_of_nonneg_left (hL l hlb) (Rat.le_of_lt hpos)
    · simp only [hpos, ↓reduceIte] at hlo
      have hneg : d ≤ 0 := Rat.not_lt.mp hpos
      cases hub : ub with
      | none => simp [hub] at hlo
      | some u =>
        simp only [hub, Option.map_some, Option.some.injEq] at hlo
        subst hlo
        exact rat_mul_le_mul_of_nonpos_left (hU u hub) hneg

/-- Universal soundness of `finalizeSparseCut` (`cuts.go:2151`): dropping tiny coefficients,
coalescing duplicate columns, scaling by `s > 0` (including subnormal scaling residuals),
and adjusting the RHS outward via exact variable-bound extrema preserves validity over ALL
feasible points in `boundsSat lb ub`. -/
theorem finalize_cut_drop_universal_sound {n : Nat}
    {v outCoeff lo : Fin n → Rat} {lb ub : Fin n → Option Rat} {x : Fin n → Rat}
    {rawRHS s finalRHS : Rat}
    (hs : 0 < s)
    (hcut : finSum n (fun j => v j * x j) ≤ rawRHS)
    (hxbnd : boundsSat lb ub x)
    (hlo : ∀ j, dropMinTerm (s * v j - outCoeff j) (lb j) (ub j) = some (lo j))
    (hrhs : s * rawRHS - finSum n lo ≤ finalRHS) :
    finSum n (fun j => outCoeff j * x j) ≤ finalRHS := by
  have hterm : ∀ j : Fin n, outCoeff j * x j ≤ (s * v j) * x j - lo j := by
    intro j
    have hle := dropMinTerm_le (hlo j) (hxbnd j).1 (hxbnd j).2
    rw [rat_sub_mul] at hle
    have h2 : outCoeff j * x j + lo j ≤ (s * v j) * x j := by
      have h3 := (Rat.le_sub_iff).mp hle
      rwa [Rat.add_comm] at h3
    exact (Rat.le_sub_iff).mpr h2
  have hsum := finSum_le_finSum hterm
  rw [finSum_sub] at hsum
  have hscaled : finSum n (fun j => (s * v j) * x j) ≤ s * rawRHS := by
    have h1 := Rat.mul_le_mul_of_nonneg_left hcut (Rat.le_of_lt hs)
    rw [← finSum_const_mul] at h1
    have hcong : finSum n (fun j => s * (v j * x j)) = finSum n (fun j => (s * v j) * x j) :=
      finSum_congr fun j => (Rat.mul_assoc s (v j) (x j)).symm
    rwa [← hcong]
  have hsub := rat_sub_le_sub hscaled (Rat.le_refl (a := finSum n lo))
  exact Rat.le_trans (Rat.le_trans hsum hsub) hrhs

/-- Complete certificate for a `finalizeSparseCut` / `finalizeCut` transformation. -/
structure FinalizedCutCert where
  numVars : Nat
  lb : Array (Option Rat)
  ub : Array (Option Rat)
  rawCut : CutSpec
  scale : Rat := 1
  finalCut : CutSpec
  deriving Repr, Inhabited

/-- Executable universal verifier for `FinalizedCutCert`. -/
def checkFinalizedCutCert (cert : FinalizedCutCert) : Except String Unit :=
  let n := cert.numVars
  let rawDense := accumTerms n cert.rawCut.terms
  let finalDense := accumTerms n cert.finalCut.terms
  let dropTerms : Fin n → Option Rat := fun j =>
    let vj := rawDense.1[j.val]'(by rw [rawDense.2]; exact j.isLt)
    let oj := finalDense.1[j.val]'(by rw [finalDense.2]; exact j.isLt)
    dropMinTerm (cert.scale * vj - oj) cert.lb[j.val]! cert.ub[j.val]!
  let dropOk := allFin n fun j => (dropTerms j).isSome
  let loSum : Rat := finSum n fun j => (dropTerms j).getD 0
  let reqRHS := cert.scale * cert.rawCut.rhs - loSum
  guardExc (cert.lb.size == n && cert.ub.size == n) s!"drop cert bound arrays must have length {n}" <|
  guardExc (decide (0 < cert.scale)) s!"drop cert scale={cert.scale} must be > 0" <|
  guardExc (allTermsInBounds n cert.rawCut.terms) "drop cert rawCut has out-of-bounds column" <|
  guardExc (allTermsInBounds n cert.finalCut.terms) "drop cert finalCut has out-of-bounds column" <|
  guardExc dropOk "drop cert dropped or perturbed coefficient on unbounded variable side" <|
  guardExc (decide (reqRHS ≤ cert.finalCut.rhs))
    s!"drop cert finalCut.rhs={cert.finalCut.rhs} < required outward RHS {reqRHS}" <|
  .ok ()

/-- Universal soundness of `checkFinalizedCutCert`: valid over ALL `x : Fin cert.numVars → Rat`
satisfying variable bounds, replacing witness-only cut-drop checks. -/
theorem checkFinalizedCutCert_sound (cert : FinalizedCutCert)
    (hok : checkFinalizedCutCert cert = .ok ())
    (hlbLen : cert.lb.size = cert.numVars)
    (hubLen : cert.ub.size = cert.numVars)
    (x : Fin cert.numVars → Rat)
    (hxbnd : boundsSat (fun j => cert.lb[j.val]'(by omega)) (fun j => cert.ub[j.val]'(by omega)) x)
    (hraw : evalCutOnFin cert.numVars cert.rawCut x ≤ cert.rawCut.rhs) :
    evalCutOnFin cert.numVars cert.finalCut x ≤ cert.finalCut.rhs := by
  unfold checkFinalizedCutCert at hok
  rcases guardExc_ok hok with ⟨_, hok⟩
  rcases guardExc_ok hok with ⟨hscaleD, hok⟩
  rcases guardExc_ok hok with ⟨hrawColsT, hok⟩
  rcases guardExc_ok hok with ⟨hfinColsT, hok⟩
  rcases guardExc_ok hok with ⟨hdropOkT, hok⟩
  rcases guardExc_ok hok with ⟨hrhsD, _⟩
  let n := cert.numVars
  let lbFn : Fin n → Option Rat := fun j => cert.lb[j.val]'(by omega)
  let ubFn : Fin n → Option Rat := fun j => cert.ub[j.val]'(by omega)
  have hlbB : ∀ j : Fin n, cert.lb[j.val]! = lbFn j := fun j => getElem!_pos cert.lb j.val (by omega)
  have hubB : ∀ j : Fin n, cert.ub[j.val]! = ubFn j := fun j => getElem!_pos cert.ub j.val (by omega)
  have hscalePos : 0 < cert.scale := of_decide_eq_true hscaleD
  have hdropAll := allFin_eq_true_iff.mp hdropOkT
  let rawDense := accumTerms n cert.rawCut.terms
  let finDense := accumTerms n cert.finalCut.terms
  let vFn : Fin n → Rat := fun j => rawDense.1[j.val]'(by rw [rawDense.2]; exact j.isLt)
  let outFn : Fin n → Rat := fun j => finDense.1[j.val]'(by rw [finDense.2]; exact j.isLt)
  let loFn : Fin n → Rat := fun j =>
    (dropMinTerm (cert.scale * vFn j - outFn j) (lbFn j) (ubFn j)).getD 0
  have hlo : ∀ j : Fin n, dropMinTerm (cert.scale * vFn j - outFn j) (lbFn j) (ubFn j) = some (loFn j) := by
    intro j
    have hj := hdropAll j
    dsimp only at hj
    rw [hlbB j, hubB j] at hj
    dsimp only [loFn]
    cases hd : dropMinTerm (cert.scale * vFn j - outFn j) (lbFn j) (ubFn j) with
    | none => rw [hd] at hj; simp at hj
    | some lo => simp
  have hrhsLe : cert.scale * cert.rawCut.rhs - finSum n loFn ≤ cert.finalCut.rhs := by
    have h := of_decide_eq_true hrhsD
    have hcong :
        finSum n (fun j => (dropMinTerm (cert.scale * vFn j - outFn j) cert.lb[j.val]! cert.ub[j.val]!).getD 0) =
        finSum n loFn :=
      finSum_congr fun j => by dsimp only [loFn]; rw [hlbB j, hubB j]
    rwa [← hcong]
  rw [evalCutOnFin_eq_finSum_accum cert.rawCut x hrawColsT] at hraw
  rw [evalCutOnFin_eq_finSum_accum cert.finalCut x hfinColsT]
  exact finalize_cut_drop_universal_sound hscalePos hraw hxbnd hlo hrhsLe

/-! ### 5. JSON Decoders & Executable Entrypoints for Wave 2 `Apps/CutsCheck.lean` -/

private def parseSenseJson : JsonVal → Except String ConstraintSense
  | .str "le" | .str "<=" => .ok .le
  | .str "eq" | .str "=" | .str "==" => .ok .eq
  | .str "ge" | .str ">=" => .ok .ge
  | .num (-1) 0 false => .ok .le
  | .num 0 0 false => .ok .eq
  | .num 1 0 false => .ok .ge
  | _ => .error "expected constraint sense ('le'|'eq'|'ge' or -1|0|1)"

private def parseOptRatJson : JsonVal → Except String (Option Rat)
  | .null => .ok none
  | v => some <$> v.asRat

private def parseRatArrayJson (arr : Array JsonVal) : Except String (Array Rat) :=
  arr.mapM JsonVal.asRat

private def parseOptRatArrayJson (arr : Array JsonVal) : Except String (Array (Option Rat)) :=
  arr.mapM parseOptRatJson

private def parseBoolArrayJson (arr : Array JsonVal) : Except String (Array Bool) :=
  arr.mapM JsonVal.asBool

private def parseSparseTermJson (v : JsonVal) : Except String SparseTerm := do
  let col ← v.getNat "col"
  let val ← v.getRat "val"
  pure { col, val }

private def parseCutSpecJson (v : JsonVal) : Except String CutSpec := do
  let terms ← (← v.getArray "terms").mapM parseSparseTermJson
  let rhs ← v.getRat "rhs"
  pure { terms, rhs }

private def parseRowSpecJson (v : JsonVal) : Except String RowSpec := do
  let terms ← (← v.getArray "terms").mapM parseSparseTermJson
  let sense ← v.getField "sense" >>= parseSenseJson
  let rhs ← v.getRat "rhs"
  pure { terms, sense, rhs }

private def parseScaleJson (v : JsonVal) : Except String Rat :=
  match v.getField? "scale" with
  | none | some .null => .ok 1
  | some sv => sv.asRat

private def parseCoverItemJson (v : JsonVal) : Except String CoverItemCert := do
  let col ← v.getNat "col"
  let weight ← v.getRat "weight"
  let comp ← match v.getField? "comp" with
    | none | some .null => pure false
    | some bv => bv.asBool
  let inCover ← v.getBool "inCover"
  let alpha ← v.getNat "alpha"
  pure { col, weight, comp, inCover, alpha }

private def parseLotSizingNodeJson (v : JsonVal) : Except String LotSizingNodeCert := do
  let prodCol ← v.getNat "prodCol"
  let invOutCol ← v.getNat "invOutCol"
  let binCol ← v.getNat "binCol"
  let demand ← v.getRat "demand"
  let vubCap ← v.getRat "vubCap"
  let vubRHS ← match v.getField? "vubRHS" with
    | none | some .null => pure (0 : Rat)
    | some rv => rv.asRat
  let inS ← v.getBool "inS"
  let coeffY ← v.getRat "coeffY"
  pure { prodCol, invOutCol, binCol, demand, vubCap, vubRHS, inS, coeffY }

private def checkOptionalWitnesses (v : JsonVal) (cut : CutSpec) : Except String Nat := do
  match v.getField? "solutions" with
  | none | some .null => pure 0
  | some sv =>
    let solsRaw ← sv.asArray
    for i in [:solsRaw.size] do
      let x ← solsRaw[i]!.asArray >>= parseRatArrayJson
      let ok ← checkCutAtPoint cut x
      if !ok then
        let lhs ← evalTerms cut.terms x
        throw s!"witness {i}: cut violated ({lhs} > {cut.rhs})"
    pure solsRaw.size

/-- JSON entrypoint for `cover-check`: universally verifies a lifted binary knapsack cover certificate. -/
def checkKnapsackCoverJson (v : JsonVal) : Except String String := do
  let numVars ← v.getNat "numVars"
  let lb ← v.getArray "lb" >>= parseOptRatArrayJson
  let ub ← v.getArray "ub" >>= parseOptRatArrayJson
  let row ← v.getField "row" >>= parseRowSpecJson
  let sign ← match v.getField? "sign" with
    | none | some .null =>
      match row.sense with
      | .ge => pure (-1 : Rat)
      | _ => pure (1 : Rat)
    | some sv => sv.asRat
  let items ← (← v.getArray "items").mapM parseCoverItemJson
  let knapRHS ← v.getRat "knapRHS"
  let scale ← parseScaleJson v
  let cut ← v.getField "cut" >>= parseCutSpecJson
  let cert : KnapsackCoverCert := { numVars, lb, ub, row, sign, items, knapRHS, scale, cut }
  checkKnapsackCoverCert cert
  let wits ← checkOptionalWitnesses v cut
  pure s!"VERIFIED (cover-check, {items.size} items, {wits} witnesses)"

/-- JSON entrypoint for `lotsizing-check`: universally verifies an `(l, S)` lot-sizing chain cut. -/
def checkLotSizingCutJson (v : JsonVal) : Except String String := do
  let numVars ← v.getNat "numVars"
  let invInCol ← match v.getField? "invInCol" with
    | none | some .null => pure none
    | some iv => some <$> iv.asNat
  let nodes ← (← v.getArray "nodes").mapM parseLotSizingNodeJson
  let cutDem ← v.getRat "cutDem"
  let scale ← parseScaleJson v
  let cut ← v.getField "cut" >>= parseCutSpecJson
  let cert : LotSizingCutCert := { numVars, invInCol, nodes, cutDem, scale, cut }
  checkLotSizingCutCert cert
  let wits ← checkOptionalWitnesses v cut
  pure s!"VERIFIED (lotsizing-check, {nodes.size} nodes, {wits} witnesses)"

/-- JSON entrypoint for `agg-cmir-check`: universally verifies a multi-row aggregated or pairwise-surrogate cMIR cut. -/
def checkAggCMIRJson (v : JsonVal) : Except String String := do
  let numVars ← v.getNat "numVars"
  let isInt ← v.getArray "isInt" >>= parseBoolArrayJson
  let lb ← v.getArray "lb" >>= parseOptRatArrayJson
  let ub ← v.getArray "ub" >>= parseOptRatArrayJson
  let rowsArr ← v.getArray "rows"
  let rows ← rowsArr.mapM fun rv => do
    let row ← parseRowSpecJson rv
    let mu ← rv.getRat "mu"
    pure (row, mu)
  let aggTerms ← (← v.getArray "aggTerms").mapM parseSparseTermJson
  let aggRHS ← v.getRat "aggRHS"
  let delta ← v.getRat "delta"
  let useLower ← v.getArray "useLower" >>= parseBoolArrayJson
  let betaHat ← match v.getField? "betaHat" with
    | none | some .null => pure none
    | some bv => some <$> bv.asRat
  let alphaHat ← match v.getField? "alphaHat" with
    | none | some .null => pure none
    | some av => some <$> (av.asArray >>= parseRatArrayJson)
  let cInt ← match v.getField? "cInt" with
    | none | some .null => pure none
    | some av => some <$> (av.asArray >>= parseRatArrayJson)
  let cCont ← match v.getField? "cCont" with
    | none | some .null => pure none
    | some av => some <$> (av.asArray >>= parseRatArrayJson)
  let scale ← parseScaleJson v
  let cut ← v.getField "cut" >>= parseCutSpecJson
  let cert : AggCMIRCert :=
    { numVars, isInt, lb, ub, rows, aggTerms, aggRHS, delta, useLower,
      betaHat, alphaHat, cInt, cCont, scale, cut }
  checkAggCMIRCert cert
  let wits ← checkOptionalWitnesses v cut
  pure s!"VERIFIED (agg-cmir-check, {rows.size} rows, {wits} witnesses)"

/-- JSON entrypoint for `drop-check`: universally verifies a `finalizeSparseCut` coefficient-drop & scaling certificate. -/
def checkFinalizedCutJson (v : JsonVal) : Except String String := do
  let numVars ← v.getNat "numVars"
  let lb ← v.getArray "lb" >>= parseOptRatArrayJson
  let ub ← v.getArray "ub" >>= parseOptRatArrayJson
  let rawCut ← v.getField "rawCut" >>= parseCutSpecJson
  let scale ← parseScaleJson v
  let finalCut ← v.getField "finalCut" >>= parseCutSpecJson
  let cert : FinalizedCutCert := { numVars, lb, ub, rawCut, scale, finalCut }
  checkFinalizedCutCert cert
  let wits ← checkOptionalWitnesses v finalCut
  pure s!"VERIFIED (drop-check, {numVars} vars, {wits} witnesses)"

end GoblinFormal.Track2C
