/-
GoblinFormal.AxiomAudit: machine-checked `#guard_msgs in #print axioms` lock
for top-level theorems across Common.Basic and Tracks 1–5.
Verifies zero `sorryAx`, zero `Lean.trustCompiler`, and zero custom axioms
(Track 5A/5B/5D bitvector proofs use only kernel-checked `bv_decide` LRAT axioms).
-/

import GoblinFormal.Common.Basic
import GoblinFormal.Track1
import GoblinFormal.Track2A
import GoblinFormal.Track2B
import GoblinFormal.Track2C
import GoblinFormal.Track3
import GoblinFormal.Track3B
import GoblinFormal.Track4
import GoblinFormal.Track5A
import GoblinFormal.Track5B
import GoblinFormal.Track5C
import GoblinFormal.Track5D

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

/-! ### Track 2C: Universal Knapsack Cover, Lot-Sizing, Aggregated cMIR & Cut-Drop Soundness -/

/-- info: 'GoblinFormal.Track2C.knapsack_cover_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2C.knapsack_cover_sound

/-- info: 'GoblinFormal.Track2C.lifted_knapsack_cover_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2C.lifted_knapsack_cover_sound

/-- info: 'GoblinFormal.Track2C.checkKnapsackCoverCert_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2C.checkKnapsackCoverCert_sound

/-- info: 'GoblinFormal.Track2C.vub_zero_forces_prod_zero' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2C.vub_zero_forces_prod_zero

/-- info: 'GoblinFormal.Track2C.lot_sizing_lS_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2C.lot_sizing_lS_sound

/-- info: 'GoblinFormal.Track2C.checkLotSizingCutCert_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2C.checkLotSizingCutCert_sound

/-- info: 'GoblinFormal.Track2C.row_agg_error_bound_opt_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2C.row_agg_error_bound_opt_sound

/-- info: 'GoblinFormal.Track2C.agg_row_cmir_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2C.agg_row_cmir_sound

/-- info: 'GoblinFormal.Track2C.checkAggCMIRCert_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2C.checkAggCMIRCert_sound

/-- info: 'GoblinFormal.Track2C.dropMinTerm_le' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2C.dropMinTerm_le

/-- info: 'GoblinFormal.Track2C.finalize_cut_drop_universal_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2C.finalize_cut_drop_universal_sound

/-- info: 'GoblinFormal.Track2C.checkFinalizedCutCert_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2C.checkFinalizedCutCert_sound

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

/-! ### Track 3B: LP/MILP Unboundedness, GCD Row Division & LP Presolve Dual Postsolve -/

/-- info: 'GoblinFormal.Track3B.checkRecessionRay_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track3B.checkRecessionRay_sound

/-- info: 'GoblinFormal.Track3B.rayPoint_primalFeasible' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track3B.rayPoint_primalFeasible

/-- info: 'GoblinFormal.Track3B.checkLPUnbounded_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track3B.checkLPUnbounded_sound

/-- info: 'GoblinFormal.Track3B.checkIntegrality_sound' depends on axioms: [propext, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track3B.checkIntegrality_sound

/-- info: 'GoblinFormal.Track3B.rayPoint_nat_integrality' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track3B.rayPoint_nat_integrality

/-- info: 'GoblinFormal.Track3B.checkMILPUnbounded_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track3B.checkMILPUnbounded_sound

/-- info: 'GoblinFormal.Track3B.gcd_row_div_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track3B.gcd_row_div_sound

/-- info: 'GoblinFormal.Track3B.lp_presolve_primal_lift' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track3B.lp_presolve_primal_lift

/-- info: 'GoblinFormal.Track3B.lp_presolve_safeDualBound_eq' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track3B.lp_presolve_safeDualBound_eq

/-- info: 'GoblinFormal.Track3B.lp_presolve_dual_bound_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track3B.lp_presolve_dual_bound_sound

/-- info: 'GoblinFormal.Track3B.lp_presolve_dual_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track3B.lp_presolve_dual_sound

/-- info: 'GoblinFormal.Track3B.lp_presolve_chain_dual_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track3B.lp_presolve_chain_dual_sound

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

/-- info: 'GoblinFormal.Track4.checkMiniVIPR_bound_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.checkMiniVIPR_bound_sound

/-- info: 'GoblinFormal.Track4.checkPermBijection_spec' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.checkPermBijection_spec

/-- info: 'GoblinFormal.Track4.checkPermInvolution_spec' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.checkPermInvolution_spec

/-- info: 'GoblinFormal.Track4.checkConPermImplies_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.checkConPermImplies_sound

/-- info: 'GoblinFormal.Track4.checkOrbitAutomorphism_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.checkOrbitAutomorphism_sound

/-- info: 'GoblinFormal.Track4.applyPresolveStep_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.applyPresolveStep_sound

/-- info: 'GoblinFormal.Track4.runPresolveSteps_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.runPresolveSteps_sound

/-- info: 'GoblinFormal.Track4.checkVIPRInstanceEq_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.checkVIPRInstanceEq_sound

/-- info: 'GoblinFormal.Track4.checkVIPRWithPresolve_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.checkVIPRWithPresolve_sound

/-- info: 'GoblinFormal.Track4.checkVIPRWithPresolve_infeasible_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.checkVIPRWithPresolve_infeasible_sound

/-- info: 'GoblinFormal.Track4.checkVIPRWithPresolve_optimal_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.checkVIPRWithPresolve_optimal_sound

/-- info: 'GoblinFormal.Track4.checkVIPRWithPresolve_bound_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.checkVIPRWithPresolve_bound_sound

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

/-! ### Track 5D: Tier 5D Phase 2 LSAP Binary Safety & IEEE-754 SIMD Min2 Equivalence -/

/-- info: 'GoblinFormal.Track5D.stepArm64D_proxy_status_pc' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5D.stepArm64D_proxy_status_pc

/-- info: 'GoblinFormal.Track5D.twoSumAssign_arm64_equiv' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5D.twoSumAssign_arm64_equiv

/-- info: 'GoblinFormal.Track5D.checkFramelessLeaf_run_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5D.checkFramelessLeaf_run_sound

/-- info: 'GoblinFormal.Track5D.exactReducedCost_and_leaves_elf_safe' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5D.exactReducedCost_and_leaves_elf_safe

/--
info: 'GoblinFormal.Track5D.exactReducedCostNoinline_elf_safe' depends on axioms: [propext,
 Classical.choice,
 Quot.sound,
 GoblinFormal.Track5A.evalCond_ls_subFlags64._native.bv_decide.ax_1_8,
 GoblinFormal.Track5A.stackGuard_32_ok._native.bv_decide.ax_1_5]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5D.exactReducedCostNoinline_elf_safe

/-- info: 'GoblinFormal.Track5D.hasPotentialCancellation_zeroGC' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5D.hasPotentialCancellation_zeroGC

/-- info: 'GoblinFormal.Track5D.hasPotentialCancellation_stackGuard' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5D.hasPotentialCancellation_stackGuard

/-- info: 'GoblinFormal.Track5D.hasPotentialCancellation_pbCert' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5D.hasPotentialCancellation_pbCert

/--
info: 'GoblinFormal.Track5D.hasPotentialCancellation_elf_safe' depends on axioms: [propext,
 Classical.choice,
 Quot.sound,
 GoblinFormal.Track5A.evalCond_ls_subFlags64._native.bv_decide.ax_1_8,
 GoblinFormal.Track5A.stackGuard_32_ok._native.bv_decide.ax_1_5,
 GoblinFormal.Track5D.cancel_stack_headroom_832._native.bv_decide.ax_1_5✝]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5D.hasPotentialCancellation_elf_safe

/--
info: 'GoblinFormal.Track5D.hasPotentialCancellation_exactD_zeroGC_and_pb' depends on axioms: [propext,
 Classical.choice,
 Quot.sound]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5D.hasPotentialCancellation_exactD_zeroGC_and_pb

/--
info: 'GoblinFormal.Track5D.f64PosInf_top_of_notNaN' depends on axioms: [propext,
 Classical.choice,
 Quot.sound,
 GoblinFormal.Track5D.f64PosInf_top_of_notNaN._native.bv_decide.ax_1_5]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5D.f64PosInf_top_of_notNaN

/--
info: 'GoblinFormal.Track5D.not_f64Ge_eq_f64Lt' depends on axioms: [propext,
 Classical.choice,
 Quot.sound,
 GoblinFormal.Track5D.not_f64Ge_eq_f64Lt._native.bv_decide.ax_1_5]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5D.not_f64Ge_eq_f64Lt

/-- info: 'GoblinFormal.Track5D.f64Lt_eq_f64Cmp_n' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5D.f64Lt_eq_f64Cmp_n

/-- info: 'GoblinFormal.Track5D.signed_zero_and_subnormal_audit' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5D.signed_zero_and_subnormal_audit

/-- info: 'GoblinFormal.Track5D.vminpdLane_signed_zero_asymmetry' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5D.vminpdLane_signed_zero_asymmetry

/--
info: 'GoblinFormal.Track5D.vminpdLane_numKey_assoc_comm' depends on axioms: [propext,
 Classical.choice,
 Quot.sound,
 GoblinFormal.Track5D.vminpdLane_numKey_assoc_comm._native.bv_decide.ax_1_5]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5D.vminpdLane_numKey_assoc_comm

/--
info: 'GoblinFormal.Track5D.insertEntry_comm' depends on axioms: [propext,
 Classical.choice,
 Quot.sound,
 GoblinFormal.Track5D.f64TotalKey_inj._native.bv_decide.ax_1_5]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5D.insertEntry_comm

/--
info: 'GoblinFormal.Track5D.insertEntry_dropThird' depends on axioms: [propext,
 Classical.choice,
 Quot.sound,
 GoblinFormal.Track5D.f64TotalKey_inj._native.bv_decide.ax_1_5]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5D.insertEntry_dropThird

/--
info: 'GoblinFormal.Track5D.mergeMin2_insert' depends on axioms: [propext,
 Classical.choice,
 Quot.sound,
 GoblinFormal.Track5D.f64TotalKey_inj._native.bv_decide.ax_1_5]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5D.mergeMin2_insert

/--
info: 'GoblinFormal.Track5D.vec4TreeMin2_eq_fold' depends on axioms: [propext,
 Classical.choice,
 Quot.sound,
 GoblinFormal.Track5D.f64TotalKey_inj._native.bv_decide.ax_1_5]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5D.vec4TreeMin2_eq_fold

/--
info: 'GoblinFormal.Track5D.vec8TreeMin2_eq_fold' depends on axioms: [propext,
 Classical.choice,
 Quot.sound,
 GoblinFormal.Track5D.f64TotalKey_inj._native.bv_decide.ax_1_5]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5D.vec8TreeMin2_eq_fold

/--
info: 'GoblinFormal.Track5D.simdTree4Reduce_eq_scalarMin2Fold' depends on axioms: [propext,
 Classical.choice,
 Quot.sound,
 GoblinFormal.Track5D.f64Lt_iff_ult_numKey._native.bv_decide.ax_1_5,
 GoblinFormal.Track5D.f64PosInf_top_of_notNaN._native.bv_decide.ax_1_5,
 GoblinFormal.Track5D.f64TotalKey_inj._native.bv_decide.ax_1_5,
 GoblinFormal.Track5D.not_f64Ge_eq_f64Lt._native.bv_decide.ax_1_5]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5D.simdTree4Reduce_eq_scalarMin2Fold

/--
info: 'GoblinFormal.Track5D.simdTree8Reduce_eq_scalarMin2Fold' depends on axioms: [propext,
 Classical.choice,
 Quot.sound,
 GoblinFormal.Track5D.f64Lt_iff_ult_numKey._native.bv_decide.ax_1_5,
 GoblinFormal.Track5D.f64PosInf_top_of_notNaN._native.bv_decide.ax_1_5,
 GoblinFormal.Track5D.f64TotalKey_inj._native.bv_decide.ax_1_5,
 GoblinFormal.Track5D.not_f64Ge_eq_f64Lt._native.bv_decide.ax_1_5]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5D.simdTree8Reduce_eq_scalarMin2Fold

/--
info: 'GoblinFormal.Track5D.scalarMin2Fold_witness_and_minimality' depends on axioms: [propext,
 Classical.choice,
 Quot.sound,
 GoblinFormal.Track5D.f64Le_iff_ule_numKey._native.bv_decide.ax_1_5,
 GoblinFormal.Track5D.f64Lt_iff_ult_numKey._native.bv_decide.ax_1_5,
 GoblinFormal.Track5D.f64PosInf_top_of_notNaN._native.bv_decide.ax_1_5,
 GoblinFormal.Track5D.f64TotalKey_inj._native.bv_decide.ax_1_5,
 GoblinFormal.Track5D.not_f64Ge_eq_f64Lt._native.bv_decide.ax_1_5]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5D.scalarMin2Fold_witness_and_minimality

/-- info: 'GoblinFormal.Track5D.min2FilteredStep_eq_scalarMin2Step' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5D.min2FilteredStep_eq_scalarMin2Step
