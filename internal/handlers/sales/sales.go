package sales

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"warehouse_system/internal/config"
	db "warehouse_system/internal/database/db"
	"warehouse_system/internal/handlers"
	"warehouse_system/internal/middlewares"
)

type SalesHandler struct {
	h *handlers.Handler
}

func NewSalesHandler(h *handlers.Handler) *SalesHandler {
	return &SalesHandler{h: h}
}

type SalesOrderItemRequest struct {
	MaterialID      int32   `json:"material_id"`
	Quantity        float64 `json:"quantity"`
	UnitPrice       float64 `json:"unit_price"`
	ShippedQuantity float64 `json:"shipped_quantity"`
}

type CreateSalesOrderRequest struct {
	OrderNumber          string                  `json:"order_number"`
	CustomerID           int32                   `json:"customer_id"`
	OrderDate            *string                 `json:"order_date"`
	ExpectedDeliveryDate *string                 `json:"expected_delivery_date"`
	Status               string                  `json:"status"`
	Items                []SalesOrderItemRequest `json:"items"`
	Meta                 json.RawMessage         `json:"meta"`
}

type UpdateSalesOrderRequest struct {
	OrderNumber          *string         `json:"order_number,omitempty"`
	CustomerID           *int32          `json:"customer_id,omitempty"`
	OrderDate            *string         `json:"order_date,omitempty"`
	ExpectedDeliveryDate *string         `json:"expected_delivery_date,omitempty"`
	Status               *string         `json:"status,omitempty"`
	ApprovedBy           *int32          `json:"approved_by,omitempty"`
	Meta                 json.RawMessage `json:"meta,omitempty"`
}

// parseFlexibleDate parses date strings in multiple formats
func parseFlexibleDate(dateStr string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse date: %s", dateStr)
}

func isValidSOStatus(s string) bool {
	validStatuses := []string{"Pending", "Approved", "Shipped", "Cancelled", "Partial"}
	for _, valid := range validStatuses {
		if s == valid {
			return true
		}
	}
	return false
}

// CreateSalesOrder creates a new sales order with items.
func (so *SalesHandler) CreateSalesOrder(w http.ResponseWriter, r *http.Request) {
	var req CreateSalesOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondBadRequest(w, "Invalid request payload", err.Error())
		return
	}

	if req.OrderNumber == "" || len(req.Items) == 0 {
		config.RespondBadRequest(w, "Missing required fields", "Order number and at least one item are required")
		return
	}

	// Set default status if not provided
	if req.Status == "" {
		req.Status = "Pending"
	}

	// Validate status
	if !isValidSOStatus(req.Status) {
		config.RespondBadRequest(w, "Invalid status", "Status must be one of: Pending, Approved, Shipped, Cancelled, Partial")
		return
	}

	// Check for duplicate order number
	_, err := so.h.Queries.GetSalesOrderByOrderNumber(context.Background(), req.OrderNumber)
	if err == nil {
		config.RespondJSON(w, http.StatusConflict, map[string]string{"error": "Sales order number already exists"})
		return
	}

	// Calculate total amount from items
	var totalAmount float64
	for _, item := range req.Items {
		if item.Quantity <= 0 || item.UnitPrice <= 0 {
			config.RespondBadRequest(w, "Invalid item data", "Quantity and unit price must be greater than 0")
			return
		}
		if item.ShippedQuantity < 0 {
			config.RespondBadRequest(w, "Invalid item data", "Shipped quantity cannot be negative")
			return
		}
		totalAmount += item.Quantity * item.UnitPrice
	}

	// Validate date logic: order_date should be before expected_delivery_date
	var orderDate, expectedDate time.Time
	if req.OrderDate != nil && *req.OrderDate != "" {
		parsedDate, err := parseFlexibleDate(*req.OrderDate)
		if err != nil {
			config.RespondBadRequest(w, "Invalid order date format", err.Error())
			return
		}
		orderDate = parsedDate
	} else {
		orderDate = time.Now()
	}

	if req.ExpectedDeliveryDate != nil && *req.ExpectedDeliveryDate != "" {
		parsedDate, err := parseFlexibleDate(*req.ExpectedDeliveryDate)
		if err != nil {
			config.RespondBadRequest(w, "Invalid expected delivery date format", err.Error())
			return
		}
		expectedDate = parsedDate

		// Check if order date is before expected delivery date
		if !orderDate.Before(expectedDate) {
			config.RespondBadRequest(w, "Invalid dates", "Order date must be before expected delivery date")
			return
		}
	}

	// Get current user ID from context
	var userID int32
	if session, ok := middlewares.GetSessionFromContext(r); ok {
		if uid, err := fmt.Sscanf(session.UserID, "%d", &userID); err == nil && uid == 1 {
			// User ID successfully parsed
		}
	}

	params := db.CreateSalesOrderParams{
		OrderNumber: req.OrderNumber,
		CustomerID:  pgtype.Int4{Int32: req.CustomerID, Valid: true},
		Status:      req.Status,
		CreatedBy:   pgtype.Int4{Int32: userID, Valid: true},
		Meta:        req.Meta,
	}

	params.TotalAmount = pgtype.Numeric{Valid: true}
	params.TotalAmount.Scan(fmt.Sprintf("%.4f", totalAmount))

	if req.OrderDate != nil && *req.OrderDate != "" {
		params.OrderDate = pgtype.Timestamptz{Time: orderDate, Valid: true}
	} else {
		params.OrderDate = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	}

	if req.ExpectedDeliveryDate != nil && *req.ExpectedDeliveryDate != "" {
		params.ExpectedDeliveryDate = pgtype.Timestamptz{Time: expectedDate, Valid: true}
	}

	salesOrder, err := so.h.Queries.CreateSalesOrder(context.Background(), params)
	if err != nil {
		so.h.Logger.Error("Failed to create sales order", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Create items
	items := make([]db.SalesOrderItem, 0, len(req.Items))
	for _, item := range req.Items {
		// Check if material is saleable
		saleable, err := so.h.Queries.CheckMaterialSaleable(context.Background(), item.MaterialID)
		if err != nil {
			config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("Material with ID %d not found", item.MaterialID)})
			return
		}
		if !saleable.Valid || !saleable.Bool {
			config.RespondBadRequest(w, "Invalid material", fmt.Sprintf("Material with ID %d is not saleable and cannot be added to sales orders", item.MaterialID))
			return
		}

		totalPrice := item.Quantity * item.UnitPrice

		itemParams := db.CreateSalesOrderItemParams{
			SalesOrderID: pgtype.Int4{Int32: salesOrder.ID, Valid: true},
			MaterialID:   pgtype.Int4{Int32: item.MaterialID, Valid: true},
		}

		itemParams.Quantity = pgtype.Numeric{Valid: true}
		itemParams.Quantity.Scan(fmt.Sprintf("%.4f", item.Quantity))

		itemParams.UnitPrice = pgtype.Numeric{Valid: true}
		itemParams.UnitPrice.Scan(fmt.Sprintf("%.4f", item.UnitPrice))

		itemParams.TotalPrice = pgtype.Numeric{Valid: true}
		itemParams.TotalPrice.Scan(fmt.Sprintf("%.4f", totalPrice))

		itemParams.ShippedQuantity = pgtype.Numeric{Valid: true}
		itemParams.ShippedQuantity.Scan(fmt.Sprintf("%.4f", item.ShippedQuantity))

		createdItem, err := so.h.Queries.CreateSalesOrderItem(context.Background(), itemParams)
		if err != nil {
			so.h.Logger.Error("Failed to create sales order item", "error", err)
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		items = append(items, createdItem)
	}

	config.RespondJSON(w, http.StatusCreated, map[string]any{
		"sales_order": salesOrder,
		"items":       items,
	})
}

// GetSalesOrder retrieves a sales order by ID with all its items.
func (so *SalesHandler) GetSalesOrder(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing sales order ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid sales order ID format", err.Error())
		return
	}

	salesOrder, err := so.h.Queries.GetSalesOrderByID(context.Background(), id)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Sales order not found"})
		return
	}

	items, err := so.h.Queries.ListSalesOrderItems(context.Background(), pgtype.Int4{Int32: id, Valid: true})
	if err != nil {
		so.h.Logger.Error("Failed to get sales order items", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]any{
		"sales_order": salesOrder,
		"items":       items,
	})
}

// UpdateSalesOrder updates an existing sales order.
func (so *SalesHandler) UpdateSalesOrder(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing sales order ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid sales order ID format", err.Error())
		return
	}

	var req UpdateSalesOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondBadRequest(w, "Invalid request payload", err.Error())
		return
	}

	// Validate status if provided
	if req.Status != nil && *req.Status != "" && !isValidSOStatus(*req.Status) {
		config.RespondBadRequest(w, "Invalid status", "Status must be one of: Pending, Approved, Shipped, Cancelled, Partial")
		return
	}

	// Validate date logic if both dates are being updated
	if req.OrderDate != nil && req.ExpectedDeliveryDate != nil && *req.OrderDate != "" && *req.ExpectedDeliveryDate != "" {
		orderDate, err1 := parseFlexibleDate(*req.OrderDate)
		expectedDate, err2 := parseFlexibleDate(*req.ExpectedDeliveryDate)
		if err1 == nil && err2 == nil {
			if !orderDate.Before(expectedDate) {
				config.RespondBadRequest(w, "Invalid dates", "Order date must be before expected delivery date")
				return
			}
		}
	}

	// Get current sales order
	current, err := so.h.Queries.GetSalesOrderByID(context.Background(), id)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Sales order not found"})
		return
	}

	// Check for duplicate order number if being updated
	if req.OrderNumber != nil && *req.OrderNumber != current.OrderNumber {
		_, err := so.h.Queries.GetSalesOrderByOrderNumber(context.Background(), *req.OrderNumber)
		if err == nil {
			config.RespondJSON(w, http.StatusConflict, map[string]string{"error": "Sales order number already exists"})
			return
		}
	}

	params := db.UpdateSalesOrderParams{
		ID: id,
	}

	// Handle order number (interface{} due to NULLIF)
	if req.OrderNumber != nil {
		params.Column2 = *req.OrderNumber
	} else {
		params.Column2 = ""
	}

	// Handle optional fields
	if req.CustomerID != nil {
		params.CustomerID = pgtype.Int4{Int32: *req.CustomerID, Valid: true}
	}
	if req.OrderDate != nil && *req.OrderDate != "" {
		parsedDate, err := parseFlexibleDate(*req.OrderDate)
		if err != nil {
			config.RespondBadRequest(w, "Invalid order date format", err.Error())
			return
		}
		params.OrderDate = pgtype.Timestamptz{Time: parsedDate, Valid: true}
	}
	if req.ExpectedDeliveryDate != nil && *req.ExpectedDeliveryDate != "" {
		parsedDate, err := parseFlexibleDate(*req.ExpectedDeliveryDate)
		if err != nil {
			config.RespondBadRequest(w, "Invalid expected delivery date format", err.Error())
			return
		}
		params.ExpectedDeliveryDate = pgtype.Timestamptz{Time: parsedDate, Valid: true}
	}
	if req.Status != nil {
		params.Column6 = *req.Status
	} else {
		params.Column6 = ""
	}
	if req.ApprovedBy != nil {
		params.ApprovedBy = pgtype.Int4{Int32: *req.ApprovedBy, Valid: true}
	}
	if req.Meta != nil {
		params.Meta = req.Meta
	}

	salesOrder, err := so.h.Queries.UpdateSalesOrder(context.Background(), params)
	if err != nil {
		so.h.Logger.Error("Failed to update sales order", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, salesOrder)
}

// DeleteSalesOrder deletes a sales order.
func (so *SalesHandler) DeleteSalesOrder(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing sales order ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid sales order ID format", err.Error())
		return
	}

	// Check if sales order exists
	_, err := so.h.Queries.GetSalesOrderByID(context.Background(), id)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Sales order not found"})
		return
	}

	err = so.h.Queries.DeleteSalesOrder(context.Background(), id)
	if err != nil {
		so.h.Logger.Error("Failed to delete sales order", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]string{"message": "Sales order deleted successfully"})
}

// ListSalesOrders returns a paginated list of sales orders with optional search.
func (so *SalesHandler) ListSalesOrders(w http.ResponseWriter, r *http.Request) {
	pagination := middlewares.GetPagination(r.Context())

	query := r.URL.Query().Get("q")

	var salesOrders []db.SalesOrder
	var total int64
	var err error

	if query != "" {
		salesOrders, err = so.h.Queries.SearchSalesOrders(context.Background(), db.SearchSalesOrdersParams{
			Query:  pgtype.Text{String: query, Valid: true},
			Limit:  int32(pagination.Limit),
			Offset: int32(pagination.Offset),
		})
		if err != nil {
			so.h.Logger.Error("Failed to search sales orders", "error", err)
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		count, err := so.h.Queries.CountSearchSalesOrders(context.Background(), pgtype.Text{String: query, Valid: true})
		if err != nil {
			so.h.Logger.Error("Failed to count search sales orders", "error", err)
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		total = count
	} else {
		salesOrders, err = so.h.Queries.ListSalesOrders(context.Background(), db.ListSalesOrdersParams{
			Limit:  int32(pagination.Limit),
			Offset: int32(pagination.Offset),
		})
		if err != nil {
			so.h.Logger.Error("Failed to list sales orders", "error", err)
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		count, err := so.h.Queries.CountSalesOrders(context.Background())
		if err != nil {
			so.h.Logger.Error("Failed to count sales orders", "error", err)
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		total = count
	}

	pagination.Total = total
	totalPages := (total + int64(pagination.Limit) - 1) / int64(pagination.Limit)

	config.RespondJSON(w, http.StatusOK, map[string]any{
		"sales_orders": salesOrders,
		"pagination": map[string]any{
			"page":        pagination.Page,
			"limit":       pagination.Limit,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

// GetSalesOrderItems retrieves all items for a sales order.
func (so *SalesHandler) GetSalesOrderItems(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing sales order ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid sales order ID format", err.Error())
		return
	}

	// Check if sales order exists
	_, err := so.h.Queries.GetSalesOrderByID(context.Background(), id)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Sales order not found"})
		return
	}

	items, err := so.h.Queries.ListSalesOrderItems(context.Background(), pgtype.Int4{Int32: id, Valid: true})
	if err != nil {
		so.h.Logger.Error("Failed to get sales order items", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]any{
		"items": items,
	})
}

// UpdateSalesOrderItem updates a single sales order item.
func (so *SalesHandler) UpdateSalesOrderItem(w http.ResponseWriter, r *http.Request) {
	soIDStr := r.PathValue("id")
	itemIDStr := r.PathValue("item_id")

	if soIDStr == "" || itemIDStr == "" {
		config.RespondBadRequest(w, "Missing sales order ID or item ID", "")
		return
	}

	var soID, itemID int32
	if _, err := fmt.Sscanf(soIDStr, "%d", &soID); err != nil {
		config.RespondBadRequest(w, "Invalid sales order ID format", err.Error())
		return
	}
	if _, err := fmt.Sscanf(itemIDStr, "%d", &itemID); err != nil {
		config.RespondBadRequest(w, "Invalid item ID format", err.Error())
		return
	}

	var req struct {
		MaterialID      *int32   `json:"material_id,omitempty"`
		Quantity        *float64 `json:"quantity,omitempty"`
		UnitPrice       *float64 `json:"unit_price,omitempty"`
		ShippedQuantity *float64 `json:"shipped_quantity,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondBadRequest(w, "Invalid request payload", err.Error())
		return
	}

	// Check if item exists and belongs to this SO
	currentItem, err := so.h.Queries.GetSalesOrderItemByID(context.Background(), itemID)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Item not found"})
		return
	}

	if currentItem.SalesOrderID.Int32 != soID {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Item does not belong to this sales order"})
		return
	}

	// Check SO status - can only update items when status is Pending
	salesOrder, err := so.h.Queries.GetSalesOrderByID(context.Background(), soID)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Sales order not found"})
		return
	}

	if salesOrder.Status == "Cancelled" {
		config.RespondJSON(w, http.StatusForbidden, map[string]string{"error": "Cannot modify items of a cancelled sales order"})
		return
	}

	if salesOrder.Status != "Pending" {
		config.RespondJSON(w, http.StatusForbidden, map[string]string{"error": "Can only update items when sales order status is Pending"})
		return
	}

	// Validate quantities if being updated
	if req.Quantity != nil && *req.Quantity <= 0 {
		config.RespondBadRequest(w, "Invalid quantity", "Quantity must be greater than 0")
		return
	}
	if req.UnitPrice != nil && *req.UnitPrice <= 0 {
		config.RespondBadRequest(w, "Invalid price", "Unit price must be greater than 0")
		return
	}
	if req.ShippedQuantity != nil && *req.ShippedQuantity < 0 {
		config.RespondBadRequest(w, "Invalid shipped quantity", "Shipped quantity cannot be negative")
		return
	}

	// Check if material is saleable when changing material_id
	if req.MaterialID != nil {
		saleable, err := so.h.Queries.CheckMaterialSaleable(context.Background(), *req.MaterialID)
		if err != nil {
			config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("Material with ID %d not found", *req.MaterialID)})
			return
		}
		if !saleable.Valid || !saleable.Bool {
			config.RespondBadRequest(w, "Invalid material", fmt.Sprintf("Material with ID %d is not saleable and cannot be added to sales orders", *req.MaterialID))
			return
		}
	}

	params := db.UpdateSalesOrderItemParams{
		ID: itemID,
	}

	// Update fields if provided
	if req.MaterialID != nil {
		params.MaterialID = pgtype.Int4{Int32: *req.MaterialID, Valid: true}
	}
	if req.Quantity != nil {
		params.Quantity = pgtype.Numeric{Valid: true}
		params.Quantity.Scan(fmt.Sprintf("%.4f", *req.Quantity))
	}
	if req.UnitPrice != nil {
		params.UnitPrice = pgtype.Numeric{Valid: true}
		params.UnitPrice.Scan(fmt.Sprintf("%.4f", *req.UnitPrice))
	}
	if req.ShippedQuantity != nil {
		params.ShippedQuantity = pgtype.Numeric{Valid: true}
		params.ShippedQuantity.Scan(fmt.Sprintf("%.4f", *req.ShippedQuantity))
	}

	// Recalculate total price if quantity or unit price changed
	if req.Quantity != nil || req.UnitPrice != nil {
		var quantity, unitPrice float64

		if req.Quantity != nil {
			quantity = *req.Quantity
		} else {
			// Get current quantity
			currentItem.Quantity.Scan(&quantity)
		}

		if req.UnitPrice != nil {
			unitPrice = *req.UnitPrice
		} else {
			// Get current unit price
			currentItem.UnitPrice.Scan(&unitPrice)
		}

		totalPrice := quantity * unitPrice
		params.TotalPrice = pgtype.Numeric{Valid: true}
		params.TotalPrice.Scan(fmt.Sprintf("%.4f", totalPrice))
	}

	item, err := so.h.Queries.UpdateSalesOrderItem(context.Background(), params)
	if err != nil {
		so.h.Logger.Error("Failed to update sales order item", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, item)
}

// AddSalesOrderItem adds a new item to an existing sales order.
func (so *SalesHandler) AddSalesOrderItem(w http.ResponseWriter, r *http.Request) {
	soIDStr := r.PathValue("id")
	if soIDStr == "" {
		config.RespondBadRequest(w, "Missing sales order ID", "")
		return
	}

	var soID int32
	if _, err := fmt.Sscanf(soIDStr, "%d", &soID); err != nil {
		config.RespondBadRequest(w, "Invalid sales order ID format", err.Error())
		return
	}

	var req struct {
		MaterialID      int32   `json:"material_id"`
		Quantity        float64 `json:"quantity"`
		UnitPrice       float64 `json:"unit_price"`
		ShippedQuantity float64 `json:"shipped_quantity"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondBadRequest(w, "Invalid request payload", err.Error())
		return
	}

	if req.Quantity <= 0 || req.UnitPrice <= 0 {
		config.RespondBadRequest(w, "Invalid data", "Quantity and unit price must be greater than 0")
		return
	}

	if req.ShippedQuantity < 0 {
		config.RespondBadRequest(w, "Invalid data", "Shipped quantity cannot be negative")
		return
	}

	// Check if sales order exists and check status
	salesOrder, err := so.h.Queries.GetSalesOrderByID(context.Background(), soID)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Sales order not found"})
		return
	}

	if salesOrder.Status == "Cancelled" {
		config.RespondJSON(w, http.StatusForbidden, map[string]string{"error": "Cannot add items to a cancelled sales order"})
		return
	}

	if salesOrder.Status != "Pending" {
		config.RespondJSON(w, http.StatusForbidden, map[string]string{"error": "Can only add items when sales order status is Pending"})
		return
	}

	// Check if material is saleable
	saleable, err := so.h.Queries.CheckMaterialSaleable(context.Background(), req.MaterialID)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("Material with ID %d not found", req.MaterialID)})
		return
	}
	if !saleable.Valid || !saleable.Bool {
		config.RespondBadRequest(w, "Invalid material", fmt.Sprintf("Material with ID %d is not saleable and cannot be added to sales orders", req.MaterialID))
		return
	}

	totalPrice := req.Quantity * req.UnitPrice

	params := db.CreateSalesOrderItemParams{
		SalesOrderID: pgtype.Int4{Int32: soID, Valid: true},
		MaterialID:   pgtype.Int4{Int32: req.MaterialID, Valid: true},
	}

	params.Quantity = pgtype.Numeric{Valid: true}
	params.Quantity.Scan(fmt.Sprintf("%.4f", req.Quantity))

	params.UnitPrice = pgtype.Numeric{Valid: true}
	params.UnitPrice.Scan(fmt.Sprintf("%.4f", req.UnitPrice))

	params.TotalPrice = pgtype.Numeric{Valid: true}
	params.TotalPrice.Scan(fmt.Sprintf("%.4f", totalPrice))

	params.ShippedQuantity = pgtype.Numeric{Valid: true}
	params.ShippedQuantity.Scan(fmt.Sprintf("%.4f", req.ShippedQuantity))

	item, err := so.h.Queries.CreateSalesOrderItem(context.Background(), params)
	if err != nil {
		so.h.Logger.Error("Failed to add sales order item", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusCreated, item)
}

// DeleteSalesOrderItem deletes a single item from a sales order.
func (so *SalesHandler) DeleteSalesOrderItem(w http.ResponseWriter, r *http.Request) {
	soIDStr := r.PathValue("id")
	itemIDStr := r.PathValue("item_id")

	if soIDStr == "" || itemIDStr == "" {
		config.RespondBadRequest(w, "Missing sales order ID or item ID", "")
		return
	}

	var soID, itemID int32
	if _, err := fmt.Sscanf(soIDStr, "%d", &soID); err != nil {
		config.RespondBadRequest(w, "Invalid sales order ID format", err.Error())
		return
	}
	if _, err := fmt.Sscanf(itemIDStr, "%d", &itemID); err != nil {
		config.RespondBadRequest(w, "Invalid item ID format", err.Error())
		return
	}

	// Check if item exists and belongs to this SO
	currentItem, err := so.h.Queries.GetSalesOrderItemByID(context.Background(), itemID)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Item not found"})
		return
	}

	if currentItem.SalesOrderID.Int32 != soID {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Item does not belong to this sales order"})
		return
	}

	// Check SO status - can only delete items when status is Pending
	salesOrder, err := so.h.Queries.GetSalesOrderByID(context.Background(), soID)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Sales order not found"})
		return
	}

	if salesOrder.Status == "Cancelled" {
		config.RespondJSON(w, http.StatusForbidden, map[string]string{"error": "Cannot delete items from a cancelled sales order"})
		return
	}

	if salesOrder.Status != "Pending" {
		config.RespondJSON(w, http.StatusForbidden, map[string]string{"error": "Can only delete items when sales order status is Pending"})
		return
	}

	// Check if this is the last item - cannot delete if only 1 item remains
	items, err := so.h.Queries.ListSalesOrderItems(context.Background(), pgtype.Int4{Int32: soID, Valid: true})
	if err != nil {
		so.h.Logger.Error("Failed to count items", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if len(items) <= 1 {
		config.RespondJSON(w, http.StatusForbidden, map[string]string{"error": "Cannot delete the last item. Sales order must have at least one item"})
		return
	}

	err = so.h.Queries.DeleteSalesOrderItem(context.Background(), itemID)
	if err != nil {
		so.h.Logger.Error("Failed to delete sales order item", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]string{"message": "Item deleted successfully"})
}
