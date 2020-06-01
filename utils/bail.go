package utils

import (
	"os"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

func LogError(err error, msg string) {
	log.WithError(err).Error(msg)
	log.Debugf("%v", trace.DebugReport(err))
}

// Bail exits with nonzero exit code and prints an error to a log.
func Bail(err error) {
	if agg, ok := trace.Unwrap(err).(trace.Aggregate); ok {
		for _, err := range agg.Errors() {
			LogError(err, "Terminating due to error")
		}
	} else {
		LogError(err, "Terminating due to error")
	}
	os.Exit(1)
}
