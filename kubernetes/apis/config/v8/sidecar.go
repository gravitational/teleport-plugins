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

package v8

import (
	"github.com/gravitational/teleport-plugins/lib"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	compcfg "k8s.io/component-base/config/v1alpha1"
	cfg "sigs.k8s.io/controller-runtime/pkg/config/v1alpha1"
)

//+kubebuilder:object:root=true

// SidecarConfig is the Schema for the sidecar configuration file.
type SidecarConfig struct {
	metav1.TypeMeta `json:",inline"`

	// ControllerManagerConfigurationSpec returns the contfigurations for controllers
	cfg.ControllerManagerConfigurationSpec `json:",inline"`

	// Teleport specifies how to connect to the local Teleport instance.
	Teleport SidecarConfigTeleport `json:"teleport"`

	// Scope specifies the set of reconcilable resources.
	Scope SidecarConfigScope `json:"scope"`
}

// SidecarConfigTeleport describes how to locate the running Teleport server.
type SidecarConfigTeleport struct {
	// Config is a path where Teleport configuration file located e.g. /etc/teleport.yaml.
	Config string `json:"config"`

	// Addr is an address of the Teleport auth server.
	Addr string `json:"addr"`

	// Role is a Teleport role with a permission to manage Teleport resources.
	Role string `json:"role"`

	// User is a Teleport user used to access Teleport Auth server.
	User string `json:"user"`
}

// SidecarConfigScope applies some filters on the object being reconciled.
type SidecarConfigScope struct {
	// Namespace restricts the namespace of the objects considered.
	// +required
	Namespace string `json:"namespace"`
}

func init() {
	schemeBuilder.Register(&SidecarConfig{})
}

// DefaultSidecarConfig returns a config object with default fields.
func DefaultSidecarConfig() SidecarConfig {
	var obj SidecarConfig

	// Set up controller manager.
	obj.ControllerManagerConfigurationSpec.Metrics = cfg.ControllerMetrics{BindAddress: ":8080"}
	obj.Health = cfg.ControllerHealth{HealthProbeBindAddress: ":8081"}
	leaderElect := true
	obj.ControllerManagerConfigurationSpec.LeaderElection = &compcfg.LeaderElectionConfiguration{
		LeaderElect:  &leaderElect,
		ResourceName: "operator.teleport.dev",
	}
	webhookPort := 9443
	obj.ControllerManagerConfigurationSpec.Webhook = cfg.ControllerWebhook{Port: &webhookPort}

	// Set up Teleport defaults
	obj.Teleport.Config = lib.DefaultConfigPath
	obj.Teleport.Addr = lib.DefaultLocalAddr
	obj.Teleport.Role = "kubernetes-sidecar"
	obj.Teleport.User = "kubernetes-sidecar"

	return obj
}
