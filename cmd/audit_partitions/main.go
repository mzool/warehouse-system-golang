package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strconv"
	"time"
	"warehouse_system/internal/config"
)

func main() {
	year := time.Now().Year()
	nextYear := year + 1
	yearStr := strconv.Itoa(year)
	nextYearStr := strconv.Itoa(nextYear)

	months := []string{"01", "02", "03", "04", "05", "06", "07", "08", "09", "10", "11", "12"}

	var sql string

	// Create partition for January
	sql += fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS audit_logs_%s_%s PARTITION OF audit_logs
		FOR VALUES FROM ('%s-%s-01') TO ('%s-%s-01');
	`, yearStr, months[0], yearStr, months[0], yearStr, months[1])

	// Create partitions for February through November
	for i := 1; i < len(months)-1; i++ {
		sql += fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS audit_logs_%s_%s PARTITION OF audit_logs
		FOR VALUES FROM ('%s-%s-01') TO ('%s-%s-01');
		`, yearStr, months[i], yearStr, months[i], yearStr, months[i+1])
	}

	// Create partition for December (ends at next year's January)
	sql += fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS audit_logs_%s_%s PARTITION OF audit_logs
		FOR VALUES FROM ('%s-%s-01') TO ('%s-01-01');
	`, yearStr, months[11], yearStr, months[11], nextYearStr)

	// Setup logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Load configuration
	cfg, err := config.LoadConfig(logger)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize database connection if needed
	dbConfig := config.DBConfig{
		DatabaseURL:       cfg.Database.URL,
		Logger:            logger,
		MaxConns:          10,
		MinConns:          2,
		HealthCheckPeriod: time.Minute,
		ConnectTimeout:    5 * time.Second,
		MaxRetries:        3,
		RetryDelay:        time.Second,
	}
	db, err := config.NewPool(&dbConfig)
	if err != nil {
		fmt.Println("Failed to connect to database:", err)
		return
	}
	defer db.Close()
	_, err = db.Exec(context.Background(), sql)
	if err != nil {
		fmt.Println("Failed to create partitions:", err)
		return
	}
	fmt.Println("Partitions created successfully.")
}
