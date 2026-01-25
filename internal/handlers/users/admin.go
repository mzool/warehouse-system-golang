package users

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"warehouse_system/internal/config"
	db "warehouse_system/internal/database/db"
	"warehouse_system/internal/handlers"
	"warehouse_system/internal/middlewares"
	"warehouse_system/internal/security"

	"github.com/jackc/pgx/v5/pgtype"
)

type CreateUserRequest struct {
	Username string `json:"username"`
	FullName string `json:"full_name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type UserHandler struct {
	h *handlers.Handler
}

func NewUserHandler(h *handlers.Handler) *UserHandler {
	return &UserHandler{h: h}
}

// CreateNewUser handles the creation of a new user for admin only.
func (u *UserHandler) CreateNewUser(w http.ResponseWriter, r *http.Request) {
	userData := CreateUserRequest{}
	err := json.NewDecoder(r.Body).Decode(&userData)
	if err != nil {
		config.RespondBadRequest(w, "Invalid request payload", err.Error())
		return
	}
	// check uniqueness of username and email can be added here
	// I wont check here i will depend on database constraints for simplicity this is internal system
	// so it is ok to send back database errors for admin users

	// hash password
	hashed, err := security.HashPassword(userData.Password)
	if err != nil {
		config.RespondJSON(w, 500, "failed to hash user password")
		return
	}
	userData.Password = hashed
	user, err := u.h.Queries.CreateUser(context.Background(), db.CreateUserParams{
		Username:     userData.Username,
		FullName:     pgtype.Text{String: userData.FullName, Valid: true},
		Email:        userData.Email,
		PasswordHash: userData.Password,
		Role:         db.UserRole(userData.Role),
	})

	// we can make this more sophisticated by checking for specific errors
	// like unique violation and respond accordingly
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	config.RespondJSON(w, http.StatusCreated, user)
}

// DeleteUser handles the deletion of a user by admin.
func (u *UserHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	// Get user ID from URL parameter (assuming router provides it)
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing user ID", "")
		return
	}

	var id int32
	_, err := fmt.Sscanf(idStr, "%d", &id)
	if err != nil {
		config.RespondBadRequest(w, "Invalid user ID format", err.Error())
		return
	}

	err = u.h.Queries.DeleteUser(context.Background(), id)
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]string{"message": "User deleted successfully"})
}

// ListAllUsers handles listing all users for admin.
func (u *UserHandler) ListAllUsers(w http.ResponseWriter, r *http.Request) {
	// Get pagination from context (set by pagination middleware)
	// If middleware not applied, use defaults
	pagination := middlewares.GetPagination(r.Context())
	limit, offset := pagination.GetSQLLimitOffset()

	users, err := u.h.Queries.ListUsers(context.Background(), db.ListUsersParams{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Get total count for pagination metadata
	total, err := u.h.Queries.CountUsers(context.Background())
	if err != nil {
		u.h.Logger.Error("Failed to count users", "error", err)
		// Continue without total count
	}

	// Set total and build pagination metadata
	pagination.SetTotal(total)

	response := map[string]interface{}{
		"users":      users,
		"pagination": pagination.BuildMeta(),
	}

	config.RespondJSON(w, http.StatusOK, response)
}

// UpdateUserRole handles updating a user's role by admin.
func (u *UserHandler) UpdateUserRole(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing user ID", "")
		return
	}

	var id int32
	_, err := fmt.Sscanf(idStr, "%d", &id)
	if err != nil {
		config.RespondBadRequest(w, "Invalid user ID format", err.Error())
		return
	}

	var req struct {
		Role string `json:"role"`
	}

	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		config.RespondBadRequest(w, "Invalid request payload", err.Error())
		return
	}

	if req.Role == "" {
		config.RespondBadRequest(w, "Role is required", "")
		return
	}

	// Get current user to preserve other fields
	currentUser, err := u.h.Queries.GetUserByID(context.Background(), id)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, "User not found")
		return
	}

	// Update user with new role
	updatedUser, err := u.h.Queries.UpdateUser(context.Background(), db.UpdateUserParams{
		ID:       id,
		Username: currentUser.Username,
		Email:    currentUser.Email,
		FullName: currentUser.FullName,
		Role:     db.UserRole(req.Role),
		IsActive: currentUser.IsActive,
	})
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	config.RespondJSON(w, http.StatusOK, updatedUser)
}

// ViewUserDetails handles viewing details of a specific user by admin.
func (u *UserHandler) ViewUserDetails(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing user ID", "")
		return
	}

	var id int32
	_, err := fmt.Sscanf(idStr, "%d", &id)
	if err != nil {
		config.RespondBadRequest(w, "Invalid user ID format", err.Error())
		return
	}

	user, err := u.h.Queries.GetUserByID(context.Background(), id)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, "User not found")
		return
	}

	config.RespondJSON(w, http.StatusOK, user)
}

// UpdateUserProfile handles updating a user's profile by admin.
func (u *UserHandler) UpdateUserProfile(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing user ID", "")
		return
	}

	var id int32
	_, err := fmt.Sscanf(idStr, "%d", &id)
	if err != nil {
		config.RespondBadRequest(w, "Invalid user ID format", err.Error())
		return
	}

	var req struct {
		Username string `json:"username"`
		FullName string `json:"full_name"`
		Email    string `json:"email"`
		IsActive *bool  `json:"is_active"`
	}

	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		config.RespondBadRequest(w, "Invalid request payload", err.Error())
		return
	}

	// Get current user to preserve unmodified fields
	currentUser, err := u.h.Queries.GetUserByID(context.Background(), id)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, "User not found")
		return
	}

	// Build update params with fallback to current values
	updateParams := db.UpdateUserParams{
		ID:       id,
		Username: currentUser.Username,
		Email:    currentUser.Email,
		FullName: currentUser.FullName,
		Role:     currentUser.Role,
		IsActive: currentUser.IsActive,
	}

	if req.Username != "" {
		updateParams.Username = req.Username
	}
	if req.Email != "" {
		updateParams.Email = req.Email
	}
	if req.FullName != "" {
		updateParams.FullName = pgtype.Text{String: req.FullName, Valid: true}
	}
	if req.IsActive != nil {
		updateParams.IsActive = pgtype.Bool{Bool: *req.IsActive, Valid: true}
	}

	updatedUser, err := u.h.Queries.UpdateUser(context.Background(), updateParams)
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	config.RespondJSON(w, http.StatusOK, updatedUser)
}
