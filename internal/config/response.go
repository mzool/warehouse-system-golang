package config

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// Response represents a standard API response structure
// This is optional - handlers can use their own response formats
type Response struct {
	Status     string         `json:"status"`
	Message    string         `json:"message"`
	Details    string         `json:"details,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
	StatusCode int            `json:"-"`
}

// ErrorResponse represents a standard error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
	Code    string `json:"code,omitempty"`
}

// SuccessResponse represents a standard success response
type SuccessResponse struct {
	Success bool           `json:"success"`
	Message string         `json:"message,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
}

// RespondJSON is a helper function to send JSON responses
// Handlers are free to use this or encode JSON directly
func RespondJSON(w http.ResponseWriter, statusCode int, data interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	return json.NewEncoder(w).Encode(data)
}

// RespondError is a helper function to send error responses
// Optional to use - handlers can create custom error responses
func RespondError(w http.ResponseWriter, statusCode int, message string, details string, logger *slog.Logger) {
	if logger != nil {
		logger.Error("responding with error",
			"status_code", statusCode,
			"message", message,
			"details", details,
		)
	}

	response := ErrorResponse{
		Error:   http.StatusText(statusCode),
		Message: message,
		Details: details,
	}

	RespondJSON(w, statusCode, response)
}

// RespondSuccess is a helper function to send success responses
// Optional to use - handlers can create custom success responses
func RespondSuccess(w http.ResponseWriter, statusCode int, message string, data map[string]any) {
	response := SuccessResponse{
		Success: true,
		Message: message,
		Data:    data,
	}

	RespondJSON(w, statusCode, response)
}

// RespondInternalError is a helper for 500 errors
func RespondInternalError(w http.ResponseWriter, err error, logger *slog.Logger) {
	if logger != nil {
		logger.Error("internal server error", "error", err)
	}

	response := ErrorResponse{
		Error:   "Internal Server Error",
		Message: "An unexpected error occurred",
		Details: "", // Don't expose internal errors in production
	}

	RespondJSON(w, http.StatusInternalServerError, response)
}

// RespondBadRequest is a helper for 400 errors
func RespondBadRequest(w http.ResponseWriter, message string, details string) {
	response := ErrorResponse{
		Error:   "Bad Request",
		Message: message,
		Details: details,
	}

	RespondJSON(w, http.StatusBadRequest, response)
}

// RespondNotFound is a helper for 404 errors
func RespondNotFound(w http.ResponseWriter, message string) {
	response := ErrorResponse{
		Error:   "Not Found",
		Message: message,
	}

	RespondJSON(w, http.StatusNotFound, response)
}

// RespondUnauthorized is a helper for 401 errors
func RespondUnauthorized(w http.ResponseWriter, message string) {
	response := ErrorResponse{
		Error:   "Unauthorized",
		Message: message,
	}

	RespondJSON(w, http.StatusUnauthorized, response)
}

// RespondForbidden is a helper for 403 errors
func RespondForbidden(w http.ResponseWriter, message string) {
	response := ErrorResponse{
		Error:   "Forbidden",
		Message: message,
	}

	RespondJSON(w, http.StatusForbidden, response)
}

// RespondConflict is a helper for 409 errors
func RespondConflict(w http.ResponseWriter, message string, details string) {
	response := ErrorResponse{
		Error:   "Conflict",
		Message: message,
		Details: details,
	}

	RespondJSON(w, http.StatusConflict, response)
}

// RespondValidationError is a helper for validation errors (422)
func RespondValidationError(w http.ResponseWriter, errors interface{}) {
	response := map[string]interface{}{
		"error":   "Validation Failed",
		"message": "One or more fields failed validation",
		"errors":  errors,
	}

	RespondJSON(w, http.StatusUnprocessableEntity, response)
}

// RespondCreated is a helper for 201 responses
func RespondCreated(w http.ResponseWriter, message string, data map[string]any) {
	RespondSuccess(w, http.StatusCreated, message, data)
}

// RespondNoContent is a helper for 204 responses (no body)
func RespondNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// Deprecated: Use RespondError instead
// ErrorHandler is kept for backward compatibility but should be avoided
// Prefer using RespondError or RespondInternalError with explicit logger
func ErrorHandler(w http.ResponseWriter, err error) {
	RespondInternalError(w, err, nil)
}

// Deprecated: Use RespondJSON or specific helpers instead
// Respond is kept for backward compatibility but should be avoided
// Prefer using RespondSuccess, RespondError, or RespondJSON directly
func Respond(w http.ResponseWriter, res Response) {
	w.Header().Set("Content-Type", "application/json")
	if res.StatusCode == 0 {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(res.StatusCode)
	}
	json.NewEncoder(w).Encode(res)
}
