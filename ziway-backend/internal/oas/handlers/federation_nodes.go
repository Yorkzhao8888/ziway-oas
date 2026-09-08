// federation_nodes.go — 自 cmd/oas/main.go 下沉（OAS-CONSOLE-09 A1），行为零变化。
package handlers

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"ziway/backend/internal/oas/authz"
	oasmodel "ziway/backend/internal/oas/model"
	"ziway/backend/pkg/response"
)

func (h *Handlers) DeleteFederationNode(c *gin.Context) {
	username, _ := c.Get("username")
	if !authz.IsInAdminWhitelistB(h.DB, username.(string)) {
		response.Forbidden(c, "access denied")
		c.Abort()
		return
	}

	id := c.Param("id")
	var node oasmodel.FederationNode
	if err := h.DB.First(&node, id).Error; err != nil {
		response.NotFound(c, "node not found")
		return
	}

	h.DB.Delete(&node)

	// Audit log
	h.DB.Create(&oasmodel.AuditLog{
		UserID:     username.(string),
		UserName:   username.(string),
		Plane:      "admin",
		Action:     "governance.federation.delete",
		Resource:   "federation_node",
		ResourceID: id,
		Detail:     fmt.Sprintf("deleted federation node: %s", node.NodeName),
		Domain:     "OAS",
	})

	response.OK(c, gin.H{"message": "node deleted"})
}

func (h *Handlers) UpdateTrustLevel(c *gin.Context) {
	username, _ := c.Get("username")
	if !authz.IsInAdminWhitelistB(h.DB, username.(string)) {
		response.Forbidden(c, "access denied")
		c.Abort()
		return
	}

	id := c.Param("id")
	var node oasmodel.FederationNode
	if err := h.DB.First(&node, id).Error; err != nil {
		response.NotFound(c, "node not found")
		return
	}

	var req struct {
		TrustLevel string `json:"trust_level" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "trust_level required")
		return
	}

	// Validate trust level
	if req.TrustLevel != "basic" && req.TrustLevel != "standard" && req.TrustLevel != "full" {
		response.BadRequest(c, "trust_level must be basic, standard, or full")
		return
	}

	oldTrust := node.TrustLevel
	h.DB.Model(&node).Update("trust_level", req.TrustLevel)

	// Audit log
	h.DB.Create(&oasmodel.AuditLog{
		UserID:     username.(string),
		UserName:   username.(string),
		Plane:      "admin",
		Action:     "governance.federation.trust",
		Resource:   "federation_node",
		ResourceID: id,
		Detail:     fmt.Sprintf("changed trust level from %s to %s for node: %s", oldTrust, req.TrustLevel, node.NodeName),
		Domain:     "OAS",
	})

	response.OK(c, gin.H{"message": "trust level updated", "old_trust": oldTrust, "new_trust": req.TrustLevel})
}

func (h *Handlers) ActivateFederationNode(c *gin.Context) {
	username, _ := c.Get("username")
	if !authz.IsInAdminWhitelistB(h.DB, username.(string)) {
		response.Forbidden(c, "access denied")
		c.Abort()
		return
	}

	id := c.Param("id")
	var node oasmodel.FederationNode
	if err := h.DB.First(&node, id).Error; err != nil {
		response.NotFound(c, "node not found")
		return
	}

	h.DB.Model(&node).Update("status", "active")

	// Audit log
	h.DB.Create(&oasmodel.AuditLog{
		UserID:     username.(string),
		UserName:   username.(string),
		Plane:      "admin",
		Action:     "governance.federation.activate",
		Resource:   "federation_node",
		ResourceID: id,
		Detail:     fmt.Sprintf("activated federation node: %s", node.NodeName),
		Domain:     "OAS",
	})

	response.OK(c, gin.H{"message": "node activated"})
}

func (h *Handlers) SuspendFederationNode(c *gin.Context) {
	username, _ := c.Get("username")
	if !authz.IsInAdminWhitelistB(h.DB, username.(string)) {
		response.Forbidden(c, "access denied")
		c.Abort()
		return
	}

	id := c.Param("id")
	var node oasmodel.FederationNode
	if err := h.DB.First(&node, id).Error; err != nil {
		response.NotFound(c, "node not found")
		return
	}

	h.DB.Model(&node).Update("status", "suspended")

	// Audit log
	h.DB.Create(&oasmodel.AuditLog{
		UserID:     username.(string),
		UserName:   username.(string),
		Plane:      "admin",
		Action:     "governance.federation.suspend",
		Resource:   "federation_node",
		ResourceID: id,
		Detail:     fmt.Sprintf("suspended federation node: %s", node.NodeName),
		Domain:     "OAS",
	})

	response.OK(c, gin.H{"message": "node suspended"})
}

func (h *Handlers) UpdateFederationNode(c *gin.Context) {
	username, _ := c.Get("username")
	if !authz.IsInAdminWhitelistB(h.DB, username.(string)) {
		response.Forbidden(c, "access denied")
		c.Abort()
		return
	}

	id := c.Param("id")
	var node oasmodel.FederationNode
	if err := h.DB.First(&node, id).Error; err != nil {
		response.NotFound(c, "node not found")
		return
	}

	var req struct {
		NodeName     string `json:"node_name"`
		TrustLevel   string `json:"trust_level"`
		PublicKey    string `json:"public_key"`
		Endpoint     string `json:"endpoint"`
		Capabilities string `json:"capabilities"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}

	// Validate trust level
	if req.TrustLevel != "" && req.TrustLevel != "basic" && req.TrustLevel != "standard" && req.TrustLevel != "full" {
		response.BadRequest(c, "trust_level must be basic, standard, or full")
		return
	}

	updates := map[string]interface{}{}
	if req.NodeName != "" {
		updates["node_name"] = req.NodeName
	}
	if req.TrustLevel != "" {
		updates["trust_level"] = req.TrustLevel
	}
	if req.PublicKey != "" {
		updates["public_key"] = req.PublicKey
	}
	if req.Endpoint != "" {
		updates["endpoint"] = req.Endpoint
	}
	if req.Capabilities != "" {
		updates["capabilities"] = req.Capabilities
	}

	h.DB.Model(&node).Updates(updates)

	// Audit log
	h.DB.Create(&oasmodel.AuditLog{
		UserID:     username.(string),
		UserName:   username.(string),
		Plane:      "admin",
		Action:     "governance.federation.update",
		Resource:   "federation_node",
		ResourceID: id,
		Detail:     fmt.Sprintf("updated federation node: %s", node.NodeName),
		Domain:     "OAS",
	})

	h.DB.First(&node, id)
	response.OK(c, node)
}

func (h *Handlers) CreateFederationNode(c *gin.Context) {
	username, _ := c.Get("username")
	if !authz.IsInAdminWhitelistB(h.DB, username.(string)) {
		response.Forbidden(c, "access denied")
		c.Abort()
		return
	}

	var node oasmodel.FederationNode
	if err := c.ShouldBindJSON(&node); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}

	// Validate trust level
	if node.TrustLevel != "basic" && node.TrustLevel != "standard" && node.TrustLevel != "full" {
		response.BadRequest(c, "trust_level must be basic, standard, or full")
		return
	}

	node.CreatedBy = username.(string)
	node.Status = "active"
	h.DB.Create(&node)

	// Audit log
	h.DB.Create(&oasmodel.AuditLog{
		UserID:     username.(string),
		UserName:   username.(string),
		Plane:      "admin",
		Action:     "governance.federation.create",
		Resource:   "federation_node",
		ResourceID: fmt.Sprintf("%d", node.ID),
		Detail:     fmt.Sprintf("registered federation node: %s (trust: %s)", node.NodeName, node.TrustLevel),
		Domain:     "OAS",
	})

	response.Created(c, node)
}

func (h *Handlers) GetFederationNode(c *gin.Context) {
	username, _ := c.Get("username")
	if !authz.IsInAdminWhitelistB(h.DB, username.(string)) {
		response.Forbidden(c, "access denied")
		c.Abort()
		return
	}
	id := c.Param("id")
	var node oasmodel.FederationNode
	if err := h.DB.First(&node, id).Error; err != nil {
		response.NotFound(c, "node not found")
		return
	}
	response.OK(c, node)
}

func (h *Handlers) ListFederationNodes(c *gin.Context) {
	username, _ := c.Get("username")
	if !authz.IsInAdminWhitelistB(h.DB, username.(string)) {
		response.Forbidden(c, "access denied")
		c.Abort()
		return
	}
	var items []oasmodel.FederationNode
	h.DB.Order("created_at DESC").Find(&items)
	response.OK(c, items)
}
