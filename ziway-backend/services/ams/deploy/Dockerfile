# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files first for layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/ziway-ambs cmd/server/main.go

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Shanghai

WORKDIR /app

# Copy binary
COPY --from=builder /bin/ziway-ambs /app/ziway-ambs

# Copy configs (excluding keys, which should be mounted)
COPY configs/rbac_model.conf /app/configs/rbac_model.conf
COPY configs/prod.yaml /app/configs/prod.yaml

# Non-root user
RUN adduser -D -u 1000 ziway
USER ziway

EXPOSE 8080 50051

HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
  CMD wget -qO- http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/ziway-ambs"]
