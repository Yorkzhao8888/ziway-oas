// Package oms is the ecosystem governance MBS — approvals, compliance checks, performance templates.
package oms

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
	return &Service{BaseService: mbs.NewBaseService("oms", "mbs_oms", deps)}
}

func (s *Service) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/health", func(c *gin.Context) {
		response.OK(c, gin.H{"ms": "oms", "status": "ok", "desc": "生态治理监督-审批/合规"})
	})
	rg.POST("/approvals", s.CreateApproval)
	rg.GET("/approvals", s.ListApprovals)
	rg.GET("/approvals/:id", s.GetApproval)
	rg.PUT("/approvals/:id/decide", s.DecideApproval)
	rg.POST("/rules", s.CreateRule)
	rg.GET("/rules", s.ListRules)
	rg.POST("/workflows", s.CreateWorkflow)
	rg.GET("/workflows", s.ListWorkflows)
}

func (s *Service) AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&Approval{}, &Rule{}, &Workflow{})
}

type Approval struct {
	ID             uint64         `gorm:"primarykey" json:"id"`
	ApprovalNo     string         `gorm:"uniqueIndex;size:32" json:"approval_no"`
	Type           string         `gorm:"size:32;index" json:"type"`
	Title          string         `gorm:"size:128" json:"title"`
	EntityType     string         `gorm:"size:32" json:"entity_type"`
	EntityID       string         `gorm:"size:32" json:"entity_id"`
	SubmitterID    string         `gorm:"size:32" json:"submitter_id"`
	ApproverID     string         `gorm:"size:32" json:"approver_id,omitempty"`
	Decision       string         `gorm:"size:16;default:pending;index" json:"decision"`
	DecisionReason string         `gorm:"type:text" json:"decision_reason,omitempty"`
	DecidedAt      *time.Time     `json:"decided_at,omitempty"`
	Content        string         `gorm:"type:text" json:"content"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

type Rule struct {
	ID        uint64         `gorm:"primarykey" json:"id"`
	Name      string         `gorm:"size:128" json:"name"`
	Category  string         `gorm:"size:32;index" json:"category"`
	Condition string         `gorm:"type:text" json:"condition"`
	Action    string         `gorm:"size:256" json:"action"`
	Status    string         `gorm:"size:16;default:active" json:"status"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

type Workflow struct {
	ID         uint64         `gorm:"primarykey" json:"id"`
	Name       string         `gorm:"size:128" json:"name"`
	Definition string         `gorm:"type:text" json:"definition"`
	Status     string         `gorm:"size:16;default:active" json:"status"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}

func (s *Service) CreateApproval(c *gin.Context) {
	var a Approval
	c.ShouldBindJSON(&a)
	a.ApprovalNo = fmt.Sprintf("APR%s%d", time.Now().Format("20060102"), time.Now().UnixNano()%10000)
	s.DB().Create(&a)
	response.Created(c, a)
}

func (s *Service) ListApprovals(c *gin.Context) {
	var items []Approval
	var total int64
	q := s.DB().Model(&Approval{})
	if st := c.Query("decision"); st != "" { q = q.Where("decision = ?", st) }
	q.Count(&total)
	paginate(c, q).Order("created_at DESC").Find(&items)
	response.OK(c, gin.H{"items": items, "total": total})
}

func (s *Service) GetApproval(c *gin.Context) {
	var a Approval
	if err := s.DB().First(&a, c.Param("id")).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	response.OK(c, a)
}

func (s *Service) DecideApproval(c *gin.Context) {
	var body struct {
		Approve bool   `json:"approve"`
		Reason  string `json:"reason"`
	}
	c.ShouldBindJSON(&body)
	var a Approval
	s.DB().First(&a, c.Param("id"))
	now := time.Now()
	if body.Approve {
		a.Decision = "approved"
	} else {
		a.Decision = "rejected"
	}
	a.DecisionReason = body.Reason
	a.DecidedAt = &now
	s.DB().Save(&a)
	response.OK(c, a)
}

func (s *Service) CreateRule(c *gin.Context) {
	var r Rule
	c.ShouldBindJSON(&r)
	s.DB().Create(&r)
	response.Created(c, r)
}

func (s *Service) ListRules(c *gin.Context) {
	var items []Rule
	s.DB().Find(&items)
	response.OK(c, items)
}

func (s *Service) CreateWorkflow(c *gin.Context) {
	var w Workflow
	c.ShouldBindJSON(&w)
	s.DB().Create(&w)
	response.Created(c, w)
}

func (s *Service) ListWorkflows(c *gin.Context) {
	var items []Workflow
	s.DB().Find(&items)
	response.OK(c, items)
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
