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

// maxResponse — защита памяти от огромного ответа провайдера.
const maxResponse = 48 << 20

// postJSON отправляет JSON и декодирует JSON-ответ. Ответ 4xx (кроме 408/429) —
// постоянная ошибка с кодом permCode; 5xx/408/429/сеть — временная (повтор).
func postJSON(ctx context.Context, c *http.Client, method, url string, hdr map[string]string, in, out any) error {
	var body io.Reader
	if in != nil {
		buf, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("provider request: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponse))
	if err != nil {
		return fmt.Errorf("provider read: %w", err)
	}
	if err := statusErr(res.StatusCode, raw); err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("provider json: %w", err)
	}
	return nil
}

func statusErr(code int, raw []byte) error {
	if code < 300 {
		return nil
	}
	snippet := strings.TrimSpace(string(raw[:min(len(raw), 300)]))
	if code >= 400 && code < 500 && code != http.StatusRequestTimeout && code != http.StatusTooManyRequests {
		// 401/403 — неверный ключ: не вина клиента, но повтор не поможет.
		if code == http.StatusUnauthorized || code == http.StatusForbidden {
			return permanent(CodeProviderFailure, "provider auth failed (%d)", code)
		}
		return permanent(CodeProviderFailure, "provider rejected request (%d): %s", code, snippet)
	}
	return fmt.Errorf("provider status %d: %s", code, snippet)
}

func dataURI(jpeg []byte) string {
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(jpeg)
}

// decodeDataURI принимает data:...;base64,XXX или чистый base64.
func decodeDataURI(s string) ([]byte, error) {
	if i := strings.Index(s, ","); strings.HasPrefix(s, "data:") && i > 0 {
		s = s[i+1:]
	}
	return base64.StdEncoding.DecodeString(s)
}
