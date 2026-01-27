package bom

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

// ============================================================================
// DEPRECATED: These handlers are kept for backward compatibility only.
// All routes now use the comprehensive handlers from bom_comprehensive.go
// which support both basic and advanced fields with defaults.
//
// These methods are no longer registered in routes.go and will be removed
// in a future version.
// ============================================================================

type BomHandler struct {
	h *handlers.Handler
}

func NewBomHandler(h *handlers.Handler) *BomHandler {
	return &BomHandler{h: h}
}

type CreateBOMRequest struct {
	FinishedMaterialID  int32           `json:"finished_material_id"`
	ComponentMaterialID int32           `json:"component_material_id"`
	Quantity            float64         `json:"quantity"`
	UnitMeasureID       *int32          `json:"unit_measure_id,omitempty"`
	Meta                json.RawMessage `json:"meta,omitempty"`
}

type UpdateBOMRequest struct {
	FinishedMaterialID  *int32          `json:"finished_material_id,omitempty"`
	ComponentMaterialID *int32          `json:"component_material_id,omitempty"`
	Quantity            *float64        `json:"quantity,omitempty"`
	UnitMeasureID       *int32          `json:"unit_measure_id,omitempty"`
	Meta                json.RawMessage `json:"meta,omitempty"`
}

// DEPRECATED: Use CreateBillOfMaterialComprehensive instead
// CreateBillOfMaterial creates a new BOM entry.
func (bh *BomHandler) CreateBillOfMaterial(w http.ResponseWriter, r *http.Request) {
	var req CreateBOMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondBadRequest(w, "Invalid request payload", err.Error())
		return
	}

	// Validate required fields
	if req.FinishedMaterialID == 0 || req.ComponentMaterialID == 0 {
		config.RespondBadRequest(w, "Missing required fields", "Finished material ID and component material ID are required")
		return
	}

	if req.Quantity <= 0 {
		config.RespondBadRequest(w, "Invalid quantity", "Quantity must be greater than 0")
		return
	}

	// Check if the same BOM already exists
	exists, err := bh.h.Queries.CheckBOMExists(context.Background(), db.CheckBOMExistsParams{
		FinishedMaterialID:  pgtype.Int4{Int32: req.FinishedMaterialID, Valid: true},
		ComponentMaterialID: pgtype.Int4{Int32: req.ComponentMaterialID, Valid: true},
	})
	if err != nil {
		bh.h.Logger.Error("Failed to check BOM existence", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if exists {
		config.RespondJSON(w, http.StatusConflict, map[string]string{"error": "BOM entry already exists for this combination"})
		return
	}

	params := db.CreateBillOfMaterialParams{
		FinishedMaterialID:  pgtype.Int4{Int32: req.FinishedMaterialID, Valid: true},
		ComponentMaterialID: pgtype.Int4{Int32: req.ComponentMaterialID, Valid: true},
		Meta:                req.Meta,
	}

	params.Quantity = pgtype.Numeric{Valid: true}
	params.Quantity.Scan(fmt.Sprintf("%.4f", req.Quantity))

	if req.UnitMeasureID != nil {
		params.UnitMeasureID = pgtype.Int4{Int32: *req.UnitMeasureID, Valid: true}
	}

	bom, err := bh.h.Queries.CreateBillOfMaterial(context.Background(), params)
	if err != nil {
		bh.h.Logger.Error("Failed to create BOM", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusCreated, bom)
}

// DEPRECATED: This method is kept for backward compatibility
// GetBillOfMaterial retrieves a BOM by ID.
func (bh *BomHandler) GetBillOfMaterial(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing BOM ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid BOM ID format", err.Error())
		return
	}

	bom, err := bh.h.Queries.GetBillOfMaterialByID(context.Background(), id)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "BOM not found"})
		return
	}

	config.RespondJSON(w, http.StatusOK, bom)
}

// DEPRECATED: Use UpdateBillOfMaterialComprehensive instead
// UpdateBillOfMaterial updates an existing BOM.
func (bh *BomHandler) UpdateBillOfMaterial(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing BOM ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid BOM ID format", err.Error())
		return
	}

	var req UpdateBOMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondBadRequest(w, "Invalid request payload", err.Error())
		return
	}

	// Validate quantity if provided
	if req.Quantity != nil && *req.Quantity <= 0 {
		config.RespondBadRequest(w, "Invalid quantity", "Quantity must be greater than 0")
		return
	}

	// Check if BOM exists
	_, err := bh.h.Queries.GetBillOfMaterialByID(context.Background(), id)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "BOM not found"})
		return
	}

	params := db.UpdateBillOfMaterialParams{
		ID: id,
	}

	if req.FinishedMaterialID != nil {
		params.FinishedMaterialID = pgtype.Int4{Int32: *req.FinishedMaterialID, Valid: true}
	}
	if req.ComponentMaterialID != nil {
		params.ComponentMaterialID = pgtype.Int4{Int32: *req.ComponentMaterialID, Valid: true}
	}
	if req.Quantity != nil {
		params.Quantity = pgtype.Numeric{Valid: true}
		params.Quantity.Scan(fmt.Sprintf("%.4f", *req.Quantity))
	}
	if req.UnitMeasureID != nil {
		params.UnitMeasureID = pgtype.Int4{Int32: *req.UnitMeasureID, Valid: true}
	}
	if req.Meta != nil {
		params.Meta = req.Meta
	}

	bom, err := bh.h.Queries.UpdateBillOfMaterial(context.Background(), params)
	if err != nil {
		bh.h.Logger.Error("Failed to update BOM", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, bom)
}

// DEPRECATED: This method is kept for backward compatibility
// DeleteBillOfMaterial deletes a BOM.
func (bh *BomHandler) DeleteBillOfMaterial(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing BOM ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid BOM ID format", err.Error())
		return
	}

	// Check if BOM exists
	_, err := bh.h.Queries.GetBillOfMaterialByID(context.Background(), id)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "BOM not found"})
		return
	}

	err = bh.h.Queries.DeleteBillOfMaterial(context.Background(), id)
	if err != nil {
		bh.h.Logger.Error("Failed to delete BOM", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]string{"message": "BOM deleted successfully"})
}

// DEPRECATED: This method is kept for backward compatibility
// ListBillsOfMaterials returns a paginated list of BOMs with optional search.
func (bh *BomHandler) ListBillsOfMaterials(w http.ResponseWriter, r *http.Request) {
	pagination := middlewares.GetPagination(r.Context())

	query := r.URL.Query().Get("q")

	var boms []db.SearchBillsOfMaterialsRow
	var total int64
	var err error

	if query != "" {
		boms, err = bh.h.Queries.SearchBillsOfMaterials(context.Background(), db.SearchBillsOfMaterialsParams{
			Query:  pgtype.Text{String: query, Valid: true},
			Limit:  int32(pagination.Limit),
			Offset: int32(pagination.Offset),
		})
		if err != nil {
			bh.h.Logger.Error("Failed to search BOMs", "error", err)
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		count, err := bh.h.Queries.CountSearchBillsOfMaterials(context.Background(), pgtype.Text{String: query, Valid: true})
		if err != nil {
			bh.h.Logger.Error("Failed to count search BOMs", "error", err)
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		total = count
	} else {
		listBoms, err := bh.h.Queries.ListBillsOfMaterials(context.Background(), db.ListBillsOfMaterialsParams{
			Limit:  int32(pagination.Limit),
			Offset: int32(pagination.Offset),
		})
		if err != nil {
			bh.h.Logger.Error("Failed to list BOMs", "error", err)
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		// Convert to SearchBillsOfMaterialsRow for consistent response
		boms = make([]db.SearchBillsOfMaterialsRow, len(listBoms))
		for i, b := range listBoms {
			boms[i] = db.SearchBillsOfMaterialsRow{
				ID:                    b.ID,
				FinishedMaterialID:    b.FinishedMaterialID,
				ComponentMaterialID:   b.ComponentMaterialID,
				Quantity:              b.Quantity,
				UnitMeasureID:         b.UnitMeasureID,
				Meta:                  b.Meta,
				CreatedAt:             b.CreatedAt,
				UpdatedAt:             b.UpdatedAt,
				FinishedMaterialName:  b.FinishedMaterialName,
				FinishedMaterialCode:  b.FinishedMaterialCode,
				ComponentMaterialName: b.ComponentMaterialName,
				ComponentMaterialCode: b.ComponentMaterialCode,
				UnitName:              b.UnitName,
				UnitAbbreviation:      b.UnitAbbreviation,
			}
		}

		count, err := bh.h.Queries.CountBillsOfMaterials(context.Background())
		if err != nil {
			bh.h.Logger.Error("Failed to count BOMs", "error", err)
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		total = count
	}

	pagination.Total = total
	totalPages := (total + int64(pagination.Limit) - 1) / int64(pagination.Limit)

	config.RespondJSON(w, http.StatusOK, map[string]any{
		"boms": boms,
		"pagination": map[string]any{
			"page":        pagination.Page,
			"limit":       pagination.Limit,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

// DEPRECATED: Consider using GetActiveBOMs for production planning
// GetBillOfMaterialsByFinishedMaterial retrieves all components for a finished material.
func (bh *BomHandler) GetBillOfMaterialsByFinishedMaterial(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing finished material ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid finished material ID format", err.Error())
		return
	}

	boms, err := bh.h.Queries.GetBillOfMaterialsByFinishedMaterial(context.Background(), pgtype.Int4{Int32: id, Valid: true})
	if err != nil {
		bh.h.Logger.Error("Failed to get BOMs by finished material", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]any{
		"finished_material_id": id,
		"components":           boms,
	})
}

// DEPRECATED: Consider using GetBOMCostBreakdown for detailed cost analysis
// GetBOMTotalCost calculates the total cost of all components for a finished material.
func (bh *BomHandler) GetBOMTotalCost(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing finished material ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid finished material ID format", err.Error())
		return
	}

	totalCost, err := bh.h.Queries.GetBOMTotalCost(context.Background(), pgtype.Int4{Int32: id, Valid: true})
	if err != nil {
		bh.h.Logger.Error("Failed to calculate BOM total cost", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	var cost float64
	if totalCost != nil {
		if numericCost, ok := totalCost.(pgtype.Numeric); ok && numericCost.Valid {
			numericCost.Scan(&cost)
		}
	}

	config.RespondJSON(w, http.StatusOK, map[string]any{
		"finished_material_id": id,
		"total_cost":           cost,
	})
}
