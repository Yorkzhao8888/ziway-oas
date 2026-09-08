// roles.go — 自 cmd/oas/main.go 下沉（OAS-CONSOLE-09 A1），行为零变化。
package handlers

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	oasmodel "ziway/backend/internal/oas/model"
	"ziway/backend/pkg/response"
)

func (h *Handlers) DeleteRole(c *gin.Context) {
	id := c.Param("id")
	var role oasmodel.OASRole
	if err := h.DB.First(&role, id).Error; err != nil {
		response.NotFound(c, "role not found")
		return
	}
	// Check if role is assigned to any users
	var count int64
	h.DB.Table("user_roles").Where("role_id = ?", id).Count(&count)
	if count > 0 {
		response.BadRequest(c, fmt.Sprintf("role is assigned to %d users, cannot delete", count))
		return
	}
	h.DB.Delete(&role, id)
	// Audit h.Log
	operator, _ := c.Get("username")
	h.DB.Create(&oasmodel.AuditLog{
		Action:     "role.delete",
		Plane:      "admin",
		UserID:     fmt.Sprintf("%v", operator),
		ResourceID: fmt.Sprintf("role-%s", id),
		Detail:     fmt.Sprintf("role_code=%s", role.RoleCode),
		IP:         c.ClientIP(),
	})
	response.OK(c, gin.H{"message": "role deleted"})
}

func (h *Handlers) UpdateRole(c *gin.Context) {
	id := c.Param("id")
	var role oasmodel.OASRole
	if err := h.DB.First(&role, id).Error; err != nil {
		response.NotFound(c, "role not found")
		return
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Permissions string `json:"permissions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.Permissions != "" {
		updates["permissions"] = req.Permissions
	}
	if len(updates) > 0 {
		h.DB.Model(&role).Updates(updates)
	}
	// Audit h.Log
	operator, _ := c.Get("username")
	h.DB.Create(&oasmodel.AuditLog{
		Action:     "role.update",
		Plane:      "admin",
		UserID:     fmt.Sprintf("%v", operator),
		ResourceID: fmt.Sprintf("role-%s", id),
		Detail:     fmt.Sprintf("updates=%v", updates),
		IP:         c.ClientIP(),
	})
	response.OK(c, gin.H{"message": "role updated"})
}

func (h *Handlers) CreateRole(c *gin.Context) {
	var req struct {
		RoleCode    string `json:"role_code" binding:"required"`
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
		Permissions string `json:"permissions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	role := oasmodel.OASRole{
		RoleCode:    req.RoleCode,
		Name:        req.Name,
		Description: req.Description,
		Permissions: req.Permissions,
	}
	if err := h.DB.Create(&role).Error; err != nil {
		response.InternalError(c, "create role failed: "+err.Error())
		return
	}
	// Audit h.Log
	operator, _ := c.Get("username")
	h.DB.Create(&oasmodel.AuditLog{
		Action:     "role.create",
		Plane:      "admin",
		UserID:     fmt.Sprintf("%v", operator),
		ResourceID: fmt.Sprintf("role-%d", role.ID),
		Detail:     fmt.Sprintf("role_code=%s, name=%s", role.RoleCode, role.Name),
		IP:         c.ClientIP(),
	})
	response.Created(c, gin.H{"id": role.ID, "role_code": role.RoleCode, "name": role.Name})
}

func (h *Handlers) ListRoles(c *gin.Context) {
	var roles []oasmodel.OASRole
	h.DB.Order("role_code").Find(&roles)
	type RoleDetail struct {
		ID          uint64 `json:"id"`
		RoleCode    string `json:"role_code"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Permissions string `json:"permissions"`
		CreatedAt   string `json:"created_at"`
		UpdatedAt   string `json:"updated_at"`
	}
	var result []RoleDetail
	for _, r := range roles {
		result = append(result, RoleDetail{
			ID:          r.ID,
			RoleCode:    r.RoleCode,
			Name:        r.Name,
			Description: r.Description,
			Permissions: r.Permissions,
			CreatedAt:   r.CreatedAt.Format(time.RFC3339),
			UpdatedAt:   r.UpdatedAt.Format(time.RFC3339),
		})
	}
	response.OK(c, gin.H{"items": result, "total": len(result)})
}
