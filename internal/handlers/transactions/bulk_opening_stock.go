package transactions

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/xuri/excelize/v2"

	"warehouse_system/internal/config"
	db "warehouse_system/internal/database/db"
)

// BulkOpeningStockEntry represents a single opening stock entry from Excel
type BulkOpeningStockEntry struct {
	MaterialCode    string
	WarehouseCode   string
	Quantity        float64
	UnitPrice       float64
	ManufactureDate *string
	ExpiryDate      *string
	Notes           *string
}

// BulkOpeningStockFailedEntry represents a failed import entry
type BulkOpeningStockFailedEntry struct {
	Row           int    `json:"row"`
	MaterialCode  string `json:"material_code"`
	WarehouseCode string `json:"warehouse_code"`
	Reason        string `json:"reason"`
}

// BulkOpeningStockResponse represents the response for bulk opening stock import
type BulkOpeningStockResponse struct {
	SuccessCount int                           `json:"success_count"`
	FailedCount  int                           `json:"failed_count"`
	TotalRows    int                           `json:"total_rows"`
	Failed       []BulkOpeningStockFailedEntry `json:"failed,omitempty"`
}

// DownloadOpeningStockTemplate generates an Excel template for bulk opening stock import
func (t *TransactionHandler) DownloadOpeningStockTemplate(w http.ResponseWriter, r *http.Request) {
	f := excelize.NewFile()
	defer f.Close()

	sheet := "Opening Stock"
	index, _ := f.NewSheet(sheet)
	f.SetActiveSheet(index)
	f.DeleteSheet("Sheet1")

	// Define headers
	headers := []string{
		"Material Code",
		"Warehouse Code",
		"Quantity",
		"Unit Price",
		"Manufacture Date (YYYY-MM-DD)",
		"Expiry Date (YYYY-MM-DD)",
		"Notes",
	}

	// Set headers
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	// Add example rows
	f.SetCellValue(sheet, "A2", "10.2401")
	f.SetCellValue(sheet, "B2", "WH-MAIN")
	f.SetCellValue(sheet, "C2", "5000")
	f.SetCellValue(sheet, "D2", "1.50")
	f.SetCellValue(sheet, "E2", "2026-01-15")
	f.SetCellValue(sheet, "F2", "2027-01-15")
	f.SetCellValue(sheet, "G2", "Initial stock entry")

	f.SetCellValue(sheet, "A3", "21.5025")
	f.SetCellValue(sheet, "B3", "WH-MAIN")
	f.SetCellValue(sheet, "C3", "10000")
	f.SetCellValue(sheet, "D3", "0.75")
	f.SetCellValue(sheet, "E3", "")
	f.SetCellValue(sheet, "F3", "")
	f.SetCellValue(sheet, "G3", "")

	// Style headers
	style, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"#E0E0E0"}, Pattern: 1},
	})
	f.SetCellStyle(sheet, "A1", "G1", style)

	// Set column widths
	f.SetColWidth(sheet, "A", "A", 15)
	f.SetColWidth(sheet, "B", "B", 15)
	f.SetColWidth(sheet, "C", "C", 12)
	f.SetColWidth(sheet, "D", "D", 12)
	f.SetColWidth(sheet, "E", "E", 25)
	f.SetColWidth(sheet, "F", "F", 25)
	f.SetColWidth(sheet, "G", "G", 30)

	// Write to response
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=opening_stock_template.xlsx")

	if err := f.Write(w); err != nil {
		t.h.Logger.Error("Failed to write Excel template", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to generate template"})
		return
	}
}

// ImportOpeningStockFromExcel imports opening stock from an Excel file
func (t *TransactionHandler) ImportOpeningStockFromExcel(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		config.RespondBadRequest(w, "Failed to parse form", err.Error())
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		config.RespondBadRequest(w, "No file uploaded", err.Error())
		return
	}
	defer file.Close()

	f, err := excelize.OpenReader(file)
	if err != nil {
		config.RespondBadRequest(w, "Invalid Excel file", err.Error())
		return
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	rows, err := f.GetRows(sheet)
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to read Excel rows"})
		return
	}

	if len(rows) < 2 {
		config.RespondBadRequest(w, "Excel file is empty", "File must have at least one data row")
		return
	}

	// Build column header map
	headers := rows[0]
	colMap := make(map[string]int)
	for i, header := range headers {
		colMap[strings.ToLower(strings.TrimSpace(header))] = i
	}

	// Helper function to get column value safely
	getCol := func(row []string, colName string) string {
		if idx, exists := colMap[colName]; exists && idx < len(row) {
			return strings.TrimSpace(row[idx])
		}
		return ""
	}

	// Validate required columns exist
	requiredCols := []string{"material code", "warehouse code", "quantity", "unit price"}
	for _, col := range requiredCols {
		if _, exists := colMap[col]; !exists {
			config.RespondBadRequest(w, "Missing required column", fmt.Sprintf("Excel file must have '%s' column", col))
			return
		}
	}

	// Start transaction
	ctx := context.Background()
	tx, err := t.h.DB.Begin(ctx)
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to begin transaction"})
		return
	}
	defer tx.Rollback(ctx)

	qtx := t.h.Queries.WithTx(tx)

	var failed []BulkOpeningStockFailedEntry
	successCount := 0

	// Process each row
	for i, row := range rows[1:] {
		rowNum := i + 2

		materialCode := getCol(row, "material code")
		warehouseCode := getCol(row, "warehouse code")
		quantityStr := getCol(row, "quantity")
		unitPriceStr := getCol(row, "unit price")
		manufactureDate := getCol(row, "manufacture date (yyyy-mm-dd)")
		expiryDate := getCol(row, "expiry date (yyyy-mm-dd)")
		notes := getCol(row, "notes")

		// Validate required fields
		if materialCode == "" || warehouseCode == "" || quantityStr == "" || unitPriceStr == "" {
			failed = append(failed, BulkOpeningStockFailedEntry{
				Row:           rowNum,
				MaterialCode:  materialCode,
				WarehouseCode: warehouseCode,
				Reason:        "Missing required fields (material code, warehouse code, quantity, unit price)",
			})
			continue
		}

		// Parse quantity
		quantity, err := strconv.ParseFloat(quantityStr, 64)
		if err != nil || quantity <= 0 {
			failed = append(failed, BulkOpeningStockFailedEntry{
				Row:           rowNum,
				MaterialCode:  materialCode,
				WarehouseCode: warehouseCode,
				Reason:        fmt.Sprintf("Invalid quantity: %s (must be a positive number)", quantityStr),
			})
			continue
		}

		// Parse unit price
		unitPrice, err := strconv.ParseFloat(unitPriceStr, 64)
		if err != nil || unitPrice < 0 {
			failed = append(failed, BulkOpeningStockFailedEntry{
				Row:           rowNum,
				MaterialCode:  materialCode,
				WarehouseCode: warehouseCode,
				Reason:        fmt.Sprintf("Invalid unit price: %s (must be a non-negative number)", unitPriceStr),
			})
			continue
		}

		// Lookup material by code
		material, err := qtx.GetMaterialByCode(ctx, materialCode)
		if err != nil {
			failed = append(failed, BulkOpeningStockFailedEntry{
				Row:           rowNum,
				MaterialCode:  materialCode,
				WarehouseCode: warehouseCode,
				Reason:        fmt.Sprintf("Material with code '%s' not found", materialCode),
			})
			continue
		}

		// Lookup warehouse by code
		warehouse, err := qtx.GetWarehouseByCode(ctx, warehouseCode)
		if err != nil {
			failed = append(failed, BulkOpeningStockFailedEntry{
				Row:           rowNum,
				MaterialCode:  materialCode,
				WarehouseCode: warehouseCode,
				Reason:        fmt.Sprintf("Warehouse with code '%s' not found", warehouseCode),
			})
			continue
		}

		// Parse dates
		var pgManufactureDate, pgExpiryDate pgtype.Date
		if manufactureDate != "" {
			t, err := time.Parse("2006-01-02", manufactureDate)
			if err != nil {
				failed = append(failed, BulkOpeningStockFailedEntry{
					Row:           rowNum,
					MaterialCode:  materialCode,
					WarehouseCode: warehouseCode,
					Reason:        fmt.Sprintf("Invalid manufacture date format: %s (use YYYY-MM-DD)", manufactureDate),
				})
				continue
			}
			pgManufactureDate = pgtype.Date{Time: t, Valid: true}
		}

		if expiryDate != "" {
			t, err := time.Parse("2006-01-02", expiryDate)
			if err != nil {
				failed = append(failed, BulkOpeningStockFailedEntry{
					Row:           rowNum,
					MaterialCode:  materialCode,
					WarehouseCode: warehouseCode,
					Reason:        fmt.Sprintf("Invalid expiry date format: %s (use YYYY-MM-DD)", expiryDate),
				})
				continue
			}
			pgExpiryDate = pgtype.Date{Time: t, Valid: true}
		}

		// Create opening stock transaction
		var pgUnitPrice pgtype.Numeric
		if err := pgUnitPrice.Scan(fmt.Sprintf("%.4f", unitPrice)); err != nil {
			failed = append(failed, BulkOpeningStockFailedEntry{
				Row:           rowNum,
				MaterialCode:  materialCode,
				WarehouseCode: warehouseCode,
				Reason:        fmt.Sprintf("Failed to process unit price: %v", err),
			})
			continue
		}

		// Set quantity
		var pgQuantity pgtype.Numeric
		if err := pgQuantity.Scan(fmt.Sprintf("%.4f", quantity)); err != nil {
			failed = append(failed, BulkOpeningStockFailedEntry{
				Row:           rowNum,
				MaterialCode:  materialCode,
				WarehouseCode: warehouseCode,
				Reason:        fmt.Sprintf("Failed to process quantity: %v", err),
			})
			continue
		}

		params := db.CreateTransactionParams{
			MaterialID:      pgtype.Int4{Int32: material.ID, Valid: true},
			WarehouseID:     pgtype.Int4{Int32: warehouse.ID, Valid: true},
			TransactionType: "opening_stock",
			Quantity:        pgQuantity,
			UnitPrice:       pgUnitPrice,
			ManufactureDate: pgManufactureDate,
			ExpiryDate:      pgExpiryDate,
		}

		// Set notes if provided
		if notes != "" {
			params.Notes = pgtype.Text{String: notes, Valid: true}
		}

		// Execute opening stock creation
		_, err = qtx.CreateTransaction(ctx, params)
		if err != nil {
			t.h.Logger.Error("Failed to create opening stock", "error", err, "material", materialCode, "warehouse", warehouseCode)
			failed = append(failed, BulkOpeningStockFailedEntry{
				Row:           rowNum,
				MaterialCode:  materialCode,
				WarehouseCode: warehouseCode,
				Reason:        fmt.Sprintf("Database error: %v", err),
			})
			continue
		}

		successCount++
	}

	// If any errors occurred during validation or processing, rollback
	if len(failed) > 0 {
		config.RespondJSON(w, http.StatusBadRequest, BulkOpeningStockResponse{
			SuccessCount: 0,
			FailedCount:  len(failed),
			TotalRows:    len(rows) - 1,
			Failed:       failed,
		})
		return
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to commit transaction"})
		return
	}

	config.RespondJSON(w, http.StatusCreated, BulkOpeningStockResponse{
		SuccessCount: successCount,
		FailedCount:  0,
		TotalRows:    len(rows) - 1,
	})
}
