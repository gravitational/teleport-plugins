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

package types

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/gravitational/trace"
)

// Database represents a database proxied by a database server.
type Database interface {
	// Resource provides common resource methods.
	Resource
	// GetNamespace returns the database namespace.
	GetNamespace() string
	// GetStaticLabels returns the database static labels.
	GetStaticLabels() map[string]string
	// SetStaticLabels sets the database static labels.
	SetStaticLabels(map[string]string)
	// GetDynamicLabels returns the database dynamic labels.
	GetDynamicLabels() map[string]CommandLabel
	// SetDynamicLabels sets the database dynamic labels.
	SetDynamicLabels(map[string]CommandLabel)
	// GetAllLabels returns combined static and dynamic labels.
	GetAllLabels() map[string]string
	// LabelsString returns all labels as a string.
	LabelsString() string
	// String returns string representation of the database.
	String() string
	// GetDescription returns the database description.
	GetDescription() string
	// GetProtocol returns the database protocol.
	GetProtocol() string
	// GetURI returns the database connection endpoint.
	GetURI() string
	// GetCA returns the database CA certificate.
	GetCA() string
	// SetCA sets the database CA certificate.
	SetCA(string)
	// GetAWS returns AWS information for RDS/Aurora databases.
	GetAWS() AWS
	// GetGCP returns GCP information for Cloud SQL databases.
	GetGCP() GCPCloudSQL
	// GetType returns the database authentication type: self-hosted, RDS, Redshift or Cloud SQL.
	GetType() string
	// IsRDS returns true if this is an RDS/Aurora database.
	IsRDS() bool
	// IsRedshift returns true if this is a Redshift database.
	IsRedshift() bool
	// IsCloudSQL returns true if this is a Cloud SQL database.
	IsCloudSQL() bool
}

// NewDatabaseV3 creates a new database resource.
func NewDatabaseV3(meta Metadata, spec DatabaseSpecV3) (*DatabaseV3, error) {
	database := &DatabaseV3{
		Metadata: meta,
		Spec:     spec,
	}
	if err := database.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	return database, nil
}

// GetVersion returns the database resource version.
func (d *DatabaseV3) GetVersion() string {
	return d.Version
}

// GetKind returns the database resource kind.
func (d *DatabaseV3) GetKind() string {
	return d.Kind
}

// GetSubKind returns the database resource subkind.
func (d *DatabaseV3) GetSubKind() string {
	return d.SubKind
}

// SetSubKind sets the database resource subkind.
func (d *DatabaseV3) SetSubKind(sk string) {
	d.SubKind = sk
}

// GetResourceID returns the database resource ID.
func (d *DatabaseV3) GetResourceID() int64 {
	return d.Metadata.ID
}

// SetResourceID sets the database resource ID.
func (d *DatabaseV3) SetResourceID(id int64) {
	d.Metadata.ID = id
}

// GetMetadata returns the database resource metadata.
func (d *DatabaseV3) GetMetadata() Metadata {
	return d.Metadata
}

// GetNamespace returns the database resource namespace.
func (d *DatabaseV3) GetNamespace() string {
	return d.Metadata.Namespace
}

// SetExpiry sets the database resource expiration time.
func (d *DatabaseV3) SetExpiry(expiry time.Time) {
	d.Metadata.SetExpiry(expiry)
}

// Expiry returns the database resource expiration time.
func (d *DatabaseV3) Expiry() time.Time {
	return d.Metadata.Expiry()
}

// GetName returns the database resource name.
func (d *DatabaseV3) GetName() string {
	return d.Metadata.Name
}

// SetName sets the database resource name.
func (d *DatabaseV3) SetName(name string) {
	d.Metadata.Name = name
}

// GetStaticLabels returns the database static labels.
func (d *DatabaseV3) GetStaticLabels() map[string]string {
	return d.Metadata.Labels
}

// SetStaticLabels sets the database static labels.
func (d *DatabaseV3) SetStaticLabels(sl map[string]string) {
	d.Metadata.Labels = sl
}

// GetDynamicLabels returns the database dynamic labels.
func (d *DatabaseV3) GetDynamicLabels() map[string]CommandLabel {
	if d.Spec.DynamicLabels == nil {
		return nil
	}
	return V2ToLabels(d.Spec.DynamicLabels)
}

// SetDynamicLabels sets the database dynamic labels
func (d *DatabaseV3) SetDynamicLabels(dl map[string]CommandLabel) {
	d.Spec.DynamicLabels = LabelsToV2(dl)
}

// GetAllLabels returns the database combined static and dynamic labels.
func (d *DatabaseV3) GetAllLabels() map[string]string {
	return CombineLabels(d.Metadata.Labels, d.Spec.DynamicLabels)
}

// LabelsString returns all database labels as a string.
func (d *DatabaseV3) LabelsString() string {
	return LabelsAsString(d.Metadata.Labels, d.Spec.DynamicLabels)
}

// GetDescription returns the database description.
func (d *DatabaseV3) GetDescription() string {
	return d.Metadata.Description
}

// GetProtocol returns the database protocol.
func (d *DatabaseV3) GetProtocol() string {
	return d.Spec.Protocol
}

// GetURI returns the database connection address.
func (d *DatabaseV3) GetURI() string {
	return d.Spec.URI
}

// GetCA returns the database CA certificate.
func (d *DatabaseV3) GetCA() string {
	return d.Spec.CACert
}

// SetCA sets the database CA certificate.
func (d *DatabaseV3) SetCA(bytes string) {
	d.Spec.CACert = bytes
}

// GetAWS returns AWS information for RDS/Aurora/Redshift databases.
func (d *DatabaseV3) GetAWS() AWS {
	return d.Spec.AWS
}

// GetGCP returns GCP information for Cloud SQL databases.
func (d *DatabaseV3) GetGCP() GCPCloudSQL {
	return d.Spec.GCP
}

// IsRDS returns true if this is a AWS RDS/Aurora instance.
func (d *DatabaseV3) IsRDS() bool {
	return d.GetType() == DatabaseTypeRDS
}

// IsRedshift returns true if this is a Redshift database instance.
func (d *DatabaseV3) IsRedshift() bool {
	return d.GetType() == DatabaseTypeRedshift
}

// IsCloudSQL returns true if this database is a Cloud SQL instance.
func (d *DatabaseV3) IsCloudSQL() bool {
	return d.GetType() == DatabaseTypeCloudSQL
}

// GetType returns the database type.
func (d *DatabaseV3) GetType() string {
	if d.Spec.AWS.Redshift.ClusterID != "" {
		return DatabaseTypeRedshift
	}
	if d.Spec.AWS.Region != "" {
		return DatabaseTypeRDS
	}
	if d.Spec.GCP.ProjectID != "" {
		return DatabaseTypeCloudSQL
	}
	return DatabaseTypeSelfHosted
}

// String returns the database string representation.
func (d *DatabaseV3) String() string {
	return fmt.Sprintf("Database(Name=%v, Type=%v, Labels=%v)",
		d.GetName(), d.GetType(), d.GetStaticLabels())
}

// setStaticFields sets static resource header and metadata fields.
func (d *DatabaseV3) setStaticFields() {
	d.Kind = KindDatabase
	d.Version = V3
}

// CheckAndSetDefaults checks and sets default values for any missing fields.
func (d *DatabaseV3) CheckAndSetDefaults() error {
	d.setStaticFields()
	if err := d.Metadata.CheckAndSetDefaults(); err != nil {
		return trace.Wrap(err)
	}
	for key := range d.Spec.DynamicLabels {
		if !IsValidLabelKey(key) {
			return trace.BadParameter("database %q invalid label key: %q", d.GetName(), key)
		}
	}
	if d.Spec.Protocol == "" {
		return trace.BadParameter("database %q protocol is empty", d.GetName())
	}
	if d.Spec.URI == "" {
		return trace.BadParameter("database %q URI is empty", d.GetName())
	}
	// In case of RDS, Aurora or Redshift, AWS information such as region or
	// cluster ID can be extracted from the endpoint if not provided.
	switch {
	case strings.Contains(d.Spec.URI, rdsEndpointSuffix):
		region, err := parseRDSEndpoint(d.Spec.URI)
		if err != nil {
			return trace.Wrap(err)
		}
		if d.Spec.AWS.Region == "" {
			d.Spec.AWS.Region = region
		}
	case strings.Contains(d.Spec.URI, redshiftEndpointSuffix):
		clusterID, region, err := parseRedshiftEndpoint(d.Spec.URI)
		if err != nil {
			return trace.Wrap(err)
		}
		if d.Spec.AWS.Redshift.ClusterID == "" {
			d.Spec.AWS.Redshift.ClusterID = clusterID
		}
		if d.Spec.AWS.Region == "" {
			d.Spec.AWS.Region = region
		}
	}
	return nil
}

// parseRDSEndpoint extracts region from the provided RDS endpoint.
func parseRDSEndpoint(endpoint string) (region string, err error) {
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		return "", trace.Wrap(err)
	}
	// RDS/Aurora endpoint looks like this:
	// aurora-instance-1.abcdefghijklmnop.us-west-1.rds.amazonaws.com
	parts := strings.Split(host, ".")
	if !strings.HasSuffix(host, rdsEndpointSuffix) || len(parts) != 6 {
		return "", trace.BadParameter("failed to parse %v as RDS endpoint", endpoint)
	}
	return parts[2], nil
}

// parseRedshiftEndpoint extracts cluster ID and region from the provided Redshift endpoint.
func parseRedshiftEndpoint(endpoint string) (clusterID, region string, err error) {
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		return "", "", trace.Wrap(err)
	}
	// Redshift endpoint looks like this:
	// redshift-cluster-1.abcdefghijklmnop.us-east-1.rds.amazonaws.com
	parts := strings.Split(host, ".")
	if !strings.HasSuffix(host, redshiftEndpointSuffix) || len(parts) != 6 {
		return "", "", trace.BadParameter("failed to parse %v as Redshift endpoint", endpoint)
	}
	return parts[0], parts[2], nil
}

const (
	// DatabaseTypeSelfHosted is the self-hosted type of database.
	DatabaseTypeSelfHosted = "self-hosted"
	// DatabaseTypeRDS is AWS-hosted RDS or Aurora database.
	DatabaseTypeRDS = "rds"
	// DatabaseTypeRedshift is AWS Redshift database.
	DatabaseTypeRedshift = "redshift"
	// DatabaseTypeCloudSQL is GCP-hosted Cloud SQL database.
	DatabaseTypeCloudSQL = "gcp"
)

// DeduplicateDatabases deduplicates databases by name.
func DeduplicateDatabases(databases []Database) (result []Database) {
	seen := make(map[string]struct{})
	for _, database := range databases {
		if _, ok := seen[database.GetName()]; ok {
			continue
		}
		seen[database.GetName()] = struct{}{}
		result = append(result, database)
	}
	return result
}

const (
	// rdsEndpointSuffix is the RDS/Aurora endpoint suffix.
	rdsEndpointSuffix = ".rds.amazonaws.com"
	// redshiftEndpointSuffix is the Redshift endpoint suffix.
	redshiftEndpointSuffix = ".redshift.amazonaws.com"
)
