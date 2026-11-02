#!/bin/sh
set -e

# 一个 entrypoint 服务三个 HTTP 服务：镜像内通过 ENV APP_BINARY=user|video|interaction
# 指定要启动的二进制，额外参数原样透传给该二进制。
# worker 镜像不使用本脚本，直接 ENTRYPOINT ["/app/worker"]。
exec "/app/${APP_BINARY:-user}" "$@"
