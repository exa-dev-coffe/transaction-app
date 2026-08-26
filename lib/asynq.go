package lib

import (
	"log/slog"

	"eka-dev.cloud/transaction-service/config"
	"github.com/hibiken/asynq"
)

var AsynqClient *asynq.Client

func InitAsynq() {
	redisOpt := asynq.RedisClientOpt{
		Addr:     config.Config.RedisUrl,
		Username: config.Config.RedisUsername,
		Password: config.Config.RedisPassword,
	}
	AsynqClient = asynq.NewClient(redisOpt)
	slog.Info("Asynq Client initialized successfully")
}
