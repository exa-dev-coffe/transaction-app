package db

import (
	"log"

	"eka-dev.cloud/transaction-service/config"
	"eka-dev.cloud/transaction-service/utils/constant"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/XSAM/otelsql"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

var DB *sqlx.DB

func init() {
	log.Println("databases init")
	dsn := config.Config.DBUrl
	if dsn == "" {
		log.Fatalln("Database DSN is not set")
	}

	// Register and open with otelsql
	sqlDb, err := otelsql.Open(constant.DialectPostgres, dsn, otelsql.WithAttributes())
	if err != nil {
		log.Fatalln("Failed to connect to database with otelsql:", err)
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
		log.Fatalln("Failed to ping database:", err)
	}

	log.Println("Database connection established")

	DB = DB
	// === Run migrations ===
	driver, err := postgres.WithInstance(DB.DB, &postgres.Config{})
	if err != nil {
		log.Fatalln("Failed to create migration driver:", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		"file://db/migrations", // path ke folder migrations kamu
		"postgres", driver,
	)
	if err != nil {
		log.Fatalln("Failed to init migrations:", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalln("Migration failed:", err)
	}

	log.Println("Migrations applied successfully")
}
