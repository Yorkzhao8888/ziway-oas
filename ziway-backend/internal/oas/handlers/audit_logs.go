// audit_logs.go — 自 cmd/oas/main.go 下沉（OAS-CONSOLE-09 A1），行为零变化。
package handlers

import (
	"github.com/gin-gonic/gin"

	"ziway/backend/internal/oas/authz"
	oasmodel "ziway/backend/internal/oas/model"
	"ziway/backend/pkg/response"
)

func (h *Handlers) ListAuditLogs(c *gin.Context) {
	username, _ := c.Get("username")
	// OAS-CONSOLE-08: roles variable removed (no longer needed for XAM filtering)

	// Check access: whitelist B (OU/AU) only
	// OAS-CONSOLE-08: XAM roles no longer have access
	isWhitelistB := authz.IsInAdminWhitelistB(h.DB, username.(string))

	if !isWhitelistB {
		response.Forbidden(c, "audit logs restricted to OU/AU admin")
		return
	}

	var items []oasmodel.AuditLog
	page, _ := parseInt(c.DefaultQuery("page", "1"))
	size, _ := parseInt(c.DefaultQuery("size", "20"))
	q := h.DB.Model(&oasmodel.AuditLog{})

	// OAS-CONSOLE-08: XAM domain filtering removed
	// if isXAM && !isWhitelistB {
	// 	domain, _ := c.Get("domain")
	// 	if domainStr, ok := domain.(string); ok && domainStr != "" {
	// 		q = q.Where("domain = ?", domainStr)
	// 	} else {
	// 		response.OK(c, gin.H{"items": []oasmodel.AuditLog{}, "total": 0, "page": page, "size": size})
	// 		return
	// 	}
	// }

	if uid := c.Query("user_id"); uid != "" {
		q = q.Where("user_id = ?", uid)
	}
	if plane := c.Query("plane"); plane != "" {
		q = q.Where("plane = ?", plane)
	}
	var total int64
	q.Count(&total)
	q.Order("created_at DESC").Offset((page - 1) * size).Limit(size).Find(&items)
	response.OK(c, gin.H{"items": items, "total": total, "page": page, "size": size})
}
