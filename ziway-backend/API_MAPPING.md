# P0 API 对接指南（APP → BOS → MBS）

> **ZW-ARC-017 权威版**。APP**只对接 BOS（:8082）**，不直连 MBS。
> BOS 在 P0 用 HTTP 反向代理透传到 MBS；P1 换成 gRPC，**APP 代码无需改动**。
> 12 BOS（5 编排 + 7 直通）· 12 MBS · 8 APP · 1 OAS = 33 项目

## 一、三个进程

| 进程 | 端口 | 谁来调 |
|---|---|---|
| OAS  | 8080 | 管理后台 / 治理（Owner/Admin Plane） |
| MBS  | 8081 | **内部**，BOS 代理调用，APP 不直连 |
| BOS  | 8082 | **所有 APP 唯一入口** |

## 二、BOS 完整清单（12个）

| # | BOS | 模式 | 编排 MBS | 消费 APP |
|---|---|---|---|---|
| 1 | COS | 编排 | cmbs + dmbs | ziway-Mall |
| 2 | DOS | 编排 | dmbs + hmbs + fmbs | ziway-Shop |
| 3 | IBOS | 编排 | imbs + fmbs | ziway-Xcase（投资Tab） |
| 4 | VBOS | 编排 | gmbs + ombs + vmbs | ziway-Xcase（运营Tab） |
| 5 | TOS | 编排 | pmbs | ziway-Lab |
| 6 | ABOS | 直通 | ams | —（系统鉴权基础） |
| 7 | EBOS | 直通 | embs | ziway-Market |
| 8 | HBOS | 直通 | hmbs | ziway-Mate |
| 9 | SBOS | 直通 | smbs | 孵化园APP（未来开发） |
| 10 | FBOS | 直通 | fmbs | —（被DOS/IBOS编排消费） |
| 11 | GBOS | 直通 | gmbs | —（被VBOS编排消费） |
| 12 | OBOS | 直通 | ombs | ziway-Xcase（治理Tab） |

> Dyard / Agent 直连 OSA 底座（:8080），不走 BOS 代理。

## 三、APP → BOS 基址映射（8 APP + 1 未来）

每个 APP 在 BOS 里有专属编排器，下面挂它要用的 MBS 代理。

| APP | BOS | 编排MBS | APP 内 API 基址（拼到 axios baseURL） |
|---|---|---|---|
| ziway-Mall   | cbos | cmbs, dmbs | `:8082/api/v1/os/cbos/proxy/cmbs`（客户）<br>`:8082/api/v1/os/cbos/proxy/dmbs`（经营） |
| ziway-Shop   | dbos | dmbs, hmbs, fmbs | `:8082/api/v1/os/dbos/proxy/dmbs`（经营）<br>`:8082/api/v1/os/dbos/proxy/hmbs`（人力）<br>`:8082/api/v1/os/dbos/proxy/fmbs`（财务） |
| ziway-Lab    | pbos | pmbs | `:8082/api/v1/os/pbos/proxy/pmbs` |
| ziway-Mate   | hbos | hmbs | `:8082/api/v1/os/hbos/proxy/hmbs` |
| ziway-Market | ebos | embs | `:8082/api/v1/os/ebos/proxy/embs` |
| ziway-Xcase  | obos | ombs | `:8082/api/v1/os/obos/proxy/ombs`（治理Tab） |
| ziway-Xcase  | vbos | gmbs, ombs, vmbs | `:8082/api/v1/os/vbos/proxy/gmbs`（风控）<br>`:8082/api/v1/os/vbos/proxy/ombs`（审批）<br>`:8082/api/v1/os/vbos/proxy/vmbs`（VCASE） |
| ziway-Xcase  | ibos | imbs, fmbs | `:8082/api/v1/os/ibos/proxy/imbs`（投资）<br>`:8082/api/v1/os/ibos/proxy/fmbs`（投资级财务） |
| ziway-Xcase  | abos | ams | `:8082/api/v1/os/abos/proxy/ams`（登录/用户/运维） |
| ziway-Dyard  | — | — | 直连 OSA :8080 |
| ziway-Agent  | — | — | 直连 OSA :8080 |
| 孵化园APP    | sbos | smbs | `:8082/api/v1/os/sbos/proxy/smbs`（未来） |

> 说明：ziway-Xcase 是复合APP，内部按Tab分别对接 obos/vbos/ibos/abos 四个BOS。

## 四、URL 改写规则（BOS自动完成，APP无感）

```
APP 调：  :8082/api/v1/os/cbos/proxy/cmbs/customers?page=1
                         └────── BOS ──────┘
BOS 转发：:8081/api/v1/cmbs/customers?page=1
                         └ MBS ┘
```
即把 `/api/v1/os/{os}/proxy/{ms}` 段替换成 `/api/v1/{ms}`，query/body/方法/header 原样透传。

## 五、主要 MBS 路由清单

### ams（鉴权/用户）
- `POST /auth/login` · `POST /auth/register` · `GET /auth/me`
- `GET/POST/PUT /users` · `GET/POST /roles`

### cmbs（客户/订单）
- 客户：`POST/GET /customers` · `GET/PUT /customers/:id`
- 订单：`POST/GET /orders` · `GET /orders/:id` · `PUT /orders/:id/status`
- 消息：`POST/GET /messages`
- 通知：`POST/GET /notifications`

### dmbs（经营/门店/KPI/GP）
- 门店：`POST/GET /stores` · `GET/PUT /stores/:id` · `GET /stores/:id/gp`
- KPI：`POST/GET /kpis`
- 门店编号强制 T56 前缀

### hmbs（人力）
- 组织：`POST/GET /organizations`
- 员工：`POST/GET /employees` · `GET/PUT /employees/:id`
- 考勤：`POST/GET /attendance`
- 请假：`POST/GET /leaves` · `PUT /leaves/:id/approve`

### fmbs（财务/GP分润）
- 总账：`POST/GET /ledger`
- 收支：`POST/GET /payments`
- 发票：`POST/GET /invoices`
- FCASE：`POST/GET /fcases` · `PUT /fcases/:id/approve`
- GP分润：`POST /gp/calculate`（DU50%/HU20%/OU15%/IU15%）

### pmbs（产品/NPI）
- 产品：`POST/GET /products` · `GET/PUT /products/:id` · `DELETE /products/:id`
- 分类：`POST/GET /categories`
- NPI：`POST/GET /npi` · `PUT /npi/:id/stage`

### embs（供给中枢 + 集市四种铺面）
- 供应商：`POST/GET /suppliers` · `GET/PUT /suppliers/:id` · `POST /suppliers/:id/approve`
- EX货池：`POST/GET /supply-items` · `GET/PUT /supply-items/:id`
- 采购单：`POST/GET /purchase-orders` · `GET /purchase-orders/:id` · `PUT /purchase-orders/:id/status`
- 合同：`POST/GET /contracts`
- DEX交付：`POST/GET /fulfillments` · `PUT /fulfillments/:id/status` · `GET /fulfillments/track/:tracking_no`
- 铺面：`POST/GET /stores` · `GET/PUT /stores/:id` · `GET /stores/:id/stats`
- 铺面商品/订单/生产/仓配/物流等（见EMBS handlers.go）

### gmbs（风控/审计）
- `GET/POST /risks` · `PUT /risks/:id/resolve`
- `GET/POST /policies` · `GET/POST /audits` · `GET /dashboard`

### ombs（审批/治理）
- `GET/POST /approvals` · `POST /approvals/:id/approve|reject`
- `GET/POST /rules` · `GET/POST /workflows`

### vmbs（价值运营/VCASE，VCASE权威源）
- `GET/POST /vcases` · `GET/PUT /vcases/:id`
- `POST /vcases/:id/submit` · `POST /vcases/:id/execution`
- `POST /vcases/:id/complete` · `POST /vcases/:id/fail`
- `GET/POST /budgets` · `GET /ops/overview`

### imbs（资本/ICASE，T43三方会签）
- `GET/POST /accounts`（IX资本账户）
- `GET/POST /icases` · `POST /icases/:id/submit`
- `POST /icases/:id/approve`（party=oas/smbs/imbs 三方依次签）
- `GET /portfolio`

### smbs（孵化，T48旋转门）
- `GET/POST /projects`（SX孵化项目）
- `POST /projects/:id/stage`（seed→sprout→growth→graduate）
- `POST /projects/:id/graduate`（毕业转DMBS，签发T56编号）
- `GET/POST /terminals`（SU种子终端）

## 六、健康检查（联调用）
```bash
curl localhost:8080/health   # OAS
curl localhost:8081/health   # MBS（看12个模块名）
curl localhost:8082/health   # BOS（看12个编排器名）
# 单模块自检
curl localhost:8081/api/v1/cmbs/health
curl localhost:8082/api/v1/os/cbos/proxy/cmbs/health  # 走代理
```

## 七、FMBS 多重消费说明

FMBS 是唯一同时有直通 BOS（FBOS）和被编排 BOS（DOS/IBOS）消费的 MBS：
- **FBOS**：财务直通代理，提供财务汇总/账本视图
- **DOS**：经营场编排，日结时 DMBS→HMBS→FMBS 跨域 Saga
- **IBOS**：投资场编排，资本部署时 IMBS→FMBS 跨域 Saga

APP 根据场景选择走哪个 BOS 入口：
- 纯财务操作 → FBOS
- 门店经营+财务 → DOS
- 投资+财务 → IBOS

## 八、前端改造步骤
1. 把原来指向各独立端口（8082-8301）的 baseURL，统一改成对应 BOS 代理基址（见第三节表）。
2. 请求路径去掉旧的服务名前缀差异，直接用 MBS 路由（见第五节，如 `/customers`、`/orders`）。
3. 登录态：`POST {abos基址}/auth/login` 拿 token，后续请求带 `Authorization: Bearer <token>` 和 `X-User-ID`。
4. 多MBS的BOS（Mall/Shop/Xcase）按业务模块切换不同代理基址。
5. P1 上线 gRPC 后，上述URL全部不变，BOS内部替换实现。

## 九、本地启动
```bash
make build
make run-all   # 后台拉起三进程，日志在 tmp/*.log
# 或分三个终端：
make run-oas   # 8080
make run-ms   # 8081
make run-os   # 8082
```
