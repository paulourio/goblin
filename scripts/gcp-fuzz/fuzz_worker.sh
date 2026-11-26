#!/usr/bin/env bash
set -euo pipefail

# Preemption-resilient Cloud Batch fuzz worker for goblin.
# Maps BATCH_TASK_INDEX (0..47) via /app/targets.tsv, runs fuzzing in
# checkpointed slices on RAM disk (/dev/shm) under goblin-fuzzwatch,
# and syncs corpus, crashes, logs, and saturation state to GCS.

TASK_INDEX="${BATCH_TASK_INDEX:?BATCH_TASK_INDEX must be set}"
TASK_OFFSET="${TASK_OFFSET:-0}"
TARGET_INDEX=$((TASK_INDEX + TASK_OFFSET))
RUN_ID="${RUN_ID:?RUN_ID must be set}"
GCS_MOUNT="${GCS_MOUNT:-/mnt/disks/fuzz-gcs}"
SLICES="${SLICES:-4}"
SLICE_DURATION="${SLICE_DURATION:-15m}"
SYNC_INTERVAL_SEC="${SYNC_INTERVAL_SEC:-300}"
EARLY_STOP_PRESET="${EARLY_STOP_PRESET:-soak}"
FORMAL="${GOBLIN_FORMAL:-0}"
SIMD_MAX_TIER="${GOBLIN_SIMD_MAX_TIER:-}"

for arg in "$@"; do
    case "${arg}" in
        --formal|--formal=1|--formal=true) FORMAL="1" ;;
        --formal=0|--formal=false) FORMAL="0" ;;
        --simd-max-tier=*) SIMD_MAX_TIER="${arg#*=}" ;;
        *)
            echo "ERROR: unknown argument ${arg}" >&2
            exit 2
            ;;
    esac
done

if [[ -n "${SIMD_MAX_TIER}" ]]; then
    case "${SIMD_MAX_TIER}" in
        auto|scalar|avx2|avx512) export GOBLIN_SIMD_MAX_TIER="${SIMD_MAX_TIER}" ;;
        *)
            echo "ERROR: invalid --simd-max-tier=${SIMD_MAX_TIER} (expected auto, scalar, avx2, or avx512)" >&2
            exit 2
            ;;
    esac
fi
if [[ "${FORMAL}" == "1" ]]; then
    export GOBLIN_FORMAL=1
fi

# Constrain native BLAS/OpenMP libraries to 1 thread per fuzz process.
export OPENBLAS_NUM_THREADS=1
export OMP_NUM_THREADS=1
export MKL_NUM_THREADS=1
export NUMEXPR_NUM_THREADS=1
export VECLIB_MAXIMUM_THREADS=1

if [[ -n "${TARGET_LIST:-}" ]]; then
    IFS=',' read -r -a _TLIST <<< "${TARGET_LIST}"
    WANT_TARGET="${_TLIST[${TARGET_INDEX}]:-}"
    ROW="$(awk -F'\t' -v tgt="${WANT_TARGET}" '$0 !~ /^#/ && $4 == tgt { print; exit }' /app/targets.tsv)"
else
    ROW="$(awk -F'\t' -v idx="${TARGET_INDEX}" '$0 !~ /^#/ && $1 == idx { print; exit }' /app/targets.tsv)"
fi
if [[ -z "${ROW}" ]]; then
    echo "ERROR: no target found in /app/targets.tsv for TARGET_INDEX=${TARGET_INDEX} (TARGET_LIST=${TARGET_LIST:-})" >&2
    exit 2
fi

IFS=$'\t' read -r _ BIN SRC_WORKDIR TARGET TARGET_CLASS MAX_PARALLEL <<< "${ROW}"

# Read real host core count without allowing OMP_NUM_THREADS=1 to clamp nproc.
HOST_CPUS="$(env -u OMP_NUM_THREADS -u OMP_THREAD_LIMIT nproc)"
if [[ "${BIN}" == *_cbc.test ]] && [[ "${HOST_CPUS}" -ge 10 ]]; then
    RESERVE=4
elif [[ "${HOST_CPUS}" -ge 6 ]]; then
    RESERVE=2
elif [[ "${HOST_CPUS}" -ge 2 ]]; then
    RESERVE=1
else
    RESERVE=0
fi
AUTO_PAR=$((HOST_CPUS - RESERVE))
if [[ "${AUTO_PAR}" -lt 1 ]]; then
    AUTO_PAR=1
fi

detect_container_mem_mib() {
    local mem_mib=0
    local mem_bytes=""
    if [[ -r /sys/fs/cgroup/memory.max ]]; then
        mem_bytes="$(tr -d '[:space:]' < /sys/fs/cgroup/memory.max 2>/dev/null || true)"
        if [[ "${mem_bytes}" =~ ^[0-9]+$ && "${mem_bytes}" -gt 0 && "${mem_bytes}" -lt 1125899906842624 ]]; then
            mem_mib="$((mem_bytes / 1048576))"
        fi
    elif [[ -r /sys/fs/cgroup/memory/memory.limit_in_bytes ]]; then
        mem_bytes="$(tr -d '[:space:]' < /sys/fs/cgroup/memory/memory.limit_in_bytes 2>/dev/null || true)"
        if [[ "${mem_bytes}" =~ ^[0-9]+$ && "${mem_bytes}" -gt 0 && "${mem_bytes}" -lt 1125899906842624 ]]; then
            mem_mib="$((mem_bytes / 1048576))"
        fi
    fi
    local mem_kib=""
    mem_kib="$(awk '/^MemTotal:/ {print $2; exit}' /proc/meminfo 2>/dev/null || true)"
    if [[ "${mem_kib}" =~ ^[0-9]+$ && "${mem_kib}" -gt 0 ]]; then
        local host_mib="$((mem_kib / 1024))"
        if [[ "${mem_mib}" -le 0 || "${host_mib}" -lt "${mem_mib}" ]]; then
            mem_mib="${host_mib}"
        fi
    fi
    echo "${mem_mib}"
}

CONTAINER_MEM_MIB="$(detect_container_mem_mib)"
if [[ -z "${PARALLEL_OVERRIDE:-}" && "${CONTAINER_MEM_MIB}" -gt 0 ]]; then
    if [[ "${TARGET}" == "FuzzMADMediumDecomposedCBC" ]]; then
        PER_WORKER_MIB=4608
    elif [[ "${BIN}" == *_cbc.test || "${BIN}" == *goblin_race.test ]]; then
        PER_WORKER_MIB=2560
    else
        PER_WORKER_MIB=768
    fi
    MEM_PAR=$((CONTAINER_MEM_MIB / PER_WORKER_MIB))
    if [[ "${MEM_PAR}" -lt 1 ]]; then
        MEM_PAR=1
    fi
    if [[ "${AUTO_PAR}" -gt "${MEM_PAR}" ]]; then
        AUTO_PAR="${MEM_PAR}"
    fi
fi

if [[ "${AUTO_PAR}" -gt "${MAX_PARALLEL}" ]]; then
    AUTO_PAR="${MAX_PARALLEL}"
fi
PARALLEL="${PARALLEL_OVERRIDE:-${AUTO_PAR}}"

configure_go_mem_env() {
    local go_mem_pct=80
    if [[ "${BIN}" == *_cbc.test ]]; then
        export GOGC="${GOGC:-50}"
        go_mem_pct=50
    fi
    local per_proc_mib=256
    if [[ "${CONTAINER_MEM_MIB}" -gt 0 ]]; then
        per_proc_mib=$(( (CONTAINER_MEM_MIB * go_mem_pct / 100) / (PARALLEL + 1) ))
        if [[ "${per_proc_mib}" -lt 256 ]]; then
            per_proc_mib=256
        fi
    fi
    export GOMEMLIMIT="${per_proc_mib}MiB"
}
configure_go_mem_env

GCS_TARGET_DIR="${GCS_MOUNT}/runs/${RUN_ID}/${TARGET}_t${TARGET_INDEX}"
# Shared persistent corpus across runs and parallel replicas of the same target.
GCS_CACHE_DIR="${GCS_MOUNT}/corpus/${TARGET}"
GCS_CRASH_DIR="${GCS_TARGET_DIR}/crashes"
GCS_LOG_DIR="${GCS_TARGET_DIR}/logs"
GCS_STATE_FILE="${GCS_TARGET_DIR}/fuzzwatch_state.json"
STATUS_FILE="${GCS_TARGET_DIR}/status.txt"
SLICES_FILE="${GCS_TARGET_DIR}/slices_done.txt"
CONTINUE_ON_CRASH="${CONTINUE_ON_CRASH:-0}"
CRASH_COUNT=0

mkdir -p "${GCS_CACHE_DIR}" "${GCS_LOG_DIR}"

# Idempotency guard: exit cleanly if a prior attempt reached a terminal state.
if [[ -f "${STATUS_FILE}" ]]; then
    PREV_STATUS="$(tr -d '[:space:]' < "${STATUS_FILE}")"
    if [[ "${PREV_STATUS}" == DONE* || "${PREV_STATUS}" == "CRASHED" ]]; then
        echo "[worker ${TASK_INDEX}] ${TARGET} already ${PREV_STATUS}; exiting cleanly."
        exit 0
    fi
fi

CPU_MODEL="$(awk -F': ' '/model name/{print $2; exit}' /proc/cpuinfo || true)"
HAS_AVX2="$(grep -o 'avx2' /proc/cpuinfo | head -n1 || echo none)"
HAS_AVX512="$(grep -o 'avx512f' /proc/cpuinfo | head -n1 || echo none)"
echo "[worker ${TASK_INDEX}] target=${TARGET} class=${TARGET_CLASS} bin=${BIN} parallel=${PARALLEL}/${MAX_PARALLEL} slices=${SLICES}x${SLICE_DURATION} early_stop=${EARLY_STOP_PRESET} formal=${FORMAL} simd_max_tier=${SIMD_MAX_TIER:-auto}"
echo "[worker ${TASK_INDEX}] host=$(hostname) nproc=${HOST_CPUS} mem_mib=${CONTAINER_MEM_MIB} gomemlimit=${GOMEMLIMIT} gogc=${GOGC:-default} avx2=${HAS_AVX2} avx512=${HAS_AVX512} cpu=${CPU_MODEL:-unknown}"

# Set up fast RAM-disk workspace and fuzz cache.
LOCAL_BASE="/dev/shm/goblin-fuzz-${TASK_INDEX}"
LOCAL_WORK="${LOCAL_BASE}/work"
LOCAL_CACHE="${LOCAL_BASE}/cache"
LOCAL_LOG="${LOCAL_BASE}/worker.log"
LOCAL_STATE="${LOCAL_BASE}/fuzzwatch_state.json"
rm -rf "${LOCAL_BASE}"
mkdir -p "${LOCAL_WORK}" "${LOCAL_CACHE}"
if [[ -d "${SRC_WORKDIR}/testdata" ]]; then
    cp -a "${SRC_WORKDIR}/testdata" "${LOCAL_WORK}/testdata"
fi
if [[ "${FORMAL}" == "1" ]]; then
    if [[ -f /app/formal/lakefile.toml && -d /app/formal/.lake/build/bin ]] && \
        find /app/formal/.lake/build/bin -maxdepth 1 -type f -executable | grep -q .; then
        ln -sfn /app/formal "${LOCAL_WORK}/formal"
    else
        echo "[worker ${TASK_INDEX}] WARNING: GOBLIN_FORMAL=1 enabled, but Lean ELFs are absent under /app/formal/.lake/build/bin" >&2
    fi
fi

# Restore previously checkpointed corpus and saturation state from GCS.
if [[ -d "${GCS_CACHE_DIR}" ]]; then
    rsync -a "${GCS_CACHE_DIR}/" "${LOCAL_CACHE}/" || true
fi
if [[ -f "${GCS_STATE_FILE}" ]]; then
    cp -f "${GCS_STATE_FILE}" "${LOCAL_STATE}" || true
fi

SLICES_DONE=0
if [[ -f "${SLICES_FILE}" ]]; then
    SLICES_DONE="$(tr -d '[:space:]' < "${SLICES_FILE}" || echo 0)"
fi
echo "[worker ${TASK_INDEX}] resuming from slice ${SLICES_DONE}/${SLICES} (cached files: $(find "${LOCAL_CACHE}" -type f | wc -l))"

PREEMPTED=0
WATCH_PID=""
SYNC_PID=""

read_json_field() {
    local key="$1"
    local file="$2"
    [[ -f "${file}" ]] || return 0
    awk -v k="\"${key}\":" '
        index($0, k) {
            sub("^.*" k "[[:space:]]*\"", "")
            sub("\"[[:space:]]*,?[[:space:]]*$", "")
            print
            exit
        }
    ' "${file}"
}

write_crash_signature() {
    local sig_file="$1"
    local repro_log="$2"
    local target="$3"
    local slice_num="$4"
    local rc="$5"
    local seeds_csv="$6"
    {
        printf 'target=%s\n' "${target}"
        printf 'slice=%s\n' "${slice_num}"
        printf 'rc=%s\n' "${rc}"
        printf 'seeds=%s\n' "${seeds_csv}"
        if [[ -s "${repro_log}" ]]; then
            grep -Em 8 '^(panic:|fatal error:|WARNING: DATA RACE|[[:space:]]*--- FAIL:|FAIL\b)|_test\.go:[0-9]+:|github\.com/paulourio/goblin' "${repro_log}" || true
        fi
    } > "${sig_file}"
}

sync_to_gcs() {
    rsync -a "${LOCAL_CACHE}/" "${GCS_CACHE_DIR}/" 2>/dev/null || true
    if [[ -f "${LOCAL_STATE}" ]]; then
        cp -f "${LOCAL_STATE}" "${GCS_STATE_FILE}" 2>/dev/null || true
    fi
    if [[ -f "${LOCAL_LOG}" ]]; then
        cp -f "${LOCAL_LOG}" "${GCS_LOG_DIR}/attempt-${BATCH_TASK_ATTEMPT:-0}.log" 2>/dev/null || true
    fi
}

on_terminate() {
    PREEMPTED=1
    echo "[worker ${TASK_INDEX}] received termination/preemption signal at $(date -u +%FT%TZ)" | tee -a "${LOCAL_LOG}"
    if [[ -n "${WATCH_PID}" ]] && kill -0 "${WATCH_PID}" 2>/dev/null; then
        kill -TERM "${WATCH_PID}" 2>/dev/null || true
        # Wait up to 28s for goblin-fuzzwatch to SIGINT Go, flush corpus, and save state.
        for _ in $(seq 1 28); do
            kill -0 "${WATCH_PID}" 2>/dev/null || break
            sleep 1
        done
        kill -KILL "${WATCH_PID}" 2>/dev/null || true
    fi
    if [[ -n "${SYNC_PID}" ]]; then
        kill "${SYNC_PID}" 2>/dev/null || true
    fi
    sync_to_gcs
    echo "PREEMPTED_AT_SLICE_${SLICES_DONE}" > "${STATUS_FILE}" || true
    exit 143
}

trap on_terminate TERM INT

# Periodic background checkpoint loop.
(
    while true; do
        sleep "${SYNC_INTERVAL_SEC}"
        sync_to_gcs
    done
) &
SYNC_PID=$!

cleanup_bg() {
    if [[ -n "${SYNC_PID}" ]]; then
        kill "${SYNC_PID}" 2>/dev/null || true
    fi
}
trap cleanup_bg EXIT

WATCH_ARGS=(
    "--target=${TARGET}"
    "--preset=${EARLY_STOP_PRESET}"
    "--class=${TARGET_CLASS}"
    "--log-file=${LOCAL_LOG}"
    "--append-log"
    "--state-file=${LOCAL_STATE}"
)
[[ -n "${MIN_TIME:-}" ]] && WATCH_ARGS+=("--min-time=${MIN_TIME}")
[[ -n "${MIN_EXECS:-}" ]] && WATCH_ARGS+=("--min-execs=${MIN_EXECS}")
[[ -n "${STALL_TIME:-}" ]] && WATCH_ARGS+=("--stall-time=${STALL_TIME}")
[[ -n "${STALL_EXECS:-}" ]] && WATCH_ARGS+=("--stall-execs=${STALL_EXECS}")
[[ -n "${WINDOW_TIME:-}" ]] && WATCH_ARGS+=("--window-time=${WINDOW_TIME}")
[[ -n "${WINDOW_EXECS:-}" ]] && WATCH_ARGS+=("--window-execs=${WINDOW_EXECS}")
[[ -n "${WINDOW_MIN_NEW:-}" ]] && WATCH_ARGS+=("--window-min-new=${WINDOW_MIN_NEW}")

cd "${LOCAL_WORK}"
mkdir -p "${LOCAL_WORK}/testdata/fuzz/${TARGET}"
OOM_RETRIES=0

for ((s = SLICES_DONE + 1; s <= SLICES; s++)); do
    echo "=== [slice ${s}/${SLICES}] starting ${TARGET} for ${SLICE_DURATION} at $(date -u +%FT%TZ) ===" | tee -a "${LOCAL_LOG}"

    # Snapshot existing seed files so newly written crashers can be detected.
    BEFORE_LIST="${LOCAL_BASE}/before_${s}.txt"
    find "${LOCAL_WORK}/testdata/fuzz/${TARGET}" -maxdepth 1 -type f -printf '%f\n' | sort > "${BEFORE_LIST}"
    LOG_START_LINE=$(( $(wc -l < "${LOCAL_LOG}" 2>/dev/null || echo 0) + 1 ))

    set +e
    /app/bin/goblin-fuzzwatch "${WATCH_ARGS[@]}" -- \
        "${BIN}" \
        -test.run='^$' \
        -test.fuzz="^${TARGET}$" \
        -test.fuzztime="${SLICE_DURATION}" \
        -test.parallel="${PARALLEL}" \
        -test.fuzzcachedir="${LOCAL_CACHE}" \
        >> "${LOCAL_LOG}" 2>&1 &
    WATCH_PID=$!
    wait "${WATCH_PID}"
    RC=$?
    WATCH_PID=""
    set -e

    if [[ "${PREEMPTED}" -eq 1 ]]; then
        exit 143
    fi

    if [[ "${RC}" -ne 0 ]]; then
        AFTER_LIST="${LOCAL_BASE}/after_${s}.txt"
        find "${LOCAL_WORK}/testdata/fuzz/${TARGET}" -maxdepth 1 -type f -printf '%f\n' | sort > "${AFTER_LIST}"
        NEW_CRASH_FILES="$(comm -13 "${BEFORE_LIST}" "${AFTER_LIST}")"
        SLICE_OUT="$(tail -n +"${LOG_START_LINE}" "${LOCAL_LOG}" 2>/dev/null || true)"
        OOM_RE='no crash will be recorded: signal: killed|fuzzing process terminated by unexpected signal|fatal error: runtime: out of memory|cannot allocate memory'
        if [[ -z "${NEW_CRASH_FILES}" ]] && printf '%s\n' "${SLICE_OUT}" | grep -Eq "${OOM_RE}"; then
            if [[ "${PARALLEL}" -gt 1 && "${OOM_RETRIES}" -lt 3 ]]; then
                OOM_RETRIES=$((OOM_RETRIES + 1))
                PARALLEL=$((PARALLEL / 2))
                if [[ "${PARALLEL}" -lt 1 ]]; then
                    PARALLEL=1
                fi
                configure_go_mem_env
                echo "[worker ${TASK_INDEX}] WARNING: OOM/signal-kill detected without crasher in slice ${s}/${SLICES}; stepping down parallel=${PARALLEL}, GOMEMLIMIT=${GOMEMLIMIT}, retrying slice ${s}" | tee -a "${LOCAL_LOG}"
                sync_to_gcs
                s=$((s - 1))
                continue
            fi
            echo "!!! FATAL: OOM/signal-kill in ${TARGET} at slice ${s}/${SLICES} (parallel=${PARALLEL}, retries=${OOM_RETRIES}) !!!" | tee -a "${LOCAL_LOG}"
            echo "FAILED_OOM" > "${STATUS_FILE}"
            sync_to_gcs
            exit 137
        fi

        CRASH_COUNT=$((CRASH_COUNT + 1))
        echo "!!! CRASH #${CRASH_COUNT} DETECTED in ${TARGET} (slice ${s}/${SLICES}, rc=${RC}) at $(date -u +%FT%TZ) !!!" | tee -a "${LOCAL_LOG}"
        CRASH_SUB="${GCS_CRASH_DIR}/crash_${CRASH_COUNT}_s${s}"
        mkdir -p "${CRASH_SUB}"
        if [[ -n "${NEW_CRASH_FILES}" ]]; then
            mkdir -p "${CRASH_SUB}/testdata_fuzz_${TARGET}"
            REPRO_LOG="${CRASH_SUB}/crash_repro.log"
            : > "${REPRO_LOG}"
            SEEDS_CSV=""
            while IFS= read -r seed_name; do
                [[ -z "${seed_name}" ]] && continue
                cp -a "${LOCAL_WORK}/testdata/fuzz/${TARGET}/${seed_name}" "${CRASH_SUB}/testdata_fuzz_${TARGET}/" || true
                SEEDS_CSV="${SEEDS_CSV:+${SEEDS_CSV},}${seed_name}"
                printf '=== Repro %s/%s ===\n' "${TARGET}" "${seed_name}" >> "${REPRO_LOG}"
                set +e
                if command -v timeout >/dev/null 2>&1; then
                    timeout 120s "${BIN}" -test.run="^${TARGET}/${seed_name}$" -test.v >> "${REPRO_LOG}" 2>&1
                else
                    "${BIN}" -test.run="^${TARGET}/${seed_name}$" -test.v >> "${REPRO_LOG}" 2>&1
                fi
                set -e
            done <<< "${NEW_CRASH_FILES}"
            write_crash_signature "${CRASH_SUB}/signature.txt" "${REPRO_LOG}" "${TARGET}" "${s}" "${RC}" "${SEEDS_CSV}"
        fi
        cp -f "${LOCAL_LOG}" "${CRASH_SUB}/crash.log" || true
        sync_to_gcs
        if [[ "${CONTINUE_ON_CRASH}" -eq 1 ]]; then
            # Remove local crasher file(s) so subsequent slices keep fuzzing past this seed.
            if [[ -n "${NEW_CRASH_FILES}" ]]; then
                while IFS= read -r newf; do
                    [[ -n "${newf}" ]] && rm -f "${LOCAL_WORK}/testdata/fuzz/${TARGET}/${newf}"
                done <<< "${NEW_CRASH_FILES}"
            fi
            echo "CRASHED_${CRASH_COUNT}" > "${STATUS_FILE}"
            SLICES_DONE="${s}"
            echo "${SLICES_DONE}" > "${SLICES_FILE}"
            continue
        fi
        echo "CRASHED" > "${STATUS_FILE}"
        # Exit 0 so Cloud Batch stops immediately without burning retries on a real bug.
        exit 0
    fi

    SLICES_DONE="${s}"
    echo "${SLICES_DONE}" > "${SLICES_FILE}"

    STOP_REASON="$(read_json_field stop_reason "${LOCAL_STATE}")"
    STOP_DETAIL="$(read_json_field stop_detail "${LOCAL_STATE}")"
    if [[ "${STOP_REASON}" == saturated_* ]]; then
        if [[ "${CRASH_COUNT}" -gt 0 ]]; then
            echo "DONE_SATURATED_WITH_${CRASH_COUNT}_CRASHES" > "${STATUS_FILE}"
        else
            echo "DONE_SATURATED" > "${STATUS_FILE}"
        fi
        sync_to_gcs
        echo "=== [slice ${s}/${SLICES}] ${TARGET} stopped early (${STOP_REASON}: ${STOP_DETAIL}) ===" | tee -a "${LOCAL_LOG}"
        exit 0
    fi

    sync_to_gcs
    echo "=== [slice ${s}/${SLICES}] finished ${TARGET} (cached files: $(find "${LOCAL_CACHE}" -type f | wc -l)) ===" | tee -a "${LOCAL_LOG}"
done

if [[ "${CRASH_COUNT}" -gt 0 ]]; then
    echo "DONE_WITH_${CRASH_COUNT}_CRASHES" > "${STATUS_FILE}"
else
    echo "DONE" > "${STATUS_FILE}"
fi
sync_to_gcs
echo "[worker ${TASK_INDEX}] ${TARGET} completed all ${SLICES} slices."
exit 0
