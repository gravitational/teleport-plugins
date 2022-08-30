package main

import (
	"context"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

func uninstall(ctx context.Context, configPath string) error {
	b, c, err := loadConfig(configPath)
	if err != nil {
		return trace.Wrap(err)
	}
	err = checkApp(ctx, b)
	if err != nil {
		return trace.Wrap(err)
	}

	var errs []error
	for _, userID := range c.Recipients.GetAllRecipients() {
		errs = append(errs, b.UninstallAppForUser(ctx, userID))
	}
	err = trace.NewAggregate(errs...)
	if err != nil {
		log.Errorln("The following error(s) happened when uninstalling the Teams App:")
		return err
	}
	log.Info("Successfully uninstalled app for all recipients")
	return nil
}
