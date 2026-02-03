package quality

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"warehouse_system/internal/config"
	db "warehouse_system/internal/database/db"

	"github.com/jackc/pgx/v5/pgtype"
)

// ============================================================================
// DASHBOARD & STATISTICS HANDLERS
// ============================================================================

func (qh *QualityHandler) GetQualityDashboardStats(w http.ResponseWriter, r *http.Request) {
	stats, err := qh.h.Queries.GetQualityDashboardStats(context.Background())
	if err != nil {
		qh.h.Logger.Error("Failed to get quality dashboard stats", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to retrieve dashboard statistics")
		return
	}

	config.RespondJSON(w, http.StatusOK, stats)
}

func (qh *QualityHandler) GetQualityInspectionTrends(w http.ResponseWriter, r *http.Request) {
	startDateStr := r.URL.Query().Get("start_date")
	endDateStr := r.URL.Query().Get("end_date")

	if startDateStr == "" || endDateStr == "" {
		config.RespondJSON(w, http.StatusBadRequest, "start_date and end_date are required")
		return
	}

	startDate, err := time.Parse("2006-01-02", startDateStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid start_date format (use YYYY-MM-DD)")
		return
	}

	endDate, err := time.Parse("2006-01-02", endDateStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid end_date format (use YYYY-MM-DD)")
		return
	}

	trends, err := qh.h.Queries.GetQualityInspectionTrends(context.Background(), db.GetQualityInspectionTrendsParams{
		InspectionDate: pgtype.Timestamptz{
			Time:  startDate,
			Valid: true,
		},
		InspectionDate_2: pgtype.Timestamptz{
			Time:  endDate,
			Valid: true,
		},
	})

	if err != nil {
		qh.h.Logger.Error("Failed to get quality inspection trends", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to retrieve inspection trends")
		return
	}

	config.RespondJSON(w, http.StatusOK, trends)
}

func (qh *QualityHandler) GetTopDefectiveMaterials(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	if limit <= 0 {
		limit = 10
	}

	materials, err := qh.h.Queries.GetTopDefectiveMaterials(context.Background(), int32(limit))

	if err != nil {
		qh.h.Logger.Error("Failed to get top defective materials", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to retrieve defective materials")
		return
	}

	config.RespondJSON(w, http.StatusOK, materials)
}

func (qh *QualityHandler) GetMaterialInspectionStats(w http.ResponseWriter, r *http.Request) {
	materialIDStr := r.PathValue("material_id")
	materialID, err := strconv.Atoi(materialIDStr)
	if err != nil {
		config.RespondJSON(w, http.StatusBadRequest, "Invalid material ID")
		return
	}

	stats, err := qh.h.Queries.GetInspectionStatsByMaterial(context.Background(), pgtype.Int4{
		Int32: int32(materialID),
		Valid: true,
	})
	if err != nil {
		qh.h.Logger.Error("Failed to get material inspection stats", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to retrieve material statistics")
		return
	}

	config.RespondJSON(w, http.StatusOK, stats)
}

func (qh *QualityHandler) CountInspectionsByStatus(w http.ResponseWriter, r *http.Request) {
	status := r.PathValue("status")

	count, err := qh.h.Queries.CountQualityInspectionsByStatus(context.Background(), db.NullQualityInspectionStatus{
		QualityInspectionStatus: db.QualityInspectionStatus(status),
		Valid:                   true,
	})
	if err != nil {
		qh.h.Logger.Error("Failed to count inspections by status", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to count inspections")
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]int64{"count": count})
}

func (qh *QualityHandler) CountNCRsByStatus(w http.ResponseWriter, r *http.Request) {
	status := r.PathValue("status")

	count, err := qh.h.Queries.CountNonConformanceReportsByStatus(context.Background(), db.NullNcrStatus{
		NcrStatus: db.NcrStatus(status),
		Valid:     true,
	})
	if err != nil {
		qh.h.Logger.Error("Failed to count NCRs by status", "error", err)
		config.RespondJSON(w, http.StatusInternalServerError, "Failed to count NCRs")
		return
	}

	config.RespondJSON(w, http.StatusOK, map[string]int64{"count": count})
}
