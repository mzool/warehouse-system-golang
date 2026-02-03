package laboratory

import (
	"encoding/json"
	"net/http"
	"strconv"
	"warehouse_system/internal/config"
	db "warehouse_system/internal/database/db"
	"warehouse_system/internal/middlewares"

	"github.com/jackc/pgx/v5/pgtype"
)

// ============================================================================
// Lab Samples
// ============================================================================

// CreateLabSample creates a new lab sample
func (l *LaboratoryHandler) CreateLabSample(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SampleNumber        string  `json:"sample_number"`
		MaterialID          int32   `json:"material_id"`
		BatchNumber         string  `json:"batch_number"`
		LotNumber           string  `json:"lot_number"`
		SampleType          string  `json:"sample_type"`
		SampleSource        string  `json:"sample_source"`
		SupplierID          int32   `json:"supplier_id"`
		InspectionID        int32   `json:"inspection_id"`
		SampleQuantity      float64 `json:"sample_quantity"`
		SampleUnitID        int32   `json:"sample_unit_id"`
		CollectedDate       string  `json:"collected_date"`
		CollectedBy         int32   `json:"collected_by"`
		ReceivedDate        string  `json:"received_date"`
		ReceivedBy          int32   `json:"received_by"`
		StorageLocation     string  `json:"storage_location"`
		StorageConditions   string  `json:"storage_conditions"`
		RetentionPeriodDays int32   `json:"retention_period_days"`
		DisposalDate        string  `json:"disposal_date"`
		SampleStatus        string  `json:"sample_status"`
		Priority            string  `json:"priority"`
		ChainOfCustody      string  `json:"chain_of_custody"`
		Attachments         string  `json:"attachments"`
		Notes               string  `json:"notes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
		return
	}

	sample, err := l.h.Queries.CreateLabSample(r.Context(), db.CreateLabSampleParams{
		SampleNumber:         req.SampleNumber,
		SampleType:           db.LabSampleType(req.SampleType),
		SampleStatus:         db.NullLabSampleStatus{LabSampleStatus: db.LabSampleStatus(req.SampleStatus), Valid: req.SampleStatus != ""},
		MaterialID:           int32Ptr(req.MaterialID),
		BatchNumber:          textPtr(req.BatchNumber),
		LotNumber:            textPtr(req.LotNumber),
		QualityInspectionID:  int32Ptr(req.InspectionID),
		PurchaseOrderID:      pgtype.Int4{},
		StockTransactionID:   pgtype.Int4{},
		SampleQuantity:       floatToNumericPrecision(req.SampleQuantity, 4),
		SampleUnitID:         int32Ptr(req.SampleUnitID),
		ContainerType:        pgtype.Text{},
		ContainerCount:       pgtype.Int4{},
		StorageLocation:      textPtr(req.StorageLocation),
		StorageConditions:    textPtr(req.StorageConditions),
		CollectedBy:          int32Ptr(req.CollectedBy),
		CollectionDate:       timestampOrNow(req.CollectedDate),
		CollectionMethod:     pgtype.Text{},
		SamplingPlan:         pgtype.Text{},
		ReceivedByLab:        int32Ptr(req.ReceivedBy),
		LabReceivedDate:      parseFlexibleTimestamp(req.ReceivedDate),
		RetentionRequired:    pgtype.Bool{},
		RetentionPeriodDays:  int32Ptr(req.RetentionPeriodDays),
		RetentionExpiryDate:  pgtype.Date{Valid: false},
		IsExternalLab:        pgtype.Bool{},
		ExternalLabName:      pgtype.Text{},
		ExternalLabReference: pgtype.Text{},
		SentToLabDate:        pgtype.Date{},
		ExpectedResultsDate:  pgtype.Date{},
		Attachments:          []byte(req.Attachments),
		Notes:                pgtype.Text{String: req.Notes, Valid: req.Notes != ""},
	})

	if err != nil {
		l.h.Logger.Error("Failed to create lab sample", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create lab sample"})
		return
	}

	config.RespondJSON(w, http.StatusCreated, sample)
}

// GetLabSample retrieves a lab sample by ID
func (l *LaboratoryHandler) GetLabSample(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
		return
	}

	sample, err := l.h.Queries.GetLabSampleByID(r.Context(), int32(id))
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Sample not found"})
		return
	}

	config.RespondJSON(w, http.StatusOK, sample)
}

// GetLabSampleByNumber retrieves a sample by number
func (l *LaboratoryHandler) GetLabSampleByNumber(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")

	sample, err := l.h.Queries.GetLabSampleByNumber(r.Context(), number)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Sample not found"})
		return
	}

	config.RespondJSON(w, http.StatusOK, sample)
}

// ListLabSamples lists all lab samples
func (l *LaboratoryHandler) ListLabSamples(w http.ResponseWriter, r *http.Request) {
	paginationParams := middlewares.GetPagination(r.Context())

	samples, err := l.h.Queries.ListLabSamples(r.Context(), db.ListLabSamplesParams{
		Limit:  int32(paginationParams.Limit),
		Offset: int32(paginationParams.Offset),
	})

	if err != nil {
		l.h.Logger.Error("Failed to list lab samples", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to retrieve samples"})
		return
	}

	config.RespondJSON(w, http.StatusOK, samples)
}

// ListLabSamplesByStatus lists samples by status
func (l *LaboratoryHandler) ListLabSamplesByStatus(w http.ResponseWriter, r *http.Request) {
	status := r.PathValue("status")
	paginationParams := middlewares.GetPagination(r.Context())

	samples, err := l.h.Queries.ListLabSamplesByStatus(r.Context(), db.ListLabSamplesByStatusParams{
		SampleStatus: db.NullLabSampleStatus{LabSampleStatus: db.LabSampleStatus(status), Valid: true},
		Limit:        int32(paginationParams.Limit),
		Offset:       int32(paginationParams.Offset),
	})

	if err != nil {
		l.h.Logger.Error("Failed to list samples by status", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to retrieve samples"})
		return
	}

	config.RespondJSON(w, http.StatusOK, samples)
}

// ListLabSamplesByMaterial lists samples by material
func (l *LaboratoryHandler) ListLabSamplesByMaterial(w http.ResponseWriter, r *http.Request) {
	materialIDStr := r.PathValue("material_id")
	materialID, err := strconv.Atoi(materialIDStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid material ID"})
		return
	}

	samples, err := l.h.Queries.ListLabSamplesByMaterial(r.Context(), pgtype.Int4{
		Int32: int32(materialID),
		Valid: true,
	})
	if err != nil {
		l.h.Logger.Error("Failed to list samples by material", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to retrieve samples"})
		return
	}

	config.RespondJSON(w, http.StatusOK, samples)
}

// UpdateLabSample updates a lab sample
func (l *LaboratoryHandler) UpdateLabSample(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
		return
	}

	var req struct {
		SampleStatus      string `json:"sample_status"`
		ReceivedDate      string `json:"received_date"`
		ReceivedBy        int32  `json:"received_by"`
		StorageLocation   string `json:"storage_location"`
		StorageConditions string `json:"storage_conditions"`
		DisposalDate      string `json:"disposal_date"`
		Notes             string `json:"notes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
		return
	}

	sample, err := l.h.Queries.UpdateLabSample(r.Context(), db.UpdateLabSampleParams{
		ID:              int32(id),
		SampleStatus:    db.NullLabSampleStatus{LabSampleStatus: db.LabSampleStatus(req.SampleStatus), Valid: req.SampleStatus != ""},
		ReceivedByLab:   int32Ptr(req.ReceivedBy),
		LabReceivedDate: parseFlexibleTimestamp(req.ReceivedDate),
		StorageLocation: textPtr(req.StorageLocation),
		DisposedDate:    parseFlexibleDate(req.DisposalDate),
		DisposedBy:      pgtype.Int4{},
		DisposalMethod:  pgtype.Text{},
		Notes:           textPtr(req.Notes),
	})

	if err != nil {
		l.h.Logger.Error("Failed to update sample", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update sample"})
		return
	}

	config.RespondJSON(w, http.StatusOK, sample)
}

// DeleteLabSample deletes a lab sample
func (l *LaboratoryHandler) DeleteLabSample(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
		return
	}

	err = l.h.Queries.DeleteLabSample(r.Context(), int32(id))
	if err != nil {
		l.h.Logger.Error("Failed to delete sample", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete sample"})
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]string{"message": "Sample deleted successfully"})
}
