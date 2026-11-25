#!/usr/bin/env bash
set -euo pipefail

# Inspect a running or completed goblin Cloud Batch fuzz soak and fetch any crashes.

gcloud_cfg() {
    local val
    val="$(gcloud config get-value "$1" 2>/dev/null || true)"
    if [[ "${val}" == "(unset)" ]]; then
        val=""
    fi
    printf '%s' "${val}"
}

PROJECT="${GCP_PROJECT:-$(gcloud_cfg project)}"
REGION="${GCP_REGION:-$(gcloud_cfg compute/region)}"
REGION2="${GCP_REGION2:-}"
RUN_ID=""
DOWNLOAD_CRASHES_DIR=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --project=*) PROJECT="${1#*=}"; shift ;;
        --region=*) REGION="${1#*=}"; shift ;;
        --region2=*) REGION2="${1#*=}"; shift ;;
        --download-crashes=*) DOWNLOAD_CRASHES_DIR="${1#*=}"; shift ;;
        --download-crashes)
            shift
            DOWNLOAD_CRASHES_DIR="${1:?--download-crashes requires a directory argument}"
            shift
            ;;
        -h|--help)
            echo "usage: $0 <run-id> [--project=P] [--region=R] [--region2=R2] [--download-crashes <dir>]" >&2
            exit 0
            ;;
        -*)
            echo "error: unknown flag $1" >&2
            exit 2
            ;;
        *)
            if [[ -z "${RUN_ID}" ]]; then
                RUN_ID="$1"
            else
                echo "error: unexpected argument $1" >&2
                exit 2
            fi
            shift
            ;;
    esac
done

if [[ -z "${RUN_ID}" ]]; then
    echo "usage: $0 <run-id> [--project=P] [--region=R] [--region2=R2] [--download-crashes <dir>]" >&2
    exit 2
fi
if [[ -z "${PROJECT}" ]]; then
    echo "error: GCP project is unset; pass --project=<id>, set GCP_PROJECT, or run 'gcloud config set project <id>'." >&2
    exit 2
fi
if [[ -z "${REGION}" ]]; then
    echo "error: GCP region is unset; pass --region=<region>, set GCP_REGION, or run 'gcloud config set compute/region <region>'." >&2
    exit 2
fi

BUCKET="${GCS_BUCKET:-${PROJECT}-goblin-fuzz}"
JOB_NAME="goblin-fuzz-${RUN_ID}"

echo "=== Cloud Batch Job Status ==="
if gcloud batch jobs describe "${JOB_NAME}" --project="${PROJECT}" --location="${REGION}" --format="yaml(status.state,status.taskGroups.group0.counts,status.statusEvents)" --quiet 2>/dev/null; then
    true
else
    echo "--- Group A (${REGION}) ---"
    gcloud batch jobs describe "${JOB_NAME}-a" --project="${PROJECT}" --location="${REGION}" --format="yaml(status.state,status.taskGroups.group0.counts,status.statusEvents)" --quiet || true
    if [[ -n "${REGION2}" ]]; then
        echo "--- Group B (${REGION2}) ---"
        gcloud batch jobs describe "${JOB_NAME}-b" --project="${PROJECT}" --location="${REGION2}" --format="yaml(status.state,status.taskGroups.group0.counts,status.statusEvents)" --quiet || true
    fi
fi

parse_state_summary() {
    awk '
        /"cum_fuzz_sec":/ { sub(/^.*:[[:space:]]*/, ""); sub(/,[[:space:]]*$/, ""); sec=$0 }
        /"cum_execs":/ { sub(/^.*:[[:space:]]*/, ""); sub(/,[[:space:]]*$/, ""); ex=$0 }
        /"cum_new_interesting":/ { sub(/^.*:[[:space:]]*/, ""); sub(/,[[:space:]]*$/, ""); nw=$0 }
        /"stop_reason":/ { sub(/^.*:[[:space:]]*"/, ""); sub(/",[[:space:]]*$/, ""); sub(/"[[:space:]]*$/, ""); rsn=$0 }
        END {
            if (ex != "" || sec != "") {
                printf "fuzz=%ss execs=%s new=%s reason=%s", sec, ex, nw, rsn
            }
        }
    '
}

echo ""
echo "=== Per-Target Status (gs://${BUCKET}/runs/${RUN_ID}) ==="
for f in $(gcloud storage ls "gs://${BUCKET}/runs/${RUN_ID}/*/status.txt" --project="${PROJECT}" --quiet 2>/dev/null || true); do
    tdir="$(dirname "${f}")"
    target="$(basename "${tdir}")"
    st="$(gcloud storage cat "${f}" --project="${PROJECT}" --quiet 2>/dev/null | tr -d '[:space:]')"
    state_json="$(gcloud storage cat "${tdir}/fuzzwatch_state.json" --project="${PROJECT}" --quiet 2>/dev/null || true)"
    summary=""
    if [[ -n "${state_json}" ]]; then
        summary="$(printf '%s\n' "${state_json}" | parse_state_summary)"
    fi
    if [[ -n "${summary}" ]]; then
        printf "  %-42s %-20s %s\n" "${target}" "${st}" "${summary}"
    else
        printf "  %-42s %s\n" "${target}" "${st}"
    fi
done

echo ""
echo "=== Discovered Crashes ==="
CRASHES="$(gcloud storage ls "gs://${BUCKET}/runs/${RUN_ID}/*/crashes/**" --project="${PROJECT}" --quiet 2>/dev/null || true)"
if [[ -z "${CRASHES}" ]]; then
    echo "  (none)"
else
    echo "${CRASHES}"
    if [[ -n "${DOWNLOAD_CRASHES_DIR}" ]]; then
        crash_prefix="gs://${BUCKET}/runs/${RUN_ID}/"
        while IFS= read -r target_dir; do
            [[ -n "${target_dir}" ]] || continue
            mkdir -p "${DOWNLOAD_CRASHES_DIR}/${target_dir}"
            gcloud storage cp -r "${crash_prefix}${target_dir}/crashes/" "${DOWNLOAD_CRASHES_DIR}/${target_dir}/" --project="${PROJECT}" --quiet
        done < <(
            printf '%s\n' "${CRASHES}" | awk -v pfx="${crash_prefix}" '
                index($0, pfx) == 1 {
                    rest = substr($0, length(pfx) + 1)
                    slash = index(rest, "/crashes")
                    if (slash > 1) {
                        td = substr(rest, 1, slash - 1)
                        if (index(td, "/") == 0 && !seen[td]++) {
                            print td
                        }
                    }
                }
            '
        )
        echo "Downloaded crashes into ${DOWNLOAD_CRASHES_DIR}"
    fi
fi
