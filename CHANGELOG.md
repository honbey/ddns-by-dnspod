# Changelog

本项目的版本记录，格式参考 [Keep a Changelog](https://keepachangelog.com/)，版本号遵循 [SemVer](https://semver.org/lang/zh-CN/)。

**2.0.0 是 Go 重写**：单二进制交付，SPA 用 `go:embed` 打进进程。
v1.1.0 及更早版本是 Python 实现，源码就在本仓库的历史里（`git log --all`），
tag 在远端（`v0.4.0` ... `v1.1.0`）；它们的变更记录以各自 tag 为准，这里不再重复。

## [Unreleased]

### 新增

- `oidc.opaque_tokens`：是否跟随 opaque token 的 userinfo 回退。**默认关闭**：
  userinfo 只能证明这个 token 属于该 issuer 的某个用户，不能证明它签给本客户端，
  因此 audience 校验对它无从谈起。只有在 issuer 按客户端隔离 userinfo 时才打开。
- Go 端到端测试（`backend/e2e/`，`-tags=e2e`）：构建真二进制、用临时库与临时密钥启动，
  覆盖嵌入的 SPA 与回退、两个探针、`/config.json`、未知 `/api` 路径必须是 JSON、
  `/metrics` 关闭时是 JSON 404；另有一个用例把验签预算压到 429，并确认健康检查
  在洪泛期间仍然公开可用。`make test-e2e` 与 CI 都会跑。
- 验签拒绝预算：`server.reject_budget_per_minute`（默认 30）。同一来源每分钟失败超过
  该值后返回 429 而不是 401，只计失败；已证明有效的 opaque token 直接放行，未知
  token 超额后仍有一次限频探测。此前没有这条底线，伪造 token 的洪泛会无限打满
  userinfo 往返。
- Playwright 端到端：`npm run test:e2e` / `make test-e2e`，`playwright.config.ts`
  与 `e2e/specs`，由 `e2e/server.sh` 起真二进制与本地 IdP；CI 同步执行。
- `.editorconfig`、`.npmrc`、`.github/renovate.json`。

### 变更

- OIDC 的 discovery、JWKS 与 userinfo 请求现在逐跳校验可信 URL，并限制重定向次数；
  初始端点可信不再等于重定向目标可信，userinfo 还会携带 Bearer token。
- opaque token 的 userinfo 结果增加短期正/负缓存与全局/按来源并发门；已证明有效的
  token 不受拒绝预算预检查影响，未知 token 超额后每 5 秒最多探测一次。默认关闭的
  `oidc.opaque_tokens` 语义不变。
- 前端登录回调和 provider metadata 查询都加时限；登出在 provider 失败或超时时仍会
  清除本地 manager 用户、重置状态并广播到其他标签页。
- 本地 IdP 统一为共享的 `frontend/e2e/mock-idp.mjs`（签发 RS256 JWT，无依赖）。
  原来的 `e2e/idp.mjs` 基于 `oidc-provider`、故意签发 opaque token，连带 38 个
  依赖一并删除；e2e 因此不再需要显式打开 `oidc.opaque_tokens`。
- issuer 可信性检查：既不是 https 也不在 loopback 的 issuer 被**永久拒绝**（记在
  `configErr` 里，后续每个请求在碰网络之前就返回）。此前没有任何检查，而 discovery
  会在第一个请求时重新执行——由路径上的攻击者写进 `jwks_uri` 的密钥决定谁算合法身份。
  discovery 返回的 `jwks_uri` 与 `userinfo_endpoint` 同样要求 https 或 loopback。
- 环境变量设为空值表示清空列表（`DDNS_NOTIFIER_ALLOW_PRIVATE=""` 即信任无人），
  而不是当作未设置。
- `/config.json` 的拉取与登出时的令牌撤销都限时；请求卡住不再拖住页面渲染或本地登出。
- e2e 的本地 IdP 显式开启 `DDNS_OIDC_OPAQUE_TOKENS`：它按设计签发 opaque token，
  生产保持默认关闭。
- The history page's domain filter memoised on an array rebuilt every render
  (`data?.items ?? []`), so it never hit. The fallback is now one shared empty
  list, which also keeps the table's row model stable.

- 版本号只写一处：`backend/openapi.yml` 的 `info.version`。构建与镜像用 `-ldflags -X`
  把它打进二进制，对外有两个出口：`-version` 参数与 `/api/health` 的 `version` 字段
  （`/api/ready` 另有 `Ready` 响应定义，不再复用 `Health`）。
- 源字典改为中文：`src/i18n/locales/zh.ts` 是源，`en.ts` 以 `typeof zh` 约束；界面语言
  既不是中英时现在回落中文。`MessageKey` 与 `i18next.d.ts` 随之改为从 `zh` 推导。
- 新增 ESLint（flat config）与 Prettier：`npm run lint`、`npm run format:check` 进 CI，
  生成物（`api-types.gen.ts`、`routeTree.gen.ts`）不参与。首次接入按既有风格统一格式。
- i18n 字典覆盖测试：新增"代码里引用的键必须存在"与"字典里没有死键"两条断言，
  删掉 13 个无人引用的键（`entity.*`、`apiKeys.disabled/enabled/scopeAria`、
  `certificates.issueStarted/saved`、`common.add/confirm`）。
- 删除 `frontend/e2e/auth-e2e.mjs` 与 `make e2e-auth`：真实登录流程已由 Playwright
  的 `e2e/specs/auth.spec.ts` 覆盖。
- 源码与文档里的 Unicode 标点（U+2014/U+2019/U+2192/U+2026）换成 ASCII，
  本地化文案除外；`AGENTS.md` 的规则改写成按码位描述，不再自相矛盾。
- 清理 `web-app-fs` 残留（`storage.ts` 的探针键与 s3 注释）与 `vite.config.ts` 的
  session cookie 陈旧注释；`AGENTS.md` 的"禁止任何 Unicode 字符"改为"只用 ASCII 标点"。

- 不再跟踪构建产物 `backend/internal/frontend/dist/index.html`：整目录忽略，
  只留 `.gitkeep`；未构建时 SPA 回落改为 503 JSON 而不是文件服务器的 404。
- `src/api-types.gen.ts` 改为入库，CI 重新生成后比对 `git diff`。
  此前它被忽略，契约漂移只能等到本地构建时才暴露。
- `make backend` 依赖 `frontend`（此前只有 `build` 按顺序调用两者，`make -j backend`
  会把旧的或空的 dist 嵌进二进制）。
- `/metrics` 在 `metrics.enabled=false` 时返回显式 JSON 404（行为不变）。
- 依赖改为精确版本并统一到同一版本线；`@playwright/test` 取代裸 `playwright`。
- 镜像改用 `node:24-slim` 与 `golang:1.27-trixie`，运行层 `scratch`：只剩静态二进制、
  CA 证书与三个空挂载点，从 180 MB 降到 49 MB。healthcheck 改为 `ddns-server -healthcheck`
  （exec 形式，探 `/api/health`），镜像里不再有 wget；tzdata 由 `time/tzdata`
  编进二进制，`TZ` 照常生效。
- 清理 `web-app-fs` 残留：`storage.ts` 的探针键与 s3 相关注释；
  `vite.config.ts` 里关于 session cookie 的陈旧注释改为 Bearer 说明。
- `AGENTS.md` 的"禁止任何 Unicode 字符"改为"优先 ASCII 标点"，并说明
  本地化文案与 DNSPod 上游中文错误串不受此限。
- compose 不再依赖外部 `proxy` 网络：使用项目默认网络并把 8080 发布到宿主机
  （`DDNS_PORT` 可改），反代指向宿主机端口即可，不必与服务同网络。
- 未配置腾讯云账号时，读取账号列表的失败只在同一浏览器提示一次；此前每次窗口
  聚焦触发的 refetch 都会再弹一条同样的 toast。判定依据是错误体新增的可选 `code`
  （目前只有 `no_account`），后端改文案不会再让这个提示退化成每次刷新弹一次。

## [2.0.0] - 2026-10-25

Go 重写。

### 新增

- 域名解析管理：记录增删改查、批量新增/删除、启用暂停、线路选择，可替代 DNSPod 控制台
- DDNS 任务：调度器按 `(账号, 域名, 地址族)` 分组执行，整组共享一次 IP 探测与记录列表
- 变更历史：按任务与按主机两套，公网地址存活时长可视化
- 证书：Let's Encrypt DNS-01 签发与自动续期，走已有 DNSPod 账号，无需对外开端口
- 通知：ntfy 与通用 webhook，支持自定义模板与请求头。一次运行只发一条消息--同一主机的多条
  记录合并成一条（含 `records` 列表，模板里可 range `Records`）；单条记录时标题与正文与旧版一字不差
- API Key：供脚本调用的对外 REST API（`/api/v1`），作用域限定到账号 + 域名
- OIDC 登录（授权码 + PKCE），界面中英双语
- 探针 `/api/health`、`/api/ready`，指标 `/metrics`（Prometheus，`metrics.enabled` 默认关）
- `-backup` 一致性快照（`VACUUM INTO`），同名可重复执行，产物 0600
- OpenAPI 契约（`backend/openapi.yaml`）作为载荷形状的唯一来源，前端类型由它生成，
  契约测试与镜像构建都会校验实现没有漂移

### 安全

- 凭证（SecretKey、通知密码）AES-256-GCM 落盘，主密钥 0600；接口只返回 `key_hint`
- `AUTH_DISABLED` 只允许 loopback 监听，否则拒绝启动
- 响应头含 CSP（`frame-ancestors 'none'`、`object-src 'none'`）、Permissions-Policy、COOP/CORP；
  HSTS 由 `server.hsts` 控制，只在 TLS 终止于反代时开
- `server.trusted_proxies` 默认只信任回环，加宽需显式配置
- 通知渠道默认拒绝回环/链路本地/私有/组播地址，校验发生在真正建连时
- API Key 只存哈希，明文仅创建时返回一次
