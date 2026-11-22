#!/usr/bin/env bash
set -euo pipefail

# Preemption-resilient Cloud Batch fuzz worker for goblin.
# Maps BATCH_TASK_INDEX (0..18) via /app/targets.tsv, runs fuzzing in
# checkpointed slices on RAM disk (/dev/shm), and syncs corpus/crashes to GCS.

TASK_INDEX="${BATCH_TASK_INDEX:?BATCH_TASK_INDEX must be set}"
TASK_OFFSET="${TASK_OFFSET:-0}"
TARGET_INDEX=$((TASK_INDEX + TASK_OFFSET))
RUN_ID="${RUN_ID:?RUN_ID must be set}"
GCS_MOUNT="${GCS_MOUNT:-/mnt/disks/fuzz-gcs}"
SLICES="${SLICES:-4}"
SLICE_DURATION="${SLICE_DURATION:-15m}"
SYNC_INTERVAL_SEC="${SYNC_INTERVAL_SEC:-300}"

if [[ -n "${TARGET_LIST:-}" ]]; then
    IFS=',' read -r -a _TLIST <<< "${TARGET_LIST}"
    WANT_TARGET="${_TLIST[${TARGET_INDEX}]:-}"
    ROW="$(awk -F'\t' -v tgt="${WANT_TARGET}" '$4 == tgt { print; exit }' /app/targets.tsv)"
else
    ROW="$(awk -F'\t' -v idx="${TARGET_INDEX}" '$1 == idx { print; exit }' /app/targets.tsv)"
fi
if [[ -z "${ROW}" ]]; then
    echo "ERROR: no target found in /app/targets.tsv for TARGET_INDEX=${TARGET_INDEX} (TARGET_LIST=${TARGET_LIST:-})" >&2
    exit 2
fi

IFS=$'\t' read -r _ BIN SRC_WORKDIR TARGET PARALLEL <<< "${ROW}"
PARALLEL="${PARALLEL_OVERRIDE:-${PARALLEL}}"

GCS_TARGET_DIR="${GCS_MOUNT}/runs/${RUN_ID}/${TARGET}_t${TARGET_INDEX}"
# Shared persistent corpus across runs & parallel replicas of the same target.
GCS_CACHE_DIR="${GCS_MOUNT}/corpus/${TARGET}"
GCS_CRASH_DIR="${GCS_TARGET_DIR}/crashes"
GCS_LOG_DIR="${GCS_TARGET_DIR}/logs"
STATUS_FILE="${GCS_TARGET_DIR}/status.txt"
SLICES_FILE="${GCS_TARGET_DIR}/slices_done.txt"
CONTINUE_ON_CRASH="${CONTINUE_ON_CRASH:-0}"
CRASH_COUNT=0

mkdir -p "${GCS_CACHE_DIR}" "${GCS_LOG_DIR}"

# Idempotency guard: if a previous attempt already finished or recorded a crash, exit 0.
if [[ -f "${STATUS_FILE}" ]]; then
    PREV_STATUS="$(tr -d '[:space:]' < "${STATUS_FILE}")"
    if [[ "${PREV_STATUS}" == "DONE" || "${PREV_STATUS}" == "CRASHED" ]]; then
        echo "[worker ${TASK_INDEX}] ${TARGET} already ${PREV_STATUS}; exiting cleanly."
        exit 0
    fi
fi

# Print CPU/ISA verification (confirm AVX-512 on C3D/C4D/C3).
echo "[worker ${TASK_INDEX}] target=${TARGET} bin=${BIN} parallel=${PARALLEL} slices=${SLICES}x${SLICE_DURATION}"
echo "[worker ${TASK_INDEX}] host=$(hostname) nproc=$(nproc) avx512=$(grep -o 'avx512f' /proc/cpuinfo | head -n1 || echo none)"

# Set up fast RAM-disk workspace and fuzz cache.
LOCAL_BASE="/dev/shm/goblin-fuzz-${TASK_INDEX}"
LOCAL_WORK="${LOCAL_BASE}/work"
LOCAL_CACHE="${LOCAL_BASE}/cache"
LOCAL_LOG="${LOCAL_BASE}/worker.log"
rm -rf "${LOCAL_BASE}"
mkdir -p "${LOCAL_WORK}" "${LOCAL_CACHE}"
if [[ -d "${SRC_WORKDIR}/testdata" ]]; then
    cp -a "${SRC_WORKDIR}/testdata" "${LOCAL_WORK}/testdata"
fi

# Restore any previously checkpointed corpus from GCS.
if [[ -d "${GCS_CACHE_DIR}" ]]; then
    rsync -a "${GCS_CACHE_DIR}/" "${LOCAL_CACHE}/" || true
fi

SLICES_DONE=0
if [[ -f "${SLICES_FILE}" ]]; then
    SLICES_DONE="$(tr -d '[:space:]' < "${SLICES_FILE}" || echo 0)"
fi
echo "[worker ${TASK_INDEX}] resuming from slice ${SLICES_DONE}/${SLICES} (cached files: $(find "${LOCAL_CACHE}" -type f | wc -l))"

PREEMPTED=0
FUZZ_PID=""
SYNC_PID=""

sync_to_gcs() {
    rsync -a "${LOCAL_CACHE}/" "${GCS_CACHE_DIR}/" 2>/dev/null || true
    if [[ -f "${LOCAL_LOG}" ]]; then
        cp -f "${LOCAL_LOG}" "${GCS_LOG_DIR}/attempt-${BATCH_TASK_ATTEMPT:-0}.log" 2>/dev/null || true
    fi
}

on_terminate() {
    PREEMPTED=1
    echo "[worker ${TASK_INDEX}] received termination/preemption signal at $(date -u +%FT%TZ)" | tee -a "${LOCAL_LOG}"
    if [[ -n "${FUZZ_PID}" ]] && kill -0 "${FUZZ_PID}" 2>/dev/null; then
        kill -INT "${FUZZ_PID}" 2>/dev/null || true
        # Wait up to 8s for Go fuzzer to flush any in-flight crasher.
        for _ in 1 2 3 4 5 6 7 8; do
            kill -0 "${FUZZ_PID}" 2>/dev/null || break
            sleep 1
        done
        kill -TERM "${FUZZ_PID}" 2>/dev/null || true
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

cd "${LOCAL_WORK}"
mkdir -p "${LOCAL_WORK}/testdata/fuzz/${TARGET}"

for ((s = SLICES_DONE + 1; s <= SLICES; s++)); do
    echo "=== [slice ${s}/${SLICES}] starting ${TARGET} for ${SLICE_DURATION} at $(date -u +%FT%TZ) ===" | tee -a "${LOCAL_LOG}"

    # Snapshot existing seed files so we can detect newly written crashers.
    BEFORE_LIST="${LOCAL_BASE}/before_${s}.txt"
    find "${LOCAL_WORK}/testdata/fuzz/${TARGET}" -maxdepth 1 -type f -printf '%f\n' | sort > "${BEFORE_LIST}"

    set +e
    "${BIN}" \
        -test.run='^$' \
        -test.fuzz="^${TARGET}$" \
        -test.fuzztime="${SLICE_DURATION}" \
        -test.parallel="${PARALLEL}" \
        -test.fuzzcachedir="${LOCAL_CACHE}" \
        >> "${LOCAL_LOG}" 2>&1 &
    FUZZ_PID=$!
    wait "${FUZZ_PID}"
    RC=$?
    FUZZ_PID=""
    set -e

    if [[ "${PREEMPTED}" -eq 1 ]]; then
        exit 143
    fi

    if [[ "${RC}" -ne 0 ]]; then
        CRASH_COUNT=$((CRASH_COUNT + 1))
        echo "!!! CRASH #${CRASH_COUNT} DETECTED in ${TARGET} (slice ${s}/${SLICES}, rc=${RC}) at $(date -u +%FT%TZ) !!!" | tee -a "${LOCAL_LOG}"
        CRASH_SUB="${GCS_CRASH_DIR}/crash_${CRASH_COUNT}_s${s}"
        mkdir -p "${CRASH_SUB}"
        rsync -a "${LOCAL_WORK}/testdata/fuzz/${TARGET}/" "${CRASH_SUB}/testdata_fuzz_${TARGET}/" || true
        cp -f "${LOCAL_LOG}" "${CRASH_SUB}/crash.log" || true
        sync_to_gcs
        if [[ "${CONTINUE_ON_CRASH}" -eq 1 ]]; then
            # Remove newly written crasher file(s) locally so subsequent slices keep fuzzing
            # instead of immediately failing on the same seed.
            AFTER_LIST="${LOCAL_BASE}/after_${s}.txt"
            find "${LOCAL_WORK}/testdata/fuzz/${TARGET}" -maxdepth 1 -type f -printf '%f\n' | sort > "${AFTER_LIST}"
            while IFS= read -r newf; do
                [[ -n "${newf}" ]] && rm -f "${LOCAL_WORK}/testdata/fuzz/${TARGET}/${newf}"
            done < <(comm -13 "${BEFORE_LIST}" "${AFTER_LIST}")
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
