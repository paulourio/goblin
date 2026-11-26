# syntax=docker/dockerfile:1

FROM golang:1.27-bookworm AS builder

ARG TARGETARCH
ARG GOAMD64=v1

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    pkg-config \
    libbz2-dev \
    zlib1g-dev \
    liblapack-dev \
    libblas-dev \
    gfortran \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

# Build vendored COIN-OR / Cbc stack cleanly for Debian Bookworm glibc into /opt/cbc-dist
# so subsequent COPY . /src never clobbers the Debian-built libraries.
COPY scripts/build_cbc.sh /src/scripts/build_cbc.sh
COPY third_party /src/third_party
RUN rm -rf /src/third_party/Cbc/dist && \
    GOBLIN_CBC_PREFIX=/opt/cbc-dist bash /src/scripts/build_cbc.sh --clean

# Copy Go module and source files, then link the Debian-built Cbc prefix.
COPY . /src
RUN rm -rf /src/third_party/Cbc/dist && ln -s /opt/cbc-dist /src/third_party/Cbc/dist

# Compile standalone coverage-instrumented fuzz binaries and the saturation watcher.
RUN mkdir -p /out && \
    arch="${TARGETARCH:-$(go env GOARCH)}" && \
    case "${arch}" in \
        amd64) \
            case "${GOAMD64}" in \
                v1|v2|v3|v4) ;; \
                *) echo "ERROR: invalid GOAMD64=${GOAMD64} (expected v1, v2, v3, or v4)" >&2; exit 2 ;; \
            esac; \
            export GOAMD64; \
            goamd64_val="${GOAMD64}" ;; \
        arm64) \
            unset GOAMD64 || true; \
            goamd64_val="na" ;; \
        *) echo "ERROR: unsupported target arch ${arch}" >&2; exit 2 ;; \
    esac && \
    export GOEXPERIMENT=simd && \
    go test -c -fuzz=. -o /out/assign.test ./assign && \
    go test -c -fuzz=. -o /out/simd.test ./internal/simd && \
    go test -c -fuzz=. -o /out/format.test ./format && \
    go test -c -fuzz=. -tags=cbc -o /out/format_cbc.test ./format && \
    go test -c -fuzz=. -o /out/goblin.test . && \
    go test -c -fuzz=. -tags=goblin_assert -o /out/goblin_assert.test . && \
    go test -c -fuzz=. -race -o /out/goblin_race.test . && \
    go test -c -fuzz=. -tags=cbc -o /out/goblin_cbc.test . && \
    CGO_ENABLED=0 go build -trimpath -o /out/goblin-fuzzwatch ./cmd/goblin-fuzzwatch && \
    cbc_rev="vendored" && \
    if [ -f /src/third_party/VERSIONS ]; then \
        cbc_rev="$(awk -F= '/^Cbc=/{print $2; exit}' /src/third_party/VERSIONS | tr -d '[:space:]')"; \
        cbc_rev="${cbc_rev:-vendored}"; \
    fi && \
    printf 'GOARCH=%s\nGOAMD64=%s\nGOEXPERIMENT=%s\nCBC=%s\n' \
        "$(go env GOARCH)" "${goamd64_val}" "${GOEXPERIMENT}" "${cbc_rev}" > /out/build_info.txt

# Stage prebuilt Lean 4 Prove Lane checker ELFs matching the target architecture.
RUN mkdir -p /out-formal/.lake/build/bin && \
    arch="${TARGETARCH:-$(go env GOARCH)}" && \
    case "${arch}" in \
        amd64) want_machine="Advanced Micro Devices X86-64" ;; \
        arm64) want_machine="AArch64" ;; \
        *) echo "ERROR: unsupported target arch ${arch}" >&2; exit 2 ;; \
    esac && \
    staged=0 && \
    for elf in lp-check vipr-check cuts-check rules-check lsap-check; do \
        src_elf="/src/formal/.lake/build/bin/${elf}"; \
        if [ -x "${src_elf}" ]; then \
            got_machine="$(readelf -h "${src_elf}" 2>/dev/null | awk -F: '/^[[:space:]]*Machine:/ {gsub(/^[[:space:]]+|[[:space:]]+$/, "", $2); print $2; exit}')"; \
            if [ "${got_machine}" = "${want_machine}" ]; then \
                cp "${src_elf}" "/out-formal/.lake/build/bin/${elf}"; \
                staged=$((staged + 1)); \
            else \
                echo "Skipping ${elf}: ELF Machine '${got_machine:-non-ELF}' != '${want_machine}'" >&2; \
            fi; \
        fi; \
    done && \
    if [ "${staged}" -gt 0 ] && [ -f /src/formal/lakefile.toml ]; then \
        cp /src/formal/lakefile.toml /out-formal/lakefile.toml; \
    fi

FROM debian:bookworm-slim AS runtime

RUN apt-get update && apt-get install -y --no-install-recommends \
    libbz2-1.0 \
    zlib1g \
    liblapack3 \
    libblas3 \
    libstdc++6 \
    rsync \
    procps \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /out/assign.test /app/bin/assign.test
COPY --from=builder /out/simd.test /app/bin/simd.test
COPY --from=builder /out/format.test /app/bin/format.test
COPY --from=builder /out/format_cbc.test /app/bin/format_cbc.test
COPY --from=builder /out/goblin.test /app/bin/goblin.test
COPY --from=builder /out/goblin_assert.test /app/bin/goblin_assert.test
COPY --from=builder /out/goblin_race.test /app/bin/goblin_race.test
COPY --from=builder /out/goblin_cbc.test /app/bin/goblin_cbc.test
COPY --from=builder /out/goblin-fuzzwatch /app/bin/goblin-fuzzwatch
COPY --from=builder /out/build_info.txt /app/build_info.txt
COPY --from=builder /opt/cbc-dist/lib/*.so* /app/lib/
COPY --from=builder /out-formal /app/formal
COPY --from=builder /src/testdata /app/work/testdata
COPY --from=builder /src/assign/testdata /app/work/assign/testdata
COPY scripts/gcp-fuzz/targets.tsv /app/targets.tsv
COPY scripts/gcp-fuzz/fuzz_worker.sh /app/fuzz_worker.sh

RUN mkdir -p /app/work/format /app/work/internal/simd && \
    chmod +x /app/fuzz_worker.sh /app/bin/* && \
    if [ -n "$(ls -A /app/formal/.lake/build/bin 2>/dev/null || true)" ]; then \
        chmod +x /app/formal/.lake/build/bin/*; \
    fi && \
    echo "/app/lib" > /etc/ld.so.conf.d/goblin-cbc.conf && \
    ldconfig

ENV LD_LIBRARY_PATH=/app/lib \
    OPENBLAS_NUM_THREADS=1 \
    OMP_NUM_THREADS=1 \
    MKL_NUM_THREADS=1 \
    NUMEXPR_NUM_THREADS=1 \
    VECLIB_MAXIMUM_THREADS=1

ENTRYPOINT ["/app/fuzz_worker.sh"]
