FROM golang:1.21-bookworm as builder
WORKDIR /app
COPY go.* ./
RUN go mod download
COPY . ./
RUN go build -mod=readonly -v -o fuyuu-router

FROM debian:bookworm-slim
RUN set -x && apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y \
    ca-certificates && \
    rm -rf /var/lib/apt/lists/*
    WORKDIR /app
COPY --from=builder /app/fuyuu-router fuyuu-router
CMD ["./fuyuu-router"]