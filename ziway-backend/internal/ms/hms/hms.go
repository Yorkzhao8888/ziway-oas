// Package hms is the human resources MBS — organization, employees, attendance, payroll.
package hms

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
	return &Service{BaseService: mbs.NewBaseService("hms", "mbs_hms", deps)}
}

func (s *Service) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/health", func(c *gin.Context) {
		response.OK(c, gin.H{"ms": "hms", "status": "ok", "desc": "人力资源服务"})
	})
	rg.POST("/organizations", s.CreateOrg)
	rg.GET("/organizations", s.ListOrgs)
	rg.POST("/employees", s.CreateEmployee)
	rg.GET("/employees", s.ListEmployees)
	rg.GET("/employees/:id", s.GetEmployee)
	rg.PUT("/employees/:id", s.UpdateEmployee)
	rg.POST("/attendance", s.ClockIn)
	rg.GET("/attendance", s.ListAttendance)
	rg.POST("/leaves", s.ApplyLeave)
	rg.GET("/leaves", s.ListLeaves)
	rg.PUT("/leaves/:id/approve", s.ApproveLeave)
}

func (s *Service) AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&Organization{}, &Employee{}, &Attendance{}, &LeaveRequest{})
}

type Organization struct {
	ID        uint64         `gorm:"primarykey" json:"id"`
	Name      string         `gorm:"size:128" json:"name"`
	ParentID  *uint64        `gorm:"index" json:"parent_id,omitempty"`
	Type      string         `gorm:"size:32" json:"type"`
	LeaderID  string         `gorm:"size:32" json:"leader_id"`
	Status    string         `gorm:"size:16;default:active" json:"status"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

type Employee struct {
	ID           uint64         `gorm:"primarykey" json:"id"`
	EmployeeCode string         `gorm:"uniqueIndex;size:32" json:"employee_code"`
	Name         string         `gorm:"size:64" json:"name"`
	Phone        string         `gorm:"size:32" json:"phone"`
	Email        string         `gorm:"size:128" json:"email"`
	OrgID        uint64         `gorm:"index" json:"org_id"`
	Position     string         `gorm:"size:64" json:"position"`
	Department   string         `gorm:"size:64" json:"department"`
	Status       string         `gorm:"size:16;default:active" json:"status"`
	EntryDate    *time.Time     `json:"entry_date,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

type Attendance struct {
	ID        uint64    `gorm:"primarykey" json:"id"`
	EmployeeID uint64  `gorm:"index" json:"employee_id"`
	Date      string    `gorm:"size:10;index" json:"date"`
	ClockIn   *time.Time `json:"clock_in,omitempty"`
	ClockOut  *time.Time `json:"clock_out,omitempty"`
	Status    string    `gorm:"size:16" json:"status"`
}

type LeaveRequest struct {
	ID        uint64         `gorm:"primarykey" json:"id"`
	EmployeeID uint64        `gorm:"index" json:"employee_id"`
	Type      string         `gorm:"size:32" json:"type"`
	StartDate *time.Time     `json:"start_date"`
	EndDate   *time.Time     `json:"end_date"`
	Reason    string         `gorm:"type:text" json:"reason"`
	Status    string         `gorm:"size:16;default:pending" json:"status"`
	ApproverID string        `gorm:"size:32" json:"approver_id,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (s *Service) CreateOrg(c *gin.Context) {
	var o Organization
	c.ShouldBindJSON(&o)
	s.DB().Create(&o)
	response.Created(c, o)
}

func (s *Service) ListOrgs(c *gin.Context) {
	var items []Organization
	s.DB().Order("created_at DESC").Find(&items)
	response.OK(c, items)
}

func (s *Service) CreateEmployee(c *gin.Context) {
	var e Employee
	if err := c.ShouldBindJSON(&e); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	e.EmployeeCode = fmt.Sprintf("XHPZ#%d", time.Now().UnixNano()%1000000)
	s.DB().Create(&e)
	response.Created(c, e)
}

func (s *Service) ListEmployees(c *gin.Context) {
	var items []Employee
	var total int64
	q := s.DB().Model(&Employee{})
	if oid := c.Query("org_id"); oid != "" {
		q = q.Where("org_id = ?", oid)
	}
	q.Count(&total)
	paginate(c, q).Order("created_at DESC").Find(&items)
	response.OK(c, gin.H{"items": items, "total": total})
}

func (s *Service) GetEmployee(c *gin.Context) {
	var e Employee
	if err := s.DB().First(&e, c.Param("id")).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	response.OK(c, e)
}

func (s *Service) UpdateEmployee(c *gin.Context) {
	var e Employee
	if err := s.DB().First(&e, c.Param("id")).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	c.ShouldBindJSON(&e)
	s.DB().Save(&e)
	response.OK(c, e)
}

func (s *Service) ClockIn(c *gin.Context) {
	var a Attendance
	c.ShouldBindJSON(&a)
	now := time.Now()
	if a.ClockIn == nil {
		a.ClockIn = &now
	}
	a.Date = now.Format("2006-01-02")
	a.Status = "normal"
	s.DB().Create(&a)
	response.Created(c, a)
}

func (s *Service) ListAttendance(c *gin.Context) {
	var items []Attendance
	s.DB().Where("employee_id = ?", c.Query("employee_id")).Order("date DESC").Limit(30).Find(&items)
	response.OK(c, items)
}

func (s *Service) ApplyLeave(c *gin.Context) {
	var l LeaveRequest
	c.ShouldBindJSON(&l)
	l.Status = "pending"
	s.DB().Create(&l)
	response.Created(c, l)
}

func (s *Service) ListLeaves(c *gin.Context) {
	var items []LeaveRequest
	s.DB().Order("created_at DESC").Limit(50).Find(&items)
	response.OK(c, items)
}

func (s *Service) ApproveLeave(c *gin.Context) {
	var body struct {
		Approve bool `json:"approve"`
	}
	c.ShouldBindJSON(&body)
	var l LeaveRequest
	s.DB().First(&l, c.Param("id"))
	if body.Approve {
		l.Status = "approved"
	} else {
		l.Status = "rejected"
	}
	s.DB().Save(&l)
	response.OK(c, l)
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
