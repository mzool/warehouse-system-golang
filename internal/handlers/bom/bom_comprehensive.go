package bom

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"warehouse_system/internal/config"
	db "warehouse_system/internal/database/db"
	"warehouse_system/internal/middlewares"
)

// CreateBOMRequestComprehensive includes all comprehensive fields
type CreateBOMRequestComprehensive struct {
	FinishedMaterialID   int32           `json:"finished_material_id"`
	ComponentMaterialID  int32           `json:"component_material_id"`
	Quantity             float64         `json:"quantity"`
	UnitMeasureID        *int32          `json:"unit_measure_id,omitempty"`
	Meta                 json.RawMessage `json:"meta,omitempty"`
	ScrapPercentage      *float64        `json:"scrap_percentage,omitempty"`
	FixedQuantity        *bool           `json:"fixed_quantity,omitempty"`
	IsOptional           *bool           `json:"is_optional,omitempty"`
	Priority             *int32          `json:"priority,omitempty"`
	ReferenceDesignator  *string         `json:"reference_designator,omitempty"`
	Notes                *string         `json:"notes,omitempty"`
	EffectiveDate        *string         `json:"effective_date,omitempty"`
	ExpiryDate           *string         `json:"expiry_date,omitempty"`
	Version              *string         `json:"version,omitempty"`
	OperationSequence    *int32          `json:"operation_sequence,omitempty"`
	EstimatedCost        *float64        `json:"estimated_cost,omitempty"`
	LeadTimeDays         *int32          `json:"lead_time_days,omitempty"`
	SupplierID           *int32          `json:"supplier_id,omitempty"`
	AlternateComponentID *int32          `json:"alternate_component_id,omitempty"`
	IsActive             *bool           `json:"is_active,omitempty"`
}

// UpdateBOMRequestComprehensive includes all comprehensive fields for update
type UpdateBOMRequestComprehensive struct {
	FinishedMaterialID   *int32          `json:"finished_material_id,omitempty"`
	ComponentMaterialID  *int32          `json:"component_material_id,omitempty"`
	Quantity             *float64        `json:"quantity,omitempty"`
	UnitMeasureID        *int32          `json:"unit_measure_id,omitempty"`
	Meta                 json.RawMessage `json:"meta,omitempty"`
	ScrapPercentage      *float64        `json:"scrap_percentage,omitempty"`
	FixedQuantity        *bool           `json:"fixed_quantity,omitempty"`
	IsOptional           *bool           `json:"is_optional,omitempty"`
	Priority             *int32          `json:"priority,omitempty"`
	ReferenceDesignator  *string         `json:"reference_designator,omitempty"`
	Notes                *string         `json:"notes,omitempty"`
	EffectiveDate        *string         `json:"effective_date,omitempty"`
	ExpiryDate           *string         `json:"expiry_date,omitempty"`
	Version              *string         `json:"version,omitempty"`
	OperationSequence    *int32          `json:"operation_sequence,omitempty"`
	EstimatedCost        *float64        `json:"estimated_cost,omitempty"`
	ActualCost           *float64        `json:"actual_cost,omitempty"`
	LeadTimeDays         *int32          `json:"lead_time_days,omitempty"`
	SupplierID           *int32          `json:"supplier_id,omitempty"`
	AlternateComponentID *int32          `json:"alternate_component_id,omitempty"`
	IsActive             *bool           `json:"is_active,omitempty"`
}

// CreateBillOfMaterialComprehensive creates a new comprehensive BOM entry
func (bh *BomHandler) CreateBillOfMaterialComprehensive(w http.ResponseWriter, r *http.Request) {
	var req CreateBOMRequestComprehensive
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

	// Validate scrap percentage
	if req.ScrapPercentage != nil && (*req.ScrapPercentage < 0 || *req.ScrapPercentage > 100) {
		config.RespondBadRequest(w, "Invalid scrap percentage", "Scrap percentage must be between 0 and 100")
		return
	}

	// Set default version if not provided
	version := "1.0"
	if req.Version != nil {
		version = *req.Version
	}

	// Check if the same BOM already exists
	exists, err := bh.h.Queries.CheckBOMExists(context.Background(), db.CheckBOMExistsParams{
		FinishedMaterialID:  pgtype.Int4{Int32: req.FinishedMaterialID, Valid: true},
		ComponentMaterialID: pgtype.Int4{Int32: req.ComponentMaterialID, Valid: true},
		Version:             pgtype.Text{String: version, Valid: true},
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

	// Set scrap percentage (default 0)
	scrapPerc := 0.0
	if req.ScrapPercentage != nil {
		scrapPerc = *req.ScrapPercentage
	}
	params.ScrapPercentage = pgtype.Numeric{Valid: true}
	params.ScrapPercentage.Scan(fmt.Sprintf("%.2f", scrapPerc))

	// Set boolean fields with defaults
	params.FixedQuantity = pgtype.Bool{Bool: false, Valid: true}
	if req.FixedQuantity != nil {
		params.FixedQuantity = pgtype.Bool{Bool: *req.FixedQuantity, Valid: true}
	}

	params.IsOptional = pgtype.Bool{Bool: false, Valid: true}
	if req.IsOptional != nil {
		params.IsOptional = pgtype.Bool{Bool: *req.IsOptional, Valid: true}
	}

	params.IsActive = pgtype.Bool{Bool: true, Valid: true}
	if req.IsActive != nil {
		params.IsActive = pgtype.Bool{Bool: *req.IsActive, Valid: true}
	}

	// Set priority (default 1)
	priority := int32(1)
	if req.Priority != nil {
		priority = *req.Priority
	}
	params.Priority = pgtype.Int4{Int32: priority, Valid: true}

	// Set version
	params.Version = pgtype.Text{String: version, Valid: true}

	// Set optional string fields
	if req.ReferenceDesignator != nil {
		params.ReferenceDesignator = pgtype.Text{String: *req.ReferenceDesignator, Valid: true}
	}
	if req.Notes != nil {
		params.Notes = pgtype.Text{String: *req.Notes, Valid: true}
	}

	// Set dates
	if req.EffectiveDate != nil {
		effectiveDate, err := time.Parse("2006-01-02", *req.EffectiveDate)
		if err != nil {
			config.RespondBadRequest(w, "Invalid effective_date format", "Use YYYY-MM-DD format")
			return
		}
		params.EffectiveDate = pgtype.Date{Time: effectiveDate, Valid: true}
	}

	if req.ExpiryDate != nil {
		expiryDate, err := time.Parse("2006-01-02", *req.ExpiryDate)
		if err != nil {
			config.RespondBadRequest(w, "Invalid expiry_date format", "Use YYYY-MM-DD format")
			return
		}
		params.ExpiryDate = pgtype.Date{Time: expiryDate, Valid: true}
	}

	// Set optional int fields
	if req.OperationSequence != nil {
		params.OperationSequence = pgtype.Int4{Int32: *req.OperationSequence, Valid: true}
	}

	leadTime := int32(0)
	if req.LeadTimeDays != nil {
		leadTime = *req.LeadTimeDays
	}
	params.LeadTimeDays = pgtype.Int4{Int32: leadTime, Valid: true}

	if req.SupplierID != nil {
		params.SupplierID = pgtype.Int4{Int32: *req.SupplierID, Valid: true}
	}

	if req.AlternateComponentID != nil {
		params.AlternateComponentID = pgtype.Int4{Int32: *req.AlternateComponentID, Valid: true}
	}

	// Set estimated cost
	if req.EstimatedCost != nil {
		params.EstimatedCost = pgtype.Numeric{Valid: true}
		params.EstimatedCost.Scan(fmt.Sprintf("%.2f", *req.EstimatedCost))
	}

	bom, err := bh.h.Queries.CreateBillOfMaterial(context.Background(), params)
	if err != nil {
		bh.h.Logger.Error("Failed to create BOM", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusCreated, bom)
}

// UpdateBillOfMaterialComprehensive updates an existing comprehensive BOM
func (bh *BomHandler) UpdateBillOfMaterialComprehensive(w http.ResponseWriter, r *http.Request) {
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

	var req UpdateBOMRequestComprehensive
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondBadRequest(w, "Invalid request payload", err.Error())
		return
	}

	// Validate quantity if provided
	if req.Quantity != nil && *req.Quantity <= 0 {
		config.RespondBadRequest(w, "Invalid quantity", "Quantity must be greater than 0")
		return
	}

	// Validate scrap percentage if provided
	if req.ScrapPercentage != nil && (*req.ScrapPercentage < 0 || *req.ScrapPercentage > 100) {
		config.RespondBadRequest(w, "Invalid scrap percentage", "Scrap percentage must be between 0 and 100")
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

	// Comprehensive fields
	if req.ScrapPercentage != nil {
		params.ScrapPercentage = pgtype.Numeric{Valid: true}
		params.ScrapPercentage.Scan(fmt.Sprintf("%.2f", *req.ScrapPercentage))
	}
	if req.FixedQuantity != nil {
		params.FixedQuantity = pgtype.Bool{Bool: *req.FixedQuantity, Valid: true}
	}
	if req.IsOptional != nil {
		params.IsOptional = pgtype.Bool{Bool: *req.IsOptional, Valid: true}
	}
	if req.Priority != nil {
		params.Priority = pgtype.Int4{Int32: *req.Priority, Valid: true}
	}
	if req.ReferenceDesignator != nil {
		params.ReferenceDesignator = pgtype.Text{String: *req.ReferenceDesignator, Valid: true}
	}
	if req.Notes != nil {
		params.Notes = pgtype.Text{String: *req.Notes, Valid: true}
	}

	if req.EffectiveDate != nil {
		effectiveDate, err := time.Parse("2006-01-02", *req.EffectiveDate)
		if err != nil {
			config.RespondBadRequest(w, "Invalid effective_date format", "Use YYYY-MM-DD format")
			return
		}
		params.EffectiveDate = pgtype.Date{Time: effectiveDate, Valid: true}
	}

	if req.ExpiryDate != nil {
		expiryDate, err := time.Parse("2006-01-02", *req.ExpiryDate)
		if err != nil {
			config.RespondBadRequest(w, "Invalid expiry_date format", "Use YYYY-MM-DD format")
			return
		}
		params.ExpiryDate = pgtype.Date{Time: expiryDate, Valid: true}
	}

	if req.Version != nil {
		params.Version = pgtype.Text{String: *req.Version, Valid: true}
	}
	if req.OperationSequence != nil {
		params.OperationSequence = pgtype.Int4{Int32: *req.OperationSequence, Valid: true}
	}
	if req.EstimatedCost != nil {
		params.EstimatedCost = pgtype.Numeric{Valid: true}
		params.EstimatedCost.Scan(fmt.Sprintf("%.2f", *req.EstimatedCost))
	}
	if req.ActualCost != nil {
		params.ActualCost = pgtype.Numeric{Valid: true}
		params.ActualCost.Scan(fmt.Sprintf("%.2f", *req.ActualCost))
	}
	if req.LeadTimeDays != nil {
		params.LeadTimeDays = pgtype.Int4{Int32: *req.LeadTimeDays, Valid: true}
	}
	if req.SupplierID != nil {
		params.SupplierID = pgtype.Int4{Int32: *req.SupplierID, Valid: true}
	}
	if req.AlternateComponentID != nil {
		params.AlternateComponentID = pgtype.Int4{Int32: *req.AlternateComponentID, Valid: true}
	}
	if req.IsActive != nil {
		params.IsActive = pgtype.Bool{Bool: *req.IsActive, Valid: true}
	}

	bom, err := bh.h.Queries.UpdateBillOfMaterial(context.Background(), params)
	if err != nil {
		bh.h.Logger.Error("Failed to update BOM", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, bom)
}

// GetActiveBOMs retrieves only active, effective BOMs for a finished material
func (bh *BomHandler) GetActiveBOMs(w http.ResponseWriter, r *http.Request) {
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

	boms, err := bh.h.Queries.GetActiveBOMsByFinishedMaterial(context.Background(), pgtype.Int4{Int32: id, Valid: true})
	if err != nil {
		bh.h.Logger.Error("Failed to get active BOMs", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]any{
		"finished_material_id": id,
		"components":           boms,
	})
}

// GetBOMCostBreakdown gets detailed cost breakdown for a finished material
func (bh *BomHandler) GetBOMCostBreakdown(w http.ResponseWriter, r *http.Request) {
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

	breakdown, err := bh.h.Queries.GetBOMCostBreakdown(context.Background(), pgtype.Int4{Int32: id, Valid: true})
	if err != nil {
		bh.h.Logger.Error("Failed to get BOM cost breakdown", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Calculate totals
	var totalCost float64
	for _, item := range breakdown {
		if item.TotalCost.Valid {
			var c float64
			item.TotalCost.Scan(&c)
			totalCost += c
		}
	}

	config.RespondJSON(w, http.StatusOK, map[string]any{
		"finished_material_id": id,
		"breakdown":            breakdown,
		"total_cost":           totalCost,
	})
}

// GetBOMVersions lists all versions for a finished material
func (bh *BomHandler) GetBOMVersions(w http.ResponseWriter, r *http.Request) {
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

	versions, err := bh.h.Queries.GetBOMVersions(context.Background(), pgtype.Int4{Int32: id, Valid: true})
	if err != nil {
		bh.h.Logger.Error("Failed to get BOM versions", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]any{
		"finished_material_id": id,
		"versions":             versions,
	})
}

// ArchiveBOM archives a BOM entry
func (bh *BomHandler) ArchiveBOM(w http.ResponseWriter, r *http.Request) {
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

	// Get user ID from session
	session, ok := middlewares.GetSessionFromContext(r)
	if !ok || session == nil {
		config.RespondJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	// Convert string UserID to int32
	var userID int32
	if _, err := fmt.Sscanf(session.UserID, "%d", &userID); err != nil {
		bh.h.Logger.Error("Invalid user ID format", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Invalid user ID"})
		return
	}

	bom, err := bh.h.Queries.ArchiveBOM(context.Background(), db.ArchiveBOMParams{
		ID:         id,
		ArchivedBy: pgtype.Int4{Int32: userID, Valid: true},
	})
	if err != nil {
		bh.h.Logger.Error("Failed to archive BOM", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, bom)
}

// GetOptionalComponents retrieves all optional components for a finished material
func (bh *BomHandler) GetOptionalComponents(w http.ResponseWriter, r *http.Request) {
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

	components, err := bh.h.Queries.GetOptionalComponents(context.Background(), pgtype.Int4{Int32: id, Valid: true})
	if err != nil {
		bh.h.Logger.Error("Failed to get optional components", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]any{
		"finished_material_id": id,
		"optional_components":  components,
	})
}
