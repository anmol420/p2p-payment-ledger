# STAGE 1
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git ca-certificates curl
RUN curl -sSL https://github.com/bufbuild/buf/releases/latest/download/buf-Linux-x86_64 \
    -o /usr/local/bin/buf && chmod +x /usr/local/bin/buf
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN buf generate
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s" \
    -o /build/server \
    ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s" \
    -o /build/migrate \
    ./cmd/migrate

# STAGE 2
FROM gcr.io/distroless/static AS final
USER nonroot:nonroot
COPY --from=builder /build/server /server
EXPOSE 50051
ENTRYPOINT ["/server"]

# STAGE 3
FROM gcr.io/distroless/static AS migrator
USER nonroot:nonroot
COPY --from=builder /build/migrate /migrator
COPY --chown=nonroot:nonroot ["cmd/migrate/migrations", "/migrations/"]
ENTRYPOINT ["/migrator"]
