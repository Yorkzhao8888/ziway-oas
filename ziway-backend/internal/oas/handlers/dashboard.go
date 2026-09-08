// dashboard.go — 自 cmd/oas/main.go 下沉（OAS-CONSOLE-09 A1），行为零变化。
package handlers

import (
	"github.com/gin-gonic/gin"

	oasmodel "ziway/backend/internal/oas/model"
	"ziway/backend/pkg/model"
	"ziway/backend/pkg/response"
)

func (h *Handlers) DashboardStats(c *gin.Context) {
	username, _ := c.Get("username")

	// 用户统计（仅 active）
	var usersTotal int64
	h.DB.Model(&oasmodel.OASUser{}).Where("status = ?", "active").Count(&usersTotal)

	// 组织统计
	var orgsTotal int64
	h.DB.Model(&model.Organization{}).Count(&orgsTotal)

	// 角色统计
	var rolesTotal int64
	h.DB.Model(&oasmodel.OASRole{}).Count(&rolesTotal)

	// 域分布（基于 organizations）
	type DomainCount struct {
		Domain string `json:"domain"`
		Count  int64  `json:"count"`
	}
	// 初始化全域 0 计数
	domainDistribution := []DomainCount{
		{Domain: "T", Count: 0},
		{Domain: "H", Count: 0},
		{Domain: "Y", Count: 0},
		{Domain: "V", Count: 0},
		{Domain: "O", Count: 0},
		{Domain: "A", Count: 0},
		{Domain: "F", Count: 0},
		{Domain: "G", Count: 0},
	}
	// 查询实际分布并更新
	var actualDistribution []DomainCount
	h.DB.Model(&model.Organization{}).
		Select("domain, COUNT(*) as count").
		Group("domain").
		Scan(&actualDistribution)
	// 更新有数据的域
	for _, actual := range actualDistribution {
		for i := range domainDistribution {
			if domainDistribution[i].Domain == actual.Domain {
				domainDistribution[i].Count = actual.Count
				break
			}
		}
	}

	// 审计摘要（仅 2admin 可见明细，OAM 只返回统计）
	isOUAU := username == "oas-ou-admin" || username == "oas-au-admin"

	result := gin.H{
		"users_total":         usersTotal,
		"orgs_total":          orgsTotal,
		"roles_total":         rolesTotal,
		"domain_distribution": domainDistribution,
	}

	if isOUAU {
		// 2admin 可见审计明细
		var recentAudits []oasmodel.AuditLog
		h.DB.Order("created_at DESC").Limit(10).Find(&recentAudits)

		type ActionCount struct {
			Action string `json:"action"`
			Count  int64  `json:"count"`
		}
		var auditSummary []ActionCount
		h.DB.Model(&oasmodel.AuditLog{}).
			Select("action, COUNT(*) as count").
			Group("action").
			Scan(&auditSummary)

		result["recent_audits"] = recentAudits
		result["audit_summary"] = auditSummary
	}

	response.OK(c, result)
}
