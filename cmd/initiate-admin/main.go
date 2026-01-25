package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"
	"warehouse_system/internal/config"
	"warehouse_system/internal/security"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/term"
)

func main() {
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

	// check if admin user exists
	adminExists, err := checkAdminExists(db)
	if err != nil {
		fmt.Println("Failed to check for existing admin user:", err)
		return
	}
	if adminExists {
		fmt.Println("Admin user already exists. Exiting.")
		return
	}
	// simple cli to initiate an admin user
	reader := bufio.NewScanner(os.Stdin)
	var username, password, email, fullName string

	fmt.Println("Initiating admin user creation")

	fmt.Println("Enter username:")
	reader.Scan()
	username = reader.Text()

	fmt.Println("Enter password:")
	passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Println("Failed to read password:", err)
		return
	}
	password = string(passwordBytes)
	fmt.Println() // Print newline after password input

	fmt.Println("Enter email:")
	reader.Scan()
	email = reader.Text()

	fmt.Println("Enter full name:")
	reader.Scan()
	fullName = reader.Text()

	// hash password
	hashedPassword, err := security.HashPassword(password)
	if err != nil {
		fmt.Println("Failed to hash password:", err)
		return
	}

	sql := `INSERT INTO users (username, password_hash, email, full_name, role, created_at)
			VALUES ($1, $2, $3, $4, 'admin', NOW())`
	_, err = db.Exec(context.Background(), sql, username, hashedPassword, email, fullName)
	if err != nil {
		fmt.Println("Failed to create admin user:", err)
		return
	}

	fmt.Println("Admin user created successfully")

}

func checkAdminExists(db *pgxpool.Pool) (bool, error) {
	sql := `SELECT COUNT(*) FROM users WHERE role = 'admin'`
	var count int
	err := db.QueryRow(context.Background(), sql).Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}
