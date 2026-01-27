package storage

import (
	"context"
	"io"
	"net/url"
	"strings"

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
	u, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, err
	}

	secure := u.Scheme == "https"
	host := strings.TrimPrefix(u.Host, "http://")
	if host == "" {
		host = u.Path
	}

	client, err := minio.New(host, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: secure,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, err
	}

	return &S3{client: client, bucket: cfg.Bucket}, nil
}

func (s *S3) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	return err
}
