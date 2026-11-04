// Package config читает конфигурацию только из переменных окружения (12-factor).
// Секреты никогда не хранятся в репозитории.
package config

import (
	"errors"
	"fmt"
	"net/netip"
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

	// SMS: log (только dev/test) | eskiz
	SMSProvider   string
	EskizURL      string
	EskizEmail    string
	EskizPassword string
	EskizFrom     string
	SMSTemplate   string // одобренный в кабинете Eskiz текст, %s — код

	// Платежи. Провайдер включён, только если заданы его ключи.
	PaymentReturnURL string // куда вернуть клиента после оплаты, напр. https://shop.uz/account/balance
	ClickServiceID   string
	ClickMerchantID  string
	ClickSecretKey   string
	ClickAllowedIPs  []string // CIDR/IP, пусто — без проверки IP (подпись проверяется всегда)
	PaymeMerchantID  string
	PaymeKey         string // ключ кассы (тестовый или боевой)
	PaymeTest        bool   // песочница checkout.test.paycom.uz
	PaymeAccountKey  string // имя поля account в кабинете Payme, по умолчанию payment_id
	PaymeAllowedIPs  []string
	PaymeIKPU        string // код ИКПУ (МХИК) для фискального чека, пусто — без detail
	PaymePackageCode string
	PaymeVATPercent  int
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
	Provider      string // off | mock | fashn | gemini | http | openai
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

	OpenAI OpenAIConfig // TRYON_PROVIDER=openai
}

// OpenAIConfig — OpenAI Images API (POST /v1/images/edits): один запрос на генерацию,
// результат — одно изображение с тремя ракурсами. Ключ берётся только из окружения
// и никогда не логируется (Config.String его не выводит).
type OpenAIConfig struct {
	APIKey  string        // OPENAI_API_KEY
	BaseURL string        // OPENAI_BASE_URL (по умолчанию https://api.openai.com) — для тестов и прокси
	Model   string        // OPENAI_IMAGE_MODEL
	Quality string        // OPENAI_IMAGE_QUALITY: low | medium | high | xhigh | max | auto
	Size    string        // OPENAI_IMAGE_SIZE: WxH или auto
	Format  string        // OPENAI_IMAGE_FORMAT: jpeg | png | webp
	Timeout time.Duration // OPENAI_TIMEOUT_SECONDS — на один HTTP-запрос к OpenAI
	// ProdApproved — OPENAI_PROD_APPROVED=true: юридическое подтверждение передачи
	// фото тела в OpenAI. Без него prod не стартует с TRYON_PROVIDER=openai (ТЗ §12).
	ProdApproved bool
}

// JobTimeout — таймаут одного задания воркера: для openai не меньше таймаута
// HTTP-запроса плюс запас на загрузку фото и сохранение результата.
func (t TryOnConfig) JobTimeout() time.Duration {
	if t.Provider == "openai" {
		return max(t.Timeout, t.OpenAI.Timeout+30*time.Second)
	}
	return t.Timeout
}

func (c Config) ClickEnabled() bool {
	return c.ClickServiceID != "" && c.ClickMerchantID != "" && c.ClickSecretKey != ""
}

func (c Config) PaymeEnabled() bool { return c.PaymeMerchantID != "" && c.PaymeKey != "" }

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
			OpenAI: OpenAIConfig{
				APIKey:       strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
				BaseURL:      os.Getenv("OPENAI_BASE_URL"),
				Model:        get("OPENAI_IMAGE_MODEL", "gpt-image-2.5-sunburst"),
				Quality:      get("OPENAI_IMAGE_QUALITY", "high"),
				Size:         get("OPENAI_IMAGE_SIZE", "1536x1024"),
				Format:       get("OPENAI_IMAGE_FORMAT", "jpeg"),
				Timeout:      time.Duration(min(max(getInt("OPENAI_TIMEOUT_SECONDS", 180), 10), 900)) * time.Second,
				ProdApproved: getBool("OPENAI_PROD_APPROVED", false),
			},
		},
		SMSProvider:      get("SMS_PROVIDER", "log"),
		EskizURL:         get("ESKIZ_URL", "https://notify.eskiz.uz/api"),
		EskizEmail:       os.Getenv("ESKIZ_EMAIL"),
		EskizPassword:    os.Getenv("ESKIZ_PASSWORD"),
		EskizFrom:        get("ESKIZ_FROM", "4546"),
		SMSTemplate:      get("SMS_OTP_TEMPLATE", "Kod dlya vhoda: %s"),
		PaymentReturnURL: os.Getenv("PAYMENT_RETURN_URL"),
		ClickServiceID:   os.Getenv("CLICK_SERVICE_ID"),
		ClickMerchantID:  os.Getenv("CLICK_MERCHANT_ID"),
		ClickSecretKey:   os.Getenv("CLICK_SECRET_KEY"),
		ClickAllowedIPs:  splitList(os.Getenv("CLICK_ALLOWED_IPS")),
		PaymeMerchantID:  os.Getenv("PAYME_MERCHANT_ID"),
		PaymeKey:         os.Getenv("PAYME_KEY"),
		// песочница по умолчанию только вне prod: забытая переменная не уведёт клиентов на test.paycom.uz
		PaymeTest:        getBool("PAYME_TEST", get("APP_ENV", "dev") != "prod"),
		PaymeAccountKey:  get("PAYME_ACCOUNT_KEY", "payment_id"),
		PaymeAllowedIPs:  splitList(os.Getenv("PAYME_ALLOWED_IPS")),
		PaymeIKPU:        os.Getenv("PAYME_IKPU"),
		PaymePackageCode: os.Getenv("PAYME_PACKAGE_CODE"),
		PaymeVATPercent:  getInt("PAYME_VAT_PERCENT", 0),
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
	switch c.SMSProvider {
	case "log":
		if c.IsProd() {
			errs = append(errs, errors.New("SMS_PROVIDER=log is forbidden in prod (use eskiz)"))
		}
	case "eskiz":
		if c.EskizEmail == "" || c.EskizPassword == "" {
			errs = append(errs, errors.New("ESKIZ_EMAIL and ESKIZ_PASSWORD are required for SMS_PROVIDER=eskiz"))
		}
		if strings.Count(c.SMSTemplate, "%") != strings.Count(c.SMSTemplate, "%s") || strings.Count(c.SMSTemplate, "%s") > 1 {
			errs = append(errs, errors.New("SMS_OTP_TEMPLATE must contain at most one %s and no other % verbs"))
		}
	default:
		errs = append(errs, fmt.Errorf("unknown SMS_PROVIDER %q", c.SMSProvider))
	}
	for _, ip := range append(append([]string{}, c.ClickAllowedIPs...), c.PaymeAllowedIPs...) {
		if _, err := netip.ParsePrefix(ip); err != nil {
			if _, err := netip.ParseAddr(ip); err != nil {
				errs = append(errs, fmt.Errorf("invalid IP or CIDR %q in CLICK_/PAYME_ALLOWED_IPS", ip))
			}
		}
	}
	// Поле error в Click Complete не подписано, а ключ Payme статичен: в prod вебхуки
	// принимаются только с IP провайдеров.
	if c.IsProd() && c.ClickEnabled() && len(c.ClickAllowedIPs) == 0 {
		errs = append(errs, errors.New("CLICK_ALLOWED_IPS is required in prod when Click is enabled"))
	}
	if c.IsProd() && c.PaymeEnabled() && len(c.PaymeAllowedIPs) == 0 {
		errs = append(errs, errors.New("PAYME_ALLOWED_IPS is required in prod when Payme is enabled"))
	}
	if (c.ClickEnabled() || c.PaymeEnabled()) && c.PaymentReturnURL == "" {
		errs = append(errs, errors.New("PAYMENT_RETURN_URL is required when a payment provider is configured"))
	}
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
	case "openai":
		errs = append(errs, c.validateOpenAI()...)
	default:
		errs = append(errs, fmt.Errorf("unknown TRYON_PROVIDER %q (off|mock|fashn|gemini|http|openai)", c.TryOn.Provider))
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

var (
	openAIQualities = map[string]bool{"low": true, "medium": true, "high": true, "xhigh": true, "max": true, "auto": true}
	openAIFormats   = map[string]bool{"jpeg": true, "png": true, "webp": true}
)

// validateOpenAI: ключ обязателен в любом окружении (без него — понятная ошибка
// конфигурации, а не фиктивная генерация); в prod — ещё и явное юридическое одобрение.
func (c Config) validateOpenAI() []error {
	o := c.TryOn.OpenAI
	var errs []error
	if o.APIKey == "" {
		errs = append(errs, errors.New("OPENAI_API_KEY is required for TRYON_PROVIDER=openai"))
	}
	if o.Model == "" {
		errs = append(errs, errors.New("OPENAI_IMAGE_MODEL must not be empty"))
	}
	if !openAIQualities[o.Quality] {
		errs = append(errs, fmt.Errorf("OPENAI_IMAGE_QUALITY %q is not one of low|medium|high|xhigh|max|auto", o.Quality))
	}
	if !openAIFormats[o.Format] {
		errs = append(errs, fmt.Errorf("OPENAI_IMAGE_FORMAT %q is not one of jpeg|png|webp", o.Format))
	}
	if err := ValidOpenAISize(o.Size); err != nil {
		errs = append(errs, err)
	}
	if c.IsProd() && !o.ProdApproved {
		errs = append(errs, errors.New("TRYON_PROVIDER=openai in prod needs OPENAI_PROD_APPROVED=true "+
			"(body photos are biometric data; confirm the legal basis for sending them to OpenAI first, see docs/TRYON.md)"))
	}
	return errs
}

// ValidOpenAISize проверяет OPENAI_IMAGE_SIZE: auto или WxH, обе стороны кратны 16,
// соотношение сторон от 1:3 до 3:1, не больше 3840x2160 по площади (правила gpt-image-2.x).
func ValidOpenAISize(s string) error {
	if s == "auto" {
		return nil
	}
	var w, h int
	if n, err := fmt.Sscanf(s, "%dx%d", &w, &h); err != nil || n != 2 || fmt.Sprintf("%dx%d", w, h) != s {
		return fmt.Errorf("OPENAI_IMAGE_SIZE %q must be auto or WIDTHxHEIGHT", s)
	}
	if w < 256 || h < 256 || w%16 != 0 || h%16 != 0 || w > 3*h || h > 3*w || w*h > 3840*2160 {
		return fmt.Errorf("OPENAI_IMAGE_SIZE %q: sides must be >=256 and divisible by 16, aspect 1:3..3:1, at most 3840x2160 pixels", s)
	}
	return nil
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
