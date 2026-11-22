#!/usr/bin/env bash
set -euo pipefail

# Local parallel fuzz soak runner for goblin (tuned for multi-core workstations).
# Runs up to CONCURRENCY fuzzers simultaneously, each with -parallel=WORKERS_PER_TARGET,
# leaving headroom for desktop responsiveness.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GOBLIN_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

PHASE="assign"
CONCURRENCY="${CONCURRENCY:-5}"
WORKERS_PER_TARGET="${WORKERS_PER_TARGET:-6}"
ASSIGN_TIME="${ASSIGN_TIME:-5m}"
SPARSE_TIME="${SPARSE_TIME:-10m}"
MAD_TIME="${MAD_TIME:-20m}"
RUN_ID="${RUN_ID:-local-$(date -u +%Y%m%d-%H%M%S)}"

for arg in "$@"; do
    case "${arg}" in
        --phase=*) PHASE="${arg#*=}" ;;
        --concurrency=*) CONCURRENCY="${arg#*=}" ;;
        --workers=*) WORKERS_PER_TARGET="${arg#*=}" ;;
        --assign-time=*) ASSIGN_TIME="${arg#*=}" ;;
        --sparse-time=*) SPARSE_TIME="${arg#*=}" ;;
        --mad-time=*) MAD_TIME="${arg#*=}" ;;
        *)
            echo "usage: $0 [--phase=assign|mad|all] [--concurrency=4] [--workers=6] [--assign-time=5m] [--sparse-time=10m] [--mad-time=20m]" >&2
            exit 2
            ;;
    esac
done

LOG_DIR="${GOBLIN_ROOT}/.cache/local-fuzz/${RUN_ID}"
mkdir -p "${LOG_DIR}"

declare -a JOBS=()
add_assign_jobs() {
    JOBS+=(
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
        local last
        last="$(grep 'fuzz: elapsed:' "${LOG_DIR}/${t}.log" | tail -n1 || true)"
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
exit "${FAILURES}"
