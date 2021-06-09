/*
Copyright 2021 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package prompt implements CLI prompts to the user.
package prompt

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/gravitational/trace"
)

// Confirmation prompts the user for a yes/no confirmation for question.
// The prompt is written to out and the answer is read from in.
//
// question should be a plain sentece without "[yes/no]"-type hints at the end.
//
// ctx can be canceled to abort the prompt.
func Confirmation(ctx context.Context, out io.Writer, in *ContextReader, question string) (bool, error) {
	fmt.Fprintf(out, "%s [y/N]: ", question)
	answer, err := in.ReadContext(ctx)
	if err != nil {
		return false, trace.WrapWithMessage(err, "failed reading prompt response")
	}
	switch strings.ToLower(strings.TrimSpace(string(answer))) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

// PickOne prompts the user to pick one of the provided string options.
// The prompt is written to out and the answer is read from in.
//
// question should be a plain sentece without the list of provided options.
//
// ctx can be canceled to abort the prompt.
func PickOne(ctx context.Context, out io.Writer, in *ContextReader, question string, options []string) (string, error) {
	fmt.Fprintf(out, "%s [%s]: ", question, strings.Join(options, ", "))
	answerOrig, err := in.ReadContext(ctx)
	if err != nil {
		return "", trace.WrapWithMessage(err, "failed reading prompt response")
	}
	answer := strings.ToLower(strings.TrimSpace(string(answerOrig)))
	for _, opt := range options {
		if strings.ToLower(opt) == answer {
			return opt, nil
		}
	}
	return "", trace.BadParameter("%q is not a valid option, please specify one of [%s]", answerOrig, strings.Join(options, ", "))
}

// Input prompts the user for freeform text input.
// The prompt is written to out and the answer is read from in.
//
// ctx can be canceled to abort the prompt.
func Input(ctx context.Context, out io.Writer, in *ContextReader, question string) (string, error) {
	fmt.Fprintf(out, "%s: ", question)
	answer, err := in.ReadContext(ctx)
	if err != nil {
		return "", trace.WrapWithMessage(err, "failed reading prompt response")
	}
	return strings.TrimSpace(string(answer)), nil
}
