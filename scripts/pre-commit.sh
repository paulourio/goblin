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
declare -a JOB_DONE=()
BATCH_FAILED=0

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
    JOB_DONE+=(0)
}

# Record completion status for a finished job PID and remove it from ACTIVE_PIDS.
finish_job_pid() {
    local done_pid="$1"
    local rc="$2"
    local idx
    for idx in "${!JOB_PIDS[@]}"; do
        if [ "${JOB_PIDS[$idx]}" = "${done_pid}" ]; then
            local name="${JOB_NAMES[$idx]}"
            local log_file="${TMP_DIR}/${name}.log"
            if [ "${rc}" -eq 0 ]; then
                printf '  [PASS] %s\n' "${name}"
            else
                printf '  [FAIL] %s\n' "${name}" >&2
                printf '  --- %s output ---\n' "${name}" >&2
                cat "${log_file}" >&2
                printf '  --- end %s ---\n' "${name}" >&2
                BATCH_FAILED=1
            fi
            JOB_DONE[$idx]=1
            break
        fi
    done
    local remaining=()
    local pid
    for pid in "${ACTIVE_PIDS[@]:-}"; do
        if [ -n "${pid}" ] && [ "${pid}" != "${done_pid}" ]; then
            remaining+=("${pid}")
        fi
    done
    ACTIVE_PIDS=(${remaining[@]+"${remaining[@]}"})
}

# Wait for one active background job to finish and record its result.
wait_one_active_job() {
    local done_pid=""
    local rc=0
    if [ "${BASH_VERSINFO[0]}" -gt 5 ] || { [ "${BASH_VERSINFO[0]}" -eq 5 ] && [ "${BASH_VERSINFO[1]}" -ge 1 ]; }; then
        if wait -n -p done_pid "${ACTIVE_PIDS[@]}"; then
            rc=0
        else
            rc=$?
        fi
    else
        done_pid="${ACTIVE_PIDS[0]}"
        if wait "${done_pid}"; then
            rc=0
        else
            rc=$?
        fi
    fi
    finish_job_pid "${done_pid}" "${rc}"
}

# Launch a named job once active background jobs drop below max_jobs.
start_bounded_job() {
    local max_jobs="$1"
    local name="$2"
    shift 2
    while [ "${#ACTIVE_PIDS[@]}" -ge "${max_jobs}" ]; do
        wait_one_active_job
    done
    start_job "${name}" "$@"
}

# Wait for all queued background jobs and dump logs for any failures.
wait_jobs() {
    local idx
    for idx in "${!JOB_PIDS[@]}"; do
        if [ "${JOB_DONE[$idx]:-0}" -eq 1 ]; then
            continue
        fi
        local pid="${JOB_PIDS[$idx]}"
        local rc=0
        if wait "${pid}"; then
            rc=0
        else
            rc=$?
        fi
        finish_job_pid "${pid}" "${rc}"
    done
    local failed="${BATCH_FAILED}"
    ACTIVE_PIDS=()
    JOB_NAMES=()
    JOB_PIDS=()
    JOB_DONE=()
    BATCH_FAILED=0
    if [ "${failed}" -ne 0 ]; then
        fail "One or more parallel pre-commit gates failed."
    fi
}

# 1. Repository Hygiene & Portability Gate
log_step "1/5 Checking repository hygiene, line endings, and path portability..."

# Ensure no cache directories or compiled artifacts exist in the working tree.
bad_artifacts="$(find . \( -name '.mypy_cache' -o -name '.ruff_cache' -o -name '__pycache__' -o -name '*.prof' -o -name '*.test' \) -not -path './.git/*' -not -path './third_party/*' -not -path './formal/.lake/*' || true)"
if [ -n "${bad_artifacts}" ]; then
    printf '%s\n' "${bad_artifacts}" >&2
    fail "Forbidden cache or build artifacts found in working tree."
fi

GREP_EXCLUDES=(--exclude=.git --exclude-dir=.git --exclude-dir=.cache --exclude-dir=third_party --exclude-dir=.lake)

# Ensure no CRLF line endings in source or documentation files.
crlf_matches="$(grep -rnI "${GREP_EXCLUDES[@]}" $'\r' . || true)"
if [ -n "${crlf_matches}" ]; then
    printf '%s\n' "${crlf_matches}" >&2
    fail "Files with CRLF line endings detected. Normalize to LF."
fi

# Ensure zero references to legacy package name, unprefixed module path, unaliased root import/qualifier, or machine-specific home paths.
legacy_pkg="g""opt"
unprefixed_import='"gob'"lin"'(/|")'
unaliased_root_import='^[[:space:]]*"github\.com/paulourio/gob'"lin"'"'
unaliased_pkg_qual='gob'"lin"'\.[A-Z]'
forbidden_pattern="(${legacy_pkg}|${unprefixed_import}|${unaliased_root_import}|${unaliased_pkg_qual})"
legacy_matches="$(grep -rnI "${GREP_EXCLUDES[@]}" -E "${forbidden_pattern}" . || true)"
if [ -n "${legacy_matches}" ]; then
    printf '%s\n' "${legacy_matches}" >&2
    fail "Forbidden legacy identifier or unprefixed module import detected."
fi
if [ -n "${HOME:-}" ]; then
    home_matches="$(grep -rnIF "${GREP_EXCLUDES[@]}" -- "${HOME}" . || true)"
    if [ -n "${home_matches}" ]; then
        printf '%s\n' "${home_matches}" >&2
        fail "Hardcoded home directory path detected."
    fi
fi

# 2. Go Formatting Gate
log_step "2/5 Checking Go formatting (gofmt)..."
unformatted="$(gofmt -l .)"
if [ -n "${unformatted}" ]; then
    printf '%s\n' "${unformatted}" >&2
    fail "Unformatted Go files detected. Run 'gofmt -w .'."
fi

# 3. Parallel Static Analysis, Python Lint, Prose Quality Gate & Doc Sync
log_step "3/5 Running Go linters, Python checks, prose gate, and doc sync in parallel..."
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
    ruff check --no-cache bench/ scripts/
    ruff format --check --no-cache bench/ scripts/
    if command -v mypy >/dev/null 2>&1; then
        mypy --no-incremental --cache-dir=/dev/null --ignore-missing-imports bench/ scripts/
    elif command -v uvx >/dev/null 2>&1; then
        uvx --no-config mypy --no-incremental --cache-dir=/dev/null --ignore-missing-imports bench/ scripts/
    else
        echo "Neither 'mypy' nor 'uvx' is available in PATH to type-check bench/ and scripts/." >&2
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
SKIP_DIRS = {".git", ".cache", "third_party", ".lake"}
go_c_files = sorted(
    p for ext in ("*.go", "*.cpp", "*.h")
    for p in root.rglob(ext)
    if not (SKIP_DIRS & set(p.parts))
)
py_files = sorted(p for p in root.rglob("*.py") if not (SKIP_DIRS & set(p.parts)))
sh_files = sorted(p for p in root.rglob("*.sh") if not (SKIP_DIRS & set(p.parts)))

def extract_go_c(path: pathlib.Path) -> str:
    blocks, cur, in_block = [], [], False
    for line in path.read_text(encoding="utf-8").splitlines():
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

def extract_hash_comments(src: str) -> list[str]:
    blocks, cur = [], []
    for line in src.splitlines():
        s = line.strip()
        if s.startswith("#") and not s.startswith("#!"):
            cur.append(re.sub(r"^#\s?", "", s))
        elif cur:
            blocks.append("\n".join(cur))
            cur = []
    if cur:
        blocks.append("\n".join(cur))
    return blocks

def extract_py(path: pathlib.Path) -> str:
    src = path.read_text(encoding="utf-8")
    tree = ast.parse(src)
    docs = [
        ast.get_docstring(n)
        for n in ast.walk(tree)
        if isinstance(n, (ast.Module, ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef))
        and ast.get_docstring(n)
    ]
    docs.extend(extract_hash_comments(src))
    return "\n\n".join(docs)

def extract_sh(path: pathlib.Path) -> str:
    return "\n\n".join(extract_hash_comments(path.read_text(encoding="utf-8")))

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
        out_p.write_text(txt + "\n", encoding="utf-8")
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

# 4. Lean 4 Build & Track 5 ELF Binary Verification
HAS_LAKE=0
if command -v lake >/dev/null 2>&1; then
    HAS_LAKE=1
elif [ -n "${HOME:-}" ] && [ -x "${HOME}/.elan/bin/lake" ]; then
    export PATH="${HOME}/.elan/bin:${PATH}"
    HAS_LAKE=1
fi

run_lake_build() {
    (
        unset $(git rev-parse --local-env-vars)
        if command -v flock >/dev/null 2>&1; then
            (cd formal && flock /tmp/goblin-formal-lake.lock bash -c 'lake build --wfail && lake lint')
        else
            (cd formal && lake build --wfail && lake lint)
        fi
    )
}

run_formal_short() {
    (
        unset $(git rev-parse --local-env-vars)
        env GOBLIN_FORMAL=1 go test "$@" -short -run '^TestFormal' . ./assign
    )
}

# Detect online CPU count for bounded test scheduling.
detect_cpu_count() {
    local n=""
    if command -v nproc >/dev/null 2>&1; then
        n="$(nproc 2>/dev/null || true)"
    elif command -v getconf >/dev/null 2>&1; then
        n="$(getconf _NPROCESSORS_ONLN 2>/dev/null || true)"
    elif command -v sysctl >/dev/null 2>&1; then
        n="$(sysctl -n hw.ncpu 2>/dev/null || true)"
    fi
    if [[ ! "${n}" =~ ^[0-9]+$ ]] || [ "${n}" -lt 1 ]; then
        n=1
    fi
    printf '%s\n' "${n}"
}

# Execute a command or shell function with an explicit GOMAXPROCS cap.
run_with_procs() {
    local procs="$1"
    shift
    (
        export GOMAXPROCS="${procs}"
        "$@"
    )
}

if [ "${HAS_LAKE}" -eq 1 ]; then
    log_step "4/5 Building Lean 4 proofs and verifying Track 5 ELF binaries..."
    start_job "lake-build" run_lake_build
    start_job "track5-arm64-elf" env PYTHONDONTWRITEBYTECODE=1 python3 scripts/extract_elf_track5.py --verify
    start_job "track5e-amd64-elf" env PYTHONDONTWRITEBYTECODE=1 python3 scripts/extract_amd64_track5e.py --verify
    wait_jobs
elif [ "${GOBLIN_STRICT_GATES:-0}" = "1" ]; then
    fail "Lean 4 lake binary not found in PATH or elan toolchain directory."
else
    log_step "4/5 Skipping Lean 4 build and Track 5 ELF verification..."
    printf 'WARNING: Lean 4 lake binary not found in PATH or .elan/bin; skipping lake build, Track 5 ELF verification, and GOBLIN_FORMAL=1 oracle tests.\n' >&2
fi

# 5. Bounded Go Test Suite Gate (Race, Formal, SIMD, CGO_ENABLED=0, Default, and opt-in CBC)
step5_cpus="$(detect_cpu_count)"
step5_budget=$(( step5_cpus * 3 / 4 ))
if [ "${step5_budget}" -lt 1 ]; then
    step5_budget=1
fi
step5_max_jobs=2
if [ "${step5_budget}" -lt 2 ]; then
    step5_max_jobs=1
fi
step5_per_job_procs=$(( step5_budget / step5_max_jobs ))
if [ "${step5_per_job_procs}" -lt 1 ]; then
    step5_per_job_procs=1
fi
if [[ "${GOMAXPROCS:-}" =~ ^[0-9]+$ ]] && [ "${GOMAXPROCS}" -ge 1 ] && [ "${GOMAXPROCS}" -lt "${step5_per_job_procs}" ]; then
    step5_per_job_procs="${GOMAXPROCS}"
fi

log_step "5/5 Running Go test suites (max_jobs=${step5_max_jobs}, -p 1, GOMAXPROCS=${step5_per_job_procs}/${step5_cpus})..."
start_bounded_job "${step5_max_jobs}" "test-race-short" run_with_procs "${step5_per_job_procs}" go test -p 1 -race -short ./...
if [ "${HAS_CBC}" -eq 1 ]; then
    start_bounded_job "${step5_max_jobs}" "test-race-cbc-short" run_with_procs "${step5_per_job_procs}" go test -p 1 -race -tags=cbc -short ./...
fi
if [ "${HAS_LAKE}" -eq 1 ]; then
    start_bounded_job "${step5_max_jobs}" "test-formal-short" run_with_procs "${step5_per_job_procs}" run_formal_short -p 1
fi
start_bounded_job "${step5_max_jobs}" "test-simd" run_with_procs "${step5_per_job_procs}" env GOEXPERIMENT=simd go test -p 1 ./...
if [ "${HAS_CBC}" -eq 1 ]; then
    start_bounded_job "${step5_max_jobs}" "test-cbc" run_with_procs "${step5_per_job_procs}" go test -p 1 -tags=cbc ./...
fi
start_bounded_job "${step5_max_jobs}" "test-cgo0" run_with_procs "${step5_per_job_procs}" env CGO_ENABLED=0 go test -p 1 ./...
start_bounded_job "${step5_max_jobs}" "test-default" run_with_procs "${step5_per_job_procs}" go test -p 1 ./...

wait_jobs

printf '\nAll pre-commit quality gates passed.\n'
