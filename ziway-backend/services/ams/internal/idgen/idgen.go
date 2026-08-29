package idgen

import (
	"fmt"
	"sync"
	"time"
)

// X*PZ# 编号生成器
// 格式：{PREFIX}-PZ#{YYYYMMDD}{SEQUENCE:04d}
// 示例：CU-PZ#202408240001, DU-PZ#202408240001, NHI-PZ#202408240001
//
// 12U基础角色前缀（以U结尾）：
// CU(消费者) DU(门店经营者) PU(创研者) EU(供应商/资源管理者)
// HU(工作者) OU(治理者) GU(监管者) AU(运维管理员) FU(财务)
// IU(投资人) VU(运营执行官) SU(孵化创业者)
//
// 帽子角色前缀（不以U结尾）：CX(客户体验专员) FX(退款专员)
// 非人类身份：NHI(Agent运行时身份)
// 服务编号：ams/cms/dms/...-SVC#0001

const (
	seqMod = 10000
)

var (
	mu       sync.Mutex
	lastDate string
	seqMap   = make(map[string]int) // prefix -> sequence
)

// 12U 基础角色前缀
var BaseRolePrefixes = []string{
	"CU", "DU", "PU", "EU", "HU", "OU",
	"GU", "AU", "FU", "IU", "VU", "SU",
}

// 帽子角色前缀
var HatRolePrefixes = []string{"CX", "FX"}

// 非人类身份前缀
const NHIPrefix = "NHI"

// Generate 生成X*PZ#编号
func Generate(prefix string) string {
	mu.Lock()
	defer mu.Unlock()

	today := time.Now().Format("20060102")
	if today != lastDate {
		seqMap = make(map[string]int)
		lastDate = today
	}

	seqMap[prefix]++
	seq := seqMap[prefix] % seqMod

	return fmt.Sprintf("%s-PZ#%s%04d", prefix, today, seq)
}

// GenerateServiceID 生成服务编号
func GenerateServiceID(serviceName string) string {
	mu.Lock()
	defer mu.Unlock()

	seqMap[serviceName]++
	seq := seqMap[serviceName] % seqMod

	return fmt.Sprintf("%s-SVC#%04d", serviceName, seq)
}

// IsValidPrefix 检查角色前缀是否合法（包含12U+帽子+NHI）
func IsValidPrefix(prefix string) bool {
	valid := map[string]bool{
		// 12U
		"CU": true, "DU": true, "PU": true, "EU": true, "HU": true,
		"OU": true, "GU": true, "AU": true, "FU": true, "IU": true,
		"VU": true, "SU": true,
		// 帽子
		"CX": true, "FX": true,
		// 非人类
		"NHI": true,
	}
	return valid[prefix]
}

// IsBaseRole 判断是否为12U基础角色
func IsBaseRole(code string) bool {
	for _, p := range BaseRolePrefixes {
		if p == code {
			return true
		}
	}
	return false
}

// IsHatRole 判断是否为帽子角色
func IsHatRole(code string) bool {
	for _, p := range HatRolePrefixes {
		if p == code {
			return true
		}
	}
	return false
}
