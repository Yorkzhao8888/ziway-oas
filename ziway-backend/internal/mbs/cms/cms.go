// Package cms is the customer management MBS — customers, orders, CX pool, messaging.
package cms

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ziway/backend/internal/mbs"
	"ziway/backend/pkg/response"
)

type Service struct {
	mbs.BaseService
}

func New(deps mbs.Dependencies) *Service {
	return &Service{BaseService: mbs.NewBaseService("cms", "mbs_cms", deps)}
}

func (s *Service) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/health", func(c *gin.Context) {
		response.OK(c, gin.H{"ms": "cms", "status": "ok", "desc": "客户管理服务"})
	})
	// 客户
	rg.POST("/customers", s.CreateCustomer)
	rg.GET("/customers", s.ListCustomers)
	rg.GET("/customers/:id", s.GetCustomer)
	rg.PUT("/customers/:id", s.UpdateCustomer)
	// 订单
	rg.POST("/orders", s.CreateOrder)
	rg.GET("/orders", s.ListOrders)
	rg.GET("/orders/:id", s.GetOrder)
	rg.PUT("/orders/:id/status", s.UpdateOrderStatus)
	// 消息
	rg.POST("/messages", s.SendMessage)
	rg.GET("/messages", s.ListMessages)
	// 通知
	rg.POST("/notifications", s.CreateNotification)
	rg.GET("/notifications", s.ListNotifications)
}

func (s *Service) AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&Customer{}, &Order{}, &Message{}, &Notification{})
}

// ===== Models =====

type Customer struct {
	ID           uint64         `gorm:"primarykey" json:"id"`
	CustomerCode string         `gorm:"uniqueIndex;size:32" json:"customer_code"`
	CustomerType string         `gorm:"size:8;index" json:"customer_type"` // C=个人/B=企业
	Name         string         `gorm:"size:128" json:"name"`
	Phone        string         `gorm:"size:32;index" json:"phone"`
	Email        string         `gorm:"size:128" json:"email"`
	Level        string         `gorm:"size:16;default:bronze" json:"level"`
	Points       int            `gorm:"default:0" json:"points"`
	Status       string         `gorm:"size:16;default:active" json:"status"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

type Order struct {
	ID          uint64         `gorm:"primarykey" json:"id"`
	OrderNo     string         `gorm:"uniqueIndex;size:32" json:"order_no"`
	CustomerID  uint64         `gorm:"index" json:"customer_id"`
	Items       string         `gorm:"type:text" json:"items"`
	TotalAmount float64        `json:"total_amount"`
	Status      string         `gorm:"size:16;default:pending;index" json:"status"`
	PaymentStatus string       `gorm:"size:16;default:unpaid" json:"payment_status"`
	Address     string         `gorm:"size:256" json:"address"`
	Remark      string         `gorm:"type:text" json:"remark"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

type Message struct {
	ID         uint64         `gorm:"primarykey" json:"id"`
	SenderID   string         `gorm:"index;size:32" json:"sender_id"`
	ReceiverID string         `gorm:"index;size:32" json:"receiver_id"`
	Channel    string         `gorm:"size:16" json:"channel"`
	Subject    string         `gorm:"size:256" json:"subject"`
	Body       string         `gorm:"type:text" json:"body"`
	Status     string         `gorm:"size:16;default:pending" json:"status"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}

type Notification struct {
	ID        uint64         `gorm:"primarykey" json:"id"`
	UserID    string         `gorm:"index;size:32" json:"user_id"`
	Type      string         `gorm:"size:32" json:"type"`
	Title     string         `gorm:"size:128" json:"title"`
	Body      string         `gorm:"type:text" json:"body"`
	Read      bool           `gorm:"default:false" json:"read"`
	Link      string         `gorm:"size:512" json:"link"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// ===== Handlers =====

func (s *Service) CreateCustomer(c *gin.Context) {
	var cu Customer
	if err := c.ShouldBindJSON(&cu); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	if cu.CustomerCode == "" {
		cu.CustomerCode = fmt.Sprintf("XCPZ#%s-%d", cu.CustomerType, time.Now().UnixNano()%1000000)
	}
	s.DB().Create(&cu)
	response.Created(c, cu)
}

func (s *Service) ListCustomers(c *gin.Context) {
	var items []Customer
	var total int64
	q := s.DB().Model(&Customer{})
	if t := c.Query("customer_type"); t != "" {
		q = q.Where("customer_type = ?", t)
	}
	if kw := c.Query("keyword"); kw != "" {
		q = q.Where("name LIKE ? OR phone LIKE ?", "%"+kw+"%", "%"+kw+"%")
	}
	q.Count(&total)
	paginate(c, q).Order("created_at DESC").Find(&items)
	response.OK(c, gin.H{"items": items, "total": total})
}

func (s *Service) GetCustomer(c *gin.Context) {
	var cu Customer
	if err := s.DB().First(&cu, c.Param("id")).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	response.OK(c, cu)
}

func (s *Service) UpdateCustomer(c *gin.Context) {
	var cu Customer
	if err := s.DB().First(&cu, c.Param("id")).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	c.ShouldBindJSON(&cu)
	s.DB().Save(&cu)
	response.OK(c, cu)
}

func (s *Service) CreateOrder(c *gin.Context) {
	var o Order
	if err := c.ShouldBindJSON(&o); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	o.OrderNo = fmt.Sprintf("CO%s%d", time.Now().Format("20060102"), time.Now().UnixNano()%10000)
	s.DB().Create(&o)
	response.Created(c, o)
}

func (s *Service) ListOrders(c *gin.Context) {
	var items []Order
	var total int64
	q := s.DB().Model(&Order{})
	if cid := c.Query("customer_id"); cid != "" {
		q = q.Where("customer_id = ?", cid)
	}
	if st := c.Query("status"); st != "" {
		q = q.Where("status = ?", st)
	}
	q.Count(&total)
	paginate(c, q).Order("created_at DESC").Find(&items)
	response.OK(c, gin.H{"items": items, "total": total})
}

func (s *Service) GetOrder(c *gin.Context) {
	var o Order
	if err := s.DB().First(&o, c.Param("id")).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	response.OK(c, o)
}

func (s *Service) UpdateOrderStatus(c *gin.Context) {
	var body struct {
		Status string `json:"status"`
	}
	c.ShouldBindJSON(&body)
	s.DB().Model(&Order{}).Where("id = ?", c.Param("id")).Update("status", body.Status)
	response.OK(c, nil)
}

func (s *Service) SendMessage(c *gin.Context) {
	var m Message
	if err := c.ShouldBindJSON(&m); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	s.DB().Create(&m)
	response.Created(c, m)
}

func (s *Service) ListMessages(c *gin.Context) {
	var items []Message
	uid := c.Query("user_id")
	s.DB().Where("sender_id = ? OR receiver_id = ?", uid, uid).Order("created_at DESC").Limit(50).Find(&items)
	response.OK(c, items)
}

func (s *Service) CreateNotification(c *gin.Context) {
	var n Notification
	if err := c.ShouldBindJSON(&n); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	s.DB().Create(&n)
	response.Created(c, n)
}

func (s *Service) ListNotifications(c *gin.Context) {
	var items []Notification
	uid := c.Query("user_id")
	s.DB().Where("user_id = ?", uid).Order("created_at DESC").Limit(50).Find(&items)
	response.OK(c, items)
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
