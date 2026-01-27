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
)

// BulkCreateBOMRequest represents a request to create multiple BOM entries at once
type BulkCreateBOMRequest struct {
	FinishedMaterialID int32                 `json:"finished_material_id"`
	Components         []BOMComponentRequest `json:"components"`
	CommonFields       *BOMCommonFields      `json:"common_fields,omitempty"` // Optional: applied to all components
}

// BOMComponentRequest represents a single component in bulk create
type BOMComponentRequest struct {
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

// BOMCommonFields represents fields that can be applied to all components
type BOMCommonFields struct {
	Version         *string  `json:"version,omitempty"`
	EffectiveDate   *string  `json:"effective_date,omitempty"`
	ExpiryDate      *string  `json:"expiry_date,omitempty"`
	IsActive        *bool    `json:"is_active,omitempty"`
	ScrapPercentage *float64 `json:"scrap_percentage,omitempty"`
}

// BulkCreateBOMResponse represents the response for bulk creation
type BulkCreateBOMResponse struct {
	FinishedMaterialID int32                        `json:"finished_material_id"`
	Created            []db.CreateBillOfMaterialRow `json:"created"`
	Failed             []BOMFailedEntry             `json:"failed,omitempty"`
	Summary            BOMBulkSummary               `json:"summary"`
}

type BOMFailedEntry struct {
	ComponentMaterialID int32  `json:"component_material_id"`
	Reason              string `json:"reason"`
}

type BOMBulkSummary struct {
	TotalRequested int `json:"total_requested"`
	SuccessCount   int `json:"success_count"`
	FailedCount    int `json:"failed_count"`
}

// BulkCreateBillOfMaterials creates multiple BOM entries at once
func (bh *BomHandler) BulkCreateBillOfMaterials(w http.ResponseWriter, r *http.Request) {
	var req BulkCreateBOMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondBadRequest(w, "Invalid request payload", err.Error())
		return
	}

	// Validate
	if req.FinishedMaterialID == 0 {
		config.RespondBadRequest(w, "Missing required fields", "Finished material ID is required")
		return
	}

	if len(req.Components) == 0 {
		config.RespondBadRequest(w, "No components provided", "At least one component is required")
		return
	}

	ctx := context.Background()
	var created []db.CreateBillOfMaterialRow
	var failed []BOMFailedEntry

	// Process each component
	for _, comp := range req.Components {
		// Validate component
		if comp.ComponentMaterialID == 0 {
			failed = append(failed, BOMFailedEntry{
				ComponentMaterialID: comp.ComponentMaterialID,
				Reason:              "Component material ID is required",
			})
			continue
		}

		if comp.Quantity <= 0 {
			failed = append(failed, BOMFailedEntry{
				ComponentMaterialID: comp.ComponentMaterialID,
				Reason:              "Quantity must be greater than 0",
			})
			continue
		}

		// Apply common fields if provided
		version := comp.Version
		effectiveDate := comp.EffectiveDate
		expiryDate := comp.ExpiryDate
		isActive := comp.IsActive
		scrapPercentage := comp.ScrapPercentage

		if req.CommonFields != nil {
			if version == nil && req.CommonFields.Version != nil {
				version = req.CommonFields.Version
			}
			if effectiveDate == nil && req.CommonFields.EffectiveDate != nil {
				effectiveDate = req.CommonFields.EffectiveDate
			}
			if expiryDate == nil && req.CommonFields.ExpiryDate != nil {
				expiryDate = req.CommonFields.ExpiryDate
			}
			if isActive == nil && req.CommonFields.IsActive != nil {
				isActive = req.CommonFields.IsActive
			}
			if scrapPercentage == nil && req.CommonFields.ScrapPercentage != nil {
				scrapPercentage = req.CommonFields.ScrapPercentage
			}
		}

		// Set defaults
		defaultVersion := "1.0"
		if version == nil {
			version = &defaultVersion
		}

		defaultPriority := int32(1)
		if comp.Priority == nil {
			comp.Priority = &defaultPriority
		}

		defaultScrap := 0.0
		if scrapPercentage == nil {
			scrapPercentage = &defaultScrap
		}

		defaultLeadTime := int32(0)
		if comp.LeadTimeDays == nil {
			comp.LeadTimeDays = &defaultLeadTime
		}

		defaultActive := true
		if isActive == nil {
			isActive = &defaultActive
		}

		defaultFixed := false
		if comp.FixedQuantity == nil {
			comp.FixedQuantity = &defaultFixed
		}

		defaultOptional := false
		if comp.IsOptional == nil {
			comp.IsOptional = &defaultOptional
		}

		// Validate scrap percentage
		if *scrapPercentage < 0 || *scrapPercentage > 100 {
			failed = append(failed, BOMFailedEntry{
				ComponentMaterialID: comp.ComponentMaterialID,
				Reason:              "Scrap percentage must be between 0 and 100",
			})
			continue
		}

		// Parse dates
		var pgEffectiveDate, pgExpiryDate pgtype.Date
		if effectiveDate != nil && *effectiveDate != "" {
			t, err := time.Parse("2006-01-02", *effectiveDate)
			if err != nil {
				failed = append(failed, BOMFailedEntry{
					ComponentMaterialID: comp.ComponentMaterialID,
					Reason:              fmt.Sprintf("Invalid effective_date format: %s", err.Error()),
				})
				continue
			}
			pgEffectiveDate = pgtype.Date{Time: t, Valid: true}
		}

		if expiryDate != nil && *expiryDate != "" {
			t, err := time.Parse("2006-01-02", *expiryDate)
			if err != nil {
				failed = append(failed, BOMFailedEntry{
					ComponentMaterialID: comp.ComponentMaterialID,
					Reason:              fmt.Sprintf("Invalid expiry_date format: %s", err.Error()),
				})
				continue
			}
			pgExpiryDate = pgtype.Date{Time: t, Valid: true}
		}

		// Check for duplicates
		exists, err := bh.h.Queries.CheckBOMExists(ctx, db.CheckBOMExistsParams{
			FinishedMaterialID:  pgtype.Int4{Int32: req.FinishedMaterialID, Valid: true},
			ComponentMaterialID: pgtype.Int4{Int32: comp.ComponentMaterialID, Valid: true},
			Version:             pgtype.Text{String: *version, Valid: true},
		})

		if err != nil {
			bh.h.Logger.Error("Failed to check BOM existence", "error", err)
			failed = append(failed, BOMFailedEntry{
				ComponentMaterialID: comp.ComponentMaterialID,
				Reason:              "Database error checking for duplicates",
			})
			continue
		}

		if exists {
			failed = append(failed, BOMFailedEntry{
				ComponentMaterialID: comp.ComponentMaterialID,
				Reason:              fmt.Sprintf("BOM already exists for this combination (version: %s)", *version),
			})
			continue
		}

		// Build create params
		var pgQuantity, pgScrap pgtype.Numeric
		pgQuantity.Scan(comp.Quantity)
		pgScrap.Scan(*scrapPercentage)

		params := db.CreateBillOfMaterialParams{
			FinishedMaterialID:  pgtype.Int4{Int32: req.FinishedMaterialID, Valid: true},
			ComponentMaterialID: pgtype.Int4{Int32: comp.ComponentMaterialID, Valid: true},
			Quantity:            pgQuantity,
			ScrapPercentage:     pgScrap,
			FixedQuantity:       pgtype.Bool{Bool: *comp.FixedQuantity, Valid: true},
			IsOptional:          pgtype.Bool{Bool: *comp.IsOptional, Valid: true},
			Priority:            pgtype.Int4{Int32: *comp.Priority, Valid: true},
			LeadTimeDays:        pgtype.Int4{Int32: *comp.LeadTimeDays, Valid: true},
			IsActive:            pgtype.Bool{Bool: *isActive, Valid: true},
			EffectiveDate:       pgEffectiveDate,
			ExpiryDate:          pgExpiryDate,
			Version:             pgtype.Text{String: *version, Valid: true},
		}

		// Optional fields
		if comp.UnitMeasureID != nil {
			params.UnitMeasureID = pgtype.Int4{Int32: *comp.UnitMeasureID, Valid: true}
		}
		if comp.Meta != nil {
			params.Meta = comp.Meta
		}
		if comp.ReferenceDesignator != nil {
			params.ReferenceDesignator = pgtype.Text{String: *comp.ReferenceDesignator, Valid: true}
		}
		if comp.Notes != nil {
			params.Notes = pgtype.Text{String: *comp.Notes, Valid: true}
		}
		if comp.OperationSequence != nil {
			params.OperationSequence = pgtype.Int4{Int32: *comp.OperationSequence, Valid: true}
		}
		if comp.EstimatedCost != nil {
			var pgCost pgtype.Numeric
			pgCost.Scan(*comp.EstimatedCost)
			params.EstimatedCost = pgCost
		}
		if comp.SupplierID != nil {
			params.SupplierID = pgtype.Int4{Int32: *comp.SupplierID, Valid: true}
		}
		if comp.AlternateComponentID != nil {
			params.AlternateComponentID = pgtype.Int4{Int32: *comp.AlternateComponentID, Valid: true}
		}

		// Create BOM entry
		bom, err := bh.h.Queries.CreateBillOfMaterial(ctx, params)
		if err != nil {
			bh.h.Logger.Error("Failed to create BOM", "error", err, "component_id", comp.ComponentMaterialID)
			failed = append(failed, BOMFailedEntry{
				ComponentMaterialID: comp.ComponentMaterialID,
				Reason:              "Failed to create BOM entry",
			})
			continue
		}

		created = append(created, bom)
	}

	response := BulkCreateBOMResponse{
		FinishedMaterialID: req.FinishedMaterialID,
		Created:            created,
		Failed:             failed,
		Summary: BOMBulkSummary{
			TotalRequested: len(req.Components),
			SuccessCount:   len(created),
			FailedCount:    len(failed),
		},
	}

	// Return 201 if at least one succeeded, 400 if all failed
	if len(created) > 0 {
		config.RespondJSON(w, http.StatusCreated, response)
	} else {
		config.RespondJSON(w, http.StatusBadRequest, response)
	}
}
