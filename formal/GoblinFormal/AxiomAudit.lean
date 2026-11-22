/-
GoblinFormal.AxiomAudit: machine-checked `#guard_msgs in #print axioms` lock
for top-level theorems across Common.Basic and Tracks 1–5.
Verifies zero `sorryAx`, zero `Lean.trustCompiler`, and zero custom axioms
(Track 5A/5B bitvector proofs use only kernel-checked `bv_decide` LRAT axioms).
-/

import GoblinFormal.Common.Basic
import GoblinFormal.Track1
import GoblinFormal.Track2A
import GoblinFormal.Track2B
import GoblinFormal.Track3
import GoblinFormal.Track4
import GoblinFormal.Track5A
import GoblinFormal.Track5B
import GoblinFormal.Track5C

/-! ### Common.Basic -/

/-- info: 'GoblinFormal.finSum_inj_le' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.finSum_inj_le

/-- info: 'GoblinFormal.finSum_inj_eq' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.finSum_inj_eq

/-- info: 'GoblinFormal.intFinSum_dvd' depends on axioms: [propext] -/
#guard_msgs in
#print axioms GoblinFormal.intFinSum_dvd

/-- info: 'GoblinFormal.float64BitsToRat_isNone_iff' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.float64BitsToRat_isNone_iff

/-! ### Track 1: LSAP Optimality & Safe Dual Bounds -/

/-- info: 'GoblinFormal.Track1.dual_lower_bound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track1.dual_lower_bound

/-- info: 'GoblinFormal.Track1.checkLSAP_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track1.checkLSAP_sound

/-- info: 'GoblinFormal.Track1.checkLSAP_sound_max' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track1.checkLSAP_sound_max

/-- info: 'GoblinFormal.Track1.checkLSAPSafe_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track1.checkLSAPSafe_sound

/-- info: 'GoblinFormal.Track1.checkLSAPSafe_sound_max' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track1.checkLSAPSafe_sound_max

/-! ### Track 2A: Presolve Rules & Symmetry Automorphisms -/

/-- info: 'GoblinFormal.Track2A.divCeilSpec_spec' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2A.divCeilSpec_spec

/-- info: 'GoblinFormal.Track2A.gcd_row_infeasible_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2A.gcd_row_infeasible_sound

/-- info: 'GoblinFormal.Track2A.gcd_row_infeasible_tol_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2A.gcd_row_infeasible_tol_sound

/-- info: 'GoblinFormal.Track2A.hasInfeasibleIntEqSpec_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2A.hasInfeasibleIntEqSpec_sound

/-- info: 'GoblinFormal.Track2A.activity_implied_upper_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2A.activity_implied_upper_sound

/-- info: 'GoblinFormal.Track2A.activity_implied_lower_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2A.activity_implied_lower_sound

/-- info: 'GoblinFormal.Track2A.presolveCanSkipTerm_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2A.presolveCanSkipTerm_sound

/-- info: 'GoblinFormal.Track2A.presolveCanSkipTerm_lower_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2A.presolveCanSkipTerm_lower_sound

/-- info: 'GoblinFormal.Track2A.binary_strengthen_pos_iff' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2A.binary_strengthen_pos_iff

/-- info: 'GoblinFormal.Track2A.binary_strengthen_pos_outward_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2A.binary_strengthen_pos_outward_sound

/-- info: 'GoblinFormal.Track2A.binary_strengthen_neg_iff' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2A.binary_strengthen_neg_iff

/-- info: 'GoblinFormal.Track2A.orbit_automorphism_preserves_opt' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2A.orbit_automorphism_preserves_opt

/-- info: 'GoblinFormal.Track2A.monotone_orbit_order_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2A.monotone_orbit_order_sound

/-! ### Track 2B: Cutting-Plane Soundness -/

/-- info: 'GoblinFormal.Track2B.gmi_cut_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2B.gmi_cut_sound

/-- info: 'GoblinFormal.Track2B.gmi_weaken_upward_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2B.gmi_weaken_upward_sound

/-- info: 'GoblinFormal.Track2B.row_agg_legal_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2B.row_agg_legal_sound

/-- info: 'GoblinFormal.Track2B.row_agg_sum_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2B.row_agg_sum_sound

/-- info: 'GoblinFormal.Track2B.row_agg_error_bound_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2B.row_agg_error_bound_sound

/-- info: 'GoblinFormal.Track2B.mir_fundamental_ineq' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2B.mir_fundamental_ineq

/-- info: 'GoblinFormal.Track2B.mir_cut_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2B.mir_cut_sound

/-- info: 'GoblinFormal.Track2B.cmir_safe_rounded_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2B.cmir_safe_rounded_sound

/-- info: 'GoblinFormal.Track2B.cmir_complement_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2B.cmir_complement_sound

/-- info: 'GoblinFormal.Track2B.vub_disagg_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2B.vub_disagg_sound

/-- info: 'GoblinFormal.Track2B.vub_disagg_opt_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2B.vub_disagg_opt_sound

/-- info: 'GoblinFormal.Track2B.cut_drop_scale_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2B.cut_drop_scale_sound

/-! ### Track 3: LP Optimality, Safe Dual Bounds & Farkas Certificates -/

/-- info: 'GoblinFormal.Track3.checkPrimalFeasible_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track3.checkPrimalFeasible_sound

/-- info: 'GoblinFormal.Track3.safeDualBound_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track3.safeDualBound_sound

/-- info: 'GoblinFormal.Track3.checkLPOptimal_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track3.checkLPOptimal_sound

/-- info: 'GoblinFormal.Track3.checkFarkas_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track3.checkFarkas_sound

/-! ### Track 4: Branch-and-Bound State Machine & Mini-VIPR Checker -/

/-- info: 'GoblinFormal.Track4.bestBound_global_valid' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.bestBound_global_valid

/-- info: 'GoblinFormal.Track4.infeasible_when_globalBound_none' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.infeasible_when_globalBound_none

/-- info: 'GoblinFormal.Track4.stepRoot_valid' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.stepRoot_valid

/-- info: 'GoblinFormal.Track4.stepSelectOpen_valid' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.stepSelectOpen_valid

/-- info: 'GoblinFormal.Track4.stepTightenLowerBound_valid' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.stepTightenLowerBound_valid

/-- info: 'GoblinFormal.Track4.stepFathomInfeas_valid' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.stepFathomInfeas_valid

/-- info: 'GoblinFormal.Track4.stepFathomBound_valid' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.stepFathomBound_valid

/-- info: 'GoblinFormal.Track4.stepBranchInt_valid' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.stepBranchInt_valid

/-- info: 'GoblinFormal.Track4.rc_fixing_ub_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.rc_fixing_ub_sound

/-- info: 'GoblinFormal.Track4.rc_fixing_lb_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.rc_fixing_lb_sound

/-- info: 'GoblinFormal.Track4.rc_fixing_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.rc_fixing_sound

/-- info: 'GoblinFormal.Track4.dual_rc_bound_valid' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.dual_rc_bound_valid

/-- info: 'GoblinFormal.Track4.stepReducedCostFix_valid' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.stepReducedCostFix_valid

/-- info: 'GoblinFormal.Track4.stepReducedCostFixInfeas_valid' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.stepReducedCostFixInfeas_valid

/-- info: 'GoblinFormal.Track4.stepIncumbent_valid' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.stepIncumbent_valid

/-- info: 'GoblinFormal.Track4.stepBudgetAbort_valid' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.stepBudgetAbort_valid

/-- info: 'GoblinFormal.Track4.stepGapClosed_optimal' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.stepGapClosed_optimal

/-- info: 'GoblinFormal.Track4.stepGapClosed_exact_optimal' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.stepGapClosed_exact_optimal

/-- info: 'GoblinFormal.Track4.checkMiniVIPR_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.checkMiniVIPR_sound

/-- info: 'GoblinFormal.Track4.checkMiniVIPR_infeasible_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.checkMiniVIPR_infeasible_sound

/-- info: 'GoblinFormal.Track4.checkMiniVIPR_optimal_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.checkMiniVIPR_optimal_sound

/-! ### Track 5A: Tier 5A ARM64 Leaf Binary Equivalence -/

/--
info: 'GoblinFormal.Track5A.splitmix64_arm64_equiv' depends on axioms: [propext,
 Classical.choice,
 Quot.sound,
 GoblinFormal.Track5A.splitmix64_arm64_equiv._native.bv_decide.ax_1_11,
 GoblinFormal.Track5A.splitmix64_arm64_equiv._native.bv_decide.ax_1_6]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5A.splitmix64_arm64_equiv

/--
info: 'GoblinFormal.Track5A.canonicalFloatBitsInlined_arm64_equiv' depends on axioms: [propext,
 Classical.choice,
 Quot.sound,
 GoblinFormal.Track5A.f64IsZero_iff._native.bv_decide.ax_1_5✝]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5A.canonicalFloatBitsInlined_arm64_equiv

/--
info: 'GoblinFormal.Track5A.canonicalFloatBits_arm64_equiv' depends on axioms: [propext,
 Classical.choice,
 Quot.sound,
 GoblinFormal.Track5A.evalCond_ls_subFlags64._native.bv_decide.ax_1_8,
 GoblinFormal.Track5A.stackGuard_32_ok._native.bv_decide.ax_1_5,
 GoblinFormal.Track5A.f64IsZero_iff._native.bv_decide.ax_1_5✝]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5A.canonicalFloatBits_arm64_equiv

/--
info: 'GoblinFormal.Track5A.divCeilInt64_arm64_equiv' depends on axioms: [propext,
 Classical.choice,
 Quot.sound,
 GoblinFormal.Track5A.evalCond_le_subFlags64_zero._native.bv_decide.ax_1_8,
 GoblinFormal.Track5A.evalCond_ls_subFlags64._native.bv_decide.ax_1_8,
 GoblinFormal.Track5A.stackGuard_16_ok._native.bv_decide.ax_1_5]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5A.divCeilInt64_arm64_equiv

/-- info: 'GoblinFormal.Track5A.gcdInt64_arm64_equiv' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5A.gcdInt64_arm64_equiv

/-! ### Track 5B: Tier 5B Zero-Alloc ARM64 Solver Kernels -/

/-- info: 'GoblinFormal.Track5B.checkZeroGC_run_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5B.checkZeroGC_run_sound

/--
info: 'GoblinFormal.Track5B.checkStackGuard_sound' depends on axioms: [propext,
 Classical.choice,
 Quot.sound,
 GoblinFormal.Track5A.evalCond_ls_subFlags64._native.bv_decide.ax_1_8,
 GoblinFormal.Track5A.stackGuard_32_ok._native.bv_decide.ax_1_5]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5B.checkStackGuard_sound

/-- info: 'GoblinFormal.Track5B.checkPanicBoundsReachability_run_safe' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5B.checkPanicBoundsReachability_run_safe

/--
info: 'GoblinFormal.Track5B.hasInfeasibleIntegerEqRow_elf_safe' depends on axioms: [propext,
 Classical.choice,
 Quot.sound,
 GoblinFormal.Track5A.evalCond_ls_subFlags64._native.bv_decide.ax_1_8,
 GoblinFormal.Track5A.stackGuard_32_ok._native.bv_decide.ax_1_5]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5B.hasInfeasibleIntegerEqRow_elf_safe

/--
info: 'GoblinFormal.Track5B.augmentRowScalar_elf_safe' depends on axioms: [propext,
 Classical.choice,
 Quot.sound,
 GoblinFormal.Track5A.evalCond_ls_subFlags64._native.bv_decide.ax_1_8,
 GoblinFormal.Track5A.stackGuard_32_ok._native.bv_decide.ax_1_5,
 GoblinFormal.Track5B.aug_stack_headroom_112._native.bv_decide.ax_1_5✝]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5B.augmentRowScalar_elf_safe

/--
info: 'GoblinFormal.Track5B.augmentRowScalar_done_nil_no_selectnbrecv' depends on axioms: [propext,
 Classical.choice,
 Quot.sound]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5B.augmentRowScalar_done_nil_no_selectnbrecv

/--
info: 'GoblinFormal.Track5B.hasInfeasibleIntegerEqRow_infeasible_gcd_exec' depends on axioms: [propext,
 Classical.choice,
 Quot.sound,
 GoblinFormal.Track5B.bv64_aligned_bytes._native.bv_decide.ax_1_5✝,
 GoblinFormal.Track5B.bv64_byte_extract._native.bv_decide.ax_1_5✝]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5B.hasInfeasibleIntegerEqRow_infeasible_gcd_exec

/--
info: 'GoblinFormal.Track5B.hasInfeasibleIntegerEqRow_feasible_gcd_exec' depends on axioms: [propext,
 Classical.choice,
 Quot.sound,
 GoblinFormal.Track5B.bv64_aligned_bytes._native.bv_decide.ax_1_5✝,
 GoblinFormal.Track5B.bv64_byte_extract._native.bv_decide.ax_1_5✝]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5B.hasInfeasibleIntegerEqRow_feasible_gcd_exec

/--
info: 'GoblinFormal.Track5B.augmentRowScalar_concrete_exec' depends on axioms: [propext,
 Classical.choice,
 Quot.sound,
 GoblinFormal.Track5B.bv64_aligned_bytes._native.bv_decide.ax_1_5✝,
 GoblinFormal.Track5B.bv64_byte_extract._native.bv_decide.ax_1_5✝]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5B.augmentRowScalar_concrete_exec

/-! ### Track 5C: Tier 5C amd64 SIMD Vector ISA & CFG Safety -/

/-- info: 'GoblinFormal.Track5C.checkVectorCFG_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5C.checkVectorCFG_sound
