// oauth_clients.go — 自 cmd/oas/main.go 下沉（OAS-CONSOLE-09 A1），行为零变化。
package handlers

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"ziway/backend/pkg/password"

	"ziway/backend/internal/oas/authz"
	oasmodel "ziway/backend/internal/oas/model"
	"ziway/backend/pkg/response"
)

func (h *Handlers) DeleteOAuthClient(c *gin.Context) {
	username, _ := c.Get("username")
	if !authz.IsInAdminWhitelistB(h.DB, username.(string)) {
		response.Forbidden(c, "access denied")
		c.Abort()
		return
	}

	id := c.Param("id")
	var client oasmodel.OAuthClient
	if err := h.DB.First(&client, id).Error; err != nil {
		response.NotFound(c, "client not found")
		return
	}

	h.DB.Delete(&client)

	// Audit log
	h.DB.Create(&oasmodel.AuditLog{
		UserID:     username.(string),
		UserName:   username.(string),
		Plane:      "admin",
		Action:     "oauth.client.delete",
		Resource:   "oauth_client",
		ResourceID: client.ClientID,
		Detail:     fmt.Sprintf("deleted OAuth client: %s", client.ClientName),
		Domain:     "OAS",
	})

	response.OK(c, gin.H{"message": "client deleted"})
}

func (h *Handlers) ActivateOAuthClient(c *gin.Context) {
	username, _ := c.Get("username")
	if !authz.IsInAdminWhitelistB(h.DB, username.(string)) {
		response.Forbidden(c, "access denied")
		c.Abort()
		return
	}

	id := c.Param("id")
	var client oasmodel.OAuthClient
	if err := h.DB.First(&client, id).Error; err != nil {
		response.NotFound(c, "client not found")
		return
	}

	h.DB.Model(&client).Update("status", "active")

	// Audit log
	h.DB.Create(&oasmodel.AuditLog{
		UserID:     username.(string),
		UserName:   username.(string),
		Plane:      "admin",
		Action:     "oauth.client.activate",
		Resource:   "oauth_client",
		ResourceID: client.ClientID,
		Detail:     fmt.Sprintf("activated OAuth client: %s", client.ClientName),
		Domain:     "OAS",
	})

	response.OK(c, gin.H{"message": "client activated"})
}

func (h *Handlers) SuspendOAuthClient(c *gin.Context) {
	username, _ := c.Get("username")
	if !authz.IsInAdminWhitelistB(h.DB, username.(string)) {
		response.Forbidden(c, "access denied")
		c.Abort()
		return
	}

	id := c.Param("id")
	var client oasmodel.OAuthClient
	if err := h.DB.First(&client, id).Error; err != nil {
		response.NotFound(c, "client not found")
		return
	}

	h.DB.Model(&client).Update("status", "inactive")

	// Audit log
	h.DB.Create(&oasmodel.AuditLog{
		UserID:     username.(string),
		UserName:   username.(string),
		Plane:      "admin",
		Action:     "oauth.client.suspend",
		Resource:   "oauth_client",
		ResourceID: client.ClientID,
		Detail:     fmt.Sprintf("suspended OAuth client: %s", client.ClientName),
		Domain:     "OAS",
	})

	response.OK(c, gin.H{"message": "client suspended"})
}

func (h *Handlers) RegenerateClientSecret(c *gin.Context) {
	username, _ := c.Get("username")
	if !authz.IsInAdminWhitelistB(h.DB, username.(string)) {
		response.Forbidden(c, "access denied")
		c.Abort()
		return
	}

	id := c.Param("id")
	var client oasmodel.OAuthClient
	if err := h.DB.First(&client, id).Error; err != nil {
		response.NotFound(c, "client not found")
		return
	}

	// Generate new secret
	plainSecret := "ocs_" + fmt.Sprintf("%x", time.Now().UnixNano()) + fmt.Sprintf("%x", time.Now().UnixNano()%0xFFFFFF)
	hashedSecret, _ := password.Hash(plainSecret)

	h.DB.Model(&client).Update("client_secret", string(hashedSecret))

	// Audit log
	h.DB.Create(&oasmodel.AuditLog{
		UserID:     username.(string),
		UserName:   username.(string),
		Plane:      "admin",
		Action:     "oauth.client.rotate_secret",
		Resource:   "oauth_client",
		ResourceID: client.ClientID,
		Detail:     fmt.Sprintf("regenerated secret for OAuth client: %s", client.ClientName),
		Domain:     "OAS",
	})

	response.OK(c, gin.H{
		"client_secret": plainSecret,
		"message":       "Save the new client_secret now. It will not be shown again.",
	})
}

func (h *Handlers) UpdateOAuthClient(c *gin.Context) {
	username, _ := c.Get("username")
	if !authz.IsInAdminWhitelistB(h.DB, username.(string)) {
		response.Forbidden(c, "access denied")
		c.Abort()
		return
	}

	id := c.Param("id")
	var client oasmodel.OAuthClient
	if err := h.DB.First(&client, id).Error; err != nil {
		response.NotFound(c, "client not found")
		return
	}

	var req struct {
		ClientName  string `json:"client_name"`
		RedirectURI string `json:"redirect_uri"`
		Scopes      string `json:"scopes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}

	updates := map[string]interface{}{}
	if req.ClientName != "" {
		updates["client_name"] = req.ClientName
	}
	if req.RedirectURI != "" {
		updates["redirect_uri"] = req.RedirectURI
	}
	if req.Scopes != "" {
		updates["scopes"] = req.Scopes
	}

	h.DB.Model(&client).Updates(updates)

	// Audit log
	h.DB.Create(&oasmodel.AuditLog{
		UserID:     username.(string),
		UserName:   username.(string),
		Plane:      "admin",
		Action:     "oauth.client.update",
		Resource:   "oauth_client",
		ResourceID: client.ClientID,
		Detail:     fmt.Sprintf("updated OAuth client: %s", client.ClientName),
		Domain:     "OAS",
	})

	h.DB.First(&client, id)
	response.OK(c, client)
}

func (h *Handlers) CreateOAuthClient(c *gin.Context) {
	username, _ := c.Get("username")
	if !authz.IsInAdminWhitelistB(h.DB, username.(string)) {
		response.Forbidden(c, "access denied")
		c.Abort()
		return
	}

	var req struct {
		ClientName  string `json:"client_name"`
		RedirectURI string `json:"redirect_uri"`
		Scopes      string `json:"scopes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	if req.ClientName == "" || req.RedirectURI == "" {
		response.BadRequest(c, "client_name and redirect_uri required")
		return
	}

	// Generate client_id and client_secret
	clientID := "oauth_" + fmt.Sprintf("%d", time.Now().UnixNano()%100000) + "_" + fmt.Sprintf("%x", time.Now().UnixNano()%0xFFFFFF)
	plainSecret := "ocs_" + fmt.Sprintf("%x", time.Now().UnixNano()) + fmt.Sprintf("%x", time.Now().UnixNano()%0xFFFFFF)
	hashedSecret, _ := password.Hash(plainSecret)

	if req.Scopes == "" {
		req.Scopes = "openid profile email"
	}

	client := oasmodel.OAuthClient{
		ClientID:     clientID,
		ClientName:   req.ClientName,
		ClientSecret: string(hashedSecret),
		RedirectURI:  req.RedirectURI,
		Scopes:       req.Scopes,
		Status:       "active",
		CreatedBy:    username.(string),
	}
	if err := h.DB.Create(&client).Error; err != nil {
		response.InternalError(c, "failed to create client")
		return
	}

	// Audit log
	h.DB.Create(&oasmodel.AuditLog{
		UserID:     username.(string),
		UserName:   username.(string),
		Plane:      "admin",
		Action:     "oauth.client.create",
		Resource:   "oauth_client",
		ResourceID: clientID,
		Detail:     fmt.Sprintf("created OAuth client: %s (%s)", req.ClientName, clientID),
		Domain:     "OAS",
	})

	// Return client with plain secret (only shown once)
	response.OK(c, gin.H{
		"client":        client,
		"client_secret": plainSecret,
		"message":       "Save the client_secret now. It will not be shown again.",
	})
}

func (h *Handlers) GetOAuthClient(c *gin.Context) {
	username, _ := c.Get("username")
	if !authz.IsInAdminWhitelistB(h.DB, username.(string)) {
		response.Forbidden(c, "access denied")
		c.Abort()
		return
	}
	id := c.Param("id")
	var client oasmodel.OAuthClient
	if err := h.DB.First(&client, id).Error; err != nil {
		response.NotFound(c, "client not found")
		return
	}
	response.OK(c, client)
}

func (h *Handlers) ListOAuthClients(c *gin.Context) {
	username, _ := c.Get("username")
	if !authz.IsInAdminWhitelistB(h.DB, username.(string)) {
		response.Forbidden(c, "access denied")
		c.Abort()
		return
	}
	var clients []oasmodel.OAuthClient
	h.DB.Order("created_at DESC").Find(&clients)
	response.OK(c, clients)
}
