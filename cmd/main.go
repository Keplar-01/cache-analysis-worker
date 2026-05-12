package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/diploma/worker-cache-interpreter/internal/config"
	"github.com/diploma/worker-cache-interpreter/internal/interpreter"
	"github.com/diploma/worker-cache-interpreter/internal/kafka"
	"github.com/diploma/worker-cache-interpreter/internal/storage"
	"github.com/diploma/worker-cache-interpreter/internal/usecase"
)

func main() {
	log.Println("[worker-cache] starting...")

	cfg := config.Load()

	// --- Infrastructure ---

	minioClient, err := storage.NewMinIOClient(cfg.MinioEndpoint, cfg.MinioAccessKey, cfg.MinioSecretKey)
	if err != nil {
		log.Fatalf("[worker-cache] minio connection failed: %v", err)
	}

	producer := kafka.NewProducer(cfg.KafkaBrokers)
	defer producer.Close()

	// --- Business logic ---

	interp := interpreter.New(cfg.InterpreterBinary, cfg.InterpreterTimeoutSec)
	cacheUC := usecase.NewCacheUseCase(interp, minioClient, producer)

	// --- Kafka consumer ---

	consumer := kafka.NewConsumer(cfg.KafkaBrokers, cacheUC)
	defer consumer.Close()

	// --- Graceful shutdown ---

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("[worker-cache] shutting down...")
		cancel()
	}()

	consumer.Listen(ctx)
}
