package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"ziway.ams/internal/jwt"
	"ziway.ams/internal/pkg"
)

// JWTAuth JWT认证中间件
func JWTAuth(verifier *jwt.Verifier, rdb *redis.Client, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			pkg.Unauthorized(c, "missing authorization header")
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			pkg.Unauthorized(c, "invalid authorization format")
			c.Abort()
			return
		}

		claims, err := verifier.Verify(parts[1])
		if err != nil {
			pkg.Unauthorized(c, "invalid or expired token")
			c.Abort()
			return
		}

		// 检查黑名单（登出的Token）
		if claims.TokenID != "" {
			blacklistKey := "ams:blacklist:" + claims.TokenID
			exists, _ := rdb.Exists(c.Request.Context(), blacklistKey).Result()
			if exists > 0 {
				pkg.Unauthorized(c, "token has been revoked")
				c.Abort()
				return
			}
		}

		// 注入上下文
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

// RBACEnforcer 权限校验接口
type RBACEnforcer interface {
	Enforce(userID, domain, resource, action string) (bool, error)
}

// RBAC 权限校验中间件
func RBAC(enforcer RBACEnforcer) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		if userID == "" {
			pkg.Unauthorized(c, "unauthenticated")
			c.Abort()
			return
		}

		domain := c.GetString("domain")
		if domain == "" {
			domain = c.GetHeader("X-Domain")
		}
		resource := c.Request.URL.Path
		action := c.Request.Method

		allowed, err := enforcer.Enforce(userID, domain, resource, action)
		if err != nil {
			pkg.InternalError(c, "permission check failed")
			c.Abort()
			return
		}

		if !allowed {
			pkg.Forbidden(c, "insufficient permissions")
			c.Abort()
			return
		}

		c.Next()
	}
}

// CORS 跨域中间件
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-Domain, X-Trace-Id")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

// TraceID 请求追踪中间件
func TraceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader("X-Trace-Id")
		if traceID == "" {
			b := make([]byte, 8)
			_, _ = rand.Read(b)
			traceID = "trace-" + hex.EncodeToString(b)
		}
		c.Set("trace_id", traceID)
		c.Header("X-Trace-Id", traceID)
		c.Next()
	}
}

// ServiceToken 服务间调用Token认证中间件
func ServiceToken(expectedToken string, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("X-Service-Token")
		if token == "" {
			pkg.Unauthorized(c, "missing service token")
			c.Abort()
			return
		}
		if token != expectedToken {
			logger.Warn("invalid service token",
				zap.String("path", c.Request.URL.Path),
			)
			pkg.Forbidden(c, "invalid service token")
			c.Abort()
			return
		}
		c.Next()
	}
}