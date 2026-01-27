package warehouses

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"warehouse_system/internal/config"
	db "warehouse_system/internal/database/db"
	"warehouse_system/internal/handlers"
	"warehouse_system/internal/middlewares"
)

type WarehouseHandler struct {
	h *handlers.Handler
}

func NewWarehouseHandler(h *handlers.Handler) *WarehouseHandler {
	return &WarehouseHandler{h: h}
}

func isValidValuationMethod(v string) bool {
	validMethods := []string{"FIFO", "LIFO", "Weighted Average"}
	for _, valid := range validMethods {
		if v == valid {
			return true
		}
	}
	return false
}

type CreateWarehouseRequest struct {
	Name            string          `json:"name"`
	Code            string          `json:"code"`
	Location        string          `json:"location"`
	Description     string          `json:"description"`
	Valuation       string          `json:"valuation"`
	ParentWarehouse *int32          `json:"parent_warehouse"`
	Capacity        float64         `json:"capacity"`
	Meta            json.RawMessage `json:"meta"`
}

type UpdateWarehouseRequest struct {
	Name            *string         `json:"name,omitempty"`
	Code            *string         `json:"code,omitempty"`
	Location        *string         `json:"location,omitempty"`
	Description     *string         `json:"description,omitempty"`
	Valuation       *string         `json:"valuation,omitempty"`
	ParentWarehouse *int32          `json:"parent_warehouse,omitempty"`
	Capacity        *float64        `json:"capacity,omitempty"`
	Meta            json.RawMessage `json:"meta,omitempty"`
}

// CreateWarehouse creates a new warehouse.
func (wh *WarehouseHandler) CreateWarehouse(w http.ResponseWriter, r *http.Request) {
	var req CreateWarehouseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondBadRequest(w, "Invalid request payload", err.Error())
		return
	}

	if req.Name == "" || req.Code == "" {
		config.RespondBadRequest(w, "Missing required fields", "Name and Code are required")
		return
	}

	if req.Valuation != "" && !isValidValuationMethod(req.Valuation) {
		config.RespondBadRequest(w, "Invalid valuation method", "Valuation must be one of: FIFO, LIFO, Weighted Average")
		return
	}

	// Check for duplicate code
	_, err := wh.h.Queries.GetWarehouseByCode(context.Background(), req.Code)
	if err == nil {
		config.RespondJSON(w, http.StatusConflict, map[string]string{"error": "Warehouse code already exists"})
		return
	}

	// Check for duplicate name
	_, err = wh.h.Queries.GetWarehouseByName(context.Background(), req.Name)
	if err == nil {
		config.RespondJSON(w, http.StatusConflict, map[string]string{"error": "Warehouse name already exists"})
		return
	}

	params := db.CreateWarehouseParams{
		Name: req.Name,
		Code: req.Code,
	}

	if req.Location != "" {
		params.Location = pgtype.Text{String: req.Location, Valid: true}
	}
	if req.Description != "" {
		params.Description = pgtype.Text{String: req.Description, Valid: true}
	}
	if req.Valuation != "" {
		params.Valuation = db.ValuationMethod(req.Valuation)
	}
	if req.ParentWarehouse != nil {
		params.ParentWarehouse = pgtype.Int4{Int32: *req.ParentWarehouse, Valid: true}
	}
	if req.Capacity > 0 {
		params.Capacity = pgtype.Numeric{Int: nil, Valid: true}
		params.Capacity.Scan(fmt.Sprintf("%.4f", req.Capacity))
	}
	if req.Meta != nil {
		params.Meta = req.Meta
	}

	warehouse, err := wh.h.Queries.CreateWarehouse(context.Background(), params)
	if err != nil {
		wh.h.Logger.Error("Failed to create warehouse", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusCreated, warehouse)
}

// GetWarehouse retrieves a warehouse by ID.
func (wh *WarehouseHandler) GetWarehouse(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing warehouse ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid warehouse ID format", err.Error())
		return
	}

	warehouse, err := wh.h.Queries.GetWarehouseByID(context.Background(), id)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Warehouse not found"})
		return
	}

	config.RespondJSON(w, http.StatusOK, warehouse)
}

// UpdateWarehouse updates an existing warehouse.
func (wh *WarehouseHandler) UpdateWarehouse(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing warehouse ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid warehouse ID format", err.Error())
		return
	}

	var req UpdateWarehouseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondBadRequest(w, "Invalid request payload", err.Error())
		return
	}

	// Validate valuation if provided
	if req.Valuation != nil && *req.Valuation != "" && !isValidValuationMethod(*req.Valuation) {
		config.RespondBadRequest(w, "Invalid valuation method", "Valuation must be one of: FIFO, LIFO, Weighted Average")
		return
	}

	// Get current warehouse
	current, err := wh.h.Queries.GetWarehouseByID(context.Background(), id)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Warehouse not found"})
		return
	}

	// Check for duplicate code if being updated
	if req.Code != nil && *req.Code != current.Code {
		_, err := wh.h.Queries.GetWarehouseByCode(context.Background(), *req.Code)
		if err == nil {
			config.RespondJSON(w, http.StatusConflict, map[string]string{"error": "Warehouse code already exists"})
			return
		}
	}

	// Check for duplicate name if being updated
	if req.Name != nil && *req.Name != current.Name {
		_, err := wh.h.Queries.GetWarehouseByName(context.Background(), *req.Name)
		if err == nil {
			config.RespondJSON(w, http.StatusConflict, map[string]string{"error": "Warehouse name already exists"})
			return
		}
	}

	params := db.UpdateWarehouseParams{
		ID: id,
	}

	// Handle name and code (interface{} due to NULLIF)
	if req.Name != nil {
		params.Column2 = *req.Name
	} else {
		params.Column2 = ""
	}

	if req.Code != nil {
		params.Column3 = *req.Code
	} else {
		params.Column3 = ""
	}

	// Handle optional fields
	if req.Location != nil {
		params.Location = pgtype.Text{String: *req.Location, Valid: true}
	}
	if req.Description != nil {
		params.Description = pgtype.Text{String: *req.Description, Valid: true}
	}
	if req.Valuation != nil {
		params.Valuation = db.ValuationMethod(*req.Valuation)
	}
	if req.ParentWarehouse != nil {
		params.ParentWarehouse = pgtype.Int4{Int32: *req.ParentWarehouse, Valid: true}
	}
	if req.Capacity != nil {
		params.Capacity = pgtype.Numeric{Int: nil, Valid: true}
		params.Capacity.Scan(fmt.Sprintf("%.4f", *req.Capacity))
	}
	if req.Meta != nil {
		params.Meta = req.Meta
	}

	warehouse, err := wh.h.Queries.UpdateWarehouse(context.Background(), params)
	if err != nil {
		wh.h.Logger.Error("Failed to update warehouse", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, warehouse)
}

// DeleteWarehouse deletes a warehouse by ID.
func (wh *WarehouseHandler) DeleteWarehouse(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing warehouse ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid warehouse ID format", err.Error())
		return
	}

	if err := wh.h.Queries.DeleteWarehouse(context.Background(), id); err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]string{"message": "Warehouse deleted successfully"})
}

// ListWarehouses lists all warehouses with pagination.
func (wh *WarehouseHandler) ListWarehouses(w http.ResponseWriter, r *http.Request) {
	pagination := middlewares.GetPagination(r.Context())
	limit, offset := pagination.GetSQLLimitOffset()

	params := db.ListWarehousesParams{
		Limit:  limit,
		Offset: offset,
	}

	warehouses, err := wh.h.Queries.ListWarehouses(context.Background(), params)
	if err != nil {
		wh.h.Logger.Error("Failed to list warehouses", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"warehouses": warehouses,
		"pagination": pagination.BuildMeta(),
	})
}
