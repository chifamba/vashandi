package main

import (
	"fmt"
	"log/slog"
)

func main() {
	fmt.Println("OpenBrain service starting...")
	slog.Info("Initializing database...")
	db := InitDB()
	slog.Info("Database initialized successfully", "db", db)
}
