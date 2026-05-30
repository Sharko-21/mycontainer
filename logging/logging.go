package logging

import (
	"fmt"
	"log/slog"
	"os"
)

type Logger interface {
	SetLogLevel(level slog.Level)
	Info(msg string, args ...any)
	Debug(msg string, args ...any)
	Debugf(msg string, args ...any)
}

func NewLogger() Logger {
	logLevel := new(slog.LevelVar)
	return &logger{
		slogger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: logLevel,
		})),
		logLevel: logLevel,
	}
}

type logger struct {
	slogger  *slog.Logger
	logLevel *slog.LevelVar
}

func (l *logger) SetLogLevel(level slog.Level) {
	l.logLevel.Set(level)
}

func (l *logger) Info(msg string, args ...any) {
	l.slogger.Info(msg, args...)
}

func (l *logger) Debug(msg string, args ...any) {
	l.slogger.Debug(msg, args...)
}

func (l *logger) Debugf(msg string, args ...any) {
	l.slogger.Debug(fmt.Sprintf(msg, args...))
}
