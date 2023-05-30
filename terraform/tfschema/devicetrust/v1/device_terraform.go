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
// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: teleport/legacy/types/device.proto

package v1

import (
	context "context"
	fmt "fmt"
	math "math"
	time "time"

	_ "github.com/gogo/protobuf/gogoproto"
	proto "github.com/gogo/protobuf/proto"
	github_com_gravitational_teleport_api_types "github.com/gravitational/teleport/api/types"
	github_com_hashicorp_terraform_plugin_framework_attr "github.com/hashicorp/terraform-plugin-framework/attr"
	github_com_hashicorp_terraform_plugin_framework_diag "github.com/hashicorp/terraform-plugin-framework/diag"
	github_com_hashicorp_terraform_plugin_framework_tfsdk "github.com/hashicorp/terraform-plugin-framework/tfsdk"
	github_com_hashicorp_terraform_plugin_framework_types "github.com/hashicorp/terraform-plugin-framework/types"
	github_com_hashicorp_terraform_plugin_go_tftypes "github.com/hashicorp/terraform-plugin-go/tftypes"
	_ "google.golang.org/protobuf/types/known/timestamppb"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf
var _ = time.Kitchen

// GenSchemaDeviceV1 returns tfsdk.Schema definition for DeviceV1
func GenSchemaDeviceV1(ctx context.Context) (github_com_hashicorp_terraform_plugin_framework_tfsdk.Schema, github_com_hashicorp_terraform_plugin_framework_diag.Diagnostics) {
	return github_com_hashicorp_terraform_plugin_framework_tfsdk.Schema{Attributes: map[string]github_com_hashicorp_terraform_plugin_framework_tfsdk.Attribute{
		"id": {
			Computed:      true,
			Optional:      false,
			PlanModifiers: []github_com_hashicorp_terraform_plugin_framework_tfsdk.AttributePlanModifier{github_com_hashicorp_terraform_plugin_framework_tfsdk.UseStateForUnknown()},
			Required:      false,
			Type:          github_com_hashicorp_terraform_plugin_framework_types.StringType,
		},
		"kind": {
			Computed:      true,
			Description:   "Kind is a resource kind",
			Optional:      true,
			PlanModifiers: []github_com_hashicorp_terraform_plugin_framework_tfsdk.AttributePlanModifier{github_com_hashicorp_terraform_plugin_framework_tfsdk.UseStateForUnknown()},
			Type:          github_com_hashicorp_terraform_plugin_framework_types.StringType,
		},
		"metadata": {
			Attributes: github_com_hashicorp_terraform_plugin_framework_tfsdk.SingleNestedAttributes(map[string]github_com_hashicorp_terraform_plugin_framework_tfsdk.Attribute{
				"labels": {
					Description: "Labels is a set of labels",
					Optional:    true,
					Type:        github_com_hashicorp_terraform_plugin_framework_types.MapType{ElemType: github_com_hashicorp_terraform_plugin_framework_types.StringType},
				},
				"name": {
					Description: "Name is an object name",
					Optional:    true,
					Type:        github_com_hashicorp_terraform_plugin_framework_types.StringType,
				},
			}),
			Computed:      true,
			Description:   "Metadata is resource metadata",
			Optional:      true,
			PlanModifiers: []github_com_hashicorp_terraform_plugin_framework_tfsdk.AttributePlanModifier{github_com_hashicorp_terraform_plugin_framework_tfsdk.UseStateForUnknown()},
		},
		"spec": {
			Attributes: github_com_hashicorp_terraform_plugin_framework_tfsdk.SingleNestedAttributes(map[string]github_com_hashicorp_terraform_plugin_framework_tfsdk.Attribute{
				"asset_tag": {
					Description:   "",
					PlanModifiers: []github_com_hashicorp_terraform_plugin_framework_tfsdk.AttributePlanModifier{github_com_hashicorp_terraform_plugin_framework_tfsdk.RequiresReplace()},
					Required:      true,
					Type:          github_com_hashicorp_terraform_plugin_framework_types.StringType,
				},
				"enroll_status": {
					Computed:      true,
					Description:   "",
					Optional:      true,
					PlanModifiers: []github_com_hashicorp_terraform_plugin_framework_tfsdk.AttributePlanModifier{github_com_hashicorp_terraform_plugin_framework_tfsdk.UseStateForUnknown()},
					Type:          github_com_hashicorp_terraform_plugin_framework_types.StringType,
				},
				"os_type": {
					Description: "",
					Required:    true,
					Type:        github_com_hashicorp_terraform_plugin_framework_types.StringType,
				},
			}),
			Description: "Specification of the device.",
			Optional:    true,
		},
		"version": {
			Computed:      true,
			Description:   "Version is version",
			Optional:      true,
			PlanModifiers: []github_com_hashicorp_terraform_plugin_framework_tfsdk.AttributePlanModifier{github_com_hashicorp_terraform_plugin_framework_tfsdk.UseStateForUnknown()},
			Type:          github_com_hashicorp_terraform_plugin_framework_types.StringType,
		},
	}}, nil
}

// CopyDeviceV1FromTerraform copies contents of the source Terraform object into a target struct
func CopyDeviceV1FromTerraform(_ context.Context, tf github_com_hashicorp_terraform_plugin_framework_types.Object, obj *github_com_gravitational_teleport_api_types.DeviceV1) github_com_hashicorp_terraform_plugin_framework_diag.Diagnostics {
	var diags github_com_hashicorp_terraform_plugin_framework_diag.Diagnostics
	{
		a, ok := tf.Attrs["kind"]
		if !ok {
			diags.Append(attrReadMissingDiag{"DeviceV1.Kind"})
		} else {
			v, ok := a.(github_com_hashicorp_terraform_plugin_framework_types.String)
			if !ok {
				diags.Append(attrReadConversionFailureDiag{"DeviceV1.Kind", "github.com/hashicorp/terraform-plugin-framework/types.String"})
			} else {
				var t string
				if !v.Null && !v.Unknown {
					t = string(v.Value)
				}
				obj.Kind = t
			}
		}
	}
	{
		a, ok := tf.Attrs["version"]
		if !ok {
			diags.Append(attrReadMissingDiag{"DeviceV1.Version"})
		} else {
			v, ok := a.(github_com_hashicorp_terraform_plugin_framework_types.String)
			if !ok {
				diags.Append(attrReadConversionFailureDiag{"DeviceV1.Version", "github.com/hashicorp/terraform-plugin-framework/types.String"})
			} else {
				var t string
				if !v.Null && !v.Unknown {
					t = string(v.Value)
				}
				obj.Version = t
			}
		}
	}
	{
		a, ok := tf.Attrs["metadata"]
		if !ok {
			diags.Append(attrReadMissingDiag{"DeviceV1.Metadata"})
		} else {
			v, ok := a.(github_com_hashicorp_terraform_plugin_framework_types.Object)
			if !ok {
				diags.Append(attrReadConversionFailureDiag{"DeviceV1.Metadata", "github.com/hashicorp/terraform-plugin-framework/types.Object"})
			} else {
				obj.Metadata = github_com_gravitational_teleport_api_types.Metadata{}
				if !v.Null && !v.Unknown {
					tf := v
					obj := &obj.Metadata
					{
						a, ok := tf.Attrs["name"]
						if !ok {
							diags.Append(attrReadMissingDiag{"DeviceV1.Metadata.Name"})
						} else {
							v, ok := a.(github_com_hashicorp_terraform_plugin_framework_types.String)
							if !ok {
								diags.Append(attrReadConversionFailureDiag{"DeviceV1.Metadata.Name", "github.com/hashicorp/terraform-plugin-framework/types.String"})
							} else {
								var t string
								if !v.Null && !v.Unknown {
									t = string(v.Value)
								}
								obj.Name = t
							}
						}
					}
					{
						a, ok := tf.Attrs["labels"]
						if !ok {
							diags.Append(attrReadMissingDiag{"DeviceV1.Metadata.Labels"})
						} else {
							v, ok := a.(github_com_hashicorp_terraform_plugin_framework_types.Map)
							if !ok {
								diags.Append(attrReadConversionFailureDiag{"DeviceV1.Metadata.Labels", "github.com/hashicorp/terraform-plugin-framework/types.Map"})
							} else {
								obj.Labels = make(map[string]string, len(v.Elems))
								if !v.Null && !v.Unknown {
									for k, a := range v.Elems {
										v, ok := a.(github_com_hashicorp_terraform_plugin_framework_types.String)
										if !ok {
											diags.Append(attrReadConversionFailureDiag{"DeviceV1.Metadata.Labels", "github_com_hashicorp_terraform_plugin_framework_types.String"})
										} else {
											var t string
											if !v.Null && !v.Unknown {
												t = string(v.Value)
											}
											obj.Labels[k] = t
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
	{
		a, ok := tf.Attrs["spec"]
		if !ok {
			diags.Append(attrReadMissingDiag{"DeviceV1.spec"})
		} else {
			v, ok := a.(github_com_hashicorp_terraform_plugin_framework_types.Object)
			if !ok {
				diags.Append(attrReadConversionFailureDiag{"DeviceV1.spec", "github.com/hashicorp/terraform-plugin-framework/types.Object"})
			} else {
				obj.Spec = nil
				if !v.Null && !v.Unknown {
					tf := v
					obj.Spec = &github_com_gravitational_teleport_api_types.DeviceSpec{}
					obj := obj.Spec
					{
						a, ok := tf.Attrs["os_type"]
						if !ok {
							diags.Append(attrReadMissingDiag{"DeviceV1.spec.os_type"})
						} else {
							v, ok := a.(github_com_hashicorp_terraform_plugin_framework_types.String)
							if !ok {
								diags.Append(attrReadConversionFailureDiag{"DeviceV1.spec.os_type", "github.com/hashicorp/terraform-plugin-framework/types.String"})
							} else {
								var t string
								if !v.Null && !v.Unknown {
									t = string(v.Value)
								}
								obj.OsType = t
							}
						}
					}
					{
						a, ok := tf.Attrs["asset_tag"]
						if !ok {
							diags.Append(attrReadMissingDiag{"DeviceV1.spec.asset_tag"})
						} else {
							v, ok := a.(github_com_hashicorp_terraform_plugin_framework_types.String)
							if !ok {
								diags.Append(attrReadConversionFailureDiag{"DeviceV1.spec.asset_tag", "github.com/hashicorp/terraform-plugin-framework/types.String"})
							} else {
								var t string
								if !v.Null && !v.Unknown {
									t = string(v.Value)
								}
								obj.AssetTag = t
							}
						}
					}
					{
						a, ok := tf.Attrs["enroll_status"]
						if !ok {
							diags.Append(attrReadMissingDiag{"DeviceV1.spec.enroll_status"})
						} else {
							v, ok := a.(github_com_hashicorp_terraform_plugin_framework_types.String)
							if !ok {
								diags.Append(attrReadConversionFailureDiag{"DeviceV1.spec.enroll_status", "github.com/hashicorp/terraform-plugin-framework/types.String"})
							} else {
								var t string
								if !v.Null && !v.Unknown {
									t = string(v.Value)
								}
								obj.EnrollStatus = t
							}
						}
					}
				}
			}
		}
	}
	return diags
}

// CopyDeviceV1ToTerraform copies contents of the source Terraform object into a target struct
func CopyDeviceV1ToTerraform(ctx context.Context, obj github_com_gravitational_teleport_api_types.DeviceV1, tf *github_com_hashicorp_terraform_plugin_framework_types.Object) github_com_hashicorp_terraform_plugin_framework_diag.Diagnostics {
	var diags github_com_hashicorp_terraform_plugin_framework_diag.Diagnostics
	tf.Null = false
	tf.Unknown = false
	if tf.Attrs == nil {
		tf.Attrs = make(map[string]github_com_hashicorp_terraform_plugin_framework_attr.Value)
	}
	{
		t, ok := tf.AttrTypes["kind"]
		if !ok {
			diags.Append(attrWriteMissingDiag{"DeviceV1.Kind"})
		} else {
			v, ok := tf.Attrs["kind"].(github_com_hashicorp_terraform_plugin_framework_types.String)
			if !ok {
				i, err := t.ValueFromTerraform(ctx, github_com_hashicorp_terraform_plugin_go_tftypes.NewValue(t.TerraformType(ctx), nil))
				if err != nil {
					diags.Append(attrWriteGeneralError{"DeviceV1.Kind", err})
				}
				v, ok = i.(github_com_hashicorp_terraform_plugin_framework_types.String)
				if !ok {
					diags.Append(attrWriteConversionFailureDiag{"DeviceV1.Kind", "github.com/hashicorp/terraform-plugin-framework/types.String"})
				}
				v.Null = string(obj.Kind) == ""
			}
			v.Value = string(obj.Kind)
			v.Unknown = false
			tf.Attrs["kind"] = v
		}
	}
	{
		t, ok := tf.AttrTypes["version"]
		if !ok {
			diags.Append(attrWriteMissingDiag{"DeviceV1.Version"})
		} else {
			v, ok := tf.Attrs["version"].(github_com_hashicorp_terraform_plugin_framework_types.String)
			if !ok {
				i, err := t.ValueFromTerraform(ctx, github_com_hashicorp_terraform_plugin_go_tftypes.NewValue(t.TerraformType(ctx), nil))
				if err != nil {
					diags.Append(attrWriteGeneralError{"DeviceV1.Version", err})
				}
				v, ok = i.(github_com_hashicorp_terraform_plugin_framework_types.String)
				if !ok {
					diags.Append(attrWriteConversionFailureDiag{"DeviceV1.Version", "github.com/hashicorp/terraform-plugin-framework/types.String"})
				}
				v.Null = string(obj.Version) == ""
			}
			v.Value = string(obj.Version)
			v.Unknown = false
			tf.Attrs["version"] = v
		}
	}
	{
		a, ok := tf.AttrTypes["metadata"]
		if !ok {
			diags.Append(attrWriteMissingDiag{"DeviceV1.Metadata"})
		} else {
			o, ok := a.(github_com_hashicorp_terraform_plugin_framework_types.ObjectType)
			if !ok {
				diags.Append(attrWriteConversionFailureDiag{"DeviceV1.Metadata", "github.com/hashicorp/terraform-plugin-framework/types.ObjectType"})
			} else {
				v, ok := tf.Attrs["metadata"].(github_com_hashicorp_terraform_plugin_framework_types.Object)
				if !ok {
					v = github_com_hashicorp_terraform_plugin_framework_types.Object{

						AttrTypes: o.AttrTypes,
						Attrs:     make(map[string]github_com_hashicorp_terraform_plugin_framework_attr.Value, len(o.AttrTypes)),
					}
				} else {
					if v.Attrs == nil {
						v.Attrs = make(map[string]github_com_hashicorp_terraform_plugin_framework_attr.Value, len(tf.AttrTypes))
					}
				}
				{
					obj := obj.Metadata
					tf := &v
					{
						t, ok := tf.AttrTypes["name"]
						if !ok {
							diags.Append(attrWriteMissingDiag{"DeviceV1.Metadata.Name"})
						} else {
							v, ok := tf.Attrs["name"].(github_com_hashicorp_terraform_plugin_framework_types.String)
							if !ok {
								i, err := t.ValueFromTerraform(ctx, github_com_hashicorp_terraform_plugin_go_tftypes.NewValue(t.TerraformType(ctx), nil))
								if err != nil {
									diags.Append(attrWriteGeneralError{"DeviceV1.Metadata.Name", err})
								}
								v, ok = i.(github_com_hashicorp_terraform_plugin_framework_types.String)
								if !ok {
									diags.Append(attrWriteConversionFailureDiag{"DeviceV1.Metadata.Name", "github.com/hashicorp/terraform-plugin-framework/types.String"})
								}
								v.Null = string(obj.Name) == ""
							}
							v.Value = string(obj.Name)
							v.Unknown = false
							tf.Attrs["name"] = v
						}
					}
					{
						a, ok := tf.AttrTypes["labels"]
						if !ok {
							diags.Append(attrWriteMissingDiag{"DeviceV1.Metadata.Labels"})
						} else {
							o, ok := a.(github_com_hashicorp_terraform_plugin_framework_types.MapType)
							if !ok {
								diags.Append(attrWriteConversionFailureDiag{"DeviceV1.Metadata.Labels", "github.com/hashicorp/terraform-plugin-framework/types.MapType"})
							} else {
								c, ok := tf.Attrs["labels"].(github_com_hashicorp_terraform_plugin_framework_types.Map)
								if !ok {
									c = github_com_hashicorp_terraform_plugin_framework_types.Map{

										ElemType: o.ElemType,
										Elems:    make(map[string]github_com_hashicorp_terraform_plugin_framework_attr.Value, len(obj.Labels)),
										Null:     true,
									}
								} else {
									if c.Elems == nil {
										c.Elems = make(map[string]github_com_hashicorp_terraform_plugin_framework_attr.Value, len(obj.Labels))
									}
								}
								if obj.Labels != nil {
									t := o.ElemType
									for k, a := range obj.Labels {
										v, ok := tf.Attrs["labels"].(github_com_hashicorp_terraform_plugin_framework_types.String)
										if !ok {
											i, err := t.ValueFromTerraform(ctx, github_com_hashicorp_terraform_plugin_go_tftypes.NewValue(t.TerraformType(ctx), nil))
											if err != nil {
												diags.Append(attrWriteGeneralError{"DeviceV1.Metadata.Labels", err})
											}
											v, ok = i.(github_com_hashicorp_terraform_plugin_framework_types.String)
											if !ok {
												diags.Append(attrWriteConversionFailureDiag{"DeviceV1.Metadata.Labels", "github.com/hashicorp/terraform-plugin-framework/types.String"})
											}
											v.Null = false
										}
										v.Value = string(a)
										v.Unknown = false
										c.Elems[k] = v
									}
									if len(obj.Labels) > 0 {
										c.Null = false
									}
								}
								c.Unknown = false
								tf.Attrs["labels"] = c
							}
						}
					}
				}
				v.Unknown = false
				tf.Attrs["metadata"] = v
			}
		}
	}
	{
		a, ok := tf.AttrTypes["spec"]
		if !ok {
			diags.Append(attrWriteMissingDiag{"DeviceV1.spec"})
		} else {
			o, ok := a.(github_com_hashicorp_terraform_plugin_framework_types.ObjectType)
			if !ok {
				diags.Append(attrWriteConversionFailureDiag{"DeviceV1.spec", "github.com/hashicorp/terraform-plugin-framework/types.ObjectType"})
			} else {
				v, ok := tf.Attrs["spec"].(github_com_hashicorp_terraform_plugin_framework_types.Object)
				if !ok {
					v = github_com_hashicorp_terraform_plugin_framework_types.Object{

						AttrTypes: o.AttrTypes,
						Attrs:     make(map[string]github_com_hashicorp_terraform_plugin_framework_attr.Value, len(o.AttrTypes)),
					}
				} else {
					if v.Attrs == nil {
						v.Attrs = make(map[string]github_com_hashicorp_terraform_plugin_framework_attr.Value, len(tf.AttrTypes))
					}
				}
				if obj.Spec == nil {
					v.Null = true
				} else {
					obj := obj.Spec
					tf := &v
					{
						t, ok := tf.AttrTypes["os_type"]
						if !ok {
							diags.Append(attrWriteMissingDiag{"DeviceV1.spec.os_type"})
						} else {
							v, ok := tf.Attrs["os_type"].(github_com_hashicorp_terraform_plugin_framework_types.String)
							if !ok {
								i, err := t.ValueFromTerraform(ctx, github_com_hashicorp_terraform_plugin_go_tftypes.NewValue(t.TerraformType(ctx), nil))
								if err != nil {
									diags.Append(attrWriteGeneralError{"DeviceV1.spec.os_type", err})
								}
								v, ok = i.(github_com_hashicorp_terraform_plugin_framework_types.String)
								if !ok {
									diags.Append(attrWriteConversionFailureDiag{"DeviceV1.spec.os_type", "github.com/hashicorp/terraform-plugin-framework/types.String"})
								}
								v.Null = string(obj.OsType) == ""
							}
							v.Value = string(obj.OsType)
							v.Unknown = false
							tf.Attrs["os_type"] = v
						}
					}
					{
						t, ok := tf.AttrTypes["asset_tag"]
						if !ok {
							diags.Append(attrWriteMissingDiag{"DeviceV1.spec.asset_tag"})
						} else {
							v, ok := tf.Attrs["asset_tag"].(github_com_hashicorp_terraform_plugin_framework_types.String)
							if !ok {
								i, err := t.ValueFromTerraform(ctx, github_com_hashicorp_terraform_plugin_go_tftypes.NewValue(t.TerraformType(ctx), nil))
								if err != nil {
									diags.Append(attrWriteGeneralError{"DeviceV1.spec.asset_tag", err})
								}
								v, ok = i.(github_com_hashicorp_terraform_plugin_framework_types.String)
								if !ok {
									diags.Append(attrWriteConversionFailureDiag{"DeviceV1.spec.asset_tag", "github.com/hashicorp/terraform-plugin-framework/types.String"})
								}
								v.Null = string(obj.AssetTag) == ""
							}
							v.Value = string(obj.AssetTag)
							v.Unknown = false
							tf.Attrs["asset_tag"] = v
						}
					}
					{
						t, ok := tf.AttrTypes["enroll_status"]
						if !ok {
							diags.Append(attrWriteMissingDiag{"DeviceV1.spec.enroll_status"})
						} else {
							v, ok := tf.Attrs["enroll_status"].(github_com_hashicorp_terraform_plugin_framework_types.String)
							if !ok {
								i, err := t.ValueFromTerraform(ctx, github_com_hashicorp_terraform_plugin_go_tftypes.NewValue(t.TerraformType(ctx), nil))
								if err != nil {
									diags.Append(attrWriteGeneralError{"DeviceV1.spec.enroll_status", err})
								}
								v, ok = i.(github_com_hashicorp_terraform_plugin_framework_types.String)
								if !ok {
									diags.Append(attrWriteConversionFailureDiag{"DeviceV1.spec.enroll_status", "github.com/hashicorp/terraform-plugin-framework/types.String"})
								}
								v.Null = string(obj.EnrollStatus) == ""
							}
							v.Value = string(obj.EnrollStatus)
							v.Unknown = false
							tf.Attrs["enroll_status"] = v
						}
					}
				}
				v.Unknown = false
				tf.Attrs["spec"] = v
			}
		}
	}
	return diags
}

// attrReadMissingDiag represents diagnostic message on an attribute missing in the source object
type attrReadMissingDiag struct {
	Path string
}

func (d attrReadMissingDiag) Severity() github_com_hashicorp_terraform_plugin_framework_diag.Severity {
	return github_com_hashicorp_terraform_plugin_framework_diag.SeverityError
}

func (d attrReadMissingDiag) Summary() string {
	return "Error reading from Terraform object"
}

func (d attrReadMissingDiag) Detail() string {
	return fmt.Sprintf("A value for %v is missing in the source Terraform object Attrs", d.Path)
}

func (d attrReadMissingDiag) Equal(o github_com_hashicorp_terraform_plugin_framework_diag.Diagnostic) bool {
	return (d.Severity() == o.Severity()) && (d.Summary() == o.Summary()) && (d.Detail() == o.Detail())
}

// attrReadConversionFailureDiag represents diagnostic message on a failed type conversion on read
type attrReadConversionFailureDiag struct {
	Path string
	Type string
}

func (d attrReadConversionFailureDiag) Severity() github_com_hashicorp_terraform_plugin_framework_diag.Severity {
	return github_com_hashicorp_terraform_plugin_framework_diag.SeverityError
}

func (d attrReadConversionFailureDiag) Summary() string {
	return "Error reading from Terraform object"
}

func (d attrReadConversionFailureDiag) Detail() string {
	return fmt.Sprintf("A value for %v can not be converted to %v", d.Path, d.Type)
}

func (d attrReadConversionFailureDiag) Equal(o github_com_hashicorp_terraform_plugin_framework_diag.Diagnostic) bool {
	return (d.Severity() == o.Severity()) && (d.Summary() == o.Summary()) && (d.Detail() == o.Detail())
}

// attrWriteMissingDiag represents diagnostic message on an attribute missing in the target object
type attrWriteMissingDiag struct {
	Path string
}

func (d attrWriteMissingDiag) Severity() github_com_hashicorp_terraform_plugin_framework_diag.Severity {
	return github_com_hashicorp_terraform_plugin_framework_diag.SeverityError
}

func (d attrWriteMissingDiag) Summary() string {
	return "Error writing to Terraform object"
}

func (d attrWriteMissingDiag) Detail() string {
	return fmt.Sprintf("A value for %v is missing in the source Terraform object AttrTypes", d.Path)
}

func (d attrWriteMissingDiag) Equal(o github_com_hashicorp_terraform_plugin_framework_diag.Diagnostic) bool {
	return (d.Severity() == o.Severity()) && (d.Summary() == o.Summary()) && (d.Detail() == o.Detail())
}

// attrWriteConversionFailureDiag represents diagnostic message on a failed type conversion on write
type attrWriteConversionFailureDiag struct {
	Path string
	Type string
}

func (d attrWriteConversionFailureDiag) Severity() github_com_hashicorp_terraform_plugin_framework_diag.Severity {
	return github_com_hashicorp_terraform_plugin_framework_diag.SeverityError
}

func (d attrWriteConversionFailureDiag) Summary() string {
	return "Error writing to Terraform object"
}

func (d attrWriteConversionFailureDiag) Detail() string {
	return fmt.Sprintf("A value for %v can not be converted to %v", d.Path, d.Type)
}

func (d attrWriteConversionFailureDiag) Equal(o github_com_hashicorp_terraform_plugin_framework_diag.Diagnostic) bool {
	return (d.Severity() == o.Severity()) && (d.Summary() == o.Summary()) && (d.Detail() == o.Detail())
}

// attrWriteGeneralError represents diagnostic message on a generic error on write
type attrWriteGeneralError struct {
	Path string
	Err  error
}

func (d attrWriteGeneralError) Severity() github_com_hashicorp_terraform_plugin_framework_diag.Severity {
	return github_com_hashicorp_terraform_plugin_framework_diag.SeverityError
}

func (d attrWriteGeneralError) Summary() string {
	return "Error writing to Terraform object"
}

func (d attrWriteGeneralError) Detail() string {
	return fmt.Sprintf("%s: %s", d.Path, d.Err.Error())
}

func (d attrWriteGeneralError) Equal(o github_com_hashicorp_terraform_plugin_framework_diag.Diagnostic) bool {
	return (d.Severity() == o.Severity()) && (d.Summary() == o.Summary()) && (d.Detail() == o.Detail())
}
