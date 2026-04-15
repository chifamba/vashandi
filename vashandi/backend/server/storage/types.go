package storage

import (
	"context"
	"io"
	"time"
)

type PutObjectInput struct {
	ObjectKey     string
	Body          []byte
	ContentType   string
	ContentLength int64
}

type GetObjectInput struct {
	ObjectKey string
}

type HeadObjectInput struct {
	ObjectKey string
}

type DeleteObjectInput struct {
	ObjectKey string
}

type GetObjectResult struct {
	Stream        io.ReadCloser
	ContentType   string
	ContentLength int64
	ETag          string
	LastModified  *time.Time
}

type HeadObjectResult struct {
	Exists        bool
	ContentType   string
	ContentLength int64
	ETag          string
	LastModified  *time.Time
}

type Provider interface {
	ProviderID() string
	PutObject(ctx context.Context, input PutObjectInput) error
	GetObject(ctx context.Context, input GetObjectInput) (*GetObjectResult, error)
	HeadObject(ctx context.Context, input HeadObjectInput) (*HeadObjectResult, error)
	DeleteObject(ctx context.Context, input DeleteObjectInput) error
}

type PutFileInput struct {
	CompanyID        string
	Namespace        string
	OriginalFilename string
	ContentType      string
	Body             []byte
}

type PutFileResult struct {
	Provider         string
	ObjectKey        string
	ContentType      string
	ByteSize         int64
	SHA256           string
	OriginalFilename string
}

type Service struct {
	provider Provider
}

func NewService(provider Provider) *Service {
	return &Service{provider: provider}
}

func (s *Service) ProviderID() string {
	return s.provider.ProviderID()
}
