package logging

import (
	"log/slog"
	"os"
	"strings"
	"time"
)

const timeFormat = "2006-01-02T15:04:05.000Z"

func ConfigureDefaultLogger(formatStr string, levelStr string, includeSource bool) *slog.Logger {

	logLevel := levelStringToLevelValue(levelStr)
	logHandlerOpts := slog.HandlerOptions{
		AddSource: includeSource,
		Level:     logLevel,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {

			// We want to use the 'message' field, not the default 'msg'
			if len(groups) == 0 && a.Key == slog.MessageKey {
				return slog.Attr{
					Key:   "message",
					Value: a.Value,
				}
			}

			// We want UTC time format, not the default. parse the default and reprint it
			// We want to use the 'message' field, not the default 'msg'
			if len(groups) == 0 && a.Key == slog.TimeKey {
				if t, ok := a.Value.Any().(time.Time); ok {
					return slog.String(a.Key, t.Format(timeFormat))
				}
			}

			return a
		},
	} // this affects new loggers
	slog.SetLogLoggerLevel(logLevel) // affects system-default logger and old log package logging

	var logger *slog.Logger = nil
	switch strings.ToLower(formatStr) {
	case "json":
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &logHandlerOpts))
	case "text":
		logger =
			slog.New(slog.NewTextHandler(os.Stdout, &logHandlerOpts))
	case "system-default", "":
		logger = slog.Default()
	default:
		panic("Unexpected log format: " + formatStr)
	}

	if logger != nil {
		slog.SetDefault(logger)
	}

	return logger
}

func levelStringToLevelValue(levelStr string) slog.Level {
	switch strings.ToUpper(levelStr) {
	case "INFO":
		return slog.LevelInfo
	case "DEBUG":
		return slog.LevelDebug
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		panic("Unknown log level value: " + levelStr)
	}
}
