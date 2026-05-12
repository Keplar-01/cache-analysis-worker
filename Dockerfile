# ============================================================
# Stage 1: Build Go worker binary (linux/amd64).
# ============================================================
FROM --platform=linux/amd64 golang:1.22-bookworm AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -buildvcs=false -o /worker-cache ./cmd

# ============================================================
# Stage 2: Runtime — worker + native cats simulator.
# ============================================================
FROM --platform=linux/amd64 ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ca-certificates \
        libstdc++6 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /worker-cache /usr/local/bin/worker-cache
COPY cats /usr/local/bin/cats
RUN chmod +x /usr/local/bin/worker-cache /usr/local/bin/cats

ENTRYPOINT ["/usr/local/bin/worker-cache"]
