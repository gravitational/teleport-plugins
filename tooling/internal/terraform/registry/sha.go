package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/gravitational/trace"
)

// Sha256Sum generates the equivalent contents of running `sha256sum` om some
// data and writes it to the supplied stream, returning the sha bytes.
func sha256Sum(dst io.Writer, filename string, data []byte) ([]byte, error) {
	sha := sha256.Sum256(data)
	_, err := fmt.Fprintf(dst, "%s  %s\n", hex.EncodeToString(sha[:]), filename)
	return sha[:], trace.Wrap(err, "failed generating sum file")
}
