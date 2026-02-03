package laboratory

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
	"warehouse_system/internal/config"
	db "warehouse_system/internal/database/db"

	"github.com/jackc/pgx/v5/pgtype"
)

// ============================================================================
// Lab Equipment
// ============================================================================

// CreateLabEquipment creates a new lab equipment
func (l *LaboratoryHandler) CreateLabEquipment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EquipmentCode            string `json:"equipment_code"`
		EquipmentName            string `json:"equipment_name"`
		EquipmentType            string `json:"equipment_type"`
		Manufacturer             string `json:"manufacturer"`
		ModelNumber              string `json:"model_number"`
		SerialNumber             string `json:"serial_number"`
		Location                 string `json:"location"`
		WarehouseID              int32  `json:"warehouse_id"`
		CalibrationFrequencyDays int32  `json:"calibration_frequency_days"`
		LastCalibrationDate      string `json:"last_calibration_date"`
		NextCalibrationDate      string `json:"next_calibration_date"`
		CalibrationStatus        string `json:"calibration_status"`
		CalibrationCertificate   string `json:"calibration_certificate"`
		LastMaintenanceDate      string `json:"last_maintenance_date"`
		NextMaintenanceDate      string `json:"next_maintenance_date"`
		MaintenanceNotes         string `json:"maintenance_notes"`
		IsOperational            bool   `json:"is_operational"`
		IsQualified              bool   `json:"is_qualified"`
		QualificationDate        string `json:"qualification_date"`
		Attachments              string `json:"attachments"`
		Notes                    string `json:"notes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
		return
	}

	equipment, err := l.h.Queries.CreateLabEquipment(r.Context(), db.CreateLabEquipmentParams{
		EquipmentCode:            req.EquipmentCode,
		EquipmentName:            req.EquipmentName,
		EquipmentType:            req.EquipmentType,
		Manufacturer:             textPtr(req.Manufacturer),
		ModelNumber:              textPtr(req.ModelNumber),
		SerialNumber:             textPtr(req.SerialNumber),
		Location:                 textPtr(req.Location),
		WarehouseID:              int32Ptr(req.WarehouseID),
		CalibrationFrequencyDays: int32Ptr(req.CalibrationFrequencyDays),
		LastCalibrationDate:      parseFlexibleDate(req.LastCalibrationDate),
		NextCalibrationDate:      parseFlexibleDate(req.NextCalibrationDate),
		CalibrationStatus:        db.NullCalibrationStatus{CalibrationStatus: db.CalibrationStatus(req.CalibrationStatus), Valid: req.CalibrationStatus != ""},
		CalibrationCertificate:   textPtr(req.CalibrationCertificate),
		LastMaintenanceDate:      parseFlexibleDate(req.LastMaintenanceDate),
		NextMaintenanceDate:      parseFlexibleDate(req.NextMaintenanceDate),
		MaintenanceNotes:         textPtr(req.MaintenanceNotes),
		IsOperational:            pgtype.Bool{Bool: req.IsOperational, Valid: true},
		IsQualified:              pgtype.Bool{Bool: req.IsQualified, Valid: true},
		QualificationDate:        parseFlexibleDate(req.QualificationDate),
		Attachments:              []byte(req.Attachments),
		Notes:                    textPtr(req.Notes),
	})

	if err != nil {
		l.h.Logger.Error("Failed to create lab equipment", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create lab equipment"})
		return
	}

	config.RespondJSON(w, http.StatusCreated, equipment)
}

// GetLabEquipment retrieves lab equipment by ID
func (l *LaboratoryHandler) GetLabEquipment(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
		return
	}

	equipment, err := l.h.Queries.GetLabEquipmentByID(r.Context(), int32(id))
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Equipment not found"})
		return
	}

	config.RespondJSON(w, http.StatusOK, equipment)
}

// GetLabEquipmentByCode retrieves equipment by code
func (l *LaboratoryHandler) GetLabEquipmentByCode(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")

	equipment, err := l.h.Queries.GetLabEquipmentByCode(r.Context(), code)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Equipment not found"})
		return
	}

	config.RespondJSON(w, http.StatusOK, equipment)
}

// ListLabEquipment lists all lab equipment
func (l *LaboratoryHandler) ListLabEquipment(w http.ResponseWriter, r *http.Request) {
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

	equipment, err := l.h.Queries.ListLabEquipment(r.Context(), db.ListLabEquipmentParams{
		Limit:  limit,
		Offset: offset,
	})

	if err != nil {
		l.h.Logger.Error("Failed to list lab equipment", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to retrieve equipment"})
		return
	}

	config.RespondJSON(w, http.StatusOK, equipment)
}

// ListLabEquipmentByType lists equipment by type
func (l *LaboratoryHandler) ListLabEquipmentByType(w http.ResponseWriter, r *http.Request) {
	equipmentType := r.PathValue("type")

	equipment, err := l.h.Queries.ListLabEquipmentByType(r.Context(), equipmentType)
	if err != nil {
		l.h.Logger.Error("Failed to list equipment by type", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to retrieve equipment"})
		return
	}

	config.RespondJSON(w, http.StatusOK, equipment)
}

// ListCalibrationDue lists equipment with calibration due
func (l *LaboratoryHandler) ListCalibrationDue(w http.ResponseWriter, r *http.Request) {
	daysStr := r.URL.Query().Get("days")
	days := int32(30) // default 30 days
	if daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil {
			days = int32(d)
		}
	}

	// Calculate future date
	futureDate := time.Now().AddDate(0, 0, int(days))
	equipment, err := l.h.Queries.ListLabEquipmentCalibrationDue(r.Context(), pgtype.Date{
		Time:  futureDate,
		Valid: true,
	})
	if err != nil {
		l.h.Logger.Error("Failed to list calibration due", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to retrieve equipment"})
		return
	}

	config.RespondJSON(w, http.StatusOK, equipment)
}

// UpdateLabEquipment updates lab equipment
func (l *LaboratoryHandler) UpdateLabEquipment(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
		return
	}

	var req struct {
		EquipmentName          string `json:"equipment_name"`
		Location               string `json:"location"`
		LastCalibrationDate    string `json:"last_calibration_date"`
		NextCalibrationDate    string `json:"next_calibration_date"`
		CalibrationStatus      string `json:"calibration_status"`
		CalibrationCertificate string `json:"calibration_certificate"`
		IsOperational          *bool  `json:"is_operational"`
		Notes                  string `json:"notes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
		return
	}

	equipment, err := l.h.Queries.UpdateLabEquipment(r.Context(), db.UpdateLabEquipmentParams{
		ID:                     int32(id),
		EquipmentName:          textPtr(req.EquipmentName),
		Location:               textPtr(req.Location),
		LastCalibrationDate:    parseFlexibleDate(req.LastCalibrationDate),
		NextCalibrationDate:    parseFlexibleDate(req.NextCalibrationDate),
		CalibrationCertificate: textPtr(req.CalibrationCertificate),
		LastMaintenanceDate:    pgtype.Date{Valid: false},
		NextMaintenanceDate:    pgtype.Date{Valid: false},
		MaintenanceNotes:       pgtype.Text{Valid: false},
		IsOperational:          boolPtr(req.IsOperational),
		IsQualified:            pgtype.Bool{Valid: false},
		Notes:                  textPtr(req.Notes),
	})

	if err != nil {
		l.h.Logger.Error("Failed to update equipment", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update equipment"})
		return
	}

	config.RespondJSON(w, http.StatusOK, equipment)
}

// DeleteLabEquipment deletes lab equipment
func (l *LaboratoryHandler) DeleteLabEquipment(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
		return
	}

	err = l.h.Queries.DeleteLabEquipment(r.Context(), int32(id))
	if err != nil {
		l.h.Logger.Error("Failed to delete equipment", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete equipment"})
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]string{"message": "Equipment deleted successfully"})
}
