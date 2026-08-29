// Package tms is the product R&D MBS — products, categories, PX pool, DPX shelf, NPI.
package tms

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
	return &Service{BaseService: mbs.NewBaseService("tms", "mbs_tms", deps)}
}

func (s *Service) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/health", func(c *gin.Context) {
		response.OK(c, gin.H{"ms": "tms", "status": "ok", "desc": "产品研发服务"})
	})
	rg.POST("/products", s.CreateProduct)
	rg.GET("/products", s.ListProducts)
	rg.GET("/products/:id", s.GetProduct)
	rg.PUT("/products/:id", s.UpdateProduct)
	rg.DELETE("/products/:id", s.DeleteProduct)
	rg.POST("/categories", s.CreateCategory)
	rg.GET("/categories", s.ListCategories)
	rg.POST("/npi", s.CreateNPI)
	rg.GET("/npi", s.ListNPI)
	rg.PUT("/npi/:id/stage", s.UpdateNPIStage)
}

func (s *Service) AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&Product{}, &Category{}, &NPIProject{})
}

type Product struct {
	ID          uint64         `gorm:"primarykey" json:"id"`
	SKUCode     string         `gorm:"uniqueIndex;size:64" json:"sku_code"`
	Name        string         `gorm:"size:128;index" json:"name"`
	CategoryID  *uint64        `gorm:"index" json:"category_id,omitempty"`
	Description string         `gorm:"type:text" json:"description"`
	Unit        string         `gorm:"size:16" json:"unit"`
	UnitPrice   float64        `json:"unit_price"`
	CostPrice   float64        `json:"cost_price"`
	Status      string         `gorm:"size:16;default:draft;index" json:"status"`
	Images      string         `gorm:"type:text" json:"images"`
	Attributes  string         `gorm:"type:text" json:"attributes"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

type Category struct {
	ID       uint64         `gorm:"primarykey" json:"id"`
	Name     string         `gorm:"size:64" json:"name"`
	ParentID *uint64        `gorm:"index" json:"parent_id,omitempty"`
	Sort     int            `json:"sort"`
	Status   string         `gorm:"size:16;default:active" json:"status"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

type NPIProject struct {
	ID          uint64         `gorm:"primarykey" json:"id"`
	ProjectCode string         `gorm:"uniqueIndex;size:32" json:"project_code"`
	Name        string         `gorm:"size:128" json:"name"`
	Description string         `gorm:"type:text" json:"description"`
	Stage       string         `gorm:"size:32;index" json:"stage"` // concept/development/testing/launched
	OwnerID     string         `gorm:"size:32" json:"owner_id"`
	StartDate   *time.Time     `json:"start_date,omitempty"`
	LaunchDate  *time.Time     `json:"launch_date,omitempty"`
	ROI         float64        `json:"roi"`
	Status      string         `gorm:"size:16;default:active" json:"status"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (s *Service) CreateProduct(c *gin.Context) {
	var p Product
	if err := c.ShouldBindJSON(&p); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	s.DB().Create(&p)
	response.Created(c, p)
}

func (s *Service) ListProducts(c *gin.Context) {
	var items []Product
	var total int64
	q := s.DB().Model(&Product{})
	if cat := c.Query("category_id"); cat != "" { q = q.Where("category_id = ?", cat) }
	if st := c.Query("status"); st != "" { q = q.Where("status = ?", st) }
	if kw := c.Query("keyword"); kw != "" { q = q.Where("name LIKE ? OR sku_code LIKE ?", "%"+kw+"%", "%"+kw+"%") }
	q.Count(&total)
	paginate(c, q).Order("created_at DESC").Find(&items)
	response.OK(c, gin.H{"items": items, "total": total})
}

func (s *Service) GetProduct(c *gin.Context) {
	var p Product
	if err := s.DB().First(&p, c.Param("id")).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	response.OK(c, p)
}

func (s *Service) UpdateProduct(c *gin.Context) {
	var p Product
	if err := s.DB().First(&p, c.Param("id")).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	c.ShouldBindJSON(&p)
	s.DB().Save(&p)
	response.OK(c, p)
}

func (s *Service) DeleteProduct(c *gin.Context) {
	s.DB().Delete(&Product{}, c.Param("id"))
	response.OK(c, nil)
}

func (s *Service) CreateCategory(c *gin.Context) {
	var cat Category
	c.ShouldBindJSON(&cat)
	s.DB().Create(&cat)
	response.Created(c, cat)
}

func (s *Service) ListCategories(c *gin.Context) {
	var items []Category
	s.DB().Order("sort ASC").Find(&items)
	response.OK(c, items)
}

func (s *Service) CreateNPI(c *gin.Context) {
	var n NPIProject
	c.ShouldBindJSON(&n)
	n.ProjectCode = fmt.Sprintf("NPI%s%d", time.Now().Format("20060102"), time.Now().UnixNano()%10000)
	s.DB().Create(&n)
	response.Created(c, n)
}

func (s *Service) ListNPI(c *gin.Context) {
	var items []NPIProject
	s.DB().Order("created_at DESC").Limit(50).Find(&items)
	response.OK(c, items)
}

func (s *Service) UpdateNPIStage(c *gin.Context) {
	var body struct {
		Stage string `json:"stage"`
	}
	c.ShouldBindJSON(&body)
	var n NPIProject
	s.DB().First(&n, c.Param("id"))
	n.Stage = body.Stage
	if body.Stage == "launched" {
		now := time.Now()
		n.LaunchDate = &now
	}
	s.DB().Save(&n)
	response.OK(c, n)
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
