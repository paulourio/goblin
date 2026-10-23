#!/usr/bin/env bash
set -euo pipefail

# Pre-commit quality gate for Goblin.
# To install: ln -sf ../../scripts/pre-commit.sh .git/hooks/pre-commit

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "${REPO_ROOT}"

log_step() {
    printf '\n==> %s\n' "$1"
}

fail() {
    printf 'ERROR: %s\n' "$1" >&2
    exit 1
}

# 1. Repository Hygiene & Portability Gate
log_step "1/6 Checking repository hygiene, line endings, and path portability..."

# Ensure no cache directories or compiled artifacts exist in the working tree.
bad_artifacts="$(find . -maxdepth 3 \( -name '.mypy_cache' -o -name '.ruff_cache' -o -name '__pycache__' -o -name '*.prof' -o -name '*.test' \) -not -path './.git/*' -not -path './third_party/*' || true)"
if [ -n "${bad_artifacts}" ]; then
    printf '%s\n' "${bad_artifacts}" >&2
    fail "Forbidden cache or build artifacts found in working tree."
fi

# Ensure no CRLF line endings in source or documentation files.
if grep -rnI --exclude-dir=.git --exclude-dir=third_party $'\r' . >/dev/null 2>&1; then
    grep -rnI --exclude-dir=.git --exclude-dir=third_party $'\r' . >&2 || true
    fail "Files with CRLF line endings detected. Normalize to LF."
fi

# Ensure zero references to legacy package name, unprefixed module path, or machine-specific home paths.
legacy_pkg="g""opt"
unprefixed_import='"gob'"lin"'(/|")'
forbidden_pattern="(${legacy_pkg}|${unprefixed_import}|${HOME})"
if grep -rnI --exclude-dir=.git --exclude-dir=third_party -E "${forbidden_pattern}" . >/dev/null 2>&1; then
    grep -rnI --exclude-dir=.git --exclude-dir=third_party -E "${forbidden_pattern}" . >&2 || true
    fail "Forbidden legacy identifier, unprefixed module import, or hardcoded home directory path detected."
fi

# 2. Go Formatting Gate
log_step "2/6 Checking Go formatting (gofmt)..."
unformatted="$(gofmt -l .)"
if [ -n "${unformatted}" ]; then
    printf '%s\n' "${unformatted}" >&2
    fail "Unformatted Go files detected. Run 'gofmt -w .'."
fi

# 3. Go Linter Gate (Default, SIMD, CGO_ENABLED=0)
log_step "3/6 Running golangci-lint across all build configurations..."
if ! command -v golangci-lint >/dev/null 2>&1; then
    fail "golangci-lint is required but not installed in PATH."
fi
golangci-lint run ./...
GOEXPERIMENT=simd golangci-lint run ./...
CGO_ENABLED=0 golangci-lint run ./...

# 4. Python Linter & Static Type Check Gate
log_step "4/6 Running Python linters (ruff & mypy) on bench/..."
if ! command -v ruff >/dev/null 2>&1; then
    fail "ruff is required but not installed in PATH."
fi
ruff check --no-cache bench/
ruff format --check --no-cache bench/

if command -v mypy >/dev/null 2>&1; then
    mypy --no-incremental --cache-dir=/dev/null --ignore-missing-imports bench/
elif command -v uvx >/dev/null 2>&1; then
    uvx mypy --no-incremental --cache-dir=/dev/null --ignore-missing-imports bench/
else
    fail "Neither 'mypy' nor 'uvx' is available in PATH to type-check bench/."
fi

# 5. Documentation Writing Quality Gate & Canonical Telemetry Sync
log_step "5/6 Verifying documentation quality gate and canonical telemetry sync..."
QUALITY_GATE_BIN="${QUALITY_GATE_BIN:-${HOME}/.gemini/config/skills/writing/bin/quality_gate}"
if [ -x "${QUALITY_GATE_BIN}" ]; then
    "${QUALITY_GATE_BIN}" --profile rfc --level strict --format compact \
        README.md docs/*.md bench/results/final_report.md
elif command -v quality_gate >/dev/null 2>&1; then
    quality_gate --profile rfc --level strict --format compact \
        README.md docs/*.md bench/results/final_report.md
else
    fail "writing quality_gate binary not found at ${QUALITY_GATE_BIN} or in PATH."
fi

go run ./cmd/goblin-bench -check-docs

# 6. Go Test Suite Gate (Default, SIMD, noasm, CGO_ENABLED=0)
log_step "6/6 Running Go test suites across all build configurations..."
go test ./...
GOEXPERIMENT=simd go test ./...
go test -tags=noasm ./...
CGO_ENABLED=0 go test ./...

printf '\nAll pre-commit quality gates passed.\n'
