// Package ems is the supply/delivery MBS — supplier onboarding, EX pool, DEX fulfillment,
// and the four market store types (supplier/producer/warehouse/logistics).
package ems

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
	return &Service{BaseService: mbs.NewBaseService("ems", "mbs_ems", deps)}
}

func (s *Service) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/health", func(c *gin.Context) {
		response.OK(c, gin.H{"ms": "ems", "status": "ok", "desc": "供给中枢+集市铺面"})
	})

	// 供应商
	rg.POST("/suppliers", s.CreateSupplier)
	rg.GET("/suppliers", s.ListSuppliers)
	rg.GET("/suppliers/:id", s.GetSupplier)
	rg.PUT("/suppliers/:id", s.UpdateSupplier)
	rg.POST("/suppliers/:id/approve", s.ApproveSupplier)

	// EX货池
	rg.POST("/supply-items", s.CreateSupplyItem)
	rg.GET("/supply-items", s.ListSupplyItems)
	rg.GET("/supply-items/:id", s.GetSupplyItem)
	rg.PUT("/supply-items/:id", s.UpdateSupplyItem)

	// 采购单
	rg.POST("/purchase-orders", s.CreatePO)
	rg.GET("/purchase-orders", s.ListPOs)
	rg.GET("/purchase-orders/:id", s.GetPO)
	rg.PUT("/purchase-orders/:id/status", s.UpdatePOStatus)

	// 合同
	rg.POST("/contracts", s.CreateContract)
	rg.GET("/contracts", s.ListContracts)

	// DEX交付
	rg.POST("/fulfillments", s.CreateFulfillment)
	rg.GET("/fulfillments", s.ListFulfillments)
	rg.PUT("/fulfillments/:id/status", s.UpdateFulfillmentStatus)
	rg.GET("/fulfillments/track/:tracking_no", s.TrackFulfillment)

	// 集市铺面（四种类型）
	rg.POST("/stores", s.CreateStore)
	rg.GET("/stores", s.ListStores)
	rg.GET("/stores/:id", s.GetStore)
	rg.PUT("/stores/:id", s.UpdateStore)
	rg.GET("/stores/:id/stats", s.GetStoreStats)
	rg.POST("/stores/apply", s.ApplyStore)
	rg.POST("/stores/applications/:id/review", s.ReviewApplication)

	// 铺面商品
	rg.POST("/products", s.CreateProduct)
	rg.GET("/products", s.ListProducts)
	rg.PUT("/products/:id", s.UpdateProduct)
	rg.DELETE("/products/:id", s.DeleteProduct)

	// 铺面订单
	rg.POST("/orders", s.CreateOrder)
	rg.GET("/orders", s.ListOrders)
	rg.PUT("/orders/:id/status", s.UpdateOrderStatus)

	// 生产商铺
	rg.PUT("/producer/profile", s.UpsertProducerProfile)
	rg.GET("/producer/profile/:store_id", s.GetProducerProfile)
	rg.POST("/producer/work-orders", s.CreateWorkOrder)
	rg.GET("/producer/work-orders", s.ListWorkOrders)
	rg.PUT("/producer/work-orders/:id", s.UpdateWorkOrder)

	// 仓配商铺
	rg.PUT("/warehouse/profile", s.UpsertWarehouseProfile)
	rg.GET("/warehouse/profile/:store_id", s.GetWarehouseProfile)
	rg.GET("/warehouse/inventory", s.ListInventory)
	rg.POST("/warehouse/inventory", s.UpsertInventory)
	rg.POST("/warehouse/outbound", s.CreateOutboundOrder)
	rg.PUT("/warehouse/outbound/:id/status", s.UpdateOutboundStatus)

	// 物流商铺
	rg.PUT("/logistics/profile", s.UpsertLogisticsProfile)
	rg.GET("/logistics/profile/:store_id", s.GetLogisticsProfile)
	rg.POST("/logistics/shipments", s.CreateShipment)
	rg.GET("/logistics/shipments", s.ListShipments)
	rg.GET("/logistics/shipments/:id", s.GetShipment)
	rg.GET("/logistics/track/:shipment_no", s.TrackShipment)
	rg.PUT("/logistics/shipments/:id/status", s.UpdateShipmentStatus)
}

func (s *Service) AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&Supplier{}, &SupplyPoolItem{}, &PurchaseOrder{}, &Contract{}, &Fulfillment{},
		&MerchantStore{}, &StoreApplication{}, &StoreProduct{}, &StoreOrder{}, &StoreReview{},
		&ProducerProfile{}, &WorkOrder{},
		&WarehouseProfile{}, &InventoryRecord{}, &OutboundOrder{},
		&LogisticsProfile{}, &Shipment{}, &TrackingEvent{},
	)
}

// ==================== Models ====================

type Supplier struct {
	ID             uint64         `gorm:"primarykey" json:"id"`
	SupplierCode   string         `gorm:"uniqueIndex;size:32" json:"supplier_code"`
	Name           string         `gorm:"size:128;index" json:"name"`
	SupplierType   string         `gorm:"size:32;index" json:"supplier_type"`
	ContactPerson  string         `gorm:"size:64" json:"contact_person"`
	ContactPhone   string         `gorm:"size:32" json:"contact_phone"`
	Email          string         `gorm:"size:128" json:"email"`
	Address        string         `gorm:"size:256" json:"address"`
	BusinessLicense string        `gorm:"size:128" json:"business_license"`
	TaxNumber      string         `gorm:"size:32" json:"tax_number"`
	Rating         float64        `gorm:"default:0" json:"rating"`
	CreditLimit    float64        `gorm:"default:0" json:"credit_limit"`
	PaymentTerms   int            `gorm:"default:30" json:"payment_terms"`
	Status         string         `gorm:"size:16;default:pending;index" json:"status"`
	ApprovedBy     string         `gorm:"size:32" json:"approved_by,omitempty"`
	ApprovedAt     *time.Time     `json:"approved_at,omitempty"`
	RejectReason   string         `gorm:"size:256" json:"reject_reason,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

type SupplyPoolItem struct {
	ID          uint64         `gorm:"primarykey" json:"id"`
	SupplierID  uint64         `gorm:"index" json:"supplier_id"`
	SKUCode     string         `gorm:"uniqueIndex;size:64" json:"sku_code"`
	Name        string         `gorm:"size:128;index" json:"name"`
	Category    string         `gorm:"size:64;index" json:"category"`
	Description string         `gorm:"type:text" json:"description"`
	Spec        string         `gorm:"size:256" json:"spec"`
	Unit        string         `gorm:"size:16" json:"unit"`
	UnitPrice   float64        `json:"unit_price"`
	MinOrderQty int            `gorm:"default:1" json:"min_order_qty"`
	StockQty    int            `gorm:"default:0" json:"stock_qty"`
	Status      string         `gorm:"size:16;default:active;index" json:"status"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

type PurchaseOrder struct {
	ID              uint64         `gorm:"primarykey" json:"id"`
	POCode          string         `gorm:"uniqueIndex;size:32" json:"po_code"`
	SupplierID      uint64         `gorm:"index" json:"supplier_id"`
	Items           string         `gorm:"type:text" json:"items"`
	TotalAmount     float64        `json:"total_amount"`
	DeliveryAddress string         `gorm:"size:256" json:"delivery_address"`
	ExpectedDate    *time.Time     `json:"expected_date,omitempty"`
	ActualDate      *time.Time     `json:"actual_date,omitempty"`
	PaymentStatus   string         `gorm:"size:16;default:unpaid" json:"payment_status"`
	Status          string         `gorm:"size:16;default:draft;index" json:"status"`
	Notes           string         `gorm:"type:text" json:"notes"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

type Contract struct {
	ID           uint64         `gorm:"primarykey" json:"id"`
	ContractNo   string         `gorm:"uniqueIndex;size:32" json:"contract_no"`
	SupplierID   uint64         `gorm:"index" json:"supplier_id"`
	Title        string         `gorm:"size:128" json:"title"`
	ContractType string         `gorm:"size:32" json:"contract_type"`
	Content      string         `gorm:"type:text" json:"content"`
	TotalAmount  float64        `json:"total_amount"`
	StartDate    *time.Time     `json:"start_date,omitempty"`
	EndDate      *time.Time     `json:"end_date,omitempty"`
	Status       string         `gorm:"size:16;default:draft" json:"status"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

type Fulfillment struct {
	ID              uint64         `gorm:"primarykey" json:"id"`
	FulfillmentNo   string         `gorm:"uniqueIndex;size:32" json:"fulfillment_no"`
	POID            uint64         `gorm:"index" json:"po_id"`
	SupplierID      uint64         `gorm:"index" json:"supplier_id"`
	CarrierType     string         `gorm:"size:32" json:"carrier_type"`
	CarrierName     string         `gorm:"size:64" json:"carrier_name"`
	TrackingNo      string         `gorm:"size:64;index" json:"tracking_no"`
	FromAddress     string         `gorm:"size:256" json:"from_address"`
	ToAddress       string         `gorm:"size:256" json:"to_address"`
	Status          string         `gorm:"size:16;default:pending;index" json:"status"`
	ShippedAt       *time.Time     `json:"shipped_at,omitempty"`
	DeliveredAt     *time.Time     `json:"delivered_at,omitempty"`
	ReceivedAt      *time.Time     `json:"received_at,omitempty"`
	ShippedQty      int            `json:"shipped_qty"`
	ReceivedQty     int            `json:"received_qty"`
	ExceptionReason string         `gorm:"size:256" json:"exception_reason,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

// ----- 集市铺面 -----

type MerchantStore struct {
	ID           uint64         `gorm:"primarykey" json:"id"`
	StoreCode    string         `gorm:"uniqueIndex;size:32" json:"store_code"`
	Name         string         `gorm:"size:128;index" json:"name"`
	StoreType    string         `gorm:"size:32;index" json:"store_type"`
	OwnerUserID  string         `gorm:"index;size:32" json:"owner_user_id"`
	SupplierID   *uint64        `gorm:"index" json:"supplier_id,omitempty"`
	Logo         string         `gorm:"size:512" json:"logo"`
	Banner       string         `gorm:"size:512" json:"banner"`
	Description  string         `gorm:"type:text" json:"description"`
	ContactPhone string         `gorm:"size:32" json:"contact_phone"`
	ContactName  string         `gorm:"size:64" json:"contact_name"`
	Address      string         `gorm:"size:256" json:"address"`
	Capabilities string         `gorm:"type:text" json:"capabilities"`
	Rating       float64        `gorm:"default:5.0" json:"rating"`
	ReviewCount  int            `gorm:"default:0" json:"review_count"`
	OrderCount   int            `gorm:"default:0" json:"order_count"`
	ProductCount int            `gorm:"default:0" json:"product_count"`
	Status       string         `gorm:"size:16;default:pending;index" json:"status"`
	Featured     bool           `gorm:"default:false;index" json:"featured"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

type StoreApplication struct {
	ID              uint64         `gorm:"primarykey" json:"id"`
	ApplicationNo   string         `gorm:"uniqueIndex;size:32" json:"application_no"`
	ApplicantID     string         `gorm:"index;size:32" json:"applicant_id"`
	ApplicantName   string         `gorm:"size:64" json:"applicant_name"`
	Phone           string         `gorm:"size:32" json:"phone"`
	StoreType       string         `gorm:"size:32;index" json:"store_type"`
	StoreName       string         `gorm:"size:128" json:"store_name"`
	Description     string         `gorm:"type:text" json:"description"`
	BusinessLicense string         `gorm:"size:128" json:"business_license"`
	Status          string         `gorm:"size:16;default:submitted;index" json:"status"`
	ReviewNote      string         `gorm:"type:text" json:"review_note,omitempty"`
	ReviewedAt      *time.Time     `json:"reviewed_at,omitempty"`
	StoreID         *uint64        `json:"store_id,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

type StoreProduct struct {
	ID          uint64         `gorm:"primarykey" json:"id"`
	StoreID     uint64         `gorm:"index" json:"store_id"`
	SKUCode     string         `gorm:"size:64;index" json:"sku_code"`
	Name        string         `gorm:"size:128;index" json:"name"`
	Category    string         `gorm:"size:64;index" json:"category"`
	Description string         `gorm:"type:text" json:"description"`
	Unit        string         `gorm:"size:16" json:"unit"`
	UnitPrice   float64        `json:"unit_price"`
	MinQty      int            `gorm:"default:1" json:"min_qty"`
	StockQty    int            `gorm:"default:0" json:"stock_qty"`
	ExtraAttrs  string         `gorm:"type:text" json:"extra_attrs"`
	Status      string         `gorm:"size:16;default:active" json:"status"`
	SortOrder   int            `gorm:"default:0" json:"sort_order"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

type StoreOrder struct {
	ID           uint64         `gorm:"primarykey" json:"id"`
	OrderNo      string         `gorm:"uniqueIndex;size:32" json:"order_no"`
	StoreID      uint64         `gorm:"index" json:"store_id"`
	BuyerUserID  string         `gorm:"index;size:32" json:"buyer_user_id"`
	Items        string         `gorm:"type:text" json:"items"`
	TotalAmount  float64        `json:"total_amount"`
	DeliveryType string         `gorm:"size:32" json:"delivery_type"`
	Address      string         `gorm:"size:256" json:"address"`
	ContactName  string         `gorm:"size:64" json:"contact_name"`
	ContactPhone string         `gorm:"size:32" json:"contact_phone"`
	Remark       string         `gorm:"type:text" json:"remark"`
	Status       string         `gorm:"size:16;default:pending;index" json:"status"`
	StatusRemark string         `gorm:"size:256" json:"status_remark,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

type StoreReview struct {
	ID        uint64         `gorm:"primarykey" json:"id"`
	StoreID   uint64         `gorm:"index" json:"store_id"`
	UserID    string         `gorm:"index;size:32" json:"user_id"`
	UserName  string         `gorm:"size:64" json:"user_name"`
	Rating    int            `json:"rating"`
	Content   string         `gorm:"type:text" json:"content"`
	Reply     string         `gorm:"type:text" json:"reply,omitempty"`
	RepliedAt *time.Time     `json:"replied_at,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// ----- 生产商铺 -----

type ProducerProfile struct {
	ID                uint64         `gorm:"primarykey" json:"id"`
	StoreID           uint64         `gorm:"uniqueIndex" json:"store_id"`
	TotalCapacity     int            `json:"total_capacity"`
	AvailableCapacity int            `json:"available_capacity"`
	CapacityUnit      string         `gorm:"size:32" json:"capacity_unit"`
	LeadTimeDays      int            `gorm:"default:7" json:"lead_time_days"`
	MOQ               int            `gorm:"default:1" json:"moq"`
	SupportOEM        bool           `json:"support_oem"`
	SupportODM        bool           `json:"support_odm"`
	Certifications    string         `gorm:"type:text" json:"certifications"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}

type WorkOrder struct {
	ID           uint64         `gorm:"primarykey" json:"id"`
	StoreID      uint64         `gorm:"index" json:"store_id"`
	WorkOrderNo  string         `gorm:"uniqueIndex;size:32" json:"work_order_no"`
	ProductName  string         `gorm:"size:128" json:"product_name"`
	Quantity     int            `json:"quantity"`
	Status       string         `gorm:"size:16;default:planned;index" json:"status"`
	Progress     int            `gorm:"default:0" json:"progress"`
	StartDate    *time.Time     `json:"start_date,omitempty"`
	DueDate      *time.Time     `json:"due_date,omitempty"`
	CompleteDate *time.Time     `json:"complete_date,omitempty"`
	QAStatus     string         `gorm:"size:16;default:pending" json:"qa_status"`
	BOMItems     string         `gorm:"type:text" json:"bom_items"`
	Note         string         `gorm:"type:text" json:"note"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// ----- 仓配商铺 -----

type WarehouseProfile struct {
	ID              uint64         `gorm:"primarykey" json:"id"`
	StoreID         uint64         `gorm:"uniqueIndex" json:"store_id"`
	TotalArea       float64        `json:"total_area"`
	AvailableArea   float64        `json:"available_area"`
	LocationCount   int            `json:"location_count"`
	TempZones       string         `gorm:"type:text" json:"temp_zones"`
	SupportBonded   bool           `json:"support_bonded"`
	PickPackEnabled bool           `gorm:"default:true" json:"pick_pack_enabled"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

type InventoryRecord struct {
	ID           uint64         `gorm:"primarykey" json:"id"`
	StoreID      uint64         `gorm:"index" json:"store_id"`
	SKUCode      string         `gorm:"size:64;index" json:"sku_code"`
	ProductName  string         `gorm:"size:128" json:"product_name"`
	LocationCode string         `gorm:"size:32;index" json:"location_code"`
	BatchNo      string         `gorm:"size:32;index" json:"batch_no"`
	Quantity     int            `json:"quantity"`
	ReservedQty  int            `json:"reserved_qty"`
	Unit         string         `gorm:"size:16" json:"unit"`
	InboundDate  *time.Time     `json:"inbound_date,omitempty"`
	ExpiryDate   *time.Time     `json:"expiry_date,omitempty"`
	Status       string         `gorm:"size:16;default:normal;index" json:"status"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

type OutboundOrder struct {
	ID           uint64         `gorm:"primarykey" json:"id"`
	StoreID      uint64         `gorm:"index" json:"store_id"`
	OutboundNo   string         `gorm:"uniqueIndex;size:32" json:"outbound_no"`
	Items        string         `gorm:"type:text" json:"items"`
	ToAddress    string         `gorm:"size:256" json:"to_address"`
	ContactName  string         `gorm:"size:64" json:"contact_name"`
	ContactPhone string         `gorm:"size:32" json:"contact_phone"`
	Status       string         `gorm:"size:16;default:draft;index" json:"status"`
	PickedAt     *time.Time     `json:"picked_at,omitempty"`
	PackedAt     *time.Time     `json:"packed_at,omitempty"`
	ShippedAt    *time.Time     `json:"shipped_at,omitempty"`
	Carrier      string         `gorm:"size:64" json:"carrier"`
	TrackingNo   string         `gorm:"size:64" json:"tracking_no"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// ----- 物流商铺 -----

type LogisticsProfile struct {
	ID               uint64         `gorm:"primarykey" json:"id"`
	StoreID          uint64         `gorm:"uniqueIndex" json:"store_id"`
	VehicleTypes     string         `gorm:"type:text" json:"vehicle_types"`
	TotalVehicles    int            `json:"total_vehicles"`
	TotalDrivers     int            `json:"total_drivers"`
	ServiceAreas     string         `gorm:"type:text" json:"service_areas"`
	RouteCoverage    string         `gorm:"type:text" json:"route_coverage"`
	SupportColdChain bool           `json:"support_cold_chain"`
	SupportCOD       bool           `json:"support_cod"`
	MaxWeight        float64        `json:"max_weight"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
}

type Shipment struct {
	ID              uint64         `gorm:"primarykey" json:"id"`
	StoreID         uint64         `gorm:"index" json:"store_id"`
	ShipmentNo      string         `gorm:"uniqueIndex;size:32" json:"shipment_no"`
	CargoDesc       string         `gorm:"size:256" json:"cargo_desc"`
	CargoWeight     float64        `json:"cargo_weight"`
	Pieces          int            `json:"pieces"`
	FromName        string         `gorm:"size:64" json:"from_name"`
	FromPhone       string         `gorm:"size:32" json:"from_phone"`
	FromAddress     string         `gorm:"size:256" json:"from_address"`
	ToName          string         `gorm:"size:64" json:"to_name"`
	ToPhone         string         `gorm:"size:32" json:"to_phone"`
	ToAddress       string         `gorm:"size:256" json:"to_address"`
	VehicleType     string         `gorm:"size:32" json:"vehicle_type"`
	DriverName      string         `gorm:"size:32" json:"driver_name"`
	DriverPhone     string         `gorm:"size:32" json:"driver_phone"`
	PlateNumber     string         `gorm:"size:16" json:"plate_number"`
	Freight         float64        `json:"freight"`
	FreightStatus   string         `gorm:"size:16;default:unpaid" json:"freight_status"`
	Status          string         `gorm:"size:16;default:pending;index" json:"status"`
	CurrentLocation string         `gorm:"size:128" json:"current_location"`
	ETADate         *time.Time     `json:"eta_date,omitempty"`
	DeliveredAt     *time.Time     `json:"delivered_at,omitempty"`
	SignedBy        string         `gorm:"size:32" json:"signed_by"`
	ExceptionNote   string         `gorm:"type:text" json:"exception_note"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

type TrackingEvent struct {
	ID          uint64    `gorm:"primarykey" json:"id"`
	ShipmentID  uint64    `gorm:"index" json:"shipment_id"`
	Status      string    `gorm:"size:32" json:"status"`
	Location    string    `gorm:"size:128" json:"location"`
	Description string    `gorm:"size:256" json:"description"`
	OccurredAt  time.Time `json:"occurred_at"`
	CreatedAt   time.Time `json:"created_at"`
}
