package main

import (
	"os"
	"path"
	"testing"
	"time"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/require"
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
				IngestConfig: IngestConfig{
					StorageDir:          "./storage",
					BatchSize:           20,
					SkipSessionTypesRaw: []string{"print"},
					SkipSessionTypes: map[string]struct{}{
						"print": {},
					},
					Timeout:     10 * time.Second,
					Concurrency: 5,
				},
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
