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
// Lab Test Assignments
// ============================================================================

// CreateLabTestAssignment creates a new test assignment
func (l *LaboratoryHandler) CreateLabTestAssignment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SampleID     int32  `json:"sample_id"`
		MethodID     int32  `json:"method_id"`
		AssignedTo   int32  `json:"assigned_to"`
		AssignedDate string `json:"assigned_date"`
		Priority     int32  `json:"priority"`
		DueDate      string `json:"due_date"`
		Notes        string `json:"notes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
		return
	}

	assignment, err := l.h.Queries.CreateLabTestAssignment(r.Context(), db.CreateLabTestAssignmentParams{
		SampleID:      req.SampleID,
		TestMethodID:  req.MethodID,
		Priority:      int32Ptr(req.Priority),
		RequestedDate: pgtype.Date{},
		ScheduledDate: pgtype.Date{},
		DueDate:       parseFlexibleDate(req.DueDate),
		AssignedTo:    int32Ptr(req.AssignedTo),
		AssignedDate:  timestampOrNow(req.AssignedDate),
		Status:        db.NullTestResultStatus{},
		IsRush:        pgtype.Bool{},
		Notes:         textPtr(req.Notes),
	})

	if err != nil {
		l.h.Logger.Error("Failed to create test assignment", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create test assignment"})
		return
	}

	config.RespondJSON(w, http.StatusCreated, assignment)
}

// GetLabTestAssignment retrieves a test assignment by ID
func (l *LaboratoryHandler) GetLabTestAssignment(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
		return
	}

	assignment, err := l.h.Queries.GetLabTestAssignmentByID(r.Context(), int32(id))
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Test assignment not found"})
		return
	}

	config.RespondJSON(w, http.StatusOK, assignment)
}

// ListLabTestAssignmentsBySample lists assignments by sample
func (l *LaboratoryHandler) ListLabTestAssignmentsBySample(w http.ResponseWriter, r *http.Request) {
	sampleIDStr := r.PathValue("sample_id")
	sampleID, err := strconv.Atoi(sampleIDStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid sample ID"})
		return
	}

	assignments, err := l.h.Queries.ListLabTestAssignmentsBySample(r.Context(), int32(sampleID))
	if err != nil {
		l.h.Logger.Error("Failed to list test assignments", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to retrieve test assignments"})
		return
	}

	config.RespondJSON(w, http.StatusOK, assignments)
}

// ListLabTestAssignmentsByAnalyst lists assignments by analyst
func (l *LaboratoryHandler) ListLabTestAssignmentsByAnalyst(w http.ResponseWriter, r *http.Request) {
	analystIDStr := r.PathValue("analyst_id")
	analystID, err := strconv.Atoi(analystIDStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid analyst ID"})
		return
	}

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

	assignments, err := l.h.Queries.ListLabTestAssignmentsByAnalyst(r.Context(), db.ListLabTestAssignmentsByAnalystParams{
		AssignedTo: pgtype.Int4{Int32: int32(analystID), Valid: true},
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		l.h.Logger.Error("Failed to list assignments by analyst", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to retrieve assignments"})
		return
	}

	config.RespondJSON(w, http.StatusOK, assignments)
}

// ListPendingTestAssignments lists pending assignments
func (l *LaboratoryHandler) ListPendingTestAssignments(w http.ResponseWriter, r *http.Request) {
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

	assignments, err := l.h.Queries.ListPendingLabTestAssignments(r.Context(), db.ListPendingLabTestAssignmentsParams{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		l.h.Logger.Error("Failed to list pending assignments", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to retrieve assignments"})
		return
	}

	config.RespondJSON(w, http.StatusOK, assignments)
}

// UpdateLabTestAssignment updates a test assignment
func (l *LaboratoryHandler) UpdateLabTestAssignment(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
		return
	}

	var req struct {
		Status        string `json:"status"`
		StartedDate   string `json:"started_date"`
		CompletedDate string `json:"completed_date"`
		ReviewNotes   string `json:"review_notes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
		return
	}

	assignment, err := l.h.Queries.UpdateLabTestAssignment(r.Context(), db.UpdateLabTestAssignmentParams{
		ID:            int32(id),
		Status:        db.NullTestResultStatus{},
		AssignedTo:    pgtype.Int4{},
		AssignedDate:  pgtype.Timestamptz{},
		ScheduledDate: pgtype.Date{},
		DueDate:       pgtype.Date{},
		StartedDate:   parseFlexibleTimestamp(req.StartedDate),
		CompletedDate: parseFlexibleTimestamp(req.CompletedDate),
		ResultValue:   pgtype.Numeric{},
		ResultText:    pgtype.Text{},
		ResultUnitID:  pgtype.Int4{},
		PassFail:      pgtype.Bool{},
		ReviewedBy:    pgtype.Int4{},
		ReviewDate:    pgtype.Timestamptz{},
		ReviewNotes:   textPtr(req.ReviewNotes),
	})

	if err != nil {
		l.h.Logger.Error("Failed to update test assignment", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update test assignment"})
		return
	}

	config.RespondJSON(w, http.StatusOK, assignment)
}

// ============================================================================
// Lab Test Results
// ============================================================================

// CreateLabTestResult creates a new test result
func (l *LaboratoryHandler) CreateLabTestResult(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AssignmentID  int32  `json:"assignment_id"`
		TestParameter string `json:"test_parameter"`
		ResultText    string `json:"result_text"`
		Attachments   string `json:"attachments"`
		Notes         string `json:"notes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
		return
	}

	result, err := l.h.Queries.CreateLabTestResult(r.Context(), db.CreateLabTestResultParams{
		TestAssignmentID:       req.AssignmentID,
		TestDate:               currentTimestamp(),
		AnalystID:              pgtype.Int4{},
		EquipmentID:            pgtype.Int4{},
		ParameterName:          req.TestParameter,
		ResultValue:            pgtype.Numeric{},
		ResultText:             textPtr(req.ResultText),
		ResultUnitID:           pgtype.Int4{},
		SpecificationMin:       pgtype.Numeric{},
		SpecificationMax:       pgtype.Numeric{},
		SpecificationTarget:    pgtype.Numeric{},
		IsInSpec:               pgtype.Bool{},
		Deviation:              pgtype.Numeric{},
		ReplicateNumber:        pgtype.Int4{},
		DilutionFactor:         pgtype.Numeric{},
		PreparationDetails:     pgtype.Text{},
		TestTemperature:        pgtype.Numeric{},
		TestHumidity:           pgtype.Numeric{},
		TestConditions:         []byte{},
		SystemSuitabilityPass:  pgtype.Bool{},
		BlankValue:             pgtype.Numeric{},
		ReferenceStandardValue: pgtype.Numeric{},
		RawDataFile:            pgtype.Text{},
		ChromatogramFile:       pgtype.Text{},
		Attachments:            []byte(req.Attachments),
		Notes:                  textPtr(req.Notes),
	})

	if err != nil {
		l.h.Logger.Error("Failed to create test result", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create test result"})
		return
	}

	config.RespondJSON(w, http.StatusCreated, result)
}

// GetLabTestResult retrieves a test result by ID
func (l *LaboratoryHandler) GetLabTestResult(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
		return
	}

	result, err := l.h.Queries.GetLabTestResultByID(r.Context(), int32(id))
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Test result not found"})
		return
	}

	config.RespondJSON(w, http.StatusOK, result)
}

// ListLabTestResultsByAssignment lists results by assignment
func (l *LaboratoryHandler) ListLabTestResultsByAssignment(w http.ResponseWriter, r *http.Request) {
	assignmentIDStr := r.PathValue("assignment_id")
	assignmentID, err := strconv.Atoi(assignmentIDStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid assignment ID"})
		return
	}

	results, err := l.h.Queries.ListLabTestResults(r.Context(), int32(assignmentID))
	if err != nil {
		l.h.Logger.Error("Failed to list test results", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to retrieve test results"})
		return
	}

	config.RespondJSON(w, http.StatusOK, results)
}

// ListLabTestResultsBySample lists results by sample
// Note: This requires joining through assignments - query not yet implemented
func (l *LaboratoryHandler) ListLabTestResultsBySample(w http.ResponseWriter, r *http.Request) {
	sampleIDStr := r.PathValue("sample_id")
	sampleID, err := strconv.Atoi(sampleIDStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid sample ID"})
		return
	}

	// TODO: Implement query to get results by sample ID
	// Need to join: lab_test_results -> lab_test_assignments -> lab_samples
	l.h.Logger.Warn("ListLabTestResultsBySample not yet implemented", "sample_id", sampleID)
	config.RespondJSON(w, http.StatusNotImplemented, map[string]string{"error": "Feature not yet implemented"})
}

// UpdateLabTestResult updates a test result
func (l *LaboratoryHandler) UpdateLabTestResult(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
		return
	}

	var req struct {
		Notes string `json:"notes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
		return
	}

	// Note: UpdateLabTestResult query doesn't exist in SQLC - this will fail at runtime
	// TODO: Add update query to laboratory.sql or remove this handler
	l.h.Logger.Warn("UpdateLabTestResult query not implemented in SQLC", "id", id)
	config.RespondJSON(w, http.StatusNotImplemented, map[string]string{"error": "Update operation not yet implemented"})

	// Placeholder for when query is implemented:
	/*
		result, err := l.h.Queries.UpdateLabTestResult(r.Context(), db.UpdateLabTestResultParams{
			ID:    int32(id),
			Notes: pgtype.Text{String: req.Notes, Valid: req.Notes != ""},
		})

		if err != nil {
			l.h.Logger.Error("Failed to update test result", "error", err)
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update test result"})
			return
		}

		config.RespondJSON(w, http.StatusOK, result)
	*/
}
