package repository

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/diploma/worker-cache-interpreter/internal/model"
	"github.com/google/uuid"
)

type ClickHouseRepo struct {
	conn clickhouse.Conn
}

func NewClickHouseRepo(addr, user, password, db string) (*ClickHouseRepo, error) {
	conn, err := connectWithRetry(addr, user, password, db)
	if err != nil {
		return nil, err
	}

	return &ClickHouseRepo{conn: conn}, nil
}

func (r *ClickHouseRepo) Close() error {
	return r.conn.Close()
}

// WriteCacheSimResult записывает полный результат CacheSim в ClickHouse:
// - dynamic_metrics: per-array промахи для JOIN с static_patterns
// - dynamic_summary: глобальные метрики по уровням кэша
func (r *ClickHouseRepo) WriteCacheSimResult(ctx context.Context, taskID string, result *model.CacheSimResult) error {
	taskUUID, err := uuid.Parse(taskID)
	if err != nil {
		return fmt.Errorf("parse task UUID: %w", err)
	}

	// 1. Per-array метрики → dynamic_metrics
	if len(result.Arrays) > 0 {
		batch, err := r.conn.PrepareBatch(ctx, `
			INSERT INTO dynamic_metrics (
				task_id, source_file, cache_level, array_name,
				misses_total, misses_read, misses_write
			)`)
		if err != nil {
			return fmt.Errorf("prepare batch dynamic_metrics: %w", err)
		}

		for _, a := range result.Arrays {
			if err := batch.Append(
				taskUUID,
				result.SourceFile,
				a.CacheLevel,
				a.ArrayName,
				a.MissesTotal,
				a.MissesRead,
				a.MissesWrite,
			); err != nil {
				return fmt.Errorf("append array metric: %w", err)
			}
		}

		if err := batch.Send(); err != nil {
			return fmt.Errorf("send dynamic_metrics batch: %w", err)
		}
	}

	// 2. Summary по уровням кэша → dynamic_summary
	for _, level := range result.CacheLevels() {
		if level.CacheLevel == "" {
			continue
		}
		if err := r.conn.Exec(ctx, `
			INSERT INTO dynamic_summary (
				task_id, source_file, cache_level,
				cache_size_kb, cache_line_size, associativity,
				total_accesses, total_hits, total_misses,
				hits_read, hits_write, misses_read, misses_write,
				miss_rate, memory_reads, memory_writes, sim_time_sec
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			taskUUID,
			result.SourceFile,
			level.CacheLevel,
			level.CacheSizeKB,
			level.CacheLineSize,
			level.Associativity,
			level.TotalAccesses,
			level.TotalHits,
			level.TotalMisses,
			level.HitsRead,
			level.HitsWrite,
			level.MissesRead,
			level.MissesWrite,
			level.MissRate,
			result.MemoryReads,
			result.MemoryWrites,
			result.SimTimeSec,
		); err != nil {
			return fmt.Errorf("insert dynamic_summary %s: %w", level.CacheLevel, err)
		}
	}

	return nil
}

func connectWithRetry(addr, user, password, db string) (clickhouse.Conn, error) {
	var conn clickhouse.Conn
	var lastErr error

	for i := range 30 {
		conn, lastErr = clickhouse.Open(&clickhouse.Options{
			Addr: []string{addr},
			Auth: clickhouse.Auth{
				Database: db,
				Username: user,
				Password: password,
			},
		})
		if lastErr == nil {
			if pingErr := conn.Ping(context.Background()); pingErr == nil {
				return conn, nil
			}
		}
		log.Printf("[clickhouse] waiting for connection... attempt %d/30", i+1)
		time.Sleep(2 * time.Second)
	}

	return nil, fmt.Errorf("clickhouse connection failed after 30 attempts: %w", lastErr)
}
