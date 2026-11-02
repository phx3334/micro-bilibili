## micro-bilibili
一个精简版仿 b 站风格的微服务全栈项目：Go 后端拆成 **user / video / interaction** 三个 HTTP + gRPC 服务与一个 **worker** 异步进程，前端为 Vue 3 单页应用。本仓库只保留业务源码与 Dockerfile，部署清单由独立 manifest 仓库维护。

相比完整业务版本，本仓库刻意收窄了功能边界，把重点放在**可部署、可观测、可演进的工程结构**上，定位是「云原生运维 / SRE 作品线的业务底座」——用一套真实但足够小的微服务拓扑，承载链路追踪、指标采集、健康检查、异步消息与多库拆分等运维实践。

## 技术栈

**后端（Go）**
- 语言/框架：Go 1.26 + Gin（HTTP）+ gRPC（服务间调用）
- 数据库：MySQL 8，单实例三库分治（`feed_user` / `feed_video` / `feed_interaction`）
- 缓存：Redis 7（RDB + AOF 混合持久化）
- 消息队列：Kafka（单节点 KRaft，无 ZooKeeper）
- 对象存储：MinIO（S3 兼容；视频/封面/头像统一走对象存储，DB 只保存对象 key）
- 转码：ffmpeg（worker 消费 Kafka 任务后转码 + 截帧生成封面）
- 鉴权：JWT（Access + Refresh 双 Token）
- 配置：YAML + `APP_*` 环境变量覆盖
- 可观测性：OpenTelemetry Go SDK（Trace）+ Prometheus（Metrics）+ Tempo（Trace 存储）+ Grafana（展示）

**前端（Vue 3）**
- Vue 3 + TypeScript + Vite
- Pinia（状态管理）、Vue Router、Tailwind CSS
- 播放器：ArtPlayer
- Axios

**部署**
- 镜像构建：Docker 多阶段构建（`user` / `video` / `interaction` / `worker` / `web` 五个 target）
- 应用编排：Helm Chart 由独立 manifest 仓库（`https://github.com/phx3334/micro-bl--manifest`）维护，values 分层 + existingSecret 对接 sealed-secrets
- 平台组件（MySQL / Redis / Kafka / 观测栈）与集群本身同样由该 manifest 仓库管理，ArgoCD 引用其中的 Chart 路径

## 架构

```
                     浏览器
                       │
                       ▼
              ┌─────────────────┐
              │   web (nginx)   │  :80 托管 SPA 静态资源
              └────────┬────────┘
      /api/v1/user     │  /api/v1/video      /api/v1/interaction
          ┌────────────┼──────────────────┐
          ▼            ▼                  ▼
   ┌────────────┐ ┌────────────┐ ┌────────────────┐
   │    user    │ │   video    │ │  interaction   │
   │ 8081/9081  │ │ 8082/9082  │ │  8083/9083     │
   └─────┬──────┘ └─────┬──────┘ └───────┬────────┘
         │  gRPC 互调（懒连接，无启动依赖顺序）
         └───────────────┼────────────────┘
                         │ Kafka（异步消息）
                         ▼
                  ┌─────────────┐
                  │   worker    │ 转码 / 计数回刷 / 热度重建
                  └──────┬──────┘
                         │ 下载源文件 / 上传转码产物
                         ▼
                  ┌─────────────┐
                  │    MinIO    │ 视频 / 封面 / 头像对象存储
                  └─────────────┘

  基础设施：MySQL(三库)、Redis、Kafka、MinIO
  观测链路：user/video/interaction/worker ──OTLP gRPC──▶ otel-collector ──▶ tempo
            prometheus ── 抓取 /metrics ──▶ user / video / interaction
            grafana(:3000) ◀── prometheus（指标） + tempo（Trace）
```

- HTTP 入口统一由 `web` 的 nginx 按路径前缀反代；媒体文件由 MinIO 提供公开直链或预签名 URL，前端直接使用后端返回的绝对地址。
- 服务间同步调用走 gRPC；转码任务与各类计数增量走 Kafka，由 worker 异步消费。
- 三个业务服务互调 gRPC，客户端为懒连接：下游未就绪只影响单次调用，不会阻塞本服务启动（这也是生产环境 K8s 编排里不给它们加启动顺序依赖的原因）。

## 目录结构

```
fake_tiktok/
├── server/                          # Go 后端（模块 fake_tiktok）
│   ├── cmd/
│   │   ├── user/                    # user-svc 入口（HTTP 8081 / gRPC 9081）
│   │   ├── video/                   # video-svc 入口（HTTP 8082 / gRPC 9082）
│   │   ├── interaction/             # interaction-svc 入口（HTTP 8083 / gRPC 9083）
│   │   └── worker/                  # worker 入口（无端口，Kafka 消费 + 定时任务）
│   ├── internal/
│   │   ├── user/                    # 账户/关注/资料：handler → logic → repository
│   │   ├── video/                   # 视频投稿/转码/列表/排行
│   │   ├── interaction/             # 点赞/评论
│   │   ├── model/                   # GORM 模型（按库分布）
│   │   └── dto/                     # request / response / cache 结构
│   ├── pkg/                         # appboot / auth / config / gormx / httpx /
│   │                                # kafkax / log / otelx / redisx / storagex / util
│   ├── api/                         # pb（gRPC 生成代码）与 proto 定义
│   ├── configs/config.yaml          # 统一配置（全部项可由 APP_* 覆盖）
│   └── docker-entrypoint.sh         # 按 APP_BINARY 启动对应 HTTP 服务
├── web/                             # Vue 3 前端
│   ├── src/                         # 源码（api / views / components / stores）
│   ├── public/                       # 静态资源（favicon、默认头像 avatar-default.jpg）
│   ├── Dockerfile                   # node 构建 + nginx 运行
│   └── nginx.conf                   # 三条 /api/v1/* 反代
├── Dockerfile                       # 多 target：user / video / interaction / worker
└── README.md
```

> 分工：本仓库只保留**业务源码与 Dockerfile**；集群本身（kubeadm）、网络（Cilium）、服务网格（Istio）、MySQL/Redis/Kafka、观测栈以及应用层 Helm Chart 全部由独立 manifest 仓库（`https://github.com/phx3334/micro-bl--manifest`）维护，ArgoCD 引用其中的 Chart 路径完成发布。

## 服务与端口

| 服务 | HTTP | gRPC | 数据库 | 职责 |
|------|------|------|--------|------|
| `user` | 8081 | 9081 | `feed_user` | 注册登录、JWT、验证码、资料与头像、关注/粉丝、个人主页 |
| `video` | 8082 | 9082 | `feed_video` | 视频上传、转码状态、详情/列表、分区排行、删除 |
| `interaction` | 8083 | 9083 | `feed_interaction` | 点赞、评论、评论点赞 |
| `worker` | 无 | 无 | `feed_video` | 消费 `video.transcode` 等 Kafka 消息、ffmpeg 转码、计数回刷、热度重建 |

所有 HTTP 服务均暴露 `/health`（MySQL + Redis 探活）与 `/metrics`（Prometheus 指标）。前端由 nginx 托管（生产在 K8s Ingress 之后）。

## 运行方式

两种运行方式任选：

- **本地开发**：依赖（MySQL / Redis / Kafka）由自己准备——可以是 K8s 集群里的服务，也可以是本机安装；服务地址通过 `.env` 的 `APP_DATABASE_*` / `APP_REDIS_*` / `APP_KAFKA_BROKERS` 指定。四个进程用 `go run` 直跑，改代码即时生效。
- **集群部署**：直接用 manifest 仓库中的 Helm Chart；本仓库只提供镜像构建所需的源码与 Dockerfile。

### 1. 准备依赖

| 依赖 | 默认地址 | 说明 |
|------|----------|------|
| MySQL 8 | `localhost:3306` | 首次使用需创建三库：`feed_user` / `feed_video` / `feed_interaction`（初始化清单见 manifest 仓库） |
| Redis 7 | `localhost:6379` | 缓存、ZSet 排行、验证码存储；密码见 manifest 仓库的 Redis 配置 |
| Kafka | `localhost:9092` | 转码任务与计数增量；单节点 KRaft 即可，允许自动建 topic |
| MinIO | `localhost:9000` | 视频/封面/头像对象存储；bucket 启动时自动创建。未启动 MinIO 时 user/video/worker 会启动失败（预期行为） |
| otel-collector | `localhost:4317` | 可选；不部署时把 `APP_OTEL_ENABLED` 设为 `false` |

### 2. 启动后端（四个进程分别启动）

前置：Go 1.26+；worker 需要宿主机可执行的 `ffmpeg`。

```bash
cd server
# .env 已被 gitignore，默认连接 localhost 的 3306/6379/9092/9000
go run ./cmd/user          # HTTP 8081 / gRPC 9081 → feed_user
go run ./cmd/video         # HTTP 8082 / gRPC 9082 → feed_video（另开终端）
go run ./cmd/interaction   # HTTP 8083 / gRPC 9083 → feed_interaction
go run ./cmd/worker        # 无端口，消费 Kafka 转码/计数消息
```

各服务未显式设置 `APP_DATABASE_DBNAME` / `APP_SERVER_PORT` / `APP_SERVER_GRPC_PORT` 时会自动回落到自己的默认值；服务间 gRPC 地址默认 `localhost:9081` / `9082` / `9083`，可用 `APP_GRPC_USER_ADDR`、`APP_GRPC_VIDEO_ADDR`、`APP_GRPC_INTERACTION_ADDR` 覆盖。

> `server` 目录下若存在 `.env` 会被优先加载（不覆盖已导出的环境变量）；统一配置在 `configs/config.yaml`，每一项都可由 `APP_*` 覆盖。
>
> 上传/播放依赖 MinIO：本地直跑前需先启动 MinIO（或对集群内 MinIO 做 port-forward），并在 `.env` 配置 `APP_MINIO_ENDPOINT/ACCESS_KEY/SECRET_KEY`。存储是 user/video/worker 的核心依赖，连接或建桶失败会直接启动失败（预期行为）。
>
> `public_read=true` 时对象走公开直链（初始化时尽力给 bucket 设置匿名 `s3:GetObject` 策略）；`public_read=false` 时对外返回预签名 GET，有效期由 `APP_MINIO_URL_EXPIRY` 控制。DB 与 Redis 只保存对象 key，响应时才生成地址。
>
> `APP_MINIO_ENDPOINT` 同时决定返回给浏览器的媒体地址，需保证浏览器可达：本地直跑用 `localhost:9000`（可 port-forward），集群部署应使用对外域名或前置反向代理，否则前端拿到集群内地址会无法播放。

### 3. 启动前端

```bash
cd web
npm install
npm run dev        # Vite 开发服务器 5173
# 生产构建
npm run build
npm run preview
```

`web/vite.config.ts` 已按路径代理到三个本地服务：`/api/v1/user` → 8081、`/api/v1/video` → 8082、`/api/v1/interaction` → 8083。媒体地址由后端返回绝对 URL（MinIO 直链或预签名），前端不再拼接静态前缀。

### 4. 构建镜像

```bash
# 五个镜像（manifest 仓库 Chart 中的镜像名与 tag 需与构建结果对齐）
docker build --target user        -t fake_tiktok-user:latest        .
docker build --target video       -t fake_tiktok-video:latest       .
docker build --target interaction -t fake_tiktok-interaction:latest .
docker build --target worker      -t fake_tiktok-worker:latest      .
docker build -f web/Dockerfile -t fake_tiktok-web:latest web
```

## 环境变量

所有配置以 `configs/config.yaml` 为默认值，用同名 `APP_*` 环境变量覆盖；常用变量表见下方。本地开发可在 `server/.env` 中按需设置（该文件已被 gitignore）。

| 变量 | 说明 | 默认 / 示例 |
|------|------|-------------|
| `APP_JWT_ACCESS_TOKEN_SECRET` / `APP_JWT_REFRESH_TOKEN_SECRET` | JWT 双 Token 密钥 | 空（生产必填） |
| `APP_DATABASE_HOST/PORT/USER/PASSWORD` | MySQL 连接 | `localhost/3306/root` |
| `APP_DATABASE_DBNAME` | 本服务的库：`feed_user` / `feed_video` / `feed_interaction` | 按服务默认 |
| `APP_SERVER_PORT` / `APP_SERVER_GRPC_PORT` | 本服务 HTTP / gRPC 端口 | `8081|8082|8083` / `9081|9082|9083` |
| `APP_GRPC_USER_ADDR` / `APP_GRPC_VIDEO_ADDR` / `APP_GRPC_INTERACTION_ADDR` | 下游 gRPC 地址 | `localhost:9081/9082/9083` |
| `APP_REDIS_HOST/PORT/PASSWORD/DB/KEY_PREFIX` | Redis 连接与键前缀 | `localhost/6379` |
| `APP_KAFKA_BROKERS` | Kafka broker 地址 | `localhost:9092` |
| `APP_MINIO_ENDPOINT` | MinIO 地址（host:port） | `localhost:9000` / 集群内 `minio:9000` |
| `APP_MINIO_ACCESS_KEY` / `APP_MINIO_SECRET_KEY` | MinIO 凭证（K8s 中来自 Secret） | 空（必填） |
| `APP_MINIO_BUCKET` | 对象存储桶（启动时自动创建） | `fake-tiktok` |
| `APP_MINIO_USE_SSL` | MinIO 是否走 TLS | `false` |
| `APP_MINIO_PUBLIC_READ` | `true` 公开直链；`false` 预签名 URL | `true` |
| `APP_MINIO_URL_EXPIRY` | 预签名 URL 有效期（Go duration） | `24h` |
| `APP_UPLOAD_MAX_FILE_SIZE` | 单文件上传上限（字节） | `1073741824`（1GB） |
| `APP_TRANSCODE_TIMEOUT` | 单次转码超时（秒） | `1800` |
| `APP_FFMPEG_BINARY_PATH` / `OUTPUT_CODEC` / `CRF` / `PRESET` | ffmpeg 参数 | `ffmpeg` / `libx264` / `23` / `medium` |
| `APP_CAPTCHA_LENGTH` | 图形验证码长度 | `6` |
| `APP_SKIP_CAPTCHA` | 跳过图形验证码（仅演示环境开启） | `false` |
| `APP_RUN_CRON` | 是否启用定时任务（仅 worker 为 `true`） | `false` |
| `APP_OTEL_ENABLED` | 是否启用链路追踪 | `false`（`.env` 中为 `true`） |
| `APP_OTEL_ENDPOINT` | OTLP gRPC 地址 | `localhost:4317` |
| `APP_DATABASE_AUTO_MIGRATE` | 启动时自动建表（首次部署后关闭） | `false` |

## 已实现功能

- **鉴权主线**：注册/登录（用户名 + 密码 + 图形验证码）、图形验证码、JWT 双 Token（Access + Refresh）、登出、头像上传、资料修改。
- **上传转码主线**：大文件表单上传（1GB / 1 小时超时）→ 源文件直传 MinIO `video/src/{objectID}.{ext}` → 落库（只存 key，状态 `transcoding`）→ 投递 Kafka `video.transcode` → worker 下载源文件、ffmpeg 转码并截帧生成封面 → 产物回写 MinIO `video/{id}.mp4` / `cover/{id}.jpg`，状态置 `published` 或 `failed` → 前端轮询 `video/status`。
- **点赞/播放计数主线**：点赞、评论等写路径只产生 Redis 计数与 Kafka 增量消息，由 worker 聚合后回刷 `videos` 表，避免高并发直写数据库。
- **播放链路**：视频详情、公开列表与热度排序（Redis ZSet）；详情页通过 interaction-svc / user-svc 回显当前用户的点赞与关注状态；视频/封面/头像地址由后端在响应时对对象 key 签名（公开直链或预签名），前端直接消费绝对 URL。
- **社交互动**：关注/取关与粉丝列表、评论与评论点赞、个人主页聚合统计（通过 gRPC 跨服务聚合）。

### 已移除功能

以下能力在精简版中已整体删除，代码、配置与依赖均不再保留：

- 邮箱注册与资料邮箱字段（注册/登录仅使用用户名 + 密码 + 图形验证码）
- 视频收藏 / 取消收藏 / 收藏夹管理
- 观看历史（播放进度记录与历史列表）
- AI 助手聊天（大模型对接、角色配置）
- 私信 / 消息通知
- 专栏文章、动态（说说）
- 弹幕
- 硬币、每日任务
- Elasticsearch 搜索
- 管理后台、用户角色/冻结字段
- 用户等级/经验、评论精选/置顶/IP 属地、关闭评论区等未闭环字段
- 熔断 / 限流组件

## 可观测性

- **健康检查**：`user`/`video`/`interaction` 的 `GET /health` 会 `Ping` MySQL 与 Redis，任一不可用返回 503，可直接作为 K8s liveness/readiness 探针。
- **指标**：`GET /metrics` 暴露 RED 指标（`http_server_requests_total`、`http_server_request_duration_seconds`、`http_server_errors_total`），由 `pkg/otelx` 的 Gin 中间件产出；本地由 Prometheus 抓取 `host.docker.internal:8081/8082/8083`（生产改为 ServiceMonitor 或 Pod 注解）。
- **Trace**：各服务通过 OTLP gRPC 上报到 `localhost:4317`（本地 otel-collector），collector 转发到 Tempo；HTTP 请求、gRPC 调用、MySQL/Redis 访问、Kafka 消息均有自动埋点，服务名为 `user-svc` / `video-svc` / `interaction-svc` / `worker-svc`。
- **展示**：Grafana `http://<host>:3000`，数据源由 manifest 仓库的 Grafana provisioning 自动注入——Prometheus（uid `prometheus`，默认数据源）与 Tempo（uid `tempo`，支持 trace → metrics 下钻）。
- **开关**：`APP_OTEL_ENABLED=false` 时观测初始化完全 noop，不产生 exporter 与额外开销；采集器不可用时只降级记日志，不阻断业务启动。

## 数据初始化

三库创建与初始化由 manifest 仓库（`https://github.com/phx3334/micro-bl--manifest`）负责；各服务的表由 `APP_DATABASE_AUTO_MIGRATE=true` 时自动建表。项目尚未上线，无历史数据需要迁移。

## CI/CD

流水线尚未建立（仓库里原先的空 `deploy.yml` 已删除）。后续按新服务矩阵实现：构建 5 个镜像 → Trivy 扫描 → 推 Harbor → ArgoCD 拉取 manifest 仓库 Chart 路径部署。在此之前请用本地 `docker build` 构建并手动推送镜像。

## 部署方式

**生产（K8s + Helm）**

```bash
# 1. 先由 manifest 仓库/平台侧准备好 MySQL / Redis / Kafka / MinIO / otel-collector，并建好三个库
# 2. 拉取 manifest 仓库，安装业务负载（镜像已在 Harbor 后可直接用 values-prod.yaml）
git clone https://github.com/phx3334/micro-bl--manifest
helm upgrade --install fake-tiktok micro-bl--manifest/deploy/helm/fake-tiktok \
  -f micro-bl--manifest/deploy/helm/fake-tiktok/values-prod.yaml \
  --set secret.existingSecret=fake-tiktok-secrets \
  -n fake-tiktok --create-namespace

# 3. 校验
kubectl -n fake-tiktok get pods,svc,ingress
```

manifest 仓库中的 Chart 渲染的资源：3 个 HTTP+gRPC 服务（Deployment + 双端口 Service）、worker（无端口）、web、ConfigMap、Secret（或复用 existingSecret）、Ingress、可选 HPA。媒体对象存 MinIO，Chart 不再渲染 uploads PVC/Volume。

几个设计取舍：

- **API 由 Ingress 直连后端**，不再经 web 的 nginx 反代，少一跳；web 只托管静态资源。
- **媒体统一走 MinIO**：user 写头像、video 写上传源、worker 下载源并写转码产物，三处共用同一 bucket；DB/Redis 只保存对象 key，响应时签名成直链或预签名 URL。
- **敏感配置走 `secret.existingSecret`**，配合 manifest 仓库的 sealed-secrets，values 里不出现明文；MinIO 凭证键名为 `MINIO_ACCESS_KEY` / `MINIO_SECRET_KEY`。
- **业务服务之间不加启动依赖**：gRPC 客户端懒连接，下游未就绪只影响单次调用。
- **灰度发布**：把 Deployment 换成 Argo Rollouts 的 `Rollout` 即可接 Prometheus 指标门禁，当前 Chart 保持最小依赖默认用原生 Deployment。

**本地（无集群）**：依赖自备（本机安装或集群内的 MySQL/Redis/Kafka/MinIO），四个进程 `go run` 直跑，改代码即时生效。

## 常用说明

- **Redis 持久化**：RDB + AOF 混合模式，配置由 manifest 仓库维护。
- **默认头像**：前端静态兜底 `web/public/images/avatar-default.jpg`，注册用户头像为空时直接使用；不再依赖服务端种子文件或 uploads 目录。
- **上传超时**：HTTP 读写超时、handler context 与前端 axios 统一为 1 小时，nginx 侧 `client_max_body_size 1g` / `proxy_*_timeout 1800s`。

## 许可证

本项目仅用于学习与交流。