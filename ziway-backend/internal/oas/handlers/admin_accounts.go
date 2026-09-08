// admin_accounts.go — 自 cmd/oas/main.go 下沉（OAS-CONSOLE-09 A1），行为零变化。
package handlers

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"ziway/backend/internal/oas/authz"
	oasmodel "ziway/backend/internal/oas/model"
	"ziway/backend/pkg/response"
)

func (h *Handlers) ResetAdminPassword(c *gin.Context) {
	username, _ := c.Get("username")
	domain, _ := c.Get("domain")
	oasEnv, _ := c.Get("oas_env")

	usernameStr, _ := username.(string)
	domainStr, _ := domain.(string)
	oasEnvStr, _ := oasEnv.(string)

	if usernameStr != "oas-ou-admin" && usernameStr != "oas-au-admin" {
		response.Forbidden(c, "only OU/AU admin can reset passwords")
		return
	}

	id, _ := parseUint(c.Param("id"))
	var user oasmodel.OASUser
	if err := h.DB.First(&user, id).Error; err != nil {
		response.NotFound(c, "user not found")
		return
	}

	if user.RoleCode != "OU" && user.RoleCode != "AU" {
		response.BadRequest(c, "can only reset OU/AU admin passwords")
		return
	}

	var req struct {
		NewPassword string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		response.InternalError(c, "password hash failed: "+err.Error())
		return
	}

	user.PasswordHash = string(passwordHash)
	if err := h.DB.Save(&user).Error; err != nil {
		response.InternalError(c, "reset password failed: "+err.Error())
		return
	}

	// 审计日志
	h.DB.Create(&oasmodel.AuditLog{
		UserID:      usernameStr,
		UserName:    usernameStr,
		Plane:       "admin",
		Action:      "admin.account.reset_password",
		Resource:    "user",
		ResourceID:  fmt.Sprintf("%d", user.ID),
		Detail:      fmt.Sprintf("username=%s, role=%s", user.Username, user.RoleCode),
		IP:          c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
		Environment: oasEnvStr,
		Domain:      domainStr,
	})

	response.OK(c, gin.H{"message": "password reset successfully"})
}

func (h *Handlers) EnableAdminAccount(c *gin.Context) {
	username, _ := c.Get("username")
	domain, _ := c.Get("domain")
	oasEnv, _ := c.Get("oas_env")

	usernameStr, _ := username.(string)
	domainStr, _ := domain.(string)
	oasEnvStr, _ := oasEnv.(string)

	if usernameStr != "oas-ou-admin" && usernameStr != "oas-au-admin" {
		response.Forbidden(c, "only OU/AU admin can enable admin accounts")
		return
	}

	id, _ := parseUint(c.Param("id"))
	var user oasmodel.OASUser
	if err := h.DB.First(&user, id).Error; err != nil {
		response.NotFound(c, "user not found")
		return
	}

	if user.RoleCode != "OU" && user.RoleCode != "AU" {
		response.BadRequest(c, "can only enable OU/AU admin accounts")
		return
	}

	user.Status = "active"
	if err := h.DB.Save(&user).Error; err != nil {
		response.InternalError(c, "enable failed: "+err.Error())
		return
	}

	// 审计日志
	h.DB.Create(&oasmodel.AuditLog{
		UserID:      usernameStr,
		UserName:    usernameStr,
		Plane:       "admin",
		Action:      "admin.account.enable",
		Resource:    "user",
		ResourceID:  fmt.Sprintf("%d", user.ID),
		Detail:      fmt.Sprintf("username=%s, role=%s", user.Username, user.RoleCode),
		IP:          c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
		Environment: oasEnvStr,
		Domain:      domainStr,
	})

	response.OK(c, user)
}

func (h *Handlers) DisableAdminAccount(c *gin.Context) {
	username, _ := c.Get("username")
	domain, _ := c.Get("domain")
	oasEnv, _ := c.Get("oas_env")

	usernameStr, _ := username.(string)
	domainStr, _ := domain.(string)
	oasEnvStr, _ := oasEnv.(string)

	if usernameStr != "oas-ou-admin" && usernameStr != "oas-au-admin" {
		response.Forbidden(c, "only OU/AU admin can disable admin accounts")
		return
	}

	id, _ := parseUint(c.Param("id"))
	var user oasmodel.OASUser
	if err := h.DB.First(&user, id).Error; err != nil {
		response.NotFound(c, "user not found")
		return
	}

	// 只能禁用 OU/AU 账号
	if user.RoleCode != "OU" && user.RoleCode != "AU" {
		response.BadRequest(c, "can only disable OU/AU admin accounts")
		return
	}

	user.Status = "disabled"
	if err := h.DB.Save(&user).Error; err != nil {
		response.InternalError(c, "disable failed: "+err.Error())
		return
	}

	// 审计日志
	h.DB.Create(&oasmodel.AuditLog{
		UserID:      usernameStr,
		UserName:    usernameStr,
		Plane:       "admin",
		Action:      "admin.account.disable",
		Resource:    "user",
		ResourceID:  fmt.Sprintf("%d", user.ID),
		Detail:      fmt.Sprintf("username=%s, role=%s", user.Username, user.RoleCode),
		IP:          c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
		Environment: oasEnvStr,
		Domain:      domainStr,
	})

	response.OK(c, user)
}

func (h *Handlers) CreateAdminAccount(c *gin.Context) {
	username, _ := c.Get("username")
	domain, _ := c.Get("domain")
	oasEnv, _ := c.Get("oas_env")

	usernameStr, _ := username.(string)
	domainStr, _ := domain.(string)
	oasEnvStr, _ := oasEnv.(string)

	// Check if it's an API key or whitelist B
	authType, _ := c.Get("auth_type")
	if authType != "api_key" && !authz.IsInAdminWhitelistB(h.DB, usernameStr) {
		response.Forbidden(c, "only OU/AU admin can create admin accounts")
		return
	}

	var req struct {
		Username    string `json:"username" binding:"required"`
		Password    string `json:"password" binding:"required"`
		DisplayName string `json:"display_name"`
		RoleCode    string `json:"role_code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	// 仅允许 OU/AU 角色
	if req.RoleCode != "OU" && req.RoleCode != "AU" {
		response.BadRequest(c, "role_code must be OU or AU")
		return
	}

	// 检查用户名是否已存在
	var count int64
	h.DB.Model(&oasmodel.OASUser{}).Where("username = ?", req.Username).Count(&count)
	if count > 0 {
		response.BadRequest(c, "username already exists")
		return
	}

	// 生成 user_code
	userCode := fmt.Sprintf("XHPZ#%s-%d", req.RoleCode, time.Now().UnixNano()%100000000)

	// 密码哈希
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		response.InternalError(c, "password hash failed: "+err.Error())
		return
	}

	user := oasmodel.OASUser{
		Username:     req.Username,
		UserCode:     userCode,
		DisplayName:  req.DisplayName,
		PasswordHash: string(passwordHash),
		RoleCode:     req.RoleCode,
		Status:       "active",
		Domain:       domainStr,
	}

	if err := h.DB.Create(&user).Error; err != nil {
		response.InternalError(c, "create failed: "+err.Error())
		return
	}

	// 审计日志
	h.DB.Create(&oasmodel.AuditLog{
		UserID:      usernameStr,
		UserName:    usernameStr,
		Plane:       "admin",
		Action:      "admin.account.create",
		Resource:    "user",
		ResourceID:  fmt.Sprintf("%d", user.ID),
		Detail:      fmt.Sprintf("username=%s, role=%s", user.Username, user.RoleCode),
		IP:          c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
		Environment: oasEnvStr,
		Domain:      domainStr,
	})

	response.OK(c, user)
}

func (h *Handlers) ListAdminAccounts(c *gin.Context) {
	username, _ := c.Get("username")
	usernameStr, _ := username.(string)

	// Check if it's an API key
	authType, _ := c.Get("auth_type")
	if authType != "api_key" {
		// 白名单 B: SU/OU/AU
		if !authz.IsInAdminWhitelistB(h.DB, usernameStr) {
			response.Forbidden(c, "only SU/OU/AU admin can manage admin accounts")
			return
		}
	}

	var admins []oasmodel.OASUser
	// OAS-CONSOLE-08: 包含所有治理角色：OU/AU/SU/OAM
	h.DB.Where("role_code IN ?", []string{"OU", "AU", "SU", "OAM"}).Find(&admins)
	response.OK(c, admins)
}
