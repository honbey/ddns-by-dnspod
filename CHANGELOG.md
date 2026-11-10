# Changelog

格式参考 [Keep a Changelog](https://keepachangelog.com/)，版本号遵循 [SemVer](https://semver.org/lang/zh-CN/)。

**2.0.0 是 Go 重写**：单二进制交付，SPA 用 `go:embed` 打进进程。
v1.1.0 及更早版本是 Python 实现，变更记录以各自 tag 为准。

## [Unreleased]

## [2.1.0] - 2026-11-10

### 新增

- DNSPod 域名分组：`GET /api/domain-groups` 按账号列出分组，`Domain` 带回
  `group_id`，域名页新增分组列与分组筛选。域名分组与单域名内的记录分组互不混淆。
- DDNS 任务批量创建：`POST /api/ddns/tasks/batch` 一次事务创建多个子域名并返回
  `created` / `skipped` / `task_ids`，只重建一次调度。任务弹窗支持空格或英文逗号
  分隔多个子域名，编辑模式仍是单值。
- 任务去重：`(账号, 域名, 子域名, 记录类型, 线路)` 判定同一任务，调度表达式不参与；
  新建、编辑、分组导入与批量创建共用一套判定。单条冲突返回带 `duplicate_task`
  码的 409 并按码翻译，批量与导入跳过已存在的主机。
- 主题在首帧就位：`public/theme-init.js` 在模块启动前确定配色，消除深色用户的白闪；
  `:root` 与 `.dark` 声明 `color-scheme`，原生滚动条、日期控件与自动填充一起切换；
  `theme-color` 由 `ThemeProvider` 维护并与 `--background` 对齐。
- 主题偏好契约：`lib/theme.ts` 收拢枚举、默认值、存储键与 `parseTheme`，首帧脚本
  保留等价白名单。存储降级按写探测与 `lib/storage.ts` 对齐，`Toaster` 改用
  `resolvedTheme`。
- 按时段招呼语：Dashboard 副标题在本地 05/12/18/23 点切换中英文案，按整点重排
  定时器并在标签页恢复可见时校准；`msUntilNextHour` 覆盖 DST 与时钟回拨。
- 主题切换动画：太阳与月亮旋转 180 度并淡入淡出，`prefers-reduced-motion` 下关闭。
- 手绘线性图标（证书、域名、账号、语言、IPv4、IPv6）接入侧栏、个人资料下拉与
  仪表盘；证书印章改为五角星。toast 新增 success / warning / info 配色令牌。
- i18n 覆盖测试与设置菜单重排，语言与主题可跟随系统，偏好存储按可用性三级降级。

### 变更

- 版本号从 `2.0.1` 升到 `2.1.0`；`backend/openapi.yml` 的 `info.version` 仍是唯一
  来源，`frontend/package.json` 与 `package-lock.json` 同步，CI 继续校验三者一致。
- 契约收紧：`BatchTaskInput.sub_domains` 声明 `minItems: 1` / `maxItems: 100`，三个
  任务输入的 `record_type` 限定为 `A` / `AAAA`，`api-types.gen.ts` 随契约重新生成。
- 分组导入改用请求体的 `account_id`；API Key 调用仍钉在密钥所属账号。
- 分页器重做为页码输入，侧栏分区改名 General，品牌标与 favicon 改为手绘 D，
  个人资料页与 Dashboard 空态重新排版。
- Playwright 升到 `1.65.0`；ESLint 只对首屏脚本声明浏览器全局，node globals
  不再泄漏到该文件；测试补上 DST 回归、上限漂移与 afterEach 隔离。

### 修复

- 登录跳转不再产生嵌套重定向。
- `displayName` 遇到空 claim 继续往下找，招呼语不会再缺名字。
- 非法的主题存储值与空串统一降级为 `system`，避免空类名触发
  `classList.add('')` 异常，也避免首帧与挂载后配色不一致。
- `theme-color` 不再残留与背景不符的旧色值；时钟回拨时定时器不再以 1ms 空转。

## [2.0.1] - 2026-11-09

### 新增

- `.github/workflows/release.yml`：标签推送或手动触发时，用 workflow 自带的
  `GITHUB_TOKEN` 构建镜像并推送到 `ghcr.io/honbey/ddns-by-dnspod`（版本 tag 与 `latest`），
  再按本段内容创建 GitHub Release。`ci.yml` 仍只做构建验证，不推送镜像。

### 变更

- 版本号从 `2.0.0` 升到 `2.0.1`。`backend/openapi.yml` 的 `info.version` 仍是唯一来源，
  `frontend/package.json` 与 `frontend/package-lock.json` 同步；`-version`、`/api/health`
  与镜像 tag 都由此推导。

## [2.0.0] - 2026-11-09

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
- OpenAPI 契约（`backend/openapi.yml`）作为载荷形状的唯一来源，前端类型由它生成，
  契约测试与镜像构建都会校验实现没有漂移
- `oidc.opaque_tokens`：opaque token 的 userinfo 回退，默认关闭。只在 issuer 按客户端隔离
  userinfo 时打开，否则 audience 校验无处落地。
- Go 端到端测试（`backend/e2e/`，`-tags=e2e`）：真二进制 + 临时库，覆盖嵌入的 SPA、两个探针、
  `/config.json`、未知 `/api` 路径的 JSON 404、`/metrics` 关闭时的 JSON 404、验签预算 429。
  `make test-e2e` 与 CI 都会跑。
- 验签拒绝预算：`server.reject_budget_per_minute`（默认 30）。同一来源每分钟失败超过该值
  返回 429；只计失败，有效 opaque token 不受影响。
- Playwright 端到端：`npm run test:e2e` / `make test-e2e`，由 `e2e/server.sh` 起真二进制与本地 IdP。
- `.editorconfig`、`.npmrc`、`.github/renovate.json`、`.env.example`。
- 变更历史页改为 Tabs：任务历史与 IP 历史共用一页。IP 历史分页浏览，支持 CSV 导入
  （`time,ip[,family]`，无偏移时间按 UTC 解析），坏行按行号报告，重复地址不产生新行。
- `GET /api/history/ip-changes`（分页）与 `POST /api/history/ip-changes/import`（multipart）；
  导入把样本按时间并入解析时间轴，既有行不被删除。
- Playwright 用例覆盖历史页的 tab 切换与 CSV 导入。

### 变更

- OIDC 的 discovery、JWKS 与 userinfo 请求逐跳校验可信 URL 并限制重定向次数；`jwks_uri` 与
  `userinfo_endpoint` 同样要求 https 或 loopback。
- opaque token 的 userinfo 结果增加短期正/负缓存与全局/按来源并发门；未知 token 超额后
  每 5 秒最多探测一次。
- 前端登录回调和 provider metadata 查询都加时限；登出在 provider 失败或超时时仍清除本地状态。
- 本地 IdP 统一为 `frontend/e2e/mock-idp.mjs`（RS256 JWT，无依赖），替换 `e2e/idp.mjs`。
- issuer 既不是 https 也不在 loopback 时被永久拒绝（`configErr`）。
- 环境变量设为空值表示清空列表，例如 `DDNS_NOTIFIER_ALLOW_PRIVATE=""`。
- `/config.json` 的拉取与登出时的令牌撤销都限时。
- e2e 的本地 IdP 显式开启 `DDNS_OIDC_OPAQUE_TOKENS`，生产保持默认关闭。
- 历史页的域名筛选改用共享的空数组，`useMemo` 才能命中。
- 版本号只写一处：`backend/openapi.yml` 的 `info.version`，经 `-ldflags -X` 打进二进制，
  由 `-version` 与 `/api/health` 对外；CI 校验 `frontend/package.json` 一致。
- i18n 源字典改为中文：`zh.ts` 是源，`en.ts` 以 `typeof zh` 约束；`MessageKey` 与
  `i18next.d.ts` 从 `zh` 推导。
- 新增 ESLint（flat config）与 Prettier，进 CI；生成物不参与格式化。
- i18n 字典覆盖测试：引用键必须存在、字典里没有死键；删掉 13 个未引用的键。
- 删除 `frontend/e2e/auth-e2e.mjs` 与 `make e2e-auth`，登录流程由 Playwright 覆盖。
- 源码与文档里的 Unicode 标点换成 ASCII，本地化文案与 DNSPod 上游错误串除外。
- 清理 `web-app-fs` 残留与 `vite.config.ts` 的陈旧 session cookie 注释。
- `backend/internal/frontend/dist` 整目录忽略，只留 `.gitkeep`；未构建时 SPA 回落 503 JSON。
- `src/api-types.gen.ts` 入库，CI 重新生成后比对 `git diff`。
- `make backend` 依赖 `frontend`，`make -j` 不会嵌入旧的 dist。
- `/metrics` 关闭时返回显式 JSON 404。
- 依赖改为精确版本；`@playwright/test` 取代裸 `playwright`。
- 镜像运行层改为 `scratch`：静态二进制 + CA 证书 + `/data` `/certs` `/tmp`，180 MB -> 49 MB；
  tzdata 由 `time/tzdata` 编进二进制。
- 容器 healthcheck 改为 `ddns-server -healthcheck`（exec 形式，探 `/api/health`），不再需要 wget。
- compose 使用项目默认网络，8080 发布到宿主回环（`DDNS_PORT` 可改）；README 部署改为复制
  `.env.example`，腾讯云凭证可留空、登录后在界面添加。
- 错误响应新增可选 `code`（目前只有 `no_account`）。未配置账号时的失败提示按 code 判定，
  同一浏览器只弹一次，保存账号后复位。
- `/api/stats/ip-changes` 的 `family` 从带 `default: v4` 的 `Family` 换为无默认值的
  `FamilyFilter`，与服务器空值表示两个地址族的既有行为一致；非法值由 200 空列表改为 400，
  两个历史 GET 的 `400` 已补进契约。
- 导入结果的 `IPHistoryImportError` 新增 required 枚举 `code`（`time_invalid`、`time_future`、
  `time_too_old`、`ip_invalid`、`family_invalid`、`family_mismatch`、`row_invalid`、
  `duplicate_row`），前端按 code 翻译；`message` 只作日志。
- CSV 导入识别 UTF-8 BOM（Excel 导出的表头），请求体超过上限时说明大小限制，
  不再误报缺少 file 字段。

### 安全

- 凭证（SecretKey、通知密码）AES-256-GCM 落盘，主密钥 0600；接口只返回 `key_hint`
- `AUTH_DISABLED` 只允许 loopback 监听，否则拒绝启动
- 响应头含 CSP（`frame-ancestors 'none'`、`object-src 'none'`）、Permissions-Policy、COOP/CORP；
  HSTS 由 `server.hsts` 控制，只在 TLS 终止于反代时开
- `server.trusted_proxies` 默认只信任回环，加宽需显式配置
- 通知渠道默认拒绝回环/链路本地/私有/组播地址，校验发生在真正建连时
- API Key 只存哈希，明文仅创建时返回一次
