package transactions

import (
	"net/http"
	"strconv"

	"warehouse_system/internal/config"
	db "warehouse_system/internal/database/db"

	"github.com/jackc/pgx/v5/pgtype"
)

// =====================================================
// QUERY ENDPOINTS
// =====================================================

// GetStockLevel - Get current stock level for a material in a warehouse
func (th *TransactionHandler) GetStockLevel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	materialIDStr := r.URL.Query().Get("material_id")
	warehouseIDStr := r.URL.Query().Get("warehouse_id")

	if materialIDStr == "" || warehouseIDStr == "" {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "material_id and warehouse_id are required"})
		return
	}

	materialID, err := strconv.Atoi(materialIDStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid material_id"})
		return
	}

	warehouseID, err := strconv.Atoi(warehouseIDStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid warehouse_id"})
		return
	}

	stockLevel, err := th.h.Queries.GetCurrentStockLevel(ctx, db.GetCurrentStockLevelParams{
		MaterialID:  pgtype.Int4{Int32: int32(materialID), Valid: true},
		WarehouseID: pgtype.Int4{Int32: int32(warehouseID), Valid: true},
	})
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to get stock level"})
		return
	}

	config.RespondJSON(w, http.StatusOK, stockLevel)
}

// GetStockLevelsByWarehouse - Get all stock levels in a warehouse
func (th *TransactionHandler) GetStockLevelsByWarehouse(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	stockLevels, err := th.h.Queries.GetStockLevelsByWarehouse(ctx)
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to get stock levels"})
		return
	}

	config.RespondJSON(w, http.StatusOK, stockLevels)
}

// GetStockLevelsByMaterial - Get stock levels for a material across all warehouses
func (th *TransactionHandler) GetStockLevelsByMaterial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	materialIDStr := r.URL.Query().Get("material_id")
	if materialIDStr == "" {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "material_id is required"})
		return
	}

	materialID, err := strconv.Atoi(materialIDStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid material_id"})
		return
	}

	stockLevels, err := th.h.Queries.GetStockLevelsByMaterial(ctx, int32(materialID))
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to get stock levels"})
		return
	}

	config.RespondJSON(w, http.StatusOK, stockLevels)
}

// GetAvailableBatches - Get available batches for a material in a warehouse
func (th *TransactionHandler) GetAvailableBatches(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	materialIDStr := r.URL.Query().Get("material_id")
	warehouseIDStr := r.URL.Query().Get("warehouse_id")

	if materialIDStr == "" || warehouseIDStr == "" {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "material_id and warehouse_id are required"})
		return
	}

	materialID, err := strconv.Atoi(materialIDStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid material_id"})
		return
	}

	warehouseID, err := strconv.Atoi(warehouseIDStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid warehouse_id"})
		return
	}

	batches, err := th.h.Queries.GetAvailableBatchesForMaterial(ctx, db.GetAvailableBatchesForMaterialParams{
		MaterialID:  pgtype.Int4{Int32: int32(materialID), Valid: true},
		WarehouseID: pgtype.Int4{Int32: int32(warehouseID), Valid: true},
	})
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to get available batches"})
		return
	}

	config.RespondJSON(w, http.StatusOK, batches)
}

// GetMovementHistory - Get movement history for a material
func (th *TransactionHandler) GetMovementHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	materialIDStr := r.URL.Query().Get("material_id")
	if materialIDStr == "" {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "material_id is required"})
		return
	}

	materialID, err := strconv.Atoi(materialIDStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid material_id"})
		return
	}

	// Get pagination params
	limit := 50
	offset := 0

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	movements, err := th.h.Queries.GetStockMovementHistory(ctx, db.GetStockMovementHistoryParams{
		MaterialID: pgtype.Int4{Int32: int32(materialID), Valid: true},
		Limit:      int32(limit),
		Offset:     int32(offset),
	})
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to get movement history"})
		return
	}

	config.RespondJSON(w, http.StatusOK, movements)
}

// GetWarehouseMovements - Get all movements for a warehouse
func (th *TransactionHandler) GetWarehouseMovements(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	warehouseIDStr := r.URL.Query().Get("warehouse_id")
	if warehouseIDStr == "" {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "warehouse_id is required"})
		return
	}

	warehouseID, err := strconv.Atoi(warehouseIDStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid warehouse_id"})
		return
	}

	// Get pagination params
	limit := 50
	offset := 0

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	movements, err := th.h.Queries.GetWarehouseStockMovements(ctx, db.GetWarehouseStockMovementsParams{
		FromWarehouseID: pgtype.Int4{Int32: int32(warehouseID), Valid: true},
		Limit:           int32(limit),
		Offset:          int32(offset),
	})
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to get warehouse movements"})
		return
	}

	config.RespondJSON(w, http.StatusOK, movements)
}

// GetMovementByID - Get details of a specific stock movement
func (th *TransactionHandler) GetMovementByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	movementIDStr := r.URL.Query().Get("id")
	if movementIDStr == "" {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}

	movementID, err := strconv.Atoi(movementIDStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid movement id"})
		return
	}

	movement, err := th.h.Queries.GetStockMovementByID(ctx, int32(movementID))
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Movement not found"})
		return
	}

	config.RespondJSON(w, http.StatusOK, movement)
}

// =====================================================
// COMPREHENSIVE STOCK ENDPOINT
// =====================================================

type BatchDetail struct {
	ID              int32   `json:"id"`
	BatchNumber     string  `json:"batch_number"`
	CurrentQuantity float64 `json:"current_quantity"`
	UnitPrice       float64 `json:"unit_price"`
	ManufactureDate *string `json:"manufacture_date,omitempty"`
	ExpiryDate      *string `json:"expiry_date,omitempty"`
	WarehouseName   string  `json:"warehouse_name,omitempty"`
	SupplierName    string  `json:"supplier_name,omitempty"`
}

type WarehouseStock struct {
	WarehouseID   int32         `json:"warehouse_id"`
	WarehouseName string        `json:"warehouse_name"`
	TotalQuantity float64       `json:"total_quantity"`
	BatchCount    int           `json:"batch_count"`
	Batches       []BatchDetail `json:"batches"`
}

type MaterialStockResponse struct {
	MaterialID   int32            `json:"material_id"`
	MaterialName string           `json:"material_name"`
	MaterialCode string           `json:"material_code"`
	TotalStock   float64          `json:"total_stock"`
	TotalBatches int              `json:"total_batches"`
	Warehouses   []WarehouseStock `json:"warehouses"`
}

// Helper function to convert pgtype.Numeric to float64
func numericToFloat(n interface{}) float64 {
	if n == nil {
		return 0
	}

	switch v := n.(type) {
	case pgtype.Numeric:
		if !v.Valid || v.Int == nil {
			return 0
		}
		// Convert using the exponent: value = Int * 10^Exp
		value := float64(v.Int.Int64())
		if v.Exp != 0 {
			// Apply the exponent (negative exp means divide)
			for i := int32(0); i < -v.Exp; i++ {
				value /= 10
			}
			for i := int32(0); i < v.Exp; i++ {
				value *= 10
			}
		}
		return value
	case float64:
		return v
	case int64:
		return float64(v)
	default:
		return 0
	}
}

// GetMaterialStock - Get comprehensive stock info for a material with all batches
func (th *TransactionHandler) GetMaterialStock(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	materialIDStr := r.URL.Query().Get("material_id")
	if materialIDStr == "" {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "material_id is required"})
		return
	}

	materialID, err := strconv.Atoi(materialIDStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid material_id"})
		return
	}

	// Get stock levels by material (aggregated by warehouse)
	stockLevels, err := th.h.Queries.GetStockLevelsByMaterial(ctx, int32(materialID))
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to get stock levels"})
		return
	}

	if len(stockLevels) == 0 {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Material not found or has no stock"})
		return
	}

	// Build response
	response := MaterialStockResponse{
		MaterialID:   stockLevels[0].MaterialID,
		MaterialName: stockLevels[0].MaterialName,
		MaterialCode: stockLevels[0].MaterialCode,
		TotalStock:   0,
		TotalBatches: 0,
		Warehouses:   []WarehouseStock{},
	}

	// Process each warehouse
	for _, level := range stockLevels {
		if !level.WarehouseID.Valid {
			continue
		}

		warehouseID := level.WarehouseID.Int32
		warehouseName := ""
		if level.WarehouseName.Valid {
			warehouseName = level.WarehouseName.String
		}

		quantity := numericToFloat(level.TotalQuantity)

		// Get batches for this warehouse
		batches, err := th.h.Queries.GetAvailableBatchesForMaterial(ctx, db.GetAvailableBatchesForMaterialParams{
			MaterialID:  pgtype.Int4{Int32: int32(materialID), Valid: true},
			WarehouseID: pgtype.Int4{Int32: warehouseID, Valid: true},
		})
		if err != nil {
			// Skip this warehouse if we can't get batches
			continue
		}

		// Convert batches to response format
		batchDetails := []BatchDetail{}
		for _, b := range batches {
			detail := BatchDetail{
				ID:              b.ID,
				BatchNumber:     b.BatchNumber,
				CurrentQuantity: numericToFloat(b.CurrentQuantity),
				UnitPrice:       numericToFloat(b.UnitPrice),
			}

			if b.ManufactureDate.Valid {
				dateStr := b.ManufactureDate.Time.Format("2006-01-02")
				detail.ManufactureDate = &dateStr
			}

			if b.ExpiryDate.Valid {
				dateStr := b.ExpiryDate.Time.Format("2006-01-02")
				detail.ExpiryDate = &dateStr
			}

			if b.WarehouseName.Valid {
				detail.WarehouseName = b.WarehouseName.String
			}

			if b.SupplierName.Valid {
				detail.SupplierName = b.SupplierName.String
			}

			batchDetails = append(batchDetails, detail)
		}

		warehouseStock := WarehouseStock{
			WarehouseID:   warehouseID,
			WarehouseName: warehouseName,
			TotalQuantity: quantity,
			BatchCount:    len(batchDetails),
			Batches:       batchDetails,
		}

		response.Warehouses = append(response.Warehouses, warehouseStock)
		response.TotalStock += quantity
		response.TotalBatches += len(batchDetails)
	}

	config.RespondJSON(w, http.StatusOK, response)
}
