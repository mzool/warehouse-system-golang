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
// NON-CONFORMANCE REPORT (NCR) HANDLERS
// ============================================================================

type CreateNCRRequest struct {
	NCRNumber        *string    `json:"ncr_number"` // Auto-generated if empty
	Title            string     `json:"title" validate:"required"`
	Description      string     `json:"description" validate:"required"`
	NCRType          string     `json:"ncr_type" validate:"required"` // supplier, process, customer, audit, other
	Severity         string     `json:"severity" validate:"required"` // critical, major, minor, observation
	Status           string     `json:"status"`                       // open, investigating, action_required, in_progress, resolved, closed, cancelled
	MaterialID       *int32     `json:"material_id"`
	BatchNumber      string     `json:"batch_number"`
	QuantityAffected *float64   `json:"quantity_affected"`
	UnitID           *int32     `json:"unit_id"`
	InspectionID     *int32     `json:"inspection_id"`
	SupplierID       *int32     `json:"supplier_id"`
	CustomerID       *int32     `json:"customer_id"`
	PurchaseOrderID  *int32     `json:"purchase_order_id"`
	SalesOrderID     *int32     `json:"sales_order_id"`
	ReportedBy       *int32     `json:"reported_by"`
	ReportedDate     *time.Time `json:"reported_date"`
	Notes            string     `json:"notes"`
	Attachments      []byte     `json:"attachments"`
}

func (qh *QualityHandler) CreateNCR(w http.ResponseWriter, r *http.Request) {
	var req CreateNCRRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	// Validate NCR type
	validTypes := []string{"supplier", "process", "customer", "audit", "other"}
	if !contains(validTypes, req.NCRType) {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid ncr_type")
		return
	}

	// Validate severity
	validSeverities := []string{"critical", "major", "minor", "observation"}
	if !contains(validSeverities, req.Severity) {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid severity")
		return
	}

	params := db.CreateNonConformanceReportParams{
		Title:       req.Title,
		Description: req.Description,
		NcrType:     db.NcrType(req.NCRType),
		Severity:    db.NcrSeverity(req.Severity),
		Status:      db.NullNcrStatus{NcrStatus: db.NcrStatus(req.Status), Valid: req.Status != ""},
		BatchNumber: pgtype.Text{String: req.BatchNumber, Valid: req.BatchNumber != ""},
		Notes:       pgtype.Text{String: req.Notes, Valid: req.Notes != ""},
	}

	if req.NCRNumber != nil && *req.NCRNumber != "" {
		params.NcrNumber = *req.NCRNumber
	} else {
		params.NcrNumber = "" // Trigger will generate
	}

	if req.MaterialID != nil {
		params.MaterialID = pgtype.Int4{Int32: *req.MaterialID, Valid: true}
	}
	if req.QuantityAffected != nil {
		if err := params.QuantityAffected.Scan(*req.QuantityAffected); err != nil {
			config.RespondJSON(w, http.StatusBadRequest, "Invalid quantity_affected")
			return
		}
	}
	if req.UnitID != nil {
		params.UnitID = pgtype.Int4{Int32: *req.UnitID, Valid: true}
	}
	if req.InspectionID != nil {
		params.InspectionID = pgtype.Int4{Int32: *req.InspectionID, Valid: true}
	}
	if req.SupplierID != nil {
		params.SupplierID = pgtype.Int4{Int32: *req.SupplierID, Valid: true}
	}
	if req.CustomerID != nil {
		params.CustomerID = pgtype.Int4{Int32: *req.CustomerID, Valid: true}
	}
	if req.PurchaseOrderID != nil {
		params.PurchaseOrderID = pgtype.Int4{Int32: *req.PurchaseOrderID, Valid: true}
	}
	if req.SalesOrderID != nil {
		params.SalesOrderID = pgtype.Int4{Int32: *req.SalesOrderID, Valid: true}
	}
	if req.ReportedBy != nil {
		params.ReportedBy = pgtype.Int4{Int32: *req.ReportedBy, Valid: true}
	}
	if req.ReportedDate != nil {
		params.ReportedDate = pgtype.Timestamptz{Time: *req.ReportedDate, Valid: true}
	}
	if req.Attachments != nil {
		params.Attachments = req.Attachments
	}

	ncr, err := qh.h.Queries.CreateNonConformanceReport(context.Background(), params)
	if err != nil {
		qh.h.Logger.Error("Failed to create NCR", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to create NCR")
		return
	}

	config.RespondJSON(w, http.StatusCreated, ncr)
}

func (qh *QualityHandler) GetNCR(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid NCR ID")
		return
	}

	ncr, err := qh.h.Queries.GetNonConformanceReportByID(context.Background(), int32(id))
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, "NCR not found")
		return
	}

	config.RespondJSON(w, http.StatusOK, ncr)
}

func (qh *QualityHandler) GetNCRByNumber(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")

	ncr, err := qh.h.Queries.GetNonConformanceReportByNumber(context.Background(), number)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, "NCR not found")
		return
	}

	config.RespondJSON(w, http.StatusOK, ncr)
}

func (qh *QualityHandler) ListNCRs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	if limit <= 0 {
		limit = 20
	}

	ncrs, err := qh.h.Queries.ListNonConformanceReports(context.Background(), db.ListNonConformanceReportsParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})

	if err != nil {
		qh.h.Logger.Error("Failed to list NCRs", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to retrieve NCRs")
		return
	}

	config.RespondJSON(w, http.StatusOK, ncrs)
}

func (qh *QualityHandler) ListOpenNCRs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	if limit <= 0 {
		limit = 50
	}

	ncrs, err := qh.h.Queries.ListOpenNonConformanceReports(context.Background(), db.ListOpenNonConformanceReportsParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})

	if err != nil {
		qh.h.Logger.Error("Failed to list open NCRs", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to retrieve open NCRs")
		return
	}

	config.RespondJSON(w, http.StatusOK, ncrs)
}

func (qh *QualityHandler) ListOverdueNCRActions(w http.ResponseWriter, r *http.Request) {
	ncrs, err := qh.h.Queries.ListOverdueNCRActions(context.Background())

	if err != nil {
		qh.h.Logger.Error("Failed to list overdue NCRs", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to retrieve overdue NCRs")
		return
	}

	config.RespondJSON(w, http.StatusOK, ncrs)
}

type UpdateNCRRequest struct {
	Status              *string    `json:"status"`
	RootCause           *string    `json:"root_cause"`
	RootCauseAnalysisBy *int32     `json:"root_cause_analysis_by"`
	RootCauseDate       *time.Time `json:"root_cause_date"`
	CorrectiveAction    *string    `json:"corrective_action"`
	PreventiveAction    *string    `json:"preventive_action"`
	ActionAssignedTo    *int32     `json:"action_assigned_to"`
	ActionDueDate       *time.Time `json:"action_due_date"`
	ActionCompletedDate *time.Time `json:"action_completed_date"`
	Disposition         *string    `json:"disposition"` // rework, scrap, return_to_supplier, use_as_is, sort
	CostImpact          *float64   `json:"cost_impact"`
	ClosedBy            *int32     `json:"closed_by"`
	ClosedDate          *time.Time `json:"closed_date"`
	Notes               *string    `json:"notes"`
	Attachments         []byte     `json:"attachments"`
}

func (qh *QualityHandler) UpdateNCR(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid NCR ID")
		return
	}

	var req UpdateNCRRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	params := db.UpdateNonConformanceReportParams{
		ID: int32(id),
	}

	if req.Status != nil {
		params.Status = db.NullNcrStatus{
			NcrStatus: db.NcrStatus(*req.Status),
			Valid:     true,
		}
	}
	if req.RootCause != nil {
		params.RootCause = pgtype.Text{String: *req.RootCause, Valid: true}
	}
	if req.RootCauseAnalysisBy != nil {
		params.RootCauseAnalysisBy = pgtype.Int4{Int32: *req.RootCauseAnalysisBy, Valid: true}
	}
	if req.RootCauseDate != nil {
		params.RootCauseDate = pgtype.Timestamptz{Time: *req.RootCauseDate, Valid: true}
	}
	if req.CorrectiveAction != nil {
		params.CorrectiveAction = pgtype.Text{String: *req.CorrectiveAction, Valid: true}
	}
	if req.PreventiveAction != nil {
		params.PreventiveAction = pgtype.Text{String: *req.PreventiveAction, Valid: true}
	}
	if req.ActionAssignedTo != nil {
		params.ActionAssignedTo = pgtype.Int4{Int32: *req.ActionAssignedTo, Valid: true}
	}
	if req.ActionDueDate != nil {
		params.ActionDueDate = pgtype.Date{Time: *req.ActionDueDate, Valid: true}
	}
	if req.ActionCompletedDate != nil {
		params.ActionCompletedDate = pgtype.Date{Time: *req.ActionCompletedDate, Valid: true}
	}
	if req.Disposition != nil {
		params.Disposition = pgtype.Text{String: *req.Disposition, Valid: true}
	}
	if req.CostImpact != nil {
		if err := params.CostImpact.Scan(*req.CostImpact); err != nil {
			config.RespondJSON(w, http.StatusBadRequest, "Invalid cost_impact")
			return
		}
	}
	if req.ClosedBy != nil {
		params.ClosedBy = pgtype.Int4{Int32: *req.ClosedBy, Valid: true}
	}
	if req.ClosedDate != nil {
		params.ClosedDate = pgtype.Timestamptz{Time: *req.ClosedDate, Valid: true}
	}
	if req.Notes != nil {
		params.Notes = pgtype.Text{String: *req.Notes, Valid: true}
	}
	if req.Attachments != nil {
		params.Attachments = req.Attachments
	}

	ncr, err := qh.h.Queries.UpdateNonConformanceReport(context.Background(), params)
	if err != nil {
		qh.h.Logger.Error("Failed to update NCR", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to update NCR")
		return
	}

	config.RespondJSON(w, http.StatusOK, ncr)
}

func (qh *QualityHandler) DeleteNCR(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid NCR ID")
		return
	}

	err = qh.h.Queries.DeleteNonConformanceReport(context.Background(), int32(id))
	if err != nil {
		qh.h.Logger.Error("Failed to delete NCR", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to delete NCR")
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]string{"message": "NCR deleted successfully"})
}
