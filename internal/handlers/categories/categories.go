package categories

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"warehouse_system/internal/config"
	db "warehouse_system/internal/database/db"
	"warehouse_system/internal/handlers"
	"warehouse_system/internal/middlewares"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/xuri/excelize/v2"
)

type CategoryHandler struct {
	h *handlers.Handler
}

func NewCategoryHandler(h *handlers.Handler) *CategoryHandler {
	return &CategoryHandler{h: h}
}

// CreateCategoryRequest represents the request payload for creating a category
type CreateCategoryRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Meta        json.RawMessage `json:"meta"`
}

// UpdateCategoryRequest represents the request payload for updating a category
type UpdateCategoryRequest struct {
	Name        *string         `json:"name"`
	Description *string         `json:"description"`
	Meta        json.RawMessage `json:"meta"`
}

// CreateCategory handles the creation of a new material category.
func (c *CategoryHandler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	var req CreateCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondBadRequest(w, "Invalid request payload", err.Error())
		return
	}

	// Validate required fields
	if req.Name == "" {
		config.RespondBadRequest(w, "Name is required", "")
		return
	}

	// Prepare parameters
	params := db.CreateCategoryParams{
		Name: req.Name,
	}

	// Handle description (optional)
	if req.Description != "" {
		params.Description = pgtype.Text{
			String: req.Description,
			Valid:  true,
		}
	}

	// Handle meta (optional)
	if len(req.Meta) > 0 {
		params.Meta = req.Meta
	}

	category, err := c.h.Queries.CreateCategory(context.Background(), params)
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	config.RespondJSON(w, http.StatusCreated, category)
}

// GetCategoryByID retrieves a material category by its ID.
func (c *CategoryHandler) GetCategoryByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing category ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid category ID format", err.Error())
		return
	}

	category, err := c.h.Queries.GetCategoryByID(context.Background(), id)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{
			"error": "Category not found",
		})
		return
	}

	config.RespondJSON(w, http.StatusOK, category)
}

// UpdateCategory updates the details of an existing material category.
func (c *CategoryHandler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing category ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid category ID format", err.Error())
		return
	}

	var req UpdateCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.RespondBadRequest(w, "Invalid request payload", err.Error())
		return
	}

	// Get current category to preserve fields that aren't being updated
	currentCategory, err := c.h.Queries.GetCategoryByID(context.Background(), id)
	if err != nil {
		config.RespondJSON(w, http.StatusNotFound, map[string]string{
			"error": "Category not found",
		})
		return
	}

	// Build update params with current values as defaults
	params := db.UpdateCategoryParams{
		ID:          id,
		Name:        currentCategory.Name,
		Description: currentCategory.Description,
		Meta:        currentCategory.Meta,
	}

	// Override with new values if provided
	if req.Name != nil {
		params.Name = *req.Name
	}
	if req.Description != nil {
		params.Description = pgtype.Text{
			String: *req.Description,
			Valid:  true,
		}
	}
	if len(req.Meta) > 0 {
		params.Meta = req.Meta
	}

	updatedCategory, err := c.h.Queries.UpdateCategory(context.Background(), params)
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	config.RespondJSON(w, http.StatusOK, updatedCategory)
}

// DeleteCategory removes a material category by its ID.
func (c *CategoryHandler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		config.RespondBadRequest(w, "Missing category ID", "")
		return
	}

	var id int32
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		config.RespondBadRequest(w, "Invalid category ID format", err.Error())
		return
	}

	if err := c.h.Queries.DeleteCategory(context.Background(), id); err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]string{
		"message": "Category deleted successfully",
	})
}

// ListCategories retrieves a list of all material categories.
func (c *CategoryHandler) ListCategories(w http.ResponseWriter, r *http.Request) {
	// Get pagination from context (set by pagination middleware)
	pagination := middlewares.GetPagination(r.Context())
	limit, offset := pagination.GetSQLLimitOffset()

	categories, err := c.h.Queries.ListCategories(context.Background(), db.ListCategoriesParams{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	// Get total count for pagination metadata
	total, err := c.h.Queries.CountCategories(context.Background())
	if err != nil {
		c.h.Logger.Error("Failed to count categories", "error", err)
		// Continue without total count
	}

	// Set total and build pagination metadata
	pagination.SetTotal(total)

	response := map[string]interface{}{
		"categories": categories,
		"pagination": pagination.BuildMeta(),
	}

	config.RespondJSON(w, http.StatusOK, response)
}

func (c *CategoryHandler) DownloadTemplate(w http.ResponseWriter, r *http.Request) {
	f := excelize.NewFile()
	defer f.Close()

	sheet := "Categories"
	index, _ := f.NewSheet(sheet)
	f.SetActiveSheet(index)
	f.DeleteSheet("Sheet1")

	headers := []string{"Name", "Description"}

	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	// Add example rows
	f.SetCellValue(sheet, "A2", "Electronics")
	f.SetCellValue(sheet, "B2", "Electronic components and devices")
	f.SetCellValue(sheet, "A3", "Raw Materials")
	f.SetCellValue(sheet, "B3", "Basic materials for production")

	w.Header().Set("Content-Disposition", "attachment; filename=categories_template.xlsx")
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	_ = f.Write(w)
}

func (c *CategoryHandler) ExportCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := c.h.Queries.ListCategories(context.Background(), db.ListCategoriesParams{
		Limit:  1000000,
		Offset: 0,
	})
	if err != nil {
		c.h.Logger.Error("Failed to fetch categories for export", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch categories"})
		return
	}

	f := excelize.NewFile()
	defer f.Close()

	sheet := "Categories"
	index, _ := f.NewSheet(sheet)
	f.SetActiveSheet(index)
	f.DeleteSheet("Sheet1")

	headers := []string{"Name", "Description", "Created At", "Updated At"}

	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	for i, category := range categories {
		row := i + 2
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), category.Name)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), category.Description.String)
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), category.CreatedAt.Time.Format("2006-01-02 15:04:05"))
		f.SetCellValue(sheet, fmt.Sprintf("D%d", row), category.UpdatedAt.Time.Format("2006-01-02 15:04:05"))
	}

	w.Header().Set("Content-Disposition", "attachment; filename=categories_export.xlsx")
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	_ = f.Write(w)
}

// ImportFromExcel handles batch import of categories from Excel file
func (c *CategoryHandler) ImportFromExcel(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form
	err := r.ParseMultipartForm(10 << 20) // 10 MB max
	if err != nil {
		config.RespondBadRequest(w, "Failed to parse form", err.Error())
		return
	}

	// Get the file from form
	file, _, err := r.FormFile("file")
	if err != nil {
		config.RespondBadRequest(w, "Failed to get file from form", err.Error())
		return
	}
	defer file.Close()

	// Open the Excel file
	f, err := excelize.OpenReader(file)
	if err != nil {
		config.RespondBadRequest(w, "Failed to read Excel file", err.Error())
		return
	}
	defer f.Close()

	// Get the first sheet
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		config.RespondBadRequest(w, "No sheets found in Excel file", "")
		return
	}

	// Read all rows
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		config.RespondBadRequest(w, "Failed to read rows from Excel", err.Error())
		return
	}

	if len(rows) < 2 {
		config.RespondBadRequest(w, "Excel file must have at least a header row and one data row", "")
		return
	}

	// Track results
	var successCount int
	var errors []map[string]interface{}

	// Process each row (skip header)
	for i, row := range rows[1:] {
		rowNum := i + 2 // Excel row number (1-based, +1 for header)

		// Ensure row has at least 2 columns
		if len(row) < 2 {
			errors = append(errors, map[string]interface{}{
				"row":   rowNum,
				"error": "Row must have at least Name and Description columns",
			})
			continue
		}

		name := row[0]
		description := ""
		if len(row) > 1 {
			description = row[1]
		}

		// Validate name
		if name == "" {
			errors = append(errors, map[string]interface{}{
				"row":   rowNum,
				"error": "Name is required",
			})
			continue
		}

		// Create category params
		params := db.CreateCategoryParams{
			Name: name,
		}

		if description != "" {
			params.Description = pgtype.Text{
				String: description,
				Valid:  true,
			}
		}

		// Create category
		_, err := c.h.Queries.CreateCategory(context.Background(), params)
		if err != nil {
			errors = append(errors, map[string]interface{}{
				"row":   rowNum,
				"name":  name,
				"error": err.Error(),
			})
			continue
		}

		successCount++
	}

	// Prepare response
	response := map[string]interface{}{
		"success_count": successCount,
		"error_count":   len(errors),
		"total_rows":    len(rows) - 1,
	}

	if len(errors) > 0 {
		response["errors"] = errors
	}

	if successCount == 0 && len(errors) > 0 {
		config.RespondJSON(w, http.StatusBadRequest, response)
		return
	}

	config.RespondJSON(w, http.StatusCreated, response)
}
