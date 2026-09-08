// Package authz 承载 OAS 权限判断函数（自 cmd/oas/main.go 下沉，OAS-CONSOLE-09 A1）。
// 依赖注入原则：查库函数显式接收 *gorm.DB；AdminUsernames 为包级静态查找表
// （唯一包级共享状态，因 oas→authz 单向依赖禁止反向导入，落位本包而非 state.go）。
package authz

import (
	"strings"

	"gorm.io/gorm"
	oasmodel "ziway/backend/internal/oas/model"
)

// AdminUsernames are the highest-privilege accounts.
// Whitelist B: only oas-ou-admin and oas-au-admin can operate on these accounts.
var AdminUsernames = map[string]bool{
	"oas-ou-admin":  true,
	"oas-au-admin":  true,
	"oas-oam-admin": true,
}

func IsAdminAccount(username string) bool {
	return AdminUsernames[username]
}

// CanOperateAdminAccount checks if the operator can manage admin accounts (Whitelist B).
func CanOperateAdminAccount(db *gorm.DB, operatorUsername string) bool {
	// Check if username is an API key
	if strings.HasPrefix(operatorUsername, "api-key:") {
		return true // API keys have admin-level access
	}

	// Query user's role_code from database
	var user oasmodel.OASUser
	if err := db.Where("username = ?", operatorUsername).First(&user).Error; err != nil {
		return false
	}

	// SU/OU/AU are admin-level roles
	return user.RoleCode == "SU" || user.RoleCode == "OU" || user.RoleCode == "AU"
}

// IsInAdminWhitelistB checks if the user is in Whitelist B (audit log access).
func IsInAdminWhitelistB(db *gorm.DB, username string) bool {
	return CanOperateAdminAccount(db, username)
}

// IsInAdminWhitelistA checks if the user is in Whitelist A (system management access).
func IsInAdminWhitelistA(db *gorm.DB, username string) bool {
	// Check if username is an API key
	if strings.HasPrefix(username, "api-key:") {
		return true // API keys have admin-level access
	}

	// Query user's role_code from database
	var user oasmodel.OASUser
	if err := db.Where("username = ?", username).First(&user).Error; err != nil {
		return false
	}

	// SU/OU/AU/OAM are admin-level roles for whitelist A
	return user.RoleCode == "SU" || user.RoleCode == "OU" || user.RoleCode == "AU" || user.RoleCode == "OAM"
}

// CanAccessOrgManagement checks if a user can access organization management.
// Allowed: OU/AU/OAM admins (whitelist A) + XAM roles (TAM/HAM/YAM/VAM).
func CanAccessOrgManagement(db *gorm.DB, username string, roles []string) bool {
	// OAS-CONSOLE-08: 仅白名单 A 用户可访问组织管理（移除 XAM 角色）
	// Whitelist A users always allowed (SU/OU/AU/OAM)
	if IsInAdminWhitelistA(db, username) {
		return true
	}
	// XAM roles no longer allowed (frozen in OAS-CONSOLE-08)
	// for _, role := range roles {
	// 	if role == "TAM" || role == "HAM" || role == "YAM" || role == "VAM" {
	// 		return true
	// 	}
	// }
	return false
}
