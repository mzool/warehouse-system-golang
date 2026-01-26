package handlers

import (
	"log/slog"
	"warehouse_system/internal/cache"
	"warehouse_system/internal/config"
	"warehouse_system/internal/database/db"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	Queries *db.Queries
	Cache   cache.Cache
	Logger  *slog.Logger
	DB      *pgxpool.Pool
	CFG     *config.Config // Application configuration
}

func NewHandler(q *db.Queries, c cache.Cache, l *slog.Logger, db *pgxpool.Pool, cfg *config.Config) *Handler {
	return &Handler{
		Queries: q,
		Cache:   c,
		Logger:  l,
		DB:      db,
		CFG:     cfg,
	}
}
