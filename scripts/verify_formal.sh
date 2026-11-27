#!/usr/bin/env bash
set -euo pipefail

# End-to-end verification script for the Lean 4 Prove Lane.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(git -C "${SCRIPT_DIR}" rev-parse --show-toplevel)"
cd "${REPO_ROOT}"
unset $(git rev-parse --local-env-vars)

if ! command -v lake >/dev/null 2>&1; then
    if [ -n "${HOME:-}" ] && [ -x "${HOME}/.elan/bin/lake" ]; then
        export PATH="${HOME}/.elan/bin:${PATH}"
    else
        echo "ERROR: lake binary not found in PATH or elan toolchain directory." >&2
        exit 1
    fi
fi

log_step() {
    printf '\n==> %s\n' "$1"
}

check_verified() {
    local exe="$1"
    local fixture="$2"
    local expect_re="${3:-^VERIFIED}"
    local out
    out="$("${exe}" "${fixture}")"
    printf '%s\n' "${out}"
    if ! printf '%s\n' "${out}" | grep -Eq "${expect_re}"; then
        printf 'ERROR: %s %s output %q did not match %s.\n' "${exe}" "${fixture}" "${out}" "${expect_re}" >&2
        exit 1
    fi
}

check_pos_neg() {
    local exe="$1"
    local subcmd="$2"
    local label="$3"
    local pos_json="$4"
    local neg_json="$5"
    local expect_re="$6"
    local -a cmd=("${exe}")
    if [ -n "${subcmd}" ]; then
        cmd+=("${subcmd}")
    fi
    local out jsonl_out

    out="$(printf '%s\n' "${pos_json}" | "${cmd[@]}" -)"
    if ! printf '%s\n' "${out}" | grep -Eq "${expect_re}"; then
        printf 'ERROR: %s %s positive CLI check failed: %s\n' "${exe}" "${label}" "${out}" >&2
        exit 1
    fi

    jsonl_out="$(printf '%s\n' "${pos_json}" | "${cmd[@]}" --jsonl)"
    if ! printf '%s\n' "${jsonl_out}" | grep -q '"ok":true'; then
        printf 'ERROR: %s %s --jsonl positive check failed: %s\n' "${exe}" "${label}" "${jsonl_out}" >&2
        exit 1
    fi

    if printf '%s\n' "${neg_json}" | "${cmd[@]}" - >/dev/null 2>&1; then
        printf 'ERROR: %s %s accepted negative payload.\n' "${exe}" "${label}" >&2
        exit 1
    fi

    jsonl_out="$(printf '%s\n' "${neg_json}" | "${cmd[@]}" --jsonl)"
    if ! printf '%s\n' "${jsonl_out}" | grep -q '"ok":false'; then
        printf 'ERROR: %s %s --jsonl accepted negative payload: %s\n' "${exe}" "${label}" "${jsonl_out}" >&2
        exit 1
    fi

    printf '  [OK] %s %s (CLI + --jsonl positive and negative)\n' "$(basename "${exe}")" "${label}"
}

check_subcmd_pos_neg() {
    local exe="$1"
    local subcmd="$2"
    local pos_json="$3"
    local neg_json="$4"
    local expect_re="$5"
    local label="${6:-${subcmd}}"
    check_pos_neg "${exe}" "${subcmd}" "${label}" "${pos_json}" "${neg_json}" "${expect_re}"
}

check_stdin_pos_neg() {
    local exe="$1"
    local label="$2"
    local pos_json="$3"
    local neg_json="$4"
    local expect_re="$5"
    check_pos_neg "${exe}" "" "${label}" "${pos_json}" "${neg_json}" "${expect_re}"
}

check_neg_cli_and_jsonl() {
    local exe="$1"
    local label="$2"
    local subcmd="$3"
    local neg_json="$4"
    local -a cmd=("${exe}")
    if [ -n "${subcmd}" ]; then
        cmd+=("${subcmd}")
    fi
    local jsonl_out

    if printf '%s\n' "${neg_json}" | "${cmd[@]}" - >/dev/null 2>&1; then
        printf 'ERROR: %s %s accepted negative payload.\n' "${exe}" "${label}" >&2
        exit 1
    fi
    jsonl_out="$(printf '%s\n' "${neg_json}" | "${cmd[@]}" --jsonl)"

    if ! printf '%s\n' "${jsonl_out}" | grep -q '"ok":false'; then
        printf 'ERROR: %s (%s) --jsonl accepted negative payload: %s\n' "${exe}" "${label}" "${jsonl_out}" >&2
        exit 1
    fi

    printf '  [OK] %s %s rejected (CLI + --jsonl)\n' "$(basename "${exe}")" "${label}"
}

check_stdin_only_neg() {
    local exe="$1"
    local label="$2"
    local expect_err_re="$3"
    local neg_json="$4"
    local out

    if out="$(printf '%s\n' "${neg_json}" | "${exe}" - 2>&1)"; then
        printf 'ERROR: %s (%s) accepted negative payload: %s\n' "${exe}" "${label}" "${out}" >&2
        exit 1
    fi
    if ! printf '%s\n' "${out}" | grep -Eq "${expect_err_re}"; then
        printf 'ERROR: %s (%s) rejection message %q missed %s\n' "${exe}" "${label}" "${out}" "${expect_err_re}" >&2
        exit 1
    fi
    printf '  [OK] %s %s rejected\n' "$(basename "${exe}")" "${label}"
}

check_empty_batch_rejected() {
    local exe="$1"
    if printf '[]\n' | "${exe}" --batch - >/dev/null 2>&1; then
        printf 'ERROR: %s accepted empty --batch [].\n' "${exe}" >&2
        exit 1
    fi
    printf '  [OK] %s empty --batch [] rejected\n' "$(basename "${exe}")"
}

export LEAN_NUM_THREADS="${LEAN_NUM_THREADS:-16}"
log_step "1/5 Building Lean 4 proofs, axiom audit, and ELF byte extractors (LEAN_NUM_THREADS=${LEAN_NUM_THREADS})..."
if command -v flock >/dev/null 2>&1; then
    (cd formal && flock /tmp/goblin-formal-lake.lock bash -c 'lake build --wfail && lake lint')
else
    (cd formal && lake build --wfail && lake lint)
fi
if command -v leanchecker >/dev/null 2>&1; then
    (cd formal && lake env leanchecker --fresh GoblinFormal)
fi
(cd formal && lake env lean --stdin <<'EOF'
import GoblinFormal.AxiomAudit
#eval GoblinFormal.AxiomAudit.auditAllAppsModules
EOF
)
(cd formal && lake env lean --stdin <<'EOF'
import GoblinFormal.AxiomAudit
open Lean Elab Command in
#eval show CommandElabM Unit from do
  let env ← getEnv
  let mut counts : Std.HashMap String Nat := {}
  for (declName, cinfo) in env.constants.toList do
    if let .thmInfo _ := cinfo then
      let mn := match env.getModuleIdxFor? declName with
        | some idx => if h : idx.toNat < env.header.moduleNames.size then env.header.moduleNames[idx.toNat].toString else ""
        | none => env.mainModule.toString
      if mn == "GoblinFormal" || mn.startsWith "GoblinFormal." then
        counts := counts.insert mn (counts.getD mn 0 + 1)
  for (m, c) in counts.toList do
    IO.println s!"{m}={c}"
  let appMods : List Name := [
    `Apps.Common, `Apps.BinCheck, `Apps.CutsCheck, `Apps.LpCheck,
    `Apps.LsapCheck, `Apps.RulesCheck, `Apps.SimdCfgCheck, `Apps.ViprCheck
  ]
  for modName in appMods do
    let mEnv ← importModules #[{ module := modName }] {}
    let mut c := 0
    for (declName, cinfo) in mEnv.constants.toList do
      if let .thmInfo _ := cinfo then
        let mn := match mEnv.getModuleIdxFor? declName with
          | some idx => if h : idx.toNat < mEnv.header.moduleNames.size then mEnv.header.moduleNames[idx.toNat].toString else ""
          | none => mEnv.mainModule.toString
        if mn == modName.toString then
          c := c + 1
    IO.println s!"{modName}={c}"
EOF
) | PYTHONDONTWRITEBYTECODE=1 python3 -c '
import pathlib, re, sys
docgen_src = pathlib.Path(sys.argv[1]).read_text(encoding="utf-8")
m = re.search(r"var leanKernelTheoremCounts = map\[string\]int\{(.*?)\n\}", docgen_src, re.S)
if not m:
    raise SystemExit("ERROR: missing leanKernelTheoremCounts in bench/docgen.go")
expected = {k: int(v) for k, v in re.findall(r"\"([^\"]+)\":\s*(\d+)", m.group(1))}
actual = {}
for line in sys.stdin.read().splitlines():
    if "=" in line:
        k, v = line.strip().split("=", 1)
        actual[k] = int(v)
for k, exp in sorted(expected.items()):
    got = actual.get(k, 0)
    if got != exp:
        raise SystemExit(f"ERROR: Lean TheoremInfo drift for {k}: docgen={exp} vs live={got}")
extra = {k: v for k, v in actual.items() if k not in expected and v > 0}
if extra:
    raise SystemExit(f"ERROR: untracked Lean TheoremInfo modules in live env: {extra}")
print(f"Verified Lean TheoremInfo manifest ({sum(expected.values())} declarations across {len(expected)} modules).")
' bench/docgen.go
PYTHONDONTWRITEBYTECODE=1 python3 scripts/extract_elf_track5.py --verify
PYTHONDONTWRITEBYTECODE=1 python3 scripts/extract_amd64_track5e.py --verify

log_step "2/5 Running CLI positive, negative, subcommand, and help smoke checks..."
BIN_DIR="${REPO_ROOT}/formal/.lake/build/bin"
(
    cd formal
    check_verified "${BIN_DIR}/lsap-check" "../testdata/formal/lsap_sample.json" '^VERIFIED mode=exact tol=0(/1)?$'
    if "${BIN_DIR}/lsap-check" "../testdata/formal/lsap_negative.json" >/dev/null 2>&1; then
        echo "ERROR: lsap-check accepted negative fixture ../testdata/formal/lsap_negative.json." >&2
        exit 1
    fi
    echo "lsap-check rejected ../testdata/formal/lsap_negative.json as expected."
    check_verified "${BIN_DIR}/lp-check" "../testdata/formal/lp_optimal_sample.json" '^MODE optimal$'
    check_verified "${BIN_DIR}/lp-check" "../testdata/formal/lp_farkas_sample.json" '^MODE farkas$'
    check_verified "${BIN_DIR}/vipr-check" "../testdata/formal/vipr_sample.json" '^VERIFIED$'
    for cli in rules-check cuts-check bin-check simd-cfg-check; do
        "${BIN_DIR}/${cli}" --help >/dev/null
        "${BIN_DIR}/${cli}" >/dev/null
        check_empty_batch_rejected "${BIN_DIR}/${cli}"
    done
    for cli in lsap-check lp-check vipr-check; do
        check_empty_batch_rejected "${BIN_DIR}/${cli}"
    done

    check_stdin_pos_neg "${BIN_DIR}/lsap-check" "native-null-forbidden" \
        '{"rows":2,"cols":2,"sense":"min","mode":"exact","cost":[[10,null],[null,10]],"row_ind":[0,1],"col_ind":[0,1],"objective":20,"u":[10,10],"v":[0,0]}' \
        '{"rows":2,"cols":2,"sense":"min","mode":"exact","cost":[[10,null],[null,10]],"row_ind":[0,1],"col_ind":[1,0],"objective":0,"u":[0,0],"v":[0,0]}' \
        '^VERIFIED mode=exact tol=0(/1)?$'

    check_stdin_pos_neg "${BIN_DIR}/lsap-check" "hall-infeasible" \
        '{"rows":2,"cols":2,"sense":"min","mode":"hall","cost":[[1,null],[null,null]],"subset":[0,1]}' \
        '{"rows":2,"cols":2,"sense":"min","mode":"hall","cost":[[1,null],[null,2]],"subset":[0,1]}' \
        '^VERIFIED mode=hall$'

    check_neg_cli_and_jsonl "${BIN_DIR}/lsap-check" "duplicate-key" "" \
        '{"rows":1,"cols":1,"sense":"min","sense":"min","mode":"exact","cost":[[1]],"row_ind":[0],"col_ind":[0],"u":[1],"v":[0]}'

    check_neg_cli_and_jsonl "${BIN_DIR}/lsap-check" "cols-1e12-oom-guard" "" \
        '{"rows":0,"cols":1000000000000,"cost":[],"row_ind":[],"col_ind":[],"u":[],"v":[]}'

    check_neg_cli_and_jsonl "${BIN_DIR}/lsap-check" "exact-nonzero-tol" "" \
        '{"rows":1,"cols":1,"sense":"min","mode":"exact","tol":"1/100","cost":[[1]],"row_ind":[0],"col_ind":[0],"u":[1],"v":[0]}'

    check_subcmd_pos_neg "${BIN_DIR}/cuts-check" "cover-check" \
        '{"op":"cover-check","numVars":3,"isInt":[true,true,true],"lb":["0/1","0/1","0/1"],"ub":["1/1","1/1","1/1"],"baseRows":[{"terms":[{"col":0,"val":"3/1"},{"col":1,"val":"3/1"},{"col":2,"val":"2/1"}],"sense":"le","rhs":"4/1"}],"rowIdx":0,"row":{"terms":[{"col":0,"val":"3/1"},{"col":1,"val":"3/1"},{"col":2,"val":"2/1"}],"sense":"le","rhs":"4/1"},"sign":"1/1","items":[{"col":0,"weight":"3/1","comp":false,"inCover":true,"alpha":1},{"col":1,"weight":"3/1","comp":false,"inCover":true,"alpha":1},{"col":2,"weight":"2/1","comp":false,"inCover":false,"alpha":0}],"knapRHS":"4/1","scale":"1/1","cut":{"terms":[{"col":0,"val":"1/1"},{"col":1,"val":"1/1"}],"rhs":"1/1"}}' \
        '{"op":"cover-check","numVars":3,"isInt":[true,true,true],"lb":["0/1","0/1","0/1"],"ub":["1/1","1/1","1/1"],"baseRows":[{"terms":[{"col":0,"val":"3/1"},{"col":1,"val":"3/1"},{"col":2,"val":"2/1"}],"sense":"le","rhs":"4/1"}],"rowIdx":0,"row":{"terms":[{"col":0,"val":"3/1"},{"col":1,"val":"3/1"},{"col":2,"val":"2/1"}],"sense":"le","rhs":"4/1"},"sign":"1/1","items":[{"col":0,"weight":"3/1","comp":false,"inCover":true,"alpha":1},{"col":1,"weight":"3/1","comp":false,"inCover":true,"alpha":1},{"col":2,"weight":"2/1","comp":false,"inCover":false,"alpha":0}],"knapRHS":"4/1","scale":"1/1","cut":{"terms":[{"col":0,"val":"1/1"},{"col":1,"val":"1/1"}],"rhs":"0/1"}}' \
        '^VERIFIED'

    check_neg_cli_and_jsonl "${BIN_DIR}/cuts-check" "cover-continuous-col" "cover-check" \
        '{"op":"cover-check","numVars":3,"isInt":[false,true,true],"lb":["0/1","0/1","0/1"],"ub":["1/1","1/1","1/1"],"baseRows":[{"terms":[{"col":0,"val":"3/1"},{"col":1,"val":"3/1"},{"col":2,"val":"2/1"}],"sense":"le","rhs":"4/1"}],"rowIdx":0,"row":{"terms":[{"col":0,"val":"3/1"},{"col":1,"val":"3/1"},{"col":2,"val":"2/1"}],"sense":"le","rhs":"4/1"},"sign":"1/1","items":[{"col":0,"weight":"3/1","comp":false,"inCover":true,"alpha":1},{"col":1,"weight":"3/1","comp":false,"inCover":true,"alpha":1},{"col":2,"weight":"2/1","comp":false,"inCover":false,"alpha":0}],"knapRHS":"4/1","scale":"1/1","cut":{"terms":[{"col":0,"val":"1/1"},{"col":1,"val":"1/1"}],"rhs":"1/1"}}'

    check_subcmd_pos_neg "${BIN_DIR}/cuts-check" "lotsizing-check" \
        '{"op":"lotsizing-check","numVars":3,"isInt":[false,false,true],"lb":["0/1","0/1","0/1"],"ub":["50/1","50/1","1/1"],"baseRows":[{"terms":[{"col":0,"val":"1/1"},{"col":1,"val":"-1/1"}],"sense":"eq","rhs":"10/1"},{"terms":[{"col":0,"val":"1/1"},{"col":2,"val":"-40/1"}],"sense":"le","rhs":"0/1"}],"invInCol":null,"nodes":[{"prodCol":0,"invOutCol":1,"binCol":2,"flowRowIdx":0,"vubRowIdx":1,"demand":"10/1","vubCap":"40/1","vubRHS":"0/1","inS":true,"coeffY":"10/1"}],"cutDem":"10/1","scale":"1/1","cut":{"terms":[{"col":2,"val":"-10/1"}],"rhs":"-10/1"}}' \
        '{"op":"lotsizing-check","numVars":3,"isInt":[false,false,true],"lb":["0/1","0/1","0/1"],"ub":["50/1","50/1","1/1"],"baseRows":[{"terms":[{"col":0,"val":"1/1"},{"col":1,"val":"-1/1"}],"sense":"eq","rhs":"10/1"},{"terms":[{"col":0,"val":"1/1"},{"col":2,"val":"-40/1"}],"sense":"le","rhs":"0/1"}],"invInCol":null,"nodes":[{"prodCol":0,"invOutCol":1,"binCol":2,"flowRowIdx":0,"vubRowIdx":1,"demand":"10/1","vubCap":"40/1","vubRHS":"0/1","inS":true,"coeffY":"10/1"}],"cutDem":"10/1","scale":"1/1","cut":{"terms":[{"col":2,"val":"-10/1"}],"rhs":"-11/1"}}' \
        '^VERIFIED'

    check_neg_cli_and_jsonl "${BIN_DIR}/cuts-check" "lotsizing-aliased-cols" "lotsizing-check" \
        '{"op":"lotsizing-check","numVars":3,"isInt":[false,false,true],"lb":["0/1","0/1","0/1"],"ub":["50/1","50/1","1/1"],"baseRows":[{"terms":[{"col":0,"val":"1/1"},{"col":1,"val":"-1/1"}],"sense":"eq","rhs":"10/1"},{"terms":[{"col":0,"val":"1/1"},{"col":2,"val":"-40/1"}],"sense":"le","rhs":"0/1"}],"invInCol":0,"nodes":[{"prodCol":0,"invOutCol":1,"binCol":2,"flowRowIdx":0,"vubRowIdx":1,"demand":"10/1","vubCap":"40/1","vubRHS":"0/1","inS":true,"coeffY":"10/1"}],"cutDem":"10/1","scale":"1/1","cut":{"terms":[{"col":0,"val":"1/1"},{"col":2,"val":"-10/1"}],"rhs":"-10/1"}}'

    check_neg_cli_and_jsonl "${BIN_DIR}/cuts-check" "gmi-witness-only" "gmi-check" \
        '{"op":"gmi-check","cuts":[{"terms":[{"col":0,"val":1}],"rhs":0}],"mip":{"numVars":1,"isInt":[true],"lb":[0],"ub":[1],"rows":[]},"solutions":[[0]]}'

    check_neg_cli_and_jsonl "${BIN_DIR}/cuts-check" "duplicate-key" "drop-check" \
        '{"op":"drop-check","op":"drop-check","numVars":1,"lb":["0/1"],"ub":["1/1"],"rawCut":{"terms":[{"col":0,"val":"1/1"}],"rhs":"1/1"},"scale":"1/1","finalCut":{"terms":[{"col":0,"val":"1/1"}],"rhs":"1/1"}}'

    check_subcmd_pos_neg "${BIN_DIR}/cuts-check" "agg-cmir-check" \
        '{"op":"agg-cmir-check","numVars":2,"isInt":[true,false],"lb":["0/1","0/1"],"ub":["5/1",null],"rows":[{"terms":[{"col":0,"val":"2/1"},{"col":1,"val":"1/1"}],"sense":"eq","rhs":"3/1","mu":"1/1"},{"terms":[{"col":1,"val":"-1/1"}],"sense":"le","rhs":"0/1","mu":"1/1"}],"aggTerms":[{"col":0,"val":"2/1"}],"aggRHS":"3/1","delta":"2/1","useLower":[true,true],"betaHat":"3/2","alphaHat":["1/1","0/1"],"cInt":["1/1","0/1"],"cCont":["0/1","0/1"],"scale":"1/1","cut":{"terms":[{"col":0,"val":"1/1"}],"rhs":"1/1"}}' \
        '{"op":"agg-cmir-check","numVars":2,"isInt":[true,false],"lb":["0/1","0/1"],"ub":["5/1",null],"rows":[{"terms":[{"col":0,"val":"2/1"},{"col":1,"val":"1/1"}],"sense":"eq","rhs":"3/1","mu":"1/1"},{"terms":[{"col":1,"val":"-1/1"}],"sense":"le","rhs":"0/1","mu":"1/1"}],"aggTerms":[{"col":0,"val":"2/1"}],"aggRHS":"3/1","delta":"2/1","useLower":[true,true],"betaHat":"3/2","alphaHat":["1/1","0/1"],"cInt":["1/1","0/1"],"cCont":["0/1","0/1"],"scale":"1/1","cut":{"terms":[{"col":0,"val":"1/1"}],"rhs":"0/1"}}' \
        '^VERIFIED'

    check_subcmd_pos_neg "${BIN_DIR}/cuts-check" "drop-check" \
        '{"op":"drop-check","numVars":2,"lb":["0/1",null],"ub":[null,"10/1"],"rawCut":{"terms":[{"col":0,"val":"1/1000"},{"col":1,"val":"1/1"}],"rhs":"5/1"},"scale":"1/1","finalCut":{"terms":[{"col":1,"val":"1/1"}],"rhs":"5/1"}}' \
        '{"op":"drop-check","numVars":2,"lb":["0/1",null],"ub":[null,"10/1"],"rawCut":{"terms":[{"col":0,"val":"1/1000"},{"col":1,"val":"1/1"}],"rhs":"5/1"},"scale":"1/1","finalCut":{"terms":[{"col":1,"val":"1/1"}],"rhs":"4/1"}}' \
        '^VERIFIED'

    check_subcmd_pos_neg "${BIN_DIR}/rules-check" "gcd-div-check" \
        '{"op":"gcd-div-check","coeffs":[6,-9],"sense":"le","rhs":"17/2","div_coeffs":[2,-3],"div_rhs":2}' \
        '{"op":"gcd-div-check","coeffs":[6,-9],"sense":"le","rhs":"17/2","div_coeffs":[2,-3],"div_rhs":1}' \
        '"gcd":3'

    check_subcmd_pos_neg "${BIN_DIR}/rules-check" "bin-strengthen" \
        '{"op":"bin-strengthen","rhs":"12/1","terms":[{"a":"5/1","l":"0/1","u":"1/1","is_int":true},{"a":"10/1","l":"0/1","u":"1/1","is_int":true}],"target_idx":0,"a":"5/1","max_other":"10/1","surplus":"3/1","new_a":"3/1","new_rhs":"10/1"}' \
        '{"op":"bin-strengthen","rhs":"12/1","terms":[{"a":"5/1","l":"0/1","u":"1/1","is_int":true},{"a":"10/1","l":"0/1","u":"1/1","is_int":true}],"target_idx":0,"a":"5/1","max_other":"0/1","surplus":"3/1","new_a":"3/1","new_rhs":"10/1"}' \
        '"proposed_valid":true' "bin-strengthen (spoofed max_other)"

    check_subcmd_pos_neg "${BIN_DIR}/rules-check" "orbit-check" \
        '{"op":"orbit-check","num_vars":2,"is_int":[true,true],"lb":["0/1","0/1"],"ub":["1/1","1/1"],"obj":["1/1","1/1"],"rows":[{"active":true,"cols":[0,1],"vals":["1/1","1/1"],"sense":"le","rhs":"1/1"}],"g_count":2,"orbits":[0,1],"row_tuples":[],"col_orbit":[0,0],"col_group":[0,1],"row_state":[-2]}' \
        '{"op":"orbit-check","num_vars":2,"is_int":[true,true],"lb":["0/1","0/1"],"ub":["1/1","1/1"],"obj":["1/1","1/1"],"rows":[{"active":true,"cols":[0,1],"vals":["1/1","2/1"],"sense":"le","rhs":"1/1"}],"g_count":2,"orbits":[0,1],"row_tuples":[],"col_orbit":[0,0],"col_group":[0,1],"row_state":[-2]}' \
        '"valid":true' "orbit-check (asymmetric row)"

    check_neg_cli_and_jsonl "${BIN_DIR}/rules-check" "duplicate-key" "divceil" \
        '{"op":"divceil","a":7,"a":8,"d":2}'

    check_subcmd_pos_neg "${BIN_DIR}/rules-check" "activity-skip" \
        '{"op":"activity-skip","slack_lo":"3/1","abs_a":"2/1","l":"0/1","u":"1/1","target_bound":"1/1","is_int":true,"go_skip":true}' \
        '{"op":"activity-skip","slack_lo":"3/1","abs_a":"2/1","l":"0/1","u":"3/2","target_bound":"3/2","is_int":true,"go_skip":true}' \
        '"can_skip":true' "activity-skip (fractional int target_bound)"

    check_neg_cli_and_jsonl "${BIN_DIR}/rules-check" "activity-skip-missing-margin" "activity-skip" \
        '{"op":"activity-skip","slack_lo":"2000000000000001/1000000000000000","abs_a":"2/1","l":"0/1","u":"1/1","target_bound":"1/1","is_int":true,"go_skip":true}'

    if printf '%s\n' '[{"op":"gcd-infeas","coeffs":[2,4],"lo":4,"hi":4}]' | "${BIN_DIR}/rules-check" --batch - >/dev/null 2>&1; then
        echo "ERROR: rules-check --batch accepted unasserted gcd-infeas query." >&2
        exit 1
    fi
    printf '  [OK] rules-check --batch unasserted gcd-infeas rejected\n'

    if printf '%s\n' '[{"op":"mir-use-lower","x":"0","l":"0","u":"0","hasL":false,"hasU":false}]' | "${BIN_DIR}/cuts-check" --batch - >/dev/null 2>&1; then
        echo "ERROR: cuts-check --batch accepted unasserted mir-use-lower query." >&2
        exit 1
    fi
    printf '  [OK] cuts-check --batch unasserted mir-use-lower rejected\n'

    check_subcmd_pos_neg "${BIN_DIR}/lp-check" "bound-conflict" \
        '{"mode":"bound-conflict","numVars":2,"numRows":0,"objConst":"0/1","obj":["1/1","2/1"],"lb":["0/1","7/1"],"ub":["5/1","3/1"],"rows":[],"col":1}' \
        '{"mode":"bound-conflict","numVars":2,"numRows":0,"objConst":"0/1","obj":["1/1","2/1"],"lb":["0/1","7/1"],"ub":["5/1","3/1"],"rows":[],"col":0}' \
        '^MODE bound-conflict$'

    check_subcmd_pos_neg "${BIN_DIR}/lp-check" "unbounded" \
        '{"mode":"unbounded","numVars":1,"numRows":1,"obj":["-1/1"],"lb":["0/1"],"ub":[null],"rows":[{"sense":"ge","rhs":"2/1","terms":[{"col":0,"coeff":"1/1"}]}],"x0":["2/1"],"d":["1/1"]}' \
        '{"mode":"unbounded","numVars":1,"numRows":1,"obj":["1/1"],"lb":["0/1"],"ub":[null],"rows":[{"sense":"ge","rhs":"2/1","terms":[{"col":0,"coeff":"1/1"}]}],"x0":["2/1"],"d":["1/1"]}' \
        '^MODE unbounded$'

    check_subcmd_pos_neg "${BIN_DIR}/lp-check" "milp-unbounded" \
        '{"mode":"milp-unbounded","numVars":2,"numRows":1,"intCols":[0],"obj":["-2/1","0/1"],"lb":["2/1","0/1"],"ub":[null,null],"rows":[{"sense":"eq","rhs":"0/1","terms":[{"col":0,"coeff":"-1/1"},{"col":1,"coeff":"3/1"}]}],"x0":["3/1","1/1"],"d":["3/1","1/1"]}' \
        '{"mode":"milp-unbounded","numVars":2,"numRows":1,"intCols":[0,1],"obj":["-2/1","0/1"],"lb":["2/1","0/1"],"ub":[null,null],"rows":[{"sense":"eq","rhs":"0/1","terms":[{"col":0,"coeff":"-1/1"},{"col":1,"coeff":"3/1"}]}],"x0":["3/1","1/1"],"d":["1/1","1/3"]}' \
        '^MODE milp-unbounded$'

    check_subcmd_pos_neg "${BIN_DIR}/lp-check" "lp-presolve" \
        '{"mode":"lp-presolve","orig":{"numVars":2,"numRows":2,"objConst":"4/1","obj":["-2/1","-1/1"],"lb":["0/1","0/1"],"ub":["10/1","10/1"],"rows":[{"sense":"le","rhs":"6/1","terms":[{"col":0,"coeff":"2/1"}]},{"sense":"le","rhs":"8/1","terms":[{"col":0,"coeff":"1/1"},{"col":1,"coeff":"1/1"}]}]},"reduced":{"numVars":2,"numRows":1,"objConst":"4/1","obj":["-2/1","-1/1"],"lb":["0/1","0/1"],"ub":["3/1","10/1"],"rows":[{"sense":"le","rhs":"8/1","terms":[{"col":0,"coeff":"1/1"},{"col":1,"coeff":"1/1"}]}]},"step":{"kind":"singletonRow","origRow":0,"col":0,"origRowIdx":[1]},"xReduced":["3/1","5/1"],"yReduced":["-1/1"]}' \
        '{"mode":"lp-presolve","orig":{"numVars":2,"numRows":2,"objConst":"4/1","obj":["-2/1","-1/1"],"lb":["0/1","0/1"],"ub":["10/1","10/1"],"rows":[{"sense":"le","rhs":"6/1","terms":[{"col":0,"coeff":"2/1"}]},{"sense":"le","rhs":"8/1","terms":[{"col":0,"coeff":"1/1"},{"col":1,"coeff":"1/1"}]}]},"reduced":{"numVars":2,"numRows":1,"objConst":"4/1","obj":["-2/1","-1/1"],"lb":["0/1","0/1"],"ub":["3/1","10/1"],"rows":[{"sense":"le","rhs":"8/1","terms":[{"col":0,"coeff":"1/1"},{"col":1,"coeff":"1/1"}]}]},"step":{"kind":"singletonRow","origRow":0,"col":0,"origRowIdx":[1]},"xReduced":["3/1","5/1"],"yReduced":["0/1"]}' \
        '^MODE lp-presolve$'

    check_neg_cli_and_jsonl "${BIN_DIR}/lp-check" "cli-json-mode-mismatch" "optimal" \
        '{"mode":"safe-bound","numVars":1,"numRows":1,"objConst":"0/1","obj":["2/1"],"lb":["0/1"],"ub":["5/1"],"rows":[{"sense":"ge","rhs":"2/1","terms":[{"col":0,"coeff":"1/1"}]}],"y":["2/1"]}'

    check_neg_cli_and_jsonl "${BIN_DIR}/lp-check" "safe-bound-missing-x" "" \
        '{"mode":"safe-bound","numVars":1,"numRows":1,"objConst":"0/1","obj":["2/1"],"lb":["0/1"],"ub":["5/1"],"rows":[{"sense":"ge","rhs":"2/1","terms":[{"col":0,"coeff":"1/1"}]}],"y":["2/1"],"primalObj":"4/1","gapTol":"0/1"}'

    check_neg_cli_and_jsonl "${BIN_DIR}/lp-check" "lp-presolve-infeasible-xReduced-no-yReduced" "lp-presolve" \
        '{"mode":"lp-presolve","orig":{"numVars":2,"numRows":2,"objConst":"0/1","obj":["1/1","1/1"],"lb":["0/1","0/1"],"ub":["10/1","10/1"],"rows":[{"sense":"ge","rhs":"4/1","terms":[{"col":0,"coeff":"1/1"}]},{"sense":"ge","rhs":"6/1","terms":[{"col":0,"coeff":"1/1"},{"col":1,"coeff":"1/1"}]}]},"reduced":{"numVars":2,"numRows":1,"objConst":"0/1","obj":["1/1","1/1"],"lb":["4/1","0/1"],"ub":["10/1","10/1"],"rows":[{"sense":"ge","rhs":"6/1","terms":[{"col":0,"coeff":"1/1"},{"col":1,"coeff":"1/1"}]}]},"step":{"kind":"singletonRow","origRow":0,"col":0,"origRowIdx":[1]},"xReduced":["0/1","0/1"]}'

    check_neg_cli_and_jsonl "${BIN_DIR}/lp-check" "lp-presolve-missing-xReduced" "lp-presolve" \
        '{"mode":"lp-presolve","orig":{"numVars":2,"numRows":2,"objConst":"0/1","obj":["1/1","1/1"],"lb":["0/1","0/1"],"ub":["10/1","10/1"],"rows":[{"sense":"ge","rhs":"4/1","terms":[{"col":0,"coeff":"1/1"}]},{"sense":"ge","rhs":"6/1","terms":[{"col":0,"coeff":"1/1"},{"col":1,"coeff":"1/1"}]}]},"reduced":{"numVars":2,"numRows":1,"objConst":"0/1","obj":["1/1","1/1"],"lb":["4/1","0/1"],"ub":["10/1","10/1"],"rows":[{"sense":"ge","rhs":"6/1","terms":[{"col":0,"coeff":"1/1"},{"col":1,"coeff":"1/1"}]}]},"step":{"kind":"singletonRow","origRow":0,"col":0,"origRowIdx":[1]}}'

    check_neg_cli_and_jsonl "${BIN_DIR}/lp-check" "optimal-sense-max" "optimal" \
        '{"mode":"optimal","sense":"max","maximize":true,"numVars":1,"numRows":1,"objConst":"0","obj":["1"],"lb":["0"],"ub":["10"],"rows":[{"terms":[{"col":0,"coeff":"1"}],"sense":"ge","rhs":"0"}],"x":["0"],"y":["0"],"tol":"0"}'

    check_stdin_pos_neg "${BIN_DIR}/vipr-check" "orbitBranch" \
        '{"n":2,"objConst":0,"obj":[1,1],"isInt":[true,true],"lb":[0,0],"ub":[1,1],"baseRows":[{"coeffs":[1,1],"sense":">=","rhs":1}],"mode":"optimal","incumbent":[1,0],"steps":[{"rule":"assume","col":0,"sense":"<=","bound":0},{"rule":"assume","col":1,"sense":"<=","bound":0},{"rule":"farkas","refs":[{"id":0,"mult":-1},{"id":1,"mult":1},{"id":2,"mult":1}]},{"rule":"assume","col":0,"sense":">=","bound":1},{"rule":"safeDual","refs":[],"boundAsms":[4]},{"rule":"orbitBranch","col":0,"leftAsm":1,"rightAsm":4,"left":3,"right":5,"orbitAsms":[2],"witnesses":[{"companion":1,"colPerm":[1,0],"rowPerm":[0]}]}],"rootStep":5}' \
        '{"n":2,"objConst":0,"obj":[1,2],"isInt":[true,true],"lb":[0,0],"ub":[1,1],"baseRows":[{"coeffs":[1,1],"sense":">=","rhs":1}],"mode":"optimal","incumbent":[1,0],"steps":[{"rule":"assume","col":0,"sense":"<=","bound":0},{"rule":"assume","col":1,"sense":"<=","bound":0},{"rule":"farkas","refs":[{"id":0,"mult":-1},{"id":1,"mult":1},{"id":2,"mult":1}]},{"rule":"assume","col":0,"sense":">=","bound":1},{"rule":"safeDual","refs":[],"boundAsms":[4]},{"rule":"orbitBranch","col":0,"leftAsm":1,"rightAsm":4,"left":3,"right":5,"orbitAsms":[2],"witnesses":[{"companion":1,"colPerm":[1,0],"rowPerm":[0]}]}],"rootStep":5}' \
        '^VERIFIED$'

    check_stdin_pos_neg "${BIN_DIR}/vipr-check" "orig+presolve" \
        '{"orig":{"n":2,"objConst":0,"obj":[1,1],"isInt":[true,true],"lb":["1/2",1],"ub":[5,5],"baseRows":[{"coeffs":[2,2],"sense":">=","rhs":5}]},"presolve":[{"op":"roundBound","col":0,"isUpper":false,"newBound":1},{"op":"gcdRowDiv","row":0,"divisor":2},{"op":"symBreakLe","u":0,"v":1,"colPerm":[1,0],"rowPerm":[0]}],"n":2,"objConst":0,"obj":[1,1],"isInt":[true,true],"lb":[1,1],"ub":[5,5],"baseRows":[{"coeffs":[1,1],"sense":">=","rhs":3},{"coeffs":[1,-1],"sense":"<=","rhs":0}],"mode":"optimal","incumbent":[1,2],"steps":[{"rule":"dualBound","refs":[{"id":0,"mult":1}]}],"rootStep":0}' \
        '{"orig":{"n":2,"objConst":0,"obj":[1,1],"isInt":[true,true],"lb":["1/2",1],"ub":[5,5],"baseRows":[{"coeffs":[2,2],"sense":">=","rhs":5}]},"presolve":[{"op":"roundBound","col":0,"isUpper":false,"newBound":2},{"op":"gcdRowDiv","row":0,"divisor":2},{"op":"symBreakLe","u":0,"v":1,"colPerm":[1,0],"rowPerm":[0]}],"n":2,"objConst":0,"obj":[1,1],"isInt":[true,true],"lb":[2,1],"ub":[5,5],"baseRows":[{"coeffs":[1,1],"sense":">=","rhs":3},{"coeffs":[1,-1],"sense":"<=","rhs":0}],"mode":"optimal","incumbent":[2,2],"steps":[{"rule":"dualBound","refs":[{"id":0,"mult":1}]}],"rootStep":0}' \
        '^VERIFIED$'

    check_neg_cli_and_jsonl "${BIN_DIR}/vipr-check" "duplicate-key" "" \
        '{"n":1,"n":1,"objConst":0,"obj":[1],"isInt":[true],"lb":[0],"ub":[1],"baseRows":[],"mode":"optimal","incumbent":[0],"steps":[{"rule":"safeDual","refs":[],"boundAsms":[]}],"rootStep":0}'

    check_neg_cli_and_jsonl "${BIN_DIR}/vipr-check" "n-1e12-sparse-obj-oom-guard" "" \
        '{"n":1000000000000,"obj":{"terms":[]},"isInt":[],"lb":[],"ub":[],"baseRows":[],"mode":"optimal"}'

    printf '%s\n' '{"mode":"match","symbol":"divCeilInt64","words":[4181724048,3945817087,1409286633,4162785278,4162814973,3506447357,3019899169,2596342786,2596342811,2600567649,4043309119,2592265280,4167009277,4165011454,3596551104,2549614137,3573751839,2835384288,2854093795,2549681013,2839578592,402653163],"targets":[{"idx":15,"symbol":"runtime.panicdivide"},{"idx":19,"symbol":"runtime.morestack_noctxt.abi0"}]}' \
        | "${BIN_DIR}/bin-check" - | grep -q '^VERIFIED$'
    printf '  [OK] bin-check divCeilInt64 exact control\n'

    check_stdin_only_neg "${BIN_DIR}/bin-check" "p5-cmp-x0-x0-panicBounds" "panicBounds" \
        '{"mode":"cfg","words":[4181724048,3945817087,1409286377,4162723838,3942645791,1409286249,4165085182,3596551104,2486173696,2854093795,2485125120,402653173],"targets":[{"idx":8,"symbol":"runtime.panicBounds"},{"idx":10,"symbol":"runtime.morestack_noctxt.abi0"}]}'

    check_stdin_only_neg "${BIN_DIR}/bin-check" "p1-makemap-zeroGC" "ZeroGC" \
        '{"mode":"cfg","words":[4181724048,3945817087,1409286313,4162723838,2484076544,4165085182,3596551104,2854093795,2485125120,402653175],"targets":[{"idx":4,"symbol":"runtime.makemap"},{"idx":8,"symbol":"runtime.morestack_noctxt.abi0"}]}'

    check_stdin_only_neg "${BIN_DIR}/bin-check" "p3-ret-x1" "ZeroGC" \
        '{"mode":"cfg","words":[4181724048,3945817087,1409286377,4162723838,4181721089,2483027971,4165085182,3596551104,3596550176,2854093795,2485125120,402653173],"targets":[{"idx":10,"symbol":"runtime.morestack_noctxt.abi0"}]}'

    check_stdin_only_neg "${BIN_DIR}/bin-check" "p6-sdiv-s-bit" "unknown ARM64" \
        '{"mode":"match","symbol":"divCeilInt64","words":[4181724048,3945817087,1409286633,4162785278,4162814973,3506447357,3019899169,3133213698,3133213723,2600567649,4043309119,2592265280,4167009277,4165011454,3596551104,2549614137,3573751839,2835384288,2854093795,2549681013,2839578592,402653163],"targets":[{"idx":15,"symbol":"runtime.panicdivide"},{"idx":19,"symbol":"runtime.morestack_noctxt.abi0"}]}'

    printf '%s\n' '{"functions":[{"name":"pkg.cleanAVX2","insns":[{"addr":"10","op":"vmovdqu","args":"(%rax),%ymm0"},{"addr":"14","op":"vzeroupper","args":""},{"addr":"17","op":"ret","args":""}]}]}' \
        | "${BIN_DIR}/simd-cfg-check" - | grep -q '^VERIFIED (1 vector functions)$'
    printf '  [OK] simd-cfg-check clean AVX2 control\n'

    check_stdin_only_neg "${BIN_DIR}/simd-cfg-check" "A2-stack-prologue-evex" "UNGUARDED_EVEX" \
        '{"functions":[{"name":"internal/simd.sumZ","insns":[{"addr":"4a0000","op":"cmp","args":"0x10(%r14),%rsp"},{"addr":"4a0004","op":"jbe","args":"4a0030 <internal/simd.sumZ+0x30>"},{"addr":"4a0006","op":"push","args":"%rbp"},{"addr":"4a0007","op":"vmovdqu64","args":"(%rax),%zmm0"},{"addr":"4a000d","op":"vzeroupper","args":""},{"addr":"4a0010","op":"pop","args":"%rbp"},{"addr":"4a0011","op":"ret","args":""},{"addr":"4a0030","op":"call","args":"460000 <runtime.morestack_noctxt.abi0>","raw":"call 460000 <runtime.morestack_noctxt.abi0>"},{"addr":"4a0035","op":"jmp","args":"4a0000 <internal/simd.sumZ>"}]}]}'

    check_stdin_only_neg "${BIN_DIR}/simd-cfg-check" "notrack-indirect-jmp" "INDIRECT_OR_UNRESOLVED_JMP" \
        '{"functions":[{"name":"pkg.ntj","insns":[{"addr":"10","op":"vmovdqu","args":"(%rax),%ymm0"},{"addr":"14","op":"notrack","args":"jmp *%rcx"},{"addr":"18","op":"vzeroupper","args":""},{"addr":"1b","op":"ret","args":""}]}]}'

    check_stdin_only_neg "${BIN_DIR}/simd-cfg-check" "ymm17-evex" "UNGUARDED_EVEX" \
        '{"functions":[{"name":"pkg.m17","insns":[{"addr":"10","op":"vaddpd","args":"%ymm17,%ymm0,%ymm0"},{"addr":"16","op":"vzeroupper","args":""},{"addr":"19","op":"ret","args":""}]}]}'

    check_stdin_only_neg "${BIN_DIR}/simd-cfg-check" "vpternlogq-evex" "UNGUARDED_EVEX" \
        '{"functions":[{"name":"pkg.tern","insns":[{"addr":"10","op":"VPTERNLOGQ","args":"$0xff,%ymm0,%ymm1,%ymm2"},{"addr":"16","op":"vzeroupper","args":""},{"addr":"19","op":"ret","args":""}]}]}'

    printf '%s\n' '{"functions":[{"name":"validAVX512Dispatch","insns":[{"addr":"0x0","op":"cmpb","args":"$0x0, 0x1234(%rip) # <x86HasAVX512>"},{"addr":"0x7","op":"je","args":"0x20"},{"addr":"0x9","op":"vaddpd","args":"%zmm0, %zmm1, %zmm2"},{"addr":"0xf","op":"vzeroupper","args":""},{"addr":"0x12","op":"retq","args":""},{"addr":"0x20","op":"retq","args":""}]}]}' \
        | "${BIN_DIR}/simd-cfg-check" - | grep -q '^VERIFIED (1 vector functions)$'
    printf '  [OK] simd-cfg-check validAVX512Dispatch control\n'

    check_stdin_only_neg "${BIN_DIR}/simd-cfg-check" "inverted-avx512-guard" "UNGUARDED_EVEX" \
        '{"functions":[{"name":"invertedAVX512Dispatch","insns":[{"addr":"0x0","op":"cmpb","args":"$0x0, 0x1234(%rip) # <x86HasAVX512>"},{"addr":"0x7","op":"je","args":"0x10"},{"addr":"0x9","op":"retq","args":""},{"addr":"0x10","op":"vaddpd","args":"%zmm0, %zmm1, %zmm2"},{"addr":"0x16","op":"vzeroupper","args":""},{"addr":"0x19","op":"retq","args":""}]}]}'

    check_stdin_only_neg "${BIN_DIR}/simd-cfg-check" "fallthrough-evex-guard" "UNGUARDED_EVEX" \
        '{"functions":[{"name":"evexOnNonAVX512Fallthrough","insns":[{"addr":"0x0","op":"cmpb","args":"$0x0, 0x1234(%rip) # <x86HasAVX512>"},{"addr":"0x7","op":"jne","args":"0x20"},{"addr":"0x9","op":"vaddpd","args":"%zmm0, %zmm1, %zmm2"},{"addr":"0xf","op":"vzeroupper","args":""},{"addr":"0x12","op":"retq","args":""},{"addr":"0x20","op":"retq","args":""}]}]}'

    check_stdin_only_neg "${BIN_DIR}/simd-cfg-check" "vpmullq-evex" "UNGUARDED_EVEX" \
        '{"functions":[{"name":"unguardedAVX512DQ","insns":[{"addr":"0x0","op":"vpmullq","args":"%ymm0, %ymm1, %ymm2"},{"addr":"0x6","op":"vzeroupper","args":""},{"addr":"0x9","op":"retq","args":""}]}]}'

    check_stdin_only_neg "${BIN_DIR}/simd-cfg-check" "intel-syntax-ymm" "Plan 9" \
        '{"functions":[{"name":"pkg.intelDirtyYmm","insns":[{"addr":"0x0","op":"vmovdqu","args":"ymm0, ymmword ptr [rax]"},{"addr":"0x4","op":"ret","args":""}]}]}'

    check_stdin_only_neg "${BIN_DIR}/simd-cfg-check" "intel-opmask-syntax" "Plan 9" \
        '{"functions":[{"name":"pkg.intelOpmask","insns":[{"addr":"0x0","op":"kmovw","args":"k1, eax"},{"addr":"0x4","op":"vaddpd","args":"%zmm0, %zmm1, %zmm2{k1}"},{"addr":"0xa","op":"ret","args":""}]}]}'
)

log_step "3/5 Running Go formal differential and certified tests (default build)..."
GOBLIN_FORMAL=1 go test -count=1 -v -run '^Test(Formal|VIPR|OraclePool|Certified|AssignCertified|MiniVIPR)' . ./assign ./internal/mad

log_step "4/5 Running Go formal differential and certified tests (GOEXPERIMENT=simd)..."
GOEXPERIMENT=simd GOBLIN_FORMAL=1 go test -count=1 -v -run '^Test(Formal|VIPR|OraclePool|Certified|AssignCertified|MiniVIPR)' . ./assign ./internal/mad

log_step "5/5 Running Go formal fuzz seed corpora across all packages (GOEXPERIMENT=simd)..."
GOEXPERIMENT=simd GOBLIN_FORMAL=1 go test -count=1 -run '^Fuzz' ./...

printf '\nAll Lean 4 formal verification gates passed.\n'
