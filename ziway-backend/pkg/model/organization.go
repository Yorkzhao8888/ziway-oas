package model

import (
	"time"

	"gorm.io/gorm"
)

// Organization 组织表（支持多级组织树）
type Organization struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	Code        string         `gorm:"type:varchar(50);uniqueIndex;not null" json:"code"` // 组织编码（唯一）
	Name        string         `gorm:"type:varchar(100);not null" json:"name"`            // 组织名称
	Description string         `gorm:"type:text" json:"description"`                      // 描述
	ParentID    *uint          `gorm:"index" json:"parent_id"`                            // 父组织 ID（nil 表示顶级）
	Domain      string         `gorm:"type:varchar(20);index" json:"domain"`              // 所属域（O/A/F/V/G/T/H/Y 等）
	Status      string         `gorm:"type:varchar(20);default:active" json:"status"`     // 状态：active/inactive
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	// 关联
	Parent   *Organization   `gorm:"foreignKey:ParentID" json:"parent,omitempty"`
	Children []Organization  `gorm:"foreignKey:ParentID" json:"children,omitempty"`
	Members  []UserOrganization `gorm:"foreignKey:OrganizationID" json:"members,omitempty"`
}

func (Organization) TableName() string {
	return "organizations"
}

// UserOrganization 用户-组织关联表
type UserOrganization struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	UserID         uint           `gorm:"index;not null" json:"user_id"`
	OrganizationID uint           `gorm:"index;not null" json:"organization_id"`
	Role           string         `gorm:"type:varchar(50)" json:"role"` // 在组织中的角色（如 member/manager/admin）
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`

	// 关联
	Organization *Organization `gorm:"foreignKey:OrganizationID" json:"organization,omitempty"`
}

func (UserOrganization) TableName() string {
	return "user_organizations"
}
