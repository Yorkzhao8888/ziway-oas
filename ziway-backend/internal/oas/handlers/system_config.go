// system_config.go — 自 cmd/oas/main.go 下沉（OAS-CONSOLE-09 A1），行为零变化。
package handlers

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ziway/backend/internal/oas/authz"
	oasmodel "ziway/backend/internal/oas/model"
	"ziway/backend/pkg/response"
)

func (h *Handlers) ServiceHeartbeat(c *gin.Context) {
	now := time.Now()
	h.DB.Model(&oasmodel.ServiceRegistry{}).Where("id = ?", c.Param("id")).Updates(map[string]interface{}{
		"status":       "healthy",
		"last_seen_at": &now,
	})
	response.OK(c, nil)
}

func (h *Handlers) CreateService(c *gin.Context) {
	var s oasmodel.ServiceRegistry
	if err := c.ShouldBindJSON(&s); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	s.RegisteredAt = time.Now()
	h.DB.Create(&s)
	response.Created(c, s)
}

func (h *Handlers) ListServices(c *gin.Context) {
	var items []oasmodel.ServiceRegistry
	h.DB.Order("service_name").Find(&items)
	response.OK(c, items)
}

// sensitiveKeyBanned 敏感 key 黑名单（模式匹配，超集覆盖任意数据集；
// 生产库 system_configs key 清单以 SELECT category, key 对账为准）
var sensitiveKeyBanned = []string{"secret", "token", "password", "passwd", "credential", "private"}

func (h *Handlers) UpdateConfig(c *gin.Context) {
	username, _ := c.Get("username")
	usernameStr, _ := username.(string)

	// 白名单 B: SU/OU/AU admin
	if !authz.IsInAdminWhitelistB(h.DB, usernameStr) {
		response.Forbidden(c, "only SU/OU/AU admin can update system config")
		return
	}

	key := c.Param("key")
	lowerKey := strings.ToLower(key)
	for _, banned := range sensitiveKeyBanned {
		if strings.Contains(lowerKey, banned) {
			response.BadRequest(c, "sensitive config key is not editable")
			return
		}
	}

	var body struct {
		Value string `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}

	var cfg oasmodel.SystemConfig
	if err := h.DB.Where("key = ?", key).First(&cfg).Error; err != nil {
		cfg = oasmodel.SystemConfig{Key: key, Category: "general"}
	} else if cfg.Encrypted {
		response.BadRequest(c, "encrypted config is not editable")
		return
	}
	oldLen := len(cfg.Value)
	cfg.Value = body.Value
	cfg.UpdatedBy = usernameStr
	h.DB.Save(&cfg)

	// 审计落库：只记长度不记 value 原文
	h.DB.Create(&oasmodel.AuditLog{
		UserID:     usernameStr,
		UserName:   usernameStr,
		Plane:      "admin",
		Action:     "governance.config.update",
		Resource:   "system_config",
		ResourceID: key,
		Detail:     fmt.Sprintf("updated config value (old: %d bytes, new: %d bytes)", oldLen, len(body.Value)),
		Domain:     "OAS",
	})

	response.OK(c, cfg)
}

func (h *Handlers) GetConfigs(c *gin.Context) {
	username, _ := c.Get("username")
	usernameStr, _ := username.(string)

	// 白名单 B: SU/OU/AU admin（读保护与 UpdateConfig 同口径）
	if !authz.IsInAdminWhitelistB(h.DB, usernameStr) {
		response.Forbidden(c, "only SU/OU/AU admin can view configs")
		return
	}

	var items []oasmodel.SystemConfig
	h.DB.Order("category, key").Find(&items)
	for i := range items {
		if items[i].Encrypted {
			items[i].Value = "******"
		}
	}
	response.OK(c, items)
}

func (h *Handlers) GetSystemConfig(c *gin.Context) {
	username, _ := c.Get("username")
	usernameStr, _ := username.(string)

	// 白名单 B: SU/OU/AU
	if !authz.IsInAdminWhitelistB(h.DB, usernameStr) {
		response.Forbidden(c, "only SU/OU/AU admin can view system config")
		return
	}

	// 读取环境变量（非敏感项）
	appEnv := os.Getenv("APP_ENV")
	if appEnv == "" {
		appEnv = "dev"
	}
	oasEnv := os.Getenv("OAS_ENV")
	if oasEnv == "" {
		oasEnv = "DEV"
	}
	dbDriver := os.Getenv("ZIWAY_DATABASE_DRIVER")
	if dbDriver == "" {
		dbDriver = "sqlite"
	}

	// 构建配置信息（不暴露敏感项）
	config := gin.H{
		"app_env":    appEnv,
		"oas_env":    oasEnv,
		"db_driver":  dbDriver,
		"go_version": runtime.Version(),
		"build_time": "2026-09-08", // 可改为实际构建时间
		"git_commit": "fa6cb5d",    // 可改为实际 commit
	}

	response.OK(c, config)
}
