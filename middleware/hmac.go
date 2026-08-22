package middleware

import (
	"fmt"
	"log/slog"
	"time"

	"eka-dev.cloud/transaction-service/utils"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/gofiber/fiber/v2"
)

func ValidateSignature(c *fiber.Ctx) error {

	signature := c.Get("X-Signature")
	timestamp := c.Get("X-Timestamp")

	if signature == "" || timestamp == "" {
		slog.Error("Missing signature or timestamp")
		return response.Unauthorized("Missing signature or timestamp", nil)
	}

	// Ensure timestamp is not older than 5 minutes to prevent replay attacks
	reqTime, err := time.Parse(time.RFC3339, timestamp)
	if err != nil || time.Since(reqTime) > 5*time.Minute {
		slog.Error("Invalid or expired timestamp", "error", err)
		return response.Unauthorized("Invalid or expired timestamp", nil)
	}

	// Get essential data to hash
	body := string(c.Body())
	query := c.Context().URI().QueryArgs().String()

	// Construct the message string
	message := fmt.Sprintf("%s%s%s", query, timestamp, body)

	slog.Info("HMAC Validation Data",
		slog.String("payload", message),
		slog.String("body", body),
	)

	// Verify the HMAC
	err = utils.VerifySignature(message, signature)
	if err != nil {
		return err
	}

	return c.Next()
}
