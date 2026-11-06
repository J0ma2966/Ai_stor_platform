package tryon

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // декодер PNG (ответ и входные фото)
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // декодер WebP
)

// OpenAI — OpenAI Images API, POST /v1/images/edits (модель по умолчанию
// gpt-image-2.5-sunburst). Одна генерация = один HTTP-запрос: в него уходят три фото
// клиента (спереди, сбоку, сзади) и от 1 до 4 фото вещей, в ответ приходит ОДНО
// изображение, на котором модель сама рисует три ракурса. Compose не нужен.
//
// Ключ передаётся только в заголовке Authorization и не попадает ни в ошибки, ни в
// логи. Фото клиента в логи не пишутся (в ошибках — только код и сообщение OpenAI).
type OpenAI struct {
	APIKey  string
	BaseURL string // по умолчанию https://api.openai.com
	Model   string // gpt-image-2.5-sunburst
	Quality string // high
	Size    string // 1536x1024 | auto
	Format  string // jpeg
	HTTP    *http.Client
}

func (o *OpenAI) Name() string { return "openai" }

// Generate (один ракурс) не используется: воркер видит CompositeProvider и вызывает
// GenerateComposite. Метод нужен только для совместимости с интерфейсом Provider.
func (o *OpenAI) Generate(context.Context, Request) ([]byte, error) {
	return nil, permanent(CodeProviderFailure, "openai provider supports only single-request composite generation")
}

const (
	openAIMaxInputSide  = 2048     // длинная сторона входного фото после подготовки
	openAIMaxInputBytes = 20 << 20 // на одно входное изображение (лимит API заметно выше)
	openAIMaxOutputPx   = 40_000_000
)

var (
	mOpenAITokens = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tryon_openai_tokens_total", Help: "OpenAI Images API usage tokens (for cost tracking)",
	}, []string{"kind"})
	mOpenAIRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tryon_openai_requests_total", Help: "OpenAI Images API requests by HTTP status (0 = network error/timeout)",
	}, []string{"status"})
)

// zoneOrder — порядок вещей в запросе и в промпте.
var zoneOrder = []string{ZoneHeadwear, ZoneTop, ZoneBottom, ZoneShoes}

var openAIZoneText = map[string]string{
	ZoneHeadwear: "HEADWEAR (hat / cap / headwear)",
	ZoneTop:      "TOP (outerwear, jacket, shirt or t-shirt)",
	ZoneBottom:   "BOTTOM (trousers or skirt)",
	ZoneShoes:    "SHOES (footwear)",
}

// openAISetText — «двойка»: верх и низ одного комплекта на одном фото.
const openAISetText = "TWO-PIECE SET (matching top AND bottom shown together in one product photo; dress the person in both pieces)"

// labeledImage — подготовленное изображение и его роль в промпте.
type labeledImage struct {
	label string
	data  []byte
}

// PrepareComposite проверяет входные данные и раскладывает их в фиксированном
// порядке: FRONT, SIDE, BACK, затем вещи HEADWEAR, TOP, BOTTOM, SHOES (только
// выбранные). Каждое фото проверяется (не пустое, JPEG/PNG/WebP, декодируется),
// уменьшается до openAIMaxInputSide и перекодируется в JPEG.
func PrepareComposite(req CompositeRequest) ([]labeledImage, []string, error) {
	photos := []struct {
		label string
		data  []byte
	}{
		{"FRONT reference photo of the person", req.FrontPhoto},
		{"SIDE reference photo of the person", req.SidePhoto},
		{"BACK reference photo of the person", req.BackPhoto},
	}
	var out []labeledImage
	for _, p := range photos {
		if len(p.data) == 0 {
			return nil, nil, permanent(CodePhotoMissing, "missing %s", p.label)
		}
		img, err := prepareImage(p.data)
		if err != nil {
			return nil, nil, permanent(CodePhotoRejected, "%s: %v", p.label, err)
		}
		out = append(out, labeledImage{p.label, img})
	}
	byZone := map[string]Garment{}
	covered := map[string]bool{}
	for _, g := range req.Garments {
		if openAIZoneText[g.Zone] == "" {
			return nil, nil, permanent(CodeUnsupported, "unknown garment zone %q", g.Zone)
		}
		for _, z := range g.covers() {
			if covered[z] {
				return nil, nil, permanent(CodeUnsupported, "more than one garment for zone %s", z)
			}
			covered[z] = true
		}
		byZone[g.Zone] = g
	}
	if len(byZone) == 0 {
		return nil, nil, permanent(CodeProductMissing, "at least one garment is required")
	}
	var missing []string
	for _, z := range zoneOrder {
		g, ok := byZone[z]
		if !ok {
			if !covered[z] { // низ, закрытый «двойкой», не «отсутствует»
				missing = append(missing, z)
			}
			continue
		}
		if len(g.Image) == 0 {
			return nil, nil, permanent(CodeProductMissing, "empty image for %s", z)
		}
		img, err := prepareImage(g.Image)
		if err != nil {
			return nil, nil, permanent(CodeProductMissing, "garment %s: %v", z, err)
		}
		label := openAIZoneText[z]
		if g.Set && z == ZoneTop {
			label = openAISetText
		}
		out = append(out, labeledImage{label + " garment reference", img})
	}
	return out, missing, nil
}

func prepareImage(b []byte) ([]byte, error) {
	if len(b) == 0 {
		return nil, errors.New("empty image")
	}
	ct := http.DetectContentType(b)
	if ct != "image/jpeg" && ct != "image/png" && ct != "image/webp" {
		return nil, fmt.Errorf("unsupported format %s", ct)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("corrupt image: %v", err)
	}
	if cfg.Width < 16 || cfg.Height < 16 || cfg.Width*cfg.Height > maxDecodePx {
		return nil, fmt.Errorf("bad image size %dx%d", cfg.Width, cfg.Height)
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("corrupt image: %v", err)
	}
	if bd := img.Bounds(); bd.Dx() > openAIMaxInputSide || bd.Dy() > openAIMaxInputSide {
		w, h := bd.Dx(), bd.Dy()
		if w >= h {
			w, h = openAIMaxInputSide, max(h*openAIMaxInputSide/w, 1)
		} else {
			w, h = max(w*openAIMaxInputSide/h, 1), openAIMaxInputSide
		}
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, bd, draw.Over, nil)
		img = dst
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	if out.Len() > openAIMaxInputBytes {
		return nil, errors.New("image too large after re-encoding")
	}
	return out.Bytes(), nil
}

// OpenAIPrompt — промпт из ТЗ §5 плюс явная карта «какое изображение что означает»,
// список категорий без референса, пол и параметры тела. Названия товаров в промпт не
// попадают: это текст продавца, а промпт должен оставаться под нашим контролем.
func OpenAIPrompt(images []labeledImage, missing []string, gender string, body Body) string {
	var b strings.Builder
	b.WriteString("Create one photorealistic virtual clothing try-on image showing the same person in three full-body views arranged horizontally in a single image.\n\n")
	b.WriteString("INPUT IMAGES (in the order they are attached):\n")
	for i, im := range images {
		fmt.Fprintf(&b, "- Image %d: %s.\n", i+1, im.label)
	}
	switch gender {
	case "female":
		b.WriteString("\nThe person is a woman.\n")
	case "male":
		b.WriteString("\nThe person is a man.\n")
	}
	if body.HeightCM != nil || body.WeightKG != nil || len(body.Params) > 0 {
		b.WriteString("Body measurements for realistic fit:")
		if body.HeightCM != nil {
			fmt.Fprintf(&b, " height %d cm;", *body.HeightCM)
		}
		if body.WeightKG != nil {
			fmt.Fprintf(&b, " weight %.1f kg;", *body.WeightKG)
		}
		for k, v := range body.Params {
			if name := sanitizeParam(k); name != "" {
				fmt.Fprintf(&b, " %s %.1f cm;", name, v)
			}
		}
		b.WriteString("\n")
	}
	b.WriteString(`
The three panels must show, from left to right:
1. FRONT VIEW - the person facing forward (use Image 1 as the reference).
2. SIDE VIEW - the person viewed from the side (use Image 2 as the reference).
3. BACK VIEW - the person viewed from behind (use Image 3 as the reference).

IDENTITY PRESERVATION:
- Preserve the identity and facial appearance of the person.
- Preserve natural body proportions, skin tone, hairstyle and physical characteristics.
- Use the three supplied photographs as references for the corresponding views.
- Do not invent a different person.

CLOTHING:
- Apply all supplied garment reference images to the person, each to its own body zone as labelled above.
- A TWO-PIECE SET reference shows a matching top and bottom together: dress the person in both pieces from that one image (it replaces both the top and the bottom).
- Preserve the original colors, patterns, materials, textures, logos and design details.
- Do not invent additional clothing items.
- Do not change the original garment design.
- Make the clothing look naturally worn and realistically fitted.

OPTIONAL CATEGORIES:
- If a garment category is not provided, preserve the corresponding original clothing from the person's photographs.
- Do not invent a hat, shoes, top or bottom if its reference image was not provided.
`)
	if len(missing) > 0 {
		names := make([]string, len(missing))
		for i, z := range missing {
			names[i] = strings.ToUpper(z)
		}
		fmt.Fprintf(&b, "- No reference was provided for: %s. Keep the person's own items for these zones exactly as in their photographs (or none, if they wear none).\n", strings.Join(names, ", "))
	}
	b.WriteString(`
COMPOSITION:
- Exactly three views of the same person.
- Full body, including head and feet.
- Equal-sized panels.
- Consistent lighting and background.
- No additional people.
- No text, captions, watermarks or decorative elements.
- No cropped body parts.
- Photorealistic result.

The output must be one image, not three separate files.`)
	return b.String()
}

type openAIResponse struct {
	Data []struct {
		B64JSON string `json:"b64_json"`
	} `json:"data"`
	Usage *struct {
		InputTokens  int64 `json:"input_tokens"`
		OutputTokens int64 `json:"output_tokens"`
	} `json:"usage"`
}

type openAIError struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Param   string `json:"param"`
		Code    string `json:"code"`
	} `json:"error"`
}

// GenerateComposite выполняет ровно один запрос к /v1/images/edits (второй — только
// если OpenAI отверг сам параметр size: тогда повтор с size=auto, как требует ТЗ §2).
func (o *OpenAI) GenerateComposite(ctx context.Context, req CompositeRequest) ([]byte, error) {
	images, missing, err := PrepareComposite(req)
	if err != nil {
		return nil, err
	}
	prompt := OpenAIPrompt(images, missing, req.Gender, req.Body)
	size := o.Size
	if size == "" {
		size = "1536x1024"
	}
	out, err := o.call(ctx, images, prompt, size)
	var se *sizeRejected
	if errors.As(err, &se) && size != "auto" {
		out, err = o.call(ctx, images, prompt, "auto")
	}
	if errors.As(err, &se) {
		return nil, permanent(CodeProviderFailure, "openai rejected size: %s", se.msg)
	}
	return out, err
}

type sizeRejected struct{ msg string }

func (e *sizeRejected) Error() string { return "openai rejected size: " + e.msg }

func (o *OpenAI) call(ctx context.Context, images []labeledImage, prompt, size string) ([]byte, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fields := [][2]string{
		{"model", or(o.Model, "gpt-image-2.5-sunburst")},
		{"prompt", prompt},
		{"n", "1"},
		{"size", size},
		{"quality", or(o.Quality, "high")},
		{"output_format", or(o.Format, "jpeg")},
	}
	for _, f := range fields {
		if err := mw.WriteField(f[0], f[1]); err != nil {
			return nil, err
		}
	}
	for i, im := range images {
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image[]"; filename="image%d.jpg"`, i+1))
		h.Set("Content-Type", "image/jpeg")
		part, err := mw.CreatePart(h)
		if err != nil {
			return nil, err
		}
		if _, err := part.Write(im.data); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	base := strings.TrimRight(or(o.BaseURL, "https://api.openai.com"), "/")
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/images/edits", &body)
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Content-Type", mw.FormDataContentType())
	hreq.Header.Set("Authorization", "Bearer "+o.APIKey)
	res, err := o.HTTP.Do(hreq)
	if err != nil {
		mOpenAIRequests.WithLabelValues("0").Inc()
		// таймаут или сеть — временная ошибка, воркер повторит; текст ошибки без ключа
		return nil, fmt.Errorf("openai request: %w", err)
	}
	defer res.Body.Close()
	mOpenAIRequests.WithLabelValues(fmt.Sprint(res.StatusCode)).Inc()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponse))
	if err != nil {
		return nil, fmt.Errorf("openai read: %w", err)
	}
	if res.StatusCode >= 300 {
		return nil, openAIStatusErr(res.StatusCode, raw)
	}
	var out openAIResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("openai: invalid json response: %v", err)
	}
	if out.Usage != nil {
		mOpenAITokens.WithLabelValues("input").Add(float64(out.Usage.InputTokens))
		mOpenAITokens.WithLabelValues("output").Add(float64(out.Usage.OutputTokens))
	}
	if len(out.Data) == 0 || out.Data[0].B64JSON == "" {
		return nil, errors.New("openai: empty response (no image)") // временная: повтор
	}
	img, err := base64.StdEncoding.DecodeString(out.Data[0].B64JSON)
	if err != nil {
		return nil, fmt.Errorf("openai: invalid base64 image: %v", err)
	}
	return normalizeResult(img)
}

// normalizeResult проверяет, что OpenAI вернул настоящее изображение разумного
// размера, и перекодирует его в JPEG (хранилище результатов хранит image/jpeg,
// заодно удаляются все встроенные метаданные).
func normalizeResult(b []byte) ([]byte, error) {
	ct := http.DetectContentType(b)
	if ct != "image/jpeg" && ct != "image/png" && ct != "image/webp" {
		return nil, fmt.Errorf("openai: invalid image (%s)", ct)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || cfg.Width < 64 || cfg.Height < 64 || cfg.Width*cfg.Height > openAIMaxOutputPx {
		return nil, fmt.Errorf("openai: invalid image header (%dx%d): %v", cfg.Width, cfg.Height, err)
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("openai: corrupt image: %v", err)
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 92}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// openAIStatusErr переводит ответ OpenAI в ошибку воркера:
//   - 400 модерация → content_blocked (без повтора);
//   - 400 по параметру size → sizeRejected (повтор с size=auto);
//   - 400, 401, 403, 404 и прочие 4xx → постоянная provider_error (повтор не поможет;
//     текст ответа на 401 не сохраняется — в нём бывает фрагмент ключа);
//   - 429 insufficient_quota / billing → постоянная (кончились деньги на счёте);
//   - 408, 409, 429 (rate limit), 5xx → временная, воркер повторит с backoff.
func openAIStatusErr(code int, raw []byte) error {
	var e openAIError
	_ = json.Unmarshal(raw, &e)
	msg := e.Error.Message
	if len(msg) > 300 {
		msg = msg[:300]
	}
	desc := strings.TrimSpace(strings.Join([]string{e.Error.Code, e.Error.Type, msg}, " "))
	switch {
	case code == http.StatusUnauthorized:
		return permanent(CodeProviderFailure, "openai auth failed (401): check OPENAI_API_KEY")
	case code == http.StatusForbidden:
		return permanent(CodeProviderFailure, "openai access denied (403): %s", or(e.Error.Code, "no access to model or organization not verified"))
	case code == http.StatusTooManyRequests && (e.Error.Code == "insufficient_quota" || e.Error.Type == "insufficient_quota" ||
		e.Error.Code == "credit_balance_exhausted" || strings.Contains(e.Error.Code, "billing")):
		return permanent(CodeProviderFailure, "openai quota exhausted (429): %s", desc)
	case code == http.StatusBadRequest && (e.Error.Code == "moderation_blocked" || e.Error.Code == "content_policy_violation" ||
		strings.Contains(e.Error.Type, "safety") || strings.Contains(strings.ToLower(msg), "safety system")):
		return permanent(CodeContentBlocked, "openai moderation: %s", desc)
	case code == http.StatusBadRequest && e.Error.Param == "size":
		return &sizeRejected{msg: msg}
	case code >= 400 && code < 500 && code != http.StatusRequestTimeout && code != http.StatusConflict && code != http.StatusTooManyRequests:
		return permanent(CodeProviderFailure, "openai rejected request (%d): %s", code, desc)
	}
	return fmt.Errorf("openai status %d: %s", code, desc)
}

func or(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
