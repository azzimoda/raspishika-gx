package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pressly/goose/v3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Open(file, migrationsDir string) (*gorm.DB, error) {
	if _, err := os.Stat(file); err != nil {
		dir := filepath.Dir(file)
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return nil, fmt.Errorf("failed to create database directory %s: %w", file, err)
		}
	}

	db, err := gorm.Open(sqlite.Open(file), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database handle: %w", err)
	}
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	if migrationsDir != "" {
		if err := migrate(sqlDB, migrationsDir); err != nil {
			return nil, err
		}
	}

	return db, nil
}

func migrate(db *sql.DB, migrationsDir string) error {
	goose.SetDialect("sqlite3")
	goose.SetBaseFS(os.DirFS(migrationsDir))
	goose.SetLogger(goose.NopLogger())

	if err := goose.Up(db, "."); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}
	return nil
}
