# 知味生态前端对接配置

> **ZW-ARC-017 权威版** · 2026-08-25
> 12 BOS · 12 MBS · 8 APP · 1 OAS = 33 项目

## 目录结构

```
frontend-config/
├── mall/          # ziway-Mall  → COS(cmbs+dmbs)
├── shop/          # ziway-Shop  → DOS(dmbs+hmbs+fmbs)
├── lab/           # ziway-Lab   → TOS(pmbs)
├── mate/          # ziway-Mate  → HBOS(hmbs)
├── market/        # ziway-Market→ EBOS(embs)
├── xcase/         # ziway-Xcase → OBOS+VBOS+IBOS+ABOS
├── dyard/         # ziway-Dyard → OSA 直连（不走 BOS）
├── agent/         # ziway-Agent → OSA 直连（不走 BOS）
├── shared/        # 统一 axios 客户端
│   └── api-client.ts
├── examples/      # 各 APP 调用示例
│   ├── mall-api-example.ts
│   ├── shop-api-example.ts
│   └── xcase-api-example.ts
└── README.md      # 本文件
```

## 每个 APP 目录

- `.env.development` — 开发环境变量（BOS 代理基址）
- `.env.production` — 生产环境变量

## 核心文件

### shared/api-client.ts
统一 axios 客户端，每个 MBS 代理基址创建独立实例，自动注入 token + X-User-ID，统一错误处理。

## ZW-ARC-017 关键变更

1. **BOS 8→12**：新增 SBOS(创业孵化)/GBOS(风控)/FBOS(财务)/OBOS(治理)
2. **COS 扩展**：cmbs → cmbs+dmbs（Mall 新增经营数据）
3. **DOS 扩展**：dmbs+fmbs → dmbs+hmbs+fmbs（Shop 新增人力模块）
4. **Xcase 新增治理 Tab**：通过 OBOS 代理 OMBS
5. **项目总数 29→33**

## 使用方式

```bash
# 1. 复制 .env.development 到你的 APP 项目
cp mall/.env.development your-app/.env.development

# 2. 安装 axios
npm i axios

# 3. 引入统一客户端
import { cmbsApi, login } from '@/utils/api-client'

# 4. 登录 + 调用
await login('user', 'pass')
const res = await cmbsApi.get('/customers')
```

详见 `examples/` 目录。
