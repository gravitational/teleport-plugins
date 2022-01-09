package lib

import (
	"os"

	log "github.com/sirupsen/logrus"
)

// Bail exits with nonzero exit code and prints an error to a log.
func Bail(err error) {
	log.WithError(err).Error("Terminating by fatal failure")
	os.Exit(1)
}
