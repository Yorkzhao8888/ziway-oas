// Package oas 承载 OAS 进程级装配（路由/中间件/seed/state，OAS-CONSOLE-09 A1）。
//
// 共享可变状态盘点结论（A0 shared-state-inventory.txt）：God file 唯一包级可变
// 状态为 adminUsernames 静态查找表。因包依赖方向固定为 oas → handlers →
// model + authz（禁止反向导入），该状态落位 internal/oas/authz 包内
// （authz.AdminUsernames），本文件保留作为包级状态的唯一声明位与归属记录，
// 后续新增包级状态必须集中于此，禁止散落各文件。
package oas
