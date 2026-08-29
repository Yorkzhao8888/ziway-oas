/**
 * ziway-Xcase API 使用示例
 *
 * 复合APP：4 个 BOS × 4 个 Tab
 *   - 投资Tab → ibos → imbs + fmbs
 *   - 运营Tab → vbos → gmbs + ombs + vmbs
 *   - 治理Tab → obos → ombs          [ZW-ARC-017 新增]
 *   - 运维Tab → abos → ambs
 */

import {
  login,
  imbsApi,
  fmbsIbosApi,
  vmbsApi,
  ombsVbosApi,
  gmbsVbosApi,
  ombsObosApi,
  ambsApi,
} from '../shared/api-client'

// ============================================================
// 0. 登录（所有 Tab 共享 auth）
// ============================================================
async function doLogin() {
  const { token, user_id } = await login('york', 'password123')
  console.log('登录成功:', token)
}

// ============================================================
// 1. 投资 Tab (ibos)
// ============================================================
async function investmentTab() {
  // 投资组合概览（IMBS）
  const portfolio = await imbsApi.get('/portfolio')
  console.log('投资组合:', portfolio.data)

  // 待审批 ICASE（IMBS，T43 三方会签）
  const pending = await imbsApi.get('/icases', {
    params: { status: 'pending' },
  })
  console.log('待签ICASE:', pending.data)

  // 资本部署（IMBS→FMBS Saga）
  await imbsApi.post('/icases/IC-001/deploy', {
    amount: 500000,
    target: 'startup-x',
  })

  // 投资级财务报表（走 ibos 下的 fmbs 代理）
  const finance = await fmbsIbosApi.get('/ledger', {
    params: { scope: 'investment' },
  })
  console.log('投资财务:', finance.data)
}

// ============================================================
// 2. 运营 Tab (vbos)
// ============================================================
async function operationsTab() {
  // VCASE 管线（VMBS 权威源）
  const pipeline = await vmbsApi.get('/vcases', {
    params: { status: 'in_flight' },
  })
  console.log('运营中的 VCASE:', pipeline.data)

  // 风控概览（GMBS）
  const risks = await gmbsVbosApi.get('/risks')
  console.log('活跃风控:', risks.data)

  // 审批队列（OMBS，72h 超时）
  const approvals = await ombsVbosApi.get('/approvals', {
    params: { status: 'pending' },
  })
  console.log('待审批:', approvals.data)

  // 治理仪表盘（VBOS 跨域聚合）
  const dashboard = await vmbsApi.get('/dashboard')
  console.log('治理仪表盘:', dashboard.data)
}

// ============================================================
// 3. 治理 Tab (obos) [ZW-ARC-017 新增]
// ============================================================
async function governanceTab() {
  // 治理策略列表（OMBS）
  const policies = await ombsObosApi.get('/policies')
  console.log('治理策略:', policies.data)

  // 审批规则（OMBS）
  const rules = await ombsObosApi.get('/rules')
  console.log('审批规则:', rules.data)

  // 审批超时检查
  const timeouts = await ombsObosApi.get('/approvals', {
    params: { overdue: true },
  })
  console.log('超时审批:', timeouts.data)
}

// ============================================================
// 4. 运维 Tab (abos)
// ============================================================
async function opsTab() {
  // 用户汇总（AMBS）
  const users = await ambsApi.get('/users/summary')
  console.log('用户汇总:', users.data)

  // 系统状态
  const status = await ambsApi.get('/system/status')
  console.log('系统状态:', status.data)
}
