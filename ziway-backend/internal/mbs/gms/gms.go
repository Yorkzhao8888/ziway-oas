// Package gms is the governance/risk MBS — risk control, compliance, audit, circuit breaker.
package gms

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ziway/backend/internal/mbs"
	"ziway/backend/pkg/response"
)

type Service struct{ mbs.BaseService }

func New(deps mbs.Dependencies) *Service {
	return &Service{BaseService: mbs.NewBaseService("gms", "mbs_gms", deps)}
}

func (s *Service) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/health", func(c *gin.Context) {
		response.OK(c, gin.H{"ms": "gms", "status": "ok", "desc": "风控治理服务"})
	})
	rg.POST("/risks", s.CreateRisk)
	rg.GET("/risks", s.ListRisks)
	rg.PUT("/risks/:id/resolve", s.ResolveRisk)
	rg.POST("/policies", s.CreatePolicy)
	rg.GET("/policies", s.ListPolicies)
	rg.POST("/audits", s.CreateAudit)
	rg.GET("/audits", s.ListAudits)
	rg.GET("/dashboard", s.Dashboard)
}

func (s *Service) AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&Risk{}, &Policy{}, &AuditRecord{})
}

type Risk struct {
	ID          uint64         `gorm:"primarykey" json:"id"`
	RiskNo      string         `gorm:"uniqueIndex;size:32" json:"risk_no"`
	Type        string         `gorm:"size:32;index" json:"type"`
	Level       string         `gorm:"size:16;index" json:"level"`
	Title       string         `gorm:"size:128" json:"title"`
	Description string         `gorm:"type:text" json:"description"`
	Source      string         `gorm:"size:32" json:"source"`
	Status      string         `gorm:"size:16;default:open;index" json:"status"`
	Resolver    string         `gorm:"size:32" json:"resolver,omitempty"`
	ResolvedAt  *time.Time     `json:"resolved_at,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

type Policy struct {
	ID        uint64         `gorm:"primarykey" json:"id"`
	Name      string         `gorm:"size:128" json:"name"`
	Type      string         `gorm:"size:32;index" json:"type"`
	Rules     string         `gorm:"type:text" json:"rules"`
	Status    string         `gorm:"size:16;default:active" json:"status"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

type AuditRecord struct {
	ID        uint64    `gorm:"primarykey" json:"id"`
	UserID    string    `gorm:"size:32;index" json:"user_id"`
	Action    string    `gorm:"size:64;index" json:"action"`
	Resource  string    `gorm:"size:128" json:"resource"`
	Detail    string    `gorm:"type:text" json:"detail"`
	IP        string    `gorm:"size:64" json:"ip"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Service) CreateRisk(c *gin.Context) {
	var r Risk
	c.ShouldBindJSON(&r)
	r.RiskNo = fmt.Sprintf("RSK%s%d", time.Now().Format("20060102"), time.Now().UnixNano()%10000)
	s.DB().Create(&r)
	response.Created(c, r)
}

func (s *Service) ListRisks(c *gin.Context) {
	var items []Risk
	var total int64
	q := s.DB().Model(&Risk{})
	if lvl := c.Query("level"); lvl != "" { q = q.Where("level = ?", lvl) }
	if st := c.Query("status"); st != "" { q = q.Where("status = ?", st) }
	q.Count(&total)
	paginate(c, q).Order("created_at DESC").Find(&items)
	response.OK(c, gin.H{"items": items, "total": total})
}

func (s *Service) ResolveRisk(c *gin.Context) {
	var r Risk
	s.DB().First(&r, c.Param("id"))
	now := time.Now()
	r.Status = "resolved"
	r.ResolvedAt = &now
	s.DB().Save(&r)
	response.OK(c, r)
}

func (s *Service) CreatePolicy(c *gin.Context) {
	var p Policy
	c.ShouldBindJSON(&p)
	s.DB().Create(&p)
	response.Created(c, p)
}

func (s *Service) ListPolicies(c *gin.Context) {
	var items []Policy
	s.DB().Find(&items)
	response.OK(c, items)
}

func (s *Service) CreateAudit(c *gin.Context) {
	var a AuditRecord
	c.ShouldBindJSON(&a)
	s.DB().Create(&a)
	response.Created(c, a)
}

func (s *Service) ListAudits(c *gin.Context) {
	var items []AuditRecord
	var total int64
	q := s.DB().Model(&AuditRecord{})
	if uid := c.Query("user_id"); uid != "" { q = q.Where("user_id = ?", uid) }
	q.Count(&total)
	paginate(c, q).Order("created_at DESC").Find(&items)
	response.OK(c, gin.H{"items": items, "total": total})
}

func (s *Service) Dashboard(c *gin.Context) {
	var openRisks, resolvedRisks int64
	s.DB().Model(&Risk{}).Where("status = ?", "open").Count(&openRisks)
	s.DB().Model(&Risk{}).Where("status = ?", "resolved").Count(&resolvedRisks)
	var auditToday int64
	s.DB().Model(&AuditRecord{}).Where("DATE(created_at) = DATE(?)", time.Now()).Count(&auditToday)
	response.OK(c, gin.H{
		"open_risks": openRisks, "resolved_risks": resolvedRisks,
		"audit_today": auditToday,
	})
}

func paginate(c *gin.Context, db *gorm.DB) *gorm.DB {
	p, _ := parseInt(c.DefaultQuery("page", "1"))
	sz, _ := parseInt(c.DefaultQuery("size", "20"))
	if p < 1 { p = 1 }
	if sz < 1 || sz > 100 { sz = 20 }
	return db.Offset((p - 1) * sz).Limit(sz)
}

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
