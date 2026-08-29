package handlers

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"ziway.ams/internal/model"
	"ziway.ams/internal/pkg"
	"ziway.ams/internal/service"
)

// AuthHandler 认证HTTP处理器
type AuthHandler struct {
	authSvc *service.AuthService
	logger  *zap.Logger
}

func NewAuthHandler(authSvc *service.AuthService, logger *zap.Logger) *AuthHandler {
	return &AuthHandler{authSvc: authSvc, logger: logger}
}

// Login POST /api/v1/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	resp, err := h.authSvc.Login(c.Request.Context(), &req)
	if err != nil {
		h.logger.Warn("login failed",
			zap.String("account", req.Account),
			zap.Error(err),
		)
		pkg.Unauthorized(c, "login failed: "+err.Error())
		return
	}

	pkg.OK(c, resp)
}

// Register POST /api/v1/auth/register
func (h *AuthHandler) Register(c *gin.Context) {
	var req struct {
		Phone    string `json:"phone" binding:"required"`
		Email    string `json:"email"`
		Password string `json:"password" binding:"required,min=6"`
		Nickname string `json:"nickname" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	user, err := h.authSvc.Register(c.Request.Context(), req.Phone, req.Email, req.Password, req.Nickname)
	if err != nil {
		pkg.BadRequest(c, err.Error())
		return
	}

	pkg.Created(c, gin.H{
		"user_id":  user.UserID,
		"nickname": user.Nickname,
		"phone":    user.Phone,
	})
}

// Me GET /api/v1/auth/me
func (h *AuthHandler) Me(c *gin.Context) {
	resp := gin.H{
		"user_id":       c.GetString("user_id"),
		"identity_type": c.GetString("identity_type"),
		"roles":         c.GetStringSlice("roles"),
		"active_role":   c.GetString("active_role"),
		"domain":        c.GetString("domain"),
	}
	if v, ok := c.Get("agent_service"); ok {
		resp["agent_service"] = v
	}
	if v, ok := c.Get("delegated_by"); ok {
		resp["delegated_by"] = v
	}
	pkg.OK(c, resp)
}

// SwitchHat POST /api/v1/auth/switch-hat
// Body: {"domain":"mall","role_code":"CX"}
func (h *AuthHandler) SwitchHat(c *gin.Context) {
	userID := c.GetString("user_id")
	var req model.SwitchHatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.BadRequest(c, "domain is required")
		return
	}

	resp, err := h.authSvc.SwitchHat(c.Request.Context(), userID, req.Domain, req.RoleCode)
	if err != nil {
		pkg.BadRequest(c, err.Error())
		return
	}

	pkg.OK(c, resp)
}

// AgentToken POST /api/v1/auth/agent-token
// 供ziway-Agent等服务调用，签发NHI Token（需Service Token认证）
// Body: {"agent_service":"ziway-Agent","delegated_by":"CU-PZ#202408240001"}
func (h *AuthHandler) AgentToken(c *gin.Context) {
	var req struct {
		AgentService string `json:"agent_service" binding:"required"`
		DelegatedBy  string `json:"delegated_by" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.BadRequest(c, "agent_service and delegated_by are required")
		return
	}

	resp, err := h.authSvc.IssueNHIToken(c.Request.Context(), req.AgentService, req.DelegatedBy)
	if err != nil {
		h.logger.Warn("NHI token issue failed",
			zap.String("agent", req.AgentService),
			zap.String("delegated_by", req.DelegatedBy),
			zap.Error(err),
		)
		pkg.BadRequest(c, err.Error())
		return
	}

	pkg.OK(c, resp)
}

// Logout POST /api/v1/auth/logout
func (h *AuthHandler) Logout(c *gin.Context) {
	tokenJTI := c.GetString("token_jti")
	if tokenJTI != "" {
		_ = h.authSvc.Logout(c.Request.Context(), tokenJTI)
	}
	pkg.OK(c, gin.H{"message": "logged out"})
}
