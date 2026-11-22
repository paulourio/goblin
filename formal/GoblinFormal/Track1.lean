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

#print axioms checkLSAP_sound
#print axioms checkLSAPSafe_sound

end GoblinFormal.Track1
