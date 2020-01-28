package utils

import (
	"github.com/gravitational/teleport/lib/utils"

	log "github.com/sirupsen/logrus"
)

// InitLogger sets up logger for a typical daemon scenario until configuration
// file is parsed
func InitLogger() {
	utils.InitLogger(utils.LoggingForDaemon, log.ErrorLevel)
}
