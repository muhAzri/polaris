// Package storage abstracts file storage behind an interface so the
// backing service (MinIO locally, S3/R2/B2 in production) can change
// without touching call sites.
package storage

import (
	"context"
	"io"
	"time"
)

type Storage interface {
	Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
	PresignGet(ctx context.Context, key string, expiry time.Duration) (string, error)
	Delete(ctx context.Context, key string) error
}
