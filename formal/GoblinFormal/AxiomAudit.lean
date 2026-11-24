/-
GoblinFormal.AxiomAudit: machine-checked `#guard_msgs in #print axioms` locks
and compile-time `#verify_all_goblin_axioms` environment sweep across
Common.Basic, Common.Json, and Tracks 1-5.
Enforces zero `sorryAx`, zero `Lean.trustCompiler`, zero `Lean.ofReduceBool`,
zero `_native.bv_decide` / `_native.native_decide`, and zero custom axioms:
every theorem across GoblinFormal depends solely on
`{propext, Classical.choice, Quot.sound}`.
-/

import Lean
import Apps.Common
import GoblinFormal.Common.Basic
import GoblinFormal.Common.Json
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
import GoblinFormal.Track5E

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

/-- info: 'GoblinFormal.float64BitsToRat_subnormal' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.float64BitsToRat_subnormal

/-- info: 'GoblinFormal.float64BitsToRat_normal' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.float64BitsToRat_normal

/-- info: 'GoblinFormal.float64BitsToRat_eq_ieee754' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.float64BitsToRat_eq_ieee754

/-! ### Track 1: LSAP Optimality, Forbidden Edges, Hall Infeasibility & Safe Dual Bounds -/

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

/-- info: 'GoblinFormal.Track1.checkLSAPTranspose_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track1.checkLSAPTranspose_sound

/-- info: 'GoblinFormal.Track1.checkLSAPTranspose_sound_max' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track1.checkLSAPTranspose_sound_max

/-- info: 'GoblinFormal.Track1.checkLSAPTransposeSafe_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track1.checkLSAPTransposeSafe_sound

/-- info: 'GoblinFormal.Track1.checkLSAPForbidden_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track1.checkLSAPForbidden_sound

/-- info: 'GoblinFormal.Track1.checkLSAPForbidden_sound_max' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track1.checkLSAPForbidden_sound_max

/-- info: 'GoblinFormal.Track1.checkLSAPForbiddenSafe_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track1.checkLSAPForbiddenSafe_sound

/-- info: 'GoblinFormal.Track1.checkLSAPForbiddenSafe_sound_max' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track1.checkLSAPForbiddenSafe_sound_max

/-- info: 'GoblinFormal.Track1.checkLSAPForbiddenTranspose_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track1.checkLSAPForbiddenTranspose_sound

/-- info: 'GoblinFormal.Track1.checkLSAPForbiddenTransposeSafe_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track1.checkLSAPForbiddenTransposeSafe_sound

/-- info: 'GoblinFormal.Track1.checkHallInfeasible_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track1.checkHallInfeasible_sound

/-- info: 'GoblinFormal.Track1.checkHallInfeasibleTranspose_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track1.checkHallInfeasibleTranspose_sound

/-- info: 'GoblinFormal.Track1.safe_objective_within_tol' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track1.safe_objective_within_tol

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

/-- info: 'GoblinFormal.Track2A.evalActivitySummarySpec_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2A.evalActivitySummarySpec_sound

/-- info: 'GoblinFormal.Track2A.binary_strengthen_pos_iff' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2A.binary_strengthen_pos_iff

/-- info: 'GoblinFormal.Track2A.binary_strengthen_pos_outward_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2A.binary_strengthen_pos_outward_sound

/-- info: 'GoblinFormal.Track2A.binary_strengthen_neg_iff' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2A.binary_strengthen_neg_iff

/-- info: 'GoblinFormal.Track2A.checkBinStrengthenFromRowSpec_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2A.checkBinStrengthenFromRowSpec_sound

/-- info: 'GoblinFormal.Track2A.orbit_automorphism_preserves_opt' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2A.orbit_automorphism_preserves_opt

/-- info: 'GoblinFormal.Track2A.monotone_orbit_order_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2A.monotone_orbit_order_sound

/-- info: 'GoblinFormal.Track2A.verifyOrbitSymmetrySpec_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2A.verifyOrbitSymmetrySpec_sound

/-- info: 'GoblinFormal.Track2A.orbit_chain_ordering_forced_zero_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2A.orbit_chain_ordering_forced_zero_sound

/-- info: 'GoblinFormal.Track2A.orbit_exact_chain_ordering_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2A.orbit_exact_chain_ordering_sound

/-- info: 'GoblinFormal.Track2A.orbit_orbitope_upper_tri_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2A.orbit_orbitope_upper_tri_sound

/-- info: 'GoblinFormal.Track2A.orbit_bijective_identity_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2A.orbit_bijective_identity_sound

/-! ### Track 2B: Cutting-Plane Soundness -/

/-- info: 'GoblinFormal.Track2B.gmi_cut_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2B.gmi_cut_sound

/-- info: 'GoblinFormal.Track2B.gmi_weaken_upward_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2B.gmi_weaken_upward_sound

/-- info: 'GoblinFormal.Track2B.checkTableauGMI_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2B.checkTableauGMI_sound

/-- info: 'GoblinFormal.Track2B.row_agg_legal_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2B.row_agg_legal_sound

/-- info: 'GoblinFormal.Track2B.row_agg_sum_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2B.row_agg_sum_sound

/-- info: 'GoblinFormal.Track2B.row_agg_error_bound_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2B.row_agg_error_bound_sound

/-- info: 'GoblinFormal.Track2B.checkRowAgg_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2B.checkRowAgg_sound

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

/-- info: 'GoblinFormal.Track2B.checkCMIRCertificate_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2B.checkCMIRCertificate_sound

/-- info: 'GoblinFormal.Track2B.vub_disagg_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2B.vub_disagg_sound

/-- info: 'GoblinFormal.Track2B.vub_disagg_opt_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2B.vub_disagg_opt_sound

/-- info: 'GoblinFormal.Track2B.checkVUBDisagg_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2B.checkVUBDisagg_sound

/-- info: 'GoblinFormal.Track2B.cut_drop_scale_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2B.cut_drop_scale_sound

/-! ### Track 2C: Universal Knapsack Cover, Lot-Sizing, Structural GMI/VUB, Aggregated cMIR & Cut-Drop Soundness -/

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

/-- info: 'GoblinFormal.Track2C.checkStructuralGMICert_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2C.checkStructuralGMICert_sound

/-- info: 'GoblinFormal.Track2C.checkStructuralVUBCert_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track2C.checkStructuralVUBCert_sound

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

/-! ### Track 3: LP Optimality, Safe Dual Bounds, Farkas & Bound-Conflict Certificates -/

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

/-- info: 'GoblinFormal.Track3.checkBoundConflict_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track3.checkBoundConflict_sound

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

/-- info: 'GoblinFormal.Track3B.evalGcdRowDiv_checkGcdRowDiv' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track3B.evalGcdRowDiv_checkGcdRowDiv

/-- info: 'GoblinFormal.Track3B.evalGcdRowDiv_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track3B.evalGcdRowDiv_sound

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

/-- info: 'GoblinFormal.Track4.stepWorkerAcquire_valid' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.stepWorkerAcquire_valid

/-- info: 'GoblinFormal.Track4.stepWorkerFinish_valid' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.stepWorkerFinish_valid

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

/-- info: 'GoblinFormal.Track4.exactGapClosedSpec_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.exactGapClosedSpec_sound

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

/-- info: 'GoblinFormal.Track4.checkVIPRWithPresolve_dim_eq' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.checkVIPRWithPresolve_dim_eq

/--
info: 'GoblinFormal.Track4.checkVIPRWithPresolve_optimal_constructive_sound' depends on axioms: [propext,
 Classical.choice,
 Quot.sound]
-/
#guard_msgs in
#print axioms GoblinFormal.Track4.checkVIPRWithPresolve_optimal_constructive_sound

/--
info: 'GoblinFormal.Track4.checkVIPRWithPresolve_optimal_same_n_sound' depends on axioms: [propext,
 Classical.choice,
 Quot.sound]
-/
#guard_msgs in
#print axioms GoblinFormal.Track4.checkVIPRWithPresolve_optimal_same_n_sound

/-- info: 'GoblinFormal.Track4.checkVIPRWithPresolve_eps_optimal_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.checkVIPRWithPresolve_eps_optimal_sound

/--
info: 'GoblinFormal.Track4.checkVIPRWithPresolve_eps_optimal_zero_tol' depends on axioms: [propext,
 Classical.choice,
 Quot.sound]
-/
#guard_msgs in
#print axioms GoblinFormal.Track4.checkVIPRWithPresolve_eps_optimal_zero_tol

/-- info: 'GoblinFormal.Track4.checkVIPRWithPresolve_bound_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track4.checkVIPRWithPresolve_bound_sound

/-! ### Track 5A: Tier 5A ARM64 Leaf Binary Equivalence -/

/-- info: 'GoblinFormal.Track5A.splitmix64_arm64_equiv' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5A.splitmix64_arm64_equiv

/-- info: 'GoblinFormal.Track5A.canonicalFloatBitsInlined_arm64_equiv' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5A.canonicalFloatBitsInlined_arm64_equiv

/-- info: 'GoblinFormal.Track5A.canonicalFloatBits_arm64_equiv' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5A.canonicalFloatBits_arm64_equiv

/-- info: 'GoblinFormal.Track5A.divCeilInt64_arm64_equiv' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5A.divCeilInt64_arm64_equiv

/-- info: 'GoblinFormal.Track5A.gcdInt64_arm64_equiv' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5A.gcdInt64_arm64_equiv

/-! ### Track 5B: Tier 5B Zero-Alloc ARM64 Solver Kernels -/

/-- info: 'GoblinFormal.Track5B.checkZeroGC_run_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5B.checkZeroGC_run_sound

/-- info: 'GoblinFormal.Track5B.checkStackGuard_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5B.checkStackGuard_sound

/-- info: 'GoblinFormal.Track5B.checkPanicBoundsReachability_run_safe' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5B.checkPanicBoundsReachability_run_safe

/--
info: 'GoblinFormal.Track5B.checkPanicBoundsReachability_run_unconditional' depends on axioms: [propext,
 Classical.choice,
 Quot.sound]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5B.checkPanicBoundsReachability_run_unconditional

/-- info: 'GoblinFormal.Track5B.hasInfeasibleIntegerEqRow_elf_safe' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5B.hasInfeasibleIntegerEqRow_elf_safe

/-- info: 'GoblinFormal.Track5B.augmentRowScalar_elf_safe' depends on axioms: [propext, Classical.choice, Quot.sound] -/
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
 Quot.sound]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5B.hasInfeasibleIntegerEqRow_infeasible_gcd_exec

/--
info: 'GoblinFormal.Track5B.hasInfeasibleIntegerEqRow_feasible_gcd_exec' depends on axioms: [propext,
 Classical.choice,
 Quot.sound]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5B.hasInfeasibleIntegerEqRow_feasible_gcd_exec

/-- info: 'GoblinFormal.Track5B.augmentRowScalar_concrete_exec' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5B.augmentRowScalar_concrete_exec

/-! ### Track 5C: Tier 5C amd64 SIMD Vector ISA & CFG Safety -/

/-- info: 'GoblinFormal.Track5C.checkVectorCFG_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5C.checkVectorCFG_sound

/--
info: 'GoblinFormal.Track5C.unguardedSucc_fallthroughGuarded_keeps_target' depends on axioms: [propext,
 Classical.choice,
 Quot.sound]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5C.unguardedSucc_fallthroughGuarded_keeps_target

/--
info: 'GoblinFormal.Track5C.unguardedSucc_targetGuarded_keeps_fallthrough' depends on axioms: [propext,
 Classical.choice,
 Quot.sound]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5C.unguardedSucc_targetGuarded_keeps_fallthrough

/-- info: 'GoblinFormal.Track5C.unguardedSucc_ambiguous_keeps_both' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5C.unguardedSucc_ambiguous_keeps_both

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

/-- info: 'GoblinFormal.Track5D.exactReducedCostNoinline_elf_safe' depends on axioms: [propext, Classical.choice, Quot.sound] -/
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

/-- info: 'GoblinFormal.Track5D.hasPotentialCancellation_elf_safe' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5D.hasPotentialCancellation_elf_safe

/--
info: 'GoblinFormal.Track5D.hasPotentialCancellation_exactD_zeroGC_and_pb' depends on axioms: [propext,
 Classical.choice,
 Quot.sound]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5D.hasPotentialCancellation_exactD_zeroGC_and_pb

/-- info: 'GoblinFormal.Track5D.f64PosInf_top_of_notNaN' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5D.f64PosInf_top_of_notNaN

/-- info: 'GoblinFormal.Track5D.not_f64Ge_eq_f64Lt' depends on axioms: [propext, Classical.choice, Quot.sound] -/
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

/-- info: 'GoblinFormal.Track5D.vminpdLane_numKey_assoc_comm' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5D.vminpdLane_numKey_assoc_comm

/-- info: 'GoblinFormal.Track5D.insertEntry_comm' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5D.insertEntry_comm

/-- info: 'GoblinFormal.Track5D.insertEntry_dropThird' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5D.insertEntry_dropThird

/-- info: 'GoblinFormal.Track5D.mergeMin2_insert' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5D.mergeMin2_insert

/-- info: 'GoblinFormal.Track5D.vec4TreeMin2_eq_fold' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5D.vec4TreeMin2_eq_fold

/-- info: 'GoblinFormal.Track5D.vec8TreeMin2_eq_fold' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5D.vec8TreeMin2_eq_fold

/-- info: 'GoblinFormal.Track5D.simdTree4Reduce_eq_scalarMin2Fold' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5D.simdTree4Reduce_eq_scalarMin2Fold

/-- info: 'GoblinFormal.Track5D.simdTree8Reduce_eq_scalarMin2Fold' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5D.simdTree8Reduce_eq_scalarMin2Fold

/-- info: 'GoblinFormal.Track5D.scalarMin2Fold_witness_and_minimality' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5D.scalarMin2Fold_witness_and_minimality

/-- info: 'GoblinFormal.Track5D.min2FilteredStep_eq_scalarMin2Step' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5D.min2FilteredStep_eq_scalarMin2Step

/-! ### Track 5E: Tier 5E Decoded amd64 AVX2 / AVX-512 Machine-Code Equivalence -/

/-- info: 'GoblinFormal.Track5E.decodeAmd64ProgramChunks_eq' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.decodeAmd64ProgramChunks_eq

/-- info: 'GoblinFormal.Track5E.allAbsLE64AVX2_decodes' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.allAbsLE64AVX2_decodes

/-- info: 'GoblinFormal.Track5E.allAbsLE64AVX512_decodes' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.allAbsLE64AVX512_decodes

/-- info: 'GoblinFormal.Track5E.minNoNaN64AVX2_decodes' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.minNoNaN64AVX2_decodes

/-- info: 'GoblinFormal.Track5E.minNoNaN64AVX512_decodes' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.minNoNaN64AVX512_decodes

/-- info: 'GoblinFormal.Track5E.initReducedRow64AVX2_decodes' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.initReducedRow64AVX2_decodes

/-- info: 'GoblinFormal.Track5E.initReducedRow64AVX512_decodes' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.initReducedRow64AVX512_decodes

/--
info: 'GoblinFormal.Track5E.evex_broadcast_and_zeromask_decode_witness' depends on axioms: [propext,
 Classical.choice,
 Quot.sound]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5E.evex_broadcast_and_zeromask_decode_witness

/-- info: 'GoblinFormal.Track5E.checkDecodedAbiSafety_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.checkDecodedAbiSafety_sound

/-- info: 'GoblinFormal.Track5E.checkDecodedFrameAbi_sound' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.checkDecodedFrameAbi_sound

/-- info: 'GoblinFormal.Track5E.allAbsLE64AVX2_abi_and_zeroGC_safe' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.allAbsLE64AVX2_abi_and_zeroGC_safe

/-- info: 'GoblinFormal.Track5E.allAbsLE64AVX512_abi_and_zeroGC_safe' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.allAbsLE64AVX512_abi_and_zeroGC_safe

/-- info: 'GoblinFormal.Track5E.minNoNaN64AVX2_abi_and_zeroGC_safe' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.minNoNaN64AVX2_abi_and_zeroGC_safe

/-- info: 'GoblinFormal.Track5E.minNoNaN64AVX512_abi_and_zeroGC_safe' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.minNoNaN64AVX512_abi_and_zeroGC_safe

/--
info: 'GoblinFormal.Track5E.initReducedRow64AVX2_abi_and_zeroGC_safe' depends on axioms: [propext,
 Classical.choice,
 Quot.sound]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5E.initReducedRow64AVX2_abi_and_zeroGC_safe

/--
info: 'GoblinFormal.Track5E.initReducedRow64AVX512_abi_and_zeroGC_safe' depends on axioms: [propext,
 Classical.choice,
 Quot.sound]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5E.initReducedRow64AVX512_abi_and_zeroGC_safe

/-- info: 'GoblinFormal.Track5E.allAbsLE64AVX2_abi_reg_preserved' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.allAbsLE64AVX2_abi_reg_preserved

/-- info: 'GoblinFormal.Track5E.allAbsLE64AVX512_abi_reg_preserved' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.allAbsLE64AVX512_abi_reg_preserved

/-- info: 'GoblinFormal.Track5E.minNoNaN64AVX2_abi_reg_preserved' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.minNoNaN64AVX2_abi_reg_preserved

/-- info: 'GoblinFormal.Track5E.minNoNaN64AVX512_abi_reg_preserved' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.minNoNaN64AVX512_abi_reg_preserved

/-- info: 'GoblinFormal.Track5E.initReducedRow64AVX2_abi_reg_preserved' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.initReducedRow64AVX2_abi_reg_preserved

/--
info: 'GoblinFormal.Track5E.initReducedRow64AVX512_abi_reg_preserved' depends on axioms: [propext,
 Classical.choice,
 Quot.sound]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5E.initReducedRow64AVX512_abi_reg_preserved

/-- info: 'GoblinFormal.Track5E.stripped_vzeroupper_rejected' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.stripped_vzeroupper_rejected

/-- info: 'GoblinFormal.Track5E.minsd_por_eq_f64Min' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.minsd_por_eq_f64Min

/-- info: 'GoblinFormal.Track5E.vminpd_track5d_canon_bridge' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.vminpd_track5d_canon_bridge

/-- info: 'GoblinFormal.Track5E.allAbsLE64AVX2_functional_equiv' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.allAbsLE64AVX2_functional_equiv

/-- info: 'GoblinFormal.Track5E.allAbsLE64AVX512_functional_equiv' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.allAbsLE64AVX512_functional_equiv

/-- info: 'GoblinFormal.Track5E.allAbsLE64_avx2_eq_avx512_mult8' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.allAbsLE64_avx2_eq_avx512_mult8

/-- info: 'GoblinFormal.Track5E.minNoNaN64AVX2_functional_equiv' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.minNoNaN64AVX2_functional_equiv

/-- info: 'GoblinFormal.Track5E.minNoNaN64AVX512_functional_equiv' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.minNoNaN64AVX512_functional_equiv

/-- info: 'GoblinFormal.Track5E.minNoNaN64_avx2_eq_avx512' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.minNoNaN64_avx2_eq_avx512

/-- info: 'GoblinFormal.Track5E.initReducedRow64AVX2_functional_equiv' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.initReducedRow64AVX2_functional_equiv

/--
info: 'GoblinFormal.Track5E.initReducedRow64AVX512_functional_equiv' depends on axioms: [propext,
 Classical.choice,
 Quot.sound]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5E.initReducedRow64AVX512_functional_equiv

/-- info: 'GoblinFormal.Track5E.initReducedRow64_avx2_eq_avx512_mult8' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.initReducedRow64_avx2_eq_avx512_mult8

/-- info: 'GoblinFormal.Track5E.rexB_0x90_not_nop' depends on axioms: [propext, Classical.choice, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.rexB_0x90_not_nop

/-- info: 'GoblinFormal.Track5E.byteRangeDisjoint64_of_aligned_ne' depends on axioms: [propext, Quot.sound] -/
#guard_msgs in
#print axioms GoblinFormal.Track5E.byteRangeDisjoint64_of_aligned_ne

/--
info: 'GoblinFormal.Track5E.allAbsLE64AVX2_aligned_byteDisjoint_equiv' depends on axioms: [propext,
 Classical.choice,
 Quot.sound]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5E.allAbsLE64AVX2_aligned_byteDisjoint_equiv

/--
info: 'GoblinFormal.Track5E.allAbsLE64AVX512_aligned_byteDisjoint_equiv' depends on axioms: [propext,
 Classical.choice,
 Quot.sound]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5E.allAbsLE64AVX512_aligned_byteDisjoint_equiv

/--
info: 'GoblinFormal.Track5E.minNoNaN64AVX2_aligned_byteDisjoint_equiv' depends on axioms: [propext,
 Classical.choice,
 Quot.sound]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5E.minNoNaN64AVX2_aligned_byteDisjoint_equiv

/--
info: 'GoblinFormal.Track5E.minNoNaN64AVX512_aligned_byteDisjoint_equiv' depends on axioms: [propext,
 Classical.choice,
 Quot.sound]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5E.minNoNaN64AVX512_aligned_byteDisjoint_equiv

/--
info: 'GoblinFormal.Track5E.initReducedRow64AVX2_aligned_byteDisjoint_equiv' depends on axioms: [propext,
 Classical.choice,
 Quot.sound]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5E.initReducedRow64AVX2_aligned_byteDisjoint_equiv

/--
info: 'GoblinFormal.Track5E.initReducedRow64AVX512_aligned_byteDisjoint_equiv' depends on axioms: [propext,
 Classical.choice,
 Quot.sound]
-/
#guard_msgs in
#print axioms GoblinFormal.Track5E.initReducedRow64AVX512_aligned_byteDisjoint_equiv

/-! ### Exhaustive Compile-Time Environment Axiom Audit -/

open Lean Elab Command in
local elab "#verify_all_goblin_axioms" : command => do
  let env ← getEnv
  let allowed : List Name := [``propext, ``Classical.choice, ``Quot.sound]
  let isGoblinMod (m : String) : Bool :=
    m == "GoblinFormal" || m.startsWith "GoblinFormal." ||
    m == "Apps" || m.startsWith "Apps."
  let modNameOf (declName : Name) : String :=
    match env.getModuleIdxFor? declName with
    | some idx =>
      if h : idx.toNat < env.header.moduleNames.size then
        env.header.moduleNames[idx.toNat].toString
      else
        ""
    | none => env.mainModule.toString
  let mut axiomDecls : List Name := []
  let mut badDecls : List (Name × List Name) := []
  let mut thmCount : Nat := 0
  for (declName, cinfo) in env.constants.toList do
    let modName := modNameOf declName
    if isGoblinMod modName then
      match cinfo with
      | .axiomInfo _ =>
        axiomDecls := declName :: axiomDecls
      | .thmInfo _ =>
        thmCount := thmCount + 1
        let axs ← liftCoreM <| collectAxioms declName
        let forbidden := axs.toList.filter (fun a => !allowed.contains a)
        if !forbidden.isEmpty then
          badDecls := (declName, forbidden) :: badDecls
      | _ =>
        if modName != "GoblinFormal.AxiomAudit" then
          let axs ← liftCoreM <| collectAxioms declName
          let forbidden := axs.toList.filter (fun a => !allowed.contains a)
          if !forbidden.isEmpty then
            badDecls := (declName, forbidden) :: badDecls
  if !axiomDecls.isEmpty then
    throwError m!"GoblinFormal/Apps declares forbidden custom/native axioms: {axiomDecls}"
  if !badDecls.isEmpty then
    throwError m!"GoblinFormal/Apps declarations depend on unpermitted axioms: {badDecls}"
  if thmCount == 0 then
    throwError "GoblinFormal axiom audit found 0 theorems"

#verify_all_goblin_axioms

open Lean Elab Command in
/-- Audits all 8 `Apps.*` CLI modules individually via `Lean.importModules` (avoiding `_root_.main`
collisions across the 7 executable roots) and verifies zero custom/forbidden axioms. -/
def GoblinFormal.AxiomAudit.auditAllAppsModules : CommandElabM Unit := do
  let allowed : List Name := [``propext, ``Classical.choice, ``Quot.sound]
  let appMods : List Name := [
    `Apps.Common, `Apps.BinCheck, `Apps.CutsCheck, `Apps.LpCheck,
    `Apps.LsapCheck, `Apps.RulesCheck, `Apps.SimdCfgCheck, `Apps.ViprCheck
  ]
  for modName in appMods do
    let mEnv ← importModules #[{ module := modName }] {}
    let (declCount, axDecls, badDecls) ← liftCoreM <| withEnv mEnv do
      let mut axDecls : List Name := []
      let mut badDecls : List (Name × List Name) := []
      let mut declCount : Nat := 0
      for (declName, cinfo) in mEnv.constants.toList do
        let mn :=
          match mEnv.getModuleIdxFor? declName with
          | some idx =>
            if h : idx.toNat < mEnv.header.moduleNames.size then
              mEnv.header.moduleNames[idx.toNat].toString
            else ""
          | none => mEnv.mainModule.toString
        if mn == "Apps" || mn.startsWith "Apps." then
          declCount := declCount + 1
          match cinfo with
          | .axiomInfo _ => axDecls := declName :: axDecls
          | _ =>
            let axs ← collectAxioms declName
            let forbidden := axs.toList.filter (fun a => !allowed.contains a)
            if !forbidden.isEmpty then
              badDecls := (declName, forbidden) :: badDecls
      pure (declCount, axDecls, badDecls)
    if declCount == 0 then
      throwError m!"Apps axiom audit found 0 declarations in {modName}"
    if !axDecls.isEmpty then
      throwError m!"{modName} declares forbidden custom/native axioms: {axDecls}"
    if !badDecls.isEmpty then
      throwError m!"{modName} declarations depend on unpermitted axioms: {badDecls}"
