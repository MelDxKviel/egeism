// Package media stores task images and attached files in MinIO (S3) and serves
// them back. Ingest downloads media from the source and puts it here; tasks
// reference objects by key (domain.Media.Key). Keys are content-addressed
// (sha1 of the bytes), so identical media dedups automatically.
package media

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"egeism/internal/config"
)

// Store wraps a MinIO client bound to one bucket.
type Store struct {
	client *minio.Client
	bucket string
	http   *http.Client
}

// New connects to MinIO and ensures the bucket exists.
func New(ctx context.Context, cfg config.Config) (*Store, error) {
	client, err := minio.New(cfg.MinIOEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinIOAccessKey, cfg.MinIOSecretKey, ""),
		Secure: cfg.MinIOUseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}
	s := &Store{client: client, bucket: cfg.MinIOBucket, http: &http.Client{Timeout: 30 * time.Second}}
	if err := s.ensureBucket(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) ensureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("bucket exists: %w", err)
	}
	if !exists {
		if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err != nil {
			return fmt.Errorf("make bucket: %w", err)
		}
	}
	return nil
}

// Object is a stored media object being read back.
type Object struct {
	Body        io.ReadCloser
	ContentType string
	Size        int64
}

// Put stores bytes under a content-addressed key and returns the key. keyHint
// supplies the file extension (from the source URL/filename).
func (s *Store) Put(ctx context.Context, data []byte, contentType, keyHint string) (string, error) {
	sum := sha1.Sum(data)
	key := hex.EncodeToString(sum[:])
	if ext := path.Ext(keyHint); ext != "" && len(ext) <= 6 {
		key += ext
	}
	_, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return "", fmt.Errorf("put object %s: %w", key, err)
	}
	return key, nil
}

// PutFromURL downloads a remote URL and stores it, returning the object key.
func (s *Store) PutFromURL(ctx context.Context, srcURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srcURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", srcURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("download %s: status %d", srcURL, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20)) // 32 MiB cap
	if err != nil {
		return "", err
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}
	return s.Put(ctx, data, ct, srcURL)
}

// Get streams an object back for serving.
func (s *Store) Get(ctx context.Context, key string) (*Object, error) {
	key = strings.TrimPrefix(key, "/")
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	info, err := obj.Stat()
	if err != nil {
		obj.Close()
		return nil, err
	}
	return &Object{Body: obj, ContentType: info.ContentType, Size: info.Size}, nil
}
