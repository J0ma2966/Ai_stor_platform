package media

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/boutique-market/backend/internal/auth"
	"github.com/boutique-market/backend/internal/httpx"
	"github.com/boutique-market/backend/internal/ratelimit"
)

type Handlers struct {
	Store      Storage
	Limiter    *ratelimit.Limiter
	Redis      *redis.Client
	MaxBytes   int64
	PublicBase string
	// Sem ограничивает одновременные декодирования на инстанс (память). Создаётся NewSem.
	Sem chan struct{}
	// Disabled — цели загрузки, выключенные конфигурацией (например, фото тела
	// при выключенной AI-примерке: нет хранилища биометрии в Узбекистане).
	Disabled map[Purpose]bool
}

func NewSem(n int) chan struct{} { return make(chan struct{}, n) }

var errBusy = httpx.NewError(http.StatusServiceUnavailable, "upload_busy", "too many uploads in progress, retry shortly")

var ruleUpload = ratelimit.Rule{Name: "upload_user", Limit: 60, Window: time.Hour}

// rolePurposes — какая роль какие файлы может загружать.
var rolePurposes = map[auth.Role]map[Purpose]bool{
	auth.RoleClient: {PurposeBodyPhoto: true, PurposeDeliveryPhoto: true},
	auth.RoleStore:  {PurposeProductImage: true, PurposeStoreBranding: true},
	auth.RoleAdmin:  {PurposeProductImage: true, PurposeStoreBranding: true},
}

// Upload: POST /v1/uploads?purpose=delivery  (тело — сам файл, Content-Type image/*)
// Ответ: {"key": "...", "url": "<подписанная ссылка на 15 мин>"}.
// Ключ потом передаётся в бизнес-запрос (например, оформление заказа), где проверяется OwnsKey.
func (h *Handlers) Upload(w http.ResponseWriter, r *http.Request) {
	p := auth.MustFrom(r.Context())
	purpose := Purpose(r.URL.Query().Get("purpose"))
	if !purpose.Valid() || !rolePurposes[p.Role][purpose] || h.Disabled[purpose] {
		httpx.Fail(w, r, httpx.NewError(http.StatusUnprocessableEntity, "invalid_purpose", "purpose not allowed"))
		return
	}
	if !h.Limiter.Check(w, r, ruleUpload, p.UserID.String()) {
		return
	}
	// один одновременный upload на пользователя (во всех инстансах)
	lock := "upl:lock:" + p.UserID.String()
	if ok, err := h.Redis.SetNX(r.Context(), lock, 1, 90*time.Second).Result(); err == nil && !ok {
		w.Header().Set("Retry-After", "2")
		httpx.Fail(w, r, httpx.NewError(http.StatusTooManyRequests, "upload_in_progress", "previous upload is still processing"))
		return
	}
	defer h.Redis.Del(context.WithoutCancel(r.Context()), lock)
	// глобальный лимит декодирований на инстанс: ждём не дольше 3 с, иначе 503
	select {
	case h.Sem <- struct{}{}:
		defer func() { <-h.Sem }()
	case <-time.After(3 * time.Second):
		w.Header().Set("Retry-After", "3")
		httpx.Fail(w, r, errBusy)
		return
	case <-r.Context().Done():
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.MaxBytes+1)
	data, err := Process(r.Body, h.MaxBytes)
	switch {
	case errors.Is(err, ErrTooLarge):
		httpx.Fail(w, r, httpx.NewError(http.StatusRequestEntityTooLarge, "file_too_large", "file too large"))
		return
	case errors.Is(err, ErrUnsupported):
		httpx.Fail(w, r, httpx.NewError(http.StatusUnsupportedMediaType, "unsupported_type", "only JPEG, PNG, WebP"))
		return
	case errors.Is(err, ErrCorrupt):
		httpx.Fail(w, r, httpx.NewError(http.StatusUnprocessableEntity, "corrupt_image", "image could not be decoded"))
		return
	case err != nil:
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			httpx.Fail(w, r, httpx.NewError(http.StatusRequestEntityTooLarge, "file_too_large", "file too large"))
			return
		}
		httpx.Fail(w, r, err)
		return
	}
	key := NewKey(p.UserID, purpose)
	if err := h.Store.Put(r.Context(), key, data, "image/jpeg"); err != nil {
		httpx.LoggerFrom(r.Context()).Error("upload put failed", slog.String("err", err.Error()))
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	u := PublicURL(h.PublicBase, key)
	if u == "" {
		if u, err = h.Store.PresignGet(r.Context(), key, 15*time.Minute); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"key": key, "url": u, "bytes": len(data)})
}
