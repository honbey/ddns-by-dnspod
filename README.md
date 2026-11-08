# DDNS by DNSPod

基于腾讯云 DNSPod API 3.0 的域名解析与动态域名（DDNS）管理工具，
**Go 后端 + React 19 前端**（shadcn-admin），单容器部署，自带 Web 管理界面。

## 功能

| 模块 | 说明 |
| --- | --- |
| 域名解析管理 | 记录的增删改查、批量新增/删除、启用暂停、线路选择，可替代 DNSPod 控制台 |
| DDNS 任务 | 每个任务独立配置域名、主机记录、类型、线路、cron 周期，支持手动触发 |
| 变更历史 | 记录每次 IP 变更，统计旧 IP 的实际存活时长并可视化 |
| 证书 | 用 Let's Encrypt 签发与自动续期（DNS-01 走已有 DNSPod 账号，无需对外开端口），文件按 certbot 命名落到挂载目录供其他 TLS 应用读取 |
| 通知渠道 | ntfy 与通用 webhook（覆盖钉钉、企业微信、Telegram 等），支持自定义模板与请求头 |
| 多账号 | 支持多个腾讯云账号，凭证 AES-256-GCM 加密存储 |
| API Key | 供脚本调用的对外 REST API，限定单个域名或用 `*` 覆盖该账号全部域名，明文仅创建时返回一次 |
| 界面语言 | 中英双语，账户菜单里切换，选择记在本地；中文是源码里的源字典，英文为译文 |

## 部署（Docker）

### 1. 启动

```bash
cp .env.example .env                 # 只填 OIDC_ISSUER 和 OIDC_CLIENT_ID
mkdir -p data certs
docker compose up -d --build         # 8080 只发布到宿主回环，DDNS_PORT 可改
docker compose logs -f
```

`OIDC_ISSUER` 与 `OIDC_CLIENT_ID` 是仅有的必填项，compose 缺了会直接报错。
腾讯云凭证可留空，登录后在账号页添加；填了只在首次启动时导入，之后以数据库为准。

compose 使用项目默认网络。`DDNS_PORT` 只改宿主侧端口，容器内仍是 8080；反代在另一个容器里时
要加入本 compose 项目，或改成 `0.0.0.0` 并在前面放宿主机防火墙。

`./data` 存 SQLite 库与加密密钥，**必须纳入备份**--密钥文件丢失将无法解密已存凭证。

### 2. 配置身份提供方（OIDC）

后端是纯资源服务器：前端作为 public client 走授权码 + PKCE，之后每次请求带
`Authorization: Bearer`，后端只校验 token，不保存会话、不发 Cookie、不信任反代注入的身份 header。

在 IdP（Authelia 等）注册 **public client**（无 client secret），回调地址
`<本站 origin>/auth/callback`。`oidc.issuer` 填 discovery 地址，`oidc.client_id` 填该 client id，
缺失会**直接启动失败**（`oidc.audience` 留空即等于 client id）。前端不需要单独配置：Go 服务
通过 `/config.json` 在运行时把 OIDC 参数交给浏览器，换 IdP 只改环境变量。展示项用 `frontend.*` 覆盖：

| 环境变量 | 对应配置 | 说明 |
| --- | --- | --- |
| `OIDC_PROVIDER_NAME` | `frontend.provider_name` | UI 里 IdP 的显示名，空则显示 `OIDC` |
| `OIDC_LOGOUT_URL` | `frontend.logout_url_template` | 供未实现标准登出的 IdP 使用，如 Authelia 的 `/logout?rd={redirectUrl}` |
| `OIDC_SCOPE` | `frontend.scope` | 登录请求的 scope |
| `OIDC_REDIRECT_PATH` | `frontend.redirect_path` | 回调路径，默认 `/auth/callback` |
| `APP_NAME` / `API_BASE_URL` | `frontend.app_name` / `frontend.api_base_url` | 页面标题与 API 前缀 |

`frontend/public/config.json`（本地用 `config.local.json` 覆盖）只在前端由其他静态托管时
有意义，那种场景需要构建期注入 OIDC、`AUTH_REQUIRED`、`ACME_ENABLED`、`APP_NAME` 等参数。
nginx 只需普通反代，无需鉴权配置。

## 运维

- **探针**：`/api/health` 存活、`/api/ready` 就绪（数据库 + IdP）。容器 healthcheck 是
  `ddns-server -healthcheck`，探 `/api/health`；就绪留给负载均衡。容器里用
  `DDNS_SERVER_LISTEN` 改监听地址，HEALTHCHECK 不带 `-c`。
- **备份**：sqlite 开 WAL，**运行中直接拷 `ddns.db` 会丢未 checkpoint 的事务**。用内置参数：

  ```bash
  ./bin/ddns-server -c config.yml -backup backups/ddns-$(date +%F).db
  ```

  产物权限 0600；参数需与服务器**同一份 `-c` 配置**，因为要指向同一个 `database.path`。
  恢复 = 停进程 -> 覆盖 `database.path` -> 起进程 -> 看 `/api/ready`。
- **日志**：`log/slog` 写 stderr，`server.log_format: json` 交给采集；访问日志带 `rid` 与 `caller`。
- **版本**：`./bin/ddns-server -version`，取自 `backend/openapi.yml` 的 `info.version`。
- **指标**：`/metrics` 默认关闭且不带鉴权，打开时把该 location 限制在内网。

## 配置

`config.example.yml` 是全量参考（复制为 `config.local.yml`，用 `-c` 指定）；compose 部署用
`.env.example`，不需要 YAML。配置项一般用 `DDNS_` + 大写路径覆盖：

| 环境变量 | 对应配置 |
| --- | --- |
| `DDNS_SERVER_LISTEN` | `server.listen` |
| `DDNS_DATABASE_PATH` | `database.path` |
| `DDNS_CRYPTO_KEY_FILE` | `crypto.key_file` |
| `DDNS_SCHEDULER_FAIL_THRESHOLD` | `scheduler.fail_threshold` |
| `DDNS_NOTIFIER_ALLOW_PRIVATE` | `notifier.allow_private` |
| `DDNS_ACME_DIRECTORY` | `acme.directory` |
| `DDNS_ACME_OUTPUT_DIR` | `acme.output_dir` |
| `DDNS_SERVER_LOG_FORMAT` | `server.log_format`（`text` / `json`） |
| `DDNS_SERVER_LOG_LEVEL` | `server.log_level` |
| `DDNS_SERVER_HSTS` | `server.hsts`（TLS 终止于反代时才开） |
| `DDNS_METRICS_ENABLED` | `metrics.enabled`（默认关） |
| `DDNS_CRYPTO_KEY` | 直接指定主密钥，优先于 `crypto.key_file` |

`server.trusted_proxies` 只能写在 YAML 里，默认只信任回环；反代在别的网段时显式加上。

## 证书（Let's Encrypt）

证书的签发与续期、文件布局、权限与注意事项见 [`docs/certificates.md`](docs/certificates.md)。

## 本地开发

```bash
make deps            # 安装 Go 与 npm 依赖
make dev-backend     # 后端运行在 :8080
make dev-frontend    # 前端运行在 :5173，API 请求代理到 8080
```

本地开发可关闭鉴权（`make dev-frontend` 还需在 `config.local.json` 里设 `authRequired: false`）：

```bash
AUTH_DISABLED=true DDNS_SERVER_LISTEN=127.0.0.1:8080 make dev-backend
```

`AUTH_DISABLED=true` 只在环回监听时允许启动；默认的 `server.listen=:8080` 绑定所有网卡，
两者叠加会被拒绝。

构建单文件可执行程序（前端资源 embed 进二进制）：

```bash
make build         # 产出 bin/ddns-server
```

## 项目结构

```
backend/                    Go 后端
  cmd/server/               程序入口与首次启动引导
  internal/
    acme/                   ACME（Let's Encrypt）签发与续期
    api/                    HTTP 接口与鉴权中间件
    auth/                   Bearer 令牌校验（JWKS 验签，opaque 回落 userinfo）
    config/                 配置加载
    ddns/                   任务执行器与 cron 调度器
    dnspod/                 腾讯云 SDK 封装
    ipdetect/               公网 IP 探测（v4/v6 分离）
    metrics/                Prometheus 注册表与 /metrics 处理器
    notifier/               ntfy 与 webhook 通知
    store/                  SQLite 持久化、凭证加密与快照备份
    frontend/               嵌入的前端构建产物
  openapi.yml              API 契约：载荷形状的唯一来源，前端类型由它生成
frontend/                   React 19 前端（shadcn-admin）
  src/i18n/                 中英字典（中文为源，英文按类型约束）
  src/features/<name>/api.ts 各功能的接口端点
docs/                       细节文档（README 只放概览与链接）
```

## API

所有接口位于 `/api`，需 `Authorization: Bearer <access token>`；`/api/health` 与
`/api/ready` 不鉴权。请求与响应的形状以 `backend/openapi.yml` 为准，
`frontend/src/api-types.gen.ts` 由它生成，契约测试与镜像构建都会校验实现没有漂移。
请求体上限 4 MiB，超过返回 413。

列表接口一律分页：`page` / `page_size`（上限 200），响应是 `{items, total, page, page_size}`。

```
GET    /api/ip?family=v4|v6                      探测当前公网 IP
GET/POST/PUT/DELETE /api/accounts[/:id]          账号管理
POST   /api/accounts/:id/verify                  验证凭证
GET    /api/domains                              域名列表
GET    /api/domains/:domain/records              记录列表
POST   /api/domains/:domain/records              新增记录
POST   /api/domains/:domain/records/batch        批量新增（≤100 条）
POST   /api/domains/:domain/records/batch-delete 批量删除（≤100 条）
PUT/DELETE /api/domains/:domain/records/:id      修改/删除记录
PATCH  /api/domains/:domain/records/:id/status   启用暂停
GET    /api/domains/:domain/groups               记录分组列表
GET    /api/domains/:domain/lines                可用线路
GET/POST/PUT/DELETE /api/ddns/tasks[/:id]        任务管理
POST   /api/ddns/tasks/:id/run                   手动触发
POST   /api/ddns/tasks/import-group/:domain      从分组导入任务（≤100 条）
POST   /api/ddns/reload                          重载调度器
GET    /api/history                              变更历史
GET    /api/stats/ip-changes?family=v4|v6      公网 IP 变更（存活时长图表）
GET/POST/PUT/DELETE /api/notify/channels[/:id]   通知渠道
POST   /api/notify/channels/:id/test             发送测试消息
GET/POST/PUT/DELETE /api/certificates[/:id]      证书管理
POST   /api/certificates/:id/issue               立即签发/续期（后台执行，返回 202，看 last_status）
GET/POST/PUT/DELETE /api/keys[/:id]              API Key 管理

# 对外 REST API（/api/v1），使用 API Key 鉴权
# key 的 domain 是单个域名，或 "*" 表示该账号下的全部域名；账号由 key 固定，不能用
# account_id 查询参数改
GET    /api/v1/domains/:domain/records           记录列表
POST   /api/v1/domains/:domain/records           新增记录
PUT    /api/v1/domains/:domain/records/:id       修改记录
DELETE /api/v1/domains/:domain/records/:id       删除记录
```

## License

MIT.
