// Code generated by _gen/main.go DO NOT EDIT
/*
Copyright 2015-2022 Gravitational, Inc.

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
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	{{.SchemaPackage}} "{{.SchemaPackagePath}}"
	{{if not .IsPlainStruct -}}
	{{.ProtoPackage}} "{{.ProtoPackagePath}}"
	{{end -}}
	"github.com/gravitational/trace"
)

// dataSourceTeleport{{.Name}}Type is the data source metadata type
type dataSourceTeleport{{.Name}}Type struct{}

// dataSourceTeleport{{.Name}} is the resource
type dataSourceTeleport{{.Name}} struct {
	p Provider
}

// GetSchema returns the data source schema
func (r dataSourceTeleport{{.Name}}Type) GetSchema(ctx context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return {{.SchemaPackage}}.GenSchema{{.TypeName}}(ctx)
}

// NewDataSource creates the empty data source
func (r dataSourceTeleport{{.Name}}Type) NewDataSource(_ context.Context, p tfsdk.Provider) (tfsdk.DataSource, diag.Diagnostics) {
	return dataSourceTeleport{{.Name}}{
		p: *(p.(*Provider)),
	}, nil
}

// Read reads teleport {{.Name}}
func (r dataSourceTeleport{{.Name}}) Read(ctx context.Context, req tfsdk.ReadDataSourceRequest, resp *tfsdk.ReadDataSourceResponse) {
	{{.VarName}}I, err := r.p.Client.{{.GetMethod}}(ctx)
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading {{.Name}}", trace.Wrap(err), "{{.Kind}}"))
		return
	}

    var state types.Object
	{{if .IsPlainStruct -}}
	{{.VarName}} := {{.VarName}}I
	{{else -}}
	{{.VarName}} := {{.VarName}}I.(*{{.ProtoPackage}}.{{.TypeName}})
	{{end -}}
	diags := {{.SchemaPackage}}.Copy{{.TypeName}}ToTerraform(ctx, *{{.VarName}}, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}
