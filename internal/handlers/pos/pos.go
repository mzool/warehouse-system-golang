package pos

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

type POSHandler struct {
	h *handlers.Handler
}

func NewPOSHandler(h *handlers.Handler) *POSHandler {
	return &POSHandler{h: h}
}

type PurchaseOrderItemRequest struct {
	MaterialID       int32   `json:"material_id"`
	Quantity         float64 `json:"quantity"`
	UnitPrice        float64 `json:"unit_price"`
	ReceivedQuantity float64 `json:"received_quantity"`
}

type CreatePurchaseOrderRequest struct {
	OrderNumber          string                     `json:"order_number"`
	SupplierID           int32                      `json:"supplier_id"`
	OrderDate            *string                    `json:"order_date"`
	ExpectedDeliveryDate *string                    `json:"expected_delivery_date"`
	Status               string                     `json:"status"`
	Items                []PurchaseOrderItemRequest `json:"items"`
	Meta                 json.RawMessage            `json:"meta"`
}

type UpdatePurchaseOrderRequest struct {
	OrderNumber          *string         `json:"order_number,omitempty"`
	SupplierID           *int32          `json:"supplier_id,omitempty"`
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

func isValidPOStatus(s string) bool {
	validStatuses := []string{"Pending", "Approved", "Received", "Cancelled", "Partial"}
	for _, valid := range validStatuses {
		if s == valid {
			return true
		}
	}
	return false
}

// CreatePurchaseOrder creates a new purchase order with items.
func (po *POSHandler) CreatePurchaseOrder(w http.ResponseWriter, r *http.Request) {
	var req CreatePurchaseOrderRequest
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
	if !isValidPOStatus(req.Status) {
		config.RespondBadRequest(w, "Invalid status", "Status must be one of: Pending, Approved, Received, Cancelled, Partial")
		return
	}

	// Check for duplicate order number
	_, err := po.h.Queries.GetPurchaseOrderByOrderNumber(context.Background(), req.OrderNumber)
	if err == nil {
		config.RespondJSON(w, http.StatusConflict, map[string]string{"error": "Purchase order number already exists"})
		return
	}

	// Calculate total amount from items
	var totalAmount float64
	for _, item := range req.Items {
		if item.Quantity <= 0 || item.UnitPrice <= 0 {
			config.RespondBadRequest(w, "Invalid item data", "Quantity and unit price must be greater than 0")
			return
		}
		if item.ReceivedQuantity < 0 {
			config.RespondBadRequest(w, "Invalid item data", "Received quantity cannot be negative")
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

	params := db.CreatePurchaseOrderParams{
		OrderNumber: req.OrderNumber,
		Status:      req.Status,
	}

	if req.SupplierID > 0 {
		params.SupplierID = pgtype.Int4{Int32: req.SupplierID, Valid: true}
	}
	if req.OrderDate != nil && *req.OrderDate != "" {
		orderDate, err := parseFlexibleDate(*req.OrderDate)
		if err != nil {
			config.RespondBadRequest(w, "Invalid order date format", err.Error())
			return
		}
		params.OrderDate = pgtype.Timestamptz{Time: orderDate, Valid: true}
	} else {
		params.OrderDate = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	}
	if req.ExpectedDeliveryDate != nil && *req.ExpectedDeliveryDate != "" {
		expectedDate, err := parseFlexibleDate(*req.ExpectedDeliveryDate)
		if err != nil {
			config.RespondBadRequest(w, "Invalid expected delivery date format", err.Error())
			return
		}
		params.ExpectedDeliveryDate = pgtype.Timestamptz{Time: expectedDate, Valid: true}
	}
	if userID > 0 {
		params.CreatedBy = pgtype.Int4{Int32: userID, Valid: true}
	}
	if req.Meta != nil {
		params.Meta = req.Meta
	}

	// Set total amount
	params.TotalAmount = pgtype.Numeric{Valid: true}
	params.TotalAmount.Scan(fmt.Sprintf("%.4f", totalAmount))

	// Create purchase order
	purchaseOrder, err := po.h.Queries.CreatePurchaseOrder(context.Background(), params)
	if err != nil {
		po.h.Logger.Error("Failed to create purchase order", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Create purchase order items
	var items []db.PurchaseOrderItem
	for _, item := range req.Items {
		totalPrice := item.Quantity * item.UnitPrice

		itemParams := db.CreatePurchaseOrderItemParams{
			PurchaseOrderID: pgtype.Int4{Int32: purchaseOrder.ID, Valid: true},
			MaterialID:      pgtype.Int4{Int32: item.MaterialID, Valid: true},
		}

		itemParams.Quantity = pgtype.Numeric{Valid: true}
		itemParams.Quantity.Scan(fmt.Sprintf("%.4f", item.Quantity))

		itemParams.UnitPrice = pgtype.Numeric{Valid: true}
		itemParams.UnitPrice.Scan(fmt.Sprintf("%.4f", item.UnitPrice))

		itemParams.TotalPrice = pgtype.Numeric{Valid: true}
		itemParams.TotalPrice.Scan(fmt.Sprintf("%.4f", totalPrice))

		itemParams.ReceivedQuantity = pgtype.Numeric{Valid: true}
		itemParams.ReceivedQuantity.Scan(fmt.Sprintf("%.4f", item.ReceivedQuantity))

		createdItem, err := po.h.Queries.CreatePurchaseOrderItem(context.Background(), itemParams)
		if err != nil {
			po.h.Logger.Error("Failed to create purchase order item", "error", err)
			// Rollback: delete the purchase order
			po.h.Queries.DeletePurchaseOrder(context.Background(), purchaseOrder.ID)
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		items = append(items, createdItem)
	}

	config.RespondJSON(w, http.StatusCreated, map[string]any{
		"purchase_order": purchaseOrder,
		"items":          items,
	})
}

// GetPurchaseOrder retrieves a purchase order by ID with its items.
func (po *POSHandler) GetPurchaseOrder(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing purchase order ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid purchase order ID format", err.Error())
		return
	}

	purchaseOrder, err := po.h.Queries.GetPurchaseOrderByID(context.Background(), id)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Purchase order not found"})
		return
	}

	// Get items
	items, err := po.h.Queries.ListPurchaseOrderItems(context.Background(), pgtype.Int4{Int32: id, Valid: true})
	if err != nil {
		po.h.Logger.Error("Failed to get purchase order items", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]any{
		"purchase_order": purchaseOrder,
		"items":          items,
	})
}

// UpdatePurchaseOrder updates an existing purchase order.
func (po *POSHandler) UpdatePurchaseOrder(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing purchase order ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid purchase order ID format", err.Error())
		return
	}

	var req UpdatePurchaseOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondBadRequest(w, "Invalid request payload", err.Error())
		return
	}

	// Validate status if provided
	if req.Status != nil && *req.Status != "" && !isValidPOStatus(*req.Status) {
		config.RespondBadRequest(w, "Invalid status", "Status must be one of: Pending, Approved, Received, Cancelled, Partial")
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

	// Get current purchase order
	current, err := po.h.Queries.GetPurchaseOrderByID(context.Background(), id)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Purchase order not found"})
		return
	}

	// Check for duplicate order number if being updated
	if req.OrderNumber != nil && *req.OrderNumber != current.OrderNumber {
		_, err := po.h.Queries.GetPurchaseOrderByOrderNumber(context.Background(), *req.OrderNumber)
		if err == nil {
			config.RespondJSON(w, http.StatusConflict, map[string]string{"error": "Purchase order number already exists"})
			return
		}
	}

	params := db.UpdatePurchaseOrderParams{
		ID: id,
	}

	// Handle order number (interface{} due to NULLIF)
	if req.OrderNumber != nil {
		params.Column2 = *req.OrderNumber
	} else {
		params.Column2 = ""
	}

	// Handle optional fields
	if req.SupplierID != nil {
		params.SupplierID = pgtype.Int4{Int32: *req.SupplierID, Valid: true}
	}
	if req.OrderDate != nil && *req.OrderDate != "" {
		orderDate, err := parseFlexibleDate(*req.OrderDate)
		if err != nil {
			config.RespondBadRequest(w, "Invalid order date format", err.Error())
			return
		}
		params.OrderDate = pgtype.Timestamptz{Time: orderDate, Valid: true}
	}
	if req.ExpectedDeliveryDate != nil && *req.ExpectedDeliveryDate != "" {
		expectedDate, err := parseFlexibleDate(*req.ExpectedDeliveryDate)
		if err != nil {
			config.RespondBadRequest(w, "Invalid expected delivery date format", err.Error())
			return
		}
		params.ExpectedDeliveryDate = pgtype.Timestamptz{Time: expectedDate, Valid: true}
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

	purchaseOrder, err := po.h.Queries.UpdatePurchaseOrder(context.Background(), params)
	if err != nil {
		po.h.Logger.Error("Failed to update purchase order", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, purchaseOrder)
}

// DeletePurchaseOrder deletes a purchase order by ID.
func (po *POSHandler) DeletePurchaseOrder(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing purchase order ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid purchase order ID format", err.Error())
		return
	}

	// Check if purchase order exists
	_, err := po.h.Queries.GetPurchaseOrderByID(context.Background(), id)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Purchase order not found"})
		return
	}

	err = po.h.Queries.DeletePurchaseOrder(context.Background(), id)
	if err != nil {
		po.h.Logger.Error("Failed to delete purchase order", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]string{"message": "Purchase order deleted successfully"})
}

// ListPurchaseOrders lists all purchase orders with pagination.
func (po *POSHandler) ListPurchaseOrders(w http.ResponseWriter, r *http.Request) {
	pagination := middlewares.GetPagination(r.Context())

	var purchaseOrders []db.PurchaseOrder
	var err error
	var totalCount int64

	// Check if there's a search query
	query := r.URL.Query().Get("query")
	if query != "" {
		// Search purchase orders
		purchaseOrders, err = po.h.Queries.SearchPurchaseOrders(context.Background(), db.SearchPurchaseOrdersParams{
			Limit:  int32(pagination.Limit),
			Offset: int32(pagination.Offset),
			Query:  pgtype.Text{String: query, Valid: true},
		})
		if err != nil {
			po.h.Logger.Error("Failed to search purchase orders", "error", err)
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		// Get search total count
		totalCount, err = po.h.Queries.CountSearchPurchaseOrders(context.Background(), pgtype.Text{String: query, Valid: true})
		if err != nil {
			po.h.Logger.Error("Failed to count search results", "error", err)
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	} else {
		// List all purchase orders
		purchaseOrders, err = po.h.Queries.ListPurchaseOrders(context.Background(), db.ListPurchaseOrdersParams{
			Limit:  int32(pagination.Limit),
			Offset: int32(pagination.Offset),
		})
		if err != nil {
			po.h.Logger.Error("Failed to list purchase orders", "error", err)
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		// Get total count
		totalCount, err = po.h.Queries.CountPurchaseOrders(context.Background())
		if err != nil {
			po.h.Logger.Error("Failed to count purchase orders", "error", err)
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}

	// Set total and build pagination metadata
	pagination.Total = totalCount

	config.RespondJSON(w, http.StatusOK, map[string]any{
		"purchase_orders": purchaseOrders,
		"pagination":      pagination.BuildMeta(),
	})
}

// GetPurchaseOrderItems retrieves all items for a purchase order.
func (po *POSHandler) GetPurchaseOrderItems(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing purchase order ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid purchase order ID format", err.Error())
		return
	}

	// Check if purchase order exists
	_, err := po.h.Queries.GetPurchaseOrderByID(context.Background(), id)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Purchase order not found"})
		return
	}

	items, err := po.h.Queries.ListPurchaseOrderItems(context.Background(), pgtype.Int4{Int32: id, Valid: true})
	if err != nil {
		po.h.Logger.Error("Failed to get purchase order items", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]any{
		"items": items,
	})
}

// UpdatePurchaseOrderItem updates a single purchase order item.
func (po *POSHandler) UpdatePurchaseOrderItem(w http.ResponseWriter, r *http.Request) {
	poIDStr := r.PathValue("id")
	itemIDStr := r.PathValue("item_id")

	if poIDStr == "" || itemIDStr == "" {
		config.RespondBadRequest(w, "Missing purchase order ID or item ID", "")
		return
	}

	var poID, itemID int32
	if _, err := fmt.Sscanf(poIDStr, "%d", &poID); err != nil {
		config.RespondBadRequest(w, "Invalid purchase order ID format", err.Error())
		return
	}
	if _, err := fmt.Sscanf(itemIDStr, "%d", &itemID); err != nil {
		config.RespondBadRequest(w, "Invalid item ID format", err.Error())
		return
	}

	var req struct {
		MaterialID       *int32   `json:"material_id,omitempty"`
		Quantity         *float64 `json:"quantity,omitempty"`
		UnitPrice        *float64 `json:"unit_price,omitempty"`
		ReceivedQuantity *float64 `json:"received_quantity,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondBadRequest(w, "Invalid request payload", err.Error())
		return
	}

	// Check if item exists and belongs to this PO
	currentItem, err := po.h.Queries.GetPurchaseOrderItemByID(context.Background(), itemID)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Item not found"})
		return
	}

	if currentItem.PurchaseOrderID.Int32 != poID {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Item does not belong to this purchase order"})
		return
	}

	// Check PO status - can only update items when status is Pending
	purchaseOrder, err := po.h.Queries.GetPurchaseOrderByID(context.Background(), poID)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Purchase order not found"})
		return
	}

	if purchaseOrder.Status == "Cancelled" {
		config.RespondJSON(w, http.StatusForbidden, map[string]string{"error": "Cannot modify items of a cancelled purchase order"})
		return
	}

	if purchaseOrder.Status != "Pending" {
		config.RespondJSON(w, http.StatusForbidden, map[string]string{"error": "Can only update items when purchase order status is Pending"})
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
	if req.ReceivedQuantity != nil && *req.ReceivedQuantity < 0 {
		config.RespondBadRequest(w, "Invalid received quantity", "Received quantity cannot be negative")
		return
	}

	params := db.UpdatePurchaseOrderItemParams{
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
	if req.ReceivedQuantity != nil {
		params.ReceivedQuantity = pgtype.Numeric{Valid: true}
		params.ReceivedQuantity.Scan(fmt.Sprintf("%.4f", *req.ReceivedQuantity))
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

	item, err := po.h.Queries.UpdatePurchaseOrderItem(context.Background(), params)
	if err != nil {
		po.h.Logger.Error("Failed to update purchase order item", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, item)
}

// AddPurchaseOrderItem adds a new item to an existing purchase order.
func (po *POSHandler) AddPurchaseOrderItem(w http.ResponseWriter, r *http.Request) {
	poIDStr := r.PathValue("id")
	if poIDStr == "" {
		config.RespondBadRequest(w, "Missing purchase order ID", "")
		return
	}

	var poID int32
	if _, err := fmt.Sscanf(poIDStr, "%d", &poID); err != nil {
		config.RespondBadRequest(w, "Invalid purchase order ID format", err.Error())
		return
	}

	var req struct {
		MaterialID       int32   `json:"material_id"`
		Quantity         float64 `json:"quantity"`
		UnitPrice        float64 `json:"unit_price"`
		ReceivedQuantity float64 `json:"received_quantity"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondBadRequest(w, "Invalid request payload", err.Error())
		return
	}

	if req.Quantity <= 0 || req.UnitPrice <= 0 {
		config.RespondBadRequest(w, "Invalid data", "Quantity and unit price must be greater than 0")
		return
	}

	if req.ReceivedQuantity < 0 {
		config.RespondBadRequest(w, "Invalid data", "Received quantity cannot be negative")
		return
	}

	// Check if purchase order exists and check status
	purchaseOrder, err := po.h.Queries.GetPurchaseOrderByID(context.Background(), poID)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Purchase order not found"})
		return
	}

	if purchaseOrder.Status == "Cancelled" {
		config.RespondJSON(w, http.StatusForbidden, map[string]string{"error": "Cannot add items to a cancelled purchase order"})
		return
	}

	if purchaseOrder.Status != "Pending" {
		config.RespondJSON(w, http.StatusForbidden, map[string]string{"error": "Can only add items when purchase order status is Pending"})
		return
	}

	totalPrice := req.Quantity * req.UnitPrice

	params := db.CreatePurchaseOrderItemParams{
		PurchaseOrderID: pgtype.Int4{Int32: poID, Valid: true},
		MaterialID:      pgtype.Int4{Int32: req.MaterialID, Valid: true},
	}

	params.Quantity = pgtype.Numeric{Valid: true}
	params.Quantity.Scan(fmt.Sprintf("%.4f", req.Quantity))

	params.UnitPrice = pgtype.Numeric{Valid: true}
	params.UnitPrice.Scan(fmt.Sprintf("%.4f", req.UnitPrice))

	params.TotalPrice = pgtype.Numeric{Valid: true}
	params.TotalPrice.Scan(fmt.Sprintf("%.4f", totalPrice))

	params.ReceivedQuantity = pgtype.Numeric{Valid: true}
	params.ReceivedQuantity.Scan(fmt.Sprintf("%.4f", req.ReceivedQuantity))

	item, err := po.h.Queries.CreatePurchaseOrderItem(context.Background(), params)
	if err != nil {
		po.h.Logger.Error("Failed to add purchase order item", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusCreated, item)
}

// DeletePurchaseOrderItem deletes a single item from a purchase order.
func (po *POSHandler) DeletePurchaseOrderItem(w http.ResponseWriter, r *http.Request) {
	poIDStr := r.PathValue("id")
	itemIDStr := r.PathValue("item_id")

	if poIDStr == "" || itemIDStr == "" {
		config.RespondBadRequest(w, "Missing purchase order ID or item ID", "")
		return
	}

	var poID, itemID int32
	if _, err := fmt.Sscanf(poIDStr, "%d", &poID); err != nil {
		config.RespondBadRequest(w, "Invalid purchase order ID format", err.Error())
		return
	}
	if _, err := fmt.Sscanf(itemIDStr, "%d", &itemID); err != nil {
		config.RespondBadRequest(w, "Invalid item ID format", err.Error())
		return
	}

	// Check if item exists and belongs to this PO
	currentItem, err := po.h.Queries.GetPurchaseOrderItemByID(context.Background(), itemID)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Item not found"})
		return
	}

	if currentItem.PurchaseOrderID.Int32 != poID {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Item does not belong to this purchase order"})
		return
	}

	// Check PO status - can only delete items when status is Pending
	purchaseOrder, err := po.h.Queries.GetPurchaseOrderByID(context.Background(), poID)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Purchase order not found"})
		return
	}

	if purchaseOrder.Status == "Cancelled" {
		config.RespondJSON(w, http.StatusForbidden, map[string]string{"error": "Cannot delete items from a cancelled purchase order"})
		return
	}

	if purchaseOrder.Status != "Pending" {
		config.RespondJSON(w, http.StatusForbidden, map[string]string{"error": "Can only delete items when purchase order status is Pending"})
		return
	}

	// Check if this is the last item - cannot delete if only 1 item remains
	items, err := po.h.Queries.ListPurchaseOrderItems(context.Background(), pgtype.Int4{Int32: poID, Valid: true})
	if err != nil {
		po.h.Logger.Error("Failed to count items", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if len(items) <= 1 {
		config.RespondJSON(w, http.StatusForbidden, map[string]string{"error": "Cannot delete the last item. Purchase order must have at least one item"})
		return
	}

	err = po.h.Queries.DeletePurchaseOrderItem(context.Background(), itemID)
	if err != nil {
		po.h.Logger.Error("Failed to delete purchase order item", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]string{"message": "Item deleted successfully"})
}
