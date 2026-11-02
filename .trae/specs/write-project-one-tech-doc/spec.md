## . 项目定位与目标

### 1.1 求职叙事方向

面向云原生运维 / SRE 岗位求职，项目主打**业务侧 SRE 全链路能力闭环**：覆盖 K8s 高可用集群搭建、CNI 与服务网格分层网络、GitOps 持续交付、全栈可观测、弹性扩缩容、稳定性保障、故障演练、压测验证。
项目并非纯业务后端项目，所有业务改造遵循「为部署、可观测、弹性叙事服务的最小改造原则」，业务功能做精简收敛，重心放在云原生平台运维链路。

### 1.2 项目目标

1. 搭建一套可复现的 kubeadm 高可用 Kubernetes 集群，基于 containerd 运行时；
2. 完成单体应用拆分为微服务，基于 gRPC 通信，配套完整 OTel 可观测埋点；
3. 落地完整 GitOps 流水线：源码提交自动构建、漏洞扫描、镜像签名、灰度发布、指标门禁；
4. 落地弹性体系：HPA 常规弹性 + KEDA 基于 Kafka 消费延迟的异步任务弹性；
5. 搭建可观测三支柱（指标、链路、日志），配套 SLO、告警、轻量 AI 告警摘要降噪；
6. 完成稳定性验证：Velero 备份恢复、Chaos Mesh 故障注入、业务场景压测；
7. 搭建媒体对象存储 + CDN 缓存播放链路，补齐视频业务闭环；
8. 产出配套技术文档、故障复盘、博客系列，作为面试作品集。

### 1.3 边界说明

- 本项目为 Demo 作品，所有高可用、稳定性方案均为学习验证；生产环境需要额外加固权限、多可用区、存储分布式化等；
- AI 模块仅为告警摘要点缀，**不做自动自愈、根因定位**；
- 业务功能大量裁剪，仅保留 4 条具备运维叙事价值的主线。

## 2. 总体架构

采用分层架构，分为接入层、平台底座层、业务数据面、可观测层。

表格

| 分层 | 组件 | 核心职责 |
| --- | --- | --- |
| 接入层 | ingress-nginx、Cloudflare CDN / Nginx 边缘缓存 | HTTPS 接入、路由分发、静态媒体缓存、防盗链 |
| 平台底座层 | kubeadm HA、Keepalived+Haproxy、containerd、Helm、Cilium、Istio、Kafka(KRaft)、MinIO、Sealed-Secrets | 控制面高可用、CNI 网络、七层服务治理、消息队列、对象存储、敏感配置加密 |
| 交付层 | Tekton、Kaniko/BuildKit、Trivy、Cosign、Harbor、ArgoCD、Argo Rollouts | CI 构建、镜像安全扫描、镜像仓库、GitOps 同步、金丝雀灰度发布 |
| 弹性层 | HPA、KEDA、kafka-lag-exporter | CPU / 内存常规扩缩容、Kafka 消费延迟驱动异步 worker 弹性 |
| 业务数据面 | user-svc、video-svc、interaction-svc、转码 Worker、Redis、MySQL | 用户服务、视频服务、互动计数服务、无状态异步转码任务、缓存、持久化数据库 |
| 可观测层 | OTel Collector、Prometheus、Tempo、Loki、Grafana、Grafana Alloy、Hubble | 指标采集、链路存储、日志存储、可视化、网络流量观测 |
| 稳定性 & 演练 | Velero、Chaos Mesh、k6、vegeta | 集群备份恢复、故障注入、业务压测 |
| 告警扩展 | Alertmanager、轻量 LLM 摘要模块 | 阈值告警推送、告警摘要聚类降噪 |

## 3. 业务拆分方案

### 3.1 拆分边界

基于原有单体 `fake_tiktok` 仓库最小改造拆分：

- `user-svc`：用户相关
- `video-svc`：视频上传、元信息管理、播放签名 URL 下发
- `interaction-svc`：点赞、播放计数
- 转码 Worker：独立 Kafka 消费进程，**属于异步消费组件，不属于同步业务服务**，无状态，由 KEDA 弹性扩缩。
服务之间同步通信使用 gRPC。

### 3.2 保留的 4 条业务主线（其余功能全部裁剪）

1. **JWT 鉴权**：统一身份校验，Istio 配合做认证策略；
2. **视频上传 → Kafka → Worker 转码**
用户上传视频存入 MinIO 对象存储；上传事件写入 Kafka；转码 Worker 消费消息，从 MinIO 拉取源文件，转码完成后将成品视频回写 MinIO；
3. **点赞播放计数**：写入 Redis 缓存，Kafka 异步聚合，最终回刷 MySQL，平衡读写压力，保证最终一致性；
4. **视频播放链路**：MinIO 预签名 URL 防盗链，上游对接 CDN 缓存。

### 3.3 中间件选型决策

- Kafka（KRaft 单节点）：替代 RabbitMQ，作为异步事件队列；
- MySQL：业务主数据持久化；
- Redis：计数热点缓存；
- MinIO：S3 兼容对象存储，存放原始视频与转码后媒体资源；同时作为 Velero 可选 S3 备份后端；
- Elasticsearch、搜索模块：整体移除；
- 熔断、限流、超时、故障注入：下沉到 Istio 基础设施层实现，业务代码不开发相关逻辑。

### 3.4 埋点规范

业务代码复用 OTel Go SDK 埋点：

- 自动埋点：HTTP/gRPC、Kafka、MySQL、Redis；
- 自定义业务 Span；
- 业务指标：5~10 个业务指标（上传量、播放量、点赞量、转码成功 / 失败等）；
- 日志：zap 结构化 JSON 日志，携带 `trace_id`，输出到 stdout；
- 由 OTel Collector 统一路由指标、链路数据；Grafana Alloy 以 DaemonSet 采集容器 stdout 日志推送 Loki。

## 4. 集群与网络设计

### 4.1 集群方案

完整版：kubeadm 高可用集群，3 master + 2 worker；Keepalived + HAProxy 承载控制面 VIP，实现 apiserver 高可用；容器运行时 containerd；包管理 Helm。
本地调试简化版：K3s 单集群，不部署 Keepalived+HAProxy，仅用于快速验证功能。

### 4.2 CNI：Cilium

选型理由：基于 eBPF，替换 kube-proxy，兼顾网络性能、网络安全、网络观测。
职责：

- Pod 网络连通；
- eBPF 实现 Service 四层负载均衡，替代 iptables/IPVS；
- NetworkPolicy 微服务网络隔离；
- Hubble：网络流量可视化，观测 Pod 之间访问。

> 
> 约束：本项目**不启用 Cilium Service Mesh**，七层治理交给 Istio。

### 4.3 服务网格：Istio（Sidecar 模式）

选型理由：把流量治理能力下沉到基础设施，业务代码无侵入。
职责：

- mTLS 服务间加密通信；
- 超时、重试、熔断；
- 灰度流量分发（配合 Argo Rollouts 金丝雀）；
- 故障注入，配合 Chaos Mesh 做稳定性验证。

### 4.4 职责切分

- Cilium：四层及以下，底层转发、网络策略、Service 负载均衡；
- Istio：七层 HTTP/gRPC 流量治理；两者分层，互不冲突。

## 5. CI/CD 流水线

完整链路：源码 Git 仓库提交 → Tekton 流水线 → 构建镜像 → 测试 → Trivy 漏洞扫描 → 镜像推送 Harbor → GitOps 更新镜像 Tag → ArgoCD 同步资源 → Argo Rollouts 金丝雀发布。

### 5.1 流水线各环节职责

1. Kaniko / BuildKit：容器镜像构建；
2. Trivy：镜像漏洞扫描，高危漏洞直接阻断流水线，禁止推送到 Harbor；
3. Cosign：镜像签名，**可选扩展，Demo 默认关闭**；
4. Harbor：私有镜像仓库，存储业务与组件镜像；
5. Tekton 流水线成功后，自动向 GitOps 清单仓库提交 PR，更新镜像 Tag（GitOps 反查）；
6. ArgoCD：持续对比 Git 清单与集群状态，执行同步；
7. Argo Rollouts：金丝雀灰度发布，**以 Prometheus 中 OTel 产出的 RED 指标（错误率、延迟）作为发布门禁**，指标异常自动回滚。

### 5.2 敏感配置

Sealed-Secrets 管理敏感信息：Secret 密文可以直接存入 Git 仓库，集群内控制器负责解密，适配 GitOps 模型。

### 5.3 清单仓库策略

Demo 阶段：业务清单、基础设施 Helm Values、ArgoCD Application 放在同一个 Git 仓库，依靠目录逻辑隔离；
生产规范：拆分为业务清单仓库、基础设施仓库，做权限与变更风险隔离。
业务多环境：使用 Kustomize base + overlay，dev/prod 两套环境补丁，dev 自动发布，prod 变更需要人工审核。

## 6. 可观测性设计

遵循可观测三支柱：指标、链路、日志。

- 指标：Prometheus 存储，业务组件通过 ServiceMonitor/PodMonitor 采集；
- 链路：Tempo 存储 Trace；Demo 使用单节点模式 + PVC 持久化；配置 OTel tail_sampling 采样策略避免链路数据膨胀；Tempo compactor 组件配置 TTL（7 天）自动清理过期链路；生产建议后端切换 MinIO 对象存储；
- 日志：Grafana Alloy DaemonSet 采集 stdout 结构化日志，推送 Loki；
- 可视化统一入口：Grafana。

### 6.1 SLO

SLO 绑定项目四条业务主线：视频上传可用性、播放接口成功率、点赞计数最终一致性延迟；SLO 用于生成告警，不作为自动发布强门禁。

### 6.2 告警分层

- Alertmanager：承载阈值告警，通过 webhook 推送告警消息到轻量 LLM 摘要模块；
- Grafana：仅提供看板可视化，不承载告警规则，避免两套告警体系冲突。

## 7. 弹性与稳定性

### 7.1 弹性方案

1. HPA：基于 CPU、内存指标，用于 user/video/interaction 同步业务服务扩缩；
2. KEDA：用于视频转码 Worker 异步消费弹性；
指标链路：kafka-lag-exporter 采集消费延迟上报 Prometheus → KEDA Prometheus Scaler 读取 lag 指标，自动扩缩 Worker Pod；
> 
> 决策说明：不使用 KEDA 原生 Kafka Scaler，统一指标面，指标同时可以在 Grafana 观测。

### 7.2 稳定性保障

1. Velero：集群资源备份与恢复演练；备份后端支持 MinIO S3 兼容桶；
2. Chaos Mesh：故障注入实验矩阵：Pod 删除、CPU 压力注入、网络延迟、网络丢包；支持**边压边搅**组合实验（压测流量持续灌入的同时注入故障）；
实验范式：预设稳态假设 → 注入故障 → 通过指标观测度量结果。

## 8. 压测与故障演练场景

压测工具分工：

- k6：模拟用户侧业务流量，视频上传、视频播放、点赞交互；
- vegeta：短时间高并发风暴，模拟计数写入风暴。

限定压测场景（搜索、弹幕等已裁剪功能不作为压测范围）：

1. Feed 浏览洪峰（视频播放请求）；
2. 视频上传、转码洪峰，制造 Kafka 积压，验证 KEDA 弹性；
3. 点赞播放计数写入风暴，验证 Redis 缓存 + 异步回刷链路；
4. 服务冷启动、发布窗口稳定性验证。

## 9. 缓存与 CDN 链路

- 源站存储：MinIO 对象存储，保存原始视频、转码完成媒体文件；
- 公网演示方案：Cloudflare CDN，回源 MinIO，缓存静态视频资源；
- 本地集群演示方案：Nginx 充当边缘缓存；
- 播放鉴权：MinIO 预签名 URL，实现资源防盗链。

## 10. AI 告警摘要模块

定位：轻量点缀组件，不属于核心 SRE 链路。

- 触发源：Alertmanager webhook 推送告警文本；
- 能力：告警文本摘要、简单告警聚类降噪；
- 边界约束：**不做根因分析、不实现自动自愈**，避免过度放大方案能力。

## 11. 硬件与成本方案

三档方案：

1. 本地验证档：32G 内存开发机，使用 K3s 简化集群，不部署控制面 HA、Keepalived+Haproxy，用于快速迭代调试；
2. 云 Demo 完整版：5 台云服务器，3×2C4G master + 2×4C8G worker，数据盘 100~200G，部署完整 kubeadm HA 集群；
3. 备选物理机档：同规格二手物理机集群，适合长期稳定跑，降低云成本。

## 12. 实施路线（分阶段里程碑，标注依赖）

> 
> 依赖规则：底座网络就绪后，才能部署业务；业务 OTel 埋点改造完成，才能打包镜像。

1. 阶段 1：环境底座搭建
部署 kubeadm HA 集群、containerd、Helm；Cilium 部署与网络验证；Istio 部署、mTLS 与流量治理验证；Ingress-nginx。
2. 阶段 2：中间件底座部署
部署 Kafka (KRaft)、MySQL、Redis、MinIO；验证连通性；部署 Sealed-Secrets。
3. 阶段 3：可观测底座部署
OTel Collector、Grafana 全家桶（Prometheus/Tempo/Loki）、Grafana Alloy、kafka-lag-exporter；配置 ServiceMonitor。
4. 阶段 4：弹性、稳定性组件部署
KEDA、Chaos Mesh、Velero。
5. 阶段 5：CI/CD 流水线搭建
Harbor、Tekton、ArgoCD、Argo Rollouts；配置 GitOps 镜像 Tag 自动更新流程。
6. 阶段 6：业务代码改造与微服务部署
单体拆分、OTel 埋点改造；构建镜像、推 Harbor；Kustomize 多环境清单，ArgoCD 同步业务服务；验证四条业务主线。
7. 阶段 7：压测、故障演练、备份恢复演练
k6/vegeta 编写压测脚本；Chaos Mesh 故障注入；Velero 备份恢复演练；收集指标、截图、演练报告。
8. 阶段 8：文档整理、博客输出、演示录屏
完善架构文档，产出系列博客，录制全链路演示视频。