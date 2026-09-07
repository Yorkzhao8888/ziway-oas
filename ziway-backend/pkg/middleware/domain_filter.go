package middleware

import (
	"github.com/gin-gonic/gin"
)

// DomainFilter 域过滤中间件
// 对 XAM 类用户（TAM/HAM/YAM/VAM）自动注入域过滤条件
// OAM/OU/AU 等超管不受限制，可访问全量数据
func DomainFilter() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从 JWT claims 中获取用户域信息
		userDomain, exists := c.Get("domain")
		if !exists {
			// 未设置域信息，跳过过滤
			c.Next()
			return
		}

		domain, ok := userDomain.(string)
		if !ok || domain == "" {
			// 域信息为空或类型错误，跳过过滤
			c.Next()
			return
		}

		// 检查是否为 XAM 类用户（需要域过滤）
		// XAM 角色：TAM/HAM/YAM/VAM
		// OAM/OU/AU 等超管不需要域过滤
		needsFilter := isXAMRole(domain)

		if needsFilter {
			// 将域信息存入上下文，供后续 handler 使用
			c.Set("filter_domain", domain)
		}

		c.Next()
	}
}

// isXAMRole 判断是否为 XAM 类角色（需要域过滤）
func isXAMRole(domain string) bool {
	switch domain {
	case "T", "H", "Y", "V":
		// TAM/HAM/YAM/VAM 需要域过滤
		return true
	default:
		// O/A/F/G 等域或超管不需要过滤
		return false
	}
}

// GetFilterDomain 获取当前请求的域过滤条件
func GetFilterDomain(c *gin.Context) (string, bool) {
	domain, exists := c.Get("filter_domain")
	if !exists {
		return "", false
	}
	d, ok := domain.(string)
	return d, ok
}
