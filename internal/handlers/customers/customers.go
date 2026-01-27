package customers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"warehouse_system/internal/config"
	db "warehouse_system/internal/database/db"
	"warehouse_system/internal/handlers"
	"warehouse_system/internal/middlewares"
)

type CustomerHandler struct {
	h *handlers.Handler
}

func NewCustomerHandler(h *handlers.Handler) *CustomerHandler {
	return &CustomerHandler{h: h}
}

type CreateCustomerRequest struct {
	Name         string          `json:"name"`
	ContactName  string          `json:"contact_name"`
	ContactEmail string          `json:"contact_email"`
	ContactPhone string          `json:"contact_phone"`
	Address      string          `json:"address"`
	Meta         json.RawMessage `json:"meta"`
}

type UpdateCustomerRequest struct {
	Name         *string         `json:"name,omitempty"`
	ContactName  *string         `json:"contact_name,omitempty"`
	ContactEmail *string         `json:"contact_email,omitempty"`
	ContactPhone *string         `json:"contact_phone,omitempty"`
	Address      *string         `json:"address,omitempty"`
	Meta         json.RawMessage `json:"meta,omitempty"`
}

// CreateCustomer creates a new customer.
func (ch *CustomerHandler) CreateCustomer(w http.ResponseWriter, r *http.Request) {
	var req CreateCustomerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondBadRequest(w, "Invalid request payload", err.Error())
		return
	}

	if req.Name == "" {
		config.RespondBadRequest(w, "Missing required fields", "Name is required")
		return
	}

	// Check for duplicate name
	_, err := ch.h.Queries.GetCustomerByName(context.Background(), req.Name)
	if err == nil {
		config.RespondJSON(w, http.StatusConflict, map[string]string{"error": "Customer name already exists"})
		return
	}

	// Check for duplicate email if provided
	if req.ContactEmail != "" {
		_, err := ch.h.Queries.GetCustomerByEmail(context.Background(), pgtype.Text{String: req.ContactEmail, Valid: true})
		if err == nil {
			config.RespondJSON(w, http.StatusConflict, map[string]string{"error": "Customer email already exists"})
			return
		}
	}

	// Check for duplicate phone if provided
	if req.ContactPhone != "" {
		_, err := ch.h.Queries.GetCustomerByPhone(context.Background(), pgtype.Text{String: req.ContactPhone, Valid: true})
		if err == nil {
			config.RespondJSON(w, http.StatusConflict, map[string]string{"error": "Customer phone already exists"})
			return
		}
	}

	params := db.CreateCustomerParams{
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

	customer, err := ch.h.Queries.CreateCustomer(context.Background(), params)
	if err != nil {
		ch.h.Logger.Error("Failed to create customer", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusCreated, customer)
}

// GetCustomer retrieves a customer by ID.
func (ch *CustomerHandler) GetCustomer(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing customer ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid customer ID format", err.Error())
		return
	}

	customer, err := ch.h.Queries.GetCustomerByID(context.Background(), id)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Customer not found"})
		return
	}

	config.RespondJSON(w, http.StatusOK, customer)
}

// UpdateCustomer updates an existing customer.
func (ch *CustomerHandler) UpdateCustomer(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing customer ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid customer ID format", err.Error())
		return
	}

	var req UpdateCustomerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondBadRequest(w, "Invalid request payload", err.Error())
		return
	}

	// Get current customer
	current, err := ch.h.Queries.GetCustomerByID(context.Background(), id)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Customer not found"})
		return
	}

	// Check for duplicate name if being updated
	if req.Name != nil && *req.Name != current.Name {
		_, err := ch.h.Queries.GetCustomerByName(context.Background(), *req.Name)
		if err == nil {
			config.RespondJSON(w, http.StatusConflict, map[string]string{"error": "Customer name already exists"})
			return
		}
	}

	// Check for duplicate email if being updated
	if req.ContactEmail != nil && *req.ContactEmail != "" {
		currentEmail := ""
		if current.ContactEmail.Valid {
			currentEmail = current.ContactEmail.String
		}
		if *req.ContactEmail != currentEmail {
			_, err := ch.h.Queries.GetCustomerByEmail(context.Background(), pgtype.Text{String: *req.ContactEmail, Valid: true})
			if err == nil {
				config.RespondJSON(w, http.StatusConflict, map[string]string{"error": "Customer email already exists"})
				return
			}
		}
	}

	// Check for duplicate phone if being updated
	if req.ContactPhone != nil && *req.ContactPhone != "" {
		currentPhone := ""
		if current.ContactPhone.Valid {
			currentPhone = current.ContactPhone.String
		}
		if *req.ContactPhone != currentPhone {
			_, err := ch.h.Queries.GetCustomerByPhone(context.Background(), pgtype.Text{String: *req.ContactPhone, Valid: true})
			if err == nil {
				config.RespondJSON(w, http.StatusConflict, map[string]string{"error": "Customer phone already exists"})
				return
			}
		}
	}

	params := db.UpdateCustomerParams{
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

	customer, err := ch.h.Queries.UpdateCustomer(context.Background(), params)
	if err != nil {
		ch.h.Logger.Error("Failed to update customer", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, customer)
}

// ListCustomers lists all customers with pagination.
func (ch *CustomerHandler) ListCustomers(w http.ResponseWriter, r *http.Request) {
	pagination := middlewares.GetPagination(r.Context())

	var customers []db.Customer
	var err error
	var totalCount int64

	// Check if there's a search query
	query := r.URL.Query().Get("query")
	if query != "" {
		// Search customers
		customers, err = ch.h.Queries.SearchCustomers(context.Background(), db.SearchCustomersParams{
			Limit:  int32(pagination.Limit),
			Offset: int32(pagination.Offset),
			Query:  pgtype.Text{String: query, Valid: true},
		})
		if err != nil {
			ch.h.Logger.Error("Failed to search customers", "error", err)
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		// Get search total count
		totalCount, err = ch.h.Queries.CountSearchCustomers(context.Background(), pgtype.Text{String: query, Valid: true})
		if err != nil {
			ch.h.Logger.Error("Failed to count search results", "error", err)
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	} else {
		// List all customers
		customers, err = ch.h.Queries.ListCustomers(context.Background(), db.ListCustomersParams{
			Limit:  int32(pagination.Limit),
			Offset: int32(pagination.Offset),
		})
		if err != nil {
			ch.h.Logger.Error("Failed to list customers", "error", err)
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		// Get total count
		totalCount, err = ch.h.Queries.CountCustomers(context.Background())
		if err != nil {
			ch.h.Logger.Error("Failed to count customers", "error", err)
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}

	// Set total and build pagination metadata
	pagination.Total = totalCount

	config.RespondJSON(w, http.StatusOK, map[string]any{
		"customers":  customers,
		"pagination": pagination.BuildMeta(),
	})
}

// DeleteCustomer deletes a customer by ID.
func (ch *CustomerHandler) DeleteCustomer(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing customer ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid customer ID format", err.Error())
		return
	}

	// Check if customer exists
	_, err := ch.h.Queries.GetCustomerByID(context.Background(), id)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Customer not found"})
		return
	}

	err = ch.h.Queries.DeleteCustomer(context.Background(), id)
	if err != nil {
		ch.h.Logger.Error("Failed to delete customer", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]string{"message": "Customer deleted successfully"})
}
