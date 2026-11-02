// Package config читает конфигурацию только из переменных окружения (12-factor).
// Секреты никогда не хранятся в репозитории.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env          string // dev | test | prod
	HTTPAddr     string
	InternalAddr string // /metrics и pprof, не публикуется наружу
	DatabaseURL  string
	DBMaxConns   int32
	RedisURL     string

	JWTSecret    []byte
	AccessTTL    time.Duration
	RefreshTTL   time.Duration
	CookieDomain string
	CookieSecure bool
	CORSOrigins  []string
	TrustedProxy bool // доверять X-Forwarded-For (только за балансировщиком)

	OTPTTL      time.Duration
	OTPCooldown time.Duration
	OTPMaxTries int
	OTPPepper   []byte
	// Префиксы номеров, на которые разрешено слать SMS (защита от SMS pumping).
	OTPAllowedPrefixes []string

	S3Endpoint  string
	S3Region    string
	S3Bucket    string
	S3AccessKey string
	S3SecretKey string
	S3UseSSL    bool
	MaxUploadMB int
	// Базовый URL публичных картинок (CDN или публичный префикс бакета), напр. https://cdn.example.uz
	PublicMediaURL    string
	UploadConcurrency int // одновременных декодирований изображений на инстанс

	AutoMigrate bool
}

func (c Config) IsProd() bool { return c.Env == "prod" }

func Load() (Config, error) {
	c := Config{
		Env:                get("APP_ENV", "dev"),
		HTTPAddr:           get("HTTP_ADDR", ":8080"),
		InternalAddr:       get("INTERNAL_ADDR", "127.0.0.1:9090"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		DBMaxConns:         int32(min(max(getInt("DB_MAX_CONNS", 20), 1), 1000)), // #nosec G115 -- clamped
		RedisURL:           get("REDIS_URL", "redis://localhost:6379/0"),
		JWTSecret:          []byte(os.Getenv("JWT_SECRET")),
		AccessTTL:          getDur("ACCESS_TTL", 15*time.Minute),
		RefreshTTL:         getDur("REFRESH_TTL", 30*24*time.Hour),
		CookieDomain:       os.Getenv("COOKIE_DOMAIN"),
		CookieSecure:       getBool("COOKIE_SECURE", true),
		CORSOrigins:        splitList(os.Getenv("CORS_ORIGINS")),
		TrustedProxy:       getBool("TRUSTED_PROXY", false),
		OTPTTL:             getDur("OTP_TTL", 5*time.Minute),
		OTPCooldown:        getDur("OTP_COOLDOWN", 60*time.Second),
		OTPMaxTries:        getInt("OTP_MAX_TRIES", 5),
		OTPPepper:          []byte(os.Getenv("OTP_PEPPER")),
		OTPAllowedPrefixes: splitList(get("OTP_ALLOWED_PREFIXES", "+998")),
		S3Endpoint:         get("S3_ENDPOINT", "localhost:9000"),
		S3Region:           get("S3_REGION", "us-east-1"),
		S3Bucket:           get("S3_BUCKET", "boutique-media"),
		S3AccessKey:        os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:        os.Getenv("S3_SECRET_KEY"),
		S3UseSSL:           getBool("S3_USE_SSL", true),
		MaxUploadMB:        getInt("MAX_UPLOAD_MB", 10),
		PublicMediaURL:     get("PUBLIC_MEDIA_URL", "http://localhost:9000/boutique-media"),
		UploadConcurrency:  min(max(getInt("UPLOAD_CONCURRENCY", 4), 1), 64),
		AutoMigrate:        getBool("AUTO_MIGRATE", false),
	}
	var errs []error
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if len(c.JWTSecret) < 32 {
		errs = append(errs, errors.New("JWT_SECRET must be at least 32 bytes"))
	}
	if len(c.OTPPepper) < 16 {
		errs = append(errs, errors.New("OTP_PEPPER must be at least 16 bytes"))
	}
	if c.IsProd() && !c.CookieSecure {
		errs = append(errs, errors.New("COOKIE_SECURE must be true in prod"))
	}
	return c, errors.Join(errs...)
}

func get(k, def string) string {
	if v, ok := os.LookupEnv(k); ok && v != "" {
		return v
	}
	return def
}

func getInt(k string, def int) int {
	v, err := strconv.Atoi(os.Getenv(k))
	if err != nil {
		return def
	}
	return v
}

func getBool(k string, def bool) bool {
	v, err := strconv.ParseBool(os.Getenv(k))
	if err != nil {
		return def
	}
	return v
}

func getDur(k string, def time.Duration) time.Duration {
	v, err := time.ParseDuration(os.Getenv(k))
	if err != nil {
		return def
	}
	return v
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// String не раскрывает секреты (безопасно для логов).
func (c Config) String() string {
	return fmt.Sprintf("env=%s http=%s internal=%s s3=%s/%s", c.Env, c.HTTPAddr, c.InternalAddr, c.S3Endpoint, c.S3Bucket)
}
