package lib

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
)

type Terminable interface {
	// Shutdown attempts to gracefully terminate.
	Shutdown(context.Context) error
	// Close does a fast (force) termination.
	Close()
}

func ServeSignals(app Terminable, shutdownTimeout time.Duration) {
	ctx := context.Background()
	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC,
		syscall.SIGTERM, // graceful shutdown
		syscall.SIGINT,  // graceful-then-fast shutdown
	)
	defer signal.Stop(sigC)

	gracefulShutdown := func() {
		tctx, tcancel := context.WithTimeout(ctx, shutdownTimeout)
		defer tcancel()
		log.Infof("Attempting graceful shutdown...")
		if err := app.Shutdown(tctx); err != nil {
			log.Infof("Graceful shutdown failed. Trying fast shutdown...")
			app.Close()
		}
	}
	var alreadyInterrupted bool
	for {
		signal := <-sigC
		switch signal {
		case syscall.SIGTERM:
			gracefulShutdown()
			return
		case syscall.SIGINT:
			if alreadyInterrupted {
				app.Close()
				return
			}
			go gracefulShutdown()
			alreadyInterrupted = true
		}
	}
}
