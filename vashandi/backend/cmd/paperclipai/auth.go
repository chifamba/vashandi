package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authentication and bootstrap utilities",
}

var bootstrapCeoCmd = &cobra.Command{
	Use:   "bootstrap-ceo",
	Short: "Create a one-time bootstrap invite URL for first instance admin",
	Run: func(cmd *cobra.Command, args []string) {
		force, _ := cmd.Flags().GetBool("force")
		expiresHours, _ := cmd.Flags().GetInt("expires-hours")
		baseUrlFlag, _ := cmd.Flags().GetString("base-url")

		// Read db config
		dbUrl := os.Getenv("DATABASE_URL")
		if dbUrl == "" {
			// fallback check
			dbUrl = "postgres://paperclip:paperclip@127.0.0.1:5432/paperclip?sslmode=disable"
		}

		db, err := gorm.Open(postgres.Open(dbUrl), &gorm.Config{})
		if err != nil {
			fmt.Printf("Could not resolve database connection for bootstrap: %v\n", err)
			return
		}

		// Check for existing instance_admin
		var adminCount int64
		db.Model(&models.InstanceUserRole{}).Where("role = ?", "instance_admin").Count(&adminCount)

		if adminCount > 0 && !force {
			fmt.Println("Instance already has an admin user. Use --force to generate a new bootstrap invite.")
			return
		}

		now := time.Now()
		
		// Revoke existing invites
		db.Model(&models.Invite{}).
			Where("invite_type = ?", "bootstrap_ceo").
			Where("revoked_at IS NULL").
			Where("accepted_at IS NULL").
			Where("expires_at > ?", now).
			Updates(map[string]interface{}{
				"revoked_at": now,
				"updated_at": now,
			})

		// Create token
		b := make([]byte, 24)
		rand.Read(b)
		token := "pcp_bootstrap_" + hex.EncodeToString(b)
		
		hash := sha256.Sum256([]byte(token))
		tokenHash := hex.EncodeToString(hash[:])

		if expiresHours < 1 {
			expiresHours = 1
		} else if expiresHours > 720 {
			expiresHours = 720
		}

		systemStr := "system"
		invite := models.Invite{
			InviteType:       "bootstrap_ceo",
			TokenHash:        tokenHash,
			AllowedJoinTypes: "human",
			ExpiresAt:        now.Add(time.Duration(expiresHours) * time.Hour),
			InvitedByUserID:  &systemStr,
		}

		if err := db.Create(&invite).Error; err != nil {
			fmt.Printf("Could not create bootstrap invite: %v\n", err)
			return
		}

		baseUrl := baseUrlFlag
		if baseUrl == "" {
			baseUrl = os.Getenv("PAPERCLIP_PUBLIC_URL")
			if baseUrl == "" {
				baseUrl = "http://localhost:3100"
			}
		}
		baseUrl = strings.TrimRight(baseUrl, "/")

		inviteUrl := fmt.Sprintf("%s/invite/%s", baseUrl, token)
		fmt.Println("Created bootstrap CEO invite.")
		fmt.Printf("Invite URL: %s\n", inviteUrl)
		fmt.Printf("Expires: %s\n", invite.ExpiresAt.Format(time.RFC3339))
	},
}

func init() {
	authCmd.AddCommand(bootstrapCeoCmd)
	rootCmd.AddCommand(authCmd)

	bootstrapCeoCmd.Flags().Bool("force", false, "Create new invite even if admin already exists")
	bootstrapCeoCmd.Flags().Int("expires-hours", 72, "Invite expiration window in hours")
	bootstrapCeoCmd.Flags().String("base-url", "", "Public base URL used to print invite link")
}
