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
ziway-backend/
├── cmd/
│   ├── oas/        # OAS 进程入口
│   ├── ms/         # MBS 进程入口（12 模块）
│   └── os/         # BOS 进程入口（12 编排器）
├── internal/
│   ├── mbs/        # MBS 模块实现（ams/cms/dms/hms/fms/tms/ems/gms/oms/vms/ims/sms）
│   └── bos/        # BOS 编排器实现（cos/dos/ibos/vbos/tos/abos/ebos/hbos/sbos/fbos/gbos/obos）
├── pkg/            # 共享库（config/db/server/jwt/kafka/middleware/logger/response/eventbus/idgen/model）
├── services/ams/   # P1 AMS 独立服务（可单独部署）
├── configs/        # 配置文件（config.yaml=prod, dev-sqlite.yaml=dev）
├── scripts/        # 部署脚本（build.sh/run.sh）
├── Makefile        # 构建与运行命令
└── setup.sh        # 一键部署脚本
```

## 关键入口 / 核心模块

- `cmd/ms/main.go`：MBS 主入口，注册 12 个模块，AutoMigrate，启动 HTTP
- `cmd/oas/main.go`：OAS 主入口，治理平面
- `cmd/os/main.go`：BOS 主入口，反向代理到 MBS
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
- SQLite 开发模式下数据存储在 `data/ziway_p0.db`，启动前需确保 `data/` 目录存在
- `services/ams/` 是 P1 独立服务，有独立的 `go.mod`，需单独构建
- `internal/` 目录名为 `mbs/` 和 `bos/`（非 `ms/` 和 `os/`），与 import 路径和 package 声明一致
- BOS（cmd/os）启动强校验安全三件套：`configs/public_key.pem` + `configs/rbac_model.conf` + `configs/rbac_policy.csv`，任一缺失 → fail-closed 拒绝启动
- JWT 中间件 Redis 黑名单检查为可选（rdb=nil 时跳过），不影响 JWT 验签本身
- `/health` 端点公开，`/api/v1/*` 全部需要 JWT + RBAC
- OAuth 表名遵循 GORM 驼峰转下划线规则：`OAuthClient` → `o_auth_clients`，`OAuthAuthorizationCode` → `o_auth_authorization_codes`
- JWT context 键为 `user_id`（非 `user_code`），提取时需类型断言：`userID, ok := userIDVal.(string)`
- 开发环境启动需清除平台注入的 `ZIWAY_DATABASE_DRIVER` 和 `ZIWAY_DATABASE_DSN`，否则会使用 PostgreSQL 而非 SQLite

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
