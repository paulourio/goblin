/-
GoblinFormal.Track2A: formal specifications and proofs for Track 2A presolve rules:
1. Integer equality GCD infeasibility and truncated divCeil.
2. Row activity bound tightening, presolveCanSkipTerm, and binary coefficient strengthening.
3. Column-orbit symmetry automorphisms and monotone orbit ordering.
-/

import GoblinFormal.Common.Basic

namespace GoblinFormal.Track2A

/-! ### Rat and FinSum algebraic helpers -/

private theorem rat_add_le_add {a b c d : Rat} (hab : a ≤ b) (hcd : c ≤ d) :
    a + c ≤ b + d :=
  Rat.le_trans (Rat.add_le_add_right.mpr hab) (Rat.add_le_add_left.mpr hcd)

private theorem rat_div_le_iff {a b c : Rat} (hc : 0 < c) :
    a / c ≤ b ↔ a ≤ b * c := by
  constructor
  · intro h
    have hm := Rat.mul_le_mul_of_nonneg_right h (Rat.le_of_lt hc)
    rwa [Rat.div_mul_cancel (Rat.ne_of_gt hc)] at hm
  · intro h
    have hm := Rat.mul_le_mul_of_nonneg_right h (Rat.le_of_lt (Rat.inv_pos.mpr hc))
    rw [Rat.mul_assoc, Rat.mul_inv_cancel c (Rat.ne_of_gt hc), Rat.mul_one] at hm
    simpa [Rat.div_def] using hm

private theorem rat_le_div_iff {a b c : Rat} (hc : 0 < c) :
    a ≤ b / c ↔ a * c ≤ b := by
  constructor
  · intro h
    have hm := Rat.mul_le_mul_of_nonneg_right h (Rat.le_of_lt hc)
    rwa [Rat.div_mul_cancel (Rat.ne_of_gt hc)] at hm
  · intro h
    have hm := Rat.mul_le_mul_of_nonneg_right h (Rat.le_of_lt (Rat.inv_pos.mpr hc))
    rw [Rat.mul_assoc, Rat.mul_inv_cancel c (Rat.ne_of_gt hc), Rat.mul_one] at hm
    simpa [Rat.div_def] using hm

private theorem rat_lt_of_lt_of_le {a b c : Rat} (hab : a < b) (hbc : b ≤ c) : a < c :=
  Rat.lt_iff_le_and_not_ge.mpr
    ⟨Rat.le_trans (Rat.le_of_lt hab) hbc, fun hca => (Rat.not_le.mpr hab) (Rat.le_trans hbc hca)⟩

private theorem rat_lt_of_le_of_lt {a b c : Rat} (hab : a ≤ b) (hbc : b < c) : a < c :=
  Rat.lt_iff_le_and_not_ge.mpr
    ⟨Rat.le_trans hab (Rat.le_of_lt hbc), fun hca => (Rat.not_le.mpr hbc) (Rat.le_trans hca hab)⟩

private theorem rat_mul_sub (a b c : Rat) : a * (b - c) = a * b - a * c := by
  rw [Rat.sub_eq_add_neg, Rat.mul_add, Rat.mul_neg, ← Rat.sub_eq_add_neg]

private theorem rat_div_neg_eq_neg_div_pos (a b : Rat) :
    a / (-b) = (-a) / b := by
  rw [Rat.div_def, Rat.div_def, Rat.neg_mul, ← Rat.mul_neg]
  congr 1
  by_cases hb : b = 0
  · subst hb
    simp
  · apply Rat.inv_eq_of_mul_eq_one
    have hnb : -b ≠ 0 := fun h => hb (by
      have := congrArg Neg.neg h
      simpa using this)
    have hmul : (-b) * (-b⁻¹) = b * b⁻¹ := by
      rw [Rat.neg_mul, Rat.mul_neg, Rat.neg_neg]
    rw [hmul, Rat.mul_inv_cancel b hb]

private theorem rat_neg_div_neg (a b : Rat) :
    (-a) / (-b) = a / b := by
  rw [rat_div_neg_eq_neg_div_pos, Rat.neg_neg]

theorem finSum_split_at {n : Nat} (f : Fin n → Rat) (k : Fin n) :
    finSum n f = finSum n (fun j => if j = k then 0 else f j) + f k := by
  have heq : ∀ j : Fin n,
      f j = (if j = k then 0 else f j) + (if j = k then f k else 0) := by
    intro j
    by_cases hj : j = k
    · subst hj
      simp [Rat.zero_add]
    · simp [hj, Rat.add_zero]
  rw [finSum_congr heq, finSum_add, finSum_single]

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

private theorem rat_eq_num_of_den_one {b : Rat} (hden : b.den = 1) : b = (b.num : Rat) := by
  conv => lhs; rw [← Rat.num_divInt_den b, hden]
  rw [Rat.divInt_ofNat, Rat.mkRat_one]

private theorem int_lt_rat_imp_le_floor {b : Rat} {q : Rat} (hden : b.den = 1) (hlt : b < q) :
    b ≤ (q.floor : Rat) := by
  rw [rat_eq_num_of_den_one hden] at hlt ⊢
  exact Rat.intCast_le_intCast.mpr (Rat.le_floor_iff.mpr (Rat.le_of_lt hlt))

private theorem rat_lt_int_imp_ceil_le {b : Rat} {q : Rat} (hden : b.den = 1) (hlt : q < b) :
    (q.ceil : Rat) ≤ b := by
  rw [rat_eq_num_of_den_one hden] at hlt ⊢
  exact Rat.intCast_le_intCast.mpr (Rat.ceil_le_iff.mpr (Rat.le_of_lt hlt))

/-- Soundness of `presolveCanSkipTerm` for upper-row (`rhs - minAct`) tightening.
When `slackLo ≤ rhs - minAct` strictly exceeds `|a_k| * (u_k - l_k)` (with integer boundary
non-fractionality), the implied bound cannot strictly tighten `u_k` (for `a_k > 0`) or `l_k`
(for `a_k < 0`). -/
theorem presolveCanSkipTerm_sound
    {n : Nat} {a l u : Fin n → Rat} {rhs slackLo absA : Rat} {isInt : Fin n → Bool}
    {k : Fin n}
    (hslack_pos : 0 < slackLo)
    (hslack_le : slackLo ≤ rhs - minAct n a l u)
    (habs_nn : 0 ≤ absA)
    (hspan_nn : 0 ≤ u k - l k)
    (hgt : absA * (u k - l k) < slackLo) :
    (0 < a k → absA = a k →
      u k < (rhs - minOther n a l u k) / a k ∧
      ((isInt k = true → (u k).den = 1) →
        u k ≤ roundUpper (isInt k) ((rhs - minOther n a l u k) / a k))) ∧
    (a k < 0 → absA = -a k →
      (rhs - minOther n a l u k) / a k < l k ∧
      ((isInt k = true → (l k).den = 1) →
        roundLower (isInt k) ((rhs - minOther n a l u k) / a k) ≤ l k)) := by
  have _ := hslack_pos
  have _ := habs_nn
  have _ := hspan_nn
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

/-- Under monotone non-decreasing costs `c_i ≤ c_j` on symmetric constraint-invariant columns
with `x_i, x_j ≥ 0`, swapping `x_i < x_j` preserves feasibility and improves or maintains objective. -/
theorem monotone_orbit_order_sound
    {n m : Nat} {P : MipSpec n m} {perm : Fin n → Fin n}
    (hinv : ∀ k : Fin n, perm (perm k) = k)
    {i j : Fin n} (hpij : perm i = j)
    (hfeas_perm : ∀ x : Fin n → Rat, MipFeasible P x → MipFeasible P (fun k => x (perm k)))
    (hc_other : ∀ k : Fin n, k ≠ i → k ≠ j → P.obj (perm k) = P.obj k)
    (hc_mono : P.obj i ≤ P.obj j)
    {x : Fin n → Rat} (hx : MipFeasible P x) (hx_lt : x i < x j)
    (hxi_nn : 0 ≤ x i) (hxj_nn : 0 ≤ x j) :
    MipFeasible P (fun k => x (perm k)) ∧
      x (perm j) ≤ x (perm i) ∧
      mipObj P.obj (fun k => x (perm k)) ≤ mipObj P.obj x := by
  have _ := hxi_nn
  have _ := hxj_nn
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
  let outTerms := terms.map fun t =>
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

private def rowPairsOf (r : OrbitWorkRow) : Array (Nat × Rat) :=
  Array.ofFn (fun k : Fin (min r.cols.size r.vals.size) =>
    (r.cols[k.1]'(Nat.lt_of_lt_of_le k.isLt (Nat.min_le_left _ _)),
     r.vals[k.1]'(Nat.lt_of_lt_of_le k.isLt (Nat.min_le_right _ _))))

private def insertSortedPair (xs : Array (Nat × Rat)) (p : Nat × Rat) :
    Option (Array (Nat × Rat)) :=
  -- Reject duplicate column indices in a row while inserting in ascending `col` order.
  let rec go (i : Nat) (acc : Array (Nat × Rat)) : Option (Array (Nat × Rat)) :=
    if h : i < xs.size then
      let cur := xs[i]
      if p.1 == cur.1 then
        none
      else if p.1 < cur.1 then
        let acc := acc.push p
        let rec tail (j : Nat) (a : Array (Nat × Rat)) : Array (Nat × Rat) :=
          if hj : j < xs.size then tail (j + 1) (a.push xs[j]) else a
        some (tail i acc)
      else
        go (i + 1) (acc.push cur)
    else
      some (acc.push p)
  termination_by xs.size - i
  go 0 #[]

private def sortUniqueRowPairs (pairs : Array (Nat × Rat)) : Option (Array (Nat × Rat)) :=
  pairs.foldlM (fun acc p => insertSortedPair acc p) #[]

/-- Executable symmetry verification matching `verifyOrbitSymmetry` in `presolve.go`. -/
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
          if hcg : cg < numVars then
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
      -- 2. Verify row tuples map identically under group actions `g = 0 .. gCount - 1`.
      let tuplesOk := (List.range numTuples).all fun tIdx =>
        let tBase := tIdx * gCount
        let r0Idx := rowTuples[tBase]!
        if hr0 : r0Idx < rows.size then
          let r0 := rows[r0Idx]
          if !r0.active || rowState[r0Idx]! != 0 || r0.cols.size != r0.vals.size then
            false
          else
            let r0Pairs : Array (Nat × Rat) := rowPairsOf r0
            match sortUniqueRowPairs r0Pairs with
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
                      let rgPairs : Array (Nat × Rat) := rowPairsOf rg
                      match sortUniqueRowPairs mapped0, sortUniqueRowPairs rgPairs with
                      | some mSorted, some gSorted => mSorted == gSorted
                      | _, _ => false
                  else
                    false
        else
          false
      -- 3. Verify active non-tuple rows (`rowState < 0`) are invariant on every orbit they intersect.
      let invOk := (List.range rows.size).all fun i =>
        let r : OrbitWorkRow := rows[i]!
        if !r.active || rowState[i]! ≥ 0 then
          true
        else if r.cols.size != r.vals.size then
          false
        else
          let rPairs : Array (Nat × Rat) := rowPairsOf r
          match sortUniqueRowPairs rPairs with
          | none => false
          | some sortedR =>
            if !(sortedR.all fun (c, _) => c < numVars) then
              false
            else
              (List.range numOrbits).all fun orbID =>
                let base := orbID * gCount
                let c0 := orbits[base]!
                match sortedR.find? (fun p => p.1 == c0) with
                | none =>
                  -- If `c0` is absent, NO member of `orbID` may appear in this row.
                  (List.range gCount).all fun g =>
                    let cg := orbits[base + g]!
                    !(sortedR.any (fun p => p.1 == cg))
                | some (_, v0) =>
                  -- If `c0` is present with `v0`, ALL `gCount` members must appear with `v0`.
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

/-- Soundness of `presolveCanSkipTerm` for lower-row (`maxAct - rhs`) tightening. -/
theorem presolveCanSkipTerm_lower_sound
    {n : Nat} {a l u : Fin n → Rat} {rhs slackLo absA : Rat} {isInt : Fin n → Bool}
    {k : Fin n}
    (hslack_pos : 0 < slackLo)
    (hslack_le : slackLo ≤ maxAct n a l u - rhs)
    (habs_nn : 0 ≤ absA)
    (hspan_nn : 0 ≤ u k - l k)
    (hgt : absA * (u k - l k) < slackLo) :
    (0 < a k → absA = a k →
      (rhs - maxOther n a l u k) / a k < l k ∧
      ((isInt k = true → (l k).den = 1) →
        roundLower (isInt k) ((rhs - maxOther n a l u k) / a k) ≤ l k)) ∧
    (a k < 0 → absA = -a k →
      u k < (rhs - maxOther n a l u k) / a k ∧
      ((isInt k = true → (u k).den = 1) →
        u k ≤ roundUpper (isInt k) ((rhs - maxOther n a l u k) / a k))) := by
  have _ := hslack_pos
  have _ := habs_nn
  have _ := hspan_nn
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

end GoblinFormal.Track2A
