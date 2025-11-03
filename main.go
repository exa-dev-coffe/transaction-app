package main

import (
	"log"

	"eka-dev.cloud/transaction-service/config"
	"eka-dev.cloud/transaction-service/db"
	_ "eka-dev.cloud/transaction-service/db"
	"eka-dev.cloud/transaction-service/lib"
	_ "eka-dev.cloud/transaction-service/lib"
	"eka-dev.cloud/transaction-service/middleware"
	"eka-dev.cloud/transaction-service/modules/transaction"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/jmoiron/sqlx"
)

func main() {
	// Load env
	initiator()

	defer func(db *sqlx.DB) {
		err := db.Close()
		if err != nil {
			log.Println("Error closing database connection:", err)
		}
	}(db.DB)

}

func initiator() {
	// Initialize the fiber app
	fiberApp := fiber.New(fiber.Config{
		ErrorHandler: middleware.ErrorHandler,
	})

	fiberApp.Use(logger.New(logger.Config{
		Format:     "[${time}] ${ip} ${method} ${path} - ${status} (${latency})\n",
		TimeFormat: "2006-01-02 15:04:05",
		TimeZone:   "Asia/Jakarta",
	}))

	fiberApp.Get("/health", func(c *fiber.Ctx) error {
		err := db.DB.Ping()
		if err != nil {
			log.Println("Database ping failed:", err)
			return c.Status(fiber.StatusInternalServerError).JSON(response.InternalServerError("Database connection error", nil))
		}
		_, err = lib.GetChannel()
		if err != nil {
			log.Println("RabbitMQ connection failed:", err)
			return c.Status(fiber.StatusInternalServerError).JSON(response.InternalServerError("RabbitMQ connection error", nil))
		}
		return c.Status(fiber.StatusOK).JSON(response.Success("OK", nil))
	})

	fiberApp.Use(cors.New(cors.Config{
		AllowOrigins: config.Config.AllowedOrigins,
		AllowHeaders: "Origin, Content-Type, Accept, Authorization, X-Timestamp, X-Signature",
		AllowMethods: "GET, POST, PUT, DELETE, OPTIONS, PATCH",
	}))

	// Initialize routes
	// Menus
	transaction.NewHandler(fiberApp, db.DB)

	fiberApp.All("*", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusNotFound).JSON(response.NotFound("Route not found", nil))
	})

	err := fiberApp.Listen(config.Config.Port)
	if err != nil {
		log.Fatalln("Failed to start server:", err)
		return
	}
}
