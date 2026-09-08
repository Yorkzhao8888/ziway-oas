// api_keys.go — 自 cmd/oas/main.go 下沉（OAS-CONSOLE-09 A1），行为零变化。
package handlers

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"math/big"

	"ziway/backend/internal/oas/authz"
	oasmodel "ziway/backend/internal/oas/model"
	"ziway/backend/pkg/response"
)

func (h *Handlers) DeleteAPIKey(c *gin.Context) {
	username, _ := c.Get("username")
	if !authz.IsInAdminWhitelistB(h.DB, username.(string)) {
		response.Forbidden(c, "access denied")
		c.Abort()
		return
	}

	id := c.Param("id")
	var k oasmodel.APIKey
	if err := h.DB.First(&k, id).Error; err != nil {
		response.NotFound(c, "key not found")
		return
	}

	h.DB.Delete(&k)

	// Audit log
	h.DB.Create(&oasmodel.AuditLog{
		UserID:     username.(string),
		UserName:   username.(string),
		Plane:      "admin",
		Action:     "governance.apikey.delete",
		Resource:   "apikey",
		ResourceID: id,
		Detail:     fmt.Sprintf("deleted api key: %s", k.KeyName),
		Domain:     "OAS",
	})

	response.OK(c, gin.H{"message": "key deleted"})
}

func (h *Handlers) EnableAPIKey(c *gin.Context) {
	username, _ := c.Get("username")
	if !authz.IsInAdminWhitelistB(h.DB, username.(string)) {
		response.Forbidden(c, "access denied")
		c.Abort()
		return
	}

	id := c.Param("id")
	var k oasmodel.APIKey
	if err := h.DB.First(&k, id).Error; err != nil {
		response.NotFound(c, "key not found")
		return
	}

	// Check expiry
	if k.ExpiresAt != nil && k.ExpiresAt.Before(time.Now()) {
		response.BadRequest(c, "cannot enable expired key")
		return
	}

	h.DB.Model(&k).Update("status", "active")

	// Audit log
	h.DB.Create(&oasmodel.AuditLog{
		UserID:     username.(string),
		UserName:   username.(string),
		Plane:      "admin",
		Action:     "governance.apikey.enable",
		Resource:   "apikey",
		ResourceID: id,
		Detail:     fmt.Sprintf("enabled api key: %s", k.KeyName),
		Domain:     "OAS",
	})

	response.OK(c, gin.H{"message": "key enabled"})
}

func (h *Handlers) DisableAPIKey(c *gin.Context) {
	username, _ := c.Get("username")
	if !authz.IsInAdminWhitelistB(h.DB, username.(string)) {
		response.Forbidden(c, "access denied")
		c.Abort()
		return
	}

	id := c.Param("id")
	var k oasmodel.APIKey
	if err := h.DB.First(&k, id).Error; err != nil {
		response.NotFound(c, "key not found")
		return
	}

	h.DB.Model(&k).Update("status", "disabled")

	// Audit log
	h.DB.Create(&oasmodel.AuditLog{
		UserID:     username.(string),
		UserName:   username.(string),
		Plane:      "admin",
		Action:     "governance.apikey.disable",
		Resource:   "apikey",
		ResourceID: id,
		Detail:     fmt.Sprintf("disabled api key: %s", k.KeyName),
		Domain:     "OAS",
	})

	response.OK(c, gin.H{"message": "key disabled"})
}

func (h *Handlers) RotateAPIKey(c *gin.Context) {
	username, _ := c.Get("username")
	if !authz.IsInAdminWhitelistB(h.DB, username.(string)) {
		response.Forbidden(c, "access denied")
		c.Abort()
		return
	}

	id := c.Param("id")
	var oldKey oasmodel.APIKey
	if err := h.DB.First(&oldKey, id).Error; err != nil {
		response.NotFound(c, "key not found")
		return
	}

	if oldKey.Status != "active" {
		response.BadRequest(c, "can only rotate active keys")
		return
	}

	// Generate new key
	prefix := "oas_" + fmt.Sprintf("%d", time.Now().UnixNano()%10000)
	randomPart := fmt.Sprintf("%x", time.Now().UnixNano()) + fmt.Sprintf("%x", big.NewInt(time.Now().UnixNano()).Int64())
	fullKey := prefix + "_" + randomPart[:32]

	hash, err := bcrypt.GenerateFromPassword([]byte(fullKey), bcrypt.DefaultCost)
	if err != nil {
		response.InternalError(c, "failed to hash key")
		return
	}

	// Atomic rotation: disable old key + create new key
	newKey := oasmodel.APIKey{
		KeyName:   oldKey.KeyName + " (rotated)",
		KeyPrefix: prefix,
		KeyHash:   string(hash),
		Scopes:    oldKey.Scopes,
		ExpiresAt: oldKey.ExpiresAt,
		Status:    "active",
		CreatedBy: username.(string),
	}

	// Transaction for atomicity
	tx := h.DB.Begin()
	if err := tx.Model(&oasmodel.APIKey{}).Where("id = ?", oldKey.ID).Update("status", "disabled").Error; err != nil {
		tx.Rollback()
		response.InternalError(c, "failed to disable old key")
		return
	}
	if err := tx.Create(&newKey).Error; err != nil {
		tx.Rollback()
		response.InternalError(c, "failed to create new key")
		return
	}
	tx.Commit()

	// Audit log
	h.DB.Create(&oasmodel.AuditLog{
		UserID:     username.(string),
		UserName:   username.(string),
		Plane:      "admin",
		Action:     "governance.apikey.rotate",
		Resource:   "apikey",
		ResourceID: fmt.Sprintf("%d", newKey.ID),
		Detail:     fmt.Sprintf("rotated api key from id=%d to id=%d", oldKey.ID, newKey.ID),
		Domain:     "OAS",
	})

	response.OK(c, gin.H{
		"id":         newKey.ID,
		"key_name":   newKey.KeyName,
		"key_prefix": newKey.KeyPrefix,
		"full_key":   fullKey,
		"scopes":     newKey.Scopes,
		"expires_at": newKey.ExpiresAt,
		"status":     newKey.Status,
		"old_key_id": oldKey.ID,
		"message":    "Key rotated. Old key disabled. Save new key now.",
	})
}

func (h *Handlers) CreateAPIKey(c *gin.Context) {
	username, _ := c.Get("username")
	if !authz.IsInAdminWhitelistB(h.DB, username.(string)) {
		response.Forbidden(c, "access denied")
		c.Abort()
		return
	}

	var req struct {
		KeyName   string     `json:"key_name"`
		Scopes    string     `json:"scopes"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}

	// Generate API key: prefix + random part
	prefix := "oas_" + fmt.Sprintf("%d", time.Now().UnixNano()%10000)
	randomPart := fmt.Sprintf("%x", time.Now().UnixNano()) + fmt.Sprintf("%x", big.NewInt(time.Now().UnixNano()).Int64())
	fullKey := prefix + "_" + randomPart[:32]

	// Hash the key
	hash, err := bcrypt.GenerateFromPassword([]byte(fullKey), bcrypt.DefaultCost)
	if err != nil {
		response.InternalError(c, "failed to hash key")
		return
	}

	k := oasmodel.APIKey{
		KeyName:   req.KeyName,
		KeyPrefix: prefix,
		KeyHash:   string(hash),
		Scopes:    req.Scopes,
		ExpiresAt: req.ExpiresAt,
		Status:    "active",
		CreatedBy: username.(string),
	}
	h.DB.Create(&k)

	// Audit log
	h.DB.Create(&oasmodel.AuditLog{
		UserID:     username.(string),
		UserName:   username.(string),
		Plane:      "admin",
		Action:     "governance.apikey.create",
		Resource:   "apikey",
		ResourceID: fmt.Sprintf("%d", k.ID),
		Detail:     fmt.Sprintf("created api key: %s", req.KeyName),
		Domain:     "OAS",
	})

	// Return full key only once
	response.Created(c, gin.H{
		"id":         k.ID,
		"key_name":   k.KeyName,
		"key_prefix": k.KeyPrefix,
		"full_key":   fullKey,
		"scopes":     k.Scopes,
		"expires_at": k.ExpiresAt,
		"status":     k.Status,
		"message":    "Save this key now. You won't be able to see it again.",
	})
}

func (h *Handlers) GetAPIKey(c *gin.Context) {
	username, _ := c.Get("username")
	if !authz.IsInAdminWhitelistB(h.DB, username.(string)) {
		response.Forbidden(c, "access denied")
		c.Abort()
		return
	}
	id := c.Param("id")
	var k oasmodel.APIKey
	if err := h.DB.First(&k, id).Error; err != nil {
		response.NotFound(c, "key not found")
		return
	}
	response.OK(c, k)
}

func (h *Handlers) ListAPIKeys(c *gin.Context) {
	username, _ := c.Get("username")
	if !authz.IsInAdminWhitelistB(h.DB, username.(string)) {
		response.Forbidden(c, "access denied")
		c.Abort()
		return
	}
	var items []oasmodel.APIKey
	h.DB.Order("created_at DESC").Find(&items)
	response.OK(c, items)
}
