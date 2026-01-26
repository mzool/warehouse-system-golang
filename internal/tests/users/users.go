package userstests

// import (
// 	"bytes"
// 	"context"
// 	"encoding/json"
// 	"net/http"
// 	"net/http/httptest"
// 	"testing"
// 	db "warehouse_system/internal/database/db"
// 	"warehouse_system/internal/handlers"
// 	"warehouse_system/internal/handlers/users"
// 	"warehouse_system/internal/security"

// 	"github.com/jackc/pgx/v5/pgtype"
// 	"github.com/jackc/pgx/v5/pgxpool"
// )

// // setupTestDB should be create a test database connection
// func setupTestDB(t *testing.T) (*pgxpool.Pool, *db.Queries) {
// 	t.Helper()
// 	// TODO: Implement actual test database setup
// 	// This would typically connect to a test database
// 	return nil, nil
// }

// // createTestUser creates a test user in the database
// func createTestUser(t *testing.T, queries *db.Queries, email, password, role string) db.CreateUserRow {
// 	t.Helper()

// 	hashedPassword, err := security.HashPassword(password)
// 	if err != nil {
// 		t.Fatalf("Failed to hash password: %v", err)
// 	}

// 	user, err := queries.CreateUser(context.Background(), db.CreateUserParams{
// 		Username:     "testuser",
// 		Email:        email,
// 		PasswordHash: hashedPassword,
// 		Role:         db.UserRole(role),
// 		FullName:     pgtype.Text{String: "Test User", Valid: true},
// 	})
// 	if err != nil {
// 		t.Fatalf("Failed to create test user: %v", err)
// 	}

// 	return user
// }

// func TestLoginUser_Success(t *testing.T) {
// 	// Setup
// 	dbPool, queries := setupTestDB(t)
// 	if dbPool == nil || queries == nil {
// 		t.Skip("Test database not configured")
// 	}
// 	defer dbPool.Close()

// 	// Create test user
// 	testEmail := "test@example.com"
// 	testPassword := "password123"
// 	createTestUser(t, queries, testEmail, testPassword, "user")

// 	// Create handler
// 	h := handlers.NewHandler(queries, nil, nil, dbPool)
// 	userHandler := users.NewUserHandler(h)

// 	// Prepare request
// 	loginReq := map[string]interface{}{
// 		"email":    testEmail,
// 		"password": testPassword,
// 		"remember": false,
// 	}
// 	body, _ := json.Marshal(loginReq)
// 	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBuffer(body))
// 	req.Header.Set("Content-Type", "application/json")
// 	w := httptest.NewRecorder()

// 	// Execute
// 	userHandler.Login(w, req)

// 	// Assert
// 	if w.Code != http.StatusOK {
// 		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
// 	}

// 	var response map[string]interface{}
// 	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
// 		t.Fatalf("Failed to decode response: %v", err)
// 	}

// 	if response["message"] != "Login successful" {
// 		t.Errorf("Expected 'Login successful', got '%v'", response["message"])
// 	}

// 	// Verify user data in response
// 	if userData, ok := response["user"].(map[string]interface{}); ok {
// 		if userData["email"] != testEmail {
// 			t.Errorf("Expected email %s, got %v", testEmail, userData["email"])
// 		}
// 	} else {
// 		t.Error("User data not found in response")
// 	}
// }

// func TestLoginUser_InvalidEmail(t *testing.T) {
// 	// Setup
// 	dbPool, queries := setupTestDB(t)
// 	if dbPool == nil || queries == nil {
// 		t.Skip("Test database not configured")
// 	}
// 	defer dbPool.Close()

// 	h := handlers.NewHandler(queries, nil, nil, dbPool)
// 	userHandler := users.NewUserHandler(h)

// 	// Prepare request with non-existent email
// 	loginReq := map[string]interface{}{
// 		"email":    "nonexistent@example.com",
// 		"password": "password123",
// 		"remember": false,
// 	}
// 	body, _ := json.Marshal(loginReq)
// 	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBuffer(body))
// 	req.Header.Set("Content-Type", "application/json")
// 	w := httptest.NewRecorder()

// 	// Execute
// 	userHandler.Login(w, req)

// 	// Assert
// 	if w.Code != http.StatusUnauthorized {
// 		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
// 	}

// 	var response map[string]interface{}
// 	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
// 		t.Fatalf("Failed to decode response: %v", err)
// 	}

// 	if response["error"] != "Invalid email or password" {
// 		t.Errorf("Expected 'Invalid email or password', got '%v'", response["error"])
// 	}
// }

// func TestLoginUser_InvalidPassword(t *testing.T) {
// 	// Setup
// 	dbPool, queries := setupTestDB(t)
// 	if dbPool == nil || queries == nil {
// 		t.Skip("Test database not configured")
// 	}
// 	defer dbPool.Close()

// 	// Create test user
// 	testEmail := "test@example.com"
// 	testPassword := "password123"
// 	createTestUser(t, queries, testEmail, testPassword, "user")

// 	h := handlers.NewHandler(queries, nil, nil, dbPool)
// 	userHandler := users.NewUserHandler(h)

// 	// Prepare request with wrong password
// 	loginReq := map[string]interface{}{
// 		"email":    testEmail,
// 		"password": "wrongpassword",
// 		"remember": false,
// 	}
// 	body, _ := json.Marshal(loginReq)
// 	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBuffer(body))
// 	req.Header.Set("Content-Type", "application/json")
// 	w := httptest.NewRecorder()

// 	// Execute
// 	userHandler.Login(w, req)

// 	// Assert
// 	if w.Code != http.StatusUnauthorized {
// 		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
// 	}

// 	var response map[string]interface{}
// 	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
// 		t.Fatalf("Failed to decode response: %v", err)
// 	}

// 	if response["error"] != "Invalid email or password" {
// 		t.Errorf("Expected 'Invalid email or password', got '%v'", response["error"])
// 	}
// }

// func TestLoginUser_MissingFields(t *testing.T) {
// 	// Setup
// 	dbPool, queries := setupTestDB(t)
// 	if dbPool == nil || queries == nil {
// 		t.Skip("Test database not configured")
// 	}
// 	defer dbPool.Close()

// 	h := handlers.NewHandler(queries, nil, nil, dbPool)
// 	userHandler := users.NewUserHandler(h)

// 	tests := []struct {
// 		name        string
// 		requestBody map[string]interface{}
// 		expectCode  int
// 	}{
// 		{
// 			name: "Missing email",
// 			requestBody: map[string]interface{}{
// 				"password": "password123",
// 			},
// 			expectCode: http.StatusBadRequest,
// 		},
// 		{
// 			name: "Missing password",
// 			requestBody: map[string]interface{}{
// 				"email": "test@example.com",
// 			},
// 			expectCode: http.StatusBadRequest,
// 		},
// 		{
// 			name: "Empty email",
// 			requestBody: map[string]interface{}{
// 				"email":    "",
// 				"password": "password123",
// 			},
// 			expectCode: http.StatusBadRequest,
// 		},
// 		{
// 			name: "Empty password",
// 			requestBody: map[string]interface{}{
// 				"email":    "test@example.com",
// 				"password": "",
// 			},
// 			expectCode: http.StatusBadRequest,
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			body, _ := json.Marshal(tt.requestBody)
// 			req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBuffer(body))
// 			req.Header.Set("Content-Type", "application/json")
// 			w := httptest.NewRecorder()

// 			userHandler.Login(w, req)

// 			if w.Code != tt.expectCode {
// 				t.Errorf("Expected status %d, got %d", tt.expectCode, w.Code)
// 			}
// 		})
// 	}
// }

// func TestLoginUser_InvalidJSON(t *testing.T) {
// 	// Setup
// 	dbPool, queries := setupTestDB(t)
// 	if dbPool == nil || queries == nil {
// 		t.Skip("Test database not configured")
// 	}
// 	defer dbPool.Close()

// 	h := handlers.NewHandler(queries, nil, nil, dbPool)
// 	userHandler := users.NewUserHandler(h)

// 	// Prepare request with invalid JSON
// 	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString("invalid json"))
// 	req.Header.Set("Content-Type", "application/json")
// 	w := httptest.NewRecorder()

// 	// Execute
// 	userHandler.Login(w, req)

// 	// Assert
// 	if w.Code != http.StatusBadRequest {
// 		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
// 	}
// }

// func TestLoginUser_InactiveAccount(t *testing.T) {
// 	// Setup
// 	dbPool, queries := setupTestDB(t)
// 	if dbPool == nil || queries == nil {
// 		t.Skip("Test database not configured")
// 	}
// 	defer dbPool.Close()

// 	// Create test user
// 	testEmail := "inactive@example.com"
// 	testPassword := "password123"
// 	user := createTestUser(t, queries, testEmail, testPassword, "user")

// 	// Deactivate user
// 	_, err := queries.UpdateUser(context.Background(), db.UpdateUserParams{
// 		ID:       user.ID,
// 		IsActive: pgtype.Bool{Bool: false, Valid: true},
// 	})
// 	if err != nil {
// 		t.Fatalf("Failed to deactivate user: %v", err)
// 	}

// 	h := handlers.NewHandler(queries, nil, nil, dbPool)
// 	userHandler := users.NewUserHandler(h)

// 	// Prepare request
// 	loginReq := map[string]interface{}{
// 		"email":    testEmail,
// 		"password": testPassword,
// 		"remember": false,
// 	}
// 	body, _ := json.Marshal(loginReq)
// 	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBuffer(body))
// 	req.Header.Set("Content-Type", "application/json")
// 	w := httptest.NewRecorder()

// 	// Execute
// 	userHandler.Login(w, req)

// 	// Assert
// 	if w.Code != http.StatusForbidden {
// 		t.Errorf("Expected status %d, got %d", http.StatusForbidden, w.Code)
// 	}

// 	var response map[string]interface{}
// 	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
// 		t.Fatalf("Failed to decode response: %v", err)
// 	}

// 	if response["error"] != "Account is deactivated" {
// 		t.Errorf("Expected 'Account is deactivated', got '%v'", response["error"])
// 	}
// }
