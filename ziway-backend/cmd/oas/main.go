package main

import (
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"

	"ziway/backend/pkg/config"
	"ziway/backend/pkg/db"
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
	}

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
