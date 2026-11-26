#!/usr/bin/env bash
set -euo pipefail

# Local parallel fuzz soak runner for the solver suite.
# Precompiles coverage-instrumented binaries into a temporary directory outside the repo
# and supervises each target with goblin-fuzzwatch for saturation-aware early stopping.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GOBLIN_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

PHASE="assign"
FORMAL="${GOBLIN_FORMAL:-0}"
SIMD_MAX_TIER="${GOBLIN_SIMD_MAX_TIER:-}"
CONCURRENCY="${CONCURRENCY:-5}"
WORKERS_PER_TARGET="${WORKERS_PER_TARGET:-6}"
ASSIGN_TIME="${ASSIGN_TIME:-5m}"
SPARSE_TIME="${SPARSE_TIME:-10m}"
SIMD_TIME="${SIMD_TIME:-5m}"
FORMAT_TIME="${FORMAT_TIME:-10m}"
MAD_TIME="${MAD_TIME:-20m}"
ASSERT_TIME="${ASSERT_TIME:-20m}"
RACE_TIME="${RACE_TIME:-10m}"
CBC_TIME="${CBC_TIME:-20m}"
EARLY_STOP_PRESET="${EARLY_STOP_PRESET:-local}"
MIN_TIME="${MIN_TIME:-}"
MIN_EXECS="${MIN_EXECS:-}"
STALL_TIME="${STALL_TIME:-}"
STALL_EXECS="${STALL_EXECS:-}"
WINDOW_TIME="${WINDOW_TIME:-}"
WINDOW_EXECS="${WINDOW_EXECS:-}"
WINDOW_MIN_NEW="${WINDOW_MIN_NEW:-}"
RUN_ID="${RUN_ID:-local-$(date -u +%Y%m%d-%H%M%S)}"
VERIFY_LOG_ONLY=""
LIST_TARGETS=0

usage() {
    cat >&2 <<'EOF'
usage: local_fuzz_soak.sh [--formal] [--simd-max-tier=auto|scalar|avx2|avx512]
  [--phase=assign|simd|format|mad|assert|race|cbc|all|all-cbc]
  [--concurrency=5] [--workers=6]
  [--assign-time=5m] [--sparse-time=10m] [--simd-time=5m] [--format-time=10m]
  [--mad-time=20m] [--assert-time=20m] [--race-time=10m] [--cbc-time=20m]
  [--early-stop=local|soak|deep|off] [--min-time=D] [--min-execs=N]
  [--stall-time=D] [--stall-execs=N] [--window-time=D] [--window-execs=N] [--window-min-new=N]
  [--verify-log=PATH] [--list-targets]
EOF
}

for arg in "$@"; do
    case "${arg}" in
        --formal) FORMAL="1" ;;
        --formal=1|--formal=true) FORMAL="1" ;;
        --formal=0|--formal=false) FORMAL="0" ;;
        --simd-max-tier=*) SIMD_MAX_TIER="${arg#*=}" ;;
        --phase=*) PHASE="${arg#*=}" ;;
        --concurrency=*) CONCURRENCY="${arg#*=}" ;;
        --workers=*) WORKERS_PER_TARGET="${arg#*=}" ;;
        --assign-time=*) ASSIGN_TIME="${arg#*=}" ;;
        --sparse-time=*) SPARSE_TIME="${arg#*=}" ;;
        --simd-time=*) SIMD_TIME="${arg#*=}" ;;
        --format-time=*) FORMAT_TIME="${arg#*=}" ;;
        --mad-time=*) MAD_TIME="${arg#*=}" ;;
        --assert-time=*) ASSERT_TIME="${arg#*=}" ;;
        --race-time=*) RACE_TIME="${arg#*=}" ;;
        --cbc-time=*) CBC_TIME="${arg#*=}" ;;
        --early-stop=*) EARLY_STOP_PRESET="${arg#*=}" ;;
        --min-time=*) MIN_TIME="${arg#*=}" ;;
        --min-execs=*) MIN_EXECS="${arg#*=}" ;;
        --stall-time=*) STALL_TIME="${arg#*=}" ;;
        --stall-execs=*) STALL_EXECS="${arg#*=}" ;;
        --window-time=*) WINDOW_TIME="${arg#*=}" ;;
        --window-execs=*) WINDOW_EXECS="${arg#*=}" ;;
        --window-min-new=*) WINDOW_MIN_NEW="${arg#*=}" ;;
        --verify-log=*) VERIFY_LOG_ONLY="${arg#*=}" ;;
        --list-targets) LIST_TARGETS=1 ;;
        *)
            usage
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

case "${EARLY_STOP_PRESET}" in
    off|local|soak|deep) ;;
    *)
        echo "ERROR: unknown --early-stop=${EARLY_STOP_PRESET} (expected off, local, soak, or deep)" >&2
        exit 2
        ;;
esac

if [[ -n "${SIMD_MAX_TIER}" ]]; then
    case "${SIMD_MAX_TIER}" in
        auto|scalar|avx2|avx512) export GOBLIN_SIMD_MAX_TIER="${SIMD_MAX_TIER}" ;;
        *)
            echo "ERROR: unknown --simd-max-tier=${SIMD_MAX_TIER} (expected auto, scalar, avx2, or avx512)" >&2
            exit 2
            ;;
    esac
fi

if [[ "${LIST_TARGETS}" -eq 0 && "${FORMAL}" == "1" ]]; then
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

case "${PHASE}" in
    assign|simd|format|mad|assert|race|cbc|all|all-cbc) ;;
    *)
        echo "ERROR: unknown --phase=${PHASE} (expected assign, simd, format, mad, assert, race, cbc, all, or all-cbc)" >&2
        exit 2
        ;;
esac

TARGETS_TSV="${GOBLIN_ROOT}/scripts/gcp-fuzz/targets.tsv"
if [[ ! -f "${TARGETS_TSV}" ]]; then
    echo "ERROR: fuzz target manifest missing at ${TARGETS_TSV}" >&2
    exit 2
fi

declare -a JOBS=()
NEED_ASSIGN_BIN=0
NEED_SIMD_BIN=0
NEED_FORMAT_BIN=0
NEED_FORMAT_CBC_BIN=0
NEED_GOBLIN_BIN=0
NEED_ASSERT_BIN=0
NEED_RACE_BIN=0
NEED_CBC_BIN=0

# Derive fuzz soak jobs and required test binaries dynamically from targets.tsv.
while IFS=$'\t' read -r _idx _bin _workdir _target _class _maxpar _extra; do
    [[ -n "${_idx}" ]] || continue
    [[ "${_idx}" == \#* ]] && continue
    if [[ -z "${_bin}" || -z "${_workdir}" || -z "${_target}" || -z "${_class}" || -n "${_extra:-}" ]]; then
        echo "ERROR: malformed row in ${TARGETS_TSV}: ${_idx}" >&2
        exit 2
    fi
    job_phase=""
    pkg_tok=""
    dur=""
    need_var=""
    case "$(basename "${_bin}")" in
        assign.test)
            job_phase="assign"
            pkg_tok="./assign"
            dur="${ASSIGN_TIME}"
            if [[ "${_target}" == "FuzzAssignSparseCoreTraps" ]]; then
                dur="${SPARSE_TIME}"
            fi
            need_var="NEED_ASSIGN_BIN"
            ;;
        simd.test)
            job_phase="simd"
            pkg_tok="./internal/simd"
            dur="${SIMD_TIME}"
            need_var="NEED_SIMD_BIN"
            ;;
        format.test)
            job_phase="format"
            pkg_tok="./format"
            dur="${FORMAT_TIME}"
            need_var="NEED_FORMAT_BIN"
            ;;
        format_cbc.test)
            job_phase="cbc"
            pkg_tok="./format-cbc"
            dur="${CBC_TIME}"
            need_var="NEED_FORMAT_CBC_BIN"
            ;;
        goblin.test)
            job_phase="mad"
            pkg_tok="."
            dur="${MAD_TIME}"
            need_var="NEED_GOBLIN_BIN"
            ;;
        goblin_assert.test)
            job_phase="assert"
            pkg_tok=".-assert"
            dur="${ASSERT_TIME}"
            need_var="NEED_ASSERT_BIN"
            ;;
        goblin_race.test)
            job_phase="race"
            pkg_tok=".-race"
            dur="${RACE_TIME}"
            need_var="NEED_RACE_BIN"
            ;;
        goblin_cbc.test)
            job_phase="cbc"
            pkg_tok=".-cbc"
            dur="${CBC_TIME}"
            need_var="NEED_CBC_BIN"
            ;;
        *)
            echo "ERROR: unsupported binary ${_bin} in ${TARGETS_TSV}" >&2
            exit 2
            ;;
    esac

    include_job=0
    case "${PHASE}" in
        all)
            if [[ "${job_phase}" != "cbc" ]]; then
                include_job=1
            fi
            ;;
        all-cbc)
            include_job=1
            ;;
        *)
            if [[ "${job_phase}" == "${PHASE}" ]]; then
                include_job=1
            fi
            ;;
    esac
    if [[ "${include_job}" -eq 1 ]]; then
        printf -v "${need_var}" '1'
        JOBS+=("${pkg_tok}|${_target}|${_class}|${dur}")
    fi
done < "${TARGETS_TSV}"

if [[ "${LIST_TARGETS}" -eq 1 ]]; then
    for spec in "${JOBS[@]}"; do
        IFS='|' read -r _pkg target _class _dur <<< "${spec}"
        printf '%s\n' "${target}"
    done
    exit 0
fi

if [[ ( "${NEED_CBC_BIN}" -eq 1 || "${NEED_FORMAT_CBC_BIN}" -eq 1 ) && ! -d "${GOBLIN_ROOT}/third_party/Cbc/dist/lib" ]]; then
    echo "ERROR: Cbc artifacts not found in ${GOBLIN_ROOT}/third_party/Cbc/dist/lib; run scripts/build_cbc.sh first." >&2
    exit 2
fi

LOG_DIR="${GOBLIN_ROOT}/.cache/local-fuzz/${RUN_ID}"
mkdir -p "${LOG_DIR}"
BIN_DIR="$(mktemp -d)"

declare -A PID_TO_TARGET=()
FAILURES=0
INTERRUPTED=0

cleanup() {
    INTERRUPTED=1
    for pid in "${!PID_TO_TARGET[@]}"; do
        kill -TERM "${pid}" 2>/dev/null || true
    done
    wait 2>/dev/null || true
    rm -rf "${BIN_DIR}"
    exit 130
}

cleanup_exit() {
    rm -rf "${BIN_DIR}"
}

trap cleanup INT TERM
trap cleanup_exit EXIT

echo "=== Local Parallel Fuzz Soak (${RUN_ID}) ==="
echo "  Phase:       ${PHASE} (${#JOBS[@]} targets)"
echo "  Formal/SIMD: GOBLIN_FORMAL=${FORMAL} GOBLIN_SIMD_MAX_TIER=${SIMD_MAX_TIER:-auto}"
echo "  Early stop:  preset=${EARLY_STOP_PRESET}"
echo "  Parallelism: ${CONCURRENCY} targets x ${WORKERS_PER_TARGET} workers = $((CONCURRENCY * WORKERS_PER_TARGET)) threads"
echo "  Logs:        ${LOG_DIR}"

echo ">>> Compiling goblin-fuzzwatch and coverage-instrumented test binaries..."
cd "${GOBLIN_ROOT}"
CGO_ENABLED=0 go build -o "${BIN_DIR}/goblin-fuzzwatch" ./cmd/goblin-fuzzwatch
if [[ "${NEED_ASSIGN_BIN}" -eq 1 ]]; then
    GOEXPERIMENT=simd go test -c -fuzz=. -o "${BIN_DIR}/assign.test" ./assign
fi
if [[ "${NEED_SIMD_BIN}" -eq 1 ]]; then
    GOEXPERIMENT=simd go test -c -fuzz=. -o "${BIN_DIR}/simd.test" ./internal/simd
fi
if [[ "${NEED_FORMAT_BIN}" -eq 1 ]]; then
    GOEXPERIMENT=simd go test -c -fuzz=. -o "${BIN_DIR}/format.test" ./format
fi
if [[ "${NEED_FORMAT_CBC_BIN}" -eq 1 ]]; then
    GOEXPERIMENT=simd go test -c -fuzz=. -tags=cbc -o "${BIN_DIR}/format_cbc.test" ./format
fi
if [[ "${NEED_GOBLIN_BIN}" -eq 1 ]]; then
    GOEXPERIMENT=simd go test -c -fuzz=. -o "${BIN_DIR}/goblin.test" .
fi
if [[ "${NEED_ASSERT_BIN}" -eq 1 ]]; then
    GOEXPERIMENT=simd go test -c -fuzz=. -tags=goblin_assert -o "${BIN_DIR}/goblin_assert.test" .
fi
if [[ "${NEED_RACE_BIN}" -eq 1 ]]; then
    GOEXPERIMENT=simd go test -c -fuzz=. -race -o "${BIN_DIR}/goblin_race.test" .
fi
if [[ "${NEED_CBC_BIN}" -eq 1 ]]; then
    GOEXPERIMENT=simd go test -c -fuzz=. -tags=cbc -o "${BIN_DIR}/goblin_cbc.test" .
fi

GOCACHE_DIR="$(go env GOCACHE)"

read_stop_reason() {
    local state_file="$1"
    [[ -f "${state_file}" ]] || return 0
    awk '
        /"stop_reason":/ {
            sub(/^.*"stop_reason":[[:space:]]*"/, "")
            sub(/"[[:space:]]*,?[[:space:]]*$/, "")
            print
            exit
        }
    ' "${state_file}"
}

wait_one() {
    local done_pid=""
    if wait -n -p done_pid; then
        local t="${PID_TO_TARGET[${done_pid}]:-unknown}"
        local log_file="${LOG_DIR}/${t}.log"
        local state_file="${LOG_DIR}/${t}.state.json"
        if [[ "${FORMAL}" == "1" ]] && ! verify_formal_target_log "${t}" "${log_file}"; then
            printf "  [FAIL] %-42s (formal log audit failed, see %s)\n" "${t}" "${log_file}"
            FAILURES=$((FAILURES + 1))
            unset "PID_TO_TARGET[${done_pid}]"
            return
        fi
        local reason last
        reason="$(read_stop_reason "${state_file}")"
        last="$(grep 'fuzz: elapsed:' "${log_file}" | tail -n1 || true)"
        if [[ -n "${reason}" ]]; then
            printf "  [PASS] %-42s [%s] %s\n" "${t}" "${reason}" "${last}"
        else
            printf "  [PASS] %-42s %s\n" "${t}" "${last}"
        fi
        unset "PID_TO_TARGET[${done_pid}]"
    else
        local rc=$?
        local t="${PID_TO_TARGET[${done_pid}]:-unknown}"
        printf "  [FAIL] %-42s (rc=%d, see %s/%s.log)\n" "${t}" "${rc}" "${LOG_DIR}" "${t}"
        FAILURES=$((FAILURES + 1))
        unset "PID_TO_TARGET[${done_pid}]"
    fi
}

WATCH_COMMON_FLAGS=("--preset=${EARLY_STOP_PRESET}")
[[ -n "${MIN_TIME}" ]] && WATCH_COMMON_FLAGS+=("--min-time=${MIN_TIME}")
[[ -n "${MIN_EXECS}" ]] && WATCH_COMMON_FLAGS+=("--min-execs=${MIN_EXECS}")
[[ -n "${STALL_TIME}" ]] && WATCH_COMMON_FLAGS+=("--stall-time=${STALL_TIME}")
[[ -n "${STALL_EXECS}" ]] && WATCH_COMMON_FLAGS+=("--stall-execs=${STALL_EXECS}")
[[ -n "${WINDOW_TIME}" ]] && WATCH_COMMON_FLAGS+=("--window-time=${WINDOW_TIME}")
[[ -n "${WINDOW_EXECS}" ]] && WATCH_COMMON_FLAGS+=("--window-execs=${WINDOW_EXECS}")
[[ -n "${WINDOW_MIN_NEW}" ]] && WATCH_COMMON_FLAGS+=("--window-min-new=${WINDOW_MIN_NEW}")

for spec in "${JOBS[@]}"; do
    if [[ "${INTERRUPTED}" -ne 0 ]]; then
        break
    fi
    while [[ "${#PID_TO_TARGET[@]}" -ge "${CONCURRENCY}" ]]; do
        wait_one
    done
    IFS='|' read -r pkg target class dur <<< "${spec}"
    local_par="${WORKERS_PER_TARGET}"
    case "${pkg}" in
        ./assign)
            test_bin="${BIN_DIR}/assign.test"
            workdir="${GOBLIN_ROOT}/assign"
            pkg_import="github.com/paulourio/goblin/assign"
            ;;
        ./internal/simd)
            test_bin="${BIN_DIR}/simd.test"
            workdir="${GOBLIN_ROOT}/internal/simd"
            pkg_import="github.com/paulourio/goblin/internal/simd"
            ;;
        ./format)
            test_bin="${BIN_DIR}/format.test"
            workdir="${GOBLIN_ROOT}/format"
            pkg_import="github.com/paulourio/goblin/format"
            ;;
        ./format-cbc)
            test_bin="${BIN_DIR}/format_cbc.test"
            workdir="${GOBLIN_ROOT}/format"
            pkg_import="github.com/paulourio/goblin/format"
            ;;
        .)
            test_bin="${BIN_DIR}/goblin.test"
            workdir="${GOBLIN_ROOT}"
            pkg_import="github.com/paulourio/goblin"
            ;;
        .-assert)
            test_bin="${BIN_DIR}/goblin_assert.test"
            workdir="${GOBLIN_ROOT}"
            pkg_import="github.com/paulourio/goblin"
            ;;
        .-race)
            test_bin="${BIN_DIR}/goblin_race.test"
            workdir="${GOBLIN_ROOT}"
            pkg_import="github.com/paulourio/goblin"
            if [[ "${local_par}" -gt 4 ]]; then
                local_par=4
            fi
            ;;
        .-cbc)
            test_bin="${BIN_DIR}/goblin_cbc.test"
            workdir="${GOBLIN_ROOT}"
            pkg_import="github.com/paulourio/goblin"
            ;;
        *)
            echo "ERROR: unsupported package token ${pkg}" >&2
            exit 2
            ;;
    esac
    cache_dir="${GOCACHE_DIR}/fuzz/${pkg_import}"
    mkdir -p "${cache_dir}"
    echo "  [START] ${target} (${pkg}, class=${class}, -fuzztime=${dur}, -parallel=${local_par})"
    (
        cd "${workdir}"
        exec "${BIN_DIR}/goblin-fuzzwatch" \
            --target="${target}" \
            --class="${class}" \
            --log-file="${LOG_DIR}/${target}.log" \
            --state-file="${LOG_DIR}/${target}.state.json" \
            "${WATCH_COMMON_FLAGS[@]}" \
            -- \
            "${test_bin}" \
            -test.run='^$' \
            -test.fuzz="^${target}$" \
            -test.fuzztime="${dur}" \
            -test.parallel="${local_par}" \
            -test.fuzzcachedir="${cache_dir}"
    ) > "${LOG_DIR}/${target}.watch.log" 2>&1 &
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
