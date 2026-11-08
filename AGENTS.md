# AGENTS.md

## Project Overview

DNSPod API 3.0 的解析管理 + DDNS。Go 后端为**无状态资源服务器**：无会话、无 Cookie、
无 trusted header，只验 `Authorization: Bearer`。
- 技术栈：Gin / GORM + 纯 Go SQLite / Viper / robfig/cron / 腾讯云 DNSPod SDK /
  golang-jwt；React 19 + Vite + Tailwind CSS v4 + shadcn-admin /
  TanStack Router + Query + Table / Recharts
- 前端是 OIDC public client（授权码 + PKCE），产物 `go:embed` 进单二进制

## Commands

```bash
make build            # = frontend + backend，产出 bin/ddns-server
make dev-frontend     # :5173，/api 代理到 8080
make dev-backend      # 后端 :8080
cd frontend && npm run typecheck && npm test
cd backend && go test ./...
make types            # 刷新 src/api-types.gen.ts（prebuild/pretypecheck 已自动跑）
cd backend && go test -run TestContract ./internal/api/   # 契约门禁
cd frontend && npm run test:e2e
cd backend && go test -tags=e2e ./e2e/ -v
./bin/ddns-server -c config.yml -backup backups/ddns-$(date +%F).db
make version
```

改前端必须 `make build` 重编后端；改路由文件后先 `npx vite build` 重新生成 `routeTree.gen.ts`。

## Guidelines

- **只用 ASCII 标点**：代码、注释、文档与提交信息里用 `-`、`->`、`...`。禁用
  U+2014、U+2013、U+2192、U+2026。本地化文案（`i18n/locales`）与 DNSPod 上游错误串不受此限

### 特别说明

- **鉴权**：`/api/*` 走 `middleware.Bearer`（401 原因只进日志，带 request id）；
  `/api/v1/*` 是 CLI 的 API Key 接口，不走 Bearer；`/api/health` 开放；
  `authRequired=false`（后端 `AUTH_DISABLED=true`）时守卫放行
- **API Key 作用域**：密钥绑定一个账号（`key.AccountID` 覆盖 query 的 `account_id`）与一个域名，
  `model.APIKeyWildcard`（`*`）表示该账号全部域名；域检查只在 `model.APIKeyCovers` 实现
- **列表一律分页**（`page`/`page_size`，上限 `maxPageSize`），包括账号/证书/密钥/渠道；
  `lib/lists.ts` 的 `collectPages` 逐页读到全量（上限 `MAX_LIST_PAGES`），`mapBounded`
  是 Run All / Test All 的并发上限入口
- **响应**：成功 `{"data":...}`，失败 `{"error":"...","request_id":"..."}`，可带可选 `code`
  （枚举在 `openapi.yml`）。出口只有 `middleware.WriteJSON` / `WriteError` / `WriteCodedError`；
  内部错误只回通用文案，禁止返回 `err.Error()`
- **探针**：`/api/health` 存活、`/api/ready` 就绪（数据库 + IdP）。容器 healthcheck 是
  `ddns-server -healthcheck`（探 `/api/health`），地址从配置推导，环境变量优先；
  用 `-c` 改 `server.listen` 时探针仍探默认端口。`/metrics` 由 `metrics.enabled` 控制，默认关
- **日志**：`log/slog` 写 stderr；`server.log_format`/`server.log_level` 可调；访问日志带 `rid`/`caller`
- **备份**：用 `-backup <path>`，与服务器同一份 `-c`；运行中拷 `ddns.db` 会丢 WAL 写入
- **契约**：`backend/openapi.yml` 是载荷形状的唯一来源，`src/api-types.gen.ts` 由它生成并入库
  （CI 校验漂移）；`contract_test.go` 双向比对路由表并校验响应
- **DDNS 执行**：按 `(账号, 域名, 地址族)` 分组，整组共享一次 IP 探测与记录列表，组内串行；
  以 DNSPod 真实记录为准，同主机同类型同线路只取第一条；探测源逐个尝试；失败只在首次写历史，
  SKIP 不写历史；通知按运行聚合（`buildRunMessage`），整组结束发一次
- **两套历史**：`ddns_history` 按任务；`public_ip_changes` 按主机（一个地址族一行，首行是基线，
  `/stats/ip-changes` 排除）
- **数据模型**：`DDNSTask` 声明 `Account` 关联，故 `account_id` 有外键；`Certificate` 不声明。
  删账号的引用检查在 `deleteAccount`（任务 + 证书），外键是兜底
- **证书**：`internal/acme` 只做 DNS-01，TXT 记录复用 `internal/dnspod` 与所选账号；文件写
  `<acme.output_dir>/<子目录>/{cert,chain,fullchain,privkey}.pem`（certbot 命名，临时文件 + rename，
  私钥权限 `acme.key_mode` 默认 0600，链为空时删掉上一轮 `chain.pem`）；签发后台执行，
  `last_status`（RUNNING -> OK/FAIL）是 UI 唯一进度（接口 202）；ACME 账号密钥加密存
  `acme_accounts`（按 directory 唯一，撞索引时回读）；删证书不碰磁盘；`/config.json` 下发
  `acmeEnabled`（= `acme.enabled`），前端据此隐藏入口，服务端 503 是唯一权威
- **证书并发与关停**：同时最多 `maxConcurrentIssuances`（2）张，排队时间算进该 run 的超时；
  关停顺序停调度器 -> 关 HTTP -> `Manager.Wait`（最多 10s）-> 关库；`inflight.Add` 必须在
  goroutine 启动前；`Start` 的 goroutine 带 recover；`notify` 的域名列表可能为空
- **缓存**：`dnspod/cache.go` 泛型 TTL 缓存规避 429；`clientCache` 按账号复用，账号增删改时 `remove()`
- **配置**：默认值 -> YAML(`-c`) -> 环境变量（`DDNS_` + 大写路径）。每个 key 都要 `SetDefault`；
  `ipdetect` 地址池默认为空；只有 `notifier.allow_private` 能用环境变量传列表；
  `DDNS_CRYPTO_KEY` 优先于 `crypto.key_file`；`Validate()` 只告警
- **前端运行时配置**：SPA 的 `/config.json` 由服务端用 `oidc.*` + `frontend.*` + `acme.enabled`
  现算；`frontend/public/config.json` 只在非 Go 静态托管时有意义
- **测试夹具**：`internal/api` 的 `newTestServer` 用 `:memory:`；要重开库的测试自己给路径
- **语言**：`dnspod` 中匹配上游错误的中文串（SDK 默认 zh-CN，翻译即失效），线路名 `"默认"`

### 前端约定

- **i18n**：`src/i18n/locales/zh.ts` 是源字典，`en.ts` 以 `typeof zh` 约束，缺键/多键是编译错误；
  新增文案先加 `zh.ts`。插值写 `{name}`，数量文案用 i18next 复数（`key`/`key_one`/`key_other`，
  传 `{ count }`）。组件用 `useTranslation()`，非 React 模块用 `i18n.t()`
- **列定义必须是 hook**：`useXxxColumns(actions)`，不能在 `useMemo` 里调工厂函数
- **zod schema 在组件内构造**（`xxxSchema(t)` + `z.infer<ReturnType<typeof xxxSchema>>`）
- **导航存 message key**：`sidebar-data.ts` 的 `titleKey` 在 `NavGroup` 里 `t()`
- **`i18n.test.ts` 的 ALLOWED**：只放输入示例与产品术语（`home`、`example.com`、`SecretId`）
- **分层**：`lib/http.ts` 是 HTTP 内核，不引入 i18n；错误文案在 `lib/describe-error.ts`；
  业务端点在 `features/<name>/api.ts`
- **错误态**：查询失败用 `components/error-state.tsx`（原因 + 重试），判定用 `isError`；
  已有数据时的刷新失败要保留屏幕内容
- **未配置账号提示**：`lib/query-client.ts` 按 `code: no_account` 判定，同一浏览器只 toast 一次；
  保存账号后 `clearMissingAccountNotice()` 复位。加 code 先改 `openapi.yml` 枚举
- **开放重定向**：`lib/redirect.ts` 的 `safeReturnPath` 是唯一判定；先剥控制字符，后判 `//`
- **跨标签页登出**：`lib/session-events.ts`；`signOut` 广播，`discardSession` 不广播
- **字体自托管**：`styles/fonts.css` 引 `@fontsource-variable/*`；加 CDN 链接会被 CSP 拦掉

### 安全约定

- 不要在 handler 里另写 `c.JSON` 拼错误体（见"响应"）
- `/metrics` 无鉴权：关闭时显式回 404 JSON，不能留给 SPA 兜底
- `server.trusted_proxies` 默认只有回环；反代在别的网段时显式加
- 安全头在 `middleware.SecureHeaders`（`frame-ancestors 'none'`、`object-src 'none'`、
  `font-src 'self'`），HSTS 由 `server.hsts` 控制，只在 TLS 终止于反代时开
- 凭证（SecretKey、ntfy 密码）AES-256-GCM 落库，主密钥 `data/secret.key`（0600，必须备份）；
  响应只给 `key_hint`
- 通知渠道默认拒绝回环/链路本地/私有/组播，校验在真正建连时（`GuardURL` +
  自定义 `DialContext`）；放行内网只能用 `notifier.allow_private`，不可做成渠道级

## Project Decision-making

1. `autoCodeSplitting` 关闭（反代链对动态 chunk 返回 502）
2. 批量删除记录走 `POST /records/batch-delete`，不是 `DELETE /records/batch`
3. `/assets/` 可永久缓存，`index.html` 必须 `no-store`
4. `os.Exit` 跳过 defer：调度器启动失败时显式关库再退出，别用 `log.Fatalf`
