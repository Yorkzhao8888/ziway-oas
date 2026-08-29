package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"ziway/backend/pkg/config"
	"ziway/backend/pkg/db"
	"ziway/backend/pkg/jwt"
	"ziway/backend/pkg/logger"
	"ziway/backend/pkg/middleware"
	"ziway/backend/pkg/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ========== OAS Models (Owner + Admin shared) ==========

// SystemConfig 系统配置项
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
	ID         uint64    `gorm:"primarykey" json:"id"`
	UserID     string    `gorm:"index;size:32" json:"user_id"`
	UserName   string    `gorm:"size:64" json:"user_name"`
	Plane      string    `gorm:"size:16;index" json:"plane"` // owner / admin
	Action     string    `gorm:"size:64;index" json:"action"`
	Resource   string    `gorm:"size:128" json:"resource"`
	ResourceID string    `gorm:"size:32" json:"resource_id"`
	Detail     string    `gorm:"type:text" json:"detail"`
	IP         string    `gorm:"size:64" json:"ip"`
	UserAgent  string    `gorm:"size:256" json:"user_agent"`
	CreatedAt  time.Time `json:"created_at" json:"created_at"`
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
	ID         uint64         `gorm:"primarykey" json:"id"`
	KeyName    string         `gorm:"size:64" json:"key_name"`
	KeyPrefix  string         `gorm:"uniqueIndex;size:16" json:"key_prefix"`
	KeyHash    string         `gorm:"size:128" json:"-"`
	Scopes     string         `gorm:"type:text" json:"scopes"`
	ExpiresAt  *time.Time     `json:"expires_at,omitempty"`
	Status     string         `gorm:"size:16;default:active" json:"status"`
	CreatedBy  string         `gorm:"size:32" json:"created_by"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
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
	IdentityType string         `gorm:"size:16;index" json:"identity_type"`
	EntityType   string         `gorm:"size:8" json:"entity_type"`
	Status       string         `gorm:"size:16;default:active" json:"status"`
	LastLoginAt  *time.Time     `json:"last_login_at,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

func (OASUser) TableName() string { return "users" }

// OASRole / OASUserRole — 与 AMS 共享同一 DB 表。
type OASRole struct {
	ID          uint64         `gorm:"primarykey"`
	RoleCode    string         `gorm:"uniqueIndex;size:32"`
	Name        string         `gorm:"size:64"`
	Description string         `gorm:"size:256"`
	Permissions string         `gorm:"type:text"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}
func (OASRole) TableName() string { return "roles" }

type OASUserRole struct {
	ID        uint64    `gorm:"primarykey"`
	UserID    uint64    `gorm:"index:idx_user_role,unique"`
	RoleID    uint64    `gorm:"index:idx_user_role,unique"`
	GrantedBy string    `gorm:"size:32"`
	GrantedAt time.Time
}
func (OASUserRole) TableName() string { return "user_roles" }

func main() {
	v, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}
	env := v.GetString("app.env")
	if env == "" {
		env = "dev"
	}
	log := logger.New(env)
	defer log.Sync()

	database, err := db.InitDB(v, log)
	if err != nil {
		log.Fatal("init db", zap.Error(err))
	}
	database.AutoMigrate(
		&SystemConfig{}, &AuditLog{}, &DomainRegistry{},
		&GovernancePolicy{}, &ServiceRegistry{}, &APIKey{},
		&RBACPolicy{}, &OASUser{}, &OASRole{}, &OASUserRole{},
	)

	r := gin.New()
	r.Use(middleware.CORS(), middleware.TraceID(), middleware.Recover(log))

	r.GET("/health", func(c *gin.Context) {
		response.OK(c, gin.H{
			"status":  "ok",
			"service": "ziway-oas",
			"planes":  []string{"owner", "admin"},
		})
	})

	api := r.Group("/api/v1")

	// ===== Auth (public, no JWT required) =====
	var jwtIssuer *jwt.Issuer
	if pkPath := v.GetString("jwt.private_key_path"); pkPath != "" {
		accessTTL := v.GetDuration("jwt.access_ttl")
		if accessTTL == 0 {
			accessTTL = 15 * time.Minute
		}
		issuer, err := jwt.NewIssuer(pkPath, accessTTL, 7*24*time.Hour, v.GetString("jwt.issuer"))
		if err != nil {
			log.Fatal("init jwt issuer", zap.Error(err))
		}
		jwtIssuer = issuer
		log.Info("JWT issuer initialized", zap.String("private_key", pkPath))
	}

	// POST /api/v1/os/:os/proxy/ams/auth/login — unified login, returns real JWT
	api.POST("/os/:os/proxy/ams/auth/login", func(c *gin.Context) {
		if jwtIssuer == nil {
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
		// Look up user from shared users table
		var user OASUser
		if err := database.Where("username = ?", req.Username).First(&user).Error; err != nil {
			response.Unauthorized(c, "invalid credentials")
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
			response.Unauthorized(c, "invalid credentials")
			return
		}
		if user.Status != "active" {
			response.Forbidden(c, "account disabled")
			return
		}
		// Update last_login_at
		now := time.Now()
		database.Model(&user).Update("last_login_at", &now)
		// Get user roles
		var roles []string
		database.Table("user_roles").
			Select("r.role_code").
			Joins("JOIN roles r ON r.id = user_roles.role_id").
			Where("user_roles.user_id = ?", user.ID).
			Pluck("r.role_code", &roles)
		activeRole := ""
		if len(roles) > 0 {
			activeRole = roles[0]
		}
		nhiFlag := user.EntityType == "N"
		claims := &jwt.Claims{
			UserID:       user.UserCode,
			IdentityID:   user.UserCode,
			IdentityType: map[string]string{"H": "human", "N": "nhi"}[user.EntityType],
			Role:         activeRole,
			SubRole:      "",
			NHIFlag:      nhiFlag,
			MSAccess:     []string{"ams", "cms", "dms", "hms", "fms", "tms", "ems", "gms", "oms", "vms", "ims", "sms"},
			Roles:        roles,
			ActiveRole:   activeRole,
			TokenID:      fmt.Sprintf("tok-%d-%d", user.ID, now.Unix()),
		}
		token, ttl, err := jwtIssuer.IssueAccessToken(claims)
		if err != nil {
			response.InternalError(c, "failed to issue token")
			return
		}
		// Audit log: token issued
		database.Create(&AuditLog{
			UserID:   user.UserCode,
			UserName: user.DisplayName,
			Plane:    "admin",
			Action:   "auth.login",
			Resource: "jwt",
			Detail:   fmt.Sprintf("result=success, role=%s, token_id=%s", activeRole, claims.TokenID),
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
	})

	// ===== Owner Plane (/owner/*) — OU 权限 =====
	owner := api.Group("/owner")
	{
		// 事业场生命周期
		owner.GET("/domains", func(c *gin.Context) {
			var items []DomainRegistry
			database.Order("created_at DESC").Find(&items)
			response.OK(c, items)
		})
		owner.POST("/domains", func(c *gin.Context) {
			var d DomainRegistry
			if err := c.ShouldBindJSON(&d); err != nil {
				response.BadRequest(c, "invalid request")
				return
			}
			database.Create(&d)
			response.Created(c, d)
		})
		owner.PUT("/domains/:id/status", func(c *gin.Context) {
			var body struct {
				Status string `json:"status"`
			}
			c.ShouldBindJSON(&body)
			database.Model(&DomainRegistry{}).Where("id = ?", c.Param("id")).Update("status", body.Status)
			response.OK(c, nil)
		})

		// 治理策略
		owner.GET("/policies", func(c *gin.Context) {
			var items []GovernancePolicy
			database.Order("created_at DESC").Find(&items)
			response.OK(c, items)
		})
		owner.POST("/policies", func(c *gin.Context) {
			var p GovernancePolicy
			if err := c.ShouldBindJSON(&p); err != nil {
				response.BadRequest(c, "invalid request")
				return
			}
			database.Create(&p)
			response.Created(c, p)
		})
		owner.PUT("/policies/:id", func(c *gin.Context) {
			var p GovernancePolicy
			if err := database.First(&p, c.Param("id")).Error; err != nil {
				response.NotFound(c, "policy not found")
				return
			}
			c.ShouldBindJSON(&p)
			database.Save(&p)
			response.OK(c, p)
		})
	}

	// ===== Admin Plane (/admin/*) — AU 权限 =====
	admin := api.Group("/admin")
	{
		// 系统配置
		admin.GET("/configs", func(c *gin.Context) {
			var items []SystemConfig
			database.Order("category, key").Find(&items)
			response.OK(c, items)
		})
		admin.PUT("/configs/:key", func(c *gin.Context) {
			var cfg SystemConfig
			if err := database.Where("key = ?", c.Param("key")).First(&cfg).Error; err != nil {
				cfg.Key = c.Param("key")
			}
			c.ShouldBindJSON(&cfg)
			database.Save(&cfg)
			response.OK(c, cfg)
		})

		// 服务注册
		admin.GET("/services", func(c *gin.Context) {
			var items []ServiceRegistry
			database.Order("service_name").Find(&items)
			response.OK(c, items)
		})
		admin.POST("/services", func(c *gin.Context) {
			var s ServiceRegistry
			if err := c.ShouldBindJSON(&s); err != nil {
				response.BadRequest(c, "invalid request")
				return
			}
			s.RegisteredAt = time.Now()
			database.Create(&s)
			response.Created(c, s)
		})
		admin.PUT("/services/:id/heartbeat", func(c *gin.Context) {
			now := time.Now()
			database.Model(&ServiceRegistry{}).Where("id = ?", c.Param("id")).Updates(map[string]interface{}{
				"status":      "healthy",
				"last_seen_at": &now,
			})
			response.OK(c, nil)
		})

		// API密钥
		admin.GET("/api-keys", func(c *gin.Context) {
			var items []APIKey
			database.Order("created_at DESC").Find(&items)
			response.OK(c, items)
		})
		admin.POST("/api-keys", func(c *gin.Context) {
			var k APIKey
			if err := c.ShouldBindJSON(&k); err != nil {
				response.BadRequest(c, "invalid request")
				return
			}
			database.Create(&k)
			response.Created(c, k)
		})

		// 审计日志
		admin.GET("/audit-logs", func(c *gin.Context) {
			var items []AuditLog
			page, _ := parseInt(c.DefaultQuery("page", "1"))
			size, _ := parseInt(c.DefaultQuery("size", "20"))
			q := database.Model(&AuditLog{})
			if uid := c.Query("user_id"); uid != "" {
				q = q.Where("user_id = ?", uid)
			}
			if plane := c.Query("plane"); plane != "" {
				q = q.Where("plane = ?", plane)
			}
			var total int64
			q.Count(&total)
			q.Order("created_at DESC").Offset((page - 1) * size).Limit(size).Find(&items)
			response.OK(c, gin.H{"items": items, "total": total, "page": page, "size": size})
		})

		// ===== RBAC 策略管理 (/admin/rbac/*) — OAS 权威源 =====
		admin.GET("/rbac/policies", func(c *gin.Context) {
			var items []RBACPolicy
			q := database.Model(&RBACPolicy{}).Where("policy_type = ?", "rbac")
			if role := c.Query("role_type"); role != "" {
				q = q.Where("role_type = ?", role)
			}
			if subject := c.Query("subject"); subject != "" {
				q = q.Where("subject = ?", subject)
			}
			q.Order("subject, resource").Find(&items)
			response.OK(c, gin.H{"items": items, "total": len(items)})
		})

		admin.POST("/rbac/policies", func(c *gin.Context) {
			var p RBACPolicy
			if err := c.ShouldBindJSON(&p); err != nil {
				response.BadRequest(c, "invalid request")
				return
			}
			p.PolicyType = "rbac"
			if p.Effect == "" {
				p.Effect = "allow"
			}
			if p.Status == "" {
				p.Status = "active"
			}
			if err := database.Create(&p).Error; err != nil {
				response.BadRequest(c, "create policy failed: "+err.Error())
				return
			}
			regeneratePolicyCSV(database, log)
			response.Created(c, p)
		})

		admin.PUT("/rbac/policies/:id", func(c *gin.Context) {
			var p RBACPolicy
			if err := database.First(&p, c.Param("id")).Error; err != nil {
				response.NotFound(c, "policy not found")
				return
			}
			c.ShouldBindJSON(&p)
			p.ID, _ = parseUint(c.Param("id"))
			database.Save(&p)
			regeneratePolicyCSV(database, log)
			response.OK(c, p)
		})

		admin.DELETE("/rbac/policies/:id", func(c *gin.Context) {
			database.Delete(&RBACPolicy{}, c.Param("id"))
			regeneratePolicyCSV(database, log)
			response.OK(c, nil)
		})

		admin.POST("/rbac/sync", func(c *gin.Context) {
			regeneratePolicyCSV(database, log)
			response.OK(c, gin.H{"message": "policy CSV regenerated"})
		})
	}

	// Seed default RBAC policies if empty
	seedRBACPolicies(database, log)
	// Seed test user for login verification
	seedTestUser(database, log)

	port := v.GetString("server.http_port")
	if port == "" {
		port = "8080"
	}
	log.Info("ziway-oas starting", zap.String("port", port))
	if err := r.Run(":" + port); err != nil {
		log.Fatal("server failed", zap.Error(err))
	}
}

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

func parseUint(s string) (uint64, error) {
	var n uint64
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// regeneratePolicyCSV reads all active RBACPolicy from DB and writes the Casbin-compatible CSV.
func regeneratePolicyCSV(database *gorm.DB, log *zap.Logger) {
	var policies []RBACPolicy
	database.Where("policy_type = ? AND status = ?", "rbac", "active").
		Order("subject, resource").Find(&policies)

	path := "configs/rbac_policy.csv"
	var sb strings.Builder
	sb.WriteString("# RBAC Policy — auto-generated by OAS (authority source)\n")
	sb.WriteString("# Format: p, <subject>, <resource>, <action>\n")
	sb.WriteString("# Role inheritance: g, <user_or_role>, <parent_role>\n\n")

	for _, p := range policies {
		sb.WriteString(fmt.Sprintf("p, %s, %s, %s\n", p.Subject, p.Resource, p.Action))
	}

	// Write role hierarchy (g lines) for 12U + CX/FX + NHI
	sb.WriteString("\n# === Role hierarchy (12U governance chain) ===\n")
	roleHierarchy := map[string]string{
		"CX": "HU", "FX": "FU",
	}
	for child, parent := range roleHierarchy {
		sb.WriteString(fmt.Sprintf("g, %s, %s\n", child, parent))
	}

	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		log.Error("failed to regenerate policy CSV", zap.Error(err))
		return
	}
	log.Info("policy CSV regenerated", zap.String("path", path), zap.Int("rules", len(policies)))
}

// seedRBACPolicies seeds the default 12U + CX/FX + NHI policies if the table is empty.
func seedRBACPolicies(database *gorm.DB, log *zap.Logger) {
	var count int64
	database.Model(&RBACPolicy{}).Where("policy_type = ?", "rbac").Count(&count)
	if count > 0 {
		return
	}

	type seedRow struct {
		subject, resource, action, roleType, domain string
	}

	seeds := []seedRow{
		// SU — SystemUser: full access
		{"SU", "/api/v1/*", "*", "base", "*"},
		// OU — OwnerUser: full access on owner plane
		{"OU", "/api/v1/owner/*", "*", "base", "*"},
		{"OU", "/api/v1/bos/*", "*", "base", "*"},
		// AU — AdminUser: read+write on admin plane
		{"AU", "/api/v1/admin/*", "*", "base", "*"},
		{"AU", "/api/v1/bos/*/proxy/*", "*", "base", "*"},
		// GU — GovernanceUser: read governance
		{"GU", "/api/v1/owner/policies", "GET", "base", "*"},
		{"GU", "/api/v1/bos/gos/*", "GET", "base", "*"},
		{"GU", "/api/v1/bos/oos/*", "GET", "base", "*"},
		// FU — FinancialUser: financial resources
		{"FU", "/api/v1/bos/dos/daily/*", "*", "base", "*"},
		{"FU", "/api/v1/bos/fos/*", "*", "base", "*"},
		// VU — VCaseUser: operations overview
		{"VU", "/api/v1/bos/vos/*", "*", "base", "*"},
		// IU — InvestmentUser: investment resources
		{"IU", "/api/v1/bos/ios/*", "*", "base", "*"},
		// DU — DomainUser: domain-scoped read
		{"DU", "/api/v1/bos/*/proxy/*", "GET", "base", "*"},
		{"DU", "/api/v1/bos/*/proxy/*", "POST", "base", "*"},
		// HU — HumanUser: basic read
		{"HU", "/api/v1/bos/*", "GET", "base", "*"},
		// CU — CustomerUser: customer-scoped read
		{"CU", "/api/v1/bos/cos/*", "GET", "base", "*"},
		// PU — PartnerUser: partner resources
		{"PU", "/api/v1/bos/eos/*", "*", "base", "*"},
		// EU — EmployeeUser: employee resources
		{"EU", "/api/v1/bos/hos/*", "*", "base", "*"},
		// Hat roles
		{"CX", "/api/v1/bos/cos/*", "*", "hat", "*"},
		{"CX", "/api/v1/bos/dos/*", "*", "hat", "*"},
		{"FX", "/api/v1/bos/fos/*", "*", "hat", "*"},
		{"FX", "/api/v1/bos/dos/daily/*", "*", "hat", "*"},
		// NHI — Non-Human Identity (Agent runtime)
		{"NHI", "/api/v1/bos/*/proxy/*", "GET", "nhi", "*"},
		{"NHI", "/api/v1/bos/*/proxy/*", "POST", "nhi", "*"},
	}

	for _, s := range seeds {
		database.Create(&RBACPolicy{
			PolicyType: "rbac",
			Subject:    s.subject,
			Resource:   s.resource,
			Action:     s.action,
			Effect:     "allow",
			Domain:     s.domain,
			RoleType:   s.roleType,
			Version:    "v1",
			Status:     "active",
			CreatedBy:  "system-seed",
		})
	}

	log.Info("RBAC seed policies created", zap.Int("count", len(seeds)))
	regeneratePolicyCSV(database, log)
}

// seedTestUser creates a test user with bcrypt-hashed password if no users exist.
func seedTestUser(database *gorm.DB, log *zap.Logger) {
	var count int64
	database.Model(&OASUser{}).Count(&count)
	if count > 0 {
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("test123"), bcrypt.DefaultCost)
	if err != nil {
		log.Error("failed to hash test password", zap.Error(err))
		return
	}
	database.Create(&OASUser{
		UserCode:     "XHPZ#SU-TEST001",
		Username:     "admin",
		PasswordHash: string(hash),
		DisplayName:  "System Admin",
		IdentityType: "SU",
		EntityType:   "H",
		Status:       "active",
	})
	hash2, _ := bcrypt.GenerateFromPassword([]byte("disabled123"), bcrypt.DefaultCost)
	database.Create(&OASUser{
		UserCode:     "XHPZ#DU-DISABLED",
		Username:     "disabled_user",
		PasswordHash: string(hash2),
		DisplayName:  "Disabled User",
		IdentityType: "DU",
		EntityType:   "H",
		Status:       "disabled",
	})
	log.Info("test users seeded", zap.String("admin", "admin/test123"), zap.String("disabled", "disabled_user/disabled123"))

	// Create SU role and assign to admin user
	var suRole OASRole
	database.Where("role_code = ?", "SU").First(&suRole)
	if suRole.ID == 0 {
		suRole = OASRole{
			RoleCode:    "SU",
			Name:        "System User",
			Description: "System administrator",
		}
		if err := database.Create(&suRole).Error; err != nil {
			log.Error("failed to create SU role", zap.Error(err))
			return
		}
		log.Info("SU role created", zap.Uint64("id", suRole.ID))
	}
	var adminUser OASUser
	database.Where("username = ?", "admin").First(&adminUser)
	if adminUser.ID > 0 {
		var existing int64
		database.Table("user_roles").Where("user_id = ? AND role_id = ?", adminUser.ID, suRole.ID).Count(&existing)
		if existing == 0 {
			assignment := OASUserRole{
				UserID:    adminUser.ID,
				RoleID:    suRole.ID,
				GrantedBy: "system-seed",
				GrantedAt: time.Now(),
			}
			if err := database.Table("user_roles").Create(&assignment).Error; err != nil {
				log.Error("failed to assign SU role to admin", zap.Error(err))
			} else {
				log.Info("SU role assigned to admin", zap.Uint64("user_id", adminUser.ID), zap.Uint64("role_id", suRole.ID))
			}
		}
	}
}
