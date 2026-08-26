package whatsapp

import (
	"io"
	"log/slog"
)

func testSlog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
