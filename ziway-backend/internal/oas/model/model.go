// Package model 承载 OAS 治理平面数据模型（自 cmd/oas/main.go 下沉，OAS-CONSOLE-09 A1）。
package model

import (
	"time"

	"gorm.io/gorm"
)

type SystemConfig struct {
	ID        uint64         `gorm:"primarykey" json:"id"`
	Key       string         `gorm:"uniqueIndex;size:128" json:"key"`
	Value     string         `gorm:"type:text" json:"value"`
	Category  string         `gorm:"size:64;index" json:"category"`
	Encrypted bool           `gorm:"default:false" json:"encrypted"`
	UpdatedBy string         `gorm:"size:32" json:"updated_by"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// AuditLog 审计日志（不可篡改）
type AuditLog struct {
	ID          uint64    `gorm:"primarykey" json:"id"`
	UserID      string    `gorm:"index;size:32" json:"user_id"`
	UserName    string    `gorm:"size:64" json:"user_name"`
	Plane       string    `gorm:"size:16;index" json:"plane"` // owner / admin
	Action      string    `gorm:"size:64;index" json:"action"`
	Resource    string    `gorm:"size:128" json:"resource"`
	ResourceID  string    `gorm:"size:32" json:"resource_id"`
	Detail      string    `gorm:"type:text" json:"detail"`
	IP          string    `gorm:"size:64" json:"ip"`
	UserAgent   string    `gorm:"size:256" json:"user_agent"`
	Environment string    `gorm:"size:16;index" json:"environment"` // DEV/BETA/RC/PROD
	Domain      string    `gorm:"size:8;index" json:"domain"`       // T/H/Y/V/O/A/F/G for XAM filtering
	CreatedAt   time.Time `json:"created_at" json:"created_at"`
}

// DomainRegistry 事业场生命周期管理（Owner Plane）
type DomainRegistry struct {
	ID          uint64         `gorm:"primarykey" json:"id"`
	DomainCode  string         `gorm:"uniqueIndex;size:32" json:"domain_code"`
	DomainName  string         `gorm:"size:128" json:"domain_name"`
	BOSName     string         `gorm:"size:32;index" json:"bos_name"` // cos/dos/...
	Status      string         `gorm:"size:16;default:active;index" json:"status"`
	OwnerUserID string         `gorm:"size:32" json:"owner_user_id"`
	Config      string         `gorm:"type:text" json:"config"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// GovernancePolicy 治理策略（Owner Plane）
type GovernancePolicy struct {
	ID          uint64         `gorm:"primarykey" json:"id"`
	PolicyCode  string         `gorm:"uniqueIndex;size:32" json:"policy_code"`
	Title       string         `gorm:"size:128" json:"title"`
	Category    string         `gorm:"size:64;index" json:"category"`
	Content     string         `gorm:"type:text" json:"content"`
	Status      string         `gorm:"size:16;default:active" json:"status"`
	ApprovedBy  string         `gorm:"size:32" json:"approved_by"`
	EffectiveAt *time.Time     `json:"effective_at,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// ServiceRegistry 服务注册（Admin Plane）
type ServiceRegistry struct {
	ID           uint64         `gorm:"primarykey" json:"id"`
	ServiceName  string         `gorm:"uniqueIndex;size:64" json:"service_name"`
	ServiceType  string         `gorm:"size:32" json:"service_type"` // mbs/bos/oas/app
	Version      string         `gorm:"size:16" json:"version"`
	Endpoint     string         `gorm:"size:256" json:"endpoint"`
	HealthCheck  string         `gorm:"size:256" json:"health_check"`
	Status       string         `gorm:"size:16;default:healthy" json:"status"`
	Metadata     string         `gorm:"type:text" json:"metadata"`
	RegisteredAt time.Time      `json:"registered_at"`
	LastSeenAt   *time.Time     `json:"last_seen_at,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// APIKey 密钥管理（Admin Plane）
type APIKey struct {
	ID        uint64         `gorm:"primarykey" json:"id"`
	KeyName   string         `gorm:"size:64" json:"key_name"`
	KeyPrefix string         `gorm:"uniqueIndex;size:16" json:"key_prefix"`
	KeyHash   string         `gorm:"size:128" json:"-"`
	Scopes    string         `gorm:"type:text" json:"scopes"`
	ExpiresAt *time.Time     `json:"expires_at,omitempty"`
	Status    string         `gorm:"size:16;default:active" json:"status"`
	CreatedBy string         `gorm:"size:32" json:"created_by"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// FederationNode 联邦节点管理（2b-3）
type FederationNode struct {
	ID           uint64         `gorm:"primarykey" json:"id"`
	NodeName     string         `gorm:"size:64" json:"node_name"`
	NodeID       string         `gorm:"uniqueIndex;size:64" json:"node_id"`
	TrustLevel   string         `gorm:"size:16;default:basic" json:"trust_level"` // basic/standard/full
	Status       string         `gorm:"size:16;default:active" json:"status"`     // active/inactive/suspended
	PublicKey    string         `gorm:"type:text" json:"public_key"`
	Endpoint     string         `gorm:"size:256" json:"endpoint"`
	Capabilities string         `gorm:"type:text" json:"capabilities"`
	CreatedBy    string         `gorm:"size:32" json:"created_by"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// OAuthClient OAuth 客户端注册（OAS-CONSOLE-06）
type OAuthClient struct {
	ID           uint64         `gorm:"primarykey" json:"id"`
	ClientID     string         `gorm:"uniqueIndex;size:64" json:"client_id"`
	ClientName   string         `gorm:"size:128" json:"client_name"`
	ClientSecret string         `gorm:"size:128" json:"-"`                    // bcrypt hashed
	RedirectURI  string         `gorm:"type:text" json:"redirect_uri"`        // comma-separated
	Scopes       string         `gorm:"type:text" json:"scopes"`              // comma-separated
	Status       string         `gorm:"size:16;default:active" json:"status"` // active/inactive
	CreatedBy    string         `gorm:"size:32" json:"created_by"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// OAuthAuthorizationCode 授权码（OAS-CONSOLE-06）
type OAuthAuthorizationCode struct {
	ID          uint64    `gorm:"primarykey" json:"id"`
	Code        string    `gorm:"uniqueIndex;size:64" json:"code"`
	ClientID    string    `gorm:"size:64;index" json:"client_id"`
	UserID      string    `gorm:"size:32;index" json:"user_id"`
	RedirectURI string    `gorm:"size:512" json:"redirect_uri"`
	Scopes      string    `gorm:"type:text" json:"scopes"`
	ExpiresAt   time.Time `json:"expires_at"`
	Used        bool      `gorm:"default:false" json:"used"`
	CreatedAt   time.Time `json:"created_at"`
}

// RBACPolicy OAS 权威源 — 唯一 RBAC 策略存储。
// PolicyType 固定为 "rbac"；OAS 为策略唯一写入点，变更后同步 CSV 供 OS 加载。
type RBACPolicy struct {
	ID         uint64         `gorm:"primarykey" json:"id"`
	PolicyType string         `gorm:"size:16;default:rbac;index" json:"policy_type"`
	Subject    string         `gorm:"size:64;index" json:"subject"`
	Resource   string         `gorm:"size:256" json:"resource"`
	Action     string         `gorm:"size:16" json:"action"`
	Effect     string         `gorm:"size:16;default:allow" json:"effect"`
	Domain     string         `gorm:"size:32;index" json:"domain"`
	RoleType   string         `gorm:"size:16" json:"role_type"`
	Version    string         `gorm:"size:32" json:"version"`
	Status     string         `gorm:"size:16;default:active;index" json:"status"`
	CreatedBy  string         `gorm:"size:32" json:"created_by"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}

// OASUser OAS 侧最小用户模型（与 AMS User 共享同一 DB 表）。
type OASUser struct {
	ID           uint64         `gorm:"primarykey" json:"id"`
	UserCode     string         `gorm:"uniqueIndex;size:32" json:"user_code"`
	Username     string         `gorm:"uniqueIndex;size:64" json:"username"`
	PasswordHash string         `gorm:"size:128" json:"-"`
	DisplayName  string         `gorm:"size:64" json:"display_name"`
	RoleCode     string         `gorm:"size:16;index" json:"role_code"`
	IdentityType string         `gorm:"size:16;index" json:"identity_type"`
	EntityType   string         `gorm:"size:8" json:"entity_type"`
	Domain       string         `gorm:"size:8;index" json:"domain,omitempty"`
	Status       string         `gorm:"size:16;default:active" json:"status"`
	LastLoginAt  *time.Time     `json:"last_login_at,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// ApprovalRequest 战略审批单（L0 治理层）
type ApprovalRequest struct {
	ID          uint64     `gorm:"primarykey" json:"id"`
	Title       string     `gorm:"size:200;not null" json:"title"`
	Description string     `gorm:"type:text" json:"description"`
	Type        string     `gorm:"size:50;not null;index" json:"type"`                   // high_privilege/org_delete/key_operation/federation
	Status      string     `gorm:"size:20;not null;default:pending;index" json:"status"` // pending/approved/rejected/executed
	RequesterID string     `gorm:"size:64;not null;index" json:"requester_id"`           // 业务编码（如 XHPZ#OU-ADMIN）
	ApproverID  *string    `gorm:"size:64" json:"approver_id"`                           // 业务编码
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ApprovedAt  *time.Time `json:"approved_at"`
	ExecutedAt  *time.Time `json:"executed_at"`
	Notes       string     `gorm:"type:text" json:"notes"`
	Domain      string     `gorm:"size:8;index" json:"domain"`
	Environment string     `gorm:"size:20" json:"environment"`
}

func (OASUser) TableName() string { return "users" }

// OASRole / OASUserRole — 与 AMS 共享同一 DB 表。
type OASRole struct {
	ID          uint64 `gorm:"primarykey"`
	RoleCode    string `gorm:"uniqueIndex;size:32"`
	Name        string `gorm:"size:64"`
	Description string `gorm:"size:256"`
	Permissions string `gorm:"type:text"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

func (OASRole) TableName() string { return "roles" }

type OASUserRole struct {
	ID        uint64 `gorm:"primarykey"`
	UserID    uint64 `gorm:"index:idx_user_role,unique"`
	RoleID    uint64 `gorm:"index:idx_user_role,unique"`
	GrantedBy string `gorm:"size:32"`
	GrantedAt time.Time
}

func (OASUserRole) TableName() string { return "user_roles" }
