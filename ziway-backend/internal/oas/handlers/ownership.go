// ownership.go — 自 cmd/oas/main.go 下沉（OAS-CONSOLE-09 A1），行为零变化。
package handlers

import (
	"encoding/json"
	"github.com/gin-gonic/gin"

	oasmodel "ziway/backend/internal/oas/model"
	"ziway/backend/pkg/response"
)

func (h *Handlers) OwnershipMatrix(c *gin.Context) {
	// 获取域注册信息
	var domains []oasmodel.DomainRegistry
	h.DB.Order("domain_code").Find(&domains)

	// 获取服务注册信息
	var services []oasmodel.ServiceRegistry
	h.DB.Order("service_name").Find(&services)

	// 构建所有权矩阵
	matrix := make(map[string]interface{})

	// 域所有权
	domainOwnership := make([]map[string]interface{}, 0)
	for _, d := range domains {
		domainOwnership = append(domainOwnership, map[string]interface{}{
			"domain_code":   d.DomainCode,
			"domain_name":   d.DomainName,
			"bos_name":      d.BOSName,
			"owner_user_id": d.OwnerUserID,
			"status":        d.Status,
		})
	}
	matrix["domains"] = domainOwnership

	// 服务归属
	serviceOwnership := make([]map[string]interface{}, 0)
	for _, s := range services {
		// 尝试从 metadata 中提取域信息
		domain := ""
		if s.Metadata != "" {
			// 简单解析 metadata JSON（如果存在）
			var meta map[string]interface{}
			if err := json.Unmarshal([]byte(s.Metadata), &meta); err == nil {
				if d, ok := meta["domain"].(string); ok {
					domain = d
				}
			}
		}

		serviceOwnership = append(serviceOwnership, map[string]interface{}{
			"service_name": s.ServiceName,
			"service_type": s.ServiceType,
			"version":      s.Version,
			"endpoint":     s.Endpoint,
			"status":       s.Status,
			"domain":       domain,
		})
	}
	matrix["services"] = serviceOwnership

	// 如果无数据，返回默认框架
	if len(domains) == 0 && len(services) == 0 {
		matrix["note"] = "暂无注册数据，以下为默认映射框架"
		matrix["default_mapping"] = []map[string]interface{}{
			{"domain": "T", "name": "技术域", "owner": "TAM", "description": "技术研发支撑"},
			{"domain": "H", "name": "人资云", "owner": "HAM", "description": "人事管理"},
			{"domain": "Y", "name": "智场域", "owner": "YAM", "description": "智场运营"},
			{"domain": "V", "name": "经营域", "owner": "VAM", "description": "经营分析"},
			{"domain": "O", "name": "组织域", "owner": "OAM", "description": "组织管理"},
			{"domain": "A", "name": "行政域", "owner": "AU-admin", "description": "行政管理"},
			{"domain": "F", "name": "财务域", "owner": "FAM", "description": "财务管理"},
			{"domain": "G", "name": "商务域", "owner": "GAM", "description": "商务管理"},
		}
	}

	response.OK(c, matrix)
}
