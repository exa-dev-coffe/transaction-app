package middleware

import (
	"eka-dev.cloud/transaction-service/config"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/gofiber/fiber/v2"
)

func RequireInternalSecret(c *fiber.Ctx) error {
	secret := c.Get("X-Internal-Token")
	if secret == "" || secret != config.Config.Secret {
		return c.Status(fiber.StatusForbidden).JSON(response.Forbidden("Forbidden: Invalid internal token", nil))
	}
	return c.Next()
}
