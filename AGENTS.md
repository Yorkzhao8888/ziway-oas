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

注意：`dist/` 已出库（OAS-CONSOLE-09 A4，git 不跟踪），build.sh 无 dist 时走源码编译。

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
- dist/ 二进制已出库（A4）：git 不跟踪；`scripts/build.sh` 优先用本地 dist/，缺失时源码编译（部署平台 runtime golang-1.25 兼容）
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
- **A4**：dist/ 出库（git rm --cached + .gitignore /dist/），build.sh 源码编译分支验证通过
- **回滚锚点**：`oas-09-mid-verified` tag（锚定 A2 自测验证点 b378cdc）；更早锚点 f7d48aff59
- **包依赖单向**：cmd/oas → internal/oas/routes → internal/oas/handlers → internal/oas/{model,authz} → pkg；RegeneratePolicyCSV 经 Handlers 函数字段注入（实现在 package oas）
- **A0 基线不变量（拆分时保留、禁止顺手修正）**：GET /api/v1/admin/roles 对 SU 返回 403；/api/v1/admin/stats 404；页面级与 API 级鉴权不一致；console-home 302 → /admin/overview?token=<JWT>；approvalsPageHTML 内部硬编码用户名自算 isOUAU/canWrite；登录 API 字段 access_token
- **OAS 测试账号**：oas-ou-admin/test123（SU），seed 于 BETA/DEV 模式

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
