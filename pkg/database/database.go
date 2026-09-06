package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pressly/goose/v3"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Config describes the database connection. Driver "sqlite" (default) uses a
// local file, driver "postgres" connects to a PostgreSQL server and applies
// the goose migrations from the "postgres" subdirectory of MigrationsDir.
type Config struct {
	Driver        string
	File          string
	MigrationsDir string
	Host          string
	Port          string
	User          string
	Password      string
	Name          string
	SSLMode       string
}

func Open(cfg Config) (*gorm.DB, error) {
	switch strings.ToLower(cfg.Driver) {
	case "postgres", "pg", "postgresql":
		return openPostgres(cfg)
	default:
		return openSQLite(cfg)
	}
}

func openPostgres(cfg Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Name, cfg.SSLMode,
	)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
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

	if cfg.MigrationsDir != "" {
		if err := migrate(sqlDB, cfg.MigrationsDir, "postgres"); err != nil {
			return nil, err
		}
	}

	return db, nil
}

func openSQLite(cfg Config) (*gorm.DB, error) {
	if _, err := os.Stat(cfg.File); err != nil {
		dir := filepath.Dir(cfg.File)
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return nil, fmt.Errorf("failed to create database directory %s: %w", cfg.File, err)
		}
	}

	db, err := gorm.Open(sqlite.Open(cfg.File), &gorm.Config{
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

	if cfg.MigrationsDir != "" {
		if err := migrate(sqlDB, cfg.MigrationsDir, "sqlite3"); err != nil {
			return nil, err
		}
	}

	return db, nil
}

func migrate(db *sql.DB, migrationsDir, dialect string) error {
	goose.SetDialect(dialect)
	goose.SetBaseFS(os.DirFS(migrationsDir))
	goose.SetLogger(goose.NopLogger())

	// goose applies the SQL nodes found in the target FSM base. Sqlite
	// migrations live at the root of migrationsDir ("."), postgres ones in the
	// "postgres" subdirectory. Unrelated directories are ignored.
	target := "."
	if dialect == "postgres" {
		target = "postgres"
	}
	if err := goose.Up(db, target); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}
	return nil
}
