package media

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/minio/minio-go/v7/pkg/policy"

	"egeism/internal/config"
)

// InitBuckets provisions storage for a deployment without a separate mc image.
// New creates both buckets and removes any anonymous policy from student
// solutions before the task bucket is made publicly downloadable.
func InitBuckets(ctx context.Context, cfg config.Config) error {
	s, err := New(ctx, cfg)
	if err != nil {
		return fmt.Errorf("initialize buckets: %w", err)
	}
	readOnly := policy.BucketAccessPolicy{
		Version:    "2012-10-17",
		Statements: policy.SetPolicy(nil, policy.BucketPolicyReadOnly, s.bucket, ""),
	}
	data, err := json.Marshal(readOnly)
	if err != nil {
		return fmt.Errorf("encode task download policy: %w", err)
	}
	if err := s.client.SetBucketPolicy(ctx, s.bucket, string(data)); err != nil {
		return fmt.Errorf("set task download policy: %w", err)
	}
	return nil
}
