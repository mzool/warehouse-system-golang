package middlewares

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
)

// PaginationConfig holds configuration for pagination middleware
// Follows v2 design: no global dependencies, explicit configuration
type PaginationConfig struct {
	// DefaultPage is the default page number if not specified
	// Default: 1
	DefaultPage int

	// DefaultPageSize is the default number of items per page
	// Default: 20
	DefaultPageSize int

	// MaxPageSize is the maximum allowed page size (security limit)
	// Prevents database overload from large queries
	// Default: 100
	MaxPageSize int

	// Logger for structured logging
	// Default: slog.Default()
	Logger *slog.Logger

	// ErrorHandler handles pagination validation errors
	// Default: returns 400 JSON error
	ErrorHandler func(w http.ResponseWriter, r *http.Request, err error)
}

// DefaultPaginationConfig returns sensible defaults for pagination
func DefaultPaginationConfig() *PaginationConfig {
	return &PaginationConfig{
		DefaultPage:     1,
		DefaultPageSize: 20,
		MaxPageSize:     100,
		Logger:          slog.Default(),
		ErrorHandler:    defaultPaginationErrorHandler,
	}
}

// PaginationParams holds pagination parameters
type PaginationParams struct {
	Page   int   `json:"page"`   // Current page number (1-indexed)
	Limit  int   `json:"limit"`  // Number of items per page
	Offset int   `json:"offset"` // Calculated offset for database query
	Total  int64 `json:"total"`  // Total number of items (optional, for response)
	Pages  int   `json:"pages"`  // Total number of pages (calculated)
}

// PaginationMeta contains pagination metadata for response
type PaginationMeta struct {
	CurrentPage  int   `json:"current_page"`
	PerPage      int   `json:"per_page"`
	TotalPages   int   `json:"total_pages"`
	TotalRecords int64 `json:"total_records"`
	HasNext      bool  `json:"has_next"`
	HasPrev      bool  `json:"has_prev"`
	NextPage     *int  `json:"next_page,omitempty"`
	PrevPage     *int  `json:"prev_page,omitempty"`
}

type contextKeyPagination string

const paginationContextKey contextKeyPagination = "pagination"

// ParsePagination extracts and validates pagination parameters from request
// Usage: params := ParsePagination(r, config)
func ParsePagination(r *http.Request, config *PaginationConfig) *PaginationParams {
	if config == nil {
		config = DefaultPaginationConfig()
	}

	query := r.URL.Query()

	// Parse page number
	page := config.DefaultPage
	if pageStr := query.Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		} else {
			config.Logger.Debug("Invalid page parameter", "value", pageStr)
		}
	}

	// Parse limit (page size)
	limit := config.DefaultPageSize
	if limitStr := query.Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		} else {
			config.Logger.Debug("Invalid limit parameter", "value", limitStr)
		}
	}

	// Enforce maximum limit (SECURITY: prevent database overload)
	if limit > config.MaxPageSize {
		config.Logger.Debug("Limit exceeds maximum, capping",
			"requested", limit,
			"maximum", config.MaxPageSize,
		)
		limit = config.MaxPageSize
	}

	// Calculate offset (0-indexed for database)
	offset := (page - 1) * limit

	return &PaginationParams{
		Page:   page,
		Limit:  limit,
		Offset: offset,
	}
}

// Validate checks if pagination parameters are within acceptable ranges
func (p *PaginationParams) Validate(config *PaginationConfig) error {
	if config == nil {
		config = DefaultPaginationConfig()
	}

	if p.Page < 1 {
		return fmt.Errorf("page must be greater than 0")
	}

	if p.Limit < 1 {
		return fmt.Errorf("limit must be greater than 0")
	}

	if p.Limit > config.MaxPageSize {
		return fmt.Errorf("limit exceeds maximum allowed size of %d", config.MaxPageSize)
	}

	if p.Offset < 0 {
		return fmt.Errorf("offset must be non-negative")
	}

	return nil
}

// SetTotal sets the total count and calculates total pages
func (p *PaginationParams) SetTotal(total int64) {
	p.Total = total
	p.Pages = int(math.Ceil(float64(total) / float64(p.Limit)))
}

// BuildMeta creates pagination metadata for API response
func (p *PaginationParams) BuildMeta() *PaginationMeta {
	meta := &PaginationMeta{
		CurrentPage:  p.Page,
		PerPage:      p.Limit,
		TotalPages:   p.Pages,
		TotalRecords: p.Total,
		HasNext:      p.Page < p.Pages,
		HasPrev:      p.Page > 1,
	}

	// Add next page number if available
	if meta.HasNext {
		nextPage := p.Page + 1
		meta.NextPage = &nextPage
	}

	// Add previous page number if available
	if meta.HasPrev {
		prevPage := p.Page - 1
		meta.PrevPage = &prevPage
	}

	return meta
}

// ToContext adds pagination params to request context
func (p *PaginationParams) ToContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, paginationContextKey, p)
}

// FromContext retrieves pagination params from request context
func FromContext(ctx context.Context) (*PaginationParams, bool) {
	params, ok := ctx.Value(paginationContextKey).(*PaginationParams)
	return params, ok
}

// GetPagination retrieves pagination from context or returns default
func GetPagination(ctx context.Context) *PaginationParams {
	params, ok := FromContext(ctx)
	if !ok {
		return &PaginationParams{
			Page:  1,
			Limit: 20,
		}
	}
	return params
}

// defaultPaginationErrorHandler is the default error handler
func defaultPaginationErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)

	response := map[string]interface{}{
		"status":  "error",
		"message": "Invalid pagination parameters",
		"error":   err.Error(),
	}

	json.NewEncoder(w).Encode(response)
}

// Pagination returns a pagination middleware
// This middleware parses, validates, and enforces pagination limits
func Pagination(config *PaginationConfig) func(next http.Handler) http.Handler {
	if config == nil {
		config = DefaultPaginationConfig()
	}

	// Set defaults
	if config.DefaultPage == 0 {
		config.DefaultPage = 1
	}
	if config.DefaultPageSize == 0 {
		config.DefaultPageSize = 20
	}
	if config.MaxPageSize == 0 {
		config.MaxPageSize = 100
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if config.ErrorHandler == nil {
		config.ErrorHandler = defaultPaginationErrorHandler
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Parse pagination parameters
			pagination := ParsePagination(r, config)

			// Validate parameters
			if err := pagination.Validate(config); err != nil {
				config.Logger.Debug("Pagination validation failed",
					"error", err,
					"page", pagination.Page,
					"limit", pagination.Limit,
				)
				config.ErrorHandler(w, r, err)
				return
			}

			config.Logger.Debug("Pagination parsed",
				"page", pagination.Page,
				"limit", pagination.Limit,
				"offset", pagination.Offset,
			)

			// Add to context for handlers to use
			ctx := pagination.ToContext(r.Context())

			// Call next handler with updated context
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ApplyPaginationToQuery is a helper to build SQL LIMIT/OFFSET clauses
// Note: For sqlc, you'd pass these directly to your query functions
func (p *PaginationParams) ApplyPaginationToQuery() (limit, offset int) {
	return p.Limit, p.Offset
}

// GetSQLLimitOffset returns LIMIT and OFFSET for SQL queries
func (p *PaginationParams) GetSQLLimitOffset() (int32, int32) {
	return int32(p.Limit), int32(p.Offset)
}

// PaginationInfo returns a human-readable string about pagination
func (p *PaginationParams) PaginationInfo() string {
	if p.Total == 0 {
		return fmt.Sprintf("No results (Page %d)", p.Page)
	}

	start := p.Offset + 1
	end := p.Offset + p.Limit
	if int64(end) > p.Total {
		end = int(p.Total)
	}

	return fmt.Sprintf("Showing %d-%d of %d results (Page %d/%d)",
		start, end, p.Total, p.Page, p.Pages)
}

// ============================================================================
// Response Helpers (Optional - use your own response format)
// ============================================================================

// PaginatedResponse is a standard paginated response structure
type PaginatedResponse struct {
	Status     string          `json:"status"`
	Message    string          `json:"message,omitempty"`
	Data       interface{}     `json:"data"`
	Pagination *PaginationMeta `json:"pagination"`
}

// RespondPaginated sends a paginated JSON response
// This is a convenience helper - you can use your own response format
func RespondPaginated(w http.ResponseWriter, data interface{}, pagination *PaginationParams, message string) {
	if message == "" {
		message = "Data retrieved successfully"
	}

	response := PaginatedResponse{
		Status:     "success",
		Message:    message,
		Data:       data,
		Pagination: pagination.BuildMeta(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
