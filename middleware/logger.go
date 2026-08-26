package middleware

import (
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/trace"
)

// InitLogger initializes slog as default logger with JSONHandler and redirects stdlib log.
// If environment variable LOG_TO_FILE=true is set, it writes logs to both Console and logs/<serviceName>.log
func InitLogger(serviceName string) {
	var writer io.Writer = os.Stdout

	if os.Getenv("LOG_TO_FILE") == "true" {
		logDir := "logs"
		if err := os.MkdirAll(logDir, 0755); err == nil {
			logFilePath := filepath.Join(logDir, serviceName+".log")
			file, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
			if err == nil {
				writer = io.MultiWriter(os.Stdout, file)
			} else {
				slog.Error("Failed to open log file", "path", logFilePath, "error", err)
			}
		} else {
			slog.Error("Failed to create log directory", "dir", logDir, "error", err)
		}
	}

	logger := slog.New(slog.NewJSONHandler(writer, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})).With("app_name", serviceName)
	slog.SetDefault(logger)

	log.SetFlags(0)
	log.SetOutput(slog.NewLogLogger(logger.Handler(), slog.LevelInfo).Writer())
}

func init() {
	InitLogger("transaction-service")
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
