# ziway-AMS · 知味生态系统/鉴权服务

知味生态全栈OS v4.9 — L3 MBS 层 · AMS 系统服务（认证鉴权基座）

## 概述

AMS 是知味生态的"信任根"，为所有微服务提供：

- **JWT RS256 签发/验签**：非对称签名，私钥只在AMS
- **Casbin RBAC+ABAC 鉴权**：支持戴帽子身份（一个用户在不同事业场有不同角色）
- **X\*PZ# 编号体系**：12类角色统一发号
- **Gin HTTP + gRPC 双协议**：8080给前端，50051给内部服务
- **Service Token 隔离**：服务间不用用户JWT

## 技术栈

| 组件 | 选型 |
|------|------|
| 语言 | Go 1.22+ |
| HTTP | Gin |
| RPC | gRPC + protobuf |
| 鉴权 | JWT(RS256) + Casbin |
| 数据库 | PostgreSQL 15 |
| 缓存 | Redis 7 |
| ORM | GORM |
| 日志 | Zap |
| 部署 | Docker + Docker Compose |

## 快速开始

### 1. 启动基础设施

```bash
make docker-up
```

### 2. 生成RSA密钥

```bash
make keys
```

### 3. 安装依赖并运行

```bash
make deps
make run
```

### 4. 验证

```bash
# 健康检查
curl http://localhost:8080/health

# 注册
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"phone":"13800138000","password":"123456","nickname":"测试用户"}'

# 登录
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"account":"13800138000","password":"123456","domain":"mall"}'
```

## API 接口

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| GET | /health | 无 | 健康检查 |
| GET | /metrics | 无 | Prometheus指标 |
| POST | /api/v1/auth/login | 无 | 登录 |
| POST | /api/v1/auth/register | 无 | 注册 |
| GET | /api/v1/auth/me | JWT | 当前用户信息 |
| POST | /api/v1/auth/switch-hat | JWT | 切换事业场（戴帽子） |
| POST | /api/v1/auth/logout | JWT | 登出 |
| GET | /api/v1/users/me/profile | JWT+RBAC | 我的资料 |
| GET | /api/v1/users/:id | JWT+RBAC | 查询用户 |
| GET | /api/v1/users | JWT+RBAC | 用户列表 |

## 12类角色

| 编码 | 名称 | 事业场 |
|------|------|--------|
| CU | 消费者 | mall |
| DU | 经营者 | shop |
| PU | 创研者 | lab |
| EU | 供应商 | market |
| HU | 工作者 | mate |
| OU | 治理员 | case |
| GU | 监管员 | case |
| AU | 管理员 | ams |
| FU | 财务 | fmbs |
| IU | 投资者 | ibos |
| VU | 访客 | * |
| SU | 超级管理员 | * |

## 项目结构

```
ziway-ams/
├── cmd/server/main.go       # 服务入口
├── configs/                 # 配置文件
├── internal/
│   ├── http/                # Gin HTTP层
│   ├── grpc/                # gRPC层
│   ├── service/             # 业务逻辑
│   ├── repository/          # 数据访问
│   ├── model/               # 数据模型
│   ├── authz/               # Casbin鉴权
│   ├── jwt/                 # JWT签发/验签
│   ├── idgen/               # X*PZ#发号器
│   └── pkg/                 # 公共包
├── migrations/              # 数据库迁移
├── deploy/Dockerfile
├── docker-compose.yml
└── Makefile
```

## 知味生态项目关系

AMS 是27个Coze Code项目中第一个落地的Go后端服务：

- **上游**：所有前端项目（Mall/Shop/Mate等）通过HTTP调用AMS登录
- **下游**：所有MBS/BOS服务通过gRPC向AMS验证Token
- **后续**：CMBS→DMBS→FMBS→COS→DOS 按MVP优先级逐个Go化
