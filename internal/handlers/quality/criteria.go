package quality

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"warehouse_system/internal/config"
	db "warehouse_system/internal/database/db"

	"github.com/jackc/pgx/v5/pgtype"
)

// ============================================================================
// INSPECTION CRITERIA HANDLERS
// ============================================================================

type CreateInspectionCriteriaRequest struct {
	Name          string   `json:"name" validate:"required"`
	Description   string   `json:"description"`
	CriteriaType  string   `json:"criteria_type" validate:"required"` // visual, measurement, functional, document
	Specification string   `json:"specification"`
	UnitID        *int32   `json:"unit_id"`
	ToleranceMin  *float64 `json:"tolerance_min"`
	ToleranceMax  *float64 `json:"tolerance_max"`
	IsCritical    bool     `json:"is_critical"`
	IsActive      bool     `json:"is_active"`
}

func (qh *QualityHandler) CreateInspectionCriteria(w http.ResponseWriter, r *http.Request) {
	var req CreateInspectionCriteriaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	var unitID pgtype.Int4
	if req.UnitID != nil {
		unitID = pgtype.Int4{Int32: *req.UnitID, Valid: true}
	}

	var toleranceMin, toleranceMax pgtype.Numeric
	if req.ToleranceMin != nil {
		if err := toleranceMin.Scan(*req.ToleranceMin); err != nil {
			config.RespondJSON(w, http.StatusBadRequest, "Invalid tolerance_min")
			return
		}
	}
	if req.ToleranceMax != nil {
		if err := toleranceMax.Scan(*req.ToleranceMax); err != nil {
			config.RespondJSON(w, http.StatusBadRequest, "Invalid tolerance_max")
			return
		}
	}

	criteria, err := qh.h.Queries.CreateQualityInspectionCriteria(context.Background(), db.CreateQualityInspectionCriteriaParams{
		Name:          req.Name,
		Description:   pgtype.Text{String: req.Description, Valid: req.Description != ""},
		CriteriaType:  req.CriteriaType,
		Specification: pgtype.Text{String: req.Specification, Valid: req.Specification != ""},
		UnitID:        unitID,
		ToleranceMin:  toleranceMin,
		ToleranceMax:  toleranceMax,
		IsCritical:    pgtype.Bool{Bool: req.IsCritical, Valid: true},
		IsActive:      pgtype.Bool{Bool: req.IsActive, Valid: true},
	})

	if err != nil {
		qh.h.Logger.Error("Failed to create inspection criteria", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to create inspection criteria")
		return
	}

	config.RespondJSON(w, http.StatusCreated, criteria)
}

func (qh *QualityHandler) GetInspectionCriteria(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid criteria ID")
		return
	}

	criteria, err := qh.h.Queries.GetQualityInspectionCriteriaByID(context.Background(), int32(id))
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, "Inspection criteria not found")
		return
	}

	config.RespondJSON(w, http.StatusOK, criteria)
}

func (qh *QualityHandler) ListInspectionCriteria(w http.ResponseWriter, r *http.Request) {
	criteria, err := qh.h.Queries.ListQualityInspectionCriteria(context.Background())
	if err != nil {
		qh.h.Logger.Error("Failed to list inspection criteria", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to retrieve inspection criteria")
		return
	}

	config.RespondJSON(w, http.StatusOK, criteria)
}

func (qh *QualityHandler) SearchInspectionCriteria(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	if limit <= 0 {
		limit = 20
	}

	criteria, err := qh.h.Queries.SearchQualityInspectionCriteria(context.Background(), db.SearchQualityInspectionCriteriaParams{
		Column1: pgtype.Text{String: query, Valid: true},
		Limit:   int32(limit),
		Offset:  int32(offset),
	})

	if err != nil {
		qh.h.Logger.Error("Failed to search inspection criteria", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to search inspection criteria")
		return
	}

	config.RespondJSON(w, http.StatusOK, criteria)
}

type UpdateInspectionCriteriaRequest struct {
	Name          *string  `json:"name"`
	Description   *string  `json:"description"`
	CriteriaType  *string  `json:"criteria_type"`
	Specification *string  `json:"specification"`
	UnitID        *int32   `json:"unit_id"`
	ToleranceMin  *float64 `json:"tolerance_min"`
	ToleranceMax  *float64 `json:"tolerance_max"`
	IsCritical    *bool    `json:"is_critical"`
	IsActive      *bool    `json:"is_active"`
}

func (qh *QualityHandler) UpdateInspectionCriteria(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid criteria ID")
		return
	}

	var req UpdateInspectionCriteriaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	params := db.UpdateQualityInspectionCriteriaParams{
		ID: int32(id),
	}

	if req.Name != nil {
		params.Name = pgtype.Text{String: *req.Name, Valid: true}
	}
	if req.Description != nil {
		params.Description = pgtype.Text{String: *req.Description, Valid: true}
	}
	if req.CriteriaType != nil {
		params.CriteriaType = pgtype.Text{String: *req.CriteriaType, Valid: true}
	}
	if req.Specification != nil {
		params.Specification = pgtype.Text{String: *req.Specification, Valid: true}
	}
	if req.UnitID != nil {
		params.UnitID = pgtype.Int4{Int32: *req.UnitID, Valid: true}
	}
	if req.ToleranceMin != nil {
		if err := params.ToleranceMin.Scan(*req.ToleranceMin); err != nil {
			config.RespondJSON(w, http.StatusBadRequest, "Invalid tolerance_min")
			return
		}
	}
	if req.ToleranceMax != nil {
		if err := params.ToleranceMax.Scan(*req.ToleranceMax); err != nil {
			config.RespondJSON(w, http.StatusBadRequest, "Invalid tolerance_max")
			return
		}
	}
	if req.IsCritical != nil {
		params.IsCritical = pgtype.Bool{Bool: *req.IsCritical, Valid: true}
	}
	if req.IsActive != nil {
		params.IsActive = pgtype.Bool{Bool: *req.IsActive, Valid: true}
	}

	criteria, err := qh.h.Queries.UpdateQualityInspectionCriteria(context.Background(), params)
	if err != nil {
		qh.h.Logger.Error("Failed to update inspection criteria", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to update inspection criteria")
		return
	}

	config.RespondJSON(w, http.StatusOK, criteria)
}

func (qh *QualityHandler) DeleteInspectionCriteria(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid criteria ID")
		return
	}

	err = qh.h.Queries.DeleteQualityInspectionCriteria(context.Background(), int32(id))
	if err != nil {
		qh.h.Logger.Error("Failed to delete inspection criteria", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to delete inspection criteria")
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]string{"message": "Inspection criteria deleted successfully"})
}
