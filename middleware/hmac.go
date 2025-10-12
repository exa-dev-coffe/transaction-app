package middleware

import (
	"fmt"
	"time"

	"eka-dev.cloud/transaction-service/utils"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"
)

func ValidateSignature(c *fiber.Ctx) error {

	signature := c.Get("X-Signature")
	timestamp := c.Get("X-Timestamp")

	if signature == "" || timestamp == "" {
		log.Error("Missing signature or timestamp")
		return response.Unauthorized("Missing signature or timestamp", nil)
	}

	// Pastikan timestamp tidak lebih dari 5 menit untuk hindari replay attack
	reqTime, err := time.Parse(time.RFC3339, timestamp)
	if err != nil || time.Since(reqTime) > 5*time.Minute {
		log.Error("Invalid or expired timestamp:", err)
		return response.Unauthorized("Invalid or expired timestamp", nil)
	}

	// Ambil data penting buat di-hash
	body := string(c.Body())
	query := c.Context().URI().QueryArgs().String()

	// Buat message string-nya
	message := fmt.Sprintf("%s%s%s", query, timestamp, body)

	// Buat HMAC-nya
	err = utils.VerifySignature(message, signature)
	if err != nil {
		return err
	}

	return c.Next()
}
