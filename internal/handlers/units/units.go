package units

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"warehouse_system/internal/config"
	db "warehouse_system/internal/database/db"
	"warehouse_system/internal/handlers"
	"warehouse_system/internal/middlewares"

	"github.com/jackc/pgx/v5/pgtype"
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
	Name             *string  `json:"name"`
	Abbreviation     *string  `json:"abbreviation"`
	ConvertionFactor *float64 `json:"convertion_factor"`
	ConvertTo        *int32   `json:"convert_to"`
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
	// Default is 1.0
	conversionFactor := 1.0
	if req.ConvertionFactor != 0 {
		conversionFactor = req.ConvertionFactor
	}

	// Set the numeric value properly
	params.ConvertionFactor = pgtype.Numeric{}
	if err := params.ConvertionFactor.Scan(fmt.Sprintf("%f", conversionFactor)); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Invalid convertion factor",
		})
		return
	}

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
	if req.ConvertTo != nil {
		params.ConvertTo = pgtype.Int4{
			Int32: *req.ConvertTo,
			Valid: true,
		}

		// If convert_to is the same as unit ID, force conversion_factor to 1
		if *req.ConvertTo == id {
			if err := params.ConvertionFactor.Scan(1.0); err != nil {
				config.RespondJSON(w, http.StatusBadRequest, map[string]string{
					"error": "Invalid convertion factor",
				})
				return
			}
		} else if req.ConvertionFactor != nil {
			// Use provided conversion factor
			if err := params.ConvertionFactor.Scan(*req.ConvertionFactor); err != nil {
				config.RespondJSON(w, http.StatusBadRequest, map[string]string{
					"error": "Invalid convertion factor",
				})
				return
			}
		} else {
			// Default to 1.0 if not provided
			if err := params.ConvertionFactor.Scan(1.0); err != nil {
				config.RespondJSON(w, http.StatusBadRequest, map[string]string{
					"error": "Invalid convertion factor",
				})
				return
			}
		}
	} else if req.ConvertionFactor != nil {
		// Only conversion factor is being updated
		if err := params.ConvertionFactor.Scan(*req.ConvertionFactor); err != nil {
			config.RespondJSON(w, http.StatusBadRequest, map[string]string{
				"error": "Invalid convertion factor",
			})
			return
		}
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
