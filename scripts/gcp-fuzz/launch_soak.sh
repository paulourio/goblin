#!/usr/bin/env bash
set -euo pipefail

# Build, push, and launch a Cloud Batch Spot VM fuzz soak for goblin.
# Supports heterogeneous machine types across two regions, up-to-4h slice budgets,
# and cumulative saturation early stopping via goblin-fuzzwatch.

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

parse_duration_sec() {
    local raw="$1"
    if [[ -z "${raw}" ]]; then
        return 1
    fi
    if [[ "${raw}" =~ ^[0-9]+$ ]]; then
        printf '%s\n' "${raw}"
        return 0
    fi
    awk -v s="${raw}" 'BEGIN {
        total = 0
        while (length(s) > 0) {
            if (match(s, /^[0-9]+(h|m|s)/)) {
                tok = substr(s, 1, RLENGTH)
                unit = substr(tok, length(tok), 1)
                val = substr(tok, 1, length(tok) - 1) + 0
                if (unit == "h") total += val * 3600
                else if (unit == "m") total += val * 60
                else total += val
                s = substr(s, RLENGTH + 1)
            } else {
                exit 1
            }
        }
        printf "%d\n", total
    }'
}

infer_machine_shape() {
    local mtype="$1"
    if [[ "${mtype}" =~ ^([a-z][0-9]+[a-z]?)-(highcpu|standard|highmem)-([0-9]+)(-lssd)?$ ]]; then
        local family="${BASH_REMATCH[1]}"
        local tier="${BASH_REMATCH[2]}"
        local vcpus="${BASH_REMATCH[3]}"
        local gib_per_cpu=2
        case "${tier}" in
            highcpu)
                case "${family}" in
                    e2|n2|n2d) gib_per_cpu=1 ;;
                    *) gib_per_cpu=2 ;;
                esac
                ;;
            standard) gib_per_cpu=4 ;;
            highmem) gib_per_cpu=8 ;;
        esac
        printf '%s %s\n' "${vcpus}" "$((vcpus * gib_per_cpu * 1024))"
        return 0
    fi
    if [[ "${mtype}" =~ ^([a-z0-9]+-)?custom-([0-9]+)-([0-9]+)(-ext)?$ ]]; then
        printf '%s %s\n' "${BASH_REMATCH[2]}" "${BASH_REMATCH[3]}"
        return 0
    fi
    return 1
}

resolve_machine_resources() {
    local mtype="$1"
    local loc="$2"
    local zones_csv="$3"
    local ov_cpu="$4"
    local ov_mem="$5"
    local vcpus=""
    local mem_mb=""

    if [[ -n "${ov_cpu}" && -n "${ov_mem}" ]]; then
        printf '%s %s\n' "${ov_cpu}" "${ov_mem}"
        return 0
    fi

    if [[ "${DRY_RUN}" -eq 0 ]]; then
        local probe_zone=""
        if [[ -n "${zones_csv}" ]]; then
            probe_zone="${zones_csv%%,*}"
        else
            probe_zone="$(gcloud compute zones list \
                --project="${PROJECT}" \
                --filter="region:(${loc}) AND status=UP" \
                --limit=1 \
                --format="value(name)" \
                --quiet 2>/dev/null || true)"
        fi
        if [[ -n "${probe_zone}" ]]; then
            local desc
            desc="$(gcloud compute machine-types describe "${mtype}" \
                --project="${PROJECT}" \
                --zone="${probe_zone}" \
                --format="csv[no-heading](guestCpus,memoryMb)" \
                --quiet 2>/dev/null || true)"
            if [[ "${desc}" =~ ^([0-9]+),([0-9]+)$ ]]; then
                vcpus="${BASH_REMATCH[1]}"
                mem_mb="${BASH_REMATCH[2]}"
            fi
        fi
    fi

    if [[ -z "${vcpus}" || -z "${mem_mb}" ]]; then
        local shape
        if shape="$(infer_machine_shape "${mtype}")"; then
            vcpus="${shape%% *}"
            mem_mb="${shape##* }"
        else
            echo "error: cannot infer vCPU/memory for machine type ${mtype}; pass --cpu-milli and --memory-mib." >&2
            return 2
        fi
    fi

    local cpu_milli=""
    if [[ "${vcpus}" -ge 6 ]]; then
        cpu_milli="$(((vcpus - 2) * 1000))"
    elif [[ "${vcpus}" -ge 2 ]]; then
        cpu_milli="$(((vcpus - 1) * 1000))"
    else
        cpu_milli=1000
    fi

    local mem_mib="$(( (mem_mb * 75 / 100) / 256 * 256 ))"
    if [[ "${mem_mib}" -lt 1024 ]]; then
        mem_mib=1024
    fi

    if [[ -n "${ov_cpu}" ]]; then
        cpu_milli="${ov_cpu}"
    fi
    if [[ -n "${ov_mem}" ]]; then
        mem_mib="${ov_mem}"
    fi
    printf '%s %s\n' "${cpu_milli}" "${mem_mib}"
}

PROJECT="${GCP_PROJECT:-$(gcloud_cfg project)}"
REGION="${GCP_REGION:-$(gcloud_cfg compute/region)}"
REGION2="${GCP_REGION2:-}"
ZONES_A="${GCP_ZONES:-}"
ZONES_B="${GCP_ZONES2:-}"
AR_REPO="${AR_REPO:-goblin-fuzz}"
MACHINE_TYPE="${MACHINE_TYPE:-c3d-highcpu-16}"
MACHINE_TYPE2="${MACHINE_TYPE2:-}"
CPU_MILLI_OV="${CPU_MILLI:-}"
MEMORY_MIB_OV="${MEMORY_MIB:-}"
CPU_MILLI2_OV="${CPU_MILLI2:-}"
MEMORY_MIB2_OV="${MEMORY_MIB2:-}"
PARALLELISM="${PARALLELISM:-9}"
PARALLELISM_SET=0
PARALLELISM2=""
REGION_A_COUNT=""
DURATION=""
SLICES="${SLICES:-4}"
SLICE_DURATION="${SLICE_DURATION:-15m}"
SLICE_DURATION_SET=0
SLICES_SET=0
MAX_RUN_DURATION="${MAX_RUN_DURATION:-}"
MAX_RETRY_COUNT="${MAX_RETRY_COUNT:-}"
RUN_ID="${RUN_ID:-soak-$(date -u +%Y%m%d-%H%M%S)}"
IMAGE_TAG=""
TARGET_LIST=""
EXCLUDE_CBC=0
CONTINUE_ON_CRASH=0
TWO_REGIONS=0
ALLOW_OVER_4H=0
DRY_RUN=0
SKIP_BUILD=0
FORMAL="${GOBLIN_FORMAL:-0}"
SIMD_MAX_TIER="${GOBLIN_SIMD_MAX_TIER:-}"

EARLY_STOP_PRESET="${EARLY_STOP_PRESET:-soak}"
MIN_TIME="${MIN_TIME:-}"
MIN_EXECS="${MIN_EXECS:-}"
STALL_TIME="${STALL_TIME:-}"
STALL_EXECS="${STALL_EXECS:-}"
WINDOW_TIME="${WINDOW_TIME:-}"
WINDOW_EXECS="${WINDOW_EXECS:-}"
WINDOW_MIN_NEW="${WINDOW_MIN_NEW:-}"

usage() {
    cat >&2 <<'EOF'
usage: launch_soak.sh [flags]
  --dry-run --skip-build --continue-on-crash --two-regions --exclude-cbc --allow-over-4h --formal
  --simd-max-tier=auto|scalar|avx2|avx512
  --project=P --region=R --region2=R2 --zones=Z1,Z2 --zones2=Z1,Z2
  --machine-type=TYPE --machine-type2=TYPE
  --cpu-milli=N --memory-mib=N --cpu-milli2=N --memory-mib2=N
  --parallelism=N --parallelism2=N --region-a-count=N
  --only=T1,T2 --run-id=ID --image-tag=TAG
  --duration=4h --total-duration=4h --slices=N --slice-duration=15m
  --max-run-duration=21960s --max-retry-count=N
  --early-stop=soak|local|deep|off
  --min-time=D --min-execs=N --stall-time=D --stall-execs=N
  --window-time=D --window-execs=N --window-min-new=N
EOF
}

for arg in "$@"; do
    case "${arg}" in
        --dry-run) DRY_RUN=1 ;;
        --skip-build) SKIP_BUILD=1 ;;
        --continue-on-crash) CONTINUE_ON_CRASH=1 ;;
        --two-regions) TWO_REGIONS=1 ;;
        --exclude-cbc) EXCLUDE_CBC=1 ;;
        --allow-over-4h) ALLOW_OVER_4H=1 ;;
        --formal|--formal=1|--formal=true) FORMAL="1" ;;
        --formal=0|--formal=false) FORMAL="0" ;;
        --simd-max-tier=*) SIMD_MAX_TIER="${arg#*=}" ;;
        --project=*) PROJECT="${arg#*=}" ;;
        --region=*) REGION="${arg#*=}" ;;
        --region2=*) REGION2="${arg#*=}"; TWO_REGIONS=1 ;;
        --zones=*) ZONES_A="${arg#*=}" ;;
        --zones2=*) ZONES_B="${arg#*=}" ;;
        --machine-type=*) MACHINE_TYPE="${arg#*=}" ;;
        --machine-type2=*) MACHINE_TYPE2="${arg#*=}" ;;
        --cpu-milli=*) CPU_MILLI_OV="${arg#*=}" ;;
        --memory-mib=*) MEMORY_MIB_OV="${arg#*=}" ;;
        --cpu-milli2=*) CPU_MILLI2_OV="${arg#*=}" ;;
        --memory-mib2=*) MEMORY_MIB2_OV="${arg#*=}" ;;
        --only=*) TARGET_LIST="${arg#*=}" ;;
        --run-id=*) RUN_ID="${arg#*=}" ;;
        --image-tag=*) IMAGE_TAG="${arg#*=}" ;;
        --parallelism=*) PARALLELISM="${arg#*=}"; PARALLELISM_SET=1 ;;
        --parallelism2=*) PARALLELISM2="${arg#*=}" ;;
        --region-a-count=*) REGION_A_COUNT="${arg#*=}" ;;
        --duration=*|--total-duration=*) DURATION="${arg#*=}" ;;
        --slices=*) SLICES="${arg#*=}"; SLICES_SET=1 ;;
        --slice-duration=*) SLICE_DURATION="${arg#*=}"; SLICE_DURATION_SET=1 ;;
        --max-run-duration=*) MAX_RUN_DURATION="${arg#*=}" ;;
        --max-retry-count=*) MAX_RETRY_COUNT="${arg#*=}" ;;
        --early-stop=*) EARLY_STOP_PRESET="${arg#*=}" ;;
        --min-time=*) MIN_TIME="${arg#*=}" ;;
        --min-execs=*) MIN_EXECS="${arg#*=}" ;;
        --stall-time=*) STALL_TIME="${arg#*=}" ;;
        --stall-execs=*) STALL_EXECS="${arg#*=}" ;;
        --window-time=*) WINDOW_TIME="${arg#*=}" ;;
        --window-execs=*) WINDOW_EXECS="${arg#*=}" ;;
        --window-min-new=*) WINDOW_MIN_NEW="${arg#*=}" ;;
        *)
            usage
            exit 2
            ;;
    esac
done

if [[ -n "${SIMD_MAX_TIER}" ]]; then
    case "${SIMD_MAX_TIER}" in
        auto|scalar|avx2|avx512) ;;
        *)
            echo "error: invalid --simd-max-tier=${SIMD_MAX_TIER} (expected auto, scalar, avx2, or avx512)." >&2
            exit 2
            ;;
    esac
fi

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

case "${EARLY_STOP_PRESET}" in
    off|local|soak|deep) ;;
    *)
        echo "error: invalid --early-stop=${EARLY_STOP_PRESET} (expected off, local, soak, or deep)." >&2
        exit 2
        ;;
esac

SLICE_SEC="$(parse_duration_sec "${SLICE_DURATION}")" || {
    echo "error: invalid --slice-duration=${SLICE_DURATION}." >&2
    exit 2
}
if [[ "${SLICE_SEC}" -le 0 ]]; then
    echo "error: --slice-duration must be positive." >&2
    exit 2
fi

if [[ -n "${DURATION}" ]]; then
    TOTAL_REQ_SEC="$(parse_duration_sec "${DURATION}")" || {
        echo "error: invalid --duration=${DURATION}." >&2
        exit 2
    }
    if [[ "${TOTAL_REQ_SEC}" -le 0 ]]; then
        echo "error: --duration must be positive." >&2
        exit 2
    fi
    if [[ "${SLICE_DURATION_SET}" -eq 0 && "${SLICES_SET}" -eq 0 && "${TOTAL_REQ_SEC}" -ge 10800 ]]; then
        SLICE_DURATION="30m"
        SLICE_SEC="$(parse_duration_sec "${SLICE_DURATION}")"
    fi
    SLICES="$(((TOTAL_REQ_SEC + SLICE_SEC - 1) / SLICE_SEC))"
fi

TOTAL_BUDGET_SEC="$((SLICES * SLICE_SEC))"
if [[ "${TOTAL_BUDGET_SEC}" -gt 14400 && "${ALLOW_OVER_4H}" -eq 0 ]]; then
    echo "error: total per-target budget ${TOTAL_BUDGET_SEC}s (${SLICES}x${SLICE_DURATION}) exceeds 4h cap (14400s); pass --allow-over-4h to override." >&2
    exit 2
fi

if [[ -z "${MAX_RUN_DURATION}" ]]; then
    MAX_RUN_DURATION="$((TOTAL_BUDGET_SEC + SLICES * 360 + 1800))s"
else
    MAX_RUN_SEC="$(parse_duration_sec "${MAX_RUN_DURATION}")" || {
        echo "error: invalid --max-run-duration=${MAX_RUN_DURATION}." >&2
        exit 2
    }
    MAX_RUN_DURATION="${MAX_RUN_SEC}s"
fi

if [[ -z "${MAX_RETRY_COUNT}" ]]; then
    if [[ "${TOTAL_BUDGET_SEC}" -gt 3600 ]]; then
        MAX_RETRY_COUNT=4
    else
        MAX_RETRY_COUNT=2
    fi
fi

if [[ -z "${TARGET_LIST}" && "${EXCLUDE_CBC}" -eq 1 ]]; then
    TARGET_LIST="$(awk -F'\t' 'NR > 1 && $0 !~ /^#/ && NF > 0 && $2 !~ /_cbc\.test$/ { out = (out == "" ? $4 : out "," $4) } END { print out }' "${SCRIPT_DIR}/targets.tsv")"
fi

if [[ -n "${TARGET_LIST}" ]]; then
    IFS=',' read -r -a _TLIST <<< "${TARGET_LIST}"
    for tgt in "${_TLIST[@]}"; do
        if ! awk -F'\t' -v t="${tgt}" 'NR > 1 && $0 !~ /^#/ && $4 == t { found=1; exit } END { exit !found }' "${SCRIPT_DIR}/targets.tsv"; then
            echo "error: unknown fuzz target ${tgt} in --only / TARGET_LIST." >&2
            exit 2
        fi
    done
    TASK_COUNT="${#_TLIST[@]}"
else
    TASK_COUNT="$(awk 'NR > 1 && $0 !~ /^#/ && NF > 0 { c++ } END { print c }' "${SCRIPT_DIR}/targets.tsv")"
fi

if [[ "${TASK_COUNT}" -lt 1 ]]; then
    echo "error: resolved zero fuzz targets." >&2
    exit 2
fi

MACHINE_TYPE2="${MACHINE_TYPE2:-${MACHINE_TYPE}}"
ZONES_B="${ZONES_B:-}"

RES_A="$(resolve_machine_resources "${MACHINE_TYPE}" "${REGION}" "${ZONES_A}" "${CPU_MILLI_OV}" "${MEMORY_MIB_OV}")"
CPU_MILLI_A="${RES_A%% *}"
MEM_MIB_A="${RES_A##* }"

BUCKET="${GCS_BUCKET:-${PROJECT}-goblin-fuzz}"
IMAGE_TAG="${IMAGE_TAG:-${RUN_ID}}"
AR_REGION="${AR_REGION:-${REGION}}"
IMAGE_URI="${AR_REGION}-docker.pkg.dev/${PROJECT}/${AR_REPO}/goblin-fuzzer:${IMAGE_TAG}"
JOB_NAME="goblin-fuzz-${RUN_ID}"

if [[ "${TWO_REGIONS}" -eq 1 && "${TASK_COUNT}" -eq 1 ]]; then
    echo "note: only 1 target selected; launching single-region job in ${REGION}." >&2
    TWO_REGIONS=0
fi

GROUP_A_COUNT="${TASK_COUNT}"
GROUP_B_COUNT=0
PAR_A="${PARALLELISM}"
PAR_B=0
CPU_MILLI_B=""
MEM_MIB_B=""

if [[ "${TWO_REGIONS}" -eq 1 ]]; then
    GROUP_A_COUNT="${REGION_A_COUNT:-$(((TASK_COUNT + 1) / 2))}"
    if [[ "${GROUP_A_COUNT}" -lt 1 || "${GROUP_A_COUNT}" -ge "${TASK_COUNT}" ]]; then
        echo "error: --region-a-count=${GROUP_A_COUNT} must be between 1 and $((TASK_COUNT - 1))." >&2
        exit 2
    fi
    GROUP_B_COUNT="$((TASK_COUNT - GROUP_A_COUNT))"
    if [[ "${PARALLELISM_SET}" -eq 1 ]]; then
        PAR_A="$((PARALLELISM < GROUP_A_COUNT ? PARALLELISM : GROUP_A_COUNT))"
    else
        PAR_A="${GROUP_A_COUNT}"
    fi
    if [[ -n "${PARALLELISM2}" ]]; then
        PAR_B="$((PARALLELISM2 < GROUP_B_COUNT ? PARALLELISM2 : GROUP_B_COUNT))"
    elif [[ "${PARALLELISM_SET}" -eq 1 ]]; then
        PAR_B="$((PARALLELISM < GROUP_B_COUNT ? PARALLELISM : GROUP_B_COUNT))"
    else
        PAR_B="${GROUP_B_COUNT}"
    fi
    RES_B="$(resolve_machine_resources "${MACHINE_TYPE2}" "${REGION2}" "${ZONES_B}" "${CPU_MILLI2_OV}" "${MEMORY_MIB2_OV}")"
    CPU_MILLI_B="${RES_B%% *}"
    MEM_MIB_B="${RES_B##* }"
elif [[ "${PARALLELISM}" -gt "${TASK_COUNT}" ]]; then
    PAR_A="${TASK_COUNT}"
fi

echo "=== Goblin GCP Spot Fuzz Soak ==="
echo "  Project:        ${PROJECT}"
echo "  Bucket:         gs://${BUCKET}"
echo "  Image:          ${IMAGE_URI}"
echo "  Job:            ${JOB_NAME}"
echo "  Targets:        ${TASK_COUNT} (exclude_cbc=${EXCLUDE_CBC})"
echo "  Budget/task:    ${SLICES} x ${SLICE_DURATION} ($((TOTAL_BUDGET_SEC / 60))m total, maxRunDuration=${MAX_RUN_DURATION}, maxRetryCount=${MAX_RETRY_COUNT})"
echo "  Formal/SIMD:    GOBLIN_FORMAL=${FORMAL} GOBLIN_SIMD_MAX_TIER=${SIMD_MAX_TIER:-auto}"
echo "  Early stop:     preset=${EARLY_STOP_PRESET} min_time=${MIN_TIME:-default} min_execs=${MIN_EXECS:-default} stall_time=${STALL_TIME:-default} stall_execs=${STALL_EXECS:-default}"
if [[ "${TWO_REGIONS}" -eq 1 ]]; then
    echo "  Group A:        ${REGION} | ${MACHINE_TYPE} (cpuMilli=${CPU_MILLI_A}, memoryMib=${MEM_MIB_A}) | tasks=0..$((GROUP_A_COUNT - 1)) (${GROUP_A_COUNT}, par=${PAR_A})"
    echo "  Group B:        ${REGION2} | ${MACHINE_TYPE2} (cpuMilli=${CPU_MILLI_B}, memoryMib=${MEM_MIB_B}) | tasks=${GROUP_A_COUNT}..$((TASK_COUNT - 1)) (${GROUP_B_COUNT}, par=${PAR_B})"
else
    echo "  Group:          ${REGION} | ${MACHINE_TYPE} (cpuMilli=${CPU_MILLI_A}, memoryMib=${MEM_MIB_A}) | tasks=0..$((TASK_COUNT - 1)) (${TASK_COUNT}, par=${PAR_A})"
fi

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

# 2. Ensure GCS bucket exists in primary region.
echo ">>> Ensuring GCS bucket gs://${BUCKET} exists in ${REGION}..."
if ! gcloud storage buckets describe "gs://${BUCKET}" --project="${PROJECT}" --quiet >/dev/null 2>&1; then
    gcloud storage buckets create "gs://${BUCKET}" \
        --project="${PROJECT}" \
        --location="${REGION}" \
        --uniform-bucket-level-access \
        --quiet
fi

# 3. Ensure Artifact Registry Docker repo exists in AR_REGION.
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

# 4. Ensure default Compute Engine SA has Batch, Logging, AR, and GCS permissions.
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
    local mtype="$6"
    local cpu_milli="$7"
    local mem_mib="$8"
    local zones_csv="$9"

    local zones_json=""
    local zone_list=""
    if [[ -n "${zones_csv}" ]]; then
        zone_list="$(printf '%s\n' "${zones_csv}" | tr ',' '\n')"
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
          "cpuMilli": "${cpu_milli}",
          "memoryMib": "${mem_mib}"
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
            "SLICE_DURATION": "${SLICE_DURATION}",
            "GOBLIN_FORMAL": "${FORMAL}",
            "GOBLIN_SIMD_MAX_TIER": "${SIMD_MAX_TIER}",
            "EARLY_STOP_PRESET": "${EARLY_STOP_PRESET}",
            "MIN_TIME": "${MIN_TIME}",
            "MIN_EXECS": "${MIN_EXECS}",
            "STALL_TIME": "${STALL_TIME}",
            "STALL_EXECS": "${STALL_EXECS}",
            "WINDOW_TIME": "${WINDOW_TIME}",
            "WINDOW_EXECS": "${WINDOW_EXECS}",
            "WINDOW_MIN_NEW": "${WINDOW_MIN_NEW}"
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
          "machineType": "${mtype}",
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
    echo ">>> Submitting Cloud Batch job ${job_name} in ${loc} (${mtype}, tasks ${offset}..$((offset + count - 1)), parallelism=${par})..."
    gcloud batch jobs submit "${job_name}" \
        --project="${PROJECT}" \
        --location="${loc}" \
        --config="${job_json}" \
        --quiet
    rm -f "${job_json}"
}

if [[ "${TWO_REGIONS}" -eq 1 ]]; then
    submit_batch_group "${JOB_NAME}-a" "${REGION}" "${GROUP_A_COUNT}" 0 "${PAR_A}" "${MACHINE_TYPE}" "${CPU_MILLI_A}" "${MEM_MIB_A}" "${ZONES_A}"
    submit_batch_group "${JOB_NAME}-b" "${REGION2}" "${GROUP_B_COUNT}" "${GROUP_A_COUNT}" "${PAR_B}" "${MACHINE_TYPE2}" "${CPU_MILLI_B}" "${MEM_MIB_B}" "${ZONES_B}"
    echo ""
    echo "=== Submitted (2 regions, ${TASK_COUNT} VMs simultaneously)! ==="
    echo "Monitor job:   ${SCRIPT_DIR}/status_soak.sh ${RUN_ID} --region=${REGION} --region2=${REGION2}"
    echo "Cancel groupA: gcloud batch jobs delete ${JOB_NAME}-a --project=${PROJECT} --location=${REGION} --quiet"
    echo "Cancel groupB: gcloud batch jobs delete ${JOB_NAME}-b --project=${PROJECT} --location=${REGION2} --quiet"
else
    submit_batch_group "${JOB_NAME}" "${REGION}" "${TASK_COUNT}" 0 "${PAR_A}" "${MACHINE_TYPE}" "${CPU_MILLI_A}" "${MEM_MIB_A}" "${ZONES_A}"
    echo ""
    echo "=== Submitted! ==="
    echo "Monitor job:   ${SCRIPT_DIR}/status_soak.sh ${RUN_ID} --region=${REGION}"
    echo "Cancel job:    gcloud batch jobs delete ${JOB_NAME} --project=${PROJECT} --location=${REGION} --quiet"
fi
