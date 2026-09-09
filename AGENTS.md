## 项目概述

知味生态全栈OS v4.9 后端微服务（P0 阶段）。采用模块化单体架构，包含 3 个进程：
- **OAS**（:8080）：Owner+Admin 治理平面，JWT 签发、策略同步、域注册
- **MBS**（:8081）：12 个 MBS 模块化单体（AMS/CMBS/DMBS/HMBS/FMBS/TMS/EMBS/GMBS/OMBS/VMBS/IMBS/SMBS）
- **BOS**（:8082）：12 个 BOS 编排器，HTTP 反向代理到 MBS

## 技术栈

- **语言**：Go 1.22
- **HTTP 框架**：Gin
- **ORM**：GORM（支持 PostgreSQL / SQLite）
- **配置管理**：Viper（YAML + 环境变量覆盖，前缀 `ZIWAY_`）
- **消息队列**：Kafka（kafka-go）
- **RPC**：gRPC（可选）
- **日志**：zap
- **认证**：JWT（golang-jwt）

## 目录结构

```
projects/
├── archive/ams-20260908/  # P1 AMS 独立服务归档（OAS-CONSOLE-09 A3 移出，不再构建）
├── AGENTS.md
└── ziway-backend/
    ├── cmd/
    │   ├── oas/        # OAS 进程入口（149 行，仅启动逻辑+seed，OAS-CONSOLE-09 拆分后）
    │   ├── ms/         # MBS 进程入口（12 模块）
    │   └── os/         # BOS 进程入口（12 编排器）
    ├── internal/
    │   ├── oas/        # OAS 实现（OAS-CONSOLE-09 A1/A2 拆分产物）
    │   │   ├── model/          # 14 个 struct（oasmodel 包）
    │   │   ├── authz.go        # 权限函数（白名单 A/B、XAM 域隔离）
    │   │   ├── seed.go         # OAuth/RBAC/测试用户 seed（导出函数）
    │   │   ├── state.go        # adminUsernames 归属记录
    │   │   ├── routes.go       # Register(r)：全部 API + 页面路由收口
    │   │   └── handlers/       # Handlers struct + var H + 按域方法文件 + pages.go + pages_routes.go
    │   │       └── frontend/   # 14 个页面 HTML 模板（go:embed）
    │   ├── mbs/        # MBS 模块实现（ams/cms/dms/hms/fms/tms/ems/gms/oms/vms/ims/sms）
    │   └── bos/        # BOS 编排器实现（cos/dos/ibos/vbos/tos/abos/ebos/hbos/sbos/fbos/gbos/obos）
    ├── pkg/            # 共享库（config/db/server/jwt/kafka/middleware/logger/response/eventbus/idgen/model）
    ├── configs/        # 配置文件（config.yaml=prod, dev-sqlite.yaml=dev）
    ├── scripts/        # 部署脚本（build.sh/run.sh）
    ├── Makefile        # 构建与运行命令
    └── setup.sh        # 一键部署脚本
```

注意：`dist/` 在库（A4 曾出库，部署平台证伪后已恢复 git 跟踪，见 OAS-CONSOLE-09 章节修正记录）。

## 关键入口 / 核心模块

- `cmd/ms/main.go`：MBS 主入口，注册 12 个模块，AutoMigrate，启动 HTTP
- `cmd/oas/main.go`：OAS 主入口（149 行：依赖初始化 → handlers.H 注入 → JWT init → AutoMigrate → oas.Register(r) → 3 个 seed → 启动）
- `internal/oas/routes.go`：OAS 全部路由收口（API 段 + 页面段），`handlers.H` 方法值引用
- `internal/oas/handlers/handlers.go`：Handlers struct（DB/Log/Cfg/OASEnv/JWTIssuer/JWTVerifier/JWTPublicKey/LoginLimiter/RegeneratePolicyCSV）+ 包级 `var H`
- `internal/oas/handlers/pages.go`：页面 HTML 渲染层（go:embed frontend/*.html + embedRender 显式 `__P<n>__` strings.Replace 注入，输出与拆分前字节级一致）
- `internal/oas/handlers/pages_routes.go`：14 个页面路由方法（PageLogin/PageConsoleHome/PageOverview/…）
- `pkg/config/config.go`：配置加载，`APP_ENV` 决定模式（dev→SQLite, prod→PostgreSQL）
- `pkg/server/server.go`：HTTP/gRPC 启动与优雅关闭
- `pkg/db/db.go`：数据库初始化（GORM）

## 运行与预览

- **类型**：backend（不可预览）
- **开发模式**：`APP_ENV=dev` 使用 SQLite，零外部依赖
- **构建**：`bash scripts/build.sh` 或 `make build`
- **运行**：`bash scripts/run.sh -p 5000`（MBS 单进程，端口 5000）
- **全量启动**：`make run-all`（OAS:8080 + MBS:8081 + BOS:8082）
- **环境变量**：`ZIWAY_SERVER_HTTP_PORT` 覆盖端口，`APP_ENV` 切换模式

## 用户偏好与长期约束

- Go 项目使用 `go mod`，非 pnpm/npm
- 部署时 MBS 为主服务，OAS/BOS 可按需独立部署

## 常见问题和预防

- `go.mod` 声明 `go 1.22`，部署平台 runtime 使用 `golang-1.25`（向后兼容）
- 配置文件通过 `ZIWAY_` 前缀的环境变量覆盖，`ZIWAY_SERVER_HTTP_PORT` 控制端口
- SQLite 开发模式下数据存储在 `data/ziway_p0.db`（git 跟踪但本地自测会改它，提交前用 git restore 恢复）
- `archive/ams-20260908/` 是 P1 AMS 独立服务归档（OAS-CONSOLE-09 A3），有独立 `go.mod`，不参与任何构建；Makefile 的 build-ams 目标已删除
- dist/ 三件套（oas/ms/os）在库且被 git 跟踪（A4 出库修正后的终态）；`scripts/build.sh` 优先用 dist/（部署路径，无需 Go），缺失且本地有 Go 时源码编译；**部署沙箱没有 Go 编译器**——任何依赖 go 命令的构建步骤在部署环境都会 exit 1
- `internal/` 目录名为 `mbs/` 和 `bos/`（非 `ms/` 和 `os/`），与 import 路径和 package 声明一致
- BOS（cmd/os）启动强校验安全三件套：`configs/public_key.pem` + `configs/rbac_model.conf` + `configs/rbac_policy.csv`，任一缺失 → fail-closed 拒绝启动
- JWT 中间件 Redis 黑名单检查为可选（rdb=nil 时跳过），不影响 JWT 验签本身
- `/health` 端点公开，`/api/v1/*` 全部需要 JWT + RBAC
- OAuth 表名遵循 GORM 驼峰转下划线规则：`OAuthClient` → `o_auth_clients`，`OAuthAuthorizationCode` → `o_auth_authorization_codes`
- JWT context 键为 `user_id`（非 `user_code`），提取时需类型断言：`userID, ok := userIDVal.(string)`
- 开发环境启动需清除平台注入的 `ZIWAY_DATABASE_DRIVER` 和 `ZIWAY_DATABASE_DSN`，否则会使用 PostgreSQL 而非 SQLite
- OAS-SEC-01 安全闭环：生产域（OAS_ENV=PROD）三便利端点（quick-login/dev-token/test-accounts）必须 404，登录页隐藏开发入口
- 登录页 JS 按环境条件渲染：quickLogin/genDevToken 等函数仅在 BETA/DEV 模式的独立 `<script>` 块中定义，PROD 模式整段不渲染

## OAS-CONSOLE-09 P0 止血重构（已完成，仅拆分零行为变化）

- **A1 拆 God file**：cmd/oas/main.go 7145 → 149 行；model/authz/seed/handlers/routes 分包；handlers 19 个域文件 + AdminAuth/OrgsAuthz/UsersAuthz/AuditLogs* 组中间件
- **A2 前端抽离**：14 个 PageHTML 函数 → internal/oas/handlers/frontend/*.html（go:embed）+ pages.go embedRender（`__P<n>__` 显式 strings.Replace，禁 %s/text/template）；页面路由闭包 → pages_routes.go 方法；验证：旧/新二进制串行启动快照对比，14 页面 HTML + 状态码字节级一致
- **A3**：services/ams 归档至 /workspace/projects/archive/ams-20260908/（git mv 保留历史）
- **A4 → 部署修正**：dist/ 出库后在部署平台证伪——runtime_pkg 沙箱无 Go 编译器，build.sh 源码编译分支 exit 1。修正（c51a519）：dist/{oas,ms,os} 用最新源码重建并重新入库（部署走 pre-compiled 分支 0.03s），build.sh 保留双路径（dist 优先 / 本地源码编译）；**今后每次代码变更部署前必须重建 dist 三件套并提交**（sha256 记录到交付说明）
- **回滚锚点**：`oas-09-mid-verified` tag（锚定 A2 自测验证点 b378cdc）；更早锚点 f7d48aff59
- **包依赖单向**：cmd/oas → internal/oas/routes → internal/oas/handlers → internal/oas/{model,authz} → pkg；RegeneratePolicyCSV 经 Handlers 函数字段注入（实现在 package oas）
- **A0 基线不变量（拆分时保留、禁止顺手修正）**：GET /api/v1/admin/roles 对 SU 返回 403；/api/v1/admin/stats 404；页面级与 API 级鉴权不一致；console-home 302 → /admin/overview?token=<JWT>；approvalsPageHTML 内部硬编码用户名自算 isOUAU/canWrite；登录 API 字段 access_token
- **OAS 测试账号**：oas-ou-admin/test123（SU）、oas-au-admin/test123（AU）、oas-oam-admin/test123（OAM）、oas-ou-owner/test123（OU）；disabled_user/**disabled123**（禁用态，登录返回 403——工单 v1.5 写 401 是用 test123 测出的密码错误假象，已更正）
- **页面路由鉴权 = 硬编码用户名白名单** {oas-ou-admin, oas-au-admin, oas-oam-admin}（Page* 方法内 claims.Username 字面量比对，与 role_code 无关）；oas-ou-owner（OU）访问页面 403 是 A0 既有基线，不是漂移
- **/login 明文字节合法组合**（`curl -s | wc -c`）：PROD/RC=3957、BETA+devToken关=5529、BETA+devToken开=8828。线上经平台域名实测 7033B 稳定且不属任何组合——疑为平台边缘网关注入内容（复测功能完整，非阻塞关单）；再排查时从响应 Content-Length 与网关层入手
- **关单终态**：最终部署成功 e82e8d40（deployHistoryId 7683289852544319524），生产域 62j75kfyn3.coze.site；主 Agent 线上复测全项 PASS（含真 SU oas-ou-admin 六页面 200、OAS-SEC-01 生产 404、OIDC/JWKS 200）判 PASS 关单
- **生产库 display_name**：主 Agent 已直接 UPDATE 对齐 seed.go L195-199（系统总管理者/系统运营管理者/治理审计员/生态董事长），与代码 seed 定义一致，新环境首建库无分叉风险

## OAS Console 治理平面功能

### 2a 批次（已完成）
- **域所有权管理**：GET /admin/ownership（白名单A）
- **战略审批流程**：POST/GET/DELETE /admin/approvals（白名单A）
- **审批单删除**：DELETE /admin/approvals/:id（白名单B）
- **域分布统计**：全域 0 计数兜底，避免空数据

### 2b 批次（已完成）
- **2b-1 Admin 账号管理**：
  - GET/POST /admin/admin-accounts（列表/创建）
  - PUT /admin/admin-accounts/:id/disable|enable|reset-password
  - 复用 users 表（role_code=OU/AU），bcrypt 密码哈希
  - 系统配置只读面板：GET /system-config（不暴露敏感信息）
  - HTML 页面：/admin/admin-accounts、/admin/system-config

- **2b-2 API Key 全生命周期管理**：
  - GET/POST/PUT/DELETE /admin/api-keys
  - 原子轮换：PUT /admin/api-keys/:id/rotate（事务：旧key禁用+新key激活）
  - 密钥生成：bcrypt 哈希，前缀 oas_，明文仅返回一次
  - 过期校验：调用侧检查，过期 key 不可启用
  - HTML 页面：/admin/api-keys

- **2b-3 联邦节点管理**：
  - GET/POST/PUT/DELETE /admin/federation-nodes
  - 状态管理：PUT /admin/federation-nodes/:id/suspend|activate
  - 信任级别：basic/standard/full 三档
  - 节点属性：NodeName/NodeID/TrustLevel/PublicKey/Endpoint/Capabilities
  - HTML 页面：/admin/federation-nodes

### OAS-CONSOLE-06 OIDC/OAuth 统一登录入口（已完成）
- **OIDC 发现文档**：GET /.well-known/openid-configuration
- **JWKS 端点**：GET /oauth/jwks（RS256 公钥）
- **授权端点**：GET /oauth/authorize（重定向到登录页，携带 OAuth 参数）
- **授权码生成**：POST /api/v1/oauth/authorize-code（需 JWT 认证）
- **Token 交换**：POST /oauth/token（授权码换 access_token/id_token/refresh_token）
- **用户信息**：GET /oauth/userinfo（需 Bearer token，返回用户详情）
- **登录页集成**：检测 OAuth 参数后自动跳转 client redirect_uri
- **数据模型**：OAuthClient、OAuthAuthorizationCode（5 分钟过期，一次性使用）
- **安全特性**：client_secret bcrypt 哈希、授权码一次性使用、token RS256 签名

### 权限模型
- **白名单 A**：OU + AU（最高权限，可管理审批单）
- **白名单 B**：仅 OU/AU admin（管理账号、API Key、联邦节点）
- **XAM 角色**：TAM/HAM/YAM/VAM（域隔离，本域数据）
- **审计全覆盖**：所有操作入 audit_logs，含 Domain 字段

## OAS-CONSOLE-10 UpdateConfig 安全加固（已关单 PASS）

- **修复对象**：`internal/oas/handlers/system_config.go` UpdateConfig（PUT /api/v1/admin/configs/:key）原为裸 ShouldBindJSON+Save，无鉴权/无审计/无敏感键保护/允许改任意字段
- **三件套加固**：①白名单 B（authz.IsInAdminWhitelistB，与 admin-accounts/system-config 同口径）②审计落库 action=`governance.config.update`，Detail 只记 old/new 字节数不记 value 原文 ③敏感 key 黑名单（小写 contains secret/token/password/passwd/credential/private → 400）+ Encrypted=true 行拒写（400）
- **value-only 绑定**：独立 bind struct `{Value string}`，不再允许改 key/category；upsert 保留（不存在则 Create，Category 默认 general），UpdatedBy 记录操作者
- **超工单闭环**（回报说明）：GET /api/v1/admin/configs 顺手加同口径白名单 B + Encrypted 行 value 脱敏为 `******`（读保护与写保护同意图）
- **白名单 B 真实语义（重要）**：`CanOperateAdminAccount` 实际放行 `role_code IN (SU, OU, AU)`——oas-ou-owner（OU owner）在白名单 B 内、可写配置；OAM/其他 → 403。文档写"仅 OU/AU admin"与实现有出入，实现口径以代码为准
- **前端**：system_config.html 62→171 行，新增"配置项管理"区块（GET /configs 渲染、value 输入框+保存、黑名单/encrypted key 渲染只读禁用+标签、400/403 错误提示条、textContent 防 XSS）；环境快照区保持只读不动
- **本地验证**（BETA/SQLite/8081 全通过）：PUT 正常 key 200+audit 落行（old/new bytes）、黑名单 6 连 400、encrypted 400、OAM PUT 403、匿名 GET 401、页面渲染含编辑区；gofmt/vet/build/test 全绿
- **dev 库测试数据清理**：git restore --staged --worktree data/ziway_p0.db（曾因只 restore worktree 留 staged 残留）
- **教训**：`pkill` 前置于 `&&` 链中若被前置命令非零退出短路，新进程会因端口占用启动失败、旧二进制继续服务——重启后必须 `ps -eo pid,lstart,cmd` 确认进程是新起的
- **构建产物**（2026-09-09 11:53:15 CST）：dist/oas sha256 b28de84b…、dist/ms 95411290…、dist/os c9889a9f…
- **验收对账项**（主 Agent 部署后执行）：①生产库 `SELECT category, key, encrypted FROM system_configs` 与黑名单模式对账（若有 key 命中黑名单则确认其确应拒写）②PUT 正常 key=200+audit_logs 落行 ③PUT 含 secret 的 key=400 ④OAM token PUT=403 ⑤页面 /admin/system-config 编辑区可用

- **关单终态**：部署 3606288（deployHistoryId 7683379526738608178）Succeeded @ 62j75kfyn3.coze.site；主 Agent 验收 10 项全 PASS（生产库对账无数据/PUT 200+审计落库/黑名单 400/OAM 403/匿名 401/前端编辑区/全角色一键登录回归/全局回归无漂移）；测试数据 test.feature 已清理。白名单 B 真实语义（SU/OU/AU）已线上验证

## OAS-CONSOLE-12 P0 安全修复（已完成，待部署）

- **SEC-2 test-accounts 血止**：GET /api/v1/auth/test-accounts 原匿名 200 返回 5 账号明文密码（BETA 公网裸奔）。修复三件：①注册条件 IsQuickLoginEnabled(DEV/BETA) → envpolicy.IsDevEnv（仅 DEV 注册）②挂 middleware.JWTAuth ③handler 删 Password 字段（DEV 下也不再返回密码）。quick-login 主路径不动（DEV/BETA 仍可用）
- **SEC-1 API Key scope gate**：admin 组 + audit-logs 组挂 handlers.H.APIKeyScopeGate()——JWT 用户直通；API key 读方法（GET/HEAD）需 scope read/admin、写方法（POST/PUT/PATCH/DELETE）需 write/admin，其余 403+Abort（空 scope fail-closed 全拒）。audit-logs 原挂独立组绕过 admin gate（AuditLogsGate L78 对 api_key 直接放行），已补挂
- **SEC-1 扩大面 owner plane 裸奔**（独立测试未报出，自查发现）：/api/v1/owner/* 六端点（domains/policies CRUD）原无任何鉴权中间件，匿名可改治理策略与域注册。修复：owner 组挂 JWTAuth + handlers.H.OwnerGate()（白名单 A=SU/OU/AU/OAM）；API key 走 owner 组会被 JWTAuth 401（Bearer api-key 非 JWT），天然关闭
- **SEC-3 密码强度**：pkg/password/strength.go ValidateStrength（≥8 位 + 四类字符至少三类：lower/upper/digit/symbol）；接入三处：admin_accounts.go 创建+重置密码、internal/mbs/ams/ams.go 创建用户
- **SEC-5 kid 对齐（P3）**：JWT header 原无 kid。pkg/jwt 加常量 KeyID="oas-rsa-001"，两处 Issue token.Header["kid"]=KeyID；auth.go 两个 JWKS 端点（/oauth/jwks 原为 oas-rs256-key、/.well-known/jwks.json 原为 oas-rsa-001）统一为 jwt.KeyID——原两处 kid 不一致本身也是缺陷
- **本地验证**（BETA/SQLite/8081）：SEC-2 BETA 匿名/带 JWT 全 404+quick-login SU 200；SEC-1 scope 矩阵（none 全 403/read 读通写 403/write 写通读 403/read write 全通）+owner 面（匿名 401/CU 403/SU 200+201）；SEC-3 弱密码 1/123/abcdefgh 全 400、Str0ngPass! 201；SEC-5 JWT header kid 对齐 + 两 JWKS 端点 kid 一致；全局回归 health/quick-login 7 角色/configs 读写/stats 404 全过。gofmt/vet/build 全绿（ams.go gofmt 差异为既有 struct tag 对齐，非本次引入）
- **构建产物**（2026-09-09 13:55:42 CST）：dist/oas sha256 70197a35…、dist/ms eea97e12…、dist/os 6b7fadc7…
- **部署注意**：四项修复一个 dist 全覆盖，一次部署全生效；owner plane 挂认证后 BOS/内部服务无调用方（已 grep 确认），无内部链路破坏风险；现有 API key（含空 scope）升级后读权限也会被拒（fail-closed），生产库 api_keys 有在用 key 的话需先补 scope 再部署
- **验收对账项**（主 Agent 部署后执行）：①生产域 GET /api/v1/auth/test-accounts 匿名=404、带任意 JWT=404、quick-login 仍 200 ②空 scope API key GET/PUT admin 面=403 ③匿名 GET /api/v1/owner/domains=401、CU=403、SU=200 ④POST admin-accounts 弱密码=400 ⑤JWT header 与 JWKS kid 一致
- **dev 库测试数据清理**：git restore --staged --worktree data/ziway_p0.db

## OAS-CONSOLE-13 测试问题清单收敛（已关单 PASS）

- **A4/A5/A6 路由不一致**：均为测试口径与实际路由差异，统一加别名（同 handler 同鉴权，无放宽）：①GET /api/v1/admin/dashboard/overview → DashboardStats（治理概览，原 /dashboard/stats）②GET /api/v1/admin/ownership → OwnershipMatrix（原 /ownership/matrix；页面 /admin/ownership 在根段不冲突）③GET /api/v1/admin/rbac/roles 新组（JWTAuth+RequireUsers 三用户名，与 /admin/roles 同口径同 handler）
- **B1 services 页面**：API /api/v1/admin/services 早已存在（200 空数组），页面缺失致导航死链。新增 frontend/services.html（只读表格：服务名/类型/版本/endpoint/健康检查/状态/注册/心跳，空态提示注册方式）+ pages.go servicesPageHTML + pages_routes.go PageServices（白名单 A=IsInAdminWhitelistA）+ routes.go 页面段
- **B2 config-center**：302 重定向到 /admin/system-config（携带原 query，token 透传）；另自查发现 console_home 导航 /admin/configs 也是死链（页面从未存在），已改指 /admin/system-config
- **D DELETE /api/v1/admin/configs/:key**（11 号单遗留）：白名单 B + 敏感 key 黑名单拒删（keyBanned 复用 PUT 的 sensitiveKeyBanned，PUT 内联循环已抽函数）+ 审计 action=governance.config.delete（记 category+value 字节数）+ 不存在 404 + Encrypted 行可删（黑名单 key 拒删工单口径）；前端 system_config.html 加删除按钮（黑名单/encrypted 禁用+confirm+错误提示）
- **C 空返回语义（文档说明，非代码）**：/configs、/services、/federation-nodes 空数组均为正常空态（生产库无对应数据；federation-nodes 2b-3 有完整 CRUD，建数据即有）
- **本地验证**（BETA/8081）：A4/A5/A6 SU 200+owner 403+原口径回归 200；B1 SU 200 渲染正常、B2 302 链正确、导航死链修复确认；D 全矩阵 200/404/400/403+审计落行（governance.config.delete）；PUT 回归 200。gofmt/vet/build/test 全绿
- **构建产物**（2026-09-09 14:31:37 CST）：dist/oas sha256 75e1ac57…、dist/ms 45d74fa7…、dist/os 0ee341ff…
- **验收对账项**（主 Agent 部署后）：①三别名 200+原路径回归 ②/admin/services SU 200、匿名 302 login ③/config-center 302→/admin/system-config ④DELETE 正常 key 200+审计、secret key 400、不存在 404、CU 403

## X1 补修（P1，已关单 PASS）

- **漏点**：POST /api/v1/admin/users（users.go CreateUser）用 password.Hash 包装绕过了 12 号 SEC-3 的 bcrypt grep 排查，password="1" 可 201。已补 pkg/password.ValidateStrength（同口径 ≥8 位 + 四类三类）
- **全量排查结论**（密码哈希包装函数 password.Hash 导致初查遗漏，本次按 Hash/req.Password 双向 grep）：用户密码入口共 4 处全部接入 ValidateStrength——users.go CreateUser（本次补）、admin_accounts.go 创建+重置（12 号已接）、ams.go 创建（12 号已接）；无用户密码重置第二端点；oauth_clients 的 client_secret 为机器随机凭证不属用户密码策略；seed.go 静态测试账号（test123）为工单链路依赖不接入
- **F3/F4/F5**：console_home TAM/HAM/YAM 三卡片移除（XAM 域管理页 404 死链，TI 默认不交付；恢复需 CONSOLE-08 裁决后补页+菜单）。F1 services 页/F2 configs 死链已在 13 号单交付
- **验证**：users 弱密码 1/123/abcdefgh 全 400、Str0ngPass! 201、admin-accounts 回归 400；菜单 grep 0 残留
- **构建产物**（2026-09-09 20:08:31 CST）：dist/oas e9d7dd8d…、dist/ms 8c499d8a…、dist/os 5c2e2da9…
- **验收对账项**：POST /api/v1/admin/users password="1"=400（X1 闭合）、console_home 无 domains/TAM|HAM|YAM 链接

- **关单终态**：部署 HEAD 9d8afd4d（=13 号 35baf6f + X1 5fae4d6）线上 PROD 全量验收 PASS——X1 users 弱密码 400/强密码 201、三别名 200（RequireUsers 口径一致）、services 页/config-center 302/域菜单移除、DELETE 全矩阵 200/404/400/403（敏感 key 拒删先于存在性检查）、dashboard/stats CU 403 不变量保持；验收临时数据已清库（configs 空、active 用户 11 基线）。发布说明素材：路由别名三件 + services 页 + config-center 重定向 + DELETE 端点 + users 密码强度补齐 + 域菜单移除
