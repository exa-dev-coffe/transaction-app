package middleware_test

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"eka-dev.cloud/transaction-service/lib"
	"eka-dev.cloud/transaction-service/middleware"
	"eka-dev.cloud/transaction-service/utils/common"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

func setupTestRedis(t *testing.T) *miniredis.Miniredis {
	mr := miniredis.RunT(t)
	lib.RedisClient = redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	return mr
}

func setupTestFiberApp(handler fiber.Handler, userId int64) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: middleware.ErrorHandler,
	})

	// Optional auth mock middleware to inject user into Locals
	if userId > 0 {
		app.Use(func(c *fiber.Ctx) error {
			c.Locals("user", &common.Claims{
				UserId:   userId,
				FullName: "Test User",
				Email:    "test@example.com",
			})
			return c.Next()
		})
	}

	app.Post("/test", middleware.Idempotency(), handler)
	return app
}

func TestIdempotency_MissingKeyPassesThrough(t *testing.T) {
	_ = setupTestRedis(t)

	var executionCount int32
	app := setupTestFiberApp(func(c *fiber.Ctx) error {
		atomic.AddInt32(&executionCount, 1)
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"count": atomic.LoadInt32(&executionCount)})
	}, 100)

	// First request without idempotency header
	req1 := httptest.NewRequest("POST", "/test", bytes.NewBufferString(`{"action":"checkout"}`))
	req1.Header.Set("Content-Type", "application/json")
	resp1, err := app.Test(req1)
	if err != nil {
		t.Fatalf("Request 1 failed: %v", err)
	}
	if resp1.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", resp1.StatusCode)
	}
	if resp1.Header.Get(middleware.HeaderIdempotentReplayed) == "true" {
		t.Errorf("Expected Idempotent-Replayed not to be true for request without key")
	}

	// Second request without idempotency header
	req2 := httptest.NewRequest("POST", "/test", bytes.NewBufferString(`{"action":"checkout"}`))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := app.Test(req2)
	if err != nil {
		t.Fatalf("Request 2 failed: %v", err)
	}
	if resp2.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", resp2.StatusCode)
	}

	// Execution count should be 2 because no idempotency key was sent
	if atomic.LoadInt32(&executionCount) != 2 {
		t.Errorf("Expected execution count 2, got %d", atomic.LoadInt32(&executionCount))
	}
}

func TestIdempotency_KeyTooLong(t *testing.T) {
	_ = setupTestRedis(t)

	app := setupTestFiberApp(func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "ok"})
	}, 100)

	overlyLongKey := strings.Repeat("x", 257)
	req := httptest.NewRequest("POST", "/test", bytes.NewBufferString(`{"action":"checkout"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", overlyLongKey)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.StatusCode != 400 {
		t.Errorf("Expected HTTP 400 for key length > 256, got %d", resp.StatusCode)
	}
}

func TestIdempotency_SuccessAndReplay(t *testing.T) {
	_ = setupTestRedis(t)

	var executionCount int32
	app := setupTestFiberApp(func(c *fiber.Ctx) error {
		atomic.AddInt32(&executionCount, 1)
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"success": true,
			"orderId": 9999,
		})
	}, 100)

	key := "checkout-uuid-abc-123"
	payload := `{"tableId":5,"total":75000}`

	// 1. First execution
	req1 := httptest.NewRequest("POST", "/test", bytes.NewBufferString(payload))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", key)

	resp1, err := app.Test(req1)
	if err != nil {
		t.Fatalf("Request 1 failed: %v", err)
	}
	if resp1.StatusCode != 201 {
		t.Fatalf("Expected 201 Created, got %d", resp1.StatusCode)
	}
	if resp1.Header.Get(middleware.HeaderIdempotentReplayed) == "true" {
		t.Errorf("First execution should not have Idempotent-Replayed: true")
	}

	body1, _ := io.ReadAll(resp1.Body)

	// 2. Duplicate request with SAME key and SAME payload
	req2 := httptest.NewRequest("POST", "/test", bytes.NewBufferString(payload))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", key)

	resp2, err := app.Test(req2)
	if err != nil {
		t.Fatalf("Request 2 failed: %v", err)
	}
	if resp2.StatusCode != 201 {
		t.Fatalf("Expected 201 Created on replay, got %d", resp2.StatusCode)
	}
	if resp2.Header.Get(middleware.HeaderIdempotentReplayed) != "true" {
		t.Errorf("Expected Idempotent-Replayed to be 'true', got '%s'", resp2.Header.Get(middleware.HeaderIdempotentReplayed))
	}

	body2, _ := io.ReadAll(resp2.Body)
	if string(body1) != string(body2) {
		t.Errorf("Replayed response body mismatch.\nExpected: %s\nGot: %s", string(body1), string(body2))
	}

	// 3. Verify handler was ONLY executed once!
	if atomic.LoadInt32(&executionCount) != 1 {
		t.Errorf("Handler should have been executed only 1 time, got %d", atomic.LoadInt32(&executionCount))
	}
}

func TestIdempotency_PayloadMismatchReturns422(t *testing.T) {
	_ = setupTestRedis(t)

	var executionCount int32
	app := setupTestFiberApp(func(c *fiber.Ctx) error {
		atomic.AddInt32(&executionCount, 1)
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"orderId": 123})
	}, 100)

	key := "checkout-uuid-mismatch-test"

	// 1. Initial request with body A
	req1 := httptest.NewRequest("POST", "/test", bytes.NewBufferString(`{"amount": 50000}`))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", key)

	resp1, err := app.Test(req1)
	if err != nil || resp1.StatusCode != 201 {
		t.Fatalf("Request 1 failed: status=%d, err=%v", resp1.StatusCode, err)
	}

	// 2. Second request with SAME key but DIFFERENT body B
	req2 := httptest.NewRequest("POST", "/test", bytes.NewBufferString(`{"amount": 100000}`))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", key)

	resp2, err := app.Test(req2)
	if err != nil {
		t.Fatalf("Request 2 failed: %v", err)
	}
	if resp2.StatusCode != 422 {
		t.Fatalf("Expected HTTP 422 Unprocessable Entity on payload mismatch, got %d", resp2.StatusCode)
	}

	bodyBytes, _ := io.ReadAll(resp2.Body)
	if !strings.Contains(string(bodyBytes), "different request payload") {
		t.Errorf("Expected payload mismatch error message, got: %s", string(bodyBytes))
	}

	// Handler must not have been executed for the second request
	if atomic.LoadInt32(&executionCount) != 1 {
		t.Errorf("Expected execution count 1, got %d", atomic.LoadInt32(&executionCount))
	}
}

func TestIdempotency_ConcurrentInFlightReturns409(t *testing.T) {
	_ = setupTestRedis(t)

	app := setupTestFiberApp(func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"ok": true})
	}, 100)

	key := "checkout-uuid-concurrent"

	// Manually acquire lock in Redis as if another worker is currently executing
	lockKey := "idempotency:lock:100:POST:/test:" + key
	_ = lib.RedisClient.Set(context.Background(), lockKey, "dummy_hash", 30*time.Second).Err()

	// New incoming request with identical key while locked
	req := httptest.NewRequest("POST", "/test", bytes.NewBufferString(`{"tableId": 1}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.StatusCode != 409 {
		t.Fatalf("Expected HTTP 409 Conflict for in-flight concurrent request, got %d", resp.StatusCode)
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(bodyBytes), "currently being processed") {
		t.Errorf("Expected conflict error message, got: %s", string(bodyBytes))
	}
}

func TestIdempotency_UserIsolation(t *testing.T) {
	_ = setupTestRedis(t)

	var executionCount int32
	app := fiber.New(fiber.Config{
		ErrorHandler: middleware.ErrorHandler,
	})

	// Dynamic auth middleware reading user from header
	app.Use(func(c *fiber.Ctx) error {
		userIdHeader := c.Get("X-Test-User-Id")
		var userId int64 = 100
		if userIdHeader == "200" {
			userId = 200
		}
		c.Locals("user", &common.Claims{
			UserId:   userId,
			FullName: "User " + userIdHeader,
		})
		return c.Next()
	})

	app.Post("/test", middleware.Idempotency(), func(c *fiber.Ctx) error {
		atomic.AddInt32(&executionCount, 1)
		claims, _ := common.GetClaimsFromLocals(c)
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"userId": claims.UserId,
		})
	})

	sharedKey := "user-isolated-key-1"
	payload := `{"tableId": 2}`

	// 1. User 100 executes with key
	req1 := httptest.NewRequest("POST", "/test", bytes.NewBufferString(payload))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", sharedKey)
	req1.Header.Set("X-Test-User-Id", "100")
	resp1, _ := app.Test(req1)
	if resp1.StatusCode != 201 {
		t.Fatalf("User 100 request failed: %d", resp1.StatusCode)
	}

	// 2. User 200 executes with the SAME key
	req2 := httptest.NewRequest("POST", "/test", bytes.NewBufferString(payload))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", sharedKey)
	req2.Header.Set("X-Test-User-Id", "200")
	resp2, _ := app.Test(req2)
	if resp2.StatusCode != 201 {
		t.Fatalf("User 200 request failed: %d", resp2.StatusCode)
	}
	if resp2.Header.Get(middleware.HeaderIdempotentReplayed) == "true" {
		t.Errorf("User 200 request should NOT be replayed from User 100 cache!")
	}

	// Both users executed their own transactions
	if atomic.LoadInt32(&executionCount) != 2 {
		t.Errorf("Expected 2 separate executions for different users, got %d", atomic.LoadInt32(&executionCount))
	}
}

func TestIdempotency_HandlerErrorReleasesLock(t *testing.T) {
	_ = setupTestRedis(t)

	var attempts int32
	app := setupTestFiberApp(func(c *fiber.Ctx) error {
		cnt := atomic.AddInt32(&attempts, 1)
		if cnt == 1 {
			// First attempt fails with Internal Server Error
			return response.InternalServerError("Database transient failure", nil)
		}
		// Second attempt succeeds
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "succeeded"})
	}, 100)

	key := "retry-after-failure-key"
	payload := `{"action": "retry"}`

	// 1. First attempt fails
	req1 := httptest.NewRequest("POST", "/test", bytes.NewBufferString(payload))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", key)
	resp1, _ := app.Test(req1)
	if resp1.StatusCode != 500 {
		t.Fatalf("Expected 500, got %d", resp1.StatusCode)
	}

	// 2. Verify lock was released and not permanently stuck
	lockKey := "idempotency:lock:100:POST:/test:" + key
	exists, _ := lib.RedisClient.Exists(context.Background(), lockKey).Result()
	if exists > 0 {
		t.Errorf("Lock key should have been released after handler failure")
	}

	// 3. Client retries with the SAME key and should be able to execute again!
	req2 := httptest.NewRequest("POST", "/test", bytes.NewBufferString(payload))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", key)
	resp2, _ := app.Test(req2)
	if resp2.StatusCode != 200 {
		t.Fatalf("Expected 200 on retry, got %d", resp2.StatusCode)
	}

	if atomic.LoadInt32(&attempts) != 2 {
		t.Errorf("Expected 2 attempts, got %d", atomic.LoadInt32(&attempts))
	}
}

func TestIdempotency_XIdempotencyKeyHeader(t *testing.T) {
	_ = setupTestRedis(t)

	var executionCount int32
	app := setupTestFiberApp(func(c *fiber.Ctx) error {
		atomic.AddInt32(&executionCount, 1)
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"msg": "success"})
	}, 100)

	key := "alt-header-uuid-test"
	payload := `{"items": [1, 2]}`

	// First request using X-Idempotency-Key
	req1 := httptest.NewRequest("POST", "/test", bytes.NewBufferString(payload))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("X-Idempotency-Key", key)
	resp1, _ := app.Test(req1)
	if resp1.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d", resp1.StatusCode)
	}

	// Second request using X-Idempotency-Key
	req2 := httptest.NewRequest("POST", "/test", bytes.NewBufferString(payload))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Idempotency-Key", key)
	resp2, _ := app.Test(req2)
	if resp2.StatusCode != 200 {
		t.Fatalf("Expected 200 on replay, got %d", resp2.StatusCode)
	}
	if resp2.Header.Get(middleware.HeaderIdempotentReplayed) != "true" {
		t.Errorf("Expected Idempotent-Replayed: true when using X-Idempotency-Key")
	}

	if atomic.LoadInt32(&executionCount) != 1 {
		t.Errorf("Expected 1 execution, got %d", atomic.LoadInt32(&executionCount))
	}
}
