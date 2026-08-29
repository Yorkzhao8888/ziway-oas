package model

import (
	"time"

	"gorm.io/gorm"
)

// Base 所有实体的基础模型
type Base struct {
	ID        uint64         `gorm:"primarykey" json:"-"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// User 统一用户模型（各服务可引用或扩展）
type User struct {
	UserID       string `gorm:"uniqueIndex;size:32" json:"user_id"`
	Phone        string `gorm:"index;size:20" json:"phone,omitempty"`
	Email        string `gorm:"index;size:128" json:"email,omitempty"`
	PasswordHash string `gorm:"size:256" json:"-"`
	Nickname     string `gorm:"size:64" json:"nickname"`
	AvatarURL    string `gorm:"size:512" json:"avatar_url,omitempty"`
	Status       int    `gorm:"default:1" json:"status"` // 1=active, 0=disabled
	Base
}

// UserRole 用户-角色关联（戴帽子）
type UserRole struct {
	UserID     string `gorm:"index;size:32" json:"user_id"`
	Domain     string `gorm:"index;size:32" json:"domain"`
	RoleCode   string `gorm:"size:16" json:"role_code"`
	ActiveRole bool   `gorm:"default:false" json:"active_role"`
	Base
}

// Role 角色定义
type Role struct {
	Code        string `gorm:"uniqueIndex;size:16" json:"code"`
	Name        string `gorm:"size:64" json:"name"`
	Domain      string `gorm:"index;size:32" json:"domain"`
	Description string `gorm:"size:256" json:"description,omitempty"`
	Base
}

// AuditLog 审计日志（所有服务共用）
type AuditLog struct {
	ID         uint64    `gorm:"primarykey" json:"id"`
	Service    string    `gorm:"index;size:32" json:"service"`
	Action     string    `gorm:"size:64" json:"action"`
	UserID     string    `gorm:"index;size:32" json:"user_id"`
	TargetType string    `gorm:"size:32" json:"target_type,omitempty"`
	TargetID   string    `gorm:"size:32" json:"target_id,omitempty"`
	Detail     string    `gorm:"type:text" json:"detail,omitempty"`
	IP         string    `gorm:"size:64" json:"ip,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}
