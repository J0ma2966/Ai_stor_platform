// Package tryon — AI-примерка: API для клиента, очередь генераций на PostgreSQL,
// воркер, провайдеры AI-моделей и сроки хранения биометрии.
//
// Поток: клиент загружает 3 фото (POST /v1/uploads?purpose=body) → POST
// /v1/tryon/generations списывает токены и ставит задание в очередь (одна
// транзакция) → воркер берёт задание (SELECT … FOR UPDATE SKIP LOCKED), для
// каждого ракурса вызывает провайдера, склеивает 3 ракурса в одну картинку и
// сохраняет её в хранилище биометрии → клиент опрашивает GET /v1/tryon/generations/{id}.
// Техническая ошибка после всех повторов → статус failed и возврат токенов.
package tryon

import (
	"context"
	"errors"
	"fmt"
)

// Зоны манекена (совпадают с enum category_zone).
const (
	ZoneHeadwear = "headwear"
	ZoneTop      = "top"
	ZoneBottom   = "bottom"
	ZoneShoes    = "shoes"
)

// Ракурсы фото в порядке склейки результата.
var Angles = []string{"front", "side", "back"}

type Garment struct {
	Zone  string
	Title string
	Brand string
	Image []byte // фото товара того же ракурса (или фронтальное), JPEG
	// Set — «двойка» (products.is_set): верх и низ на одном фото. Бывает только в зоне
	// top и занимает также зону bottom — отдельного низа в запросе тогда нет.
	Set bool
}

// covers — зоны манекена, которые занимает вещь.
func (g Garment) covers() []string {
	if g.Set && g.Zone == ZoneTop {
		return []string{ZoneTop, ZoneBottom}
	}
	return []string{g.Zone}
}

// Body — необязательные параметры тела.
type Body struct {
	HeightCM *int16             `json:"height_cm,omitempty"`
	WeightKG *float64           `json:"weight_kg,omitempty"`
	Params   map[string]float64 `json:"params,omitempty"`
}

type Request struct {
	Angle    string // front | side | back
	Person   []byte // фото клиента, JPEG
	Garments []Garment
	Gender   string // male | female | ""
	Body     Body
}

// Provider генерирует изображение человека в выбранных вещах для одного ракурса.
// Ответ — PNG или JPEG. Реализация должна уважать ctx (таймаут задания).
type Provider interface {
	Name() string
	Generate(ctx context.Context, req Request) ([]byte, error)
}

// CompositeRequest — все входные данные одной генерации для провайдеров, которые
// сами рисуют итоговое изображение с тремя ракурсами (OpenAI): три фото клиента и
// по одной вещи на зону. Фото — JPEG/PNG/WebP.
type CompositeRequest struct {
	FrontPhoto []byte
	SidePhoto  []byte
	BackPhoto  []byte
	Garments   []Garment // 1..4, не больше одной на зону
	Gender     string    // male | female | ""
	Body       Body
}

// CompositeProvider — провайдер «один запрос — одно изображение с тремя ракурсами».
// Если провайдер реализует этот интерфейс, воркер вызывает GenerateComposite ровно
// один раз на попытку и НЕ склеивает результат через Compose. Остальные провайдеры
// (Gemini, FASHN, HTTP, Mock) работают по-старому: Generate на каждый ракурс + Compose.
type CompositeProvider interface {
	Provider
	GenerateComposite(ctx context.Context, req CompositeRequest) ([]byte, error)
}

// PermanentError — ошибка, которую повтор не исправит (фото без человека, модерация
// контента). Code уходит фронту как error_code.
type PermanentError struct {
	Code string
	Msg  string
}

func (e *PermanentError) Error() string { return e.Code + ": " + e.Msg }

func permanent(code, format string, a ...any) error {
	return &PermanentError{Code: code, Msg: fmt.Sprintf(format, a...)}
}

// Коды ошибок генерации для фронта.
const (
	CodePhotoRejected   = "photo_rejected"    // на фото не найден человек/поза
	CodeContentBlocked  = "content_blocked"   // модерация провайдера
	CodeUnsupported     = "unsupported_items" // провайдер не умеет эти категории
	CodePhotoMissing    = "photo_missing"     // фото удалено до запуска
	CodeProductMissing  = "product_unavailable"
	CodeProviderFailure = "provider_error"            // техническая ошибка после всех повторов
	CodeTransferConsent = "transfer_consent_required" // нет согласия на передачу фото этому провайдеру
)

// ErrorCode возвращает код для фронта.
func ErrorCode(err error) string {
	var pe *PermanentError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return CodeProviderFailure
}

func IsPermanent(err error) bool {
	var pe *PermanentError
	return errors.As(err, &pe)
}
