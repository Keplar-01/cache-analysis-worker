package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const BucketAnalysisArtifacts = "analysis-artifacts"

type MinIOClient struct {
	client *minio.Client
}

func NewMinIOClient(endpoint, accessKey, secretKey string) (*MinIOClient, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: false,
	})
	if err != nil {
		return nil, fmt.Errorf("minio connect: %w", err)
	}

	return &MinIOClient{client: client}, nil
}

// DownloadSource скачивает .c файл из MinIO в локальную директорию.
func (m *MinIOClient) DownloadSource(ctx context.Context, s3Path, workDir string) (string, error) {
	parts := strings.SplitN(s3Path, "/", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid s3 path format: %s", s3Path)
	}

	bucket, key := parts[0], parts[1]

	obj, err := m.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return "", fmt.Errorf("get object %s/%s: %w", bucket, key, err)
	}
	defer obj.Close()

	localPath := filepath.Join(workDir, filepath.Base(key))

	f, err := os.Create(localPath)
	if err != nil {
		return "", fmt.Errorf("create local file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, obj); err != nil {
		return "", fmt.Errorf("copy from minio: %w", err)
	}

	return localPath, nil
}

func (m *MinIOClient) DownloadObject(ctx context.Context, s3Path, workDir string) (string, error) {
	parts := strings.SplitN(s3Path, "/", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid s3 path format: %s", s3Path)
	}

	bucket, key := parts[0], parts[1]

	obj, err := m.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return "", fmt.Errorf("get object %s/%s: %w", bucket, key, err)
	}
	defer obj.Close()

	localPath := filepath.Join(workDir, filepath.Base(key))

	f, err := os.Create(localPath)
	if err != nil {
		return "", fmt.Errorf("create local file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, obj); err != nil {
		return "", fmt.Errorf("copy from minio: %w", err)
	}

	return localPath, nil
}

// UploadArtifact загружает JSON-результат в MinIO.
func (m *MinIOClient) UploadArtifact(ctx context.Context, taskID string, data []byte) (string, error) {
	key := fmt.Sprintf("%s/cache-out.json", taskID)

	_, err := m.client.PutObject(
		ctx,
		BucketAnalysisArtifacts,
		key,
		bytes.NewReader(data),
		int64(len(data)),
		minio.PutObjectOptions{ContentType: "application/json"},
	)
	if err != nil {
		return "", fmt.Errorf("put object: %w", err)
	}

	return BucketAnalysisArtifacts + "/" + key, nil
}
