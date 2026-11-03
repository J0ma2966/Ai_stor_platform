package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
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
	// Get читает объект целиком (для воркера AI-примерки; объекты ограничены MaxUploadMB).
	Get(ctx context.Context, key string) ([]byte, error)
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

// maxGet — защита памяти воркера: больше этого объект не читаем.
const maxGet = 64 << 20

var ErrObjectNotFound = errors.New("object not found")

func (s *S3) Get(ctx context.Context, key string) ([]byte, error) {
	obj, err := s.c.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close()
	data, err := io.ReadAll(io.LimitReader(obj, maxGet+1))
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, ErrObjectNotFound
		}
		return nil, err
	}
	if len(data) > maxGet {
		return nil, fmt.Errorf("object %s exceeds %d bytes", key, maxGet)
	}
	return data, nil
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
func (m *Memory) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.Objs[key]
	if !ok {
		return nil, ErrObjectNotFound
	}
	return bytes.Clone(d), nil
}

// Has — есть ли объект (для тестов).
func (m *Memory) Has(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.Objs[key]
	return ok
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

// Routed направляет биометрические объекты (фото клиента, результаты примерки) в
// отдельное хранилище Biometric — физически в Узбекистане (закон РУз «О персональных
// данных», ст. 27-1: локализация ПД граждан РУз). Остальное — в Default.
// Маршрут определяется только по ключу, поэтому весь код работает с одним Storage.
type Routed struct {
	Default   Storage
	Biometric Storage
}

func (r Routed) pick(key string) Storage {
	if IsBiometricKey(key) {
		return r.Biometric
	}
	return r.Default
}

func (r Routed) Put(ctx context.Context, key string, data []byte, ct string) error {
	return r.pick(key).Put(ctx, key, data, ct)
}
func (r Routed) Get(ctx context.Context, key string) ([]byte, error) {
	return r.pick(key).Get(ctx, key)
}
func (r Routed) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return r.pick(key).PresignGet(ctx, key, ttl)
}
func (r Routed) Delete(ctx context.Context, key string) error { return r.pick(key).Delete(ctx, key) }
func (r Routed) Ping(ctx context.Context) error {
	return errors.Join(r.Default.Ping(ctx), r.Biometric.Ping(ctx))
}

// List обходит оба хранилища: в каждом — только «свои» ключи (если это один и тот же
// бакет, объект не будет выдан дважды).
func (r Routed) List(ctx context.Context, prefix string, before time.Time, fn func(string) error) error {
	if err := r.Default.List(ctx, prefix, before, func(k string) error {
		if IsBiometricKey(k) {
			return nil
		}
		return fn(k)
	}); err != nil {
		return err
	}
	return r.Biometric.List(ctx, prefix, before, func(k string) error {
		if !IsBiometricKey(k) {
			return nil
		}
		return fn(k)
	})
}
