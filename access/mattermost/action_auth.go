package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"

	"github.com/gravitational/trace"
)

type ActionAuth struct {
	secret string
}

func (auth *ActionAuth) Sign(action, reqID string) ([]byte, error) {
	data := fmt.Sprintf("%s/%s", action, reqID)
	mac := hmac.New(sha256.New, []byte(auth.secret))
	_, err := mac.Write([]byte(data))
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return mac.Sum(nil), nil
}

func (auth *ActionAuth) Verify(action, reqID string, signature []byte) (bool, error) {
	validSignature, err := auth.Sign(action, reqID)
	if err != nil {
		return false, trace.Wrap(err)
	}
	return hmac.Equal(signature, validSignature), nil
}
