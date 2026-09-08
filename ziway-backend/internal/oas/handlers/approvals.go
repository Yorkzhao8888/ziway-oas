// approvals.go — 自 cmd/oas/main.go 下沉（OAS-CONSOLE-09 A1），行为零变化。
package handlers

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"ziway/backend/internal/oas/authz"
	oasmodel "ziway/backend/internal/oas/model"
	"ziway/backend/pkg/response"
)

func (h *Handlers) DeleteApproval(c *gin.Context) {
	username, _ := c.Get("username")
	domain, _ := c.Get("domain")
	oasEnv, _ := c.Get("oas_env")

	// 类型断言
	usernameStr, _ := username.(string)
	domainStr, _ := domain.(string)
	oasEnvStr, _ := oasEnv.(string)

	// 白名单 B: SU/OU/AU
	if !authz.IsInAdminWhitelistB(h.DB, usernameStr) {
		response.Forbidden(c, "only SU/OU/AU admin can delete approvals")
		return
	}

	id, _ := parseUint(c.Param("id"))
	var approval oasmodel.ApprovalRequest
	if err := h.DB.First(&approval, id).Error; err != nil {
		response.NotFound(c, "approval not found")
		return
	}

	if err := h.DB.Delete(&approval).Error; err != nil {
		response.InternalError(c, "delete failed: "+err.Error())
		return
	}

	// 审计日志
	h.DB.Create(&oasmodel.AuditLog{
		UserID:      usernameStr,
		UserName:    usernameStr,
		Plane:       "admin",
		Action:      "governance.approval.delete",
		Resource:    "approval_request",
		ResourceID:  fmt.Sprintf("%d", approval.ID),
		Detail:      fmt.Sprintf("title=%s, status=%s", approval.Title, approval.Status),
		IP:          c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
		Environment: oasEnvStr,
		Domain:      domainStr,
	})

	response.OK(c, gin.H{"message": "approval deleted"})
}

func (h *Handlers) ExecuteApproval(c *gin.Context) {
	username, _ := c.Get("username")
	domain, _ := c.Get("domain")
	oasEnv, _ := c.Get("oas_env")

	// 类型断言
	usernameStr, _ := username.(string)
	domainStr, _ := domain.(string)
	oasEnvStr, _ := oasEnv.(string)

	// 仅 OU/AU 可标记执行
	if usernameStr != "oas-ou-admin" && usernameStr != "oas-au-admin" {
		response.Forbidden(c, "only OU/AU admin can execute")
		return
	}

	id, _ := parseUint(c.Param("id"))
	var approval oasmodel.ApprovalRequest
	if err := h.DB.First(&approval, id).Error; err != nil {
		response.NotFound(c, "approval not found")
		return
	}

	if approval.Status != "approved" {
		response.BadRequest(c, "approval is not approved")
		return
	}

	now := time.Now()
	approval.Status = "executed"
	approval.ExecutedAt = &now

	if err := h.DB.Save(&approval).Error; err != nil {
		response.InternalError(c, "execute failed: "+err.Error())
		return
	}

	// 审计日志
	h.DB.Create(&oasmodel.AuditLog{
		UserID:      usernameStr,
		UserName:    usernameStr,
		Plane:       "admin",
		Action:      "governance.approval.execute",
		Resource:    "approval_request",
		ResourceID:  fmt.Sprintf("%d", approval.ID),
		Detail:      fmt.Sprintf("title=%s", approval.Title),
		IP:          c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
		Environment: oasEnvStr,
		Domain:      domainStr,
	})

	response.OK(c, approval)
}

func (h *Handlers) RejectApproval(c *gin.Context) {
	username, _ := c.Get("username")
	userID, _ := c.Get("user_id")
	domain, _ := c.Get("domain")
	oasEnv, _ := c.Get("oas_env")

	// 类型断言
	usernameStr, _ := username.(string)
	userIDStr, _ := userID.(string)
	domainStr, _ := domain.(string)
	oasEnvStr, _ := oasEnv.(string)

	// 仅 OU/AU 可拒绝
	if usernameStr != "oas-ou-admin" && usernameStr != "oas-au-admin" {
		response.Forbidden(c, "only OU/AU admin can reject")
		return
	}

	id, _ := parseUint(c.Param("id"))
	var approval oasmodel.ApprovalRequest
	if err := h.DB.First(&approval, id).Error; err != nil {
		response.NotFound(c, "approval not found")
		return
	}

	if approval.Status != "pending" {
		response.BadRequest(c, "approval is not pending")
		return
	}

	var req struct {
		Notes string `json:"notes"`
	}
	c.ShouldBindJSON(&req)

	approval.Status = "rejected"
	approval.ApproverID = ptrString(userIDStr)
	approval.Notes = req.Notes

	if err := h.DB.Save(&approval).Error; err != nil {
		response.InternalError(c, "reject failed: "+err.Error())
		return
	}

	// 审计日志
	h.DB.Create(&oasmodel.AuditLog{
		UserID:      usernameStr,
		UserName:    usernameStr,
		Plane:       "admin",
		Action:      "governance.approval.reject",
		Resource:    "approval_request",
		ResourceID:  fmt.Sprintf("%d", approval.ID),
		Detail:      fmt.Sprintf("title=%s, notes=%s", approval.Title, req.Notes),
		IP:          c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
		Environment: oasEnvStr,
		Domain:      domainStr,
	})

	response.OK(c, approval)
}

func (h *Handlers) ApproveApproval(c *gin.Context) {
	username, _ := c.Get("username")
	userID, _ := c.Get("user_id")
	domain, _ := c.Get("domain")
	oasEnv, _ := c.Get("oas_env")

	// 类型断言
	usernameStr, _ := username.(string)
	userIDStr, _ := userID.(string)
	domainStr, _ := domain.(string)
	oasEnvStr, _ := oasEnv.(string)

	// 白名单 B: SU/OU/AU
	if !authz.IsInAdminWhitelistB(h.DB, usernameStr) {
		response.Forbidden(c, "only SU/OU/AU admin can approve")
		return
	}

	id, _ := parseUint(c.Param("id"))
	var approval oasmodel.ApprovalRequest
	if err := h.DB.First(&approval, id).Error; err != nil {
		response.NotFound(c, "approval not found")
		return
	}

	if approval.Status != "pending" {
		response.BadRequest(c, "approval is not pending")
		return
	}

	now := time.Now()
	approval.Status = "approved"
	approval.ApproverID = ptrString(userIDStr)
	approval.ApprovedAt = &now

	if err := h.DB.Save(&approval).Error; err != nil {
		response.InternalError(c, "approve failed: "+err.Error())
		return
	}

	// 审计日志
	h.DB.Create(&oasmodel.AuditLog{
		UserID:      usernameStr,
		UserName:    usernameStr,
		Plane:       "admin",
		Action:      "governance.approval.approve",
		Resource:    "approval_request",
		ResourceID:  fmt.Sprintf("%d", approval.ID),
		Detail:      fmt.Sprintf("title=%s", approval.Title),
		IP:          c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
		Environment: oasEnvStr,
		Domain:      domainStr,
	})

	response.OK(c, approval)
}

func (h *Handlers) CreateApproval(c *gin.Context) {
	username, _ := c.Get("username")
	userID, _ := c.Get("user_id")
	domain, _ := c.Get("domain")
	oasEnv, _ := c.Get("oas_env")

	// 类型断言
	usernameStr, _ := username.(string)
	userIDStr, _ := userID.(string)
	domainStr, _ := domain.(string)
	oasEnvStr, _ := oasEnv.(string)

	// 白名单 B: SU/OU/AU
	if !authz.IsInAdminWhitelistB(h.DB, usernameStr) {
		response.Forbidden(c, "only SU/OU/AU admin can create approvals")
		return
	}

	var req struct {
		Title       string `json:"title" binding:"required"`
		Description string `json:"description"`
		Type        string `json:"type" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// 验证类型
	validTypes := map[string]bool{
		"high_privilege": true,
		"org_delete":     true,
		"key_operation":  true,
		"federation":     true,
	}
	if !validTypes[req.Type] {
		response.BadRequest(c, "invalid approval type")
		return
	}

	approval := oasmodel.ApprovalRequest{
		Title:       req.Title,
		Description: req.Description,
		Type:        req.Type,
		Status:      "pending",
		RequesterID: userIDStr,
		Domain:      domainStr,
		Environment: oasEnvStr,
	}

	if err := h.DB.Create(&approval).Error; err != nil {
		response.InternalError(c, "create approval failed: "+err.Error())
		return
	}

	// 审计日志
	h.DB.Create(&oasmodel.AuditLog{
		UserID:      usernameStr,
		UserName:    usernameStr,
		Plane:       "admin",
		Action:      "governance.approval.create",
		Resource:    "approval_request",
		ResourceID:  fmt.Sprintf("%d", approval.ID),
		Detail:      fmt.Sprintf("type=%s, title=%s", approval.Type, approval.Title),
		IP:          c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
		Environment: oasEnvStr,
		Domain:      domainStr,
	})

	response.Created(c, approval)
}

func (h *Handlers) ListApprovals(c *gin.Context) {
	username, _ := c.Get("username")
	usernameStr, _ := username.(string)

	// 白名单 B: SU/OU/AU
	if !authz.IsInAdminWhitelistB(h.DB, usernameStr) {
		response.Forbidden(c, "only SU/OU/AU admin can view approvals")
		return
	}

	var approvals []oasmodel.ApprovalRequest
	h.DB.Order("created_at DESC").Find(&approvals)
	response.OK(c, approvals)
}
