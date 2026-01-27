package users

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
	"warehouse_system/internal/config"
	db "warehouse_system/internal/database/db"
	"warehouse_system/internal/middlewares"
	"warehouse_system/internal/security"

	"github.com/jackc/pgx/v5/pgtype"
)

// UpdateProfileRequest represents the request payload for updating user profile
type UpdateProfileRequest struct {
	Username string `json:"username,omitempty"`
	FullName string `json:"full_name,omitempty"`
	Email    string `json:"email,omitempty"`
}

// ChangePasswordRequest represents the request payload for changing password
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// UpdateMyProfile handles updating the profile of the currently authenticated user.
func (u *UserHandler) UpdateMyProfile(w http.ResponseWriter, r *http.Request) {
	// Get user session from context (set by auth middleware)
	session, ok := middlewares.GetSessionFromContext(r)
	if !ok {
		config.RespondJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "User not authenticated",
		})
		return
	}

	// Parse user ID from session
	var userID int32
	_, err := fmt.Sscanf(session.UserID, "%d", &userID)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Invalid user ID",
		})
		return
	}

	// Parse request body
	var req UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondBadRequest(w, "Invalid request payload", err.Error())
		return
	}

	// Get current user data
	currentUser, err := u.h.Queries.GetUserByID(context.Background(), userID)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{
			"error": "User not found",
		})
		return
	}

	// Prepare update params with COALESCE logic
	updateParams := db.UpdateUserParams{
		ID: userID,
	}

	// Only update fields that are provided
	if req.Username != "" && req.Username != currentUser.Username {
		updateParams.Username = req.Username
	} else {
		updateParams.Username = currentUser.Username
	}

	if req.Email != "" && req.Email != currentUser.Email {
		updateParams.Email = req.Email
	} else {
		updateParams.Email = currentUser.Email
	}

	if req.FullName != "" {
		updateParams.FullName = pgtype.Text{String: req.FullName, Valid: true}
	} else {
		updateParams.FullName = currentUser.FullName
	}

	updateParams.Role = db.UserRole(currentUser.Role) // Role remains unchanged
	updateParams.IsActive = currentUser.IsActive

	// Update user
	updatedUser, err := u.h.Queries.UpdateUser(context.Background(), updateParams)
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{
			"error":   "Failed to update profile",
			"details": err.Error(),
		})
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Profile updated successfully",
		"user": map[string]interface{}{
			"id":         updatedUser.ID,
			"username":   updatedUser.Username,
			"email":      updatedUser.Email,
			"full_name":  updatedUser.FullName,
			"role":       updatedUser.Role,
			"is_active":  updatedUser.IsActive,
			"updated_at": updatedUser.UpdatedAt,
		},
	})
}

// ViewMyProfile handles viewing the profile of the currently authenticated user.
func (u *UserHandler) ViewMyProfile(w http.ResponseWriter, r *http.Request) {
	// Get user session from context
	session, ok := middlewares.GetSessionFromContext(r)
	if !ok {
		config.RespondJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "User not authenticated",
		})
		return
	}

	// Parse user ID from session
	var userID int32
	_, err := fmt.Sscanf(session.UserID, "%d", &userID)
	if err != nil {
		u.h.Logger.Error("Failed to parse user ID", "error", err, "user_id_string", session.UserID)
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Invalid user ID",
		})
		return
	}

	// Get user from database
	user, err := u.h.Queries.GetUserByID(context.Background(), userID)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{
			"error": "User not found",
		})
		return
	}

	// Return user profile (exclude sensitive data)
	config.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"id":         user.ID,
		"username":   user.Username,
		"email":      user.Email,
		"full_name":  user.FullName,
		"role":       user.Role,
		"is_active":  user.IsActive,
		"created_at": user.CreatedAt,
		"updated_at": user.UpdatedAt,
	})
}

// ChangeMyPassword handles changing the password of the currently authenticated user.
func (u *UserHandler) ChangeMyPassword(w http.ResponseWriter, r *http.Request) {
	// Get user session from context
	session, ok := middlewares.GetSessionFromContext(r)
	if !ok {
		config.RespondJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "User not authenticated",
		})
		return
	}

	// Parse user ID from session
	var userID int32
	_, err := fmt.Sscanf(session.UserID, "%d", &userID)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Invalid user ID",
		})
		return
	}

	// Parse request body
	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondBadRequest(w, "Invalid request payload", err.Error())
		return
	}

	// Validate input
	if req.CurrentPassword == "" || req.NewPassword == "" {
		config.RespondBadRequest(w, "All password fields are required", "")
		return
	}

	// Validate password strength
	if len(req.NewPassword) < 8 {
		config.RespondBadRequest(w, "New password must be at least 8 characters long", "")
		return
	}

	// Get user password hash
	var userHash string
	err = u.h.DB.QueryRow(r.Context(), `
		SELECT password_hash FROM users WHERE id = $1
	`, userID).Scan(&userHash)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{
			"error": "User not found",
		})
		return
	}

	// Verify current password - IMPORTANT: parameters are (password, hash)
	correct, err := security.VerifyPassword(req.CurrentPassword, userHash)
	if err != nil || !correct {
		config.RespondJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "Current password is incorrect",
		})
		return
	}

	// Hash new password
	hashedPassword, err := security.HashPassword(req.NewPassword)
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Failed to hash password",
		})
		return
	}

	// Update password
	err = u.h.Queries.UpdateUserPassword(context.Background(), db.UpdateUserPasswordParams{
		ID:           userID,
		PasswordHash: hashedPassword,
	})
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Failed to update password",
		})
		return
	}

	// Revoke ALL sessions for this user for security
	// This forces re-login on all devices with the new password
	// Prevents session hijacking if password was changed due to compromise
	err = middlewares.RevokeUserSessions(r.Context(), &middlewares.SessionConfig{
		Cache:            u.h.Cache,
		SecretKey:        []byte(u.h.CFG.Auth.SessionSecret),
		SessionKeyPrefix: "session:",
		Logger:           u.h.Logger,
	}, session.UserID)
	if err != nil {
		u.h.Logger.Error("Failed to revoke user sessions after password change", "error", err, "user_id", userID)
	}

	// Clear the current session cookie
	middlewares.DeleteSessionCookie(w, &middlewares.SessionConfig{
		CookieName:     "session",
		CookiePath:     "/",
		CookieSecure:   true,
		CookieHTTPOnly: true,
		CookieSameSite: http.SameSiteLaxMode,
	})

	u.h.Logger.Info("Password changed successfully - all sessions revoked", "user_id", userID)

	config.RespondJSON(w, http.StatusOK, map[string]string{
		"message": "Password changed successfully. All sessions have been logged out. Please login again with your new password.",
	})
}

// LoginRequest represents the login request payload
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Remember bool   `json:"remember"`
}

// Login handles user login.
func (u *UserHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondBadRequest(w, "Invalid request payload", err.Error())
		return
	}

	// Validate input
	if req.Email == "" || req.Password == "" {
		config.RespondBadRequest(w, "Email and password are required", "")
		return
	}

	// Get user auth
	userAuth, err := u.h.Queries.GetUserAuth(context.Background(), req.Email)
	if err != nil {
		u.h.Logger.Warn("Login attempt with unknown email", "email", req.Email)
		config.RespondJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "Invalid email or password",
		})
		return
	}

	// Verify password
	correct, err := security.VerifyPassword(req.Password, userAuth.PasswordHash)
	if err != nil {
		u.h.Logger.Error("Password verification error", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Internal server error, please try again later",
		})
		return
	}

	u.h.Logger.Info("Password verification result", "correct", correct)

	if !correct {
		u.h.Logger.Warn("Invalid password attempt", "email", req.Email)
		config.RespondJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "Invalid email or password",
		})
		return
	}

	// Check if user is active
	if !userAuth.IsActive.Bool {
		config.RespondJSON(w, http.StatusForbidden, map[string]string{
			"error": "Account is deactivated",
		})
		return
	}

	ipAddress := middlewares.GetIp(r)
	userAgent := r.UserAgent()
	// log the login attempt
	u.h.Queries.LogAudit(r.Context(), db.LogAuditParams{
		UserID:   pgtype.Int4{Int32: int32(userAuth.ID), Valid: true},
		Username: pgtype.Text{String: userAuth.Username, Valid: true},
		Action:   "login",
		Entity:   "users",
		Details:  []byte(fmt.Sprintf(`{"ip_address": "%s", "user_agent": "%s"}`, ipAddress, userAgent)),
	})

	sessionDuration := 24 * time.Hour
	if req.Remember {
		sessionDuration = 7 * 24 * time.Hour // 7 days
	}
	// Create session
	session, err := middlewares.NewSession(r.Context(), &middlewares.SimpleSessionCreator{
		Cache:            u.h.Cache,
		SecretKey:        []byte(u.h.CFG.Auth.SessionSecret),
		SessionKeyPrefix: "session:",
		SessionDuration:  sessionDuration,
		Logger:           u.h.Logger,
	}, strconv.Itoa(int(userAuth.ID)), userAuth.Email, userAuth.Username, 2, r)
	if err != nil {
		u.h.Logger.Error("Failed to create session", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Failed to create user session",
		})
		return
	}

	// Set session cookie
	middlewares.SetSessionCookie(w, &middlewares.SessionConfig{
		CookieName:      "session",
		CookiePath:      "/",
		CookieSecure:    true,
		CookieHTTPOnly:  true,
		CookieSameSite:  http.SameSiteLaxMode,
		SessionDuration: sessionDuration,
	}, session)

	// log
	u.h.Logger.Info("User logged in successfully", "email", req.Email, "user_id", userAuth.ID)

	config.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Login successful",
		"user": map[string]interface{}{
			"id":        userAuth.ID,
			"username":  userAuth.Username,
			"email":     userAuth.Email,
			"is_active": userAuth.IsActive.Bool,
			"full_name": userAuth.FullName,
			"role":      userAuth.Role,
		},
	})
}

func (u *UserHandler) GetUserPermissionsRoles(ctx context.Context, userId string) (roles []string, permissions []string, authVersion int, err error) {
	var role string
	u.h.DB.QueryRow(ctx, `SELECT role from users where id = 1$`, userId).Scan(&role)

	return roles, permissions, 1, nil

}

// Logout handles user logout.
func (u *UserHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Get user session from context
	session, ok := middlewares.GetSessionFromContext(r)
	if !ok {
		config.RespondJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "User not authenticated",
		})
		return
	}

	// Revoke session
	err := middlewares.RevokeSession(r.Context(), &middlewares.SessionConfig{
		Cache:            u.h.Cache,
		SecretKey:        []byte(u.h.CFG.Auth.SessionSecret),
		SessionKeyPrefix: "session:",
		Logger:           u.h.Logger,
	}, session.SessionID)
	if err != nil {
		u.h.Logger.Error("Failed to revoke session", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Failed to revoke user session",
		})
		return
	}

	// Clear session cookie
	middlewares.DeleteSessionCookie(w, &middlewares.SessionConfig{
		CookieName:     "session",
		CookiePath:     "/",
		CookieSecure:   true,
		CookieHTTPOnly: true,
		CookieSameSite: http.SameSiteLaxMode,
	})

	config.RespondJSON(w, http.StatusOK, map[string]string{
		"message": "Logout successful",
	})
}
