/-
GoblinFormal.Common.Basic: pure Init foundations for finite sums over Rat/Int
and exact IEEE-754 binary64 bit-pattern conversion to dyadic Rat.
-/

namespace GoblinFormal

/-- Finite sum of `f` over `Fin n` in `Rat`. -/
def finSum : (n : Nat) → (Fin n → Rat) → Rat
  | 0, _ => 0
  | n + 1, f => finSum n (fun i => f i.castSucc) + f (Fin.last n)

/-- Finite sum of `f` over `Fin n` in `Int`. -/
def intFinSum : (n : Nat) → (Fin n → Int) → Int
  | 0, _ => 0
  | n + 1, f => intFinSum n (fun i => f i.castSucc) + f (Fin.last n)

@[simp]
theorem finSum_zero_n (f : Fin 0 → Rat) : finSum 0 f = 0 := rfl

theorem finSum_succ (n : Nat) (f : Fin (n + 1) → Rat) :
    finSum (n + 1) f = finSum n (fun i => f i.castSucc) + f (Fin.last n) := rfl

theorem finSum_congr {n : Nat} {f g : Fin n → Rat} (h : ∀ i, f i = g i) :
    finSum n f = finSum n g := by
  induction n with
  | zero => rfl
  | succ n ih =>
    simp only [finSum_succ]
    rw [ih (fun i => h i.castSucc), h (Fin.last n)]

@[simp]
theorem finSum_zero (n : Nat) : finSum n (fun _ => (0 : Rat)) = 0 := by
  induction n with
  | zero => rfl
  | succ n ih =>
    simp [finSum_succ, ih, Rat.add_zero]

theorem finSum_add (n : Nat) (f g : Fin n → Rat) :
    finSum n (fun i => f i + g i) = finSum n f + finSum n g := by
  induction n with
  | zero => simp [Rat.add_zero]
  | succ n ih =>
    simp only [finSum_succ, ih (fun i => f i.castSucc) (fun i => g i.castSucc)]
    rw [Rat.add_assoc, Rat.add_assoc, Rat.add_left_comm (f (Fin.last n))]

theorem finSum_neg (n : Nat) (f : Fin n → Rat) :
    finSum n (fun i => -f i) = -finSum n f := by
  induction n with
  | zero => simp
  | succ n ih =>
    simp only [finSum_succ, ih (fun i => f i.castSucc), Rat.neg_add]

theorem finSum_sub (n : Nat) (f g : Fin n → Rat) :
    finSum n (fun i => f i - g i) = finSum n f - finSum n g := by
  have h1 : finSum n (fun i => f i - g i) = finSum n (fun i => f i + (-g i)) :=
    finSum_congr (fun i => Rat.sub_eq_add_neg (f i) (g i))
  rw [h1, finSum_add, finSum_neg, ← Rat.sub_eq_add_neg]

theorem finSum_const_mul (n : Nat) (c : Rat) (f : Fin n → Rat) :
    finSum n (fun i => c * f i) = c * finSum n f := by
  induction n with
  | zero => simp
  | succ n ih =>
    simp only [finSum_succ, ih (fun i => f i.castSucc), Rat.mul_add]

theorem finSum_mul_const (n : Nat) (f : Fin n → Rat) (c : Rat) :
    finSum n (fun i => f i * c) = finSum n f * c := by
  have h : finSum n (fun i => f i * c) = finSum n (fun i => c * f i) :=
    finSum_congr (fun i => Rat.mul_comm (f i) c)
  rw [h, finSum_const_mul, Rat.mul_comm]

theorem finSum_const (n : Nat) (c : Rat) :
    finSum n (fun _ => c) = (n : Rat) * c := by
  induction n with
  | zero => simp
  | succ n ih =>
    simp only [finSum_succ, ih, Rat.natCast_add, Rat.natCast_ofNat, Rat.add_mul, Rat.one_mul]

private theorem rat_add_le_add {a b c d : Rat} (hab : a ≤ b) (hcd : c ≤ d) :
    a + c ≤ b + d :=
  Rat.le_trans (Rat.add_le_add_right.mpr hab) (Rat.add_le_add_left.mpr hcd)

theorem finSum_le_finSum {n : Nat} {f g : Fin n → Rat} (h : ∀ i, f i ≤ g i) :
    finSum n f ≤ finSum n g := by
  induction n with
  | zero => exact Rat.le_refl
  | succ n ih =>
    simp only [finSum_succ]
    exact rat_add_le_add (ih (fun i => h i.castSucc)) (h (Fin.last n))

theorem finSum_nonneg {n : Nat} {f : Fin n → Rat} (h : ∀ i, 0 ≤ f i) :
    0 ≤ finSum n f := by
  have h0 : finSum n (fun _ => (0 : Rat)) ≤ finSum n f := finSum_le_finSum h
  simpa [finSum_zero] using h0

theorem finSum_single {n : Nat} (k : Fin n) (c : Rat) :
    finSum n (fun j => if j = k then c else 0) = c := by
  induction n with
  | zero => exact Fin.elim0 k
  | succ n ih =>
    rw [finSum_succ]
    by_cases hk : k = Fin.last n
    · subst hk
      have hleft : finSum n (fun i : Fin n => if i.castSucc = Fin.last n then c else 0) = 0 := by
        have h0 :
            finSum n (fun i : Fin n => if i.castSucc = Fin.last n then c else 0) =
            finSum n (fun _ : Fin n => (0 : Rat)) := by
          apply finSum_congr
          intro i
          have hne : i.castSucc ≠ Fin.last n := by
            intro heq
            have hval := congrArg Fin.val heq
            simp only [Fin.val_castSucc, Fin.val_last] at hval
            omega
          simp [hne]
        rw [h0, finSum_zero]
      rw [hleft, Rat.zero_add]
      simp
    · have hklt : k.val < n := by
        have hklt' := k.isLt
        have hne : k.val ≠ n := fun hval => hk (Fin.ext hval)
        omega
      let k0 : Fin n := ⟨k.val, hklt⟩
      have hk0 : k = k0.castSucc := Fin.ext rfl
      have hleft :
          finSum n (fun i : Fin n => if i.castSucc = k then c else 0) =
          finSum n (fun i : Fin n => if i = k0 then c else 0) := by
        apply finSum_congr
        intro i
        have hiff : i.castSucc = k ↔ i = k0 := by
          rw [hk0]
          exact ⟨fun h => Fin.ext (by simpa [Fin.val_castSucc] using congrArg Fin.val h),
                 fun h => h ▸ rfl⟩
        simp [hiff]
      have hright : (if Fin.last n = k then c else 0) = 0 := by
        simp [Ne.symm hk]
      rw [hleft, ih k0, hright, Rat.add_zero]

theorem le_finSum_of_nonneg {n : Nat} {f : Fin n → Rat} (h : ∀ i, 0 ≤ f i) (k : Fin n) :
    f k ≤ finSum n f := by
  have hle : ∀ j : Fin n, (if j = k then f k else 0) ≤ f j := by
    intro j
    by_cases hj : j = k
    · subst hj; simp
    · simp [hj, h j]
  have hsum := finSum_le_finSum hle
  rwa [finSum_single] at hsum

theorem finSum_inj_le {m n : Nat} {g : Fin n → Rat} {q : Fin m → Fin n}
    (hg : ∀ j, 0 ≤ g j) (hq : ∀ i₁ i₂, q i₁ = q i₂ → i₁ = i₂) :
    finSum m (fun i => g (q i)) ≤ finSum n g := by
  induction m generalizing g with
  | zero =>
    simp only [finSum_zero_n]
    exact finSum_nonneg hg
  | succ m ih =>
    let k : Fin n := q (Fin.last m)
    let g' : Fin n → Rat := fun j => if j = k then 0 else g j
    have hg' : ∀ j, 0 ≤ g' j := by
      intro j
      dsimp [g']
      split
      · exact Rat.le_refl
      · exact hg j
    have hq' : ∀ i₁ i₂ : Fin m, q i₁.castSucc = q i₂.castSucc → i₁ = i₂ := by
      intro i₁ i₂ heq
      have hinj := hq i₁.castSucc i₂.castSucc heq
      exact Fin.ext (by simpa [Fin.ext_iff] using hinj)
    have hih := ih hg' hq'
    have hmatch :
        finSum m (fun i => g' (q i.castSucc)) =
        finSum m (fun i => g (q i.castSucc)) := by
      apply finSum_congr
      intro i
      dsimp [g']
      have hne : q i.castSucc ≠ k := by
        intro heq
        have hinj := hq i.castSucc (Fin.last m) heq
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
    rw [finSum_succ, ← hmatch, hsplit]
    exact Rat.add_le_add_right.mpr hih

private theorem rat_neg_le_abs (x : Rat) : -x ≤ x.abs := by
  rcases Rat.le_total (a := 0) (b := x) with hx | hx
  · rw [Rat.abs_of_nonneg hx]
    have hneg : -x ≤ 0 := Rat.neg_le_iff.mpr (by simpa using hx)
    exact Rat.le_trans hneg hx
  · rw [Rat.abs_of_nonpos hx]
    exact Rat.le_refl

private theorem finSum_inj_le_any {n : Nat} {g : Fin n → Rat} {q : Fin n → Fin n}
    (hq : ∀ i₁ i₂, q i₁ = q i₂ → i₁ = i₂) :
    finSum n (fun i => g (q i)) ≤ finSum n g := by
  let C : Rat := finSum n (fun j => (g j).abs)
  have hnonneg : ∀ j, 0 ≤ g j + C := by
    intro j
    have h1 : -g j ≤ (g j).abs := rat_neg_le_abs (g j)
    have h2 : (g j).abs ≤ C :=
      le_finSum_of_nonneg (fun k => Rat.abs_nonneg) j
    have h3 : -g j ≤ C := Rat.le_trans h1 h2
    have h4 := (Rat.add_le_add_left (c := g j)).mpr h3
    simpa [Rat.add_neg_cancel] using h4
  have hle := finSum_inj_le hnonneg hq
  rw [finSum_add, finSum_add, finSum_const] at hle
  exact Rat.add_le_add_right.mp hle

theorem finSum_inj_eq {n : Nat} {g : Fin n → Rat} {q : Fin n → Fin n}
    (hq : ∀ i₁ i₂, q i₁ = q i₂ → i₁ = i₂) :
    finSum n (fun i => g (q i)) = finSum n g := by
  have h1 : finSum n (fun i => g (q i)) ≤ finSum n g :=
    finSum_inj_le_any hq
  have h2_neg : finSum n (fun i => -g (q i)) ≤ finSum n (fun j => -g j) :=
    finSum_inj_le_any (g := fun j => -g j) hq
  rw [finSum_neg, finSum_neg, Rat.neg_le_neg_iff] at h2_neg
  exact Rat.le_antisymm h1 h2_neg

@[simp]
theorem intFinSum_zero_n (f : Fin 0 → Int) : intFinSum 0 f = 0 := rfl

theorem intFinSum_succ (n : Nat) (f : Fin (n + 1) → Int) :
    intFinSum (n + 1) f = intFinSum n (fun i => f i.castSucc) + f (Fin.last n) := rfl

theorem intFinSum_congr {n : Nat} {f g : Fin n → Int} (h : ∀ i, f i = g i) :
    intFinSum n f = intFinSum n g := by
  induction n with
  | zero => rfl
  | succ n ih =>
    simp only [intFinSum_succ]
    rw [ih (fun i => h i.castSucc), h (Fin.last n)]

@[simp]
theorem intFinSum_zero (n : Nat) : intFinSum n (fun _ => (0 : Int)) = 0 := by
  induction n with
  | zero => rfl
  | succ n ih => simp [intFinSum_succ, ih]

theorem intFinSum_add (n : Nat) (f g : Fin n → Int) :
    intFinSum n (fun i => f i + g i) = intFinSum n f + intFinSum n g := by
  induction n with
  | zero => simp
  | succ n ih =>
    simp only [intFinSum_succ, ih (fun i => f i.castSucc) (fun i => g i.castSucc)]
    omega

theorem intFinSum_neg (n : Nat) (f : Fin n → Int) :
    intFinSum n (fun i => -f i) = -intFinSum n f := by
  induction n with
  | zero => simp
  | succ n ih =>
    simp only [intFinSum_succ, ih (fun i => f i.castSucc)]
    omega

theorem intFinSum_sub (n : Nat) (f g : Fin n → Int) :
    intFinSum n (fun i => f i - g i) = intFinSum n f - intFinSum n g := by
  induction n with
  | zero => simp
  | succ n ih =>
    simp only [intFinSum_succ, ih (fun i => f i.castSucc) (fun i => g i.castSucc)]
    omega

theorem intFinSum_const_mul (n : Nat) (c : Int) (f : Fin n → Int) :
    intFinSum n (fun i => c * f i) = c * intFinSum n f := by
  induction n with
  | zero => simp
  | succ n ih =>
    simp only [intFinSum_succ, ih (fun i => f i.castSucc), Int.mul_add]

theorem intFinSum_mul_const (n : Nat) (f : Fin n → Int) (c : Int) :
    intFinSum n (fun i => f i * c) = intFinSum n f * c := by
  have h : intFinSum n (fun i => f i * c) = intFinSum n (fun i => c * f i) :=
    intFinSum_congr (fun i => Int.mul_comm (f i) c)
  rw [h, intFinSum_const_mul, Int.mul_comm]

theorem intFinSum_le_intFinSum {n : Nat} {f g : Fin n → Int} (h : ∀ i, f i ≤ g i) :
    intFinSum n f ≤ intFinSum n g := by
  induction n with
  | zero => exact Int.le_refl 0
  | succ n ih =>
    simp only [intFinSum_succ]
    have h1 := ih (fun i => h i.castSucc)
    have h2 := h (Fin.last n)
    omega

theorem intFinSum_nonneg {n : Nat} {f : Fin n → Int} (h : ∀ i, 0 ≤ f i) :
    0 ≤ intFinSum n f := by
  have h0 : intFinSum n (fun _ => (0 : Int)) ≤ intFinSum n f := intFinSum_le_intFinSum h
  simpa [intFinSum_zero] using h0

theorem intFinSum_dvd_of_dvd {n : Nat} {g : Int} {f : Fin n → Int} (h : ∀ j, g ∣ f j) :
    g ∣ intFinSum n f := by
  induction n with
  | zero =>
    simp only [intFinSum_zero_n]
    exact Int.dvd_zero g
  | succ n ih =>
    rw [intFinSum_succ]
    exact Int.dvd_add (ih (fun i => h i.castSucc)) (h (Fin.last n))

theorem intFinSum_dvd {n : Nat} {g : Int} {a x : Fin n → Int} (h : ∀ j, g ∣ a j) :
    g ∣ intFinSum n (fun j => a j * x j) :=
  intFinSum_dvd_of_dvd (fun j => Int.dvd_mul_of_dvd_left (h j))

theorem intFinSum_eq_finSum (n : Nat) (f : Fin n → Int) :
    (intFinSum n f : Rat) = finSum n (fun i => (f i : Rat)) := by
  induction n with
  | zero => rfl
  | succ n ih =>
    simp only [intFinSum_succ, finSum_succ, Rat.intCast_add, ih (fun i => f i.castSucc)]

theorem intFinSum_inj_le {m n : Nat} {g : Fin n → Int} {q : Fin m → Fin n}
    (hg : ∀ j, 0 ≤ g j) (hq : ∀ i₁ i₂, q i₁ = q i₂ → i₁ = i₂) :
    intFinSum m (fun i => g (q i)) ≤ intFinSum n g := by
  rw [← Rat.intCast_le_intCast, intFinSum_eq_finSum, intFinSum_eq_finSum]
  exact finSum_inj_le (fun j => Rat.intCast_nonneg.mpr (hg j)) hq

theorem intFinSum_inj_eq {n : Nat} {g : Fin n → Int} {q : Fin n → Fin n}
    (hq : ∀ i₁ i₂, q i₁ = q i₂ → i₁ = i₂) :
    intFinSum n (fun i => g (q i)) = intFinSum n g := by
  rw [← Rat.intCast_inj, intFinSum_eq_finSum, intFinSum_eq_finSum]
  exact finSum_inj_eq (g := fun i => (g i : Rat)) hq

/-- Exact dyadic rational `m * 2^e` in `Rat`. -/
def dyadicRat (m : Int) (e : Int) : Rat :=
  if 0 ≤ e then
    ((m * ((2 ^ e.toNat : Nat) : Int) : Int) : Rat)
  else
    mkRat m (2 ^ (-e).toNat)

theorem dyadicRat_eq_zpow (m : Int) (e : Int) :
    dyadicRat m e = (m : Rat) * (2 : Rat) ^ e := by
  unfold dyadicRat
  split
  · next he =>
    have hnat : (e.toNat : Int) = e := Int.toNat_of_nonneg he
    rw [← hnat, Rat.zpow_natCast]
    push_cast
    rfl
  · next he =>
    have hneg : 0 ≤ -e := by omega
    have hnat : ((-e).toNat : Int) = -e := Int.toNat_of_nonneg hneg
    have hneg_e : e = -(((-e).toNat : Nat) : Int) := by omega
    conv => rhs; rw [hneg_e]
    rw [Rat.zpow_neg, Rat.zpow_natCast, ← Rat.div_def, ← Rat.natCast_ofNat (a := 2),
      ← Rat.natCast_pow, ← Rat.mkRat_eq_div]

@[simp]
theorem dyadicRat_zero (e : Int) : dyadicRat 0 e = 0 := by
  unfold dyadicRat
  split <;> simp

theorem dyadicRat_neg (m : Int) (e : Int) :
    dyadicRat (-m) e = -dyadicRat m e := by
  rw [dyadicRat_eq_zpow, dyadicRat_eq_zpow, Rat.intCast_neg, Rat.neg_mul]

theorem dyadicRat_nonneg {m : Int} (hm : 0 ≤ m) (e : Int) :
    0 ≤ dyadicRat m e := by
  rw [dyadicRat_eq_zpow]
  exact Rat.mul_nonneg (Rat.intCast_nonneg.mpr hm)
    (Rat.zpow_nonneg (Rat.intCast_nonneg.mpr (by omega)))

theorem dyadicRat_nonpos {m : Int} (hm : m ≤ 0) (e : Int) :
    dyadicRat m e ≤ 0 := by
  have hm' : 0 ≤ -m := by omega
  have h := dyadicRat_nonneg hm' e
  rw [dyadicRat_neg, Rat.le_neg_iff] at h
  simpa using h

/-- Converts an IEEE-754 binary64 bit pattern to its exact `Rat` value,
returning `none` on Inf/NaN (`exponent == 0x7ff`). -/
def float64BitsToRat (bits : UInt64) : Option Rat :=
  let sign : Bool := (bits >>> 63) != 0
  let exp : Nat := ((bits >>> 52) &&& 0x7ff).toNat
  let mant : Nat := (bits &&& 0x000fffffffffffff).toNat
  if exp == 0x7ff then
    none
  else if exp == 0 then
    if mant == 0 then
      some 0
    else
      let signedMant : Int := if sign then -(mant : Int) else (mant : Int)
      some (dyadicRat signedMant (-1074))
  else
    let sig : Nat := (1 <<< 52) + mant
    let signedSig : Int := if sign then -(sig : Int) else (sig : Int)
    some (dyadicRat signedSig ((exp : Int) - 1075))

theorem float64BitsToRat_zero : float64BitsToRat 0 = some 0 := by
  decide

theorem float64BitsToRat_negZero : float64BitsToRat 0x8000000000000000 = some 0 := by
  decide

theorem float64BitsToRat_isNone_iff (bits : UInt64) :
    (float64BitsToRat bits).isNone ↔ ((bits >>> 52) &&& 0x7ff).toNat = 0x7ff := by
  unfold float64BitsToRat
  dsimp only
  by_cases h1 : ((bits >>> 52) &&& 0x7ff).toNat = 0x7ff
  · simp [h1]
  · have h1b : (((bits >>> 52) &&& 0x7ff).toNat == 0x7ff) = false := beq_false_of_ne h1
    simp only [h1b, Bool.false_eq_true, ↓reduceIte]
    by_cases h2 : ((bits >>> 52) &&& 0x7ff).toNat == 0
    · simp only [h2, ↓reduceIte]
      by_cases h3 : (bits &&& 0x000fffffffffffff).toNat == 0
      · simp only [h3, ↓reduceIte, Option.isNone_some, Bool.false_eq_true, false_iff]
        exact h1
      · simp only [h3, Bool.false_eq_true, ↓reduceIte, Option.isNone_some, Bool.false_eq_true,
          false_iff]
        exact h1
    · simp only [h2, Bool.false_eq_true, ↓reduceIte, Option.isNone_some, Bool.false_eq_true,
        false_iff]
      exact h1

theorem float64BitsToRat_nonneg_of_sign_false {bits : UInt64} {r : Rat}
    (hs : (bits >>> 63) = 0) (hr : float64BitsToRat bits = some r) : 0 ≤ r := by
  unfold float64BitsToRat at hr
  dsimp only at hr
  have hsign : ((bits >>> 63) != 0) = false := by simp [hs]
  rw [hsign] at hr
  by_cases h1 : ((bits >>> 52) &&& 0x7ff).toNat == 0x7ff
  · simp only [h1, ↓reduceIte] at hr
    contradiction
  · simp only [h1, Bool.false_eq_true, ↓reduceIte] at hr
    by_cases h2 : ((bits >>> 52) &&& 0x7ff).toNat == 0
    · simp only [h2, ↓reduceIte] at hr
      by_cases h3 : (bits &&& 0x000fffffffffffff).toNat == 0
      · simp only [h3, ↓reduceIte] at hr
        rw [← Option.some.inj hr]
        exact Rat.le_refl
      · simp only [h3, Bool.false_eq_true, ↓reduceIte] at hr
        rw [← Option.some.inj hr]
        exact dyadicRat_nonneg (Int.natCast_nonneg _) _
    · simp only [h2, Bool.false_eq_true, ↓reduceIte] at hr
      rw [← Option.some.inj hr]
      exact dyadicRat_nonneg (Int.natCast_nonneg _) _

theorem float64BitsToRat_nonpos_of_sign_true {bits : UInt64} {r : Rat}
    (hs : (bits >>> 63) ≠ 0) (hr : float64BitsToRat bits = some r) : r ≤ 0 := by
  unfold float64BitsToRat at hr
  dsimp only at hr
  have hsign : ((bits >>> 63) != 0) = true := bne_iff_ne.mpr hs
  rw [hsign] at hr
  by_cases h1 : ((bits >>> 52) &&& 0x7ff).toNat == 0x7ff
  · simp only [h1, ↓reduceIte] at hr
    contradiction
  · simp only [h1, Bool.false_eq_true, ↓reduceIte] at hr
    by_cases h2 : ((bits >>> 52) &&& 0x7ff).toNat == 0
    · simp only [h2, ↓reduceIte] at hr
      by_cases h3 : (bits &&& 0x000fffffffffffff).toNat == 0
      · simp only [h3, ↓reduceIte] at hr
        rw [← Option.some.inj hr]
        exact Rat.le_refl
      · simp only [h3, Bool.false_eq_true, ↓reduceIte] at hr
        rw [← Option.some.inj hr]
        exact dyadicRat_nonpos (Int.neg_nonpos_of_nonneg (Int.natCast_nonneg _)) _
    · simp only [h2, Bool.false_eq_true, ↓reduceIte] at hr
      rw [← Option.some.inj hr]
      exact dyadicRat_nonpos (Int.neg_nonpos_of_nonneg (Int.natCast_nonneg _)) _

end GoblinFormal
