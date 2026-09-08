// orgs.go — 自 cmd/oas/main.go 下沉（OAS-CONSOLE-09 A1），行为零变化。
package handlers

import (
	"fmt"

	"github.com/gin-gonic/gin"

	oasmodel "ziway/backend/internal/oas/model"
	"ziway/backend/pkg/middleware"
	"ziway/backend/pkg/model"
	"ziway/backend/pkg/response"
)

func (h *Handlers) RemoveOrgMember(c *gin.Context) {
	id, _ := parseUint(c.Param("id"))
	userId, _ := parseUint(c.Param("userId"))

	// First check if org exists and belongs to user's domain
	var org model.Organization
	orgQuery := h.DB
	if filterDomain, ok := middleware.GetFilterDomain(c); ok && filterDomain != "" {
		orgQuery = orgQuery.Where("domain = ?", filterDomain)
	}
	if err := orgQuery.First(&org, id).Error; err != nil {
		response.NotFound(c, "org not found")
		return
	}

	var member model.UserOrganization
	if err := h.DB.Where("organization_id = ? AND user_id = ?", org.ID, userId).First(&member).Error; err != nil {
		response.NotFound(c, "member not found")
		return
	}
	if err := h.DB.Delete(&member).Error; err != nil {
		response.InternalError(c, "remove member failed: "+err.Error())
		return
	}
	operator, _ := c.Get("user_id")
	domain, _ := c.Get("domain")
	h.DB.Create(&oasmodel.AuditLog{
		Plane:       "admin",
		Action:      "org.member.remove",
		UserID:      fmt.Sprintf("%v", operator),
		ResourceID:  fmt.Sprintf("org-%d", org.ID),
		Detail:      fmt.Sprintf("user_id=%d", userId),
		IP:          c.ClientIP(),
		Environment: h.OASEnv.String(),
		Domain:      fmt.Sprintf("%v", domain),
	})
	response.OK(c, gin.H{"message": "member removed"})
}

func (h *Handlers) AddOrgMember(c *gin.Context) {
	id, _ := parseUint(c.Param("id"))

	// First check if org exists and belongs to user's domain
	var org model.Organization
	orgQuery := h.DB
	if filterDomain, ok := middleware.GetFilterDomain(c); ok && filterDomain != "" {
		orgQuery = orgQuery.Where("domain = ?", filterDomain)
	}
	if err := orgQuery.First(&org, id).Error; err != nil {
		response.NotFound(c, "org not found")
		return
	}

	var req struct {
		UserID uint   `json:"user_id" binding:"required"`
		Role   string `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	// Check if user exists
	var user oasmodel.OASUser
	if err := h.DB.First(&user, req.UserID).Error; err != nil {
		response.NotFound(c, "user not found")
		return
	}
	// Check if already member
	var existing model.UserOrganization
	if err := h.DB.Where("user_id = ? AND organization_id = ?", req.UserID, org.ID).First(&existing).Error; err == nil {
		response.BadRequest(c, "user already member of this org")
		return
	}
	member := model.UserOrganization{
		UserID:         req.UserID,
		OrganizationID: org.ID,
		Role:           req.Role,
	}
	if err := h.DB.Create(&member).Error; err != nil {
		response.InternalError(c, "add member failed: "+err.Error())
		return
	}
	operator, _ := c.Get("user_id")
	domain, _ := c.Get("domain")
	h.DB.Create(&oasmodel.AuditLog{
		Plane:       "admin",
		Action:      "org.member.add",
		UserID:      fmt.Sprintf("%v", operator),
		ResourceID:  fmt.Sprintf("org-%d", org.ID),
		Detail:      fmt.Sprintf("user_id=%d, role=%s", req.UserID, req.Role),
		IP:          c.ClientIP(),
		Environment: h.OASEnv.String(),
		Domain:      fmt.Sprintf("%v", domain),
	})
	response.Created(c, member)
}

func (h *Handlers) ListOrgMembers(c *gin.Context) {
	id, _ := parseUint(c.Param("id"))

	// First check if org exists and belongs to user's domain
	var org model.Organization
	orgQuery := h.DB
	if filterDomain, ok := middleware.GetFilterDomain(c); ok && filterDomain != "" {
		orgQuery = orgQuery.Where("domain = ?", filterDomain)
	}
	if err := orgQuery.First(&org, id).Error; err != nil {
		response.NotFound(c, "org not found")
		return
	}

	var members []model.UserOrganization
	if err := h.DB.Where("organization_id = ?", id).Find(&members).Error; err != nil {
		response.InternalError(c, "load members failed: "+err.Error())
		return
	}
	response.OK(c, gin.H{"items": members, "total": len(members)})
}

func (h *Handlers) DeleteOrg(c *gin.Context) {
	id, _ := parseUint(c.Param("id"))
	var org model.Organization
	if err := h.DB.First(&org, id).Error; err != nil {
		response.NotFound(c, "org not found")
		return
	}
	// Check if has children
	var childCount int64
	h.DB.Model(&model.Organization{}).Where("parent_id = ?", org.ID).Count(&childCount)
	if childCount > 0 {
		response.BadRequest(c, "cannot delete org with children")
		return
	}
	// Check if has members
	var memberCount int64
	h.DB.Model(&model.UserOrganization{}).Where("organization_id = ?", org.ID).Count(&memberCount)
	if memberCount > 0 {
		response.BadRequest(c, "cannot delete org with members")
		return
	}
	if err := h.DB.Delete(&org).Error; err != nil {
		response.InternalError(c, "delete org failed: "+err.Error())
		return
	}
	operator, _ := c.Get("user_id")
	domain, _ := c.Get("domain")
	h.DB.Create(&oasmodel.AuditLog{
		Plane:       "admin",
		Action:      "org.delete",
		UserID:      fmt.Sprintf("%v", operator),
		ResourceID:  fmt.Sprintf("org-%d", org.ID),
		Detail:      fmt.Sprintf("code=%s", org.Code),
		IP:          c.ClientIP(),
		Environment: h.OASEnv.String(),
		Domain:      fmt.Sprintf("%v", domain),
	})
	response.OK(c, gin.H{"message": "org deleted"})
}

func (h *Handlers) UpdateOrg(c *gin.Context) {
	id, _ := parseUint(c.Param("id"))
	var org model.Organization
	if err := h.DB.First(&org, id).Error; err != nil {
		response.NotFound(c, "org not found")
		return
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		ParentID    *uint  `json:"parent_id"`
		Domain      string `json:"domain"`
		Status      string `json:"status"`
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
	if req.ParentID != nil {
		updates["parent_id"] = *req.ParentID
	}
	if req.Domain != "" {
		updates["domain"] = req.Domain
	}
	if req.Status != "" {
		updates["status"] = req.Status
	}
	if err := h.DB.Model(&org).Updates(updates).Error; err != nil {
		response.InternalError(c, "update org failed: "+err.Error())
		return
	}
	operator, _ := c.Get("user_id")
	domain, _ := c.Get("domain")
	h.DB.Create(&oasmodel.AuditLog{
		Plane:       "admin",
		Action:      "org.update",
		UserID:      fmt.Sprintf("%v", operator),
		ResourceID:  fmt.Sprintf("org-%d", org.ID),
		Detail:      fmt.Sprintf("updates=%v", updates),
		IP:          c.ClientIP(),
		Environment: h.OASEnv.String(),
		Domain:      fmt.Sprintf("%v", domain),
	})
	response.OK(c, org)
}

func (h *Handlers) CreateOrg(c *gin.Context) {
	var req struct {
		Code        string `json:"code" binding:"required"`
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
		ParentID    *uint  `json:"parent_id"`
		Domain      string `json:"domain"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	org := model.Organization{
		Code:        req.Code,
		Name:        req.Name,
		Description: req.Description,
		ParentID:    req.ParentID,
		Domain:      req.Domain,
		Status:      "active",
	}
	if err := h.DB.Create(&org).Error; err != nil {
		response.InternalError(c, "create org failed: "+err.Error())
		return
	}
	operator, _ := c.Get("user_id")
	domain, _ := c.Get("domain")
	h.DB.Create(&oasmodel.AuditLog{
		Plane:       "admin",
		Action:      "org.create",
		UserID:      fmt.Sprintf("%v", operator),
		ResourceID:  fmt.Sprintf("org-%d", org.ID),
		Detail:      fmt.Sprintf("code=%s, name=%s, domain=%s", org.Code, org.Name, org.Domain),
		IP:          c.ClientIP(),
		Environment: h.OASEnv.String(),
		Domain:      fmt.Sprintf("%v", domain),
	})
	response.Created(c, org)
}

func (h *Handlers) GetOrg(c *gin.Context) {
	id, _ := parseUint(c.Param("id"))
	var org model.Organization
	query := h.DB.Preload("Parent").Preload("Children").Preload("Members")

	// Apply domain filter if present
	if filterDomain, ok := middleware.GetFilterDomain(c); ok && filterDomain != "" {
		query = query.Where("domain = ?", filterDomain)
	}

	if err := query.First(&org, id).Error; err != nil {
		response.NotFound(c, "org not found")
		return
	}
	response.OK(c, org)
}

func (h *Handlers) ListOrgs(c *gin.Context) {
	var orgs []model.Organization
	query := h.DB.Preload("Parent").Preload("Children")

	// Apply domain filter if present
	if filterDomain, ok := middleware.GetFilterDomain(c); ok && filterDomain != "" {
		query = query.Where("domain = ?", filterDomain)
	}

	if err := query.Find(&orgs).Error; err != nil {
		response.InternalError(c, "load orgs failed: "+err.Error())
		return
	}
	// Build tree structure (only top-level orgs with children)
	orgMap := make(map[uint]*model.Organization)
	var topOrgs []model.Organization
	for i := range orgs {
		orgMap[orgs[i].ID] = &orgs[i]
	}
	for i := range orgs {
		if orgs[i].ParentID == nil {
			topOrgs = append(topOrgs, orgs[i])
		}
	}
	response.OK(c, gin.H{"items": topOrgs, "total": len(topOrgs)})
}
