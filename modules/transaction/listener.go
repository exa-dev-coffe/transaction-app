package transaction

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"eka-dev.cloud/transaction-service/lib"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/jmoiron/sqlx"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Listener interface {
}

type transactionListener struct {
	service Service
	db      *sqlx.DB
	ch      *amqp.Channel
}

func NewListener(ch *amqp.Channel, service Service, db *sqlx.DB) Listener {
	l := &transactionListener{service: service, db: db, ch: ch}

	if err := l.ListenPosQrisSettlement(); err != nil {
		slog.Error("Failed to start listening to pos.payment.settled queue", "error", err)
	}

	return l
}

func (l *transactionListener) ListenPosQrisSettlement() error {
	slog.Info("Starting to listen to pos.payment.settled queue")
	return lib.ListenQueue(
		l.ch,
		"pos.payment.settled",
		"pos.payment.settled",
		"",
		lib.ExchangeDirect,
		func(delivery amqp.Delivery) error {
			slog.Info("Received pos.payment.settled message", "body", string(delivery.Body))
			var req PosQrisSettledEvent
			if err := json.Unmarshal(delivery.Body, &req); err != nil {
				slog.Error("Failed to parse POS payment settled message body", "error", err)
				return response.InternalServerError("Failed to parse message body", nil)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			_, err := l.service.SettlePosQrisPayment(ctx, req.OrderRef, req.PaymentStatus)
			if err != nil {
				slog.Error("Failed to settle POS QRIS payment", "error", err)
				return err
			}

			return nil
		},
		true,
		false, false, false, false, "Transaction Service POS QRIS Settlement", true, amqp.Table{},
		amqp.Table{},
	)
}
