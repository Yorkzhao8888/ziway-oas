// Package fms is the finance MBS — GP engine, ledger, FCASE, payments.
package fms

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
	return &Service{BaseService: mbs.NewBaseService("fms", "mbs_fms", deps)}
}

func (s *Service) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/health", func(c *gin.Context) {
		response.OK(c, gin.H{"ms": "fms", "status": "ok", "desc": "财务服务-GP分润/账务"})
	})
	rg.POST("/ledger", s.CreateEntry)
	rg.GET("/ledger", s.ListEntries)
	rg.POST("/payments", s.CreatePayment)
	rg.GET("/payments", s.ListPayments)
	rg.POST("/invoices", s.CreateInvoice)
	rg.GET("/invoices", s.ListInvoices)
	rg.POST("/fcases", s.CreateFCase)
	rg.GET("/fcases", s.ListFCases)
	rg.PUT("/fcases/:id/approve", s.ApproveFCase)
	rg.POST("/gp/calculate", s.CalculateGP)
}

func (s *Service) AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&LedgerEntry{}, &Payment{}, &Invoice{}, &FCase{})
}

type LedgerEntry struct {
	ID          uint64         `gorm:"primarykey" json:"id"`
	EntryNo     string         `gorm:"uniqueIndex;size:32" json:"entry_no"`
	AccountCode string         `gorm:"size:32;index" json:"account_code"`
	Debit       float64        `gorm:"default:0" json:"debit"`
	Credit      float64        `gorm:"default:0" json:"credit"`
	Summary     string         `gorm:"size:256" json:"summary"`
	RefType     string         `gorm:"size:32" json:"ref_type"`
	RefID       string         `gorm:"size:32" json:"ref_id"`
	Period      string         `gorm:"size:16;index" json:"period"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

type Payment struct {
	ID            uint64         `gorm:"primarykey" json:"id"`
	PaymentNo     string         `gorm:"uniqueIndex;size:32" json:"payment_no"`
	PayerID       string         `gorm:"size:32;index" json:"payer_id"`
	PayeeID       string         `gorm:"size:32;index" json:"payee_id"`
	Amount        float64        `json:"amount"`
	Currency      string         `gorm:"size:8;default:CNY" json:"currency"`
	Method        string         `gorm:"size:32" json:"method"`
	Status        string         `gorm:"size:16;default:pending" json:"status"`
	TransactionID string         `gorm:"size:64" json:"transaction_id"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

type Invoice struct {
	ID         uint64         `gorm:"primarykey" json:"id"`
	InvoiceNo  string         `gorm:"uniqueIndex;size:32" json:"invoice_no"`
	Title      string         `gorm:"size:128" json:"title"`
	TaxNumber  string         `gorm:"size:32" json:"tax_number"`
	Amount     float64        `json:"amount"`
	Tax        float64        `json:"tax"`
	Status     string         `gorm:"size:16;default:draft" json:"status"`
	IssuedAt   *time.Time     `json:"issued_at,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}

type FCase struct {
	ID          uint64         `gorm:"primarykey" json:"id"`
	FCaseNo     string         `gorm:"uniqueIndex;size:32" json:"fcase_no"`
	Title       string         `gorm:"size:128" json:"title"`
	Type        string         `gorm:"size:32" json:"type"`
	Amount      float64        `json:"amount"`
	Description string         `gorm:"type:text" json:"description"`
	Status      string         `gorm:"size:16;default:draft" json:"status"`
	ApprovedBy  string         `gorm:"size:32" json:"approved_by,omitempty"`
	ApprovedAt  *time.Time     `json:"approved_at,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (s *Service) CreateEntry(c *gin.Context) {
	var e LedgerEntry
	c.ShouldBindJSON(&e)
	e.EntryNo = fmt.Sprintf("LE%s%d", time.Now().Format("20060102"), time.Now().UnixNano()%10000)
	s.DB().Create(&e)
	response.Created(c, e)
}

func (s *Service) ListEntries(c *gin.Context) {
	var items []LedgerEntry
	var total int64
	q := s.DB().Model(&LedgerEntry{})
	if p := c.Query("period"); p != "" { q = q.Where("period = ?", p) }
	q.Count(&total)
	paginate(c, q).Order("created_at DESC").Find(&items)
	response.OK(c, gin.H{"items": items, "total": total})
}

func (s *Service) CreatePayment(c *gin.Context) {
	var p Payment
	c.ShouldBindJSON(&p)
	p.PaymentNo = fmt.Sprintf("PAY%s%d", time.Now().Format("20060102"), time.Now().UnixNano()%10000)
	s.DB().Create(&p)
	response.Created(c, p)
}

func (s *Service) ListPayments(c *gin.Context) {
	var items []Payment
	s.DB().Order("created_at DESC").Limit(50).Find(&items)
	response.OK(c, items)
}

func (s *Service) CreateInvoice(c *gin.Context) {
	var i Invoice
	c.ShouldBindJSON(&i)
	i.InvoiceNo = fmt.Sprintf("INV%s%d", time.Now().Format("20060102"), time.Now().UnixNano()%10000)
	s.DB().Create(&i)
	response.Created(c, i)
}

func (s *Service) ListInvoices(c *gin.Context) {
	var items []Invoice
	s.DB().Order("created_at DESC").Limit(50).Find(&items)
	response.OK(c, items)
}

func (s *Service) CreateFCase(c *gin.Context) {
	var f FCase
	c.ShouldBindJSON(&f)
	f.FCaseNo = fmt.Sprintf("FCASE%s%d", time.Now().Format("20060102"), time.Now().UnixNano()%10000)
	s.DB().Create(&f)
	response.Created(c, f)
}

func (s *Service) ListFCases(c *gin.Context) {
	var items []FCase
	var total int64
	q := s.DB().Model(&FCase{})
	if st := c.Query("status"); st != "" { q = q.Where("status = ?", st) }
	q.Count(&total)
	paginate(c, q).Order("created_at DESC").Find(&items)
	response.OK(c, gin.H{"items": items, "total": total})
}

func (s *Service) ApproveFCase(c *gin.Context) {
	var f FCase
	s.DB().First(&f, c.Param("id"))
	now := time.Now()
	f.Status = "approved"
	f.ApprovedAt = &now
	s.DB().Save(&f)
	response.OK(c, f)
}

func (s *Service) CalculateGP(c *gin.Context) {
	var body struct {
		Revenue float64 `json:"revenue"`
		Cost    float64 `json:"cost"`
	}
	c.ShouldBindJSON(&body)
	gp := body.Revenue - body.Cost
	gpRate := 0.0
	if body.Revenue > 0 { gpRate = gp / body.Revenue * 100 }
	response.OK(c, gin.H{
		"revenue": body.Revenue, "cost": body.Cost,
		"gp": gp, "gp_rate": gpRate,
		"shares": gin.H{
			"du": gp * 0.5, "hu": gp * 0.2, "ou": gp * 0.15, "iu": gp * 0.15,
		},
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
