package middleware

import (
	"errors"
	"log/slog"
	"time"

	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/trace"
)

// Middleware global error handler
func ErrorHandler(c *fiber.Ctx, err error) error {
	reqId := c.Locals("requestid")
	var reqIdStr string
	if reqId != nil {
		reqIdStr = reqId.(string)
	}

	attrs := []any{
		slog.String("request_id", reqIdStr),
		slog.String("error", err.Error()),
		slog.String("path", c.Path()),
	}

	span := trace.SpanFromContext(c.UserContext())
	if span.SpanContext().IsValid() {
		attrs = append(attrs,
			slog.String("trace_id", span.SpanContext().TraceID().String()),
			slog.String("span_id", span.SpanContext().SpanID().String()),
		)
	}

	slog.Error("Unhandled error occurred", attrs...)

	// Kalau error sudah tipe *AppError, balikin langsung
	var appErr *response.AppError
	if errors.As(err, &appErr) {
		return c.Status(appErr.Code).JSON(response.Response{
			Message:   appErr.Message,
			Data:      appErr.Data,
			Success:   false,
			TimeStamp: time.Now(),
		})
	}

	// Kalau error bukan AppError → fallback ke Internal Server Error
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
		"message":   "Internal server error",
		"data":      nil,
		"success":   false,
		"timestamp": time.Now(),
	})
}
