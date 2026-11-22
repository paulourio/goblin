package goblin_test

import (
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"testing"

	gb "github.com/paulourio/goblin"
)

func formalFuzzEnabled() bool {
	return gb.FormalOracleEnabled()
}

func callFormalOracleJSONL(tb testing.TB, exeName string, payload []byte) (gb.FormalOracleResp, error) {
	tb.Helper()
	return gb.CallFormalOracleJSONL(tb, exeName, payload)
}

func callFormalOracleSubcmdJSONL(tb testing.TB, exeName, subcmd string, payload []byte) (gb.FormalOracleResp, error) {
	tb.Helper()
	return gb.CallFormalOracleSubcmdJSONL(tb, exeName, subcmd, payload)
}

func formalOracleCall(tb testing.TB, exeName string, reqJSON []byte) (ok bool, boundRat [2]string, detailOrErr string) {
	tb.Helper()
	return gb.FormalOracleCall(tb, exeName, reqJSON)
}

func mustFormalLPCheck(tb testing.TB, reqJSON []byte) *big.Rat {
	tb.Helper()
	resp, err := gb.FormalCheckLPJSONL(tb, reqJSON)
	if err != nil {
		tb.Fatalf("lp-check --jsonl transport error: %v", err)
	}
	if !resp.OK {
		tb.Fatalf("lp-check --jsonl rejected certificate: %s", resp.Error)
	}
	if resp.Bound == nil {
		return nil
	}
	b, bErr := resp.Bound.BigRat()
	if bErr != nil {
		tb.Fatalf("lp-check --jsonl invalid bound: %v", bErr)
	}
	return b
}

func mustFormalCutsCheck(tb testing.TB, reqJSON []byte) {
	tb.Helper()
	resp, err := gb.FormalCheckCutsJSONL(tb, reqJSON)
	if err != nil {
		tb.Fatalf("cuts-check --jsonl transport error: %v", err)
	}
	if !resp.OK {
		tb.Fatalf("cuts-check --jsonl rejected cut: %s", resp.Error)
	}
}

func mustFormalRulesCheck(tb testing.TB, reqJSON []byte) jsontext.Value {
	tb.Helper()
	resp, err := gb.FormalCheckRulesJSONL(tb, reqJSON)
	if err != nil {
		tb.Fatalf("rules-check --jsonl transport error: %v", err)
	}
	if !resp.OK {
		tb.Fatalf("rules-check --jsonl rejected presolve rule: %s", resp.Error)
	}
	return resp.Result
}

func mustFormalVIPRCheck(tb testing.TB, reqJSON []byte) *big.Rat {
	tb.Helper()
	resp, err := gb.FormalCheckVIPRJSONL(tb, reqJSON)
	if err != nil {
		tb.Fatalf("vipr-check --jsonl transport error: %v", err)
	}
	if !resp.OK {
		tb.Fatalf("vipr-check --jsonl rejected certificate: %s", resp.Error)
	}
	if resp.Bound == nil {
		return nil
	}
	b, bErr := resp.Bound.BigRat()
	if bErr != nil {
		tb.Fatalf("vipr-check --jsonl invalid bound: %v", bErr)
	}
	return b
}

func TestFormalFuzzBridgeOffline(t *testing.T) {
	t.Parallel()
	want := os.Getenv("GOBLIN_FORMAL") == "1"
	if got := formalFuzzEnabled(); got != want {
		t.Fatalf("formalFuzzEnabled()=%v, want %v", got, want)
	}
	if !formalFuzzEnabled() {
		if _, err := callFormalOracleJSONL(t, "lp-check", []byte(`{}`)); err == nil {
			t.Fatal("expected error calling oracle when GOBLIN_FORMAL != 1")
		}
	}
}

func TestFormalFuzzBridgeConcurrent(t *testing.T) {
	t.Parallel()
	if !formalFuzzEnabled() {
		t.Skip("set GOBLIN_FORMAL=1 to run formal fuzz bridge live checks")
	}

	lpFarkas, err := os.ReadFile(filepath.Join("testdata", "formal", "lp_farkas_sample.json"))
	if err != nil {
		t.Fatalf("read lp_farkas_sample.json: %v", err)
	}
	viprCert, err := os.ReadFile(filepath.Join("testdata", "formal", "vipr_sample.json"))
	if err != nil {
		t.Fatalf("read vipr_sample.json: %v", err)
	}

	lpSafe := []byte(`{
		"mode": "safe-bound",
		"numVars": 2,
		"numRows": 1,
		"objConst": "1/2",
		"obj": ["2/1", "3/1"],
		"lb": ["0/1", "0/1"],
		"ub": ["5/1", "5/1"],
		"rows": [{"sense": "ge", "rhs": "4/1", "terms": [{"col": 0, "coeff": "1/1"}, {"col": 1, "coeff": "1/1"}]}],
		"y": ["2/1"]
	}`)
	cutsReq := []byte(`{"op":"row-legal-mu","sense":"ge","mu":"-3/2","expected":true}`)
	dropReq := []byte(`{"op":"drop-check","numVars":2,"lb":["0/1",null],"ub":[null,"10/1"],"rawCut":{"terms":[{"col":0,"val":"1/1000"},{"col":1,"val":"1/1"}],"rhs":"5/1"},"scale":"1/1","finalCut":{"terms":[{"col":1,"val":"1/1"}],"rhs":"5/1"}}`)
	rulesReq := []byte(`{"op":"divceil","a":-7,"d":3,"expect_q":-2,"expect_floor":-3}`)
	gcdDivReq := []byte(`{"op":"gcd-div-check","coeffs":[6,-9],"sense":"le","rhs":"17/2","div_coeffs":[2,-3],"div_rhs":2}`)
	lpUnbReq := []byte(`{"mode":"unbounded","numVars":1,"numRows":1,"obj":["-1/1"],"lb":["0/1"],"ub":[null],"rows":[{"sense":"ge","rhs":"2/1","terms":[{"col":0,"coeff":"1/1"}]}],"x0":["2/1"],"d":["1/1"]}`)
	badRulesReq := []byte(`{"op":"divceil","a":-7,"d":3,"expect_q":-99}`)

	bnd := mustFormalLPCheck(t, lpSafe)
	if bnd == nil || bnd.Cmp(big.NewRat(17, 2)) != 0 {
		t.Fatalf("mustFormalLPCheck bound=%v, want 17/2", bnd)
	}
	mustFormalLPCheck(t, lpFarkas)
	mustFormalLPCheck(t, lpUnbReq)
	mustFormalCutsCheck(t, cutsReq)
	mustFormalCutsCheck(t, dropReq)
	if res := mustFormalRulesCheck(t, rulesReq); len(res) == 0 {
		t.Fatal("expected non-empty result from mustFormalRulesCheck")
	}
	if res := mustFormalRulesCheck(t, gcdDivReq); len(res) == 0 {
		t.Fatal("expected non-empty result from gcd-div-check")
	}
	if rSub, subErr := callFormalOracleSubcmdJSONL(t, "cuts-check", "drop-check", dropReq); subErr != nil || !rSub.OK {
		t.Fatalf("callFormalOracleSubcmdJSONL drop-check failed: resp=%+v err=%v", rSub, subErr)
	}
	mustFormalVIPRCheck(t, viprCert)

	const workers = 4
	const iters = 30
	errCh := make(chan error, workers*iters)
	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := range iters {
				switch (id + i) % 5 {
				case 0:
					ok, ratStr, msg := formalOracleCall(t, "lp-check", lpSafe)
					if !ok || ratStr != [2]string{"17", "2"} {
						errCh <- fmt.Errorf("lpSafe failed: ok=%v rat=%v msg=%s", ok, ratStr, msg)
					}
				case 1:
					ok, _, msg := formalOracleCall(t, "cuts-check", cutsReq)
					if !ok {
						errCh <- fmt.Errorf("cutsReq failed: %s", msg)
					}
				case 2:
					ok, _, msg := formalOracleCall(t, "rules-check", rulesReq)
					if !ok {
						errCh <- fmt.Errorf("rulesReq failed: %s", msg)
					}
				case 3:
					ok, _, _ := formalOracleCall(t, "rules-check", badRulesReq)
					if ok {
						errCh <- errors.New("badRulesReq unexpectedly succeeded")
					}
				case 4:
					ok, _, msg := formalOracleCall(t, "vipr-check", viprCert)
					if !ok {
						errCh <- fmt.Errorf("viprCert failed: %s", msg)
					}
				}
			}
		}(w)
	}
	wg.Wait()
	close(errCh)
	for e := range errCh {
		t.Fatal(e)
	}
}
