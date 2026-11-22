#!/usr/bin/env bash
set -euo pipefail

# Build, push, and launch a Cloud Batch Spot VM fuzz soak for goblin.
# Adheres to explicit --project, --quiet, and idempotent GCP resource setup.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GOBLIN_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

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
AR_REPO="${AR_REPO:-goblin-fuzz}"
MACHINE_TYPE="${MACHINE_TYPE:-c3d-highcpu-16}"
PARALLELISM="${PARALLELISM:-9}"
SLICES="${SLICES:-4}"
SLICE_DURATION="${SLICE_DURATION:-15m}"
MAX_RUN_DURATION="${MAX_RUN_DURATION:-4500s}"
MAX_RETRY_COUNT="${MAX_RETRY_COUNT:-2}"
RUN_ID="${RUN_ID:-soak-$(date -u +%Y%m%d-%H%M%S)}"
IMAGE_TAG=""
TARGET_LIST=""
CONTINUE_ON_CRASH=0
TWO_REGIONS=0
DRY_RUN=0
SKIP_BUILD=0

for arg in "$@"; do
    case "${arg}" in
        --dry-run) DRY_RUN=1 ;;
        --skip-build) SKIP_BUILD=1 ;;
        --continue-on-crash) CONTINUE_ON_CRASH=1 ;;
        --two-regions) TWO_REGIONS=1 ;;
        --project=*) PROJECT="${arg#*=}" ;;
        --region=*) REGION="${arg#*=}" ;;
        --region2=*) REGION2="${arg#*=}"; TWO_REGIONS=1 ;;
        --only=*) TARGET_LIST="${arg#*=}" ;;
        --run-id=*) RUN_ID="${arg#*=}" ;;
        --image-tag=*) IMAGE_TAG="${arg#*=}" ;;
        --parallelism=*) PARALLELISM="${arg#*=}" ;;
        --slices=*) SLICES="${arg#*=}" ;;
        --slice-duration=*) SLICE_DURATION="${arg#*=}" ;;
        *)
            echo "usage: $0 [--dry-run] [--skip-build] [--continue-on-crash] [--two-regions] [--project=P] [--region=R] [--region2=R2] [--only=T1,T2] [--run-id=ID] [--image-tag=TAG] [--parallelism=N] [--slices=N] [--slice-duration=15m]" >&2
            exit 2
            ;;
    esac
done

if [[ -z "${PROJECT}" ]]; then
    echo "error: GCP project is unset; pass --project=<id>, set GCP_PROJECT, or run 'gcloud config set project <id>'." >&2
    exit 2
fi
if [[ -z "${REGION}" ]]; then
    echo "error: GCP region is unset; pass --region=<region>, set GCP_REGION, or run 'gcloud config set compute/region <region>'." >&2
    exit 2
fi
if [[ "${TWO_REGIONS}" -eq 1 && -z "${REGION2}" ]]; then
    echo "error: --two-regions requires --region2=<region> or GCP_REGION2." >&2
    exit 2
fi

BUCKET="${GCS_BUCKET:-${PROJECT}-goblin-fuzz}"
IMAGE_TAG="${IMAGE_TAG:-${RUN_ID}}"
if [[ -n "${TARGET_LIST}" ]]; then
    IFS=',' read -r -a _TLIST <<< "${TARGET_LIST}"
    TASK_COUNT="${#_TLIST[@]}"
else
    TASK_COUNT="$(awk 'NR > 1 && $0 !~ /^#/ && NF > 0 { c++ } END { print c }' "${SCRIPT_DIR}/targets.tsv")"
fi
AR_REGION="${AR_REGION:-${REGION}}"
IMAGE_URI="${AR_REGION}-docker.pkg.dev/${PROJECT}/${AR_REPO}/goblin-fuzzer:${IMAGE_TAG}"
JOB_NAME="goblin-fuzz-${RUN_ID}"

echo "=== Goblin GCP Spot Fuzz Soak ==="
echo "  Project:        ${PROJECT}"
echo "  Region:         ${REGION}"
echo "  Bucket:         gs://${BUCKET}"
echo "  Image:          ${IMAGE_URI}"
echo "  Job:            ${JOB_NAME}"
echo "  Machine:        ${MACHINE_TYPE} (SPOT)"
echo "  Tasks:          ${TASK_COUNT} (parallelism=${PARALLELISM})"
echo "  Budget/task:    ${SLICES} x ${SLICE_DURATION} (maxRunDuration=${MAX_RUN_DURATION}, maxRetryCount=${MAX_RETRY_COUNT})"

if [[ "${DRY_RUN}" -eq 1 ]]; then
    echo "[dry-run] skipping GCP mutations and docker build."
    exit 0
fi

# 1. Enable required APIs idempotently.
echo ">>> Ensuring required APIs are enabled..."
gcloud services enable \
    batch.googleapis.com \
    artifactregistry.googleapis.com \
    compute.googleapis.com \
    storage.googleapis.com \
    logging.googleapis.com \
    --project="${PROJECT}" \
    --quiet

# 2. Ensure regional GCS bucket exists (same region => $0 transfer).
echo ">>> Ensuring GCS bucket gs://${BUCKET} exists in ${REGION}..."
if ! gcloud storage buckets describe "gs://${BUCKET}" --project="${PROJECT}" --quiet >/dev/null 2>&1; then
    gcloud storage buckets create "gs://${BUCKET}" \
        --project="${PROJECT}" \
        --location="${REGION}" \
        --uniform-bucket-level-access \
        --quiet
fi

# 3. Ensure Artifact Registry Docker repo exists (always in AR_REGION).
echo ">>> Ensuring Artifact Registry repo ${AR_REPO} exists in ${AR_REGION}..."
if ! gcloud artifacts repositories describe "${AR_REPO}" \
    --project="${PROJECT}" \
    --location="${AR_REGION}" \
    --quiet >/dev/null 2>&1; then
    gcloud artifacts repositories create "${AR_REPO}" \
        --project="${PROJECT}" \
        --location="${AR_REGION}" \
        --repository-format=docker \
        --description="Goblin fuzz soak runner images" \
        --quiet
fi

# 4. Ensure default Compute Engine SA has Batch/Logging/AR/GCS permissions.
PROJECT_NUMBER="$(gcloud projects describe "${PROJECT}" --format='value(projectNumber)' --quiet)"
COMPUTE_SA="${PROJECT_NUMBER}-compute@developer.gserviceaccount.com"
echo ">>> Ensuring IAM roles for ${COMPUTE_SA}..."
for role in roles/batch.agentReporter roles/logging.logWriter roles/artifactregistry.reader; do
    gcloud projects add-iam-policy-binding "${PROJECT}" \
        --member="serviceAccount:${COMPUTE_SA}" \
        --role="${role}" \
        --condition=None \
        --quiet >/dev/null
done
gcloud storage buckets add-iam-policy-binding "gs://${BUCKET}" \
    --member="serviceAccount:${COMPUTE_SA}" \
    --role="roles/storage.objectAdmin" \
    --project="${PROJECT}" \
    --quiet >/dev/null

# 5. Build and push container image.
if [[ "${SKIP_BUILD}" -eq 0 ]]; then
    echo ">>> Configuring Docker auth for ${AR_REGION}-docker.pkg.dev..."
    gcloud auth configure-docker "${AR_REGION}-docker.pkg.dev" --quiet

    echo ">>> Building fuzzer image ${IMAGE_URI}..."
    docker build \
        -f "${SCRIPT_DIR}/Dockerfile" \
        -t "${IMAGE_URI}" \
        "${GOBLIN_ROOT}"

    echo ">>> Pushing ${IMAGE_URI}..."
    docker push "${IMAGE_URI}"
fi

# 6. Helper to submit one regional Cloud Batch job group.
submit_batch_group() {
    local job_name="$1"
    local loc="$2"
    local count="$3"
    local offset="$4"
    local par="$5"

    local zones_json=""
    local zone_list=""
    if [[ -n "${GCP_ZONES:-}" ]]; then
        zone_list="$(printf '%s\n' "${GCP_ZONES}" | tr ',' '\n')"
    else
        zone_list="$(gcloud compute zones list \
            --project="${PROJECT}" \
            --filter="region:(${loc}) AND status=UP" \
            --format="value(name)" \
            --quiet 2>/dev/null || true)"
    fi
    if [[ -n "${zone_list}" ]]; then
        local z
        while IFS= read -r z; do
            [[ -z "${z}" ]] && continue
            if [[ -n "${zones_json}" ]]; then
                zones_json+=", "
            fi
            zones_json+="\"zones/${z}\""
        done <<< "${zone_list}"
    fi
    if [[ -z "${zones_json}" ]]; then
        zones_json="\"regions/${loc}\""
    fi

    local job_json
    job_json="$(mktemp /tmp/goblin_batch_job.XXXXXX.json)"
    cat > "${job_json}" <<EOF
{
  "taskGroups": [
    {
      "taskCount": "${count}",
      "parallelism": "${par}",
      "taskCountPerNode": "1",
      "taskSpec": {
        "computeResource": {
          "cpuMilli": "14000",
          "memoryMib": "24576"
        },
        "runnables": [
          {
            "container": {
              "imageUri": "${IMAGE_URI}",
              "volumes": [
                "/mnt/disks/fuzz-gcs:/mnt/disks/fuzz-gcs:rw"
              ]
            }
          }
        ],
        "volumes": [
          {
            "gcs": {
              "remotePath": "${BUCKET}"
            },
            "mountPath": "/mnt/disks/fuzz-gcs"
          }
        ],
        "environment": {
          "variables": {
            "RUN_ID": "${RUN_ID}",
            "TASK_OFFSET": "${offset}",
            "TARGET_LIST": "${TARGET_LIST}",
            "CONTINUE_ON_CRASH": "${CONTINUE_ON_CRASH}",
            "GCS_MOUNT": "/mnt/disks/fuzz-gcs",
            "SLICES": "${SLICES}",
            "SLICE_DURATION": "${SLICE_DURATION}"
          }
        },
        "maxRunDuration": "${MAX_RUN_DURATION}",
        "maxRetryCount": ${MAX_RETRY_COUNT},
        "lifecyclePolicies": [
          {
            "action": "RETRY_TASK",
            "actionCondition": {
              "exitCodes": [50001, 143]
            }
          }
        ]
      }
    }
  ],
  "allocationPolicy": {
    "instances": [
      {
        "installOpsAgent": true,
        "policy": {
          "machineType": "${MACHINE_TYPE}",
          "provisioningModel": "SPOT",
          "bootDisk": {
            "type": "pd-balanced",
            "sizeGb": "30"
          }
        }
      }
    ],
    "location": {
      "allowedLocations": [${zones_json}]
    },
    "serviceAccount": {
      "email": "${COMPUTE_SA}"
    }
  },
  "logsPolicy": {
    "destination": "CLOUD_LOGGING"
  }
}
EOF
    echo ">>> Submitting Cloud Batch job ${job_name} in ${loc} (tasks ${offset}..$((offset + count - 1)), parallelism=${par})..."
    gcloud batch jobs submit "${job_name}" \
        --project="${PROJECT}" \
        --location="${loc}" \
        --config="${job_json}" \
        --quiet
    rm -f "${job_json}"
}

if [[ "${TWO_REGIONS}" -eq 1 ]]; then
    GROUP_A_COUNT=9
    GROUP_B_COUNT=$((TASK_COUNT - GROUP_A_COUNT))
    submit_batch_group "${JOB_NAME}-a" "${REGION}" "${GROUP_A_COUNT}" 0 "${GROUP_A_COUNT}"
    submit_batch_group "${JOB_NAME}-b" "${REGION2}" "${GROUP_B_COUNT}" "${GROUP_A_COUNT}" "${GROUP_B_COUNT}"
    echo ""
    echo "=== Submitted (2 regions, ${TASK_COUNT} VMs simultaneously)! ==="
    echo "Monitor job:   ${SCRIPT_DIR}/status_soak.sh ${RUN_ID}"
    echo "Cancel groupA: gcloud batch jobs delete ${JOB_NAME}-a --project=${PROJECT} --location=${REGION} --quiet"
    echo "Cancel groupB: gcloud batch jobs delete ${JOB_NAME}-b --project=${PROJECT} --location=${REGION2} --quiet"
else
    submit_batch_group "${JOB_NAME}" "${REGION}" "${TASK_COUNT}" 0 "${PARALLELISM}"
    echo ""
    echo "=== Submitted! ==="
    echo "Monitor job:   ${SCRIPT_DIR}/status_soak.sh ${RUN_ID}"
    echo "Cancel job:    gcloud batch jobs delete ${JOB_NAME} --project=${PROJECT} --location=${REGION} --quiet"
fi
