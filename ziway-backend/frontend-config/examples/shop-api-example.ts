/**
 * ziway-Shop API 使用示例
 *
 * Shop 对接 DBOS → DMBS(经营) + HMBS(人力) + FMBS(财务)  [ZW-ARC-017 更新]
 */

import { dmbsShopApi, hmbsShopApi, fmbsShopApi, ambsApi, login } from '../shared/api-client'

async function main() {
  // 1. 登录
  await login('york', 'password123')

  // 2. 门店经营（DMBS）
  const stores = await dmbsShopApi.get('/stores', {
    params: { page: 1 },
  })
  console.log('门店列表:', stores.data)

  // 3. 日结 KPI（DMBS）
  const kpis = await dmbsShopApi.get('/kpis', {
    params: { date: '2026-08-25' },
  })
  console.log('今日KPI:', kpis.data)

  // 4. 员工考勤（HMBS）[ZW-ARC-017 新增]
  const attendance = await hmbsShopApi.get('/attendance', {
    params: { date: '2026-08-25' },
  })
  console.log('今日考勤:', attendance.data)

  // 5. 财务记账（FMBS）
  const ledger = await fmbsShopApi.get('/ledger', {
    params: { period: '2026-08' },
  })
  console.log('月度账本:', ledger.data)

  // 6. 日结编排（DMBS→HMBS→FMBS Saga）
  await dmbsShopApi.post('/daily/close', {
    date: '2026-08-25',
    store_id: 'T56-001',
  })
  console.log('日结已发起')
}
