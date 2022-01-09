/*
Copyright 2021.

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

package main

import (
	"strconv"

	"github.com/gravitational/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	kzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// ZapCLI describes CLI opions of a zap logger.
type ZapCLI struct {
	ZapDevel           bool                   `kong:"help='Development Mode defaults(encoder=consoleEncoder,logLevel=Debug,stackTraceLevel=Warn). Production Mode defaults(encoder=jsonEncoder,logLevel=Info,stackTraceLevel=Error).',placeholder=true"`
	ZapEncoder         ZapCLIEncoder          `kong:"help='Zap log encoding (one of json or console).',placeholder='console'"`
	ZapLogLevel        *ZapCLILogLevel        `kong:"help='Zap Level to configure the verbosity of logging. Can be one of debug, info, error, or any integer value > 0 which corresponds to custom debug levels of increasing verbosity.',placeholder='debug'"`
	ZapStacktraceLevel *ZapCLIStacktraceLevel `kong:"help='Zap Level at and above which stacktraces are captured (one of info, error, panic).',placeholder='warn'"`
}

// ZapCLIEncoder serves a --zap-encoder CLI option.
type ZapCLIEncoder kzap.NewEncoderFunc

// ZapCLILogLevel serves a --zap-log-level CLI option.
type ZapCLILogLevel zap.AtomicLevel

// ZapCLIStacktraceLevel serves a --zap-stacktrace-level CLI option.
type ZapCLIStacktraceLevel zap.AtomicLevel

// ZapOptions converts CLI options to the options object for controller runtime.
func (cli ZapCLI) ZapOptions() *kzap.Options {
	var opts kzap.Options

	if cli.ZapEncoder != nil {
		opts.NewEncoder = kzap.NewEncoderFunc(cli.ZapEncoder)
	}

	if cli.ZapLogLevel != nil {
		opts.Level = (*zap.AtomicLevel)(cli.ZapLogLevel)
	}

	if cli.ZapStacktraceLevel != nil {
		opts.StacktraceLevel = (*zap.AtomicLevel)(cli.ZapLogLevel)
	}

	return &opts
}

// UnmarshalText returns a log encoder by its string identifier.
func (l *ZapCLIEncoder) UnmarshalText(text []byte) error {
	str := string(text)
	switch str {
	case "json":
		*l = newJSONEncoder
	case "console":
		*l = newConsoleEncoder
	default:
		return trace.BadParameter("invalid encoder value %s", str)
	}
	return nil
}

// UnmarshalText returns a log level by its string identifier.
func (l *ZapCLILogLevel) UnmarshalText(text []byte) error {
	str := string(text)
	switch str {
	case "debug":
		*l = ZapCLILogLevel(zap.NewAtomicLevelAt(zapcore.DebugLevel))
	case "info":
		*l = ZapCLILogLevel(zap.NewAtomicLevelAt(zapcore.InfoLevel))
	case "error":
		*l = ZapCLILogLevel(zap.NewAtomicLevelAt(zapcore.ErrorLevel))
	default:
		logLevel, err := strconv.Atoi(str)
		if err != nil {
			return trace.Wrap(err)
		}
		if logLevel <= 0 {
			return trace.BadParameter("invalid log level %s", str)
		}
		intLevel := -1 * logLevel
		*l = ZapCLILogLevel(zap.NewAtomicLevelAt(zapcore.Level(int8(intLevel))))
	}
	return nil
}

// UnmarshalText returns a stacktrace level by its string identifier.
func (l *ZapCLIStacktraceLevel) UnmarshalText(text []byte) error {
	str := string(text)
	switch str {
	case "info":
		*l = ZapCLIStacktraceLevel(zap.NewAtomicLevelAt(zapcore.InfoLevel))
	case "error":
		*l = ZapCLIStacktraceLevel(zap.NewAtomicLevelAt(zapcore.ErrorLevel))
	case "panic":
		*l = ZapCLIStacktraceLevel(zap.NewAtomicLevelAt(zapcore.PanicLevel))
	default:
		return trace.BadParameter("invalid stacktrace level %s", str)
	}
	return nil
}

func newJSONEncoder(opts ...kzap.EncoderConfigOption) zapcore.Encoder {
	encoderConfig := zap.NewProductionEncoderConfig()
	for _, opt := range opts {
		opt(&encoderConfig)
	}
	return zapcore.NewJSONEncoder(encoderConfig)
}

func newConsoleEncoder(opts ...kzap.EncoderConfigOption) zapcore.Encoder {
	encoderConfig := zap.NewDevelopmentEncoderConfig()
	for _, opt := range opts {
		opt(&encoderConfig)
	}
	return zapcore.NewConsoleEncoder(encoderConfig)
}
