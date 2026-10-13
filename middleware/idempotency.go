package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"eka-dev.cloud/transaction-service/lib"
	"eka-dev.cloud/transaction-service/utils/common"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

const (
	HeaderIdempotencyKey      = "Idempotency-Key"
	HeaderXIdempotencyKey     = "X-Idempotency-Key"
	HeaderIdempotentReplayed  = "Idempotent-Replayed"
	HeaderXIdempotentReplayed = "X-Idempotent-Replayed"

	DefaultLockTTL          = 60 * time.Second
	DefaultDataTTL          = 24 * time.Hour
	MaxIdempotencyKeyLength = 256
)

// IdempotencyConfig defines configuration options for Idempotency middleware
type IdempotencyConfig struct {
	LockTTL time.Duration
	DataTTL time.Duration
}

// IdempotencyRecord stores cached HTTP response and payload verification metadata
type IdempotencyRecord struct {
	StatusCode  int               `json:"statusCode"`
	Headers     map[string]string `json:"headers"`
	Body        string            `json:"body"`
	RequestHash string            `json:"requestHash"`
	CreatedAt   int64             `json:"createdAt"`
}

// Idempotency creates a Fiber middleware that ensures idempotent handling of requests.
// If an Idempotency-Key (or X-Idempotency-Key) header is provided:
// - A duplicate completed request returns the cached previous response with 'Idempotent-Replayed: true'.
// - A duplicate in-flight request returns HTTP 409 Conflict.
// - Re-using the same key with a different body returns HTTP 422 Unprocessable Entity.
// - Downstream services (e.g. gRPC calls to wallet-app or master-data-app) are never invoked on duplicate requests.
func Idempotency(configs ...IdempotencyConfig) fiber.Handler {
	cfg := IdempotencyConfig{
		LockTTL: DefaultLockTTL,
		DataTTL: DefaultDataTTL,
	}
	if len(configs) > 0 {
		if configs[0].LockTTL > 0 {
			cfg.LockTTL = configs[0].LockTTL
		}
		if configs[0].DataTTL > 0 {
			cfg.DataTTL = configs[0].DataTTL
		}
	}

	return func(c *fiber.Ctx) error {
		idempotencyKey := strings.TrimSpace(c.Get(HeaderIdempotencyKey))
		if idempotencyKey == "" {
			idempotencyKey = strings.TrimSpace(c.Get(HeaderXIdempotencyKey))
		}

		// If no idempotency key is supplied, proceed normally without idempotency enforcement
		if idempotencyKey == "" {
			return c.Next()
		}

		// Enforce maximum key length to prevent resource exhaustion
		if len(idempotencyKey) > MaxIdempotencyKeyLength {
			return response.BadRequest("Idempotency-Key must not exceed 256 characters", nil)
		}

		// Isolate idempotency per authenticated user so keys do not collide or leak cross-tenant
		userId := "anon"
		claims, err := common.GetClaimsFromLocals(c)
		if err == nil && claims != nil && claims.UserId > 0 {
			userId = fmt.Sprintf("%d", claims.UserId)
		}

		// Compute payload hash to verify integrity against reused keys with mutated payloads
		bodyBytes := c.Body()
		hash := sha256.Sum256(bodyBytes)
		requestHash := hex.EncodeToString(hash[:])

		route := c.Method() + ":" + c.Path()
		dataKey := fmt.Sprintf("idempotency:data:%s:%s:%s", userId, route, idempotencyKey)
		lockKey := fmt.Sprintf("idempotency:lock:%s:%s:%s", userId, route, idempotencyKey)

		// Fail-open gracefully if Redis is not configured or unavailable
		if lib.RedisClient == nil {
			slog.Warn("Redis client unavailable, bypassing idempotency cache check")
			return c.Next()
		}

		ctx := c.UserContext()
		if ctx == nil {
			ctx = context.Background()
		}

		// 1. Check if this request has already been completed and cached
		cachedData, err := lib.RedisClient.Get(ctx, dataKey).Result()
		if err == nil && cachedData != "" {
			var record IdempotencyRecord
			if parseErr := json.Unmarshal([]byte(cachedData), &record); parseErr == nil {
				// Detect payload mutation with same idempotency key
				if record.RequestHash != "" && record.RequestHash != requestHash {
					slog.Warn("Idempotency key payload mismatch",
						"userId", userId, "key", idempotencyKey, "path", c.Path())
					return response.UnprocessableEntity("Idempotency key was previously used with a different request payload", nil)
				}

				// Replay the cached response
				for headerName, headerVal := range record.Headers {
					c.Set(headerName, headerVal)
				}
				c.Set(HeaderIdempotentReplayed, "true")
				c.Set(HeaderXIdempotentReplayed, "true")
				slog.Info("Idempotent request replayed from cache",
					"userId", userId, "key", idempotencyKey, "status", record.StatusCode)
				return c.Status(record.StatusCode).SendString(record.Body)
			}
		} else if err != nil && err != redis.Nil {
			slog.Warn("Failed to query idempotency data from Redis", "error", err)
		}

		// 2. Acquire atomic lock for in-flight request
		acquired, err := lib.RedisClient.SetNX(ctx, lockKey, requestHash, cfg.LockTTL).Result()
		if err != nil {
			slog.Warn("Failed to acquire idempotency lock in Redis, proceeding without lock", "error", err)
			return c.Next()
		}

		if !acquired {
			// Another concurrent request with this key is already executing
			slog.Warn("Concurrent request with duplicate idempotency key rejected",
				"userId", userId, "key", idempotencyKey, "path", c.Path())
			return response.Conflict("A request with this idempotency key is currently being processed", nil)
		}

		// Ensure lock is released if an unexpected panic or early exit occurs
		lockReleased := false
		defer func() {
			if !lockReleased && lib.RedisClient != nil {
				_ = lib.RedisClient.Del(context.Background(), lockKey).Err()
			}
		}()

		// 3. Execute downstream handler chain
		execErr := c.Next()
		if execErr != nil {
			// On handler error, release lock immediately so client can retry
			lockReleased = true
			_ = lib.RedisClient.Del(ctx, lockKey).Err()
			return execErr
		}

		statusCode := c.Response().StatusCode()

		// 4. Cache response on success or client error (do not cache internal server errors 5xx)
		if statusCode >= 200 && statusCode < 500 {
			contentType := string(c.Response().Header.ContentType())
			if contentType == "" {
				contentType = fiber.MIMEApplicationJSONCharsetUTF8
			}

			record := IdempotencyRecord{
				StatusCode: statusCode,
				Headers: map[string]string{
					fiber.HeaderContentType: contentType,
				},
				Body:        string(c.Response().Body()),
				RequestHash: requestHash,
				CreatedAt:   time.Now().Unix(),
			}

			recordBytes, err := json.Marshal(record)
			if err == nil {
				_ = lib.RedisClient.Set(ctx, dataKey, recordBytes, cfg.DataTTL).Err()
			}
			c.Set(HeaderIdempotentReplayed, "false")
			c.Set(HeaderXIdempotentReplayed, "false")
		}

		// 5. Release transient in-flight lock
		lockReleased = true
		_ = lib.RedisClient.Del(ctx, lockKey).Err()

		return nil
	}
}
