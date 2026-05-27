FROM debian:bookworm-slim AS nsjail-base

COPY scripts/ /app/scripts/
RUN chmod +x /app/scripts/install.sh && /app/scripts/install.sh


FROM golang:1.23-bookworm AS go-builder

# Install protoc and the Go protobuf plugin
RUN apt-get update && apt-get install -y --no-install-recommends \
        protobuf-compiler \
    && rm -rf /var/lib/apt/lists/*

RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.34.2

WORKDIR /build

# Copy go module files first for layer caching.
COPY server/go.mod ./
RUN go mod download

COPY server/ ./

# Generate protobuf Go bindings
# --go_opt=paths=source_relative keeps the output path relative to the
# input file, so proto/judge.proto -> proto/judge.pb.go inside /build
RUN protoc \
        --go_out=. \
        --go_opt=paths=source_relative \
        proto/judge.proto

# Build the static binary
RUN CGO_ENABLED=0 GOOS=linux go build -mod=mod -ldflags="-s -w" -o /goboxd .


FROM nsjail-base AS nsjail-server

# Build the pre-chroot sandbox directory
RUN mkdir -p /sandbox \
    && cp -r /bin  /sandbox/ \
    && cp -r /lib  /sandbox/ \
    && cp -r /lib64 /sandbox/ \
    && cp -r /usr  /sandbox/

# Copy compiled server binary and config
COPY --from=go-builder /goboxd /app/goboxd
COPY server/config/languages.yaml /app/config/languages.yaml

# Create a writable /tmp inside the sandbox (nsjail workdir mount point)
# and stub directories for device/proc/etc bind-mounts
RUN mkdir -p /sandbox/tmp && chmod 1777 /sandbox/tmp \
    && mkdir -p /sandbox/dev \
    && mkdir -p /sandbox/proc \
    && mkdir -p /sandbox/etc

WORKDIR /app

EXPOSE 8000

ENV CONFIG_PATH=/app/config/languages.yaml

CMD ["/app/goboxd"]
