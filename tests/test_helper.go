package tests

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"eka-dev.cloud/transaction-service/config"
	"eka-dev.cloud/transaction-service/db"
	"eka-dev.cloud/transaction-service/lib"
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

var (
	sharedDB       *sqlx.DB
	sharedTeardown func()
	setupOnce      sync.Once
	setupErr       error
)

func SetupTestPostgresTransaction(t *testing.T) (*sqlx.DB, func()) {
	setupOnce.Do(func() {
		ctx := context.Background()

		// 1. Singleton PostgreSQL Testcontainer
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
			setupErr = fmt.Errorf("Docker/Testcontainers unavailable: %v", err)
			return
		}

		connStr, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			setupErr = fmt.Errorf("failed to get connection string: %v", err)
			return
		}

		dbConn, err := sqlx.Connect("postgres", connStr)
		if err != nil {
			setupErr = fmt.Errorf("failed to connect to test postgres database: %v", err)
			return
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
		sharedDB = dbConn

		// 2. Singleton RabbitMQ Testcontainer
		rmqReq := testcontainers.ContainerRequest{
			Image:        "rabbitmq:3-alpine",
			ExposedPorts: []string{"5672/tcp"},
			WaitingFor:   wait.ForLog("Server startup complete").WithStartupTimeout(45 * time.Second),
		}
		rmqContainer, rmqErr := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: rmqReq,
			Started:          true,
		})
		if rmqErr == nil {
			host, _ := rmqContainer.Host(ctx)
			port, _ := rmqContainer.MappedPort(ctx, "5672")
			config.Config.RabbitmqUrl = fmt.Sprintf("amqp://guest:guest@%s:%s/", host, port.Port())
		}

		sharedTeardown = func() {
			lib.ResetConnection()
			_ = dbConn.Close()
			_ = postgresContainer.Terminate(ctx)
			if rmqContainer != nil {
				_ = rmqContainer.Terminate(ctx)
			}
		}
	})

	if setupErr != nil {
		t.Skipf("Skipping integration test: %v", setupErr)
	}

	return sharedDB, func() {
		// Dummy teardown per test file - real teardown runs at process exit
	}
}

func SetupMockExternalServices() *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		switch r.URL.Path {
		case "/api/internal/available-menus-table":
			_, _ = w.Write([]byte(`{
				"success": true,
				"message": "Success",
				"data": [
					{"id": 10, "name": "Espresso", "price": 25000, "description": "Coffee", "photo": "coffee.jpg"}
				]
			}`))
		case "/api/internal/data-menus-table":
			_, _ = w.Write([]byte(`{
				"success": true,
				"message": "Success",
				"data": {
					"menus": [{"id": 10, "name": "Espresso", "price": 25000, "description": "Coffee", "photo": "coffee.jpg"}],
					"tables": [{"id": 1, "name": "Table 1"}, {"id": 3, "name": "Table 3"}]
				}
			}`))
		case "/api/internal/name-users":
			_, _ = w.Write([]byte(`{
				"success": true,
				"message": "Success",
				"data": [
					{"userId": 100, "fullName": "Test User", "email": "user@test.com"}
				]
			}`))
		case "/api/internal/pay":
			_, _ = w.Write([]byte(`{"success": true, "message": "Payment successful"}`))
		default:
			_, _ = w.Write([]byte(`{"success": true, "message": "Success"}`))
		}
	}))

	config.Config.ServiceMasterDataUrl = server.URL
	config.Config.ServiceWalletUrl = server.URL
	config.Config.ServiceAccountUrl = server.URL

	return server
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
