package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/diploma/worker-cache-interpreter/internal/interpreter"
	"github.com/diploma/worker-cache-interpreter/internal/kafka"
	"github.com/diploma/worker-cache-interpreter/internal/model"
	"github.com/diploma/worker-cache-interpreter/internal/storage"
)

// CacheUseCase — бизнес-логика пайплайна кэш-интерпретации.
type CacheUseCase struct {
	interp   *interpreter.Interpreter
	minio    *storage.MinIOClient
	producer *kafka.Producer
}

func NewCacheUseCase(
	interp *interpreter.Interpreter,
	m *storage.MinIOClient,
	p *kafka.Producer,
) *CacheUseCase {
	return &CacheUseCase{
		interp:   interp,
		minio:    m,
		producer: p,
	}
}

// HandleStartEvent реализует kafka.MessageHandler.
func (uc *CacheUseCase) HandleStartEvent(ctx context.Context, event model.StartEvent) error {
	log.Printf("[usecase] processing cache task %s, file: %s", event.TaskID, event.FileS3Path)

	if err := uc.process(ctx, event); err != nil {
		log.Printf("[usecase] task %s failed: %v", event.TaskID, err)
		uc.sendCompleted(ctx, event.TaskID, "error", "", err.Error())
		return err
	}

	log.Printf("[usecase] task %s completed successfully", event.TaskID)
	return nil
}

func (uc *CacheUseCase) process(ctx context.Context, event model.StartEvent) error {
	workDir, err := os.MkdirTemp("", "cache-interp-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	// 1. Скачиваем исходный .c файл из MinIO
	sourceFile, err := uc.minio.DownloadSource(ctx, event.FileS3Path, workDir)
	if err != nil {
		return fmt.Errorf("download source: %w", err)
	}

	log.Printf("[usecase] downloaded .c file to %s", sourceFile)

	// 2. Запускаем cats над локальным .c файлом.
	result, err := uc.interp.Run(ctx, sourceFile)
	if err != nil {
		return fmt.Errorf("cachesim run: %w", err)
	}

	log.Printf("[usecase] CacheSim done: L1 access=%d miss=%d, L2 access=%d miss=%d, arrays=%d",
		result.L1.TotalAccesses, result.L1.TotalMisses,
		result.L2.TotalAccesses, result.L2.TotalMisses,
		len(result.Arrays))

	// 3. Загружаем JSON-артефакт в MinIO
	outputJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cache result: %w", err)
	}

	artifactPath, err := uc.minio.UploadArtifact(ctx, event.TaskID, outputJSON)
	if err != nil {
		return fmt.Errorf("upload artifact: %w", err)
	}

	// 4. Отправляем событие завершения.
	uc.sendCompleted(ctx, event.TaskID, "success", artifactPath, "")
	return nil
}

func (uc *CacheUseCase) sendCompleted(ctx context.Context, taskID, status, artifactPath, errMsg string) {
	event := model.CompletedEvent{
		TaskID:         taskID,
		Status:         status,
		ArtifactS3Path: artifactPath,
		Error:          errMsg,
	}

	if err := uc.producer.Publish(ctx, kafka.TopicCacheCompleted, taskID, event); err != nil {
		log.Printf("[usecase] failed to send completed event: %v", err)
	}
}
