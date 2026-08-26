package whatsapp

import (
	"fmt"
	"log/slog"

	waLog "go.mau.fi/whatsmeow/util/log"
)

type WhatsmeowLogger struct {
	logger *slog.Logger
}

var _ waLog.Logger = WhatsmeowLogger{}

func NewWhatsmeowLogger(logger *slog.Logger) WhatsmeowLogger {
	if logger == nil {
		logger = slog.Default()
	}
	return WhatsmeowLogger{logger: logger.With("component", "whatsmeow")}
}

func (l WhatsmeowLogger) Warnf(msg string, args ...any) {
	l.logger.Warn(fmt.Sprintf(msg, args...))
}

func (l WhatsmeowLogger) Errorf(msg string, args ...any) {
	l.logger.Error(fmt.Sprintf(msg, args...))
}

func (l WhatsmeowLogger) Infof(msg string, args ...any) {
	l.logger.Info(fmt.Sprintf(msg, args...))
}

func (l WhatsmeowLogger) Debugf(msg string, args ...any) {
	// WhatsMeow has no trace level; its most detailed output maps to slog debug.
	l.logger.Debug(fmt.Sprintf(msg, args...))
}

func (l WhatsmeowLogger) Sub(module string) waLog.Logger {
	return WhatsmeowLogger{logger: l.logger.With("module", module)}
}
