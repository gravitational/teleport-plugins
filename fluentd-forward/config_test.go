/*
Copyright 2015-2021 Gravitational, Inc.

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
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

const (
	// existingFile contains file name which exists, guaranteed
	existingFile = "config_test.go"

	// nonExistingFile contains file name of non-existing file
	nonExistingFile = "____"
)

// argAssertion is the helper struct for asserting CLI arg
type argAssertion struct {
	arg   string
	msg   string
	value string
}

// setupCleanViper sets viper config to empty
func setupCleanViper() {
	viper.Reset()
	viper.Set("start-time", defaultStartTime.Format(time.RFC3339)) // When reset, viper restes default values as well
}

// setupFluentdArgs sets args required for fluentd
func setupFluentdArgs() {
	viper.Set("fluentd-url", "http://localhost:1234")
	viper.Set("fluentd-key", existingFile)
	viper.Set("fluentd-cert", existingFile)
}

func TestFluentdArgs(t *testing.T) {
	setupCleanViper()

	var a = []argAssertion{
		{
			"fluentd-url",
			"Pass fluentd url",
			"http://localhost:8888",
		},
		{
			"fluentd-cert",
			"HTTPS must be enabled in fluentd. Please, specify fluentd TLS certificate",
			nonExistingFile,
		},
		{
			"fluentd-cert",
			"Fluentd certificate file does not exist " + nonExistingFile,
			existingFile,
		},
		{
			"fluentd-key",
			"HTTPS must be enabled in fluentd. Please, specify fluentd TLS key",
			nonExistingFile,
		},
		{
			"fluentd-key",
			"Fluentd key file does not exist " + nonExistingFile,
			existingFile,
		},
	}

	assertArgs(t, a)
}

func TestTeleportIdentity(t *testing.T) {
	setupCleanViper()
	setupFluentdArgs()

	var a = []argAssertion{
		{
			"teleport-identity",
			"Please, specify either identity file or certificates to connect to Teleport",
			existingFile,
		},
		{
			"storage",
			"Storage dir is empty, pass storage dir",
			"./tmp",
		},
	}

	assertArgs(t, a)

	_, err := newConfig()
	require.NoError(t, err)
}

func TestTeleportCerts(t *testing.T) {
	setupCleanViper()
	setupFluentdArgs()

	var a = []argAssertion{
		{
			"teleport-ca",
			"Please, specify either identity file or certificates to connect to Teleport",
			nonExistingFile,
		},
		{
			"teleport-addr",
			"Please, specify Teleport address",
			"https://localhost:4343",
		},
		{
			"teleport-ca",
			"Teleport TLS CA file does not exist ____",
			existingFile,
		},
		{
			"teleport-cert",
			"Please, provide Teleport TLS certificate",
			nonExistingFile,
		},
		{
			"teleport-cert",
			"Teleport TLS certificate file does not exist ____",
			existingFile,
		},
		{
			"teleport-key",
			"Please, provide Teleport TLS key",
			nonExistingFile,
		},
		{
			"teleport-key",
			"Teleport TLS key file does not exist ____",
			existingFile,
		},
		{
			"storage",
			"Storage dir is empty, pass storage dir",
			"./tmp",
		},
	}

	assertArgs(t, a)

	_, err := newConfig()
	require.NoError(t, err)
}

// assertArgs runs provided arg assertions
func assertArgs(t *testing.T, a []argAssertion) {
	for _, a := range a {
		_, err := newConfig()
		require.EqualError(t, err, a.msg)

		viper.Set(a.arg, a.value)
	}
}
