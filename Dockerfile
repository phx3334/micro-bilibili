ARG GO_VERSION=1.26
ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.google.cn

# ---------------------------------------------------------------------------
# 构建阶段：一次编译出 4 个二进制（user / video / interaction / worker）
# 拆分后每个服务是独立进程，镜像通过下面 4 个 target 分别产出，避免重复编译。
# ---------------------------------------------------------------------------
FROM golang:${GO_VERSION} AS builder
ARG GOPROXY
ARG GOSUMDB
ENV GOPROXY=${GOPROXY} \
  GOSUMDB=${GOSUMDB}

WORKDIR /src/server

COPY server/go.mod server/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
  --mount=type=cache,target=/root/.cache/go-build \
  go mod download

COPY server ./

ENV CGO_ENABLED=0
# -trimpath 去掉构建机绝对路径，-s -w 去掉符号表与调试信息，减小镜像体积。
# 四条 build 串在一个 RUN 里，共用同一层 go-build 缓存挂载。
RUN --mount=type=cache,target=/go/pkg/mod \
  --mount=type=cache,target=/root/.cache/go-build \
  go build -trimpath -ldflags="-s -w" -o /out/user ./cmd/user && \
  go build -trimpath -ldflags="-s -w" -o /out/video ./cmd/video && \
  go build -trimpath -ldflags="-s -w" -o /out/interaction ./cmd/interaction && \
  go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker

# ---------------------------------------------------------------------------
# 运行阶段公共底座：仅含 CA 证书与时区数据，业务镜像在其上按 target 叠加
# ---------------------------------------------------------------------------
FROM alpine:3.21 AS base
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
# 日志目录：媒体文件全部写入 MinIO，本地不再需要 uploads 目录。
RUN mkdir -p /app/logs

# ---------------------------------------------------------------------------
# user-svc：HTTP 8081 / gRPC 9081，库 feed_user
# ---------------------------------------------------------------------------
FROM base AS user
COPY --from=builder /src/server/configs /app/configs
COPY --from=builder /out/user /app/user
COPY server/docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh
# entrypoint 按 APP_BINARY 选择要 exec 的二进制
ENV APP_BINARY=user
EXPOSE 8081 9081
ENTRYPOINT ["/app/docker-entrypoint.sh"]

# ---------------------------------------------------------------------------
# video-svc：HTTP 8082 / gRPC 9082，库 feed_video
# 媒体文件（上传源、转码产物、封面）全部写入 MinIO 对象存储
# ---------------------------------------------------------------------------
FROM base AS video
COPY --from=builder /src/server/configs /app/configs
COPY --from=builder /out/video /app/video
COPY server/docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh
ENV APP_BINARY=video
EXPOSE 8082 9082
ENTRYPOINT ["/app/docker-entrypoint.sh"]

# ---------------------------------------------------------------------------
# interaction-svc：HTTP 8083 / gRPC 9083，库 feed_interaction
# ---------------------------------------------------------------------------
FROM base AS interaction
COPY --from=builder /src/server/configs /app/configs
COPY --from=builder /out/interaction /app/interaction
COPY server/docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh
ENV APP_BINARY=interaction
EXPOSE 8083 9083
ENTRYPOINT ["/app/docker-entrypoint.sh"]

# ---------------------------------------------------------------------------
# worker：无 HTTP/gRPC 端口，消费 Kafka 并调用 ffmpeg 转码，回刷 feed_video 库
# 需要 ffmpeg 运行时（转码 + 截帧生成封面），因此单独安装。
# worker 直接启动二进制，不使用 entrypoint。
# ---------------------------------------------------------------------------
FROM base AS worker
RUN apk add --no-cache ffmpeg
COPY --from=builder /src/server/configs /app/configs
COPY --from=builder /out/worker /app/worker
ENTRYPOINT ["/app/worker"]