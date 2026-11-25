package goblin

import (
	"errors"
	"math/big"
	"testing"
)

func TestVIPRSparseVecMissingOrNullTermsRejected(t *testing.T) {
	t.Parallel()

	validCases := []struct {
		name  string
		n     int
		raw   string
		terms int
	}{
		{
			name:  "zero_dim_empty_terms",
			n:     0,
			raw:   `{"terms":[]}`,
			terms: 0,
		},
		{
			name:  "single_term",
			n:     2,
			raw:   `{"terms":[{"col":1,"coeff":3}]}`,
			terms: 1,
		},
		{
			name:  "cancelling_duplicate_terms",
			n:     2,
			raw:   `{"terms":[{"col":0,"coeff":2},{"col":0,"coeff":-2}]}`,
			terms: 0,
		},
	}
	for _, tc := range validCases {
		_, gotTerms, ok := decodeVIPRVec(tc.n, viprRawJSON(tc.raw))
		if !ok || len(gotTerms) != tc.terms {
			t.Fatalf("%s: decodeVIPRVec(%d, %s) got (ok=%v, len=%d), want (true, %d)",
				tc.name, tc.n, tc.raw, ok, len(gotTerms), tc.terms)
		}
	}

	invalidSparse := []struct {
		name string
		n    int
		raw  string
	}{
		{name: "empty_object", n: 1, raw: `{}`},
		{name: "unrelated_key_only", n: 1, raw: `{"coffs":[]}`},
		{name: "terms_null", n: 1, raw: `{"terms":null}`},
		{name: "terms_int", n: 1, raw: `{"terms":123}`},
		{name: "term_empty_obj", n: 1, raw: `{"terms":[{}]}`},
		{name: "term_missing_coeff", n: 1, raw: `{"terms":[{"col":0}]}`},
		{name: "term_missing_col", n: 1, raw: `{"terms":[{"coeff":1}]}`},
		{name: "term_null_col", n: 1, raw: `{"terms":[{"col":null,"coeff":1}]}`},
		{name: "term_string_col", n: 1, raw: `{"terms":[{"col":"0","coeff":1}]}`},
		{name: "term_null_coeff", n: 1, raw: `{"terms":[{"col":0,"coeff":null}]}`},
		{name: "term_non_object", n: 1, raw: `{"terms":[1]}`},
	}
	for _, tc := range invalidSparse {
		if _, ok := unmarshalCertSparseVecJSON(viprRawJSON(tc.raw)); ok && tc.name != "term_null_coeff" {
			t.Fatalf("%s: unmarshalCertSparseVecJSON(%s) accepted malformed sparse vector", tc.name, tc.raw)
		}
		if _, _, okVec := decodeVIPRVec(tc.n, viprRawJSON(tc.raw)); okVec {
			t.Fatalf("%s: decodeVIPRVec(%d, %s) accepted malformed sparse vector", tc.name, tc.n, tc.raw)
		}
	}
}

func TestVIPRLinConCorruptedCoeffsRepro(t *testing.T) {
	t.Parallel()

	// Positive control: inline sparse constraint without "coeffs" uses "terms".
	inlineSparse := `{"terms":[{"col":0,"coeff":1}],"sense":"<=","rhs":-1}`
	lc, ok := decodeVIPRLinCon(1, viprRawJSON(inlineSparse))
	if !ok || len(lc.terms) != 1 || lc.terms[0].col != 0 || lc.terms[0].val.Cmp(big.NewRat(1, 1)) != 0 {
		t.Fatalf("inline sparse lincon failed: ok=%v lc=%+v", ok, lc)
	}

	// Negative controls: corrupted or missing "coeffs" without "terms" must fail closed.
	badLinCons := []string{
		`{"coffs":[1],"sense":"<=","rhs":-1}`,
		`{"sense":"<=","rhs":-1}`,
		`{"coeffs":null,"sense":"<=","rhs":-1}`,
		`{"coeffs":{},"sense":"<=","rhs":-1}`,
		`{"coeffs":{"terms":null},"sense":"<=","rhs":-1}`,
	}
	for _, raw := range badLinCons {
		if _, okBad := decodeVIPRLinCon(1, viprRawJSON(raw)); okBad {
			t.Fatalf("decodeVIPRLinCon accepted malformed constraint %s", raw)
		}
	}

	// End-to-end certificate repro: mutating "coeffs" to "coffs" previously made Go
	// decode 0*x <= -1 and accept a bogus infeasible proof.
	tamperedCert := `{
		"n": 1,
		"objConst": 0,
		"obj": [1],
		"isInt": [true],
		"lb": [0],
		"ub": [5],
		"baseRows": [{"coffs": [1], "sense": "<=", "rhs": -1}],
		"mode": "infeasible",
		"steps": [{"rule": "conIndex", "row": 0}],
		"rootStep": 0
	}`
	if _, err := verifyMiniVIPRBytes([]byte(tamperedCert), nil); !errors.Is(err, ErrCertificationFailed) {
		t.Fatalf("tampered baseRows[0] got err=%v, want ErrCertificationFailed", err)
	}
}

func TestVIPRJSONParserParityWithLean(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		raw       string
		wantValid bool
	}{
		{
			name: "valid_zero_dim_infeasible",
			raw: `{
				"n": 0, "objConst": 0, "obj": [], "isInt": [], "lb": [], "ub": [],
				"baseRows": [{"coeffs": [], "sense": "<=", "rhs": -1}],
				"mode": "infeasible",
				"steps": [{"rule": "conIndex", "row": 0}],
				"rootStep": 0
			}`,
			wantValid: true,
		},
		{
			name: "valid_zero_dim_optimal_sparse_incumbent",
			raw: `{
				"n": 0, "objConst": 0, "obj": {"terms": []}, "isInt": [], "lb": [], "ub": [],
				"baseRows": [],
				"mode": "optimal",
				"incumbent": {"terms": []},
				"steps": [{"rule": "safeDual", "refs": []}],
				"rootStep": 0
			}`,
			wantValid: true,
		},
		{
			name: "valid_sparse_incumbent_optimal",
			raw: `{
				"n": 1, "objConst": 0, "obj": [1], "isInt": [true], "lb": [2], "ub": [5],
				"baseRows": [],
				"mode": "optimal",
				"incumbent": {"terms": [{"col": 0, "coeff": 2}]},
				"steps": [{"rule": "safeDual", "refs": []}],
				"rootStep": 0
			}`,
			wantValid: true,
		},
		{
			name: "valid_inline_sparse_base_row",
			raw: `{
				"n": 1, "objConst": 0, "obj": [1], "isInt": [true], "lb": [0], "ub": [5],
				"baseRows": [{"terms": [{"col": 0, "coeff": 1}], "sense": ">=", "rhs": 2}],
				"mode": "optimal",
				"incumbent": [2],
				"steps": [{"rule": "safeDual", "refs": [{"id": 0, "mult": 1}]}],
				"rootStep": 0
			}`,
			wantValid: true,
		},
		{
			name: "valid_explicit_min_sense_and_maximize_false",
			raw: `{
				"sense": "  MINIMIZE ", "maximize": false,
				"n": 1, "objConst": 0, "obj": [1], "isInt": [true], "lb": [0], "ub": [5],
				"baseRows": [],
				"mode": "optimal",
				"incumbent": [0],
				"steps": [{"rule": "safeDual", "refs": []}],
				"rootStep": 0
			}`,
			wantValid: true,
		},
		{
			name: "valid_presolve_orig_extra_key",
			raw: `{
				"orig": {"n": 1, "objConst": 0, "obj": [1], "isInt": [true], "lb": [0], "ub": [5], "baseRows": [], "extra": 1},
				"presolve": [],
				"n": 1, "objConst": 0, "obj": [1], "isInt": [true], "lb": [0], "ub": [5],
				"baseRows": [],
				"mode": "optimal",
				"incumbent": [0],
				"steps": [{"rule": "safeDual", "refs": []}],
				"rootStep": 0
			}`,
			wantValid: true,
		},
		{
			name: "valid_leading_zeros_in_small_exponent",
			raw: `{
				"n": 1, "objConst": 0, "obj": [1e00000], "isInt": [true], "lb": [0], "ub": [5],
				"baseRows": [],
				"mode": "optimal",
				"incumbent": [0],
				"steps": [{"rule": "safeDual", "refs": []}],
				"rootStep": 0
			}`,
			wantValid: true,
		},
		{
			name: "reject_corrupted_coeffs_key_in_baseRows",
			raw: `{
				"n": 1, "objConst": 0, "obj": [1], "isInt": [true], "lb": [0], "ub": [5],
				"baseRows": [{"coffs": [1], "sense": "<=", "rhs": -1}],
				"mode": "infeasible",
				"steps": [{"rule": "conIndex", "row": 0}],
				"rootStep": 0
			}`,
			wantValid: false,
		},
		{
			name: "reject_null_terms_in_sparse_obj",
			raw: `{
				"n": 0, "objConst": 0, "obj": {"terms": null}, "isInt": [], "lb": [], "ub": [],
				"baseRows": [{"coeffs": [], "sense": "<=", "rhs": -1}],
				"mode": "infeasible",
				"steps": [{"rule": "conIndex", "row": 0}],
				"rootStep": 0
			}`,
			wantValid: false,
		},
		{
			name: "reject_missing_terms_in_sparse_obj",
			raw: `{
				"n": 0, "objConst": 0, "obj": {}, "isInt": [], "lb": [], "ub": [],
				"baseRows": [{"coeffs": [], "sense": "<=", "rhs": -1}],
				"mode": "infeasible",
				"steps": [{"rule": "conIndex", "row": 0}],
				"rootStep": 0
			}`,
			wantValid: false,
		},
		{
			name: "reject_missing_baseRows",
			raw: `{
				"n": 1, "objConst": 0, "obj": [1], "isInt": [true], "lb": [0], "ub": [5],
				"mode": "optimal",
				"incumbent": [0],
				"steps": [{"rule": "safeDual", "refs": []}],
				"rootStep": 0
			}`,
			wantValid: false,
		},
		{
			name: "reject_null_baseRows",
			raw: `{
				"n": 1, "objConst": 0, "obj": [1], "isInt": [true], "lb": [0], "ub": [5],
				"baseRows": null,
				"mode": "optimal",
				"incumbent": [0],
				"steps": [{"rule": "safeDual", "refs": []}],
				"rootStep": 0
			}`,
			wantValid: false,
		},
		{
			name: "reject_zero_dim_missing_isInt",
			raw: `{
				"n": 0, "objConst": 0, "obj": [], "lb": [], "ub": [],
				"baseRows": [{"coeffs": [], "sense": "<=", "rhs": -1}],
				"mode": "infeasible",
				"steps": [{"rule": "conIndex", "row": 0}],
				"rootStep": 0
			}`,
			wantValid: false,
		},
		{
			name: "reject_zero_dim_null_lb",
			raw: `{
				"n": 0, "objConst": 0, "obj": [], "isInt": [], "lb": null, "ub": [],
				"baseRows": [{"coeffs": [], "sense": "<=", "rhs": -1}],
				"mode": "infeasible",
				"steps": [{"rule": "conIndex", "row": 0}],
				"rootStep": 0
			}`,
			wantValid: false,
		},
		{
			name: "reject_zero_dim_missing_ub",
			raw: `{
				"n": 0, "objConst": 0, "obj": [], "isInt": [], "lb": [],
				"baseRows": [{"coeffs": [], "sense": "<=", "rhs": -1}],
				"mode": "infeasible",
				"steps": [{"rule": "conIndex", "row": 0}],
				"rootStep": 0
			}`,
			wantValid: false,
		},
		{
			name: "reject_zero_dim_missing_incumbent_optimal",
			raw: `{
				"n": 0, "objConst": 0, "obj": [], "isInt": [], "lb": [], "ub": [],
				"baseRows": [],
				"mode": "optimal",
				"steps": [{"rule": "safeDual", "refs": []}],
				"rootStep": 0
			}`,
			wantValid: false,
		},
		{
			name: "reject_zero_dim_null_incumbent_optimal",
			raw: `{
				"n": 0, "objConst": 0, "obj": [], "isInt": [], "lb": [], "ub": [],
				"baseRows": [],
				"mode": "optimal",
				"incumbent": null,
				"steps": [{"rule": "safeDual", "refs": []}],
				"rootStep": 0
			}`,
			wantValid: false,
		},
		{
			name: "reject_maximize_true_on_instance",
			raw: `{
				"maximize": true,
				"n": 1, "objConst": 0, "obj": [1], "isInt": [true], "lb": [0], "ub": [5],
				"baseRows": [],
				"mode": "optimal",
				"incumbent": [0],
				"steps": [{"rule": "safeDual", "refs": []}],
				"rootStep": 0
			}`,
			wantValid: false,
		},
		{
			name: "reject_sense_max_on_orig",
			raw: `{
				"orig": {"sense": "max", "n": 1, "objConst": 0, "obj": [1], "isInt": [true], "lb": [0], "ub": [5], "baseRows": []},
				"presolve": [],
				"n": 1, "objConst": 0, "obj": [1], "isInt": [true], "lb": [0], "ub": [5],
				"baseRows": [],
				"mode": "optimal",
				"incumbent": [0],
				"steps": [{"rule": "safeDual", "refs": []}],
				"rootStep": 0
			}`,
			wantValid: false,
		},
		{
			name: "reject_string_bound_in_assume",
			raw: `{
				"n": 1, "objConst": 0, "obj": [1], "isInt": [true], "lb": [2], "ub": [5],
				"baseRows": [],
				"mode": "infeasible",
				"steps": [
					{"rule": "assume", "col": 0, "sense": "<=", "bound": "1"},
					{"rule": "assume", "col": 0, "sense": ">=", "bound": 2},
					{"rule": "boundConflict", "col": 0, "ubRef": 0},
					{"rule": "boundConflict", "col": 0, "lbRef": 1},
					{"rule": "branch", "col": 0, "splitVal": 1, "leftAsm": 0, "rightAsm": 1, "left": 2, "right": 3}
				],
				"rootStep": 4
			}`,
			wantValid: false,
		},
		{
			name: "reject_string_splitVal_in_branch",
			raw: `{
				"n": 1, "objConst": 0, "obj": [1], "isInt": [true], "lb": [0], "ub": [5],
				"baseRows": [],
				"mode": "optimal",
				"incumbent": [0],
				"steps": [
					{"rule": "assume", "col": 0, "sense": "<=", "bound": 0},
					{"rule": "assume", "col": 0, "sense": ">=", "bound": 1},
					{"rule": "safeDual", "refs": [], "boundAsms": [0]},
					{"rule": "safeDual", "refs": [], "boundAsms": [1]},
					{"rule": "branch", "col": 0, "splitVal": "0", "leftAsm": 0, "rightAsm": 1, "left": 2, "right": 3}
				],
				"rootStep": 4
			}`,
			wantValid: false,
		},
		{
			name: "reject_string_divisor_in_presolve_gcdRowDiv",
			raw: `{
				"orig": {"n": 1, "objConst": 0, "obj": [1], "isInt": [true], "lb": [0], "ub": [5], "baseRows": [{"coeffs": [2], "sense": ">=", "rhs": 2}]},
				"presolve": [{"op": "gcdRowDiv", "row": 0, "divisor": "2"}],
				"n": 1, "objConst": 0, "obj": [1], "isInt": [true], "lb": [0], "ub": [5],
				"baseRows": [{"coeffs": [1], "sense": ">=", "rhs": 1}],
				"mode": "optimal",
				"incumbent": [1],
				"steps": [{"rule": "safeDual", "refs": [{"id": 0, "mult": 1}]}],
				"rootStep": 0
			}`,
			wantValid: false,
		},
		{
			name: "reject_string_col_in_presolve",
			raw: `{
				"orig": {"n": 1, "objConst": 0, "obj": [1], "isInt": [true], "lb": [0], "ub": [5], "baseRows": []},
				"presolve": [{"op": "redundantRow", "row": "0"}],
				"n": 1, "objConst": 0, "obj": [1], "isInt": [true], "lb": [0], "ub": [5],
				"baseRows": [],
				"mode": "optimal",
				"incumbent": [0],
				"steps": [{"rule": "safeDual", "refs": []}],
				"rootStep": 0
			}`,
			wantValid: false,
		},
		{
			name: "reject_huge_zero_exponent_in_rat",
			raw: `{
				"n": 1, "objConst": 0e-50000, "obj": [1], "isInt": [true], "lb": [0], "ub": [5],
				"baseRows": [],
				"mode": "optimal",
				"incumbent": [0],
				"steps": [{"rule": "safeDual", "refs": []}],
				"rootStep": 0
			}`,
			wantValid: false,
		},
		{
			name: "valid_zero_at_exact_exponent_bound",
			raw: `{
				"n": 1, "objConst": 0e10000, "obj": [1], "isInt": [true], "lb": [0], "ub": [5],
				"baseRows": [],
				"mode": "optimal",
				"incumbent": [0],
				"steps": [{"rule": "safeDual", "refs": []}],
				"rootStep": 0
			}`,
			wantValid: true,
		},
		{
			name: "reject_zero_past_exponent_bound",
			raw: `{
				"n": 1, "objConst": 0e10001, "obj": [1], "isInt": [true], "lb": [0], "ub": [5],
				"baseRows": [],
				"mode": "optimal",
				"incumbent": [0],
				"steps": [{"rule": "safeDual", "refs": []}],
				"rootStep": 0
			}`,
			wantValid: false,
		},
		{
			name: "valid_bound_mode_sparse_incumbent",
			raw: `{
				"n": 1, "objConst": 0, "obj": [1], "isInt": [true], "lb": [2], "ub": [5],
				"baseRows": [],
				"mode": "bound",
				"targetLB": 2,
				"incumbent": {"terms": [{"col": 0, "coeff": 2}]},
				"steps": [{"rule": "safeDual", "refs": []}],
				"rootStep": 0
			}`,
			wantValid: true,
		},
		{
			name: "reject_null_steps",
			raw: `{
				"n": 1, "objConst": 0, "obj": [1], "isInt": [true], "lb": [0], "ub": [5],
				"baseRows": [],
				"mode": "optimal",
				"incumbent": [0],
				"steps": null,
				"rootStep": 0
			}`,
			wantValid: false,
		},
		{
			name: "reject_missing_steps",
			raw: `{
				"n": 1, "objConst": 0, "obj": [1], "isInt": [true], "lb": [0], "ub": [5],
				"baseRows": [],
				"mode": "optimal",
				"incumbent": [0],
				"rootStep": 0
			}`,
			wantValid: false,
		},
		{
			name: "reject_bound_mode_bad_incumbent_zero_dim",
			raw: `{
				"n": 0, "objConst": 0, "obj": [], "isInt": [], "lb": [], "ub": [],
				"baseRows": [],
				"mode": "bound",
				"targetLB": 0,
				"incumbent": 123,
				"steps": [{"rule": "safeDual", "refs": []}],
				"rootStep": 0
			}`,
			wantValid: false,
		},
		{
			name: "reject_string_lbRef_in_boundConflict",
			raw: `{
				"n": 1, "objConst": 0, "obj": [1], "isInt": [true], "lb": [2], "ub": [5],
				"baseRows": [],
				"mode": "infeasible",
				"steps": [
					{"rule": "assume", "col": 0, "sense": ">=", "bound": 3},
					{"rule": "assume", "col": 0, "sense": "<=", "bound": 2},
					{"rule": "boundConflict", "col": 0, "lbRef": "0", "ubRef": 1},
					{"rule": "branch", "col": 0, "splitVal": 2, "leftAsm": 1, "rightAsm": 0, "left": 2, "right": 2}
				],
				"rootStep": 3
			}`,
			wantValid: false,
		},
		{
			name: "reject_string_orbitAsms_in_orbitBranch",
			raw: `{
				"n": 2, "objConst": 0, "obj": [1, 1], "isInt": [true, true], "lb": [0, 0], "ub": [1, 1],
				"baseRows": [{"coeffs": [1, 1], "sense": ">=", "rhs": 1}],
				"mode": "optimal",
				"incumbent": [1, 0],
				"steps": [
					{"rule": "assume", "col": 0, "sense": "<=", "bound": 0},
					{"rule": "assume", "col": 1, "sense": "<=", "bound": 0},
					{"rule": "farkas", "refs": [{"id": 0, "mult": -1}, {"id": 1, "mult": 1}, {"id": 2, "mult": 1}]},
					{"rule": "assume", "col": 0, "sense": ">=", "bound": 1},
					{"rule": "safeDual", "refs": [], "boundAsms": [4]},
					{
						"rule": "orbitBranch", "col": 0, "leftAsm": 1, "rightAsm": 4, "left": 3, "right": 5,
						"orbitAsms": ["2"],
						"witnesses": [{"companion": 1, "colPerm": [1, 0], "rowPerm": [0]}]
					}
				],
				"rootStep": 5
			}`,
			wantValid: false,
		},
	}

	checkLean := FormalOracleEnabled()
	for _, tc := range cases {
		_, goErr := verifyMiniVIPRBytes([]byte(tc.raw), nil)
		goOK := goErr == nil
		if goOK != tc.wantValid {
			t.Fatalf("%s: Go verifyMiniVIPRBytes ok=%v (err=%v), want %v", tc.name, goOK, goErr, tc.wantValid)
		}
		if checkLean {
			resp, err := FormalCheckVIPRJSONL(t, []byte(tc.raw))
			if err != nil {
				t.Fatalf("%s: FormalCheckVIPRJSONL transport error: %v", tc.name, err)
			}
			if resp.OK != tc.wantValid {
				t.Fatalf("%s: Lean vipr-check ok=%v (err=%s), want %v", tc.name, resp.OK, resp.Error, tc.wantValid)
			}
		}
	}
}
