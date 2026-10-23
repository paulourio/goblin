#!/usr/bin/env bash
set -euo pipefail

# Build the vendored COIN-OR dependency stack (CoinUtils, Osi, Clp, Cgl) and Cbc
# out-of-tree into third_party/Cbc/dist (which is gitignored) with Go-safe signal
# handling (no SIGSEGV/SIGABRT/SIGFPE/SIGINT hijacking).

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GOBLIN_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
THIRD_PARTY="${GOBLIN_ROOT}/third_party"
CBC_SRC="${THIRD_PARTY}/Cbc"
PREFIX="${CBC_SRC}/dist"
WORK_DIR="${TMPDIR:-/tmp}/goblin_coinor_build"
JOBS="$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 8)"

if [ "${1:-}" = "--clean" ]; then
    rm -rf "${WORK_DIR}" "${PREFIX}"
fi

mkdir -p "${WORK_DIR}/build" "${PREFIX}"
export PKG_CONFIG_PATH="${PREFIX}/lib/pkgconfig:${PKG_CONFIG_PATH:-}"

# Ensure ClpSolve disables SIGINT registration by default for cgo safety.
if ! grep -q "options_\[2\] = 1;" "${THIRD_PARTY}/Clp/src/ClpSolve.cpp"; then
    perl -0777 -pi -e 's/options_\[3\] = 1;/options_[2] = 1;\n  options_[3] = 1;/g' "${THIRD_PARTY}/Clp/src/ClpSolve.cpp"
fi

build_pkg() {
    local name="$1"
    local src_dir="$2"
    shift 2
    mkdir -p "${WORK_DIR}/build/${name}"
    pushd "${WORK_DIR}/build/${name}" >/dev/null
    if [ ! -f Makefile ] || ! grep -q -- "${PREFIX}" Makefile 2>/dev/null; then
        rm -rf "${WORK_DIR}/build/${name}"/*
        "${src_dir}/configure" \
            --prefix="${PREFIX}" \
            --disable-static \
            --enable-shared \
            --without-sample \
            --without-netlib \
            --without-miplib3 \
            CXXFLAGS="-O3 -fPIC" \
            CFLAGS="-O3 -fPIC" \
            "$@"
    fi
    while IFS= read -r -d '' cfg; do
        perl -pi -e 's|#define HAVE_EXECINFO_H 1|/* #undef HAVE_EXECINFO_H */|g; s|#define HAVE_SIGNAL_H 1|/* #undef HAVE_SIGNAL_H */|g' "${cfg}"
    done < <(find . -name "config.h" -print0 2>/dev/null)
    make -j"${JOBS}"
    make install
    popd >/dev/null
}

build_pkg "CoinUtils" "${THIRD_PARTY}/CoinUtils"
build_pkg "Osi"       "${THIRD_PARTY}/Osi"
build_pkg "Clp"       "${THIRD_PARTY}/Clp"
build_pkg "Cgl"       "${THIRD_PARTY}/Cgl"
build_pkg "Cbc"       "${CBC_SRC}" --enable-cbc-parallel

echo "COIN-OR CBC stack installed to ${PREFIX}"
