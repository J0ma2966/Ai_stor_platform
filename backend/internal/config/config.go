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

	// Хранилище биометрии (фото клиента, результаты примерки). Должно физически
	// находиться в Узбекистане. Если BIOMETRIC_S3_ENDPOINT пуст — используется
	// основное хранилище (только dev/test; в prod сервер не стартует).
	BiometricS3Endpoint  string
	BiometricS3Region    string
	BiometricS3Bucket    string
	BiometricS3AccessKey string
	BiometricS3SecretKey string
	BiometricS3UseSSL    bool
	// Страна размещения хранилища биометрии (ISO 3166-1). В prod обязана быть UZ.
	BiometricCountry string

	TryOn TryOnConfig
}

// TryOnConfig — AI-примерка: провайдер, воркер, сроки хранения.
type TryOnConfig struct {
	Provider      string // off | mock | fashn | gemini | http
	APIKey        string // TRYON_API_KEY (fashn, gemini, http — Bearer)
	Endpoint      string // TRYON_ENDPOINT — для http (свой сервер) или переопределения URL провайдера
	Model         string // TRYON_MODEL — модель провайдера (gemini: gemini-2.5-flash-image)
	Timeout       time.Duration
	Workers       int  // одновременных генераций на процесс
	InProcess     bool // запускать воркер внутри API (dev); в prod — отдельный процесс `api worker`
	MaxAttempts   int
	MaxActive     int           // активных (в очереди/в работе) генераций на клиента
	PhotoTTL      time.Duration // несохранённые фото после завершения генерации
	SavedPhotoTTL time.Duration // сохранённые фото без использования
	ResultTTL     time.Duration // результат, если образ не сохранён
	SavedLookTTL  time.Duration // сохранённый образ
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

		BiometricS3Endpoint:  os.Getenv("BIOMETRIC_S3_ENDPOINT"),
		BiometricS3Region:    get("BIOMETRIC_S3_REGION", "uz-tashkent-1"),
		BiometricS3Bucket:    get("BIOMETRIC_S3_BUCKET", "boutique-biometric"),
		BiometricS3AccessKey: os.Getenv("BIOMETRIC_S3_ACCESS_KEY"),
		BiometricS3SecretKey: os.Getenv("BIOMETRIC_S3_SECRET_KEY"),
		BiometricS3UseSSL:    getBool("BIOMETRIC_S3_USE_SSL", true),
		BiometricCountry:     strings.ToUpper(get("BIOMETRIC_STORAGE_COUNTRY", "")),
		TryOn: TryOnConfig{
			Provider:      get("TRYON_PROVIDER", "mock"),
			APIKey:        os.Getenv("TRYON_API_KEY"),
			Endpoint:      os.Getenv("TRYON_ENDPOINT"),
			Model:         os.Getenv("TRYON_MODEL"),
			Timeout:       getDur("TRYON_TIMEOUT", 3*time.Minute),
			Workers:       min(max(getInt("TRYON_WORKERS", 4), 1), 64),
			InProcess:     getBool("TRYON_WORKER_INPROCESS", true),
			MaxAttempts:   min(max(getInt("TRYON_MAX_ATTEMPTS", 3), 1), 10),
			MaxActive:     min(max(getInt("TRYON_MAX_ACTIVE_PER_USER", 2), 1), 20),
			PhotoTTL:      getDur("TRYON_PHOTO_TTL", 24*time.Hour),
			SavedPhotoTTL: getDur("TRYON_SAVED_PHOTO_TTL", 180*24*time.Hour),
			ResultTTL:     getDur("TRYON_RESULT_TTL", 30*24*time.Hour),
			SavedLookTTL:  getDur("TRYON_SAVED_LOOK_TTL", 365*24*time.Hour),
		},
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
	errs = append(errs, c.validateTryOn()...)
	return c, errors.Join(errs...)
}

// TryOnEnabled: TRYON_PROVIDER=off полностью выключает AI-примерку (решение
// владельца от 2026-11-03: запуск prod без примерки). Тогда не нужны ни ключи
// провайдера, ни хранилище биометрии, а загрузка фото тела запрещена.
func (c Config) TryOnEnabled() bool { return c.TryOn.Provider != "off" }

func (c Config) validateTryOn() []error {
	var errs []error
	switch c.TryOn.Provider {
	case "off":
		return nil
	case "mock":
		if c.IsProd() {
			errs = append(errs, errors.New("TRYON_PROVIDER=mock is not allowed in prod"))
		}
	case "fashn", "gemini":
		if c.TryOn.APIKey == "" {
			errs = append(errs, fmt.Errorf("TRYON_API_KEY is required for TRYON_PROVIDER=%s", c.TryOn.Provider))
		}
	case "http":
		if c.TryOn.Endpoint == "" {
			errs = append(errs, errors.New("TRYON_ENDPOINT is required for TRYON_PROVIDER=http"))
		}
	default:
		errs = append(errs, fmt.Errorf("unknown TRYON_PROVIDER %q (off|mock|fashn|gemini|http)", c.TryOn.Provider))
	}
	if c.IsProd() {
		if c.BiometricS3Endpoint == "" {
			errs = append(errs, errors.New("BIOMETRIC_S3_ENDPOINT is required in prod (biometric data must be stored in Uzbekistan)"))
		}
		if c.BiometricCountry != "UZ" {
			errs = append(errs, errors.New("BIOMETRIC_STORAGE_COUNTRY must be UZ in prod"))
		}
	}
	return errs
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
	bio := "shared"
	if c.BiometricS3Endpoint != "" {
		bio = c.BiometricS3Endpoint + "/" + c.BiometricS3Bucket + " (" + c.BiometricCountry + ")"
	}
	return fmt.Sprintf("env=%s http=%s internal=%s s3=%s/%s biometric=%s tryon=%s",
		c.Env, c.HTTPAddr, c.InternalAddr, c.S3Endpoint, c.S3Bucket, bio, c.TryOn.Provider)
}
