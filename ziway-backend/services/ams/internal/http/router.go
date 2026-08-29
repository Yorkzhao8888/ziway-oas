package http

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"ziway.ams/internal/authz"
	"ziway.ams/internal/http/handlers"
	"ziway.ams/internal/http/middleware"
	"ziway.ams/internal/jwt"
	"ziway.ams/internal/repository"
	"ziway.ams/internal/service"

	"github.com/redis/go-redis/v9"
)

// RouterConfig 路由配置
type RouterConfig struct {
	AuthSvc      *service.AuthService
	UserRepo     *repository.UserRepo
	RoleRepo     *repository.RoleRepo
	Verifier     *jwt.Verifier
	Enforcer     *authz.Enforcer
	RDB          *redis.Client
	ServiceToken string // 服务间调用Token
	Logger       *zap.Logger
}

// NewRouter 创建Gin路由
func NewRouter(cfg *RouterConfig) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// 全局中间件
	r.Use(gin.Recovery())
	r.Use(middleware.TraceID())
	r.Use(middleware.CORS())
	r.Use(ginLogger(cfg.Logger))

	authHandler := handlers.NewAuthHandler(cfg.AuthSvc, cfg.Logger)
	userHandler := handlers.NewUserHandler(cfg.UserRepo, cfg.Logger)
	oasHandler := handlers.NewOASHandler(cfg.Enforcer, cfg.RoleRepo, cfg.Logger)

	// Health & Metrics (no auth)
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": "ziway-ams"})
	})
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// API v1
	v1 := r.Group("/api/v1")
	{
		// 公开接口（无需认证）
		auth := v1.Group("/auth")
		{
			auth.POST("/login", authHandler.Login)
			auth.POST("/register", authHandler.Register)
		}

		// 服务间接口（Service Token认证）
		internal := v1.Group("/internal")
		internal.Use(middleware.ServiceToken(cfg.ServiceToken, cfg.Logger))
		{
			internal.POST("/agent-token", authHandler.AgentToken)
			// OAS治理底座对接：策略下发、角色重载、对账快照
			internal.POST("/sync-policy", oasHandler.SyncPolicy)
			internal.POST("/reload-roles", oasHandler.ReloadRoles)
			internal.GET("/policy-snapshot", oasHandler.PolicySnapshot)
		}

		// 需要认证的接口
		authed := v1.Group("")
		authed.Use(middleware.JWTAuth(cfg.Verifier, cfg.RDB, cfg.Logger))
		{
			authed.GET("/auth/me", authHandler.Me)
			authed.POST("/auth/switch-hat", authHandler.SwitchHat)
			authed.POST("/auth/logout", authHandler.Logout)

			// 用户管理（需要RBAC）
			users := authed.Group("/users")
			users.Use(middleware.RBAC(cfg.Enforcer))
			{
				users.GET("/me/profile", userHandler.GetMyProfile)
				users.GET("/:id", userHandler.GetUser)
				users.GET("", userHandler.ListUsers)
			}
		}
	}

	return r
}

func ginLogger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		logger.Debug("http request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.String("trace_id", c.GetString("trace_id")),
		)
	}
}
