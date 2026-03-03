# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS builder
WORKDIR /app

# 先下载依赖（利用 Docker 缓存层）
COPY go.mod go.sum ./
RUN go mod download

# 编译
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o restfs .

# ---- runtime ----
FROM alpine:3.23
RUN apk add --no-cache ca-certificates tzdata su-exec

WORKDIR /app
COPY --from=builder /app/restfs .
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh && mkdir -p /data

VOLUME ["/data"]
EXPOSE 8080

ENV DATA_DIR="/data" \
    PORT="8080" \
    PUID="1000" \
    PGID="1000"

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget -qO /dev/null http://localhost:${PORT:-8080}/ || exit 1

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["./restfs"]
