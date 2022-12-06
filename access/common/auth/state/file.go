package state

import (
	"context"
	"encoding/json"
	"os"

	"github.com/gravitational/trace"
)

// NB: racy, does not use file-locking or similar
type fileState struct {
	filename string
}

func NewFileState(filename string) (State, error) {
	return &fileState{filename: filename}, nil
}

func (f *fileState) GetCredentials(_ context.Context) (*Credentials, error) {
	payload, err := os.ReadFile(f.filename)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	var creds Credentials
	err = json.Unmarshal(payload, &creds)
	if err != nil {
		return nil, trace.Wrap(err)
	} else if creds.AccessToken == "" {
		return nil, trace.NotFound("state does not contain `AccessToken`")
	} else if creds.RefreshToken == "" {
		return nil, trace.NotFound("state does not contain `RefreshToken`")
	} else if creds.ExpiresAt.IsZero() {
		return nil, trace.NotFound("state does not contain `ExpiresAt`")
	}

	return &creds, nil
}

func (f *fileState) PutCredentials(_ context.Context, creds *Credentials) error {
	payload, err := json.Marshal(&creds)

	if err != nil {
		return trace.Wrap(err)
	}

	return trace.Wrap(os.WriteFile(f.filename, payload, 0600))
}
