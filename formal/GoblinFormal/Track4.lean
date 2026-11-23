/-
GoblinFormal.Track4: Branch-and-Bound global bound monotonicity state machine
and verified Mini-VIPR rational MILP trace checker.
-/

import GoblinFormal.Common.Basic
import GoblinFormal.Common.Json

namespace GoblinFormal.Track4

open GoblinFormal

/-! ## Section 1: Problem Model & Rational Vector Primitives -/

/-- Relational sense for linear constraints. -/
inductive RowSense where
  | le
  | eq
  | ge
  deriving Repr, DecidableEq, Inhabited

/-- Fixed-length rational vector backed by a size-invariant `Array Rat`. -/
structure RatVec (n : Nat) where
  data : Array Rat
  size_eq : data.size = n
  deriving Repr

/-- Indexed lookup into a `RatVec n`. -/
@[inline]
def RatVec.get {n : Nat} (v : RatVec n) (j : Fin n) : Rat :=
  v.data[j.1]'(by rw [v.size_eq]; exact j.2)

/-- Constructs a `RatVec n` from a function on `Fin n`. -/
def RatVec.ofFn {n : Nat} (f : Fin n → Rat) : RatVec n :=
  ⟨Array.ofFn f, Array.size_ofFn⟩

@[simp]
theorem RatVec.get_ofFn {n : Nat} (f : Fin n → Rat) (j : Fin n) :
    (RatVec.ofFn f).get j = f j := by
  simp [RatVec.ofFn, RatVec.get, Array.getElem_ofFn]

theorem RatVec.ext {n : Nat} {a b : RatVec n} (h : ∀ j : Fin n, a.get j = b.get j) :
    a = b := by
  cases a with
  | mk da ha =>
    cases b with
    | mk db hb =>
      have hArr : da = db := by
        apply Array.ext
        · rw [ha, hb]
        · intro i hi1 _
          have hi : i < n := ha ▸ hi1
          exact h ⟨i, hi⟩
      subst hArr
      rfl

/-- Zero vector in `ℚ^n`. -/
def RatVec.zero (n : Nat) : RatVec n :=
  RatVec.ofFn (fun _ => 0)

@[simp]
theorem RatVec.get_zero {n : Nat} (j : Fin n) : (RatVec.zero n).get j = 0 := by
  simp [RatVec.zero]

/-- Standard basis vector `e_k` in `ℚ^n`. -/
def RatVec.unit (n : Nat) (k : Fin n) : RatVec n :=
  RatVec.ofFn (fun j => if j = k then 1 else 0)

@[simp]
theorem RatVec.get_unit {n : Nat} (k j : Fin n) :
    (RatVec.unit n k).get j = if j = k then 1 else 0 := by
  simp [RatVec.unit]

/-- Linear combination `a + m * b`. -/
def RatVec.addScaled {n : Nat} (a b : RatVec n) (m : Rat) : RatVec n :=
  RatVec.ofFn (fun j => a.get j + m * b.get j)

@[simp]
theorem RatVec.get_addScaled {n : Nat} (a b : RatVec n) (m : Rat) (j : Fin n) :
    (a.addScaled b m).get j = a.get j + m * b.get j := by
  simp [RatVec.addScaled]

/-- Exact componentwise equality test on `RatVec n`. -/
def RatVec.beq {n : Nat} (a b : RatVec n) : Bool :=
  loop 0
where
  loop (i : Nat) : Bool :=
    if hi : i < n then
      if a.data[i]'(by rw [a.size_eq]; exact hi) ==
          b.data[i]'(by rw [b.size_eq]; exact hi) then
        loop (i + 1)
      else
        false
    else
      true
  termination_by n - i

private theorem RatVec.beq_loop_sound {n : Nat} (a b : RatVec n) :
    ∀ k (hk : k ≤ n),
      (∀ i (hi : i < k),
        a.data[i]'(by rw [a.size_eq]; exact Nat.lt_of_lt_of_le hi hk) =
        b.data[i]'(by rw [b.size_eq]; exact Nat.lt_of_lt_of_le hi hk)) →
      RatVec.beq.loop a b k = true →
      ∀ j : Fin n, a.get j = b.get j := by
  intro k hk hprev hloop j
  unfold RatVec.beq.loop at hloop
  split at hloop
  · next hlt =>
    split at hloop
    · next heq =>
      have hcur : a.data[k]'(by rw [a.size_eq]; exact hlt) =
          b.data[k]'(by rw [b.size_eq]; exact hlt) := beq_iff_eq.mp heq
      have hnext : ∀ i (hi : i < k + 1),
          a.data[i]'(by rw [a.size_eq]; exact Nat.lt_of_lt_of_le hi hlt) =
          b.data[i]'(by rw [b.size_eq]; exact Nat.lt_of_lt_of_le hi hlt) := by
        intro i hi
        by_cases hik : i = k
        · subst hik; exact hcur
        · exact hprev i (by omega)
      exact beq_loop_sound a b (k + 1) hlt hnext hloop j
    · contradiction
  · next hge =>
    have hkn : k = n := by omega
    subst hkn
    exact hprev j.1 j.2
termination_by k _ _ _ => n - k

private theorem RatVec.beq_loop_refl {n : Nat} (a : RatVec n) (k : Nat) :
    RatVec.beq.loop a a k = true := by
  unfold RatVec.beq.loop
  split
  · simp
    exact RatVec.beq_loop_refl a (k + 1)
  · rfl
termination_by n - k

theorem RatVec.eq_of_beq {n : Nat} {a b : RatVec n} (h : a.beq b = true) : a = b := by
  apply RatVec.ext
  exact RatVec.beq_loop_sound a b 0 (Nat.zero_le n) (fun _ hi => by omega) h

theorem RatVec.beq_refl {n : Nat} (a : RatVec n) : a.beq a = true :=
  RatVec.beq_loop_refl a 0

theorem RatVec.beq_iff {n : Nat} (a b : RatVec n) : a.beq b = true ↔ a = b :=
  ⟨RatVec.eq_of_beq, fun h => h ▸ RatVec.beq_refl a⟩

/-- Exact dot product `∑ j, a_j * x_j` expressed via `finSum`. -/
def RatVec.dot {n : Nat} (a : RatVec n) (x : Fin n → Rat) : Rat :=
  finSum n (fun j => a.get j * x j)

theorem RatVec.dot_zero {n : Nat} (x : Fin n → Rat) : (RatVec.zero n).dot x = 0 := by
  unfold RatVec.dot
  have hcongr : finSum n (fun j => (RatVec.zero n).get j * x j) = finSum n (fun _ => (0 : Rat)) := by
    apply finSum_congr
    intro j
    simp [Rat.zero_mul]
  rw [hcongr, finSum_zero]

theorem RatVec.dot_unit {n : Nat} (k : Fin n) (x : Fin n → Rat) :
    (RatVec.unit n k).dot x = x k := by
  unfold RatVec.dot
  have hcongr :
      finSum n (fun j => (RatVec.unit n k).get j * x j) =
      finSum n (fun j => if j = k then x k else 0) := by
    apply finSum_congr
    intro j
    by_cases hjk : j = k
    · subst hjk; simp [Rat.one_mul]
    · simp [hjk, Rat.zero_mul]
  rw [hcongr, finSum_single]

theorem RatVec.dot_addScaled {n : Nat} (a b : RatVec n) (m : Rat) (x : Fin n → Rat) :
    (a.addScaled b m).dot x = a.dot x + m * b.dot x := by
  unfold RatVec.dot
  have hcongr :
      finSum n (fun j => (a.addScaled b m).get j * x j) =
      finSum n (fun j => a.get j * x j + m * (b.get j * x j)) := by
    apply finSum_congr
    intro j
    simp [RatVec.get_addScaled, Rat.add_mul, Rat.mul_assoc]
  rw [hcongr, finSum_add, finSum_const_mul]

/-- Tail-recursive evaluator for `a · x` over two `RatVec n` values. -/
def RatVec.dotVec {n : Nat} (a x : RatVec n) : Rat :=
  loop 0 0
where
  loop (i : Nat) (acc : Rat) : Rat :=
    if hi : i < n then
      let ai := a.data[i]'(by rw [a.size_eq]; exact hi)
      let xi := x.data[i]'(by rw [x.size_eq]; exact hi)
      loop (i + 1) (acc + ai * xi)
    else
      acc
  termination_by n - i

private def finPrefixSum (n : Nat) (f : Fin n → Rat) : (k : Nat) → k ≤ n → Rat
  | 0, _ => 0
  | k + 1, hk => finPrefixSum n f k (Nat.le_of_lt hk) + f ⟨k, hk⟩

private theorem finPrefixSum_eq_finSum (n : Nat) (f : Fin n → Rat) :
    ∀ k (hk : k ≤ n), finPrefixSum n f k hk = finSum k (fun i => f ⟨i.1, Nat.lt_of_lt_of_le i.2 hk⟩) := by
  intro k hk
  induction k with
  | zero => rfl
  | succ k ih =>
    simp only [finPrefixSum, finSum_succ, ih (Nat.le_of_lt hk)]
    rfl

private theorem dotVec_loop_eq {n : Nat} (a x : RatVec n) :
    ∀ k (hk : k ≤ n),
      RatVec.dotVec.loop a x k (finPrefixSum n (fun j => a.get j * x.get j) k hk) =
      finPrefixSum n (fun j => a.get j * x.get j) n (Nat.le_refl n) := by
  intro k hk
  unfold RatVec.dotVec.loop
  split
  · next hlt =>
    have hstep := dotVec_loop_eq a x (k + 1) hlt
    simpa [finPrefixSum, RatVec.get] using hstep
  · next hge =>
    have hkn : k = n := by omega
    subst hkn
    rfl
termination_by k _ => n - k

theorem RatVec.dotVec_eq_dot {n : Nat} (a x : RatVec n) :
    a.dotVec x = a.dot x.get := by
  unfold RatVec.dotVec RatVec.dot
  have h0 := dotVec_loop_eq a x 0 (Nat.zero_le n)
  have hfin := finPrefixSum_eq_finSum n (fun j => a.get j * x.get j) n (Nat.le_refl n)
  rw [finPrefixSum] at h0
  rw [h0, hfin]

private theorem rat_lt_of_lt_of_le {a b c : Rat} (hab : a < b) (hbc : b ≤ c) : a < c :=
  Rat.not_le.mp fun hca => Rat.not_le.mpr hab (Rat.le_trans hbc hca)

private theorem rat_lt_trans {a b c : Rat} (hab : a < b) (hbc : b < c) : a < c :=
  rat_lt_of_lt_of_le hab (Rat.le_of_lt hbc)

private theorem rat_mul_sub (a b c : Rat) : a * (b - c) = a * b - a * c := by
  rw [Rat.sub_eq_add_neg, Rat.mul_add, Rat.mul_neg, ← Rat.sub_eq_add_neg]

theorem finSum_swap (m n : Nat) (f : Fin m → Fin n → Rat) :
    finSum m (fun i => finSum n (fun j => f i j)) =
    finSum n (fun j => finSum m (fun i => f i j)) := by
  induction m with
  | zero =>
    simp [finSum_zero]
  | succ m ih =>
    simp only [finSum_succ]
    rw [ih (fun i j => f i.castSucc j)]
    rw [← finSum_add]

private theorem rat_div_le_iff {a b c : Rat} (hb : 0 < b) : a / b ≤ c ↔ a ≤ c * b := by
  constructor
  · intro h
    calc a = a / b * b := (Rat.div_mul_cancel (Rat.ne_of_gt hb)).symm
      _ ≤ c * b := Rat.mul_le_mul_of_nonneg_right h (Rat.le_of_lt hb)
  · intro h
    exact Rat.le_of_mul_le_mul_right (by rwa [Rat.div_mul_cancel (Rat.ne_of_gt hb)]) hb

private theorem rat_le_div_iff {a b c : Rat} (hc : 0 < c) : a ≤ b / c ↔ a * c ≤ b := by
  constructor
  · intro h
    calc a * c ≤ b / c * c := Rat.mul_le_mul_of_nonneg_right h (Rat.le_of_lt hc)
      _ = b := Rat.div_mul_cancel (Rat.ne_of_gt hc)
  · intro h
    exact Rat.le_of_mul_le_mul_right (by rwa [Rat.div_mul_cancel (Rat.ne_of_gt hc)]) hc

/-- Linear constraint `coeffs · x <sense> rhs` in `ℚ^n`. -/
structure LinCon (n : Nat) where
  coeffs : RatVec n
  sense : RowSense
  rhs : Rat
  deriving Repr

/-- Exact structural equality check on `LinCon n`. -/
def LinCon.beq {n : Nat} (a b : LinCon n) : Bool :=
  a.coeffs.beq b.coeffs && a.sense == b.sense && a.rhs == b.rhs

theorem LinCon.eq_of_beq {n : Nat} {a b : LinCon n} (h : a.beq b = true) : a = b := by
  unfold LinCon.beq at h
  simp only [Bool.and_eq_true, beq_iff_eq] at h
  rcases h with ⟨⟨hc, hs⟩, hr⟩
  have hcoeffs : a.coeffs = b.coeffs := RatVec.eq_of_beq hc
  cases a; cases b
  simp only at hcoeffs hs hr
  subst hcoeffs; subst hs; subst hr
  rfl

/-- Mathematical satisfaction of `con` at point `x : Fin n → Rat`. -/
def LinCon.sat {n : Nat} (con : LinCon n) (x : Fin n → Rat) : Prop :=
  let lhs := con.coeffs.dot x
  match con.sense with
  | .le => lhs ≤ con.rhs
  | .eq => lhs = con.rhs
  | .ge => con.rhs ≤ lhs

/-- Executable satisfaction check of `con` at `x : RatVec n`. -/
def LinCon.satBool {n : Nat} (con : LinCon n) (x : RatVec n) : Bool :=
  let lhs := con.coeffs.dotVec x
  match con.sense with
  | .le => decide (lhs ≤ con.rhs)
  | .eq => lhs == con.rhs
  | .ge => decide (con.rhs ≤ lhs)

theorem LinCon.satBool_iff {n : Nat} (con : LinCon n) (x : RatVec n) :
    con.satBool x = true ↔ con.sat x.get := by
  unfold LinCon.satBool LinCon.sat
  rw [RatVec.dotVec_eq_dot]
  cases con.sense <;> simp

/-- MILP problem instance over `ℚ^n` in minimization form. -/
structure VIPRInstance where
  n : Nat
  objConst : Rat
  obj : Fin n → Rat
  isInt : Fin n → Bool
  baseRows : Array (LinCon n)
  lb : Fin n → Option Rat
  ub : Fin n → Option Rat

/-- Integrality predicate on `Rat`. -/
def isRatInt (r : Rat) : Prop := ∃ z : Int, r = (z : Rat)

theorem isRatInt_iff_den_eq_one (r : Rat) : isRatInt r ↔ r.den = 1 := by
  constructor
  · rintro ⟨z, rfl⟩
    exact Rat.den_intCast z
  · intro hden
    refine ⟨r.num, ?_⟩
    have h := Rat.num_divInt_den r
    rw [hden] at h
    have h1 : Rat.divInt r.num ((1 : Nat) : Int) = (r.num : Rat) := by
      rw [Rat.divInt_ofNat]
      exact Rat.mkRat_one r.num
    change Rat.divInt r.num ((1 : Nat) : Int) = r at h
    rw [h1] at h
    exact h.symm

/-- Variable bound satisfaction for `(lb, ub)` at `x`. -/
def inVarBounds {n : Nat} (lb ub : Fin n → Option Rat) (x : Fin n → Rat) : Prop :=
  ∀ j : Fin n,
    (∀ l, lb j = some l → l ≤ x j) ∧
    (∀ u, ub j = some u → x j ≤ u)

/-- Mixed-integer linear feasibility of `x` for instance `P`. -/
def MILPFeasible (P : VIPRInstance) (x : Fin P.n → Rat) : Prop :=
  inVarBounds P.lb P.ub x ∧
  (∀ i : Fin P.baseRows.size, P.baseRows[i].sat x) ∧
  (∀ j : Fin P.n, P.isInt j = true → isRatInt (x j))

/-- Shifted minimization objective `objConst + ∑ obj_j * x_j`. -/
def LPObjective (P : VIPRInstance) (x : Fin P.n → Rat) : Rat :=
  P.objConst + finSum P.n (fun j => P.obj j * x j)


/-! ## Section 2: Abstract `babEngine` Bound Monotonicity State Machine -/

private theorem rat_le_min_iff {x a b : Rat} : x ≤ min a b ↔ x ≤ a ∧ x ≤ b := by
  rw [Rat.min_def]
  split
  · exact ⟨fun h => ⟨h, Rat.le_trans h ‹_›⟩, fun h => h.1⟩
  · exact ⟨fun h => ⟨Rat.le_trans h (Rat.le_of_lt (Rat.not_le.mp ‹_›)), h⟩, fun h => h.2⟩

private theorem rat_min_le_left (a b : Rat) : min a b ≤ a :=
  (rat_le_min_iff.mp Rat.le_refl).1

private theorem rat_min_le_right (a b : Rat) : min a b ≤ b :=
  (rat_le_min_iff.mp Rat.le_refl).2

private theorem rat_min_comm (a b : Rat) : min a b = min b a :=
  Rat.le_antisymm
    (rat_le_min_iff.mpr ⟨rat_min_le_right a b, rat_min_le_left a b⟩)
    (rat_le_min_iff.mpr ⟨rat_min_le_right b a, rat_min_le_left b a⟩)

private theorem rat_min_assoc (a b c : Rat) : min (min a b) c = min a (min b c) :=
  Rat.le_antisymm
    (rat_le_min_iff.mpr ⟨Rat.le_trans (rat_min_le_left _ _) (rat_min_le_left _ _),
      rat_le_min_iff.mpr ⟨Rat.le_trans (rat_min_le_left _ _) (rat_min_le_right _ _),
        rat_min_le_right _ _⟩⟩)
    (rat_le_min_iff.mpr ⟨rat_le_min_iff.mpr ⟨rat_min_le_left _ _,
        Rat.le_trans (rat_min_le_right _ _) (rat_min_le_left _ _)⟩,
      Rat.le_trans (rat_min_le_right _ _) (rat_min_le_right _ _)⟩)

private theorem rat_min_self (a : Rat) : min a a = a :=
  Rat.le_antisymm (rat_min_le_left a a) (rat_le_min_iff.mpr ⟨Rat.le_refl, Rat.le_refl⟩)

/-- Minimum over `Option Rat` where `none` represents `+∞`. -/
def minOpt (a b : Option Rat) : Option Rat :=
  match a, b with
  | none, y => y
  | x, none => x
  | some x, some y => some (min x y)

@[simp]
theorem minOpt_none_left (b : Option Rat) : minOpt none b = b := by
  cases b <;> rfl

@[simp]
theorem minOpt_none_right (a : Option Rat) : minOpt a none = a := by
  cases a <;> rfl

theorem minOpt_comm (a b : Option Rat) : minOpt a b = minOpt b a := by
  cases a <;> cases b <;> simp [minOpt, rat_min_comm]

theorem minOpt_assoc (a b c : Option Rat) : minOpt (minOpt a b) c = minOpt a (minOpt b c) := by
  cases a <;> cases b <;> cases c <;> simp [minOpt, rat_min_assoc]

theorem minOpt_self (a : Option Rat) : minOpt a a = a := by
  cases a <;> simp [minOpt, rat_min_self]

instance : Std.Associative minOpt := ⟨minOpt_assoc⟩
instance : Std.Commutative minOpt := ⟨minOpt_comm⟩

theorem minOpt_eq_none_iff (a b : Option Rat) : minOpt a b = none ↔ a = none ∧ b = none := by
  cases a <;> cases b <;> simp [minOpt]

theorem minOpt_le_left {a b : Option Rat} {x m : Rat}
    (ha : a = some x) (hm : minOpt a b = some m) : m ≤ x := by
  subst ha
  cases b with
  | none =>
    simp [minOpt] at hm
    subst hm
    exact Rat.le_refl
  | some y =>
    simp [minOpt] at hm
    subst hm
    exact rat_min_le_left x y

theorem minOpt_le_right {a b : Option Rat} {y m : Rat}
    (hb : b = some y) (hm : minOpt a b = some m) : m ≤ y := by
  rw [minOpt_comm] at hm
  exact minOpt_le_left hb hm

theorem minOpt_isSome_of_left {a : Option Rat} {x : Rat} (b : Option Rat) (ha : a = some x) :
    ∃ m, minOpt a b = some m := by
  subst ha
  cases b with
  | none => exact ⟨x, rfl⟩
  | some y => exact ⟨min x y, rfl⟩

theorem minOpt_isSome_of_right (a : Option Rat) {b : Option Rat} {y : Rat} (hb : b = some y) :
    ∃ m, minOpt a b = some m := by
  rw [minOpt_comm]
  exact minOpt_isSome_of_left a hb

/-- Ordering on `Option Rat` where `none` represents `+∞`. -/
def leOpt (a b : Option Rat) : Prop :=
  match a, b with
  | _, none => True
  | none, some _ => False
  | some x, some y => x ≤ y

theorem leOpt_refl (a : Option Rat) : leOpt a a := by
  cases a with
  | none => trivial
  | some _ => exact Rat.le_refl

theorem leOpt_trans {a b c : Option Rat} (hab : leOpt a b) (hbc : leOpt b c) : leOpt a c := by
  cases a <;> cases b <;> cases c <;> simp [leOpt] at hab hbc ⊢
  exact Rat.le_trans hab hbc

theorem minOpt_monotone {a1 a2 b1 b2 : Option Rat} (ha : leOpt a1 a2) (hb : leOpt b1 b2) :
    leOpt (minOpt a1 b1) (minOpt a2 b2) := by
  cases a1 <;> cases a2 <;> cases b1 <;> cases b2 <;> simp [leOpt, minOpt] at ha hb ⊢
  · exact hb
  · exact Rat.le_trans (rat_min_le_right _ _) hb
  · exact ha
  · exact Rat.le_trans (rat_min_le_left _ _) ha
  · exact rat_le_min_iff.mpr
      ⟨Rat.le_trans (rat_min_le_left _ _) ha, Rat.le_trans (rat_min_le_right _ _) hb⟩

/-- Tightens an optional lower bound (`none = -∞`) with finite `l`. -/
def tightenLBOpt (old : Option Rat) (l : Rat) : Option Rat :=
  match old with
  | none => some l
  | some l0 => some (max l0 l)

/-- Tightens an optional upper bound (`none = +∞`) with finite `u`. -/
def tightenUBOpt (old : Option Rat) (u : Rat) : Option Rat :=
  match old with
  | none => some u
  | some u0 => some (min u0 u)

/-- Search node in the abstract Branch-and-Bound tree. -/
structure BabNode (n : Nat) where
  lb : Fin n → Option Rat
  ub : Fin n → Option Rat
  cuts : (Fin n → Rat) → Prop := fun _ => True
  lowerBound : Rat

/-- Feasible domain of a `BabNode`. -/
def BabNode.domain {n : Nat} (nd : BabNode n) (x : Fin n → Rat) : Prop :=
  inVarBounds nd.lb nd.ub x ∧ nd.cuts x

/-- Tightens variable `col`'s upper bound to `≤ u` on `nd`. -/
def BabNode.tightenUB {n : Nat} (nd : BabNode n) (col : Fin n) (u : Rat) : BabNode n :=
  { nd with ub := fun j => if j = col then tightenUBOpt (nd.ub j) u else nd.ub j }

/-- Tightens variable `col`'s lower bound to `≥ l` on `nd`. -/
def BabNode.tightenLB {n : Nat} (nd : BabNode n) (col : Fin n) (l : Rat) : BabNode n :=
  { nd with lb := fun j => if j = col then tightenLBOpt (nd.lb j) l else nd.lb j }

theorem BabNode.domain_tightenUB {n : Nat} (nd : BabNode n) (col : Fin n) (u : Rat)
    (x : Fin n → Rat) (hdom : nd.domain x) (hx : x col ≤ u) :
    (nd.tightenUB col u).domain x := by
  refine ⟨fun j => ?_, hdom.2⟩
  refine ⟨(hdom.1 j).1, ?_⟩
  intro u' hu'
  dsimp [BabNode.tightenUB] at hu'
  split at hu'
  · next hj =>
    subst hj
    unfold tightenUBOpt at hu'
    cases h0 : nd.ub j with
    | none =>
      rw [h0] at hu'
      injection hu' with hEq
      subst hEq
      exact hx
    | some u0 =>
      rw [h0] at hu'
      injection hu' with hEq
      subst hEq
      simp only [Rat.min_def]
      split
      · exact (hdom.1 j).2 u0 h0
      · exact hx
  · exact (hdom.1 j).2 u' hu'

theorem BabNode.domain_tightenLB {n : Nat} (nd : BabNode n) (col : Fin n) (l : Rat)
    (x : Fin n → Rat) (hdom : nd.domain x) (hx : l ≤ x col) :
    (nd.tightenLB col l).domain x := by
  refine ⟨fun j => ?_, hdom.2⟩
  refine ⟨?_, (hdom.1 j).2⟩
  intro l' hl'
  dsimp [BabNode.tightenLB] at hl'
  split at hl'
  · next hj =>
    subst hj
    unfold tightenLBOpt at hl'
    cases h0 : nd.lb j with
    | none =>
      rw [h0] at hl'
      injection hl' with hEq
      subst hEq
      exact hx
    | some l0 =>
      rw [h0] at hl'
      injection hl' with hEq
      subst hEq
      simp only [Rat.max_def]
      split
      · exact hx
      · exact (hdom.1 j).1 l0 h0
  · exact (hdom.1 j).1 l' hl'

/-- Abstract `babEngine` state matching `bab.go:196-241` and `bab.go:2624-2635`. -/
structure BabState (n : Nat) where
  openQueue : List (BabNode n)
  plungeNode : Option (BabNode n)
  bestObjMin : Option Rat
  prunedMinBound : Option Rat
  unsolvedMinBound : Option Rat
  bestPrimal : Option (Fin n → Rat) := none

/-- Minimum lower bound across a list of open nodes. -/
def listMinBound {n : Nat} : List (BabNode n) → Option Rat
  | [] => none
  | nd :: rest => minOpt (some nd.lowerBound) (listMinBound rest)

theorem listMinBound_isSome_of_mem {n : Nat} {qs : List (BabNode n)} {nd : BabNode n}
    (hmem : nd ∈ qs) : ∃ m, listMinBound qs = some m := by
  cases qs with
  | nil => simp at hmem
  | cons q rest => exact minOpt_isSome_of_left (listMinBound rest) rfl

theorem listMinBound_le_mem {n : Nat} {qs : List (BabNode n)} {nd : BabNode n} {m : Rat}
    (hmem : nd ∈ qs) (hm : listMinBound qs = some m) : m ≤ nd.lowerBound := by
  induction qs generalizing m with
  | nil => simp at hmem
  | cons q rest ih =>
    simp only [listMinBound] at hm
    rcases List.mem_cons.mp hmem with rfl | hrest
    · exact minOpt_le_left rfl hm
    · obtain ⟨mr, hmr⟩ := listMinBound_isSome_of_mem hrest
      exact Rat.le_trans (minOpt_le_right hmr hm) (ih hrest hmr)

/-- Global dual lower bound matching `babEngine.currentGlobalBound` (`bab.go:2624`). -/
def currentGlobalBound {n : Nat} (s : BabState n) : Option Rat :=
  minOpt s.bestObjMin
    (minOpt s.unsolvedMinBound
      (minOpt s.prunedMinBound
        (minOpt (s.plungeNode.map (·.lowerBound)) (listMinBound s.openQueue))))

/-- Exact optimality gap predicate matching `isGapClosed` (`bab.go:3435`). -/
def isGapClosedSpec (lb ub relTol absTol : Rat) : Bool :=
  let diff := ub - lb
  if diff ≤ absTol then true
  else
    let denom := max (1 : Rat) ub.abs
    diff / denom ≤ relTol

/-- Optional-bound gap closure predicate (`none` means `+∞` / `-∞`, so open). -/
def isGapClosedOpt (lb ub : Option Rat) (relTol absTol : Rat) : Bool :=
  match lb, ub with
  | some l, some u => isGapClosedSpec l u relTol absTol
  | _, _ => false

/-- Global Branch-and-Bound coverage invariant. -/
def ValidBabState (P : VIPRInstance) (s : BabState P.n) : Prop :=
  (∀ x : Fin P.n → Rat, MILPFeasible P x →
    (∃ z, s.bestObjMin = some z ∧ z ≤ LPObjective P x) ∨
    (∃ p, s.prunedMinBound = some p ∧ p ≤ LPObjective P x) ∨
    (∃ u, s.unsolvedMinBound = some u ∧ u ≤ LPObjective P x) ∨
    (∃ nd, s.plungeNode = some nd ∧ nd.domain x ∧ nd.lowerBound ≤ LPObjective P x) ∨
    (∃ nd ∈ s.openQueue, nd.domain x ∧ nd.lowerBound ≤ LPObjective P x)) ∧
  (∀ xInc, s.bestPrimal = some xInc →
    MILPFeasible P xInc ∧ s.bestObjMin = some (LPObjective P xInc))

theorem currentGlobalBound_le_bestObjMin {n : Nat} {s : BabState n} {z b : Rat}
    (hz : s.bestObjMin = some z) (hb : currentGlobalBound s = some b) : b ≤ z :=
  minOpt_le_left hz hb

theorem currentGlobalBound_le_unsolved {n : Nat} {s : BabState n} {u b : Rat}
    (hu : s.unsolvedMinBound = some u) (hb : currentGlobalBound s = some b) : b ≤ u := by
  unfold currentGlobalBound at hb
  obtain ⟨m3, hm3⟩ := minOpt_isSome_of_left
    (minOpt s.prunedMinBound (minOpt (s.plungeNode.map (·.lowerBound)) (listMinBound s.openQueue))) hu
  exact Rat.le_trans (minOpt_le_right hm3 hb) (minOpt_le_left hu hm3)

private theorem currentGlobalBound_unfold_le {n : Nat} {s : BabState n} {b : Rat}
    (hb : currentGlobalBound s = some b) :
    (∀ z, s.bestObjMin = some z → b ≤ z) ∧
    (∀ u, s.unsolvedMinBound = some u → b ≤ u) ∧
    (∀ p, s.prunedMinBound = some p → b ≤ p) ∧
    (∀ nd, s.plungeNode = some nd → b ≤ nd.lowerBound) ∧
    (∀ nd ∈ s.openQueue, b ≤ nd.lowerBound) := by
  unfold currentGlobalBound at hb
  refine ⟨?_, ?_, ?_, ?_, ?_⟩
  · intro z hz
    exact minOpt_le_left hz hb
  · intro u hu
    obtain ⟨m3, hm3⟩ := minOpt_isSome_of_left
      (minOpt s.prunedMinBound (minOpt (s.plungeNode.map (·.lowerBound)) (listMinBound s.openQueue))) hu
    exact Rat.le_trans (minOpt_le_right hm3 hb) (minOpt_le_left hu hm3)
  · intro p hp
    obtain ⟨m2, hm2⟩ := minOpt_isSome_of_left
      (minOpt (s.plungeNode.map (·.lowerBound)) (listMinBound s.openQueue)) hp
    obtain ⟨m1, hm1⟩ := minOpt_isSome_of_right s.unsolvedMinBound hm2
    exact Rat.le_trans (minOpt_le_right hm1 hb)
      (Rat.le_trans (minOpt_le_right hm2 hm1) (minOpt_le_left hp hm2))
  · intro nd hnd
    have hpl : s.plungeNode.map (·.lowerBound) = some nd.lowerBound := by simp [hnd]
    obtain ⟨m3, hm3⟩ := minOpt_isSome_of_left (listMinBound s.openQueue) hpl
    obtain ⟨m2, hm2⟩ := minOpt_isSome_of_right s.prunedMinBound hm3
    obtain ⟨m1, hm1⟩ := minOpt_isSome_of_right s.unsolvedMinBound hm2
    exact Rat.le_trans (minOpt_le_right hm1 hb)
      (Rat.le_trans (minOpt_le_right hm2 hm1)
        (Rat.le_trans (minOpt_le_right hm3 hm2) (minOpt_le_left hpl hm3)))
  · intro nd hmem
    obtain ⟨mq, hmq⟩ := listMinBound_isSome_of_mem hmem
    obtain ⟨m3, hm3⟩ := minOpt_isSome_of_right (s.plungeNode.map (·.lowerBound)) hmq
    obtain ⟨m2, hm2⟩ := minOpt_isSome_of_right s.prunedMinBound hm3
    obtain ⟨m1, hm1⟩ := minOpt_isSome_of_right s.unsolvedMinBound hm2
    exact Rat.le_trans (minOpt_le_right hm1 hb)
      (Rat.le_trans (minOpt_le_right hm2 hm1)
        (Rat.le_trans (minOpt_le_right hm3 hm2)
          (Rat.le_trans (minOpt_le_right hmq hm3) (listMinBound_le_mem hmem hmq))))

/-- Soundness of `currentGlobalBound`: any finite global bound lower-bounds every MILP solution. -/
theorem bestBound_global_valid {P : VIPRInstance} {s : BabState P.n} {b : Rat}
    (hvalid : ValidBabState P s) (hb : currentGlobalBound s = some b) :
    ∀ x : Fin P.n → Rat, MILPFeasible P x → b ≤ LPObjective P x := by
  intro x hx
  have hle := currentGlobalBound_unfold_le hb
  rcases hvalid.1 x hx with
    ⟨z, hz, hzx⟩ | ⟨p, hp, hpx⟩ | ⟨u, hu, hux⟩ | ⟨nd, hnd, _, hndx⟩ | ⟨nd, hmem, _, hndx⟩
  · exact Rat.le_trans (hle.1 z hz) hzx
  · exact Rat.le_trans (hle.2.2.1 p hp) hpx
  · exact Rat.le_trans (hle.2.1 u hu) hux
  · exact Rat.le_trans (hle.2.2.2.1 nd hnd) hndx
  · exact Rat.le_trans (hle.2.2.2.2 nd hmem) hndx

/-- When `currentGlobalBound s = none`, all queues and bounds are empty, certifying infeasibility. -/
theorem infeasible_when_globalBound_none {P : VIPRInstance} {s : BabState P.n}
    (hvalid : ValidBabState P s) (hnone : currentGlobalBound s = none) :
    ¬ ∃ x : Fin P.n → Rat, MILPFeasible P x := by
  rintro ⟨x, hx⟩
  unfold currentGlobalBound at hnone
  obtain ⟨hbest, h1⟩ := (minOpt_eq_none_iff _ _).mp hnone
  obtain ⟨huns, h2⟩ := (minOpt_eq_none_iff _ _).mp h1
  obtain ⟨hprun, h3⟩ := (minOpt_eq_none_iff _ _).mp h2
  obtain ⟨hplunge, hq⟩ := (minOpt_eq_none_iff _ _).mp h3
  rcases hvalid.1 x hx with
    ⟨z, hz, _⟩ | ⟨p, hp, _⟩ | ⟨u, hu, _⟩ | ⟨nd, hnd, _, _⟩ | ⟨nd, hmem, _, _⟩
  · rw [hz] at hbest; contradiction
  · rw [hp] at hprun; contradiction
  · rw [hu] at huns; contradiction
  · rw [hnd] at hplunge; contradiction
  · obtain ⟨m, hm⟩ := listMinBound_isSome_of_mem hmem
    rw [hm] at hq; contradiction

/-- Root initialization from a valid root LP bound. -/
def stepRoot (P : VIPRInstance) (lb : Rat) : BabState P.n :=
  { openQueue := [{ lb := P.lb, ub := P.ub, cuts := fun _ => True, lowerBound := lb }]
    plungeNode := none
    bestObjMin := none
    prunedMinBound := none
    unsolvedMinBound := none
    bestPrimal := none }

theorem stepRoot_valid {P : VIPRInstance} {lb : Rat}
    (hlb : ∀ x, MILPFeasible P x → lb ≤ LPObjective P x) :
    ValidBabState P (stepRoot P lb) := by
  refine ⟨fun x hx => ?_, fun _ h => by contradiction⟩
  right; right; right; right
  refine ⟨{ lb := P.lb, ub := P.ub, cuts := fun _ => True, lowerBound := lb }, ?_, ?_, hlb x hx⟩
  · simp [stepRoot]
  · exact ⟨hx.1, trivial⟩

private def removeAt {α : Type} : List α → Nat → Option (α × List α)
  | [], _ => none
  | a :: rest, 0 => some (a, rest)
  | a :: rest, k + 1 => (removeAt rest k).map fun (x, rs) => (x, a :: rs)

private theorem removeAt_mem {α : Type} {xs rest : List α} {k : Nat} {x : α}
    (h : removeAt xs k = some (x, rest)) :
    x ∈ xs ∧ ∀ y, y ∈ xs ↔ (y = x ∨ y ∈ rest) := by
  induction xs generalizing k rest with
  | nil => simp [removeAt] at h
  | cons a tl ih =>
    cases k with
    | zero =>
      simp [removeAt] at h
      rcases h with ⟨rfl, rfl⟩
      exact ⟨by simp, fun y => by simp⟩
    | succ k' =>
      simp only [removeAt, Option.map_eq_some_iff] at h
      rcases h with ⟨⟨y0, rs0⟩, hrec, hpair⟩
      injection hpair with hx hrest
      subst hx; subst hrest
      obtain ⟨hxmem, hiff⟩ := ih hrec
      refine ⟨List.mem_cons_of_mem a hxmem, fun y => ?_⟩
      simp only [List.mem_cons, hiff y]
      exact ⟨fun
        | Or.inl hya => Or.inr (Or.inl hya)
        | Or.inr (Or.inl hy0) => Or.inl hy0
        | Or.inr (Or.inr hyr) => Or.inr (Or.inr hyr),
        fun
        | Or.inl hy0 => Or.inr (Or.inl hy0)
        | Or.inr (Or.inl hya) => Or.inl hya
        | Or.inr (Or.inr hyr) => Or.inr (Or.inr hyr)⟩

private theorem listMinBound_removeAt {n : Nat} {xs rest : List (BabNode n)} {k : Nat}
    {x : BabNode n} (h : removeAt xs k = some (x, rest)) :
    listMinBound xs = minOpt (some x.lowerBound) (listMinBound rest) := by
  induction xs generalizing k rest with
  | nil => simp [removeAt] at h
  | cons a tl ih =>
    cases k with
    | zero =>
      simp [removeAt] at h
      rcases h with ⟨rfl, rfl⟩
      rfl
    | succ k' =>
      simp only [removeAt, Option.map_eq_some_iff] at h
      rcases h with ⟨⟨y0, rs0⟩, hrec, hpair⟩
      injection hpair with hx hrest
      subst hx; subst hrest
      simp only [listMinBound, ih hrec]
      ac_rfl

/-- Selects node at `idx` from `openQueue` into `plungeNode`, pushing any prior `plungeNode`. -/
def stepSelectOpen {n : Nat} (s : BabState n) (idx : Nat) : BabState n :=
  match removeAt s.openQueue idx with
  | none => s
  | some (nd, rest) =>
    let q' := match s.plungeNode with
      | none => rest
      | some old => old :: rest
    { s with openQueue := q', plungeNode := some nd }

theorem stepSelectOpen_valid {P : VIPRInstance} {s : BabState P.n} (idx : Nat)
    (hvalid : ValidBabState P s) : ValidBabState P (stepSelectOpen s idx) := by
  unfold stepSelectOpen
  cases hrem : removeAt s.openQueue idx with
  | none => exact hvalid
  | some pair =>
    rcases pair with ⟨nd, rest⟩
    obtain ⟨_, hiff⟩ := removeAt_mem hrem
    refine ⟨fun x hx => ?_, hvalid.2⟩
    rcases hvalid.1 x hx with h | h | h | ⟨pnd, hpnd, hdom, hlb⟩ | ⟨qnd, hmem, hdom, hlb⟩
    · exact Or.inl h
    · exact Or.inr (Or.inl h)
    · exact Or.inr (Or.inr (Or.inl h))
    · right; right; right; right
      refine ⟨pnd, ?_, hdom, hlb⟩
      simp [hpnd]
    · rcases (hiff qnd).mp hmem with rfl | hrest
      · right; right; right; left
        exact ⟨qnd, rfl, hdom, hlb⟩
      · right; right; right; right
        refine ⟨qnd, ?_, hdom, hlb⟩
        cases s.plungeNode <;> simp [hrest]

theorem stepSelectOpen_preserves_bound {n : Nat} (s : BabState n) (idx : Nat) :
    currentGlobalBound (stepSelectOpen s idx) = currentGlobalBound s := by
  unfold stepSelectOpen
  cases hrem : removeAt s.openQueue idx with
  | none => rfl
  | some pair =>
    rcases pair with ⟨nd, rest⟩
    dsimp only [currentGlobalBound]
    rw [listMinBound_removeAt hrem]
    cases hp : s.plungeNode with
    | none => simp [minOpt]
    | some old =>
      simp only [Option.map_some, listMinBound]
      ac_rfl

/-- Tightens the active `plungeNode`'s lower bound via `max`. -/
def stepTightenLowerBound {n : Nat} (s : BabState n) (newLB : Rat) : BabState n :=
  match s.plungeNode with
  | none => s
  | some nd => { s with plungeNode := some { nd with lowerBound := max nd.lowerBound newLB } }

theorem stepTightenLowerBound_valid {P : VIPRInstance} {s : BabState P.n} {newLB : Rat}
    (hvalid : ValidBabState P s)
    (hlb : ∀ nd, s.plungeNode = some nd → ∀ x, MILPFeasible P x → nd.domain x → newLB ≤ LPObjective P x) :
    ValidBabState P (stepTightenLowerBound s newLB) := by
  unfold stepTightenLowerBound
  cases hp : s.plungeNode with
  | none => exact hvalid
  | some nd =>
    refine ⟨fun x hx => ?_, hvalid.2⟩
    rcases hvalid.1 x hx with h | h | h | ⟨pnd, hpnd, hdom, hlb0⟩ | h
    · exact Or.inl h
    · exact Or.inr (Or.inl h)
    · exact Or.inr (Or.inr (Or.inl h))
    · rw [hp] at hpnd; injection hpnd with heq; subst heq
      right; right; right; left
      refine ⟨{ nd with lowerBound := max nd.lowerBound newLB }, rfl, hdom, ?_⟩
      simp only [Rat.max_def]
      split
      · exact hlb nd hp x hx hdom
      · exact hlb0
    · exact Or.inr (Or.inr (Or.inr (Or.inr h)))

theorem stepTightenLowerBound_monotone {n : Nat} (s : BabState n) (newLB : Rat) :
    leOpt (currentGlobalBound s) (currentGlobalBound (stepTightenLowerBound s newLB)) := by
  unfold stepTightenLowerBound
  cases hp : s.plungeNode with
  | none => exact leOpt_refl _
  | some nd =>
    dsimp only [currentGlobalBound]
    rw [hp]
    apply minOpt_monotone (leOpt_refl _)
    apply minOpt_monotone (leOpt_refl _)
    apply minOpt_monotone (leOpt_refl _)
    apply minOpt_monotone _ (leOpt_refl _)
    simp only [Option.map_some, leOpt, Rat.max_def]
    split
    · assumption
    · exact Rat.le_refl

/-- Fathoms the active `plungeNode` when its domain contains no MILP-feasible point. -/
def stepFathomInfeas {n : Nat} (s : BabState n) : BabState n :=
  { s with plungeNode := none }

theorem stepFathomInfeas_valid {P : VIPRInstance} {s : BabState P.n}
    (hvalid : ValidBabState P s)
    (hinfeas : ∀ nd, s.plungeNode = some nd → ∀ x, MILPFeasible P x → ¬ nd.domain x) :
    ValidBabState P (stepFathomInfeas s) := by
  refine ⟨fun x hx => ?_, hvalid.2⟩
  rcases hvalid.1 x hx with h | h | h | ⟨nd, hnd, hdom, _⟩ | h
  · exact Or.inl h
  · exact Or.inr (Or.inl h)
  · exact Or.inr (Or.inr (Or.inl h))
  · exact absurd hdom (hinfeas nd hnd x hx)
  · exact Or.inr (Or.inr (Or.inr (Or.inr h)))

theorem stepFathomInfeas_monotone {n : Nat} (s : BabState n) :
    leOpt (currentGlobalBound s) (currentGlobalBound (stepFathomInfeas s)) := by
  unfold stepFathomInfeas currentGlobalBound
  dsimp only
  apply minOpt_monotone (leOpt_refl _)
  apply minOpt_monotone (leOpt_refl _)
  apply minOpt_monotone (leOpt_refl _)
  cases s.plungeNode <;> cases listMinBound s.openQueue <;> simp [minOpt, leOpt]
  exact rat_min_le_right _ _

/-- Fathoms the active `plungeNode` by bound, recording its lower bound in `prunedMinBound`. -/
def stepFathomBound {n : Nat} (s : BabState n) : BabState n :=
  match s.plungeNode with
  | none => s
  | some nd =>
    { s with
      plungeNode := none
      prunedMinBound := minOpt s.prunedMinBound (some nd.lowerBound) }

theorem stepFathomBound_valid {P : VIPRInstance} {s : BabState P.n}
    (hvalid : ValidBabState P s) :
    ValidBabState P (stepFathomBound s) := by
  unfold stepFathomBound
  cases hp : s.plungeNode with
  | none => exact hvalid
  | some nd =>
    refine ⟨fun x hx => ?_, hvalid.2⟩
    rcases hvalid.1 x hx with h | ⟨p, hpr, hpx⟩ | h | ⟨pnd, hpnd, _, hlb⟩ | h
    · exact Or.inl h
    · right; left
      obtain ⟨m, hm⟩ := minOpt_isSome_of_left (some nd.lowerBound) hpr
      exact ⟨m, hm, Rat.le_trans (minOpt_le_left hpr hm) hpx⟩
    · exact Or.inr (Or.inr (Or.inl h))
    · rw [hp] at hpnd; injection hpnd with heq; subst heq
      right; left
      obtain ⟨m, hm⟩ := minOpt_isSome_of_right s.prunedMinBound
        (rfl : some nd.lowerBound = some nd.lowerBound)
      exact ⟨m, hm, Rat.le_trans (minOpt_le_right rfl hm) hlb⟩
    · exact Or.inr (Or.inr (Or.inr (Or.inr h)))

theorem stepFathomBound_preserves_bound {n : Nat} (s : BabState n) :
    currentGlobalBound (stepFathomBound s) = currentGlobalBound s := by
  unfold stepFathomBound
  cases hp : s.plungeNode with
  | none => rfl
  | some nd =>
    dsimp only [currentGlobalBound]
    rw [hp]
    simp only [Option.map_some, Option.map_none, minOpt_none_left]
    ac_rfl

theorem int_split_exhaustive (v : Rat) (hv : isRatInt v) (k : Int) :
    v ≤ (k : Rat) ∨ ((k + 1 : Int) : Rat) ≤ v := by
  rcases hv with ⟨z, rfl⟩
  rw [Rat.intCast_le_intCast, Rat.intCast_le_intCast]
  omega

/-- Branches on integer variable `j` with split `x_j ≤ k` vs `x_j ≥ k + 1`. -/
def stepBranchInt {n : Nat} (s : BabState n) (j : Fin n) (k : Int) (chooseUpFirst : Bool) :
    BabState n :=
  match s.plungeNode with
  | none => s
  | some nd =>
    let downChild := nd.tightenUB j (k : Rat)
    let upChild := nd.tightenLB j ((k + 1 : Int) : Rat)
    if chooseUpFirst then
      { s with plungeNode := some upChild, openQueue := downChild :: s.openQueue }
    else
      { s with plungeNode := some downChild, openQueue := upChild :: s.openQueue }

theorem stepBranchInt_valid {P : VIPRInstance} {s : BabState P.n} {j : Fin P.n}
    (k : Int) (chooseUpFirst : Bool)
    (hvalid : ValidBabState P s) (hj : P.isInt j = true) :
    ValidBabState P (stepBranchInt s j k chooseUpFirst) := by
  unfold stepBranchInt
  cases hp : s.plungeNode with
  | none => exact hvalid
  | some nd =>
    have hinv : ∀ x, MILPFeasible P x →
        (∃ z, s.bestObjMin = some z ∧ z ≤ LPObjective P x) ∨
        (∃ p, s.prunedMinBound = some p ∧ p ≤ LPObjective P x) ∨
        (∃ u, s.unsolvedMinBound = some u ∧ u ≤ LPObjective P x) ∨
        (∃ cnd, (if chooseUpFirst then some (nd.tightenLB j ((k + 1 : Int) : Rat))
            else some (nd.tightenUB j (k : Rat))) = some cnd ∧
          cnd.domain x ∧ cnd.lowerBound ≤ LPObjective P x) ∨
        (∃ qnd ∈ (if chooseUpFirst then nd.tightenUB j (k : Rat) :: s.openQueue
            else nd.tightenLB j ((k + 1 : Int) : Rat) :: s.openQueue),
          qnd.domain x ∧ qnd.lowerBound ≤ LPObjective P x) := by
      intro x hx
      rcases hvalid.1 x hx with h | h | h | ⟨pnd, hpnd, hdom, hlb⟩ | ⟨qnd, hmem, hdom, hlb⟩
      · exact Or.inl h
      · exact Or.inr (Or.inl h)
      · exact Or.inr (Or.inr (Or.inl h))
      · rw [hp] at hpnd; injection hpnd with heq; subst heq
        rcases int_split_exhaustive (x j) (hx.2.2 j hj) k with hdown | hup
        · have hdomDown := BabNode.domain_tightenUB nd j (k : Rat) x hdom hdown
          cases chooseUpFirst
          · right; right; right; left
            exact ⟨nd.tightenUB j (k : Rat), rfl, hdomDown, hlb⟩
          · right; right; right; right
            exact ⟨nd.tightenUB j (k : Rat), by simp, hdomDown, hlb⟩
        · have hdomUp := BabNode.domain_tightenLB nd j ((k + 1 : Int) : Rat) x hdom hup
          cases chooseUpFirst
          · right; right; right; right
            exact ⟨nd.tightenLB j ((k + 1 : Int) : Rat), by simp, hdomUp, hlb⟩
          · right; right; right; left
            exact ⟨nd.tightenLB j ((k + 1 : Int) : Rat), rfl, hdomUp, hlb⟩
      · right; right; right; right
        cases chooseUpFirst
        · exact ⟨qnd, List.mem_cons_of_mem _ hmem, hdom, hlb⟩
        · exact ⟨qnd, List.mem_cons_of_mem _ hmem, hdom, hlb⟩
    cases chooseUpFirst
    · exact ⟨fun x hx => by simpa using hinv x hx, hvalid.2⟩
    · exact ⟨fun x hx => by simpa using hinv x hx, hvalid.2⟩

theorem stepBranchInt_preserves_bound {n : Nat} (s : BabState n) (j : Fin n) (k : Int)
    (chooseUpFirst : Bool) :
    currentGlobalBound (stepBranchInt s j k chooseUpFirst) = currentGlobalBound s := by
  unfold stepBranchInt
  cases hp : s.plungeNode with
  | none => rfl
  | some nd =>
    have hdup : ∀ a b, minOpt a (minOpt a b) = minOpt a b := by
      intro a b; rw [← minOpt_assoc, minOpt_self]
    cases chooseUpFirst
    · simp only [Bool.false_eq_true, ↓reduceIte]
      dsimp only [currentGlobalBound, listMinBound, BabNode.tightenUB, BabNode.tightenLB]
      rw [hp]
      simp only [Option.map_some, hdup]
    · simp only [↓reduceIte]
      dsimp only [currentGlobalBound, listMinBound, BabNode.tightenUB, BabNode.tightenLB]
      rw [hp]
      simp only [Option.map_some, hdup]

/-- Upper bound tightening soundness by reduced cost (`r_j > 0` at lower bound `l`). -/
theorem rc_fixing_ub_sound {P : VIPRInstance} {j : Fin P.n} {x : Fin P.n → Rat}
    {z0 zInc rj l : Rat}
    (hr : 0 < rj)
    (hint : isRatInt (x j))
    (hdual : z0 + rj * (x j - l) ≤ LPObjective P x)
    (hcut : LPObjective P x ≤ zInc) :
    x j ≤ ((l + (zInc - z0) / rj).floor : Rat) := by
  have h1 : rj * (x j - l) ≤ zInc - z0 := by
    have hle := Rat.le_trans hdual hcut
    exact Rat.le_sub_iff.mpr (by rw [Rat.add_comm]; exact hle)
  have h2 : x j - l ≤ (zInc - z0) / rj := by
    rw [rat_le_div_iff hr]
    rw [Rat.mul_comm]
    exact h1
  have h3 : x j ≤ (zInc - z0) / rj + l := Rat.sub_right_le_iff_le_add.mp h2
  rw [Rat.add_comm] at h3
  rcases hint with ⟨z, hz⟩
  rw [hz] at h3 ⊢
  rw [Rat.intCast_le_intCast]
  exact Rat.le_floor_iff.mpr h3

/-- Lower bound tightening soundness by reduced cost (`r_j < 0` at upper bound `u`). -/
theorem rc_fixing_lb_sound {P : VIPRInstance} {j : Fin P.n} {x : Fin P.n → Rat}
    {z0 zInc rj u : Rat}
    (hr : rj < 0)
    (hint : isRatInt (x j))
    (hdual : z0 + rj * (x j - u) ≤ LPObjective P x)
    (hcut : LPObjective P x ≤ zInc) :
    ((u - (zInc - z0) / (-rj)).ceil : Rat) ≤ x j := by
  have hpos : 0 < -rj := by
    rw [← Rat.neg_zero] at hr
    exact Rat.neg_lt_neg_iff.mpr hr
  have hrew : rj * (x j - u) = (-rj) * (u - x j) := by
    rw [Rat.neg_mul, ← Rat.mul_neg, Rat.neg_sub]
  have h1 : (-rj) * (u - x j) ≤ zInc - z0 := by
    rw [← hrew]
    have hle := Rat.le_trans hdual hcut
    exact Rat.le_sub_iff.mpr (by rw [Rat.add_comm]; exact hle)
  have h2 : u - x j ≤ (zInc - z0) / (-rj) := by
    rw [rat_le_div_iff hpos, Rat.mul_comm]
    exact h1
  have h3 : u ≤ (zInc - z0) / (-rj) + x j := Rat.sub_right_le_iff_le_add.mp h2
  have h4 : u - (zInc - z0) / (-rj) ≤ x j := by
    rw [Rat.add_comm] at h3
    exact Rat.sub_right_le_iff_le_add.mpr h3
  rcases hint with ⟨z, hz⟩
  rw [hz] at h4 ⊢
  rw [Rat.intCast_le_intCast]
  exact Rat.ceil_le_iff.mpr h4

/-- Combined reduced-cost fixing soundness for both upper- and lower-bound directions. -/
theorem rc_fixing_sound {P : VIPRInstance} {j : Fin P.n} {x : Fin P.n → Rat}
    {z0 zInc rj bd : Rat}
    (hint : isRatInt (x j))
    (hdual : z0 + rj * (x j - bd) ≤ LPObjective P x)
    (hcut : LPObjective P x < zInc) :
    (0 < rj → x j ≤ ((bd + (zInc - z0) / rj).floor : Rat)) ∧
    (rj < 0 → ((bd - (zInc - z0) / (-rj)).ceil : Rat) ≤ x j) :=
  ⟨fun hr => rc_fixing_ub_sound hr hint hdual (Rat.le_of_lt hcut),
   fun hr => rc_fixing_lb_sound hr hint hdual (Rat.le_of_lt hcut)⟩

/-- Dual-multiplier proof of the reduced-cost lower bound `z0 + r_j * (x_j - bd) ≤ LPObjective P x`. -/
theorem dual_rc_bound_valid {P : VIPRInstance} {y : Fin P.baseRows.size → Rat}
    {j : Fin P.n} {bd : Rat} {x : Fin P.n → Rat}
    (hx : MILPFeasible P x)
    (hy_sign : ∀ i : Fin P.baseRows.size,
      (P.baseRows[i].sense = .le → y i ≤ 0) ∧
      (P.baseRows[i].sense = .ge → 0 ≤ y i))
    (z0 : Rat)
    (hz0 : z0 ≤ P.objConst +
      finSum P.baseRows.size (fun i => y i * P.baseRows[i].rhs) +
      finSum P.n (fun k =>
        let rk := P.obj k - finSum P.baseRows.size (fun i => y i * P.baseRows[i].coeffs.get k)
        if k = j then rk * bd else rk * x k)) :
    z0 + (P.obj j - finSum P.baseRows.size (fun i => y i * P.baseRows[i].coeffs.get j)) * (x j - bd) ≤
      LPObjective P x := by
  let r : Fin P.n → Rat := fun k =>
    P.obj k - finSum P.baseRows.size (fun i => y i * P.baseRows[i].coeffs.get k)
  have hrow : ∀ i : Fin P.baseRows.size, y i * P.baseRows[i].rhs ≤ y i * P.baseRows[i].coeffs.dot x := by
    intro i
    have hsat := hx.2.1 i
    unfold LinCon.sat at hsat
    cases hs : P.baseRows[i].sense with
    | le =>
      rw [hs] at hsat
      have hneg : 0 ≤ -y i := Rat.le_neg_iff.mp (by simpa using (hy_sign i).1 hs)
      have hmul := Rat.mul_le_mul_of_nonneg_left hsat hneg
      rw [Rat.neg_mul, Rat.neg_mul, Rat.neg_le_neg_iff] at hmul
      exact hmul
    | eq =>
      rw [hs] at hsat
      rw [hsat]
      exact Rat.le_refl
    | ge =>
      rw [hs] at hsat
      have hpos : 0 ≤ y i := (hy_sign i).2 hs
      exact Rat.mul_le_mul_of_nonneg_left hsat hpos
  have hrowSum :
      finSum P.baseRows.size (fun i => y i * P.baseRows[i].rhs) ≤
      finSum P.baseRows.size (fun i => y i * P.baseRows[i].coeffs.dot x) :=
    finSum_le_finSum hrow
  have hswap :
      finSum P.baseRows.size (fun i => y i * P.baseRows[i].coeffs.dot x) =
      finSum P.n (fun k => finSum P.baseRows.size (fun i => y i * P.baseRows[i].coeffs.get k) * x k) := by
    have h1 :
        finSum P.baseRows.size (fun i => y i * P.baseRows[i].coeffs.dot x) =
        finSum P.baseRows.size (fun i => finSum P.n (fun k => (y i * P.baseRows[i].coeffs.get k) * x k)) := by
      apply finSum_congr
      intro i
      unfold RatVec.dot
      rw [← finSum_const_mul]
      apply finSum_congr
      intro k
      rw [Rat.mul_assoc]
    rw [h1, finSum_swap]
    apply finSum_congr
    intro k
    rw [← finSum_mul_const]
  have hrcSum :
      finSum P.n (fun k => if k = j then r k * bd else r k * x k) + r j * (x j - bd) =
      finSum P.n (fun k => r k * x k) := by
    have hdiff :
        finSum P.n (fun k => r k * x k) -
        finSum P.n (fun k => if k = j then r k * bd else r k * x k) =
        r j * (x j - bd) := by
      rw [← finSum_sub]
      have hsingle :
          finSum P.n (fun k => r k * x k - (if k = j then r k * bd else r k * x k)) =
          finSum P.n (fun k => if k = j then r j * (x j - bd) else 0) := by
        apply finSum_congr
        intro k
        by_cases hkj : k = j
        · subst hkj; simp [rat_mul_sub]
        · simp [hkj, Rat.sub_self]
      rw [hsingle, finSum_single]
    rw [← hdiff]
    rw [Rat.add_comm, Rat.sub_add_cancel]
  have hobjSplit :
      finSum P.baseRows.size (fun i => y i * P.baseRows[i].coeffs.dot x) +
      finSum P.n (fun k => r k * x k) =
      finSum P.n (fun k => P.obj k * x k) := by
    rw [hswap, ← finSum_add]
    apply finSum_congr
    intro k
    dsimp [r]
    rw [← Rat.add_mul, Rat.add_comm, Rat.sub_add_cancel]
  calc
    z0 + r j * (x j - bd)
      ≤ (P.objConst + finSum P.baseRows.size (fun i => y i * P.baseRows[i].rhs) +
          finSum P.n (fun k => if k = j then r k * bd else r k * x k)) + r j * (x j - bd) :=
        Rat.add_le_add_right.mpr hz0
    _ = P.objConst + finSum P.baseRows.size (fun i => y i * P.baseRows[i].rhs) +
          (finSum P.n (fun k => if k = j then r k * bd else r k * x k) + r j * (x j - bd)) := by
        rw [Rat.add_assoc]
    _ = P.objConst + finSum P.baseRows.size (fun i => y i * P.baseRows[i].rhs) +
          finSum P.n (fun k => r k * x k) := by
        rw [hrcSum]
    _ ≤ P.objConst + finSum P.baseRows.size (fun i => y i * P.baseRows[i].coeffs.dot x) +
          finSum P.n (fun k => r k * x k) :=
        Rat.add_le_add_right.mpr (Rat.add_le_add_left.mpr hrowSum)
    _ = P.objConst + (finSum P.baseRows.size (fun i => y i * P.baseRows[i].coeffs.dot x) +
          finSum P.n (fun k => r k * x k)) := by
        rw [Rat.add_assoc]
    _ = LPObjective P x := by
        rw [hobjSplit]; rfl

/-- Applies reduced-cost domain tightening to `plungeNode`. -/
def stepReducedCostFix {n : Nat} (s : BabState n) (tighten : BabNode n → BabNode n) : BabState n :=
  match s.plungeNode with
  | none => s
  | some nd => { s with plungeNode := some (tighten nd) }

theorem stepReducedCostFix_valid {P : VIPRInstance} {s : BabState P.n}
    (tighten : BabNode P.n → BabNode P.n) {zInc : Rat}
    (hvalid : ValidBabState P s)
    (hbest : s.bestObjMin = some zInc)
    (htighten : ∀ nd, s.plungeNode = some nd →
      (tighten nd).lowerBound = nd.lowerBound ∧
      ∀ x, MILPFeasible P x → nd.domain x → LPObjective P x < zInc → (tighten nd).domain x) :
    ValidBabState P (stepReducedCostFix s tighten) := by
  unfold stepReducedCostFix
  cases hp : s.plungeNode with
  | none => exact hvalid
  | some nd =>
    obtain ⟨hlbEq, hdomKeep⟩ := htighten nd hp
    refine ⟨fun x hx => ?_, hvalid.2⟩
    rcases hvalid.1 x hx with h | h | h | ⟨pnd, hpnd, hdom, hlb⟩ | h
    · exact Or.inl h
    · exact Or.inr (Or.inl h)
    · exact Or.inr (Or.inr (Or.inl h))
    · rw [hp] at hpnd; injection hpnd with heq; subst heq
      by_cases hlt : LPObjective P x < zInc
      · right; right; right; left
        exact ⟨tighten nd, rfl, hdomKeep x hx hdom hlt, hlbEq ▸ hlb⟩
      · left
        exact ⟨zInc, hbest, Rat.not_lt.mp hlt⟩
    · exact Or.inr (Or.inr (Or.inr (Or.inr h)))

theorem stepReducedCostFix_preserves_bound {n : Nat} (s : BabState n)
    (tighten : BabNode n → BabNode n)
    (hlb : ∀ nd, s.plungeNode = some nd → (tighten nd).lowerBound = nd.lowerBound) :
    currentGlobalBound (stepReducedCostFix s tighten) = currentGlobalBound s := by
  unfold stepReducedCostFix
  cases hp : s.plungeNode with
  | none => rfl
  | some nd =>
    simp only [currentGlobalBound, hp, Option.map_some, hlb nd hp]

/-- Fathoms `plungeNode` when reduced-cost fixing proves no strictly improving point remains. -/
theorem stepReducedCostFixInfeas_valid {P : VIPRInstance} {s : BabState P.n} {zInc : Rat}
    (hvalid : ValidBabState P s)
    (hbest : s.bestObjMin = some zInc)
    (hempty : ∀ nd, s.plungeNode = some nd →
      ∀ x, MILPFeasible P x → nd.domain x → zInc ≤ LPObjective P x) :
    ValidBabState P (stepFathomInfeas s) := by
  refine ⟨fun x hx => ?_, hvalid.2⟩
  rcases hvalid.1 x hx with h | h | h | ⟨nd, hnd, hdom, _⟩ | h
  · exact Or.inl h
  · exact Or.inr (Or.inl h)
  · exact Or.inr (Or.inr (Or.inl h))
  · left
    exact ⟨zInc, hbest, hempty nd hnd x hx hdom⟩
  · exact Or.inr (Or.inr (Or.inr (Or.inr h)))

/-- Updates `bestObjMin` and `bestPrimal` when `xInc` improves the incumbent (`bab.go:2689`). -/
def stepIncumbent (P : VIPRInstance) (s : BabState P.n) (xInc : Fin P.n → Rat) :
    BabState P.n :=
  let zInc := LPObjective P xInc
  match s.bestObjMin with
  | none => { s with bestObjMin := some zInc, bestPrimal := some xInc }
  | some z0 =>
    if zInc < z0 then
      { s with bestObjMin := some zInc, bestPrimal := some xInc }
    else
      s

theorem stepIncumbent_valid {P : VIPRInstance} {s : BabState P.n} (xInc : Fin P.n → Rat)
    (hvalid : ValidBabState P s) (hxInc : MILPFeasible P xInc) :
    ValidBabState P (stepIncumbent P s xInc) := by
  unfold stepIncumbent
  dsimp only
  cases hb : s.bestObjMin with
  | none =>
    simp only
    refine ⟨fun x hx => ?_, fun x' hx' => ?_⟩
    · rcases hvalid.1 x hx with ⟨z, hz, _⟩ | h | h | h | h
      · rw [hz] at hb; contradiction
      · exact Or.inr (Or.inl h)
      · exact Or.inr (Or.inr (Or.inl h))
      · exact Or.inr (Or.inr (Or.inr (Or.inl h)))
      · exact Or.inr (Or.inr (Or.inr (Or.inr h)))
    · injection hx' with heq; subst heq
      exact ⟨hxInc, rfl⟩
  | some z0 =>
    simp only
    by_cases hlt : LPObjective P xInc < z0
    · simp only [hlt, ↓reduceIte]
      refine ⟨fun x hx => ?_, fun x' hx' => ?_⟩
      · rcases hvalid.1 x hx with ⟨z, hz, hzx⟩ | h | h | h | h
        · rw [hz] at hb; injection hb with heq; subst heq
          left
          exact ⟨LPObjective P xInc, rfl, Rat.le_trans (Rat.le_of_lt hlt) hzx⟩
        · exact Or.inr (Or.inl h)
        · exact Or.inr (Or.inr (Or.inl h))
        · exact Or.inr (Or.inr (Or.inr (Or.inl h)))
        · exact Or.inr (Or.inr (Or.inr (Or.inr h)))
      · injection hx' with heq; subst heq
        exact ⟨hxInc, rfl⟩
    · simp only [hlt, ↓reduceIte]
      exact hvalid

theorem stepIncumbent_preserves_bound {P : VIPRInstance} {s : BabState P.n}
    (xInc : Fin P.n → Rat) (hvalid : ValidBabState P s) (hxInc : MILPFeasible P xInc) :
    currentGlobalBound (stepIncumbent P s xInc) = currentGlobalBound s := by
  unfold stepIncumbent
  dsimp only
  cases hb : s.bestObjMin with
  | none =>
    simp only
    cases hcur : currentGlobalBound s with
    | none =>
      exfalso
      exact infeasible_when_globalBound_none hvalid hcur ⟨xInc, hxInc⟩
    | some b =>
      have hble : b ≤ LPObjective P xInc := bestBound_global_valid hvalid hcur xInc hxInc
      unfold currentGlobalBound at hcur ⊢
      rw [hb, minOpt_none_left] at hcur
      dsimp only
      rw [hcur]
      have hmin_eq : min (LPObjective P xInc) b = b :=
        Rat.le_antisymm (rat_min_le_right _ _) (rat_le_min_iff.mpr ⟨hble, Rat.le_refl⟩)
      simp [minOpt, hmin_eq]
  | some z0 =>
    simp only
    by_cases hlt : LPObjective P xInc < z0
    · simp only [hlt, ↓reduceIte]
      cases hcur : currentGlobalBound s with
      | none =>
        exfalso
        exact infeasible_when_globalBound_none hvalid hcur ⟨xInc, hxInc⟩
      | some b =>
        have hble : b ≤ LPObjective P xInc := bestBound_global_valid hvalid hcur xInc hxInc
        unfold currentGlobalBound at hcur ⊢
        rw [hb] at hcur
        cases hrest : minOpt s.unsolvedMinBound
            (minOpt s.prunedMinBound (minOpt (s.plungeNode.map (·.lowerBound)) (listMinBound s.openQueue))) with
        | none =>
          rw [hrest, minOpt_none_right] at hcur
          injection hcur with hbz
          exfalso
          rw [hbz] at hlt
          exact Rat.not_le.mpr hlt hble
        | some mr =>
          rw [hrest, minOpt] at hcur
          injection hcur with hmin
          have hmr_lt : mr < z0 := by
            rcases Rat.le_total (a := z0) (b := mr) with hzmr | hmrz
            · have hmin_eq : min z0 mr = z0 :=
                Rat.le_antisymm (rat_min_le_left _ _) (rat_le_min_iff.mpr ⟨Rat.le_refl, hzmr⟩)
              have hbz : z0 = b := by rw [← hmin, hmin_eq]
              exfalso
              rw [hbz] at hlt
              exact Rat.not_le.mpr hlt hble
            · exact Rat.lt_of_le_of_ne hmrz (fun heq => by
                have hmin_eq : min z0 mr = z0 := by rw [← heq, rat_min_self]
                have hbz : z0 = b := by rw [← hmin, hmin_eq]
                rw [hbz] at hlt
                exact Rat.not_le.mpr hlt hble)
          have hbmr : mr = b := by
            have hmin_eq : min z0 mr = mr :=
              Rat.le_antisymm (rat_min_le_right _ _) (rat_le_min_iff.mpr ⟨Rat.le_of_lt hmr_lt, Rat.le_refl⟩)
            rw [← hmin, hmin_eq]
          have hm1 : min (LPObjective P xInc) mr = b := by
            rw [hbmr]
            exact Rat.le_antisymm (rat_min_le_right _ _) (rat_le_min_iff.mpr ⟨hble, Rat.le_refl⟩)
          simp [minOpt, hm1]
    · simp only [hlt, ↓reduceIte]

/-- Moves all unprocessed open/active nodes into `unsolvedMinBound` on budget exhaustion. -/
def stepBudgetAbort {n : Nat} (s : BabState n) : BabState n :=
  let rem := minOpt (s.plungeNode.map (·.lowerBound)) (listMinBound s.openQueue)
  { s with
    openQueue := []
    plungeNode := none
    unsolvedMinBound := minOpt s.unsolvedMinBound rem }

theorem stepBudgetAbort_valid {P : VIPRInstance} {s : BabState P.n}
    (hvalid : ValidBabState P s) : ValidBabState P (stepBudgetAbort s) := by
  unfold stepBudgetAbort
  dsimp only
  refine ⟨fun x hx => ?_, hvalid.2⟩
  rcases hvalid.1 x hx with h | h | ⟨u, hu, hux⟩ | ⟨nd, hnd, _, hlb⟩ | ⟨nd, hmem, _, hlb⟩
  · exact Or.inl h
  · exact Or.inr (Or.inl h)
  · right; right; left
    obtain ⟨m, hm⟩ := minOpt_isSome_of_left
      (minOpt (s.plungeNode.map (·.lowerBound)) (listMinBound s.openQueue)) hu
    exact ⟨m, hm, Rat.le_trans (minOpt_le_left hu hm) hux⟩
  · right; right; left
    have hpl : s.plungeNode.map (·.lowerBound) = some nd.lowerBound := by simp [hnd]
    obtain ⟨m1, hm1⟩ := minOpt_isSome_of_left (listMinBound s.openQueue) hpl
    obtain ⟨m2, hm2⟩ := minOpt_isSome_of_right s.unsolvedMinBound hm1
    exact ⟨m2, hm2, Rat.le_trans (minOpt_le_right hm1 hm2)
      (Rat.le_trans (minOpt_le_left hpl hm1) hlb)⟩
  · right; right; left
    obtain ⟨mq, hmq⟩ := listMinBound_isSome_of_mem hmem
    obtain ⟨m1, hm1⟩ := minOpt_isSome_of_right (s.plungeNode.map (·.lowerBound)) hmq
    obtain ⟨m2, hm2⟩ := minOpt_isSome_of_right s.unsolvedMinBound hm1
    exact ⟨m2, hm2, Rat.le_trans (minOpt_le_right hm1 hm2)
      (Rat.le_trans (minOpt_le_right hmq hm1) (Rat.le_trans (listMinBound_le_mem hmem hmq) hlb))⟩

theorem stepBudgetAbort_preserves_bound {n : Nat} (s : BabState n) :
    currentGlobalBound (stepBudgetAbort s) = currentGlobalBound s := by
  unfold stepBudgetAbort currentGlobalBound
  simp only [Option.map_none, listMinBound, minOpt_none_right]
  ac_rfl

/-- Terminal optimality theorem when all queues are drained and gap is closed. -/
theorem stepGapClosed_optimal {P : VIPRInstance} {s : BabState P.n} {xInc : Fin P.n → Rat}
    {lb relTol absTol : Rat}
    (hvalid : ValidBabState P s)
    (hq : s.openQueue = [])
    (hp : s.plungeNode = none)
    (hu : s.unsolvedMinBound = none)
    (hxInc : MILPFeasible P xInc)
    (hbest : s.bestObjMin = some (LPObjective P xInc))
    (hlb : currentGlobalBound s = some lb)
    (hgap : isGapClosedSpec lb (LPObjective P xInc) relTol absTol = true) :
    ∀ x : Fin P.n → Rat, MILPFeasible P x →
      LPObjective P xInc - LPObjective P x ≤ absTol ∨
      (LPObjective P xInc - LPObjective P x) / max (1 : Rat) (LPObjective P xInc).abs ≤ relTol := by
  have _ := hq; have _ := hp; have _ := hu; have _ := hxInc; have _ := hbest
  intro x hx
  have hx_lb : lb ≤ LPObjective P x := bestBound_global_valid hvalid hlb x hx
  have hdiff_le : LPObjective P xInc - LPObjective P x ≤ LPObjective P xInc - lb := by
    rw [Rat.sub_eq_add_neg, Rat.sub_eq_add_neg]
    exact Rat.add_le_add_left.mpr (Rat.neg_le_neg hx_lb)
  unfold isGapClosedSpec at hgap
  dsimp only at hgap
  split at hgap
  · next habs =>
    exact Or.inl (Rat.le_trans hdiff_le habs)
  · right
    have hdenom_pos : 0 < max (1 : Rat) (LPObjective P xInc).abs := by
      have h1 : (1 : Rat) ≤ max (1 : Rat) (LPObjective P xInc).abs := by
        rw [Rat.max_def]
        split
        · assumption
        · exact Rat.le_refl
      exact rat_lt_of_lt_of_le (by decide : (0 : Rat) < 1) h1
    have hrel : (LPObjective P xInc - lb) / max (1 : Rat) (LPObjective P xInc).abs ≤ relTol :=
      decide_eq_true_eq.mp hgap
    have hdiv_le :
        (LPObjective P xInc - LPObjective P x) / max (1 : Rat) (LPObjective P xInc).abs ≤
        (LPObjective P xInc - lb) / max (1 : Rat) (LPObjective P xInc).abs := by
      rw [Rat.div_def, Rat.div_def]
      exact Rat.mul_le_mul_of_nonneg_right hdiff_le
        (Rat.le_of_lt (Rat.inv_pos.mpr hdenom_pos))
    exact Rat.le_trans hdiv_le hrel

/-- Exact optimality specialization when `relTol = 0` and `absTol = 0`. -/
theorem stepGapClosed_exact_optimal {P : VIPRInstance} {s : BabState P.n} {xInc : Fin P.n → Rat}
    {lb : Rat}
    (hvalid : ValidBabState P s)
    (hq : s.openQueue = [])
    (hp : s.plungeNode = none)
    (hu : s.unsolvedMinBound = none)
    (hxInc : MILPFeasible P xInc)
    (hbest : s.bestObjMin = some (LPObjective P xInc))
    (hlb : currentGlobalBound s = some lb)
    (hgap : isGapClosedSpec lb (LPObjective P xInc) 0 0 = true) :
    ∀ x : Fin P.n → Rat, MILPFeasible P x → LPObjective P xInc ≤ LPObjective P x := by
  intro x hx
  rcases stepGapClosed_optimal hvalid hq hp hu hxInc hbest hlb hgap x hx with habs | hrel
  · rw [Rat.sub_eq_add_neg] at habs
    have := Rat.add_le_add_right (c := LPObjective P x) |>.mpr habs
    simpa [Rat.add_assoc, Rat.neg_add_cancel, Rat.zero_add, Rat.add_zero] using this
  · have hdenom_pos : 0 < max (1 : Rat) (LPObjective P xInc).abs := by
      have h1 : (1 : Rat) ≤ max (1 : Rat) (LPObjective P xInc).abs := by
        rw [Rat.max_def]
        split
        · assumption
        · exact Rat.le_refl
      exact rat_lt_of_lt_of_le (by decide : (0 : Rat) < 1) h1
    have hle0 : LPObjective P xInc - LPObjective P x ≤ 0 := by
      have hmul := (rat_div_le_iff hdenom_pos).mp hrel
      simpa [Rat.zero_mul] using hmul
    rw [Rat.sub_eq_add_neg] at hle0
    have := Rat.add_le_add_right (c := LPObjective P x) |>.mpr hle0
    simpa [Rat.add_assoc, Rat.neg_add_cancel, Rat.zero_add, Rat.add_zero] using this

/-! ## Section 3: Mini-VIPR Rational MILP Trace Checker & Soundness -/

/-- Certificate target mode: global infeasibility, certified optimal incumbent, or certified lower bound. -/
inductive CertMode (n : Nat) where
  | infeasible
  | optimal (incumbent : RatVec n)
  | bound (targetLB : Rat)
  deriving Repr

abbrev VIPRMode := CertMode

/-- Weighted reference to a constraint in the derivation table. -/
structure LinRef where
  id : Nat
  mult : Rat
  deriving Repr, DecidableEq

/-- Weighted reference to an initial variable bound (`P.lb` or `P.ub`). -/
structure VarBoundRef where
  col : Nat
  isUpper : Bool
  mult : Rat
  deriving Repr, DecidableEq

/-- Explicit node-automorphism witness for an orbital branching companion `c ≠ br`. -/
structure OrbitWitness where
  companion : Nat
  colPerm : Array Nat
  rowPerm : Array Nat
  supportEntries : Array Nat := #[]
  deriving Repr

/-- Supported derivation rules in a Mini-VIPR certificate. -/
inductive StepRule (n : Nat) where
  | conIndex (row : Nat)
  | assume (col : Nat) (isUpper : Bool) (bound : Int)
  | lincomb (sense : RowSense) (refs : Array LinRef) (bounds : Array VarBoundRef)
  | roundDown (ref : Nat)
  | roundUp (ref : Nat)
  | branch (col : Nat) (splitVal : Int) (leftAsm rightAsm left right : Nat)
  | orbitBranch (col : Nat) (leftAsm rightAsm left right : Nat)
      (orbitAsms : Array Nat) (witnesses : Array OrbitWitness)
      (extraAsms : Array Nat := #[])
  | farkas (refs : Array LinRef) (bounds : Array VarBoundRef)
  | boundConflict (col : Nat) (lbRef ubRef : Option Nat)
  | dualBound (refs : Array LinRef) (bounds : Array VarBoundRef)
  | safeDual (refs : Array LinRef) (boundAsms : Array Nat)
  deriving Repr

/-- Single proof step with optional claimed constraint and assumption check. -/
structure ProofStep (n : Nat) where
  rule : StepRule n
  claimedCon : Option (LinCon n) := none
  claimedAssumptions : Option (Array Nat) := none
  deriving Repr

/-- Complete Mini-VIPR rational MILP certificate. -/
structure MiniVIPRCert where
  inst : VIPRInstance
  mode : CertMode inst.n
  steps : Array (ProofStep inst.n)
  rootStep : Nat
  incumbent? : Option (RatVec inst.n) := none

abbrev MiniVIPRCert.prob (cert : MiniVIPRCert) : VIPRInstance := cert.inst

/-- Verified derived entry with its active assumption ID set. -/
structure DerivedEntry (n : Nat) where
  con : LinCon n
  assumptions : List Nat
  deriving Repr

/-- Checks whether `x : RatVec P.n` satisfies all variable bounds, base rows, and integrality. -/
def checkIncumbentFeasible (P : VIPRInstance) (x : RatVec P.n) : Bool :=
  checkBounds 0 && checkRows 0
where
  checkBounds (i : Nat) : Bool :=
    if hi : i < P.n then
      let j : Fin P.n := ⟨i, hi⟩
      let xi := x.get j
      let okL := match P.lb j with | none => true | some l => decide (l ≤ xi)
      let okU := match P.ub j with | none => true | some u => decide (xi ≤ u)
      let okI := if P.isInt j then xi.den == 1 else true
      if okL && okU && okI then checkBounds (i + 1) else false
    else true
  termination_by P.n - i
  checkRows (i : Nat) : Bool :=
    if hi : i < P.baseRows.size then
      if P.baseRows[i].satBool x then checkRows (i + 1) else false
    else true
  termination_by P.baseRows.size - i

private theorem checkBounds_sound (P : VIPRInstance) (x : RatVec P.n) :
    ∀ k (hk : k ≤ P.n),
      (∀ i (hi : i < k),
        let j : Fin P.n := ⟨i, Nat.lt_of_lt_of_le hi hk⟩
        (∀ l, P.lb j = some l → l ≤ x.get j) ∧
        (∀ u, P.ub j = some u → x.get j ≤ u) ∧
        (P.isInt j = true → isRatInt (x.get j))) →
      checkIncumbentFeasible.checkBounds P x k = true →
      ∀ j : Fin P.n,
        ((∀ l, P.lb j = some l → l ≤ x.get j) ∧
         (∀ u, P.ub j = some u → x.get j ≤ u)) ∧
        (P.isInt j = true → isRatInt (x.get j)) := by
  intro k hk hprev hloop j
  unfold checkIncumbentFeasible.checkBounds at hloop
  dsimp only at hloop
  split at hloop
  · next hlt =>
    by_cases hok :
        ((match P.lb ⟨k, hlt⟩ with | none => true | some l => decide (l ≤ x.get ⟨k, hlt⟩)) &&
         (match P.ub ⟨k, hlt⟩ with | none => true | some u => decide (x.get ⟨k, hlt⟩ ≤ u)) &&
         (if P.isInt ⟨k, hlt⟩ then (x.get ⟨k, hlt⟩).den == 1 else true)) = true
    · simp only [hok, ↓reduceIte] at hloop
      simp only [Bool.and_eq_true] at hok
      rcases hok with ⟨⟨hokL, hokU⟩, hokI⟩
      have hcurL : ∀ l, P.lb ⟨k, hlt⟩ = some l → l ≤ x.get ⟨k, hlt⟩ := by
        intro l hl
        rw [hl] at hokL
        exact decide_eq_true_eq.mp hokL
      have hcurU : ∀ u, P.ub ⟨k, hlt⟩ = some u → x.get ⟨k, hlt⟩ ≤ u := by
        intro u hu
        rw [hu] at hokU
        exact decide_eq_true_eq.mp hokU
      have hcurI : P.isInt ⟨k, hlt⟩ = true → isRatInt (x.get ⟨k, hlt⟩) := by
        intro hint
        rw [hint] at hokI
        exact (isRatInt_iff_den_eq_one _).mpr (beq_iff_eq.mp hokI)
      have hnext : ∀ i (hi : i < k + 1),
          let ji : Fin P.n := ⟨i, Nat.lt_of_lt_of_le hi hlt⟩
          (∀ l, P.lb ji = some l → l ≤ x.get ji) ∧
          (∀ u, P.ub ji = some u → x.get ji ≤ u) ∧
          (P.isInt ji = true → isRatInt (x.get ji)) := by
        intro i hi
        by_cases hik : i = k
        · subst hik; exact ⟨hcurL, hcurU, hcurI⟩
        · exact hprev i (by omega)
      exact checkBounds_sound P x (k + 1) hlt hnext hloop j
    · simp only [hok] at hloop
      contradiction
  · next hge =>
    have hkn : k = P.n := by omega
    subst hkn
    have h := hprev j.1 j.2
    exact ⟨⟨h.1, h.2.1⟩, h.2.2⟩
termination_by k _ _ _ => P.n - k

private theorem checkRows_sound (P : VIPRInstance) (x : RatVec P.n) :
    ∀ k (hk : k ≤ P.baseRows.size),
      (∀ i (hi : i < k), P.baseRows[i].sat x.get) →
      checkIncumbentFeasible.checkRows P x k = true →
      ∀ i : Fin P.baseRows.size, P.baseRows[i].sat x.get := by
  intro k hk hprev hloop i
  unfold checkIncumbentFeasible.checkRows at hloop
  split at hloop
  · next hlt =>
    split at hloop
    · next hsat =>
      have hcur : P.baseRows[k].sat x.get := (LinCon.satBool_iff _ _).mp hsat
      have hnext : ∀ m (hm : m < k + 1), P.baseRows[m].sat x.get := by
        intro m hm
        by_cases hmk : m = k
        · subst hmk; exact hcur
        · exact hprev m (by omega)
      exact checkRows_sound P x (k + 1) hlt hnext hloop i
    · contradiction
  · next hge =>
    have hkEq : k = P.baseRows.size := by omega
    subst hkEq
    exact hprev i.1 i.2
termination_by k _ _ _ => P.baseRows.size - k

theorem checkIncumbentFeasible_sound (P : VIPRInstance) (x : RatVec P.n)
    (h : checkIncumbentFeasible P x = true) : MILPFeasible P x.get := by
  unfold checkIncumbentFeasible at h
  simp only [Bool.and_eq_true] at h
  have hb := checkBounds_sound P x 0 (Nat.zero_le P.n) (fun _ hi => by omega) h.1
  have hr := checkRows_sound P x 0 (Nat.zero_le P.baseRows.size) (fun _ hi => by omega) h.2
  exact ⟨fun j => (hb j).1, hr, fun j hj => (hb j).2 hj⟩

/-- Deduplicated union of assumption IDs. -/
def mergeAsms (a b : List Nat) : List Nat :=
  a ++ b.filter (fun x => !a.contains x)

theorem mem_mergeAsms {x : Nat} {a b : List Nat} :
    x ∈ mergeAsms a b ↔ x ∈ a ∨ x ∈ b := by
  unfold mergeAsms
  simp only [List.mem_append, List.mem_filter, Bool.not_eq_true']
  constructor
  · rintro (ha | ⟨hb, _⟩)
    · exact Or.inl ha
    · exact Or.inr hb
  · rintro (ha | hb)
    · exact Or.inl ha
    · by_cases hca : a.contains x = true
      · exact Or.inl (List.contains_iff_mem.mp hca)
      · exact Or.inr ⟨hb, Bool.eq_false_iff.mpr hca⟩

/-- Checks exact set equality between computed assumption IDs and claimed IDs. -/
def checkAssumptionsMatch (actual : List Nat) (claimed : Array Nat) : Bool :=
  actual.all (fun a => claimed.contains a) &&
  claimed.toList.all (fun a => actual.contains a)

/-- Contradiction predicate for a linear constraint (`0 · x <sense> rhs` inconsistent with `0`). -/
def isUnsatCon {n : Nat} (con : LinCon n) : Bool :=
  con.coeffs.beq (RatVec.zero n) &&
  match con.sense with
  | .le => decide (con.rhs < 0)
  | .ge => decide (0 < con.rhs)
  | .eq => con.rhs != 0

theorem isUnsatCon_sound {n : Nat} {con : LinCon n} (h : isUnsatCon con = true)
    (x : Fin n → Rat) : ¬ con.sat x := by
  unfold isUnsatCon at h
  simp only [Bool.and_eq_true] at h
  have hc : con.coeffs = RatVec.zero n := RatVec.eq_of_beq h.1
  unfold LinCon.sat
  rw [hc, RatVec.dot_zero]
  have hr := h.2
  cases hs : con.sense with
  | le =>
    rw [hs] at hr
    exact Rat.not_le.mpr (decide_eq_true_eq.mp hr)
  | ge =>
    rw [hs] at hr
    exact Rat.not_le.mpr (decide_eq_true_eq.mp hr)
  | eq =>
    rw [hs] at hr
    exact Ne.symm (bne_iff_ne.mp hr)

/-- Multiplier sign legality relative to target and source constraint senses. -/
def legalMult (targetSense rowSense : RowSense) (m : Rat) : Bool :=
  match targetSense with
  | .le =>
    match rowSense with
    | .le => decide (0 ≤ m)
    | .ge => decide (m ≤ 0)
    | .eq => true
  | .ge =>
    match rowSense with
    | .le => decide (m ≤ 0)
    | .ge => decide (0 ≤ m)
    | .eq => true
  | .eq => rowSense == .eq

private def linSatSense {n : Nat} (v : RatVec n) (sense : RowSense) (rhs : Rat)
    (x : Fin n → Rat) : Prop :=
  match sense with
  | .le => v.dot x ≤ rhs
  | .ge => rhs ≤ v.dot x
  | .eq => v.dot x = rhs

private theorem linSatSense_zero {n : Nat} (sense : RowSense) (x : Fin n → Rat) :
    linSatSense (RatVec.zero n) sense 0 x := by
  cases sense <;> simp [linSatSense, RatVec.dot_zero]

private theorem linSatSense_step {n : Nat} {v : RatVec n} {c : LinCon n}
    {targetSense : RowSense} {rhs m : Rat} {x : Fin n → Rat}
    (hv : linSatSense v targetSense rhs x)
    (hc : c.sat x)
    (hm : m = 0 ∨ legalMult targetSense c.sense m = true) :
    linSatSense (v.addScaled c.coeffs m) targetSense (rhs + m * c.rhs) x := by
  have hscaled : linSatSense (RatVec.ofFn fun j => m * c.coeffs.get j) targetSense (m * c.rhs) x := by
    have hdot : (RatVec.ofFn fun j => m * c.coeffs.get j).dot x = m * c.coeffs.dot x := by
      unfold RatVec.dot
      simp only [RatVec.get_ofFn, Rat.mul_assoc]
      exact finSum_const_mul n m (fun j => c.coeffs.get j * x j)
    unfold linSatSense
    rw [hdot]
    rcases hm with rfl | hleg
    · cases targetSense <;> simp [Rat.zero_mul]
    · unfold LinCon.sat at hc
      cases targetSense with
      | le =>
        cases hs : c.sense with
        | le =>
          rw [hs] at hc hleg
          exact Rat.mul_le_mul_of_nonneg_left hc (decide_eq_true_eq.mp hleg)
        | ge =>
          rw [hs] at hc hleg
          have hneg : 0 ≤ -m := Rat.le_neg_iff.mp (by simpa using decide_eq_true_eq.mp hleg)
          have hmul := Rat.mul_le_mul_of_nonneg_left hc hneg
          rw [Rat.neg_mul, Rat.neg_mul, Rat.neg_le_neg_iff] at hmul
          exact hmul
        | eq =>
          rw [hs] at hc
          rw [hc]
          exact Rat.le_refl
      | ge =>
        cases hs : c.sense with
        | le =>
          rw [hs] at hc hleg
          have hneg : 0 ≤ -m := Rat.le_neg_iff.mp (by simpa using decide_eq_true_eq.mp hleg)
          have hmul := Rat.mul_le_mul_of_nonneg_left hc hneg
          rw [Rat.neg_mul, Rat.neg_mul, Rat.neg_le_neg_iff] at hmul
          exact hmul
        | ge =>
          rw [hs] at hc hleg
          exact Rat.mul_le_mul_of_nonneg_left hc (decide_eq_true_eq.mp hleg)
        | eq =>
          rw [hs] at hc
          rw [hc]
          exact Rat.le_refl
      | eq =>
        cases hs : c.sense with
        | le => simp [legalMult, hs] at hleg
        | ge => simp [legalMult, hs] at hleg
        | eq => rw [hs] at hc; rw [hc]
  unfold linSatSense at hv hscaled ⊢
  rw [RatVec.dot_addScaled]
  have hdot : (RatVec.ofFn fun j => m * c.coeffs.get j).dot x = m * c.coeffs.dot x := by
    unfold RatVec.dot
    simp only [RatVec.get_ofFn, Rat.mul_assoc]
    exact finSum_const_mul n m (fun j => c.coeffs.get j * x j)
  rw [hdot] at hscaled
  cases targetSense with
  | le =>
    exact Rat.le_trans (Rat.add_le_add_right.mpr hv) (Rat.add_le_add_left.mpr hscaled)
  | ge =>
    exact Rat.le_trans (Rat.add_le_add_right.mpr hv) (Rat.add_le_add_left.mpr hscaled)
  | eq =>
    rw [hv, hscaled]

/-- Active assumption satisfaction over `entries`. -/
def asmsSat {n : Nat} (entries : Array (DerivedEntry n)) (asms : List Nat)
    (x : Fin n → Rat) : Prop :=
  ∀ a ∈ asms, ∃ ha : a < entries.size, entries[a].con.sat x

theorem asmsSat_mono {n : Nat} {entries : Array (DerivedEntry n)} {a1 a2 : List Nat}
    {x : Fin n → Rat} (hsub : ∀ a ∈ a1, a ∈ a2) (hsat : asmsSat entries a2 x) :
    asmsSat entries a1 x :=
  fun a ha => hsat a (hsub a ha)

/-- Validity invariant for every entry in `entries`. -/
def entryValid (P : VIPRInstance) (entries : Array (DerivedEntry P.n))
    (e : DerivedEntry P.n) : Prop :=
  (∀ a ∈ e.assumptions, a < entries.size) ∧
  (∀ x : Fin P.n → Rat, MILPFeasible P x → asmsSat entries e.assumptions x → e.con.sat x)

def ValidTable (P : VIPRInstance) (entries : Array (DerivedEntry P.n)) : Prop :=
  ∀ i (hi : i < entries.size), entryValid P entries entries[i]

theorem validTable_init (P : VIPRInstance) :
    ValidTable P (P.baseRows.map fun r => { con := r, assumptions := [] }) := by
  intro i hi
  simp only [Array.size_map] at hi
  simp only [Array.getElem_map]
  refine ⟨fun a ha => by simp at ha, fun x hx _ => ?_⟩
  exact hx.2.1 ⟨i, hi⟩

theorem validTable_push {P : VIPRInstance} {entries : Array (DerivedEntry P.n)}
    (ht : ValidTable P entries) (e : DerivedEntry P.n)
    (hsub : ∀ a ∈ e.assumptions, a < entries.size + 1)
    (hsat : ∀ x : Fin P.n → Rat, MILPFeasible P x →
      asmsSat (entries.push e) e.assumptions x → e.con.sat x) :
    ValidTable P (entries.push e) := by
  intro i hi
  rw [Array.size_push] at hi
  by_cases hlt : i < entries.size
  · have hget : (entries.push e)[i] = entries[i] := Array.getElem_push_lt hlt
    rw [hget]
    obtain ⟨haLt, hvalid⟩ := ht i hlt
    refine ⟨fun a ha => ?_, fun x hx hasms => ?_⟩
    · rw [Array.size_push]; exact Nat.lt_succ_of_lt (haLt a ha)
    · apply hvalid x hx
      intro a ha
      have halt : a < entries.size := haLt a ha
      obtain ⟨_, hsa⟩ := hasms a ha
      have hgeta : (entries.push e)[a] = entries[a] := Array.getElem_push_lt halt
      rw [hgeta] at hsa
      exact ⟨halt, hsa⟩
  · have heq : i = entries.size := by omega
    subst heq
    have hget : (entries.push e)[entries.size] = e := Array.getElem_push_eq
    rw [hget]
    exact ⟨fun a ha => by rw [Array.size_push]; exact hsub a ha, hsat⟩

private def stepLinBound (P : VIPRInstance) (sense : RowSense)
    (v : RatVec P.n) (rhs : Rat) (br : VarBoundRef) : Option (RatVec P.n × Rat) :=
  if br.mult == 0 then
    some (v, rhs)
  else if hcol : br.col < P.n then
    let j : Fin P.n := ⟨br.col, hcol⟩
    if br.isUpper then
      match P.ub j with
      | some u =>
        if legalMult sense .le br.mult then
          some (v.addScaled (RatVec.unit P.n j) br.mult, rhs + br.mult * u)
        else none
      | none => none
    else
      match P.lb j with
      | some l =>
        if legalMult sense .ge br.mult then
          some (v.addScaled (RatVec.unit P.n j) br.mult, rhs + br.mult * l)
        else none
      | none => none
  else none

/-- Evaluates a linear combination of `refs` and `bounds`. -/
def evalLinComb (P : VIPRInstance) (entries : Array (DerivedEntry P.n))
    (sense : RowSense) (refs : Array LinRef) (bounds : Array VarBoundRef) :
    Option (DerivedEntry P.n) :=
  match goRefs 0 (RatVec.zero P.n) 0 [] with
  | none => none
  | some (v1, r1, asms1) =>
    match goBounds 0 v1 r1 with
    | none => none
    | some (v2, r2) =>
      some { con := { coeffs := v2, sense := sense, rhs := r2 }, assumptions := asms1 }
where
  goRefs (i : Nat) (v : RatVec P.n) (rhs : Rat) (asms : List Nat) :
      Option (RatVec P.n × Rat × List Nat) :=
    if hi : i < refs.size then
      let rf := refs[i]
      if rf.mult == 0 then
        goRefs (i + 1) v rhs asms
      else if hid : rf.id < entries.size then
        let entry := entries[rf.id]
        if legalMult sense entry.con.sense rf.mult then
          goRefs (i + 1) (v.addScaled entry.con.coeffs rf.mult)
            (rhs + rf.mult * entry.con.rhs) (mergeAsms asms entry.assumptions)
        else none
      else none
    else some (v, rhs, asms)
  termination_by refs.size - i
  goBounds (i : Nat) (v : RatVec P.n) (rhs : Rat) : Option (RatVec P.n × Rat) :=
    if hi : i < bounds.size then
      match stepLinBound P sense v rhs bounds[i] with
      | some (v', rhs') => goBounds (i + 1) v' rhs'
      | none => none
    else some (v, rhs)
  termination_by bounds.size - i

private theorem goRefs_sound (P : VIPRInstance) (entries : Array (DerivedEntry P.n))
    (ht : ValidTable P entries) (sense : RowSense) (refs : Array LinRef) :
    ∀ k (v0 v1 : RatVec P.n) (r0 r1 : Rat) (a0 a1 : List Nat),
      (∀ a ∈ a0, a < entries.size) →
      (∀ x, MILPFeasible P x → asmsSat entries a0 x → linSatSense v0 sense r0 x) →
      evalLinComb.goRefs P entries sense refs k v0 r0 a0 = some (v1, r1, a1) →
      (∀ a ∈ a1, a < entries.size) ∧
      (∀ x, MILPFeasible P x → asmsSat entries a1 x → linSatSense v1 sense r1 x) := by
  intro k v0 v1 r0 r1 a0 a1 ha0 hv0 hgo
  unfold evalLinComb.goRefs at hgo
  dsimp only at hgo
  split at hgo
  · next hi =>
    by_cases hm0 : refs[k].mult = 0
    · have hm0b : (refs[k].mult == 0) = true := beq_iff_eq.mpr hm0
      simp only [hm0b, ↓reduceIte] at hgo
      exact goRefs_sound P entries ht sense refs (k + 1) v0 v1 r0 r1 a0 a1 ha0 hv0 hgo
    · have hm0b : (refs[k].mult == 0) = false := beq_false_of_ne hm0
      simp only [hm0b, Bool.false_eq_true, ↓reduceIte] at hgo
      by_cases hid : refs[k].id < entries.size
      · simp only [hid, ↓reduceDIte] at hgo
        by_cases hleg : legalMult sense entries[refs[k].id].con.sense refs[k].mult = true
        · simp only [hleg, ↓reduceIte] at hgo
          have hEnt := ht refs[k].id hid
          have ha' : ∀ a ∈ mergeAsms a0 entries[refs[k].id].assumptions, a < entries.size := by
            intro a ha
            rcases mem_mergeAsms.mp ha with ha0' | he'
            · exact ha0 a ha0'
            · exact hEnt.1 a he'
          have hv' : ∀ x, MILPFeasible P x →
              asmsSat entries (mergeAsms a0 entries[refs[k].id].assumptions) x →
              linSatSense (v0.addScaled entries[refs[k].id].con.coeffs refs[k].mult)
                sense (r0 + refs[k].mult * entries[refs[k].id].con.rhs) x := by
            intro x hx hasm
            have ha0_sat : asmsSat entries a0 x :=
              asmsSat_mono (fun a ha => mem_mergeAsms.mpr (Or.inl ha)) hasm
            have he_sat : asmsSat entries entries[refs[k].id].assumptions x :=
              asmsSat_mono (fun a ha => mem_mergeAsms.mpr (Or.inr ha)) hasm
            exact linSatSense_step (hv0 x hx ha0_sat) (hEnt.2 x hx he_sat) (Or.inr hleg)
          exact goRefs_sound P entries ht sense refs (k + 1) _ v1 _ r1 _ a1 ha' hv' hgo
        · simp only [hleg] at hgo
          contradiction
      · simp only [hid, ↓reduceDIte] at hgo
        contradiction
  · injection hgo with hEq
    have hv : v0 = v1 := congrArg Prod.fst hEq
    have hr : r0 = r1 := congrArg (fun p => p.2.1) hEq
    have ha : a0 = a1 := congrArg (fun p => p.2.2) hEq
    subst hv; subst hr; subst ha
    exact ⟨ha0, hv0⟩
termination_by k _ _ _ _ _ _ => refs.size - k

private theorem linSatSense_step_ub {n : Nat} {v : RatVec n} {targetSense : RowSense}
    {rhs m u : Rat} (j : Fin n) {x : Fin n → Rat}
    (hv : linSatSense v targetSense rhs x)
    (hxj : x j ≤ u)
    (hleg : legalMult targetSense .le m = true) :
    linSatSense (v.addScaled (RatVec.unit n j) m) targetSense (rhs + m * u) x := by
  have hcon : ({ coeffs := RatVec.unit n j, sense := RowSense.le, rhs := u } : LinCon n).sat x := by
    unfold LinCon.sat
    rw [RatVec.dot_unit]
    exact hxj
  exact @linSatSense_step n v { coeffs := RatVec.unit n j, sense := RowSense.le, rhs := u }
    targetSense rhs m x hv hcon (Or.inr hleg)

private theorem linSatSense_step_lb {n : Nat} {v : RatVec n} {targetSense : RowSense}
    {rhs m l : Rat} (j : Fin n) {x : Fin n → Rat}
    (hv : linSatSense v targetSense rhs x)
    (hxj : l ≤ x j)
    (hleg : legalMult targetSense .ge m = true) :
    linSatSense (v.addScaled (RatVec.unit n j) m) targetSense (rhs + m * l) x := by
  have hcon : ({ coeffs := RatVec.unit n j, sense := RowSense.ge, rhs := l } : LinCon n).sat x := by
    unfold LinCon.sat
    rw [RatVec.dot_unit]
    exact hxj
  exact @linSatSense_step n v { coeffs := RatVec.unit n j, sense := RowSense.ge, rhs := l }
    targetSense rhs m x hv hcon (Or.inr hleg)

private theorem stepLinBound_sound {P : VIPRInstance} {entries : Array (DerivedEntry P.n)}
    {sense : RowSense} {asms : List Nat} {v0 v1 : RatVec P.n} {r0 r1 : Rat}
    {br : VarBoundRef}
    (hv0 : ∀ x, MILPFeasible P x → asmsSat entries asms x → linSatSense v0 sense r0 x)
    (h : stepLinBound P sense v0 r0 br = some (v1, r1)) :
    ∀ x, MILPFeasible P x → asmsSat entries asms x → linSatSense v1 sense r1 x := by
  unfold stepLinBound at h
  split at h
  · injection h with hp
    have hvEq : v0 = v1 := congrArg Prod.fst hp
    have hrEq : r0 = r1 := congrArg Prod.snd hp
    subst hvEq; subst hrEq
    exact hv0
  · split at h
    · next hcol =>
      split at h
      · cases hub : P.ub ⟨br.col, hcol⟩ with
        | none => simp [hub] at h
        | some u =>
          simp only [hub] at h
          split at h
          · next hleg =>
            injection h with hp
            have hvEq : v0.addScaled (RatVec.unit P.n ⟨br.col, hcol⟩) br.mult = v1 :=
              congrArg Prod.fst hp
            have hrEq : r0 + br.mult * u = r1 := congrArg Prod.snd hp
            subst hvEq; subst hrEq
            intro x hx hasm
            exact linSatSense_step_ub ⟨br.col, hcol⟩ (hv0 x hx hasm)
              ((hx.1 ⟨br.col, hcol⟩).2 u hub) hleg
          · contradiction
      · cases hlb : P.lb ⟨br.col, hcol⟩ with
        | none => simp [hlb] at h
        | some l =>
          simp only [hlb] at h
          split at h
          · next hleg =>
            injection h with hp
            have hvEq : v0.addScaled (RatVec.unit P.n ⟨br.col, hcol⟩) br.mult = v1 :=
              congrArg Prod.fst hp
            have hrEq : r0 + br.mult * l = r1 := congrArg Prod.snd hp
            subst hvEq; subst hrEq
            intro x hx hasm
            exact linSatSense_step_lb ⟨br.col, hcol⟩ (hv0 x hx hasm)
              ((hx.1 ⟨br.col, hcol⟩).1 l hlb) hleg
          · contradiction
    · contradiction

private theorem goBounds_sound (P : VIPRInstance) (entries : Array (DerivedEntry P.n))
    (sense : RowSense) (bounds : Array VarBoundRef) (asms : List Nat) :
    ∀ k (v0 v1 : RatVec P.n) (r0 r1 : Rat),
      (∀ x, MILPFeasible P x → asmsSat entries asms x → linSatSense v0 sense r0 x) →
      evalLinComb.goBounds P sense bounds k v0 r0 = some (v1, r1) →
      (∀ x, MILPFeasible P x → asmsSat entries asms x → linSatSense v1 sense r1 x) := by
  intro k v0 v1 r0 r1 hv0 hgo
  unfold evalLinComb.goBounds at hgo
  split at hgo
  · next hi =>
    cases hs : stepLinBound P sense v0 r0 bounds[k] with
    | none => simp [hs] at hgo
    | some p =>
      rcases p with ⟨v', r'⟩
      simp only [hs] at hgo
      exact goBounds_sound P entries sense bounds asms (k + 1) v' v1 r' r1
        (stepLinBound_sound hv0 hs) hgo
  · injection hgo with hEq
    have hv : v0 = v1 := congrArg Prod.fst hEq
    have hr : r0 = r1 := congrArg Prod.snd hEq
    subst hv; subst hr
    exact hv0
termination_by k _ _ _ _ => bounds.size - k

theorem evalLinComb_sound {P : VIPRInstance} {entries : Array (DerivedEntry P.n)}
    (ht : ValidTable P entries) {sense : RowSense} {refs : Array LinRef}
    {bounds : Array VarBoundRef} {e : DerivedEntry P.n}
    (h : evalLinComb P entries sense refs bounds = some e) :
    entryValid P entries e := by
  unfold evalLinComb at h
  cases href : evalLinComb.goRefs P entries sense refs 0 (RatVec.zero P.n) 0 [] with
  | none => simp [href] at h
  | some p1 =>
    rcases p1 with ⟨v1, r1, asms1⟩
    simp only [href] at h
    cases hbnd : evalLinComb.goBounds P sense bounds 0 v1 r1 with
    | none => simp [hbnd] at h
    | some p2 =>
      rcases p2 with ⟨v2, r2⟩
      simp only [hbnd] at h
      injection h with heq; subst heq
      have h1 := goRefs_sound P entries ht sense refs 0 (RatVec.zero P.n) v1 0 r1 [] asms1
        (fun _ ha => by simp at ha) (fun x _ _ => linSatSense_zero sense x) href
      have h2 := goBounds_sound P entries sense bounds asms1 0 v1 v2 r1 r2 h1.2 hbnd
      refine ⟨h1.1, fun x hx hasms => ?_⟩
      have hsat := h2 x hx hasms
      unfold LinCon.sat
      unfold linSatSense at hsat
      cases sense <;> exact hsat

private theorem evalLinComb_sense {P : VIPRInstance} {entries : Array (DerivedEntry P.n)}
    {sense : RowSense} {refs : Array LinRef} {bounds : Array VarBoundRef} {e : DerivedEntry P.n}
    (h : evalLinComb P entries sense refs bounds = some e) :
    e.con.sense = sense := by
  unfold evalLinComb at h
  cases href : evalLinComb.goRefs P entries sense refs 0 (RatVec.zero P.n) 0 [] with
  | none => simp [href] at h
  | some p1 =>
    rcases p1 with ⟨v1, r1, asms1⟩
    simp only [href] at h
    cases hbnd : evalLinComb.goBounds P sense bounds 0 v1 r1 with
    | none => simp [hbnd] at h
    | some p2 =>
      rcases p2 with ⟨v2, r2⟩
      simp only [hbnd] at h
      injection h with heq; subst heq; rfl

/-- Checks whether every continuous coordinate is 0 and every integer coordinate has `den = 1`. -/
def isIntegralLinVec (P : VIPRInstance) (v : RatVec P.n) : Bool :=
  loop 0
where
  loop (i : Nat) : Bool :=
    if hi : i < P.n then
      let j : Fin P.n := ⟨i, hi⟩
      let c := v.get j
      let ok := if P.isInt j then c.den == 1 else c == 0
      if ok then loop (i + 1) else false
    else true
  termination_by P.n - i

private theorem isIntegralLinVec_loop_sound (P : VIPRInstance) (v : RatVec P.n) :
    ∀ k (hk : k ≤ P.n),
      (∀ i (hi : i < k),
        let j : Fin P.n := ⟨i, Nat.lt_of_lt_of_le hi hk⟩
        (P.isInt j = true → (v.get j).den = 1) ∧
        (P.isInt j = false → v.get j = 0)) →
      isIntegralLinVec.loop P v k = true →
      ∀ j : Fin P.n,
        (P.isInt j = true → (v.get j).den = 1) ∧
        (P.isInt j = false → v.get j = 0) := by
  intro k hk hprev hloop j
  unfold isIntegralLinVec.loop at hloop
  dsimp only at hloop
  split at hloop
  · next hlt =>
    by_cases hok : (if P.isInt ⟨k, hlt⟩ then (v.get ⟨k, hlt⟩).den == 1 else v.get ⟨k, hlt⟩ == 0) = true
    · simp only [hok, ↓reduceIte] at hloop
      have hcur :
          (P.isInt ⟨k, hlt⟩ = true → (v.get ⟨k, hlt⟩).den = 1) ∧
          (P.isInt ⟨k, hlt⟩ = false → v.get ⟨k, hlt⟩ = 0) := by
        constructor
        · intro hi
          simp only [hi, ↓reduceIte] at hok
          exact beq_iff_eq.mp hok
        · intro hi
          simp only [hi, Bool.false_eq_true, ↓reduceIte] at hok
          exact beq_iff_eq.mp hok
      have hnext : ∀ i (hi : i < k + 1),
          let ji : Fin P.n := ⟨i, Nat.lt_of_lt_of_le hi hlt⟩
          (P.isInt ji = true → (v.get ji).den = 1) ∧
          (P.isInt ji = false → v.get ji = 0) := by
        intro i hi
        by_cases hik : i = k
        · subst hik; exact hcur
        · exact hprev i (by omega)
      exact isIntegralLinVec_loop_sound P v (k + 1) hlt hnext hloop j
    · simp only [hok] at hloop; contradiction
  · next hge =>
    have hkn : k = P.n := by omega
    subst hkn
    exact hprev j.1 j.2
termination_by k _ _ _ => P.n - k

private theorem isIntegralLinVec_dot_int {P : VIPRInstance} {v : RatVec P.n}
    (hv : isIntegralLinVec P v = true) {x : Fin P.n → Rat} (hx : MILPFeasible P x) :
    isRatInt (v.dot x) := by
  have hcoord := isIntegralLinVec_loop_sound P v 0 (Nat.zero_le P.n) (fun _ hi => by omega) hv
  suffices h : ∀ m (hm : m ≤ P.n),
      isRatInt (finSum m (fun i => v.get ⟨i.1, Nat.lt_of_lt_of_le i.2 hm⟩ *
        x ⟨i.1, Nat.lt_of_lt_of_le i.2 hm⟩)) by
    exact h P.n (Nat.le_refl P.n)
  intro m hm
  induction m with
  | zero => exact ⟨0, rfl⟩
  | succ m ih =>
    simp only [finSum_succ, Fin.val_castSucc, Fin.val_last]
    obtain ⟨z1, hz1⟩ := ih (Nat.le_of_lt hm)
    let j : Fin P.n := ⟨m, hm⟩
    have hj_prod : isRatInt (v.get j * x j) := by
      cases hint : P.isInt j with
      | true =>
        obtain ⟨zv, hzv⟩ := (isRatInt_iff_den_eq_one (v.get j)).mpr ((hcoord j).1 hint)
        obtain ⟨zx, hzx⟩ := hx.2.2 j hint
        refine ⟨zv * zx, ?_⟩
        rw [hzv, hzx, Rat.intCast_mul]
      | false =>
        have h0 : v.get j = 0 := (hcoord j).2 hint
        refine ⟨0, ?_⟩
        rw [h0, Rat.zero_mul]; rfl
    obtain ⟨z2, hz2⟩ := hj_prod
    refine ⟨z1 + z2, ?_⟩
    change (finSum m fun i => v.get ⟨i.1, Nat.lt_of_lt_of_le i.2 (Nat.le_of_lt hm)⟩ *
        x ⟨i.1, Nat.lt_of_lt_of_le i.2 (Nat.le_of_lt hm)⟩) + v.get j * x j = ↑(z1 + z2)
    rw [hz1, hz2, Rat.intCast_add]

/-- Merges two branch leaf conclusions once assumptions on `col ≤ k` and `col ≥ k + 1` are verified. -/
def mergeBranchCon {n : Nat} (cL cR : LinCon n) : Option (LinCon n) :=
  if isUnsatCon cL && isUnsatCon cR then
    some { coeffs := RatVec.zero n, sense := .le, rhs := -1 }
  else if isUnsatCon cL then
    some cR
  else if isUnsatCon cR then
    some cL
  else if cL.coeffs.beq cR.coeffs && cL.sense == cR.sense then
    match cL.sense with
    | .le => some { coeffs := cL.coeffs, sense := .le, rhs := max cL.rhs cR.rhs }
    | .ge => some { coeffs := cL.coeffs, sense := .ge, rhs := min cL.rhs cR.rhs }
    | .eq => if cL.rhs == cR.rhs then some cL else none
  else
    none

private theorem mergeBranchCon_sound {n : Nat} {cL cR cOut : LinCon n} {x : Fin n → Rat}
    (hm : mergeBranchCon cL cR = some cOut)
    (hbranch : cL.sat x ∨ cR.sat x) : cOut.sat x := by
  unfold mergeBranchCon at hm
  split at hm
  · next hboth =>
    injection hm with heq; subst heq
    have hboth' : isUnsatCon cL = true ∧ isUnsatCon cR = true := Bool.and_eq_true _ _ |>.mp hboth
    rcases hbranch with hl | hr
    · exact absurd hl (isUnsatCon_sound hboth'.1 x)
    · exact absurd hr (isUnsatCon_sound hboth'.2 x)
  · split at hm
    · next hlU =>
      injection hm with heq; subst heq
      rcases hbranch with hl | hr
      · exact absurd hl (isUnsatCon_sound hlU x)
      · exact hr
    · split at hm
      · next hrU =>
        injection hm with heq; subst heq
        rcases hbranch with hl | hr
        · exact hl
        · exact absurd hr (isUnsatCon_sound hrU x)
      · split at hm
        · next hmatch =>
          simp only [Bool.and_eq_true, beq_iff_eq] at hmatch
          have hcEq : cL.coeffs = cR.coeffs := RatVec.eq_of_beq hmatch.1
          have hsEq : cL.sense = cR.sense := hmatch.2
          cases hs : cL.sense with
          | le =>
            rw [hs] at hm
            dsimp only at hm
            injection hm with heq; subst heq
            unfold LinCon.sat at hbranch ⊢
            rw [← hsEq, ← hcEq, hs] at hbranch
            dsimp only at hbranch ⊢
            rcases hbranch with hl | hr
            · exact Rat.le_trans hl (by
                rw [Rat.max_def]
                split
                · assumption
                · exact Rat.le_refl)
            · exact Rat.le_trans hr (by
                rw [Rat.max_def]
                split
                · exact Rat.le_refl
                · exact Rat.le_of_lt (Rat.not_le.mp ‹_›))
          | ge =>
            rw [hs] at hm
            dsimp only at hm
            injection hm with heq; subst heq
            unfold LinCon.sat at hbranch ⊢
            rw [← hsEq, ← hcEq, hs] at hbranch
            dsimp only at hbranch ⊢
            rcases hbranch with hl | hr
            · exact Rat.le_trans (rat_min_le_left cL.rhs cR.rhs) hl
            · exact Rat.le_trans (rat_min_le_right cL.rhs cR.rhs) hr
          | eq =>
            rw [hs] at hm
            dsimp only at hm
            split at hm
            · next hrEq =>
              injection hm with heq; subst heq
              have hrEq' : cL.rhs = cR.rhs := beq_iff_eq.mp hrEq
              unfold LinCon.sat at hbranch ⊢
              rw [← hsEq, ← hcEq, ← hrEq'] at hbranch
              rw [hs] at hbranch ⊢
              dsimp only at hbranch ⊢
              rcases hbranch with hl | hr <;> assumption
            · contradiction
        · contradiction

private def findTightestBound (P : VIPRInstance) (entries : Array (DerivedEntry P.n))
    (boundAsms : Array Nat) (j : Fin P.n) (wantUpper : Bool) :
    Option (Rat × List Nat) :=
  let init : Option (Rat × List Nat) :=
    if wantUpper then
      (P.ub j).map (·, [])
    else
      (P.lb j).map (·, [])
  go 0 init
where
  go (i : Nat) (best : Option (Rat × List Nat)) : Option (Rat × List Nat) :=
    if hi : i < boundAsms.size then
      let id := boundAsms[i]
      if hid : id < entries.size then
        let e := entries[id]
        let okSense :=
          if wantUpper then e.con.sense == .le || e.con.sense == .eq
          else e.con.sense == .ge || e.con.sense == .eq
        if okSense && e.con.coeffs.beq (RatVec.unit P.n j) then
          let cand := e.con.rhs
          let best' := match best with
            | none => some (cand, e.assumptions)
            | some (b0, asms0) =>
              if wantUpper then
                if cand < b0 then some (cand, e.assumptions) else some (b0, asms0)
              else
                if b0 < cand then some (cand, e.assumptions) else some (b0, asms0)
          go (i + 1) best'
        else
          go (i + 1) best
      else
        go (i + 1) best
    else
      best
  termination_by boundAsms.size - i

private theorem findTightestBound_go_sound (P : VIPRInstance) (entries : Array (DerivedEntry P.n))
    (ht : ValidTable P entries) (boundAsms : Array Nat) (j : Fin P.n) (wantUpper : Bool) :
    ∀ k (init : Option (Rat × List Nat)) (out : Rat × List Nat),
      (∀ bd asms, init = some (bd, asms) →
        (∀ a ∈ asms, a < entries.size) ∧
        (∀ x, MILPFeasible P x → asmsSat entries asms x →
          if wantUpper then x j ≤ bd else bd ≤ x j)) →
      findTightestBound.go P entries boundAsms j wantUpper k init = some out →
      (∀ a ∈ out.2, a < entries.size) ∧
      (∀ x, MILPFeasible P x → asmsSat entries out.2 x →
        if wantUpper then x j ≤ out.1 else out.1 ≤ x j) := by
  intro k init out hinit hgo
  unfold findTightestBound.go at hgo
  dsimp only at hgo
  split at hgo
  · next hi =>
    by_cases hid : boundAsms[k] < entries.size
    · simp only [hid, ↓reduceDIte] at hgo
      have hEnt := ht boundAsms[k] hid
      by_cases hok :
          ((if wantUpper then entries[boundAsms[k]].con.sense == .le || entries[boundAsms[k]].con.sense == .eq
            else entries[boundAsms[k]].con.sense == .ge || entries[boundAsms[k]].con.sense == .eq) &&
           entries[boundAsms[k]].con.coeffs.beq (RatVec.unit P.n j)) = true
      · simp only [hok, ↓reduceIte] at hgo
        have hok' := Bool.and_eq_true _ _ |>.mp hok
        have hunit : entries[boundAsms[k]].con.coeffs = RatVec.unit P.n j := RatVec.eq_of_beq hok'.2
        have hcur : ∀ x, MILPFeasible P x → asmsSat entries entries[boundAsms[k]].assumptions x →
            if wantUpper then x j ≤ entries[boundAsms[k]].con.rhs else entries[boundAsms[k]].con.rhs ≤ x j := by
          intro x hx hasm
          have hsat := hEnt.2 x hx hasm
          unfold LinCon.sat at hsat
          rw [hunit, RatVec.dot_unit] at hsat
          cases wantUpper
          · simp only [Bool.false_eq_true, ↓reduceIte] at hok' ⊢
            rcases Bool.or_eq_true _ _ |>.mp hok'.1 with hs | hs
            · rw [beq_iff_eq.mp hs] at hsat; exact hsat
            · rw [beq_iff_eq.mp hs] at hsat; exact hsat ▸ Rat.le_refl
          · simp only [↓reduceIte] at hok' ⊢
            rcases Bool.or_eq_true _ _ |>.mp hok'.1 with hs | hs
            · rw [beq_iff_eq.mp hs] at hsat; exact hsat
            · rw [beq_iff_eq.mp hs] at hsat; exact hsat ▸ Rat.le_refl
        have hnext : ∀ bd asms,
            (match init with
            | none => some (entries[boundAsms[k]].con.rhs, entries[boundAsms[k]].assumptions)
            | some (b0, asms0) =>
              if wantUpper then
                if entries[boundAsms[k]].con.rhs < b0 then
                  some (entries[boundAsms[k]].con.rhs, entries[boundAsms[k]].assumptions)
                else some (b0, asms0)
              else
                if b0 < entries[boundAsms[k]].con.rhs then
                  some (entries[boundAsms[k]].con.rhs, entries[boundAsms[k]].assumptions)
                else some (b0, asms0)) =
            some (bd, asms) →
            (∀ a ∈ asms, a < entries.size) ∧
            (∀ x, MILPFeasible P x → asmsSat entries asms x →
              if wantUpper then x j ≤ bd else bd ≤ x j) := by
          intro bd asms hmatch
          cases init with
          | none =>
            injection hmatch with hpair
            have hbEq : entries[boundAsms[k]].con.rhs = bd := congrArg Prod.fst hpair
            have haEq : entries[boundAsms[k]].assumptions = asms := congrArg Prod.snd hpair
            subst hbEq; subst haEq
            exact ⟨hEnt.1, hcur⟩
          | some p0 =>
            rcases p0 with ⟨b0, asms0⟩
            dsimp only at hmatch
            cases wantUpper
            · simp only [Bool.false_eq_true, ↓reduceIte] at hmatch
              split at hmatch
              · injection hmatch with hp
                have hbEq : entries[boundAsms[k]].con.rhs = bd := congrArg Prod.fst hp
                have haEq : entries[boundAsms[k]].assumptions = asms := congrArg Prod.snd hp
                subst hbEq; subst haEq
                exact ⟨hEnt.1, hcur⟩
              · injection hmatch with hp
                have hbEq : b0 = bd := congrArg Prod.fst hp
                have haEq : asms0 = asms := congrArg Prod.snd hp
                subst hbEq; subst haEq
                exact hinit b0 asms0 rfl
            · simp only [↓reduceIte] at hmatch
              split at hmatch
              · injection hmatch with hp
                have hbEq : entries[boundAsms[k]].con.rhs = bd := congrArg Prod.fst hp
                have haEq : entries[boundAsms[k]].assumptions = asms := congrArg Prod.snd hp
                subst hbEq; subst haEq
                exact ⟨hEnt.1, hcur⟩
              · injection hmatch with hp
                have hbEq : b0 = bd := congrArg Prod.fst hp
                have haEq : asms0 = asms := congrArg Prod.snd hp
                subst hbEq; subst haEq
                exact hinit b0 asms0 rfl
        exact findTightestBound_go_sound P entries ht boundAsms j wantUpper (k + 1) _ out hnext hgo
      · simp only [hok] at hgo
        exact findTightestBound_go_sound P entries ht boundAsms j wantUpper (k + 1) init out hinit hgo
    · simp only [hid, ↓reduceDIte] at hgo
      exact findTightestBound_go_sound P entries ht boundAsms j wantUpper (k + 1) init out hinit hgo
  · exact hinit out.1 out.2 (by rcases out with ⟨b, a⟩; exact hgo)
termination_by k _ _ _ _ => boundAsms.size - k

private theorem findTightestBound_sound (P : VIPRInstance) (entries : Array (DerivedEntry P.n))
    (ht : ValidTable P entries) (boundAsms : Array Nat) (j : Fin P.n) (wantUpper : Bool)
    (out : Rat × List Nat)
    (h : findTightestBound P entries boundAsms j wantUpper = some out) :
    (∀ a ∈ out.2, a < entries.size) ∧
    (∀ x, MILPFeasible P x → asmsSat entries out.2 x →
      if wantUpper then x j ≤ out.1 else out.1 ≤ x j) := by
  unfold findTightestBound at h
  apply findTightestBound_go_sound P entries ht boundAsms j wantUpper 0 _ out _ h
  intro bd asms hinit
  cases wantUpper
  · simp only [Bool.false_eq_true, ↓reduceIte] at hinit ⊢
    cases hlb : P.lb j with
    | none => simp [hlb] at hinit
    | some l =>
      simp only [hlb, Option.map_some] at hinit
      injection hinit with hp
      have hlEq : l = bd := congrArg Prod.fst hp
      have haEq : ([] : List Nat) = asms := congrArg Prod.snd hp
      subst hlEq; subst haEq
      exact ⟨fun _ ha => by simp at ha, fun x hx _ => (hx.1 j).1 l hlb⟩
  · simp only [↓reduceIte] at hinit ⊢
    cases hub : P.ub j with
    | none => simp [hub] at hinit
    | some u =>
      simp only [hub, Option.map_some] at hinit
      injection hinit with hp
      have huEq : u = bd := congrArg Prod.fst hp
      have haEq : ([] : List Nat) = asms := congrArg Prod.snd hp
      subst huEq; subst haEq
      exact ⟨fun _ ha => by simp at ha, fun x hx _ => (hx.1 j).2 u hub⟩

/-- Safe Neumaier-Shcherbina dual bounding over `refs` and active bound assumptions. -/
def evalSafeDual (P : VIPRInstance) (entries : Array (DerivedEntry P.n))
    (refs : Array LinRef) (boundAsms : Array Nat) : Option (DerivedEntry P.n) :=
  match evalLinComb P entries .ge refs #[] with
  | none => none
  | some rowEntry =>
    match goCols 0 rowEntry.con.coeffs rowEntry.con.rhs rowEntry.assumptions with
    | none => none
    | some (totalRhs, finalAsms) =>
      some { con := { coeffs := RatVec.ofFn P.obj, sense := .ge, rhs := totalRhs },
             assumptions := finalAsms }
where
  goCols (i : Nat) (rowCoeffs : RatVec P.n) (accRhs : Rat) (accAsms : List Nat) :
      Option (Rat × List Nat) :=
    if hi : i < P.n then
      let j : Fin P.n := ⟨i, hi⟩
      let rj := P.obj j - rowCoeffs.get j
      if rj == 0 then
        goCols (i + 1) rowCoeffs accRhs accAsms
      else if 0 < rj then
        match findTightestBound P entries boundAsms j false with
        | some (l, asmsL) =>
          goCols (i + 1) rowCoeffs (accRhs + rj * l) (mergeAsms accAsms asmsL)
        | none => none
      else
        match findTightestBound P entries boundAsms j true with
        | some (u, asmsU) =>
          goCols (i + 1) rowCoeffs (accRhs + rj * u) (mergeAsms accAsms asmsU)
        | none => none
    else
      some (accRhs, accAsms)
  termination_by P.n - i

private theorem goCols_sound (P : VIPRInstance) (entries : Array (DerivedEntry P.n))
    (ht : ValidTable P entries) (boundAsms : Array Nat) (rowCoeffs : RatVec P.n) :
    ∀ k (hk : k ≤ P.n) (r0 : Rat) (a0 : List Nat) (rOut : Rat) (aOut : List Nat),
      (∀ a ∈ a0, a < entries.size) →
      (∀ x, MILPFeasible P x → asmsSat entries a0 x →
        r0 ≤ rowCoeffs.dot x + finSum k (fun i =>
          let j : Fin P.n := ⟨i.1, Nat.lt_of_lt_of_le i.2 hk⟩
          (P.obj j - rowCoeffs.get j) * x j)) →
      evalSafeDual.goCols P entries boundAsms k rowCoeffs r0 a0 = some (rOut, aOut) →
      (∀ a ∈ aOut, a < entries.size) ∧
      (∀ x, MILPFeasible P x → asmsSat entries aOut x →
        rOut ≤ finSum P.n (fun j => P.obj j * x j)) := by
  intro k hk r0 a0 rOut aOut ha0 hr0 hgo
  unfold evalSafeDual.goCols at hgo
  dsimp only at hgo
  split at hgo
  · next hlt =>
    let j : Fin P.n := ⟨k, hlt⟩
    let rj := P.obj j - rowCoeffs.get j
    have hsplit : ∀ x : Fin P.n → Rat,
        finSum (k + 1) (fun i =>
          let ji : Fin P.n := ⟨i.1, Nat.lt_of_lt_of_le i.2 hlt⟩
          (P.obj ji - rowCoeffs.get ji) * x ji) =
        finSum k (fun i =>
          let ji : Fin P.n := ⟨i.1, Nat.lt_of_lt_of_le i.2 hk⟩
          (P.obj ji - rowCoeffs.get ji) * x ji) + rj * x j := by
      intro x; simp [finSum_succ, j, rj]
    split at hgo
    · next h0b =>
      have h0 : rj = 0 := beq_iff_eq.mp h0b
      have hr' : ∀ x, MILPFeasible P x → asmsSat entries a0 x →
          r0 ≤ rowCoeffs.dot x + finSum (k + 1) (fun i =>
            let ji : Fin P.n := ⟨i.1, Nat.lt_of_lt_of_le i.2 hlt⟩
            (P.obj ji - rowCoeffs.get ji) * x ji) := by
        intro x hx hasm
        rw [hsplit x, h0, Rat.zero_mul, Rat.add_zero]
        exact hr0 x hx hasm
      exact goCols_sound P entries ht boundAsms rowCoeffs (k + 1) hlt r0 a0 rOut aOut ha0 hr' hgo
    · next h0b =>
      have h0 : rj ≠ 0 := fun heq => absurd (beq_iff_eq.mpr heq) h0b
      split at hgo
      · next hpos =>
        cases hfb : findTightestBound P entries boundAsms ⟨k, hlt⟩ false with
        | none => simp [hfb] at hgo
        | some pL =>
          rcases pL with ⟨l, asmsL⟩
          simp only [hfb] at hgo
          have hL := findTightestBound_sound P entries ht boundAsms ⟨k, hlt⟩ false (l, asmsL) hfb
          have ha' : ∀ a ∈ mergeAsms a0 asmsL, a < entries.size := by
            intro a ha
            rcases mem_mergeAsms.mp ha with ha0' | hal'
            · exact ha0 a ha0'
            · exact hL.1 a hal'
          have hr' : ∀ x, MILPFeasible P x → asmsSat entries (mergeAsms a0 asmsL) x →
              r0 + rj * l ≤ rowCoeffs.dot x + finSum (k + 1) (fun i =>
                let ji : Fin P.n := ⟨i.1, Nat.lt_of_lt_of_le i.2 hlt⟩
                (P.obj ji - rowCoeffs.get ji) * x ji) := by
            intro x hx hasm
            have h1 := hr0 x hx (asmsSat_mono (fun a ha => mem_mergeAsms.mpr (Or.inl ha)) hasm)
            have h2 := hL.2 x hx (asmsSat_mono (fun a ha => mem_mergeAsms.mpr (Or.inr ha)) hasm)
            simp only [Bool.false_eq_true, ↓reduceIte] at h2
            have hmul : rj * l ≤ rj * x j := Rat.mul_le_mul_of_nonneg_left h2 (Rat.le_of_lt hpos)
            rw [hsplit x, ← Rat.add_assoc]
            exact Rat.le_trans (Rat.add_le_add_right.mpr h1) (Rat.add_le_add_left.mpr hmul)
          exact goCols_sound P entries ht boundAsms rowCoeffs (k + 1) hlt (r0 + rj * l)
            (mergeAsms a0 asmsL) rOut aOut ha' hr' hgo
      · next hpos =>
        have hneg : rj < 0 := Rat.lt_of_le_of_ne (Rat.not_lt.mp hpos) h0
        have hneg0 : 0 ≤ -rj := by
          rw [← Rat.neg_zero]
          exact Rat.neg_le_neg (Rat.le_of_lt hneg)
        cases hfb : findTightestBound P entries boundAsms ⟨k, hlt⟩ true with
        | none => simp [hfb] at hgo
        | some pU =>
          rcases pU with ⟨u, asmsU⟩
          simp only [hfb] at hgo
          have hU := findTightestBound_sound P entries ht boundAsms ⟨k, hlt⟩ true (u, asmsU) hfb
          have ha' : ∀ a ∈ mergeAsms a0 asmsU, a < entries.size := by
            intro a ha
            rcases mem_mergeAsms.mp ha with ha0' | hau'
            · exact ha0 a ha0'
            · exact hU.1 a hau'
          have hr' : ∀ x, MILPFeasible P x → asmsSat entries (mergeAsms a0 asmsU) x →
              r0 + rj * u ≤ rowCoeffs.dot x + finSum (k + 1) (fun i =>
                let ji : Fin P.n := ⟨i.1, Nat.lt_of_lt_of_le i.2 hlt⟩
                (P.obj ji - rowCoeffs.get ji) * x ji) := by
            intro x hx hasm
            have h1 := hr0 x hx (asmsSat_mono (fun a ha => mem_mergeAsms.mpr (Or.inl ha)) hasm)
            have h2 := hU.2 x hx (asmsSat_mono (fun a ha => mem_mergeAsms.mpr (Or.inr ha)) hasm)
            simp only [↓reduceIte] at h2
            have hmul0 := Rat.mul_le_mul_of_nonneg_left h2 hneg0
            rw [Rat.neg_mul, Rat.neg_mul, Rat.neg_le_neg_iff] at hmul0
            rw [hsplit x, ← Rat.add_assoc]
            exact Rat.le_trans (Rat.add_le_add_right.mpr h1) (Rat.add_le_add_left.mpr hmul0)
          exact goCols_sound P entries ht boundAsms rowCoeffs (k + 1) hlt (r0 + rj * u)
            (mergeAsms a0 asmsU) rOut aOut ha' hr' hgo
  · next hge =>
    have hkn : k = P.n := by omega
    subst hkn
    injection hgo with hEq
    have hrEq : r0 = rOut := congrArg Prod.fst hEq
    have haEq : a0 = aOut := congrArg Prod.snd hEq
    subst hrEq; subst haEq
    refine ⟨ha0, fun x hx hasm => ?_⟩
    have h1 := hr0 x hx hasm
    have hsum :
        rowCoeffs.dot x + finSum P.n (fun j => (P.obj j - rowCoeffs.get j) * x j) =
        finSum P.n (fun j => P.obj j * x j) := by
      unfold RatVec.dot
      rw [← finSum_add]
      apply finSum_congr
      intro j
      rw [← Rat.add_mul, Rat.add_comm, Rat.sub_add_cancel]
    rwa [← hsum]
termination_by k _ _ _ _ _ _ => P.n - k

private def getBoundLB (P : VIPRInstance) (entries : Array (DerivedEntry P.n))
    (j : Fin P.n) : Option Nat → Option (Rat × List Nat)
  | none => (P.lb j).map (·, [])
  | some id =>
    if hid : id < entries.size then
      if (entries[id].con.sense == .ge || entries[id].con.sense == .eq) &&
          entries[id].con.coeffs.beq (RatVec.unit P.n j) then
        some (entries[id].con.rhs, entries[id].assumptions)
      else none
    else none

private def getBoundUB (P : VIPRInstance) (entries : Array (DerivedEntry P.n))
    (j : Fin P.n) : Option Nat → Option (Rat × List Nat)
  | none => (P.ub j).map (·, [])
  | some id =>
    if hid : id < entries.size then
      if (entries[id].con.sense == .le || entries[id].con.sense == .eq) &&
          entries[id].con.coeffs.beq (RatVec.unit P.n j) then
        some (entries[id].con.rhs, entries[id].assumptions)
      else none
    else none

/-! ### Permutation & Orbital Automorphism Verification -/

/-- Builds a candidate inverse array `inv` for a permutation `perm` of `[0 .. n-1]`. -/
def buildPermInv (n : Nat) (perm : Array Nat) : Array Nat :=
  go 0 (Array.replicate n n)
where
  go (i : Nat) (inv : Array Nat) : Array Nat :=
    if hi : i < perm.size then
      let p := perm[i]
      if hp : p < inv.size then
        go (i + 1) (inv.set p i hp)
      else
        go (i + 1) inv
    else inv
  termination_by perm.size - i

private theorem buildPermInv_go_size (n : Nat) (perm : Array Nat) :
    ∀ k (inv : Array Nat), inv.size = n → (buildPermInv.go perm k inv).size = n := by
  intro k inv hsz
  unfold buildPermInv.go
  split
  · next hi =>
    by_cases hp : perm[k] < inv.size
    · simp only [hp, ↓reduceDIte]
      exact buildPermInv_go_size n perm (k + 1) (inv.set perm[k] k hp) (by rw [Array.size_set]; exact hsz)
    · simp only [hp, ↓reduceDIte]
      exact buildPermInv_go_size n perm (k + 1) inv hsz
  · exact hsz
termination_by k _ _ => perm.size - k

theorem buildPermInv_size (n : Nat) (perm : Array Nat) :
    (buildPermInv n perm).size = n := by
  unfold buildPermInv
  exact buildPermInv_go_size n perm 0 (Array.replicate n n) (by simp)

/-- Helper indexing into a sized permutation array. -/
@[inline]
private def permAt {n : Nat} (perm : Array Nat) (hsz : perm.size = n) (j : Fin n) : Nat :=
  perm[j.1]'(by rw [hsz]; exact j.2)

/-- Helper lifting a validated permutation array to `Fin n → Fin n`. -/
@[inline]
private def permFn {n : Nat} (perm : Array Nat) (hsz : perm.size = n)
    (hlt : ∀ j : Fin n, permAt perm hsz j < n) (j : Fin n) : Fin n :=
  ⟨permAt perm hsz j, hlt j⟩

/-- Checks whether `perm` is a bijection on `Fin n` via `inv[perm[i]] = i`. -/
def checkPermBijection (n : Nat) (perm : Array Nat) : Bool :=
  if perm.size == n then
    let inv := buildPermInv n perm
    loop inv 0
  else false
where
  loop (inv : Array Nat) (i : Nat) : Bool :=
    if hi : i < n then
      if hp : i < perm.size then
        let p := perm[i]
        if hpn : p < inv.size then
          if inv[p] == i then loop inv (i + 1) else false
        else false
      else false
    else true
  termination_by n - i

private theorem checkPermBijection_loop_sound (n : Nat) (perm inv : Array Nat)
    (hsz : perm.size = n) (hinvSz : inv.size = n) :
    ∀ k (hk : k ≤ n),
      (∀ i (hi : i < k),
        ∃ hp : permAt perm hsz ⟨i, Nat.lt_of_lt_of_le hi hk⟩ < n,
          inv[permAt perm hsz ⟨i, Nat.lt_of_lt_of_le hi hk⟩]'(by rw [hinvSz]; exact hp) = i) →
      checkPermBijection.loop n perm inv k = true →
      ∀ j : Fin n,
        ∃ hp : permAt perm hsz j < n,
          inv[permAt perm hsz j]'(by rw [hinvSz]; exact hp) = j.1 := by
  intro k hk hprev hloop j
  unfold checkPermBijection.loop at hloop
  dsimp only at hloop
  split at hloop
  · next hlt =>
    have hpk : k < perm.size := by rw [hsz]; exact hlt
    simp only [hpk, ↓reduceDIte] at hloop
    split at hloop
    · next hpn =>
      split at hloop
      · next heq =>
        have hpn' : permAt perm hsz ⟨k, hlt⟩ < n := by
          change perm[k]'hpk < n
          omega
        have hcur : inv[permAt perm hsz ⟨k, hlt⟩]'(by rw [hinvSz]; exact hpn') = k := beq_iff_eq.mp heq
        have hnext : ∀ i (hi : i < k + 1),
            ∃ hp : permAt perm hsz ⟨i, Nat.lt_of_lt_of_le hi hlt⟩ < n,
              inv[permAt perm hsz ⟨i, Nat.lt_of_lt_of_le hi hlt⟩]'(by rw [hinvSz]; exact hp) = i := by
          intro i hi
          by_cases hik : i = k
          · subst hik; exact ⟨hpn', hcur⟩
          · exact hprev i (by omega)
        exact checkPermBijection_loop_sound n perm inv hsz hinvSz (k + 1) hlt hnext hloop j
      · contradiction
    · contradiction
  · next hge =>
    have hkn : k = n := by omega
    subst hkn
    exact hprev j.1 j.2
termination_by k _ _ _ => n - k

theorem checkPermBijection_spec {n : Nat} {perm : Array Nat}
    (h : checkPermBijection n perm = true) :
    ∃ hsz : perm.size = n,
      ∃ hlt : ∀ j : Fin n, permAt perm hsz j < n,
        ∀ i₁ i₂ : Fin n, permFn perm hsz hlt i₁ = permFn perm hsz hlt i₂ → i₁ = i₂ := by
  unfold checkPermBijection at h
  dsimp only at h
  split at h
  · next hszb =>
    have hsz : perm.size = n := beq_iff_eq.mp hszb
    have hinvSz : (buildPermInv n perm).size = n := buildPermInv_size n perm
    have hall := checkPermBijection_loop_sound n perm (buildPermInv n perm) hsz hinvSz
      0 (Nat.zero_le n) (fun _ hi => by omega) h
    have hlt : ∀ j : Fin n, permAt perm hsz j < n := fun j => (hall j).1
    refine ⟨hsz, hlt, ?_⟩
    intro i₁ i₂ heq
    have hv : permAt perm hsz i₁ = permAt perm hsz i₂ := congrArg Fin.val heq
    have h1 := (hall i₁).2
    have h2 := (hall i₂).2
    have h3 : (buildPermInv n perm)[permAt perm hsz i₁]'(by rw [hinvSz]; exact hlt i₁) =
        (buildPermInv n perm)[permAt perm hsz i₂]'(by rw [hinvSz]; exact hlt i₂) := by
      congr 1
    rw [h1, h2] at h3
    exact Fin.ext h3
  · contradiction

/-- Checks whether `perm` is an involution (`perm[perm[i]] = i`) on `Fin n`. -/
def checkPermInvolution (n : Nat) (perm : Array Nat) : Bool :=
  if perm.size == n then
    loop 0
  else false
where
  loop (i : Nat) : Bool :=
    if hi : i < n then
      if hp : i < perm.size then
        let p := perm[i]
        if hpn : p < perm.size then
          if perm[p] == i then loop (i + 1) else false
        else false
      else false
    else true
  termination_by n - i

private theorem checkPermInvolution_loop_sound (n : Nat) (perm : Array Nat)
    (hsz : perm.size = n) :
    ∀ k (hk : k ≤ n),
      (∀ i (hi : i < k),
        ∃ hp : permAt perm hsz ⟨i, Nat.lt_of_lt_of_le hi hk⟩ < n,
          permAt perm hsz ⟨permAt perm hsz ⟨i, Nat.lt_of_lt_of_le hi hk⟩, hp⟩ = i) →
      checkPermInvolution.loop n perm k = true →
      ∀ j : Fin n,
        ∃ hp : permAt perm hsz j < n,
          permAt perm hsz ⟨permAt perm hsz j, hp⟩ = j.1 := by
  intro k hk hprev hloop j
  unfold checkPermInvolution.loop at hloop
  dsimp only at hloop
  split at hloop
  · next hlt =>
    have hpk : k < perm.size := by rw [hsz]; exact hlt
    simp only [hpk, ↓reduceDIte] at hloop
    split at hloop
    · next hpn =>
      split at hloop
      · next heq =>
        have hpn' : permAt perm hsz ⟨k, hlt⟩ < n := by
          change perm[k]'hpk < n
          omega
        have hcur : permAt perm hsz ⟨permAt perm hsz ⟨k, hlt⟩, hpn'⟩ = k := beq_iff_eq.mp heq
        have hnext : ∀ i (hi : i < k + 1),
            ∃ hp : permAt perm hsz ⟨i, Nat.lt_of_lt_of_le hi hlt⟩ < n,
              permAt perm hsz ⟨permAt perm hsz ⟨i, Nat.lt_of_lt_of_le hi hlt⟩, hp⟩ = i := by
          intro i hi
          by_cases hik : i = k
          · subst hik; exact ⟨hpn', hcur⟩
          · exact hprev i (by omega)
        exact checkPermInvolution_loop_sound n perm hsz (k + 1) hlt hnext hloop j
      · contradiction
    · contradiction
  · next hge =>
    have hkn : k = n := by omega
    subst hkn
    exact hprev j.1 j.2
termination_by k _ _ _ => n - k

theorem checkPermInvolution_spec {n : Nat} {perm : Array Nat}
    (h : checkPermInvolution n perm = true) :
    ∃ hsz : perm.size = n,
      ∃ hlt : ∀ j : Fin n, permAt perm hsz j < n,
        (∀ j : Fin n, permFn perm hsz hlt (permFn perm hsz hlt j) = j) ∧
        (∀ i₁ i₂ : Fin n, permFn perm hsz hlt i₁ = permFn perm hsz hlt i₂ → i₁ = i₂) := by
  unfold checkPermInvolution at h
  split at h
  · next hszb =>
    have hsz : perm.size = n := beq_iff_eq.mp hszb
    have hall := checkPermInvolution_loop_sound n perm hsz 0 (Nat.zero_le n)
      (fun _ hi => by omega) h
    have hlt : ∀ j : Fin n, permAt perm hsz j < n := fun j => (hall j).1
    have hinv : ∀ j : Fin n, permFn perm hsz hlt (permFn perm hsz hlt j) = j := by
      intro j
      exact Fin.ext (hall j).2
    refine ⟨hsz, hlt, hinv, ?_⟩
    intro i₁ i₂ heq
    have h1 := hinv i₁
    have h2 := hinv i₂
    rw [← h1, ← h2, heq]
  · contradiction

/-- Checks whether `dst.get j = (±) src.get (perm[j])` for all `j : Fin n`. -/
def checkCoeffsPermMatch {n : Nat} (dst src : RatVec n) (perm : Array Nat) (negate : Bool) : Bool :=
  loop 0
where
  loop (i : Nat) : Bool :=
    if hi : i < n then
      if hp : i < perm.size then
        let p := perm[i]
        if hpn : p < n then
          let dj := dst.get ⟨i, hi⟩
          let sp := src.get ⟨p, hpn⟩
          let ok := if negate then dj == -sp else dj == sp
          if ok then loop (i + 1) else false
        else false
      else false
    else true
  termination_by n - i

private theorem checkCoeffsPermMatch_loop_sound {n : Nat} (dst src : RatVec n)
    (perm : Array Nat) (negate : Bool) (hsz : perm.size = n)
    (hlt : ∀ j : Fin n, permAt perm hsz j < n) :
    ∀ k (hk : k ≤ n),
      (∀ i (hi : i < k),
        let j : Fin n := ⟨i, Nat.lt_of_lt_of_le hi hk⟩
        if negate then dst.get j = -(src.get (permFn perm hsz hlt j))
        else dst.get j = src.get (permFn perm hsz hlt j)) →
      checkCoeffsPermMatch.loop dst src perm negate k = true →
      ∀ j : Fin n,
        if negate then dst.get j = -(src.get (permFn perm hsz hlt j))
        else dst.get j = src.get (permFn perm hsz hlt j) := by
  intro k hk hprev hloop j
  unfold checkCoeffsPermMatch.loop at hloop
  dsimp only at hloop
  split at hloop
  · next hltK =>
    have hpk : k < perm.size := by rw [hsz]; exact hltK
    have hpn : perm[k]'hpk < n := hlt ⟨k, hltK⟩
    simp only [hpk, hpn, ↓reduceDIte] at hloop
    by_cases hok : (if negate then dst.get ⟨k, hltK⟩ == -(src.get ⟨perm[k]'hpk, hpn⟩)
        else dst.get ⟨k, hltK⟩ == src.get ⟨perm[k]'hpk, hpn⟩) = true
    · simp only [hok, ↓reduceIte] at hloop
      have hcur : if negate then dst.get ⟨k, hltK⟩ = -(src.get (permFn perm hsz hlt ⟨k, hltK⟩))
          else dst.get ⟨k, hltK⟩ = src.get (permFn perm hsz hlt ⟨k, hltK⟩) := by
        cases negate <;> simp only [Bool.false_eq_true, ↓reduceIte] at hok ⊢ <;>
          exact beq_iff_eq.mp hok
      have hnext : ∀ i (hi : i < k + 1),
          let ji : Fin n := ⟨i, Nat.lt_of_lt_of_le hi hltK⟩
          if negate then dst.get ji = -(src.get (permFn perm hsz hlt ji))
          else dst.get ji = src.get (permFn perm hsz hlt ji) := by
        intro i hi
        by_cases hik : i = k
        · subst hik; exact hcur
        · exact hprev i (by omega)
      exact checkCoeffsPermMatch_loop_sound dst src perm negate hsz hlt (k + 1) hltK hnext hloop j
    · simp only [hok] at hloop; contradiction
  · next hge =>
    have hkn : k = n := by omega
    subst hkn
    exact hprev j.1 j.2
termination_by k _ _ _ => n - k

private theorem dot_perm_pos {n : Nat} {dst src : RatVec n} {f : Fin n → Fin n}
    (hf : ∀ i₁ i₂, f i₁ = f i₂ → i₁ = i₂)
    (hm : ∀ j : Fin n, dst.get j = src.get (f j)) (x : Fin n → Rat) :
    dst.dot (fun j => x (f j)) = src.dot x := by
  unfold RatVec.dot
  have h1 : finSum n (fun j => dst.get j * x (f j)) =
      finSum n (fun j => src.get (f j) * x (f j)) :=
    finSum_congr (fun j => by rw [hm j])
  rw [h1]
  exact finSum_inj_eq (g := fun k => src.get k * x k) hf

private theorem dot_perm_neg {n : Nat} {dst src : RatVec n} {f : Fin n → Fin n}
    (hf : ∀ i₁ i₂, f i₁ = f i₂ → i₁ = i₂)
    (hm : ∀ j : Fin n, dst.get j = -(src.get (f j))) (x : Fin n → Rat) :
    dst.dot (fun j => x (f j)) = -(src.dot x) := by
  unfold RatVec.dot
  have h1 : finSum n (fun j => dst.get j * x (f j)) =
      finSum n (fun j => -(src.get (f j) * x (f j))) :=
    finSum_congr (fun j => by rw [hm j, Rat.neg_mul])
  rw [h1, finSum_neg]
  exact congrArg Neg.neg (finSum_inj_eq (g := fun k => src.get k * x k) hf)

/-- Checks whether `src.sat x` implies `dst.sat (fun j => x (perm j))` (supporting sign-flipped rows). -/
def checkConPermImplies {n : Nat} (src dst : LinCon n) (perm : Array Nat) : Bool :=
  (checkCoeffsPermMatch dst.coeffs src.coeffs perm false &&
   match dst.sense with
   | .le => (src.sense == .le || src.sense == .eq) && decide (src.rhs ≤ dst.rhs)
   | .ge => (src.sense == .ge || src.sense == .eq) && decide (dst.rhs ≤ src.rhs)
   | .eq => src.sense == .eq && src.rhs == dst.rhs) ||
  (checkCoeffsPermMatch dst.coeffs src.coeffs perm true &&
   match dst.sense with
   | .le => (src.sense == .ge || src.sense == .eq) && decide (-src.rhs ≤ dst.rhs)
   | .ge => (src.sense == .le || src.sense == .eq) && decide (dst.rhs ≤ -src.rhs)
   | .eq => src.sense == .eq && dst.rhs == -src.rhs)

theorem checkConPermImplies_sound {n : Nat} {src dst : LinCon n} {perm : Array Nat}
    (hsz : perm.size = n) (hlt : ∀ j : Fin n, permAt perm hsz j < n)
    (hf : ∀ i₁ i₂ : Fin n, permFn perm hsz hlt i₁ = permFn perm hsz hlt i₂ → i₁ = i₂)
    (hchk : checkConPermImplies src dst perm = true)
    {x : Fin n → Rat} (hsat : src.sat x) :
    dst.sat (fun j => x (permFn perm hsz hlt j)) := by
  unfold checkConPermImplies at hchk
  rcases Bool.or_eq_true _ _ |>.mp hchk with hpos | hneg
  · simp only [Bool.and_eq_true] at hpos
    have hm := checkCoeffsPermMatch_loop_sound dst.coeffs src.coeffs perm false hsz hlt
      0 (Nat.zero_le n) (fun _ hi => by omega) hpos.1
    have hm' : ∀ j : Fin n, dst.coeffs.get j = src.coeffs.get (permFn perm hsz hlt j) := by
      intro j; simpa using hm j
    have hdot : dst.coeffs.dot (fun j => x (permFn perm hsz hlt j)) = src.coeffs.dot x :=
      dot_perm_pos hf hm' x
    unfold LinCon.sat at hsat ⊢
    rw [hdot]
    have hs := hpos.2
    cases hd : dst.sense with
    | le =>
      rw [hd] at hs
      simp only [Bool.and_eq_true] at hs
      have hr : src.rhs ≤ dst.rhs := decide_eq_true_eq.mp hs.2
      rcases Bool.or_eq_true _ _ |>.mp hs.1 with hss | hss
      · rw [beq_iff_eq.mp hss] at hsat; exact Rat.le_trans hsat hr
      · rw [beq_iff_eq.mp hss] at hsat; exact hsat ▸ hr
    | ge =>
      rw [hd] at hs
      simp only [Bool.and_eq_true] at hs
      have hr : dst.rhs ≤ src.rhs := decide_eq_true_eq.mp hs.2
      rcases Bool.or_eq_true _ _ |>.mp hs.1 with hss | hss
      · rw [beq_iff_eq.mp hss] at hsat; exact Rat.le_trans hr hsat
      · rw [beq_iff_eq.mp hss] at hsat; exact hsat ▸ hr
    | eq =>
      rw [hd] at hs
      simp only [Bool.and_eq_true, beq_iff_eq] at hs
      rw [hs.1] at hsat
      rw [hsat, hs.2]
  · simp only [Bool.and_eq_true] at hneg
    have hm := checkCoeffsPermMatch_loop_sound dst.coeffs src.coeffs perm true hsz hlt
      0 (Nat.zero_le n) (fun _ hi => by omega) hneg.1
    have hm' : ∀ j : Fin n, dst.coeffs.get j = -(src.coeffs.get (permFn perm hsz hlt j)) := by
      intro j; simpa using hm j
    have hdot : dst.coeffs.dot (fun j => x (permFn perm hsz hlt j)) = -(src.coeffs.dot x) :=
      dot_perm_neg hf hm' x
    unfold LinCon.sat at hsat ⊢
    rw [hdot]
    have hs := hneg.2
    cases hd : dst.sense with
    | le =>
      rw [hd] at hs
      simp only [Bool.and_eq_true] at hs
      have hr : -src.rhs ≤ dst.rhs := decide_eq_true_eq.mp hs.2
      rcases Bool.or_eq_true _ _ |>.mp hs.1 with hss | hss
      · rw [beq_iff_eq.mp hss] at hsat
        exact Rat.le_trans (Rat.neg_le_neg hsat) hr
      · rw [beq_iff_eq.mp hss] at hsat
        rw [hsat]; exact hr
    | ge =>
      rw [hd] at hs
      simp only [Bool.and_eq_true] at hs
      have hr : dst.rhs ≤ -src.rhs := decide_eq_true_eq.mp hs.2
      rcases Bool.or_eq_true _ _ |>.mp hs.1 with hss | hss
      · rw [beq_iff_eq.mp hss] at hsat
        exact Rat.le_trans hr (Rat.neg_le_neg hsat)
      · rw [beq_iff_eq.mp hss] at hsat
        rw [hsat]; exact hr
    | eq =>
      rw [hd] at hs
      simp only [Bool.and_eq_true, beq_iff_eq] at hs
      rw [hs.1] at hsat
      rw [hsat, hs.2]

/-- Checks whether `P`'s base bounds at `perm[k]` directly imply `dst.sat (x ∘ perm)`. -/
def checkBaseBoundImpliesPerm (P : VIPRInstance) (dst : LinCon P.n) (perm : Array Nat) : Bool :=
  loop 0
where
  loop (k : Nat) : Bool :=
    if hk : k < P.n then
      if hp : k < perm.size then
        let p := perm[k]
        if hpn : p < P.n then
          let jk : Fin P.n := ⟨k, hk⟩
          let jp : Fin P.n := ⟨p, hpn⟩
          if dst.coeffs.beq (RatVec.unit P.n jk) then
            match dst.sense with
            | .le =>
              match P.ub jp with
              | some u => decide (u ≤ dst.rhs)
              | none => false
            | .ge =>
              match P.lb jp with
              | some l => decide (dst.rhs ≤ l)
              | none => false
            | .eq =>
              match P.lb jp, P.ub jp with
              | some l, some u => decide (dst.rhs ≤ l) && decide (u ≤ dst.rhs)
              | _, _ => false
          else loop (k + 1)
        else false
      else false
    else false
  termination_by P.n - k

private theorem checkBaseBoundImpliesPerm_loop_sound (P : VIPRInstance) (dst : LinCon P.n)
    (perm : Array Nat) (hsz : perm.size = P.n)
    (hlt : ∀ j : Fin P.n, permAt perm hsz j < P.n) :
    ∀ k,
      checkBaseBoundImpliesPerm.loop P dst perm k = true →
      ∀ x : Fin P.n → Rat, MILPFeasible P x →
        dst.sat (fun j => x (permFn perm hsz hlt j)) := by
  intro k hloop x hx
  unfold checkBaseBoundImpliesPerm.loop at hloop
  dsimp only at hloop
  split at hloop
  · next hk =>
    have hpk : k < perm.size := by rw [hsz]; exact hk
    have hpn : perm[k]'hpk < P.n := hlt ⟨k, hk⟩
    simp only [hpk, hpn, ↓reduceDIte] at hloop
    split at hloop
    · next hunit =>
      have hcEq : dst.coeffs = RatVec.unit P.n ⟨k, hk⟩ := RatVec.eq_of_beq hunit
      unfold LinCon.sat
      rw [hcEq, RatVec.dot_unit]
      cases hs : dst.sense with
      | le =>
        rw [hs] at hloop
        cases hub : P.ub ⟨perm[k]'hpk, hpn⟩ with
        | none => simp [hub] at hloop
        | some u =>
          rw [hub] at hloop
          exact Rat.le_trans ((hx.1 ⟨perm[k]'hpk, hpn⟩).2 u hub) (decide_eq_true_eq.mp hloop)
      | ge =>
        rw [hs] at hloop
        cases hlb : P.lb ⟨perm[k]'hpk, hpn⟩ with
        | none => simp [hlb] at hloop
        | some l =>
          rw [hlb] at hloop
          exact Rat.le_trans (decide_eq_true_eq.mp hloop) ((hx.1 ⟨perm[k]'hpk, hpn⟩).1 l hlb)
      | eq =>
        rw [hs] at hloop
        cases hlb : P.lb ⟨perm[k]'hpk, hpn⟩ with
        | none => simp [hlb] at hloop
        | some l =>
          cases hub : P.ub ⟨perm[k]'hpk, hpn⟩ with
          | none => simp [hlb, hub] at hloop
          | some u =>
            rw [hlb, hub] at hloop
            simp only [Bool.and_eq_true] at hloop
            exact Rat.le_antisymm
              (Rat.le_trans ((hx.1 ⟨perm[k]'hpk, hpn⟩).2 u hub) (decide_eq_true_eq.mp hloop.2))
              (Rat.le_trans (decide_eq_true_eq.mp hloop.1) ((hx.1 ⟨perm[k]'hpk, hpn⟩).1 l hlb))
    · exact checkBaseBoundImpliesPerm_loop_sound P dst perm hsz hlt (k + 1) hloop x hx
  · contradiction
termination_by k _ _ _ => P.n - k

private def optLbLe (a b : Option Rat) : Bool :=
  match a, b with
  | none, _ => true
  | some l1, some l2 => decide (l1 ≤ l2)
  | some _, none => false

private def optUbLe (a b : Option Rat) : Bool :=
  match a, b with
  | _, none => true
  | some u1, some u2 => decide (u1 ≤ u2)
  | none, some _ => false

/-- Checks that `w` is an exact node-level automorphism mapping `br ↦ c`, preserving `P`'s bounds,
integrality, objective, base rows, conclusion `rightCon`, and all active `rightRem` assumptions. -/
def checkOrbitAutomorphism (P : VIPRInstance) (entries : Array (DerivedEntry P.n))
    (outAsms rightRem : List Nat) (br c : Fin P.n)
    (rightCon : LinCon P.n) (w : OrbitWitness) : Bool :=
  checkPermBijection P.n w.colPerm &&
  (if hbr : br.1 < w.colPerm.size then w.colPerm[br.1] == c.1 else false) &&
  (w.rowPerm.size == P.baseRows.size) &&
  checkCols 0 &&
  checkRows 0 &&
  (isUnsatCon rightCon || checkCoeffsPermMatch rightCon.coeffs rightCon.coeffs w.colPerm false) &&
  checkSupports 0 &&
  checkRightRem rightRem
where
  checkCols (i : Nat) : Bool :=
    if hi : i < P.n then
      if hp : i < w.colPerm.size then
        let p := w.colPerm[i]
        if hpn : p < P.n then
          let j : Fin P.n := ⟨i, hi⟩
          let jp : Fin P.n := ⟨p, hpn⟩
          let okInt := !P.isInt j || P.isInt jp
          let okObj := P.obj j == P.obj jp
          let okLb := optLbLe (P.lb j) (P.lb jp)
          let okUb := optUbLe (P.ub jp) (P.ub j)
          if okInt && okObj && okLb && okUb then checkCols (i + 1) else false
        else false
      else false
    else true
  termination_by P.n - i
  checkRows (i : Nat) : Bool :=
    if hi : i < P.baseRows.size then
      if hrp : i < w.rowPerm.size then
        let rIdx := w.rowPerm[i]
        if hr : rIdx < P.baseRows.size then
          if checkConPermImplies P.baseRows[rIdx] P.baseRows[i] w.colPerm then
            checkRows (i + 1)
          else false
        else false
      else false
    else true
  termination_by P.baseRows.size - i
  checkSupports (i : Nat) : Bool :=
    if hi : i < w.supportEntries.size then
      let b := w.supportEntries[i]
      if hb : b < entries.size then
        if entries[b].assumptions.all (fun d => outAsms.contains d) then
          checkSupports (i + 1)
        else false
      else false
    else true
  termination_by w.supportEntries.size - i
  anyOutMatch (conA : LinCon P.n) : List Nat → Bool
    | [] => false
    | b :: bs =>
      let ok := if hb : b < entries.size then checkConPermImplies entries[b].con conA w.colPerm else false
      ok || anyOutMatch conA bs
  anySupportMatch (conA : LinCon P.n) (i : Nat) : Bool :=
    if hi : i < w.supportEntries.size then
      let b := w.supportEntries[i]
      let ok := if hb : b < entries.size then checkConPermImplies entries[b].con conA w.colPerm else false
      ok || anySupportMatch conA (i + 1)
    else false
  termination_by w.supportEntries.size - i
  checkRightRem : List Nat → Bool
    | [] => true
    | a :: rest =>
      if ha : a < entries.size then
        let conA := entries[a].con
        let ok :=
          checkBaseBoundImpliesPerm P conA w.colPerm ||
          checkConPermImplies conA conA w.colPerm ||
          anyOutMatch conA outAsms ||
          anySupportMatch conA 0
        ok && checkRightRem rest
      else false

private theorem checkOrbitAutomorphism_cols_sound (P : VIPRInstance) (w : OrbitWitness)
    (hsz : w.colPerm.size = P.n) (hlt : ∀ j : Fin P.n, permAt w.colPerm hsz j < P.n) :
    ∀ k (hk : k ≤ P.n),
      (∀ i (hi : i < k),
        let j : Fin P.n := ⟨i, Nat.lt_of_lt_of_le hi hk⟩
        let jp : Fin P.n := permFn w.colPerm hsz hlt j
        (P.isInt j = true → P.isInt jp = true) ∧
        P.obj j = P.obj jp ∧
        (∀ l, P.lb j = some l → ∃ l', P.lb jp = some l' ∧ l ≤ l') ∧
        (∀ u, P.ub j = some u → ∃ u', P.ub jp = some u' ∧ u' ≤ u)) →
      checkOrbitAutomorphism.checkCols P w k = true →
      ∀ j : Fin P.n,
        let jp : Fin P.n := permFn w.colPerm hsz hlt j
        (P.isInt j = true → P.isInt jp = true) ∧
        P.obj j = P.obj jp ∧
        (∀ l, P.lb j = some l → ∃ l', P.lb jp = some l' ∧ l ≤ l') ∧
        (∀ u, P.ub j = some u → ∃ u', P.ub jp = some u' ∧ u' ≤ u) := by
  intro k hk hprev hloop j
  unfold checkOrbitAutomorphism.checkCols at hloop
  dsimp only at hloop
  split at hloop
  · next hltK =>
    have hpk : k < w.colPerm.size := by rw [hsz]; exact hltK
    have hpn : w.colPerm[k]'hpk < P.n := hlt ⟨k, hltK⟩
    simp only [hpk, hpn, ↓reduceDIte] at hloop
    split at hloop
    · next hok =>
      let jk : Fin P.n := ⟨k, hltK⟩
      let jpk : Fin P.n := ⟨w.colPerm[k]'hpk, hpn⟩
      simp only [Bool.and_eq_true] at hok
      have hcurInt : P.isInt jk = true → P.isInt jpk = true := by
        intro hi
        rcases Bool.or_eq_true _ _ |>.mp hok.1.1.1 with hno | hyes
        · rw [hi] at hno; contradiction
        · exact hyes
      have hcurObj : P.obj jk = P.obj jpk := beq_iff_eq.mp hok.1.1.2
      have hcurLb : ∀ l, P.lb jk = some l → ∃ l', P.lb jpk = some l' ∧ l ≤ l' := by
        intro l hl
        have hlb := hok.1.2
        unfold optLbLe at hlb
        rw [hl] at hlb
        cases h2 : P.lb jpk with
        | none => rw [h2] at hlb; contradiction
        | some l' =>
          rw [h2] at hlb
          exact ⟨l', rfl, decide_eq_true_eq.mp hlb⟩
      have hcurUb : ∀ u, P.ub jk = some u → ∃ u', P.ub jpk = some u' ∧ u' ≤ u := by
        intro u hu
        have hub := hok.2
        unfold optUbLe at hub
        rw [hu] at hub
        cases h2 : P.ub jpk with
        | none => rw [h2] at hub; contradiction
        | some u' =>
          rw [h2] at hub
          exact ⟨u', rfl, decide_eq_true_eq.mp hub⟩
      have hnext : ∀ i (hi : i < k + 1),
          let ji : Fin P.n := ⟨i, Nat.lt_of_lt_of_le hi hltK⟩
          let jpi : Fin P.n := permFn w.colPerm hsz hlt ji
          (P.isInt ji = true → P.isInt jpi = true) ∧
          P.obj ji = P.obj jpi ∧
          (∀ l, P.lb ji = some l → ∃ l', P.lb jpi = some l' ∧ l ≤ l') ∧
          (∀ u, P.ub ji = some u → ∃ u', P.ub jpi = some u' ∧ u' ≤ u) := by
        intro i hi
        by_cases hik : i = k
        · subst hik; exact ⟨hcurInt, hcurObj, hcurLb, hcurUb⟩
        · exact hprev i (by omega)
      exact checkOrbitAutomorphism_cols_sound P w hsz hlt (k + 1) hltK hnext hloop j
    · contradiction
  · next hge =>
    have hkn : k = P.n := by omega
    subst hkn
    exact hprev j.1 j.2
termination_by k _ _ _ => P.n - k

private theorem checkOrbitAutomorphism_rows_sound (P : VIPRInstance) (w : OrbitWitness)
    (hsz : w.colPerm.size = P.n) (hlt : ∀ j : Fin P.n, permAt w.colPerm hsz j < P.n)
    (hf : ∀ i₁ i₂ : Fin P.n, permFn w.colPerm hsz hlt i₁ = permFn w.colPerm hsz hlt i₂ → i₁ = i₂)
    (hrsz : w.rowPerm.size = P.baseRows.size) :
    ∀ k (hk : k ≤ P.baseRows.size),
      (∀ i (hi : i < k),
        ∀ x : Fin P.n → Rat, MILPFeasible P x →
          P.baseRows[i].sat (fun j => x (permFn w.colPerm hsz hlt j))) →
      checkOrbitAutomorphism.checkRows P w k = true →
      ∀ i : Fin P.baseRows.size,
        ∀ x : Fin P.n → Rat, MILPFeasible P x →
          P.baseRows[i].sat (fun j => x (permFn w.colPerm hsz hlt j)) := by
  intro k hk hprev hloop i x hx
  unfold checkOrbitAutomorphism.checkRows at hloop
  dsimp only at hloop
  split at hloop
  · next hltK =>
    have hrp : k < w.rowPerm.size := by rw [hrsz]; exact hltK
    simp only [hrp, ↓reduceDIte] at hloop
    split at hloop
    · next hr =>
      split at hloop
      · next himp =>
        have hcur : ∀ y : Fin P.n → Rat, MILPFeasible P y →
            P.baseRows[k].sat (fun j => y (permFn w.colPerm hsz hlt j)) := by
          intro y hy
          exact checkConPermImplies_sound hsz hlt hf himp (hy.2.1 ⟨w.rowPerm[k], hr⟩)
        have hnext : ∀ m (hm : m < k + 1),
            ∀ y : Fin P.n → Rat, MILPFeasible P y →
              P.baseRows[m].sat (fun j => y (permFn w.colPerm hsz hlt j)) := by
          intro m hm
          by_cases hmk : m = k
          · subst hmk; exact hcur
          · exact hprev m (by omega)
        exact checkOrbitAutomorphism_rows_sound P w hsz hlt hf hrsz (k + 1) hltK hnext hloop i x hx
      · contradiction
    · contradiction
  · next hge =>
    have hkn : k = P.baseRows.size := by omega
    subst hkn
    exact hprev i.1 i.2 x hx
termination_by k _ _ _ _ _ => P.baseRows.size - k

private theorem checkOrbitAutomorphism_supports_sound (P : VIPRInstance)
    (entries : Array (DerivedEntry P.n)) (outAsms : List Nat) (w : OrbitWitness) :
    ∀ k (hk : k ≤ w.supportEntries.size),
      (∀ i (hi : i < k),
        ∃ hb : w.supportEntries[i]'(Nat.lt_of_lt_of_le hi hk) < entries.size,
          ∀ d ∈ entries[w.supportEntries[i]'(Nat.lt_of_lt_of_le hi hk)].assumptions, d ∈ outAsms) →
      checkOrbitAutomorphism.checkSupports P entries outAsms w k = true →
      ∀ i : Fin w.supportEntries.size,
        ∃ hb : w.supportEntries[i] < entries.size,
          ∀ d ∈ entries[w.supportEntries[i]].assumptions, d ∈ outAsms := by
  intro k hk hprev hloop i
  unfold checkOrbitAutomorphism.checkSupports at hloop
  dsimp only at hloop
  split at hloop
  · next hltK =>
    split at hloop
    · next hb =>
      split at hloop
      · next hall =>
        have hcur : ∀ d ∈ entries[w.supportEntries[k]].assumptions, d ∈ outAsms := by
          intro d hd
          exact List.contains_iff_mem.mp (List.all_eq_true.mp hall d hd)
        have hnext : ∀ m (hm : m < k + 1),
            ∃ hb' : w.supportEntries[m]'(Nat.lt_of_lt_of_le hm hltK) < entries.size,
              ∀ d ∈ entries[w.supportEntries[m]'(Nat.lt_of_lt_of_le hm hltK)].assumptions, d ∈ outAsms := by
          intro m hm
          by_cases hmk : m = k
          · subst hmk; exact ⟨hb, hcur⟩
          · exact hprev m (by omega)
        exact checkOrbitAutomorphism_supports_sound P entries outAsms w (k + 1) hltK hnext hloop i
      · contradiction
    · contradiction
  · next hge =>
    have hkn : k = w.supportEntries.size := by omega
    subst hkn
    exact hprev i.1 i.2
termination_by k _ _ _ => w.supportEntries.size - k

private theorem anyOutMatch_sound (P : VIPRInstance) (entries : Array (DerivedEntry P.n))
    (w : OrbitWitness) (hsz : w.colPerm.size = P.n)
    (hlt : ∀ j : Fin P.n, permAt w.colPerm hsz j < P.n)
    (hf : ∀ i₁ i₂ : Fin P.n, permFn w.colPerm hsz hlt i₁ = permFn w.colPerm hsz hlt i₂ → i₁ = i₂)
    (conA : LinCon P.n) (x : Fin P.n → Rat) :
    ∀ bs : List Nat,
      (∀ b ∈ bs, ∃ hb : b < entries.size, entries[b].con.sat x) →
      checkOrbitAutomorphism.anyOutMatch P entries w conA bs = true →
      conA.sat (fun j => x (permFn w.colPerm hsz hlt j)) := by
  intro bs hsat hchk
  induction bs with
  | nil => simp [checkOrbitAutomorphism.anyOutMatch] at hchk
  | cons b rest ih =>
    unfold checkOrbitAutomorphism.anyOutMatch at hchk
    rcases Bool.or_eq_true _ _ |>.mp hchk with hhead | htail
    · split at hhead
      · next hb =>
        obtain ⟨_, hsb⟩ := hsat b (by simp)
        exact checkConPermImplies_sound hsz hlt hf hhead hsb
      · contradiction
    · exact ih (fun b' hb' => hsat b' (List.mem_cons_of_mem b hb')) htail

private theorem anySupportMatch_sound (P : VIPRInstance) (entries : Array (DerivedEntry P.n))
    (ht : ValidTable P entries) (outAsms : List Nat) (w : OrbitWitness)
    (hsz : w.colPerm.size = P.n) (hlt : ∀ j : Fin P.n, permAt w.colPerm hsz j < P.n)
    (hf : ∀ i₁ i₂ : Fin P.n, permFn w.colPerm hsz hlt i₁ = permFn w.colPerm hsz hlt i₂ → i₁ = i₂)
    (hsupp : ∀ i : Fin w.supportEntries.size,
      ∃ hb : w.supportEntries[i] < entries.size,
        ∀ d ∈ entries[w.supportEntries[i]].assumptions, d ∈ outAsms)
    (conA : LinCon P.n) (x : Fin P.n → Rat) (hx : MILPFeasible P x)
    (hasm : asmsSat entries outAsms x) :
    ∀ k,
      checkOrbitAutomorphism.anySupportMatch P entries w conA k = true →
      conA.sat (fun j => x (permFn w.colPerm hsz hlt j)) := by
  intro k hchk
  unfold checkOrbitAutomorphism.anySupportMatch at hchk
  dsimp only at hchk
  split at hchk
  · next hk =>
    rcases Bool.or_eq_true _ _ |>.mp hchk with hhead | htail
    · split at hhead
      · next hb =>
        obtain ⟨_, hsub⟩ := hsupp ⟨k, hk⟩
        have hsb : entries[w.supportEntries[k]].con.sat x :=
          (ht w.supportEntries[k] hb).2 x hx (asmsSat_mono hsub hasm)
        exact checkConPermImplies_sound hsz hlt hf hhead hsb
      · contradiction
    · exact anySupportMatch_sound P entries ht outAsms w hsz hlt hf hsupp conA x hx hasm (k + 1) htail
  · contradiction
termination_by k _ => w.supportEntries.size - k

private theorem checkRightRem_sound (P : VIPRInstance) (entries : Array (DerivedEntry P.n))
    (ht : ValidTable P entries) (outAsms : List Nat) (w : OrbitWitness)
    (hsz : w.colPerm.size = P.n) (hlt : ∀ j : Fin P.n, permAt w.colPerm hsz j < P.n)
    (hf : ∀ i₁ i₂ : Fin P.n, permFn w.colPerm hsz hlt i₁ = permFn w.colPerm hsz hlt i₂ → i₁ = i₂)
    (hsupp : ∀ i : Fin w.supportEntries.size,
      ∃ hb : w.supportEntries[i] < entries.size,
        ∀ d ∈ entries[w.supportEntries[i]].assumptions, d ∈ outAsms)
    (x : Fin P.n → Rat) (hx : MILPFeasible P x) (hasm : asmsSat entries outAsms x) :
    ∀ rs : List Nat,
      (∀ a ∈ rs, a ∈ outAsms) →
      checkOrbitAutomorphism.checkRightRem P entries outAsms w rs = true →
      asmsSat entries rs (fun j => x (permFn w.colPerm hsz hlt j)) := by
  intro rs hsub hchk
  induction rs with
  | nil =>
    intro a ha; simp at ha
  | cons a rest ih =>
    unfold checkOrbitAutomorphism.checkRightRem at hchk
    dsimp only at hchk
    split at hchk
    · next ha =>
      simp only [Bool.and_eq_true] at hchk
      intro q hq
      rcases List.mem_cons.mp hq with hqa | hrest
      · subst hqa
        refine ⟨ha, ?_⟩
        rcases Bool.or_eq_true _ _ |>.mp hchk.1 with h123 | h4
        · rcases Bool.or_eq_true _ _ |>.mp h123 with h12 | h3
          · rcases Bool.or_eq_true _ _ |>.mp h12 with h1 | h2
            · exact checkBaseBoundImpliesPerm_loop_sound P entries[q].con w.colPerm hsz hlt 0 h1 x hx
            · obtain ⟨_, hsa⟩ := hasm q (hsub q (by simp))
              exact checkConPermImplies_sound hsz hlt hf h2 hsa
          · exact anyOutMatch_sound P entries w hsz hlt hf entries[q].con x outAsms hasm h3
        · exact anySupportMatch_sound P entries ht outAsms w hsz hlt hf hsupp entries[q].con x hx hasm 0 h4
      · exact ih (fun y hy => hsub y (List.mem_cons_of_mem a hy)) hchk.2 q hrest
    · contradiction

/-- Soundness of the executable node-automorphism checker for orbital branching. -/
theorem checkOrbitAutomorphism_sound
    {P : VIPRInstance} {entries : Array (DerivedEntry P.n)}
    (ht : ValidTable P entries)
    {outAsms rightRem : List Nat} {br c : Fin P.n}
    {rightCon : LinCon P.n} {w : OrbitWitness}
    (hcheck : checkOrbitAutomorphism P entries outAsms rightRem br c rightCon w = true)
    (hsubR : ∀ a ∈ rightRem, a ∈ outAsms)
    {x : Fin P.n → Rat} (hx : MILPFeasible P x)
    (hasm : asmsSat entries outAsms x) (hxc : (1 : Rat) ≤ x c) :
    ∃ x' : Fin P.n → Rat,
      MILPFeasible P x' ∧
      LPObjective P x' = LPObjective P x ∧
      (1 : Rat) ≤ x' br ∧
      asmsSat entries rightRem x' ∧
      (rightCon.sat x' → rightCon.sat x) := by
  unfold checkOrbitAutomorphism at hcheck
  simp only [Bool.and_eq_true] at hcheck
  rcases hcheck with ⟨⟨⟨⟨⟨⟨⟨hbij, hbrMap⟩, hrszB⟩, hcols⟩, hrows⟩, hconInv⟩, hsupp⟩, hrem⟩
  obtain ⟨hsz, hlt, hf⟩ := checkPermBijection_spec hbij
  have hrsz : w.rowPerm.size = P.baseRows.size := beq_iff_eq.mp hrszB
  let f : Fin P.n → Fin P.n := permFn w.colPerm hsz hlt
  let x' : Fin P.n → Rat := fun j => x (f j)
  have hcolSpec := checkOrbitAutomorphism_cols_sound P w hsz hlt 0 (Nat.zero_le P.n)
    (fun _ hi => by omega) hcols
  have hrowSpec := checkOrbitAutomorphism_rows_sound P w hsz hlt hf hrsz 0 (Nat.zero_le P.baseRows.size)
    (fun _ hi => by omega) hrows
  have hsuppSpec := checkOrbitAutomorphism_supports_sound P entries outAsms w 0
    (Nat.zero_le w.supportEntries.size) (fun _ hi => by omega) hsupp
  have hx'Feas : MILPFeasible P x' := by
    refine ⟨fun j => ⟨?_, ?_⟩, fun i => hrowSpec i x hx, fun j hj => ?_⟩
    · intro l hl
      obtain ⟨l', hl', hle⟩ := (hcolSpec j).2.2.1 l hl
      exact Rat.le_trans hle ((hx.1 (f j)).1 l' hl')
    · intro u hu
      obtain ⟨u', hu', hle⟩ := (hcolSpec j).2.2.2 u hu
      exact Rat.le_trans ((hx.1 (f j)).2 u' hu') hle
    · exact hx.2.2 (f j) ((hcolSpec j).1 hj)
  have hobjEq : LPObjective P x' = LPObjective P x := by
    unfold LPObjective
    congr 1
    have h1 : finSum P.n (fun j => P.obj j * x (f j)) =
        finSum P.n (fun j => P.obj (f j) * x (f j)) :=
      finSum_congr (fun j => by rw [(hcolSpec j).2.1])
    rw [h1]
    exact finSum_inj_eq (g := fun k => P.obj k * x k) hf
  have hx'Br : (1 : Rat) ≤ x' br := by
    have hbrLt : br.1 < w.colPerm.size := by rw [hsz]; exact br.2
    simp only [hbrLt, ↓reduceDIte, beq_iff_eq] at hbrMap
    have hfeq : f br = c := Fin.ext hbrMap
    dsimp only [x']
    rw [hfeq]
    exact hxc
  have hremSat : asmsSat entries rightRem x' :=
    checkRightRem_sound P entries ht outAsms w hsz hlt hf hsuppSpec x hx hasm rightRem hsubR hrem
  have hconSat : rightCon.sat x' → rightCon.sat x := by
    intro hsat'
    rcases Bool.or_eq_true _ _ |>.mp hconInv with hunsat | hmatch
    · exact absurd hsat' (isUnsatCon_sound hunsat x')
    · have hm := checkCoeffsPermMatch_loop_sound rightCon.coeffs rightCon.coeffs w.colPerm false
        hsz hlt 0 (Nat.zero_le P.n) (fun _ hi => by omega) hmatch
      have hm' : ∀ j : Fin P.n, rightCon.coeffs.get j = rightCon.coeffs.get (f j) := by
        intro j; simpa [f] using hm j
      have hdot : rightCon.coeffs.dot x' = rightCon.coeffs.dot x := dot_perm_pos hf hm' x
      unfold LinCon.sat at hsat' ⊢
      rwa [hdot] at hsat'
  exact ⟨x', hx'Feas, hobjEq, hx'Br, hremSat, hconSat⟩

private def checkOrbitWitnesses (P : VIPRInstance) (entries : Array (DerivedEntry P.n))
    (outAsms rightRem : List Nat) (br : Fin P.n) (rightCon : LinCon P.n)
    (orbitAsms : Array Nat) (witnesses : Array OrbitWitness) (i : Nat) : Bool :=
  if hi : i < orbitAsms.size then
    if hw : i < witnesses.size then
      let oa := orbitAsms[i]
      let w := witnesses[i]
      if hoa : oa < entries.size then
        if hc : w.companion < P.n then
          if P.isInt ⟨w.companion, hc⟩ then
            let wantOA : LinCon P.n :=
              { coeffs := RatVec.unit P.n ⟨w.companion, hc⟩, sense := .le, rhs := 0 }
            if entries[oa].con.beq wantOA && (entries[oa].assumptions == [oa]) &&
                checkOrbitAutomorphism P entries outAsms rightRem br ⟨w.companion, hc⟩ rightCon w then
              checkOrbitWitnesses P entries outAsms rightRem br rightCon orbitAsms witnesses (i + 1)
            else false
          else false
        else false
      else false
    else false
  else true
termination_by orbitAsms.size - i

private theorem checkOrbitWitnesses_sound (P : VIPRInstance) (entries : Array (DerivedEntry P.n))
    (outAsms rightRem : List Nat) (br : Fin P.n) (rightCon : LinCon P.n)
    (orbitAsms : Array Nat) (witnesses : Array OrbitWitness)
    (hsz : orbitAsms.size = witnesses.size) :
    ∀ k (hk : k ≤ orbitAsms.size),
      (∀ i (hi : i < k),
        let oa := orbitAsms[i]'(Nat.lt_of_lt_of_le hi hk)
        let w := witnesses[i]'(by rw [← hsz]; exact Nat.lt_of_lt_of_le hi hk)
        ∃ hoa : oa < entries.size, ∃ hc : w.companion < P.n,
          P.isInt ⟨w.companion, hc⟩ = true ∧
          entries[oa].con = { coeffs := RatVec.unit P.n ⟨w.companion, hc⟩, sense := .le, rhs := 0 } ∧
          checkOrbitAutomorphism P entries outAsms rightRem br ⟨w.companion, hc⟩ rightCon w = true) →
      checkOrbitWitnesses P entries outAsms rightRem br rightCon orbitAsms witnesses k = true →
      ∀ i : Fin orbitAsms.size,
        let oa := orbitAsms[i.1]
        let w := witnesses[i.1]'(by rw [← hsz]; exact i.2)
        ∃ hoa : oa < entries.size, ∃ hc : w.companion < P.n,
          P.isInt ⟨w.companion, hc⟩ = true ∧
          entries[oa].con = { coeffs := RatVec.unit P.n ⟨w.companion, hc⟩, sense := .le, rhs := 0 } ∧
          checkOrbitAutomorphism P entries outAsms rightRem br ⟨w.companion, hc⟩ rightCon w = true := by
  intro k hk hprev hloop i
  unfold checkOrbitWitnesses at hloop
  dsimp only at hloop
  split at hloop
  · next hltK =>
    have hwK : k < witnesses.size := by rw [← hsz]; exact hltK
    simp only [hwK, ↓reduceDIte] at hloop
    split at hloop
    · next hoa =>
      split at hloop
      · next hc =>
        split at hloop
        · next hint =>
          split at hloop
          · next hok =>
            simp only [Bool.and_eq_true] at hok
            have hcur : ∃ hoa' : orbitAsms[k] < entries.size,
                ∃ hc' : witnesses[k].companion < P.n,
                  P.isInt ⟨witnesses[k].companion, hc'⟩ = true ∧
                  entries[orbitAsms[k]].con =
                    { coeffs := RatVec.unit P.n ⟨witnesses[k].companion, hc'⟩, sense := .le, rhs := 0 } ∧
                  checkOrbitAutomorphism P entries outAsms rightRem br
                    ⟨witnesses[k].companion, hc'⟩ rightCon witnesses[k] = true :=
              ⟨hoa, hc, hint, LinCon.eq_of_beq hok.1.1, hok.2⟩
            have hnext : ∀ m (hm : m < k + 1),
                let oa := orbitAsms[m]'(Nat.lt_of_lt_of_le hm hltK)
                let w := witnesses[m]'(by rw [← hsz]; exact Nat.lt_of_lt_of_le hm hltK)
                ∃ hoa' : oa < entries.size, ∃ hc' : w.companion < P.n,
                  P.isInt ⟨w.companion, hc'⟩ = true ∧
                  entries[oa].con = { coeffs := RatVec.unit P.n ⟨w.companion, hc'⟩, sense := .le, rhs := 0 } ∧
                  checkOrbitAutomorphism P entries outAsms rightRem br ⟨w.companion, hc'⟩ rightCon w = true := by
              intro m hm
              by_cases hmk : m = k
              · subst hmk; exact hcur
              · exact hprev m (by omega)
            exact checkOrbitWitnesses_sound P entries outAsms rightRem br rightCon orbitAsms witnesses
              hsz (k + 1) hltK hnext hloop i
          · contradiction
        · contradiction
      · contradiction
    · contradiction
  · next hge =>
    have hkn : k = orbitAsms.size := by omega
    subst hkn
    exact hprev i.1 i.2
termination_by k _ _ _ => orbitAsms.size - k

private def checkAllLt (bound : Nat) (arr : Array Nat) (i : Nat) : Bool :=
  if hi : i < arr.size then
    if arr[i] < bound then checkAllLt bound arr (i + 1) else false
  else true
termination_by arr.size - i

private theorem checkAllLt_sound (bound : Nat) (arr : Array Nat) :
    ∀ k (hk : k ≤ arr.size),
      (∀ i (hi : i < k), arr[i]'(Nat.lt_of_lt_of_le hi hk) < bound) →
      checkAllLt bound arr k = true →
      ∀ a ∈ arr.toList, a < bound := by
  intro k hk hprev hloop a ha
  unfold checkAllLt at hloop
  split at hloop
  · next hlt =>
    split at hloop
    · next hok =>
      have hnext : ∀ i (hi : i < k + 1), arr[i]'(Nat.lt_of_lt_of_le hi hlt) < bound := by
        intro i hi
        by_cases hik : i = k
        · subst hik; exact hok
        · exact hprev i (by omega)
      exact checkAllLt_sound bound arr (k + 1) hlt hnext hloop a ha
    · contradiction
  · next hge =>
    have hkn : k = arr.size := by omega
    subst hkn
    obtain ⟨idx, hidx, rfl⟩ := List.mem_iff_getElem.mp ha
    have hlt' : idx < arr.size := by simpa using hidx
    exact hprev idx hlt'
termination_by k _ _ _ => arr.size - k

/-- Evaluates a single `StepRule` against the current `entries` table. -/
def evalRule (P : VIPRInstance) (entries : Array (DerivedEntry P.n))
    (rule : StepRule P.n) : Option (DerivedEntry P.n) :=
  match rule with
  | .conIndex row =>
    if hr : row < P.baseRows.size then
      some { con := P.baseRows[row], assumptions := [] }
    else none
  | .assume col isUpper bound =>
    if hcol : col < P.n then
      if P.isInt ⟨col, hcol⟩ then
        some {
          con := {
            coeffs := RatVec.unit P.n ⟨col, hcol⟩
            sense := if isUpper then .le else .ge
            rhs := (bound : Rat)
          }
          assumptions := [entries.size]
        }
      else none
    else none
  | .lincomb sense refs bounds =>
    evalLinComb P entries sense refs bounds
  | .roundDown ref =>
    if hr : ref < entries.size then
      if (entries[ref].con.sense == .le || entries[ref].con.sense == .eq) &&
          isIntegralLinVec P entries[ref].con.coeffs then
        some {
          con := { coeffs := entries[ref].con.coeffs, sense := .le, rhs := (entries[ref].con.rhs.floor : Rat) }
          assumptions := entries[ref].assumptions
        }
      else none
    else none
  | .roundUp ref =>
    if hr : ref < entries.size then
      if (entries[ref].con.sense == .ge || entries[ref].con.sense == .eq) &&
          isIntegralLinVec P entries[ref].con.coeffs then
        some {
          con := { coeffs := entries[ref].con.coeffs, sense := .ge, rhs := (entries[ref].con.rhs.ceil : Rat) }
          assumptions := entries[ref].assumptions
        }
      else none
    else none
  | .branch col splitVal leftAsm rightAsm left right =>
    if hcol : col < P.n then
      if P.isInt ⟨col, hcol⟩ then
        if hla : leftAsm < entries.size then
          if hra : rightAsm < entries.size then
            if hl : left < entries.size then
              if hr : right < entries.size then
                let wantLA : LinCon P.n :=
                  { coeffs := RatVec.unit P.n ⟨col, hcol⟩, sense := .le, rhs := (splitVal : Rat) }
                let wantRA : LinCon P.n :=
                  { coeffs := RatVec.unit P.n ⟨col, hcol⟩, sense := .ge, rhs := ((splitVal + 1 : Int) : Rat) }
                if entries[leftAsm].con.beq wantLA && entries[rightAsm].con.beq wantRA &&
                    (entries[leftAsm].assumptions == [leftAsm]) &&
                    (entries[rightAsm].assumptions == [rightAsm]) then
                  match mergeBranchCon entries[left].con entries[right].con with
                  | some cOut =>
                    let asms := mergeAsms
                      (entries[left].assumptions.filter (· != leftAsm))
                      (entries[right].assumptions.filter (· != rightAsm))
                    some { con := cOut, assumptions := asms }
                  | none => none
                else none
              else none
            else none
          else none
        else none
      else none
    else none
  | .orbitBranch col leftAsm rightAsm left right orbitAsms witnesses extraAsms =>
    if hcol : col < P.n then
      if P.isInt ⟨col, hcol⟩ then
        if hla : leftAsm < entries.size then
          if hra : rightAsm < entries.size then
            if hl : left < entries.size then
              if hr : right < entries.size then
                let wantLA : LinCon P.n :=
                  { coeffs := RatVec.unit P.n ⟨col, hcol⟩, sense := .le, rhs := 0 }
                let wantRA : LinCon P.n :=
                  { coeffs := RatVec.unit P.n ⟨col, hcol⟩, sense := .ge, rhs := 1 }
                if entries[leftAsm].con.beq wantLA && entries[rightAsm].con.beq wantRA &&
                    (entries[leftAsm].assumptions == [leftAsm]) &&
                    (entries[rightAsm].assumptions == [rightAsm]) &&
                    (orbitAsms.size == witnesses.size) &&
                    checkAllLt entries.size extraAsms 0 then
                  match mergeBranchCon entries[left].con entries[right].con with
                  | some cOut =>
                    let leftRem := entries[left].assumptions.filter
                      (fun a => a != leftAsm && !orbitAsms.toList.contains a)
                    let rightRem := entries[right].assumptions.filter (· != rightAsm)
                    let outAsms := mergeAsms (mergeAsms leftRem rightRem) extraAsms.toList
                    if checkOrbitWitnesses P entries outAsms rightRem ⟨col, hcol⟩
                        entries[right].con orbitAsms witnesses 0 then
                      some { con := cOut, assumptions := outAsms }
                    else none
                  | none => none
                else none
              else none
            else none
          else none
        else none
      else none
    else none
  | .farkas refs bounds =>
    match evalLinComb P entries .le refs bounds with
    | none => none
    | some e => if isUnsatCon e.con then some e else none
  | .boundConflict col lbRef ubRef =>
    if hcol : col < P.n then
      let j : Fin P.n := ⟨col, hcol⟩
      match getBoundLB P entries j lbRef, getBoundUB P entries j ubRef with
      | some (l, asmsL), some (u, asmsU) =>
        if u < l then
          some {
            con := { coeffs := RatVec.zero P.n, sense := .le, rhs := u - l }
            assumptions := mergeAsms asmsL asmsU
          }
        else none
      | _, _ => none
    else none
  | .dualBound refs bounds =>
    match evalLinComb P entries .ge refs bounds with
    | none => none
    | some e => if e.con.coeffs.beq (RatVec.ofFn P.obj) then some e else none
  | .safeDual refs boundAsms =>
    evalSafeDual P entries refs boundAsms

/-- Evaluates a single Mini-VIPR `ProofStep` against the current `entries` table. -/
def evalProofStep (P : VIPRInstance) (entries : Array (DerivedEntry P.n))
    (step : ProofStep P.n) : Option (DerivedEntry P.n) :=
  match evalRule P entries step.rule with
  | none => none
  | some entry =>
    let okCon := match step.claimedCon with
      | none => true
      | some cc => entry.con.beq cc
    let okAsms := match step.claimedAssumptions with
      | none => true
      | some ca => checkAssumptionsMatch entry.assumptions ca
    if okCon && okAsms then some entry else none

private theorem getBoundLB_sound {P : VIPRInstance} {entries : Array (DerivedEntry P.n)}
    (ht : ValidTable P entries) (j : Fin P.n) (lbRef : Option Nat) (l : Rat) (asmsL : List Nat)
    (h : getBoundLB P entries j lbRef = some (l, asmsL)) :
    (∀ a ∈ asmsL, a < entries.size) ∧
    (∀ x, MILPFeasible P x → asmsSat entries asmsL x → l ≤ x j) := by
  cases lbRef with
  | none =>
    simp only [getBoundLB] at h
    cases hpl : P.lb j with
    | none => simp [hpl] at h
    | some l0 =>
      simp only [hpl, Option.map_some] at h
      injection h with hp
      have hlEq : l0 = l := congrArg Prod.fst hp
      have haEq : ([] : List Nat) = asmsL := congrArg Prod.snd hp
      subst hlEq; subst haEq
      exact ⟨fun _ ha => by simp at ha, fun x hx _ => (hx.1 j).1 l0 hpl⟩
  | some id =>
    simp only [getBoundLB] at h
    split at h
    · next hid =>
      split at h
      · next hok =>
        simp only [Bool.and_eq_true] at hok
        injection h with hp
        have hlEq : entries[id].con.rhs = l := congrArg Prod.fst hp
        have haEq : entries[id].assumptions = asmsL := congrArg Prod.snd hp
        subst hlEq; subst haEq
        have hEnt := ht id hid
        refine ⟨hEnt.1, fun x hx hasm => ?_⟩
        have hsat := hEnt.2 x hx hasm
        unfold LinCon.sat at hsat
        rw [RatVec.eq_of_beq hok.2, RatVec.dot_unit] at hsat
        rcases Bool.or_eq_true _ _ |>.mp hok.1 with hs | hs
        · rw [beq_iff_eq.mp hs] at hsat; exact hsat
        · rw [beq_iff_eq.mp hs] at hsat; exact hsat ▸ Rat.le_refl
      · contradiction
    · contradiction

private theorem getBoundUB_sound {P : VIPRInstance} {entries : Array (DerivedEntry P.n)}
    (ht : ValidTable P entries) (j : Fin P.n) (ubRef : Option Nat) (u : Rat) (asmsU : List Nat)
    (h : getBoundUB P entries j ubRef = some (u, asmsU)) :
    (∀ a ∈ asmsU, a < entries.size) ∧
    (∀ x, MILPFeasible P x → asmsSat entries asmsU x → x j ≤ u) := by
  cases ubRef with
  | none =>
    simp only [getBoundUB] at h
    cases hpu : P.ub j with
    | none => simp [hpu] at h
    | some u0 =>
      simp only [hpu, Option.map_some] at h
      injection h with hp
      have huEq : u0 = u := congrArg Prod.fst hp
      have haEq : ([] : List Nat) = asmsU := congrArg Prod.snd hp
      subst huEq; subst haEq
      exact ⟨fun _ ha => by simp at ha, fun x hx _ => (hx.1 j).2 u0 hpu⟩
  | some id =>
    simp only [getBoundUB] at h
    split at h
    · next hid =>
      split at h
      · next hok =>
        simp only [Bool.and_eq_true] at hok
        injection h with hp
        have huEq : entries[id].con.rhs = u := congrArg Prod.fst hp
        have haEq : entries[id].assumptions = asmsU := congrArg Prod.snd hp
        subst huEq; subst haEq
        have hEnt := ht id hid
        refine ⟨hEnt.1, fun x hx hasm => ?_⟩
        have hsat := hEnt.2 x hx hasm
        unfold LinCon.sat at hsat
        rw [RatVec.eq_of_beq hok.2, RatVec.dot_unit] at hsat
        rcases Bool.or_eq_true _ _ |>.mp hok.1 with hs | hs
        · rw [beq_iff_eq.mp hs] at hsat; exact hsat
        · rw [beq_iff_eq.mp hs] at hsat; exact hsat ▸ Rat.le_refl
      · contradiction
    · contradiction

private theorem evalRule_valid (P : VIPRInstance) (entries : Array (DerivedEntry P.n))
    (ht : ValidTable P entries) (rule : StepRule P.n) (e : DerivedEntry P.n)
    (he : evalRule P entries rule = some e) :
    ValidTable P (entries.push e) := by
  cases rule with
  | conIndex row =>
    simp only [evalRule] at he
    split at he
    · next hr =>
      injection he with hec; subst hec
      exact validTable_push ht _ (fun _ ha => by simp at ha) (fun x hx _ => hx.2.1 ⟨row, hr⟩)
    · contradiction
  | assume col isUpper bound =>
    simp only [evalRule] at he
    split at he
    · next hcol =>
      split at he
      · injection he with hec; subst hec
        apply validTable_push ht _
        · intro a ha; simp at ha; subst ha; exact Nat.lt_succ_self _
        · intro x _ hasm
          obtain ⟨_, hsat⟩ := hasm entries.size (by simp)
          simpa [Array.getElem_push_eq] using hsat
      · contradiction
    · contradiction
  | lincomb sense refs bounds =>
    simp only [evalRule] at he
    have hv := evalLinComb_sound ht he
    exact validTable_push ht _ (fun a ha => Nat.lt_succ_of_lt (hv.1 a ha))
      (fun x hx hasm => hv.2 x hx (fun a ha => by
        obtain ⟨_, hs⟩ := hasm a ha
        have halt := hv.1 a ha
        rw [Array.getElem_push_lt halt] at hs
        exact ⟨halt, hs⟩))
  | roundDown ref =>
    simp only [evalRule] at he
    split at he
    · next hr =>
      split at he
      · next hok =>
        simp only [Bool.and_eq_true] at hok
        injection he with hec; subst hec
        have hEnt := ht ref hr
        apply validTable_push ht
          { con := { coeffs := entries[ref].con.coeffs, sense := .le, rhs := (entries[ref].con.rhs.floor : Rat) },
            assumptions := entries[ref].assumptions }
          (fun a ha => Nat.lt_succ_of_lt (hEnt.1 a ha))
        intro x hx hasm
        have hsat0 := hEnt.2 x hx (fun a ha => by
          obtain ⟨_, hs⟩ := hasm a ha
          have halt := hEnt.1 a ha
          rw [Array.getElem_push_lt halt] at hs
          exact ⟨halt, hs⟩)
        have hle : entries[ref].con.coeffs.dot x ≤ entries[ref].con.rhs := by
          unfold LinCon.sat at hsat0
          rcases Bool.or_eq_true _ _ |>.mp hok.1 with hs | hs
          · rw [beq_iff_eq.mp hs] at hsat0; exact hsat0
          · rw [beq_iff_eq.mp hs] at hsat0; exact hsat0 ▸ Rat.le_refl
        obtain ⟨z, hz⟩ := isIntegralLinVec_dot_int hok.2 hx
        unfold LinCon.sat
        dsimp only
        rw [hz] at hle ⊢
        rw [Rat.intCast_le_intCast]
        exact Rat.le_floor_iff.mpr hle
      · contradiction
    · contradiction
  | roundUp ref =>
    simp only [evalRule] at he
    split at he
    · next hr =>
      split at he
      · next hok =>
        simp only [Bool.and_eq_true] at hok
        injection he with hec; subst hec
        have hEnt := ht ref hr
        apply validTable_push ht
          { con := { coeffs := entries[ref].con.coeffs, sense := .ge, rhs := (entries[ref].con.rhs.ceil : Rat) },
            assumptions := entries[ref].assumptions }
          (fun a ha => Nat.lt_succ_of_lt (hEnt.1 a ha))
        intro x hx hasm
        have hsat0 := hEnt.2 x hx (fun a ha => by
          obtain ⟨_, hs⟩ := hasm a ha
          have halt := hEnt.1 a ha
          rw [Array.getElem_push_lt halt] at hs
          exact ⟨halt, hs⟩)
        have hge : entries[ref].con.rhs ≤ entries[ref].con.coeffs.dot x := by
          unfold LinCon.sat at hsat0
          rcases Bool.or_eq_true _ _ |>.mp hok.1 with hs | hs
          · rw [beq_iff_eq.mp hs] at hsat0; exact hsat0
          · rw [beq_iff_eq.mp hs] at hsat0; exact hsat0 ▸ Rat.le_refl
        obtain ⟨z, hz⟩ := isIntegralLinVec_dot_int hok.2 hx
        unfold LinCon.sat
        dsimp only
        rw [hz] at hge ⊢
        rw [Rat.intCast_le_intCast]
        exact Rat.ceil_le_iff.mpr hge
      · contradiction
    · contradiction
  | branch col splitVal leftAsm rightAsm left right =>
    simp only [evalRule] at he
    split at he
    · next hcol =>
      split at he
      · next hj =>
        split at he
        · next hla =>
          split at he
          · next hra =>
            split at he
            · next hl =>
              split at he
              · next hr =>
                split at he
                · next hok =>
                  simp only [Bool.and_eq_true] at hok
                  cases hm : mergeBranchCon entries[left].con entries[right].con with
                  | none => simp [hm] at he
                  | some cOut =>
                    simp only [hm] at he
                    injection he with hec; subst hec
                    have hEntL := ht left hl
                    have hEntR := ht right hr
                    have hsub : ∀ a ∈ mergeAsms (entries[left].assumptions.filter (· != leftAsm))
                        (entries[right].assumptions.filter (· != rightAsm)), a < entries.size + 1 := by
                      intro a ha
                      rcases mem_mergeAsms.mp ha with hfl | hfr
                      · exact Nat.lt_succ_of_lt (hEntL.1 a (List.mem_filter.mp hfl).1)
                      · exact Nat.lt_succ_of_lt (hEntR.1 a (List.mem_filter.mp hfr).1)
                    apply validTable_push ht _ hsub
                    intro x hx hasm
                    have hLAeq : entries[leftAsm].con =
                        { coeffs := RatVec.unit P.n ⟨col, hcol⟩, sense := .le, rhs := (splitVal : Rat) } :=
                      LinCon.eq_of_beq hok.1.1.1
                    have hRAeq : entries[rightAsm].con =
                        { coeffs := RatVec.unit P.n ⟨col, hcol⟩, sense := .ge, rhs := ((splitVal + 1 : Int) : Rat) } :=
                      LinCon.eq_of_beq hok.1.1.2
                    have hasm' : ∀ a ∈ mergeAsms (entries[left].assumptions.filter (· != leftAsm))
                        (entries[right].assumptions.filter (· != rightAsm)),
                        ∃ ha : a < entries.size, entries[a].con.sat x := by
                      intro a ha
                      obtain ⟨_, hs⟩ := hasm a ha
                      have halt : a < entries.size := by
                        rcases mem_mergeAsms.mp ha with hfl | hfr
                        · exact hEntL.1 a (List.mem_filter.mp hfl).1
                        · exact hEntR.1 a (List.mem_filter.mp hfr).1
                      rw [Array.getElem_push_lt halt] at hs
                      exact ⟨halt, hs⟩
                    apply mergeBranchCon_sound hm
                    rcases int_split_exhaustive (x ⟨col, hcol⟩) (hx.2.2 ⟨col, hcol⟩ hj) splitVal with hdown | hup
                    · left
                      apply hEntL.2 x hx
                      intro a ha
                      by_cases hala : a = leftAsm
                      · subst hala
                        refine ⟨hla, ?_⟩
                        rw [hLAeq]
                        unfold LinCon.sat
                        rw [RatVec.dot_unit]
                        exact hdown
                      · have hmem : a ∈ entries[left].assumptions.filter (· != leftAsm) :=
                          List.mem_filter.mpr ⟨ha, bne_iff_ne.mpr hala⟩
                        exact hasm' a (mem_mergeAsms.mpr (Or.inl hmem))
                    · right
                      apply hEntR.2 x hx
                      intro a ha
                      by_cases hara : a = rightAsm
                      · subst hara
                        refine ⟨hra, ?_⟩
                        rw [hRAeq]
                        unfold LinCon.sat
                        rw [RatVec.dot_unit]
                        exact hup
                      · have hmem : a ∈ entries[right].assumptions.filter (· != rightAsm) :=
                          List.mem_filter.mpr ⟨ha, bne_iff_ne.mpr hara⟩
                        exact hasm' a (mem_mergeAsms.mpr (Or.inr hmem))
                · contradiction
              · contradiction
            · contradiction
          · contradiction
        · contradiction
      · contradiction
    · contradiction
  | orbitBranch col leftAsm rightAsm left right orbitAsms witnesses extraAsms =>
    simp only [evalRule] at he
    split at he
    · next hcol =>
      split at he
      · next hj =>
        split at he
        · next hla =>
          split at he
          · next hra =>
            split at he
            · next hl =>
              split at he
              · next hr =>
                split at he
                · next hok =>
                  simp only [Bool.and_eq_true] at hok
                  cases hm : mergeBranchCon entries[left].con entries[right].con with
                  | none => simp [hm] at he
                  | some cOut =>
                    simp only [hm] at he
                    split at he
                    · next hwit =>
                      injection he with hec; subst hec
                      have hEntL := ht left hl
                      have hEntR := ht right hr
                      have hszW : orbitAsms.size = witnesses.size := beq_iff_eq.mp hok.1.2
                      have hextraLt : ∀ a ∈ extraAsms.toList, a < entries.size :=
                        checkAllLt_sound entries.size extraAsms 0 (Nat.zero_le _)
                          (fun _ hi => by omega) hok.2
                      let leftRem := entries[left].assumptions.filter
                        (fun a => a != leftAsm && !orbitAsms.toList.contains a)
                      let rightRem := entries[right].assumptions.filter (· != rightAsm)
                      let outAsms := mergeAsms (mergeAsms leftRem rightRem) extraAsms.toList
                      have houtLt : ∀ a ∈ outAsms, a < entries.size := by
                        intro a ha
                        rcases mem_mergeAsms.mp ha with hlr | hex
                        · rcases mem_mergeAsms.mp hlr with hfl | hfr
                          · exact hEntL.1 a (List.mem_filter.mp hfl).1
                          · exact hEntR.1 a (List.mem_filter.mp hfr).1
                        · exact hextraLt a hex
                      have hsub : ∀ a ∈ outAsms, a < entries.size + 1 :=
                        fun a ha => Nat.lt_succ_of_lt (houtLt a ha)
                      have hsubR : ∀ a ∈ rightRem, a ∈ outAsms :=
                        fun a ha => mem_mergeAsms.mpr (Or.inl (mem_mergeAsms.mpr (Or.inr ha)))
                      have hLAeq : entries[leftAsm].con =
                          { coeffs := RatVec.unit P.n ⟨col, hcol⟩, sense := .le, rhs := 0 } :=
                        LinCon.eq_of_beq hok.1.1.1.1.1
                      have hRAeq : entries[rightAsm].con =
                          { coeffs := RatVec.unit P.n ⟨col, hcol⟩, sense := .ge, rhs := 1 } :=
                        LinCon.eq_of_beq hok.1.1.1.1.2
                      have hwSpec := checkOrbitWitnesses_sound P entries outAsms rightRem
                        ⟨col, hcol⟩ entries[right].con orbitAsms witnesses hszW
                        0 (Nat.zero_le _) (fun _ hi => by omega) hwit
                      apply validTable_push ht _ hsub
                      intro x hx hasm
                      have hasm' : asmsSat entries outAsms x := by
                        intro a ha
                        obtain ⟨_, hs⟩ := hasm a ha
                        have halt := houtLt a ha
                        rw [Array.getElem_push_lt halt] at hs
                        exact ⟨halt, hs⟩
                      apply mergeBranchCon_sound hm
                      rcases int_split_exhaustive (x ⟨col, hcol⟩) (hx.2.2 ⟨col, hcol⟩ hj) 0 with hdown | hup
                      · by_cases hall : ∀ i : Fin orbitAsms.size,
                            ∃ hc : (witnesses[i.1]'(by rw [← hszW]; exact i.2)).companion < P.n,
                              x ⟨(witnesses[i.1]'(by rw [← hszW]; exact i.2)).companion, hc⟩ ≤ 0
                        · left
                          apply hEntL.2 x hx
                          intro a ha
                          by_cases hala : a = leftAsm
                          · subst hala
                            refine ⟨hla, ?_⟩
                            rw [hLAeq]; unfold LinCon.sat; rw [RatVec.dot_unit]; exact hdown
                          · by_cases horb : a ∈ orbitAsms.toList
                            · obtain ⟨idx, hidx, hget⟩ := List.mem_iff_getElem.mp horb
                              have hltIdx : idx < orbitAsms.size := by simpa using hidx
                              have haEq : orbitAsms[idx]'hltIdx = a := by simpa using hget
                              obtain ⟨hoa, hc, _, hconOA, _⟩ := hwSpec ⟨idx, hltIdx⟩
                              obtain ⟨hc', hle0⟩ := hall ⟨idx, hltIdx⟩
                              have hoa' : a < entries.size := by simpa [haEq] using hoa
                              have hconOA' : entries[a].con =
                                  { coeffs := RatVec.unit P.n ⟨(witnesses[idx]'(by rw [← hszW]; exact hltIdx)).companion, hc⟩,
                                    sense := .le, rhs := 0 } := by
                                simpa [haEq] using hconOA
                              refine ⟨hoa', ?_⟩
                              rw [hconOA']; unfold LinCon.sat; rw [RatVec.dot_unit]
                              exact hle0
                            · have hnotC : (!orbitAsms.toList.contains a) = true := by
                                simp only [Bool.not_eq_true']
                                exact Bool.eq_false_iff.mpr fun hct => horb (List.contains_iff_mem.mp hct)
                              have hmem : a ∈ leftRem :=
                                List.mem_filter.mpr ⟨ha, Bool.and_eq_true _ _ |>.mpr ⟨bne_iff_ne.mpr hala, hnotC⟩⟩
                              exact hasm' a (mem_mergeAsms.mpr (Or.inl (mem_mergeAsms.mpr (Or.inl hmem))))
                        · right
                          have hex : ∃ i : Fin orbitAsms.size,
                              ¬ ∃ hc : (witnesses[i.1]'(by rw [← hszW]; exact i.2)).companion < P.n,
                                x ⟨(witnesses[i.1]'(by rw [← hszW]; exact i.2)).companion, hc⟩ ≤ 0 := by
                            exact Classical.not_forall.mp hall
                          rcases hex with ⟨idx, hnotle⟩
                          obtain ⟨_, hc, hintC, _, hchkAuto⟩ := hwSpec idx
                          have hnot0 : ¬ (x ⟨(witnesses[idx.1]'(by rw [← hszW]; exact idx.2)).companion, hc⟩ ≤ 0) :=
                            fun hle => hnotle ⟨hc, hle⟩
                          have hxc1 : (1 : Rat) ≤ x ⟨(witnesses[idx.1]'(by rw [← hszW]; exact idx.2)).companion, hc⟩ := by
                            rcases int_split_exhaustive
                                (x ⟨(witnesses[idx.1]'(by rw [← hszW]; exact idx.2)).companion, hc⟩)
                                (hx.2.2 ⟨(witnesses[idx.1]'(by rw [← hszW]; exact idx.2)).companion, hc⟩ hintC) 0 with h0 | h1
                            · exact absurd h0 hnot0
                            · simpa using h1
                          obtain ⟨x', hx'Feas, _, hx'Br, hremSat', hconImp⟩ :=
                            checkOrbitAutomorphism_sound ht hchkAuto hsubR hx hasm' hxc1
                          apply hconImp
                          apply hEntR.2 x' hx'Feas
                          intro a ha
                          by_cases hara : a = rightAsm
                          · subst hara
                            refine ⟨hra, ?_⟩
                            rw [hRAeq]; unfold LinCon.sat; rw [RatVec.dot_unit]
                            exact hx'Br
                          · have hmem : a ∈ rightRem :=
                              List.mem_filter.mpr ⟨ha, bne_iff_ne.mpr hara⟩
                            exact hremSat' a hmem
                      · right
                        apply hEntR.2 x hx
                        intro a ha
                        by_cases hara : a = rightAsm
                        · subst hara
                          refine ⟨hra, ?_⟩
                          rw [hRAeq]; unfold LinCon.sat; rw [RatVec.dot_unit]
                          simpa using hup
                        · have hmem : a ∈ rightRem :=
                            List.mem_filter.mpr ⟨ha, bne_iff_ne.mpr hara⟩
                          exact hasm' a (hsubR a hmem)
                    · contradiction
                · contradiction
              · contradiction
            · contradiction
          · contradiction
        · contradiction
      · contradiction
    · contradiction
  | farkas refs bounds =>
    simp only [evalRule] at he
    cases hlin : evalLinComb P entries .le refs bounds with
    | none => simp [hlin] at he
    | some e0 =>
      simp only [hlin] at he
      split at he
      · injection he with hec; subst hec
        have hv := evalLinComb_sound ht hlin
        exact validTable_push ht _ (fun a ha => Nat.lt_succ_of_lt (hv.1 a ha))
          (fun x hx hasm => hv.2 x hx (fun a ha => by
            obtain ⟨_, hs⟩ := hasm a ha
            have halt := hv.1 a ha
            rw [Array.getElem_push_lt halt] at hs
            exact ⟨halt, hs⟩))
      · contradiction
  | boundConflict col lbRef ubRef =>
    simp only [evalRule] at he
    split at he
    · next hcol =>
      cases hL : getBoundLB P entries ⟨col, hcol⟩ lbRef with
      | none => simp [hL] at he
      | some pL =>
        rcases pL with ⟨l, asmsL⟩
        cases hU : getBoundUB P entries ⟨col, hcol⟩ ubRef with
        | none => simp [hL, hU] at he
        | some pU =>
          rcases pU with ⟨u, asmsU⟩
          simp only [hL, hU] at he
          split at he
          · next hlt =>
            injection he with hec; subst hec
            have hLB := getBoundLB_sound ht ⟨col, hcol⟩ lbRef l asmsL hL
            have hUB := getBoundUB_sound ht ⟨col, hcol⟩ ubRef u asmsU hU
            have hsub : ∀ a ∈ mergeAsms asmsL asmsU, a < entries.size + 1 := by
              intro a ha
              rcases mem_mergeAsms.mp ha with hl' | hu'
              · exact Nat.lt_succ_of_lt (hLB.1 a hl')
              · exact Nat.lt_succ_of_lt (hUB.1 a hu')
            apply validTable_push ht _ hsub
            intro x hx hasm
            have hasm' : ∀ a ∈ mergeAsms asmsL asmsU, ∃ ha : a < entries.size, entries[a].con.sat x := by
              intro a ha
              obtain ⟨_, hs⟩ := hasm a ha
              have halt : a < entries.size := by
                rcases mem_mergeAsms.mp ha with hl' | hu'
                · exact hLB.1 a hl'
                · exact hUB.1 a hu'
              rw [Array.getElem_push_lt halt] at hs
              exact ⟨halt, hs⟩
            have hlx := hLB.2 x hx (asmsSat_mono (fun a ha => mem_mergeAsms.mpr (Or.inl ha)) hasm')
            have hux := hUB.2 x hx (asmsSat_mono (fun a ha => mem_mergeAsms.mpr (Or.inr ha)) hasm')
            unfold LinCon.sat
            rw [RatVec.dot_zero]
            have hlu : l ≤ u := Rat.le_trans hlx hux
            exact absurd hlu (Rat.not_le.mpr hlt)
          · contradiction
    · contradiction
  | dualBound refs bounds =>
    simp only [evalRule] at he
    cases hlin : evalLinComb P entries .ge refs bounds with
    | none => simp [hlin] at he
    | some e0 =>
      simp only [hlin] at he
      split at he
      · injection he with hec; subst hec
        have hv := evalLinComb_sound ht hlin
        exact validTable_push ht _ (fun a ha => Nat.lt_succ_of_lt (hv.1 a ha))
          (fun x hx hasm => hv.2 x hx (fun a ha => by
            obtain ⟨_, hs⟩ := hasm a ha
            have halt := hv.1 a ha
            rw [Array.getElem_push_lt halt] at hs
            exact ⟨halt, hs⟩))
      · contradiction
  | safeDual refs boundAsms =>
    simp only [evalRule, evalSafeDual] at he
    cases hlin : evalLinComb P entries .ge refs #[] with
    | none => simp [hlin] at he
    | some rowEntry =>
      simp only [hlin] at he
      cases hcols : evalSafeDual.goCols P entries boundAsms 0
          rowEntry.con.coeffs rowEntry.con.rhs rowEntry.assumptions with
      | none => simp [hcols] at he
      | some pOut =>
        rcases pOut with ⟨totalRhs, finalAsms⟩
        simp only [hcols] at he
        injection he with hec; subst hec
        have hrow := evalLinComb_sound ht hlin
        have hcolSound := goCols_sound P entries ht boundAsms rowEntry.con.coeffs
          0 (Nat.zero_le P.n) rowEntry.con.rhs rowEntry.assumptions totalRhs finalAsms
          hrow.1 (fun x hx hasm => by
            have hsat := hrow.2 x hx hasm
            unfold LinCon.sat at hsat
            rw [evalLinComb_sense hlin] at hsat
            simpa [finSum_zero_n, Rat.add_zero] using hsat) hcols
        apply validTable_push ht _ (fun a ha => Nat.lt_succ_of_lt (hcolSound.1 a ha))
        intro x hx hasm
        have hsat := hcolSound.2 x hx (fun a ha => by
          obtain ⟨_, hs⟩ := hasm a ha
          have halt := hcolSound.1 a ha
          rw [Array.getElem_push_lt halt] at hs
          exact ⟨halt, hs⟩)
        unfold LinCon.sat
        dsimp only
        unfold RatVec.dot
        simpa using hsat

private theorem evalProofStep_valid (P : VIPRInstance) (entries : Array (DerivedEntry P.n))
    (ht : ValidTable P entries) (step : ProofStep P.n) (e : DerivedEntry P.n)
    (he : evalProofStep P entries step = some e) :
    ValidTable P (entries.push e) := by
  unfold evalProofStep at he
  cases hrule : evalRule P entries step.rule with
  | none => simp [hrule] at he
  | some entry =>
    simp only [hrule] at he
    by_cases hcond :
        ((match step.claimedCon with | none => true | some cc => entry.con.beq cc) &&
         (match step.claimedAssumptions with
          | none => true
          | some ca => checkAssumptionsMatch entry.assumptions ca)) = true
    · simp only [hcond, ↓reduceIte] at he
      injection he with hEq; subst hEq
      exact evalRule_valid P entries ht step.rule entry hrule
    · simp only [hcond] at he
      contradiction

private def runSteps (P : VIPRInstance) (steps : Array (ProofStep P.n))
    (i : Nat) (entries : Array (DerivedEntry P.n)) : Option (Array (DerivedEntry P.n)) :=
  if hi : i < steps.size then
    match evalProofStep P entries steps[i] with
    | none => none
    | some e => runSteps P steps (i + 1) (entries.push e)
  else
    some entries
termination_by steps.size - i

private theorem runSteps_sound (P : VIPRInstance) (steps : Array (ProofStep P.n)) :
    ∀ k (entries0 entriesOut : Array (DerivedEntry P.n)),
      ValidTable P entries0 →
      runSteps P steps k entries0 = some entriesOut →
      entriesOut.size = entries0.size + (steps.size - k) ∧ ValidTable P entriesOut := by
  intro k entries0 entriesOut ht hrun
  unfold runSteps at hrun
  split at hrun
  · next hi =>
    cases hstep : evalProofStep P entries0 steps[k] with
    | none => simp [hstep] at hrun
    | some e =>
      simp only [hstep] at hrun
      have ht' := evalProofStep_valid P entries0 ht steps[k] e hstep
      obtain ⟨hsz, htout⟩ := runSteps_sound P steps (k + 1) (entries0.push e) entriesOut ht' hrun
      rw [Array.size_push] at hsz
      refine ⟨by omega, htout⟩
  · next hge =>
    injection hrun with heq; subst heq
    have h0 : steps.size - k = 0 := by omega
    exact ⟨by omega, ht⟩
termination_by k _ _ _ => steps.size - k

/-- Executable Mini-VIPR rational certificate checker. -/
def checkMiniVIPR (cert : MiniVIPRCert) : Bool :=
  let init := cert.inst.baseRows.map fun r => { con := r, assumptions := [] }
  match runSteps cert.inst cert.steps 0 init with
  | none => false
  | some entries =>
    if hroot : cert.rootStep < cert.steps.size then
      let rootIdx := cert.inst.baseRows.size + cert.rootStep
      if hidx : rootIdx < entries.size then
        let rootEntry := entries[rootIdx]
        if !rootEntry.assumptions.isEmpty then
          false
        else
          match cert.mode with
          | .infeasible => isUnsatCon rootEntry.con
          | .optimal xStar =>
            checkIncumbentFeasible cert.inst xStar &&
            (isUnsatCon rootEntry.con ||
             (rootEntry.con.coeffs.beq (RatVec.ofFn cert.inst.obj) &&
              (rootEntry.con.sense == .ge || rootEntry.con.sense == .eq) &&
              decide ((RatVec.ofFn cert.inst.obj).dotVec xStar ≤ rootEntry.con.rhs)))
          | .bound targetLB =>
            let okInc :=
              match cert.incumbent? with
              | none => true
              | some xInc => checkIncumbentFeasible cert.inst xInc
            okInc &&
            (isUnsatCon rootEntry.con ||
             (rootEntry.con.coeffs.beq (RatVec.ofFn cert.inst.obj) &&
              (rootEntry.con.sense == .ge || rootEntry.con.sense == .eq) &&
              decide (targetLB ≤ cert.inst.objConst + rootEntry.con.rhs)))
      else false
    else false

/-- Top-level soundness theorem for `checkMiniVIPR` covering `infeasible`, `optimal`, and `bound` modes. -/
theorem checkMiniVIPR_sound (cert : MiniVIPRCert) (h : checkMiniVIPR cert = true) :
    match cert.mode with
    | .infeasible => ¬ ∃ x : Fin cert.inst.n → Rat, MILPFeasible cert.inst x
    | .optimal xStar =>
        MILPFeasible cert.inst xStar.get ∧
        ∀ x : Fin cert.inst.n → Rat, MILPFeasible cert.inst x →
          LPObjective cert.inst xStar.get ≤ LPObjective cert.inst x
    | .bound targetLB =>
        (∀ xInc, cert.incumbent? = some xInc → MILPFeasible cert.inst xInc.get) ∧
        ∀ x : Fin cert.inst.n → Rat, MILPFeasible cert.inst x →
          targetLB ≤ LPObjective cert.inst x := by
  unfold checkMiniVIPR at h
  dsimp only at h
  cases hrun : runSteps cert.inst cert.steps 0
      (cert.inst.baseRows.map fun r => { con := r, assumptions := [] }) with
  | none => simp [hrun] at h
  | some entries =>
    simp only [hrun] at h
    split at h
    · next hroot =>
      split at h
      · next hidx =>
        split at h
        · contradiction
        · next hasm =>
          have hemp : entries[cert.inst.baseRows.size + cert.rootStep].assumptions = [] := by
            cases hlist : entries[cert.inst.baseRows.size + cert.rootStep].assumptions with
            | nil => rfl
            | cons _ _ => simp [hlist] at hasm
          obtain ⟨_, htable⟩ := runSteps_sound cert.inst cert.steps 0 _ entries
            (validTable_init cert.inst) hrun
          have hEnt := htable (cert.inst.baseRows.size + cert.rootStep) hidx
          have hrootSat : ∀ x, MILPFeasible cert.inst x →
              entries[cert.inst.baseRows.size + cert.rootStep].con.sat x := by
            intro x hx
            apply hEnt.2 x hx
            intro a ha
            rw [hemp] at ha
            simp at ha
          cases hmode : cert.mode with
          | infeasible =>
            rw [hmode] at h
            rintro ⟨x, hx⟩
            exact absurd (hrootSat x hx) (isUnsatCon_sound h x)
          | optimal xStar =>
            rw [hmode] at h
            simp only [Bool.and_eq_true] at h
            refine ⟨checkIncumbentFeasible_sound cert.inst xStar h.1, fun x hx => ?_⟩
            have hsat := hrootSat x hx
            rcases Bool.or_eq_true _ _ |>.mp h.2 with hunsat | hbound
            · exact absurd hsat (isUnsatCon_sound hunsat x)
            · simp only [Bool.and_eq_true] at hbound
              rcases hbound with ⟨⟨hcoeffs, hsense⟩, hrhs⟩
              have hcEq : entries[cert.inst.baseRows.size + cert.rootStep].con.coeffs =
                  RatVec.ofFn cert.inst.obj := RatVec.eq_of_beq hcoeffs
              have hge : entries[cert.inst.baseRows.size + cert.rootStep].con.rhs ≤
                  entries[cert.inst.baseRows.size + cert.rootStep].con.coeffs.dot x := by
                unfold LinCon.sat at hsat
                rcases Bool.or_eq_true _ _ |>.mp hsense with hs | hs
                · rw [beq_iff_eq.mp hs] at hsat; exact hsat
                · rw [beq_iff_eq.mp hs] at hsat; exact hsat ▸ Rat.le_refl
              rw [hcEq] at hge
              have hdot : (RatVec.ofFn cert.inst.obj).dot x =
                  finSum cert.inst.n (fun j => cert.inst.obj j * x j) := by
                unfold RatVec.dot
                apply finSum_congr
                intro j
                simp
              have hdotStar : (RatVec.ofFn cert.inst.obj).dotVec xStar =
                  finSum cert.inst.n (fun j => cert.inst.obj j * xStar.get j) := by
                rw [RatVec.dotVec_eq_dot]
                unfold RatVec.dot
                apply finSum_congr
                intro j
                simp
              have hle_rhs : finSum cert.inst.n (fun j => cert.inst.obj j * xStar.get j) ≤
                  entries[cert.inst.baseRows.size + cert.rootStep].con.rhs := by
                rw [← hdotStar]
                exact decide_eq_true_eq.mp hrhs
              have hle_x : finSum cert.inst.n (fun j => cert.inst.obj j * xStar.get j) ≤
                  finSum cert.inst.n (fun j => cert.inst.obj j * x j) :=
                Rat.le_trans hle_rhs (by rwa [← hdot])
              unfold LPObjective
              exact Rat.add_le_add_left.mpr hle_x
          | bound targetLB =>
            rw [hmode] at h
            simp only [Bool.and_eq_true] at h
            refine ⟨?_, fun x hx => ?_⟩
            · intro xInc hxInc
              have hinc := h.1
              rw [hxInc] at hinc
              exact checkIncumbentFeasible_sound cert.inst xInc hinc
            · have hsat := hrootSat x hx
              rcases Bool.or_eq_true _ _ |>.mp h.2 with hunsat | hbound
              · exact absurd hsat (isUnsatCon_sound hunsat x)
              · simp only [Bool.and_eq_true] at hbound
                rcases hbound with ⟨⟨hcoeffs, hsense⟩, hrhs⟩
                have hcEq : entries[cert.inst.baseRows.size + cert.rootStep].con.coeffs =
                    RatVec.ofFn cert.inst.obj := RatVec.eq_of_beq hcoeffs
                have hge : entries[cert.inst.baseRows.size + cert.rootStep].con.rhs ≤
                    entries[cert.inst.baseRows.size + cert.rootStep].con.coeffs.dot x := by
                  unfold LinCon.sat at hsat
                  rcases Bool.or_eq_true _ _ |>.mp hsense with hs | hs
                  · rw [beq_iff_eq.mp hs] at hsat; exact hsat
                  · rw [beq_iff_eq.mp hs] at hsat; exact hsat ▸ Rat.le_refl
                rw [hcEq] at hge
                have hdot : (RatVec.ofFn cert.inst.obj).dot x =
                    finSum cert.inst.n (fun j => cert.inst.obj j * x j) := by
                  unfold RatVec.dot
                  apply finSum_congr
                  intro j
                  simp
                rw [hdot] at hge
                have htarget : targetLB ≤
                    cert.inst.objConst + entries[cert.inst.baseRows.size + cert.rootStep].con.rhs :=
                  decide_eq_true_eq.mp hrhs
                unfold LPObjective
                exact Rat.le_trans htarget (Rat.add_le_add_left.mpr hge)
      · contradiction
    · contradiction

theorem checkMiniVIPR_infeasible_sound (cert : MiniVIPRCert)
    (hmode : cert.mode = .infeasible) (h : checkMiniVIPR cert = true) :
    ¬ ∃ x : Fin cert.inst.n → Rat, MILPFeasible cert.inst x := by
  have hs := checkMiniVIPR_sound cert h
  rwa [hmode] at hs

theorem checkMiniVIPR_optimal_sound (cert : MiniVIPRCert) (xStar : RatVec cert.inst.n)
    (hmode : cert.mode = .optimal xStar) (h : checkMiniVIPR cert = true) :
    MILPFeasible cert.inst xStar.get ∧
    ∀ x : Fin cert.inst.n → Rat, MILPFeasible cert.inst x →
      LPObjective cert.inst xStar.get ≤ LPObjective cert.inst x := by
  have hs := checkMiniVIPR_sound cert h
  rwa [hmode] at hs

theorem checkMiniVIPR_bound_sound (cert : MiniVIPRCert) {targetLB : Rat}
    (h : checkMiniVIPR cert = true) (hmode : cert.mode = .bound targetLB) :
    ∀ x : Fin cert.prob.n → Rat, MILPFeasible cert.prob x →
      targetLB ≤ LPObjective cert.prob x := by
  have hs := checkMiniVIPR_sound cert h
  rw [hmode] at hs
  exact hs.2


/-! ## Section 4: Verified Presolve Envelope (`orig -> reduced`) -/

private theorem rat_floor_le (r : Rat) : (r.floor : Rat) ≤ r :=
  Rat.le_floor_iff.mp (Int.le_refl r.floor)

private theorem rat_le_ceil (r : Rat) : r ≤ (r.ceil : Rat) :=
  Rat.ceil_le_iff.mp (Int.le_refl r.ceil)

private theorem rat_mul_le_mul_of_nonpos_left {a b c : Rat} (hab : a ≤ b) (hc : c ≤ 0) :
    c * b ≤ c * a := by
  have hneg : 0 ≤ -c := Rat.le_neg_iff.mp (by simpa using hc)
  have hmul := Rat.mul_le_mul_of_nonneg_left hab hneg
  rw [Rat.neg_mul, Rat.neg_mul, Rat.neg_le_neg_iff] at hmul
  exact hmul

private theorem rat_div_le_div_of_pos {a b c : Rat} (hab : a ≤ b) (hc : 0 < c) :
    a / c ≤ b / c := by
  rw [Rat.div_def, Rat.div_def]
  exact Rat.mul_le_mul_of_nonneg_right hab (Rat.le_of_lt (Rat.inv_pos.mpr hc))

private theorem rat_div_le_div_of_neg {a b c : Rat} (hab : a ≤ b) (hc : c < 0) :
    b / c ≤ a / c := by
  have hpos : 0 < -c := by
    rw [← Rat.neg_zero] at hc
    exact Rat.neg_lt_neg_iff.mpr hc
  have hne : c ≠ 0 := Rat.ne_of_lt hc
  have hmul : (b / c) * (-c) ≤ (a / c) * (-c) := by
    rw [Rat.mul_neg, Rat.mul_neg, Rat.div_mul_cancel hne, Rat.div_mul_cancel hne, Rat.neg_le_neg_iff]
    exact hab
  exact Rat.le_of_mul_le_mul_right hmul hpos

private theorem rat_mul_div_cancel_left {a b : Rat} (ha : a ≠ 0) : a * (b / a) = b := by
  rw [Rat.mul_comm]
  exact Rat.div_mul_cancel ha

private theorem rat_div_mul_eq {a b : Rat} (hb : b ≠ 0) : (a * b) / b = a := by
  rw [Rat.div_def, Rat.mul_assoc, Rat.mul_inv_cancel b hb, Rat.mul_one]

private theorem rat_binary_zero_or_one {v : Rat} (h0 : 0 ≤ v) (h1 : v ≤ 1) (hint : isRatInt v) :
    v = 0 ∨ v = 1 := by
  rcases hint with ⟨z, rfl⟩
  have hz0 : (0 : Int) ≤ z := Rat.intCast_le_intCast.mp h0
  have hz1 : z ≤ (1 : Int) := Rat.intCast_le_intCast.mp h1
  have hcases : z = 0 ∨ z = 1 := by omega
  rcases hcases with rfl | rfl
  · left; rfl
  · right; rfl

/-- Intermediate MILP state during exact rational presolve over `n` variables. -/
structure PresolveState (n : Nat) where
  objConst : Rat
  obj : RatVec n
  isInt : Array Bool
  isInt_size : isInt.size = n
  lb : Array (Option Rat)
  lb_size : lb.size = n
  ub : Array (Option Rat)
  ub_size : ub.size = n
  rows : Array (Option (LinCon n))

@[inline]
def PresolveState.isIntAt {n : Nat} (st : PresolveState n) (j : Fin n) : Bool :=
  st.isInt[j.1]'(by rw [st.isInt_size]; exact j.2)

@[inline]
def PresolveState.lbAt {n : Nat} (st : PresolveState n) (j : Fin n) : Option Rat :=
  st.lb[j.1]'(by rw [st.lb_size]; exact j.2)

@[inline]
def PresolveState.ubAt {n : Nat} (st : PresolveState n) (j : Fin n) : Option Rat :=
  st.ub[j.1]'(by rw [st.ub_size]; exact j.2)

/-- Compacts active `some` rows in index order. -/
def compactOptRows {n : Nat} (rows : Array (Option (LinCon n))) : Array (LinCon n) :=
  go 0 #[]
where
  go (i : Nat) (acc : Array (LinCon n)) : Array (LinCon n) :=
    if hi : i < rows.size then
      match rows[i] with
      | none => go (i + 1) acc
      | some r => go (i + 1) (acc.push r)
    else acc
  termination_by rows.size - i

private theorem compactOptRows_go_iff {n : Nat} (rows : Array (Option (LinCon n)))
    (x : Fin n → Rat) :
    ∀ k (_hk : k ≤ rows.size) (acc out : Array (LinCon n)),
      compactOptRows.go rows k acc = out →
      ((∀ i : Fin out.size, out[i].sat x) ↔
       ((∀ i : Fin acc.size, acc[i].sat x) ∧
        (∀ i (hi : i < rows.size), k ≤ i → ∀ r, rows[i] = some r → r.sat x))) := by
  intro k hk acc out hout
  unfold compactOptRows.go at hout
  split at hout
  · next hlt =>
    cases hcur : rows[k] with
    | none =>
      simp only [hcur] at hout
      rw [compactOptRows_go_iff rows x (k + 1) hlt acc out hout]
      constructor
      · rintro ⟨hacc, htail⟩
        refine ⟨hacc, fun i hi hki r hr => ?_⟩
        by_cases hik : i = k
        · subst hik; rw [hcur] at hr; contradiction
        · exact htail i hi (by omega) r hr
      · rintro ⟨hacc, hall⟩
        exact ⟨hacc, fun i hi hki r hr => hall i hi (by omega) r hr⟩
    | some rk =>
      simp only [hcur] at hout
      rw [compactOptRows_go_iff rows x (k + 1) hlt (acc.push rk) out hout]
      constructor
      · rintro ⟨hacc, htail⟩
        refine ⟨fun i => ?_, fun i hi hki r hr => ?_⟩
        · have h := hacc ⟨i.1, by rw [Array.size_push]; exact Nat.lt_succ_of_lt i.2⟩
          simpa [Array.getElem_push_lt i.2] using h
        · by_cases hik : i = k
          · subst hik
            rw [hcur] at hr; injection hr with hrEq; subst r
            have h := hacc ⟨acc.size, by rw [Array.size_push]; exact Nat.lt_succ_self _⟩
            simpa [Array.getElem_push_eq] using h
          · exact htail i hi (by omega) r hr
      · rintro ⟨hacc, hall⟩
        refine ⟨fun i => ?_, fun i hi hki r hr => hall i hi (by omega) r hr⟩
        have hisz : i.1 < acc.size + 1 := by simpa [Array.size_push] using i.2
        by_cases hilt : i.1 < acc.size
        · simpa [Array.getElem_push_lt hilt] using hacc ⟨i.1, hilt⟩
        · have heq : i.1 = acc.size := by omega
          have hget : (acc.push rk)[i] = rk := by
            change (acc.push rk)[i.1] = rk
            simp only [heq, Array.getElem_push_eq]
          rw [hget]
          exact hall k hlt (Nat.le_refl k) rk hcur
  · next hge =>
    subst hout
    have hkn : k = rows.size := by omega
    subst hkn
    constructor
    · intro hacc; exact ⟨hacc, fun i hi hle _ _ => by omega⟩
    · rintro ⟨hacc, _⟩; exact hacc
termination_by k _ _ _ _ => rows.size - k

theorem compactOptRows_sat_iff {n : Nat} (rows : Array (Option (LinCon n))) (x : Fin n → Rat) :
    (∀ i : Fin (compactOptRows rows).size, (compactOptRows rows)[i].sat x) ↔
    (∀ i : Fin rows.size, ∀ r, rows[i] = some r → r.sat x) := by
  unfold compactOptRows
  rw [compactOptRows_go_iff rows x 0 (Nat.zero_le rows.size) #[] _ rfl]
  constructor
  · rintro ⟨_, hall⟩ i r hr
    exact hall i.1 i.2 (Nat.zero_le i.1) r hr
  · intro hall
    refine ⟨fun i => Fin.elim0 (by simpa using i), fun i hi _ r hr => hall ⟨i, hi⟩ r hr⟩

/-- Constructs an initial `PresolveState` from a `VIPRInstance`. -/
def PresolveState.ofInstance (P : VIPRInstance) : PresolveState P.n :=
  { objConst := P.objConst
    obj := RatVec.ofFn P.obj
    isInt := Array.ofFn P.isInt
    isInt_size := Array.size_ofFn
    lb := Array.ofFn P.lb
    lb_size := Array.size_ofFn
    ub := Array.ofFn P.ub
    ub_size := Array.size_ofFn
    rows := P.baseRows.map some }

/-- Converts a `PresolveState n` into a compacted `VIPRInstance`. -/
def PresolveState.toInstance {n : Nat} (st : PresolveState n) : VIPRInstance :=
  { n := n
    objConst := st.objConst
    obj := fun j => st.obj.get j
    isInt := fun j => st.isIntAt j
    baseRows := compactOptRows st.rows
    lb := fun j => st.lbAt j
    ub := fun j => st.ubAt j }

/-- Mixed-integer feasibility predicate for a `PresolveState n`. -/
def StateFeasible {n : Nat} (st : PresolveState n) (x : Fin n → Rat) : Prop :=
  inVarBounds st.lbAt st.ubAt x ∧
  (∀ i : Fin st.rows.size, ∀ r, st.rows[i] = some r → r.sat x) ∧
  (∀ j : Fin n, st.isIntAt j = true → isRatInt (x j))

/-- Shifted minimization objective for a `PresolveState n`. -/
def StateObjective {n : Nat} (st : PresolveState n) (x : Fin n → Rat) : Rat :=
  st.objConst + finSum n (fun j => st.obj.get j * x j)

theorem stateFeasible_ofInstance (P : VIPRInstance) (x : Fin P.n → Rat) :
    StateFeasible (PresolveState.ofInstance P) x ↔ MILPFeasible P x := by
  unfold StateFeasible MILPFeasible PresolveState.ofInstance PresolveState.lbAt
    PresolveState.ubAt PresolveState.isIntAt inVarBounds
  simp only [Array.getElem_ofFn]
  constructor
  · rintro ⟨hb, hr, hi⟩
    refine ⟨hb, fun i => hr ⟨i.1, by rw [Array.size_map]; exact i.2⟩ P.baseRows[i] (by simp), hi⟩
  · rintro ⟨hb, hr, hi⟩
    refine ⟨hb, fun i r hsome => ?_, hi⟩
    have hget : (Array.map some P.baseRows)[i] = some (P.baseRows[i.1]'(by simpa [Array.size_map] using i.2)) := by
      change (Array.map some P.baseRows)[i.1] = _
      simp
    rw [hget] at hsome
    injection hsome with hEq; subst hEq
    exact hr ⟨i.1, by simpa [Array.size_map] using i.2⟩

theorem stateObjective_ofInstance (P : VIPRInstance) (x : Fin P.n → Rat) :
    StateObjective (PresolveState.ofInstance P) x = LPObjective P x := by
  unfold StateObjective LPObjective PresolveState.ofInstance
  simp only [RatVec.get_ofFn]

theorem stateFeasible_toInstance {n : Nat} (st : PresolveState n) (x : Fin n → Rat) :
    StateFeasible st x ↔ MILPFeasible st.toInstance x := by
  unfold StateFeasible MILPFeasible PresolveState.toInstance
  dsimp only
  rw [compactOptRows_sat_iff]

theorem stateObjective_toInstance {n : Nat} (st : PresolveState n) (x : Fin n → Rat) :
    StateObjective st x = LPObjective st.toInstance x := rfl

/-! ### Exact Activity Bounds on `PresolveState` -/

/-- Minimum contribution of `a * x` over `x ∈ [lb, ub]`. -/
def termMinAct (a : Rat) (lb ub : Option Rat) : Option Rat :=
  if a == 0 then some 0
  else if 0 < a then lb.map (a * ·)
  else ub.map (a * ·)

/-- Maximum contribution of `a * x` over `x ∈ [lb, ub]`. -/
def termMaxAct (a : Rat) (lb ub : Option Rat) : Option Rat :=
  if a == 0 then some 0
  else if 0 < a then ub.map (a * ·)
  else lb.map (a * ·)

private theorem termMinAct_le {a m x : Rat} {lb ub : Option Rat}
    (hlb : ∀ l, lb = some l → l ≤ x) (hub : ∀ u, ub = some u → x ≤ u)
    (hm : termMinAct a lb ub = some m) : m ≤ a * x := by
  unfold termMinAct at hm
  split at hm
  · next h0 =>
    injection hm with heq; subst heq
    rw [beq_iff_eq.mp h0, Rat.zero_mul]; exact Rat.le_refl
  · next hne =>
    split at hm
    · next hpos =>
      cases h1 : lb with
      | none => simp [h1] at hm
      | some l =>
        simp only [h1, Option.map_some] at hm
        injection hm with heq; subst heq
        exact Rat.mul_le_mul_of_nonneg_left (hlb l h1) (Rat.le_of_lt hpos)
    · next hnotpos =>
      have hle0 : a ≤ 0 := Rat.not_lt.mp hnotpos
      cases h2 : ub with
      | none => simp [h2] at hm
      | some u =>
        simp only [h2, Option.map_some] at hm
        injection hm with heq; subst heq
        exact rat_mul_le_mul_of_nonpos_left (hub u h2) hle0

private theorem termMaxAct_ge {a M x : Rat} {lb ub : Option Rat}
    (hlb : ∀ l, lb = some l → l ≤ x) (hub : ∀ u, ub = some u → x ≤ u)
    (hM : termMaxAct a lb ub = some M) : a * x ≤ M := by
  unfold termMaxAct at hM
  split at hM
  · next h0 =>
    injection hM with heq; subst heq
    rw [beq_iff_eq.mp h0, Rat.zero_mul]; exact Rat.le_refl
  · next hne =>
    split at hM
    · next hpos =>
      cases h2 : ub with
      | none => simp [h2] at hM
      | some u =>
        simp only [h2, Option.map_some] at hM
        injection hM with heq; subst heq
        exact Rat.mul_le_mul_of_nonneg_left (hub u h2) (Rat.le_of_lt hpos)
    · next hnotpos =>
      have hle0 : a ≤ 0 := Rat.not_lt.mp hnotpos
      cases h1 : lb with
      | none => simp [h1] at hM
      | some l =>
        simp only [h1, Option.map_some] at hM
        injection hM with heq; subst heq
        exact rat_mul_le_mul_of_nonpos_left (hlb l h1) hle0

/-- Sums `termMinAct` over all columns except an optional `skip` column. -/
def sumMinAct {n : Nat} (st : PresolveState n) (coeffs : RatVec n) (skip : Option (Fin n)) :
    Option Rat :=
  go 0 0
where
  go (i : Nat) (acc : Rat) : Option Rat :=
    if hi : i < n then
      let j : Fin n := ⟨i, hi⟩
      if skip == some j then
        go (i + 1) acc
      else
        match termMinAct (coeffs.get j) (st.lbAt j) (st.ubAt j) with
        | none => none
        | some t => go (i + 1) (acc + t)
    else some acc
  termination_by n - i

/-- Sums `termMaxAct` over all columns except an optional `skip` column. -/
def sumMaxAct {n : Nat} (st : PresolveState n) (coeffs : RatVec n) (skip : Option (Fin n)) :
    Option Rat :=
  go 0 0
where
  go (i : Nat) (acc : Rat) : Option Rat :=
    if hi : i < n then
      let j : Fin n := ⟨i, hi⟩
      if skip == some j then
        go (i + 1) acc
      else
        match termMaxAct (coeffs.get j) (st.lbAt j) (st.ubAt j) with
        | none => none
        | some t => go (i + 1) (acc + t)
    else some acc
  termination_by n - i

private theorem sumMinAct_go_le {n : Nat} (st : PresolveState n) (coeffs : RatVec n)
    (skip : Option (Fin n)) (x : Fin n → Rat) (hb : inVarBounds st.lbAt st.ubAt x) :
    ∀ k (hk : k ≤ n) (acc out : Rat),
      acc ≤ finSum k (fun i =>
        let j : Fin n := ⟨i.1, Nat.lt_of_lt_of_le i.2 hk⟩
        if skip = some j then 0 else coeffs.get j * x j) →
      sumMinAct.go st coeffs skip k acc = some out →
      out ≤ finSum n (fun j => if skip = some j then 0 else coeffs.get j * x j) := by
  intro k hk acc out hacc hgo
  unfold sumMinAct.go at hgo
  dsimp only at hgo
  split at hgo
  · next hlt =>
    let jk : Fin n := ⟨k, hlt⟩
    have hsplit :
        finSum (k + 1) (fun i =>
          let ji : Fin n := ⟨i.1, Nat.lt_of_lt_of_le i.2 hlt⟩
          if skip = some ji then 0 else coeffs.get ji * x ji) =
        finSum k (fun i =>
          let ji : Fin n := ⟨i.1, Nat.lt_of_lt_of_le i.2 hk⟩
          if skip = some ji then 0 else coeffs.get ji * x ji) +
        (if skip = some jk then 0 else coeffs.get jk * x jk) := by
      simp [finSum_succ, jk]
    by_cases hskip : skip = some jk
    · have hsb : (skip == some ⟨k, hlt⟩) = true := beq_iff_eq.mpr hskip
      simp only [hsb, ↓reduceIte] at hgo
      have hacc' : acc ≤ finSum (k + 1) (fun i =>
          let ji : Fin n := ⟨i.1, Nat.lt_of_lt_of_le i.2 hlt⟩
          if skip = some ji then 0 else coeffs.get ji * x ji) := by
        rw [hsplit]
        simpa [hskip, Rat.add_zero] using hacc
      exact sumMinAct_go_le st coeffs skip x hb (k + 1) hlt acc out hacc' hgo
    · have hsb : (skip == some ⟨k, hlt⟩) = false := beq_false_of_ne hskip
      simp only [hsb, Bool.false_eq_true, ↓reduceIte] at hgo
      cases ht : termMinAct (coeffs.get ⟨k, hlt⟩) (st.lbAt ⟨k, hlt⟩) (st.ubAt ⟨k, hlt⟩) with
      | none => simp [ht] at hgo
      | some t =>
        simp only [ht] at hgo
        have htle : t ≤ coeffs.get jk * x jk := termMinAct_le (hb jk).1 (hb jk).2 ht
        have hacc' : acc + t ≤ finSum (k + 1) (fun i =>
            let ji : Fin n := ⟨i.1, Nat.lt_of_lt_of_le i.2 hlt⟩
            if skip = some ji then 0 else coeffs.get ji * x ji) := by
          rw [hsplit]
          simp only [hskip, ↓reduceIte]
          exact Rat.le_trans (Rat.add_le_add_right.mpr hacc) (Rat.add_le_add_left.mpr htle)
        exact sumMinAct_go_le st coeffs skip x hb (k + 1) hlt (acc + t) out hacc' hgo
  · next hge =>
    have hkn : k = n := by omega
    subst hkn
    injection hgo with heq; subst heq
    exact hacc
termination_by k _ _ _ _ => n - k

private theorem sumMaxAct_go_ge {n : Nat} (st : PresolveState n) (coeffs : RatVec n)
    (skip : Option (Fin n)) (x : Fin n → Rat) (hb : inVarBounds st.lbAt st.ubAt x) :
    ∀ k (hk : k ≤ n) (acc out : Rat),
      finSum k (fun i =>
        let j : Fin n := ⟨i.1, Nat.lt_of_lt_of_le i.2 hk⟩
        if skip = some j then 0 else coeffs.get j * x j) ≤ acc →
      sumMaxAct.go st coeffs skip k acc = some out →
      finSum n (fun j => if skip = some j then 0 else coeffs.get j * x j) ≤ out := by
  intro k hk acc out hacc hgo
  unfold sumMaxAct.go at hgo
  dsimp only at hgo
  split at hgo
  · next hlt =>
    let jk : Fin n := ⟨k, hlt⟩
    have hsplit :
        finSum (k + 1) (fun i =>
          let ji : Fin n := ⟨i.1, Nat.lt_of_lt_of_le i.2 hlt⟩
          if skip = some ji then 0 else coeffs.get ji * x ji) =
        finSum k (fun i =>
          let ji : Fin n := ⟨i.1, Nat.lt_of_lt_of_le i.2 hk⟩
          if skip = some ji then 0 else coeffs.get ji * x ji) +
        (if skip = some jk then 0 else coeffs.get jk * x jk) := by
      simp [finSum_succ, jk]
    by_cases hskip : skip = some jk
    · have hsb : (skip == some ⟨k, hlt⟩) = true := beq_iff_eq.mpr hskip
      simp only [hsb, ↓reduceIte] at hgo
      have hacc' : finSum (k + 1) (fun i =>
          let ji : Fin n := ⟨i.1, Nat.lt_of_lt_of_le i.2 hlt⟩
          if skip = some ji then 0 else coeffs.get ji * x ji) ≤ acc := by
        rw [hsplit]
        simpa [hskip, Rat.add_zero] using hacc
      exact sumMaxAct_go_ge st coeffs skip x hb (k + 1) hlt acc out hacc' hgo
    · have hsb : (skip == some ⟨k, hlt⟩) = false := beq_false_of_ne hskip
      simp only [hsb, Bool.false_eq_true, ↓reduceIte] at hgo
      cases ht : termMaxAct (coeffs.get ⟨k, hlt⟩) (st.lbAt ⟨k, hlt⟩) (st.ubAt ⟨k, hlt⟩) with
      | none => simp [ht] at hgo
      | some t =>
        simp only [ht] at hgo
        have htge : coeffs.get jk * x jk ≤ t := termMaxAct_ge (hb jk).1 (hb jk).2 ht
        have hacc' : finSum (k + 1) (fun i =>
            let ji : Fin n := ⟨i.1, Nat.lt_of_lt_of_le i.2 hlt⟩
            if skip = some ji then 0 else coeffs.get ji * x ji) ≤ acc + t := by
          rw [hsplit]
          simp only [hskip, ↓reduceIte]
          exact Rat.le_trans (Rat.add_le_add_right.mpr hacc) (Rat.add_le_add_left.mpr htge)
        exact sumMaxAct_go_ge st coeffs skip x hb (k + 1) hlt (acc + t) out hacc' hgo
  · next hge =>
    have hkn : k = n := by omega
    subst hkn
    injection hgo with heq; subst heq
    exact hacc
termination_by k _ _ _ _ => n - k

private theorem dot_split_col {n : Nat} (coeffs : RatVec n) (c : Fin n) (x : Fin n → Rat) :
    coeffs.dot x =
    finSum n (fun j => if some c = some j then 0 else coeffs.get j * x j) + coeffs.get c * x c := by
  unfold RatVec.dot
  have h1 : ∀ j : Fin n, coeffs.get j * x j =
      (if some c = some j then 0 else coeffs.get j * x j) +
      (if j = c then coeffs.get c * x c else 0) := by
    intro j
    by_cases hjc : j = c
    · subst hjc; simp [Rat.zero_add]
    · have hne : some c ≠ some j := fun heq => hjc (Option.some.inj heq).symm
      simp [hjc, hne, Rat.add_zero]
  rw [finSum_congr h1, finSum_add, finSum_single]

private theorem sumMinAct_none_le {n : Nat} {st : PresolveState n} {coeffs : RatVec n} {m : Rat}
    {x : Fin n → Rat} (hb : inVarBounds st.lbAt st.ubAt x)
    (hm : sumMinAct st coeffs none = some m) : m ≤ coeffs.dot x := by
  have h := sumMinAct_go_le st coeffs none x hb 0 (Nat.zero_le n) 0 m Rat.le_refl hm
  have heq : finSum n (fun j => if (none : Option (Fin n)) = some j then 0 else coeffs.get j * x j) =
      coeffs.dot x := by
    unfold RatVec.dot
    apply finSum_congr; intro j; simp
  rwa [heq] at h

private theorem sumMaxAct_none_ge {n : Nat} {st : PresolveState n} {coeffs : RatVec n} {M : Rat}
    {x : Fin n → Rat} (hb : inVarBounds st.lbAt st.ubAt x)
    (hM : sumMaxAct st coeffs none = some M) : coeffs.dot x ≤ M := by
  have h := sumMaxAct_go_ge st coeffs none x hb 0 (Nat.zero_le n) 0 M Rat.le_refl hM
  have heq : finSum n (fun j => if (none : Option (Fin n)) = some j then 0 else coeffs.get j * x j) =
      coeffs.dot x := by
    unfold RatVec.dot
    apply finSum_congr; intro j; simp
  rwa [heq] at h

private theorem sumMinAct_some_le {n : Nat} {st : PresolveState n} {coeffs : RatVec n}
    {c : Fin n} {m : Rat} {x : Fin n → Rat} (hb : inVarBounds st.lbAt st.ubAt x)
    (hm : sumMinAct st coeffs (some c) = some m) :
    m + coeffs.get c * x c ≤ coeffs.dot x := by
  have h := sumMinAct_go_le st coeffs (some c) x hb 0 (Nat.zero_le n) 0 m Rat.le_refl hm
  rw [dot_split_col coeffs c x]
  exact Rat.add_le_add_right.mpr h

private theorem sumMaxAct_some_ge {n : Nat} {st : PresolveState n} {coeffs : RatVec n}
    {c : Fin n} {M : Rat} {x : Fin n → Rat} (hb : inVarBounds st.lbAt st.ubAt x)
    (hM : sumMaxAct st coeffs (some c) = some M) :
    coeffs.dot x ≤ M + coeffs.get c * x c := by
  have h := sumMaxAct_go_ge st coeffs (some c) x hb 0 (Nat.zero_le n) 0 M Rat.le_refl hM
  rw [dot_split_col coeffs c x]
  exact Rat.add_le_add_right.mpr h

/-- Exact rational presolve transformation steps mapping `orig -> reduced`. -/
inductive PresolveStep where
  | roundBound (col : Nat) (isUpper : Bool) (newBound : Rat)
  | activityBound (row col : Nat) (isUpper roundInt : Bool) (newBound : Rat)
  | gcdRowDiv (row : Nat) (divisor : Int)
  | substFixedRow (row col : Nat) (val : Rat)
  | substFixedObj (col : Nat) (val : Rat)
  | singletonRowElim (row col : Nat) (roundInt : Bool)
  | redundantRow (row : Nat)
  | binStrengthen (row col : Nat) (delta : Rat)
  | symBreakLe (u v : Nat) (colPerm rowPerm : Array Nat)
  deriving Repr

private def checkOnlyColNonzero {n : Nat} (coeffs : RatVec n) (col : Fin n) : Bool :=
  loop 0
where
  loop (i : Nat) : Bool :=
    if hi : i < n then
      if i == col.1 || coeffs.get ⟨i, hi⟩ == 0 then loop (i + 1) else false
    else true
  termination_by n - i

private theorem checkOnlyColNonzero_loop_sound {n : Nat} (coeffs : RatVec n) (col : Fin n) :
    ∀ k (hk : k ≤ n),
      (∀ i (hi : i < k), i ≠ col.1 → coeffs.get ⟨i, Nat.lt_of_lt_of_le hi hk⟩ = 0) →
      checkOnlyColNonzero.loop coeffs col k = true →
      ∀ j : Fin n, j ≠ col → coeffs.get j = 0 := by
  intro k hk hprev hloop j hj
  unfold checkOnlyColNonzero.loop at hloop
  split at hloop
  · next hlt =>
    split at hloop
    · next hok =>
      have hcur : k ≠ col.1 → coeffs.get ⟨k, hlt⟩ = 0 := by
        intro hne
        rcases Bool.or_eq_true _ _ |>.mp hok with heq | hz
        · exact absurd (beq_iff_eq.mp heq) hne
        · exact beq_iff_eq.mp hz
      have hnext : ∀ i (hi : i < k + 1), i ≠ col.1 →
          coeffs.get ⟨i, Nat.lt_of_lt_of_le hi hlt⟩ = 0 := by
        intro i hi hne
        by_cases hik : i = k
        · subst hik; exact hcur hne
        · exact hprev i (by omega) hne
      exact checkOnlyColNonzero_loop_sound coeffs col (k + 1) hlt hnext hloop j hj
    · contradiction
  · next hge =>
    have hkn : k = n := by omega
    subst hkn
    exact hprev j.1 j.2 (fun hv => hj (Fin.ext hv))
termination_by k _ _ _ => n - k

private theorem dot_singleton {n : Nat} {coeffs : RatVec n} {col : Fin n}
    (hzero : ∀ j : Fin n, j ≠ col → coeffs.get j = 0) (x : Fin n → Rat) :
    coeffs.dot x = coeffs.get col * x col := by
  unfold RatVec.dot
  have h1 : finSum n (fun j => coeffs.get j * x j) =
      finSum n (fun j => if j = col then coeffs.get col * x col else 0) := by
    apply finSum_congr
    intro j
    by_cases h : j = col
    · subst h; simp
    · simp [h, hzero j h, Rat.zero_mul]
  rw [h1, finSum_single]

/-- Checks whether `row` is a fixed-column symmetry-breaking inequality `x_w - x_v ≤ 0` (or `x_v - x_w ≥ 0`)
where `colPerm[w] = w` and `colPerm[v] = u`, which stays satisfied when `x_v < x_u`. -/
private def checkSymChainRow {n : Nat} (row : LinCon n) (v : Fin n) (colPerm : Array Nat) : Bool :=
  loop 0
where
  loop (w : Nat) : Bool :=
    if hw : w < n then
      if hp : w < colPerm.size then
        let jw : Fin n := ⟨w, hw⟩
        if colPerm[w] == w && w != v.1 then
          let leTarget : LinCon n :=
            { coeffs := RatVec.ofFn (fun j => if j = jw then 1 else if j = v then -1 else 0),
              sense := .le, rhs := 0 }
          let geTarget : LinCon n :=
            { coeffs := RatVec.ofFn (fun j => if j = v then 1 else if j = jw then -1 else 0),
              sense := .ge, rhs := 0 }
          if row.beq leTarget || row.beq geTarget then true
          else loop (w + 1)
        else loop (w + 1)
      else false
    else false
  termination_by n - w

private theorem dot_diff_unit {n : Nat} {a b : Fin n} (hne : a ≠ b) (x : Fin n → Rat) :
    (RatVec.ofFn (fun j : Fin n => if j = a then (1 : Rat) else if j = b then -1 else 0)).dot x =
    x a - x b := by
  unfold RatVec.dot
  have h1 :
      finSum n (fun j : Fin n =>
        (RatVec.ofFn (fun k : Fin n => if k = a then (1 : Rat) else if k = b then -1 else 0)).get j * x j) =
      finSum n (fun j : Fin n => (if j = a then x a else 0) + (if j = b then -x b else 0)) := by
    apply finSum_congr
    intro j
    simp only [RatVec.get_ofFn]
    by_cases hja : j = a
    · subst hja
      simp [hne, Rat.one_mul, Rat.add_zero]
    · by_cases hjb : j = b
      · subst hjb
        simp [hja, Rat.neg_mul, Rat.one_mul, Rat.zero_add]
      · simp [hja, hjb, Rat.zero_mul, Rat.add_zero]
  rw [h1, finSum_add, finSum_single, finSum_single, Rat.sub_eq_add_neg]

private theorem checkSymChainRow_loop_sound {n : Nat} (row : LinCon n) (u v : Fin n)
    (colPerm : Array Nat) (hsz : colPerm.size = n)
    (hlt : ∀ j : Fin n, permAt colPerm hsz j < n)
    (hvu : permFn colPerm hsz hlt v = u)
    {x : Fin n → Rat} (hsat : row.sat x) (hvxu : x v ≤ x u) :
    ∀ k,
      checkSymChainRow.loop row v colPerm k = true →
      row.sat (fun j => x (permFn colPerm hsz hlt j)) := by
  intro k hloop
  unfold checkSymChainRow.loop at hloop
  dsimp only at hloop
  split at hloop
  · next hw =>
    have hpw : k < colPerm.size := by rw [hsz]; exact hw
    simp only [hpw, ↓reduceDIte] at hloop
    split at hloop
    · next hfix =>
      simp only [Bool.and_eq_true, beq_iff_eq, bne_iff_ne] at hfix
      let jw : Fin n := ⟨k, hw⟩
      have hne : jw ≠ v := fun heq => hfix.2 (congrArg Fin.val heq)
      have hfw : permFn colPerm hsz hlt jw = jw := Fin.ext hfix.1
      split at hloop
      · next hmatch =>
        rcases Bool.or_eq_true _ _ |>.mp hmatch with hle | hge
        · have hrEq := LinCon.eq_of_beq hle
          rw [hrEq] at hsat ⊢
          unfold LinCon.sat at hsat ⊢
          rw [dot_diff_unit hne] at hsat ⊢
          rw [hfw, hvu]
          have hxw : x jw ≤ x v := by
            have := Rat.add_le_add_right (c := x v) |>.mpr hsat
            simpa [Rat.sub_eq_add_neg, Rat.add_assoc, Rat.neg_add_cancel, Rat.zero_add, Rat.add_zero] using this
          have hxu : x jw ≤ x u := Rat.le_trans hxw hvxu
          rw [Rat.sub_eq_add_neg]
          have := Rat.add_le_add_right (c := -x u) |>.mpr hxu
          simpa [Rat.add_neg_cancel] using this
        · have hrEq := LinCon.eq_of_beq hge
          rw [hrEq] at hsat ⊢
          unfold LinCon.sat at hsat ⊢
          rw [dot_diff_unit (Ne.symm hne)] at hsat ⊢
          rw [hfw, hvu]
          have hxw : x jw ≤ x v := by
            have := Rat.add_le_add_right (c := x jw) |>.mpr hsat
            simpa [Rat.sub_eq_add_neg, Rat.add_assoc, Rat.neg_add_cancel, Rat.zero_add, Rat.add_zero] using this
          have hxu : x jw ≤ x u := Rat.le_trans hxw hvxu
          rw [Rat.sub_eq_add_neg]
          have := Rat.add_le_add_right (c := -x jw) |>.mpr hxu
          simpa [Rat.add_neg_cancel] using this
      · exact checkSymChainRow_loop_sound row u v colPerm hsz hlt hvu hsat hvxu (k + 1) hloop
    · exact checkSymChainRow_loop_sound row u v colPerm hsz hlt hvu hsat hvxu (k + 1) hloop
  · contradiction
termination_by k _ => n - k

private def checkSymBreakCols {n : Nat} (st : PresolveState n) (colPerm : Array Nat) : Bool :=
  loop 0
where
  loop (i : Nat) : Bool :=
    if hi : i < n then
      if hp : i < colPerm.size then
        let p := colPerm[i]
        if hpn : p < n then
          let j : Fin n := ⟨i, hi⟩
          let jp : Fin n := ⟨p, hpn⟩
          let okInt := !st.isIntAt j || st.isIntAt jp
          let okLb := optLbLe (st.lbAt j) (st.lbAt jp)
          let okUb := optUbLe (st.ubAt jp) (st.ubAt j)
          if okInt && okLb && okUb then loop (i + 1) else false
        else false
      else false
    else true
  termination_by n - i

private def checkSymBreakObj {n : Nat} (st : PresolveState n) (u v : Fin n) (colPerm : Array Nat) : Bool :=
  decide (st.obj.get v ≤ st.obj.get u) && loop 0
where
  loop (i : Nat) : Bool :=
    if hi : i < n then
      if hp : i < colPerm.size then
        let p := colPerm[i]
        if hpn : p < n then
          let j : Fin n := ⟨i, hi⟩
          let jp : Fin n := ⟨p, hpn⟩
          if i == u.1 || i == v.1 || st.obj.get j == st.obj.get jp then
            loop (i + 1)
          else false
        else false
      else false
    else true
  termination_by n - i

private def checkSymBreakRows {n : Nat} (st : PresolveState n) (v : Fin n)
    (colPerm rowPerm : Array Nat) : Bool :=
  (rowPerm.size == st.rows.size) && loop 0
where
  loop (i : Nat) : Bool :=
    if hi : i < st.rows.size then
      match st.rows[i] with
      | none => loop (i + 1)
      | some rowI =>
        let directOk :=
          if hrp : i < rowPerm.size then
            let rIdx := rowPerm[i]
            if hr : rIdx < st.rows.size then
              match st.rows[rIdx] with
              | some rowR => checkConPermImplies rowR rowI colPerm
              | none => false
            else false
          else false
        if directOk || checkSymChainRow rowI v colPerm then
          loop (i + 1)
        else false
    else true
  termination_by st.rows.size - i

/-- Executes one exact rational `PresolveStep` on `st`, returning `none` if any verification check fails. -/
def applyPresolveStep {n : Nat} (st : PresolveState n) (step : PresolveStep) :
    Option (PresolveState n) :=
  match step with
  | .roundBound col isUpper newBound =>
    if hcol : col < n then
      let j : Fin n := ⟨col, hcol⟩
      if st.isIntAt j then
        if isUpper then
          match st.ubAt j with
          | none => none
          | some oldU =>
            if decide ((oldU.floor : Rat) ≤ newBound) && decide (newBound ≤ oldU) then
              let ub' := st.ub.set col (some newBound) (by rw [st.ub_size]; exact hcol)
              some { st with ub := ub', ub_size := by rw [Array.size_set]; exact st.ub_size }
            else none
        else
          match st.lbAt j with
          | none => none
          | some oldL =>
            if decide (newBound ≤ (oldL.ceil : Rat)) && decide (oldL ≤ newBound) then
              let lb' := st.lb.set col (some newBound) (by rw [st.lb_size]; exact hcol)
              some { st with lb := lb', lb_size := by rw [Array.size_set]; exact st.lb_size }
            else none
      else none
    else none
  | .activityBound row col isUpper roundInt newBound =>
    if hrow : row < st.rows.size then
      match st.rows[row] with
      | none => none
      | some con =>
        if hcol : col < n then
          let j : Fin n := ⟨col, hcol⟩
          let a := con.coeffs.get j
          if a == 0 then none
          else if isUpper then
            let imp? : Option Rat :=
              if 0 < a then
                if con.sense == .le || con.sense == .eq then
                  (sumMinAct st con.coeffs (some j)).map (fun m => (con.rhs - m) / a)
                else none
              else
                if con.sense == .ge || con.sense == .eq then
                  (sumMaxAct st con.coeffs (some j)).map (fun M => (con.rhs - M) / a)
                else none
            match imp? with
            | none => none
            | some imp =>
              let effImp? : Option Rat :=
                if roundInt then
                  if st.isIntAt j then some (imp.floor : Rat) else none
                else some imp
              match effImp? with
              | none => none
              | some effImp =>
                if decide (effImp ≤ newBound) && optUbLe (some newBound) (st.ubAt j) then
                  let ub' := st.ub.set col (some newBound) (by rw [st.ub_size]; exact hcol)
                  some { st with ub := ub', ub_size := by rw [Array.size_set]; exact st.ub_size }
                else none
          else
            let imp? : Option Rat :=
              if 0 < a then
                if con.sense == .ge || con.sense == .eq then
                  (sumMaxAct st con.coeffs (some j)).map (fun M => (con.rhs - M) / a)
                else none
              else
                if con.sense == .le || con.sense == .eq then
                  (sumMinAct st con.coeffs (some j)).map (fun m => (con.rhs - m) / a)
                else none
            match imp? with
            | none => none
            | some imp =>
              let effImp? : Option Rat :=
                if roundInt then
                  if st.isIntAt j then some (imp.ceil : Rat) else none
                else some imp
              match effImp? with
              | none => none
              | some effImp =>
                if decide (newBound ≤ effImp) && optLbLe (st.lbAt j) (some newBound) then
                  let lb' := st.lb.set col (some newBound) (by rw [st.lb_size]; exact hcol)
                  some { st with lb := lb', lb_size := by rw [Array.size_set]; exact st.lb_size }
                else none
        else none
    else none
  | .gcdRowDiv row divisor =>
    if hrow : row < st.rows.size then
      if 0 < divisor then
        match st.rows[row] with
        | none => none
        | some con =>
          let dRat : Rat := (divisor : Rat)
          let newCoeffs : RatVec n := RatVec.ofFn (fun j => con.coeffs.get j / dRat)
          if isIntegralLinVec st.toInstance newCoeffs then
            let qRhs := con.rhs / dRat
            match con.sense with
            | .le =>
              let newCon : LinCon n := { coeffs := newCoeffs, sense := .le, rhs := (qRhs.floor : Rat) }
              some { st with rows := st.rows.set row (some newCon) hrow }
            | .ge =>
              let newCon : LinCon n := { coeffs := newCoeffs, sense := .ge, rhs := (qRhs.ceil : Rat) }
              some { st with rows := st.rows.set row (some newCon) hrow }
            | .eq =>
              if qRhs.den == 1 then
                let newCon : LinCon n := { coeffs := newCoeffs, sense := .eq, rhs := qRhs }
                some { st with rows := st.rows.set row (some newCon) hrow }
              else none
          else none
      else none
    else none
  | .substFixedRow row col val =>
    if hrow : row < st.rows.size then
      if hcol : col < n then
        let j : Fin n := ⟨col, hcol⟩
        if st.lbAt j == some val && st.ubAt j == some val then
          match st.rows[row] with
          | none => none
          | some con =>
            let a := con.coeffs.get j
            let newCoeffs : RatVec n := RatVec.ofFn (fun k => if k = j then 0 else con.coeffs.get k)
            let newCon : LinCon n := { coeffs := newCoeffs, sense := con.sense, rhs := con.rhs - a * val }
            some { st with rows := st.rows.set row (some newCon) hrow }
        else none
      else none
    else none
  | .substFixedObj col val =>
    if hcol : col < n then
      let j : Fin n := ⟨col, hcol⟩
      if st.lbAt j == some val && st.ubAt j == some val then
        let c := st.obj.get j
        let newObj : RatVec n := RatVec.ofFn (fun k => if k = j then 0 else st.obj.get k)
        some { st with obj := newObj, objConst := st.objConst + c * val }
      else none
    else none
  | .singletonRowElim row col roundInt =>
    if hrow : row < st.rows.size then
      if hcol : col < n then
        let j : Fin n := ⟨col, hcol⟩
        match st.rows[row] with
        | none => none
        | some con =>
          let a := con.coeffs.get j
          if !(a == 0) && checkOnlyColNonzero con.coeffs j then
            let b := con.rhs / a
            let rows' := st.rows.set row none hrow
            match con.sense with
            | .eq =>
              if roundInt && !(st.isIntAt j && b.den == 1) then none
              else
                let lb' := st.lb.set col (tightenLBOpt (st.lbAt j) b) (by rw [st.lb_size]; exact hcol)
                let ub' := st.ub.set col (tightenUBOpt (st.ubAt j) b) (by rw [st.ub_size]; exact hcol)
                some { st with
                  lb := lb', lb_size := by rw [Array.size_set]; exact st.lb_size
                  ub := ub', ub_size := by rw [Array.size_set]; exact st.ub_size
                  rows := rows' }
            | .le =>
              if 0 < a then
                let uNew? : Option Rat :=
                  if roundInt then (if st.isIntAt j then some (b.floor : Rat) else none) else some b
                match uNew? with
                | none => none
                | some uNew =>
                  let ub' := st.ub.set col (tightenUBOpt (st.ubAt j) uNew) (by rw [st.ub_size]; exact hcol)
                  some { st with ub := ub', ub_size := by rw [Array.size_set]; exact st.ub_size, rows := rows' }
              else
                let lNew? : Option Rat :=
                  if roundInt then (if st.isIntAt j then some (b.ceil : Rat) else none) else some b
                match lNew? with
                | none => none
                | some lNew =>
                  let lb' := st.lb.set col (tightenLBOpt (st.lbAt j) lNew) (by rw [st.lb_size]; exact hcol)
                  some { st with lb := lb', lb_size := by rw [Array.size_set]; exact st.lb_size, rows := rows' }
            | .ge =>
              if 0 < a then
                let lNew? : Option Rat :=
                  if roundInt then (if st.isIntAt j then some (b.ceil : Rat) else none) else some b
                match lNew? with
                | none => none
                | some lNew =>
                  let lb' := st.lb.set col (tightenLBOpt (st.lbAt j) lNew) (by rw [st.lb_size]; exact hcol)
                  some { st with lb := lb', lb_size := by rw [Array.size_set]; exact st.lb_size, rows := rows' }
              else
                let uNew? : Option Rat :=
                  if roundInt then (if st.isIntAt j then some (b.floor : Rat) else none) else some b
                match uNew? with
                | none => none
                | some uNew =>
                  let ub' := st.ub.set col (tightenUBOpt (st.ubAt j) uNew) (by rw [st.ub_size]; exact hcol)
                  some { st with ub := ub', ub_size := by rw [Array.size_set]; exact st.ub_size, rows := rows' }
          else none
      else none
    else none
  | .redundantRow row =>
    if hrow : row < st.rows.size then
      match st.rows[row] with
      | none => none
      | some con =>
        let ok :=
          match con.sense with
          | .le =>
            match sumMaxAct st con.coeffs none with
            | some M => decide (M ≤ con.rhs)
            | none => false
          | .ge =>
            match sumMinAct st con.coeffs none with
            | some m => decide (con.rhs ≤ m)
            | none => false
          | .eq =>
            match sumMinAct st con.coeffs none, sumMaxAct st con.coeffs none with
            | some m, some M => decide (con.rhs ≤ m) && decide (M ≤ con.rhs)
            | _, _ => false
        if ok then some { st with rows := st.rows.set row none hrow } else none
    else none
  | .binStrengthen row col delta =>
    if hrow : row < st.rows.size then
      if hcol : col < n then
        let j : Fin n := ⟨col, hcol⟩
        if st.isIntAt j && st.lbAt j == some 0 && st.ubAt j == some 1 && decide (0 ≤ delta) then
          match st.rows[row] with
          | none => none
          | some con =>
            if con.sense == .le then
              let a := con.coeffs.get j
              if a == 0 then none
              else
                match sumMaxAct st con.coeffs (some j) with
                | none => none
                | some maxExcl =>
                  if 0 < a then
                    if decide (maxExcl ≤ con.rhs - delta) then
                      let newCoeffs : RatVec n :=
                        RatVec.ofFn (fun k => if k = j then a - delta else con.coeffs.get k)
                      let newCon : LinCon n := { coeffs := newCoeffs, sense := .le, rhs := con.rhs - delta }
                      some { st with rows := st.rows.set row (some newCon) hrow }
                    else none
                  else
                    if decide (maxExcl + (a + delta) ≤ con.rhs) then
                      let newCoeffs : RatVec n :=
                        RatVec.ofFn (fun k => if k = j then a + delta else con.coeffs.get k)
                      let newCon : LinCon n := { coeffs := newCoeffs, sense := .le, rhs := con.rhs }
                      some { st with rows := st.rows.set row (some newCon) hrow }
                    else none
            else none
        else none
      else none
    else none
  | .symBreakLe u v colPerm rowPerm =>
    if hu : u < n then
      if hv : v < n then
        if u != v then
          let ju : Fin n := ⟨u, hu⟩
          let jv : Fin n := ⟨v, hv⟩
          if checkPermInvolution n colPerm &&
              (if hpu : u < colPerm.size then colPerm[u] == v else false) &&
              checkSymBreakCols st colPerm &&
              checkSymBreakObj st ju jv colPerm &&
              checkSymBreakRows st jv colPerm rowPerm then
            let newCon : LinCon n := {
              coeffs := RatVec.ofFn (fun j => if j = ju then 1 else if j = jv then -1 else 0)
              sense := .le
              rhs := 0
            }
            some { st with rows := st.rows.push (some newCon) }
          else none
        else none
      else none
    else none

private theorem array_set_getElem_fin {α : Type} (arr : Array α) {row : Nat} (hrow : row < arr.size)
    (v : α) (i : Fin (arr.set row v hrow).size) :
    (arr.set row v hrow)[i] =
      if i.1 = row then v else arr[i.1]'(by simpa [Array.size_set] using i.2) := by
  change (arr.set row v hrow)[i.1] = _
  by_cases h : i.1 = row
  · simp [h]
  · simp only [h, ↓reduceIte]
    exact Array.getElem_set_ne hrow (by simpa [Array.size_set] using i.2) (Ne.symm h)

private theorem state_set_ub_inVarBounds {n : Nat} (st : PresolveState n)
    {col : Nat} (hcol : col < n) (newU : Rat) (x : Fin n → Rat) :
    let ub' := st.ub.set col (some newU) (by rw [st.ub_size]; exact hcol)
    let st' : PresolveState n := { st with ub := ub', ub_size := by rw [Array.size_set]; exact st.ub_size }
    inVarBounds st'.lbAt st'.ubAt x ↔
    (∀ j : Fin n,
      (∀ l, st.lbAt j = some l → l ≤ x j) ∧
      (if j = ⟨col, hcol⟩ then x j ≤ newU else ∀ u, st.ubAt j = some u → x j ≤ u)) := by
  intro ub' st'
  constructor
  · intro hb j
    refine ⟨(hb j).1, ?_⟩
    by_cases h : j = ⟨col, hcol⟩
    · subst h
      simp only [↓reduceIte]
      apply (hb ⟨col, hcol⟩).2 newU
      unfold PresolveState.ubAt
      simp [st', ub']
    · simp only [h, ↓reduceIte]
      intro u hu
      have hne : col ≠ j.1 := fun heq => h (Fin.ext heq.symm)
      apply (hb j).2 u
      unfold PresolveState.ubAt at hu ⊢
      simpa [st', ub', Array.getElem_set_ne (by rw [st.ub_size]; exact hcol) (by rw [st.ub_size]; exact j.2) hne] using hu
  · intro hb j
    refine ⟨(hb j).1, fun u hu => ?_⟩
    by_cases h : j = ⟨col, hcol⟩
    · subst h
      unfold PresolveState.ubAt at hu
      simp only [st', ub', Array.getElem_set_self] at hu
      injection hu with heq; subst heq
      have hj := (hb ⟨col, hcol⟩).2
      simpa using hj
    · have hne : col ≠ j.1 := fun heq => h (Fin.ext heq.symm)
      unfold PresolveState.ubAt at hu
      simp only [st', ub', Array.getElem_set_ne (by rw [st.ub_size]; exact hcol) (by rw [st.ub_size]; exact j.2) hne] at hu
      have hj := (hb j).2
      simp only [h, ↓reduceIte] at hj
      exact hj u hu

private theorem state_set_lb_inVarBounds {n : Nat} (st : PresolveState n)
    {col : Nat} (hcol : col < n) (newL : Rat) (x : Fin n → Rat) :
    let lb' := st.lb.set col (some newL) (by rw [st.lb_size]; exact hcol)
    let st' : PresolveState n := { st with lb := lb', lb_size := by rw [Array.size_set]; exact st.lb_size }
    inVarBounds st'.lbAt st'.ubAt x ↔
    (∀ j : Fin n,
      (if j = ⟨col, hcol⟩ then newL ≤ x j else ∀ l, st.lbAt j = some l → l ≤ x j) ∧
      (∀ u, st.ubAt j = some u → x j ≤ u)) := by
  intro lb' st'
  constructor
  · intro hb j
    refine ⟨?_, (hb j).2⟩
    by_cases h : j = ⟨col, hcol⟩
    · subst h
      simp only [↓reduceIte]
      apply (hb ⟨col, hcol⟩).1 newL
      unfold PresolveState.lbAt
      simp [st', lb']
    · simp only [h, ↓reduceIte]
      intro l hl
      have hne : col ≠ j.1 := fun heq => h (Fin.ext heq.symm)
      apply (hb j).1 l
      unfold PresolveState.lbAt at hl ⊢
      simpa [st', lb', Array.getElem_set_ne (by rw [st.lb_size]; exact hcol) (by rw [st.lb_size]; exact j.2) hne] using hl
  · intro hb j
    refine ⟨fun l hl => ?_, (hb j).2⟩
    by_cases h : j = ⟨col, hcol⟩
    · subst h
      unfold PresolveState.lbAt at hl
      simp only [st', lb', Array.getElem_set_self] at hl
      injection hl with heq; subst heq
      have hj := (hb ⟨col, hcol⟩).1
      simpa using hj
    · have hne : col ≠ j.1 := fun heq => h (Fin.ext heq.symm)
      unfold PresolveState.lbAt at hl
      simp only [st', lb', Array.getElem_set_ne (by rw [st.lb_size]; exact hcol) (by rw [st.lb_size]; exact j.2) hne] at hl
      have hj := (hb j).1
      simp only [h, ↓reduceIte] at hj
      exact hj l hl

private theorem state_set_ub_opt_inVarBounds {n : Nat} (st : PresolveState n)
    {col : Nat} (hcol : col < n) (newU : Rat) (x : Fin n → Rat) :
    let ub' := st.ub.set col (tightenUBOpt (st.ubAt ⟨col, hcol⟩) newU) (by rw [st.ub_size]; exact hcol)
    let st' : PresolveState n := { st with ub := ub', ub_size := by rw [Array.size_set]; exact st.ub_size }
    inVarBounds st'.lbAt st'.ubAt x ↔ (inVarBounds st.lbAt st.ubAt x ∧ x ⟨col, hcol⟩ ≤ newU) := by
  intro ub' st'
  constructor
  · intro hb
    have hxU : x ⟨col, hcol⟩ ≤ newU := by
      cases h0 : st.ubAt ⟨col, hcol⟩ with
      | none =>
        apply (hb ⟨col, hcol⟩).2 newU
        unfold PresolveState.ubAt
        simp [st', ub', tightenUBOpt, h0]
      | some u0 =>
        have hm := (hb ⟨col, hcol⟩).2 (min u0 newU) (by
          unfold PresolveState.ubAt
          simp [st', ub', tightenUBOpt, h0])
        exact Rat.le_trans hm (rat_min_le_right u0 newU)
    refine ⟨fun j => ⟨(hb j).1, fun u hu => ?_⟩, hxU⟩
    by_cases h : j = ⟨col, hcol⟩
    · subst h
      have hm := (hb ⟨col, hcol⟩).2 (min u newU) (by
        unfold PresolveState.ubAt
        simp [st', ub', tightenUBOpt, hu])
      exact Rat.le_trans hm (rat_min_le_left u newU)
    · have hne : col ≠ j.1 := fun heq => h (Fin.ext heq.symm)
      apply (hb j).2 u
      unfold PresolveState.ubAt at hu ⊢
      simpa [st', ub', Array.getElem_set_ne (by rw [st.ub_size]; exact hcol) (by rw [st.ub_size]; exact j.2) hne] using hu
  · rintro ⟨hb, hxU⟩ j
    refine ⟨(hb j).1, fun u hu => ?_⟩
    by_cases h : j = ⟨col, hcol⟩
    · subst h
      unfold PresolveState.ubAt at hu
      simp only [st', ub', Array.getElem_set_self] at hu
      cases h0 : st.ubAt ⟨col, hcol⟩ with
      | none =>
        rw [h0, tightenUBOpt] at hu
        injection hu with heq; subst heq
        exact hxU
      | some u0 =>
        rw [h0, tightenUBOpt] at hu
        injection hu with heq; subst heq
        exact rat_le_min_iff.mpr ⟨(hb ⟨col, hcol⟩).2 u0 h0, hxU⟩
    · have hne : col ≠ j.1 := fun heq => h (Fin.ext heq.symm)
      unfold PresolveState.ubAt at hu
      simp only [st', ub', Array.getElem_set_ne (by rw [st.ub_size]; exact hcol) (by rw [st.ub_size]; exact j.2) hne] at hu
      exact (hb j).2 u hu

private theorem state_set_lb_opt_inVarBounds {n : Nat} (st : PresolveState n)
    {col : Nat} (hcol : col < n) (newL : Rat) (x : Fin n → Rat) :
    let lb' := st.lb.set col (tightenLBOpt (st.lbAt ⟨col, hcol⟩) newL) (by rw [st.lb_size]; exact hcol)
    let st' : PresolveState n := { st with lb := lb', lb_size := by rw [Array.size_set]; exact st.lb_size }
    inVarBounds st'.lbAt st'.ubAt x ↔ (inVarBounds st.lbAt st.ubAt x ∧ newL ≤ x ⟨col, hcol⟩) := by
  intro lb' st'
  constructor
  · intro hb
    have hxL : newL ≤ x ⟨col, hcol⟩ := by
      cases h0 : st.lbAt ⟨col, hcol⟩ with
      | none =>
        apply (hb ⟨col, hcol⟩).1 newL
        unfold PresolveState.lbAt
        simp [st', lb', tightenLBOpt, h0]
      | some l0 =>
        have hm := (hb ⟨col, hcol⟩).1 (max l0 newL) (by
          unfold PresolveState.lbAt
          simp [st', lb', tightenLBOpt, h0])
        have hle : newL ≤ max l0 newL := by
          rw [Rat.max_def]; split
          · exact Rat.le_refl
          · exact Rat.le_of_lt (Rat.not_le.mp ‹_›)
        exact Rat.le_trans hle hm
    refine ⟨fun j => ⟨fun l hl => ?_, (hb j).2⟩, hxL⟩
    by_cases h : j = ⟨col, hcol⟩
    · subst h
      have hm := (hb ⟨col, hcol⟩).1 (max l newL) (by
        unfold PresolveState.lbAt
        simp [st', lb', tightenLBOpt, hl])
      have hle : l ≤ max l newL := by
        rw [Rat.max_def]; split
        · assumption
        · exact Rat.le_refl
      exact Rat.le_trans hle hm
    · have hne : col ≠ j.1 := fun heq => h (Fin.ext heq.symm)
      apply (hb j).1 l
      unfold PresolveState.lbAt at hl ⊢
      simpa [st', lb', Array.getElem_set_ne (by rw [st.lb_size]; exact hcol) (by rw [st.lb_size]; exact j.2) hne] using hl
  · rintro ⟨hb, hxL⟩ j
    refine ⟨fun l hl => ?_, (hb j).2⟩
    by_cases h : j = ⟨col, hcol⟩
    · subst h
      unfold PresolveState.lbAt at hl
      simp only [st', lb', Array.getElem_set_self] at hl
      cases h0 : st.lbAt ⟨col, hcol⟩ with
      | none =>
        rw [h0, tightenLBOpt] at hl
        injection hl with heq; subst heq
        exact hxL
      | some l0 =>
        rw [h0, tightenLBOpt] at hl
        injection hl with heq; subst heq
        rw [Rat.max_def]; split
        · exact hxL
        · exact (hb ⟨col, hcol⟩).1 l0 h0
    · have hne : col ≠ j.1 := fun heq => h (Fin.ext heq.symm)
      unfold PresolveState.lbAt at hl
      simp only [st', lb', Array.getElem_set_ne (by rw [st.lb_size]; exact hcol) (by rw [st.lb_size]; exact j.2) hne] at hl
      exact (hb j).1 l hl

private theorem checkSymBreakCols_sound {n : Nat} (st : PresolveState n) (colPerm : Array Nat)
    (hsz : colPerm.size = n) (hlt : ∀ j : Fin n, permAt colPerm hsz j < n) :
    ∀ k (hk : k ≤ n),
      (∀ i (hi : i < k),
        let j : Fin n := ⟨i, Nat.lt_of_lt_of_le hi hk⟩
        let jp : Fin n := permFn colPerm hsz hlt j
        (st.isIntAt j = true → st.isIntAt jp = true) ∧
        (∀ l, st.lbAt j = some l → ∃ l', st.lbAt jp = some l' ∧ l ≤ l') ∧
        (∀ u, st.ubAt j = some u → ∃ u', st.ubAt jp = some u' ∧ u' ≤ u)) →
      checkSymBreakCols.loop st colPerm k = true →
      ∀ j : Fin n,
        let jp : Fin n := permFn colPerm hsz hlt j
        (st.isIntAt j = true → st.isIntAt jp = true) ∧
        (∀ l, st.lbAt j = some l → ∃ l', st.lbAt jp = some l' ∧ l ≤ l') ∧
        (∀ u, st.ubAt j = some u → ∃ u', st.ubAt jp = some u' ∧ u' ≤ u) := by
  intro k hk hprev hloop j
  unfold checkSymBreakCols.loop at hloop
  dsimp only at hloop
  split at hloop
  · next hltK =>
    have hpk : k < colPerm.size := by rw [hsz]; exact hltK
    have hpn : colPerm[k]'hpk < n := hlt ⟨k, hltK⟩
    simp only [hpk, hpn, ↓reduceDIte] at hloop
    split at hloop
    · next hok =>
      let jk : Fin n := ⟨k, hltK⟩
      let jpk : Fin n := ⟨colPerm[k]'hpk, hpn⟩
      simp only [Bool.and_eq_true] at hok
      have hcurInt : st.isIntAt jk = true → st.isIntAt jpk = true := by
        intro hi
        rcases Bool.or_eq_true _ _ |>.mp hok.1.1 with hno | hyes
        · rw [hi] at hno; contradiction
        · exact hyes
      have hcurLb : ∀ l, st.lbAt jk = some l → ∃ l', st.lbAt jpk = some l' ∧ l ≤ l' := by
        intro l hl
        have hlb := hok.1.2
        unfold optLbLe at hlb; rw [hl] at hlb
        cases h2 : st.lbAt jpk with
        | none => rw [h2] at hlb; contradiction
        | some l' => rw [h2] at hlb; exact ⟨l', rfl, decide_eq_true_eq.mp hlb⟩
      have hcurUb : ∀ u, st.ubAt jk = some u → ∃ u', st.ubAt jpk = some u' ∧ u' ≤ u := by
        intro u hu
        have hub := hok.2
        unfold optUbLe at hub; rw [hu] at hub
        cases h2 : st.ubAt jpk with
        | none => rw [h2] at hub; contradiction
        | some u' => rw [h2] at hub; exact ⟨u', rfl, decide_eq_true_eq.mp hub⟩
      have hnext : ∀ i (hi : i < k + 1),
          let ji : Fin n := ⟨i, Nat.lt_of_lt_of_le hi hltK⟩
          let jpi : Fin n := permFn colPerm hsz hlt ji
          (st.isIntAt ji = true → st.isIntAt jpi = true) ∧
          (∀ l, st.lbAt ji = some l → ∃ l', st.lbAt jpi = some l' ∧ l ≤ l') ∧
          (∀ u, st.ubAt ji = some u → ∃ u', st.ubAt jpi = some u' ∧ u' ≤ u) := by
        intro i hi
        by_cases hik : i = k
        · subst hik; exact ⟨hcurInt, hcurLb, hcurUb⟩
        · exact hprev i (by omega)
      exact checkSymBreakCols_sound st colPerm hsz hlt (k + 1) hltK hnext hloop j
    · contradiction
  · next hge =>
    have hkn : k = n := by omega
    subst hkn
    exact hprev j.1 j.2
termination_by k _ _ _ => n - k

private theorem checkSymBreakObj_loop_sound {n : Nat} (st : PresolveState n) (u v : Fin n)
    (colPerm : Array Nat) (hsz : colPerm.size = n) (hlt : ∀ j : Fin n, permAt colPerm hsz j < n) :
    ∀ k (hk : k ≤ n),
      (∀ i (hi : i < k),
        let j : Fin n := ⟨i, Nat.lt_of_lt_of_le hi hk⟩
        j ≠ u → j ≠ v → st.obj.get j = st.obj.get (permFn colPerm hsz hlt j)) →
      checkSymBreakObj.loop st u v colPerm k = true →
      ∀ j : Fin n, j ≠ u → j ≠ v → st.obj.get j = st.obj.get (permFn colPerm hsz hlt j) := by
  intro k hk hprev hloop j hju hjv
  unfold checkSymBreakObj.loop at hloop
  dsimp only at hloop
  split at hloop
  · next hltK =>
    have hpk : k < colPerm.size := by rw [hsz]; exact hltK
    have hpn : colPerm[k]'hpk < n := hlt ⟨k, hltK⟩
    simp only [hpk, hpn, ↓reduceDIte] at hloop
    split at hloop
    · next hok =>
      have hcur : (⟨k, hltK⟩ : Fin n) ≠ u → (⟨k, hltK⟩ : Fin n) ≠ v →
          st.obj.get ⟨k, hltK⟩ = st.obj.get (permFn colPerm hsz hlt ⟨k, hltK⟩) := by
        intro hku hkv
        rcases Bool.or_eq_true _ _ |>.mp hok with h12 | heq
        · rcases Bool.or_eq_true _ _ |>.mp h12 with h1 | h2
          · exact absurd (Fin.ext (beq_iff_eq.mp h1)) hku
          · exact absurd (Fin.ext (beq_iff_eq.mp h2)) hkv
        · exact beq_iff_eq.mp heq
      have hnext : ∀ i (hi : i < k + 1),
          (⟨i, Nat.lt_of_lt_of_le hi hltK⟩ : Fin n) ≠ u →
          (⟨i, Nat.lt_of_lt_of_le hi hltK⟩ : Fin n) ≠ v →
          st.obj.get ⟨i, Nat.lt_of_lt_of_le hi hltK⟩ =
            st.obj.get (permFn colPerm hsz hlt ⟨i, Nat.lt_of_lt_of_le hi hltK⟩) := by
        intro i hi hiu hiv
        by_cases hik : i = k
        · subst hik; exact hcur hiu hiv
        · exact hprev i (by omega) hiu hiv
      exact checkSymBreakObj_loop_sound st u v colPerm hsz hlt (k + 1) hltK hnext hloop j hju hjv
    · contradiction
  · next hge =>
    have hkn : k = n := by omega
    subst hkn
    exact hprev j.1 j.2 hju hjv
termination_by k _ _ _ _ _ => n - k

private theorem stateObj_perm_le_of_mono {n : Nat} {c : RatVec n} {x : Fin n → Rat}
    {f : Fin n → Fin n} (hinv : ∀ k : Fin n, f (f k) = k)
    (hf : ∀ i₁ i₂ : Fin n, f i₁ = f i₂ → i₁ = i₂)
    {u v : Fin n} (huv : f u = v) (hne : u ≠ v)
    (hc_other : ∀ k : Fin n, k ≠ u → k ≠ v → c.get k = c.get (f k))
    (hc_mono : c.get v ≤ c.get u) (hx_le : x v ≤ x u) :
    finSum n (fun k => c.get k * x (f k)) ≤ finSum n (fun k => c.get k * x k) := by
  have hvu : f v = u := by rw [← huv, hinv]
  let ceq : Fin n → Rat := fun k => if k = u then c.get v else c.get k
  let delta : Fin n → Rat := fun k => if k = u then c.get u - c.get v else 0
  have hceq_perm : ∀ k : Fin n, ceq k = ceq (f k) := by
    intro k
    by_cases hku : k = u
    · subst hku; simp [ceq, huv]
    · by_cases hkv : k = v
      · subst hkv; simp [ceq, hvu]
      · have hfneU : f k ≠ u := fun h => hkv (hf k v (h.trans hvu.symm))
        simp only [ceq, hku, hfneU, ↓reduceIte]
        exact hc_other k hku hkv
  have hceq_sum :
      finSum n (fun k => ceq k * x (f k)) = finSum n (fun k => ceq k * x k) := by
    have h1 : finSum n (fun k => ceq k * x (f k)) =
        finSum n (fun k => ceq (f k) * x (f k)) :=
      finSum_congr (fun k => by rw [hceq_perm k])
    rw [h1]
    exact finSum_inj_eq (g := fun k => ceq k * x k) hf
  have hdelta_perm : finSum n (fun k => delta k * x (f k)) = (c.get u - c.get v) * x v := by
    have h1 : finSum n (fun k => delta k * x (f k)) =
        finSum n (fun k => if k = u then (c.get u - c.get v) * x v else 0) := by
      apply finSum_congr; intro k
      by_cases hku : k = u
      · subst hku; simp [delta, huv]
      · simp [delta, hku, Rat.zero_mul]
    rw [h1, finSum_single]
  have hdelta_orig : finSum n (fun k => delta k * x k) = (c.get u - c.get v) * x u := by
    have h1 : finSum n (fun k => delta k * x k) =
        finSum n (fun k => if k = u then (c.get u - c.get v) * x u else 0) := by
      apply finSum_congr; intro k
      by_cases hku : k = u
      · subst hku; simp [delta]
      · simp [delta, hku, Rat.zero_mul]
    rw [h1, finSum_single]
  have hsplit_c : ∀ k : Fin n, c.get k = ceq k + delta k := by
    intro k
    by_cases hku : k = u
    · subst hku
      simp only [ceq, delta, ↓reduceIte]
      rw [Rat.add_comm, Rat.sub_add_cancel]
    · simp [ceq, delta, hku, Rat.add_zero]
  have hlhs : finSum n (fun k => c.get k * x (f k)) =
      finSum n (fun k => ceq k * x (f k)) + finSum n (fun k => delta k * x (f k)) := by
    rw [← finSum_add]
    apply finSum_congr; intro k
    rw [hsplit_c k, Rat.add_mul]
  have hrhs : finSum n (fun k => c.get k * x k) =
      finSum n (fun k => ceq k * x k) + finSum n (fun k => delta k * x k) := by
    rw [← finSum_add]
    apply finSum_congr; intro k
    rw [hsplit_c k, Rat.add_mul]
  rw [hlhs, hrhs, hceq_sum, hdelta_perm, hdelta_orig]
  have hdiff_nonneg : 0 ≤ c.get u - c.get v := by
    have := Rat.add_le_add_right (c := -c.get v) |>.mpr hc_mono
    simpa [Rat.sub_eq_add_neg, Rat.add_neg_cancel] using this
  exact Rat.add_le_add_left.mpr (Rat.mul_le_mul_of_nonneg_left hx_le hdiff_nonneg)

private theorem checkSymBreakRows_sound {n : Nat} (st : PresolveState n) (u v : Fin n)
    (colPerm rowPerm : Array Nat) (hsz : colPerm.size = n)
    (hlt : ∀ j : Fin n, permAt colPerm hsz j < n)
    (hf : ∀ i₁ i₂ : Fin n, permFn colPerm hsz hlt i₁ = permFn colPerm hsz hlt i₂ → i₁ = i₂)
    (hvu : permFn colPerm hsz hlt v = u)
    (hrsz : rowPerm.size = st.rows.size) :
    ∀ k (hk : k ≤ st.rows.size),
      (∀ i (hi : i < k), ∀ r, st.rows[i]'(Nat.lt_of_lt_of_le hi hk) = some r →
        ∀ x : Fin n → Rat, StateFeasible st x → x v ≤ x u →
          r.sat (fun j => x (permFn colPerm hsz hlt j))) →
      checkSymBreakRows.loop st v colPerm rowPerm k = true →
      ∀ i : Fin st.rows.size, ∀ r, st.rows[i] = some r →
        ∀ x : Fin n → Rat, StateFeasible st x → x v ≤ x u →
          r.sat (fun j => x (permFn colPerm hsz hlt j)) := by
  intro k hk hprev hloop i r hr x hx hvxu
  unfold checkSymBreakRows.loop at hloop
  dsimp only at hloop
  split at hloop
  · next hltK =>
    cases hcur : st.rows[k] with
    | none =>
      simp only [hcur] at hloop
      have hnext : ∀ m (hm : m < k + 1), ∀ rm, st.rows[m]'(Nat.lt_of_lt_of_le hm hltK) = some rm →
          ∀ y : Fin n → Rat, StateFeasible st y → y v ≤ y u →
            rm.sat (fun j => y (permFn colPerm hsz hlt j)) := by
        intro m hm rm hrm y hy hyv
        by_cases hmk : m = k
        · subst hmk; rw [hcur] at hrm; contradiction
        · exact hprev m (by omega) rm hrm y hy hyv
      exact checkSymBreakRows_sound st u v colPerm rowPerm hsz hlt hf hvu hrsz
        (k + 1) hltK hnext hloop i r hr x hx hvxu
    | some rowI =>
      have hrpk : k < rowPerm.size := by rw [hrsz]; exact hltK
      simp only [hcur, hrpk, ↓reduceDIte] at hloop
      generalize hokEq :
        ((if hr : rowPerm[k]'hrpk < st.rows.size then
            match st.rows[rowPerm[k]'hrpk]'hr with
            | some rowR => checkConPermImplies rowR rowI colPerm
            | none => false
          else false) || checkSymChainRow rowI v colPerm) = rowOk at hloop
      cases hokB : rowOk with
      | false => simp [hokB] at hloop
      | true =>
        simp only [hokB, ↓reduceIte] at hloop
        have hok :
            ((if hr : rowPerm[k]'hrpk < st.rows.size then
                match st.rows[rowPerm[k]'hrpk]'hr with
                | some rowR => checkConPermImplies rowR rowI colPerm
                | none => false
              else false) || checkSymChainRow rowI v colPerm) = true := hokEq.trans hokB
        have hcurSat : ∀ y : Fin n → Rat, StateFeasible st y → y v ≤ y u →
            rowI.sat (fun j => y (permFn colPerm hsz hlt j)) := by
          intro y hy hyv
          rcases Bool.or_eq_true _ _ |>.mp hok with hdir | hchain
          · split at hdir
            · next hri =>
              cases hsrc : st.rows[rowPerm[k]'hrpk]'hri with
              | none => simp [hsrc] at hdir
              | some rowR =>
                simp only [hsrc] at hdir
                exact checkConPermImplies_sound hsz hlt hf hdir (hy.2.1 ⟨rowPerm[k]'hrpk, hri⟩ rowR hsrc)
            · contradiction
          · exact checkSymChainRow_loop_sound rowI u v colPerm hsz hlt hvu
              (hy.2.1 ⟨k, hltK⟩ rowI hcur) hyv 0 hchain
        have hnext : ∀ m (hm : m < k + 1), ∀ rm, st.rows[m]'(Nat.lt_of_lt_of_le hm hltK) = some rm →
            ∀ y : Fin n → Rat, StateFeasible st y → y v ≤ y u →
              rm.sat (fun j => y (permFn colPerm hsz hlt j)) := by
          intro m hm rm hrm y hy hyv
          by_cases hmk : m = k
          · subst hmk
            rw [hcur] at hrm; injection hrm with hEq; subst hEq
            exact hcurSat y hy hyv
          · exact hprev m (by omega) rm hrm y hy hyv
        exact checkSymBreakRows_sound st u v colPerm rowPerm hsz hlt hf hvu hrsz
          (k + 1) hltK hnext hloop i r hr x hx hvxu
  · next hge =>
    have hkn : k = st.rows.size := by omega
    subst hkn
    exact hprev i.1 i.2 r hr x hx hvxu
termination_by k _ _ _ _ _ _ _ _ => st.rows.size - k

/-- Soundness of a single `PresolveStep`: proves both forward objective-preserving projection
and backward solution lifting with exact objective equality. -/
theorem applyPresolveStep_sound {n : Nat} {st st' : PresolveState n} {step : PresolveStep}
    (h : applyPresolveStep st step = some st') :
    (∀ x : Fin n → Rat, StateFeasible st x →
      ∃ x' : Fin n → Rat, StateFeasible st' x' ∧ StateObjective st' x' ≤ StateObjective st x) ∧
    (∀ x' : Fin n → Rat, StateFeasible st' x' →
      StateFeasible st x' ∧ StateObjective st x' = StateObjective st' x') := by
  cases step with
  | roundBound col isUpper newBound =>
    unfold applyPresolveStep at h; dsimp only at h
    split at h
    · next hcol =>
      split at h
      · next hint =>
        cases isUpper
        · simp only [Bool.false_eq_true, ↓reduceIte] at h
          cases h0 : st.lbAt ⟨col, hcol⟩ with
          | none => simp [h0] at h
          | some oldL =>
            simp only [h0] at h
            split at h
            · next hok =>
              injection h with hst; subst hst
              simp only [Bool.and_eq_true] at hok
              have h1 : newBound ≤ (oldL.ceil : Rat) := decide_eq_true_eq.mp hok.1
              have h2 : oldL ≤ newBound := decide_eq_true_eq.mp hok.2
              constructor
              · intro x hx
                refine ⟨x, ⟨?_, hx.2.1, hx.2.2⟩, Rat.le_refl⟩
                refine (state_set_lb_inVarBounds st hcol newBound x).mpr (fun j => ⟨?_, (hx.1 j).2⟩)
                by_cases hj : j = ⟨col, hcol⟩
                · subst hj; simp only [↓reduceIte]
                  obtain ⟨z, hz⟩ := hx.2.2 ⟨col, hcol⟩ hint
                  have hge : oldL ≤ (z : Rat) := hz ▸ (hx.1 ⟨col, hcol⟩).1 oldL h0
                  have hceil : (oldL.ceil : Rat) ≤ (z : Rat) :=
                    Rat.intCast_le_intCast.mpr (Rat.ceil_le_iff.mpr hge)
                  rw [hz]; exact Rat.le_trans h1 hceil
                · simp only [hj, ↓reduceIte]; exact (hx.1 j).1
              · intro x' hx'
                have hb' := (state_set_lb_inVarBounds st hcol newBound x').mp hx'.1
                refine ⟨⟨fun j => ⟨?_, (hb' j).2⟩, hx'.2.1, hx'.2.2⟩, rfl⟩
                intro l hl
                by_cases hj : j = ⟨col, hcol⟩
                · subst hj
                  rw [h0] at hl; injection hl with hlEq; subst hlEq
                  have hxNew : newBound ≤ x' ⟨col, hcol⟩ := by simpa using (hb' ⟨col, hcol⟩).1
                  exact Rat.le_trans h2 hxNew
                · have hne := (hb' j).1; simp only [hj, ↓reduceIte] at hne; exact hne l hl
            · contradiction
        · simp only [↓reduceIte] at h
          cases h0 : st.ubAt ⟨col, hcol⟩ with
          | none => simp [h0] at h
          | some oldU =>
            simp only [h0] at h
            split at h
            · next hok =>
              injection h with hst; subst hst
              simp only [Bool.and_eq_true] at hok
              have h1 : (oldU.floor : Rat) ≤ newBound := decide_eq_true_eq.mp hok.1
              have h2 : newBound ≤ oldU := decide_eq_true_eq.mp hok.2
              constructor
              · intro x hx
                refine ⟨x, ⟨?_, hx.2.1, hx.2.2⟩, Rat.le_refl⟩
                refine (state_set_ub_inVarBounds st hcol newBound x).mpr (fun j => ⟨(hx.1 j).1, ?_⟩)
                by_cases hj : j = ⟨col, hcol⟩
                · subst hj; simp only [↓reduceIte]
                  obtain ⟨z, hz⟩ := hx.2.2 ⟨col, hcol⟩ hint
                  have hle : (z : Rat) ≤ oldU := hz ▸ (hx.1 ⟨col, hcol⟩).2 oldU h0
                  have hfl : (z : Rat) ≤ (oldU.floor : Rat) :=
                    Rat.intCast_le_intCast.mpr (Rat.le_floor_iff.mpr hle)
                  rw [hz]; exact Rat.le_trans hfl h1
                · simp only [hj, ↓reduceIte]; exact (hx.1 j).2
              · intro x' hx'
                have hb' := (state_set_ub_inVarBounds st hcol newBound x').mp hx'.1
                refine ⟨⟨fun j => ⟨(hb' j).1, ?_⟩, hx'.2.1, hx'.2.2⟩, rfl⟩
                intro u hu
                by_cases hj : j = ⟨col, hcol⟩
                · subst hj
                  rw [h0] at hu; injection hu with huEq; subst huEq
                  have hxNew : x' ⟨col, hcol⟩ ≤ newBound := by simpa using (hb' ⟨col, hcol⟩).2
                  exact Rat.le_trans hxNew h2
                · have hne := (hb' j).2; simp only [hj, ↓reduceIte] at hne; exact hne u hu
            · contradiction
      · contradiction
    · contradiction
  | activityBound row col isUpper roundInt newBound =>
    unfold applyPresolveStep at h; dsimp only at h
    split at h
    · next hrow =>
      cases hcon : st.rows[row] with
      | none => simp [hcon] at h
      | some con =>
        simp only [hcon] at h
        split at h
        · next hcol =>
          let j : Fin n := ⟨col, hcol⟩
          let a : Rat := con.coeffs.get j
          by_cases ha0 : a = 0
          · have ha0b : (a == 0) = true := beq_iff_eq.mpr ha0
            simp only [a, j, ha0b, ↓reduceIte] at h; contradiction
          · have ha0b : (a == 0) = false := beq_false_of_ne ha0
            simp only [a, j, ha0b, Bool.false_eq_true, ↓reduceIte] at h
            cases isUpper
            · -- Lower bound tightening
              simp only [Bool.false_eq_true, ↓reduceIte] at h
              have himp_step :
                  ∀ imp,
                    (if 0 < a then
                      if con.sense == .ge || con.sense == .eq then
                        (sumMaxAct st con.coeffs (some j)).map (fun M => (con.rhs - M) / a)
                      else none
                    else
                      if con.sense == .le || con.sense == .eq then
                        (sumMinAct st con.coeffs (some j)).map (fun m => (con.rhs - m) / a)
                      else none) = some imp →
                    ∀ x : Fin n → Rat, StateFeasible st x → imp ≤ x j := by
                intro imp himp x hx
                have hsat := hx.2.1 ⟨row, hrow⟩ con hcon
                unfold LinCon.sat at hsat
                by_cases hpos : 0 < a
                · simp only [hpos, ↓reduceIte] at himp
                  split at himp
                  · next hs =>
                    cases hM : sumMaxAct st con.coeffs (some j) with
                    | none => simp [hM] at himp
                    | some M =>
                      simp only [hM, Option.map_some] at himp
                      injection himp with hEq; subst hEq
                      have hge : con.rhs ≤ con.coeffs.dot x := by
                        rcases Bool.or_eq_true _ _ |>.mp hs with h1 | h1
                        · rw [beq_iff_eq.mp h1] at hsat; exact hsat
                        · rw [beq_iff_eq.mp h1] at hsat; exact hsat ▸ Rat.le_refl
                      have hleM := sumMaxAct_some_ge hx.1 hM
                      have h1 : con.rhs ≤ M + a * x j := Rat.le_trans hge hleM
                      have h2 : con.rhs - M ≤ a * x j := by
                        have := Rat.add_le_add_right (c := -M) |>.mpr h1
                        rw [Rat.add_comm M (a * x j), Rat.add_assoc, Rat.add_neg_cancel, Rat.add_zero, ← Rat.sub_eq_add_neg] at this
                        exact this
                      have hdiv := rat_div_le_div_of_pos h2 hpos
                      rwa [Rat.mul_comm a (x j), rat_div_mul_eq ha0] at hdiv
                  · contradiction
                · have hneg : a < 0 := Rat.lt_of_le_of_ne (Rat.not_lt.mp hpos) ha0
                  simp only [hpos, ↓reduceIte] at himp
                  split at himp
                  · next hs =>
                    cases hm : sumMinAct st con.coeffs (some j) with
                    | none => simp [hm] at himp
                    | some m =>
                      simp only [hm, Option.map_some] at himp
                      injection himp with hEq; subst hEq
                      have hle : con.coeffs.dot x ≤ con.rhs := by
                        rcases Bool.or_eq_true _ _ |>.mp hs with h1 | h1
                        · rw [beq_iff_eq.mp h1] at hsat; exact hsat
                        · rw [beq_iff_eq.mp h1] at hsat; exact hsat ▸ Rat.le_refl
                      have hgeM := sumMinAct_some_le hx.1 hm
                      have h1 : m + a * x j ≤ con.rhs := Rat.le_trans hgeM hle
                      have h2 : a * x j ≤ con.rhs - m := by
                        have := Rat.add_le_add_right (c := -m) |>.mpr h1
                        rw [Rat.add_comm m (a * x j), Rat.add_assoc, Rat.add_neg_cancel, Rat.add_zero, ← Rat.sub_eq_add_neg] at this
                        exact this
                      have hdiv := rat_div_le_div_of_neg h2 hneg
                      rwa [Rat.mul_comm a (x j), rat_div_mul_eq ha0] at hdiv
                  · contradiction
              generalize himpEq :
                (if 0 < a then
                  if con.sense == .ge || con.sense == .eq then
                    (sumMaxAct st con.coeffs (some j)).map (fun M => (con.rhs - M) / a)
                  else none
                else
                  if con.sense == .le || con.sense == .eq then
                    (sumMinAct st con.coeffs (some j)).map (fun m => (con.rhs - m) / a)
                  else none) = impOpt at h
              cases impOpt with
              | none => simp at h
              | some imp =>
                simp only at h
                have hxImp := himp_step imp himpEq
                cases roundInt
                · simp only [Bool.false_eq_true, ↓reduceIte] at h
                  split at h
                  · next hok =>
                    injection h with hst; subst hst
                    simp only [Bool.and_eq_true] at hok
                    have hle : newBound ≤ imp := decide_eq_true_eq.mp hok.1
                    have hold : ∀ l, st.lbAt j = some l → l ≤ newBound := by
                      intro l hl; have hb := hok.2; unfold optLbLe at hb; rw [hl] at hb
                      exact decide_eq_true_eq.mp hb
                    constructor
                    · intro x hx
                      refine ⟨x, ⟨?_, hx.2.1, hx.2.2⟩, Rat.le_refl⟩
                      refine (state_set_lb_inVarBounds st hcol newBound x).mpr (fun k => ⟨?_, (hx.1 k).2⟩)
                      by_cases hkj : k = ⟨col, hcol⟩
                      · subst hkj; simp only [↓reduceIte]; exact Rat.le_trans hle (hxImp x hx)
                      · simp only [hkj, ↓reduceIte]; exact (hx.1 k).1
                    · intro x' hx'
                      have hb' := (state_set_lb_inVarBounds st hcol newBound x').mp hx'.1
                      refine ⟨⟨fun k => ⟨?_, (hb' k).2⟩, hx'.2.1, hx'.2.2⟩, rfl⟩
                      intro l hl
                      by_cases hkj : k = ⟨col, hcol⟩
                      · subst hkj
                        have hxN : newBound ≤ x' ⟨col, hcol⟩ := by simpa using (hb' ⟨col, hcol⟩).1
                        exact Rat.le_trans (hold l hl) hxN
                      · have hne := (hb' k).1; simp only [hkj, ↓reduceIte] at hne; exact hne l hl
                  · contradiction
                · simp only [↓reduceIte] at h
                  cases hint : st.isIntAt j with
                  | false => simp [j, hint] at h
                  | true =>
                    simp only [j, hint, ↓reduceIte] at h
                    split at h
                    · next hok =>
                      injection h with hst; subst hst
                      simp only [Bool.and_eq_true] at hok
                      have hle : newBound ≤ (imp.ceil : Rat) := decide_eq_true_eq.mp hok.1
                      have hold : ∀ l, st.lbAt j = some l → l ≤ newBound := by
                        intro l hl; have hb := hok.2; unfold optLbLe at hb; rw [hl] at hb
                        exact decide_eq_true_eq.mp hb
                      constructor
                      · intro x hx
                        refine ⟨x, ⟨?_, hx.2.1, hx.2.2⟩, Rat.le_refl⟩
                        refine (state_set_lb_inVarBounds st hcol newBound x).mpr (fun k => ⟨?_, (hx.1 k).2⟩)
                        by_cases hkj : k = ⟨col, hcol⟩
                        · subst hkj; simp only [↓reduceIte]
                          obtain ⟨z, hz⟩ := hx.2.2 j hint
                          have hge : imp ≤ (z : Rat) := hz ▸ hxImp x hx
                          have hcl : (imp.ceil : Rat) ≤ (z : Rat) :=
                            Rat.intCast_le_intCast.mpr (Rat.ceil_le_iff.mpr hge)
                          rw [hz]; exact Rat.le_trans hle hcl
                        · simp only [hkj, ↓reduceIte]; exact (hx.1 k).1
                      · intro x' hx'
                        have hb' := (state_set_lb_inVarBounds st hcol newBound x').mp hx'.1
                        refine ⟨⟨fun k => ⟨?_, (hb' k).2⟩, hx'.2.1, hx'.2.2⟩, rfl⟩
                        intro l hl
                        by_cases hkj : k = ⟨col, hcol⟩
                        · subst hkj
                          have hxN : newBound ≤ x' ⟨col, hcol⟩ := by simpa using (hb' ⟨col, hcol⟩).1
                          exact Rat.le_trans (hold l hl) hxN
                        · have hne := (hb' k).1; simp only [hkj, ↓reduceIte] at hne; exact hne l hl
                    · contradiction
            · -- Upper bound tightening
              simp only [↓reduceIte] at h
              have himp_step :
                  ∀ imp,
                    (if 0 < a then
                      if con.sense == .le || con.sense == .eq then
                        (sumMinAct st con.coeffs (some j)).map (fun m => (con.rhs - m) / a)
                      else none
                    else
                      if con.sense == .ge || con.sense == .eq then
                        (sumMaxAct st con.coeffs (some j)).map (fun M => (con.rhs - M) / a)
                      else none) = some imp →
                    ∀ x : Fin n → Rat, StateFeasible st x → x j ≤ imp := by
                intro imp himp x hx
                have hsat := hx.2.1 ⟨row, hrow⟩ con hcon
                unfold LinCon.sat at hsat
                by_cases hpos : 0 < a
                · simp only [hpos, ↓reduceIte] at himp
                  split at himp
                  · next hs =>
                    cases hm : sumMinAct st con.coeffs (some j) with
                    | none => simp [hm] at himp
                    | some m =>
                      simp only [hm, Option.map_some] at himp
                      injection himp with hEq; subst hEq
                      have hle : con.coeffs.dot x ≤ con.rhs := by
                        rcases Bool.or_eq_true _ _ |>.mp hs with h1 | h1
                        · rw [beq_iff_eq.mp h1] at hsat; exact hsat
                        · rw [beq_iff_eq.mp h1] at hsat; exact hsat ▸ Rat.le_refl
                      have hgeM := sumMinAct_some_le hx.1 hm
                      have h1 : m + a * x j ≤ con.rhs := Rat.le_trans hgeM hle
                      have h2 : a * x j ≤ con.rhs - m := by
                        have := Rat.add_le_add_right (c := -m) |>.mpr h1
                        rw [Rat.add_comm m (a * x j), Rat.add_assoc, Rat.add_neg_cancel, Rat.add_zero, ← Rat.sub_eq_add_neg] at this
                        exact this
                      have hdiv := rat_div_le_div_of_pos h2 hpos
                      rwa [Rat.mul_comm a (x j), rat_div_mul_eq ha0] at hdiv
                  · contradiction
                · have hneg : a < 0 := Rat.lt_of_le_of_ne (Rat.not_lt.mp hpos) ha0
                  simp only [hpos, ↓reduceIte] at himp
                  split at himp
                  · next hs =>
                    cases hM : sumMaxAct st con.coeffs (some j) with
                    | none => simp [hM] at himp
                    | some M =>
                      simp only [hM, Option.map_some] at himp
                      injection himp with hEq; subst hEq
                      have hge : con.rhs ≤ con.coeffs.dot x := by
                        rcases Bool.or_eq_true _ _ |>.mp hs with h1 | h1
                        · rw [beq_iff_eq.mp h1] at hsat; exact hsat
                        · rw [beq_iff_eq.mp h1] at hsat; exact hsat ▸ Rat.le_refl
                      have hleM := sumMaxAct_some_ge hx.1 hM
                      have h1 : con.rhs ≤ M + a * x j := Rat.le_trans hge hleM
                      have h2 : con.rhs - M ≤ a * x j := by
                        have := Rat.add_le_add_right (c := -M) |>.mpr h1
                        rw [Rat.add_comm M (a * x j), Rat.add_assoc, Rat.add_neg_cancel, Rat.add_zero, ← Rat.sub_eq_add_neg] at this
                        exact this
                      have hdiv := rat_div_le_div_of_neg h2 hneg
                      rwa [Rat.mul_comm a (x j), rat_div_mul_eq ha0] at hdiv
                  · contradiction
              generalize himpEq :
                (if 0 < a then
                  if con.sense == .le || con.sense == .eq then
                    (sumMinAct st con.coeffs (some j)).map (fun m => (con.rhs - m) / a)
                  else none
                else
                  if con.sense == .ge || con.sense == .eq then
                    (sumMaxAct st con.coeffs (some j)).map (fun M => (con.rhs - M) / a)
                  else none) = impOpt at h
              cases impOpt with
              | none => simp at h
              | some imp =>
                simp only at h
                have hxImp := himp_step imp himpEq
                cases roundInt
                · simp only [Bool.false_eq_true, ↓reduceIte] at h
                  split at h
                  · next hok =>
                    injection h with hst; subst hst
                    simp only [Bool.and_eq_true] at hok
                    have hle : imp ≤ newBound := decide_eq_true_eq.mp hok.1
                    have hold : ∀ u, st.ubAt j = some u → newBound ≤ u := by
                      intro u hu; have hb := hok.2; unfold optUbLe at hb; rw [hu] at hb
                      exact decide_eq_true_eq.mp hb
                    constructor
                    · intro x hx
                      refine ⟨x, ⟨?_, hx.2.1, hx.2.2⟩, Rat.le_refl⟩
                      refine (state_set_ub_inVarBounds st hcol newBound x).mpr (fun k => ⟨(hx.1 k).1, ?_⟩)
                      by_cases hkj : k = ⟨col, hcol⟩
                      · subst hkj; simp only [↓reduceIte]; exact Rat.le_trans (hxImp x hx) hle
                      · simp only [hkj, ↓reduceIte]; exact (hx.1 k).2
                    · intro x' hx'
                      have hb' := (state_set_ub_inVarBounds st hcol newBound x').mp hx'.1
                      refine ⟨⟨fun k => ⟨(hb' k).1, ?_⟩, hx'.2.1, hx'.2.2⟩, rfl⟩
                      intro u hu
                      by_cases hkj : k = ⟨col, hcol⟩
                      · subst hkj
                        have hxN : x' ⟨col, hcol⟩ ≤ newBound := by simpa using (hb' ⟨col, hcol⟩).2
                        exact Rat.le_trans hxN (hold u hu)
                      · have hne := (hb' k).2; simp only [hkj, ↓reduceIte] at hne; exact hne u hu
                  · contradiction
                · simp only [↓reduceIte] at h
                  cases hint : st.isIntAt j with
                  | false => simp [j, hint] at h
                  | true =>
                    simp only [j, hint, ↓reduceIte] at h
                    split at h
                    · next hok =>
                      injection h with hst; subst hst
                      simp only [Bool.and_eq_true] at hok
                      have hle : (imp.floor : Rat) ≤ newBound := decide_eq_true_eq.mp hok.1
                      have hold : ∀ u, st.ubAt j = some u → newBound ≤ u := by
                        intro u hu; have hb := hok.2; unfold optUbLe at hb; rw [hu] at hb
                        exact decide_eq_true_eq.mp hb
                      constructor
                      · intro x hx
                        refine ⟨x, ⟨?_, hx.2.1, hx.2.2⟩, Rat.le_refl⟩
                        refine (state_set_ub_inVarBounds st hcol newBound x).mpr (fun k => ⟨(hx.1 k).1, ?_⟩)
                        by_cases hkj : k = ⟨col, hcol⟩
                        · subst hkj; simp only [↓reduceIte]
                          obtain ⟨z, hz⟩ := hx.2.2 j hint
                          have hle' : (z : Rat) ≤ imp := hz ▸ hxImp x hx
                          have hfl : (z : Rat) ≤ (imp.floor : Rat) :=
                            Rat.intCast_le_intCast.mpr (Rat.le_floor_iff.mpr hle')
                          rw [hz]; exact Rat.le_trans hfl hle
                        · simp only [hkj, ↓reduceIte]; exact (hx.1 k).2
                      · intro x' hx'
                        have hb' := (state_set_ub_inVarBounds st hcol newBound x').mp hx'.1
                        refine ⟨⟨fun k => ⟨(hb' k).1, ?_⟩, hx'.2.1, hx'.2.2⟩, rfl⟩
                        intro u hu
                        by_cases hkj : k = ⟨col, hcol⟩
                        · subst hkj
                          have hxN : x' ⟨col, hcol⟩ ≤ newBound := by simpa using (hb' ⟨col, hcol⟩).2
                          exact Rat.le_trans hxN (hold u hu)
                        · have hne := (hb' k).2; simp only [hkj, ↓reduceIte] at hne; exact hne u hu
                    · contradiction
        · contradiction
    · contradiction
  | gcdRowDiv row divisor =>
    unfold applyPresolveStep at h; dsimp only at h
    split at h
    · next hrow =>
      split at h
      · next hposInt =>
        cases hcon : st.rows[row] with
        | none => simp [hcon] at h
        | some con =>
          simp only [hcon] at h
          let dRat : Rat := (divisor : Rat)
          have hdPos : 0 < dRat := by
            change ((0 : Int) : Rat) < (divisor : Rat)
            exact Rat.intCast_lt_intCast.mpr hposInt
          have hdNe : dRat ≠ 0 := Rat.ne_of_gt hdPos
          let newCoeffs : RatVec n := RatVec.ofFn (fun j => con.coeffs.get j / dRat)
          have hdotDiv : ∀ x : Fin n → Rat, newCoeffs.dot x = con.coeffs.dot x / dRat := by
            intro x
            unfold RatVec.dot
            have h1 : finSum n (fun j => newCoeffs.get j * x j) =
                finSum n (fun j => (con.coeffs.get j * x j) * dRat⁻¹) := by
              apply finSum_congr; intro j
              simp only [newCoeffs, RatVec.get_ofFn, Rat.div_def, Rat.mul_assoc, Rat.mul_comm dRat⁻¹ (x j)]
            rw [h1, finSum_mul_const, ← Rat.div_def]
          split at h
          · next hintVec =>
            cases hs : con.sense with
            | le =>
              rw [hs] at h; dsimp only at h
              injection h with hst; subst hst
              have hnewSat : ∀ x : Fin n → Rat, StateFeasible st x →
                  ({ coeffs := newCoeffs, sense := .le, rhs := ((con.rhs / dRat).floor : Rat) } : LinCon n).sat x := by
                intro x hx
                have hsat := hx.2.1 ⟨row, hrow⟩ con hcon
                unfold LinCon.sat at hsat ⊢; rw [hs] at hsat
                rw [hdotDiv]
                have hle : con.coeffs.dot x / dRat ≤ con.rhs / dRat := rat_div_le_div_of_pos hsat hdPos
                have hintDot : isRatInt (newCoeffs.dot x) :=
                  isIntegralLinVec_dot_int hintVec ((stateFeasible_toInstance st x).mp hx)
                rw [hdotDiv] at hintDot
                obtain ⟨z, hz⟩ := hintDot
                rw [hz] at hle ⊢
                exact Rat.intCast_le_intCast.mpr (Rat.le_floor_iff.mpr hle)
              constructor
              · intro x hx
                refine ⟨x, ⟨hx.1, fun i r hr => ?_, hx.2.2⟩, Rat.le_refl⟩
                rw [array_set_getElem_fin] at hr
                by_cases hir : i.1 = row
                · simp only [hir, ↓reduceIte] at hr
                  injection hr with hrEq; subst r
                  exact hnewSat x hx
                · simp only [hir, ↓reduceIte] at hr
                  exact hx.2.1 ⟨i.1, by simpa [Array.size_set] using i.2⟩ r hr
              · intro x' hx'
                refine ⟨⟨hx'.1, fun i r hr => ?_, hx'.2.2⟩, rfl⟩
                by_cases hir : i.1 = row
                · have hrEq : r = con := by
                    have hget : st.rows[i] = some con := by
                      change st.rows[i.1] = some con; simpa [hir] using hcon
                    rw [hget] at hr; injection hr with h1; exact h1.symm
                  subst r
                  have hs' := hx'.2.1 ⟨row, by rw [Array.size_set]; exact hrow⟩
                    { coeffs := newCoeffs, sense := .le, rhs := ((con.rhs / dRat).floor : Rat) }
                    (by rw [array_set_getElem_fin]; simp only [↓reduceIte]; rfl)
                  unfold LinCon.sat at hs' ⊢; rw [hs]
                  rw [hdotDiv] at hs'
                  have h1 : con.coeffs.dot x' / dRat ≤ con.rhs / dRat :=
                    Rat.le_trans hs' (rat_floor_le (con.rhs / dRat))
                  have hmul := Rat.mul_le_mul_of_nonneg_right h1 (Rat.le_of_lt hdPos)
                  rwa [Rat.div_mul_cancel hdNe, Rat.div_mul_cancel hdNe] at hmul
                · exact hx'.2.1 ⟨i.1, by rw [Array.size_set]; exact i.2⟩ r
                    (by rw [array_set_getElem_fin]; simp only [hir, ↓reduceIte]; exact hr)
            | ge =>
              rw [hs] at h; dsimp only at h
              injection h with hst; subst hst
              have hnewSat : ∀ x : Fin n → Rat, StateFeasible st x →
                  ({ coeffs := newCoeffs, sense := .ge, rhs := ((con.rhs / dRat).ceil : Rat) } : LinCon n).sat x := by
                intro x hx
                have hsat := hx.2.1 ⟨row, hrow⟩ con hcon
                unfold LinCon.sat at hsat ⊢; rw [hs] at hsat
                rw [hdotDiv]
                have hge : con.rhs / dRat ≤ con.coeffs.dot x / dRat := rat_div_le_div_of_pos hsat hdPos
                have hintDot : isRatInt (newCoeffs.dot x) :=
                  isIntegralLinVec_dot_int hintVec ((stateFeasible_toInstance st x).mp hx)
                rw [hdotDiv] at hintDot
                obtain ⟨z, hz⟩ := hintDot
                rw [hz] at hge ⊢
                exact Rat.intCast_le_intCast.mpr (Rat.ceil_le_iff.mpr hge)
              constructor
              · intro x hx
                refine ⟨x, ⟨hx.1, fun i r hr => ?_, hx.2.2⟩, Rat.le_refl⟩
                rw [array_set_getElem_fin] at hr
                by_cases hir : i.1 = row
                · simp only [hir, ↓reduceIte] at hr
                  injection hr with hrEq; subst r
                  exact hnewSat x hx
                · simp only [hir, ↓reduceIte] at hr
                  exact hx.2.1 ⟨i.1, by simpa [Array.size_set] using i.2⟩ r hr
              · intro x' hx'
                refine ⟨⟨hx'.1, fun i r hr => ?_, hx'.2.2⟩, rfl⟩
                by_cases hir : i.1 = row
                · have hrEq : r = con := by
                    have hget : st.rows[i] = some con := by
                      change st.rows[i.1] = some con; simpa [hir] using hcon
                    rw [hget] at hr; injection hr with h1; exact h1.symm
                  subst r
                  have hs' := hx'.2.1 ⟨row, by rw [Array.size_set]; exact hrow⟩
                    { coeffs := newCoeffs, sense := .ge, rhs := ((con.rhs / dRat).ceil : Rat) }
                    (by rw [array_set_getElem_fin]; simp only [↓reduceIte]; rfl)
                  unfold LinCon.sat at hs' ⊢; rw [hs]
                  rw [hdotDiv] at hs'
                  have h1 : con.rhs / dRat ≤ con.coeffs.dot x' / dRat :=
                    Rat.le_trans (rat_le_ceil (con.rhs / dRat)) hs'
                  have hmul := Rat.mul_le_mul_of_nonneg_right h1 (Rat.le_of_lt hdPos)
                  rwa [Rat.div_mul_cancel hdNe, Rat.div_mul_cancel hdNe] at hmul
                · exact hx'.2.1 ⟨i.1, by rw [Array.size_set]; exact i.2⟩ r
                    (by rw [array_set_getElem_fin]; simp only [hir, ↓reduceIte]; exact hr)
            | eq =>
              rw [hs] at h; dsimp only at h
              split at h
              · injection h with hst; subst hst
                constructor
                · intro x hx
                  refine ⟨x, ⟨hx.1, fun i r hr => ?_, hx.2.2⟩, Rat.le_refl⟩
                  rw [array_set_getElem_fin] at hr
                  by_cases hir : i.1 = row
                  · simp only [hir, ↓reduceIte] at hr
                    injection hr with hrEq; subst r
                    have hsat := hx.2.1 ⟨row, hrow⟩ con hcon
                    unfold LinCon.sat at hsat ⊢; rw [hs] at hsat
                    rw [hdotDiv, hsat]
                  · simp only [hir, ↓reduceIte] at hr
                    exact hx.2.1 ⟨i.1, by simpa [Array.size_set] using i.2⟩ r hr
                · intro x' hx'
                  refine ⟨⟨hx'.1, fun i r hr => ?_, hx'.2.2⟩, rfl⟩
                  by_cases hir : i.1 = row
                  · have hrEq : r = con := by
                      have hget : st.rows[i] = some con := by
                        change st.rows[i.1] = some con; simpa [hir] using hcon
                      rw [hget] at hr; injection hr with h1; exact h1.symm
                    subst r
                    have hs' := hx'.2.1 ⟨row, by rw [Array.size_set]; exact hrow⟩
                      { coeffs := newCoeffs, sense := .eq, rhs := con.rhs / dRat }
                      (by rw [array_set_getElem_fin]; simp only [↓reduceIte]; rfl)
                    unfold LinCon.sat at hs' ⊢; rw [hs]
                    rw [hdotDiv] at hs'
                    have hmul := congrArg (· * dRat) hs'
                    rwa [Rat.div_mul_cancel hdNe, Rat.div_mul_cancel hdNe] at hmul
                  · exact hx'.2.1 ⟨i.1, by rw [Array.size_set]; exact i.2⟩ r
                      (by rw [array_set_getElem_fin]; simp only [hir, ↓reduceIte]; exact hr)
              · contradiction
          · contradiction
      · contradiction
    · contradiction
  | substFixedRow row col val =>
    unfold applyPresolveStep at h; dsimp only at h
    split at h
    · next hrow =>
      split at h
      · next hcol =>
        split at h
        · next hfix =>
          simp only [Bool.and_eq_true, beq_iff_eq] at hfix
          cases hcon : st.rows[row] with
          | none => simp [hcon] at h
          | some con =>
            simp only [hcon] at h
            injection h with hst; subst hst
            let j : Fin n := ⟨col, hcol⟩
            let a := con.coeffs.get j
            let newCoeffs : RatVec n := RatVec.ofFn (fun k => if k = j then 0 else con.coeffs.get k)
            have hdotEq : ∀ x : Fin n → Rat, x j = val →
                newCoeffs.dot x = con.coeffs.dot x - a * val := by
              intro x hxv
              have hsp := dot_split_col con.coeffs j x
              have hnc : newCoeffs.dot x =
                  finSum n (fun k => if some j = some k then 0 else con.coeffs.get k * x k) := by
                unfold RatVec.dot
                apply finSum_congr; intro k
                by_cases hkj : k = j
                · subst hkj; simp [newCoeffs, Rat.zero_mul]
                · have hne : some j ≠ some k := fun e => hkj (Option.some.inj e).symm
                  simp [newCoeffs, hkj, hne]
              rw [hnc, hsp, hxv]
              have hcancel :
                  (finSum n fun k => if some j = some k then 0 else con.coeffs.get k * x k) + a * val - a * val =
                  finSum n fun k => if some j = some k then 0 else con.coeffs.get k * x k := by
                rw [Rat.sub_eq_add_neg, Rat.add_assoc, Rat.add_neg_cancel, Rat.add_zero]
              exact hcancel.symm
            have hsatIff : ∀ x : Fin n → Rat, inVarBounds st.lbAt st.ubAt x →
                (({ coeffs := newCoeffs, sense := con.sense, rhs := con.rhs - a * val } : LinCon n).sat x ↔
                 con.sat x) := by
              intro x hb
              have hxv : x j = val :=
                Rat.le_antisymm ((hb j).2 val hfix.2) ((hb j).1 val hfix.1)
              unfold LinCon.sat
              rw [hdotEq x hxv]
              cases con.sense with
              | le =>
                constructor
                · intro h1
                  have := Rat.add_le_add_right (c := a * val) |>.mpr h1
                  simpa [Rat.sub_eq_add_neg, Rat.add_assoc, Rat.neg_add_cancel, Rat.add_zero] using this
                · intro h1
                  have := Rat.add_le_add_right (c := -(a * val)) |>.mpr h1
                  simpa [Rat.sub_eq_add_neg] using this
              | ge =>
                constructor
                · intro h1
                  have := Rat.add_le_add_right (c := a * val) |>.mpr h1
                  simpa [Rat.sub_eq_add_neg, Rat.add_assoc, Rat.neg_add_cancel, Rat.add_zero] using this
                · intro h1
                  have := Rat.add_le_add_right (c := -(a * val)) |>.mpr h1
                  simpa [Rat.sub_eq_add_neg] using this
              | eq =>
                constructor
                · intro h1
                  have := congrArg (· + a * val) h1
                  simpa [Rat.sub_eq_add_neg, Rat.add_assoc, Rat.neg_add_cancel, Rat.add_zero] using this
                · intro h1; rw [h1]
            constructor
            · intro x hx
              refine ⟨x, ⟨hx.1, fun i r hr => ?_, hx.2.2⟩, Rat.le_refl⟩
              rw [array_set_getElem_fin] at hr
              by_cases hir : i.1 = row
              · simp only [hir, ↓reduceIte] at hr
                injection hr with hrEq; subst r
                exact (hsatIff x hx.1).mpr (hx.2.1 ⟨row, hrow⟩ con hcon)
              · simp only [hir, ↓reduceIte] at hr
                exact hx.2.1 ⟨i.1, by simpa [Array.size_set] using i.2⟩ r hr
            · intro x' hx'
              refine ⟨⟨hx'.1, fun i r hr => ?_, hx'.2.2⟩, rfl⟩
              by_cases hir : i.1 = row
              · have hrEq : r = con := by
                  have hget : st.rows[i] = some con := by
                    change st.rows[i.1] = some con; simpa [hir] using hcon
                  rw [hget] at hr; injection hr with h1; exact h1.symm
                subst r
                have hs' := hx'.2.1 ⟨row, by rw [Array.size_set]; exact hrow⟩
                  { coeffs := newCoeffs, sense := con.sense, rhs := con.rhs - a * val }
                  (by rw [array_set_getElem_fin]; simp only [↓reduceIte]; rfl)
                exact (hsatIff x' hx'.1).mp hs'
              · exact hx'.2.1 ⟨i.1, by rw [Array.size_set]; exact i.2⟩ r
                  (by rw [array_set_getElem_fin]; simp only [hir, ↓reduceIte]; exact hr)
        · contradiction
      · contradiction
    · contradiction
  | substFixedObj col val =>
    unfold applyPresolveStep at h; dsimp only at h
    split at h
    · next hcol =>
      split at h
      · next hfix =>
        simp only [Bool.and_eq_true, beq_iff_eq] at hfix
        injection h with hst; subst hst
        let j : Fin n := ⟨col, hcol⟩
        have hobjEq : ∀ x : Fin n → Rat, inVarBounds st.lbAt st.ubAt x →
            StateObjective { st with
              obj := RatVec.ofFn (fun k => if k = j then 0 else st.obj.get k),
              objConst := st.objConst + st.obj.get j * val } x =
            StateObjective st x := by
          intro x hb
          have hxv : x j = val :=
            Rat.le_antisymm ((hb j).2 val hfix.2) ((hb j).1 val hfix.1)
          unfold StateObjective
          have hsp : finSum n (fun k => st.obj.get k * x k) =
              finSum n (fun k => (if k = j then 0 else st.obj.get k) * x k) + st.obj.get j * val := by
            have h1 : ∀ k : Fin n, st.obj.get k * x k =
                (if k = j then 0 else st.obj.get k) * x k + (if k = j then st.obj.get j * val else 0) := by
              intro k
              by_cases hkj : k = j
              · subst hkj; simp [hxv, Rat.zero_mul, Rat.zero_add]
              · simp [hkj, Rat.add_zero]
            rw [finSum_congr h1, finSum_add, finSum_single]
          simp only [RatVec.get_ofFn]
          rw [hsp, Rat.add_assoc, Rat.add_comm (st.obj.get j * val)]
        constructor
        · intro x hx
          refine ⟨x, hx, ?_⟩
          rw [hobjEq x hx.1]; exact Rat.le_refl
        · intro x' hx'
          exact ⟨hx', (hobjEq x' hx'.1).symm⟩
      · contradiction
    · contradiction
  | singletonRowElim row col roundInt =>
    unfold applyPresolveStep at h; dsimp only at h
    split at h
    · next hrow =>
      split at h
      · next hcol =>
        let j : Fin n := ⟨col, hcol⟩
        cases hcon : st.rows[row] with
        | none => simp [hcon] at h
        | some con =>
          simp only [hcon] at h
          let a : Rat := con.coeffs.get j
          split at h
          · next hok =>
            simp only [Bool.and_eq_true, Bool.not_eq_true'] at hok
            have ha0 : a ≠ 0 := fun heq => by
              have hb : (a == 0) = true := beq_iff_eq.mpr heq
              rw [hb] at hok; exact Bool.noConfusion hok.1
            have hzero := checkOnlyColNonzero_loop_sound con.coeffs j 0 (Nat.zero_le n)
              (fun _ hi => by omega) hok.2
            have hdot : ∀ x : Fin n → Rat, con.coeffs.dot x = a * x j :=
              fun x => dot_singleton hzero x
            let b : Rat := con.rhs / a
            cases hs : con.sense with
            | eq =>
              rw [hs] at h; dsimp only at h
              split at h
              · contradiction
              · injection h with hst; subst hst
                have hsatIff : ∀ x : Fin n → Rat, con.sat x ↔ (b ≤ x j ∧ x j ≤ b) := by
                  intro x; unfold LinCon.sat; rw [hs, hdot]
                  constructor
                  · intro heq
                    have hxEq : x j = b := by
                      have := congrArg (· / a) heq
                      rwa [Rat.mul_comm a (x j), rat_div_mul_eq ha0] at this
                    rw [hxEq]; exact ⟨Rat.le_refl, Rat.le_refl⟩
                  · rintro ⟨h1, h2⟩
                    have hxEq : x j = b := Rat.le_antisymm h2 h1
                    rw [hxEq]; exact rat_mul_div_cancel_left ha0
                have hlbub : ∀ x : Fin n → Rat,
                    inVarBounds
                      (PresolveState.lbAt { st with
                        lb := st.lb.set col (tightenLBOpt (st.lbAt j) b) (by rw [st.lb_size]; exact hcol)
                        lb_size := by rw [Array.size_set]; exact st.lb_size
                        ub := st.ub.set col (tightenUBOpt (st.ubAt j) b) (by rw [st.ub_size]; exact hcol)
                        ub_size := by rw [Array.size_set]; exact st.ub_size
                        rows := st.rows.set row none hrow })
                      (PresolveState.ubAt { st with
                        lb := st.lb.set col (tightenLBOpt (st.lbAt j) b) (by rw [st.lb_size]; exact hcol)
                        lb_size := by rw [Array.size_set]; exact st.lb_size
                        ub := st.ub.set col (tightenUBOpt (st.ubAt j) b) (by rw [st.ub_size]; exact hcol)
                        ub_size := by rw [Array.size_set]; exact st.ub_size
                        rows := st.rows.set row none hrow }) x ↔
                    (inVarBounds st.lbAt st.ubAt x ∧ b ≤ x j ∧ x j ≤ b) := by
                  intro x
                  let stMid : PresolveState n := { st with
                    lb := st.lb.set col (tightenLBOpt (st.lbAt j) b) (by rw [st.lb_size]; exact hcol)
                    lb_size := by rw [Array.size_set]; exact st.lb_size }
                  have h1 := state_set_ub_opt_inVarBounds stMid hcol b x
                  have h2 := state_set_lb_opt_inVarBounds st hcol b x
                  dsimp only at h1 h2
                  have hstEq : inVarBounds
                      (PresolveState.lbAt { st with
                        lb := st.lb.set col (tightenLBOpt (st.lbAt j) b) (by rw [st.lb_size]; exact hcol)
                        lb_size := by rw [Array.size_set]; exact st.lb_size
                        ub := st.ub.set col (tightenUBOpt (st.ubAt j) b) (by rw [st.ub_size]; exact hcol)
                        ub_size := by rw [Array.size_set]; exact st.ub_size
                        rows := st.rows.set row none hrow })
                      (PresolveState.ubAt { st with
                        lb := st.lb.set col (tightenLBOpt (st.lbAt j) b) (by rw [st.lb_size]; exact hcol)
                        lb_size := by rw [Array.size_set]; exact st.lb_size
                        ub := st.ub.set col (tightenUBOpt (st.ubAt j) b) (by rw [st.ub_size]; exact hcol)
                        ub_size := by rw [Array.size_set]; exact st.ub_size
                        rows := st.rows.set row none hrow }) x ↔
                      inVarBounds
                        (PresolveState.lbAt { stMid with
                          ub := stMid.ub.set col (tightenUBOpt (stMid.ubAt ⟨col, hcol⟩) b) (by rw [stMid.ub_size]; exact hcol)
                          ub_size := by rw [Array.size_set]; exact stMid.ub_size })
                        (PresolveState.ubAt { stMid with
                          ub := stMid.ub.set col (tightenUBOpt (stMid.ubAt ⟨col, hcol⟩) b) (by rw [stMid.ub_size]; exact hcol)
                          ub_size := by rw [Array.size_set]; exact stMid.ub_size }) x := Iff.rfl
                  rw [hstEq, h1, h2]
                  exact ⟨fun ⟨⟨hb, hl⟩, hu⟩ => ⟨hb, hl, hu⟩, fun ⟨hb, hl, hu⟩ => ⟨⟨hb, hl⟩, hu⟩⟩
                constructor
                · intro x hx
                  refine ⟨x, ⟨(hlbub x).mpr ⟨hx.1, (hsatIff x).mp (hx.2.1 ⟨row, hrow⟩ con hcon)⟩,
                    fun i r hr => ?_, hx.2.2⟩, Rat.le_refl⟩
                  rw [array_set_getElem_fin] at hr
                  by_cases hir : i.1 = row
                  · simp [hir] at hr
                  · simp only [hir, ↓reduceIte] at hr
                    exact hx.2.1 ⟨i.1, by simpa [Array.size_set] using i.2⟩ r hr
                · intro x' hx'
                  obtain ⟨hb, hl, hu⟩ := (hlbub x').mp hx'.1
                  refine ⟨⟨hb, fun i r hr => ?_, hx'.2.2⟩, rfl⟩
                  by_cases hir : i.1 = row
                  · have hrEq : r = con := by
                      have hget : st.rows[i] = some con := by
                        change st.rows[i.1] = some con; simpa [hir] using hcon
                      rw [hget] at hr; injection hr with h1; exact h1.symm
                    subst r; exact (hsatIff x').mpr ⟨hl, hu⟩
                  · exact hx'.2.1 ⟨i.1, by rw [Array.size_set]; exact i.2⟩ r
                      (by rw [array_set_getElem_fin]; simp only [hir, ↓reduceIte]; exact hr)
            | le =>
              rw [hs] at h; dsimp only at h
              by_cases hpos : 0 < a
              · simp only [a, j, hpos, ↓reduceIte] at h
                have hsatIff : ∀ x : Fin n → Rat, con.sat x ↔ x j ≤ b := by
                  intro x; unfold LinCon.sat; rw [hs, hdot]
                  constructor
                  · intro hle
                    have hdiv := rat_div_le_div_of_pos hle hpos
                    rwa [Rat.mul_comm a (x j), rat_div_mul_eq ha0] at hdiv
                  · intro hle
                    have hmul := Rat.mul_le_mul_of_nonneg_left hle (Rat.le_of_lt hpos)
                    rwa [rat_mul_div_cancel_left ha0] at hmul
                cases roundInt
                · simp only [Bool.false_eq_true, ↓reduceIte] at h
                  injection h with hst; subst hst
                  constructor
                  · intro x hx
                    refine ⟨x, ⟨(state_set_ub_opt_inVarBounds st hcol b x).mpr
                      ⟨hx.1, (hsatIff x).mp (hx.2.1 ⟨row, hrow⟩ con hcon)⟩,
                      fun i r hr => ?_, hx.2.2⟩, Rat.le_refl⟩
                    rw [array_set_getElem_fin] at hr
                    by_cases hir : i.1 = row
                    · simp [hir] at hr
                    · simp only [hir, ↓reduceIte] at hr
                      exact hx.2.1 ⟨i.1, by simpa [Array.size_set] using i.2⟩ r hr
                  · intro x' hx'
                    obtain ⟨hb, hu⟩ := (state_set_ub_opt_inVarBounds st hcol b x').mp hx'.1
                    refine ⟨⟨hb, fun i r hr => ?_, hx'.2.2⟩, rfl⟩
                    by_cases hir : i.1 = row
                    · have hrEq : r = con := by
                        have hget : st.rows[i] = some con := by
                          change st.rows[i.1] = some con; simpa [hir] using hcon
                        rw [hget] at hr; injection hr with h1; exact h1.symm
                      subst r; exact (hsatIff x').mpr hu
                    · exact hx'.2.1 ⟨i.1, by rw [Array.size_set]; exact i.2⟩ r
                        (by rw [array_set_getElem_fin]; simp only [hir, ↓reduceIte]; exact hr)
                · simp only [↓reduceIte] at h
                  cases hint : st.isIntAt ⟨col, hcol⟩ with
                  | false => simp [hint] at h
                  | true =>
                    simp only [hint, ↓reduceIte] at h; injection h with hst; subst hst
                    constructor
                    · intro x hx
                      obtain ⟨z, hz⟩ := hx.2.2 j hint
                      have hle : (z : Rat) ≤ b := hz ▸ (hsatIff x).mp (hx.2.1 ⟨row, hrow⟩ con hcon)
                      have hfl : x j ≤ (b.floor : Rat) := by
                        rw [hz]; exact Rat.intCast_le_intCast.mpr (Rat.le_floor_iff.mpr hle)
                      refine ⟨x, ⟨(state_set_ub_opt_inVarBounds st hcol (b.floor : Rat) x).mpr ⟨hx.1, hfl⟩,
                        fun i r hr => ?_, hx.2.2⟩, Rat.le_refl⟩
                      rw [array_set_getElem_fin] at hr
                      by_cases hir : i.1 = row
                      · simp [hir] at hr
                      · simp only [hir, ↓reduceIte] at hr
                        exact hx.2.1 ⟨i.1, by simpa [Array.size_set] using i.2⟩ r hr
                    · intro x' hx'
                      obtain ⟨hb, hu⟩ := (state_set_ub_opt_inVarBounds st hcol (b.floor : Rat) x').mp hx'.1
                      refine ⟨⟨hb, fun i r hr => ?_, hx'.2.2⟩, rfl⟩
                      by_cases hir : i.1 = row
                      · have hrEq : r = con := by
                          have hget : st.rows[i] = some con := by
                            change st.rows[i.1] = some con; simpa [hir] using hcon
                          rw [hget] at hr; injection hr with h1; exact h1.symm
                        subst r; exact (hsatIff x').mpr (Rat.le_trans hu (rat_floor_le b))
                      · exact hx'.2.1 ⟨i.1, by rw [Array.size_set]; exact i.2⟩ r
                          (by rw [array_set_getElem_fin]; simp only [hir, ↓reduceIte]; exact hr)
              · have hneg : a < 0 := Rat.lt_of_le_of_ne (Rat.not_lt.mp hpos) ha0
                simp only [a, j, hpos, ↓reduceIte] at h
                have hsatIff : ∀ x : Fin n → Rat, con.sat x ↔ b ≤ x j := by
                  intro x; unfold LinCon.sat; rw [hs, hdot]
                  constructor
                  · intro hle
                    have hdiv := rat_div_le_div_of_neg hle hneg
                    rwa [Rat.mul_comm a (x j), rat_div_mul_eq ha0] at hdiv
                  · intro hge
                    have hmul := rat_mul_le_mul_of_nonpos_left hge (Rat.le_of_lt hneg)
                    rwa [rat_mul_div_cancel_left ha0] at hmul
                cases roundInt
                · simp only [Bool.false_eq_true, ↓reduceIte] at h
                  injection h with hst; subst hst
                  constructor
                  · intro x hx
                    refine ⟨x, ⟨(state_set_lb_opt_inVarBounds st hcol b x).mpr
                      ⟨hx.1, (hsatIff x).mp (hx.2.1 ⟨row, hrow⟩ con hcon)⟩,
                      fun i r hr => ?_, hx.2.2⟩, Rat.le_refl⟩
                    rw [array_set_getElem_fin] at hr
                    by_cases hir : i.1 = row
                    · simp [hir] at hr
                    · simp only [hir, ↓reduceIte] at hr
                      exact hx.2.1 ⟨i.1, by simpa [Array.size_set] using i.2⟩ r hr
                  · intro x' hx'
                    obtain ⟨hb, hl⟩ := (state_set_lb_opt_inVarBounds st hcol b x').mp hx'.1
                    refine ⟨⟨hb, fun i r hr => ?_, hx'.2.2⟩, rfl⟩
                    by_cases hir : i.1 = row
                    · have hrEq : r = con := by
                        have hget : st.rows[i] = some con := by
                          change st.rows[i.1] = some con; simpa [hir] using hcon
                        rw [hget] at hr; injection hr with h1; exact h1.symm
                      subst r; exact (hsatIff x').mpr hl
                    · exact hx'.2.1 ⟨i.1, by rw [Array.size_set]; exact i.2⟩ r
                        (by rw [array_set_getElem_fin]; simp only [hir, ↓reduceIte]; exact hr)
                · simp only [↓reduceIte] at h
                  cases hint : st.isIntAt ⟨col, hcol⟩ with
                  | false => simp [hint] at h
                  | true =>
                    simp only [hint, ↓reduceIte] at h; injection h with hst; subst hst
                    constructor
                    · intro x hx
                      obtain ⟨z, hz⟩ := hx.2.2 j hint
                      have hge : b ≤ (z : Rat) := hz ▸ (hsatIff x).mp (hx.2.1 ⟨row, hrow⟩ con hcon)
                      have hcl : (b.ceil : Rat) ≤ x j := by
                        rw [hz]; exact Rat.intCast_le_intCast.mpr (Rat.ceil_le_iff.mpr hge)
                      refine ⟨x, ⟨(state_set_lb_opt_inVarBounds st hcol (b.ceil : Rat) x).mpr ⟨hx.1, hcl⟩,
                        fun i r hr => ?_, hx.2.2⟩, Rat.le_refl⟩
                      rw [array_set_getElem_fin] at hr
                      by_cases hir : i.1 = row
                      · simp [hir] at hr
                      · simp only [hir, ↓reduceIte] at hr
                        exact hx.2.1 ⟨i.1, by simpa [Array.size_set] using i.2⟩ r hr
                    · intro x' hx'
                      obtain ⟨hb, hl⟩ := (state_set_lb_opt_inVarBounds st hcol (b.ceil : Rat) x').mp hx'.1
                      refine ⟨⟨hb, fun i r hr => ?_, hx'.2.2⟩, rfl⟩
                      by_cases hir : i.1 = row
                      · have hrEq : r = con := by
                          have hget : st.rows[i] = some con := by
                            change st.rows[i.1] = some con; simpa [hir] using hcon
                          rw [hget] at hr; injection hr with h1; exact h1.symm
                        subst r; exact (hsatIff x').mpr (Rat.le_trans (rat_le_ceil b) hl)
                      · exact hx'.2.1 ⟨i.1, by rw [Array.size_set]; exact i.2⟩ r
                          (by rw [array_set_getElem_fin]; simp only [hir, ↓reduceIte]; exact hr)
            | ge =>
              rw [hs] at h; dsimp only at h
              by_cases hpos : 0 < a
              · simp only [a, j, hpos, ↓reduceIte] at h
                have hsatIff : ∀ x : Fin n → Rat, con.sat x ↔ b ≤ x j := by
                  intro x; unfold LinCon.sat; rw [hs, hdot]
                  constructor
                  · intro hge
                    have hdiv := rat_div_le_div_of_pos hge hpos
                    rwa [Rat.mul_comm a (x j), rat_div_mul_eq ha0] at hdiv
                  · intro hge
                    have hmul := Rat.mul_le_mul_of_nonneg_left hge (Rat.le_of_lt hpos)
                    rwa [rat_mul_div_cancel_left ha0] at hmul
                cases roundInt
                · simp only [Bool.false_eq_true, ↓reduceIte] at h
                  injection h with hst; subst hst
                  constructor
                  · intro x hx
                    refine ⟨x, ⟨(state_set_lb_opt_inVarBounds st hcol b x).mpr
                      ⟨hx.1, (hsatIff x).mp (hx.2.1 ⟨row, hrow⟩ con hcon)⟩,
                      fun i r hr => ?_, hx.2.2⟩, Rat.le_refl⟩
                    rw [array_set_getElem_fin] at hr
                    by_cases hir : i.1 = row
                    · simp [hir] at hr
                    · simp only [hir, ↓reduceIte] at hr
                      exact hx.2.1 ⟨i.1, by simpa [Array.size_set] using i.2⟩ r hr
                  · intro x' hx'
                    obtain ⟨hb, hl⟩ := (state_set_lb_opt_inVarBounds st hcol b x').mp hx'.1
                    refine ⟨⟨hb, fun i r hr => ?_, hx'.2.2⟩, rfl⟩
                    by_cases hir : i.1 = row
                    · have hrEq : r = con := by
                        have hget : st.rows[i] = some con := by
                          change st.rows[i.1] = some con; simpa [hir] using hcon
                        rw [hget] at hr; injection hr with h1; exact h1.symm
                      subst r; exact (hsatIff x').mpr hl
                    · exact hx'.2.1 ⟨i.1, by rw [Array.size_set]; exact i.2⟩ r
                        (by rw [array_set_getElem_fin]; simp only [hir, ↓reduceIte]; exact hr)
                · simp only [↓reduceIte] at h
                  cases hint : st.isIntAt ⟨col, hcol⟩ with
                  | false => simp [hint] at h
                  | true =>
                    simp only [hint, ↓reduceIte] at h; injection h with hst; subst hst
                    constructor
                    · intro x hx
                      obtain ⟨z, hz⟩ := hx.2.2 j hint
                      have hge : b ≤ (z : Rat) := hz ▸ (hsatIff x).mp (hx.2.1 ⟨row, hrow⟩ con hcon)
                      have hcl : (b.ceil : Rat) ≤ x j := by
                        rw [hz]; exact Rat.intCast_le_intCast.mpr (Rat.ceil_le_iff.mpr hge)
                      refine ⟨x, ⟨(state_set_lb_opt_inVarBounds st hcol (b.ceil : Rat) x).mpr ⟨hx.1, hcl⟩,
                        fun i r hr => ?_, hx.2.2⟩, Rat.le_refl⟩
                      rw [array_set_getElem_fin] at hr
                      by_cases hir : i.1 = row
                      · simp [hir] at hr
                      · simp only [hir, ↓reduceIte] at hr
                        exact hx.2.1 ⟨i.1, by simpa [Array.size_set] using i.2⟩ r hr
                    · intro x' hx'
                      obtain ⟨hb, hl⟩ := (state_set_lb_opt_inVarBounds st hcol (b.ceil : Rat) x').mp hx'.1
                      refine ⟨⟨hb, fun i r hr => ?_, hx'.2.2⟩, rfl⟩
                      by_cases hir : i.1 = row
                      · have hrEq : r = con := by
                          have hget : st.rows[i] = some con := by
                            change st.rows[i.1] = some con; simpa [hir] using hcon
                          rw [hget] at hr; injection hr with h1; exact h1.symm
                        subst r; exact (hsatIff x').mpr (Rat.le_trans (rat_le_ceil b) hl)
                      · exact hx'.2.1 ⟨i.1, by rw [Array.size_set]; exact i.2⟩ r
                          (by rw [array_set_getElem_fin]; simp only [hir, ↓reduceIte]; exact hr)
              · have hneg : a < 0 := Rat.lt_of_le_of_ne (Rat.not_lt.mp hpos) ha0
                simp only [a, j, hpos, ↓reduceIte] at h
                have hsatIff : ∀ x : Fin n → Rat, con.sat x ↔ x j ≤ b := by
                  intro x; unfold LinCon.sat; rw [hs, hdot]
                  constructor
                  · intro hge
                    have hdiv := rat_div_le_div_of_neg hge hneg
                    rwa [Rat.mul_comm a (x j), rat_div_mul_eq ha0] at hdiv
                  · intro hle
                    have hmul := rat_mul_le_mul_of_nonpos_left hle (Rat.le_of_lt hneg)
                    rwa [rat_mul_div_cancel_left ha0] at hmul
                cases roundInt
                · simp only [Bool.false_eq_true, ↓reduceIte] at h
                  injection h with hst; subst hst
                  constructor
                  · intro x hx
                    refine ⟨x, ⟨(state_set_ub_opt_inVarBounds st hcol b x).mpr
                      ⟨hx.1, (hsatIff x).mp (hx.2.1 ⟨row, hrow⟩ con hcon)⟩,
                      fun i r hr => ?_, hx.2.2⟩, Rat.le_refl⟩
                    rw [array_set_getElem_fin] at hr
                    by_cases hir : i.1 = row
                    · simp [hir] at hr
                    · simp only [hir, ↓reduceIte] at hr
                      exact hx.2.1 ⟨i.1, by simpa [Array.size_set] using i.2⟩ r hr
                  · intro x' hx'
                    obtain ⟨hb, hu⟩ := (state_set_ub_opt_inVarBounds st hcol b x').mp hx'.1
                    refine ⟨⟨hb, fun i r hr => ?_, hx'.2.2⟩, rfl⟩
                    by_cases hir : i.1 = row
                    · have hrEq : r = con := by
                        have hget : st.rows[i] = some con := by
                          change st.rows[i.1] = some con; simpa [hir] using hcon
                        rw [hget] at hr; injection hr with h1; exact h1.symm
                      subst r; exact (hsatIff x').mpr hu
                    · exact hx'.2.1 ⟨i.1, by rw [Array.size_set]; exact i.2⟩ r
                        (by rw [array_set_getElem_fin]; simp only [hir, ↓reduceIte]; exact hr)
                · simp only [↓reduceIte] at h
                  cases hint : st.isIntAt ⟨col, hcol⟩ with
                  | false => simp [hint] at h
                  | true =>
                    simp only [hint, ↓reduceIte] at h; injection h with hst; subst hst
                    constructor
                    · intro x hx
                      obtain ⟨z, hz⟩ := hx.2.2 j hint
                      have hle : (z : Rat) ≤ b := hz ▸ (hsatIff x).mp (hx.2.1 ⟨row, hrow⟩ con hcon)
                      have hfl : x j ≤ (b.floor : Rat) := by
                        rw [hz]; exact Rat.intCast_le_intCast.mpr (Rat.le_floor_iff.mpr hle)
                      refine ⟨x, ⟨(state_set_ub_opt_inVarBounds st hcol (b.floor : Rat) x).mpr ⟨hx.1, hfl⟩,
                        fun i r hr => ?_, hx.2.2⟩, Rat.le_refl⟩
                      rw [array_set_getElem_fin] at hr
                      by_cases hir : i.1 = row
                      · simp [hir] at hr
                      · simp only [hir, ↓reduceIte] at hr
                        exact hx.2.1 ⟨i.1, by simpa [Array.size_set] using i.2⟩ r hr
                    · intro x' hx'
                      obtain ⟨hb, hu⟩ := (state_set_ub_opt_inVarBounds st hcol (b.floor : Rat) x').mp hx'.1
                      refine ⟨⟨hb, fun i r hr => ?_, hx'.2.2⟩, rfl⟩
                      by_cases hir : i.1 = row
                      · have hrEq : r = con := by
                          have hget : st.rows[i] = some con := by
                            change st.rows[i.1] = some con; simpa [hir] using hcon
                          rw [hget] at hr; injection hr with h1; exact h1.symm
                        subst r; exact (hsatIff x').mpr (Rat.le_trans hu (rat_floor_le b))
                      · exact hx'.2.1 ⟨i.1, by rw [Array.size_set]; exact i.2⟩ r
                          (by rw [array_set_getElem_fin]; simp only [hir, ↓reduceIte]; exact hr)
          · contradiction
      · contradiction
    · contradiction
  | redundantRow row =>
    unfold applyPresolveStep at h; dsimp only at h
    split at h
    · next hrow =>
      cases hcon : st.rows[row] with
      | none => simp [hcon] at h
      | some con =>
        simp only [hcon] at h
        generalize hisRedEq :
          (match con.sense with
          | .le =>
            match sumMaxAct st con.coeffs none with
            | some M => decide (M ≤ con.rhs)
            | none => false
          | .ge =>
            match sumMinAct st con.coeffs none with
            | some m => decide (con.rhs ≤ m)
            | none => false
          | .eq =>
            match sumMinAct st con.coeffs none, sumMaxAct st con.coeffs none with
            | some m, some M => decide (con.rhs ≤ m) && decide (M ≤ con.rhs)
            | _, _ => false) = isRed at h
        cases hokB : isRed with
        | false => simp [hokB] at h
        | true =>
          simp only [hokB, ↓reduceIte] at h
          injection h with hst; subst hst
          have hok :
              (match con.sense with
              | .le =>
                match sumMaxAct st con.coeffs none with
                | some M => decide (M ≤ con.rhs)
                | none => false
              | .ge =>
                match sumMinAct st con.coeffs none with
                | some m => decide (con.rhs ≤ m)
                | none => false
              | .eq =>
                match sumMinAct st con.coeffs none, sumMaxAct st con.coeffs none with
                | some m, some M => decide (con.rhs ≤ m) && decide (M ≤ con.rhs)
                | _, _ => false) = true := hisRedEq.trans hokB
          have himplied : ∀ x' : Fin n → Rat, inVarBounds st.lbAt st.ubAt x' → con.sat x' := by
            intro x' hb
            unfold LinCon.sat
            cases hs : con.sense with
            | le =>
              rw [hs] at hok
              cases hM : sumMaxAct st con.coeffs none with
              | none => simp [hM] at hok
              | some M =>
                rw [hM] at hok
                exact Rat.le_trans (sumMaxAct_none_ge hb hM) (decide_eq_true_eq.mp hok)
            | ge =>
              rw [hs] at hok
              cases hm : sumMinAct st con.coeffs none with
              | none => simp [hm] at hok
              | some m =>
                rw [hm] at hok
                exact Rat.le_trans (decide_eq_true_eq.mp hok) (sumMinAct_none_le hb hm)
            | eq =>
              rw [hs] at hok
              cases hm : sumMinAct st con.coeffs none with
              | none => simp [hm] at hok
              | some m =>
                cases hM : sumMaxAct st con.coeffs none with
                | none => simp [hm, hM] at hok
                | some M =>
                  rw [hm, hM] at hok
                  simp only [Bool.and_eq_true] at hok
                  exact Rat.le_antisymm
                    (Rat.le_trans (sumMaxAct_none_ge hb hM) (decide_eq_true_eq.mp hok.2))
                    (Rat.le_trans (decide_eq_true_eq.mp hok.1) (sumMinAct_none_le hb hm))
          constructor
          · intro x hx
            refine ⟨x, ⟨hx.1, fun i r hr => ?_, hx.2.2⟩, Rat.le_refl⟩
            rw [array_set_getElem_fin] at hr
            by_cases hir : i.1 = row
            · simp [hir] at hr
            · simp only [hir, ↓reduceIte] at hr
              exact hx.2.1 ⟨i.1, by simpa [Array.size_set] using i.2⟩ r hr
          · intro x' hx'
            refine ⟨⟨hx'.1, fun i r hr => ?_, hx'.2.2⟩, rfl⟩
            by_cases hir : i.1 = row
            · have hrEq : r = con := by
                have hget : st.rows[i] = some con := by
                  change st.rows[i.1] = some con; simpa [hir] using hcon
                rw [hget] at hr; injection hr with h1; exact h1.symm
              subst r; exact himplied x' hx'.1
            · exact hx'.2.1 ⟨i.1, by rw [Array.size_set]; exact i.2⟩ r
                (by rw [array_set_getElem_fin]; simp only [hir, ↓reduceIte]; exact hr)
    · contradiction
  | binStrengthen row col delta =>
    unfold applyPresolveStep at h; dsimp only at h
    split at h
    · next hrow =>
      split at h
      · next hcol =>
        let j : Fin n := ⟨col, hcol⟩
        split at h
        · next hbin =>
          simp only [Bool.and_eq_true, beq_iff_eq] at hbin
          rcases hbin with ⟨⟨⟨hint, hlb0⟩, hub1⟩, hdelB⟩
          have hdel : 0 ≤ delta := decide_eq_true_eq.mp hdelB
          cases hcon : st.rows[row] with
          | none => simp [hcon] at h
          | some con =>
            simp only [hcon] at h
            split at h
            · next hleSense =>
              have hsEq : con.sense = .le := beq_iff_eq.mp hleSense
              let a : Rat := con.coeffs.get j
              by_cases ha0 : a = 0
              · have ha0b : (a == 0) = true := beq_iff_eq.mpr ha0
                simp only [a, j, ha0b, ↓reduceIte] at h; contradiction
              · have ha0b : (a == 0) = false := beq_false_of_ne ha0
                simp only [a, j, ha0b, Bool.false_eq_true, ↓reduceIte] at h
                cases hM : sumMaxAct st con.coeffs (some ⟨col, hcol⟩) with
                | none => simp [hM] at h
                | some maxExcl =>
                  simp only [hM] at h
                  by_cases hpos : 0 < a
                  · simp only [a, j, hpos, ↓reduceIte] at h
                    split at h
                    · next hMle =>
                      injection h with hst; subst hst
                      have hMle' : maxExcl ≤ con.rhs - delta := decide_eq_true_eq.mp hMle
                      let newCoeffs : RatVec n :=
                        RatVec.ofFn (fun k => if k = j then a - delta else con.coeffs.get k)
                      have hdotDiff : ∀ x : Fin n → Rat,
                          newCoeffs.dot x = con.coeffs.dot x - delta * x j := by
                        intro x
                        rw [dot_split_col newCoeffs j x, dot_split_col con.coeffs j x]
                        have hrest :
                            finSum n (fun k => if some j = some k then 0 else newCoeffs.get k * x k) =
                            finSum n (fun k => if some j = some k then 0 else con.coeffs.get k * x k) := by
                          apply finSum_congr; intro k
                          by_cases hkj : k = j
                          · subst hkj; simp
                          · have hne : some j ≠ some k := fun e => hkj (Option.some.inj e).symm
                            simp [newCoeffs, hkj, hne]
                        rw [hrest]
                        have hgetJ : newCoeffs.get j = a - delta := by simp [newCoeffs]
                        rw [hgetJ]
                        change _ + (a - delta) * x j = _ + a * x j - delta * x j
                        rw [Rat.sub_eq_add_neg, Rat.add_mul, Rat.neg_mul, ← Rat.add_assoc, ← Rat.sub_eq_add_neg]
                      constructor
                      · intro x hx
                        have hbin01 := rat_binary_zero_or_one ((hx.1 j).1 0 hlb0) ((hx.1 j).2 1 hub1) (hx.2.2 j hint)
                        have hnewSat : ({ coeffs := newCoeffs, sense := .le, rhs := con.rhs - delta } : LinCon n).sat x := by
                          unfold LinCon.sat; dsimp only
                          rcases hbin01 with hx0 | hx1
                          · rw [dot_split_col newCoeffs j x, hx0, Rat.mul_zero, Rat.add_zero]
                            have hrest :
                                finSum n (fun k => if some j = some k then 0 else newCoeffs.get k * x k) =
                                finSum n (fun k => if some j = some k then 0 else con.coeffs.get k * x k) := by
                              apply finSum_congr; intro k
                              by_cases hkj : k = j
                              · subst hkj; simp
                              · have hne : some j ≠ some k := fun e => hkj (Option.some.inj e).symm
                                simp [newCoeffs, hkj, hne]
                            rw [hrest]
                            have hleM := sumMaxAct_go_ge st con.coeffs (some j) x hx.1 0 (Nat.zero_le n) 0 maxExcl Rat.le_refl hM
                            exact Rat.le_trans hleM hMle'
                          · rw [hdotDiff, hx1, Rat.mul_one]
                            have hsat := hx.2.1 ⟨row, hrow⟩ con hcon
                            unfold LinCon.sat at hsat; rw [hsEq] at hsat
                            rw [Rat.sub_eq_add_neg, Rat.sub_eq_add_neg]
                            exact Rat.add_le_add_right.mpr hsat
                        refine ⟨x, ⟨hx.1, fun i r hr => ?_, hx.2.2⟩, Rat.le_refl⟩
                        rw [array_set_getElem_fin] at hr
                        by_cases hir : i.1 = row
                        · simp only [hir, ↓reduceIte] at hr
                          injection hr with hrEq; subst r; exact hnewSat
                        · simp only [hir, ↓reduceIte] at hr
                          exact hx.2.1 ⟨i.1, by simpa [Array.size_set] using i.2⟩ r hr
                      · intro x' hx'
                        refine ⟨⟨hx'.1, fun i r hr => ?_, hx'.2.2⟩, rfl⟩
                        by_cases hir : i.1 = row
                        · have hrEq : r = con := by
                            have hget : st.rows[i] = some con := by
                              change st.rows[i.1] = some con; simpa [hir] using hcon
                            rw [hget] at hr; injection hr with h1; exact h1.symm
                          subst r
                          have hs' := hx'.2.1 ⟨row, by rw [Array.size_set]; exact hrow⟩
                            { coeffs := newCoeffs, sense := .le, rhs := con.rhs - delta }
                            (by rw [array_set_getElem_fin]; simp only [↓reduceIte]; rfl)
                          unfold LinCon.sat at hs' ⊢; rw [hsEq]
                          rw [hdotDiff] at hs'
                          have hdx : delta * x' j ≤ delta := by
                            have hmul := Rat.mul_le_mul_of_nonneg_left ((hx'.1 j).2 1 hub1) hdel
                            simpa [Rat.mul_one] using hmul
                          have h1 := Rat.add_le_add_right (c := delta * x' j) |>.mpr hs'
                          have h2 : con.coeffs.dot x' ≤ con.rhs - delta + delta * x' j := by
                            simpa [Rat.sub_eq_add_neg, Rat.add_assoc, Rat.neg_add_cancel, Rat.add_zero] using h1
                          have h3 : con.rhs - delta + delta * x' j ≤ con.rhs - delta + delta :=
                            Rat.add_le_add_left.mpr hdx
                          have h4 : con.rhs - delta + delta = con.rhs := Rat.sub_add_cancel
                          exact Rat.le_trans h2 (Rat.le_trans h3 (by rw [h4]; exact Rat.le_refl))
                        · exact hx'.2.1 ⟨i.1, by rw [Array.size_set]; exact i.2⟩ r
                            (by rw [array_set_getElem_fin]; simp only [hir, ↓reduceIte]; exact hr)
                    · contradiction
                  · simp only [a, j, hpos, ↓reduceIte] at h
                    split at h
                    · next hMle =>
                      injection h with hst; subst hst
                      have hMle' : maxExcl + (a + delta) ≤ con.rhs := decide_eq_true_eq.mp hMle
                      let newCoeffs : RatVec n :=
                        RatVec.ofFn (fun k => if k = j then a + delta else con.coeffs.get k)
                      have hdotDiff : ∀ x : Fin n → Rat,
                          newCoeffs.dot x = con.coeffs.dot x + delta * x j := by
                        intro x
                        rw [dot_split_col newCoeffs j x, dot_split_col con.coeffs j x]
                        have hrest :
                            finSum n (fun k => if some j = some k then 0 else newCoeffs.get k * x k) =
                            finSum n (fun k => if some j = some k then 0 else con.coeffs.get k * x k) := by
                          apply finSum_congr; intro k
                          by_cases hkj : k = j
                          · subst hkj; simp
                          · have hne : some j ≠ some k := fun e => hkj (Option.some.inj e).symm
                            simp [newCoeffs, hkj, hne]
                        rw [hrest]
                        have hgetJ : newCoeffs.get j = a + delta := by simp [newCoeffs]
                        rw [hgetJ]
                        change _ + (a + delta) * x j = _ + a * x j + delta * x j
                        rw [Rat.add_mul, Rat.add_assoc]
                      constructor
                      · intro x hx
                        have hbin01 := rat_binary_zero_or_one ((hx.1 j).1 0 hlb0) ((hx.1 j).2 1 hub1) (hx.2.2 j hint)
                        have hnewSat : ({ coeffs := newCoeffs, sense := .le, rhs := con.rhs } : LinCon n).sat x := by
                          unfold LinCon.sat; dsimp only
                          rcases hbin01 with hx0 | hx1
                          · rw [hdotDiff, hx0, Rat.mul_zero, Rat.add_zero]
                            have hsat := hx.2.1 ⟨row, hrow⟩ con hcon
                            unfold LinCon.sat at hsat; rwa [hsEq] at hsat
                          · rw [dot_split_col newCoeffs j x, hx1, Rat.mul_one]
                            have hrest :
                                finSum n (fun k => if some j = some k then 0 else newCoeffs.get k * x k) =
                                finSum n (fun k => if some j = some k then 0 else con.coeffs.get k * x k) := by
                              apply finSum_congr; intro k
                              by_cases hkj : k = j
                              · subst hkj; simp
                              · have hne : some j ≠ some k := fun e => hkj (Option.some.inj e).symm
                                simp [newCoeffs, hkj, hne]
                            rw [hrest]
                            have hgetJ : newCoeffs.get j = a + delta := by simp [newCoeffs]
                            rw [hgetJ]
                            have hleM := sumMaxAct_go_ge st con.coeffs (some j) x hx.1 0 (Nat.zero_le n) 0 maxExcl Rat.le_refl hM
                            exact Rat.le_trans (Rat.add_le_add_right.mpr hleM) hMle'
                        refine ⟨x, ⟨hx.1, fun i r hr => ?_, hx.2.2⟩, Rat.le_refl⟩
                        rw [array_set_getElem_fin] at hr
                        by_cases hir : i.1 = row
                        · simp only [hir, ↓reduceIte] at hr
                          injection hr with hrEq; subst r; exact hnewSat
                        · simp only [hir, ↓reduceIte] at hr
                          exact hx.2.1 ⟨i.1, by simpa [Array.size_set] using i.2⟩ r hr
                      · intro x' hx'
                        refine ⟨⟨hx'.1, fun i r hr => ?_, hx'.2.2⟩, rfl⟩
                        by_cases hir : i.1 = row
                        · have hrEq : r = con := by
                            have hget : st.rows[i] = some con := by
                              change st.rows[i.1] = some con; simpa [hir] using hcon
                            rw [hget] at hr; injection hr with h1; exact h1.symm
                          subst r
                          have hs' := hx'.2.1 ⟨row, by rw [Array.size_set]; exact hrow⟩
                            { coeffs := newCoeffs, sense := .le, rhs := con.rhs }
                            (by rw [array_set_getElem_fin]; simp only [↓reduceIte]; rfl)
                          unfold LinCon.sat at hs' ⊢; rw [hsEq]
                          rw [hdotDiff] at hs'
                          have hdx : 0 ≤ delta * x' j :=
                            Rat.mul_nonneg hdel ((hx'.1 j).1 0 hlb0)
                          have hle : con.coeffs.dot x' ≤ con.coeffs.dot x' + delta * x' j := by
                            simpa [Rat.add_zero] using Rat.add_le_add_left (c := con.coeffs.dot x') |>.mpr hdx
                          exact Rat.le_trans hle hs'
                        · exact hx'.2.1 ⟨i.1, by rw [Array.size_set]; exact i.2⟩ r
                            (by rw [array_set_getElem_fin]; simp only [hir, ↓reduceIte]; exact hr)
                    · contradiction
            · contradiction
        · contradiction
      · contradiction
    · contradiction
  | symBreakLe u v colPerm rowPerm =>
    unfold applyPresolveStep at h; dsimp only at h
    split at h
    · next hu =>
      split at h
      · next hv =>
        split at h
        · next huvNe =>
          by_cases hpu : u < colPerm.size
          · simp only [hpu, ↓reduceDIte] at h
            split at h
            · next hok =>
              injection h with hst; subst hst
              let ju : Fin n := ⟨u, hu⟩
              let jv : Fin n := ⟨v, hv⟩
              have hne : ju ≠ jv := fun e => (bne_iff_ne.mp huvNe) (congrArg Fin.val e)
              simp only [Bool.and_eq_true] at hok
              rcases hok with ⟨⟨⟨⟨hinvB, huvMap⟩, hcols⟩, hobj⟩, hrows⟩
              obtain ⟨hsz, hlt, hinv, hf⟩ := checkPermInvolution_spec hinvB
              have huvEq : colPerm[u]'hpu = v := beq_iff_eq.mp huvMap
              have hfu : permFn colPerm hsz hlt ju = jv := Fin.ext huvEq
              have hfv : permFn colPerm hsz hlt jv = ju := by rw [← hfu, hinv]
              have hcolSpec := checkSymBreakCols_sound st colPerm hsz hlt 0 (Nat.zero_le n)
                (fun _ hi => by omega) hcols
              unfold checkSymBreakObj at hobj
              simp only [Bool.and_eq_true] at hobj
              have hobjMono : st.obj.get jv ≤ st.obj.get ju := decide_eq_true_eq.mp hobj.1
              have hobjOther := checkSymBreakObj_loop_sound st ju jv colPerm hsz hlt 0 (Nat.zero_le n)
                (fun _ hi => by omega) hobj.2
              unfold checkSymBreakRows at hrows
              simp only [Bool.and_eq_true] at hrows
              have hrsz : rowPerm.size = st.rows.size := beq_iff_eq.mp hrows.1
              have hrowSpec := checkSymBreakRows_sound st ju jv colPerm rowPerm hsz hlt hf hfv hrsz
                0 (Nat.zero_le st.rows.size) (fun _ hi => by omega) hrows.2
              let newCon : LinCon n :=
                { coeffs := RatVec.ofFn (fun j => if j = ju then 1 else if j = jv then -1 else 0),
                  sense := .le, rhs := 0 }
              constructor
              · intro x hx
                rcases Rat.le_total (a := x ju) (b := x jv) with hle | hge
                · refine ⟨x, ⟨hx.1, fun i r hr => ?_, hx.2.2⟩, Rat.le_refl⟩
                  have hisz : i.1 < st.rows.size + 1 := by simpa [Array.size_push] using i.2
                  change (st.rows.push (some newCon))[i.1] = some r at hr
                  by_cases hilt : i.1 < st.rows.size
                  · simp only [Array.getElem_push_lt hilt] at hr
                    exact hx.2.1 ⟨i.1, hilt⟩ r hr
                  · have heq : i.1 = st.rows.size := by omega
                    simp only [heq, Array.getElem_push_eq] at hr
                    injection hr with hrEq; subst r
                    unfold LinCon.sat; rw [dot_diff_unit hne]
                    have := Rat.add_le_add_right (c := -x jv) |>.mpr hle
                    simpa [Rat.sub_eq_add_neg, Rat.add_neg_cancel] using this
                · let x' : Fin n → Rat := fun j => x (permFn colPerm hsz hlt j)
                  have hx'B : inVarBounds st.lbAt st.ubAt x' := by
                    intro j
                    refine ⟨fun l hl => ?_, fun ub huB => ?_⟩
                    · obtain ⟨l', hl', hle'⟩ := (hcolSpec j).2.1 l hl
                      exact Rat.le_trans hle' ((hx.1 (permFn colPerm hsz hlt j)).1 l' hl')
                    · obtain ⟨u', hu', hle'⟩ := (hcolSpec j).2.2 ub huB
                      exact Rat.le_trans ((hx.1 (permFn colPerm hsz hlt j)).2 u' hu') hle'
                  have hx'I : ∀ j : Fin n, st.isIntAt j = true → isRatInt (x' j) :=
                    fun j hj => hx.2.2 (permFn colPerm hsz hlt j) ((hcolSpec j).1 hj)
                  have hx'R : ∀ i : Fin (st.rows.push (some newCon)).size, ∀ r,
                      (st.rows.push (some newCon))[i] = some r → r.sat x' := by
                    intro i r hr
                    have hisz : i.1 < st.rows.size + 1 := by simpa [Array.size_push] using i.2
                    change (st.rows.push (some newCon))[i.1] = some r at hr
                    by_cases hilt : i.1 < st.rows.size
                    · simp only [Array.getElem_push_lt hilt] at hr
                      exact hrowSpec ⟨i.1, hilt⟩ r hr x hx hge
                    · have heq : i.1 = st.rows.size := by omega
                      simp only [heq, Array.getElem_push_eq] at hr
                      injection hr with hrEq; subst r
                      unfold LinCon.sat; rw [dot_diff_unit hne]
                      dsimp only [x']; rw [hfu, hfv]
                      have := Rat.add_le_add_right (c := -x ju) |>.mpr hge
                      simpa [Rat.sub_eq_add_neg, Rat.add_neg_cancel] using this
                  have hx'Obj : StateObjective { st with rows := st.rows.push (some newCon) } x' ≤ StateObjective st x := by
                    unfold StateObjective
                    exact Rat.add_le_add_left.mpr
                      (stateObj_perm_le_of_mono hinv hf hfu hne hobjOther hobjMono hge)
                  exact ⟨x', ⟨hx'B, hx'R, hx'I⟩, hx'Obj⟩
              · intro x' hx'
                refine ⟨⟨hx'.1, fun i r hr => ?_, hx'.2.2⟩, rfl⟩
                exact hx'.2.1 ⟨i.1, by rw [Array.size_push]; exact Nat.lt_succ_of_lt i.2⟩ r
                  (by change (st.rows.push (some newCon))[i.1] = some r; simpa [Array.getElem_push_lt i.2] using hr)
            · contradiction
          · simp [hpu] at h
        · contradiction
      · contradiction
    · contradiction

/-- Applies a sequence of presolve steps starting from index `idx`. -/
def runPresolveSteps {n : Nat} (steps : Array PresolveStep) (idx : Nat)
    (st : PresolveState n) : Option (PresolveState n) :=
  if h : idx < steps.size then
    match applyPresolveStep st steps[idx] with
    | some st' => runPresolveSteps steps (idx + 1) st'
    | none => none
  else
    some st
termination_by steps.size - idx

/-- Soundness of a sequence of presolve steps: preserves both forward objective-bounding
projection and backward feasible solution lifting with exact objective equality. -/
theorem runPresolveSteps_sound {n : Nat} (steps : Array PresolveStep) :
    ∀ idx (st stFinal : PresolveState n),
      runPresolveSteps steps idx st = some stFinal →
      (∀ x : Fin n → Rat, StateFeasible st x →
        ∃ x' : Fin n → Rat, StateFeasible stFinal x' ∧ StateObjective stFinal x' ≤ StateObjective st x) ∧
      (∀ x' : Fin n → Rat, StateFeasible stFinal x' →
        StateFeasible st x' ∧ StateObjective st x' = StateObjective stFinal x') := by
  intro idx st stFinal hrun
  unfold runPresolveSteps at hrun
  split at hrun
  · next hlt =>
    cases hstep : applyPresolveStep st steps[idx] with
    | none => simp [hstep] at hrun
    | some stMid =>
      simp only [hstep] at hrun
      obtain ⟨hFwd1, hBwd1⟩ := applyPresolveStep_sound hstep
      obtain ⟨hFwd2, hBwd2⟩ := runPresolveSteps_sound steps (idx + 1) stMid stFinal hrun
      constructor
      · intro x hx
        obtain ⟨xMid, hxMid, hobj1⟩ := hFwd1 x hx
        obtain ⟨xFinal, hxFinal, hobj2⟩ := hFwd2 xMid hxMid
        exact ⟨xFinal, hxFinal, Rat.le_trans hobj2 hobj1⟩
      · intro xFinal hxFinal
        obtain ⟨hxMid, hobj2⟩ := hBwd2 xFinal hxFinal
        obtain ⟨hxOrig, hobj1⟩ := hBwd1 xFinal hxMid
        exact ⟨hxOrig, hobj1.trans hobj2⟩
  · next hge =>
    injection hrun with hEq; subst hEq
    exact ⟨fun x hx => ⟨x, hx, Rat.le_refl⟩, fun x' hx' => ⟨hx', rfl⟩⟩
termination_by idx _ _ _ => steps.size - idx

/-- Exact structural equality check between two `VIPRInstance`s sharing dimension `n`. -/
def checkVIPRInstanceEqSame {n : Nat} (objConst1 objConst2 : Rat)
    (obj1 obj2 : Fin n → Rat) (isInt1 isInt2 : Fin n → Bool)
    (lb1 lb2 ub1 ub2 : Fin n → Option Rat)
    (rows1 rows2 : Array (LinCon n)) : Bool :=
  objConst1 == objConst2 &&
  checkCols 0 &&
  (rows1.size == rows2.size) &&
  checkRows 0
where
  checkCols (k : Nat) : Bool :=
    if hk : k < n then
      let j : Fin n := ⟨k, hk⟩
      if obj1 j == obj2 j && isInt1 j == isInt2 j && lb1 j == lb2 j && ub1 j == ub2 j then
        checkCols (k + 1)
      else false
    else true
  termination_by n - k
  checkRows (i : Nat) : Bool :=
    if hi : i < rows1.size then
      if hi2 : i < rows2.size then
        if rows1[i].beq rows2[i] then checkRows (i + 1) else false
      else false
    else true
  termination_by rows1.size - i

private theorem checkVIPRInstanceEqSame_cols_sound {n : Nat}
    (obj1 obj2 : Fin n → Rat) (isInt1 isInt2 : Fin n → Bool)
    (lb1 lb2 ub1 ub2 : Fin n → Option Rat) :
    ∀ k (hk : k ≤ n),
      (∀ i (hi : i < k),
        let j : Fin n := ⟨i, Nat.lt_of_lt_of_le hi hk⟩
        obj1 j = obj2 j ∧ isInt1 j = isInt2 j ∧ lb1 j = lb2 j ∧ ub1 j = ub2 j) →
      checkVIPRInstanceEqSame.checkCols obj1 obj2 isInt1 isInt2 lb1 lb2 ub1 ub2 k = true →
      ∀ j : Fin n, obj1 j = obj2 j ∧ isInt1 j = isInt2 j ∧ lb1 j = lb2 j ∧ ub1 j = ub2 j := by
  intro k hk hprev hloop j
  unfold checkVIPRInstanceEqSame.checkCols at hloop
  dsimp only at hloop
  split at hloop
  · next hlt =>
    split at hloop
    · next hok =>
      simp only [Bool.and_eq_true, beq_iff_eq] at hok
      have hnext : ∀ i (hi : i < k + 1),
          let ji : Fin n := ⟨i, Nat.lt_of_lt_of_le hi hlt⟩
          obj1 ji = obj2 ji ∧ isInt1 ji = isInt2 ji ∧ lb1 ji = lb2 ji ∧ ub1 ji = ub2 ji := by
        intro i hi
        by_cases hik : i = k
        · subst hik; exact ⟨hok.1.1.1, hok.1.1.2, hok.1.2, hok.2⟩
        · exact hprev i (by omega)
      exact checkVIPRInstanceEqSame_cols_sound obj1 obj2 isInt1 isInt2 lb1 lb2 ub1 ub2 (k + 1) hlt hnext hloop j
    · contradiction
  · next hge =>
    have hkn : k = n := by omega
    subst hkn
    exact hprev j.1 j.2
termination_by k _ _ _ => n - k

private theorem checkVIPRInstanceEqSame_rows_sound {n : Nat}
    (rows1 rows2 : Array (LinCon n)) (hsz : rows1.size = rows2.size) :
    ∀ k (hk : k ≤ rows1.size),
      (∀ i (hi : i < k),
        rows1[i]'(Nat.lt_of_lt_of_le hi hk) = rows2[i]'(hsz ▸ Nat.lt_of_lt_of_le hi hk)) →
      checkVIPRInstanceEqSame.checkRows rows1 rows2 k = true →
      ∀ i (hi : i < rows1.size), rows1[i]'hi = rows2[i]'(hsz ▸ hi) := by
  intro k hk hprev hloop i hi
  unfold checkVIPRInstanceEqSame.checkRows at hloop
  split at hloop
  · next hlt =>
    have hlt2 : k < rows2.size := by rw [← hsz]; exact hlt
    simp only [hlt2, ↓reduceDIte] at hloop
    split at hloop
    · next hok =>
      have heq : rows1[k]'hlt = rows2[k]'hlt2 := LinCon.eq_of_beq hok
      have hnext : ∀ m (hm : m < k + 1),
          rows1[m]'(Nat.lt_of_lt_of_le hm hlt) = rows2[m]'(hsz ▸ Nat.lt_of_lt_of_le hm hlt) := by
        intro m hm
        by_cases hmk : m = k
        · subst hmk; exact heq
        · exact hprev m (by omega)
      exact checkVIPRInstanceEqSame_rows_sound rows1 rows2 hsz (k + 1) hlt hnext hloop i hi
    · contradiction
  · next hge =>
    have hkn : k = rows1.size := by omega
    subst hkn
    exact hprev i hi
termination_by k _ _ _ _ => rows1.size - k

private theorem checkVIPRInstanceEqSame_sound {n : Nat} {objConst1 objConst2 : Rat}
    {obj1 obj2 : Fin n → Rat} {isInt1 isInt2 : Fin n → Bool}
    {lb1 lb2 ub1 ub2 : Fin n → Option Rat}
    {rows1 rows2 : Array (LinCon n)}
    (h : checkVIPRInstanceEqSame objConst1 objConst2 obj1 obj2 isInt1 isInt2 lb1 lb2 ub1 ub2 rows1 rows2 = true) :
    objConst1 = objConst2 ∧ obj1 = obj2 ∧ isInt1 = isInt2 ∧ lb1 = lb2 ∧ ub1 = ub2 ∧ rows1 = rows2 := by
  unfold checkVIPRInstanceEqSame at h
  simp only [Bool.and_eq_true, beq_iff_eq] at h
  rcases h with ⟨⟨⟨hc, hcols⟩, hsz⟩, hrows⟩
  have hcolSpec := checkVIPRInstanceEqSame_cols_sound obj1 obj2 isInt1 isInt2 lb1 lb2 ub1 ub2 0 (Nat.zero_le n)
    (fun _ hi => by omega) hcols
  have hrowSpec := checkVIPRInstanceEqSame_rows_sound rows1 rows2 hsz 0 (Nat.zero_le rows1.size)
    (fun _ hi => by omega) hrows
  refine ⟨hc, funext (fun j => (hcolSpec j).1), funext (fun j => (hcolSpec j).2.1),
    funext (fun j => (hcolSpec j).2.2.1), funext (fun j => (hcolSpec j).2.2.2),
    Array.ext hsz (fun i hi1 hi2 => by simpa using hrowSpec i hi1)⟩

/-- Exact structural equality check between two `VIPRInstance` values. -/
def checkVIPRInstanceEq : VIPRInstance → VIPRInstance → Bool
  | ⟨n1, c1, o1, i1, r1, l1, u1⟩, ⟨n2, c2, o2, i2, r2, l2, u2⟩ =>
    if hN : n1 = n2 then
      match n2, hN, c2, o2, i2, r2, l2, u2 with
      | _, rfl, c2, o2, i2, r2, l2, u2 =>
        checkVIPRInstanceEqSame c1 c2 o1 o2 i1 i2 l1 l2 u1 u2 r1 r2
    else false

theorem checkVIPRInstanceEq_sound {P Q : VIPRInstance}
    (h : checkVIPRInstanceEq P Q = true) : P = Q := by
  cases P with | mk n1 c1 o1 i1 r1 l1 u1 =>
  cases Q with | mk n2 c2 o2 i2 r2 l2 u2 =>
  unfold checkVIPRInstanceEq at h
  dsimp only at h
  split at h
  · next hN =>
    subst hN
    dsimp only at h
    obtain ⟨hc, ho, hi, hl, hu, hr⟩ := checkVIPRInstanceEqSame_sound h
    subst hc; subst ho; subst hi; subst hl; subst hu; subst hr
    rfl
  · contradiction

/-- Combined presolve + Mini-VIPR certificate relating an original MILP `orig`
through a verified presolve reduction sequence to a verified `reduced` Mini-VIPR proof. -/
structure VIPRWithPresolveCert where
  orig : VIPRInstance
  presolve : Array PresolveStep
  reduced : MiniVIPRCert

/-- Executable verifier for a presolve-enveloped Mini-VIPR certificate. -/
def checkVIPRWithPresolve (cert : VIPRWithPresolveCert) : Bool :=
  match runPresolveSteps cert.presolve 0 (PresolveState.ofInstance cert.orig) with
  | none => false
  | some stFinal =>
    checkVIPRInstanceEq stFinal.toInstance cert.reduced.inst &&
    checkMiniVIPR cert.reduced

/-- Top-level soundness theorem for presolve-enveloped Mini-VIPR certificates:
directly certifies infeasibility, global optimality (with a feasible lifted solution
on `cert.orig`), or a global objective lower bound on `cert.orig`. -/
theorem checkVIPRWithPresolve_sound (cert : VIPRWithPresolveCert)
    (h : checkVIPRWithPresolve cert = true) :
    match cert.reduced.mode with
    | .infeasible =>
      ¬ ∃ x : Fin cert.orig.n → Rat, MILPFeasible cert.orig x
    | .optimal xStar =>
      ∃ xOrig : Fin cert.orig.n → Rat,
        MILPFeasible cert.orig xOrig ∧
        LPObjective cert.orig xOrig = LPObjective cert.reduced.inst xStar.get ∧
        ∀ x : Fin cert.orig.n → Rat, MILPFeasible cert.orig x →
          LPObjective cert.orig xOrig ≤ LPObjective cert.orig x
    | .bound targetLB =>
      (∀ xInc, cert.reduced.incumbent? = some xInc →
        ∃ xOrig : Fin cert.orig.n → Rat,
          MILPFeasible cert.orig xOrig ∧
          LPObjective cert.orig xOrig = LPObjective cert.reduced.inst xInc.get) ∧
      ∀ x : Fin cert.orig.n → Rat, MILPFeasible cert.orig x →
        targetLB ≤ LPObjective cert.orig x := by
  cases cert with
  | mk orig presolve reduced =>
    unfold checkVIPRWithPresolve at h
    dsimp only at h
    cases hrun : runPresolveSteps presolve 0 (PresolveState.ofInstance orig) with
    | none => simp [hrun] at h
    | some stFinal =>
      simp only [hrun, Bool.and_eq_true] at h
      have hInstEq : stFinal.toInstance = reduced.inst := checkVIPRInstanceEq_sound h.1
      have hRedSound := checkMiniVIPR_sound reduced h.2
      obtain ⟨hFwdSt, hBwdSt⟩ := runPresolveSteps_sound presolve 0 (PresolveState.ofInstance orig) stFinal hrun
      have hFwd : ∀ x : Fin orig.n → Rat, MILPFeasible orig x →
          ∃ x' : Fin orig.n → Rat, MILPFeasible stFinal.toInstance x' ∧
            LPObjective stFinal.toInstance x' ≤ LPObjective orig x := by
        intro x hx
        obtain ⟨x', hx', hobj⟩ := hFwdSt x ((stateFeasible_ofInstance orig x).mpr hx)
        refine ⟨x', (stateFeasible_toInstance stFinal x').mp hx', ?_⟩
        rw [← stateObjective_toInstance, ← stateObjective_ofInstance]
        exact hobj
      have hBwd : ∀ x' : Fin orig.n → Rat, MILPFeasible stFinal.toInstance x' →
          MILPFeasible orig x' ∧ LPObjective orig x' = LPObjective stFinal.toInstance x' := by
        intro x' hx'
        obtain ⟨hxOrig, hobj⟩ := hBwdSt x' ((stateFeasible_toInstance stFinal x').mpr hx')
        refine ⟨(stateFeasible_ofInstance orig x').mp hxOrig, ?_⟩
        rw [← stateObjective_ofInstance, ← stateObjective_toInstance]
        exact hobj
      cases reduced with
      | mk redInst mode steps rootStep inc =>
        dsimp only at hInstEq hRedSound ⊢
        subst hInstEq
        cases mode with
        | infeasible =>
          rintro ⟨x, hx⟩
          obtain ⟨x', hx', _⟩ := hFwd x hx
          exact hRedSound ⟨x', hx'⟩
        | optimal xStar =>
          obtain ⟨hFeasStar, hOptStar⟩ := hRedSound
          obtain ⟨hOrigFeas, hObjEq⟩ := hBwd xStar.get hFeasStar
          refine ⟨xStar.get, hOrigFeas, hObjEq, fun x hx => ?_⟩
          obtain ⟨x', hx', hObjLe⟩ := hFwd x hx
          rw [hObjEq]
          exact Rat.le_trans (hOptStar x' hx') hObjLe
        | bound targetLB =>
          obtain ⟨hIncSound, hBoundSound⟩ := hRedSound
          refine ⟨fun xInc hxInc => ?_, fun x hx => ?_⟩
          · obtain ⟨hOrigFeas, hObjEq⟩ := hBwd xInc.get (hIncSound xInc hxInc)
            exact ⟨xInc.get, hOrigFeas, hObjEq⟩
          · obtain ⟨x', hx', hObjLe⟩ := hFwd x hx
            exact Rat.le_trans (hBoundSound x' hx') hObjLe

theorem checkVIPRWithPresolve_infeasible_sound (cert : VIPRWithPresolveCert)
    (hmode : cert.reduced.mode = .infeasible) (h : checkVIPRWithPresolve cert = true) :
    ¬ ∃ x : Fin cert.orig.n → Rat, MILPFeasible cert.orig x := by
  have hs := checkVIPRWithPresolve_sound cert h
  rwa [hmode] at hs

theorem checkVIPRWithPresolve_optimal_sound (cert : VIPRWithPresolveCert)
    (xStar : RatVec cert.reduced.inst.n)
    (hmode : cert.reduced.mode = .optimal xStar) (h : checkVIPRWithPresolve cert = true) :
    ∃ xOrig : Fin cert.orig.n → Rat,
      MILPFeasible cert.orig xOrig ∧
      LPObjective cert.orig xOrig = LPObjective cert.reduced.inst xStar.get ∧
      ∀ x : Fin cert.orig.n → Rat, MILPFeasible cert.orig x →
        LPObjective cert.orig xOrig ≤ LPObjective cert.orig x := by
  have hs := checkVIPRWithPresolve_sound cert h
  rwa [hmode] at hs

theorem checkVIPRWithPresolve_bound_sound (cert : VIPRWithPresolveCert) {targetLB : Rat}
    (h : checkVIPRWithPresolve cert = true) (hmode : cert.reduced.mode = .bound targetLB) :
    ∀ x : Fin cert.orig.n → Rat, MILPFeasible cert.orig x →
      targetLB ≤ LPObjective cert.orig x := by
  have hs := checkVIPRWithPresolve_sound cert h
  rw [hmode] at hs
  exact hs.2


/-! ## Section 5: JSON Decoding & Verification Entry Point -/

private def parseRowSense (s : String) : Except String RowSense :=
  match s with
  | "<=" | "le" => .ok .le
  | "=" | "==" | "eq" => .ok .eq
  | ">=" | "ge" => .ok .ge
  | _ => .error s!"invalid constraint sense '{s}'"

private def decodeRatVec (n : Nat) (v : JsonVal) : Except String (RatVec n) := do
  match v with
  | .arr elems =>
    if hsz : elems.size = n then do
      let mut out : Array Rat := Array.mkEmpty n
      for elem in elems do
        let r ← elem.asRat
        out := out.push r
      if h : out.size = n then
        .ok ⟨out, h⟩
      else
        .error "internal size mismatch decoding RatVec"
    else
      .error s!"expected coefficient vector of length {n}, got {elems.size}"
  | .obj _ =>
    let terms ← v.getArray "terms"
    let mut out : Array Rat := Array.replicate n 0
    for t in terms do
      let col ← t.getNat "col"
      let coeff ← t.getRat "coeff"
      if hcol : col < out.size then
        out := out.set col (out[col] + coeff)
      else
        .error s!"sparse column index {col} out of range [0, {n})"
    if h : out.size = n then
      .ok ⟨out, h⟩
    else
      .error "internal size mismatch decoding sparse RatVec"
  | _ => .error "expected rational array or sparse terms object"

private def decodeLinCon (n : Nat) (v : JsonVal) : Except String (LinCon n) := do
  let coeffsVal ←
    match v.getField? "coeffs" with
    | some c => .ok c
    | none => .ok v
  let coeffs ← decodeRatVec n coeffsVal
  let senseStr ← v.getString "sense"
  let sense ← parseRowSense senseStr
  let rhs ← v.getRat "rhs"
  .ok { coeffs := coeffs, sense := sense, rhs := rhs }

private def decodeOptRatVec (n : Nat) (name : String) (v : JsonVal) :
    Except String { arr : Array (Option Rat) // arr.size = n } := do
  let elems ← v.getArray name
  if _hsz : elems.size = n then
    let mut out : Array (Option Rat) := Array.mkEmpty n
    for elem in elems do
      match elem with
      | .null => out := out.push none
      | _ =>
        let r ← elem.asRat
        out := out.push (some r)
    if h : out.size = n then .ok ⟨out, h⟩
    else .error s!"internal size mismatch decoding {name}"
  else
    .error s!"expected {name} length {n}, got {elems.size}"

private def decodeBoolVec (n : Nat) (name : String) (v : JsonVal) :
    Except String { arr : Array Bool // arr.size = n } := do
  let elems ← v.getArray name
  if _hsz : elems.size = n then
    let mut out : Array Bool := Array.mkEmpty n
    for elem in elems do
      let b ← elem.asBool
      out := out.push b
    if h : out.size = n then .ok ⟨out, h⟩
    else .error s!"internal size mismatch decoding {name}"
  else
    .error s!"expected {name} length {n}, got {elems.size}"

private def decodeLinRefs (v : JsonVal) (key : String) : Except String (Array LinRef) := do
  let arr ← v.getArray key
  let mut out : Array LinRef := Array.mkEmpty arr.size
  for item in arr do
    let id ← item.getNat "id"
    let mult ← item.getRat "mult"
    out := out.push { id := id, mult := mult }
  .ok out

private def decodeVarBoundRefs (v : JsonVal) : Except String (Array VarBoundRef) := do
  match v.getField? "bounds" with
  | none => .ok #[]
  | some bv =>
    let arr ← bv.asArray
    let mut out : Array VarBoundRef := Array.mkEmpty arr.size
    for item in arr do
      let col ← item.getNat "col"
      let mult ← item.getRat "mult"
      let isUpper ←
        match item.getField? "kind" with
        | some kv =>
          let ks ← kv.asString
          match ks with
          | "ub" | "<=" => .ok true
          | "lb" | ">=" => .ok false
          | _ => .error s!"invalid bound kind '{ks}'"
        | none => item.getBool "isUpper"
      out := out.push { col := col, isUpper := isUpper, mult := mult }
    .ok out

private def decodeNatArray (arrVal : JsonVal) : Except String (Array Nat) := do
  let arr ← arrVal.asArray
  let mut out : Array Nat := Array.mkEmpty arr.size
  for item in arr do
    let x ← item.asNat
    out := out.push x
  .ok out

private def decodeOptNat (v : JsonVal) (key : String) : Except String (Option Nat) := do
  match v.getField? key with
  | none => .ok none
  | some .null => .ok none
  | some x =>
    let n ← x.asNat
    .ok (some n)

private def decodeProofStep (n : Nat) (v : JsonVal) : Except String (ProofStep n) := do
  let ruleName ← v.getString "rule"
  let rule ←
    match ruleName with
    | "conIndex" =>
      let row ← v.getNat "row"
      .ok (.conIndex row)
    | "assume" =>
      let col ← v.getNat "col"
      let bound ← v.getInt "bound"
      let isUpper ←
        match v.getField? "sense" with
        | some sv =>
          let ss ← sv.asString
          match ss with
          | "<=" | "le" => .ok true
          | ">=" | "ge" => .ok false
          | _ => .error s!"assume requires '<=' or '>=', got '{ss}'"
        | none => v.getBool "isUpper"
      .ok (.assume col isUpper bound)
    | "lincomb" =>
      let senseStr ← v.getString "sense"
      let sense ← parseRowSense senseStr
      let refs ← decodeLinRefs v "refs"
      let bounds ← decodeVarBoundRefs v
      .ok (.lincomb sense refs bounds)
    | "roundDown" =>
      let ref ← v.getNat "ref"
      .ok (.roundDown ref)
    | "roundUp" =>
      let ref ← v.getNat "ref"
      .ok (.roundUp ref)
    | "branch" =>
      let col ← v.getNat "col"
      let splitVal ← v.getInt "splitVal"
      let leftAsm ← v.getNat "leftAsm"
      let rightAsm ← v.getNat "rightAsm"
      let left ← v.getNat "left"
      let right ← v.getNat "right"
      .ok (.branch col splitVal leftAsm rightAsm left right)
    | "orbitBranch" =>
      let col ← v.getNat "col"
      let leftAsm ← v.getNat "leftAsm"
      let rightAsm ← v.getNat "rightAsm"
      let left ← v.getNat "left"
      let right ← v.getNat "right"
      let orbitAsms ← v.getField "orbitAsms" >>= decodeNatArray
      let witArr ← v.getArray "witnesses"
      let mut witnesses : Array OrbitWitness := Array.mkEmpty witArr.size
      for wv in witArr do
        let comp ← wv.getNat "companion"
        let colPerm ← wv.getField "colPerm" >>= decodeNatArray
        let rowPerm ← wv.getField "rowPerm" >>= decodeNatArray
        let supportEntries ←
          match wv.getField? "supportEntries" with
          | none | some .null => .ok #[]
          | some sv => decodeNatArray sv
        witnesses := witnesses.push {
          companion := comp
          colPerm := colPerm
          rowPerm := rowPerm
          supportEntries := supportEntries
        }
      let extraAsms ←
        match v.getField? "extraAsms" with
        | none | some .null => .ok #[]
        | some ev => decodeNatArray ev
      .ok (.orbitBranch col leftAsm rightAsm left right orbitAsms witnesses extraAsms)
    | "farkas" =>
      let refs ← decodeLinRefs v "refs"
      let bounds ← decodeVarBoundRefs v
      .ok (.farkas refs bounds)
    | "boundConflict" =>
      let col ← v.getNat "col"
      let lbRef ← decodeOptNat v "lbRef"
      let ubRef ← decodeOptNat v "ubRef"
      .ok (.boundConflict col lbRef ubRef)
    | "dualBound" =>
      let refs ← decodeLinRefs v "refs"
      let bounds ← decodeVarBoundRefs v
      .ok (.dualBound refs bounds)
    | "safeDual" =>
      let refs ← decodeLinRefs v "refs"
      let boundAsms ←
        match v.getField? "boundAsms" with
        | none => .ok #[]
        | some bv => decodeNatArray bv
      .ok (.safeDual refs boundAsms)
    | _ => .error s!"unknown Mini-VIPR rule '{ruleName}'"
  let claimedCon ←
    match v.getField? "con" with
    | none => .ok none
    | some .null => .ok none
    | some cv =>
      let c ← decodeLinCon n cv
      .ok (some c)
  let claimedAssumptions ←
    match v.getField? "assumptions" with
    | none => .ok none
    | some .null => .ok none
    | some av =>
      let arr ← decodeNatArray av
      .ok (some arr)
  .ok { rule := rule, claimedCon := claimedCon, claimedAssumptions := claimedAssumptions }

private def decodeOptBool (v : JsonVal) (key : String) (default : Bool) : Except String Bool :=
  match v.getField? key with
  | none | some .null => .ok default
  | some bv => bv.asBool

/-- Decodes a `VIPRInstance` from a parsed `JsonVal` object. -/
def decodeVIPRInstance (v : JsonVal) : Except String VIPRInstance := do
  let n ← v.getNat "n"
  let objConst ←
    match v.getField? "objConst" with
    | none | some .null => .ok (0 : Rat)
    | some ov => ov.asRat
  let objVal ← v.getField "obj"
  let objVec ← decodeRatVec n objVal
  let isIntVec ← decodeBoolVec n "isInt" v
  let lbVec ← decodeOptRatVec n "lb" v
  let ubVec ← decodeOptRatVec n "ub" v
  let rowsArr ← v.getArray "baseRows"
  let mut baseRows : Array (LinCon n) := Array.mkEmpty rowsArr.size
  for rv in rowsArr do
    let rc ← decodeLinCon n rv
    baseRows := baseRows.push rc
  .ok {
    n := n
    objConst := objConst
    obj := fun j => objVec.get j
    isInt := fun j => isIntVec.1[j.1]'(by rw [isIntVec.2]; exact j.2)
    baseRows := baseRows
    lb := fun j => lbVec.1[j.1]'(by rw [lbVec.2]; exact j.2)
    ub := fun j => ubVec.1[j.1]'(by rw [ubVec.2]; exact j.2)
  }

/-- Decodes a single `PresolveStep` from a parsed `JsonVal`. -/
def decodePresolveStep (v : JsonVal) : Except String PresolveStep := do
  let opName ← v.getString "op"
  match opName with
  | "roundBound" =>
    let col ← v.getNat "col"
    let isUpper ← v.getBool "isUpper"
    let newBound ← v.getRat "newBound"
    .ok (.roundBound col isUpper newBound)
  | "activityBound" =>
    let row ← v.getNat "row"
    let col ← v.getNat "col"
    let isUpper ← v.getBool "isUpper"
    let roundInt ← decodeOptBool v "roundInt" false
    let newBound ← v.getRat "newBound"
    .ok (.activityBound row col isUpper roundInt newBound)
  | "gcdRowDiv" =>
    let row ← v.getNat "row"
    let divisor ← v.getInt "divisor"
    .ok (.gcdRowDiv row divisor)
  | "substFixedRow" =>
    let row ← v.getNat "row"
    let col ← v.getNat "col"
    let val ← v.getRat "val"
    .ok (.substFixedRow row col val)
  | "substFixedObj" =>
    let col ← v.getNat "col"
    let val ← v.getRat "val"
    .ok (.substFixedObj col val)
  | "singletonRowElim" =>
    let row ← v.getNat "row"
    let col ← v.getNat "col"
    let roundInt ← decodeOptBool v "roundInt" false
    .ok (.singletonRowElim row col roundInt)
  | "redundantRow" =>
    let row ← v.getNat "row"
    .ok (.redundantRow row)
  | "binStrengthen" =>
    let row ← v.getNat "row"
    let col ← v.getNat "col"
    let delta ← v.getRat "delta"
    .ok (.binStrengthen row col delta)
  | "symBreakLe" =>
    let u ← v.getNat "u"
    let vCol ← v.getNat "v"
    let colPerm ← v.getField "colPerm" >>= decodeNatArray
    let rowPerm ← v.getField "rowPerm" >>= decodeNatArray
    .ok (.symBreakLe u vCol colPerm rowPerm)
  | _ => .error s!"unknown presolve op '{opName}'"

/-- Decodes a `MiniVIPRCert` from a parsed `JsonVal`. -/
def decodeMiniVIPRCert (v : JsonVal) : Except String MiniVIPRCert := do
  let inst ← decodeVIPRInstance v
  let modeStr ← v.getString "mode"
  let (mode, optInc) : CertMode inst.n × Option (RatVec inst.n) ←
    match modeStr with
    | "infeasible" => .ok (.infeasible, none)
    | "optimal" =>
      let incVal ← v.getField "incumbent"
      let incVec ← decodeRatVec inst.n incVal
      .ok (.optimal incVec, some incVec)
    | "bound" =>
      let targetLB ← v.getRat "targetLB"
      let incOpt ←
        match v.getField? "incumbent" with
        | none | some .null => .ok none
        | some iv => some <$> decodeRatVec inst.n iv
      .ok (.bound targetLB, incOpt)
    | _ => .error s!"invalid Mini-VIPR mode '{modeStr}'"
  let stepsArr ← v.getArray "steps"
  let mut steps : Array (ProofStep inst.n) := Array.mkEmpty stepsArr.size
  for sv in stepsArr do
    let ps ← decodeProofStep inst.n sv
    steps := steps.push ps
  let rootStep ←
    match v.getField? "rootStep" with
    | some rv => rv.asNat
    | none =>
      if steps.size > 0 then .ok (steps.size - 1)
      else .error "missing rootStep and steps is empty"
  .ok { inst := inst, mode := mode, steps := steps, rootStep := rootStep, incumbent? := optInc }

/-- Decodes a presolve-enveloped `VIPRWithPresolveCert` from a parsed `JsonVal`. -/
def decodeVIPRWithPresolveCert (v : JsonVal) : Except String VIPRWithPresolveCert := do
  let origVal ← v.getField "orig"
  let orig ← decodeVIPRInstance origVal
  let presolve ←
    match v.getField? "presolve" with
    | none | some .null => .ok #[]
    | some pv =>
      let arr ← pv.asArray
      let mut out : Array PresolveStep := Array.mkEmpty arr.size
      for sv in arr do
        let ps ← decodePresolveStep sv
        out := out.push ps
      .ok out
  let reduced ← decodeMiniVIPRCert v
  .ok { orig := orig, presolve := presolve, reduced := reduced }

/-- Decodes and verifies a parsed `JsonVal` Mini-VIPR certificate. -/
def verifyMiniVIPRJsonVal (jv : JsonVal) : Except String MiniVIPRCert := do
  let cert ← decodeMiniVIPRCert jv
  if checkMiniVIPR cert then
    .ok cert
  else
    .error "Mini-VIPR trace failed verification"

/-- Parses and verifies a JSON-encoded Mini-VIPR certificate. -/
def verifyMiniVIPRJson (raw : String) : Except String Unit := do
  let jv ← parseJson raw
  let _ ← verifyMiniVIPRJsonVal jv
  .ok ()

/-- Decodes and verifies a parsed `JsonVal` presolve-enveloped Mini-VIPR certificate. -/
def verifyVIPRWithPresolveJsonVal (jv : JsonVal) : Except String VIPRWithPresolveCert := do
  let cert ← decodeVIPRWithPresolveCert jv
  if checkVIPRWithPresolve cert then
    .ok cert
  else
    .error "Presolve-enveloped Mini-VIPR trace failed verification"

/-- Parses and verifies a JSON-encoded presolve-enveloped Mini-VIPR certificate. -/
def verifyVIPRWithPresolveJson (raw : String) : Except String Unit := do
  let jv ← parseJson raw
  let _ ← verifyVIPRWithPresolveJsonVal jv
  .ok ()

end GoblinFormal.Track4
