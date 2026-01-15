package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"main_service/internal/config"
	addProduct "main_service/internal/http-server/handlers/products/add"
	deleteProduct "main_service/internal/http-server/handlers/products/delete"
	getProducts "main_service/internal/http-server/handlers/products/get"
	getByID "main_service/internal/http-server/handlers/products/get_by_id"
	"main_service/internal/lib/jwt"
	"main_service/internal/lib/parser"
	"main_service/internal/lib/validators"
	authMiddlware "main_service/internal/middleware/auth"
	"main_service/internal/middleware/products"
	"main_service/internal/rabbitmq"
	"main_service/internal/storage/postgres"
	"main_service/internal/storage/redis"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-playground/validator/v10"
)

const (
	envLocal = "local"
	envDev   = "dev"
	envProd  = "prod"
)

func main() {
	cfg := config.MustLoad("./config/config.yaml")

	log := setupLogger(cfg.Env)

	log.Info("starting main service", slog.String("env", cfg.Env))

	// * Context для иницализации компонентов
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jwtParser := jwt.New(cfg.JWTSecret)

	redisClient, err := redis.New(ctx, cfg.Redis.Addr, cfg.Redis.Db, cfg.Redis.DefaultTTL)
	if err != nil {
		log.Error("failed to connect redis", slog.String("err", err.Error()))
		os.Exit(1)
	}
	defer redisClient.Close()

	log.Info("redis connected successfully",
		slog.String("addr", cfg.Redis.Addr),
		slog.Int("db", cfg.Redis.Db),
		slog.Duration("default_ttl", cfg.Redis.DefaultTTL),
	)

	postgresClient, err := postgres.New(ctx, cfg)
	if err != nil {
		log.Error("failed to connect posgtreSQL", slog.String("err", err.Error()))
		os.Exit(1)
	}
	defer postgresClient.Close()

	log.Info("postgresql connected successfully",
		slog.String("host", cfg.Postgres.Host),
		slog.Int("port", cfg.Postgres.Port),
		slog.String("database", cfg.Postgres.DBName),
	)

	rabbitMQClient, err := rabbitmq.New(cfg.RabbitMQ.URL)
	if err != nil {
		log.Error("failed to connect rabbitMQ", slog.String("err", err.Error()))
		os.Exit(1)
	}
	defer rabbitMQClient.Close()

	log.Info("rabbitmq connected successfully",
		slog.Int("workers", cfg.RabbitMQ.WorkerPoolSize),
	)

	rabbitMQProducer, err := rabbitmq.NewProducer(
		rabbitMQClient.Channel,
		cfg.RabbitMQ.ProducerQueueName,
	)
	if err != nil {
		log.Error("failed to init producer", slog.String("err", err.Error()))
		os.Exit(1)
	}

	rabbitMQConsumer, err := rabbitmq.NewConsumer(
		rabbitMQClient.Channel,
		log,
		cfg.RabbitMQ.ConsumerQueueName,
		cfg.RabbitMQ.WorkerPoolSize,
	)
	if err != nil {
		log.Error("failed to init consumer", slog.String("err", err.Error()))
		os.Exit(1)
	}

	prodOP := products.New(
		log,
		postgresClient,
		redisClient,
		rabbitMQProducer,
		cfg.CheckInterval,
		cfg.ParsingBatchSize,
	)

	log.Info("starting periodic product parsing")
	go func() {
		if err := prodOP.RunPeriodicParsing(ctx); err != nil && err != context.Canceled {
			log.Error("periodic parsing stopped with error",
				slog.String("error", err.Error()),
			)
		}
	}()

	parserClient := parser.New(postgresClient, rabbitMQConsumer)

	log.Info("starting message parser")
	if err := parserClient.Run(ctx); err != nil {
		log.Error("failed to start parser", slog.String("error", err.Error()))
		os.Exit(1)
	}
	log.Info("parser started successfully")

	requestValidator := validator.New()
	if err := validators.RegisterProductURLValidator(requestValidator); err != nil {
		log.Error("Failed to register product URL validator:", slog.String("error", err.Error()))
		os.Exit(1)
	}

	router := setupRouter(
		log,
		requestValidator,
		postgresClient,
		prodOP,
		jwtParser,
	)

	srv := &http.Server{
		Addr:           cfg.HTTPServer.Address,
		Handler:        router,
		ReadTimeout:    cfg.HTTPServer.Timeout,
		WriteTimeout:   cfg.HTTPServer.Timeout,
		IdleTimeout:    cfg.HTTPServer.IdleTimeout,
		MaxHeaderBytes: 1 << 20, // * 1 MB
	}

	serverErrors := make(chan error, 1)
	go func() {
		log.Info("starting http server", slog.String("address", cfg.HTTPServer.Address))
		serverErrors <- srv.ListenAndServe()
	}()

	// * graceful shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		log.Error("server error", slog.String("error", err.Error()))
		os.Exit(1)

	case sig := <-shutdown:
		log.Info("shutdown signal received", slog.String("signal", sig.String()))

		cancel()

		// * Graceful shutdown context
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		log.Info("shutting down http server")

		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error("failed to shutdown server gracefully", slog.String("error", err.Error()))

			if closeErr := srv.Close(); closeErr != nil {
				log.Error("failed to force close server", slog.String("error", closeErr.Error()))
			}
		}

		log.Info("server stopped gracefully")
	}
}

func setupRouter(
	log *slog.Logger,
	validate *validator.Validate,
	postgres *postgres.PostgresRepo,
	prodOP *products.ProductOperator,
	jwtParser *jwt.JWTParser,
) *chi.Mux {
	r := chi.NewRouter()

	r.Use(authMiddlware.New(log, jwtParser))
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(middleware.Compress(5))

	r.Post("/product", addProduct.New(log, prodOP, validate))
	r.Get("/products", getProducts.New(log, postgres))
	r.Get("/product", getByID.New(log, prodOP))
	r.Delete("/product", deleteProduct.New(log, postgres))

	return r
}

func setupLogger(env string) *slog.Logger {
	var log *slog.Logger

	switch env {
	case envLocal:
		log = slog.New(
			slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}),
		)
	case envDev:
		log = slog.New(
			slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}),
		)
	case envProd:
		log = slog.New(
			slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}),
		)
	}

	return log
}
