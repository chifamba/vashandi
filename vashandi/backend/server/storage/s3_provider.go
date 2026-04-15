package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithy "github.com/aws/smithy-go"

	"github.com/chifamba/vashandi/vashandi/backend/shared"
)

type s3Client interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

type S3Provider struct {
	bucket string
	prefix string
	client s3Client
}

func normalizePrefix(prefix string) string {
	return strings.Trim(strings.TrimSpace(prefix), "/")
}

func buildS3Key(prefix, objectKey string) string {
	if prefix == "" {
		return objectKey
	}
	return prefix + "/" + objectKey
}

func NewS3Provider(ctx context.Context, cfg shared.StorageS3Config) (*S3Provider, error) {
	return newS3Provider(ctx, cfg, nil)
}

func newS3Provider(ctx context.Context, cfg shared.StorageS3Config, client s3Client) (*S3Provider, error) {
	bucket := strings.TrimSpace(cfg.Bucket)
	region := strings.TrimSpace(cfg.Region)
	if bucket == "" {
		return nil, newStatusError(422, "s3 storage bucket is required")
	}
	if region == "" {
		return nil, newStatusError(422, "s3 storage region is required")
	}
	if client == nil {
		awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
		if err != nil {
			return nil, err
		}
		client = s3.NewFromConfig(awsCfg, func(options *s3.Options) {
			options.UsePathStyle = cfg.ForcePathStyle
			if endpoint := strings.TrimSpace(cfg.Endpoint); endpoint != "" {
				options.BaseEndpoint = &endpoint
			}
		})
	}
	return &S3Provider{
		bucket: bucket,
		prefix: normalizePrefix(cfg.Prefix),
		client: client,
	}, nil
}

func (p *S3Provider) ProviderID() string {
	return "s3"
}

func (p *S3Provider) PutObject(ctx context.Context, input PutObjectInput) error {
	_, err := p.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        &p.bucket,
		Key:           stringPtr(buildS3Key(p.prefix, input.ObjectKey)),
		Body:          bytes.NewReader(input.Body),
		ContentType:   stringPtr(input.ContentType),
		ContentLength: &input.ContentLength,
	})
	return err
}

func (p *S3Provider) GetObject(ctx context.Context, input GetObjectInput) (*GetObjectResult, error) {
	output, err := p.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &p.bucket,
		Key:    stringPtr(buildS3Key(p.prefix, input.ObjectKey)),
	})
	if err != nil {
		if isS3NotFound(err) {
			return nil, newStatusError(404, "object not found")
		}
		return nil, err
	}
	var lastModified *time.Time
	if output.LastModified != nil {
		lastModified = output.LastModified
	}
	return &GetObjectResult{
		Stream:        output.Body,
		ContentType:   valueOrEmpty(output.ContentType),
		ContentLength: valueOrZero(output.ContentLength),
		ETag:          valueOrEmpty(output.ETag),
		LastModified:  lastModified,
	}, nil
}

func (p *S3Provider) HeadObject(ctx context.Context, input HeadObjectInput) (*HeadObjectResult, error) {
	output, err := p.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &p.bucket,
		Key:    stringPtr(buildS3Key(p.prefix, input.ObjectKey)),
	})
	if err != nil {
		if isS3NotFound(err) {
			return &HeadObjectResult{Exists: false}, nil
		}
		return nil, err
	}
	var lastModified *time.Time
	if output.LastModified != nil {
		lastModified = output.LastModified
	}
	return &HeadObjectResult{
		Exists:        true,
		ContentType:   valueOrEmpty(output.ContentType),
		ContentLength: valueOrZero(output.ContentLength),
		ETag:          valueOrEmpty(output.ETag),
		LastModified:  lastModified,
	}, nil
}

func (p *S3Provider) DeleteObject(ctx context.Context, input DeleteObjectInput) error {
	_, err := p.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &p.bucket,
		Key:    stringPtr(buildS3Key(p.prefix, input.ObjectKey)),
	})
	return err
}

func isS3NotFound(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound", "NotFoundException":
			return true
		}
	}
	var noSuchKey *types.NoSuchKey
	return errors.As(err, &noSuchKey)
}

func stringPtr(value string) *string {
	return &value
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func valueOrZero(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func readAllCloser(reader io.ReadCloser) ([]byte, error) {
	defer reader.Close()
	return io.ReadAll(reader)
}
