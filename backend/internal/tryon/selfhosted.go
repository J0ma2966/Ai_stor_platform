package tryon

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// SelfHosted — собственный сервер инференса (например, open-source модель на GPU в
// дата-центре в Узбекистане — тогда биометрия вообще не покидает страну).
// Контракт (docs/TRYON.md):
//
//	POST <TRYON_ENDPOINT>  Authorization: Bearer <TRYON_API_KEY>
//	{"angle":"front","gender":"female","body":{...},"person_image":"<base64 jpeg>",
//	 "garments":[{"zone":"top","set":false,"title":"...","brand":"...","image":"<base64 jpeg>"}]}
//	set=true — «двойка»: верх и низ на одном фото (зона top), низа отдельно нет.
//	200 → тело — картинка (image/jpeg или image/png)
//	422 {"code":"photo_rejected|content_blocked|unsupported_items"} → без повтора
//	429/5xx → повтор
type SelfHosted struct {
	Endpoint string
	APIKey   string
	HTTP     *http.Client
}

func (s *SelfHosted) Name() string { return "http" }

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

func (s *SelfHosted) Generate(ctx context.Context, req Request) ([]byte, error) {
	type garment struct {
		Zone  string `json:"zone"`
		Set   bool   `json:"set"`
		Title string `json:"title"`
		Brand string `json:"brand"`
		Image string `json:"image"`
	}
	in := struct {
		Angle    string    `json:"angle"`
		Gender   string    `json:"gender,omitempty"`
		Body     Body      `json:"body"`
		Person   string    `json:"person_image"`
		Garments []garment `json:"garments"`
	}{Angle: req.Angle, Gender: req.Gender, Body: req.Body, Person: b64(req.Person)}
	for _, g := range req.Garments {
		in.Garments = append(in.Garments, garment{g.Zone, g.Set, g.Title, g.Brand, b64(g.Image)})
	}
	buf, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Endpoint, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	hr.Header.Set("Content-Type", "application/json")
	if s.APIKey != "" {
		hr.Header.Set("Authorization", "Bearer "+s.APIKey)
	}
	res, err := s.HTTP.Do(hr)
	if err != nil {
		return nil, fmt.Errorf("tryon server: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponse))
	if err != nil {
		return nil, err
	}
	if res.StatusCode == http.StatusUnprocessableEntity {
		var e struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(raw, &e)
		switch e.Code {
		case CodePhotoRejected, CodeContentBlocked, CodeUnsupported:
			return nil, permanent(e.Code, "tryon server rejected input")
		}
		return nil, permanent(CodeProviderFailure, "tryon server 422")
	}
	if err := statusErr(res.StatusCode, raw); err != nil {
		return nil, err
	}
	if ct := http.DetectContentType(raw); !strings.HasPrefix(ct, "image/") {
		return nil, fmt.Errorf("tryon server returned %s", ct)
	}
	return raw, nil
}
