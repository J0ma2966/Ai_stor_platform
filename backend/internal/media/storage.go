package media

import (
	"bytes"
	"context"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Storage — объектное хранилище. Бакет приватный; наружу выдаются только
// краткоживущие подписанные ссылки.
type Storage interface {
	Put(ctx context.Context, key string, data []byte, contentType string) error
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	Delete(ctx context.Context, key string) error
	Ping(ctx context.Context) error
	// List вызывает fn для объектов с префиксом, изменённых раньше before.
	List(ctx context.Context, prefix string, before time.Time, fn func(key string) error) error
}

type S3 struct {
	c      *minio.Client
	bucket string
}

func NewS3(endpoint, region, bucket, access, secret string, useSSL bool) (*S3, error) {
	c, err := minio.New(endpoint, &minio.Options{
		Creds: credentials.NewStaticV4(access, secret, ""), Secure: useSSL, Region: region,
	})
	if err != nil {
		return nil, err
	}
	return &S3{c: c, bucket: bucket}, nil
}

func (s *S3) Put(ctx context.Context, key string, data []byte, ct string) error {
	_, err := s.c.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: ct, CacheControl: "private, max-age=3600"})
	return err
}

func (s *S3) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	u, err := s.c.PresignedGetObject(ctx, s.bucket, key, ttl, url.Values{})
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	return s.c.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

func (s *S3) Ping(ctx context.Context) error {
	_, err := s.c.BucketExists(ctx, s.bucket)
	return err
}

func (s *S3) List(ctx context.Context, prefix string, before time.Time, fn func(string) error) error {
	for obj := range s.c.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if obj.Err != nil {
			return obj.Err
		}
		if obj.LastModified.Before(before) {
			if err := fn(obj.Key); err != nil {
				return err
			}
		}
	}
	return nil
}

// Memory — хранилище в памяти для тестов.
type Memory struct {
	mu   sync.Mutex
	Objs map[string][]byte
	// Mod — время изменения объектов (тесты могут «состарить» объект).
	Mod map[string]time.Time
}

func NewMemory() *Memory { return &Memory{Objs: map[string][]byte{}, Mod: map[string]time.Time{}} }

func (m *Memory) List(_ context.Context, prefix string, before time.Time, fn func(string) error) error {
	m.mu.Lock()
	var keys []string
	for k := range m.Objs {
		if strings.HasPrefix(k, prefix) && m.Mod[k].Before(before) {
			keys = append(keys, k)
		}
	}
	m.mu.Unlock()
	for _, k := range keys {
		if err := fn(k); err != nil {
			return err
		}
	}
	return nil
}

func (m *Memory) Put(_ context.Context, key string, data []byte, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Objs[key] = data
	m.Mod[key] = time.Now()
	return nil
}
func (m *Memory) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return "memory://" + key, nil
}
func (m *Memory) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Objs, key)
	delete(m.Mod, key)
	return nil
}
func (m *Memory) Ping(context.Context) error { return nil }
