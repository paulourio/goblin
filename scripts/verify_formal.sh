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
    if ! printf '%s\n' "${out}" | grep -q '^VERIFIED$'; then
        printf 'ERROR: %s %s did not print VERIFIED.\n' "${exe}" "${fixture}" >&2
        exit 1
    fi
}

log_step "1/4 Building Lean 4 proofs and axiom audit..."
if command -v flock >/dev/null 2>&1; then
    (cd formal && flock /tmp/goblin-formal-lake.lock lake build)
else
    (cd formal && lake build)
fi

log_step "2/4 Running CLI positive, negative, and help smoke checks..."
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
)

log_step "3/4 Running Go formal differential tests (default build)..."
GOBLIN_FORMAL=1 go test -count=1 -v -run TestFormal . ./assign

log_step "4/4 Running Go formal differential tests (GOEXPERIMENT=simd)..."
GOEXPERIMENT=simd GOBLIN_FORMAL=1 go test -count=1 -v -run TestFormal . ./assign

printf '\nAll Lean 4 formal verification gates passed.\n'
