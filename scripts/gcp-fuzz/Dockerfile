# syntax=docker/dockerfile:1

FROM golang:1.27-bookworm AS builder

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
    GOEXPERIMENT=simd GOAMD64=v1 go test -c -fuzz=. -o /out/assign.test ./assign && \
    GOEXPERIMENT=simd GOAMD64=v1 go test -c -fuzz=. -o /out/goblin.test . && \
    GOEXPERIMENT=simd GOAMD64=v1 go test -c -fuzz=. -tags=cbc -o /out/goblin_cbc.test . && \
    CGO_ENABLED=0 go build -trimpath -o /out/goblin-fuzzwatch ./cmd/goblin-fuzzwatch

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
COPY --from=builder /out/goblin.test /app/bin/goblin.test
COPY --from=builder /out/goblin_cbc.test /app/bin/goblin_cbc.test
COPY --from=builder /out/goblin-fuzzwatch /app/bin/goblin-fuzzwatch
COPY --from=builder /opt/cbc-dist/lib/*.so* /app/lib/
COPY --from=builder /src/testdata /app/work/testdata
COPY --from=builder /src/assign/testdata /app/work/assign/testdata
COPY scripts/gcp-fuzz/targets.tsv /app/targets.tsv
COPY scripts/gcp-fuzz/fuzz_worker.sh /app/fuzz_worker.sh

RUN chmod +x /app/fuzz_worker.sh /app/bin/* && \
    echo "/app/lib" > /etc/ld.so.conf.d/goblin-cbc.conf && \
    ldconfig

ENV LD_LIBRARY_PATH=/app/lib \
    OPENBLAS_NUM_THREADS=1 \
    OMP_NUM_THREADS=1 \
    MKL_NUM_THREADS=1 \
    NUMEXPR_NUM_THREADS=1 \
    VECLIB_MAXIMUM_THREADS=1

ENTRYPOINT ["/app/fuzz_worker.sh"]
