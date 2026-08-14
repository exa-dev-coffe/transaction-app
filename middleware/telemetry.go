package middleware

import (
	"fmt"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "fiber-http"

// TraceMiddleware instruments incoming HTTP requests in Fiber with OpenTelemetry
func TraceMiddleware() fiber.Handler {
	tracer := otel.GetTracerProvider().Tracer(tracerName)
	propagator := otel.GetTextMapPropagator()

	return func(c *fiber.Ctx) error {
		// Extract incoming W3C Trace Context headers (traceparent, tracestate)
		reqHeader := make(http.Header)
		c.Request().Header.VisitAll(func(k, v []byte) {
			reqHeader.Add(string(k), string(v))
		})
		ctx := propagator.Extract(c.Context(), propagation.HeaderCarrier(reqHeader))

		spanName := fmt.Sprintf("%s %s", c.Method(), c.Path())

		attrs := []attribute.KeyValue{
			semconv.HTTPRequestMethodKey.String(c.Method()),
			semconv.HTTPRouteKey.String(c.Path()),
			semconv.URLPathKey.String(c.Path()),
			semconv.ClientAddressKey.String(c.IP()),
			semconv.ServerAddressKey.String(c.Hostname()),
		}

		ctx, span := tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attrs...),
		)
		defer span.End()

		// Inject active span context into Fiber context
		c.SetUserContext(ctx)

		spanCtx := span.SpanContext()
		if spanCtx.IsValid() {
			c.Set("X-Trace-Id", spanCtx.TraceID().String())
			c.Set("X-Span-Id", spanCtx.SpanID().String())
		}

		err := c.Next()

		status := c.Response().StatusCode()
		span.SetAttributes(semconv.HTTPResponseStatusCodeKey.Int(status))

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else if status >= 500 {
			span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", status))
		} else {
			span.SetStatus(codes.Ok, "")
		}

		return err
	}
}
