// Package ims is the capital/investment MBS — ICASE, capital accounts, portfolio.
// ZW-ARC-015: T43 三方会签 — OAS owner + sms + ims for capital deployment.
// ims owns ICASE (Investment Case) and IX (capital account) data.
package ims

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
	return &Service{BaseService: mbs.NewBaseService("ims", "mbs_ims", deps)}
}

func (s *Service) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/health", func(c *gin.Context) {
		response.OK(c, gin.H{"ms": "ims", "status": "ok", "desc": "资本投资服务"})
	})
	// Capital accounts (IX)
	rg.POST("/accounts", s.CreateAccount)
	rg.GET("/accounts", s.ListAccounts)
	rg.GET("/accounts/:id", s.GetAccount)
	// ICASE
	rg.POST("/icases", s.CreateICASE)
	rg.GET("/icases", s.ListICASE)
	rg.GET("/icases/:id", s.GetICASE)
	rg.PUT("/icases/:id", s.UpdateICASE)
	rg.POST("/icases/:id/submit", s.SubmitICASE)
	rg.POST("/icases/:id/approve", s.ApproveICASE) // T43 three-party
	rg.POST("/icases/:id/reject", s.RejectICASE)
	// Portfolio
	rg.GET("/portfolio", s.PortfolioOverview)
}

func (s *Service) AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&CapitalAccount{}, &ICASE{}, &ICASEApproval{}, &InvestmentPosition{})
}

// ===== Models =====

// CapitalAccount — IX 资本账户
type CapitalAccount struct {
	ID            uint64         `gorm:"primarykey" json:"id"`
	AccountNo     string         `gorm:"uniqueIndex;size:32" json:"account_no"`
	Name          string         `gorm:"size:128" json:"name"`
	AccountType   string         `gorm:"size:32;index" json:"account_type"` // fund/individual/entity
	OwnerID       string         `gorm:"size:32;index" json:"owner_id"`
	Currency      string         `gorm:"size:8;default:CNY" json:"currency"`
	Committed     float64        `gorm:"type:decimal(18,2);default:0" json:"committed"`
	Deployed      float64        `gorm:"type:decimal(18,2);default:0" json:"deployed"`
	Returned      float64        `gorm:"type:decimal(18,2);default:0" json:"returned"`
	Status        string         `gorm:"size:16;default:active" json:"status"`
	RiskProfile   string         `gorm:"size:32" json:"risk_profile"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

// ICASE — Investment Case 投资案
type ICASE struct {
	ID             uint64         `gorm:"primarykey" json:"id"`
	CaseNo         string         `gorm:"uniqueIndex;size:32" json:"case_no"`
	Title          string         `gorm:"size:128" json:"title"`
	Description    string         `gorm:"type:text" json:"description"`
	InvestType     string         `gorm:"size:32;index" json:"invest_type"` // equity/debt/convertible/grant
	TargetEntity   string         `gorm:"size:128" json:"target_entity"`
	Amount         float64        `gorm:"type:decimal(18,2)" json:"amount"`
	Currency       string         `gorm:"size:8;default:CNY" json:"currency"`
	Valuation      float64        `gorm:"type:decimal(18,2)" json:"valuation,omitempty"`
	EquityPct      float64        `gorm:"type:decimal(8,4)" json:"equity_pct,omitempty"`
	ExpectedIRR    float64        `gorm:"type:decimal(8,4)" json:"expected_irr,omitempty"`
	TenorMonths    int            `json:"tenor_months,omitempty"`
	// T43: three-party sign-off — OAS owner / sms / ims
	OASApproved    bool           `gorm:"default:false" json:"oas_approved"`
	SMBSApproved   bool           `gorm:"default:false" json:"sms_approved"`
	IMBSApproved   bool           `gorm:"default:false" json:"ims_approved"`
	Status         string         `gorm:"size:32;default:draft;index" json:"status"`
	// draft → submitted → oas_approved → sms_approved → ims_approved → deployed
	//       ↘ rejected at any stage
	AccountID      *uint64        `json:"account_id,omitempty"`
	SubmittedAt    *time.Time     `json:"submitted_at,omitempty"`
	DeployedAt     *time.Time     `json:"deployed_at,omitempty"`
	ExitDate       *time.Time     `json:"exit_date,omitempty"`
	ExitAmount     float64        `gorm:"type:decimal(18,2);default:0" json:"exit_amount"`
	Notes          string         `gorm:"type:text" json:"notes,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

// ICASEApproval — 三方会签记录 (T43)
type ICASEApproval struct {
	ID         uint64    `gorm:"primarykey" json:"id"`
	ICASEID    uint64    `gorm:"index" json:"icase_id"`
	Party      string    `gorm:"size:32" json:"party"` // oas/sms/ims
	Approver   string    `gorm:"size:32" json:"approver"`
	Action     string    `gorm:"size:32" json:"action"` // approved/rejected
	Comment    string    `gorm:"type:text" json:"comment,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// InvestmentPosition — 投资持仓
type InvestmentPosition struct {
	ID          uint64     `gorm:"primarykey" json:"id"`
	ICASEID     uint64     `gorm:"index" json:"icase_id"`
	AccountID   uint64     `gorm:"index" json:"account_id"`
	Amount      float64    `gorm:"type:decimal(18,2)" json:"amount"`
	CurrentVal  float64    `gorm:"type:decimal(18,2)" json:"current_val"`
	UnrealizedPL float64   `gorm:"type:decimal(18,2)" json:"unrealized_pl"`
	Status      string     `gorm:"size:16;default:open" json:"status"`
	OpenedAt    time.Time  `json:"opened_at"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
}

// ===== Handlers =====

func (s *Service) CreateAccount(c *gin.Context) {
	var a CapitalAccount
	if err := c.ShouldBindJSON(&a); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	a.AccountNo = fmt.Sprintf("IX%s%d", time.Now().Format("20060102"), time.Now().UnixNano()%10000)
	s.DB().Create(&a)
	response.Created(c, a)
}

func (s *Service) ListAccounts(c *gin.Context) {
	var items []CapitalAccount
	var total int64
	q := s.DB().Model(&CapitalAccount{})
	if t := c.Query("account_type"); t != "" {
		q = q.Where("account_type = ?", t)
	}
	q.Count(&total)
	paginate(c, q).Order("created_at DESC").Find(&items)
	response.OK(c, gin.H{"items": items, "total": total})
}

func (s *Service) GetAccount(c *gin.Context) {
	var a CapitalAccount
	if err := s.DB().First(&a, c.Param("id")).Error; err != nil {
		response.NotFound(c, "account not found")
		return
	}
	response.OK(c, a)
}

func (s *Service) CreateICASE(c *gin.Context) {
	var ic ICASE
	if err := c.ShouldBindJSON(&ic); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	ic.CaseNo = fmt.Sprintf("IC%s%d", time.Now().Format("20060102"), time.Now().UnixNano()%10000)
	ic.Status = "draft"
	s.DB().Create(&ic)
	response.Created(c, ic)
}

func (s *Service) ListICASE(c *gin.Context) {
	var items []ICASE
	var total int64
	q := s.DB().Model(&ICASE{})
	if st := c.Query("status"); st != "" {
		q = q.Where("status = ?", st)
	}
	if t := c.Query("invest_type"); t != "" {
		q = q.Where("invest_type = ?", t)
	}
	q.Count(&total)
	paginate(c, q).Order("created_at DESC").Find(&items)
	response.OK(c, gin.H{"items": items, "total": total})
}

func (s *Service) GetICASE(c *gin.Context) {
	var ic ICASE
	if err := s.DB().First(&ic, c.Param("id")).Error; err != nil {
		response.NotFound(c, "ICASE not found")
		return
	}
	response.OK(c, ic)
}

func (s *Service) UpdateICASE(c *gin.Context) {
	var ic ICASE
	if err := s.DB().First(&ic, c.Param("id")).Error; err != nil {
		response.NotFound(c, "ICASE not found")
		return
	}
	c.ShouldBindJSON(&ic)
	s.DB().Save(&ic)
	response.OK(c, ic)
}

func (s *Service) SubmitICASE(c *gin.Context) {
	var ic ICASE
	if err := s.DB().First(&ic, c.Param("id")).Error; err != nil {
		response.NotFound(c, "ICASE not found")
		return
	}
	if ic.Status != "draft" && ic.Status != "rejected" {
		response.Conflict(c, "can only submit from draft/rejected")
		return
	}
	now := time.Now()
	ic.Status = "submitted"
	ic.SubmittedAt = &now
	s.DB().Save(&ic)
	response.OK(c, ic)
}

// ApproveICASE handles T43 three-party sign-off.
// party must be one of: oas, sms, ims
func (s *Service) ApproveICASE(c *gin.Context) {
	var req struct {
		Party   string `json:"party" binding:"required"`
		Comment string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "party required (oas/sms/ims)")
		return
	}
	var ic ICASE
	if err := s.DB().First(&ic, c.Param("id")).Error; err != nil {
		response.NotFound(c, "ICASE not found")
		return
	}
	approver := c.GetHeader("X-User-ID")
	if approver == "" {
		approver = "system"
	}

	rec := ICASEApproval{ICASEID: ic.ID, Party: req.Party, Approver: approver, Action: "approved", Comment: req.Comment}
	s.DB().Create(&rec)

	switch req.Party {
	case "oas":
		ic.OASApproved = true
		if ic.Status == "submitted" {
			ic.Status = "oas_approved"
		}
	case "sms":
		ic.SMBSApproved = true
		if ic.OASApproved && ic.Status == "oas_approved" {
			ic.Status = "sms_approved"
		}
	case "ims":
		ic.IMBSApproved = true
		if ic.OASApproved && ic.SMBSApproved {
			ic.Status = "deployed"
			now := time.Now()
			ic.DeployedAt = &now
			// Create position
			if ic.AccountID != nil {
				s.DB().Create(&InvestmentPosition{
					ICASEID:   ic.ID,
					AccountID: *ic.AccountID,
					Amount:    ic.Amount,
					CurrentVal: ic.Amount,
					Status:    "open",
					OpenedAt:  now,
				})
			}
		}
	default:
		response.BadRequest(c, "invalid party, must be oas/sms/ims")
		return
	}
	s.DB().Save(&ic)
	response.OK(c, ic)
}

func (s *Service) RejectICASE(c *gin.Context) {
	var req struct {
		Party   string `json:"party" binding:"required"`
		Comment string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "party required")
		return
	}
	var ic ICASE
	if err := s.DB().First(&ic, c.Param("id")).Error; err != nil {
		response.NotFound(c, "ICASE not found")
		return
	}
	approver := c.GetHeader("X-User-ID")
	if approver == "" {
		approver = "system"
	}
	s.DB().Create(&ICASEApproval{
		ICASEID: ic.ID, Party: req.Party, Approver: approver,
		Action: "rejected", Comment: req.Comment,
	})
	ic.Status = "rejected"
	s.DB().Save(&ic)
	response.OK(c, ic)
}

func (s *Service) PortfolioOverview(c *gin.Context) {
	var totalCommitted, totalDeployed, totalReturned float64
	s.DB().Model(&CapitalAccount{}).Select("COALESCE(SUM(committed),0)").Scan(&totalCommitted)
	s.DB().Model(&CapitalAccount{}).Select("COALESCE(SUM(deployed),0)").Scan(&totalDeployed)
	s.DB().Model(&CapitalAccount{}).Select("COALESCE(SUM(returned),0)").Scan(&totalReturned)

	var draft, submitted, deployed, exited int64
	s.DB().Model(&ICASE{}).Where("status = ?", "draft").Count(&draft)
	s.DB().Model(&ICASE{}).Where("status = ?", "submitted").Count(&submitted)
	s.DB().Model(&ICASE{}).Where("status = ?", "deployed").Count(&deployed)
	s.DB().Model(&InvestmentPosition{}).Where("status = ?", "closed").Count(&exited)

	response.OK(c, gin.H{
		"capital": gin.H{
			"committed": totalCommitted, "deployed": totalDeployed, "returned": totalReturned,
			"deployable": totalCommitted - totalDeployed,
		},
		"icases": gin.H{
			"draft": draft, "submitted": submitted, "deployed": deployed, "exited": exited,
		},
	})
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
