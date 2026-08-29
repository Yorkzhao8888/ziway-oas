# 知味生态全栈OS v4.9 - 后端微服务

> **ZW-ARC-017 权威版** · 2026-08-25
> 12 BOS · 12 MBS · 8 APP · 1 OAS = **33 项目**

## 架构总览

```
┌──────────────────────────────────────────────────────────────┐
│  APP层（交互面）  8 APP + 1 未来                              │
│  Mall · Shop · Lab · Mate · Market · Xcase · Dyard · Agent   │
│  （孵化园APP 未来开发）                                        │
├──────────────────────────────────────────────────────────────┤
│  BOS层（编排层）  12 BOS                                     │
│  编排(5): COS · DOS · IBOS · VBOS · TOS                  │
│  直通(7): ABOS · EBOS · HBOS · SBOS · FBOS · GBOS · OBOS   │
├──────────────────────────────────────────────────────────────┤
│  MBS层（原子能力基座）  12 MBS                                │
│  AMS · CMBS · DMBS · HMBS · FMBS · TMS · EMBS             │
│  GMBS · OMBS · VMBS · IMBS · SMBS                           │
├──────────────────────────────────────────────────────────────┤
│  OAS层（治理根基）  1 项目（Owner+Admin 逻辑平面合并）         │
│  ziway-OAS                                                   │
└──────────────────────────────────────────────────────────────┘
```

## P0 三进程架构

| 进程 | 端口 | 职责 |
|------|------|------|
| OAS  | 8080 | Owner+Admin 治理平面（JWT签发、策略同步、域注册） |
| MBS  | 8081 | 12 个 MBS 模块化单体（各独立 schema） |
| BOS  | 8082 | 12 个 BOS 编排器（HTTP 反向代理到 MBS） |

> P0 阶段 BOS 和 MBS 各是单进程多模块；P1 按需拆分为独立进程。

## BOS 编排清单（12个）

| BOS | 模式 | 编排 MBS | 消费 APP |
|-----|------|----------|----------|
| COS | 编排 | cmbs + dmbs | ziway-Mall |
| DOS | 编排 | dmbs + hmbs + fmbs | ziway-Shop |
| IBOS | 编排 | imbs + fmbs | Xcase（投资Tab） |
| VBOS | 编排 | gmbs + ombs + vmbs | Xcase（运营Tab） |
| TOS | 编排 | pmbs | ziway-Lab |
| ABOS | 直通 | ams | —（系统鉴权基础） |
| EBOS | 直通 | embs | ziway-Market |
| HBOS | 直通 | hmbs | ziway-Mate |
| SBOS | 直通 | smbs | 孵化园APP（未来） |
| FBOS | 直通 | fmbs | —（被 DOS/IBOS 消费） |
| GBOS | 直通 | gmbs | —（被 VBOS 消费） |
| OBOS | 直通 | ombs | Xcase（治理Tab） |

> Dyard / Agent 直连 OSA（:8080），不走 BOS 代理。

## MBS 模块清单（12个）

| MBS | Schema | 职责 |
|-----|--------|------|
| AMS | ams | 认证鉴权、JWT、RBAC、12U 角色 |
| CMBS | cmbs | 客户管理、订单、消息通知 |
| DMBS | dmbs | 门店经营、KPI、GP（T56 编号） |
| HMBS | hmbs | 组织、员工、考勤、请假 |
| FMBS | fmbs | 总账、收支、发票、GP 分润 |
| TMS | pmbs | 产品、分类、NPI 管线 |
| EMBS | embs | 供应商、采购、DEX 交付、四种铺面 |
| GMBS | gmbs | 风控策略、合规检查、审计 |
| OMBS | ombs | 审批流、治理策略、工作流 |
| VMBS | vmbs | VCASE 权威源、预算、运营概览 |
| IMBS | imbs | 资本账户、ICASE（T43 三方会签） |
| SMBS | smbs | 孵化项目（T48 旋转门）、SU 终端 |

## 快速开始

### SQLite 开发模式（零依赖）

```bash
# 1. 下载依赖
make deps

# 2. 编译全部
make build

# 3. 启动三进程
make run-all

# 4. 验证
curl localhost:8080/health   # OAS
curl localhost:8081/health   # MBS（12 个模块）
curl localhost:8082/health   # BOS（12 个编排器）
```

### 完整开发环境（Docker）

```bash
docker-compose up -d    # PG + Redis + Kafka
APP_ENV=prod make build
./bin/oas & ./bin/ms & ./bin/os
```

## 项目结构

```
ziway-backend/
├── cmd/                          # P0 三进程入口
│   ├── oas/main.go               # :8080 治理底座
│   ├── ms/main.go               # :8081 12 MBS 单体
│   └── os/main.go               # :8082 12 BOS 编排器
├── internal/
│   ├── ms/                      # 12 个 MBS 模块（P0 单体包）
│   │   ├── ams/ cmbs/ dmbs/ hmbs/ fmbs/ pmbs/
│   │   └── embs/ gmbs/ ombs/ vmbs/ imbs/ smbs/
│   └── os/                      # 12 个 BOS 编排器
│       ├── os.go                # Orchestrator 接口
│       ├── proxy.go              # HTTP 反向代理
│       ├── cbos/ dbos/ ibos/ vbos/ pbos/    # 编排模式(5)
│       └── abos/ ebos/ hbos/ sbos/ fbos/ gbos/ obos/  # 直通模式(7)
├── pkg/                          # 共享基础包
│   ├── config/ db/ jwt/ kafka/ logger/
│   ├── middleware/ response/ idgen/ model/ server/
│   └── eventbus/
├── services/
│   └── ams/                     # AMS P1 独立服务（完整 JWT+Casbin+12U，独立Go模块）
│       ├── cmd/server/           # 独立入口
│       ├── internal/             # authz/jwt/repository/service
│       ├── migrations/           # SQL migrations
│       └── configs/              # 含 RSA 密钥 + rbac_model.conf
├── frontend-config/              # 8 APP 前端 .env + 共享客户端
├── API_MAPPING.md                # APP→BOS→MBS 路由映射
├── configs/                      # dev-sqlite.yaml / config.yaml
├── docker-compose.yml            # PG+Redis+Kafka
├── setup.sh                      # Mac 一键部署脚本
└── Makefile                      # 构建/运行/测试
```

## 技术栈

- **语言**: Go 1.22+ · 模块名 `ziway/backend`
- **HTTP**: Gin · 反向代理 BOS→MBS
- **数据库**: PostgreSQL(生产) / SQLite(开发) · GORM
- **缓存**: Redis（P1）
- **消息**: Kafka（P1）/ EventBus(P0 内存)
- **日志**: Zap · **配置**: Viper

## 12U 角色系统

| 角色 | 代码 | 说明 |
|------|------|------|
| 消费者 | CU | C端用户 |
| 事业者 | DU | 事业场主 |
| 创作者 | PU | 内容/知识产出 |
| 员工 | EU | 组织内工作者 |
| 人力 | HU | 人力管理 |
| 运营 | OU | 运营管理 |
| 治理 | GU | 治理决策 |
| 管理 | AU | 系统管理 |
| 财务 | FU | 财务管理 |
| 集成 | IU | 系统集成 |
| 虚拟 | VU | Agent管理 |
| 安全 | SU | 安全/孵化 |
| 帽子 | CX/FX | 体验帽子 |
| NHI | — | 非人类身份 |

## 权威文档

- ZW-ARC-017：BOS 扩展至 12 个（LOCKED）
- ZW-ARC-016：四层架构结构性修正
- ZW-ARC-015：架构修正裁定
- 详见 API_MAPPING.md 了解路由详情
