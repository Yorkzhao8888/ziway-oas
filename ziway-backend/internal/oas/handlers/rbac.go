// rbac.go — 自 cmd/oas/main.go 下沉（OAS-CONSOLE-09 A1），行为零变化。
package handlers

import (
	"github.com/gin-gonic/gin"

	oasmodel "ziway/backend/internal/oas/model"
	"ziway/backend/pkg/response"
)

func (h *Handlers) SyncRBAC(c *gin.Context) {
	h.RegeneratePolicyCSV(h.DB, h.Log)
	response.OK(c, gin.H{"message": "policy CSV regenerated"})
}

func (h *Handlers) DeleteRBACPolicy(c *gin.Context) {
	h.DB.Delete(&oasmodel.RBACPolicy{}, c.Param("id"))
	h.RegeneratePolicyCSV(h.DB, h.Log)
	response.OK(c, nil)
}

func (h *Handlers) UpdateRBACPolicy(c *gin.Context) {
	var p oasmodel.RBACPolicy
	if err := h.DB.First(&p, c.Param("id")).Error; err != nil {
		response.NotFound(c, "policy not found")
		return
	}
	c.ShouldBindJSON(&p)
	p.ID, _ = parseUint(c.Param("id"))
	h.DB.Save(&p)
	h.RegeneratePolicyCSV(h.DB, h.Log)
	response.OK(c, p)
}

func (h *Handlers) CreateRBACPolicy(c *gin.Context) {
	var p oasmodel.RBACPolicy
	if err := c.ShouldBindJSON(&p); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	p.PolicyType = "rbac"
	if p.Effect == "" {
		p.Effect = "allow"
	}
	if p.Status == "" {
		p.Status = "active"
	}
	if err := h.DB.Create(&p).Error; err != nil {
		response.BadRequest(c, "create policy failed: "+err.Error())
		return
	}
	h.RegeneratePolicyCSV(h.DB, h.Log)
	response.Created(c, p)
}

func (h *Handlers) ListRBACPolicies(c *gin.Context) {
	var items []oasmodel.RBACPolicy
	q := h.DB.Model(&oasmodel.RBACPolicy{}).Where("policy_type = ?", "rbac")
	if role := c.Query("role_type"); role != "" {
		q = q.Where("role_type = ?", role)
	}
	if subject := c.Query("subject"); subject != "" {
		q = q.Where("subject = ?", subject)
	}
	q.Order("subject, resource").Find(&items)
	response.OK(c, gin.H{"items": items, "total": len(items)})
}
