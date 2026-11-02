package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-playground/validator/v10"
)

var validate = func() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())
	v.RegisterTagNameFunc(func(f reflectStructField) string {
		name := strings.SplitN(f.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})
	_ = v.RegisterValidation("e164", func(fl validator.FieldLevel) bool { return e164.MatchString(fl.Field().String()) })
	return v
}()

// JSON пишет ответ {"data": ...}.
func JSON(w http.ResponseWriter, status int, data any) {
	writeJSON(w, status, map[string]any{"data": data})
}

// List пишет ответ {"data": [...], "next_cursor": "..."} для курсорной пагинации.
func List(w http.ResponseWriter, items any, nextCursor string) {
	body := map[string]any{"data": items}
	if nextCursor != "" {
		body["next_cursor"] = nextCursor
	}
	writeJSON(w, http.StatusOK, body)
}

func NoContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }

// Fail пишет ошибку {"error": {...}, "request_id": "..."}.
// Неизвестные ошибки логируются целиком, а клиенту уходит только "internal".
func Fail(w http.ResponseWriter, r *http.Request, err error) {
	e := AsError(err)
	if e == nil {
		LoggerFrom(r.Context()).Error("unhandled error", slog.String("err", err.Error()))
		e = ErrInternal
	} else if e.Status >= 500 {
		LoggerFrom(r.Context()).Error("server error", slog.String("code", e.Code), slog.String("err", e.Message))
	}
	writeJSON(w, e.Status, map[string]any{"error": e, "request_id": RequestID(r.Context())})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// Decode читает JSON (не больше 1 МБ, без неизвестных полей) и валидирует по тегам `validate`.
func Decode(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return NewError(http.StatusRequestEntityTooLarge, "payload_too_large", "request body too large")
		}
		return NewError(http.StatusBadRequest, "invalid_json", sanitizeJSONErr(err))
	}
	if dec.More() {
		return NewError(http.StatusBadRequest, "invalid_json", "unexpected data after JSON body")
	}
	if _, err := io.Copy(io.Discard, r.Body); err != nil {
		return ErrBadRequest
	}
	return Validate(dst)
}

func Validate(v any) error {
	err := validate.Struct(v)
	if err == nil {
		return nil
	}
	var ves validator.ValidationErrors
	if !errors.As(err, &ves) {
		return ErrBadRequest
	}
	fields := make(map[string]string, len(ves))
	for _, fe := range ves {
		fields[fe.Field()] = fe.Tag()
	}
	return &Error{Status: http.StatusUnprocessableEntity, Code: "validation_failed", Message: "request validation failed", Fields: fields}
}

func sanitizeJSONErr(err error) string {
	var se *json.SyntaxError
	var te *json.UnmarshalTypeError
	switch {
	case errors.As(err, &se):
		return fmt.Sprintf("syntax error at offset %d", se.Offset)
	case errors.As(err, &te):
		return fmt.Sprintf("field %q has wrong type", te.Field)
	case strings.HasPrefix(err.Error(), "json: unknown field"):
		return err.Error()[len("json: "):]
	case errors.Is(err, io.EOF):
		return "empty body"
	}
	return "invalid JSON"
}
