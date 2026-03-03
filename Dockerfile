# syntax=docker/dockerfile:1
FROM golang:1.23-alpine AS builder
WORKDIR /app

# 先下载依赖（利用 Docker 缓存层）
COPY go.mod go.sum ./
RUN go mod download

# 编译
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o depot .

# ---- runtime ----
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /app/depot .

VOLUME ["/data"]
EXPOSE 8080

ENV DATA_DIR="/data" \
    PORT="8080"

CMD ["./depot"]
