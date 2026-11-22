//go:build cgo && cbc

#include "cbc_bridge.h"

#include <algorithm>
#include <cmath>
#include <cstdint>
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
// during branch-and-cut. Serializing C++ solver calls at the bridge
// boundary guarantees memory safety when Go callers invoke goblin concurrently
// from multiple goroutines.
std::mutex g_cbc_mutex;

thread_local int tl_total_cuts = 0;
thread_local double tl_root_lp_min = 0.0;
thread_local double tl_root_cut_min = 0.0;
thread_local bool tl_has_root_lp = false;
thread_local bool tl_root_lp_unscaled = false;
thread_local bool tl_has_root_cut = false;
thread_local bool tl_cutoff_hit = false;
thread_local const double *tl_orig_obj_min = nullptr;
thread_local int tl_num_cols = 0;
thread_local const int *tl_cancel_flag = nullptr;
thread_local int tl_max_iter = 0;
thread_local bool tl_iter_limit_hit = false;

double recompute_wide_dot(int n, const double *a_vec, const double *b_vec);

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
            if (model_->isDualObjectiveLimitReached() || std::fabs(model_->objectiveValue()) >= 1e48) {
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
            double rawRoot = currentSolver->solver()->getObjValue();
            if (std::fabs(rawRoot) >= 1e48) {
                tl_cutoff_hit = true;
            }
            if (tl_orig_obj_min && tl_num_cols > 0 &&
                currentSolver->solver()->getColSolution() &&
                currentSolver->solver()->getNumCols() == tl_num_cols) {
                double wideRoot = recompute_wide_dot(tl_num_cols, currentSolver->solver()->getColSolution(), tl_orig_obj_min);
                if (std::isfinite(wideRoot)) {
                    tl_root_lp_min = wideRoot;
                    tl_root_lp_unscaled = true;
                    tl_has_root_lp = true;
                }
            } else if (std::fabs(rawRoot) < 1e48) {
                tl_root_lp_min = rawRoot;
                tl_root_lp_unscaled = false;
                tl_has_root_lp = true;
            }
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
        if (std::fabs(contObj) >= 1e48 && std::fabs(contObj) < 0.5 * COIN_DBL_MAX) {
            tl_cutoff_hit = true;
        }
        if (std::fabs(contObj) < 1e40 && !tl_has_root_lp) {
            tl_root_lp_min = contObj;
            tl_root_lp_unscaled = false;
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

struct TinyCoeffScanResult {
    bool dropped_tiny;
    bool presolve_tiny;
};

TinyCoeffScanResult scan_clp_tiny_coeffs(
    int num_cols,
    int num_rows,
    const int *col_ptr,
    const int *row_idx,
    const double *col_val,
    const double *col_lb,
    const double *col_ub,
    bool need_dropped,
    bool need_presolve
) {
    TinyCoeffScanResult res{false, false};
    if (num_cols <= 0 || num_rows <= 0 || !col_ptr || !col_val || (!need_dropped && !need_presolve)) {
        return res;
    }
    const double max_check = need_presolve ? 1e-9 : 1e-18;
    double droppedStack[128];
    double presolveStack[128];
    std::vector<double> droppedHeap;
    std::vector<double> presolveHeap;
    double *droppedSum = nullptr;
    double *presolveSum = nullptr;

    auto ensure_dropped = [&]() -> double * {
        if (!droppedSum) {
            if (num_rows <= 128) {
                std::memset(droppedStack, 0, sizeof(double) * static_cast<size_t>(num_rows));
                droppedSum = droppedStack;
            } else {
                droppedHeap.assign(static_cast<size_t>(num_rows), 0.0);
                droppedSum = droppedHeap.data();
            }
        }
        return droppedSum;
    };
    auto ensure_presolve = [&]() -> double * {
        if (!presolveSum) {
            if (num_rows <= 128) {
                std::memset(presolveStack, 0, sizeof(double) * static_cast<size_t>(num_rows));
                presolveSum = presolveStack;
            } else {
                presolveHeap.assign(static_cast<size_t>(num_rows), 0.0);
                presolveSum = presolveHeap.data();
            }
        }
        return presolveSum;
    };

    for (int j = 0; j < num_cols; ++j) {
        double lb = col_lb ? col_lb[j] : 0.0;
        double ub = col_ub ? col_ub[j] : 0.0;
        if (lb >= ub && lb == 0.0) {
            continue;
        }
        bool infB = (std::fabs(lb) >= 1e30 || std::fabs(ub) >= 1e30);
        double maxB = std::max(1.0, std::max(std::fabs(lb), std::fabs(ub)));
        int start = col_ptr[j];
        int end = col_ptr[j + 1];
        for (int k = start; k < end; ++k) {
            double av = std::fabs(col_val[k]);
            if (av <= 0.0 || av > max_check) {
                continue;
            }
            int r = row_idx ? row_idx[k] : 0;
            if (need_dropped && !res.dropped_tiny && av <= 1e-18) {
                if (infB) {
                    res.dropped_tiny = true;
                } else if (r >= 0 && r < num_rows) {
                    double *ds = ensure_dropped();
                    ds[r] += av * maxB;
                    if (ds[r] >= 1e-8 || !std::isfinite(ds[r])) {
                        res.dropped_tiny = true;
                    }
                }
            }
            if (need_presolve && !res.presolve_tiny && av <= 1e-9) {
                if (infB) {
                    res.presolve_tiny = true;
                } else if (r >= 0 && r < num_rows) {
                    double *ps = ensure_presolve();
                    ps[r] += av * maxB;
                    if (ps[r] >= 1e-7 || !std::isfinite(ps[r])) {
                        res.presolve_tiny = true;
                    }
                }
            }
            if ((!need_dropped || res.dropped_tiny) && (!need_presolve || res.presolve_tiny)) {
                return res;
            }
        }
    }
    return res;
}

double compute_clp_large_value(
    int num_cols,
    const double *col_lb,
    const double *col_ub,
    int num_rows,
    const double *row_lb,
    const double *row_ub
) {
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
        return std::min(1e30, 100.0 * maxFin);
    }
    return 0.0;
}

void apply_clp_large_value(OsiClpSolverInterface &solver, double wantLarge) {
    if (wantLarge <= 0.0) {
        return;
    }
    ClpSimplex *clp = solver.getModelPtr();
    if (clp && wantLarge > clp->largeValue()) {
        clp->setLargeValue(wantLarge);
    }
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
    apply_clp_large_value(solver, compute_clp_large_value(num_cols, col_lb, col_ub, num_rows, row_lb, row_ub));
}

bool has_unrepresentable_clp_finite_bound(
    int num_cols,
    const double *col_lb,
    const double *col_ub,
    int num_rows,
    const double *row_lb,
    const double *row_ub
) {
    const double infFloor = 0.5 * COIN_DBL_MAX;
    for (int j = 0; j < num_cols; ++j) {
        if (col_lb && col_lb[j] > -infFloor && std::fabs(col_lb[j]) >= 1e29) {
            return true;
        }
        if (col_ub && col_ub[j] < infFloor && std::fabs(col_ub[j]) >= 1e29) {
            return true;
        }
    }
    for (int i = 0; i < num_rows; ++i) {
        if (row_lb && row_lb[i] > -infFloor && std::fabs(row_lb[i]) >= 1e29) {
            return true;
        }
        if (row_ub && row_ub[i] < infFloor && std::fabs(row_ub[i]) >= 1e29) {
            return true;
        }
    }
    return false;
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
    double droppedObjSum = 0.0;
    double tinyObjSum = 0.0;
    double smallestTinyAo = std::numeric_limits<double>::infinity();
    for (int j = 0; j < num_cols; ++j) {
        double lb = col_lb ? col_lb[j] : 0.0;
        double ub = col_ub ? col_ub[j] : 0.0;
        if (lb == 0.0 && ub == 0.0) {
            continue;
        }
        if (lb > ub) {
            continue;
        }
        double ao = std::fabs(obj[j]);
        double orig_ao = orig_obj ? std::fabs(orig_obj[j]) : ao;
        if (orig_ao <= 0.0) {
            continue;
        }
        bool unbounded = (std::fabs(lb) >= 1e29 || std::fabs(ub) >= 1e29 || !std::isfinite(ub - lb));
        double maxB = unbounded ? std::numeric_limits<double>::infinity() : std::max(std::fabs(lb), std::fabs(ub));
        if (ao <= 1e-18) {
            if (unbounded) {
                dropped_obj = true;
            } else {
                droppedObjSum += orig_ao * maxB;
                if (droppedObjSum >= 1e-8 || !std::isfinite(droppedObjSum)) {
                    dropped_obj = true;
                }
            }
        } else if (ao <= 1e-5 && lb < ub) {
            double span = unbounded ? std::numeric_limits<double>::infinity() : (ub - lb);
            if (ao < smallestTinyAo) {
                smallestTinyAo = ao;
            }
            if (unbounded || orig_ao * span >= 1e-7) {
                if (ao < min_sig_obj) {
                    min_sig_obj = ao;
                }
            } else {
                tinyObjSum += orig_ao * span;
                if (tinyObjSum >= 1e-7 || !std::isfinite(tinyObjSum)) {
                    if (smallestTinyAo < min_sig_obj) {
                        min_sig_obj = smallestTinyAo;
                    }
                }
            }
        }
    }
}

template <size_t InlineCap = 64>
class SmallDoubleVec {
public:
    SmallDoubleVec() : data_(inline_buf_), size_(0), cap_(InlineCap) {}
    SmallDoubleVec(const SmallDoubleVec &other) : data_(inline_buf_), size_(0), cap_(InlineCap) {
        assign_from(other.data_, other.size_);
    }
    SmallDoubleVec &operator=(const SmallDoubleVec &other) {
        if (this != &other) {
            assign_from(other.data_, other.size_);
        }
        return *this;
    }
    ~SmallDoubleVec() {
        if (data_ != inline_buf_) {
            delete[] data_;
        }
    }

    bool empty() const { return size_ == 0; }
    size_t size() const { return size_; }
    double &operator[](size_t i) { return data_[i]; }
    double operator[](size_t i) const { return data_[i]; }
    double front() const { return data_[0]; }
    double back() const { return data_[size_ - 1]; }
    const double *begin() const { return data_; }
    const double *end() const { return data_ + size_; }

    void resize(size_t n) {
        if (n > cap_) {
            grow(n);
        }
        size_ = n;
    }

    void push_back(double v) {
        if (size_ == cap_) {
            grow(cap_ * 2);
        }
        data_[size_++] = v;
    }

private:
    void grow(size_t min_cap) {
        size_t new_cap = cap_ * 2;
        if (new_cap < min_cap) {
            new_cap = min_cap;
        }
        double *nb = new double[new_cap];
        if (size_ > 0) {
            std::memcpy(nb, data_, sizeof(double) * size_);
        }
        if (data_ != inline_buf_) {
            delete[] data_;
        }
        data_ = nb;
        cap_ = new_cap;
    }

    void assign_from(const double *src, size_t n) {
        if (n > cap_) {
            grow(n);
        }
        if (n > 0) {
            std::memcpy(data_, src, sizeof(double) * n);
        }
        size_ = n;
    }

    double *data_;
    size_t size_;
    size_t cap_;
    double inline_buf_[InlineCap];
};

using WideExpansion = SmallDoubleVec<64>;

bool grow_expansion(WideExpansion &e, double b) {
    if (b == 0.0) {
        return true;
    }
    if (!std::isfinite(b)) {
        return false;
    }
    double q = b;
    size_t w = 0;
    for (size_t i = 0; i < e.size(); ++i) {
        double en = e[i];
        double ns = q + en;
        if (!std::isfinite(ns)) {
            e.resize(w);
            return false;
        }
        double bb = ns - q;
        double h = (q - (ns - bb)) + (en - bb);
        q = ns;
        if (h != 0.0) {
            e[w++] = h;
        }
    }
    e.resize(w);
    if (q != 0.0) {
        e.push_back(q);
    }
    return true;
}

void compress_expansion(WideExpansion &e) {
    if (e.empty()) {
        return;
    }
    int n = static_cast<int>(e.size());
    double q = e[n - 1];
    int bottom = n - 1;
    for (int i = n - 2; i >= 0; --i) {
        double newQ = q + e[i];
        double bb = newQ - q;
        double err = (q - (newQ - bb)) + (e[i] - bb);
        if (err != 0.0) {
            e[bottom--] = newQ;
            q = err;
        } else {
            q = newQ;
        }
    }
    e[bottom] = q;
    int w = 0;
    q = e[bottom];
    for (int i = bottom + 1; i < n; ++i) {
        double newQ = e[i] + q;
        double bb = newQ - e[i];
        double err = (e[i] - (newQ - bb)) + (q - bb);
        if (err != 0.0) {
            e[w++] = err;
        }
        q = newQ;
    }
    if (q != 0.0) {
        e[w++] = q;
    }
    e.resize(static_cast<size_t>(w));
}

void expansion_top_rem(const WideExpansion &comp, double &top, double &rem) {
    top = 0.0;
    rem = 0.0;
    if (comp.empty()) {
        return;
    }
    if (comp.size() == 1) {
        top = comp[0];
        return;
    }
    double r = 0.0;
    for (size_t i = 0; i + 1 < comp.size(); ++i) {
        r += comp[i];
    }
    double hi = comp.back();
    top = hi + r;
    double bb = top - hi;
    rem = (hi - (top - bb)) + (r - bb);
}

double round_to_odd_sticky(double top, double rem) {
    if (rem == 0.0) {
        return top;
    }
    std::uint64_t bits = 0;
    std::memcpy(&bits, &top, sizeof(bits));
    if ((bits & 1ULL) != 0ULL) {
        return top;
    }
    if ((top > 0.0) == (rem > 0.0)) {
        return std::nextafter(top, std::copysign(std::numeric_limits<double>::infinity(), top));
    }
    return std::nextafter(top, 0.0);
}

double recompute_wide_dot(int n, const double *a_vec, const double *b_vec) {
    constexpr int kWideHalf = 528;
    constexpr int kWideCut = 960;
    WideExpansion hi, mid, lo;
    for (int j = 0; j < n; ++j) {
        double xj = a_vec[j];
        double cj = b_vec[j];
        if (xj == 0.0 || cj == 0.0) {
            if ((!std::isfinite(xj) && cj == 0.0) || (!std::isfinite(cj) && xj == 0.0)) {
                return std::numeric_limits<double>::quiet_NaN();
            }
            continue;
        }
        if (!std::isfinite(xj) || !std::isfinite(cj)) {
            return std::numeric_limits<double>::quiet_NaN();
        }
        int ex = 0, ec = 0;
        std::frexp(xj, &ex);
        std::frexp(cj, &ec);
        int sumExp = ex + ec;
        if (sumExp > kWideCut) {
            double a = std::ldexp(xj, -kWideHalf);
            double b = std::ldexp(cj, -kWideHalf);
            double p = a * b;
            double pe = std::fma(a, b, -p);
            if (!grow_expansion(hi, pe) || !grow_expansion(hi, p)) {
                return std::numeric_limits<double>::quiet_NaN();
            }
        } else if (sumExp < -kWideCut) {
            double a = std::ldexp(xj, kWideHalf);
            double b = std::ldexp(cj, kWideHalf);
            double p = a * b;
            double pe = std::fma(a, b, -p);
            if (!grow_expansion(lo, pe) || !grow_expansion(lo, p)) {
                return std::numeric_limits<double>::quiet_NaN();
            }
        } else {
            double p = xj * cj;
            double pe = std::fma(xj, cj, -p);
            if (!grow_expansion(mid, pe) || !grow_expansion(mid, p)) {
                return std::numeric_limits<double>::quiet_NaN();
            }
        }
    }
    compress_expansion(mid);
    if (hi.empty() && lo.empty()) {
        double top = 0.0, rem = 0.0;
        expansion_top_rem(mid, top, rem);
        double comb = top + rem;
        return std::isnan(comb) ? top : comb;
    }
    compress_expansion(hi);
    compress_expansion(lo);

    WideExpansion midMerged;
    if (hi.empty()) {
        midMerged = mid;
    } else {
        double hiTop = hi.back();
        int eHi = 0;
        std::frexp(std::fabs(hiTop), &eHi);
        if (eHi + 2 * kWideHalf > 1026) {
            return std::copysign(std::numeric_limits<double>::infinity(), hiTop);
        }
        WideExpansion half;
        for (double h : hi) {
            grow_expansion(half, std::ldexp(h, kWideHalf));
        }
        for (double m : mid) {
            if (std::fabs(m) >= 0x1p-400) {
                grow_expansion(half, std::ldexp(m, -kWideHalf));
            }
        }
        compress_expansion(half);
        if (!half.empty()) {
            int eHalf = 0;
            std::frexp(std::fabs(half.back()), &eHalf);
            if (eHalf + kWideHalf > 1024) {
                WideExpansion sc4;
                for (double m : mid) {
                    if (std::fabs(m) < 0x1p-400 && std::fabs(m) >= 0x1p-1018) {
                        grow_expansion(sc4, std::ldexp(m, -4));
                    }
                }
                for (double c : half) {
                    grow_expansion(sc4, std::ldexp(c, kWideHalf - 4));
                }
                compress_expansion(sc4);
                double t4 = 0.0, r4 = 0.0;
                expansion_top_rem(sc4, t4, r4);
                double res = std::ldexp(t4, 4);
                if (!std::isfinite(res)) {
                    if (std::fabs(t4) == 0x1p1020 && ((t4 > 0.0) != (r4 > 0.0)) && std::fabs(r4) > 0x1p966) {
                        return std::copysign(std::numeric_limits<double>::max(), t4);
                    }
                    return res;
                }
                double comb = res + std::ldexp(r4, 4);
                return std::isnan(comb) ? res : comb;
            }
        }
        for (double m : mid) {
            if (std::fabs(m) < 0x1p-400) {
                grow_expansion(midMerged, m);
            }
        }
        for (double c : half) {
            grow_expansion(midMerged, std::ldexp(c, kWideHalf));
        }
        compress_expansion(midMerged);
    }

    if (lo.empty()) {
        double top = 0.0, rem = 0.0;
        expansion_top_rem(midMerged, top, rem);
        double comb = top + rem;
        return std::isnan(comb) ? top : comb;
    }

    WideExpansion loSmall;
    bool addedLoNorm = false;
    for (double l : lo) {
        if (std::fabs(l) >= 0x1p34) {
            grow_expansion(midMerged, std::ldexp(std::ldexp(l, -kWideHalf), -kWideHalf));
            addedLoNorm = true;
        } else {
            loSmall.push_back(l);
        }
    }
    if (addedLoNorm) {
        compress_expansion(midMerged);
    }
    if (loSmall.empty()) {
        double top = 0.0, rem = 0.0;
        expansion_top_rem(midMerged, top, rem);
        double comb = top + rem;
        return std::isnan(comb) ? top : comb;
    }

    if (midMerged.empty()) {
        double loTop = 0.0, loRem = 0.0;
        expansion_top_rem(loSmall, loTop, loRem);
        if (loTop == 0.0) {
            return 0.0;
        }
        double adj = round_to_odd_sticky(loTop, loRem);
        return std::ldexp(std::ldexp(adj, -kWideHalf), -kWideHalf);
    }

    if (std::fabs(midMerged.front()) < 0x1p-960) {
        WideExpansion up, topKeep;
        for (double m : midMerged) {
            if (std::fabs(m) < 0x1p400) {
                grow_expansion(up, std::ldexp(m, kWideHalf));
            } else {
                topKeep.push_back(m);
            }
        }
        for (double l : loSmall) {
            if (std::fabs(l) >= 0x1p-490) {
                grow_expansion(up, std::ldexp(l, -kWideHalf));
            }
        }
        compress_expansion(up);
        if (topKeep.empty()) {
            double uTop = 0.0, uRem = 0.0;
            expansion_top_rem(up, uTop, uRem);
            if (uTop == 0.0) {
                return 0.0;
            }
            if (std::fabs(uTop) >= 0x1p-493) {
                return std::ldexp(uTop, -kWideHalf) + std::ldexp(uRem, -kWideHalf);
            }
            double adj = round_to_odd_sticky(uTop, uRem);
            return std::ldexp(adj, -kWideHalf);
        }
        WideExpansion rebuilt;
        for (double u : up) {
            if (std::fabs(u) >= 0x1p-494) {
                grow_expansion(rebuilt, std::ldexp(u, -kWideHalf));
            }
        }
        for (double m : topKeep) {
            grow_expansion(rebuilt, m);
        }
        compress_expansion(rebuilt);
        double top = 0.0, rem = 0.0;
        expansion_top_rem(rebuilt, top, rem);
        double comb = top + rem;
        return std::isnan(comb) ? top : comb;
    }

    double loTop = 0.0, loRem = 0.0;
    expansion_top_rem(loSmall, loTop, loRem);
    double loUnscaled = std::ldexp(std::ldexp(round_to_odd_sticky(loTop, loRem), -kWideHalf), -kWideHalf);
    double r = 0.0;
    for (size_t i = 0; i + 1 < midMerged.size(); ++i) {
        r += midMerged[i];
    }
    r = (r == 0.0) ? loUnscaled : (r + loUnscaled);
    double hiVal = midMerged.back();
    double s = hiVal + r;
    double bb = s - hiVal;
    double e = (hiVal - (s - bb)) + (r - bb);
    double comb = s + e;
    return std::isnan(comb) ? s : comb;
}

} // namespace

struct TinyEntry {
    int row;
    int col;
    double av;
};

struct GoblinLPSolver {
    OsiClpSolverInterface solver;
    bool initialized;
    bool presolve_disabled;
    bool has_extreme_col_bound;
    bool has_extreme_row_bound;
    int num_cols;
    int num_rows;
    std::vector<double> col_lb;
    std::vector<double> col_ub;
    std::vector<double> obj_min;
    std::vector<TinyEntry> tiny_entries;

    GoblinLPSolver()
        : initialized(false),
          presolve_disabled(false),
          has_extreme_col_bound(false),
          has_extreme_row_bound(false),
          num_cols(0),
          num_rows(0) {
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
        if (tiny_entries.empty() || num_rows <= 0) {
            return false;
        }
        std::vector<double> rowSum(static_cast<size_t>(num_rows), 0.0);
        for (const auto &te : tiny_entries) {
            int j = te.col;
            if (j < 0 || j >= num_cols) {
                continue;
            }
            if (col_lb[j] >= col_ub[j] && col_lb[j] == 0.0) {
                continue;
            }
            if (std::fabs(col_lb[j]) >= 1e30 || std::fabs(col_ub[j]) >= 1e30) {
                return true;
            }
            double maxB = std::max(1.0, std::max(std::fabs(col_lb[j]), std::fabs(col_ub[j])));
            if (te.row >= 0 && te.row < num_rows) {
                double &rs = rowSum[static_cast<size_t>(te.row)];
                rs += te.av * maxB;
                if (rs >= 1e-8 || !std::isfinite(rs)) {
                    return true;
                }
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
    if (has_unrepresentable_clp_finite_bound(num_cols, col_lb, col_ub, num_rows, row_lb, row_ub)) {
        out->status = -12; // StatusNumericalError
        return;
    }

    bool iterHit = false;
    OsiClpSolverInterface solver;
    configure_silent_clp(solver);

    static const int dummy_ptr[1] = {0};
    const int *c_ptr = col_ptr ? col_ptr : dummy_ptr;
    bool droppedObj = false;
    double minSigObj = std::numeric_limits<double>::infinity();
    scan_clp_tiny_obj(num_cols, col_lb, col_ub, obj_min, obj_min, droppedObj, minSigObj);
    const bool forceTightDual = (enable_presolve < 0);
    const bool wantPresolve = (enable_presolve > 0) && !std::isfinite(minSigObj);
    const TinyCoeffScanResult tinyScan = scan_clp_tiny_coeffs(
        num_cols, num_rows, c_ptr, row_idx, col_val, col_lb, col_ub, !droppedObj, wantPresolve
    );
    const bool droppedTiny = droppedObj || tinyScan.dropped_tiny;
    const bool effPresolve = wantPresolve && !tinyScan.presolve_tiny;

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
        double obj = solver.getObjValue();
        if (num_cols > 0 && solver.getColSolution() && obj_min) {
            obj = recompute_wide_dot(num_cols, solver.getColSolution(), obj_min);
        }
        if (!std::isfinite(obj)) {
            out->status = -12; // StatusNumericalError
        } else {
            out->status = 1; // StatusOptimal
            out->has_sol = 1;
            out->has_best_bound = 1;
            out->has_root_lp = 1;
            out->has_root_cut = 1;
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
    if (has_unrepresentable_clp_finite_bound(num_cols, col_lb, col_ub, num_rows, row_lb, row_ub)) {
        out->status = -12; // StatusNumericalError
        return;
    }
    tl_total_cuts = 0;
    tl_root_lp_min = 0.0;
    tl_root_cut_min = 0.0;
    tl_has_root_lp = false;
    tl_root_lp_unscaled = false;
    tl_has_root_cut = false;
    tl_cutoff_hit = false;
    tl_orig_obj_min = obj_min;
    tl_num_cols = num_cols;
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
        if (std::ldexp(maxAbsObj, -objExp) >= 1e40) {
            tl_orig_obj_min = nullptr;
            tl_num_cols = 0;
            tl_cancel_flag = nullptr;
            tl_max_iter = 0;
            out->status = -12; // StatusNumericalError
            return;
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
    const bool wantPresolve = (enable_presolve > 0) && !std::isfinite(minSigObj);
    const TinyCoeffScanResult tinyScan = scan_clp_tiny_coeffs(
        num_cols, num_rows, c_ptr, row_idx, col_val, col_lb, col_ub, !droppedObj, wantPresolve
    );
    const bool droppedTiny = droppedObj || tinyScan.dropped_tiny;
    const bool effPresolve = wantPresolve && !tinyScan.presolve_tiny;
    const double wantLargeVal = compute_clp_large_value(num_cols, col_lb, col_ub, num_rows, row_lb, row_ub);
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
    apply_clp_large_value(solver, wantLargeVal);
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
                apply_clp_large_value(*cbcClp, wantLargeVal);
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
    tl_orig_obj_min = nullptr;
    tl_num_cols = 0;
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

    if (tl_cutoff_hit || (tl_has_root_lp && !tl_root_lp_unscaled && std::fabs(tl_root_lp_min) >= 1e48)) {
        cutoffHit = true;
    }
    if (std::isfinite(stats.continuous) && std::fabs(stats.continuous) >= 1e48 && std::fabs(stats.continuous) < 0.5 * COIN_DBL_MAX) {
        cutoffHit = true;
    }

    const bool rootSolved = !m.isProvenInfeasible() && !m.isProvenDualInfeasible() &&
                            !m.isInitialSolveAbandoned();
    if (tl_has_root_lp) {
        double v = (!tl_root_lp_unscaled && objExp > 0) ? std::ldexp(tl_root_lp_min, objExp) : tl_root_lp_min;
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
    bool hasSol = (bestSol != nullptr && std::isfinite(rawObjVal) && std::fabs(rawObjVal) < 1e50);

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
            recomputedObj = recompute_wide_dot(num_cols, solSrc, obj_min);
        }
        if (!std::isfinite(recomputedObj)) {
            out->status = -12; // StatusNumericalError
            out->has_sol = 0;
            return;
        }
        out->obj_min = recomputedObj;
    }
    out->has_sol = hasSol ? 1 : 0;

    bool userCanceled = (cancel_flag && __atomic_load_n(cancel_flag, __ATOMIC_RELAXED) != 0);
    bool timeLimited = userCanceled || m.isSecondsLimitReached() || m.secondaryStatus() == 4;
    bool iterLimited = iterHit || tl_iter_limit_hit || m.secondaryStatus() == 8 ||
                       (max_iter > 0 && out->iterations >= max_iter &&
                        !m.isProvenOptimal() && !m.isProvenInfeasible());

    // Guard against stale CbcModel bestPossibleObjective_ values on aborted solves:
    // before branching (nodes == 0), CbcModel.cpp:4047 can copy solver_->getObjValue() from
    // an interrupted LP, and CbcModel.cpp:5979 can overwrite bestPossibleObjective_ with
    // bestObjective_ when a Clp cancel event leaves CbcModel::status_ == 0.
    bool untrustedCbcBestBound =
        ((timeLimited || iterLimited) && out->nodes == 0) ||
        ((userCanceled || iterLimited) && m.status() == 0 && m.secondaryStatus() != 2);

    double rawBestBnd = m.getBestPossibleObjValue();
    bool hasBestBnd = (!untrustedCbcBestBound && std::isfinite(rawBestBnd) && std::fabs(rawBestBnd) < 1e50);
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
        if (untrustedCbcBestBound) {
            out->has_root_cut = 0;
            out->root_cut_min = 0.0;
        }
        if (!untrustedCbcBestBound && out->has_root_cut) {
            out->best_bound_min = out->root_cut_min;
            out->has_best_bound = 1;
            hasBestBnd = true;
        } else if (out->has_root_lp) {
            out->best_bound_min = out->root_lp_min;
            out->has_best_bound = 1;
            hasBestBnd = true;
        } else {
            out->best_bound_min = untrustedCbcBestBound ? 0.0 : rawBestBnd;
        }
    }

    bool rawObjDrifted = false;
    if (hasSol) {
        double cbcObjMin = (objExp > 0) ? std::ldexp(rawObjVal, objExp) : rawObjVal;
        if (!std::isfinite(cbcObjMin) ||
            std::fabs(cbcObjMin - out->obj_min) > std::max(1e-4, 1e-7 * std::max(1.0, std::fabs(out->obj_min)))) {
            rawObjDrifted = true;
        }
    }
    bool gapClosed = false;
    if (hasSol && hasBestBnd) {
        double rawDiff = out->obj_min - out->best_bound_min;
        double tol = std::max(abs_gap_tol, rel_gap_tol * std::max(1.0, std::fabs(out->obj_min)));
        double invTol = rawObjDrifted ? std::min(tol, 1e-7 * std::max(1.0, std::fabs(out->obj_min))) : tol;
        gapClosed = (rawDiff >= -invTol && rawDiff <= tol);
    }

    if (iterLimited && gapClosed && !userCanceled && !iterHit && !tl_iter_limit_hit && m.secondaryStatus() != 8) {
        iterLimited = false;
    }

    if (droppedTiny) {
        out->has_sol = 0;
        out->has_best_bound = 0;
        out->has_root_lp = 0;
        out->has_root_cut = 0;
        out->obj_min = 0.0;
        out->best_bound_min = 0.0;
        out->root_lp_min = 0.0;
        out->root_cut_min = 0.0;
        if (timeLimited) {
            out->status = -4; // StatusTimeLimit
        } else if (iterLimited) {
            out->status = -8; // StatusIterationLimit
        } else if (m.isNodeLimitReached() || m.secondaryStatus() == 3) {
            out->status = -6; // StatusNodeLimit
        } else {
            out->status = -12; // StatusNumericalError
        }
    } else if (!userCanceled && !iterLimited &&
        ((!timeLimited && !rawObjDrifted && m.isProvenOptimal()) || gapClosed ||
         (!timeLimited && !rawObjDrifted && m.status() == 0 && hasSol && m.secondaryStatus() == 0))) {
        out->status = 1; // StatusOptimal
        if (hasSol && !hasBestBnd) {
            out->best_bound_min = out->obj_min;
            out->has_best_bound = 1;
        }
    } else if (!userCanceled && !iterLimited &&
               (m.isProvenInfeasible() || m.isInitialSolveProvenPrimalInfeasible() ||
                (m.status() == 0 && !hasSol && m.secondaryStatus() == 1))) {
        if (!hasSol && trackCutoff && !cutoffHit && (maxAbsObj >= 1e20 || !m.isInitialSolveProvenPrimalInfeasible())) {
            solver.initialSolve();
            if ((solver.isProvenOptimal() && std::fabs(solver.getObjValue()) >= 1e48) ||
                (!solver.isProvenPrimalInfeasible() && !solver.isProvenOptimal() && !solver.isProvenDualInfeasible()) ||
                (maxAbsObj >= 1e20 && solver.isProvenDualInfeasible())) {
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
    } else if (m.isAbandoned() || m.isInitialSolveAbandoned() || m.status() == 2 || rawObjDrifted) {
        out->status = -12; // StatusNumericalError
    } else {
        out->status = -3; // StatusUndefined
    }
    if (!droppedTiny && hasSol && out->has_sol) {
        if (out->has_best_bound && out->best_bound_min > out->obj_min) {
            out->best_bound_min = out->obj_min;
        }
        if (out->has_root_lp && out->root_lp_min > out->obj_min) {
            out->root_lp_min = out->obj_min;
        }
        if (out->has_root_cut && out->root_cut_min > out->obj_min) {
            out->root_cut_min = out->obj_min;
        }
    }
    } catch (...) {
        tl_orig_obj_min = nullptr;
        tl_num_cols = 0;
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
        s->has_extreme_col_bound = has_unrepresentable_clp_finite_bound(num_cols, col_lb, col_ub, 0, nullptr, nullptr);
        s->has_extreme_row_bound = has_unrepresentable_clp_finite_bound(0, nullptr, nullptr, num_rows, row_lb, row_ub);
        if (num_cols > 0) {
            s->col_lb.assign(num_cols, 0.0);
            s->col_ub.assign(num_cols, 0.0);
            s->obj_min.assign(num_cols, 0.0);
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
                    if (av > 0.0 && av <= 1e-18) {
                        int r = row_idx ? row_idx[k] : 0;
                        s->tiny_entries.push_back(TinyEntry{r, j, av});
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
        s->has_extreme_col_bound = has_unrepresentable_clp_finite_bound(s->num_cols, s->col_lb.data(), s->col_ub.data(), 0, nullptr, nullptr);
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
        int cutRow = s->solver.getNumRows();
        std::vector<TinyEntry> cutTiny;
        if (nnz > 0 && col_idx && val) {
            for (int k = 0; k < nnz; ++k) {
                int c = col_idx[k];
                if (c < 0 || c >= s->num_cols) {
                    continue;
                }
                double av = std::fabs(val[k]);
                if (av > 0.0 && av <= 1e-18) {
                    cutTiny.push_back(TinyEntry{cutRow, c, av});
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
        if (rhs < 0.5 * COIN_DBL_MAX && rhs > -0.5 * COIN_DBL_MAX && std::fabs(rhs) >= 1e29) {
            s->has_extreme_row_bound = true;
        }
        for (const auto &te : cutTiny) {
            s->tiny_entries.push_back(te);
        }
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
        if (s->has_extreme_col_bound || s->has_extreme_row_bound) {
            out->status = -12; // StatusNumericalError
            return;
        }
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
            double obj = s->solver.getObjValue();
            if (nc > 0 && s->solver.getColSolution() && !s->obj_min.empty()) {
                obj = recompute_wide_dot(std::min(nc, static_cast<int>(s->obj_min.size())), s->solver.getColSolution(), s->obj_min.data());
            }
            if (!std::isfinite(obj)) {
                out->status = -12; // StatusNumericalError
            } else {
                out->status = 1; // StatusOptimal
                out->has_sol = 1;
                out->has_best_bound = 1;
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
