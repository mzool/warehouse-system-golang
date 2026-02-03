package transactions

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"warehouse_system/internal/config"
	db "warehouse_system/internal/database/db"
	"warehouse_system/internal/middlewares"
)

// =====================================================
// ADDITIONAL REQUEST TYPES
// =====================================================

type CustomerReturnRequest struct {
	SalesOrderID int32   `json:"sales_order_id"`
	MaterialID   int32   `json:"material_id"`
	Quantity     float64 `json:"quantity"`
	Notes        *string `json:"notes,omitempty"`
}

// =====================================================
// HELPER FUNCTIONS FOR BATCH ALLOCATION
// =====================================================

func getValuationMethod(ctx context.Context, queries *db.Queries, materialID, warehouseID int32) (string, error) {
	method, err := queries.GetMaterialValuationMethod(ctx, db.GetMaterialValuationMethodParams{
		ID:   materialID,
		ID_2: warehouseID,
	})
	if err != nil {
		return "", fmt.Errorf("failed to get valuation method: %w", err)
	}

	if method != "" {
		return string(method), nil
	}
	return "FIFO", nil // default
}

func allocateBatchesAuto(ctx context.Context, queries *db.Queries, materialID, warehouseID int32, quantity float64, valuationMethod string) ([]BatchAllocation, error) {
	var batches []db.Batch
	var err error

	if valuationMethod == "LIFO" {
		batches, err = queries.GetBatchesByWarehouseAndMaterialLIFO(ctx, db.GetBatchesByWarehouseAndMaterialLIFOParams{
			WarehouseID: pgtype.Int4{Int32: warehouseID, Valid: true},
			MaterialID:  pgtype.Int4{Int32: materialID, Valid: true},
		})
	} else {
		// FIFO or Weighted Average (use FIFO for allocation)
		batches, err = queries.GetBatchesByWarehouseAndMaterial(ctx, db.GetBatchesByWarehouseAndMaterialParams{
			WarehouseID: pgtype.Int4{Int32: warehouseID, Valid: true},
			MaterialID:  pgtype.Int4{Int32: materialID, Valid: true},
		})
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get batches: %w", err)
	}

	allocations := []BatchAllocation{}
	remaining := quantity

	for _, batch := range batches {
		if remaining <= 0 {
			break
		}

		// Convert pgtype.Numeric to float64
		batchQty, _ := batch.CurrentQuantity.Float64Value()
		allocQty := remaining
		if allocQty > batchQty.Float64 {
			allocQty = batchQty.Float64
		}

		allocations = append(allocations, BatchAllocation{
			BatchID:  batch.ID,
			Quantity: allocQty,
		})

		remaining -= allocQty
	}

	if remaining > 0 {
		return nil, fmt.Errorf("insufficient stock: need %.2f more units", remaining)
	}

	return allocations, nil
}

func validateBatchAllocations(ctx context.Context, queries *db.Queries, batches []BatchAllocation, totalQuantity float64) error {
	if len(batches) == 0 {
		return fmt.Errorf("no batches provided")
	}

	// Get batch IDs
	batchIDs := make([]int32, len(batches))
	for i, b := range batches {
		batchIDs[i] = b.BatchID
	}

	// Fetch actual batches
	actualBatches, err := queries.GetBatchesByIDs(ctx, batchIDs)
	if err != nil {
		return fmt.Errorf("failed to fetch batches: %w", err)
	}

	if len(actualBatches) != len(batches) {
		return fmt.Errorf("some batches not found")
	}

	// Create map for quick lookup
	batchMap := make(map[int32]db.Batch)
	for _, b := range actualBatches {
		batchMap[b.ID] = b
	}

	// Validate each allocation
	allocatedTotal := 0.0
	for _, alloc := range batches {
		batch, exists := batchMap[alloc.BatchID]
		if !exists {
			return fmt.Errorf("batch %d not found", alloc.BatchID)
		}

		if alloc.Quantity <= 0 {
			return fmt.Errorf("batch %d: quantity must be positive", alloc.BatchID)
		}

		// Convert pgtype.Numeric to float64
		currentQty, _ := batch.CurrentQuantity.Float64Value()
		if alloc.Quantity > currentQty.Float64 {
			return fmt.Errorf("batch %d: insufficient quantity (available: %.2f, requested: %.2f)",
				alloc.BatchID, currentQty.Float64, alloc.Quantity)
		}

		allocatedTotal += alloc.Quantity
	}

	// Allow small floating point differences
	if allocatedTotal < totalQuantity-0.0001 || allocatedTotal > totalQuantity+0.0001 {
		return fmt.Errorf("allocated quantity (%.2f) does not match requested quantity (%.2f)",
			allocatedTotal, totalQuantity)
	}

	return nil
}

// =====================================================
// SALE
// =====================================================

func (th *TransactionHandler) Sale(w http.ResponseWriter, r *http.Request) {
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

	var req SaleRequest
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

	// Get batch allocations
	var allocations []BatchAllocation
	if req.UseManual {
		if err := validateBatchAllocations(ctx, queries, req.Batches, req.Quantity); err != nil {
			config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		allocations = req.Batches
	} else {
		valuationMethod, err := getValuationMethod(ctx, queries, req.MaterialID, req.WarehouseID)
		if err != nil {
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to get valuation method"})
			return
		}

		allocations, err = allocateBatchesAuto(ctx, queries, req.MaterialID, req.WarehouseID, req.Quantity, valuationMethod)
		if err != nil {
			config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}

	// Create stock movement
	reference := fmt.Sprintf("SO-%d", req.SalesOrderID)
	movement, err := queries.CreateStockMovement(ctx, db.CreateStockMovementParams{
		MaterialID:      pgtype.Int4{Int32: req.MaterialID, Valid: true},
		FromWarehouseID: pgtype.Int4{Int32: req.WarehouseID, Valid: true},
		Quantity:        decimalFromFloat(req.Quantity),
		StockDirection:  db.StockDirectionOUT,
		MovementType:    db.StockMovementTypeSALE,
		Reference:       pgtype.Text{String: reference, Valid: true},
		PerformedBy:     pgtype.Int4{Int32: userID, Valid: true},
		MovementDate:    pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create stock movement"})
		return
	}

	// Update batches
	for _, alloc := range allocations {
		_, err := queries.UpdateBatchQuantity(ctx, db.UpdateBatchQuantityParams{
			ID:              alloc.BatchID,
			CurrentQuantity: decimalFromFloat(-alloc.Quantity),
		})
		if err != nil {
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update batch quantity"})
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to commit transaction"})
		return
	}

	batchIDs := make([]int32, len(allocations))
	for i, a := range allocations {
		batchIDs[i] = a.BatchID
	}

	config.RespondJSON(w, http.StatusCreated, TransactionResponse{
		Success:    true,
		Message:    "Sale recorded successfully",
		MovementID: movement.ID,
		BatchIDs:   batchIDs,
	})
}

// =====================================================
// CUSTOMER RETURN
// =====================================================

func (th *TransactionHandler) CustomerReturn(w http.ResponseWriter, r *http.Request) {
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

	var req CustomerReturnRequest
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

	// Get original sale movements and batches
	saleReference := fmt.Sprintf("SO-%d", req.SalesOrderID)
	saleMovements, err := queries.GetStockMovementsByReference(ctx, pgtype.Text{String: saleReference, Valid: true})
	if err != nil || len(saleMovements) == 0 {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Original sale not found"})
		return
	}

	// Find the sale movement for this material
	var originalMovementID int32
	var originalWarehouseID int32
	found := false
	for _, sm := range saleMovements {
		if sm.MaterialID.Valid && sm.MaterialID.Int32 == req.MaterialID {
			originalMovementID = sm.ID
			if sm.FromWarehouseID.Valid {
				originalWarehouseID = sm.FromWarehouseID.Int32
			}
			found = true
			break
		}
	}

	if !found {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Material not found in original sale"})
		return
	}

	// Create return movement
	var notes pgtype.Text
	if req.Notes != nil {
		notes = pgtype.Text{String: *req.Notes, Valid: true}
	}

	returnReference := fmt.Sprintf("RETURN-SO-%d-M%d", req.SalesOrderID, req.MaterialID)
	movement, err := queries.CreateStockMovement(ctx, db.CreateStockMovementParams{
		MaterialID:     pgtype.Int4{Int32: req.MaterialID, Valid: true},
		ToWarehouseID:  pgtype.Int4{Int32: originalWarehouseID, Valid: true},
		Quantity:       decimalFromFloat(req.Quantity),
		StockDirection: db.StockDirectionIN,
		MovementType:   db.StockMovementTypeCUSTOMERRETURN,
		Reference:      pgtype.Text{String: returnReference, Valid: true},
		PerformedBy:    pgtype.Int4{Int32: userID, Valid: true},
		MovementDate:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
		Notes:          notes,
	})
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create return movement"})
		return
	}

	// Note: For simplicity, we're not tracking which specific batches to return to
	// In a more sophisticated system, you would store batch allocations per sale
	// and return to the exact same batches. For now, we'll create a new batch.

	batchNumber, err := generateBatchNumber(ctx, queries, req.MaterialID, "return")
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to generate batch number"})
		return
	}

	batch, err := queries.CreateBatch(ctx, db.CreateBatchParams{
		MaterialID:      pgtype.Int4{Int32: req.MaterialID, Valid: true},
		WarehouseID:     pgtype.Int4{Int32: originalWarehouseID, Valid: true},
		MovementID:      pgtype.Int4{Int32: movement.ID, Valid: true},
		UnitPrice:       decimalFromFloat(0), // Return pricing could be different
		BatchNumber:     batchNumber,
		StartQuantity:   decimalFromFloat(req.Quantity),
		CurrentQuantity: decimalFromFloat(req.Quantity),
		Notes:           pgtype.Text{String: fmt.Sprintf("Return from sale order %d (movement %d)", req.SalesOrderID, originalMovementID), Valid: true},
	})
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create return batch"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to commit transaction"})
		return
	}

	config.RespondJSON(w, http.StatusCreated, TransactionResponse{
		Success:    true,
		Message:    "Customer return recorded successfully",
		MovementID: movement.ID,
		BatchIDs:   []int32{batch.ID},
	})
}

// =====================================================
// TRANSFER
// =====================================================

func (th *TransactionHandler) Transfer(w http.ResponseWriter, r *http.Request) {
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

	var req TransferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	if req.Quantity <= 0 {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Quantity must be positive"})
		return
	}

	if req.FromWarehouseID == req.ToWarehouseID {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Source and destination warehouses must be different"})
		return
	}

	tx, err := th.h.DB.Begin(ctx)
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback(ctx)

	queries := th.h.Queries.WithTx(tx)

	// Get batch allocations for transfer out
	var allocations []BatchAllocation
	if req.UseManual {
		if err := validateBatchAllocations(ctx, queries, req.Batches, req.Quantity); err != nil {
			config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		allocations = req.Batches
	} else {
		valuationMethod, err := getValuationMethod(ctx, queries, req.MaterialID, req.FromWarehouseID)
		if err != nil {
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to get valuation method"})
			return
		}

		allocations, err = allocateBatchesAuto(ctx, queries, req.MaterialID, req.FromWarehouseID, req.Quantity, valuationMethod)
		if err != nil {
			config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}

	// Create transfer out movement
	var notes pgtype.Text
	if req.Notes != nil {
		notes = pgtype.Text{String: *req.Notes, Valid: true}
	}

	transferRef := fmt.Sprintf("TRANSFER-%d-%d-M%d-%d", req.FromWarehouseID, req.ToWarehouseID, req.MaterialID, time.Now().Unix())

	movementOut, err := queries.CreateStockMovement(ctx, db.CreateStockMovementParams{
		MaterialID:      pgtype.Int4{Int32: req.MaterialID, Valid: true},
		FromWarehouseID: pgtype.Int4{Int32: req.FromWarehouseID, Valid: true},
		ToWarehouseID:   pgtype.Int4{Int32: req.ToWarehouseID, Valid: true},
		Quantity:        decimalFromFloat(req.Quantity),
		StockDirection:  db.StockDirectionOUT,
		MovementType:    db.StockMovementTypeTRANSFEROUT,
		Reference:       pgtype.Text{String: transferRef, Valid: true},
		PerformedBy:     pgtype.Int4{Int32: userID, Valid: true},
		MovementDate:    pgtype.Timestamptz{Time: time.Now(), Valid: true},
		Notes:           notes,
	})
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create transfer out movement"})
		return
	}

	// Update source batches
	for _, alloc := range allocations {
		_, err := queries.UpdateBatchQuantity(ctx, db.UpdateBatchQuantityParams{
			ID:              alloc.BatchID,
			CurrentQuantity: decimalFromFloat(-alloc.Quantity),
		})
		if err != nil {
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update source batch"})
			return
		}
	}

	// Create transfer in movement
	movementIn, err := queries.CreateStockMovement(ctx, db.CreateStockMovementParams{
		MaterialID:      pgtype.Int4{Int32: req.MaterialID, Valid: true},
		FromWarehouseID: pgtype.Int4{Int32: req.FromWarehouseID, Valid: true},
		ToWarehouseID:   pgtype.Int4{Int32: req.ToWarehouseID, Valid: true},
		Quantity:        decimalFromFloat(req.Quantity),
		StockDirection:  db.StockDirectionIN,
		MovementType:    db.StockMovementTypeTRANSFERIN,
		Reference:       pgtype.Text{String: transferRef, Valid: true},
		PerformedBy:     pgtype.Int4{Int32: userID, Valid: true},
		MovementDate:    pgtype.Timestamptz{Time: time.Now(), Valid: true},
		Notes:           notes,
	})
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create transfer in movement"})
		return
	}

	// Create new batches in destination warehouse
	newBatchIDs := []int32{}
	for _, alloc := range allocations {
		sourceBatch, err := queries.GetBatchByID(ctx, alloc.BatchID)
		if err != nil {
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to get source batch"})
			return
		}

		batchNumber, err := generateBatchNumber(ctx, queries, req.MaterialID, "transfer")
		if err != nil {
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to generate batch number"})
			return
		}

		newBatch, err := queries.CreateBatch(ctx, db.CreateBatchParams{
			MaterialID:      pgtype.Int4{Int32: req.MaterialID, Valid: true},
			SupplierID:      sourceBatch.SupplierID,
			WarehouseID:     pgtype.Int4{Int32: req.ToWarehouseID, Valid: true},
			MovementID:      pgtype.Int4{Int32: movementIn.ID, Valid: true},
			UnitPrice:       sourceBatch.UnitPrice,
			BatchNumber:     batchNumber,
			ManufactureDate: sourceBatch.ManufactureDate,
			ExpiryDate:      sourceBatch.ExpiryDate,
			StartQuantity:   decimalFromFloat(alloc.Quantity),
			CurrentQuantity: decimalFromFloat(alloc.Quantity),
			Notes:           pgtype.Text{String: fmt.Sprintf("Transferred from batch %s (movement %d)", sourceBatch.BatchNumber, movementOut.ID), Valid: true},
		})
		if err != nil {
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create destination batch"})
			return
		}

		newBatchIDs = append(newBatchIDs, newBatch.ID)
	}

	if err := tx.Commit(ctx); err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to commit transaction"})
		return
	}

	config.RespondJSON(w, http.StatusCreated, TransactionResponse{
		Success:    true,
		Message:    "Transfer completed successfully",
		MovementID: movementOut.ID,
		BatchIDs:   newBatchIDs,
	})
}

// =====================================================
// SCRAP
// =====================================================

func (th *TransactionHandler) Scrap(w http.ResponseWriter, r *http.Request) {
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

	var req ScrapRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	if req.Quantity <= 0 {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Quantity must be positive"})
		return
	}

	if req.Reason == "" {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Reason is required for scrap"})
		return
	}

	tx, err := th.h.DB.Begin(ctx)
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback(ctx)

	queries := th.h.Queries.WithTx(tx)

	// Get batch allocations
	var allocations []BatchAllocation
	if req.UseManual {
		if err := validateBatchAllocations(ctx, queries, req.Batches, req.Quantity); err != nil {
			config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		allocations = req.Batches
	} else {
		valuationMethod, err := getValuationMethod(ctx, queries, req.MaterialID, req.WarehouseID)
		if err != nil {
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to get valuation method"})
			return
		}

		allocations, err = allocateBatchesAuto(ctx, queries, req.MaterialID, req.WarehouseID, req.Quantity, valuationMethod)
		if err != nil {
			config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}

	// Create scrap movement
	movement, err := queries.CreateStockMovement(ctx, db.CreateStockMovementParams{
		MaterialID:      pgtype.Int4{Int32: req.MaterialID, Valid: true},
		FromWarehouseID: pgtype.Int4{Int32: req.WarehouseID, Valid: true},
		Quantity:        decimalFromFloat(req.Quantity),
		StockDirection:  db.StockDirectionOUT,
		MovementType:    db.StockMovementTypeSCRAP,
		Reference:       pgtype.Text{String: fmt.Sprintf("SCRAP-M%d-%d", req.MaterialID, time.Now().Unix()), Valid: true},
		PerformedBy:     pgtype.Int4{Int32: userID, Valid: true},
		MovementDate:    pgtype.Timestamptz{Time: time.Now(), Valid: true},
		Notes:           pgtype.Text{String: req.Reason, Valid: true},
	})
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create scrap movement"})
		return
	}

	// Update batches
	batchIDs := []int32{}
	for _, alloc := range allocations {
		_, err := queries.UpdateBatchQuantity(ctx, db.UpdateBatchQuantityParams{
			ID:              alloc.BatchID,
			CurrentQuantity: decimalFromFloat(-alloc.Quantity),
		})
		if err != nil {
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update batch quantity"})
			return
		}
		batchIDs = append(batchIDs, alloc.BatchID)
	}

	if err := tx.Commit(ctx); err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to commit transaction"})
		return
	}

	config.RespondJSON(w, http.StatusCreated, TransactionResponse{
		Success:    true,
		Message:    "Scrap recorded successfully",
		MovementID: movement.ID,
		BatchIDs:   batchIDs,
	})
}

// =====================================================
// ADJUSTMENT
// =====================================================

func (th *TransactionHandler) Adjustment(w http.ResponseWriter, r *http.Request) {
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

	var req AdjustmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	if req.Quantity <= 0 {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Quantity must be positive"})
		return
	}

	if req.Reason == "" {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Reason is required for adjustment"})
		return
	}

	if req.Direction != "IN" && req.Direction != "OUT" {
		config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "Direction must be 'IN' or 'OUT'"})
		return
	}

	tx, err := th.h.DB.Begin(ctx)
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback(ctx)

	queries := th.h.Queries.WithTx(tx)

	var movement db.StockMovement
	var batchIDs []int32

	if req.Direction == "IN" {
		// Adjustment IN - add stock
		movementType := db.StockMovementTypeADJUSTMENTIN

		movement, err = queries.CreateStockMovement(ctx, db.CreateStockMovementParams{
			MaterialID:     pgtype.Int4{Int32: req.MaterialID, Valid: true},
			ToWarehouseID:  pgtype.Int4{Int32: req.WarehouseID, Valid: true},
			Quantity:       decimalFromFloat(req.Quantity),
			StockDirection: db.StockDirectionIN,
			MovementType:   movementType,
			Reference:      pgtype.Text{String: fmt.Sprintf("ADJ-IN-M%d-%d", req.MaterialID, time.Now().Unix()), Valid: true},
			PerformedBy:    pgtype.Int4{Int32: userID, Valid: true},
			MovementDate:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
			Notes:          pgtype.Text{String: req.Reason, Valid: true},
		})
		if err != nil {
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create adjustment movement"})
			return
		}

		// Create new batch
		batchNumber, err := generateBatchNumber(ctx, queries, req.MaterialID, "adjustment")
		if err != nil {
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to generate batch number"})
			return
		}

		unitPrice := 0.0
		if req.UnitPrice != nil {
			unitPrice = *req.UnitPrice
		}

		batch, err := queries.CreateBatch(ctx, db.CreateBatchParams{
			MaterialID:      pgtype.Int4{Int32: req.MaterialID, Valid: true},
			WarehouseID:     pgtype.Int4{Int32: req.WarehouseID, Valid: true},
			MovementID:      pgtype.Int4{Int32: movement.ID, Valid: true},
			UnitPrice:       decimalFromFloat(unitPrice),
			BatchNumber:     batchNumber,
			StartQuantity:   decimalFromFloat(req.Quantity),
			CurrentQuantity: decimalFromFloat(req.Quantity),
			Notes:           pgtype.Text{String: req.Reason, Valid: true},
		})
		if err != nil {
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create adjustment batch"})
			return
		}

		batchIDs = []int32{batch.ID}

	} else {
		// Adjustment OUT - remove stock
		// Get batch allocations
		var allocations []BatchAllocation
		if req.UseManual {
			if err := validateBatchAllocations(ctx, queries, req.Batches, req.Quantity); err != nil {
				config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			allocations = req.Batches
		} else {
			valuationMethod, err := getValuationMethod(ctx, queries, req.MaterialID, req.WarehouseID)
			if err != nil {
				config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to get valuation method"})
				return
			}

			allocations, err = allocateBatchesAuto(ctx, queries, req.MaterialID, req.WarehouseID, req.Quantity, valuationMethod)
			if err != nil {
				config.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
		}

		movementType := db.StockMovementTypeADJUSTMENTOUT

		movement, err = queries.CreateStockMovement(ctx, db.CreateStockMovementParams{
			MaterialID:      pgtype.Int4{Int32: req.MaterialID, Valid: true},
			FromWarehouseID: pgtype.Int4{Int32: req.WarehouseID, Valid: true},
			Quantity:        decimalFromFloat(req.Quantity),
			StockDirection:  db.StockDirectionOUT,
			MovementType:    movementType,
			Reference:       pgtype.Text{String: fmt.Sprintf("ADJ-OUT-M%d-%d", req.MaterialID, time.Now().Unix()), Valid: true},
			PerformedBy:     pgtype.Int4{Int32: userID, Valid: true},
			MovementDate:    pgtype.Timestamptz{Time: time.Now(), Valid: true},
			Notes:           pgtype.Text{String: req.Reason, Valid: true},
		})
		if err != nil {
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create adjustment movement"})
			return
		}

		// Update batches
		for _, alloc := range allocations {
			_, err := queries.UpdateBatchQuantity(ctx, db.UpdateBatchQuantityParams{
				ID:              alloc.BatchID,
				CurrentQuantity: decimalFromFloat(-alloc.Quantity),
			})
			if err != nil {
				config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update batch quantity"})
				return
			}
			batchIDs = append(batchIDs, alloc.BatchID)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to commit transaction"})
		return
	}

	config.RespondJSON(w, http.StatusCreated, TransactionResponse{
		Success:    true,
		Message:    fmt.Sprintf("Adjustment %s recorded successfully", req.Direction),
		MovementID: movement.ID,
		BatchIDs:   batchIDs,
	})
}
