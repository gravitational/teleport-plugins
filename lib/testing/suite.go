package testing

import (
	"context"
	"io/ioutil"
	"os"
	"time"

	"github.com/gravitational/teleport-plugins/lib/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
	pluginCtx context.Context
	ctx       context.Context
	app       AppI
}

type AppI interface {
	Run(ctx context.Context) error
	WaitReady(ctx context.Context) (bool, error)
	Err() error
	Shutdown(ctx context.Context) error
}

func (s *Suite) SetContext(timeout time.Duration) (context.Context, context.Context) {
	t := s.T()
	t.Helper()

	require.Nil(t, s.pluginCtx, "Context cannot be set twice")

	ctx, _ := logger.WithField(context.Background(), "test", t.Name())
	// We set pluginCtx timeout slightly higher than ctx for test assertions to fall earlier than
	// plugin fails.
	pluginCtx, pluginCtxCancel := context.WithTimeout(ctx, timeout+100*time.Millisecond)
	ctx, cancel := context.WithTimeout(pluginCtx, timeout)
	t.Cleanup(func() {
		cancel()
		pluginCtxCancel()
		s.pluginCtx = nil
		s.ctx = nil
	})
	s.pluginCtx, s.ctx = pluginCtx, ctx
	return pluginCtx, ctx
}

func (s *Suite) PluginCtx() context.Context {
	if ctx := s.pluginCtx; ctx != nil {
		return ctx
	}
	ctx, _ := s.SetContext(5 * time.Second)
	return ctx
}

func (s *Suite) NewTmpFile(pattern string) *os.File {
	t := s.T()
	t.Helper()

	file, err := ioutil.TempFile("", pattern)
	require.NoError(t, err)
	t.Cleanup(func() {
		err := os.Remove(file.Name())
		require.NoError(t, err)
	})
	return file
}

func (s *Suite) Ctx() context.Context {
	t := s.T()
	t.Helper()

	if ctx := s.ctx; ctx != nil {
		return ctx
	}
	_, ctx := s.SetContext(5 * time.Second)
	return ctx
}

func (s *Suite) StartApp(app AppI) {
	t := s.T()
	t.Helper()

	require.Nil(t, s.app, "Cannot start app twice")

	ctx := s.PluginCtx()

	go func() {
		if err := app.Run(ctx); err != nil {
			panic(err)
		}
	}()

	t.Cleanup(func() {
		err := app.Shutdown(ctx)
		assert.NoError(t, err)
		assert.NoError(t, app.Err())
		s.app = nil
	})

	ok, err := app.WaitReady(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	s.app = app
}
