#!/usr/bin/env bash
set -euo pipefail

# Local parallel fuzz soak runner for the solver suite.
# Runs up to CONCURRENCY fuzzers in parallel, each with -parallel=WORKERS_PER_TARGET.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GOBLIN_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

PHASE="assign"
FORMAL="${GOBLIN_FORMAL:-0}"
CONCURRENCY="${CONCURRENCY:-5}"
WORKERS_PER_TARGET="${WORKERS_PER_TARGET:-6}"
ASSIGN_TIME="${ASSIGN_TIME:-5m}"
SPARSE_TIME="${SPARSE_TIME:-10m}"
MAD_TIME="${MAD_TIME:-20m}"
RUN_ID="${RUN_ID:-local-$(date -u +%Y%m%d-%H%M%S)}"
VERIFY_LOG_ONLY=""

for arg in "$@"; do
    case "${arg}" in
        --formal) FORMAL="1" ;;
        --formal=1|--formal=true) FORMAL="1" ;;
        --formal=0|--formal=false) FORMAL="0" ;;
        --phase=*) PHASE="${arg#*=}" ;;
        --concurrency=*) CONCURRENCY="${arg#*=}" ;;
        --workers=*) WORKERS_PER_TARGET="${arg#*=}" ;;
        --assign-time=*) ASSIGN_TIME="${arg#*=}" ;;
        --sparse-time=*) SPARSE_TIME="${arg#*=}" ;;
        --mad-time=*) MAD_TIME="${arg#*=}" ;;
        --verify-log=*) VERIFY_LOG_ONLY="${arg#*=}" ;;
        *)
            echo "usage: $0 [--formal] [--phase=assign|mad|all] [--concurrency=4] [--workers=6] [--assign-time=5m] [--sparse-time=10m] [--mad-time=20m]" >&2
            exit 2
            ;;
    esac
done

verify_formal_target_log() {
    local target="$1"
    local log_file="$2"
    if [[ ! -s "${log_file}" ]]; then
        echo "formal log check failed for ${target}: empty or missing log ${log_file}" >&2
        return 1
    fi
    if grep -q -- '--- SKIP:' "${log_file}"; then
        echo "formal log check failed for ${target}: silent formal skip (--- SKIP:) detected in ${log_file}" >&2
        return 1
    fi
    if grep -Eq 'no fuzz tests to fuzz|no tests to run' "${log_file}"; then
        echo "formal log check failed for ${target}: no fuzz target executed in ${log_file}" >&2
        return 1
    fi
    if ! grep -q 'fuzz: elapsed:' "${log_file}"; then
        echo "formal log check failed for ${target}: missing 'fuzz: elapsed:' progress in ${log_file}" >&2
        return 1
    fi
    if ! grep -Eq 'fuzz: elapsed:.*(execs: [1-9][0-9]*|[1-9][0-9]*/[0-9]+ completed)' "${log_file}"; then
        echo "formal log check failed for ${target}: zero executed fuzz checks in ${log_file}" >&2
        return 1
    fi
    if ! grep -q '^PASS$' "${log_file}"; then
        echo "formal log check failed for ${target}: missing terminal PASS line in ${log_file}" >&2
        return 1
    fi
    return 0
}

if [[ -n "${VERIFY_LOG_ONLY}" ]]; then
    verify_formal_target_log "$(basename "${VERIFY_LOG_ONLY}")" "${VERIFY_LOG_ONLY}"
    exit 0
fi

if [[ "${FORMAL}" == "1" ]]; then
    export GOBLIN_FORMAL=1
    if ! command -v lake >/dev/null 2>&1 && [[ -n "${HOME:-}" && -x "${HOME}/.elan/bin/lake" ]]; then
        export PATH="${HOME}/.elan/bin:${PATH}"
    fi
    NEED_BUILD=0
    for bin_name in lp-check cuts-check rules-check vipr-check lsap-check; do
        if [[ ! -x "${GOBLIN_ROOT}/formal/.lake/build/bin/${bin_name}" ]]; then
            NEED_BUILD=1
            break
        fi
    done
    if [[ "${NEED_BUILD}" -eq 1 ]]; then
        (
            cd "${GOBLIN_ROOT}/formal"
            if command -v flock >/dev/null 2>&1; then
                flock /tmp/goblin-formal-lake.lock lake build lp-check cuts-check rules-check vipr-check lsap-check
            else
                lake build lp-check cuts-check rules-check vipr-check lsap-check
            fi
        )
    fi
fi

LOG_DIR="${GOBLIN_ROOT}/.cache/local-fuzz/${RUN_ID}"
mkdir -p "${LOG_DIR}"

declare -a JOBS=()
add_assign_jobs() {
    JOBS+=(
        "./assign|FuzzAssignAdversarialFormal|${ASSIGN_TIME}"
        "./assign|FuzzAssignPlantedCancellationAndStage2|${ASSIGN_TIME}"
        "./assign|FuzzAssignCollinearAndRowDominated|${ASSIGN_TIME}"
        "./assign|FuzzAssignWideAccAndIEEE754Edges|${ASSIGN_TIME}"
        "./assign|FuzzAssignHallAndTopology|${ASSIGN_TIME}"
        "./assign|FuzzAssignMetamorphicAndDirtyWorkspace|${ASSIGN_TIME}"
        "./assign|FuzzLinearSum|${ASSIGN_TIME}"
        "./assign|FuzzAssignSparseCoreTraps|${SPARSE_TIME}"
    )
}
add_mad_jobs() {
    JOBS+=(
        ".|FuzzAdversarialLPFormal|${MAD_TIME}"
        ".|FuzzAdversarialMIPFormal|${MAD_TIME}"
        ".|FuzzMIPSmall|${MAD_TIME}"
        ".|FuzzLPSmall|${MAD_TIME}"
        ".|FuzzMADExactOptimum|${MAD_TIME}"
        ".|FuzzMADAdversarialRegimes|${MAD_TIME}"
        ".|FuzzMADMetamorphicAndLimits|${MAD_TIME}"
        ".|FuzzMADMediumDecomposed|${MAD_TIME}"
        ".|FuzzMILPSolver|${MAD_TIME}"
        ".|FuzzLPSolver|${MAD_TIME}"
        ".|FuzzPuLPJSON|${MAD_TIME}"
    )
}

case "${PHASE}" in
    assign) add_assign_jobs ;;
    mad) add_mad_jobs ;;
    all) add_assign_jobs; add_mad_jobs ;;
    *)
        echo "ERROR: unknown --phase=${PHASE} (expected assign, mad, or all)" >&2
        exit 2
        ;;
esac

echo "=== Local Parallel Fuzz Soak (${RUN_ID}) ==="
echo "  Phase:       ${PHASE} (${#JOBS[@]} targets)"
echo "  Formal:      GOBLIN_FORMAL=${FORMAL}"
echo "  Parallelism: ${CONCURRENCY} targets x ${WORKERS_PER_TARGET} workers = $((CONCURRENCY * WORKERS_PER_TARGET)) threads"
echo "  Logs:        ${LOG_DIR}"

declare -A PID_TO_TARGET=()
FAILURES=0

cleanup() {
    for pid in "${!PID_TO_TARGET[@]}"; do
        kill -INT "${pid}" 2>/dev/null || true
    done
}
trap cleanup INT TERM

wait_one() {
    local done_pid=""
    if wait -n -p done_pid; then
        local t="${PID_TO_TARGET[${done_pid}]:-unknown}"
        local log_file="${LOG_DIR}/${t}.log"
        if [[ "${FORMAL}" == "1" ]] && ! verify_formal_target_log "${t}" "${log_file}"; then
            printf "  [FAIL] %-42s (formal log audit failed, see %s)\n" "${t}" "${log_file}"
            FAILURES=$((FAILURES + 1))
            unset "PID_TO_TARGET[${done_pid}]"
            return
        fi
        local last
        last="$(grep 'fuzz: elapsed:' "${log_file}" | tail -n1 || true)"
        printf "  [PASS] %-42s %s\n" "${t}" "${last}"
        unset "PID_TO_TARGET[${done_pid}]"
    else
        local rc=$?
        local t="${PID_TO_TARGET[${done_pid}]:-unknown}"
        printf "  [FAIL] %-42s (rc=%d, see %s/%s.log)\n" "${t}" "${rc}" "${LOG_DIR}" "${t}"
        FAILURES=$((FAILURES + 1))
        unset "PID_TO_TARGET[${done_pid}]"
    fi
}

cd "${GOBLIN_ROOT}"
for spec in "${JOBS[@]}"; do
    while [[ "${#PID_TO_TARGET[@]}" -ge "${CONCURRENCY}" ]]; do
        wait_one
    done
    IFS='|' read -r pkg target dur <<< "${spec}"
    echo "  [START] ${target} (${pkg}, -fuzztime=${dur}, -parallel=${WORKERS_PER_TARGET})"
    (
        GOEXPERIMENT=simd go test \
            -run='^$' \
            -fuzz="^${target}$" \
            -fuzztime="${dur}" \
            -parallel="${WORKERS_PER_TARGET}" \
            "${pkg}" > "${LOG_DIR}/${target}.log" 2>&1
    ) &
    PID_TO_TARGET[$!]="${target}"
done

while [[ "${#PID_TO_TARGET[@]}" -gt 0 ]]; do
    wait_one
done

echo ""
echo "=== Local Soak Complete: ${#JOBS[@]} targets, ${FAILURES} failures ==="
echo "Logs: ${LOG_DIR}"
if [[ "${FAILURES}" -gt 0 ]]; then
    exit 1
fi
exit 0
