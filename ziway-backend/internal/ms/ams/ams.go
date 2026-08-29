// Package ams is the system/authentication MBS — JWT, NHI, X*PZ# issuance.
package ams

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ziway/backend/internal/mbs"
	"ziway/backend/pkg/response"
)

type Service struct {
	mbs.BaseService
}

func New(deps mbs.Dependencies) *Service {
	return &Service{BaseService: mbs.NewBaseService("ams", "mbs_ams", deps)}
}

func (s *Service) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/health", func(c *gin.Context) {
		response.OK(c, gin.H{"ms": "ams", "status": "ok", "desc": "系统/鉴权服务"})
	})
	rg.POST("/auth/login", s.Login)
	rg.POST("/auth/register", s.Register)
	rg.GET("/auth/me", s.Me)
	rg.GET("/users", s.ListUsers)
	rg.GET("/users/:id", s.GetUser)
	rg.PUT("/users/:id", s.UpdateUser)
	rg.GET("/roles", s.ListRoles)
	rg.POST("/roles", s.CreateRole)
}

func (s *Service) AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&User{}, &Role{}, &UserRole{})
}

// ===== Models =====

type User struct {
	ID            uint64         `gorm:"primarykey" json:"id"`
	UserCode      string         `gorm:"uniqueIndex;size:32" json:"user_code"`
	Username      string         `gorm:"uniqueIndex;size:64" json:"username"`
	PasswordHash  string         `gorm:"size:128" json:"-"`
	DisplayName   string         `gorm:"size:64" json:"display_name"`
	Phone         string         `gorm:"size:32" json:"phone"`
	Email         string         `gorm:"size:128" json:"email"`
	Avatar        string         `gorm:"size:512" json:"avatar"`
	IdentityType  string         `gorm:"size:16;index" json:"identity_type"` // CU/DU/HU/PU/EU/OU/GU/AU/FU/IU/VU/SU
	EntityType    string         `gorm:"size:8" json:"entity_type"`         // H=human/N=nhi
	Status        string         `gorm:"size:16;default:active" json:"status"`
	LastLoginAt   *time.Time     `json:"last_login_at,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

type Role struct {
	ID          uint64         `gorm:"primarykey" json:"id"`
	RoleCode    string         `gorm:"uniqueIndex;size:32" json:"role_code"`
	Name        string         `gorm:"size:64" json:"name"`
	Description string         `gorm:"size:256" json:"description"`
	Permissions string         `gorm:"type:text" json:"permissions"` // JSON
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

type UserRole struct {
	ID        uint64    `gorm:"primarykey" json:"id"`
	UserID    uint64    `gorm:"index:idx_user_role,unique" json:"user_id"`
	RoleID    uint64    `gorm:"index:idx_user_role,unique" json:"role_id"`
	GrantedBy string    `gorm:"size:32" json:"granted_by"`
	GrantedAt time.Time `json:"granted_at"`
}

// ===== Handlers =====

func (s *Service) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "username and password required")
		return
	}
	var user User
	if err := s.DB().Where("username = ?", req.Username).First(&user).Error; err != nil {
		response.Unauthorized(c, "invalid credentials")
		return
	}
	if user.Status != "active" {
		response.Forbidden(c, "account disabled")
		return
	}
	now := time.Now()
	s.DB().Model(&user).Update("last_login_at", &now)
	response.OK(c, gin.H{
		"user":          user,
		"access_token":  "dev-token-" + user.UserCode,
		"token_type":    "Bearer",
		"expires_in":    86400,
	})
}

func (s *Service) Register(c *gin.Context) {
	var req struct {
		Username     string `json:"username" binding:"required"`
		Password     string `json:"password" binding:"required"`
		DisplayName  string `json:"display_name"`
		Phone        string `json:"phone"`
		Email        string `json:"email"`
		IdentityType string `json:"identity_type" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	user := User{
		UserCode:     "XHPZ#" + req.IdentityType + "-" + randomCode(),
		Username:     req.Username,
		PasswordHash: req.Password, // dev only
		DisplayName:  req.DisplayName,
		Phone:        req.Phone,
		Email:        req.Email,
		IdentityType: req.IdentityType,
		EntityType:   "H",
		Status:       "active",
	}
	if err := s.DB().Create(&user).Error; err != nil {
		response.InternalError(c, "failed to create user")
		return
	}
	response.Created(c, user)
}

func (s *Service) Me(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		response.Unauthorized(c, "not authenticated")
		return
	}
	var user User
	if err := s.DB().First(&user, userID).Error; err != nil {
		response.NotFound(c, "user not found")
		return
	}
	response.OK(c, user)
}

func (s *Service) ListUsers(c *gin.Context) {
	var items []User
	var total int64
	page, _ := parseInt(c.DefaultQuery("page", "1"))
	size, _ := parseInt(c.DefaultQuery("size", "20"))
	q := s.DB().Model(&User{})
	if it := c.Query("identity_type"); it != "" {
		q = q.Where("identity_type = ?", it)
	}
	if st := c.Query("status"); st != "" {
		q = q.Where("status = ?", st)
	}
	q.Count(&total)
	q.Order("created_at DESC").Offset((page - 1) * size).Limit(size).Find(&items)
	response.OK(c, gin.H{"items": items, "total": total, "page": page, "size": size})
}

func (s *Service) GetUser(c *gin.Context) {
	var user User
	if err := s.DB().First(&user, c.Param("id")).Error; err != nil {
		response.NotFound(c, "user not found")
		return
	}
	response.OK(c, user)
}

func (s *Service) UpdateUser(c *gin.Context) {
	var user User
	if err := s.DB().First(&user, c.Param("id")).Error; err != nil {
		response.NotFound(c, "user not found")
		return
	}
	c.ShouldBindJSON(&user)
	s.DB().Save(&user)
	response.OK(c, user)
}

func (s *Service) ListRoles(c *gin.Context) {
	var roles []Role
	s.DB().Find(&roles)
	response.OK(c, roles)
}

func (s *Service) CreateRole(c *gin.Context) {
	var role Role
	if err := c.ShouldBindJSON(&role); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	s.DB().Create(&role)
	response.Created(c, role)
}

func randomCode() string {
	return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
}

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
