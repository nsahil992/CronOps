package main

import (
	"database/sql"
	"log"
)

// RunMigrations handles database schema migrations
func RunMigrations(db *sql.DB) {
	log.Println("Running database migrations...")

	// Create cron_expressions table if it doesn't exist
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS cron_expressions (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			expression VARCHAR(255) NOT NULL,
			description TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		log.Fatalf("Error creating cron_expressions table: %v", err)
	}

	log.Println("Migrations completed successfully")
}
