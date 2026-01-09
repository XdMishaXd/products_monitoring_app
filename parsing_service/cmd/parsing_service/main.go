package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"parsing_service/internal/config"
	msgHandler "parsing_service/internal/middleware/message_handler"
	ebayParserDebug "parsing_service/internal/parsers/debug"
	"parsing_service/internal/rabbitmq"
)

const (
	envLocal = "local"
	envDev   = "dev"
	envProd  = "prod"
)

func main() {
	cfg := config.MustLoad()

	log := setupLogger(cfg.Env)

	log.Info("starting main service", slog.String("env", cfg.Env))

	rabbitMQClient, err := rabbitmq.New(cfg.RabbitMQ.URL)
	if err != nil {
		log.Error("failed to connect rabbitMQ", slog.String("err", err.Error()))
		os.Exit(1)
	}
	defer rabbitMQClient.Close()

	log.Info("rabbitmq connected successfully",
		slog.Int("workers", cfg.RabbitMQ.WorkerPoolSize),
	)

	rabbitMQProducer := rabbitmq.NewProducer(
		rabbitMQClient.Channel,
		cfg.RabbitMQ.ProducerQueueName,
	)
	rabbitMQConsumer := rabbitmq.NewConsumer(
		rabbitMQClient.Channel,
		log,
		cfg.RabbitMQ.ConsumerQueueName,
		cfg.RabbitMQ.WorkerPoolSize,
	)

	ebayParser := ebayParserDebug.NewEbayParser()

	messageHandler := msgHandler.New(log, rabbitMQProducer, ebayParser, cfg.ParseRetries)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Info("starting consumer goroutine")

		if err := rabbitMQConsumer.Consume(ctx, messageHandler.Handle); err != nil {
			log.Error("consumer error", slog.String("err", err.Error()))
			cancel()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Info("starting producer goroutine")

		<-ctx.Done()
		log.Info("producer goroutine stopped")
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Info("starting parser goroutine")

		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Info("parser goroutine stopped")
				return
			case <-ticker.C:
				log.Debug("parser tick")
			}
		}
	}()

	log.Info("parsing service started successfully")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	<-quit
	log.Info("shutting down parsing service...")

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Info("all goroutines stopped gracefully")
	case <-shutdownCtx.Done():
		log.Warn("shutdown timeout exceeded, forcing exit")
	}

	log.Info("parsing service stopped")
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
