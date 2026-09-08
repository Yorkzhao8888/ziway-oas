# OAS-CONSOLE-08 需求单：管理员身份收窄（减负）

- 日期：2026-09-08
- 发往窗口：OAS/oas-backend（7679323618953855019）
- 优先级：P0（权限模型调整）
- 状态：已发送，待设计回报
- 关联：OAS-CONSOLE-02（三层视图/白名单 A/B 定版）；OAS-CONSOLE-07（配置看板/OAuth 客户端管理）

## 一、背景
用户拍板 OAS 减负：当前管理员身份 7 类（SU/AU/OAM + TAM/HAM/YAM/VAM 四域管理员）过多，且 XAM 域管理身份无真实使用方（33 项目尚未接入）。收窄为 3 类，冻结域管理身份，降低权限模型复杂度。

## 二、收窄方案
### 身份收窄（7 → 3）
| 身份 | 现状 | 收窄后 |
|---|---|---|
| SU（OU-admin） | 2admin 全量 | **保留**（白名单 A+B 全量） |
| AU（AU-admin） | 2admin 全量 | **保留**（白名单 A+B 全量） |
| OAM | 白名单 A 统计只读 | **保留**（仅统计/看板只读，管理操作 403 不回退） |
| TAM/HAM/YAM/VAM（XAM 域管理） | 域隔离管理视图 | **冻结**：账号停用（disabled 不删除）、移除域管理入口、角色 seed 停止 |
| VAM 域间协调 | 同上 | **冻结**（并入 XAM 冻结范围） |

### 界面收窄
- /admin 管理界面不再提供域管理视图（XAM 视图入口移除）；侧边栏仅呈现 2admin + OAM 视角
- 保留 2admin 全量管理页 + OAM 只读看板（overview/ownership/stats）

### 权限模型（不回退）
- 白名单 A：SU / AU / OAM（OAM 仅 dashboard 统计只读，其余 6 类管理端点 403）
- 白名单 B：SU / AU（admin 账号操作、审计、api-keys、oauth-clients、approvals、system-config、federation-nodes）
- 匿名 401、未认证 302

### 数据层
- 域隔离字段/表保留（不删表不删列），域管理功能冻结，未来接入时按需恢复
- 审计域过滤保留（审计能力不缩水）

### 清理
- XAM（TAM/HAM/YAM/VAM）账号 status=disabled（不物理删除，保留数据层）
- 域角色 seed 逻辑停止（或注释，不创建新 XAM 账号）

## 二·补、帽子语义对齐（09-08 20:30 用户确认追加）
### 背景
生态 13U 权威口径：OU=生态董事长（最高领导人）、AU=系统总运营长、SU=Seedup 系统方。OAS 治理帽用 SU/AU/OAM 编码与 13U 撞名，造成语义混淆（SU 易被误认为创始人；OU 本人无入口）。

### 对齐方案
1. **治理帽显示名对齐生态语言**（界面/审计显示名，代码内部编码可保留，不增加重构成本）：
   - SU（OU-admin）→ 显示名「系统总管理者」
   - AU（AU-admin）→ 显示名「系统运营管理者」
   - OAM → 显示名「治理审计员」
2. **新增 OU 本人入口（L0 治理看板）**：创始人/最高领导人登录可见治理看板（所有权/战略审批/治理概览），只读 + 审批，不碰日常运维；seed 账号 `oas-ou-owner`（role=OU）。
   - OU（本人）与 SU（OU-admin 系统总管理者）区分：OU=治理者本人，SU=其授权的执行者。
3. **13U 业务帽不进治理权**：EU/DU/YU 等 13U 账号只借身份登录（OIDC 跳转业务系统），OAS Console 无治理入口（白名单外 403）。
4. **13U 授权边界（非本次开发）**：OAS 授权 13U 的机制已具备（OIDC+JWKS+JWT roles/domain/org claims），各系统接入时消费授权；本次不做 13U 授权新功能。

### 验收补充（并入验收标准）
- OU 本人账号（oas-ou-owner）可登录见 L0 治理看板（只读），不显示系统管理菜单
- 管理界面/审计显示名为中文（系统总管理者/系统运营管理者/治理审计员）
- EU/DU/YU 业务账号登录 OAS 后 Console 无治理入口（403/无菜单），可正常跳转业务系统

## 三、验收标准
1. SU/AU 登录后管理全量可用；OAM 仅 overview/stats/ownership 200，audit-logs/api-keys/oauth-clients/approvals/system-config/federation-nodes/admin-accounts 403
2. TAM/HAM/YAM/VAM 账号不可登录（disabled 401）或登录后无管理入口（403/无菜单）
3. /admin 界面无域管理视图入口；侧边栏收窄
4. 回归不回退：登录/OIDC 授权码链路/JWKS/审计记录/白名单 A/B/匿名 401/未登录 302
5. 帽子语义对齐（见"二·补"验收补充）
6. 测试数据清理

## 四、开发要求
- 重建 dist/oas 入仓，回报 commit + 构建时间 + sha256
- 分批回报-部署-验收
- 先回报当前管理员身份/角色/白名单实现清单，确认后实施
