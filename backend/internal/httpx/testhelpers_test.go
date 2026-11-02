package httpx

import (
	"io"
	"log/slog"
)

func nil_logger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
