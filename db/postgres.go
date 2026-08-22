package db

import (
	"log/slog"
	"os"

	"eka-dev.cloud/transaction-service/config"
	"eka-dev.cloud/transaction-service/utils/constant"
	"github.com/XSAM/otelsql"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

var DB *sqlx.DB

func init() {
	slog.Info("databases init")
	dsn := config.Config.DBUrl
	if dsn == "" {
		slog.Warn("Database DSN is not set, skipping default DB initialization for test environment")
		return
	}

	// Register and open with otelsql
	sqlDb, err := otelsql.Open(constant.DialectPostgres, dsn, otelsql.WithAttributes())
	if err != nil {
		slog.Error("Failed to connect to database with otelsql", "error", err)
		os.Exit(1)
	}

	// Tell sqlx to treat this connection as standard 'postgres' for named query binding ($1, $2, etc.)
	DB = sqlx.NewDb(sqlDb, "postgres")

	// Db configuration
	DB.SetMaxOpenConns(config.Config.DBMaxPoolSize)
	DB.SetMaxIdleConns(config.Config.DBMinPoolSize)
	DB.SetConnMaxIdleTime(config.Config.DBIdleTimeout)
	DB.SetConnMaxLifetime(config.Config.DBMaxConnLifetime)

	err = DB.Ping()
	if err != nil {
		slog.Error("Failed to ping database", "error", err)
		os.Exit(1)
	}

	slog.Info("Database connection established")

	// === Run migrations ===
	driver, err := postgres.WithInstance(DB.DB, &postgres.Config{})
	if err != nil {
		slog.Error("Failed to create migration driver", "error", err)
		os.Exit(1)
	}

	m, err := migrate.NewWithDatabaseInstance(
		"file://db/migrations", // path ke folder migrations kamu
		"postgres", driver,
	)
	if err != nil {
		slog.Error("Failed to init migrations", "error", err)
		os.Exit(1)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		slog.Error("Migration failed", "error", err)
		os.Exit(1)
	}

	slog.Info("Migrations applied successfully")
}
