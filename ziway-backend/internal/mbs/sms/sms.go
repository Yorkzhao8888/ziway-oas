// Package sms is the incubation MBS — SX incubator, SU seed terminals, revolving door to dms (T48).
// ZW-ARC-015: sms incubation data is in an independent schema. When a project
// "graduates" (revolving door T48), data migrates to dms and the entity gets a T56 prefixed operator code.
package sms

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
	return &Service{BaseService: mbs.NewBaseService("sms", "mbs_sms", deps)}
}

func (s *Service) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/health", func(c *gin.Context) {
		response.OK(c, gin.H{"ms": "sms", "status": "ok", "desc": "孵化服务"})
	})
	// SX — incubation projects
	rg.POST("/projects", s.CreateProject)
	rg.GET("/projects", s.ListProjects)
	rg.GET("/projects/:id", s.GetProject)
	rg.PUT("/projects/:id", s.UpdateProject)
	rg.POST("/projects/:id/stage", s.AdvanceStage)
	// SU — seed terminals
	rg.POST("/terminals", s.CreateTerminal)
	rg.GET("/terminals", s.ListTerminals)
	rg.POST("/terminals/:id/activate", s.ActivateTerminal)
	// Revolving door T48
	rg.POST("/projects/:id/graduate", s.GraduateProject)
	rg.GET("/graduates", s.ListGraduates)
}

func (s *Service) AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&IncubationProject{}, &SeedTerminal{}, &StageRecord{}, &GraduateRecord{})
}

// ===== Models =====

// IncubationProject — SX 孵化项目
type IncubationProject struct {
	ID            uint64         `gorm:"primarykey" json:"id"`
	ProjectNo     string         `gorm:"uniqueIndex;size:32" json:"project_no"`
	Name          string         `gorm:"size:128" json:"name"`
	Description   string         `gorm:"type:text" json:"description"`
	Category      string         `gorm:"size:32;index" json:"category"`
	Stage         string         `gorm:"size:32;default:seed;index" json:"stage"`
	// seed → sprout → growth → graduate → (migrated to dms)
	//                ↘ terminated
	FounderID     string         `gorm:"size:32;index" json:"founder_id"`
	FounderName   string         `gorm:"size:64" json:"founder_name"`
	MentorID      string         `gorm:"size:32" json:"mentor_id,omitempty"`
	Budget        float64        `gorm:"type:decimal(14,2);default:0" json:"budget"`
	Used          float64        `gorm:"type:decimal(14,2);default:0" json:"used"`
	KPIs          string         `gorm:"type:text" json:"kpis"` // JSON
	// Graduation tracking (T48 revolving door)
	Graduated     bool           `gorm:"default:false" json:"graduated"`
	GraduatedAt   *time.Time     `json:"graduated_at,omitempty"`
	DMBSCode      string         `gorm:"size:32" json:"dms_code,omitempty"` // T56 prefixed code after migration
	Status        string         `gorm:"size:16;default:active" json:"status"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

// SeedTerminal — SU 种子终端
type SeedTerminal struct {
	ID           uint64         `gorm:"primarykey" json:"id"`
	TerminalNo   string         `gorm:"uniqueIndex;size:32" json:"terminal_no"`
	Name         string         `gorm:"size:128" json:"name"`
	ProjectID    *uint64        `gorm:"index" json:"project_id,omitempty"`
	Type         string         `gorm:"size:32;index" json:"type"` // pos/kiosk/mobile/online
	Location     string         `gorm:"size:256" json:"location"`
	OperatorID   string         `gorm:"size:32" json:"operator_id"`
	Status       string         `gorm:"size:16;default:pending;index" json:"status"` // pending/active/suspended/retired
	ActivatedAt  *time.Time     `json:"activated_at,omitempty"`
	Metadata     string         `gorm:"type:text" json:"metadata"` // JSON
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// StageRecord — 孵化阶段变更记录
type StageRecord struct {
	ID         uint64    `gorm:"primarykey" json:"id"`
	ProjectID  uint64    `gorm:"index" json:"project_id"`
	FromStage  string    `gorm:"size:32" json:"from_stage"`
	ToStage    string    `gorm:"size:32" json:"to_stage"`
	Note       string    `gorm:"type:text" json:"note,omitempty"`
	Operator   string    `gorm:"size:32" json:"operator"`
	CreatedAt  time.Time `json:"created_at"`
}

// GraduateRecord — T48 旋转门记录
type GraduateRecord struct {
	ID           uint64    `gorm:"primarykey" json:"id"`
	ProjectID    uint64    `gorm:"uniqueIndex" json:"project_id"`
	ProjectNo    string    `gorm:"size:32" json:"project_no"`
	DMBSCode     string    `gorm:"size:32" json:"dms_code"` // T56 prefixed
	MigratedData string    `gorm:"type:text" json:"migrated_data"`
	ApprovedBy   string    `gorm:"size:32" json:"approved_by"`
	CreatedAt    time.Time `json:"created_at"`
}

// ===== Handlers =====

func (s *Service) CreateProject(c *gin.Context) {
	var p IncubationProject
	if err := c.ShouldBindJSON(&p); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	p.ProjectNo = fmt.Sprintf("SX%s%d", time.Now().Format("20060102"), time.Now().UnixNano()%10000)
	p.Stage = "seed"
	s.DB().Create(&p)
	response.Created(c, p)
}

func (s *Service) ListProjects(c *gin.Context) {
	var items []IncubationProject
	var total int64
	q := s.DB().Model(&IncubationProject{})
	if st := c.Query("stage"); st != "" {
		q = q.Where("stage = ?", st)
	}
	if g := c.Query("graduated"); g == "true" {
		q = q.Where("graduated = ?", true)
	}
	q.Count(&total)
	paginate(c, q).Order("created_at DESC").Find(&items)
	response.OK(c, gin.H{"items": items, "total": total})
}

func (s *Service) GetProject(c *gin.Context) {
	var p IncubationProject
	if err := s.DB().First(&p, c.Param("id")).Error; err != nil {
		response.NotFound(c, "project not found")
		return
	}
	response.OK(c, p)
}

func (s *Service) UpdateProject(c *gin.Context) {
	var p IncubationProject
	if err := s.DB().First(&p, c.Param("id")).Error; err != nil {
		response.NotFound(c, "project not found")
		return
	}
	c.ShouldBindJSON(&p)
	s.DB().Save(&p)
	response.OK(c, p)
}

func (s *Service) AdvanceStage(c *gin.Context) {
	var req struct {
		ToStage string `json:"to_stage" binding:"required"`
		Note    string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "to_stage required")
		return
	}
	var p IncubationProject
	if err := s.DB().First(&p, c.Param("id")).Error; err != nil {
		response.NotFound(c, "project not found")
		return
	}
	validStages := map[string]bool{"seed": true, "sprout": true, "growth": true, "graduate": true, "terminated": true}
	if !validStages[req.ToStage] {
		response.BadRequest(c, "invalid stage")
		return
	}
	operator := c.GetHeader("X-User-ID")
	if operator == "" {
		operator = "system"
	}
	s.DB().Create(&StageRecord{
		ProjectID: p.ID, FromStage: p.Stage, ToStage: req.ToStage,
		Note: req.Note, Operator: operator,
	})
	p.Stage = req.ToStage
	if req.ToStage == "terminated" {
		p.Status = "terminated"
	}
	s.DB().Save(&p)
	response.OK(c, p)
}

func (s *Service) CreateTerminal(c *gin.Context) {
	var t SeedTerminal
	if err := c.ShouldBindJSON(&t); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	t.TerminalNo = fmt.Sprintf("SU%s%d", time.Now().Format("20060102"), time.Now().UnixNano()%10000)
	t.Status = "pending"
	s.DB().Create(&t)
	response.Created(c, t)
}

func (s *Service) ListTerminals(c *gin.Context) {
	var items []SeedTerminal
	var total int64
	q := s.DB().Model(&SeedTerminal{})
	if st := c.Query("status"); st != "" {
		q = q.Where("status = ?", st)
	}
	q.Count(&total)
	paginate(c, q).Order("created_at DESC").Find(&items)
	response.OK(c, gin.H{"items": items, "total": total})
}

func (s *Service) ActivateTerminal(c *gin.Context) {
	var t SeedTerminal
	if err := s.DB().First(&t, c.Param("id")).Error; err != nil {
		response.NotFound(c, "terminal not found")
		return
	}
	now := time.Now()
	t.Status = "active"
	t.ActivatedAt = &now
	s.DB().Save(&t)
	response.OK(c, t)
}

// GraduateProject implements T48 revolving door — migrates SX project to dms
// with a T56-prefixed operator code. In P0 this records the migration intent;
// actual cross-schema data migration is done by dms in P1.
func (s *Service) GraduateProject(c *gin.Context) {
	var p IncubationProject
	if err := s.DB().First(&p, c.Param("id")).Error; err != nil {
		response.NotFound(c, "project not found")
		return
	}
	if p.Stage != "growth" && p.Stage != "graduate" {
		response.Conflict(c, "project must be in growth/graduate stage")
		return
	}
	if p.Graduated {
		response.Conflict(c, "project already graduated")
		return
	}
	approvedBy := c.GetHeader("X-User-ID")
	if approvedBy == "" {
		approvedBy = "system"
	}
	// Generate T56 prefixed dms operator code
	dmsCode := fmt.Sprintf("T56-DP-%s%d", time.Now().Format("20060102"), time.Now().UnixNano()%10000)
	now := time.Now()
	p.Graduated = true
	p.GraduatedAt = &now
	p.DMBSCode = dmsCode
	p.Stage = "graduate"
	s.DB().Save(&p)

	s.DB().Create(&GraduateRecord{
		ProjectID:  p.ID,
		ProjectNo:  p.ProjectNo,
		DMBSCode:   dmsCode,
		ApprovedBy: approvedBy,
	})
	response.OK(c, gin.H{
		"project_id": p.ID,
		"project_no": p.ProjectNo,
		"dms_code":  dmsCode,
		"message":    "T48 revolving door: graduated to dms",
	})
}

func (s *Service) ListGraduates(c *gin.Context) {
	var items []GraduateRecord
	var total int64
	q := s.DB().Model(&GraduateRecord{})
	q.Count(&total)
	paginate(c, q).Order("created_at DESC").Find(&items)
	response.OK(c, gin.H{"items": items, "total": total})
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
