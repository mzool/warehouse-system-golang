package units

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"warehouse_system/internal/config"
	db "warehouse_system/internal/database/db"
	"warehouse_system/internal/handlers"
	"warehouse_system/internal/middlewares"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/xuri/excelize/v2"
)

type UnitHandler struct {
	h *handlers.Handler
}

func NewUnitHandler(h *handlers.Handler) *UnitHandler {
	return &UnitHandler{h: h}
}

// CreateUnitRequest represents the request payload for creating a unit
type CreateUnitRequest struct {
	Name             string  `json:"name"`
	Abbreviation     string  `json:"abbreviation"`
	ConvertionFactor float64 `json:"convertion_factor"`
	ConvertTo        int32   `json:"convert_to"`
}

// UpdateUnitRequest represents the request payload for updating a unit
type UpdateUnitRequest struct {
	Name             *string  `json:"name,omitempty"`
	Abbreviation     *string  `json:"abbreviation,omitempty"`
	ConvertionFactor *float64 `json:"convertion_factor,omitempty"`
	ConvertTo        *int32   `json:"convert_to"`                  // No omitempty - we need to detect null
	RemoveConversion bool     `json:"remove_conversion,omitempty"` // Explicit flag to remove conversion
}

// CreateUnit handles the creation of a new measurement unit.
func (u *UnitHandler) CreateUnit(w http.ResponseWriter, r *http.Request) {
	var req CreateUnitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondBadRequest(w, "Invalid request payload", err.Error())
		return
	}

	// Validate required fields
	if req.Name == "" {
		config.RespondBadRequest(w, "Name is required", "")
		return
	}
	if req.Abbreviation == "" {
		config.RespondBadRequest(w, "Abbreviation is required", "")
		return
	}

	// Prepare parameters
	params := db.CreateUnitParams{
		Name:         req.Name,
		Abbreviation: req.Abbreviation,
	}

	// Handle convert_to (optional)
	if req.ConvertTo != 0 {
		params.ConvertTo = pgtype.Int4{
			Int32: req.ConvertTo,
			Valid: true,
		}
	}

	// Handle conversion_factor
	conversionFactor := 1.0
	if req.ConvertionFactor != 0 {
		conversionFactor = req.ConvertionFactor
	}

	// Set the numeric value properly using string conversion
	var numericValue pgtype.Numeric
	if err := numericValue.Scan(fmt.Sprintf("%.4f", conversionFactor)); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Invalid convertion factor",
		})
		return
	}
	params.ConvertionFactor = numericValue

	unit, err := u.h.Queries.CreateUnit(context.Background(), params)
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	config.RespondJSON(w, http.StatusCreated, unit)
}

// GetUnitByID retrieves a measurement unit by its ID.
func (u *UnitHandler) GetUnitByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing unit ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid unit ID format", err.Error())
		return
	}

	unit, err := u.h.Queries.GetUnitByID(context.Background(), id)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{
			"error": "Unit not found",
		})
		return
	}

	config.RespondJSON(w, http.StatusOK, unit)
}

// GetUnitByName retrieves a measurement unit by its name.
func (u *UnitHandler) GetUnitByName(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		config.RespondBadRequest(w, "Missing unit name", "")
		return
	}

	unit, err := u.h.Queries.GetUnitByName(context.Background(), name)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{
			"error": "Unit not found",
		})
		return
	}

	config.RespondJSON(w, http.StatusOK, unit)
}

// GetUnitByAbbreviation retrieves a measurement unit by its abbreviation.
func (u *UnitHandler) GetUnitByAbbreviation(w http.ResponseWriter, r *http.Request) {
	abbreviation := r.URL.Query().Get("abbreviation")
	if abbreviation == "" {
		config.RespondBadRequest(w, "Missing unit abbreviation", "")
		return
	}

	unit, err := u.h.Queries.GetUnitByAbbreviation(context.Background(), abbreviation)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{
			"error": "Unit not found",
		})
		return
	}

	config.RespondJSON(w, http.StatusOK, unit)
}

// UpdateUnit updates the details of an existing measurement unit.
func (u *UnitHandler) UpdateUnit(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing unit ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid unit ID format", err.Error())
		return
	}

	var req UpdateUnitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondBadRequest(w, "Invalid request payload", err.Error())
		return
	}

	// Get current unit to preserve fields that aren't being updated
	currentUnit, err := u.h.Queries.GetUnitByID(context.Background(), id)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{
			"error": "Unit not found",
		})
		return
	}

	// Build update params with current values as defaults
	params := db.UpdateUnitParams{
		ID:               id,
		Name:             currentUnit.Name,
		Abbreviation:     currentUnit.Abbreviation,
		ConvertionFactor: currentUnit.ConvertionFactor,
		ConvertTo:        currentUnit.ConvertTo,
	}

	// Override with new values if provided
	if req.Name != nil {
		params.Name = *req.Name
	}
	if req.Abbreviation != nil {
		params.Abbreviation = *req.Abbreviation
	}

	// Handle convert_to update
	// Option 1: Explicit flag to remove conversion
	if req.RemoveConversion {
		params.ConvertTo = pgtype.Int4{Valid: false}
		// When removing conversion, set factor to 1.0 (base unit)
		var numericValue pgtype.Numeric
		if err := numericValue.Scan("1.0"); err != nil {
			config.RespondJSON(w, http.StatusBadRequest, map[string]string{
				"error": "Invalid convertion factor",
			})
			return
		}
		params.ConvertionFactor = numericValue
	} else if req.ConvertTo != nil {
		// Option 2: convert_to sent with a value
		// If convert_to is 0, also treat it as removing the conversion
		if *req.ConvertTo == 0 {
			params.ConvertTo = pgtype.Int4{Valid: false}
			// When removing conversion, set factor to 1.0 (base unit)
			var numericValue pgtype.Numeric
			if err := numericValue.Scan("1.0"); err != nil {
				config.RespondJSON(w, http.StatusBadRequest, map[string]string{
					"error": "Invalid convertion factor",
				})
				return
			}
			params.ConvertionFactor = numericValue
		} else {
			params.ConvertTo = pgtype.Int4{
				Int32: *req.ConvertTo,
				Valid: true,
			}

			// If convert_to is the same as unit ID, force conversion_factor to 1
			if *req.ConvertTo == id {
				var numericValue pgtype.Numeric
				if err := numericValue.Scan("1.0"); err != nil {
					config.RespondJSON(w, http.StatusBadRequest, map[string]string{
						"error": "Invalid convertion factor",
					})
					return
				}
				params.ConvertionFactor = numericValue
			} else if req.ConvertionFactor != nil {
				// Use provided conversion factor
				var numericValue pgtype.Numeric
				if err := numericValue.Scan(fmt.Sprintf("%.4f", *req.ConvertionFactor)); err != nil {
					config.RespondJSON(w, http.StatusBadRequest, map[string]string{
						"error": "Invalid convertion factor",
					})
					return
				}
				params.ConvertionFactor = numericValue
			} else {
				// Default to 1.0 if not provided
				var numericValue pgtype.Numeric
				if err := numericValue.Scan("1.0"); err != nil {
					config.RespondJSON(w, http.StatusBadRequest, map[string]string{
						"error": "Invalid convertion factor",
					})
					return
				}
				params.ConvertionFactor = numericValue
			}
		}
	} else if req.ConvertionFactor != nil {
		// Only conversion factor is being updated
		var numericValue pgtype.Numeric
		if err := numericValue.Scan(fmt.Sprintf("%.4f", *req.ConvertionFactor)); err != nil {
			config.RespondJSON(w, http.StatusBadRequest, map[string]string{
				"error": "Invalid convertion factor",
			})
			return
		}
		params.ConvertionFactor = numericValue
	}

	updatedUnit, err := u.h.Queries.UpdateUnit(context.Background(), params)
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	config.RespondJSON(w, http.StatusOK, updatedUnit)
}

// DeleteUnit removes a measurement unit by its ID.
func (u *UnitHandler) DeleteUnit(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing unit ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid unit ID format", err.Error())
		return
	}

	// Dropped --> less db queries, bad UX and safe
	// Check if other units reference this unit
	// unitReferences, err := u.h.Queries.CheckUnitReferences(context.Background(), pgtype.Int4{
	// 	Int32: id,
	// 	Valid: true,
	// })
	// if err != nil {
	// 	config.RespondJSON(w, http.StatusInternalServerError, map[string]string{
	// 		"error": "Failed to check unit references",
	// 	})
	// 	return
	// }

	// if unitReferences > 0 {
	// 	config.RespondJSON(w, http.StatusConflict, map[string]string{
	// 		"error":   "Cannot delete unit",
	// 		"message": fmt.Sprintf("This unit is referenced by %d other unit(s). Please update or delete those units first.", unitReferences),
	// 	})
	// 	return
	// }

	// Check if materials use this unit
	// materialCount, err := u.h.Queries.CheckUnitUsedByMaterials(context.Background(), pgtype.Int4{
	// 	Int32: id,
	// 	Valid: true,
	// })
	// if err != nil {
	// 	config.RespondJSON(w, http.StatusInternalServerError, map[string]string{
	// 		"error": "Failed to check material usage",
	// 	})
	// 	return
	// }

	// if materialCount > 0 {
	// 	config.RespondJSON(w, http.StatusConflict, map[string]string{
	// 		"error":   "Cannot delete unit",
	// 		"message": fmt.Sprintf("This unit is used by %d material(s). Please update those materials first.", materialCount),
	// 	})
	// 	return
	// }

	// Safe to delete
	if err := u.h.Queries.DeleteUnit(context.Background(), id); err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]string{
		"message": "Unit deleted successfully",
	})
}

// ListUnits retrieves a list of all measurement units.
func (u *UnitHandler) ListUnits(w http.ResponseWriter, r *http.Request) {
	// Get pagination from context (set by pagination middleware)
	pagination := middlewares.GetPagination(r.Context())
	limit, offset := pagination.GetSQLLimitOffset()

	units, err := u.h.Queries.ListUnits(context.Background(), db.ListUnitsParams{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	// Get total count for pagination metadata
	total, err := u.h.Queries.CountUnits(context.Background())
	if err != nil {
		u.h.Logger.Error("Failed to count units", "error", err)
		// Continue without total count
	}

	// Set total and build pagination metadata
	pagination.SetTotal(total)

	response := map[string]interface{}{
		"units":      units,
		"pagination": pagination.BuildMeta(),
	}

	config.RespondJSON(w, http.StatusOK, response)
}

func (u *UnitHandler) DownloadTemplate(w http.ResponseWriter, r *http.Request) {
	f := excelize.NewFile()
	defer f.Close()

	sheet := "Units"
	index, _ := f.NewSheet(sheet)
	f.SetActiveSheet(index)
	f.DeleteSheet("Sheet1")

	headers := []string{"Name", "Abbreviation", "Conversion Factor", "Convert To Unit Name"}

	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	// Add example rows
	f.SetCellValue(sheet, "A2", "Gram")
	f.SetCellValue(sheet, "B2", "g")
	f.SetCellValue(sheet, "C2", "1")
	f.SetCellValue(sheet, "D2", "")

	f.SetCellValue(sheet, "A3", "Kilogram")
	f.SetCellValue(sheet, "B3", "kg")
	f.SetCellValue(sheet, "C3", "1000")
	f.SetCellValue(sheet, "D3", "Gram")

	w.Header().Set("Content-Disposition", "attachment; filename=units_template.xlsx")
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	_ = f.Write(w)
}

func (u *UnitHandler) ExportUnits(w http.ResponseWriter, r *http.Request) {
	units, err := u.h.Queries.ListUnits(context.Background(), db.ListUnitsParams{
		Limit:  1000000,
		Offset: 0,
	})
	if err != nil {
		u.h.Logger.Error("Failed to fetch units for export", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch units"})
		return
	}

	f := excelize.NewFile()
	defer f.Close()

	sheet := "Units"
	index, _ := f.NewSheet(sheet)
	f.SetActiveSheet(index)
	f.DeleteSheet("Sheet1")

	headers := []string{"ID", "Name", "Abbreviation", "Conversion Factor", "Convert To", "Created At", "Updated At"}

	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	for i, unit := range units {
		row := i + 2
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), unit.ID)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), unit.Name)
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), unit.Abbreviation)

		// Format numeric considering scale and exponent
		var factorStr string
		if unit.ConvertionFactor.Valid {
			// Convert to float64 for proper decimal representation
			floatVal, _ := unit.ConvertionFactor.Float64Value()
			factorStr = fmt.Sprintf("%.4f", floatVal.Float64)
		}
		f.SetCellValue(sheet, fmt.Sprintf("D%d", row), factorStr)

		f.SetCellValue(sheet, fmt.Sprintf("E%d", row), unit.ConvertToName.String)
		f.SetCellValue(sheet, fmt.Sprintf("F%d", row), unit.CreatedAt.Time.Format("2006-01-02 15:04:05"))
		f.SetCellValue(sheet, fmt.Sprintf("G%d", row), unit.UpdatedAt.Time.Format("2006-01-02 15:04:05"))
	}

	w.Header().Set("Content-Disposition", "attachment; filename=units_export.xlsx")
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	_ = f.Write(w)
}

func (u *UnitHandler) ImportFromExcel(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		config.RespondBadRequest(w, "Failed to parse form", err.Error())
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		config.RespondBadRequest(w, "No file uploaded", err.Error())
		return
	}
	defer file.Close()

	f, err := excelize.OpenReader(file)
	if err != nil {
		config.RespondBadRequest(w, "Invalid Excel file", err.Error())
		return
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	rows, err := f.GetRows(sheet)
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to read Excel rows"})
		return
	}

	if len(rows) < 2 {
		config.RespondBadRequest(w, "Excel file is empty", "")
		return
	}

	successCount := 0
	errorCount := 0
	errors := []string{}

	for i, row := range rows[1:] {
		rowNum := i + 2
		if len(row) < 3 {
			errorCount++
			errors = append(errors, fmt.Sprintf("Row %d: insufficient columns", rowNum))
			continue
		}

		name := strings.TrimSpace(row[0])
		abbreviation := strings.TrimSpace(row[1])
		conversionFactorStr := strings.TrimSpace(row[2])
		convertToName := ""
		if len(row) > 3 {
			convertToName = strings.TrimSpace(row[3])
		}

		if name == "" || abbreviation == "" {
			errorCount++
			errors = append(errors, fmt.Sprintf("Row %d: name and abbreviation are required", rowNum))
			continue
		}

		conversionFactor := 1.0
		if conversionFactorStr != "" {
			if parsed, err := strconv.ParseFloat(conversionFactorStr, 64); err == nil {
				conversionFactor = parsed
			} else {
				errorCount++
				errors = append(errors, fmt.Sprintf("Row %d: invalid conversion factor", rowNum))
				continue
			}
		}

		// Prepare conversion factor as pgtype.Numeric
		var numericFactor pgtype.Numeric
		numericFactor.Scan(fmt.Sprintf("%.4f", conversionFactor))

		// Handle convert_to by name lookup
		var convertToID pgtype.Int4
		if convertToName != "" {
			targetUnit, err := u.h.Queries.GetUnitByName(context.Background(), convertToName)
			if err != nil {
				errorCount++
				errors = append(errors, fmt.Sprintf("Row %d: target unit '%s' not found", rowNum, convertToName))
				continue
			}
			convertToID = pgtype.Int4{Int32: targetUnit.ID, Valid: true}
		}

		params := db.CreateUnitParams{
			Name:             name,
			Abbreviation:     abbreviation,
			ConvertionFactor: numericFactor,
			ConvertTo:        convertToID,
		}

		_, err := u.h.Queries.CreateUnit(context.Background(), params)
		if err != nil {
			errorCount++
			errors = append(errors, fmt.Sprintf("Row %d: %s", rowNum, err.Error()))
			continue
		}

		successCount++
	}

	config.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message":       "Import completed",
		"success_count": successCount,
		"error_count":   errorCount,
		"errors":        errors,
	})
}
