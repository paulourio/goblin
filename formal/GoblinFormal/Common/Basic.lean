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

/-! ### Elementary Rat algebra, order, and absolute-value helpers -/

theorem rat_add_le_add {a b c d : Rat} (hab : a ≤ b) (hcd : c ≤ d) :
    a + c ≤ b + d :=
  Rat.le_trans (Rat.add_le_add_right.mpr hab) (Rat.add_le_add_left.mpr hcd)

theorem rat_neg_nonneg {a : Rat} (ha : a ≤ 0) : 0 ≤ -a := by
  simpa using Rat.neg_le_neg ha

theorem rat_sub_zero (a : Rat) : a - 0 = a := by
  rw [Rat.sub_eq_add_neg, Rat.neg_zero, Rat.add_zero]

theorem rat_zero_sub (a : Rat) : 0 - a = -a := by
  rw [Rat.sub_eq_add_neg, Rat.zero_add]

theorem rat_sub_mul (a b c : Rat) : (a - b) * c = a * c - b * c := by
  rw [Rat.sub_eq_add_neg, Rat.add_mul, Rat.neg_mul, ← Rat.sub_eq_add_neg]

theorem rat_mul_sub (a b c : Rat) : a * (b - c) = a * b - a * c := by
  rw [Rat.sub_eq_add_neg, Rat.mul_add, Rat.mul_neg, ← Rat.sub_eq_add_neg]

theorem rat_lt_of_lt_of_le {a b c : Rat} (hab : a < b) (hbc : b ≤ c) : a < c :=
  Rat.not_le.mp fun hca => Rat.not_le.mpr hab (Rat.le_trans hbc hca)

theorem rat_lt_of_le_of_lt {a b c : Rat} (hab : a ≤ b) (hbc : b < c) : a < c :=
  Rat.not_le.mp fun hca => Rat.not_le.mpr hbc (Rat.le_trans hca hab)

theorem rat_sub_le_sub {a b c d : Rat} (hab : a ≤ b) (hdc : d ≤ c) :
    a - c ≤ b - d := by
  rw [Rat.sub_eq_add_neg, Rat.sub_eq_add_neg]
  exact rat_add_le_add hab (Rat.neg_le_neg hdc)

theorem rat_mul_le_mul_of_nonpos_right {a b c : Rat} (hab : a ≤ b) (hc : c ≤ 0) :
    b * c ≤ a * c := by
  have h := Rat.mul_le_mul_of_nonneg_right hab (rat_neg_nonneg hc)
  rwa [Rat.mul_neg, Rat.mul_neg, Rat.neg_le_neg_iff] at h

theorem rat_mul_le_mul_of_nonpos_left {a b c : Rat} (hab : a ≤ b) (hc : c ≤ 0) :
    c * b ≤ c * a := by
  rw [Rat.mul_comm c b, Rat.mul_comm c a]
  exact rat_mul_le_mul_of_nonpos_right hab hc

theorem rat_div_le_iff {a b c : Rat} (hb : 0 < b) : a / b ≤ c ↔ a ≤ c * b := by
  constructor
  · intro h
    have h1 := Rat.mul_le_mul_of_nonneg_right h (Rat.le_of_lt hb)
    rwa [Rat.div_mul_cancel (Rat.ne_of_gt hb)] at h1
  · intro h
    exact Rat.le_of_mul_le_mul_right (by rwa [Rat.div_mul_cancel (Rat.ne_of_gt hb)]) hb

theorem rat_le_div_iff {a b c : Rat} (hc : 0 < c) : a ≤ b / c ↔ a * c ≤ b := by
  constructor
  · intro h
    have h1 := Rat.mul_le_mul_of_nonneg_right h (Rat.le_of_lt hc)
    rwa [Rat.div_mul_cancel (Rat.ne_of_gt hc)] at h1
  · intro h
    exact Rat.le_of_mul_le_mul_right (by rwa [Rat.div_mul_cancel (Rat.ne_of_gt hc)]) hc

theorem rat_div_le_div_of_pos {a b c : Rat} (hab : a ≤ b) (hc : 0 < c) :
    a / c ≤ b / c := by
  rw [Rat.div_def, Rat.div_def]
  exact Rat.mul_le_mul_of_nonneg_right hab (Rat.le_of_lt (Rat.inv_pos.mpr hc))

theorem rat_le_min_iff {a b c : Rat} : a ≤ min b c ↔ a ≤ b ∧ a ≤ c := by
  rw [Rat.min_def]
  split
  · next h => exact ⟨fun h1 => ⟨h1, Rat.le_trans h1 h⟩, And.left⟩
  · next h =>
    have hlt : c ≤ b := Rat.le_of_lt (Rat.not_le.mp h)
    exact ⟨fun h1 => ⟨Rat.le_trans h1 hlt, h1⟩, And.right⟩

theorem rat_le_max_left {a b : Rat} : a ≤ max a b := by
  rw [Rat.max_def]
  split
  · assumption
  · exact Rat.le_refl

theorem rat_le_max_right {a b : Rat} : b ≤ max a b := by
  rw [Rat.max_def]
  split
  · exact Rat.le_refl
  · next h => exact Rat.le_of_lt (Rat.not_le.mp h)

theorem rat_abs_le_iff {a b : Rat} : a.abs ≤ b ↔ -b ≤ a ∧ a ≤ b := by
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

theorem rat_le_abs (a : Rat) : a ≤ a.abs :=
  (rat_abs_le_iff.mp Rat.le_refl).2

theorem rat_abs_mul (a b : Rat) : (a * b).abs = a.abs * b.abs := by
  rcases Rat.le_total (a := 0) (b := a) with ha | ha <;>
    rcases Rat.le_total (a := 0) (b := b) with hb | hb
  · rw [Rat.abs_of_nonneg ha, Rat.abs_of_nonneg hb, Rat.abs_of_nonneg (Rat.mul_nonneg ha hb)]
  · have hab : a * b ≤ 0 := by simpa [Rat.mul_zero] using Rat.mul_le_mul_of_nonneg_left hb ha
    rw [Rat.abs_of_nonneg ha, Rat.abs_of_nonpos hb, Rat.abs_of_nonpos hab, Rat.mul_neg]
  · have hab : a * b ≤ 0 := by simpa [Rat.zero_mul] using Rat.mul_le_mul_of_nonneg_right ha hb
    rw [Rat.abs_of_nonpos ha, Rat.abs_of_nonneg hb, Rat.abs_of_nonpos hab, Rat.neg_mul]
  · have hab : 0 ≤ a * b := by
      simpa [Rat.neg_mul, Rat.mul_neg, Rat.neg_neg] using
        Rat.mul_nonneg (rat_neg_nonneg ha) (rat_neg_nonneg hb)
    rw [Rat.abs_of_nonpos ha, Rat.abs_of_nonpos hb, Rat.abs_of_nonneg hab,
      Rat.neg_mul, Rat.mul_neg, Rat.neg_neg]

theorem rat_eq_num_of_den_one {r : Rat} (hden : r.den = 1) : r = (r.num : Rat) :=
  Rat.ext (by simp) (by simpa using hden)

/-! ### Finite-sum monotonicity, splitting, swapping, and permutation invariance -/

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

theorem finSum_split_at {n : Nat} (f : Fin n → Rat) (k : Fin n) :
    finSum n f = finSum n (fun j => if j = k then 0 else f j) + f k := by
  have heq : ∀ j : Fin n,
      f j = (if j = k then 0 else f j) + (if j = k then f k else 0) := by
    intro j
    by_cases hj : j = k
    · subst hj; simp [Rat.zero_add]
    · simp [hj, Rat.add_zero]
  rw [finSum_congr heq, finSum_add, finSum_single]

theorem finSum_swap {m n : Nat} (f : Fin m → Fin n → Rat) :
    finSum m (fun i => finSum n (fun j => f i j)) =
      finSum n (fun j => finSum m (fun i => f i j)) := by
  induction m with
  | zero =>
    simp only [finSum_zero_n]
    exact (finSum_zero n).symm
  | succ m ih =>
    simp only [finSum_succ]
    rw [ih (fun i j => f i.castSucc j), ← finSum_add]

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
    have hsplit : finSum n g = finSum n g' + g k := finSum_split_at g k
    rw [finSum_succ, ← hmatch, hsplit]
    exact Rat.add_le_add_right.mpr hih

private theorem rat_neg_le_abs (x : Rat) : -x ≤ x.abs :=
  Rat.neg_le_iff.mp (rat_abs_le_iff.mp Rat.le_refl).1

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

/-- Sign bit of an IEEE-754 binary64 bit pattern (`0` or `1`). -/
def ieee754Sign (bits : UInt64) : Nat :=
  bits.toNat / 2 ^ 63

/-- Biased 11-bit exponent field of an IEEE-754 binary64 bit pattern (`0 .. 2047`). -/
def ieee754Exp (bits : UInt64) : Nat :=
  (bits.toNat / 2 ^ 52) % 2 ^ 11

/-- 52-bit trailing significand field of an IEEE-754 binary64 bit pattern (`0 .. 2^52 - 1`). -/
def ieee754Mant (bits : UInt64) : Nat :=
  bits.toNat % 2 ^ 52

/-- Textbook IEEE-754-2008 binary64 rational value function:
`none` for `exp = 2047` (Inf/NaN),
`(-1)^sign * (mant / 2^52) * 2^(-1022)` for `exp = 0` (zero/subnormal),
`(-1)^sign * (1 + mant / 2^52) * 2^(exp - 1023)` for `1 ≤ exp ≤ 2046` (normal). -/
def ieee754Binary64Val (bits : UInt64) : Option Rat :=
  let s := ieee754Sign bits
  let e := ieee754Exp bits
  let m := ieee754Mant bits
  if e == 0x7ff then
    none
  else if e == 0 then
    some ((-1 : Rat) ^ s * ((m : Rat) / (2 : Rat) ^ (52 : Nat)) * (2 : Rat) ^ (-1022 : Int))
  else
    some ((-1 : Rat) ^ s * (1 + (m : Rat) / (2 : Rat) ^ (52 : Nat)) * (2 : Rat) ^ ((e : Int) - 1023))

theorem float64Bits_sign_eq (bits : UInt64) :
    (bits >>> 63).toNat = ieee754Sign bits := by
  unfold ieee754Sign
  rw [UInt64.toNat_shiftRight]
  have hmod : (63 : UInt64).toNat % 64 = 63 := by decide
  rw [hmod, Nat.shiftRight_eq_div_pow]

theorem float64Bits_exp_eq (bits : UInt64) :
    ((bits >>> 52) &&& 0x7ff).toNat = ieee754Exp bits := by
  unfold ieee754Exp
  rw [UInt64.toNat_and, UInt64.toNat_shiftRight]
  have hmod : (52 : UInt64).toNat % 64 = 52 := by decide
  have hmask : (0x7ff : UInt64).toNat = 2 ^ 11 - 1 := by decide
  rw [hmod, hmask, Nat.shiftRight_eq_div_pow, Nat.and_two_pow_sub_one_eq_mod]

theorem float64Bits_mant_eq (bits : UInt64) :
    (bits &&& 0x000fffffffffffff).toNat = ieee754Mant bits := by
  unfold ieee754Mant
  rw [UInt64.toNat_and]
  have hmask : (0x000fffffffffffff : UInt64).toNat = 2 ^ 52 - 1 := by decide
  rw [hmask, Nat.and_two_pow_sub_one_eq_mod]

/-- Exact 64-bit field reconstruction: `bits.toNat = sign * 2^63 + exp * 2^52 + mant`
with exact field ranges `sign < 2`, `exp < 2048`, `mant < 2^52`. -/
theorem float64Bits_field_decomp (bits : UInt64) :
    ieee754Sign bits < 2 ∧
    ieee754Exp bits < 2 ^ 11 ∧
    ieee754Mant bits < 2 ^ 52 ∧
    bits.toNat = ieee754Sign bits * 2 ^ 63 + ieee754Exp bits * 2 ^ 52 + ieee754Mant bits := by
  unfold ieee754Sign ieee754Exp ieee754Mant
  have hlt : bits.toNat < 2 ^ 64 := bits.toBitVec.isLt
  refine ⟨by omega, Nat.mod_lt _ (by decide), Nat.mod_lt _ (by decide), ?_⟩
  have hdiv : bits.toNat = (bits.toNat / 2 ^ 52) * 2 ^ 52 + bits.toNat % 2 ^ 52 := by
    have hdm := Nat.div_add_mod bits.toNat (2 ^ 52)
    rw [Nat.mul_comm] at hdm
    exact hdm.symm
  have hq : bits.toNat / 2 ^ 52 =
      (bits.toNat / 2 ^ 63) * 2 ^ 11 + (bits.toNat / 2 ^ 52) % 2 ^ 11 := by
    have hdiv2 : (bits.toNat / 2 ^ 52) / 2 ^ 11 = bits.toNat / 2 ^ 63 := by
      rw [Nat.div_div_eq_div_mul]
    have hdm := Nat.div_add_mod (bits.toNat / 2 ^ 52) (2 ^ 11)
    rw [hdiv2, Nat.mul_comm] at hdm
    exact hdm.symm
  have hpow63 : (2 ^ 11 : Nat) * 2 ^ 52 = 2 ^ 63 := by decide
  conv => lhs; rw [hdiv, hq]
  rw [Nat.add_mul, Nat.mul_assoc, hpow63]

private theorem rat_signed_nat_eq_sign_pow (signBit : Nat) (hs : signBit < 2) (k : Nat) :
    (((if (signBit != 0) then -(k : Int) else (k : Int)) : Int) : Rat) =
      (-1 : Rat) ^ signBit * (k : Rat) := by
  have hcases : signBit = 0 ∨ signBit = 1 := by omega
  rcases hcases with rfl | rfl
  · simp [Rat.one_mul, Rat.intCast_natCast]
  · simp [Rat.neg_mul, Rat.one_mul, Rat.intCast_natCast]

private theorem rat_pow2_52_ne_zero : ((2 : Rat) ^ (52 : Nat)) ≠ 0 := by
  intro h
  have hcast : ((2 ^ 52 : Nat) : Rat) = (0 : Nat) := by
    rw [Rat.natCast_pow]
    exact h
  have hnat : (2 ^ 52 : Nat) = 0 := Rat.natCast_inj.mp hcast
  exact absurd hnat (by decide)

private theorem rat_two_ne_zero : (2 : Rat) ≠ 0 := by
  decide

private theorem float64Bits_sign_bne_zero (bits : UInt64) :
    ((bits >>> 63) != 0) = (ieee754Sign bits != 0) := by
  rw [← float64Bits_sign_eq bits]
  by_cases h : (bits >>> 63) = 0
  · simp [h]
  · have hne : (bits >>> 63).toNat ≠ 0 := by
      intro hz
      exact h (UInt64.eq_of_toBitVec_eq (BitVec.eq_of_toNat_eq hz))
    rw [bne_iff_ne.mpr h, bne_iff_ne.mpr hne]

/-- IEEE-754 subnormal and signed-zero value theorem for `float64BitsToRat`:
when `exp = 0`, `float64BitsToRat bits = some ((-1)^sign * mant * 2^(-1074))`. -/
theorem float64BitsToRat_subnormal {bits : UInt64} (he : ieee754Exp bits = 0) :
    float64BitsToRat bits =
      some ((-1 : Rat) ^ ieee754Sign bits * (ieee754Mant bits : Rat) * (2 : Rat) ^ (-1074 : Int)) := by
  unfold float64BitsToRat
  dsimp only
  have heEq := float64Bits_exp_eq bits
  have hmEq := float64Bits_mant_eq bits
  have hsLt : ieee754Sign bits < 2 := (float64Bits_field_decomp bits).1
  have hsignBool := float64Bits_sign_bne_zero bits
  rw [heEq, he, hmEq, hsignBool]
  simp only [show (0 == 0x7ff) = false by decide, Bool.false_eq_true, ↓reduceIte,
    show (0 == 0) = true by decide]
  by_cases hm : ieee754Mant bits = 0
  · rw [hm]
    simp only [beq_self_eq_true, ↓reduceIte]
    have hzero : (-1 : Rat) ^ ieee754Sign bits * ((0 : Nat) : Rat) * (2 : Rat) ^ (-1074 : Int) = 0 := by
      rw [show ((0 : Nat) : Rat) = 0 from rfl, Rat.mul_zero, Rat.zero_mul]
    rw [hzero]
  · have hmb : (ieee754Mant bits == 0) = false := beq_false_of_ne hm
    simp only [hmb, Bool.false_eq_true, ↓reduceIte]
    apply congrArg some
    rw [dyadicRat_eq_zpow, rat_signed_nat_eq_sign_pow (ieee754Sign bits) hsLt (ieee754Mant bits)]

/-- IEEE-754 normal value theorem for `float64BitsToRat`:
when `0 < exp < 2047`, `float64BitsToRat bits = some ((-1)^sign * (2^52 + mant) * 2^(exp - 1075))`. -/
theorem float64BitsToRat_normal {bits : UInt64}
    (hePos : 0 < ieee754Exp bits) (heLt : ieee754Exp bits < 0x7ff) :
    float64BitsToRat bits =
      some ((-1 : Rat) ^ ieee754Sign bits *
        ((2 ^ 52 + ieee754Mant bits : Nat) : Rat) *
        (2 : Rat) ^ ((ieee754Exp bits : Int) - 1075)) := by
  unfold float64BitsToRat
  dsimp only
  have heEq := float64Bits_exp_eq bits
  have hmEq := float64Bits_mant_eq bits
  have hsLt : ieee754Sign bits < 2 := (float64Bits_field_decomp bits).1
  have hsignBool := float64Bits_sign_bne_zero bits
  have hne7ff : (ieee754Exp bits == 0x7ff) = false := beq_false_of_ne (by omega)
  have hne0 : (ieee754Exp bits == 0) = false := beq_false_of_ne (by omega)
  have hshift52 : (1 <<< 52 : Nat) = 2 ^ 52 := by decide
  rw [heEq, hmEq, hsignBool, hne7ff, hne0, hshift52]
  simp only [Bool.false_eq_true, ↓reduceIte]
  apply congrArg some
  rw [dyadicRat_eq_zpow,
    rat_signed_nat_eq_sign_pow (ieee754Sign bits) hsLt (2 ^ 52 + ieee754Mant bits)]

/-- Equivalence between `float64BitsToRat` and the textbook IEEE-754-2008 binary64
value specification `ieee754Binary64Val` across all 2^64 bit patterns
(Inf/NaN, signed zeros, subnormals, and normals). -/
theorem float64BitsToRat_eq_ieee754 (bits : UInt64) :
    float64BitsToRat bits = ieee754Binary64Val bits := by
  unfold ieee754Binary64Val
  dsimp only
  have heLtAll : ieee754Exp bits < 2 ^ 11 := (float64Bits_field_decomp bits).2.1
  by_cases h7ff : ieee754Exp bits = 0x7ff
  · have hnone : (float64BitsToRat bits).isNone = true := by
      rw [float64BitsToRat_isNone_iff, float64Bits_exp_eq]
      exact h7ff
    cases hres : float64BitsToRat bits with
    | none =>
      simp [h7ff]
    | some r =>
      rw [hres] at hnone
      simp at hnone
  · have hne7ff : (ieee754Exp bits == 0x7ff) = false := beq_false_of_ne h7ff
    simp only [hne7ff, Bool.false_eq_true, ↓reduceIte]
    by_cases h0 : ieee754Exp bits = 0
    · simp only [h0, beq_self_eq_true, ↓reduceIte]
      rw [float64BitsToRat_subnormal h0]
      apply congrArg some
      have hpow : (2 : Rat) ^ (-1074 : Int) =
          ((2 : Rat) ^ (52 : Nat))⁻¹ * (2 : Rat) ^ (-1022 : Int) := by
        have hsplit : (-1074 : Int) = -((52 : Nat) : Int) + (-1022 : Int) := by omega
        rw [hsplit, Rat.zpow_add rat_two_ne_zero, Rat.zpow_neg, Rat.zpow_natCast]
      rw [hpow, Rat.div_def, ← Rat.mul_assoc, Rat.mul_assoc ((-1 : Rat) ^ ieee754Sign bits)]
    · have hne0 : (ieee754Exp bits == 0) = false := beq_false_of_ne h0
      simp only [hne0, Bool.false_eq_true, ↓reduceIte]
      have hePos : 0 < ieee754Exp bits := by omega
      have heLt : ieee754Exp bits < 0x7ff := by omega
      rw [float64BitsToRat_normal hePos heLt]
      apply congrArg some
      have hpow : (2 : Rat) ^ ((ieee754Exp bits : Int) - 1075) =
          ((2 : Rat) ^ (52 : Nat))⁻¹ * (2 : Rat) ^ ((ieee754Exp bits : Int) - 1023) := by
        have hsplit : ((ieee754Exp bits : Int) - 1075) =
            -((52 : Nat) : Int) + ((ieee754Exp bits : Int) - 1023) := by omega
        rw [hsplit, Rat.zpow_add rat_two_ne_zero, Rat.zpow_neg, Rat.zpow_natCast]
      have hsig :
          ((2 ^ 52 + ieee754Mant bits : Nat) : Rat) * ((2 : Rat) ^ (52 : Nat))⁻¹ =
          1 + (ieee754Mant bits : Rat) / (2 : Rat) ^ (52 : Nat) := by
        rw [Rat.natCast_add, Rat.natCast_pow, Rat.natCast_ofNat, Rat.add_mul,
          Rat.mul_inv_cancel _ rat_pow2_52_ne_zero, Rat.div_def]
      rw [hpow, ← Rat.mul_assoc, Rat.mul_assoc ((-1 : Rat) ^ ieee754Sign bits), hsig]

end GoblinFormal
