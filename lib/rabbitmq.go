package lib

import (
	"context"
	"fmt"
	"sync"
	"time"

	"eka-dev.cloud/transaction-service/config"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/gofiber/fiber/v2/log"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// AmqpHeaderCarrier adapts amqp.Table to OpenTelemetry TextMapCarrier
type AmqpHeaderCarrier amqp.Table

func (c AmqpHeaderCarrier) Get(key string) string {
	if val, ok := c[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func (c AmqpHeaderCarrier) Set(key string, value string) {
	c[key] = value
}

func (c AmqpHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

var (
	conn     *amqp.Connection
	connOnce sync.Once
	mu       sync.Mutex
)

type ExchangeType string

const (
	ExchangeDirect  ExchangeType = "direct"
	ExchangeTopic   ExchangeType = "topic"
	ExchangeFanout  ExchangeType = "fanout"
	ExchangeHeaders ExchangeType = "headers"
)

// GetConnection -> dapet koneksi dengan auto-retry
func GetConnection() *amqp.Connection {
	mu.Lock()
	defer mu.Unlock()

	if conn != nil && !conn.IsClosed() {
		return conn
	}

	if config.Config.RabbitmqUrl == "" {
		return nil
	}

	// retry loop kalau gagal
	for {
		c, err := amqp.Dial(config.Config.RabbitmqUrl)
		if err != nil {
			log.Error("❌ Failed to connect to RabbitMQ, retrying in 5s:", err)
			time.Sleep(5 * time.Second)
			continue
		}
		conn = c
		log.Info("✅ Connected to RabbitMQ")
		break
	}

	return conn
}

func ResetConnection() {
	mu.Lock()
	defer mu.Unlock()
	if conn != nil {
		_ = conn.Close()
		conn = nil
	}
}

// GetChannel -> bikin channel baru (safe untuk goroutine)
func GetChannel() (*amqp.Channel, error) {
	c := GetConnection()
	if c == nil {
		return nil, fmt.Errorf("rabbitmq connection unavailable")
	}
	return c.Channel()
}

func SendMessage(
	ch *amqp.Channel,
	queueName string,
	routingKey string,
	exchange string,
	exchangeType ExchangeType,
	props amqp.Publishing,
	message string,
	durable bool,
	exclusive bool,
	autoDelete bool,
	headers amqp.Table,
) error {

	if exchange != "" {
		// ✅ Declare exchange (idempotent — aman kalau dipanggil berulang)
		if err := ch.ExchangeDeclare(
			exchange,
			string(exchangeType),
			durable,
			autoDelete,
			false, // internal
			false, // noWait
			nil,   // args
		); err != nil {
			log.Error("Failed to declare exchange:", err)
			return response.InternalServerError("Failed to declare exchange", nil)
		}
	}

	switch exchangeType {
	case ExchangeFanout, ExchangeHeaders:
		// FANOUT dan HEADERS → broadcast tanpa routing key
		if err := ch.Publish(
			exchange,
			"", // routing key kosong
			false,
			false,
			props,
		); err != nil {
			log.Error("Failed to publish message:", err)
			return response.InternalServerError("Failed to publish message", nil)
		}

	case ExchangeDirect, ExchangeTopic:
		// Declare queue (idempotent juga)
		if _, err := ch.QueueDeclare(
			queueName,
			durable,
			autoDelete,
			exclusive,
			false,   // noWait
			headers, // args
		); err != nil {
			log.Error("Failed to declare queue:", err)
			return response.InternalServerError("Failed to declare queue", nil)
		}

		if exchange != "" {
			// Bind queue ke exchange dengan routing key
			if err := ch.QueueBind(
				queueName,
				routingKey,
				exchange,
				false,
				nil,
			); err != nil {
				log.Error("Failed to bind queue:", err)
				return response.InternalServerError("Failed to bind queue", nil)
			}
		}

		if props.Headers == nil {
			props.Headers = make(amqp.Table)
		}
		tracer := otel.GetTracerProvider().Tracer("rabbitmq-client")
		ctx, span := tracer.Start(context.Background(), fmt.Sprintf("rabbitmq.publish %s", queueName),
			trace.WithSpanKind(trace.SpanKindProducer),
			trace.WithAttributes(
				attribute.String("messaging.system", "rabbitmq"),
				attribute.String("messaging.destination", exchange),
				attribute.String("messaging.rabbitmq.routing_key", routingKey),
			),
		)
		defer span.End()
		otel.GetTextMapPropagator().Inject(ctx, AmqpHeaderCarrier(props.Headers))

		// Publish message
		if err := ch.Publish(
			exchange,
			routingKey,
			false,
			false,
			props,
		); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			log.Error("Failed to publish message:", err)
			return response.InternalServerError("Failed to publish message", nil)
		}
		span.SetStatus(codes.Ok, "")

	default:
		log.Errorf("[!] Unsupported exchange type: %v\n", exchangeType)
		return nil
	}

	log.Infof("[x] Sent '%s' to exchange='%s', queue='%s', routingKey='%s', type='%s'\n",
		message, exchange, queueName, routingKey, exchangeType)

	return nil
}

func ListenQueue(
	ch *amqp.Channel,
	queueName string,
	exchange string,
	routingKey string,
	exchangeType ExchangeType,
	handler func(amqp.Delivery) error,
	durable bool,
	autoDelete bool,
	exclusive bool,
	noWait bool,
	autoAck bool,
	consumerName string,
	noLocal bool,
	bindHeaders amqp.Table, // ← buat QueueBind
	consumeArgs amqp.Table, // ← buat Consume
) error {

	if exchange != "" {
		// 1️⃣ Declare exchange
		if err := ch.ExchangeDeclare(
			exchange,
			string(exchangeType),
			durable,
			autoDelete,
			false,
			noWait,
			nil,
		); err != nil {
			return err
		}
	}

	// 2️⃣ Declare queue
	q, err := ch.QueueDeclare(
		queueName,
		durable,
		autoDelete,
		exclusive,
		noWait,
		nil,
	)
	if err != nil {
		return err
	}

	if exchange != "" {
		// 3️⃣ Bind ke exchange (pakai bindHeaders)
		if err := ch.QueueBind(
			q.Name,
			routingKey,
			exchange,
			noWait,
			bindHeaders,
		); err != nil {
			return err
		}
	}

	// 4️⃣ Consume (pakai consumeArgs)
	msgs, err := ch.Consume(
		q.Name,
		consumerName,
		autoAck,
		exclusive,
		noLocal,
		noWait,
		consumeArgs,
	)
	if err != nil {
		return err
	}

	log.Infof("[*] Listening queue '%s' (exchange='%s', routingKey='%s', consumer=%s)...",
		q.Name, exchange, routingKey, consumerName)

	go func() {
		tracer := otel.GetTracerProvider().Tracer("rabbitmq-client")
		propagator := otel.GetTextMapPropagator()

		for msg := range msgs {
			var carrier AmqpHeaderCarrier
			if msg.Headers != nil {
				carrier = AmqpHeaderCarrier(msg.Headers)
			} else {
				carrier = make(AmqpHeaderCarrier)
			}
			ctx := propagator.Extract(context.Background(), carrier)

			_, span := tracer.Start(ctx, fmt.Sprintf("rabbitmq.consume %s", queueName),
				trace.WithSpanKind(trace.SpanKindConsumer),
				trace.WithAttributes(
					attribute.String("messaging.system", "rabbitmq"),
					attribute.String("messaging.source", queueName),
					attribute.String("messaging.operation", "receive"),
				),
			)

			if err := handler(msg); err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				log.Errorf("[!] Handler error: %v", err)
				if !autoAck {
					_ = msg.Nack(false, true)
				}
			} else {
				span.SetStatus(codes.Ok, "")
				if !autoAck {
					_ = msg.Ack(false)
				}
			}
			span.End()
		}
	}()

	return nil
}

func HealthCheck() error {
	c := GetConnection()
	if c.IsClosed() {
		return amqp.ErrClosed
	}
	return nil
}
