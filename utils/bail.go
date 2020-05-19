package utils

import (
	"os"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

// Bail exits with nonzero exit code and prints an error to a log.
func Bail(err error) {
	if agg, ok := trace.Unwrap(err).(trace.Aggregate); ok {
		for _, aggErr := range agg.Errors() {
			log.WithError(aggErr).Error("Terminating...")
		}
	} else {
		log.WithError(err).Error("Terminating...")
	}
	log.Debugf("%v", trace.DebugReport(err))
	os.Exit(1)
}
