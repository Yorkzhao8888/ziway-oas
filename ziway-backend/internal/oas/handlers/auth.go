// auth.go — 自 cmd/oas/main.go 下沉（OAS-CONSOLE-09 A1），行为零变化。
package handlers

import (
	"encoding/base64"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	oasmodel "ziway/backend/internal/oas/model"
	"ziway/backend/pkg/jwt"
	"ziway/backend/pkg/password"
	"ziway/backend/pkg/response"
)

func (h *Handlers) AuthRoles(c *gin.Context) {
	var roles []oasmodel.OASRole
	h.DB.Order("role_code").Find(&roles)
	type RoleVO struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	var result []RoleVO
	for _, r := range roles {
		result = append(result, RoleVO{Code: r.RoleCode, Name: r.Name})
	}
	response.OK(c, result)
}

func (h *Handlers) Login(c *gin.Context) {
	if h.JWTIssuer == nil {
		response.InternalError(c, "jwt issuer not configured")
		return
	}
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "username and password required")
		return
	}
	// Rate limit check
	clientIP := c.ClientIP()
	if !h.LoginLimiter.Check(clientIP, req.Username) {
		lockout := h.LoginLimiter.LockoutRemaining(clientIP, req.Username)
		response.TooManyRequests(c, "too many failed attempts, try again in "+lockout.Round(time.Second).String())
		return
	}
	var user oasmodel.OASUser
	if err := h.DB.Where("username = ?", req.Username).First(&user).Error; err != nil {
		h.LoginLimiter.RecordFailure(clientIP, req.Username)
		response.Unauthorized(c, "invalid credentials")
		return
	}
	if err := password.Verify(req.Password, user.PasswordHash); err != nil {
		h.LoginLimiter.RecordFailure(clientIP, req.Username)
		response.Unauthorized(c, "invalid credentials")
		return
	}
	if user.Status != "active" {
		response.Forbidden(c, "account disabled")
		return
	}
	h.LoginLimiter.RecordSuccess(clientIP, req.Username)
	now := time.Now()
	h.DB.Model(&user).Update("last_login_at", &now)
	var roles []string
	// 优先使用 users.role_code 作为 JWT role 的唯一来源
	if user.RoleCode != "" {
		roles = []string{user.RoleCode}
	} else {
		// 如果 users.role_code 为空，才查询 user_roles 表
		h.DB.Table("user_roles").
			Select("r.role_code").
			Joins("JOIN roles r ON r.id = user_roles.role_id").
			Where("user_roles.user_id = ?", user.ID).
			Pluck("r.role_code", &roles)
	}
	activeRole := ""
	if len(roles) > 0 {
		activeRole = roles[0]
	}
	claims := &jwt.Claims{
		UserID:       user.UserCode,
		IdentityID:   user.UserCode,
		IdentityType: map[string]string{"H": "human", "N": "nhi"}[user.EntityType],
		Username:     user.Username,
		Role:         activeRole,
		SubRole:      "",
		NHIFlag:      user.EntityType == "N",
		MSAccess:     []string{"ams", "cms", "dms", "hms", "fms", "tms", "ems", "gms", "oms", "vms", "ims", "sms"},
		Roles:        roles,
		ActiveRole:   activeRole,
		Domain:       user.Domain,
		TokenID:      fmt.Sprintf("login-%d-%d", user.ID, now.Unix()),
	}
	token, ttl, err := h.JWTIssuer.IssueAccessToken(claims)
	if err != nil {
		response.InternalError(c, "failed to issue token")
		return
	}
	h.DB.Create(&oasmodel.AuditLog{
		UserID:      user.UserCode,
		UserName:    user.DisplayName,
		Plane:       "admin",
		Action:      "auth.login",
		Resource:    "jwt",
		Detail:      fmt.Sprintf("env=%s, result=success, role=%s, token_id=%s", h.OASEnv.String(), activeRole, claims.TokenID),
		IP:          c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
		Environment: h.OASEnv.String(),
		Domain:      user.Domain,
	})
	response.OK(c, gin.H{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   ttl,
		"identity_id":  user.UserCode,
		"role":         activeRole,
		"username":     user.Username,
	})
}

func (h *Handlers) Userinfo(c *gin.Context) {
	userIDVal, exists := c.Get("user_id")
	if !exists {
		response.InternalError(c, "user_id not found in context")
		return
	}
	userID, ok := userIDVal.(string)
	if !ok || userID == "" {
		response.InternalError(c, "invalid user_id in context")
		return
	}
	var user oasmodel.OASUser
	if err := h.DB.Where("user_code = ?", userID).First(&user).Error; err != nil {
		response.NotFound(c, "user not found")
		return
	}

	response.OK(c, gin.H{
		"sub":      user.UserCode,
		"name":     user.DisplayName,
		"email":    user.Username + "@ziway.eco", // Synthetic email
		"role":     user.RoleCode,
		"domain":   user.Domain,
		"username": user.Username,
	})
}

func (h *Handlers) OAuthToken(c *gin.Context) {
	if h.JWTIssuer == nil {
		response.InternalError(c, "jwt issuer not configured")
		return
	}

	grantType := c.PostForm("grant_type")
	if grantType != "authorization_code" {
		response.BadRequest(c, "unsupported grant_type")
		return
	}

	code := c.PostForm("code")
	clientID := c.PostForm("client_id")
	clientSecret := c.PostForm("client_secret")
	redirectURI := c.PostForm("redirect_uri")

	// Validate client
	var client oasmodel.OAuthClient
	if err := h.DB.Where("client_id = ? AND status = ?", clientID, "active").First(&client).Error; err != nil {
		response.Unauthorized(c, "invalid client")
		return
	}

	// Verify client secret
	if err := password.Verify(clientSecret, client.ClientSecret); err != nil {
		response.Unauthorized(c, "invalid client_secret")
		return
	}

	// Validate authorization code
	var authCode oasmodel.OAuthAuthorizationCode
	if err := h.DB.Where("code = ? AND client_id = ? AND used = ?", code, clientID, false).First(&authCode).Error; err != nil {
		response.BadRequest(c, "invalid or expired code")
		return
	}

	// Check expiration
	if time.Now().After(authCode.ExpiresAt) {
		response.BadRequest(c, "code expired")
		return
	}

	// Validate redirect_uri matches
	if authCode.RedirectURI != redirectURI {
		response.BadRequest(c, "redirect_uri mismatch")
		return
	}

	// Mark code as used
	h.DB.Model(&authCode).Update("used", true)

	// Get user
	var user oasmodel.OASUser
	if err := h.DB.Where("user_code = ?", authCode.UserID).First(&user).Error; err != nil {
		response.InternalError(c, "user not found")
		return
	}

	// Issue tokens
	var roles []string
	if user.RoleCode != "" {
		roles = []string{user.RoleCode}
	} else {
		h.DB.Table("user_roles").
			Select("r.role_code").
			Joins("JOIN roles r ON r.id = user_roles.role_id").
			Where("user_roles.user_id = ?", user.ID).
			Pluck("r.role_code", &roles)
	}
	activeRole := ""
	if len(roles) > 0 {
		activeRole = roles[0]
	}

	claims := &jwt.Claims{
		UserID:       user.UserCode,
		IdentityID:   user.UserCode,
		IdentityType: map[string]string{"H": "human", "N": "nhi"}[user.EntityType],
		Username:     user.Username,
		Role:         activeRole,
		SubRole:      "",
		NHIFlag:      user.EntityType == "N",
		MSAccess:     []string{"ams", "cms", "dms", "hms", "fms", "tms", "ems", "gms", "oms", "vms", "ims", "sms"},
		Roles:        roles,
		ActiveRole:   activeRole,
		Domain:       user.Domain,
		TokenID:      fmt.Sprintf("oauth-%d-%d", user.ID, time.Now().Unix()),
	}

	accessToken, accessTTL, err := h.JWTIssuer.IssueAccessToken(claims)
	if err != nil {
		response.InternalError(c, "failed to issue access token")
		return
	}

	refreshToken, err := h.JWTIssuer.IssueRefreshToken(user.UserCode, map[string]string{"H": "human", "N": "nhi"}[user.EntityType], claims.TokenID)
	if err != nil {
		response.InternalError(c, "failed to issue refresh token")
		return
	}

	// Audit log
	h.DB.Create(&oasmodel.AuditLog{
		UserID:      user.UserCode,
		UserName:    user.DisplayName,
		Plane:       "admin",
		Action:      "oauth.token",
		Resource:    "jwt",
		Detail:      fmt.Sprintf("env=%s, client=%s, grant=authorization_code, role=%s", h.OASEnv.String(), clientID, activeRole),
		IP:          c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
		Environment: h.OASEnv.String(),
		Domain:      user.Domain,
	})

	response.OK(c, gin.H{
		"access_token":  accessToken,
		"token_type":    "Bearer",
		"expires_in":    accessTTL,
		"refresh_token": refreshToken,
		"id_token":      accessToken, // Simplified: use access_token as id_token
		"scope":         authCode.Scopes,
	})
}

func (h *Handlers) OAuthAuthorize(c *gin.Context) {
	clientID := c.Query("client_id")
	redirectURI := c.Query("redirect_uri")
	responseType := c.Query("response_type")
	scope := c.Query("scope")
	state := c.Query("state")

	if responseType != "code" {
		response.BadRequest(c, "unsupported response_type, only 'code' is supported")
		return
	}

	// Validate client
	var client oasmodel.OAuthClient
	if err := h.DB.Where("client_id = ? AND status = ?", clientID, "active").First(&client).Error; err != nil {
		response.BadRequest(c, "invalid client_id")
		return
	}

	// Validate redirect_uri
	allowedURIs := strings.Split(client.RedirectURI, ",")
	uriValid := false
	for _, uri := range allowedURIs {
		if strings.TrimSpace(uri) == redirectURI {
			uriValid = true
			break
		}
	}
	if !uriValid {
		response.BadRequest(c, "invalid redirect_uri")
		return
	}

	// Check if user is already logged in (via session or cookie)
	// For now, redirect to login page with OAuth context
	loginURL := fmt.Sprintf("/login?oauth=1&client_id=%s&redirect_uri=%s&response_type=%s&scope=%s&state=%s",
		clientID, redirectURI, responseType, scope, state)
	c.Redirect(302, loginURL)
}

func (h *Handlers) OAuthJWKS(c *gin.Context) {
	if h.JWTPublicKey == nil {
		response.InternalError(c, "public key not configured")
		return
	}
	// Convert RSA public key to JWK format
	keyJSON := gin.H{
		"kty": "RSA",
		"use": "sig",
		"alg": "RS256",
		"kid": "oas-rs256-key",
		"n":   base64.RawURLEncoding.EncodeToString(h.JWTPublicKey.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(h.JWTPublicKey.E)).Bytes()),
	}
	response.OK(c, gin.H{"keys": []gin.H{keyJSON}})
}

func (h *Handlers) OpenIDConfiguration(c *gin.Context) {
	issuer := h.Cfg.GetString("jwt.issuer")
	if issuer == "" {
		issuer = "https://oas.ziway.eco"
	}
	baseURL := c.Request.Host
	if !strings.HasPrefix(baseURL, "http") {
		scheme := "https"
		if strings.Contains(baseURL, "localhost") || strings.Contains(baseURL, "127.0.0.1") {
			scheme = "http"
		}
		baseURL = scheme + "://" + baseURL
	}
	response.OK(c, gin.H{
		"issuer":                                issuer,
		"authorization_endpoint":                baseURL + "/oauth/authorize",
		"token_endpoint":                        baseURL + "/oauth/token",
		"userinfo_endpoint":                     baseURL + "/oauth/userinfo",
		"jwks_uri":                              baseURL + "/oauth/jwks",
		"scopes_supported":                      []string{"openid", "profile", "email"},
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_post", "client_secret_basic"},
		"claims_supported":                      []string{"sub", "iss", "aud", "exp", "iat", "name", "email", "role"},
	})
}

func (h *Handlers) AuthorizeCode(c *gin.Context) {
	var req struct {
		ClientID    string `json:"client_id" binding:"required"`
		RedirectURI string `json:"redirect_uri" binding:"required"`
		State       string `json:"state"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "client_id and redirect_uri required")
		return
	}

	// Validate client
	var client oasmodel.OAuthClient
	if err := h.DB.Where("client_id = ? AND status = ?", req.ClientID, "active").First(&client).Error; err != nil {
		response.BadRequest(c, "invalid client_id")
		return
	}

	// Validate redirect_uri
	allowedURIs := strings.Split(client.RedirectURI, ",")
	uriValid := false
	for _, uri := range allowedURIs {
		if strings.TrimSpace(uri) == req.RedirectURI {
			uriValid = true
			break
		}
	}
	if !uriValid {
		response.BadRequest(c, "invalid redirect_uri")
		return
	}

	// Get user from JWT
	userIDVal, exists := c.Get("user_id")
	if !exists {
		response.InternalError(c, "user_id not found in context")
		return
	}
	userID, ok := userIDVal.(string)
	if !ok || userID == "" {
		response.InternalError(c, "invalid user_id in context")
		return
	}
	var user oasmodel.OASUser
	if err := h.DB.Where("user_code = ?", userID).First(&user).Error; err != nil {
		response.InternalError(c, "user not found")
		return
	}

	// Generate authorization code
	code := fmt.Sprintf("auth_%d_%d", user.ID, time.Now().UnixNano())
	authCode := oasmodel.OAuthAuthorizationCode{
		Code:        code,
		ClientID:    req.ClientID,
		UserID:      user.UserCode,
		RedirectURI: req.RedirectURI,
		Scopes:      client.Scopes,
		ExpiresAt:   time.Now().Add(5 * time.Minute),
		Used:        false,
	}
	if err := h.DB.Create(&authCode).Error; err != nil {
		response.InternalError(c, "failed to create authorization code")
		return
	}

	// Audit log
	h.DB.Create(&oasmodel.AuditLog{
		UserID:      user.UserCode,
		UserName:    user.DisplayName,
		Plane:       "admin",
		Action:      "oauth.authorize",
		Resource:    "code",
		Detail:      fmt.Sprintf("env=%s, client=%s, redirect=%s, state=%s", h.OASEnv.String(), req.ClientID, req.RedirectURI, req.State),
		IP:          c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
		Environment: h.OASEnv.String(),
		Domain:      user.Domain,
	})

	response.OK(c, gin.H{
		"code": code,
	})
}

func (h *Handlers) DevToken(c *gin.Context) {
	if h.JWTIssuer == nil {
		response.InternalError(c, "jwt issuer not configured")
		return
	}
	var req struct {
		Username       string `json:"username"`
		Role           string `json:"role"`
		ExpiresMinutes int    `json:"expires_minutes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || (req.Role == "" && req.Username == "") {
		response.BadRequest(c, "role or username required")
		return
	}

	// Default 30 minutes, max 60 minutes
	if req.ExpiresMinutes <= 0 {
		req.ExpiresMinutes = 30
	}
	if req.ExpiresMinutes > 60 {
		req.ExpiresMinutes = 60
	}

	// Find user by role or username
	var user oasmodel.OASUser
	if req.Username != "" {
		if err := h.DB.Where("username = ?", req.Username).First(&user).Error; err != nil {
			response.NotFound(c, "user not found")
			return
		}
	} else {
		// Find a user with the specified role
		var userID uint64
		if err := h.DB.Table("user_roles").
			Select("user_roles.user_id").
			Joins("JOIN roles r ON r.id = user_roles.role_id").
			Where("r.role_code = ?", req.Role).
			Pluck("user_roles.user_id", &userID).Error; err != nil || userID == 0 {
			response.NotFound(c, "no user found for role: "+req.Role)
			return
		}
		if err := h.DB.First(&user, userID).Error; err != nil {
			response.NotFound(c, "user not found")
			return
		}
	}

	// Get user roles
	var roles []string
	h.DB.Table("user_roles").
		Select("r.role_code").
		Joins("JOIN roles r ON r.id = user_roles.role_id").
		Where("user_roles.user_id = ?", user.ID).
		Pluck("r.role_code", &roles)
	activeRole := ""
	if len(roles) > 0 {
		activeRole = roles[0]
	}

	now := time.Now()
	ttl := time.Duration(req.ExpiresMinutes) * time.Minute
	claims := &jwt.Claims{
		UserID:       user.UserCode,
		IdentityID:   user.UserCode,
		IdentityType: map[string]string{"H": "human", "N": "nhi"}[user.EntityType],
		Username:     user.Username,
		Role:         activeRole,
		SubRole:      "",
		NHIFlag:      user.EntityType == "N",
		MSAccess:     []string{"ams", "cms", "dms", "hms", "fms", "tms", "ems", "gms", "oms", "vms", "ims", "sms"},
		Roles:        roles,
		ActiveRole:   activeRole,
		TokenID:      fmt.Sprintf("dev-%d-%d", user.ID, now.Unix()),
	}
	token, _, err := h.JWTIssuer.IssueAccessTokenWithTTL(claims, ttl)
	if err != nil {
		response.InternalError(c, "failed to issue token")
		return
	}

	expiresAt := now.Add(ttl)

	// Audit log
	h.DB.Create(&oasmodel.AuditLog{
		UserID:      user.UserCode,
		UserName:    user.DisplayName,
		Plane:       "admin",
		Action:      "auth.dev-token",
		Resource:    "jwt",
		Detail:      fmt.Sprintf("env=%s, mode=dev-token, role=%s, expires=%dm, ip=%s, token_id=%s", h.OASEnv.String(), activeRole, req.ExpiresMinutes, c.ClientIP(), claims.TokenID),
		IP:          c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
		Environment: h.OASEnv.String(),
	})

	response.OK(c, gin.H{
		"token":      token,
		"expires_at": expiresAt.Format(time.RFC3339),
		"username":   user.Username,
		"role":       activeRole,
		"user_code":  user.UserCode,
	})
}

func (h *Handlers) TestAccounts(c *gin.Context) {
	type TestAccount struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Role        string `json:"role"`
		Password    string `json:"password"`
	}
	accounts := []TestAccount{
		{Username: "admin", DisplayName: "系统管理员", Role: "SU", Password: "test123"},
		{Username: "operator", DisplayName: "运营人员", Role: "AU", Password: "test123"},
		{Username: "customer", DisplayName: "客户用户", Role: "CU", Password: "test123"},
		{Username: "viewer", DisplayName: "访客", Role: "GU", Password: "test123"},
		{Username: "em", DisplayName: "供给运营长", Role: "EM", Password: "test123"},
	}
	response.OK(c, gin.H{
		"edition":  "beta",
		"accounts": accounts,
	})
}

func (h *Handlers) QuickLogin(c *gin.Context) {
	if h.JWTIssuer == nil {
		response.InternalError(c, "jwt issuer not configured")
		return
	}
	var req struct {
		Role     string `json:"role"`
		Username string `json:"username"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || (req.Role == "" && req.Username == "") {
		response.BadRequest(c, "role or username required")
		return
	}

	// Find user by role or username
	var user oasmodel.OASUser
	if req.Username != "" {
		if err := h.DB.Where("username = ?", req.Username).First(&user).Error; err != nil {
			response.NotFound(c, "test user not found")
			return
		}
	} else {
		// Find a test user with the specified role
		var userID uint64
		if err := h.DB.Table("user_roles").
			Select("user_roles.user_id").
			Joins("JOIN roles r ON r.id = user_roles.role_id").
			Where("r.role_code = ? AND user_roles.granted_by = ?", req.Role, "system-seed").
			Pluck("user_roles.user_id", &userID).Error; err != nil || userID == 0 {
			response.NotFound(c, "no test user found for role: "+req.Role)
			return
		}
		if err := h.DB.First(&user, userID).Error; err != nil {
			response.NotFound(c, "test user not found")
			return
		}
	}

	// Get user roles
	var roles []string
	// 优先使用 users.role_code 作为 JWT role 的唯一来源
	if user.RoleCode != "" {
		roles = []string{user.RoleCode}
	} else {
		// 如果 users.role_code 为空，才查询 user_roles 表
		h.DB.Table("user_roles").
			Select("r.role_code").
			Joins("JOIN roles r ON r.id = user_roles.role_id").
			Where("user_roles.user_id = ?", user.ID).
			Pluck("r.role_code", &roles)
	}
	activeRole := ""
	if len(roles) > 0 {
		activeRole = roles[0]
	}

	now := time.Now()
	claims := &jwt.Claims{
		UserID:       user.UserCode,
		IdentityID:   user.UserCode,
		IdentityType: map[string]string{"H": "human", "N": "nhi"}[user.EntityType],
		Username:     user.Username,
		Role:         activeRole,
		SubRole:      "",
		NHIFlag:      user.EntityType == "N",
		MSAccess:     []string{"ams", "cms", "dms", "hms", "fms", "tms", "ems", "gms", "oms", "vms", "ims", "sms"},
		Roles:        roles,
		ActiveRole:   activeRole,
		TokenID:      fmt.Sprintf("quick-%d-%d", user.ID, now.Unix()),
	}
	token, ttl, err := h.JWTIssuer.IssueAccessToken(claims)
	if err != nil {
		response.InternalError(c, "failed to issue token")
		return
	}

	// Audit log
	h.DB.Create(&oasmodel.AuditLog{
		UserID:      user.UserCode,
		UserName:    user.DisplayName,
		Plane:       "admin",
		Action:      "auth.quick-login",
		Resource:    "jwt",
		Detail:      fmt.Sprintf("env=%s, role=%s, token_id=%s", h.OASEnv.String(), activeRole, claims.TokenID),
		IP:          c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
		Environment: h.OASEnv.String(),
		Domain:      user.Domain,
	})

	response.OK(c, gin.H{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   ttl,
		"identity_id":  user.UserCode,
		"role":         activeRole,
		"sub_role":     "",
		"nhi_flag":     user.EntityType == "N",
		"edition":      "beta",
		"quick_login":  true,
	})
}

func (h *Handlers) ProxyAMSLogin(c *gin.Context) {
	if h.JWTIssuer == nil {
		response.InternalError(c, "jwt issuer not configured")
		return
	}
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Unauthorized(c, "username and password required")
		return
	}
	// Rate limit check
	clientIP := c.ClientIP()
	if !h.LoginLimiter.Check(clientIP, req.Username) {
		lockout := h.LoginLimiter.LockoutRemaining(clientIP, req.Username)
		response.TooManyRequests(c, "too many failed attempts, try again in "+lockout.Round(time.Second).String())
		return
	}
	// Look up user from shared users table
	var user oasmodel.OASUser
	if err := h.DB.Where("username = ?", req.Username).First(&user).Error; err != nil {
		h.LoginLimiter.RecordFailure(clientIP, req.Username)
		response.Unauthorized(c, "invalid credentials")
		return
	}
	if err := password.Verify(req.Password, user.PasswordHash); err != nil {
		h.LoginLimiter.RecordFailure(clientIP, req.Username)
		response.Unauthorized(c, "invalid credentials")
		return
	}
	if user.Status != "active" {
		response.Forbidden(c, "account disabled")
		return
	}
	h.LoginLimiter.RecordSuccess(clientIP, req.Username)
	// Update last_login_at
	now := time.Now()
	h.DB.Model(&user).Update("last_login_at", &now)
	// Get user roles
	var roles []string
	// 优先使用 users.role_code 作为 JWT role 的唯一来源
	if user.RoleCode != "" {
		roles = []string{user.RoleCode}
	} else {
		// 如果 users.role_code 为空，才查询 user_roles 表
		h.DB.Table("user_roles").
			Select("r.role_code").
			Joins("JOIN roles r ON r.id = user_roles.role_id").
			Where("user_roles.user_id = ?", user.ID).
			Pluck("r.role_code", &roles)
	}
	activeRole := ""
	if len(roles) > 0 {
		activeRole = roles[0]
	}
	nhiFlag := user.EntityType == "N"
	claims := &jwt.Claims{
		UserID:       user.UserCode,
		IdentityID:   user.UserCode,
		IdentityType: map[string]string{"H": "human", "N": "nhi"}[user.EntityType],
		Username:     user.Username,
		Role:         activeRole,
		SubRole:      "",
		NHIFlag:      nhiFlag,
		MSAccess:     []string{"ams", "cms", "dms", "hms", "fms", "tms", "ems", "gms", "oms", "vms", "ims", "sms"},
		Roles:        roles,
		ActiveRole:   activeRole,
		Domain:       user.Domain,
		TokenID:      fmt.Sprintf("tok-%d-%d", user.ID, now.Unix()),
	}
	token, ttl, err := h.JWTIssuer.IssueAccessToken(claims)
	if err != nil {
		response.InternalError(c, "failed to issue token")
		return
	}
	// Audit log: token issued
	h.DB.Create(&oasmodel.AuditLog{
		UserID:      user.UserCode,
		UserName:    user.DisplayName,
		Plane:       "admin",
		Action:      "auth.login",
		Resource:    "jwt",
		Detail:      fmt.Sprintf("env=%s, result=success, role=%s, token_id=%s", h.OASEnv.String(), activeRole, claims.TokenID),
		IP:          c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
		Environment: h.OASEnv.String(),
		Domain:      user.Domain,
	})
	response.OK(c, gin.H{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   ttl,
		"identity_id":  user.UserCode,
		"role":         activeRole,
		"sub_role":     "",
		"nhi_flag":     nhiFlag,
	})
}

func (h *Handlers) WellKnownJWKS(c *gin.Context) {
	if h.JWTPublicKey == nil {
		c.JSON(500, gin.H{"error": "public key not available"})
		return
	}
	// Convert RSA public key to JWK format
	nBytes := h.JWTPublicKey.N.Bytes()
	eBytes := big.NewInt(int64(h.JWTPublicKey.E)).Bytes()
	jwk := gin.H{
		"kty": "RSA",
		"use": "sig",
		"alg": "RS256",
		"kid": "oas-rsa-001",
		"n":   base64.RawURLEncoding.EncodeToString(nBytes),
		"e":   base64.RawURLEncoding.EncodeToString(eBytes),
	}
	c.JSON(200, gin.H{"keys": []gin.H{jwk}})
}

func (h *Handlers) PublicKey(c *gin.Context) {
	pubKeyPath := h.Cfg.GetString("jwt.public_key_path")
	if pubKeyPath == "" {
		response.InternalError(c, "public key not configured")
		return
	}
	pubData, err := os.ReadFile(pubKeyPath)
	if err != nil {
		response.InternalError(c, "failed to read public key")
		return
	}
	response.OK(c, gin.H{
		"algorithm":  "RS256",
		"key_type":   "RSA",
		"format":     "PEM",
		"public_key": string(pubData),
		"issuer":     h.Cfg.GetString("jwt.issuer"),
	})
}

func (h *Handlers) Health(c *gin.Context) {
	response.OK(c, gin.H{
		"status":  "ok",
		"service": "ziway-oas",
		"planes":  []string{"owner", "admin"},
	})
}
