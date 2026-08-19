package main

import (
	"context"
	"log"

	"eka-dev.cloud/transaction-service/config"
	"eka-dev.cloud/transaction-service/db"
	_ "eka-dev.cloud/transaction-service/db"
	"eka-dev.cloud/transaction-service/lib"
	_ "eka-dev.cloud/transaction-service/lib"
	"eka-dev.cloud/transaction-service/middleware"
	"eka-dev.cloud/transaction-service/modules/transaction"
	"eka-dev.cloud/transaction-service/modules/voucher"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/jmoiron/sqlx"
)

func main() {
	middleware.InitLogger("transaction-service")
	
	shutdown, err := lib.InitTracer("transaction-service")
	if err == nil && shutdown != nil {
		defer shutdown(context.Background())
	}

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
	// Initialize Asynq Client
	lib.InitAsynq()

	// Initialize the fiber app
	fiberApp := fiber.New(fiber.Config{
		ErrorHandler: middleware.ErrorHandler,
	})

	fiberApp.Use(requestid.New())
	fiberApp.Use(middleware.TraceMiddleware())
	fiberApp.Use(middleware.RequestLogger())

	fiberApp.Get("/health", func(c *fiber.Ctx) error {
		err := db.DB.Ping()
		if err != nil {
			log.Println("Database ping failed:", err)
			return c.Status(fiber.StatusInternalServerError).JSON(response.InternalServerError("Database connection error", nil))
		}
		err = lib.HealthCheck()
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

	// Initialize modules
	voucherRepo := voucher.NewVoucherRepository(db.DB)
	voucherService := voucher.NewVoucherService(voucherRepo, db.DB)
	voucher.NewHandler(fiberApp, voucherService)

	transactionRepo := transaction.NewTransactionRepository(db.DB)
	transactionService := transaction.NewTransactionService(transactionRepo, voucherService, db.DB)
	transaction.NewHandler(fiberApp, transactionService, db.DB)

	fiberApp.All("*", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusNotFound).JSON(response.NotFound("Route not found", nil))
	})

	err := fiberApp.Listen(config.Config.Port)
	if err != nil {
		log.Fatalln("Failed to start server:", err)
		return
	}
}
