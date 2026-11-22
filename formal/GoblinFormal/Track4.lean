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

/-- Certificate target mode: global infeasibility or certified optimal incumbent. -/
inductive CertMode (n : Nat) where
  | infeasible
  | optimal (incumbent : RatVec n)
  deriving Repr

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

/-- Supported derivation rules in a Mini-VIPR certificate. -/
inductive StepRule (n : Nat) where
  | conIndex (row : Nat)
  | assume (col : Nat) (isUpper : Bool) (bound : Int)
  | lincomb (sense : RowSense) (refs : Array LinRef) (bounds : Array VarBoundRef)
  | roundDown (ref : Nat)
  | roundUp (ref : Nat)
  | branch (col : Nat) (splitVal : Int) (leftAsm rightAsm left right : Nat)
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
      else false
    else false

/-- Top-level soundness theorem for `checkMiniVIPR` covering both `infeasible` and `optimal` modes. -/
theorem checkMiniVIPR_sound (cert : MiniVIPRCert) (h : checkMiniVIPR cert = true) :
    match cert.mode with
    | .infeasible => ¬ ∃ x : Fin cert.inst.n → Rat, MILPFeasible cert.inst x
    | .optimal xStar =>
        MILPFeasible cert.inst xStar.get ∧
        ∀ x : Fin cert.inst.n → Rat, MILPFeasible cert.inst x →
          LPObjective cert.inst xStar.get ≤ LPObjective cert.inst x := by
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


/-! ## Section 4: JSON Decoding & Verification Entry Point -/

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

/-- Decodes a `MiniVIPRCert` from a parsed `JsonVal`. -/
def decodeMiniVIPRCert (v : JsonVal) : Except String MiniVIPRCert := do
  let n ← v.getNat "n"
  let objConst ←
    match v.getField? "objConst" with
    | none => .ok (0 : Rat)
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
  let inst : VIPRInstance := {
    n := n
    objConst := objConst
    obj := fun j => objVec.get j
    isInt := fun j => isIntVec.1[j.1]'(by rw [isIntVec.2]; exact j.2)
    baseRows := baseRows
    lb := fun j => lbVec.1[j.1]'(by rw [lbVec.2]; exact j.2)
    ub := fun j => ubVec.1[j.1]'(by rw [ubVec.2]; exact j.2)
  }
  let modeStr ← v.getString "mode"
  let mode : CertMode inst.n ←
    match modeStr with
    | "infeasible" => .ok .infeasible
    | "optimal" =>
      let incVal ← v.getField "incumbent"
      let incVec ← decodeRatVec inst.n incVal
      .ok (.optimal incVec)
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
  .ok { inst := inst, mode := mode, steps := steps, rootStep := rootStep }

/-- Parses and verifies a JSON-encoded Mini-VIPR certificate. -/
def verifyMiniVIPRJson (raw : String) : Except String Unit := do
  let jv ← parseJson raw
  let cert ← decodeMiniVIPRCert jv
  if checkMiniVIPR cert then
    .ok ()
  else
    .error "Mini-VIPR trace failed verification"

end GoblinFormal.Track4
