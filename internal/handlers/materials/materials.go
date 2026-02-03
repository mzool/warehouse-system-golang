package materials

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/xuri/excelize/v2"

	"warehouse_system/internal/config"
	db "warehouse_system/internal/database/db"
	"warehouse_system/internal/handlers"
	"warehouse_system/internal/middlewares"
)

type MaterialHandler struct {
	h *handlers.Handler
}

func NewMaterialHandler(h *handlers.Handler) *MaterialHandler {
	return &MaterialHandler{h: h}
}

func isValidMaterialType(t string) bool {
	// Normalize input: trim whitespace and convert to lowercase
	t = strings.ToLower(strings.TrimSpace(t))
	validTypes := []string{"raw", "intermediate", "finished", "consumable", "service"}
	for _, valid := range validTypes {
		if t == valid {
			return true
		}
	}
	return false
}

func isValidValuationMethod(v string) bool {
	validMethods := []string{"FIFO", "LIFO", "Weighted Average"}
	for _, valid := range validMethods {
		if v == valid {
			return true
		}
	}
	return false
}

type CreateMaterialRequest struct {
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Valuation     string          `json:"valuation"`
	Type          string          `json:"type"`
	Code          string          `json:"code"`
	SKU           string          `json:"sku"`
	Barcode       string          `json:"barcode"`
	MeasureUnitID int32           `json:"measure_unit_id"`
	Category      int32           `json:"category"`
	UnitPrice     float64         `json:"unit_price"`
	SalePrice     float64         `json:"sale_price"`
	Weight        float64         `json:"weight"`
	Volume        float64         `json:"volume"`
	Density       float64         `json:"density"`
	TaxRate       float64         `json:"tax_rate"`
	DiscountRate  float64         `json:"discount_rate"`
	Saleable      bool            `json:"saleable"`
	IsActive      bool            `json:"is_active"`
	IsFragile     bool            `json:"is_fragile"`
	IsFlammable   bool            `json:"is_flammable"`
	IsToxic       bool            `json:"is_toxic"`
	ImageURL      string          `json:"image_url"`
	DocumentURL   string          `json:"document_url"`
	Meta          json.RawMessage `json:"meta"`
}

type UpdateMaterialRequest struct {
	Name          *string         `json:"name,omitempty"`
	Description   *string         `json:"description,omitempty"`
	Valuation     *string         `json:"valuation,omitempty"`
	Type          *string         `json:"type,omitempty"`
	Code          *string         `json:"code,omitempty"`
	SKU           *string         `json:"sku,omitempty"`
	Barcode       *string         `json:"barcode,omitempty"`
	MeasureUnitID *int32          `json:"measure_unit_id,omitempty"`
	Category      *int32          `json:"category,omitempty"`
	UnitPrice     *float64        `json:"unit_price,omitempty"`
	SalePrice     *float64        `json:"sale_price,omitempty"`
	Weight        *float64        `json:"weight,omitempty"`
	Volume        *float64        `json:"volume,omitempty"`
	Density       *float64        `json:"density,omitempty"`
	TaxRate       *float64        `json:"tax_rate,omitempty"`
	DiscountRate  *float64        `json:"discount_rate,omitempty"`
	Saleable      *bool           `json:"saleable,omitempty"`
	IsActive      *bool           `json:"is_active,omitempty"`
	IsFragile     *bool           `json:"is_fragile,omitempty"`
	IsFlammable   *bool           `json:"is_flammable,omitempty"`
	IsToxic       *bool           `json:"is_toxic,omitempty"`
	ImageURL      *string         `json:"image_url,omitempty"`
	DocumentURL   *string         `json:"document_url,omitempty"`
	Meta          json.RawMessage `json:"meta,omitempty"`
}

func (m *MaterialHandler) CreateMaterial(w http.ResponseWriter, r *http.Request) {
	var req CreateMaterialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondBadRequest(w, "Invalid request payload", err.Error())
		return
	}

	if req.Name == "" || req.Code == "" || req.SKU == "" || req.Type == "" {
		config.RespondBadRequest(w, "Missing required fields", "")
		return
	}

	// Normalize type to lowercase
	req.Type = strings.ToLower(strings.TrimSpace(req.Type))

	if !isValidMaterialType(req.Type) {
		config.RespondBadRequest(w, "Invalid material type", "Type must be one of: raw, intermediate, finished, consumable, service")
		return
	}

	if req.Valuation != "" && !isValidValuationMethod(req.Valuation) {
		config.RespondBadRequest(w, "Invalid valuation method", "Valuation must be one of: FIFO, LIFO, Weighted Average")
		return
	}

	codeExists, err := m.h.Queries.CheckDuplicateCode(context.Background(), db.CheckDuplicateCodeParams{
		Code: req.Code,
		ID:   0,
	})
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to validate code"})
		return
	}
	if codeExists {
		config.RespondJSON(w, http.StatusConflict, map[string]string{"error": "Material code already exists"})
		return
	}

	skuExists, err := m.h.Queries.CheckDuplicateSKU(context.Background(), db.CheckDuplicateSKUParams{
		Sku: req.SKU,
		ID:  0,
	})
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to validate SKU"})
		return
	}
	if skuExists {
		config.RespondJSON(w, http.StatusConflict, map[string]string{"error": "Material SKU already exists"})
		return
	}

	params := db.CreateMaterialParams{
		Name:     req.Name,
		Code:     req.Code,
		Sku:      req.SKU,
		Type:     db.MaterialType(req.Type),
		Saleable: pgtype.Bool{Bool: req.Saleable, Valid: true},
		IsActive: pgtype.Bool{Bool: req.IsActive, Valid: true},
		Category: pgtype.Int4{Int32: req.Category, Valid: true},
	}

	// Set optional fields
	if req.Description != "" {
		params.Description = pgtype.Text{String: req.Description, Valid: true}
	}
	if req.Valuation != "" {
		params.Valuation = db.NullValuationMethod{ValuationMethod: db.ValuationMethod(req.Valuation), Valid: true}
	}
	if req.UnitPrice > 0 {
		params.UnitPrice = pgtype.Numeric{Int: big.NewInt(int64(req.UnitPrice * 100)), Exp: -2, Valid: true}
	}
	if req.SalePrice > 0 {
		params.SalePrice = pgtype.Numeric{Int: big.NewInt(int64(req.SalePrice * 100)), Exp: -2, Valid: true}
	}
	if req.Barcode != "" {
		params.Barcode = pgtype.Text{String: req.Barcode, Valid: true}
	}
	if req.MeasureUnitID != 0 {
		params.MeasureUnitID = pgtype.Int4{Int32: req.MeasureUnitID, Valid: true}
	}
	if req.Weight > 0 {
		params.Weight = pgtype.Numeric{Int: big.NewInt(int64(req.Weight * 1000)), Exp: -3, Valid: true}
	}
	if req.Volume > 0 {
		params.Volume = pgtype.Numeric{Int: big.NewInt(int64(req.Volume * 1000)), Exp: -3, Valid: true}
	}
	if req.Density > 0 {
		params.Density = pgtype.Numeric{Int: big.NewInt(int64(req.Density * 1000)), Exp: -3, Valid: true}
	}
	if req.IsToxic {
		params.IsToxic = pgtype.Bool{Bool: req.IsToxic, Valid: true}
	}
	if req.IsFlammable {
		params.IsFlammable = pgtype.Bool{Bool: req.IsFlammable, Valid: true}
	}
	if req.IsFragile {
		params.IsFragile = pgtype.Bool{Bool: req.IsFragile, Valid: true}
	}
	if req.ImageURL != "" {
		params.ImageUrl = pgtype.Text{String: req.ImageURL, Valid: true}
	}
	if req.DocumentURL != "" {
		params.DocumentUrl = pgtype.Text{String: req.DocumentURL, Valid: true}
	}
	if req.TaxRate > 0 {
		params.TaxRate = pgtype.Numeric{Int: big.NewInt(int64(req.TaxRate * 10000)), Exp: -4, Valid: true}
	}
	if req.DiscountRate > 0 {
		params.DiscountRate = pgtype.Numeric{Int: big.NewInt(int64(req.DiscountRate * 10000)), Exp: -4, Valid: true}
	}
	if req.Meta != nil {
		params.Meta = req.Meta
	}

	material, err := m.h.Queries.CreateMaterial(context.Background(), params)
	if err != nil {
		m.h.Logger.Error("Failed to create material", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusCreated, material)
}
func (m *MaterialHandler) GetMaterialByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing material ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid material ID format", err.Error())
		return
	}

	material, err := m.h.Queries.GetMaterialByID(context.Background(), id)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Material not found"})
		return
	}

	config.RespondJSON(w, http.StatusOK, material)
}
func (m *MaterialHandler) SearchMaterials(w http.ResponseWriter, r *http.Request) {
	pagination := middlewares.GetPagination(r.Context())
	limit, offset := pagination.GetSQLLimitOffset()

	// Get query parameters
	query := r.URL.Query().Get("query")
	typeFilter := r.URL.Query().Get("type")
	categoryStr := r.URL.Query().Get("category")
	isActiveStr := r.URL.Query().Get("is_active")
	saleableStr := r.URL.Query().Get("saleable")
	archivedStr := r.URL.Query().Get("archived")

	params := db.SearchMaterialsParams{
		Limit:  limit,
		Offset: offset,
	}

	// Set query parameter
	if query != "" {
		params.Query = pgtype.Text{String: query, Valid: true}
	}

	// Set type filter with validation
	if typeFilter != "" {
		if !isValidMaterialType(typeFilter) {
			config.RespondBadRequest(w, "Invalid material type", "Type must be one of: raw, intermediate, finished, consumable, service")
			return
		}
		params.TypeFilter = db.NullMaterialType{MaterialType: db.MaterialType(typeFilter), Valid: true}
	}

	// Set category filter
	if categoryStr != "" {
		if category, err := strconv.Atoi(categoryStr); err == nil {
			params.Category = pgtype.Int4{Int32: int32(category), Valid: true}
		}
	}

	// Set is_active filter
	if isActiveStr != "" {
		if isActive, err := strconv.ParseBool(isActiveStr); err == nil {
			params.IsActive = pgtype.Bool{Bool: isActive, Valid: true}
		}
	}

	// Set saleable filter
	if saleableStr != "" {
		if saleable, err := strconv.ParseBool(saleableStr); err == nil {
			params.Saleable = pgtype.Bool{Bool: saleable, Valid: true}
		}
	}

	// Set archived filter (defaults to false if not specified)
	if archivedStr != "" {
		if archived, err := strconv.ParseBool(archivedStr); err == nil {
			params.Archived = pgtype.Bool{Bool: archived, Valid: true}
		}
	} else {
		// Default to non-archived materials
		params.Archived = pgtype.Bool{Bool: false, Valid: true}
	}

	materials, err := m.h.Queries.SearchMaterials(context.Background(), params)
	if err != nil {
		m.h.Logger.Error("Failed to search materials", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Build count params
	countParams := db.CountMaterialsParams{}
	if query != "" {
		countParams.Query = pgtype.Text{String: query, Valid: true}
	}
	if typeFilter != "" && isValidMaterialType(typeFilter) {
		countParams.TypeFilter = db.NullMaterialType{MaterialType: db.MaterialType(typeFilter), Valid: true}
	}
	if categoryStr != "" {
		if category, err := strconv.Atoi(categoryStr); err == nil {
			countParams.Category = pgtype.Int4{Int32: int32(category), Valid: true}
		}
	}
	if isActiveStr != "" {
		if isActive, err := strconv.ParseBool(isActiveStr); err == nil {
			countParams.IsActive = pgtype.Bool{Bool: isActive, Valid: true}
		}
	}
	if saleableStr != "" {
		if saleable, err := strconv.ParseBool(saleableStr); err == nil {
			countParams.Saleable = pgtype.Bool{Bool: saleable, Valid: true}
		}
	}
	if archivedStr != "" {
		if archived, err := strconv.ParseBool(archivedStr); err == nil {
			countParams.Archived = pgtype.Bool{Bool: archived, Valid: true}
		}
	} else {
		// Default to non-archived materials
		countParams.Archived = pgtype.Bool{Bool: false, Valid: true}
	}

	total, _ := m.h.Queries.CountMaterials(context.Background(), countParams)
	pagination.SetTotal(total)

	config.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"materials":  materials,
		"pagination": pagination.BuildMeta(),
	})
}
func (m *MaterialHandler) ArchiveMaterial(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	var id int32

	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid material ID", err.Error())
		return
	}

	if err := m.h.Queries.ArchiveMaterial(context.Background(), id); err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]string{"message": "Material archived successfully"})
}

func (m *MaterialHandler) RestoreMaterial(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	var id int32

	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid material ID", err.Error())
		return
	}

	if err := m.h.Queries.RestoreMaterial(context.Background(), id); err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]string{"message": "Material restored successfully"})
}

func (m *MaterialHandler) DownloadTemplate(w http.ResponseWriter, r *http.Request) {
	f := excelize.NewFile()
	defer f.Close()

	sheet := "Materials"
	index, _ := f.NewSheet(sheet)
	f.SetActiveSheet(index)
	f.DeleteSheet("Sheet1")

	headers := []string{
		"Name", "Description", "Type", "Code", "SKU", "Barcode",
		"Saleable", "Unit Price", "Sale Price", "Category Name",
		"Unit Abbreviation", "Weight", "Tax Rate",
		"Is Toxic", "Is Flammable", "Is Fragile", "Is Active",
	}

	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	// Add example row
	f.SetCellValue(sheet, "A2", "Steel Plate")
	f.SetCellValue(sheet, "B2", "High-quality steel plate for manufacturing")
	f.SetCellValue(sheet, "C2", "raw")
	f.SetCellValue(sheet, "D2", "STL-001")
	f.SetCellValue(sheet, "E2", "SKU-STL-001")
	f.SetCellValue(sheet, "F2", "123456789")
	f.SetCellValue(sheet, "G2", "TRUE")
	f.SetCellValue(sheet, "H2", "50.00")
	f.SetCellValue(sheet, "I2", "75.00")
	f.SetCellValue(sheet, "J2", "Raw Materials")
	f.SetCellValue(sheet, "K2", "kg")
	f.SetCellValue(sheet, "L2", "100.50")
	f.SetCellValue(sheet, "M2", "15.00")
	f.SetCellValue(sheet, "N2", "FALSE")
	f.SetCellValue(sheet, "O2", "FALSE")
	f.SetCellValue(sheet, "P2", "FALSE")
	f.SetCellValue(sheet, "Q2", "TRUE")

	w.Header().Set("Content-Disposition", "attachment; filename=materials_template.xlsx")
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	_ = f.Write(w)
}

func (m *MaterialHandler) ExportMaterials(w http.ResponseWriter, r *http.Request) {
	materials, err := m.h.Queries.ExportAllMaterials(context.Background())
	if err != nil {
		m.h.Logger.Error("Failed to fetch materials for export", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch materials"})
		return
	}

	f := excelize.NewFile()
	defer f.Close()

	sheet := "Materials"
	index, _ := f.NewSheet(sheet)
	f.SetActiveSheet(index)
	f.DeleteSheet("Sheet1")

	headers := []string{
		"Name", "Description", "Valuation", "Type", "Code", "SKU", "Barcode",
		"Category", "Unit Abbreviation", "Saleable", "Unit Price", "Sale Price",
		"Weight", "Volume", "Density", "Tax Rate", "Discount Rate",
		"Is Active", "Is Fragile", "Is Flammable", "Is Toxic",
		"Image URL", "Document URL", "Archived", "Created At", "Updated At",
	}

	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	for i, material := range materials {
		row := i + 2
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), material.Name)

		// Handle optional text fields
		if material.Description.Valid {
			f.SetCellValue(sheet, fmt.Sprintf("B%d", row), material.Description.String)
		}

		// Handle valuation enum
		if material.Valuation.Valid {
			f.SetCellValue(sheet, fmt.Sprintf("C%d", row), string(material.Valuation.ValuationMethod))
		}

		f.SetCellValue(sheet, fmt.Sprintf("D%d", row), string(material.Type))
		f.SetCellValue(sheet, fmt.Sprintf("E%d", row), material.Code)
		f.SetCellValue(sheet, fmt.Sprintf("F%d", row), material.Sku)

		if material.Barcode.Valid {
			f.SetCellValue(sheet, fmt.Sprintf("G%d", row), material.Barcode.String)
		}
		if material.CategoryName.Valid {
			f.SetCellValue(sheet, fmt.Sprintf("H%d", row), material.CategoryName.String)
		}
		if material.UnitAbbreviation.Valid {
			f.SetCellValue(sheet, fmt.Sprintf("I%d", row), material.UnitAbbreviation.String)
		}

		// Handle boolean fields
		if material.Saleable.Valid {
			f.SetCellValue(sheet, fmt.Sprintf("J%d", row), material.Saleable.Bool)
		}

		// Handle numeric price fields
		if material.UnitPrice.Valid {
			value, _ := material.UnitPrice.Float64Value()
			f.SetCellValue(sheet, fmt.Sprintf("K%d", row), value.Float64)
		}
		if material.SalePrice.Valid {
			value, _ := material.SalePrice.Float64Value()
			f.SetCellValue(sheet, fmt.Sprintf("L%d", row), value.Float64)
		}

		// Handle numeric measurement fields
		if material.Weight.Valid {
			value, _ := material.Weight.Float64Value()
			f.SetCellValue(sheet, fmt.Sprintf("M%d", row), value.Float64)
		}
		if material.Volume.Valid {
			value, _ := material.Volume.Float64Value()
			f.SetCellValue(sheet, fmt.Sprintf("N%d", row), value.Float64)
		}
		if material.Density.Valid {
			value, _ := material.Density.Float64Value()
			f.SetCellValue(sheet, fmt.Sprintf("O%d", row), value.Float64)
		}

		// Handle numeric percentage fields
		if material.TaxRate.Valid {
			value, _ := material.TaxRate.Float64Value()
			f.SetCellValue(sheet, fmt.Sprintf("P%d", row), value.Float64)
		}
		if material.DiscountRate.Valid {
			value, _ := material.DiscountRate.Float64Value()
			f.SetCellValue(sheet, fmt.Sprintf("Q%d", row), value.Float64)
		}

		// Handle boolean flags
		if material.IsActive.Valid {
			f.SetCellValue(sheet, fmt.Sprintf("R%d", row), material.IsActive.Bool)
		}
		if material.IsFragile.Valid {
			f.SetCellValue(sheet, fmt.Sprintf("S%d", row), material.IsFragile.Bool)
		}
		if material.IsFlammable.Valid {
			f.SetCellValue(sheet, fmt.Sprintf("T%d", row), material.IsFlammable.Bool)
		}
		if material.IsToxic.Valid {
			f.SetCellValue(sheet, fmt.Sprintf("U%d", row), material.IsToxic.Bool)
		}

		// Handle URL fields
		if material.ImageUrl.Valid {
			f.SetCellValue(sheet, fmt.Sprintf("V%d", row), material.ImageUrl.String)
		}
		if material.DocumentUrl.Valid {
			f.SetCellValue(sheet, fmt.Sprintf("W%d", row), material.DocumentUrl.String)
		}

		// Archived status
		if material.Archived.Valid {
			f.SetCellValue(sheet, fmt.Sprintf("X%d", row), material.Archived.Bool)
		}

		// Timestamps
		if material.CreatedAt.Valid {
			f.SetCellValue(sheet, fmt.Sprintf("Y%d", row), material.CreatedAt.Time.Format("2006-01-02 15:04:05"))
		}
		if material.UpdatedAt.Valid {
			f.SetCellValue(sheet, fmt.Sprintf("Z%d", row), material.UpdatedAt.Time.Format("2006-01-02 15:04:05"))
		}
	}

	w.Header().Set("Content-Disposition", "attachment; filename=materials_export.xlsx")
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	_ = f.Write(w)
}

func (m *MaterialHandler) UpdateMaterial(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing material ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid material ID format", err.Error())
		return
	}

	var req UpdateMaterialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondBadRequest(w, "Invalid request payload", err.Error())
		return
	}

	current, err := m.h.Queries.GetMaterialByID(context.Background(), id)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "Material not found"})
		return
	}

	// Duplicate code check
	if req.Code != nil && *req.Code != current.Code {
		exists, err := m.h.Queries.CheckDuplicateCode(context.Background(), db.CheckDuplicateCodeParams{
			Code: *req.Code,
			ID:   id,
		})
		if err != nil {
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to validate code"})
			return
		}
		if exists {
			config.RespondJSON(w, http.StatusConflict, map[string]string{"error": "Material code already exists"})
			return
		}
	}

	// Duplicate SKU check
	if req.SKU != nil && *req.SKU != current.Sku {
		exists, err := m.h.Queries.CheckDuplicateSKU(context.Background(), db.CheckDuplicateSKUParams{
			Sku: *req.SKU,
			ID:  id,
		})
		if err != nil {
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to validate SKU"})
			return
		}
		if exists {
			config.RespondJSON(w, http.StatusConflict, map[string]string{"error": "Material SKU already exists"})
			return
		}
	}

	// Validate valuation method if provided
	if req.Valuation != nil && *req.Valuation != "" && !isValidValuationMethod(*req.Valuation) {
		config.RespondBadRequest(w, "Invalid valuation method", "Valuation must be one of: FIFO, LIFO, Weighted Average")
		return
	}

	// Validate material type if provided
	if req.Type != nil && *req.Type != "" {
		// Normalize type to lowercase
		normalizedType := strings.ToLower(strings.TrimSpace(*req.Type))
		req.Type = &normalizedType
		if !isValidMaterialType(*req.Type) {
			config.RespondBadRequest(w, "Invalid material type", "Type must be one of: raw, intermediate, finished, consumable, service")
			return
		}
	}

	params := db.UpdateMaterialParams{
		ID: id,
	}

	// Handle name (Column2 is interface{} due to NULLIF)
	if req.Name != nil {
		params.Column2 = *req.Name
	} else {
		params.Column2 = ""
	}

	// Handle code (Column10 is interface{} due to NULLIF)
	if req.Code != nil {
		params.Column10 = *req.Code
	} else {
		params.Column10 = ""
	}

	// Handle SKU (Column11 is interface{} due to NULLIF)
	if req.SKU != nil {
		params.Column11 = *req.SKU
	} else {
		params.Column11 = ""
	}

	// Optional text fields
	if req.Description != nil {
		params.Description = pgtype.Text{String: *req.Description, Valid: true}
	}
	if req.Barcode != nil {
		params.Barcode = pgtype.Text{String: *req.Barcode, Valid: true}
	}
	if req.ImageURL != nil {
		params.ImageUrl = pgtype.Text{String: *req.ImageURL, Valid: true}
	}
	if req.DocumentURL != nil {
		params.DocumentUrl = pgtype.Text{String: *req.DocumentURL, Valid: true}
	}

	// Enum and category fields
	if req.Valuation != nil {
		params.Valuation = db.NullValuationMethod{ValuationMethod: db.ValuationMethod(*req.Valuation), Valid: true}
	}
	if req.Type != nil {
		params.Type = db.MaterialType(*req.Type)
	}
	if req.Category != nil {
		params.Category = pgtype.Int4{Int32: *req.Category, Valid: true}
	}
	if req.MeasureUnitID != nil {
		params.MeasureUnitID = pgtype.Int4{Int32: *req.MeasureUnitID, Valid: true}
	}

	// Boolean fields
	if req.Saleable != nil {
		params.Saleable = pgtype.Bool{Bool: *req.Saleable, Valid: true}
	}
	if req.IsActive != nil {
		params.IsActive = pgtype.Bool{Bool: *req.IsActive, Valid: true}
	}
	if req.IsToxic != nil {
		params.IsToxic = pgtype.Bool{Bool: *req.IsToxic, Valid: true}
	}
	if req.IsFlammable != nil {
		params.IsFlammable = pgtype.Bool{Bool: *req.IsFlammable, Valid: true}
	}
	if req.IsFragile != nil {
		params.IsFragile = pgtype.Bool{Bool: *req.IsFragile, Valid: true}
	}

	// Numeric price fields
	if req.UnitPrice != nil {
		params.UnitPrice = pgtype.Numeric{Int: big.NewInt(int64(*req.UnitPrice * 100)), Exp: -2, Valid: true}
	}
	if req.SalePrice != nil {
		params.SalePrice = pgtype.Numeric{Int: big.NewInt(int64(*req.SalePrice * 100)), Exp: -2, Valid: true}
	}

	// Numeric measurement fields
	if req.Weight != nil {
		params.Weight = pgtype.Numeric{Int: big.NewInt(int64(*req.Weight * 1000)), Exp: -3, Valid: true}
	}
	if req.Volume != nil {
		params.Volume = pgtype.Numeric{Int: big.NewInt(int64(*req.Volume * 1000)), Exp: -3, Valid: true}
	}
	if req.Density != nil {
		params.Density = pgtype.Numeric{Int: big.NewInt(int64(*req.Density * 1000)), Exp: -3, Valid: true}
	}

	// Numeric percentage fields
	if req.TaxRate != nil {
		params.TaxRate = pgtype.Numeric{Int: big.NewInt(int64(*req.TaxRate * 10000)), Exp: -4, Valid: true}
	}
	if req.DiscountRate != nil {
		params.DiscountRate = pgtype.Numeric{Int: big.NewInt(int64(*req.DiscountRate * 10000)), Exp: -4, Valid: true}
	}

	// Meta field
	if req.Meta != nil {
		params.Meta = req.Meta
	}

	updated, err := m.h.Queries.UpdateMaterial(context.Background(), params)
	if err != nil {
		m.h.Logger.Error("Failed to update material", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	config.RespondJSON(w, http.StatusOK, updated)
}
func (m *MaterialHandler) ImportFromExcel(w http.ResponseWriter, r *http.Request) {
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
		config.RespondBadRequest(w, "Excel file is empty", "")
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
	requiredCols := []string{"name", "type", "code", "sku"}
	for _, col := range requiredCols {
		if _, exists := colMap[col]; !exists {
			config.RespondBadRequest(w, "Missing required column", fmt.Sprintf("Excel file must have '%s' column", col))
			return
		}
	}

	tx, err := m.h.DB.Begin(context.Background())
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to begin transaction"})
		return
	}
	defer tx.Rollback(context.Background())

	qtx := m.h.Queries.WithTx(tx)

	errorCount := 0
	errors := []string{}
	batchParams := []db.BatchCreateMaterialsParams{}

	for i, row := range rows[1:] {
		rowNum := i + 2

		name := getCol(row, "name")
		code := getCol(row, "code")
		sku := getCol(row, "sku")
		materialType := strings.ToLower(getCol(row, "type"))
		categoryName := getCol(row, "category name")
		unitAbbr := getCol(row, "unit abbreviation")

		if name == "" || code == "" || sku == "" || materialType == "" {
			errorCount++
			errors = append(errors, fmt.Sprintf("Row %d: missing required fields (name, code, sku, type)", rowNum))
			continue
		}

		if !isValidMaterialType(materialType) {
			errorCount++
			errors = append(errors, fmt.Sprintf("Row %d: invalid material type '%s' (must be: raw, intermediate, finished, consumable, service)", rowNum, materialType))
			continue
		}

		unitPrice, _ := strconv.ParseFloat(getCol(row, "unit price"), 64)
		salePrice, _ := strconv.ParseFloat(getCol(row, "sale price"), 64)
		weight, _ := strconv.ParseFloat(getCol(row, "weight"), 64)
		taxRate, _ := strconv.ParseFloat(getCol(row, "tax rate"), 64)

		params := db.BatchCreateMaterialsParams{
			Name:        name,
			Code:        code,
			Sku:         sku,
			Type:        db.MaterialType(materialType),
			Saleable:    pgtype.Bool{Bool: strings.EqualFold(getCol(row, "saleable"), "TRUE"), Valid: true},
			IsActive:    pgtype.Bool{Bool: strings.EqualFold(getCol(row, "is active"), "TRUE"), Valid: true},
			IsToxic:     pgtype.Bool{Bool: strings.EqualFold(getCol(row, "is toxic"), "TRUE"), Valid: true},
			IsFlammable: pgtype.Bool{Bool: strings.EqualFold(getCol(row, "is flammable"), "TRUE"), Valid: true},
			IsFragile:   pgtype.Bool{Bool: strings.EqualFold(getCol(row, "is fragile"), "TRUE"), Valid: true},
		}

		// Lookup category by name
		if categoryName != "" {
			category, err := qtx.GetCategoryByName(context.Background(), categoryName)
			if err == nil {
				params.Category = pgtype.Int4{Int32: category.ID, Valid: true}
			} else {
				errorCount++
				errors = append(errors, fmt.Sprintf("Row %d: category '%s' not found", rowNum, categoryName))
				continue
			}
		}

		// Lookup unit by abbreviation
		if unitAbbr != "" {
			unit, err := qtx.GetUnitByAbbreviation(context.Background(), unitAbbr)
			if err == nil {
				params.MeasureUnitID = pgtype.Int4{Int32: unit.ID, Valid: true}
			} else {
				errorCount++
				errors = append(errors, fmt.Sprintf("Row %d: unit abbreviation '%s' not found", rowNum, unitAbbr))
				continue
			}
		}

		if unitPrice > 0 {
			var n pgtype.Numeric
			n.Scan(fmt.Sprintf("%.4f", unitPrice))
			params.UnitPrice = n
		}

		if salePrice > 0 {
			var n pgtype.Numeric
			n.Scan(fmt.Sprintf("%.4f", salePrice))
			params.SalePrice = n
		}

		if weight > 0 {
			var n pgtype.Numeric
			n.Scan(fmt.Sprintf("%.4f", weight))
			params.Weight = n
		}

		if taxRate > 0 {
			var n pgtype.Numeric
			n.Scan(fmt.Sprintf("%.2f", taxRate))
			params.TaxRate = n
		}

		barcode := getCol(row, "barcode")
		if barcode != "" {
			params.Barcode = pgtype.Text{String: barcode, Valid: true}
		}

		description := getCol(row, "description")
		if description != "" {
			params.Description = pgtype.Text{String: description, Valid: true}
		}

		batchParams = append(batchParams, params)
	}

	// If any errors occurred during validation, rollback and return all errors
	if errorCount > 0 {
		config.RespondJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":       "Import failed - no materials were imported due to validation errors",
			"error_count": errorCount,
			"errors":      errors,
		})
		return
	}

	// All validations passed, proceed with batch insert
	successCount := 0
	if len(batchParams) > 0 {
		count, err := qtx.BatchCreateMaterials(context.Background(), batchParams)
		if err != nil {
			config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to insert materials: " + err.Error()})
			return
		}
		successCount = int(count)
	}

	if err := tx.Commit(context.Background()); err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to commit transaction"})
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message":       "Import completed successfully",
		"success_count": successCount,
	})
}
