/-
GoblinFormal.Track1: Verified LSAP Optimality Checker (square and rectangular).
Certifies primal-dual solutions via complementary slackness and provides a
rigorous dual lower bound (`checkLSAPSafe`) for inexact floating-point duals.
-/

import GoblinFormal.Common.Basic
import GoblinFormal.Common.Json

namespace GoblinFormal.Track1

/-- Tail-recursive bounded universal check over `Fin n` starting at index `k`. -/
def allFinFrom (n : Nat) (f : Fin n → Bool) (k : Nat) : Bool :=
  if h : k < n then
    if f ⟨k, h⟩ then allFinFrom n f (k + 1) else false
  else
    true
termination_by n - k

/-- Decidable universal quantifier over `Fin n`. -/
def allFin (n : Nat) (f : Fin n → Bool) : Bool :=
  allFinFrom n f 0

/-- Decidable existential quantifier over `Fin n`. -/
def anyFin (n : Nat) (f : Fin n → Bool) : Bool :=
  !allFin n (fun i => !f i)

private theorem allFinFrom_eq_true_aux (n : Nat) (f : Fin n → Bool) (d k : Nat)
    (hd : n - k = d) :
    allFinFrom n f k = true ↔ ∀ i : Fin n, k ≤ i.val → f i = true := by
  induction d generalizing k with
  | zero =>
    have hge : ¬(k < n) := by omega
    rw [allFinFrom]
    simp only [hge, ↓reduceDIte, true_iff]
    intro i hi
    omega
  | succ d ih =>
    by_cases hkn : k < n
    · rw [allFinFrom]
      simp only [hkn, ↓reduceDIte]
      by_cases hfk : f ⟨k, hkn⟩ = true
      · simp only [hfk, ↓reduceIte]
        have hih := ih (k + 1) (by omega)
        rw [hih]
        constructor
        · intro h i hi
          by_cases heq : i.val = k
          · have hfi : i = ⟨k, hkn⟩ := Fin.ext heq
            rw [hfi]; exact hfk
          · exact h i (by omega)
        · intro h i hi
          exact h i (by omega)
      · have hfkf : f ⟨k, hkn⟩ = false := Bool.eq_false_iff.mpr hfk
        simp only [hfk, ↓reduceIte, Bool.false_eq_true, false_iff]
        intro h
        have ht := h ⟨k, hkn⟩ (Nat.le_refl k)
        rw [ht] at hfkf
        contradiction
    · have hge : ¬(k < n) := hkn
      rw [allFinFrom]
      simp only [hge, ↓reduceDIte, true_iff]
      intro i hi
      omega

theorem allFin_eq_true {n : Nat} {f : Fin n → Bool} :
    allFin n f = true ↔ ∀ i : Fin n, f i = true := by
  unfold allFin
  rw [allFinFrom_eq_true_aux n f n 0 (Nat.sub_zero n)]
  exact ⟨fun h i => h i (Nat.zero_le _), fun h i _ => h i⟩

theorem anyFin_eq_false {n : Nat} {f : Fin n → Bool} :
    anyFin n f = false ↔ ∀ i : Fin n, f i = false := by
  unfold anyFin
  rw [Bool.not_eq_false', allFin_eq_true]
  constructor
  · intro h i
    have hi := h i
    cases hfi : f i <;> simp_all
  · intro h i
    simp [h i]

/-- Checks injectivity of `p : Fin m → Fin n`. -/
def checkInjective (m n : Nat) (p : Fin m → Fin n) : Bool :=
  allFin m (fun i₁ => allFin m (fun i₂ => !(p i₁ == p i₂) || (i₁ == i₂)))

theorem checkInjective_iff {m n : Nat} {p : Fin m → Fin n} :
    checkInjective m n p = true ↔ ∀ i₁ i₂ : Fin m, p i₁ = p i₂ → i₁ = i₂ := by
  unfold checkInjective
  rw [allFin_eq_true]
  constructor
  · intro h i₁ i₂ hp
    have hi := (allFin_eq_true.mp (h i₁)) i₂
    simp only [Bool.or_eq_true, Bool.not_eq_true', beq_iff_eq, beq_eq_false_iff_ne] at hi
    rcases hi with hne | heq
    · exact absurd hp hne
    · exact heq
  · intro h i₁
    rw [allFin_eq_true]
    intro i₂
    simp only [Bool.or_eq_true, Bool.not_eq_true', beq_iff_eq, beq_eq_false_iff_ne]
    by_cases hp : p i₁ = p i₂
    · right; exact h i₁ i₂ hp
    · left; exact hp

/-- Checks dual feasibility: `∀ i j, u i + v j ≤ C i j`. -/
def checkDualFeasible (m n : Nat) (C : Fin m → Fin n → Rat)
    (u : Fin m → Rat) (v : Fin n → Rat) : Bool :=
  allFin m (fun i => allFin n (fun j => decide (u i + v j ≤ C i j)))

theorem checkDualFeasible_iff {m n : Nat} {C : Fin m → Fin n → Rat}
    {u : Fin m → Rat} {v : Fin n → Rat} :
    checkDualFeasible m n C u v = true ↔ ∀ (i : Fin m) (j : Fin n), u i + v j ≤ C i j := by
  unfold checkDualFeasible
  rw [allFin_eq_true]
  constructor
  · intro h i j
    have hij := (allFin_eq_true.mp (h i)) j
    exact of_decide_eq_true hij
  · intro h i
    rw [allFin_eq_true]
    intro j
    exact decide_eq_true (h i j)

/-- Checks rectangular dual conditions when `m < n`:
every `v j ≤ 0` and every unassigned column `j` has `v j = 0`. -/
def checkRectDual (m n : Nat) (p : Fin m → Fin n) (v : Fin n → Rat) : Bool :=
  if m < n then
    allFin n (fun j =>
      decide (v j ≤ 0) &&
      (anyFin m (fun i => p i == j) || decide (v j = 0)))
  else
    true

theorem checkRectDual_iff {m n : Nat} {p : Fin m → Fin n} {v : Fin n → Rat} :
    checkRectDual m n p v = true ↔
      (m < n → (∀ j : Fin n, v j ≤ 0) ∧ (∀ j : Fin n, (∀ i : Fin m, p i ≠ j) → v j = 0)) := by
  unfold checkRectDual
  split
  · next hmn =>
    simp only [allFin_eq_true, Bool.and_eq_true, Bool.or_eq_true]
    constructor
    · intro h _
      refine ⟨fun j => of_decide_eq_true (h j).1, fun j hj => ?_⟩
      rcases (h j).2 with hany | hv0
      · have hnot : anyFin m (fun i => p i == j) = false :=
          anyFin_eq_false.mpr (fun i => beq_false_of_ne (hj i))
        rw [hnot] at hany
        contradiction
      · exact of_decide_eq_true hv0
    · intro h j
      have ⟨hle, hunass⟩ := h hmn
      refine ⟨decide_eq_true (hle j), ?_⟩
      by_cases hex : ∃ i : Fin m, p i = j
      · left
        rcases hex with ⟨i, hi⟩
        cases hres : anyFin m (fun k => p k == j)
        · have hf := anyFin_eq_false.mp hres i
          simp [hi] at hf
        · rfl
      · right
        have hforall : ∀ i : Fin m, p i ≠ j := fun i hpi => hex ⟨i, hpi⟩
        exact decide_eq_true (hunass j hforall)
  · next hmn =>
    simp [hmn]

/-- Checks complementary slackness on matched edges: `∀ i, u i + v (p i) = C i (p i)`. -/
def checkCompSlack (m n : Nat) (C : Fin m → Fin n → Rat)
    (p : Fin m → Fin n) (u : Fin m → Rat) (v : Fin n → Rat) : Bool :=
  allFin m (fun i => decide (u i + v (p i) = C i (p i)))

theorem checkCompSlack_iff {m n : Nat} {C : Fin m → Fin n → Rat}
    {p : Fin m → Fin n} {u : Fin m → Rat} {v : Fin n → Rat} :
    checkCompSlack m n C p u v = true ↔ ∀ i : Fin m, u i + v (p i) = C i (p i) := by
  unfold checkCompSlack
  rw [allFin_eq_true]
  exact ⟨fun h i => of_decide_eq_true (h i), fun h i => decide_eq_true (h i)⟩

/-- Exact LSAP optimality certificate checker for `m ≤ n`. -/
def checkLSAP (m n : Nat) (C : Fin m → Fin n → Rat)
    (p : Fin m → Fin n) (u : Fin m → Rat) (v : Fin n → Rat) : Bool :=
  decide (m ≤ n) &&
  checkInjective m n p &&
  checkDualFeasible m n C u v &&
  checkRectDual m n p v &&
  checkCompSlack m n C p u v

/-- For an injective `p : Fin m → Fin n`, if `g` vanishes outside the range of `p`,
the sum over `p` equals the full sum over `Fin n`. -/
theorem finSum_inj_eq_of_unassigned_zero {m n : Nat} {g : Fin n → Rat} {p : Fin m → Fin n}
    (hp : ∀ i₁ i₂, p i₁ = p i₂ → i₁ = i₂)
    (hunassigned : ∀ j : Fin n, (∀ i : Fin m, p i ≠ j) → g j = 0) :
    finSum m (fun i => g (p i)) = finSum n g := by
  induction m generalizing g with
  | zero =>
    simp only [finSum_zero_n]
    have hg0 : ∀ j : Fin n, g j = 0 := fun j => hunassigned j (fun i => Fin.elim0 i)
    rw [finSum_congr hg0, finSum_zero]
  | succ m ih =>
    let k : Fin n := p (Fin.last m)
    let g' : Fin n → Rat := fun j => if j = k then 0 else g j
    have hp' : ∀ i₁ i₂ : Fin m, p i₁.castSucc = p i₂.castSucc → i₁ = i₂ := by
      intro i₁ i₂ heq
      have hinj := hp i₁.castSucc i₂.castSucc heq
      exact Fin.ext (by simpa [Fin.ext_iff] using hinj)
    have hunassigned' : ∀ j : Fin n, (∀ i : Fin m, p i.castSucc ≠ j) → g' j = 0 := by
      intro j hj
      dsimp [g']
      by_cases hjk : j = k
      · simp [hjk]
      · simp only [hjk, ↓reduceIte]
        apply hunassigned j
        intro i
        by_cases hil : i = Fin.last m
        · subst hil; exact Ne.symm hjk
        · have hilt : i.val < m := by
            have := i.isLt
            have hne : i.val ≠ m := fun hv => hil (Fin.ext hv)
            omega
          exact hj ⟨i.val, hilt⟩
    have hih := ih hp' hunassigned'
    have hmatch :
        finSum m (fun i => g' (p i.castSucc)) = finSum m (fun i => g (p i.castSucc)) := by
      apply finSum_congr
      intro i
      dsimp [g']
      have hne : p i.castSucc ≠ k := by
        intro heq
        have hinj := hp i.castSucc (Fin.last m) heq
        have hval := congrArg Fin.val hinj
        simp only [Fin.val_castSucc, Fin.val_last] at hval
        omega
      simp [hne]
    have hsplit : finSum n g = finSum n g' + g k := by
      have heq : ∀ j, g j = g' j + (if j = k then g k else 0) := by
        intro j
        dsimp [g']
        by_cases hj : j = k
        · subst hj; simp [Rat.zero_add]
        · simp [hj, Rat.add_zero]
      rw [finSum_congr heq, finSum_add, finSum_single]
    rw [finSum_succ, ← hmatch, hih, ← hsplit]

/-- Summing a non-positive function over an injective image is at least the full sum. -/
theorem finSum_inj_nonpos_ge {m n : Nat} {g : Fin n → Rat} {q : Fin m → Fin n}
    (hg : ∀ j, g j ≤ 0) (hq : ∀ i₁ i₂, q i₁ = q i₂ → i₁ = i₂) :
    finSum n g ≤ finSum m (fun i => g (q i)) := by
  have hnonneg : ∀ j, 0 ≤ -g j := by
    intro j
    exact Rat.le_neg_iff.mpr (by simpa using hg j)
  have hle := finSum_inj_le hnonneg hq
  rw [finSum_neg, finSum_neg, Rat.neg_le_neg_iff] at hle
  exact hle

/-- Weak duality lower bound for any dual-feasible `(u, v)` with `v ≤ 0` when `m < n`. -/
theorem dual_lower_bound {m n : Nat} (hm : m ≤ n) (C : Fin m → Fin n → Rat)
    (u : Fin m → Rat) (v : Fin n → Rat)
    (hfeas : ∀ (i : Fin m) (j : Fin n), u i + v j ≤ C i j)
    (hrect : m < n → ∀ j : Fin n, v j ≤ 0)
    (q : Fin m → Fin n) (hq : ∀ i₁ i₂, q i₁ = q i₂ → i₁ = i₂) :
    finSum m u + finSum n v ≤ finSum m (fun i => C i (q i)) := by
  have hv_le : finSum n v ≤ finSum m (fun i => v (q i)) := by
    rcases Nat.eq_or_lt_of_le hm with rfl | hlt
    · rw [finSum_inj_eq hq]
      exact Rat.le_refl
    · exact finSum_inj_nonpos_ge (hrect hlt) hq
  have hsum_le :
      finSum m u + finSum n v ≤ finSum m u + finSum m (fun i => v (q i)) :=
    Rat.add_le_add_left.mpr hv_le
  have hfeas_sum :
      finSum m u + finSum m (fun i => v (q i)) ≤ finSum m (fun i => C i (q i)) := by
    rw [← finSum_add]
    exact finSum_le_finSum (fun i => hfeas i (q i))
  exact Rat.le_trans hsum_le hfeas_sum

/-- Soundness of the exact LSAP certificate checker `checkLSAP`:
every valid certificate guarantees global optimality over all injective matchings. -/
theorem checkLSAP_sound {m n : Nat} {C : Fin m → Fin n → Rat}
    {p : Fin m → Fin n} {u : Fin m → Rat} {v : Fin n → Rat}
    (hok : checkLSAP m n C p u v = true)
    (q : Fin m → Fin n) (hq : ∀ i₁ i₂, q i₁ = q i₂ → i₁ = i₂) :
    finSum m (fun i => C i (p i)) ≤ finSum m (fun i => C i (q i)) := by
  unfold checkLSAP at hok
  simp only [Bool.and_eq_true] at hok
  rcases hok with ⟨⟨⟨⟨hm_b, hinj_b⟩, hfeas_b⟩, hrect_b⟩, hcs_b⟩
  have hm : m ≤ n := of_decide_eq_true hm_b
  have hp : ∀ i₁ i₂ : Fin m, p i₁ = p i₂ → i₁ = i₂ := checkInjective_iff.mp hinj_b
  have hfeas : ∀ (i : Fin m) (j : Fin n), u i + v j ≤ C i j := checkDualFeasible_iff.mp hfeas_b
  have hrect := checkRectDual_iff.mp hrect_b
  have hcs : ∀ i : Fin m, u i + v (p i) = C i (p i) := checkCompSlack_iff.mp hcs_b
  have hv_eq : finSum m (fun i => v (p i)) = finSum n v := by
    rcases Nat.eq_or_lt_of_le hm with rfl | hlt
    · exact finSum_inj_eq hp
    · exact finSum_inj_eq_of_unassigned_zero hp (hrect hlt).2
  have hprimal : finSum m (fun i => C i (p i)) = finSum m u + finSum n v := by
    have hcongr : finSum m (fun i => C i (p i)) = finSum m (fun i => u i + v (p i)) :=
      finSum_congr (fun i => (hcs i).symm)
    rw [hcongr, finSum_add, hv_eq]
  rw [hprimal]
  exact dual_lower_bound hm C u v hfeas (fun h => (hrect h).1) q hq

/-- Corollary of `checkLSAP_sound` for maximization problems encoded via negated costs. -/
theorem checkLSAP_sound_max {m n : Nat} {C : Fin m → Fin n → Rat}
    {p : Fin m → Fin n} {u : Fin m → Rat} {v : Fin n → Rat}
    (hok : checkLSAP m n (fun i j => -C i j) p u v = true)
    (q : Fin m → Fin n) (hq : ∀ i₁ i₂, q i₁ = q i₂ → i₁ = i₂) :
    finSum m (fun i => C i (q i)) ≤ finSum m (fun i => C i (p i)) := by
  have hmin := checkLSAP_sound hok q hq
  rw [finSum_neg, finSum_neg, Rat.neg_le_neg_iff] at hmin
  exact hmin

/-- Tail-recursive minimum over `Fin n` starting from `k` with accumulator `acc`. -/
def minFinLoop (n : Nat) (f : Fin n → Rat) (k : Nat) (acc : Rat) : Rat :=
  if h : k < n then
    let cur := f ⟨k, h⟩
    minFinLoop n f (k + 1) (if acc ≤ cur then acc else cur)
  else
    acc
termination_by n - k

private theorem minFinLoop_le_acc_aux (n : Nat) (f : Fin n → Rat) (d k : Nat)
    (acc : Rat) (hd : n - k = d) :
    minFinLoop n f k acc ≤ acc := by
  induction d generalizing k acc with
  | zero =>
    have hge : ¬(k < n) := by omega
    rw [minFinLoop]
    simp only [hge, ↓reduceDIte]
    exact Rat.le_refl
  | succ d ih =>
    by_cases hkn : k < n
    · rw [minFinLoop]
      simp only [hkn, ↓reduceDIte]
      let nextAcc := if acc ≤ f ⟨k, hkn⟩ then acc else f ⟨k, hkn⟩
      have hih := ih (k + 1) nextAcc (by omega)
      have hnext : nextAcc ≤ acc := by
        dsimp [nextAcc]
        split
        · exact Rat.le_refl
        · next hnot =>
          rcases Rat.le_total (a := acc) (b := f ⟨k, hkn⟩) with hle | hge
          · exact absurd hle hnot
          · exact hge
      exact Rat.le_trans hih hnext
    · rw [minFinLoop]
      simp only [hkn, ↓reduceDIte]
      exact Rat.le_refl

private theorem minFinLoop_le_elem_aux (n : Nat) (f : Fin n → Rat) (d k : Nat)
    (acc : Rat) (hd : n - k = d) :
    ∀ j : Fin n, k ≤ j.val → minFinLoop n f k acc ≤ f j := by
  induction d generalizing k acc with
  | zero =>
    intro j hj
    omega
  | succ d ih =>
    intro j hj
    by_cases hkn : k < n
    · rw [minFinLoop]
      simp only [hkn, ↓reduceDIte]
      let cur := f ⟨k, hkn⟩
      let nextAcc := if acc ≤ cur then acc else cur
      by_cases heq : j.val = k
      · have hfj : f j = cur := congrArg f (Fin.ext heq)
        rw [hfj]
        have h1 : minFinLoop n f (k + 1) nextAcc ≤ nextAcc :=
          minFinLoop_le_acc_aux n f (n - (k + 1)) (k + 1) nextAcc rfl
        have h2 : nextAcc ≤ cur := by
          dsimp [nextAcc]
          split
          · next hle => exact hle
          · exact Rat.le_refl
        exact Rat.le_trans h1 h2
      · exact ih (k + 1) nextAcc (by omega) j (by omega)
    · omega

/-- Exact safe row dual potential `uSafe i = min_{j} (C i j - v j)`. -/
def uSafe (m n : Nat) (C : Fin m → Fin n → Rat) (v : Fin n → Rat) (i : Fin m) : Rat :=
  match n with
  | 0 => 0
  | n' + 1 =>
    minFinLoop (n' + 1) (fun j => C i j - v j) 1
      (C i ⟨0, Nat.zero_lt_succ n'⟩ - v ⟨0, Nat.zero_lt_succ n'⟩)

theorem uSafe_add_le {m n : Nat} (C : Fin m → Fin n → Rat) (v : Fin n → Rat)
    (i : Fin m) (j : Fin n) :
    uSafe m n C v i + v j ≤ C i j := by
  cases n with
  | zero => exact Fin.elim0 j
  | succ n' =>
    have hdiff : uSafe m (n' + 1) C v i ≤ C i j - v j := by
      unfold uSafe
      dsimp only
      by_cases hj0 : j.val = 0
      · have hjz : j = ⟨0, Nat.zero_lt_succ n'⟩ := Fin.ext hj0
        rw [hjz]
        exact minFinLoop_le_acc_aux (n' + 1) (fun k => C i k - v k) _ 1 _ rfl
      · have hjge : 1 ≤ j.val := by omega
        exact minFinLoop_le_elem_aux (n' + 1) (fun k => C i k - v k) _ 1 _ rfl j hjge
    have hadd := Rat.add_le_add_right (c := v j).mpr hdiff
    rwa [Rat.sub_add_cancel] at hadd

/-- Exact rational dual lower bound induced by `v` and `uSafe`. -/
def safeLowerBound (m n : Nat) (C : Fin m → Fin n → Rat) (v : Fin n → Rat) : Rat :=
  finSum m (uSafe m n C v) + finSum n v

theorem uSafe_lower_bound {m n : Nat} (hm : m ≤ n) (C : Fin m → Fin n → Rat)
    (v : Fin n → Rat) (hrect : m < n → ∀ j : Fin n, v j ≤ 0)
    (q : Fin m → Fin n) (hq : ∀ i₁ i₂, q i₁ = q i₂ → i₁ = i₂) :
    safeLowerBound m n C v ≤ finSum m (fun i => C i (q i)) :=
  dual_lower_bound hm C (uSafe m n C v) v (uSafe_add_le C v) hrect q hq

/-- Safe LSAP verifier for inexact floating-point duals `v`:
checks `m ≤ n`, injectivity of `p`, `v ≤ 0` when `m < n`, `0 ≤ tol`, and
`primal(p) - safeLowerBound(C, v) ≤ tol`. -/
def checkLSAPSafe (m n : Nat) (C : Fin m → Fin n → Rat)
    (p : Fin m → Fin n) (v : Fin n → Rat) (tol : Rat) : Bool :=
  decide (m ≤ n) &&
  checkInjective m n p &&
  (if m < n then allFin n (fun j => decide (v j ≤ 0)) else true) &&
  decide (0 ≤ tol) &&
  decide (finSum m (fun i => C i (p i)) - safeLowerBound m n C v ≤ tol)

/-- Soundness of `checkLSAPSafe`:
proves both that `safeLowerBound` is a valid global lower bound for every injective `q`,
and that the primal matching `p` is within `tol` of global optimality. -/
theorem checkLSAPSafe_sound {m n : Nat} {C : Fin m → Fin n → Rat}
    {p : Fin m → Fin n} {v : Fin n → Rat} {tol : Rat}
    (hok : checkLSAPSafe m n C p v tol = true)
    (q : Fin m → Fin n) (hq : ∀ i₁ i₂, q i₁ = q i₂ → i₁ = i₂) :
    safeLowerBound m n C v ≤ finSum m (fun i => C i (q i)) ∧
    finSum m (fun i => C i (p i)) - tol ≤ finSum m (fun i => C i (q i)) := by
  unfold checkLSAPSafe at hok
  simp only [Bool.and_eq_true] at hok
  rcases hok with ⟨⟨⟨⟨hm_b, _⟩, hrect_b⟩, _⟩, hgap_b⟩
  have hm : m ≤ n := of_decide_eq_true hm_b
  have hrect : m < n → ∀ j : Fin n, v j ≤ 0 := by
    intro hlt j
    simp only [hlt, ↓reduceIte, allFin_eq_true] at hrect_b
    exact of_decide_eq_true (hrect_b j)
  have hlb : safeLowerBound m n C v ≤ finSum m (fun i => C i (q i)) :=
    uSafe_lower_bound hm C v hrect q hq
  have hgap : finSum m (fun i => C i (p i)) - safeLowerBound m n C v ≤ tol :=
    of_decide_eq_true hgap_b
  have hstep1 : finSum m (fun i => C i (p i)) ≤ tol + safeLowerBound m n C v := by
    have hadd := Rat.add_le_add_right (c := safeLowerBound m n C v).mpr hgap
    rwa [Rat.sub_add_cancel] at hadd
  have hstep2 : finSum m (fun i => C i (p i)) - tol ≤ safeLowerBound m n C v := by
    have hadd := Rat.add_le_add_right (c := -tol).mpr hstep1
    rw [Rat.add_comm tol (safeLowerBound m n C v), Rat.add_assoc, Rat.add_neg_cancel,
      Rat.add_zero, ← Rat.sub_eq_add_neg] at hadd
    exact hadd
  exact ⟨hlb, Rat.le_trans hstep2 hlb⟩

/-- Corollary of `checkLSAPSafe_sound` for maximization problems encoded via negated costs. -/
theorem checkLSAPSafe_sound_max {m n : Nat} {C : Fin m → Fin n → Rat}
    {p : Fin m → Fin n} {v : Fin n → Rat} {tol : Rat}
    (hok : checkLSAPSafe m n (fun i j => -C i j) p v tol = true)
    (q : Fin m → Fin n) (hq : ∀ i₁ i₂, q i₁ = q i₂ → i₁ = i₂) :
    finSum m (fun i => C i (q i)) ≤ -safeLowerBound m n (fun i j => -C i j) v ∧
    finSum m (fun i => C i (q i)) ≤ finSum m (fun i => C i (p i)) + tol := by
  have ⟨hlb, hgap⟩ := checkLSAPSafe_sound hok q hq
  rw [finSum_neg] at hlb
  have h1 : finSum m (fun i => C i (q i)) ≤ -safeLowerBound m n (fun i j => -C i j) v :=
    Rat.le_neg_iff.mp hlb
  rw [finSum_neg, finSum_neg] at hgap
  have hneg : -finSum m (fun i => C i (p i)) - tol ≤ -finSum m (fun i => C i (q i)) := hgap
  have hstep : - (finSum m (fun i => C i (p i)) + tol) ≤ -finSum m (fun i => C i (q i)) := by
    simpa [Rat.neg_add, Rat.sub_eq_add_neg] using hneg
  exact ⟨h1, Rat.neg_le_neg_iff.mp hstep⟩

/-! ### Transpose Checkers (`n ≤ m`, covering tall `m > n` matrices) -/

/-- Exact LSAP optimality checker for tall matrices (`n ≤ m`, in particular `m > n`),
matching each column `j : Fin n` to a distinct row `p j : Fin m` with row duals `u`
and column duals `v`. -/
def checkLSAPTranspose (m n : Nat) (C : Fin m → Fin n → Rat)
    (p : Fin n → Fin m) (u : Fin m → Rat) (v : Fin n → Rat) : Bool :=
  checkLSAP n m (fun j i => C i j) p v u

theorem checkLSAPTranspose_injective {m n : Nat} {C : Fin m → Fin n → Rat}
    {p : Fin n → Fin m} {u : Fin m → Rat} {v : Fin n → Rat}
    (hok : checkLSAPTranspose m n C p u v = true) :
    ∀ j₁ j₂ : Fin n, p j₁ = p j₂ → j₁ = j₂ := by
  unfold checkLSAPTranspose checkLSAP at hok
  simp only [Bool.and_eq_true] at hok
  exact checkInjective_iff.mp hok.1.1.1.2

/-- Soundness of `checkLSAPTranspose`: every valid tall-matrix certificate guarantees
global optimality over all injective column-to-row matchings `q : Fin n → Fin m`. -/
theorem checkLSAPTranspose_sound {m n : Nat} {C : Fin m → Fin n → Rat}
    {p : Fin n → Fin m} {u : Fin m → Rat} {v : Fin n → Rat}
    (hok : checkLSAPTranspose m n C p u v = true)
    (q : Fin n → Fin m) (hq : ∀ j₁ j₂, q j₁ = q j₂ → j₁ = j₂) :
    finSum n (fun j => C (p j) j) ≤ finSum n (fun j => C (q j) j) :=
  checkLSAP_sound hok q hq

/-- Maximization corollary of `checkLSAPTranspose_sound` via negated costs. -/
theorem checkLSAPTranspose_sound_max {m n : Nat} {C : Fin m → Fin n → Rat}
    {p : Fin n → Fin m} {u : Fin m → Rat} {v : Fin n → Rat}
    (hok : checkLSAPTranspose m n (fun i j => -C i j) p u v = true)
    (q : Fin n → Fin m) (hq : ∀ j₁ j₂, q j₁ = q j₂ → j₁ = j₂) :
    finSum n (fun j => C (q j) j) ≤ finSum n (fun j => C (p j) j) :=
  checkLSAP_sound_max (C := fun j i => C i j) hok q hq

/-- Safe LSAP lower-bound verifier for tall matrices (`n ≤ m`) using row duals `u`. -/
def checkLSAPTransposeSafe (m n : Nat) (C : Fin m → Fin n → Rat)
    (p : Fin n → Fin m) (u : Fin m → Rat) (tol : Rat) : Bool :=
  checkLSAPSafe n m (fun j i => C i j) p u tol

/-- Exact rational dual lower bound for tall matrices induced by row duals `u`. -/
def safeLowerBoundTranspose (m n : Nat) (C : Fin m → Fin n → Rat) (u : Fin m → Rat) : Rat :=
  safeLowerBound n m (fun j i => C i j) u

theorem checkLSAPTransposeSafe_sound {m n : Nat} {C : Fin m → Fin n → Rat}
    {p : Fin n → Fin m} {u : Fin m → Rat} {tol : Rat}
    (hok : checkLSAPTransposeSafe m n C p u tol = true)
    (q : Fin n → Fin m) (hq : ∀ j₁ j₂, q j₁ = q j₂ → j₁ = j₂) :
    safeLowerBoundTranspose m n C u ≤ finSum n (fun j => C (q j) j) ∧
    finSum n (fun j => C (p j) j) - tol ≤ finSum n (fun j => C (q j) j) :=
  checkLSAPSafe_sound hok q hq

theorem checkLSAPTransposeSafe_sound_max {m n : Nat} {C : Fin m → Fin n → Rat}
    {p : Fin n → Fin m} {u : Fin m → Rat} {tol : Rat}
    (hok : checkLSAPTransposeSafe m n (fun i j => -C i j) p u tol = true)
    (q : Fin n → Fin m) (hq : ∀ j₁ j₂, q j₁ = q j₂ → j₁ = j₂) :
    finSum n (fun j => C (q j) j) ≤ -safeLowerBoundTranspose m n (fun i j => -C i j) u ∧
    finSum n (fun j => C (q j) j) ≤ finSum n (fun j => C (p j) j) + tol :=
  checkLSAPSafe_sound_max (C := fun j i => C i j) hok q hq

/-! ### Forbidden-Edge LSAP (`Option Rat`, where `none` denotes forbidden `±∞`) -/

/-- Exact rational cost sum of a matching `q : Fin m → Fin n` on an `Option Rat` matrix
(for finite-edge matchings, every `(C i (q i)).getD 0` is the exact edge weight). -/
def optCostSum (m n : Nat) (C : Fin m → Fin n → Option Rat) (q : Fin m → Fin n) : Rat :=
  finSum m (fun i => (C i (q i)).getD 0)

/-- Negates finite costs of an `Option Rat` matrix while preserving forbidden (`none`) cells. -/
def negOptCost {m n : Nat} (C : Fin m → Fin n → Option Rat) : Fin m → Fin n → Option Rat :=
  fun i j => (C i j).map (- ·)

theorem negOptCost_isSome {m n : Nat} (C : Fin m → Fin n → Option Rat) (i : Fin m) (j : Fin n) :
    (negOptCost C i j).isSome = (C i j).isSome := by
  unfold negOptCost
  cases C i j <;> rfl

theorem optCostSum_negOptCost {m n : Nat} (C : Fin m → Fin n → Option Rat) (q : Fin m → Fin n) :
    optCostSum m n (negOptCost C) q = -optCostSum m n C q := by
  unfold optCostSum negOptCost
  rw [← finSum_neg]
  apply finSum_congr
  intro i
  cases C i (q i) with
  | none => simp
  | some c => rfl

/-- Checks dual feasibility on all finite edges: `∀ i j c, C i j = some c → u i + v j ≤ c`. -/
def checkDualFeasibleOpt (m n : Nat) (C : Fin m → Fin n → Option Rat)
    (u : Fin m → Rat) (v : Fin n → Rat) : Bool :=
  allFin m (fun i => allFin n (fun j =>
    match C i j with
    | none => true
    | some c => decide (u i + v j ≤ c)))

theorem checkDualFeasibleOpt_iff {m n : Nat} {C : Fin m → Fin n → Option Rat}
    {u : Fin m → Rat} {v : Fin n → Rat} :
    checkDualFeasibleOpt m n C u v = true ↔
      ∀ (i : Fin m) (j : Fin n) (c : Rat), C i j = some c → u i + v j ≤ c := by
  unfold checkDualFeasibleOpt
  rw [allFin_eq_true]
  constructor
  · intro h i j c hcij
    have hij := (allFin_eq_true.mp (h i)) j
    rw [hcij] at hij
    exact of_decide_eq_true hij
  · intro h i
    rw [allFin_eq_true]
    intro j
    cases hcij : C i j with
    | none => rfl
    | some c => exact decide_eq_true (h i j c hcij)

/-- Checks that `p` uses only finite edges and satisfies complementary slackness on each matched edge. -/
def checkCompSlackOpt (m n : Nat) (C : Fin m → Fin n → Option Rat)
    (p : Fin m → Fin n) (u : Fin m → Rat) (v : Fin n → Rat) : Bool :=
  allFin m (fun i =>
    match C i (p i) with
    | none => false
    | some c => decide (u i + v (p i) = c))

theorem checkCompSlackOpt_iff {m n : Nat} {C : Fin m → Fin n → Option Rat}
    {p : Fin m → Fin n} {u : Fin m → Rat} {v : Fin n → Rat} :
    checkCompSlackOpt m n C p u v = true ↔
      (∀ i : Fin m, (C i (p i)).isSome = true) ∧
      (∀ i : Fin m, u i + v (p i) = (C i (p i)).getD 0) := by
  unfold checkCompSlackOpt
  rw [allFin_eq_true]
  constructor
  · intro h
    constructor
    · intro i
      have hi := h i
      cases hc : C i (p i) with
      | none => rw [hc] at hi; contradiction
      | some c => rfl
    · intro i
      have hi := h i
      cases hc : C i (p i) with
      | none => rw [hc] at hi; contradiction
      | some c =>
        rw [hc] at hi
        exact of_decide_eq_true hi
  · rintro ⟨hfin, hcs⟩ i
    have hfi := hfin i
    have hci := hcs i
    cases hc : C i (p i) with
    | none => rw [hc] at hfi; contradiction
    | some c =>
      rw [hc] at hci
      exact decide_eq_true hci

/-- Checks that every matched edge `C i (p i)` is finite (`some`). -/
def checkFiniteMatching (m n : Nat) (C : Fin m → Fin n → Option Rat) (p : Fin m → Fin n) : Bool :=
  allFin m (fun i => (C i (p i)).isSome)

theorem checkFiniteMatching_iff {m n : Nat} {C : Fin m → Fin n → Option Rat} {p : Fin m → Fin n} :
    checkFiniteMatching m n C p = true ↔ ∀ i : Fin m, (C i (p i)).isSome = true := by
  unfold checkFiniteMatching
  exact allFin_eq_true

/-- Exact LSAP optimality checker over `Option Rat` costs (`m ≤ n`), natively supporting
forbidden (`none`) edges without big-M approximations. -/
def checkLSAPForbidden (m n : Nat) (C : Fin m → Fin n → Option Rat)
    (p : Fin m → Fin n) (u : Fin m → Rat) (v : Fin n → Rat) : Bool :=
  decide (m ≤ n) &&
  checkInjective m n p &&
  checkDualFeasibleOpt m n C u v &&
  checkRectDual m n p v &&
  checkCompSlackOpt m n C p u v

/-- Weak duality lower bound on an `Option Rat` matrix over all finite-edge injective matchings. -/
theorem dual_lower_bound_forbidden {m n : Nat} (hm : m ≤ n)
    (C : Fin m → Fin n → Option Rat) (u : Fin m → Rat) (v : Fin n → Rat)
    (hfeas : ∀ (i : Fin m) (j : Fin n) (c : Rat), C i j = some c → u i + v j ≤ c)
    (hrect : m < n → ∀ j : Fin n, v j ≤ 0)
    (q : Fin m → Fin n)
    (hq : ∀ i₁ i₂, q i₁ = q i₂ → i₁ = i₂)
    (hfin : ∀ i : Fin m, (C i (q i)).isSome = true) :
    finSum m u + finSum n v ≤ optCostSum m n C q := by
  have hv_le : finSum n v ≤ finSum m (fun i => v (q i)) := by
    rcases Nat.eq_or_lt_of_le hm with rfl | hlt
    · rw [finSum_inj_eq hq]
      exact Rat.le_refl
    · exact finSum_inj_nonpos_ge (hrect hlt) hq
  have hsum_le :
      finSum m u + finSum n v ≤ finSum m u + finSum m (fun i => v (q i)) :=
    Rat.add_le_add_left.mpr hv_le
  have hfeas_sum :
      finSum m u + finSum m (fun i => v (q i)) ≤ optCostSum m n C q := by
    unfold optCostSum
    rw [← finSum_add]
    apply finSum_le_finSum
    intro i
    have hfi := hfin i
    cases hc : C i (q i) with
    | none => rw [hc] at hfi; contradiction
    | some c => exact hfeas i (q i) c hc
  exact Rat.le_trans hsum_le hfeas_sum

/-- Soundness of `checkLSAPForbidden`: proves `p` is injective, uses only finite edges,
and minimizes cost over all injective finite-edge matchings `q : Fin m → Fin n`. -/
theorem checkLSAPForbidden_sound {m n : Nat} {C : Fin m → Fin n → Option Rat}
    {p : Fin m → Fin n} {u : Fin m → Rat} {v : Fin n → Rat}
    (hok : checkLSAPForbidden m n C p u v = true) :
    (∀ i₁ i₂ : Fin m, p i₁ = p i₂ → i₁ = i₂) ∧
    (∀ i : Fin m, (C i (p i)).isSome = true) ∧
    (∀ (q : Fin m → Fin n),
      (∀ i₁ i₂, q i₁ = q i₂ → i₁ = i₂) →
      (∀ i : Fin m, (C i (q i)).isSome = true) →
      optCostSum m n C p ≤ optCostSum m n C q) := by
  unfold checkLSAPForbidden at hok
  simp only [Bool.and_eq_true] at hok
  rcases hok with ⟨⟨⟨⟨hm_b, hinj_b⟩, hfeas_b⟩, hrect_b⟩, hcs_b⟩
  have hm : m ≤ n := of_decide_eq_true hm_b
  have hp : ∀ i₁ i₂ : Fin m, p i₁ = p i₂ → i₁ = i₂ := checkInjective_iff.mp hinj_b
  have hfeas := checkDualFeasibleOpt_iff.mp hfeas_b
  have hrect := checkRectDual_iff.mp hrect_b
  have ⟨hpFin, hcs⟩ := checkCompSlackOpt_iff.mp hcs_b
  have hv_eq : finSum m (fun i => v (p i)) = finSum n v := by
    rcases Nat.eq_or_lt_of_le hm with rfl | hlt
    · exact finSum_inj_eq hp
    · exact finSum_inj_eq_of_unassigned_zero hp (hrect hlt).2
  have hprimal : optCostSum m n C p = finSum m u + finSum n v := by
    unfold optCostSum
    have hcongr :
        finSum m (fun i => (C i (p i)).getD 0) = finSum m (fun i => u i + v (p i)) :=
      finSum_congr (fun i => (hcs i).symm)
    rw [hcongr, finSum_add, hv_eq]
  refine ⟨hp, hpFin, fun q hq hqFin => ?_⟩
  rw [hprimal]
  exact dual_lower_bound_forbidden hm C u v hfeas (fun h => (hrect h).1) q hq hqFin

/-- Maximization corollary of `checkLSAPForbidden_sound` over negated `Option Rat` costs. -/
theorem checkLSAPForbidden_sound_max {m n : Nat} {C : Fin m → Fin n → Option Rat}
    {p : Fin m → Fin n} {u : Fin m → Rat} {v : Fin n → Rat}
    (hok : checkLSAPForbidden m n (negOptCost C) p u v = true) :
    (∀ i₁ i₂ : Fin m, p i₁ = p i₂ → i₁ = i₂) ∧
    (∀ i : Fin m, (C i (p i)).isSome = true) ∧
    (∀ (q : Fin m → Fin n),
      (∀ i₁ i₂, q i₁ = q i₂ → i₁ = i₂) →
      (∀ i : Fin m, (C i (q i)).isSome = true) →
      optCostSum m n C q ≤ optCostSum m n C p) := by
  have ⟨hp, hpFin, hopt⟩ := checkLSAPForbidden_sound hok
  refine ⟨hp, fun i => ?_, fun q hq hqFin => ?_⟩
  · simpa [negOptCost_isSome] using hpFin i
  · have hqNeg : ∀ i : Fin m, (negOptCost C i (q i)).isSome = true := by
      intro i; simpa [negOptCost_isSome] using hqFin i
    have hmin := hopt q hq hqNeg
    rw [optCostSum_negOptCost, optCostSum_negOptCost, Rat.neg_le_neg_iff] at hmin
    exact hmin

/-- Tail-recursive minimum over `Fin n` ignoring `none` entries. -/
def minFinOptLoop (n : Nat) (f : Fin n → Option Rat) (k : Nat) (acc : Option Rat) : Option Rat :=
  if h : k < n then
    let nextAcc :=
      match acc, f ⟨k, h⟩ with
      | none, cur => cur
      | some a, none => some a
      | some a, some c => some (if a ≤ c then a else c)
    minFinOptLoop n f (k + 1) nextAcc
  else
    acc
termination_by n - k

private theorem minFinOptLoop_le_acc_aux (n : Nat) (f : Fin n → Option Rat) (d k : Nat)
    (a : Rat) (hd : n - k = d) :
    ∀ r : Rat, minFinOptLoop n f k (some a) = some r → r ≤ a := by
  induction d generalizing k a with
  | zero =>
    intro r hr
    have hge : ¬(k < n) := by omega
    rw [minFinOptLoop] at hr
    simp only [hge, ↓reduceDIte] at hr
    rw [← Option.some.inj hr]
    exact Rat.le_refl
  | succ d ih =>
    intro r hr
    by_cases hkn : k < n
    · rw [minFinOptLoop] at hr
      simp only [hkn, ↓reduceDIte] at hr
      cases hfk : f ⟨k, hkn⟩ with
      | none =>
        rw [hfk] at hr
        exact ih (k + 1) a (by omega) r hr
      | some c =>
        rw [hfk] at hr
        let a' := if a ≤ c then a else c
        have h1 : r ≤ a' := ih (k + 1) a' (by omega) r hr
        have h2 : a' ≤ a := by
          dsimp [a']
          split
          · exact Rat.le_refl
          · next hnot =>
            rcases Rat.le_total (a := a) (b := c) with hle | hge
            · exact absurd hle hnot
            · exact hge
        exact Rat.le_trans h1 h2
    · rw [minFinOptLoop] at hr
      simp only [hkn, ↓reduceDIte] at hr
      rw [← Option.some.inj hr]
      exact Rat.le_refl

private theorem minFinOptLoop_le_elem_aux (n : Nat) (f : Fin n → Option Rat) (d k : Nat)
    (acc : Option Rat) (hd : n - k = d) :
    ∀ (j : Fin n) (c r : Rat), k ≤ j.val → f j = some c →
      minFinOptLoop n f k acc = some r → r ≤ c := by
  induction d generalizing k acc with
  | zero =>
    intro j _ _ hj _ _
    omega
  | succ d ih =>
    intro j c r hj hfj hr
    by_cases hkn : k < n
    · rw [minFinOptLoop] at hr
      simp only [hkn, ↓reduceDIte] at hr
      by_cases heq : j.val = k
      · have hfk : f ⟨k, hkn⟩ = some c := by
          have : (⟨k, hkn⟩ : Fin n) = j := Fin.ext heq.symm
          rw [this, hfj]
        rw [hfk] at hr
        cases acc with
        | none =>
          exact minFinOptLoop_le_acc_aux n f (n - (k + 1)) (k + 1) c rfl r hr
        | some a =>
          let a' := if a ≤ c then a else c
          have h1 : r ≤ a' := minFinOptLoop_le_acc_aux n f (n - (k + 1)) (k + 1) a' rfl r hr
          have h2 : a' ≤ c := by
            dsimp [a']
            split
            · next hle => exact hle
            · exact Rat.le_refl
          exact Rat.le_trans h1 h2
      · exact ih (k + 1) _ (by omega) j c r (by omega) hfj hr
    · omega

/-- Safe row dual potential over finite edges: `uSafeOpt i = min_{j : C i j = some c} (c - v j)`. -/
def uSafeOpt (m n : Nat) (C : Fin m → Fin n → Option Rat) (v : Fin n → Rat) (i : Fin m) :
    Option Rat :=
  minFinOptLoop n (fun j => (C i j).map (fun c => c - v j)) 0 none

theorem uSafeOpt_add_le {m n : Nat} (C : Fin m → Fin n → Option Rat) (v : Fin n → Rat)
    {i : Fin m} {u_i : Rat} (hu : uSafeOpt m n C v i = some u_i)
    (j : Fin n) (c : Rat) (hcij : C i j = some c) :
    u_i + v j ≤ c := by
  unfold uSafeOpt at hu
  have hfj : ((C i j).map (fun x => x - v j)) = some (c - v j) := by
    rw [hcij]; rfl
  have hdiff : u_i ≤ c - v j :=
    minFinOptLoop_le_elem_aux n (fun k => (C i k).map (fun x => x - v k)) n 0 none (Nat.sub_zero n)
      j (c - v j) u_i (Nat.zero_le _) hfj hu
  have hadd := Rat.add_le_add_right (c := v j).mpr hdiff
  rwa [Rat.sub_add_cancel] at hadd

/-- Finite sum of `Option Rat` terms, returning `none` if any term is `none`. -/
def finSumOpt : (n : Nat) → (Fin n → Option Rat) → Option Rat
  | 0, _ => some 0
  | n + 1, f =>
    match finSumOpt n (fun i => f i.castSucc), f (Fin.last n) with
    | some s, some x => some (s + x)
    | _, _ => none

theorem finSumOpt_eq_some_iff {n : Nat} {f : Fin n → Option Rat} {s : Rat}
    (hs : finSumOpt n f = some s) :
    (∀ i : Fin n, (f i).isSome = true) ∧
    s = finSum n (fun i => (f i).getD 0) := by
  induction n generalizing s with
  | zero =>
    simp only [finSumOpt] at hs
    rw [← Option.some.inj hs]
    exact ⟨fun i => Fin.elim0 i, rfl⟩
  | succ n ih =>
    simp only [finSumOpt] at hs
    cases h1 : finSumOpt n (fun i => f i.castSucc) with
    | none => rw [h1] at hs; contradiction
    | some s0 =>
      rw [h1] at hs
      cases h2 : f (Fin.last n) with
      | none => rw [h2] at hs; contradiction
      | some x =>
        rw [h2] at hs
        have ⟨hfin0, hs0⟩ := ih h1
        have hsEq : s = s0 + x := (Option.some.inj hs).symm
        constructor
        · intro i
          by_cases hil : i = Fin.last n
          · subst hil; rw [h2]; rfl
          · have hilt : i.val < n := by
              have := i.isLt
              have hne : i.val ≠ n := fun hv => hil (Fin.ext hv)
              omega
            have hiEq : i = (⟨i.val, hilt⟩ : Fin n).castSucc := Fin.ext rfl
            rw [hiEq]
            exact hfin0 ⟨i.val, hilt⟩
        · rw [hsEq, finSum_succ, hs0, h2]
          rfl

/-- Exact rational dual lower bound for an `Option Rat` cost matrix induced by `v` and `uSafeOpt`. -/
def safeLowerBoundOpt (m n : Nat) (C : Fin m → Fin n → Option Rat) (v : Fin n → Rat) : Option Rat :=
  (finSumOpt m (uSafeOpt m n C v)).map (fun su => su + finSum n v)

theorem uSafeOpt_lower_bound {m n : Nat} (hm : m ≤ n) (C : Fin m → Fin n → Option Rat)
    (v : Fin n → Rat) {lb : Rat} (hlb : safeLowerBoundOpt m n C v = some lb)
    (hrect : m < n → ∀ j : Fin n, v j ≤ 0)
    (q : Fin m → Fin n)
    (hq : ∀ i₁ i₂, q i₁ = q i₂ → i₁ = i₂)
    (hfin : ∀ i : Fin m, (C i (q i)).isSome = true) :
    lb ≤ optCostSum m n C q := by
  unfold safeLowerBoundOpt at hlb
  cases hsu : finSumOpt m (uSafeOpt m n C v) with
  | none => rw [hsu] at hlb; contradiction
  | some su =>
    rw [hsu] at hlb
    have hlbEq : lb = su + finSum n v := (Option.some.inj hlb).symm
    have ⟨huSome, hsuEq⟩ := finSumOpt_eq_some_iff hsu
    let uFn : Fin m → Rat := fun i => (uSafeOpt m n C v i).getD 0
    have hfeas : ∀ (i : Fin m) (j : Fin n) (c : Rat), C i j = some c → uFn i + v j ≤ c := by
      intro i j c hcij
      dsimp [uFn]
      have hsi := huSome i
      cases hui : uSafeOpt m n C v i with
      | none => rw [hui] at hsi; contradiction
      | some ui => exact uSafeOpt_add_le C v hui j c hcij
    rw [hlbEq, hsuEq]
    exact dual_lower_bound_forbidden hm C uFn v hfeas hrect q hq hfin

/-- Safe LSAP verifier over `Option Rat` costs (`m ≤ n`), checking injectivity,
finite matched edges, rectangular dual sign, and gap `primal(p) - lb ≤ tol`. -/
def checkLSAPForbiddenSafe (m n : Nat) (C : Fin m → Fin n → Option Rat)
    (p : Fin m → Fin n) (v : Fin n → Rat) (tol : Rat) : Bool :=
  decide (m ≤ n) &&
  checkInjective m n p &&
  checkFiniteMatching m n C p &&
  (if m < n then allFin n (fun j => decide (v j ≤ 0)) else true) &&
  decide (0 ≤ tol) &&
  match safeLowerBoundOpt m n C v with
  | none => false
  | some lb => decide (optCostSum m n C p - lb ≤ tol)

/-- Soundness of `checkLSAPForbiddenSafe`: proves `p` is an injective finite-edge matching,
exhibits the certified lower bound `lb`, and proves `lb ≤ opt(q)` and `cost(p) - tol ≤ cost(q)`
for every injective finite-edge matching `q`. -/
theorem checkLSAPForbiddenSafe_sound {m n : Nat} {C : Fin m → Fin n → Option Rat}
    {p : Fin m → Fin n} {v : Fin n → Rat} {tol : Rat}
    (hok : checkLSAPForbiddenSafe m n C p v tol = true) :
    (∀ i₁ i₂ : Fin m, p i₁ = p i₂ → i₁ = i₂) ∧
    (∀ i : Fin m, (C i (p i)).isSome = true) ∧
    (∃ lb : Rat, safeLowerBoundOpt m n C v = some lb ∧
      ∀ (q : Fin m → Fin n),
        (∀ i₁ i₂, q i₁ = q i₂ → i₁ = i₂) →
        (∀ i : Fin m, (C i (q i)).isSome = true) →
        lb ≤ optCostSum m n C q ∧ optCostSum m n C p - tol ≤ optCostSum m n C q) := by
  unfold checkLSAPForbiddenSafe at hok
  simp only [Bool.and_eq_true] at hok
  rcases hok with ⟨⟨⟨⟨⟨hm_b, hinj_b⟩, hfin_b⟩, hrect_b⟩, _⟩, hlb_b⟩
  have hm : m ≤ n := of_decide_eq_true hm_b
  have hp : ∀ i₁ i₂ : Fin m, p i₁ = p i₂ → i₁ = i₂ := checkInjective_iff.mp hinj_b
  have hpFin : ∀ i : Fin m, (C i (p i)).isSome = true := checkFiniteMatching_iff.mp hfin_b
  have hrect : m < n → ∀ j : Fin n, v j ≤ 0 := by
    intro hlt j
    simp only [hlt, ↓reduceIte, allFin_eq_true] at hrect_b
    exact of_decide_eq_true (hrect_b j)
  cases hlbEq : safeLowerBoundOpt m n C v with
  | none => rw [hlbEq] at hlb_b; contradiction
  | some lb =>
    rw [hlbEq] at hlb_b
    have hgap : optCostSum m n C p - lb ≤ tol := of_decide_eq_true hlb_b
    have hstep1 : optCostSum m n C p ≤ tol + lb := by
      have hadd := Rat.add_le_add_right (c := lb).mpr hgap
      rwa [Rat.sub_add_cancel] at hadd
    have hstep2 : optCostSum m n C p - tol ≤ lb := by
      have hadd := Rat.add_le_add_right (c := -tol).mpr hstep1
      rw [Rat.add_comm tol lb, Rat.add_assoc, Rat.add_neg_cancel,
        Rat.add_zero, ← Rat.sub_eq_add_neg] at hadd
      exact hadd
    refine ⟨hp, hpFin, ⟨lb, rfl, fun q hq hqFin => ?_⟩⟩
    have hqLb := uSafeOpt_lower_bound hm C v hlbEq hrect q hq hqFin
    exact ⟨hqLb, Rat.le_trans hstep2 hqLb⟩

/-- Maximization corollary of `checkLSAPForbiddenSafe_sound`. -/
theorem checkLSAPForbiddenSafe_sound_max {m n : Nat} {C : Fin m → Fin n → Option Rat}
    {p : Fin m → Fin n} {v : Fin n → Rat} {tol : Rat}
    (hok : checkLSAPForbiddenSafe m n (negOptCost C) p v tol = true) :
    (∀ i₁ i₂ : Fin m, p i₁ = p i₂ → i₁ = i₂) ∧
    (∀ i : Fin m, (C i (p i)).isSome = true) ∧
    (∃ lb : Rat, safeLowerBoundOpt m n (negOptCost C) v = some lb ∧
      ∀ (q : Fin m → Fin n),
        (∀ i₁ i₂, q i₁ = q i₂ → i₁ = i₂) →
        (∀ i : Fin m, (C i (q i)).isSome = true) →
        optCostSum m n C q ≤ -lb ∧ optCostSum m n C q ≤ optCostSum m n C p + tol) := by
  have ⟨hp, hpFin, ⟨lb, hlbEq, hopt⟩⟩ := checkLSAPForbiddenSafe_sound hok
  refine ⟨hp, fun i => ?_, ⟨lb, hlbEq, fun q hq hqFin => ?_⟩⟩
  · simpa [negOptCost_isSome] using hpFin i
  · have hqNeg : ∀ i : Fin m, (negOptCost C i (q i)).isSome = true := by
      intro i; simpa [negOptCost_isSome] using hqFin i
    have ⟨hlb, hgap⟩ := hopt q hq hqNeg
    rw [optCostSum_negOptCost] at hlb
    have h1 : optCostSum m n C q ≤ -lb := Rat.le_neg_iff.mp hlb
    rw [optCostSum_negOptCost, optCostSum_negOptCost] at hgap
    have hstep : -(optCostSum m n C p + tol) ≤ -optCostSum m n C q := by
      simpa [Rat.neg_add, Rat.sub_eq_add_neg] using hgap
    exact ⟨h1, Rat.neg_le_neg_iff.mp hstep⟩

/-- Exact forbidden-edge LSAP checker for tall matrices (`n ≤ m`, in particular `m > n`). -/
def checkLSAPForbiddenTranspose (m n : Nat) (C : Fin m → Fin n → Option Rat)
    (p : Fin n → Fin m) (u : Fin m → Rat) (v : Fin n → Rat) : Bool :=
  checkLSAPForbidden n m (fun j i => C i j) p v u

theorem checkLSAPForbiddenTranspose_sound {m n : Nat} {C : Fin m → Fin n → Option Rat}
    {p : Fin n → Fin m} {u : Fin m → Rat} {v : Fin n → Rat}
    (hok : checkLSAPForbiddenTranspose m n C p u v = true) :
    (∀ j₁ j₂ : Fin n, p j₁ = p j₂ → j₁ = j₂) ∧
    (∀ j : Fin n, (C (p j) j).isSome = true) ∧
    (∀ (q : Fin n → Fin m),
      (∀ j₁ j₂, q j₁ = q j₂ → j₁ = j₂) →
      (∀ j : Fin n, (C (q j) j).isSome = true) →
      finSum n (fun j => (C (p j) j).getD 0) ≤ finSum n (fun j => (C (q j) j).getD 0)) :=
  checkLSAPForbidden_sound hok

theorem checkLSAPForbiddenTranspose_sound_max {m n : Nat} {C : Fin m → Fin n → Option Rat}
    {p : Fin n → Fin m} {u : Fin m → Rat} {v : Fin n → Rat}
    (hok : checkLSAPForbiddenTranspose m n (negOptCost C) p u v = true) :
    (∀ j₁ j₂ : Fin n, p j₁ = p j₂ → j₁ = j₂) ∧
    (∀ j : Fin n, (C (p j) j).isSome = true) ∧
    (∀ (q : Fin n → Fin m),
      (∀ j₁ j₂, q j₁ = q j₂ → j₁ = j₂) →
      (∀ j : Fin n, (C (q j) j).isSome = true) →
      finSum n (fun j => (C (q j) j).getD 0) ≤ finSum n (fun j => (C (p j) j).getD 0)) :=
  checkLSAPForbidden_sound_max (C := fun j i => C i j) hok

/-- Safe forbidden-edge LSAP checker for tall matrices (`n ≤ m`) using row duals `u`. -/
def checkLSAPForbiddenTransposeSafe (m n : Nat) (C : Fin m → Fin n → Option Rat)
    (p : Fin n → Fin m) (u : Fin m → Rat) (tol : Rat) : Bool :=
  checkLSAPForbiddenSafe n m (fun j i => C i j) p u tol

/-- Exact rational dual lower bound for tall forbidden-edge matrices induced by row duals `u`. -/
def safeLowerBoundOptTranspose (m n : Nat) (C : Fin m → Fin n → Option Rat) (u : Fin m → Rat) :
    Option Rat :=
  safeLowerBoundOpt n m (fun j i => C i j) u

theorem checkLSAPForbiddenTransposeSafe_sound {m n : Nat} {C : Fin m → Fin n → Option Rat}
    {p : Fin n → Fin m} {u : Fin m → Rat} {tol : Rat}
    (hok : checkLSAPForbiddenTransposeSafe m n C p u tol = true) :
    (∀ j₁ j₂ : Fin n, p j₁ = p j₂ → j₁ = j₂) ∧
    (∀ j : Fin n, (C (p j) j).isSome = true) ∧
    (∃ lb : Rat, safeLowerBoundOptTranspose m n C u = some lb ∧
      ∀ (q : Fin n → Fin m),
        (∀ j₁ j₂, q j₁ = q j₂ → j₁ = j₂) →
        (∀ j : Fin n, (C (q j) j).isSome = true) →
        lb ≤ finSum n (fun j => (C (q j) j).getD 0) ∧
        finSum n (fun j => (C (p j) j).getD 0) - tol ≤ finSum n (fun j => (C (q j) j).getD 0)) :=
  checkLSAPForbiddenSafe_sound hok

theorem checkLSAPForbiddenTransposeSafe_sound_max {m n : Nat} {C : Fin m → Fin n → Option Rat}
    {p : Fin n → Fin m} {u : Fin m → Rat} {tol : Rat}
    (hok : checkLSAPForbiddenTransposeSafe m n (negOptCost C) p u tol = true) :
    (∀ j₁ j₂ : Fin n, p j₁ = p j₂ → j₁ = j₂) ∧
    (∀ j : Fin n, (C (p j) j).isSome = true) ∧
    (∃ lb : Rat, safeLowerBoundOptTranspose m n (negOptCost C) u = some lb ∧
      ∀ (q : Fin n → Fin m),
        (∀ j₁ j₂, q j₁ = q j₂ → j₁ = j₂) →
        (∀ j : Fin n, (C (q j) j).isSome = true) →
        finSum n (fun j => (C (q j) j).getD 0) ≤ -lb ∧
        finSum n (fun j => (C (q j) j).getD 0) ≤ finSum n (fun j => (C (p j) j).getD 0) + tol) :=
  checkLSAPForbiddenSafe_sound_max (C := fun j i => C i j) hok

/-! ### Hall's Marriage Theorem Infeasibility Verifier (`Option Rat`) -/

/-- Tail-recursive bounded counter of indices `i ∈ [start, k)` with `pred i = true`. -/
def countFinFrom (k : Nat) (pred : Fin k → Bool) (start acc : Nat) : Nat :=
  if h : start < k then
    countFinFrom k pred (start + 1) (if pred ⟨start, h⟩ then acc + 1 else acc)
  else
    acc
termination_by k - start

/-- Counts indices `i : Fin k` satisfying `pred i = true`. -/
def countFin (k : Nat) (pred : Fin k → Bool) : Nat :=
  countFinFrom k pred 0 0

private theorem countFinFrom_castSucc_aux (k : Nat) (pred : Fin (k + 1) → Bool)
    (d start acc : Nat) (hstart : start ≤ k) (hd : k - start = d) :
    countFinFrom (k + 1) pred start acc =
      countFinFrom k (fun i => pred i.castSucc) start acc +
        (if pred (Fin.last k) then 1 else 0) := by
  induction d generalizing start acc with
  | zero =>
    have heq : start = k := by omega
    subst heq
    rw [countFinFrom, countFinFrom]
    simp only [Nat.lt_succ_self, ↓reduceDIte, Nat.lt_irrefl]
    rw [countFinFrom]
    simp only [Nat.lt_irrefl, ↓reduceDIte]
    have hlast : (⟨start, Nat.lt_succ_self start⟩ : Fin (start + 1)) = Fin.last start := rfl
    rw [hlast]
    by_cases hp : pred (Fin.last start) = true <;> simp [hp]
  | succ d ih =>
    have hsk : start < k := by omega
    have hsk1 : start < k + 1 := by omega
    rw [countFinFrom, countFinFrom]
    simp only [hsk, hsk1, ↓reduceDIte]
    have hcast : (⟨start, hsk1⟩ : Fin (k + 1)) = (⟨start, hsk⟩ : Fin k).castSucc := rfl
    rw [hcast]
    exact ih (start + 1) _ (by omega) (by omega)

theorem countFin_eq_finSum (k : Nat) (pred : Fin k → Bool) :
    (countFin k pred : Rat) = finSum k (fun i => if pred i then (1 : Rat) else 0) := by
  induction k with
  | zero =>
    unfold countFin countFinFrom
    simp
  | succ k ih =>
    have hsucc :
        countFin (k + 1) pred =
          countFin k (fun i => pred i.castSucc) + (if pred (Fin.last k) then 1 else 0) := by
      unfold countFin
      exact countFinFrom_castSucc_aux k pred k 0 0 (Nat.zero_le k) (Nat.sub_zero k)
    rw [hsucc, finSum_succ, Rat.natCast_add, ih (fun i => pred i.castSucc)]
    by_cases hp : pred (Fin.last k) = true <;> simp [hp]

/-- True iff column `j : Fin n` has at least one finite edge from a row in `rowMask`. -/
def hasFiniteNeighborIn (m n : Nat) (C : Fin m → Fin n → Option Rat)
    (rowMask : Fin m → Bool) (j : Fin n) : Bool :=
  anyFin m (fun i => rowMask i && (C i j).isSome)

theorem hasFiniteNeighborIn_of_mem {m n : Nat} {C : Fin m → Fin n → Option Rat}
    {rowMask : Fin m → Bool} {i : Fin m} {j : Fin n}
    (hmem : rowMask i = true) (hfin : (C i j).isSome = true) :
    hasFiniteNeighborIn m n C rowMask j = true := by
  unfold hasFiniteNeighborIn
  cases h : anyFin m (fun r => rowMask r && (C r j).isSome) with
  | true => rfl
  | false =>
    have hfalse := anyFin_eq_false.mp h i
    simp [hmem, hfin] at hfalse

/-- Executable Hall's Marriage Theorem infeasibility certificate checker over `Option Rat` costs:
verifies `|N_finite(S)| < |S|` for the witness row subset `S = {i | rowMask i = true}`. -/
def checkHallInfeasible (m n : Nat) (C : Fin m → Fin n → Option Rat)
    (rowMask : Fin m → Bool) : Bool :=
  decide (countFin n (hasFiniteNeighborIn m n C rowMask) < countFin m rowMask)

/-- Soundness of `checkHallInfeasible`: if `|N_finite(S)| < |S|`, then no injective
finite-edge matching `q : Fin m → Fin n` exists. -/
theorem checkHallInfeasible_sound {m n : Nat} {C : Fin m → Fin n → Option Rat}
    {rowMask : Fin m → Bool}
    (hok : checkHallInfeasible m n C rowMask = true) :
    ¬ ∃ (q : Fin m → Fin n),
      (∀ i₁ i₂, q i₁ = q i₂ → i₁ = i₂) ∧
      (∀ i : Fin m, (C i (q i)).isSome = true) := by
  unfold checkHallInfeasible at hok
  have hlt : countFin n (hasFiniteNeighborIn m n C rowMask) < countFin m rowMask :=
    of_decide_eq_true hok
  rintro ⟨q, hqInj, hqFin⟩
  let g : Fin n → Rat := fun j => if hasFiniteNeighborIn m n C rowMask j then 1 else 0
  have hgNonneg : ∀ j : Fin n, 0 ≤ g j := by
    intro j
    dsimp [g]
    split
    · decide
    · exact Rat.le_refl
  have hpoint : ∀ i : Fin m, (if rowMask i then (1 : Rat) else 0) ≤ g (q i) := by
    intro i
    by_cases hm : rowMask i = true
    · simp only [hm, ↓reduceIte]
      have hneigh : hasFiniteNeighborIn m n C rowMask (q i) = true :=
        hasFiniteNeighborIn_of_mem hm (hqFin i)
      dsimp [g]
      simp only [hneigh, ↓reduceIte]
      exact Rat.le_refl
    · simp only [hm, Bool.false_eq_true, ↓reduceIte]
      exact hgNonneg (q i)
  have h1 :
      finSum m (fun i => if rowMask i then (1 : Rat) else 0) ≤ finSum m (fun i => g (q i)) :=
    finSum_le_finSum hpoint
  have h2 : finSum m (fun i => g (q i)) ≤ finSum n g :=
    finSum_inj_le hgNonneg hqInj
  have hleRat : (countFin m rowMask : Rat) ≤ (countFin n (hasFiniteNeighborIn m n C rowMask) : Rat) := by
    rw [countFin_eq_finSum, countFin_eq_finSum]
    exact Rat.le_trans h1 h2
  have hleNat : countFin m rowMask ≤ countFin n (hasFiniteNeighborIn m n C rowMask) :=
    Rat.natCast_le_natCast.mp hleRat
  omega

/-- Hall infeasibility checker for tall matrices (`m > n`), certifying via a column witness
subset `colMask : Fin n → Bool` that no injective finite matching `q : Fin n → Fin m` exists. -/
def checkHallInfeasibleTranspose (m n : Nat) (C : Fin m → Fin n → Option Rat)
    (colMask : Fin n → Bool) : Bool :=
  checkHallInfeasible n m (fun j i => C i j) colMask

theorem checkHallInfeasibleTranspose_sound {m n : Nat} {C : Fin m → Fin n → Option Rat}
    {colMask : Fin n → Bool}
    (hok : checkHallInfeasibleTranspose m n C colMask = true) :
    ¬ ∃ (q : Fin n → Fin m),
      (∀ j₁ j₂, q j₁ = q j₂ → j₁ = j₂) ∧
      (∀ j : Fin n, (C (q j) j).isSome = true) :=
  checkHallInfeasible_sound hok

/-- Objective error bound for safe mode: if `lb ≤ opt ≤ primal` and the declared objective `obj`
satisfies `primal - tol ≤ obj ≤ lb + tol`, then `|obj - opt| ≤ tol` (eliminating `2 * tol` drift). -/
theorem safe_objective_within_tol {lb opt primal obj tol : Rat}
    (hlb : lb ≤ opt) (hopt : opt ≤ primal)
    (hlo : primal - tol ≤ obj) (hhi : obj ≤ lb + tol) :
    (obj - opt).abs ≤ tol := by
  have h1 : obj - opt ≤ tol := by
    have hsub : obj - opt ≤ (lb + tol) - lb := by
      have ha : obj - opt ≤ (lb + tol) - opt := by
        rw [Rat.sub_eq_add_neg, Rat.sub_eq_add_neg]
        exact Rat.add_le_add_right.mpr hhi
      have hb : (lb + tol) - opt ≤ (lb + tol) - lb := by
        rw [Rat.sub_eq_add_neg, Rat.sub_eq_add_neg]
        exact Rat.add_le_add_left.mpr (Rat.neg_le_neg_iff.mpr hlb)
      exact Rat.le_trans ha hb
    have hsimp : (lb + tol) - lb = tol := by
      rw [Rat.add_comm lb tol, Rat.add_sub_cancel]
    rwa [hsimp] at hsub
  have h2 : -(obj - opt) ≤ tol := by
    have hneg : -(obj - opt) = opt - obj := by
      rw [Rat.sub_eq_add_neg, Rat.neg_add, Rat.neg_neg, Rat.add_comm, ← Rat.sub_eq_add_neg]
    rw [hneg]
    have hsub : opt - obj ≤ primal - (primal - tol) := by
      have ha : opt - obj ≤ primal - obj := by
        rw [Rat.sub_eq_add_neg, Rat.sub_eq_add_neg]
        exact Rat.add_le_add_right.mpr hopt
      have hb : primal - obj ≤ primal - (primal - tol) := by
        rw [Rat.sub_eq_add_neg, Rat.sub_eq_add_neg]
        exact Rat.add_le_add_left.mpr (Rat.neg_le_neg_iff.mpr hlo)
      exact Rat.le_trans ha hb
    have hsimp : primal - (primal - tol) = tol := by
      rw [Rat.sub_eq_add_neg, Rat.sub_eq_add_neg, Rat.neg_add, Rat.neg_neg,
        ← Rat.add_assoc, Rat.add_neg_cancel, Rat.zero_add]
    rwa [hsimp] at hsub
  rcases Rat.le_total (a := 0) (b := obj - opt) with hpos | hnonpos
  · rw [Rat.abs_of_nonneg hpos]
    exact h1
  · rw [Rat.abs_of_nonpos hnonpos]
    exact h2

end GoblinFormal.Track1
