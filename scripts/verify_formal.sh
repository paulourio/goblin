#!/usr/bin/env bash
set -euo pipefail

# End-to-end verification script for the Lean 4 Prove Lane.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(git -C "${SCRIPT_DIR}" rev-parse --show-toplevel)"
cd "${REPO_ROOT}"

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
    local out
    out="$("${exe}" "${fixture}")"
    printf '%s\n' "${out}"
    if ! printf '%s\n' "${out}" | grep -q '^VERIFIED'; then
        printf 'ERROR: %s %s did not print VERIFIED.\n' "${exe}" "${fixture}" >&2
        exit 1
    fi
}

check_subcmd_pos_neg() {
    local exe="$1"
    local subcmd="$2"
    local pos_json="$3"
    local neg_json="$4"
    local expect_re="$5"
    local out jsonl_out

    out="$(printf '%s\n' "${pos_json}" | "${exe}" "${subcmd}" -)"
    if ! printf '%s\n' "${out}" | grep -Eq "${expect_re}"; then
        printf 'ERROR: %s %s positive CLI check failed: %s\n' "${exe}" "${subcmd}" "${out}" >&2
        exit 1
    fi

    jsonl_out="$(printf '%s\n' "${pos_json}" | "${exe}" "${subcmd}" --jsonl)"
    if ! printf '%s\n' "${jsonl_out}" | grep -q '"ok":true'; then
        printf 'ERROR: %s %s --jsonl positive check failed: %s\n' "${exe}" "${subcmd}" "${jsonl_out}" >&2
        exit 1
    fi

    if printf '%s\n' "${neg_json}" | "${exe}" "${subcmd}" - >/dev/null 2>&1; then
        printf 'ERROR: %s %s accepted negative payload.\n' "${exe}" "${subcmd}" >&2
        exit 1
    fi

    jsonl_out="$(printf '%s\n' "${neg_json}" | "${exe}" "${subcmd}" --jsonl)"
    if ! printf '%s\n' "${jsonl_out}" | grep -q '"ok":false'; then
        printf 'ERROR: %s %s --jsonl accepted negative payload: %s\n' "${exe}" "${subcmd}" "${jsonl_out}" >&2
        exit 1
    fi

    printf '  [OK] %s %s (CLI + --jsonl positive and negative)\n' "$(basename "${exe}")" "${subcmd}"
}

check_stdin_pos_neg() {
    local exe="$1"
    local label="$2"
    local pos_json="$3"
    local neg_json="$4"
    local expect_re="$5"
    local out jsonl_out

    out="$(printf '%s\n' "${pos_json}" | "${exe}" -)"
    if ! printf '%s\n' "${out}" | grep -Eq "${expect_re}"; then
        printf 'ERROR: %s (%s) positive CLI check failed: %s\n' "${exe}" "${label}" "${out}" >&2
        exit 1
    fi

    jsonl_out="$(printf '%s\n' "${pos_json}" | "${exe}" --jsonl)"
    if ! printf '%s\n' "${jsonl_out}" | grep -q '"ok":true'; then
        printf 'ERROR: %s (%s) --jsonl positive check failed: %s\n' "${exe}" "${label}" "${jsonl_out}" >&2
        exit 1
    fi

    if printf '%s\n' "${neg_json}" | "${exe}" - >/dev/null 2>&1; then
        printf 'ERROR: %s (%s) accepted negative payload.\n' "${exe}" "${label}" >&2
        exit 1
    fi

    jsonl_out="$(printf '%s\n' "${neg_json}" | "${exe}" --jsonl)"
    if ! printf '%s\n' "${jsonl_out}" | grep -q '"ok":false'; then
        printf 'ERROR: %s (%s) --jsonl accepted negative payload: %s\n' "${exe}" "${label}" "${jsonl_out}" >&2
        exit 1
    fi

    printf '  [OK] %s %s (CLI + --jsonl positive and negative)\n' "$(basename "${exe}")" "${label}"
}

export LEAN_NUM_THREADS="${LEAN_NUM_THREADS:-16}"
log_step "1/5 Building Lean 4 proofs, axiom audit, and ELF byte extractors (LEAN_NUM_THREADS=${LEAN_NUM_THREADS})..."
if command -v flock >/dev/null 2>&1; then
    (cd formal && flock /tmp/goblin-formal-lake.lock lake build --wfail)
else
    (cd formal && lake build --wfail)
fi
PYTHONDONTWRITEBYTECODE=1 python3 scripts/extract_elf_track5.py --verify
PYTHONDONTWRITEBYTECODE=1 python3 scripts/extract_amd64_track5e.py --verify

log_step "2/5 Running CLI positive, negative, subcommand, and help smoke checks..."
BIN_DIR="${REPO_ROOT}/formal/.lake/build/bin"
(
    cd formal
    check_verified "${BIN_DIR}/lsap-check" "../testdata/formal/lsap_sample.json"
    if "${BIN_DIR}/lsap-check" "../testdata/formal/lsap_negative.json" >/dev/null 2>&1; then
        echo "ERROR: lsap-check accepted negative fixture ../testdata/formal/lsap_negative.json." >&2
        exit 1
    fi
    echo "lsap-check rejected ../testdata/formal/lsap_negative.json as expected."
    check_verified "${BIN_DIR}/lp-check" "../testdata/formal/lp_optimal_sample.json"
    check_verified "${BIN_DIR}/lp-check" "../testdata/formal/lp_farkas_sample.json"
    check_verified "${BIN_DIR}/vipr-check" "../testdata/formal/vipr_sample.json"
    for cli in rules-check cuts-check bin-check simd-cfg-check; do
        "${BIN_DIR}/${cli}" --help >/dev/null
        "${BIN_DIR}/${cli}" >/dev/null
    done

    check_subcmd_pos_neg "${BIN_DIR}/cuts-check" "cover-check" \
        '{"op":"cover-check","numVars":3,"lb":["0/1","0/1","0/1"],"ub":["1/1","1/1","1/1"],"row":{"terms":[{"col":0,"val":"3/1"},{"col":1,"val":"3/1"},{"col":2,"val":"2/1"}],"sense":"le","rhs":"4/1"},"sign":"1/1","items":[{"col":0,"weight":"3/1","comp":false,"inCover":true,"alpha":1},{"col":1,"weight":"3/1","comp":false,"inCover":true,"alpha":1},{"col":2,"weight":"2/1","comp":false,"inCover":false,"alpha":0}],"knapRHS":"4/1","scale":"1/1","cut":{"terms":[{"col":0,"val":"1/1"},{"col":1,"val":"1/1"}],"rhs":"1/1"}}' \
        '{"op":"cover-check","numVars":3,"lb":["0/1","0/1","0/1"],"ub":["1/1","1/1","1/1"],"row":{"terms":[{"col":0,"val":"3/1"},{"col":1,"val":"3/1"},{"col":2,"val":"2/1"}],"sense":"le","rhs":"4/1"},"sign":"1/1","items":[{"col":0,"weight":"3/1","comp":false,"inCover":true,"alpha":1},{"col":1,"weight":"3/1","comp":false,"inCover":true,"alpha":1},{"col":2,"weight":"2/1","comp":false,"inCover":false,"alpha":0}],"knapRHS":"4/1","scale":"1/1","cut":{"terms":[{"col":0,"val":"1/1"},{"col":1,"val":"1/1"}],"rhs":"0/1"}}' \
        '^VERIFIED'

    check_subcmd_pos_neg "${BIN_DIR}/cuts-check" "lotsizing-check" \
        '{"op":"lotsizing-check","numVars":3,"invInCol":null,"nodes":[{"prodCol":0,"invOutCol":1,"binCol":2,"demand":"10/1","vubCap":"40/1","vubRHS":"0/1","inS":true,"coeffY":"10/1"}],"cutDem":"10/1","scale":"1/1","cut":{"terms":[{"col":2,"val":"-10/1"}],"rhs":"-10/1"}}' \
        '{"op":"lotsizing-check","numVars":3,"invInCol":null,"nodes":[{"prodCol":0,"invOutCol":1,"binCol":2,"demand":"10/1","vubCap":"40/1","vubRHS":"0/1","inS":true,"coeffY":"10/1"}],"cutDem":"10/1","scale":"1/1","cut":{"terms":[{"col":2,"val":"-10/1"}],"rhs":"-11/1"}}' \
        '^VERIFIED'

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

    check_subcmd_pos_neg "${BIN_DIR}/lp-check" "unbounded" \
        '{"mode":"unbounded","numVars":1,"numRows":1,"obj":["-1/1"],"lb":["0/1"],"ub":[null],"rows":[{"sense":"ge","rhs":"2/1","terms":[{"col":0,"coeff":"1/1"}]}],"x0":["2/1"],"d":["1/1"]}' \
        '{"mode":"unbounded","numVars":1,"numRows":1,"obj":["1/1"],"lb":["0/1"],"ub":[null],"rows":[{"sense":"ge","rhs":"2/1","terms":[{"col":0,"coeff":"1/1"}]}],"x0":["2/1"],"d":["1/1"]}' \
        '^VERIFIED$'

    check_subcmd_pos_neg "${BIN_DIR}/lp-check" "milp-unbounded" \
        '{"mode":"milp-unbounded","numVars":2,"numRows":1,"intCols":[0],"obj":["-2/1","0/1"],"lb":["2/1","0/1"],"ub":[null,null],"rows":[{"sense":"eq","rhs":"0/1","terms":[{"col":0,"coeff":"-1/1"},{"col":1,"coeff":"3/1"}]}],"x0":["3/1","1/1"],"d":["3/1","1/1"]}' \
        '{"mode":"milp-unbounded","numVars":2,"numRows":1,"intCols":[0,1],"obj":["-2/1","0/1"],"lb":["2/1","0/1"],"ub":[null,null],"rows":[{"sense":"eq","rhs":"0/1","terms":[{"col":0,"coeff":"-1/1"},{"col":1,"coeff":"3/1"}]}],"x0":["3/1","1/1"],"d":["1/1","1/3"]}' \
        '^VERIFIED$'

    check_subcmd_pos_neg "${BIN_DIR}/lp-check" "lp-presolve" \
        '{"mode":"lp-presolve","orig":{"numVars":2,"numRows":2,"objConst":"4/1","obj":["-2/1","-1/1"],"lb":["0/1","0/1"],"ub":["10/1","10/1"],"rows":[{"sense":"le","rhs":"6/1","terms":[{"col":0,"coeff":"2/1"}]},{"sense":"le","rhs":"8/1","terms":[{"col":0,"coeff":"1/1"},{"col":1,"coeff":"1/1"}]}]},"reduced":{"numVars":2,"numRows":1,"objConst":"4/1","obj":["-2/1","-1/1"],"lb":["0/1","0/1"],"ub":["3/1","10/1"],"rows":[{"sense":"le","rhs":"8/1","terms":[{"col":0,"coeff":"1/1"},{"col":1,"coeff":"1/1"}]}]},"step":{"kind":"singletonRow","origRow":0,"col":0,"origRowIdx":[1]},"xReduced":["3/1","5/1"],"yReduced":["-1/1"]}' \
        '{"mode":"lp-presolve","orig":{"numVars":2,"numRows":2,"objConst":"4/1","obj":["-2/1","-1/1"],"lb":["0/1","0/1"],"ub":["10/1","10/1"],"rows":[{"sense":"le","rhs":"6/1","terms":[{"col":0,"coeff":"2/1"}]},{"sense":"le","rhs":"8/1","terms":[{"col":0,"coeff":"1/1"},{"col":1,"coeff":"1/1"}]}]},"reduced":{"numVars":2,"numRows":1,"objConst":"4/1","obj":["-2/1","-1/1"],"lb":["0/1","0/1"],"ub":["3/1","10/1"],"rows":[{"sense":"le","rhs":"8/1","terms":[{"col":0,"coeff":"1/1"},{"col":1,"coeff":"1/1"}]}]},"step":{"kind":"singletonRow","origRow":0,"col":0,"origRowIdx":[1]},"xReduced":["3/1","5/1"],"yReduced":["0/1"]}' \
        '^VERIFIED$'

    check_stdin_pos_neg "${BIN_DIR}/vipr-check" "orbitBranch" \
        '{"n":2,"objConst":0,"obj":[1,1],"isInt":[true,true],"lb":[0,0],"ub":[1,1],"baseRows":[{"coeffs":[1,1],"sense":">=","rhs":1}],"mode":"optimal","incumbent":[1,0],"steps":[{"rule":"assume","col":0,"sense":"<=","bound":0},{"rule":"assume","col":1,"sense":"<=","bound":0},{"rule":"farkas","refs":[{"id":0,"mult":-1},{"id":1,"mult":1},{"id":2,"mult":1}]},{"rule":"assume","col":0,"sense":">=","bound":1},{"rule":"safeDual","refs":[],"boundAsms":[4]},{"rule":"orbitBranch","col":0,"leftAsm":1,"rightAsm":4,"left":3,"right":5,"orbitAsms":[2],"witnesses":[{"companion":1,"colPerm":[1,0],"rowPerm":[0]}]}],"rootStep":5}' \
        '{"n":2,"objConst":0,"obj":[1,2],"isInt":[true,true],"lb":[0,0],"ub":[1,1],"baseRows":[{"coeffs":[1,1],"sense":">=","rhs":1}],"mode":"optimal","incumbent":[1,0],"steps":[{"rule":"assume","col":0,"sense":"<=","bound":0},{"rule":"assume","col":1,"sense":"<=","bound":0},{"rule":"farkas","refs":[{"id":0,"mult":-1},{"id":1,"mult":1},{"id":2,"mult":1}]},{"rule":"assume","col":0,"sense":">=","bound":1},{"rule":"safeDual","refs":[],"boundAsms":[4]},{"rule":"orbitBranch","col":0,"leftAsm":1,"rightAsm":4,"left":3,"right":5,"orbitAsms":[2],"witnesses":[{"companion":1,"colPerm":[1,0],"rowPerm":[0]}]}],"rootStep":5}' \
        '^VERIFIED$'

    check_stdin_pos_neg "${BIN_DIR}/vipr-check" "orig+presolve" \
        '{"orig":{"n":2,"objConst":0,"obj":[1,1],"isInt":[true,true],"lb":["1/2",1],"ub":[5,5],"baseRows":[{"coeffs":[2,2],"sense":">=","rhs":5}]},"presolve":[{"op":"roundBound","col":0,"isUpper":false,"newBound":1},{"op":"gcdRowDiv","row":0,"divisor":2},{"op":"symBreakLe","u":0,"v":1,"colPerm":[1,0],"rowPerm":[0]}],"n":2,"objConst":0,"obj":[1,1],"isInt":[true,true],"lb":[1,1],"ub":[5,5],"baseRows":[{"coeffs":[1,1],"sense":">=","rhs":3},{"coeffs":[1,-1],"sense":"<=","rhs":0}],"mode":"optimal","incumbent":[1,2],"steps":[{"rule":"dualBound","refs":[{"id":0,"mult":1}]}],"rootStep":0}' \
        '{"orig":{"n":2,"objConst":0,"obj":[1,1],"isInt":[true,true],"lb":["1/2",1],"ub":[5,5],"baseRows":[{"coeffs":[2,2],"sense":">=","rhs":5}]},"presolve":[{"op":"roundBound","col":0,"isUpper":false,"newBound":2},{"op":"gcdRowDiv","row":0,"divisor":2},{"op":"symBreakLe","u":0,"v":1,"colPerm":[1,0],"rowPerm":[0]}],"n":2,"objConst":0,"obj":[1,1],"isInt":[true,true],"lb":[2,1],"ub":[5,5],"baseRows":[{"coeffs":[1,1],"sense":">=","rhs":3},{"coeffs":[1,-1],"sense":"<=","rhs":0}],"mode":"optimal","incumbent":[2,2],"steps":[{"rule":"dualBound","refs":[{"id":0,"mult":1}]}],"rootStep":0}' \
        '^VERIFIED$'
)

log_step "3/5 Running Go formal differential and certified tests (default build)..."
GOBLIN_FORMAL=1 go test -count=1 -v -run '^Test(Formal|VIPR|OraclePool|Certified|AssignCertified|MiniVIPR)' . ./assign

log_step "4/5 Running Go formal differential and certified tests (GOEXPERIMENT=simd)..."
GOEXPERIMENT=simd GOBLIN_FORMAL=1 go test -count=1 -v -run '^Test(Formal|VIPR|OraclePool|Certified|AssignCertified|MiniVIPR)' . ./assign

log_step "5/5 Running Go formal fuzz seed corpora (GOEXPERIMENT=simd)..."
GOEXPERIMENT=simd GOBLIN_FORMAL=1 go test -count=1 -run '^Fuzz' . ./assign

printf '\nAll Lean 4 formal verification gates passed.\n'
