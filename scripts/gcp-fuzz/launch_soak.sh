#!/usr/bin/env bash
set -euo pipefail

# Build, push, and launch a Cloud Batch Spot VM fuzz soak for goblin.
# Supports heterogeneous machine types across two regions, up-to-4h slice budgets,
# and cumulative saturation early stopping via goblin-fuzzwatch.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GOBLIN_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
export CLOUDSDK_CORE_DISABLE_PROMPTS=1

gcloud_cfg() {
    local val
    val="$(gcloud config get-value "$1" --quiet 2>/dev/null || true)"
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

infer_machine_arch() {
    local mtype="$1"
    case "${mtype}" in
        c4a-*|t2a-*|n4a-*|a4x-*|c4a|t2a|n4a|a4x)
            printf 'arm64\n'
            ;;
        *)
            printf 'amd64\n'
            ;;
    esac
}

default_goamd64_for_machine() {
    local mtype="$1"
    if [[ "${mtype}" =~ ^(c3d|c3|c4|c4d|n4|n4d|n2|n2d|c2|c2d|t2d|m3|m4|h3|h4d|x4|z3)(-|$) ]]; then
        printf 'v3\n'
    else
        printf 'v1\n'
    fi
}

resolve_goamd64_for_group() {
    local arch="$1"
    local mtype="$2"
    if [[ "${arch}" == "arm64" ]]; then
        printf '\n'
        return 0
    fi
    if [[ -n "${GOAMD64_OV}" ]]; then
        printf '%s\n' "${GOAMD64_OV}"
        return 0
    fi
    default_goamd64_for_machine "${mtype}"
}

infer_boot_disk_type() {
    local mtype="$1"
    if [[ "${mtype}" =~ ^(c4a|c4|c4d|n4|n4a|n4d|h4d|m4|x4|z3)(-|$) ]]; then
        printf 'hyperdisk-balanced\n'
    else
        printf 'pd-balanced\n'
    fi
}

resolve_install_ops_agent() {
    local arch="$1"
    if [[ "${INSTALL_OPS_AGENT}" != "auto" ]]; then
        printf '%s\n' "${INSTALL_OPS_AGENT}"
        return 0
    fi
    if [[ "${arch}" == "arm64" ]]; then
        printf 'false\n'
    else
        printf 'true\n'
    fi
}

image_uri_for_profile() {
    local arch="$1"
    local goamd64_lvl="$2"
    if [[ "${arch}" == "arm64" ]]; then
        printf '%s\n' "${AR_REGION}-docker.pkg.dev/${PROJECT}/${AR_REPO}/goblin-fuzzer:${IMAGE_TAG}-arm64"
    else
        printf '%s\n' "${AR_REGION}-docker.pkg.dev/${PROJECT}/${AR_REPO}/goblin-fuzzer:${IMAGE_TAG}-amd64-${goamd64_lvl}"
    fi
}

discover_zones_for_machine() {
    local mtype="$1"
    local loc="$2"
    local up_zones=""
    local mtype_zones=""
    up_zones="$(gcloud compute zones list \
        --project="${PROJECT}" \
        --filter="region:(${loc}) AND status=UP" \
        --format="value(name)" \
        --quiet 2>/dev/null || true)"
    if [[ -z "${up_zones}" ]]; then
        return 0
    fi
    mtype_zones="$(gcloud compute machine-types list \
        --project="${PROJECT}" \
        --filter="name=${mtype}" \
        --format="value(zone.basename())" \
        --quiet 2>/dev/null | grep -E "^${loc}-" || true)"
    if [[ -z "${mtype_zones}" ]]; then
        printf '%s\n' "${up_zones}"
        return 0
    fi
    awk 'NR == FNR { ok[$0] = 1; next } ok[$0] { print }' \
        <(printf '%s\n' "${mtype_zones}") \
        <(printf '%s\n' "${up_zones}")
}

resolve_machine_arch() {
    local mtype="$1"
    local loc="$2"
    local zones_csv="$3"
    if [[ "${DRY_RUN}" -eq 0 ]]; then
        local probe_zone=""
        if [[ -n "${zones_csv}" ]]; then
            probe_zone="${zones_csv%%,*}"
        else
            probe_zone="$(discover_zones_for_machine "${mtype}" "${loc}" | head -n1)"
        fi
        if [[ -n "${probe_zone}" ]]; then
            local raw_arch=""
            raw_arch="$(gcloud compute machine-types describe "${mtype}" \
                --project="${PROJECT}" \
                --zone="${probe_zone}" \
                --format="value(architecture)" \
                --quiet 2>/dev/null || true)"
            case "${raw_arch}" in
                ARM64|arm64)
                    printf 'arm64\n'
                    return 0
                    ;;
                X86_64|x86_64|AMD64|amd64)
                    printf 'amd64\n'
                    return 0
                    ;;
            esac
        fi
    fi
    infer_machine_arch "${mtype}"
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
            probe_zone="$(discover_zones_for_machine "${mtype}" "${loc}" | head -n1)"
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
TWO_GROUPS=0
ALLOW_OVER_4H=0
DRY_RUN=0
SKIP_BUILD=0
FORMAL="${GOBLIN_FORMAL:-0}"
SIMD_MAX_TIER="${GOBLIN_SIMD_MAX_TIER:-}"
GOAMD64_OV="${GOAMD64:-}"
PROVISIONING_MODEL="${PROVISIONING_MODEL:-SPOT}"
INSTALL_OPS_AGENT="${INSTALL_OPS_AGENT:-auto}"
BUILDER_BOOT_GB="${BUILDER_BOOT_GB:-40}"

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
  --goamd64=v1|v2|v3|v4 --provisioning-model=SPOT|STANDARD
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
        --two-regions) TWO_REGIONS=1; TWO_GROUPS=1 ;;
        --exclude-cbc) EXCLUDE_CBC=1 ;;
        --allow-over-4h) ALLOW_OVER_4H=1 ;;
        --formal|--formal=1|--formal=true) FORMAL="1" ;;
        --formal=0|--formal=false) FORMAL="0" ;;
        --simd-max-tier=*) SIMD_MAX_TIER="${arg#*=}" ;;
        --goamd64=*) GOAMD64_OV="${arg#*=}" ;;
        --provisioning-model=*) PROVISIONING_MODEL="${arg#*=}" ;;
        --project=*) PROJECT="${arg#*=}" ;;
        --region=*) REGION="${arg#*=}" ;;
        --region2=*) REGION2="${arg#*=}"; TWO_GROUPS=1 ;;
        --zones=*) ZONES_A="${arg#*=}" ;;
        --zones2=*) ZONES_B="${arg#*=}" ;;
        --machine-type=*) MACHINE_TYPE="${arg#*=}" ;;
        --machine-type2=*) MACHINE_TYPE2="${arg#*=}"; TWO_GROUPS=1 ;;
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

if [[ -n "${MACHINE_TYPE2}" ]]; then
    TWO_GROUPS=1
fi

if [[ -n "${SIMD_MAX_TIER}" ]]; then
    case "${SIMD_MAX_TIER}" in
        auto|scalar|avx2|avx512) ;;
        *)
            echo "error: invalid --simd-max-tier=${SIMD_MAX_TIER} (expected auto, scalar, avx2, or avx512)." >&2
            exit 2
            ;;
    esac
fi

if [[ -n "${GOAMD64_OV}" ]]; then
    case "${GOAMD64_OV}" in
        v1|v2|v3|v4) ;;
        *)
            echo "error: invalid --goamd64=${GOAMD64_OV} (expected v1, v2, v3, or v4)." >&2
            exit 2
            ;;
    esac
fi

case "${PROVISIONING_MODEL}" in
    SPOT|STANDARD) ;;
    *)
        echo "error: invalid --provisioning-model=${PROVISIONING_MODEL} (expected SPOT or STANDARD)." >&2
        exit 2
        ;;
esac

case "${INSTALL_OPS_AGENT}" in
    auto|true|false) ;;
    *)
        echo "error: invalid INSTALL_OPS_AGENT=${INSTALL_OPS_AGENT} (expected auto, true, or false)." >&2
        exit 2
        ;;
esac

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
if [[ "${TWO_GROUPS}" -eq 1 && -z "${REGION2}" ]]; then
    REGION2="${REGION}"
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
ARCH_A="$(resolve_machine_arch "${MACHINE_TYPE}" "${REGION}" "${ZONES_A}")"
GOAMD64_A="$(resolve_goamd64_for_group "${ARCH_A}" "${MACHINE_TYPE}")"
BOOT_DISK_A="$(infer_boot_disk_type "${MACHINE_TYPE}")"
OPS_AGENT_A="$(resolve_install_ops_agent "${ARCH_A}")"

BUCKET="${GCS_BUCKET:-${PROJECT}-goblin-fuzz}"
IMAGE_TAG="${IMAGE_TAG:-${RUN_ID}}"
AR_REGION="${AR_REGION:-${REGION}}"
IMAGE_URI_A="$(image_uri_for_profile "${ARCH_A}" "${GOAMD64_A}")"
JOB_NAME="goblin-fuzz-${RUN_ID}"

if [[ "${TWO_GROUPS}" -eq 1 && "${TASK_COUNT}" -eq 1 ]]; then
    echo "note: only 1 target selected; launching single-group job in ${REGION}." >&2
    TWO_GROUPS=0
    TWO_REGIONS=0
fi

GROUP_A_COUNT="${TASK_COUNT}"
GROUP_B_COUNT=0
PAR_A="${PARALLELISM}"
PAR_B=0
CPU_MILLI_B=""
MEM_MIB_B=""
ARCH_B=""
GOAMD64_B=""
BOOT_DISK_B=""
OPS_AGENT_B=""
IMAGE_URI_B=""

if [[ "${TWO_GROUPS}" -eq 1 ]]; then
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
    ARCH_B="$(resolve_machine_arch "${MACHINE_TYPE2}" "${REGION2}" "${ZONES_B}")"
    GOAMD64_B="$(resolve_goamd64_for_group "${ARCH_B}" "${MACHINE_TYPE2}")"
    BOOT_DISK_B="$(infer_boot_disk_type "${MACHINE_TYPE2}")"
    OPS_AGENT_B="$(resolve_install_ops_agent "${ARCH_B}")"
    IMAGE_URI_B="$(image_uri_for_profile "${ARCH_B}" "${GOAMD64_B}")"
elif [[ "${PARALLELISM}" -gt "${TASK_COUNT}" ]]; then
    PAR_A="${TASK_COUNT}"
fi

detect_arm64_builder_mode() {
    local host_m
    host_m="$(uname -m)"
    if [[ "${host_m}" == "aarch64" || "${host_m}" == "arm64" ]]; then
        printf 'native\n'
        return 0
    fi
    if docker buildx version >/dev/null 2>&1 && \
        docker buildx inspect --bootstrap 2>/dev/null | grep -q 'linux/arm64' && \
        grep -q '^enabled' /proc/sys/fs/binfmt_misc/qemu-aarch64* 2>/dev/null; then
        printf 'buildx\n'
        return 0
    fi
    printf 'gce-ephemeral\n'
}

NEED_ARM64=0
if [[ "${ARCH_A}" == "arm64" ]] || [[ "${TWO_GROUPS}" -eq 1 && "${ARCH_B}" == "arm64" ]]; then
    NEED_ARM64=1
fi
ARM64_BUILDER_MODE="none"
if [[ "${NEED_ARM64}" -eq 1 ]]; then
    ARM64_BUILDER_MODE="$(detect_arm64_builder_mode)"
fi

echo "=== Goblin GCP Fuzz Soak ==="
echo "  Project:        ${PROJECT}"
echo "  Bucket:         gs://${BUCKET}"
echo "  Job:            ${JOB_NAME}"
echo "  Provisioning:   ${PROVISIONING_MODEL}"
echo "  Targets:        ${TASK_COUNT} (exclude_cbc=${EXCLUDE_CBC})"
echo "  Budget/task:    ${SLICES} x ${SLICE_DURATION} ($((TOTAL_BUDGET_SEC / 60))m total, maxRunDuration=${MAX_RUN_DURATION}, maxRetryCount=${MAX_RETRY_COUNT})"
echo "  Formal/SIMD:    GOBLIN_FORMAL=${FORMAL} GOBLIN_SIMD_MAX_TIER=${SIMD_MAX_TIER:-auto}"
echo "  Early stop:     preset=${EARLY_STOP_PRESET} min_time=${MIN_TIME:-default} min_execs=${MIN_EXECS:-default} stall_time=${STALL_TIME:-default} stall_execs=${STALL_EXECS:-default}"
echo "  ARM64 builder:  arm64_builder=${ARM64_BUILDER_MODE}"
if [[ "${TWO_GROUPS}" -eq 1 ]]; then
    echo "  Group A:        ${REGION} | ${MACHINE_TYPE} (arch=${ARCH_A}, goamd64=${GOAMD64_A:-na}, bootDisk=${BOOT_DISK_A}, installOpsAgent=${OPS_AGENT_A}, cpuMilli=${CPU_MILLI_A}, memoryMib=${MEM_MIB_A}) | imageUri=${IMAGE_URI_A} | tasks=0..$((GROUP_A_COUNT - 1)) (${GROUP_A_COUNT}, par=${PAR_A})"
    echo "  Group B:        ${REGION2} | ${MACHINE_TYPE2} (arch=${ARCH_B}, goamd64=${GOAMD64_B:-na}, bootDisk=${BOOT_DISK_B}, installOpsAgent=${OPS_AGENT_B}, cpuMilli=${CPU_MILLI_B}, memoryMib=${MEM_MIB_B}) | imageUri=${IMAGE_URI_B} | tasks=${GROUP_A_COUNT}..$((TASK_COUNT - 1)) (${GROUP_B_COUNT}, par=${PAR_B})"
else
    echo "  Group:          ${REGION} | ${MACHINE_TYPE} (arch=${ARCH_A}, goamd64=${GOAMD64_A:-na}, bootDisk=${BOOT_DISK_A}, installOpsAgent=${OPS_AGENT_A}, cpuMilli=${CPU_MILLI_A}, memoryMib=${MEM_MIB_A}) | imageUri=${IMAGE_URI_A} | tasks=0..$((TASK_COUNT - 1)) (${TASK_COUNT}, par=${PAR_A})"
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

BUILDER_VM_NAME=""
BUILDER_VM_ZONE=""
BUILDER_TARBALL=""
BUILDER_SSH_IAP=0

cleanup_builder_vm() {
    if [[ -n "${BUILDER_TARBALL}" ]]; then
        rm -f "${BUILDER_TARBALL}" || true
        BUILDER_TARBALL=""
    fi
    if [[ -n "${BUILDER_VM_NAME}" && -n "${BUILDER_VM_ZONE}" ]]; then
        echo ">>> Cleaning up ephemeral ARM64 builder VM ${BUILDER_VM_NAME} (${BUILDER_VM_ZONE})..." >&2
        gcloud compute instances delete "${BUILDER_VM_NAME}" \
            --project="${PROJECT}" \
            --zone="${BUILDER_VM_ZONE}" \
            --quiet >/dev/null 2>&1 || true
        BUILDER_VM_NAME=""
        BUILDER_VM_ZONE=""
    fi
}

vm_ssh() {
    local vm_name="$1"
    local vm_zone="$2"
    shift 2
    if [[ "${BUILDER_SSH_IAP}" -eq 1 ]]; then
        gcloud compute ssh "${vm_name}" \
            --project="${PROJECT}" \
            --zone="${vm_zone}" \
            --tunnel-through-iap \
            --ssh-flag="-T" \
            --ssh-flag="-o ConnectTimeout=20" \
            --ssh-flag="-o ServerAliveInterval=15" \
            --ssh-flag="-o ServerAliveCountMax=8" \
            --ssh-flag="-o BatchMode=yes" \
            --ssh-flag="-o StrictHostKeyChecking=no" \
            --quiet \
            -- "$@"
    else
        gcloud compute ssh "${vm_name}" \
            --project="${PROJECT}" \
            --zone="${vm_zone}" \
            --ssh-flag="-T" \
            --ssh-flag="-o ConnectTimeout=15" \
            --ssh-flag="-o ServerAliveInterval=15" \
            --ssh-flag="-o ServerAliveCountMax=8" \
            --ssh-flag="-o BatchMode=yes" \
            --ssh-flag="-o StrictHostKeyChecking=no" \
            --quiet \
            -- "$@"
    fi
}

vm_scp() {
    local vm_zone="$1"
    shift
    if [[ "${BUILDER_SSH_IAP}" -eq 1 ]]; then
        gcloud compute scp \
            --project="${PROJECT}" \
            --zone="${vm_zone}" \
            --tunnel-through-iap \
            --scp-flag="-o ConnectTimeout=20" \
            --scp-flag="-o ServerAliveInterval=15" \
            --scp-flag="-o ServerAliveCountMax=8" \
            --scp-flag="-o BatchMode=yes" \
            --scp-flag="-o StrictHostKeyChecking=no" \
            --quiet \
            "$@"
    else
        gcloud compute scp \
            --project="${PROJECT}" \
            --zone="${vm_zone}" \
            --scp-flag="-o ConnectTimeout=15" \
            --scp-flag="-o ServerAliveInterval=15" \
            --scp-flag="-o ServerAliveCountMax=8" \
            --scp-flag="-o BatchMode=yes" \
            --scp-flag="-o StrictHostKeyChecking=no" \
            --quiet \
            "$@"
    fi
}

wait_vm_ssh() {
    local vm_name="$1"
    local vm_zone="$2"
    local attempt
    for attempt in $(seq 1 30); do
        if gcloud compute ssh "${vm_name}" \
            --project="${PROJECT}" \
            --zone="${vm_zone}" \
            --ssh-flag="-o ConnectTimeout=8" \
            --ssh-flag="-o BatchMode=yes" \
            --ssh-flag="-o StrictHostKeyChecking=no" \
            --quiet \
            -- "uname -m" >/dev/null 2>&1; then
            BUILDER_SSH_IAP=0
            return 0
        fi
        if (( attempt >= 5 )) && gcloud compute ssh "${vm_name}" \
            --project="${PROJECT}" \
            --zone="${vm_zone}" \
            --tunnel-through-iap \
            --ssh-flag="-o ConnectTimeout=10" \
            --ssh-flag="-o BatchMode=yes" \
            --ssh-flag="-o StrictHostKeyChecking=no" \
            --quiet \
            -- "uname -m" >/dev/null 2>&1; then
            BUILDER_SSH_IAP=1
            return 0
        fi
        sleep 5
    done
    echo "error: timed out waiting for SSH on ${vm_name} (${vm_zone})." >&2
    return 1
}

build_and_push_amd64_image() {
    local uri="$1"
    local goamd64_lvl="$2"
    local host_m
    host_m="$(uname -m)"

    echo ">>> Building amd64 fuzzer image ${uri} (GOAMD64=${goamd64_lvl})..."
    if [[ "${host_m}" == "x86_64" || "${host_m}" == "amd64" ]]; then
        docker build \
            --platform=linux/amd64 \
            --build-arg "GOAMD64=${goamd64_lvl}" \
            -f "${SCRIPT_DIR}/Dockerfile" \
            -t "${uri}" \
            "${GOBLIN_ROOT}"
        echo ">>> Pushing ${uri}..."
        docker push "${uri}"
    elif docker buildx version >/dev/null 2>&1; then
        docker buildx build \
            --platform=linux/amd64 \
            --build-arg "GOAMD64=${goamd64_lvl}" \
            --push \
            -f "${SCRIPT_DIR}/Dockerfile" \
            -t "${uri}" \
            "${GOBLIN_ROOT}"
    else
        echo "error: cannot build linux/amd64 image from host arch ${host_m} without docker buildx." >&2
        return 1
    fi
}

build_and_push_arm64_image() {
    local uri="$1"
    echo ">>> Building arm64 fuzzer image ${uri} via ${ARM64_BUILDER_MODE}..."
    case "${ARM64_BUILDER_MODE}" in
        native)
            docker build \
                -f "${SCRIPT_DIR}/Dockerfile" \
                -t "${uri}" \
                "${GOBLIN_ROOT}"
            echo ">>> Pushing ${uri}..."
            docker push "${uri}"
            return 0
            ;;
        buildx)
            docker buildx build \
                --platform=linux/arm64 \
                --push \
                -f "${SCRIPT_DIR}/Dockerfile" \
                -t "${uri}" \
                "${GOBLIN_ROOT}"
            return 0
            ;;
    esac

    trap cleanup_builder_vm EXIT INT TERM

    local raw_suffix vm_name
    raw_suffix="$(printf '%s' "${RUN_ID}" | tr '[:upper:]_' '[:lower:]-' | tr -cd 'a-z0-9-')"
    vm_name="goblin-arm64-builder-${raw_suffix}-$$"
    vm_name="${vm_name:0:63}"
    vm_name="${vm_name%-}"

    local -a builder_specs=()
    local z cand_loc
    local -a search_locs=("${REGION}")
    if [[ "${AR_REGION}" != "${REGION}" ]]; then
        search_locs+=("${AR_REGION}")
    fi
    if [[ "${TWO_GROUPS}" -eq 1 && "${REGION2}" != "${REGION}" && "${REGION2}" != "${AR_REGION}" ]]; then
        search_locs+=("${REGION2}")
    fi
    for cand_loc in "${search_locs[@]}"; do
        while IFS= read -r z; do
            [[ -n "${z}" ]] && builder_specs+=("c4a-standard-4|hyperdisk-balanced|${z}")
        done < <(discover_zones_for_machine "c4a-standard-4" "${cand_loc}")
    done
    for cand_loc in "${search_locs[@]}"; do
        while IFS= read -r z; do
            [[ -n "${z}" ]] && builder_specs+=("t2a-standard-4|pd-balanced|${z}")
        done < <(discover_zones_for_machine "t2a-standard-4" "${cand_loc}")
    done
    if [[ "${#builder_specs[@]}" -eq 0 ]]; then
        echo "error: no c4a-standard-4 or t2a-standard-4 zones available for ephemeral ARM64 builder VM." >&2
        return 1
    fi

    local spec mtype disk_type created=0
    for spec in "${builder_specs[@]}"; do
        IFS='|' read -r mtype disk_type z <<< "${spec}"
        echo ">>> Provisioning ephemeral ARM64 builder VM ${vm_name} (${mtype}, ${disk_type}, ${BUILDER_BOOT_GB}GB) in ${z}..."
        BUILDER_VM_NAME="${vm_name}"
        BUILDER_VM_ZONE="${z}"
        if gcloud compute instances create "${vm_name}" \
            --project="${PROJECT}" \
            --zone="${z}" \
            --machine-type="${mtype}" \
            --image-family=debian-12-arm64 \
            --image-project=debian-cloud \
            --boot-disk-type="${disk_type}" \
            --boot-disk-size="${BUILDER_BOOT_GB}GB" \
            --no-service-account \
            --no-scopes \
            --quiet; then
            created=1
            break
        fi
        gcloud compute instances delete "${vm_name}" \
            --project="${PROJECT}" \
            --zone="${z}" \
            --quiet >/dev/null 2>&1 || true
        BUILDER_VM_NAME=""
        BUILDER_VM_ZONE=""
    done
    if [[ "${created}" -ne 1 ]]; then
        echo "error: failed to provision ephemeral ARM64 builder VM in any candidate zone." >&2
        return 1
    fi

    echo ">>> Waiting for SSH on ${BUILDER_VM_NAME} (${BUILDER_VM_ZONE})..."
    wait_vm_ssh "${BUILDER_VM_NAME}" "${BUILDER_VM_ZONE}"

    BUILDER_TARBALL="$(mktemp /tmp/goblin-arm64-src.XXXXXX.tar.gz)"
    echo ">>> Packaging minimal ARM64 build context tarball..."
    tar -C "${GOBLIN_ROOT}" \
        --exclude='.git' \
        --exclude='.cache' \
        --exclude='.mypy_cache' \
        --exclude='.ruff_cache' \
        --exclude='__pycache__' \
        --exclude='./third_party/Cbc/dist' \
        --exclude='./formal/.lake' \
        --exclude='*.test' \
        --exclude='*.prof' \
        -czf "${BUILDER_TARBALL}" .

    echo ">>> Uploading build context to ${BUILDER_VM_NAME}..."
    vm_scp "${BUILDER_VM_ZONE}" "${BUILDER_TARBALL}" "${BUILDER_VM_NAME}:/tmp/goblin-src.tar.gz"
    rm -f "${BUILDER_TARBALL}"
    BUILDER_TARBALL=""

    echo ">>> Installing docker.io and building ${uri} on ${BUILDER_VM_NAME}..."
    vm_ssh "${BUILDER_VM_NAME}" "${BUILDER_VM_ZONE}" "cat > /tmp/goblin-build-arm64.sh && chmod +x /tmp/goblin-build-arm64.sh" <<EOF
#!/usr/bin/env bash
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
sudo apt-get update
sudo apt-get install -y --no-install-recommends docker.io ca-certificates
sudo systemctl start docker || true
rm -rf /tmp/goblin-src
mkdir -p /tmp/goblin-src
tar -xzf /tmp/goblin-src.tar.gz -C /tmp/goblin-src
rm -f /tmp/goblin-src.tar.gz
sudo docker build -f /tmp/goblin-src/scripts/gcp-fuzz/Dockerfile -t '${uri}' /tmp/goblin-src
EOF
    vm_ssh "${BUILDER_VM_NAME}" "${BUILDER_VM_ZONE}" "cat > /tmp/goblin-run-remote.sh && chmod +x /tmp/goblin-run-remote.sh" <<'EOF'
#!/usr/bin/env bash
set +e
bash /tmp/goblin-build-arm64.sh
rc=$?
printf '%s\n' "${rc}" > /tmp/goblin-remote.rc
EOF
    vm_ssh "${BUILDER_VM_NAME}" "${BUILDER_VM_ZONE}" \
        "rm -f /tmp/goblin-remote.rc /tmp/goblin-remote.log /tmp/goblin-remote.pid; nohup setsid bash /tmp/goblin-run-remote.sh </dev/null >/tmp/goblin-remote.log 2>&1 & echo \$! > /tmp/goblin-remote.pid; sleep 1"

    local poll status_out rc_val=""
    for poll in $(seq 1 160); do
        sleep 15
        status_out="$(vm_ssh "${BUILDER_VM_NAME}" "${BUILDER_VM_ZONE}" \
            "if [ -f /tmp/goblin-remote.rc ]; then echo DONE:\$(cat /tmp/goblin-remote.rc); elif [ -f /tmp/goblin-remote.pid ] && kill -0 \$(cat /tmp/goblin-remote.pid) 2>/dev/null; then echo RUNNING; tail -n 2 /tmp/goblin-remote.log 2>/dev/null || true; else echo DEAD; fi" 2>/dev/null || echo "POLL_RETRY")"
        if [[ "${status_out}" == DONE:* ]]; then
            rc_val="${status_out#DONE:}"
            rc_val="${rc_val%%[$'\r\n']*}"
            break
        fi
        if [[ "${status_out}" == "DEAD" ]]; then
            echo "error: remote ARM64 build process exited without writing status file." >&2
            vm_ssh "${BUILDER_VM_NAME}" "${BUILDER_VM_ZONE}" "tail -n 120 /tmp/goblin-remote.log 2>/dev/null || true" >&2 || true
            return 1
        fi
        printf '    [arm64-build poll %d] %s\n' "${poll}" "$(printf '%s' "${status_out}" | tr '\n' ' ' | cut -c1-160)"
    done
    if [[ -z "${rc_val}" || "${rc_val}" != "0" ]]; then
        echo "error: remote ARM64 docker build failed (rc=${rc_val:-timeout})." >&2
        vm_ssh "${BUILDER_VM_NAME}" "${BUILDER_VM_ZONE}" "tail -n 120 /tmp/goblin-remote.log 2>/dev/null || true" >&2 || true
        return 1
    fi

    echo ">>> Pushing ${uri} from ${BUILDER_VM_NAME} to Artifact Registry..."
    gcloud auth print-access-token --quiet | \
        vm_ssh "${BUILDER_VM_NAME}" "${BUILDER_VM_ZONE}" \
        "sudo docker login -u oauth2accesstoken --password-stdin 'https://${AR_REGION}-docker.pkg.dev'"
    vm_ssh "${BUILDER_VM_NAME}" "${BUILDER_VM_ZONE}" "sudo docker push '${uri}'"

    echo ">>> Deleting ephemeral ARM64 builder VM ${BUILDER_VM_NAME} (${BUILDER_VM_ZONE}) before Batch submission..."
    gcloud compute instances delete "${BUILDER_VM_NAME}" \
        --project="${PROJECT}" \
        --zone="${BUILDER_VM_ZONE}" \
        --quiet
    BUILDER_VM_NAME=""
    BUILDER_VM_ZONE=""
}

# 5. Build and push deduplicated per-arch container images.
if [[ "${SKIP_BUILD}" -eq 0 ]]; then
    declare -a AMD64_LEVELS=()
    if [[ "${ARCH_A}" == "amd64" ]]; then
        AMD64_LEVELS+=("${GOAMD64_A}")
    fi
    if [[ "${TWO_GROUPS}" -eq 1 && "${ARCH_B}" == "amd64" ]]; then
        if [[ "${#AMD64_LEVELS[@]}" -eq 0 || "${AMD64_LEVELS[0]}" != "${GOAMD64_B}" ]]; then
            AMD64_LEVELS+=("${GOAMD64_B}")
        fi
    fi

    if [[ "${#AMD64_LEVELS[@]}" -gt 0 ]] || [[ "${NEED_ARM64}" -eq 1 && "${ARM64_BUILDER_MODE}" != "gce-ephemeral" ]]; then
        echo ">>> Configuring Docker auth for ${AR_REGION}-docker.pkg.dev..."
        gcloud auth configure-docker "${AR_REGION}-docker.pkg.dev" --quiet
    fi

    for lvl in "${AMD64_LEVELS[@]:-}"; do
        [[ -z "${lvl}" ]] && continue
        build_and_push_amd64_image "$(image_uri_for_profile amd64 "${lvl}")" "${lvl}"
    done

    if [[ "${NEED_ARM64}" -eq 1 ]]; then
        build_and_push_arm64_image "$(image_uri_for_profile arm64 "")"
    fi
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
    local image_uri="${10}"
    local boot_disk_type="${11}"
    local install_ops_agent="${12}"

    local zones_json=""
    local zone_list=""
    if [[ -n "${zones_csv}" ]]; then
        zone_list="$(printf '%s\n' "${zones_csv}" | tr ',' '\n')"
    else
        zone_list="$(discover_zones_for_machine "${mtype}" "${loc}")"
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
              "imageUri": "${image_uri}",
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
        "installOpsAgent": ${install_ops_agent},
        "policy": {
          "machineType": "${mtype}",
          "provisioningModel": "${PROVISIONING_MODEL}",
          "bootDisk": {
            "type": "${boot_disk_type}",
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
    echo ">>> Submitting Cloud Batch job ${job_name} in ${loc} (${mtype}, image=${image_uri}, disk=${boot_disk_type}, tasks ${offset}..$((offset + count - 1)), parallelism=${par})..."
    gcloud batch jobs submit "${job_name}" \
        --project="${PROJECT}" \
        --location="${loc}" \
        --config="${job_json}" \
        --quiet
    rm -f "${job_json}"
}

if [[ "${TWO_GROUPS}" -eq 1 ]]; then
    submit_batch_group "${JOB_NAME}-a" "${REGION}" "${GROUP_A_COUNT}" 0 "${PAR_A}" "${MACHINE_TYPE}" "${CPU_MILLI_A}" "${MEM_MIB_A}" "${ZONES_A}" "${IMAGE_URI_A}" "${BOOT_DISK_A}" "${OPS_AGENT_A}"
    submit_batch_group "${JOB_NAME}-b" "${REGION2}" "${GROUP_B_COUNT}" "${GROUP_A_COUNT}" "${PAR_B}" "${MACHINE_TYPE2}" "${CPU_MILLI_B}" "${MEM_MIB_B}" "${ZONES_B}" "${IMAGE_URI_B}" "${BOOT_DISK_B}" "${OPS_AGENT_B}"
    echo ""
    echo "=== Submitted (2 groups, ${TASK_COUNT} tasks)! ==="
    echo "Monitor job:   ${SCRIPT_DIR}/status_soak.sh ${RUN_ID} --region=${REGION} --region2=${REGION2} --two-regions"
    echo "Cancel groupA: gcloud batch jobs delete ${JOB_NAME}-a --project=${PROJECT} --location=${REGION} --quiet"
    echo "Cancel groupB: gcloud batch jobs delete ${JOB_NAME}-b --project=${PROJECT} --location=${REGION2} --quiet"
else
    submit_batch_group "${JOB_NAME}" "${REGION}" "${TASK_COUNT}" 0 "${PAR_A}" "${MACHINE_TYPE}" "${CPU_MILLI_A}" "${MEM_MIB_A}" "${ZONES_A}" "${IMAGE_URI_A}" "${BOOT_DISK_A}" "${OPS_AGENT_A}"
    echo ""
    echo "=== Submitted! ==="
    echo "Monitor job:   ${SCRIPT_DIR}/status_soak.sh ${RUN_ID} --region=${REGION}"
    echo "Cancel job:    gcloud batch jobs delete ${JOB_NAME} --project=${PROJECT} --location=${REGION} --quiet"
fi
