/-
GoblinFormal.Track2B: Cutting-plane soundness for GMI, row aggregation,
single-row cMIR (including complementation and outward safe rounding),
and variable-upper-bound (VUB) disaggregation.
-/

import GoblinFormal.Common.Basic
import GoblinFormal.Common.Json

namespace GoblinFormal.Track2B

/-! ### Basic Rat and finSum helpers -/

private theorem rat_lt_of_lt_of_le {a b c : Rat} (hab : a < b) (hbc : b ≤ c) : a < c :=
  Rat.lt_iff_le_and_not_ge.mpr
    ⟨Rat.le_trans (Rat.le_of_lt hab) hbc, fun h => Rat.not_le.mpr hab (Rat.le_trans hbc h)⟩

private theorem rat_neg_nonneg {a : Rat} (ha : a ≤ 0) : 0 ≤ -a := by
  simpa using Rat.neg_le_neg ha

private theorem rat_sub_zero (a : Rat) : a - 0 = a := by
  rw [Rat.sub_eq_add_neg, Rat.neg_zero, Rat.add_zero]

private theorem rat_zero_sub (a : Rat) : 0 - a = -a := by
  rw [Rat.sub_eq_add_neg, Rat.zero_add]

private theorem rat_zero_div (a : Rat) : 0 / a = 0 := by
  rw [Rat.div_def, Rat.zero_mul]

private theorem rat_neg_div (a b : Rat) : (-a) / b = -(a / b) := by
  rw [Rat.div_def, Rat.div_def, Rat.neg_mul]

private theorem rat_sub_mul (a b c : Rat) : (a - b) * c = a * c - b * c := by
  rw [Rat.sub_eq_add_neg, Rat.add_mul, Rat.neg_mul, ← Rat.sub_eq_add_neg]

private theorem rat_mul_sub (a b c : Rat) : a * (b - c) = a * b - a * c := by
  rw [Rat.sub_eq_add_neg, Rat.mul_add, Rat.mul_neg, ← Rat.sub_eq_add_neg]

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

private theorem rat_div_nonneg {a b : Rat} (ha : 0 ≤ a) (hb : 0 ≤ b) : 0 ≤ a / b := by
  rw [Rat.div_def]
  by_cases hb0 : b = 0
  · subst hb0
    simp
  · have hbpos : 0 < b := Rat.lt_of_le_of_ne hb (Ne.symm hb0)
    exact Rat.mul_nonneg ha (Rat.le_of_lt (Rat.inv_pos.mpr hbpos))

private theorem rat_div_le_iff {a b c : Rat} (hb : 0 < b) : a / b ≤ c ↔ a ≤ c * b := by
  constructor
  · intro h
    have h1 := Rat.mul_le_mul_of_nonneg_right h (Rat.le_of_lt hb)
    rwa [Rat.div_mul_cancel (Rat.ne_of_gt hb)] at h1
  · intro h
    apply Rat.le_of_mul_le_mul_right _ hb
    rwa [Rat.div_mul_cancel (Rat.ne_of_gt hb)]

private theorem rat_le_div_iff {a b c : Rat} (hc : 0 < c) : a ≤ b / c ↔ a * c ≤ b := by
  constructor
  · intro h
    have h1 := Rat.mul_le_mul_of_nonneg_right h (Rat.le_of_lt hc)
    rwa [Rat.div_mul_cancel (Rat.ne_of_gt hc)] at h1
  · intro h
    apply Rat.le_of_mul_le_mul_right _ hc
    rwa [Rat.div_mul_cancel (Rat.ne_of_gt hc)]

private theorem rat_div_le_div_of_nonneg {a b c : Rat} (hab : a ≤ b) (hc : 0 < c) :
    a / c ≤ b / c := by
  rw [rat_div_le_iff hc, Rat.div_mul_cancel (Rat.ne_of_gt hc)]
  exact hab

private theorem rat_add_div (a b c : Rat) : (a + b) / c = a / c + b / c := by
  simp [Rat.div_def, Rat.add_mul]

private theorem rat_sub_div (a b c : Rat) : (a - b) / c = a / c - b / c := by
  simp [Rat.div_def, Rat.sub_eq_add_neg, Rat.add_mul, Rat.neg_mul]

private theorem rat_div_self {a : Rat} (ha : a ≠ 0) : a / a = 1 := by
  rw [Rat.div_def, Rat.mul_inv_cancel _ ha]

private theorem rat_le_min_iff {a b c : Rat} : a ≤ min b c ↔ a ≤ b ∧ a ≤ c := by
  rw [Rat.min_def]
  split
  · next h => exact ⟨fun h1 => ⟨h1, Rat.le_trans h1 h⟩, fun h1 => h1.1⟩
  · next h =>
    have hlt : c ≤ b := Rat.le_of_lt (Rat.not_le.mp h)
    exact ⟨fun h1 => ⟨Rat.le_trans h1 hlt, h1⟩, fun h1 => h1.2⟩

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
      exact rat_div_le_div_of_nonneg hSpos_ge hf0_pos
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
      exact rat_div_le_div_of_nonneg hSneg_ge h1mf0_pos
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
      apply rat_div_le_div_of_nonneg _ hdelta
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
      have hd := rat_div_le_div_of_nonneg hshifted_le hdelta
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

/-- Soundness of dropping small cut coefficients via bounds and scaling by `s > 0` (`cuts.go:finalizeSparseCut`). -/
theorem cut_drop_scale_sound {n : Nat} {v kept lb ub x : Fin n → Rat}
    {rhs droppedRHS s cutRHS : Rat}
    (hcut : finSum n (fun j => v j * x j) ≤ rhs)
    (_hxbnd : ∀ j, lb j ≤ x j ∧ x j ≤ ub j)
    (hdrop : ∀ j, kept j = v j ∨ (kept j = 0 ∧ min (v j * lb j) (v j * ub j) ≤ v j * x j))
    (hdropRHS : rhs - finSum n (fun j => if kept j = v j then 0 else min (v j * lb j) (v j * ub j)) ≤ droppedRHS)
    (hs : 0 < s)
    (hscale : s * droppedRHS ≤ cutRHS) :
    finSum n (fun j => (s * kept j) * x j) ≤ cutRHS := by
  have hkept : ∀ j, kept j * x j ≤ v j * x j - (if kept j = v j then 0 else min (v j * lb j) (v j * ub j)) := by
    intro j
    by_cases hkv : kept j = v j
    · simp [hkv, rat_sub_zero]
    · rcases hdrop j with hkv' | ⟨hk0, hmin⟩
      · exact absurd hkv' hkv
      · have hite : (if kept j = v j then (0 : Rat) else min (v j * lb j) (v j * ub j)) =
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

/-! ### 5. Executable Checkers for `cuts-check` CLI -/

/-- Sparse linear term `(col, val)`. -/
structure SparseTerm where
  col : Nat
  val : Rat
  deriving Repr, Inhabited

/-- Sparse linear cut `∑ val * x[col] ≤ rhs`. -/
structure CutSpec where
  terms : Array SparseTerm
  rhs : Rat
  deriving Repr, Inhabited

/-- Sparse linear constraint row. -/
structure RowSpec where
  terms : Array SparseTerm
  sense : ConstraintSense
  rhs : Rat
  deriving Repr, Inhabited

/-- Mixed-integer linear program specification for structural cut checks. -/
structure MIPSpec where
  numVars : Nat
  isInt : Array Bool
  lb : Array (Option Rat)
  ub : Array (Option Rat)
  rows : Array RowSpec
  deriving Repr, Inhabited

def evalTerms (terms : Array SparseTerm) (x : Array Rat) : Except String Rat := do
  let mut acc : Rat := 0
  for t in terms do
    if h : t.col < x.size then
      acc := acc + t.val * x[t.col]
    else
      throw s!"column index {t.col} out of bounds (numVars={x.size})"
  pure acc

def evalArrayDot (a x : Array Rat) : Except String Rat := do
  if a.size != x.size then
    throw s!"vector length mismatch: {a.size} vs {x.size}"
  let mut acc : Rat := 0
  for i in [:a.size] do
    if _h : i < a.size then
      acc := acc + a[i]! * x[i]!
  pure acc

def satisfiesSenseBool (lhs : Rat) (sense : ConstraintSense) (rhs : Rat) : Bool :=
  match sense with
  | .le => decide (lhs ≤ rhs)
  | .eq => decide (lhs == rhs)
  | .ge => decide (rhs ≤ lhs)

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

def checkCutAtPoint (cut : CutSpec) (x : Array Rat) : Except String Bool := do
  let lhs ← evalTerms cut.terms x
  pure (decide (lhs ≤ cut.rhs))

/-- Verifies a GMI tableau cut (exact or upward-weakened) and checks all witness points. -/
def checkTableauGMI
    (aI aC : Array Rat)
    (beta : Rat)
    (cIOpt cCOpt : Option (Array Rat))
    (solutions : Array (Rat × Array Rat × Array Rat)) : Except String Unit := do
  if solutions.isEmpty then
    throw "GMI verification requires at least one witness solution"
  let f0 := fracPart beta
  if !(0 < f0 && f0 < 1) then
    throw s!"GMI row beta={beta} has non-fractional f0={f0}"
  let exactI := aI.map (fun a => gmiIntCoeff a f0)
  let exactC := aC.map (fun g => gmiContCoeff g f0)
  let cI ← match cIOpt with
    | none => pure exactI
    | some c => do
      if c.size != aI.size then
        throw s!"cI length {c.size} != aI length {aI.size}"
      for j in [:c.size] do
        if !(exactI[j]! ≤ c[j]!) then
          throw s!"cI[{j}]={c[j]!} < exact gmiIntCoeff={exactI[j]!}"
      pure c
  let cC ← match cCOpt with
    | none => pure exactC
    | some c => do
      if c.size != aC.size then
        throw s!"cC length {c.size} != aC length {aC.size}"
      for k in [:c.size] do
        if !(exactC[k]! ≤ c[k]!) then
          throw s!"cC[{k}]={c[k]!} < exact gmiContCoeff={exactC[k]!}"
      pure c
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

/-- Verifies a safe-rounded complemented MIR certificate (`cmir_safe_rounded_sound`) on witness points. -/
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
    (solutions : Array (Array Rat × Array Rat)) : Except String Unit := do
  if solutions.isEmpty then
    throw "cMIR verification requires at least one witness solution"
  let mI := aI.size
  let mC := aC.size
  if lbI.size != mI || ubI.size != mI || useLowerI.size != mI then
    throw "cMIR integer dimension mismatch"
  if lbC.size != mC || ubC.size != mC || useLowerC.size != mC then
    throw "cMIR continuous dimension mismatch"
  if !(0 < delta) then
    throw s!"cMIR delta={delta} must be > 0"
  if let some (lpI, lpC) := lpPrimalOpt then
    if lpI.size != mI || lpC.size != mC then
      throw "cMIR lpPrimal dimension mismatch"
    for j in [:mI] do
      let hasL := lbI[j]!.isSome
      let hasU := ubI[j]!.isSome
      let l := lbI[j]!.getD 0
      let u := ubI[j]!.getD 0
      if useLowerI[j]! != mirUseLowerSpec lpI[j]! l u hasL hasU then
        throw s!"cMIR integer useLower[{j}] does not match mirUseLowerSpec"
    for k in [:mC] do
      let hasL := lbC[k]!.isSome
      let hasU := ubC[k]!.isSome
      let l := lbC[k]!.getD 0
      let u := ubC[k]!.getD 0
      if useLowerC[k]! != mirUseLowerSpec lpC[k]! l u hasL hasU then
        throw s!"cMIR continuous useLower[{k}] does not match mirUseLowerSpec"
  let mut bndI : Array Rat := Array.mkEmpty mI
  let mut fixedI : Array Bool := Array.mkEmpty mI
  for j in [:mI] do
    if useLowerI[j]! then
      match lbI[j]! with
      | some l =>
        if l.den != 1 then
          throw s!"integer variable {j} active lower bound {l} is not integer"
        bndI := bndI.push l
      | none => throw s!"integer variable {j} missing lower bound"
    else
      match ubI[j]! with
      | some u =>
        if u.den != 1 then
          throw s!"integer variable {j} active upper bound {u} is not integer"
        bndI := bndI.push u
      | none => throw s!"integer variable {j} missing upper bound"
    let isFix := match lbI[j]!, ubI[j]! with
      | some l, some u => l == u
      | _, _ => false
    fixedI := fixedI.push isFix
  let mut bndC : Array Rat := Array.mkEmpty mC
  for k in [:mC] do
    if useLowerC[k]! then
      match lbC[k]! with
      | some l => bndC := bndC.push l
      | none => throw s!"continuous variable {k} missing lower bound"
    else
      match ubC[k]! with
      | some u => bndC := bndC.push u
      | none => throw s!"continuous variable {k} missing upper bound"
  let shiftI ← evalArrayDot aI bndI
  let shiftC ← evalArrayDot aC bndC
  let exactBeta := (rhs - shiftI - shiftC) / delta
  let betaHat := betaHatOpt.getD exactBeta
  if !(exactBeta ≤ betaHat) then
    throw s!"cMIR betaHat={betaHat} < exactBeta={exactBeta}"
  let f0 := fracPart betaHat
  let exactAlpha : Array Rat := Id.run do
    let mut out := Array.mkEmpty mI
    for j in [:mI] do
      out := out.push (cmirSignedCoeff useLowerI[j]! aI[j]! / delta)
    out
  let alphaHat ← match alphaHatOpt with
    | none => pure exactAlpha
    | some ah => do
      if ah.size != mI then
        throw s!"alphaHat length {ah.size} != {mI}"
      for j in [:mI] do
        if !(ah[j]! ≤ exactAlpha[j]!) then
          throw s!"alphaHat[{j}]={ah[j]!} > exactAlpha={exactAlpha[j]!}"
      pure ah
  let exactCI : Array Rat := alphaHat.map (fun al => mirIntCoeff al f0)
  let cI ← match cIOpt with
    | none => pure exactCI
    | some ci => do
      if ci.size != mI then
        throw s!"cI length {ci.size} != {mI}"
      for j in [:mI] do
        if !(fixedI[j]! && ci[j]! == 0) && !(ci[j]! ≤ exactCI[j]!) then
          throw s!"cI[{j}]={ci[j]!} > mirIntCoeff={exactCI[j]!}"
      pure ci
  let exactCC : Array Rat := Id.run do
    let mut out := Array.mkEmpty mC
    for k in [:mC] do
      let sg := cmirSignedCoeff useLowerC[k]! aC[k]!
      let num := max (-sg) 0
      out := out.push (num / (delta * (1 - f0)))
    out
  let cC ← match cCOpt with
    | none => pure exactCC
    | some cc => do
      if cc.size != mC then
        throw s!"cC length {cc.size} != {mC}"
      for k in [:mC] do
        if !(exactCC[k]! ≤ cc[k]!) then
          throw s!"cC[{k}]={cc[k]!} < exactContCoeff={exactCC[k]!}"
      pure cc
  let origI : Array Rat := Id.run do
    let mut out := Array.mkEmpty mI
    for j in [:mI] do
      out := out.push (cmirSignedCoeff useLowerI[j]! cI[j]!)
    out
  let origC : Array Rat := Id.run do
    let mut out := Array.mkEmpty mC
    for k in [:mC] do
      out := out.push (if useLowerC[k]! then -cC[k]! else cC[k]!)
    out
  let exactCutRHS := (betaHat.floor : Rat) + (← evalArrayDot origI bndI) + (← evalArrayDot origC bndC)
  let cutRHS := cutRHSOpt.getD exactCutRHS
  if !(exactCutRHS ≤ cutRHS) then
    throw s!"cutRHS={cutRHS} < exactCutRHS={exactCutRHS}"
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

/-- Verifies a VUB disaggregation certificate matching `vub_disagg_opt_sound`. -/
def checkVUBDisagg
    (a : Array Rat)
    (u : Array (Option Rat))
    (cap rhsRow : Rat)
    (targetIdx : Nat)
    (effUB cutRHS : Rat)
    (solutions : Array (Array Rat × Rat)) : Except String Unit := do
  if solutions.isEmpty then
    throw "VUB verification requires at least one witness solution"
  if a.size != u.size then
    throw s!"VUB arrays a ({a.size}) and u ({u.size}) mismatch"
  if !(targetIdx < a.size) then
    throw s!"targetIdx {targetIdx} out of bounds ({a.size})"
  if !(0 ≤ cap) then
    throw s!"VUB capacity C={cap} must be non-negative"
  if !(rhsRow ≤ 0) then
    throw s!"VUB row RHS={rhsRow} must be ≤ 0"
  for j in [:a.size] do
    if !(0 < a[j]!) then
      throw s!"VUB coeff a[{j}]={a[j]!} must be > 0"
  let reqEff := vubEffUB u[targetIdx]! cap a[targetIdx]!
  if !(reqEff ≤ effUB) then
    throw s!"violated min bound: effUB={effUB} < min(u, C/a)={reqEff}"
  if !(0 ≤ cutRHS) then
    throw s!"VUB cutRHS={cutRHS} must be ≥ 0"
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

/-- Verifies a batch of structural cuts against a MIP and feasible mixed-integer solutions. -/
def checkStructuralCuts
    (mipOpt : Option MIPSpec)
    (cuts : Array CutSpec)
    (solutions : Array (Array Rat)) : Except String Unit := do
  if solutions.isEmpty then
    throw "structural cut verification requires at least one feasible solution"
  for sIdx in [:solutions.size] do
    let x := solutions[sIdx]!
    if let some mip := mipOpt then
      checkMIPFeasible mip x
    for cIdx in [:cuts.size] do
      let ok ← checkCutAtPoint cuts[cIdx]! x
      if !ok then
        let lhs ← evalTerms cuts[cIdx]!.terms x
        throw s!"cut {cIdx} violated at solution {sIdx}: lhs={lhs} > rhs={cuts[cIdx]!.rhs}"

/-- Verifies row aggregation multiplier legality (`rowLegalMuSignSpec`) and aggregated row validity on witness points. -/
def checkRowAgg
    (rows : Array (RowSpec × Rat))
    (allowZero : Bool)
    (aggCutOpt : Option CutSpec)
    (solutions : Array (Array Rat)) : Except String Unit := do
  if solutions.isEmpty then
    throw "row aggregation verification requires at least one witness solution"
  if rows.isEmpty then
    throw "row aggregation verification requires at least one row"
  for idx in [:rows.size] do
    let (row, mu) := rows[idx]!
    let legal := (allowZero && mu == 0) || rowLegalMuSignSpec row.sense mu
    if !legal then
      throw s!"row {idx}: illegal multiplier mu={mu} for sense={repr row.sense}"
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

end GoblinFormal.Track2B
