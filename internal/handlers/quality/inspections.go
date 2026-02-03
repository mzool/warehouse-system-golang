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
// QUALITY INSPECTION HANDLERS
// ============================================================================

type CreateQualityInspectionRequest struct {
	InspectionNumber   *string    `json:"inspection_number"`                   // Auto-generated if empty
	InspectionType     string     `json:"inspection_type" validate:"required"` // incoming, in_process, final, periodic, audit, customer_return
	InspectionStatus   string     `json:"inspection_status"`                   // pending, in_progress, passed, failed, partial, on_hold, cancelled
	MaterialID         int32      `json:"material_id" validate:"required"`
	BatchNumber        string     `json:"batch_number"`
	LotNumber          string     `json:"lot_number"`
	Quantity           float64    `json:"quantity" validate:"required,gt=0"`
	UnitID             *int32     `json:"unit_id"`
	PurchaseOrderID    *int32     `json:"purchase_order_id"`
	SalesOrderID       *int32     `json:"sales_order_id"`
	StockTransactionID *int32     `json:"stock_transaction_id"`
	SupplierID         *int32     `json:"supplier_id"`
	InspectionDate     *time.Time `json:"inspection_date"`
	InspectorID        *int32     `json:"inspector_id"`
	Notes              string     `json:"notes"`
	Attachments        []byte     `json:"attachments"`
}

func (qh *QualityHandler) CreateQualityInspection(w http.ResponseWriter, r *http.Request) {
	var req CreateQualityInspectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	// Validate inspection type
	validTypes := []string{"incoming", "in_process", "final", "periodic", "audit", "customer_return"}
	if !contains(validTypes, req.InspectionType) {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid inspection_type")
		return
	}

	// Validate inspection status if provided
	if req.InspectionStatus != "" {
		validStatuses := []string{"pending", "in_progress", "passed", "failed", "partial", "on_hold", "cancelled"}
		if !contains(validStatuses, req.InspectionStatus) {
			config.RespondJSON(w, http.StatusBadRequest, "Invalid inspection_status")
			return
		}
	}

	var quantity pgtype.Numeric
	if err := quantity.Scan(req.Quantity); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid quantity")
		return
	}

	params := db.CreateQualityInspectionParams{
		InspectionType: db.QualityInspectionType(req.InspectionType),
		InspectionStatus: db.NullQualityInspectionStatus{
			QualityInspectionStatus: db.QualityInspectionStatus(req.InspectionStatus),
			Valid:                   req.InspectionStatus != "",
		},
		MaterialID:  pgtype.Int4{Int32: req.MaterialID, Valid: true},
		BatchNumber: pgtype.Text{String: req.BatchNumber, Valid: req.BatchNumber != ""},
		LotNumber:   pgtype.Text{String: req.LotNumber, Valid: req.LotNumber != ""},
		Quantity:    quantity,
		Notes:       pgtype.Text{String: req.Notes, Valid: req.Notes != ""},
	}

	if req.InspectionNumber != nil && *req.InspectionNumber != "" {
		params.InspectionNumber = *req.InspectionNumber
	} else {
		params.InspectionNumber = "" // Trigger will generate
	}

	if req.UnitID != nil {
		params.UnitID = pgtype.Int4{Int32: *req.UnitID, Valid: true}
	}
	if req.PurchaseOrderID != nil {
		params.PurchaseOrderID = pgtype.Int4{Int32: *req.PurchaseOrderID, Valid: true}
	}
	if req.SalesOrderID != nil {
		params.SalesOrderID = pgtype.Int4{Int32: *req.SalesOrderID, Valid: true}
	}
	if req.StockTransactionID != nil {
		params.StockMovementID = pgtype.Int4{Int32: *req.StockTransactionID, Valid: true}
	}
	if req.SupplierID != nil {
		params.SupplierID = pgtype.Int4{Int32: *req.SupplierID, Valid: true}
	}
	if req.InspectionDate != nil {
		params.InspectionDate = pgtype.Timestamptz{Time: *req.InspectionDate, Valid: true}
	}
	if req.InspectorID != nil {
		params.InspectorID = pgtype.Int4{Int32: *req.InspectorID, Valid: true}
	}
	if req.Attachments != nil {
		params.Attachments = req.Attachments
	}

	inspection, err := qh.h.Queries.CreateQualityInspection(context.Background(), params)
	if err != nil {
		qh.h.Logger.Error("Failed to create quality inspection", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to create quality inspection")
		return
	}

	config.RespondJSON(w, http.StatusCreated, inspection)
}

func (qh *QualityHandler) GetQualityInspection(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid inspection ID")
		return
	}

	inspection, err := qh.h.Queries.GetQualityInspectionByID(context.Background(), int32(id))
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, "Quality inspection not found")
		return
	}

	config.RespondJSON(w, http.StatusOK, inspection)
}

func (qh *QualityHandler) GetQualityInspectionByNumber(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")

	inspection, err := qh.h.Queries.GetQualityInspectionByNumber(context.Background(), number)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, "Quality inspection not found")
		return
	}

	config.RespondJSON(w, http.StatusOK, inspection)
}

func (qh *QualityHandler) ListQualityInspections(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	if limit <= 0 {
		limit = 20
	}

	inspections, err := qh.h.Queries.ListQualityInspections(context.Background(), db.ListQualityInspectionsParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})

	if err != nil {
		qh.h.Logger.Error("Failed to list quality inspections", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to retrieve quality inspections")
		return
	}

	config.RespondJSON(w, http.StatusOK, inspections)
}

func (qh *QualityHandler) ListPendingInspections(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	if limit <= 0 {
		limit = 50
	}

	inspections, err := qh.h.Queries.ListPendingInspections(context.Background(), db.ListPendingInspectionsParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})

	if err != nil {
		qh.h.Logger.Error("Failed to list pending inspections", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to retrieve pending inspections")
		return
	}

	config.RespondJSON(w, http.StatusOK, inspections)
}

func (qh *QualityHandler) ListQualityInspectionsByStatus(w http.ResponseWriter, r *http.Request) {
	status := r.PathValue("status")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	if limit <= 0 {
		limit = 20
	}

	inspections, err := qh.h.Queries.ListQualityInspectionsByStatus(context.Background(), db.ListQualityInspectionsByStatusParams{
		InspectionStatus: db.NullQualityInspectionStatus{
			QualityInspectionStatus: db.QualityInspectionStatus(status),
			Valid:                   true,
		},
		Limit:  int32(limit),
		Offset: int32(offset),
	})

	if err != nil {
		qh.h.Logger.Error("Failed to list inspections by status", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to retrieve inspections")
		return
	}

	config.RespondJSON(w, http.StatusOK, inspections)
}

func (qh *QualityHandler) ListQualityInspectionsByMaterial(w http.ResponseWriter, r *http.Request) {
	materialIDStr := r.PathValue("material_id")
	materialID, err := strconv.Atoi(materialIDStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid material ID")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	if limit <= 0 {
		limit = 20
	}

	inspections, err := qh.h.Queries.ListQualityInspectionsByMaterial(context.Background(), db.ListQualityInspectionsByMaterialParams{
		MaterialID: pgtype.Int4{Int32: int32(materialID), Valid: true},
		Limit:      int32(limit),
		Offset:     int32(offset),
	})

	if err != nil {
		qh.h.Logger.Error("Failed to list inspections by material", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to retrieve inspections")
		return
	}

	config.RespondJSON(w, http.StatusOK, inspections)
}

func (qh *QualityHandler) ListQualityInspectionsByBatch(w http.ResponseWriter, r *http.Request) {
	batch := r.PathValue("batch")

	inspections, err := qh.h.Queries.ListQualityInspectionsByBatch(context.Background(), pgtype.Text{String: batch, Valid: true})

	if err != nil {
		qh.h.Logger.Error("Failed to list inspections by batch", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to retrieve inspections")
		return
	}

	config.RespondJSON(w, http.StatusOK, inspections)
}

type UpdateQualityInspectionRequest struct {
	InspectionStatus *string    `json:"inspection_status"`
	InspectionDate   *time.Time `json:"inspection_date"`
	InspectorID      *int32     `json:"inspector_id"`
	ApprovedByID     *int32     `json:"approved_by_id"`
	QuantityPassed   *float64   `json:"quantity_passed"`
	QuantityFailed   *float64   `json:"quantity_failed"`
	QuantityOnHold   *float64   `json:"quantity_on_hold"`
	FinalDecision    *string    `json:"final_decision"` // unrestricted, quarantine, blocked, rejected
	DecisionDate     *time.Time `json:"decision_date"`
	Notes            *string    `json:"notes"`
	Attachments      []byte     `json:"attachments"`
}

func (qh *QualityHandler) UpdateQualityInspection(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid inspection ID")
		return
	}

	var req UpdateQualityInspectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	params := db.UpdateQualityInspectionParams{
		ID: int32(id),
	}

	if req.InspectionStatus != nil {
		params.InspectionStatus = db.NullQualityInspectionStatus{
			QualityInspectionStatus: db.QualityInspectionStatus(*req.InspectionStatus),
			Valid:                   true,
		}
	}
	if req.InspectionDate != nil {
		params.InspectionDate = pgtype.Timestamptz{Time: *req.InspectionDate, Valid: true}
	}
	if req.InspectorID != nil {
		params.InspectorID = pgtype.Int4{Int32: *req.InspectorID, Valid: true}
	}
	if req.ApprovedByID != nil {
		params.ApprovedByID = pgtype.Int4{Int32: *req.ApprovedByID, Valid: true}
	}
	if req.QuantityPassed != nil {
		if err := params.QuantityPassed.Scan(*req.QuantityPassed); err != nil {
			config.RespondJSON(w, http.StatusBadRequest, "Invalid quantity_passed")
			return
		}
	}
	if req.QuantityFailed != nil {
		if err := params.QuantityFailed.Scan(*req.QuantityFailed); err != nil {
			config.RespondJSON(w, http.StatusBadRequest, "Invalid quantity_failed")
			return
		}
	}
	if req.QuantityOnHold != nil {
		if err := params.QuantityOnHold.Scan(*req.QuantityOnHold); err != nil {
			config.RespondJSON(w, http.StatusBadRequest, "Invalid quantity_on_hold")
			return
		}
	}
	if req.FinalDecision != nil {
		params.FinalDecision = db.NullQualityStatus{
			QualityStatus: db.QualityStatus(*req.FinalDecision),
			Valid:         true,
		}
	}
	if req.DecisionDate != nil {
		params.DecisionDate = pgtype.Timestamptz{Time: *req.DecisionDate, Valid: true}
	}
	if req.Notes != nil {
		params.Notes = pgtype.Text{String: *req.Notes, Valid: true}
	}
	if req.Attachments != nil {
		params.Attachments = req.Attachments
	}

	inspection, err := qh.h.Queries.UpdateQualityInspection(context.Background(), params)
	if err != nil {
		qh.h.Logger.Error("Failed to update quality inspection", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to update quality inspection")
		return
	}

	config.RespondJSON(w, http.StatusOK, inspection)
}

func (qh *QualityHandler) DeleteQualityInspection(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid inspection ID")
		return
	}

	err = qh.h.Queries.DeleteQualityInspection(context.Background(), int32(id))
	if err != nil {
		qh.h.Logger.Error("Failed to delete quality inspection", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to delete quality inspection")
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]string{"message": "Quality inspection deleted successfully"})
}

// ============================================================================
// INSPECTION RESULTS HANDLERS
// ============================================================================

type CreateInspectionResultRequest struct {
	InspectionID  int32    `json:"inspection_id" validate:"required"`
	CriteriaID    *int32   `json:"criteria_id"`
	CriteriaName  string   `json:"criteria_name" validate:"required"`
	MeasuredValue *float64 `json:"measured_value"`
	TextValue     string   `json:"text_value"`
	IsPassed      *bool    `json:"is_passed"`
	Deviation     *float64 `json:"deviation"`
	SampleNumber  *int32   `json:"sample_number"`
	Notes         string   `json:"notes"`
	PhotoURLs     []byte   `json:"photo_urls"`
}

func (qh *QualityHandler) CreateInspectionResult(w http.ResponseWriter, r *http.Request) {
	var req CreateInspectionResultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	params := db.CreateQualityInspectionResultParams{
		InspectionID: req.InspectionID,
		CriteriaName: req.CriteriaName,
		TextValue:    pgtype.Text{String: req.TextValue, Valid: req.TextValue != ""},
		Notes:        pgtype.Text{String: req.Notes, Valid: req.Notes != ""},
	}

	if req.CriteriaID != nil {
		params.CriteriaID = pgtype.Int4{Int32: *req.CriteriaID, Valid: true}
	}
	if req.MeasuredValue != nil {
		if err := params.MeasuredValue.Scan(*req.MeasuredValue); err != nil {
			config.RespondJSON(w, http.StatusBadRequest, "Invalid measured_value")
			return
		}
	}
	if req.IsPassed != nil {
		params.IsPassed = pgtype.Bool{Bool: *req.IsPassed, Valid: true}
	}
	if req.Deviation != nil {
		if err := params.Deviation.Scan(*req.Deviation); err != nil {
			config.RespondJSON(w, http.StatusBadRequest, "Invalid deviation")
			return
		}
	}
	if req.SampleNumber != nil {
		params.SampleNumber = pgtype.Int4{Int32: *req.SampleNumber, Valid: true}
	}
	if req.PhotoURLs != nil {
		params.PhotoUrls = req.PhotoURLs
	}

	result, err := qh.h.Queries.CreateQualityInspectionResult(context.Background(), params)
	if err != nil {
		qh.h.Logger.Error("Failed to create inspection result", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to create inspection result")
		return
	}

	config.RespondJSON(w, http.StatusCreated, result)
}

func (qh *QualityHandler) ListInspectionResults(w http.ResponseWriter, r *http.Request) {
	inspectionIDStr := r.PathValue("inspection_id")
	inspectionID, err := strconv.Atoi(inspectionIDStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid inspection ID")
		return
	}

	results, err := qh.h.Queries.ListQualityInspectionResults(context.Background(), int32(inspectionID))
	if err != nil {
		qh.h.Logger.Error("Failed to list inspection results", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to retrieve inspection results")
		return
	}

	config.RespondJSON(w, http.StatusOK, results)
}

// Helper function
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
