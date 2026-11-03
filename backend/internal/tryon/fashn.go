package tryon

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Fashn — FASHN AI (https://docs.fashn.ai), модель tryon-v1.6. Поддерживает верх и
// низ (tops/bottoms); головные уборы и обувь модель не переодевает — они остаются
// как на фото клиента (UI предупреждает). Несколько вещей — цепочкой: сначала низ,
// потом верх. return_base64=true: FASHN хранит такой результат не дольше 60 минут
// и не держит его в истории запросов (минимизация передачи биометрии).
type Fashn struct {
	APIKey  string
	BaseURL string // по умолчанию https://api.fashn.ai
	Mode    string // performance | balanced | quality
	HTTP    *http.Client
	Poll    time.Duration
}

func (f *Fashn) Name() string { return "fashn" }

var fashnCategory = map[string]string{ZoneTop: "tops", ZoneBottom: "bottoms"}

func (f *Fashn) Generate(ctx context.Context, req Request) ([]byte, error) {
	var chain []Garment
	for _, z := range []string{ZoneBottom, ZoneTop} {
		for _, g := range req.Garments {
			if g.Zone == z {
				chain = append(chain, g)
			}
		}
	}
	if len(chain) == 0 {
		return nil, permanent(CodeUnsupported, "fashn supports only tops and bottoms")
	}
	person := dataURI(req.Person)
	var out []byte
	for _, g := range chain {
		res, err := f.run(ctx, person, g)
		if err != nil {
			return nil, err
		}
		out = res
		person = dataURI(res)
		if strings.HasPrefix(http.DetectContentType(res), "image/png") {
			person = "data:image/png;base64," + strings.TrimPrefix(dataURI(res), "data:image/jpeg;base64,")
		}
	}
	return out, nil
}

type fashnStatus struct {
	ID     string   `json:"id"`
	Status string   `json:"status"`
	Output []string `json:"output"`
	Error  *struct {
		Name    string `json:"name"`
		Message string `json:"message"`
	} `json:"error"`
}

func (f *Fashn) base() string {
	if f.BaseURL != "" {
		return strings.TrimRight(f.BaseURL, "/")
	}
	return "https://api.fashn.ai"
}

func (f *Fashn) run(ctx context.Context, person string, g Garment) ([]byte, error) {
	hdr := map[string]string{"Authorization": "Bearer " + f.APIKey}
	mode := f.Mode
	if mode == "" {
		mode = "balanced"
	}
	in := map[string]any{
		"model_name": "tryon-v1.6",
		"inputs": map[string]any{
			"model_image":      person,
			"garment_image":    dataURI(g.Image),
			"category":         fashnCategory[g.Zone],
			"mode":             mode,
			"output_format":    "jpeg",
			"return_base64":    true,
			"moderation_level": "conservative",
		},
	}
	var started fashnStatus
	if err := postJSON(ctx, f.HTTP, http.MethodPost, f.base()+"/v1/run", hdr, in, &started); err != nil {
		return nil, err
	}
	if started.Error != nil {
		return nil, fashnErr(started.Error.Name, started.Error.Message)
	}
	if started.ID == "" {
		return nil, fmt.Errorf("fashn: empty prediction id")
	}
	poll := f.Poll
	if poll == 0 {
		poll = 2 * time.Second
	}
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(poll):
		}
		var st fashnStatus
		if err := postJSON(ctx, f.HTTP, http.MethodGet, f.base()+"/v1/status/"+started.ID, hdr, nil, &st); err != nil {
			if IsPermanent(err) {
				return nil, err
			}
			continue // сетевой сбой опроса — пробуем ещё, общий таймаут ограничивает
		}
		switch st.Status {
		case "completed":
			if len(st.Output) == 0 {
				return nil, fmt.Errorf("fashn: completed without output")
			}
			return decodeDataURI(st.Output[0])
		case "failed", "canceled", "cancelled":
			if st.Error != nil {
				return nil, fashnErr(st.Error.Name, st.Error.Message)
			}
			return nil, fmt.Errorf("fashn: prediction %s", st.Status)
		}
	}
}

func fashnErr(name, msg string) error {
	switch name {
	case "PoseError", "ImageLoadError":
		return permanent(CodePhotoRejected, "fashn %s: %s", name, msg)
	case "ContentModerationError", "InputValidationError":
		return permanent(CodeContentBlocked, "fashn %s: %s", name, msg)
	}
	return fmt.Errorf("fashn %s: %s", name, msg) // PipelineError, ThirdPartyError и т.п. — повтор
}
