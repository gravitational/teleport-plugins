package filename

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseFilename(t *testing.T) {
	t.Run("WithLeadingPath", func(t *testing.T) {
		info, err := Parse("/some/bath/to/file/terraform-provider-teleport-v7.0.0-darwin-amd64-bin.tar.gz")
		require.NoError(t, err)

		require.Equal(t, "terraform-provider", info.Type)
		require.Equal(t, "7.0.0", info.Version)
		require.Equal(t, "darwin", info.OS)
		require.Equal(t, "amd64", info.Arch)
	})

	t.Run("WithoutLeadingPath", func(t *testing.T) {
		info, err := Parse("terraform-provider-teleport-v1.2.3-linux-arm-bin.tar.gz")
		require.NoError(t, err)
		require.Equal(t, "terraform-provider", info.Type)
		require.Equal(t, "1.2.3", info.Version)
		require.Equal(t, "linux", info.OS)
		require.Equal(t, "arm", info.Arch)
	})

	t.Run("Random Junk", func(t *testing.T) {
		_, err := Parse("blahblahblah")
		require.Error(t, err)
	})
}

func TestGenerateFilename(t *testing.T) {
	info := Info{Type: "some-plugin", Version: "1.2.3", OS: "darwin", Arch: "amd64"}
	fn := info.Filename(".banana")
	require.Equal(t, "some-plugin-teleport-v1.2.3-darwin-amd64-bin.banana", fn)
}
