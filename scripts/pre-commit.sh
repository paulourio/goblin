#!/usr/bin/env bash
set -euo pipefail

# Pre-commit quality gate for Goblin.
# To install: ln -sf ../../scripts/pre-commit.sh .git/hooks/pre-commit

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "${REPO_ROOT}"

TMP_DIR="$(mktemp -d)"
declare -a ACTIVE_PIDS=()
declare -a JOB_NAMES=()
declare -a JOB_PIDS=()

cleanup() {
    local pid
    for pid in "${ACTIVE_PIDS[@]:-}"; do
        if [ -n "${pid}" ] && kill -0 "${pid}" 2>/dev/null; then
            kill "${pid}" 2>/dev/null || true
        fi
    done
    rm -rf "${TMP_DIR}"
}
trap cleanup EXIT INT TERM

log_step() {
    printf '\n==> %s\n' "$1"
}

fail() {
    printf 'ERROR: %s\n' "$1" >&2
    exit 1
}

# Launch a named job in the background with isolated log capture.
start_job() {
    local name="$1"
    shift
    local log_file="${TMP_DIR}/${name}.log"
    ( "$@" ) >"${log_file}" 2>&1 &
    local pid=$!
    ACTIVE_PIDS+=("${pid}")
    JOB_NAMES+=("${name}")
    JOB_PIDS+=("${pid}")
}

# Wait for all queued background jobs and dump logs for any failures.
wait_jobs() {
    local failed=0
    local idx
    for idx in "${!JOB_PIDS[@]}"; do
        local name="${JOB_NAMES[$idx]}"
        local pid="${JOB_PIDS[$idx]}"
        local log_file="${TMP_DIR}/${name}.log"
        if wait "${pid}"; then
            printf '  [PASS] %s\n' "${name}"
        else
            printf '  [FAIL] %s\n' "${name}" >&2
            printf '  --- %s output ---\n' "${name}" >&2
            cat "${log_file}" >&2
            printf '  --- end %s ---\n' "${name}" >&2
            failed=1
        fi
    done
    ACTIVE_PIDS=()
    JOB_NAMES=()
    JOB_PIDS=()
    if [ "${failed}" -ne 0 ]; then
        fail "One or more parallel pre-commit gates failed."
    fi
}

# 1. Repository Hygiene & Portability Gate
log_step "1/4 Checking repository hygiene, line endings, and path portability..."

# Ensure no cache directories or compiled artifacts exist in the working tree.
bad_artifacts="$(find . \( -name '.mypy_cache' -o -name '.ruff_cache' -o -name '__pycache__' -o -name '*.prof' -o -name '*.test' \) -not -path './.git/*' -not -path './third_party/*' || true)"
if [ -n "${bad_artifacts}" ]; then
    printf '%s\n' "${bad_artifacts}" >&2
    fail "Forbidden cache or build artifacts found in working tree."
fi

# Ensure no CRLF line endings in source or documentation files.
if grep -rnI --exclude-dir=.git --exclude-dir=.cache --exclude-dir=third_party $'\r' . >/dev/null 2>&1; then
    grep -rnI --exclude-dir=.git --exclude-dir=.cache --exclude-dir=third_party $'\r' . >&2 || true
    fail "Files with CRLF line endings detected. Normalize to LF."
fi

# Ensure zero references to legacy package name, unprefixed module path, unaliased root import/qualifier, or machine-specific home paths.
legacy_pkg="g""opt"
unprefixed_import='"gob'"lin"'(/|")'
unaliased_root_import='^[[:space:]]*"github\.com/paulourio/gob'"lin"'"'
unaliased_pkg_qual='gob'"lin"'\.[A-Z]'
forbidden_pattern="(${legacy_pkg}|${unprefixed_import}|${unaliased_root_import}|${unaliased_pkg_qual})"
if grep -rnI --exclude-dir=.git --exclude-dir=.cache --exclude-dir=third_party -E "${forbidden_pattern}" . >/dev/null 2>&1; then
    grep -rnI --exclude-dir=.git --exclude-dir=.cache --exclude-dir=third_party -E "${forbidden_pattern}" . >&2 || true
    fail "Forbidden legacy identifier or unprefixed module import detected."
fi
if [ -n "${HOME:-}" ] && grep -rnIF --exclude-dir=.git --exclude-dir=.cache --exclude-dir=third_party -- "${HOME}" . >/dev/null 2>&1; then
    grep -rnIF --exclude-dir=.git --exclude-dir=.cache --exclude-dir=third_party -- "${HOME}" . >&2 || true
    fail "Hardcoded home directory path detected."
fi

# 2. Go Formatting Gate
log_step "2/4 Checking Go formatting (gofmt)..."
unformatted="$(gofmt -l .)"
if [ -n "${unformatted}" ]; then
    printf '%s\n' "${unformatted}" >&2
    fail "Unformatted Go files detected. Run 'gofmt -w .'."
fi

# 3. Parallel Static Analysis, Python Lint, Prose Quality Gate & Doc Sync
log_step "3/4 Running Go linters, Python checks, prose gate, and doc sync in parallel..."
if ! command -v golangci-lint >/dev/null 2>&1; then
    fail "golangci-lint is required but not installed in PATH."
fi
if ! command -v ruff >/dev/null 2>&1; then
    fail "ruff is required but not installed in PATH."
fi

HAS_CBC=0
if [ -d "third_party/Cbc/dist/lib" ]; then
    HAS_CBC=1
fi

LINT_CACHE_ROOT="${XDG_CACHE_HOME:-${HOME}/.cache}/golangci-lint-goblin"
mkdir -p "${LINT_CACHE_ROOT}/default" "${LINT_CACHE_ROOT}/simd" "${LINT_CACHE_ROOT}/cgo0" "${LINT_CACHE_ROOT}/cbc"

start_job "golangci-default" env "GOLANGCI_LINT_CACHE=${LINT_CACHE_ROOT}/default" golangci-lint run --allow-parallel-runners ./...
start_job "golangci-simd" env GOEXPERIMENT=simd "GOLANGCI_LINT_CACHE=${LINT_CACHE_ROOT}/simd" golangci-lint run --allow-parallel-runners ./...
start_job "golangci-cgo0" env CGO_ENABLED=0 "GOLANGCI_LINT_CACHE=${LINT_CACHE_ROOT}/cgo0" golangci-lint run --allow-parallel-runners ./...
if [ "${HAS_CBC}" -eq 1 ]; then
    start_job "golangci-cbc" env "GOLANGCI_LINT_CACHE=${LINT_CACHE_ROOT}/cbc" golangci-lint run --allow-parallel-runners --build-tags=cbc ./...
fi

run_python_lint() {
    ruff check --no-cache bench/
    ruff format --check --no-cache bench/
    if command -v mypy >/dev/null 2>&1; then
        mypy --no-incremental --cache-dir=/dev/null --ignore-missing-imports bench/
    elif command -v uvx >/dev/null 2>&1; then
        uvx --no-config mypy --no-incremental --cache-dir=/dev/null --ignore-missing-imports bench/
    else
        echo "Neither 'mypy' nor 'uvx' is available in PATH to type-check bench/." >&2
        return 1
    fi
}
start_job "python-lint" run_python_lint

QUALITY_GATE_BIN="${QUALITY_GATE_BIN:-${HOME}/.gemini/config/skills/writing/bin/quality_gate}"
if [ ! -x "${QUALITY_GATE_BIN}" ]; then
    if command -v quality_gate >/dev/null 2>&1; then
        QUALITY_GATE_BIN="$(command -v quality_gate)"
    elif [ "${GOBLIN_STRICT_GATES:-0}" = "1" ]; then
        fail "writing quality_gate binary not found at ${QUALITY_GATE_BIN} or in PATH."
    else
        printf 'WARNING: writing quality_gate binary not installed; skipping stylometric prose check.\n' >&2
        QUALITY_GATE_BIN=""
    fi
fi

run_quality_gate() {
    "${QUALITY_GATE_BIN}" --profile rfc --level strict --format compact \
        README.md docs/*.md bench/results/final_report.md

    python3 - "${QUALITY_GATE_BIN}" << 'PYEOF'
import ast, pathlib, re, subprocess, sys, tempfile

qg = sys.argv[1]
root = pathlib.Path(".")
go_c_files = sorted(
    p for ext in ("*.go", "*.cpp", "*.h")
    for p in root.rglob(ext)
    if "third_party" not in p.parts
)
py_files = sorted(p for p in root.rglob("*.py") if "third_party" not in p.parts)
sh_files = sorted(p for p in root.rglob("*.sh") if "third_party" not in p.parts)

def extract_go_c(path: pathlib.Path) -> str:
    blocks, cur, in_block = [], [], False
    for line in path.read_text().splitlines():
        s = line.strip()
        if s.startswith("/*"):
            in_block = True
        if in_block:
            if "*/" in s:
                in_block = False
            continue
        if s.startswith("//"):
            if re.match(r"^//(go:|#cgo|nolint|\s*#include|\s*#define|\s*#stdlib|\s*Output:)", s):
                if cur:
                    blocks.append("\n".join(cur))
                    cur = []
                continue
            cur.append(re.sub(r"^//\s?", "", s))
        else:
            if cur:
                blocks.append("\n".join(cur))
                cur = []
            m = re.search(r'(?<!:)//\s*(.+)$', s)
            if m and not ('"http' in s or "'//" in s or '"//' in s):
                tr = m.group(1).strip()
                if not tr.startswith(("nolint", "GOBLIN_CBC_BRIDGE_H")):
                    blocks.append(tr)
    if cur:
        blocks.append("\n".join(cur))
    return "\n\n".join(blocks)

def extract_py(path: pathlib.Path) -> str:
    src = path.read_text()
    tree = ast.parse(src)
    docs = [
        ast.get_docstring(n)
        for n in ast.walk(tree)
        if isinstance(n, (ast.Module, ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef))
        and ast.get_docstring(n)
    ]
    cur = []
    for line in src.splitlines():
        s = line.strip()
        if s.startswith("#") and not s.startswith("#!"):
            cur.append(re.sub(r"^#\s?", "", s))
        elif cur:
            docs.append("\n".join(cur))
            cur = []
    if cur:
        docs.append("\n".join(cur))
    return "\n\n".join(docs)

def extract_sh(path: pathlib.Path) -> str:
    docs, cur = [], []
    for line in path.read_text().splitlines():
        s = line.strip()
        if s.startswith("#") and not s.startswith("#!"):
            cur.append(re.sub(r"^#\s?", "", s))
        elif cur:
            docs.append("\n".join(cur))
            cur = []
    if cur:
        docs.append("\n".join(cur))
    return "\n\n".join(docs)

with tempfile.TemporaryDirectory() as tmpdir:
    extracted = []
    for p in go_c_files + py_files + sh_files:
        txt = (
            extract_go_c(p) if p.suffix in (".go", ".cpp", ".h")
            else extract_py(p) if p.suffix == ".py"
            else extract_sh(p)
        )
        if not txt.strip():
            continue
        out_p = pathlib.Path(tmpdir) / (str(p).replace("/", "__") + ".md")
        out_p.write_text(txt + "\n")
        extracted.append(str(out_p))
    res = subprocess.run([qg, "--profile", "rfc", "--level", "strict", "--format", "compact"] + extracted)
    if res.returncode != 0:
        sys.exit(res.returncode)
PYEOF
}

if [ -n "${QUALITY_GATE_BIN}" ]; then
    start_job "quality-gate" run_quality_gate
fi
start_job "check-docs" go run ./cmd/goblin-bench -check-docs

wait_jobs

# 4. Parallel Go Test Suite Gate (Default, SIMD, CGO_ENABLED=0, Race, and opt-in CBC)
log_step "4/4 Running Go test suites across all build configurations in parallel..."
start_job "test-default" go test ./...
start_job "test-simd" env GOEXPERIMENT=simd go test ./...
start_job "test-cgo0" env CGO_ENABLED=0 go test ./...
start_job "test-race-short" go test -race -short ./...
if [ "${HAS_CBC}" -eq 1 ]; then
    start_job "test-cbc" go test -tags=cbc ./...
    start_job "test-race-cbc-short" go test -race -tags=cbc -short ./...
fi

wait_jobs

printf '\nAll pre-commit quality gates passed.\n'
