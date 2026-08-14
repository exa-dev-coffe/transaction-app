package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"eka-dev.cloud/transaction-service/utils/common"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/gofiber/fiber/v2/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

func InternalRequest(signature string, timestamp string, url string, method string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		log.Error("Failed to create request:", err)
		return nil, response.InternalServerError("Internal Server Error", nil)
	}

	req.Header.Add("x-signature", signature)
	req.Header.Add("x-timestamp", timestamp)
	req.Header.Set("Content-Type", "application/json")

	tracer := otel.GetTracerProvider().Tracer("http-client")
	ctx, span := tracer.Start(context.Background(), fmt.Sprintf("HTTP %s", method),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			semconv.HTTPRequestMethodKey.String(method),
			semconv.URLFullKey.String(url),
		),
	)
	defer span.End()

	// Inject W3C Trace Context (traceparent, tracestate) into outgoing HTTP headers
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req.WithContext(ctx))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		log.Error("Failed to send request:", err)
		return nil, response.InternalServerError("Internal Server Error", nil)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Error("Failed to close response body:", err)
		}
	}(res.Body)

	span.SetAttributes(semconv.HTTPResponseStatusCodeKey.Int(res.StatusCode))
	if res.StatusCode >= 400 {
		span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", res.StatusCode))
	} else {
		span.SetStatus(codes.Ok, "")
	}

	if res.StatusCode != http.StatusOK {
		resBody, err := io.ReadAll(res.Body)
		if err != nil {
			log.Error("Failed to read response body:", err)
			return nil, response.InternalServerError("Internal Server Error", nil)
		}
		log.Errorf("Received non-OK response: %s, body: %s", res.Status, string(resBody))

		var errorResponse common.InternalResponse

		err = json.Unmarshal(resBody, &errorResponse)
		if err != nil {
			log.Error("Failed to unmarshal error response:", err)
			return nil, response.InternalServerError("Internal Server Error", nil)
		}

		return nil, response.CustomError(res.StatusCode, errorResponse.Message, nil)
	}

	//Process the response body as needed
	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		log.Error("Failed to read response body:", err)
		return nil, response.InternalServerError("Internal Server Error", nil)
	}

	return resBody, nil
}
