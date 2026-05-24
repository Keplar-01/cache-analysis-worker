package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	KafkaBrokers []string

	MinioEndpoint  string
	MinioAccessKey string
	MinioSecretKey string

	InterpreterBinary     string
	InterpreterTimeoutSec int
}

func Load() *Config {
	return &Config{
		KafkaBrokers:          strings.Split(getEnv("KAFKA_BROKERS", "localhost:9092"), ","),
		MinioEndpoint:         getEnv("MINIO_ENDPOINT", "localhost:9000"),
		MinioAccessKey:        getEnv("MINIO_ACCESS_KEY", "minioadmin"),
		MinioSecretKey:        getEnv("MINIO_SECRET_KEY", "minioadmin123"),
		InterpreterBinary:     getEnv("INTERPRETER_BINARY", "/usr/local/bin/cats"),
		InterpreterTimeoutSec: getEnvInt("INTERPRETER_TIMEOUT_SEC", 600),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		parsed, err := strconv.Atoi(v)
		if err == nil {
			return parsed
		}
	}
	return fallback
}
