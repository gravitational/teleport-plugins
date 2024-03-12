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
{{- if .RandomMetadataName }}
    "crypto/rand"
    "encoding/hex"
{{- end }}
	"fmt"
{{- range $i, $a := .ExtraImports }}
	"{{$a}}"
{{- end }}
{{ if .UUIDMetadataName }}
	"github.com/google/uuid"
{{- end }}
	{{.ProtoPackage}} "{{.ProtoPackagePath}}"
	{{ if .ConvertPackagePath -}}
    convert "{{.ConvertPackagePath}}"
    {{- end}}
	"github.com/gravitational/teleport/integrations/lib/backoff"
	"github.com/gravitational/trace"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jonboulle/clockwork"
{{- if .Namespaced }}
	"github.com/gravitational/teleport/api/defaults"
{{- end }}

	{{ schemaImport . }}
)

// resourceTeleport{{.Name}}Type is the resource metadata type
type resourceTeleport{{.Name}}Type struct{}

// resourceTeleport{{.Name}} is the resource
type resourceTeleport{{.Name}} struct {
	p Provider
}

// GetSchema returns the resource schema
func (r resourceTeleport{{.Name}}Type) GetSchema(ctx context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return {{.SchemaPackage}}.GenSchema{{.TypeName}}(ctx)
}

// NewResource creates the empty resource
func (r resourceTeleport{{.Name}}Type) NewResource(_ context.Context, p tfsdk.Provider) (tfsdk.Resource, diag.Diagnostics) {
	return resourceTeleport{{.Name}}{
		p: *(p.(*Provider)),
	}, nil
}

// Create creates the {{.Name}}
func (r resourceTeleport{{.Name}}) Create(ctx context.Context, req tfsdk.CreateResourceRequest, resp *tfsdk.CreateResourceResponse) {
	var err error
	if !r.p.IsConfigured(resp.Diagnostics) {
		return
	}

	var plan types.Object
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	{{.VarName}} := &{{.ProtoPackage}}.{{.TypeName}}{}
	diags = {{.SchemaPackage}}.Copy{{.TypeName}}FromTerraform(ctx, plan, {{.VarName}})
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	{{if .RandomMetadataName -}}
	if {{.VarName}}.Metadata.Name == "" {
		b := make([]byte, 32)
		_, err := rand.Read(b)
		if err != nil {
			resp.Diagnostics.AddError("Failed to generate random token", err.Error())
			return
		}
		{{.VarName}}.Metadata.Name = hex.EncodeToString(b)
	}
	{{end -}}
	{{if .UUIDMetadataName -}}
	if {{.VarName}}.Metadata.Name == "" {
		{{.VarName}}.Metadata.Name = uuid.NewString()
	}
	{{end -}}
	{{if .DefaultVersion -}}
	if {{.VarName}}.Version == "" {
		{{.VarName}}.Version = "{{.DefaultVersion}}"
	}
	{{- end}}
{{- if .ConvertPackagePath}}
	{{.VarName}}Resource, err := convert.FromProto({{.VarName}})
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading {{.Name}}", trace.Errorf("Can not convert %T to {{.TypeName}}: %s", {{.VarName}}Resource, err), "{{.Kind}}"))
		return
	}
{{- else }}
	{{.VarName}}Resource := {{.VarName}}
{{end}}

{{- if .ForceSetKind }}
	{{.VarName}}Resource.Kind = {{.ForceSetKind}}
{{- end}}

{{if .HasCheckAndSetDefaults -}}
	err = {{.VarName}}Resource.CheckAndSetDefaults()
	if err != nil {
	resp.Diagnostics.Append(diagFromWrappedErr("Error setting {{.Name}} defaults", trace.Wrap(err), "{{.Kind}}"))
	return
	}
{{- end}}

	id := {{.VarName}}Resource.Metadata.Name

	_, err = r.p.Client.{{.GetMethod}}(ctx, {{if .Namespaced}}defaults.Namespace, {{end}}id{{if ne .WithSecrets ""}}, {{.WithSecrets}}{{end}})
	if !trace.IsNotFound(err) {
		if err == nil {
			existErr := fmt.Sprintf("{{.Name}} exists in Teleport. Either remove it (tctl rm {{.Kind}}/%v)"+
				" or import it to the existing state (terraform import {{.TerraformResourceType}}.%v %v)", id, id, id)

			resp.Diagnostics.Append(diagFromErr("{{.Name}} exists in Teleport", trace.Errorf(existErr)))
			return
		}

		resp.Diagnostics.Append(diagFromWrappedErr("Error reading {{.Name}}", trace.Wrap(err), "{{.Kind}}"))
		return
	}

	{{if eq .UpsertMethodArity 2}}_, {{end}}err = r.p.Client.{{.CreateMethod}}(ctx, {{.VarName}}Resource)
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error creating {{.Name}}", trace.Wrap(err), "{{.Kind}}"))
		return
	}
	{{- if .IsPlainStruct }}
		var {{.VarName}}I *{{.ProtoPackage}}.{{.Name}}
	{{- else }}
		{{if .ConvertPackagePath -}}
	var {{.VarName}}I = {{.VarName}}Resource
		{{- else }}
	// Not really an inferface, just using the same name for easier templating.
	var {{.VarName}}I {{.ProtoPackage}}.{{ if ne .IfaceName ""}}{{.IfaceName}}{{else}}{{.Name}}{{end}}
		{{- end}}
	{{- end }}
	tries := 0
	backoff := backoff.NewDecorr(r.p.RetryConfig.Base, r.p.RetryConfig.Cap, clockwork.NewRealClock())
	for {
		tries = tries + 1
		{{.VarName}}I, err = r.p.Client.{{.GetMethod}}(ctx, {{if .Namespaced}}defaults.Namespace, {{end}}id{{if ne .WithSecrets ""}}, {{.WithSecrets}}{{end}})
		if trace.IsNotFound(err) {
			if bErr := backoff.Do(ctx); bErr != nil {
				resp.Diagnostics.Append(diagFromWrappedErr("Error reading {{.Name}}", trace.Wrap(err), "{{.Kind}}"))
				return
			}
			if tries >= r.p.RetryConfig.MaxTries {
				diagMessage := fmt.Sprintf("Error reading {{.Name}} (tried %d times) - state outdated, please import resource", tries)
				resp.Diagnostics.Append(diagFromWrappedErr(diagMessage, trace.Wrap(err), "{{.Kind}}"))
				return
			}
			continue
		}
		break
	}

	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading {{.Name}}", trace.Wrap(err), "{{.Kind}}"))
		return
	}

	{{if or .IsPlainStruct .ConvertPackagePath -}}
	{{.VarName}}Resource = {{.VarName}}I
	{{else -}}
	{{.VarName}}Resource, ok := {{.VarName}}I.(*{{.ProtoPackage}}.{{.TypeName}})
	if !ok {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading {{.Name}}", trace.Errorf("Can not convert %T to {{.TypeName}}", {{.VarName}}I), "{{.Kind}}"))
		return
	}
	{{- end}}

	{{- if .ConvertPackagePath}}
	{{.VarName}} = convert.ToProto({{.VarName}}Resource)
	{{else}}
	{{.VarName}} = {{.VarName}}Resource
	{{- end }}

	diags = {{.SchemaPackage}}.Copy{{.TypeName}}ToTerraform(ctx, {{.VarName}}, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	plan.Attrs["id"] = types.String{Value: {{.ID}}}

	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read reads teleport {{.Name}}
func (r resourceTeleport{{.Name}}) Read(ctx context.Context, req tfsdk.ReadResourceRequest, resp *tfsdk.ReadResourceResponse) {
	var state types.Object
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var id types.String
	{{- if .ConvertPackagePath}}
	diags = req.State.GetAttribute(ctx, path.Root("header").AtName("metadata").AtName("name"), &id)
	{{- else }}
	diags = req.State.GetAttribute(ctx, path.Root("metadata").AtName("name"), &id)
	{{- end}}
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	{{.VarName}}I, err := r.p.Client.{{.GetMethod}}(ctx, {{if .Namespaced}}defaults.Namespace, {{end}}id.Value{{if ne .WithSecrets ""}}, {{.WithSecrets}}{{end}})
	if trace.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}

	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading {{.Name}}", trace.Wrap(err), "{{.Kind}}"))
		return
	}
	{{if .IsPlainStruct -}}
	{{.VarName}} := {{.VarName}}I
	{{else if .ConvertPackagePath -}}
	{{.VarName}} := convert.ToProto({{.VarName}}I)
	{{ else }}
	{{.VarName}} := {{.VarName}}I.(*{{.ProtoPackage}}.{{.TypeName}})
	{{end -}}
	diags = {{.SchemaPackage}}.Copy{{.TypeName}}ToTerraform(ctx, {{.VarName}}, &state)
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

// Update updates teleport {{.Name}}
func (r resourceTeleport{{.Name}}) Update(ctx context.Context, req tfsdk.UpdateResourceRequest, resp *tfsdk.UpdateResourceResponse) {
	if !r.p.IsConfigured(resp.Diagnostics) {
		return
	}

	var plan types.Object
	diags := req.Plan.Get(ctx, &plan)

	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	{{.VarName}} := &{{.ProtoPackage}}.{{.TypeName}}{}
	diags = {{.SchemaPackage}}.Copy{{.TypeName}}FromTerraform(ctx, plan, {{.VarName}})
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

{{- if .ConvertPackagePath}}
	{{.VarName}}Resource, err := convert.FromProto({{.VarName}})
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading {{.Name}}", trace.Errorf("Can not convert %T to {{.TypeName}}: %s", {{.VarName}}Resource, err), "{{.Kind}}"))
		return
	}
{{- else }}
	{{.VarName}}Resource := {{.VarName}}
{{end}}

	{{if .HasCheckAndSetDefaults -}}
	if err := {{.VarName}}Resource.CheckAndSetDefaults(); err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error updating {{.Name}}", err, "{{.Kind}}"))
		return
	}
	{{- end}}
	name := {{.VarName}}Resource.Metadata.Name

	{{.VarName}}Before, err := r.p.Client.{{.GetMethod}}(ctx, {{if .Namespaced}}defaults.Namespace, {{end}}name{{if ne .WithSecrets ""}}, {{.WithSecrets}}{{end}})
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading {{.Name}}", err, "{{.Kind}}"))
		return
	}
	{{- $VarName := .VarName }}
	{{- range $field := .PropagatedFields }}
	{{ $VarName }}Resource.{{ $field }} = {{ $VarName }}Before.{{ $field }}
	{{- end }}

	{{if eq .UpsertMethodArity 2}}_, {{end}}err = r.p.Client.{{.UpdateMethod}}(ctx, {{.VarName}}Resource)
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error updating {{.Name}}", err, "{{.Kind}}"))
		return
	}

	{{- if .IsPlainStruct }}
		var {{.VarName}}I *{{.ProtoPackage}}.{{.Name}}
	{{- else }}
		{{if .ConvertPackagePath -}}
	var {{.VarName}}I = {{.VarName}}Resource
		{{- else }}
	// Not really an inferface, just using the same name for easier templating.
	var {{.VarName}}I {{.ProtoPackage}}.{{ if ne .IfaceName ""}}{{.IfaceName}}{{else}}{{.Name}}{{end}}
		{{- end}}
	{{- end }}

	tries := 0
	backoff := backoff.NewDecorr(r.p.RetryConfig.Base, r.p.RetryConfig.Cap, clockwork.NewRealClock())
	for {
		tries = tries + 1
		{{.VarName}}I, err = r.p.Client.{{.GetMethod}}(ctx, {{if .Namespaced}}defaults.Namespace, {{end}}name{{if ne .WithSecrets ""}}, {{.WithSecrets}}{{end}})
		if err != nil {
			resp.Diagnostics.Append(diagFromWrappedErr("Error reading {{.Name}}", err, "{{.Kind}}"))
			return
		}
		if {{.VarName}}Before.GetMetadata().ID != {{.VarName}}I.GetMetadata().ID || {{.HasStaticID}} {
			break
		}

		if err := backoff.Do(ctx); err != nil {
			resp.Diagnostics.Append(diagFromWrappedErr("Error reading {{.Name}}", trace.Wrap(err), "{{.Kind}}"))
			return
		}
		if tries >= r.p.RetryConfig.MaxTries {
			diagMessage := fmt.Sprintf("Error reading {{.Name}} (tried %d times) - state outdated, please import resource", tries)
			resp.Diagnostics.AddError(diagMessage, "{{.Kind}}")
			return
		}
	}

	{{if or .IsPlainStruct .ConvertPackagePath -}}
	{{.VarName}}Resource = {{.VarName}}I
	{{else -}}
	{{.VarName}}Resource, ok := {{.VarName}}I.(*{{.ProtoPackage}}.{{.TypeName}})
	if !ok {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading {{.Name}}", trace.Errorf("Can not convert %T to {{.TypeName}}", {{.VarName}}I), "{{.Kind}}"))
		return
	}
	{{- end}}
	diags = {{.SchemaPackage}}.Copy{{.TypeName}}ToTerraform(ctx, {{.VarName}}, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Delete deletes Teleport {{.Name}}
func (r resourceTeleport{{.Name}}) Delete(ctx context.Context, req tfsdk.DeleteResourceRequest, resp *tfsdk.DeleteResourceResponse) {
	var id types.String
	{{- if .ConvertPackagePath}}
	diags := req.State.GetAttribute(ctx, path.Root("header").AtName("metadata").AtName("name"), &id)
	{{- else }}
	diags := req.State.GetAttribute(ctx, path.Root("metadata").AtName("name"), &id)
	{{- end}}
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.p.Client.{{.DeleteMethod}}(ctx, {{if .Namespaced}}defaults.Namespace, {{end}}id.Value)
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error deleting {{.TypeName}}", trace.Wrap(err), "{{.Kind}}"))
		return
	}

	resp.State.RemoveResource(ctx)
}

// ImportState imports {{.Name}} state
func (r resourceTeleport{{.Name}}) ImportState(ctx context.Context, req tfsdk.ImportResourceStateRequest, resp *tfsdk.ImportResourceStateResponse) {
	{{.VarName}}, err := r.p.Client.{{.GetMethod}}(ctx, {{if .Namespaced}}defaults.Namespace, {{end}}req.ID{{if ne .WithSecrets ""}}, {{.WithSecrets}}{{end}})
	if err != nil {
		resp.Diagnostics.Append(diagFromWrappedErr("Error reading {{.Name}}", trace.Wrap(err), "{{.Kind}}"))
		return
	}

	{{if .IsPlainStruct -}}
	{{.VarName}}Resource := {{.VarName}}
	{{else if .ConvertPackagePath -}}
	{{.VarName}}Resource := convert.ToProto({{.VarName}})
	{{else}}
	{{.VarName}}Resource := {{.VarName}}.(*{{.ProtoPackage}}.{{.TypeName}})
	{{- end}}

	var state types.Object

	diags := resp.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = {{.SchemaPackage}}.Copy{{.TypeName}}ToTerraform(ctx, {{.VarName}}Resource, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

{{- if or .IsPlainStruct .ConvertPackagePath}}
	id := {{.VarName}}.Metadata.Name
{{- else }}
	id := {{.VarName}}Resource.GetName()
{{- end}}

	state.Attrs["id"] = types.String{Value: id}

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}
