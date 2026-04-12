package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strconv"
	"time"

	migrate "github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	obdb "github.com/chifamba/vashandi/openbrain/db"
	"github.com/chifamba/vashandi/openbrain/internal/brain"
)

// InitDB creates a pgxpool-backed database connection, runs golang-migrate
// SQL migrations, and then returns a GORM DB handle using the same pool.
func InitDB() *gorm.DB {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://paperclip:paperclip@localhost:5432/paperclip?sslmode=disable"
	}

	// --- pgxpool: configurable connection pool ---
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		log.Fatalf("failed to parse DATABASE_URL: %v", err)
	}
	poolCfg.MaxConns = envInt32("DB_MAX_CONNS", 20)
	poolCfg.MinConns = envInt32("DB_MIN_CONNS", 2)
	poolCfg.MaxConnIdleTime = envDuration("DB_MAX_CONN_IDLE_SECS", 5*time.Minute)
	poolCfg.MaxConnLifetime = envDuration("DB_MAX_CONN_LIFETIME_SECS", 30*time.Minute)

	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		log.Fatalf("failed to create connection pool: %v", err)
	}

	// Convert pgxpool → database/sql.DB (used by both golang-migrate and GORM).
	sqlDB := stdlib.OpenDBFromPool(pool)

	// --- golang-migrate: run versioned SQL migrations ---
	if err := runMigrations(sqlDB); err != nil {
		log.Fatalf("failed to run database migrations: %v", err)
	}

	// --- GORM: open from existing sql.DB (reuses the pool) ---
	db, err := gorm.Open(gormpostgres.New(gormpostgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to open gorm db: %v", err)
	}

	// AutoMigrate handles any model changes not yet covered by migration files
	// (additive only; for SQLite in tests it also bootstraps the schema).
	service := brain.NewService(db)
	if err := service.AutoMigrate(); err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}
	return db
}

// runMigrations applies all pending golang-migrate SQL migrations embedded in
// the openbrain/db/migrations directory.
func runMigrations(sqlDB *sql.DB) error {
	src, err := iofs.New(obdb.Migrations, "migrations")
	if err != nil {
		return fmt.Errorf("creating migration source: %w", err)
	}
	driver, err := migratepostgres.WithInstance(sqlDB, &migratepostgres.Config{})
	if err != nil {
		return fmt.Errorf("creating migration driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "postgres", driver)
	if err != nil {
		return fmt.Errorf("creating migrator: %w", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("applying migrations: %w", err)
	}
	v, _, _ := m.Version()
	slog.Info("database migrations applied", "version", v)
	return nil
}

// envInt32 reads an integer environment variable, falling back to def.
func envInt32(key string, def int32) int32 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil {
			return int32(n)
		}
	}
	return def
}

// envDuration reads a duration (in whole seconds) environment variable,
// falling back to def.
func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if secs, err := strconv.ParseInt(v, 10, 64); err == nil {
			return time.Duration(secs) * time.Second
		}
	}
	return def
}
