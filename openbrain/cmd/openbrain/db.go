package main

import (
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/openbrain/internal/brain"
)

func InitDB() *gorm.DB {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://paperclip:paperclip@localhost:5432/paperclip?sslmode=disable"
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}
	service := brain.NewService(db)
	if err := service.AutoMigrate(); err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}
	return db
}
