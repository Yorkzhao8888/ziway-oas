package ems

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ziway/backend/pkg/response"
)

// ==================== Supplier Handlers ====================

func (s *Service) CreateSupplier(c *gin.Context) {
	var sup Supplier
	if err := c.ShouldBindJSON(&sup); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	if sup.SupplierCode == "" {
		sup.SupplierCode = fmt.Sprintf("XEPZ#%d", time.Now().UnixNano()%1000000)
	}
	if err := s.DB().Create(&sup).Error; err != nil {
		response.InternalError(c, "create failed")
		return
	}
	response.Created(c, sup)
}

func (s *Service) ListSuppliers(c *gin.Context) {
	var items []Supplier
	var total int64
	q := paginate(c, s.DB().Model(&Supplier{}))
	if st := c.Query("status"); st != "" {
		q = q.Where("status = ?", st)
	}
	if t := c.Query("supplier_type"); t != "" {
		q = q.Where("supplier_type = ?", t)
	}
	q.Count(&total)
	q.Order("created_at DESC").Find(&items)
	response.OK(c, gin.H{"items": items, "total": total})
}

func (s *Service) GetSupplier(c *gin.Context) {
	var sup Supplier
	if err := s.DB().First(&sup, c.Param("id")).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	response.OK(c, sup)
}

func (s *Service) UpdateSupplier(c *gin.Context) {
	var sup Supplier
	if err := s.DB().First(&sup, c.Param("id")).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	c.ShouldBindJSON(&sup)
	s.DB().Save(&sup)
	response.OK(c, sup)
}

func (s *Service) ApproveSupplier(c *gin.Context) {
	var sup Supplier
	if err := s.DB().First(&sup, c.Param("id")).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	now := time.Now()
	sup.Status = "approved"
	sup.ApprovedAt = &now
	s.DB().Save(&sup)
	response.OK(c, sup)
}

// ==================== SupplyItem Handlers ====================

func (s *Service) CreateSupplyItem(c *gin.Context) {
	var item SupplyPoolItem
	if err := c.ShouldBindJSON(&item); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	s.DB().Create(&item)
	response.Created(c, item)
}

func (s *Service) ListSupplyItems(c *gin.Context) {
	var items []SupplyPoolItem
	var total int64
	q := paginate(c, s.DB().Model(&SupplyPoolItem{}))
	if sup := c.Query("supplier_id"); sup != "" {
		q = q.Where("supplier_id = ?", sup)
	}
	if cat := c.Query("category"); cat != "" {
		q = q.Where("category = ?", cat)
	}
	q.Count(&total)
	q.Order("created_at DESC").Find(&items)
	response.OK(c, gin.H{"items": items, "total": total})
}

func (s *Service) GetSupplyItem(c *gin.Context) {
	var item SupplyPoolItem
	if err := s.DB().First(&item, c.Param("id")).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	response.OK(c, item)
}

func (s *Service) UpdateSupplyItem(c *gin.Context) {
	var item SupplyPoolItem
	if err := s.DB().First(&item, c.Param("id")).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	c.ShouldBindJSON(&item)
	s.DB().Save(&item)
	response.OK(c, item)
}

// ==================== PurchaseOrder Handlers ====================

func (s *Service) CreatePO(c *gin.Context) {
	var po PurchaseOrder
	if err := c.ShouldBindJSON(&po); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	po.POCode = fmt.Sprintf("PO%s%d", time.Now().Format("20060102"), time.Now().UnixNano()%10000)
	s.DB().Create(&po)
	response.Created(c, po)
}

func (s *Service) ListPOs(c *gin.Context) {
	var items []PurchaseOrder
	var total int64
	q := paginate(c, s.DB().Model(&PurchaseOrder{}))
	if sid := c.Query("supplier_id"); sid != "" {
		q = q.Where("supplier_id = ?", sid)
	}
	if st := c.Query("status"); st != "" {
		q = q.Where("status = ?", st)
	}
	q.Count(&total)
	q.Order("created_at DESC").Find(&items)
	response.OK(c, gin.H{"items": items, "total": total})
}

func (s *Service) GetPO(c *gin.Context) {
	var po PurchaseOrder
	if err := s.DB().First(&po, c.Param("id")).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	response.OK(c, po)
}

func (s *Service) UpdatePOStatus(c *gin.Context) {
	var body struct {
		Status string `json:"status"`
	}
	c.ShouldBindJSON(&body)
	var po PurchaseOrder
	if err := s.DB().First(&po, c.Param("id")).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	po.Status = body.Status
	if body.Status == "received" || body.Status == "completed" {
		now := time.Now()
		po.ActualDate = &now
	}
	s.DB().Save(&po)
	response.OK(c, po)
}

// ==================== Contract Handlers ====================

func (s *Service) CreateContract(c *gin.Context) {
	var ct Contract
	if err := c.ShouldBindJSON(&ct); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	ct.ContractNo = fmt.Sprintf("CT%s%d", time.Now().Format("20060102"), time.Now().UnixNano()%10000)
	s.DB().Create(&ct)
	response.Created(c, ct)
}

func (s *Service) ListContracts(c *gin.Context) {
	var items []Contract
	var total int64
	q := paginate(c, s.DB().Model(&Contract{}))
	if sid := c.Query("supplier_id"); sid != "" {
		q = q.Where("supplier_id = ?", sid)
	}
	q.Count(&total)
	q.Order("created_at DESC").Find(&items)
	response.OK(c, gin.H{"items": items, "total": total})
}

// ==================== Fulfillment Handlers ====================

func (s *Service) CreateFulfillment(c *gin.Context) {
	var f Fulfillment
	if err := c.ShouldBindJSON(&f); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	f.FulfillmentNo = fmt.Sprintf("FL%s%d", time.Now().Format("20060102"), time.Now().UnixNano()%10000)
	s.DB().Create(&f)
	response.Created(c, f)
}

func (s *Service) ListFulfillments(c *gin.Context) {
	var items []Fulfillment
	var total int64
	q := paginate(c, s.DB().Model(&Fulfillment{}))
	if st := c.Query("status"); st != "" {
		q = q.Where("status = ?", st)
	}
	q.Count(&total)
	q.Order("created_at DESC").Find(&items)
	response.OK(c, gin.H{"items": items, "total": total})
}

func (s *Service) UpdateFulfillmentStatus(c *gin.Context) {
	var body struct {
		Status      string `json:"status"`
		Location    string `json:"location"`
		Description string `json:"description"`
	}
	c.ShouldBindJSON(&body)
	var f Fulfillment
	if err := s.DB().First(&f, c.Param("id")).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	f.Status = body.Status
	now := time.Now()
	switch body.Status {
	case "shipped":
		f.ShippedAt = &now
	case "delivered":
		f.DeliveredAt = &now
	case "received":
		f.ReceivedAt = &now
	}
	s.DB().Save(&f)
	response.OK(c, f)
}

func (s *Service) TrackFulfillment(c *gin.Context) {
	var f Fulfillment
	if err := s.DB().Where("tracking_no = ?", c.Param("tracking_no")).First(&f).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	response.OK(c, f)
}

// ==================== Store Handlers ====================

func (s *Service) CreateStore(c *gin.Context) {
	var st MerchantStore
	if err := c.ShouldBindJSON(&st); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	if st.StoreCode == "" {
		st.StoreCode = fmt.Sprintf("ST%s%d", time.Now().Format("20060102"), time.Now().UnixNano()%10000)
	}
	st.Capabilities = defaultCapabilities(st.StoreType)
	s.DB().Create(&st)
	response.Created(c, st)
}

func (s *Service) ListStores(c *gin.Context) {
	var items []MerchantStore
	var total int64
	q := s.DB().Model(&MerchantStore{}).Where("status = ?", "active")
	if t := c.Query("store_type"); t != "" {
		q = q.Where("store_type = ?", t)
	}
	if kw := c.Query("keyword"); kw != "" {
		q = q.Where("name LIKE ?", "%"+kw+"%")
	}
	q.Count(&total)
	paginate(c, q).Order("featured DESC, rating DESC, created_at DESC").Find(&items)
	response.OK(c, gin.H{"items": items, "total": total})
}

func (s *Service) GetStore(c *gin.Context) {
	var st MerchantStore
	if err := s.DB().First(&st, c.Param("id")).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	response.OK(c, st)
}

func (s *Service) UpdateStore(c *gin.Context) {
	var st MerchantStore
	if err := s.DB().First(&st, c.Param("id")).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	c.ShouldBindJSON(&st)
	s.DB().Save(&st)
	response.OK(c, st)
}

func (s *Service) GetStoreStats(c *gin.Context) {
	var st MerchantStore
	if err := s.DB().First(&st, c.Param("id")).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	var orderCount int64
	var revenue float64
	s.DB().Model(&StoreOrder{}).Where("store_id = ?", st.ID).Count(&orderCount)
	s.DB().Model(&StoreOrder{}).Where("store_id = ? AND status = ?", st.ID, "completed").Select("COALESCE(SUM(total_amount),0)").Scan(&revenue)
	var productCount int64
	s.DB().Model(&StoreProduct{}).Where("store_id = ? AND status = ?", st.ID, "active").Count(&productCount)
	response.OK(c, gin.H{
		"store_id": st.ID, "store_name": st.Name, "store_type": st.StoreType,
		"rating": st.Rating, "review_count": st.ReviewCount,
		"order_count": orderCount, "total_revenue": revenue, "product_count": productCount,
	})
}

func (s *Service) ApplyStore(c *gin.Context) {
	var app StoreApplication
	if err := c.ShouldBindJSON(&app); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	app.ApplicationNo = fmt.Sprintf("APP%s%d", time.Now().Format("20060102"), time.Now().UnixNano()%10000)
	app.Status = "submitted"
	s.DB().Create(&app)
	response.Created(c, app)
}

func (s *Service) ReviewApplication(c *gin.Context) {
	var body struct {
		Approve bool   `json:"approve"`
		Note    string `json:"note"`
	}
	c.ShouldBindJSON(&body)
	var app StoreApplication
	if err := s.DB().First(&app, c.Param("id")).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	now := time.Now()
	app.ReviewedAt = &now
	app.ReviewNote = body.Note
	if body.Approve {
		app.Status = "approved"
		st := MerchantStore{
			StoreCode:    fmt.Sprintf("ST%s%d", time.Now().Format("20060102"), time.Now().UnixNano()%10000),
			Name:         app.StoreName,
			StoreType:    app.StoreType,
			OwnerUserID:  app.ApplicantID,
			Description:  app.Description,
			ContactPhone: app.Phone,
			ContactName:  app.ApplicantName,
			Status:       "active",
			Capabilities: defaultCapabilities(app.StoreType),
		}
		s.DB().Create(&st)
		sid := st.ID
		app.StoreID = &sid
	} else {
		app.Status = "rejected"
	}
	s.DB().Save(&app)
	response.OK(c, app)
}

// ==================== Product Handlers ====================

func (s *Service) CreateProduct(c *gin.Context) {
	var p StoreProduct
	if err := c.ShouldBindJSON(&p); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	s.DB().Create(&p)
	s.DB().Model(&MerchantStore{}).Where("id = ?", p.StoreID).UpdateColumn("product_count", gorm.Expr("product_count + 1"))
	response.Created(c, p)
}

func (s *Service) ListProducts(c *gin.Context) {
	var items []StoreProduct
	var total int64
	q := s.DB().Model(&StoreProduct{}).Where("status = ?", "active")
	if sid := c.Query("store_id"); sid != "" {
		q = q.Where("store_id = ?", sid)
	}
	if cat := c.Query("category"); cat != "" {
		q = q.Where("category = ?", cat)
	}
	q.Count(&total)
	paginate(c, q).Order("sort_order ASC, created_at DESC").Find(&items)
	response.OK(c, gin.H{"items": items, "total": total})
}

func (s *Service) UpdateProduct(c *gin.Context) {
	var p StoreProduct
	if err := s.DB().First(&p, c.Param("id")).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	c.ShouldBindJSON(&p)
	s.DB().Save(&p)
	response.OK(c, p)
}

func (s *Service) DeleteProduct(c *gin.Context) {
	var p StoreProduct
	s.DB().First(&p, c.Param("id"))
	s.DB().Delete(&StoreProduct{}, c.Param("id"))
	if p.ID > 0 {
		s.DB().Model(&MerchantStore{}).Where("id = ?", p.StoreID).UpdateColumn("product_count", gorm.Expr("MAX(product_count - 1, 0)"))
	}
	response.OK(c, nil)
}

// ==================== Order Handlers ====================

func (s *Service) CreateOrder(c *gin.Context) {
	var o StoreOrder
	if err := c.ShouldBindJSON(&o); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	o.OrderNo = fmt.Sprintf("SO%s%d", time.Now().Format("20060102"), time.Now().UnixNano()%10000)
	o.Status = "pending"
	s.DB().Create(&o)
	s.DB().Model(&MerchantStore{}).Where("id = ?", o.StoreID).UpdateColumn("order_count", gorm.Expr("order_count + 1"))
	response.Created(c, o)
}

func (s *Service) ListOrders(c *gin.Context) {
	var items []StoreOrder
	var total int64
	q := s.DB().Model(&StoreOrder{})
	if sid := c.Query("store_id"); sid != "" {
		q = q.Where("store_id = ?", sid)
	}
	if uid := c.Query("buyer_user_id"); uid != "" {
		q = q.Where("buyer_user_id = ?", uid)
	}
	if st := c.Query("status"); st != "" {
		q = q.Where("status = ?", st)
	}
	q.Count(&total)
	paginate(c, q).Order("created_at DESC").Find(&items)
	response.OK(c, gin.H{"items": items, "total": total})
}

func (s *Service) UpdateOrderStatus(c *gin.Context) {
	var body struct {
		Status string `json:"status"`
		Remark string `json:"remark"`
	}
	c.ShouldBindJSON(&body)
	var o StoreOrder
	if err := s.DB().First(&o, c.Param("id")).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	o.Status = body.Status
	o.StatusRemark = body.Remark
	s.DB().Save(&o)
	response.OK(c, o)
}

// ==================== Producer Profile ====================

func (s *Service) UpsertProducerProfile(c *gin.Context) {
	var p ProducerProfile
	if err := c.ShouldBindJSON(&p); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	var existing ProducerProfile
	if s.DB().Where("store_id = ?", p.StoreID).First(&existing).Error == nil {
		p.ID = existing.ID
		s.DB().Save(&p)
	} else {
		s.DB().Create(&p)
	}
	response.OK(c, p)
}

func (s *Service) GetProducerProfile(c *gin.Context) {
	var p ProducerProfile
	if err := s.DB().Where("store_id = ?", c.Param("store_id")).First(&p).Error; err != nil {
		response.NotFound(c, "producer profile not found")
		return
	}
	response.OK(c, p)
}

func (s *Service) CreateWorkOrder(c *gin.Context) {
	var w WorkOrder
	if err := c.ShouldBindJSON(&w); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	w.WorkOrderNo = fmt.Sprintf("WO%s%d", time.Now().Format("20060102"), time.Now().UnixNano()%10000)
	s.DB().Create(&w)
	response.Created(c, w)
}

func (s *Service) ListWorkOrders(c *gin.Context) {
	var items []WorkOrder
	var total int64
	q := s.DB().Model(&WorkOrder{})
	if sid := c.Query("store_id"); sid != "" {
		q = q.Where("store_id = ?", sid)
	}
	if st := c.Query("status"); st != "" {
		q = q.Where("status = ?", st)
	}
	q.Count(&total)
	paginate(c, q).Order("created_at DESC").Find(&items)
	response.OK(c, gin.H{"items": items, "total": total})
}

func (s *Service) UpdateWorkOrder(c *gin.Context) {
	var w WorkOrder
	if err := s.DB().First(&w, c.Param("id")).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	c.ShouldBindJSON(&w)
	if w.Status == "completed" {
		now := time.Now()
		w.CompleteDate = &now
	}
	s.DB().Save(&w)
	response.OK(c, w)
}

// ==================== Warehouse Profile ====================

func (s *Service) UpsertWarehouseProfile(c *gin.Context) {
	var w WarehouseProfile
	if err := c.ShouldBindJSON(&w); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	var existing WarehouseProfile
	if s.DB().Where("store_id = ?", w.StoreID).First(&existing).Error == nil {
		w.ID = existing.ID
		s.DB().Save(&w)
	} else {
		s.DB().Create(&w)
	}
	response.OK(c, w)
}

func (s *Service) GetWarehouseProfile(c *gin.Context) {
	var w WarehouseProfile
	if err := s.DB().Where("store_id = ?", c.Param("store_id")).First(&w).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	response.OK(c, w)
}

func (s *Service) ListInventory(c *gin.Context) {
	var items []InventoryRecord
	var total int64
	q := s.DB().Model(&InventoryRecord{})
	if sid := c.Query("store_id"); sid != "" {
		q = q.Where("store_id = ?", sid)
	}
	if sku := c.Query("sku_code"); sku != "" {
		q = q.Where("sku_code = ?", sku)
	}
	q.Count(&total)
	paginate(c, q).Order("created_at DESC").Find(&items)
	response.OK(c, gin.H{"items": items, "total": total})
}

func (s *Service) UpsertInventory(c *gin.Context) {
	var rec InventoryRecord
	if err := c.ShouldBindJSON(&rec); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	var existing InventoryRecord
	if s.DB().Where("store_id = ? AND sku_code = ? AND location_code = ? AND batch_no = ?",
		rec.StoreID, rec.SKUCode, rec.LocationCode, rec.BatchNo).First(&existing).Error == nil {
		rec.ID = existing.ID
		s.DB().Save(&rec)
	} else {
		s.DB().Create(&rec)
	}
	response.OK(c, rec)
}

func (s *Service) CreateOutboundOrder(c *gin.Context) {
	var o OutboundOrder
	if err := c.ShouldBindJSON(&o); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	o.OutboundNo = fmt.Sprintf("OB%s%d", time.Now().Format("20060102"), time.Now().UnixNano()%10000)
	s.DB().Create(&o)
	response.Created(c, o)
}

func (s *Service) UpdateOutboundStatus(c *gin.Context) {
	var body struct {
		Status     string `json:"status"`
		Carrier    string `json:"carrier"`
		TrackingNo string `json:"tracking_no"`
	}
	c.ShouldBindJSON(&body)
	var o OutboundOrder
	if err := s.DB().First(&o, c.Param("id")).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	o.Status = body.Status
	now := time.Now()
	switch body.Status {
	case "picked":
		o.PickedAt = &now
	case "packed":
		o.PackedAt = &now
	case "shipped":
		o.ShippedAt = &now
		o.Carrier = body.Carrier
		o.TrackingNo = body.TrackingNo
	}
	s.DB().Save(&o)
	response.OK(c, o)
}

// ==================== Logistics Profile ====================

func (s *Service) UpsertLogisticsProfile(c *gin.Context) {
	var l LogisticsProfile
	if err := c.ShouldBindJSON(&l); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	var existing LogisticsProfile
	if s.DB().Where("store_id = ?", l.StoreID).First(&existing).Error == nil {
		l.ID = existing.ID
		s.DB().Save(&l)
	} else {
		s.DB().Create(&l)
	}
	response.OK(c, l)
}

func (s *Service) GetLogisticsProfile(c *gin.Context) {
	var l LogisticsProfile
	if err := s.DB().Where("store_id = ?", c.Param("store_id")).First(&l).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	response.OK(c, l)
}

func (s *Service) CreateShipment(c *gin.Context) {
	var sh Shipment
	if err := c.ShouldBindJSON(&sh); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	sh.ShipmentNo = fmt.Sprintf("SF%s%d", time.Now().Format("20060102"), time.Now().UnixNano()%10000)
	s.DB().Create(&sh)
	response.Created(c, sh)
}

func (s *Service) ListShipments(c *gin.Context) {
	var items []Shipment
	var total int64
	q := s.DB().Model(&Shipment{})
	if sid := c.Query("store_id"); sid != "" {
		q = q.Where("store_id = ?", sid)
	}
	if st := c.Query("status"); st != "" {
		q = q.Where("status = ?", st)
	}
	q.Count(&total)
	paginate(c, q).Order("created_at DESC").Find(&items)
	response.OK(c, gin.H{"items": items, "total": total})
}

func (s *Service) GetShipment(c *gin.Context) {
	var sh Shipment
	if err := s.DB().First(&sh, c.Param("id")).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	var events []TrackingEvent
	s.DB().Where("shipment_id = ?", sh.ID).Order("occurred_at ASC").Find(&events)
	response.OK(c, gin.H{"shipment": sh, "tracking_events": events})
}

func (s *Service) TrackShipment(c *gin.Context) {
	var sh Shipment
	if err := s.DB().Where("shipment_no = ?", c.Param("shipment_no")).First(&sh).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	var events []TrackingEvent
	s.DB().Where("shipment_id = ?", sh.ID).Order("occurred_at ASC").Find(&events)
	response.OK(c, gin.H{"shipment": sh, "tracking_events": events})
}

func (s *Service) UpdateShipmentStatus(c *gin.Context) {
	var body struct {
		Status      string  `json:"status"`
		Location    string  `json:"location"`
		Description string  `json:"description"`
		Freight     float64 `json:"freight"`
	}
	c.ShouldBindJSON(&body)
	var sh Shipment
	if err := s.DB().First(&sh, c.Param("id")).Error; err != nil {
		response.NotFound(c, "not found")
		return
	}
	sh.Status = body.Status
	if body.Location != "" {
		sh.CurrentLocation = body.Location
	}
	if body.Status == "delivered" {
		now := time.Now()
		sh.DeliveredAt = &now
	}
	if body.Freight > 0 {
		sh.Freight = body.Freight
	}
	s.DB().Save(&sh)
	event := TrackingEvent{
		ShipmentID:  sh.ID,
		Status:      body.Status,
		Location:    body.Location,
		Description: body.Description,
		OccurredAt:  time.Now(),
	}
	s.DB().Create(&event)
	response.OK(c, sh)
}

// ==================== Helpers ====================

func defaultCapabilities(storeType string) string {
	switch storeType {
	case "supplier":
		return `["supply","quote","po","fulfillment"]`
	case "producer":
		return `["production","capacity","work_order"]`
	case "warehouse":
		return `["storage","inventory","inbound","outbound"]`
	case "logistics":
		return `["shipping","tracking","freight"]`
	default:
		return `["basic"]`
	}
}

func paginate(c *gin.Context, db *gorm.DB) *gorm.DB {
	page, _ := parseInt(c.DefaultQuery("page", "1"))
	size, _ := parseInt(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	return db.Offset((page - 1) * size).Limit(size)
}

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
