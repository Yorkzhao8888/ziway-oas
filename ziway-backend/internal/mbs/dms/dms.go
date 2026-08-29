// Package dms is the business operation MBS — store management, DX store chief, GP calculation.
package dms

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
	return &Service{BaseService: mbs.NewBaseService("dms", "mbs_dms", deps)}
}

func (s *Service) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/health", func(c *gin.Context) {
		response.OK(c, gin.H{"ms": "dms", "status": "ok", "desc": "经营服务-事业场/店长/GP"})
	})
	rg.POST("/stores", s.CreateStore)
	rg.GET("/stores", s.ListStores)
	rg.GET("/stores/:id", s.GetStore)
	rg.PUT("/stores/:id", s.UpdateStore)
	rg.GET("/stores/:id/gp", s.CalculateGP)
	rg.POST("/kpis", s.CreateKPI)
	rg.GET("/kpis", s.ListKPIs)
}

func (s *Service) AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&Store{}, &KPI{})
}

type Store struct {
	ID        uint64         `gorm:"primarykey" json:"id"`
	StoreCode string         `gorm:"uniqueIndex;size:32" json:"store_code"`
	Name      string         `gorm:"size:128" json:"name"`
	Prefix    string         `gorm:"size:8;index" json:"prefix"` // T56 强制事业场前缀
	Type      string         `gorm:"size:32" json:"type"`
	ManagerID string         `gorm:"size:32" json:"manager_id"`
	Address   string         `gorm:"size:256" json:"address"`
	Status    string         `gorm:"size:16;default:active" json:"status"`
	Revenue   float64        `gorm:"default:0" json:"revenue"`
	Cost      float64        `gorm:"default:0" json:"cost"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

type KPI struct {
	ID        uint64    `gorm:"primarykey" json:"id"`
	StoreID   uint64    `gorm:"index" json:"store_id"`
	Period    string    `gorm:"size:16;index" json:"period"`
	Revenue   float64   `json:"revenue"`
	Cost      float64   `json:"cost"`
	GP        float64   `json:"gp"`
	GPRate    float64   `json:"gp_rate"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Service) CreateStore(c *gin.Context) {
	var st Store
	if err := c.ShouldBindJSON(&st); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	if st.StoreCode == "" {
		st.StoreCode = fmt.Sprintf("XDPZ#%s-%d", st.Prefix, time.Now().UnixNano()%1000000)
	}
	s.DB().Create(&st)
	response.Created(c, st)
}

func (s *Service) ListStores(c *gin.Context) {
	var items []Store
	var total int64
	q := s.DB().Model(&Store{})
	if st := c.Query("status"); st != "" {
		q = q.Where("status = ?", st)
	}
	q.Count(&total)
	paginate(c, q).Order("created_at DESC").Find(&items)
	response.OK(c, gin.H{"items": items, "total": total})
}

func (s *Service) GetStore(c *gin.Context) {
	var st Store
	if err := s.DB().First(&st, c.Param("id")).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	response.OK(c, st)
}

func (s *Service) UpdateStore(c *gin.Context) {
	var st Store
	if err := s.DB().First(&st, c.Param("id")).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	c.ShouldBindJSON(&st)
	s.DB().Save(&st)
	response.OK(c, st)
}

func (s *Service) CalculateGP(c *gin.Context) {
	var st Store
	if err := s.DB().First(&st, c.Param("id")).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	gp := st.Revenue - st.Cost
	gpRate := 0.0
	if st.Revenue > 0 {
		gpRate = gp / st.Revenue * 100
	}
	response.OK(c, gin.H{
		"store_id": st.ID, "revenue": st.Revenue, "cost": st.Cost,
		"gp": gp, "gp_rate": gpRate, "bonus_rate": 0.05 + gpRate/100*0.1,
	})
}

func (s *Service) CreateKPI(c *gin.Context) {
	var k KPI
	if err := c.ShouldBindJSON(&k); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	k.GP = k.Revenue - k.Cost
	if k.Revenue > 0 {
		k.GPRate = k.GP / k.Revenue * 100
	}
	s.DB().Create(&k)
	response.Created(c, k)
}

func (s *Service) ListKPIs(c *gin.Context) {
	var items []KPI
	s.DB().Where("store_id = ?", c.Query("store_id")).Order("period DESC").Find(&items)
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
