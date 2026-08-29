/**
 * ziway-Mall API 使用示例
 *
 * Mall 对接 CBOS → CMBS(客户) + DMBS(经营)  [ZW-ARC-017 更新]
 */

import { cmbsApi, dmbsMallApi, ambsApi, login } from '../shared/api-client'

async function main() {
  // 1. 登录
  await login('york', 'password123')

  // 2. 客户管理（CMBS）
  const customers = await cmbsApi.get('/customers', {
    params: { page: 1, size: 20 },
  })
  console.log('客户列表:', customers.data)

  // 3. 订单管理（CMBS）
  const orders = await cmbsApi.get('/orders', {
    params: { status: 'active' },
  })
  console.log('活跃订单:', orders.data)

  // 4. 经营数据（DMBS，CBOS 跨域代理）[ZW-ARC-017 新增]
  const stores = await dmbsMallApi.get('/stores', {
    params: { page: 1 },
  })
  console.log('门店列表:', stores.data)

  // 5. 客户汇总（CBOS 聚合视图）
  const summary = await cmbsApi.get('/customers/summary')
  console.log('客户概览:', summary.data)
}
