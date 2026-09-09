// users.go — 自 cmd/oas/main.go 下沉（OAS-CONSOLE-09 A1），行为零变化。
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

func (h *Handlers) DeleteUser(c *gin.Context) {
	// 白名单 B：删除用户仅 SU/OU/AU 可操作
	operatorUsername, _ := c.Get("username")
	if !authz.IsInAdminWhitelistB(h.DB, fmt.Sprintf("%v", operatorUsername)) {
		response.Forbidden(c, "only SU/OU/AU admin can delete users")
		return
	}
	id, _ := parseUint(c.Param("id"))
	var targetUser oasmodel.OASUser
	if h.DB.First(&targetUser, id).Error != nil {
		response.NotFound(c, "user not found")
		return
	}
	// 删除用户
	if err := h.DB.Delete(&targetUser).Error; err != nil {
		response.InternalError(c, "failed to delete user: "+err.Error())
		return
	}
	// 删除用户角色关联
	h.DB.Where("user_id = ?", id).Delete(&oasmodel.OASUserRole{})
	// 审计日志 - 记录操作者身份，resource_id 为被删用户 id
	var operator oasmodel.OASUser
	operatorUserCode := ""
	operatorDisplayName := ""
	if h.DB.Where("username = ?", operatorUsername).First(&operator).Error == nil {
		operatorUserCode = operator.UserCode
		operatorDisplayName = operator.DisplayName
	}
	h.DB.Create(&oasmodel.AuditLog{
		Plane:       "admin",
		Action:      "admin.account.delete",
		UserID:      operatorUserCode,
		UserName:    operatorDisplayName,
		Resource:    "user",
		ResourceID:  fmt.Sprintf("%d", id),
		Detail:      fmt.Sprintf("env=%s, deleted_user_id=%d, deleted_username=%s, deleted_role=%s", h.OASEnv.String(), id, targetUser.Username, targetUser.RoleCode),
		IP:          c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
		Environment: h.OASEnv.String(),
		Domain:      operator.Domain,
	})
	response.OK(c, gin.H{"id": id, "username": targetUser.Username})
}

func (h *Handlers) UpdateUserStatus(c *gin.Context) {
	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "status required")
		return
	}
	if req.Status != "active" && req.Status != "disabled" {
		response.BadRequest(c, "status must be active or disabled")
		return
	}
	id, _ := parseUint(c.Param("id"))
	// 白名单 B：修改 admin 账号状态仅 OU/AU admin 可操作
	var targetUser oasmodel.OASUser
	h.DB.First(&targetUser, id)
	operatorUsername, _ := c.Get("username")
	if authz.IsAdminAccount(targetUser.Username) && !authz.CanOperateAdminAccount(h.DB, fmt.Sprintf("%v", operatorUsername)) {
		response.Forbidden(c, "only oas-ou-admin and oas-au-admin can modify admin account status")
		return
	}
	h.DB.Model(&oasmodel.OASUser{}).Where("id = ?", id).Update("status", req.Status)
	h.DB.Create(&oasmodel.AuditLog{
		Plane:       "admin",
		Action:      "user.update_status",
		Resource:    "user",
		Detail:      fmt.Sprintf("env=%s, user_id=%d, status=%s", h.OASEnv.String(), id, req.Status),
		IP:          c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
		Environment: h.OASEnv.String(),
	})
	response.OK(c, nil)
}

func (h *Handlers) UpdateUserRoles(c *gin.Context) {
	var req struct {
		Roles []string `json:"roles" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "roles required")
		return
	}
	id, _ := parseUint(c.Param("id"))
	// 白名单 B：修改 admin 账号角色仅 OU/AU admin 可操作
	var targetUser oasmodel.OASUser
	h.DB.First(&targetUser, id)
	operatorUsername, _ := c.Get("username")
	if authz.IsAdminAccount(targetUser.Username) && !authz.CanOperateAdminAccount(h.DB, fmt.Sprintf("%v", operatorUsername)) {
		response.Forbidden(c, "only oas-ou-admin and oas-au-admin can modify admin account roles")
		return
	}
	h.DB.Where("user_id = ?", id).Delete(&oasmodel.OASUserRole{})
	for _, rc := range req.Roles {
		var role oasmodel.OASRole
		if h.DB.Where("role_code = ?", rc).First(&role).Error == nil {
			h.DB.Table("user_roles").Create(&oasmodel.OASUserRole{
				UserID:    id,
				RoleID:    role.ID,
				GrantedBy: "admin",
				GrantedAt: time.Now(),
			})
		}
	}
	h.DB.Create(&oasmodel.AuditLog{
		Plane:       "admin",
		Action:      "user.update_roles",
		Resource:    "user",
		Detail:      fmt.Sprintf("env=%s, user_id=%d, roles=%v", h.OASEnv.String(), id, req.Roles),
		IP:          c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
		Environment: h.OASEnv.String(),
	})
	response.OK(c, nil)
}

func (h *Handlers) CreateUser(c *gin.Context) {
	var req struct {
		Username    string   `json:"username" binding:"required"`
		Password    string   `json:"password" binding:"required"`
		DisplayName string   `json:"display_name"`
		RoleCode    string   `json:"role_code" binding:"required"`
		Roles       []string `json:"roles"`
		Domain      string   `json:"domain"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "username, password, role_code required")
		return
	}
	// 白名单 B：创建 admin 账号仅 OU/AU admin 可操作
	operatorUsername, _ := c.Get("username")
	if authz.IsAdminAccount(req.Username) && !authz.CanOperateAdminAccount(h.DB, fmt.Sprintf("%v", operatorUsername)) {
		response.Forbidden(c, "only oas-ou-admin and oas-au-admin can create admin accounts")
		return
	}
	var existing oasmodel.OASUser
	if h.DB.Where("username = ?", req.Username).First(&existing).Error == nil {
		response.BadRequest(c, "username already exists")
		return
	}
	// 校验 role_code 是否存在
	var roleCheck oasmodel.OASRole
	if h.DB.Where("role_code = ?", req.RoleCode).First(&roleCheck).Error != nil {
		response.BadRequest(c, "role_code not found: "+req.RoleCode)
		return
	}
	// OAS-CONSOLE-X1: 密码强度校验（≥8 位 + 四类字符至少三类），与 admin-accounts/ams 同口径
	if err := password.ValidateStrength(req.Password); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	hash, err := password.Hash(req.Password)
	if err != nil {
		response.InternalError(c, "failed to hash password")
		return
	}
	displayName := req.DisplayName
	if displayName == "" {
		displayName = req.Username
	}
	userCode := fmt.Sprintf("XHPZ#%s-%d", req.RoleCode, time.Now().UnixNano()%100000)
	user := oasmodel.OASUser{
		UserCode:     userCode,
		Username:     req.Username,
		PasswordHash: string(hash),
		DisplayName:  displayName,
		IdentityType: req.RoleCode,
		EntityType:   "H",
		Status:       "active",
		Domain:       req.Domain,
	}
	if err := h.DB.Create(&user).Error; err != nil {
		response.BadRequest(c, "create user failed: "+err.Error())
		return
	}
	roleCodes := req.Roles
	if len(roleCodes) == 0 {
		roleCodes = []string{req.RoleCode}
	}
	for _, rc := range roleCodes {
		var role oasmodel.OASRole
		if h.DB.Where("role_code = ?", rc).First(&role).Error == nil {
			h.DB.Table("user_roles").Create(&oasmodel.OASUserRole{
				UserID:    user.ID,
				RoleID:    role.ID,
				GrantedBy: "admin",
				GrantedAt: time.Now(),
			})
		}
	}
	h.DB.Create(&oasmodel.AuditLog{
		Plane:       "admin",
		Action:      "user.create",
		UserID:      user.UserCode,
		UserName:    user.DisplayName,
		Resource:    "user",
		Detail:      fmt.Sprintf("env=%s, username=%s, roles=%v", h.OASEnv.String(), req.Username, roleCodes),
		IP:          c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
		Environment: h.OASEnv.String(),
		Domain:      user.Domain,
	})
	response.Created(c, gin.H{"id": user.ID, "username": user.Username, "user_code": user.UserCode})
}

func (h *Handlers) ListUsers(c *gin.Context) {
	// 白名单 B：用户管理仅 SU/OU/AU 可访问
	operatorUsername, _ := c.Get("username")
	if !authz.IsInAdminWhitelistB(h.DB, fmt.Sprintf("%v", operatorUsername)) {
		response.Forbidden(c, "only SU/OU/AU admin can manage users")
		return
	}
	type UserVO struct {
		ID          uint64   `json:"id"`
		UserCode    string   `json:"user_code"`
		Username    string   `json:"username"`
		DisplayName string   `json:"display_name"`
		Status      string   `json:"status"`
		Roles       []string `json:"roles"`
		LastLoginAt *string  `json:"last_login_at,omitempty"`
		CreatedAt   string   `json:"created_at"`
	}
	var users []oasmodel.OASUser
	h.DB.Order("created_at DESC").Find(&users)
	var result []UserVO
	for _, u := range users {
		var roles []string
		h.DB.Table("user_roles").
			Select("r.role_code").
			Joins("JOIN roles r ON r.id = user_roles.role_id").
			Where("user_roles.user_id = ?", u.ID).
			Pluck("r.role_code", &roles)
		vo := UserVO{
			ID:          u.ID,
			UserCode:    u.UserCode,
			Username:    u.Username,
			DisplayName: u.DisplayName,
			Status:      u.Status,
			Roles:       roles,
			CreatedAt:   u.CreatedAt.Format(time.RFC3339),
		}
		if u.LastLoginAt != nil {
			s := u.LastLoginAt.Format(time.RFC3339)
			vo.LastLoginAt = &s
		}
		result = append(result, vo)
	}
	response.OK(c, gin.H{"items": result, "total": len(result)})
}
