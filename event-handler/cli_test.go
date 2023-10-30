package main

import (
	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/require"
	"os"
	"path"
	"testing"
	"time"
)

// StartCmdConfig is mostly to test that the TOML file parsing works as
// expected.
func TestStartCmdConfig(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)

	testCases := []struct {
		name string
		args []string

		want StartCmdConfig
	}{
		{
			name: "standard",
			args: []string{"start", "--config", "testdata/config.toml"},
			want: StartCmdConfig{
				FluentdConfig: FluentdConfig{
					FluentdURL:        "https://localhost:8888/test.log",
					FluentdSessionURL: "https://localhost:8888/session",
					FluentdCert:       path.Join(wd, "testdata", "fake-file"),
					FluentdKey:        path.Join(wd, "testdata", "fake-file"),
					FluentdCA:         path.Join(wd, "testdata", "fake-file"),
				},
				TeleportConfig: TeleportConfig{
					TeleportAddr:            "localhost:3025",
					TeleportIdentityFile:    path.Join(wd, "testdata", "fake-file"),
					TeleportRefreshEnabled:  true,
					TeleportRefreshInterval: 2 * time.Minute,
				},
				IngestConfig: IngestConfig{},
				LockConfig: LockConfig{
					LockFailedAttemptsCount: 3,
					LockPeriod:              time.Minute,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cli := CLI{}
			parser, err := kong.New(
				&cli,
				kong.UsageOnError(),
				kong.Configuration(KongTOMLResolver),
				kong.Name(pluginName),
				kong.Description(pluginDescription),
			)
			require.NoError(t, err)
			_, err = parser.Parse(tc.args)
			require.NoError(t, err)

			require.Equal(t, tc.want, cli.Start)
		})
	}
}
