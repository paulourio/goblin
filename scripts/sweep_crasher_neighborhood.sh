#!/usr/bin/env bash
set -euo pipefail

# Sweep +/- N parameter and byte neighborhood variants around a Go fuzz crasher seed
# in an isolated temporary repository overlay outside the git working tree.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GOBLIN_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
TARGETS_TSV="${GOBLIN_ROOT}/scripts/gcp-fuzz/targets.tsv"

TARGET=""
SEED_PATH=""
RADIUS="2"
WORK_DIR=""
PKG=""
BUILD_TAGS=""
RACE=0
FORMAL="${GOBLIN_FORMAL:-0}"
SIMD_MAX_TIER="${GOBLIN_SIMD_MAX_TIER:-}"
MAX_VARIANTS="1024"
KEEP=0
VERBOSE=0

usage() {
    cat >&2 <<'EOF'
usage: sweep_crasher_neighborhood.sh --target=<FuzzTarget> --seed=<seed-file> [options]
  --target=NAME           Go fuzz target function name (required)
  --seed=PATH             Path to Go 'go test fuzz v1' seed file (required)
  --radius=N              Byte/scalar perturbation radius +/- N (default: 2)
  --work-dir=DIR          Scratch directory outside repo (default: mktemp -d)
  --pkg=PKG               Package path (., ./assign, ./format, ./internal/simd; auto-detected from targets.tsv)
  --tags=TAGS             Extra go test -tags (auto-detected from targets.tsv when omitted)
  --race                  Run go test with -race (auto-detected from targets.tsv when omitted)
  --formal                Export GOBLIN_FORMAL=1 for Lean oracle verification
  --simd-max-tier=TIER    Set GOBLIN_SIMD_MAX_TIER (auto|scalar|avx2|avx512)
  --max-variants=N        Maximum generated variants (default: 1024)
  --verbose               Pass -v to go test
  --keep                  Keep scratch work directory after completion
EOF
}

for arg in "$@"; do
    case "${arg}" in
        --target=*) TARGET="${arg#*=}" ;;
        --seed=*) SEED_PATH="${arg#*=}" ;;
        --radius=*) RADIUS="${arg#*=}" ;;
        --work-dir=*) WORK_DIR="${arg#*=}" ;;
        --pkg=*) PKG="${arg#*=}" ;;
        --tags=*) BUILD_TAGS="${arg#*=}" ;;
        --race) RACE=1 ;;
        --formal|--formal=1|--formal=true) FORMAL="1" ;;
        --formal=0|--formal=false) FORMAL="0" ;;
        --simd-max-tier=*) SIMD_MAX_TIER="${arg#*=}" ;;
        --max-variants=*) MAX_VARIANTS="${arg#*=}" ;;
        --verbose|-v) VERBOSE=1 ;;
        --keep) KEEP=1 ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "ERROR: unknown option ${arg}" >&2
            usage
            exit 2
            ;;
    esac
done

if [[ -z "${TARGET}" || -z "${SEED_PATH}" ]]; then
    echo "ERROR: --target and --seed are required." >&2
    usage
    exit 2
fi
if [[ ! -f "${SEED_PATH}" ]]; then
    echo "ERROR: seed file not found: ${SEED_PATH}" >&2
    exit 2
fi
if ! [[ "${RADIUS}" =~ ^[0-9]+$ ]] || [[ "${RADIUS}" -lt 1 ]]; then
    echo "ERROR: --radius must be a positive integer, got ${RADIUS}" >&2
    exit 2
fi
if ! [[ "${MAX_VARIANTS}" =~ ^[0-9]+$ ]] || [[ "${MAX_VARIANTS}" -lt 1 ]]; then
    echo "ERROR: --max-variants must be a positive integer, got ${MAX_VARIANTS}" >&2
    exit 2
fi

if [[ -n "${SIMD_MAX_TIER}" ]]; then
    case "${SIMD_MAX_TIER}" in
        auto|scalar|avx2|avx512) export GOBLIN_SIMD_MAX_TIER="${SIMD_MAX_TIER}" ;;
        *)
            echo "ERROR: invalid --simd-max-tier=${SIMD_MAX_TIER}" >&2
            exit 2
            ;;
    esac
fi

if [[ "${FORMAL}" == "1" ]]; then
    export GOBLIN_FORMAL=1
    if ! command -v lake >/dev/null 2>&1 && [[ -n "${HOME:-}" && -x "${HOME}/.elan/bin/lake" ]]; then
        export PATH="${HOME}/.elan/bin:${PATH}"
    fi
fi

# Resolve package, build tags, and -race automatically from targets.tsv when omitted.
if [[ -f "${TARGETS_TSV}" ]]; then
    TSV_ROW="$(awk -F'\t' -v tgt="${TARGET}" 'NR > 1 && $0 !~ /^#/ && $4 == tgt { print; exit }' "${TARGETS_TSV}")"
    if [[ -n "${TSV_ROW}" ]]; then
        IFS=$'\t' read -r _ TSV_BIN _ _ _ _ <<< "${TSV_ROW}"
        if [[ -z "${PKG}" ]]; then
            case "${TSV_BIN}" in
                */assign.test) PKG="./assign" ;;
                */simd.test) PKG="./internal/simd" ;;
                */format.test|*/format_cbc.test) PKG="./format" ;;
                *) PKG="." ;;
            esac
        fi
        if [[ -z "${BUILD_TAGS}" ]]; then
            case "${TSV_BIN}" in
                *_cbc.test) BUILD_TAGS="cbc" ;;
                *goblin_assert.test) BUILD_TAGS="goblin_assert" ;;
            esac
        fi
        if [[ "${TSV_BIN}" == *goblin_race.test ]]; then
            RACE=1
        fi
    fi
fi
PKG="${PKG:-.}"

CREATED_WORK_DIR=0
if [[ -z "${WORK_DIR}" ]]; then
    WORK_DIR="$(mktemp -d)"
    CREATED_WORK_DIR=1
else
    mkdir -p "${WORK_DIR}"
fi

ABS_ROOT="$(cd "${GOBLIN_ROOT}" && pwd -P)"
ABS_WORK="$(cd "${WORK_DIR}" && pwd -P)"
case "${ABS_WORK}/" in
    "${ABS_ROOT}/"*)
        echo "ERROR: --work-dir (${ABS_WORK}) must be outside repository root (${ABS_ROOT})." >&2
        exit 2
        ;;
esac

cleanup() {
    if [[ "${KEEP}" -eq 0 && "${CREATED_WORK_DIR}" -eq 1 ]]; then
        rm -rf "${ABS_WORK}"
    fi
}
trap cleanup EXIT

OVERLAY_REPO="${ABS_WORK}/repo"
rm -rf "${OVERLAY_REPO}"
mkdir -p "${OVERLAY_REPO}"

PKG_SUBDIR=""
case "${PKG}" in
    .) PKG_SUBDIR="" ;;
    ./*) PKG_SUBDIR="${PKG#./}" ;;
    *) PKG_SUBDIR="${PKG}" ;;
esac

# Symlink repository entries into the scratch overlay without altering the git worktree.
for entry in "${ABS_ROOT}"/* "${ABS_ROOT}"/.[!.]*; do
    [[ -e "${entry}" || -L "${entry}" ]] || continue
    base="$(basename "${entry}")"
    case "${base}" in
        .git|.cache|testdata) continue ;;
    esac
    if [[ -n "${PKG_SUBDIR}" && "${base}" == "${PKG_SUBDIR%%/*}" ]]; then
        continue
    fi
    ln -s "${entry}" "${OVERLAY_REPO}/${base}"
done

if [[ -z "${PKG_SUBDIR}" ]]; then
    mkdir -p "${OVERLAY_REPO}/testdata/fuzz/${TARGET}"
    if [[ -d "${ABS_ROOT}/testdata" ]]; then
        for td in "${ABS_ROOT}/testdata"/*; do
            [[ -e "${td}" || -L "${td}" ]] || continue
            tdbase="$(basename "${td}")"
            [[ "${tdbase}" == "fuzz" ]] && continue
            ln -s "${td}" "${OVERLAY_REPO}/testdata/${tdbase}"
        done
    fi
    TARGET_FUZZ_DIR="${OVERLAY_REPO}/testdata/fuzz/${TARGET}"
else
    # Mirror intermediate parent directories down to PKG_SUBDIR so package testdata remains isolated.
    cur_src="${ABS_ROOT}"
    cur_dst="${OVERLAY_REPO}"
    IFS='/' read -r -a parts <<< "${PKG_SUBDIR}"
    for ((i = 0; i < ${#parts[@]}; i++)); do
        part="${parts[$i]}"
        mkdir -p "${cur_dst}/${part}"
        if ((i < ${#parts[@]} - 1)); then
            next_part="${parts[$((i + 1))]}"
            for entry in "${cur_src}/${part}"/*; do
                [[ -e "${entry}" || -L "${entry}" ]] || continue
                ebase="$(basename "${entry}")"
                [[ "${ebase}" == "${next_part}" ]] && continue
                ln -s "${entry}" "${cur_dst}/${part}/${ebase}"
            done
        else
            for entry in "${cur_src}/${part}"/*; do
                [[ -e "${entry}" || -L "${entry}" ]] || continue
                ebase="$(basename "${entry}")"
                [[ "${ebase}" == "testdata" ]] && continue
                ln -s "${entry}" "${cur_dst}/${part}/${ebase}"
            done
            mkdir -p "${cur_dst}/${part}/testdata/fuzz/${TARGET}"
            if [[ -d "${cur_src}/${part}/testdata" ]]; then
                for td in "${cur_src}/${part}/testdata"/*; do
                    [[ -e "${td}" || -L "${td}" ]] || continue
                    tdbase="$(basename "${td}")"
                    [[ "${tdbase}" == "fuzz" ]] && continue
                    ln -s "${td}" "${cur_dst}/${part}/testdata/${tdbase}"
                done
            fi
            TARGET_FUZZ_DIR="${cur_dst}/${part}/testdata/fuzz/${TARGET}"
        fi
        cur_src="${cur_src}/${part}"
        cur_dst="${cur_dst}/${part}"
    done
fi

VARIANT_COUNT="$(python3 - "${SEED_PATH}" "${TARGET_FUZZ_DIR}" "${RADIUS}" "${MAX_VARIANTS}" <<'PYEOF'
import pathlib
import re
import sys

seed_path = pathlib.Path(sys.argv[1])
out_dir = pathlib.Path(sys.argv[2])
radius = int(sys.argv[3])
max_variants = int(sys.argv[4])

lines = seed_path.read_text(encoding="utf-8").splitlines()
if not lines or lines[0].strip() != "go test fuzz v1":
    raise SystemExit(f"invalid Go fuzz corpus header in {seed_path}")

params = [ln for ln in lines[1:] if ln.strip()]

INT_BOUNDS = {
    "int8": (-128, 127),
    "int16": (-32768, 32767),
    "int32": (-2147483648, 2147483647),
    "rune": (-2147483648, 2147483647),
    "int64": (-(1 << 63), (1 << 63) - 1),
    "int": (-(1 << 63), (1 << 63) - 1),
    "uint8": (0, 255),
    "byte": (0, 255),
    "uint16": (0, 65535),
    "uint32": (0, 4294967295),
    "uint64": (0, (1 << 64) - 1),
    "uint": (0, (1 << 64) - 1),
}

SIMPLE_ESCAPES = {
    "a": b"\a",
    "b": b"\b",
    "f": b"\f",
    "n": b"\n",
    "r": b"\r",
    "t": b"\t",
    "v": b"\v",
    "\\": b"\\",
    '"': b'"',
    "'": b"'",
}

def decode_go_quoted(lit: str) -> bytes:
    if len(lit) >= 2 and lit[0] == "`" and lit[-1] == "`":
        return lit[1:-1].encode("utf-8")
    if len(lit) < 2 or lit[0] != '"' or lit[-1] != '"':
        raise ValueError(f"unsupported Go string literal: {lit!r}")
    body = lit[1:-1]
    out = bytearray()
    i = 0
    while i < len(body):
        ch = body[i]
        if ch != "\\":
            out.extend(ch.encode("utf-8"))
            i += 1
            continue
        i += 1
        if i >= len(body):
            raise ValueError("dangling escape in Go literal")
        esc = body[i]
        i += 1
        if esc in SIMPLE_ESCAPES:
            out.extend(SIMPLE_ESCAPES[esc])
        elif esc == "x":
            out.append(int(body[i : i + 2], 16))
            i += 2
        elif esc == "u":
            out.extend(chr(int(body[i : i + 4], 16)).encode("utf-8"))
            i += 4
        elif esc == "U":
            out.extend(chr(int(body[i : i + 8], 16)).encode("utf-8"))
            i += 8
        elif "0" <= esc <= "7":
            oct_digits = esc + body[i : i + 2]
            out.append(int(oct_digits, 8))
            i += 2
        else:
            raise ValueError(f"unsupported escape \\{esc} in Go literal")
    return bytes(out)

def encode_go_bytes(data: bytes) -> str:
    parts = ['"']
    for b in data:
        if b == 0x22:
            parts.append('\\"')
        elif b == 0x5C:
            parts.append("\\\\")
        elif 0x20 <= b <= 0x7E:
            parts.append(chr(b))
        else:
            parts.append(f"\\x{b:02x}")
    parts.append('"')
    return "".join(parts)

def emit(variant_idx: int, p_lines: list[str]) -> int:
    name = f"sweep_{variant_idx:04d}"
    (out_dir / name).write_text("go test fuzz v1\n" + "\n".join(p_lines) + "\n", encoding="utf-8")
    return variant_idx + 1

count = emit(0, params)

for p_idx, raw in enumerate(params):
    if count >= max_variants:
        break
    m = re.match(r"^([a-zA-Z0-9_\[\]]+)\((.*)\)$", raw.strip())
    if not m:
        continue
    typ, inner = m.group(1), m.group(2).strip()

    if typ in INT_BOUNDS:
        lo, hi = INT_BOUNDS[typ]
        if (typ in ("byte", "rune")) and len(inner) >= 2 and inner[0] == "'" and inner[-1] == "'":
            dec = decode_go_quoted('"' + inner[1:-1].replace('"', '\\"') + '"')
            base_val = ord(dec.decode("utf-8", errors="replace")[0]) if typ == "rune" else dec[0]
        else:
            base_val = int(inner, 0)
        for delta in range(-radius, radius + 1):
            if delta == 0 or count >= max_variants:
                continue
            cand = min(hi, max(lo, base_val + delta))
            if cand == base_val:
                continue
            mut = list(params)
            mut[p_idx] = f"{typ}({cand})"
            count = emit(count, mut)
    elif typ in ("float32", "float64"):
        base_f = float(inner)
        for delta in range(-radius, radius + 1):
            if delta == 0 or count >= max_variants:
                continue
            mut = list(params)
            mut[p_idx] = f"{typ}({base_f + float(delta)!r})"
            count = emit(count, mut)
    elif typ == "[]byte":
        buf = decode_go_quoted(inner)
        if not buf:
            for d in range(1, radius + 1):
                if count >= max_variants:
                    break
                mut = list(params)
                mut[p_idx] = f"[]byte({encode_go_bytes(bytes([d & 0xFF]))})"
                count = emit(count, mut)
            continue
        offsets = list(range(len(buf)))
        budget = max(1, (max_variants - count) // max(1, 2 * radius))
        if len(offsets) > budget:
            head = offsets[: min(32, len(offsets))]
            tail = offsets[max(32, len(offsets) - 32) :]
            rem_budget = max(1, budget - len(head) - len(tail))
            mid = offsets[32 : max(32, len(offsets) - 32)]
            step = max(1, len(mid) // rem_budget)
            offsets = sorted(set(head + mid[::step] + tail))
        for b_idx in offsets:
            orig_b = buf[b_idx]
            for delta in range(-radius, radius + 1):
                if delta == 0 or count >= max_variants:
                    continue
                nb = (orig_b + delta) & 0xFF
                bmut = bytearray(buf)
                bmut[b_idx] = nb
                mut = list(params)
                mut[p_idx] = f"[]byte({encode_go_bytes(bytes(bmut))})"
                count = emit(count, mut)

print(count)
PYEOF
)"

echo "=== Crasher Neighborhood Sweep ==="
echo "  Target:   ${TARGET} (${PKG})"
echo "  Seed:     ${SEED_PATH}"
echo "  Radius:   +/- ${RADIUS} (${VARIANT_COUNT} variants)"
echo "  Work dir: ${ABS_WORK}"

TEST_CMD=(go test -count=1 "-run=^${TARGET}/sweep_")
if [[ "${VERBOSE}" -eq 1 ]]; then
    TEST_CMD+=("-v")
fi
if [[ -n "${BUILD_TAGS}" ]]; then
    TEST_CMD+=("-tags=${BUILD_TAGS}")
fi
if [[ "${RACE}" -eq 1 ]]; then
    TEST_CMD+=("-race")
fi
TEST_CMD+=("${PKG}")

cd "${OVERLAY_REPO}"
TEST_LOG="${ABS_WORK}/sweep.log"
set +e
GOEXPERIMENT=simd "${TEST_CMD[@]}" 2>&1 | tee "${TEST_LOG}"
RC="${PIPESTATUS[0]}"
set -e
if [[ "${RC}" -ne 0 ]]; then
    echo "=== Sweep Failed (rc=${RC}, log=${TEST_LOG}) ===" >&2
    exit "${RC}"
fi
if grep -q 'testing: warning: no tests to run' "${TEST_LOG}"; then
    echo "ERROR: target ${TARGET} matched no fuzz tests in ${PKG}." >&2
    exit 1
fi
echo "=== Sweep Passed (${VARIANT_COUNT} variants) ==="
