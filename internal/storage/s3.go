package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3 struct {
	client *minio.Client
	bucket string
}

type Config struct {
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
}

func New(cfg Config) (*S3, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("S3_ENDPOINT empty")
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("S3_BUCKET empty")
	}

	u, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse S3_ENDPOINT: %w", err)
	}

	secure := u.Scheme == "https"

	host := u.Host
	if host == "" {
		// In case someone passes "minio:9000" without scheme
		host = u.Path
	}

	client, err := minio.New(host, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: secure,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("minio.New: %w", err)
	}

	return &S3{client: client, bucket: cfg.Bucket}, nil
}

func (s *S3) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("put object: %w", err)
	}
	return nil
}

func (s *S3) Get(ctx context.Context, key string) ([]byte, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object: %w", err)
	}
	defer obj.Close()

	b, err := io.ReadAll(obj)
	if err != nil {
		return nil, fmt.Errorf("read object: %w", err)
	}
	return b, nil
}
