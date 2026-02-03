package quality

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"warehouse_system/internal/config"
	db "warehouse_system/internal/database/db"

	"github.com/jackc/pgx/v5/pgtype"
)

// ============================================================================
// QUALITY HOLDS HANDLERS
// ============================================================================

type CreateQualityHoldRequest struct {
	HoldNumber          *string    `json:"hold_number"` // Auto-generated if empty
	MaterialID          int32      `json:"material_id" validate:"required"`
	WarehouseID         *int32     `json:"warehouse_id"`
	BatchNumber         string     `json:"batch_number"`
	LotNumber           string     `json:"lot_number"`
	Quantity            float64    `json:"quantity" validate:"required,gt=0"`
	UnitID              *int32     `json:"unit_id"`
	QualityStatus       string     `json:"quality_status" validate:"required"` // unrestricted, quarantine, blocked, rejected
	HoldReason          string     `json:"hold_reason" validate:"required"`
	InspectionID        *int32     `json:"inspection_id"`
	NCRID               *int32     `json:"ncr_id"`
	PlacedBy            *int32     `json:"placed_by"`
	PlacedDate          *time.Time `json:"placed_date"`
	ExpectedReleaseDate *time.Time `json:"expected_release_date"`
}

func (qh *QualityHandler) CreateQualityHold(w http.ResponseWriter, r *http.Request) {
	var req CreateQualityHoldRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	// Validate quality status
	validStatuses := []string{"unrestricted", "quarantine", "blocked", "rejected"}
	if !contains(validStatuses, req.QualityStatus) {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid quality_status")
		return
	}

	var quantity pgtype.Numeric
	if err := quantity.Scan(req.Quantity); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid quantity")
		return
	}

	params := db.CreateQualityHoldParams{
		MaterialID:    req.MaterialID,
		BatchNumber:   pgtype.Text{String: req.BatchNumber, Valid: req.BatchNumber != ""},
		LotNumber:     pgtype.Text{String: req.LotNumber, Valid: req.LotNumber != ""},
		Quantity:      quantity,
		QualityStatus: db.QualityStatus(req.QualityStatus),
		HoldReason:    req.HoldReason,
	}

	if req.HoldNumber != nil && *req.HoldNumber != "" {
		params.HoldNumber = *req.HoldNumber
	} else {
		params.HoldNumber = "" // Trigger will generate
	}

	if req.WarehouseID != nil {
		params.WarehouseID = pgtype.Int4{Int32: *req.WarehouseID, Valid: true}
	}
	if req.UnitID != nil {
		params.UnitID = pgtype.Int4{Int32: *req.UnitID, Valid: true}
	}
	if req.InspectionID != nil {
		params.InspectionID = pgtype.Int4{Int32: *req.InspectionID, Valid: true}
	}
	if req.NCRID != nil {
		params.NcrID = pgtype.Int4{Int32: *req.NCRID, Valid: true}
	}
	if req.PlacedBy != nil {
		params.PlacedBy = pgtype.Int4{Int32: *req.PlacedBy, Valid: true}
	}
	if req.PlacedDate != nil {
		params.PlacedDate = pgtype.Timestamptz{Time: *req.PlacedDate, Valid: true}
	}
	if req.ExpectedReleaseDate != nil {
		params.ExpectedReleaseDate = pgtype.Date{Time: *req.ExpectedReleaseDate, Valid: true}
	}

	hold, err := qh.h.Queries.CreateQualityHold(context.Background(), params)
	if err != nil {
		qh.h.Logger.Error("Failed to create quality hold", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to create quality hold")
		return
	}

	config.RespondJSON(w, http.StatusCreated, hold)
}

func (qh *QualityHandler) GetQualityHold(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid hold ID")
		return
	}

	hold, err := qh.h.Queries.GetQualityHoldByID(context.Background(), int32(id))
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, "Quality hold not found")
		return
	}

	config.RespondJSON(w, http.StatusOK, hold)
}

func (qh *QualityHandler) GetQualityHoldByNumber(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")

	hold, err := qh.h.Queries.GetQualityHoldByNumber(context.Background(), number)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, "Quality hold not found")
		return
	}

	config.RespondJSON(w, http.StatusOK, hold)
}

func (qh *QualityHandler) ListQualityHolds(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	if limit <= 0 {
		limit = 20
	}

	holds, err := qh.h.Queries.ListQualityHolds(context.Background(), db.ListQualityHoldsParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})

	if err != nil {
		qh.h.Logger.Error("Failed to list quality holds", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to retrieve quality holds")
		return
	}

	config.RespondJSON(w, http.StatusOK, holds)
}

func (qh *QualityHandler) ListActiveQualityHolds(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	if limit <= 0 {
		limit = 50
	}

	holds, err := qh.h.Queries.ListActiveQualityHolds(context.Background(), db.ListActiveQualityHoldsParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})

	if err != nil {
		qh.h.Logger.Error("Failed to list active quality holds", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to retrieve active quality holds")
		return
	}

	config.RespondJSON(w, http.StatusOK, holds)
}

func (qh *QualityHandler) ListQualityHoldsByMaterial(w http.ResponseWriter, r *http.Request) {
	materialIDStr := r.PathValue("material_id")
	materialID, err := strconv.Atoi(materialIDStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid material ID")
		return
	}

	holds, err := qh.h.Queries.ListQualityHoldsByMaterial(context.Background(), int32(materialID))

	if err != nil {
		qh.h.Logger.Error("Failed to list quality holds by material", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to retrieve quality holds")
		return
	}

	config.RespondJSON(w, http.StatusOK, holds)
}

func (qh *QualityHandler) ListQualityHoldsByBatch(w http.ResponseWriter, r *http.Request) {
	batch := r.PathValue("batch")

	holds, err := qh.h.Queries.ListQualityHoldsByBatch(context.Background(), pgtype.Text{String: batch, Valid: true})

	if err != nil {
		qh.h.Logger.Error("Failed to list quality holds by batch", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to retrieve quality holds")
		return
	}

	config.RespondJSON(w, http.StatusOK, holds)
}

type UpdateQualityHoldRequest struct {
	QualityStatus       *string    `json:"quality_status"`
	HoldReason          *string    `json:"hold_reason"`
	ExpectedReleaseDate *time.Time `json:"expected_release_date"`
	IsReleased          *bool      `json:"is_released"`
	ReleasedBy          *int32     `json:"released_by"`
	ReleasedDate        *time.Time `json:"released_date"`
	ReleaseNotes        *string    `json:"release_notes"`
}

func (qh *QualityHandler) UpdateQualityHold(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid hold ID")
		return
	}

	var req UpdateQualityHoldRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	params := db.UpdateQualityHoldParams{
		ID: int32(id),
	}

	if req.QualityStatus != nil {
		params.QualityStatus = db.NullQualityStatus{
			QualityStatus: db.QualityStatus(*req.QualityStatus),
			Valid:         true,
		}
	}
	if req.HoldReason != nil {
		params.HoldReason = pgtype.Text{String: *req.HoldReason, Valid: true}
	}
	if req.ExpectedReleaseDate != nil {
		params.ExpectedReleaseDate = pgtype.Date{Time: *req.ExpectedReleaseDate, Valid: true}
	}
	if req.IsReleased != nil {
		params.IsReleased = pgtype.Bool{Bool: *req.IsReleased, Valid: true}
	}
	if req.ReleasedBy != nil {
		params.ReleasedBy = pgtype.Int4{Int32: *req.ReleasedBy, Valid: true}
	}
	if req.ReleasedDate != nil {
		params.ReleasedDate = pgtype.Timestamptz{Time: *req.ReleasedDate, Valid: true}
	}
	if req.ReleaseNotes != nil {
		params.ReleaseNotes = pgtype.Text{String: *req.ReleaseNotes, Valid: true}
	}

	hold, err := qh.h.Queries.UpdateQualityHold(context.Background(), params)
	if err != nil {
		qh.h.Logger.Error("Failed to update quality hold", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to update quality hold")
		return
	}

	config.RespondJSON(w, http.StatusOK, hold)
}

type ReleaseQualityHoldRequest struct {
	ReleasedBy   int32  `json:"released_by" validate:"required"`
	ReleaseNotes string `json:"release_notes"`
}

func (qh *QualityHandler) ReleaseQualityHold(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid hold ID")
		return
	}

	var req ReleaseQualityHoldRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	hold, err := qh.h.Queries.ReleaseQualityHold(context.Background(), db.ReleaseQualityHoldParams{
		ID:           int32(id),
		ReleasedBy:   pgtype.Int4{Int32: req.ReleasedBy, Valid: true},
		ReleaseNotes: pgtype.Text{String: req.ReleaseNotes, Valid: req.ReleaseNotes != ""},
	})

	if err != nil {
		qh.h.Logger.Error("Failed to release quality hold", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to release quality hold")
		return
	}

	config.RespondJSON(w, http.StatusOK, hold)
}

func (qh *QualityHandler) DeleteQualityHold(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid hold ID")
		return
	}

	err = qh.h.Queries.DeleteQualityHold(context.Background(), int32(id))
	if err != nil {
		qh.h.Logger.Error("Failed to delete quality hold", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to delete quality hold")
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]string{"message": "Quality hold deleted successfully"})
}
