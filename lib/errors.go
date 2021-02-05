package lib

import (
	"context"
	"io"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/gravitational/trace"
	"github.com/gravitational/trace/trail"
)

// TODO: remove this when trail.FromGRPC will understand additional error codes
func FromGRPC(err error) error {
	switch {
	case err == io.EOF:
		fallthrough
	case status.Code(err) == codes.Canceled, err == context.Canceled:
		fallthrough
	case status.Code(err) == codes.DeadlineExceeded, err == context.DeadlineExceeded:
		return trace.Wrap(err)
	default:
		return trail.FromGRPC(err)
	}
}

// TODO: remove this when trail.FromGRPC will understand additional error codes
func IsCanceled(err error) bool {
	err = trace.Unwrap(err)
	return err == context.Canceled || status.Code(err) == codes.Canceled
}

// TODO: remove this when trail.FromGRPC will understand additional error codes
func IsDeadline(err error) bool {
	err = trace.Unwrap(err)
	return err == context.DeadlineExceeded || status.Code(err) == codes.DeadlineExceeded
}
