package middleware

import (
	"log/slog"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/trace"
)

// InitLogger initializes slog as default logger with JSONHandler
func InitLogger(serviceName string) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})).With("app_name", serviceName)
	slog.SetDefault(logger)
}

// RequestLogger middleware logs HTTP requests using slog with request_id
func RequestLogger() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		err := c.Next()

		latency := time.Since(start)
		reqId := c.Locals("requestid")
		
		var reqIdStr string
		if reqId != nil {
			reqIdStr = reqId.(string)
		}

		status := c.Response().StatusCode()
		msg := "HTTP Request"

		attrs := []slog.Attr{
			slog.String("request_id", reqIdStr),
			slog.String("method", c.Method()),
			slog.String("path", c.Path()),
			slog.Int("status", status),
			slog.String("latency", latency.String()),
			slog.String("ip", c.IP()),
		}

		span := trace.SpanFromContext(c.UserContext())
		if span.SpanContext().IsValid() {
			attrs = append(attrs,
				slog.String("trace_id", span.SpanContext().TraceID().String()),
				slog.String("span_id", span.SpanContext().SpanID().String()),
			)
		}

		if err != nil {
			attrs = append(attrs, slog.String("error", err.Error()))
			slog.LogAttrs(c.Context(), slog.LevelError, msg, attrs...)
		} else if status >= 400 {
			slog.LogAttrs(c.Context(), slog.LevelWarn, msg, attrs...)
		} else {
			slog.LogAttrs(c.Context(), slog.LevelInfo, msg, attrs...)
		}

		return err
	}
}
