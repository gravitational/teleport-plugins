package utils

import (
	"context"
	"os"
	"strings"

	"github.com/gravitational/trace"

	log "github.com/sirupsen/logrus"
)

type LogConfig struct {
	Output   string `toml:"output"`
	Severity string `toml:"severity"`
}

type loggerKey struct{}

// InitLogger sets up logger for a typical daemon scenario until configuration
// file is parsed
func InitLogger() {
	log.SetFormatter(&trace.TextFormatter{
		DisableTimestamp: true,
		EnableColors:     trace.IsTerminal(os.Stderr),
		ComponentPadding: 1, // We don't use components so strip the padding
	})
	log.SetOutput(os.Stderr)
}

func SetupLogger(conf LogConfig) error {
	switch conf.Output {
	case "stderr", "error", "2":
		log.SetOutput(os.Stderr)
	case "stdout", "out", "1":
		log.SetOutput(os.Stdout)
	default:
		// assume it's a file path:
		logFile, err := os.Create(conf.Output)
		if err != nil {
			return trace.Wrap(err, "failed to create the log file")
		}
		log.SetOutput(logFile)
	}

	switch strings.ToLower(conf.Severity) {
	case "info":
		log.SetLevel(log.InfoLevel)
	case "err", "error":
		log.SetLevel(log.ErrorLevel)
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "warn", "warning":
		log.SetLevel(log.WarnLevel)
	default:
		return trace.BadParameter("unsupported logger severity: '%v'", conf.Severity)
	}

	return nil
}

func WithLogger(ctx context.Context, logger *log.Entry) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

func WithLogField(ctx context.Context, key string, value interface{}) (context.Context, *log.Entry) {
	logger := GetLogger(ctx).WithField(key, value)
	return context.WithValue(ctx, loggerKey{}, logger), logger
}

func WithLogFields(ctx context.Context, logFields log.Fields) (context.Context, *log.Entry) {
	logger := GetLogger(ctx).WithFields(logFields)
	return context.WithValue(ctx, loggerKey{}, logger), logger
}

func GetLogger(ctx context.Context) *log.Entry {
	if logger, ok := ctx.Value(loggerKey{}).(*log.Entry); ok && logger != nil {
		return logger
	}

	return log.NewEntry(log.StandardLogger())
}
