package tryon

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// Gemini — Google Gemini API, модель генерации/редактирования изображений
// (по умолчанию gemini-2.5-flash-image). Одним запросом на ракурс надевает все
// выбранные вещи, включая головной убор и обувь.
type Gemini struct {
	APIKey  string
	Model   string
	BaseURL string // по умолчанию https://generativelanguage.googleapis.com
	HTTP    *http.Client
}

func (g *Gemini) Name() string { return "gemini" }

var angleText = map[string]string{
	"front": "from the front",
	"side":  "from the side (profile)",
	"back":  "from the back",
}

var zoneText = map[string]string{
	ZoneHeadwear: "headwear", ZoneTop: "top / outerwear", ZoneBottom: "trousers / skirt", ZoneShoes: "shoes",
}

// Prompt — инструкция модели. Экспортирована для тестов и подбора формулировок.
func Prompt(req Request) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Virtual try-on. The first image is a full-body photo of a real person seen %s. ", angleText[req.Angle])
	b.WriteString("Dress this exact person in the clothing items shown in the following product images, replacing only the matching garments. ")
	b.WriteString("Preserve the person's face, identity, body shape and proportions, skin tone, hair, pose, camera angle, framing and background. ")
	fmt.Fprintf(&b, "The output must remain a single photorealistic full-body view %s, head to feet, with realistic fabric drape, fit, lighting and shadows. ", angleText[req.Angle])
	b.WriteString("Keep each item's color, pattern, material and details exactly as in its product photo. Do not add text, logos or watermarks. Do not add other people.")
	if req.Body.HeightCM != nil || req.Body.WeightKG != nil || len(req.Body.Params) > 0 {
		b.WriteString(" Body measurements for fit reference:")
		if req.Body.HeightCM != nil {
			fmt.Fprintf(&b, " height %d cm;", *req.Body.HeightCM)
		}
		if req.Body.WeightKG != nil {
			fmt.Fprintf(&b, " weight %.1f kg;", *req.Body.WeightKG)
		}
		for k, v := range req.Body.Params {
			fmt.Fprintf(&b, " %s %.1f cm;", sanitizeParam(k), v)
		}
	}
	return b.String()
}

// sanitizeParam — имя параметра тела идёт в промпт: только буквы/цифры/подчёркивание.
func sanitizeParam(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
		if b.Len() >= 32 {
			break
		}
	}
	return b.String()
}

type gemPart struct {
	Text       string     `json:"text,omitempty"`
	InlineData *gemInline `json:"inlineData,omitempty"`
}
type gemInline struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

func (g *Gemini) Generate(ctx context.Context, req Request) ([]byte, error) {
	parts := []gemPart{{Text: Prompt(req)}, {InlineData: &gemInline{"image/jpeg", b64(req.Person)}}}
	for i, it := range req.Garments {
		parts = append(parts,
			gemPart{Text: fmt.Sprintf("Product %d (%s):", i+1, zoneText[it.Zone])},
			gemPart{InlineData: &gemInline{"image/jpeg", b64(it.Image)}})
	}
	in := map[string]any{
		"contents":         []any{map[string]any{"role": "user", "parts": parts}},
		"generationConfig": map[string]any{"responseModalities": []string{"IMAGE"}},
	}
	model := g.Model
	if model == "" {
		model = "gemini-2.5-flash-image"
	}
	base := strings.TrimRight(g.BaseURL, "/")
	if base == "" {
		base = "https://generativelanguage.googleapis.com"
	}
	var out struct {
		Candidates []struct {
			Content struct {
				Parts []gemPart `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		PromptFeedback *struct {
			BlockReason string `json:"blockReason"`
		} `json:"promptFeedback"`
	}
	url := base + "/v1beta/models/" + model + ":generateContent"
	if err := postJSON(ctx, g.HTTP, http.MethodPost, url, map[string]string{"x-goog-api-key": g.APIKey}, in, &out); err != nil {
		return nil, err
	}
	if out.PromptFeedback != nil && out.PromptFeedback.BlockReason != "" {
		return nil, permanent(CodeContentBlocked, "gemini blocked: %s", out.PromptFeedback.BlockReason)
	}
	for _, c := range out.Candidates {
		for _, p := range c.Content.Parts {
			if p.InlineData != nil && p.InlineData.Data != "" {
				return decodeDataURI(p.InlineData.Data)
			}
		}
		switch c.FinishReason {
		case "SAFETY", "PROHIBITED_CONTENT", "IMAGE_SAFETY", "BLOCKLIST", "SPII":
			return nil, permanent(CodeContentBlocked, "gemini finish: %s", c.FinishReason)
		}
	}
	return nil, fmt.Errorf("gemini: no image in response")
}
