# AGENTS.md

## Project Overview

DNSPod API 3.0 的解析管理 + DDNS。Go 后端为**无状态资源服务器**：无会话、无 Cookie、
无 trusted header，只验 `Authorization: Bearer`。
- 技术栈：gin / GORM + 纯 Go sqlite / viper / robfig/cron / 腾讯云 dnspod SDK /
  golang-jwt；React 19 + Vite + Tailwind v4 + shadcn-admin / TanStack Router+Query+Table / recharts
- 前端是 OIDC public client（授权码 + PKCE），产物 `go:embed` 进单二进制

## Commands

```bash
make build            # = frontend + backend，产出 bin/ddns-server
# 改前端必须 `make build` 重编后端（产物 embed 进二进制），
# 只跑 `npm run build` 对运行中的进程无效
make dev-frontend     # :5173，/api 代理到 8080
make dev-backend      # 后端 :8080
cd frontend && npm run typecheck && npm test
cd backend && go test ./...
# 改路由文件后先 npx vite build 重新生成 routeTree.gen.ts，再跑 tsc
make types            # 显式刷新 src/api-types.gen.ts（prebuild/pretypecheck 已自动跑）
cd backend && go test -run TestContract ./internal/api/   # 契约门禁
cd frontend && npm run test:e2e                          # Playwright, 真二进制 + 本地 IdP
cd backend && go test -tags=e2e ./e2e/ -v                # Go e2e, 直接驱动构建出的二进制
./bin/ddns-server -c config.yml -backup backups/ddns-$(date +%F).db  # 一致性快照后退出
make version          # 打印构建进二进制的版本（唯一来源是 backend/openapi.yml 的 info.version）
```

## Guidelines

- **项目还在快速开发中，不考虑前向兼容与迁移**
- **禁止任何无用户批准的文档修改**
- **只用 ASCII 标点**：代码、注释、文档与提交信息里用 `-`、`->`、`...`。禁用
  U+2014（em dash）、U+2013（en dash）、U+2192（arrow）、U+2026（ellipsis）。
  面向用户的本地化文案（`i18n/locales`）按各自语言的习惯书写，不受此限；
  DNSPod 上游错误串必须原样匹配，也不受此限

### 特别说明

- **鉴权**：`/api/*` 走 `middleware.Bearer`（401 原因只进日志，带 request id）；
  `/api/v1/*` 是给 CLI 的 API Key 接口，故意不走 Bearer；`/api/health` 开放；
  前端 `authRequired=false`（对应后端 `AUTH_DISABLED=true`）时守卫放行
- **API Key 作用域**：密钥绑定一个账号（`key.AccountID` 覆盖 query 的 `account_id`）
  与一个域名，`model.APIKeyWildcard`（`*`）表示该账号下全部域名；域检查只有
  `model.APIKeyCovers` 一处实现，改作用域语义时别在中间件里另写一份
- **列表一律分页**（`page`/`page_size`，上限 `maxPageSize`），包括账号/证书/密钥/渠道这几个小表；
  `lib/lists.ts` 的 `collectPages` 逐页读到全量（上限 `MAX_LIST_PAGES`，触顶时页面写
  `Showing the first N of M`），`mapBounded` 是所有批量动作
  （Run All / Test All）的并发上限入口，别回到 `forEach` 直接扇出
- **响应**：成功 `{"data":...}`，失败 `{"error":"...","request_id":"..."}`，出口只有
  `middleware.WriteJSON` / `middleware.WriteError`；内部错误只回通用文案，禁止返回 `err.Error()`
- **探针**：`/api/health` 存活、`/api/ready` 就绪（后者才是容器 healthcheck）。`/metrics` 走
  `metrics.enabled`，默认关
- **日志**：一律 `log/slog` 写 stderr；`server.log_format`/`server.log_level` 可调（客户端取消记在
  debug）；访问日志带 `rid`/`caller`
- **备份**：用 `-backup <path>`，别在运行中拷 `ddns.db`（WAL 会丢未 checkpoint 的写入）；
  该参数需要和服务器**同一份 `-c` 配置**
- **契约**：`backend/openapi.yml` 是载荷形状的唯一来源，`src/api-types.gen.ts` 由它生成（不入库）；
  `contract_test.go` 双向比对路由表并校验响应
- **DDNS 执行**：调度器按 `(账号, 域名, 地址族)` 分组，整组共享一次 IP 探测与
  记录列表，组内串行更新；以 DNSPod 真实记录为准；同主机同类型同线路只取
  第一条；探测源逐个尝试；失败只在首次写历史，SKIP 不写历史；通知按运行聚合（`buildRunMessage`），
  每条记录只标记要不要通知，发送在整组结束时做一次
- **两套历史**：`ddns_history` 按任务（N 个同族任务记 N 遍）；
  `public_ip_changes` 按主机（一个地址族一行，首行是基线，`/stats/ip-changes` 排除）
- **数据模型**：`DDNSTask` 声明了 `Account` 关联（`Preload("Account")` 要用），因此
  `account_id` 有外键；`Certificate` 不声明，无外键。删账号的引用检查写在 `deleteAccount`
  （任务 + 证书都要查），外键只是漏检时的兜底
- **证书**：`internal/acme` 只做 DNS-01（容器不对外开端口），TXT 记录复用
  `internal/dnspod` 与证书所选账号；文件写
  `<acme.output_dir>/<子目录>/{cert,chain,fullchain,privkey}.pem`（certbot 同名，
  临时文件 + rename 原子替换，私钥权限 `acme.key_mode` 默认 0600，这次的链为空时要删掉
  上一轮的 `chain.pem`）；签发一律后台跑，行里的 `last_status`（RUNNING -> OK/FAIL）是 UI
  唯一的进度来源（接口返回 202）；ACME 账号密钥加密存 `acme_accounts`（按 directory 唯一，
  建号撞唯一索引时回读既有行），删证书不碰磁盘文件；`/config.json` 下发 `acmeEnabled`
  （= `acme.enabled`），前端据此隐藏/禁用签发入口，但服务端 503 仍是唯一权威--
  UI 只是提前说，不是权限
- **证书的并发与失败**：同时最多 `maxConcurrentIssuances`（2）张，排队时间算进该 run 自己
  的超时；关停顺序是停调度器 -> 关 HTTP -> `Manager.Wait`（最多 10s，让在途 run 把
  `_acme-challenge` 记录删掉）-> 关库，`inflight.Add` 必须在 goroutine 启动前（`Add` 与
  `Wait` 并发是 WaitGroup 误用）；`Start` 的 goroutine 带 recover--panic 会带走同进程的
  HTTP 与 DDNS 调度；`notify` 的域名列表可能为空（域名不可解析的行），不要索引它
- **缓存**：`dnspod/cache.go` 泛型 TTL 缓存规避 429；`clientCache` 按账号复用
  客户端，账号增删改时记得 `remove()`
- **配置**：默认值 -> YAML(`-c`) -> 环境变量（`DDNS_` + 大写路径）。每个 key 都必须有
  `SetDefault`，否则它的 `DDNS_*` 环境变量是空操作（viper 只覆盖它已知的 key）；
  `ipdetect` 地址池默认为空（部署决策）；只有 `notifier.allow_private` 能用环境变量
  传列表；`DDNS_CRYPTO_KEY` 优先于 `crypto.key_file`；`Validate()` 只告警不退出
- **前端运行时配置**：SPA 的 `/config.json` 由服务端用 `oidc.*`（disabled/issuer/client_id）
  + `frontend.*`（展示项）+ `acme.enabled`（能力位）现算，镜像里不含部署值；
  `frontend/public/config.json` 只在非 Go 静态托管时有意义（那条路径用构建期
  `AUTH_REQUIRED` / `ACME_ENABLED` 注入）
- **测试夹具**：`internal/api` 的 `newTestServer` 用 `:memory:`--文件库的首次迁移是
  fsync 成本（125 个用例 131s -> 0.4s）；store 固定单连接，schema 活得够久，
  但**要重开库的测试必须自己给路径**
- **语言**：`dnspod` 中匹配上游错误的中文标记字符串（SDK 默认 zh-CN，翻译即失效），DNSPod 线路名 `"默认"`

### 前端约定

- **i18n**：`src/i18n/locales/zh.ts` 是源字典（界面文案用中文写），`en.ts` 以 `typeof zh`
  约束，缺键/多键是编译错误；新增文案先加 `zh.ts`。插值写 `{name}`，数量文案用 i18next 复数
  （`key`/`key_one`/`key_other`，调用传 `{ count }`）。组件里 `const { t } = useTranslation()`，
  非 React 模块（`describeError`、`main.tsx` 的 toast）用 `i18n.t()`。
- **列定义必须是 hook**：`useXxxColumns(actions)`，不能在 `useMemo` 里调工厂函数--钩子规则不允许它读 `t()`，
  那样列标题会冻结在首次渲染的语言。
- **zod schema 在组件内构造**（`xxxSchema(t)` + `z.infer<ReturnType<typeof xxxSchema>>`）：模块级 schema 会把
  校验消息冻结在首次导入的语言。
- **导航存 message key**：`sidebar-data.ts` 的 `titleKey` 在 `NavGroup` 里 `t()`，否则切换语言不生效。
- **`i18n.test.ts` 的 ALLOWED 是显式决策**：只放输入示例与产品术语（`home`、`example.com`、`SecretId`），
  每加一条都要能说出"为什么不翻译"。
- **分层**：`lib/http.ts` 是 HTTP 内核（axios 实例、`ApiError`、`request`），不引入 i18n；错误文案在
  `lib/describe-error.ts`。业务端点在 `features/<name>/api.ts`。
- **错误态**：查询失败用 `components/error-state.tsx`（原因 + 重试），别让表格显示"暂无数据"--
  那是在撒谎。判定用 `isError`，已有数据时的刷新失败要保留屏幕内容。
- **开放重定向**：`lib/redirect.ts` 的 `safeReturnPath` 是唯一判定，别在别处再写一份前缀检查；
  它先剥控制字符（`/\t/evil.com` 这类），后判 `//`。
- **跨标签页登出**：`lib/session-events.ts`；`signOut` 广播，`discardSession`（过期）不广播--
  另一个标签页的会话被拒不应牵连自己。
- **字体自托管**：`styles/fonts.css` 引 `@fontsource-variable/*`；再加 CDN 链接会被 CSP 静默拦掉。

### 安全约定

- 不要在 handler 里另写 `c.JSON` 拼错误体（见"响应"）。
- `/metrics` 无鉴权：关闭时显式回 404 JSON，不能留给 SPA 兜底（那会变成 200 + index.html）。
- `server.trusted_proxies` 默认只有回环；反代在别的网段时要显式加，加宽到整个私网等于让同网段主机自选日志里的地址。
- 安全头在 `middleware.SecureHeaders`（含 `frame-ancestors 'none'`、`object-src 'none'`、`font-src 'self'`），
  HSTS 单独由 `server.hsts` 控制，只在 TLS 终止于反代时开。
- 凭证（SecretKey、ntfy 密码）AES-256-GCM 落库，主密钥 `data/secret.key`（0600，
  丢失即永久无法解密，必须备份）；响应只给 `key_hint`，永不含明文
- 通知渠道默认拒绝回环/链路本地/私有/组播，校验发生在真正建连时（`GuardURL` +
  自定义 `DialContext`）；放行内网只能用 `notifier.allow_private`，
  不可做成渠道级

## Project Decision-making

1. `autoCodeSplitting` 关闭是有意的（反代链对动态 chunk 返回 502）
2. 批量删除记录走 `POST /records/batch-delete`，不是 `DELETE /records/batch`
3. `/assets/` 可永久缓存，`index.html` 必须 `no-store`
4. `os.Exit` 跳过 defer：调度器启动失败时显式关库再退出，别用 `log.Fatalf`