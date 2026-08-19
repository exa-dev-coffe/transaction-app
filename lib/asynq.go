package lib

import (
	"eka-dev.cloud/transaction-service/config"
	"github.com/gofiber/fiber/v2/log"
	"github.com/hibiken/asynq"
)

var AsynqClient *asynq.Client

func InitAsynq() {
	redisOpt := asynq.RedisClientOpt{
		Addr:     config.Config.RedisUrl,
		Password: config.Config.RedisPassword,
	}
	AsynqClient = asynq.NewClient(redisOpt)
	log.Info("Asynq Client initialized successfully")
}
