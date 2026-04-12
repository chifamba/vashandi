package main

import (
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/openbrain/db/models"
)

// InitDB connects to Postgres, sets up pgvector, and auto-migrates models
func InitDB() *gorm.DB {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://paperclip:paperclip@localhost:5432/paperclip?sslmode=disable"
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	// Make sure the pgvector extension exists
	db.Exec("CREATE EXTENSION IF NOT EXISTS vector")

	// Auto-migrate schema
	err = db.AutoMigrate(
		&models.Namespace{},
		&models.Memory{},
		&models.Edge{},
	)

	if err != nil {
		log.Fatalf("failed to auto-migrate database: %v", err)
	}

	return db
}
