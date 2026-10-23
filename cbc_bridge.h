#ifndef GOBLIN_CBC_BRIDGE_H
#define GOBLIN_CBC_BRIDGE_H

#ifdef __cplusplus
extern "C" {
#endif

typedef struct {
    int status;
    int has_sol;
    double obj_min;
    double best_bound_min;
    double root_lp_min;
    double root_cut_min;
    int nodes;
    int iterations;
    int cuts_added;
} GoblinResult;

typedef struct GoblinLPSolver GoblinLPSolver;

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
);

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
);

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
);

void goblin_lp_free(GoblinLPSolver *s);

void goblin_lp_set_bounds(
    GoblinLPSolver *s,
    int num_cols,
    const double *col_lb,
    const double *col_ub
);

void goblin_lp_add_cut(
    GoblinLPSolver *s,
    int nnz,
    const int *col_idx,
    const double *val,
    double rhs
);

void goblin_lp_solve(
    GoblinLPSolver *s,
    GoblinResult *out,
    double *primal_out,
    double *dual_out,
    double *rc_out
);

#ifdef __cplusplus
}
#endif

#endif // GOBLIN_CBC_BRIDGE_H
