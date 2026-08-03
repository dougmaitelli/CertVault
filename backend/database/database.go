package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Database owns the application's database connection and schema lifecycle.
type Database struct {
	orm *gorm.DB
	sql *sql.DB
}

// Open connects to SQLite, configures its connection pool, and applies migrations.
func Open(path string) (*Database, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	dsn := fmt.Sprintf("%s?_busy_timeout=5000&_foreign_keys=on&_journal_mode=WAL", path)
	orm, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	sqlDB, err := orm.DB()
	if err != nil {
		return nil, fmt.Errorf("access connection pool: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	db := &Database{orm: orm, sql: sqlDB}
	if err := db.migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return db, nil
}

func (d *Database) migrate() error {
	if err := d.orm.AutoMigrate(
		&Certificate{},
		&CertificateVersion{},
		&Job{},
		&APIKey{},
		&APIKeyCertificate{},
		&AuditEvent{},
		&Setting{},
	); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	return nil
}

// ORM returns the configured GORM connection for repository packages.
func (d *Database) ORM() *gorm.DB {
	return d.orm
}

// Ping verifies that the database connection is available.
func (d *Database) Ping(ctx context.Context) error {
	return d.sql.PingContext(ctx)
}

// Close releases the underlying SQL connection pool.
func (d *Database) Close() error {
	return d.sql.Close()
}
