package model

// StartEvent — входящее сообщение из events.analysis.start_cache.
type StartEvent struct {
	TaskID           string `json:"task_id"`
	FileS3Path       string `json:"file_s3_path"`
	ProjectID        string `json:"project_id"`
	CacheProfileHash string `json:"cache_profile_hash"`
	// CacheConfigS3Path — объект конфигурации из MinIO (bucket/key как у file_s3_path).
	// Коллега может скачать через MinIO перед вызовом интерпретатора; см. документацию воркера.
	CacheConfigS3Path string `json:"cache_config_s3_path,omitempty"`
}

// CompletedEvent — исходящее сообщение в events.analysis.cache_completed.
type CompletedEvent struct {
	TaskID         string `json:"task_id"`
	Status         string `json:"status"`
	ArtifactS3Path string `json:"artifact_s3_path,omitempty"`
	Error          string `json:"error,omitempty"`
}
