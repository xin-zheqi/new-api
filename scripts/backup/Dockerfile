# This image contains clients only; it never runs a database server.
FROM postgres:15.20-bookworm@sha256:72ccb1ad4dc9d5813ea40a8c1c87af746f16658fb029c17fe570ea9b881f68b3 AS postgres-client
# Build the pinned upstream clients with the same patched Go release as New API.
# Source modules are verified by Go's checksum database; no compiler is shipped.
FROM golang:1.26.10-bookworm@sha256:23ce71337b726608afb1bc00986e630a05034a87a87ea1fc2cd1ae5cf33387eb AS tools
ENV CGO_ENABLED=0 GOMAXPROCS=2 GOTOOLCHAIN=local
ARG GOPROXY=https://proxy.golang.org,direct
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go install -p=2 -trimpath -ldflags="-s -w -X main.version=0.18.1" github.com/restic/restic/cmd/restic@v0.18.1
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go install -p=2 -trimpath -ldflags="-s -w -X github.com/rclone/rclone/fs.Version=v1.71.2" github.com/rclone/rclone@v1.71.2
RUN --mount=type=cache,target=/go/pkg/mod \
    mkdir -p /tool-licenses \
    && cp /go/pkg/mod/github.com/restic/restic@v0.18.1/LICENSE /tool-licenses/restic.LICENSE \
    && cp /go/pkg/mod/github.com/rclone/rclone@v1.71.2/COPYING /tool-licenses/rclone.LICENSE

FROM debian:bookworm-slim@sha256:734a72b3d6142a71993b4bc580ca01d8ca687f77a2c422d96a961d3376b179bc
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       python3 ca-certificates tzdata libpq5 libssl3 liblz4-1 libzstd1 libreadline8 \
    && rm -rf /var/lib/apt/lists/*
COPY --from=postgres-client /usr/lib/postgresql/15/bin/pg_dump /usr/local/bin/pg_dump
COPY --from=postgres-client /usr/lib/postgresql/15/bin/pg_restore /usr/local/bin/pg_restore
COPY --from=postgres-client /usr/lib/postgresql/15/bin/psql /usr/local/bin/psql
COPY --from=tools /go/bin/restic /usr/local/bin/restic
COPY --from=tools /go/bin/rclone /usr/local/bin/rclone
COPY --from=tools /tool-licenses /usr/share/licenses/new-api-backup/
COPY --from=tools /usr/local/go/LICENSE /usr/share/licenses/new-api-backup/Go.LICENSE
COPY LICENSE /usr/share/licenses/new-api-backup/LICENSE
# Ship the complete interpreted program and its build instructions with the image.
COPY Dockerfile .dockerignore README.md /opt/backup/
COPY runner.py /opt/backup/runner.py
ENV PYTHONUNBUFFERED=1 PYTHONDONTWRITEBYTECODE=1 TZ=Asia/Shanghai
WORKDIR /opt/backup
HEALTHCHECK --interval=60s --timeout=10s --start-period=5m --retries=3 \
    CMD ["python3", "/opt/backup/runner.py", "health"]
ENTRYPOINT ["python3", "/opt/backup/runner.py"]
CMD ["run"]
