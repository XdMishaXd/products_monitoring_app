package config

import (
	"log"
	"os"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Env              string        `yaml:"env" env-default:"local"`
	JWTSecret        string        `yaml:"jwt_secret" env-required:"true"`
	CheckInterval    time.Duration `yaml:"check_interval" env-default:"30m"`
	ParsingBatchSize int           `yaml:"parsing_batch_size" env-default:"10"`
	RabbitMQ         `yaml:"rabbitmq"`
	Postgres         `yaml:"postgres"`
	HTTPServer       `yaml:"http_server"`
	Redis            `yaml:"redis"`
}

type HTTPServer struct {
	Address     string        `yaml:"address" env-default:"localhost:8080"`
	Timeout     time.Duration `yaml:"timeout" env-default:"4s"`
	IdleTimeout time.Duration `yaml:"idle_timeout" env-default:"60s"`
}

type Postgres struct {
	Host     string `yaml:"host" env-default:"postgres"`
	Port     int    `yaml:"port" env-default:"5432"`
	User     string `yaml:"user" env-required:"true"`
	Password string `yaml:"password" env-required:"true"`
	DBName   string `yaml:"dbname" env-required:"true"`
	SSLMode  string `yaml:"sslmode" env-default:"disabled"`
}

type RabbitMQ struct {
	URL               string `yaml:"url" env-required:"true"`
	ProducerQueueName string `yaml:"producer_queue_name" env-required:"true"`
	ConsumerQueueName string `yaml:"consumer_queue_name" env-required:"true"`
	WorkerPoolSize    int    `yaml:"worker_pool_size" env-default:"10"`
}

type Redis struct {
	Addr       string        `yaml:"addr" env-default:"redis:6379"`
	Db         int           `yaml:"db" env-default:"1"`
	DefaultTTL time.Duration `yaml:"default_ttl" env-default:"1m"`
}

func MustLoad(configPath string) *Config {
	// проверка существования файла
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Fatalf("config file does not exist: %s", configPath)
	}

	var cfg Config

	if err := cleanenv.ReadConfig(configPath, &cfg); err != nil {
		log.Fatalf("cannot read config: %s", configPath)
	}

	return &cfg
}
