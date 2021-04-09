/*
Copyright 2020 Gravitational, Inc.

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

package types

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gravitational/teleport/api/utils"

	"github.com/gogo/protobuf/proto"
	"github.com/gravitational/trace"
)

// Server represents a Node, Proxy or Auth server in a Teleport cluster
type Server interface {
	// Resource provides common resource headers
	Resource
	// GetTeleportVersion returns the teleport version the server is running on
	GetTeleportVersion() string
	// GetAddr return server address
	GetAddr() string
	// GetHostname returns server hostname
	GetHostname() string
	// GetNamespace returns server namespace
	GetNamespace() string
	// GetAllLabels returns server's static and dynamic label values merged together
	GetAllLabels() map[string]string
	// GetLabels returns server's static label key pairs
	GetLabels() map[string]string
	// GetCmdLabels gets command labels
	GetCmdLabels() map[string]CommandLabel
	// SetCmdLabels sets command labels.
	SetCmdLabels(cmdLabels map[string]CommandLabel)
	// GetPublicAddr is an optional field that returns the public address this cluster can be reached at.
	GetPublicAddr() string
	// GetRotation gets the state of certificate authority rotation.
	GetRotation() Rotation
	// SetRotation sets the state of certificate authority rotation.
	SetRotation(Rotation)
	// GetUseTunnel gets if a reverse tunnel should be used to connect to this node.
	GetUseTunnel() bool
	// SetUseTunnel sets if a reverse tunnel should be used to connect to this node.
	SetUseTunnel(bool)
	// String returns string representation of the server
	String() string
	// SetAddr sets server address
	SetAddr(addr string)
	// SetPublicAddr sets the public address this cluster can be reached at.
	SetPublicAddr(string)
	// SetNamespace sets server namespace
	SetNamespace(namespace string)
	// GetApps gets the list of applications this server is proxying.
	GetApps() []*App
	// GetApps gets the list of applications this server is proxying.
	SetApps([]*App)
	// GetKubeClusters returns the kubernetes clusters directly handled by this
	// server.
	GetKubernetesClusters() []*KubernetesCluster
	// SetKubeClusters sets the kubernetes clusters handled by this server.
	SetKubernetesClusters([]*KubernetesCluster)
	// MatchAgainst takes a map of labels and returns True if this server
	// has ALL of them
	//
	// Any server matches against an empty label set
	MatchAgainst(labels map[string]string) bool
	// LabelsString returns a comma separated string with all node's labels
	LabelsString() string
	// CheckAndSetDefaults checks and set default values for any missing fields.
	CheckAndSetDefaults() error

	// DeepCopy creates a clone of this server value
	DeepCopy() Server
}

// GetVersion returns resource version
func (s *ServerV2) GetVersion() string {
	return s.Version
}

// GetTeleportVersion returns the teleport version the server is running on
func (s *ServerV2) GetTeleportVersion() string {
	return s.Spec.Version
}

// GetKind returns resource kind
func (s *ServerV2) GetKind() string {
	return s.Kind
}

// GetSubKind returns resource sub kind
func (s *ServerV2) GetSubKind() string {
	return s.SubKind
}

// SetSubKind sets resource subkind
func (s *ServerV2) SetSubKind(sk string) {
	s.SubKind = sk
}

// GetResourceID returns resource ID
func (s *ServerV2) GetResourceID() int64 {
	return s.Metadata.ID
}

// SetResourceID sets resource ID
func (s *ServerV2) SetResourceID(id int64) {
	s.Metadata.ID = id
}

// GetMetadata returns metadata
func (s *ServerV2) GetMetadata() Metadata {
	return s.Metadata
}

// SetNamespace sets server namespace
func (s *ServerV2) SetNamespace(namespace string) {
	s.Metadata.Namespace = namespace
}

// SetAddr sets server address
func (s *ServerV2) SetAddr(addr string) {
	s.Spec.Addr = addr
}

// SetExpiry sets expiry time for the object
func (s *ServerV2) SetExpiry(expires time.Time) {
	s.Metadata.SetExpiry(expires)
}

// Expiry returns object expiry setting
func (s *ServerV2) Expiry() time.Time {
	return s.Metadata.Expiry()
}

// SetTTL sets Expires header using the provided clock.
// Use SetExpiry instead.
// DELETE IN 7.0.0
func (s *ServerV2) SetTTL(clock Clock, ttl time.Duration) {
	s.Metadata.SetTTL(clock, ttl)
}

// SetPublicAddr sets the public address this cluster can be reached at.
func (s *ServerV2) SetPublicAddr(addr string) {
	s.Spec.PublicAddr = addr
}

// GetName returns server name
func (s *ServerV2) GetName() string {
	return s.Metadata.Name
}

// SetName sets the name of the TrustedCluster.
func (s *ServerV2) SetName(e string) {
	s.Metadata.Name = e
}

// GetAddr return server address
func (s *ServerV2) GetAddr() string {
	return s.Spec.Addr
}

// GetPublicAddr is an optional field that returns the public address this cluster can be reached at.
func (s *ServerV2) GetPublicAddr() string {
	return s.Spec.PublicAddr
}

// GetRotation gets the state of certificate authority rotation.
func (s *ServerV2) GetRotation() Rotation {
	return s.Spec.Rotation
}

// SetRotation sets the state of certificate authority rotation.
func (s *ServerV2) SetRotation(r Rotation) {
	s.Spec.Rotation = r
}

// GetUseTunnel gets if a reverse tunnel should be used to connect to this node.
func (s *ServerV2) GetUseTunnel() bool {
	return s.Spec.UseTunnel
}

// SetUseTunnel sets if a reverse tunnel should be used to connect to this node.
func (s *ServerV2) SetUseTunnel(useTunnel bool) {
	s.Spec.UseTunnel = useTunnel
}

// GetHostname returns server hostname
func (s *ServerV2) GetHostname() string {
	return s.Spec.Hostname
}

// GetLabels returns server's static label key pairs
func (s *ServerV2) GetLabels() map[string]string {
	return s.Metadata.Labels
}

// GetCmdLabels returns command labels
func (s *ServerV2) GetCmdLabels() map[string]CommandLabel {
	if s.Spec.CmdLabels == nil {
		return nil
	}
	return V2ToLabels(s.Spec.CmdLabels)
}

// SetCmdLabels sets dynamic labels.
func (s *ServerV2) SetCmdLabels(cmdLabels map[string]CommandLabel) {
	s.Spec.CmdLabels = LabelsToV2(cmdLabels)
}

// GetApps gets the list of applications this server is proxying.
func (s *ServerV2) GetApps() []*App {
	return s.Spec.Apps
}

// SetApps sets the list of applications this server is proxying.
func (s *ServerV2) SetApps(apps []*App) {
	s.Spec.Apps = apps
}

func (s *ServerV2) String() string {
	return fmt.Sprintf("Server(name=%v, namespace=%v, addr=%v, labels=%v)", s.Metadata.Name, s.Metadata.Namespace, s.Spec.Addr, s.Metadata.Labels)
}

// GetNamespace returns server namespace
func (s *ServerV2) GetNamespace() string {
	return ProcessNamespace(s.Metadata.Namespace)
}

// GetAllLabels returns the full key:value map of both static labels and
// "command labels"
func (s *ServerV2) GetAllLabels() map[string]string {
	return CombineLabels(s.Metadata.Labels, s.Spec.CmdLabels)
}

// CombineLabels combines the passed in static and dynamic labels.
func CombineLabels(static map[string]string, dynamic map[string]CommandLabelV2) map[string]string {
	lmap := make(map[string]string)
	for key, value := range static {
		lmap[key] = value
	}
	for key, cmd := range dynamic {
		lmap[key] = cmd.Result
	}
	return lmap
}

// GetKubernetesClusters returns the kubernetes clusters directly handled by this
// server.
func (s *ServerV2) GetKubernetesClusters() []*KubernetesCluster { return s.Spec.KubernetesClusters }

// SetKubernetesClusters sets the kubernetes clusters handled by this server.
func (s *ServerV2) SetKubernetesClusters(clusters []*KubernetesCluster) {
	s.Spec.KubernetesClusters = clusters
}

// MatchAgainst takes a map of labels and returns True if this server
// has ALL of them
//
// Any server matches against an empty label set
func (s *ServerV2) MatchAgainst(labels map[string]string) bool {
	if labels != nil {
		myLabels := s.GetAllLabels()
		for key, value := range labels {
			if myLabels[key] != value {
				return false
			}
		}
	}
	return true
}

// LabelsString returns a comma separated string of all labels.
func (s *ServerV2) LabelsString() string {
	return LabelsAsString(s.Metadata.Labels, s.Spec.CmdLabels)
}

// LabelsAsString combines static and dynamic labels and returns a comma
// separated string.
func LabelsAsString(static map[string]string, dynamic map[string]CommandLabelV2) string {
	labels := []string{}
	for key, val := range static {
		labels = append(labels, fmt.Sprintf("%s=%s", key, val))
	}
	for key, val := range dynamic {
		labels = append(labels, fmt.Sprintf("%s=%s", key, val.Result))
	}
	sort.Strings(labels)
	return strings.Join(labels, ",")
}

// CheckAndSetDefaults checks and set default values for any missing fields.
func (s *ServerV2) CheckAndSetDefaults() error {
	// TODO(awly): default s.Metadata.Expiry if not set (use
	// defaults.ServerAnnounceTTL).

	err := s.Metadata.CheckAndSetDefaults()
	if err != nil {
		return trace.Wrap(err)
	}
	if s.Kind == "" {
		return trace.BadParameter("server Kind is empty")
	}

	for key := range s.Spec.CmdLabels {
		if !IsValidLabelKey(key) {
			return trace.BadParameter("invalid label key: %q", key)
		}
	}
	for _, kc := range s.Spec.KubernetesClusters {
		if !validKubeClusterName.MatchString(kc.Name) {
			return trace.BadParameter("invalid kubernetes cluster name: %q", kc.Name)
		}
	}

	return nil
}

// DeepCopy creates a clone of this server value
func (s *ServerV2) DeepCopy() Server {
	return proto.Clone(s).(*ServerV2)
}

// CommandLabel is a label that has a value as a result of the
// output generated by running command, e.g. hostname
type CommandLabel interface {
	// GetPeriod returns label period
	GetPeriod() time.Duration
	// SetPeriod sets label period
	SetPeriod(time.Duration)
	// GetResult returns label result
	GetResult() string
	// SetResult sets label result
	SetResult(string)
	// GetCommand returns to execute and set as a label result
	GetCommand() []string
	// Clone returns label copy
	Clone() CommandLabel
	// Equals returns true if label is equal to the other one
	// false otherwise
	Equals(CommandLabel) bool
}

// Equals returns true if labels are equal, false otherwise
func (c *CommandLabelV2) Equals(other CommandLabel) bool {
	if c.GetPeriod() != other.GetPeriod() {
		return false
	}
	if c.GetResult() != other.GetResult() {
		return false
	}
	if !utils.StringSlicesEqual(c.GetCommand(), other.GetCommand()) {
		return false
	}
	return true
}

// Clone returns non-shallow copy of the label
func (c *CommandLabelV2) Clone() CommandLabel {
	command := make([]string, len(c.Command))
	copy(command, c.Command)
	return &CommandLabelV2{
		Command: command,
		Period:  c.Period,
		Result:  c.Result,
	}
}

// SetResult sets label result
func (c *CommandLabelV2) SetResult(r string) {
	c.Result = r
}

// SetPeriod sets label period
func (c *CommandLabelV2) SetPeriod(p time.Duration) {
	c.Period = Duration(p)
}

// GetPeriod returns label period
func (c *CommandLabelV2) GetPeriod() time.Duration {
	return c.Period.Duration()
}

// GetResult returns label result
func (c *CommandLabelV2) GetResult() string {
	return c.Result
}

// GetCommand returns to execute and set as a label result
func (c *CommandLabelV2) GetCommand() []string {
	return c.Command
}

// V2ToLabels converts concrete type to command label interface.
func V2ToLabels(l map[string]CommandLabelV2) map[string]CommandLabel {
	out := make(map[string]CommandLabel, len(l))
	for key := range l {
		val := l[key]
		out[key] = &val
	}
	return out
}

// LabelsToV2 converts labels from interface to V2 spec
func LabelsToV2(labels map[string]CommandLabel) map[string]CommandLabelV2 {
	out := make(map[string]CommandLabelV2, len(labels))
	for key, val := range labels {
		out[key] = CommandLabelV2{
			Period:  NewDuration(val.GetPeriod()),
			Result:  val.GetResult(),
			Command: val.GetCommand(),
		}
	}
	return out
}

// validKubeClusterName filters the allowed characters in kubernetes cluster
// names. We need this because cluster names are used for cert filenames on the
// client side, in the ~/.tsh directory. Restricting characters helps with
// sneaky cluster names being used for client directory traversal and exploits.
var validKubeClusterName = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
