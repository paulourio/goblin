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

/-! ### 0. Pure `Init` Helpers: `Option Rat`, `allFin`, and Sparse Accumulation -/

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

private theorem allFin_congr {n : Nat} {p q : Fin n → Bool} (h : ∀ i, p i = q i) :
    allFin n p = allFin n q := by
  induction n with
  | zero => rfl
  | succ n ih =>
    simp only [allFin, ih (fun i => h i.castSucc), h (Fin.last n)]

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

private theorem guardExc_congr {c₁ c₂ : Bool} {m₁ m₂ : String} {k₁ k₂ : Except String Unit}
    (hc : c₁ = c₂) (hm : m₁ = m₂) (hk : k₁ = k₂) :
    guardExc c₁ m₁ k₁ = guardExc c₂ m₂ k₂ := by
  subst hc hm hk
  rfl

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

private def accumScaledTermsAux (n : Nat) (scale : Rat) (acc : Array Rat) (hsz : acc.size = n) :
    List SparseTerm → { a : Array Rat // a.size = n }
  | [] => ⟨acc, hsz⟩
  | t :: ts =>
    if h : t.col < n then
      let acc' := acc.set t.col (acc[t.col]'(by omega) + scale * t.val) (by omega)
      accumScaledTermsAux n scale acc' (by simp [acc', hsz]) ts
    else
      accumScaledTermsAux n scale acc hsz ts

private theorem accumScaledTermsAux_getElem {n : Nat} (scale : Rat) (acc : Array Rat)
    (hsz : acc.size = n) (terms : List SparseTerm) (j : Fin n) :
    (accumScaledTermsAux n scale acc hsz terms).1[j.val]'(by
      rw [(accumScaledTermsAux n scale acc hsz terms).2]; exact j.isLt) =
      acc[j.val]'(by rw [hsz]; exact j.isLt) + scale * termsCoeff terms j.val := by
  induction terms generalizing acc hsz with
  | nil =>
    simp [accumScaledTermsAux, termsCoeff, Rat.mul_zero, Rat.add_zero]
  | cons t ts ih =>
    dsimp only [accumScaledTermsAux]
    split
    · next h =>
      let acc' := acc.set t.col (acc[t.col]'(by omega) + scale * t.val) (by omega)
      have hsz' : acc'.size = n := by simp [acc', hsz]
      rw [ih acc' hsz']
      simp only [termsCoeff]
      by_cases hjeq : t.col = j.val
      · have hbeq : (t.col == j.val) = true := beq_iff_eq.mpr hjeq
        simp only [hbeq, ↓reduceIte]
        have hget : acc'[j.val]'(by rw [hsz']; exact j.isLt) =
            acc[j.val]'(by rw [hsz]; exact j.isLt) + scale * t.val := by
          simp [acc', hjeq]
        rw [hget, Rat.mul_add, Rat.add_assoc]
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

private def accumWeightedRowsAux (n : Nat) (baseRows : Array RowSpec) (pairs : Array (Nat × Rat)) :
    (k : Nat) → k ≤ pairs.size → (acc : Array Rat) → acc.size = n →
      { a : Array Rat // a.size = n }
  | 0, _, acc, hsz => ⟨acc, hsz⟩
  | k + 1, hk, acc, hsz =>
    let p := pairs[k]'(by omega)
    let res1 :=
      if h : p.1 < baseRows.size then
        if p.2 == 0 then ⟨acc, hsz⟩
        else accumScaledTermsAux n p.2 acc hsz baseRows[p.1].terms.toList
      else ⟨acc, hsz⟩
    accumWeightedRowsAux n baseRows pairs k (by omega) res1.1 res1.2

private theorem accumWeightedRowsAux_getElem {n : Nat} (baseRows : Array RowSpec)
    (pairs : Array (Nat × Rat)) (k : Nat) (hk : k ≤ pairs.size)
    (acc : Array Rat) (hsz : acc.size = n) (j : Fin n) :
    (accumWeightedRowsAux n baseRows pairs k hk acc hsz).1[j.val]'(by
      rw [(accumWeightedRowsAux n baseRows pairs k hk acc hsz).2]; exact j.isLt) =
      acc[j.val]'(by rw [hsz]; exact j.isLt) + finSum k (fun i =>
        let p := pairs[i.val]'(by omega)
        if h : p.1 < baseRows.size then p.2 * termsCoeff baseRows[p.1].terms.toList j.val else 0) := by
  induction k generalizing acc hsz with
  | zero =>
    simp [accumWeightedRowsAux, finSum_zero_n, Rat.add_zero]
  | succ k ih =>
    dsimp only [accumWeightedRowsAux]
    let p := pairs[k]'(by omega)
    let res1 :=
      if h : p.1 < baseRows.size then
        if p.2 == 0 then ⟨acc, hsz⟩
        else accumScaledTermsAux n p.2 acc hsz baseRows[p.1].terms.toList
      else ⟨acc, hsz⟩
    have hstep :
        res1.1[j.val]'(by rw [res1.2]; exact j.isLt) =
          acc[j.val]'(by rw [hsz]; exact j.isLt) +
            (if h : p.1 < baseRows.size then p.2 * termsCoeff baseRows[p.1].terms.toList j.val else 0) := by
      dsimp only [res1]
      by_cases hlt : p.1 < baseRows.size
      · simp only [hlt, ↓reduceDIte]
        by_cases hw0 : p.2 = 0
        · simp [hw0, Rat.zero_mul, Rat.add_zero]
        · have hbeq : (p.2 == 0) = false := beq_false_of_ne hw0
          simp only [hbeq, Bool.false_eq_true, ↓reduceIte]
          exact accumScaledTermsAux_getElem p.2 acc hsz baseRows[p.1].terms.toList j
      · simp [hlt, Rat.add_zero]
    have hmain := ih (by omega) res1.1 res1.2
    have hwrap :
        (accumWeightedRowsAux n baseRows pairs (k + 1) hk acc hsz).1[j.val]'(by
          rw [(accumWeightedRowsAux n baseRows pairs (k + 1) hk acc hsz).2]; exact j.isLt) =
          (accumWeightedRowsAux n baseRows pairs k (by omega) res1.1 res1.2).1[j.val]'(by
            rw [(accumWeightedRowsAux n baseRows pairs k (by omega) res1.1 res1.2).2]; exact j.isLt) := rfl
    exact hwrap.trans (hmain.trans (by
      rw [hstep, finSum_succ, Rat.add_assoc]
      congr 1
      exact Rat.add_comm _ _))

private def accumWeightedRowsN (n m : Nat) (baseRows : Array RowSpec) (pairs : Array (Nat × Rat))
    (hsz : pairs.size = m) : { a : Array Rat // a.size = n } :=
  accumWeightedRowsAux n baseRows pairs m (by omega) (Array.replicate n 0) (by simp)

private theorem accumWeightedRowsN_getElem {n m : Nat} (baseRows : Array RowSpec)
    (pairs : Array (Nat × Rat)) (hsz : pairs.size = m) (j : Fin n) :
    (accumWeightedRowsN n m baseRows pairs hsz).1[j.val]'(by
      rw [(accumWeightedRowsN n m baseRows pairs hsz).2]; exact j.isLt) =
      finSum m (fun i =>
        let p := pairs[i.val]'(by omega)
        if h : p.1 < baseRows.size then p.2 * termsCoeff baseRows[p.1].terms.toList j.val else 0) := by
  unfold accumWeightedRowsN
  rw [accumWeightedRowsAux_getElem]
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

/-- Executable existential quantifier over `Fin n`. -/
def anyFin : (n : Nat) → (Fin n → Bool) → Bool
  | 0, _ => false
  | n + 1, p => anyFin n (fun i => p i.castSucc) || p (Fin.last n)

theorem anyFin_eq_true_iff {n : Nat} {p : Fin n → Bool} :
    anyFin n p = true ↔ ∃ i : Fin n, p i = true := by
  induction n with
  | zero =>
    constructor
    · intro h; simp [anyFin] at h
    · rintro ⟨i, _⟩; exact Fin.elim0 i
  | succ n ih =>
    simp only [anyFin, Bool.or_eq_true, ih]
    constructor
    · rintro (⟨i, hi⟩ | hlast)
      · exact ⟨i.castSucc, hi⟩
      · exact ⟨Fin.last n, hlast⟩
    · rintro ⟨i, hi⟩
      by_cases hlt : i.val < n
      · left
        have heq : i = (⟨i.val, hlt⟩ : Fin n).castSucc := Fin.ext rfl
        rw [heq] at hi
        exact ⟨⟨i.val, hlt⟩, hi⟩
      · right
        have heq : i = Fin.last n := Fin.ext (by simp [Fin.val_last]; have := i.isLt; omega)
        rwa [heq] at hi

/-- Full MIP feasibility predicate over optional bounds, integrality flags, and base rows. -/
def MIPFeasible (n : Nat) (isInt : Fin n → Bool) (lb ub : Fin n → Option Rat)
    (baseRows : Array RowSpec) (x : Fin n → Rat) : Prop :=
  boundsSat lb ub x ∧
  (∀ j : Fin n, isInt j = true → isIntVal (x j)) ∧
  (∀ r : Fin baseRows.size, rowSatOnFin n baseRows[r.val] x)

/-- Executable check that column `c` is an integer variable with finite bounds in `[0, 1]`. -/
def isBinaryCol (isInt : Array Bool) (lb ub : Array (Option Rat)) (c : Nat) : Bool :=
  if h : c < isInt.size ∧ c < lb.size ∧ c < ub.size then
    isInt[c]'(h.1) &&
    match lb[c]'(h.2.1), ub[c]'(h.2.2) with
    | some l, some u => decide (0 ≤ l) && decide (u ≤ 1)
    | _, _ => false
  else false

/-- Executable check that column `c` has a finite non-negative lower bound `0 ≤ l`. -/
def isNonnegCol (lb : Array (Option Rat)) (c : Nat) : Bool :=
  if h : c < lb.size then
    match lb[c]'h with
    | some l => decide (0 ≤ l)
    | none => false
  else false

theorem isBinVal_of_int_bounds {x l u : Rat}
    (hint : isIntVal x) (hl : l ≤ x) (hu : x ≤ u) (h0 : 0 ≤ l) (h1 : u ≤ 1) :
    isBinVal x := by
  rcases hint with ⟨z, rfl⟩
  have hz0 : (0 : Int) ≤ z := Rat.intCast_le_intCast.mp (Rat.le_trans h0 hl)
  have hz1 : z ≤ (1 : Int) := Rat.intCast_le_intCast.mp (Rat.le_trans hu h1)
  have hcases : z = 0 ∨ z = 1 := by omega
  rcases hcases with rfl | rfl
  · exact Or.inl rfl
  · exact Or.inr rfl

theorem isBinaryCol_sound {n : Nat} {isInt : Array Bool} {lb ub : Array (Option Rat)}
    {baseRows : Array RowSpec} {x : Fin n → Rat}
    (hIntLen : isInt.size = n) (hlbLen : lb.size = n) (hubLen : ub.size = n)
    (hfeas : MIPFeasible n (fun j => isInt[j.val]'(by omega))
      (fun j => lb[j.val]'(by omega)) (fun j => ub[j.val]'(by omega)) baseRows x)
    {c : Nat} (hc : c < n) (hbin : isBinaryCol isInt lb ub c = true) :
    isBinVal (x ⟨c, hc⟩) := by
  unfold isBinaryCol at hbin
  have hlt : c < isInt.size ∧ c < lb.size ∧ c < ub.size := by omega
  rw [dite_eq_left hlt] at hbin
  simp only [Bool.and_eq_true] at hbin
  have hIntX := hfeas.2.1 ⟨c, hc⟩ hbin.1
  have hbnd := hbin.2
  cases hlb : lb[c]'(by omega) with
  | none => simp [hlb] at hbnd
  | some l =>
    cases hub : ub[c]'(by omega) with
    | none => simp [hlb, hub] at hbnd
    | some u =>
      simp only [hlb, hub, Bool.and_eq_true, decide_eq_true_eq] at hbnd
      exact isBinVal_of_int_bounds hIntX ((hfeas.1 ⟨c, hc⟩).1 l hlb) ((hfeas.1 ⟨c, hc⟩).2 u hub) hbnd.1 hbnd.2

theorem isNonnegCol_sound {n : Nat} {isInt : Array Bool} {lb ub : Array (Option Rat)}
    {baseRows : Array RowSpec} {x : Fin n → Rat}
    (hIntLen : isInt.size = n) (hlbLen : lb.size = n) (hubLen : ub.size = n)
    (hfeas : MIPFeasible n (fun j => isInt[j.val]'(by omega))
      (fun j => lb[j.val]'(by omega)) (fun j => ub[j.val]'(by omega)) baseRows x)
    {c : Nat} (hc : c < n) (hnn : isNonnegCol lb c = true) :
    0 ≤ x ⟨c, hc⟩ := by
  unfold isNonnegCol at hnn
  have hlt : c < lb.size := by omega
  simp only [hlt, ↓reduceDIte] at hnn
  cases hlb : lb[c]'(by omega) with
  | none => simp [hlb] at hnn
  | some l =>
    simp only [hlb, decide_eq_true_eq] at hnn
    exact Rat.le_trans hnn ((hfeas.1 ⟨c, hc⟩).1 l hlb)

/-- Checks whether `baseRow` matches `targetRow` identically in sense, RHS, and dense coefficients. -/
def rowMatchesBaseRow (n : Nat) (baseRow targetRow : RowSpec) : Bool :=
  allTermsInBounds n baseRow.terms &&
  (baseRow.sense == targetRow.sense) &&
  decide (baseRow.rhs = targetRow.rhs) &&
  let baseDense := accumTerms n baseRow.terms
  let tgtDense := accumTerms n targetRow.terms
  allFin n fun j =>
    decide (baseDense.1[j.val]'(by rw [baseDense.2]; exact j.isLt) =
      tgtDense.1[j.val]'(by rw [tgtDense.2]; exact j.isLt))

theorem rowMatchesBaseRow_sound {n : Nat} {baseRow targetRow : RowSpec} {x : Fin n → Rat}
    (hmatch : rowMatchesBaseRow n baseRow targetRow = true)
    (htgtCols : allTermsInBounds n targetRow.terms = true)
    (hbaseSat : rowSatOnFin n baseRow x) :
    rowSatOnFin n targetRow x := by
  unfold rowMatchesBaseRow at hmatch
  simp only [Bool.and_eq_true, beq_iff_eq, decide_eq_true_eq] at hmatch
  rcases hmatch with ⟨⟨⟨hbaseCols, hsense⟩, hrhs⟩, hcoeffs⟩
  have hall := allFin_eq_true_iff.mp hcoeffs
  unfold rowSatOnFin at hbaseSat ⊢
  rw [evalRowOnFin_eq_finSum_accum baseRow x hbaseCols] at hbaseSat
  rw [evalRowOnFin_eq_finSum_accum targetRow x htgtCols]
  have hsum :
      finSum n (fun j => (accumTerms n baseRow.terms).1[j.val]'(by rw [(accumTerms n baseRow.terms).2]; exact j.isLt) * x j) =
      finSum n (fun j => (accumTerms n targetRow.terms).1[j.val]'(by rw [(accumTerms n targetRow.terms).2]; exact j.isLt) * x j) :=
    finSum_congr fun j => by rw [of_decide_eq_true (hall j)]
  rwa [hsum, hsense, hrhs] at hbaseSat

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
  /-- Column index of the binary variable in the knapsack row. -/
  col : Nat
  /-- Non-negative knapsack weight of the item. -/
  weight : Rat
  /-- Whether the binary variable is complemented (`1 - x[col]`) in the knapsack form. -/
  comp : Bool
  /-- Whether the item belongs to the initial minimal cover. -/
  inCover : Bool
  /-- Lifted integer cut coefficient `α` for this item. -/
  alpha : Nat
  deriving Repr, Inhabited

/-- Complete certificate for a minimal/lifted binary knapsack cover cut (with optional positive scaling),
verified end-to-end against variable integrality/bounds and `baseRows`. -/
structure KnapsackCoverCert where
  /-- Number of structural variables in the problem. -/
  numVars : Nat
  /-- Integrality flags for each variable. -/
  isInt : Array Bool
  /-- Optional lower bounds for each variable (`none` = `-∞`). -/
  lb : Array (Option Rat)
  /-- Optional upper bounds for each variable (`none` = `+∞`). -/
  ub : Array (Option Rat)
  /-- Base linear constraints of the MILP instance. -/
  baseRows : Array RowSpec
  /-- Optional index into `baseRows` when the knapsack row comes from a single base row. -/
  rowIdx : Option Nat := none
  /-- Optional `(rowIndex, multiplier)` pairs when the knapsack row is aggregated from `baseRows`. -/
  aggMultipliers : Option (Array (Nat × Rat)) := none
  /-- Source linear row used to derive the binary knapsack inequality. -/
  row : RowSpec
  /-- Orientation multiplier (`1` or `-1`) converting `row` into `≤` form. -/
  sign : Rat
  /-- Binary knapsack items with their weights, complement flags, cover membership, and lifted coefficients. -/
  items : Array CoverItemCert
  /-- Right-hand side of the derived binary knapsack inequality. -/
  knapRHS : Rat
  /-- Positive scaling factor applied to the cut. -/
  scale : Rat := 1
  /-- Final emitted knapsack cover cut. -/
  cut : CutSpec
  deriving Repr, Inhabited

/-- Verifies that the scaled inequality `sign * row ≤ sign * row.rhs` is implied by `baseRows`
(either by matching a base row verbatim, or via legal `aggMultipliers` with optional bound-protected
error compensation). -/
def checkCoverRowFromBase (n : Nat) (lb ub : Array (Option Rat)) (baseRows : Array RowSpec)
    (rowIdxOpt : Option Nat) (aggMultsOpt : Option (Array (Nat × Rat)))
    (row : RowSpec) (sign : Rat) : Bool :=
  match aggMultsOpt with
  | some mus =>
    !mus.isEmpty &&
    allFin mus.size (fun i =>
      let (rIdx, mu) := mus[i.val]
      if h : rIdx < baseRows.size then
        allTermsInBounds n baseRows[rIdx].terms &&
        (decide (mu = 0) || rowLegalMuSignSpec baseRows[rIdx].sense mu)
      else false) &&
    let rowDense := accumTerms n row.terms
    let exactCoeff : Fin n → Rat := fun j =>
      finSum mus.size fun i =>
        let (rIdx, mu) := mus[i.val]
        if h : rIdx < baseRows.size then mu * termsCoeff baseRows[rIdx].terms.toList j.val else 0
    let exactRHS : Rat :=
      finSum mus.size fun i =>
        let (rIdx, mu) := mus[i.val]
        if h : rIdx < baseRows.size then mu * baseRows[rIdx].rhs else 0
    let errOk := allFin n fun j =>
      (aggColErrTerm (sign * rowDense.1[j.val]'(by rw [rowDense.2]; exact j.isLt))
        (exactCoeff j) lb[j.val]! ub[j.val]!).isSome
    let errSum : Rat := finSum n fun j =>
      (aggColErrTerm (sign * rowDense.1[j.val]'(by rw [rowDense.2]; exact j.isLt))
        (exactCoeff j) lb[j.val]! ub[j.val]!).getD 0
    errOk && decide (exactRHS + errSum ≤ sign * row.rhs)
  | none =>
    rowLegalMuSignSpec row.sense sign &&
    match rowIdxOpt with
    | some rIdx =>
      if h : rIdx < baseRows.size then rowMatchesBaseRow n baseRows[rIdx] row else false
    | none =>
      anyFin baseRows.size fun rIdx => rowMatchesBaseRow n baseRows[rIdx.val] row

private theorem checkCoverRowFromBase_sound {n : Nat}
    {isInt : Array Bool} {lb ub : Array (Option Rat)} {baseRows : Array RowSpec}
    {rowIdxOpt : Option Nat} {aggMultsOpt : Option (Array (Nat × Rat))}
    {row : RowSpec} {sign : Rat} {x : Fin n → Rat}
    (hIntLen : isInt.size = n) (hlbLen : lb.size = n) (hubLen : ub.size = n)
    (hrowCols : allTermsInBounds n row.terms = true)
    (hder : checkCoverRowFromBase n lb ub baseRows rowIdxOpt aggMultsOpt row sign = true)
    (hfeas : MIPFeasible n (fun j => isInt[j.val]'(by omega))
      (fun j => lb[j.val]'(by omega)) (fun j => ub[j.val]'(by omega)) baseRows x) :
    finSum n (fun j =>
      (sign * (accumTerms n row.terms).1[j.val]'(by rw [(accumTerms n row.terms).2]; exact j.isLt)) * x j) ≤
      sign * row.rhs := by
  unfold checkCoverRowFromBase at hder
  cases hagg : aggMultsOpt with
  | none =>
    rw [hagg] at hder
    simp only [Bool.and_eq_true] at hder
    rcases hder with ⟨hsignOk, hmatch⟩
    have hrowSat : rowSatOnFin n row x := by
      cases hrIdx : rowIdxOpt with
      | some rIdx =>
        simp only [hrIdx] at hmatch
        by_cases hlt : rIdx < baseRows.size
        · simp only [hlt, ↓reduceDIte] at hmatch
          exact rowMatchesBaseRow_sound hmatch hrowCols (hfeas.2.2 ⟨rIdx, hlt⟩)
        · simp only [hlt, ↓reduceDIte] at hmatch
          contradiction
      | none =>
        simp only [hrIdx] at hmatch
        rcases anyFin_eq_true_iff.mp hmatch with ⟨rIdx, hrMatch⟩
        exact rowMatchesBaseRow_sound hrMatch hrowCols (hfeas.2.2 rIdx)
    unfold rowSatOnFin at hrowSat
    rw [evalRowOnFin_eq_finSum_accum row x hrowCols] at hrowSat
    exact row_agg_legal_sound hsignOk hrowSat
  | some mus =>
    rw [hagg] at hder
    simp only [Bool.and_eq_true, decide_eq_true_eq] at hder
    rcases hder with ⟨⟨_, hmusAll⟩, ⟨herrOk, hrhsLe⟩⟩
    have hmus := allFin_eq_true_iff.mp hmusAll
    have herrAll := allFin_eq_true_iff.mp herrOk
    have hrIdxLt : ∀ i : Fin mus.size, mus[i.val].1 < baseRows.size := by
      intro i
      have hi := hmus i
      split at hi
      · assumption
      · contradiction
    let AFn : Fin mus.size → Fin n → Rat := fun i j =>
      termsCoeff (baseRows[mus[i.val].1]'(hrIdxLt i)).terms.toList j.val
    let senseFn : Fin mus.size → ConstraintSense := fun i => (baseRows[mus[i.val].1]'(hrIdxLt i)).sense
    let rhsFn : Fin mus.size → Rat := fun i => (baseRows[mus[i.val].1]'(hrIdxLt i)).rhs
    let muFn : Fin mus.size → Rat := fun i => mus[i.val].2
    have hmuOk : ∀ i : Fin mus.size, muFn i = 0 ∨ rowLegalMuSignSpec (senseFn i) (muFn i) = true := by
      intro i
      have hi := hmus i
      simp only [hrIdxLt i, ↓reduceDIte, Bool.and_eq_true, Bool.or_eq_true, decide_eq_true_eq] at hi
      exact hi.2
    have hsatOk : ∀ i : Fin mus.size,
        satisfiesSense (finSum n (fun j => AFn i j * x j)) (senseFn i) (rhsFn i) := by
      intro i
      have hi := hmus i
      simp only [hrIdxLt i, ↓reduceDIte, Bool.and_eq_true] at hi
      have hcols := allTermsInBounds_iff.mp hi.1
      have hsat := hfeas.2.2 ⟨mus[i.val].1, hrIdxLt i⟩
      unfold rowSatOnFin evalRowOnFin at hsat
      rwa [evalTermsList_eq_finSum _ x hcols] at hsat
    have hexactAgg := row_agg_sum_sound hmuOk hsatOk
    let rowDense := accumTerms n row.terms
    let cvFn : Fin n → Rat := fun j => sign * rowDense.1[j.val]'(by rw [rowDense.2]; exact j.isLt)
    let lbFn : Fin n → Option Rat := fun j => lb[j.val]'(by omega)
    let ubFn : Fin n → Option Rat := fun j => ub[j.val]'(by omega)
    have hlbB : ∀ j : Fin n, lb[j.val]! = lbFn j := fun j => getElem!_pos lb j.val (by omega)
    have hubB : ∀ j : Fin n, ub[j.val]! = ubFn j := fun j => getElem!_pos ub j.val (by omega)
    have hexCoeffEq : ∀ j : Fin n,
        (finSum mus.size fun i =>
          if h : mus[i.val].1 < baseRows.size then mus[i.val].2 * termsCoeff baseRows[mus[i.val].1].terms.toList j.val else 0) =
        finSum mus.size (fun i => muFn i * AFn i j) :=
      fun j => finSum_congr fun i => by simp [hrIdxLt i, muFn, AFn]
    have hexRHSEq :
        (finSum mus.size fun i =>
          if h : mus[i.val].1 < baseRows.size then mus[i.val].2 * baseRows[mus[i.val].1].rhs else 0) =
        finSum mus.size (fun i => muFn i * rhsFn i) :=
      finSum_congr fun i => by simp [hrIdxLt i, muFn, rhsFn]
    let errTermFn : Fin n → Rat := fun j =>
      (aggColErrTerm (cvFn j) (finSum mus.size fun i => muFn i * AFn i j) (lbFn j) (ubFn j)).getD 0
    have herr : ∀ j : Fin n, ∃ e,
        aggColErrTerm (cvFn j) (finSum mus.size fun i => muFn i * AFn i j) (lbFn j) (ubFn j) = some e ∧
        e ≤ errTermFn j := by
      intro j
      have hj := herrAll j
      rw [hexCoeffEq j, hlbB j, hubB j] at hj
      cases he : aggColErrTerm (cvFn j) (finSum mus.size fun i => muFn i * AFn i j) (lbFn j) (ubFn j) with
      | none => rw [he] at hj; simp at hj
      | some e => exact ⟨e, rfl, by simp [errTermFn, he]⟩
    have hrhsLe' : finSum mus.size (fun i => muFn i * rhsFn i) + 0 + finSum n errTermFn ≤ sign * row.rhs := by
      rw [Rat.add_zero]
      have hcong : finSum n (fun j =>
          (aggColErrTerm (sign * rowDense.1[j.val]'(by rw [rowDense.2]; exact j.isLt))
            (finSum mus.size fun i => if h : mus[i.val].1 < baseRows.size then mus[i.val].2 * termsCoeff baseRows[mus[i.val].1].terms.toList j.val else 0)
            lb[j.val]! ub[j.val]!).getD 0) = finSum n errTermFn :=
        finSum_congr fun j => by dsimp only [errTermFn, cvFn]; rw [hexCoeffEq j, hlbB j, hubB j]
      rwa [hexRHSEq, hcong] at hrhsLe
    exact row_agg_error_bound_opt_sound hexactAgg hfeas.1 herr Rat.le_refl hrhsLe'

/-- Executable verifier for `KnapsackCoverCert` verifying binary integrality/bounds on every
cover item and proving the source row follows from `baseRows`. -/
def checkKnapsackCoverCert (cert : KnapsackCoverCert) : Except String Unit :=
  let n := cert.numVars
  if !(cert.isInt.size == n && cert.lb.size == n && cert.ub.size == n) then
    .error s!"cover cert metadata arrays (isInt, lb, ub) must have length {n}"
  else
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
    decide (it.col < n) &&
    isBinaryCol cert.isInt cert.lb cert.ub it.col &&
    decide (0 ≤ it.weight) &&
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
  guardExc (allTermsInBounds n cert.row.terms) "cover cert row has out-of-bounds column index" <|
  guardExc (allTermsInBounds n cert.cut.terms) "cover cert cut has out-of-bounds column index" <|
  guardExc (decide (0 < cert.scale)) s!"cover cert scale={cert.scale} must be > 0" <|
  guardExc (checkCoverRowFromBase n cert.lb cert.ub cert.baseRows cert.rowIdx cert.aggMultipliers cert.row cert.sign)
    "cover cert row is not verified against baseRows" <|
  guardExc itemsOk "cover cert item is not a binary [0,1] integer variable, has negative weight, or cover alpha != 1" <|
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

/-- Universal soundness of `checkKnapsackCoverCert` over `MIPFeasible` (no unverified binary
or source-row assumptions). -/
theorem checkKnapsackCoverCert_sound (cert : KnapsackCoverCert)
    (hok : checkKnapsackCoverCert cert = .ok ())
    (hIntLen : cert.isInt.size = cert.numVars)
    (hlbLen : cert.lb.size = cert.numVars)
    (hubLen : cert.ub.size = cert.numVars)
    (x : Fin cert.numVars → Rat)
    (hfeas : MIPFeasible cert.numVars
      (fun j => cert.isInt[j.val]'(by omega))
      (fun j => cert.lb[j.val]'(by omega))
      (fun j => cert.ub[j.val]'(by omega))
      cert.baseRows x) :
    evalCutOnFin cert.numVars cert.cut x ≤ cert.cut.rhs := by
  unfold checkKnapsackCoverCert at hok
  simp only [hIntLen, hlbLen, hubLen, beq_self_eq_true, Bool.and_self, Bool.not_true, Bool.false_eq_true, ↓reduceIte] at hok
  rcases guardExc_ok hok with ⟨hrowColsT, hok⟩
  rcases guardExc_ok hok with ⟨hcutColsT, hok⟩
  rcases guardExc_ok hok with ⟨hscaleD, hok⟩
  rcases guardExc_ok hok with ⟨hrowBaseOk, hok⟩
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
    exact hi.1.1.1
  have hbinItem : ∀ i : Fin cert.items.size, isBinaryCol cert.isInt cert.lb cert.ub cert.items[i.val].col = true := by
    intro i
    have hi := hitemsAll i
    simp only [Bool.and_eq_true] at hi
    exact hi.1.1.2
  have hxBin : ∀ i : Fin cert.items.size, isBinVal (x ⟨cert.items[i.val].col, hcolLt i⟩) :=
    fun i => isBinaryCol_sound hIntLen hlbLen hubLen hfeas (hcolLt i) (hbinItem i)
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
      cert.sign * cert.row.rhs :=
    checkCoverRowFromBase_sound hIntLen hlbLen hubLen hrowColsT hrowBaseOk hfeas
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
    hfeas.1 hxBin hrowLe hwNN hcovPos hcovAlpha hrelaxProp hknapLe'
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
  /-- Production variable column index for this period. -/
  prodCol : Nat
  /-- Outgoing inventory variable column index for this period. -/
  invOutCol : Nat
  /-- Binary setup variable column index for this period. -/
  binCol : Nat
  /-- Demand for this period. -/
  demand : Rat
  /-- Variable upper bound capacity coefficient linking production to the binary setup variable. -/
  vubCap : Rat
  /-- Right-hand side of the variable upper bound constraint. -/
  vubRHS : Rat := 0
  /-- Whether this period is included in the cut subset `S`. -/
  inS : Bool
  /-- Coefficient on the binary setup variable `y` in the `(l, S)` cut. -/
  coeffY : Rat
  /-- Optional index in `baseRows` of the flow-conservation row for this period. -/
  flowRowIdx : Option Nat := none
  /-- Optional index in `baseRows` of the VUB row for this period. -/
  vubRowIdx : Option Nat := none
  deriving Repr, Inhabited

/-- Complete certificate for an `(l, S)` lot-sizing flow-conservation cut (with optional positive scaling),
verified end-to-end against variable domains, column distinctness, and `baseRows`. -/
structure LotSizingCutCert where
  /-- Number of structural variables in the problem. -/
  numVars : Nat
  /-- Integrality flags for each variable. -/
  isInt : Array Bool
  /-- Optional lower bounds for each variable (`none` = `-∞`). -/
  lb : Array (Option Rat)
  /-- Optional upper bounds for each variable (`none` = `+∞`). -/
  ub : Array (Option Rat)
  /-- Base linear constraints of the MILP instance. -/
  baseRows : Array RowSpec
  /-- Optional incoming inventory variable column index at the start of the chain. -/
  invInCol : Option Nat
  /-- Ordered sequence of lot-sizing period nodes forming the `(l, S)` chain. -/
  nodes : Array LotSizingNodeCert
  /-- Cumulative demand right-hand side for the `(l, S)` cut. -/
  cutDem : Rat
  /-- Positive scaling factor applied to the cut. -/
  scale : Rat := 1
  /-- Final emitted `(l, S)` lot-sizing cut. -/
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

private def optColCoeff (opt : Option Nat) (j : Nat) : Rat :=
  match opt with
  | some c => if c == j then 1 else 0
  | none => 0

private def optColVal {n : Nat} (opt : Option Nat) (x : Fin n → Rat) : Rat :=
  match opt with
  | some c => if hc : c < n then x ⟨c, hc⟩ else 0
  | none => 0

/-- Checks whether an equality base row matches `s * (prevInv + x[prodCol] - x[invOutCol]) = s * demand`. -/
def flowRowMatchesOriented (n : Nat) (prevInvCol : Option Nat) (prodCol invOutCol : Nat)
    (demand s : Rat) (r : RowSpec) : Bool :=
  allTermsInBounds n r.terms &&
  (r.sense == .eq) &&
  decide (r.rhs = s * demand) &&
  let rd := accumTerms n r.terms
  allFin n fun j =>
    let exp : Rat :=
      optColCoeff prevInvCol j.val +
      (if prodCol == j.val then (1 : Rat) else 0) +
      (if invOutCol == j.val then (-1 : Rat) else 0)
    decide (rd.1[j.val]'(by rw [rd.2]; exact j.isLt) = s * exp)

/-- Checks whether a base row matches a lot-sizing flow conservation equality (up to `±1` sign). -/
def flowRowMatchesBase (n : Nat) (prevInvCol : Option Nat) (prodCol invOutCol : Nat)
    (demand : Rat) (r : RowSpec) : Bool :=
  flowRowMatchesOriented n prevInvCol prodCol invOutCol demand 1 r ||
  flowRowMatchesOriented n prevInvCol prodCol invOutCol demand (-1) r

private theorem flowRowMatchesOriented_sound {n : Nat}
    {prevInvCol : Option Nat} {prodCol invOutCol : Nat} {demand s : Rat} {r : RowSpec}
    {x : Fin n → Rat}
    (hsNe : s ≠ 0)
    (hp : prodCol < n) (hi : invOutCol < n)
    (hprev : ∀ c, prevInvCol = some c → c < n)
    (hmatch : flowRowMatchesOriented n prevInvCol prodCol invOutCol demand s r = true)
    (hsat : rowSatOnFin n r x) :
    optColVal prevInvCol x + x ⟨prodCol, hp⟩ - x ⟨invOutCol, hi⟩ = demand := by
  unfold flowRowMatchesOriented at hmatch
  simp only [Bool.and_eq_true] at hmatch
  rcases hmatch with ⟨⟨⟨hcols, hsenseB⟩, hrhsD⟩, hcoeffs⟩
  have hsense : r.sense = .eq := beq_iff_eq.mp hsenseB
  have hrhs : r.rhs = s * demand := of_decide_eq_true hrhsD
  have hall := allFin_eq_true_iff.mp hcoeffs
  unfold rowSatOnFin at hsat
  rw [evalRowOnFin_eq_finSum_accum r x hcols, hsense] at hsat
  dsimp only [satisfiesSense] at hsat
  rw [hrhs] at hsat
  have hsumEq :
      finSum n (fun j => (accumTerms n r.terms).1[j.val]'(by rw [(accumTerms n r.terms).2]; exact j.isLt) * x j) =
      s * (optColVal prevInvCol x + x ⟨prodCol, hp⟩ - x ⟨invOutCol, hi⟩) := by
    have h1 :
        finSum n (fun j => (accumTerms n r.terms).1[j.val]'(by rw [(accumTerms n r.terms).2]; exact j.isLt) * x j) =
        s * finSum n (fun j =>
          (optColCoeff prevInvCol j.val +
           (if prodCol == j.val then (1 : Rat) else 0) +
           (if invOutCol == j.val then (-1 : Rat) else 0)) * x j) := by
      rw [← finSum_const_mul]
      apply finSum_congr
      intro j
      have hj := of_decide_eq_true (hall j)
      rw [hj, Rat.mul_assoc]
    rw [h1]
    congr 1
    have hadd :
        finSum n (fun j =>
          (optColCoeff prevInvCol j.val +
           (if prodCol == j.val then (1 : Rat) else 0) +
           (if invOutCol == j.val then (-1 : Rat) else 0)) * x j) =
        finSum n (fun j => optColCoeff prevInvCol j.val * x j) +
        finSum n (fun j => (if prodCol == j.val then (1 : Rat) else 0) * x j) +
        finSum n (fun j => (if invOutCol == j.val then (-1 : Rat) else 0) * x j) := by
      rw [← finSum_add, ← finSum_add]
      exact finSum_congr fun j => by rw [Rat.add_mul, Rat.add_mul]
    rw [hadd]
    have hPartPrev :
        finSum n (fun j => optColCoeff prevInvCol j.val * x j) = optColVal prevInvCol x := by
      cases hpc : prevInvCol with
      | none => simp [optColCoeff, optColVal]
      | some c =>
        have hc := hprev c hpc
        simp only [optColCoeff, optColVal, hc, ↓reduceDIte]
        have hcong :
            finSum n (fun j : Fin n => (if c == j.val then (1 : Rat) else 0) * x j) =
            finSum n (fun j : Fin n => if j = ⟨c, hc⟩ then x ⟨c, hc⟩ else 0) := by
          apply finSum_congr; intro j
          by_cases hj : j = ⟨c, hc⟩
          · subst hj; simp [Rat.one_mul]
          · have hne : (c == j.val) = false := beq_false_of_ne (fun h => hj (Fin.ext h.symm))
            simp [hj, hne]
        rw [hcong, finSum_single]
    have hPartProd :
        finSum n (fun j => (if prodCol == j.val then (1 : Rat) else 0) * x j) = x ⟨prodCol, hp⟩ := by
      have hcong :
          finSum n (fun j : Fin n => (if prodCol == j.val then (1 : Rat) else 0) * x j) =
          finSum n (fun j : Fin n => if j = ⟨prodCol, hp⟩ then x ⟨prodCol, hp⟩ else 0) := by
        apply finSum_congr; intro j
        by_cases hj : j = ⟨prodCol, hp⟩
        · subst hj; simp [Rat.one_mul]
        · have hne : (prodCol == j.val) = false := beq_false_of_ne (fun h => hj (Fin.ext h.symm))
          simp [hj, hne]
      rw [hcong, finSum_single]
    have hPartInv :
        finSum n (fun j => (if invOutCol == j.val then (-1 : Rat) else 0) * x j) = -x ⟨invOutCol, hi⟩ := by
      have hcong :
          finSum n (fun j : Fin n => (if invOutCol == j.val then (-1 : Rat) else 0) * x j) =
          finSum n (fun j : Fin n => if j = ⟨invOutCol, hi⟩ then -x ⟨invOutCol, hi⟩ else 0) := by
        apply finSum_congr; intro j
        by_cases hj : j = ⟨invOutCol, hi⟩
        · subst hj; simp [Rat.neg_mul, Rat.one_mul]
        · have hne : (invOutCol == j.val) = false := beq_false_of_ne (fun h => hj (Fin.ext h.symm))
          simp [hj, hne]
      rw [hcong, finSum_single]
    rw [hPartPrev, hPartProd, hPartInv, ← Rat.sub_eq_add_neg]
  rw [hsumEq] at hsat
  have hmul := congrArg (s⁻¹ * ·) hsat
  rwa [← Rat.mul_assoc, ← Rat.mul_assoc, Rat.inv_mul_cancel _ hsNe, Rat.one_mul, Rat.one_mul] at hmul

private theorem flowRowMatchesBase_sound {n : Nat}
    {prevInvCol : Option Nat} {prodCol invOutCol : Nat} {demand : Rat} {r : RowSpec}
    {x : Fin n → Rat}
    (hp : prodCol < n) (hi : invOutCol < n)
    (hprev : ∀ c, prevInvCol = some c → c < n)
    (hmatch : flowRowMatchesBase n prevInvCol prodCol invOutCol demand r = true)
    (hsat : rowSatOnFin n r x) :
    optColVal prevInvCol x + x ⟨prodCol, hp⟩ - x ⟨invOutCol, hi⟩ = demand := by
  unfold flowRowMatchesBase at hmatch
  rcases Bool.or_eq_true _ _ |>.mp hmatch with h1 | hm1
  · exact flowRowMatchesOriented_sound (by decide) hp hi hprev h1 hsat
  · exact flowRowMatchesOriented_sound (by decide) hp hi hprev hm1 hsat

/-- Checks whether a base row scaled by `s ∈ {1, -1}` implies `x[prodCol] - vubCap * x[binCol] ≤ vubRHS`. -/
def vubRowImpliesOriented (n : Nat) (lb : Array (Option Rat))
    (prodCol binCol : Nat) (vubCap vubRHS s : Rat) (r : RowSpec) : Bool :=
  if hp : prodCol < n then
    if hb : binCol < n then
      allTermsInBounds n r.terms &&
      rowLegalMuSignSpec r.sense s &&
      decide (prodCol ≠ binCol) &&
      let rd := accumTerms n r.terms
      let ap := s * rd.1[prodCol]'(by rw [rd.2]; exact hp)
      let ay := s * rd.1[binCol]'(by rw [rd.2]; exact hb)
      decide (0 < ap) &&
      decide (-ay ≤ ap * vubCap) &&
      decide (s * r.rhs ≤ ap * vubRHS) &&
      allFin n fun j =>
        if j.val == prodCol || j.val == binCol then true
        else
          let cj := s * rd.1[j.val]'(by rw [rd.2]; exact j.isLt)
          decide (cj = 0) || (decide (0 < cj) && isNonnegCol lb j.val)
    else false
  else false

/-- Checks whether a base row implies `x[prodCol] - vubCap * x[binCol] ≤ vubRHS` (either orientation). -/
def vubRowImpliesBase (n : Nat) (lb : Array (Option Rat))
    (prodCol binCol : Nat) (vubCap vubRHS : Rat) (r : RowSpec) : Bool :=
  vubRowImpliesOriented n lb prodCol binCol vubCap vubRHS 1 r ||
  vubRowImpliesOriented n lb prodCol binCol vubCap vubRHS (-1) r

private theorem vubRowImpliesOriented_sound {n : Nat}
    {isInt : Array Bool} {lb ub : Array (Option Rat)} {baseRows : Array RowSpec}
    {prodCol binCol : Nat} {vubCap vubRHS s : Rat} {r : RowSpec} {x : Fin n → Rat}
    (hIntLen : isInt.size = n) (hlbLen : lb.size = n) (hubLen : ub.size = n)
    (hp : prodCol < n) (hb : binCol < n)
    (hfeas : MIPFeasible n (fun j => isInt[j.val]'(by omega))
      (fun j => lb[j.val]'(by omega)) (fun j => ub[j.val]'(by omega)) baseRows x)
    (hyNN : 0 ≤ x ⟨binCol, hb⟩)
    (himp : vubRowImpliesOriented n lb prodCol binCol vubCap vubRHS s r = true)
    (hsat : rowSatOnFin n r x) :
    x ⟨prodCol, hp⟩ - vubCap * x ⟨binCol, hb⟩ ≤ vubRHS := by
  unfold vubRowImpliesOriented at himp
  simp only [hp, hb, ↓reduceDIte, Bool.and_eq_true, decide_eq_true_eq] at himp
  rcases himp with ⟨⟨⟨hcols, hleg⟩, hne⟩, ⟨⟨⟨hapPos, hayLe⟩, hrhsLe⟩, hothers⟩⟩
  have hothersAll := allFin_eq_true_iff.mp hothers
  let rd := accumTerms n r.terms
  let cFn : Fin n → Rat := fun j => s * rd.1[j.val]'(by rw [rd.2]; exact j.isLt)
  have hscaled : finSum n (fun j => cFn j * x j) ≤ s * r.rhs := by
    unfold rowSatOnFin at hsat
    rw [evalRowOnFin_eq_finSum_accum r x hcols] at hsat
    exact row_agg_legal_sound hleg hsat
  let pFin : Fin n := ⟨prodCol, hp⟩
  let bFin : Fin n := ⟨binCol, hb⟩
  have hpbNe : pFin ≠ bFin := fun h => hne (congrArg Fin.val h)
  have htwoLe : cFn pFin * x pFin + cFn bFin * x bFin ≤ finSum n (fun j => cFn j * x j) := by
    have hterm : ∀ j : Fin n,
        ((if j = pFin then cFn pFin * x pFin else 0) + (if j = bFin then cFn bFin * x bFin else 0)) ≤
        cFn j * x j := by
      intro j
      by_cases hjp : j = pFin
      · subst hjp; simp [hpbNe, Rat.add_zero]
      · by_cases hjb : j = bFin
        · subst hjb; simp [hjp, Rat.zero_add]
        · simp only [hjp, hjb, ↓reduceIte, Rat.add_zero]
          have ho := hothersAll j
          have hne1 : (j.val == prodCol) = false := beq_false_of_ne (fun h => hjp (Fin.ext h))
          have hne2 : (j.val == binCol) = false := beq_false_of_ne (fun h => hjb (Fin.ext h))
          simp only [hne1, hne2, Bool.or_self, Bool.false_eq_true, ↓reduceIte,
            Bool.or_eq_true, Bool.and_eq_true, decide_eq_true_eq] at ho
          rcases ho with h0 | ⟨hcPos, hnn⟩
          · change 0 ≤ cFn j * x j
            have hc0 : cFn j = 0 := h0
            rw [hc0, Rat.zero_mul]; exact Rat.le_refl
          · exact Rat.mul_nonneg (Rat.le_of_lt hcPos)
              (isNonnegCol_sound hIntLen hlbLen hubLen hfeas j.isLt hnn)
    have hsum := finSum_le_finSum hterm
    rwa [finSum_add, finSum_single, finSum_single] at hsum
  have hpairLe : cFn pFin * x pFin + cFn bFin * x bFin ≤ cFn pFin * vubRHS :=
    Rat.le_trans htwoLe (Rat.le_trans hscaled hrhsLe)
  have hayNeg : -(cFn pFin * vubCap) ≤ cFn bFin := by
    have h := Rat.neg_le_neg hayLe
    simpa [Rat.neg_neg] using h
  have hmulY : -(cFn pFin * vubCap) * x bFin ≤ cFn bFin * x bFin :=
    Rat.mul_le_mul_of_nonneg_right hayNeg hyNN
  have hcomb : cFn pFin * (x pFin - vubCap * x bFin) ≤ cFn pFin * vubRHS := by
    have heq : cFn pFin * (x pFin - vubCap * x bFin) = cFn pFin * x pFin + -(cFn pFin * vubCap) * x bFin := by
      rw [rat_mul_sub, Rat.sub_eq_add_neg, ← Rat.mul_assoc, ← Rat.neg_mul]
    rw [heq]
    exact Rat.le_trans (Rat.add_le_add_left.mpr hmulY) hpairLe
  exact Rat.le_of_mul_le_mul_left hcomb hapPos

private theorem vubRowImpliesBase_sound {n : Nat}
    {isInt : Array Bool} {lb ub : Array (Option Rat)} {baseRows : Array RowSpec}
    {prodCol binCol : Nat} {vubCap vubRHS : Rat} {r : RowSpec} {x : Fin n → Rat}
    (hIntLen : isInt.size = n) (hlbLen : lb.size = n) (hubLen : ub.size = n)
    (hp : prodCol < n) (hb : binCol < n)
    (hfeas : MIPFeasible n (fun j => isInt[j.val]'(by omega))
      (fun j => lb[j.val]'(by omega)) (fun j => ub[j.val]'(by omega)) baseRows x)
    (hyNN : 0 ≤ x ⟨binCol, hb⟩)
    (himp : vubRowImpliesBase n lb prodCol binCol vubCap vubRHS r = true)
    (hsat : rowSatOnFin n r x) :
    x ⟨prodCol, hp⟩ - vubCap * x ⟨binCol, hb⟩ ≤ vubRHS := by
  unfold vubRowImpliesBase at himp
  rcases Bool.or_eq_true _ _ |>.mp himp with h1 | hm1
  · exact vubRowImpliesOriented_sound hIntLen hlbLen hubLen hp hb hfeas hyNN h1 hsat
  · exact vubRowImpliesOriented_sound hIntLen hlbLen hubLen hp hb hfeas hyNN hm1 hsat

/-- Executable verifier for `LotSizingCutCert` enforcing non-negative/binary domains, column
distinctness (`invInCol`, `prodCol`, `invOutCol` pairwise distinct and disjoint from all `binCol`),
and base-row verification of every node's flow conservation equality and VUB constraint. -/
def checkLotSizingCutCert (cert : LotSizingCutCert) : Except String Unit :=
  let n := cert.numVars
  if !(cert.isInt.size == n && cert.lb.size == n && cert.ub.size == n) then
    .error s!"lot-sizing cert metadata arrays (isInt, lb, ub) must have length {n}"
  else
  let L := cert.nodes.size
  let invOk : Bool := match cert.invInCol with
    | none => true
    | some c => decide (c < n) && isNonnegCol cert.lb c
  let distinctOk : Bool := allFin L fun τ =>
    let nd := cert.nodes[τ.val]
    let invDisj := match cert.invInCol with
      | none => true
      | some c => decide (c ≠ nd.prodCol) && decide (c ≠ nd.invOutCol) && decide (c ≠ nd.binCol)
    let selfDisj := decide (nd.prodCol ≠ nd.invOutCol) &&
      decide (nd.binCol ≠ nd.prodCol) && decide (nd.binCol ≠ nd.invOutCol)
    let pairDisj := allFin L fun τ' =>
      let nd' := cert.nodes[τ'.val]
      let binDisj := decide (nd.binCol ≠ nd'.prodCol) && decide (nd.binCol ≠ nd'.invOutCol)
      let flowDisj := if τ'.val == τ.val then true
        else decide (nd.prodCol ≠ nd'.prodCol) &&
             decide (nd.prodCol ≠ nd'.invOutCol) &&
             decide (nd.invOutCol ≠ nd'.invOutCol)
      binDisj && flowDisj
    invDisj && selfDisj && pairDisj
  let dFn : Fin L → Rat := fun τ => cert.nodes[τ.val].demand
  let nodesOk := allFin L fun τ =>
    let nd := cert.nodes[τ.val]
    let colOk := decide (nd.prodCol < n) && decide (nd.invOutCol < n) && decide (nd.binCol < n)
    let domOk := isNonnegCol cert.lb nd.prodCol && isNonnegCol cert.lb nd.invOutCol &&
      isBinaryCol cert.isInt cert.lb cert.ub nd.binCol
    let valOk := decide (0 ≤ nd.demand) && decide (0 ≤ nd.vubCap) &&
      decide (nd.vubRHS ≤ 0) && decide (0 ≤ nd.coeffY)
    let covOk := if nd.inS then
        decide (suffixDemand dFn τ ≤ nd.coeffY) || hasEarlierBinInS cert.nodes τ.val nd.binCol
      else
        decide (nd.coeffY == 0)
    let prevInvCol : Option Nat :=
      if h0 : τ.val = 0 then cert.invInCol else some (cert.nodes[τ.val - 1]'(by omega)).invOutCol
    let flowOk := match nd.flowRowIdx with
      | some rIdx =>
        if h : rIdx < cert.baseRows.size then
          flowRowMatchesBase n prevInvCol nd.prodCol nd.invOutCol nd.demand cert.baseRows[rIdx]
        else false
      | none =>
        anyFin cert.baseRows.size fun rIdx =>
          flowRowMatchesBase n prevInvCol nd.prodCol nd.invOutCol nd.demand cert.baseRows[rIdx.val]
    let vubOk := match nd.vubRowIdx with
      | some rIdx =>
        if h : rIdx < cert.baseRows.size then
          vubRowImpliesBase n cert.lb nd.prodCol nd.binCol nd.vubCap nd.vubRHS cert.baseRows[rIdx]
        else false
      | none =>
        anyFin cert.baseRows.size fun rIdx =>
          vubRowImpliesBase n cert.lb nd.prodCol nd.binCol nd.vubCap nd.vubRHS cert.baseRows[rIdx.val]
    colOk && domOk && valOk && covOk && flowOk && vubOk
  let totalDem := finSum L dFn
  let cutDense := accumTerms n cert.cut.terms
  let coeffOk := allFin n fun j =>
    decide (cutDense.1[j.val]'(by rw [cutDense.2]; exact j.isLt) =
      cert.scale * lotSizingExpectedCoeff cert j.val)
  guardExc (L != 0) "lot-sizing cert requires at least one flow node" <|
  guardExc (decide (0 < cert.scale)) s!"lot-sizing cert scale={cert.scale} must be > 0" <|
  guardExc (allTermsInBounds n cert.cut.terms) "lot-sizing cert cut has out-of-bounds column index" <|
  guardExc invOk "lot-sizing cert invInCol out of bounds or not non-negative" <|
  guardExc distinctOk "lot-sizing cert has colliding prodCol/invOutCol/invInCol/binCol column indices" <|
  guardExc nodesOk "lot-sizing cert node failed domain, base-row flow/VUB, or suffix-demand check" <|
  guardExc (decide (cert.cutDem ≤ totalDem)) s!"lot-sizing cert cutDem={cert.cutDem} > totalDemand={totalDem}" <|
  guardExc coeffOk "lot-sizing cert cut coefficients do not match scaled (l, S) terms" <|
  guardExc (decide (cert.scale * (-cert.cutDem) ≤ cert.cut.rhs))
    s!"lot-sizing cert cut.rhs={cert.cut.rhs} < scaled -cutDem={cert.scale * (-cert.cutDem)}" <|
  .ok ()

set_option maxHeartbeats 400000 in
/-- Universal soundness of `checkLotSizingCutCert` over `MIPFeasible` (no unverified flow,
VUB, non-negativity, or binary assumptions). -/
theorem checkLotSizingCutCert_sound (cert : LotSizingCutCert)
    (hok : checkLotSizingCutCert cert = .ok ())
    (hIntLen : cert.isInt.size = cert.numVars)
    (hlbLen : cert.lb.size = cert.numVars)
    (hubLen : cert.ub.size = cert.numVars)
    (x : Fin cert.numVars → Rat)
    (hfeas : MIPFeasible cert.numVars
      (fun j => cert.isInt[j.val]'(by omega))
      (fun j => cert.lb[j.val]'(by omega))
      (fun j => cert.ub[j.val]'(by omega))
      cert.baseRows x) :
    evalCutOnFin cert.numVars cert.cut x ≤ cert.cut.rhs := by
  unfold checkLotSizingCutCert at hok
  simp only [hIntLen, hlbLen, hubLen, beq_self_eq_true, Bool.and_self, Bool.not_true, Bool.false_eq_true, ↓reduceIte] at hok
  rcases guardExc_ok hok with ⟨_, hok⟩
  rcases guardExc_ok hok with ⟨hscaleD, hok⟩
  rcases guardExc_ok hok with ⟨hcutColsT, hok⟩
  rcases guardExc_ok hok with ⟨hinvOkT, hok⟩
  rcases guardExc_ok hok with ⟨_, hok⟩
  rcases guardExc_ok hok with ⟨hnodesOkT, hok⟩
  rcases guardExc_ok hok with ⟨hcutDemD, hok⟩
  rcases guardExc_ok hok with ⟨hcoeffOkT, hok⟩
  rcases guardExc_ok hok with ⟨hcutRHSD, _⟩
  have hscalePos : 0 < cert.scale := of_decide_eq_true hscaleD
  have hInvInOk : ∀ c, cert.invInCol = some c → c < cert.numVars ∧ isNonnegCol cert.lb c = true := by
    intro c hc
    simp only [hc, Bool.and_eq_true, decide_eq_true_eq] at hinvOkT
    exact hinvOkT
  have hInvInLt : ∀ c, cert.invInCol = some c → c < cert.numVars := fun c hc => (hInvInOk c hc).1
  have hnodesAll := allFin_eq_true_iff.mp hnodesOkT
  have hcutDemLe : cert.cutDem ≤ finSum cert.nodes.size (fun τ => cert.nodes[τ.val].demand) :=
    of_decide_eq_true hcutDemD
  have hcoeffAll := allFin_eq_true_iff.mp hcoeffOkT
  have hcutRHSLe : cert.scale * (-cert.cutDem) ≤ cert.cut.rhs :=
    of_decide_eq_true hcutRHSD
  have hnodeUnpack : ∀ τ : Fin cert.nodes.size,
      let nd := cert.nodes[τ.val]
      (nd.prodCol < cert.numVars ∧ nd.invOutCol < cert.numVars ∧ nd.binCol < cert.numVars) ∧
      (isNonnegCol cert.lb nd.prodCol = true ∧ isNonnegCol cert.lb nd.invOutCol = true ∧
        isBinaryCol cert.isInt cert.lb cert.ub nd.binCol = true) ∧
      (0 ≤ nd.demand ∧ 0 ≤ nd.vubCap ∧ nd.vubRHS ≤ 0 ∧ 0 ≤ nd.coeffY) ∧
      (if nd.inS then
          decide (suffixDemand (fun s => cert.nodes[s.val].demand) τ ≤ nd.coeffY) || hasEarlierBinInS cert.nodes τ.val nd.binCol
        else decide (nd.coeffY == 0)) = true ∧
      (let prevInvCol : Option Nat :=
          if h0 : τ.val = 0 then cert.invInCol else some (cert.nodes[τ.val - 1]'(by omega)).invOutCol
       (match nd.flowRowIdx with
        | some rIdx => if h : rIdx < cert.baseRows.size then flowRowMatchesBase cert.numVars prevInvCol nd.prodCol nd.invOutCol nd.demand cert.baseRows[rIdx] else false
        | none => anyFin cert.baseRows.size fun rIdx => flowRowMatchesBase cert.numVars prevInvCol nd.prodCol nd.invOutCol nd.demand cert.baseRows[rIdx.val]) = true) ∧
      ((match nd.vubRowIdx with
        | some rIdx => if h : rIdx < cert.baseRows.size then vubRowImpliesBase cert.numVars cert.lb nd.prodCol nd.binCol nd.vubCap nd.vubRHS cert.baseRows[rIdx] else false
        | none => anyFin cert.baseRows.size fun rIdx => vubRowImpliesBase cert.numVars cert.lb nd.prodCol nd.binCol nd.vubCap nd.vubRHS cert.baseRows[rIdx.val]) = true) := by
    intro τ
    have h := hnodesAll τ
    simp only [Bool.and_eq_true] at h
    rcases h with ⟨⟨⟨⟨⟨hcol, hdom⟩, hval⟩, hcov⟩, hfl⟩, hvub⟩
    simp only [decide_eq_true_eq] at hcol hval
    exact ⟨⟨hcol.1.1, hcol.1.2, hcol.2⟩, ⟨hdom.1.1, hdom.1.2, hdom.2⟩,
      ⟨hval.1.1.1, hval.1.1.2, hval.1.2, hval.2⟩, hcov, hfl, hvub⟩
  have hpLt : ∀ τ : Fin cert.nodes.size, cert.nodes[τ.val].prodCol < cert.numVars := fun τ => (hnodeUnpack τ).1.1
  have hiLt : ∀ τ : Fin cert.nodes.size, cert.nodes[τ.val].invOutCol < cert.numVars := fun τ => (hnodeUnpack τ).1.2.1
  have hbLt : ∀ τ : Fin cert.nodes.size, cert.nodes[τ.val].binCol < cert.numVars := fun τ => (hnodeUnpack τ).1.2.2
  have hvubRHSLe : ∀ τ : Fin cert.nodes.size, cert.nodes[τ.val].vubRHS ≤ 0 := fun τ => (hnodeUnpack τ).2.2.1.2.2.1
  have hcoeffYNN : ∀ τ : Fin cert.nodes.size, 0 ≤ cert.nodes[τ.val].coeffY := fun τ => (hnodeUnpack τ).2.2.1.2.2.2
  let xExt : Nat → Rat := fun c => if h : c < cert.numVars then x ⟨c, h⟩ else 0
  let I0 : Rat := match cert.invInCol with | none => 0 | some c => xExt c
  have hI0_nn : 0 ≤ I0 := by
    dsimp [I0, xExt]
    cases hI : cert.invInCol with
    | none => exact Rat.le_refl
    | some c =>
      rcases hInvInOk c hI with ⟨hc, hnn⟩
      simp only [hc, ↓reduceDIte]
      exact isNonnegCol_sound hIntLen hlbLen hubLen hfeas hc hnn
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
    rcases hnodeUnpack τ with ⟨_, ⟨hpNN, hiNN, hbBin⟩, _, _, hflOk, hvubOk⟩
    have hpx : 0 ≤ xProd τ := isNonnegCol_sound hIntLen hlbLen hubLen hfeas (hpLt τ) hpNN
    have hix : 0 ≤ xInv τ := isNonnegCol_sound hIntLen hlbLen hubLen hfeas (hiLt τ) hiNN
    have hbin : isBinVal (x ⟨cert.nodes[τ.val].binCol, hbLt τ⟩) :=
      isBinaryCol_sound hIntLen hlbLen hubLen hfeas (hbLt τ) hbBin
    have hyNN : 0 ≤ x ⟨cert.nodes[τ.val].binCol, hbLt τ⟩ := by
      rcases hbin with h0 | h1
      · rw [h0]; exact Rat.le_refl
      · rw [h1]; decide
    have hvub : xProd τ - cert.nodes[τ.val].vubCap * x ⟨cert.nodes[τ.val].binCol, hbLt τ⟩ ≤ cert.nodes[τ.val].vubRHS := by
      cases hvi : cert.nodes[τ.val].vubRowIdx with
      | some rIdx =>
        simp only [hvi] at hvubOk
        by_cases hr : rIdx < cert.baseRows.size
        · simp only [hr, ↓reduceDIte] at hvubOk
          exact vubRowImpliesBase_sound hIntLen hlbLen hubLen (hpLt τ) (hbLt τ) hfeas hyNN hvubOk (hfeas.2.2 ⟨rIdx, hr⟩)
        · simp only [hr, ↓reduceDIte] at hvubOk
          contradiction
      | none =>
        simp only [hvi] at hvubOk
        rcases anyFin_eq_true_iff.mp hvubOk with ⟨rIdx, hrOk⟩
        exact vubRowImpliesBase_sound hIntLen hlbLen hubLen (hpLt τ) (hbLt τ) hfeas hyNN hrOk (hfeas.2.2 rIdx)
    let prevInvCol : Option Nat :=
      if h0 : τ.val = 0 then cert.invInCol else some (cert.nodes[τ.val - 1]'(by omega)).invOutCol
    have hprevLt : ∀ c, prevInvCol = some c → c < cert.numVars := by
      intro c hc
      dsimp only [prevInvCol] at hc
      split at hc
      · exact hInvInLt c hc
      · simp only [Option.some.injEq] at hc
        rw [← hc]
        exact hiLt ⟨τ.val - 1, by omega⟩
    have hflRaw : optColVal prevInvCol x + xProd τ - xInv τ = dFn τ := by
      cases hfi : cert.nodes[τ.val].flowRowIdx with
      | some rIdx =>
        simp only [hfi] at hflOk
        by_cases hr : rIdx < cert.baseRows.size
        · simp only [hr, ↓reduceDIte] at hflOk
          exact flowRowMatchesBase_sound (hpLt τ) (hiLt τ) hprevLt hflOk (hfeas.2.2 ⟨rIdx, hr⟩)
        · simp only [hr, ↓reduceDIte] at hflOk
          contradiction
      | none =>
        simp only [hfi] at hflOk
        rcases anyFin_eq_true_iff.mp hflOk with ⟨rIdx, hrOk⟩
        exact flowRowMatchesBase_sound (hpLt τ) (hiLt τ) hprevLt hrOk (hfeas.2.2 rIdx)
    have hprevEq :
        (if _h : τ.val = 0 then I0 else xInv ⟨τ.val - 1, by omega⟩) = optColVal prevInvCol x := by
      dsimp only [prevInvCol]
      by_cases h0 : τ.val = 0
      · cases hI : cert.invInCol <;> simp [h0, I0, xExt, optColVal, hI]
      · simp [h0, xInv, optColVal, hiLt ⟨τ.val - 1, by omega⟩]
    have hyExt : xExt (binColFn τ) = x ⟨cert.nodes[τ.val].binCol, hbLt τ⟩ := by
      simp [xExt, binColFn, hbLt τ]
    refine ⟨hpx, hix, by rwa [hyExt], fun hy0 => ?_, by rwa [hprevEq]⟩
    rw [hyExt] at hy0
    exact vub_zero_forces_prod_zero hpx (hvubRHSLe τ) hvub hy0
  have hcovCond : ∀ τ : Fin cert.nodes.size, inSFn τ = true →
      suffixDemand dFn τ ≤ coeffYFn τ ∨
      ∃ τ' : Fin cert.nodes.size, τ'.val < τ.val ∧ inSFn τ' = true ∧ binColFn τ' = binColFn τ := by
    intro τ hS
    have hcov := (hnodeUnpack τ).2.2.2.1
    dsimp only [inSFn] at hS
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
  /-- Number of structural variables in the problem. -/
  numVars : Nat
  /-- Integrality flags for each variable. -/
  isInt : Array Bool
  /-- Optional lower bounds for each variable (`none` = `-∞`). -/
  lb : Array (Option Rat)
  /-- Optional upper bounds for each variable (`none` = `+∞`). -/
  ub : Array (Option Rat)
  /-- Base rows paired with their aggregation multipliers. -/
  rows : Array (RowSpec × Rat)
  /-- Sparse terms of the materialized aggregated inequality. -/
  aggTerms : Array SparseTerm
  /-- Right-hand side of the materialized aggregated inequality. -/
  aggRHS : Rat
  /-- Positive cMIR divisor `δ`. -/
  delta : Rat
  /-- Per-variable bound-substitution side (`true` = lower bound, `false` = upper bound). -/
  useLower : Array Bool
  /-- Optional upward-weakened scaled right-hand side `β̂`. -/
  betaHat : Option Rat := none
  /-- Optional upward-weakened scaled integer coefficients `α̂`. -/
  alphaHat : Option (Array Rat) := none
  /-- Optional upward-weakened integer cut coefficients. -/
  cInt : Option (Array Rat) := none
  /-- Optional upward-weakened continuous cut coefficients. -/
  cCont : Option (Array Rat) := none
  /-- Positive scaling factor applied to the final cut. -/
  scale : Rat := 1
  /-- Final emitted cMIR cut. -/
  cut : CutSpec
  deriving Repr, Inhabited

/-- Executable verifier for `AggCMIRCert`. -/
def checkAggCMIRCert (cert : AggCMIRCert) : Except String Unit :=
  let n := cert.numVars
  let alphaLenOk : Bool := match cert.alphaHat with | none => true | some arr => arr.size == n
  let cIntLenOk : Bool := match cert.cInt with | none => true | some arr => arr.size == n
  let cContLenOk : Bool := match cert.cCont with | none => true | some arr => arr.size == n
  if !(cert.isInt.size == n && cert.lb.size == n && cert.ub.size == n && cert.useLower.size == n) then
    .error s!"agg-cmir cert metadata arrays must have length {n}"
  else if !(alphaLenOk && cIntLenOk && cContLenOk) then
    .error "agg-cmir cert optional certificate array length mismatch"
  else
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
  let alphaOk := allFin n fun j =>
    if cert.isInt[j.val]! && decide (cvFn j ≠ 0) then
      decide (alphaHatFn j ≤ cmirSignedCoeff cert.useLower[j.val]! (cvFn j) / cert.delta)
    else true
  let cIntFn : Fin n → Rat := fun j =>
    if cvFn j == 0 then 0
    else match cert.cInt with
      | none => mirIntCoeff (alphaHatFn j) f0
      | some arr => arr[j.val]!
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
  guardExc alphaOk "agg-cmir cert alphaHat exceeds exact scaled integer coefficient" <|
  guardExc cIntOk "agg-cmir cert cInt exceeds exact mirIntCoeff" <|
  guardExc cContOk "agg-cmir cert cCont below exact continuous MIR coefficient" <|
  guardExc cutCoeffOk "agg-cmir cert cut coefficients do not match scaled cMIR terms" <|
  guardExc (decide (cert.scale * exactCutRHS ≤ cert.cut.rhs))
    s!"agg-cmir cert cut.rhs={cert.cut.rhs} < scaled exactCutRHS={cert.scale * exactCutRHS}" <|
  .ok ()

private theorem ite_not_error_ok {b : Bool} {msg : String} {k : Except String Unit}
    (h : (if !b then Except.error msg else k) = .ok ()) : b = true ∧ k = .ok () := by
  cases b <;> simp at h ⊢
  exact h

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
  simp only [hIntLen, hlbLen, hubLen, huLen, beq_self_eq_true, Bool.and_self, Bool.not_true, Bool.false_eq_true, ↓reduceIte] at hok
  rcases ite_not_error_ok hok with ⟨_, hok⟩
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
    simp only [Bool.and_eq_true, Bool.or_eq_true, decide_eq_true_eq] at hi
    rcases hi.2 with h0 | hleg
    · exact Or.inl (by simpa [muFn] using h0)
    · exact Or.inr hleg
  have hsat : ∀ i : Fin m, satisfiesSense (finSum n (fun j => AFn i j * x j)) (senseFn i) (rowRHSFn i) := by
    intro i
    have hi := hrowsAll i
    simp only [Bool.and_eq_true] at hi
    have hcols := allTermsInBounds_iff.mp hi.1
    have hrow := hrowsSat i
    unfold rowSatOnFin evalRowOnFin at hrow
    rwa [evalTermsList_eq_finSum cert.rows[i.val].1.terms.toList x hcols] at hrow
  have herr : ∀ j : Fin n, ∃ e,
      aggColErrTerm (cvFn j) (finSum m fun i => muFn i * AFn i j) (lbFn j) (ubFn j) = some e ∧ e ≤ errTermFn j := by
    intro j
    have hj := herrAll j
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
    rw [hcIntEq j, hcContEq j] at hj
    simp only [hisIntB, huLB] at hj
    change (accumTerms n cert.cut.terms).1[j.val]'(by rw [(accumTerms n cert.cut.terms).2]; exact j.isLt) =
      cert.scale * unscaledCut j at hj
    rw [hj, Rat.mul_assoc]
  have hrhsLe : cert.scale * exactCutRHS ≤ cert.cut.rhs := by
    have h := of_decide_eq_true hcutRHSD
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
  /-- Number of structural variables in the problem. -/
  numVars : Nat
  /-- Optional lower bounds for each variable (`none` = `-∞`). -/
  lb : Array (Option Rat)
  /-- Optional upper bounds for each variable (`none` = `+∞`). -/
  ub : Array (Option Rat)
  /-- Unfinalized input cut before coefficient dropping and scaling. -/
  rawCut : CutSpec
  /-- Positive scaling factor applied during finalization. -/
  scale : Rat := 1
  /-- Resulting finalized cut after coefficient dropping and scaling. -/
  finalCut : CutSpec
  deriving Repr, Inhabited

/-- Executable universal verifier for `FinalizedCutCert`. -/
def checkFinalizedCutCert (cert : FinalizedCutCert) : Except String Unit :=
  let n := cert.numVars
  if !(cert.lb.size == n && cert.ub.size == n) then
    .error s!"drop cert bound arrays must have length {n}"
  else
  let rawDense := accumTerms n cert.rawCut.terms
  let finalDense := accumTerms n cert.finalCut.terms
  let dropTerms : Fin n → Option Rat := fun j =>
    let vj := rawDense.1[j.val]'(by rw [rawDense.2]; exact j.isLt)
    let oj := finalDense.1[j.val]'(by rw [finalDense.2]; exact j.isLt)
    dropMinTerm (cert.scale * vj - oj) cert.lb[j.val]! cert.ub[j.val]!
  let dropOk := allFin n fun j => (dropTerms j).isSome
  let loSum : Rat := finSum n fun j => (dropTerms j).getD 0
  let reqRHS := cert.scale * cert.rawCut.rhs - loSum
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
  simp only [hlbLen, hubLen, beq_self_eq_true, Bool.and_self, Bool.not_true, Bool.false_eq_true, ↓reduceIte] at hok
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

/-! ### 4b. Universal Structural GMI Cut Certificate (Production Tableau Derivation + Finalization) -/

private theorem finSum_append {a b : Nat} (f : Fin (a + b) → Rat) :
    finSum (a + b) f =
      finSum a (fun i => f ⟨i.val, by omega⟩) +
      finSum b (fun j => f ⟨a + j.val, by omega⟩) := by
  induction b with
  | zero => simp [Rat.add_zero]
  | succ b ih =>
    change finSum ((a + b) + 1) f = _
    rw [finSum_succ, ih (fun k => f k.castSucc), finSum_succ, Rat.add_assoc]
    rfl

private def slackSignVal : ConstraintSense → Rat
  | .le => 1
  | .ge => -1
  | .eq => 0

/-- Complete structural GMI certificate deriving a cut directly from `baseRows` via rational
tableau row multipliers `rho : Array (Nat × Rat)`, non-basic bound side `colAtUpper`,
optional upward-weakened GMI coefficients (`cStruct`, `cSlack`), and `finalizeSparseCut`
coefficient dropping & scaling. -/
structure StructuralGMICert where
  /-- Number of structural variables in the problem. -/
  numVars : Nat
  /-- Integrality flags for each variable. -/
  isInt : Array Bool
  /-- Optional lower bounds for each variable (`none` = `-∞`). -/
  lb : Array (Option Rat)
  /-- Optional upper bounds for each variable (`none` = `+∞`). -/
  ub : Array (Option Rat)
  /-- Base linear constraints of the MILP instance. -/
  baseRows : Array RowSpec
  /-- Column index of the fractional basic integer variable generating the GMI cut. -/
  basicCol : Nat
  /-- Sparse tableau row multipliers `(rowIndex, multiplier)` combining `baseRows`. -/
  rho : Array (Nat × Rat)
  /-- Per-variable non-basic bound side (`true` = at upper bound, `false` = at lower bound). -/
  colAtUpper : Array Bool
  /-- Optional upward-weakened GMI coefficients for structural variables. -/
  cStruct : Option (Array Rat) := none
  /-- Optional upward-weakened GMI coefficients for slack variables. -/
  cSlack : Option (Array Rat) := none
  /-- Positive scaling factor applied during cut finalization. -/
  scale : Rat := 1
  /-- Final emitted structural GMI cut. -/
  cut : CutSpec
  deriving Repr, Inhabited

private def sgmiAlpha (cert : StructuralGMICert) (j : Fin cert.numVars) : Rat :=
  finSum cert.rho.size fun i =>
    let (rIdx, rVal) := cert.rho[i.val]
    if h : rIdx < cert.baseRows.size then rVal * termsCoeff cert.baseRows[rIdx].terms.toList j.val else 0

private def sgmiBeta0 (cert : StructuralGMICert) : Rat :=
  finSum cert.rho.size fun i =>
    let (rIdx, rVal) := cert.rho[i.val]
    if h : rIdx < cert.baseRows.size then rVal * cert.baseRows[rIdx].rhs else 0

private def sgmiActiveCol (cert : StructuralGMICert) (j : Fin cert.numVars) : Bool :=
  (j.val != cert.basicCol) && decide (sgmiAlpha cert j ≠ 0)

private def sgmiColBnd (cert : StructuralGMICert) (j : Fin cert.numVars) : Option Rat :=
  if !sgmiActiveCol cert j then some 0
  else if cert.colAtUpper[j.val]! then cert.ub[j.val]! else cert.lb[j.val]!

private def sgmiBeta (cert : StructuralGMICert) : Rat :=
  sgmiBeta0 cert - finSum cert.numVars (fun j =>
    if sgmiActiveCol cert j then sgmiAlpha cert j * (sgmiColBnd cert j).getD 0 else 0)

private def sgmiAlphaTilde (cert : StructuralGMICert) (j : Fin cert.numVars) : Rat :=
  if !sgmiActiveCol cert j then 0
  else if cert.colAtUpper[j.val]! then -sgmiAlpha cert j else sgmiAlpha cert j

private def sgmiUseInt (cert : StructuralGMICert) (j : Fin cert.numVars) : Bool :=
  sgmiActiveCol cert j && cert.isInt[j.val]! && ((sgmiColBnd cert j).getD 0).den == 1

private def sgmiExactStruct (cert : StructuralGMICert) (j : Fin cert.numVars) : Rat :=
  let f0 := fracPart (sgmiBeta cert)
  if !sgmiActiveCol cert j then 0
  else if sgmiUseInt cert j then gmiIntCoeff (sgmiAlphaTilde cert j) f0 else gmiContCoeff (sgmiAlphaTilde cert j) f0

private def sgmiCStruct (cert : StructuralGMICert) (j : Fin cert.numVars) : Rat :=
  if !sgmiActiveCol cert j then 0 else optCoeffAt cert.cStruct (sgmiExactStruct cert j) j.val

private def sgmiRowSense (cert : StructuralGMICert) (i : Fin cert.rho.size) : ConstraintSense :=
  let rIdx := cert.rho[i.val].1
  if h : rIdx < cert.baseRows.size then cert.baseRows[rIdx].sense else .eq

private def sgmiExactSlack (cert : StructuralGMICert) (i : Fin cert.rho.size) : Rat :=
  let f0 := fracPart (sgmiBeta cert)
  let rVal := cert.rho[i.val].2
  match sgmiRowSense cert i with
  | .le => gmiContCoeff rVal f0
  | .ge => gmiContCoeff (-rVal) f0
  | .eq => 0

private def sgmiCSlack (cert : StructuralGMICert) (i : Fin cert.rho.size) : Rat :=
  match sgmiRowSense cert i with
  | .eq => 0
  | _ => optCoeffAt cert.cSlack (sgmiExactSlack cert i) i.val

private def sgmiV (cert : StructuralGMICert) (j : Fin cert.numVars) : Rat :=
  (if !sgmiActiveCol cert j then 0 else if cert.colAtUpper[j.val]! then sgmiCStruct cert j else -sgmiCStruct cert j) +
  finSum cert.rho.size fun i =>
    let rIdx := cert.rho[i.val].1
    if h : rIdx < cert.baseRows.size then
      (slackSignVal cert.baseRows[rIdx].sense * sgmiCSlack cert i) * termsCoeff cert.baseRows[rIdx].terms.toList j.val
    else 0

private def sgmiRawRHS (cert : StructuralGMICert) : Rat :=
  -1 +
  finSum cert.numVars (fun j =>
    if !sgmiActiveCol cert j then 0
    else (if cert.colAtUpper[j.val]! then sgmiCStruct cert j else -sgmiCStruct cert j) * (sgmiColBnd cert j).getD 0) +
  finSum cert.rho.size fun i =>
    let rIdx := cert.rho[i.val].1
    if h : rIdx < cert.baseRows.size then
      (slackSignVal cert.baseRows[rIdx].sense * sgmiCSlack cert i) * cert.baseRows[rIdx].rhs
    else 0

/-- Executable universal verifier for `StructuralGMICert`. -/
def checkStructuralGMICert (cert : StructuralGMICert) : Except String Unit :=
  let n := cert.numVars
  let m := cert.rho.size
  if !(cert.isInt.size == n && cert.lb.size == n && cert.ub.size == n && cert.colAtUpper.size == n) then
    .error s!"structural GMI cert metadata arrays must have length {n}"
  else
  let rhoOk := allFin m fun i =>
    let (rIdx, _) := cert.rho[i.val]
    if h : rIdx < cert.baseRows.size then allTermsInBounds n cert.baseRows[rIdx].terms else false
  let basicOk : Bool :=
    if hB : cert.basicCol < n then
      cert.isInt[cert.basicCol]! && decide (sgmiAlpha cert ⟨cert.basicCol, hB⟩ = 1)
    else false
  let colBndOk := allFin n fun j => (sgmiColBnd cert j).isSome
  let f0 : Rat := fracPart (sgmiBeta cert)
  let cStructLenOk : Bool := match cert.cStruct with | none => true | some arr => arr.size == n
  let cStructOk := allFin n fun j =>
    if sgmiActiveCol cert j then decide (sgmiExactStruct cert j ≤ sgmiCStruct cert j) else true
  let cSlackLenOk : Bool := match cert.cSlack with | none => true | some arr => arr.size == m
  let cSlackOk := allFin m fun i =>
    match sgmiRowSense cert i with
    | .eq => true
    | _ => decide (sgmiExactSlack cert i ≤ sgmiCSlack cert i)
  let cutDense := accumTerms n cert.cut.terms
  let dropTerms : Fin n → Option Rat := fun j =>
    dropMinTerm (cert.scale * sgmiV cert j - cutDense.1[j.val]'(by rw [cutDense.2]; exact j.isLt))
      cert.lb[j.val]! cert.ub[j.val]!
  let dropOk := allFin n fun j => (dropTerms j).isSome
  let reqRHS := cert.scale * sgmiRawRHS cert - finSum n (fun j => (dropTerms j).getD 0)
  guardExc (decide (0 < cert.scale)) s!"structural GMI cert scale={cert.scale} must be > 0" <|
  guardExc (allTermsInBounds n cert.cut.terms) "structural GMI cert cut has out-of-bounds column" <|
  guardExc rhoOk "structural GMI cert rho references out-of-bounds row or column" <|
  guardExc basicOk "structural GMI cert basicCol must be an integer variable with tableau coeff 1" <|
  guardExc colBndOk "structural GMI cert active non-basic column missing active bound" <|
  guardExc (decide (0 < f0) && decide (f0 < 1)) s!"structural GMI cert beta={sgmiBeta cert} has non-fractional f0={f0}" <|
  guardExc (cStructLenOk && cSlackLenOk) "structural GMI cert optional coeff array length mismatch" <|
  guardExc cStructOk "structural GMI cert cStruct coefficient below exact GMI coefficient" <|
  guardExc cSlackOk "structural GMI cert cSlack coefficient below exact GMI coefficient" <|
  guardExc dropOk "structural GMI cert finalize/drop perturbed coefficient on unbounded variable side" <|
  guardExc (decide (reqRHS ≤ cert.cut.rhs))
    s!"structural GMI cert cut.rhs={cert.cut.rhs} < required outward RHS {reqRHS}" <|
  .ok ()

set_option maxHeartbeats 600000 in
/-- Universal soundness of `checkStructuralGMICert` over `MIPFeasible`: derives the tableau
equality directly from `baseRows` via `rho`, applies `Track2B.gmi_weaken_upward_sound`, back-substitutes
all non-basic variables and slacks, and applies `finalize_cut_drop_universal_sound`. -/
theorem checkStructuralGMICert_sound (cert : StructuralGMICert)
    (hok : checkStructuralGMICert cert = .ok ())
    (hIntLen : cert.isInt.size = cert.numVars)
    (hlbLen : cert.lb.size = cert.numVars)
    (hubLen : cert.ub.size = cert.numVars)
    (hAtULen : cert.colAtUpper.size = cert.numVars)
    (x : Fin cert.numVars → Rat)
    (hfeas : MIPFeasible cert.numVars
      (fun j => cert.isInt[j.val]'(by omega))
      (fun j => cert.lb[j.val]'(by omega))
      (fun j => cert.ub[j.val]'(by omega))
      cert.baseRows x) :
    evalCutOnFin cert.numVars cert.cut x ≤ cert.cut.rhs := by
  unfold checkStructuralGMICert at hok
  simp only [hIntLen, hlbLen, hubLen, hAtULen, beq_self_eq_true, Bool.and_self, Bool.not_true, Bool.false_eq_true, ↓reduceIte] at hok
  rcases guardExc_ok hok with ⟨hscaleD, hok⟩
  rcases guardExc_ok hok with ⟨hcutColsT, hok⟩
  rcases guardExc_ok hok with ⟨hrhoOkT, hok⟩
  rcases guardExc_ok hok with ⟨hbasicOkT, hok⟩
  rcases guardExc_ok hok with ⟨hcolBndOkT, hok⟩
  rcases guardExc_ok hok with ⟨hf0OkT, hok⟩
  rcases guardExc_ok hok with ⟨_, hok⟩
  rcases guardExc_ok hok with ⟨hcStructOkT, hok⟩
  rcases guardExc_ok hok with ⟨hcSlackOkT, hok⟩
  rcases guardExc_ok hok with ⟨hdropOkT, hok⟩
  rcases guardExc_ok hok with ⟨hrhsD, _⟩
  have hscalePos : 0 < cert.scale := of_decide_eq_true hscaleD
  have hrhoAll := allFin_eq_true_iff.mp hrhoOkT
  have hrIdxLt : ∀ i : Fin cert.rho.size, cert.rho[i.val].1 < cert.baseRows.size := by
    intro i
    have hi := hrhoAll i
    by_cases hlt : cert.rho[i.val].1 < cert.baseRows.size
    · exact hlt
    · simp only [hlt, ↓reduceDIte] at hi
      contradiction
  have hrCols : ∀ i : Fin cert.rho.size,
      allTermsInBounds cert.numVars (cert.baseRows[cert.rho[i.val].1]'(hrIdxLt i)).terms = true := by
    intro i; have hi := hrhoAll i; simp only [hrIdxLt i, ↓reduceDIte] at hi; exact hi
  have hB : cert.basicCol < cert.numVars := by
    by_cases hlt : cert.basicCol < cert.numVars
    · exact hlt
    · simp only [hlt, ↓reduceDIte] at hbasicOkT
      contradiction
  simp only [hB, ↓reduceDIte, Bool.and_eq_true, decide_eq_true_eq] at hbasicOkT
  have hBasicInt : isIntVal (x ⟨cert.basicCol, hB⟩) := by
    apply hfeas.2.1 ⟨cert.basicCol, hB⟩
    have h1 := hbasicOkT.1
    rwa [getElem!_pos cert.isInt cert.basicCol (by omega)] at h1
  have hAlphaB : sgmiAlpha cert ⟨cert.basicCol, hB⟩ = 1 := hbasicOkT.2
  let f0 : Rat := fracPart (sgmiBeta cert)
  simp only [Bool.and_eq_true, decide_eq_true_eq] at hf0OkT
  have hf0Pos : 0 < f0 := hf0OkT.1
  have hf0Lt : f0 < 1 := hf0OkT.2
  have hcolBndAll := allFin_eq_true_iff.mp hcolBndOkT
  have hcStructAll := allFin_eq_true_iff.mp hcStructOkT
  have hcSlackAll := allFin_eq_true_iff.mp hcSlackOkT
  have hdropAll := allFin_eq_true_iff.mp hdropOkT
  let yStruct : Fin cert.numVars → Rat := fun j =>
    if !sgmiActiveCol cert j then 0
    else if cert.colAtUpper[j.val]! then (sgmiColBnd cert j).getD 0 - x j else x j - (sgmiColBnd cert j).getD 0
  have hyStructNN : ∀ j : Fin cert.numVars, 0 ≤ yStruct j := by
    intro j
    dsimp only [yStruct]
    cases ha : sgmiActiveCol cert j
    · simp
    · simp only [Bool.not_true, Bool.false_eq_true, ↓reduceIte]
      have hsome := hcolBndAll j
      unfold sgmiColBnd at hsome ⊢
      simp only [ha, Bool.not_true, Bool.false_eq_true, ↓reduceIte] at hsome ⊢
      cases hu : cert.colAtUpper[j.val]!
      · simp only [hu, Bool.false_eq_true, ↓reduceIte] at hsome ⊢
        rw [getElem!_pos cert.lb j.val (by omega)] at hsome ⊢
        cases hlb : cert.lb[j.val]'(by omega) with
        | none => simp [hlb] at hsome
        | some l =>
          simp only [Option.getD_some]
          exact (Rat.le_iff_sub_nonneg l (x j)).mp ((hfeas.1 j).1 l hlb)
      · simp only [hu, ↓reduceIte] at hsome ⊢
        rw [getElem!_pos cert.ub j.val (by omega)] at hsome ⊢
        cases hub : cert.ub[j.val]'(by omega) with
        | none => simp [hub] at hsome
        | some u =>
          simp only [Option.getD_some]
          exact (Rat.le_iff_sub_nonneg (x j) u).mp ((hfeas.1 j).2 u hub)
  have hyStructInt : ∀ j : Fin cert.numVars, sgmiUseInt cert j = true → isIntVal (yStruct j) := by
    intro j huI
    unfold sgmiUseInt at huI
    simp only [Bool.and_eq_true, beq_iff_eq] at huI
    rcases huI with ⟨⟨ha, hIntJ⟩, hden1⟩
    have hIntX : isIntVal (x j) := hfeas.2.1 j (by rwa [getElem!_pos cert.isInt j.val (by omega)] at hIntJ)
    rcases hIntX with ⟨zx, hzx⟩
    have hBndInt : (sgmiColBnd cert j).getD 0 = (((sgmiColBnd cert j).getD 0).num : Rat) :=
      rat_eq_num_of_den_one hden1
    dsimp only [yStruct]
    simp only [ha, Bool.not_true, Bool.false_eq_true, ↓reduceIte]
    cases cert.colAtUpper[j.val]!
    · refine ⟨zx - ((sgmiColBnd cert j).getD 0).num, ?_⟩
      simp only [Bool.false_eq_true, ↓reduceIte]
      rw [Rat.intCast_sub, ← hzx, ← hBndInt]
    · refine ⟨((sgmiColBnd cert j).getD 0).num - zx, ?_⟩
      simp only [↓reduceIte]
      rw [Rat.intCast_sub, ← hzx, ← hBndInt]
  let rowEvalAt : Fin cert.rho.size → Rat := fun i =>
    evalRowOnFin cert.numVars (cert.baseRows[cert.rho[i.val].1]'(hrIdxLt i)) x
  let rowRHSAt : Fin cert.rho.size → Rat := fun i =>
    (cert.baseRows[cert.rho[i.val].1]'(hrIdxLt i)).rhs
  let ySlack : Fin cert.rho.size → Rat := fun i =>
    slackSignVal (sgmiRowSense cert i) * (rowRHSAt i - rowEvalAt i)
  have hySlackNN : ∀ i : Fin cert.rho.size, 0 ≤ ySlack i := by
    intro i
    have hsat := hfeas.2.2 ⟨cert.rho[i.val].1, hrIdxLt i⟩
    unfold rowSatOnFin at hsat
    dsimp only [ySlack, rowRHSAt, rowEvalAt]
    unfold sgmiRowSense
    simp only [hrIdxLt i, ↓reduceDIte]
    cases hs : (cert.baseRows[cert.rho[i.val].1]'(hrIdxLt i)).sense <;> rw [hs] at hsat
    · simp only [slackSignVal, Rat.one_mul]
      exact (Rat.le_iff_sub_nonneg _ _).mp hsat
    · simp only [slackSignVal, Rat.zero_mul]; exact Rat.le_refl
    · simp only [slackSignVal, Rat.neg_mul, Rat.one_mul, Rat.neg_sub]
      exact (Rat.le_iff_sub_nonneg _ _).mp hsat
  let aI : Fin cert.numVars → Rat := fun j => if sgmiUseInt cert j then sgmiAlphaTilde cert j else 0
  let xI : Fin cert.numVars → Rat := fun j => if sgmiUseInt cert j then yStruct j else 0
  let cI : Fin cert.numVars → Rat := fun j => if sgmiUseInt cert j then sgmiCStruct cert j else 0
  let aC : Fin (cert.numVars + cert.rho.size) → Rat := fun k =>
    if hk : k.val < cert.numVars then
      let j : Fin cert.numVars := ⟨k.val, hk⟩
      if sgmiActiveCol cert j && !sgmiUseInt cert j then sgmiAlphaTilde cert j else 0
    else
      let i : Fin cert.rho.size := ⟨k.val - cert.numVars, by omega⟩
      slackSignVal (sgmiRowSense cert i) * cert.rho[i.val].2
  let xC : Fin (cert.numVars + cert.rho.size) → Rat := fun k =>
    if hk : k.val < cert.numVars then
      let j : Fin cert.numVars := ⟨k.val, hk⟩
      if sgmiActiveCol cert j && !sgmiUseInt cert j then yStruct j else 0
    else
      ySlack ⟨k.val - cert.numVars, by omega⟩
  let cC : Fin (cert.numVars + cert.rho.size) → Rat := fun k =>
    if hk : k.val < cert.numVars then
      let j : Fin cert.numVars := ⟨k.val, hk⟩
      if sgmiActiveCol cert j && !sgmiUseInt cert j then sgmiCStruct cert j else 0
    else
      sgmiCSlack cert ⟨k.val - cert.numVars, by omega⟩
  have hrowEq : x ⟨cert.basicCol, hB⟩ + finSum cert.numVars (fun j => aI j * xI j) +
      finSum (cert.numVars + cert.rho.size) (fun k => aC k * xC k) = sgmiBeta cert := by
    rw [finSum_append]
    have hC1 : finSum cert.numVars (fun j => aC ⟨j.val, by omega⟩ * xC ⟨j.val, by omega⟩) =
        finSum cert.numVars (fun j => if sgmiActiveCol cert j && !sgmiUseInt cert j then sgmiAlphaTilde cert j * yStruct j else 0) := by
      apply finSum_congr; intro j
      dsimp only [aC, xC]
      simp only [j.isLt, ↓reduceDIte]
      cases sgmiActiveCol cert j && !sgmiUseInt cert j <;> simp
    have hC2 : finSum cert.rho.size (fun i => aC ⟨cert.numVars + i.val, by omega⟩ * xC ⟨cert.numVars + i.val, by omega⟩) =
        finSum cert.rho.size (fun i => cert.rho[i.val].2 * (rowRHSAt i - rowEvalAt i)) := by
      apply finSum_congr; intro i
      dsimp only [aC, xC, ySlack]
      have hnot : ¬(cert.numVars + i.val < cert.numVars) := by omega
      have hsub : cert.numVars + i.val - cert.numVars = i.val := by omega
      simp only [hnot, ↓reduceDIte, hsub]
      have hsat := hfeas.2.2 ⟨cert.rho[i.val].1, hrIdxLt i⟩
      unfold rowSatOnFin at hsat
      dsimp only [rowRHSAt, rowEvalAt]
      unfold sgmiRowSense
      simp only [hrIdxLt i, ↓reduceDIte]
      cases hs : (cert.baseRows[cert.rho[i.val].1]'(hrIdxLt i)).sense <;> rw [hs] at hsat
      · simp [slackSignVal, Rat.one_mul]
      · dsimp only [satisfiesSense] at hsat
        simp [slackSignVal, hsat, Rat.sub_self, Rat.mul_zero]
      · simp [slackSignVal, Rat.neg_mul, Rat.mul_neg, Rat.one_mul, Rat.neg_neg]
    have hI_add_C1 :
        finSum cert.numVars (fun j => aI j * xI j) +
        finSum cert.numVars (fun j => if sgmiActiveCol cert j && !sgmiUseInt cert j then sgmiAlphaTilde cert j * yStruct j else 0) =
        finSum cert.numVars (fun j => if sgmiActiveCol cert j then sgmiAlpha cert j * x j - sgmiAlpha cert j * (sgmiColBnd cert j).getD 0 else 0) := by
      rw [← finSum_add]
      apply finSum_congr; intro j
      dsimp only [aI, xI]
      by_cases ha : sgmiActiveCol cert j = true
      · cases huI : sgmiUseInt cert j
        · simp only [ha, Bool.false_eq_true, Bool.not_false, Bool.and_self, ↓reduceIte,
            Rat.zero_mul, Rat.zero_add]
          dsimp only [yStruct]; unfold sgmiAlphaTilde
          simp only [ha, Bool.not_true, Bool.false_eq_true, ↓reduceIte]
          cases cert.colAtUpper[j.val]!
          · simp [rat_mul_sub]
          · simp only [↓reduceIte]
            rw [Rat.neg_mul, rat_mul_sub, Rat.neg_sub]
        · simp only [ha, Bool.not_true, Bool.and_false, Bool.false_eq_true, ↓reduceIte,
            Rat.add_zero]
          dsimp only [yStruct]; unfold sgmiAlphaTilde
          simp only [ha, Bool.not_true, Bool.false_eq_true, ↓reduceIte]
          cases cert.colAtUpper[j.val]!
          · simp [rat_mul_sub]
          · simp only [↓reduceIte]
            rw [Rat.neg_mul, rat_mul_sub, Rat.neg_sub]
      · have haf : sgmiActiveCol cert j = false := Bool.eq_false_iff.mpr ha
        have huIf : sgmiUseInt cert j = false := by simp [sgmiUseInt, haf]
        simp [haf, huIf, Rat.add_zero]
    have hAddB :
        x ⟨cert.basicCol, hB⟩ +
        finSum cert.numVars (fun j => if sgmiActiveCol cert j then sgmiAlpha cert j * x j - sgmiAlpha cert j * (sgmiColBnd cert j).getD 0 else 0) =
        finSum cert.numVars (fun j => sgmiAlpha cert j * x j) -
        finSum cert.numVars (fun j => if sgmiActiveCol cert j then sgmiAlpha cert j * (sgmiColBnd cert j).getD 0 else 0) := by
      have hsingle : x ⟨cert.basicCol, hB⟩ =
          finSum cert.numVars (fun j => if j = ⟨cert.basicCol, hB⟩ then sgmiAlpha cert ⟨cert.basicCol, hB⟩ * x ⟨cert.basicCol, hB⟩ else 0) := by
        rw [finSum_single, hAlphaB, Rat.one_mul]
      rw [hsingle, ← finSum_add, ← finSum_sub]
      apply finSum_congr; intro j
      by_cases hjB : j = ⟨cert.basicCol, hB⟩
      · subst hjB
        have haf : sgmiActiveCol cert ⟨cert.basicCol, hB⟩ = false := by simp [sgmiActiveCol]
        simp [haf, Rat.add_zero, rat_sub_zero]
      · have hne : (j.val != cert.basicCol) = true := by
          simp only [bne_iff_ne]; intro h; exact hjB (Fin.ext h)
        by_cases ha0 : sgmiAlpha cert j = 0
        · have haf : sgmiActiveCol cert j = false := by simp [sgmiActiveCol, hne, ha0]
          simp [hjB, haf, ha0, Rat.zero_mul, Rat.add_zero, rat_sub_zero]
        · have hat : sgmiActiveCol cert j = true := by simp [sgmiActiveCol, hne, ha0]
          simp [hjB, hat, Rat.zero_add]
    have hSlackCancel :
        finSum cert.rho.size (fun i => cert.rho[i.val].2 * (rowRHSAt i - rowEvalAt i)) =
        sgmiBeta0 cert - finSum cert.numVars (fun j => sgmiAlpha cert j * x j) := by
      have hsplit : finSum cert.rho.size (fun i => cert.rho[i.val].2 * (rowRHSAt i - rowEvalAt i)) =
          finSum cert.rho.size (fun i => cert.rho[i.val].2 * rowRHSAt i) -
          finSum cert.rho.size (fun i => cert.rho[i.val].2 * rowEvalAt i) := by
        rw [← finSum_sub]
        exact finSum_congr fun i => rat_mul_sub _ _ _
      rw [hsplit]
      have h1 : finSum cert.rho.size (fun i => cert.rho[i.val].2 * rowRHSAt i) = sgmiBeta0 cert :=
        finSum_congr fun i => by simp [rowRHSAt, hrIdxLt i]
      have h2 : finSum cert.rho.size (fun i => cert.rho[i.val].2 * rowEvalAt i) =
          finSum cert.numVars (fun j => sgmiAlpha cert j * x j) := by
        have hev : ∀ i : Fin cert.rho.size, cert.rho[i.val].2 * rowEvalAt i =
            finSum cert.numVars (fun j => (cert.rho[i.val].2 * termsCoeff (cert.baseRows[cert.rho[i.val].1]'(hrIdxLt i)).terms.toList j.val) * x j) := by
          intro i
          dsimp only [rowEvalAt, evalRowOnFin]
          rw [evalTermsList_eq_finSum _ x (allTermsInBounds_iff.mp (hrCols i)), ← finSum_const_mul]
          exact finSum_congr fun j => (Rat.mul_assoc _ _ _).symm
        rw [finSum_congr hev, finSum_swap]
        apply finSum_congr; intro j
        rw [finSum_mul_const]
        congr 1
        exact finSum_congr fun i => by simp [hrIdxLt i]
      rw [h1, h2]
    rw [hC1, hC2, ← Rat.add_assoc, Rat.add_assoc (x ⟨cert.basicCol, hB⟩), hI_add_C1, hAddB, hSlackCancel]
    unfold sgmiBeta
    simp only [Rat.sub_eq_add_neg]
    have hre :
        (finSum cert.numVars fun j => sgmiAlpha cert j * x j) +
          -(finSum cert.numVars fun j => if sgmiActiveCol cert j then sgmiAlpha cert j * (sgmiColBnd cert j).getD 0 else 0) +
          (sgmiBeta0 cert + -(finSum cert.numVars fun j => sgmiAlpha cert j * x j)) =
        ((finSum cert.numVars fun j => sgmiAlpha cert j * x j) + -(finSum cert.numVars fun j => sgmiAlpha cert j * x j)) +
          (sgmiBeta0 cert + -(finSum cert.numVars fun j => if sgmiActiveCol cert j then sgmiAlpha cert j * (sgmiColBnd cert j).getD 0 else 0)) := by ac_rfl
    rw [hre, Rat.add_neg_cancel, Rat.zero_add]
  have hxI_ok : ∀ j : Fin cert.numVars, 0 ≤ xI j ∧ ∃ z : Int, xI j = (z : Rat) := by
    intro j
    dsimp only [xI]
    cases huI : sgmiUseInt cert j
    · simp only [Bool.false_eq_true, ↓reduceIte]; exact ⟨Rat.le_refl, ⟨0, rfl⟩⟩
    · simp only [↓reduceIte]; exact ⟨hyStructNN j, hyStructInt j huI⟩
  have hxC_ok : ∀ k : Fin (cert.numVars + cert.rho.size), 0 ≤ xC k := by
    intro k
    dsimp only [xC]
    split
    · split
      · exact hyStructNN _
      · exact Rat.le_refl
    · exact hySlackNN _
  have hcI_ok : ∀ j : Fin cert.numVars, gmiIntCoeff (aI j) f0 ≤ cI j := by
    intro j
    dsimp only [aI, cI]
    cases huI : sgmiUseInt cert j
    · simp only [Bool.false_eq_true, ↓reduceIte]
      have h0 : gmiIntCoeff 0 f0 = 0 := by
        unfold gmiIntCoeff fracPart
        have hfl0 : Rat.floor (0 : Rat) = 0 := by decide
        rw [hfl0]
        simp only [Rat.intCast_zero, Rat.sub_self, Rat.le_of_lt hf0Pos, ↓reduceIte, Rat.div_def, Rat.zero_mul]
      rw [h0]; exact Rat.le_refl
    · simp only [↓reduceIte]
      have ha : sgmiActiveCol cert j = true := by
        unfold sgmiUseInt at huI; simp only [Bool.and_eq_true] at huI; exact huI.1.1
      have hj := hcStructAll j
      simp only [ha, ↓reduceIte, decide_eq_true_eq] at hj
      unfold sgmiExactStruct at hj
      simp only [ha, Bool.not_true, Bool.false_eq_true, huI, ↓reduceIte] at hj
      exact hj
  have hcC_ok : ∀ k : Fin (cert.numVars + cert.rho.size), gmiContCoeff (aC k) f0 ≤ cC k := by
    intro k
    dsimp only [aC, cC]
    split
    · next hk =>
      let j : Fin cert.numVars := ⟨k.val, hk⟩
      cases hac : sgmiActiveCol cert j && !sgmiUseInt cert j
      · simp only [Bool.false_eq_true, ↓reduceIte]
        have h0 : gmiContCoeff 0 f0 = 0 := by simp [gmiContCoeff, Rat.div_def, Rat.zero_mul]
        rw [h0]; exact Rat.le_refl
      · simp only [↓reduceIte]
        simp only [Bool.and_eq_true, Bool.not_eq_true'] at hac
        have hj := hcStructAll j
        simp only [hac.1, ↓reduceIte, decide_eq_true_eq] at hj
        unfold sgmiExactStruct at hj
        simp only [hac.1, Bool.not_true, Bool.false_eq_true, hac.2, ↓reduceIte] at hj
        exact hj
    · next hnk =>
      let i : Fin cert.rho.size := ⟨k.val - cert.numVars, by omega⟩
      change gmiContCoeff (slackSignVal (sgmiRowSense cert i) * cert.rho[i.val].2) f0 ≤ sgmiCSlack cert i
      have hi := hcSlackAll i
      cases hs : sgmiRowSense cert i
      · rw [hs] at hi
        have hle : sgmiExactSlack cert i ≤ sgmiCSlack cert i := of_decide_eq_true hi
        simpa [sgmiExactSlack, hs, slackSignVal, Rat.one_mul, f0] using hle
      · simp only [slackSignVal, Rat.zero_mul, sgmiCSlack, hs]
        have h0 : gmiContCoeff 0 f0 = 0 := by simp [gmiContCoeff, Rat.div_def, Rat.zero_mul]
        rw [h0]; exact Rat.le_refl
      · rw [hs] at hi
        have hle : sgmiExactSlack cert i ≤ sgmiCSlack cert i := of_decide_eq_true hi
        simpa [sgmiExactSlack, hs, slackSignVal, Rat.neg_mul, Rat.one_mul, f0] using hle
  have hgmi := gmi_weaken_upward_sound hrowEq hBasicInt hxI_ok hxC_ok rfl hf0Pos hf0Lt hcI_ok hcC_ok
  rw [finSum_append] at hgmi
  have hrawCut : finSum cert.numVars (fun j => sgmiV cert j * x j) ≤ sgmiRawRHS cert := by
    have hStructSum :
        finSum cert.numVars (fun j => cI j * xI j) +
        finSum cert.numVars (fun j => cC ⟨j.val, by omega⟩ * xC ⟨j.val, by omega⟩) =
        finSum cert.numVars (fun j =>
          if !sgmiActiveCol cert j then 0
          else (if cert.colAtUpper[j.val]! then sgmiCStruct cert j else -sgmiCStruct cert j) * (sgmiColBnd cert j).getD 0 -
               (if cert.colAtUpper[j.val]! then sgmiCStruct cert j else -sgmiCStruct cert j) * x j) := by
      rw [← finSum_add]
      apply finSum_congr; intro j
      dsimp only [cI, xI, cC, xC]
      simp only [j.isLt, ↓reduceDIte]
      by_cases ha : sgmiActiveCol cert j = true
      · cases huI : sgmiUseInt cert j
        · simp only [ha, Bool.false_eq_true, Bool.not_false, Bool.not_true, Bool.and_self,
            ↓reduceIte, Rat.zero_mul, Rat.zero_add]
          dsimp only [yStruct]
          simp only [ha, Bool.not_true, Bool.false_eq_true, ↓reduceIte]
          cases cert.colAtUpper[j.val]!
          · simp only [Bool.false_eq_true, ↓reduceIte]
            rw [rat_mul_sub, Rat.neg_mul, Rat.neg_mul, Rat.sub_eq_add_neg, Rat.sub_eq_add_neg, Rat.neg_neg, Rat.add_comm]
          · simp only [↓reduceIte]
            rw [rat_mul_sub]
        · simp only [ha, Bool.not_true, Bool.and_false, Bool.false_eq_true, ↓reduceIte,
            Rat.zero_mul, Rat.add_zero]
          dsimp only [yStruct]
          simp only [ha, Bool.not_true, Bool.false_eq_true, ↓reduceIte]
          cases cert.colAtUpper[j.val]!
          · simp only [Bool.false_eq_true, ↓reduceIte]
            rw [rat_mul_sub, Rat.neg_mul, Rat.neg_mul, Rat.sub_eq_add_neg, Rat.sub_eq_add_neg, Rat.neg_neg, Rat.add_comm]
          · simp only [↓reduceIte]
            rw [rat_mul_sub]
      · have haf : sgmiActiveCol cert j = false := Bool.eq_false_iff.mpr ha
        have huIf : sgmiUseInt cert j = false := by simp [sgmiUseInt, haf]
        simp [haf, huIf, Rat.add_zero]
    have hSlackSum :
        finSum cert.rho.size (fun i => cC ⟨cert.numVars + i.val, by omega⟩ * xC ⟨cert.numVars + i.val, by omega⟩) =
        finSum cert.rho.size (fun i =>
          if h : cert.rho[i.val].1 < cert.baseRows.size then
            (slackSignVal cert.baseRows[cert.rho[i.val].1].sense * sgmiCSlack cert i) * cert.baseRows[cert.rho[i.val].1].rhs
          else 0) -
        finSum cert.numVars (fun j =>
          (finSum cert.rho.size fun i =>
            if h : cert.rho[i.val].1 < cert.baseRows.size then
              (slackSignVal cert.baseRows[cert.rho[i.val].1].sense * sgmiCSlack cert i) * termsCoeff cert.baseRows[cert.rho[i.val].1].terms.toList j.val
            else 0) * x j) := by
      have hstep1 : finSum cert.rho.size (fun i => cC ⟨cert.numVars + i.val, by omega⟩ * xC ⟨cert.numVars + i.val, by omega⟩) =
          finSum cert.rho.size (fun i => (slackSignVal (sgmiRowSense cert i) * sgmiCSlack cert i) * rowRHSAt i) -
          finSum cert.rho.size (fun i => (slackSignVal (sgmiRowSense cert i) * sgmiCSlack cert i) * rowEvalAt i) := by
        rw [← finSum_sub]
        apply finSum_congr; intro i
        dsimp only [cC, xC, ySlack]
        have hnot : ¬(cert.numVars + i.val < cert.numVars) := by omega
        have hsub : cert.numVars + i.val - cert.numVars = i.val := by omega
        simp only [hnot, ↓reduceDIte, hsub]
        rw [← Rat.mul_assoc, Rat.mul_comm (sgmiCSlack cert i) (slackSignVal (sgmiRowSense cert i)), rat_mul_sub]
      rw [hstep1]
      congr 1
      · exact finSum_congr fun i => by simp [sgmiRowSense, rowRHSAt, hrIdxLt i]
      · have hev : ∀ i : Fin cert.rho.size, (slackSignVal (sgmiRowSense cert i) * sgmiCSlack cert i) * rowEvalAt i =
            finSum cert.numVars (fun j => ((if h : cert.rho[i.val].1 < cert.baseRows.size then
              (slackSignVal cert.baseRows[cert.rho[i.val].1].sense * sgmiCSlack cert i) * termsCoeff cert.baseRows[cert.rho[i.val].1].terms.toList j.val
            else 0)) * x j) := by
          intro i
          dsimp only [rowEvalAt, evalRowOnFin]
          rw [evalTermsList_eq_finSum _ x (allTermsInBounds_iff.mp (hrCols i)), ← finSum_const_mul]
          apply finSum_congr; intro j
          simp only [sgmiRowSense, hrIdxLt i, ↓reduceDIte, Rat.mul_assoc]
        rw [finSum_congr hev, finSum_swap]
        exact finSum_congr fun j => by rw [finSum_mul_const]
    rw [← Rat.add_assoc, hStructSum, hSlackSum] at hgmi
    have hStructSplit :
        finSum cert.numVars (fun j =>
          if !sgmiActiveCol cert j then 0
          else (if cert.colAtUpper[j.val]! then sgmiCStruct cert j else -sgmiCStruct cert j) * (sgmiColBnd cert j).getD 0 -
               (if cert.colAtUpper[j.val]! then sgmiCStruct cert j else -sgmiCStruct cert j) * x j) =
        finSum cert.numVars (fun j =>
          if !sgmiActiveCol cert j then 0
          else (if cert.colAtUpper[j.val]! then sgmiCStruct cert j else -sgmiCStruct cert j) * (sgmiColBnd cert j).getD 0) -
        finSum cert.numVars (fun j =>
          (if !sgmiActiveCol cert j then 0 else if cert.colAtUpper[j.val]! then sgmiCStruct cert j else -sgmiCStruct cert j) * x j) := by
      rw [← finSum_sub]
      apply finSum_congr; intro j
      cases sgmiActiveCol cert j <;> simp [Rat.zero_mul, rat_sub_zero]
    rw [hStructSplit] at hgmi
    have hvSplit : finSum cert.numVars (fun j => sgmiV cert j * x j) =
        finSum cert.numVars (fun j => (if !sgmiActiveCol cert j then 0 else if cert.colAtUpper[j.val]! then sgmiCStruct cert j else -sgmiCStruct cert j) * x j) +
        finSum cert.numVars (fun j =>
          (finSum cert.rho.size fun i =>
            if h : cert.rho[i.val].1 < cert.baseRows.size then
              (slackSignVal cert.baseRows[cert.rho[i.val].1].sense * sgmiCSlack cert i) * termsCoeff cert.baseRows[cert.rho[i.val].1].terms.toList j.val
            else 0) * x j) := by
      unfold sgmiV
      rw [← finSum_add]
      exact finSum_congr fun j => Rat.add_mul _ _ _
    rw [hvSplit]
    unfold sgmiRawRHS
    let X1 := finSum cert.numVars (fun j => (if !sgmiActiveCol cert j then 0 else if cert.colAtUpper[j.val]! then sgmiCStruct cert j else -sgmiCStruct cert j) * x j)
    let X2 := finSum cert.numVars (fun j => (finSum cert.rho.size fun i => if h : cert.rho[i.val].1 < cert.baseRows.size then (slackSignVal cert.baseRows[cert.rho[i.val].1].sense * sgmiCSlack cert i) * termsCoeff cert.baseRows[cert.rho[i.val].1].terms.toList j.val else 0) * x j)
    let A := finSum cert.numVars (fun j => if !sgmiActiveCol cert j then 0 else (if cert.colAtUpper[j.val]! then sgmiCStruct cert j else -sgmiCStruct cert j) * (sgmiColBnd cert j).getD 0)
    let B := finSum cert.rho.size (fun i => if h : cert.rho[i.val].1 < cert.baseRows.size then (slackSignVal cert.baseRows[cert.rho[i.val].1].sense * sgmiCSlack cert i) * cert.baseRows[cert.rho[i.val].1].rhs else 0)
    have hmove : (1 : Rat) + ((X1 + X2) + (-1 : Rat)) ≤ (A - X1 + (B - X2)) + ((X1 + X2) + (-1 : Rat)) :=
      (Rat.add_le_add_right (c := (X1 + X2) + (-1 : Rat))).mpr hgmi
    have hL : (1 : Rat) + ((X1 + X2) + (-1 : Rat)) = X1 + X2 := by
      have : (1 : Rat) + ((X1 + X2) + (-1 : Rat)) = ((1 : Rat) + (-1 : Rat)) + (X1 + X2) := by ac_rfl
      rw [this, Rat.add_neg_cancel, Rat.zero_add]
    have hR : (A - X1 + (B - X2)) + ((X1 + X2) + (-1 : Rat)) = -1 + A + B := by
      simp only [Rat.sub_eq_add_neg]
      have : A + -X1 + (B + -X2) + (X1 + X2 + -1) = (-1 + A + B) + (-X1 + X1) + (-X2 + X2) := by ac_rfl
      rw [this, Rat.neg_add_cancel, Rat.neg_add_cancel, Rat.add_zero, Rat.add_zero]
    rwa [hL, hR] at hmove
  let lbFn : Fin cert.numVars → Option Rat := fun j => cert.lb[j.val]'(by omega)
  let ubFn : Fin cert.numVars → Option Rat := fun j => cert.ub[j.val]'(by omega)
  have hlbB : ∀ j : Fin cert.numVars, cert.lb[j.val]! = lbFn j := fun j => getElem!_pos cert.lb j.val (by omega)
  have hubB : ∀ j : Fin cert.numVars, cert.ub[j.val]! = ubFn j := fun j => getElem!_pos cert.ub j.val (by omega)
  let cutDense := accumTerms cert.numVars cert.cut.terms
  let outFn : Fin cert.numVars → Rat := fun j => cutDense.1[j.val]'(by rw [cutDense.2]; exact j.isLt)
  let loFn : Fin cert.numVars → Rat := fun j => (dropMinTerm (cert.scale * sgmiV cert j - outFn j) (lbFn j) (ubFn j)).getD 0
  have hlo : ∀ j : Fin cert.numVars, dropMinTerm (cert.scale * sgmiV cert j - outFn j) (lbFn j) (ubFn j) = some (loFn j) := by
    intro j
    have hj := hdropAll j
    rw [hlbB j, hubB j] at hj
    dsimp only [loFn]
    cases hd : dropMinTerm (cert.scale * sgmiV cert j - outFn j) (lbFn j) (ubFn j) with
    | none => rw [hd] at hj; simp at hj
    | some lo => simp
  have hrhsLe : cert.scale * sgmiRawRHS cert - finSum cert.numVars loFn ≤ cert.cut.rhs := by
    have h := of_decide_eq_true hrhsD
    have hcong : finSum cert.numVars (fun j => (dropMinTerm (cert.scale * sgmiV cert j - outFn j) cert.lb[j.val]! cert.ub[j.val]!).getD 0) =
        finSum cert.numVars loFn :=
      finSum_congr fun j => by dsimp only [loFn]; rw [hlbB j, hubB j]
    rwa [← hcong]
  rw [evalCutOnFin_eq_finSum_accum cert.cut x hcutColsT]
  exact finalize_cut_drop_universal_sound hscalePos hrawCut hfeas.1 hlo hrhsLe

private def sgmiBuildActiveArr (cert : StructuralGMICert)
    (alphaDense : { a : Array Rat // a.size = cert.numVars }) :
    { a : Array Bool // a.size = cert.numVars } :=
  ⟨Array.ofFn (fun (j : Fin cert.numVars) =>
    (j.val != cert.basicCol) &&
      decide (alphaDense.1[j.val]'(by rw [alphaDense.2]; exact j.isLt) ≠ 0)),
   Array.size_ofFn⟩

private theorem sgmiBuildActiveArr_getElem (cert : StructuralGMICert)
    (alphaDense : { a : Array Rat // a.size = cert.numVars })
    (halpha : ∀ j : Fin cert.numVars,
      alphaDense.1[j.val]'(by rw [alphaDense.2]; exact j.isLt) = sgmiAlpha cert j)
    (j : Fin cert.numVars) :
    (sgmiBuildActiveArr cert alphaDense).1[j.val]'(by
      rw [(sgmiBuildActiveArr cert alphaDense).2]; exact j.isLt) = sgmiActiveCol cert j := by
  unfold sgmiBuildActiveArr sgmiActiveCol
  simp only [Array.getElem_ofFn, halpha j]

private def sgmiBuildColBndArr (cert : StructuralGMICert)
    (activeArr : { a : Array Bool // a.size = cert.numVars }) :
    { a : Array (Option Rat) // a.size = cert.numVars } :=
  ⟨Array.ofFn (fun (j : Fin cert.numVars) =>
    if !(activeArr.1[j.val]'(by rw [activeArr.2]; exact j.isLt)) then some 0
    else if cert.colAtUpper[j.val]! then cert.ub[j.val]! else cert.lb[j.val]!),
   Array.size_ofFn⟩

private theorem sgmiBuildColBndArr_getElem (cert : StructuralGMICert)
    (activeArr : { a : Array Bool // a.size = cert.numVars })
    (hactive : ∀ j : Fin cert.numVars,
      activeArr.1[j.val]'(by rw [activeArr.2]; exact j.isLt) = sgmiActiveCol cert j)
    (j : Fin cert.numVars) :
    (sgmiBuildColBndArr cert activeArr).1[j.val]'(by
      rw [(sgmiBuildColBndArr cert activeArr).2]; exact j.isLt) = sgmiColBnd cert j := by
  unfold sgmiBuildColBndArr sgmiColBnd
  simp only [Array.getElem_ofFn, hactive j]

private def sgmiFastBeta (cert : StructuralGMICert)
    (alphaDense : { a : Array Rat // a.size = cert.numVars })
    (activeArr : { a : Array Bool // a.size = cert.numVars })
    (colBndArr : { a : Array (Option Rat) // a.size = cert.numVars }) : Rat :=
  sgmiBeta0 cert - finSum cert.numVars (fun j =>
    if activeArr.1[j.val]'(by rw [activeArr.2]; exact j.isLt) then
      alphaDense.1[j.val]'(by rw [alphaDense.2]; exact j.isLt) *
        (colBndArr.1[j.val]'(by rw [colBndArr.2]; exact j.isLt)).getD 0
    else 0)

private theorem sgmiFastBeta_eq (cert : StructuralGMICert)
    (alphaDense : { a : Array Rat // a.size = cert.numVars })
    (activeArr : { a : Array Bool // a.size = cert.numVars })
    (colBndArr : { a : Array (Option Rat) // a.size = cert.numVars })
    (halpha : ∀ j : Fin cert.numVars,
      alphaDense.1[j.val]'(by rw [alphaDense.2]; exact j.isLt) = sgmiAlpha cert j)
    (hactive : ∀ j : Fin cert.numVars,
      activeArr.1[j.val]'(by rw [activeArr.2]; exact j.isLt) = sgmiActiveCol cert j)
    (hcolBnd : ∀ j : Fin cert.numVars,
      colBndArr.1[j.val]'(by rw [colBndArr.2]; exact j.isLt) = sgmiColBnd cert j) :
    sgmiFastBeta cert alphaDense activeArr colBndArr = sgmiBeta cert := by
  unfold sgmiFastBeta sgmiBeta
  congr 1
  exact finSum_congr fun j => by rw [hactive j, halpha j, hcolBnd j]

private def sgmiBuildExactStructArr (cert : StructuralGMICert)
    (alphaDense : { a : Array Rat // a.size = cert.numVars })
    (activeArr : { a : Array Bool // a.size = cert.numVars })
    (colBndArr : { a : Array (Option Rat) // a.size = cert.numVars })
    (f0 : Rat) : { a : Array Rat // a.size = cert.numVars } :=
  ⟨Array.ofFn (fun (j : Fin cert.numVars) =>
    if !(activeArr.1[j.val]'(by rw [activeArr.2]; exact j.isLt)) then 0
    else
      let a := alphaDense.1[j.val]'(by rw [alphaDense.2]; exact j.isLt)
      let aTilde := if cert.colAtUpper[j.val]! then -a else a
      let useInt := cert.isInt[j.val]! &&
        ((colBndArr.1[j.val]'(by rw [colBndArr.2]; exact j.isLt)).getD 0).den == 1
      if useInt then gmiIntCoeff aTilde f0 else gmiContCoeff aTilde f0),
   Array.size_ofFn⟩

private theorem sgmiBuildExactStructArr_getElem (cert : StructuralGMICert)
    (alphaDense : { a : Array Rat // a.size = cert.numVars })
    (activeArr : { a : Array Bool // a.size = cert.numVars })
    (colBndArr : { a : Array (Option Rat) // a.size = cert.numVars })
    (f0 : Rat)
    (halpha : ∀ j : Fin cert.numVars,
      alphaDense.1[j.val]'(by rw [alphaDense.2]; exact j.isLt) = sgmiAlpha cert j)
    (hactive : ∀ j : Fin cert.numVars,
      activeArr.1[j.val]'(by rw [activeArr.2]; exact j.isLt) = sgmiActiveCol cert j)
    (hcolBnd : ∀ j : Fin cert.numVars,
      colBndArr.1[j.val]'(by rw [colBndArr.2]; exact j.isLt) = sgmiColBnd cert j)
    (hf0 : f0 = fracPart (sgmiBeta cert))
    (j : Fin cert.numVars) :
    (sgmiBuildExactStructArr cert alphaDense activeArr colBndArr f0).1[j.val]'(by
      rw [(sgmiBuildExactStructArr cert alphaDense activeArr colBndArr f0).2]; exact j.isLt) =
      sgmiExactStruct cert j := by
  unfold sgmiBuildExactStructArr sgmiExactStruct sgmiAlphaTilde sgmiUseInt
  simp only [Array.getElem_ofFn, hactive j, halpha j, hcolBnd j, hf0]
  cases sgmiActiveCol cert j <;> simp

private def sgmiBuildCStructArr (cert : StructuralGMICert)
    (activeArr : { a : Array Bool // a.size = cert.numVars })
    (exactStructArr : { a : Array Rat // a.size = cert.numVars }) :
    { a : Array Rat // a.size = cert.numVars } :=
  ⟨Array.ofFn (fun (j : Fin cert.numVars) =>
    if !(activeArr.1[j.val]'(by rw [activeArr.2]; exact j.isLt)) then 0
    else optCoeffAt cert.cStruct
      (exactStructArr.1[j.val]'(by rw [exactStructArr.2]; exact j.isLt)) j.val),
   Array.size_ofFn⟩

private theorem sgmiBuildCStructArr_getElem (cert : StructuralGMICert)
    (activeArr : { a : Array Bool // a.size = cert.numVars })
    (exactStructArr : { a : Array Rat // a.size = cert.numVars })
    (hactive : ∀ j : Fin cert.numVars,
      activeArr.1[j.val]'(by rw [activeArr.2]; exact j.isLt) = sgmiActiveCol cert j)
    (hexact : ∀ j : Fin cert.numVars,
      exactStructArr.1[j.val]'(by rw [exactStructArr.2]; exact j.isLt) = sgmiExactStruct cert j)
    (j : Fin cert.numVars) :
    (sgmiBuildCStructArr cert activeArr exactStructArr).1[j.val]'(by
      rw [(sgmiBuildCStructArr cert activeArr exactStructArr).2]; exact j.isLt) =
      sgmiCStruct cert j := by
  unfold sgmiBuildCStructArr sgmiCStruct
  simp only [Array.getElem_ofFn, hactive j, hexact j]

private def sgmiBuildExactSlackArr (cert : StructuralGMICert) (f0 : Rat) :
    { a : Array Rat // a.size = cert.rho.size } :=
  ⟨Array.ofFn (fun (i : Fin cert.rho.size) =>
    let rVal := cert.rho[i.val].2
    match sgmiRowSense cert i with
    | .le => gmiContCoeff rVal f0
    | .ge => gmiContCoeff (-rVal) f0
    | .eq => 0),
   Array.size_ofFn⟩

private theorem sgmiBuildExactSlackArr_getElem (cert : StructuralGMICert) (f0 : Rat)
    (hf0 : f0 = fracPart (sgmiBeta cert)) (i : Fin cert.rho.size) :
    (sgmiBuildExactSlackArr cert f0).1[i.val]'(by
      rw [(sgmiBuildExactSlackArr cert f0).2]; exact i.isLt) = sgmiExactSlack cert i := by
  unfold sgmiBuildExactSlackArr sgmiExactSlack
  simp only [Array.getElem_ofFn, hf0]

private def sgmiBuildCSlackArr (cert : StructuralGMICert)
    (exactSlackArr : { a : Array Rat // a.size = cert.rho.size }) :
    { a : Array Rat // a.size = cert.rho.size } :=
  ⟨Array.ofFn (fun (i : Fin cert.rho.size) =>
    match sgmiRowSense cert i with
    | .eq => 0
    | _ => optCoeffAt cert.cSlack
      (exactSlackArr.1[i.val]'(by rw [exactSlackArr.2]; exact i.isLt)) i.val),
   Array.size_ofFn⟩

private theorem sgmiBuildCSlackArr_getElem (cert : StructuralGMICert)
    (exactSlackArr : { a : Array Rat // a.size = cert.rho.size })
    (hexact : ∀ i : Fin cert.rho.size,
      exactSlackArr.1[i.val]'(by rw [exactSlackArr.2]; exact i.isLt) = sgmiExactSlack cert i)
    (i : Fin cert.rho.size) :
    (sgmiBuildCSlackArr cert exactSlackArr).1[i.val]'(by
      rw [(sgmiBuildCSlackArr cert exactSlackArr).2]; exact i.isLt) = sgmiCSlack cert i := by
  unfold sgmiBuildCSlackArr sgmiCSlack
  simp only [Array.getElem_ofFn, hexact i]

private def sgmiBuildSlackPairs (cert : StructuralGMICert)
    (cSlackArr : { a : Array Rat // a.size = cert.rho.size }) :
    { a : Array (Nat × Rat) // a.size = cert.rho.size } :=
  ⟨Array.ofFn (fun (i : Fin cert.rho.size) =>
    let rIdx := cert.rho[i.val].1
    let c := cSlackArr.1[i.val]'(by rw [cSlackArr.2]; exact i.isLt)
    let w :=
      if h : rIdx < cert.baseRows.size then slackSignVal cert.baseRows[rIdx].sense * c else 0
    (rIdx, w)),
   Array.size_ofFn⟩

private def sgmiBuildVArr (cert : StructuralGMICert)
    (activeArr : { a : Array Bool // a.size = cert.numVars })
    (cStructArr : { a : Array Rat // a.size = cert.numVars })
    (vRowsDense : { a : Array Rat // a.size = cert.numVars }) :
    { a : Array Rat // a.size = cert.numVars } :=
  ⟨Array.ofFn (fun (j : Fin cert.numVars) =>
    (if !(activeArr.1[j.val]'(by rw [activeArr.2]; exact j.isLt)) then 0
     else
       let c := cStructArr.1[j.val]'(by rw [cStructArr.2]; exact j.isLt)
       if cert.colAtUpper[j.val]! then c else -c) +
    vRowsDense.1[j.val]'(by rw [vRowsDense.2]; exact j.isLt)),
   Array.size_ofFn⟩

private theorem sgmiBuildVArr_getElem (cert : StructuralGMICert)
    (activeArr : { a : Array Bool // a.size = cert.numVars })
    (cStructArr : { a : Array Rat // a.size = cert.numVars })
    (cSlackArr : { a : Array Rat // a.size = cert.rho.size })
    (hactive : ∀ j : Fin cert.numVars,
      activeArr.1[j.val]'(by rw [activeArr.2]; exact j.isLt) = sgmiActiveCol cert j)
    (hcStruct : ∀ j : Fin cert.numVars,
      cStructArr.1[j.val]'(by rw [cStructArr.2]; exact j.isLt) = sgmiCStruct cert j)
    (hcSlack : ∀ i : Fin cert.rho.size,
      cSlackArr.1[i.val]'(by rw [cSlackArr.2]; exact i.isLt) = sgmiCSlack cert i)
    (j : Fin cert.numVars) :
    let sp := sgmiBuildSlackPairs cert cSlackArr
    let vRows := accumWeightedRowsN cert.numVars cert.rho.size cert.baseRows sp.1 sp.2
    (sgmiBuildVArr cert activeArr cStructArr vRows).1[j.val]'(by
      rw [(sgmiBuildVArr cert activeArr cStructArr vRows).2]; exact j.isLt) =
      sgmiV cert j := by
  dsimp only
  unfold sgmiBuildVArr sgmiV
  simp only [Array.getElem_ofFn, hactive j, hcStruct j]
  congr 1
  rw [accumWeightedRowsN_getElem]
  apply finSum_congr
  intro i
  unfold sgmiBuildSlackPairs
  simp only [Array.getElem_ofFn, hcSlack i]
  split <;> rfl

private def sgmiFastRawRHS (cert : StructuralGMICert)
    (activeArr : { a : Array Bool // a.size = cert.numVars })
    (cStructArr : { a : Array Rat // a.size = cert.numVars })
    (colBndArr : { a : Array (Option Rat) // a.size = cert.numVars })
    (cSlackArr : { a : Array Rat // a.size = cert.rho.size }) : Rat :=
  -1 +
  finSum cert.numVars (fun j =>
    if !(activeArr.1[j.val]'(by rw [activeArr.2]; exact j.isLt)) then 0
    else
      let c := cStructArr.1[j.val]'(by rw [cStructArr.2]; exact j.isLt)
      (if cert.colAtUpper[j.val]! then c else -c) *
        (colBndArr.1[j.val]'(by rw [colBndArr.2]; exact j.isLt)).getD 0) +
  finSum cert.rho.size fun i =>
    let rIdx := cert.rho[i.val].1
    if h : rIdx < cert.baseRows.size then
      (slackSignVal cert.baseRows[rIdx].sense *
        (cSlackArr.1[i.val]'(by rw [cSlackArr.2]; exact i.isLt))) * cert.baseRows[rIdx].rhs
    else 0

private theorem sgmiFastRawRHS_eq (cert : StructuralGMICert)
    (activeArr : { a : Array Bool // a.size = cert.numVars })
    (cStructArr : { a : Array Rat // a.size = cert.numVars })
    (colBndArr : { a : Array (Option Rat) // a.size = cert.numVars })
    (cSlackArr : { a : Array Rat // a.size = cert.rho.size })
    (hactive : ∀ j : Fin cert.numVars,
      activeArr.1[j.val]'(by rw [activeArr.2]; exact j.isLt) = sgmiActiveCol cert j)
    (hcStruct : ∀ j : Fin cert.numVars,
      cStructArr.1[j.val]'(by rw [cStructArr.2]; exact j.isLt) = sgmiCStruct cert j)
    (hcolBnd : ∀ j : Fin cert.numVars,
      colBndArr.1[j.val]'(by rw [colBndArr.2]; exact j.isLt) = sgmiColBnd cert j)
    (hcSlack : ∀ i : Fin cert.rho.size,
      cSlackArr.1[i.val]'(by rw [cSlackArr.2]; exact i.isLt) = sgmiCSlack cert i) :
    sgmiFastRawRHS cert activeArr cStructArr colBndArr cSlackArr = sgmiRawRHS cert := by
  unfold sgmiFastRawRHS sgmiRawRHS
  have h1 :
      finSum cert.numVars (fun j =>
        if !(activeArr.1[j.val]'(by rw [activeArr.2]; exact j.isLt)) then 0
        else
          let c := cStructArr.1[j.val]'(by rw [cStructArr.2]; exact j.isLt)
          (if cert.colAtUpper[j.val]! then c else -c) *
            (colBndArr.1[j.val]'(by rw [colBndArr.2]; exact j.isLt)).getD 0) =
      finSum cert.numVars (fun j =>
        if !sgmiActiveCol cert j then 0
        else (if cert.colAtUpper[j.val]! then sgmiCStruct cert j else -sgmiCStruct cert j) *
          (sgmiColBnd cert j).getD 0) :=
    finSum_congr fun j => by simp only [hactive j, hcStruct j, hcolBnd j]
  have h2 :
      finSum cert.rho.size (fun i =>
        let rIdx := cert.rho[i.val].1
        if h : rIdx < cert.baseRows.size then
          (slackSignVal cert.baseRows[rIdx].sense *
            (cSlackArr.1[i.val]'(by rw [cSlackArr.2]; exact i.isLt))) * cert.baseRows[rIdx].rhs
        else 0) =
      finSum cert.rho.size (fun i =>
        let rIdx := cert.rho[i.val].1
        if h : rIdx < cert.baseRows.size then
          (slackSignVal cert.baseRows[rIdx].sense * sgmiCSlack cert i) * cert.baseRows[rIdx].rhs
        else 0) :=
    finSum_congr fun i => by simp only [hcSlack i]
  rw [h1, h2]

/-- Linear-time `O(n + |rho| * avgRowNnz)` fast verifier equivalent to `checkStructuralGMICert`. -/
def checkStructuralGMICertFast (cert : StructuralGMICert) : Except String Unit :=
  let n := cert.numVars
  let m := cert.rho.size
  if !(cert.isInt.size == n && cert.lb.size == n && cert.ub.size == n && cert.colAtUpper.size == n) then
    .error s!"structural GMI cert metadata arrays must have length {n}"
  else
  let rhoOk := allFin m fun i =>
    let (rIdx, _) := cert.rho[i.val]
    if h : rIdx < cert.baseRows.size then allTermsInBounds n cert.baseRows[rIdx].terms else false
  let alphaDense := accumWeightedRowsN n m cert.baseRows cert.rho rfl
  let basicOk : Bool :=
    if hB : cert.basicCol < n then
      cert.isInt[cert.basicCol]! &&
        decide (alphaDense.1[cert.basicCol]'(by rw [alphaDense.2]; exact hB) = 1)
    else false
  let activeArr := sgmiBuildActiveArr cert alphaDense
  let colBndArr := sgmiBuildColBndArr cert activeArr
  let colBndOk := allFin n fun j =>
    (colBndArr.1[j.val]'(by rw [colBndArr.2]; exact j.isLt)).isSome
  let betaVal := sgmiFastBeta cert alphaDense activeArr colBndArr
  let f0 := fracPart betaVal
  let cStructLenOk : Bool := match cert.cStruct with | none => true | some arr => arr.size == n
  let exactStructArr := sgmiBuildExactStructArr cert alphaDense activeArr colBndArr f0
  let cStructArr := sgmiBuildCStructArr cert activeArr exactStructArr
  let cStructOk := allFin n fun j =>
    if activeArr.1[j.val]'(by rw [activeArr.2]; exact j.isLt) then
      decide (exactStructArr.1[j.val]'(by rw [exactStructArr.2]; exact j.isLt) ≤
        cStructArr.1[j.val]'(by rw [cStructArr.2]; exact j.isLt))
    else true
  let cSlackLenOk : Bool := match cert.cSlack with | none => true | some arr => arr.size == m
  let exactSlackArr := sgmiBuildExactSlackArr cert f0
  let cSlackArr := sgmiBuildCSlackArr cert exactSlackArr
  let cSlackOk := allFin m fun i =>
    match sgmiRowSense cert i with
    | .eq => true
    | _ => decide (exactSlackArr.1[i.val]'(by rw [exactSlackArr.2]; exact i.isLt) ≤
      cSlackArr.1[i.val]'(by rw [cSlackArr.2]; exact i.isLt))
  let slackPairs := sgmiBuildSlackPairs cert cSlackArr
  let vRowsDense := accumWeightedRowsN n m cert.baseRows slackPairs.1 slackPairs.2
  let vArr := sgmiBuildVArr cert activeArr cStructArr vRowsDense
  let rawRHS := sgmiFastRawRHS cert activeArr cStructArr colBndArr cSlackArr
  let cutDense := accumTerms n cert.cut.terms
  let dropTerms : Fin n → Option Rat := fun j =>
    dropMinTerm
      (cert.scale * (vArr.1[j.val]'(by rw [vArr.2]; exact j.isLt)) -
        cutDense.1[j.val]'(by rw [cutDense.2]; exact j.isLt))
      cert.lb[j.val]! cert.ub[j.val]!
  let dropOk := allFin n fun j => (dropTerms j).isSome
  let reqRHS := cert.scale * rawRHS - finSum n (fun j => (dropTerms j).getD 0)
  guardExc (decide (0 < cert.scale)) s!"structural GMI cert scale={cert.scale} must be > 0" <|
  guardExc (allTermsInBounds n cert.cut.terms) "structural GMI cert cut has out-of-bounds column" <|
  guardExc rhoOk "structural GMI cert rho references out-of-bounds row or column" <|
  guardExc basicOk "structural GMI cert basicCol must be an integer variable with tableau coeff 1" <|
  guardExc colBndOk "structural GMI cert active non-basic column missing active bound" <|
  guardExc (decide (0 < f0) && decide (f0 < 1)) s!"structural GMI cert beta={betaVal} has non-fractional f0={f0}" <|
  guardExc (cStructLenOk && cSlackLenOk) "structural GMI cert optional coeff array length mismatch" <|
  guardExc cStructOk "structural GMI cert cStruct coefficient below exact GMI coefficient" <|
  guardExc cSlackOk "structural GMI cert cSlack coefficient below exact GMI coefficient" <|
  guardExc dropOk "structural GMI cert finalize/drop perturbed coefficient on unbounded variable side" <|
  guardExc (decide (reqRHS ≤ cert.cut.rhs))
    s!"structural GMI cert cut.rhs={cert.cut.rhs} < required outward RHS {reqRHS}" <|
  .ok ()

theorem checkStructuralGMICertFast_eq (cert : StructuralGMICert) :
    checkStructuralGMICertFast cert = checkStructuralGMICert cert := by
  unfold checkStructuralGMICertFast checkStructuralGMICert
  dsimp only
  split
  · rfl
  · next _hmeta =>
    let alphaDense := accumWeightedRowsN cert.numVars cert.rho.size cert.baseRows cert.rho rfl
    have halpha : ∀ j : Fin cert.numVars,
        alphaDense.1[j.val]'(by rw [alphaDense.2]; exact j.isLt) = sgmiAlpha cert j := fun j =>
      accumWeightedRowsN_getElem cert.baseRows cert.rho rfl j
    let activeArr := sgmiBuildActiveArr cert alphaDense
    have hactive : ∀ j : Fin cert.numVars,
        activeArr.1[j.val]'(by rw [activeArr.2]; exact j.isLt) = sgmiActiveCol cert j := fun j =>
      sgmiBuildActiveArr_getElem cert alphaDense halpha j
    let colBndArr := sgmiBuildColBndArr cert activeArr
    have hcolBnd : ∀ j : Fin cert.numVars,
        colBndArr.1[j.val]'(by rw [colBndArr.2]; exact j.isLt) = sgmiColBnd cert j := fun j =>
      sgmiBuildColBndArr_getElem cert activeArr hactive j
    have hbasic :
        (if hB : cert.basicCol < cert.numVars then
          cert.isInt[cert.basicCol]! &&
            decide (alphaDense.1[cert.basicCol]'(by rw [alphaDense.2]; exact hB) = 1)
         else false) =
        (if hB : cert.basicCol < cert.numVars then
          cert.isInt[cert.basicCol]! && decide (sgmiAlpha cert ⟨cert.basicCol, hB⟩ = 1)
         else false) := by
      split
      · next hB => rw [halpha ⟨cert.basicCol, hB⟩]
      · rfl
    have hcolBndOk :
        allFin cert.numVars (fun j =>
          (colBndArr.1[j.val]'(by rw [colBndArr.2]; exact j.isLt)).isSome) =
          allFin cert.numVars (fun j => (sgmiColBnd cert j).isSome) :=
      allFin_congr fun j => by rw [hcolBnd j]
    have hbeta : sgmiFastBeta cert alphaDense activeArr colBndArr = sgmiBeta cert :=
      sgmiFastBeta_eq cert alphaDense activeArr colBndArr halpha hactive hcolBnd
    have hf0 : fracPart (sgmiFastBeta cert alphaDense activeArr colBndArr) = fracPart (sgmiBeta cert) :=
      congrArg fracPart hbeta
    let exactStructArr := sgmiBuildExactStructArr cert alphaDense activeArr colBndArr
      (fracPart (sgmiFastBeta cert alphaDense activeArr colBndArr))
    have hexactStruct : ∀ j : Fin cert.numVars,
        exactStructArr.1[j.val]'(by rw [exactStructArr.2]; exact j.isLt) =
          sgmiExactStruct cert j := fun j =>
      sgmiBuildExactStructArr_getElem cert alphaDense activeArr colBndArr
        (fracPart (sgmiFastBeta cert alphaDense activeArr colBndArr)) halpha hactive hcolBnd hf0 j
    let cStructArr := sgmiBuildCStructArr cert activeArr exactStructArr
    have hcStruct : ∀ j : Fin cert.numVars,
        cStructArr.1[j.val]'(by rw [cStructArr.2]; exact j.isLt) = sgmiCStruct cert j := fun j =>
      sgmiBuildCStructArr_getElem cert activeArr exactStructArr hactive hexactStruct j
    have hcStructOk :
        allFin cert.numVars (fun j =>
          if activeArr.1[j.val]'(by rw [activeArr.2]; exact j.isLt) then
            decide (exactStructArr.1[j.val]'(by rw [exactStructArr.2]; exact j.isLt) ≤
              cStructArr.1[j.val]'(by rw [cStructArr.2]; exact j.isLt))
          else true) =
          allFin cert.numVars (fun j =>
            if sgmiActiveCol cert j then decide (sgmiExactStruct cert j ≤ sgmiCStruct cert j) else true) :=
      allFin_congr fun j => by rw [hactive j, hexactStruct j, hcStruct j]
    let exactSlackArr := sgmiBuildExactSlackArr cert
      (fracPart (sgmiFastBeta cert alphaDense activeArr colBndArr))
    have hexactSlack : ∀ i : Fin cert.rho.size,
        exactSlackArr.1[i.val]'(by rw [exactSlackArr.2]; exact i.isLt) = sgmiExactSlack cert i :=
      fun i => sgmiBuildExactSlackArr_getElem cert
        (fracPart (sgmiFastBeta cert alphaDense activeArr colBndArr)) hf0 i
    let cSlackArr := sgmiBuildCSlackArr cert exactSlackArr
    have hcSlack : ∀ i : Fin cert.rho.size,
        cSlackArr.1[i.val]'(by rw [cSlackArr.2]; exact i.isLt) = sgmiCSlack cert i := fun i =>
      sgmiBuildCSlackArr_getElem cert exactSlackArr hexactSlack i
    have hcSlackOk :
        allFin cert.rho.size (fun i =>
          match sgmiRowSense cert i with
          | .eq => true
          | _ => decide (exactSlackArr.1[i.val]'(by rw [exactSlackArr.2]; exact i.isLt) ≤
            cSlackArr.1[i.val]'(by rw [cSlackArr.2]; exact i.isLt))) =
          allFin cert.rho.size (fun i =>
            match sgmiRowSense cert i with
            | .eq => true
            | _ => decide (sgmiExactSlack cert i ≤ sgmiCSlack cert i)) :=
      allFin_congr fun i => by cases sgmiRowSense cert i <;> simp [hexactSlack i, hcSlack i]
    let slackPairs := sgmiBuildSlackPairs cert cSlackArr
    let vRowsDense := accumWeightedRowsN cert.numVars cert.rho.size cert.baseRows slackPairs.1 slackPairs.2
    let vArr := sgmiBuildVArr cert activeArr cStructArr vRowsDense
    have hv : ∀ j : Fin cert.numVars,
        vArr.1[j.val]'(by rw [vArr.2]; exact j.isLt) = sgmiV cert j := fun j =>
      sgmiBuildVArr_getElem cert activeArr cStructArr cSlackArr hactive hcStruct hcSlack j
    have hrawRHS :
        sgmiFastRawRHS cert activeArr cStructArr colBndArr cSlackArr = sgmiRawRHS cert :=
      sgmiFastRawRHS_eq cert activeArr cStructArr colBndArr cSlackArr hactive hcStruct hcolBnd hcSlack
    let cutDense := accumTerms cert.numVars cert.cut.terms
    have hdrop : ∀ j : Fin cert.numVars,
        dropMinTerm
          (cert.scale * (vArr.1[j.val]'(by rw [vArr.2]; exact j.isLt)) -
            cutDense.1[j.val]'(by rw [cutDense.2]; exact j.isLt))
          cert.lb[j.val]! cert.ub[j.val]! =
        dropMinTerm
          (cert.scale * sgmiV cert j -
            cutDense.1[j.val]'(by rw [cutDense.2]; exact j.isLt))
          cert.lb[j.val]! cert.ub[j.val]! := fun j => by
      rw [hv j]
    have hdropOk :
        allFin cert.numVars (fun j =>
          (dropMinTerm
            (cert.scale * (vArr.1[j.val]'(by rw [vArr.2]; exact j.isLt)) -
              cutDense.1[j.val]'(by rw [cutDense.2]; exact j.isLt))
            cert.lb[j.val]! cert.ub[j.val]!).isSome) =
          allFin cert.numVars (fun j =>
            (dropMinTerm
              (cert.scale * sgmiV cert j -
                cutDense.1[j.val]'(by rw [cutDense.2]; exact j.isLt))
              cert.lb[j.val]! cert.ub[j.val]!).isSome) :=
      allFin_congr fun j => by rw [hdrop j]
    have hreqRHS :
        cert.scale * sgmiFastRawRHS cert activeArr cStructArr colBndArr cSlackArr -
          finSum cert.numVars (fun j =>
            (dropMinTerm
              (cert.scale * (vArr.1[j.val]'(by rw [vArr.2]; exact j.isLt)) -
                cutDense.1[j.val]'(by rw [cutDense.2]; exact j.isLt))
              cert.lb[j.val]! cert.ub[j.val]!).getD 0) =
        cert.scale * sgmiRawRHS cert -
          finSum cert.numVars (fun j =>
            (dropMinTerm
              (cert.scale * sgmiV cert j -
                cutDense.1[j.val]'(by rw [cutDense.2]; exact j.isLt))
              cert.lb[j.val]! cert.ub[j.val]!).getD 0) := by
      rw [hrawRHS, finSum_congr fun j => by rw [hdrop j]]
    refine guardExc_congr rfl rfl ?_
    refine guardExc_congr rfl rfl ?_
    refine guardExc_congr rfl rfl ?_
    refine guardExc_congr hbasic rfl ?_
    refine guardExc_congr hcolBndOk rfl ?_
    refine guardExc_congr (by rw [hbeta]) (by rw [hbeta]) ?_
    refine guardExc_congr rfl rfl ?_
    refine guardExc_congr hcStructOk rfl ?_
    refine guardExc_congr hcSlackOk rfl ?_
    refine guardExc_congr hdropOk rfl ?_
    refine guardExc_congr (by rw [hreqRHS]) (by rw [hreqRHS]) rfl

theorem checkStructuralGMICertFast_sound (cert : StructuralGMICert)
    (hok : checkStructuralGMICertFast cert = .ok ())
    (hIntLen : cert.isInt.size = cert.numVars)
    (hlbLen : cert.lb.size = cert.numVars)
    (hubLen : cert.ub.size = cert.numVars)
    (hAtULen : cert.colAtUpper.size = cert.numVars)
    (x : Fin cert.numVars → Rat)
    (hfeas : MIPFeasible cert.numVars
      (fun j => cert.isInt[j.val]'(by omega))
      (fun j => cert.lb[j.val]'(by omega))
      (fun j => cert.ub[j.val]'(by omega))
      cert.baseRows x) :
    evalCutOnFin cert.numVars cert.cut x ≤ cert.cut.rhs := by
  rw [checkStructuralGMICertFast_eq] at hok
  exact checkStructuralGMICert_sound cert hok hIntLen hlbLen hubLen hAtULen x hfeas

/-! ### 4c. Universal Structural VUB Disaggregation Certificate Over `baseRows` -/

/-- Complete structural VUB disaggregation certificate deriving `x[targetCol] - effUB * x[binCol] ≤ 0`
directly from a base row in `baseRows`, followed by `finalizeSparseCut` drop & scaling. -/
structure StructuralVUBCert where
  /-- Number of structural variables in the problem. -/
  numVars : Nat
  /-- Integrality flags for each variable. -/
  isInt : Array Bool
  /-- Optional lower bounds for each variable (`none` = `-∞`). -/
  lb : Array (Option Rat)
  /-- Optional upper bounds for each variable (`none` = `+∞`). -/
  ub : Array (Option Rat)
  /-- Base linear constraints of the MILP instance. -/
  baseRows : Array RowSpec
  /-- Index in `baseRows` of the row certifying the variable upper bound. -/
  rowIdx : Nat
  /-- Column index of the binary indicator variable. -/
  binCol : Nat
  /-- Column index of the target variable bounded by `effUB * x[binCol]`. -/
  targetCol : Nat
  /-- Effective upper-bound coefficient multiplying the binary variable. -/
  effUB : Rat
  /-- Positive scaling factor applied during cut finalization. -/
  scale : Rat := 1
  /-- Final emitted VUB disaggregation cut. -/
  cut : CutSpec
  deriving Repr, Inhabited

/-- Checks whether `baseRow` in orientation `s ∈ {1, -1}` certifies `x[targetCol] ≤ effUB * x[binCol]`. -/
def structVUBRowOkOriented (n : Nat) (lb ub : Array (Option Rat))
    (binCol targetCol : Nat) (effUB s : Rat) (r : RowSpec) : Bool :=
  if hb : binCol < n then
    if ht : targetCol < n then
      allTermsInBounds n r.terms &&
      rowLegalMuSignSpec r.sense s &&
      decide (s * r.rhs ≤ 0) &&
      let rd := accumTerms n r.terms
      let cap := -(s * rd.1[binCol]'(by rw [rd.2]; exact hb))
      let atgt := s * rd.1[targetCol]'(by rw [rd.2]; exact ht)
      decide (0 ≤ cap) &&
      decide (0 < atgt) &&
      decide (vubEffUB ub[targetCol]! cap atgt ≤ effUB) &&
      allFin n fun j =>
        if j.val == binCol then true
        else
          let cj := s * rd.1[j.val]'(by rw [rd.2]; exact j.isLt)
          decide (cj = 0) || (decide (0 < cj) && isNonnegCol lb j.val)
    else false
  else false

/-- Executable universal verifier for `StructuralVUBCert`. -/
def checkStructuralVUBCert (cert : StructuralVUBCert) : Except String Unit :=
  let n := cert.numVars
  if !(cert.isInt.size == n && cert.lb.size == n && cert.ub.size == n) then
    .error s!"structural VUB cert metadata arrays must have length {n}"
  else
  let rowOk : Bool :=
    if h : cert.rowIdx < cert.baseRows.size then
      let r := cert.baseRows[cert.rowIdx]
      structVUBRowOkOriented n cert.lb cert.ub cert.binCol cert.targetCol cert.effUB 1 r ||
      structVUBRowOkOriented n cert.lb cert.ub cert.binCol cert.targetCol cert.effUB (-1) r
    else false
  let cutDense := accumTerms n cert.cut.terms
  let rawV : Fin n → Rat := fun j =>
    (if j.val == cert.targetCol then (1 : Rat) else 0) +
    (if j.val == cert.binCol then -cert.effUB else 0)
  let dropTerms : Fin n → Option Rat := fun j =>
    dropMinTerm (cert.scale * rawV j - cutDense.1[j.val]'(by rw [cutDense.2]; exact j.isLt))
      cert.lb[j.val]! cert.ub[j.val]!
  let dropOk := allFin n fun j => (dropTerms j).isSome
  let reqRHS := -finSum n (fun j => (dropTerms j).getD 0)
  guardExc (decide (0 < cert.scale)) s!"structural VUB cert scale={cert.scale} must be > 0" <|
  guardExc (allTermsInBounds n cert.cut.terms) "structural VUB cert cut has out-of-bounds column" <|
  guardExc (decide (cert.binCol < n) && decide (cert.targetCol < n) && decide (cert.binCol ≠ cert.targetCol))
    "structural VUB cert binCol/targetCol out of bounds or equal" <|
  guardExc (isBinaryCol cert.isInt cert.lb cert.ub cert.binCol)
    s!"structural VUB cert binCol={cert.binCol} is not a binary [0,1] integer variable" <|
  guardExc rowOk "structural VUB cert baseRows[rowIdx] does not certify disaggregated VUB bound" <|
  guardExc dropOk "structural VUB cert finalize/drop perturbed coefficient on unbounded variable side" <|
  guardExc (decide (reqRHS ≤ cert.cut.rhs))
    s!"structural VUB cert cut.rhs={cert.cut.rhs} < required outward RHS {reqRHS}" <|
  .ok ()

private theorem structVUBRowOkOriented_sound {n : Nat}
    {isInt : Array Bool} {lb ub : Array (Option Rat)} {baseRows : Array RowSpec}
    {binCol targetCol : Nat} {effUB s : Rat} {r : RowSpec} {x : Fin n → Rat}
    (hIntLen : isInt.size = n) (hlbLen : lb.size = n) (hubLen : ub.size = n)
    (hb : binCol < n) (ht : targetCol < n) (hne : binCol ≠ targetCol)
    (hfeas : MIPFeasible n (fun j => isInt[j.val]'(by omega))
      (fun j => lb[j.val]'(by omega)) (fun j => ub[j.val]'(by omega)) baseRows x)
    (hyBin : isBinVal (x ⟨binCol, hb⟩))
    (hok : structVUBRowOkOriented n lb ub binCol targetCol effUB s r = true)
    (hsat : rowSatOnFin n r x) :
    x ⟨targetCol, ht⟩ - effUB * x ⟨binCol, hb⟩ ≤ 0 := by
  unfold structVUBRowOkOriented at hok
  simp only [hb, ht, ↓reduceDIte, Bool.and_eq_true, decide_eq_true_eq] at hok
  rcases hok with ⟨⟨⟨hcols, hleg⟩, hrhsLe⟩, ⟨⟨⟨hcapNN, hatgtPos⟩, heffLe⟩, hothers⟩⟩
  have hothersAll := allFin_eq_true_iff.mp hothers
  let rd := accumTerms n r.terms
  let cFn : Fin n → Rat := fun j => s * rd.1[j.val]'(by rw [rd.2]; exact j.isLt)
  let cap : Rat := -cFn ⟨binCol, hb⟩
  have hscaled : finSum n (fun j => cFn j * x j) ≤ s * r.rhs := by
    unfold rowSatOnFin at hsat
    rw [evalRowOnFin_eq_finSum_accum r x hcols] at hsat
    exact row_agg_legal_sound hleg hsat
  -- Use `vub_disagg_opt_sound` on the active positive terms (embedding `Fin n` with inactive slots = 0)
  have hrowSplit : cFn ⟨targetCol, ht⟩ * x ⟨targetCol, ht⟩ - cap * x ⟨binCol, hb⟩ ≤ 0 := by
    let tFin : Fin n := ⟨targetCol, ht⟩
    let bFin : Fin n := ⟨binCol, hb⟩
    have htbNe : tFin ≠ bFin := fun h => hne (congrArg Fin.val h).symm
    have hterm : ∀ j : Fin n,
        ((if j = tFin then cFn tFin * x tFin else 0) + (if j = bFin then cFn bFin * x bFin else 0)) ≤
        cFn j * x j := by
      intro j
      by_cases hjt : j = tFin
      · subst hjt; simp [htbNe, Rat.add_zero]
      · by_cases hjb : j = bFin
        · subst hjb; simp [hjt, Rat.zero_add]
        · simp only [hjt, hjb, ↓reduceIte, Rat.add_zero]
          have ho := hothersAll j
          have hneB : (j.val == binCol) = false := beq_false_of_ne (fun h => hjb (Fin.ext h))
          simp only [hneB, Bool.false_eq_true, ↓reduceIte, Bool.or_eq_true, Bool.and_eq_true, decide_eq_true_eq] at ho
          rcases ho with h0 | ⟨hcPos, hnn⟩
          · change 0 ≤ cFn j * x j
            have hc0 : cFn j = 0 := h0
            rw [hc0, Rat.zero_mul]; exact Rat.le_refl
          · exact Rat.mul_nonneg (Rat.le_of_lt hcPos)
              (isNonnegCol_sound hIntLen hlbLen hubLen hfeas j.isLt hnn)
    have hsum := finSum_le_finSum hterm
    rw [finSum_add, finSum_single, finSum_single] at hsum
    have hcomb := Rat.le_trans hsum (Rat.le_trans hscaled hrhsLe)
    have hcapEq : cFn tFin * x tFin - cap * x bFin = cFn tFin * x tFin + cFn bFin * x bFin := by
      dsimp only [cap, bFin]; rw [Rat.sub_eq_add_neg, Rat.neg_mul, Rat.neg_neg]
    rwa [← hcapEq] at hcomb
  have htNN : 0 ≤ x ⟨targetCol, ht⟩ := by
    have ho := hothersAll ⟨targetCol, ht⟩
    have hneB : (targetCol == binCol) = false := beq_false_of_ne (Ne.symm hne)
    simp only [hneB, Bool.false_eq_true, ↓reduceIte, Bool.or_eq_true, Bool.and_eq_true, decide_eq_true_eq] at ho
    rcases ho with h0 | ⟨_, hnn⟩
    · exact absurd h0 (Rat.ne_of_gt hatgtPos)
    · exact isNonnegCol_sound hIntLen hlbLen hubLen hfeas ht hnn
  have h1row : finSum 1 (fun _ => cFn ⟨targetCol, ht⟩ * x ⟨targetCol, ht⟩) - cap * x ⟨binCol, hb⟩ ≤ 0 := by
    simpa [finSum, Rat.zero_add] using hrowSplit
  have heff' : vubEffUB (ub[targetCol]'(by omega)) cap (cFn ⟨targetCol, ht⟩) ≤ effUB := by
    rwa [getElem!_pos ub targetCol (by omega)] at heffLe
  have hvub := vub_disagg_opt_sound (n := 1)
    (a := fun _ => cFn ⟨targetCol, ht⟩) (x := fun _ => x ⟨targetCol, ht⟩)
    (u := fun _ => ub[targetCol]'(by omega)) (C := cap) (rhs := 0) (y := x ⟨binCol, hb⟩)
    h1row Rat.le_refl (fun _ => hatgtPos) (fun _ => htNN)
    (fun _ uk huk => (hfeas.1 ⟨targetCol, ht⟩).2 uk huk) hcapNN hyBin ⟨0, by omega⟩ heff'
  have hsub := (Rat.add_le_add_right (c := -(effUB * x ⟨binCol, hb⟩))).mpr hvub
  simpa [Rat.sub_eq_add_neg, Rat.add_neg_cancel] using hsub

/-- Universal soundness of `checkStructuralVUBCert` over `MIPFeasible`. -/
theorem checkStructuralVUBCert_sound (cert : StructuralVUBCert)
    (hok : checkStructuralVUBCert cert = .ok ())
    (hIntLen : cert.isInt.size = cert.numVars)
    (hlbLen : cert.lb.size = cert.numVars)
    (hubLen : cert.ub.size = cert.numVars)
    (x : Fin cert.numVars → Rat)
    (hfeas : MIPFeasible cert.numVars
      (fun j => cert.isInt[j.val]'(by omega))
      (fun j => cert.lb[j.val]'(by omega))
      (fun j => cert.ub[j.val]'(by omega))
      cert.baseRows x) :
    evalCutOnFin cert.numVars cert.cut x ≤ cert.cut.rhs := by
  unfold checkStructuralVUBCert at hok
  simp only [hIntLen, hlbLen, hubLen, beq_self_eq_true, Bool.and_self, Bool.not_true, Bool.false_eq_true, ↓reduceIte] at hok
  rcases guardExc_ok hok with ⟨hscaleD, hok⟩
  rcases guardExc_ok hok with ⟨hcutColsT, hok⟩
  rcases guardExc_ok hok with ⟨hcolBndT, hok⟩
  rcases guardExc_ok hok with ⟨hbinColT, hok⟩
  rcases guardExc_ok hok with ⟨hrowOkT, hok⟩
  rcases guardExc_ok hok with ⟨hdropOkT, hok⟩
  rcases guardExc_ok hok with ⟨hrhsD, _⟩
  let n := cert.numVars
  have hscalePos : 0 < cert.scale := of_decide_eq_true hscaleD
  simp only [Bool.and_eq_true, decide_eq_true_eq] at hcolBndT
  rcases hcolBndT with ⟨⟨hb, ht⟩, hne⟩
  have hyBin : isBinVal (x ⟨cert.binCol, hb⟩) :=
    isBinaryCol_sound hIntLen hlbLen hubLen hfeas hb hbinColT
  have hrawLe0 : x ⟨cert.targetCol, ht⟩ - cert.effUB * x ⟨cert.binCol, hb⟩ ≤ 0 := by
    split at hrowOkT
    · next hr =>
      rcases Bool.or_eq_true _ _ |>.mp hrowOkT with h1 | hm1
      · exact structVUBRowOkOriented_sound hIntLen hlbLen hubLen hb ht hne hfeas hyBin h1 (hfeas.2.2 ⟨cert.rowIdx, hr⟩)
      · exact structVUBRowOkOriented_sound hIntLen hlbLen hubLen hb ht hne hfeas hyBin hm1 (hfeas.2.2 ⟨cert.rowIdx, hr⟩)
    · contradiction
  let rawV : Fin n → Rat := fun j =>
    (if j.val == cert.targetCol then (1 : Rat) else 0) +
    (if j.val == cert.binCol then -cert.effUB else 0)
  have hrawSum : finSum n (fun j => rawV j * x j) ≤ 0 := by
    have hsplit : finSum n (fun j => rawV j * x j) =
        finSum n (fun j => (if j.val == cert.targetCol then (1 : Rat) else 0) * x j) +
        finSum n (fun j => (if j.val == cert.binCol then -cert.effUB else 0) * x j) := by
      rw [← finSum_add]
      exact finSum_congr fun j => Rat.add_mul _ _ _
    rw [hsplit]
    have h1 : finSum n (fun j => (if j.val == cert.targetCol then (1 : Rat) else 0) * x j) = x ⟨cert.targetCol, ht⟩ := by
      have hc : finSum n (fun j : Fin n => (if j.val == cert.targetCol then (1 : Rat) else 0) * x j) =
          finSum n (fun j : Fin n => if j = ⟨cert.targetCol, ht⟩ then x ⟨cert.targetCol, ht⟩ else 0) := by
        apply finSum_congr; intro j
        by_cases hj : j = ⟨cert.targetCol, ht⟩
        · subst hj; simp [Rat.one_mul]
        · have hne' : (j.val == cert.targetCol) = false := beq_false_of_ne (fun h => hj (Fin.ext h))
          simp [hj, hne']
      rw [hc, finSum_single]
    have h2 : finSum n (fun j => (if j.val == cert.binCol then -cert.effUB else 0) * x j) = -(cert.effUB * x ⟨cert.binCol, hb⟩) := by
      have hc : finSum n (fun j : Fin n => (if j.val == cert.binCol then -cert.effUB else 0) * x j) =
          finSum n (fun j : Fin n => if j = ⟨cert.binCol, hb⟩ then -(cert.effUB * x ⟨cert.binCol, hb⟩) else 0) := by
        apply finSum_congr; intro j
        by_cases hj : j = ⟨cert.binCol, hb⟩
        · subst hj; simp [Rat.neg_mul]
        · have hne' : (j.val == cert.binCol) = false := beq_false_of_ne (fun h => hj (Fin.ext h))
          simp [hj, hne']
      rw [hc, finSum_single]
    rw [h1, h2, ← Rat.sub_eq_add_neg]
    exact hrawLe0
  let lbFn : Fin n → Option Rat := fun j => cert.lb[j.val]'(by omega)
  let ubFn : Fin n → Option Rat := fun j => cert.ub[j.val]'(by omega)
  have hlbB : ∀ j : Fin n, cert.lb[j.val]! = lbFn j := fun j => getElem!_pos cert.lb j.val (by omega)
  have hubB : ∀ j : Fin n, cert.ub[j.val]! = ubFn j := fun j => getElem!_pos cert.ub j.val (by omega)
  let cutDense := accumTerms n cert.cut.terms
  let outFn : Fin n → Rat := fun j => cutDense.1[j.val]'(by rw [cutDense.2]; exact j.isLt)
  let loFn : Fin n → Rat := fun j => (dropMinTerm (cert.scale * rawV j - outFn j) (lbFn j) (ubFn j)).getD 0
  have hdropAll := allFin_eq_true_iff.mp hdropOkT
  have hlo : ∀ j : Fin n, dropMinTerm (cert.scale * rawV j - outFn j) (lbFn j) (ubFn j) = some (loFn j) := by
    intro j
    have hj := hdropAll j
    rw [hlbB j, hubB j] at hj
    dsimp only [loFn]
    cases hd : dropMinTerm (cert.scale * rawV j - outFn j) (lbFn j) (ubFn j) with
    | none => rw [hd] at hj; simp at hj
    | some lo => simp
  have hrhsLe : cert.scale * 0 - finSum n loFn ≤ cert.cut.rhs := by
    have h := of_decide_eq_true hrhsD
    have hcong : finSum n (fun j => (dropMinTerm (cert.scale * rawV j - outFn j) cert.lb[j.val]! cert.ub[j.val]!).getD 0) =
        finSum n loFn :=
      finSum_congr fun j => by dsimp only [loFn]; rw [hlbB j, hubB j]
    rw [Rat.mul_zero, rat_zero_sub]
    rwa [← hcong]
  rw [evalCutOnFin_eq_finSum_accum cert.cut x hcutColsT]
  exact finalize_cut_drop_universal_sound hscalePos hrawSum hfeas.1 hlo hrhsLe

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
  let flowRowIdx ← match v.getField? "flowRowIdx" with
    | none | some .null => pure none
    | some fv => some <$> fv.asNat
  let vubRowIdx ← match v.getField? "vubRowIdx" with
    | none | some .null => pure none
    | some vv => some <$> vv.asNat
  pure { prodCol, invOutCol, binCol, demand, vubCap, vubRHS, inS, coeffY, flowRowIdx, vubRowIdx }

private def parseRowMultPairJson (v : JsonVal) : Except String (Nat × Rat) := do
  let rowIdx ← v.getNat "rowIdx"
  let mu ← v.getRat "mu"
  pure (rowIdx, mu)

private def parseRhoEntryJson (v : JsonVal) : Except String (Nat × Rat) := do
  let rowIdx ← v.getNat "rowIdx"
  let val ← match v.getField? "val" with
    | some rv => rv.asRat
    | none => v.getRat "mu"
  pure (rowIdx, val)

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

/-- JSON entrypoint for `cover-check`: universally verifies a lifted binary knapsack cover certificate
against `isInt`, `(lb, ub)`, and `baseRows`. -/
def checkKnapsackCoverJson (v : JsonVal) : Except String String := do
  let numVars ← v.getNat "numVars"
  let isInt ← v.getArray "isInt" >>= parseBoolArrayJson
  let lb ← v.getArray "lb" >>= parseOptRatArrayJson
  let ub ← v.getArray "ub" >>= parseOptRatArrayJson
  let baseRows ← (← v.getArray "baseRows").mapM parseRowSpecJson
  let rowIdx ← match v.getField? "rowIdx" with
    | none | some .null => pure none
    | some rv => some <$> rv.asNat
  let aggMultipliers ← match v.getField? "aggMultipliers" with
    | none | some .null => pure none
    | some av => some <$> ((← av.asArray).mapM parseRowMultPairJson)
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
  let cert : KnapsackCoverCert :=
    { numVars, isInt, lb, ub, baseRows, rowIdx, aggMultipliers, row, sign, items, knapRHS, scale, cut }
  checkKnapsackCoverCert cert
  let wits ← checkOptionalWitnesses v cut
  pure s!"VERIFIED (cover-check, {items.size} items, {wits} witnesses)"

/-- JSON entrypoint for `lotsizing-check`: universally verifies an `(l, S)` lot-sizing chain cut
against `isInt`, `(lb, ub)`, column distinctness, and `baseRows`. -/
def checkLotSizingCutJson (v : JsonVal) : Except String String := do
  let numVars ← v.getNat "numVars"
  let isInt ← v.getArray "isInt" >>= parseBoolArrayJson
  let lb ← v.getArray "lb" >>= parseOptRatArrayJson
  let ub ← v.getArray "ub" >>= parseOptRatArrayJson
  let baseRows ← (← v.getArray "baseRows").mapM parseRowSpecJson
  let invInCol ← match v.getField? "invInCol" with
    | none | some .null => pure none
    | some iv => some <$> iv.asNat
  let nodes ← (← v.getArray "nodes").mapM parseLotSizingNodeJson
  let cutDem ← v.getRat "cutDem"
  let scale ← parseScaleJson v
  let cut ← v.getField "cut" >>= parseCutSpecJson
  let cert : LotSizingCutCert := { numVars, isInt, lb, ub, baseRows, invInCol, nodes, cutDem, scale, cut }
  checkLotSizingCutCert cert
  let wits ← checkOptionalWitnesses v cut
  pure s!"VERIFIED (lotsizing-check, {nodes.size} nodes, {wits} witnesses)"

/-- JSON entrypoint for structural-derivation `gmi-check`: universally verifies a GMI cut derived
from `baseRows` via rational tableau multipliers `rho`. -/
def checkStructuralGMIJson (v : JsonVal) : Except String String := do
  let numVars ← v.getNat "numVars"
  let isInt ← v.getArray "isInt" >>= parseBoolArrayJson
  let lb ← v.getArray "lb" >>= parseOptRatArrayJson
  let ub ← v.getArray "ub" >>= parseOptRatArrayJson
  let baseRows ← (← v.getArray "baseRows").mapM parseRowSpecJson
  let basicCol ← v.getNat "basicCol"
  let rho ← (← v.getArray "rho").mapM parseRhoEntryJson
  let colAtUpper ← v.getArray "colAtUpper" >>= parseBoolArrayJson
  let cStruct ← match v.getField? "cStruct" with
    | none | some .null => pure none
    | some av => some <$> (av.asArray >>= parseRatArrayJson)
  let cSlack ← match v.getField? "cSlack" with
    | none | some .null => pure none
    | some av => some <$> (av.asArray >>= parseRatArrayJson)
  let scale ← parseScaleJson v
  let cut ← v.getField "cut" >>= parseCutSpecJson
  let cert : StructuralGMICert :=
    { numVars, isInt, lb, ub, baseRows, basicCol, rho, colAtUpper, cStruct, cSlack, scale, cut }
  checkStructuralGMICertFast cert
  let wits ← checkOptionalWitnesses v cut
  pure s!"VERIFIED (gmi-check-structural, {rho.size} rows, {wits} witnesses)"

/-- JSON entrypoint for structural-derivation `vub-check`: universally verifies a disaggregated VUB
cut derived directly from `baseRows[rowIdx]`. -/
def checkStructuralVUBJson (v : JsonVal) : Except String String := do
  let numVars ← v.getNat "numVars"
  let isInt ← v.getArray "isInt" >>= parseBoolArrayJson
  let lb ← v.getArray "lb" >>= parseOptRatArrayJson
  let ub ← v.getArray "ub" >>= parseOptRatArrayJson
  let baseRows ← (← v.getArray "baseRows").mapM parseRowSpecJson
  let rowIdx ← v.getNat "rowIdx"
  let binCol ← v.getNat "binCol"
  let targetCol ← v.getNat "targetCol"
  let effUB ← v.getRat "effUB"
  let scale ← parseScaleJson v
  let cut ← v.getField "cut" >>= parseCutSpecJson
  let cert : StructuralVUBCert :=
    { numVars, isInt, lb, ub, baseRows, rowIdx, binCol, targetCol, effUB, scale, cut }
  checkStructuralVUBCert cert
  let wits ← checkOptionalWitnesses v cut
  pure s!"VERIFIED (vub-check-structural, row {rowIdx}, {wits} witnesses)"

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
