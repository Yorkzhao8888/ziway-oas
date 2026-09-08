// owner.go — 自 cmd/oas/main.go 下沉（OAS-CONSOLE-09 A1），行为零变化。
package handlers

import (
	"github.com/gin-gonic/gin"

	oasmodel "ziway/backend/internal/oas/model"
	"ziway/backend/pkg/response"
)

func (h *Handlers) UpdatePolicy(c *gin.Context) {
	var p oasmodel.GovernancePolicy
	if err := h.DB.First(&p, c.Param("id")).Error; err != nil {
		response.NotFound(c, "policy not found")
		return
	}
	c.ShouldBindJSON(&p)
	h.DB.Save(&p)
	response.OK(c, p)
}

func (h *Handlers) CreatePolicy(c *gin.Context) {
	var p oasmodel.GovernancePolicy
	if err := c.ShouldBindJSON(&p); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	h.DB.Create(&p)
	response.Created(c, p)
}

func (h *Handlers) ListPolicies(c *gin.Context) {
	var items []oasmodel.GovernancePolicy
	h.DB.Order("created_at DESC").Find(&items)
	response.OK(c, items)
}

func (h *Handlers) UpdateDomainStatus(c *gin.Context) {
	var body struct {
		Status string `json:"status"`
	}
	c.ShouldBindJSON(&body)
	h.DB.Model(&oasmodel.DomainRegistry{}).Where("id = ?", c.Param("id")).Update("status", body.Status)
	response.OK(c, nil)
}

func (h *Handlers) CreateDomain(c *gin.Context) {
	var d oasmodel.DomainRegistry
	if err := c.ShouldBindJSON(&d); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	h.DB.Create(&d)
	response.Created(c, d)
}

func (h *Handlers) ListDomains(c *gin.Context) {
	var items []oasmodel.DomainRegistry
	h.DB.Order("created_at DESC").Find(&items)
	response.OK(c, items)
}
