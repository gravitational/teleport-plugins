package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	// cursorConfig represents required config
	cursorConfig = &Config{
		StorageDir:   "tmp",
		TeleportAddr: "https://localhost:8888",
	}
)

func TestCursor(t *testing.T) {
	cursor, err := NewCursor(cursorConfig)
	require.NoError(t, err)

	v1, err := cursor.Get()
	require.NoError(t, err)
	require.Equal(t, "", v1)

	cursor.Set("test")

	cursor, err = NewCursor(cursorConfig)
	require.NoError(t, err)

	v2, err := cursor.Get()
	require.NoError(t, err)
	require.Equal(t, "test", v2)
}
