package goblin_test

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	gb "github.com/paulourio/goblin"
)

type fatalPanicSentinel struct {
	msg string
}

type fatalCaptureTB struct {
	testing.TB
	failed bool
	msg    string
}

func (f *fatalCaptureTB) Helper() {}

func (f *fatalCaptureTB) Error(args ...any) {
	f.failed = true
	if f.msg == "" {
		f.msg = fmt.Sprint(args...)
	}
}

func (f *fatalCaptureTB) Errorf(format string, args ...any) {
	f.failed = true
	if f.msg == "" {
		f.msg = fmt.Sprintf(format, args...)
	}
}

func (f *fatalCaptureTB) Fail() {
	f.failed = true
}

func (f *fatalCaptureTB) Failed() bool {
	return f.failed
}

func (f *fatalCaptureTB) FailNow() {
	f.failed = true
	panic(fatalPanicSentinel{msg: f.msg})
}

func (f *fatalCaptureTB) Fatal(args ...any) {
	f.failed = true
	f.msg = fmt.Sprint(args...)
	panic(fatalPanicSentinel{msg: f.msg})
}

func (f *fatalCaptureTB) Fatalf(format string, args ...any) {
	f.failed = true
	f.msg = fmt.Sprintf(format, args...)
	panic(fatalPanicSentinel{msg: f.msg})
}

// captureVerifierFatal runs fn inside an isolated goroutine so Fatal and FailNow calls
// terminate only the child goroutine while recording the failure message for negative-control assertions.
func captureVerifierFatal(t *testing.T, fn func(tb testing.TB)) (failed bool, msg string) {
	t.Helper()
	capTB := &fatalCaptureTB{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				if fp, ok := r.(fatalPanicSentinel); ok {
					capTB.failed = true
					if capTB.msg == "" {
						capTB.msg = fp.msg
					}
					return
				}
				panic(r)
			}
		}()
		fn(capTB)
		if capTB.failed {
			runtime.Goexit()
		}
	}()
	<-done
	return capTB.failed, capTB.msg
}

func mustFailVerifier(t *testing.T, name string, wantSubstr string, fn func(tb testing.TB)) {
	t.Helper()
	failed, msg := captureVerifierFatal(t, fn)
	if !failed {
		t.Fatalf("[%s] expected verifier to fail closed, got pass", name)
	}
	if wantSubstr != "" && !strings.Contains(msg, wantSubstr) {
		t.Fatalf("[%s] failure message %q missing substring %q", name, msg, wantSubstr)
	}
}

func TestFormalZeroEscapePositiveCorners(t *testing.T) {
	t.Parallel()
	if !formalFuzzEnabled() {
		t.Skip("set GOBLIN_FORMAL=1 to run zero-escape formal verification checks")
	}

	t.Run("ZeroConstraintOptimalAndUnboundedLP", func(t *testing.T) {
		t.Parallel()
		mOpt := gb.NewModel("zero_row_opt", gb.Minimize)
		x0 := mOpt.AddContVar("x0", 3, 8)
		x1 := mOpt.AddContVar("x1", -4, 2)
		xFreeZero := mOpt.AddContVar("xf0", math.Inf(-1), math.Inf(1))
		mOpt.SetObjective(gb.NewExpr(gb.T(x0, 2), gb.T(x1, -3), gb.T(xFreeZero, 0)).AddConstant(1.5))
		pOpt, err := mOpt.CompileChecked()
		if err != nil {
			t.Fatalf("CompileChecked: %v", err)
		}
		verifyFormalRootLP(t, "zero_row_opt/root", pOpt)
		resPre := verifyFormalPresolvedLP(t, "zero_row_opt/pre", pOpt, true)
		if resPre.Status != gb.StatusOptimal || math.Abs(resPre.Objective-1.5) > 1e-9 {
			t.Fatalf("unexpected zero-row LP result: status=%v obj=%g", resPre.Status, resPre.Objective)
		}

		mUnb := gb.NewModel("zero_row_unb", gb.Maximize)
		xu := mUnb.AddContVar("xu", 0, math.Inf(1))
		mUnb.SetObjective(gb.NewExpr(gb.T(xu, 2.5)).AddConstant(-4.0))
		pUnb, err := mUnb.CompileChecked()
		if err != nil {
			t.Fatalf("CompileChecked: %v", err)
		}
		verifyFormalRootLP(t, "zero_row_unb/root", pUnb)
		resUnbPre := verifyFormalPresolvedLP(t, "zero_row_unb/pre", pUnb, true)
		if resUnbPre.Status != gb.StatusUnbounded {
			t.Fatalf("expected StatusUnbounded on 0-row unbounded LP, got %v", resUnbPre.Status)
		}
	})

	t.Run("NonDyadicFreeVarAndPresolvedInfeasibleLP", func(t *testing.T) {
		t.Parallel()
		mFree := gb.NewModel("free_nondyadic", gb.Minimize)
		xf := mFree.AddContVar("xf", math.Inf(-1), math.Inf(1))
		xp := mFree.AddContVar("xp", 0, math.Inf(1))
		mFree.SetObjective(gb.NewExpr(gb.T(xf, 5), gb.T(xp, 7)).AddConstant(2.0))
		mFree.AddConstraint("eq", gb.Eq(gb.NewExpr(gb.T(xf, 3), gb.T(xp, 1)), 4))
		mFree.AddConstraint("ge", gb.Ge(gb.NewExpr(gb.T(xp, 3)), 2))
		pFree, err := mFree.CompileChecked()
		if err != nil {
			t.Fatalf("CompileChecked: %v", err)
		}
		verifyFormalRootLP(t, "free_nondyadic/root", pFree)
		resFreePre := verifyFormalPresolvedLP(t, "free_nondyadic/pre", pFree, true)
		if resFreePre.Status != gb.StatusOptimal {
			t.Fatalf("expected StatusOptimal on free non-dyadic LP, got %v", resFreePre.Status)
		}

		mInf := gb.NewModel("pre_infeas_lp", gb.Minimize)
		y0 := mInf.AddContVar("y0", 0, 5)
		y1 := mInf.AddContVar("y1", math.Inf(-1), math.Inf(1))
		mInf.SetObjective(gb.NewExpr(gb.T(y0, 1)))
		mInf.AddConstraint("eq1", gb.Eq(gb.NewExpr(gb.T(y1, 3)), 6))
		mInf.AddConstraint("le", gb.Le(gb.NewExpr(gb.T(y0, 2), gb.T(y1, 1)), 1))
		pInf, err := mInf.CompileChecked()
		if err != nil {
			t.Fatalf("CompileChecked: %v", err)
		}
		verifyFormalRootLP(t, "pre_infeas_lp/root", pInf)
		resInfPre := verifyFormalPresolvedLP(t, "pre_infeas_lp/pre", pInf, true)
		if resInfPre.Status != gb.StatusInfeasible {
			t.Fatalf("expected StatusInfeasible, got %v", resInfPre.Status)
		}
	})

	t.Run("NonDyadicContinuousMIPAndMeyerUnboundedMIP", func(t *testing.T) {
		t.Parallel()
		mMIP := gb.NewModel("mip_third_cont", gb.Minimize)
		z := mMIP.AddIntVar("z", 1, 5)
		c := mMIP.AddContVar("c", 0, 10)
		mMIP.SetObjective(gb.NewExpr(gb.T(z, 2), gb.T(c, 3)))
		mMIP.AddConstraint("eq_third", gb.Eq(gb.NewExpr(gb.T(c, 3), gb.T(z, -1)), 0))
		pMIP, err := mMIP.CompileChecked()
		if err != nil {
			t.Fatalf("CompileChecked: %v", err)
		}
		resFull, _, _ := runFullFeatureVsVIPRCheck(t, "mip_third_cont", pMIP, 2, 200, time.Second, 0, 0)
		if resFull.Status != gb.StatusOptimal || math.Abs(resFull.Objective-3.0) > 1e-6 {
			t.Fatalf("expected Optimal=3.0, got status=%v obj=%g", resFull.Status, resFull.Objective)
		}

		mUnb := gb.NewModel("mip_meyer_unb", gb.Minimize)
		ix := mUnb.AddIntVar("ix", 1, math.Inf(1))
		iy := mUnb.AddIntVar("iy", 0, math.Inf(1))
		mUnb.SetObjective(gb.NewExpr(gb.T(ix, -3), gb.T(iy, 1)))
		mUnb.AddConstraint("dio", gb.Eq(gb.NewExpr(gb.T(ix, 3), gb.T(iy, -2)), 0))
		pUnb, err := mUnb.CompileChecked()
		if err != nil {
			t.Fatalf("CompileChecked: %v", err)
		}
		resUnb, resVIPRUnb, _ := runFullFeatureVsVIPRCheck(t, "mip_meyer_unb", pUnb, 2, 200, time.Second, 1e-6, 1e-6)
		if resUnb.Status != gb.StatusUnbounded || resVIPRUnb.Status != gb.StatusUnbounded {
			t.Fatalf("expected StatusUnbounded on Meyer MIP, got full=%v vipr=%v", resUnb.Status, resVIPRUnb.Status)
		}
	})
}

func TestFormalZeroEscapeNegativeControls(t *testing.T) {
	t.Parallel()
	if !formalFuzzEnabled() {
		t.Skip("set GOBLIN_FORMAL=1 to run formal negative controls")
	}

	// Case 1: uncertified infeasible LP.
	t.Run("UncertifiedStatusInfeasibleLP", func(t *testing.T) {
		t.Parallel()
		m := gb.NewModel("infeas_lp", gb.Minimize)
		x0 := m.AddContVar("x0", 0, 10)
		x1 := m.AddContVar("x1", 0, 10)
		m.SetObjective(gb.NewExpr(gb.T(x0, 1), gb.T(x1, 1)))
		m.AddConstraint("c_le", gb.Le(gb.NewExpr(gb.T(x0, 1), gb.T(x1, 1)), 3))
		m.AddConstraint("c_ge", gb.Ge(gb.NewExpr(gb.T(x0, 1), gb.T(x1, 1)), 5))
		p := m.Compile()

		mustFailVerifier(t, "uncertified_infeas_nil_lp", "ExportLPCheckPayload failed on StatusInfeasible", func(tb testing.TB) {
			tb.Helper()
			verifyFormalLPResult(tb, "neg_infeas", p, nil, gb.LPResult{
				Status: gb.StatusInfeasible,
				Dual:   []float64{0, 0},
			})
		})

		ws := gb.AcquireWorkspace(p.NumVars(), p.NumConstraints())
		defer gb.ReleaseWorkspace(ws)
		unsolvedLP := gb.NewLPSolver(p, ws)
		mustFailVerifier(t, "uncertified_infeas_unsolved_basis", "ExportLPCheckPayload failed on StatusInfeasible", func(tb testing.TB) {
			tb.Helper()
			verifyFormalLPResult(tb, "neg_infeas_basis", p, unsolvedLP, gb.LPResult{
				Status: gb.StatusInfeasible,
				Dual:   []float64{0, 0},
			})
		})
	})

	// 2. Check false StatusInfeasible on a feasible LP and presolved SolveCompiled run.
	t.Run("FalseStatusInfeasibleLPAndPresolvedLP", func(t *testing.T) {
		t.Parallel()
		m := gb.NewModel("feas_lp", gb.Minimize)
		x0 := m.AddContVar("x0", 0, 10)
		x1 := m.AddContVar("x1", 0, 10)
		m.SetObjective(gb.NewExpr(gb.T(x0, 2), gb.T(x1, 3)))
		m.AddConstraint("dem", gb.Ge(gb.NewExpr(gb.T(x0, 1), gb.T(x1, 1)), 4))
		p := m.Compile()

		ws := gb.AcquireWorkspace(p.NumVars(), p.NumConstraints())
		defer gb.ReleaseWorkspace(ws)
		lp := gb.NewLPSolver(p, ws)
		realRes := lp.Solve()
		if realRes.Status != gb.StatusOptimal {
			t.Fatalf("setup LP status=%v, want Optimal", realRes.Status)
		}

		mustFailVerifier(t, "false_infeas_lp_with_iters", "ExportLPCheckPayload failed on StatusInfeasible", func(tb testing.TB) {
			tb.Helper()
			verifyFormalLPResult(tb, "neg_false_infeas", p, lp, gb.LPResult{
				Status:     gb.StatusInfeasible,
				Iterations: max(1, realRes.Iterations),
			})
		})

		mustFailVerifier(t, "presolved_false_infeas", "presolved StatusInfeasible LP failed Farkas certification", func(tb testing.TB) {
			tb.Helper()
			if err := checkFormalPresolvedLPResult(tb, "neg_pre_infeas", p, gb.Result{
				Status: gb.StatusInfeasible,
			}, true); err != nil {
				tb.Fatal(err)
			}
		})
	})

	// 3. Check uncertified free-variable LP on direct and presolved results.
	t.Run("UncertifiedFreeVarLP", func(t *testing.T) {
		t.Parallel()
		m := gb.NewModel("free_lp", gb.Minimize)
		xf := m.AddContVar("xf", math.Inf(-1), math.Inf(1))
		m.SetObjective(gb.NewExpr(gb.T(xf, 6)))
		m.AddConstraint("eq", gb.Eq(gb.NewExpr(gb.T(xf, 3)), 9))
		p := m.Compile()

		mustFailVerifier(t, "free_lp_wrong_dual", "reduced cost", func(tb testing.TB) {
			tb.Helper()
			verifyFormalLPResult(tb, "neg_free_lp", p, nil, gb.LPResult{
				Status:    gb.StatusOptimal,
				Objective: 18,
				Primal:    []float64{3},
				Dual:      []float64{0},
			})
		})

		mustFailVerifier(t, "free_lp_unexportable_nondyadic_no_basis", "ExportLPCheckPayload failed on StatusOptimal", func(tb testing.TB) {
			tb.Helper()
			mND := gb.NewModel("free_nd", gb.Minimize)
			xfND := mND.AddContVar("xf", math.Inf(-1), math.Inf(1))
			mND.SetObjective(gb.NewExpr(gb.T(xfND, 5)))
			mND.AddConstraint("eq", gb.Eq(gb.NewExpr(gb.T(xfND, 3)), 3))
			pND := mND.Compile()
			verifyFormalLPResult(tb, "neg_free_nd", pND, nil, gb.LPResult{
				Status:    gb.StatusOptimal,
				Objective: 5,
				Primal:    []float64{1},
				Dual:      []float64{5.0 / 3.0},
			})
		})

		mustFailVerifier(t, "presolved_free_lp_missing_duals", "LP dual length=0, want 1", func(tb testing.TB) {
			tb.Helper()
			if err := checkFormalPresolvedLPResult(tb, "neg_pre_free", p, gb.Result{
				Status:    gb.StatusOptimal,
				Objective: 18,
				Values:    []float64{3},
				Duals:     nil,
			}, true); err != nil {
				tb.Fatal(err)
			}
		})
	})

	// Case 4: zero-row LP faults.
	t.Run("ZeroConstraintLPCorruptions", func(t *testing.T) {
		t.Parallel()
		m := gb.NewModel("zero_row", gb.Minimize)
		x := m.AddContVar("x", 2, 10)
		m.SetObjective(gb.NewExpr(gb.T(x, 3)))
		p := m.Compile()

		mustFailVerifier(t, "zero_row_suboptimal", "duality gap", func(tb testing.TB) {
			tb.Helper()
			verifyFormalLPResult(tb, "neg_zero_opt", p, nil, gb.LPResult{
				Status:    gb.StatusOptimal,
				Objective: 15,
				Primal:    []float64{5},
			})
		})

		mustFailVerifier(t, "zero_row_false_infeasible", "0-constraint LP without bound conflict", func(tb testing.TB) {
			tb.Helper()
			verifyFormalLPResult(tb, "neg_zero_inf", p, nil, gb.LPResult{
				Status: gb.StatusInfeasible,
			})
		})
	})

	// Case 5: false unbounded status.
	t.Run("FalseStatusUnboundedLPAndMIP", func(t *testing.T) {
		t.Parallel()
		mLP := gb.NewModel("bounded_lp", gb.Maximize)
		x := mLP.AddContVar("x", 0, 5)
		mLP.SetObjective(gb.NewExpr(gb.T(x, 4)))
		mLP.AddConstraint("cap", gb.Le(gb.NewExpr(gb.T(x, 1)), 5))
		pLP := mLP.Compile()

		mustFailVerifier(t, "false_unb_lp", "ExportLPCheckPayload failed on StatusUnbounded", func(tb testing.TB) {
			tb.Helper()
			verifyFormalLPResult(tb, "neg_unb_lp", pLP, nil, gb.LPResult{
				Status: gb.StatusUnbounded,
				Primal: []float64{5},
			})
		})

		mMIP := gb.NewModel("bounded_mip", gb.Minimize)
		z := mMIP.AddIntVar("z", 0, math.Inf(1))
		mMIP.SetObjective(gb.NewExpr(gb.T(z, 2)))
		mMIP.AddConstraint("ge", gb.Ge(gb.NewExpr(gb.T(z, 1)), 1))
		pMIP := mMIP.Compile()

		mustFailVerifier(t, "false_unb_mip_vipr", "StatusUnbounded failed ExportUnboundedCheckPayload", func(tb testing.TB) {
			tb.Helper()
			if err := checkFormalVIPRResult(tb, "neg_unb_mip", pMIP, gb.Result{
				Status: gb.StatusUnbounded,
				Values: []float64{1},
			}, gb.NewVIPRTrace(), 1e-6, 1e-6); err != nil {
				tb.Fatal(err)
			}
		})
	})

	// 6. Check terminal failure statuses vs true resource limits.
	t.Run("TerminalFailureVsResourceLimitStatuses", func(t *testing.T) {
		t.Parallel()
		m := gb.NewModel("status_gate", gb.Minimize)
		x := m.AddIntVar("x", 0, 5)
		m.SetObjective(gb.NewExpr(gb.T(x, 1)))
		m.AddConstraint("c", gb.Ge(gb.NewExpr(gb.T(x, 1)), 2))
		p := m.Compile()

		for _, bad := range []gb.Status{
			gb.StatusNumericalError,
			gb.StatusUndefined,
			gb.StatusNotSolved,
			gb.StatusGapLimit,
		} {
			mustFailVerifier(t, "lp_bad_"+bad.String(), "uncertified LP terminal status", func(tb testing.TB) {
				tb.Helper()
				verifyFormalLPResult(tb, "neg_st_lp", p, nil, gb.LPResult{Status: bad})
			})
			mustFailVerifier(t, "mip_bad_"+bad.String(), "uncertified MILP terminal status", func(tb testing.TB) {
				tb.Helper()
				verifyMILPInvariants(tb, "neg_st_mip", p, gb.Result{Status: bad, Backend: gb.BackendGo})
			})
		}

		mustFailVerifier(t, "lp_num_err_bounded_iters", "uncertified LP terminal status NumericalError", func(tb testing.TB) {
			tb.Helper()
			verifyFormalLPResult(tb, "neg_st_lp_it", p, nil, gb.LPResult{Status: gb.StatusNumericalError, Iterations: 12})
		})

		mFreeOpt := gb.NewModel("free_opt_gate", gb.Minimize)
		xf := mFreeOpt.AddContVar("xf", math.Inf(-1), math.Inf(1))
		mFreeOpt.SetObjective(gb.NewExpr(gb.T(xf, 2)))
		mFreeOpt.AddConstraint("eq", gb.Eq(gb.NewExpr(gb.T(xf, 1)), 4))
		pFreeOpt := mFreeOpt.Compile()
		mustFailVerifier(t, "lp_num_err_free_opt_iters", "uncertified LP terminal status NumericalError", func(tb testing.TB) {
			tb.Helper()
			verifyFormalLPResult(tb, "neg_st_free_opt", pFreeOpt, nil, gb.LPResult{Status: gb.StatusNumericalError, Iterations: 12})
		})

		// True resource limits pass when uncertified.
		for _, lim := range []gb.Status{gb.StatusTimeLimit, gb.StatusNodeLimit, gb.StatusIterationLimit} {
			verifyFormalLPResult(t, "ok_lim_lp", p, nil, gb.LPResult{Status: lim})
			if err := checkFormalVIPRResult(t, "ok_lim_mip", p, gb.Result{Status: lim}, gb.NewVIPRTrace(), 1e-6, 1e-6); err != nil {
				t.Fatalf("expected resource limit %v to pass uncertified VIPR check, got %v", lim, err)
			}
		}
	})

	// 7. Check missing VIPR certificate and perturbed full-feature incumbents.
	t.Run("UncertifiedVIPRAndFullFeatureViolations", func(t *testing.T) {
		t.Parallel()
		m := gb.NewModel("vipr_gate", gb.Minimize)
		x := m.AddIntVar("x", 0, 10)
		y := m.AddIntVar("y", 0, 10)
		m.SetObjective(gb.NewExpr(gb.T(x, 3), gb.T(y, 4)))
		m.AddConstraint("dem", gb.Ge(gb.NewExpr(gb.T(x, 1), gb.T(y, 1)), 3))
		p := m.Compile()

		mustFailVerifier(t, "missing_vipr_cert_optimal", "missing VIPRTrace certificate", func(tb testing.TB) {
			tb.Helper()
			if err := checkFormalVIPRResult(tb, "neg_no_cert", p, gb.Result{
				Status:    gb.StatusOptimal,
				Objective: 9,
				BestBound: 9,
				Values:    []float64{3, 0},
			}, gb.NewVIPRTrace(), 1e-6, 1e-6); err != nil {
				tb.Fatal(err)
			}
		})

		mustFailVerifier(t, "missing_vipr_cert_infeasible", "missing VIPRTrace certificate", func(tb testing.TB) {
			tb.Helper()
			if err := checkFormalVIPRResult(tb, "neg_no_cert_inf", p, gb.Result{
				Status: gb.StatusInfeasible,
			}, gb.NewVIPRTrace(), 1e-6, 1e-6); err != nil {
				tb.Fatal(err)
			}
		})

		resVIPR, tr := verifyFormalVIPRTrace(t, "real_vipr", p, 200, time.Second,
			gb.WithPresolve(false), gb.WithRelGapTol(0), gb.WithAbsGapTol(0))

		// Inject a 1e-7 step that passes float checks while breaking exact-Q integer bounds.
		mustFailVerifier(t, "full_feature_inexact_nibble", "not exact-Q MIP feasible", func(tb testing.TB) {
			tb.Helper()
			assertFullFeatureVsVIPR(tb, "neg_nibble", p, gb.Result{
				Status:      gb.StatusOptimal,
				Backend:     gb.BackendGo,
				Objective:   3.0 * (3.0 - 1e-7),
				BestBound:   3.0 * (3.0 - 1e-7),
				RootLPBound: 9.0,
				Values:      []float64{3.0 - 1e-7, 0},
			}, resVIPR, tr, 0, 0)
		})

		// Inject an inflated BestBound on NodeLimit above the certified VIPR TargetLB.
		mustFailVerifier(t, "full_feature_inflated_best_bound", "BestBound", func(tb testing.TB) {
			tb.Helper()
			assertFullFeatureVsVIPR(tb, "neg_bnd", p, gb.Result{
				Status:    gb.StatusNodeLimit,
				Backend:   gb.BackendGo,
				BestBound: 12.0,
			}, resVIPR, tr, 0, 0)
		})
	})
}

func TestFormalZeroEscapeSoakLogAudit(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		return
	}
	scriptPath := filepath.Join("scripts", "local_fuzz_soak.sh")
	dir := t.TempDir()

	writeLog := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return p
	}
	runAudit := func(logPath string) (string, error) {
		cmd := exec.CommandContext(t.Context(), "bash", scriptPath, "--formal", "--verify-log="+logPath)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	validLog := writeLog("valid.log", strings.Join([]string{
		"fuzz: elapsed: 0s, gathering baseline coverage: 0/12 completed",
		"fuzz: elapsed: 3s, gathering baseline coverage: 12/12 completed, now fuzzing with 4 workers",
		"fuzz: elapsed: 5s, execs: 214 (42/sec), new interesting: 1 (total: 13)",
		"PASS",
		"ok  \tgithub.com/paulourio/goblin\t5.112s",
		"",
	}, "\n"))
	if out, err := runAudit(validLog); err != nil {
		t.Fatalf("expected valid formal fuzz log to pass, got err=%v out=%s", err, out)
	}

	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{
			name: "silent_skip.log",
			body: "fuzz: elapsed: 5s, execs: 40 (8/sec), new interesting: 0 (total: 10)\n--- SKIP: FuzzLPSmall/seed#0 (0.00s)\nPASS\n",
			want: "silent formal skip",
		},
		{
			name: "missing_elapsed.log",
			body: "PASS\nok\tgithub.com/paulourio/goblin\t0.010s\n",
			want: "missing 'fuzz: elapsed:'",
		},
		{
			name: "zero_execs.log",
			body: "fuzz: elapsed: 0s, gathering baseline coverage: 0/12 completed\nPASS\n",
			want: "zero executed fuzz checks",
		},
		{
			name: "no_fuzz_target.log",
			body: "testing: warning: no fuzz tests to fuzz\nPASS\n",
			want: "no fuzz target executed",
		},
	} {
		lp := writeLog(tc.name, tc.body)
		out, err := runAudit(lp)
		if err == nil {
			t.Fatalf("[%s] expected soak log audit failure, got exit 0 (out=%s)", tc.name, out)
		}
		if !strings.Contains(out, tc.want) {
			t.Fatalf("[%s] output %q missing %q", tc.name, out, tc.want)
		}
	}
}
