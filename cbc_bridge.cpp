//go:build cgo && cbc

#include "cbc_bridge.h"

#include <algorithm>
#include <cmath>
#include <cstdio>
#include <cstring>
#include <deque>
#include <limits>
#include <mutex>
#include <string>
#include <vector>

#include "CoinFinite.hpp"
#include "ClpEventHandler.hpp"
#include "ClpSimplex.hpp"
#include "ClpSolve.hpp"
#include "OsiClpSolverInterface.hpp"
#include "CbcEventHandler.hpp"
#include "CbcModel.hpp"
#include "CbcCutGenerator.hpp"
#include "CbcSolver.hpp"

namespace {

// COIN-OR Cgl (specifically CglZeroHalf/Cgl012cut.cpp) and CbcModel
// (openblas_set_num_threads) mutate unsynchronized file-scope static state
// during branch-and-cut. Serializing C++ solver invocations at the bridge
// boundary guarantees memory safety when Go callers invoke goblin concurrently
// from multiple goroutines.
std::mutex g_cbc_mutex;

thread_local int tl_total_cuts = 0;
thread_local double tl_root_lp_min = 0.0;
thread_local double tl_root_cut_min = 0.0;
thread_local bool tl_has_root_lp = false;
thread_local bool tl_has_root_cut = false;
thread_local const int *tl_cancel_flag = nullptr;
thread_local int tl_max_iter = 0;
thread_local bool tl_iter_limit_hit = false;

class GoblinClpCancelHandler : public ClpEventHandler {
public:
    GoblinClpCancelHandler(const int *cancel_flag, int max_iter = 0, bool *iter_hit = nullptr, bool *cutoff_hit = nullptr)
        : ClpEventHandler(), cancel_flag_(cancel_flag), max_iter_(max_iter), iter_hit_(iter_hit), cutoff_hit_(cutoff_hit) {}
    GoblinClpCancelHandler(const GoblinClpCancelHandler &rhs)
        : ClpEventHandler(rhs),
          cancel_flag_(rhs.cancel_flag_),
          max_iter_(rhs.max_iter_),
          iter_hit_(rhs.iter_hit_),
          cutoff_hit_(rhs.cutoff_hit_) {}
    GoblinClpCancelHandler &operator=(const GoblinClpCancelHandler &rhs) {
        if (this != &rhs) {
            ClpEventHandler::operator=(rhs);
            cancel_flag_ = rhs.cancel_flag_;
            max_iter_ = rhs.max_iter_;
            iter_hit_ = rhs.iter_hit_;
            cutoff_hit_ = rhs.cutoff_hit_;
        }
        return *this;
    }
    ClpEventHandler *clone() const override {
        return new GoblinClpCancelHandler(*this);
    }
    int event(Event) override {
        if (cutoff_hit_ && model_) {
            if (model_->isDualObjectiveLimitReached() || model_->objectiveValue() >= 1e48) {
                *cutoff_hit_ = true;
            }
        }
        if (cancel_flag_ && __atomic_load_n(cancel_flag_, __ATOMIC_RELAXED) != 0) {
            return 0; // Returning zero sets ClpModel status to five (stopped by event).
        }
        if (max_iter_ > 0 && model_ && model_->numberIterations() >= max_iter_) {
            if (iter_hit_) {
                *iter_hit_ = true;
            }
            return 0;
        }
        return -1;
    }

private:
    const int *cancel_flag_;
    int max_iter_;
    bool *iter_hit_;
    bool *cutoff_hit_;
};

class GoblinCbcCancelHandler : public CbcEventHandler {
public:
    GoblinCbcCancelHandler(const int *cancel_flag, int max_iter = 0, bool *iter_hit = nullptr)
        : CbcEventHandler(), cancel_flag_(cancel_flag), max_iter_(max_iter), iter_hit_(iter_hit) {}
    GoblinCbcCancelHandler(const GoblinCbcCancelHandler &rhs)
        : CbcEventHandler(rhs),
          cancel_flag_(rhs.cancel_flag_),
          max_iter_(rhs.max_iter_),
          iter_hit_(rhs.iter_hit_) {}
    GoblinCbcCancelHandler &operator=(const GoblinCbcCancelHandler &rhs) {
        if (this != &rhs) {
            CbcEventHandler::operator=(rhs);
            cancel_flag_ = rhs.cancel_flag_;
            max_iter_ = rhs.max_iter_;
            iter_hit_ = rhs.iter_hit_;
        }
        return *this;
    }
    CbcEventHandler *clone() const override {
        return new GoblinCbcCancelHandler(*this);
    }
    CbcAction event(CbcEvent) override {
        if (cancel_flag_ && __atomic_load_n(cancel_flag_, __ATOMIC_RELAXED) != 0) {
            return CbcEventHandler::stop;
        }
        if (max_iter_ > 0 && model_ && model_->getIterationCount() >= max_iter_) {
            if (iter_hit_) {
                *iter_hit_ = true;
            }
            return CbcEventHandler::stop;
        }
        return CbcEventHandler::noAction;
    }

private:
    const int *cancel_flag_;
    int max_iter_;
    bool *iter_hit_;
};

int goblin_cbc_callback(CbcModel *currentSolver, int whereFrom) {
    if (!currentSolver) {
        return 0;
    }
    if (tl_cancel_flag && __atomic_load_n(tl_cancel_flag, __ATOMIC_RELAXED) != 0) {
        return 1;
    }
    if (tl_max_iter > 0 && currentSolver->getIterationCount() >= tl_max_iter) {
        tl_iter_limit_hit = true;
        return 1;
    }
    if (whereFrom == 1) {
        if (currentSolver->solver() && currentSolver->solver()->isProvenOptimal()) {
            tl_root_lp_min = currentSolver->solver()->getObjValue();
            tl_has_root_lp = true;
        }
    } else if (whereFrom == 4 || whereFrom == 5) {
        int nGen = currentSolver->numberCutGenerators();
        int totalCuts = 0;
        for (int i = 0; i < nGen; ++i) {
            CbcCutGenerator *gen = currentSolver->cutGenerator(i);
            if (gen) {
                totalCuts += gen->numberCutsInTotal() + gen->numberColumnCuts();
            }
        }
        if (totalCuts > tl_total_cuts) {
            tl_total_cuts = totalCuts;
        }
        double contObj = currentSolver->getContinuousObjective();
        if (std::fabs(contObj) < 1e40 && !tl_has_root_lp) {
            tl_root_lp_min = contObj;
            tl_has_root_lp = true;
        }
        double cutObj = currentSolver->rootObjectiveAfterCuts();
        if (std::fabs(cutObj) < 1e40) {
            tl_root_cut_min = cutObj;
            tl_has_root_cut = true;
        }
    }
    return 0;
}

void configure_silent_clp(OsiClpSolverInterface &solver) {
    solver.messageHandler()->setLogLevel(0);
    solver.setHintParam(OsiDoReducePrint, true, OsiHintTry);
    ClpSimplex *clp = solver.getModelPtr();
    if (clp) {
        clp->setLogLevel(0);
    }
    ClpSolve solveOpts;
    solveOpts.setSpecialOption(2, 1); // disable SIGINT handler in ClpSolve
    solver.setSolveOptions(solveOpts);
}

bool has_clp_tiny_coeff_within(
    int num_cols,
    const int *col_ptr,
    const double *col_val,
    const double *col_lb,
    const double *col_ub,
    double max_tiny
) {
    if (num_cols <= 0 || !col_ptr || !col_val) {
        return false;
    }
    for (int j = 0; j < num_cols; ++j) {
        double maxB = 1.0;
        if (col_lb) {
            maxB = std::max(maxB, std::fabs(col_lb[j]));
        }
        if (col_ub) {
            maxB = std::max(maxB, std::fabs(col_ub[j]));
        }
        int start = col_ptr[j];
        int end = col_ptr[j + 1];
        for (int k = start; k < end; ++k) {
            double av = std::fabs(col_val[k]);
            if (av > 0.0 && av <= max_tiny && (maxB >= 1e30 || av * maxB >= 1e-7)) {
                return true;
            }
        }
    }
    return false;
}

bool has_clp_dropped_tiny_coeff(
    int num_cols,
    const int *col_ptr,
    const double *col_val,
    const double *col_lb,
    const double *col_ub
) {
    return has_clp_tiny_coeff_within(num_cols, col_ptr, col_val, col_lb, col_ub, 1e-18);
}

bool has_clp_presolve_tiny_coeff(
    int num_cols,
    const int *col_ptr,
    const double *col_val,
    const double *col_lb,
    const double *col_ub
) {
    return has_clp_tiny_coeff_within(num_cols, col_ptr, col_val, col_lb, col_ub, 1e-9);
}

void raise_clp_large_value_if_needed(
    OsiClpSolverInterface &solver,
    int num_cols,
    const double *col_lb,
    const double *col_ub,
    int num_rows,
    const double *row_lb,
    const double *row_ub
) {
    ClpSimplex *clp = solver.getModelPtr();
    if (!clp) {
        return;
    }
    double maxFin = 0.0;
    for (int j = 0; j < num_cols; ++j) {
        if (col_lb) {
            double a = std::fabs(col_lb[j]);
            if (a < 1e29 && a > maxFin) maxFin = a;
        }
        if (col_ub) {
            double a = std::fabs(col_ub[j]);
            if (a < 1e29 && a > maxFin) maxFin = a;
        }
    }
    for (int i = 0; i < num_rows; ++i) {
        if (row_lb) {
            double a = std::fabs(row_lb[i]);
            if (a < 1e29 && a > maxFin) maxFin = a;
        }
        if (row_ub) {
            double a = std::fabs(row_ub[i]);
            if (a < 1e29 && a > maxFin) maxFin = a;
        }
    }
    if (maxFin >= 1e18) {
        double wantLarge = std::min(1e30, 100.0 * maxFin);
        if (wantLarge > clp->largeValue()) {
            clp->setLargeValue(wantLarge);
        }
    }
}

void scan_clp_tiny_obj(
    int num_cols,
    const double *col_lb,
    const double *col_ub,
    const double *obj,
    const double *orig_obj,
    bool &dropped_obj,
    double &min_sig_obj
) {
    dropped_obj = false;
    min_sig_obj = std::numeric_limits<double>::infinity();
    if (num_cols <= 0 || !obj) {
        return;
    }
    for (int j = 0; j < num_cols; ++j) {
        double lb = col_lb ? col_lb[j] : 0.0;
        double ub = col_ub ? col_ub[j] : 0.0;
        if (lb >= ub) {
            continue;
        }
        bool unbounded = (std::fabs(lb) >= 1e30 || std::fabs(ub) >= 1e30 || !std::isfinite(ub - lb));
        double span = unbounded ? std::numeric_limits<double>::infinity() : (ub - lb);
        double ao = std::fabs(obj[j]);
        double orig_ao = orig_obj ? std::fabs(orig_obj[j]) : ao;
        if (orig_ao <= 0.0 || (!unbounded && orig_ao * span < 1e-7)) {
            continue;
        }
        if (ao <= 1e-18) {
            dropped_obj = true;
        } else if (ao <= 1e-5 && ao < min_sig_obj) {
            min_sig_obj = ao;
        }
    }
}

double recompute_wide_dot(int n, const double *a_vec, const double *b_vec) {
    constexpr int kWideShift = 1056;
    constexpr int kWideHalf = 528;
    constexpr int kWideCut = 960;
    double hiS = 0.0, hiC = 0.0, midS = 0.0, midC = 0.0;
    auto addTwoSum = [](double &s, double &c, double p, double pe) {
        double ns = s + p;
        double bb = ns - s;
        double se = (s - (ns - bb)) + (p - bb);
        s = ns;
        c += pe + se;
    };
    for (int j = 0; j < n; ++j) {
        double xj = a_vec[j];
        double cj = b_vec[j];
        if (xj == 0.0 || cj == 0.0) {
            continue;
        }
        if (!std::isfinite(xj) || !std::isfinite(cj)) {
            return std::numeric_limits<double>::quiet_NaN();
        }
        int ex = 0, ec = 0;
        std::frexp(xj, &ex);
        std::frexp(cj, &ec);
        if (ex + ec > kWideCut) {
            double a = std::ldexp(xj, -kWideHalf);
            double b = std::ldexp(cj, -kWideHalf);
            double p = a * b;
            addTwoSum(hiS, hiC, p, std::fma(a, b, -p));
        } else {
            double p = xj * cj;
            addTwoSum(midS, midC, p, std::fma(xj, cj, -p));
        }
    }
    if (!std::isfinite(hiS) || !std::isfinite(hiC) || !std::isfinite(midS) || !std::isfinite(midC)) {
        return std::numeric_limits<double>::quiet_NaN();
    }
    auto finishTwoSum = [](double s, double c, double &outS, double &outRem) {
        outS = s + c;
        double bb = outS - s;
        outRem = (s - (outS - bb)) + (c - bb);
    };
    double hiTot = 0.0, hiRem = 0.0, midTot = 0.0, midRem = 0.0;
    finishTwoSum(hiS, hiC, hiTot, hiRem);
    finishTwoSum(midS, midC, midTot, midRem);
    if (hiTot == 0.0 && hiRem == 0.0) {
        return midTot + midRem;
    }
    double hiHalf = std::ldexp(hiTot, kWideHalf);
    if (!std::isfinite(hiHalf)) {
        return hiHalf;
    }
    double mHalf = std::ldexp(midTot, -kWideHalf);
    double s1 = hiHalf + mHalf;
    double bb1 = s1 - hiHalf;
    double e1 = (hiHalf - (s1 - bb1)) + (mHalf - bb1);
    double remHalf = e1 + std::ldexp(hiRem, kWideHalf) + std::ldexp(midRem, -kWideHalf);
    double totHalf = 0.0, totRemHalf = 0.0;
    finishTwoSum(s1, remHalf, totHalf, totRemHalf);
    double res = std::ldexp(totHalf, kWideHalf);
    if (!std::isfinite(res)) {
        return res;
    }
    double comb = res + std::ldexp(totRemHalf, kWideHalf);
    return std::isnan(comb) ? res : comb;
}

} // namespace

struct GoblinLPSolver {
    OsiClpSolverInterface solver;
    bool initialized;
    bool presolve_disabled;
    int num_cols;
    int num_rows;
    std::vector<double> col_lb;
    std::vector<double> col_ub;
    std::vector<double> obj_min;
    std::vector<double> max_dropped_tiny;

    GoblinLPSolver() : initialized(false), presolve_disabled(false), num_cols(0), num_rows(0) {
        configure_silent_clp(solver);
    }

    void disable_initial_presolve() {
        if (presolve_disabled) {
            return;
        }
        presolve_disabled = true;
        ClpSolve solveOpts;
        solveOpts.setSpecialOption(2, 1);
        solveOpts.setPresolveType(ClpSolve::presolveOff);
        solver.setHintParam(OsiDoPresolveInInitial, false, OsiHintTry);
        solver.setSolveOptions(solveOpts);
    }

    void set_dual_tolerance(double dual_tol) {
        double dt = std::max(1e-19, dual_tol);
        solver.setDblParam(OsiDualTolerance, dt);
        if (solver.getModelPtr()) {
            solver.getModelPtr()->setDualTolerance(dt);
        }
    }

    bool refresh_tiny_obj_and_check_dropped() {
        bool droppedObj = false;
        double minSigObj = std::numeric_limits<double>::infinity();
        if (num_cols > 0 && !obj_min.empty()) {
            scan_clp_tiny_obj(num_cols, col_lb.data(), col_ub.data(), obj_min.data(), obj_min.data(), droppedObj, minSigObj);
        }
        if (std::isfinite(minSigObj)) {
            if (!initialized) {
                disable_initial_presolve();
            }
            double wantTol = std::max(1e-19, std::min(1e-9, 0.01 * minSigObj));
            double curTol = 1e-7;
            solver.getDblParam(OsiDualTolerance, curTol);
            if (wantTol < curTol) {
                set_dual_tolerance(wantTol);
            }
        }
        return droppedObj;
    }

    bool has_dropped_tiny_for_current_bounds() {
        if (refresh_tiny_obj_and_check_dropped()) {
            return true;
        }
        for (int j = 0; j < num_cols; ++j) {
            if (col_lb[j] >= col_ub[j] && col_lb[j] == 0.0) {
                // Zero-fixed column contributes zero to every row.
                continue;
            }
            double av = max_dropped_tiny[j];
            if (av <= 0.0) {
                continue;
            }
            double maxB = std::max(1.0, std::max(std::fabs(col_lb[j]), std::fabs(col_ub[j])));
            if (maxB >= 1e30 || av * maxB >= 1e-7) {
                return true;
            }
        }
        return false;
    }
};

extern "C" {

void goblin_solve_lp(
    int num_cols,
    int num_rows,
    const int *col_ptr,
    const int *row_idx,
    const double *col_val,
    const double *col_lb,
    const double *col_ub,
    const double *obj_min,
    const double *row_lb,
    const double *row_ub,
    int enable_presolve,
    int max_iter,
    double time_limit_sec,
    const int *cancel_flag,
    GoblinResult *out,
    double *primal_out,
    double *dual_out,
    double *rc_out
) {
    std::lock_guard<std::mutex> lock(g_cbc_mutex);
    if (!out) return;
    std::memset(out, 0, sizeof(GoblinResult));
    if (cancel_flag && __atomic_load_n(cancel_flag, __ATOMIC_RELAXED) != 0) {
        out->status = -4; // StatusTimeLimit
        return;
    }
    try {

    bool iterHit = false;
    OsiClpSolverInterface solver;
    configure_silent_clp(solver);

    static const int dummy_ptr[1] = {0};
    const int *c_ptr = col_ptr ? col_ptr : dummy_ptr;
    bool droppedObj = false;
    double minSigObj = std::numeric_limits<double>::infinity();
    scan_clp_tiny_obj(num_cols, col_lb, col_ub, obj_min, obj_min, droppedObj, minSigObj);
    const bool forceTightDual = (enable_presolve < 0);
    const bool droppedTiny = droppedObj || has_clp_dropped_tiny_coeff(num_cols, c_ptr, col_val, col_lb, col_ub);
    const bool effPresolve = (enable_presolve > 0) && !std::isfinite(minSigObj) &&
                             !has_clp_presolve_tiny_coeff(num_cols, c_ptr, col_val, col_lb, col_ub);

    solver.loadProblem(
        num_cols,
        num_rows,
        c_ptr,
        row_idx,
        col_val,
        col_lb,
        col_ub,
        obj_min,
        row_lb,
        row_ub
    );
    solver.setObjSense(1.0);
    raise_clp_large_value_if_needed(solver, num_cols, col_lb, col_ub, num_rows, row_lb, row_ub);
    if (forceTightDual || std::isfinite(minSigObj)) {
        double dualTol = forceTightDual ? 1e-16 : std::max(1e-19, std::min(1e-9, 0.01 * minSigObj));
        if (forceTightDual && std::isfinite(minSigObj)) {
            dualTol = std::min(dualTol, std::max(1e-19, 0.01 * minSigObj));
        }
        solver.setDblParam(OsiDualTolerance, dualTol);
        if (solver.getModelPtr()) {
            solver.getModelPtr()->setDualTolerance(dualTol);
        }
    }

    ClpSolve solveOpts;
    solveOpts.setSpecialOption(2, 1);
    if (!effPresolve) {
        solveOpts.setPresolveType(ClpSolve::presolveOff);
        solver.setHintParam(OsiDoPresolveInInitial, false, OsiHintTry);
    } else {
        solveOpts.setPresolveType(ClpSolve::presolveOn);
        solver.setHintParam(OsiDoPresolveInInitial, true, OsiHintTry);
    }
    solver.setSolveOptions(solveOpts);

    if (max_iter > 0) {
        solver.setIntParam(OsiMaxNumIteration, max_iter);
        if (solver.getModelPtr()) {
            solver.getModelPtr()->setMaximumIterations(max_iter);
        }
    }
    if (time_limit_sec > 0.0 && solver.getModelPtr()) {
        solver.getModelPtr()->setMaximumWallSeconds(time_limit_sec);
    }
    if ((cancel_flag || max_iter > 0) && solver.getModelPtr()) {
        GoblinClpCancelHandler cancelHandler(cancel_flag, max_iter, &iterHit);
        solver.getModelPtr()->passInEventHandler(&cancelHandler);
    }

    solver.initialSolve();

    out->iterations = solver.getIterationCount();
    out->nodes = 0;
    out->cuts_added = 0;

    bool userCanceled = (cancel_flag && __atomic_load_n(cancel_flag, __ATOMIC_RELAXED) != 0);
    bool iterLimited = iterHit || (max_iter > 0 && out->iterations >= max_iter &&
                                   !solver.isProvenOptimal() &&
                                   !solver.isProvenPrimalInfeasible() &&
                                   !solver.isProvenDualInfeasible());
    if (userCanceled) {
        out->status = -4; // StatusTimeLimit
    } else if (droppedTiny && (solver.isProvenOptimal() || solver.isProvenPrimalInfeasible() || solver.isProvenDualInfeasible())) {
        out->status = -12; // StatusNumericalError
    } else if (solver.isProvenOptimal()) {
        out->status = 1; // StatusOptimal
        out->has_sol = 1;
        out->has_best_bound = 1;
        out->has_root_lp = 1;
        out->has_root_cut = 1;
        double obj = solver.getObjValue();
        out->obj_min = obj;
        out->best_bound_min = obj;
        out->root_lp_min = obj;
        out->root_cut_min = obj;
        if (num_cols > 0 && primal_out && solver.getColSolution()) {
            std::memcpy(primal_out, solver.getColSolution(), sizeof(double) * num_cols);
        }
        if (num_rows > 0 && dual_out && solver.getRowPrice()) {
            std::memcpy(dual_out, solver.getRowPrice(), sizeof(double) * num_rows);
        }
        if (num_cols > 0 && rc_out && solver.getReducedCost()) {
            std::memcpy(rc_out, solver.getReducedCost(), sizeof(double) * num_cols);
        }
    } else if (solver.isProvenPrimalInfeasible()) {
        out->status = -1; // StatusInfeasible
    } else if (solver.isProvenDualInfeasible()) {
        out->status = -2; // StatusUnbounded
    } else if (iterLimited || solver.isIterationLimitReached()) {
        out->status = -8; // StatusIterationLimit
    } else if (solver.getModelPtr() && solver.getModelPtr()->status() == 5) {
        out->status = -4; // StatusTimeLimit
    } else {
        out->status = -12; // StatusNumericalError
    }
    } catch (...) {
        out->status = -12; // StatusNumericalError
    }
}

void goblin_solve_mip(
    int num_cols,
    int num_rows,
    const int *col_ptr,
    const int *row_idx,
    const double *col_val,
    const double *col_lb,
    const double *col_ub,
    const double *obj_min,
    const double *row_lb,
    const double *row_ub,
    int num_ints,
    const int *int_cols,
    int enable_presolve,
    int enable_cuts,
    int enable_heuristics,
    int max_nodes,
    int max_cut_passes,
    int max_iter,
    double rel_gap_tol,
    double abs_gap_tol,
    double time_limit_sec,
    const int *cancel_flag,
    GoblinResult *out,
    double *primal_out
) {
    std::lock_guard<std::mutex> lock(g_cbc_mutex);
    if (!out) return;
    std::memset(out, 0, sizeof(GoblinResult));
    if (cancel_flag && __atomic_load_n(cancel_flag, __ATOMIC_RELAXED) != 0) {
        out->status = -4; // StatusTimeLimit
        return;
    }
    try {
    tl_total_cuts = 0;
    tl_root_lp_min = 0.0;
    tl_root_cut_min = 0.0;
    tl_has_root_lp = false;
    tl_has_root_cut = false;
    tl_cancel_flag = cancel_flag;
    tl_max_iter = max_iter;
    tl_iter_limit_hit = false;

    bool iterHit = false;
    bool cutoffHit = false;
    double maxAbsObj = 0.0;
    double minNonZeroAbsObj = std::numeric_limits<double>::infinity();
    if (num_cols > 0 && obj_min) {
        for (int j = 0; j < num_cols; ++j) {
            double ao = std::fabs(obj_min[j]);
            if (ao > maxAbsObj) {
                maxAbsObj = ao;
            }
            if (ao > 0.0 && ao < minNonZeroAbsObj) {
                minNonZeroAbsObj = ao;
            }
        }
    }

    int objExp = 0;
    std::vector<double> scaledObj;
    const double *mipObjPtr = obj_min;
    double mipAbsGapTol = abs_gap_tol;
    if (maxAbsObj >= 1e35 && obj_min && num_cols > 0) {
        int refExp = 0;
        std::frexp(maxAbsObj, &refExp);
        int wantExp = std::max(0, refExp - 40);
        if (std::isfinite(minNonZeroAbsObj) && minNonZeroAbsObj > 0.0) {
            int minExp = 0;
            std::frexp(minNonZeroAbsObj, &minExp);
            int maxSafeExp = std::max(0, (minExp - 1) - (-1000));
            wantExp = std::min(wantExp, maxSafeExp);
        }
        if (wantExp > 0) {
            objExp = wantExp;
            scaledObj.resize(num_cols);
            for (int j = 0; j < num_cols; ++j) {
                scaledObj[j] = std::ldexp(obj_min[j], -objExp);
            }
            mipObjPtr = scaledObj.data();
            mipAbsGapTol = std::ldexp(abs_gap_tol, -objExp);
        }
    }
    const bool trackCutoff = true;

    OsiClpSolverInterface solver;
    configure_silent_clp(solver);

    static const int dummy_ptr[1] = {0};
    const int *c_ptr = col_ptr ? col_ptr : dummy_ptr;
    bool droppedObj = false;
    double minSigObj = std::numeric_limits<double>::infinity();
    scan_clp_tiny_obj(num_cols, col_lb, col_ub, mipObjPtr, obj_min, droppedObj, minSigObj);
    const bool droppedTiny = droppedObj || has_clp_dropped_tiny_coeff(num_cols, c_ptr, col_val, col_lb, col_ub);
    const bool effPresolve = (enable_presolve > 0) && !std::isfinite(minSigObj) &&
                             !has_clp_presolve_tiny_coeff(num_cols, c_ptr, col_val, col_lb, col_ub);
    const double customDualTol = std::isfinite(minSigObj)
                                     ? std::max(1e-19, std::min(1e-9, 0.01 * minSigObj))
                                     : 0.0;

    solver.loadProblem(
        num_cols,
        num_rows,
        c_ptr,
        row_idx,
        col_val,
        col_lb,
        col_ub,
        mipObjPtr,
        row_lb,
        row_ub
    );
    solver.setObjSense(1.0);
    raise_clp_large_value_if_needed(solver, num_cols, col_lb, col_ub, num_rows, row_lb, row_ub);
    if (customDualTol > 0.0) {
        solver.setDblParam(OsiDualTolerance, customDualTol);
        if (solver.getModelPtr()) {
            solver.getModelPtr()->setDualTolerance(customDualTol);
        }
    }
    if (num_ints > 0 && int_cols) {
        solver.setInteger(int_cols, num_ints);
    }
    if (max_iter > 0) {
        solver.setIntParam(OsiMaxNumIteration, max_iter);
        if (solver.getModelPtr()) {
            solver.getModelPtr()->setMaximumIterations(max_iter);
        }
    }
    if ((cancel_flag || max_iter > 0 || trackCutoff) && solver.getModelPtr()) {
        GoblinClpCancelHandler clpCancelHandler(cancel_flag, max_iter, &iterHit, trackCutoff ? &cutoffHit : nullptr);
        solver.getModelPtr()->passInEventHandler(&clpCancelHandler);
    }

    CbcModel cbcModel(solver);
    cbcModel.setLogLevel(0);
    cbcModel.messageHandler()->setLogLevel(0);

    CbcSolver cbcSolver(cbcModel);
    cbcSolver.initialize();
    cbcSolver.parameters().disableWelcomePrinting();
    cbcSolver.parameters().disableSignalHandler();
    cbcSolver.setPrinting(false);
    if ((cancel_flag || max_iter > 0 || trackCutoff) && cbcSolver.model()) {
        if (cancel_flag || max_iter > 0) {
            GoblinCbcCancelHandler cbcCancelHandler(cancel_flag, max_iter, &iterHit);
            cbcSolver.model()->passInEventHandler(&cbcCancelHandler);
        }
        OsiClpSolverInterface *cbcClp = dynamic_cast<OsiClpSolverInterface *>(cbcSolver.model()->solver());
        if (cbcClp) {
            if (customDualTol > 0.0) {
                cbcClp->setDblParam(OsiDualTolerance, customDualTol);
            }
            if (max_iter > 0) {
                cbcClp->setIntParam(OsiMaxNumIteration, max_iter);
            }
            if (cbcClp->getModelPtr()) {
                raise_clp_large_value_if_needed(*cbcClp, num_cols, col_lb, col_ub, num_rows, row_lb, row_ub);
                if (customDualTol > 0.0) {
                    cbcClp->getModelPtr()->setDualTolerance(customDualTol);
                }
                if (max_iter > 0) {
                    cbcClp->getModelPtr()->setMaximumIterations(max_iter);
                }
                GoblinClpCancelHandler clpCancelHandler(cancel_flag, max_iter, &iterHit, trackCutoff ? &cutoffHit : nullptr);
                cbcClp->getModelPtr()->passInEventHandler(&clpCancelHandler);
            }
        }
    }

    CbcParameters &params = cbcSolver.parameters();
    params[CbcParam::LOGLEVEL]->setVal(0);
    params[CbcParam::LPLOGLEVEL]->setVal(0);
    params[CbcParam::LPITERFREQ]->setVal(0);
    params[CbcParam::LPTIMEFREQ]->setVal(0.0);

    std::deque<std::string> inputQueue;
    auto addParamD = [&](int code, const char *name, double val) {
        params[code]->setVal(val);
        char buf[64];
        std::snprintf(buf, sizeof(buf), "-%s=%.17g", name, val);
        inputQueue.emplace_back(buf);
    };
    auto addParamI = [&](int code, const char *name, int val) {
        params[code]->setVal(val);
        char buf[64];
        std::snprintf(buf, sizeof(buf), "-%s=%d", name, val);
        inputQueue.emplace_back(buf);
    };
    auto addParamKwd = [&](int code, const char *name, const char *val) {
        params[code]->setVal(std::string(val));
        std::string s = std::string("-") + name + "=" + val;
        inputQueue.emplace_back(s);
    };

    inputQueue.emplace_back("-log=0");
    inputQueue.emplace_back("-lpIterFreq=0");
    inputQueue.emplace_back("-lpTimeFreq=0");

    if (customDualTol > 0.0) {
        params.clpParameters()[ClpParam::DUALTOLERANCE]->setVal(customDualTol);
        char dtBuf[64];
        std::snprintf(dtBuf, sizeof(dtBuf), "-dualT=%.17g", customDualTol);
        inputQueue.emplace_back(dtBuf);
    }

    if (!effPresolve) {
        addParamKwd(CbcParam::PREPROCESS, "preprocess", "off");
        params.clpParameters()[ClpParam::PRESOLVE]->setVal(std::string("off"));
        inputQueue.emplace_back("-presolve=off");
        params[CbcParam::BOUNDPROPLEVEL]->setVal(std::string("off"));
        addParamKwd(CbcParam::USECGRAPH, "cgraph", "off");
        addParamKwd(CbcParam::CLQSTRENGTHENING, "clqstr", "off");
    }

    if (!enable_cuts || max_cut_passes <= 0) {
        addParamKwd(CbcParam::CUTSTRATEGY, "cuts", "off");
        addParamI(CbcParam::CUTPASS, "passC", 0);
        addParamI(CbcParam::CUTPASSINTREE, "passt", 0);
    } else {
        addParamI(CbcParam::CUTPASS, "passC", max_cut_passes);
    }

    if (!enable_heuristics) {
        addParamKwd(CbcParam::HEURISTICSTRATEGY, "heuristics", "off");
    }

    addParamD(CbcParam::GAPRATIO, "ratio", rel_gap_tol);
    addParamD(CbcParam::ALLOWABLEGAP, "allow", mipAbsGapTol);
    addParamD(CbcParam::INTEGERTOLERANCE, "integerT", 1e-6);

    if (max_nodes > 0) {
        addParamI(CbcParam::MAXNODES, "maxN", max_nodes);
    }
    if (time_limit_sec > 0.0) {
        if (cbcSolver.model()) {
            cbcSolver.model()->setUseElapsedTime(true);
        }
        addParamD(CbcParam::TIMELIMIT, "sec", time_limit_sec);
    }

    inputQueue.emplace_back("-solve");
    inputQueue.emplace_back("-quit");

    cbcSolver.run(inputQueue, goblin_cbc_callback);
    tl_cancel_flag = nullptr;
    tl_max_iter = 0;

    if (!cbcSolver.model()) {
        out->status = -12; // StatusNumericalError
        return;
    }
    CbcModel &m = *cbcSolver.model();
    const CbcSolverStatistics &stats = cbcSolver.getStatistics();

    out->nodes = m.getNodeCount();
    out->iterations = m.getIterationCount();
    out->cuts_added = tl_total_cuts;

    if (tl_has_root_lp && tl_root_lp_min >= 1e48) {
        cutoffHit = true;
    }
    if (std::isfinite(stats.continuous) && stats.continuous >= 1e48 && stats.continuous < 1e100) {
        cutoffHit = true;
    }

    const bool rootSolved = !m.isProvenInfeasible() && !m.isProvenDualInfeasible() &&
                            !m.isInitialSolveAbandoned();
    if (tl_has_root_lp) {
        double v = (objExp > 0) ? std::ldexp(tl_root_lp_min, objExp) : tl_root_lp_min;
        if (std::isfinite(v)) {
            out->root_lp_min = v;
            out->has_root_lp = 1;
        }
    } else if (rootSolved && std::fabs(stats.continuous) < 1e40) {
        double v = (objExp > 0) ? std::ldexp(stats.continuous, objExp) : stats.continuous;
        if (std::isfinite(v)) {
            out->root_lp_min = v;
            out->has_root_lp = 1;
        }
    }

    if (tl_has_root_cut) {
        double v = (objExp > 0) ? std::ldexp(tl_root_cut_min, objExp) : tl_root_cut_min;
        if (std::isfinite(v)) {
            out->root_cut_min = v;
            out->has_root_cut = 1;
        }
    } else if (rootSolved && std::fabs(stats.tighter) < 1e40) {
        double v = (objExp > 0) ? std::ldexp(stats.tighter, objExp) : stats.tighter;
        if (std::isfinite(v)) {
            out->root_cut_min = v;
            out->has_root_cut = 1;
        }
    } else if (out->has_root_lp) {
        out->root_cut_min = out->root_lp_min;
        out->has_root_cut = 1;
    }

    const double *bestSol = m.bestSolution();
    double rawObjVal = m.getMinimizationObjValue();
    bool hasSol = (bestSol != nullptr && std::isfinite(rawObjVal) && rawObjVal < 1e50);

    if (hasSol) {
        const double *solSrc = bestSol;
        if (num_cols > 0 && primal_out) {
            std::memcpy(primal_out, bestSol, sizeof(double) * num_cols);
            for (int k = 0; k < num_ints; ++k) {
                int col = int_cols[k];
                if (col >= 0 && col < num_cols) {
                    double v = primal_out[col];
                    double r = std::floor(v + 0.5);
                    if (std::fabs(v - r) <= 1e-4) {
                        primal_out[col] = r;
                    }
                }
            }
            solSrc = primal_out;
        }
        double recomputedObj = 0.0;
        if (num_cols > 0 && solSrc && obj_min) {
            for (int j = 0; j < num_cols; ++j) {
                recomputedObj += solSrc[j] * obj_min[j];
            }
            if (!std::isfinite(recomputedObj)) {
                recomputedObj = recompute_wide_dot(num_cols, solSrc, obj_min);
            }
        }
        if (!std::isfinite(recomputedObj)) {
            out->status = -12; // StatusNumericalError
            out->has_sol = 0;
            return;
        }
        out->obj_min = recomputedObj;
    }
    out->has_sol = hasSol ? 1 : 0;

    double rawBestBnd = m.getBestPossibleObjValue();
    bool hasBestBnd = (std::isfinite(rawBestBnd) && std::fabs(rawBestBnd) < 1e50);
    if (hasBestBnd) {
        double bnd = (objExp > 0) ? std::ldexp(rawBestBnd, objExp) : rawBestBnd;
        if (std::isfinite(bnd)) {
            out->best_bound_min = bnd;
            out->has_best_bound = 1;
        } else {
            hasBestBnd = false;
        }
    }
    if (!hasBestBnd) {
        if (out->has_root_cut) {
            out->best_bound_min = out->root_cut_min;
            out->has_best_bound = 1;
            hasBestBnd = true;
        } else if (out->has_root_lp) {
            out->best_bound_min = out->root_lp_min;
            out->has_best_bound = 1;
            hasBestBnd = true;
        } else {
            out->best_bound_min = rawBestBnd;
        }
    }

    bool gapClosed = false;
    if (hasSol && hasBestBnd) {
        double gap = std::max(0.0, out->obj_min - out->best_bound_min);
        double tol = std::max(abs_gap_tol, rel_gap_tol * std::max(1.0, std::fabs(out->obj_min)));
        gapClosed = (gap <= tol);
    }

    bool userCanceled = (cancel_flag && __atomic_load_n(cancel_flag, __ATOMIC_RELAXED) != 0);
    bool iterLimited = iterHit || tl_iter_limit_hit || m.secondaryStatus() == 8 ||
                       (max_iter > 0 && out->iterations >= max_iter &&
                        !m.isProvenOptimal() && !m.isProvenInfeasible() && !gapClosed);

    if (droppedTiny) {
        out->has_sol = 0;
        out->has_best_bound = 0;
        out->has_root_lp = 0;
        out->has_root_cut = 0;
        out->obj_min = 0.0;
        out->best_bound_min = 0.0;
        out->root_lp_min = 0.0;
        out->root_cut_min = 0.0;
        if (userCanceled || m.isSecondsLimitReached() || m.secondaryStatus() == 4) {
            out->status = -4; // StatusTimeLimit
        } else if (iterLimited) {
            out->status = -8; // StatusIterationLimit
        } else if (m.isNodeLimitReached() || m.secondaryStatus() == 3) {
            out->status = -6; // StatusNodeLimit
        } else {
            out->status = -12; // StatusNumericalError
        }
    } else if (!userCanceled && !iterLimited &&
        (m.isProvenOptimal() || gapClosed || (m.status() == 0 && hasSol && m.secondaryStatus() == 0))) {
        out->status = 1; // StatusOptimal
        if (hasSol && hasBestBnd) {
            if (out->best_bound_min > out->obj_min) {
                out->best_bound_min = out->obj_min;
            }
            out->has_best_bound = 1;
        } else if (hasSol) {
            out->best_bound_min = out->obj_min;
            out->has_best_bound = 1;
        }
    } else if (!userCanceled && !iterLimited &&
               (m.isProvenInfeasible() || m.isInitialSolveProvenPrimalInfeasible() ||
                (m.status() == 0 && !hasSol && m.secondaryStatus() == 1))) {
        if (!hasSol && trackCutoff && !cutoffHit && !m.isInitialSolveProvenPrimalInfeasible()) {
            solver.initialSolve();
            if (solver.isProvenOptimal() && solver.getObjValue() >= 1e48) {
                cutoffHit = true;
            }
        }
        out->status = (!hasSol && cutoffHit) ? -12 : -1; // StatusNumericalError when >= 1e50 cutoff saturated
    } else if (!userCanceled && !iterLimited &&
               (m.isContinuousUnbounded() || m.isInitialSolveProvenDualInfeasible() || m.secondaryStatus() == 7)) {
        out->status = -2; // StatusUnbounded
    } else if (userCanceled || m.isSecondsLimitReached() || m.secondaryStatus() == 4) {
        out->status = -4; // StatusTimeLimit
    } else if (iterLimited) {
        out->status = -8; // StatusIterationLimit
    } else if (m.isNodeLimitReached() || m.secondaryStatus() == 3) {
        out->status = -6; // StatusNodeLimit
    } else if (m.isAbandoned() || m.isInitialSolveAbandoned() || m.status() == 2) {
        out->status = -12; // StatusNumericalError
    } else {
        out->status = -3; // StatusUndefined
    }
    } catch (...) {
        tl_cancel_flag = nullptr;
        tl_max_iter = 0;
        out->status = -12; // StatusNumericalError
    }
}

GoblinLPSolver* goblin_lp_new(
    int num_cols,
    int num_rows,
    const int *col_ptr,
    const int *row_idx,
    const double *col_val,
    const double *col_lb,
    const double *col_ub,
    const double *obj_min,
    const double *row_lb,
    const double *row_ub
) {
    std::lock_guard<std::mutex> lock(g_cbc_mutex);
    GoblinLPSolver *s = nullptr;
    try {
        s = new GoblinLPSolver();
        s->num_cols = num_cols;
        s->num_rows = num_rows;
        if (num_cols > 0) {
            s->col_lb.assign(num_cols, 0.0);
            s->col_ub.assign(num_cols, 0.0);
            s->obj_min.assign(num_cols, 0.0);
            s->max_dropped_tiny.assign(num_cols, 0.0);
            if (col_lb) {
                std::memcpy(s->col_lb.data(), col_lb, sizeof(double) * num_cols);
            }
            if (col_ub) {
                std::memcpy(s->col_ub.data(), col_ub, sizeof(double) * num_cols);
            }
            if (obj_min) {
                std::memcpy(s->obj_min.data(), obj_min, sizeof(double) * num_cols);
            }
        }

        static const int dummy_ptr[1] = {0};
        const int *c_ptr = col_ptr ? col_ptr : dummy_ptr;
        bool hasPresolveTiny = false;
        if (num_cols > 0 && col_ptr && col_val) {
            for (int j = 0; j < num_cols; ++j) {
                int start = col_ptr[j];
                int end = col_ptr[j + 1];
                for (int k = start; k < end; ++k) {
                    double av = std::fabs(col_val[k]);
                    if (av > 0.0 && av <= 1e-18 && av > s->max_dropped_tiny[j]) {
                        s->max_dropped_tiny[j] = av;
                    }
                    if (av > 0.0 && av <= 1e-9) {
                        hasPresolveTiny = true;
                    }
                }
            }
        }

        s->solver.loadProblem(
            num_cols,
            num_rows,
            c_ptr,
            row_idx,
            col_val,
            col_lb,
            col_ub,
            obj_min,
            row_lb,
            row_ub
        );
        s->solver.setObjSense(1.0);
        raise_clp_large_value_if_needed(s->solver, num_cols, col_lb, col_ub, num_rows, row_lb, row_ub);
        if (hasPresolveTiny) {
            s->disable_initial_presolve();
        }
        s->refresh_tiny_obj_and_check_dropped();
        return s;
    } catch (...) {
        delete s;
        return nullptr;
    }
}

void goblin_lp_free(GoblinLPSolver *s) {
    std::lock_guard<std::mutex> lock(g_cbc_mutex);
    try {
        delete s;
    } catch (...) {
    }
}

void goblin_lp_set_bounds(
    GoblinLPSolver *s,
    int num_cols,
    const double *col_lb,
    const double *col_ub
) {
    std::lock_guard<std::mutex> lock(g_cbc_mutex);
    if (!s || !col_lb || !col_ub) return;
    try {
        for (int j = 0; j < num_cols && j < s->num_cols; ++j) {
            s->col_lb[j] = col_lb[j];
            s->col_ub[j] = col_ub[j];
            s->solver.setColBounds(j, col_lb[j], col_ub[j]);
        }
        raise_clp_large_value_if_needed(s->solver, s->num_cols, s->col_lb.data(), s->col_ub.data(), 0, nullptr, nullptr);
        s->refresh_tiny_obj_and_check_dropped();
    } catch (...) {
    }
}

void goblin_lp_tighten_dual_tol(GoblinLPSolver *s, double dual_tol) {
    std::lock_guard<std::mutex> lock(g_cbc_mutex);
    if (!s) return;
    try {
        s->disable_initial_presolve();
        s->set_dual_tolerance(dual_tol);
    } catch (...) {
    }
}

int goblin_lp_add_cut(
    GoblinLPSolver *s,
    int nnz,
    const int *col_idx,
    const double *val,
    double rhs
) {
    std::lock_guard<std::mutex> lock(g_cbc_mutex);
    if (!s) return 0;
    try {
        bool hasPresolveTiny = false;
        if (nnz > 0 && col_idx && val) {
            for (int k = 0; k < nnz; ++k) {
                int c = col_idx[k];
                if (c < 0 || c >= s->num_cols) {
                    continue;
                }
                double av = std::fabs(val[k]);
                if (av > 0.0 && av <= 1e-18 && av > s->max_dropped_tiny[c]) {
                    s->max_dropped_tiny[c] = av;
                }
                if (av > 0.0 && av <= 1e-9) {
                    hasPresolveTiny = true;
                }
            }
        }
        if (hasPresolveTiny && !s->initialized) {
            s->disable_initial_presolve();
        }
        s->solver.addRow(nnz, col_idx, val, -COIN_DBL_MAX, rhs);
        s->num_rows = s->solver.getNumRows();
        return 1;
    } catch (...) {
        return 0;
    }
}

void goblin_lp_solve(
    GoblinLPSolver *s,
    int out_cols,
    int out_rows,
    GoblinResult *out,
    double *primal_out,
    double *dual_out,
    double *rc_out
) {
    std::lock_guard<std::mutex> lock(g_cbc_mutex);
    if (!out) return;
    std::memset(out, 0, sizeof(GoblinResult));
    if (!s) {
        out->status = -12; // StatusNumericalError
        return;
    }
    try {
        const bool droppedTiny = s->has_dropped_tiny_for_current_bounds();
        if (!s->initialized) {
            s->solver.initialSolve();
            s->initialized = true;
        } else {
            s->solver.resolve();
        }

        out->iterations = s->solver.getIterationCount();
        int nc = s->solver.getNumCols();
        int nr = s->solver.getNumRows();
        if (nc < 0 || nr < 0 || nc > out_cols || nr > out_rows) {
            out->status = -12; // StatusNumericalError
            return;
        }

        if (droppedTiny && (s->solver.isProvenOptimal() || s->solver.isProvenPrimalInfeasible() || s->solver.isProvenDualInfeasible())) {
            out->status = -12; // StatusNumericalError
        } else if (s->solver.isProvenOptimal()) {
            out->status = 1; // StatusOptimal
            out->has_sol = 1;
            out->has_best_bound = 1;
            double obj = s->solver.getObjValue();
            out->obj_min = obj;
            out->best_bound_min = obj;
            out->root_lp_min = obj;
            out->root_cut_min = obj;
            if (nc > 0 && primal_out && s->solver.getColSolution()) {
                std::memcpy(primal_out, s->solver.getColSolution(), sizeof(double) * nc);
            }
            if (nr > 0 && dual_out && s->solver.getRowPrice()) {
                std::memcpy(dual_out, s->solver.getRowPrice(), sizeof(double) * nr);
            }
            if (nc > 0 && rc_out && s->solver.getReducedCost()) {
                std::memcpy(rc_out, s->solver.getReducedCost(), sizeof(double) * nc);
            }
        } else if (s->solver.isProvenPrimalInfeasible()) {
            out->status = -1; // StatusInfeasible
        } else if (s->solver.isProvenDualInfeasible()) {
            out->status = -2; // StatusUnbounded
        } else if (s->solver.isIterationLimitReached()) {
            out->status = -8; // StatusIterationLimit
        } else {
            out->status = -12; // StatusNumericalError
        }
    } catch (...) {
        out->status = -12; // StatusNumericalError
    }
}

} // extern "C"
