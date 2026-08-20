package tests

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"eka-dev.cloud/transaction-service/config"
	"eka-dev.cloud/transaction-service/db"
	"eka-dev.cloud/transaction-service/middleware"
	"eka-dev.cloud/transaction-service/modules/transaction"
	"eka-dev.cloud/transaction-service/modules/voucher"
	"eka-dev.cloud/transaction-service/utils/common"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func SetupTestPostgresTransaction(t *testing.T) (*sqlx.DB, func()) {
	ctx := context.Background()
	postgresContainer, err := postgres.Run(ctx,
		"postgres:15-alpine",
		postgres.WithDatabase("transaction_test"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		t.Skipf("Skipping integration test: Docker/Testcontainers unavailable: %v", err)
	}

	connStr, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	dbConn, err := sqlx.Connect("postgres", connStr)
	if err != nil {
		t.Fatalf("failed to connect to test postgres database: %v", err)
	}

	// Apply all .up.sql database schema migrations
	migrationFiles, _ := filepath.Glob("../db/migrations/*.up.sql")
	if len(migrationFiles) == 0 {
		migrationFiles, _ = filepath.Glob("db/migrations/*.up.sql")
	}
	sort.Strings(migrationFiles)
	for _, f := range migrationFiles {
		sqlContent, err := os.ReadFile(f)
		if err == nil && len(sqlContent) > 0 {
			_, _ = dbConn.Exec(string(sqlContent))
		}
	}

	// Set package db.DB global pointer to real test DB
	db.DB = dbConn

	teardown := func() {
		_ = dbConn.Close()
		_ = postgresContainer.Terminate(ctx)
	}

	return dbConn, teardown
}

func SetupTestApp(dbConn *sqlx.DB) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: middleware.ErrorHandler,
	})

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(response.Success("OK", nil))
	})

	// Real Handlers & Services! (No Mocks!)
	voucherRepo := voucher.NewVoucherRepository(dbConn)
	voucherService := voucher.NewVoucherService(voucherRepo, dbConn)
	voucher.NewHandler(app, voucherService)

	transactionRepo := transaction.NewTransactionRepository(dbConn)
	transactionService := transaction.NewTransactionService(transactionRepo, voucherService, dbConn)
	transaction.NewHandler(app, transactionService, dbConn)

	app.All("*", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusNotFound).JSON(response.NotFound("Route not found", nil))
	})

	return app
}

func GenerateTestToken(userId int64, email, role string) string {
	claims := common.Claims{
		FullName: "Test User",
		Email:    email,
		UserId:   userId,
		Type:     "ACCESS",
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(config.Config.SecretJwt))
	return tokenString
}

func ExecuteTestRequest(app *fiber.App, method, url string, body []byte, token string) (*http.Response, error) {
	var req *http.Request
	if len(body) > 0 {
		req = httptest.NewRequest(method, url, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, url, nil)
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	return app.Test(req)
}
