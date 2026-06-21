package database

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jmoiron/sqlx"
)

type migrationFile struct {
	name string
	sql  string
}

// checkMigration checks if a migration exists in the database. If it does not exist, it returns an error.
func checkMigration(db *sqlx.DB, name string) error {
	var exists bool
	if err := db.Get(&exists, "SELECT EXISTS(SELECT 1 FROM migrations WHERE name = ?)", name); err != nil {
		return fmt.Errorf("failed to check migration %s: %w", name, err)
	}
	if exists {
		return nil
	}
	return fmt.Errorf("migration %s does not exist", name)
}

func applyMigration(db *sqlx.DB, name string, sql string) error {
	if _, err := db.Exec(sql); err != nil {
		return fmt.Errorf("failed to apply migration %s: %w", name, err)
	}

	if _, err := db.Exec("INSERT INTO migrations (name) VALUES (?)", name); err != nil {
		return fmt.Errorf("failed to record migration %s: %w", name, err)
	}

	return nil
}

func applyMigrations(db *sqlx.DB, migrationsDir string) error {
	if err := ensureMigrationsTable(db); err != nil {
		return fmt.Errorf("failed to ensure migrations table: %w", err)
	}

	files, err := migrationFiles(migrationsDir)
	if err != nil {
		return fmt.Errorf("failed to load migration files: %w", err)
	}

	count := 0
	for _, file := range files {
		name := file.name
		if err := checkMigration(db, name); err != nil {
			if err := applyMigration(db, name, file.sql); err != nil {
				return fmt.Errorf("failed to apply migration %s: %w", name, err)
			}
			count++
		}
	}

	return nil
}

func migrationFiles(dir string) ([]migrationFile, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return make([]migrationFile, 0), nil
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return nil, fmt.Errorf("failed to load migration files: %w", err)
	}

	var migrations []migrationFile
	for _, file := range files {
		name := filepath.Base(file)
		sql, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read migration file %s: %w", name, err)
		}
		migrations = append(migrations, migrationFile{name: name, sql: string(sql)})
	}
	return migrations, nil
}

func ensureMigrationsTable(db *sqlx.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS migrations (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return fmt.Errorf("failed to ensure migration table: %w", err)
	}
	return nil
}

func GetMigrationsInfo(db *sqlx.DB, migrationsDir string) (*MigrationsInfo, error) {
	// Get migration files
	migrationFiles, err := migrationFiles(migrationsDir)
	if err != nil {
		return nil, err
	}
	migrationFilesStr := make([]string, 0, len(migrationFiles))
	for _, file := range migrationFiles {
		migrationFilesStr = append(migrationFilesStr, file.name)
	}

	// Get applied migrations
	rows, err := db.Query("SELECT name FROM migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	appliedMigrations := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		appliedMigrations = append(appliedMigrations, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &MigrationsInfo{
		MigrationFiles:    migrationFilesStr,
		AppliedMigrations: appliedMigrations,
	}, nil
}

type MigrationsInfo struct {
	MigrationFiles    []string
	AppliedMigrations []string
}
