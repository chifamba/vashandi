package main

import (
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	token := "vashandi_admin_setup"
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(token)))

	invite := models.Invite{
		InviteType:       "bootstrap_ceo",
		TokenHash:        hash,
		AllowedJoinTypes: "both",
		ExpiresAt:        time.Now().Add(24 * time.Hour),
	}

	if err := db.Create(&invite).Error; err != nil {
		log.Fatalf("failed to create bootstrap invite: %v", err)
	}

	publicURL := os.Getenv("PAPERCLIP_PUBLIC_URL")
	if publicURL == "" {
		publicURL = "http://localhost:8080"
	}

	fmt.Printf("Bootstrap invite created!\n")
	fmt.Printf("Token: %s\n", token)
	fmt.Printf("URL: %s/invite/%s\n", publicURL, token)
}
