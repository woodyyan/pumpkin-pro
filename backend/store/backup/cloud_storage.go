package backup

import (
	"context"
)

// CloudObjectInfo describes a single object in cloud storage.
type CloudObjectInfo struct {
	Key  string
	Size int64
}

// CloudStorageClient abstracts backup cloud operations for COS and tests.
type CloudStorageClient interface {
	Upload(ctx context.Context, objectKey, localPath, contentType string) error
	List(ctx context.Context, prefix string) ([]CloudObjectInfo, error)
}
