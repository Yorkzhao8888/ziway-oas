// handlers.go — Handlers 依赖容器（OAS-CONSOLE-09 A1）。
// 收拢原 main() 闭包捕获的依赖，方法值经 handlers.H 挂到路由；依赖单向：
// cmd/oas → internal/oas/handlers → model/authz/pkg，禁止反向导入。
package handlers

import (
	"crypto/rsa"

	"github.com/spf13/viper"
	"gorm.io/gorm"

	"ziway/backend/pkg/envpolicy"
	"ziway/backend/pkg/jwt"
	"ziway/backend/pkg/ratelimit"

	"go.uber.org/zap"
)

// H 全局单例，main() 初始化后路由注册引用其方法值。
var H *Handlers

type Handlers struct {
	DB           *gorm.DB
	Log          *zap.Logger
	Cfg          *viper.Viper
	OASEnv       envpolicy.Environment
	JWTIssuer    *jwt.Issuer
	JWTVerifier  *jwt.Verifier
	JWTPublicKey *rsa.PublicKey
	LoginLimiter *ratelimit.LoginLimiter
	// RegeneratePolicyCSV 由 main 注入（实现在 package oas，handlers 不得反向导入）。
	RegeneratePolicyCSV func(*gorm.DB, *zap.Logger)
}
