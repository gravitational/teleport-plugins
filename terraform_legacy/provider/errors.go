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

package provider

import (
	"github.com/gravitational/trace"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	log "github.com/sirupsen/logrus"
)

// describeErr wraps error with additional information
func describeErr(err error, r string) error {
	var notFoundErr = "Terraform user has no rights to perform this action. Check that Terraform user role has " +
		"['list','create','read','update','delete'] verbs for '" + r + "' resource."
	var accessDeniedErr = "Terraform user is missing on the Teleport side. Check that your auth credentials (certs) " +
		"specified in provider configuration belong to existing user and are not expired."

	if trace.IsNotFound(err) {
		return trace.WrapWithMessage(err, notFoundErr)
	}

	if trace.IsAccessDenied(err) {
		return trace.WrapWithMessage(err, accessDeniedErr)
	}

	return trace.Wrap(err)
}

// diagFromErr converts error to diag.Diagnostics. If logging level is debug, provides trace.DebugReport instead of short text.
func diagFromErr(err error) diag.Diagnostics {
	if log.GetLevel() >= log.DebugLevel {
		return []diag.Diagnostic{{
			Severity: diag.Error,
			Summary:  err.Error(),
			Detail:   trace.DebugReport(err),
		}}
	}

	return diag.FromErr(err)
}
