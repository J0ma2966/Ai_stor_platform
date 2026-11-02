// Package httpx — общие соглашения HTTP API: формат ответов и ошибок,
// декодирование и валидация тела, middleware.
package httpx

import (
	"errors"
	"net/http"
)

// Error — ошибка API. Code — стабильный машинный код (фронт переводит его на ru/uz),
// Message — человекочитаемое пояснение для разработчика.
type Error struct {
	Status  int               `json:"-"`
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func NewError(status int, code, msg string) *Error {
	return &Error{Status: status, Code: code, Message: msg}
}

var (
	ErrBadRequest   = NewError(http.StatusBadRequest, "bad_request", "malformed request")
	ErrUnauthorized = NewError(http.StatusUnauthorized, "unauthorized", "authentication required")
	ErrForbidden    = NewError(http.StatusForbidden, "forbidden", "access denied")
	ErrNotFound     = NewError(http.StatusNotFound, "not_found", "resource not found")
	ErrConflict     = NewError(http.StatusConflict, "conflict", "conflict")
	ErrTooMany      = NewError(http.StatusTooManyRequests, "rate_limited", "too many requests")
	ErrInternal     = NewError(http.StatusInternalServerError, "internal", "internal error")
	ErrUnavailable  = NewError(http.StatusServiceUnavailable, "unavailable", "service temporarily unavailable")
)

func AsError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return nil
}
