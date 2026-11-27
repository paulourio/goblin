#!/usr/bin/env bash
set -euo pipefail

# Replay GCS and repository fuzz corpora under GOEXPERIMENT=simd and GOBLIN_FORMAL=1,
# then aggregate statement coverage and cold blocks across solver packages.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

gcloud_project() {
    local val
    val="$(gcloud config get-value project 2>/dev/null || true)"
    if [[ "${val}" == "(unset)" ]]; then
        val=""
    fi
    printf '%s' "${val}"
}

default_jobs() {
    local cpus=1
    if command -v nproc >/dev/null 2>&1; then
        cpus="$(nproc 2>/dev/null || echo 1)"
    fi
    local j=$((cpus / 4))
    if [[ "${j}" -lt 1 ]]; then
        j=1
    elif [[ "${j}" -gt 6 ]]; then
        j=6
    fi
    printf '%s' "${j}"
}

PROJECT="${GCP_PROJECT:-$(gcloud_project)}"
BUCKET="${GCS_BUCKET:-${PROJECT:+${PROJECT}-goblin-fuzz}}"
SKIP_GCS=0
KEEP=0
LIST_TARGETS=0
WORK_DIR=""
JOBS="${CORPUS_AUDIT_JOBS:-$(default_jobs)}"
REPLAY_TIMEOUT="${CORPUS_AUDIT_TIMEOUT:-240m}"
GOAMD64_LEVEL="${GOAMD64:-}"
declare -a ONLY_TARGETS=()

usage() {
    cat >&2 <<'EOF'
usage: corpus_audit.sh [--skip-gcs] [--only=<target>] [--work-dir=<tmp>] [--keep] [--jobs=<n>] [--goamd64=v1|v2|v3|v4] [--list-targets]

Options:
  --skip-gcs         Skip GCS corpus sync and replay local/staged corpora only
  --only=<target>    Audit only the named fuzz target (repeatable or comma-separated)
  --work-dir=<tmp>   Use an explicit scratch directory outside the repository
  --keep             Preserve the scratch directory, staged corpora, logs, and profiles
  --jobs=<n>         Max concurrent target replay workers (default: min(6, max(1, nproc/4)))
  --goamd64=LEVEL    Set GOAMD64 microarchitecture level (v1, v2, v3, or v4)
  --list-targets     Print selected target names derived from targets.tsv and exit
  -h, --help         Show this help message
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --skip-gcs)
            SKIP_GCS=1
            shift
            ;;
        --only=*)
            IFS=',' read -r -a _parsed_only <<< "${1#*=}"
            ONLY_TARGETS+=("${_parsed_only[@]}")
            shift
            ;;
        --only)
            shift
            val="${1:?--only requires a target name}"
            IFS=',' read -r -a _parsed_only <<< "${val}"
            ONLY_TARGETS+=("${_parsed_only[@]}")
            shift
            ;;
        --work-dir=*)
            WORK_DIR="${1#*=}"
            shift
            ;;
        --work-dir)
            shift
            WORK_DIR="${1:?--work-dir requires a directory path}"
            shift
            ;;
        --jobs=*)
            JOBS="${1#*=}"
            shift
            ;;
        --jobs)
            shift
            JOBS="${1:?--jobs requires an integer count}"
            shift
            ;;
        --goamd64=*)
            GOAMD64_LEVEL="${1#*=}"
            shift
            ;;
        --goamd64)
            shift
            GOAMD64_LEVEL="${1:?--goamd64 requires v1, v2, v3, or v4}"
            shift
            ;;
        --keep)
            KEEP=1
            shift
            ;;
        --list-targets)
            LIST_TARGETS=1
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "error: unknown argument $1" >&2
            usage
            exit 2
            ;;
    esac
done

if ! [[ "${JOBS}" =~ ^[1-9][0-9]*$ ]]; then
    echo "error: --jobs must be a positive integer, got '${JOBS}'" >&2
    exit 2
fi

if [[ -n "${GOAMD64_LEVEL}" ]]; then
    case "${GOAMD64_LEVEL}" in
        v1|v2|v3|v4)
            export GOAMD64="${GOAMD64_LEVEL}"
            ;;
        *)
            echo "error: --goamd64 must be one of v1, v2, v3, or v4, got '${GOAMD64_LEVEL}'" >&2
            exit 2
            ;;
    esac
fi

# Discover all fuzz targets and their build/package metadata dynamically from targets.tsv.
TARGETS_TSV="${REPO_ROOT}/scripts/gcp-fuzz/targets.tsv"
if [[ ! -f "${TARGETS_TSV}" ]]; then
    echo "error: fuzz target manifest missing at ${TARGETS_TSV}" >&2
    exit 2
fi

declare -A TARGET_BIN=()
declare -A TARGET_WORKDIR=()
declare -A TARGET_CLASS=()
declare -a _RACE_TARGETS=()
declare -a _HEAVY_TARGETS=()
declare -a _PARSER_TARGETS=()
declare -a _FAST_TARGETS=()
declare -a _OTHER_TARGETS=()

while IFS=$'\t' read -r _idx _bin _workdir _target _class _maxpar _extra; do
    [[ -n "${_idx}" ]] || continue
    [[ "${_idx}" == \#* ]] && continue
    if [[ -z "${_bin}" || -z "${_workdir}" || -z "${_target}" || -z "${_class}" || -n "${_extra:-}" ]]; then
        echo "error: malformed row in ${TARGETS_TSV}: ${_idx}" >&2
        exit 2
    fi
    TARGET_BIN["${_target}"]="${_bin}"
    TARGET_WORKDIR["${_target}"]="${_workdir}"
    TARGET_CLASS["${_target}"]="${_class}"
    case "${_class}" in
        race) _RACE_TARGETS+=("${_target}") ;;
        heavy) _HEAVY_TARGETS+=("${_target}") ;;
        parser) _PARSER_TARGETS+=("${_target}") ;;
        fast) _FAST_TARGETS+=("${_target}") ;;
        *) _OTHER_TARGETS+=("${_target}") ;;
    esac
done < "${TARGETS_TSV}"

# Order race and heaviest targets first to minimize parallel tail latency.
ALL_TARGETS=(
    "${_RACE_TARGETS[@]}"
    "${_HEAVY_TARGETS[@]}"
    "${_PARSER_TARGETS[@]}"
    "${_FAST_TARGETS[@]}"
    "${_OTHER_TARGETS[@]}"
)

if [[ "${#ALL_TARGETS[@]}" -eq 0 ]]; then
    echo "error: no targets discovered in ${TARGETS_TSV}" >&2
    exit 2
fi

is_known_target() {
    local cand="$1"
    [[ -n "${TARGET_BIN[${cand}]:-}" ]]
}

declare -a SELECTED_TARGETS=()
if [[ "${#ONLY_TARGETS[@]}" -gt 0 ]]; then
    declare -A seen_only=()
    for t in "${ONLY_TARGETS[@]}"; do
        [[ -n "${t}" ]] || continue
        if ! is_known_target "${t}"; then
            echo "error: unknown fuzz target '${t}'" >&2
            exit 2
        fi
        if [[ -z "${seen_only[${t}]:-}" ]]; then
            seen_only["${t}"]=1
            SELECTED_TARGETS+=("${t}")
        fi
    done
    if [[ "${#SELECTED_TARGETS[@]}" -eq 0 ]]; then
        echo "error: --only did not specify any target" >&2
        exit 2
    fi
else
    SELECTED_TARGETS=("${ALL_TARGETS[@]}")
fi

if [[ "${LIST_TARGETS}" -eq 1 ]]; then
    printf '%s\n' "${SELECTED_TARGETS[@]}"
    exit 0
fi

if [[ "${SKIP_GCS}" -eq 0 ]]; then
    if ! command -v gcloud >/dev/null 2>&1; then
        echo "error: gcloud CLI is required unless --skip-gcs is set" >&2
        exit 2
    fi
    if [[ -z "${BUCKET}" ]]; then
        echo "error: GCS bucket could not be resolved; set GCP_PROJECT or GCS_BUCKET, run 'gcloud config set project <id>', or pass --skip-gcs" >&2
        exit 2
    fi
fi

if ! command -v lake >/dev/null 2>&1 && [[ -n "${HOME:-}" && -x "${HOME}/.elan/bin/lake" ]]; then
    export PATH="${HOME}/.elan/bin:${PATH}"
fi
if ! command -v lake >/dev/null 2>&1; then
    for bin_name in lp-check cuts-check rules-check vipr-check lsap-check; do
        if [[ ! -x "${REPO_ROOT}/formal/.lake/build/bin/${bin_name}" ]]; then
            echo "error: Lean 4 lake binary or prebuilt ${bin_name} is required for GOBLIN_FORMAL=1 replay" >&2
            exit 2
        fi
    done
fi

NEED_CBC=0
for t in "${SELECTED_TARGETS[@]}"; do
    if [[ "${TARGET_BIN[${t}]}" == *_cbc.test ]]; then
        NEED_CBC=1
        break
    fi
done
if [[ "${NEED_CBC}" -eq 1 && ! -d "${REPO_ROOT}/third_party/Cbc/dist/lib" ]]; then
    echo "error: CBC library directory missing at ${REPO_ROOT}/third_party/Cbc/dist/lib" >&2
    exit 2
fi

REPO_REAL="$(cd "${REPO_ROOT}" && pwd -P)"
AUTO_WORK_DIR=0
CREATED_WORK_DIR=0
SYNCED_GCS=0

if [[ -z "${WORK_DIR}" ]]; then
    WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/goblin-corpus-audit.XXXXXX")"
    AUTO_WORK_DIR=1
else
    if [[ ! -e "${WORK_DIR}" ]]; then
        CREATED_WORK_DIR=1
    fi
    mkdir -p "${WORK_DIR}"
fi
WORK_DIR="$(cd "${WORK_DIR}" && pwd -P)"

case "${WORK_DIR}" in
    "${REPO_REAL}"|"${REPO_REAL}"/*)
        echo "error: work-dir '${WORK_DIR}' must reside outside repository '${REPO_REAL}'" >&2
        exit 2
        ;;
esac

CORPUS_DIR="${WORK_DIR}/corpus"
STAGE_DIR="${WORK_DIR}/stage"
PROFILE_DIR="${WORK_DIR}/profiles"
LOG_DIR="${WORK_DIR}/logs"
declare -A PID_TO_TARGET=()

kill_tree() {
    local pid="$1"
    local child
    for child in $(pgrep -P "${pid}" 2>/dev/null || true); do
        kill_tree "${child}"
    done
    kill -TERM "${pid}" 2>/dev/null || true
}

cleanup() {
    local pid
    for pid in "${!PID_TO_TARGET[@]}"; do
        kill_tree "${pid}"
    done
    wait 2>/dev/null || true
    if [[ "${KEEP}" -eq 1 ]]; then
        return 0
    fi
    if [[ "${AUTO_WORK_DIR}" -eq 1 || "${CREATED_WORK_DIR}" -eq 1 ]]; then
        rm -rf "${WORK_DIR}"
    else
        rm -rf "${STAGE_DIR}" "${PROFILE_DIR}" "${LOG_DIR}"
        if [[ "${SYNCED_GCS}" -eq 1 ]]; then
            rm -rf "${CORPUS_DIR}"
        fi
    fi
}
trap cleanup EXIT INT TERM

mkdir -p "${CORPUS_DIR}" "${STAGE_DIR}" "${PROFILE_DIR}" "${LOG_DIR}"

if [[ "${SKIP_GCS}" -eq 0 ]]; then
    echo "=== Syncing GCS corpus from gs://${BUCKET}/corpus into ${CORPUS_DIR} ==="
    if [[ "${#ONLY_TARGETS[@]}" -gt 0 ]]; then
        for t in "${SELECTED_TARGETS[@]}"; do
            mkdir -p "${CORPUS_DIR}/${t}"
            if ! gcloud storage rsync -r "gs://${BUCKET}/corpus/${t}" "${CORPUS_DIR}/${t}" --project="${PROJECT}" --quiet >"${LOG_DIR}/gcs_rsync_${t}.log" 2>&1; then
                cat "${LOG_DIR}/gcs_rsync_${t}.log" >&2
                echo "error: failed to sync gs://${BUCKET}/corpus/${t}" >&2
                exit 1
            fi
        done
    else
        if ! gcloud storage rsync -r "gs://${BUCKET}/corpus" "${CORPUS_DIR}" --project="${PROJECT}" --quiet >"${LOG_DIR}/gcs_rsync.log" 2>&1; then
            cat "${LOG_DIR}/gcs_rsync.log" >&2
            echo "error: failed to sync gs://${BUCKET}/corpus" >&2
            exit 1
        fi
    fi
    SYNCED_GCS=1
    total_gcs_files="$(find "${CORPUS_DIR}" -type f | wc -l | tr -d '[:space:]')"
    echo "  Synced ${total_gcs_files} corpus seed files."
else
    echo "=== Skipping GCS sync (--skip-gcs) ==="
fi

echo "=== Staging isolated workspace at ${STAGE_DIR} ==="
tar -C "${REPO_ROOT}" \
    --exclude='.git' \
    --exclude='.cache' \
    --exclude='./third_party/Cbc/dist' \
    --exclude='./formal/.lake' \
    --exclude='./testdata/fuzz' \
    --exclude='./assign/testdata/fuzz' \
    --exclude='./format/testdata/fuzz' \
    --exclude='./internal/simd/testdata/fuzz' \
    -cf - . | tar -C "${STAGE_DIR}" -xf -

if [[ -e "${REPO_ROOT}/third_party/Cbc/dist" ]]; then
    mkdir -p "${STAGE_DIR}/third_party/Cbc"
    ln -sfn "$(cd "${REPO_ROOT}/third_party/Cbc/dist" && pwd -P)" "${STAGE_DIR}/third_party/Cbc/dist"
fi
if [[ -e "${REPO_ROOT}/formal/.lake" ]]; then
    mkdir -p "${STAGE_DIR}/formal"
    ln -sfn "$(cd "${REPO_ROOT}/formal/.lake" && pwd -P)" "${STAGE_DIR}/formal/.lake"
fi

# Pin native BLAS/OpenMP libraries to one thread per replay worker.
export OPENBLAS_NUM_THREADS=1
export OMP_NUM_THREADS=1
export MKL_NUM_THREADS=1
export NUMEXPR_NUM_THREADS=1
export VECLIB_MAXIMUM_THREADS=1

HOST_GOARCH="$(go env GOARCH)"
EFFECTIVE_GOAMD64="$(go env GOAMD64)"
if [[ -z "${EFFECTIVE_GOAMD64}" ]]; then
    EFFECTIVE_GOAMD64="n/a"
fi

echo "=== Replaying ${#SELECTED_TARGETS[@]} fuzz targets (jobs=${JOBS}, timeout=${REPLAY_TIMEOUT}) under GOARCH=${HOST_GOARCH} GOAMD64=${EFFECTIVE_GOAMD64} GOEXPERIMENT=simd GOBLIN_FORMAL=1 ==="
FAILURES=0
declare -a FAILED_TARGETS=()
declare -a COVER_PROFILES=()

stage_and_run_target() {
    local t="$1"
    local tsv_bin="${TARGET_BIN[${t}]}"
    local tsv_workdir="${TARGET_WORKDIR[${t}]}"
    local sub_pkg="${tsv_workdir#/app/work}"
    sub_pkg="${sub_pkg#/}"

    local pkg_arg="."
    local pkg_rel=""
    local coverpkg_arg="github.com/paulourio/goblin,github.com/paulourio/goblin/format,github.com/paulourio/goblin/internal/mad"
    if [[ -n "${sub_pkg}" ]]; then
        pkg_arg="./${sub_pkg}"
        pkg_rel="${sub_pkg}/"
        case "${sub_pkg}" in
            assign|format)
                coverpkg_arg="github.com/paulourio/goblin,github.com/paulourio/goblin/${sub_pkg}"
                ;;
            *)
                coverpkg_arg="github.com/paulourio/goblin/${sub_pkg}"
                ;;
        esac
    fi

    local -a extra_flags=()
    case "$(basename "${tsv_bin}")" in
        *_cbc.test)
            extra_flags+=("-tags=cbc")
            ;;
        *_assert.test)
            extra_flags+=("-tags=goblin_assert")
            ;;
        *_race.test)
            extra_flags+=("-race")
            ;;
    esac

    local stage_fuzz_dir="${STAGE_DIR}/${pkg_rel}testdata/fuzz/${t}"
    rm -rf "${stage_fuzz_dir}"
    mkdir -p "${stage_fuzz_dir}"
    if [[ -d "${REPO_ROOT}/${pkg_rel}testdata/fuzz/${t}" ]]; then
        find "${REPO_ROOT}/${pkg_rel}testdata/fuzz/${t}" -maxdepth 1 -type f -exec cp -f -t "${stage_fuzz_dir}" {} +
    fi
    if [[ -d "${CORPUS_DIR}/${t}" ]]; then
        find "${CORPUS_DIR}/${t}" -type f -exec cp -f -t "${stage_fuzz_dir}" {} +
    fi
    local seed_count
    seed_count="$(find "${stage_fuzz_dir}" -maxdepth 1 -type f | wc -l | tr -d '[:space:]')"

    local prof_file="${PROFILE_DIR}/${t}.cov"
    local log_file="${LOG_DIR}/${t}.log"
    local meta_file="${LOG_DIR}/${t}.meta"
    local start_ts
    start_ts="$(date +%s)"

    set +e
    (
        cd "${STAGE_DIR}"
        GOEXPERIMENT=simd GOBLIN_FORMAL=1 go test -count=1 \
            "-timeout=${REPLAY_TIMEOUT}" \
            ${extra_flags[@]+"${extra_flags[@]}"} \
            -run="^${t}$" \
            "-coverpkg=${coverpkg_arg}" \
            "-coverprofile=${prof_file}" \
            "${pkg_arg}"
    ) >"${log_file}" 2>&1
    local rc=$?
    set -e

    local elapsed=$(( $(date +%s) - start_ts ))
    printf '%s %s %s\n' "${seed_count}" "${elapsed}" "${stage_fuzz_dir}" >"${meta_file}"
    return "${rc}"
}

wait_one() {
    local done_pid=""
    local rc=0
    if wait -n -p done_pid; then
        rc=0
    else
        rc=$?
    fi
    local t="${PID_TO_TARGET[${done_pid}]:-unknown}"
    unset "PID_TO_TARGET[${done_pid}]"

    local meta_file="${LOG_DIR}/${t}.meta"
    local log_file="${LOG_DIR}/${t}.log"
    local prof_file="${PROFILE_DIR}/${t}.cov"
    local seed_count="0"
    local elapsed="0"
    local stage_fuzz_dir=""
    if [[ -f "${meta_file}" ]]; then
        read -r seed_count elapsed stage_fuzz_dir < "${meta_file}" || true
    fi

    if [[ "${rc}" -eq 0 ]] && grep -Eq 'no tests to run|no fuzz tests to fuzz' "${log_file}"; then
        rc=1
    fi
    if [[ "${rc}" -eq 0 ]] && grep -q "^--- SKIP: ${t}" "${log_file}"; then
        rc=1
    fi

    if [[ "${rc}" -eq 0 ]]; then
        printf '  [PASS] %-42s seeds=%-6s %ds\n' "${t}" "${seed_count}" "${elapsed}"
        if [[ -s "${prof_file}" ]]; then
            COVER_PROFILES+=("${prof_file}")
        fi
    else
        printf '  [FAIL] %-42s seeds=%-6s %ds (rc=%d)\n' "${t}" "${seed_count}" "${elapsed}" "${rc}" >&2
        cat "${log_file}" >&2
        FAILURES=$((FAILURES + 1))
        FAILED_TARGETS+=("${t}")
        if [[ -n "${stage_fuzz_dir}" ]]; then
            local failing_seeds
            failing_seeds="$(sed -n -E "s/^[[:space:]]*--- FAIL: (${t}\/[^[:space:]]+).*/\1/p" "${log_file}" || true)"
            if [[ -n "${failing_seeds}" ]]; then
                while IFS= read -r sub; do
                    local seed_name="${sub#"${t}/"}"
                    local seed_path="${stage_fuzz_dir}/${seed_name}"
                    if [[ -f "${seed_path}" ]]; then
                        printf '  --- Failing seed: %s (%s) ---\n' "${sub}" "${seed_path}" >&2
                        cat "${seed_path}" >&2
                        printf '\n  --- End seed ---\n' >&2
                    fi
                done <<< "${failing_seeds}"
            fi
        fi
    fi

    if [[ "${KEEP}" -eq 0 && -n "${stage_fuzz_dir}" ]]; then
        rm -rf "${stage_fuzz_dir}"
    fi
}

for t in "${SELECTED_TARGETS[@]}"; do
    while [[ "${#PID_TO_TARGET[@]}" -ge "${JOBS}" ]]; do
        wait_one
    done
    (
        trap - EXIT INT TERM
        stage_and_run_target "${t}"
    ) &
    PID_TO_TARGET[$!]="${t}"
done

while [[ "${#PID_TO_TARGET[@]}" -gt 0 ]]; do
    wait_one
done

if [[ "${#COVER_PROFILES[@]}" -gt 0 ]]; then
    echo ""
    python3 - "${REPO_ROOT}" "${PROFILE_DIR}/merged.cov" "${COVER_PROFILES[@]}" <<'PYEOF'
import pathlib
import re
import sys

repo_root = pathlib.Path(sys.argv[1])
merged_path = pathlib.Path(sys.argv[2])
profile_paths = [pathlib.Path(p) for p in sys.argv[3:]]

TARGET_FILES = [
    "simplex.go",
    "lu.go",
    "bab.go",
    "presolve.go",
    "cuts.go",
    "heuristics.go",
    "vipr_trace.go",
    "certified.go",
    "exact_polish.go",
    "cbc_precondition.go",
    "format/pulp_json_fast.go",
]
TARGET_SET = set(TARGET_FILES)
MODULE_PREFIX = "github.com/paulourio/goblin/"

LINE_RE = re.compile(r"^([^:]+):(\d+)\.(\d+),(\d+)\.(\d+)\s+(\d+)\s+(\d+)$")

# Aggregate coverage counts across all profiles using max execution count per block.
blocks: dict[tuple[str, int, int, int, int, int], int] = {}
for ppath in profile_paths:
    for line in ppath.read_text(encoding="utf-8").splitlines():
        if not line or line.startswith("mode:"):
            continue
        m = LINE_RE.match(line)
        if not m:
            continue
        full_file, sl, sc, el, ec, stmts, cnt = m.groups()
        if full_file.startswith(MODULE_PREFIX):
            rel = full_file[len(MODULE_PREFIX):]
        else:
            rel = full_file
        key = (rel, int(sl), int(sc), int(el), int(ec), int(stmts))
        cval = int(cnt)
        prev = blocks.get(key)
        if prev is None or cval > prev:
            blocks[key] = cval

with merged_path.open("w", encoding="utf-8") as fh:
    fh.write("mode: set\n")
    for (rel, sl, sc, el, ec, stmts), cnt in sorted(blocks.items()):
        c_out = 1 if cnt > 0 else 0
        fh.write(f"{MODULE_PREFIX}{rel}:{sl}.{sc},{el}.{ec} {stmts} {c_out}\n")

by_file: dict[str, list[tuple[int, int, int, int, int, int]]] = {f: [] for f in TARGET_FILES}
for (rel, sl, sc, el, ec, stmts), cnt in blocks.items():
    if rel in TARGET_SET:
        by_file[rel].append((sl, sc, el, ec, stmts, cnt))

func_cache: dict[str, list[str]] = {}
FUNC_RE = re.compile(r"^func\s+(?:\([^)]*\)\s*)?([A-Za-z0-9_]+)\s*\(")

def enclosing_func(rel: str, line_no: int) -> str:
    lines = func_cache.get(rel)
    if lines is None:
        src_path = repo_root / rel
        lines = src_path.read_text(encoding="utf-8").splitlines() if src_path.is_file() else []
        func_cache[rel] = lines
    idx = min(max(line_no - 1, 0), len(lines) - 1)
    while idx >= 0:
        m = FUNC_RE.match(lines[idx])
        if m:
            return m.group(1)
        idx -= 1
    return "<top>"

stats = []
tot_stmts = 0
tot_cov = 0
for rel in TARGET_FILES:
    entries = by_file[rel]
    total = sum(e[4] for e in entries)
    cov = sum(e[4] for e in entries if e[5] > 0)
    tot_stmts += total
    tot_cov += cov
    pct = (100.0 * cov / total) if total > 0 else -1.0
    stats.append((pct, total - cov, cov, total, rel))

stats.sort(key=lambda r: (r[0] if r[0] >= 0 else 101.0, -r[1], r[4]))

print("=== Ranked Statement Coverage Across Target Files ===")
print(f"{'Rank':<5} {'File':<28} {'Covered':>9} {'Total':>9} {'Uncov':>9} {'Cov %':>8}")
print("-" * 73)
for rank, (pct, uncov, cov, total, rel) in enumerate(stats, start=1):
    pct_str = f"{pct:6.2f}%" if pct >= 0 else "   N/A"
    print(f"{rank:<5} {rel:<28} {cov:>9} {total:>9} {uncov:>9} {pct_str:>8}")
print("-" * 73)
tot_pct = (100.0 * tot_cov / tot_stmts) if tot_stmts > 0 else 0.0
print(f"{'ALL':<5} {'(11 target files)':<28} {tot_cov:>9} {tot_stmts:>9} {tot_stmts - tot_cov:>9} {tot_pct:7.2f}%")

# Merge adjacent uncovered blocks unless a covered block lies strictly between them.
all_cold = []
cold_by_file: dict[str, list[tuple[int, int, int, str]]] = {}

for rel in TARGET_FILES:
    entries = sorted(by_file[rel])
    covered = [(sl, sc, el, ec) for (sl, sc, el, ec, _, cnt) in entries if cnt > 0]
    uncovered = [(sl, sc, el, ec, stmts) for (sl, sc, el, ec, stmts, cnt) in entries if cnt == 0 and stmts > 0]
    if not uncovered:
        continue
    merged = []
    cur_sl, cur_sc, cur_el, cur_ec, cur_stmts = uncovered[0]
    cov_idx = 0
    for sl, sc, el, ec, stmts in uncovered[1:]:
        while cov_idx < len(covered) and (covered[cov_idx][2], covered[cov_idx][3]) <= (cur_el, cur_ec):
            cov_idx += 1
        intervening = False
        if cov_idx < len(covered):
            c_sl, c_sc, c_el, c_ec = covered[cov_idx]
            if (c_sl, c_sc) > (cur_el, cur_ec) and (c_el, c_ec) < (sl, sc):
                intervening = True
        if not intervening and sl <= cur_el + 1:
            if (el, ec) > (cur_el, cur_ec):
                cur_el, cur_ec = el, ec
            cur_stmts += stmts
        else:
            fn = enclosing_func(rel, cur_sl)
            merged.append((cur_stmts, cur_sl, cur_el, fn))
            cur_sl, cur_sc, cur_el, cur_ec, cur_stmts = sl, sc, el, ec, stmts
    fn = enclosing_func(rel, cur_sl)
    merged.append((cur_stmts, cur_sl, cur_el, fn))
    merged.sort(key=lambda item: (-item[0], -(item[2] - item[1]), item[1]))
    cold_by_file[rel] = merged
    for stmts, sl, el, fn in merged:
        all_cold.append((stmts, el - sl + 1, rel, sl, el, fn))

all_cold.sort(key=lambda item: (-item[0], -item[1], item[2], item[3]))

print("")
print("=== Top 25 Contiguous Uncovered Line Blocks (Overall) ===")
print(f"{'Rank':<5} {'File':<28} {'Lines':<15} {'Stmts':>7} {'Function'}")
print("-" * 80)
for rank, (stmts, _, rel, sl, el, fn) in enumerate(all_cold[:25], start=1):
    span = f"L{sl}-L{el}"
    print(f"{rank:<5} {rel:<28} {span:<15} {stmts:>7} {fn}")

print("")
print("=== Top 5 Contiguous Uncovered Line Blocks Per File ===")
for rel in TARGET_FILES:
    blocks_f = cold_by_file.get(rel)
    if not blocks_f:
        continue
    print(f"  {rel}:")
    for stmts, sl, el, fn in blocks_f[:5]:
        print(f"    L{sl}-L{el} ({stmts} stmts) in {fn}")
PYEOF
fi

echo ""
if [[ "${KEEP}" -eq 1 ]]; then
    echo "Preserved audit artifacts under ${WORK_DIR}"
fi

if [[ "${FAILURES}" -gt 0 ]]; then
    echo "AUDIT FAILED: ${FAILURES} target(s) failed (${FAILED_TARGETS[*]})" >&2
    exit 1
fi

echo "AUDIT PASSED: ${#SELECTED_TARGETS[@]} target(s) verified clean."
exit 0
