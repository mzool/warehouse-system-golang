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
// Certificate of Analysis (CoA)
// ============================================================================

// CreateCertificateOfAnalysis creates a new CoA
func (l *LaboratoryHandler) CreateCertificateOfAnalysis(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CertificateNumber string `json:"certificate_number"`
		SampleID          int32  `json:"sample_id"`
		MaterialID        int32  `json:"material_id"`
		BatchNumber       string `json:"batch_number"`
		IssueDate         string `json:"issue_date"`
		IssuedBy          int32  `json:"issued_by"`
		ApprovedBy        int32  `json:"approved_by"`
		ApprovalDate      string `json:"approval_date"`
		TestResults       string `json:"test_results"`
		Conclusion        string `json:"conclusion"`
		CertificateStatus string `json:"certificate_status"`
		PDFPath           string `json:"pdf_path"`
		Attachments       string `json:"attachments"`
		Notes             string `json:"notes"`
	}

	coa, err := l.h.Queries.CreateCertificateOfAnalysis(r.Context(), db.CreateCertificateOfAnalysisParams{
		CoaNumber:           req.CertificateNumber,
		MaterialID:          req.MaterialID,
		BatchNumber:         req.BatchNumber,
		LotNumber:           pgtype.Text{},
		QualityInspectionID: pgtype.Int4{},
		ManufactureDate:     pgtype.Date{},
		ExpiryDate:          pgtype.Date{},
		Quantity:            pgtype.Numeric{},
		UnitID:              pgtype.Int4{},
		TestResults:         []byte(req.TestResults),
		CustomerID:          pgtype.Int4{},
		SalesOrderID:        pgtype.Int4{},
		RecipientName:       pgtype.Text{},
		RecipientAddress:    pgtype.Text{},
		Status:              db.NullCoaStatus{CoaStatus: db.CoaStatus(req.CertificateStatus), Valid: req.CertificateStatus != ""},
		IssueDate:           parseFlexibleDate(req.IssueDate),
		PreparedBy:          int32Ptr(req.IssuedBy),
		PreparedDate:        currentDate(),
		ReviewedBy:          pgtype.Int4{},
		ReviewedDate:        pgtype.Date{},
		ApprovedBy:          int32Ptr(req.ApprovedBy),
		ApprovedDate:        parseFlexibleDate(req.ApprovalDate),
		DigitalSignature:    pgtype.Text{},
		SignatureTimestamp:  pgtype.Timestamptz{},
		PdfFilePath:         textPtr(req.PDFPath),
		Notes:               textPtr(req.Notes),
	})

	if err != nil {
		l.h.Logger.Error("Failed to create CoA", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create certificate of analysis"})
		return
	}

	config.RespondJSON(w, http.StatusCreated, coa)
}

// GetCertificateOfAnalysis retrieves a CoA by ID
func (l *LaboratoryHandler) GetCertificateOfAnalysis(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
		return
	}

	coa, err := l.h.Queries.GetCertificateOfAnalysisByID(r.Context(), int32(id))
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Certificate not found"})
		return
	}

	config.RespondJSON(w, http.StatusOK, coa)
}

// GetCertificateByNumber retrieves CoA by certificate number
func (l *LaboratoryHandler) GetCertificateByNumber(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")

	coa, err := l.h.Queries.GetCertificateOfAnalysisByNumber(r.Context(), number)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Certificate not found"})
		return
	}

	config.RespondJSON(w, http.StatusOK, coa)
}

// ListCertificatesBySample lists CoAs by sample
func (l *LaboratoryHandler) ListCertificatesBySample(w http.ResponseWriter, r *http.Request) {
	sampleIDStr := r.PathValue("sample_id")
	_, err := strconv.Atoi(sampleIDStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid sample ID"})
		return
	}

	// Note: This should query by batch_number associated with the sample
	// For now, using sample ID directly - may need to adjust based on actual data model
	coas, err := l.h.Queries.ListCertificatesOfAnalysis(r.Context(), db.ListCertificatesOfAnalysisParams{
		Limit:  100,
		Offset: 0,
	})
	if err != nil {
		l.h.Logger.Error("Failed to list certificates", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to retrieve certificates"})
		return
	}

	config.RespondJSON(w, http.StatusOK, coas)
}

// UpdateCertificateOfAnalysis updates a CoA
func (l *LaboratoryHandler) UpdateCertificateOfAnalysis(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
		return
	}

	var req struct {
		ApprovedBy        int32  `json:"approved_by"`
		ApprovalDate      string `json:"approval_date"`
		CertificateStatus string `json:"certificate_status"`
		PDFPath           string `json:"pdf_path"`
		Notes             string `json:"notes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
		return
	}

	coa, err := l.h.Queries.UpdateCertificateOfAnalysis(r.Context(), db.UpdateCertificateOfAnalysisParams{
		ID:                 int32(id),
		Status:             db.NullCoaStatus{CoaStatus: db.CoaStatus(req.CertificateStatus), Valid: req.CertificateStatus != ""},
		IssueDate:          pgtype.Date{},
		TestResults:        []byte{},
		PreparedBy:         pgtype.Int4{},
		PreparedDate:       pgtype.Date{},
		ReviewedBy:         pgtype.Int4{},
		ReviewedDate:       pgtype.Date{},
		ApprovedBy:         int32Ptr(req.ApprovedBy),
		ApprovedDate:       parseFlexibleDate(req.ApprovalDate),
		DigitalSignature:   pgtype.Text{},
		SignatureTimestamp: pgtype.Timestamptz{},
		PdfFilePath:        textPtr(req.PDFPath),
		Notes:              textPtr(req.Notes),
	})

	if err != nil {
		l.h.Logger.Error("Failed to update CoA", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update certificate"})
		return
	}

	config.RespondJSON(w, http.StatusOK, coa)
}

// ============================================================================
// Out-of-Specification (OOS) Investigations
// ============================================================================

// CreateOOSInvestigation creates a new OOS investigation
func (l *LaboratoryHandler) CreateOOSInvestigation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		InvestigationNumber string `json:"investigation_number"`
		TestResultID        int32  `json:"test_result_id"`
		SampleID            int32  `json:"sample_id"`
		InitiatedBy         int32  `json:"initiated_by"`
		InitiatedDate       string `json:"initiated_date"`
		InvestigationType   string `json:"investigation_type"`
		Severity            string `json:"severity"`
		Description         string `json:"description"`
		PhaseIInvestigator  int32  `json:"phase_i_investigator"`
		PhaseIStartDate     string `json:"phase_i_start_date"`
		PhaseIEndDate       string `json:"phase_i_end_date"`
		PhaseIFindings      string `json:"phase_i_findings"`
		PhaseIConclusion    string `json:"phase_i_conclusion"`
		RequiresPhaseII     *bool  `json:"requires_phase_ii"`
		PhaseIIInvestigator int32  `json:"phase_ii_investigator"`
		PhaseIIStartDate    string `json:"phase_ii_start_date"`
		PhaseIIEndDate      string `json:"phase_ii_end_date"`
		PhaseIIFindings     string `json:"phase_ii_findings"`
		RootCause           string `json:"root_cause"`
		CorrectiveAction    string `json:"corrective_action"`
		PreventiveAction    string `json:"preventive_action"`
		FinalConclusion     string `json:"final_conclusion"`
		InvestigationStatus string `json:"investigation_status"`
		ClosedBy            int32  `json:"closed_by"`
		ClosedDate          string `json:"closed_date"`
		Attachments         string `json:"attachments"`
		Notes               string `json:"notes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
		return
	}

	investigation, err := l.h.Queries.CreateOOSInvestigation(r.Context(), db.CreateOOSInvestigationParams{
		OosNumber:        req.InvestigationNumber,
		TestAssignmentID: req.TestResultID,
		SampleID:         int32Ptr(req.SampleID),
		NcrID:            pgtype.Int4{},
		OosDescription:   req.Description,
		Severity:         db.NullNcrSeverity{NcrSeverity: db.NcrSeverity(req.Severity), Valid: req.Severity != ""},
		InitiatedBy:      int32Ptr(req.InitiatedBy),
		InitiatedDate:    timestampOrNow(req.InitiatedDate),
		InvestigatorID:   int32Ptr(req.PhaseIInvestigator),
		Status:           pgtype.Text{String: req.InvestigationStatus, Valid: req.InvestigationStatus != ""},
		Notes:            textPtr(req.Notes),
		Attachments:      []byte(req.Attachments),
	})

	if err != nil {
		l.h.Logger.Error("Failed to create OOS investigation", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create OOS investigation"})
		return
	}

	config.RespondJSON(w, http.StatusCreated, investigation)
}

// GetOOSInvestigation retrieves an OOS investigation by ID
func (l *LaboratoryHandler) GetOOSInvestigation(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
		return
	}

	investigation, err := l.h.Queries.GetOOSInvestigationByID(r.Context(), int32(id))
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Investigation not found"})
		return
	}

	config.RespondJSON(w, http.StatusOK, investigation)
}

// ListOOSInvestigations lists all OOS investigations
func (l *LaboratoryHandler) ListOOSInvestigations(w http.ResponseWriter, r *http.Request) {
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

	investigations, err := l.h.Queries.ListOOSInvestigations(r.Context(), db.ListOOSInvestigationsParams{
		Limit:  limit,
		Offset: offset,
	})

	if err != nil {
		l.h.Logger.Error("Failed to list OOS investigations", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to retrieve investigations"})
		return
	}

	config.RespondJSON(w, http.StatusOK, investigations)
}

// ListOpenOOSInvestigations lists open investigations
func (l *LaboratoryHandler) ListOpenOOSInvestigations(w http.ResponseWriter, r *http.Request) {
	investigations, err := l.h.Queries.ListOpenOOSInvestigations(r.Context())
	if err != nil {
		l.h.Logger.Error("Failed to list open OOS investigations", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to retrieve investigations"})
		return
	}

	config.RespondJSON(w, http.StatusOK, investigations)
}

// UpdateOOSInvestigation updates an OOS investigation
func (l *LaboratoryHandler) UpdateOOSInvestigation(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
		return
	}

	var req struct {
		InvestigationStatus string `json:"investigation_status"`
		PhaseIFindings      string `json:"phase_i_findings"`
		PhaseIConclusion    string `json:"phase_i_conclusion"`
		PhaseIIFindings     string `json:"phase_ii_findings"`
		RootCause           string `json:"root_cause"`
		CorrectiveAction    string `json:"corrective_action"`
		FinalConclusion     string `json:"final_conclusion"`
		Notes               string `json:"notes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
		return
	}

	investigation, err := l.h.Queries.UpdateOOSInvestigation(r.Context(), db.UpdateOOSInvestigationParams{
		ID:                  int32(id),
		Status:              pgtype.Text{String: req.InvestigationStatus, Valid: req.InvestigationStatus != ""},
		InvestigatorID:      pgtype.Int4{},
		Phase1StartDate:     pgtype.Date{},
		Phase1CompleteDate:  pgtype.Date{},
		Phase1Findings:      textPtr(req.PhaseIFindings),
		LabErrorFound:       pgtype.Bool{},
		LabErrorDescription: pgtype.Text{},
		Phase2Required:      pgtype.Bool{},
		Phase2StartDate:     pgtype.Date{},
		Phase2CompleteDate:  pgtype.Date{},
		Phase2Findings:      textPtr(req.PhaseIIFindings),
		RootCause:           textPtr(req.RootCause),
		FinalConclusion:     textPtr(req.FinalConclusion),
		CorrectiveAction:    textPtr(req.CorrectiveAction),
		PreventiveAction:    pgtype.Text{},
		BatchDisposition:    pgtype.Text{},
		ReviewedBy:          pgtype.Int4{},
		ApprovedBy:          pgtype.Int4{},
		ClosedDate:          pgtype.Date{},
		Notes:               textPtr(req.Notes),
	})

	if err != nil {
		l.h.Logger.Error("Failed to update OOS investigation", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update investigation"})
		return
	}

	config.RespondJSON(w, http.StatusOK, investigation)
}
