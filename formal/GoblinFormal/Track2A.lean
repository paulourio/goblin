/-
GoblinFormal.Track2A: formal specifications and proofs for Track 2A presolve rules:
1. Integer equality GCD infeasibility and truncated divCeil.
2. Row activity bound tightening, presolveCanSkipTerm, and binary coefficient strengthening.
3. Column-orbit symmetry automorphisms and monotone orbit ordering.
-/

import GoblinFormal.Common.Basic

namespace GoblinFormal.Track2A

/-! ### Rat algebraic helpers -/

private theorem rat_div_neg_eq_neg_div_pos (a b : Rat) :
    a / (-b) = (-a) / b := by
  rw [Rat.div_def, Rat.div_def, Rat.neg_mul, ← Rat.mul_neg]
  congr 1
  by_cases hb : b = 0
  · subst hb
    simp
  · apply Rat.inv_eq_of_mul_eq_one
    have hmul : (-b) * (-b⁻¹) = b * b⁻¹ := by
      rw [Rat.neg_mul, Rat.mul_neg, Rat.neg_neg]
    rw [hmul, Rat.mul_inv_cancel b hb]

/-! ### 1. Integer Equality GCD Infeasibility & divCeil -/

/-- Truncated integer ceiling division matching Go `divCeilInt64`. -/
def divCeilSpec (a d : Int) : Int :=
  let q := Int.tdiv a d
  if 0 < Int.tmod a d then q + 1 else q

/-- Truncated integer floor division matching Go `divFloorInt64`. -/
def divFloorSpec (a d : Int) : Int :=
  let q := Int.tdiv a d
  if Int.tmod a d < 0 then q - 1 else q

/-- Non-negative GCD specification matching Go `gcdInt64` on non-negative inputs. -/
def gcdSpec (a b : Int) : Int :=
  (Int.gcd a b : Int)

theorem gcdSpec_nonneg (a b : Int) : 0 ≤ gcdSpec a b :=
  Int.natCast_nonneg _

theorem gcdSpec_dvd_left (a b : Int) : gcdSpec a b ∣ a := by
  unfold gcdSpec
  exact Int.gcd_dvd_left a b

theorem gcdSpec_dvd_right (a b : Int) : gcdSpec a b ∣ b := by
  unfold gcdSpec
  exact Int.gcd_dvd_right a b

theorem gcdSpec_pos_of_ne_zero {a b : Int} (h : a ≠ 0 ∨ b ≠ 0) : 0 < gcdSpec a b := by
  unfold gcdSpec
  have hnat : a.natAbs ≠ 0 ∨ b.natAbs ≠ 0 := by
    rcases h with ha | hb
    · left; exact Int.natAbs_ne_zero.mpr ha
    · right; exact Int.natAbs_ne_zero.mpr hb
  have hg : 0 < Int.gcd a b := by
    rcases hnat with ha | hb
    · exact Nat.gcd_pos_of_pos_left b.natAbs (Nat.pos_of_ne_zero ha)
    · exact Nat.gcd_pos_of_pos_right a.natAbs (Nat.pos_of_ne_zero hb)
  exact Int.natCast_pos.mpr hg

/-- Ceiling division soundness and minimality for positive divisors. -/
theorem divCeilSpec_spec {a d : Int} (hd : 0 < d) :
    a ≤ divCeilSpec a d * d ∧ ∀ k : Int, a ≤ k * d → divCeilSpec a d ≤ k := by
  unfold divCeilSpec
  dsimp only
  have hdecomp : Int.tmod a d + d * Int.tdiv a d = a := Int.tmod_add_mul_tdiv a d
  have hlt : Int.tmod a d < d := Int.tmod_lt_of_pos a hd
  have hgt : -d < Int.tmod a d := Int.lt_tmod_of_pos a hd
  split
  · next hpos =>
    constructor
    · rw [Int.add_mul, Int.one_mul, Int.mul_comm (Int.tdiv a d) d]
      omega
    · intro k hk
      rw [Int.mul_comm d (Int.tdiv a d)] at hdecomp
      have hkd : Int.tdiv a d * d < k * d := by omega
      have hdiff : 0 < (k - Int.tdiv a d) * d := by
        rw [Int.sub_mul]
        omega
      have hkgt : 0 < k - Int.tdiv a d := by
        apply Classical.byContradiction
        intro hle
        have hle' : k - Int.tdiv a d ≤ 0 := Int.not_lt.mp hle
        have hnp : (k - Int.tdiv a d) * d ≤ 0 :=
          Int.mul_nonpos_of_nonpos_of_nonneg hle' (Int.le_of_lt hd)
        omega
      omega
  · next hnonpos =>
    have hrle : Int.tmod a d ≤ 0 := Int.not_lt.mp hnonpos
    constructor
    · rw [Int.mul_comm (Int.tdiv a d) d]
      omega
    · intro k hk
      rw [Int.mul_comm d (Int.tdiv a d)] at hdecomp
      have hdiff : (Int.tdiv a d - k) * d < d := by
        rw [Int.sub_mul]
        omega
      have hlt1 : Int.tdiv a d - k < 1 := by
        apply Classical.byContradiction
        intro hge
        have hge1 : 1 ≤ Int.tdiv a d - k := Int.not_lt.mp hge
        have hmul : d ≤ (Int.tdiv a d - k) * d := by
          have := Int.mul_le_mul_of_nonneg_right hge1 (Int.le_of_lt hd)
          simpa using this
        omega
      omega

/-- Fold GCD over a list of integer coefficients. -/
def listGcd : List Int → Int
  | [] => 0
  | x :: xs => gcdSpec x (listGcd xs)

theorem listGcd_nonneg (xs : List Int) : 0 ≤ listGcd xs := by
  induction xs with
  | nil => exact Int.le_refl 0
  | cons x xs _ => exact gcdSpec_nonneg x (listGcd xs)

theorem listGcd_dvd {xs : List Int} {c : Int} (hc : c ∈ xs) : listGcd xs ∣ c := by
  induction xs with
  | nil => contradiction
  | cons x xs ih =>
    simp only [listGcd]
    rcases List.mem_cons.mp hc with rfl | hmem
    · exact gcdSpec_dvd_left c (listGcd xs)
    · exact Int.dvd_trans (gcdSpec_dvd_right x (listGcd xs)) (ih hmem)

/-- Executable GCD infeasibility check for an integer equality row `lo ≤ ∑ a_j x_j ≤ hi`. -/
def hasInfeasibleIntEqSpec (coeffs : List Int) (lo hi : Int) : Bool :=
  let g := listGcd coeffs
  if 0 < g then
    hi < divCeilSpec lo g * g
  else
    hi < 0 || 0 < lo

/-- Soundness of GCD infeasibility for integer linear equations over integer bounds `[lo, hi]`. -/
theorem gcd_row_infeasible_sound
    {n : Nat} {a : Fin n → Int} {g lo hi : Int}
    (hg : 0 < g) (hdvd : ∀ j : Fin n, g ∣ a j) (hgap : divCeilSpec lo g * g > hi) :
    ¬ ∃ x : Fin n → Int,
      lo ≤ intFinSum n (fun j => a j * x j) ∧ intFinSum n (fun j => a j * x j) ≤ hi := by
  rintro ⟨x, hlo, hhi⟩
  have hdiv : g ∣ intFinSum n (fun j => a j * x j) := intFinSum_dvd hdvd
  rcases hdiv with ⟨k, hk⟩
  have hlo' : lo ≤ k * g := by
    rw [hk, Int.mul_comm] at hlo
    exact hlo
  have hceil : divCeilSpec lo g ≤ k := (divCeilSpec_spec hg).2 k hlo'
  have hmul : divCeilSpec lo g * g ≤ k * g :=
    Int.mul_le_mul_of_nonneg_right hceil (Int.le_of_lt hg)
  rw [Int.mul_comm k g, ← hk] at hmul
  omega

/-- Soundness of GCD infeasibility with rational tolerance `|a · x - rhs| ≤ tol`. -/
theorem gcd_row_infeasible_tol_sound
    {n : Nat} {a : Fin n → Int} {g : Int} {rhs tol : Rat}
    (hg : 0 < g) (hdvd : ∀ j : Fin n, g ∣ a j)
    (hgap : divCeilSpec (rhs - tol).ceil g * g > (rhs + tol).floor) :
    ¬ ∃ x : Fin n → Int,
      (((intFinSum n (fun j => a j * x j) : Int) : Rat) - rhs).abs ≤ tol := by
  rintro ⟨x, htol⟩
  let s : Int := intFinSum n (fun j => a j * x j)
  have hs_abs : ((s : Rat) - rhs).abs ≤ tol := htol
  have hlo_rat : rhs - tol ≤ (s : Rat) := by
    have hneg : -((s : Rat) - rhs) ≤ ((s : Rat) - rhs).abs := by
      rcases Rat.le_total (a := 0) (b := (s : Rat) - rhs) with h | h
      · rw [Rat.abs_of_nonneg h]
        have h1 : -((s : Rat) - rhs) ≤ 0 := Rat.neg_le_iff.mpr (by simpa using h)
        exact Rat.le_trans h1 h
      · rw [Rat.abs_of_nonpos h]
        exact Rat.le_refl
    have hle : rhs - (s : Rat) ≤ tol := by
      simpa [Rat.neg_sub] using Rat.le_trans hneg hs_abs
    have h1 := (Rat.add_le_add_right (c := (s : Rat) - tol)).mpr hle
    have h2 : rhs - (s : Rat) + ((s : Rat) - tol) = rhs - tol := by
      rw [Rat.sub_eq_add_neg, Rat.sub_eq_add_neg, Rat.add_assoc, ← Rat.add_assoc (-(s : Rat)),
        Rat.neg_add_cancel, Rat.zero_add, ← Rat.sub_eq_add_neg]
    have h3 : tol + ((s : Rat) - tol) = (s : Rat) := by
      rw [Rat.add_comm, Rat.sub_add_cancel]
    rw [h2, h3] at h1
    exact h1
  have hhi_rat : (s : Rat) ≤ rhs + tol := by
    have hpos : (s : Rat) - rhs ≤ ((s : Rat) - rhs).abs := by
      rcases Rat.le_total (a := 0) (b := (s : Rat) - rhs) with h | h
      · rw [Rat.abs_of_nonneg h]
        exact Rat.le_refl
      · rw [Rat.abs_of_nonpos h]
        exact Rat.le_trans h (Rat.le_neg_iff.mpr (by simpa using h))
    have hle : (s : Rat) - rhs ≤ tol := Rat.le_trans hpos hs_abs
    simpa [Rat.add_comm] using Rat.sub_right_le_iff_le_add.mp hle
  have hlo : (rhs - tol).ceil ≤ s := Rat.ceil_le_iff.mpr hlo_rat
  have hhi : s ≤ (rhs + tol).floor := Rat.le_floor_iff.mpr hhi_rat
  exact gcd_row_infeasible_sound hg hdvd hgap ⟨x, hlo, hhi⟩

/-! ### 2. Presolve Activity Tightening, `presolveCanSkipTerm`, & Binary Strengthening -/

/-- Mixed-integer single-row feasibility predicate. -/
def RowFeasible (n : Nat) (a l u x : Fin n → Rat) (rhs : Rat)
    (isInt : Fin n → Bool) (hasLo hasHi : Bool) : Prop :=
  (∀ j : Fin n, l j ≤ x j ∧ x j ≤ u j) ∧
  (∀ j : Fin n, isInt j = true → ∃ z : Int, x j = (z : Rat)) ∧
  (hasHi = true → finSum n (fun j => a j * x j) ≤ rhs) ∧
  (hasLo = true → rhs ≤ finSum n (fun j => a j * x j))

/-- Minimum contribution of `a_j * x_j` for `x_j ∈ [l_j, u_j]`. -/
def termMin (aj lj uj : Rat) : Rat :=
  if 0 ≤ aj then aj * lj else aj * uj

/-- Maximum contribution of `a_j * x_j` for `x_j ∈ [l_j, u_j]`. -/
def termMax (aj lj uj : Rat) : Rat :=
  if 0 ≤ aj then aj * uj else aj * lj

/-- Minimum row activity `minAct` over box `[l, u]`. -/
def minAct (n : Nat) (a l u : Fin n → Rat) : Rat :=
  finSum n (fun j => termMin (a j) (l j) (u j))

/-- Maximum row activity `maxAct` over box `[l, u]`. -/
def maxAct (n : Nat) (a l u : Fin n → Rat) : Rat :=
  finSum n (fun j => termMax (a j) (l j) (u j))

/-- Minimum activity excluding variable `k`. -/
def minOther (n : Nat) (a l u : Fin n → Rat) (k : Fin n) : Rat :=
  finSum n (fun j => if j = k then 0 else termMin (a j) (l j) (u j))

/-- Maximum activity excluding variable `k`. -/
def maxOther (n : Nat) (a l u : Fin n → Rat) (k : Fin n) : Rat :=
  finSum n (fun j => if j = k then 0 else termMax (a j) (l j) (u j))

theorem minAct_eq_add_termMin (n : Nat) (a l u : Fin n → Rat) (k : Fin n) :
    minAct n a l u = minOther n a l u k + termMin (a k) (l k) (u k) :=
  finSum_split_at (fun j => termMin (a j) (l j) (u j)) k

theorem maxAct_eq_add_termMax (n : Nat) (a l u : Fin n → Rat) (k : Fin n) :
    maxAct n a l u = maxOther n a l u k + termMax (a k) (l k) (u k) :=
  finSum_split_at (fun j => termMax (a j) (l j) (u j)) k

theorem termMin_le_mul {aj lj uj xj : Rat} (hl : lj ≤ xj) (hu : xj ≤ uj) :
    termMin aj lj uj ≤ aj * xj := by
  unfold termMin
  split
  · next hpos =>
    exact Rat.mul_le_mul_of_nonneg_left hl hpos
  · next hneg =>
    have hle : aj ≤ 0 := Rat.le_of_lt (Rat.not_le.mp hneg)
    have hnn : 0 ≤ -aj := Rat.le_neg_iff.mpr (by simpa using hle)
    have hm := Rat.mul_le_mul_of_nonneg_left hu hnn
    rw [Rat.neg_mul, Rat.neg_mul, Rat.neg_le_neg_iff] at hm
    exact hm

theorem mul_le_termMax {aj lj uj xj : Rat} (hl : lj ≤ xj) (hu : xj ≤ uj) :
    aj * xj ≤ termMax aj lj uj := by
  unfold termMax
  split
  · next hpos =>
    exact Rat.mul_le_mul_of_nonneg_left hu hpos
  · next hneg =>
    have hle : aj ≤ 0 := Rat.le_of_lt (Rat.not_le.mp hneg)
    have hnn : 0 ≤ -aj := Rat.le_neg_iff.mpr (by simpa using hle)
    have hm := Rat.mul_le_mul_of_nonneg_left hl hnn
    rw [Rat.neg_mul, Rat.neg_mul, Rat.neg_le_neg_iff] at hm
    exact hm

theorem minAct_le_sum {n : Nat} {a l u x : Fin n → Rat}
    (hb : ∀ j : Fin n, l j ≤ x j ∧ x j ≤ u j) :
    minAct n a l u ≤ finSum n (fun j => a j * x j) :=
  finSum_le_finSum (fun j => termMin_le_mul (hb j).1 (hb j).2)

theorem sum_le_maxAct {n : Nat} {a l u x : Fin n → Rat}
    (hb : ∀ j : Fin n, l j ≤ x j ∧ x j ≤ u j) :
    finSum n (fun j => a j * x j) ≤ maxAct n a l u :=
  finSum_le_finSum (fun j => mul_le_termMax (hb j).1 (hb j).2)

theorem minOther_le_sumOther {n : Nat} {a l u x : Fin n → Rat} (k : Fin n)
    (hb : ∀ j : Fin n, l j ≤ x j ∧ x j ≤ u j) :
    minOther n a l u k ≤ finSum n (fun j => if j = k then 0 else a j * x j) := by
  apply finSum_le_finSum
  intro j
  split
  · exact Rat.le_refl
  · exact termMin_le_mul (hb j).1 (hb j).2

theorem sumOther_le_maxOther {n : Nat} {a l u x : Fin n → Rat} (k : Fin n)
    (hb : ∀ j : Fin n, l j ≤ x j ∧ x j ≤ u j) :
    finSum n (fun j => if j = k then 0 else a j * x j) ≤ maxOther n a l u k := by
  apply finSum_le_finSum
  intro j
  split
  · exact Rat.le_refl
  · exact mul_le_termMax (hb j).1 (hb j).2

/-- Integer floor tightening for upper bounds. -/
def roundUpper (isInt : Bool) (q : Rat) : Rat :=
  if isInt then (q.floor : Rat) else q

/-- Integer ceil tightening for lower bounds. -/
def roundLower (isInt : Bool) (q : Rat) : Rat :=
  if isInt then (q.ceil : Rat) else q

/-- Implied upper bound on variable `k` from a row (including integer floor rounding). -/
def impliedUpper (n : Nat) (a l u : Fin n → Rat) (rhs : Rat)
    (isInt : Fin n → Bool) (hasLo hasHi : Bool) (k : Fin n) : Option Rat :=
  if hasHi && decide (0 < a k) then
    some (roundUpper (isInt k) ((rhs - minOther n a l u k) / a k))
  else if hasLo && decide (a k < 0) then
    some (roundUpper (isInt k) ((rhs - maxOther n a l u k) / a k))
  else
    none

/-- Implied lower bound on variable `k` from a row (including integer ceil rounding). -/
def impliedLower (n : Nat) (a l u : Fin n → Rat) (rhs : Rat)
    (isInt : Fin n → Bool) (hasLo hasHi : Bool) (k : Fin n) : Option Rat :=
  if hasHi && decide (a k < 0) then
    some (roundLower (isInt k) ((rhs - minOther n a l u k) / a k))
  else if hasLo && decide (0 < a k) then
    some (roundLower (isInt k) ((rhs - maxOther n a l u k) / a k))
  else
    none

private theorem roundUpper_sound {x q : Rat} {isInt : Bool}
    (hx : x ≤ q) (hint : isInt = true → ∃ z : Int, x = (z : Rat)) :
    x ≤ roundUpper isInt q := by
  unfold roundUpper
  cases isInt
  · exact hx
  · rcases hint rfl with ⟨z, rfl⟩
    exact Rat.intCast_le_intCast.mpr (Rat.le_floor_iff.mpr hx)

private theorem roundLower_sound {x q : Rat} {isInt : Bool}
    (hx : q ≤ x) (hint : isInt = true → ∃ z : Int, x = (z : Rat)) :
    roundLower isInt q ≤ x := by
  unfold roundLower
  cases isInt
  · exact hx
  · rcases hint rfl with ⟨z, rfl⟩
    exact Rat.intCast_le_intCast.mpr (Rat.ceil_le_iff.mpr hx)

/-- Soundness of implied upper bound tightening (both continuous and integer-floored). -/
theorem activity_implied_upper_sound
    {n : Nat} {a l u x : Fin n → Rat} {rhs : Rat} {isInt : Fin n → Bool}
    {hasLo hasHi : Bool} {k : Fin n} {ub : Rat}
    (hfeas : RowFeasible n a l u x rhs isInt hasLo hasHi)
    (himp : impliedUpper n a l u rhs isInt hasLo hasHi k = some ub) :
    x k ≤ ub := by
  rcases hfeas with ⟨hb, hint, hhi, hlo⟩
  unfold impliedUpper at himp
  by_cases h1 : hasHi = true ∧ 0 < a k
  · have hb1 : (hasHi && decide (0 < a k)) = true := by simp [h1.1, h1.2]
    rw [hb1] at himp
    simp only [↓reduceIte, Option.some.injEq] at himp
    subst himp
    have hsum : finSum n (fun j => a j * x j) ≤ rhs := hhi h1.1
    rw [finSum_split_at (fun j => a j * x j) k] at hsum
    have hmo := minOther_le_sumOther (a := a) k hb
    have hak_le : minOther n a l u k + a k * x k ≤ rhs :=
      Rat.le_trans (Rat.add_le_add_right.mpr hmo) hsum
    have hle_sub : a k * x k ≤ rhs - minOther n a l u k := by
      have := Rat.add_le_add_left (c := -minOther n a l u k) |>.mpr hak_le
      simpa [Rat.add_comm, Rat.sub_eq_add_neg, Rat.add_assoc, Rat.add_neg_cancel, Rat.add_zero] using this
    have hraw : x k ≤ (rhs - minOther n a l u k) / a k :=
      (rat_le_div_iff h1.2).mpr (by rwa [Rat.mul_comm])
    exact roundUpper_sound hraw (hint k)
  · have hb1 : (hasHi && decide (0 < a k)) = false := by
      cases hasHi <;> simp_all
    rw [hb1] at himp
    simp only [Bool.false_eq_true, ↓reduceIte] at himp
    by_cases h2 : hasLo = true ∧ a k < 0
    · have hb2 : (hasLo && decide (a k < 0)) = true := by simp [h2.1, h2.2]
      rw [hb2] at himp
      simp only [↓reduceIte, Option.some.injEq] at himp
      subst himp
      have hsum : rhs ≤ finSum n (fun j => a j * x j) := hlo h2.1
      rw [finSum_split_at (fun j => a j * x j) k] at hsum
      have hmo := sumOther_le_maxOther (a := a) k hb
      have hle_sum : rhs ≤ maxOther n a l u k + a k * x k :=
        Rat.le_trans hsum (Rat.add_le_add_right.mpr hmo)
      have hneg_pos : 0 < -a k := Rat.lt_neg_iff.mpr (by simpa using h2.2)
      have hle_sub : (-a k) * x k ≤ maxOther n a l u k - rhs := by
        have h1' : rhs - maxOther n a l u k ≤ a k * x k := by
          have := Rat.add_le_add_right (c := -maxOther n a l u k) |>.mpr hle_sum
          simpa [Rat.sub_eq_add_neg, Rat.add_comm, Rat.add_assoc, Rat.add_neg_cancel, Rat.add_zero] using this
        have h2' := Rat.neg_le_neg h1'
        simpa [Rat.neg_sub, Rat.neg_mul] using h2'
      have hraw : x k ≤ (maxOther n a l u k - rhs) / (-a k) :=
        (rat_le_div_iff hneg_pos).mpr (by rwa [Rat.mul_comm])
      have hraw' : x k ≤ (rhs - maxOther n a l u k) / a k := by
        have heq : (maxOther n a l u k - rhs) / (-a k) = (rhs - maxOther n a l u k) / a k := by
          rw [rat_div_neg_eq_neg_div_pos, Rat.neg_sub]
        rwa [← heq]
      exact roundUpper_sound hraw' (hint k)
    · have hb2 : (hasLo && decide (a k < 0)) = false := by
        cases hasLo <;> simp_all
      rw [hb2] at himp
      simp at himp

/-- Soundness of implied lower bound tightening (both continuous and integer-ceiled). -/
theorem activity_implied_lower_sound
    {n : Nat} {a l u x : Fin n → Rat} {rhs : Rat} {isInt : Fin n → Bool}
    {hasLo hasHi : Bool} {k : Fin n} {lb : Rat}
    (hfeas : RowFeasible n a l u x rhs isInt hasLo hasHi)
    (himp : impliedLower n a l u rhs isInt hasLo hasHi k = some lb) :
    lb ≤ x k := by
  rcases hfeas with ⟨hb, hint, hhi, hlo⟩
  unfold impliedLower at himp
  by_cases h1 : hasHi = true ∧ a k < 0
  · have hb1 : (hasHi && decide (a k < 0)) = true := by simp [h1.1, h1.2]
    rw [hb1] at himp
    simp only [↓reduceIte, Option.some.injEq] at himp
    subst himp
    have hsum : finSum n (fun j => a j * x j) ≤ rhs := hhi h1.1
    rw [finSum_split_at (fun j => a j * x j) k] at hsum
    have hmo := minOther_le_sumOther (a := a) k hb
    have hak_le : minOther n a l u k + a k * x k ≤ rhs :=
      Rat.le_trans (Rat.add_le_add_right.mpr hmo) hsum
    have hneg_pos : 0 < -a k := Rat.lt_neg_iff.mpr (by simpa using h1.2)
    have hsub_le : minOther n a l u k - rhs ≤ (-a k) * x k := by
      have h1' : a k * x k ≤ rhs - minOther n a l u k := by
        have := Rat.add_le_add_left (c := -minOther n a l u k) |>.mpr hak_le
        simpa [Rat.add_comm, Rat.sub_eq_add_neg, Rat.add_assoc, Rat.add_neg_cancel, Rat.add_zero] using this
      have h2' := Rat.neg_le_neg h1'
      simpa [Rat.neg_sub, Rat.neg_mul] using h2'
    have hraw : (minOther n a l u k - rhs) / (-a k) ≤ x k :=
      (rat_div_le_iff hneg_pos).mpr (by rwa [Rat.mul_comm (x k) (-a k)])
    have hraw' : (rhs - minOther n a l u k) / a k ≤ x k := by
      have heq : (minOther n a l u k - rhs) / (-a k) = (rhs - minOther n a l u k) / a k := by
        rw [rat_div_neg_eq_neg_div_pos, Rat.neg_sub]
      rwa [← heq]
    exact roundLower_sound hraw' (hint k)
  · have hb1 : (hasHi && decide (a k < 0)) = false := by
      cases hasHi <;> simp_all
    rw [hb1] at himp
    simp only [Bool.false_eq_true, ↓reduceIte] at himp
    by_cases h2 : hasLo = true ∧ 0 < a k
    · have hb2 : (hasLo && decide (0 < a k)) = true := by simp [h2.1, h2.2]
      rw [hb2] at himp
      simp only [↓reduceIte, Option.some.injEq] at himp
      subst himp
      have hsum : rhs ≤ finSum n (fun j => a j * x j) := hlo h2.1
      rw [finSum_split_at (fun j => a j * x j) k] at hsum
      have hmo := sumOther_le_maxOther (a := a) k hb
      have hle_sum : rhs ≤ maxOther n a l u k + a k * x k :=
        Rat.le_trans hsum (Rat.add_le_add_right.mpr hmo)
      have hsub_le : rhs - maxOther n a l u k ≤ a k * x k := by
        have := Rat.add_le_add_right (c := -maxOther n a l u k) |>.mpr hle_sum
        simpa [Rat.sub_eq_add_neg, Rat.add_comm, Rat.add_assoc, Rat.add_neg_cancel, Rat.add_zero] using this
      have hraw : (rhs - maxOther n a l u k) / a k ≤ x k :=
        (rat_div_le_iff h2.2).mpr (by rwa [Rat.mul_comm (x k) (a k)])
      exact roundLower_sound hraw (hint k)
    · have hb2 : (hasLo && decide (0 < a k)) = false := by
        cases hasLo <;> simp_all
      rw [hb2] at himp
      simp at himp

/-- Exact relative guard margin `0x1p-30` from `presolve.go`. -/
def slackGuardRelMargin : Rat :=
  mkRat 1 (2 ^ 30)

/-- Executable specification matching `presolveCanSkipTerm` in `presolve.go`. -/
def canSkipTermSpec (slackLo absA l u targetBound : Rat) (isInt : Bool) : Bool :=
  if slackLo ≤ 0 then false
  else if isInt && targetBound.den != 1 then false
  else
    let span := u - l
    if span < 0 then false
    else
      let prod := absA * span
      slackLo > prod + slackGuardRelMargin * prod

theorem canSkipTermSpec_implies_strict
    {slackLo absA l u targetBound : Rat} {isInt : Bool}
    (habs : 0 ≤ absA)
    (hskip : canSkipTermSpec slackLo absA l u targetBound isInt = true) :
    0 < slackLo ∧ 0 ≤ u - l ∧ absA * (u - l) < slackLo ∧
      (isInt = true → targetBound.den = 1) := by
  unfold canSkipTermSpec at hskip
  dsimp only at hskip
  split at hskip
  · contradiction
  · next hpos =>
    have hslack : 0 < slackLo := Rat.not_le.mp hpos
    split at hskip
    · contradiction
    · next hint_frac =>
      split at hskip
      · contradiction
      · next hspan =>
        have hspan_nn : 0 ≤ u - l := Rat.not_lt.mp hspan
        have hgt_margin : absA * (u - l) + slackGuardRelMargin * (absA * (u - l)) < slackLo :=
          of_decide_eq_true hskip
        have hprod_nn : 0 ≤ absA * (u - l) := Rat.mul_nonneg habs hspan_nn
        have hmargin_nn : 0 ≤ slackGuardRelMargin := by
          unfold slackGuardRelMargin
          decide
        have hmargin_prod_nn : 0 ≤ slackGuardRelMargin * (absA * (u - l)) :=
          Rat.mul_nonneg hmargin_nn hprod_nn
        have hprod_le : absA * (u - l) ≤
            absA * (u - l) + slackGuardRelMargin * (absA * (u - l)) := by
          simpa [Rat.add_zero] using
            (Rat.add_le_add_left (c := absA * (u - l))).mpr hmargin_prod_nn
        have hstrict : absA * (u - l) < slackLo := rat_lt_of_le_of_lt hprod_le hgt_margin
        refine ⟨hslack, hspan_nn, hstrict, ?_⟩
        intro hi
        apply Classical.byContradiction
        intro hne
        apply hint_frac
        simp [hi, hne]

private theorem int_lt_rat_imp_le_floor {b : Rat} {q : Rat} (hden : b.den = 1) (hlt : b < q) :
    b ≤ (q.floor : Rat) := by
  rw [rat_eq_num_of_den_one hden] at hlt ⊢
  exact Rat.intCast_le_intCast.mpr (Rat.le_floor_iff.mpr (Rat.le_of_lt hlt))

private theorem rat_lt_int_imp_ceil_le {b : Rat} {q : Rat} (hden : b.den = 1) (hlt : q < b) :
    (q.ceil : Rat) ≤ b := by
  rw [rat_eq_num_of_den_one hden] at hlt ⊢
  exact Rat.intCast_le_intCast.mpr (Rat.ceil_le_iff.mpr (Rat.le_of_lt hlt))

/-- Completeness of `presolveCanSkipTerm` for upper-row (`rhs - minAct`) tightening.
When `slackLo ≤ rhs - minAct` strictly exceeds `|a_k| * (u_k - l_k)` (with integer boundary
non-fractionality), the implied bound cannot strictly tighten `u_k` (for `a_k > 0`) or `l_k`
(for `a_k < 0`). Skipping a term is always sound; this proves no tightening is missed. -/
theorem presolveCanSkipTerm_sound
    {n : Nat} {a l u : Fin n → Rat} {rhs slackLo absA : Rat} {isInt : Fin n → Bool}
    {k : Fin n}
    (hslack_le : slackLo ≤ rhs - minAct n a l u)
    (hgt : absA * (u k - l k) < slackLo) :
    (0 < a k → absA = a k →
      u k < (rhs - minOther n a l u k) / a k ∧
      ((isInt k = true → (u k).den = 1) →
        u k ≤ roundUpper (isInt k) ((rhs - minOther n a l u k) / a k))) ∧
    (a k < 0 → absA = -a k →
      (rhs - minOther n a l u k) / a k < l k ∧
      ((isInt k = true → (l k).den = 1) →
        roundLower (isInt k) ((rhs - minOther n a l u k) / a k) ≤ l k)) := by
  have hlt_diff : absA * (u k - l k) < rhs - minAct n a l u := rat_lt_of_lt_of_le hgt hslack_le
  rw [minAct_eq_add_termMin n a l u k] at hlt_diff
  constructor
  · intro hak hAbsEq
    rw [hAbsEq] at hlt_diff
    have htm : termMin (a k) (l k) (u k) = a k * l k := by
      unfold termMin
      simp [Rat.le_of_lt hak]
    rw [htm] at hlt_diff
    have h1 : a k * u k - a k * l k < rhs - (minOther n a l u k + a k * l k) := by
      simpa [rat_mul_sub] using hlt_diff
    have h2 : a k * u k < rhs - minOther n a l u k := by
      have hadd := (Rat.add_lt_add_right (c := a k * l k)).mpr h1
      have hlhs : a k * u k - a k * l k + a k * l k = a k * u k := Rat.sub_add_cancel
      have hrhs : rhs - (minOther n a l u k + a k * l k) + a k * l k = rhs - minOther n a l u k := by
        rw [Rat.sub_eq_add_neg, Rat.neg_add, ← Rat.add_assoc, Rat.add_assoc (rhs + -minOther n a l u k),
          Rat.neg_add_cancel, Rat.add_zero, ← Rat.sub_eq_add_neg]
      rwa [hlhs, hrhs] at hadd
    have hraw : u k < (rhs - minOther n a l u k) / a k := by
      rw [Rat.lt_div_iff hak, Rat.mul_comm]
      exact h2
    refine ⟨hraw, ?_⟩
    intro hint
    unfold roundUpper
    cases hkInt : isInt k
    · exact Rat.le_of_lt hraw
    · exact int_lt_rat_imp_le_floor (hint hkInt) hraw
  · intro hak hAbsEq
    rw [hAbsEq] at hlt_diff
    have htm : termMin (a k) (l k) (u k) = a k * u k := by
      unfold termMin
      have hnot : ¬ (0 ≤ a k) := Rat.not_le.mpr hak
      simp [hnot]
    rw [htm] at hlt_diff
    have hneg_pos : 0 < -a k := Rat.lt_neg_iff.mpr (by simpa using hak)
    have h1 : (-a k) * u k - (-a k) * l k < rhs - (minOther n a l u k + a k * u k) := by
      simpa [rat_mul_sub] using hlt_diff
    have h2 : a k * l k < rhs - minOther n a l u k := by
      have hadd := (Rat.add_lt_add_right (c := a k * u k)).mpr h1
      have hlhs : (-a k) * u k - (-a k) * l k + a k * u k = a k * l k := by
        rw [Rat.sub_eq_add_neg, Rat.neg_mul, Rat.neg_mul, Rat.neg_neg,
          Rat.add_assoc, Rat.add_comm (a k * l k), ← Rat.add_assoc,
          Rat.neg_add_cancel, Rat.zero_add]
      have hrhs : rhs - (minOther n a l u k + a k * u k) + a k * u k = rhs - minOther n a l u k := by
        rw [Rat.sub_eq_add_neg, Rat.neg_add, ← Rat.add_assoc, Rat.add_assoc (rhs + -minOther n a l u k),
          Rat.neg_add_cancel, Rat.add_zero, ← Rat.sub_eq_add_neg]
      rwa [hlhs, hrhs] at hadd
    have h3 : minOther n a l u k - rhs < (-a k) * l k := by
      have hneg := Rat.neg_lt_neg h2
      have hl : -(rhs - minOther n a l u k) = minOther n a l u k - rhs := Rat.neg_sub _ _
      have hr : -(a k * l k) = (-a k) * l k := (Rat.neg_mul (a k) (l k)).symm
      rwa [hl, hr] at hneg
    have hraw_neg : (minOther n a l u k - rhs) / (-a k) < l k := by
      rw [Rat.div_lt_iff hneg_pos, Rat.mul_comm (l k) (-a k)]
      exact h3
    have hraw : (rhs - minOther n a l u k) / a k < l k := by
      have heq : (minOther n a l u k - rhs) / (-a k) = (rhs - minOther n a l u k) / a k := by
        rw [rat_div_neg_eq_neg_div_pos, Rat.neg_sub]
      rwa [← heq]
    refine ⟨hraw, ?_⟩
    intro hint
    unfold roundLower
    cases hkInt : isInt k
    · exact Rat.le_of_lt hraw
    · exact rat_lt_int_imp_ceil_le (hint hkInt) hraw

/-- Binary coefficient strengthening when `a > 0`. -/
def strengthenBinaryPosSpec (rhs a surplus : Rat) : Option (Rat × Rat) :=
  if 0 < surplus && surplus < a then
    some (surplus, rhs - (a - surplus))
  else
    none

/-- Binary coefficient strengthening when `a < 0`. -/
def strengthenBinaryNegSpec (rhs a surplus : Rat) : Option (Rat × Rat) :=
  if 0 < surplus && surplus < -a then
    some (-surplus, rhs)
  else
    none

/-- Savelsbergh binary coefficient strengthening equivalence for positive coefficient `a_k > 0`.
Here `surplus` satisfies `maxOther + a_k - rhs ≤ surplus` (e.g., `surplus = maxAct - rhs`) and `surplus < a_k`. -/
theorem binary_strengthen_pos_iff
    {n : Nat} {a x : Fin n → Rat} {rhs maxOther surplus : Rat} {k : Fin n}
    (hbin : x k = 0 ∨ x k = 1)
    (hother : finSum n (fun j => if j = k then 0 else a j * x j) ≤ maxOther)
    (hsurpLo : maxOther + a k - rhs ≤ surplus)
    (hsurpLt : surplus < a k) :
    finSum n (fun j => a j * x j) ≤ rhs ↔
      finSum n (fun j => if j = k then surplus * x j else a j * x j) ≤ rhs - (a k - surplus) := by
  let S : Rat := finSum n (fun j => if j = k then 0 else a j * x j)
  have hsum_orig : finSum n (fun j => a j * x j) = S + a k * x k :=
    finSum_split_at (fun j => a j * x j) k
  have hsum_new : finSum n (fun j => if j = k then surplus * x j else a j * x j) = S + surplus * x k := by
    rw [finSum_split_at (fun j => if j = k then surplus * x j else a j * x j) k]
    simp only [↓reduceIte]
    congr 1
    apply finSum_congr
    intro j
    by_cases hj : j = k
    · subst hj; simp
    · simp [hj]
  rw [hsum_orig, hsum_new]
  rcases hbin with hx0 | hx1
  · rw [hx0, Rat.mul_zero, Rat.mul_zero, Rat.add_zero]
    have hmo_lt_rhs : maxOther < rhs := by
      have h1 : maxOther + a k - rhs < a k := rat_lt_of_le_of_lt hsurpLo hsurpLt
      have h2 := (Rat.add_lt_add_right (c := rhs - a k)).mpr h1
      have hl : maxOther + a k - rhs + (rhs - a k) = maxOther := by
        rw [Rat.sub_eq_add_neg, Rat.sub_eq_add_neg, Rat.add_assoc (maxOther + a k),
          ← Rat.add_assoc (-rhs) rhs (-a k), Rat.neg_add_cancel, Rat.zero_add,
          Rat.add_assoc maxOther, Rat.add_neg_cancel, Rat.add_zero]
      have hr : a k + (rhs - a k) = rhs := by
        rw [Rat.add_comm, Rat.sub_add_cancel]
      rwa [hl, hr] at h2
    have hmo_le_new : maxOther ≤ rhs - (a k - surplus) := by
      have h1 := (Rat.add_le_add_right (c := rhs - a k)).mpr hsurpLo
      have hl : maxOther + a k - rhs + (rhs - a k) = maxOther := by
        rw [Rat.sub_eq_add_neg, Rat.sub_eq_add_neg, Rat.add_assoc (maxOther + a k),
          ← Rat.add_assoc (-rhs) rhs (-a k), Rat.neg_add_cancel, Rat.zero_add,
          Rat.add_assoc maxOther, Rat.add_neg_cancel, Rat.add_zero]
      have hr : surplus + (rhs - a k) = rhs - (a k - surplus) := by
        rw [Rat.sub_eq_add_neg, Rat.sub_eq_add_neg, Rat.neg_sub, Rat.sub_eq_add_neg,
          Rat.add_comm surplus, Rat.add_assoc, Rat.add_comm (-a k) surplus]
      rwa [hl, hr] at h1
    constructor
    · intro _
      exact Rat.le_trans hother hmo_le_new
    · intro _
      exact Rat.le_trans hother (Rat.le_of_lt hmo_lt_rhs)
  · rw [hx1, Rat.mul_one, Rat.mul_one]
    constructor
    · intro h
      have h1 := (Rat.add_le_add_right (c := -(a k - surplus))).mpr h
      have hl : S + a k + -(a k - surplus) = S + surplus := by
        rw [Rat.neg_sub, Rat.sub_eq_add_neg, Rat.add_assoc, Rat.add_comm surplus (-a k),
          ← Rat.add_assoc (a k) (-a k) surplus, Rat.add_neg_cancel, Rat.zero_add]
      rwa [hl, ← Rat.sub_eq_add_neg] at h1
    · intro h
      have h1 := (Rat.add_le_add_right (c := a k - surplus)).mpr h
      have hl : S + surplus + (a k - surplus) = S + a k := by
        rw [Rat.add_assoc, Rat.add_comm surplus (a k - surplus), Rat.sub_add_cancel]
      have hr : rhs - (a k - surplus) + (a k - surplus) = rhs := Rat.sub_add_cancel
      rwa [hl, hr] at h1

/-- Outward-rounded binary strengthening soundness for `a_k > 0` (`newRHS ≥ rhs - (a_k - surplus)`). -/
theorem binary_strengthen_pos_outward_sound
    {n : Nat} {a x : Fin n → Rat} {rhs maxOther surplus newRHS : Rat} {k : Fin n}
    (hbin : x k = 0 ∨ x k = 1)
    (hother : finSum n (fun j => if j = k then 0 else a j * x j) ≤ maxOther)
    (hsurpLo : maxOther + a k - rhs ≤ surplus)
    (hsurpLt : surplus < a k)
    (hround : rhs - (a k - surplus) ≤ newRHS)
    (horig : finSum n (fun j => a j * x j) ≤ rhs) :
    finSum n (fun j => if j = k then surplus * x j else a j * x j) ≤ newRHS :=
  Rat.le_trans ((binary_strengthen_pos_iff hbin hother hsurpLo hsurpLt).mp horig) hround

/-- Savelsbergh binary coefficient strengthening equivalence for negative coefficient `a_k < 0`.
Here `surplus` satisfies `maxOther - rhs ≤ surplus` (e.g., `surplus = maxAct - rhs`) and `surplus < -a_k`. -/
theorem binary_strengthen_neg_iff
    {n : Nat} {a x : Fin n → Rat} {rhs maxOther surplus : Rat} {k : Fin n}
    (hbin : x k = 0 ∨ x k = 1)
    (hother : finSum n (fun j => if j = k then 0 else a j * x j) ≤ maxOther)
    (hsurpLo : maxOther - rhs ≤ surplus)
    (hsurpLt : surplus < -a k) :
    finSum n (fun j => a j * x j) ≤ rhs ↔
      finSum n (fun j => if j = k then (-surplus) * x j else a j * x j) ≤ rhs := by
  let S : Rat := finSum n (fun j => if j = k then 0 else a j * x j)
  have hsum_orig : finSum n (fun j => a j * x j) = S + a k * x k :=
    finSum_split_at (fun j => a j * x j) k
  have hsum_new : finSum n (fun j => if j = k then (-surplus) * x j else a j * x j) = S + (-surplus) * x k := by
    rw [finSum_split_at (fun j => if j = k then (-surplus) * x j else a j * x j) k]
    simp only [↓reduceIte]
    congr 1
    apply finSum_congr
    intro j
    by_cases hj : j = k
    · subst hj; simp
    · simp [hj]
  rw [hsum_orig, hsum_new]
  rcases hbin with hx0 | hx1
  · rw [hx0, Rat.mul_zero, Rat.mul_zero, Rat.add_zero]
  · rw [hx1, Rat.mul_one, Rat.mul_one]
    have h_orig_true : S + a k ≤ rhs := by
      have hmo_lt : maxOther - rhs < -a k := rat_lt_of_le_of_lt hsurpLo hsurpLt
      have hmo_le : maxOther ≤ rhs - a k := by
        have h1 := (Rat.add_lt_add_right (c := rhs)).mpr hmo_lt
        rw [Rat.sub_add_cancel] at h1
        have h2 : (-a k) + rhs = rhs - a k := by
          rw [Rat.add_comm, ← Rat.sub_eq_add_neg]
        rw [h2] at h1
        exact Rat.le_of_lt h1
      have hS_le : S ≤ rhs - a k := Rat.le_trans hother hmo_le
      have hadd := (Rat.add_le_add_right (c := a k)).mpr hS_le
      rwa [Rat.sub_add_cancel] at hadd
    have h_new_true : S + (-surplus) ≤ rhs := by
      have hmo_le : maxOther ≤ rhs + surplus := by
        have h1 := (Rat.add_le_add_right (c := rhs)).mpr hsurpLo
        rw [Rat.sub_add_cancel, Rat.add_comm surplus rhs] at h1
        exact h1
      have hS_le : S ≤ rhs + surplus := Rat.le_trans hother hmo_le
      have hadd := (Rat.add_le_add_right (c := -surplus)).mpr hS_le
      have hr : rhs + surplus + -surplus = rhs := by
        rw [Rat.add_assoc, Rat.add_neg_cancel, Rat.add_zero]
      rwa [hr] at hadd
    exact ⟨fun _ => h_new_true, fun _ => h_orig_true⟩

/-! ### 3. Orbital Symmetry Breaking -/

/-- Row sense matching `ConstraintSense`. -/
inductive Sense where
  | le
  | ge
  | eq
  deriving DecidableEq, Repr, Inhabited

/-- Linear constraint row over `Fin n`. -/
structure RowSpec (n : Nat) where
  coeffs : Fin n → Rat
  sense : Sense
  rhs : Rat

/-- Row satisfaction predicate at `x`. -/
def RowSpec.holds {n : Nat} (r : RowSpec n) (x : Fin n → Rat) : Prop :=
  let s := finSum n (fun j => r.coeffs j * x j)
  match r.sense with
  | .le => s ≤ r.rhs
  | .ge => r.rhs ≤ s
  | .eq => s = r.rhs

/-- Mixed-integer linear program specification. -/
structure MipSpec (n m : Nat) where
  l : Fin n → Rat
  u : Fin n → Rat
  isInt : Fin n → Bool
  obj : Fin n → Rat
  rows : Fin m → RowSpec n

/-- Full MIP feasibility predicate. -/
def MipFeasible {n m : Nat} (P : MipSpec n m) (x : Fin n → Rat) : Prop :=
  (∀ j : Fin n, P.l j ≤ x j ∧ x j ≤ P.u j) ∧
  (∀ j : Fin n, P.isInt j = true → ∃ z : Int, x j = (z : Rat)) ∧
  (∀ i : Fin m, (P.rows i).holds x)

/-- Objective value `c · x`. -/
def mipObj {n : Nat} (c x : Fin n → Rat) : Rat :=
  finSum n (fun j => c j * x j)

/-- Column automorphism preserving variable types, bounds, objective, and feasibility. -/
structure Aut {n m : Nat} (P : MipSpec n m) where
  perm : Fin n → Fin n
  inv : Fin n → Fin n
  left_inv : ∀ j : Fin n, inv (perm j) = j
  right_inv : ∀ j : Fin n, perm (inv j) = j
  preserves_isInt : ∀ j : Fin n, P.isInt (perm j) = P.isInt j
  preserves_lb : ∀ j : Fin n, P.l (perm j) = P.l j
  preserves_ub : ∀ j : Fin n, P.u (perm j) = P.u j
  preserves_obj : ∀ j : Fin n, P.obj (perm j) = P.obj j
  preserves_rows : ∀ x : Fin n → Rat,
    (∀ i : Fin m, (P.rows i).holds x) →
    ∀ i : Fin m, (P.rows i).holds (fun j => x (perm j))

theorem Aut.perm_inj {n m : Nat} {P : MipSpec n m} (A : Aut P)
    {a b : Fin n} (h : A.perm a = A.perm b) : a = b := by
  have h1 := congrArg A.inv h
  rwa [A.left_inv, A.left_inv] at h1

theorem Aut.preserves_feasible {n m : Nat} {P : MipSpec n m} (A : Aut P)
    {x : Fin n → Rat} (hx : MipFeasible P x) :
    MipFeasible P (fun j => x (A.perm j)) := by
  rcases hx with ⟨hb, hint, hrows⟩
  refine ⟨?_, ?_, A.preserves_rows x hrows⟩
  · intro j
    rw [← A.preserves_lb j, ← A.preserves_ub j]
    exact hb (A.perm j)
  · intro j hj
    rw [← A.preserves_isInt j] at hj
    exact hint (A.perm j) hj

theorem Aut.preserves_obj_sum {n m : Nat} {P : MipSpec n m} (A : Aut P)
    (x : Fin n → Rat) :
    mipObj P.obj (fun j => x (A.perm j)) = mipObj P.obj x := by
  unfold mipObj
  have heq : finSum n (fun j => P.obj j * x (A.perm j)) =
      finSum n (fun j => P.obj (A.perm j) * x (A.perm j)) := by
    apply finSum_congr
    intro j
    rw [A.preserves_obj j]
  rw [heq]
  exact finSum_inj_eq (g := fun j => P.obj j * x j) (fun _ _ h => A.perm_inj h)

/-- Transposition permutation swapping indices `i` and `j`. -/
def swapIdx {n : Nat} (i j k : Fin n) : Fin n :=
  if k = i then j else if k = j then i else k

theorem swapIdx_involutive {n : Nat} (i j k : Fin n) :
    swapIdx i j (swapIdx i j k) = k := by
  unfold swapIdx
  by_cases hki : k = i
  · subst hki
    by_cases hij : k = j
    · subst hij; simp
    · simp [Ne.symm hij]
  · by_cases hkj : k = j
    · subst hkj; simp [hki]
    · simp [hki, hkj]

theorem swapIdx_left {n : Nat} (i j : Fin n) : swapIdx i j i = j := by
  simp [swapIdx]

theorem swapIdx_right {n : Nat} (i j : Fin n) : swapIdx i j j = i := by
  unfold swapIdx
  by_cases hji : j = i
  · subst hji; simp
  · simp [hji]

/-- Symmetry breaking cut `x_i ≥ x_j` preserves existence of solutions of objective `≤ z`
for any involutive automorphism `A` swapping `i` and `j`. -/
theorem orbit_automorphism_preserves_opt
    {n m : Nat} {P : MipSpec n m} (A : Aut P)
    (hinv : ∀ k : Fin n, A.perm (A.perm k) = k)
    {i j : Fin n} (hpij : A.perm i = j) (z : Rat) :
    (∃ x : Fin n → Rat, MipFeasible P x ∧ mipObj P.obj x ≤ z) ↔
      (∃ x : Fin n → Rat, MipFeasible P x ∧ x j ≤ x i ∧ mipObj P.obj x ≤ z) := by
  constructor
  · rintro ⟨x, hfeas, hobj⟩
    by_cases hord : x j ≤ x i
    · exact ⟨x, hfeas, hord, hobj⟩
    · have hlt : x i < x j := Rat.not_le.mp hord
      let x' : Fin n → Rat := fun k => x (A.perm k)
      have hpji : A.perm j = i := by
        rw [← hpij, hinv i]
      have hfeas' : MipFeasible P x' := A.preserves_feasible hfeas
      have hobj' : mipObj P.obj x' ≤ z := by
        have heq := A.preserves_obj_sum x
        change mipObj P.obj x' = mipObj P.obj x at heq
        rw [heq]
        exact hobj
      have hord' : x' j ≤ x' i := by
        change x (A.perm j) ≤ x (A.perm i)
        rw [hpji, hpij]
        exact Rat.le_of_lt hlt
      exact ⟨x', hfeas', hord', hobj'⟩
  · rintro ⟨x, hfeas, _, hobj⟩
    exact ⟨x, hfeas, hobj⟩

private theorem mipObj_perm_le_of_mono {n : Nat} {c x : Fin n → Rat} {perm : Fin n → Fin n}
    (hinv : ∀ k : Fin n, perm (perm k) = k)
    {i j : Fin n} (hpij : perm i = j)
    (hc_other : ∀ k : Fin n, k ≠ i → k ≠ j → c (perm k) = c k)
    (hc_mono : c i ≤ c j)
    (hx_le : x i ≤ x j) :
    mipObj c (fun k => x (perm k)) ≤ mipObj c x := by
  have hp_inj : ∀ a b : Fin n, perm a = perm b → a = b := by
    intro a b h
    have := congrArg perm h
    rwa [hinv, hinv] at this
  have hpji : perm j = i := by
    rw [← hpij, hinv i]
  by_cases hij : i = j
  · subst hij
    have heq : mipObj c (fun k => x (perm k)) = mipObj c x := by
      unfold mipObj
      have h1 : finSum n (fun k => c k * x (perm k)) =
          finSum n (fun k => c (perm k) * x (perm k)) := by
        apply finSum_congr
        intro k
        by_cases hki : k = i
        · subst hki; rw [hpij]
        · rw [hc_other k hki hki]
      rw [h1]
      exact finSum_inj_eq (g := fun k => c k * x k) hp_inj
    rw [heq]
    exact Rat.le_refl
  · let ceq : Fin n → Rat := fun k => if k = j then c i else c k
    let delta : Fin n → Rat := fun k => if k = j then c j - c i else 0
    have hc_split : ∀ k : Fin n, c k = ceq k + delta k := by
      intro k
      dsimp [ceq, delta]
      by_cases hkj : k = j
      · subst hkj
        simp only [↓reduceIte]
        rw [Rat.add_comm, Rat.sub_add_cancel]
      · simp [hkj, Rat.add_zero]
    have hceq_perm : ∀ k : Fin n, ceq (perm k) = ceq k := by
      intro k
      dsimp [ceq]
      by_cases hki : k = i
      · subst hki
        have hne : k ≠ j := hij
        simp [hpij, hne]
      · by_cases hkj : k = j
        · subst hkj
          have hpne : perm k ≠ k := by
            rw [hpji]
            exact hij
          simp [hpji]
        · have hpne : perm k ≠ j := by
            intro hpeq
            apply hki
            exact hp_inj k i (hpeq.trans hpij.symm)
          simp [hkj, hpne, hc_other k hki hkj]
    have hceq_sum :
        finSum n (fun k => ceq k * x (perm k)) = finSum n (fun k => ceq k * x k) := by
      have h1 : finSum n (fun k => ceq k * x (perm k)) =
          finSum n (fun k => ceq (perm k) * x (perm k)) := by
        apply finSum_congr
        intro k
        rw [hceq_perm k]
      rw [h1]
      exact finSum_inj_eq (g := fun k => ceq k * x k) hp_inj
    have hdelta_perm :
        finSum n (fun k => delta k * x (perm k)) = (c j - c i) * x i := by
      have heq : ∀ k : Fin n,
          delta k * x (perm k) = if k = j then (c j - c i) * x i else 0 := by
        intro k
        dsimp [delta]
        by_cases hkj : k = j
        · subst hkj; simp [hpji]
        · simp [hkj, Rat.zero_mul]
      rw [finSum_congr heq, finSum_single]
    have hdelta_orig :
        finSum n (fun k => delta k * x k) = (c j - c i) * x j := by
      have heq : ∀ k : Fin n,
          delta k * x k = if k = j then (c j - c i) * x j else 0 := by
        intro k
        dsimp [delta]
        by_cases hkj : k = j
        · subst hkj; simp
        · simp [hkj, Rat.zero_mul]
      rw [finSum_congr heq, finSum_single]
    have hdiff_nn : 0 ≤ c j - c i := (Rat.le_iff_sub_nonneg (c i) (c j)).mp hc_mono
    have hdelta_le : (c j - c i) * x i ≤ (c j - c i) * x j :=
      Rat.mul_le_mul_of_nonneg_left hx_le hdiff_nn
    unfold mipObj
    have hlhs : finSum n (fun k => c k * x (perm k)) =
        finSum n (fun k => ceq k * x (perm k)) + finSum n (fun k => delta k * x (perm k)) := by
      have h1 : ∀ k, c k * x (perm k) = ceq k * x (perm k) + delta k * x (perm k) := by
        intro k
        rw [hc_split k, Rat.add_mul]
      rw [finSum_congr h1, finSum_add]
    have hrhs : finSum n (fun k => c k * x k) =
        finSum n (fun k => ceq k * x k) + finSum n (fun k => delta k * x k) := by
      have h1 : ∀ k, c k * x k = ceq k * x k + delta k * x k := by
        intro k
        rw [hc_split k, Rat.add_mul]
      rw [finSum_congr h1, finSum_add]
    rw [hlhs, hrhs, hceq_sum, hdelta_perm, hdelta_orig]
    exact Rat.add_le_add_left.mpr hdelta_le

/-- Under monotone non-decreasing costs `c_i ≤ c_j` on symmetric constraint-invariant columns,
swapping `x_i < x_j` preserves feasibility and improves or maintains objective. -/
theorem monotone_orbit_order_sound
    {n m : Nat} {P : MipSpec n m} {perm : Fin n → Fin n}
    (hinv : ∀ k : Fin n, perm (perm k) = k)
    {i j : Fin n} (hpij : perm i = j)
    (hfeas_perm : ∀ x : Fin n → Rat, MipFeasible P x → MipFeasible P (fun k => x (perm k)))
    (hc_other : ∀ k : Fin n, k ≠ i → k ≠ j → P.obj (perm k) = P.obj k)
    (hc_mono : P.obj i ≤ P.obj j)
    {x : Fin n → Rat} (hx : MipFeasible P x) (hx_lt : x i < x j) :
    MipFeasible P (fun k => x (perm k)) ∧
      x (perm j) ≤ x (perm i) ∧
      mipObj P.obj (fun k => x (perm k)) ≤ mipObj P.obj x := by
  have hpji : perm j = i := by
    rw [← hpij, hinv i]
  refine ⟨hfeas_perm x hx, ?_, ?_⟩
  · rw [hpji, hpij]
    exact Rat.le_of_lt hx_lt
  · exact mipObj_perm_le_of_mono hinv hpij hc_other hc_mono (Rat.le_of_lt hx_lt)

/-! ### 4. Executable Specs for `Apps/RulesCheck.lean` -/

/-- Extended rational bound supporting `-∞` and `+∞`. -/
inductive ExtRat where
  | negInf
  | fin (r : Rat)
  | posInf
  deriving DecidableEq, Repr, Inhabited

/-- Checks whether an extended lower bound is non-negative (`r ≥ 0` or `+∞`). -/
def ExtRat.isNonneg : ExtRat → Bool
  | .negInf => false
  | .fin r => decide (0 ≤ r)
  | .posInf => true

/-- Term descriptor for executable row activity evaluation (supporting `none` for `±∞`). -/
structure ActivityTerm where
  coeff : Rat
  lb : Option Rat
  ub : Option Rat
  isInt : Bool

/-- Per-term implied quotients and rounded bounds. -/
structure ActivityTermOut where
  quotUpper : Option Rat
  quotLower : Option Rat
  impliedUB : Option Rat
  impliedLB : Option Rat

/-- Full executable activity summary output matching `evalRowActivitySummary` + `residualQuotient`. -/
structure ActivitySummaryOut where
  minInfCount : Nat
  maxInfCount : Nat
  minAct : Option Rat
  maxAct : Option Rat
  slack : Option Rat
  deficit : Option Rat
  terms : Array ActivityTermOut

private def termMinOpt (t : ActivityTerm) : Option Rat :=
  if 0 < t.coeff then t.lb.map (t.coeff * ·)
  else if t.coeff < 0 then t.ub.map (t.coeff * ·)
  else some 0

private def termMaxOpt (t : ActivityTerm) : Option Rat :=
  if 0 < t.coeff then t.ub.map (t.coeff * ·)
  else if t.coeff < 0 then t.lb.map (t.coeff * ·)
  else some 0

private def evalActivityTermSpec (rhs : Rat) (hasLo hasHi : Bool)
    (minInfCount : Nat) (minFinSum : Rat) (maxInfCount : Nat) (maxFinSum : Rat)
    (t : ActivityTerm) : ActivityTermOut :=
  if t.coeff == 0 then
    { quotUpper := none, quotLower := none, impliedUB := none, impliedLB := none }
  else
    let minOtherOpt : Option Rat :=
      match termMinOpt t with
      | some v => if minInfCount == 0 then some (minFinSum - v) else none
      | none => if minInfCount == 1 then some minFinSum else none
    let maxOtherOpt : Option Rat :=
      match termMaxOpt t with
      | some v => if maxInfCount == 0 then some (maxFinSum - v) else none
      | none => if maxInfCount == 1 then some maxFinSum else none
    let quotUpper := minOtherOpt.map fun mo => (rhs - mo) / t.coeff
    let quotLower := maxOtherOpt.map fun mo => (rhs - mo) / t.coeff
    let impliedUB :=
      if hasHi && decide (0 < t.coeff) then
        quotUpper.map (roundUpper t.isInt)
      else if hasLo && decide (t.coeff < 0) then
        quotLower.map (roundUpper t.isInt)
      else
        none
    let impliedLB :=
      if hasHi && decide (t.coeff < 0) then
        quotUpper.map (roundLower t.isInt)
      else if hasLo && decide (0 < t.coeff) then
        quotLower.map (roundLower t.isInt)
      else
        none
    { quotUpper, quotLower, impliedUB, impliedLB }

/-- Evaluates exact row activity summary and per-term residual quotients / implied bounds. -/
def evalActivitySummarySpec (rhs : Rat) (hasLo hasHi : Bool)
    (terms : Array ActivityTerm) : ActivitySummaryOut :=
  let (minInfCount, minFinSum) := terms.foldl (fun (acc : Nat × Rat) t =>
    match termMinOpt t with
    | some v => (acc.1, acc.2 + v)
    | none => (acc.1 + 1, acc.2)) (0, 0)
  let (maxInfCount, maxFinSum) := terms.foldl (fun (acc : Nat × Rat) t =>
    match termMaxOpt t with
    | some v => (acc.1, acc.2 + v)
    | none => (acc.1 + 1, acc.2)) (0, 0)
  let minActOpt := if minInfCount == 0 then some minFinSum else none
  let maxActOpt := if maxInfCount == 0 then some maxFinSum else none
  let slackOpt := minActOpt.map (rhs - ·)
  let deficitOpt := maxActOpt.map (rhs - ·)
  let outTerms := terms.map
    (evalActivityTermSpec rhs hasLo hasHi minInfCount minFinSum maxInfCount maxFinSum)
  { minInfCount, maxInfCount, minAct := minActOpt, maxAct := maxActOpt,
    slack := slackOpt, deficit := deficitOpt, terms := outTerms }

/-- Sparse row representation for executable orbit verification. -/
structure OrbitWorkRow where
  active : Bool
  cols : Array Nat
  vals : Array Rat
  sense : Sense
  rhs : Rat
  deriving Repr, Inhabited

private def rowPairsList (r : OrbitWorkRow) : List (Nat × Rat) :=
  List.zip r.cols.toList r.vals.toList

private def insertSortedPairList : List (Nat × Rat) → (Nat × Rat) → Option (List (Nat × Rat))
  | [], p => some [p]
  | cur :: rest, p =>
    if p.1 == cur.1 then
      none
    else if p.1 < cur.1 then
      some (p :: cur :: rest)
    else
      (insertSortedPairList rest p).map (cur :: ·)

private def sortUniqueRowPairsList (pairs : List (Nat × Rat)) : Option (List (Nat × Rat)) :=
  pairs.foldlM insertSortedPairList []

private def buildRowTupleSlotsAux (numRows gCount : Nat) (rowTuples : Array Nat) :
    Nat → Array (Option (Nat × Nat)) → Option (Array (Option (Nat × Nat)))
  | 0, acc => some acc
  | k + 1, acc =>
    match buildRowTupleSlotsAux numRows gCount rowTuples k acc with
    | none => none
    | some slots =>
      if hlt : k < rowTuples.size then
        let rIdx := rowTuples[k]
        if hr : rIdx < slots.size then
          match slots[rIdx] with
          | some _ => none
          | none => some (slots.set rIdx (some (k / gCount, k % gCount)) hr)
        else
          none
      else
        none

private def buildRowTupleSlots (numRows gCount : Nat) (rowTuples : Array Nat) :
    Option (Array (Option (Nat × Nat))) :=
  if gCount == 0 then none
  else buildRowTupleSlotsAux numRows gCount rowTuples rowTuples.size (Array.replicate numRows none)

/-- Executable `S_G` orbit symmetry verifier matching `verifyOrbitSymmetry` in `presolve.go`,
strictly enforcing `gCount ≥ 2`, `numVars > 0`, no duplicate columns in any active row,
valid `rowState ∈ {-2, -1} ∪ [0, gCount)`, exact bijective `rowTuples` coverage of every
non-negative `rowState` row, orbit invariance on `rowState == -2` rows, and orbit-disjointness
on active `rowState == -1` rows. -/
def verifyOrbitSymmetrySpec
    (numVars : Nat)
    (isInt : Array Bool)
    (lb ub : Array ExtRat)
    (rows : Array OrbitWorkRow)
    (gCount : Nat)
    (orbits rowTuples : Array Nat)
    (colOrbit colGroup rowState : Array Int) : Bool :=
  if gCount < 2 || numVars == 0 || orbits.size == 0 ||
      orbits.size % gCount != 0 || rowTuples.size % gCount != 0 then
    false
  else if isInt.size != numVars || lb.size != numVars || ub.size != numVars ||
      colOrbit.size != numVars || colGroup.size != numVars || rowState.size != rows.size then
    false
  else
    let numOrbits := orbits.size / gCount
    let numTuples := rowTuples.size / gCount
    -- 1. Verify orbit table consistency, bijectivity with `colOrbit`/`colGroup`, and domain equality.
    let orbsOk := (List.range numOrbits).all fun orbID =>
      let base := orbID * gCount
      let c0 := orbits[base]!
      if c0 ≥ numVars then false
      else
        (List.range gCount).all fun g =>
          let cg := orbits[base + g]!
          if _hcg : cg < numVars then
            colOrbit[cg]! == (orbID : Int) &&
            colGroup[cg]! == (g : Int) &&
            (g == 0 ||
              (isInt[cg]! == isInt[c0]! && lb[cg]! == lb[c0]! && ub[cg]! == ub[c0]!))
          else
            false
    let colMapOk := (List.range numVars).all fun c =>
      let orb := colOrbit[c]!
      let grp := colGroup[c]!
      if orb < 0 then
        grp < 0
      else
        orb < (numOrbits : Int) && 0 ≤ grp && grp < (gCount : Int) &&
        orbits[orb.toNat * gCount + grp.toNat]! == c
    if !orbsOk || !colMapOk then
      false
    else
      -- 2. Verify `rowState` ranges and exact bijective `rowTuples` coverage of all `rowState ≥ 0` rows.
      match buildRowTupleSlots rows.size gCount rowTuples with
      | none => false
      | some slots =>
        let rowStateOk := (List.range rows.size).all fun i =>
          let st := rowState[i]!
          if st == -2 || st == -1 then
            (slots[i]!).isNone
          else if 0 ≤ st && st < (gCount : Int) then
            match slots[i]! with
            | some (_, g) => decide ((g : Int) = st)
            | none => false
          else
            false
        if !rowStateOk then
          false
        else
          -- 3. Verify row tuples map identically under group actions `g = 0 .. gCount - 1`.
          let tuplesOk := (List.range numTuples).all fun tIdx =>
            let tBase := tIdx * gCount
            let r0Idx := rowTuples[tBase]!
            if hr0 : r0Idx < rows.size then
              let r0 := rows[r0Idx]
              if !r0.active || rowState[r0Idx]! != 0 || r0.cols.size != r0.vals.size then
                false
              else
                match sortUniqueRowPairsList (rowPairsList r0) with
                | none => false
                | some sorted0 =>
                  let r0ColsOk := sorted0.all fun (c0, _) =>
                    c0 < numVars && (colOrbit[c0]! < 0 || colGroup[c0]! == 0)
                  if !r0ColsOk then
                    false
                  else
                    (List.range (gCount - 1)).all fun gOff =>
                      let g := gOff + 1
                      let rgIdx := rowTuples[tBase + g]!
                      if hrg : rgIdx < rows.size then
                        let rg := rows[rgIdx]
                        if !rg.active || rowState[rgIdx]! != (g : Int) ||
                            rg.sense != r0.sense || rg.rhs != r0.rhs ||
                            rg.cols.size != r0.cols.size || rg.vals.size != r0.vals.size then
                          false
                        else
                          let mapped0 := sorted0.map fun (c0, v0) =>
                            let orb := colOrbit[c0]!
                            let cg := if orb ≥ 0 then orbits[orb.toNat * gCount + g]! else c0
                            (cg, v0)
                          match sortUniqueRowPairsList mapped0, sortUniqueRowPairsList (rowPairsList rg) with
                          | some mSorted, some gSorted => mSorted == gSorted
                          | _, _ => false
                      else
                        false
            else
              false
          -- 4. Verify active non-tuple rows:
          --    `rowState == -2` must be invariant on every orbit;
          --    `rowState == -1` must touch zero orbit columns.
          let invOk := (List.range rows.size).all fun i =>
            let r : OrbitWorkRow := rows[i]!
            let st := rowState[i]!
            if !r.active || st ≥ 0 then
              true
            else if r.cols.size != r.vals.size then
              false
            else
              match sortUniqueRowPairsList (rowPairsList r) with
              | none => false
              | some sortedR =>
                if !(sortedR.all fun (c, _) => c < numVars) then
                  false
                else if st == -1 then
                  sortedR.all fun (c, _) => colOrbit[c]! < 0
                else
                  (List.range numOrbits).all fun orbID =>
                    let base := orbID * gCount
                    let c0 := orbits[base]!
                    match sortedR.find? (fun p => p.1 == c0) with
                    | none =>
                      (List.range gCount).all fun g =>
                        let cg := orbits[base + g]!
                        !(sortedR.any (fun p => p.1 == cg))
                    | some (_, v0) =>
                      (List.range gCount).all fun g =>
                        let cg := orbits[base + g]!
                        sortedR.any (fun p => p.1 == cg && p.2 == v0)
          tuplesOk && invOk

/-- Checks whether objective coefficients are identical across every orbit. -/
def exactOrbitObjSpec (gCount : Nat) (orbits : Array Nat) (obj : Array Rat) : Bool :=
  if gCount < 2 || orbits.size == 0 || orbits.size % gCount != 0 then
    false
  else
    let numOrbits := orbits.size / gCount
    (List.range numOrbits).all fun orbID =>
      let base := orbID * gCount
      let c0 := orbits[base]!
      if hc0 : c0 < obj.size then
        (List.range (gCount - 1)).all fun gOff =>
          let cg := orbits[base + gOff + 1]!
          if hcg : cg < obj.size then obj[cg] == obj[c0] else false
      else
        false

/-- Checks whether orbit lower bounds are non-negative and objectives are non-decreasing. -/
def monotoneOrbitNonNegSpec (gCount : Nat) (orbits : Array Nat)
    (lb : Array ExtRat) (obj : Array Rat) : Bool :=
  if gCount < 2 || orbits.size == 0 || orbits.size % gCount != 0 then
    false
  else
    let numOrbits := orbits.size / gCount
    (List.range numOrbits).all fun orbID =>
      let base := orbID * gCount
      let c0 := orbits[base]!
      if hlb : c0 < lb.size then
        if !lb[c0].isNonneg then
          false
        else
          (List.range (gCount - 1)).all fun g =>
            let ca := orbits[base + g]!
            let cb := orbits[base + g + 1]!
            if hca : ca < obj.size then
              if hcb : cb < obj.size then obj[ca] ≤ obj[cb] else false
            else
              false
      else
        false

/-- Soundness of the executable `hasInfeasibleIntEqSpec` decision procedure. -/
theorem hasInfeasibleIntEqSpec_sound
    {coeffs : List Int} {lo hi : Int}
    (hinfeas : hasInfeasibleIntEqSpec coeffs lo hi = true) :
    ¬ ∃ x : Fin coeffs.length → Int,
      lo ≤ intFinSum coeffs.length (fun j => coeffs[j.1] * x j) ∧
      intFinSum coeffs.length (fun j => coeffs[j.1] * x j) ≤ hi := by
  unfold hasInfeasibleIntEqSpec at hinfeas
  dsimp only at hinfeas
  split at hinfeas
  · next hg =>
    have hgap : divCeilSpec lo (listGcd coeffs) * listGcd coeffs > hi := of_decide_eq_true hinfeas
    have hdvd : ∀ j : Fin coeffs.length, listGcd coeffs ∣ coeffs[j.1] :=
      fun j => listGcd_dvd (List.getElem_mem j.isLt)
    exact gcd_row_infeasible_sound hg hdvd hgap
  · next hg_le =>
    have hgnn : 0 ≤ listGcd coeffs := listGcd_nonneg coeffs
    have hg0 : listGcd coeffs = 0 := by omega
    rintro ⟨x, hlo, hhi⟩
    have hzero : ∀ j : Fin coeffs.length, coeffs[j.1] = 0 := by
      intro j
      have hd : listGcd coeffs ∣ coeffs[j.1] := listGcd_dvd (List.getElem_mem j.isLt)
      rw [hg0] at hd
      exact Int.zero_dvd.mp hd
    have hsum : intFinSum coeffs.length (fun j => coeffs[j.1] * x j) = 0 := by
      have heq : intFinSum coeffs.length (fun j => coeffs[j.1] * x j) =
          intFinSum coeffs.length (fun _ => (0 : Int)) := by
        apply intFinSum_congr
        intro j
        rw [hzero j, Int.zero_mul]
      rw [heq, intFinSum_zero]
    rw [hsum] at hlo hhi
    have hor : hi < 0 ∨ 0 < lo := by
      simpa [Bool.or_eq_true, decide_eq_true_eq] using hinfeas
    omega

private theorem rat_add_sub_cancel_mid (x y z : Rat) : (x + y) - z + -y = x - z := by
  rw [Rat.sub_eq_add_neg, Rat.add_assoc (x + y), Rat.add_comm (-z) (-y),
    ← Rat.add_assoc (x + y) (-y) (-z), Rat.add_assoc x y (-y), Rat.add_neg_cancel,
    Rat.add_zero, ← Rat.sub_eq_add_neg]

/-- Completeness of `presolveCanSkipTerm` for lower-row (`maxAct - rhs`) tightening. -/
theorem presolveCanSkipTerm_lower_sound
    {n : Nat} {a l u : Fin n → Rat} {rhs slackLo absA : Rat} {isInt : Fin n → Bool}
    {k : Fin n}
    (hslack_le : slackLo ≤ maxAct n a l u - rhs)
    (hgt : absA * (u k - l k) < slackLo) :
    (0 < a k → absA = a k →
      (rhs - maxOther n a l u k) / a k < l k ∧
      ((isInt k = true → (l k).den = 1) →
        roundLower (isInt k) ((rhs - maxOther n a l u k) / a k) ≤ l k)) ∧
    (a k < 0 → absA = -a k →
      u k < (rhs - maxOther n a l u k) / a k ∧
      ((isInt k = true → (u k).den = 1) →
        u k ≤ roundUpper (isInt k) ((rhs - maxOther n a l u k) / a k))) := by
  have hlt_diff : absA * (u k - l k) < maxAct n a l u - rhs := rat_lt_of_lt_of_le hgt hslack_le
  rw [maxAct_eq_add_termMax n a l u k] at hlt_diff
  constructor
  · intro hak hAbsEq
    rw [hAbsEq] at hlt_diff
    have htm : termMax (a k) (l k) (u k) = a k * u k := by
      unfold termMax
      simp [Rat.le_of_lt hak]
    rw [htm] at hlt_diff
    have h1 : a k * u k - a k * l k < (maxOther n a l u k + a k * u k) - rhs := by
      simpa [rat_mul_sub] using hlt_diff
    have h2 : -a k * l k < maxOther n a l u k - rhs := by
      have hadd := (Rat.add_lt_add_right (c := -(a k * u k))).mpr h1
      have hlhs : a k * u k - a k * l k + -(a k * u k) = -a k * l k := by
        rw [Rat.sub_eq_add_neg, Rat.add_comm (a k * u k), Rat.add_assoc,
          Rat.add_neg_cancel, Rat.add_zero, Rat.neg_mul]
      have hrhs : (maxOther n a l u k + a k * u k) - rhs + -(a k * u k) =
          maxOther n a l u k - rhs :=
        rat_add_sub_cancel_mid _ _ _
      rwa [hlhs, hrhs] at hadd
    have h3 : rhs - maxOther n a l u k < a k * l k := by
      have hneg := Rat.neg_lt_neg h2
      have hl : -(maxOther n a l u k - rhs) = rhs - maxOther n a l u k := Rat.neg_sub _ _
      have hr : -(-a k * l k) = a k * l k := by
        rw [Rat.neg_mul, Rat.neg_neg]
      rwa [hl, hr] at hneg
    have hraw : (rhs - maxOther n a l u k) / a k < l k := by
      rw [Rat.div_lt_iff hak, Rat.mul_comm]
      exact h3
    refine ⟨hraw, ?_⟩
    intro hint
    unfold roundLower
    cases hkInt : isInt k
    · exact Rat.le_of_lt hraw
    · exact rat_lt_int_imp_ceil_le (hint hkInt) hraw
  · intro hak hAbsEq
    rw [hAbsEq] at hlt_diff
    have htm : termMax (a k) (l k) (u k) = a k * l k := by
      unfold termMax
      have hnot : ¬ (0 ≤ a k) := Rat.not_le.mpr hak
      simp [hnot]
    rw [htm] at hlt_diff
    have hneg_pos : 0 < -a k := Rat.lt_neg_iff.mpr (by simpa using hak)
    have h1 : (-a k) * u k - (-a k) * l k < (maxOther n a l u k + a k * l k) - rhs := by
      simpa [rat_mul_sub] using hlt_diff
    have h2 : (-a k) * u k < maxOther n a l u k - rhs := by
      have hadd := (Rat.add_lt_add_right (c := -(a k * l k))).mpr h1
      have hlhs : (-a k) * u k - (-a k) * l k + -(a k * l k) = (-a k) * u k := by
        rw [Rat.sub_eq_add_neg, Rat.neg_mul, Rat.neg_mul, Rat.neg_neg,
          Rat.add_assoc, Rat.add_neg_cancel, Rat.add_zero]
      have hrhs : (maxOther n a l u k + a k * l k) - rhs + -(a k * l k) =
          maxOther n a l u k - rhs :=
        rat_add_sub_cancel_mid _ _ _
      rwa [hlhs, hrhs] at hadd
    have hraw_neg : u k < (maxOther n a l u k - rhs) / (-a k) := by
      rw [Rat.lt_div_iff hneg_pos, Rat.mul_comm]
      exact h2
    have hraw : u k < (rhs - maxOther n a l u k) / a k := by
      have heq : (maxOther n a l u k - rhs) / (-a k) = (rhs - maxOther n a l u k) / a k := by
        rw [rat_div_neg_eq_neg_div_pos, Rat.neg_sub]
      rwa [← heq]
    refine ⟨hraw, ?_⟩
    intro hint
    unfold roundUpper
    cases hkInt : isInt k
    · exact Rat.le_of_lt hraw
    · exact int_lt_rat_imp_le_floor (hint hkInt) hraw

/-- Constructs an `Aut P` from a column bijection paired with a row-mapping function `rowPerm`
that preserves each row's sense, RHS, and permuted coefficients. -/
def mkRowAut {n m : Nat} (P : MipSpec n m)
    (perm inv : Fin n → Fin n)
    (hleft : ∀ j, inv (perm j) = j)
    (hright : ∀ j, perm (inv j) = j)
    (hisInt : ∀ j, P.isInt (perm j) = P.isInt j)
    (hlb : ∀ j, P.l (perm j) = P.l j)
    (hub : ∀ j, P.u (perm j) = P.u j)
    (hobj : ∀ j, P.obj (perm j) = P.obj j)
    (rowPerm : Fin m → Fin m)
    (hsense : ∀ i, (P.rows i).sense = (P.rows (rowPerm i)).sense)
    (hrhs : ∀ i, (P.rows i).rhs = (P.rows (rowPerm i)).rhs)
    (hcoeffs : ∀ i j, (P.rows i).coeffs j = (P.rows (rowPerm i)).coeffs (perm j)) :
    Aut P :=
  { perm := perm
    inv := inv
    left_inv := hleft
    right_inv := hright
    preserves_isInt := hisInt
    preserves_lb := hlb
    preserves_ub := hub
    preserves_obj := hobj
    preserves_rows := by
      intro x hx i
      have hp_inj : ∀ a b : Fin n, perm a = perm b → a = b := by
        intro a b h
        have := congrArg inv h
        rwa [hleft, hleft] at this
      have hsum :
          finSum n (fun j => (P.rows i).coeffs j * x (perm j)) =
          finSum n (fun j => (P.rows (rowPerm i)).coeffs j * x j) := by
        have h1 :
            finSum n (fun j => (P.rows i).coeffs j * x (perm j)) =
            finSum n (fun j => (P.rows (rowPerm i)).coeffs (perm j) * x (perm j)) := by
          apply finSum_congr
          intro j
          rw [hcoeffs i j]
        rw [h1]
        exact finSum_inj_eq (g := fun j => (P.rows (rowPerm i)).coeffs j * x j) hp_inj
      have hhold := hx (rowPerm i)
      unfold RowSpec.holds at hhold ⊢
      rw [hsum, hsense i, hrhs i]
      exact hhold }

/-! ### 5. Soundness of `evalActivitySummarySpec` and `binStrengthenFromRowSpec` over `Option Rat` Bounds -/

/-- Mixed-integer single-row feasibility predicate over optional (`±∞`) variable bounds. -/
def RowFeasibleOpt (terms : Array ActivityTerm) (rhs : Rat) (hasLo hasHi : Bool)
    (x : Fin terms.size → Rat) : Prop :=
  (∀ j : Fin terms.size,
    (∀ l, terms[j.val].lb = some l → l ≤ x j) ∧
    (∀ u, terms[j.val].ub = some u → x j ≤ u)) ∧
  (∀ j : Fin terms.size, terms[j.val].isInt = true → ∃ z : Int, x j = (z : Rat)) ∧
  (hasHi = true → finSum terms.size (fun j => terms[j.val].coeff * x j) ≤ rhs) ∧
  (hasLo = true → rhs ≤ finSum terms.size (fun j => terms[j.val].coeff * x j))

private theorem termMinOpt_le_mul {t : ActivityTerm} {xj v : Rat}
    (hL : ∀ l, t.lb = some l → l ≤ xj)
    (hU : ∀ u, t.ub = some u → xj ≤ u)
    (hv : termMinOpt t = some v) : v ≤ t.coeff * xj := by
  unfold termMinOpt at hv
  by_cases hpos : 0 < t.coeff
  · simp only [hpos, ↓reduceIte] at hv
    cases hlb : t.lb with
    | none => simp [hlb] at hv
    | some l =>
      simp only [hlb, Option.map_some, Option.some.injEq] at hv
      subst hv
      exact Rat.mul_le_mul_of_nonneg_left (hL l hlb) (Rat.le_of_lt hpos)
  · simp only [hpos, ↓reduceIte] at hv
    by_cases hneg : t.coeff < 0
    · simp only [hneg, ↓reduceIte] at hv
      cases hub : t.ub with
      | none => simp [hub] at hv
      | some u =>
        simp only [hub, Option.map_some, Option.some.injEq] at hv
        subst hv
        have hnn : 0 ≤ -t.coeff := Rat.le_neg_iff.mpr (by simpa using Rat.le_of_lt hneg)
        have hm := Rat.mul_le_mul_of_nonneg_left (hU u hub) hnn
        rw [Rat.neg_mul, Rat.neg_mul, Rat.neg_le_neg_iff] at hm
        exact hm
    · simp only [hneg, ↓reduceIte, Option.some.injEq] at hv
      subst hv
      have hc0 : t.coeff = 0 := Rat.le_antisymm (Rat.not_lt.mp hpos) (Rat.not_lt.mp hneg)
      rw [hc0, Rat.zero_mul]
      exact Rat.le_refl

private theorem mul_le_termMaxOpt {t : ActivityTerm} {xj v : Rat}
    (hL : ∀ l, t.lb = some l → l ≤ xj)
    (hU : ∀ u, t.ub = some u → xj ≤ u)
    (hv : termMaxOpt t = some v) : t.coeff * xj ≤ v := by
  unfold termMaxOpt at hv
  by_cases hpos : 0 < t.coeff
  · simp only [hpos, ↓reduceIte] at hv
    cases hub : t.ub with
    | none => simp [hub] at hv
    | some u =>
      simp only [hub, Option.map_some, Option.some.injEq] at hv
      subst hv
      exact Rat.mul_le_mul_of_nonneg_left (hU u hub) (Rat.le_of_lt hpos)
  · simp only [hpos, ↓reduceIte] at hv
    by_cases hneg : t.coeff < 0
    · simp only [hneg, ↓reduceIte] at hv
      cases hlb : t.lb with
      | none => simp [hlb] at hv
      | some l =>
        simp only [hlb, Option.map_some, Option.some.injEq] at hv
        subst hv
        have hnn : 0 ≤ -t.coeff := Rat.le_neg_iff.mpr (by simpa using Rat.le_of_lt hneg)
        have hm := Rat.mul_le_mul_of_nonneg_left (hL l hlb) hnn
        rw [Rat.neg_mul, Rat.neg_mul, Rat.neg_le_neg_iff] at hm
        exact hm
    · simp only [hneg, ↓reduceIte, Option.some.injEq] at hv
      subst hv
      have hc0 : t.coeff = 0 := Rat.le_antisymm (Rat.not_lt.mp hpos) (Rat.not_lt.mp hneg)
      rw [hc0, Rat.zero_mul]
      exact Rat.le_refl

private def countNoneFin : (n : Nat) → (Fin n → Option Rat) → Nat
  | 0, _ => 0
  | n + 1, f => countNoneFin n (fun i => f i.castSucc) + (if (f (Fin.last n)).isNone then 1 else 0)

private theorem countNoneFin_zero_iff {n : Nat} {f : Fin n → Option Rat} :
    countNoneFin n f = 0 ↔ ∀ i : Fin n, (f i).isSome = true := by
  induction n with
  | zero =>
    constructor
    · intro _ i; exact Fin.elim0 i
    · intro _; rfl
  | succ n ih =>
    simp only [countNoneFin]
    constructor
    · intro h i
      have hprev : countNoneFin n (fun j => f j.castSucc) = 0 := by omega
      have hlast : (if (f (Fin.last n)).isNone then 1 else 0) = 0 := by omega
      by_cases hi : i.val < n
      · have heq : i = (⟨i.val, hi⟩ : Fin n).castSucc := Fin.ext rfl
        rw [heq]; exact ih.mp hprev ⟨i.val, hi⟩
      · have hval : i = Fin.last n := Fin.ext (by simp [Fin.val_last]; have := i.isLt; omega)
        rw [hval]
        cases hf : f (Fin.last n) with
        | none => simp [hf] at hlast
        | some _ => rfl
    · intro hall
      have hprev := ih.mpr (fun j => hall j.castSucc)
      have hlast := hall (Fin.last n)
      cases hf : f (Fin.last n) with
      | none => simp [hf] at hlast
      | some _ => simp [hprev]

private theorem countNoneFin_one_other_some {n : Nat} {f : Fin n → Option Rat} {k : Fin n}
    (h1 : countNoneFin n f = 1) (hk : f k = none) :
    ∀ j : Fin n, j ≠ k → (f j).isSome = true := by
  induction n with
  | zero => exact Fin.elim0 k
  | succ n ih =>
    simp only [countNoneFin] at h1
    by_cases hkl : k = Fin.last n
    · subst hkl
      simp only [hk, Option.isNone_none, ↓reduceIte] at h1
      have hprev0 : countNoneFin n (fun i => f i.castSucc) = 0 := by omega
      intro j hne
      have hjlt : j.val < n := by
        have := j.isLt
        have hvalne : j.val ≠ n := fun h => hne (Fin.ext h)
        omega
      have heq : j = (⟨j.val, hjlt⟩ : Fin n).castSucc := Fin.ext rfl
      rw [heq]
      exact countNoneFin_zero_iff.mp hprev0 ⟨j.val, hjlt⟩
    · have hklt : k.val < n := by
        have := k.isLt
        have hvalne : k.val ≠ n := fun h => hkl (Fin.ext h)
        omega
      let k0 : Fin n := ⟨k.val, hklt⟩
      have hk0 : k = k0.castSucc := Fin.ext rfl
      have hge1 : 1 ≤ countNoneFin n (fun i => f i.castSucc) := by
        apply Classical.byContradiction
        intro hlt1
        have h0 : countNoneFin n (fun i => f i.castSucc) = 0 := by omega
        have hsome := countNoneFin_zero_iff.mp h0 k0
        rw [← hk0, hk] at hsome
        simp at hsome
      have hprev1 : countNoneFin n (fun i => f i.castSucc) = 1 := by omega
      have hlast0 : (if (f (Fin.last n)).isNone then 1 else 0) = 0 := by omega
      intro j hne
      by_cases hj : j.val < n
      · let j0 : Fin n := ⟨j.val, hj⟩
        have hj0 : j = j0.castSucc := Fin.ext rfl
        have hne0 : j0 ≠ k0 := fun heq => hne (by rw [hj0, hk0, heq])
        rw [hj0]
        exact ih hprev1 (by rwa [← hk0]) j0 hne0
      · have hjl : j = Fin.last n := Fin.ext (by simp [Fin.val_last]; have := j.isLt; omega)
        rw [hjl]
        cases hf : f (Fin.last n) with
        | none => simp [hf] at hlast0
        | some _ => rfl

private theorem foldl_opt_sum_eq {α : Type} (xs : Array α) (g : α → Option Rat) :
    xs.foldl (fun (acc : Nat × Rat) t =>
      match g t with
      | some v => (acc.1, acc.2 + v)
      | none => (acc.1 + 1, acc.2)) (0, 0) =
    (countNoneFin xs.size (fun i => g xs[i.val]),
     finSum xs.size (fun i => (g xs[i.val]).getD 0)) := by
  rw [← Array.foldl_toList]
  suffices h : ∀ (k : Nat) (hk : k ≤ xs.size),
      (xs.toList.take k).foldl (fun (acc : Nat × Rat) t =>
        match g t with
        | some v => (acc.1, acc.2 + v)
        | none => (acc.1 + 1, acc.2)) (0, 0) =
      (countNoneFin k (fun i => g (xs[i.val]'(by omega))),
       finSum k (fun i => (g (xs[i.val]'(by omega))).getD 0)) by
    have hfull := h xs.size (Nat.le_refl _)
    have htake : xs.toList.take xs.size = xs.toList := List.take_of_length_le (by simp)
    rw [htake] at hfull
    exact hfull
  intro k hk
  induction k with
  | zero => simp [countNoneFin, finSum]
  | succ k ih =>
    have hlt : k < xs.toList.length := by simp; omega
    rw [List.take_succ_eq_append_getElem hlt, List.foldl_append, ih (by omega)]
    simp only [List.foldl_cons, List.foldl_nil, countNoneFin, finSum_succ]
    have hget : xs.toList[k] = xs[k]'(by omega) := rfl
    rw [hget]
    cases hg : g (xs[k]'(by omega)) with
    | none => simp [hg, Rat.add_zero]
    | some v => simp [hg]

/-- Universal soundness of `evalActivitySummarySpec` over arbitrary `Option Rat` bounds,
covering finite activity bounds and both `infCount = 0` and `infCount = 1` implied bounds. -/
theorem evalActivitySummarySpec_sound
    (rhs : Rat) (hasLo hasHi : Bool) (terms : Array ActivityTerm)
    (x : Fin terms.size → Rat)
    (hfeas : RowFeasibleOpt terms rhs hasLo hasHi x) :
    let res := evalActivitySummarySpec rhs hasLo hasHi terms
    (∀ m, res.minAct = some m → m ≤ finSum terms.size (fun j => terms[j.val].coeff * x j)) ∧
    (∀ M, res.maxAct = some M → finSum terms.size (fun j => terms[j.val].coeff * x j) ≤ M) ∧
    (∀ (k : Fin terms.size) (ub : Rat),
      (res.terms[k.val]'(by simp [res, evalActivitySummarySpec])).impliedUB = some ub → x k ≤ ub) ∧
    (∀ (k : Fin terms.size) (lb : Rat),
      (res.terms[k.val]'(by simp [res, evalActivitySummarySpec])).impliedLB = some lb → lb ≤ x k) := by
  rcases hfeas with ⟨hb, hint, hhi, hlo⟩
  dsimp only
  let minF : Fin terms.size → Option Rat := fun j => termMinOpt terms[j.val]
  let maxF : Fin terms.size → Option Rat := fun j => termMaxOpt terms[j.val]
  have hMinFold := foldl_opt_sum_eq terms termMinOpt
  have hMaxFold := foldl_opt_sum_eq terms termMaxOpt
  have hMinOtherLe : ∀ (k : Fin terms.size) (mo : Rat),
      (match termMinOpt terms[k.val] with
       | some v => if countNoneFin terms.size minF == 0 then some (finSum terms.size (fun j => (minF j).getD 0) - v) else none
       | none => if countNoneFin terms.size minF == 1 then some (finSum terms.size (fun j => (minF j).getD 0)) else none) = some mo →
      mo ≤ finSum terms.size (fun j => if j = k then 0 else terms[j.val].coeff * x j) := by
    intro k mo hmo
    cases htk : termMinOpt terms[k.val] with
    | some vk =>
      rw [htk] at hmo
      by_cases h0 : countNoneFin terms.size minF = 0
      · have hbeq : (countNoneFin terms.size minF == 0) = true := beq_iff_eq.mpr h0
        simp only [hbeq, ↓reduceIte, Option.some.injEq] at hmo
        subst hmo
        have hall := countNoneFin_zero_iff.mp h0
        have hsplit := finSum_split_at (fun j => (minF j).getD 0) k
        have hfk : (minF k).getD 0 = vk := by simp [minF, htk]
        rw [hsplit, hfk, Rat.add_sub_cancel]
        apply finSum_le_finSum
        intro j
        by_cases hjk : j = k
        · simp [hjk]
        · simp only [hjk, ↓reduceIte]
          have hjs := hall j
          cases hmj : minF j with
          | none => simp [hmj] at hjs
          | some vj =>
            simp only [Option.getD_some]
            exact termMinOpt_le_mul (hb j).1 (hb j).2 hmj
      · have hbeq : (countNoneFin terms.size minF == 0) = false := beq_false_of_ne h0
        simp [hbeq] at hmo
    | none =>
      rw [htk] at hmo
      by_cases h1 : countNoneFin terms.size minF = 1
      · have hbeq : (countNoneFin terms.size minF == 1) = true := beq_iff_eq.mpr h1
        simp only [hbeq, ↓reduceIte, Option.some.injEq] at hmo
        subst hmo
        have hother := countNoneFin_one_other_some h1 htk
        have hsplit := finSum_split_at (fun j => (minF j).getD 0) k
        have hfk : (minF k).getD 0 = 0 := by simp [minF, htk]
        rw [hsplit, hfk, Rat.add_zero]
        apply finSum_le_finSum
        intro j
        by_cases hjk : j = k
        · simp [hjk]
        · simp only [hjk, ↓reduceIte]
          have hjs := hother j hjk
          cases hmj : minF j with
          | none => simp [hmj] at hjs
          | some vj =>
            simp only [Option.getD_some]
            exact termMinOpt_le_mul (hb j).1 (hb j).2 hmj
      · have hbeq : (countNoneFin terms.size minF == 1) = false := beq_false_of_ne h1
        simp [hbeq] at hmo
  have hMaxOtherGe : ∀ (k : Fin terms.size) (mo : Rat),
      (match termMaxOpt terms[k.val] with
       | some v => if countNoneFin terms.size maxF == 0 then some (finSum terms.size (fun j => (maxF j).getD 0) - v) else none
       | none => if countNoneFin terms.size maxF == 1 then some (finSum terms.size (fun j => (maxF j).getD 0)) else none) = some mo →
      finSum terms.size (fun j => if j = k then 0 else terms[j.val].coeff * x j) ≤ mo := by
    intro k mo hmo
    cases htk : termMaxOpt terms[k.val] with
    | some vk =>
      rw [htk] at hmo
      by_cases h0 : countNoneFin terms.size maxF = 0
      · have hbeq : (countNoneFin terms.size maxF == 0) = true := beq_iff_eq.mpr h0
        simp only [hbeq, ↓reduceIte, Option.some.injEq] at hmo
        subst hmo
        have hall := countNoneFin_zero_iff.mp h0
        have hsplit := finSum_split_at (fun j => (maxF j).getD 0) k
        have hfk : (maxF k).getD 0 = vk := by simp [maxF, htk]
        rw [hsplit, hfk, Rat.add_sub_cancel]
        apply finSum_le_finSum
        intro j
        by_cases hjk : j = k
        · simp [hjk]
        · simp only [hjk, ↓reduceIte]
          have hjs := hall j
          cases hmj : maxF j with
          | none => simp [hmj] at hjs
          | some vj =>
            simp only [Option.getD_some]
            exact mul_le_termMaxOpt (hb j).1 (hb j).2 hmj
      · have hbeq : (countNoneFin terms.size maxF == 0) = false := beq_false_of_ne h0
        simp [hbeq] at hmo
    | none =>
      rw [htk] at hmo
      by_cases h1 : countNoneFin terms.size maxF = 1
      · have hbeq : (countNoneFin terms.size maxF == 1) = true := beq_iff_eq.mpr h1
        simp only [hbeq, ↓reduceIte, Option.some.injEq] at hmo
        subst hmo
        have hother := countNoneFin_one_other_some h1 htk
        have hsplit := finSum_split_at (fun j => (maxF j).getD 0) k
        have hfk : (maxF k).getD 0 = 0 := by simp [maxF, htk]
        rw [hsplit, hfk, Rat.add_zero]
        apply finSum_le_finSum
        intro j
        by_cases hjk : j = k
        · simp [hjk]
        · simp only [hjk, ↓reduceIte]
          have hjs := hother j hjk
          cases hmj : maxF j with
          | none => simp [hmj] at hjs
          | some vj =>
            simp only [Option.getD_some]
            exact mul_le_termMaxOpt (hb j).1 (hb j).2 hmj
      · have hbeq : (countNoneFin terms.size maxF == 1) = false := beq_false_of_ne h1
        simp [hbeq] at hmo
  have hTermEq : ∀ (k : Fin terms.size),
      ((evalActivitySummarySpec rhs hasLo hasHi terms).terms[k.val]'(by simp [evalActivitySummarySpec])) =
        evalActivityTermSpec rhs hasLo hasHi
          (countNoneFin terms.size minF) (finSum terms.size (fun j => (minF j).getD 0))
          (countNoneFin terms.size maxF) (finSum terms.size (fun j => (maxF j).getD 0))
          terms[k.val] := by
    intro k
    have h1 :
        ((evalActivitySummarySpec rhs hasLo hasHi terms).terms[k.val]'(by simp [evalActivitySummarySpec])) =
        evalActivityTermSpec rhs hasLo hasHi
          (terms.foldl (fun (acc : Nat × Rat) t =>
            match termMinOpt t with
            | some v => (acc.1, acc.2 + v)
            | none => (acc.1 + 1, acc.2)) (0, 0)).1
          (terms.foldl (fun (acc : Nat × Rat) t =>
            match termMinOpt t with
            | some v => (acc.1, acc.2 + v)
            | none => (acc.1 + 1, acc.2)) (0, 0)).2
          (terms.foldl (fun (acc : Nat × Rat) t =>
            match termMaxOpt t with
            | some v => (acc.1, acc.2 + v)
            | none => (acc.1 + 1, acc.2)) (0, 0)).1
          (terms.foldl (fun (acc : Nat × Rat) t =>
            match termMaxOpt t with
            | some v => (acc.1, acc.2 + v)
            | none => (acc.1 + 1, acc.2)) (0, 0)).2
          terms[k.val] := by
      unfold evalActivitySummarySpec
      simp only [Array.getElem_map]
    rw [h1, hMinFold, hMaxFold]
  refine ⟨?_, ?_, ?_, ?_⟩
  · intro m hm
    unfold evalActivitySummarySpec at hm
    rw [hMinFold, hMaxFold] at hm
    dsimp only at hm
    split at hm
    · next hbeq =>
      have h0 : countNoneFin terms.size minF = 0 := beq_iff_eq.mp hbeq
      simp only [Option.some.injEq] at hm
      subst hm
      have hall := countNoneFin_zero_iff.mp h0
      apply finSum_le_finSum
      intro j
      have hjs := hall j
      cases hmj : minF j with
      | none => simp [hmj] at hjs
      | some vj =>
        dsimp only [minF] at hmj
        rw [hmj]
        simp only [Option.getD_some]
        exact termMinOpt_le_mul (hb j).1 (hb j).2 hmj
    · contradiction
  · intro M hM
    unfold evalActivitySummarySpec at hM
    rw [hMinFold, hMaxFold] at hM
    dsimp only at hM
    split at hM
    · next hbeq =>
      have h0 : countNoneFin terms.size maxF = 0 := beq_iff_eq.mp hbeq
      simp only [Option.some.injEq] at hM
      subst hM
      have hall := countNoneFin_zero_iff.mp h0
      apply finSum_le_finSum
      intro j
      have hjs := hall j
      cases hmj : maxF j with
      | none => simp [hmj] at hjs
      | some vj =>
        dsimp only [maxF] at hmj
        rw [hmj]
        simp only [Option.getD_some]
        exact mul_le_termMaxOpt (hb j).1 (hb j).2 hmj
    · contradiction
  · intro k ub hub
    rw [hTermEq k] at hub
    unfold evalActivityTermSpec at hub
    by_cases hc0 : terms[k.val].coeff = 0
    · have hbeq : (terms[k.val].coeff == 0) = true := beq_iff_eq.mpr hc0
      simp [hbeq] at hub
    · have hbeq : (terms[k.val].coeff == 0) = false := beq_false_of_ne hc0
      simp only [hbeq, Bool.false_eq_true, ↓reduceIte] at hub
      by_cases h1 : hasHi = true ∧ 0 < terms[k.val].coeff
      · have hb1 : (hasHi && decide (0 < terms[k.val].coeff)) = true := by simp [h1.1, h1.2]
        simp only [hb1, ↓reduceIte] at hub
        cases hmo : (match termMinOpt terms[k.val] with
          | some v => if countNoneFin terms.size minF == 0 then some (finSum terms.size (fun j => (minF j).getD 0) - v) else none
          | none => if countNoneFin terms.size minF == 1 then some (finSum terms.size (fun j => (minF j).getD 0)) else none) with
        | none => rw [hmo] at hub; simp at hub
        | some mo =>
          have hmo' := hMinOtherLe k mo hmo
          rw [hmo] at hub
          simp only [Option.map_some, Option.some.injEq] at hub
          subst hub
          have hsum := hhi h1.1
          rw [finSum_split_at (fun j => terms[j.val].coeff * x j) k] at hsum
          have hak_le : mo + terms[k.val].coeff * x k ≤ rhs :=
            Rat.le_trans (Rat.add_le_add_right.mpr hmo') hsum
          have hle_sub : terms[k.val].coeff * x k ≤ rhs - mo := by
            have := (Rat.add_le_add_left (c := -mo)).mpr hak_le
            rw [← Rat.add_assoc, Rat.neg_add_cancel, Rat.zero_add, Rat.add_comm (-mo), ← Rat.sub_eq_add_neg] at this
            exact this
          have hraw : x k ≤ (rhs - mo) / terms[k.val].coeff :=
            (rat_le_div_iff h1.2).mpr (by rwa [Rat.mul_comm])
          exact roundUpper_sound hraw (hint k)
      · have hb1 : (hasHi && decide (0 < terms[k.val].coeff)) = false := by cases hasHi <;> simp_all
        simp only [hb1, Bool.false_eq_true, ↓reduceIte] at hub
        by_cases h2 : hasLo = true ∧ terms[k.val].coeff < 0
        · have hb2 : (hasLo && decide (terms[k.val].coeff < 0)) = true := by simp [h2.1, h2.2]
          simp only [hb2, ↓reduceIte] at hub
          cases hmo : (match termMaxOpt terms[k.val] with
            | some v => if countNoneFin terms.size maxF == 0 then some (finSum terms.size (fun j => (maxF j).getD 0) - v) else none
            | none => if countNoneFin terms.size maxF == 1 then some (finSum terms.size (fun j => (maxF j).getD 0)) else none) with
          | none => rw [hmo] at hub; simp at hub
          | some mo =>
            have hmo' := hMaxOtherGe k mo hmo
            rw [hmo] at hub
            simp only [Option.map_some, Option.some.injEq] at hub
            subst hub
            have hsum := hlo h2.1
            rw [finSum_split_at (fun j => terms[j.val].coeff * x j) k] at hsum
            have hle_sum : rhs ≤ mo + terms[k.val].coeff * x k :=
              Rat.le_trans hsum (Rat.add_le_add_right.mpr hmo')
            have hneg_pos : 0 < -terms[k.val].coeff := Rat.lt_neg_iff.mpr (by simpa using h2.2)
            have hle_sub : (-terms[k.val].coeff) * x k ≤ mo - rhs := by
              have h1' : rhs - mo ≤ terms[k.val].coeff * x k := by
                have := (Rat.add_le_add_left (c := -mo)).mpr hle_sum
                rw [← Rat.add_assoc, Rat.neg_add_cancel, Rat.zero_add, Rat.add_comm (-mo), ← Rat.sub_eq_add_neg] at this
                exact this
              have h2' := Rat.neg_le_neg h1'
              simpa [Rat.neg_sub, Rat.neg_mul] using h2'
            have hraw : x k ≤ (mo - rhs) / (-terms[k.val].coeff) :=
              (rat_le_div_iff hneg_pos).mpr (by rwa [Rat.mul_comm])
            have hraw' : x k ≤ (rhs - mo) / terms[k.val].coeff := by
              have heq : (mo - rhs) / (-terms[k.val].coeff) = (rhs - mo) / terms[k.val].coeff := by
                rw [rat_div_neg_eq_neg_div_pos, Rat.neg_sub]
              rwa [← heq]
            exact roundUpper_sound hraw' (hint k)
        · have hb2 : (hasLo && decide (terms[k.val].coeff < 0)) = false := by cases hasLo <;> simp_all
          simp [hb2] at hub
  · intro k lb hlb
    rw [hTermEq k] at hlb
    unfold evalActivityTermSpec at hlb
    by_cases hc0 : terms[k.val].coeff = 0
    · have hbeq : (terms[k.val].coeff == 0) = true := beq_iff_eq.mpr hc0
      simp [hbeq] at hlb
    · have hbeq : (terms[k.val].coeff == 0) = false := beq_false_of_ne hc0
      simp only [hbeq, Bool.false_eq_true, ↓reduceIte] at hlb
      by_cases h1 : hasHi = true ∧ terms[k.val].coeff < 0
      · have hb1 : (hasHi && decide (terms[k.val].coeff < 0)) = true := by simp [h1.1, h1.2]
        simp only [hb1, ↓reduceIte] at hlb
        cases hmo : (match termMinOpt terms[k.val] with
          | some v => if countNoneFin terms.size minF == 0 then some (finSum terms.size (fun j => (minF j).getD 0) - v) else none
          | none => if countNoneFin terms.size minF == 1 then some (finSum terms.size (fun j => (minF j).getD 0)) else none) with
        | none => rw [hmo] at hlb; simp at hlb
        | some mo =>
          have hmo' := hMinOtherLe k mo hmo
          rw [hmo] at hlb
          simp only [Option.map_some, Option.some.injEq] at hlb
          subst hlb
          have hsum := hhi h1.1
          rw [finSum_split_at (fun j => terms[j.val].coeff * x j) k] at hsum
          have hak_le : mo + terms[k.val].coeff * x k ≤ rhs :=
            Rat.le_trans (Rat.add_le_add_right.mpr hmo') hsum
          have hneg_pos : 0 < -terms[k.val].coeff := Rat.lt_neg_iff.mpr (by simpa using h1.2)
          have hsub_le : mo - rhs ≤ (-terms[k.val].coeff) * x k := by
            have h1' : terms[k.val].coeff * x k ≤ rhs - mo := by
              have := (Rat.add_le_add_left (c := -mo)).mpr hak_le
              rw [← Rat.add_assoc, Rat.neg_add_cancel, Rat.zero_add, Rat.add_comm (-mo), ← Rat.sub_eq_add_neg] at this
              exact this
            have h2' := Rat.neg_le_neg h1'
            simpa [Rat.neg_sub, Rat.neg_mul] using h2'
          have hraw : (mo - rhs) / (-terms[k.val].coeff) ≤ x k :=
            (rat_div_le_iff hneg_pos).mpr (by rwa [Rat.mul_comm (x k) (-terms[k.val].coeff)])
          have hraw' : (rhs - mo) / terms[k.val].coeff ≤ x k := by
            have heq : (mo - rhs) / (-terms[k.val].coeff) = (rhs - mo) / terms[k.val].coeff := by
              rw [rat_div_neg_eq_neg_div_pos, Rat.neg_sub]
            rwa [← heq]
          exact roundLower_sound hraw' (hint k)
      · have hb1 : (hasHi && decide (terms[k.val].coeff < 0)) = false := by cases hasHi <;> simp_all
        simp only [hb1, Bool.false_eq_true, ↓reduceIte] at hlb
        by_cases h2 : hasLo = true ∧ 0 < terms[k.val].coeff
        · have hb2 : (hasLo && decide (0 < terms[k.val].coeff)) = true := by simp [h2.1, h2.2]
          simp only [hb2, ↓reduceIte] at hlb
          cases hmo : (match termMaxOpt terms[k.val] with
            | some v => if countNoneFin terms.size maxF == 0 then some (finSum terms.size (fun j => (maxF j).getD 0) - v) else none
            | none => if countNoneFin terms.size maxF == 1 then some (finSum terms.size (fun j => (maxF j).getD 0)) else none) with
          | none => rw [hmo] at hlb; simp at hlb
          | some mo =>
            have hmo' := hMaxOtherGe k mo hmo
            rw [hmo] at hlb
            simp only [Option.map_some, Option.some.injEq] at hlb
            subst hlb
            have hsum := hlo h2.1
            rw [finSum_split_at (fun j => terms[j.val].coeff * x j) k] at hsum
            have hle_sum : rhs ≤ mo + terms[k.val].coeff * x k :=
              Rat.le_trans hsum (Rat.add_le_add_right.mpr hmo')
            have hsub_le : rhs - mo ≤ terms[k.val].coeff * x k := by
              have := (Rat.add_le_add_left (c := -mo)).mpr hle_sum
              rw [← Rat.add_assoc, Rat.neg_add_cancel, Rat.zero_add, Rat.add_comm (-mo), ← Rat.sub_eq_add_neg] at this
              exact this
            have hraw : (rhs - mo) / terms[k.val].coeff ≤ x k :=
              (rat_div_le_iff h2.2).mpr (by rwa [Rat.mul_comm (x k) terms[k.val].coeff])
            exact roundLower_sound hraw (hint k)
        · have hb2 : (hasLo && decide (0 < terms[k.val].coeff)) = false := by cases hasLo <;> simp_all
          simp [hb2] at hlb

/-- Computes `maxOther` directly from `terms` excluding `targetIdx`, returning `none` if
any non-target term lacks a required finite bound. -/
def computeMaxOtherOpt (terms : Array ActivityTerm) (targetIdx : Nat) : Option Rat :=
  let otherOpt : Fin terms.size → Option Rat := fun j =>
    if j.val == targetIdx then some 0 else termMaxOpt terms[j.val]
  if countNoneFin terms.size otherOpt == 0 then
    some (finSum terms.size (fun j => (otherOpt j).getD 0))
  else
    none

/-- Checks whether `terms[targetIdx]` is a valid binary `[0, 1]` integer variable. -/
def isBinaryActivityTerm (t : ActivityTerm) : Bool :=
  t.isInt &&
  match t.lb, t.ub with
  | some l, some u => decide (0 ≤ l) && decide (u ≤ 1)
  | _, _ => false

/-- Executable Savelsbergh binary strengthening certificate checker computing `maxOther`
directly from `terms` and variable bounds (never trusting an unverified input `max_other`). -/
def checkBinStrengthenFromRowSpec
    (rhs : Rat) (terms : Array ActivityTerm) (targetIdx : Nat)
    (surplus newA newRHS : Rat) : Except String Rat :=
  if hIdx : targetIdx < terms.size then
    let tk := terms[targetIdx]
    if !isBinaryActivityTerm tk then
      .error s!"target_idx {targetIdx} is not a binary [0,1] integer variable"
    else
      match computeMaxOtherOpt terms targetIdx with
      | none => .error "missing required finite bound to compute max_other"
      | some maxO =>
        let a := tk.coeff
        if 0 < a then
          if 0 < surplus && surplus < a && maxO + a - rhs ≤ surplus &&
              newA == surplus && rhs - (a - surplus) ≤ newRHS then
            .ok maxO
          else
            .error s!"positive binary strengthening check failed (a={a}, max_other={maxO}, surplus={surplus})"
        else if a < 0 then
          if 0 < surplus && surplus < -a && maxO - rhs ≤ surplus &&
              newA == -surplus && rhs ≤ newRHS then
            .ok maxO
          else
            .error s!"negative binary strengthening check failed (a={a}, max_other={maxO}, surplus={surplus})"
        else
          .error "binary strengthening requires nonzero target coefficient"
  else
    .error s!"target_idx {targetIdx} out of bounds ({terms.size})"

/-- Universal soundness of `checkBinStrengthenFromRowSpec` over `RowFeasibleOpt`. -/
theorem checkBinStrengthenFromRowSpec_sound
    {rhs : Rat} {terms : Array ActivityTerm} {targetIdx : Nat}
    {surplus newA newRHS maxO : Rat}
    (hok : checkBinStrengthenFromRowSpec rhs terms targetIdx surplus newA newRHS = .ok maxO)
    (x : Fin terms.size → Rat)
    (hfeas : RowFeasibleOpt terms rhs false true x) :
    finSum terms.size (fun j =>
      (if j.val = targetIdx then newA else terms[j.val].coeff) * x j) ≤ newRHS := by
  unfold checkBinStrengthenFromRowSpec at hok
  split at hok
  · next hIdx =>
    let k : Fin terms.size := ⟨targetIdx, hIdx⟩
    have hbinB : isBinaryActivityTerm terms[targetIdx] = true := by
      cases hb : isBinaryActivityTerm terms[targetIdx]
      · simp [hb] at hok
      · rfl
    simp only [hbinB, Bool.not_true, Bool.false_eq_true, ↓reduceIte] at hok
    rcases hfeas with ⟨hb, hint, hhi, _⟩
    have hxBin : x k = 0 ∨ x k = 1 := by
      unfold isBinaryActivityTerm at hbinB
      simp only [Bool.and_eq_true] at hbinB
      rcases hint k hbinB.1 with ⟨z, hz⟩
      cases hlb : terms[targetIdx].lb with
      | none => simp [hlb] at hbinB
      | some l =>
        cases hub : terms[targetIdx].ub with
        | none => simp [hlb, hub] at hbinB
        | some u =>
          simp only [hlb, hub, Bool.and_eq_true, decide_eq_true_eq] at hbinB
          have hlx : l ≤ (z : Rat) := hz ▸ (hb k).1 l hlb
          have hxu : (z : Rat) ≤ u := hz ▸ (hb k).2 u hub
          have h0 : (0 : Int) ≤ z := Rat.intCast_le_intCast.mp (Rat.le_trans hbinB.2.1 hlx)
          have h1 : z ≤ (1 : Int) := Rat.intCast_le_intCast.mp (Rat.le_trans hxu hbinB.2.2)
          have hcases : z = 0 ∨ z = 1 := by omega
          rcases hcases with rfl | rfl
          · left; exact hz
          · right; exact hz
    cases hmo : computeMaxOtherOpt terms targetIdx with
    | none => simp [hmo] at hok
    | some mo =>
      simp only [hmo] at hok
      have hother : finSum terms.size (fun j => if j = k then 0 else terms[j.val].coeff * x j) ≤ mo := by
        unfold computeMaxOtherOpt at hmo
        dsimp only at hmo
        split at hmo
        · next hbeq =>
          have h0 : countNoneFin terms.size (fun j => if j.val == targetIdx then some 0 else termMaxOpt terms[j.val]) = 0 :=
            beq_iff_eq.mp hbeq
          simp only [Option.some.injEq] at hmo
          subst hmo
          have hall := countNoneFin_zero_iff.mp h0
          apply finSum_le_finSum
          intro j
          by_cases hjk : j = k
          · subst hjk; simp [k]
          · have hne : (j.val == targetIdx) = false := beq_false_of_ne (fun heq => hjk (Fin.ext heq))
            simp only [hjk, hne, Bool.false_eq_true, ↓reduceIte]
            have hjs := hall j
            simp only [hne, Bool.false_eq_true, ↓reduceIte] at hjs
            cases htm : termMaxOpt terms[j.val] with
            | none => simp [htm] at hjs
            | some vj =>
              simp only [Option.getD_some]
              exact mul_le_termMaxOpt (hb j).1 (hb j).2 htm
        · contradiction
      have horig := hhi rfl
      have hiteEq : ∀ j : Fin terms.size,
          (if j.val = targetIdx then newA else terms[j.val].coeff) * x j =
          if j = k then newA * x j else terms[j.val].coeff * x j := by
        intro j
        by_cases hjk : j = k
        · subst hjk; simp [k]
        · have hne : j.val ≠ targetIdx := fun heq => hjk (Fin.ext heq)
          simp [hjk, hne]
      rw [finSum_congr hiteEq]
      by_cases hpos : 0 < terms[targetIdx].coeff
      · simp only [hpos, ↓reduceIte] at hok
        split at hok
        · next hcond =>
          simp only [Bool.and_eq_true, decide_eq_true_eq, beq_iff_eq] at hcond
          rcases hcond with ⟨⟨⟨⟨_, hlt⟩, hlo⟩, rfl⟩, hrhs⟩
          exact binary_strengthen_pos_outward_sound hxBin hother hlo hlt hrhs horig
        · contradiction
      · simp only [hpos, ↓reduceIte] at hok
        by_cases hneg : terms[targetIdx].coeff < 0
        · simp only [hneg, ↓reduceIte] at hok
          split at hok
          · next hcond =>
            simp only [Bool.and_eq_true, decide_eq_true_eq, beq_iff_eq] at hcond
            rcases hcond with ⟨⟨⟨⟨_, hlt⟩, hlo⟩, rfl⟩, hrhs⟩
            exact Rat.le_trans ((binary_strengthen_neg_iff hxBin hother hlo hlt).mp horig) hrhs
          · contradiction
        · simp [hneg] at hok
  · contradiction

/-! ### 6. Full `S_G` Soundness of `verifyOrbitSymmetrySpec` and Multi-Element Chain/Orbitope Theorems -/

/-- Extended rational lower-bound satisfaction `b ≤ x`. -/
def ExtRat.leVal (b : ExtRat) (x : Rat) : Prop :=
  match b with
  | .negInf => True
  | .fin r => r ≤ x
  | .posInf => False

/-- Extended rational upper-bound satisfaction `x ≤ b`. -/
def ExtRat.geVal (b : ExtRat) (x : Rat) : Prop :=
  match b with
  | .negInf => False
  | .fin r => x ≤ r
  | .posInf => True

/-- Sum of coefficients for column `c` in a pair list `(col, val)`. -/
def pairCoeffList : List (Nat × Rat) → Nat → Rat
  | [], _ => 0
  | (col, val) :: rest, c => (if col == c then val else 0) + pairCoeffList rest c

/-- Evaluates a pair list `(col, val)` against `x : Fin n → Rat`. -/
def evalPairList {n : Nat} : List (Nat × Rat) → (Fin n → Rat) → Rat
  | [], _ => 0
  | (col, val) :: rest, x =>
    (if h : col < n then val * x ⟨col, h⟩ else 0) + evalPairList rest x

/-- Satisfaction of an `OrbitWorkRow` over `x : Fin n → Rat` (inactive rows hold trivially). -/
def OrbitRowHolds (n : Nat) (r : OrbitWorkRow) (x : Fin n → Rat) : Prop :=
  r.active = true →
  let s := evalPairList (rowPairsList r) x
  match r.sense with
  | .le => s ≤ r.rhs
  | .ge => r.rhs ≤ s
  | .eq => s = r.rhs

/-- Full MIP feasibility predicate over `ExtRat` bounds and `OrbitWorkRow` rows. -/
def ExtMipFeasible (numVars : Nat) (isInt : Array Bool) (lb ub : Array ExtRat)
    (rows : Array OrbitWorkRow) (x : Fin numVars → Rat) : Prop :=
  (∀ j : Fin numVars, (lb[j.val]!).leVal (x j) ∧ (ub[j.val]!).geVal (x j)) ∧
  (∀ j : Fin numVars, isInt[j.val]! = true → ∃ z : Int, x j = (z : Rat)) ∧
  (∀ i : Fin rows.size, OrbitRowHolds numVars rows[i.val] x)

private theorem evalPairList_eq_finSum {n : Nat} (ps : List (Nat × Rat)) (x : Fin n → Rat)
    (hcols : ∀ p ∈ ps, p.1 < n) :
    evalPairList ps x = finSum n (fun j => pairCoeffList ps j.val * x j) := by
  induction ps with
  | nil => simp [evalPairList, pairCoeffList]
  | cons p rest ih =>
    rcases p with ⟨col, val⟩
    have hlt : col < n := hcols (col, val) (List.mem_cons_self ..)
    have hrest : ∀ q ∈ rest, q.1 < n := fun q hq => hcols q (List.mem_cons_of_mem _ hq)
    simp only [evalPairList, pairCoeffList, hlt, ↓reduceDIte]
    rw [ih hrest]
    have hsingle :
        val * x ⟨col, hlt⟩ =
          finSum n (fun j : Fin n => (if col == j.val then val else 0) * x j) := by
      have hcong :
          finSum n (fun j : Fin n => (if col == j.val then val else 0) * x j) =
            finSum n (fun j : Fin n => if j = ⟨col, hlt⟩ then val * x ⟨col, hlt⟩ else 0) := by
        apply finSum_congr
        intro j
        by_cases hj : j = ⟨col, hlt⟩
        · subst hj; simp
        · have hne : (col == j.val) = false := beq_false_of_ne (fun heq => hj (Fin.ext heq.symm))
          simp [hj, hne]
      rw [hcong, finSum_single]
    rw [hsingle, ← finSum_add]
    exact finSum_congr fun j => (Rat.add_mul _ _ _).symm

/-- Strictly ascending column indices in a sparse row pair list. -/
private def StrictSortedPairs : List (Nat × Rat) → Prop
  | [] => True
  | p :: rest => (∀ q ∈ rest, p.1 < q.1) ∧ StrictSortedPairs rest

private theorem strictSorted_pairCoeff_mem {xs : List (Nat × Rat)} (hs : StrictSortedPairs xs)
    {c : Nat} {v : Rat} (hmem : (c, v) ∈ xs) : pairCoeffList xs c = v := by
  induction xs with
  | nil => simp at hmem
  | cons cur rest ih =>
    rcases hs with ⟨hgt, hrest⟩
    rcases List.mem_cons.mp hmem with rfl | hmem'
    · have hzero : pairCoeffList rest c = 0 := by
        clear ih hrest hmem
        induction rest with
        | nil => rfl
        | cons r rs ihr =>
          have hlt : c < r.1 := hgt r (List.mem_cons_self ..)
          have hne : (r.1 == c) = false := beq_false_of_ne (by omega)
          simp only [pairCoeffList, hne, Bool.false_eq_true, ↓reduceIte, Rat.zero_add]
          exact ihr (fun q hq => hgt q (List.mem_cons_of_mem _ hq))
      simp [pairCoeffList, hzero, Rat.add_zero]
    · have hlt : cur.1 < c := hgt (c, v) hmem'
      have hne : (cur.1 == c) = false := beq_false_of_ne (by omega)
      simp only [pairCoeffList, hne, Bool.false_eq_true, ↓reduceIte, Rat.zero_add]
      exact ih hrest hmem'

private theorem pairCoeff_not_mem {xs : List (Nat × Rat)} {c : Nat}
    (hnot : ∀ q ∈ xs, q.1 ≠ c) : pairCoeffList xs c = 0 := by
  induction xs with
  | nil => rfl
  | cons cur rest ih =>
    have hne : (cur.1 == c) = false := beq_false_of_ne (hnot cur (List.mem_cons_self ..))
    simp only [pairCoeffList, hne, Bool.false_eq_true, ↓reduceIte, Rat.zero_add]
    exact ih (fun q hq => hnot q (List.mem_cons_of_mem _ hq))

private theorem insertSortedPairList_props {xs : List (Nat × Rat)} (hs : StrictSortedPairs xs)
    {p : Nat × Rat} {ys : List (Nat × Rat)}
    (h : insertSortedPairList xs p = some ys) :
    StrictSortedPairs ys ∧
    (∀ c, pairCoeffList ys c = (if p.1 == c then p.2 else 0) + pairCoeffList xs c) ∧
    (∀ q, q ∈ ys ↔ q = p ∨ q ∈ xs) := by
  induction xs generalizing ys with
  | nil =>
    simp only [insertSortedPairList, Option.some.injEq] at h
    subst h
    refine ⟨⟨fun _ hq => by simp at hq, trivial⟩, fun c => by simp [pairCoeffList], fun q => by simp⟩
  | cons cur rest ih =>
    rcases hs with ⟨hgt, hrest⟩
    unfold insertSortedPairList at h
    by_cases heq : p.1 = cur.1
    · have hb : (p.1 == cur.1) = true := beq_iff_eq.mpr heq
      simp [hb] at h
    · have hb : (p.1 == cur.1) = false := beq_false_of_ne heq
      simp only [hb, Bool.false_eq_true, ↓reduceIte] at h
      by_cases hlt : p.1 < cur.1
      · simp only [hlt, ↓reduceIte, Option.some.injEq] at h
        subst h
        refine ⟨⟨?_, ⟨hgt, hrest⟩⟩, fun c => by simp [pairCoeffList], fun q => by simp⟩
        intro q hq
        rcases List.mem_cons.mp hq with rfl | hmem
        · exact hlt
        · exact Nat.lt_trans hlt (hgt q hmem)
      · simp only [hlt, ↓reduceIte] at h
        cases hrec : insertSortedPairList rest p with
        | none => simp [hrec] at h
        | some zs =>
          simp only [hrec, Option.map_some, Option.some.injEq] at h
          subst h
          rcases ih hrest hrec with ⟨hzsS, hzsC, hzsM⟩
          refine ⟨⟨?_, hzsS⟩, ?_, ?_⟩
          · intro q hq
            rcases (hzsM q).mp hq with rfl | hmem
            · omega
            · exact hgt q hmem
          · intro c
            simp only [pairCoeffList, hzsC c]
            rw [Rat.add_left_comm]
          · intro q
            simp only [List.mem_cons, hzsM q]
            constructor
            · rintro (rfl | rfl | hmem)
              · exact Or.inr (Or.inl rfl)
              · exact Or.inl rfl
              · exact Or.inr (Or.inr hmem)
            · rintro (rfl | rfl | hmem)
              · exact Or.inr (Or.inl rfl)
              · exact Or.inl rfl
              · exact Or.inr (Or.inr hmem)

private theorem sortUniqueRowPairsList_props {pairs sorted : List (Nat × Rat)}
    (h : sortUniqueRowPairsList pairs = some sorted) :
    StrictSortedPairs sorted ∧
    (∀ c, pairCoeffList sorted c = pairCoeffList pairs c) ∧
    (∀ q, q ∈ sorted ↔ q ∈ pairs) := by
  unfold sortUniqueRowPairsList at h
  suffices hgen : ∀ (ps acc res : List (Nat × Rat)),
      StrictSortedPairs acc →
      ps.foldlM insertSortedPairList acc = some res →
      StrictSortedPairs res ∧
      (∀ c, pairCoeffList res c = pairCoeffList ps c + pairCoeffList acc c) ∧
      (∀ q, q ∈ res ↔ q ∈ ps ∨ q ∈ acc) by
    rcases hgen pairs [] sorted trivial h with ⟨hs, hc, hm⟩
    refine ⟨hs, fun c => by simpa [pairCoeffList, Rat.add_zero] using hc c,
      fun q => by simpa using hm q⟩
  intro ps
  induction ps with
  | nil =>
    intro acc res hacc hfold
    simp only [List.foldlM_nil, pure, Option.some.injEq] at hfold
    subst hfold
    exact ⟨hacc, fun c => by simp [pairCoeffList, Rat.zero_add], fun q => by simp⟩
  | cons p rest ih =>
    intro acc res hacc hfold
    simp only [List.foldlM_cons, bind, Option.bind] at hfold
    cases hins : insertSortedPairList acc p with
    | none => simp [hins] at hfold
    | some acc' =>
      simp only [hins] at hfold
      rcases insertSortedPairList_props hacc hins with ⟨hacc'S, hacc'C, hacc'M⟩
      rcases ih acc' res hacc'S hfold with ⟨hresS, hresC, hresM⟩
      refine ⟨hresS, ?_, ?_⟩
      · intro c
        rw [hresC c, hacc'C c]
        simp only [pairCoeffList]
        rw [Rat.add_assoc, Rat.add_left_comm (pairCoeffList rest c)]
      · intro q
        rw [hresM q, hacc'M q]
        simp only [List.mem_cons]
        constructor
        · rintro (h1 | rfl | h2)
          · exact Or.inl (Or.inr h1)
          · exact Or.inl (Or.inl rfl)
          · exact Or.inr h2
        · rintro ((rfl | h1) | h2)
          · exact Or.inr (Or.inl rfl)
          · exact Or.inl h1
          · exact Or.inr (Or.inr h2)

private theorem buildRowTupleSlotsAux_size (numRows gCount : Nat) (rowTuples : Array Nat)
    (k : Nat) {acc slots : Array (Option (Nat × Nat))}
    (h : buildRowTupleSlotsAux numRows gCount rowTuples k acc = some slots) :
    slots.size = acc.size := by
  induction k generalizing slots with
  | zero =>
    simp only [buildRowTupleSlotsAux, Option.some.injEq] at h
    subst h; rfl
  | succ k ih =>
    simp only [buildRowTupleSlotsAux] at h
    cases hprev : buildRowTupleSlotsAux numRows gCount rowTuples k acc with
    | none => simp [hprev] at h
    | some prev =>
      simp only [hprev] at h
      split at h
      · split at h
        · cases hget : prev[rowTuples[k]] with
          | some _ => simp [hget] at h
          | none =>
            simp only [hget, Option.some.injEq] at h
            subst h
            simp [ih hprev]
        · contradiction
      · contradiction

private theorem buildRowTupleSlotsAux_props (numRows gCount : Nat) (rowTuples : Array Nat)
    (k : Nat) (hk : k ≤ rowTuples.size)
    {slots : Array (Option (Nat × Nat))}
    (h : buildRowTupleSlotsAux numRows gCount rowTuples k (Array.replicate numRows none) = some slots) :
    slots.size = numRows ∧
    (∀ idx (hidx : idx < k),
      let rIdx := rowTuples[idx]'(Nat.lt_of_lt_of_le hidx hk)
      ∃ hr : rIdx < numRows,
        slots[rIdx]'(by rw [buildRowTupleSlotsAux_size _ _ _ _ h]; simpa using hr) =
          some (idx / gCount, idx % gCount)) ∧
    (∀ r (hr : r < numRows) tIdx g,
      slots[r]'(by rw [buildRowTupleSlotsAux_size _ _ _ _ h]; simpa using hr) = some (tIdx, g) →
      ∃ idx, ∃ hidx : idx < k,
        rowTuples[idx]'(Nat.lt_of_lt_of_le hidx hk) = r ∧
        tIdx = idx / gCount ∧ g = idx % gCount) := by
  induction k generalizing slots with
  | zero =>
    simp only [buildRowTupleSlotsAux, Option.some.injEq] at h
    subst h
    refine ⟨by simp, fun idx hidx => by omega, fun r hr tIdx g hget => ?_⟩
    simp at hget
  | succ k ih =>
    simp only [buildRowTupleSlotsAux] at h
    cases hprev : buildRowTupleSlotsAux numRows gCount rowTuples k (Array.replicate numRows none) with
    | none => simp [hprev] at h
    | some prev =>
      simp only [hprev] at h
      rcases ih (by omega) hprev with ⟨hprevSz, hprevMap, hprevInv⟩
      have hklt : k < rowTuples.size := by omega
      simp only [hklt, ↓reduceDIte] at h
      let rK := rowTuples[k]
      by_cases hrK : rK < prev.size
      · simp only [rK, hrK, ↓reduceDIte] at h
        cases hslotK : prev[rK] with
        | some _ => simp [rK, hslotK] at h
        | none =>
          simp only [rK, hslotK, Option.some.injEq] at h
          subst h
          have hsz : (prev.set rK (some (k / gCount, k % gCount)) hrK).size = numRows := by
            simp [hprevSz]
          refine ⟨hsz, ?_, ?_⟩
          · intro idx hidx
            by_cases hidxk : idx = k
            · subst hidxk
              refine ⟨by omega, ?_⟩
              simp
            · have hidxlt : idx < k := by omega
              rcases hprevMap idx hidxlt with ⟨hrI, hgetI⟩
              have hne : rowTuples[idx]'(Nat.lt_of_lt_of_le hidx hk) ≠ rK := by
                intro heq
                have hsome : prev[rK] = some (idx / gCount, idx % gCount) := by
                  simpa [heq] using hgetI
                rw [hslotK] at hsome
                contradiction
              dsimp only [rK] at hne
              refine ⟨hrI, ?_⟩
              simp only [Array.getElem_set, Ne.symm hne, ↓reduceIte]
              exact hgetI
          · intro r hr tIdx g hget
            by_cases hrrK : r = rK
            · subst hrrK
              dsimp only [rK] at hget
              simp only [Array.getElem_set_self, Option.some.injEq, Prod.mk.injEq] at hget
              exact ⟨k, Nat.lt_succ_self k, rfl, hget.1.symm, hget.2.symm⟩
            · dsimp only [rK] at hrrK
              simp only [Array.getElem_set, Ne.symm hrrK, ↓reduceIte] at hget
              rcases hprevInv r hr tIdx g hget with ⟨idx, hidx, hrEq, htEq, hgEq⟩
              exact ⟨idx, Nat.lt_trans hidx (Nat.lt_succ_self k), hrEq, htEq, hgEq⟩
      · dsimp only [rK] at hrK
        simp only [hrK, ↓reduceDIte] at h
        contradiction

private theorem list_all_range_iff {n : Nat} {p : Nat → Bool} :
    (List.range n).all p = true ↔ ∀ i, i < n → p i = true := by
  simp only [List.all_eq_true, List.mem_range]

private theorem pairCoeffList_map_iff {ps : List (Nat × Rat)} {f : Nat → Nat} {c : Nat}
    (hf : ∀ p ∈ ps, (f p.1 == c) = (p.1 == c)) :
    pairCoeffList (ps.map fun p => (f p.1, p.2)) c = pairCoeffList ps c := by
  induction ps with
  | nil => rfl
  | cons p rest ih =>
    simp only [List.map_cons, pairCoeffList]
    rw [hf p (List.mem_cons_self ..), ih (fun q hq => hf q (List.mem_cons_of_mem _ hq))]

private theorem pairCoeffList_map_eq_map {ps : List (Nat × Rat)} {f1 f2 : Nat → Nat} {c1 c2 : Nat}
    (hf : ∀ p ∈ ps, (f1 p.1 == c1) = (f2 p.1 == c2)) :
    pairCoeffList (ps.map fun p => (f1 p.1, p.2)) c1 =
      pairCoeffList (ps.map fun p => (f2 p.1, p.2)) c2 := by
  induction ps with
  | nil => rfl
  | cons p rest ih =>
    simp only [List.map_cons, pairCoeffList]
    rw [hf p (List.mem_cons_self ..), ih (fun q hq => hf q (List.mem_cons_of_mem _ hq))]

/-- Column action of a group permutation `π : Fin gCount → Fin gCount` on `Fin numVars`. -/
def orbitColAction (numVars gCount : Nat) (orbits : Array Nat) (colOrbit colGroup : Array Int)
    (π : Fin gCount → Fin gCount) (j : Fin numVars) : Fin numVars :=
  let orb := colOrbit[j.val]!
  if _h : 0 ≤ orb then
    let grp := colGroup[j.val]!
    if hgrp : 0 ≤ grp ∧ grp.toNat < gCount then
      let c := orbits[orb.toNat * gCount + (π ⟨grp.toNat, hgrp.2⟩).val]!
      if hc : c < numVars then ⟨c, hc⟩ else j
    else j
  else j

set_option maxHeartbeats 600000 in
/-- Full `S_G` soundness theorem for `verifyOrbitSymmetrySpec`:
whenever `verifyOrbitSymmetrySpec` returns `true`, EVERY group permutation `π ∈ S_G`
(bijection `Fin gCount → Fin gCount`) induces a bijective column permutation `orbitColAction`
that preserves `ExtMipFeasible` across all active rows and variable domains, and preserves
`mipObj` whenever `exactOrbitObjSpec` holds. -/
theorem verifyOrbitSymmetrySpec_sound
    {numVars gCount : Nat}
    {isInt : Array Bool} {lb ub : Array ExtRat}
    {rows : Array OrbitWorkRow}
    {orbits rowTuples : Array Nat}
    {colOrbit colGroup rowState : Array Int}
    (hvalid : verifyOrbitSymmetrySpec numVars isInt lb ub rows gCount orbits rowTuples colOrbit colGroup rowState = true)
    (π πInv : Fin gCount → Fin gCount)
    (hπL : ∀ g, πInv (π g) = g)
    (hπR : ∀ g, π (πInv g) = g) :
    let act := orbitColAction numVars gCount orbits colOrbit colGroup π
    let actInv := orbitColAction numVars gCount orbits colOrbit colGroup πInv
    (∀ j : Fin numVars, actInv (act j) = j) ∧
    (∀ j : Fin numVars, act (actInv j) = j) ∧
    (∀ x : Fin numVars → Rat,
      ExtMipFeasible numVars isInt lb ub rows x →
      ExtMipFeasible numVars isInt lb ub rows (fun j => x (act j))) ∧
    (∀ (obj : Array Rat), obj.size = numVars →
      exactOrbitObjSpec gCount orbits obj = true →
      ∀ x : Fin numVars → Rat,
        mipObj (fun j => obj[j.val]!) (fun j => x (act j)) = mipObj (fun j => obj[j.val]!) x) := by
  unfold verifyOrbitSymmetrySpec at hvalid
  have hDim1 : (gCount < 2 || numVars == 0 || orbits.size == 0 ||
      orbits.size % gCount != 0 || rowTuples.size % gCount != 0) = false := by
    cases hc : (gCount < 2 || numVars == 0 || orbits.size == 0 ||
      orbits.size % gCount != 0 || rowTuples.size % gCount != 0)
    · rfl
    · simp [hc] at hvalid
  simp only [hDim1, Bool.false_eq_true, ↓reduceIte] at hvalid
  have hDim2 : (isInt.size != numVars || lb.size != numVars || ub.size != numVars ||
      colOrbit.size != numVars || colGroup.size != numVars || rowState.size != rows.size) = false := by
    cases hc : (isInt.size != numVars || lb.size != numVars || ub.size != numVars ||
      colOrbit.size != numVars || colGroup.size != numVars || rowState.size != rows.size)
    · rfl
    · simp [hc] at hvalid
  simp only [hDim2, Bool.false_eq_true, ↓reduceIte] at hvalid
  have hgGe2 : 2 ≤ gCount := by
    apply Classical.byContradiction; intro hlt
    have : (gCount < 2 || numVars == 0 || orbits.size == 0 ||
      orbits.size % gCount != 0 || rowTuples.size % gCount != 0) = true := by simp [show gCount < 2 by omega]
    rw [hDim1] at this; contradiction
  have hNumVarsPos : 0 < numVars := by
    apply Classical.byContradiction; intro h0
    have : (gCount < 2 || numVars == 0 || orbits.size == 0 ||
      orbits.size % gCount != 0 || rowTuples.size % gCount != 0) = true := by simp [show numVars = 0 by omega]
    rw [hDim1] at this; contradiction
  have hOrbNonEmpty : 0 < orbits.size := by
    apply Classical.byContradiction; intro h0
    have : (gCount < 2 || numVars == 0 || orbits.size == 0 ||
      orbits.size % gCount != 0 || rowTuples.size % gCount != 0) = true := by simp [show orbits.size = 0 by omega]
    rw [hDim1] at this; contradiction
  have hOrbMod : orbits.size % gCount = 0 := by
    apply Classical.byContradiction; intro hne
    have : (gCount < 2 || numVars == 0 || orbits.size == 0 ||
      orbits.size % gCount != 0 || rowTuples.size % gCount != 0) = true := by simp [hne]
    rw [hDim1] at this; contradiction
  have hTupMod : rowTuples.size % gCount = 0 := by
    apply Classical.byContradiction; intro hne
    have : (gCount < 2 || numVars == 0 || orbits.size == 0 ||
      orbits.size % gCount != 0 || rowTuples.size % gCount != 0) = true := by simp [hne]
    rw [hDim1] at this; contradiction
  have hIsIntSz : isInt.size = numVars := by
    apply Classical.byContradiction; intro hne
    have : (isInt.size != numVars || lb.size != numVars || ub.size != numVars ||
      colOrbit.size != numVars || colGroup.size != numVars || rowState.size != rows.size) = true := by simp [hne]
    rw [hDim2] at this; contradiction
  have hLbSz : lb.size = numVars := by
    apply Classical.byContradiction; intro hne
    have : (isInt.size != numVars || lb.size != numVars || ub.size != numVars ||
      colOrbit.size != numVars || colGroup.size != numVars || rowState.size != rows.size) = true := by simp [hne]
    rw [hDim2] at this; contradiction
  have hUbSz : ub.size = numVars := by
    apply Classical.byContradiction; intro hne
    have : (isInt.size != numVars || lb.size != numVars || ub.size != numVars ||
      colOrbit.size != numVars || colGroup.size != numVars || rowState.size != rows.size) = true := by simp [hne]
    rw [hDim2] at this; contradiction
  have hColOrbSz : colOrbit.size = numVars := by
    apply Classical.byContradiction; intro hne
    have : (isInt.size != numVars || lb.size != numVars || ub.size != numVars ||
      colOrbit.size != numVars || colGroup.size != numVars || rowState.size != rows.size) = true := by simp [hne]
    rw [hDim2] at this; contradiction
  have hColGrpSz : colGroup.size = numVars := by
    apply Classical.byContradiction; intro hne
    have : (isInt.size != numVars || lb.size != numVars || ub.size != numVars ||
      colOrbit.size != numVars || colGroup.size != numVars || rowState.size != rows.size) = true := by simp [hne]
    rw [hDim2] at this; contradiction
  have hRowStSz : rowState.size = rows.size := by
    apply Classical.byContradiction; intro hne
    have : (isInt.size != numVars || lb.size != numVars || ub.size != numVars ||
      colOrbit.size != numVars || colGroup.size != numVars || rowState.size != rows.size) = true := by simp [hne]
    rw [hDim2] at this; contradiction
  let numOrbits := orbits.size / gCount
  let numTuples := rowTuples.size / gCount
  have hOrbSizeEq : numOrbits * gCount = orbits.size := Nat.div_mul_cancel (Nat.dvd_of_mod_eq_zero hOrbMod)
  have hTupSizeEq : numTuples * gCount = rowTuples.size := Nat.div_mul_cancel (Nat.dvd_of_mod_eq_zero hTupMod)
  -- Extract `orbsOk` and `colMapOk`
  let orbsOkB : Bool := (List.range numOrbits).all fun orbID =>
    let base := orbID * gCount
    let c0 := orbits[base]!
    if c0 ≥ numVars then false
    else
      (List.range gCount).all fun g =>
        let cg := orbits[base + g]!
        if _hcg : cg < numVars then
          colOrbit[cg]! == (orbID : Int) &&
          colGroup[cg]! == (g : Int) &&
          (g == 0 ||
            (isInt[cg]! == isInt[c0]! && lb[cg]! == lb[c0]! && ub[cg]! == ub[c0]!))
        else false
  let colMapOkB : Bool := (List.range numVars).all fun c =>
    let orb := colOrbit[c]!
    let grp := colGroup[c]!
    if orb < 0 then grp < 0
    else
      orb < (numOrbits : Int) && 0 ≤ grp && grp < (gCount : Int) &&
      orbits[orb.toNat * gCount + grp.toNat]! == c
  have hOrbsColOk : orbsOkB = true ∧ colMapOkB = true := by
    split at hvalid
    · contradiction
    · next hnot =>
      change ¬((!orbsOkB || !colMapOkB) = true) at hnot
      cases h1 : orbsOkB
      · exfalso; apply hnot; simp [h1]
      · cases h2 : colMapOkB
        · exfalso; apply hnot; simp [h2]
        · exact ⟨rfl, rfl⟩
  have hNotOrbsCol : (!orbsOkB || !colMapOkB) = false := by
    simp only [hOrbsColOk.1, hOrbsColOk.2]
    decide
  change (if (!orbsOkB || !colMapOkB) = true then false else _) = true at hvalid
  rw [hNotOrbsCol] at hvalid
  simp only [Bool.false_eq_true, ↓reduceIte] at hvalid
  -- Unpack orbit properties
  have hOrbEntry : ∀ orbID g, orbID < numOrbits → g < gCount →
      let c0 := orbits[orbID * gCount]!
      let cg := orbits[orbID * gCount + g]!
      cg < numVars ∧ colOrbit[cg]! = (orbID : Int) ∧ colGroup[cg]! = (g : Int) ∧
      isInt[cg]! = isInt[c0]! ∧ lb[cg]! = lb[c0]! ∧ ub[cg]! = ub[c0]! := by
    intro orbID g horb hg
    have h1 := list_all_range_iff.mp hOrbsColOk.1 orbID horb
    dsimp only at h1
    by_cases hc0 : orbits[orbID * gCount]! ≥ numVars
    · simp [hc0] at h1
    · simp only [hc0, ↓reduceIte] at h1
      have h2 := list_all_range_iff.mp h1 g hg
      split at h2
      · next hcg =>
        simp only [Bool.and_eq_true, Bool.or_eq_true, beq_iff_eq] at h2
        refine ⟨hcg, h2.1.1, h2.1.2, ?_⟩
        rcases h2.2 with rfl | ⟨⟨hi, hl⟩, hu⟩
        · simp
        · exact ⟨hi, hl, hu⟩
      · contradiction
  have hColEntry : ∀ c, c < numVars →
      (colOrbit[c]! < 0 ∧ colGroup[c]! < 0) ∨
      (0 ≤ colOrbit[c]! ∧ colOrbit[c]! < (numOrbits : Int) ∧
       0 ≤ colGroup[c]! ∧ colGroup[c]! < (gCount : Int) ∧
       orbits[colOrbit[c]!.toNat * gCount + colGroup[c]!.toNat]! = c) := by
    intro c hc
    have h1 := list_all_range_iff.mp hOrbsColOk.2 c hc
    dsimp only at h1
    by_cases hneg : colOrbit[c]! < 0
    · left
      simp only [hneg, ↓reduceIte, decide_eq_true_eq] at h1
      exact ⟨hneg, h1⟩
    · right
      simp only [hneg, ↓reduceIte, Bool.and_eq_true, decide_eq_true_eq, beq_iff_eq] at h1
      exact ⟨Int.not_lt.mp hneg, h1.1.1.1, h1.1.1.2, h1.1.2, h1.2⟩
  -- Action characterization lemma for any `σ : Fin gCount → Fin gCount`
  have hActChar : ∀ (σ : Fin gCount → Fin gCount) (j : Fin numVars),
      (colOrbit[j.val]! < 0 → orbitColAction numVars gCount orbits colOrbit colGroup σ j = j) ∧
      (∀ (orbID g : Nat) (horb : orbID < numOrbits) (hg : g < gCount),
        colOrbit[j.val]! = (orbID : Int) → colGroup[j.val]! = (g : Int) →
        (orbitColAction numVars gCount orbits colOrbit colGroup σ j).val =
          orbits[orbID * gCount + (σ ⟨g, hg⟩).val]! ∧
        colOrbit[(orbitColAction numVars gCount orbits colOrbit colGroup σ j).val]! = (orbID : Int) ∧
        colGroup[(orbitColAction numVars gCount orbits colOrbit colGroup σ j).val]! = ((σ ⟨g, hg⟩).val : Int) ∧
        isInt[(orbitColAction numVars gCount orbits colOrbit colGroup σ j).val]! = isInt[j.val]! ∧
        lb[(orbitColAction numVars gCount orbits colOrbit colGroup σ j).val]! = lb[j.val]! ∧
        ub[(orbitColAction numVars gCount orbits colOrbit colGroup σ j).val]! = ub[j.val]!) := by
    intro σ j
    constructor
    · intro hneg
      unfold orbitColAction
      have hnot : ¬(0 ≤ colOrbit[j.val]!) := Int.not_le.mpr hneg
      simp [hnot]
    · intro orbID g horb hg hco hcg
      unfold orbitColAction
      have hnn : 0 ≤ colOrbit[j.val]! := by omega
      have hgnn : 0 ≤ colGroup[j.val]! ∧ colGroup[j.val]!.toNat < gCount := by
        rw [hcg]; exact ⟨Int.natCast_nonneg g, by simpa using hg⟩
      have htoO : colOrbit[j.val]!.toNat = orbID := by rw [hco]; rfl
      have htoG : colGroup[j.val]!.toNat = g := by rw [hcg]; rfl
      have hfinG : (⟨colGroup[j.val]!.toNat, hgnn.2⟩ : Fin gCount) = ⟨g, hg⟩ := Fin.ext htoG
      have hOE := hOrbEntry orbID (σ ⟨g, hg⟩).val horb (σ ⟨g, hg⟩).isLt
      have hOE0 := hOrbEntry orbID g horb hg
      have hjEq : orbits[orbID * gCount + g]! = j.val := by
        rcases hColEntry j.val j.isLt with ⟨hn, _⟩ | ⟨_, _, _, _, heq⟩
        · omega
        · rwa [htoO, htoG] at heq
      simp only [hnn, hgnn, ↓reduceDIte, htoO, hfinG, hOE.1]
      refine ⟨rfl, hOE.2.1, hOE.2.2.1, ?_, ?_, ?_⟩
      · exact (hOE.2.2.2.1.trans hOE0.2.2.2.1.symm).trans (congrArg (fun k => isInt[k]!) hjEq)
      · exact (hOE.2.2.2.2.1.trans hOE0.2.2.2.2.1.symm).trans (congrArg (fun k => lb[k]!) hjEq)
      · exact (hOE.2.2.2.2.2.trans hOE0.2.2.2.2.2.symm).trans (congrArg (fun k => ub[k]!) hjEq)
  -- Left and right inverse of `orbitColAction`
  have hActInv : ∀ (σ τ : Fin gCount → Fin gCount) (_h : ∀ g, τ (σ g) = g) (j : Fin numVars),
      orbitColAction numVars gCount orbits colOrbit colGroup τ
        (orbitColAction numVars gCount orbits colOrbit colGroup σ j) = j := by
    intro σ τ hστ j
    rcases hColEntry j.val j.isLt with ⟨hneg, _⟩ | ⟨honn, holt, hgnn, hglt, horbeq⟩
    · rw [(hActChar σ j).1 hneg, (hActChar τ j).1 hneg]
    · let orbID := colOrbit[j.val]!.toNat
      let g := colGroup[j.val]!.toNat
      have horb : orbID < numOrbits := by omega
      have hg : g < gCount := by omega
      have hco : colOrbit[j.val]! = (orbID : Int) := (Int.toNat_of_nonneg honn).symm
      have hcg : colGroup[j.val]! = (g : Int) := (Int.toNat_of_nonneg hgnn).symm
      have h1 := (hActChar σ j).2 orbID g horb hg hco hcg
      let j1 := orbitColAction numVars gCount orbits colOrbit colGroup σ j
      have h2 := (hActChar τ j1).2 orbID (σ ⟨g, hg⟩).val horb (σ ⟨g, hg⟩).isLt h1.2.1 h1.2.2.1
      apply Fin.ext
      rw [h2.1, hστ ⟨g, hg⟩]
      exact horbeq
  have hLeftInv := hActInv π πInv hπL
  have hRightInv := hActInv πInv π hπR
  -- Unpack `buildRowTupleSlots`, `rowStateOk`, `tuplesOk`, `invOk`
  cases hSlots : buildRowTupleSlots rows.size gCount rowTuples with
  | none => rw [hSlots] at hvalid; simp at hvalid
  | some slots =>
    rw [hSlots] at hvalid
    dsimp only at hvalid
    unfold buildRowTupleSlots at hSlots
    have hgNe0 : (gCount == 0) = false := beq_false_of_ne (by omega)
    simp only [hgNe0, Bool.false_eq_true, ↓reduceIte] at hSlots
    rcases buildRowTupleSlotsAux_props rows.size gCount rowTuples rowTuples.size (Nat.le_refl _) hSlots with
      ⟨hSlotsSz, hSlotsMap, hSlotsInv⟩
    let rowStateOkB : Bool := (List.range rows.size).all fun i =>
      let st := rowState[i]!
      if st == -2 || st == -1 then (slots[i]!).isNone
      else if 0 ≤ st && st < (gCount : Int) then
        match slots[i]! with
        | some (_, g) => decide ((g : Int) = st)
        | none => false
      else false
    have hRowStateOk : rowStateOkB = true := by
      split at hvalid
      · contradiction
      · next hnot =>
        change ¬((!rowStateOkB) = true) at hnot
        cases hc : rowStateOkB
        · exfalso; apply hnot; simp [hc]
        · rfl
    have hNotRowState : (!rowStateOkB) = false := by
      simp only [hRowStateOk]
      decide
    change (if (!rowStateOkB) = true then false else _) = true at hvalid
    rw [hNotRowState] at hvalid
    simp only [Bool.false_eq_true, ↓reduceIte, Bool.and_eq_true] at hvalid
    rcases hvalid with ⟨hTuplesOk, hInvOk⟩
    clear hDim1 hDim2 hNotOrbsCol hNotRowState
    refine ⟨hLeftInv, hRightInv, ?_, ?_⟩
    · -- Preservation of `ExtMipFeasible`
      intro x hfeas
      rcases hfeas with ⟨hxb, hxint, hxrows⟩
      let act := orbitColAction numVars gCount orbits colOrbit colGroup π
      have hActInj : ∀ a b : Fin numVars, act a = act b → a = b := by
        intro a b heq
        have := congrArg (orbitColAction numVars gCount orbits colOrbit colGroup πInv) heq
        rwa [hLeftInv, hLeftInv] at this
      refine ⟨?_, ?_, ?_⟩
      · intro j
        rcases hColEntry j.val j.isLt with ⟨hneg, _⟩ | ⟨honn, holt, hgnn, hglt, _⟩
        · have heq : act j = j := (hActChar π j).1 hneg
          change (lb[j.val]!).leVal (x (act j)) ∧ (ub[j.val]!).geVal (x (act j))
          rw [heq]; exact hxb j
        · let orbID := colOrbit[j.val]!.toNat
          let g := colGroup[j.val]!.toNat
          have h1 := (hActChar π j).2 orbID g (by omega) (by omega)
            (Int.toNat_of_nonneg honn).symm (Int.toNat_of_nonneg hgnn).symm
          rw [← h1.2.2.2.2.1, ← h1.2.2.2.2.2]
          exact hxb (act j)
      · intro j hj
        rcases hColEntry j.val j.isLt with ⟨hneg, _⟩ | ⟨honn, holt, hgnn, hglt, _⟩
        · have heq : act j = j := (hActChar π j).1 hneg
          change ∃ z : Int, x (act j) = (z : Rat)
          rw [heq]; exact hxint j hj
        · let orbID := colOrbit[j.val]!.toNat
          let g := colGroup[j.val]!.toNat
          have h1 := (hActChar π j).2 orbID g (by omega) (by omega)
            (Int.toNat_of_nonneg honn).symm (Int.toNat_of_nonneg hgnn).symm
          rw [← h1.2.2.2.1] at hj
          exact hxint (act j) hj
      · -- Active rows preservation
        intro i hActI
        have hStI := list_all_range_iff.mp hRowStateOk i.val i.isLt
        dsimp only at hStI
        by_cases hNonTup : rowState[i.val]! = -2 ∨ rowState[i.val]! = -1
        · -- Invariant (`-2`) or orbit-disjoint (`-1`) row: fixed by `rowPerm`
          have hInvI := list_all_range_iff.mp hInvOk i.val i.isLt
          have hri : rows[i.val]! = rows[i.val] := getElem!_pos rows i.val i.isLt
          rw [hri] at hInvI
          have hnotGe : ¬(rowState[i.val]! ≥ 0) := by rcases hNonTup with h | h <;> omega
          have hcondFalse : (!rows[i.val].active || decide (rowState[i.val]! ≥ 0)) = false := by
            simp [hActI, hnotGe]
          simp only [hcondFalse, Bool.false_eq_true, ↓reduceIte] at hInvI
          have hColValSz : rows[i.val].cols.size = rows[i.val].vals.size := by
            apply Classical.byContradiction
            intro hne
            have hbt : (rows[i.val].cols.size != rows[i.val].vals.size) = true := by simp [hne]
            simp only [hbt, ↓reduceIte] at hInvI
            contradiction
          have hbtF : (rows[i.val].cols.size != rows[i.val].vals.size) = false := by simp [hColValSz]
          simp only [hbtF, Bool.false_eq_true, ↓reduceIte] at hInvI
          cases hSort : sortUniqueRowPairsList (rowPairsList rows[i.val]) with
          | none => simp [hSort] at hInvI
          | some sortedR =>
            simp only [hSort] at hInvI
            rcases sortUniqueRowPairsList_props hSort with ⟨hSortS, hSortC, hSortM⟩
            have hColsIn : ∀ p ∈ sortedR, p.1 < numVars := by
              split at hInvI
              · contradiction
              · next hnot =>
                have hall : sortedR.all (fun p => decide (p.1 < numVars)) = true := by
                  cases hc : sortedR.all (fun p => decide (p.1 < numVars))
                  · exfalso; apply hnot; simp [hc]
                  · rfl
                intro p hp
                exact of_decide_eq_true (List.all_eq_true.mp hall p hp)
            have hNotColsIn : (!(sortedR.all fun (c, _) => decide (c < numVars))) = false := by
              have hall : sortedR.all (fun p => decide (p.1 < numVars)) = true :=
                List.all_eq_true.mpr (fun p hp => decide_eq_true (hColsIn p hp))
              simp [hall]
            simp only [hNotColsIn, Bool.false_eq_true, ↓reduceIte] at hInvI
            have hOrigCols : ∀ p ∈ rowPairsList rows[i.val], p.1 < numVars :=
              fun p hp => hColsIn p ((hSortM p).mpr hp)
            -- Show `pairCoeffList (rowPairsList rows[i]) (act j) = pairCoeffList (rowPairsList rows[i]) j`
            have hCoeffEq : ∀ j : Fin numVars,
                pairCoeffList (rowPairsList rows[i.val]) j.val =
                pairCoeffList (rowPairsList rows[i.val]) (act j).val := by
              intro j
              rw [← hSortC j.val, ← hSortC (act j).val]
              rcases hColEntry j.val j.isLt with ⟨hneg, _⟩ | ⟨honn, holt, hgnn, hglt, horbeq⟩
              · dsimp only [act]; rw [(hActChar π j).1 hneg]
              · let orbID := colOrbit[j.val]!.toNat
                let g := colGroup[j.val]!.toNat
                have horb : orbID < numOrbits := by omega
                have hg : g < gCount := by omega
                have hActJ := (hActChar π j).2 orbID g horb hg
                  (Int.toNat_of_nonneg honn).symm (Int.toNat_of_nonneg hgnn).symm
                rcases hNonTup with hm2 | hm1
                · have hne1 : (rowState[i.val]! == -1) = false := by rw [hm2]; decide
                  simp only [hne1, Bool.false_eq_true, ↓reduceIte] at hInvI
                  have hOrbInv := list_all_range_iff.mp hInvI orbID horb
                  cases hFind : sortedR.find? (fun p => p.1 == orbits[orbID * gCount]!) with
                  | none =>
                    simp only [hFind] at hOrbInv
                    have hNo1 := list_all_range_iff.mp hOrbInv g hg
                    have hNo2 := list_all_range_iff.mp hOrbInv (π ⟨g, hg⟩).val (π ⟨g, hg⟩).isLt
                    rw [horbeq] at hNo1
                    rw [← hActJ.1] at hNo2
                    have hNot1 : ∀ q ∈ sortedR, q.1 ≠ j.val := by
                      intro q hq heq
                      have hany : sortedR.any (fun p => p.1 == j.val) = true :=
                        List.any_eq_true.mpr ⟨q, hq, beq_iff_eq.mpr heq⟩
                      simp [hany] at hNo1
                    have hNot2 : ∀ q ∈ sortedR, q.1 ≠ (act j).val := by
                      intro q hq heq
                      have hany : sortedR.any (fun p => p.1 == (orbitColAction numVars gCount orbits colOrbit colGroup π j).val) = true :=
                        List.any_eq_true.mpr ⟨q, hq, beq_iff_eq.mpr heq⟩
                      rw [hany] at hNo2
                      simp at hNo2
                    rw [pairCoeff_not_mem hNot1, pairCoeff_not_mem hNot2]
                  | some p0 =>
                    rcases p0 with ⟨c0p, v0⟩
                    simp only [hFind] at hOrbInv
                    have hHas1 := list_all_range_iff.mp hOrbInv g hg
                    have hHas2 := list_all_range_iff.mp hOrbInv (π ⟨g, hg⟩).val (π ⟨g, hg⟩).isLt
                    rw [horbeq] at hHas1
                    rw [← hActJ.1] at hHas2
                    rcases List.any_eq_true.mp hHas1 with ⟨q1, hq1Mem, hq1Eq⟩
                    rcases List.any_eq_true.mp hHas2 with ⟨q2, hq2Mem, hq2Eq⟩
                    simp only [Bool.and_eq_true, beq_iff_eq] at hq1Eq hq2Eq
                    have hq1Pair : q1 = (j.val, v0) := Prod.ext hq1Eq.1 hq1Eq.2
                    have hq2Pair : q2 = ((act j).val, v0) := Prod.ext hq2Eq.1 hq2Eq.2
                    rw [hq1Pair] at hq1Mem
                    rw [hq2Pair] at hq2Mem
                    rw [strictSorted_pairCoeff_mem hSortS hq1Mem, strictSorted_pairCoeff_mem hSortS hq2Mem]
                · have heq1 : (rowState[i.val]! == -1) = true := by rw [hm1]; decide
                  simp only [heq1, ↓reduceIte] at hInvI
                  have hAllNeg : ∀ q ∈ sortedR, colOrbit[q.1]! < 0 := by
                    intro q hq
                    exact of_decide_eq_true (List.all_eq_true.mp hInvI q hq)
                  have hNot1 : ∀ q ∈ sortedR, q.1 ≠ j.val := by
                    intro q hq heq
                    have hlt := hAllNeg q hq
                    rw [heq] at hlt
                    omega
                  have hNot2 : ∀ q ∈ sortedR, q.1 ≠ (act j).val := by
                    intro q hq heq
                    have hlt := hAllNeg q hq
                    rw [heq, hActJ.2.1] at hlt
                    omega
                  rw [pairCoeff_not_mem hNot1, pairCoeff_not_mem hNot2]
            have hEvalEq : evalPairList (rowPairsList rows[i.val]) (fun j => x (act j)) =
                evalPairList (rowPairsList rows[i.val]) x := by
              rw [evalPairList_eq_finSum _ _ hOrigCols, evalPairList_eq_finSum _ _ hOrigCols]
              have hcong :
                  finSum numVars (fun j => pairCoeffList (rowPairsList rows[i.val]) j.val * x (act j)) =
                  finSum numVars (fun j => pairCoeffList (rowPairsList rows[i.val]) (act j).val * x (act j)) :=
                finSum_congr fun j => by rw [hCoeffEq j]
              rw [hcong]
              exact finSum_inj_eq (g := fun j => pairCoeffList (rowPairsList rows[i.val]) j.val * x j) hActInj
            have hOrigHold := hxrows i hActI
            dsimp only [OrbitRowHolds] at hOrigHold ⊢
            rw [hEvalEq]
            exact hOrigHold
        · -- Tuple row (`0 ≤ rowState[i] < gCount`)
          have hNotM : (rowState[i.val]! == -2 || rowState[i.val]! == -1) = false := by
            simp only [Bool.or_eq_false_iff, beq_eq_false_iff_ne]
            exact ⟨fun h => hNonTup (Or.inl h), fun h => hNonTup (Or.inr h)⟩
          simp only [hNotM, Bool.false_eq_true, ↓reduceIte] at hStI
          have hStRange : 0 ≤ rowState[i.val]! ∧ rowState[i.val]! < (gCount : Int) := by
            split at hStI
            · next hr => simpa [Bool.and_eq_true, decide_eq_true_eq] using hr
            · contradiction
          have hStRangeB : (decide (0 ≤ rowState[i.val]!) && decide (rowState[i.val]! < (gCount : Int))) = true := by
            simp [hStRange.1, hStRange.2]
          simp only [hStRangeB, ↓reduceIte] at hStI
          have hSlotPos : slots[i.val]! = slots[i.val]'(by omega) := getElem!_pos slots i.val (by omega)
          rw [hSlotPos] at hStI
          cases hSl : slots[i.val]'(by omega) with
          | none => simp [hSl] at hStI
          | some tg =>
            rcases tg with ⟨tIdx, g⟩
            simp only [hSl, decide_eq_true_eq] at hStI
            rcases hSlotsInv i.val i.isLt tIdx g hSl with ⟨idx, hidx, hrIdxEq, htEq, hgEq⟩
            have hgLt : g < gCount := by rw [hgEq]; exact Nat.mod_lt idx (by omega)
            have htLt : tIdx < numTuples := by
              rw [htEq]
              have : idx < numTuples * gCount := by rwa [hTupSizeEq]
              exact Nat.div_lt_of_lt_mul (by rwa [Nat.mul_comm])
            have hIdxEq : tIdx * gCount + g = idx := by
              rw [htEq, hgEq, Nat.mul_comm]
              exact Nat.div_add_mod idx gCount
            -- Inspect `tuplesOk` at `tIdx`
            have hTup := list_all_range_iff.mp hTuplesOk tIdx htLt
            have hR0IdxLt : tIdx * gCount < rowTuples.size := by
              have : tIdx * gCount + 0 < numTuples * gCount := by
                have := Nat.mul_lt_mul_of_pos_right htLt (show 0 < gCount by omega)
                omega
              omega
            have hR0Bang : rowTuples[tIdx * gCount]! = rowTuples[tIdx * gCount]'hR0IdxLt :=
              getElem!_pos rowTuples (tIdx * gCount) hR0IdxLt
            let r0Idx := rowTuples[tIdx * gCount]!
            have hR0Bounds : r0Idx < rows.size := by
              dsimp only [r0Idx]; split at hTup
              · assumption
              · contradiction
            have hR0Bounds' : rowTuples[tIdx * gCount]! < rows.size := hR0Bounds
            simp only [hR0Bounds', ↓reduceDIte] at hTup
            let r0 := rows[r0Idx]'hR0Bounds
            have hR0BaseOk : r0.active = true ∧ rowState[r0Idx]! = 0 ∧ r0.cols.size = r0.vals.size := by
              split at hTup
              · contradiction
              · next hnot =>
                simp only [Bool.or_eq_true, Bool.not_eq_true', bne_iff_ne] at hnot
                refine ⟨?_, ?_, ?_⟩
                · cases ha : r0.active
                  · exfalso; exact hnot (Or.inl (Or.inl ha))
                  · rfl
                · apply Classical.byContradiction; intro hne; exact hnot (Or.inl (Or.inr hne))
                · apply Classical.byContradiction; intro hne; exact hnot (Or.inr hne)
            have hR0CondF : (!r0.active || rowState[r0Idx]! != 0 || r0.cols.size != r0.vals.size) = false := by
              simp [hR0BaseOk.1, hR0BaseOk.2.1, hR0BaseOk.2.2]
            dsimp only [r0, r0Idx] at hR0CondF
            simp only [hR0CondF, Bool.false_eq_true, ↓reduceIte] at hTup
            cases hSort0 : sortUniqueRowPairsList (rowPairsList (rows[rowTuples[tIdx * gCount]!]'hR0Bounds')) with
            | none => simp [hSort0] at hTup
            | some sorted0 =>
              simp only [hSort0] at hTup
              have hSort0' : sortUniqueRowPairsList (rowPairsList r0) = some sorted0 := hSort0
              rcases sortUniqueRowPairsList_props hSort0' with ⟨_, hSort0C, hSort0M⟩
              have hR0ColsOk : ∀ p ∈ sorted0, p.1 < numVars ∧ (colOrbit[p.1]! < 0 ∨ colGroup[p.1]! = 0) := by
                split at hTup
                · contradiction
                · next hnot =>
                  have hall : sorted0.all (fun p => decide (p.1 < numVars) && (decide (colOrbit[p.1]! < 0) || colGroup[p.1]! == 0)) = true := by
                    cases hc : sorted0.all _
                    · exfalso; exact hnot (by simp [hc])
                    · rfl
                  intro p hp
                  have hp' := List.all_eq_true.mp hall p hp
                  simp only [Bool.and_eq_true, Bool.or_eq_true, decide_eq_true_eq, beq_iff_eq] at hp'
                  exact hp'
              have hNotR0Cols : (!(sorted0.all fun (c0, _) => decide (c0 < numVars) && (decide (colOrbit[c0]! < 0) || colGroup[c0]! == 0))) = false := by
                have hall : sorted0.all (fun p => decide (p.1 < numVars) && (decide (colOrbit[p.1]! < 0) || colGroup[p.1]! == 0)) = true :=
                  List.all_eq_true.mpr fun p hp => by
                    have hp' := hR0ColsOk p hp
                    simp only [Bool.and_eq_true, Bool.or_eq_true, decide_eq_true_eq, beq_iff_eq]
                    exact hp'
                simp [hall]
              simp only [hNotR0Cols, Bool.false_eq_true, ↓reduceIte] at hTup
              -- Helper: characterization of tuple slot `gSlot : Fin gCount`
              let mapG : Fin gCount → Nat → Nat := fun gSlot c0 =>
                let orb := colOrbit[c0]!
                if orb ≥ 0 then orbits[orb.toNat * gCount + gSlot.val]! else c0
              have hSlotRowChar : ∀ gSlot : Fin gCount,
                  let rgIdx := rowTuples[tIdx * gCount + gSlot.val]!
                  ∃ hrg : rgIdx < rows.size,
                    let rg := rows[rgIdx]'hrg
                    rg.active = true ∧ rg.sense = r0.sense ∧ rg.rhs = r0.rhs ∧
                    (∀ p ∈ rowPairsList rg, p.1 < numVars) ∧
                    (∀ c : Nat, pairCoeffList (rowPairsList rg) c =
                      pairCoeffList (sorted0.map fun p => (mapG gSlot p.1, p.2)) c) := by
                intro gSlot
                by_cases hg0 : gSlot.val = 0
                · have hidx0 : tIdx * gCount + gSlot.val = tIdx * gCount := by omega
                  refine ⟨by simpa [hidx0] using hR0Bounds, ?_, by simp [hidx0, r0, r0Idx], by simp [hidx0, r0, r0Idx], ?_, ?_⟩
                  · simpa [hidx0] using hR0BaseOk.1
                  · intro p hp
                    have hp0 : p ∈ rowPairsList r0 := by simpa [hidx0, r0, r0Idx] using hp
                    exact (hR0ColsOk p ((hSort0M p).mpr hp0)).1
                  · intro c
                    have hEq0 : rowPairsList (rows[rowTuples[tIdx * gCount + gSlot.val]!]'(by simpa [hidx0] using hR0Bounds)) =
                        rowPairsList r0 := by simp [hidx0, r0, r0Idx]
                    rw [hEq0, ← hSort0C c]
                    symm
                    apply pairCoeffList_map_iff
                    intro p hp
                    rcases hR0ColsOk p hp with ⟨hpLt, hpOrb | hpGrp⟩
                    · have hnotGe : ¬(colOrbit[p.1]! ≥ 0) := by omega
                      simp [mapG, hnotGe]
                    · rcases hColEntry p.1 hpLt with ⟨hneg, _⟩ | ⟨honn, _, _, _, horbeq⟩
                      · have hnotGe : ¬(colOrbit[p.1]! ≥ 0) := by omega
                        simp [mapG, hnotGe]
                      · have hge : colOrbit[p.1]! ≥ 0 := honn
                        have htoG : colGroup[p.1]!.toNat = 0 := by rw [hpGrp]; rfl
                        rw [htoG] at horbeq
                        simp only [mapG, hge, ↓reduceIte, hg0]
                        rw [horbeq]
                · have hOffLt : gSlot.val - 1 < gCount - 1 := by omega
                  have hG := list_all_range_iff.mp hTup (gSlot.val - 1) hOffLt
                  have hOffEq : gSlot.val - 1 + 1 = gSlot.val := by omega
                  rw [hOffEq] at hG
                  split at hG
                  · next hrg =>
                    let rg := rows[rowTuples[tIdx * gCount + gSlot.val]!]
                    split at hG
                    · contradiction
                    · next hnot =>
                      simp only [Bool.or_eq_true, Bool.not_eq_true', bne_iff_ne] at hnot
                      have hRgBase : rg.active = true ∧ rg.sense = r0.sense ∧ rg.rhs = r0.rhs := by
                        refine ⟨?_, ?_, ?_⟩
                        · cases ha : rg.active
                          · exfalso; exact hnot (Or.inl (Or.inl (Or.inl (Or.inl (Or.inl ha)))))
                          · rfl
                        · apply Classical.byContradiction; intro hne; exact hnot (Or.inl (Or.inl (Or.inl (Or.inr hne))))
                        · apply Classical.byContradiction; intro hne; exact hnot (Or.inl (Or.inl (Or.inr hne)))
                      cases hSm : sortUniqueRowPairsList (sorted0.map fun p => (mapG gSlot p.1, p.2)) with
                      | none => simp [mapG] at hSm; simp [hSm] at hG
                      | some mSorted =>
                        cases hSg : sortUniqueRowPairsList (rowPairsList (rows[rowTuples[tIdx * gCount + gSlot.val]!]'hrg)) with
                        | none => simp [mapG] at hSm; simp [hSm, hSg] at hG
                        | some gSorted =>
                          simp only [mapG] at hSm
                          simp only [hSm, hSg, beq_iff_eq] at hG
                          have hSm' : sortUniqueRowPairsList (sorted0.map fun p => (mapG gSlot p.1, p.2)) = some mSorted := hSm
                          have hSg' : sortUniqueRowPairsList (rowPairsList rg) = some gSorted := hSg
                          rcases sortUniqueRowPairsList_props hSm' with ⟨_, hmC, hmM⟩
                          rcases sortUniqueRowPairsList_props hSg' with ⟨_, hgC, hgM⟩
                          refine ⟨hrg, hRgBase.1, hRgBase.2.1, hRgBase.2.2, ?_, ?_⟩
                          · intro p hp
                            have hpInM : p ∈ mSorted := by rw [hG]; exact (hgM p).mpr hp
                            rcases List.mem_map.mp ((hmM p).mp hpInM) with ⟨q, hqMem, hqEq⟩
                            rw [← hqEq]
                            dsimp only [mapG]
                            rcases hR0ColsOk q hqMem with ⟨hqLt, _⟩
                            by_cases hge : colOrbit[q.1]! ≥ 0
                            · simp only [hge, ↓reduceIte]
                              rcases hColEntry q.1 hqLt with ⟨hneg, _⟩ | ⟨honn, holt, _, _, _⟩
                              · omega
                              · exact (hOrbEntry colOrbit[q.1]!.toNat gSlot.val (by omega) gSlot.isLt).1
                            · simp [hge, hqLt]
                          · intro c
                            rw [← hgC c, ← hG, hmC c]
                  · contradiction
              -- Apply `hSlotRowChar` at `g` (which is row `i`) and `g' = π g`
              let gFin : Fin gCount := ⟨g, hgLt⟩
              let g'Fin : Fin gCount := π gFin
              rcases hSlotRowChar gFin with ⟨hrg, _, hSenseG, hRhsG, hColsG, hCoeffG⟩
              rcases hSlotRowChar g'Fin with ⟨hrg', hActG', hSenseG', hRhsG', hColsG', hCoeffG'⟩
              have hiEqG : rowTuples[tIdx * gCount + gFin.val]! = i.val := by
                have hlt : tIdx * gCount + g < rowTuples.size := by omega
                rw [getElem!_pos rowTuples _ hlt]
                simpa [gFin, hIdxEq] using hrIdxEq
              have hRowIEq : rows[i.val] = rows[rowTuples[tIdx * gCount + gFin.val]!] := by
                simp [hiEqG]
              -- Key iff: `mapG gFin c0 = j.val ↔ mapG g'Fin c0 = (act j).val`
              have hCoeffMatch : ∀ j : Fin numVars,
                  pairCoeffList (rowPairsList rows[i.val]) j.val =
                  pairCoeffList (rowPairsList (rows[rowTuples[tIdx * gCount + g'Fin.val]!]'hrg')) (act j).val := by
                intro j
                have hgi : rowPairsList rows[i.val] = rowPairsList (rows[rowTuples[tIdx * gCount + gFin.val]!]'hrg) :=
                  congrArg rowPairsList hRowIEq
                rw [hgi, hCoeffG j.val, hCoeffG' (act j).val]
                apply pairCoeffList_map_eq_map
                intro p hp
                rcases hR0ColsOk p hp with ⟨hpLt, hpOrb | hpGrp⟩
                · have hnotGe : ¬(colOrbit[p.1]! ≥ 0) := by omega
                  simp only [mapG, hnotGe, ↓reduceIte]
                  rcases hColEntry j.val j.isLt with ⟨hjNeg, _⟩ | ⟨honn, holt, hgnn, hglt, _⟩
                  · dsimp only [act]; rw [(hActChar π j).1 hjNeg]
                  · have hActJ := (hActChar π j).2 colOrbit[j.val]!.toNat colGroup[j.val]!.toNat
                      (by omega) (by omega) (Int.toNat_of_nonneg honn).symm (Int.toNat_of_nonneg hgnn).symm
                    have hne1 : p.1 ≠ j.val := by intro heq; rw [heq] at hpOrb; omega
                    have hne2 : p.1 ≠ (act j).val := by
                      intro heq; have h2 := hActJ.2.1; rw [← heq] at h2; omega
                    rw [beq_false_of_ne hne1, beq_false_of_ne hne2]
                · rcases hColEntry p.1 hpLt with ⟨hpNeg, _⟩ | ⟨hpOnn, hpOlt, _, _, _⟩
                  · have hnotGe : ¬(colOrbit[p.1]! ≥ 0) := by omega
                    simp only [mapG, hnotGe, ↓reduceIte]
                    rcases hColEntry j.val j.isLt with ⟨hjNeg, _⟩ | ⟨honn, holt, hgnn, hglt, _⟩
                    · dsimp only [act]; rw [(hActChar π j).1 hjNeg]
                    · have hActJ := (hActChar π j).2 colOrbit[j.val]!.toNat colGroup[j.val]!.toNat
                        (by omega) (by omega) (Int.toNat_of_nonneg honn).symm (Int.toNat_of_nonneg hgnn).symm
                      have hne1 : p.1 ≠ j.val := by intro heq; rw [heq] at hpNeg; omega
                      have hne2 : p.1 ≠ (act j).val := by
                        intro heq; have h2 := hActJ.2.1; rw [← heq] at h2; omega
                      rw [beq_false_of_ne hne1, beq_false_of_ne hne2]
                  · have hge : colOrbit[p.1]! ≥ 0 := hpOnn
                    simp only [mapG, hge, ↓reduceIte]
                    let orbP := colOrbit[p.1]!.toNat
                    have horbP : orbP < numOrbits := by omega
                    have hOEg := hOrbEntry orbP gFin.val horbP gFin.isLt
                    have hOEg' := hOrbEntry orbP g'Fin.val horbP g'Fin.isLt
                    apply Bool.eq_iff_iff.mpr
                    simp only [beq_iff_eq]
                    constructor
                    · intro hEq
                      rcases hColEntry j.val j.isLt with ⟨hjNeg, _⟩ | ⟨honn, holt, hgnn, hglt, _⟩
                      · exfalso; have := hOEg.2.1; rw [hEq] at this; omega
                      · have hcoJ : colOrbit[j.val]! = (orbP : Int) := by rw [← hEq]; exact hOEg.2.1
                        have hcgJ : colGroup[j.val]! = (gFin.val : Int) := by rw [← hEq]; exact hOEg.2.2.1
                        have hActJ := (hActChar π j).2 orbP gFin.val horbP gFin.isLt hcoJ hcgJ
                        exact hActJ.1.symm
                    · intro hEq
                      rcases hColEntry j.val j.isLt with ⟨hjNeg, _⟩ | ⟨honn, holt, hgnn, hglt, hjOrbEq⟩
                      · exfalso
                        have hAj : act j = j := (hActChar π j).1 hjNeg
                        rw [hAj] at hEq
                        have := hOEg'.2.1; rw [hEq] at this; omega
                      · let orbJ := colOrbit[j.val]!.toNat
                        let gJ := colGroup[j.val]!.toNat
                        have hOrbJLt : orbJ < numOrbits := by omega
                        have hgJLt : gJ < gCount := by omega
                        have hActJ := (hActChar π j).2 orbJ gJ hOrbJLt hgJLt
                          (Int.toNat_of_nonneg honn).symm (Int.toNat_of_nonneg hgnn).symm
                        have hOrbEq : orbP = orbJ := by
                          have h1 := hOEg'.2.1
                          rw [hEq, hActJ.2.1] at h1
                          omega
                        have hπEq : π gFin = π ⟨gJ, hgJLt⟩ := by
                          have h1 := hOEg'.2.2.1
                          rw [hEq, hActJ.2.2.1] at h1
                          exact Fin.ext (by dsimp [g'Fin] at h1; omega)
                        have hgEqFin : gFin = ⟨gJ, hgJLt⟩ := by
                          have := congrArg πInv hπEq
                          rwa [hπL, hπL] at this
                        have hgValEq : gFin.val = gJ := congrArg Fin.val hgEqFin
                        dsimp only [orbP, orbJ, gJ] at hOrbEq hgValEq
                        rw [hOrbEq, hgValEq]
                        exact hjOrbEq
              have hColsI : ∀ p ∈ rowPairsList rows[i.val], p.1 < numVars := by
                intro p hp; apply hColsG p; simpa [hiEqG] using hp
              have hEvalEq : evalPairList (rowPairsList rows[i.val]) (fun j => x (act j)) =
                  evalPairList (rowPairsList (rows[rowTuples[tIdx * gCount + g'Fin.val]!]'hrg')) x := by
                rw [evalPairList_eq_finSum _ _ hColsI, evalPairList_eq_finSum _ _ hColsG']
                have hcong :
                    finSum numVars (fun j => pairCoeffList (rowPairsList rows[i.val]) j.val * x (act j)) =
                    finSum numVars (fun j => pairCoeffList (rowPairsList (rows[rowTuples[tIdx * gCount + g'Fin.val]!]'hrg')) (act j).val * x (act j)) :=
                  finSum_congr fun j => by rw [hCoeffMatch j]
                rw [hcong]
                exact finSum_inj_eq (g := fun j => pairCoeffList (rowPairsList (rows[rowTuples[tIdx * gCount + g'Fin.val]!]'hrg')) j.val * x j) hActInj
              have hHoldG' := hxrows ⟨rowTuples[tIdx * gCount + g'Fin.val]!, hrg'⟩ hActG'
              dsimp only [OrbitRowHolds] at hHoldG' ⊢
              have hSenseI : rows[i.val].sense = (rows[rowTuples[tIdx * gCount + g'Fin.val]!]'hrg').sense := by
                have h1 : rows[i.val].sense = r0.sense := by simpa [hiEqG] using hSenseG
                rw [h1, hSenseG']
              have hRhsI : rows[i.val].rhs = (rows[rowTuples[tIdx * gCount + g'Fin.val]!]'hrg').rhs := by
                have h1 : rows[i.val].rhs = r0.rhs := by simpa [hiEqG] using hRhsG
                rw [h1, hRhsG']
              rw [hEvalEq, hSenseI, hRhsI]
              exact hHoldG'
    · -- Preservation of `mipObj` under `exactOrbitObjSpec`
      intro obj hObjSz hExact x
      let act := orbitColAction numVars gCount orbits colOrbit colGroup π
      have hActInj : ∀ a b : Fin numVars, act a = act b → a = b := by
        intro a b heq
        have := congrArg (orbitColAction numVars gCount orbits colOrbit colGroup πInv) heq
        rwa [hLeftInv, hLeftInv] at this
      unfold exactOrbitObjSpec at hExact
      have hCondF : (gCount < 2 || orbits.size == 0 || orbits.size % gCount != 0) = false := by
        simp [show ¬(gCount < 2) by omega, show orbits.size ≠ 0 by omega, hOrbMod]
      simp only [hCondF, Bool.false_eq_true, ↓reduceIte] at hExact
      have hObjPres : ∀ j : Fin numVars, obj[(act j).val]! = obj[j.val]! := by
        intro j
        rcases hColEntry j.val j.isLt with ⟨hneg, _⟩ | ⟨honn, holt, hgnn, hglt, horbeq⟩
        · dsimp only [act]; rw [(hActChar π j).1 hneg]
        · let orbID := colOrbit[j.val]!.toNat
          let g := colGroup[j.val]!.toNat
          have horb : orbID < numOrbits := by omega
          have hg : g < gCount := by omega
          have hActJ := (hActChar π j).2 orbID g horb hg
            (Int.toNat_of_nonneg honn).symm (Int.toNat_of_nonneg hgnn).symm
          have hOrbEx := list_all_range_iff.mp hExact orbID horb
          have hc0Lt : orbits[orbID * gCount]! < obj.size := by
            rw [hObjSz]; exact (hOrbEntry orbID 0 horb (by omega)).1
          simp only [hc0Lt, ↓reduceDIte] at hOrbEx
          have hObjEq0 : ∀ k (hk : k < gCount), obj[orbits[orbID * gCount + k]!]! = obj[orbits[orbID * gCount]!]! := by
            intro k hk
            by_cases hk0 : k = 0
            · simp [hk0]
            · have hkOff := list_all_range_iff.mp hOrbEx (k - 1) (by omega)
              have hkEq : k - 1 + 1 = k := by omega
              rw [Nat.add_assoc, hkEq] at hkOff
              have hckLt : orbits[orbID * gCount + k]! < obj.size := by
                rw [hObjSz]; exact (hOrbEntry orbID k horb hk).1
              simp only [hckLt, ↓reduceDIte, beq_iff_eq] at hkOff
              rw [getElem!_pos obj _ hckLt, getElem!_pos obj _ hc0Lt]
              exact hkOff
          rw [hActJ.1, hObjEq0 (π ⟨g, hg⟩).val (π ⟨g, hg⟩).isLt, ← hObjEq0 g hg, horbeq]
      unfold mipObj
      have hcong :
          finSum numVars (fun j => obj[j.val]! * x (act j)) =
          finSum numVars (fun j => obj[(act j).val]! * x (act j)) :=
        finSum_congr fun j => by rw [hObjPres j]
      rw [hcong]
      exact finSum_inj_eq (g := fun j => obj[j.val]! * x j) hActInj

/-! ### 7. Multi-Element Symmetric-Group Chain Ordering, Forced-Zero & Orbitope Theorems (T2-M2) -/

/-- Counts inversions `|{(a, b) : a < b ∧ v a < v b}|` in a vector `v : Fin G → Rat`. -/
private def invCount {G : Nat} (v : Fin G → Rat) : Nat :=
  (List.finRange G).foldl (fun acc a =>
    acc + ((List.finRange G).filter (fun b => decide (a.val < b.val) && decide (v a < v b))).length) 0

private theorem list_filter_length_eq_sum {α : Type} (xs : List α) (p : α → Bool) :
    (xs.filter p).length = xs.foldl (fun acc x => acc + if p x then 1 else 0) 0 := by
  suffices h : ∀ (init : Nat),
      init + (xs.filter p).length = xs.foldl (fun acc x => acc + if p x then 1 else 0) init by
    simpa using h 0
  induction xs with
  | nil => intro init; simp
  | cons x rest ih =>
    intro init
    simp only [List.filter_cons, List.foldl_cons]
    cases hp : p x
    · simp only [Bool.false_eq_true, ↓reduceIte, Nat.add_zero]
      exact ih init
    · simp only [↓reduceIte, List.length_cons]
      have h1 := ih (init + 1)
      omega

private theorem invCount_eq_pairSum {G : Nat} (v : Fin G → Rat) :
    invCount v =
      (List.finRange G).foldl (fun acc a =>
        acc + (List.finRange G).foldl (fun acc2 b =>
          acc2 + if a.val < b.val ∧ v a < v b then 1 else 0) 0) 0 := by
  unfold invCount
  congr 1
  funext acc a
  rw [list_filter_length_eq_sum]
  congr 1
  congr 1
  funext acc2 b
  by_cases h1 : a.val < b.val
  · by_cases h2 : v a < v b
    · simp [h1, h2]
    · simp [h1, h2]
  · simp [h1]

private theorem foldl_add_init {α : Type} (xs : List α) (f : α → Nat) (init : Nat) :
    xs.foldl (fun acc x => acc + f x) init = init + xs.foldl (fun acc x => acc + f x) 0 := by
  induction xs generalizing init with
  | nil => simp
  | cons x rest ih =>
    simp only [List.foldl_cons, Nat.zero_add]
    rw [ih (init + f x), ih (f x), Nat.add_assoc]

private theorem foldl_add_le {α : Type} (xs : List α) (f g : α → Nat)
    (h : ∀ x ∈ xs, f x ≤ g x) :
    xs.foldl (fun acc x => acc + f x) 0 ≤ xs.foldl (fun acc x => acc + g x) 0 := by
  induction xs with
  | nil => exact Nat.le_refl 0
  | cons x rest ih =>
    simp only [List.foldl_cons, Nat.zero_add]
    rw [foldl_add_init rest f (f x), foldl_add_init rest g (g x)]
    have h1 := h x (List.mem_cons_self ..)
    have h2 := ih (fun y hy => h y (List.mem_cons_of_mem _ hy))
    omega

private theorem foldl_add_lt {α : Type} (xs : List α) (f g : α → Nat)
    (hle : ∀ x ∈ xs, f x ≤ g x) (hlt : ∃ x ∈ xs, f x < g x) :
    xs.foldl (fun acc x => acc + f x) 0 < xs.foldl (fun acc x => acc + g x) 0 := by
  induction xs with
  | nil => rcases hlt with ⟨_, hmem, _⟩; simp at hmem
  | cons x rest ih =>
    simp only [List.foldl_cons, Nat.zero_add]
    rw [foldl_add_init rest f (f x), foldl_add_init rest g (g x)]
    rcases hlt with ⟨w, hwMem, hwLt⟩
    rcases List.mem_cons.mp hwMem with rfl | hwRest
    · have h2 := foldl_add_le rest f g (fun y hy => hle y (List.mem_cons_of_mem _ hy))
      omega
    · have h1 := hle x (List.mem_cons_self ..)
      have h2 := ih (fun y hy => hle y (List.mem_cons_of_mem _ hy)) ⟨w, hwRest, hwLt⟩
      omega

private theorem foldl_finRange_swap {G : Nat} (i j : Fin G) (f : Fin G → Nat) :
    (List.finRange G).foldl (fun acc k => acc + f (swapIdx i j k)) 0 =
    (List.finRange G).foldl (fun acc k => acc + f k) 0 := by
  -- Use `finSum_inj_eq` via `Nat -> Rat` cast!
  have hcast : ∀ (g : Fin G → Nat),
      (((List.foldl (fun (acc : Nat) k => acc + g k) 0 (List.finRange G)) : Nat) : Rat) =
        finSum G (fun k => (g k : Rat)) := by
    intro g
    suffices h : ∀ (k : Nat) (hk : k ≤ G),
        (((List.foldl (fun (acc : Nat) x => acc + g x) 0 ((List.finRange G).take k)) : Nat) : Rat) =
        finSum k (fun x => (g ⟨x.val, by omega⟩ : Rat)) by
      have hfull := h G (Nat.le_refl G)
      have htake : (List.finRange G).take G = List.finRange G :=
        List.take_of_length_le (by simp)
      rw [htake] at hfull
      exact hfull
    intro k hk
    induction k with
    | zero => simp [finSum]
    | succ k ih =>
      have hlt : k < (List.finRange G).length := by simp; omega
      rw [List.take_succ_eq_append_getElem hlt, List.foldl_append]
      simp only [List.foldl_cons, List.foldl_nil, Rat.natCast_add, ih (by omega), finSum_succ]
      congr 1
      have hget : (List.finRange G)[k]'hlt = ⟨k, by omega⟩ := by simp
      rw [hget]
      rfl
  have heqRat :
      (((List.foldl (fun (acc : Nat) k => acc + f (swapIdx i j k)) 0 (List.finRange G)) : Nat) : Rat) =
      (((List.foldl (fun (acc : Nat) k => acc + f k) 0 (List.finRange G)) : Nat) : Rat) := by
    rw [hcast (fun k => f (swapIdx i j k)), hcast f]
    exact finSum_inj_eq (g := fun k => (f k : Rat)) (fun a b h => by
      have := congrArg (swapIdx i j) h
      rwa [swapIdx_involutive, swapIdx_involutive] at this)
  exact Rat.natCast_inj.mp heqRat

private theorem invCount_adj_swap_lt {G : Nat} (v : Fin G → Rat) (g : Nat) (hg : g + 1 < G)
    (hinv : v ⟨g, by omega⟩ < v ⟨g + 1, hg⟩) :
    let i : Fin G := ⟨g, by omega⟩
    let j : Fin G := ⟨g + 1, hg⟩
    invCount (fun k => v (swapIdx i j k)) < invCount v := by
  intro i j
  let v' : Fin G → Rat := fun k => v (swapIdx i j k)
  rw [invCount_eq_pairSum v', invCount_eq_pairSum v]
  -- Pairwise comparison after permuting outer index by `swapIdx i j`
  let rowCnt (w : Fin G → Rat) (a : Fin G) : Nat :=
    (List.finRange G).foldl (fun acc b => acc + if a.val < b.val ∧ w a < w b then 1 else 0) 0
  change (List.finRange G).foldl (fun acc a => acc + rowCnt v' a) 0 <
         (List.finRange G).foldl (fun acc a => acc + rowCnt v a) 0
  rw [← foldl_finRange_swap i j (rowCnt v')]
  apply foldl_add_lt
  · intro a _
    dsimp only [rowCnt]
    rw [← foldl_finRange_swap i j (fun b => if (swapIdx i j a).val < b.val ∧ v' (swapIdx i j a) < v' b then 1 else 0)]
    apply foldl_add_le
    intro b _
    have hvi : v' (swapIdx i j a) = v a := by dsimp [v']; rw [swapIdx_involutive]
    have hvb : v' (swapIdx i j b) = v b := by dsimp [v']; rw [swapIdx_involutive]
    rw [hvi, hvb]
    by_cases ha_eq : a = i
    · subst ha_eq
      by_cases hb_eq : b = j
      · subst hb_eq
        have hnot : ¬((swapIdx i j i).val < (swapIdx i j j).val) := by
          rw [swapIdx_left, swapIdx_right]; dsimp [i, j]; omega
        simp [hnot]
      · by_cases hb_i : b = i
        · subst hb_i; simp [swapIdx_left]
        · have hsb : swapIdx i j b = b := by unfold swapIdx; simp [hb_i, hb_eq]
          have hord : (swapIdx i j i).val < b.val → i.val < b.val := by
            rw [swapIdx_left]; dsimp [i, j]; omega
          split
          · next h => simp [hsb] at h; simp [hord h.1, h.2]
          · simp
    · by_cases ha_j : a = j
      · subst ha_j
        by_cases hb_i : b = i
        · subst hb_i
          have hnot : ¬(v j < v i) := Rat.not_lt.mpr (Rat.le_of_lt hinv)
          simp [hnot]
        · by_cases hb_j : b = j
          · subst hb_j; simp [swapIdx_right]
          · have hsb : swapIdx i j b = b := by unfold swapIdx; simp [hb_i, hb_j]
            have hord : (swapIdx i j j).val < b.val ↔ j.val < b.val := by
              rw [swapIdx_right]; dsimp [i, j]
              have hne1 : b.val ≠ g := fun h => hb_i (Fin.ext h)
              have hne2 : b.val ≠ g + 1 := fun h => hb_j (Fin.ext h)
              omega
            rw [hsb]; simp only [hord]; exact Nat.le_refl _
      · have hsa : swapIdx i j a = a := by unfold swapIdx; simp [ha_eq, ha_j]
        rw [hsa]
        by_cases hb_i : b = i
        · subst hb_i
          rw [swapIdx_left]
          have hord : a.val < j.val ↔ a.val < i.val := by
            dsimp [i, j]
            have hne1 : a.val ≠ g := fun h => ha_eq (Fin.ext h)
            have hne2 : a.val ≠ g + 1 := fun h => ha_j (Fin.ext h)
            omega
          simp only [hord]; exact Nat.le_refl _
        · by_cases hb_j : b = j
          · subst hb_j
            rw [swapIdx_right]
            have hord : a.val < i.val → a.val < j.val := by dsimp [i, j]; omega
            split
            · next h => simp [hord h.1, h.2]
            · simp
          · have hsb : swapIdx i j b = b := by unfold swapIdx; simp [hb_i, hb_j]
            rw [hsb]; exact Nat.le_refl _
  · refine ⟨i, List.mem_finRange i, ?_⟩
    dsimp only [rowCnt]
    rw [← foldl_finRange_swap i j (fun b => if (swapIdx i j i).val < b.val ∧ v' (swapIdx i j i) < v' b then 1 else 0)]
    apply foldl_add_lt
    · intro b _
      have hvi : v' (swapIdx i j i) = v i := by dsimp [v']; rw [swapIdx_involutive]
      have hvb : v' (swapIdx i j b) = v b := by dsimp [v']; rw [swapIdx_involutive]
      rw [hvi, hvb]
      by_cases hb_eq : b = j
      · subst hb_eq
        have hnot : ¬((swapIdx i j i).val < (swapIdx i j j).val) := by
          rw [swapIdx_left, swapIdx_right]; dsimp [i, j]; omega
        simp [hnot]
      · by_cases hb_i : b = i
        · subst hb_i; simp [swapIdx_left]
        · have hsb : swapIdx i j b = b := by unfold swapIdx; simp [hb_i, hb_eq]
          have hord : (swapIdx i j i).val < b.val → i.val < b.val := by
            rw [swapIdx_left]; dsimp [i, j]; omega
          rw [hsb]
          split
          · next h => simp [hord h.1, h.2]
          · simp
    · refine ⟨j, List.mem_finRange j, ?_⟩
      have hvi : v' (swapIdx i j i) = v i := by dsimp [v']; rw [swapIdx_involutive]
      have hvj : v' (swapIdx i j j) = v j := by dsimp [v']; rw [swapIdx_involutive]
      rw [hvi, hvj, swapIdx_left, swapIdx_right]
      have hij : i.val < j.val := by dsimp [i, j]; omega
      have hji : ¬(j.val < i.val) := by dsimp [i, j]; omega
      simp only [hij, hji, false_and, ↓reduceIte, true_and]
      dsimp only [i, j]
      simp [hinv]

/-- Multi-element `S_G` chain ordering under adjacent group transpositions (finite bubble-sort
composition): if every adjacent swap `swap(g, g+1)` preserves feasibility and does not increase
the objective whenever `x (y g) < x (y (g+1))`, then for every feasible `x` there exists a
feasible `x'` with `obj x' ≤ obj x` satisfying ALL `G - 1` ordering inequalities
`x' (y (g+1)) ≤ x' (y g)` simultaneously. -/
theorem orbit_chain_ordering_by_adj_swaps
    {n G : Nat} {Feas : (Fin n → Rat) → Prop} {obj : (Fin n → Rat) → Rat}
    {y : Fin G → Fin n}
    {adjSwap : (g : Nat) → g + 1 < G → Fin n → Fin n}
    (hSwapY : ∀ g (hg : g + 1 < G) (k : Fin G),
      adjSwap g hg (y k) = y (swapIdx ⟨g, by omega⟩ ⟨g + 1, hg⟩ k))
    (hStep : ∀ g (hg : g + 1 < G) (x : Fin n → Rat),
      Feas x → x (y ⟨g, by omega⟩) < x (y ⟨g + 1, hg⟩) →
      Feas (fun j => x (adjSwap g hg j)) ∧ obj (fun j => x (adjSwap g hg j)) ≤ obj x)
    {x : Fin n → Rat} (hx : Feas x) :
    ∃ x' : Fin n → Rat, Feas x' ∧ obj x' ≤ obj x ∧
      ∀ (g : Nat) (hg : g + 1 < G), x' (y ⟨g + 1, hg⟩) ≤ x' (y ⟨g, by omega⟩) := by
  suffices h : ∀ (bound : Nat) (x : Fin n → Rat),
      invCount (fun k => x (y k)) ≤ bound → Feas x →
      ∃ x' : Fin n → Rat, Feas x' ∧ obj x' ≤ obj x ∧
        ∀ (g : Nat) (hg : g + 1 < G), x' (y ⟨g + 1, hg⟩) ≤ x' (y ⟨g, by omega⟩) by
    exact h (invCount (fun k => x (y k))) x (Nat.le_refl _) hx
  intro bound
  induction bound with
  | zero =>
    intro x h0 hfx
    refine ⟨x, hfx, Rat.le_refl, ?_⟩
    intro g hg
    apply Classical.byContradiction
    intro hnot
    have hlt : x (y ⟨g, by omega⟩) < x (y ⟨g + 1, hg⟩) := Rat.not_le.mp hnot
    have hdec := invCount_adj_swap_lt (fun k => x (y k)) g hg hlt
    omega
  | succ b ih =>
    intro x hle hfx
    by_cases hord : ∀ (g : Nat) (hg : g + 1 < G), x (y ⟨g + 1, hg⟩) ≤ x (y ⟨g, by omega⟩)
    · exact ⟨x, hfx, Rat.le_refl, hord⟩
    · have hex : ∃ g, ∃ hg : g + 1 < G, x (y ⟨g, by omega⟩) < x (y ⟨g + 1, hg⟩) := by
        apply Classical.byContradiction
        intro hno
        apply hord
        intro g hg
        apply Classical.byContradiction
        intro hnot
        exact hno ⟨g, hg, Rat.not_le.mp hnot⟩
      rcases hex with ⟨g, hg, hlt⟩
      let x1 : Fin n → Rat := fun j => x (adjSwap g hg j)
      rcases hStep g hg x hfx hlt with ⟨hfx1, hobj1⟩
      have hvy : (fun k => x1 (y k)) = fun k => x (y (swapIdx ⟨g, by omega⟩ ⟨g + 1, hg⟩ k)) := by
        funext k; dsimp [x1]; rw [hSwapY g hg k]
      have hdec : invCount (fun k => x1 (y k)) < invCount (fun k => x (y k)) := by
        rw [hvy]
        exact invCount_adj_swap_lt (fun k => x (y k)) g hg hlt
      rcases ih x1 (by omega) hfx1 with ⟨x', hfx', hobj', hord'⟩
      exact ⟨x', hfx', Rat.le_trans hobj' hobj1, hord'⟩

/-- Multi-orbit objective comparison under an adjacent group swap `(g, g+1)`:
if every orbit either has constant objective (`c(orb, g) = c(orb, g+1)`) OR has
monotone objective (`c(orb, g) ≤ c(orb, g+1)`) with `x(orb, g) ≤ x(orb, g+1)` (which
holds for binary `openOrb` and every non-negative `forcedZero` orbit whenever `x(y, g) = 0`),
then swapping group `g` and `g+1` across all orbits simultaneously does not increase `mipObj`. -/
theorem orbit_adj_swap_obj_le_of_forced_zero
    {n G numOrbits : Nat}
    {orbCol : Fin numOrbits → Fin G → Fin n}
    {colOrb : Fin n → Option (Fin numOrbits × Fin G)}
    {c x : Fin n → Rat}
    (hBij1 : ∀ o g, colOrb (orbCol o g) = some (o, g))
    (hBij2 : ∀ j o g, colOrb j = some (o, g) → orbCol o g = j)
    (g : Nat) (hg : g + 1 < G)
    (hOrbDelta : ∀ o : Fin numOrbits,
      c (orbCol o ⟨g, by omega⟩) = c (orbCol o ⟨g + 1, hg⟩) ∨
      (c (orbCol o ⟨g, by omega⟩) ≤ c (orbCol o ⟨g + 1, hg⟩) ∧
       x (orbCol o ⟨g, by omega⟩) ≤ x (orbCol o ⟨g + 1, hg⟩))) :
    let g0 : Fin G := ⟨g, by omega⟩
    let g1 : Fin G := ⟨g + 1, hg⟩
    let swapCol : Fin n → Fin n := fun j =>
      match colOrb j with
      | none => j
      | some (o, grp) => orbCol o (swapIdx g0 g1 grp)
    mipObj c (fun j => x (swapCol j)) ≤ mipObj c x := by
  intro g0 g1 swapCol
  have hInv : ∀ j : Fin n, swapCol (swapCol j) = j := by
    intro j
    dsimp only [swapCol]
    cases hco : colOrb j with
    | none => simp [hco]
    | some og =>
      rcases og with ⟨o, grp⟩
      simp only [hBij1 o (swapIdx g0 g1 grp), swapIdx_involutive]
      exact hBij2 j o grp hco
  have hInj : ∀ a b : Fin n, swapCol a = swapCol b → a = b := by
    intro a b heq
    have := congrArg swapCol heq
    rwa [hInv, hInv] at this
  -- Rewrite `mipObj c (x ∘ swapCol) = ∑ c(swapCol j) * x(j)` via involution `swapCol`!
  unfold mipObj
  have hPerm : finSum n (fun j => c j * x (swapCol j)) =
      finSum n (fun j => c (swapCol j) * x j) := by
    have h1 := finSum_inj_eq (g := fun j => c (swapCol j) * x j) hInj
    rw [← h1]
    exact finSum_congr fun j => by rw [hInv j]
  rw [hPerm]
  -- Compare `∑ (c(swapCol j) - c j) * x j ≤ 0`, decomposing by orbits
  have hSubLe : finSum n (fun j => (c (swapCol j) - c j) * x j) ≤ 0 := by
    let term (j : Fin n) : Rat := (c (swapCol j) - c j) * x j
    have hNonSwapZero : ∀ j : Fin n,
        (match colOrb j with | none => True | some (_, grp) => grp ≠ g0 ∧ grp ≠ g1) →
        term j = 0 := by
      intro j hns
      dsimp only [term, swapCol]
      cases hco : colOrb j with
      | none => simp [Rat.sub_self, Rat.zero_mul]
      | some og =>
        rcases og with ⟨o, grp⟩
        rw [hco] at hns
        have hfix : swapIdx g0 g1 grp = grp := by unfold swapIdx; simp [hns.1, hns.2]
        simp only [hfix, hBij2 j o grp hco, Rat.sub_self, Rat.zero_mul]
    have hDecomp : finSum n term =
        finSum numOrbits (fun o => term (orbCol o g0) + term (orbCol o g1)) := by
      rw [finSum_add]
      have hpair : ∀ j : Fin n,
          term j =
            finSum numOrbits (fun o => if j = orbCol o g0 then term (orbCol o g0) else 0) +
            finSum numOrbits (fun o => if j = orbCol o g1 then term (orbCol o g1) else 0) := by
        intro j
        cases hco : colOrb j with
        | none =>
          have h1 : ∀ o, j ≠ orbCol o g0 := fun o heq => by rw [heq, hBij1] at hco; contradiction
          have h2 : ∀ o, j ≠ orbCol o g1 := fun o heq => by rw [heq, hBij1] at hco; contradiction
          have hc1 : ∀ o, (if j = orbCol o g0 then term (orbCol o g0) else 0) = 0 := fun o => by simp [h1 o]
          have hc2 : ∀ o, (if j = orbCol o g1 then term (orbCol o g1) else 0) = 0 := fun o => by simp [h2 o]
          have hz1 : finSum numOrbits (fun o => if j = orbCol o g0 then term (orbCol o g0) else 0) = 0 := by
            rw [finSum_congr hc1]; exact finSum_zero numOrbits
          have hz2 : finSum numOrbits (fun o => if j = orbCol o g1 then term (orbCol o g1) else 0) = 0 := by
            rw [finSum_congr hc2]; exact finSum_zero numOrbits
          rw [hz1, hz2, Rat.add_zero]
          exact hNonSwapZero j (by simp [hco])
        | some og =>
          rcases og with ⟨o0, grp⟩
          have hjEq : orbCol o0 grp = j := hBij2 j o0 grp hco
          by_cases hgrp0 : grp = g0
          · subst hgrp0
            have hne01 : g0 ≠ g1 := by intro h; have := congrArg Fin.val h; dsimp [g0, g1] at this; omega
            have h1 : finSum numOrbits (fun o => if j = orbCol o g0 then term (orbCol o g0) else 0) = term (orbCol o0 g0) := by
              have hc : ∀ o, (if j = orbCol o g0 then term (orbCol o g0) else 0) =
                  if o = o0 then term (orbCol o0 g0) else 0 := by
                intro o
                by_cases ho : o = o0
                · subst ho; simp [hjEq]
                · have hne : j ≠ orbCol o g0 := by
                    intro heq; rw [heq, hBij1] at hco; simp at hco; exact ho hco
                  simp [ho, hne]
              rw [finSum_congr hc, finSum_single]
            have h2 : finSum numOrbits (fun o => if j = orbCol o g1 then term (orbCol o g1) else 0) = 0 := by
              have hne : ∀ o, j ≠ orbCol o g1 := by
                intro o heq; rw [heq, hBij1] at hco; simp at hco; exact hne01 hco.2.symm
              have hc : ∀ o, (if j = orbCol o g1 then term (orbCol o g1) else 0) = 0 := fun o => by simp [hne o]
              rw [finSum_congr hc]; exact finSum_zero numOrbits
            rw [h1, h2, Rat.add_zero, hjEq]
          · by_cases hgrp1 : grp = g1
            · subst hgrp1
              have h1 : finSum numOrbits (fun o => if j = orbCol o g0 then term (orbCol o g0) else 0) = 0 := by
                have hne : ∀ o, j ≠ orbCol o g0 := by
                  intro o heq; rw [heq, hBij1] at hco; simp at hco; exact hgrp0 hco.2.symm
                have hc : ∀ o, (if j = orbCol o g0 then term (orbCol o g0) else 0) = 0 := fun o => by simp [hne o]
                rw [finSum_congr hc]; exact finSum_zero numOrbits
              have h2 : finSum numOrbits (fun o => if j = orbCol o g1 then term (orbCol o g1) else 0) = term (orbCol o0 g1) := by
                have hc : ∀ o, (if j = orbCol o g1 then term (orbCol o g1) else 0) =
                    if o = o0 then term (orbCol o0 g1) else 0 := by
                  intro o
                  by_cases ho : o = o0
                  · subst ho; simp [hjEq]
                  · have hne : j ≠ orbCol o g1 := by
                      intro heq; rw [heq, hBij1] at hco; simp at hco; exact ho hco
                    simp [ho, hne]
                rw [finSum_congr hc, finSum_single]
              rw [h1, h2, Rat.zero_add, hjEq]
            · have h1 : finSum numOrbits (fun o => if j = orbCol o g0 then term (orbCol o g0) else 0) = 0 := by
                have hne : ∀ o, j ≠ orbCol o g0 := by
                  intro o heq; rw [heq, hBij1] at hco; simp at hco; exact hgrp0 hco.2.symm
                have hc : ∀ o, (if j = orbCol o g0 then term (orbCol o g0) else 0) = 0 := fun o => by simp [hne o]
                rw [finSum_congr hc]; exact finSum_zero numOrbits
              have h2 : finSum numOrbits (fun o => if j = orbCol o g1 then term (orbCol o g1) else 0) = 0 := by
                have hne : ∀ o, j ≠ orbCol o g1 := by
                  intro o heq; rw [heq, hBij1] at hco; simp at hco; exact hgrp1 hco.2.symm
                have hc : ∀ o, (if j = orbCol o g1 then term (orbCol o g1) else 0) = 0 := fun o => by simp [hne o]
                rw [finSum_congr hc]; exact finSum_zero numOrbits
              rw [h1, h2, Rat.add_zero]
              exact hNonSwapZero j (by simp [hco, hgrp0, hgrp1])
      rw [finSum_congr hpair, finSum_add]
      have hswap : ∀ (m : Nat) (f : Fin numOrbits → Fin m → Rat),
          finSum m (fun j => finSum numOrbits (fun o => f o j)) =
          finSum numOrbits (fun o => finSum m (fun j => f o j)) := by
        intro m
        induction m with
        | zero => intro f; simp [finSum_zero]
        | succ m ih =>
          intro f
          simp only [finSum_succ]
          rw [ih (fun o j => f o j.castSucc), ← finSum_add]
      rw [hswap n, hswap n]
      congr 1
      · exact finSum_congr fun o => finSum_single (orbCol o g0) (term (orbCol o g0))
      · exact finSum_congr fun o => finSum_single (orbCol o g1) (term (orbCol o g1))
    rw [hDecomp]
    have hEach : ∀ o : Fin numOrbits, term (orbCol o g0) + term (orbCol o g1) ≤ 0 := by
      intro o
      have hs0 : swapCol (orbCol o g0) = orbCol o g1 := by
        simp [swapCol, hBij1, swapIdx_left]
      have hs1 : swapCol (orbCol o g1) = orbCol o g0 := by
        simp [swapCol, hBij1, swapIdx_right]
      dsimp only [term]
      rw [hs0, hs1]
      rcases hOrbDelta o with hceq | ⟨hcLe, hxLe⟩
      · dsimp only [g0, g1]
        simp [hceq, Rat.sub_self, Rat.zero_mul, Rat.add_zero]
      · have hfact :
            (c (orbCol o g1) - c (orbCol o g0)) * x (orbCol o g0) +
            (c (orbCol o g0) - c (orbCol o g1)) * x (orbCol o g1) =
            -((c (orbCol o g1) - c (orbCol o g0)) * (x (orbCol o g1) - x (orbCol o g0))) := by
          have h1 : c (orbCol o g0) - c (orbCol o g1) = -(c (orbCol o g1) - c (orbCol o g0)) := by
            rw [Rat.neg_sub]
          rw [h1, Rat.neg_mul, ← Rat.sub_eq_add_neg, ← rat_mul_sub, ← Rat.mul_neg, Rat.neg_sub]
        rw [hfact]
        have hcd : 0 ≤ c (orbCol o g1) - c (orbCol o g0) := (Rat.le_iff_sub_nonneg _ _).mp hcLe
        have hxd : 0 ≤ x (orbCol o g1) - x (orbCol o g0) := (Rat.le_iff_sub_nonneg _ _).mp hxLe
        have hnn : 0 ≤ (c (orbCol o g1) - c (orbCol o g0)) * (x (orbCol o g1) - x (orbCol o g0)) :=
          Rat.mul_nonneg hcd hxd
        simpa using Rat.neg_le_neg hnn
    have hsum := finSum_le_finSum hEach
    simpa [finSum_zero] using hsum
  have hDiff : finSum n (fun j => c (swapCol j) * x j) - finSum n (fun j => c j * x j) ≤ 0 := by
    rw [← finSum_sub]
    have hc : ∀ j, c (swapCol j) * x j - c j * x j = (c (swapCol j) - c j) * x j := fun j => by
      rw [Rat.sub_eq_add_neg, Rat.sub_eq_add_neg, Rat.add_mul, Rat.neg_mul]
    rw [finSum_congr hc]
    exact hSubLe
  have hadd := (Rat.add_le_add_right (c := finSum n (fun j => c j * x j))).mpr hDiff
  rwa [Rat.sub_add_cancel, Rat.zero_add] at hadd

/-- Full Rule B multi-element monotone chain ordering soundness with forced-zero non-constant
objective orbits (`presolve.go:3377-3457`):
under `S_G` feasibility invariance, if `openOrb` is binary with non-decreasing objective and every
other orbit either has constant objective OR has non-decreasing objective, non-negative values,
and is forced to `0` whenever `x (openOrb, g) = 0`, then adding ALL `G - 1` ordering cuts
`x (openOrb, g+1) ≤ x (openOrb, g)` (`g = 0 .. G - 2`) preserves existence of solutions of
objective `≤ z`. -/
theorem orbit_chain_ordering_forced_zero_sound
    {n G numOrbits : Nat}
    {Feas : (Fin n → Rat) → Prop} {c : Fin n → Rat}
    {orbCol : Fin numOrbits → Fin G → Fin n}
    {colOrb : Fin n → Option (Fin numOrbits × Fin G)}
    (openOrb : Fin numOrbits)
    (hBij1 : ∀ o g, colOrb (orbCol o g) = some (o, g))
    (hBij2 : ∀ j o g, colOrb j = some (o, g) → orbCol o g = j)
    (hSym : ∀ (π : Fin G → Fin G), (∀ g, π (π g) = g) →
      ∀ x, Feas x → Feas (fun j =>
        x (match colOrb j with | none => j | some (o, grp) => orbCol o (π grp))))
    (hOpenBin : ∀ x, Feas x → ∀ g : Fin G, x (orbCol openOrb g) = 0 ∨ x (orbCol openOrb g) = 1)
    (hOrbitsObj : ∀ o : Fin numOrbits,
      (∀ g : Fin G, c (orbCol o g) = c (orbCol o ⟨0, Nat.lt_of_le_of_lt (Nat.zero_le _) g.isLt⟩)) ∨
      ((∀ (g : Nat) (hg : g + 1 < G), c (orbCol o ⟨g, by omega⟩) ≤ c (orbCol o ⟨g + 1, hg⟩)) ∧
       (∀ x, Feas x → ∀ g : Fin G, 0 ≤ x (orbCol o g)) ∧
       (∀ x, Feas x → ∀ g : Fin G, x (orbCol openOrb g) = 0 → x (orbCol o g) = 0)))
    {x : Fin n → Rat} (hx : Feas x) :
    ∃ x' : Fin n → Rat, Feas x' ∧ mipObj c x' ≤ mipObj c x ∧
      ∀ (g : Nat) (hg : g + 1 < G),
        x' (orbCol openOrb ⟨g + 1, hg⟩) ≤ x' (orbCol openOrb ⟨g, by omega⟩) := by
  let adjSwap : (g : Nat) → g + 1 < G → Fin n → Fin n := fun g hg j =>
    match colOrb j with
    | none => j
    | some (o, grp) => orbCol o (swapIdx ⟨g, by omega⟩ ⟨g + 1, hg⟩ grp)
  apply orbit_chain_ordering_by_adj_swaps (y := orbCol openOrb) (adjSwap := adjSwap)
  · intro g hg k
    dsimp only [adjSwap]
    rw [hBij1]
  · intro g hg xCur hfxCur hlt
    have hInvG : ∀ k : Fin G, swapIdx ⟨g, by omega⟩ ⟨g + 1, hg⟩ (swapIdx ⟨g, by omega⟩ ⟨g + 1, hg⟩ k) = k :=
      fun k => swapIdx_involutive _ _ k
    refine ⟨hSym (swapIdx ⟨g, by omega⟩ ⟨g + 1, hg⟩) hInvG xCur hfxCur, ?_⟩
    have hx0 : xCur (orbCol openOrb ⟨g, by omega⟩) = 0 := by
      rcases hOpenBin xCur hfxCur ⟨g, by omega⟩ with h0 | h1
      · exact h0
      · rcases hOpenBin xCur hfxCur ⟨g + 1, hg⟩ with h0' | h1'
        · rw [h1, h0'] at hlt; exact absurd hlt (by decide)
        · rw [h1, h1'] at hlt; exact absurd hlt Rat.lt_irrefl
    apply orbit_adj_swap_obj_le_of_forced_zero hBij1 hBij2 g hg
    intro o
    rcases hOrbitsObj o with hConst | ⟨hMono, hNN, hFZ⟩
    · left
      rw [hConst ⟨g, by omega⟩, hConst ⟨g + 1, hg⟩]
    · right
      refine ⟨hMono g hg, ?_⟩
      rw [hFZ xCur hfxCur ⟨g, by omega⟩ hx0]
      exact hNN xCur hfxCur ⟨g + 1, hg⟩
  · exact hx

/-- Exact-objective multi-element `S_G` chain ordering soundness (for arbitrary rational orbit
values under group-invariant objective): finite bubble-sort composition yields `x'` with
`mipObj c x' = mipObj c x` and `x' (y (g+1)) ≤ x' (y g)` for all `g = 0 .. G - 2`. -/
theorem orbit_exact_chain_ordering_sound
    {n G : Nat} {Feas : (Fin n → Rat) → Prop} {c : Fin n → Rat}
    {y : Fin G → Fin n}
    {adjSwap : (g : Nat) → g + 1 < G → Fin n → Fin n}
    (hSwapY : ∀ g (hg : g + 1 < G) (k : Fin G),
      adjSwap g hg (y k) = y (swapIdx ⟨g, by omega⟩ ⟨g + 1, hg⟩ k))
    (hFeas : ∀ g (hg : g + 1 < G) (x : Fin n → Rat),
      Feas x → Feas (fun j => x (adjSwap g hg j)))
    (hObj : ∀ g (hg : g + 1 < G) (x : Fin n → Rat),
      mipObj c (fun j => x (adjSwap g hg j)) = mipObj c x)
    {x : Fin n → Rat} (hx : Feas x) :
    ∃ x' : Fin n → Rat, Feas x' ∧ mipObj c x' = mipObj c x ∧
      ∀ (g : Nat) (hg : g + 1 < G), x' (y ⟨g + 1, hg⟩) ≤ x' (y ⟨g, by omega⟩) := by
  have hStep : ∀ g (hg : g + 1 < G) (x : Fin n → Rat),
      Feas x → x (y ⟨g, by omega⟩) < x (y ⟨g + 1, hg⟩) →
      (Feas (fun j => x (adjSwap g hg j)) ∧ mipObj c (fun j => x (adjSwap g hg j)) = mipObj c x) :=
    fun g hg x hfx _ => ⟨hFeas g hg x hfx, hObj g hg x⟩
  suffices h : ∀ (bound : Nat) (x : Fin n → Rat),
      invCount (fun k => x (y k)) ≤ bound → Feas x →
      ∃ x' : Fin n → Rat, Feas x' ∧ mipObj c x' = mipObj c x ∧
        ∀ (g : Nat) (hg : g + 1 < G), x' (y ⟨g + 1, hg⟩) ≤ x' (y ⟨g, by omega⟩) by
    exact h (invCount (fun k => x (y k))) x (Nat.le_refl _) hx
  intro bound
  induction bound with
  | zero =>
    intro x h0 hfx
    refine ⟨x, hfx, rfl, ?_⟩
    intro g hg
    apply Classical.byContradiction
    intro hnot
    have hlt := Rat.not_le.mp hnot
    have hdec := invCount_adj_swap_lt (fun k => x (y k)) g hg hlt
    omega
  | succ b ih =>
    intro x hle hfx
    by_cases hord : ∀ (g : Nat) (hg : g + 1 < G), x (y ⟨g + 1, hg⟩) ≤ x (y ⟨g, by omega⟩)
    · exact ⟨x, hfx, rfl, hord⟩
    · have hex : ∃ g, ∃ hg : g + 1 < G, x (y ⟨g, by omega⟩) < x (y ⟨g + 1, hg⟩) := by
        apply Classical.byContradiction
        intro hno
        apply hord
        intro g hg
        apply Classical.byContradiction
        intro hnot
        exact hno ⟨g, hg, Rat.not_le.mp hnot⟩
      rcases hex with ⟨g, hg, hlt⟩
      let x1 : Fin n → Rat := fun j => x (adjSwap g hg j)
      rcases hStep g hg x hfx hlt with ⟨hfx1, hobj1⟩
      have hvy : (fun k => x1 (y k)) = fun k => x (y (swapIdx ⟨g, by omega⟩ ⟨g + 1, hg⟩ k)) := by
        funext k; dsimp [x1]; rw [hSwapY g hg k]
      have hdec : invCount (fun k => x1 (y k)) < invCount (fun k => x (y k)) := by
        rw [hvy]
        exact invCount_adj_swap_lt (fun k => x (y k)) g hg hlt
      rcases ih x1 (by omega) hfx1 with ⟨x', hfx', hobj', hord'⟩
      exact ⟨x', hfx', hobj'.trans hobj1, hord'⟩

private theorem pigeonhole_zero_col {r G : Nat} (hr : r < G)
    (M : Fin r → Fin G → Bool)
    (hAtMostOne : ∀ row : Fin r, ∀ g1 g2 : Fin G, M row g1 = true → M row g2 = true → g1 = g2) :
    ∃ h : Fin G, h.val ≤ r ∧ ∀ row : Fin r, M row h = false := by
  -- Induction on `r`: `r` rows have at most `r` distinct true columns in `{0..r}` of size `r+1`.
  suffices h : ∀ (k : Nat) (hk : k ≤ r) (S : List (Fin G)),
      S.Nodup → S.length = k + 1 → (∀ g ∈ S, g.val ≤ r) →
      ∃ h ∈ S, ∀ row : Fin k, M ⟨row.val, by omega⟩ h = false by
    let S0 : List (Fin G) := (List.finRange (r + 1)).map (fun i => ⟨i.val, by omega⟩)
    have hNodup : S0.Nodup := by
      dsimp [S0]
      have hmap : ∀ {α β : Type} (f : α → β) (l : List α),
          (∀ a b, f a = f b → a = b) → l.Nodup → (l.map f).Nodup := by
        intro α β f l hinj hnd
        induction l with
        | nil => simp
        | cons a rest ih =>
          rw [List.nodup_cons] at hnd
          simp only [List.map_cons, List.nodup_cons]
          refine ⟨?_, ih hnd.2⟩
          intro hmem
          rcases List.mem_map.mp hmem with ⟨b, hbMem, hfab⟩
          exact hnd.1 ((hinj b a hfab) ▸ hbMem)
      exact hmap (fun i : Fin (r + 1) => (⟨i.val, by omega⟩ : Fin G)) (List.finRange (r + 1))
        (fun a b heq => Fin.ext (by simpa using congrArg Fin.val heq)) (List.nodup_finRange (r + 1))
    have hLen : S0.length = r + 1 := by simp [S0]
    have hLe : ∀ g ∈ S0, g.val ≤ r := by
      intro g hg
      rcases List.mem_map.mp hg with ⟨i, _, rfl⟩
      dsimp only
      have := i.isLt
      omega
    rcases h r (Nat.le_refl r) S0 hNodup hLen hLe with ⟨col, hcolMem, hzero⟩
    exact ⟨col, hLe col hcolMem, fun row => hzero row⟩
  intro k
  induction k with
  | zero =>
    intro _ S _ hLen _
    cases S with
    | nil => simp at hLen
    | cons c _ => exact ⟨c, List.mem_cons_self .., fun row => Fin.elim0 row⟩
  | succ k ih =>
    intro hk S hNodup hLen hLe
    let lastRow : Fin r := ⟨k, by omega⟩
    by_cases hex : ∃ c ∈ S, M lastRow c = true
    · rcases hex with ⟨c0, hc0Mem, hc0True⟩
      let S' := S.erase c0
      have hNodup' : S'.Nodup := List.Nodup.erase c0 hNodup
      have hLen' : S'.length = k + 1 := by
        dsimp only [S']
        rw [List.length_erase_of_mem hc0Mem]
        omega
      have hLe' : ∀ g ∈ S', g.val ≤ r := fun g hg => hLe g (List.mem_of_mem_erase hg)
      rcases ih (by omega) S' hNodup' hLen' hLe' with ⟨col, hcolMem', hzeroPrev⟩
      have hcolMem : col ∈ S := List.mem_of_mem_erase hcolMem'
      have hne : col ≠ c0 := by
        intro heq
        subst heq
        exact (List.Nodup.not_mem_erase hNodup) hcolMem'
      refine ⟨col, hcolMem, ?_⟩
      intro row
      by_cases hrk : row.val < k
      · exact hzeroPrev ⟨row.val, hrk⟩
      · have hreq : (⟨row.val, by omega⟩ : Fin r) = lastRow := Fin.ext (by dsimp [lastRow]; omega)
        rw [hreq]
        cases hm : M lastRow col
        · rfl
        · exact absurd (hAtMostOne lastRow col c0 hm hc0True) hne
    · cases S with
      | nil => simp at hLen
      | cons c rest =>
        have hNodup' : rest.Nodup := (List.nodup_cons.mp hNodup).2
        have hLen' : rest.length = k + 1 := by simp at hLen; omega
        have hLe' : ∀ g ∈ rest, g.val ≤ r := fun g hg => hLe g (List.mem_cons_of_mem _ hg)
        rcases ih (by omega) rest hNodup' hLen' hLe' with ⟨col, hcolMem', hzeroPrev⟩
        have hcolMem : col ∈ c :: rest := List.mem_cons_of_mem _ hcolMem'
        refine ⟨col, hcolMem, ?_⟩
        intro row
        by_cases hrk : row.val < k
        · exact hzeroPrev ⟨row.val, hrk⟩
        · have hreq : (⟨row.val, by omega⟩ : Fin r) = lastRow := Fin.ext (by dsimp [lastRow]; omega)
          rw [hreq]
          cases hm : M lastRow col
          · rfl
          · exact absurd ⟨col, hcolMem, hm⟩ hex

/-- Soundness of Subcase A2 orbitope upper-triangular zero fixings (`presolve.go:3345-3369`):
under exact-objective `S_G` symmetry and packing/partitioning binary orbits `p_0, ..., p_{K-1}`
(`∑_g x (p_r, g) ≤ 1`), every feasible `x` can be permuted by a finite composition of group
transpositions into a feasible `x'` of equal objective satisfying `x' (p_r, g) = 0` for all
`r < min K (G - 1)` and `g > r` (and if `p_0` is an equality orbit `= 1`, `x' (p_0, 0) = 1`). -/
theorem orbit_orbitope_upper_tri_sound
    {n G K : Nat} {Feas : (Fin n → Rat) → Prop} {c : Fin n → Rat}
    {pCol : Fin K → Fin G → Fin n}
    {swapAct : Fin G → Fin G → Fin n → Fin n}
    (hSwapP : ∀ a b (r : Fin K) (g : Fin G), swapAct a b (pCol r g) = pCol r (swapIdx a b g))
    (hSym : ∀ a b x, Feas x → Feas (fun j => x (swapAct a b j)) ∧ mipObj c (fun j => x (swapAct a b j)) = mipObj c x)
    (hBin : ∀ x, Feas x → ∀ r g, x (pCol r g) = 0 ∨ x (pCol r g) = 1)
    (hPack : ∀ x, Feas x → ∀ r, finSum G (fun g => x (pCol r g)) ≤ 1)
    {x : Fin n → Rat} (hx : Feas x) :
    ∃ x' : Fin n → Rat, Feas x' ∧ mipObj c x' = mipObj c x ∧
      (∀ (r : Fin K) (g : Fin G), r.val < G - 1 → r.val < g.val → x' (pCol r g) = 0) ∧
      (∀ (hK : 0 < K) (hG : 0 < G),
        finSum G (fun g => x' (pCol ⟨0, hK⟩ g)) = 1 → x' (pCol ⟨0, hK⟩ ⟨0, hG⟩) = 1) := by
  have hAtMostOne : ∀ y, Feas y → ∀ (r : Fin K) (g1 g2 : Fin G),
      y (pCol r g1) = 1 → y (pCol r g2) = 1 → g1 = g2 := by
    intro y hfy r g1 g2 h1 h2
    apply Classical.byContradiction
    intro hne
    have hnn : ∀ g : Fin G, 0 ≤ y (pCol r g) := fun g => by
      rcases hBin y hfy r g with h0 | h1'
      · rw [h0]; exact Rat.le_refl
      · rw [h1']; decide
    have hle : (2 : Rat) ≤ finSum G (fun g => y (pCol r g)) := by
      have hterm : ∀ g : Fin G,
          ((if g = g1 then (1 : Rat) else 0) + (if g = g2 then (1 : Rat) else 0)) ≤ y (pCol r g) := by
        intro g
        by_cases hgg1 : g = g1
        · subst hgg1; simp [hne, h1, Rat.add_zero]
        · by_cases hgg2 : g = g2
          · subst hgg2; simp [hgg1, h2, Rat.zero_add]
          · simp [hgg1, hgg2, Rat.add_zero, hnn g]
      have hsum := finSum_le_finSum hterm
      rw [finSum_add, finSum_single, finSum_single] at hsum
      have h11 : (1 : Rat) + 1 = 2 := by rw [← Rat.intCast_one, ← Rat.intCast_add]; rfl
      rwa [h11] at hsum
    have hp := hPack y hfy r
    exact absurd (Rat.le_trans hle hp) (by decide)
  -- Build prefix `m ≤ min K (G - 1)` by induction
  suffices hPref : ∀ (m : Nat) (hm : m ≤ K) (hmG : m ≤ G - 1),
      ∃ x' : Fin n → Rat, Feas x' ∧ mipObj c x' = mipObj c x ∧
        ∀ (r : Fin K) (g : Fin G), r.val < m → r.val < g.val → x' (pCol r g) = 0 by
    rcases hPref (min K (G - 1)) (Nat.min_le_left _ _) (Nat.min_le_right _ _) with ⟨x', hfx', hobj', htri⟩
    refine ⟨x', hfx', hobj', ?_, ?_⟩
    · intro r g hrG hrg
      have hrm : r.val < min K (G - 1) := by have := r.isLt; omega
      exact htri r g hrm hrg
    · intro hK hG hEq1
      by_cases hG1 : G = 1
      · have hsum : finSum G (fun g => x' (pCol ⟨0, hK⟩ g)) = x' (pCol ⟨0, hK⟩ ⟨0, hG⟩) := by
          subst hG1
          simp [finSum, Rat.zero_add]
        rw [← hsum]; exact hEq1
      · have hAllZero : ∀ g : Fin G, g ≠ ⟨0, hG⟩ → x' (pCol ⟨0, hK⟩ g) = 0 := by
          intro g hne
          have hgPos : 0 < g.val := by
            have : g.val ≠ 0 := fun h => hne (Fin.ext h)
            omega
          exact htri ⟨0, hK⟩ g (by dsimp; omega) hgPos
        have hsum : finSum G (fun g => x' (pCol ⟨0, hK⟩ g)) = x' (pCol ⟨0, hK⟩ ⟨0, hG⟩) := by
          have hc : ∀ g : Fin G, x' (pCol ⟨0, hK⟩ g) =
              if g = ⟨0, hG⟩ then x' (pCol ⟨0, hK⟩ ⟨0, hG⟩) else 0 := by
            intro g
            by_cases h0 : g = ⟨0, hG⟩
            · subst h0; simp
            · simp [h0, hAllZero g h0]
          rw [finSum_congr hc, finSum_single]
        rw [← hsum]; exact hEq1
  intro m
  induction m with
  | zero =>
    intro _ _
    exact ⟨x, hx, rfl, fun _ _ hr0 _ => by omega⟩
  | succ m ih =>
    intro hm hmG
    rcases ih (by omega) (by omega) with ⟨x0, hfx0, hobj0, htri0⟩
    let rCur : Fin K := ⟨m, by omega⟩
    by_cases hBad : ∃ g : Fin G, m < g.val ∧ x0 (pCol rCur g) = 1
    · rcases hBad with ⟨g1, hmg1, hg1One⟩
      let M : Fin m → Fin G → Bool := fun row g =>
        decide (x0 (pCol ⟨row.val, by omega⟩ g) = 1)
      have hMUniq : ∀ row : Fin m, ∀ a b : Fin G, M row a = true → M row b = true → a = b := by
        intro row a b ha hb
        exact hAtMostOne x0 hfx0 ⟨row.val, by omega⟩ a b (of_decide_eq_true ha) (of_decide_eq_true hb)
      rcases pigeonhole_zero_col (show m < G by omega) M hMUniq with ⟨h0, hh0Le, hh0Zero⟩
      let x1 : Fin n → Rat := fun j => x0 (swapAct h0 g1 j)
      rcases hSym h0 g1 x0 hfx0 with ⟨hfx1, hobj1⟩
      refine ⟨x1, hfx1, hobj1.trans hobj0, ?_⟩
      intro r g hrLt hrg
      dsimp only [x1]
      rw [hSwapP h0 g1 r g]
      by_cases hrm : r.val < m
      · by_cases hgh0 : g = h0
        · subst hgh0
          rw [swapIdx_left]
          exact htri0 r g1 hrm (by omega)
        · by_cases hgg1 : g = g1
          · subst hgg1
            rw [swapIdx_right]
            have hz := hh0Zero ⟨r.val, hrm⟩
            dsimp only [M] at hz
            rcases hBin x0 hfx0 r h0 with h0' | h1'
            · exact h0'
            · simp [h1'] at hz
          · have hs : swapIdx h0 g1 g = g := by unfold swapIdx; simp [hgh0, hgg1]
            rw [hs]
            exact htri0 r g hrm hrg
      · have hreq : r = rCur := Fin.ext (by dsimp [rCur]; omega)
        rw [hreq]
        have hneG1 : swapIdx h0 g1 g ≠ g1 := by
          intro heq
          have hgh : g = h0 := by
            have := congrArg (swapIdx h0 g1) heq
            rwa [swapIdx_involutive, swapIdx_right] at this
          subst hgh
          omega
        rcases hBin x0 hfx0 rCur (swapIdx h0 g1 g) with h0' | h1'
        · exact h0'
        · exact absurd (hAtMostOne x0 hfx0 rCur _ g1 h1' hg1One) hneG1
    · refine ⟨x0, hfx0, hobj0, ?_⟩
      intro r g hrLt hrg
      by_cases hrm : r.val < m
      · exact htri0 r g hrm hrg
      · have hreq : r = rCur := Fin.ext (by dsimp [rCur]; omega)
        rw [hreq]
        rcases hBin x0 hfx0 rCur g with h0' | h1'
        · exact h0'
        · exact absurd ⟨g, by omega, h1'⟩ hBad

/-- Soundness of Subcase A1 bijective `G × G` designated-subset identity fixing
(`presolve.go:3302-3343`): under exact-objective `S_G` symmetry, if `G` binary orbits
`d_0, ..., d_{G-1}` satisfy `∑_g x (d_r, g) ≤ 1` and `∑_r x (d_r, g) = 1`, then every feasible `x`
can be permuted by an element `π ∈ S_G` into a feasible `x'` of equal objective satisfying
`x' (d_r, g) = 1` if `g = r` and `0` otherwise. -/
theorem orbit_bijective_identity_sound
    {n G : Nat} {Feas : (Fin n → Rat) → Prop} {c : Fin n → Rat}
    {dCol : Fin G → Fin G → Fin n}
    {permAct : (Fin G → Fin G) → Fin n → Fin n}
    (hPermD : ∀ π (r g : Fin G), permAct π (dCol r g) = dCol r (π g))
    (hSym : ∀ (π πInv : Fin G → Fin G), (∀ g, πInv (π g) = g) → (∀ g, π (πInv g) = g) →
      ∀ x, Feas x → Feas (fun j => x (permAct π j)) ∧ mipObj c (fun j => x (permAct π j)) = mipObj c x)
    (hBin : ∀ x, Feas x → ∀ r g, x (dCol r g) = 0 ∨ x (dCol r g) = 1)
    (hRowPack : ∀ x, Feas x → ∀ r, finSum G (fun g => x (dCol r g)) ≤ 1)
    (hColPart : ∀ x, Feas x → ∀ g, finSum G (fun r => x (dCol r g)) = 1)
    {x : Fin n → Rat} (hx : Feas x) :
    ∃ x' : Fin n → Rat, Feas x' ∧ mipObj c x' = mipObj c x ∧
      ∀ (r g : Fin G), x' (dCol r g) = if g = r then 1 else 0 := by
  have hRowUniq : ∀ r g1 g2, x (dCol r g1) = 1 → x (dCol r g2) = 1 → g1 = g2 := by
    intro r g1 g2 h1 h2
    apply Classical.byContradiction
    intro hne
    have hnn : ∀ g, 0 ≤ x (dCol r g) := fun g => by
      rcases hBin x hx r g with h0 | h1'
      · rw [h0]; exact Rat.le_refl
      · rw [h1']; decide
    have hle : (2 : Rat) ≤ finSum G (fun g => x (dCol r g)) := by
      have hterm : ∀ g : Fin G,
          ((if g = g1 then (1 : Rat) else 0) + (if g = g2 then (1 : Rat) else 0)) ≤ x (dCol r g) := by
        intro g
        by_cases hgg1 : g = g1
        · subst hgg1; simp [hne, h1, Rat.add_zero]
        · by_cases hgg2 : g = g2
          · subst hgg2; simp [hgg1, h2, Rat.zero_add]
          · simp [hgg1, hgg2, Rat.add_zero, hnn g]
      have hsum := finSum_le_finSum hterm
      rw [finSum_add, finSum_single, finSum_single] at hsum
      have h11 : (1 : Rat) + 1 = 2 := by rw [← Rat.intCast_one, ← Rat.intCast_add]; rfl
      rwa [h11] at hsum
    exact absurd (Rat.le_trans hle (hRowPack x hx r)) (by decide)
  have hColUniq : ∀ g r1 r2, x (dCol r1 g) = 1 → x (dCol r2 g) = 1 → r1 = r2 := by
    intro g r1 r2 h1 h2
    apply Classical.byContradiction
    intro hne
    have hnn : ∀ r, 0 ≤ x (dCol r g) := fun r => by
      rcases hBin x hx r g with h0 | h1'
      · rw [h0]; exact Rat.le_refl
      · rw [h1']; decide
    have hle : (2 : Rat) ≤ finSum G (fun r => x (dCol r g)) := by
      have hterm : ∀ r : Fin G,
          ((if r = r1 then (1 : Rat) else 0) + (if r = r2 then (1 : Rat) else 0)) ≤ x (dCol r g) := by
        intro r
        by_cases hrr1 : r = r1
        · subst hrr1; simp [hne, h1, Rat.add_zero]
        · by_cases hrr2 : r = r2
          · subst hrr2; simp [hrr1, h2, Rat.zero_add]
          · simp [hrr1, hrr2, Rat.add_zero, hnn r]
      have hsum := finSum_le_finSum hterm
      rw [finSum_add, finSum_single, finSum_single] at hsum
      have h11 : (1 : Rat) + 1 = 2 := by rw [← Rat.intCast_one, ← Rat.intCast_add]; rfl
      rwa [h11] at hsum
    rw [hColPart x hx g] at hle
    exact absurd hle (by decide)
  have hColEx : ∀ g : Fin G, ∃ r : Fin G, x (dCol r g) = 1 := by
    intro g
    apply Classical.byContradiction
    intro hno
    have hall0 : ∀ r : Fin G, x (dCol r g) = 0 := fun r => by
      rcases hBin x hx r g with h0 | h1
      · exact h0
      · exact absurd ⟨r, h1⟩ hno
    have hsum := hColPart x hx g
    rw [finSum_congr hall0, finSum_zero] at hsum
    exact absurd hsum (by decide)
  let πInv : Fin G → Fin G := fun g => Classical.choose (hColEx g)
  have hπInvSpec : ∀ g : Fin G, x (dCol (πInv g) g) = 1 := fun g => Classical.choose_spec (hColEx g)
  have hπInvInj : ∀ g1 g2 : Fin G, πInv g1 = πInv g2 → g1 = g2 := by
    intro g1 g2 heq
    have h1 := hπInvSpec g1
    have h2 := hπInvSpec g2
    rw [heq] at h1
    exact hRowUniq (πInv g2) g1 g2 h1 h2
  -- Since `πInv : Fin G → Fin G` is injective, it is surjective!
  have hπInvSurj : ∀ r : Fin G, ∃ g : Fin G, πInv g = r := by
    intro r
    apply Classical.byContradiction
    intro hno
    -- Sum of `1` over all `g` equals sum over `Fin G` of `if r' = r then 0 else ...`, or via `finSum_inj_eq`:
    have h1 : finSum G (fun g => if πInv g = r then (1 : Rat) else 0) = 0 := by
      have hc : ∀ g, (if πInv g = r then (1 : Rat) else 0) = 0 := fun g => by
        simp [show πInv g ≠ r from fun heq => hno ⟨g, heq⟩]
      rw [finSum_congr hc]
      exact finSum_zero G
    have h2 : finSum G (fun g => if πInv g = r then (1 : Rat) else 0) =
        finSum G (fun k => if k = r then (1 : Rat) else 0) :=
      finSum_inj_eq (g := fun k => if k = r then (1 : Rat) else 0) hπInvInj
    rw [finSum_single] at h2
    rw [h1] at h2
    exact absurd h2 (by decide)
  let π : Fin G → Fin G := fun r => Classical.choose (hπInvSurj r)
  have hπL : ∀ r : Fin G, πInv (π r) = r := fun r => Classical.choose_spec (hπInvSurj r)
  have hπR : ∀ g : Fin G, π (πInv g) = g := fun g => hπInvInj _ _ (hπL (πInv g))
  let x' : Fin n → Rat := fun j => x (permAct π j)
  rcases hSym π πInv hπL hπR x hx with ⟨hfx', hobj'⟩
  refine ⟨x', hfx', hobj', ?_⟩
  intro r g
  dsimp only [x']
  rw [hPermD π r g]
  by_cases hgr : g = r
  · subst hgr
    simp only [↓reduceIte]
    have h := hπInvSpec (π g)
    rwa [hπL g] at h
  · simp only [hgr, ↓reduceIte]
    have hne : π g ≠ π r := fun heq => hgr (by have := congrArg πInv heq; rwa [hπL, hπL] at this)
    have h1 : x (dCol r (π r)) = 1 := by have h := hπInvSpec (π r); rwa [hπL r] at h
    rcases hBin x hx r (π g) with h0 | h1'
    · exact h0
    · exact absurd (hRowUniq r (π g) (π r) h1' h1) hne

end GoblinFormal.Track2A
