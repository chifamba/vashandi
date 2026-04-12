// Package db provides the embedded SQL migration files for OpenBrain.
package db

import "embed"

// Migrations holds all versioned golang-migrate SQL files under db/migrations/.
// Import this package and pass Migrations to iofs.New to run the migrations.
//
//go:embed migrations/*.sql
var Migrations embed.FS
