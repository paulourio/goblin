/-
GoblinFormal.Track2B: Cutting-plane soundness for GMI, row aggregation,
single-row cMIR (including complementation and outward safe rounding),
and variable-upper-bound (VUB) disaggregation.
-/

import GoblinFormal.Common.Basic
import GoblinFormal.Common.Json

namespace GoblinFormal.Track2B

/-! ### Track 2B-specific Rat and finSum helpers -/

private theorem rat_zero_div (a : Rat) : 0 / a = 0 := by
  rw [Rat.div_def, Rat.zero_mul]

private theorem rat_neg_div (a b : Rat) : (-a) / b = -(a / b) := by
  rw [Rat.div_def, Rat.div_def, Rat.neg_mul]

private theorem rat_mul_right_comm (a b c : Rat) : a * b * c = a * c * b := by
  rw [Rat.mul_assoc, Rat.mul_comm b c, ← Rat.mul_assoc]

private theorem rat_add_sub_assoc (a b c : Rat) : a + (b - c) = a + b - c := by
  rw [Rat.sub_eq_add_neg, Rat.sub_eq_add_neg, Rat.add_assoc]

private theorem rat_sub_sub_cancel (a b : Rat) : a - (a - b) = b := by
  rw [Rat.sub_eq_add_neg, Rat.sub_eq_add_neg, Rat.neg_add, Rat.neg_neg, ← Rat.add_assoc,
    Rat.add_neg_cancel, Rat.zero_add]

private theorem rat_eq_add_sub (a b : Rat) : b = a + (b - a) := by
  rw [Rat.add_comm, Rat.sub_add_cancel]

private theorem rat_neg_eq_neg_add_sub (a b : Rat) : -b = -a + (a - b) := by
  rw [Rat.sub_eq_add_neg, ← Rat.add_assoc, Rat.neg_add_cancel, Rat.zero_add]

private theorem rat_neg_eq_neg_sub_sub (a b : Rat) : -b = -a - (b - a) := by
  rw [Rat.sub_eq_add_neg, Rat.sub_eq_add_neg, Rat.neg_add, Rat.neg_neg, Rat.add_comm (-b) a,
    ← Rat.add_assoc, Rat.neg_add_cancel, Rat.zero_add]

private theorem rat_div_nonneg {a b : Rat} (ha : 0 ≤ a) (hb : 0 ≤ b) : 0 ≤ a / b := by
  rw [Rat.div_def]
  by_cases hb0 : b = 0
  · subst hb0
    simp
  · have hbpos : 0 < b := Rat.lt_of_le_of_ne hb (Ne.symm hb0)
    exact Rat.mul_nonneg ha (Rat.le_of_lt (Rat.inv_pos.mpr hbpos))

private theorem rat_add_div (a b c : Rat) : (a + b) / c = a / c + b / c := by
  simp [Rat.div_def, Rat.add_mul]

private theorem rat_sub_div (a b c : Rat) : (a - b) / c = a / c - b / c := by
  simp [Rat.div_def, Rat.sub_eq_add_neg, Rat.add_mul, Rat.neg_mul]

private theorem rat_div_self {a : Rat} (ha : a ≠ 0) : a / a = 1 := by
  rw [Rat.div_def, Rat.mul_inv_cancel _ ha]

private theorem finSum_div (n : Nat) (f : Fin n → Rat) (c : Rat) :
    finSum n f / c = finSum n (fun i => f i / c) := by
  simp only [Rat.div_def, ← finSum_mul_const]

/-- Fractional part `a - floor(a) ∈ [0, 1)`. -/
def fracPart (a : Rat) : Rat :=
  a - (a.floor : Rat)

theorem fracPart_nonneg (a : Rat) : 0 ≤ fracPart a :=
  (Rat.le_iff_sub_nonneg (a.floor : Rat) a).mp (Rat.floor_le a)

theorem fracPart_lt_one (a : Rat) : fracPart a < 1 := by
  have h : a < ((a.floor + 1 : Int) : Rat) := Rat.lt_floor_add_one a
  rw [Rat.intCast_add, Rat.intCast_one] at h
  have h1 := Rat.add_lt_add_right (c := -(a.floor : Rat)) |>.mpr h
  rw [← Rat.sub_eq_add_neg, Rat.add_comm (a.floor : Rat) 1, Rat.add_assoc,
    ← Rat.sub_eq_add_neg, Rat.sub_self, Rat.add_zero] at h1
  exact h1

theorem one_sub_fracPart_pos (a : Rat) : 0 < 1 - fracPart a :=
  (Rat.lt_iff_sub_pos (fracPart a) 1).mp (fracPart_lt_one a)

theorem fracPart_intCast (z : Int) : fracPart (z : Rat) = 0 := by
  simp [fracPart, Rat.floor_intCast, Rat.sub_self]

/-! ### 1. Gomory Mixed-Integer (GMI) Cut Soundness -/

/-- Exact GMI coefficient for a non-basic non-negative integer variable. -/
def gmiIntCoeff (a f0 : Rat) : Rat :=
  let fk := fracPart a
  if fk ≤ f0 then fk / f0 else (1 - fk) / (1 - f0)

/-- Exact GMI coefficient for a non-basic non-negative continuous variable. -/
def gmiContCoeff (g f0 : Rat) : Rat :=
  if 0 ≤ g then g / f0 else (-g) / (1 - f0)

private def posPart (x : Rat) : Rat := max x 0
private def negPart (x : Rat) : Rat := max (-x) 0

private theorem posPart_nonneg (x : Rat) : 0 ≤ posPart x := rat_le_max_right
private theorem negPart_nonneg (x : Rat) : 0 ≤ negPart x := rat_le_max_right

private theorem posPart_eq_self {x : Rat} (hx : 0 ≤ x) : posPart x = x := by
  unfold posPart
  rw [Rat.max_def]
  split
  · next h => exact Rat.le_antisymm hx h
  · rfl

private theorem posPart_eq_zero {x : Rat} (hx : x ≤ 0) : posPart x = 0 := by
  unfold posPart
  rw [Rat.max_def]
  split
  · rfl
  · next h => exact absurd hx h

private theorem negPart_eq_neg {x : Rat} (hx : x ≤ 0) : negPart x = -x :=
  posPart_eq_self (rat_neg_nonneg hx)

private theorem negPart_eq_zero {x : Rat} (hx : 0 ≤ x) : negPart x = 0 :=
  posPart_eq_zero (by simpa using Rat.neg_le_neg hx)

private theorem posPart_sub_negPart (x : Rat) : posPart x - negPart x = x := by
  rcases Rat.le_total (a := 0) (b := x) with hx | hx
  · rw [posPart_eq_self hx, negPart_eq_zero hx, rat_sub_zero]
  · rw [posPart_eq_zero hx, negPart_eq_neg hx, rat_zero_sub, Rat.neg_neg]

private def gmiIntRound (a f0 : Rat) : Int :=
  if fracPart a ≤ f0 then a.floor else a.floor + 1

private def gmiIntRem (a f0 : Rat) : Rat :=
  a - (gmiIntRound a f0 : Rat)

private theorem gmiIntCoeff_eq_split (a f0 : Rat) :
    gmiIntCoeff a f0 =
      posPart (gmiIntRem a f0) / f0 + negPart (gmiIntRem a f0) / (1 - f0) := by
  unfold gmiIntCoeff gmiIntRem gmiIntRound
  by_cases h : fracPart a ≤ f0
  · simp only [h, ↓reduceIte]
    have hrem : a - (a.floor : Rat) = fracPart a := rfl
    rw [hrem, posPart_eq_self (fracPart_nonneg a), negPart_eq_zero (fracPart_nonneg a),
      rat_zero_div, Rat.add_zero]
  · simp only [h, ↓reduceIte]
    have hrem : a - ((a.floor + 1 : Int) : Rat) = fracPart a - 1 := by
      rw [Rat.intCast_add, Rat.intCast_one, fracPart]
      simp only [Rat.sub_eq_add_neg, Rat.neg_add, Rat.add_assoc]
    have hlt : fracPart a - 1 < 0 := by
      have h1 := fracPart_lt_one a
      have h2 := (Rat.add_lt_add_right (c := (-1 : Rat))).mpr h1
      rw [← Rat.sub_eq_add_neg, Rat.add_neg_cancel] at h2
      exact h2
    have hle : fracPart a - 1 ≤ 0 := Rat.le_of_lt hlt
    rw [hrem, posPart_eq_zero hle, negPart_eq_neg hle, Rat.neg_sub, rat_zero_div, Rat.zero_add]

private theorem gmiContCoeff_eq_split (g f0 : Rat) :
    gmiContCoeff g f0 = posPart g / f0 + negPart g / (1 - f0) := by
  unfold gmiContCoeff
  by_cases h : 0 ≤ g
  · simp only [h, ↓reduceIte]
    rw [posPart_eq_self h, negPart_eq_zero h, rat_zero_div, Rat.add_zero]
  · simp only [h, ↓reduceIte]
    have hle : g ≤ 0 := Rat.le_of_lt (Rat.not_le.mp h)
    rw [posPart_eq_zero hle, negPart_eq_neg hle, rat_zero_div, Rat.zero_add]

/-- Soundness of the Gomory Mixed-Integer (GMI) cut over `Rat`. -/
theorem gmi_cut_sound {mI mC : Nat}
    {aI : Fin mI → Rat} {xI : Fin mI → Rat}
    {aC : Fin mC → Rat} {xC : Fin mC → Rat}
    {xB beta f0 : Rat}
    (hrow : xB + finSum mI (fun j => aI j * xI j) + finSum mC (fun k => aC k * xC k) = beta)
    (hxB : ∃ z : Int, xB = (z : Rat))
    (hxI : ∀ j, 0 ≤ xI j ∧ ∃ z : Int, xI j = (z : Rat))
    (hxC : ∀ k, 0 ≤ xC k)
    (hf0 : f0 = fracPart beta)
    (hf0_pos : 0 < f0)
    (hf0_lt : f0 < 1) :
    1 ≤ finSum mI (fun j => gmiIntCoeff (aI j) f0 * xI j) +
        finSum mC (fun k => gmiContCoeff (aC k) f0 * xC k) := by
  rcases hxB with ⟨zB, hzB⟩
  let zI : Fin mI → Int := fun j => Classical.choose (hxI j).2
  have hzI : ∀ j, xI j = (zI j : Rat) := fun j => Classical.choose_spec (hxI j).2
  let K : Int := zB + intFinSum mI (fun j => gmiIntRound (aI j) f0 * zI j) - beta.floor
  let Spos : Rat :=
    finSum mI (fun j => posPart (gmiIntRem (aI j) f0) * xI j) +
    finSum mC (fun k => posPart (aC k) * xC k)
  let Sneg : Rat :=
    finSum mI (fun j => negPart (gmiIntRem (aI j) f0) * xI j) +
    finSum mC (fun k => negPart (aC k) * xC k)
  have hSpos_nonneg : 0 ≤ Spos :=
    Rat.add_nonneg
      (finSum_nonneg fun j => Rat.mul_nonneg (posPart_nonneg _) (hxI j).1)
      (finSum_nonneg fun k => Rat.mul_nonneg (posPart_nonneg _) (hxC k))
  have hSneg_nonneg : 0 ≤ Sneg :=
    Rat.add_nonneg
      (finSum_nonneg fun j => Rat.mul_nonneg (negPart_nonneg _) (hxI j).1)
      (finSum_nonneg fun k => Rat.mul_nonneg (negPart_nonneg _) (hxC k))
  have h1mf0_pos : 0 < 1 - f0 := (Rat.lt_iff_sub_pos f0 1).mp hf0_lt
  have hK_eq : (K : Rat) + Spos - Sneg = f0 := by
    have hsplitI :
        finSum mI (fun j => aI j * xI j) =
          (intFinSum mI (fun j => gmiIntRound (aI j) f0 * zI j) : Rat) +
            finSum mI (fun j => posPart (gmiIntRem (aI j) f0) * xI j) -
            finSum mI (fun j => negPart (gmiIntRem (aI j) f0) * xI j) := by
      have haI : ∀ j, aI j * xI j =
          ((gmiIntRound (aI j) f0 * zI j : Int) : Rat) +
            (posPart (gmiIntRem (aI j) f0) * xI j - negPart (gmiIntRem (aI j) f0) * xI j) := by
        intro j
        rw [Rat.intCast_mul, ← hzI j, ← rat_sub_mul, posPart_sub_negPart, gmiIntRem,
          ← Rat.add_mul, Rat.add_comm (gmiIntRound (aI j) f0 : Rat), Rat.sub_add_cancel]
      rw [finSum_congr haI, finSum_add, finSum_sub, intFinSum_eq_finSum, rat_add_sub_assoc]
    have hsplitC :
        finSum mC (fun k => aC k * xC k) =
          finSum mC (fun k => posPart (aC k) * xC k) -
          finSum mC (fun k => negPart (aC k) * xC k) := by
      have haC : ∀ k, aC k * xC k =
          posPart (aC k) * xC k - negPart (aC k) * xC k := by
        intro k
        rw [← rat_sub_mul, posPart_sub_negPart]
      rw [finSum_congr haC, finSum_sub]
    have hbeta : beta = (beta.floor : Rat) + f0 := by
      rw [hf0, fracPart, Rat.add_comm (beta.floor : Rat), Rat.sub_add_cancel]
    have hK_cast :
        (K : Rat) =
          (zB : Rat) + (intFinSum mI (fun j => gmiIntRound (aI j) f0 * zI j) : Rat) -
            (beta.floor : Rat) := by
      dsimp [K]
      rw [Rat.intCast_sub, Rat.intCast_add]
    have hr := hrow
    rw [hzB, hsplitI, hsplitC, hbeta] at hr
    have hsum :
        (K : Rat) + Spos - Sneg + (beta.floor : Rat) =
          ((zB : Rat) +
            ((intFinSum mI (fun j => gmiIntRound (aI j) f0 * zI j) : Rat) +
              finSum mI (fun j => posPart (gmiIntRem (aI j) f0) * xI j) -
              finSum mI (fun j => negPart (gmiIntRem (aI j) f0) * xI j)) +
            (finSum mC (fun k => posPart (aC k) * xC k) -
             finSum mC (fun k => negPart (aC k) * xC k))) := by
      dsimp [Spos, Sneg]
      rw [hK_cast]
      have hcancel :
          ((zB : Rat) + (intFinSum mI (fun j => gmiIntRound (aI j) f0 * zI j) : Rat) -
              (beta.floor : Rat)) +
            (finSum mI (fun j => posPart (gmiIntRem (aI j) f0) * xI j) +
             finSum mC (fun k => posPart (aC k) * xC k)) -
            (finSum mI (fun j => negPart (gmiIntRem (aI j) f0) * xI j) +
             finSum mC (fun k => negPart (aC k) * xC k)) +
            (beta.floor : Rat) =
          (-(beta.floor : Rat) + (beta.floor : Rat)) +
            ((zB : Rat) +
              ((intFinSum mI (fun j => gmiIntRound (aI j) f0 * zI j) : Rat) +
                finSum mI (fun j => posPart (gmiIntRem (aI j) f0) * xI j) -
                finSum mI (fun j => negPart (gmiIntRem (aI j) f0) * xI j)) +
              (finSum mC (fun k => posPart (aC k) * xC k) -
               finSum mC (fun k => negPart (aC k) * xC k))) := by
        simp only [Rat.sub_eq_add_neg, Rat.neg_add]
        ac_rfl
      rw [hcancel, Rat.neg_add_cancel, Rat.zero_add]
    have hr' := hsum.trans hr
    have hr'' := congrArg (· - (beta.floor : Rat)) hr'
    rw [Rat.add_sub_cancel, Rat.add_comm (beta.floor : Rat) f0, Rat.add_sub_cancel] at hr''
    exact hr''
  have hrhs_eq :
      finSum mI (fun j => gmiIntCoeff (aI j) f0 * xI j) +
        finSum mC (fun k => gmiContCoeff (aC k) f0 * xC k) =
      Spos / f0 + Sneg / (1 - f0) := by
    have hI :
        finSum mI (fun j => gmiIntCoeff (aI j) f0 * xI j) =
          finSum mI (fun j => posPart (gmiIntRem (aI j) f0) * xI j) / f0 +
          finSum mI (fun j => negPart (gmiIntRem (aI j) f0) * xI j) / (1 - f0) := by
      have hterm : ∀ j, gmiIntCoeff (aI j) f0 * xI j =
          (posPart (gmiIntRem (aI j) f0) * xI j) / f0 +
          (negPart (gmiIntRem (aI j) f0) * xI j) / (1 - f0) := by
        intro j
        rw [gmiIntCoeff_eq_split, Rat.add_mul, Rat.div_def, Rat.div_def, Rat.div_def, Rat.div_def,
          rat_mul_right_comm _ f0⁻¹ (xI j), rat_mul_right_comm _ (1 - f0)⁻¹ (xI j)]
      rw [finSum_congr hterm, finSum_add, finSum_div, finSum_div]
    have hC :
        finSum mC (fun k => gmiContCoeff (aC k) f0 * xC k) =
          finSum mC (fun k => posPart (aC k) * xC k) / f0 +
          finSum mC (fun k => negPart (aC k) * xC k) / (1 - f0) := by
      have hterm : ∀ k, gmiContCoeff (aC k) f0 * xC k =
          (posPart (aC k) * xC k) / f0 + (negPart (aC k) * xC k) / (1 - f0) := by
        intro k
        rw [gmiContCoeff_eq_split, Rat.add_mul, Rat.div_def, Rat.div_def, Rat.div_def, Rat.div_def,
          rat_mul_right_comm _ f0⁻¹ (xC k), rat_mul_right_comm _ (1 - f0)⁻¹ (xC k)]
      rw [finSum_congr hterm, finSum_add, finSum_div, finSum_div]
    rw [hI, hC]
    dsimp [Spos, Sneg]
    rw [rat_add_div, rat_add_div, Rat.add_assoc, Rat.add_left_comm
      (finSum mI (fun j => negPart (gmiIntRem (aI j) f0) * xI j) / (1 - f0)),
      Rat.add_assoc]
  rw [hrhs_eq]
  have hK_cases : K ≤ 0 ∨ 1 ≤ K := by omega
  have hS_sub : Spos - Sneg = f0 - (K : Rat) := by
    symm
    rw [← hK_eq]
    have hre : ((K : Rat) + Spos - Sneg) - (K : Rat) = (-(K : Rat) + (K : Rat)) + (Spos - Sneg) := by
      simp only [Rat.sub_eq_add_neg]
      ac_rfl
    rw [hre, Rat.neg_add_cancel, Rat.zero_add]
  rcases hK_cases with hK0 | hK1
  · have hK_rat : (K : Rat) ≤ 0 := Rat.intCast_nonpos.mpr hK0
    have hSpos_ge : f0 ≤ Spos := by
      have h3 : f0 ≤ Spos - Sneg := by
        rw [hS_sub]
        have hnk : 0 ≤ -(K : Rat) := rat_neg_nonneg hK_rat
        have h1 := (Rat.add_le_add_left (c := f0)).mpr hnk
        rwa [Rat.add_zero, ← Rat.sub_eq_add_neg] at h1
      have h4 : Spos - Sneg ≤ Spos := by
        have hns := Rat.neg_le_neg hSneg_nonneg
        have h1 := (Rat.add_le_add_left (c := Spos)).mpr hns
        rwa [Rat.neg_zero, Rat.add_zero, ← Rat.sub_eq_add_neg] at h1
      exact Rat.le_trans h3 h4
    have hdiv1 : 1 ≤ Spos / f0 := by
      conv => lhs; rw [← rat_div_self (Rat.ne_of_gt hf0_pos)]
      exact rat_div_le_div_of_pos hSpos_ge hf0_pos
    have hdiv2 : 0 ≤ Sneg / (1 - f0) := rat_div_nonneg hSneg_nonneg (Rat.le_of_lt h1mf0_pos)
    simpa [Rat.add_zero] using rat_add_le_add hdiv1 hdiv2
  · have hK_rat : (1 : Rat) ≤ (K : Rat) := by simpa using (Rat.intCast_le_intCast.mpr hK1)
    have hSneg_ge : 1 - f0 ≤ Sneg := by
      have h2 : Sneg - Spos = (K : Rat) - f0 := by
        have h'' := congrArg (-·) hS_sub
        simpa [Rat.neg_sub] using h''
      have h3 : 1 - f0 ≤ Sneg - Spos := by
        rw [h2]
        exact rat_sub_le_sub hK_rat Rat.le_refl
      have h4 : Sneg - Spos ≤ Sneg := by
        have hns := Rat.neg_le_neg hSpos_nonneg
        have h1 := (Rat.add_le_add_left (c := Sneg)).mpr hns
        rwa [Rat.neg_zero, Rat.add_zero, ← Rat.sub_eq_add_neg] at h1
      exact Rat.le_trans h3 h4
    have hdiv1 : 0 ≤ Spos / f0 := rat_div_nonneg hSpos_nonneg (Rat.le_of_lt hf0_pos)
    have hdiv2 : 1 ≤ Sneg / (1 - f0) := by
      conv => lhs; rw [← rat_div_self (Rat.ne_of_gt h1mf0_pos)]
      exact rat_div_le_div_of_pos hSneg_ge h1mf0_pos
    simpa [Rat.zero_add] using rat_add_le_add hdiv1 hdiv2

/-- Upward coefficient weakening (modeling `nextUp` in `separateGMICutsEx`) preserves GMI soundness. -/
theorem gmi_weaken_upward_sound {mI mC : Nat}
    {aI cI : Fin mI → Rat} {xI : Fin mI → Rat}
    {aC cC : Fin mC → Rat} {xC : Fin mC → Rat}
    {xB beta f0 : Rat}
    (hrow : xB + finSum mI (fun j => aI j * xI j) + finSum mC (fun k => aC k * xC k) = beta)
    (hxB : ∃ z : Int, xB = (z : Rat))
    (hxI : ∀ j, 0 ≤ xI j ∧ ∃ z : Int, xI j = (z : Rat))
    (hxC : ∀ k, 0 ≤ xC k)
    (hf0 : f0 = fracPart beta)
    (hf0_pos : 0 < f0)
    (hf0_lt : f0 < 1)
    (hcI : ∀ j, gmiIntCoeff (aI j) f0 ≤ cI j)
    (hcC : ∀ k, gmiContCoeff (aC k) f0 ≤ cC k) :
    1 ≤ finSum mI (fun j => cI j * xI j) + finSum mC (fun k => cC k * xC k) := by
  have hbase := gmi_cut_sound hrow hxB hxI hxC hf0 hf0_pos hf0_lt
  have hI : finSum mI (fun j => gmiIntCoeff (aI j) f0 * xI j) ≤ finSum mI (fun j => cI j * xI j) :=
    finSum_le_finSum fun j => Rat.mul_le_mul_of_nonneg_right (hcI j) (hxI j).1
  have hC : finSum mC (fun k => gmiContCoeff (aC k) f0 * xC k) ≤ finSum mC (fun k => cC k * xC k) :=
    finSum_le_finSum fun k => Rat.mul_le_mul_of_nonneg_right (hcC k) (hxC k)
  exact Rat.le_trans hbase (rat_add_le_add hI hC)

/-! ### 2. Row Aggregation Multiplier Sign Legality & `mirUseLower` -/

/-- Linear constraint sense matching `ConstraintSense` in Go. -/
inductive ConstraintSense where
  | le
  | eq
  | ge
  deriving DecidableEq, Repr, Inhabited

/-- Satisfaction of a row `lhs <sense> rhs`. -/
def satisfiesSense (lhs : Rat) (sense : ConstraintSense) (rhs : Rat) : Prop :=
  match sense with
  | .le => lhs ≤ rhs
  | .eq => lhs = rhs
  | .ge => rhs ≤ lhs

/-- Executable multiplier sign legality specification matching `cuts.go:rowLegalMuSign`. -/
def rowLegalMuSignSpec (sense : ConstraintSense) (mu : Rat) : Bool :=
  match sense with
  | .eq => mu != 0
  | .le => 0 < mu
  | .ge => mu < 0

/-- Executable bound-selection rule matching `cuts.go:mirUseLower`. -/
def mirUseLowerSpec (x l u : Rat) (hasL hasU : Bool) : Bool :=
  if !hasU then true
  else if !hasL then false
  else decide (x - l ≤ u - x)

/-- Scaling a satisfied row by a legal multiplier yields a valid `≤` inequality. -/
theorem row_agg_legal_sound {n : Nat} {a x : Fin n → Rat}
    {sense : ConstraintSense} {rhs mu : Rat}
    (hmu : rowLegalMuSignSpec sense mu = true)
    (hsat : satisfiesSense (finSum n (fun j => a j * x j)) sense rhs) :
    finSum n (fun j => (mu * a j) * x j) ≤ mu * rhs := by
  have hmul_sum :
      finSum n (fun j => (mu * a j) * x j) = mu * finSum n (fun j => a j * x j) := by
    rw [← finSum_const_mul]
    exact finSum_congr fun j => Rat.mul_assoc mu (a j) (x j)
  rw [hmul_sum]
  cases sense with
  | eq =>
    dsimp [satisfiesSense] at hsat
    rw [hsat]
    exact Rat.le_refl
  | le =>
    dsimp [rowLegalMuSignSpec] at hmu
    dsimp [satisfiesSense] at hsat
    have h0 : 0 ≤ mu := Rat.le_of_lt (of_decide_eq_true hmu)
    exact Rat.mul_le_mul_of_nonneg_left hsat h0
  | ge =>
    dsimp [rowLegalMuSignSpec] at hmu
    dsimp [satisfiesSense] at hsat
    have h0 : mu ≤ 0 := Rat.le_of_lt (of_decide_eq_true hmu)
    exact rat_mul_le_mul_of_nonpos_left hsat h0

/-- Summing multiple valid rows scaled by legal (or zero) multipliers preserves feasibility. -/
theorem row_agg_sum_sound {m n : Nat}
    {A : Fin m → Fin n → Rat} {sense : Fin m → ConstraintSense}
    {rhs mu : Fin m → Rat} {x : Fin n → Rat}
    (hmu : ∀ i, mu i = 0 ∨ rowLegalMuSignSpec (sense i) (mu i) = true)
    (hsat : ∀ i, satisfiesSense (finSum n (fun j => A i j * x j)) (sense i) (rhs i)) :
    finSum n (fun j => finSum m (fun i => mu i * A i j) * x j) ≤
      finSum m (fun i => mu i * rhs i) := by
  have hrow : ∀ i : Fin m, finSum n (fun j => (mu i * A i j) * x j) ≤ mu i * rhs i := by
    intro i
    rcases hmu i with h0 | hleg
    · rw [h0]
      have hz : finSum n (fun j => (0 * A i j) * x j) = 0 := by
        have h : finSum n (fun j => (0 * A i j) * x j) = finSum n (fun _ => (0 : Rat)) :=
          finSum_congr fun j => by simp
        rw [h, finSum_zero]
      rw [hz, Rat.zero_mul]
      exact Rat.le_refl
    · exact row_agg_legal_sound hleg (hsat i)
  have hsum := finSum_le_finSum hrow
  have hswap :
      finSum m (fun i => finSum n (fun j => (mu i * A i j) * x j)) =
        finSum n (fun j => finSum m (fun i => mu i * A i j) * x j) := by
    rw [finSum_swap]
    exact finSum_congr fun j => by rw [finSum_mul_const]
  rwa [hswap] at hsum

/-- Soundness of error-bounded row materialization (`cuts.go:materializeAggRow`). -/
theorem row_agg_error_bound_sound {n : Nat}
    {cv exactCoeff lb ub ce maxB x : Fin n → Rat}
    {exactRHS rhsErr aggRHS : Rat}
    (hexact : finSum n (fun j => exactCoeff j * x j) ≤ exactRHS)
    (hxbnd : ∀ j, lb j ≤ x j ∧ x j ≤ ub j)
    (hce : ∀ j, (cv j - exactCoeff j).abs ≤ ce j)
    (hmaxB : ∀ j, max (lb j).abs (ub j).abs ≤ maxB j)
    (hrhsErr : 0 ≤ rhsErr)
    (hrhs : exactRHS + rhsErr + finSum n (fun j => ce j * maxB j) ≤ aggRHS) :
    finSum n (fun j => cv j * x j) ≤ aggRHS := by
  have hxabs : ∀ j, (x j).abs ≤ maxB j := by
    intro j
    have h1 : (x j).abs ≤ max (lb j).abs (ub j).abs := by
      rw [rat_abs_le_iff]
      have hlb : -max (lb j).abs (ub j).abs ≤ lb j := by
        have h := Rat.neg_le_neg (Rat.le_trans (rat_le_abs (-lb j))
          (by simpa [Rat.abs_neg] using rat_le_max_left (a := (lb j).abs) (b := (ub j).abs)))
        simpa [Rat.neg_neg] using h
      have hub : ub j ≤ max (lb j).abs (ub j).abs :=
        Rat.le_trans (rat_le_abs (ub j)) rat_le_max_right
      exact ⟨Rat.le_trans hlb (hxbnd j).1, Rat.le_trans (hxbnd j).2 hub⟩
    exact Rat.le_trans h1 (hmaxB j)
  have hterm : ∀ j, cv j * x j ≤ exactCoeff j * x j + ce j * maxB j := by
    intro j
    have hdiff : (cv j - exactCoeff j) * x j ≤ ((cv j - exactCoeff j) * x j).abs := rat_le_abs _
    rw [rat_abs_mul] at hdiff
    have hce_nn : 0 ≤ ce j := Rat.le_trans Rat.abs_nonneg (hce j)
    have hmul : (cv j - exactCoeff j).abs * (x j).abs ≤ ce j * maxB j := by
      have h1 := Rat.mul_le_mul_of_nonneg_right (hce j) (Rat.abs_nonneg (x := x j))
      have h2 := Rat.mul_le_mul_of_nonneg_left (hxabs j) hce_nn
      exact Rat.le_trans h1 h2
    have hle : (cv j - exactCoeff j) * x j ≤ ce j * maxB j := Rat.le_trans hdiff hmul
    have hcv : cv j * x j = exactCoeff j * x j + (cv j - exactCoeff j) * x j := by
      rw [rat_sub_mul, Rat.add_comm (exactCoeff j * x j), Rat.sub_add_cancel]
    rw [hcv]
    exact Rat.add_le_add_left.mpr hle
  have h1 := finSum_le_finSum hterm
  rw [finSum_add] at h1
  have h2 := rat_add_le_add hexact (Rat.le_refl (a := finSum n (fun j => ce j * maxB j)))
  have h3 := Rat.le_trans h1 h2
  have h4 : exactRHS + finSum n (fun j => ce j * maxB j) ≤
      exactRHS + rhsErr + finSum n (fun j => ce j * maxB j) := by
    have hle := Rat.add_le_add_left (c := exactRHS) |>.mpr hrhsErr
    rw [Rat.add_zero] at hle
    exact Rat.add_le_add_right.mpr hle
  exact Rat.le_trans (Rat.le_trans h3 h4) hrhs

/-! ### 3. Mixed-Integer Rounding (MIR) & Safe-Rounded cMIR Soundness -/

/-- Exact Marchand-Wolsey MIR integer variable coefficient. -/
def mirIntCoeff (alpha f0 : Rat) : Rat :=
  (alpha.floor : Rat) + max (fracPart alpha - f0) 0 / (1 - f0)

/-- Fundamental single-variable MIR inequality over `Rat`. -/
theorem mir_fundamental_ineq {z : Int} {s b : Rat}
    (hs : 0 ≤ s) (hle : (z : Rat) ≤ b + s) :
    (z : Rat) ≤ (b.floor : Rat) + s / (1 - fracPart b) := by
  have h1mf_pos : 0 < 1 - fracPart b := one_sub_fracPart_pos b
  have hf_nn : 0 ≤ fracPart b := fracPart_nonneg b
  have hz_cases : z ≤ b.floor ∨ b.floor + 1 ≤ z := by omega
  rcases hz_cases with hz0 | hz1
  · have hz_rat : (z : Rat) ≤ (b.floor : Rat) := Rat.intCast_le_intCast.mpr hz0
    have hdiv_nn : 0 ≤ s / (1 - fracPart b) := rat_div_nonneg hs (Rat.le_of_lt h1mf_pos)
    simpa [Rat.add_zero] using rat_add_le_add hz_rat hdiv_nn
  · let k : Int := z - b.floor
    have hk_ge1 : 1 ≤ k := by omega
    have hk_rat_ge1 : (1 : Rat) ≤ (k : Rat) := by simpa using Rat.intCast_le_intCast.mpr hk_ge1
    have hkf_ge_f : fracPart b ≤ (k : Rat) * fracPart b := by
      simpa [Rat.one_mul] using Rat.mul_le_mul_of_nonneg_right hk_rat_ge1 hf_nn
    have hk_mul : (k : Rat) * (1 - fracPart b) ≤ (k : Rat) - fracPart b := by
      rw [rat_mul_sub, Rat.mul_one]
      exact rat_sub_le_sub Rat.le_refl hkf_ge_f
    have hsub_eq : (k : Rat) - fracPart b = (z : Rat) - b := by
      dsimp [k, fracPart]
      rw [Rat.intCast_sub]
      simp only [Rat.sub_eq_add_neg, Rat.neg_add, Rat.neg_neg, Rat.add_assoc]
      rw [Rat.add_left_comm (-(b.floor : Rat)) (-b), Rat.add_comm (-(b.floor : Rat)) (b.floor : Rat),
        Rat.add_neg_cancel, Rat.add_zero]
    have hkmul_le_s : (k : Rat) * (1 - fracPart b) ≤ s := by
      have hzb : (z : Rat) - b ≤ s :=
        (Rat.sub_right_le_iff_le_add).mpr (by rwa [Rat.add_comm])
      exact Rat.le_trans hk_mul (by rwa [hsub_eq])
    have hk_le_div : (k : Rat) ≤ s / (1 - fracPart b) :=
      (rat_le_div_iff h1mf_pos).mpr hkmul_le_s
    have hz_eq : (z : Rat) = (b.floor : Rat) + (k : Rat) := by
      dsimp [k]
      rw [Rat.intCast_sub, Rat.add_comm (b.floor : Rat), Rat.sub_add_cancel]
    rw [hz_eq]
    exact Rat.add_le_add_left.mpr hk_le_div

private theorem mirIntCoeff_split (alpha f0 : Rat) :
    mirIntCoeff alpha f0 =
      (if fracPart alpha ≤ f0 then (alpha.floor : Rat) else ((alpha.floor + 1 : Int) : Rat)) -
        (if fracPart alpha ≤ f0 then 0 else 1 - fracPart alpha) / (1 - f0) := by
  unfold mirIntCoeff
  by_cases h : fracPart alpha ≤ f0
  · simp only [h, ↓reduceIte]
    have hle : fracPart alpha - f0 ≤ 0 := by
      have h1 := (Rat.add_le_add_right (c := -f0)).mpr h
      rwa [← Rat.sub_eq_add_neg, Rat.add_neg_cancel] at h1
    change (alpha.floor : Rat) + posPart (fracPart alpha - f0) / (1 - f0) = (alpha.floor : Rat) - 0 / (1 - f0)
    rw [posPart_eq_zero hle, rat_zero_div, Rat.add_zero, rat_sub_zero]
  · simp only [h, ↓reduceIte]
    have hlt : f0 < fracPart alpha := Rat.not_le.mp h
    have hpos : 0 ≤ fracPart alpha - f0 :=
      Rat.le_of_lt ((Rat.lt_iff_sub_pos f0 (fracPart alpha)).mp hlt)
    change (alpha.floor : Rat) + posPart (fracPart alpha - f0) / (1 - f0) =
      ((alpha.floor + 1 : Int) : Rat) - (1 - fracPart alpha) / (1 - f0)
    rw [posPart_eq_self hpos, Rat.intCast_add, Rat.intCast_one]
    by_cases hf1 : 1 - f0 = 0
    · have hf01 : f0 = 1 := by
        have := congrArg (· + f0) hf1
        simpa [Rat.sub_add_cancel, Rat.zero_add] using this.symm
      rw [hf01] at hlt
      exact absurd hlt (Rat.not_lt.mpr (Rat.le_of_lt (fracPart_lt_one alpha)))
    · have h1sub : 1 - (1 - fracPart alpha) / (1 - f0) = (fracPart alpha - f0) / (1 - f0) := by
        conv => lhs; lhs; rw [← rat_div_self hf1]
        rw [← rat_sub_div]
        congr 1
        simp only [Rat.sub_eq_add_neg, Rat.neg_add, Rat.neg_neg]
        have hre : (1 : Rat) + -f0 + (-1 + fracPart alpha) = (1 + -1) + (fracPart alpha + -f0) := by
          ac_rfl
        rw [hre, Rat.add_neg_cancel, Rat.zero_add]
      rw [← h1sub, rat_add_sub_assoc]

/-- Multi-variable Marchand-Wolsey Mixed-Integer Rounding (MIR) cut soundness. -/
theorem mir_cut_sound {mI : Nat} {alpha : Fin mI → Rat} {zI : Fin mI → Int} {b s : Rat}
    (hz : ∀ j, 0 ≤ zI j) (hs : 0 ≤ s)
    (hineq : finSum mI (fun j => alpha j * (zI j : Rat)) ≤ b + s) :
    finSum mI (fun j => mirIntCoeff (alpha j) (fracPart b) * (zI j : Rat)) ≤
      (b.floor : Rat) + s / (1 - fracPart b) := by
  let f0 := fracPart b
  let cInt : Fin mI → Int := fun j =>
    if fracPart (alpha j) ≤ f0 then (alpha j).floor else (alpha j).floor + 1
  let cExtra : Fin mI → Rat := fun j =>
    if fracPart (alpha j) ≤ f0 then 0 else 1 - fracPart (alpha j)
  let zLeft : Int := intFinSum mI (fun j => cInt j * zI j)
  let sExtra : Rat := finSum mI (fun j => cExtra j * (zI j : Rat))
  have hsExtra_nn : 0 ≤ sExtra := by
    apply finSum_nonneg
    intro j
    apply Rat.mul_nonneg _ (Rat.intCast_nonneg.mpr (hz j))
    dsimp [cExtra]
    split
    · exact Rat.le_refl
    · exact Rat.le_of_lt (one_sub_fracPart_pos (alpha j))
  have hterm_le : ∀ j, ((cInt j * zI j : Int) : Rat) - cExtra j * (zI j : Rat) ≤
      alpha j * (zI j : Rat) := by
    intro j
    rw [Rat.intCast_mul, ← rat_sub_mul]
    apply Rat.mul_le_mul_of_nonneg_right _ (Rat.intCast_nonneg.mpr (hz j))
    dsimp [cInt, cExtra]
    split
    · rw [rat_sub_zero]
      exact Rat.floor_le (alpha j)
    · rw [Rat.intCast_add, Rat.intCast_one, fracPart]
      have hid : ((alpha j).floor : Rat) + 1 - (1 - (alpha j - ((alpha j).floor : Rat))) = alpha j := by
        have h1 :
            ((alpha j).floor : Rat) + 1 - (1 - (alpha j - ((alpha j).floor : Rat))) =
              (((alpha j).floor : Rat) + -((alpha j).floor : Rat)) + (1 + -1) + alpha j := by
          simp only [Rat.sub_eq_add_neg, Rat.neg_add, Rat.neg_neg]
          ac_rfl
        rw [h1, Rat.add_neg_cancel, Rat.add_neg_cancel, Rat.zero_add, Rat.zero_add]
      rw [hid]
      exact Rat.le_refl
  have hsum_le : (zLeft : Rat) - sExtra ≤ b + s := by
    have h1 := finSum_le_finSum hterm_le
    rw [finSum_sub, ← intFinSum_eq_finSum] at h1
    exact Rat.le_trans h1 hineq
  have hfund := mir_fundamental_ineq (Rat.add_nonneg hs hsExtra_nn)
    (by
      have h1 := (Rat.sub_right_le_iff_le_add).mp hsum_le
      rwa [Rat.add_comm (b + s) sExtra, ← Rat.add_assoc, Rat.add_comm sExtra b, Rat.add_assoc,
        Rat.add_comm sExtra s] at h1)
  have hsub_div :
      (zLeft : Rat) - sExtra / (1 - f0) ≤ (b.floor : Rat) + s / (1 - f0) := by
    have hsplit : (s + sExtra) / (1 - f0) = s / (1 - f0) + sExtra / (1 - f0) :=
      rat_add_div s sExtra (1 - f0)
    change (zLeft : Rat) ≤ (b.floor : Rat) + (s + sExtra) / (1 - f0) at hfund
    rw [hsplit, ← Rat.add_assoc] at hfund
    exact (Rat.sub_right_le_iff_le_add).mpr hfund
  have hlhs_eq :
      finSum mI (fun j => mirIntCoeff (alpha j) f0 * (zI j : Rat)) =
        (zLeft : Rat) - sExtra / (1 - f0) := by
    have hterm : ∀ j, mirIntCoeff (alpha j) f0 * (zI j : Rat) =
        ((cInt j * zI j : Int) : Rat) - (cExtra j * (zI j : Rat)) / (1 - f0) := by
      intro j
      have hcast : (if fracPart (alpha j) ≤ f0 then ((alpha j).floor : Rat) else (((alpha j).floor + 1 : Int) : Rat)) =
          (cInt j : Rat) := by
        dsimp [cInt]
        split <;> rfl
      rw [mirIntCoeff_split (alpha j) f0, hcast, rat_sub_mul, Rat.intCast_mul, Rat.div_def,
        Rat.div_def, rat_mul_right_comm (cExtra j) (1 - f0)⁻¹ (zI j : Rat)]
    rw [finSum_congr hterm, finSum_sub, ← intFinSum_eq_finSum, ← finSum_div]
  rw [hlhs_eq]
  exact hsub_div

/-- Chosen bound for complementation (`l` when `useLower = true`, `u` when `useLower = false`). -/
def cmirBound (useLower : Bool) (l u : Rat) : Rat :=
  if useLower then l else u

/-- Signed coefficient after complementation (`a` when `useLower = true`, `-a` when `useLower = false`). -/
def cmirSignedCoeff (useLower : Bool) (a : Rat) : Rat :=
  if useLower then a else -a

/-- Exact shifted RHS before dividing by `delta`. -/
def cmirShiftedRHS {mI mC : Nat}
    (aI lI uI : Fin mI → Rat) (useLowerI : Fin mI → Bool)
    (aC lC uC : Fin mC → Rat) (useLowerC : Fin mC → Bool)
    (rhs : Rat) : Rat :=
  rhs - finSum mI (fun j => aI j * cmirBound (useLowerI j) (lI j) (uI j)) -
    finSum mC (fun k => aC k * cmirBound (useLowerC k) (lC k) (uC k))

/-- Soundness of complemented and outward safe-rounded single-row cMIR (`cuts.go:separateCMIRFromSparseRow`). -/
theorem cmir_safe_rounded_sound {mI mC : Nat}
    {aI xI lI uI : Fin mI → Rat} {hasLI hasUI useLowerI : Fin mI → Bool}
    {aC xC lC uC : Fin mC → Rat} {hasLC hasUC useLowerC : Fin mC → Bool}
    {rhs delta betaHat cutRHS : Rat}
    {alphaHat cI : Fin mI → Rat} {cC : Fin mC → Rat}
    (hdelta : 0 < delta)
    (hxI : ∀ j, (∃ z : Int, xI j = (z : Rat)) ∧
      (hasLI j = true → lI j ≤ xI j) ∧ (hasUI j = true → xI j ≤ uI j) ∧
      (if useLowerI j then hasLI j = true ∧ ∃ z : Int, lI j = (z : Rat)
       else hasUI j = true ∧ ∃ z : Int, uI j = (z : Rat)))
    (hxC : ∀ k, (hasLC k = true → lC k ≤ xC k) ∧ (hasUC k = true → xC k ≤ uC k) ∧
      (if useLowerC k then hasLC k = true else hasUC k = true))
    (hrow : finSum mI (fun j => aI j * xI j) + finSum mC (fun k => aC k * xC k) ≤ rhs)
    (hbeta : cmirShiftedRHS aI lI uI useLowerI aC lC uC useLowerC rhs / delta ≤ betaHat)
    (halpha : ∀ j, alphaHat j ≤ cmirSignedCoeff (useLowerI j) (aI j) / delta)
    (hcI : ∀ j, (hasLI j = true ∧ hasUI j = true ∧ lI j = uI j ∧ cI j = 0) ∨
      cI j ≤ mirIntCoeff (alphaHat j) (fracPart betaHat))
    (hcC : ∀ k, max (-(cmirSignedCoeff (useLowerC k) (aC k))) 0 / (delta * (1 - fracPart betaHat)) ≤ cC k)
    (hrhs : (betaHat.floor : Rat) +
      finSum mI (fun j => cmirSignedCoeff (useLowerI j) (cI j) * cmirBound (useLowerI j) (lI j) (uI j)) +
      finSum mC (fun k => (if useLowerC k then -cC k else cC k) * cmirBound (useLowerC k) (lC k) (uC k)) ≤ cutRHS) :
    finSum mI (fun j => cmirSignedCoeff (useLowerI j) (cI j) * xI j) +
      finSum mC (fun k => (if useLowerC k then -cC k else cC k) * xC k) ≤ cutRHS := by
  let yI : Fin mI → Rat := fun j =>
    if useLowerI j then xI j - lI j else uI j - xI j
  let yC : Fin mC → Rat := fun k =>
    if useLowerC k then xC k - lC k else uC k - xC k
  have hyI_prop : ∀ j, 0 ≤ yI j ∧ ∃ z : Int, yI j = (z : Rat) := by
    intro j
    rcases hxI j with ⟨⟨zx, hzx⟩, hL, hU, huse⟩
    dsimp [yI]
    cases hu : useLowerI j
    · simp only [hu, Bool.false_eq_true, ↓reduceIte] at huse ⊢
      rcases huse with ⟨hHasU, ⟨zu, hzu⟩⟩
      exact ⟨(Rat.le_iff_sub_nonneg _ _).mp (hU hHasU), ⟨zu - zx, by rw [hzu, hzx, Rat.intCast_sub]⟩⟩
    · simp only [hu, ↓reduceIte] at huse ⊢
      rcases huse with ⟨hHasL, ⟨zl, hzl⟩⟩
      exact ⟨(Rat.le_iff_sub_nonneg _ _).mp (hL hHasL), ⟨zx - zl, by rw [hzx, hzl, Rat.intCast_sub]⟩⟩
  let zI : Fin mI → Int := fun j => Classical.choose (hyI_prop j).2
  have hzI : ∀ j, yI j = (zI j : Rat) := fun j => Classical.choose_spec (hyI_prop j).2
  have hzI_nn : ∀ j, 0 ≤ zI j := fun j =>
    Rat.intCast_nonneg.mp (by rw [← hzI j]; exact (hyI_prop j).1)
  have hyC_nn : ∀ k, 0 ≤ yC k := by
    intro k
    rcases hxC k with ⟨hL, hU, huse⟩
    dsimp [yC]
    cases hu : useLowerC k
    · simp only [hu, Bool.false_eq_true, ↓reduceIte] at huse ⊢
      exact (Rat.le_iff_sub_nonneg _ _).mp (hU huse)
    · simp only [hu, ↓reduceIte] at huse ⊢
      exact (Rat.le_iff_sub_nonneg _ _).mp (hL huse)
  have hshiftI : ∀ j, aI j * xI j =
      aI j * cmirBound (useLowerI j) (lI j) (uI j) + cmirSignedCoeff (useLowerI j) (aI j) * yI j := by
    intro j
    dsimp [cmirBound, cmirSignedCoeff, yI]
    cases useLowerI j <;> simp only [↓reduceIte, Bool.false_eq_true]
    · rw [Rat.neg_mul, rat_mul_sub, Rat.neg_sub]
      exact rat_eq_add_sub (aI j * uI j) (aI j * xI j)
    · rw [rat_mul_sub, Rat.add_comm (aI j * lI j), Rat.sub_add_cancel]
  have hshiftC : ∀ k, aC k * xC k =
      aC k * cmirBound (useLowerC k) (lC k) (uC k) + cmirSignedCoeff (useLowerC k) (aC k) * yC k := by
    intro k
    dsimp [cmirBound, cmirSignedCoeff, yC]
    cases useLowerC k <;> simp only [↓reduceIte, Bool.false_eq_true]
    · rw [Rat.neg_mul, rat_mul_sub, Rat.neg_sub]
      exact rat_eq_add_sub (aC k * uC k) (aC k * xC k)
    · rw [rat_mul_sub, Rat.add_comm (aC k * lC k), Rat.sub_add_cancel]
  have hshifted_le :
      finSum mI (fun j => cmirSignedCoeff (useLowerI j) (aI j) * yI j) +
        finSum mC (fun k => cmirSignedCoeff (useLowerC k) (aC k) * yC k) ≤
      cmirShiftedRHS aI lI uI useLowerI aC lC uC useLowerC rhs := by
    rw [finSum_congr hshiftI, finSum_congr hshiftC, finSum_add, finSum_add] at hrow
    dsimp [cmirShiftedRHS]
    have hre :
        (finSum mI fun i => aI i * cmirBound (useLowerI i) (lI i) (uI i)) +
            (finSum mI fun i => cmirSignedCoeff (useLowerI i) (aI i) * yI i) +
          ((finSum mC fun i => aC i * cmirBound (useLowerC i) (lC i) (uC i)) +
            (finSum mC fun i => cmirSignedCoeff (useLowerC i) (aC i) * yC i)) =
        ((finSum mI fun i => cmirSignedCoeff (useLowerI i) (aI i) * yI i) +
          (finSum mC fun i => cmirSignedCoeff (useLowerC i) (aC i) * yC i)) +
          (finSum mC fun k => aC k * cmirBound (useLowerC k) (lC k) (uC k)) +
          (finSum mI fun j => aI j * cmirBound (useLowerI j) (lI j) (uI j)) := by
      ac_rfl
    rw [hre] at hrow
    exact (Rat.le_sub_iff).mpr ((Rat.le_sub_iff).mpr hrow)
  let sCont : Rat := finSum mC (fun k => (max (-(cmirSignedCoeff (useLowerC k) (aC k))) 0 / delta) * yC k)
  have hsCont_nn : 0 ≤ sCont :=
    finSum_nonneg fun k =>
      Rat.mul_nonneg (rat_div_nonneg rat_le_max_right (Rat.le_of_lt hdelta)) (hyC_nn k)
  have hcont_drop :
      -sCont ≤ finSum mC (fun k => cmirSignedCoeff (useLowerC k) (aC k) * yC k) / delta := by
    rw [finSum_div, ← finSum_neg]
    apply finSum_le_finSum
    intro k
    have hcoeff : -(max (-(cmirSignedCoeff (useLowerC k) (aC k))) 0 / delta) ≤
        cmirSignedCoeff (useLowerC k) (aC k) / delta := by
      rw [← rat_neg_div]
      apply rat_div_le_div_of_pos _ hdelta
      have h1 := Rat.neg_le_neg (rat_le_max_left (a := -(cmirSignedCoeff (useLowerC k) (aC k))) (b := 0))
      simpa [Rat.neg_neg] using h1
    rw [← Rat.neg_mul]
    have hm := Rat.mul_le_mul_of_nonneg_right hcoeff (hyC_nn k)
    simp only [Rat.div_def] at hm ⊢
    rw [rat_mul_right_comm (cmirSignedCoeff (useLowerC k) (aC k)) (yC k) delta⁻¹]
    exact hm
  have hmir_in :
      finSum mI (fun j => alphaHat j * (zI j : Rat)) ≤ betaHat + sCont := by
    have h1 : finSum mI (fun j => alphaHat j * (zI j : Rat)) ≤
        finSum mI (fun j => cmirSignedCoeff (useLowerI j) (aI j) * yI j) / delta := by
      rw [finSum_div]
      apply finSum_le_finSum
      intro j
      rw [← hzI j, Rat.div_def, Rat.mul_assoc _ (yI j) delta⁻¹, Rat.mul_comm (yI j) delta⁻¹,
        ← Rat.mul_assoc, ← Rat.div_def]
      exact Rat.mul_le_mul_of_nonneg_right (halpha j) (hyI_prop j).1
    have h2 : finSum mI (fun j => cmirSignedCoeff (useLowerI j) (aI j) * yI j) / delta ≤
        cmirShiftedRHS aI lI uI useLowerI aC lC uC useLowerC rhs / delta + sCont := by
      have hd := rat_div_le_div_of_pos hshifted_le hdelta
      rw [rat_add_div] at hd
      have hsub := Rat.le_trans (Rat.add_le_add_left.mpr hcont_drop) hd
      have h4 := (Rat.le_sub_iff).mpr (by simpa [Rat.sub_eq_add_neg, Rat.add_comm (-sCont)] using hsub)
      simpa [Rat.sub_eq_add_neg, Rat.neg_neg] using h4
    exact Rat.le_trans h1 (Rat.le_trans h2 (Rat.add_le_add_right.mpr hbeta))
  have hmir := mir_cut_sound hzI_nn hsCont_nn hmir_in
  have hLHS_le :
      finSum mI (fun j => cI j * yI j) ≤
        (betaHat.floor : Rat) + finSum mC (fun k => cC k * yC k) := by
    have hI_safe :
        finSum mI (fun j => cI j * yI j) ≤
          finSum mI (fun j => mirIntCoeff (alphaHat j) (fracPart betaHat) * (zI j : Rat)) := by
      apply finSum_le_finSum
      intro j
      rw [← hzI j]
      rcases hcI j with ⟨hHasL, hHasU, hlu, hc0⟩ | hcle
      · rcases hxI j with ⟨_, hL, hU, _⟩
        have hx_eq : xI j = lI j := Rat.le_antisymm (by rw [hlu]; exact hU hHasU) (hL hHasL)
        have hy0 : yI j = 0 := by
          dsimp [yI]
          cases useLowerI j <;> simp [hlu, hx_eq, Rat.sub_self]
        rw [hc0, hy0, Rat.zero_mul, Rat.mul_zero]
        exact Rat.le_refl
      · exact Rat.mul_le_mul_of_nonneg_right hcle (hyI_prop j).1
    have hC_safe :
        sCont / (1 - fracPart betaHat) ≤ finSum mC (fun k => cC k * yC k) := by
      dsimp [sCont]
      rw [finSum_div]
      apply finSum_le_finSum
      intro k
      have hassoc :
          (max (-(cmirSignedCoeff (useLowerC k) (aC k))) 0 / delta * yC k) / (1 - fracPart betaHat) =
            (max (-(cmirSignedCoeff (useLowerC k) (aC k))) 0 / (delta * (1 - fracPart betaHat))) * yC k := by
        rw [Rat.div_def, Rat.div_def, Rat.div_def, Rat.inv_mul_rev]
        rw [Rat.mul_assoc _ (yC k) (1 - fracPart betaHat)⁻¹, Rat.mul_comm (yC k) (1 - fracPart betaHat)⁻¹,
          ← Rat.mul_assoc, Rat.mul_assoc _ delta⁻¹ (1 - fracPart betaHat)⁻¹,
          Rat.mul_comm delta⁻¹ (1 - fracPart betaHat)⁻¹]
      rw [hassoc]
      exact Rat.mul_le_mul_of_nonneg_right (hcC k) (hyC_nn k)
    exact Rat.le_trans hI_safe (Rat.le_trans hmir (Rat.add_le_add_left.mpr hC_safe))
  have hbackI : ∀ j, cmirSignedCoeff (useLowerI j) (cI j) * xI j =
      cmirSignedCoeff (useLowerI j) (cI j) * cmirBound (useLowerI j) (lI j) (uI j) + cI j * yI j := by
    intro j
    dsimp [cmirBound, cmirSignedCoeff, yI]
    cases useLowerI j <;> simp only [↓reduceIte, Bool.false_eq_true]
    · rw [Rat.neg_mul, Rat.neg_mul, rat_mul_sub]
      exact rat_neg_eq_neg_add_sub (cI j * uI j) (cI j * xI j)
    · rw [rat_mul_sub, Rat.add_comm (cI j * lI j), Rat.sub_add_cancel]
  have hbackC : ∀ k, (if useLowerC k then -cC k else cC k) * xC k =
      (if useLowerC k then -cC k else cC k) * cmirBound (useLowerC k) (lC k) (uC k) - cC k * yC k := by
    intro k
    dsimp [cmirBound, yC]
    cases useLowerC k <;> simp only [↓reduceIte, Bool.false_eq_true]
    · rw [rat_mul_sub, rat_sub_sub_cancel]
    · rw [Rat.neg_mul, Rat.neg_mul, rat_mul_sub]
      exact rat_neg_eq_neg_sub_sub (cC k * lC k) (cC k * xC k)
  rw [finSum_congr hbackI, finSum_congr hbackC, finSum_add, finSum_sub]
  have h1 :
      finSum mI (fun j => cI j * yI j) - finSum mC (fun k => cC k * yC k) ≤ (betaHat.floor : Rat) :=
    (Rat.sub_right_le_iff_le_add).mpr hLHS_le
  let Bsum : Rat :=
    finSum mI (fun j => cmirSignedCoeff (useLowerI j) (cI j) * cmirBound (useLowerI j) (lI j) (uI j)) +
    finSum mC (fun k => (if useLowerC k then -cC k else cC k) * cmirBound (useLowerC k) (lC k) (uC k))
  have h2 :
      Bsum + (finSum mI (fun j => cI j * yI j) - finSum mC (fun k => cC k * yC k)) ≤
        Bsum + (betaHat.floor : Rat) :=
    Rat.add_le_add_left.mpr h1
  have hre :
      (finSum mI (fun j => cmirSignedCoeff (useLowerI j) (cI j) * cmirBound (useLowerI j) (lI j) (uI j)) +
        finSum mI (fun j => cI j * yI j)) +
      (finSum mC (fun k => (if useLowerC k then -cC k else cC k) * cmirBound (useLowerC k) (lC k) (uC k)) -
        finSum mC (fun k => cC k * yC k)) =
      Bsum + (finSum mI (fun j => cI j * yI j) - finSum mC (fun k => cC k * yC k)) := by
    dsimp [Bsum]
    simp only [Rat.sub_eq_add_neg]
    ac_rfl
  have hBsum_le : Bsum + (betaHat.floor : Rat) ≤ cutRHS := by
    dsimp [Bsum]
    have hB2 :
        (finSum mI (fun j => cmirSignedCoeff (useLowerI j) (cI j) * cmirBound (useLowerI j) (lI j) (uI j)) +
          finSum mC (fun k => (if useLowerC k then -cC k else cC k) * cmirBound (useLowerC k) (lC k) (uC k))) +
          (betaHat.floor : Rat) =
        (betaHat.floor : Rat) +
          finSum mI (fun j => cmirSignedCoeff (useLowerI j) (cI j) * cmirBound (useLowerI j) (lI j) (uI j)) +
          finSum mC (fun k => (if useLowerC k then -cC k else cC k) * cmirBound (useLowerC k) (lC k) (uC k)) := by
      ac_rfl
    rw [hB2]
    exact hrhs
  rw [hre]
  exact Rat.le_trans h2 hBsum_le

/-- Exact complementation cMIR cut soundness (specialization of `cmir_safe_rounded_sound`). -/
theorem cmir_complement_sound {mI mC : Nat}
    {aI xI lI uI : Fin mI → Rat} {hasLI hasUI useLowerI : Fin mI → Bool}
    {aC xC lC uC : Fin mC → Rat} {hasLC hasUC useLowerC : Fin mC → Bool}
    {rhs delta : Rat}
    (hdelta : 0 < delta)
    (hxI : ∀ j, (∃ z : Int, xI j = (z : Rat)) ∧
      (hasLI j = true → lI j ≤ xI j) ∧ (hasUI j = true → xI j ≤ uI j) ∧
      (if useLowerI j then hasLI j = true ∧ ∃ z : Int, lI j = (z : Rat)
       else hasUI j = true ∧ ∃ z : Int, uI j = (z : Rat)))
    (hxC : ∀ k, (hasLC k = true → lC k ≤ xC k) ∧ (hasUC k = true → xC k ≤ uC k) ∧
      (if useLowerC k then hasLC k = true else hasUC k = true))
    (hrow : finSum mI (fun j => aI j * xI j) + finSum mC (fun k => aC k * xC k) ≤ rhs) :
    let betaHat := cmirShiftedRHS aI lI uI useLowerI aC lC uC useLowerC rhs / delta
    let f0 := fracPart betaHat
    let cI : Fin mI → Rat := fun j => mirIntCoeff (cmirSignedCoeff (useLowerI j) (aI j) / delta) f0
    let cC : Fin mC → Rat := fun k => max (-(cmirSignedCoeff (useLowerC k) (aC k))) 0 / (delta * (1 - f0))
    finSum mI (fun j => cmirSignedCoeff (useLowerI j) (cI j) * xI j) +
      finSum mC (fun k => (if useLowerC k then -cC k else cC k) * xC k) ≤
    (betaHat.floor : Rat) +
      finSum mI (fun j => cmirSignedCoeff (useLowerI j) (cI j) * cmirBound (useLowerI j) (lI j) (uI j)) +
      finSum mC (fun k => (if useLowerC k then -cC k else cC k) * cmirBound (useLowerC k) (lC k) (uC k)) :=
  cmir_safe_rounded_sound hdelta hxI hxC hrow Rat.le_refl (fun _ => Rat.le_refl)
    (fun _ => Or.inr Rat.le_refl) (fun _ => Rat.le_refl) Rat.le_refl

/-! ### 4. Variable Upper Bound (VUB) Disaggregation & Sparse Cut Relaxation -/

/-- Soundness of disaggregated VUB inequalities (`cuts.go:separateFlowAndVUBCuts`). -/
theorem vub_disagg_sound {n : Nat} {a x u : Fin n → Rat} {C y : Rat}
    (hrow : finSum n (fun j => a j * x j) - C * y ≤ 0)
    (ha : ∀ j, 0 < a j)
    (hx : ∀ j, 0 ≤ x j ∧ x j ≤ u j)
    (_hC : 0 ≤ C)
    (hy : y = 0 ∨ y = 1) :
    ∀ k : Fin n, x k ≤ min (u k) (C / a k) * y := by
  intro k
  have hterm_nn : ∀ j, 0 ≤ a j * x j := fun j =>
    Rat.mul_nonneg (Rat.le_of_lt (ha j)) (hx j).1
  have hk_le_sum : a k * x k ≤ finSum n (fun j => a j * x j) :=
    le_finSum_of_nonneg hterm_nn k
  have hsum_le_Cy : finSum n (fun j => a j * x j) ≤ C * y := by
    have h1 := (Rat.sub_right_le_iff_le_add).mp hrow
    rwa [Rat.zero_add] at h1
  have hak_le_Cy : a k * x k ≤ C * y := Rat.le_trans hk_le_sum hsum_le_Cy
  rcases hy with hy0 | hy1
  · rw [hy0, Rat.mul_zero] at hak_le_Cy ⊢
    apply Rat.le_of_mul_le_mul_left _ (ha k)
    rwa [Rat.mul_zero]
  · rw [hy1, Rat.mul_one] at hak_le_Cy ⊢
    rw [rat_le_min_iff]
    refine ⟨(hx k).2, ?_⟩
    rw [rat_le_div_iff (ha k), Rat.mul_comm]
    exact hak_le_Cy

/-- Effective VUB upper bound with optional finite variable upper bound. -/
def vubEffUB (u : Option Rat) (C a : Rat) : Rat :=
  match u with
  | some uk => min uk (C / a)
  | none => C / a

/-- Optional upper-bound and safe outward-rounded VUB disaggregation soundness. -/
theorem vub_disagg_opt_sound {n : Nat} {a x : Fin n → Rat} {u : Fin n → Option Rat}
    {C rhs y : Rat}
    (hrow : finSum n (fun j => a j * x j) - C * y ≤ rhs)
    (hrhs : rhs ≤ 0)
    (ha : ∀ j, 0 < a j)
    (hx : ∀ j, 0 ≤ x j)
    (hu : ∀ j uk, u j = some uk → x j ≤ uk)
    (_hC : 0 ≤ C)
    (hy : y = 0 ∨ y = 1) :
    ∀ (k : Fin n) {effUB : Rat}, vubEffUB (u k) C (a k) ≤ effUB → x k ≤ effUB * y := by
  intro k effUB heff
  have hrow0 : finSum n (fun j => a j * x j) - C * y ≤ 0 := Rat.le_trans hrow hrhs
  have hterm_nn : ∀ j, 0 ≤ a j * x j := fun j =>
    Rat.mul_nonneg (Rat.le_of_lt (ha j)) (hx j)
  have hak_le_Cy : a k * x k ≤ C * y := by
    have h1 := (Rat.sub_right_le_iff_le_add).mp hrow0
    rw [Rat.zero_add] at h1
    exact Rat.le_trans (le_finSum_of_nonneg hterm_nn k) h1
  have hy_nn : 0 ≤ y := by
    rcases hy with rfl | rfl
    · exact Rat.le_refl
    · exact Rat.le_of_lt (by decide)
  have hexact : x k ≤ vubEffUB (u k) C (a k) * y := by
    rcases hy with rfl | rfl
    · rw [Rat.mul_zero] at hak_le_Cy ⊢
      apply Rat.le_of_mul_le_mul_left _ (ha k)
      rwa [Rat.mul_zero]
    · rw [Rat.mul_one] at hak_le_Cy ⊢
      dsimp [vubEffUB]
      cases huk : u k with
      | none =>
        rw [rat_le_div_iff (ha k), Rat.mul_comm]
        exact hak_le_Cy
      | some uk =>
        rw [rat_le_min_iff]
        refine ⟨hu k uk huk, ?_⟩
        rw [rat_le_div_iff (ha k), Rat.mul_comm]
        exact hak_le_Cy
  exact Rat.le_trans hexact (Rat.mul_le_mul_of_nonneg_right heff hy_nn)

private theorem min_mul_bounds_le {v l u x : Rat} (hl : l ≤ x) (hu : x ≤ u) :
    min (v * l) (v * u) ≤ v * x := by
  rcases Rat.le_total (a := 0) (b := v) with hv | hv
  · have hmin : min (v * l) (v * u) ≤ v * l := by
      rw [Rat.min_def]
      split
      · exact Rat.le_refl
      · next h => exact Rat.le_of_lt (Rat.not_le.mp h)
    exact Rat.le_trans hmin (Rat.mul_le_mul_of_nonneg_left hl hv)
  · have hmin : min (v * l) (v * u) ≤ v * u := by
      rw [Rat.min_def]
      split
      · assumption
      · exact Rat.le_refl
    exact Rat.le_trans hmin (rat_mul_le_mul_of_nonpos_left hu hv)

/-- Soundness of dropping small cut coefficients via bounds and scaling by `s > 0` (`cuts.go:finalizeSparseCut`). -/
theorem cut_drop_scale_sound {n : Nat} {v kept lb ub x : Fin n → Rat}
    {rhs droppedRHS s cutRHS : Rat}
    (hcut : finSum n (fun j => v j * x j) ≤ rhs)
    (hxbnd : ∀ j, lb j ≤ x j ∧ x j ≤ ub j)
    (hdrop : ∀ j, kept j = v j ∨ kept j = 0)
    (hdropRHS : rhs - finSum n (fun j => if kept j = v j then 0 else min (v j * lb j) (v j * ub j)) ≤ droppedRHS)
    (hs : 0 < s)
    (hscale : s * droppedRHS ≤ cutRHS) :
    finSum n (fun j => (s * kept j) * x j) ≤ cutRHS := by
  have hkept : ∀ j, kept j * x j ≤ v j * x j - (if kept j = v j then 0 else min (v j * lb j) (v j * ub j)) := by
    intro j
    by_cases hkv : kept j = v j
    · simp [hkv, rat_sub_zero]
    · rcases hdrop j with hkv' | hk0
      · exact absurd hkv' hkv
      · have hmin : min (v j * lb j) (v j * ub j) ≤ v j * x j :=
          min_mul_bounds_le (hxbnd j).1 (hxbnd j).2
        have hite : (if kept j = v j then (0 : Rat) else min (v j * lb j) (v j * ub j)) =
            min (v j * lb j) (v j * ub j) := by
          split
          · next heq => exact absurd heq hkv
          · rfl
        rw [hite, hk0, Rat.zero_mul]
        exact (Rat.le_sub_iff).mpr (by simpa [Rat.zero_add] using hmin)
  have h1 := finSum_le_finSum hkept
  rw [finSum_sub] at h1
  have h2 := Rat.le_trans h1 (rat_sub_le_sub
    (c := finSum n (fun j => if kept j = v j then 0 else min (v j * lb j) (v j * ub j)))
    (d := finSum n (fun j => if kept j = v j then 0 else min (v j * lb j) (v j * ub j)))
    hcut Rat.le_refl)
  have h3 := Rat.le_trans h2 hdropRHS
  have h4 := Rat.mul_le_mul_of_nonneg_left h3 (Rat.le_of_lt hs)
  have hlhs : finSum n (fun j => (s * kept j) * x j) = s * finSum n (fun j => kept j * x j) := by
    rw [← finSum_const_mul]
    exact finSum_congr fun j => Rat.mul_assoc s (kept j) (x j)
  rw [hlhs]
  exact Rat.le_trans h4 hscale

/-! ### 5. Executable Checkers and Universal Soundness Proofs for `cuts-check` CLI -/

/-- Sparse linear term `(col, val)`. -/
structure SparseTerm where
  /-- Zero-based column index of the variable. -/
  col : Nat
  /-- Rational coefficient multiplying `x[col]`. -/
  val : Rat
  deriving Repr, Inhabited

/-- Sparse linear cut `∑ val * x[col] ≤ rhs`. -/
structure CutSpec where
  /-- Sparse nonzero terms on the left-hand side of the cut. -/
  terms : Array SparseTerm
  /-- Right-hand side upper bound of the cut. -/
  rhs : Rat
  deriving Repr, Inhabited

/-- Sparse linear constraint row. -/
structure RowSpec where
  /-- Sparse nonzero terms on the left-hand side of the row. -/
  terms : Array SparseTerm
  /-- Constraint relation sense (`≤`, `≥`, or `=`). -/
  sense : ConstraintSense
  /-- Right-hand side constant of the row. -/
  rhs : Rat
  deriving Repr, Inhabited

/-- Mixed-integer linear program specification for structural cut checks. -/
structure MIPSpec where
  /-- Total number of decision variables. -/
  numVars : Nat
  /-- Integrality flag for each variable (`true` for integer variables). -/
  isInt : Array Bool
  /-- Optional finite lower bound for each variable (`none` means `-∞`). -/
  lb : Array (Option Rat)
  /-- Optional finite upper bound for each variable (`none` means `+∞`). -/
  ub : Array (Option Rat)
  /-- Linear constraint rows of the MIP. -/
  rows : Array RowSpec
  deriving Repr, Inhabited

/-- Evaluates a sparse linear combination `∑ t.val * x[t.col]`, failing if any column index is out of bounds. -/
def evalTerms (terms : Array SparseTerm) (x : Array Rat) : Except String Rat := do
  let mut acc : Rat := 0
  for t in terms do
    if h : t.col < x.size then
      acc := acc + t.val * x[t.col]
    else
      throw s!"column index {t.col} out of bounds (numVars={x.size})"
  pure acc

/-- Evaluates the dense dot product `∑ a[i] * x[i]`, failing if the array lengths differ. -/
def evalArrayDot (a x : Array Rat) : Except String Rat := do
  if a.size != x.size then
    throw s!"vector length mismatch: {a.size} vs {x.size}"
  let mut acc : Rat := 0
  for i in [:a.size] do
    if _h : i < a.size then
      acc := acc + a[i]! * x[i]!
  pure acc

/-- Boolean test checking whether `lhs` satisfies `sense` against `rhs`. -/
def satisfiesSenseBool (lhs : Rat) (sense : ConstraintSense) (rhs : Rat) : Bool :=
  match sense with
  | .le => decide (lhs ≤ rhs)
  | .eq => decide (lhs == rhs)
  | .ge => decide (rhs ≤ lhs)

/-- Checks that `x` is a feasible solution to `mip` (dimensions, integrality, bounds, and all rows). -/
def checkMIPFeasible (mip : MIPSpec) (x : Array Rat) : Except String Unit := do
  if x.size != mip.numVars then
    throw s!"solution length {x.size} != numVars {mip.numVars}"
  if mip.isInt.size != mip.numVars || mip.lb.size != mip.numVars || mip.ub.size != mip.numVars then
    throw s!"MIP metadata arrays must match numVars={mip.numVars}"
  for j in [:mip.numVars] do
    let xj := x[j]!
    if mip.isInt[j]! && xj.den != 1 then
      throw s!"integer variable {j} has non-integral value {xj}"
    match mip.lb[j]! with
    | some l =>
      if !(l ≤ xj) then
        throw s!"variable {j} value {xj} < lower bound {l}"
    | none => pure ()
    match mip.ub[j]! with
    | some u =>
      if !(xj ≤ u) then
        throw s!"variable {j} value {xj} > upper bound {u}"
    | none => pure ()
  for idx in [:mip.rows.size] do
    let row := mip.rows[idx]!
    let lhs ← evalTerms row.terms x
    if !satisfiesSenseBool lhs row.sense row.rhs then
      throw s!"row {idx} violated: lhs={lhs} sense={repr row.sense} rhs={row.rhs}"

/-- Evaluates whether `cut` is satisfied (`lhs ≤ cut.rhs`) at point `x`. -/
def checkCutAtPoint (cut : CutSpec) (x : Array Rat) : Except String Bool := do
  let lhs ← evalTerms cut.terms x
  pure (decide (lhs ≤ cut.rhs))

private def allFinB : (n : Nat) → (Fin n → Bool) → Bool
  | 0, _ => true
  | n + 1, p => allFinB n (fun i => p i.castSucc) && p (Fin.last n)

private theorem allFinB_eq_true_iff {n : Nat} {p : Fin n → Bool} :
    allFinB n p = true ↔ ∀ i : Fin n, p i = true := by
  induction n with
  | zero =>
    constructor
    · intro _ i; exact Fin.elim0 i
    · intro _; rfl
  | succ n ih =>
    simp only [allFinB, Bool.and_eq_true, ih]
    constructor
    · intro ⟨hprev, hlast⟩ i
      by_cases hi : i.val < n
      · have heq : i = (⟨i.val, hi⟩ : Fin n).castSucc := Fin.ext rfl
        rw [heq]; exact hprev ⟨i.val, hi⟩
      · have heq : i = Fin.last n := Fin.ext (by simp [Fin.val_last]; have := i.isLt; omega)
        rw [heq]; exact hlast
    · intro hall
      exact ⟨fun i => hall i.castSucc, hall (Fin.last n)⟩

private def guardExcB {α : Type} (c : Bool) (msg : String) (k : Except String α) : Except String α :=
  if c then k else .error msg

private theorem guardExcB_ok {α : Type} {c : Bool} {msg : String} {k : Except String α} {v : α}
    (h : guardExcB c msg k = .ok v) : c = true ∧ k = .ok v := by
  unfold guardExcB at h
  cases c
  · simp at h
  · exact ⟨rfl, h⟩

/-- Sum of coefficients for column `c` in a sparse term list (handling duplicates). -/
def sparseTermsCoeff : List SparseTerm → Nat → Rat
  | [], _ => 0
  | t :: ts, c => (if t.col == c then t.val else 0) + sparseTermsCoeff ts c

/-- Evaluates a sparse term list against `x : Fin n → Rat` (zero for out-of-range cols). -/
def evalSparseTermsFin (n : Nat) (terms : List SparseTerm) (x : Fin n → Rat) : Rat :=
  match terms with
  | [] => 0
  | t :: ts =>
    (if h : t.col < n then t.val * x ⟨t.col, h⟩ else 0) + evalSparseTermsFin n ts x

private theorem sparseTermsCoeff_not_mem {ts : List SparseTerm} {c : Nat}
    (hnot : ∀ t ∈ ts, t.col ≠ c) : sparseTermsCoeff ts c = 0 := by
  induction ts with
  | nil => rfl
  | cons t rest ih =>
    have hne : (t.col == c) = false := beq_false_of_ne (hnot t (List.mem_cons_self ..))
    simp only [sparseTermsCoeff, hne, Bool.false_eq_true, ↓reduceIte, Rat.zero_add]
    exact ih (fun u hu => hnot u (List.mem_cons_of_mem _ hu))

private theorem evalSparseTermsFin_eq_finSum {n : Nat} (ts : List SparseTerm) (x : Fin n → Rat)
    (hcols : ∀ t ∈ ts, t.col < n) :
    evalSparseTermsFin n ts x = finSum n (fun j => sparseTermsCoeff ts j.val * x j) := by
  induction ts with
  | nil => simp [evalSparseTermsFin, sparseTermsCoeff]
  | cons t rest ih =>
    have ht : t.col < n := hcols t (List.mem_cons_self ..)
    have hrest : ∀ u ∈ rest, u.col < n := fun u hu => hcols u (List.mem_cons_of_mem _ hu)
    simp only [evalSparseTermsFin, sparseTermsCoeff, ht, ↓reduceDIte]
    rw [ih hrest]
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
        · have hne : (t.col == j.val) = false := beq_false_of_ne (fun heq => hj (Fin.ext heq.symm))
          simp [hj, hne]
      rw [hcong, finSum_single]
    rw [hsingle, ← finSum_add]
    exact finSum_congr fun j => (Rat.add_mul _ _ _).symm

/-- Helper avoiding dependent `match` generalization over optional certificate coefficient arrays. -/
def optCoeffAt (opt : Option (Array Rat)) (fallback : Rat) (i : Nat) : Rat :=
  match opt with
  | none => fallback
  | some c => if h : i < c.size then c[i] else fallback

private theorem ite_not_error_okB {b : Bool} {msg : String} {k : Except String Unit}
    (h : (if !b then Except.error msg else k) = .ok ()) : b = true ∧ k = .ok () := by
  cases b <;> simp at h ⊢
  exact h

/-- Algebraic core of `checkTableauGMI`: checks `0 < fracPart beta < 1` and exact/weakened
GMI coefficients `gmiIntCoeff ≤ cI`, `gmiContCoeff ≤ cC`. -/
def checkTableauGMIAlg
    (aI aC : Array Rat) (beta : Rat)
    (cIOpt cCOpt : Option (Array Rat)) : Except String Unit :=
  let f0 := fracPart beta
  let cILenOk : Bool := match cIOpt with | none => true | some c => c.size == aI.size
  let cCLenOk : Bool := match cCOpt with | none => true | some c => c.size == aC.size
  let cIOk : Bool := allFinB aI.size (fun j =>
    decide (gmiIntCoeff aI[j.val] f0 ≤ optCoeffAt cIOpt (gmiIntCoeff aI[j.val] f0) j.val))
  let cCOk : Bool := allFinB aC.size (fun k =>
    decide (gmiContCoeff aC[k.val] f0 ≤ optCoeffAt cCOpt (gmiContCoeff aC[k.val] f0) k.val))
  guardExcB (decide (0 < f0) && decide (f0 < 1)) s!"GMI row beta={beta} has non-fractional f0={f0}" <|
  guardExcB cILenOk s!"cI length mismatch with aI ({aI.size})" <|
  guardExcB cIOk "cI coefficient below exact gmiIntCoeff" <|
  guardExcB cCLenOk s!"cC length mismatch with aC ({aC.size})" <|
  guardExcB cCOk "cC coefficient below exact gmiContCoeff" <|
  .ok ()

private def checkTableauGMIWitnesses
    (aI aC : Array Rat) (beta : Rat)
    (cIOpt cCOpt : Option (Array Rat))
    (solutions : Array (Rat × Array Rat × Array Rat)) : Except String Unit := do
  let f0 := fracPart beta
  let cI := cIOpt.getD (aI.map (fun a => gmiIntCoeff a f0))
  let cC := cCOpt.getD (aC.map (fun g => gmiContCoeff g f0))
  for sIdx in [:solutions.size] do
    let (xB, xI, xC) := solutions[sIdx]!
    if xB.den != 1 then
      throw s!"solution {sIdx}: xB={xB} is not integer"
    if xI.size != aI.size || xC.size != aC.size then
      throw s!"solution {sIdx}: dimension mismatch"
    for j in [:xI.size] do
      let v := xI[j]!
      if v < 0 || v.den != 1 then
        throw s!"solution {sIdx}: xI[{j}]={v} must be non-negative integer"
    for k in [:xC.size] do
      let v := xC[k]!
      if v < 0 then
        throw s!"solution {sIdx}: xC[{k}]={v} must be non-negative"
    let dotI ← evalArrayDot aI xI
    let dotC ← evalArrayDot aC xC
    if xB + dotI + dotC != beta then
      throw s!"solution {sIdx}: tableau equality failed: {xB} + {dotI} + {dotC} != {beta}"
    let cutI ← evalArrayDot cI xI
    let cutC ← evalArrayDot cC xC
    if !(1 ≤ cutI + cutC) then
      throw s!"solution {sIdx}: GMI cut violated: {cutI + cutC} < 1"

/-- Verifies a GMI tableau cut (exact or upward-weakened) algebraically over all feasible
mixed-integer points, plus any optional witness points supplied. -/
def checkTableauGMI
    (aI aC : Array Rat)
    (beta : Rat)
    (cIOpt cCOpt : Option (Array Rat))
    (solutions : Array (Rat × Array Rat × Array Rat)) : Except String Unit :=
  match checkTableauGMIAlg aI aC beta cIOpt cCOpt with
  | .error e => .error e
  | .ok () => checkTableauGMIWitnesses aI aC beta cIOpt cCOpt solutions

/-- Universal soundness of `checkTableauGMI`: whenever `checkTableauGMI` succeeds, the
verified GMI cut holds for EVERY mixed-integer non-negative point satisfying the tableau row. -/
theorem checkTableauGMI_sound
    {aI aC : Array Rat} {beta : Rat}
    {cIOpt cCOpt : Option (Array Rat)}
    {solutions : Array (Rat × Array Rat × Array Rat)}
    (hok : checkTableauGMI aI aC beta cIOpt cCOpt solutions = .ok ())
    {xB : Rat} {xI : Fin aI.size → Rat} {xC : Fin aC.size → Rat}
    (hrow : xB + finSum aI.size (fun j => aI[j.val] * xI j) +
      finSum aC.size (fun k => aC[k.val] * xC k) = beta)
    (hxB : ∃ z : Int, xB = (z : Rat))
    (hxI : ∀ j, 0 ≤ xI j ∧ ∃ z : Int, xI j = (z : Rat))
    (hxC : ∀ k, 0 ≤ xC k) :
    let f0 := fracPart beta
    let cIFn : Fin aI.size → Rat := fun j => optCoeffAt cIOpt (gmiIntCoeff aI[j.val] f0) j.val
    let cCFn : Fin aC.size → Rat := fun k => optCoeffAt cCOpt (gmiContCoeff aC[k.val] f0) k.val
    1 ≤ finSum aI.size (fun j => cIFn j * xI j) + finSum aC.size (fun k => cCFn k * xC k) := by
  have halg : checkTableauGMIAlg aI aC beta cIOpt cCOpt = .ok () := by
    unfold checkTableauGMI at hok
    cases h : checkTableauGMIAlg aI aC beta cIOpt cCOpt with
    | error e => rw [h] at hok; contradiction
    | ok _ => rfl
  unfold checkTableauGMIAlg at halg
  rcases guardExcB_ok halg with ⟨hf0B, halg⟩
  rcases guardExcB_ok halg with ⟨_, halg⟩
  rcases guardExcB_ok halg with ⟨hcIOk, halg⟩
  rcases guardExcB_ok halg with ⟨_, halg⟩
  rcases guardExcB_ok halg with ⟨hcCOk, _⟩
  simp only [Bool.and_eq_true, decide_eq_true_eq] at hf0B
  have hcI : ∀ j : Fin aI.size, gmiIntCoeff aI[j.val] (fracPart beta) ≤
      optCoeffAt cIOpt (gmiIntCoeff aI[j.val] (fracPart beta)) j.val :=
    fun j => of_decide_eq_true (allFinB_eq_true_iff.mp hcIOk j)
  have hcC : ∀ k : Fin aC.size, gmiContCoeff aC[k.val] (fracPart beta) ≤
      optCoeffAt cCOpt (gmiContCoeff aC[k.val] (fracPart beta)) k.val :=
    fun k => of_decide_eq_true (allFinB_eq_true_iff.mp hcCOk k)
  exact gmi_weaken_upward_sound hrow hxB hxI hxC rfl hf0B.1 hf0B.2 hcI hcC

/-- Algebraic core of `checkCMIRCertificate` matching `cmir_safe_rounded_sound`. -/
def checkCMIRCertificateAlg
    (aI aC : Array Rat)
    (lbI ubI lbC ubC : Array (Option Rat))
    (useLowerI useLowerC : Array Bool)
    (rhs delta : Rat)
    (betaHatOpt : Option Rat)
    (alphaHatOpt cIOpt cCOpt : Option (Array Rat))
    (cutRHSOpt : Option Rat)
    (lpPrimalOpt : Option (Array Rat × Array Rat)) : Except String Unit :=
  let mI := aI.size
  let mC := aC.size
  if !(lbI.size == mI && ubI.size == mI && useLowerI.size == mI) then
    .error "cMIR integer dimension mismatch"
  else if !(lbC.size == mC && ubC.size == mC && useLowerC.size == mC) then
    .error "cMIR continuous dimension mismatch"
  else
  let lpOk : Bool := match lpPrimalOpt with
    | none => true
    | some (lpI, lpC) =>
      lpI.size == mI && lpC.size == mC &&
      allFinB mI (fun j =>
        let lOpt := lbI[j.val]!
        let uOpt := ubI[j.val]!
        useLowerI[j.val]! == mirUseLowerSpec lpI[j.val]! (lOpt.getD 0) (uOpt.getD 0) lOpt.isSome uOpt.isSome) &&
      allFinB mC (fun k =>
        let lOpt := lbC[k.val]!
        let uOpt := ubC[k.val]!
        useLowerC[k.val]! == mirUseLowerSpec lpC[k.val]! (lOpt.getD 0) (uOpt.getD 0) lOpt.isSome uOpt.isSome)
  let intBndOk : Bool := allFinB mI fun j =>
    if useLowerI[j.val]! then
      match lbI[j.val]! with | some l => l.den == 1 | none => false
    else
      match ubI[j.val]! with | some u => u.den == 1 | none => false
  let contBndOk : Bool := allFinB mC fun k =>
    if useLowerC[k.val]! then (lbC[k.val]!).isSome else (ubC[k.val]!).isSome
  let bndIFn : Fin mI → Rat := fun j =>
    cmirBound useLowerI[j.val]! (lbI[j.val]!.getD 0) (ubI[j.val]!.getD 0)
  let bndCFn : Fin mC → Rat := fun k =>
    cmirBound useLowerC[k.val]! (lbC[k.val]!.getD 0) (ubC[k.val]!.getD 0)
  let exactBeta : Rat :=
    (rhs - finSum mI (fun j => aI[j.val] * bndIFn j) - finSum mC (fun k => aC[k.val] * bndCFn k)) / delta
  let betaHat : Rat := betaHatOpt.getD exactBeta
  let f0 : Rat := fracPart betaHat
  let alphaLenOk : Bool := match alphaHatOpt with | none => true | some ah => ah.size == mI
  let alphaHatFn : Fin mI → Rat := fun j =>
    optCoeffAt alphaHatOpt (cmirSignedCoeff useLowerI[j.val]! aI[j.val] / delta) j.val
  let alphaOk : Bool := allFinB mI fun j =>
    decide (alphaHatFn j ≤ cmirSignedCoeff useLowerI[j.val]! aI[j.val] / delta)
  let cILenOk : Bool := match cIOpt with | none => true | some ci => ci.size == mI
  let cIFn : Fin mI → Rat := fun j =>
    optCoeffAt cIOpt (mirIntCoeff (alphaHatFn j) f0) j.val
  let cIOk : Bool := allFinB mI fun j =>
    let isFix := match lbI[j.val]!, ubI[j.val]! with | some l, some u => l == u | _, _ => false
    (isFix && cIFn j == 0) || decide (cIFn j ≤ mirIntCoeff (alphaHatFn j) f0)
  let cCLenOk : Bool := match cCOpt with | none => true | some cc => cc.size == mC
  let cCFn : Fin mC → Rat := fun k =>
    optCoeffAt cCOpt (max (-(cmirSignedCoeff useLowerC[k.val]! aC[k.val])) 0 / (delta * (1 - f0))) k.val
  let cCOk : Bool := allFinB mC fun k =>
    decide (max (-(cmirSignedCoeff useLowerC[k.val]! aC[k.val])) 0 / (delta * (1 - f0)) ≤ cCFn k)
  let exactCutRHS : Rat := (betaHat.floor : Rat) +
    finSum mI (fun j => cmirSignedCoeff useLowerI[j.val]! (cIFn j) * bndIFn j) +
    finSum mC (fun k => (if useLowerC[k.val]! then -cCFn k else cCFn k) * bndCFn k)
  let cutRHS : Rat := cutRHSOpt.getD exactCutRHS
  guardExcB (decide (0 < delta)) s!"cMIR delta={delta} must be > 0" <|
  guardExcB lpOk "cMIR lpPrimal / useLower parity mismatch" <|
  guardExcB intBndOk "cMIR integer variable missing integral active bound" <|
  guardExcB contBndOk "cMIR continuous variable missing active bound" <|
  guardExcB (decide (exactBeta ≤ betaHat)) s!"cMIR betaHat={betaHat} < exactBeta={exactBeta}" <|
  guardExcB (alphaLenOk && cILenOk && cCLenOk) "cMIR optional array length mismatch" <|
  guardExcB alphaOk "cMIR alphaHat exceeds exact scaled integer coefficient" <|
  guardExcB cIOk "cMIR cI exceeds mirIntCoeff" <|
  guardExcB cCOk "cMIR cC below exact continuous MIR coefficient" <|
  guardExcB (decide (exactCutRHS ≤ cutRHS)) s!"cutRHS={cutRHS} < exactCutRHS={exactCutRHS}" <|
  .ok ()

private def checkCMIRCertificateWitnesses
    (aI aC : Array Rat)
    (lbI ubI lbC ubC : Array (Option Rat))
    (useLowerI useLowerC : Array Bool)
    (rhs delta : Rat)
    (betaHatOpt : Option Rat)
    (alphaHatOpt cIOpt cCOpt : Option (Array Rat))
    (cutRHSOpt : Option Rat)
    (solutions : Array (Array Rat × Array Rat)) : Except String Unit := do
  let mI := aI.size
  let mC := aC.size
  let bndIFn : Fin mI → Rat := fun j =>
    cmirBound useLowerI[j.val]! (lbI[j.val]!.getD 0) (ubI[j.val]!.getD 0)
  let bndCFn : Fin mC → Rat := fun k =>
    cmirBound useLowerC[k.val]! (lbC[k.val]!.getD 0) (ubC[k.val]!.getD 0)
  let exactBeta : Rat :=
    (rhs - finSum mI (fun j => aI[j.val] * bndIFn j) - finSum mC (fun k => aC[k.val] * bndCFn k)) / delta
  let betaHat : Rat := betaHatOpt.getD exactBeta
  let f0 : Rat := fracPart betaHat
  let alphaHatFn : Fin mI → Rat := fun j =>
    match alphaHatOpt with
    | none => cmirSignedCoeff useLowerI[j.val]! aI[j.val] / delta
    | some ah => ah[j.val]!
  let cIFn : Fin mI → Rat := fun j =>
    match cIOpt with | none => mirIntCoeff (alphaHatFn j) f0 | some ci => ci[j.val]!
  let cCFn : Fin mC → Rat := fun k =>
    match cCOpt with
    | none => max (-(cmirSignedCoeff useLowerC[k.val]! aC[k.val])) 0 / (delta * (1 - f0))
    | some cc => cc[k.val]!
  let exactCutRHS : Rat := (betaHat.floor : Rat) +
    finSum mI (fun j => cmirSignedCoeff useLowerI[j.val]! (cIFn j) * bndIFn j) +
    finSum mC (fun k => (if useLowerC[k.val]! then -cCFn k else cCFn k) * bndCFn k)
  let cutRHS : Rat := cutRHSOpt.getD exactCutRHS
  let origI : Array Rat := Array.ofFn fun (j : Fin mI) => cmirSignedCoeff useLowerI[j.val]! (cIFn j)
  let origC : Array Rat := Array.ofFn fun (k : Fin mC) => if useLowerC[k.val]! then -cCFn k else cCFn k
  for sIdx in [:solutions.size] do
    let (xI, xC) := solutions[sIdx]!
    if xI.size != mI || xC.size != mC then
      throw s!"solution {sIdx}: dimension mismatch"
    for j in [:mI] do
      let v := xI[j]!
      if v.den != 1 then
        throw s!"solution {sIdx}: xI[{j}]={v} not integral"
      if let some l := lbI[j]! then
        if !(l ≤ v) then throw s!"solution {sIdx}: xI[{j}] < lb"
      if let some u := ubI[j]! then
        if !(v ≤ u) then throw s!"solution {sIdx}: xI[{j}] > ub"
    for k in [:mC] do
      let v := xC[k]!
      if let some l := lbC[k]! then
        if !(l ≤ v) then throw s!"solution {sIdx}: xC[{k}] < lb"
      if let some u := ubC[k]! then
        if !(v ≤ u) then throw s!"solution {sIdx}: xC[{k}] > ub"
    let rowLHS := (← evalArrayDot aI xI) + (← evalArrayDot aC xC)
    if !(rowLHS ≤ rhs) then
      throw s!"solution {sIdx}: original row violated: {rowLHS} > {rhs}"
    let cutLHS := (← evalArrayDot origI xI) + (← evalArrayDot origC xC)
    if !(cutLHS ≤ cutRHS) then
      throw s!"solution {sIdx}: cMIR cut violated: {cutLHS} > {cutRHS}"

/-- Verifies a safe-rounded complemented MIR certificate (`cmir_safe_rounded_sound`)
algebraically over all feasible points, plus any optional witness points supplied. -/
def checkCMIRCertificate
    (aI aC : Array Rat)
    (lbI ubI lbC ubC : Array (Option Rat))
    (useLowerI useLowerC : Array Bool)
    (rhs delta : Rat)
    (betaHatOpt : Option Rat)
    (alphaHatOpt cIOpt : Option (Array Rat))
    (cCOpt : Option (Array Rat))
    (cutRHSOpt : Option Rat)
    (lpPrimalOpt : Option (Array Rat × Array Rat))
    (solutions : Array (Array Rat × Array Rat)) : Except String Unit :=
  match checkCMIRCertificateAlg aI aC lbI ubI lbC ubC useLowerI useLowerC rhs delta
      betaHatOpt alphaHatOpt cIOpt cCOpt cutRHSOpt lpPrimalOpt with
  | .error e => .error e
  | .ok () =>
    checkCMIRCertificateWitnesses aI aC lbI ubI lbC ubC useLowerI useLowerC rhs delta
      betaHatOpt alphaHatOpt cIOpt cCOpt cutRHSOpt solutions

/-- Universal soundness of `checkCMIRCertificate`: whenever `checkCMIRCertificate` succeeds,
the verified complemented safe-rounded MIR cut holds for EVERY mixed-integer point satisfying
the optional variable bounds and the source inequality. -/
theorem checkCMIRCertificate_sound
    {aI aC : Array Rat}
    {lbI ubI lbC ubC : Array (Option Rat)}
    {useLowerI useLowerC : Array Bool}
    {rhs delta : Rat}
    {betaHatOpt : Option Rat}
    {alphaHatOpt cIOpt cCOpt : Option (Array Rat)}
    {cutRHSOpt : Option Rat}
    {lpPrimalOpt : Option (Array Rat × Array Rat)}
    {solutions : Array (Array Rat × Array Rat)}
    (hok : checkCMIRCertificate aI aC lbI ubI lbC ubC useLowerI useLowerC rhs delta
      betaHatOpt alphaHatOpt cIOpt cCOpt cutRHSOpt lpPrimalOpt solutions = .ok ())
    {xI : Fin aI.size → Rat} {xC : Fin aC.size → Rat}
    (hxI : ∀ j : Fin aI.size, (∃ z : Int, xI j = (z : Rat)) ∧
      (∀ l, lbI[j.val]! = some l → l ≤ xI j) ∧ (∀ u, ubI[j.val]! = some u → xI j ≤ u))
    (hxC : ∀ k : Fin aC.size,
      (∀ l, lbC[k.val]! = some l → l ≤ xC k) ∧ (∀ u, ubC[k.val]! = some u → xC k ≤ u))
    (hrow : finSum aI.size (fun j => aI[j.val] * xI j) +
      finSum aC.size (fun k => aC[k.val] * xC k) ≤ rhs) :
    let bndIFn : Fin aI.size → Rat := fun j =>
      cmirBound useLowerI[j.val]! (lbI[j.val]!.getD 0) (ubI[j.val]!.getD 0)
    let bndCFn : Fin aC.size → Rat := fun k =>
      cmirBound useLowerC[k.val]! (lbC[k.val]!.getD 0) (ubC[k.val]!.getD 0)
    let exactBeta : Rat :=
      (rhs - finSum aI.size (fun j => aI[j.val] * bndIFn j) - finSum aC.size (fun k => aC[k.val] * bndCFn k)) / delta
    let betaHat : Rat := betaHatOpt.getD exactBeta
    let f0 : Rat := fracPart betaHat
    let alphaHatFn : Fin aI.size → Rat := fun j =>
      optCoeffAt alphaHatOpt (cmirSignedCoeff useLowerI[j.val]! aI[j.val] / delta) j.val
    let cIFn : Fin aI.size → Rat := fun j =>
      optCoeffAt cIOpt (mirIntCoeff (alphaHatFn j) f0) j.val
    let cCFn : Fin aC.size → Rat := fun k =>
      optCoeffAt cCOpt (max (-(cmirSignedCoeff useLowerC[k.val]! aC[k.val])) 0 / (delta * (1 - f0))) k.val
    let exactCutRHS : Rat := (betaHat.floor : Rat) +
      finSum aI.size (fun j => cmirSignedCoeff useLowerI[j.val]! (cIFn j) * bndIFn j) +
      finSum aC.size (fun k => (if useLowerC[k.val]! then -cCFn k else cCFn k) * bndCFn k)
    let cutRHS : Rat := cutRHSOpt.getD exactCutRHS
    finSum aI.size (fun j => cmirSignedCoeff useLowerI[j.val]! (cIFn j) * xI j) +
      finSum aC.size (fun k => (if useLowerC[k.val]! then -cCFn k else cCFn k) * xC k) ≤ cutRHS := by
  have halg : checkCMIRCertificateAlg aI aC lbI ubI lbC ubC useLowerI useLowerC rhs delta
      betaHatOpt alphaHatOpt cIOpt cCOpt cutRHSOpt lpPrimalOpt = .ok () := by
    unfold checkCMIRCertificate at hok
    cases h : checkCMIRCertificateAlg aI aC lbI ubI lbC ubC useLowerI useLowerC rhs delta
        betaHatOpt alphaHatOpt cIOpt cCOpt cutRHSOpt lpPrimalOpt with
    | error e => rw [h] at hok; contradiction
    | ok _ => rfl
  unfold checkCMIRCertificateAlg at halg
  rcases ite_not_error_okB halg with ⟨_, halg⟩
  rcases ite_not_error_okB halg with ⟨_, halg⟩
  rcases guardExcB_ok halg with ⟨hdeltaD, halg⟩
  rcases guardExcB_ok halg with ⟨_, halg⟩
  rcases guardExcB_ok halg with ⟨hintBndOk, halg⟩
  rcases guardExcB_ok halg with ⟨hcontBndOk, halg⟩
  rcases guardExcB_ok halg with ⟨hbetaD, halg⟩
  rcases guardExcB_ok halg with ⟨_, halg⟩
  rcases guardExcB_ok halg with ⟨halphaOk, halg⟩
  rcases guardExcB_ok halg with ⟨hcIOk, halg⟩
  rcases guardExcB_ok halg with ⟨hcCOk, halg⟩
  rcases guardExcB_ok halg with ⟨hcutRHSD, _⟩
  have hdeltaPos : 0 < delta := of_decide_eq_true hdeltaD
  have hintBndAll := allFinB_eq_true_iff.mp hintBndOk
  have hcontBndAll := allFinB_eq_true_iff.mp hcontBndOk
  have halphaAll := allFinB_eq_true_iff.mp halphaOk
  have hcIAll := allFinB_eq_true_iff.mp hcIOk
  have hcCAll := allFinB_eq_true_iff.mp hcCOk
  let lIFn : Fin aI.size → Rat := fun j => (lbI[j.val]!).getD 0
  let uIFn : Fin aI.size → Rat := fun j => (ubI[j.val]!).getD 0
  let hasLI : Fin aI.size → Bool := fun j => (lbI[j.val]!).isSome
  let hasUI : Fin aI.size → Bool := fun j => (ubI[j.val]!).isSome
  let lCFn : Fin aC.size → Rat := fun k => (lbC[k.val]!).getD 0
  let uCFn : Fin aC.size → Rat := fun k => (ubC[k.val]!).getD 0
  let hasLC : Fin aC.size → Bool := fun k => (lbC[k.val]!).isSome
  let hasUC : Fin aC.size → Bool := fun k => (ubC[k.val]!).isSome
  have hxI' : ∀ j : Fin aI.size, (∃ z : Int, xI j = (z : Rat)) ∧
      (hasLI j = true → lIFn j ≤ xI j) ∧ (hasUI j = true → xI j ≤ uIFn j) ∧
      (if useLowerI[j.val]! then hasLI j = true ∧ ∃ z : Int, lIFn j = (z : Rat)
       else hasUI j = true ∧ ∃ z : Int, uIFn j = (z : Rat)) := by
    intro j
    rcases hxI j with ⟨hzx, hL, hU⟩
    refine ⟨hzx, ?_, ?_, ?_⟩
    · intro hhas
      cases hlb : lbI[j.val]! with
      | none => simp [hasLI, hlb] at hhas
      | some l => simp only [lIFn, hlb, Option.getD_some]; exact hL l hlb
    · intro hhas
      cases hub : ubI[j.val]! with
      | none => simp [hasUI, hub] at hhas
      | some u => simp only [uIFn, hub, Option.getD_some]; exact hU u hub
    · have hb := hintBndAll j
      cases hu : useLowerI[j.val]!
      · simp only [hu, Bool.false_eq_true, ↓reduceIte] at hb ⊢
        cases hub : ubI[j.val]! with
        | none => simp [hub] at hb
        | some u =>
          simp only [hub, beq_iff_eq] at hb
          exact ⟨by simp [hasUI, hub], ⟨u.num, by simp only [uIFn, hub, Option.getD_some]; exact rat_eq_num_of_den_one hb⟩⟩
      · simp only [hu, ↓reduceIte] at hb ⊢
        cases hlb : lbI[j.val]! with
        | none => simp [hlb] at hb
        | some l =>
          simp only [hlb, beq_iff_eq] at hb
          exact ⟨by simp [hasLI, hlb], ⟨l.num, by simp only [lIFn, hlb, Option.getD_some]; exact rat_eq_num_of_den_one hb⟩⟩
  have hxC' : ∀ k : Fin aC.size, (hasLC k = true → lCFn k ≤ xC k) ∧ (hasUC k = true → xC k ≤ uCFn k) ∧
      (if useLowerC[k.val]! then hasLC k = true else hasUC k = true) := by
    intro k
    rcases hxC k with ⟨hL, hU⟩
    refine ⟨?_, ?_, ?_⟩
    · intro hhas
      cases hlb : lbC[k.val]! with
      | none => simp [hasLC, hlb] at hhas
      | some l => simp only [lCFn, hlb, Option.getD_some]; exact hL l hlb
    · intro hhas
      cases hub : ubC[k.val]! with
      | none => simp [hasUC, hub] at hhas
      | some u => simp only [uCFn, hub, Option.getD_some]; exact hU u hub
    · have hb := hcontBndAll k
      cases hu : useLowerC[k.val]! <;> simp [hasLC, hasUC, hu] at hb ⊢ <;> exact hb
  let exactBeta : Rat :=
    cmirShiftedRHS (fun j => aI[j.val]) lIFn uIFn (fun j => useLowerI[j.val]!)
      (fun k => aC[k.val]) lCFn uCFn (fun k => useLowerC[k.val]!) rhs / delta
  let betaHat : Rat := betaHatOpt.getD exactBeta
  let alphaHatFn : Fin aI.size → Rat := fun j =>
    optCoeffAt alphaHatOpt (cmirSignedCoeff useLowerI[j.val]! aI[j.val] / delta) j.val
  let cIFn : Fin aI.size → Rat := fun j =>
    optCoeffAt cIOpt (mirIntCoeff (alphaHatFn j) (fracPart betaHat)) j.val
  let cCFn : Fin aC.size → Rat := fun k =>
    optCoeffAt cCOpt (max (-(cmirSignedCoeff useLowerC[k.val]! aC[k.val])) 0 / (delta * (1 - fracPart betaHat))) k.val
  have hcI' : ∀ j : Fin aI.size,
      (hasLI j = true ∧ hasUI j = true ∧ lIFn j = uIFn j ∧ cIFn j = 0) ∨
      cIFn j ≤ mirIntCoeff (alphaHatFn j) (fracPart betaHat) := by
    intro j
    have hj := hcIAll j
    dsimp only at hj
    simp only [Bool.or_eq_true, Bool.and_eq_true, beq_iff_eq, decide_eq_true_eq] at hj
    rcases hj with ⟨hfix, hc0⟩ | hle
    · left
      cases hlb : lbI[j.val]! with
      | none => simp [hlb] at hfix
      | some l =>
        cases hub : ubI[j.val]! with
        | none => simp [hlb, hub] at hfix
        | some u =>
          simp only [hlb, hub, beq_iff_eq] at hfix
          exact ⟨by simp [hasLI, hlb], by simp [hasUI, hub], by simp [lIFn, uIFn, hlb, hub, hfix], hc0⟩
    · right; exact hle
  exact cmir_safe_rounded_sound
    (aI := fun j => aI[j.val]) (xI := xI) (lI := lIFn) (uI := uIFn)
    (hasLI := hasLI) (hasUI := hasUI) (useLowerI := fun j => useLowerI[j.val]!)
    (aC := fun k => aC[k.val]) (xC := xC) (lC := lCFn) (uC := uCFn)
    (hasLC := hasLC) (hasUC := hasUC) (useLowerC := fun k => useLowerC[k.val]!)
    (rhs := rhs) (delta := delta) (betaHat := betaHat)
    (alphaHat := alphaHatFn) (cI := cIFn) (cC := cCFn)
    hdeltaPos hxI' hxC' hrow (of_decide_eq_true hbetaD)
    (fun j => of_decide_eq_true (halphaAll j)) hcI'
    (fun k => of_decide_eq_true (hcCAll k)) (of_decide_eq_true hcutRHSD)

/-- Algebraic core of `checkVUBDisagg` matching `vub_disagg_opt_sound`. -/
def checkVUBDisaggAlg
    (a : Array Rat) (u : Array (Option Rat))
    (cap rhsRow : Rat) (targetIdx : Nat) (effUB cutRHS : Rat) : Except String Unit :=
  if !(a.size == u.size) then
    .error s!"VUB arrays a ({a.size}) and u ({u.size}) mismatch"
  else if !(decide (targetIdx < a.size)) then
    .error s!"targetIdx {targetIdx} out of bounds ({a.size})"
  else
  let aPosOk : Bool := allFinB a.size (fun j => decide (0 < a[j.val]))
  guardExcB (decide (0 ≤ cap)) s!"VUB capacity C={cap} must be non-negative" <|
  guardExcB (decide (rhsRow ≤ 0)) s!"VUB row RHS={rhsRow} must be ≤ 0" <|
  guardExcB aPosOk "VUB coefficients must all be > 0" <|
  guardExcB (decide (vubEffUB u[targetIdx]! cap a[targetIdx]! ≤ effUB))
    s!"violated min bound: effUB={effUB} < min(u, C/a)={vubEffUB u[targetIdx]! cap a[targetIdx]!}" <|
  guardExcB (decide (0 ≤ cutRHS)) s!"VUB cutRHS={cutRHS} must be ≥ 0" <|
  .ok ()

private def checkVUBDisaggWitnesses
    (a : Array Rat) (u : Array (Option Rat))
    (cap rhsRow : Rat) (targetIdx : Nat) (effUB cutRHS : Rat)
    (solutions : Array (Array Rat × Rat)) : Except String Unit := do
  for sIdx in [:solutions.size] do
    let (x, y) := solutions[sIdx]!
    if x.size != a.size then
      throw s!"solution {sIdx}: x length {x.size} != {a.size}"
    if !(y == 0 || y == 1) then
      throw s!"solution {sIdx}: y={y} must be binary"
    for j in [:x.size] do
      let xj := x[j]!
      if !(0 ≤ xj) then
        throw s!"solution {sIdx}: x[{j}]={xj} < 0"
      match u[j]! with
      | some uj =>
        if !(xj ≤ uj) then
          throw s!"solution {sIdx}: x[{j}]={xj} > u[{j}]={uj}"
      | none => pure ()
    let ax ← evalArrayDot a x
    if !(ax - cap * y ≤ rhsRow) then
      throw s!"solution {sIdx}: VUB row violated: {ax - cap * y} > {rhsRow}"
    if !(x[targetIdx]! - effUB * y ≤ cutRHS) then
      throw s!"solution {sIdx}: disaggregated VUB cut violated: {x[targetIdx]! - effUB * y} > {cutRHS}"

/-- Verifies a VUB disaggregation certificate (`vub_disagg_opt_sound`) algebraically over all
feasible points, plus any optional witness points supplied. -/
def checkVUBDisagg
    (a : Array Rat)
    (u : Array (Option Rat))
    (cap rhsRow : Rat)
    (targetIdx : Nat)
    (effUB cutRHS : Rat)
    (solutions : Array (Array Rat × Rat)) : Except String Unit :=
  match checkVUBDisaggAlg a u cap rhsRow targetIdx effUB cutRHS with
  | .error e => .error e
  | .ok () => checkVUBDisaggWitnesses a u cap rhsRow targetIdx effUB cutRHS solutions

/-- Universal soundness of `checkVUBDisagg`: whenever `checkVUBDisagg` succeeds, the
disaggregated VUB inequality `x[targetIdx] - effUB * y ≤ cutRHS` holds for EVERY feasible `(x, y)`. -/
theorem checkVUBDisagg_sound
    {a : Array Rat} {u : Array (Option Rat)}
    {cap rhsRow : Rat} {targetIdx : Nat} {effUB cutRHS : Rat}
    {solutions : Array (Array Rat × Rat)}
    (hok : checkVUBDisagg a u cap rhsRow targetIdx effUB cutRHS solutions = .ok ())
    {x : Fin a.size → Rat} {y : Rat}
    (hx : ∀ j, 0 ≤ x j)
    (hu : ∀ j uj, u[j.val]! = some uj → x j ≤ uj)
    (hy : y = 0 ∨ y = 1)
    (hrow : finSum a.size (fun j => a[j.val] * x j) - cap * y ≤ rhsRow) :
    ∃ ht : targetIdx < a.size, x ⟨targetIdx, ht⟩ - effUB * y ≤ cutRHS := by
  have halg : checkVUBDisaggAlg a u cap rhsRow targetIdx effUB cutRHS = .ok () := by
    unfold checkVUBDisagg at hok
    cases h : checkVUBDisaggAlg a u cap rhsRow targetIdx effUB cutRHS with
    | error e => rw [h] at hok; contradiction
    | ok _ => rfl
  unfold checkVUBDisaggAlg at halg
  rcases ite_not_error_okB halg with ⟨_, halg⟩
  rcases ite_not_error_okB halg with ⟨htD, halg⟩
  rcases guardExcB_ok halg with ⟨hcapD, halg⟩
  rcases guardExcB_ok halg with ⟨hrhsD, halg⟩
  rcases guardExcB_ok halg with ⟨haPosOk, halg⟩
  rcases guardExcB_ok halg with ⟨heffD, halg⟩
  rcases guardExcB_ok halg with ⟨hcutRHSD, _⟩
  have ht : targetIdx < a.size := of_decide_eq_true htD
  have haPos : ∀ j : Fin a.size, 0 < a[j.val] := fun j => of_decide_eq_true (allFinB_eq_true_iff.mp haPosOk j)
  have heff : vubEffUB u[targetIdx]! cap a[targetIdx] ≤ effUB := by
    have h := of_decide_eq_true heffD
    rwa [getElem!_pos a targetIdx ht] at h
  have hvub := vub_disagg_opt_sound hrow (of_decide_eq_true hrhsD) haPos hx hu
    (of_decide_eq_true hcapD) hy ⟨targetIdx, ht⟩ heff
  have hsub : x ⟨targetIdx, ht⟩ - effUB * y ≤ 0 := by
    have h := (Rat.add_le_add_right (c := -(effUB * y))).mpr hvub
    simpa [Rat.sub_eq_add_neg, Rat.add_neg_cancel] using h
  exact ⟨ht, Rat.le_trans hsub (of_decide_eq_true hcutRHSD)⟩

/-- Algebraic core of `checkRowAgg`: verifies multiplier sign legality on all rows AND, when
`aggCutOpt = some cut`, algebraically verifies exact coefficient matching across every
mentioned column plus `∑ mu_i * rhs_i ≤ cut.rhs`. -/
def checkRowAggAlg
    (rows : Array (RowSpec × Rat))
    (allowZero : Bool)
    (aggCutOpt : Option CutSpec) : Except String Unit :=
  let m := rows.size
  let muOk : Bool := allFinB m fun i =>
    let (row, mu) := rows[i.val]
    (allowZero && mu == 0) || rowLegalMuSignSpec row.sense mu
  let cutOk : Bool := match aggCutOpt with
    | none => true
    | some cut =>
      let exactRHS : Rat := finSum m (fun i => rows[i.val].2 * rows[i.val].1.rhs)
      let rowColsOk : Bool := allFinB m fun i =>
        let terms := rows[i.val].1.terms
        allFinB terms.size fun k =>
          let c := terms[k.val].col
          decide (finSum m (fun r => rows[r.val].2 * sparseTermsCoeff rows[r.val].1.terms.toList c) =
            sparseTermsCoeff cut.terms.toList c)
      let cutColsOk : Bool := allFinB cut.terms.size fun k =>
        let c := cut.terms[k.val].col
        decide (finSum m (fun r => rows[r.val].2 * sparseTermsCoeff rows[r.val].1.terms.toList c) =
          sparseTermsCoeff cut.terms.toList c)
      rowColsOk && cutColsOk && decide (exactRHS ≤ cut.rhs)
  guardExcB (!rows.isEmpty) "row aggregation verification requires at least one row" <|
  guardExcB muOk "row aggregation has illegal multiplier sign for constraint sense" <|
  guardExcB cutOk "aggregated cut coefficients or RHS do not match algebraic linear combination" <|
  .ok ()

private def checkRowAggWitnesses
    (rows : Array (RowSpec × Rat))
    (aggCutOpt : Option CutSpec)
    (solutions : Array (Array Rat)) : Except String Unit := do
  for sIdx in [:solutions.size] do
    let x := solutions[sIdx]!
    let mut aggLHS : Rat := 0
    let mut aggRHS : Rat := 0
    for rIdx in [:rows.size] do
      let (row, mu) := rows[rIdx]!
      let lhs ← evalTerms row.terms x
      if !satisfiesSenseBool lhs row.sense row.rhs then
        throw s!"solution {sIdx}, row {rIdx}: constraint violated (lhs={lhs}, rhs={row.rhs})"
      aggLHS := aggLHS + mu * lhs
      aggRHS := aggRHS + mu * row.rhs
    if !(aggLHS ≤ aggRHS) then
      throw s!"solution {sIdx}: aggregated inequality violated: {aggLHS} > {aggRHS}"
    if let some cut := aggCutOpt then
      let ok ← checkCutAtPoint cut x
      if !ok then
        let clhs ← evalTerms cut.terms x
        throw s!"solution {sIdx}: aggregated cut violated: {clhs} > {cut.rhs}"

/-- Verifies row aggregation multiplier legality (`rowLegalMuSignSpec`) and exact algebraic
aggregation of `aggCutOpt` over all feasible points, plus any optional witness points supplied. -/
def checkRowAgg
    (rows : Array (RowSpec × Rat))
    (allowZero : Bool)
    (aggCutOpt : Option CutSpec)
    (solutions : Array (Array Rat)) : Except String Unit :=
  match checkRowAggAlg rows allowZero aggCutOpt with
  | .error e => .error e
  | .ok () => checkRowAggWitnesses rows aggCutOpt solutions

/-- Universal soundness of `checkRowAgg`: whenever `checkRowAgg` succeeds, BOTH the exact
linear combination `∑ mu_i * row_i ≤ ∑ mu_i * rhs_i` AND (when provided) `aggCut` hold for
EVERY point `x : Fin n → Rat` satisfying all source rows. -/
theorem checkRowAgg_sound
    {rows : Array (RowSpec × Rat)} {allowZero : Bool}
    {aggCutOpt : Option CutSpec} {solutions : Array (Array Rat)}
    (hok : checkRowAgg rows allowZero aggCutOpt solutions = .ok ())
    {n : Nat} {x : Fin n → Rat}
    (hRowCols : ∀ i : Fin rows.size, ∀ t ∈ rows[i.val].1.terms.toList, t.col < n)
    (hRowSat : ∀ i : Fin rows.size,
      satisfiesSense (evalSparseTermsFin n rows[i.val].1.terms.toList x) rows[i.val].1.sense rows[i.val].1.rhs) :
    finSum rows.size (fun i => rows[i.val].2 * evalSparseTermsFin n rows[i.val].1.terms.toList x) ≤
      finSum rows.size (fun i => rows[i.val].2 * rows[i.val].1.rhs) ∧
    (∀ cut, aggCutOpt = some cut → (∀ t ∈ cut.terms.toList, t.col < n) →
      evalSparseTermsFin n cut.terms.toList x ≤ cut.rhs) := by
  have halg : checkRowAggAlg rows allowZero aggCutOpt = .ok () := by
    unfold checkRowAgg at hok
    cases h : checkRowAggAlg rows allowZero aggCutOpt with
    | error e => rw [h] at hok; contradiction
    | ok _ => rfl
  unfold checkRowAggAlg at halg
  rcases guardExcB_ok halg with ⟨_, halg⟩
  rcases guardExcB_ok halg with ⟨hmuOk, halg⟩
  rcases guardExcB_ok halg with ⟨hcutOk, _⟩
  have hmuAll := allFinB_eq_true_iff.mp hmuOk
  have hmu : ∀ i : Fin rows.size,
      rows[i.val].2 = 0 ∨ rowLegalMuSignSpec rows[i.val].1.sense rows[i.val].2 = true := by
    intro i
    have hi := hmuAll i
    dsimp only at hi
    simp only [Bool.or_eq_true, Bool.and_eq_true, beq_iff_eq] at hi
    rcases hi with ⟨_, h0⟩ | hleg
    · exact Or.inl h0
    · exact Or.inr hleg
  have hsatFin : ∀ i : Fin rows.size,
      satisfiesSense (finSum n (fun j => sparseTermsCoeff rows[i.val].1.terms.toList j.val * x j))
        rows[i.val].1.sense rows[i.val].1.rhs := by
    intro i
    have h := hRowSat i
    rwa [evalSparseTermsFin_eq_finSum _ x (hRowCols i)] at h
  have hagg := row_agg_sum_sound hmu hsatFin
  have hswapLhs :
      finSum rows.size (fun i => rows[i.val].2 * evalSparseTermsFin n rows[i.val].1.terms.toList x) =
      finSum n (fun j => finSum rows.size (fun i => rows[i.val].2 * sparseTermsCoeff rows[i.val].1.terms.toList j.val) * x j) := by
    have h1 :
        finSum rows.size (fun i => rows[i.val].2 * evalSparseTermsFin n rows[i.val].1.terms.toList x) =
        finSum rows.size (fun i => finSum n (fun j => (rows[i.val].2 * sparseTermsCoeff rows[i.val].1.terms.toList j.val) * x j)) := by
      apply finSum_congr
      intro i
      rw [evalSparseTermsFin_eq_finSum _ x (hRowCols i), ← finSum_const_mul]
      exact finSum_congr fun j => (Rat.mul_assoc _ _ _).symm
    rw [h1, finSum_swap]
    exact finSum_congr fun j => by rw [finSum_mul_const]
  refine ⟨by rwa [hswapLhs], ?_⟩
  intro cut hcutEq hcutCols
  rw [hcutEq] at hcutOk
  simp only [Bool.and_eq_true, decide_eq_true_eq] at hcutOk
  rcases hcutOk with ⟨⟨hrowColsOk, hcutColsOk⟩, hrhsLe⟩
  have hrowColsAll := allFinB_eq_true_iff.mp hrowColsOk
  have hcutColsAll := allFinB_eq_true_iff.mp hcutColsOk
  have hCoeffAll : ∀ c : Nat,
      finSum rows.size (fun r => rows[r.val].2 * sparseTermsCoeff rows[r.val].1.terms.toList c) =
      sparseTermsCoeff cut.terms.toList c := by
    intro c
    by_cases hInCut : ∃ t ∈ cut.terms.toList, t.col = c
    · rcases hInCut with ⟨t, htMem, rfl⟩
      rw [List.mem_iff_getElem] at htMem
      rcases htMem with ⟨k, hk, rfl⟩
      have hk' : k < cut.terms.size := by simpa using hk
      exact of_decide_eq_true (hcutColsAll ⟨k, hk'⟩)
    · by_cases hInRow : ∃ i : Fin rows.size, ∃ t ∈ rows[i.val].1.terms.toList, t.col = c
      · rcases hInRow with ⟨i, t, htMem, rfl⟩
        rw [List.mem_iff_getElem] at htMem
        rcases htMem with ⟨k, hk, rfl⟩
        have hk' : k < rows[i.val].1.terms.size := by simpa using hk
        exact of_decide_eq_true (allFinB_eq_true_iff.mp (hrowColsAll i) ⟨k, hk'⟩)
      · have hCut0 : sparseTermsCoeff cut.terms.toList c = 0 :=
          sparseTermsCoeff_not_mem (fun t ht heq => hInCut ⟨t, ht, heq⟩)
        have hRows0 : ∀ r : Fin rows.size,
            rows[r.val].2 * sparseTermsCoeff rows[r.val].1.terms.toList c = 0 := by
          intro r
          have hr0 : sparseTermsCoeff rows[r.val].1.terms.toList c = 0 :=
            sparseTermsCoeff_not_mem (fun t ht heq => hInRow ⟨r, t, ht, heq⟩)
          rw [hr0, Rat.mul_zero]
        rw [finSum_congr hRows0, finSum_zero, hCut0]
  rw [evalSparseTermsFin_eq_finSum cut.terms.toList x hcutCols]
  have hmatch : finSum n (fun j => sparseTermsCoeff cut.terms.toList j.val * x j) =
      finSum n (fun j => finSum rows.size (fun r => rows[r.val].2 * sparseTermsCoeff rows[r.val].1.terms.toList j.val) * x j) :=
    finSum_congr fun j => by rw [hCoeffAll j.val]
  rw [hmatch]
  exact Rat.le_trans hagg hrhsLe

end GoblinFormal.Track2B
