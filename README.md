# DDNS by DNSPod

基于腾讯云 DNSPod API 3.0 的域名解析与动态域名（DDNS）管理工具，
**Go 后端 + React 19 前端**（shadcn-admin），单容器部署，自带 Web 管理界面。

本项目由原先的 Python 脚本版本重写而来，保留核心 DDNS 能力，并补齐了解析记录管理、
多任务调度、变更历史与多渠道通知。

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

### 1. 准备凭证

在 `~/.env` 中写入（注意 `export` 不能省略，否则 `source` 后子进程读不到）：

```bash
# 腾讯云 API 密钥（建议只授予 DNS 解析权限）
export TENCENTCLOUD_SECRET_ID="AKIDxxxxxxxx"
export TENCENTCLOUD_SECRET_KEY="xxxxxxxx"

# ntfy 通知（可选）
export NTFY_SERVER="https://ntfy.example.com"
export NTFY_TOPIC="ddns_by_dnspod"
export NTFY_USER="your-username"
export NTFY_PASS="your-password"
```

首次启动时，后端会自动读取这些变量创建默认账号与通知渠道；之后以数据库为准，不再依赖环境变量。

### 2. 启动

```bash
docker network create proxy          # 若尚未创建
mkdir -p data certs                  # 容器以 root 运行，挂载目录不需要 chown
docker compose up -d --build
docker compose logs -f
```

`OIDC_ISSUER` 与 `OIDC_CLIENT_ID`（下一步）必须在 `up` 之前导出：compose 缺这两项会直接报错，
而不是起一个连不上 IdP 的容器。

数据（SQLite 库与加密密钥）保存在 `./data`，**务必纳入备份**--密钥文件丢失将导致已存凭证无法解密。

### 3. 配置身份提供方（OIDC）

后端是**纯资源服务器**：浏览器（前端作为 public client，授权码 + PKCE）自己完成登录，
之后每次请求把 access token 放在 `Authorization: Bearer` 上，后端只校验这个 token。
所以后端不保存会话、不下发 Cookie、不处理登录跳转，也不再信任反代注入的任何身份 header。

在 IdP（Authelia 等）注册一个 **public client**（PKCE，无 client secret），
回调地址填 `<本站 origin>/auth/callback`，然后：

- 后端：`oidc.issuer` 填 IdP 的 discovery 地址，`oidc.client_id` 填该 client id。
  这两项必填，缺失会**直接启动失败**（`oidc.audience` 留空即等于 client id）；
- 前端：**不需要单独配置**。Go 服务在 `/config.json` 把 `oidc.disabled` /
  `oidc.issuer` / `oidc.client_id` 直接交给浏览器，所以镜像与部署无关，
  换 IdP 只改容器环境变量，不用重新构建。界面上的展示项用 `frontend.*` 覆盖：

| 环境变量 | 对应配置 | 说明 |
| --- | --- | --- |
| `OIDC_PROVIDER_NAME` | `frontend.provider_name` | UI 里 IdP 的显示名，空则显示 `OIDC` |
| `OIDC_LOGOUT_URL` | `frontend.logout_url_template` | 供未实现标准登出的 IdP 使用，如 Authelia 的 `/logout?rd={redirectUrl}` |
| `OIDC_SCOPE` | `frontend.scope` | 登录请求的 scope |
| `OIDC_REDIRECT_PATH` | `frontend.redirect_path` | 回调路径，默认 `/auth/callback` |
| `APP_NAME` / `API_BASE_URL` | `frontend.app_name` / `frontend.api_base_url` | 页面标题与 API 前缀 |

这一组用裸名而不是 `DDNS_` 前缀：浏览器在运行期读到的就是这些词，前后端不必各记一套。

compose 里就是：

```yaml
    environment:
      - OIDC_ISSUER=https://auth.example.com
      - OIDC_CLIENT_ID=ddns-by-dnspod
      - OIDC_PROVIDER_NAME=Authelia
      - "OIDC_LOGOUT_URL=https://auth.example.com/logout?rd={redirectUrl}"
```

`frontend/public/config.json`（本地可用 `config.local.json` 覆盖）只在**前端由其他静态
托管**（如 EdgeOne）时才有意义；那种场景才需要构建期把 `OIDC_ISSUER` / `OIDC_CLIENT_ID` /
`OIDC_SCOPE` / `OIDC_PROVIDER_NAME` / `OIDC_LOGOUT_URL` / `AUTH_REQUIRED` / `ACME_ENABLED` /
`APP_NAME` / `API_BASE_URL` 注入产物（`ACME_ENABLED` 控制证书页的签发入口是否可用，
对应 Go 服务的 `acme.enabled`）。
nginx 只需做普通反代，无需任何鉴权配置。

## 运维

- **探针**：`/api/health` 是存活（进程在），`/api/ready` 是就绪（数据库能答、身份提供方已连通过）。
  容器 healthcheck 用 ready--活着不等于能服务。
- **备份**：sqlite 开 WAL，**运行中直接拷 `ddns.db` 会丢未 checkpoint 的事务**。用内置参数：

  ```bash
  ./bin/ddns-server -c config.yml -backup backups/ddns-$(date +%F).db
  ```

  它走 `VACUUM INTO` 写临时文件再原子替换，同名可每天重复执行；产物权限 0600（含全部凭证与私钥）。
  该参数需要和服务器**同一份 `-c` 配置**，因为要指向同一个 `database.path`。恢复 = 停进程 -> 覆盖
  `database.path` -> 起进程 -> 看 `/api/ready`。
- **日志**：全部 `log/slog`，一行一条写 stderr；`server.log_format: json` 交给采集，访问日志带 `rid`
  与 `caller`（谁发的请求）。
- **版本**：`./bin/ddns-server -version` 报出构建版本；它取自 `backend/openapi.yml` 的
  `info.version`，构建时注入，`frontend/package.json` 里的同名字段由 CI 校验一致。
- **指标**：`/metrics`（Prometheus 文本格式）默认**关闭**，且不带鉴权--打开时它和 UI 同端口，
  反代转发什么它就暴露什么，请把该 location 限制在内网。

## 配置

`config.example.yml` 是带注释的全量配置参考，复制为 `config.local.yml` 后启动时用
`-c config.local.yml` 指定。Docker/compose 部署不需要这个文件，环境变量就够了。

大多数配置项用 `DDNS_` + 大写路径的环境变量覆盖，例如：

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
| `DDNS_SERVER_LOG_LEVEL` | `server.log_level`（`debug` 才看得到"客户端取消"这类记录） |
| `DDNS_SERVER_HSTS` | `server.hsts`（TLS 终止于反代时才开） |
| `DDNS_METRICS_ENABLED` | `metrics.enabled`（默认关） |

`server.trusted_proxies` 是列表，只能写在 YAML 里；默认只有回环，反代在别的网段时必须显式加上，
加宽到整个私网等于让同网段任何主机自选日志里的来源地址。

`oidc` 与 `frontend` 两段是例外：它们用**裸名**（`OIDC_ISSUER`、`OIDC_CLIENT_ID`、
`AUTH_DISABLED`、`APP_NAME`......），清单见上一节。

另可用 `DDNS_CRYPTO_KEY`（base64 编码的 32 字节）直接指定主密钥，优先级高于密钥文件。

## 证书（Let's Encrypt）

证书的签发与续期、文件布局、权限与注意事项见 [`docs/certificates.md`](docs/certificates.md)。

## 本地开发

```bash
make deps            # 安装 Go 与 npm 依赖
make dev-backend     # 后端运行在 :8080
make dev-frontend    # 前端运行在 :5173，API 请求代理到 8080
```

本地开发建议关闭鉴权，省去配置 IdP 的麻烦（`make dev-frontend` 由 vite 托管，
需同时在 `config.local.json` 里设 `authRequired: false`；跑后端二进制时
`authRequired` 直接来自 `AUTH_DISABLED`）：

```bash
AUTH_DISABLED=true DDNS_SERVER_LISTEN=127.0.0.1:8080 make dev-backend
```

注意：`AUTH_DISABLED=true` 只在监听环回地址时允许启动。默认 `server.listen=:8080`
绑定所有网卡，两者叠加等于把整个 DNS 控制台暴露给所在网络，因此这种组合会被拒绝启动。

构建单文件可执行程序（前端资源嵌入二进制）：

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

所有接口位于 `/api` 下，需在 `Authorization: Bearer <access token>` 带上登录后拿到的令牌，
缺失或校验失败返回 401。`/api/health` 与 `/api/ready` 不鉴权，供容器探针使用。
请求与响应的形状以 `backend/openapi.yml` 为准，`frontend/src/api-types.gen.ts` 由它生成，
契约测试与镜像构建都会校验实现没有漂移。
请求体上限 4 MiB，超过返回 413（与"JSON 无法解析"的 400 区分开）。

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