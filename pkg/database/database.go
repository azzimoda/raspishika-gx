package database

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func Open(file, migrationsDir string) (*sqlx.DB, error) {
	if _, err := os.Stat(file); err != nil {
		dir := filepath.Dir(file)
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return nil, fmt.Errorf("failed to create database directory %s: %w", file, err)
		}
	}

	db, err := sqlx.Open("sqlite3", file)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	if migrationsDir != "" {
		if err := applyMigrations(db, migrationsDir); err != nil {
			return nil, err
		}
	}

	return db, nil
}
