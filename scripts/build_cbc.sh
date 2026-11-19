#!/usr/bin/env bash
set -euo pipefail

# Build the vendored COIN-OR dependency stack (CoinUtils, Osi, Clp, Cgl) and Cbc
# out-of-tree into third_party/Cbc/dist (gitignored) with release assertions disabled
# (-DNDEBUG) and Go-safe signal handling (no SIGSEGV/SIGABRT/SIGFPE/SIGINT hijacking).

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GOBLIN_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
THIRD_PARTY="${GOBLIN_THIRD_PARTY:-${GOBLIN_ROOT}/third_party}"
CBC_SRC="${THIRD_PARTY}/Cbc"
PREFIX="${GOBLIN_CBC_PREFIX:-${CBC_SRC}/dist}"
CLEAN=0
KEEP_WORK=0
for arg in "$@"; do
    case "${arg}" in
        --clean)
            CLEAN=1
            ;;
        --keep-work)
            KEEP_WORK=1
            ;;
        *)
            echo "usage: $0 [--clean] [--keep-work]" >&2
            exit 2
            ;;
    esac
done

WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/goblin_coinor_build.XXXXXX")"
if [ "${KEEP_WORK}" -eq 0 ]; then
    trap 'rm -rf "${WORK_DIR}"' EXIT
else
    echo "retaining build workspace at ${WORK_DIR}"
fi
JOBS="$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 8)"

CBC_REV="c1c171ec"
CLP_REV="9390927"
if [ -f "${THIRD_PARTY}/VERSIONS" ]; then
    CBC_REV="$(awk -F= '/^Cbc=/{print $2}' "${THIRD_PARTY}/VERSIONS" | tr -d '[:space:]')"
    CLP_REV="$(awk -F= '/^Clp=/{print $2}' "${THIRD_PARTY}/VERSIONS" | tr -d '[:space:]')"
fi

if [ "${CLEAN}" -eq 1 ]; then
    rm -rf "${PREFIX}"
fi

mkdir -p "${WORK_DIR}/build" "${PREFIX}"
export PKG_CONFIG_PATH="${PREFIX}/lib/pkgconfig:${PKG_CONFIG_PATH:-}"

# Ensure ClpSolve disables SIGINT registration by default for cgo safety without mutating third_party in place.
CLP_SRC="${THIRD_PARTY}/Clp"
if ! grep -q "options_\[2\] = 1;" "${CLP_SRC}/src/ClpSolve.cpp"; then
    CLP_SRC="${WORK_DIR}/src/Clp"
    mkdir -p "${WORK_DIR}/src"
    cp -a "${THIRD_PARTY}/Clp" "${CLP_SRC}"
    perl -0777 -pi -e 's/options_\[3\] = 1;/options_[2] = 1;\n  options_[3] = 1;/g' "${CLP_SRC}/src/ClpSolve.cpp"
fi

build_pkg() {
    local name="$1"
    local src_dir="$2"
    shift 2
    mkdir -p "${WORK_DIR}/build/${name}"
    pushd "${WORK_DIR}/build/${name}" >/dev/null
    "${src_dir}/configure" \
        --prefix="${PREFIX}" \
        --disable-static \
        --enable-shared \
        --without-sample \
        --without-netlib \
        --without-miplib3 \
        CXXFLAGS="-O3 -DNDEBUG -fPIC" \
        CFLAGS="-O3 -DNDEBUG -fPIC" \
        "$@"
    while IFS= read -r -d '' cfg; do
        perl -pi -e 's|#define HAVE_EXECINFO_H 1|/* #undef HAVE_EXECINFO_H */|g; s|#define HAVE_SIGNAL_H 1|/* #undef HAVE_SIGNAL_H */|g' "${cfg}"
        perl -pi -e "s|#define CBC_GIT_HASH \".*\"|#define CBC_GIT_HASH \"${CBC_REV}\"|g; s|#define CLP_GIT_HASH \".*\"|#define CLP_GIT_HASH \"${CLP_REV}\"|g" "${cfg}"
    done < <(find . -name "config*.h" -print0 2>/dev/null)
    make -j"${JOBS}"
    make install
    popd >/dev/null
}

build_pkg "CoinUtils" "${THIRD_PARTY}/CoinUtils"
build_pkg "Osi"       "${THIRD_PARTY}/Osi"
build_pkg "Clp"       "${CLP_SRC}"
build_pkg "Cgl"       "${THIRD_PARTY}/Cgl"
build_pkg "Cbc"       "${CBC_SRC}" --enable-cbc-parallel

echo "COIN-OR CBC stack installed to ${PREFIX}"
