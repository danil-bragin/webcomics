// Package minio implements the pipeline.AssetStore port over an S3-compatible
// backend (MinIO in dev, R2/S3 in prod — the SDK is identical).
package minio

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/samber/do/v2"

	"github.com/example/dddcqrs/internal/infrastructure/config"
)

// Store is the AssetStore adapter. Holds the client + default bucket so
// callers don't have to thread bucket through every call.
type Store struct {
	client         *minio.Client
	bucket         string
	publicEndpoint string
	publicUseSSL   bool
}

func New(i do.Injector) (*Store, error) {
	cfg := do.MustInvoke[*config.Config](i)
	c, err := minio.New(cfg.MinIOEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinIOAccessKey, cfg.MinIOSecretKey, ""),
		Secure: cfg.MinIOUseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("minio new: %w", err)
	}
	// Ensure default bucket exists. Safe on every boot.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	exists, err := c.BucketExists(ctx, cfg.MinIOBucket)
	if err != nil {
		return nil, fmt.Errorf("minio bucket exists: %w", err)
	}
	if !exists {
		if err := c.MakeBucket(ctx, cfg.MinIOBucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("minio make bucket: %w", err)
		}
	}
	pub := cfg.MinIOPublicEndpoint
	if pub == "" {
		pub = cfg.MinIOEndpoint
	}
	return &Store{
		client:         c,
		bucket:         cfg.MinIOBucket,
		publicEndpoint: pub,
		publicUseSSL:   cfg.MinIOUseSSL,
	}, nil
}

func (s *Store) Bucket() string { return s.bucket }

// Client exposes the underlying MinIO SDK client for transport handlers that
// stream tiny manifest files directly (e.g. the music library proxy).
func (s *Store) Client() *minio.Client { return s.client }

// PresignGet returns a time-limited GET URL with the public endpoint host so
// browsers can fetch the object without exposing private network details.
func (s *Store) PresignGet(ctx context.Context, bucket, key string, ttlSeconds int) (string, error) {
	if bucket == "" {
		bucket = s.bucket
	}
	if ttlSeconds <= 0 {
		ttlSeconds = 300
	}
	u, err := s.client.PresignedGetObject(ctx, bucket, key, time.Duration(ttlSeconds)*time.Second, url.Values{})
	if err != nil {
		return "", err
	}
	return s.rewriteHost(u), nil
}

// PutBytes uploads a buffer as the named object. Used by handlers that accept
// multipart uploads (audio library) or download from a URL and stash to MinIO.
func (s *Store) PutBytes(ctx context.Context, bucket, key string, data []byte, contentType string) error {
	if bucket == "" {
		bucket = s.bucket
	}
	_, err := s.client.PutObject(ctx, bucket, key, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType})
	return err
}

// PutReader streams an io.Reader into MinIO. Used for multipart bodies where
// the full size is not pre-read.
func (s *Store) PutReader(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) error {
	if bucket == "" {
		bucket = s.bucket
	}
	_, err := s.client.PutObject(ctx, bucket, key, r, size,
		minio.PutObjectOptions{ContentType: contentType})
	return err
}

// RemoveObject deletes an object from MinIO. Best-effort; the caller decides
// what to do on failure (orphan track row vs. fail the delete).
func (s *Store) RemoveObject(ctx context.Context, bucket, key string) error {
	if bucket == "" {
		bucket = s.bucket
	}
	return s.client.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{})
}

func (s *Store) PresignPut(ctx context.Context, bucket, key string, ttlSeconds int) (string, error) {
	if bucket == "" {
		bucket = s.bucket
	}
	if ttlSeconds <= 0 {
		ttlSeconds = 300
	}
	u, err := s.client.PresignedPutObject(ctx, bucket, key, time.Duration(ttlSeconds)*time.Second)
	if err != nil {
		return "", err
	}
	return s.rewriteHost(u), nil
}

func (s *Store) rewriteHost(u *url.URL) string {
	scheme := "http"
	if s.publicUseSSL {
		scheme = "https"
	}
	u.Scheme = scheme
	u.Host = s.publicEndpoint
	return u.String()
}

func (s *Store) HealthCheck() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := s.client.BucketExists(ctx, s.bucket)
	return err
}

func (s *Store) Shutdown() error { return nil }
