package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"ziway/backend/pkg/jwt"
	"ziway/backend/pkg/response"
)

// JWTAuth JWT认证中间件
// rdb may be nil — blacklist check is skipped when Redis is unavailable.
func JWTAuth(verifier *jwt.Verifier, rdb *redis.Client, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			response.Unauthorized(c, "missing authorization header")
			c.Abort()
			return
		}
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			response.Unauthorized(c, "invalid authorization format")
			c.Abort()
			return
		}
		claims, err := verifier.Verify(parts[1])
		if err != nil {
			logger.Warn("jwt verification failed",
				zap.String("path", c.Request.URL.Path),
				zap.Error(err),
			)
			response.Unauthorized(c, "invalid or expired token")
			c.Abort()
			return
		}
		// 黑名单检查（Redis 可用时）
		if rdb != nil && claims.TokenID != "" {
			exists, _ := rdb.Exists(c.Request.Context(), "ziway:blacklist:"+claims.TokenID).Result()
			if exists > 0 {
				logger.Warn("token revoked",
					zap.String("user_id", claims.UserID),
					zap.String("jti", claims.TokenID),
				)
				response.Unauthorized(c, "token revoked")
				c.Abort()
				return
			}
		}
		c.Set("user_id", claims.UserID)
		c.Set("identity_type", claims.IdentityType)
		c.Set("roles", claims.Roles)
		c.Set("active_role", claims.ActiveRole)
		c.Set("domain", claims.Domain)
		c.Set("token_jti", claims.TokenID)
		if claims.IdentityType == "nhi" {
			c.Set("agent_service", claims.AgentService)
			c.Set("delegated_by", claims.DelegatedBy)
		}
		c.Next()
	}
}

// ServiceToken 服务间调用认证
func ServiceToken(token string, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		t := c.GetHeader("X-Service-Token")
		if t == "" || t != token {
			logger.Warn("invalid service token", zap.String("path", c.Request.URL.Path))
			response.Forbidden(c, "invalid service token")
			c.Abort()
			return
		}
		c.Next()
	}
}

// RBACEnforcer 权限检查接口
type RBACEnforcer interface {
	Enforce(userID, domain, resource, action string) (bool, error)
}

// RoleAwareEnforcer extends RBACEnforcer with role-based checking.
type RoleAwareEnforcer interface {
	RBACEnforcer
	EnforceWithRoles(userID string, roles []string, resource, action string) (bool, error)
}

// RBAC 权限校验中间件
func RBAC(enforcer RBACEnforcer, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		if userID == "" {
			response.Unauthorized(c, "unauthenticated")
			c.Abort()
			return
		}

		var roles []string
		if r, ok := c.Get("roles"); ok {
			if roleSlice, ok := r.([]string); ok {
				roles = roleSlice
			}
		}

		domain := c.GetString("domain")
		if domain == "" {
			domain = c.GetHeader("X-Domain")
		}

		resource := c.Request.URL.Path
		action := c.Request.Method

		var allowed bool
		var err error

		if ra, ok := enforcer.(RoleAwareEnforcer); ok && len(roles) > 0 {
			allowed, err = ra.EnforceWithRoles(userID, roles, resource, action)
		} else {
			allowed, err = enforcer.Enforce(userID, domain, resource, action)
		}

		if err != nil {
			logger.Error("rbac enforce error",
				zap.String("user_id", userID),
				zap.String("path", resource),
				zap.Error(err),
			)
			response.Forbidden(c, "authorization error")
			c.Abort()
			return
		}
		if !allowed {
			logger.Warn("rbac denied",
				zap.String("user_id", userID),
				zap.Strings("roles", roles),
				zap.String("path", resource),
				zap.String("method", action),
			)
			response.Forbidden(c, "insufficient permissions")
			c.Abort()
			return
		}
		c.Next()
	}
}

// CORS 跨域
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-Domain, X-Trace-Id, X-Service-Token")
		c.Header("Access-Control-Max-Age", "86400")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

// TraceID 请求追踪
func TraceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader("X-Trace-Id")
		if traceID == "" {
			b := make([]byte, 8)
			rand.Read(b)
			traceID = "trace-" + hex.EncodeToString(b)
		}
		c.Set("trace_id", traceID)
		c.Header("X-Trace-Id", traceID)
		c.Next()
	}
}

// Recover 恢复panic
func Recover(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic recovered",
					zap.Any("error", r),
					zap.String("path", c.Request.URL.Path),
				)
				response.InternalError(c, "internal server error")
				c.Abort()
			}
		}()
		c.Next()
	}
}
