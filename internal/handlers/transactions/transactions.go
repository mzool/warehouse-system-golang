package transactions

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"warehouse_system/internal/config"
	db "warehouse_system/internal/database/db"
	"warehouse_system/internal/handlers"
	"warehouse_system/internal/middlewares"

	"github.com/jackc/pgx/v5/pgtype"
)

type TransactionHandler struct {
	h *handlers.Handler
}

func NewTransactionHandler(h *handlers.Handler) *TransactionHandler {
	return &TransactionHandler{h: h}
}

type BatchAllocation struct {
	BatchID  int32   `json:"batch_id"`
	Quantity float64 `json:"quantity"`
}

type OpeningStockRequest struct {
	MaterialID      int32                  `json:"material_id"`
	WarehouseID     int32                  `json:"warehouse_id"`
	Quantity        float64                `json:"quantity"`
	UnitPrice       float64                `json:"unit_price"`
	ManufactureDate *string                `json:"manufacture_date,omitempty"`
	ExpiryDate      *string                `json:"expiry_date,omitempty"`
	Notes           *string                `json:"notes,omitempty"`
	Meta            map[string]interface{} `json:"meta,omitempty"`
}

type PurchaseReceiptRequest struct {
	MaterialID      int32                  `json:"material_id"`
	WarehouseID     int32                  `json:"warehouse_id"`
	SupplierID      *int32                 `json:"supplier_id,omitempty"`
	PurchaseOrderID *int32                 `json:"purchase_order_id,omitempty"`
	Quantity        float64                `json:"quantity"`
	UnitPrice       float64                `json:"unit_price"`
	ManufactureDate *string                `json:"manufacture_date,omitempty"`
	ExpiryDate      *string                `json:"expiry_date,omitempty"`
	Notes           *string                `json:"notes,omitempty"`
	Meta            map[string]interface{} `json:"meta,omitempty"`
}

type SaleRequest struct {
	SalesOrderID int32             `json:"sales_order_id"`
	WarehouseID  int32             `json:"warehouse_id"`
	MaterialID   int32             `json:"material_id"`
	Quantity     float64           `json:"quantity"`
	UseManual    bool              `json:"use_manual"`
	Batches      []BatchAllocation `json:"batches,omitempty"`
}

type TransferRequest struct {
	MaterialID      int32             `json:"material_id"`
	FromWarehouseID int32             `json:"from_warehouse_id"`
	ToWarehouseID   int32             `json:"to_warehouse_id"`
	Quantity        float64           `json:"quantity"`
	UseManual       bool              `json:"use_manual"`
	Batches         []BatchAllocation `json:"batches,omitempty"`
	Notes           *string           `json:"notes,omitempty"`
}

type ScrapRequest struct {
	MaterialID  int32             `json:"material_id"`
	WarehouseID int32             `json:"warehouse_id"`
	Quantity    float64           `json:"quantity"`
	Reason      string            `json:"reason"`
	UseManual   bool              `json:"use_manual"`
	Batches     []BatchAllocation `json:"batches,omitempty"`
}

type AdjustmentRequest struct {
	MaterialID  int32             `json:"material_id"`
	WarehouseID int32             `json:"warehouse_id"`
	Quantity    float64           `json:"quantity"`
	Direction   string            `json:"direction"`
	Reason      string            `json:"reason"`
	UnitPrice   *float64          `json:"unit_price,omitempty"`
	UseManual   bool              `json:"use_manual"`
	Batches     []BatchAllocation `json:"batches,omitempty"`
}

type TransactionResponse struct {
	Success    bool    `json:"success"`
	Message    string  `json:"message"`
	MovementID int32   `json:"movement_id,omitempty"`
	BatchIDs   []int32 `json:"batch_ids,omitempty"`
}

//////////////////////////////////////////////////////
// HELPER FUNCTIONS
//////////////////////////////////////////////////////

// Helper function to safely get string value from pointer
func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Helper function to safely get int32 value from pointer
func int32Value(i *int32) int32 {
	if i == nil {
		return 0
	}
	return *i
}

// Helper function to parse date string to pgtype.Date
func parseDate(dateStr *string) pgtype.Date {
	if dateStr == nil || *dateStr == "" {
		return pgtype.Date{Valid: false}
	}
	t, err := time.Parse("2006-01-02", *dateStr)
	if err != nil {
		return pgtype.Date{Valid: false}
	}
	return pgtype.Date{Time: t, Valid: true}
}

// decimalFromFloat converts float64 to pgtype.Numeric without manual scaling
// PostgreSQL NUMERIC type handles decimals natively
func decimalFromFloat(f float64) pgtype.Numeric {
	return pgtype.Numeric{
		Int:   new(big.Int).SetInt64(int64(f * 100)),
		Exp:   -2,
		Valid: true,
	}
}

func generateBatchNumber(
	ctx context.Context,
	queries *db.Queries,
	materialID int32,
	batchType string,
) (string, error) {

	year := time.Now().Year()

	if batchType == "open" {
		return fmt.Sprintf("%d/%d/open", materialID, year), nil
	}

	lastBatch, err := queries.GetLastBatchNumberForMaterial(ctx, pgtype.Int4{Int32: materialID, Valid: true})
	if err != nil && err != sql.ErrNoRows {
		return "", fmt.Errorf("failed to get last batch: %w", err)
	}

	counter := 1

	if lastBatch != "" {
		parts := strings.Split(lastBatch, "/")
		if len(parts) == 3 {
			lastCounter, _ := strconv.Atoi(parts[2])
			counter = lastCounter + 1
		}
	}

	return fmt.Sprintf("%d/%d/%d", materialID, year, counter), nil
}

//////////////////////////////////////////////////////
// OPENING STOCK HANDLER
//////////////////////////////////////////////////////

func (th *TransactionHandler) OpeningStock(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get user session from context
	session, ok := middlewares.GetSessionFromContext(r)
	if !ok {
		config.RespondJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized - Authentication required"})
		return
	}

	// Parse user ID from session
	var userID int32
	_, err := fmt.Sscanf(session.UserID, "%d", &userID)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid user ID"})
		return
	}

	var req OpeningStockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	if req.Quantity <= 0 {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Quantity must be positive"})
		return
	}

	tx, err := th.h.DB.Begin(ctx)
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback(ctx)

	queries := th.h.Queries.WithTx(tx)

	exists, err := queries.CheckOpeningStockExists(ctx, pgtype.Int4{Int32: req.MaterialID, Valid: true})
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to check existing opening stock"})
		return
	}

	if exists {
		config.RespondJSON(w, http.StatusConflict, map[string]string{"error": "Opening stock already exists for this material"})
		return
	}

	batchNumber, err := generateBatchNumber(ctx, queries, req.MaterialID, "open")
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to generate batch number"})
		return
	}

	// Create stock movement
	movement, err := queries.CreateStockMovement(ctx, db.CreateStockMovementParams{
		MaterialID:      pgtype.Int4{Int32: req.MaterialID, Valid: true},
		FromWarehouseID: pgtype.Int4{Valid: false},
		ToWarehouseID:   pgtype.Int4{Int32: req.WarehouseID, Valid: true},
		Quantity:        decimalFromFloat(req.Quantity),
		StockDirection:  db.StockDirectionIN,
		MovementType:    db.StockMovementTypeOPENING,
		Reference:       pgtype.Text{Valid: false},
		PerformedBy:     pgtype.Int4{Int32: userID, Valid: true},
		MovementDate:    pgtype.Timestamptz{Time: time.Now(), Valid: true},
		Notes:           pgtype.Text{String: stringValue(req.Notes), Valid: req.Notes != nil && *req.Notes != ""},
	})
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create stock movement"})
		return
	}

	// Prepare metadata
	var metaJSON []byte
	if req.Meta != nil {
		metaJSON, _ = json.Marshal(req.Meta)
	}

	// Create batch
	batch, err := queries.CreateBatch(ctx, db.CreateBatchParams{
		MaterialID:      pgtype.Int4{Int32: req.MaterialID, Valid: true},
		SupplierID:      pgtype.Int4{Valid: false},
		WarehouseID:     pgtype.Int4{Int32: req.WarehouseID, Valid: true},
		MovementID:      pgtype.Int4{Int32: movement.ID, Valid: true},
		UnitPrice:       decimalFromFloat(req.UnitPrice),
		BatchNumber:     batchNumber,
		ManufactureDate: parseDate(req.ManufactureDate),
		ExpiryDate:      parseDate(req.ExpiryDate),
		StartQuantity:   decimalFromFloat(req.Quantity),
		CurrentQuantity: decimalFromFloat(req.Quantity),
		Notes:           pgtype.Text{String: stringValue(req.Notes), Valid: req.Notes != nil && *req.Notes != ""},
		Meta:            metaJSON,
	})
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create batch"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to commit transaction"})
		return
	}

	config.RespondJSON(w, http.StatusCreated, TransactionResponse{
		Success:    true,
		Message:    "Opening stock recorded successfully",
		MovementID: movement.ID,
		BatchIDs:   []int32{batch.ID},
	})
}

//////////////////////////////////////////////////////
// PURCHASE RECEIPT HANDLER
//////////////////////////////////////////////////////

func (th *TransactionHandler) PurchaseReceipt(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get user session from context
	session, ok := middlewares.GetSessionFromContext(r)
	if !ok {
		config.RespondJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized - Authentication required"})
		return
	}

	// Parse user ID from session
	var userID int32
	_, err := fmt.Sscanf(session.UserID, "%d", &userID)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid user ID"})
		return
	}

	var req PurchaseReceiptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	if req.Quantity <= 0 {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Quantity must be positive"})
		return
	}

	tx, err := th.h.DB.Begin(ctx)
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback(ctx)

	queries := th.h.Queries.WithTx(tx)

	batchNumber, err := generateBatchNumber(ctx, queries, req.MaterialID, "purchase")
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to generate batch number"})
		return
	}

	// Create stock movement
	var poRef string
	if req.PurchaseOrderID != nil && *req.PurchaseOrderID != 0 {
		poRef = fmt.Sprintf("PO-%d", *req.PurchaseOrderID)
	}
	movement, err := queries.CreateStockMovement(ctx, db.CreateStockMovementParams{
		MaterialID:      pgtype.Int4{Int32: req.MaterialID, Valid: true},
		FromWarehouseID: pgtype.Int4{Valid: false},
		ToWarehouseID:   pgtype.Int4{Int32: req.WarehouseID, Valid: true},
		Quantity:        decimalFromFloat(req.Quantity),
		StockDirection:  db.StockDirectionIN,
		MovementType:    db.StockMovementTypePURCHASERECEIPT,
		Reference:       pgtype.Text{String: poRef, Valid: poRef != ""},
		PerformedBy:     pgtype.Int4{Int32: userID, Valid: true},
		MovementDate:    pgtype.Timestamptz{Time: time.Now(), Valid: true},
		Notes:           pgtype.Text{String: stringValue(req.Notes), Valid: req.Notes != nil && *req.Notes != ""},
	})
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create stock movement"})
		return
	}

	// Prepare metadata
	var metaJSON []byte
	if req.Meta != nil {
		metaJSON, _ = json.Marshal(req.Meta)
	}

	// Create batch
	batch, err := queries.CreateBatch(ctx, db.CreateBatchParams{
		MaterialID:      pgtype.Int4{Int32: req.MaterialID, Valid: true},
		SupplierID:      pgtype.Int4{Int32: int32Value(req.SupplierID), Valid: req.SupplierID != nil && *req.SupplierID != 0},
		WarehouseID:     pgtype.Int4{Int32: req.WarehouseID, Valid: true},
		MovementID:      pgtype.Int4{Int32: movement.ID, Valid: true},
		UnitPrice:       decimalFromFloat(req.UnitPrice),
		BatchNumber:     batchNumber,
		ManufactureDate: parseDate(req.ManufactureDate),
		ExpiryDate:      parseDate(req.ExpiryDate),
		StartQuantity:   decimalFromFloat(req.Quantity),
		CurrentQuantity: decimalFromFloat(req.Quantity),
		Notes:           pgtype.Text{String: stringValue(req.Notes), Valid: req.Notes != nil && *req.Notes != ""},
		Meta:            metaJSON,
	})
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create batch"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to commit transaction"})
		return
	}

	config.RespondJSON(w, http.StatusCreated, TransactionResponse{
		Success:    true,
		Message:    "Purchase receipt recorded successfully",
		MovementID: movement.ID,
		BatchIDs:   []int32{batch.ID},
	})
}
