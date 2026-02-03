package laboratory

import (
	"encoding/json"
	"net/http"
	"strconv"
	"warehouse_system/internal/config"
	db "warehouse_system/internal/database/db"

	"github.com/jackc/pgx/v5/pgtype"
)

// ============================================================================
// Test Methods
// ============================================================================

// CreateLabTestMethod creates a new lab test method
func (l *LaboratoryHandler) CreateLabTestMethod(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MethodCode           string  `json:"method_code"`
		MethodName           string  `json:"method_name"`
		Description          string  `json:"description"`
		StandardReference    string  `json:"standard_reference"`
		StandardOrganization string  `json:"standard_organization"`
		TestType             string  `json:"test_type"`
		TestCategory         string  `json:"test_category"`
		Methodology          string  `json:"methodology"`
		SampleSize           float64 `json:"sample_size"`
		SampleUnitID         int32   `json:"sample_unit_id"`
		PreparationTime      int32   `json:"preparation_time"`
		TestDuration         int32   `json:"test_duration"`
		RequiredEquipment    string  `json:"required_equipment"`
		SpecificationLimits  string  `json:"specification_limits"`
		Version              string  `json:"version"`
		EffectiveDate        string  `json:"effective_date"`
		SupersedesMethodID   int32   `json:"supersedes_method_id"`
		Status               string  `json:"status"`
		ApprovedBy           int32   `json:"approved_by"`
		ApprovalDate         string  `json:"approval_date"`
		Attachments          string  `json:"attachments"`
		Notes                string  `json:"notes"`
		IsActive             bool    `json:"is_active"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
		return
	}

	method, err := l.h.Queries.CreateLabTestMethod(r.Context(), db.CreateLabTestMethodParams{
		MethodCode:           req.MethodCode,
		MethodName:           req.MethodName,
		Description:          textPtr(req.Description),
		StandardReference:    textPtr(req.StandardReference),
		StandardOrganization: textPtr(req.StandardOrganization),
		TestType:             req.TestType,
		TestCategory:         textPtr(req.TestCategory),
		Methodology:          textPtr(req.Methodology),
		SampleSize:           floatToNumericPrecision(req.SampleSize, 4),
		SampleUnitID:         int32Ptr(req.SampleUnitID),
		PreparationTime:      int32Ptr(req.PreparationTime),
		TestDuration:         int32Ptr(req.TestDuration),
		RequiredEquipment:    []byte(req.RequiredEquipment),
		SpecificationLimits:  []byte(req.SpecificationLimits),
		Version:              textPtr(req.Version),
		EffectiveDate:        parseFlexibleDate(req.EffectiveDate),
		SupersedesMethodID:   int32Ptr(req.SupersedesMethodID),
		Status:               db.NullTestMethodStatus{TestMethodStatus: db.TestMethodStatus(req.Status), Valid: req.Status != ""},
		ApprovedBy:           int32Ptr(req.ApprovedBy),
		ApprovalDate:         parseFlexibleDate(req.ApprovalDate),
		Attachments:          []byte(req.Attachments),
		Notes:                textPtr(req.Notes),
		IsActive:             pgtype.Bool{Bool: req.IsActive, Valid: true},
	})

	if err != nil {
		l.h.Logger.Error("Failed to create lab test method", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create lab test method"})
		return
	}

	config.RespondJSON(w, http.StatusCreated, method)
}

// GetLabTestMethod retrieves a lab test method by ID
func (l *LaboratoryHandler) GetLabTestMethod(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
		return
	}

	method, err := l.h.Queries.GetLabTestMethodByID(r.Context(), int32(id))
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Test method not found"})
		return
	}

	config.RespondJSON(w, http.StatusOK, method)
}

// GetLabTestMethodByCode retrieves a lab test method by code
func (l *LaboratoryHandler) GetLabTestMethodByCode(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")

	method, err := l.h.Queries.GetLabTestMethodByCode(r.Context(), code)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Test method not found"})
		return
	}

	config.RespondJSON(w, http.StatusOK, method)
}

// ListLabTestMethods lists all lab test methods
func (l *LaboratoryHandler) ListLabTestMethods(w http.ResponseWriter, r *http.Request) {
	// Get pagination parameters
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")
	var limit, offset int32 = 20, 0
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = int32(l)
	}
	if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
		offset = int32(o)
	}

	methods, err := l.h.Queries.ListLabTestMethods(r.Context(), db.ListLabTestMethodsParams{
		Limit:  limit,
		Offset: offset,
	})

	if err != nil {
		l.h.Logger.Error("Failed to list lab test methods", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to retrieve test methods"})
		return
	}

	config.RespondJSON(w, http.StatusOK, methods)
}

// ListLabTestMethodsByType lists test methods by type
func (l *LaboratoryHandler) ListLabTestMethodsByType(w http.ResponseWriter, r *http.Request) {
	testType := r.PathValue("type")

	methods, err := l.h.Queries.ListLabTestMethodsByType(r.Context(), testType)
	if err != nil {
		l.h.Logger.Error("Failed to list test methods by type", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to retrieve test methods"})
		return
	}

	config.RespondJSON(w, http.StatusOK, methods)
}

// SearchLabTestMethods searches test methods
func (l *LaboratoryHandler) SearchLabTestMethods(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	// Get pagination parameters
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")
	var limit, offset int32 = 20, 0
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = int32(l)
	}
	if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
		offset = int32(o)
	}

	methods, err := l.h.Queries.SearchLabTestMethods(r.Context(), db.SearchLabTestMethodsParams{
		Column1: pgtype.Text{String: query, Valid: query != ""},
		Limit:   limit,
		Offset:  offset,
	})

	if err != nil {
		l.h.Logger.Error("Failed to search test methods", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to search test methods"})
		return
	}

	config.RespondJSON(w, http.StatusOK, methods)
}

// UpdateLabTestMethod updates a lab test method
func (l *LaboratoryHandler) UpdateLabTestMethod(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
		return
	}

	var req struct {
		MethodName        string `json:"method_name"`
		Description       string `json:"description"`
		StandardReference string `json:"standard_reference"`
		Methodology       string `json:"methodology"`
		Status            string `json:"status"`
		ApprovedBy        int32  `json:"approved_by"`
		ApprovalDate      string `json:"approval_date"`
		IsActive          *bool  `json:"is_active"`
		Notes             string `json:"notes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
		return
	}

	method, err := l.h.Queries.UpdateLabTestMethod(r.Context(), db.UpdateLabTestMethodParams{
		ID:                int32(id),
		MethodName:        textPtr(req.MethodName),
		Description:       textPtr(req.Description),
		StandardReference: textPtr(req.StandardReference),
		Methodology:       textPtr(req.Methodology),
		Status:            db.NullTestMethodStatus{TestMethodStatus: db.TestMethodStatus(req.Status), Valid: req.Status != ""},
		ApprovedBy:        int32Ptr(req.ApprovedBy),
		ApprovalDate:      parseFlexibleDate(req.ApprovalDate),
		IsActive:          boolPtr(req.IsActive),
		Notes:             textPtr(req.Notes),
	})

	if err != nil {
		l.h.Logger.Error("Failed to update test method", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update test method"})
		return
	}

	config.RespondJSON(w, http.StatusOK, method)
}

// DeleteLabTestMethod deletes a lab test method
func (l *LaboratoryHandler) DeleteLabTestMethod(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
		return
	}

	err = l.h.Queries.DeleteLabTestMethod(r.Context(), int32(id))
	if err != nil {
		l.h.Logger.Error("Failed to delete test method", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete test method"})
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]string{"message": "Test method deleted successfully"})
}
