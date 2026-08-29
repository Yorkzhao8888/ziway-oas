// Package vms is the value-operations MBS — VCASE lifecycle, budget, operations monitoring.
// vms is the authoritative source for VCASE (Value Case).
// ZW-ARC-015: vms publishes VCASE_DRAFT_CREATED / VCASE_SUBMITTED /
// VCASE_EXECUTION_UPDATED / VCASE_COMPLETION_SUBMITTED.
// oms publishes VCASE_APPROVED / VCASE_REJECTED.
// Saga: VCASE_APPROVED but vms execution fails → VCASE_EXECUTION_FAILED → oms rollback.
// Approval timeout: 72h → VCASE_APPROVAL_TIMEOUT.
package vms

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"ziway/backend/internal/mbs"
	"ziway/backend/pkg/response"
)

type Service struct{ mbs.BaseService }

func New(deps mbs.Dependencies) *Service {
	return &Service{BaseService: mbs.NewBaseService("vms", "mbs_vms", deps)}
}

func (s *Service) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/health", func(c *gin.Context) {
		response.OK(c, gin.H{"ms": "vms", "status": "ok", "desc": "价值运营服务（VCASE权威源）"})
	})
	// VCASE lifecycle
	rg.POST("/vcases", s.CreateVCASE)
	rg.GET("/vcases", s.ListVCASE)
	rg.GET("/vcases/:id", s.GetVCASE)
	rg.PUT("/vcases/:id", s.UpdateVCASE)
	rg.POST("/vcases/:id/submit", s.SubmitVCASE)
	rg.POST("/vcases/:id/execution", s.UpdateExecution)
	rg.POST("/vcases/:id/complete", s.SubmitCompletion)
	rg.POST("/vcases/:id/fail", s.MarkFailed)
	// Budget
	rg.POST("/budgets", s.CreateBudget)
	rg.GET("/budgets", s.ListBudgets)
	rg.PUT("/budgets/:id", s.UpdateBudget)
	// Ops monitor
	rg.GET("/ops/overview", s.OpsOverview)
}

func (s *Service) AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&VCASE{}, &Budget{}, &ExecutionLog{}, &OpsMetric{})
}

// ===== Models =====

// VCASE — Value Case，价值运营案
type VCASE struct {
	ID            uint64         `gorm:"primarykey" json:"id"`
	CaseNo        string         `gorm:"uniqueIndex;size:32" json:"case_no"`
	Title         string         `gorm:"size:128" json:"title"`
	Description   string         `gorm:"type:text" json:"description"`
	CaseType      string         `gorm:"size:32;index" json:"case_type"` // growth/efficiency/risk/innovation
	Priority      string         `gorm:"size:16;default:P2" json:"priority"`
	Status        string         `gorm:"size:32;default:draft;index" json:"status"`
	// draft → submitted → approved → executing → completed → closed
	//                ↘ rejected    ↘ failed (rollback)
	OwnerID       string         `gorm:"size:32;index" json:"owner_id"`
	OwnerName     string         `gorm:"size:64" json:"owner_name"`
	OrgUnit       string         `gorm:"size:64" json:"org_unit"`
	BudgetID      *uint64        `json:"budget_id,omitempty"`
	PlannedStart  *time.Time     `json:"planned_start,omitempty"`
	PlannedEnd    *time.Time     `json:"planned_end,omitempty"`
	ActualStart   *time.Time     `json:"actual_start,omitempty"`
	ActualEnd     *time.Time     `json:"actual_end,omitempty"`
	// KPIs & targets stored as JSON
	Targets       string         `gorm:"type:text" json:"targets"`
	// Execution progress 0-100
	Progress      int            `gorm:"default:0" json:"progress"`
	// Approval tracking
	ApprovedBy    string         `gorm:"size:32" json:"approved_by,omitempty"`
	ApprovedAt    *time.Time     `json:"approved_at,omitempty"`
	RejectReason  string         `gorm:"type:text" json:"reject_reason,omitempty"`
	SubmittedAt   *time.Time     `json:"submitted_at,omitempty"`
	// Completion
	CompletionNote string        `gorm:"type:text" json:"completion_note,omitempty"`
	CompletedAt   *time.Time     `json:"completed_at,omitempty"`
	FailureReason string         `gorm:"type:text" json:"failure_reason,omitempty"`
	// Timestamps
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

// Budget — VCASE 预算
type Budget struct {
	ID          uint64         `gorm:"primarykey" json:"id"`
	BudgetNo    string         `gorm:"uniqueIndex;size:32" json:"budget_no"`
	CaseID      *uint64        `gorm:"index" json:"case_id,omitempty"`
	FiscalYear  int            `json:"fiscal_year"`
	Category    string         `gorm:"size:32;index" json:"category"`
	Amount      float64        `gorm:"type:decimal(16,2)" json:"amount"`
	Used        float64        `gorm:"type:decimal(16,2);default:0" json:"used"`
	Currency    string         `gorm:"size:8;default:CNY" json:"currency"`
	Status      string         `gorm:"size:16;default:draft" json:"status"`
	ApprovedBy  string         `gorm:"size:32" json:"approved_by,omitempty"`
	Note        string         `gorm:"type:text" json:"note,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// ExecutionLog — VCASE 执行日志
type ExecutionLog struct {
	ID        uint64    `gorm:"primarykey" json:"id"`
	CaseID    uint64    `gorm:"index" json:"case_id"`
	Action    string    `gorm:"size:64" json:"action"`
	Detail    string    `gorm:"type:text" json:"detail"`
	Operator  string    `gorm:"size:32" json:"operator"`
	CreatedAt time.Time `json:"created_at"`
}

// OpsMetric — 运营监控指标快照
type OpsMetric struct {
	ID        uint64    `gorm:"primarykey" json:"id"`
	MetricKey string    `gorm:"size:64;index" json:"metric_key"`
	MetricVal float64   `json:"metric_val"`
	Dimension string    `gorm:"size:32" json:"dimension"`
	RecordedAt time.Time `json:"recorded_at"`
}

// ===== Handlers =====

func (s *Service) CreateVCASE(c *gin.Context) {
	var v VCASE
	if err := c.ShouldBindJSON(&v); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	v.CaseNo = fmt.Sprintf("VC%s%d", time.Now().Format("20060102"), time.Now().UnixNano()%10000)
	v.Status = "draft"
	if err := s.DB().Create(&v).Error; err != nil {
		response.InternalError(c, "failed to create VCASE")
		return
	}
	s.logExecution(v.ID, "created", "VCASE created", v.OwnerID)
	s.publish("VCASE_DRAFT_CREATED", v)
	response.Created(c, v)
}

func (s *Service) ListVCASE(c *gin.Context) {
	var items []VCASE
	var total int64
	q := s.DB().Model(&VCASE{})
	if st := c.Query("status"); st != "" {
		q = q.Where("status = ?", st)
	}
	if ct := c.Query("case_type"); ct != "" {
		q = q.Where("case_type = ?", ct)
	}
	if ow := c.Query("owner_id"); ow != "" {
		q = q.Where("owner_id = ?", ow)
	}
	q.Count(&total)
	paginate(c, q).Order("created_at DESC").Find(&items)
	response.OK(c, gin.H{"items": items, "total": total})
}

func (s *Service) GetVCASE(c *gin.Context) {
	var v VCASE
	if err := s.DB().First(&v, c.Param("id")).Error; err != nil {
		response.NotFound(c, "VCASE not found")
		return
	}
	response.OK(c, v)
}

func (s *Service) UpdateVCASE(c *gin.Context) {
	var v VCASE
	if err := s.DB().First(&v, c.Param("id")).Error; err != nil {
		response.NotFound(c, "VCASE not found")
		return
	}
	c.ShouldBindJSON(&v)
	s.DB().Save(&v)
	response.OK(c, v)
}

func (s *Service) SubmitVCASE(c *gin.Context) {
	var v VCASE
	if err := s.DB().First(&v, c.Param("id")).Error; err != nil {
		response.NotFound(c, "VCASE not found")
		return
	}
	if v.Status != "draft" && v.Status != "rejected" {
		response.Conflict(c, "can only submit from draft or rejected status")
		return
	}
	now := time.Now()
	v.Status = "submitted"
	v.SubmittedAt = &now
	s.DB().Save(&v)
	s.logExecution(v.ID, "submitted", "VCASE submitted for approval", v.OwnerID)
	s.publish("VCASE_SUBMITTED", v)
	response.OK(c, v)
}

func (s *Service) UpdateExecution(c *gin.Context) {
	var req struct {
		Progress int    `json:"progress"`
		Note     string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	var v VCASE
	if err := s.DB().First(&v, c.Param("id")).Error; err != nil {
		response.NotFound(c, "VCASE not found")
		return
	}
	if v.Status != "approved" && v.Status != "executing" {
		response.Conflict(c, "VCASE not in executable status")
		return
	}
	v.Status = "executing"
	v.Progress = req.Progress
	now := time.Now()
	if v.ActualStart == nil {
		v.ActualStart = &now
	}
	s.DB().Save(&v)
	s.logExecution(v.ID, "execution_updated",
		fmt.Sprintf("progress=%d, note=%s", req.Progress, req.Note), v.OwnerID)
	s.publish("VCASE_EXECUTION_UPDATED", v)
	response.OK(c, v)
}

func (s *Service) SubmitCompletion(c *gin.Context) {
	var req struct {
		Note string `json:"note"`
	}
	c.ShouldBindJSON(&req)
	var v VCASE
	if err := s.DB().First(&v, c.Param("id")).Error; err != nil {
		response.NotFound(c, "VCASE not found")
		return
	}
	now := time.Now()
	v.Status = "completed"
	v.Progress = 100
	v.CompletionNote = req.Note
	v.ActualEnd = &now
	v.CompletedAt = &now
	s.DB().Save(&v)
	s.logExecution(v.ID, "completed", req.Note, v.OwnerID)
	s.publish("VCASE_COMPLETION_SUBMITTED", v)
	response.OK(c, v)
}

func (s *Service) MarkFailed(c *gin.Context) {
	var req struct {
		Reason string `json:"reason"`
	}
	c.ShouldBindJSON(&req)
	var v VCASE
	if err := s.DB().First(&v, c.Param("id")).Error; err != nil {
		response.NotFound(c, "VCASE not found")
		return
	}
	v.Status = "failed"
	v.FailureReason = req.Reason
	s.DB().Save(&v)
	s.logExecution(v.ID, "failed", req.Reason, v.OwnerID)
	// Saga compensation event
	s.publish("VCASE_EXECUTION_FAILED", v)
	response.OK(c, v)
}

func (s *Service) CreateBudget(c *gin.Context) {
	var b Budget
	if err := c.ShouldBindJSON(&b); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	b.BudgetNo = fmt.Sprintf("BDG%s%d", time.Now().Format("20060102"), time.Now().UnixNano()%10000)
	s.DB().Create(&b)
	response.Created(c, b)
}

func (s *Service) ListBudgets(c *gin.Context) {
	var items []Budget
	var total int64
	q := s.DB().Model(&Budget{})
	if yr := c.Query("fiscal_year"); yr != "" {
		q = q.Where("fiscal_year = ?", yr)
	}
	q.Count(&total)
	paginate(c, q).Order("created_at DESC").Find(&items)
	response.OK(c, gin.H{"items": items, "total": total})
}

func (s *Service) UpdateBudget(c *gin.Context) {
	var b Budget
	if err := s.DB().First(&b, c.Param("id")).Error; err != nil {
		response.NotFound(c, "budget not found")
		return
	}
	c.ShouldBindJSON(&b)
	s.DB().Save(&b)
	response.OK(c, b)
}

func (s *Service) OpsOverview(c *gin.Context) {
	var draft, submitted, approved, executing, completed, failed int64
	s.DB().Model(&VCASE{}).Where("status = ?", "draft").Count(&draft)
	s.DB().Model(&VCASE{}).Where("status = ?", "submitted").Count(&submitted)
	s.DB().Model(&VCASE{}).Where("status = ?", "approved").Count(&approved)
	s.DB().Model(&VCASE{}).Where("status = ?", "executing").Count(&executing)
	s.DB().Model(&VCASE{}).Where("status = ?", "completed").Count(&completed)
	s.DB().Model(&VCASE{}).Where("status = ?", "failed").Count(&failed)

	var totalBudget, usedBudget float64
	s.DB().Model(&Budget{}).Select("COALESCE(SUM(amount),0)").Scan(&totalBudget)
	s.DB().Model(&Budget{}).Select("COALESCE(SUM(used),0)").Scan(&usedBudget)

	response.OK(c, gin.H{
		"vcase_by_status": gin.H{
			"draft": draft, "submitted": submitted, "approved": approved,
			"executing": executing, "completed": completed, "failed": failed,
		},
		"budget": gin.H{
			"total": totalBudget, "used": usedBudget,
			"remaining": totalBudget - usedBudget,
		},
	})
}

// ===== Helpers =====

func (s *Service) logExecution(caseID uint64, action, detail, operator string) {
	s.DB().Create(&ExecutionLog{
		CaseID:   caseID,
		Action:   action,
		Detail:   detail,
		Operator: operator,
	})
}

func (s *Service) publish(topic string, payload interface{}) {
	// P0: in-process event via EventBus; P1: Kafka. Best-effort, non-blocking.
	go func() {
		data, err := json.Marshal(payload)
		if err != nil {
			s.Log().Error("marshal VCASE event", zap.String("topic", topic), zap.Error(err))
			return
		}
		if err := s.Events().Publish(topic, data); err != nil {
			s.Log().Error("publish VCASE event", zap.String("topic", topic), zap.Error(err))
		}
	}()
}

func paginate(c *gin.Context, db *gorm.DB) *gorm.DB {
	p, _ := parseInt(c.DefaultQuery("page", "1"))
	sz, _ := parseInt(c.DefaultQuery("size", "20"))
	if p < 1 {
		p = 1
	}
	if sz < 1 || sz > 100 {
		sz = 20
	}
	return db.Offset((p - 1) * sz).Limit(sz)
}

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
