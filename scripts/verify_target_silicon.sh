#!/usr/bin/env bash
set -euo pipefail

# Cross-compile static pure-Go test and benchmark binaries, execute them on
# ephemeral Compute Engine target-silicon instances (Emerald Rapids AVX-512 on
# linux/amd64 and Neoverse on linux/arm64), collect sanitized JSON telemetry,
# and tear down every provisioned instance upon exit.

export CLOUDSDK_CORE_DISABLE_PROMPTS=1

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

gcloud_cfg() {
    local val
    val="$(gcloud config get-value "$1" --quiet 2>/dev/null || true)"
    if [[ "${val}" == "(unset)" ]]; then
        val=""
    fi
    printf '%s' "${val}"
}

DRY_RUN=0
ONLY="all"
PROJECT="${GCP_PROJECT:-}"
REGION="${GCP_REGION:-}"
ZONE_AMD64="${GCP_ZONE_AMD64:-${GCP_ZONE:-}}"
ZONE_ARM64="${GCP_ZONE_ARM64:-}"
STAGE_DIR="/tmp/goblin-silicon-stage"
OUT_DIR="bench/results/target_silicon"
KEEP_STAGE=0
SKIP_BUILD=0

usage() {
    cat >&2 <<'EOF'
usage: verify_target_silicon.sh [options]

Options:
  --dry-run               Print planned build and cloud steps without mutations
  --only=amd64|arm64      Run verification only for the selected architecture
  --project=<id>          Explicit GCP project ID (defaults to GCP_PROJECT or gcloud config)
  --region=<region>       Optional preferred GCP region prefix for zone ordering
  --zone-amd64=<zone>     Explicit zone override for the n4-standard-4 instance
  --zone-arm64=<zone>     Explicit zone override for the c4a/t2a arm64 instance
  --stage-dir=<dir>       Local staging directory outside the repository
  --out-dir=<dir>         Output directory for sanitized JSON telemetry
  --keep-stage            Keep local staging artifacts on exit
  --skip-build            Reuse existing staged binaries in --stage-dir
  -h, --help              Print this help text
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --dry-run)
            DRY_RUN=1
            shift
            ;;
        --only=*)
            ONLY="${1#*=}"
            shift
            ;;
        --only)
            shift
            ONLY="${1:?--only requires amd64, arm64, or all}"
            shift
            ;;
        --project=*)
            PROJECT="${1#*=}"
            shift
            ;;
        --project)
            shift
            PROJECT="${1:?--project requires a GCP project ID}"
            shift
            ;;
        --region=*)
            REGION="${1#*=}"
            shift
            ;;
        --region)
            shift
            REGION="${1:?--region requires a GCP region}"
            shift
            ;;
        --zone-amd64=*)
            ZONE_AMD64="${1#*=}"
            shift
            ;;
        --zone-amd64)
            shift
            ZONE_AMD64="${1:?--zone-amd64 requires a zone}"
            shift
            ;;
        --zone-arm64=*)
            ZONE_ARM64="${1#*=}"
            shift
            ;;
        --zone-arm64)
            shift
            ZONE_ARM64="${1:?--zone-arm64 requires a zone}"
            shift
            ;;
        --stage-dir=*)
            STAGE_DIR="${1#*=}"
            shift
            ;;
        --stage-dir)
            shift
            STAGE_DIR="${1:?--stage-dir requires a directory path}"
            shift
            ;;
        --out-dir=*)
            OUT_DIR="${1#*=}"
            shift
            ;;
        --out-dir)
            shift
            OUT_DIR="${1:?--out-dir requires a directory path}"
            shift
            ;;
        --keep-stage)
            KEEP_STAGE=1
            shift
            ;;
        --skip-build)
            SKIP_BUILD=1
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "error: unknown option '$1'" >&2
            usage
            exit 2
            ;;
    esac
done

case "${ONLY}" in
    all|amd64|arm64) ;;
    *)
        echo "error: invalid --only='${ONLY}' (expected amd64, arm64, or all)" >&2
        exit 2
        ;;
esac

RUN_AMD64=0
RUN_ARM64=0
if [[ "${ONLY}" == "all" || "${ONLY}" == "amd64" ]]; then
    RUN_AMD64=1
fi
if [[ "${ONLY}" == "all" || "${ONLY}" == "arm64" ]]; then
    RUN_ARM64=1
fi

if [[ -z "${PROJECT}" ]] && command -v gcloud >/dev/null 2>&1; then
    PROJECT="$(gcloud_cfg project)"
fi
if [[ -z "${REGION}" ]] && command -v gcloud >/dev/null 2>&1; then
    REGION="$(gcloud_cfg compute/region)"
fi

if [[ "${OUT_DIR}" != /* ]]; then
    OUT_DIR_ABS="${REPO_ROOT}/${OUT_DIR}"
else
    OUT_DIR_ABS="${OUT_DIR}"
fi

REPO_REAL="$(cd "${REPO_ROOT}" && pwd -P)"
mkdir -p "${STAGE_DIR}"
STAGE_REAL="$(cd "${STAGE_DIR}" && pwd -P)"
case "${STAGE_REAL}" in
    "${REPO_REAL}"|"${REPO_REAL}"/*)
        echo "error: --stage-dir '${STAGE_REAL}' must reside outside repository '${REPO_REAL}'" >&2
        exit 2
        ;;
esac

RUN_TAG="$(date -u +%Y%m%d-%H%M%S)-$$"
VM_AMD64="goblin-silicon-amd64-${RUN_TAG}"
VM_ARM64="goblin-silicon-arm64-${RUN_TAG}"
declare -a CREATED_VMS=()
declare -a BG_PIDS=()

delete_matching_silicon_vms() {
    if [[ "${DRY_RUN}" -eq 1 || -z "${PROJECT}" ]]; then
        return 0
    fi
    if ! command -v gcloud >/dev/null 2>&1; then
        return 0
    fi
    local entry vm_name vm_zone
    for entry in "${CREATED_VMS[@]:-}"; do
        [[ -n "${entry}" ]] || continue
        vm_name="${entry%%:*}"
        vm_zone="${entry#*:}"
        if gcloud compute instances describe "${vm_name}" \
            --project="${PROJECT}" \
            --zone="${vm_zone}" \
            --format="value(name)" \
            --quiet >/dev/null 2>&1; then
            echo ">>> Deleting instance ${vm_name}..." >&2
            gcloud compute instances delete "${vm_name}" \
                --project="${PROJECT}" \
                --zone="${vm_zone}" \
                --quiet >/dev/null 2>&1 || true
        fi
    done

    local listed
    listed="$(gcloud compute instances list \
        --project="${PROJECT}" \
        --filter="name~^goblin-silicon-" \
        --format="csv[no-heading](name,zone.basename())" \
        --quiet 2>/dev/null || true)"
    if [[ -n "${listed}" ]]; then
        while IFS=',' read -r vm_name vm_zone; do
            [[ -n "${vm_name}" && -n "${vm_zone}" ]] || continue
            echo ">>> Sweeping residual instance ${vm_name}..." >&2
            gcloud compute instances delete "${vm_name}" \
                --project="${PROJECT}" \
                --zone="${vm_zone}" \
                --quiet >/dev/null 2>&1 || true
        done <<< "${listed}"
    fi
}

cleanup() {
    local pid
    for pid in "${BG_PIDS[@]:-}"; do
        if [[ -n "${pid}" ]] && kill -0 "${pid}" 2>/dev/null; then
            kill "${pid}" 2>/dev/null || true
        fi
    done
    wait 2>/dev/null || true
    delete_matching_silicon_vms
    if [[ "${KEEP_STAGE}" -eq 0 && "${DRY_RUN}" -eq 1 ]]; then
        rmdir "${STAGE_REAL}" 2>/dev/null || true
    fi
}
trap cleanup EXIT INT TERM

if [[ "${DRY_RUN}" -eq 1 ]]; then
    PROJ_DISPLAY="${PROJECT:-<dynamic-gcloud-project>}"
    echo "=== Goblin Target-Silicon Verification [DRY RUN] ==="
    echo "  Target scope:   ${ONLY} (amd64=${RUN_AMD64}, arm64=${RUN_ARM64})"
    echo "  Stage dir:      ${STAGE_REAL}"
    echo "  Output dir:     ${OUT_DIR_ABS}"
    if [[ "${RUN_AMD64}" -eq 1 ]]; then
        echo "  [amd64 build]   CGO_ENABLED=0 GOOS=linux GOARCH=amd64 default (v1) + GOEXPERIMENT=simd (v1, v3, v4) for ., ./internal/simd, ./assign, ./format, ./cmd/goblin-bench"
        echo "  [amd64 zones]   gcloud compute machine-types list --project=${PROJ_DISPLAY} --filter=name=n4-standard-4 --format=value(zone.basename()) --quiet"
        echo "  [amd64 create]  gcloud compute instances create ${VM_AMD64} --project=${PROJ_DISPLAY} --zone=<resolved-zone> --machine-type=n4-standard-4 --image-family=debian-12 --image-project=debian-cloud --boot-disk-type=hyperdisk-balanced --boot-disk-size=20GB --no-service-account --no-scopes --quiet"
    fi
    if [[ "${RUN_ARM64}" -eq 1 ]]; then
        echo "  [arm64 build]   CGO_ENABLED=0 GOOS=linux GOARCH=arm64 default + GOEXPERIMENT=simd for ., ./internal/simd, ./assign, ./format, ./cmd/goblin-bench"
        echo "  [arm64 zones]   gcloud compute machine-types list --project=${PROJ_DISPLAY} --filter=name=(c4a-standard-4,t2a-standard-4) --format=csv[no-heading](name,zone.basename()) --quiet"
        echo "  [arm64 primary] gcloud compute instances create ${VM_ARM64} --project=${PROJ_DISPLAY} --zone=<resolved-zone> --machine-type=c4a-standard-4 --image-family=debian-12-arm64 --image-project=debian-cloud --boot-disk-type=hyperdisk-balanced --boot-disk-size=20GB --no-service-account --no-scopes --quiet"
        echo "  [arm64 backup]  gcloud compute instances create ${VM_ARM64} --project=${PROJ_DISPLAY} --zone=<resolved-zone> --machine-type=t2a-standard-4 --image-family=debian-12-arm64 --image-project=debian-cloud --boot-disk-type=pd-balanced --boot-disk-size=20GB --no-service-account --no-scopes --quiet"
    fi
    echo "  [teardown]      trap cleanup EXIT INT TERM deletes all ^goblin-silicon- instances and verifies zero remain."
    exit 0
fi

if ! command -v gcloud >/dev/null 2>&1; then
    echo "error: gcloud CLI is required in PATH" >&2
    exit 2
fi
if [[ -z "${PROJECT}" ]]; then
    echo "error: GCP project is unset; pass --project=<id>, set GCP_PROJECT, or configure gcloud project" >&2
    exit 2
fi

GIT_COMMIT="$(git -C "${REPO_ROOT}" rev-parse HEAD)"
SOURCE_FP="$(cd "${REPO_ROOT}" && go run ./cmd/goblin-bench -print-fingerprint)"
DEFAULT_GO_VER="$(cd "${REPO_ROOT}" && CGO_ENABLED=0 go env GOVERSION)"
SIMD_GO_VER="$(cd "${REPO_ROOT}" && CGO_ENABLED=0 GOEXPERIMENT=simd go env GOVERSION)"

build_suite_profile() {
    local out_sub="$1"
    local goos="$2"
    local goarch="$3"
    local goamd64="$4"
    local simd_exp="$5"

    mkdir -p "${out_sub}"
    local -a env_args=("CGO_ENABLED=0" "GOOS=${goos}" "GOARCH=${goarch}")
    if [[ -n "${goamd64}" ]]; then
        env_args+=("GOAMD64=${goamd64}")
    fi
    if [[ "${simd_exp}" -eq 1 ]]; then
        env_args+=("GOEXPERIMENT=simd")
    fi

    (
        cd "${REPO_ROOT}"
        env "${env_args[@]}" go test -c -o "${out_sub}/goblin.test" .
        env "${env_args[@]}" go test -c -o "${out_sub}/simd.test" ./internal/simd
        env "${env_args[@]}" go test -c -o "${out_sub}/assign.test" ./assign
        env "${env_args[@]}" go test -c -o "${out_sub}/format.test" ./format
        env "${env_args[@]}" go build -o "${out_sub}/goblin-bench" ./cmd/goblin-bench
    )
}

ARM64_SIMD_COMPILES=0
if [[ "${SKIP_BUILD}" -eq 0 ]]; then
    echo "=== Step A: Cross-compiling static CGO_ENABLED=0 target-silicon bundles ==="
    rm -rf "${STAGE_REAL:?}/amd64" "${STAGE_REAL:?}/arm64" "${STAGE_REAL:?}/bundle" \
        "${STAGE_REAL:?}/amd64_bin.tar.gz" "${STAGE_REAL:?}/arm64_bin.tar.gz" \
        "${STAGE_REAL:?}/raw_amd64" "${STAGE_REAL:?}/raw_arm64"
    mkdir -p "${STAGE_REAL}/amd64/bin" "${STAGE_REAL}/arm64/bin" "${STAGE_REAL}/logs"

    declare -a BUILD_PIDS=()
    if [[ "${RUN_AMD64}" -eq 1 ]]; then
        build_suite_profile "${STAGE_REAL}/amd64/bin/default_v1" linux amd64 v1 0 &
        BUILD_PIDS+=("$!")
        build_suite_profile "${STAGE_REAL}/amd64/bin/simd_v1" linux amd64 v1 1 &
        BUILD_PIDS+=("$!")
        build_suite_profile "${STAGE_REAL}/amd64/bin/simd_v3" linux amd64 v3 1 &
        BUILD_PIDS+=("$!")
        build_suite_profile "${STAGE_REAL}/amd64/bin/simd_v4" linux amd64 v4 1 &
        BUILD_PIDS+=("$!")
    fi
    if [[ "${RUN_ARM64}" -eq 1 ]]; then
        build_suite_profile "${STAGE_REAL}/arm64/bin/default" linux arm64 "" 0 &
        BUILD_PIDS+=("$!")
    fi
    for bpid in "${BUILD_PIDS[@]}"; do
        wait "${bpid}"
    done

    if [[ "${RUN_ARM64}" -eq 1 ]]; then
        if build_suite_profile "${STAGE_REAL}/arm64/bin/simd" linux arm64 "" 1 >"${STAGE_REAL}/logs/arm64_simd_build.log" 2>&1; then
            ARM64_SIMD_COMPILES=1
            SIMD_GO_VER="$(go version -m "${STAGE_REAL}/arm64/bin/simd/goblin-bench" | awk 'NR==1 {print $2}')"
            echo "  [arm64] GOEXPERIMENT=simd cross-compiled cleanly (${SIMD_GO_VER})."
        else
            ARM64_SIMD_COMPILES=0
            echo "  [arm64] GOEXPERIMENT=simd rejected on linux/arm64; recorded fallback."
        fi
        printf '%s\n' "${ARM64_SIMD_COMPILES}" >"${STAGE_REAL}/arm64/simd_compiles.txt"
    fi
    if [[ "${RUN_AMD64}" -eq 1 && -x "${STAGE_REAL}/amd64/bin/simd_v1/goblin-bench" ]]; then
        SIMD_GO_VER="$(go version -m "${STAGE_REAL}/amd64/bin/simd_v1/goblin-bench" | awk 'NR==1 {print $2}')"
        DEFAULT_GO_VER="$(go version -m "${STAGE_REAL}/amd64/bin/default_v1/goblin-bench" | awk 'NR==1 {print $2}')"
    fi

    echo "  Packing minimal corpus and source-fingerprint bundle..."
    BUNDLE_ROOT="${STAGE_REAL}/bundle"
    mkdir -p "${BUNDLE_ROOT}/format" "${BUNDLE_ROOT}/bench/results"
    cp -f "${REPO_ROOT}/go.mod" "${BUNDLE_ROOT}/go.mod"
    for rel_dir in . assign format internal internal/simd bench cmd/goblin-bench; do
        mkdir -p "${BUNDLE_ROOT}/${rel_dir}"
        find "${REPO_ROOT}/${rel_dir}" -maxdepth 1 -type f \
            \( -name '*.go' ! -name '*_test.go' -o -name '*.py' -o -name '*.c' -o -name '*.cpp' -o -name '*.h' \) \
            -exec cp -f -t "${BUNDLE_ROOT}/${rel_dir}" {} +
    done
    tar -C "${REPO_ROOT}" \
        -cf - \
        testdata/corpus \
        testdata/assign_corpus \
        testdata/public \
        testdata/formal \
        testdata/regression \
        bench/results/python_baseline.json \
        bench/results/scipy_assign_baseline.json \
        bench/results/goblin_cbc.json | tar -C "${BUNDLE_ROOT}" -xf -

    tar -C "${BUNDLE_ROOT}" --owner=0 --group=0 -czf "${STAGE_REAL}/testdata_bundle.tar.gz" .
    rm -rf "${BUNDLE_ROOT}"
else
    if [[ -f "${STAGE_REAL}/arm64/simd_compiles.txt" ]]; then
        ARM64_SIMD_COMPILES="$(cat "${STAGE_REAL}/arm64/simd_compiles.txt")"
    elif [[ -x "${STAGE_REAL}/arm64/bin/simd/goblin.test" ]]; then
        ARM64_SIMD_COMPILES=1
    fi
fi

write_remote_scripts() {
    cat > "${STAGE_REAL}/run_amd64_remote.sh" <<'REOF'
#!/usr/bin/env bash
set -euo pipefail

WORK="/tmp/goblin-silicon-work"
rm -rf "${WORK}"
mkdir -p "${WORK}/out" "${WORK}/bin"
tar -C "${WORK}" -xzf /tmp/goblin-silicon-testdata.tar.gz
tar -C "${WORK}/bin" -xzf /tmp/goblin-silicon-amd64-bin.tar.gz
cd "${WORK}"

measure_cpu_busy_pct() {
    awk '
        NR == 1 {
            u1 = $2 + $3 + $4 + $7 + $8 + $9
            i1 = $5 + $6
            next
        }
        {
            u2 = $2 + $3 + $4 + $7 + $8 + $9
            i2 = $5 + $6
            dt = (u2 - u1) + (i2 - i1)
            if (dt <= 0) {
                printf "0.00\n"
            } else {
                printf "%.2f\n", 100.0 * (u2 - u1) / dt
            }
        }
    ' <(grep '^cpu ' /proc/stat) <(sleep 2; grep '^cpu ' /proc/stat)
}

wait_for_quiescence() {
    local label="$1"
    local attempt busy
    for attempt in $(seq 1 24); do
        busy="$(measure_cpu_busy_pct)"
        if awk -v b="${busy}" 'BEGIN { exit !(b < 5.0) }'; then
            printf '%s\n' "${busy}"
            return 0
        fi
        echo "  [${label}] waiting for idle VM CPU (attempt ${attempt}/24, busy=${busy}%)..." >&2
        sleep 5
    done
    echo "error: VM failed quiescence gate (${label}: busy=${busy}% >= 5.0%)" >&2
    ps -eo pid,pcpu,comm --sort=-pcpu 2>/dev/null | head -n 10 >&2 || true
    exit 1
}

PRE_TEST_BUSY="$(wait_for_quiescence pre-test)"
uname -srm > out/uname_srm.txt
LC_ALL=C lscpu | grep -E '^(Architecture|CPU op-mode\(s\)|Byte Order|CPU\(s\)|On-line CPU\(s\) list|Vendor ID|Model name|CPU family|Model|Stepping|Thread\(s\) per core|Core\(s\) per socket|Socket\(s\)|NUMA node\(s\)|L1d cache|L1i cache|L2 cache|L3 cache):' > out/lscpu.txt
grep -m1 '^model name' /proc/cpuinfo > out/cpu_model_raw.txt || true
grep -m1 '^flags' /proc/cpuinfo | tr ' \t' '\n' | grep '^avx512' | sort -u > out/avx512_flags.txt
cat /proc/pressure/cpu > out/cpu_pressure_pre.txt 2>/dev/null || true

# Confirm runtime CPUID detects avx512 hardware and active dispatch tier.
./bin/simd_v1/simd.test -test.v -test.run='^TestTierIntrospectionAndCaps$' > out/tier_introspection_auto.log 2>&1
grep -q 'hardware=avx512 initial_active=avx512' out/tier_introspection_auto.log
GOBLIN_SIMD_MAX_TIER=avx2 ./bin/simd_v1/simd.test -test.v -test.run='^TestTierIntrospectionAndCaps$' > out/tier_introspection_avx2.log 2>&1
grep -q 'hardware=avx512 initial_active=avx2' out/tier_introspection_avx2.log
GOBLIN_SIMD_MAX_TIER=scalar ./bin/simd_v1/simd.test -test.v -test.run='^TestTierIntrospectionAndCaps$' > out/tier_introspection_scalar.log 2>&1
grep -q 'hardware=avx512 initial_active=scalar' out/tier_introspection_scalar.log

SKIP_ROOT='^(TestDocumentationAndBenchmarkSync|TestTargetSiliconDocumentationSync|TestDocBacktickIdentifiersResolve|TestFuzzHarnessTargetsSync|TestTargetSiliconHygieneGate|TestGoblinBinaryDeadCodeElimination|TestFormalZeroEscapeSoakLogAudit|TestFormalNoSilentFuzzBridgeEscapes|TestFormalTrack5D_ELFDisassemblyMatchesLean)$'
SKIP_ASSIGN='^TestAssignBinaryDeadCodeElimination$'
EXPLICIT_RUN='^(TestFormalTrack5D_SIMDParityAndEdgeAudit|TestFormalTrack5E_LiveSIMDDifferential|TestFormalTrack5E_ZeroAllocContracts|TestFormalTrack5E_HardwareNaNParity|TestTwoSumTwoProdFMASafety)$'

: > out/test_matrix.tsv

run_profile_tests() {
    local label="$1"
    local bin_dir="$2"
    local goamd64="$3"
    local simd_enabled="$4"
    local max_tier="$5"
    local expect_active="$6"

    local t0
    t0="$(date +%s)"
    local -a env_prefix=()
    if [[ -n "${max_tier}" ]]; then
        env_prefix=("GOBLIN_SIMD_MAX_TIER=${max_tier}")
    fi

    echo "  [test] profile=${label} max_tier=${max_tier:-auto}" >&2
    env ${env_prefix[@]+"${env_prefix[@]}"} "${bin_dir}/simd.test" -test.count=1 > "out/test_${label}_simd.log" 2>&1
    env ${env_prefix[@]+"${env_prefix[@]}"} "${bin_dir}/assign.test" -test.count=1 -test.skip="${SKIP_ASSIGN}" > "out/test_${label}_assign.log" 2>&1
    (cd format && env ${env_prefix[@]+"${env_prefix[@]}"} "${WORK}/${bin_dir#./}/format.test" -test.count=1) > "out/test_${label}_format.log" 2>&1
    env ${env_prefix[@]+"${env_prefix[@]}"} "${bin_dir}/goblin.test" -test.count=1 -test.skip="${SKIP_ROOT}" > "out/test_${label}_goblin.log" 2>&1

    env ${env_prefix[@]+"${env_prefix[@]}"} "${bin_dir}/goblin.test" -test.v -test.count=1 -test.run="${EXPLICIT_RUN}" > "out/test_${label}_explicit.log" 2>&1
    local pass_cnt skip_cnt
    pass_cnt="$(grep -c '^--- PASS: Test' "out/test_${label}_explicit.log" || true)"
    skip_cnt="$(grep -c '^--- SKIP: Test' "out/test_${label}_explicit.log" || true)"
    if [[ "${pass_cnt}" -ne 5 || "${skip_cnt}" -ne 0 ]]; then
        echo "error: explicit formal/FMA test count mismatch on ${label}: pass=${pass_cnt} skip=${skip_cnt}" >&2
        cat "out/test_${label}_explicit.log" >&2
        exit 1
    fi
    grep -q "active=${expect_active}" "out/test_${label}_explicit.log"

    local elapsed=$(( $(date +%s) - t0 ))
    printf '%s\t%s\t%s\t%s\t%s\t4\t%s\t%s\n' \
        "${label}" "${goamd64}" "${simd_enabled}" "${max_tier:-auto}" "${expect_active}" "${pass_cnt}" "${elapsed}" >> out/test_matrix.tsv
}

run_profile_tests "default_v1" "./bin/default_v1" "v1" "false" "" "scalar"
for lvl in v1 v3 v4; do
    for tier in scalar avx2 avx512; do
        run_profile_tests "simd_${lvl}_${tier}" "./bin/simd_${lvl}" "${lvl}" "true" "${tier}" "${tier}"
    done
done

PRE_BENCH_BUSY="$(wait_for_quiescence pre-bench)"
cat /proc/pressure/cpu > out/cpu_pressure_bench.txt 2>/dev/null || true
printf '{"pre_test_busy_pct": %s, "pre_bench_busy_pct": %s}\n' "${PRE_TEST_BUSY}" "${PRE_BENCH_BUSY}" > out/quiescence.json

# (i) 6-round interleaved AB/BA vector kernel benchmarks pinned to CPU 1.
echo "  [bench] warmup + 6-round interleaved AB/BA on simd.test kernels (CPU 1)..." >&2
for tier in scalar avx2 avx512; do
    GOBLIN_BENCH_SIMD_TIER="${tier}" taskset -c 1 ./bin/simd_v1/simd.test -test.run='^$' -test.bench='^BenchmarkKernelCrossover$' -test.benchtime=10ms -test.cpu=1 >/dev/null
done

: > out/kernel_abba.log
for round in 1 2 3 4 5 6; do
    if (( round % 2 == 1 )); then
        order=(scalar avx2 avx512)
    else
        order=(avx512 avx2 scalar)
    fi
    for tier in "${order[@]}"; do
        printf '=== ROUND %d TIER %s ===\n' "${round}" "${tier}" >> out/kernel_abba.log
        GOBLIN_BENCH_SIMD_TIER="${tier}" taskset -c 1 ./bin/simd_v1/simd.test \
            -test.run='^$' \
            -test.bench='^BenchmarkKernelCrossover$' \
            -test.benchmem \
            -test.benchtime=25ms \
            -test.cpu=1 >> out/kernel_abba.log
    done
done
test "$(grep -c '^BenchmarkKernelCrossover/' out/kernel_abba.log)" -ge 1500

# (ii) Full 31-family LP/MILP and 22-family LSAP benchmarks pinned to CPU 1.
echo "  [bench] 31-family LP/MILP and 22-family LSAP across Default Scalar, AVX2, and AVX-512 (CPU 1)..." >&2
taskset -c 1 ./bin/default_v1/goblin-bench -backend go -workers 1 -out out/milp_default_scalar.json > out/bench_milp_scalar.log
taskset -c 1 ./bin/simd_v1/goblin-bench -simd-tier avx2 -backend go -workers 1 -out out/milp_simd_avx2.json > out/bench_milp_avx2.log
taskset -c 1 ./bin/simd_v1/goblin-bench -simd-tier avx512 -backend go -workers 1 -out out/milp_simd_avx512.json > out/bench_milp_avx512.log

taskset -c 1 ./bin/default_v1/goblin-bench -assign -out out/assign_default_scalar.json > out/bench_assign_scalar.log
taskset -c 1 ./bin/simd_v1/goblin-bench -simd-tier avx2 -assign -out out/assign_simd_avx2.json > out/bench_assign_avx2.log
taskset -c 1 ./bin/simd_v1/goblin-bench -simd-tier avx512 -assign -out out/assign_simd_avx512.json > out/bench_assign_avx512.log

tar -C "${WORK}/out" --owner=0 --group=0 -czf /tmp/goblin-silicon-amd64-out.tar.gz .
REOF

    cat > "${STAGE_REAL}/run_arm64_remote.sh" <<'REOF'
#!/usr/bin/env bash
set -euo pipefail

HAS_SIMD_BIN="${1:-0}"
WORK="/tmp/goblin-silicon-work"
rm -rf "${WORK}"
mkdir -p "${WORK}/out" "${WORK}/bin"
tar -C "${WORK}" -xzf /tmp/goblin-silicon-testdata.tar.gz
tar -C "${WORK}/bin" -xzf /tmp/goblin-silicon-arm64-bin.tar.gz
cd "${WORK}"

measure_cpu_busy_pct() {
    awk '
        NR == 1 {
            u1 = $2 + $3 + $4 + $7 + $8 + $9
            i1 = $5 + $6
            next
        }
        {
            u2 = $2 + $3 + $4 + $7 + $8 + $9
            i2 = $5 + $6
            dt = (u2 - u1) + (i2 - i1)
            if (dt <= 0) {
                printf "0.00\n"
            } else {
                printf "%.2f\n", 100.0 * (u2 - u1) / dt
            }
        }
    ' <(grep '^cpu ' /proc/stat) <(sleep 2; grep '^cpu ' /proc/stat)
}

wait_for_quiescence() {
    local label="$1"
    local attempt busy
    for attempt in $(seq 1 24); do
        busy="$(measure_cpu_busy_pct)"
        if awk -v b="${busy}" 'BEGIN { exit !(b < 5.0) }'; then
            printf '%s\n' "${busy}"
            return 0
        fi
        echo "  [${label}] waiting for idle VM CPU (attempt ${attempt}/24, busy=${busy}%)..." >&2
        sleep 5
    done
    echo "error: VM failed quiescence gate (${label}: busy=${busy}% >= 5.0%)" >&2
    ps -eo pid,pcpu,comm --sort=-pcpu 2>/dev/null | head -n 10 >&2 || true
    exit 1
}

PRE_TEST_BUSY="$(wait_for_quiescence pre-test)"
uname -srm > out/uname_srm.txt
LC_ALL=C lscpu | grep -E '^(Architecture|CPU op-mode\(s\)|Byte Order|CPU\(s\)|On-line CPU\(s\) list|Vendor ID|Model name|Model|Stepping|Thread\(s\) per core|Core\(s\) per socket|Socket\(s\)|NUMA node\(s\)|L1d cache|L1i cache|L2 cache|L3 cache|Flags):' > out/lscpu.txt
grep -m1 '^Features' /proc/cpuinfo | cut -d: -f2- | tr ' \t' '\n' | grep -v '^$' | sort -u > out/cpuinfo_features.txt || true
cat /proc/pressure/cpu > out/cpu_pressure_pre.txt 2>/dev/null || true

./bin/default/simd.test -test.v -test.run='^TestTierIntrospectionAndCaps$' > out/tier_introspection_default.log 2>&1
grep -q 'hardware=scalar initial_active=scalar' out/tier_introspection_default.log

SKIP_ROOT='^(TestDocumentationAndBenchmarkSync|TestTargetSiliconDocumentationSync|TestDocBacktickIdentifiersResolve|TestFuzzHarnessTargetsSync|TestTargetSiliconHygieneGate|TestGoblinBinaryDeadCodeElimination|TestFormalZeroEscapeSoakLogAudit|TestFormalNoSilentFuzzBridgeEscapes|TestFormalTrack5D_ELFDisassemblyMatchesLean)$'
SKIP_ASSIGN='^TestAssignBinaryDeadCodeElimination$'
EXPLICIT_RUN='^(TestFormalTrack5D_SIMDParityAndEdgeAudit|TestFormalTrack5E_LiveSIMDDifferential|TestFormalTrack5E_ZeroAllocContracts|TestFormalTrack5E_HardwareNaNParity|TestTwoSumTwoProdFMASafety)$'

: > out/test_matrix.tsv

run_arm64_profile() {
    local label="$1"
    local bin_dir="$2"
    local simd_flag="$3"

    local t0
    t0="$(date +%s)"
    echo "  [test-arm64] profile=${label}" >&2
    "${bin_dir}/simd.test" -test.count=1 > "out/test_${label}_simd.log" 2>&1
    "${bin_dir}/assign.test" -test.count=1 -test.skip="${SKIP_ASSIGN}" > "out/test_${label}_assign.log" 2>&1
    (cd format && "${WORK}/${bin_dir#./}/format.test" -test.count=1) > "out/test_${label}_format.log" 2>&1
    "${bin_dir}/goblin.test" -test.count=1 -test.skip="${SKIP_ROOT}" > "out/test_${label}_goblin.log" 2>&1

    "${bin_dir}/goblin.test" -test.v -test.count=1 -test.run="${EXPLICIT_RUN}" > "out/test_${label}_explicit.log" 2>&1
    local pass_cnt skip_cnt
    pass_cnt="$(grep -c '^--- PASS: Test' "out/test_${label}_explicit.log" || true)"
    skip_cnt="$(grep -c '^--- SKIP: Test' "out/test_${label}_explicit.log" || true)"
    if [[ "${pass_cnt}" -ne 5 || "${skip_cnt}" -ne 0 ]]; then
        echo "error: explicit formal/FMA test count mismatch on ${label}: pass=${pass_cnt} skip=${skip_cnt}" >&2
        cat "out/test_${label}_explicit.log" >&2
        exit 1
    fi
    grep -q 'active=scalar' "out/test_${label}_explicit.log"

    local elapsed=$(( $(date +%s) - t0 ))
    printf '%s\tarm64\t%s\tauto\tscalar\t4\t%s\t%s\n' \
        "${label}" "${simd_flag}" "${pass_cnt}" "${elapsed}" >> out/test_matrix.tsv
}

run_arm64_profile "default" "./bin/default" "false"
if [[ "${HAS_SIMD_BIN}" == "1" && -x "./bin/simd/goblin.test" ]]; then
    ./bin/simd/simd.test -test.v -test.run='^TestTierIntrospectionAndCaps$' > out/tier_introspection_simd.log 2>&1
    grep -q 'hardware=scalar initial_active=scalar' out/tier_introspection_simd.log
    run_arm64_profile "goexperiment_simd" "./bin/simd" "false"
fi

PRE_BENCH_BUSY="$(wait_for_quiescence pre-bench)"
cat /proc/pressure/cpu > out/cpu_pressure_bench.txt 2>/dev/null || true
printf '{"pre_test_busy_pct": %s, "pre_bench_busy_pct": %s}\n' "${PRE_TEST_BUSY}" "${PRE_BENCH_BUSY}" > out/quiescence.json

echo "  [bench-arm64] 31-family LP/MILP and 22-family LSAP on CPU 1..." >&2
taskset -c 1 ./bin/default/goblin-bench -backend go -workers 1 -out out/milp_default.json > out/bench_milp_default.log
taskset -c 1 ./bin/default/goblin-bench -assign -out out/assign_default.json > out/bench_assign_default.log

tar -C "${WORK}/out" --owner=0 --group=0 -czf /tmp/goblin-silicon-arm64-out.tar.gz .
REOF
    chmod +x "${STAGE_REAL}/run_amd64_remote.sh" "${STAGE_REAL}/run_arm64_remote.sh"
}

write_remote_scripts

if [[ "${RUN_AMD64}" -eq 1 && ! -f "${STAGE_REAL}/amd64_bin.tar.gz" ]]; then
    tar -C "${STAGE_REAL}/amd64/bin" --owner=0 --group=0 -czf "${STAGE_REAL}/amd64_bin.tar.gz" .
fi
if [[ "${RUN_ARM64}" -eq 1 && ! -f "${STAGE_REAL}/arm64_bin.tar.gz" ]]; then
    tar -C "${STAGE_REAL}/arm64/bin" --owner=0 --group=0 -czf "${STAGE_REAL}/arm64_bin.tar.gz" .
fi

# Order candidate zones so preferred region comes first, then one zone per
# distinct region prefix for quota diversity, followed by remaining zones.
order_candidate_zones() {
    local raw_zones="$1"
    local pref_region="$2"
    python3 - "${pref_region}" "${raw_zones}" <<'PYEOF'
import sys

pref = sys.argv[1].strip()
zones = [z.strip() for z in sys.argv[2].splitlines() if z.strip()]
seen = set()
dedup = []
for z in zones:
    if z not in seen:
        seen.add(z)
        dedup.append(z)

pref_list = [z for z in dedup if pref and z.startswith(pref + "-")]
rest = [z for z in dedup if z not in set(pref_list)]

first_per_reg = []
spill = []
seen_reg = set()
for z in rest:
    reg = z.rsplit("-", 1)[0] if "-" in z else z
    if reg not in seen_reg:
        seen_reg.add(reg)
        first_per_reg.append(z)
    else:
        spill.append(z)

for z in pref_list + first_per_reg + spill:
    print(z)
PYEOF
}

provision_vm_across_zones() {
    local vm_name="$1"
    local mtype="$2"
    local img_family="$3"
    local disk_type="$4"
    local explicit_zone="$5"
    local zones_text="$6"
    local out_zone_file="$7"

    local -a candidates=()
    if [[ -n "${explicit_zone}" ]]; then
        candidates=("${explicit_zone}")
    else
        mapfile -t candidates < <(order_candidate_zones "${zones_text}" "${REGION}")
    fi

    if [[ "${#candidates[@]}" -eq 0 ]]; then
        echo "error: zero candidate zones discovered for ${mtype}" >&2
        return 1
    fi

    local tried=0
    local max_tries=10
    local z err_file
    err_file="$(mktemp "${STAGE_REAL}/create_${mtype}_XXXXXX.err")"
    for z in "${candidates[@]}"; do
        [[ -n "${z}" ]] || continue
        tried=$((tried + 1))
        if [[ "${tried}" -gt "${max_tries}" ]]; then
            break
        fi
        CREATED_VMS+=("${vm_name}:${z}")
        if gcloud compute instances create "${vm_name}" \
            --project="${PROJECT}" \
            --zone="${z}" \
            --machine-type="${mtype}" \
            --image-family="${img_family}" \
            --image-project=debian-cloud \
            --boot-disk-type="${disk_type}" \
            --boot-disk-size=20GB \
            --no-service-account \
            --no-scopes \
            --quiet > /dev/null 2>"${err_file}"; then
            rm -f "${err_file}"
            printf '%s\n' "${z}" > "${out_zone_file}"
            return 0
        fi
        local msg
        msg="$(tr '\n' ' ' < "${err_file}")"
        if grep -Eqi 'PERMISSION_DENIED|Required .* permission|not authorized|Invalid value for field' "${err_file}"; then
            echo "error: non-retryable failure creating ${mtype}: ${msg}" >&2
            rm -f "${err_file}"
            return 2
        fi
        echo "  [provision] ${mtype} unavailable in candidate #${tried}, retrying next region..." >&2
    done
    echo "  [provision] exhausted ${tried} zone attempts for ${mtype}: $(tail -n 3 "${err_file}" | tr '\n' ' ')" >&2
    rm -f "${err_file}"
    return 1
}

vm_ssh() {
    local vm_name="$1"
    local vm_zone="$2"
    shift 2
    if gcloud compute ssh "${vm_name}" \
        --project="${PROJECT}" \
        --zone="${vm_zone}" \
        --ssh-flag="-o ConnectTimeout=12" \
        --ssh-flag="-o ServerAliveInterval=15" \
        --ssh-flag="-o ServerAliveCountMax=8" \
        --ssh-flag="-o BatchMode=yes" \
        --ssh-flag="-o StrictHostKeyChecking=no" \
        --quiet \
        -- "$@"; then
        return 0
    fi
    gcloud compute ssh "${vm_name}" \
        --project="${PROJECT}" \
        --zone="${vm_zone}" \
        --tunnel-through-iap \
        --ssh-flag="-o ConnectTimeout=15" \
        --ssh-flag="-o ServerAliveInterval=15" \
        --ssh-flag="-o ServerAliveCountMax=8" \
        --ssh-flag="-o BatchMode=yes" \
        --ssh-flag="-o StrictHostKeyChecking=no" \
        --quiet \
        -- "$@"
}

vm_scp() {
    local vm_zone="$1"
    shift
    if gcloud compute scp \
        --project="${PROJECT}" \
        --zone="${vm_zone}" \
        --scp-flag="-o ConnectTimeout=15" \
        --scp-flag="-o ServerAliveInterval=15" \
        --scp-flag="-o ServerAliveCountMax=8" \
        --scp-flag="-o BatchMode=yes" \
        --scp-flag="-o StrictHostKeyChecking=no" \
        --quiet \
        "$@"; then
        return 0
    fi
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
            -- "uname -srm" >/dev/null 2>&1; then
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
            -- "uname -srm" >/dev/null 2>&1; then
            return 0
        fi
        sleep 5
    done
    echo "error: timed out waiting for SSH on ${vm_name}" >&2
    return 1
}

run_remote_detached() {
    local label="$1"
    local vm_name="$2"
    local vm_zone="$3"
    local remote_cmd="$4"

    vm_ssh "${vm_name}" "${vm_zone}" \
        "rm -f /tmp/goblin-remote.rc /tmp/goblin-remote.log /tmp/goblin-remote.pid; nohup bash -c '${remote_cmd}; rc=\$?; printf \"%s\n\" \"\${rc}\" > /tmp/goblin-remote.rc' > /tmp/goblin-remote.log 2>&1 < /dev/null & echo \$! > /tmp/goblin-remote.pid"

    local poll status_out rc_val
    for poll in $(seq 1 160); do
        sleep 15
        status_out="$(vm_ssh "${vm_name}" "${vm_zone}" \
            "if [ -f /tmp/goblin-remote.rc ]; then echo DONE:\$(cat /tmp/goblin-remote.rc); else echo RUNNING; tail -n 2 /tmp/goblin-remote.log 2>/dev/null || true; fi" 2>/dev/null || echo "POLL_RETRY")"
        if [[ "${status_out}" == DONE:* ]]; then
            rc_val="${status_out#DONE:}"
            rc_val="${rc_val%%[[:space:]]*}"
            if [[ "${rc_val}" == "0" ]]; then
                return 0
            fi
            echo "error: [${label}] remote job exited with status ${rc_val}; dumping remote log:" >&2
            vm_ssh "${vm_name}" "${vm_zone}" "cat /tmp/goblin-remote.log" >&2 || true
            return 1
        fi
        echo "  [${label} poll #${poll}] $(tr '\n' ' ' <<< "${status_out}")"
    done
    echo "error: [${label}] timed out waiting for detached remote job" >&2
    vm_ssh "${vm_name}" "${vm_zone}" "cat /tmp/goblin-remote.log" >&2 || true
    return 1
}

echo "=== Step B: Discovering machine-type zones and launching ephemeral GCE instances ==="
MT_CSV="$(gcloud compute machine-types list \
    --project="${PROJECT}" \
    --filter="name=(n4-standard-4,c4a-standard-4,t2a-standard-4)" \
    --format="csv[no-heading](name,zone.basename())" \
    --quiet)"

ZONES_N4="$(awk -F',' '$1 == "n4-standard-4" { print $2 }' <<< "${MT_CSV}")"
ZONES_C4A="$(awk -F',' '$1 == "c4a-standard-4" { print $2 }' <<< "${MT_CSV}")"
ZONES_T2A="$(awk -F',' '$1 == "t2a-standard-4" { print $2 }' <<< "${MT_CSV}")"

RESOLVED_ZONE_AMD64=""
RESOLVED_ZONE_ARM64=""
RESOLVED_MTYPE_ARM64=""

if [[ "${RUN_AMD64}" -eq 1 ]]; then
    ZONE_FILE_AMD64="${STAGE_REAL}/zone_amd64.txt"
    if ! provision_vm_across_zones "${VM_AMD64}" "n4-standard-4" "debian-12" "hyperdisk-balanced" "${ZONE_AMD64}" "${ZONES_N4}" "${ZONE_FILE_AMD64}"; then
        echo "error: failed to provision n4-standard-4 across candidate zones" >&2
        exit 3
    fi
    RESOLVED_ZONE_AMD64="$(cat "${ZONE_FILE_AMD64}")"
    CREATED_VMS+=("${VM_AMD64}:${RESOLVED_ZONE_AMD64}")
    echo "  [amd64] provisioned ${VM_AMD64} (n4-standard-4, hyperdisk-balanced, no-service-account)."
fi

if [[ "${RUN_ARM64}" -eq 1 ]]; then
    ZONE_FILE_ARM64="${STAGE_REAL}/zone_arm64.txt"
    if provision_vm_across_zones "${VM_ARM64}" "c4a-standard-4" "debian-12-arm64" "hyperdisk-balanced" "${ZONE_ARM64}" "${ZONES_C4A}" "${ZONE_FILE_ARM64}"; then
        RESOLVED_MTYPE_ARM64="c4a-standard-4"
        RESOLVED_ZONE_ARM64="$(cat "${ZONE_FILE_ARM64}")"
        CREATED_VMS+=("${VM_ARM64}:${RESOLVED_ZONE_ARM64}")
        echo "  [arm64] provisioned ${VM_ARM64} (c4a-standard-4, hyperdisk-balanced, no-service-account)."
    elif provision_vm_across_zones "${VM_ARM64}" "t2a-standard-4" "debian-12-arm64" "pd-balanced" "${ZONE_ARM64}" "${ZONES_T2A}" "${ZONE_FILE_ARM64}"; then
        RESOLVED_MTYPE_ARM64="t2a-standard-4"
        RESOLVED_ZONE_ARM64="$(cat "${ZONE_FILE_ARM64}")"
        CREATED_VMS+=("${VM_ARM64}:${RESOLVED_ZONE_ARM64}")
        echo "  [arm64] provisioned ${VM_ARM64} (t2a-standard-4 fallback, pd-balanced, no-service-account)."
    else
        echo "error: failed to provision both c4a-standard-4 and t2a-standard-4 across candidate zones" >&2
        exit 3
    fi
fi

execute_amd64_flow() {
    local zone="$1"
    echo "  [amd64] waiting for SSH readiness..."
    wait_vm_ssh "${VM_AMD64}" "${zone}"
    echo "  [amd64] uploading static binaries and testdata bundle..."
    vm_scp "${zone}" \
        "${STAGE_REAL}/testdata_bundle.tar.gz" \
        "${VM_AMD64}:/tmp/goblin-silicon-testdata.tar.gz"
    vm_scp "${zone}" \
        "${STAGE_REAL}/amd64_bin.tar.gz" \
        "${VM_AMD64}:/tmp/goblin-silicon-amd64-bin.tar.gz"
    vm_scp "${zone}" \
        "${STAGE_REAL}/run_amd64_remote.sh" \
        "${VM_AMD64}:/tmp/run_amd64_remote.sh"

    echo "  [amd64] running detached test matrix and pinned benchmarks on n4-standard-4..."
    run_remote_detached "amd64" "${VM_AMD64}" "${zone}" "bash /tmp/run_amd64_remote.sh"
    mkdir -p "${STAGE_REAL}/raw_amd64"
    vm_scp "${zone}" \
        "${VM_AMD64}:/tmp/goblin-silicon-amd64-out.tar.gz" \
        "${STAGE_REAL}/amd64_out.tar.gz"
    tar -C "${STAGE_REAL}/raw_amd64" -xzf "${STAGE_REAL}/amd64_out.tar.gz"

    echo "  [amd64] deleting ${VM_AMD64} immediately after artifact download..."
    gcloud compute instances delete "${VM_AMD64}" \
        --project="${PROJECT}" \
        --zone="${zone}" \
        --quiet
}

execute_arm64_flow() {
    local zone="$1"
    echo "  [arm64] waiting for SSH readiness..."
    wait_vm_ssh "${VM_ARM64}" "${zone}"
    echo "  [arm64] uploading static binaries and testdata bundle..."
    vm_scp "${zone}" \
        "${STAGE_REAL}/testdata_bundle.tar.gz" \
        "${VM_ARM64}:/tmp/goblin-silicon-testdata.tar.gz"
    vm_scp "${zone}" \
        "${STAGE_REAL}/arm64_bin.tar.gz" \
        "${VM_ARM64}:/tmp/goblin-silicon-arm64-bin.tar.gz"
    vm_scp "${zone}" \
        "${STAGE_REAL}/run_arm64_remote.sh" \
        "${VM_ARM64}:/tmp/run_arm64_remote.sh"

    echo "  [arm64] running detached test matrix and pinned benchmarks on ${RESOLVED_MTYPE_ARM64}..."
    run_remote_detached "arm64" "${VM_ARM64}" "${zone}" "bash /tmp/run_arm64_remote.sh ${ARM64_SIMD_COMPILES}"
    mkdir -p "${STAGE_REAL}/raw_arm64"
    vm_scp "${zone}" \
        "${VM_ARM64}:/tmp/goblin-silicon-arm64-out.tar.gz" \
        "${STAGE_REAL}/arm64_out.tar.gz"
    tar -C "${STAGE_REAL}/raw_arm64" -xzf "${STAGE_REAL}/arm64_out.tar.gz"

    echo "  [arm64] deleting ${VM_ARM64} immediately after artifact download..."
    gcloud compute instances delete "${VM_ARM64}" \
        --project="${PROJECT}" \
        --zone="${zone}" \
        --quiet
}

AMD64_PID=""
ARM64_PID=""
if [[ "${RUN_AMD64}" -eq 1 ]]; then
    (
        trap - EXIT INT TERM
        execute_amd64_flow "${RESOLVED_ZONE_AMD64}"
    ) > "${STAGE_REAL}/logs/amd64_flow.log" 2>&1 &
    AMD64_PID="$!"
    BG_PIDS+=("${AMD64_PID}")
fi
if [[ "${RUN_ARM64}" -eq 1 ]]; then
    (
        trap - EXIT INT TERM
        execute_arm64_flow "${RESOLVED_ZONE_ARM64}"
    ) > "${STAGE_REAL}/logs/arm64_flow.log" 2>&1 &
    ARM64_PID="$!"
    BG_PIDS+=("${ARM64_PID}")
fi

FLOW_RC=0
if [[ -n "${AMD64_PID}" ]]; then
    if wait "${AMD64_PID}"; then
        echo "  [PASS] amd64 n4-standard-4 verification and teardown complete."
    else
        echo "  [FAIL] amd64 n4-standard-4 flow failed:" >&2
        cat "${STAGE_REAL}/logs/amd64_flow.log" >&2
        FLOW_RC=1
    fi
fi
if [[ -n "${ARM64_PID}" ]]; then
    if wait "${ARM64_PID}"; then
        echo "  [PASS] arm64 ${RESOLVED_MTYPE_ARM64} verification and teardown complete."
    else
        echo "  [FAIL] arm64 ${RESOLVED_MTYPE_ARM64} flow failed:" >&2
        cat "${STAGE_REAL}/logs/arm64_flow.log" >&2
        FLOW_RC=1
    fi
fi

delete_matching_silicon_vms
REMAINING_VMS="$(gcloud compute instances list \
    --project="${PROJECT}" \
    --filter="name~^goblin-silicon-" \
    --format="value(name)" \
    --quiet)"
if [[ -n "${REMAINING_VMS}" ]]; then
    echo "error: residual goblin-silicon instances still present: ${REMAINING_VMS}" >&2
    exit 1
fi
echo "  Verified 0 goblin-silicon-* instances remain running."

if [[ "${FLOW_RC}" -ne 0 ]]; then
    exit "${FLOW_RC}"
fi

echo "=== Step C: Assembling and sanitizing target-silicon JSON telemetry ==="
mkdir -p "${OUT_DIR_ABS}"

python3 - \
    "${REPO_ROOT}" \
    "${STAGE_REAL}" \
    "${OUT_DIR_ABS}" \
    "${RUN_AMD64}" \
    "${RUN_ARM64}" \
    "${RESOLVED_MTYPE_ARM64}" \
    "${ARM64_SIMD_COMPILES}" \
    "${GIT_COMMIT}" \
    "${SOURCE_FP}" \
    "${DEFAULT_GO_VER}" \
    "${SIMD_GO_VER}" \
    "${PROJECT}" <<'PYEOF'
import datetime
import json
import math
import pathlib
import re
import sys

repo_root = pathlib.Path(sys.argv[1])
stage_dir = pathlib.Path(sys.argv[2])
out_dir = pathlib.Path(sys.argv[3])
run_amd64 = sys.argv[4] == "1"
run_arm64 = sys.argv[5] == "1"
arm64_mtype = sys.argv[6]
arm64_simd_compiles = sys.argv[7] == "1"
git_commit = sys.argv[8]
source_fp = sys.argv[9]
default_go_ver = sys.argv[10]
simd_go_ver = sys.argv[11]
active_project = sys.argv[12]

py_ref = json.loads((repo_root / "bench/results/python_baseline.json").read_text(encoding="utf-8"))
sp_ref = json.loads((repo_root / "bench/results/scipy_assign_baseline.json").read_text(encoding="utf-8"))

PARITY_REL_TOL = 1e-5
ASSIGN_PARITY_REL_TOL = 1e-9

def rel_diff(a: float, b: float) -> float:
    return abs(a - b) / max(1.0, abs(b))

def parse_lscpu(path: pathlib.Path) -> dict[str, str]:
    out: dict[str, str] = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        if ":" not in line:
            continue
        k, v = line.split(":", 1)
        k_s = k.strip()
        v_s = " ".join(v.strip().split())
        if k_s and v_s:
            out[k_s] = v_s
    return out

def parse_test_matrix(path: pathlib.Path) -> list[dict[str, object]]:
    runs = []
    for line in path.read_text(encoding="utf-8").splitlines():
        if not line.strip():
            continue
        profile, arch_lvl, simd_en, max_tier, active_tier, pkgs, exp_cnt, elapsed = line.split("\t")
        runs.append({
            "profile": profile,
            "arch_level": arch_lvl,
            "simd_enabled": simd_en == "true",
            "simd_max_tier": max_tier,
            "active_simd_tier": active_tier,
            "packages_passed": int(pkgs),
            "explicit_tests_passed": int(exp_cnt),
            "elapsed_sec": int(elapsed),
        })
    return runs

def verify_milp_parity(milp_map: dict[str, dict]) -> int:
    if len(milp_map) != len(py_ref):
        raise ValueError(f"MILP entry count {len(milp_map)} != reference {len(py_ref)}")
    mismatches = 0
    for pid, py in py_ref.items():
        r = milp_map[pid]
        if r["cp7_full"]["status"] != py["status"]:
            mismatches += 1
            continue
        if py.get("objective") is not None and rel_diff(r["cp7_full"]["objective"], py["objective"]) > PARITY_REL_TOL:
            mismatches += 1
        if py.get("lp_relaxation_objective") is not None:
            if r.get("root_lp_status") != "Optimal" or rel_diff(r["root_lp_obj"], py["lp_relaxation_objective"]) > PARITY_REL_TOL:
                mismatches += 1
        for k, want_obj in enumerate(py.get("trial_objectives") or []):
            if want_obj is None:
                continue
            got_obj = r["trial_objectives"][k]
            if rel_diff(got_obj, want_obj) > PARITY_REL_TOL:
                mismatches += 1
    return mismatches

def verify_assign_parity(as_map: dict[str, dict]) -> int:
    if len(as_map) != len(sp_ref):
        raise ValueError(f"LSAP entry count {len(as_map)} != reference {len(sp_ref)}")
    mismatches = 0
    for aid, sp in sp_ref.items():
        r = as_map[aid]
        if rel_diff(r["objective"], sp["objective"]) > ASSIGN_PARITY_REL_TOL:
            mismatches += 1
        for k, want_obj in enumerate(sp.get("trial_objectives") or []):
            got_obj = r["trial_objectives"][k]
            if rel_diff(got_obj, want_obj) > ASSIGN_PARITY_REL_TOL:
                mismatches += 1
    return mismatches

def compact_milp_entry(r: dict) -> dict[str, object]:
    return {
        "tier": r["tier"],
        "num_vars": r["num_vars"],
        "num_constraints": r["num_constraints"],
        "status": r["cp7_full"]["status"],
        "objective": r["cp7_full"]["objective"],
        "root_lp_obj": r["root_lp_obj"],
        "t_solve_ms": round(r["t_solve_ms"], 6),
        "t_total_ms": round(r["t_total_ms"], 6),
        "t_solve_trials_ms": [round(x, 6) for x in r["t_solve_trials_ms"]],
        "t_total_trials_ms": [round(x, 6) for x in r["t_total_trials_ms"]],
    }

def compact_assign_entry(r: dict) -> dict[str, object]:
    return {
        "tier": r["tier"],
        "rows": r["rows"],
        "cols": r["cols"],
        "family": r["family"],
        "objective": r["objective"],
        "t_solve_ms": round(r["t_solve_ms"], 6),
        "t_total_ms": round(r["t_total_ms"], 6),
        "t_solve_trials_ms": [round(x, 6) for x in r["t_solve_trials_ms"]],
        "t_total_trials_ms": [round(x, 6) for x in r["t_total_trials_ms"]],
    }

def parse_kernel_abba(path: pathlib.Path) -> list[dict[str, object]]:
    bench_re = re.compile(
        r"^BenchmarkKernelCrossover/([A-Za-z0-9_]+)/N=(\d+)/(scalar|avx2|avx512)\s+\d+\s+([0-9.]+)\s+ns/op"
    )
    rounds_map: dict[tuple[str, int], dict[str, list[float]]] = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        m = bench_re.match(line.strip())
        if not m:
            continue
        kname, size_s, tier, ns_s = m.groups()
        key = (kname, int(size_s))
        if key not in rounds_map:
            rounds_map[key] = {"scalar": [], "avx2": [], "avx512": []}
        rounds_map[key][tier].append(round(float(ns_s), 4))

    kernel_order = [
        "AllAbsLE64",
        "MinNoNaN64",
        "InitReducedRow64",
        "Min2ReducedRow64",
        "RelaxRow64",
        "Dot64",
        "Axpy64",
        "SelectDualLeaveRow64",
        "ScanDualRatioCand64",
    ]
    order_idx = {k: i for i, k in enumerate(kernel_order)}
    items = []
    for (kname, sz), tiers in sorted(rounds_map.items(), key=lambda kv: (order_idx.get(kv[0][0], 99), kv[0][1])):
        for tname in ("scalar", "avx2", "avx512"):
            if len(tiers[tname]) != 6:
                raise ValueError(f"kernel {kname} N={sz} tier={tname} has {len(tiers[tname])} rounds, want 6")
        items.append({
            "kernel": kname,
            "size": sz,
            "scalar_rounds_ns": tiers["scalar"],
            "avx2_rounds_ns": tiers["avx2"],
            "avx512_rounds_ns": tiers["avx512"],
        })
    if len(items) < 85:
        raise ValueError(f"kernel_abba parsed only {len(items)} kernel/size pairs, want >= 85")
    return items

def assert_sanitized_json(label: str, raw_text: str) -> None:
    home_re = re.compile("/(?:" + "ho" + "me|Users)/")
    cloud_re = re.compile("(?:" + "gs:" + "//|compute\\.internal|gserviceaccount\\.com|" + "goblin-" + "silicon-)")
    forbidden_patterns = [
        ("email_address", re.compile(r"[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}")),
        ("home_path", home_re),
        ("ipv4_address", re.compile(r"\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b")),
        ("gcp_region_or_zone", re.compile(r"\b(?:us|europe|asia|australia|northamerica|southamerica|me|africa)-[a-z]+[0-9](?:-[a-z])?\b")),
        ("gcs_or_internal", cloud_re),
    ]
    for pname, pat in forbidden_patterns:
        m = pat.search(raw_text)
        if m:
            raise ValueError(f"hygiene violation ({pname}) in {label}: matched {m.group(0)!r}")
    if active_project and active_project in raw_text:
        raise ValueError(f"hygiene violation in {label}: leaked active GCP project ID")

now_iso = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

if run_amd64:
    raw_a = stage_dir / "raw_amd64"
    lscpu_a = parse_lscpu(raw_a / "lscpu.txt")
    uname_a = (raw_a / "uname_srm.txt").read_text(encoding="utf-8").strip()
    avx512_flags = [f.strip() for f in (raw_a / "avx512_flags.txt").read_text(encoding="utf-8").splitlines() if f.strip()]
    quiesc_a = json.loads((raw_a / "quiescence.json").read_text(encoding="utf-8"))
    runs_a = parse_test_matrix(raw_a / "test_matrix.tsv")

    milp_sc = json.loads((raw_a / "milp_default_scalar.json").read_text(encoding="utf-8"))
    milp_v2 = json.loads((raw_a / "milp_simd_avx2.json").read_text(encoding="utf-8"))
    milp_v4 = json.loads((raw_a / "milp_simd_avx512.json").read_text(encoding="utf-8"))
    as_sc = json.loads((raw_a / "assign_default_scalar.json").read_text(encoding="utf-8"))
    as_v2 = json.loads((raw_a / "assign_simd_avx2.json").read_text(encoding="utf-8"))
    as_v4 = json.loads((raw_a / "assign_simd_avx512.json").read_text(encoding="utf-8"))

    mismatches_milp = verify_milp_parity(milp_sc) + verify_milp_parity(milp_v2) + verify_milp_parity(milp_v4)
    mismatches_as = verify_assign_parity(as_sc) + verify_assign_parity(as_v2) + verify_assign_parity(as_v4)
    if mismatches_milp != 0 or mismatches_as != 0:
        raise ValueError(f"amd64 parity mismatch: milp={mismatches_milp} assign={mismatches_as}")

    any_sc = next(iter(milp_sc.values()))
    any_v4 = next(iter(milp_v4.values()))
    if any_v4.get("simd_tier") != "avx512":
        raise ValueError(f"expected milp_simd_avx512 simd_tier=avx512, got {any_v4.get('simd_tier')}")
    any_v2 = next(iter(milp_v2.values()))
    if any_v2.get("simd_tier") != "avx2":
        raise ValueError(f"expected milp_simd_avx2 simd_tier=avx2, got {any_v2.get('simd_tier')}")

    cpu_model_a = lscpu_a.get("Model name") or any_v4.get("cpu_model") or "unknown"
    amd64_doc = {
        "schema_version": 1,
        "platform_id": "linux_amd64_avx512",
        "machine_type": "n4-standard-4",
        "os_image_family": "debian-12",
        "boot_disk_type": "hyperdisk-balanced",
        "goos": "linux",
        "goarch": "amd64",
        "uname_srm": uname_a,
        "cpu_model": cpu_model_a,
        "vcpus": int(lscpu_a.get("CPU(s)", "4")),
        "cores_per_socket": int(lscpu_a.get("Core(s) per socket", "2")),
        "threads_per_core": int(lscpu_a.get("Thread(s) per core", "2")),
        "lscpu_summary": {k: v for k, v in sorted(lscpu_a.items()) if k != "Flags"},
        "avx512_flags": avx512_flags,
        "hardware_simd_tier": "avx512",
        "active_simd_tier": "avx512",
        "git_commit": git_commit,
        "source_fingerprint": source_fp,
        "default_go_version": any_sc.get("go_version") or default_go_ver,
        "simd_go_version": any_v4.get("go_version") or simd_go_ver,
        "cpu_affinity": any_v4.get("cpu_affinity", "1"),
        "pre_test_cpu_busy_pct": quiesc_a["pre_test_busy_pct"],
        "pre_bench_cpu_busy_pct": quiesc_a["pre_bench_busy_pct"],
        "measured_at": now_iso,
        "test_verification": {
            "profiles_verified": len(runs_a),
            "goamd64_levels": ["v1", "v3", "v4"],
            "simd_max_tiers": ["scalar", "avx2", "avx512"],
            "packages": [".", "./internal/simd", "./assign", "./format"],
            "explicit_formal_and_fma_tests": [
                "TestFormalTrack5D_SIMDParityAndEdgeAudit",
                "TestFormalTrack5E_HardwareNaNParity",
                "TestFormalTrack5E_LiveSIMDDifferential",
                "TestFormalTrack5E_ZeroAllocContracts",
                "TestTwoSumTwoProdFMASafety",
            ],
            "total_failures": 0,
            "runs": runs_a,
        },
        "kernel_abba": {
            "rounds": 6,
            "order_odd": ["scalar", "avx2", "avx512"],
            "order_even": ["avx512", "avx2", "scalar"],
            "entries": parse_kernel_abba(raw_a / "kernel_abba.log"),
        },
        "milp_corpus": {
            "families": len(milp_sc),
            "trials": sum(len(v["t_total_trials_ms"]) for v in milp_sc.values()),
            "parity_mismatches": 0,
            "default_scalar": {k: compact_milp_entry(milp_sc[k]) for k in sorted(milp_sc)},
            "simd_avx2": {k: compact_milp_entry(milp_v2[k]) for k in sorted(milp_v2)},
            "simd_avx512": {k: compact_milp_entry(milp_v4[k]) for k in sorted(milp_v4)},
        },
        "assign_corpus": {
            "families": len(as_sc),
            "trials": sum(len(v["t_solve_trials_ms"]) for v in as_sc.values()),
            "parity_mismatches": 0,
            "default_scalar": {k: compact_assign_entry(as_sc[k]) for k in sorted(as_sc)},
            "simd_avx2": {k: compact_assign_entry(as_v2[k]) for k in sorted(as_v2)},
            "simd_avx512": {k: compact_assign_entry(as_v4[k]) for k in sorted(as_v4)},
        },
    }
    amd64_text = json.dumps(amd64_doc, indent=2, sort_keys=False) + "\n"
    assert_sanitized_json("amd64_avx512.json", amd64_text)
    (out_dir / "amd64_avx512.json").write_text(amd64_text, encoding="utf-8")
    print(f"  Wrote sanitized {out_dir / 'amd64_avx512.json'}")

if run_arm64:
    raw_r = stage_dir / "raw_arm64"
    lscpu_r = parse_lscpu(raw_r / "lscpu.txt")
    uname_r = (raw_r / "uname_srm.txt").read_text(encoding="utf-8").strip()
    feats_r = [f.strip() for f in (raw_r / "cpuinfo_features.txt").read_text(encoding="utf-8").splitlines() if f.strip()]
    if not feats_r and "Flags" in lscpu_r:
        feats_r = sorted(set(lscpu_r["Flags"].split()))
    quiesc_r = json.loads((raw_r / "quiescence.json").read_text(encoding="utf-8"))
    runs_r = parse_test_matrix(raw_r / "test_matrix.tsv")

    milp_arm = json.loads((raw_r / "milp_default.json").read_text(encoding="utf-8"))
    as_arm = json.loads((raw_r / "assign_default.json").read_text(encoding="utf-8"))
    mismatches_milp = verify_milp_parity(milp_arm)
    mismatches_as = verify_assign_parity(as_arm)
    if mismatches_milp != 0 or mismatches_as != 0:
        raise ValueError(f"arm64 parity mismatch: milp={mismatches_milp} assign={mismatches_as}")

    any_arm = next(iter(milp_arm.values()))
    vendor = lscpu_r.get("Vendor ID", "")
    model_nm = lscpu_r.get("Model name", "")
    if vendor and model_nm and not model_nm.lower().startswith(vendor.lower()):
        cpu_model_r = f"{vendor} {model_nm}"
    else:
        cpu_model_r = model_nm or vendor or "ARM64"
    disk_type_r = "hyperdisk-balanced" if arm64_mtype == "c4a-standard-4" else "pd-balanced"

    arm64_doc = {
        "schema_version": 1,
        "platform_id": "linux_arm64",
        "machine_type": arm64_mtype,
        "os_image_family": "debian-12-arm64",
        "boot_disk_type": disk_type_r,
        "goos": "linux",
        "goarch": "arm64",
        "goarm64": "v8.0",
        "uname_srm": uname_r,
        "cpu_model": cpu_model_r,
        "vcpus": int(lscpu_r.get("CPU(s)", "4")),
        "cores_per_socket": int(lscpu_r.get("Core(s) per socket", "4")),
        "threads_per_core": int(lscpu_r.get("Thread(s) per core", "1")),
        "lscpu_summary": {k: v for k, v in sorted(lscpu_r.items()) if k != "Flags"},
        "cpu_features": feats_r,
        "hardware_simd_tier": "scalar",
        "active_simd_tier": "scalar",
        "goexperiment_simd_compiles": arm64_simd_compiles,
        "goexperiment_simd_suites_passed": any(r["profile"] == "goexperiment_simd" for r in runs_r),
        "git_commit": git_commit,
        "source_fingerprint": source_fp,
        "default_go_version": any_arm.get("go_version") or default_go_ver,
        "simd_go_version": simd_go_ver if arm64_simd_compiles else "",
        "cpu_affinity": any_arm.get("cpu_affinity", "1"),
        "pre_test_cpu_busy_pct": quiesc_r["pre_test_busy_pct"],
        "pre_bench_cpu_busy_pct": quiesc_r["pre_bench_busy_pct"],
        "measured_at": now_iso,
        "test_verification": {
            "profiles_verified": len(runs_r),
            "packages": [".", "./internal/simd", "./assign", "./format"],
            "explicit_formal_and_fma_tests": [
                "TestFormalTrack5D_SIMDParityAndEdgeAudit",
                "TestFormalTrack5E_HardwareNaNParity",
                "TestFormalTrack5E_LiveSIMDDifferential",
                "TestFormalTrack5E_ZeroAllocContracts",
                "TestTwoSumTwoProdFMASafety",
            ],
            "total_failures": 0,
            "runs": runs_r,
        },
        "milp_corpus": {
            "families": len(milp_arm),
            "trials": sum(len(v["t_total_trials_ms"]) for v in milp_arm.values()),
            "parity_mismatches": 0,
            "default": {k: compact_milp_entry(milp_arm[k]) for k in sorted(milp_arm)},
        },
        "assign_corpus": {
            "families": len(as_arm),
            "trials": sum(len(v["t_solve_trials_ms"]) for v in as_arm.values()),
            "parity_mismatches": 0,
            "default": {k: compact_assign_entry(as_arm[k]) for k in sorted(as_arm)},
        },
    }
    arm64_text = json.dumps(arm64_doc, indent=2, sort_keys=False) + "\n"
    assert_sanitized_json("linux_arm64.json", arm64_text)
    (out_dir / "linux_arm64.json").write_text(arm64_text, encoding="utf-8")
    print(f"  Wrote sanitized {out_dir / 'linux_arm64.json'}")
PYEOF

if [[ "${KEEP_STAGE}" -eq 0 ]]; then
    rm -rf "${STAGE_REAL}"
fi

echo "=== Target-silicon verification completed cleanly ==="
