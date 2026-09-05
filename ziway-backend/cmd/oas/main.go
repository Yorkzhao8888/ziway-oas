package main

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	"ziway/backend/pkg/password"
	"ziway/backend/pkg/ratelimit"

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

	// Login rate limiter: 5 failures = 15 min lockout
	loginLimiter := ratelimit.NewLoginLimiter(5, 15*time.Minute)

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
	var jwtVerifier *jwt.Verifier
	var jwtPublicKey *rsa.PublicKey
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

		// Load public key for verification endpoints
		pubKeyPath := v.GetString("jwt.public_key_path")
		if pubKeyPath != "" {
			pubData, err := os.ReadFile(pubKeyPath)
			if err == nil {
				block, _ := pem.Decode(pubData)
				if block != nil {
					pub, err := x509.ParsePKIXPublicKey(block.Bytes)
					if err == nil {
						if rsaPub, ok := pub.(*rsa.PublicKey); ok {
							jwtPublicKey = rsaPub
							log.Info("JWT public key loaded", zap.String("path", pubKeyPath))
						}
					}
				}
			}
			// Init verifier for admin auth middleware
			verifier, err := jwt.NewVerifier(pubKeyPath)
			if err != nil {
				log.Error("failed to init JWT verifier", zap.Error(err))
			} else {
				jwtVerifier = verifier
				log.Info("JWT verifier initialized")
			}
		}
	}

	// GET /api/v1/auth/public-key — return PEM for RS256 verification
	api.GET("/auth/public-key", func(c *gin.Context) {
		pubKeyPath := v.GetString("jwt.public_key_path")
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
			"algorithm": "RS256",
			"key_type":  "RSA",
			"format":    "PEM",
			"public_key": string(pubData),
			"issuer":    v.GetString("jwt.issuer"),
		})
	})

	// GET /.well-known/jwks.json — standard JWK Set endpoint
	r.GET("/.well-known/jwks.json", func(c *gin.Context) {
		if jwtPublicKey == nil {
			c.JSON(500, gin.H{"error": "public key not available"})
			return
		}
		// Convert RSA public key to JWK format
		nBytes := jwtPublicKey.N.Bytes()
		eBytes := big.NewInt(int64(jwtPublicKey.E)).Bytes()
		jwk := gin.H{
			"kty": "RSA",
			"use": "sig",
			"alg": "RS256",
			"kid": "oas-rsa-001",
			"n":   base64.RawURLEncoding.EncodeToString(nBytes),
			"e":   base64.RawURLEncoding.EncodeToString(eBytes),
		}
		c.JSON(200, gin.H{"keys": []gin.H{jwk}})
	})

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
		// Rate limit check
		clientIP := c.ClientIP()
		if !loginLimiter.Check(clientIP, req.Username) {
			lockout := loginLimiter.LockoutRemaining(clientIP, req.Username)
			response.TooManyRequests(c, "too many failed attempts, try again in "+lockout.Round(time.Second).String())
			return
		}
		// Look up user from shared users table
		var user OASUser
		if err := database.Where("username = ?", req.Username).First(&user).Error; err != nil {
			loginLimiter.RecordFailure(clientIP, req.Username)
			response.Unauthorized(c, "invalid credentials")
			return
		}
		if err := password.Verify(req.Password, user.PasswordHash); err != nil {
			loginLimiter.RecordFailure(clientIP, req.Username)
			response.Unauthorized(c, "invalid credentials")
			return
		}
		if user.Status != "active" {
			response.Forbidden(c, "account disabled")
			return
		}
		loginLimiter.RecordSuccess(clientIP, req.Username)
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
			Username:     user.Username,
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

	// ===== Beta Edition: Quick Login API (一键登录) =====
	// Only available in beta edition for testing purposes
	edition := v.GetString("app.edition")
	if edition == "" {
		edition = "production"
	}
	log.Info("product edition", zap.String("edition", edition))

	if edition == "beta" {
		// POST /api/v1/auth/quick-login — one-click login for testing (beta only)
		// Request: {"role": "SU"} or {"username": "admin"}
		// Returns JWT without password verification
		api.POST("/auth/quick-login", func(c *gin.Context) {
			if jwtIssuer == nil {
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
			var user OASUser
			if req.Username != "" {
				if err := database.Where("username = ?", req.Username).First(&user).Error; err != nil {
					response.NotFound(c, "test user not found")
					return
				}
			} else {
				// Find a test user with the specified role
				var userID uint64
				if err := database.Table("user_roles").
					Select("user_roles.user_id").
					Joins("JOIN roles r ON r.id = user_roles.role_id").
					Where("r.role_code = ? AND user_roles.granted_by = ?", req.Role, "system-seed").
					Pluck("user_roles.user_id", &userID).Error; err != nil || userID == 0 {
					response.NotFound(c, "no test user found for role: "+req.Role)
					return
				}
				if err := database.First(&user, userID).Error; err != nil {
					response.NotFound(c, "test user not found")
					return
				}
			}

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
			token, ttl, err := jwtIssuer.IssueAccessToken(claims)
			if err != nil {
				response.InternalError(c, "failed to issue token")
				return
			}

			// Audit log
			database.Create(&AuditLog{
				UserID:   user.UserCode,
				UserName: user.DisplayName,
				Plane:    "admin",
				Action:   "auth.quick-login",
				Resource: "jwt",
				Detail:   fmt.Sprintf("edition=beta, role=%s, token_id=%s", activeRole, claims.TokenID),
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
		})

		// GET /api/v1/auth/test-accounts — list available test accounts (beta only)
		api.GET("/auth/test-accounts", func(c *gin.Context) {
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
		})
	}

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

	// ===== Admin Plane (/admin/*) — 白名单 A: OU/AU/OAM =====
	if jwtVerifier == nil {
		log.Fatal("admin routes require JWT verifier, but it is not initialized")
	}
	admin := api.Group("/admin")
	admin.Use(middleware.JWTAuth(jwtVerifier, nil, log))
	admin.Use(middleware.RequireUsers("oas-ou-admin", "oas-au-admin", "oas-oam-admin"))
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

		// 审计日志 — 白名单 B: 仅 OU/AU（OAM 不可读审计，已裁定）
		admin.GET("/audit-logs", func(c *gin.Context) {
			username, _ := c.Get("username")
			if !isInAdminWhitelistB(username.(string)) {
				response.Forbidden(c, "audit logs restricted to OU/AU admin")
				return
			}
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
	// Seed test users based on product edition
	edition = v.GetString("app.edition")
	if edition == "" {
		edition = "production"
	}
	seedTestUsers(database, log, edition)

	// ===== Login Page (GET /login) =====
	r.GET("/login", func(c *gin.Context) {
		redirect := c.Query("redirect")
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(200, loginPageHTML(redirect, edition))
	})

	// ===== POST /api/v1/auth/login — username+password login =====
	api.POST("/auth/login", func(c *gin.Context) {
		if jwtIssuer == nil {
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
		if !loginLimiter.Check(clientIP, req.Username) {
			lockout := loginLimiter.LockoutRemaining(clientIP, req.Username)
			response.TooManyRequests(c, "too many failed attempts, try again in "+lockout.Round(time.Second).String())
			return
		}
		var user OASUser
		if err := database.Where("username = ?", req.Username).First(&user).Error; err != nil {
			loginLimiter.RecordFailure(clientIP, req.Username)
			response.Unauthorized(c, "invalid credentials")
			return
		}
		if err := password.Verify(req.Password, user.PasswordHash); err != nil {
			loginLimiter.RecordFailure(clientIP, req.Username)
			response.Unauthorized(c, "invalid credentials")
			return
		}
		if user.Status != "active" {
			response.Forbidden(c, "account disabled")
			return
		}
		loginLimiter.RecordSuccess(clientIP, req.Username)
		now := time.Now()
		database.Model(&user).Update("last_login_at", &now)
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
			TokenID:      fmt.Sprintf("login-%d-%d", user.ID, now.Unix()),
		}
		token, ttl, err := jwtIssuer.IssueAccessToken(claims)
		if err != nil {
			response.InternalError(c, "failed to issue token")
			return
		}
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
			"username":     user.Username,
		})
	})

	// ===== User Management API (/api/v1/admin/users/*) — JWT + SU/AU only =====
	adminUsers := api.Group("/admin")
	if jwtVerifier != nil {
		adminUsers.Use(middleware.JWTAuth(jwtVerifier, nil, log))
		// 白名单 A：系统管理访问（/admin/*）= OU-admin + AU-admin + OAM
		adminUsers.Use(middleware.RequireUsers("oas-ou-admin", "oas-au-admin", "oas-oam-admin"))
	}
	adminUsers.GET("/users", func(c *gin.Context) {
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
		var users []OASUser
		database.Order("created_at DESC").Find(&users)
		var result []UserVO
		for _, u := range users {
			var roles []string
			database.Table("user_roles").
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
	})

	adminUsers.POST("/users", func(c *gin.Context) {
		var req struct {
			Username    string   `json:"username" binding:"required"`
			Password    string   `json:"password" binding:"required"`
			DisplayName string   `json:"display_name"`
			RoleCode    string   `json:"role_code" binding:"required"`
			Roles       []string `json:"roles"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, "username, password, role_code required")
			return
		}
		// 白名单 B：创建 admin 账号仅 OU/AU admin 可操作
		operatorUsername, _ := c.Get("username")
		if isAdminAccount(req.Username) && !canOperateAdminAccount(fmt.Sprintf("%v", operatorUsername)) {
			response.Forbidden(c, "only oas-ou-admin and oas-au-admin can create admin accounts")
			return
		}
		var existing OASUser
		if database.Where("username = ?", req.Username).First(&existing).Error == nil {
			response.BadRequest(c, "username already exists")
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
		user := OASUser{
			UserCode:     userCode,
			Username:     req.Username,
			PasswordHash: string(hash),
			DisplayName:  displayName,
			IdentityType: req.RoleCode,
			EntityType:   "H",
			Status:       "active",
		}
		if err := database.Create(&user).Error; err != nil {
			response.BadRequest(c, "create user failed: "+err.Error())
			return
		}
		roleCodes := req.Roles
		if len(roleCodes) == 0 {
			roleCodes = []string{req.RoleCode}
		}
		for _, rc := range roleCodes {
			var role OASRole
			if database.Where("role_code = ?", rc).First(&role).Error == nil {
				database.Table("user_roles").Create(&OASUserRole{
					UserID:    user.ID,
					RoleID:    role.ID,
					GrantedBy: "admin",
					GrantedAt: time.Now(),
				})
			}
		}
		database.Create(&AuditLog{
			Plane:    "admin",
			Action:   "user.create",
			UserID:   user.UserCode,
			UserName: user.DisplayName,
			Resource: "user",
			Detail:   fmt.Sprintf("username=%s, roles=%v", req.Username, roleCodes),
		})
		response.Created(c, gin.H{"id": user.ID, "username": user.Username, "user_code": user.UserCode})
	})

	adminUsers.PUT("/users/:id/roles", func(c *gin.Context) {
		var req struct {
			Roles []string `json:"roles" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, "roles required")
			return
		}
		id, _ := parseUint(c.Param("id"))
		// 白名单 B：修改 admin 账号角色仅 OU/AU admin 可操作
		var targetUser OASUser
		database.First(&targetUser, id)
		operatorUsername, _ := c.Get("username")
		if isAdminAccount(targetUser.Username) && !canOperateAdminAccount(fmt.Sprintf("%v", operatorUsername)) {
			response.Forbidden(c, "only oas-ou-admin and oas-au-admin can modify admin account roles")
			return
		}
		database.Where("user_id = ?", id).Delete(&OASUserRole{})
		for _, rc := range req.Roles {
			var role OASRole
			if database.Where("role_code = ?", rc).First(&role).Error == nil {
				database.Table("user_roles").Create(&OASUserRole{
					UserID:    id,
					RoleID:    role.ID,
					GrantedBy: "admin",
					GrantedAt: time.Now(),
				})
			}
		}
		database.Create(&AuditLog{
			Plane:    "admin",
			Action:   "user.update_roles",
			Resource: "user",
			Detail:   fmt.Sprintf("user_id=%d, roles=%v", id, req.Roles),
		})
		response.OK(c, nil)
	})

	adminUsers.PUT("/users/:id/status", func(c *gin.Context) {
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
		var targetUser OASUser
		database.First(&targetUser, id)
		operatorUsername, _ := c.Get("username")
		if isAdminAccount(targetUser.Username) && !canOperateAdminAccount(fmt.Sprintf("%v", operatorUsername)) {
			response.Forbidden(c, "only oas-ou-admin and oas-au-admin can modify admin account status")
			return
		}
		database.Model(&OASUser{}).Where("id = ?", id).Update("status", req.Status)
		database.Create(&AuditLog{
			Plane:    "admin",
			Action:   "user.update_status",
			Resource: "user",
			Detail:   fmt.Sprintf("user_id=%d, status=%s", id, req.Status),
		})
		response.OK(c, nil)
	})

	// GET /api/v1/auth/roles — list available roles
	api.GET("/auth/roles", func(c *gin.Context) {
		var roles []OASRole
		database.Order("role_code").Find(&roles)
		type RoleVO struct {
			Code string `json:"code"`
			Name string `json:"name"`
		}
		var result []RoleVO
		for _, r := range roles {
			result = append(result, RoleVO{Code: r.RoleCode, Name: r.Name})
		}
		response.OK(c, result)
	})

	// ===== User Management Page (GET /admin/users) — JWT required =====
	r.GET("/admin/users", func(c *gin.Context) {
		if jwtVerifier == nil {
			response.InternalError(c, "jwt verifier not configured")
			return
		}
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Redirect(302, "/login?redirect=/admin/users")
			return
		}
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.Redirect(302, "/login?redirect=/admin/users")
			return
		}
		claims, err := jwtVerifier.Verify(parts[1])
		if err != nil {
			c.Redirect(302, "/login?redirect=/admin/users")
			return
		}
		// 白名单 A：系统管理访问 = OU-admin + AU-admin + OAM
		if claims.Username != "oas-ou-admin" && claims.Username != "oas-au-admin" && claims.Username != "oas-oam-admin" {
			response.Forbidden(c, "access denied")
			return
		}
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(200, userMgmtPageHTML())
	})

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
		// EM — EnterpriseManager: supply chain management (C1)
		{"EM", "/api/v1/bos/*", "*", "base", "*"},
		{"EM", "/api/v1/os/*/proxy/*", "*", "base", "*"},
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

// adminUsernames are the highest-privilege accounts.
// Whitelist B: only oas-ou-admin and oas-au-admin can operate on these accounts.
var adminUsernames = map[string]bool{
	"oas-ou-admin":  true,
	"oas-au-admin":  true,
	"oas-oam-admin": true,
}

func isAdminAccount(username string) bool {
	return adminUsernames[username]
}

// canOperateAdminAccount checks if the operator can manage admin accounts (Whitelist B).
func canOperateAdminAccount(operatorUsername string) bool {
	return operatorUsername == "oas-ou-admin" || operatorUsername == "oas-au-admin"
}

// isInAdminWhitelistB checks if the user is in Whitelist B (audit log access).
func isInAdminWhitelistB(username string) bool {
	return canOperateAdminAccount(username)
}

// seedTestUser creates a test user with bcrypt-hashed password if no users exist.
// seedTestUsers creates test accounts based on product edition.
// Beta edition: multiple test accounts with different roles (SU/AU/CU/GU)
// Production edition: only admin account
// Admin accounts (oas-ou-admin, oas-au-admin) are always created.
func seedTestUsers(database *gorm.DB, log *zap.Logger, edition string) {
	hash, err := password.Hash("test123")
	if err != nil {
		log.Error("failed to hash test password", zap.Error(err))
		return
	}
	hashStr := string(hash)

	// 最高管理者账号（始终创建，用于用户管理接口鉴权）
	adminAccounts := []struct {
		UserCode     string
		Username     string
		DisplayName  string
		IdentityType string
		RoleCode     string
		RoleName     string
	}{
		{UserCode: "XHPZ#OU-ADMIN", Username: "oas-ou-admin", DisplayName: "OAS 组织管理员", IdentityType: "OU", RoleCode: "SU", RoleName: "System User"},
		{UserCode: "XHPZ#AU-ADMIN", Username: "oas-au-admin", DisplayName: "OAS 运营管理员", IdentityType: "AU", RoleCode: "SU", RoleName: "System User"},
		{UserCode: "XHPZ#OAM-ADMIN", Username: "oas-oam-admin", DisplayName: "OAS 权限执行管理员", IdentityType: "OAM", RoleCode: "SU", RoleName: "System User"},
	}

	// 确保 SU 角色存在
	var suRole OASRole
	database.Where("role_code = ?", "SU").First(&suRole)
	if suRole.ID == 0 {
		suRole = OASRole{RoleCode: "SU", Name: "System User", Description: "System User role"}
		database.Create(&suRole)
	}

	// 创建管理员账号（如果不存在）
	for _, acc := range adminAccounts {
		var existing OASUser
		database.Where("username = ?", acc.Username).First(&existing)
		if existing.ID == 0 {
			user := OASUser{
				UserCode:     acc.UserCode,
				Username:     acc.Username,
				PasswordHash: hashStr,
				DisplayName:  acc.DisplayName,
				IdentityType: acc.IdentityType,
				EntityType:   "H",
				Status:       "active",
			}
			if err := database.Create(&user).Error; err == nil {
				assignment := OASUserRole{UserID: user.ID, RoleID: suRole.ID, GrantedBy: "system-seed", GrantedAt: time.Now()}
				database.Table("user_roles").Create(&assignment)
				log.Info("admin user created", zap.String("username", acc.Username))
			}
		}
	}

	// 检查是否已有其他用户
	var count int64
	database.Model(&OASUser{}).Count(&count)
	if count > 2 { // 已有除管理员外的用户
		return
	}

	// Define test accounts for beta edition
	type testAccount struct {
		UserCode     string
		Username     string
		DisplayName  string
		IdentityType string
		RoleCode     string
		RoleName     string
	}

	accounts := []testAccount{
		// 默认测试账号
		{UserCode: "XHPZ#SU-TEST001", Username: "admin", DisplayName: "系统管理员", IdentityType: "SU", RoleCode: "SU", RoleName: "System User"},
	}

	// Beta edition: add more test accounts with different roles
	if edition == "beta" {
		accounts = append(accounts,
			testAccount{UserCode: "XHPZ#AU-TEST001", Username: "operator", DisplayName: "运营人员", IdentityType: "AU", RoleCode: "AU", RoleName: "Admin User"},
			testAccount{UserCode: "XHPZ#CU-TEST001", Username: "customer", DisplayName: "客户用户", IdentityType: "CU", RoleCode: "CU", RoleName: "Customer User"},
			testAccount{UserCode: "XHPZ#GU-TEST001", Username: "viewer", DisplayName: "访客", IdentityType: "GU", RoleCode: "GU", RoleName: "Guest User"},
			testAccount{UserCode: "XHPZ#EM-TEST001", Username: "em", DisplayName: "供给运营长", IdentityType: "EM", RoleCode: "EM", RoleName: "Enterprise Manager"},
		)
		log.Info("beta edition: seeding multiple test accounts")
	}

	// Create roles and users
	for _, acc := range accounts {
		// Create role if not exists
		var role OASRole
		database.Where("role_code = ?", acc.RoleCode).First(&role)
		if role.ID == 0 {
			role = OASRole{
				RoleCode:    acc.RoleCode,
				Name:        acc.RoleName,
				Description: acc.RoleName + " role",
			}
			if err := database.Create(&role).Error; err != nil {
				log.Error("failed to create role", zap.String("role", acc.RoleCode), zap.Error(err))
				continue
			}
			log.Info("role created", zap.String("role_code", acc.RoleCode), zap.Uint64("id", role.ID))
		}

		// Create user
		user := OASUser{
			UserCode:     acc.UserCode,
			Username:     acc.Username,
			PasswordHash: hashStr,
			DisplayName:  acc.DisplayName,
			IdentityType: acc.IdentityType,
			EntityType:   "H",
			Status:       "active",
		}
		if err := database.Create(&user).Error; err != nil {
			log.Error("failed to create user", zap.String("username", acc.Username), zap.Error(err))
			continue
		}

		// Assign role
		assignment := OASUserRole{
			UserID:    user.ID,
			RoleID:    role.ID,
			GrantedBy: "system-seed",
			GrantedAt: time.Now(),
		}
		if err := database.Table("user_roles").Create(&assignment).Error; err != nil {
			log.Error("failed to assign role", zap.String("username", acc.Username), zap.Error(err))
		} else {
			log.Info("test user created", zap.String("username", acc.Username), zap.String("role", acc.RoleCode))
		}
	}

	// Create disabled user for testing 403 response
	hash2, _ := password.Hash("disabled123")
	database.Create(&OASUser{
		UserCode:     "XHPZ#DU-DISABLED",
		Username:     "disabled_user",
		PasswordHash: string(hash2),
		DisplayName:  "Disabled User",
		IdentityType: "DU",
		EntityType:   "H",
		Status:       "disabled",
	})

	log.Info("test users seeded", zap.String("edition", edition), zap.Int("count", len(accounts)))
}

// loginPageHTML returns the login page HTML.
func loginPageHTML(redirect, edition string) string {
	quickLoginSection := ""
	if edition == "beta" {
		quickLoginSection = `
		<div style="margin-top:24px;padding-top:20px;border-top:1px solid #e5e7eb">
			<p style="font-size:13px;color:#6b7280;margin-bottom:12px">内测快捷登录</p>
			<div style="display:flex;gap:8px;flex-wrap:wrap">
				<button onclick="quickLogin('SU')" style="padding:6px 14px;border:1px solid #d1d5db;border-radius:6px;background:#f9fafb;cursor:pointer;font-size:13px">SU 管理员</button>
				<button onclick="quickLogin('AU')" style="padding:6px 14px;border:1px solid #d1d5db;border-radius:6px;background:#f9fafb;cursor:pointer;font-size:13px">AU 运营</button>
				<button onclick="quickLogin('CU')" style="padding:6px 14px;border:1px solid #d1d5db;border-radius:6px;background:#f9fafb;cursor:pointer;font-size:13px">CU 客户</button>
				<button onclick="quickLogin('GU')" style="padding:6px 14px;border:1px solid #d1d5db;border-radius:6px;background:#f9fafb;cursor:pointer;font-size:13px">GU 访客</button>
				<button onclick="quickLogin('EM')" style="padding:6px 14px;border:1px solid #d1d5db;border-radius:6px;background:#f9fafb;cursor:pointer;font-size:13px">EM 供给</button>
			</div>
		</div>`
	}
	redirectAttr := ""
	if redirect != "" {
		redirectAttr = `data-redirect="` + redirect + `"`
	}
	return `<!DOCTYPE html>
<html lang="zh">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>OAS Login</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:#f3f4f6;display:flex;align-items:center;justify-content:center;min-height:100vh}
.card{background:#fff;border-radius:12px;box-shadow:0 1px 3px rgba(0,0,0,.1);padding:40px;width:100%;max-width:400px}
h1{font-size:20px;font-weight:600;margin-bottom:4px;color:#111827}
.sub{font-size:13px;color:#6b7280;margin-bottom:24px}
label{display:block;font-size:13px;font-weight:500;color:#374151;margin-bottom:4px}
input{width:100%;padding:10px 12px;border:1px solid #d1d5db;border-radius:8px;font-size:14px;margin-bottom:16px;outline:none;transition:border .15s}
input:focus{border-color:#2563eb;box-shadow:0 0 0 3px rgba(37,99,235,.1)}
.btn{width:100%;padding:10px;border:none;border-radius:8px;background:#2563eb;color:#fff;font-size:14px;font-weight:500;cursor:pointer;transition:background .15s}
.btn:hover{background:#1d4ed8}
.btn:disabled{background:#93c5fd;cursor:not-allowed}
.err{color:#dc2626;font-size:13px;margin-top:8px;display:none}
</style>
</head>
<body>
<div class="card" id="form" ` + redirectAttr + `>
	<h1>知味 OAS</h1>
	<p class="sub">统一身份认证</p>
	<form onsubmit="return doLogin(event)">
		<label for="username">用户名</label>
		<input id="username" name="username" autocomplete="username" required>
		<label for="password">密码</label>
		<input id="password" name="password" type="password" autocomplete="current-password" required>
		<button class="btn" type="submit" id="submitBtn">登录</button>
		<p class="err" id="errMsg"></p>
	</form>` + quickLoginSection + `
</div>
<script>
async function doLogin(e){
	e.preventDefault();
	const btn=document.getElementById('submitBtn');
	const err=document.getElementById('errMsg');
	btn.disabled=true;err.style.display='none';
	try{
		const r=await fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},
			body:JSON.stringify({username:document.getElementById('username').value,password:document.getElementById('password').value})});
		const d=await r.json();
		if(d.code!==200)throw new Error(d.message||'login failed');
		handleToken(d.data);
	}catch(ex){err.textContent=ex.message;err.style.display='block';btn.disabled=false}
}
async function quickLogin(role){
	const btn=document.getElementById('submitBtn');
	btn.disabled=true;
	try{
		const r=await fetch('/api/v1/auth/quick-login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({role})});
		const d=await r.json();
		if(d.code!==200)throw new Error(d.message||'quick login failed');
		handleToken(d.data);
	}catch(ex){alert(ex.message);btn.disabled=false}
}
function handleToken(data){
	const redirect=document.getElementById('form').dataset.redirect;
	if(redirect){
		const sep=redirect.includes('?')?'&':'?';
		window.location.href=redirect+sep+'token='+data.access_token;
	}else{
		document.getElementById('form').innerHTML='<h1>登录成功</h1><p class="sub">角色: '+data.role+'</p><pre style="font-size:11px;word-break:break-all;background:#f9fafb;padding:12px;border-radius:8px;margin-top:12px;max-height:200px;overflow:auto">'+data.access_token+'</pre><p style="margin-top:12px;font-size:13px;color:#6b7280">Token 有效期: '+data.expires_in+'s</p>';
	}
}
</script>
</body>
</html>`
}

// userMgmtPageHTML returns the user management page HTML.
func userMgmtPageHTML() string {
	return `<!DOCTYPE html>
<html lang="zh">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>OAS User Management</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:#f3f4f6;color:#111827}
.header{background:#fff;border-bottom:1px solid #e5e7eb;padding:16px 24px;display:flex;align-items:center;justify-content:space-between}
.header h1{font-size:18px;font-weight:600}
.container{max-width:960px;margin:24px auto;padding:0 24px}
.card{background:#fff;border-radius:10px;box-shadow:0 1px 2px rgba(0,0,0,.06);padding:24px;margin-bottom:20px}
.card h2{font-size:16px;font-weight:600;margin-bottom:16px}
table{width:100%;border-collapse:collapse;font-size:14px}
th{text-align:left;padding:10px 12px;border-bottom:2px solid #e5e7eb;font-weight:600;color:#6b7280;font-size:12px;text-transform:uppercase}
td{padding:10px 12px;border-bottom:1px solid #f3f4f6}
.badge{display:inline-block;padding:2px 8px;border-radius:9999px;font-size:12px;font-weight:500}
.badge-active{background:#d1fae5;color:#065f46}
.badge-disabled{background:#fee2e2;color:#991b1b}
.badge-role{background:#e0e7ff;color:#3730a3;margin-right:4px}
.btn{padding:6px 14px;border:none;border-radius:6px;font-size:13px;cursor:pointer;font-weight:500}
.btn-primary{background:#2563eb;color:#fff}.btn-primary:hover{background:#1d4ed8}
.btn-danger{background:#fee2e2;color:#991b1b}.btn-danger:hover{background:#fecaca}
.btn-success{background:#d1fae5;color:#065f46}.btn-success:hover{background:#a7f3d0}
.btn-sm{padding:4px 10px;font-size:12px}
.form-row{display:flex;gap:12px;margin-bottom:12px}
.form-row>div{flex:1}
.form-row label{display:block;font-size:12px;font-weight:500;color:#6b7280;margin-bottom:4px}
.form-row input,.form-row select{width:100%;padding:8px 10px;border:1px solid #d1d5db;border-radius:6px;font-size:13px}
.modal-overlay{display:none;position:fixed;inset:0;background:rgba(0,0,0,.4);z-index:100;align-items:center;justify-content:center}
.modal-overlay.show{display:flex}
.modal{background:#fff;border-radius:12px;padding:24px;width:100%;max-width:420px}
.modal h3{font-size:16px;margin-bottom:16px}
.empty{text-align:center;padding:40px;color:#9ca3af;font-size:14px}
</style>
</head>
<body>
<div class="header">
	<h1>知味 OAS 用户管理</h1>
	<a href="/login" style="font-size:13px;color:#6b7280;text-decoration:none">← 返回登录</a>
</div>
<div class="container">
	<div class="card">
		<div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:16px">
			<h2>用户列表</h2>
			<button class="btn btn-primary" onclick="showAddModal()">+ 新增用户</button>
		</div>
		<div id="userTable"><div class="empty">加载中...</div></div>
	</div>
</div>

<div class="modal-overlay" id="addModal">
	<div class="modal">
		<h3>新增用户</h3>
		<div class="form-row"><div><label>用户名</label><input id="newUsername" required></div></div>
		<div class="form-row"><div><label>密码</label><input id="newPassword" type="password" required></div></div>
		<div class="form-row"><div><label>显示名</label><input id="newDisplayName"></div></div>
		<div class="form-row"><div><label>角色</label><select id="newRole"></select></div></div>
		<div style="display:flex;gap:8px;justify-content:flex-end;margin-top:16px">
			<button class="btn" style="background:#f3f4f6" onclick="hideAddModal()">取消</button>
			<button class="btn btn-primary" onclick="createUser()">创建</button>
		</div>
	</div>
</div>

<script>
const API='/api/v1';
async function loadUsers(){
	const r=await fetch(API+'/admin/users');
	const d=await r.json();
	const items=d.data.items||[];
	if(!items.length){document.getElementById('userTable').innerHTML='<div class="empty">暂无用户</div>';return}
	let html='<table><thead><tr><th>用户名</th><th>显示名</th><th>角色</th><th>状态</th><th>操作</th></tr></thead><tbody>';
	for(const u of items){
		const roles=u.roles.map(r=>'<span class="badge badge-role">'+r+'</span>').join('');
		const status=u.status==='active'?'<span class="badge badge-active">启用</span>':'<span class="badge badge-disabled">禁用</span>';
		const toggleBtn=u.status==='active'
			?'<button class="btn btn-danger btn-sm" onclick="toggleStatus('+u.id+',\'disabled\')">禁用</button>'
			:'<button class="btn btn-success btn-sm" onclick="toggleStatus('+u.id+',\'active\')">启用</button>';
		html+='<tr><td>'+u.username+'</td><td>'+u.display_name+'</td><td>'+roles+'</td><td>'+status+'</td><td>'+toggleBtn+'</td></tr>';
	}
	html+='</tbody></table>';
	document.getElementById('userTable').innerHTML=html;
}
async function loadRoles(){
	const r=await fetch(API+'/auth/roles');
	const d=await r.json();
	const sel=document.getElementById('newRole');
	sel.innerHTML='';
	for(const role of d.data){sel.innerHTML+='<option value="'+role.code+'">'+role.code+' - '+role.name+'</option>'}
}
function showAddModal(){document.getElementById('addModal').classList.add('show');loadRoles()}
function hideAddModal(){document.getElementById('addModal').classList.remove('show')}
async function createUser(){
	const body={
		username:document.getElementById('newUsername').value,
		password:document.getElementById('newPassword').value,
		display_name:document.getElementById('newDisplayName').value,
		role_code:document.getElementById('newRole').value
	};
	const r=await fetch(API+'/admin/users',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
	const d=await r.json();
	if(d.code===201||d.code===200){hideAddModal();loadUsers()}else{alert(d.message||'create failed')}
}
async function toggleStatus(id,status){
	await fetch(API+'/admin/users/'+id+'/status',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({status})});
	loadUsers();
}
loadUsers();
</script>
</body>
</html>`
}
