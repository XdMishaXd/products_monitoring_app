package config

import (
	"log"
	"os"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Env          string `yaml:"env" env-default:"local"`
	ParseRetries int    `yaml:"parse_retries" env-default:"3"`
	RabbitMQ     `yaml:"rabbitmq"`
}

type RabbitMQ struct {
	URL               string `yaml:"url" env-required:"true"`
	ProducerQueueName string `yaml:"producer_queue_name" env-required:"true"`
	ConsumerQueueName string `yaml:"consumer_queue_name" env-required:"true"`
	WorkerPoolSize    int    `yaml:"worker_pool_size" env-default:"10"`
}

func MustLoad() *Config {
	configPath := "./config/config.yaml"

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Fatalf("config file does not exist: %s", configPath)
	}

	var cfg Config

	if err := cleanenv.ReadConfig(configPath, &cfg); err != nil {
		log.Fatalf("cannot read config: %s", configPath)
	}

	return &cfg
}
