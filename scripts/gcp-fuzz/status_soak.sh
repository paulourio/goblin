#!/usr/bin/env bash
set -euo pipefail

# Inspect a running or completed goblin Cloud Batch fuzz soak and fetch any crashes.

RUN_ID="${1:?usage: $0 <run-id> [--download-crashes <dir>]}"

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

if [[ -z "${PROJECT}" ]]; then
    echo "error: GCP project is unset; set GCP_PROJECT or run 'gcloud config set project <id>'." >&2
    exit 2
fi
if [[ -z "${REGION}" ]]; then
    echo "error: GCP region is unset; set GCP_REGION or run 'gcloud config set compute/region <region>'." >&2
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

echo ""
echo "=== Per-Target Status (gs://${BUCKET}/runs/${RUN_ID}) ==="
for f in $(gcloud storage ls "gs://${BUCKET}/runs/${RUN_ID}/*/status.txt" --project="${PROJECT}" --quiet 2>/dev/null || true); do
    target="$(basename "$(dirname "${f}")")"
    st="$(gcloud storage cat "${f}" --project="${PROJECT}" --quiet 2>/dev/null | tr -d '[:space:]')"
    printf "  %-42s %s\n" "${target}" "${st}"
done

echo ""
echo "=== Discovered Crashes ==="
CRASHES="$(gcloud storage ls "gs://${BUCKET}/runs/${RUN_ID}/*/crashes/**" --project="${PROJECT}" --quiet 2>/dev/null || true)"
if [[ -z "${CRASHES}" ]]; then
    echo "  (none)"
else
    echo "${CRASHES}"
    if [[ "${2:-}" == "--download-crashes" && -n "${3:-}" ]]; then
        mkdir -p "${3}"
        gcloud storage cp -r "gs://${BUCKET}/runs/${RUN_ID}/*/crashes" "${3}/" --project="${PROJECT}" --quiet
        echo "Downloaded crashes into ${3}"
    fi
fi
