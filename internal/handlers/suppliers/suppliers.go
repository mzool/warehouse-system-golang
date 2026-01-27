package suppliers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"warehouse_system/internal/config"
	db "warehouse_system/internal/database/db"
	"warehouse_system/internal/handlers"
	"warehouse_system/internal/middlewares"
)

type SupplierHandler struct {
	h *handlers.Handler
}

func NewSupplierHandler(h *handlers.Handler) *SupplierHandler {
	return &SupplierHandler{h: h}
}

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

func isValidEmail(email string) bool {
	return emailRegex.MatchString(email)
}

func isValidPhone(phone string) bool {
	// Basic phone validation - at least 10 digits
	digitOnly := regexp.MustCompile(`[^\d]`).ReplaceAllString(phone, "")
	return len(digitOnly) >= 10
}

type CreateSupplierRequest struct {
	Name         string          `json:"name"`
	ContactName  string          `json:"contact_name"`
	ContactEmail string          `json:"contact_email"`
	ContactPhone string          `json:"contact_phone"`
	Address      string          `json:"address"`
	Meta         json.RawMessage `json:"meta"`
}

type UpdateSupplierRequest struct {
	Name         *string         `json:"name,omitempty"`
	ContactName  *string         `json:"contact_name,omitempty"`
	ContactEmail *string         `json:"contact_email,omitempty"`
	ContactPhone *string         `json:"contact_phone,omitempty"`
	Address      *string         `json:"address,omitempty"`
	Meta         json.RawMessage `json:"meta,omitempty"`
}

// CreateSupplier creates a new supplier.
func (sh *SupplierHandler) CreateSupplier(w http.ResponseWriter, r *http.Request) {
	var req CreateSupplierRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondBadRequest(w, "Invalid request payload", err.Error())
		return
	}

	// Validate required fields
	if req.Name == "" {
		config.RespondBadRequest(w, "Missing required fields", "Name is required")
		return
	}

	// Trim whitespace
	req.Name = strings.TrimSpace(req.Name)
	req.ContactName = strings.TrimSpace(req.ContactName)
	req.ContactEmail = strings.TrimSpace(req.ContactEmail)
	req.ContactPhone = strings.TrimSpace(req.ContactPhone)

	// Validate email format if provided
	if req.ContactEmail != "" && !isValidEmail(req.ContactEmail) {
		config.RespondBadRequest(w, "Invalid email format", "Please provide a valid email address")
		return
	}

	// Validate phone format if provided
	if req.ContactPhone != "" && !isValidPhone(req.ContactPhone) {
		config.RespondBadRequest(w, "Invalid phone format", "Phone number must contain at least 10 digits")
		return
	}

	// Check for duplicate name
	_, err := sh.h.Queries.GetSupplierByName(context.Background(), req.Name)
	if err == nil {
		config.RespondJSON(w, http.StatusConflict, map[string]string{"error": "Supplier name already exists"})
		return
	}

	// Check for duplicate email if provided
	if req.ContactEmail != "" {
		_, err := sh.h.Queries.GetSupplierByEmail(context.Background(), pgtype.Text{String: req.ContactEmail, Valid: true})
		if err == nil {
			config.RespondJSON(w, http.StatusConflict, map[string]string{"error": "Supplier email already exists"})
			return
		}
	}

	// Check for duplicate phone if provided
	if req.ContactPhone != "" {
		_, err := sh.h.Queries.GetSupplierByPhone(context.Background(), pgtype.Text{String: req.ContactPhone, Valid: true})
		if err == nil {
			config.RespondJSON(w, http.StatusConflict, map[string]string{"error": "Supplier phone already exists"})
			return
		}
	}

	params := db.CreateSupplierParams{
		Name: req.Name,
	}

	if req.ContactName != "" {
		params.ContactName = pgtype.Text{String: req.ContactName, Valid: true}
	}
	if req.ContactEmail != "" {
		params.ContactEmail = pgtype.Text{String: req.ContactEmail, Valid: true}
	}
	if req.ContactPhone != "" {
		params.ContactPhone = pgtype.Text{String: req.ContactPhone, Valid: true}
	}
	if req.Address != "" {
		params.Address = pgtype.Text{String: req.Address, Valid: true}
	}
	if req.Meta != nil {
		params.Meta = req.Meta
	}

	supplier, err := sh.h.Queries.CreateSupplier(context.Background(), params)
	if err != nil {
		sh.h.Logger.Error("Failed to create supplier", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusCreated, supplier)
}

// GetSupplier retrieves a supplier by ID.
func (sh *SupplierHandler) GetSupplier(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing supplier ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid supplier ID format", err.Error())
		return
	}

	supplier, err := sh.h.Queries.GetSupplierByID(context.Background(), id)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Supplier not found"})
		return
	}

	config.RespondJSON(w, http.StatusOK, supplier)
}

// UpdateSupplier updates an existing supplier.
func (sh *SupplierHandler) UpdateSupplier(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing supplier ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid supplier ID format", err.Error())
		return
	}

	var req UpdateSupplierRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondBadRequest(w, "Invalid request payload", err.Error())
		return
	}

	// Trim whitespace
	if req.Name != nil {
		*req.Name = strings.TrimSpace(*req.Name)
	}
	if req.ContactName != nil {
		*req.ContactName = strings.TrimSpace(*req.ContactName)
	}
	if req.ContactEmail != nil {
		*req.ContactEmail = strings.TrimSpace(*req.ContactEmail)
	}
	if req.ContactPhone != nil {
		*req.ContactPhone = strings.TrimSpace(*req.ContactPhone)
	}

	// Validate email format if provided
	if req.ContactEmail != nil && *req.ContactEmail != "" && !isValidEmail(*req.ContactEmail) {
		config.RespondBadRequest(w, "Invalid email format", "Please provide a valid email address")
		return
	}

	// Validate phone format if provided
	if req.ContactPhone != nil && *req.ContactPhone != "" && !isValidPhone(*req.ContactPhone) {
		config.RespondBadRequest(w, "Invalid phone format", "Phone number must contain at least 10 digits")
		return
	}

	// Get current supplier
	current, err := sh.h.Queries.GetSupplierByID(context.Background(), id)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Supplier not found"})
		return
	}

	// Check for duplicate name if being updated
	if req.Name != nil && *req.Name != current.Name {
		_, err := sh.h.Queries.GetSupplierByName(context.Background(), *req.Name)
		if err == nil {
			config.RespondJSON(w, http.StatusConflict, map[string]string{"error": "Supplier name already exists"})
			return
		}
	}

	// Check for duplicate email if being updated
	if req.ContactEmail != nil && *req.ContactEmail != "" {
		if !current.ContactEmail.Valid || *req.ContactEmail != current.ContactEmail.String {
			_, err := sh.h.Queries.GetSupplierByEmail(context.Background(), pgtype.Text{String: *req.ContactEmail, Valid: true})
			if err == nil {
				config.RespondJSON(w, http.StatusConflict, map[string]string{"error": "Supplier email already exists"})
				return
			}
		}
	}

	// Check for duplicate phone if being updated
	if req.ContactPhone != nil && *req.ContactPhone != "" {
		if !current.ContactPhone.Valid || *req.ContactPhone != current.ContactPhone.String {
			_, err := sh.h.Queries.GetSupplierByPhone(context.Background(), pgtype.Text{String: *req.ContactPhone, Valid: true})
			if err == nil {
				config.RespondJSON(w, http.StatusConflict, map[string]string{"error": "Supplier phone already exists"})
				return
			}
		}
	}

	params := db.UpdateSupplierParams{
		ID: id,
	}

	// Handle name (interface{} due to NULLIF)
	if req.Name != nil {
		params.Column2 = *req.Name
	} else {
		params.Column2 = ""
	}

	// Handle optional fields
	if req.ContactName != nil {
		params.ContactName = pgtype.Text{String: *req.ContactName, Valid: true}
	}
	if req.ContactEmail != nil {
		params.ContactEmail = pgtype.Text{String: *req.ContactEmail, Valid: true}
	}
	if req.ContactPhone != nil {
		params.ContactPhone = pgtype.Text{String: *req.ContactPhone, Valid: true}
	}
	if req.Address != nil {
		params.Address = pgtype.Text{String: *req.Address, Valid: true}
	}
	if req.Meta != nil {
		params.Meta = req.Meta
	}

	supplier, err := sh.h.Queries.UpdateSupplier(context.Background(), params)
	if err != nil {
		sh.h.Logger.Error("Failed to update supplier", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, supplier)
}

// DeleteSupplier deletes a supplier by ID.
func (sh *SupplierHandler) DeleteSupplier(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing supplier ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid supplier ID format", err.Error())
		return
	}

	if err := sh.h.Queries.DeleteSupplier(context.Background(), id); err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]string{"message": "Supplier deleted successfully"})
}

// ListSuppliers lists all suppliers with pagination and search.
func (sh *SupplierHandler) ListSuppliers(w http.ResponseWriter, r *http.Request) {
	pagination := middlewares.GetPagination(r.Context())
	limit, offset := pagination.GetSQLLimitOffset()

	// Get search query parameter
	query := r.URL.Query().Get("query")

	var suppliers []db.Supplier
	var total int64
	var err error

	if query != "" {
		// Search suppliers
		params := db.SearchSuppliersParams{
			Limit:  limit,
			Offset: offset,
			Query:  pgtype.Text{String: query, Valid: true},
		}

		suppliers, err = sh.h.Queries.SearchSuppliers(context.Background(), params)
		if err != nil {
			sh.h.Logger.Error("Failed to search suppliers", "error", err)
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		total, _ = sh.h.Queries.CountSearchSuppliers(context.Background(), pgtype.Text{String: query, Valid: true})
	} else {
		// List all suppliers
		params := db.ListSuppliersParams{
			Limit:  limit,
			Offset: offset,
		}

		suppliers, err = sh.h.Queries.ListSuppliers(context.Background(), params)
		if err != nil {
			sh.h.Logger.Error("Failed to list suppliers", "error", err)
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		total, _ = sh.h.Queries.CountSuppliers(context.Background())
	}

	pagination.SetTotal(total)

	config.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"suppliers":  suppliers,
		"pagination": pagination.BuildMeta(),
	})
}
