#include "cbc_bridge.h"

#include <algorithm>
#include <cmath>
#include <cstdio>
#include <cstring>
#include <deque>
#include <mutex>
#include <string>

#include "CoinFinite.hpp"
#include "ClpSimplex.hpp"
#include "ClpSolve.hpp"
#include "OsiClpSolverInterface.hpp"
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

int goblin_cbc_callback(CbcModel *currentSolver, int whereFrom) {
    if (!currentSolver) {
        return 0;
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

} // namespace

struct GoblinLPSolver {
    OsiClpSolverInterface solver;
    bool initialized;
    int num_cols;
    int num_rows;

    GoblinLPSolver() : initialized(false), num_cols(0), num_rows(0) {
        configure_silent_clp(solver);
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
    double time_limit_sec,
    GoblinResult *out,
    double *primal_out,
    double *dual_out,
    double *rc_out
) {
    std::lock_guard<std::mutex> lock(g_cbc_mutex);
    std::memset(out, 0, sizeof(GoblinResult));

    OsiClpSolverInterface solver;
    configure_silent_clp(solver);

    static const int dummy_ptr[1] = {0};
    const int *c_ptr = col_ptr ? col_ptr : dummy_ptr;

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

    ClpSolve solveOpts;
    solveOpts.setSpecialOption(2, 1);
    if (!enable_presolve) {
        solveOpts.setPresolveType(ClpSolve::presolveOff);
        solver.setHintParam(OsiDoPresolveInInitial, false, OsiHintTry);
    } else {
        solveOpts.setPresolveType(ClpSolve::presolveOn);
        solver.setHintParam(OsiDoPresolveInInitial, true, OsiHintTry);
    }
    solver.setSolveOptions(solveOpts);

    if (time_limit_sec > 0.0 && solver.getModelPtr()) {
        solver.getModelPtr()->setMaximumWallSeconds(time_limit_sec);
    }

    solver.initialSolve();

    out->iterations = solver.getIterationCount();
    out->nodes = 0;
    out->cuts_added = 0;

    if (solver.isProvenOptimal()) {
        out->status = 1; // StatusOptimal
        out->has_sol = 1;
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
    } else if (solver.isIterationLimitReached() ||
               (solver.getModelPtr() && solver.getModelPtr()->hitMaximumIterations())) {
        out->status = -4; // StatusTimeLimit
    } else {
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
    double rel_gap_tol,
    double abs_gap_tol,
    double time_limit_sec,
    GoblinResult *out,
    double *primal_out
) {
    std::lock_guard<std::mutex> lock(g_cbc_mutex);
    std::memset(out, 0, sizeof(GoblinResult));
    tl_total_cuts = 0;
    tl_root_lp_min = 0.0;
    tl_root_cut_min = 0.0;
    tl_has_root_lp = false;
    tl_has_root_cut = false;

    OsiClpSolverInterface solver;
    configure_silent_clp(solver);

    static const int dummy_ptr[1] = {0};
    const int *c_ptr = col_ptr ? col_ptr : dummy_ptr;

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
    if (num_ints > 0 && int_cols) {
        solver.setInteger(int_cols, num_ints);
    }

    CbcModel cbcModel(solver);
    cbcModel.setLogLevel(0);
    cbcModel.messageHandler()->setLogLevel(0);

    CbcSolver cbcSolver(cbcModel);
    cbcSolver.initialize();
    cbcSolver.parameters().disableWelcomePrinting();
    cbcSolver.parameters().disableSignalHandler();
    cbcSolver.setPrinting(false);

    CbcParameters &params = cbcSolver.parameters();
    params[CbcParam::LOGLEVEL]->setVal(0);
    params[CbcParam::LPLOGLEVEL]->setVal(0);
    params[CbcParam::LPITERFREQ]->setVal(0);
    params[CbcParam::LPTIMEFREQ]->setVal(0.0);

    std::deque<std::string> inputQueue;
    auto addParamD = [&](int code, const char *name, double val) {
        params[code]->setVal(val);
        char buf[64];
        std::snprintf(buf, sizeof(buf), "-%s=%.12g", name, val);
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

    if (!enable_presolve) {
        addParamKwd(CbcParam::PREPROCESS, "preprocess", "off");
        params.clpParameters()[ClpParam::PRESOLVE]->setVal(std::string("off"));
        inputQueue.emplace_back("-presolve=off");
        params[CbcParam::BOUNDPROPLEVEL]->setVal(std::string("off"));
        addParamKwd(CbcParam::USECGRAPH, "cgraph", "off");
        addParamKwd(CbcParam::CLQSTRENGTHENING, "clqstr", "off");
    }

    if (!enable_cuts) {
        addParamKwd(CbcParam::CUTSTRATEGY, "cuts", "off");
        addParamI(CbcParam::CUTPASS, "passC", 0);
        addParamI(CbcParam::CUTPASSINTREE, "passt", 0);
    } else if (max_cut_passes > 0) {
        addParamI(CbcParam::CUTPASS, "passC", max_cut_passes);
    }

    if (!enable_heuristics) {
        addParamKwd(CbcParam::HEURISTICSTRATEGY, "heuristics", "off");
    }

    addParamD(CbcParam::GAPRATIO, "ratio", rel_gap_tol);
    addParamD(CbcParam::ALLOWABLEGAP, "allow", abs_gap_tol);
    addParamD(CbcParam::INTEGERTOLERANCE, "integerT", 1e-6);

    if (max_nodes > 0) {
        addParamI(CbcParam::MAXNODES, "maxN", max_nodes);
    }
    if (time_limit_sec > 0.0) {
        cbcSolver.model()->setUseElapsedTime(true);
        addParamD(CbcParam::TIMELIMIT, "sec", time_limit_sec);
    }

    inputQueue.emplace_back("-solve");
    inputQueue.emplace_back("-quit");

    cbcSolver.run(inputQueue, goblin_cbc_callback);

    CbcModel &m = *cbcSolver.model();
    const CbcSolverStatistics &stats = cbcSolver.getStatistics();

    out->nodes = m.getNodeCount();
    out->iterations = m.getIterationCount();
    out->cuts_added = tl_total_cuts;

    if (tl_has_root_lp) {
        out->root_lp_min = tl_root_lp_min;
    } else if (std::fabs(stats.continuous) < 1e40) {
        out->root_lp_min = stats.continuous;
    }

    if (tl_has_root_cut) {
        out->root_cut_min = tl_root_cut_min;
    } else if (std::fabs(stats.tighter) < 1e40) {
        out->root_cut_min = stats.tighter;
    } else {
        out->root_cut_min = out->root_lp_min;
    }

    const double *bestSol = m.bestSolution();
    bool hasSol = (bestSol != nullptr && m.getMinimizationObjValue() < 1e49);
    out->has_sol = hasSol ? 1 : 0;

    if (hasSol) {
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
        }
        double recomputedObj = 0.0;
        for (int j = 0; j < num_cols; ++j) {
            recomputedObj += primal_out[j] * obj_min[j];
        }
        out->obj_min = recomputedObj;
    }

    double bestBnd = m.getBestPossibleObjValue();
    if (std::fabs(bestBnd) < 1e49) {
        out->best_bound_min = bestBnd;
    } else if (hasSol) {
        out->best_bound_min = out->obj_min;
    }

    bool gapClosed = false;
    if (hasSol && std::fabs(out->best_bound_min) < 1e49) {
        double gap = std::fabs(out->obj_min - out->best_bound_min);
        double tol = std::max(abs_gap_tol, rel_gap_tol * std::max(1.0, std::fabs(out->obj_min)));
        gapClosed = (gap <= tol + 1e-6);
    }

    if (m.isProvenOptimal() || gapClosed) {
        out->status = 1; // StatusOptimal
        out->best_bound_min = out->obj_min;
    } else if (m.isProvenInfeasible() || m.isInitialSolveProvenPrimalInfeasible()) {
        out->status = -1; // StatusInfeasible
    } else if (m.isContinuousUnbounded() || m.isInitialSolveProvenDualInfeasible()) {
        out->status = -2; // StatusUnbounded
    } else if (m.isSecondsLimitReached()) {
        out->status = -4; // StatusTimeLimit
    } else if (m.isNodeLimitReached()) {
        out->status = -6; // StatusNodeLimit
    } else if (m.isAbandoned() || m.isInitialSolveAbandoned()) {
        out->status = -12; // StatusNumericalError
    } else if (hasSol) {
        out->status = 1; // StatusOptimal
        out->best_bound_min = out->obj_min;
    } else {
        out->status = -1; // StatusInfeasible
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
    GoblinLPSolver *s = new GoblinLPSolver();
    s->num_cols = num_cols;
    s->num_rows = num_rows;

    static const int dummy_ptr[1] = {0};
    const int *c_ptr = col_ptr ? col_ptr : dummy_ptr;

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
    return s;
}

void goblin_lp_free(GoblinLPSolver *s) {
    delete s;
}

void goblin_lp_set_bounds(
    GoblinLPSolver *s,
    int num_cols,
    const double *col_lb,
    const double *col_ub
) {
    if (!s) return;
    for (int j = 0; j < num_cols && j < s->num_cols; ++j) {
        s->solver.setColBounds(j, col_lb[j], col_ub[j]);
    }
}

void goblin_lp_add_cut(
    GoblinLPSolver *s,
    int nnz,
    const int *col_idx,
    const double *val,
    double rhs
) {
    if (!s) return;
    s->solver.addRow(nnz, col_idx, val, -COIN_DBL_MAX, rhs);
    s->num_rows = s->solver.getNumRows();
}

void goblin_lp_solve(
    GoblinLPSolver *s,
    GoblinResult *out,
    double *primal_out,
    double *dual_out,
    double *rc_out
) {
    std::lock_guard<std::mutex> lock(g_cbc_mutex);
    std::memset(out, 0, sizeof(GoblinResult));
    if (!s) {
        out->status = 7;
        return;
    }

    if (!s->initialized) {
        s->solver.initialSolve();
        s->initialized = true;
    } else {
        s->solver.resolve();
    }

    out->iterations = s->solver.getIterationCount();
    int nc = s->solver.getNumCols();
    int nr = s->solver.getNumRows();

    if (s->solver.isProvenOptimal()) {
        out->status = 1; // StatusOptimal
        out->has_sol = 1;
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
        out->status = -4; // StatusTimeLimit
    } else {
        out->status = -12; // StatusNumericalError
    }
}

} // extern "C"
