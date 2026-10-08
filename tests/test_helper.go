package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

		// 3. Singleton Redis Testcontainer
		redisContainer, rErr := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image:        "redis:7-alpine",
				ExposedPorts: []string{"6379/tcp"},
				WaitingFor:   wait.ForLog("Ready to accept connections"),
			},
			Started: true,
		})
		if rErr == nil && redisContainer != nil {
			endpoint, _ := redisContainer.Endpoint(ctx, "")
			config.Config.RedisUrl = endpoint
		}

		lib.InitRedis()
		primeStandardPermissions()

		sharedTeardown = func() {
			lib.ResetConnection()
			_ = dbConn.Close()
			_ = postgresContainer.Terminate(ctx)
			if rmqContainer != nil {
				_ = rmqContainer.Terminate(ctx)
			}
			if redisContainer != nil {
				_ = redisContainer.Terminate(ctx)
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

func primeStandardPermissions() {
	if lib.RedisClient == nil {
		return
	}
	ctx := context.Background()

	// Admin (role 1)
	adminPerms := map[string]common.PermissionAction{
		"catalog":   {View: true, Create: true, Edit: true, Delete: true},
		"category":  {View: true, Create: true, Edit: true, Delete: true},
		"table":     {View: true, Create: true, Edit: true, Delete: true},
		"voucher":   {View: true, Create: true, Edit: true, Delete: true},
		"promotion": {View: true, Create: true, Edit: true, Delete: true},
		"barista":   {View: true, Create: true, Edit: true, Delete: true},
		"order":     {View: true, Create: true, Edit: true, Delete: true},
		"inventory": {View: true, Create: true, Edit: true, Delete: true},
		"report":    {View: true, Create: true, Edit: true, Delete: true},
		"pos":       {View: true, Create: true, Edit: true, Delete: true},
	}
	adminBytes, _ := json.Marshal(adminPerms)
	lib.RedisClient.Set(ctx, "auth:role_permissions:1", string(adminBytes), 24*time.Hour)

	// User / Customer (role 2) - No management/staff permissions
	userPerms := map[string]common.PermissionAction{}
	userBytes, _ := json.Marshal(userPerms)
	lib.RedisClient.Set(ctx, "auth:role_permissions:2", string(userBytes), 24*time.Hour)

	// Barista (role 3)
	baristaPerms := map[string]common.PermissionAction{
		"catalog":   {View: true},
		"order":     {View: true, Edit: true},
		"inventory": {View: true, Edit: true},
		"report":    {View: true},
	}
	baristaBytes, _ := json.Marshal(baristaPerms)
	lib.RedisClient.Set(ctx, "auth:role_permissions:3", string(baristaBytes), 24*time.Hour)
}

func SetupMockExternalServices() *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		switch r.URL.Path {
		case "/api/internal/available-menus-table":
			idsParam := r.URL.Query().Get("ids")
			if strings.Contains(idsParam, "11") && !strings.Contains(idsParam, "10") {
				_, _ = w.Write([]byte(`{
					"success": true,
					"message": "Success",
					"data": [
						{
							"id": 11,
							"name": "Matcha Latte",
							"price": 30000,
							"effectivePrice": 24000,
							"discount": {
								"promotionId": 99,
								"promotionName": "Flash Promo 20%",
								"discountType": "PERCENTAGE",
								"discountValue": 20,
								"savings": 6000
							},
							"description": "Matcha",
							"photo": "matcha.jpg"
						}
					]
				}`))
			} else {
				_, _ = w.Write([]byte(`{
					"success": true,
					"message": "Success",
					"data": [
						{"id": 10, "name": "Espresso", "price": 25000, "description": "Coffee", "photo": "coffee.jpg"}
					]
				}`))
			}
		case "/api/internal/data-menus-table":
			_, _ = w.Write([]byte(`{
				"success": true,
				"message": "Success",
				"data": {
					"menus": [
						{"id": 10, "name": "Espresso", "price": 25000, "description": "Coffee", "photo": "coffee.jpg"},
						{
							"id": 11,
							"name": "Matcha Latte",
							"price": 30000,
							"effectivePrice": 24000,
							"discount": {
								"promotionId": 99,
								"promotionName": "Flash Promo 20%",
								"discountType": "PERCENTAGE",
								"discountValue": 20,
								"savings": 6000
							},
							"description": "Matcha",
							"photo": "matcha.jpg"
						}
					],
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
		case "/api/internal/pos/wallet/pay":
			_, _ = w.Write([]byte(`{
				"success": true,
				"message": "Payment successful",
				"data": {
					"success": true,
					"userId": 100,
					"customerName": "Test User",
					"customerEmail": "user@test.com",
					"amountPaid": 50000,
					"remainingBalance": 100000,
					"message": "Payment successful"
				}
			}`))
		case "/api/internal/pos/wallet/refund":
			_, _ = w.Write([]byte(`{"success": true, "message": "Refund successful"}`))
		case "/api/internal/pos/qris/charge":
			_, _ = w.Write([]byte(`{
				"success": true,
				"message": "QRIS generated",
				"data": {
					"orderId": "pos-qris-test-123",
					"qrString": "00020101021226590014ID.LINKAJA.WWW0118936009180000010002",
					"qrUrl": "https://api.sandbox.midtrans.com/v2/qris/pos-qris-test-123/qr-code",
					"paymentStatus": "PENDING"
				}
			}`))
		default:
			if strings.HasPrefix(r.URL.Path, "/api/internal/pos/qris/status/") {
				_, _ = w.Write([]byte(`{
					"success": true,
					"message": "Status retrieved",
					"data": {
						"transaction_status": "settlement",
						"fraud_status": "accept"
					}
				}`))
				return
			}
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
	voucher.NewHandler(app, voucherService, dbConn)

	transactionRepo := transaction.NewTransactionRepository(dbConn)
	transactionService := transaction.NewTransactionService(transactionRepo, voucherService, dbConn)
	transaction.NewHandler(app, transactionService, dbConn)

	app.All("*", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusNotFound).JSON(response.NotFound("Route not found", nil))
	})

	return app
}

func GenerateTestToken(userId int64, email, role string) string {
	roleId := 2
	if strings.EqualFold(role, "admin") {
		roleId = 1
	} else if strings.EqualFold(role, "barista") {
		roleId = 3
	}

	perms := make(map[string]common.PermissionAction)
	if roleId == 1 {
		features := []string{"catalog", "category", "table", "voucher", "promotion", "barista", "order", "inventory", "report", "role_management", "pos"}
		for _, f := range features {
			perms[f] = common.PermissionAction{View: true, Create: true, Edit: true, Delete: true}
		}
	} else if roleId == 3 {
		perms["catalog"] = common.PermissionAction{View: true}
		perms["order"] = common.PermissionAction{View: true, Edit: true}
		perms["inventory"] = common.PermissionAction{View: true, Edit: true}
		perms["report"] = common.PermissionAction{View: true}
	} else {
		// User / Customer (role 2) has no staff permissions
	}

	if lib.RedisClient != nil && len(perms) > 0 {
		permBytes, _ := json.Marshal(perms)
		lib.RedisClient.Set(context.Background(), fmt.Sprintf("auth:role_permissions:%d", roleId), string(permBytes), 24*time.Hour)
	}

	claims := common.Claims{
		FullName: "Test User",
		Email:    email,
		UserId:   userId,
		Type:     "ACCESS",
		Role:     role,
		RoleId:   roleId,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	secret := config.Config.SecretJwt
	if secret == "" {
		secret = "super-secret-jwt-key"
		config.Config.SecretJwt = secret
	}
	tokenString, _ := token.SignedString([]byte(secret))
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

	return app.Test(req, 5000)
}
