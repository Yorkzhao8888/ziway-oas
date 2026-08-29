package model

import (
	"time"
)

// 身份类型
const (
	IdentityTypeHuman = "human" // 人类用户
	IdentityTypeNHI   = "nhi"   // Non-Human Identity（Agent运行时）
)

// 角色类型
const (
	RoleTypeBase = "base" // 12U基础角色
	RoleTypeHat  = "hat"  // 帽子角色（CX/FX）
	RoleTypeNHI  = "nhi"  // 非人类身份
)

// User 用户表 — 对应知味生态12U角色持有者，也可承载NHI
type User struct {
	ID           int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID       string    `gorm:"column:user_id;type:varchar(32);uniqueIndex;not null" json:"user_id"`             // X*PZ# 编号，如 CU-PZ#202408240001
	IdentityType string    `gorm:"column:identity_type;type:varchar(10);default:'human';index" json:"identity_type"` // human / nhi
	Phone        string    `gorm:"column:phone;type:varchar(20);uniqueIndex" json:"phone,omitempty"`
	Email        string    `gorm:"column:email;type:varchar(100);uniqueIndex" json:"email,omitempty"`
	PasswordHash string    `gorm:"column:password_hash;type:varchar(255)" json:"-"`
	Nickname     string    `gorm:"column:nickname;type:varchar(50)" json:"nickname"`
	AvatarURL    string    `gorm:"column:avatar_url;type:varchar(500)" json:"avatar_url,omitempty"`
	// NHI专属：Agent服务名、委托方用户ID
	AgentService string `gorm:"column:agent_service;type:varchar(50)" json:"agent_service,omitempty"` // ziway-Agent等
	DelegatedBy  string `gorm:"column:delegated_by;type:varchar(32)" json:"delegated_by,omitempty"`  // 委托方UserID
	Status       int16  `gorm:"column:status;type:smallint;default:1;index" json:"status"`            // 1启用 0禁用
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`

	// 关联
	Roles []Role `gorm:"many2many:user_roles;" json:"roles,omitempty"`
}

func (User) TableName() string { return "users" }

// IsNHI 是否为非人类身份
func (u *User) IsNHI() bool {
	return u.IdentityType == IdentityTypeNHI
}

// Role 角色表 — 12U基础角色 + 2帽子角色 + NHI
type Role struct {
	ID          int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	RoleCode    string    `gorm:"column:role_code;type:varchar(20);uniqueIndex;not null" json:"role_code"` // CU/DU/.../CX/FX
	RoleName    string    `gorm:"column:role_name;type:varchar(50);not null" json:"role_name"`
	RoleType    string    `gorm:"column:role_type;type:varchar(10);default:'base';index" json:"role_type"` // base/hat/nhi
	Domain      string    `gorm:"column:domain;type:varchar(20);index" json:"domain"`                       // mall/shop/lab/market/mate/case/ams/oas/dyard
	ParentRole  string    `gorm:"column:parent_role;type:varchar(20)" json:"parent_role,omitempty"`         // 帽子归属：CX→HU, FX→(HU/FU)
	Description string    `gorm:"column:description;type:varchar(200)" json:"description,omitempty"`
	Status      int16     `gorm:"column:status;type:smallint;default:1" json:"status"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (Role) TableName() string { return "roles" }

// Permission 权限表
type Permission struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	PermCode  string    `gorm:"column:perm_code;type:varchar(80);uniqueIndex;not null" json:"perm_code"` // order:read, payment:write
	Resource  string    `gorm:"column:resource;type:varchar(200)" json:"resource"`                        // /api/v1/orders
	Action    string    `gorm:"column:action;type:varchar(10)" json:"action"`                             // GET/POST/PUT/DELETE
	Domain    string    `gorm:"column:domain;type:varchar(20);index" json:"domain"`
	Status    int16     `gorm:"column:status;type:smallint;default:1" json:"status"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (Permission) TableName() string { return "permissions" }

// UserRole 用户-角色关联（含domain，支持戴帽子）
type UserRole struct {
	UserID    int64      `gorm:"column:user_id;primaryKey" json:"user_id"`
	RoleID    int64      `gorm:"column:role_id;primaryKey" json:"role_id"`
	Domain    string     `gorm:"column:domain;type:varchar(20);primaryKey" json:"domain"` // 在哪个事业场戴这顶帽子
	GrantedBy *int64     `gorm:"column:granted_by" json:"granted_by,omitempty"`
	GrantedAt time.Time  `gorm:"column:granted_at;autoCreateTime" json:"granted_at"`
	ExpiresAt *time.Time `gorm:"column:expires_at" json:"expires_at,omitempty"`
}

func (UserRole) TableName() string { return "user_roles" }

// RolePermission 角色-权限关联
type RolePermission struct {
	RoleID    int64     `gorm:"column:role_id;primaryKey" json:"role_id"`
	PermID    int64     `gorm:"column:perm_id;primaryKey" json:"perm_id"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (RolePermission) TableName() string { return "role_permissions" }

// LoginRequest 登录请求
type LoginRequest struct {
	Account      string `json:"account" binding:"required"`  // 手机号或邮箱
	Password     string `json:"password" binding:"required,min=6"`
	Domain       string `json:"domain"`                       // 登录到哪个事业场（戴帽子）
	IdentityType string `json:"identity_type,omitempty"`      // human / nhi（Agent调用时传入）
}

// TokenResponse Token响应
type TokenResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int64     `json:"expires_in"`
	User         *UserInfo `json:"user"`
}

// UserInfo 用户信息（登录返回）
type UserInfo struct {
	UserID       string   `json:"user_id"`
	IdentityType string   `json:"identity_type"`
	Nickname     string   `json:"nickname"`
	AvatarURL    string   `json:"avatar_url,omitempty"`
	Roles        []string `json:"roles"`                // 当前domain下的角色
	ActiveRole   string   `json:"active_role,omitempty"` // 当前佩戴的主角色/帽子
	Domain       string   `json:"domain,omitempty"`      // 当前事业场
	// NHI专属
	AgentService string `json:"agent_service,omitempty"`
	DelegatedBy  string `json:"delegated_by,omitempty"`
}

// RefreshTokenRequest 刷新Token
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// SwitchHatRequest 切换帽子请求
type SwitchHatRequest struct {
	Domain   string `json:"domain" binding:"required"`    // 目标事业场
	RoleCode string `json:"role_code,omitempty"`           // 指定佩戴的角色/帽子（可选，不指定则取该domain下所有角色）
}
