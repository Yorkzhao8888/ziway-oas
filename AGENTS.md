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
