package lib

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// InitTracer initializes an OTLP gRPC OpenTelemetry TracerProvider
func InitTracer(serviceName string) (func(context.Context) error, error) {
	ctx := context.Background()

	envServiceName := os.Getenv("OTEL_SERVICE_NAME")
	if envServiceName != "" {
		serviceName = envServiceName
	}

	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		endpoint = "alloy.observability.svc.cluster.local:4317"
	}
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
		),
		resource.WithSchemaURL(semconv.SchemaURL),
	)
	if err != nil {
		slog.Warn("Failed to create OTel resource", slog.String("error", err.Error()))
		res = resource.Default()
	}

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		otlptracegrpc.WithTimeout(5*time.Second),
	)
	if err != nil {
		slog.Warn("Failed to create OTLP trace exporter", slog.String("error", err.Error()), slog.String("endpoint", endpoint))
		return func(context.Context) error { return nil }, nil
	}

	bsp := sdktrace.NewBatchSpanProcessor(exporter,
		sdktrace.WithBatchTimeout(5*time.Second),
		sdktrace.WithMaxExportBatchSize(512),
	)

	// Configure sampler (default 100%, or percentage from env var)
	var sampler sdktrace.Sampler = sdktrace.AlwaysSample()
	samplerArg := os.Getenv("OTEL_TRACES_SAMPLER_ARG")
	if samplerArg != "" {
		if ratio, err := strconv.ParseFloat(samplerArg, 64); err == nil {
			// ParentBased ensures that if the caller (e.g. Kong/Frontend) sampled the trace, 
			// this service will also sample it, keeping the distributed trace intact.
			sampler = sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))
		} else {
			slog.Warn("Invalid OTEL_TRACES_SAMPLER_ARG, falling back to 100% sampling", slog.String("error", err.Error()))
		}
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sampler),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
	)

	otel.SetTracerProvider(tp)

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	slog.Info("OpenTelemetry tracer initialized",
		slog.String("service_name", serviceName),
		slog.String("otlp_endpoint", endpoint),
	)

	return tp.Shutdown, nil
}
